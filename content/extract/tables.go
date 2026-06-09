package extract

import (
	"math"
	"sort"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// detectTables finds table structures from graphic lines and maps text into cells.
// Populates doc.Tables.
func detectTables(doc *types.ContentDocument) {
	for _, page := range doc.Pages {
		tables := detectTablesOnPage(&page)
		doc.Tables = append(doc.Tables, tables...)
	}
}

// detectTablesOnPage analyzes graphics on a single page to find table grids
func detectTablesOnPage(page *types.Page) []types.TableItem {
	if len(page.Graphics) == 0 {
		return nil
	}

	// Collect horizontal and vertical lines from graphics
	hLines, vLines := extractGridLines(page.Graphics, page.Height)

	if len(hLines) < 2 || len(vLines) < 2 {
		// Also try rectangle-based detection
		return detectTablesFromRects(page)
	}

	// Cluster lines by position
	hClusters := clusterValues(extractPositions(hLines, false), 2.0)
	vClusters := clusterValues(extractPositions(vLines, true), 2.0)

	if len(hClusters) < 3 || len(vClusters) < 3 {
		// Need at least 3 boundaries to form a 2×2 table
		return detectTablesFromRects(page)
	}

	// Sort clusters
	sort.Float64s(hClusters) // bottom to top in PDF coords
	sort.Float64s(vClusters) // left to right

	numRows := len(hClusters) - 1
	numCols := len(vClusters) - 1

	if numRows < 2 || numCols < 2 {
		return nil
	}

	// Map text into cells
	cells := mapTextToCells(page.Text, hClusters, vClusters, page.PageNumber)

	// Detect merged cells from graphic rectangles that span multiple grid positions
	cells = detectMergedCells(cells, page.Graphics, hClusters, vClusters)

	// Calculate bounding box
	bbox := &types.SemanticBBox{
		Left:   vClusters[0],
		Right:  vClusters[len(vClusters)-1],
		Bottom: hClusters[0],
		Top:    hClusters[len(hClusters)-1],
		Page:   page.PageNumber,
	}

	table := types.TableItem{
		Cells:   cells,
		NumRows: numRows,
		NumCols: numCols,
		BBox:    bbox,
	}

	// Detect header rows from filled background rectangles
	table.HeaderRows = detectHeaderRows(page.Graphics, hClusters, vClusters)
	if table.HeaderRows > 0 {
		for i := range table.Cells {
			if table.Cells[i].Row < table.HeaderRows {
				table.Cells[i].IsHeader = true
			}
		}
	}

	// Look for caption
	table.Caption = findTableCaption(page, bbox)

	return []types.TableItem{table}
}

// ---- Line extraction ----

type gridLine struct {
	x1, y1, x2, y2 float64
}

func (l gridLine) isHorizontal(tolerance float64) bool {
	return math.Abs(l.y1-l.y2) < tolerance
}

func (l gridLine) isVertical(tolerance float64) bool {
	return math.Abs(l.x1-l.x2) < tolerance
}

func (l gridLine) length() float64 {
	dx := l.x2 - l.x1
	dy := l.y2 - l.y1
	return math.Sqrt(dx*dx + dy*dy)
}

// extractGridLines pulls out horizontal and vertical lines from graphics.
// Lines shorter than minLineLen are ignored to reduce false positives.
func extractGridLines(graphics []types.Graphic, pageHeight float64) (hLines, vLines []gridLine) {
	const tolerance = 1.5
	const minLineLen = 10.0

	for _, g := range graphics {
		// Direct line graphics
		if g.Type == types.GraphicTypeLine && g.Path != nil {
			lines := pathToLines(g.Path.Operations)
			for _, line := range lines {
				if line.length() < minLineLen {
					continue
				}
				if line.isHorizontal(tolerance) {
					hLines = append(hLines, line)
				} else if line.isVertical(tolerance) {
					vLines = append(vLines, line)
				}
			}
			continue
		}

		// Path-based graphics
		if g.Path != nil {
			lines := pathToLines(g.Path.Operations)
			for _, line := range lines {
				if line.length() < minLineLen {
					continue
				}
				if line.isHorizontal(tolerance) {
					hLines = append(hLines, line)
				} else if line.isVertical(tolerance) {
					vLines = append(vLines, line)
				}
			}
			continue
		}

		// Rectangles can form cell borders
		if g.Type == types.GraphicTypeRectangle && g.BoundingBox != nil {
			bb := g.BoundingBox
			w := bb.UpperX - bb.LowerX
			h := bb.UpperY - bb.LowerY

			// Thin rectangles act as lines
			if h < 3 && w > minLineLen {
				y := (bb.LowerY + bb.UpperY) / 2
				hLines = append(hLines, gridLine{bb.LowerX, y, bb.UpperX, y})
			} else if w < 3 && h > minLineLen {
				x := (bb.LowerX + bb.UpperX) / 2
				vLines = append(vLines, gridLine{x, bb.LowerY, x, bb.UpperY})
			} else if w > minLineLen && h > minLineLen {
				// Full rectangle — decompose into 4 lines
				hLines = append(hLines,
					gridLine{bb.LowerX, bb.LowerY, bb.UpperX, bb.LowerY}, // bottom
					gridLine{bb.LowerX, bb.UpperY, bb.UpperX, bb.UpperY}, // top
				)
				vLines = append(vLines,
					gridLine{bb.LowerX, bb.LowerY, bb.LowerX, bb.UpperY}, // left
					gridLine{bb.UpperX, bb.LowerY, bb.UpperX, bb.UpperY}, // right
				)
			}
		}
	}

	return hLines, vLines
}

// pathToLines extracts line segments from path operations
func pathToLines(ops []types.PathOperation) []gridLine {
	var lines []gridLine
	var curX, curY float64
	var moveX, moveY float64

	for _, op := range ops {
		switch op.Type {
		case types.PathOpMove:
			if len(op.Points) >= 1 {
				curX, curY = op.Points[0].X, op.Points[0].Y
				moveX, moveY = curX, curY
			}
		case types.PathOpLine:
			if len(op.Points) >= 1 {
				lines = append(lines, gridLine{curX, curY, op.Points[0].X, op.Points[0].Y})
				curX, curY = op.Points[0].X, op.Points[0].Y
			}
		case types.PathOpClose:
			if curX != moveX || curY != moveY {
				lines = append(lines, gridLine{curX, curY, moveX, moveY})
				curX, curY = moveX, moveY
			}
		case types.PathOpRectangle:
			if len(op.Points) >= 2 {
				x, y := op.Points[0].X, op.Points[0].Y
				w, h := op.Points[1].X, op.Points[1].Y
				lines = append(lines,
					gridLine{x, y, x + w, y},
					gridLine{x + w, y, x + w, y + h},
					gridLine{x + w, y + h, x, y + h},
					gridLine{x, y + h, x, y},
				)
			}
		}
	}
	return lines
}

// ---- Clustering ----

func extractPositions(lines []gridLine, vertical bool) []float64 {
	var vals []float64
	for _, l := range lines {
		if vertical {
			vals = append(vals, (l.x1+l.x2)/2)
		} else {
			vals = append(vals, (l.y1+l.y2)/2)
		}
	}
	return vals
}

// clusterValues groups float values into clusters, returning the mean of each cluster
func clusterValues(vals []float64, tolerance float64) []float64 {
	if len(vals) == 0 {
		return nil
	}

	sort.Float64s(vals)

	var clusters [][]float64
	current := []float64{vals[0]}

	for _, v := range vals[1:] {
		if v-current[len(current)-1] <= tolerance {
			current = append(current, v)
		} else {
			clusters = append(clusters, current)
			current = []float64{v}
		}
	}
	clusters = append(clusters, current)

	var result []float64
	for _, c := range clusters {
		sum := 0.0
		for _, v := range c {
			sum += v
		}
		result = append(result, sum/float64(len(c)))
	}
	return result
}

// ---- Cell mapping ----

func mapTextToCells(textElements []types.TextElement, hClusters, vClusters []float64, pageNum int) []types.TableCell {
	numRows := len(hClusters) - 1
	numCols := len(vClusters) - 1

	// Build grid of string builders
	grid := make([][]strings.Builder, numRows)
	for i := range grid {
		grid[i] = make([]strings.Builder, numCols)
	}

	for _, el := range textElements {
		// Find which cell this element belongs to
		col := -1
		for c := 0; c < numCols; c++ {
			if el.X >= vClusters[c]-2 && el.X < vClusters[c+1]+2 {
				col = c
				break
			}
		}

		// Row: hClusters are sorted bottom-to-top; rows are top-to-bottom
		row := -1
		for r := 0; r < numRows; r++ {
			// Row r spans from hClusters[numRows-r] (top) to hClusters[numRows-r-1] (bottom)
			top := hClusters[numRows-r]
			bottom := hClusters[numRows-r-1]
			if el.Y <= top+2 && el.Y >= bottom-2 {
				row = r
				break
			}
		}

		if row >= 0 && col >= 0 {
			if grid[row][col].Len() > 0 {
				grid[row][col].WriteByte(' ')
			}
			grid[row][col].WriteString(el.Text)
		}
	}

	var cells []types.TableCell
	for r := 0; r < numRows; r++ {
		for c := 0; c < numCols; c++ {
			text := strings.TrimSpace(grid[r][c].String())
			if text != "" {
				cells = append(cells, types.TableCell{
					Text: text,
					Row:  r,
					Col:  c,
				})
			}
		}
	}
	return cells
}

// ---- Merged cell detection ----

// detectMergedCells examines filled rectangles that span multiple grid positions
// and sets ColSpan/RowSpan on affected cells accordingly.
func detectMergedCells(cells []types.TableCell, graphics []types.Graphic, hClusters, vClusters []float64) []types.TableCell {
	numRows := len(hClusters) - 1
	numCols := len(vClusters) - 1
	if numRows < 2 || numCols < 2 {
		return cells
	}

	// Build cell lookup for quick updates
	type cellKey struct{ row, col int }
	index := make(map[cellKey]int, len(cells))
	for i, c := range cells {
		index[cellKey{c.Row, c.Col}] = i
	}

	for _, g := range graphics {
		if g.BoundingBox == nil {
			continue
		}
		bb := g.BoundingBox
		w := bb.UpperX - bb.LowerX
		h := bb.UpperY - bb.LowerY
		if w < 5 || h < 5 {
			continue
		}

		// Determine which grid columns this rect spans
		colStart, colEnd := -1, -1
		for c := 0; c < numCols; c++ {
			if bb.LowerX <= vClusters[c]+3 && colStart < 0 {
				colStart = c
			}
			if bb.UpperX >= vClusters[c+1]-3 {
				colEnd = c
			}
		}

		// Determine which grid rows this rect spans (PDF Y increases upward)
		rowStart, rowEnd := -1, -1
		for r := 0; r < numRows; r++ {
			top := hClusters[numRows-r]
			bottom := hClusters[numRows-r-1]
			if bb.UpperY >= top-3 && rowStart < 0 {
				rowStart = r
			}
			if bb.LowerY <= bottom+3 {
				rowEnd = r
			}
		}

		if colStart < 0 || colEnd < colStart || rowStart < 0 || rowEnd < rowStart {
			continue
		}

		colSpan := colEnd - colStart + 1
		rowSpan := rowEnd - rowStart + 1
		if colSpan <= 1 && rowSpan <= 1 {
			continue
		}

		// Apply span to the top-left cell of the merged region
		key := cellKey{rowStart, colStart}
		if idx, ok := index[key]; ok {
			if colSpan > 1 {
				cells[idx].ColSpan = colSpan
			}
			if rowSpan > 1 {
				cells[idx].RowSpan = rowSpan
			}
		}
	}

	return cells
}

// ---- Header row detection ----

// detectHeaderRows returns the number of leading rows that appear to be header rows.
// It looks for filled (non-white) rectangles that cover the top rows of the grid.
func detectHeaderRows(graphics []types.Graphic, hClusters, vClusters []float64) int {
	numRows := len(hClusters) - 1
	if numRows < 2 {
		return 0
	}

	tableLeft := vClusters[0]
	tableRight := vClusters[len(vClusters)-1]
	tableWidth := tableRight - tableLeft

	for r := 0; r < numRows && r < 3; r++ {
		rowTop := hClusters[numRows-r]
		rowBottom := hClusters[numRows-r-1]
		rowHeight := rowTop - rowBottom

		for _, g := range graphics {
			if g.FillColor == nil || isWhiteOrNone(g.FillColor) {
				continue
			}
			if g.BoundingBox == nil {
				continue
			}
			bb := g.BoundingBox
			// Check if the filled rect covers this row substantially
			if bb.LowerY <= rowBottom+2 && bb.UpperY >= rowTop-2 &&
				bb.LowerX <= tableLeft+5 && bb.UpperX >= tableRight-5 {
				// This filled rect spans the full row width — it's a header background
				_ = rowHeight
				_ = tableWidth
				return r + 1
			}
		}
	}

	return 0
}

// isWhiteOrNone returns true if the color is white or near-white (not a real fill).
func isWhiteOrNone(c *types.Color) bool {
	if c == nil {
		return true
	}
	// Treat near-white as transparent background
	return c.R > 0.95 && c.G > 0.95 && c.B > 0.95
}

// ---- Rectangle-based table detection ----

// detectTablesFromRects tries to detect tables from aligned rectangles
func detectTablesFromRects(page *types.Page) []types.TableItem {
	var rects []types.Rectangle
	for _, g := range page.Graphics {
		if g.Type == types.GraphicTypeRectangle && g.BoundingBox != nil {
			bb := *g.BoundingBox
			w := bb.UpperX - bb.LowerX
			h := bb.UpperY - bb.LowerY
			// Only consider rectangles that could be table cells (reasonable size)
			if w > 10 && h > 5 && w < 500 && h < 200 {
				rects = append(rects, bb)
			}
		}
	}

	if len(rects) < 4 {
		return nil
	}

	// Check if rectangles form a grid pattern
	// Group by similar Y positions (rows) and X positions (columns)
	yPositions := make([]float64, len(rects))
	xPositions := make([]float64, len(rects))
	for i, r := range rects {
		yPositions[i] = r.LowerY
		xPositions[i] = r.LowerX
	}

	yClusters := clusterValues(yPositions, 3.0)
	xClusters := clusterValues(xPositions, 3.0)

	if len(yClusters) < 2 || len(xClusters) < 2 {
		return nil
	}

	// We have a grid pattern — extract horizontal/vertical boundaries from rects
	var hBounds, vBounds []float64
	for _, r := range rects {
		hBounds = append(hBounds, r.LowerY, r.UpperY)
		vBounds = append(vBounds, r.LowerX, r.UpperX)
	}

	hClusters := clusterValues(hBounds, 2.0)
	vClusters := clusterValues(vBounds, 2.0)

	if len(hClusters) < 3 || len(vClusters) < 3 {
		return nil
	}

	sort.Float64s(hClusters)
	sort.Float64s(vClusters)

	numRows := len(hClusters) - 1
	numCols := len(vClusters) - 1

	if numRows < 2 || numCols < 2 {
		return nil
	}

	cells := mapTextToCells(page.Text, hClusters, vClusters, page.PageNumber)
	cells = detectMergedCells(cells, page.Graphics, hClusters, vClusters)

	bbox := &types.SemanticBBox{
		Left:   vClusters[0],
		Right:  vClusters[len(vClusters)-1],
		Bottom: hClusters[0],
		Top:    hClusters[len(hClusters)-1],
		Page:   page.PageNumber,
	}

	table := types.TableItem{
		Cells:   cells,
		NumRows: numRows,
		NumCols: numCols,
		BBox:    bbox,
	}

	table.HeaderRows = detectHeaderRows(page.Graphics, hClusters, vClusters)
	if table.HeaderRows > 0 {
		for i := range table.Cells {
			if table.Cells[i].Row < table.HeaderRows {
				table.Cells[i].IsHeader = true
			}
		}
	}

	table.Caption = findTableCaption(page, bbox)

	return []types.TableItem{table}
}

// findTableCaption looks for text near the table that could be a caption
func findTableCaption(page *types.Page, tableBBox *types.SemanticBBox) string {
	if tableBBox == nil {
		return ""
	}

	// Look for text just above or below the table
	tolerance := 20.0 // points

	var candidates []string
	for _, el := range page.Text {
		// Check if text is near the top of the table
		if math.Abs(el.Y-tableBBox.Top) < tolerance && el.X >= tableBBox.Left-10 {
			if looksLikeCaption(el.Text) {
				candidates = append(candidates, el.Text)
			}
		}
		// Check below
		if math.Abs(el.Y-tableBBox.Bottom) < tolerance && el.X >= tableBBox.Left-10 {
			if looksLikeCaption(el.Text) {
				candidates = append(candidates, el.Text)
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func looksLikeCaption(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(lower, "table") ||
		strings.HasPrefix(lower, "fig") ||
		strings.HasPrefix(lower, "figure")
}
