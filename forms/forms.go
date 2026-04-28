// Package forms provides a unified interface for working with PDF forms
// It supports both AcroForm and XFA form types with automatic detection
package forms

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
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
	return []error{fmt.Errorf("XFA form validation is not implemented")}
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

	return FormTypeUnknown, types.NewPDFError(types.ErrCodeNoForms, "no forms detected in PDF")
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

	// Try XFA — decrypt first if the PDF is encrypted, since XFA stream
	// extraction requires plaintext bytes (encryptInfo=nil path).
	xfaBytes := pdfBytes
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		pwd := password
		if len(pwd) == 0 {
			pwd = []byte("")
		}
		if decrypted, decErr := decryptForXFA(pdfBytes, pwd, verbose); decErr == nil {
			xfaBytes = decrypted
		}
	}
	streams, err := xfa.ExtractAllXFAStreams(xfaBytes, nil, verbose)
	if err == nil && streams.Template != nil && len(streams.Template.Data) > 0 {
		// Parse XFA form
		formSchema, err := xfa.ParseXFAForm(string(streams.Template.Data), verbose)
		if err != nil {
			return nil, types.WrapError(types.ErrCodeInvalidForm, "failed to parse XFA form", err)
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

	return nil, types.NewPDFError(types.ErrCodeNoForms, "no forms found in PDF")
}

// ExtractAcroForm extracts an AcroForm (type-specific)
func ExtractAcroForm(pdfBytes []byte, password []byte, verbose bool) (*acroform.AcroForm, error) {
	return acroform.ExtractAcroForm(pdfBytes, password, verbose)
}

// ExtractXFA extracts XFA form data (type-specific)
// Returns the FormSchema and Datasets separately
func ExtractXFA(pdfBytes []byte, password []byte, verbose bool) (*types.FormSchema, *types.XFADatasets, error) {
	xfaBytes := pdfBytes
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		pwd := password
		if len(pwd) == 0 {
			pwd = []byte("")
		}
		if decrypted, decErr := decryptForXFA(pdfBytes, pwd, verbose); decErr == nil {
			xfaBytes = decrypted
		}
	}
	streams, err := xfa.ExtractAllXFAStreams(xfaBytes, nil, verbose)
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

// decryptForXFA decrypts an encrypted PDF so that XFA stream extraction can
// operate on plaintext bytes. Uses parse + write directly to avoid an import
// cycle with core/manipulate (which imports forms via content/extract).
func decryptForXFA(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	if !pdf.IsEncrypted() {
		return pdfBytes, nil
	}
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("decrypt: no document catalog in trailer")
	}
	var rootNum, encryptObjNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)
	if trailer.EncryptRef != "" {
		fmt.Sscanf(trailer.EncryptRef, "%d", &encryptObjNum)
	}
	w := write.NewPDFWriter()
	for _, n := range pdf.Objects() {
		if n == encryptObjNum {
			continue
		}
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		if n == rootNum {
			body = xfaRemoveEncryptRef(body)
		}
		dictBytes, streamBytes := xfaSplitContent(body)
		if streamBytes != nil {
			w.SetRawStreamObject(n, dictBytes, streamBytes)
		} else {
			w.SetObject(n, dictBytes)
		}
	}
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}
	return w.Bytes()
}

var xfaEncryptRefRE = regexp.MustCompile(`/Encrypt\s+\d+\s+\d+\s+R`)

func xfaRemoveEncryptRef(b []byte) []byte {
	return xfaEncryptRefRE.ReplaceAll(b, nil)
}

func xfaSplitContent(content []byte) (dictBytes, streamBytes []byte) {
	for _, sep := range [][]byte{[]byte("\nstream\r\n"), []byte("\nstream\n")} {
		idx := bytes.Index(content, sep)
		if idx < 0 {
			continue
		}
		dict := content[:idx]
		rest := content[idx+len(sep):]
		for _, end := range [][]byte{[]byte("\nendstream"), []byte("endstream")} {
			if j := bytes.Index(rest, end); j >= 0 {
				return dict, rest[:j]
			}
		}
	}
	return content, nil
}
