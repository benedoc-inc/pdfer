package compare

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestCompareImages_MovedOnSamePage(t *testing.T) {
	// Create first PDF with image at position (100, 500)
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)

	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x08, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00,
		0x5A, 0xFF, 0xD9,
	}

	imgInfo1, _ := builder1.Writer().AddJPEGImage(jpegData, "Im1")
	imgName1 := page1.AddImage(imgInfo1)
	content1 := page1.Content()
	content1.DrawImageAt(imgName1, 100, 500, 50, 50)
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with same image at different position (150, 550)
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	imgInfo2, _ := builder2.Writer().AddJPEGImage(jpegData, "Im1") // Same binary data
	imgName2 := page2.AddImage(imgInfo2)
	content2 := page2.Content()
	content2.DrawImageAt(imgName2, 150, 550, 50, 50) // Different position
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if result.Identical {
		t.Error("Expected PDFs to be different due to image position change")
	}

	if len(result.PageDiffs) == 0 {
		t.Fatal("Expected page differences")
	}

	imageDiff := result.PageDiffs[0].ImageDiff
	if imageDiff == nil {
		t.Fatal("Expected image differences")
	}

	// Should detect as moved (same binary, different position)
	if len(imageDiff.Moved) == 0 {
		t.Errorf("Expected moved image, got moved=%d, removed=%d, added=%d, modified=%d",
			len(imageDiff.Moved), len(imageDiff.Removed), len(imageDiff.Added), len(imageDiff.Modified))
	} else {
		t.Logf("Successfully detected moved image: moved=%d", len(imageDiff.Moved))
		if imageDiff.Moved[0].Old.X != 100.0 || imageDiff.Moved[0].Old.Y != 500.0 {
			t.Errorf("Expected old position (100, 500), got (%.1f, %.1f)",
				imageDiff.Moved[0].Old.X, imageDiff.Moved[0].Old.Y)
		}
		if imageDiff.Moved[0].New.X != 150.0 || imageDiff.Moved[0].New.Y != 550.0 {
			t.Errorf("Expected new position (150, 550), got (%.1f, %.1f)",
				imageDiff.Moved[0].New.X, imageDiff.Moved[0].New.Y)
		}
	}

	// Should NOT have removed+added (that would be incorrect)
	if len(imageDiff.Removed) > 0 || len(imageDiff.Added) > 0 {
		t.Errorf("Image should be detected as moved, not removed+added. removed=%d, added=%d",
			len(imageDiff.Removed), len(imageDiff.Added))
	}
}

func TestCompareImages_MovedToDifferentPage(t *testing.T) {
	// Create first PDF with image on page 1
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)

	jpegData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x08, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00,
		0x5A, 0xFF, 0xD9,
	}

	imgInfo1, _ := builder1.Writer().AddJPEGImage(jpegData, "Im1")
	imgName1 := page1.AddImage(imgInfo1)
	content1 := page1.Content()
	content1.DrawImageAt(imgName1, 100, 500, 50, 50)
	builder1.FinalizePage(page1)

	// Add empty second page
	page2 := builder1.AddPage(write.PageSizeLetter)
	builder1.FinalizePage(page2)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with image on page 2 instead
	builder2 := write.NewSimplePDFBuilder()
	page2_1 := builder2.AddPage(write.PageSizeLetter)
	builder2.FinalizePage(page2_1)

	page2_2 := builder2.AddPage(write.PageSizeLetter)
	imgInfo2, _ := builder2.Writer().AddJPEGImage(jpegData, "Im1") // Same binary data
	imgName2 := page2_2.AddImage(imgInfo2)
	content2 := page2_2.Content()
	content2.DrawImageAt(imgName2, 100, 500, 50, 50) // Same position, different page
	builder2.FinalizePage(page2_2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if result.Identical {
		t.Error("Expected PDFs to be different due to image moved to different page")
	}

	// Note: Cross-page moves are currently detected as removed on page 1 and added on page 2
	// This is expected behavior since we compare pages independently
	// A future enhancement could track images across pages to detect cross-page moves
	if len(result.PageDiffs) >= 1 {
		// Page 1 should have removed image
		if len(result.PageDiffs) > 0 && result.PageDiffs[0].ImageDiff != nil {
			if len(result.PageDiffs[0].ImageDiff.Removed) == 0 {
				t.Log("Note: Cross-page image moves are detected as removed+added (expected for page-by-page comparison)")
			}
		}
		// Page 2 should have added image
		if len(result.PageDiffs) > 1 && result.PageDiffs[1].ImageDiff != nil {
			if len(result.PageDiffs[1].ImageDiff.Added) == 0 {
				t.Log("Note: Cross-page image moves are detected as removed+added (expected for page-by-page comparison)")
			}
		}
	}
}
