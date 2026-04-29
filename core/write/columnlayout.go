package write

// ColumnLayout is a multi-column positioning helper.
// It tracks the current Y cursor in each column so callers can place
// content (text boxes, tables, images) sequentially within columns.
type ColumnLayout struct {
	page      *PageBuilder
	cols      int
	colX      []float64 // left edge of each column
	colWidth  float64   // width of each column (uniform)
	cursorY   []float64 // current top-of-content Y for each column (decreases as content is added)
	top       float64   // initial top Y
}

// NewColumnLayout creates a column layout within [x, x+width] starting at y.
// cols is the number of columns; gutter is the space between them.
// y is the top of the layout area (PDF coordinate — y increases upward).
func (pb *PageBuilder) NewColumnLayout(x, y, width float64, cols int, gutter float64) *ColumnLayout {
	if cols < 1 {
		cols = 1
	}
	totalGutter := gutter * float64(cols-1)
	colW := (width - totalGutter) / float64(cols)
	if colW < 0 {
		colW = 0
	}

	colX := make([]float64, cols)
	for i := 0; i < cols; i++ {
		colX[i] = x + float64(i)*(colW+gutter)
	}

	cursors := make([]float64, cols)
	for i := range cursors {
		cursors[i] = y
	}

	return &ColumnLayout{
		page:     pb,
		cols:     cols,
		colX:     colX,
		colWidth: colW,
		cursorY:  cursors,
		top:      y,
	}
}

// X returns the left edge of column i.
func (cl *ColumnLayout) X(i int) float64 {
	if i < 0 || i >= cl.cols {
		return cl.colX[0]
	}
	return cl.colX[i]
}

// Width returns the width of each column.
func (cl *ColumnLayout) Width() float64 {
	return cl.colWidth
}

// CurrentY returns the current cursor Y for column i.
// Content should start at this Y and extend downward.
func (cl *ColumnLayout) CurrentY(i int) float64 {
	if i < 0 || i >= cl.cols {
		return cl.cursorY[0]
	}
	return cl.cursorY[i]
}

// Advance moves the cursor of column i down by height points.
func (cl *ColumnLayout) Advance(i int, height float64) {
	if i < 0 || i >= cl.cols {
		return
	}
	cl.cursorY[i] -= height
}

// DrawTextBox draws word-wrapped text in column i starting at the current
// cursor position, then advances the cursor by the height used.
// gap adds extra vertical space below the text block.
func (cl *ColumnLayout) DrawTextBox(col int, text string, style TextBoxStyle, gap float64) float64 {
	x := cl.X(col)
	y := cl.CurrentY(col)
	h := cl.page.DrawTextBox(x, y, cl.colWidth, text, style)
	cl.Advance(col, h+gap)
	return h
}

// DrawTable draws a table in column i (or spanning multiple columns if the
// table is wider than one column) starting at the current cursor of column i,
// then advances that cursor by the height consumed.
func (cl *ColumnLayout) DrawTable(col int, t *TableBuilder, gap float64) float64 {
	x := cl.X(col)
	y := cl.CurrentY(col)
	h := cl.page.DrawTable(x, y, t)
	cl.Advance(col, h+gap)
	return h
}

// BalanceColumns sets all cursors to the minimum (lowest) value among all columns.
// This is useful after filling multiple columns to bring them to a common baseline.
func (cl *ColumnLayout) BalanceColumns() {
	minY := cl.cursorY[0]
	for _, y := range cl.cursorY[1:] {
		if y < minY {
			minY = y
		}
	}
	for i := range cl.cursorY {
		cl.cursorY[i] = minY
	}
}

// LowestY returns the lowest (smallest) Y across all column cursors.
func (cl *ColumnLayout) LowestY() float64 {
	minY := cl.cursorY[0]
	for _, y := range cl.cursorY[1:] {
		if y < minY {
			minY = y
		}
	}
	return minY
}

// Reset sets all cursors back to the initial top Y.
func (cl *ColumnLayout) Reset() {
	for i := range cl.cursorY {
		cl.cursorY[i] = cl.top
	}
}
