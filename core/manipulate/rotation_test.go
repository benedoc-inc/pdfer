package manipulate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/write"
)

func TestRotatePage(t *testing.T) {
	// Create a test PDF
	builder := write.NewSimplePDFBuilder()
	page1 := builder.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Page 1")
	content1.EndText()
	builder.FinalizePage(page1)

	page2 := builder.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Page 2")
	content2.EndText()
	builder.FinalizePage(page2)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Rotate first page by 90 degrees
	manipulator, err := NewPDFManipulator(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to create manipulator: %v", err)
	}

	err = manipulator.RotatePage(1, 90)
	if err != nil {
		t.Fatalf("Failed to rotate page: %v", err)
	}

	// Rebuild PDF
	rotatedPDF, err := manipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	// Verify rotation was applied
	doc, err := extract.ExtractContent(rotatedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) < 1 {
		t.Fatal("Expected at least 1 page")
	}

	// First page should be rotated 90 degrees
	if doc.Pages[0].Rotation != 90 {
		t.Errorf("Expected page 1 rotation to be 90, got %d", doc.Pages[0].Rotation)
	}

	// Second page should not be rotated
	if len(doc.Pages) > 1 && doc.Pages[1].Rotation != 0 {
		t.Errorf("Expected page 2 rotation to be 0, got %d", doc.Pages[1].Rotation)
	}
}

func TestRotateAllPages(t *testing.T) {
	// Create a test PDF with 2 pages
	builder := write.NewSimplePDFBuilder()
	page1 := builder.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Page 1")
	content1.EndText()
	builder.FinalizePage(page1)

	page2 := builder.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Page 2")
	content2.EndText()
	builder.FinalizePage(page2)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Rotate all pages by 180 degrees
	manipulator, err := NewPDFManipulator(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to create manipulator: %v", err)
	}

	err = manipulator.RotateAllPages(180)
	if err != nil {
		t.Fatalf("Failed to rotate all pages: %v", err)
	}

	// Rebuild PDF
	rotatedPDF, err := manipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	// Verify all pages were rotated
	doc, err := extract.ExtractContent(rotatedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 2 {
		t.Fatalf("Expected 2 pages, got %d", len(doc.Pages))
	}

	for i, page := range doc.Pages {
		if page.Rotation != 180 {
			t.Errorf("Expected page %d rotation to be 180, got %d", i+1, page.Rotation)
		}
	}
}

func TestRotatePage_FromDisk(t *testing.T) {
	// Test with a real PDF from disk
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_combined.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	// Rotate first page
	manipulator, err := NewPDFManipulator(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to create manipulator: %v", err)
	}

	err = manipulator.RotatePage(1, 90)
	if err != nil {
		t.Fatalf("Failed to rotate page: %v", err)
	}

	// Rebuild and verify
	rotatedPDF, err := manipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	doc, err := extract.ExtractContent(rotatedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) > 0 && doc.Pages[0].Rotation != 90 {
		t.Errorf("Expected page 1 rotation to be 90, got %d", doc.Pages[0].Rotation)
	}
}
