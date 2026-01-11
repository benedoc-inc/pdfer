// Package acroform provides AcroForm field filling functionality
package acroform

import (
	"github.com/benedoc-inc/pdfer/types"
)

// FillForm fills AcroForm fields with values from FormData
// Returns modified PDF bytes
// This is a convenience wrapper around FillFormFields
func FillForm(pdfBytes []byte, formData types.FormData, password []byte, verbose bool) ([]byte, error) {
	return FillFormFields(pdfBytes, formData, password, verbose)
}
