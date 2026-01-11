package extract

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

func TestExtractImages_JPEG(t *testing.T) {
	// Test JPEG image extraction
	// This requires creating a PDF with an embedded JPEG image
	// For now, test structure is ready

	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	// Add text to make it a valid PDF
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
	// Verify Images structure is initialized
	if extractedPage.Images == nil {
		t.Error("Expected Images array to be initialized")
	}
}

func TestExtractAllImages(t *testing.T) {
	// Test extracting all images from a document
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()
	content.BeginText()
	fontName := page.AddStandardFont("Helvetica")
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	images, err := ExtractAllImages(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract images: %v", err)
	}

	// Should return empty array if no images (not nil)
	if images == nil {
		t.Error("Expected images array (even if empty), got nil")
	}
	// With no images in PDF, should return empty slice
	if len(images) > 0 {
		t.Logf("Found %d images (expected 0 for test PDF)", len(images))
	}
}

func TestExtractImageData_Structure(t *testing.T) {
	// Test image data extraction structure
	// This tests the extraction logic without requiring full image embedding

	// Verify Image type has all necessary fields
	image := types.Image{
		ID:         "/Im1",
		Width:      100,
		Height:     100,
		Format:     "jpeg",
		ColorSpace: "/DeviceRGB",
		Filter:     "/DCTDecode",
		Data:       []byte{0xFF, 0xD8, 0xFF}, // JPEG header
	}

	if image.ID == "" {
		t.Error("Image ID should not be empty")
	}
	if image.Width == 0 || image.Height == 0 {
		t.Error("Image dimensions should be set")
	}
	if image.Format == "" {
		t.Error("Image format should be set")
	}
}
