// Package acroform provides object replacement for form filling
package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// ReplaceFieldObject replaces a field object in a PDF
func ReplaceFieldObject(pdfBytes []byte, objNum, genNum int, newContent []byte, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	pdfStr := string(pdfBytes)

	// Find the object
	objPattern := regexp.MustCompile(fmt.Sprintf(`%d\s+%d\s+obj`, objNum, genNum))
	objMatch := objPattern.FindStringIndex(pdfStr)
	if objMatch == nil {
		return nil, fmt.Errorf("object %d %d not found", objNum, genNum)
	}

	objStart := objMatch[0]
	objHeaderEnd := objStart + objMatch[1] - objMatch[0]

	// Find endobj - search from after the header
	searchStart := objHeaderEnd
	endObjPattern := regexp.MustCompile(`endobj`)
	endObjMatches := endObjPattern.FindAllStringIndex(pdfStr[searchStart:], -1)
	if len(endObjMatches) == 0 {
		return nil, fmt.Errorf("endobj not found for object %d", objNum)
	}

	// Find the first endobj after our object start
	endObjPos := searchStart + endObjMatches[0][1]

	// Reconstruct: before object + header + new content + endobj + after object
	before := pdfBytes[:objHeaderEnd]
	after := pdfBytes[endObjPos:]

	result := make([]byte, 0, len(before)+len(newContent)+len(after)+20)
	result = append(result, before...)
	result = append(result, []byte("\n")...)
	result = append(result, newContent...)
	result = append(result, []byte("\nendobj\n")...)
	result = append(result, after...)

	if verbose {
		oldSize := endObjPos - objHeaderEnd
		fmt.Printf("Replaced object %d %d: %d bytes -> %d bytes\n", objNum, genNum, oldSize, len(newContent))
	}

	return result, nil
}

// FillFieldValue fills a field with a value by replacing the object
func FillFieldValue(pdfBytes []byte, field *Field, value interface{}, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Get current field object
	fieldData, err := parser.GetObject(pdfBytes, field.ObjectNum, encryptInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)
	valueStr := formatFieldValue(value, field.FT)

	// Replace or add /V entry
	vPattern := regexp.MustCompile(`/V\s*(?:\([^)]*\)|/[^\s]+|\[[^\]]*\])`)
	newV := fmt.Sprintf("/V (%s)", escapeFieldValue(valueStr))

	var newFieldStr string
	if vPattern.MatchString(fieldStr) {
		// Replace existing value
		newFieldStr = vPattern.ReplaceAllString(fieldStr, newV)
	} else {
		// Add new /V entry before the closing >>
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return nil, fmt.Errorf("field dictionary not found")
		}
		newFieldStr = fieldStr[:dictEnd] + newV + " " + fieldStr[dictEnd:]
	}

	// Replace the object
	return ReplaceFieldObject(pdfBytes, field.ObjectNum, field.Generation, []byte(newFieldStr), encryptInfo, verbose)
}

// formatFieldValue formats a value for PDF based on field type
func formatFieldValue(value interface{}, fieldType string) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if fieldType == "Btn" {
			// For checkboxes, use appearance state name
			if v {
				return "Yes" // Or field-specific appearance name
			}
			return "Off"
		}
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// escapeFieldValue escapes special characters in field values
func escapeFieldValue(s string) string {
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

// FillFormFields fills multiple fields in a PDF
func FillFormFields(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("PDF bytes are empty")
	}

	// Parse PDF to get encryption info
	var encryptInfo *types.PDFEncryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		// Use encryption package to parse
		_, encryptInfo, _ = encryption.DecryptPDF(pdfBytes, password, verbose)
	}

	// Extract AcroForm
	acroForm, err := ParseAcroForm(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	if acroForm == nil {
		return nil, fmt.Errorf("AcroForm is nil")
	}

	// Fill each field
	result := make([]byte, len(pdfBytes))
	copy(result, pdfBytes)

	for fieldName, value := range formData {
		field := acroForm.FindFieldByName(fieldName)
		if field == nil {
			if verbose {
				fmt.Printf("Warning: Field '%s' not found, skipping\n", fieldName)
			}
			continue
		}

		var err error
		newResult, err := FillFieldValue(result, field, value, encryptInfo, verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to fill field '%s': %w", fieldName, err)
		}
		if len(newResult) == 0 {
			return nil, fmt.Errorf("FillFieldValue returned empty result for field '%s'", fieldName)
		}
		result = newResult

		if verbose {
			fmt.Printf("Filled field '%s' with value '%v'\n", fieldName, value)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("result PDF is empty after filling")
	}

	return result, nil
}
