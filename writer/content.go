// Package writer provides PDF writing capabilities including page content streams
package writer

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

// --- Raw Operations ---

// Raw writes raw content stream data
func (cs *ContentStream) Raw(data string) *ContentStream {
	cs.buf.WriteString(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		cs.buf.WriteByte('\n')
	}
	return cs
}

// escapePDFString escapes special characters in a PDF string
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
			result.WriteRune(c)
		}
	}
	return result.String()
}
