// Package forms provides a unified interface for working with PDF forms
// It supports both AcroForm and XFA form types with automatic detection
package forms

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/forms/acroform"
	"github.com/benedoc-inc/pdfer/forms/xfa"
	"github.com/benedoc-inc/pdfer/types"
)

// FormType represents the type of form
type FormType string

const (
	FormTypeAcroForm FormType = "acroform"
	FormTypeXFA      FormType = "xfa"
	FormTypeUnknown  FormType = "unknown"
)

// Form represents a unified form interface
type Form interface {
	// Type returns the form type (AcroForm or XFA)
	Type() FormType

	// Schema returns the form schema (structure and fields)
	Schema() *types.FormSchema

	// Fill fills the form with the provided data and returns modified PDF bytes
	Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error)

	// Validate validates form data against the form's validation rules
	Validate(data types.FormData) []error

	// GetValues returns the current values of all form fields
	GetValues() map[string]interface{}
}

// AcroFormWrapper wraps an AcroForm to implement the Form interface
type AcroFormWrapper struct {
	acroForm *acroform.AcroForm
	pdfBytes []byte
	password []byte
}

func (w *AcroFormWrapper) Type() FormType {
	return FormTypeAcroForm
}

func (w *AcroFormWrapper) Schema() *types.FormSchema {
	return w.acroForm.ToFormSchema()
}

func (w *AcroFormWrapper) Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error) {
	return acroform.FillFormFieldsWithStreams(pdfBytes, data, password, verbose)
}

func (w *AcroFormWrapper) Validate(data types.FormData) []error {
	return acroform.ValidateFormData(w.acroForm, data)
}

func (w *AcroFormWrapper) GetValues() map[string]interface{} {
	return w.acroForm.GetFieldValues()
}

// XFAFormWrapper wraps XFA form data to implement the Form interface
type XFAFormWrapper struct {
	formSchema *types.FormSchema
	datasets   *types.XFADatasets
	pdfBytes   []byte
	password   []byte
}

func (w *XFAFormWrapper) Type() FormType {
	return FormTypeXFA
}

func (w *XFAFormWrapper) Schema() *types.FormSchema {
	return w.formSchema
}

func (w *XFAFormWrapper) Fill(pdfBytes []byte, data types.FormData, password []byte, verbose bool) ([]byte, error) {
	// Get encryption info if needed
	var encryptInfo *types.PDFEncryption
	if len(password) > 0 || len(w.password) > 0 {
		pwd := password
		if len(pwd) == 0 {
			pwd = w.password
		}
		// Parse encryption if PDF is encrypted
		// For now, use the XFA update function
		return xfa.UpdateXFAInPDF(pdfBytes, data, encryptInfo, verbose)
	}
	return xfa.UpdateXFAInPDF(pdfBytes, data, nil, verbose)
}

func (w *XFAFormWrapper) Validate(data types.FormData) []error {
	// XFA validation would go here
	// For now, return empty (XFA validation is more complex)
	return []error{}
}

func (w *XFAFormWrapper) GetValues() map[string]interface{} {
	if w.datasets != nil {
		return w.datasets.Fields
	}
	return make(map[string]interface{})
}

// Detect detects the form type in a PDF
func Detect(pdfBytes []byte, password []byte, verbose bool) (FormType, error) {
	// Try AcroForm first
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, verbose)
	if err == nil && acroForm != nil && len(acroForm.Fields) > 0 {
		return FormTypeAcroForm, nil
	}

	// Try XFA
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, nil, verbose)
	if err == nil && streams.Template != nil && len(streams.Template.Data) > 0 {
		return FormTypeXFA, nil
	}

	return FormTypeUnknown, fmt.Errorf("no forms detected in PDF")
}

// Extract extracts and returns a unified Form interface
// It automatically detects whether the PDF contains AcroForm or XFA forms
func Extract(pdfBytes []byte, password []byte, verbose bool) (Form, error) {
	// Try AcroForm first
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, verbose)
	if err == nil && acroForm != nil && len(acroForm.Fields) > 0 {
		return &AcroFormWrapper{
			acroForm: acroForm,
			pdfBytes: pdfBytes,
			password: password,
		}, nil
	}

	// Try XFA
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, nil, verbose)
	if err == nil && streams.Template != nil && len(streams.Template.Data) > 0 {
		// Parse XFA form
		formSchema, err := xfa.ParseXFAForm(string(streams.Template.Data), verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to parse XFA form: %w", err)
		}

		var datasets *types.XFADatasets
		if streams.Datasets != nil {
			datasets, _ = xfa.ParseXFADatasets(string(streams.Datasets.Data), verbose)
		}

		return &XFAFormWrapper{
			formSchema: formSchema,
			datasets:   datasets,
			pdfBytes:   pdfBytes,
			password:   password,
		}, nil
	}

	return nil, fmt.Errorf("no forms found in PDF")
}

// ExtractAcroForm extracts an AcroForm (type-specific)
func ExtractAcroForm(pdfBytes []byte, password []byte, verbose bool) (*acroform.AcroForm, error) {
	return acroform.ExtractAcroForm(pdfBytes, password, verbose)
}

// ExtractXFA extracts XFA form data (type-specific)
// Returns the FormSchema and Datasets separately
func ExtractXFA(pdfBytes []byte, password []byte, verbose bool) (*types.FormSchema, *types.XFADatasets, error) {
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, nil, verbose)
	if err != nil {
		return nil, nil, err
	}

	var formSchema *types.FormSchema
	var datasets *types.XFADatasets

	if streams.Template != nil {
		formSchema, _ = xfa.ParseXFAForm(string(streams.Template.Data), verbose)
	}
	if streams.Datasets != nil {
		datasets, _ = xfa.ParseXFADatasets(string(streams.Datasets.Data), verbose)
	}

	return formSchema, datasets, nil
}
