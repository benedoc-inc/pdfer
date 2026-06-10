package extract

import (
	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// CreateTestPDFWithText creates a simple PDF with known text content for testing extraction
// Returns the PDF bytes and the expected text elements
func CreateTestPDFWithText(texts []TestText) ([]byte, []types.TextElement, error) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	var expectedElements []types.TextElement

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)

	for i, text := range texts {
		content.SetTextPosition(text.X, text.Y)
		content.SetFont(fontName, text.FontSize)
		content.ShowText(text.Text)

		expectedElements = append(expectedElements, types.TextElement{
			Text:     text.Text,
			X:        text.X,
			Y:        text.Y,
			FontName: fontName,
			FontSize: text.FontSize,
			Width:    text.Width, // Approximate, will be refined by extractor
			Height:   text.FontSize,
		})

		// Move down for next line
		if i < len(texts)-1 {
			content.SetTextPosition(text.X, text.Y-15)
		}
	}

	content.EndText()

	pdfBytes, err := builder.Bytes()
	if err != nil {
		return nil, nil, err
	}

	return pdfBytes, expectedElements, nil
}

// TestText represents text to be added to a test PDF
type TestText struct {
	Text     string
	X        float64
	Y        float64
	FontSize float64
	Width    float64 // Expected width (approximate)
}

// CreateTestPDFWithComplexText creates a PDF with complex text operations for testing
func CreateTestPDFWithComplexText() ([]byte, []types.TextElement, error) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	var expectedElements []types.TextElement

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 14)

	// Simple text
	content.SetTextPosition(72, 720)
	content.ShowText("Simple Text")
	expectedElements = append(expectedElements, types.TextElement{
		Text:     "Simple Text",
		X:        72,
		Y:        720,
		FontName: fontName,
		FontSize: 14,
		Height:   14,
	})

	// Text with character spacing
	content.SetCharSpacing(2)
	content.SetTextPosition(72, 700)
	content.ShowText("Spaced Text")
	expectedElements = append(expectedElements, types.TextElement{
		Text:        "Spaced Text",
		X:           72,
		Y:           700,
		FontName:    fontName,
		FontSize:    14,
		CharSpacing: 2,
		Height:      14,
	})

	// Text with word spacing
	content.SetCharSpacing(0)
	content.SetWordSpacing(5)
	content.SetTextPosition(72, 680)
	content.ShowText("Word Spaced Text")
	expectedElements = append(expectedElements, types.TextElement{
		Text:        "Word Spaced Text",
		X:           72,
		Y:           680,
		FontName:    fontName,
		FontSize:    14,
		WordSpacing: 5,
		Height:      14,
	})

	// Text with text matrix
	content.SetWordSpacing(0)
	content.SetTextMatrix(1, 0, 0, 1, 72, 660)
	content.ShowText("Matrix Text")
	expectedElements = append(expectedElements, types.TextElement{
		Text:       "Matrix Text",
		X:          72,
		Y:          660,
		FontName:   fontName,
		FontSize:   14,
		TextMatrix: [6]float64{1, 0, 0, 1, 72, 660},
		Height:     14,
	})

	// Text array (TJ operator)
	content.SetTextPosition(72, 640)
	content.ShowTextArray([]interface{}{"Array", -20, "Text"})
	expectedElements = append(expectedElements, types.TextElement{
		Text:     "ArrayText", // Concatenated
		X:        72,
		Y:        640,
		FontName: fontName,
		FontSize: 14,
		Height:   14,
	})

	content.EndText()

	pdfBytes, err := builder.Bytes()
	if err != nil {
		return nil, nil, err
	}

	return pdfBytes, expectedElements, nil
}

// CreateTestPDFWithGraphics creates a PDF with graphics for testing extraction
func CreateTestPDFWithGraphics() ([]byte, []types.Graphic, error) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	var expectedGraphics []types.Graphic

	content := page.Content()

	// Rectangle
	content.Rectangle(100, 100, 200, 150)
	content.Fill()
	expectedGraphics = append(expectedGraphics, types.Graphic{
		Type:        types.GraphicTypeRectangle,
		FillColor:   &types.Color{R: 0, G: 0, B: 0}, // Default black
		BoundingBox: &types.Rectangle{LowerX: 100, LowerY: 100, UpperX: 300, UpperY: 250},
	})

	// Line
	content.SetLineWidth(2)
	content.MoveTo(50, 50)
	content.LineTo(200, 200)
	content.Stroke()
	expectedGraphics = append(expectedGraphics, types.Graphic{
		Type:        types.GraphicTypeLine,
		StrokeColor: &types.Color{R: 0, G: 0, B: 0},
		LineWidth:   2,
	})

	// Circle (approximated as path)
	content.SetLineWidth(1)
	content.SetStrokeColorRGB(1, 0, 0) // Red
	content.MoveTo(300, 300)
	// Simple circle approximation
	content.CurveTo(350, 300, 400, 350, 400, 400)
	content.CurveTo(400, 450, 350, 500, 300, 500)
	content.CurveTo(250, 500, 200, 450, 200, 400)
	content.CurveTo(200, 350, 250, 300, 300, 300)
	content.ClosePath()
	content.Stroke()
	expectedGraphics = append(expectedGraphics, types.Graphic{
		Type:        types.GraphicTypeCircle,
		StrokeColor: &types.Color{R: 1, G: 0, B: 0},
		LineWidth:   1,
	})

	pdfBytes, err := builder.Bytes()
	if err != nil {
		return nil, nil, err
	}

	return pdfBytes, expectedGraphics, nil
}

// ParseTestPDF parses a PDF created by test helpers
func ParseTestPDF(pdfBytes []byte) (*parse.PDF, error) {
	return parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: []byte(""),
		Verbose:  false,
	})
}

// CompareTextElements compares extracted text elements with expected ones
// Returns true if they match (allowing for small differences in width calculations)
func CompareTextElements(extracted, expected []types.TextElement) bool {
	if len(extracted) != len(expected) {
		return false
	}

	for i := range extracted {
		e := extracted[i]
		exp := expected[i]

		if e.Text != exp.Text {
			return false
		}
		if abs(e.X-exp.X) > 0.1 {
			return false
		}
		if abs(e.Y-exp.Y) > 0.1 {
			return false
		}
		if e.FontName != exp.FontName {
			return false
		}
		if abs(e.FontSize-exp.FontSize) > 0.1 {
			return false
		}
		// Width can vary, so we allow larger tolerance
		if abs(e.Width-exp.Width) > 5.0 && exp.Width > 0 {
			return false
		}
	}

	return true
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
