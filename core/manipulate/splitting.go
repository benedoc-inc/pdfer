package manipulate

import (
	"fmt"
)

// SplitPDF splits a PDF into multiple PDFs based on page ranges
// pageRanges is a slice of page number ranges, e.g., []PageRange{{1, 3}, {4, 5}}
// Returns a slice of PDF bytes, one for each range
func SplitPDF(pdfBytes []byte, pageRanges []PageRange, password []byte, verbose bool) ([][]byte, error) {
	if len(pageRanges) == 0 {
		return nil, fmt.Errorf("no page ranges specified")
	}

	// Create manipulator
	manipulator, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to create manipulator: %w", err)
	}

	// Get all page object numbers
	allPageObjNums, err := manipulator.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("failed to get page objects: %w", err)
	}

	totalPages := len(allPageObjNums)
	if totalPages == 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}

	// Validate ranges
	for i, r := range pageRanges {
		if r.Start < 1 || r.Start > totalPages {
			return nil, fmt.Errorf("range %d: start page %d out of range (1-%d)", i+1, r.Start, totalPages)
		}
		if r.End < r.Start || r.End > totalPages {
			return nil, fmt.Errorf("range %d: end page %d out of range (%d-%d)", i+1, r.End, r.Start, totalPages)
		}
	}

	// Extract each range
	var result [][]byte
	for _, r := range pageRanges {
		pageNumbers := make([]int, 0, r.End-r.Start+1)
		for i := r.Start; i <= r.End; i++ {
			pageNumbers = append(pageNumbers, i)
		}

		extractedPDF, err := ExtractPages(pdfBytes, pageNumbers, password, verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to extract range %d-%d: %w", r.Start, r.End, err)
		}

		result = append(result, extractedPDF)
	}

	return result, nil
}

// PageRange represents a range of pages (1-based, inclusive)
type PageRange struct {
	Start int // First page number (1-based)
	End   int // Last page number (1-based, inclusive)
}

// SplitPDFByPageCount splits a PDF into multiple PDFs with a fixed number of pages each
// Returns a slice of PDF bytes
func SplitPDFByPageCount(pdfBytes []byte, pagesPerPDF int, password []byte, verbose bool) ([][]byte, error) {
	if pagesPerPDF < 1 {
		return nil, fmt.Errorf("pagesPerPDF must be at least 1")
	}

	// Create manipulator to get page count
	manipulator, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to create manipulator: %w", err)
	}

	pageObjNums, err := manipulator.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("failed to get page objects: %w", err)
	}

	totalPages := len(pageObjNums)
	if totalPages == 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}

	// Create ranges
	var ranges []PageRange
	for start := 1; start <= totalPages; start += pagesPerPDF {
		end := start + pagesPerPDF - 1
		if end > totalPages {
			end = totalPages
		}
		ranges = append(ranges, PageRange{Start: start, End: end})
	}

	return SplitPDF(pdfBytes, ranges, password, verbose)
}
