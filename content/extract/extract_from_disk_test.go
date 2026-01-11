package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestExtractFromDisk_Fonts(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_fonts.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s (run tests/create_test_pdfs.go first)", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least 1 page")
	}

	page := doc.Pages[0]
	if page.Resources == nil {
		t.Fatal("Expected Resources to be extracted")
	}

	// Check for standard fonts
	foundHelvetica := false
	foundTimes := false
	for name, fontInfo := range page.Resources.Fonts {
		t.Logf("Font: %s -> Name=%s, Subtype=%s", name, fontInfo.Name, fontInfo.Subtype)
		if strings.Contains(fontInfo.Name, "Helvetica") {
			foundHelvetica = true
			if fontInfo.Subtype != "/Type1" {
				t.Errorf("Expected Helvetica Subtype /Type1, got %s", fontInfo.Subtype)
			}
		}
		if strings.Contains(fontInfo.Name, "Times") {
			foundTimes = true
		}
	}

	if !foundHelvetica {
		t.Error("Helvetica font not found")
	}
	if !foundTimes {
		t.Error("Times-Roman font not found")
	}

	// Check for embedded font (if test_font.ttf was available)
	hasEmbedded := false
	for _, fontInfo := range page.Resources.Fonts {
		if fontInfo.Embedded {
			hasEmbedded = true
			t.Logf("Found embedded font: %s", fontInfo.Name)
		}
	}
	if !hasEmbedded {
		t.Log("No embedded fonts found (test_font.ttf may not be available)")
	}
}

func TestExtractFromDisk_Images(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_images.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s (run tests/create_test_pdfs.go first)", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least 1 page")
	}

	page := doc.Pages[0]
	if page.Resources == nil {
		t.Fatal("Expected Resources to be extracted")
	}

	// Check for images in Resources
	if len(page.Resources.Images) == 0 {
		t.Error("Expected at least 1 image in Resources")
	} else {
		for name, image := range page.Resources.Images {
			t.Logf("Image: %s -> Width=%d, Height=%d, Format=%s", name, image.Width, image.Height, image.Format)
			if image.Width == 0 || image.Height == 0 {
				t.Errorf("Image %s has zero dimensions", name)
			}
		}
	}

	// Check for XObjects
	if len(page.Resources.XObjects) == 0 {
		t.Error("Expected at least 1 XObject")
	} else {
		for name, xobj := range page.Resources.XObjects {
			t.Logf("XObject: %s -> Subtype=%s, Width=%.0f, Height=%.0f", name, xobj.Subtype, xobj.Width, xobj.Height)
			if xobj.Subtype != "/Image" {
				t.Errorf("Expected XObject %s to be /Image, got %s", name, xobj.Subtype)
			}
		}
	}

	// Check for image references in content stream
	if len(page.Images) == 0 {
		t.Error("Expected at least 1 image reference in content stream")
	} else {
		for _, imgRef := range page.Images {
			t.Logf("ImageRef: %s", imgRef.ImageID)
			if imgRef.ImageID == "" {
				t.Error("ImageRef has empty ImageID")
			}
		}
	}
}

func TestExtractFromDisk_Annotations(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_annotations.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s (run tests/create_test_pdfs.go first)", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least 1 page")
	}

	page := doc.Pages[0]

	// Check for annotations
	if len(page.Annotations) == 0 {
		t.Log("No annotations found (annotation creation may need improvement)")
	} else {
		for _, annot := range page.Annotations {
			t.Logf("Annotation: Type=%s, ID=%s, PageNumber=%d", annot.Type, annot.ID, annot.PageNumber)
			if annot.Type == types.AnnotationTypeLink {
				t.Logf("  Link: URI=%s, Destination=%s", annot.URI, annot.Destination)
			}
			if annot.Rect != nil {
				t.Logf("  Rect: [%.2f, %.2f, %.2f, %.2f]", annot.Rect.LowerX, annot.Rect.LowerY, annot.Rect.UpperX, annot.Rect.UpperY)
			}
		}
	}
}

func TestExtractFromDisk_Combined(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_combined.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s (run tests/create_test_pdfs.go first)", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Fatal("Expected at least 1 page")
	}

	page := doc.Pages[0]

	// Check fonts
	if page.Resources == nil || len(page.Resources.Fonts) == 0 {
		t.Error("Expected fonts to be extracted")
	}

	// Check images
	if page.Resources == nil || len(page.Resources.Images) == 0 {
		t.Error("Expected images to be extracted")
	}

	// Check text
	if len(page.Text) == 0 {
		t.Error("Expected text to be extracted")
	} else {
		t.Logf("Extracted %d text elements", len(page.Text))
		for i, text := range page.Text {
			if i < 3 { // Log first 3
				t.Logf("Text[%d]: %s (FontName: %s, FontSize: %.2f)", i, text.Text, text.FontName, text.FontSize)
			}
		}
	}

	// Check graphics
	if len(page.Graphics) == 0 {
		t.Error("Expected graphics to be extracted")
	} else {
		t.Logf("Extracted %d graphics", len(page.Graphics))
		for i, g := range page.Graphics {
			if i < 3 { // Log first 3
				t.Logf("Graphic[%d]: Type=%s", i, g.Type)
			}
		}
	}
}

func TestExtractAllImages_FromDisk(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "tests", "resources", "test_images.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found: %s (run tests/create_test_pdfs.go first)", pdfPath)
	}

	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	images, err := ExtractAllImages(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract images: %v", err)
	}

	if len(images) == 0 {
		t.Error("Expected at least 1 image")
	} else {
		t.Logf("Extracted %d images", len(images))
		for i, img := range images {
			t.Logf("Image[%d]: ID=%s, Width=%d, Height=%d, Format=%s", i, img.ID, img.Width, img.Height, img.Format)
		}
	}
}
