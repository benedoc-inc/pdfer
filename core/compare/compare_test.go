package compare

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestComparePDFs_Identical(t *testing.T) {
	// Create a test PDF
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	content := page.Content()
	content.BeginText()
	font := page.AddStandardFont("Helvetica")
	content.SetFont(font, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test Document")
	content.EndText()
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Compare PDF with itself
	result, err := ComparePDFs(pdfBytes, pdfBytes, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	if !result.Identical {
		t.Errorf("Expected PDFs to be identical, but found %d differences", result.Summary.TotalDifferences)
		if len(result.Differences) > 0 {
			t.Logf("Differences: %+v", result.Differences)
		}
	}
}

func TestComparePDFs_DifferentText(t *testing.T) {
	// Create first PDF
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Original Text")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, err := builder1.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF1: %v", err)
	}

	// Create second PDF with different text
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Modified Text")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, err := builder2.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF2: %v", err)
	}

	// Compare PDFs
	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	if result.Identical {
		t.Error("Expected PDFs to be different, but they were marked as identical")
	}

	if !result.Summary.ContentChanged {
		t.Error("Expected content to be marked as changed")
	}

	if len(result.PageDiffs) == 0 {
		t.Error("Expected page differences to be found")
	} else if result.PageDiffs[0].TextDiff == nil {
		t.Error("Expected text differences to be found")
	} else {
		textDiff := result.PageDiffs[0].TextDiff
		// Text at same position with different content should be marked as modified
		// or as removed+added if positions don't match
		if len(textDiff.Modified) == 0 && len(textDiff.Removed) == 0 && len(textDiff.Added) == 0 {
			t.Errorf("Expected text differences (modified, removed, or added), got modified=%d, removed=%d, added=%d",
				len(textDiff.Modified), len(textDiff.Removed), len(textDiff.Added))
		}
		// At least one type of difference should be present
		if len(textDiff.Modified) > 0 || len(textDiff.Removed) > 0 || len(textDiff.Added) > 0 {
			t.Logf("Text differences found: modified=%d, removed=%d, added=%d",
				len(textDiff.Modified), len(textDiff.Removed), len(textDiff.Added))
		}
	}
}

func TestComparePDFs_DifferentPageCount(t *testing.T) {
	// Create first PDF with 2 pages
	builder1 := write.NewSimplePDFBuilder()
	for i := 1; i <= 2; i++ {
		page := builder1.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder1.FinalizePage(page)
	}
	pdf1, err := builder1.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF1: %v", err)
	}

	// Create second PDF with 3 pages
	builder2 := write.NewSimplePDFBuilder()
	for i := 1; i <= 3; i++ {
		page := builder2.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder2.FinalizePage(page)
	}
	pdf2, err := builder2.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF2: %v", err)
	}

	// Compare PDFs
	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	if result.Identical {
		t.Error("Expected PDFs to be different due to page count")
	}

	if !result.Summary.StructureChanged {
		t.Error("Expected structure to be marked as changed")
	}

	if result.StructureDiff == nil || result.StructureDiff.PageCountDiff == nil {
		t.Error("Expected page count difference to be found")
	} else {
		if result.StructureDiff.PageCountDiff.OldValue != 2 || result.StructureDiff.PageCountDiff.NewValue != 3 {
			t.Errorf("Expected page count 2 -> 3, got %v -> %v", result.StructureDiff.PageCountDiff.OldValue, result.StructureDiff.PageCountDiff.NewValue)
		}
	}

	// Should have a page difference for the added page
	if len(result.PageDiffs) == 0 {
		t.Error("Expected page differences for added page")
	}
}

func TestComparePDFs_JSONOutput(t *testing.T) {
	// Create two slightly different PDFs
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Version 1")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Version 2")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare PDFs: %v", err)
	}

	// Test JSON serialization
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal result to JSON: %v", err)
	}

	if len(jsonBytes) == 0 {
		t.Error("JSON output is empty")
	}

	// Verify it's valid JSON by unmarshaling
	var decoded ComparisonResult
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if decoded.Identical == result.Identical && decoded.Summary.TotalDifferences == result.Summary.TotalDifferences {
		t.Logf("JSON serialization successful: %d bytes", len(jsonBytes))
	} else {
		t.Error("JSON unmarshaling produced different result")
	}
}
