package manipulate

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// TextStamp describes a text overlay to paint onto a page.
type TextStamp struct {
	Text     string  // text to render
	FontName string  // standard Type1 font name; default "Helvetica"
	FontSize float64 // in points; default 12
	X, Y     float64 // position in points from bottom-left of the page
	R, G, B  float64 // fill colour, 0–1 each; default black
	Angle    float64 // rotation in degrees, counter-clockwise; default 0
	Opacity  float64 // 0 = transparent, 1 = opaque; default 1
}

// PageNumberOptions configures automatic page-number stamping.
type PageNumberOptions struct {
	FontName string             // standard font; default "Helvetica"
	FontSize float64            // default 10
	Format   string             // printf-style; one %d = page, two %d = (page, total); default "%d"
	Position PageNumberPosition // default BottomCenter
	MarginX  float64            // extra inset from page edge, default 36 pt
	MarginY  float64            // extra inset from page edge, default 36 pt
	R, G, B  float64            // fill colour, 0–1 each; default black
}

// PageNumberPosition controls where page numbers are placed.
type PageNumberPosition int

const (
	BottomCenter PageNumberPosition = iota
	BottomRight
	BottomLeft
	TopCenter
	TopRight
	TopLeft
)

// StampText adds a text overlay to the specified page (1-based).
func (m *PDFManipulator) StampText(pageNumber int, stamp TextStamp) error {
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return err
	}
	if pageNumber < 1 || pageNumber > len(pageObjNums) {
		return fmt.Errorf("page %d out of range [1,%d]", pageNumber, len(pageObjNums))
	}
	return m.stampOnPage(pageObjNums[pageNumber-1], stamp)
}

// StampAllPages adds the same text overlay to every page.
func (m *PDFManipulator) StampAllPages(stamp TextStamp) error {
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return err
	}
	for _, objNum := range pageObjNums {
		if err := m.stampOnPage(objNum, stamp); err != nil {
			return err
		}
	}
	return nil
}

// StampPageNumbers adds page numbers to all pages.
func (m *PDFManipulator) StampPageNumbers(opts PageNumberOptions) error {
	if opts.FontName == "" {
		opts.FontName = "Helvetica"
	}
	if opts.FontSize == 0 {
		opts.FontSize = 10
	}
	if opts.Format == "" {
		opts.Format = "%d"
	}
	if opts.MarginX == 0 {
		opts.MarginX = 36
	}
	if opts.MarginY == 0 {
		opts.MarginY = 36
	}

	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return err
	}
	total := len(pageObjNums)

	for i, objNum := range pageObjNums {
		pageNum := i + 1
		text := stampFormatPageNumber(opts.Format, pageNum, total)
		x, y := stampCalcPageNumberPos(string(m.objects[objNum]), opts)
		stamp := TextStamp{
			Text:     text,
			FontName: opts.FontName,
			FontSize: opts.FontSize,
			X:        x,
			Y:        y,
			R:        opts.R,
			G:        opts.G,
			B:        opts.B,
			Opacity:  1,
		}
		if err := m.stampOnPage(objNum, stamp); err != nil {
			return err
		}
	}
	return nil
}

// StampText adds a text overlay to one page of pdfBytes (1-based page number).
func StampText(pdfBytes []byte, pageNumber int, stamp TextStamp, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("stamp text: %w", err)
	}
	if err := m.StampText(pageNumber, stamp); err != nil {
		return nil, err
	}
	return m.Rebuild()
}

// StampAllPages adds the same text overlay to every page of pdfBytes.
func StampAllPages(pdfBytes []byte, stamp TextStamp, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("stamp all pages: %w", err)
	}
	if err := m.StampAllPages(stamp); err != nil {
		return nil, err
	}
	return m.Rebuild()
}

// StampPageNumbers adds automatic page numbers to all pages of pdfBytes.
func StampPageNumbers(pdfBytes []byte, opts PageNumberOptions, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("stamp page numbers: %w", err)
	}
	if err := m.StampPageNumbers(opts); err != nil {
		return nil, err
	}
	return m.Rebuild()
}

// --- internal helpers -------------------------------------------------------

func (m *PDFManipulator) stampOnPage(pageObjNum int, stamp TextStamp) error {
	if stamp.FontName == "" {
		stamp.FontName = "Helvetica"
	}
	if stamp.FontSize == 0 {
		stamp.FontSize = 12
	}
	if stamp.Opacity == 0 {
		stamp.Opacity = 1
	}

	fontKey := stampFontKey(stamp.FontName)
	gsKey := ""
	if stamp.Opacity < 1 {
		gsKey = "GS_stamp"
	}

	// Build and register a new content stream object.
	streamData := buildStampContentStream(stamp, fontKey, gsKey)
	newObjNum := stampNextObjNum(m.objects)
	objBytes := fmt.Sprintf("%d 0 obj\n<</Length %d>>\nstream\n%s\nendstream\nendobj",
		newObjNum, len(streamData), streamData)
	m.objects[newObjNum] = []byte(objBytes)

	// Update the page dict.
	pageStr := string(m.objects[pageObjNum])
	pageStr = stampInjectFont(pageStr, fontKey, stamp.FontName)
	if gsKey != "" {
		pageStr = stampInjectExtGState(pageStr, gsKey, stamp.Opacity)
	}
	pageStr = stampAppendContents(pageStr, fmt.Sprintf("%d 0 R", newObjNum))
	m.objects[pageObjNum] = []byte(pageStr)
	return nil
}

// stampNextObjNum returns the first object number not already in use.
func stampNextObjNum(objects map[int][]byte) int {
	max := 0
	for n := range objects {
		if n > max {
			max = n
		}
	}
	return max + 1
}

// stampFontKey turns "Helvetica-Bold" into the PDF name "F_Helvetica_Bold".
func stampFontKey(fontName string) string {
	return "F_" + strings.ReplaceAll(fontName, "-", "_")
}

// buildStampContentStream returns the PDF content operators for the stamp.
func buildStampContentStream(stamp TextStamp, fontKey, gsKey string) string {
	var sb strings.Builder
	sb.WriteString("q\n")
	if gsKey != "" {
		fmt.Fprintf(&sb, "/%s gs\n", gsKey)
	}
	sb.WriteString("BT\n")

	// Text matrix: rotation + translation.
	rad := stamp.Angle * math.Pi / 180
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	fmt.Fprintf(&sb, "%.6f %.6f %.6f %.6f %.4f %.4f Tm\n",
		cosA, sinA, -sinA, cosA, stamp.X, stamp.Y)

	fmt.Fprintf(&sb, "/%s %.4f Tf\n", fontKey, stamp.FontSize)
	fmt.Fprintf(&sb, "%.4f %.4f %.4f rg\n", stamp.R, stamp.G, stamp.B)
	fmt.Fprintf(&sb, "(%s) Tj\n", stampEscapeString(stamp.Text))
	sb.WriteString("ET\nQ")
	return sb.String()
}

// stampEscapeString escapes a string for use in a PDF literal string.
func stampEscapeString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// stampFormatPageNumber formats a page number according to the format string.
// The format may contain one %d (page) or two %d (page, total).
func stampFormatPageNumber(format string, page, total int) string {
	count := strings.Count(format, "%d")
	if count >= 2 {
		return fmt.Sprintf(format, page, total)
	}
	return fmt.Sprintf(format, page)
}

// stampCalcPageNumberPos returns the (x, y) for a page number stamp given the
// page dict string and the options.
func stampCalcPageNumberPos(pageStr string, opts PageNumberOptions) (x, y float64) {
	width, height := stampParseMediaBox(pageStr)
	if width == 0 {
		width = 612
	}
	if height == 0 {
		height = 792
	}

	// Approximate text width for centering (5 pts per char at 10 pts).
	textWidth := float64(len(stampFormatPageNumber(opts.Format, 99, 999))) * opts.FontSize * 0.5

	switch opts.Position {
	case BottomRight:
		return width - opts.MarginX - textWidth, opts.MarginY
	case BottomLeft:
		return opts.MarginX, opts.MarginY
	case TopCenter:
		return (width - textWidth) / 2, height - opts.MarginY - opts.FontSize
	case TopRight:
		return width - opts.MarginX - textWidth, height - opts.MarginY - opts.FontSize
	case TopLeft:
		return opts.MarginX, height - opts.MarginY - opts.FontSize
	default: // BottomCenter
		return (width - textWidth) / 2, opts.MarginY
	}
}

// stampParseMediaBox parses the [x0 y0 x1 y1] MediaBox and returns (width, height).
func stampParseMediaBox(pageStr string) (width, height float64) {
	re := regexp.MustCompile(`/MediaBox\s*\[\s*([0-9.+-]+)\s+([0-9.+-]+)\s+([0-9.+-]+)\s+([0-9.+-]+)\s*\]`)
	m := re.FindStringSubmatch(pageStr)
	if m == nil {
		return 0, 0
	}
	var x0, y0, x1, y1 float64
	fmt.Sscanf(m[1], "%f", &x0)
	fmt.Sscanf(m[2], "%f", &y0)
	fmt.Sscanf(m[3], "%f", &x1)
	fmt.Sscanf(m[4], "%f", &y1)
	return x1 - x0, y1 - y0
}

// stampAppendContents updates the /Contents entry in the page dict so it
// includes newRef. If /Contents is a single reference, it becomes an array.
func stampAppendContents(pageStr, newRef string) string {
	re := regexp.MustCompile(`/Contents\s+(\[([^\]]*)\]|(\d+\s+\d+\s+R))`)
	loc := re.FindStringSubmatchIndex(pageStr)
	if loc == nil {
		return setDictValue(pageStr, "/Contents", "["+newRef+"]")
	}
	full := pageStr[loc[0]:loc[1]]
	if strings.Contains(full, "[") {
		// Already an array — insert before closing ].
		closeIdx := strings.LastIndex(full, "]")
		newFull := full[:closeIdx] + " " + newRef + "]"
		return pageStr[:loc[0]] + newFull + pageStr[loc[1]:]
	}
	// Single reference — convert to array.
	oldRef := strings.TrimSpace(full[len("/Contents"):])
	newFull := "/Contents [" + strings.TrimSpace(oldRef) + " " + newRef + "]"
	return pageStr[:loc[0]] + newFull + pageStr[loc[1]:]
}

// stampInjectFont ensures /Resources/Font contains an entry for fontKey mapped
// to an inline Type1 dict for baseFontName.
func stampInjectFont(pageStr, fontKey, baseFontName string) string {
	fontEntry := fmt.Sprintf("/%s<</Type/Font/Subtype/Type1/BaseFont/%s>>", fontKey, baseFontName)

	resPos := strings.Index(pageStr, "/Resources")
	if resPos == -1 {
		// No /Resources at all — inject a brand-new one.
		return stampInsertBeforeClosingDict(pageStr, fmt.Sprintf("/Resources<</Font<<%s>>>>", fontEntry))
	}
	afterKey := resPos + len("/Resources")

	// Try inline dict first.
	open, close := stampFindDictBounds(pageStr, afterKey)
	if open == -1 {
		// External reference — too complex to rewrite here; inject an inline Resources.
		return stampInsertBeforeClosingDict(pageStr, fmt.Sprintf("/Resources<</Font<<%s>>>>", fontEntry))
	}

	resDict := pageStr[open:close] // "<<...>>"
	fontPos := strings.Index(resDict, "/Font")
	if fontPos == -1 {
		// No /Font subdictionary — prepend one.
		newResDict := "<<" + fmt.Sprintf("/Font<<%s>>", fontEntry) + resDict[2:]
		return pageStr[:open] + newResDict + pageStr[close:]
	}

	// /Font exists — inject our entry into its dict.
	fOpen, fClose := stampFindDictBounds(resDict, fontPos+len("/Font"))
	if fOpen == -1 {
		return pageStr // couldn't parse, leave unchanged
	}
	// Check if the font key is already present.
	fontDictContent := resDict[fOpen:fClose]
	if strings.Contains(fontDictContent, "/"+fontKey) {
		return pageStr // already injected
	}
	newFontDict := "<<" + fontEntry + fontDictContent[2:]
	newResDict := resDict[:fOpen] + newFontDict + resDict[fClose:]
	return pageStr[:open] + newResDict + pageStr[close:]
}

// stampInjectExtGState ensures /Resources/ExtGState contains a graphics state
// entry with the given opacity.
func stampInjectExtGState(pageStr, gsKey string, opacity float64) string {
	gsEntry := fmt.Sprintf("/%s<</Type/ExtGState/ca %.4f/CA %.4f>>", gsKey, opacity, opacity)

	resPos := strings.Index(pageStr, "/Resources")
	if resPos == -1 {
		return pageStr
	}
	afterKey := resPos + len("/Resources")
	open, close := stampFindDictBounds(pageStr, afterKey)
	if open == -1 {
		return pageStr
	}
	resDict := pageStr[open:close]

	gsPos := strings.Index(resDict, "/ExtGState")
	if gsPos == -1 {
		newResDict := "<<" + fmt.Sprintf("/ExtGState<<%s>>", gsEntry) + resDict[2:]
		return pageStr[:open] + newResDict + pageStr[close:]
	}
	gOpen, gClose := stampFindDictBounds(resDict, gsPos+len("/ExtGState"))
	if gOpen == -1 {
		return pageStr
	}
	gsContent := resDict[gOpen:gClose]
	if strings.Contains(gsContent, "/"+gsKey) {
		return pageStr
	}
	newGSDict := "<<" + gsEntry + gsContent[2:]
	newResDict := resDict[:gOpen] + newGSDict + resDict[gClose:]
	return pageStr[:open] + newResDict + pageStr[close:]
}

// stampFindDictBounds finds the bounds of a <<...>> dict starting at or after
// searchFrom, returning (openIdx, closeIdx) where pageStr[openIdx:closeIdx] is
// the full "<<...>>" string. Returns (-1, -1) if not found.
func stampFindDictBounds(s string, searchFrom int) (open, close int) {
	i := searchFrom
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	if i+1 >= len(s) || s[i] != '<' || s[i+1] != '<' {
		return -1, -1
	}
	open = i
	depth := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '<' && s[i+1] == '<' {
			depth++
			i += 2
		} else if i+1 < len(s) && s[i] == '>' && s[i+1] == '>' {
			depth--
			i += 2
			if depth == 0 {
				return open, i
			}
		} else {
			i++
		}
	}
	return -1, -1
}

// stampInsertBeforeClosingDict inserts entry before the last >> in s.
func stampInsertBeforeClosingDict(s, entry string) string {
	last := strings.LastIndex(s, ">>")
	if last == -1 {
		return s
	}
	return s[:last] + entry + s[last:]
}
