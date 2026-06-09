package extract

import (
	"math"
	"sort"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// ---- Intermediate types (package-internal) ----

// textLine groups TextElements that share the same visual line
type textLine struct {
	Elements []types.TextElement
	Y        float64 // representative Y position (average)
	MinX     float64
	MaxX     float64
	FontSize float64 // dominant (most frequent) font size
	Text     string  // concatenated text
	Page     int
}

// textColumn represents a vertical column of text lines
type textColumn struct {
	Lines []textLine
	MinX  float64
	MaxX  float64
}

// ---- Public analysis entry point ----

// analyzeLayout sorts elements into reading order, groups into lines, and detects columns.
// Modifies doc.Pages[].Text in place to reflect reading order.
func analyzeLayout(doc *types.ContentDocument) {
	for i := range doc.Pages {
		page := &doc.Pages[i]
		if len(page.Text) == 0 {
			continue
		}

		// Sort into reading order
		sortReadingOrder(page.Text, page.Height)

		// Group into lines
		lines := groupLines(page.Text, page.PageNumber)

		// Detect columns and interleave reading order
		columns := detectColumns(lines, page.Width)
		ordered := columnReadingOrder(columns)

		// Flatten back to TextElements in reading order
		var reordered []types.TextElement
		for _, line := range ordered {
			reordered = append(reordered, line.Elements...)
		}
		page.Text = reordered
	}
}

// ---- Reading order sort ----

// sortReadingOrder sorts TextElements by Y (descending, top to bottom in visual space)
// then by X (left to right). PDF origin is bottom-left, so larger Y = higher on page.
func sortReadingOrder(elements []types.TextElement, pageHeight float64) {
	sort.SliceStable(elements, func(i, j int) bool {
		a, b := elements[i], elements[j]
		// Same line tolerance based on larger font
		tolerance := math.Max(a.FontSize, b.FontSize) * 0.3
		if tolerance == 0 {
			tolerance = 3.0
		}
		// Higher Y = higher on page, so sort descending
		if math.Abs(a.Y-b.Y) > tolerance {
			return a.Y > b.Y
		}
		return a.X < b.X
	})
}

// ---- Line grouping ----

// groupLines clusters TextElements into lines based on Y-proximity
func groupLines(elements []types.TextElement, pageNum int) []textLine {
	if len(elements) == 0 {
		return nil
	}

	var lines []textLine
	var currentElements []types.TextElement
	var currentY float64

	for i, el := range elements {
		if i == 0 {
			currentElements = []types.TextElement{el}
			currentY = el.Y
			continue
		}

		// Check if this element is on the same line
		tolerance := math.Max(el.FontSize, currentFontSize(currentElements)) * 0.3
		if tolerance == 0 {
			tolerance = 3.0
		}

		if math.Abs(el.Y-currentY) <= tolerance {
			currentElements = append(currentElements, el)
		} else {
			// Finish current line
			lines = append(lines, buildLine(currentElements, pageNum))
			currentElements = []types.TextElement{el}
			currentY = el.Y
		}
	}
	if len(currentElements) > 0 {
		lines = append(lines, buildLine(currentElements, pageNum))
	}

	return lines
}

func buildLine(elements []types.TextElement, pageNum int) textLine {
	// Sort elements left to right within the line
	sort.SliceStable(elements, func(i, j int) bool {
		return elements[i].X < elements[j].X
	})

	minX := elements[0].X
	maxX := elements[len(elements)-1].X + elements[len(elements)-1].Width
	avgY := 0.0
	for _, el := range elements {
		avgY += el.Y
	}
	avgY /= float64(len(elements))

	// Build concatenated text with spaces between non-adjacent elements
	var b strings.Builder
	for i, el := range elements {
		if i > 0 {
			prevEnd := elements[i-1].X + elements[i-1].Width
			gap := el.X - prevEnd
			fontSize := math.Max(el.FontSize, elements[i-1].FontSize)
			if fontSize == 0 {
				fontSize = 12
			}

			needsSpace := gap > fontSize*0.25

			// Fallback: when computed width overshoots (gap ≤ 0.1×fontSize),
			// check position-to-position distance to infer missing space.
			// This handles CID fonts where width data is unavailable.
			if !needsSpace && gap < fontSize*0.1 {
				posGap := el.X - elements[i-1].X
				prevCharCount := float64(len([]rune(elements[i-1].Text)))
				if prevCharCount > 0 {
					impliedCharWidth := posGap / prevCharCount
					if impliedCharWidth > fontSize*0.65 {
						needsSpace = true
					}
				}
			}

			if needsSpace {
				if !strings.HasSuffix(elements[i-1].Text, " ") && !strings.HasPrefix(el.Text, " ") {
					b.WriteByte(' ')
				}
			}
		}
		b.WriteString(el.Text)
	}

	return textLine{
		Elements: elements,
		Y:        avgY,
		MinX:     minX,
		MaxX:     maxX,
		FontSize: dominantFontSize(elements),
		Text:     collapseSpaces(b.String()),
		Page:     pageNum,
	}
}

// collapseSpaces replaces runs of multiple spaces with a single space
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

func currentFontSize(elements []types.TextElement) float64 {
	if len(elements) == 0 {
		return 12
	}
	return elements[len(elements)-1].FontSize
}

func dominantFontSize(elements []types.TextElement) float64 {
	if len(elements) == 0 {
		return 12
	}
	freq := make(map[float64]int)
	for _, el := range elements {
		// Round to 0.5pt for bucketing
		key := math.Round(el.FontSize*2) / 2
		freq[key]++
	}
	best := 0.0
	bestCount := 0
	for size, count := range freq {
		if count > bestCount {
			best = size
			bestCount = count
		}
	}
	return best
}

// ---- Column detection ----

// detectColumns identifies columns by finding large X-gaps between lines.
// Lines spanning >70% of page width are separated as "spanning" lines
// and excluded from column clustering to avoid mis-assignment.
func detectColumns(lines []textLine, pageWidth float64) []textColumn {
	if len(lines) == 0 {
		return nil
	}

	// Use a gap-based approach: collect all line MinX values and look for clusters
	// Heuristic: if lines clearly split into 2+ X ranges with a gap > pageWidth/6
	minGapThreshold := pageWidth / 6
	if minGapThreshold < 50 {
		minGapThreshold = 50
	}

	// First pass: cluster all lines to detect if multi-column
	cols := clusterLinesByX(lines, minGapThreshold)

	// If we have 2+ columns, separate full-width lines and re-cluster
	if len(cols) >= 2 && pageWidth > 0 {
		spanThreshold := pageWidth * 0.70
		var narrowLines, spanningLines []textLine
		for _, line := range lines {
			lineWidth := line.MaxX - line.MinX
			if lineWidth > spanThreshold {
				spanningLines = append(spanningLines, line)
			} else {
				narrowLines = append(narrowLines, line)
			}
		}

		if len(spanningLines) > 0 && len(narrowLines) > 0 {
			// Re-cluster with only narrow lines
			cols = clusterLinesByX(narrowLines, minGapThreshold)

			// Add spanning lines as a special column (MinX=0 signals spanning)
			spanCol := textColumn{Lines: spanningLines, MinX: -1, MaxX: -1}
			sort.SliceStable(spanCol.Lines, func(a, b int) bool {
				return spanCol.Lines[a].Y > spanCol.Lines[b].Y
			})
			cols = append(cols, spanCol)
		}
	}

	return cols
}

// clusterLinesByX groups lines into column clusters by X position
func clusterLinesByX(lines []textLine, minGapThreshold float64) []textColumn {
	type xRange struct {
		minX, maxX float64
	}
	var ranges []xRange
	for _, l := range lines {
		ranges = append(ranges, xRange{l.MinX, l.MaxX})
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].minX < ranges[j].minX
	})

	type colRange struct {
		minX, maxX float64
	}
	var cols []colRange
	cols = append(cols, colRange{ranges[0].minX, ranges[0].maxX})

	for _, r := range ranges[1:] {
		merged := false
		for i := range cols {
			if r.minX <= cols[i].maxX+minGapThreshold/2 {
				if r.maxX > cols[i].maxX {
					cols[i].maxX = r.maxX
				}
				if r.minX < cols[i].minX {
					cols[i].minX = r.minX
				}
				merged = true
				break
			}
		}
		if !merged {
			cols = append(cols, colRange{r.minX, r.maxX})
		}
	}

	result := make([]textColumn, len(cols))
	for i, c := range cols {
		result[i].MinX = c.minX
		result[i].MaxX = c.maxX
	}

	for _, line := range lines {
		midX := (line.MinX + line.MaxX) / 2
		bestCol := 0
		bestDist := math.MaxFloat64
		for i, c := range cols {
			colMid := (c.minX + c.maxX) / 2
			dist := math.Abs(midX - colMid)
			if dist < bestDist {
				bestDist = dist
				bestCol = i
			}
		}
		result[bestCol].Lines = append(result[bestCol].Lines, line)
	}

	for i := range result {
		sort.SliceStable(result[i].Lines, func(a, b int) bool {
			return result[i].Lines[a].Y > result[i].Lines[b].Y
		})
	}

	return result
}

// columnReadingOrder produces the final line order with appropriate reading strategy.
// For 2-column layouts, interleaves by Y-band; otherwise concatenates left to right.
// Spanning lines (MinX == -1) are interleaved at their Y position.
func columnReadingOrder(columns []textColumn) []textLine {
	// Separate spanning columns from regular columns
	var regularCols []textColumn
	var spanningLines []textLine
	for _, col := range columns {
		if col.MinX == -1 {
			spanningLines = append(spanningLines, col.Lines...)
		} else {
			regularCols = append(regularCols, col)
		}
	}

	// Sort regular columns left to right
	sort.Slice(regularCols, func(i, j int) bool {
		return regularCols[i].MinX < regularCols[j].MinX
	})

	// Get columnar content in reading order
	var columnarLines []textLine
	if len(regularCols) == 2 {
		columnarLines = interleaveTwoColumns(regularCols[0], regularCols[1])
	} else {
		for _, col := range regularCols {
			columnarLines = append(columnarLines, col.Lines...)
		}
	}

	// If no spanning lines, return columnar lines directly
	if len(spanningLines) == 0 {
		return columnarLines
	}

	// Interleave spanning lines at the correct vertical position
	return interleaveSpanningLines(columnarLines, spanningLines)
}

// interleaveSpanningLines merges spanning (full-width) lines into the columnar
// reading order at their correct vertical position.
func interleaveSpanningLines(columnar, spanning []textLine) []textLine {
	var result []textLine
	si := 0
	ci := 0
	for si < len(spanning) && ci < len(columnar) {
		// Higher Y = higher on page in PDF coords, so emit higher-Y first
		if spanning[si].Y > columnar[ci].Y {
			result = append(result, spanning[si])
			si++
		} else {
			result = append(result, columnar[ci])
			ci++
		}
	}
	for si < len(spanning) {
		result = append(result, spanning[si])
		si++
	}
	for ci < len(columnar) {
		result = append(result, columnar[ci])
		ci++
	}
	return result
}

// interleaveTwoColumns merges two columns by Y position using a two-pointer merge.
// Lines at the same Y-band get left-then-right ordering; otherwise higher Y goes first.
func interleaveTwoColumns(left, right textColumn) []textLine {
	// Check if both columns span similar vertical range — if so, read left fully then right
	leftRange := columnYRange(left)
	rightRange := columnYRange(right)

	overlap := math.Min(leftRange[0], rightRange[0]) - math.Max(leftRange[1], rightRange[1])
	leftSpan := leftRange[0] - leftRange[1]
	rightSpan := rightRange[0] - rightRange[1]
	maxSpan := math.Max(leftSpan, rightSpan)

	// If columns overlap by >50% of the taller column, use standard left-then-right
	if maxSpan > 0 && overlap/maxSpan > 0.5 {
		var result []textLine
		result = append(result, left.Lines...)
		result = append(result, right.Lines...)
		return result
	}

	// Otherwise, interleave by Y — same Y-band: left first
	var result []textLine
	li, ri := 0, 0
	for li < len(left.Lines) && ri < len(right.Lines) {
		lY := left.Lines[li].Y
		rY := right.Lines[ri].Y
		tolerance := math.Max(left.Lines[li].FontSize, right.Lines[ri].FontSize) * 0.5
		if tolerance == 0 {
			tolerance = 3.0
		}

		if math.Abs(lY-rY) <= tolerance {
			result = append(result, left.Lines[li])
			result = append(result, right.Lines[ri])
			li++
			ri++
		} else if lY > rY {
			result = append(result, left.Lines[li])
			li++
		} else {
			result = append(result, right.Lines[ri])
			ri++
		}
	}
	for li < len(left.Lines) {
		result = append(result, left.Lines[li])
		li++
	}
	for ri < len(right.Lines) {
		result = append(result, right.Lines[ri])
		ri++
	}
	return result
}

func columnYRange(col textColumn) [2]float64 {
	if len(col.Lines) == 0 {
		return [2]float64{0, 0}
	}
	maxY := col.Lines[0].Y
	minY := col.Lines[len(col.Lines)-1].Y
	return [2]float64{maxY, minY}
}

// ---- Exported helpers for semantic analysis ----

// groupPageLines groups a page's text elements into textLines.
// Exported for use by semantic.go.
func groupPageLines(page *types.Page) []textLine {
	if len(page.Text) == 0 {
		return nil
	}
	return groupLines(page.Text, page.PageNumber)
}
