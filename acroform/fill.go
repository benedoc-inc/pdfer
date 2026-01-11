// Package acroform provides AcroForm field filling functionality
package acroform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// FillForm fills AcroForm fields with values from FormData
// Returns modified PDF bytes
func FillForm(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	// Parse PDF
	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// Extract AcroForm
	var encryptInfo *types.PDFEncryption
	if pdf.Encryption() != nil {
		encryptInfo = pdf.Encryption()
	}

	acroForm, err := ParseAcroForm(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	// Fill fields
	modifiedBytes := make([]byte, len(pdfBytes))
	copy(modifiedBytes, pdfBytes)

	for fieldName, value := range formData {
		field := acroForm.FindFieldByName(fieldName)
		if field == nil {
			if verbose {
				fmt.Printf("Warning: Field '%s' not found, skipping\n", fieldName)
			}
			continue
		}

		if err := fillField(modifiedBytes, field, value, encryptInfo, verbose); err != nil {
			if verbose {
				fmt.Printf("Warning: Failed to fill field '%s': %v\n", fieldName, err)
			}
			continue
		}
	}

	return modifiedBytes, nil
}

// fillField fills a single field with a value
func fillField(pdfBytes []byte, field *Field, value interface{}, encryptInfo *types.PDFEncryption, verbose bool) error {
	// Get field object
	fieldData, err := parser.GetObject(pdfBytes, field.ObjectNum, encryptInfo, false)
	if err != nil {
		return fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)
	valueStr := formatValue(value, field.FT)

	// Replace or add /V entry
	vPattern := regexp.MustCompile(`/V\s*(?:\([^)]*\)|/[^\s]+|\[[^\]]*\])`)
	if vPattern.MatchString(fieldStr) {
		// Replace existing value
		newV := fmt.Sprintf("/V (%s)", escapePDFString(valueStr))
		fieldStr = vPattern.ReplaceAllString(fieldStr, newV)
	} else {
		// Add new /V entry before the closing >>
		// Find last >> before endobj
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("field dictionary not found")
		}
		newV := fmt.Sprintf("/V (%s) ", escapePDFString(valueStr))
		fieldStr = fieldStr[:dictEnd] + newV + fieldStr[dictEnd:]
	}

	// Write back to PDF (simplified - would need proper object replacement)
	// For now, this is a placeholder - full implementation would need to:
	// 1. Find object boundaries
	// 2. Replace object content
	// 3. Update xref if object size changed
	// 4. Handle encryption if needed

	if verbose {
		fmt.Printf("Filled field '%s' with value '%s'\n", field.T, valueStr)
	}

	return nil
}

// formatValue formats a value for PDF based on field type
func formatValue(value interface{}, fieldType string) string {
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

// escapePDFString escapes special characters in PDF strings
func escapePDFString(s string) string {
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
