package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestExtractFromGeneratedPDF(t *testing.T) {
	// Create a PDF with known content
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	// Add text content
	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 14)
	content.SetTextPosition(72, 720)
	content.ShowText("Hello, PDF World!")
	content.SetTextPosition(72, 700)
	content.ShowText("This is a test document.")
	content.SetTextPosition(72, 680)
	content.SetFont(fontName, 12)
	content.ShowText("Testing text extraction.")
	content.EndText()

	// Add some graphics
	content.SetFillColorRGB(0.8, 0.8, 0.8)
	content.Rectangle(100, 100, 200, 150)
	content.Fill()

	content.SetLineWidth(2)
	content.SetStrokeColorRGB(1, 0, 0)
	content.MoveTo(50, 50)
	content.LineTo(200, 200)
	content.Stroke()

	// Finalize the page
	builder.FinalizePage(page)

	// Generate PDF
	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}

	// Save to tests/resources
	resourceDir := filepath.Join("tests", "resources")
	if err := os.MkdirAll(resourceDir, 0755); err != nil {
		t.Fatalf("Failed to create resources directory: %v", err)
	}

	testPDFPath := filepath.Join(resourceDir, "test_extraction.pdf")
	if err := os.WriteFile(testPDFPath, pdfBytes, 0644); err != nil {
		t.Fatalf("Failed to write test PDF: %v", err)
	}

	t.Logf("Generated test PDF: %s (%d bytes)", testPDFPath, len(pdfBytes))

	// Now extract content from it
	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	// Verify metadata
	if doc.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
	if doc.Metadata.PDFVersion == "" {
		t.Error("PDF version should be extracted")
	}

	// Verify pages
	if len(doc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(doc.Pages))
	}

	extractedPage := doc.Pages[0]
	if extractedPage.Width != 612 || extractedPage.Height != 792 {
		t.Errorf("Expected page size 612x792, got %.0fx%.0f", extractedPage.Width, extractedPage.Height)
	}

	// Verify text extraction (when implemented)
	// For now, just check that the page structure is correct
	t.Logf("Extracted page: %d text elements, %d graphics, %d images",
		len(extractedPage.Text), len(extractedPage.Graphics), len(extractedPage.Images))

	// Verify we can serialize to JSON
	jsonBytes, err := ExtractContentToJSON(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to serialize to JSON: %v", err)
	}

	if len(jsonBytes) == 0 {
		t.Error("JSON output should not be empty")
	}

	t.Logf("JSON serialization: %d bytes", len(jsonBytes))
}

func TestExtractComplexText(t *testing.T) {
	// Create a PDF with complex text operations
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeA4)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)

	// Simple text
	content.SetTextPosition(72, 720)
	content.ShowText("Simple Text")

	// Text with character spacing
	content.SetCharSpacing(2)
	content.SetTextPosition(72, 700)
	content.ShowText("Spaced Text")

	// Text with word spacing
	content.SetCharSpacing(0)
	content.SetWordSpacing(5)
	content.SetTextPosition(72, 680)
	content.ShowText("Word Spaced Text")

	// Text with text matrix
	content.SetWordSpacing(0)
	content.SetTextMatrix(1, 0, 0, 1, 72, 660)
	content.ShowText("Matrix Text")

	// Text array (TJ operator)
	content.SetTextPosition(72, 640)
	content.ShowTextArray([]interface{}{"Array", -20, "Text"})

	content.EndText()

	// Finalize the page
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}

	// Save to tests/resources
	resourceDir := filepath.Join("tests", "resources")
	if err := os.MkdirAll(resourceDir, 0755); err != nil {
		t.Fatalf("Failed to create resources directory: %v", err)
	}

	testPDFPath := filepath.Join(resourceDir, "test_complex_text.pdf")
	if err := os.WriteFile(testPDFPath, pdfBytes, 0644); err != nil {
		t.Fatalf("Failed to write test PDF: %v", err)
	}

	t.Logf("Generated complex text PDF: %s (%d bytes)", testPDFPath, len(pdfBytes))

	// Extract content
	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(doc.Pages))
	}

	extractedPage := doc.Pages[0]
	t.Logf("Extracted %d text elements from complex text PDF", len(extractedPage.Text))

	// When text extraction is implemented, we should verify:
	// - All 5 text strings are extracted
	// - Character spacing is captured
	// - Word spacing is captured
	// - Text matrix is captured
	// - Text array is properly parsed
}

func TestExtractGraphics(t *testing.T) {
	// Create a PDF with graphics
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()

	// Rectangle
	content.SetFillColorRGB(0.8, 0.8, 0.8)
	content.Rectangle(100, 100, 200, 150)
	content.Fill()

	// Line
	content.SetLineWidth(2)
	content.SetStrokeColorRGB(1, 0, 0)
	content.MoveTo(50, 50)
	content.LineTo(200, 200)
	content.Stroke()

	// Circle (approximated with curves)
	content.SetLineWidth(1)
	content.SetStrokeColorRGB(0, 0, 1)
	content.MoveTo(300, 300)
	content.CurveTo(350, 300, 400, 350, 400, 400)
	content.CurveTo(400, 450, 350, 500, 300, 500)
	content.CurveTo(250, 500, 200, 450, 200, 400)
	content.CurveTo(200, 350, 250, 300, 300, 300)
	content.ClosePath()
	content.Stroke()

	// Finalize the page
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}

	// Save to tests/resources
	resourceDir := filepath.Join("tests", "resources")
	if err := os.MkdirAll(resourceDir, 0755); err != nil {
		t.Fatalf("Failed to create resources directory: %v", err)
	}

	testPDFPath := filepath.Join(resourceDir, "test_graphics.pdf")
	if err := os.WriteFile(testPDFPath, pdfBytes, 0644); err != nil {
		t.Fatalf("Failed to write test PDF: %v", err)
	}

	t.Logf("Generated graphics PDF: %s (%d bytes)", testPDFPath, len(pdfBytes))

	// Extract content
	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(doc.Pages))
	}

	extractedPage := doc.Pages[0]
	t.Logf("Extracted %d graphics from graphics PDF", len(extractedPage.Graphics))

	// When graphics extraction is implemented, we should verify:
	// - Rectangle is extracted with correct bounds and fill color
	// - Line is extracted with correct endpoints, width, and stroke color
	// - Circle/path is extracted correctly
}
