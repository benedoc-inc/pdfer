// Package pdfer provides pure Go PDF processing with comprehensive XFA support.
//
// This is a zero-dependency PDF library that can:
//   - Decrypt PDFs (RC4 and AES encryption)
//   - Parse PDF structure (xref, objects, streams)
//   - Extract and modify XFA forms
//   - Create PDFs from scratch
//
// # Quick Start
//
// Extract XFA from an encrypted PDF:
//
//	import "github.com/benedoc-inc/pdfer"
//	import "github.com/benedoc-inc/pdfer/core/encrypt"
//	import "github.com/benedoc-inc/pdfer/forms/xfa"
//
//	// Decrypt
//	_, encInfo, _ := encryption.DecryptPDF(pdfBytes, password, false)
//
//	// Extract XFA
//	streams, _ := xfa.ExtractAllXFAStreams(pdfBytes, encInfo, false)
//
// # Packages
//
//   - encryption: PDF decryption (RC4, AES-128, AES-256)
//   - parser: Low-level PDF parsing
//   - types: Common data structures
//   - writer: PDF creation and modification
//   - xfa: XFA form processing
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
	return "0.9.5"
}
