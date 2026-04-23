package extract

import "strings"

// standardFontWidths contains character widths for the 14 standard PDF fonts.
// Widths are in units of 1/1000 of the font size (text space units).
// Data from the PDF specification, Appendix D.
//
// Only printable ASCII (32-126) plus a few common glyphs are included.
// Missing characters fall back to the default width for the font.

// getStandardFontWidth returns the width of a rune in the given standard font,
// in thousandths of the font size. Returns (width, true) if found, or (0, false).
func getStandardFontWidth(fontBaseName string, r rune) (int, bool) {
	name := normalizeStandardFontName(fontBaseName)
	table, ok := standardFontWidths[name]
	if !ok {
		return 0, false
	}
	w, ok := table[r]
	return w, ok
}

// getStandardFontDefaultWidth returns the default width for missing glyphs in a standard font
func getStandardFontDefaultWidth(fontBaseName string) int {
	name := normalizeStandardFontName(fontBaseName)
	if strings.HasPrefix(name, "Courier") {
		return 600 // Courier is monospaced
	}
	return 500 // reasonable default for proportional fonts
}

// normalizeStandardFontName strips leading "/" and maps aliases
func normalizeStandardFontName(name string) string {
	name = strings.TrimPrefix(name, "/")
	// Common aliases
	switch name {
	case "TimesNewRoman", "Times":
		return "Times-Roman"
	case "TimesNewRoman,Bold", "Times,Bold":
		return "Times-Bold"
	case "TimesNewRoman,Italic", "Times,Italic":
		return "Times-Italic"
	case "TimesNewRoman,BoldItalic", "Times,BoldItalic":
		return "Times-BoldItalic"
	case "Arial":
		return "Helvetica"
	case "Arial,Bold":
		return "Helvetica-Bold"
	case "Arial,Italic":
		return "Helvetica-Oblique"
	case "Arial,BoldItalic":
		return "Helvetica-BoldOblique"
	}
	return name
}

// measureStringWidth returns the total width of a string in text space units (thousandths of font size).
// Uses the standard font width table if available, otherwise falls back to default width.
func measureStringWidth(text string, fontBaseName string) float64 {
	name := normalizeStandardFontName(fontBaseName)
	table, hasTable := standardFontWidths[name]
	defaultW := getStandardFontDefaultWidth(fontBaseName)

	var total int
	for _, r := range text {
		if hasTable {
			if w, ok := table[r]; ok {
				total += w
				continue
			}
		}
		total += defaultW
	}
	return float64(total)
}

// standardFontWidths maps font name → (rune → width in thousandths).
// Only the most common characters are included; see PDF Spec Appendix D.
var standardFontWidths = map[string]map[rune]int{
	"Helvetica":             helveticaWidths,
	"Helvetica-Bold":        helveticaBoldWidths,
	"Helvetica-Oblique":     helveticaObliqueWidths,
	"Helvetica-BoldOblique": helveticaBoldObliqueWidths,
	"Times-Roman":           timesRomanWidths,
	"Times-Bold":            timesBoldWidths,
	"Times-Italic":          timesItalicWidths,
	"Times-BoldItalic":      timesBoldItalicWidths,
	"Courier":               courierWidths,
	"Courier-Bold":          courierBoldWidths,
	"Courier-Oblique":       courierObliqueWidths,
	"Courier-BoldOblique":   courierBoldObliqueWidths,
	"Symbol":                symbolWidths,
	"ZapfDingbats":          zapfDingbatsWidths,
}

// Helvetica (ASCII subset)
var helveticaWidths = map[rune]int{
	' ': 278, '!': 278, '"': 355, '#': 556, '$': 556, '%': 889, '&': 667, '\'': 191,
	'(': 333, ')': 333, '*': 389, '+': 584, ',': 278, '-': 333, '.': 278, '/': 278,
	'0': 556, '1': 556, '2': 556, '3': 556, '4': 556, '5': 556, '6': 556, '7': 556,
	'8': 556, '9': 556, ':': 278, ';': 278, '<': 584, '=': 584, '>': 584, '?': 556,
	'@': 1015, 'A': 667, 'B': 667, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778,
	'H': 722, 'I': 278, 'J': 500, 'K': 667, 'L': 556, 'M': 833, 'N': 722, 'O': 778,
	'P': 667, 'Q': 778, 'R': 722, 'S': 667, 'T': 611, 'U': 722, 'V': 667, 'W': 944,
	'X': 667, 'Y': 667, 'Z': 611, '[': 278, '\\': 278, ']': 278, '^': 469, '_': 556,
	'`': 333, 'a': 556, 'b': 556, 'c': 500, 'd': 556, 'e': 556, 'f': 278, 'g': 556,
	'h': 556, 'i': 222, 'j': 222, 'k': 500, 'l': 222, 'm': 833, 'n': 556, 'o': 556,
	'p': 556, 'q': 556, 'r': 333, 's': 500, 't': 278, 'u': 556, 'v': 500, 'w': 722,
	'x': 500, 'y': 500, 'z': 500, '{': 334, '|': 260, '}': 334, '~': 584,
}

// Helvetica-Bold
var helveticaBoldWidths = map[rune]int{
	' ': 278, '!': 333, '"': 474, '#': 556, '$': 556, '%': 889, '&': 722, '\'': 238,
	'(': 333, ')': 333, '*': 389, '+': 584, ',': 278, '-': 333, '.': 278, '/': 278,
	'0': 556, '1': 556, '2': 556, '3': 556, '4': 556, '5': 556, '6': 556, '7': 556,
	'8': 556, '9': 556, ':': 333, ';': 333, '<': 584, '=': 584, '>': 584, '?': 611,
	'@': 975, 'A': 722, 'B': 722, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778,
	'H': 722, 'I': 278, 'J': 556, 'K': 722, 'L': 611, 'M': 833, 'N': 722, 'O': 778,
	'P': 667, 'Q': 778, 'R': 722, 'S': 667, 'T': 611, 'U': 722, 'V': 667, 'W': 944,
	'X': 667, 'Y': 667, 'Z': 611, '[': 333, '\\': 278, ']': 333, '^': 584, '_': 556,
	'`': 333, 'a': 556, 'b': 611, 'c': 556, 'd': 611, 'e': 556, 'f': 333, 'g': 611,
	'h': 611, 'i': 278, 'j': 278, 'k': 556, 'l': 278, 'm': 889, 'n': 611, 'o': 611,
	'p': 611, 'q': 611, 'r': 389, 's': 556, 't': 333, 'u': 611, 'v': 556, 'w': 778,
	'x': 556, 'y': 556, 'z': 500, '{': 389, '|': 280, '}': 389, '~': 584,
}

// Helvetica-Oblique (same metrics as Helvetica)
var helveticaObliqueWidths = helveticaWidths

// Helvetica-BoldOblique (same metrics as Helvetica-Bold)
var helveticaBoldObliqueWidths = helveticaBoldWidths

// Times-Roman
var timesRomanWidths = map[rune]int{
	' ': 250, '!': 333, '"': 408, '#': 500, '$': 500, '%': 833, '&': 778, '\'': 180,
	'(': 333, ')': 333, '*': 500, '+': 564, ',': 250, '-': 333, '.': 250, '/': 278,
	'0': 500, '1': 500, '2': 500, '3': 500, '4': 500, '5': 500, '6': 500, '7': 500,
	'8': 500, '9': 500, ':': 278, ';': 278, '<': 564, '=': 564, '>': 564, '?': 444,
	'@': 921, 'A': 722, 'B': 667, 'C': 667, 'D': 722, 'E': 611, 'F': 556, 'G': 722,
	'H': 722, 'I': 333, 'J': 389, 'K': 722, 'L': 611, 'M': 889, 'N': 722, 'O': 722,
	'P': 556, 'Q': 722, 'R': 667, 'S': 556, 'T': 611, 'U': 722, 'V': 722, 'W': 944,
	'X': 722, 'Y': 722, 'Z': 611, '[': 333, '\\': 278, ']': 333, '^': 469, '_': 500,
	'`': 333, 'a': 444, 'b': 500, 'c': 444, 'd': 500, 'e': 444, 'f': 333, 'g': 500,
	'h': 500, 'i': 278, 'j': 278, 'k': 500, 'l': 278, 'm': 778, 'n': 500, 'o': 500,
	'p': 500, 'q': 500, 'r': 333, 's': 389, 't': 278, 'u': 500, 'v': 500, 'w': 722,
	'x': 500, 'y': 500, 'z': 444, '{': 480, '|': 200, '}': 480, '~': 541,
}

// Times-Bold
var timesBoldWidths = map[rune]int{
	' ': 250, '!': 333, '"': 555, '#': 500, '$': 500, '%': 1000, '&': 833, '\'': 278,
	'(': 333, ')': 333, '*': 500, '+': 570, ',': 250, '-': 333, '.': 250, '/': 278,
	'0': 500, '1': 500, '2': 500, '3': 500, '4': 500, '5': 500, '6': 500, '7': 500,
	'8': 500, '9': 500, ':': 333, ';': 333, '<': 570, '=': 570, '>': 570, '?': 500,
	'@': 930, 'A': 722, 'B': 667, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778,
	'H': 778, 'I': 389, 'J': 500, 'K': 778, 'L': 667, 'M': 944, 'N': 722, 'O': 778,
	'P': 611, 'Q': 778, 'R': 722, 'S': 556, 'T': 667, 'U': 722, 'V': 722, 'W': 1000,
	'X': 722, 'Y': 722, 'Z': 667, '[': 333, '\\': 278, ']': 333, '^': 581, '_': 500,
	'`': 333, 'a': 500, 'b': 556, 'c': 444, 'd': 556, 'e': 444, 'f': 333, 'g': 500,
	'h': 556, 'i': 278, 'j': 333, 'k': 556, 'l': 278, 'm': 833, 'n': 556, 'o': 500,
	'p': 556, 'q': 556, 'r': 444, 's': 389, 't': 333, 'u': 556, 'v': 500, 'w': 722,
	'x': 500, 'y': 500, 'z': 444, '{': 394, '|': 220, '}': 394, '~': 520,
}

// Times-Italic
var timesItalicWidths = map[rune]int{
	' ': 250, '!': 333, '"': 420, '#': 500, '$': 500, '%': 833, '&': 778, '\'': 214,
	'(': 333, ')': 333, '*': 500, '+': 675, ',': 250, '-': 333, '.': 250, '/': 278,
	'0': 500, '1': 500, '2': 500, '3': 500, '4': 500, '5': 500, '6': 500, '7': 500,
	'8': 500, '9': 500, ':': 333, ';': 333, '<': 675, '=': 675, '>': 675, '?': 500,
	'@': 920, 'A': 611, 'B': 611, 'C': 667, 'D': 722, 'E': 611, 'F': 611, 'G': 722,
	'H': 722, 'I': 333, 'J': 444, 'K': 667, 'L': 556, 'M': 833, 'N': 667, 'O': 722,
	'P': 611, 'Q': 722, 'R': 611, 'S': 500, 'T': 556, 'U': 722, 'V': 611, 'W': 833,
	'X': 611, 'Y': 556, 'Z': 556, '[': 389, '\\': 278, ']': 389, '^': 422, '_': 500,
	'`': 333, 'a': 500, 'b': 500, 'c': 444, 'd': 500, 'e': 444, 'f': 278, 'g': 500,
	'h': 500, 'i': 278, 'j': 278, 'k': 444, 'l': 278, 'm': 722, 'n': 500, 'o': 500,
	'p': 500, 'q': 500, 'r': 389, 's': 389, 't': 278, 'u': 500, 'v': 444, 'w': 667,
	'x': 444, 'y': 444, 'z': 389, '{': 400, '|': 275, '}': 400, '~': 541,
}

// Times-BoldItalic
var timesBoldItalicWidths = map[rune]int{
	' ': 250, '!': 389, '"': 555, '#': 500, '$': 500, '%': 833, '&': 778, '\'': 278,
	'(': 333, ')': 333, '*': 500, '+': 570, ',': 250, '-': 333, '.': 250, '/': 278,
	'0': 500, '1': 500, '2': 500, '3': 500, '4': 500, '5': 500, '6': 500, '7': 500,
	'8': 500, '9': 500, ':': 333, ';': 333, '<': 570, '=': 570, '>': 570, '?': 500,
	'@': 832, 'A': 667, 'B': 667, 'C': 667, 'D': 722, 'E': 667, 'F': 667, 'G': 722,
	'H': 778, 'I': 389, 'J': 500, 'K': 667, 'L': 611, 'M': 889, 'N': 722, 'O': 722,
	'P': 611, 'Q': 722, 'R': 667, 'S': 556, 'T': 611, 'U': 722, 'V': 667, 'W': 889,
	'X': 667, 'Y': 611, 'Z': 611, '[': 333, '\\': 278, ']': 333, '^': 570, '_': 500,
	'`': 333, 'a': 500, 'b': 500, 'c': 444, 'd': 500, 'e': 444, 'f': 333, 'g': 500,
	'h': 556, 'i': 278, 'j': 278, 'k': 500, 'l': 278, 'm': 778, 'n': 556, 'o': 500,
	'p': 556, 'q': 500, 'r': 389, 's': 389, 't': 278, 'u': 556, 'v': 444, 'w': 667,
	'x': 500, 'y': 444, 'z': 389, '{': 348, '|': 220, '}': 348, '~': 570,
}

// Courier (monospaced — all characters 600)
var courierWidths = generateMonoWidths(600)

// Courier-Bold (monospaced — all characters 600)
var courierBoldWidths = courierWidths

// Courier-Oblique (monospaced — all characters 600)
var courierObliqueWidths = courierWidths

// Courier-BoldOblique (monospaced — all characters 600)
var courierBoldObliqueWidths = courierWidths

// Symbol — only a few common entries
var symbolWidths = map[rune]int{
	' ': 250, '+': 549, '-': 549, '=': 549, '(': 333, ')': 333,
	'0': 500, '1': 500, '2': 500, '3': 500, '4': 500, '5': 500,
	'6': 500, '7': 500, '8': 500, '9': 500,
}

// ZapfDingbats — minimal subset
var zapfDingbatsWidths = map[rune]int{
	' ': 278,
}

// generateMonoWidths creates a width table where every printable ASCII char has the same width
func generateMonoWidths(w int) map[rune]int {
	m := make(map[rune]int, 95)
	for r := rune(32); r <= 126; r++ {
		m[r] = w
	}
	return m
}
