// Package acroform provides AcroForm extraction functions
package acroform

import (
	"bytes"
	"fmt"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/types"
)

// ExtractAcroForm extracts AcroForm structure from a PDF
// This is the main entry point for AcroForm extraction
func ExtractAcroForm(pdfBytes []byte, password []byte, verbose bool) (*AcroForm, error) {
	// Handle encryption
	var encryptInfo *types.PDFEncryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		var err error
		_, encryptInfo, err = encrypt.DecryptPDF(pdfBytes, password, verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt PDF: %w", err)
		}
	}

	// Parse AcroForm
	acroForm, err := ParseAcroForm(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	return acroForm, nil
}

// ExtractFormSchema extracts AcroForm and converts it to FormSchema
func ExtractFormSchema(pdfBytes []byte, password []byte, verbose bool) (*types.FormSchema, error) {
	acroForm, err := ExtractAcroForm(pdfBytes, password, verbose)
	if err != nil {
		return nil, err
	}

	return acroForm.ToFormSchema(), nil
}

// GetFieldValues extracts current values from all AcroForm fields
func (af *AcroForm) GetFieldValues() map[string]interface{} {
	values := make(map[string]interface{})

	for _, field := range af.Fields {
		fieldName := field.GetFullName()
		if fieldName != "" && field.V != nil {
			values[fieldName] = field.V
		}

		// Also get values from child fields
		for _, kid := range field.Kids {
			kidName := kid.GetFullName()
			if kidName != "" && kid.V != nil {
				values[kidName] = kid.V
			}
		}
	}

	return values
}

// GetFullName returns the full field name (handles hierarchical names)
func (f *Field) GetFullName() string {
	if f.Parent != nil {
		parentName := f.Parent.GetFullName()
		if parentName != "" {
			return parentName + "." + f.T
		}
	}
	return f.T
}

// FindFieldByName finds a field by its name
func (af *AcroForm) FindFieldByName(name string) *Field {
	for _, field := range af.Fields {
		if field.GetFullName() == name || field.T == name {
			return field
		}

		// Search in kids
		for _, kid := range field.Kids {
			if kid.GetFullName() == name || kid.T == name {
				return kid
			}
		}
	}
	return nil
}
