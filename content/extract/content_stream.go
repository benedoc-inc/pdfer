package extract

import (
	"math"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// parseContentStream parses a PDF content stream and extracts text, graphics, and images
func parseContentStream(contentStr string, pageNum int, fontDecoders map[string]*FontDecoder, verbose bool) ([]types.TextElement, []types.Graphic, []types.ImageRef) {
	return parseContentStreamWithDecoders(contentStr, nil, pageNum, fontDecoders, verbose)
}

// parseContentStreamWithDecoders parses a PDF content stream with font decoding support.
// Uses a token-based parser instead of line-based regex for correct handling of
// multi-line strings, same-line operators, and TJ spacing adjustments.
func parseContentStreamWithDecoders(contentStr string, _ interface{}, pageNum int, fontDecoders map[string]*FontDecoder, verbose bool) ([]types.TextElement, []types.Graphic, []types.ImageRef) {
	var textElements []types.TextElement
	var graphics []types.Graphic
	var imageRefs []types.ImageRef

	ts := &textState{
		textMatrix: [6]float64{1, 0, 0, 1, 0, 0},
		lineMatrix: [6]float64{1, 0, 0, 1, 0, 0},
		leading:    0,
		ctm:        [6]float64{1, 0, 0, 1, 0, 0},
	}

	gs := &graphicsState{
		lineWidth:   1,
		fillColor:   &types.Color{Space: types.ColorSpaceRGB, R: 0, G: 0, B: 0},
		strokeColor: &types.Color{Space: types.ColorSpaceRGB, R: 0, G: 0, B: 0},
	}

	currentMatrix := [6]float64{1, 0, 0, 1, 0, 0}

	// State stack (for q/Q)
	type savedState struct {
		matrix [6]float64
		gs     graphicsState
	}
	var stateStack []savedState

	// Path builder for proper path tracking
	var currentPath []types.PathOperation

	// Tokenize the content stream
	tokens := tokenize(contentStr)
	operandStack := make([]string, 0, 8)

	for _, tok := range tokens {
		switch tok.kind {
		case tokenNumber, tokenName:
			operandStack = append(operandStack, tok.value)
			continue
		case tokenString, tokenHexString:
			// Push with a prefix so we can distinguish in operator handlers
			operandStack = append(operandStack, tok.value)
			continue
		case tokenArray:
			operandStack = append(operandStack, tok.value)
			continue
		case tokenOperator:
			// fall through to operator dispatch
		default:
			continue
		}

		op := tok.value
		switch op {
		// ---- Text object ----
		case "BT":
			ts.inText = true
			ts.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
			ts.lineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
			ts.x = 0
			ts.y = 0

		case "ET":
			ts.inText = false

		// ---- Text positioning ----
		case "Td":
			if len(operandStack) >= 2 {
				tx := parseNum(operandStack[len(operandStack)-2])
				ty := parseNum(operandStack[len(operandStack)-1])
				ts.lineMatrix = translateMatrix(ts.lineMatrix, tx, ty)
				ts.textMatrix = ts.lineMatrix
				ts.x = ts.textMatrix[4]
				ts.y = ts.textMatrix[5]
			}

		case "TD":
			if len(operandStack) >= 2 {
				tx := parseNum(operandStack[len(operandStack)-2])
				ty := parseNum(operandStack[len(operandStack)-1])
				ts.leading = -ty
				ts.lineMatrix = translateMatrix(ts.lineMatrix, tx, ty)
				ts.textMatrix = ts.lineMatrix
				ts.x = ts.textMatrix[4]
				ts.y = ts.textMatrix[5]
			}

		case "Tm":
			if len(operandStack) >= 6 {
				for i := 0; i < 6; i++ {
					ts.textMatrix[i] = parseNum(operandStack[len(operandStack)-6+i])
				}
				ts.lineMatrix = ts.textMatrix
				ts.x = ts.textMatrix[4]
				ts.y = ts.textMatrix[5]
			}

		case "T*":
			ts.lineMatrix = translateMatrix(ts.lineMatrix, 0, -ts.leading)
			ts.textMatrix = ts.lineMatrix
			ts.x = ts.textMatrix[4]
			ts.y = ts.textMatrix[5]

		// ---- Text state ----
		case "Tf":
			if len(operandStack) >= 2 {
				ts.fontName = operandStack[len(operandStack)-2]
				ts.fontSize = parseNum(operandStack[len(operandStack)-1])
				if fontDecoders != nil {
					ts.decoder = fontDecoders[ts.fontName]
				}
				ts.fontBaseName = resolveFontBaseName(ts.fontName, fontDecoders)
			}

		case "Tc":
			if len(operandStack) >= 1 {
				ts.charSpacing = parseNum(operandStack[len(operandStack)-1])
			}

		case "Tw":
			if len(operandStack) >= 1 {
				ts.wordSpacing = parseNum(operandStack[len(operandStack)-1])
			}

		case "Ts":
			if len(operandStack) >= 1 {
				ts.textRise = parseNum(operandStack[len(operandStack)-1])
			}

		case "TL":
			if len(operandStack) >= 1 {
				ts.leading = parseNum(operandStack[len(operandStack)-1])
			}

		// ---- Show text ----
		case "Tj":
			if len(operandStack) >= 1 {
				raw := operandStack[len(operandStack)-1]
				isHex := strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">")
				var text string
				if isHex {
					text = decodeTextWithFont(raw[1:len(raw)-1], ts.decoder, true)
				} else {
					if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
						raw = raw[1 : len(raw)-1]
					}
					rawText := unescapePDFString(raw)
					text = decodeTextWithFont(rawText, ts.decoder, false)
				}
				el := createTextElement(text, ts)
				textElements = append(textElements, el)
				advanceCursor(ts, text)
			}

		case "'":
			// Move to next line, then show text (like T* then Tj)
			ts.lineMatrix = translateMatrix(ts.lineMatrix, 0, -ts.leading)
			ts.textMatrix = ts.lineMatrix
			ts.x = ts.textMatrix[4]
			ts.y = ts.textMatrix[5]
			if len(operandStack) >= 1 {
				raw := operandStack[len(operandStack)-1]
				isHex := strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">")
				var text string
				if isHex {
					text = decodeTextWithFont(raw[1:len(raw)-1], ts.decoder, true)
				} else {
					if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
						raw = raw[1 : len(raw)-1]
					}
					rawText := unescapePDFString(raw)
					text = decodeTextWithFont(rawText, ts.decoder, false)
				}
				el := createTextElement(text, ts)
				textElements = append(textElements, el)
				advanceCursor(ts, text)
			}

		case "\"":
			// Set word spacing, char spacing, then show text
			if len(operandStack) >= 3 {
				ts.wordSpacing = parseNum(operandStack[len(operandStack)-3])
				ts.charSpacing = parseNum(operandStack[len(operandStack)-2])
				ts.lineMatrix = translateMatrix(ts.lineMatrix, 0, -ts.leading)
				ts.textMatrix = ts.lineMatrix
				ts.x = ts.textMatrix[4]
				ts.y = ts.textMatrix[5]
				raw := operandStack[len(operandStack)-1]
				isHex := strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">")
				var text string
				if isHex {
					text = decodeTextWithFont(raw[1:len(raw)-1], ts.decoder, true)
				} else {
					if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
						raw = raw[1 : len(raw)-1]
					}
					rawText := unescapePDFString(raw)
					text = decodeTextWithFont(rawText, ts.decoder, false)
				}
				el := createTextElement(text, ts)
				textElements = append(textElements, el)
				advanceCursor(ts, text)
			}

		case "TJ":
			if len(operandStack) >= 1 {
				arrStr := operandStack[len(operandStack)-1]
				elems := parseTJWordSplit(arrStr, ts)
				textElements = append(textElements, elems...)
			}

		// ---- Path construction ----
		case "m":
			if len(operandStack) >= 2 {
				x := parseNum(operandStack[len(operandStack)-2])
				y := parseNum(operandStack[len(operandStack)-1])
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpMove,
					Points: []types.Point{{X: x, Y: y}},
				})
			}

		case "l":
			if len(operandStack) >= 2 {
				x := parseNum(operandStack[len(operandStack)-2])
				y := parseNum(operandStack[len(operandStack)-1])
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpLine,
					Points: []types.Point{{X: x, Y: y}},
				})
			}

		case "c":
			if len(operandStack) >= 6 {
				p := make([]types.Point, 3)
				for i := 0; i < 3; i++ {
					p[i] = types.Point{
						X: parseNum(operandStack[len(operandStack)-6+i*2]),
						Y: parseNum(operandStack[len(operandStack)-5+i*2]),
					}
				}
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpCurve,
					Points: p,
				})
			}

		case "v":
			if len(operandStack) >= 4 {
				// Current point is first control point (not available here, approximate)
				p := make([]types.Point, 2)
				for i := 0; i < 2; i++ {
					p[i] = types.Point{
						X: parseNum(operandStack[len(operandStack)-4+i*2]),
						Y: parseNum(operandStack[len(operandStack)-3+i*2]),
					}
				}
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpCurve,
					Points: p,
				})
			}

		case "y":
			if len(operandStack) >= 4 {
				p := make([]types.Point, 2)
				for i := 0; i < 2; i++ {
					p[i] = types.Point{
						X: parseNum(operandStack[len(operandStack)-4+i*2]),
						Y: parseNum(operandStack[len(operandStack)-3+i*2]),
					}
				}
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpCurve,
					Points: p,
				})
			}

		case "re":
			if len(operandStack) >= 4 {
				x := parseNum(operandStack[len(operandStack)-4])
				y := parseNum(operandStack[len(operandStack)-3])
				w := parseNum(operandStack[len(operandStack)-2])
				h := parseNum(operandStack[len(operandStack)-1])
				currentPath = append(currentPath, types.PathOperation{
					Type:   types.PathOpRectangle,
					Points: []types.Point{{X: x, Y: y}, {X: w, Y: h}},
				})
			}

		case "h":
			currentPath = append(currentPath, types.PathOperation{Type: types.PathOpClose})

		// ---- Path painting ----
		case "S":
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, false, true))
			currentPath = nil

		case "s":
			// Close and stroke
			currentPath = append(currentPath, types.PathOperation{Type: types.PathOpClose})
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, false, true))
			currentPath = nil

		case "f", "F":
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, false))
			currentPath = nil

		case "f*":
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, false))
			currentPath = nil

		case "B":
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, true))
			currentPath = nil

		case "B*":
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, true))
			currentPath = nil

		case "b":
			currentPath = append(currentPath, types.PathOperation{Type: types.PathOpClose})
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, true))
			currentPath = nil

		case "b*":
			currentPath = append(currentPath, types.PathOperation{Type: types.PathOpClose})
			graphics = append(graphics, buildGraphicFromPath(currentPath, gs, true, true))
			currentPath = nil

		case "n":
			// No-op path painting (used with clipping)
			currentPath = nil

		// ---- Graphics state ----
		case "q":
			stateStack = append(stateStack, savedState{
				matrix: currentMatrix,
				gs:     *gs,
			})

		case "Q":
			if len(stateStack) > 0 {
				s := stateStack[len(stateStack)-1]
				stateStack = stateStack[:len(stateStack)-1]
				currentMatrix = s.matrix
				ts.ctm = currentMatrix
				*gs = s.gs
			}

		case "cm":
			if len(operandStack) >= 6 {
				m := [6]float64{}
				for i := 0; i < 6; i++ {
					m[i] = parseNum(operandStack[len(operandStack)-6+i])
				}
				currentMatrix = multiplyMatrix(currentMatrix, m)
				ts.ctm = currentMatrix
			}

		case "w":
			if len(operandStack) >= 1 {
				gs.lineWidth = parseNum(operandStack[len(operandStack)-1])
			}

		case "J":
			if len(operandStack) >= 1 {
				gs.lineCap = int(parseNum(operandStack[len(operandStack)-1]))
			}

		case "j":
			if len(operandStack) >= 1 {
				gs.lineJoin = int(parseNum(operandStack[len(operandStack)-1]))
			}

		case "M":
			if len(operandStack) >= 1 {
				gs.miterLimit = parseNum(operandStack[len(operandStack)-1])
			}

		// ---- Color ----
		case "rg":
			if len(operandStack) >= 3 {
				gs.fillColor = &types.Color{
					Space: types.ColorSpaceRGB,
					R:     parseNum(operandStack[len(operandStack)-3]),
					G:     parseNum(operandStack[len(operandStack)-2]),
					B:     parseNum(operandStack[len(operandStack)-1]),
				}
			}

		case "RG":
			if len(operandStack) >= 3 {
				gs.strokeColor = &types.Color{
					Space: types.ColorSpaceRGB,
					R:     parseNum(operandStack[len(operandStack)-3]),
					G:     parseNum(operandStack[len(operandStack)-2]),
					B:     parseNum(operandStack[len(operandStack)-1]),
				}
			}

		case "g":
			if len(operandStack) >= 1 {
				v := parseNum(operandStack[len(operandStack)-1])
				gs.fillColor = &types.Color{Space: types.ColorSpaceGray, R: v}
			}

		case "G":
			if len(operandStack) >= 1 {
				v := parseNum(operandStack[len(operandStack)-1])
				gs.strokeColor = &types.Color{Space: types.ColorSpaceGray, R: v}
			}

		case "k":
			if len(operandStack) >= 4 {
				gs.fillColor = &types.Color{
					Space: types.ColorSpaceCMYK,
					C:     parseNum(operandStack[len(operandStack)-4]),
					M:     parseNum(operandStack[len(operandStack)-3]),
					Y:     parseNum(operandStack[len(operandStack)-2]),
					K:     parseNum(operandStack[len(operandStack)-1]),
				}
			}

		case "K":
			if len(operandStack) >= 4 {
				gs.strokeColor = &types.Color{
					Space: types.ColorSpaceCMYK,
					C:     parseNum(operandStack[len(operandStack)-4]),
					M:     parseNum(operandStack[len(operandStack)-3]),
					Y:     parseNum(operandStack[len(operandStack)-2]),
					K:     parseNum(operandStack[len(operandStack)-1]),
				}
			}

		case "cs":
			// Set fill color space — operand is name, ignore for now
		case "CS":
			// Set stroke color space
		case "sc", "scn":
			// Set fill color in current color space — we'd need to track cs
			// For now, treat as generic: last 1-4 operands
		case "SC", "SCN":
			// Set stroke color in current color space

		// ---- XObject / Image ----
		case "Do":
			if len(operandStack) >= 1 {
				name := operandStack[len(operandStack)-1]
				x := currentMatrix[4]
				y := currentMatrix[5]
				w := currentMatrix[0]
				h := currentMatrix[3]
				if w < 0 {
					w = -w
				}
				if h < 0 {
					h = -h
				}
				imageRefs = append(imageRefs, types.ImageRef{
					ImageID:   name,
					X:         x,
					Y:         y,
					Width:     w,
					Height:    h,
					Transform: currentMatrix,
				})
			}

		// ---- Clipping ----
		case "W", "W*":
			// Clipping — we don't modify the path, just note it

			// ---- Inline images (BI/ID/EI) are handled by tokenizer ----
		}

		// Clear operand stack after each operator
		operandStack = operandStack[:0]
	}

	return textElements, graphics, imageRefs
}

// ---- Token types and tokenizer ----

type tokenKind int

const (
	tokenNumber    tokenKind = iota
	tokenName                // /Name
	tokenString              // (literal string)
	tokenHexString           // <hex string>
	tokenArray               // [...] (for TJ arrays)
	tokenOperator            // PDF operator keyword
)

type token struct {
	kind  tokenKind
	value string
}

// tokenize splits a PDF content stream into tokens.
// Handles: numbers, names (/Foo), literal strings ((...)), hex strings (<...>),
// arrays ([...]), and operators (alphabetic keywords).
func tokenize(s string) []token {
	var tokens []token
	i := 0
	n := len(s)

	for i < n {
		c := s[i]

		// Skip whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\x00' || c == '\x0c' {
			i++
			continue
		}

		// Skip comments
		if c == '%' {
			for i < n && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			continue
		}

		// Literal string
		if c == '(' {
			str, end := scanLiteralString(s, i)
			tokens = append(tokens, token{kind: tokenString, value: str})
			i = end
			continue
		}

		// Hex string
		if c == '<' && (i+1 >= n || s[i+1] != '<') {
			str, end := scanHexString(s, i)
			tokens = append(tokens, token{kind: tokenHexString, value: str})
			i = end
			continue
		}

		// Dictionary — skip << and >> as delimiters (shouldn't appear in content streams normally)
		if c == '<' && i+1 < n && s[i+1] == '<' {
			i += 2
			continue
		}
		if c == '>' && i+1 < n && s[i+1] == '>' {
			i += 2
			continue
		}

		// Array (used by TJ)
		if c == '[' {
			arr, end := scanArray(s, i)
			tokens = append(tokens, token{kind: tokenArray, value: arr})
			i = end
			continue
		}

		// Name
		if c == '/' {
			start := i
			i++
			for i < n && !isDelimiterOrWhitespace(s[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokenName, value: s[start:i]})
			continue
		}

		// Number (including negative and decimal)
		if c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9') {
			start := i
			i++
			for i < n && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
				i++
			}
			tokens = append(tokens, token{kind: tokenNumber, value: s[start:i]})
			continue
		}

		// Operator or keyword (alphabetic)
		if isAlpha(c) || c == '\'' || c == '"' {
			start := i
			i++
			for i < n && (isAlpha(s[i]) || s[i] == '*') {
				i++
			}
			tokens = append(tokens, token{kind: tokenOperator, value: s[start:i]})
			continue
		}

		// Handle inline image (BI ... ID <data> EI)
		if c == 'B' && i+1 < n && s[i+1] == 'I' && (i+2 >= n || isDelimiterOrWhitespace(s[i+2])) {
			// Already handled as operator above
		}

		// Skip unknown byte
		i++
	}

	return tokens
}

func scanLiteralString(s string, start int) (string, int) {
	// start points at '('
	depth := 1
	i := start + 1
	var b strings.Builder
	b.WriteByte('(')
	for i < len(s) && depth > 0 {
		if s[i] == '\\' && i+1 < len(s) {
			b.WriteByte(s[i])
			b.WriteByte(s[i+1])
			i += 2
			continue
		}
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), i
}

func scanHexString(s string, start int) (string, int) {
	// start points at '<'
	i := start + 1
	var b strings.Builder
	b.WriteByte('<')
	for i < len(s) && s[i] != '>' {
		b.WriteByte(s[i])
		i++
	}
	b.WriteByte('>')
	if i < len(s) {
		i++ // skip '>'
	}
	return b.String(), i
}

func scanArray(s string, start int) (string, int) {
	// start points at '['
	depth := 1
	i := start + 1
	var b strings.Builder
	b.WriteByte('[')
	for i < len(s) && depth > 0 {
		c := s[i]
		if c == '[' {
			depth++
		} else if c == ']' {
			depth--
		} else if c == '(' {
			// scan literal string inside array
			str, end := scanLiteralString(s, i)
			b.WriteString(str)
			i = end
			continue
		} else if c == '<' && (i+1 >= len(s) || s[i+1] != '<') {
			str, end := scanHexString(s, i)
			b.WriteString(str)
			i = end
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String(), i
}

func isDelimiterOrWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\x00', '\x0c',
		'(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ---- State types ----

// textState tracks the current text rendering state
type textState struct {
	fontName     string
	fontBaseName string // resolved base font name for width lookups
	fontSize     float64
	x            float64
	y            float64
	charSpacing  float64
	wordSpacing  float64
	textRise     float64
	textMatrix   [6]float64
	lineMatrix   [6]float64
	leading      float64
	inText       bool
	decoder      *FontDecoder
	ctm          [6]float64 // current transformation matrix (from cm/q/Q)
}

// graphicsState tracks the current graphics rendering state
type graphicsState struct {
	lineWidth   float64
	lineCap     int
	lineJoin    int
	miterLimit  float64
	fillColor   *types.Color
	strokeColor *types.Color
}

// ---- Text element creation ----

// effectiveScale computes the combined scale factor from the text matrix and CTM.
// Per PDF spec 9.4.4, the rendering matrix is Tm × CTM.
func effectiveScale(state *textState) float64 {
	rm := multiplyMatrix(state.textMatrix, state.ctm)
	scale := math.Sqrt(rm[0]*rm[0] + rm[1]*rm[1])
	if scale == 0 {
		return 1
	}
	return scale
}

// effectiveFontSize computes the actual rendered font size by combining
// Tf size with text matrix and CTM scaling.
func effectiveFontSize(state *textState) float64 {
	size := state.fontSize * effectiveScale(state)
	// Fallback: degenerate text matrices can produce zero scale even when fontSize > 0
	if size == 0 && state.fontSize > 0 {
		return state.fontSize
	}
	return size
}

// createTextElement creates a TextElement from text and current text state
func createTextElement(text string, state *textState) types.TextElement {
	width := computeTextWidth(text, state)
	effSize := effectiveFontSize(state)

	// Determine font style from font name
	fontFamily, fontStyle := parseFontStyle(state.fontName)

	return types.TextElement{
		Text:        text,
		X:           state.x,
		Y:           state.y,
		Width:       width,
		Height:      effSize,
		FontName:    state.fontName,
		FontSize:    effSize,
		FontFamily:  fontFamily,
		FontStyle:   fontStyle,
		CharSpacing: state.charSpacing,
		WordSpacing: state.wordSpacing,
		TextRise:    state.textRise,
		TextMatrix:  state.textMatrix,
	}
}

// computeTextWidth calculates text width using font metrics when available.
// Width is in user space units (points).
func computeTextWidth(text string, state *textState) float64 {
	effSize := effectiveFontSize(state)
	if effSize == 0 {
		return 0
	}

	// Try CID font widths first (Type0/Identity-H fonts)
	if state.decoder != nil && state.decoder.isCIDFont {
		totalW := 0.0
		runes := []rune(text)
		hasWidth := false
		for _, r := range runes {
			w := state.decoder.CIDCharWidth(int(r))
			totalW += float64(w)
			if w != 1000 { // non-default means we have real data
				hasWidth = true
			}
		}
		if hasWidth || len(state.decoder.cidWidths) > 0 {
			return totalW / 1000.0 * effSize
		}
	}

	// Try standard font width tables
	w := measureStringWidth(text, state.fontBaseName)
	if w > 0 {
		// w is in thousandths of font size; convert to points
		return w / 1000.0 * effSize
	}

	// Fallback: heuristic
	return float64(len([]rune(text))) * effSize * 0.5
}

// advanceCursor advances the text cursor after rendering text.
// Per PDF spec: new X = old X + sum of (charWidth + charSpacing) for each char,
// plus wordSpacing for each space character. Spacing is in text space, scaled by the matrix.
func advanceCursor(state *textState, text string) {
	w := computeTextWidth(text, state)
	scale := effectiveScale(state)
	nChars := float64(len([]rune(text)))
	w += nChars * state.charSpacing * scale
	for _, r := range text {
		if r == ' ' {
			w += state.wordSpacing * scale
		}
	}
	state.x += w
	state.textMatrix[4] = state.x
}

// ---- TJ word splitting ----

// tjElement represents one element in a TJ array: either a string or a spacing adjustment
type tjElement struct {
	isString  bool
	text      string // decoded text (if isString)
	rawString string // raw string before decoding
	isHex     bool   // was this a hex string?
	adjust    float64
}

// parseTJArray parses a TJ array string into elements
func parseTJArray(arrStr string) []tjElement {
	var elems []tjElement
	// Strip outer brackets
	arrStr = strings.TrimSpace(arrStr)
	if len(arrStr) >= 2 && arrStr[0] == '[' {
		arrStr = arrStr[1 : len(arrStr)-1]
	}

	i := 0
	for i < len(arrStr) {
		// Skip whitespace
		for i < len(arrStr) && (arrStr[i] == ' ' || arrStr[i] == '\t' || arrStr[i] == '\n' || arrStr[i] == '\r') {
			i++
		}
		if i >= len(arrStr) {
			break
		}

		switch {
		case arrStr[i] == '(':
			// Literal string
			start := i + 1
			depth := 1
			i++
			for i < len(arrStr) && depth > 0 {
				if arrStr[i] == '\\' && i+1 < len(arrStr) {
					i += 2
					continue
				}
				if arrStr[i] == '(' {
					depth++
				} else if arrStr[i] == ')' {
					depth--
				}
				i++
			}
			if depth == 0 {
				elems = append(elems, tjElement{
					isString:  true,
					rawString: arrStr[start : i-1],
					isHex:     false,
				})
			}

		case arrStr[i] == '<':
			// Hex string
			start := i + 1
			i++
			for i < len(arrStr) && arrStr[i] != '>' {
				i++
			}
			if i < len(arrStr) {
				elems = append(elems, tjElement{
					isString:  true,
					rawString: arrStr[start:i],
					isHex:     true,
				})
				i++
			}

		case arrStr[i] == '-' || arrStr[i] == '+' || arrStr[i] == '.' || (arrStr[i] >= '0' && arrStr[i] <= '9'):
			// Number
			start := i
			i++
			for i < len(arrStr) && (arrStr[i] == '.' || (arrStr[i] >= '0' && arrStr[i] <= '9')) {
				i++
			}
			if v, err := strconv.ParseFloat(arrStr[start:i], 64); err == nil {
				elems = append(elems, tjElement{adjust: v})
			}

		default:
			i++
		}
	}

	return elems
}

// parseTJWordSplit parses a TJ array, splitting into per-word TextElements
// when large spacing adjustments indicate word boundaries.
func parseTJWordSplit(arrStr string, ts *textState) []types.TextElement {
	elems := parseTJArray(arrStr)

	// Threshold: spacing adjustment < -100 (in thousandths of text space) = word break
	const wordBreakThreshold = -100.0

	var result []types.TextElement
	var wordBuf strings.Builder
	wordStartX := ts.x

	flushWord := func() {
		text := wordBuf.String()
		if text == "" {
			return
		}
		saved := ts.x
		ts.x = wordStartX
		el := createTextElement(text, ts)
		result = append(result, el)
		ts.x = saved
		wordBuf.Reset()
	}

	for _, elem := range elems {
		if elem.isString {
			var text string
			if elem.isHex {
				text = decodeTextWithFont(elem.rawString, ts.decoder, true)
			} else {
				raw := unescapePDFString(elem.rawString)
				text = decodeTextWithFont(raw, ts.decoder, false)
			}
			if wordBuf.Len() == 0 {
				wordStartX = ts.x
			}
			wordBuf.WriteString(text)
			// Advance cursor by rendered width
			w := computeTextWidth(text, ts)
			ts.x += w
			ts.textMatrix[4] = ts.x
		} else {
			// Spacing adjustment (in thousandths of text space)
			effSize := effectiveFontSize(ts)
			if elem.adjust < wordBreakThreshold {
				flushWord()
				// Apply the spacing
				displacement := -elem.adjust / 1000.0 * effSize
				ts.x += displacement
				ts.textMatrix[4] = ts.x
				wordStartX = ts.x
			} else {
				// Small adjustment — just adjust cursor, no word break
				displacement := -elem.adjust / 1000.0 * effSize
				ts.x += displacement
				ts.textMatrix[4] = ts.x
			}
		}
	}
	flushWord()

	// If no word splitting occurred, return all as one element for backward compat
	if len(result) == 0 {
		return nil
	}
	if len(result) == 1 {
		return result
	}
	return result
}

// ---- Font helpers ----

// resolveFontBaseName extracts the base font name for width table lookups
func resolveFontBaseName(fontName string, decoders map[string]*FontDecoder) string {
	if decoders != nil {
		if dec, ok := decoders[fontName]; ok && dec.fontName != "" {
			return dec.fontName
		}
	}
	// Strip leading /
	return strings.TrimPrefix(fontName, "/")
}

// parseFontStyle extracts family and style from a font name like "/Helvetica-Bold"
func parseFontStyle(fontName string) (family, style string) {
	name := strings.TrimPrefix(fontName, "/")
	parts := strings.SplitN(name, "-", 2)
	family = parts[0]
	if len(parts) < 2 {
		return family, "normal"
	}
	suffix := strings.ToLower(parts[1])
	switch {
	case strings.Contains(suffix, "boldital") || strings.Contains(suffix, "boldoblique"):
		return family, "bold-italic"
	case strings.Contains(suffix, "bold"):
		return family, "bold"
	case strings.Contains(suffix, "ital") || strings.Contains(suffix, "oblique"):
		return family, "italic"
	}
	return family, "normal"
}

// ---- Graphics helpers ----

// buildGraphicFromPath creates a Graphic from accumulated path operations
func buildGraphicFromPath(ops []types.PathOperation, gs *graphicsState, fill, stroke bool) types.Graphic {
	gType, bbox := classifyPath(ops)

	var fillColor, strokeColor *types.Color
	if fill && gs.fillColor != nil {
		c := *gs.fillColor
		fillColor = &c
	}
	if stroke && gs.strokeColor != nil {
		c := *gs.strokeColor
		strokeColor = &c
	}

	g := types.Graphic{
		Type:        gType,
		FillColor:   fillColor,
		StrokeColor: strokeColor,
		LineWidth:   gs.lineWidth,
		LineCap:     gs.lineCap,
		LineJoin:    gs.lineJoin,
		MiterLimit:  gs.miterLimit,
		BoundingBox: bbox,
	}

	if gType == types.GraphicTypePath {
		closed := false
		for _, op := range ops {
			if op.Type == types.PathOpClose {
				closed = true
				break
			}
		}
		g.Path = &types.Path{
			Operations: ops,
			Closed:     closed,
		}
	}

	return g
}

// classifyPath determines the graphic type and bounding box from path operations
func classifyPath(ops []types.PathOperation) (types.GraphicType, *types.Rectangle) {
	if len(ops) == 0 {
		return types.GraphicTypePath, nil
	}

	// Single rectangle?
	if len(ops) == 1 && ops[0].Type == types.PathOpRectangle && len(ops[0].Points) == 2 {
		x, y := ops[0].Points[0].X, ops[0].Points[0].Y
		w, h := ops[0].Points[1].X, ops[0].Points[1].Y
		return types.GraphicTypeRectangle, &types.Rectangle{
			LowerX: x,
			LowerY: y,
			UpperX: x + w,
			UpperY: y + h,
		}
	}

	// Simple line: move + line (no other ops besides close)
	moveCount, lineCount, curveCount, rectCount := 0, 0, 0, 0
	for _, op := range ops {
		switch op.Type {
		case types.PathOpMove:
			moveCount++
		case types.PathOpLine:
			lineCount++
		case types.PathOpCurve:
			curveCount++
		case types.PathOpRectangle:
			rectCount++
		}
	}

	if moveCount == 1 && lineCount == 1 && curveCount == 0 && rectCount == 0 {
		return types.GraphicTypeLine, pathBBox(ops)
	}

	return types.GraphicTypePath, pathBBox(ops)
}

// pathBBox computes a bounding box from path operations
func pathBBox(ops []types.PathOperation) *types.Rectangle {
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	hasPoints := false

	for _, op := range ops {
		for _, pt := range op.Points {
			if op.Type == types.PathOpRectangle && len(op.Points) == 2 {
				// Points are (x,y) and (w,h)
				x, y, w, h := op.Points[0].X, op.Points[0].Y, op.Points[1].X, op.Points[1].Y
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x+w > maxX {
					maxX = x + w
				}
				if y+h > maxY {
					maxY = y + h
				}
				hasPoints = true
				break // both points processed
			}
			if pt.X < minX {
				minX = pt.X
			}
			if pt.Y < minY {
				minY = pt.Y
			}
			if pt.X > maxX {
				maxX = pt.X
			}
			if pt.Y > maxY {
				maxY = pt.Y
			}
			hasPoints = true
		}
	}

	if !hasPoints {
		return nil
	}
	return &types.Rectangle{LowerX: minX, LowerY: minY, UpperX: maxX, UpperY: maxY}
}

// ---- Matrix helpers ----

func translateMatrix(m [6]float64, tx, ty float64) [6]float64 {
	return [6]float64{
		m[0], m[1],
		m[2], m[3],
		m[0]*tx + m[2]*ty + m[4],
		m[1]*tx + m[3]*ty + m[5],
	}
}

func multiplyMatrix(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

// ---- Parsing helpers ----

func parseNum(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// decodeTextWithFont decodes raw text bytes using the font's encoding
func decodeTextWithFont(rawText string, decoder *FontDecoder, isHex bool) string {
	if decoder == nil {
		if isHex {
			return decodeHexToLatin1(rawText)
		}
		return rawText
	}

	if isHex {
		return decoder.DecodeHex(rawText)
	}
	return decoder.Decode([]byte(rawText))
}

// decodeHexToLatin1 decodes a hex string to Latin-1 text (fallback when no font decoder)
func decodeHexToLatin1(hexStr string) string {
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\r", "")

	if len(hexStr)%2 != 0 {
		hexStr += "0"
	}

	var result strings.Builder
	for i := 0; i < len(hexStr); i += 2 {
		if val, err := strconv.ParseInt(hexStr[i:i+2], 16, 32); err == nil {
			result.WriteRune(rune(val))
		}
	}
	return result.String()
}
