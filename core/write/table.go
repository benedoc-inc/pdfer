package write

// TableStyle configures table appearance.
type TableStyle struct {
	BorderColor    [3]float64 // RGB; default {0,0,0} black
	BorderWidth    float64    // default 0.5
	HeaderFill     [3]float64 // RGB background for header rows; default {0.85,0.85,0.85}
	CellPadding    float64    // padding inside each cell in points; default 4
	FontName       string     // resource name, e.g. "/F1"; default "/F1"
	FontSize       float64    // points; default 10
	HeaderFontSize float64    // points for header cells; 0 = same as FontSize
}

func (s *TableStyle) fillDefaults() {
	if s.BorderWidth == 0 {
		s.BorderWidth = 0.5
	}
	if s.HeaderFill == [3]float64{} {
		s.HeaderFill = [3]float64{0.85, 0.85, 0.85}
	}
	if s.CellPadding == 0 {
		s.CellPadding = 4
	}
	if s.FontName == "" {
		s.FontName = "/F1"
	}
	if s.FontSize == 0 {
		s.FontSize = 10
	}
	if s.HeaderFontSize == 0 {
		s.HeaderFontSize = s.FontSize
	}
}

// CellBuilder holds the content and style for a single table cell.
type CellBuilder struct {
	Text    string
	ColSpan int
	RowSpan int
	Align   TextAlign
	BgColor *[3]float64
}

// WithColSpan sets the column span for this cell.
func (c *CellBuilder) WithColSpan(n int) *CellBuilder {
	c.ColSpan = n
	return c
}

// WithRowSpan sets the row span for this cell.
func (c *CellBuilder) WithRowSpan(n int) *CellBuilder {
	c.RowSpan = n
	return c
}

// WithBgColor sets a per-cell background color.
func (c *CellBuilder) WithBgColor(r, g, b float64) *CellBuilder {
	color := [3]float64{r, g, b}
	c.BgColor = &color
	return c
}

// WithAlign sets horizontal alignment.
func (c *CellBuilder) WithAlign(a TextAlign) *CellBuilder {
	c.Align = a
	return c
}

// RowBuilder holds the cells for one table row.
type RowBuilder struct {
	cells     []*CellBuilder
	isHeader  bool
	minHeight float64 // minimum row height override; 0 = auto
}

// AddCell appends a cell with the given text and returns it for further configuration.
func (r *RowBuilder) AddCell(text string) *CellBuilder {
	c := &CellBuilder{Text: text}
	r.cells = append(r.cells, c)
	return c
}

// WithMinHeight sets a minimum height for this row.
func (r *RowBuilder) WithMinHeight(h float64) *RowBuilder {
	r.minHeight = h
	return r
}

// Cells returns all cells in the row.
func (r *RowBuilder) Cells() []*CellBuilder {
	return r.cells
}

// WithBgColor applies a background color to every cell in the row.
func (r *RowBuilder) WithBgColor(red, g, b float64) *RowBuilder {
	for _, c := range r.cells {
		c.WithBgColor(red, g, b)
	}
	return r
}

// TableBuilder assembles rows for a table.
type TableBuilder struct {
	rows      []*RowBuilder
	colWidths []float64 // explicit column widths in points
	style     TableStyle
}

// NewTableBuilder creates a TableBuilder with explicit column widths.
// colWidths specifies the width of each column in points.
func NewTableBuilder(colWidths []float64, style TableStyle) *TableBuilder {
	style.fillDefaults()
	return &TableBuilder{colWidths: colWidths, style: style}
}

// AddRow appends a new row. Pass isHeader=true for header rows.
func (t *TableBuilder) AddRow(isHeader bool) *RowBuilder {
	r := &RowBuilder{isHeader: isHeader}
	t.rows = append(t.rows, r)
	return r
}

// TotalWidth returns the sum of all column widths.
func (t *TableBuilder) TotalWidth() float64 {
	total := 0.0
	for _, w := range t.colWidths {
		total += w
	}
	return total
}

// DrawTable draws the table with its top-left corner at (x, y) in the page's
// coordinate system (y increases upward). Returns the total height consumed.
func (pb *PageBuilder) DrawTable(x, y float64, t *TableBuilder) float64 {
	if len(t.rows) == 0 || len(t.colWidths) == 0 {
		return 0
	}

	st := t.style
	padding := st.CellPadding
	leading := st.FontSize * 1.2

	// Build span shadow map: spanMap[r][c] = true means this cell is covered by a span
	// starting at another cell and should be skipped during rendering.
	numRows := len(t.rows)
	numCols := len(t.colWidths)
	spanMap := make([][]bool, numRows)
	for i := range spanMap {
		spanMap[i] = make([]bool, numCols)
	}

	// ---- Pass 1: compute row heights ----
	rowHeights := make([]float64, numRows)
	for ri, row := range t.rows {
		fontSize := st.FontSize
		if row.isHeader {
			fontSize = st.HeaderFontSize
		}
		minH := row.minHeight
		if minH == 0 {
			minH = fontSize + padding*2
		}

		cellIdx := 0
		for ci := 0; ci < numCols; ci++ {
			if spanMap[ri][ci] {
				continue
			}
			if cellIdx >= len(row.cells) {
				break
			}
			cell := row.cells[cellIdx]
			cellIdx++

			colSpan := cell.ColSpan
			if colSpan < 1 {
				colSpan = 1
			}
			rowSpan := cell.RowSpan
			if rowSpan < 1 {
				rowSpan = 1
			}

			// Mark shadow cells
			for sr := ri; sr < ri+rowSpan && sr < numRows; sr++ {
				for sc := ci; sc < ci+colSpan && sc < numCols; sc++ {
					if sr == ri && sc == ci {
						continue
					}
					spanMap[sr][sc] = true
				}
			}

			// Compute effective cell width
			cellWidth := 0.0
			for cs := ci; cs < ci+colSpan && cs < numCols; cs++ {
				cellWidth += t.colWidths[cs]
			}
			innerWidth := cellWidth - padding*2

			// Estimate text height
			lines := wrapText(cell.Text, innerWidth, fontSize)
			textH := float64(len(lines))*leading + padding*2
			if textH < minH {
				textH = minH
			}

			// For multi-row spans, attribute height only to the span start row.
			// Single-row height is the text height; multi-row distributes.
			if rowSpan == 1 && textH > rowHeights[ri] {
				rowHeights[ri] = textH
			}
		}

		if rowHeights[ri] < minH {
			rowHeights[ri] = minH
		}
	}

	// Reset span map for pass 2
	for i := range spanMap {
		for j := range spanMap[i] {
			spanMap[i][j] = false
		}
	}

	// ---- Pass 2: draw cells ----
	cs := pb.content
	totalHeight := 0.0
	for _, h := range rowHeights {
		totalHeight += h
	}

	// curY tracks the top of the current row (PDF y increases upward).
	curY := y

	for ri, row := range t.rows {
		rowH := rowHeights[ri]
		rowBottom := curY - rowH

		fontSize := st.FontSize
		if row.isHeader {
			fontSize = st.HeaderFontSize
		}

		cellX := x
		cellIdx := 0

		for ci := 0; ci < numCols; ci++ {
			colW := t.colWidths[ci]
			if spanMap[ri][ci] {
				cellX += colW
				continue
			}
			if cellIdx >= len(row.cells) {
				cellX += colW
				continue
			}
			cell := row.cells[cellIdx]
			cellIdx++

			colSpan := cell.ColSpan
			if colSpan < 1 {
				colSpan = 1
			}
			rowSpan := cell.RowSpan
			if rowSpan < 1 {
				rowSpan = 1
			}

			// Mark shadow cells
			for sr := ri; sr < ri+rowSpan && sr < numRows; sr++ {
				for sc := ci; sc < ci+colSpan && sc < numCols; sc++ {
					if sr == ri && sc == ci {
						continue
					}
					spanMap[sr][sc] = true
				}
			}

			// Compute cell dimensions
			cellW := 0.0
			for cs2 := ci; cs2 < ci+colSpan && cs2 < numCols; cs2++ {
				cellW += t.colWidths[cs2]
			}
			cellH := 0.0
			for rs := ri; rs < ri+rowSpan && rs < numRows; rs++ {
				cellH += rowHeights[rs]
			}
			cellBottom := curY - cellH

			// Background fill
			var bgColor [3]float64
			if cell.BgColor != nil {
				bgColor = *cell.BgColor
			} else if row.isHeader {
				bgColor = st.HeaderFill
			}
			if bgColor != [3]float64{0, 0, 0} || cell.BgColor != nil || row.isHeader {
				cs.SaveState()
				cs.SetFillColorRGB(bgColor[0], bgColor[1], bgColor[2])
				cs.Rectangle(cellX, cellBottom, cellW, cellH)
				cs.Fill()
				cs.RestoreState()
			}

			// Cell border
			cs.SaveState()
			cs.SetStrokeColorRGB(st.BorderColor[0], st.BorderColor[1], st.BorderColor[2])
			cs.SetLineWidth(st.BorderWidth)
			cs.Rectangle(cellX, cellBottom, cellW, cellH)
			cs.Stroke()
			cs.RestoreState()

			// Text — draw inside padded region, top-aligned
			innerW := cellW - padding*2
			textX := cellX + padding
			textY := curY - padding - fontSize // baseline of first line

			lines := wrapText(cell.Text, innerW, fontSize)
			leading2 := fontSize * 1.2
			for li, line := range lines {
				lx := textX
				if cell.Align == TextAlignCenter {
					lw := approxTextWidth(line, fontSize)
					lx = textX + (innerW-lw)/2
				} else if cell.Align == TextAlignRight {
					lw := approxTextWidth(line, fontSize)
					lx = textX + innerW - lw
				}
				ly := textY - float64(li)*leading2
				cs.BeginText()
				cs.SetFont(st.FontName, fontSize)
				cs.SetTextMatrix(1, 0, 0, 1, lx, ly)
				cs.ShowText(line)
				cs.EndText()
			}

			cellX += colW
		}

		curY = rowBottom
		_ = rowBottom
	}

	return totalHeight
}
