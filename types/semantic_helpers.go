package types

import "strings"

// FullText returns all text items concatenated with newlines
func (doc *ContentDocument) FullText() string {
	var b strings.Builder
	for i, item := range doc.TextItems {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(item.Text)
	}
	return b.String()
}

// SectionHeaders returns all text items labeled as section headers or titles
func (doc *ContentDocument) SectionHeaders() []TextItem {
	var headers []TextItem
	for _, item := range doc.TextItems {
		if item.Label == TextLabelSectionHeader || item.Label == TextLabelTitle {
			headers = append(headers, item)
		}
	}
	return headers
}

// PageTextItems returns text items whose bounding box is on the given page
func (doc *ContentDocument) PageTextItems(page int) []TextItem {
	var items []TextItem
	for _, item := range doc.TextItems {
		if item.BBox != nil && item.BBox.Page == page {
			items = append(items, item)
		}
	}
	return items
}

// FindSection returns the first text item with matching label and case-insensitive text substring
func (doc *ContentDocument) FindSection(name string) *TextItem {
	lower := strings.ToLower(name)
	for i, item := range doc.TextItems {
		if (item.Label == TextLabelSectionHeader || item.Label == TextLabelTitle) &&
			strings.Contains(strings.ToLower(item.Text), lower) {
			return &doc.TextItems[i]
		}
	}
	return nil
}

// TextBetweenHeaders returns the concatenated text of all text items between two headers (by name substring)
// Start is inclusive, end is exclusive. If end is empty, returns everything after start.
func (doc *ContentDocument) TextBetweenHeaders(start, end string) string {
	startLower := strings.ToLower(start)
	endLower := strings.ToLower(end)

	var b strings.Builder
	collecting := false
	for _, item := range doc.TextItems {
		isHeader := item.Label == TextLabelSectionHeader || item.Label == TextLabelTitle
		textLower := strings.ToLower(item.Text)

		if !collecting && isHeader && strings.Contains(textLower, startLower) {
			collecting = true
			continue
		}
		if collecting && isHeader && end != "" && strings.Contains(textLower, endLower) {
			break
		}
		if collecting {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(item.Text)
		}
	}
	return b.String()
}

// TableDicts converts tables into a slice of row-maps (column-header → cell-text).
// Uses the first row as column headers. Returns nil if no tables exist.
func (doc *ContentDocument) TableDicts() [][]map[string]string {
	if len(doc.Tables) == 0 {
		return nil
	}

	var result [][]map[string]string
	for _, table := range doc.Tables {
		if table.NumRows < 2 || table.NumCols == 0 {
			continue
		}

		// Build a 2D grid
		grid := make([][]string, table.NumRows)
		for i := range grid {
			grid[i] = make([]string, table.NumCols)
		}
		for _, cell := range table.Cells {
			if cell.Row < table.NumRows && cell.Col < table.NumCols {
				grid[cell.Row][cell.Col] = cell.Text
			}
		}

		// First row is headers
		headers := grid[0]
		var rows []map[string]string
		for r := 1; r < table.NumRows; r++ {
			row := make(map[string]string, len(headers))
			for c, h := range headers {
				if c < len(grid[r]) {
					row[h] = grid[r][c]
				}
			}
			rows = append(rows, row)
		}
		result = append(result, rows)
	}
	return result
}

// HasSemanticContent returns true if any semantic analysis results are present
func (doc *ContentDocument) HasSemanticContent() bool {
	return len(doc.TextItems) > 0 || len(doc.Tables) > 0 || len(doc.Pictures) > 0 || len(doc.Formulas) > 0
}
