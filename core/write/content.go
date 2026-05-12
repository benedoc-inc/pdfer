// Package writer provides PDF writing capabilities including page content streams
package write

import (
	"bytes"
	"fmt"
)

// ContentStream builds PDF page content streams
type ContentStream struct {
	buf bytes.Buffer
}

// NewContentStream creates a new content stream builder
func NewContentStream() *ContentStream {
	return &ContentStream{}
}

// Bytes returns the content stream data
func (cs *ContentStream) Bytes() []byte {
	return cs.buf.Bytes()
}

// String returns the content stream as a string
func (cs *ContentStream) String() string {
	return cs.buf.String()
}

// --- Graphics State Operations ---

// SaveState saves the current graphics state (q operator)
func (cs *ContentStream) SaveState() *ContentStream {
	cs.buf.WriteString("q\n")
	return cs
}

// RestoreState restores the previous graphics state (Q operator)
func (cs *ContentStream) RestoreState() *ContentStream {
	cs.buf.WriteString("Q\n")
	return cs
}

// SetMatrix sets the current transformation matrix (cm operator)
func (cs *ContentStream) SetMatrix(a, b, c, d, e, f float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f %.4f %.4f cm\n", a, b, c, d, e, f))
	return cs
}

// Translate moves the origin
func (cs *ContentStream) Translate(tx, ty float64) *ContentStream {
	return cs.SetMatrix(1, 0, 0, 1, tx, ty)
}

// Scale scales the coordinate system
func (cs *ContentStream) Scale(sx, sy float64) *ContentStream {
	return cs.SetMatrix(sx, 0, 0, sy, 0, 0)
}

// SetExtGState applies a named ExtGState dictionary (gs operator)
// name should be a bare resource name (e.g. "WMgs") without a leading slash
func (cs *ContentStream) SetExtGState(name string) *ContentStream {
	cs.buf.WriteString(name + " gs\n")
	return cs
}

// --- Color Operations ---

// SetFillColorRGB sets the fill color (rg operator)
func (cs *ContentStream) SetFillColorRGB(r, g, b float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f rg\n", r, g, b))
	return cs
}

// SetStrokeColorRGB sets the stroke color (RG operator)
func (cs *ContentStream) SetStrokeColorRGB(r, g, b float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f RG\n", r, g, b))
	return cs
}

// SetFillColorGray sets the fill color to grayscale (g operator)
func (cs *ContentStream) SetFillColorGray(gray float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f g\n", gray))
	return cs
}

// SetStrokeColorGray sets the stroke color to grayscale (G operator)
func (cs *ContentStream) SetStrokeColorGray(gray float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f G\n", gray))
	return cs
}

// --- Path Operations ---

// MoveTo starts a new subpath (m operator)
func (cs *ContentStream) MoveTo(x, y float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f m\n", x, y))
	return cs
}

// LineTo appends a line segment (l operator)
func (cs *ContentStream) LineTo(x, y float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f l\n", x, y))
	return cs
}

// Rectangle appends a rectangle (re operator)
func (cs *ContentStream) Rectangle(x, y, width, height float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f re\n", x, y, width, height))
	return cs
}

// Stroke strokes the current path (S operator)
func (cs *ContentStream) Stroke() *ContentStream {
	cs.buf.WriteString("S\n")
	return cs
}

// Fill fills the current path (f operator)
func (cs *ContentStream) Fill() *ContentStream {
	cs.buf.WriteString("f\n")
	return cs
}

// FillStroke fills and strokes the current path (B operator)
func (cs *ContentStream) FillStroke() *ContentStream {
	cs.buf.WriteString("B\n")
	return cs
}

// ClosePath closes the current subpath (h operator)
func (cs *ContentStream) ClosePath() *ContentStream {
	cs.buf.WriteString("h\n")
	return cs
}

// ClosePathStroke closes and strokes the path (s operator)
func (cs *ContentStream) ClosePathStroke() *ContentStream {
	cs.buf.WriteString("s\n")
	return cs
}

// SetLineWidth sets the line width (w operator)
func (cs *ContentStream) SetLineWidth(width float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f w\n", width))
	return cs
}

// --- Text Operations ---

// BeginText starts a text object (BT operator)
func (cs *ContentStream) BeginText() *ContentStream {
	cs.buf.WriteString("BT\n")
	return cs
}

// EndText ends a text object (ET operator)
func (cs *ContentStream) EndText() *ContentStream {
	cs.buf.WriteString("ET\n")
	return cs
}

// SetFont sets the font and size (Tf operator)
// fontName should be a resource name like "/F1"
func (cs *ContentStream) SetFont(fontName string, size float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%s %.4f Tf\n", fontName, size))
	return cs
}

// SetTextPosition sets the text position (Td operator)
func (cs *ContentStream) SetTextPosition(x, y float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f Td\n", x, y))
	return cs
}

// SetTextMatrix sets the text matrix (Tm operator)
func (cs *ContentStream) SetTextMatrix(a, b, c, d, e, f float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f %.4f %.4f Tm\n", a, b, c, d, e, f))
	return cs
}

// ShowText displays a string (Tj operator)
func (cs *ContentStream) ShowText(text string) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("(%s) Tj\n", escapePDFString(text)))
	return cs
}

// ShowTextNextLine moves to next line and shows text (' operator)
func (cs *ContentStream) ShowTextNextLine(text string) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("(%s) '\n", escapePDFString(text)))
	return cs
}

// ShowTextArray displays text with individual character positioning (TJ operator)
// Each element in the array can be either a string or a number (adjustment in thousandths of a text space unit)
// Example: ShowTextArray([]interface{}{"Hello", -50, "World"}) moves "World" 0.05 units to the left
func (cs *ContentStream) ShowTextArray(elements []interface{}) *ContentStream {
	cs.buf.WriteString("[")
	for i, elem := range elements {
		if i > 0 {
			cs.buf.WriteString(" ")
		}
		switch v := elem.(type) {
		case string:
			cs.buf.WriteString(fmt.Sprintf("(%s)", escapePDFString(v)))
		case int:
			cs.buf.WriteString(fmt.Sprintf("%d", v))
		case float64:
			cs.buf.WriteString(fmt.Sprintf("%.4f", v))
		}
	}
	cs.buf.WriteString("] TJ\n")
	return cs
}

// ShowTextHex displays text using hexadecimal string notation (TJ with hex)
// Useful for characters that are difficult to escape in literal strings
func (cs *ContentStream) ShowTextHex(hexString string) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("<%s> Tj\n", hexString))
	return cs
}

// MoveTextPosition moves the text position relative to current position (TD operator)
// This is equivalent to: SetTextLeading(-ty); SetTextPosition(tx, ty)
func (cs *ContentStream) MoveTextPosition(tx, ty float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f TD\n", tx, ty))
	return cs
}

// SetTextRenderMode sets the text rendering mode (Tr operator)
// Mode: 0=fill, 1=stroke, 2=fill+stroke, 3=invisible, 4=fill+clip, 5=stroke+clip, 6=fill+stroke+clip, 7=clip
func (cs *ContentStream) SetTextRenderMode(mode int) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%d Tr\n", mode))
	return cs
}

// SetTextLeading sets the text leading (TL operator)
func (cs *ContentStream) SetTextLeading(leading float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f TL\n", leading))
	return cs
}

// NextLine moves to the next line (T* operator)
func (cs *ContentStream) NextLine() *ContentStream {
	cs.buf.WriteString("T*\n")
	return cs
}

// SetCharSpacing sets character spacing (Tc operator)
func (cs *ContentStream) SetCharSpacing(spacing float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f Tc\n", spacing))
	return cs
}

// SetWordSpacing sets word spacing (Tw operator)
func (cs *ContentStream) SetWordSpacing(spacing float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f Tw\n", spacing))
	return cs
}

// SetTextRise sets text rise (Ts operator)
func (cs *ContentStream) SetTextRise(rise float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f Ts\n", rise))
	return cs
}

// --- Image Operations ---

// DrawImage draws an image XObject (Do operator)
// imageName should be a resource name like "/Im1"
// The image is drawn with its lower-left corner at the origin
// You should use SetMatrix to position and scale it first
func (cs *ContentStream) DrawImage(imageName string) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%s Do\n", imageName))
	return cs
}

// DrawImageAt draws an image at a specific position and size
func (cs *ContentStream) DrawImageAt(imageName string, x, y, width, height float64) *ContentStream {
	cs.SaveState()
	cs.SetMatrix(width, 0, 0, height, x, y)
	cs.DrawImage(imageName)
	cs.RestoreState()
	return cs
}

// --- Additional Graphics Operations ---

// CurveTo appends a cubic Bezier curve (c operator)
func (cs *ContentStream) CurveTo(x1, y1, x2, y2, x3, y3 float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f %.4f %.4f c\n", x1, y1, x2, y2, x3, y3))
	return cs
}

// CurveToV appends a cubic Bezier curve (v operator) - first control point = current point
func (cs *ContentStream) CurveToV(x2, y2, x3, y3 float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f v\n", x2, y2, x3, y3))
	return cs
}

// CurveToY appends a cubic Bezier curve (y operator) - second control point = end point
func (cs *ContentStream) CurveToY(x1, y1, x3, y3 float64) *ContentStream {
	cs.buf.WriteString(fmt.Sprintf("%.4f %.4f %.4f %.4f y\n", x1, y1, x3, y3))
	return cs
}

// SetLineDash sets the line dash pattern (d operator)
// dashArray is the dash pattern, dashPhase is the phase offset
func (cs *ContentStream) SetLineDash(dashArray []float64, dashPhase float64) *ContentStream {
	cs.buf.WriteString("[")
	for i, dash := range dashArray {
		if i > 0 {
			cs.buf.WriteString(" ")
		}
		cs.buf.WriteString(fmt.Sprintf("%.4f", dash))
	}
	cs.buf.WriteString(fmt.Sprintf("] %.4f d\n", dashPhase))
	return cs
}

// --- Raw Operations ---

// Raw writes raw content stream data
func (cs *ContentStream) Raw(data string) *ContentStream {
	cs.buf.WriteString(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		cs.buf.WriteByte('\n')
	}
	return cs
}

// escapePDFString escapes a Go string for use in a PDF literal string with a
// standard Type1 font that uses WinAnsiEncoding.  ASCII characters (0–127) are
// passed through; characters in the Latin-1 Supplement (U+00A0–U+00FF) map
// directly to their WinAnsi byte value; the Windows-1252 "high" range
// (U+0080–U+009F) is mapped to its WinAnsi equivalents.  Code points with no
// WinAnsiEncoding representation are silently dropped.
func escapePDFString(s string) string {
	var result bytes.Buffer
	for _, c := range s {
		switch c {
		case '(':
			result.WriteString("\\(")
		case ')':
			result.WriteString("\\)")
		case '\\':
			result.WriteString("\\\\")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			if c < 128 {
				result.WriteByte(byte(c))
			} else if b, ok := runeToWinAnsi(c); ok {
				result.WriteByte(b)
			}
			// silently drop characters outside WinAnsiEncoding
		}
	}
	return result.String()
}

// runeToWinAnsi converts a Unicode code point to its WinAnsiEncoding byte.
// Returns (byte, true) on success, (0, false) if the code point has no mapping.
func runeToWinAnsi(r rune) (byte, bool) {
	// Latin-1 Supplement maps directly (WinAnsi bytes 0xA0–0xFF == Unicode U+00A0–U+00FF).
	if r >= 0x00A0 && r <= 0x00FF {
		return byte(r), true
	}
	// Windows-1252 specific mappings for the 0x80–0x9F range.
	switch r {
	case 0x20AC:
		return 0x80, true // €
	case 0x201A:
		return 0x82, true // ‚
	case 0x0192:
		return 0x83, true // ƒ
	case 0x201E:
		return 0x84, true // „
	case 0x2026:
		return 0x85, true // …
	case 0x2020:
		return 0x86, true // †
	case 0x2021:
		return 0x87, true // ‡
	case 0x02C6:
		return 0x88, true // ˆ
	case 0x2030:
		return 0x89, true // ‰
	case 0x0160:
		return 0x8A, true // Š
	case 0x2039:
		return 0x8B, true // ‹
	case 0x0152:
		return 0x8C, true // Œ
	case 0x017D:
		return 0x8E, true // Ž
	case 0x2018:
		return 0x91, true // '
	case 0x2019:
		return 0x92, true // '
	case 0x201C:
		return 0x93, true // "
	case 0x201D:
		return 0x94, true // "
	case 0x2022:
		return 0x95, true // •
	case 0x2013:
		return 0x96, true // –
	case 0x2014:
		return 0x97, true // —
	case 0x02DC:
		return 0x98, true // ˜
	case 0x2122:
		return 0x99, true // ™
	case 0x0161:
		return 0x9A, true // š
	case 0x203A:
		return 0x9B, true // ›
	case 0x0153:
		return 0x9C, true // œ
	case 0x017E:
		return 0x9E, true // ž
	case 0x0178:
		return 0x9F, true // Ÿ
	}
	return 0, false
}
