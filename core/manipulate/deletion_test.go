package manipulate

import (
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/write"
)

func TestDeletePage(t *testing.T) {
	// Create a test PDF with 3 pages
	builder := write.NewSimplePDFBuilder()

	for i := 1; i <= 3; i++ {
		page := builder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Delete page 2
	manipulator, err := NewPDFManipulator(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to create manipulator: %v", err)
	}

	err = manipulator.DeletePage(2)
	if err != nil {
		t.Fatalf("Failed to delete page: %v", err)
	}

	// Rebuild PDF
	modifiedPDF, err := manipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	if len(modifiedPDF) == 0 {
		t.Fatal("Rebuilt PDF is empty")
	}

	// Verify page was deleted using ExtractContent
	doc, err := extract.ExtractContent(modifiedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 2 {
		t.Errorf("Expected 2 pages after deletion, got %d", len(doc.Pages))
	}
}

func TestDeletePages(t *testing.T) {
	// Create a test PDF with 5 pages
	builder := write.NewSimplePDFBuilder()

	for i := 1; i <= 5; i++ {
		page := builder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Delete pages 2 and 4
	manipulator, err := NewPDFManipulator(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to create manipulator: %v", err)
	}

	err = manipulator.DeletePages([]int{2, 4})
	if err != nil {
		t.Fatalf("Failed to delete pages: %v", err)
	}

	// Rebuild PDF
	modifiedPDF, err := manipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	// Verify pages were deleted using ExtractContent
	doc, err := extract.ExtractContent(modifiedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 3 {
		t.Errorf("Expected 3 pages after deletion, got %d", len(doc.Pages))
	}
}
