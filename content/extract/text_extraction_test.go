package extract

import (
	"os"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
)

func TestTextExtraction_510kSummary(t *testing.T) {
	// Read the 510k summary PDF - this one might be image-only (scanned)
	pdfPath := "../../tests/resources/K141167_summary_1.pdf"
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Parse the PDF
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{})
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Extract pages
	pages, err := ExtractPages(pdfBytes, pdf, false)
	if err != nil {
		t.Fatalf("Failed to extract pages: %v", err)
	}

	if len(pages) == 0 {
		t.Fatal("No pages extracted")
	}

	// Check if this is an image-only PDF
	hasText := false
	hasImages := false
	for _, page := range pages {
		if len(page.Text) > 0 {
			hasText = true
		}
		if len(page.Images) > 0 || (page.Resources != nil && len(page.Resources.Images) > 0) {
			hasImages = true
		}
	}

	if !hasText && hasImages {
		t.Log("This PDF appears to be image-only (scanned document) - no text to extract")
		t.Skip("Image-only PDF - text extraction not applicable")
	}

	// Check that we extracted some readable text
	var allText strings.Builder
	for _, page := range pages {
		for _, textElem := range page.Text {
			allText.WriteString(textElem.Text)
			allText.WriteString(" ")
		}
	}

	extractedText := allText.String()
	if len(extractedText) > 0 {
		t.Logf("Extracted text preview (first 500 chars): %s", extractedText[:min(500, len(extractedText))])

		// Check for readable text - should contain ASCII letters
		readableChars := 0
		for _, r := range extractedText {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == ' ' {
				readableChars++
			}
		}

		totalChars := len(extractedText)
		readableRatio := float64(readableChars) / float64(totalChars)
		t.Logf("Readable character ratio: %.2f%% (%d/%d)", readableRatio*100, readableChars, totalChars)

		// Text should be mostly readable (at least 50% letters/spaces)
		if readableRatio < 0.5 {
			t.Errorf("Text does not appear to be readable (only %.2f%% readable characters)", readableRatio*100)
		}
	}
}

func TestTextExtraction_TestFontsPDF(t *testing.T) {
	// Read a test PDF with known text content
	pdfPath := "../../tests/resources/test_fonts.pdf"
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Parse the PDF
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{})
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Extract pages
	pages, err := ExtractPages(pdfBytes, pdf, false)
	if err != nil {
		t.Fatalf("Failed to extract pages: %v", err)
	}

	if len(pages) == 0 {
		t.Fatal("No pages extracted")
	}

	// Print extracted text for debugging
	var allText strings.Builder
	for _, page := range pages {
		for _, textElem := range page.Text {
			allText.WriteString(textElem.Text)
			allText.WriteString(" ")
		}
	}

	extractedText := allText.String()
	t.Logf("Extracted text: %s", extractedText)
}

func TestExtractToDirectory_510kSummary(t *testing.T) {
	// Test directory extraction with the 510k summary PDF
	pdfPath := "../../tests/resources/K141167_summary_1.pdf"
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Create temp output directory
	outputDir := t.TempDir()

	// Extract to directory
	result, err := ExtractToDirectory(pdfBytes, nil, outputDir, false)
	if err != nil {
		t.Fatalf("Failed to extract to directory: %v", err)
	}

	t.Logf("Extraction result:")
	t.Logf("  Path: %s", result.Path)
	t.Logf("  Structure file: %s", result.StructureFile)
	t.Logf("  Page count: %d", result.PageCount)
	t.Logf("  Has text: %v", result.HasText)
	t.Logf("  Has images: %v", result.HasImages)
	t.Logf("  Is scanned PDF: %v", result.IsScannedPDF)
	t.Logf("  Image count: %d", result.ImageCount)
	t.Logf("  Image files: %v", result.ImageFiles)

	// Verify structure file exists
	if _, err := os.Stat(result.StructureFile); os.IsNotExist(err) {
		t.Error("Structure file does not exist")
	}

	// Verify images were extracted
	if result.IsScannedPDF {
		t.Log("This is a scanned PDF - verifying images were extracted")
		if len(result.ImageFiles) == 0 {
			t.Error("No images extracted from scanned PDF")
		}
	}

	// Verify image files exist
	for _, imgFile := range result.ImageFiles {
		if _, err := os.Stat(imgFile); os.IsNotExist(err) {
			t.Errorf("Image file does not exist: %s", imgFile)
		}
	}
}

func TestExtractToDirectory_TextPDF(t *testing.T) {
	// Test directory extraction with a text-based PDF
	pdfPath := "../../tests/resources/test_fonts.pdf"
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("Skipping test: %v", err)
	}

	// Create temp output directory
	outputDir := t.TempDir()

	// Extract to directory
	result, err := ExtractToDirectory(pdfBytes, nil, outputDir, false)
	if err != nil {
		t.Fatalf("Failed to extract to directory: %v", err)
	}

	t.Logf("Extraction result:")
	t.Logf("  Page count: %d", result.PageCount)
	t.Logf("  Has text: %v", result.HasText)
	t.Logf("  Text char count: %d", result.TextCharCount)
	t.Logf("  Is scanned PDF: %v", result.IsScannedPDF)

	// Verify text was extracted
	if !result.HasText {
		t.Error("Expected text-based PDF to have text content")
	}

	if result.IsScannedPDF {
		t.Error("Text-based PDF should not be classified as scanned")
	}

	if result.TextCharCount == 0 {
		t.Error("Expected non-zero text character count")
	}
}
