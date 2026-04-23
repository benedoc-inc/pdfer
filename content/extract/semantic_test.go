package extract

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

func TestSemanticAnalysis_BasicParagraphs(t *testing.T) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)

	// First paragraph line
	content.SetTextPosition(72, 700)
	content.ShowText("This is the first paragraph. It has some text.")

	// Second line of first paragraph (normal spacing)
	content.SetTextPosition(72, 686)
	content.ShowText("And this continues the first paragraph.")

	// Second paragraph (larger gap)
	content.SetTextPosition(72, 650)
	content.ShowText("This is a second paragraph after a larger gap.")

	content.EndText()
	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	if !doc.HasSemanticContent() {
		t.Fatal("Expected semantic content")
	}

	if len(doc.TextItems) == 0 {
		t.Fatal("Expected TextItems")
	}

	if doc.FullText() == "" {
		t.Error("FullText should not be empty")
	}
}

func TestSemanticAnalysis_HeadingDetection(t *testing.T) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()

	// Title (large font) — use Tm for absolute positioning
	content.SetFont(fontName, 24)
	content.SetTextMatrix(1, 0, 0, 1, 72, 720)
	content.ShowText("Document Title")

	// Body text (normal font)
	content.SetFont(fontName, 12)
	content.SetTextMatrix(1, 0, 0, 1, 72, 690)
	content.ShowText("This is regular body text under the title.")

	content.SetTextMatrix(1, 0, 0, 1, 72, 676)
	content.ShowText("More body text here.")

	// Section header (larger than body)
	content.SetFont(fontName, 16)
	content.SetTextMatrix(1, 0, 0, 1, 72, 640)
	content.ShowText("Section One")

	// More body
	content.SetFont(fontName, 12)
	content.SetTextMatrix(1, 0, 0, 1, 72, 620)
	content.ShowText("Content under section one.")

	content.EndText()
	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	headers := doc.SectionHeaders()
	if len(headers) == 0 {
		t.Fatal("Expected at least one section header")
	}

	// The title and section header should be detected
	foundTitle := false
	foundSection := false
	for _, h := range headers {
		if h.Text == "Document Title" {
			foundTitle = true
		}
		if h.Text == "Section One" {
			foundSection = true
		}
	}
	if !foundTitle {
		t.Error("Expected 'Document Title' to be detected as header")
	}
	if !foundSection {
		t.Error("Expected 'Section One' to be detected as header")
	}
}

func TestSemanticAnalysis_WordCells(t *testing.T) {
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

	if len(doc.TextItems) == 0 {
		t.Fatal("Expected TextItems")
	}

	// Check that word cells were generated
	for _, item := range doc.TextItems {
		if len(item.WordCells) == 0 {
			t.Error("Expected WordCells on TextItem")
		}
		for _, wc := range item.WordCells {
			if wc.Confidence != 1.0 {
				t.Errorf("Expected confidence 1.0 for native extraction, got %f", wc.Confidence)
			}
			if wc.FromOCR {
				t.Error("Expected FromOCR=false for native extraction")
			}
		}
	}
}

func TestSemanticAnalysis_BBoxes(t *testing.T) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)
	content.SetTextPosition(72, 720)
	content.ShowText("Test text with BBox")
	content.EndText()
	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	for _, item := range doc.TextItems {
		if item.BBox == nil {
			t.Error("Expected BBox on TextItem")
			continue
		}
		if item.BBox.Page != 1 {
			t.Errorf("Expected page 1, got %d", item.BBox.Page)
		}
		if item.BBox.Left >= item.BBox.Right {
			t.Error("BBox left should be < right")
		}
	}
}

func TestSemanticAnalysis_PictureDetection(t *testing.T) {
	// Verify PictureItems are generated from image refs on pages
	doc := &types.ContentDocument{
		Pages: []types.Page{
			{
				PageNumber: 1,
				Width:      612,
				Height:     792,
				Text: []types.TextElement{
					{Text: "Page with image", X: 72, Y: 720, FontSize: 12, Width: 100, Height: 12, FontName: "/F1"},
				},
				Images: []types.ImageRef{
					{ImageID: "/Im1", X: 72, Y: 400, Width: 200, Height: 160},
				},
			},
		},
	}

	// Run semantic pipeline
	analyzeLayout(doc)
	analyzeSemantics(doc)

	if len(doc.Pictures) == 0 {
		t.Error("Expected at least one PictureItem from image ref")
	} else {
		pic := doc.Pictures[0]
		if pic.BBox == nil {
			t.Error("Expected BBox on PictureItem")
		} else if pic.BBox.Page != 1 {
			t.Errorf("Expected page 1, got %d", pic.BBox.Page)
		}
	}
}

func TestSemanticAnalysis_FindSection(t *testing.T) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()

	// Section headers with body text — use Tm for absolute positioning
	content.SetFont(fontName, 18)
	content.SetTextMatrix(1, 0, 0, 1, 72, 720)
	content.ShowText("Introduction")

	content.SetFont(fontName, 12)
	content.SetTextMatrix(1, 0, 0, 1, 72, 700)
	content.ShowText("Intro body text.")

	content.SetFont(fontName, 18)
	content.SetTextMatrix(1, 0, 0, 1, 72, 660)
	content.ShowText("Methods")

	content.SetFont(fontName, 12)
	content.SetTextMatrix(1, 0, 0, 1, 72, 640)
	content.ShowText("Methods body text.")

	content.EndText()
	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	s := doc.FindSection("introduction")
	if s == nil {
		t.Fatal("Expected to find Introduction section")
	}
	if s.Text != "Introduction" {
		t.Errorf("Expected 'Introduction', got '%s'", s.Text)
	}

	text := doc.TextBetweenHeaders("introduction", "methods")
	if text == "" {
		t.Error("Expected text between Introduction and Methods")
	}
}

func TestPageTextItems(t *testing.T) {
	builder := write.NewSimplePDFBuilder()

	// Page 1
	page1 := builder.AddPage(write.PageSizeLetter)
	fontName := page1.AddStandardFont("Helvetica")
	c1 := page1.Content()
	c1.BeginText()
	c1.SetFont(fontName, 12)
	c1.SetTextPosition(72, 720)
	c1.ShowText("Page 1 text")
	c1.EndText()
	builder.FinalizePage(page1)

	// Page 2
	page2 := builder.AddPage(write.PageSizeLetter)
	fontName2 := page2.AddStandardFont("Helvetica")
	c2 := page2.Content()
	c2.BeginText()
	c2.SetFont(fontName2, 12)
	c2.SetTextPosition(72, 720)
	c2.ShowText("Page 2 text")
	c2.EndText()
	builder.FinalizePage(page2)

	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	p1Items := doc.PageTextItems(1)
	p2Items := doc.PageTextItems(2)

	if len(p1Items) == 0 {
		t.Error("Expected text items on page 1")
	}
	if len(p2Items) == 0 {
		t.Error("Expected text items on page 2")
	}
}

func TestLayoutAnalysis_ReadingOrder(t *testing.T) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	content := page.Content()
	content.BeginText()
	content.SetFont(fontName, 12)

	// Add text in non-reading order using Tm for absolute positioning
	content.SetTextMatrix(1, 0, 0, 1, 72, 600) // bottom
	content.ShowText("Third line")
	content.SetTextMatrix(1, 0, 0, 1, 72, 720) // top
	content.ShowText("First line")
	content.SetTextMatrix(1, 0, 0, 1, 72, 660) // middle
	content.ShowText("Second line")

	content.EndText()
	builder.FinalizePage(page)
	pdfBytes, _ := builder.Bytes()

	doc, err := ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract: %v", err)
	}

	// After layout analysis, text should be in reading order (top to bottom = highest Y first)
	page0 := doc.Pages[0]
	if len(page0.Text) < 3 {
		t.Fatalf("Expected at least 3 text elements, got %d", len(page0.Text))
	}

	if page0.Text[0].Text != "First line" {
		t.Errorf("Expected first element to be 'First line', got '%s'", page0.Text[0].Text)
	}
	if page0.Text[1].Text != "Second line" {
		t.Errorf("Expected second element to be 'Second line', got '%s'", page0.Text[1].Text)
	}
	if page0.Text[2].Text != "Third line" {
		t.Errorf("Expected third element to be 'Third line', got '%s'", page0.Text[2].Text)
	}
}

func TestContentDocument_WithSemanticFields(t *testing.T) {
	// Verify that TextItems, Tables, Pictures, Formulas are populated correctly
	doc := &types.ContentDocument{
		Pages: []types.Page{{PageNumber: 1}},
		TextItems: []types.TextItem{
			{Text: "Test", Label: types.TextLabelText},
		},
		Tables: []types.TableItem{
			{NumRows: 1, NumCols: 1, Cells: []types.TableCell{{Text: "Cell", Row: 0, Col: 0}}},
		},
	}

	if !doc.HasSemanticContent() {
		t.Error("Expected HasSemanticContent to be true")
	}
	if doc.FullText() != "Test" {
		t.Errorf("Expected FullText='Test', got '%s'", doc.FullText())
	}
}
