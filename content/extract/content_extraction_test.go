package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

func TestExtractText_TjOperator(t *testing.T) {
	// Test simple text extraction with Tj operator
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Hello World")
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
	if len(extractedPage.Text) != 1 {
		t.Fatalf("Expected 1 text element, got %d", len(extractedPage.Text))
	}

	text := extractedPage.Text[0]
	if text.Text != "Hello World" {
		t.Errorf("Expected text 'Hello World', got '%s'", text.Text)
	}
	if text.FontName != fontName {
		t.Errorf("Expected font %s, got %s", fontName, text.FontName)
	}
	if text.FontSize != 12 {
		t.Errorf("Expected font size 12, got %.2f", text.FontSize)
	}
}

func TestExtractText_TJOperator(t *testing.T) {
	// Test text array extraction with TJ operator
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowTextArray([]interface{}{"Hello", -20, "World"})
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) != 1 {
		t.Fatalf("Expected 1 text element, got %d", len(extractedPage.Text))
	}

	text := extractedPage.Text[0]
	if text.Text != "HelloWorld" {
		t.Errorf("Expected text 'HelloWorld', got '%s'", text.Text)
	}
}

func TestExtractText_CharacterSpacing(t *testing.T) {
	// Test text with character spacing
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetCharSpacing(2.5)
	content.SetTextPosition(72, 720)
	content.ShowText("Spaced")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) != 1 {
		t.Fatalf("Expected 1 text element, got %d", len(extractedPage.Text))
	}

	text := extractedPage.Text[0]
	if text.CharSpacing != 2.5 {
		t.Errorf("Expected char spacing 2.5, got %.2f", text.CharSpacing)
	}
}

func TestExtractText_WordSpacing(t *testing.T) {
	// Test text with word spacing
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetWordSpacing(5.0)
	content.SetTextPosition(72, 720)
	content.ShowText("Word Spaced")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) != 1 {
		t.Fatalf("Expected 1 text element, got %d", len(extractedPage.Text))
	}

	text := extractedPage.Text[0]
	if text.WordSpacing != 5.0 {
		t.Errorf("Expected word spacing 5.0, got %.2f", text.WordSpacing)
	}
}

func TestExtractText_TextMatrix(t *testing.T) {
	// Test text with text matrix
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextMatrix(1, 0, 0, 1, 100, 200)
	content.ShowText("Matrix")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) != 1 {
		t.Fatalf("Expected 1 text element, got %d", len(extractedPage.Text))
	}

	text := extractedPage.Text[0]
	expectedMatrix := [6]float64{1, 0, 0, 1, 100, 200}
	if text.TextMatrix != expectedMatrix {
		t.Errorf("Expected text matrix %v, got %v", expectedMatrix, text.TextMatrix)
	}
}

func TestExtractGraphics_Rectangle(t *testing.T) {
	// Test rectangle extraction
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()
	content.SetFillColorRGB(0.8, 0.8, 0.8)
	content.Rectangle(100, 100, 200, 150)
	content.Fill()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Graphics) != 1 {
		t.Fatalf("Expected 1 graphic, got %d", len(extractedPage.Graphics))
	}

	graphic := extractedPage.Graphics[0]
	if graphic.Type != types.GraphicTypeRectangle {
		t.Errorf("Expected rectangle, got %s", graphic.Type)
	}
	if graphic.FillColor == nil {
		t.Error("Expected fill color")
	} else {
		if graphic.FillColor.R != 0.8 || graphic.FillColor.G != 0.8 || graphic.FillColor.B != 0.8 {
			t.Errorf("Expected RGB(0.8, 0.8, 0.8), got RGB(%.2f, %.2f, %.2f)",
				graphic.FillColor.R, graphic.FillColor.G, graphic.FillColor.B)
		}
	}
	if graphic.BoundingBox == nil {
		t.Error("Expected bounding box")
	} else {
		if graphic.BoundingBox.LowerX != 100 || graphic.BoundingBox.LowerY != 100 {
			t.Errorf("Expected lower corner (100, 100), got (%.2f, %.2f)",
				graphic.BoundingBox.LowerX, graphic.BoundingBox.LowerY)
		}
		if graphic.BoundingBox.UpperX != 300 || graphic.BoundingBox.UpperY != 250 {
			t.Errorf("Expected upper corner (300, 250), got (%.2f, %.2f)",
				graphic.BoundingBox.UpperX, graphic.BoundingBox.UpperY)
		}
	}
}

func TestExtractGraphics_Line(t *testing.T) {
	// Test line extraction
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()
	content.SetLineWidth(2)
	content.SetStrokeColorRGB(1, 0, 0)
	content.MoveTo(50, 50)
	content.LineTo(200, 200)
	content.Stroke()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Graphics) < 1 {
		t.Fatalf("Expected at least 1 graphic, got %d", len(extractedPage.Graphics))
	}

	// Find line graphic
	var lineGraphic *types.Graphic
	for i := range extractedPage.Graphics {
		if extractedPage.Graphics[i].Type == types.GraphicTypeLine {
			lineGraphic = &extractedPage.Graphics[i]
			break
		}
	}

	if lineGraphic == nil {
		t.Fatal("Expected line graphic not found")
	}

	if lineGraphic.LineWidth != 2 {
		t.Errorf("Expected line width 2, got %.2f", lineGraphic.LineWidth)
	}
	if lineGraphic.StrokeColor == nil {
		t.Error("Expected stroke color")
	} else {
		if lineGraphic.StrokeColor.R != 1 || lineGraphic.StrokeColor.G != 0 || lineGraphic.StrokeColor.B != 0 {
			t.Errorf("Expected red stroke, got RGB(%.2f, %.2f, %.2f)",
				lineGraphic.StrokeColor.R, lineGraphic.StrokeColor.G, lineGraphic.StrokeColor.B)
		}
	}
}

func TestExtractGraphics_LineWidth(t *testing.T) {
	// Test line width extraction
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()
	content.SetLineWidth(3.5)
	content.MoveTo(0, 0)
	content.LineTo(100, 100)
	content.Stroke()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	// Find line graphic
	for _, graphic := range extractedPage.Graphics {
		if graphic.Type == types.GraphicTypeLine {
			if graphic.LineWidth != 3.5 {
				t.Errorf("Expected line width 3.5, got %.2f", graphic.LineWidth)
			}
			return
		}
	}
	t.Error("Line graphic not found")
}

func TestExtractGraphics_Colors(t *testing.T) {
	// Test color extraction for fill and stroke
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	content := page.Content()
	content.SetFillColorRGB(0.2, 0.4, 0.6)
	content.SetStrokeColorRGB(0.8, 0.1, 0.3)
	content.Rectangle(0, 0, 100, 100)
	content.FillStroke()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Graphics) != 1 {
		t.Fatalf("Expected 1 graphic, got %d", len(extractedPage.Graphics))
	}

	graphic := extractedPage.Graphics[0]
	if graphic.FillColor == nil {
		t.Error("Expected fill color")
	} else {
		if graphic.FillColor.R != 0.2 || graphic.FillColor.G != 0.4 || graphic.FillColor.B != 0.6 {
			t.Errorf("Expected fill RGB(0.2, 0.4, 0.6), got RGB(%.2f, %.2f, %.2f)",
				graphic.FillColor.R, graphic.FillColor.G, graphic.FillColor.B)
		}
	}
	if graphic.StrokeColor == nil {
		t.Error("Expected stroke color")
	} else {
		if graphic.StrokeColor.R != 0.8 || graphic.StrokeColor.G != 0.1 || graphic.StrokeColor.B != 0.3 {
			t.Errorf("Expected stroke RGB(0.8, 0.1, 0.3), got RGB(%.2f, %.2f, %.2f)",
				graphic.StrokeColor.R, graphic.StrokeColor.G, graphic.StrokeColor.B)
		}
	}
}

func TestExtractMultipleContentTypes(t *testing.T) {
	// Test extraction of multiple content types on same page
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()

	// Add text
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Text 1")
	content.SetTextPosition(72, 700)
	content.ShowText("Text 2")
	content.EndText()

	// Add graphics
	content.SetFillColorRGB(0.5, 0.5, 0.5)
	content.Rectangle(100, 100, 50, 50)
	content.Fill()

	content.SetLineWidth(1)
	content.MoveTo(200, 200)
	content.LineTo(250, 250)
	content.Stroke()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	// Save for inspection
	resourceDir := filepath.Join("tests", "resources")
	if err := os.MkdirAll(resourceDir, 0755); err != nil {
		t.Fatalf("Failed to create resource directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "test_multiple_content.pdf"), pdfBytes, 0644); err != nil {
		t.Fatalf("Failed to write test PDF: %v", err)
	}

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) < 2 {
		t.Errorf("Expected at least 2 text elements, got %d", len(extractedPage.Text))
	}
	if len(extractedPage.Graphics) < 2 {
		t.Errorf("Expected at least 2 graphics, got %d", len(extractedPage.Graphics))
	}

	// Verify text
	foundText1 := false
	foundText2 := false
	for _, text := range extractedPage.Text {
		if text.Text == "Text 1" {
			foundText1 = true
		}
		if text.Text == "Text 2" {
			foundText2 = true
		}
	}
	if !foundText1 {
		t.Error("Text 1 not found")
	}
	if !foundText2 {
		t.Error("Text 2 not found")
	}

	// Verify graphics
	foundRect := false
	foundLine := false
	for _, graphic := range extractedPage.Graphics {
		if graphic.Type == types.GraphicTypeRectangle {
			foundRect = true
		}
		if graphic.Type == types.GraphicTypeLine {
			foundLine = true
		}
	}
	if !foundRect {
		t.Error("Rectangle not found")
	}
	if !foundLine {
		t.Error("Line not found")
	}
}

func TestExtractComplexTextOperations(t *testing.T) {
	// Test all text operations together
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeA4)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 14)

	// Simple text
	content.SetTextPosition(72, 720)
	content.ShowText("Simple")

	// Text with spacing
	content.SetCharSpacing(2)
	content.SetTextPosition(72, 700)
	content.ShowText("Spaced")

	// Text with word spacing
	content.SetCharSpacing(0)
	content.SetWordSpacing(5)
	content.SetTextPosition(72, 680)
	content.ShowText("Word Spaced")

	// Text with matrix
	content.SetWordSpacing(0)
	content.SetTextMatrix(1, 0, 0, 1, 72, 660)
	content.ShowText("Matrix")

	// Text array
	content.SetTextPosition(72, 640)
	content.ShowTextArray([]interface{}{"Array", -20, "Text"})

	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	extractedPage := doc.Pages[0]
	if len(extractedPage.Text) < 5 {
		t.Errorf("Expected at least 5 text elements, got %d", len(extractedPage.Text))
	}

	// Verify all text strings are present
	expectedTexts := []string{"Simple", "Spaced", "Word Spaced", "Matrix", "ArrayText"}
	found := make(map[string]bool)
	for _, text := range extractedPage.Text {
		for _, expected := range expectedTexts {
			if text.Text == expected || (expected == "ArrayText" && text.Text == "ArrayText") {
				found[expected] = true
			}
		}
	}

	for _, expected := range expectedTexts {
		if !found[expected] {
			t.Errorf("Expected text '%s' not found", expected)
		}
	}
}
