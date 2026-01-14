// Package acroform provides AcroForm (standard PDF form) parsing and manipulation
package acroform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// AcroForm represents an AcroForm dictionary structure
type AcroForm struct {
	Fields          []*Field
	NeedAppearances bool
	SignatureFields []int // Object numbers of signature fields
	XFA             bool  // True if XFA is present (hybrid form)
}

// Field represents a single AcroForm field
type Field struct {
	ObjectNum  int
	Generation int
	FT         string                 // Field type: Tx (text), Btn (button), Ch (choice), Sig (signature)
	Parent     *Field                 // Parent field (for fields in a hierarchy)
	Kids       []*Field               // Child fields
	T          string                 // Field name (partial name)
	TU         string                 // Alternate field name
	TM         string                 // Mapping name
	Ff         int                    // Field flags
	V          interface{}            // Field value
	DV         interface{}            // Default value
	AA         map[string]interface{} // Additional actions
	DA         string                 // Default appearance string
	Q          int                    // Quadding (justification)
	MaxLen     int                    // Maximum length (for text fields)
	Opt        []interface{}          // Options (for choice fields)
	TI         int                    // Top index (for choice fields)
	I          []int                  // Selected indices (for choice fields)
	Rect       []float64              // Field rectangle [llx lly urx ury]
	Page       int                    // Page number (0-indexed)
}

// ParseAcroForm extracts AcroForm structure from a PDF
func ParseAcroForm(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) (*AcroForm, error) {
	// Find AcroForm reference in catalog
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: []byte(""),
		Verbose:  verbose,
	})
	if err != nil {
		return nil, types.WrapError(types.ErrCodeMalformedPDF, "failed to parse PDF", err)
	}

	// Search for /AcroForm in the PDF
	// We'll parse directly from bytes for now
	_ = pdf // May use later for better catalog access
	return parseAcroFormFromBytes(pdfBytes, encryptInfo, verbose)
}

// parseAcroFormFromBytes finds and parses AcroForm by searching PDF bytes
func parseAcroFormFromBytes(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) (*AcroForm, error) {
	pdfStr := string(pdfBytes)

	// Find AcroForm reference
	acroFormPattern := regexp.MustCompile(`/AcroForm\s+(\d+)\s+(\d+)\s+R`)
	acroFormMatch := acroFormPattern.FindStringSubmatch(pdfStr)

	if acroFormMatch == nil {
		// Try inline AcroForm
		acroFormInlinePattern := regexp.MustCompile(`/AcroForm\s*<<`)
		if acroFormInlinePattern.FindStringIndex(pdfStr) == nil {
			return nil, types.NewPDFError(types.ErrCodeNoForms, "AcroForm not found in PDF")
		}
		// Inline AcroForm - would need more complex parsing
		return nil, types.NewPDFError(types.ErrCodeUnsupportedPDF, "inline AcroForm not yet supported")
	}

	acroFormObjNum, err := strconv.Atoi(acroFormMatch[1])
	if err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidObject, "invalid AcroForm object number", err)
	}

	// Get AcroForm object
	acroFormData, err := parse.GetObject(pdfBytes, acroFormObjNum, encryptInfo, verbose)
	if err != nil {
		return nil, types.WrapErrorf(types.ErrCodeObjectNotFound, err, "failed to get AcroForm object %d", acroFormObjNum)
	}

	acroForm := &AcroForm{
		Fields: make([]*Field, 0),
	}

	// Parse AcroForm dictionary
	if err := parseAcroFormDict(acroFormData, acroForm, pdfBytes, encryptInfo, verbose); err != nil {
		return nil, types.WrapError(types.ErrCodeInvalidForm, "failed to parse AcroForm dictionary", err)
	}

	return acroForm, nil
}

// parseAcroFormDict parses the AcroForm dictionary
func parseAcroFormDict(data []byte, acroForm *AcroForm, pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) error {
	dataStr := string(data)

	// Check for XFA (hybrid form)
	if strings.Contains(dataStr, "/XFA") {
		acroForm.XFA = true
		if verbose {
			fmt.Println("Found XFA in AcroForm (hybrid form)")
		}
	}

	// Find Fields array
	fieldsPattern := regexp.MustCompile(`/Fields\s*\[([^\]]*)\]`)
	fieldsMatch := fieldsPattern.FindStringSubmatch(dataStr)
	if fieldsMatch == nil {
		return types.NewPDFError(types.ErrCodeInvalidForm, "Fields array not found in AcroForm")
	}

	fieldsStr := fieldsMatch[1]
	if verbose {
		fmt.Printf("Found Fields array: %s\n", fieldsStr)
	}

	// Extract field references
	fieldRefPattern := regexp.MustCompile(`(\d+)\s+(\d+)\s+R`)
	fieldRefs := fieldRefPattern.FindAllStringSubmatch(fieldsStr, -1)

	for _, ref := range fieldRefs {
		objNum, _ := strconv.Atoi(ref[1])
		genNum, _ := strconv.Atoi(ref[2])

		field, err := parseField(pdfBytes, objNum, genNum, encryptInfo, verbose)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: Failed to parse field %d: %v\n", objNum, err)
			}
			continue
		}

		acroForm.Fields = append(acroForm.Fields, field)
	}

	return nil
}

// parseField parses a single field dictionary
func parseField(pdfBytes []byte, objNum, genNum int, encryptInfo *types.PDFEncryption, verbose bool) (*Field, error) {
	fieldData, err := parse.GetObject(pdfBytes, objNum, encryptInfo, verbose)
	if err != nil {
		return nil, types.WrapError(types.ErrCodeFieldNotFound, "failed to get field object", err)
	}

	field := &Field{
		ObjectNum:  objNum,
		Generation: genNum,
		Kids:       make([]*Field, 0),
	}

	dataStr := string(fieldData)

	// Extract field type (FT)
	if ftMatch := regexp.MustCompile(`/FT\s*/(\w+)`).FindStringSubmatch(dataStr); ftMatch != nil {
		field.FT = ftMatch[1]
	}

	// Extract field name (T)
	if tMatch := regexp.MustCompile(`/T\s*\(([^)]*)\)`).FindStringSubmatch(dataStr); tMatch != nil {
		field.T = tMatch[1]
	} else if tMatch := regexp.MustCompile(`/T\s*/(\w+)`).FindStringSubmatch(dataStr); tMatch != nil {
		field.T = tMatch[1]
	}

	// Extract alternate name (TU)
	if tuMatch := regexp.MustCompile(`/TU\s*\(([^)]*)\)`).FindStringSubmatch(dataStr); tuMatch != nil {
		field.TU = tuMatch[1]
	}

	// Extract field flags (Ff)
	if ffMatch := regexp.MustCompile(`/Ff\s+(\d+)`).FindStringSubmatch(dataStr); ffMatch != nil {
		field.Ff, _ = strconv.Atoi(ffMatch[1])
	}

	// Extract value (V)
	if vMatch := regexp.MustCompile(`/V\s*\(([^)]*)\)`).FindStringSubmatch(dataStr); vMatch != nil {
		field.V = vMatch[1]
	} else if vMatch := regexp.MustCompile(`/V\s*/(\w+)`).FindStringSubmatch(dataStr); vMatch != nil {
		field.V = vMatch[1]
	} else if vMatch := regexp.MustCompile(`/V\s*\[([^\]]*)\]`).FindStringSubmatch(dataStr); vMatch != nil {
		// Array value (for choice fields)
		field.V = parseArray(vMatch[1])
	}

	// Extract default value (DV)
	if dvMatch := regexp.MustCompile(`/DV\s*\(([^)]*)\)`).FindStringSubmatch(dataStr); dvMatch != nil {
		field.DV = dvMatch[1]
	}

	// Extract maximum length (MaxLen)
	if maxLenMatch := regexp.MustCompile(`/MaxLen\s+(\d+)`).FindStringSubmatch(dataStr); maxLenMatch != nil {
		field.MaxLen, _ = strconv.Atoi(maxLenMatch[1])
	}

	// Extract options (Opt) - for choice fields
	if optMatch := regexp.MustCompile(`/Opt\s*\[([^\]]*)\]`).FindStringSubmatch(dataStr); optMatch != nil {
		field.Opt = parseOptions(optMatch[1])
	}

	// Extract rectangle (Rect) - field position
	if rectMatch := regexp.MustCompile(`/Rect\s*\[([^\]]*)\]`).FindStringSubmatch(dataStr); rectMatch != nil {
		field.Rect = parseRect(rectMatch[1])
	}

	// Extract kids (child fields)
	if kidsMatch := regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`).FindStringSubmatch(dataStr); kidsMatch != nil {
		kidsStr := kidsMatch[1]
		kidRefPattern := regexp.MustCompile(`(\d+)\s+(\d+)\s+R`)
		kidRefs := kidRefPattern.FindAllStringSubmatch(kidsStr, -1)

		for _, ref := range kidRefs {
			kidObjNum, _ := strconv.Atoi(ref[1])
			kidGenNum, _ := strconv.Atoi(ref[2])

			kidField, err := parseField(pdfBytes, kidObjNum, kidGenNum, encryptInfo, verbose)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: Failed to parse kid field %d: %v\n", kidObjNum, err)
				}
				continue
			}
			kidField.Parent = field
			field.Kids = append(field.Kids, kidField)
		}
	}

	return field, nil
}

// parseArray parses a PDF array string
func parseArray(arrStr string) []interface{} {
	items := make([]interface{}, 0)
	// Simple parsing - split by spaces and handle strings
	parts := strings.Fields(arrStr)
	for _, part := range parts {
		if strings.HasPrefix(part, "(") && strings.HasSuffix(part, ")") {
			items = append(items, part[1:len(part)-1])
		} else {
			items = append(items, part)
		}
	}
	return items
}

// parseOptions parses choice field options
func parseOptions(optStr string) []interface{} {
	// Options can be array of strings or array of [value, display] pairs
	options := make([]interface{}, 0)

	// Try to parse as array of strings first
	if strings.Contains(optStr, "(") {
		// Extract string values
		strPattern := regexp.MustCompile(`\(([^)]*)\)`)
		matches := strPattern.FindAllStringSubmatch(optStr, -1)
		for _, match := range matches {
			options = append(options, match[1])
		}
	}

	return options
}

// parseRect parses a rectangle array [llx lly urx ury]
func parseRect(rectStr string) []float64 {
	parts := strings.Fields(rectStr)
	rect := make([]float64, 0, 4)
	for _, part := range parts {
		if val, err := strconv.ParseFloat(part, 64); err == nil {
			rect = append(rect, val)
		}
	}
	return rect
}

// ToFormSchema converts AcroForm to FormSchema
func (af *AcroForm) ToFormSchema() *types.FormSchema {
	schema := &types.FormSchema{
		Metadata: types.FormMetadata{
			FormType: "AcroForm",
		},
		Questions: make([]types.Question, 0),
		Rules:     make([]types.Rule, 0),
	}

	for _, field := range af.Fields {
		question := field.ToQuestion()
		if question != nil {
			schema.Questions = append(schema.Questions, *question)
		}
	}

	return schema
}

// ToQuestion converts a Field to a Question
func (f *Field) ToQuestion() *types.Question {
	question := &types.Question{
		ID:         fmt.Sprintf("field_%d", f.ObjectNum),
		Name:       f.T,
		Label:      f.TU,
		Type:       mapFieldTypeWithFlags(f.FT, f.Ff),
		Required:   (f.Ff & 0x2) != 0, // Required flag
		ReadOnly:   (f.Ff & 0x1) != 0, // ReadOnly flag
		Properties: make(map[string]interface{}),
	}

	// Set default value
	if f.DV != nil {
		question.Default = f.DV
	}

	// Set current value
	if f.V != nil {
		// Value will be set when filling
	}

	// Add options for choice fields
	if len(f.Opt) > 0 {
		question.Options = make([]types.Option, 0, len(f.Opt))
		for i, opt := range f.Opt {
			optStr := fmt.Sprintf("%v", opt)
			question.Options = append(question.Options, types.Option{
				Value: optStr,
				Label: optStr,
			})
			// Check if this option is selected
			if f.I != nil {
				for _, idx := range f.I {
					if idx == i {
						question.Options[i].Selected = true
					}
				}
			}
		}
	}

	// Add validation
	if f.MaxLen > 0 {
		question.Validation = &types.ValidationRules{
			MaxLength: &f.MaxLen,
		}
	}

	// Add position properties
	if len(f.Rect) >= 4 {
		question.Properties["rect"] = f.Rect
		question.Properties["x"] = f.Rect[0]
		question.Properties["y"] = f.Rect[1]
		question.Properties["width"] = f.Rect[2] - f.Rect[0]
		question.Properties["height"] = f.Rect[3] - f.Rect[1]
	}

	question.Properties["page"] = f.Page
	question.Properties["object_num"] = f.ObjectNum

	return question
}

// mapFieldType maps AcroForm field type to ResponseType
func mapFieldType(ft string) types.ResponseType {
	switch ft {
	case "Tx":
		return types.ResponseTypeText
	case "Btn":
		// Check flags to determine if checkbox or button
		return types.ResponseTypeButton // Will be refined based on flags
	case "Ch":
		return types.ResponseTypeSelect
	case "Sig":
		return types.ResponseTypeSignature
	default:
		return types.ResponseTypeUnknown
	}
}

// mapFieldTypeWithFlags maps field type using flags to distinguish checkboxes from buttons
func mapFieldTypeWithFlags(ft string, flags int) types.ResponseType {
	switch ft {
	case "Tx":
		return types.ResponseTypeText
	case "Btn":
		// Checkbox flag is 0x8000 (bit 15)
		if (flags & 0x8000) != 0 {
			return types.ResponseTypeCheckbox
		}
		// Radio button flag is 0x10000 (bit 16)
		if (flags & 0x10000) != 0 {
			return types.ResponseTypeRadio
		}
		return types.ResponseTypeButton
	case "Ch":
		// Combo box flag is 0x20000 (bit 17)
		if (flags & 0x20000) != 0 {
			return types.ResponseTypeSelect // Dropdown
		}
		return types.ResponseTypeSelect // List box
	case "Sig":
		return types.ResponseTypeSignature
	default:
		return types.ResponseTypeUnknown
	}
}
