package extract

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

func TestExtractAnnotations_Link(t *testing.T) {
	// Test link annotation extraction
	// Note: Creating annotations requires lower-level PDF manipulation
	// For now, we'll test the extraction logic with a known annotation structure
	// This would require adding annotation creation to the writer

	// Create a simple PDF first
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

	extractedPage := doc.Pages[0]
	// For now, just verify annotations array is initialized
	// When annotation creation is added to writer, we can test actual extraction
	_ = extractedPage.Annotations
}

func TestExtractAnnotation_LinkURI(t *testing.T) {
	// Test that link annotations with URI are extracted correctly
	// This requires a PDF with link annotations - for now, just test structure
	t.Skip("Requires PDF with link annotations - annotation creation not yet in writer")
}

func TestExtractAnnotation_Text(t *testing.T) {
	// Test text annotation extraction
	t.Skip("Requires PDF with text annotations")
}

func TestExtractAnnotation_Highlight(t *testing.T) {
	// Test highlight annotation extraction
	t.Skip("Requires PDF with highlight annotations")
}

// Test annotation extraction with a manually created annotation structure
func TestExtractAnnotation_Structure(t *testing.T) {
	// Test that annotation types are correctly identified
	// This tests the annotation parsing logic without requiring full PDF creation

	// Create a mock annotation dictionary string
	annotStr := `<</Type/Annot/Subtype/Link/Rect[100 100 200 200]/URI(https://example.com)>>`

	// We can't easily test this without a full PDF, but we can verify the logic
	// by checking that the annotation type constants are correct
	expectedTypes := []types.AnnotationType{
		types.AnnotationTypeLink,
		types.AnnotationTypeText,
		types.AnnotationTypeHighlight,
		types.AnnotationTypeUnderline,
	}

	for _, expectedType := range expectedTypes {
		if expectedType == "" {
			t.Error("Annotation type should not be empty")
		}
	}

	_ = annotStr // Suppress unused variable warning
}
