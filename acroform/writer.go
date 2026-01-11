// Package acroform provides AcroForm creation and writing functionality
package acroform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/writer"
)

// FieldBuilder helps build AcroForm fields
type FieldBuilder struct {
	writer     *writer.PDFWriter
	fields     []*FieldDef
	nextObjNum int
}

// FieldDef represents a field definition for creation
type FieldDef struct {
	Name         string
	Type         string // Tx, Btn, Ch, Sig
	Value        interface{}
	DefaultValue interface{}
	Rect         []float64 // [llx lly urx ury]
	Page         int
	Flags        int
	Options      []string // For choice fields
	MaxLen       int      // For text fields
	Required     bool
	ReadOnly     bool
}

// NewFieldBuilder creates a new field builder
func NewFieldBuilder(w *writer.PDFWriter) *FieldBuilder {
	return &FieldBuilder{
		writer: w,
		fields: make([]*FieldDef, 0),
	}
}

// AddTextField adds a text field to the form
func (fb *FieldBuilder) AddTextField(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{
		Name: name,
		Type: "Tx",
		Rect: rect,
		Page: page,
	}
	fb.fields = append(fb.fields, field)
	return field
}

// AddCheckbox adds a checkbox field
func (fb *FieldBuilder) AddCheckbox(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{
		Name:  name,
		Type:  "Btn",
		Rect:  rect,
		Page:  page,
		Flags: 0x8000, // Checkbox flag
	}
	fb.fields = append(fb.fields, field)
	return field
}

// AddRadioButton adds a radio button field
func (fb *FieldBuilder) AddRadioButton(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{
		Name:  name,
		Type:  "Btn",
		Rect:  rect,
		Page:  page,
		Flags: 0x10000, // Radio button flag
	}
	fb.fields = append(fb.fields, field)
	return field
}

// AddChoiceField adds a choice (dropdown/list) field
func (fb *FieldBuilder) AddChoiceField(name string, rect []float64, page int, options []string) *FieldDef {
	field := &FieldDef{
		Name:    name,
		Type:    "Ch",
		Rect:    rect,
		Page:    page,
		Options: options,
	}
	fb.fields = append(fb.fields, field)
	return field
}

// AddButton adds a push button
func (fb *FieldBuilder) AddButton(name string, rect []float64, page int) *FieldDef {
	field := &FieldDef{
		Name: name,
		Type: "Btn",
		Rect: rect,
		Page: page,
	}
	fb.fields = append(fb.fields, field)
	return field
}

// SetValue sets the field value
func (fd *FieldDef) SetValue(value interface{}) *FieldDef {
	fd.Value = value
	return fd
}

// SetDefault sets the default value
func (fd *FieldDef) SetDefault(value interface{}) *FieldDef {
	fd.DefaultValue = value
	return fd
}

// SetRequired marks the field as required
func (fd *FieldDef) SetRequired(required bool) *FieldDef {
	fd.Required = required
	if required {
		fd.Flags |= 0x2
	} else {
		fd.Flags &^= 0x2
	}
	return fd
}

// SetReadOnly marks the field as read-only
func (fd *FieldDef) SetReadOnly(readonly bool) *FieldDef {
	fd.ReadOnly = readonly
	if readonly {
		fd.Flags |= 0x1
	} else {
		fd.Flags &^= 0x1
	}
	return fd
}

// SetMaxLength sets maximum length for text fields
func (fd *FieldDef) SetMaxLength(maxLen int) *FieldDef {
	fd.MaxLen = maxLen
	return fd
}

// Build creates the AcroForm dictionary and field objects
func (fb *FieldBuilder) Build() (int, error) {
	if len(fb.fields) == 0 {
		return 0, fmt.Errorf("no fields to build")
	}

	// Create field objects
	fieldRefs := make([]string, 0, len(fb.fields))
	for _, field := range fb.fields {
		fieldObjNum := fb.createFieldObject(field)
		fieldRefs = append(fieldRefs, fmt.Sprintf("%d 0 R", fieldObjNum))
	}

	// Create AcroForm dictionary
	fieldsArray := "[" + strings.Join(fieldRefs, " ") + "]"
	acroFormDict := fmt.Sprintf("<< /Fields %s /NeedAppearances true >>", fieldsArray)
	acroFormNum := fb.writer.AddObject([]byte(acroFormDict))

	return acroFormNum, nil
}

// createFieldObject creates a PDF field object
func (fb *FieldBuilder) createFieldObject(field *FieldDef) int {
	var dict strings.Builder
	dict.WriteString("<<")

	// Field type
	dict.WriteString(fmt.Sprintf(" /FT /%s", field.Type))

	// Field name
	if field.Name != "" {
		dict.WriteString(fmt.Sprintf(" /T (%s)", escapeFieldName(field.Name)))
	}

	// Rectangle
	if len(field.Rect) >= 4 {
		dict.WriteString(fmt.Sprintf(" /Rect [%.2f %.2f %.2f %.2f]",
			field.Rect[0], field.Rect[1], field.Rect[2], field.Rect[3]))
	}

	// Flags
	if field.Flags != 0 {
		dict.WriteString(fmt.Sprintf(" /Ff %d", field.Flags))
	}

	// Value
	if field.Value != nil {
		valueStr := formatFieldValueForWriter(field.Value, field.Type)
		dict.WriteString(fmt.Sprintf(" /V (%s)", escapeFieldValueForWriter(valueStr)))
	}

	// Default value
	if field.DefaultValue != nil {
		valueStr := formatFieldValueForWriter(field.DefaultValue, field.Type)
		dict.WriteString(fmt.Sprintf(" /DV (%s)", escapeFieldValueForWriter(valueStr)))
	}

	// Maximum length
	if field.MaxLen > 0 {
		dict.WriteString(fmt.Sprintf(" /MaxLen %d", field.MaxLen))
	}

	// Options (for choice fields)
	if len(field.Options) > 0 {
		dict.WriteString(" /Opt [")
		for i, opt := range field.Options {
			if i > 0 {
				dict.WriteString(" ")
			}
			dict.WriteString(fmt.Sprintf("(%s)", escapeFieldValue(opt)))
		}
		dict.WriteString("]")
	}

	// Page reference (would need to be set properly in real implementation)
	// For now, we'll add it as a property

	dict.WriteString(" >>")

	return fb.writer.AddObject([]byte(dict.String()))
}

// formatFieldValueForWriter formats a value for PDF writing
func formatFieldValueForWriter(value interface{}, fieldType string) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if fieldType == "Btn" {
			if v {
				return "Yes"
			}
			return "Off"
		}
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// escapeFieldName escapes special characters in field names
func escapeFieldName(s string) string {
	return escapeFieldValueForWriter(s)
}

// escapeFieldValueForWriter escapes special characters in field values
func escapeFieldValueForWriter(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '(':
			result.WriteString("\\(")
		case ')':
			result.WriteString("\\)")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			if r > 127 {
				result.WriteString(fmt.Sprintf("\\%03o", r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

// AddAcroFormToCatalog adds AcroForm reference to catalog
func AddAcroFormToCatalog(w *writer.PDFWriter, catalogNum, acroFormNum int) error {
	// Get existing catalog
	catalogData, err := w.GetObject(catalogNum)
	if err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Add AcroForm reference
	catalogStr := string(catalogData)
	if !strings.Contains(catalogStr, "/AcroForm") {
		// Find the closing >>
		dictEnd := strings.LastIndex(catalogStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("invalid catalog dictionary")
		}
		newEntry := fmt.Sprintf(" /AcroForm %d 0 R", acroFormNum)
		catalogStr = catalogStr[:dictEnd] + newEntry + catalogStr[dictEnd:]
		w.SetObject(catalogNum, []byte(catalogStr))
	}

	return nil
}
