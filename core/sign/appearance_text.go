package sign

import (
	"strconv"
	"strings"
	"unicode"
)

const (
	// appearanceFontSize is the Helvetica size used in the signature box.
	appearanceFontSize = 8.0

	// appearanceLineHeight is the leading between lines, in points.
	appearanceLineHeight = 10.0

	// appearancePadding is the inset from the box border on each side. The
	// text origin sits at 4pt from the left, so a line has the box width less
	// twice that before it touches the frame.
	appearancePadding = 4.0

	// appearanceEllipsis marks a truncated line.
	//
	// ⚠ Three periods rather than U+2026. The ellipsis exists at 0x85 in
	// WinAnsi, but a viewer falling back to StandardEncoding renders that
	// position differently, and a signature box is the wrong place to discover
	// a font-encoding edge.
	appearanceEllipsis = "..."
)

// fitAppearanceLine truncates a line so it cannot overflow the signature box.
//
// ⚠⚠ A line that runs past the border is worse than a shortened one: the text
// does not wrap or clip at the Form XObject's BBox in every viewer, so an over-
// long signer name can print across the document underneath the box. Truncating
// is visible and honest; overflow looks like a rendering fault.
func fitAppearanceLine(line string, boxWidth float64) string {
	line = sanitizeAppearanceLine(line)

	maxWidth := boxWidth - 2*appearancePadding
	if maxWidth <= 0 {
		return ""
	}

	if helveticaWidth(line, appearanceFontSize) <= maxWidth {
		return line
	}

	// Trim runes until the line plus its ellipsis fits.
	runes := []rune(line)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]

		candidate := strings.TrimRight(string(runes), " ") + appearanceEllipsis
		if helveticaWidth(candidate, appearanceFontSize) <= maxWidth {
			return candidate
		}
	}

	return ""
}

// fitAppearanceLines drops lines that would render below the signature box.
//
// ⚠⚠ The vertical mirror of fitAppearanceLine, and for the same reason: a line
// drawn past the bottom edge does not clip to the Form XObject's BBox in every
// viewer, so extra caller-supplied lines can print down the document underneath
// the box. The first baseline sits at height-appearanceLineHeight-1 and each
// line steps down appearanceLineHeight, so a line fits only while its baseline
// stays above the bottom edge. When lines are dropped the last kept line becomes
// an ellipsis so the truncation is visible rather than silent.
func fitAppearanceLines(lines []string, boxHeight float64) []string {
	maxLines := int((boxHeight - 1) / appearanceLineHeight)
	if maxLines <= 0 {
		return nil
	}

	if len(lines) <= maxLines {
		return lines
	}

	kept := append([]string(nil), lines[:maxLines-1]...)

	return append(kept, appearanceEllipsis)
}

// sanitizeAppearanceLine removes what a single-line PDF text run cannot show.
//
// ⚠ Newlines and tabs are the dangerous ones: they are not line breaks inside
// a Tj string, they are control bytes, and they make the rest of the line
// disappear in some viewers.
func sanitizeAppearanceLine(line string) string {
	return strings.Map(
		func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\t' {
				return ' '
			}

			if unicode.IsControl(r) {
				return -1
			}

			return r
		},
		line,
	)
}

// helveticaWidth estimates the rendered width of text in Helvetica.
//
// ⚠ An estimate, deliberately: exact metrics need the AFM tables, and this is
// used only to decide where to truncate. The per-character values are the real
// Helvetica widths for the common ranges, so the estimate errs small on text
// that is mostly letters and digits — which is what a signature box holds.
func helveticaWidth(text string, fontSize float64) float64 {
	var total float64

	for _, r := range text {
		total += helveticaCharWidth(r)
	}

	return total * fontSize
}

func helveticaCharWidth(r rune) float64 {
	switch {
	case r == ' ':
		return 0.278
	case r >= '0' && r <= '9':
		return 0.556
	case r == 'i' || r == 'l' || r == 'j' || r == '!' || r == '|' || r == '.' ||
		r == ',' || r == ':' || r == ';' || r == '\'':
		return 0.222
	case r == 'f' || r == 't' || r == 'r' || r == '(' || r == ')' || r == '/' || r == '-':
		return 0.333
	case r == 'm' || r == 'w' || r == 'M' || r == 'W' || r == '@':
		return 0.889
	case r >= 'a' && r <= 'z':
		return 0.528
	case r >= 'A' && r <= 'Z':
		return 0.667
	default:
		return 0.556
	}
}

// winAnsiCP1252High maps the runes CP1252 places in 0x80-0x9F, where Latin-1
// has control codes.
//
// ⚠ Without this block a curly quote or a dash — which every word processor
// produces — falls through to the substitute character, so a name pasted from
// Word would lose its punctuation.
var winAnsiCP1252High = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86,
	'‡': 0x87, 'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C,
	'Ž': 0x8E, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95,
	'–': 0x96, '—': 0x97, '˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B,
	'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// winAnsiSubstitute stands in for a rune WinAnsi cannot represent.
const winAnsiSubstitute = '?'

// escapeAppearanceStr encodes a line as a PDF literal string in WinAnsi.
//
// ⚠⚠ NOT escapePDFStr, and the difference is not cosmetic. escapePDFStr emits
// runes as UTF-8, which is correct for a PDF text string but wrong inside a
// content stream: bytes there are indexes into the FONT's encoding, so a UTF-8
// sequence renders as two or three wrong glyphs. A signer called "José" became
// "JosÃ©" on the face of the signed document.
//
// ⚠ The caller must declare /Encoding/WinAnsiEncoding on the font, or the
// viewer falls back to StandardEncoding and the high range means something
// else again.
func escapeAppearanceStr(s string) string {
	var b strings.Builder

	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)

			continue
		case '(':
			b.WriteString(`\(`)

			continue
		case ')':
			b.WriteString(`\)`)

			continue
		}

		code, ok := winAnsiByte(r)
		if !ok {
			b.WriteRune(winAnsiSubstitute)

			continue
		}

		if code < 0x80 {
			b.WriteByte(code)

			continue
		}

		// ⚠ Octal, because a raw high byte in a literal string is legal but
		// survives fewer round trips through tools that re-encode the file.
		b.WriteString(`\`)
		b.WriteString(strconv.FormatUint(uint64(code), 8))
	}

	return b.String()
}

// winAnsiByte maps a rune to its WinAnsi code point.
func winAnsiByte(r rune) (byte, bool) {
	switch {
	case r >= 0x20 && r <= 0x7E:
		return byte(r), true
	case r >= 0xA0 && r <= 0xFF:
		return byte(r), true
	}

	if code, ok := winAnsiCP1252High[r]; ok {
		return code, true
	}

	return 0, false
}
