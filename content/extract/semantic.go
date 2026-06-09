package extract

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/benedoc-inc/pdfer/v2/types"
)

type textPos struct {
	text string
	y    float64
}

// analyzeSemantics runs after layout analysis. Groups lines into paragraphs,
// detects headings, list items, captions, and generates WordCells.
// Populates doc.TextItems.
func analyzeSemantics(doc *types.ContentDocument) {
	// First pass: build TextItems per page
	bodyFontSize := computeBodyFontSize(doc)

	for i := range doc.Pages {
		page := &doc.Pages[i]
		lines := groupPageLines(page)
		if len(lines) == 0 {
			continue
		}

		items := buildTextItems(lines, bodyFontSize, page.PageNumber)
		doc.TextItems = append(doc.TextItems, items...)
	}

	// Generate PictureItems from image refs
	for _, page := range doc.Pages {
		for _, img := range page.Images {
			doc.Pictures = append(doc.Pictures, types.PictureItem{
				BBox: &types.SemanticBBox{
					Left:   img.X,
					Right:  img.X + img.Width,
					Bottom: img.Y,
					Top:    img.Y + img.Height,
					Page:   page.PageNumber,
				},
			})
		}
	}
}

// detectHeadersFooters looks for repeated text across pages in the top/bottom margins.
// Re-labels matching TextItems as page_header or page_footer.
func detectHeadersFooters(doc *types.ContentDocument) {
	if len(doc.Pages) < 3 || len(doc.TextItems) == 0 {
		return
	}

	// Compute page height from first page
	pageHeight := 792.0 // default letter
	if len(doc.Pages) > 0 && doc.Pages[0].Height > 0 {
		pageHeight = doc.Pages[0].Height
	}

	headerZone := pageHeight * 0.95 // top 5%
	footerZone := pageHeight * 0.05 // bottom 5%

	// Collect header/footer candidates per page
	headersByPage := make(map[int][]textPos)
	footersByPage := make(map[int][]textPos)

	for _, item := range doc.TextItems {
		if item.BBox == nil {
			continue
		}
		page := item.BBox.Page
		y := item.BBox.Top

		if y >= headerZone {
			headersByPage[page] = append(headersByPage[page], textPos{normalizeForMatch(item.Text), y})
		}
		if item.BBox.Bottom <= footerZone {
			footersByPage[page] = append(footersByPage[page], textPos{normalizeForMatch(item.Text), item.BBox.Bottom})
		}
	}

	// Find text patterns that appear on 3+ pages
	headerPatterns := findRepeatedPatterns(headersByPage, 3)
	footerPatterns := findRepeatedPatterns(footersByPage, 3)

	// Re-label matching TextItems
	for i := range doc.TextItems {
		item := &doc.TextItems[i]
		if item.BBox == nil {
			continue
		}
		normalized := normalizeForMatch(item.Text)
		if item.BBox.Top >= headerZone && headerPatterns[normalized] {
			item.Label = types.TextLabelPageHeader
		}
		if item.BBox.Bottom <= footerZone && footerPatterns[normalized] {
			item.Label = types.TextLabelPageFooter
		}
	}
}

// ---- Body font size detection ----

// computeBodyFontSize finds the most common font size across the document.
// On ties, prefers smaller sizes (body text is typically smaller than headings).
func computeBodyFontSize(doc *types.ContentDocument) float64 {
	freq := make(map[float64]int)
	for _, page := range doc.Pages {
		for _, el := range page.Text {
			if el.FontSize > 0 {
				key := math.Round(el.FontSize*2) / 2
				freq[key]++
			}
		}
	}

	if len(freq) == 0 {
		return 12
	}

	// Find the most frequent font size; on ties, prefer smaller
	bestSize := 0.0
	bestCount := 0
	for size, count := range freq {
		if count > bestCount || (count == bestCount && (bestSize == 0 || size < bestSize)) {
			bestSize = size
			bestCount = count
		}
	}
	return bestSize
}

// ---- Paragraph and heading detection ----

func buildTextItems(lines []textLine, bodyFontSize float64, pageNum int) []types.TextItem {
	if len(lines) == 0 {
		return nil
	}

	// Compute median line spacing for paragraph detection
	medianSpacing := computeMedianLineSpacing(lines)

	var items []types.TextItem
	var paragraphLines []textLine

	flushParagraph := func() {
		if len(paragraphLines) == 0 {
			return
		}
		item := linesToTextItem(paragraphLines, bodyFontSize, pageNum)
		items = append(items, item)
		paragraphLines = nil
	}

	for i, line := range lines {
		// Check if this line starts a new paragraph
		if i > 0 && medianSpacing > 0 {
			gap := lines[i-1].Y - line.Y // positive = line is below prev (in PDF coords)
			if gap > medianSpacing*1.5 {
				flushParagraph()
			}
		}

		// Check if this line is a heading (should be its own item)
		if isHeadingLine(line, bodyFontSize, lines, i) {
			flushParagraph()
			item := lineToHeading(line, bodyFontSize, pageNum)
			items = append(items, item)
			continue
		}

		// Check if this line is a list item
		if isListItemLine(line) {
			flushParagraph()
			item := lineToListItem(line, pageNum)
			items = append(items, item)
			continue
		}

		paragraphLines = append(paragraphLines, line)
	}
	flushParagraph()

	return items
}

func computeMedianLineSpacing(lines []textLine) float64 {
	if len(lines) < 2 {
		return 0
	}

	var spacings []float64
	for i := 1; i < len(lines); i++ {
		gap := lines[i-1].Y - lines[i].Y
		if gap > 0 {
			spacings = append(spacings, gap)
		}
	}

	if len(spacings) == 0 {
		return 0
	}

	sort.Float64s(spacings)
	return spacings[len(spacings)/2]
}

func isHeadingLine(line textLine, bodyFontSize float64, lines []textLine, idx int) bool {
	if bodyFontSize <= 0 {
		return false
	}

	text := strings.TrimSpace(line.Text)
	if len(text) == 0 {
		return false
	}

	// Significantly larger font = heading (strongest signal, overrides indentation)
	// Require structural evidence: short text followed by body-sized text
	if line.FontSize >= bodyFontSize*1.2 {
		if len(text) < 120 && isFollowedByBodySizedText(lines, idx, bodyFontSize) {
			return true
		}
	}

	// Indented text is almost never a heading for weaker signals (bold, all-caps)
	medianMinX := computeMedianMinX(lines)
	if isIndented(line, medianMinX, bodyFontSize) {
		return false
	}

	// Short bold text at body font size — require structural evidence
	if len(text) >= 2 && len(text) < 80 && line.FontSize >= bodyFontSize {
		allBold := len(line.Elements) > 0
		for _, el := range line.Elements {
			if !isBoldFont(el.FontName) && !isBoldFont(el.FontFamily) {
				allBold = false
				break
			}
		}
		if allBold {
			wordCount := len(strings.Fields(text))
			if wordCount >= 2 || hasSectionNumbering(text) || isCommonHeadingWord(text) {
				if isFollowedByBodyText(lines, idx, bodyFontSize) {
					return true
				}
			}
		}
	}

	// Numbered section headings at body font size — structural pattern, no bold required
	if hasSectionNumbering(text) && line.FontSize >= bodyFontSize*0.9 {
		rest := stripSectionNumber(text)
		if len(rest) >= 2 && len(rest) < 60 {
			if isFollowedByBodyText(lines, idx, bodyFontSize) {
				return true
			}
		}
	}

	// All-caps short text — require 2+ words or common heading pattern, plus followed by body text
	// Exclude high-numeric content (table rows, IDs, etc.)
	if len(text) > 2 && len(text) < 60 && isAllCaps(text) && !hasHighNumericContent(text) {
		wordCount := len(strings.Fields(text))
		if wordCount >= 2 || isCommonHeadingWord(text) {
			if isFollowedByBodyText(lines, idx, bodyFontSize) {
				return true
			}
		}
	}

	return false
}

// isIndented checks whether a line starts significantly to the right of the body text margin
func isIndented(line textLine, medianMinX float64, fontSize float64) bool {
	threshold := fontSize * 2
	if threshold < 12 {
		threshold = 12
	}
	return line.MinX > medianMinX+threshold
}

// isFollowedByBodySizedText checks if the next non-empty line is at body font size
// with at least minimal length. More lenient than isFollowedByBodyText (15 vs 30 chars).
// Used for the font-size heading path where the size difference is strong evidence.
func isFollowedByBodySizedText(lines []textLine, idx int, bodyFontSize float64) bool {
	for j := idx + 1; j < len(lines) && j <= idx+3; j++ {
		text := strings.TrimSpace(lines[j].Text)
		if len(text) == 0 {
			continue
		}
		sizeTolerance := bodyFontSize * 0.2
		if len(text) > 15 && math.Abs(lines[j].FontSize-bodyFontSize) <= sizeTolerance {
			return true
		}
		// Hit a non-body-sized or too-short line — be conservative
		return false
	}
	// End of page — allow headings at the bottom
	return true
}

// isFollowedByBodyText checks if the next non-empty line looks like regular body text
func isFollowedByBodyText(lines []textLine, idx int, bodyFontSize float64) bool {
	for j := idx + 1; j < len(lines) && j <= idx+3; j++ {
		text := strings.TrimSpace(lines[j].Text)
		if len(text) == 0 {
			continue
		}
		// Body text: reasonably long and at body font size
		sizeTolerance := bodyFontSize * 0.2
		if len(text) > 30 && math.Abs(lines[j].FontSize-bodyFontSize) <= sizeTolerance {
			return true
		}
		// If we hit another short line, it's not clear — be conservative
		return false
	}
	// End of page — allow headings at the bottom
	return true
}

// hasSectionNumbering recognizes patterns like "1.", "2.1", "III.", "A."
func hasSectionNumbering(text string) bool {
	if len(text) < 2 {
		return false
	}
	// Decimal: "1.", "2.1 ", "10.3.2 "
	i := 0
	for i < len(text) && ((text[i] >= '0' && text[i] <= '9') || text[i] == '.') {
		i++
	}
	if i > 0 && i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		return true
	}
	// Roman numerals
	for _, r := range []string{"I.", "II.", "III.", "IV.", "V.", "VI.", "VII.", "VIII.", "IX.", "X."} {
		if strings.HasPrefix(text, r) {
			return true
		}
	}
	return false
}

// stripSectionNumber removes a leading section number like "1.", "2.1 ", "III. " etc.
func stripSectionNumber(text string) string {
	i := 0
	for i < len(text) && (text[i] >= '0' && text[i] <= '9' || text[i] == '.') {
		i++
	}
	// If no decimal digits consumed, try Roman numerals
	if i == 0 {
		for _, r := range []string{"VIII.", "VII.", "III.", "II.", "IV.", "VI.", "IX.", "I.", "V.", "X."} {
			if strings.HasPrefix(text, r) {
				i = len(r)
				break
			}
		}
	}
	for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
		i++
	}
	return text[i:]
}

// isCommonHeadingWord checks if text is a commonly used section heading
func isCommonHeadingWord(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	switch upper {
	case "ABSTRACT", "INTRODUCTION", "BACKGROUND", "METHODS", "METHODOLOGY",
		"RESULTS", "DISCUSSION", "CONCLUSION", "CONCLUSIONS", "REFERENCES",
		"BIBLIOGRAPHY", "ACKNOWLEDGMENTS", "ACKNOWLEDGEMENTS", "APPENDIX",
		"SUMMARY", "OVERVIEW", "PREFACE", "FOREWORD", "CONTENTS", "INDEX",
		"GLOSSARY":
		return true
	}
	return false
}

func computeMedianMinX(lines []textLine) float64 {
	if len(lines) == 0 {
		return 0
	}
	xs := make([]float64, len(lines))
	for i, l := range lines {
		xs[i] = l.MinX
	}
	sort.Float64s(xs)
	return xs[len(xs)/2]
}

func isListItemLine(line textLine) bool {
	text := strings.TrimSpace(line.Text)
	if len(text) < 2 {
		return false
	}

	// Bullet characters
	first := rune(text[0])
	if first == '\u2022' || first == '\u2023' || first == '\u2043' || // •, ‣, ⁃
		first == '-' || first == '*' {
		return true
	}

	// Numbered: "1.", "1)", "(1)", "a.", "a)", "(a)", "i.", "(i)"
	if len(text) >= 2 {
		if (text[0] >= '0' && text[0] <= '9') && (text[1] == '.' || text[1] == ')') {
			return true
		}
		if text[0] == '(' && len(text) >= 3 {
			// (1) or (a) or (i)
			if (text[1] >= '0' && text[1] <= '9') || (text[1] >= 'a' && text[1] <= 'z') {
				if text[2] == ')' {
					return true
				}
			}
		}
		if (text[0] >= 'a' && text[0] <= 'z') && (text[1] == '.' || text[1] == ')') {
			return true
		}
	}

	return false
}

func lineToHeading(line textLine, bodyFontSize float64, pageNum int) types.TextItem {
	level := computeHeadingLevel(line.FontSize, bodyFontSize)

	return types.TextItem{
		Text:      strings.TrimSpace(line.Text),
		Label:     types.TextLabelSectionHeader,
		BBox:      lineBBox(line, pageNum),
		WordCells: lineToWordCells(line, pageNum),
		Level:     level,
	}
}

func lineToListItem(line textLine, pageNum int) types.TextItem {
	return types.TextItem{
		Text:      strings.TrimSpace(line.Text),
		Label:     types.TextLabelListItem,
		BBox:      lineBBox(line, pageNum),
		WordCells: lineToWordCells(line, pageNum),
	}
}

func linesToTextItem(lines []textLine, bodyFontSize float64, pageNum int) types.TextItem {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strings.TrimSpace(line.Text))
	}

	// Compute bounding box encompassing all lines
	bbox := mergeLineBBoxes(lines, pageNum)

	// Collect word cells from all lines
	var wordCells []types.WordCell
	for _, line := range lines {
		wordCells = append(wordCells, lineToWordCells(line, pageNum)...)
	}

	return types.TextItem{
		Text:      b.String(),
		Label:     types.TextLabelText,
		BBox:      bbox,
		WordCells: wordCells,
	}
}

// ---- Heading level ----

func computeHeadingLevel(fontSize, bodyFontSize float64) int {
	if bodyFontSize <= 0 {
		return 1
	}
	ratio := fontSize / bodyFontSize
	switch {
	case ratio >= 2.0:
		return 1
	case ratio >= 1.5:
		return 2
	case ratio >= 1.2:
		return 3
	default:
		return 4
	}
}

// ---- Font analysis helpers ----

func isBoldFont(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "bold") || strings.Contains(lower, "black") || strings.Contains(lower, "heavy")
}

func isAllCaps(text string) bool {
	hasLetter := false
	for _, r := range text {
		if unicode.IsLetter(r) {
			hasLetter = true
			if unicode.IsLower(r) {
				return false
			}
		}
	}
	return hasLetter
}

// hasHighNumericContent checks if text has a high ratio of digits to alphanumeric chars.
// Used to filter out table rows, IDs, and other numeric-heavy lines from heading detection.
func hasHighNumericContent(text string) bool {
	digits, alphas := 0, 0
	for _, r := range text {
		if unicode.IsDigit(r) {
			digits++
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alphas++
		}
	}
	return alphas > 0 && float64(digits)/float64(alphas) > 0.3
}

// ---- BBox helpers ----

func lineBBox(line textLine, pageNum int) *types.SemanticBBox {
	return &types.SemanticBBox{
		Left:   line.MinX,
		Right:  line.MaxX,
		Top:    line.Y + line.FontSize, // approximate top
		Bottom: line.Y,
		Page:   pageNum,
	}
}

func mergeLineBBoxes(lines []textLine, pageNum int) *types.SemanticBBox {
	if len(lines) == 0 {
		return nil
	}
	minX := lines[0].MinX
	maxX := lines[0].MaxX
	maxY := lines[0].Y + lines[0].FontSize
	minY := lines[0].Y

	for _, line := range lines[1:] {
		if line.MinX < minX {
			minX = line.MinX
		}
		if line.MaxX > maxX {
			maxX = line.MaxX
		}
		if line.Y+line.FontSize > maxY {
			maxY = line.Y + line.FontSize
		}
		if line.Y < minY {
			minY = line.Y
		}
	}

	return &types.SemanticBBox{
		Left:   minX,
		Right:  maxX,
		Top:    maxY,
		Bottom: minY,
		Page:   pageNum,
	}
}

// ---- WordCell generation ----

func lineToWordCells(line textLine, pageNum int) []types.WordCell {
	var cells []types.WordCell
	for _, el := range line.Elements {
		// Each TextElement becomes a WordCell
		cells = append(cells, types.WordCell{
			Text: el.Text,
			BBox: &types.SemanticBBox{
				Left:   el.X,
				Right:  el.X + el.Width,
				Top:    el.Y + el.FontSize,
				Bottom: el.Y,
				Page:   pageNum,
			},
			Confidence: 1.0,
			FromOCR:    false,
		})
	}
	return cells
}

// ---- Header/footer pattern matching ----

// normalizeForMatch strips page numbers and normalizes whitespace for comparison
func normalizeForMatch(text string) string {
	text = strings.TrimSpace(text)
	// Remove common page number patterns
	// "Page X of Y" → "Page * of *"
	// Standalone numbers → ""
	text = strings.ToLower(text)
	// Replace sequences of digits with a placeholder
	var b strings.Builder
	inDigits := false
	for _, r := range text {
		if r >= '0' && r <= '9' {
			if !inDigits {
				b.WriteByte('#')
				inDigits = true
			}
		} else {
			inDigits = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// findRepeatedPatterns returns patterns that appear on minPages or more pages
func findRepeatedPatterns(byPage map[int][]textPos, minPages int) map[string]bool {
	// Count how many pages each pattern appears on
	patternPages := make(map[string]map[int]bool)
	for page, items := range byPage {
		for _, item := range items {
			if patternPages[item.text] == nil {
				patternPages[item.text] = make(map[int]bool)
			}
			patternPages[item.text][page] = true
		}
	}

	result := make(map[string]bool)
	for pattern, pages := range patternPages {
		if len(pages) >= minPages {
			result[pattern] = true
		}
	}
	return result
}
