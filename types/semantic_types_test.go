package types

import (
	"encoding/json"
	"testing"
)

func TestSemanticBBox_JSON(t *testing.T) {
	bbox := SemanticBBox{Left: 72, Right: 540, Top: 720, Bottom: 36, Page: 1}
	data, err := json.Marshal(bbox)
	if err != nil {
		t.Fatal(err)
	}

	// Verify short JSON keys for Python compatibility
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}

	if m["l"] != 72.0 {
		t.Errorf("expected l=72, got %v", m["l"])
	}
	if m["r"] != 540.0 {
		t.Errorf("expected r=540, got %v", m["r"])
	}
	if m["t"] != 720.0 {
		t.Errorf("expected t=720, got %v", m["t"])
	}
	if m["b"] != 36.0 {
		t.Errorf("expected b=36, got %v", m["b"])
	}
	if m["page"] != 1.0 {
		t.Errorf("expected page=1, got %v", m["page"])
	}
}

func TestTextItem_JSON_RoundTrip(t *testing.T) {
	item := TextItem{
		Text:  "Introduction",
		Label: TextLabelSectionHeader,
		BBox:  &SemanticBBox{Left: 72, Right: 200, Top: 720, Bottom: 706, Page: 1},
		Level: 1,
		WordCells: []WordCell{
			{Text: "Introduction", BBox: &SemanticBBox{Left: 72, Right: 200, Top: 720, Bottom: 706, Page: 1}, Confidence: 1.0},
		},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	var decoded TextItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Text != "Introduction" {
		t.Errorf("text: want Introduction, got %s", decoded.Text)
	}
	if decoded.Label != TextLabelSectionHeader {
		t.Errorf("label: want section_header, got %s", decoded.Label)
	}
	if decoded.Level != 1 {
		t.Errorf("level: want 1, got %d", decoded.Level)
	}
	if len(decoded.WordCells) != 1 {
		t.Errorf("word_cells: want 1, got %d", len(decoded.WordCells))
	}
}

func TestTableItem_JSON(t *testing.T) {
	table := TableItem{
		Cells: []TableCell{
			{Text: "Name", Row: 0, Col: 0},
			{Text: "Age", Row: 0, Col: 1},
			{Text: "Alice", Row: 1, Col: 0},
			{Text: "30", Row: 1, Col: 1},
		},
		NumRows: 2,
		NumCols: 2,
	}

	data, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}

	var decoded TableItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.NumRows != 2 || decoded.NumCols != 2 {
		t.Errorf("dimensions: want 2x2, got %dx%d", decoded.NumRows, decoded.NumCols)
	}
	if len(decoded.Cells) != 4 {
		t.Errorf("cells: want 4, got %d", len(decoded.Cells))
	}
}

func TestTextLabel_Values(t *testing.T) {
	labels := []TextLabel{
		TextLabelText, TextLabelSectionHeader, TextLabelTitle,
		TextLabelCaption, TextLabelListItem, TextLabelPageHeader,
		TextLabelPageFooter, TextLabelFootnote, TextLabelFormula,
	}
	expected := []string{
		"text", "section_header", "title", "caption", "list_item",
		"page_header", "page_footer", "footnote", "formula",
	}

	for i, label := range labels {
		if string(label) != expected[i] {
			t.Errorf("label %d: want %s, got %s", i, expected[i], label)
		}
	}
}

func TestContentDocument_SemanticFields_Omitempty(t *testing.T) {
	doc := ContentDocument{
		Pages: []Page{{PageNumber: 1, Width: 612, Height: 792}},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	// These fields should be omitted when empty
	if contains(s, "text_items") {
		t.Error("text_items should be omitted when nil")
	}
	if contains(s, "tables") {
		t.Error("tables should be omitted when nil")
	}
	if contains(s, "pictures") {
		t.Error("pictures should be omitted when nil")
	}
	if contains(s, "formulas") {
		t.Error("formulas should be omitted when nil")
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
