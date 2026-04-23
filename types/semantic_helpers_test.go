package types

import "testing"

func TestFullText(t *testing.T) {
	doc := &ContentDocument{
		TextItems: []TextItem{
			{Text: "Hello World", Label: TextLabelTitle},
			{Text: "This is body text.", Label: TextLabelText},
			{Text: "Section 1", Label: TextLabelSectionHeader},
		},
	}

	got := doc.FullText()
	expected := "Hello World\nThis is body text.\nSection 1"
	if got != expected {
		t.Errorf("FullText: want %q, got %q", expected, got)
	}
}

func TestSectionHeaders(t *testing.T) {
	doc := &ContentDocument{
		TextItems: []TextItem{
			{Text: "Title", Label: TextLabelTitle},
			{Text: "Body text", Label: TextLabelText},
			{Text: "Section A", Label: TextLabelSectionHeader, Level: 1},
			{Text: "More body", Label: TextLabelText},
			{Text: "Section B", Label: TextLabelSectionHeader, Level: 2},
		},
	}

	headers := doc.SectionHeaders()
	if len(headers) != 3 {
		t.Fatalf("want 3 headers, got %d", len(headers))
	}
	if headers[0].Text != "Title" {
		t.Errorf("want Title, got %s", headers[0].Text)
	}
	if headers[2].Text != "Section B" {
		t.Errorf("want Section B, got %s", headers[2].Text)
	}
}

func TestPageTextItems(t *testing.T) {
	doc := &ContentDocument{
		TextItems: []TextItem{
			{Text: "Page 1 text", BBox: &SemanticBBox{Page: 1}},
			{Text: "Page 2 text", BBox: &SemanticBBox{Page: 2}},
			{Text: "More page 1", BBox: &SemanticBBox{Page: 1}},
		},
	}

	items := doc.PageTextItems(1)
	if len(items) != 2 {
		t.Errorf("want 2 items for page 1, got %d", len(items))
	}
}

func TestFindSection(t *testing.T) {
	doc := &ContentDocument{
		TextItems: []TextItem{
			{Text: "1. Introduction", Label: TextLabelSectionHeader},
			{Text: "Body", Label: TextLabelText},
			{Text: "2. Methods and Materials", Label: TextLabelSectionHeader},
		},
	}

	s := doc.FindSection("methods")
	if s == nil {
		t.Fatal("expected to find section")
	}
	if s.Text != "2. Methods and Materials" {
		t.Errorf("wrong section: %s", s.Text)
	}

	s = doc.FindSection("nonexistent")
	if s != nil {
		t.Error("expected nil for nonexistent section")
	}
}

func TestTextBetweenHeaders(t *testing.T) {
	doc := &ContentDocument{
		TextItems: []TextItem{
			{Text: "Introduction", Label: TextLabelSectionHeader},
			{Text: "First paragraph.", Label: TextLabelText},
			{Text: "Second paragraph.", Label: TextLabelText},
			{Text: "Methods", Label: TextLabelSectionHeader},
			{Text: "Method details.", Label: TextLabelText},
		},
	}

	got := doc.TextBetweenHeaders("introduction", "methods")
	expected := "First paragraph.\nSecond paragraph."
	if got != expected {
		t.Errorf("want %q, got %q", expected, got)
	}

	// With empty end, get everything after start
	got = doc.TextBetweenHeaders("methods", "")
	if got != "Method details." {
		t.Errorf("want 'Method details.', got %q", got)
	}
}

func TestTableDicts(t *testing.T) {
	doc := &ContentDocument{
		Tables: []TableItem{
			{
				NumRows: 3,
				NumCols: 2,
				Cells: []TableCell{
					{Text: "Name", Row: 0, Col: 0},
					{Text: "Age", Row: 0, Col: 1},
					{Text: "Alice", Row: 1, Col: 0},
					{Text: "30", Row: 1, Col: 1},
					{Text: "Bob", Row: 2, Col: 0},
					{Text: "25", Row: 2, Col: 1},
				},
			},
		},
	}

	dicts := doc.TableDicts()
	if len(dicts) != 1 {
		t.Fatalf("want 1 table, got %d", len(dicts))
	}
	rows := dicts[0]
	if len(rows) != 2 {
		t.Fatalf("want 2 data rows, got %d", len(rows))
	}
	if rows[0]["Name"] != "Alice" {
		t.Errorf("row 0 Name: want Alice, got %s", rows[0]["Name"])
	}
	if rows[1]["Age"] != "25" {
		t.Errorf("row 1 Age: want 25, got %s", rows[1]["Age"])
	}
}

func TestHasSemanticContent(t *testing.T) {
	doc := &ContentDocument{}
	if doc.HasSemanticContent() {
		t.Error("empty doc should not have semantic content")
	}

	doc.TextItems = []TextItem{{Text: "hello"}}
	if !doc.HasSemanticContent() {
		t.Error("doc with TextItems should have semantic content")
	}
}
