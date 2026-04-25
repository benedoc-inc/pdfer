// Package pdfer provides pure Go PDF processing with zero external dependencies.
//
// # Common operations
//
// Encrypt and decrypt:
//
//	out, err := pdfer.EncryptPDF(pdfBytes, []byte("password"), nil)
//	out, _, err := pdfer.DecryptPDF(pdfBytes, []byte("password"), false)
//
// Merge, split, redact:
//
//	out, err := pdfer.MergePDFs([][]byte{a, b}, nil, false)
//	parts, err := pdfer.SplitPDF(pdfBytes, []pdfer.PageRange{{1, 3}, {4, 6}}, nil, false)
//	out, err := pdfer.Redact(pdfBytes, []pdfer.RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}, nil)
//
// Fill and flatten forms (AcroForm and XFA auto-detected):
//
//	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
//	out, err := form.Fill(pdfBytes, pdfer.FormData{"name": "Alice"}, nil, false)
//	out, err = pdfer.FlattenForm(out, nil, false)
//
// # Sub-packages
//
// Import sub-packages directly for lower-level control:
//
//   - core/encrypt  — RC4/AES decryption and AES-128/256 encryption primitives
//   - core/parse    — PDF structure parsing (xref, objects, streams)
//   - core/write    — PDF generation (SimplePDFBuilder, PDFWriter)
//   - core/manipulate — document-level operations (merge, split, redact, encrypt)
//   - forms/acroform — AcroForm parsing, filling, appearance streams
//   - forms/xfa     — XFA stream extraction and dataset updating
//   - content/extract — text, image, annotation and metadata extraction
package pdfer

import (
	"github.com/benedoc-inc/pdfer/types"
)

// Re-export common types for convenience.
// Users can import just "github.com/benedoc-inc/pdfer" for basic usage.

// Encryption holds PDF encryption parameters and derived keys.
type Encryption = types.PDFEncryption

// FormSchema represents a parsed XFA form structure.
type FormSchema = types.FormSchema

// Question represents a single form field.
type Question = types.Question

// Rule represents a validation or calculation rule.
type Rule = types.Rule

// FormData is a map of field names to values for form filling.
type FormData = types.FormData

// XFADatasets represents parsed XFA datasets.
type XFADatasets = types.XFADatasets

// XFAConfig represents parsed XFA configuration.
type XFAConfig = types.XFAConfig

// XFALocaleSet represents parsed XFA localization data.
type XFALocaleSet = types.XFALocaleSet

// Version returns the library version.
func Version() string {
	return "0.9.25"
}
