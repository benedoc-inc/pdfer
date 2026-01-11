package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestExtractResources_Fonts(t *testing.T) {
	// Test font extraction from Resources
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if len(doc.Pages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(doc.Pages))
	}

	extractedPage := doc.Pages[0]
	if extractedPage.Resources == nil {
		t.Logf("Resources is nil. Page object might not have Resources extracted.")
		t.Fatal("Expected Resources to be extracted")
	}

	// Check that fonts were extracted
	if len(extractedPage.Resources.Fonts) == 0 {
		t.Logf("No fonts found in Resources. Resources structure: Fonts=%d, Images=%d, XObjects=%d",
			len(extractedPage.Resources.Fonts),
			len(extractedPage.Resources.Images),
			len(extractedPage.Resources.XObjects))
		t.Error("Expected at least 1 font, got 0")
	} else {
		// Find the Helvetica font
		found := false
		for name, fontInfo := range extractedPage.Resources.Fonts {
			t.Logf("Font: %s -> Name=%s, Subtype=%s, Family=%s", name, fontInfo.Name, fontInfo.Subtype, fontInfo.Family)
			// BaseFont might be "/Helvetica" or just "Helvetica"
			if fontInfo.Name == "/Helvetica" || fontInfo.Name == "Helvetica" || strings.Contains(fontInfo.Name, "Helvetica") {
				found = true
				if fontInfo.Subtype != "/Type1" {
					t.Errorf("Expected Subtype /Type1, got %s", fontInfo.Subtype)
				}
				t.Logf("Found font: %s -> %s (Subtype: %s)", name, fontInfo.Name, fontInfo.Subtype)
			}
		}
		if !found {
			t.Error("Helvetica font not found in extracted resources")
		}
	}
}

func TestExtractResources_EmbeddedFont(t *testing.T) {
	// Test extraction of embedded fonts
	// This requires a font file - skip if not available
	fontPath := filepath.Join("tests", "resources", "test_font.ttf")
	if _, err := os.Stat(fontPath); os.IsNotExist(err) {
		t.Skip("test_font.ttf not found, skipping embedded font test")
	}

	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Skipf("Failed to read font file: %v", err)
	}

	// Create PDF with embedded font
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	// Note: This would require using the font package to embed
	// For now, just test that the structure supports it
	_ = fontData

	content := page.Content()
	content.BeginText()
	// Use standard font for now
	fontName := page.AddStandardFont("Helvetica")
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if extractedPage.Resources == nil {
		t.Fatal("Expected Resources")
	}

	// Verify fonts are extracted
	if len(extractedPage.Resources.Fonts) == 0 {
		t.Error("Expected fonts to be extracted")
	}
}

func TestExtractResources_Images(t *testing.T) {
	// Test image XObject extraction
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	// Add an image (this creates an XObject)
	// For now, we'll just test that the structure is ready
	// Actual image embedding would require image data

	content := page.Content()
	content.BeginText()
	fontName := page.AddStandardFont("Helvetica")
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test with image")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if extractedPage.Resources == nil {
		t.Fatal("Expected Resources")
	}

	// XObjects should be extracted (even if empty)
	if extractedPage.Resources.XObjects == nil {
		t.Error("Expected XObjects map to be initialized")
	}
}
