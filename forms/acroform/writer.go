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
	BorderStyle  string  // PDF /BS /S value: "S" solid (default), "U" underline, "D" dashed, "B" beveled, "I" inset
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

// AddUnderlineTextField adds a text field with only a bottom border (underline style).
func (fb *FieldBuilder) AddUnderlineTextField(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{Name: name, Type: "Tx", Rect: rect, Page: page, BorderStyle: "U"}
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

// AddListBox adds a list box (scrollable selection, always visible).
// Unlike a combo box, a list box displays multiple options simultaneously.
func (fb *FieldBuilder) AddListBox(name string, rect []float64, page int, options []string) *FieldDef {
	// Ch type without the Combo flag (1<<17)
	field := &FieldDef{Name: name, Type: "Ch", Rect: rect, Page: page, Options: options}
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

// SetMultiline makes a text field multi-line (wraps text, shows scrollbar).
func (fd *FieldDef) SetMultiline(v bool) *FieldDef {
	if v {
		fd.Flags |= 1 << 12
	} else {
		fd.Flags &^= 1 << 12
	}
	return fd
}

// SetPassword makes a text field a password field (input is obscured with •).
func (fd *FieldDef) SetPassword(v bool) *FieldDef {
	if v {
		fd.Flags |= 1 << 13
	} else {
		fd.Flags &^= 1 << 13
	}
	return fd
}

// SetBorderStyle sets the PDF border style for a text field.
// Common values: "S" solid (default), "U" underline (bottom line only),
// "D" dashed, "B" beveled, "I" inset.
func (fd *FieldDef) SetBorderStyle(style string) *FieldDef {
	fd.BorderStyle = style
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
	bs := field.BorderStyle
	if bs == "" {
		bs = "S"
	}
	fmt.Fprintf(b, "/BS<</W 1/S/%s>>", bs)

	// Password fields need an explicit AP stream so non-Adobe viewers (Preview, Skim)
	// show masked bullets instead of plain text. Adobe derives the masked appearance
	// from the /Ff bit automatically, but other viewers require it to be explicit.
	if field.Flags&(1<<13) != 0 {
		apNum := fb.passwordFieldAppearance(field)
		fmt.Fprintf(b, "/AP<</N %d 0 R>>", apNum)
	}
}

// passwordFieldAppearance builds a Form XObject that renders a password field:
// white background, border, and masked bullets for any pre-filled value.
func (fb *FieldBuilder) passwordFieldAppearance(field *FieldDef) int {
	w := field.Rect[2] - field.Rect[0]
	h := field.Rect[3] - field.Rect[1]

	fontSize := field.FontSize
	if fontSize == 0 {
		fontSize = 12
	}

	var s strings.Builder

	// White background.
	s.WriteString("q\n1 g\n")
	fmt.Fprintf(&s, "0 0 %.4f %.4f re\nf\nQ\n", w, h)

	// Border — solid box or underline-only depending on BorderStyle.
	bs := field.BorderStyle
	s.WriteString("q\n0 G\n0.5 w\n")
	if bs == "U" {
		fmt.Fprintf(&s, "0 0 m\n%.4f 0 l\nS\n", w)
	} else {
		fmt.Fprintf(&s, "0.25 0.25 %.4f %.4f re\nS\n", w-0.5, h-0.5)
	}
	s.WriteString("Q\n")

	// If the field has a pre-filled value, render masked bullets.
	if field.Value != nil {
		valueStr := fmt.Sprint(field.Value)
		n := len([]rune(valueStr))
		if n > 0 {
			// Vertical centering: Helvetica cap-height ≈ 0.72 × fontSize.
			capHeight := 0.72 * fontSize
			baselineY := (h - capHeight) / 2

			// \225 is the PDF octal escape for WinAnsi byte 0x95 = bullet •.
			bullets := strings.Repeat(`\225`, n)

			s.WriteString("BT\n")
			fmt.Fprintf(&s, "/Helv %.4g Tf\n", fontSize)
			fmt.Fprintf(&s, "2 %.4f Td\n", baselineY)
			fmt.Fprintf(&s, "(%s) Tj\n", bullets)
			s.WriteString("ET\n")
		}
	}

	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
		"/Resources": write.Dictionary{
			"/Font": write.Dictionary{
				"/Helv": fmt.Sprintf("%d 0 R", fb.fontNum),
			},
		},
	}
	return fb.writer.AddStreamObject(dict, []byte(s.String()), false)
}

func (fb *FieldBuilder) writeBtnKeys(b *strings.Builder, field *FieldDef) {
	isPushButton := field.Flags&(1<<16) != 0
	if isPushButton {
		w := field.Rect[2] - field.Rect[0]
		h := field.Rect[3] - field.Rect[1]
		apNum := fb.pushButtonAppearance(w, h)
		fmt.Fprintf(b, "/AP<</N %d 0 R>>", apNum)
		// /MK /CA supplies the button label that viewers render on top.
		fmt.Fprintf(b, "/MK<</CA(%s)/TP 0>>", escapeFieldStr(field.Name))
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

	w := field.Rect[2] - field.Rect[0]
	h := field.Rect[3] - field.Rect[1]

	isRadio := field.Flags&(1<<15) != 0
	var yesNum, offNum int
	if isRadio {
		yesNum = fb.radioOnAppearance(w, h)
		offNum = fb.radioOffAppearance(w, h)
	} else {
		yesNum = fb.checkmarkAppearance(w, h)
		offNum = fb.boxAppearance(w, h)
	}
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

// pushButtonAppearance draws a raised-style push button background.
// The button caption is set via /MK /CA and rendered by the viewer.
func (fb *FieldBuilder) pushButtonAppearance(w, h float64) int {
	var s strings.Builder
	// Light gray fill
	s.WriteString("q\n0.7529 0.7529 0.7529 rg\n")
	fmt.Fprintf(&s, "0 0 %.4f %.4f re\nf\n", w, h)
	// Raised bevel: bright top/left edges, dark bottom/right edges
	bw := 1.5
	// Highlight (top-left)
	s.WriteString("1 1 1 RG\n")
	fmt.Fprintf(&s, "%.4f w\n", bw)
	fmt.Fprintf(&s, "%.4f %.4f m %.4f %.4f l %.4f %.4f l S\n",
		0.0, 0.0, 0.0, h, w, h)
	// Shadow (bottom-right)
	s.WriteString("0.5 0.5 0.5 RG\n")
	fmt.Fprintf(&s, "%.4f w\n", bw)
	fmt.Fprintf(&s, "%.4f %.4f m %.4f %.4f l %.4f %.4f l S\n",
		w, h, w, 0.0, 0.0, 0.0)
	// Dark outer border
	s.WriteString("0 0 0 RG\n0.5 w\n")
	fmt.Fprintf(&s, "0.25 0.25 %.4f %.4f re\nS\nQ\n", w-0.5, h-0.5)

	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
	}
	return fb.writer.AddStreamObject(dict, []byte(s.String()), false)
}

// boxAppearance draws a plain bordered box (unchecked checkbox state).
func (fb *FieldBuilder) boxAppearance(w, h float64) int {
	// Inset 0.25pt so the 0.5pt stroke is fully inside the BBox.
	stream := fmt.Sprintf("q\n0.5 w\n0 G\n0.25 0.25 %.4f %.4f re\nS\nQ\n", w-0.5, h-0.5)
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
	}
	return fb.writer.AddStreamObject(dict, []byte(stream), false)
}

// checkmarkAppearance draws a bordered box with a ✓ inside (checked checkbox state).
func (fb *FieldBuilder) checkmarkAppearance(w, h float64) int {
	stream := fmt.Sprintf("q\n"+
		"0.5 w\n0 G\n0.25 0.25 %.4f %.4f re\nS\n"+ // box border (inset so stroke stays in BBox)
		"0 g\n%.4f w\n"+ // checkmark stroke
		"%.4f %.4f m\n%.4f %.4f l\n%.4f %.4f l\nS\nQ\n",
		w-0.5, h-0.5,
		w*0.12,
		w*0.15, h*0.50,
		w*0.38, h*0.24,
		w*0.82, h*0.70,
	)
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
	}
	return fb.writer.AddStreamObject(dict, []byte(stream), false)
}

// radioOffAppearance draws an empty circle (unchecked radio button state).
func (fb *FieldBuilder) radioOffAppearance(w, h float64) int {
	stream := circleStream(w, h, false)
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
	}
	return fb.writer.AddStreamObject(dict, []byte(stream), false)
}

// radioOnAppearance draws a circle with a filled dot inside (checked radio button state).
func (fb *FieldBuilder) radioOnAppearance(w, h float64) int {
	stream := circleStream(w, h, true)
	dict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0.0, 0.0, w, h},
	}
	return fb.writer.AddStreamObject(dict, []byte(stream), false)
}

// circleStream returns a PDF content stream that draws a circle centered in [0 0 w h].
// If dot=true, a smaller filled circle is drawn inside to indicate the "on" state.
func circleStream(w, h float64, dot bool) string {
	cx, cy := w/2, h/2
	r := min64(w, h)/2 - 0.5 // 0.5pt inset keeps the 0.5pt stroke fully inside the BBox
	var b strings.Builder
	fmt.Fprintf(&b, "q\n0.5 w\n0 G\n")
	writeCirclePath(&b, cx, cy, r)
	fmt.Fprintf(&b, "S\n")
	if dot {
		dr := r * 0.4
		fmt.Fprintf(&b, "0 g\n")
		writeCirclePath(&b, cx, cy, dr)
		fmt.Fprintf(&b, "f\n")
	}
	fmt.Fprintf(&b, "Q\n")
	return b.String()
}

func writeCirclePath(b *strings.Builder, cx, cy, r float64) {
	const k = 0.5523
	fmt.Fprintf(b, "%.4f %.4f m\n", cx, cy+r)
	fmt.Fprintf(b, "%.4f %.4f %.4f %.4f %.4f %.4f c\n", cx+k*r, cy+r, cx+r, cy+k*r, cx+r, cy)
	fmt.Fprintf(b, "%.4f %.4f %.4f %.4f %.4f %.4f c\n", cx+r, cy-k*r, cx+k*r, cy-r, cx, cy-r)
	fmt.Fprintf(b, "%.4f %.4f %.4f %.4f %.4f %.4f c\n", cx-k*r, cy-r, cx-r, cy-k*r, cx-r, cy)
	fmt.Fprintf(b, "%.4f %.4f %.4f %.4f %.4f %.4f c\n", cx-r, cy+k*r, cx-k*r, cy+r, cx, cy+r)
	fmt.Fprintf(b, "h\n")
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
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
