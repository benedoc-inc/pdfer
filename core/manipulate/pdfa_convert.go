package manipulate

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/core/write"
)

// ConvertToPDFA converts a PDF to PDF/A format.
//
// conformance selects the conformance level:
//   - "1b" → PDF/A-1b (default when empty)
//   - "2b" → PDF/A-2b
//   - "3b" → PDF/A-3b
//
// PDF/A cannot be encrypted, so if the input PDF is encrypted it is first
// decrypted using password. The output is always unencrypted.
func ConvertToPDFA(pdfBytes []byte, password []byte, conformance string) ([]byte, error) {
	// Decrypt if necessary so that FixPDFA receives unencrypted bytes.
	decrypted, err := DecryptPDF(pdfBytes, password, false)
	if err != nil {
		return nil, fmt.Errorf("convert to pdf/a: decrypt failed: %w", err)
	}

	// Map the conformance string to write.PDFAConformance.
	var conf write.PDFAConformance
	switch conformance {
	case "", "1b":
		conf = write.PDFA1b
	case "2b":
		conf = write.PDFA2b
	case "3b":
		conf = write.PDFA3b
	default:
		return nil, fmt.Errorf("convert to pdf/a: unsupported conformance %q (use \"1b\", \"2b\", or \"3b\")", conformance)
	}

	out, err := write.ConvertToPDFA(decrypted, conf)
	if err != nil {
		return nil, fmt.Errorf("convert to pdf/a: %w", err)
	}
	return out, nil
}
