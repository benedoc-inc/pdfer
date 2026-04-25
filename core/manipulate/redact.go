package manipulate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
)

// RedactBox specifies a rectangular region to redact on a particular page.
type RedactBox struct {
	Page int        // 1-indexed page number
	Rect [4]float64 // [llx lly urx ury] in PDF user space (points from bottom-left)
}

// Redact permanently removes content within the specified regions and replaces
// each area with an opaque black rectangle. It:
//   - rewrites page content streams to suppress text/path operators inside boxes
//   - zeros out image XObject streams whose placement overlaps a box
//   - zeros out annotation objects (text, links, highlights, etc.) whose /Rect
//     overlaps a box
//
// XMP metadata and /Info dictionary entries are not cleared by this call; call
// RedactMetadata separately for document-level metadata.
//
// Note: PDF incremental updates may contain prior revisions. Supply a clean
// (already-repaired/flattened) source PDF for full forensic redaction; or call
// Repair first.
func Redact(pdfBytes []byte, boxes []RedactBox, password []byte) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, false)
	if err != nil {
		return nil, fmt.Errorf("redact: parse failed: %w", err)
	}
	if err := m.ApplyRedactions(boxes); err != nil {
		return nil, err
	}
	return m.Rebuild()
}

// ApplyRedactions modifies the manipulator in-place, applying redaction to the
// specified page regions. Call Rebuild() afterwards to emit the modified PDF.
func (m *PDFManipulator) ApplyRedactions(boxes []RedactBox) error {
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return err
	}

	// Group boxes by page index (0-based).
	pageBoxes := make(map[int][][4]float64)
	for _, b := range boxes {
		idx := b.Page - 1
		if idx < 0 || idx >= len(pageObjNums) {
			return fmt.Errorf("redact: page %d out of range [1,%d]", b.Page, len(pageObjNums))
		}
		pageBoxes[idx] = append(pageBoxes[idx], b.Rect)
	}

	for pageIdx, rects := range pageBoxes {
		if err := m.redactPage(pageObjNums[pageIdx], rects); err != nil {
			return err
		}
	}
	return nil
}

// redactPage rewrites the content streams for a single page.
func (m *PDFManipulator) redactPage(pageObjNum int, boxes [][4]float64) error {
	pageStr := string(m.objects[pageObjNum])

	// Resolve /Contents references.
	// /Contents can be "N G R" (single) or "[N G R N G R ...]" (array).
	contentsRE := regexp.MustCompile(`/Contents\s*(\[[^\]]*\]|\d+\s+\d+\s+R)`)
	contentsM := contentsRE.FindStringSubmatch(pageStr)
	if contentsM == nil {
		return nil // no content
	}
	contentsVal := contentsM[1]
	refRE := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	refMatches := refRE.FindAllStringSubmatch(contentsVal, -1)
	if len(refMatches) == 0 {
		return nil
	}

	var streamObjNums []int
	var combined strings.Builder
	for _, rm := range refMatches {
		objNum, _ := strconv.Atoi(rm[1])
		streamObjNums = append(streamObjNums, objNum)
		text, err := redactDecodeStream(m.objects[objNum])
		if err != nil || text == "" {
			continue
		}
		combined.WriteString(text)
		combined.WriteByte('\n')
	}

	// Rewrite the stream, suppressing redacted content.
	suppressedXObjs := make(map[string]bool)
	rewritten := redactContentStream(combined.String(), boxes, suppressedXObjs)

	// Append opaque fill rectangles as an overlay.
	var overlay strings.Builder
	overlay.WriteString("q\n0 g\n")
	for _, box := range boxes {
		fmt.Fprintf(&overlay, "%.4g %.4g %.4g %.4g re\nf\n",
			box[0], box[1], box[2]-box[0], box[3]-box[1])
	}
	overlay.WriteString("Q\n")
	rewritten += "\n" + overlay.String()

	// Overwrite the first content stream object with the combined rewritten data.
	firstNum := streamObjNums[0]
	genNum := redactGenNum(m.objects[firstNum])
	newObj := fmt.Sprintf("%d %d obj\n<</Length %d>>\nstream\n%sendstream\nendobj\n",
		firstNum, genNum, len(rewritten), rewritten)
	m.objects[firstNum] = []byte(newObj)

	// Blank out any additional stream objects (their data is already merged above).
	for _, n := range streamObjNums[1:] {
		g := redactGenNum(m.objects[n])
		m.objects[n] = []byte(fmt.Sprintf("%d %d obj\n<</Length 0>>\nstream\nendstream\nendobj\n", n, g))
	}

	// If there were multiple streams, update /Contents to point to just the first.
	if len(streamObjNums) > 1 {
		newContents := fmt.Sprintf("%d 0 R", firstNum)
		pageStr = setDictValue(pageStr, "/Contents", newContents)
		m.objects[pageObjNum] = []byte(pageStr)
	}

	// Zero out image XObject data for every image whose placement was suppressed.
	for name := range suppressedXObjs {
		m.zeroXObjectStream(pageObjNum, name)
	}

	// Zero out annotation objects whose /Rect overlaps any redaction box.
	m.redactPageAnnotations(pageObjNum, boxes)

	return nil
}

// redactPageAnnotations zeros out annotation objects whose /Rect overlaps any
// of the given boxes. The /Annots array in the page dict is left intact; the
// referenced objects are replaced with empty dicts so the data is unrecoverable.
func (m *PDFManipulator) redactPageAnnotations(pageObjNum int, boxes [][4]float64) {
	pageStr := string(m.objects[pageObjNum])

	annotsRaw := extractDictValue(pageStr, "/Annots")
	if annotsRaw == "" {
		return
	}

	// /Annots can be an inline array "[N G R ...]" or an indirect ref "N G R".
	annotsStr := annotsRaw
	indirectRE := regexp.MustCompile(`^(\d+)\s+\d+\s+R$`)
	if m2 := indirectRE.FindStringSubmatch(strings.TrimSpace(annotsRaw)); m2 != nil {
		n, _ := strconv.Atoi(m2[1])
		if obj, ok := m.objects[n]; ok {
			s := string(obj)
			start := strings.Index(s, "[")
			end := strings.LastIndex(s, "]")
			if start >= 0 && end > start {
				annotsStr = s[start : end+1]
			}
		}
	}

	refRE := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	for _, ref := range refRE.FindAllStringSubmatch(annotsStr, -1) {
		annotObjNum, _ := strconv.Atoi(ref[1])
		annotObj, ok := m.objects[annotObjNum]
		if !ok {
			continue
		}
		rectStr := extractDictValue(string(annotObj), "/Rect")
		llx, lly, urx, ury, ok2 := parseRectArray(rectStr)
		if !ok2 {
			continue
		}
		if redactBBoxOverlap(llx, lly, urx, ury, boxes) {
			genNum := redactGenNum(annotObj)
			m.objects[annotObjNum] = []byte(fmt.Sprintf("%d %d obj\n<<>>\nendobj\n", annotObjNum, genNum))
		}
	}
}

// zeroXObjectStream resolves an XObject name to its object number via the page's
// resource dictionary and replaces its stream with a 1×1 black pixel so that
// no image data remains.
func (m *PDFManipulator) zeroXObjectStream(pageObjNum int, xobjName string) {
	objNum := m.xObjectObjNum(pageObjNum, xobjName)
	if objNum == 0 {
		return
	}
	target, ok := m.objects[objNum]
	if !ok {
		return
	}
	ts := string(target)
	if !strings.Contains(ts, "/Subtype/Image") && !strings.Contains(ts, "/Subtype /Image") {
		return
	}
	genNum := redactGenNum(target)
	m.objects[objNum] = []byte(fmt.Sprintf(
		"%d %d obj\n<</Type/XObject/Subtype/Image/Width 1/Height 1/ColorSpace/DeviceGray/BitsPerComponent 8/Length 1>>\nstream\n\x00\nendstream\nendobj\n",
		objNum, genNum))
}

// xObjectObjNum resolves an XObject resource name to its object number by
// searching the page dict and (if needed) an indirect /Resources object.
func (m *PDFManipulator) xObjectObjNum(pageObjNum int, xobjName string) int {
	search := []string{string(m.objects[pageObjNum])}

	// Follow indirect /Resources if present.
	resVal := extractDictValue(search[0], "/Resources")
	if resVal != "" {
		refRE := regexp.MustCompile(`^(\d+)\s+\d+\s+R$`)
		if m2 := refRE.FindStringSubmatch(strings.TrimSpace(resVal)); m2 != nil {
			n, _ := strconv.Atoi(m2[1])
			if obj, ok := m.objects[n]; ok {
				search = append(search, string(obj))
			}
		}
	}

	// Look for "/<name> N G R" in the search set.
	pat := regexp.MustCompile(`(?:^|[\s/<>])` + regexp.QuoteMeta("/"+xobjName) + `\s+(\d+)\s+\d+\s+R`)
	for _, s := range search {
		if m2 := pat.FindStringSubmatch(s); m2 != nil {
			n, _ := strconv.Atoi(m2[1])
			return n
		}
	}
	return 0
}

// parseRectArray parses a PDF array string "[x1 y1 x2 y2]" into four floats.
func parseRectArray(s string) (llx, lly, urx, ury float64, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '[' {
		return 0, 0, 0, 0, false
	}
	end := strings.Index(s, "]")
	if end < 1 {
		return 0, 0, 0, 0, false
	}
	parts := strings.Fields(s[1:end])
	if len(parts) < 4 {
		return 0, 0, 0, 0, false
	}
	v := [4]float64{}
	for i := 0; i < 4; i++ {
		f, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		v[i] = f
	}
	return v[0], v[1], v[2], v[3], true
}

// redactDecodeStream extracts and decompresses stream data from raw object bytes.
func redactDecodeStream(objBytes []byte) (string, error) {
	if len(objBytes) == 0 {
		return "", nil
	}
	s := string(objBytes)
	si := strings.Index(s, "stream")
	if si == -1 {
		return "", nil
	}
	// Detect FlateDecode filter.
	header := s[:si]
	compressed := strings.Contains(header, "/FlateDecode") ||
		strings.Contains(header, "FlateDecode")

	// Skip "stream" + EOL.
	start := si + 6
	if start < len(s) && s[start] == '\r' {
		start++
	}
	if start < len(s) && s[start] == '\n' {
		start++
	}
	end := strings.LastIndex(s, "endstream")
	if end <= start {
		return "", nil
	}
	raw := []byte(s[start:end])
	// Trim trailing whitespace.
	for len(raw) > 0 && (raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r') {
		raw = raw[:len(raw)-1]
	}

	if compressed {
		decoded, err := parse.DecodeFlateDecode(raw)
		if err != nil {
			return string(raw), nil // return as-is on decode failure
		}
		return string(decoded), nil
	}
	return string(raw), nil
}

// redactGenNum extracts the generation number from object header bytes.
func redactGenNum(objBytes []byte) int {
	re := regexp.MustCompile(`^\s*\d+\s+(\d+)\s+obj`)
	if m := re.FindStringSubmatch(string(objBytes)); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// ---- Content stream rewriter -----------------------------------------------

// redactContentStream rewrites a PDF content stream, suppressing operators
// that draw within any of the given bounding boxes. If suppressedXObjs is
// non-nil, XObject names whose Do invocation is suppressed are recorded there.
func redactContentStream(content string, boxes [][4]float64, suppressedXObjs map[string]bool) string {
	tokens := redactTokenize(content)

	var out strings.Builder

	// Graphics state.
	ctm := [6]float64{1, 0, 0, 1, 0, 0}
	var ctmStack [][6]float64

	// Text state.
	var textMatrix [6]float64
	var lineMatrix [6]float64
	leading := 0.0
	inText := false

	// Path buffering: hold path construction tokens until a painting op decides fate.
	var pathBuf strings.Builder
	var pathPoints [][2]float64

	// Operand stack for the current operator.
	var ops []string

	clearOps := func() { ops = ops[:0] }

	// emit writes the current operands + the given operator to out.
	emit := func(op string) {
		for _, o := range ops {
			out.WriteString(o)
			out.WriteByte(' ')
		}
		out.WriteString(op)
		out.WriteByte('\n')
	}

	// bufferPath writes the current operands + operator to the path buffer.
	bufferPath := func(op string) {
		for _, o := range ops {
			pathBuf.WriteString(o)
			pathBuf.WriteByte(' ')
		}
		pathBuf.WriteString(op)
		pathBuf.WriteByte('\n')
	}

	// pathBboxOverlaps returns true if the accumulated path points overlap a box.
	pathBboxOverlaps := func() bool {
		if len(pathPoints) == 0 {
			return false
		}
		minX, minY := pathPoints[0][0], pathPoints[0][1]
		maxX, maxY := minX, minY
		for _, p := range pathPoints[1:] {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
		return redactBBoxOverlap(minX, minY, maxX, maxY, boxes)
	}

	flushPath := func(op string, suppress bool) {
		if !suppress {
			out.WriteString(pathBuf.String())
			emit(op)
		}
		// Clear path buffer regardless.
		pathBuf.Reset()
		pathPoints = pathPoints[:0]
	}

	// Transform a point from user space through the CTM.
	// For typical PDFs the CTM is identity and (px, py) ≡ (ops[n-2], ops[n-1]).
	applyCtm := func(px, py float64) (float64, float64) {
		return px*ctm[0] + py*ctm[2] + ctm[4],
			px*ctm[1] + py*ctm[3] + ctm[5]
	}

	addPathPoint := func(px, py float64) {
		x, y := applyCtm(px, py)
		pathPoints = append(pathPoints, [2]float64{x, y})
	}

	// textPos returns the current text position in page (user) space.
	textPos := func() (float64, float64) {
		return textMatrix[4]*ctm[0] + textMatrix[5]*ctm[2] + ctm[4],
			textMatrix[4]*ctm[1] + textMatrix[5]*ctm[3] + ctm[5]
	}

	// textInBox returns true if the current text position is inside any box.
	textInBox := func() bool {
		x, y := textPos()
		for _, box := range boxes {
			if x >= box[0] && x <= box[2] && y >= box[1] && y <= box[3] {
				return true
			}
		}
		return false
	}

	// Matrix multiply: result = m1 × m2 (PDF column-vector convention).
	mulMat := func(a, b [6]float64) [6]float64 {
		return [6]float64{
			a[0]*b[0] + a[1]*b[2],
			a[0]*b[1] + a[1]*b[3],
			a[2]*b[0] + a[3]*b[2],
			a[2]*b[1] + a[3]*b[3],
			a[4]*b[0] + a[5]*b[2] + b[4],
			a[4]*b[1] + a[5]*b[3] + b[5],
		}
	}

	parseN := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	for _, tok := range tokens {
		if tok.kind == rtokOperand {
			ops = append(ops, tok.value)
			continue
		}

		// Operator token.
		op := tok.value

		switch op {
		// ---- Graphics state ----
		case "q":
			ctmStack = append(ctmStack, ctm)
			emit("q")

		case "Q":
			if len(ctmStack) > 0 {
				ctm = ctmStack[len(ctmStack)-1]
				ctmStack = ctmStack[:len(ctmStack)-1]
			}
			emit("Q")

		case "cm":
			if len(ops) >= 6 {
				m := [6]float64{
					parseN(ops[0]), parseN(ops[1]),
					parseN(ops[2]), parseN(ops[3]),
					parseN(ops[4]), parseN(ops[5]),
				}
				ctm = mulMat(m, ctm)
			}
			emit("cm")

		// ---- Text object ----
		case "BT":
			inText = true
			textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
			lineMatrix = textMatrix
			emit("BT")

		case "ET":
			inText = false
			emit("ET")

		// ---- Text positioning ----
		case "Td":
			if len(ops) >= 2 {
				tx, ty := parseN(ops[len(ops)-2]), parseN(ops[len(ops)-1])
				lineMatrix = redactTranslate(lineMatrix, tx, ty)
				textMatrix = lineMatrix
			}
			emit("Td")

		case "TD":
			if len(ops) >= 2 {
				tx, ty := parseN(ops[len(ops)-2]), parseN(ops[len(ops)-1])
				leading = -ty
				lineMatrix = redactTranslate(lineMatrix, tx, ty)
				textMatrix = lineMatrix
			}
			emit("TD")

		case "Tm":
			if len(ops) >= 6 {
				for i := 0; i < 6; i++ {
					textMatrix[i] = parseN(ops[i])
				}
				lineMatrix = textMatrix
			}
			emit("Tm")

		case "T*":
			lineMatrix = redactTranslate(lineMatrix, 0, -leading)
			textMatrix = lineMatrix
			emit("T*")

		// ---- Text showing ----
		case "Tj", "TJ":
			if inText && textInBox() {
				// suppress
			} else {
				emit(op)
			}

		case "'":
			// T* + Tj
			lineMatrix = redactTranslate(lineMatrix, 0, -leading)
			textMatrix = lineMatrix
			if inText && textInBox() {
				// suppress (still emit the line-advance implicitly via T*)
				out.WriteString("T*\n")
			} else {
				emit("'")
			}

		case "\"":
			// set wordSpacing, charSpacing + ' operator
			if len(ops) >= 2 {
				lineMatrix = redactTranslate(lineMatrix, 0, -leading)
				textMatrix = lineMatrix
			}
			if inText && textInBox() {
				// emit spacing operators but suppress the text
				if len(ops) >= 2 {
					fmt.Fprintf(&out, "%s %s Tw Tc T*\n", ops[0], ops[1])
				}
			} else {
				emit("\"")
			}

		// ---- Path construction ----
		case "m":
			if len(ops) >= 2 {
				addPathPoint(parseN(ops[len(ops)-2]), parseN(ops[len(ops)-1]))
			}
			bufferPath("m")

		case "l":
			if len(ops) >= 2 {
				addPathPoint(parseN(ops[len(ops)-2]), parseN(ops[len(ops)-1]))
			}
			bufferPath("l")

		case "c":
			if len(ops) >= 6 {
				// Three control/endpoint pairs — add all to bbox.
				for i := 0; i+1 < len(ops); i += 2 {
					addPathPoint(parseN(ops[i]), parseN(ops[i+1]))
				}
			}
			bufferPath("c")

		case "v":
			if len(ops) >= 4 {
				addPathPoint(parseN(ops[0]), parseN(ops[1]))
				addPathPoint(parseN(ops[2]), parseN(ops[3]))
			}
			bufferPath("v")

		case "y":
			if len(ops) >= 4 {
				addPathPoint(parseN(ops[0]), parseN(ops[1]))
				addPathPoint(parseN(ops[2]), parseN(ops[3]))
			}
			bufferPath("y")

		case "re":
			if len(ops) >= 4 {
				x, y := parseN(ops[0]), parseN(ops[1])
				w, h := parseN(ops[2]), parseN(ops[3])
				addPathPoint(x, y)
				addPathPoint(x+w, y)
				addPathPoint(x+w, y+h)
				addPathPoint(x, y+h)
			}
			bufferPath("re")

		case "h":
			bufferPath("h")

		// ---- Path painting ----
		case "S", "s", "f", "F", "B", "B*", "b", "b*", "n":
			flushPath(op, pathBboxOverlaps())

		// Clipping operators: always pass through (suppressing them breaks layout).
		case "W", "W*":
			out.WriteString(pathBuf.String())
			pathBuf.Reset()
			pathPoints = pathPoints[:0]
			emit(op)

		// ---- XObject invocation ----
		case "Do":
			// Transform the unit square through the current CTM to get the image's
			// bounding box in page space, then check against redaction boxes.
			minX, minY, maxX, maxY := imageCtmBBox(ctm)
			suppress := redactBBoxOverlap(minX, minY, maxX, maxY, boxes)
			if suppress && suppressedXObjs != nil && len(ops) > 0 {
				suppressedXObjs[strings.TrimPrefix(ops[len(ops)-1], "/")] = true
			}
			if !suppress {
				emit("Do")
			}

		// ---- Marked content (always pass through) ----
		case "BMC", "BDC", "EMC":
			emit(op)

		// ---- Everything else ----
		default:
			emit(op)
		}

		clearOps()
	}

	// Flush any remaining path buffer (should not happen in a well-formed stream).
	if pathBuf.Len() > 0 {
		out.WriteString(pathBuf.String())
	}

	return out.String()
}

// redactTranslate applies a translation to a PDF matrix.
func redactTranslate(m [6]float64, tx, ty float64) [6]float64 {
	m[4] += tx*m[0] + ty*m[2]
	m[5] += tx*m[1] + ty*m[3]
	return m
}

// redactBBoxOverlap returns true if (minX,minY)–(maxX,maxY) overlaps any box.
func redactBBoxOverlap(minX, minY, maxX, maxY float64, boxes [][4]float64) bool {
	for _, box := range boxes {
		if maxX >= box[0] && minX <= box[2] && maxY >= box[1] && minY <= box[3] {
			return true
		}
	}
	return false
}

// ---- Tokenizer -------------------------------------------------------------

type rtokKind int

const (
	rtokOperand  rtokKind = iota // number, name, string, hex, array
	rtokOperator                 // PDF content stream operator keyword
)

type rtok struct {
	kind  rtokKind
	value string // original text representation
}

// redactTokenize splits a PDF content stream string into tokens.
// Operands are numbers, names (/Foo), literal strings ((…)), hex strings (<…>),
// and arrays ([…]). Everything else that starts with a letter or '" is an operator.
func redactTokenize(s string) []rtok {
	var toks []rtok
	i := 0
	n := len(s)

	for i < n {
		c := s[i]

		// Whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x00' || c == '\x0c' {
			i++
			continue
		}

		// Comment
		if c == '%' {
			for i < n && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			continue
		}

		// Literal string (...)
		if c == '(' {
			end := redactScanString(s, i)
			toks = append(toks, rtok{rtokOperand, s[i:end]})
			i = end
			continue
		}

		// Hex string <...> or dict delimiter <<
		if c == '<' {
			if i+1 < n && s[i+1] == '<' {
				i += 2
				continue
			}
			end := redactScanHex(s, i)
			toks = append(toks, rtok{rtokOperand, s[i:end]})
			i = end
			continue
		}

		if c == '>' {
			if i+1 < n && s[i+1] == '>' {
				i += 2
				continue
			}
			i++
			continue
		}

		// Array [...]
		if c == '[' {
			end := redactScanArray(s, i)
			toks = append(toks, rtok{rtokOperand, s[i:end]})
			i = end
			continue
		}

		// Name /Foo
		if c == '/' {
			start := i
			i++
			for i < n && !redactIsDelim(s[i]) {
				i++
			}
			toks = append(toks, rtok{rtokOperand, s[start:i]})
			continue
		}

		// Number
		if c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9') {
			start := i
			i++
			for i < n && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
				i++
			}
			toks = append(toks, rtok{rtokOperand, s[start:i]})
			continue
		}

		// Operator / keyword (letters, *, ', ")
		if redactIsAlpha(c) || c == '\'' || c == '"' {
			start := i
			i++
			for i < n && (redactIsAlpha(s[i]) || s[i] == '*') {
				i++
			}
			toks = append(toks, rtok{rtokOperator, s[start:i]})
			continue
		}

		i++ // skip unknown
	}
	return toks
}

func redactIsDelim(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
		c == '(' || c == ')' || c == '<' || c == '>' ||
		c == '[' || c == ']' || c == '{' || c == '}' ||
		c == '/' || c == '%'
}

func redactIsAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// redactScanString finds the end of a parenthetical PDF string starting at i.
func redactScanString(s string, i int) int {
	depth := 0
	i++ // skip '('
	for i < len(s) {
		c := s[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			if depth == 0 {
				return i + 1
			}
			depth--
		}
		i++
	}
	return i
}

// redactScanHex finds the end of a <hex> PDF string starting at i.
func redactScanHex(s string, i int) int {
	i++ // skip '<'
	for i < len(s) && s[i] != '>' {
		i++
	}
	if i < len(s) {
		i++ // skip '>'
	}
	return i
}

// redactScanArray finds the end of a [...] array starting at i.
func redactScanArray(s string, i int) int {
	depth := 0
	for i < len(s) {
		c := s[i]
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
			if depth == 0 {
				return i + 1
			}
		} else if c == '(' {
			i = redactScanString(s, i)
			continue
		}
		i++
	}
	return i
}

// imageCtmBBox returns the axis-aligned bounding box of the unit square [0,1]²
// transformed by ctm. When an image XObject is invoked with Do, the CTM maps
// the unit square to the image's page-space footprint.
func imageCtmBBox(ctm [6]float64) (minX, minY, maxX, maxY float64) {
	a, b, c, d, e, f := ctm[0], ctm[1], ctm[2], ctm[3], ctm[4], ctm[5]
	// Four corners of the unit square mapped through the CTM.
	xs := [4]float64{e, a + e, c + e, a + c + e}
	ys := [4]float64{f, b + f, d + f, b + d + f}
	minX, maxX = xs[0], xs[0]
	minY, maxY = ys[0], ys[0]
	for i := 1; i < 4; i++ {
		if xs[i] < minX {
			minX = xs[i]
		}
		if xs[i] > maxX {
			maxX = xs[i]
		}
		if ys[i] < minY {
			minY = ys[i]
		}
		if ys[i] > maxY {
			maxY = ys[i]
		}
	}
	return
}
