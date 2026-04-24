package acroform

import (
	"fmt"
	"strings"

	"github.com/benedoc-inc/pdfer/core/write"
)

// FieldBuilder helps build AcroForm fields
type FieldBuilder struct {
	writer  *write.PDFWriter
	fields  []*FieldDef
	fontNum int // Helvetica font object number (shared across all fields)
}

// FieldDef represents a field definition for creation
type FieldDef struct {
	Name         string
	Type         string // Tx, Btn, Ch
	Value        interface{}
	DefaultValue interface{}
	Rect         []float64 // [llx lly urx ury]
	Page         int       // 0-based page index
	Flags        int
	Options      []string // For choice fields
	MaxLen       int      // For text fields
	Required     bool
	ReadOnly     bool
	FontSize     float64 // 0 = auto
}

// NewFieldBuilder creates a new field builder
func NewFieldBuilder(w *write.PDFWriter) *FieldBuilder {
	return &FieldBuilder{
		writer: w,
		fields: make([]*FieldDef, 0),
	}
}

// AddTextField adds a text field to the form
func (fb *FieldBuilder) AddTextField(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{Name: name, Type: "Tx", Rect: rect, Page: page}
	fb.fields = append(fb.fields, field)
	return field
}

// AddCheckbox adds a checkbox field
func (fb *FieldBuilder) AddCheckbox(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{Name: name, Type: "Btn", Rect: rect, Page: page, Flags: 1 << 14}
	fb.fields = append(fb.fields, field)
	return field
}

// AddRadioButton adds a radio button field
func (fb *FieldBuilder) AddRadioButton(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{Name: name, Type: "Btn", Rect: rect, Page: page, Flags: (1 << 14) | (1 << 15)}
	fb.fields = append(fb.fields, field)
	return field
}

// AddChoiceField adds a combo-box (dropdown) field
func (fb *FieldBuilder) AddChoiceField(name string, rect []float64, page int, options []string) *FieldDef {
	field := &FieldDef{Name: name, Type: "Ch", Rect: rect, Page: page, Options: options, Flags: 1 << 17}
	fb.fields = append(fb.fields, field)
	return field
}

// AddButton adds a push button
func (fb *FieldBuilder) AddButton(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{Name: name, Type: "Btn", Rect: rect, Page: page, Flags: 1 << 16}
	fb.fields = append(fb.fields, field)
	return field
}

// --- Fluent setters ---

func (fd *FieldDef) SetValue(value interface{}) *FieldDef {
	fd.Value = value
	return fd
}

func (fd *FieldDef) SetDefault(value interface{}) *FieldDef {
	fd.DefaultValue = value
	return fd
}

func (fd *FieldDef) SetRequired(required bool) *FieldDef {
	fd.Required = required
	if required {
		fd.Flags |= 0x2
	} else {
		fd.Flags &^= 0x2
	}
	return fd
}

func (fd *FieldDef) SetReadOnly(readonly bool) *FieldDef {
	fd.ReadOnly = readonly
	if readonly {
		fd.Flags |= 0x1
	} else {
		fd.Flags &^= 0x1
	}
	return fd
}

func (fd *FieldDef) SetMaxLength(maxLen int) *FieldDef {
	fd.MaxLen = maxLen
	return fd
}

func (fd *FieldDef) SetFontSize(pt float64) *FieldDef {
	fd.FontSize = pt
	return fd
}

// --- Build ---

// Build creates all field objects without page linkage (useful for unit testing).
// Returns the AcroForm dict object number.
func (fb *FieldBuilder) Build() (int, error) {
	acroNum, _, err := fb.buildWithPages(nil)
	return acroNum, err
}

// buildWithPages creates field+widget objects with /P page references.
// pageObjNums is a slice of page object numbers (index 0 = first page).
// Returns the AcroForm object number and a map from page index to field object numbers.
func (fb *FieldBuilder) buildWithPages(pageObjNums []int) (int, map[int][]int, error) {
	if len(fb.fields) == 0 {
		return 0, nil, fmt.Errorf("no fields to build")
	}

	// Create one shared Helvetica font for /DA strings.
	fb.fontNum = fb.writer.AddObject([]byte(
		"<</Type/Font/Subtype/Type1/BaseFont/Helvetica/Encoding/WinAnsiEncoding>>",
	))

	fieldRefs := make([]string, 0, len(fb.fields))
	fieldsByPage := make(map[int][]int)

	for _, field := range fb.fields {
		objNum := fb.createFieldObject(field, pageObjNums)
		fieldRefs = append(fieldRefs, fmt.Sprintf("%d 0 R", objNum))
		fieldsByPage[field.Page] = append(fieldsByPage[field.Page], objNum)
	}

	// Default appearance string references the shared Helvetica font alias "Helv".
	defaultDA := "/Helv 12 Tf 0 g"

	// /DR gives the AcroForm-level font resources.
	drDict := fmt.Sprintf("<</Font<</Helv %d 0 R>>>>", fb.fontNum)

	fieldsArray := "[" + strings.Join(fieldRefs, " ") + "]"
	acroFormDict := fmt.Sprintf(
		"<</Fields %s/NeedAppearances true/DR %s/DA (%s)/Q 0>>",
		fieldsArray, drDict, defaultDA,
	)
	acroFormNum := fb.writer.AddObject([]byte(acroFormDict))

	return acroFormNum, fieldsByPage, nil
}

// createFieldObject serializes a combined field+widget annotation dict.
func (fb *FieldBuilder) createFieldObject(field *FieldDef, pageObjNums []int) int {
	var b strings.Builder

	b.WriteString("<</Type/Annot/Subtype/Widget")
	fmt.Fprintf(&b, "/FT/%s", field.Type)
	fmt.Fprintf(&b, "/T(%s)", escapeFieldStr(field.Name))

	if len(field.Rect) >= 4 {
		fmt.Fprintf(&b, "/Rect[%.2f %.2f %.2f %.2f]",
			field.Rect[0], field.Rect[1], field.Rect[2], field.Rect[3])
	}

	// /P page reference
	if pageObjNums != nil && field.Page >= 0 && field.Page < len(pageObjNums) {
		fmt.Fprintf(&b, "/P %d 0 R", pageObjNums[field.Page])
	}

	// Flags
	if field.Flags != 0 {
		fmt.Fprintf(&b, "/Ff %d", field.Flags)
	}

	switch field.Type {
	case "Tx":
		fb.writeTxKeys(&b, field)
	case "Btn":
		fb.writeBtnKeys(&b, field)
	case "Ch":
		fb.writeChKeys(&b, field)
	}

	// Print flag so the field shows when printing.
	b.WriteString("/F 4")

	b.WriteString(">>")
	return fb.writer.AddObject([]byte(b.String()))
}

func (fb *FieldBuilder) writeTxKeys(b *strings.Builder, field *FieldDef) {
	fontSize := field.FontSize
	if fontSize == 0 {
		fontSize = 12
	}
	fmt.Fprintf(b, "/DA(/Helv %.4g Tf 0 g)", fontSize)

	if field.Value != nil {
		fmt.Fprintf(b, "/V(%s)", escapeFieldStr(fmt.Sprint(field.Value)))
	}
	if field.DefaultValue != nil {
		fmt.Fprintf(b, "/DV(%s)", escapeFieldStr(fmt.Sprint(field.DefaultValue)))
	}
	if field.MaxLen > 0 {
		fmt.Fprintf(b, "/MaxLen %d", field.MaxLen)
	}
	// Solid 1pt border via /BS.
	b.WriteString("/BS<</W 1/S/S>>")
}

func (fb *FieldBuilder) writeBtnKeys(b *strings.Builder, field *FieldDef) {
	isPushButton := field.Flags&(1<<16) != 0
	if isPushButton {
		// Push buttons don't have a value; they trigger actions.
		return
	}

	// Checkbox or radio button.
	checked := false
	if field.Value != nil {
		if v, ok := field.Value.(bool); ok {
			checked = v
		} else if s, ok := field.Value.(string); ok {
			checked = s == "Yes" || s == "On" || s == "true"
		}
	}

	checkedState := "Off"
	if checked {
		checkedState = "Yes"
	}
	fmt.Fprintf(b, "/V/%s/DV/Off/AS/%s", checkedState, checkedState)

	// Appearance streams: Yes = checkmark, Off = empty.
	w := field.Rect[2] - field.Rect[0]
	h := field.Rect[3] - field.Rect[1]
	yesNum := fb.checkmarkAppearance(w, h)
	offNum := fb.emptyAppearance(w, h)
	fmt.Fprintf(b, "/AP<</N<</Yes %d 0 R/Off %d 0 R>>>>", yesNum, offNum)
}

func (fb *FieldBuilder) writeChKeys(b *strings.Builder, field *FieldDef) {
	fontSize := field.FontSize
	if fontSize == 0 {
		fontSize = 12
	}
	fmt.Fprintf(b, "/DA(/Helv %.4g Tf 0 g)", fontSize)

	if len(field.Options) > 0 {
		b.WriteString("/Opt[")
		for i, opt := range field.Options {
			if i > 0 {
				b.WriteByte(' ')
			}
			fmt.Fprintf(b, "(%s)", escapeFieldStr(opt))
		}
		b.WriteByte(']')
	}
	if field.Value != nil {
		fmt.Fprintf(b, "/V(%s)", escapeFieldStr(fmt.Sprint(field.Value)))
	} else if field.DefaultValue != nil {
		fmt.Fprintf(b, "/V(%s)", escapeFieldStr(fmt.Sprint(field.DefaultValue)))
	}
	if field.DefaultValue != nil {
		fmt.Fprintf(b, "/DV(%s)", escapeFieldStr(fmt.Sprint(field.DefaultValue)))
	}
}

// checkmarkAppearance returns the object number of a simple checkmark XObject.
func (fb *FieldBuilder) checkmarkAppearance(w, h float64) int {
	// Draw a simple ✓ using two line segments scaled to BBox [0 0 w h].
	stream := fmt.Sprintf("q\n"+
		"0 g\n"+
		"%.4f w\n"+
		"%.4f %.4f m\n"+
		"%.4f %.4f l\n"+
		"%.4f %.4f l\n"+
		"S\nQ\n",
		w*0.1,          // line width
		w*0.15, h*0.50, // start
		w*0.38, h*0.24, // middle dip
		w*0.82, h*0.70, // end
	)
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    fmt.Sprintf("[0 0 %.4f %.4f]", w, h),
	}
	return fb.writer.AddStreamObject(dict, []byte(stream), false)
}

// emptyAppearance returns an empty form XObject (for unchecked state).
func (fb *FieldBuilder) emptyAppearance(w, h float64) int {
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    fmt.Sprintf("[0 0 %.4f %.4f]", w, h),
	}
	return fb.writer.AddStreamObject(dict, []byte(" "), false)
}

// escapeFieldStr escapes a string for use inside PDF parenthesised string literals.
func escapeFieldStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString("\\\\")
		case '(':
			b.WriteString("\\(")
		case ')':
			b.WriteString("\\)")
		case '\r':
			b.WriteString("\\r")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// AddAcroFormToCatalog adds AcroForm reference to catalog (kept for compatibility).
func AddAcroFormToCatalog(w *write.PDFWriter, catalogNum, acroFormNum int) error {
	catalogData, err := w.GetObject(catalogNum)
	if err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}
	catalogStr := string(catalogData)
	if strings.Contains(catalogStr, "/AcroForm") {
		return nil
	}
	dictEnd := strings.LastIndex(catalogStr, ">>")
	if dictEnd == -1 {
		return fmt.Errorf("invalid catalog dictionary")
	}
	w.SetObject(catalogNum, []byte(
		catalogStr[:dictEnd]+fmt.Sprintf("/AcroForm %d 0 R", acroFormNum)+catalogStr[dictEnd:],
	))
	return nil
}
