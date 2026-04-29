package write

import (
	"strings"
	"unicode"
)

// TextAlign specifies horizontal text alignment within a box.
type TextAlign int

const (
	TextAlignLeft   TextAlign = iota
	TextAlignCenter TextAlign = 1
	TextAlignRight  TextAlign = 2
)

// TextBoxStyle configures a drawn text box.
type TextBoxStyle struct {
	FontName string    // resource name, e.g. "/F1"
	FontSize float64   // points; default 10
	Leading  float64   // line spacing; 0 = auto (1.2 * FontSize)
	Align    TextAlign
	Color    [3]float64 // RGB text color; zero value = black
}

// DrawTextBox draws word-wrapped text into the content stream starting at (x, y)
// (bottom-left of the first line) within the given width.
// Returns the total height consumed (positive downward from y).
func (pb *PageBuilder) DrawTextBox(x, y, width float64, text string, style TextBoxStyle) float64 {
	if style.FontSize == 0 {
		style.FontSize = 10
	}
	leading := style.Leading
	if leading == 0 {
		leading = style.FontSize * 1.2
	}
	if style.FontName == "" {
		style.FontName = "/F1"
	}

	lines := wrapText(text, width, style.FontSize)
	if len(lines) == 0 {
		return 0
	}

	cs := pb.content
	cs.SaveState()
	if style.Color[0] != 0 || style.Color[1] != 0 || style.Color[2] != 0 {
		cs.SetFillColorRGB(style.Color[0], style.Color[1], style.Color[2])
	}

	// Draw each line as its own BT/ET block with an absolute text matrix.
	for i, line := range lines {
		lineY := y - float64(i)*leading
		lineX := x

		if style.Align != TextAlignLeft && line != "" {
			lineWidth := approxTextWidth(line, style.FontSize)
			switch style.Align {
			case TextAlignCenter:
				lineX = x + (width-lineWidth)/2
			case TextAlignRight:
				lineX = x + width - lineWidth
			}
		}

		cs.BeginText()
		cs.SetFont(style.FontName, style.FontSize)
		cs.SetTextMatrix(1, 0, 0, 1, lineX, lineY)
		cs.ShowText(line)
		cs.EndText()
	}

	cs.RestoreState()
	return float64(len(lines)) * leading
}

// wrapText breaks text into lines that fit within maxWidth at the given font size.
func wrapText(text string, maxWidth, fontSize float64) []string {
	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			result = append(result, "")
			continue
		}
		result = append(result, wrapParagraph(paragraph, maxWidth, fontSize)...)
	}
	return result
}

// wrapParagraph wraps a single paragraph (no newlines) into lines.
func wrapParagraph(text string, maxWidth, fontSize float64) []string {
	words := strings.FieldsFunc(text, unicode.IsSpace)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	var current strings.Builder

	for _, word := range words {
		candidate := word
		if current.Len() > 0 {
			candidate = current.String() + " " + word
		}

		if approxTextWidth(candidate, fontSize) <= maxWidth {
			current.Reset()
			current.WriteString(candidate)
		} else {
			if current.Len() > 0 {
				lines = append(lines, current.String())
			}
			// If a single word is wider than maxWidth, output it anyway
			current.Reset()
			current.WriteString(word)
		}
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

// approxTextWidth estimates the rendered width of text at the given font size.
// Uses a conservative average of 0.55 character widths per point for proportional fonts.
// This is suitable for layout estimation; precise metrics require AFM data.
func approxTextWidth(text string, fontSize float64) float64 {
	// Per-character width estimates (normalized to font size 1.0) for Helvetica.
	// Characters not in the map use the default average.
	var total float64
	for _, ch := range text {
		total += charWidth(ch)
	}
	return total * fontSize
}

// charWidth returns the normalized width (width / 1000) for a character.
// Values are approximate Helvetica metrics; used for layout estimation only.
func charWidth(ch rune) float64 {
	switch {
	case ch == ' ':
		return 0.278
	case ch >= '0' && ch <= '9':
		return 0.556
	case ch == 'i' || ch == 'l' || ch == '!' || ch == '|' || ch == '1':
		return 0.222
	case ch == 'f' || ch == 'j' || ch == 'r' || ch == 't':
		return 0.333
	case ch == 'm' || ch == 'w' || ch == 'M' || ch == 'W':
		return 0.722
	case ch >= 'a' && ch <= 'z':
		return 0.500
	case ch >= 'A' && ch <= 'Z':
		return 0.611
	case ch == '.' || ch == ',' || ch == ':' || ch == ';':
		return 0.278
	case ch == '-':
		return 0.333
	default:
		return 0.556
	}
}
