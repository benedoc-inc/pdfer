package manipulate

import "fmt"

// Repair rebuilds a PDF into a clean single-revision form:
//   - flattens all incremental updates into one revision
//   - removes orphaned cross-reference entries
//   - rebuilds the xref table from scratch
//   - preserves all content, resources, and metadata
//
// It does not validate or fix content-stream operators.
// For encrypted PDFs supply the password; the output will be unencrypted.
func Repair(pdfBytes []byte, password []byte) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, false)
	if err != nil {
		return nil, fmt.Errorf("repair: parse failed: %w", err)
	}
	out, err := m.Rebuild()
	if err != nil {
		return nil, fmt.Errorf("repair: rebuild failed: %w", err)
	}
	return out, nil
}
