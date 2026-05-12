package acroform

import "strings"

// helveticaWidths maps each rune to its Helvetica glyph advance width in
// 1/1000 em units. Source: Adobe Helvetica AFM (WinAnsiEncoding subset).
var helveticaWidths = map[rune]int{
	// ASCII printable (U+0020 – U+007E)
	' ': 278, '!': 278, '"': 355, '#': 556, '$': 556, '%': 889, '&': 667,
	'\'': 222, '(': 333, ')': 333, '*': 389, '+': 584, ',': 278, '-': 333,
	'.': 278, '/': 278,
	'0': 556, '1': 556, '2': 556, '3': 556, '4': 556,
	'5': 556, '6': 556, '7': 556, '8': 556, '9': 556,
	':': 278, ';': 278, '<': 584, '=': 584, '>': 584, '?': 556, '@': 1015,
	'A': 667, 'B': 667, 'C': 722, 'D': 722, 'E': 667, 'F': 611, 'G': 778,
	'H': 722, 'I': 278, 'J': 500, 'K': 667, 'L': 556, 'M': 833, 'N': 722,
	'O': 778, 'P': 667, 'Q': 778, 'R': 722, 'S': 667, 'T': 611, 'U': 722,
	'V': 667, 'W': 944, 'X': 667, 'Y': 667, 'Z': 611,
	'[': 278, '\\': 278, ']': 278, '^': 469, '_': 556, '`': 333,
	'a': 556, 'b': 556, 'c': 500, 'd': 556, 'e': 556, 'f': 278, 'g': 556,
	'h': 556, 'i': 222, 'j': 222, 'k': 500, 'l': 222, 'm': 833, 'n': 556,
	'o': 556, 'p': 556, 'q': 556, 'r': 333, 's': 500, 't': 278, 'u': 556,
	'v': 500, 'w': 722, 'x': 500, 'y': 500, 'z': 444,
	'{': 334, '|': 260, '}': 334, '~': 584,
	// Windows-1252 high range (U+0080 – U+009F mapped to WinAnsi 0x80–0x9F)
	'€': 556,  // € 0x80
	'‚': 222,  // ‚ 0x82
	'ƒ': 556,  // ƒ 0x83
	'„': 556,  // „ 0x84
	'…': 1000, // … 0x85
	'†': 556,  // † 0x86
	'‡': 556,  // ‡ 0x87
	'ˆ': 278,  // ˆ 0x88
	'‰': 1000, // ‰ 0x89
	'Š': 667,  // Š 0x8A
	'‹': 333,  // ‹ 0x8B
	'Œ': 1000, // Œ 0x8C
	'Ž': 611,  // Ž 0x8E
	'‘': 222,  // ' 0x91
	'’': 222,  // ' 0x92
	'“': 333,  // " 0x93
	'”': 333,  // " 0x94
	'•': 350,  // • 0x95
	'–': 556,  // – 0x96
	'—': 1000, // — 0x97
	'˜': 333,  // ˜ 0x98
	'™': 1000, // ™ 0x99
	'š': 500,  // š 0x9A
	'›': 333,  // › 0x9B
	'œ': 944,  // œ 0x9C
	'ž': 444,  // ž 0x9E
	'Ÿ': 667,  // Ÿ 0x9F
	// Latin-1 Supplement (U+00A0 – U+00FF = WinAnsi 0xA0–0xFF)
	' ': 278,  // non-breaking space
	'¡': 333,  // ¡
	'¢': 556,  // ¢
	'£': 556,  // £
	'¤': 556,  // ¤
	'¥': 556,  // ¥
	'¦': 260,  // ¦
	'§': 556,  // §
	'¨': 333,  // ¨
	'©': 737,  // ©
	'ª': 370,  // ª
	'«': 556,  // «
	'¬': 584,  // ¬
	'­': 333,  // ­ soft hyphen
	'®': 737,  // ®
	'¯': 333,  // ¯
	'°': 400,  // °
	'±': 584,  // ±
	'²': 333,  // ²
	'³': 333,  // ³
	'´': 333,  // ´
	'µ': 556,  // µ
	'¶': 537,  // ¶
	'·': 278,  // ·
	'¸': 333,  // ¸
	'¹': 333,  // ¹
	'º': 365,  // º
	'»': 556,  // »
	'¼': 834,  // ¼
	'½': 834,  // ½
	'¾': 834,  // ¾
	'¿': 611,  // ¿
	'À': 667,  // À
	'Á': 667,  // Á
	'Â': 667,  // Â
	'Ã': 667,  // Ã
	'Ä': 667,  // Ä
	'Å': 667,  // Å
	'Æ': 1000, // Æ
	'Ç': 722,  // Ç
	'È': 667,  // È
	'É': 667,  // É
	'Ê': 667,  // Ê
	'Ë': 667,  // Ë
	'Ì': 278,  // Ì
	'Í': 278,  // Í
	'Î': 278,  // Î
	'Ï': 278,  // Ï
	'Ð': 722,  // Ð
	'Ñ': 722,  // Ñ
	'Ò': 778,  // Ò
	'Ó': 778,  // Ó
	'Ô': 778,  // Ô
	'Õ': 778,  // Õ
	'Ö': 778,  // Ö
	'×': 584,  // ×
	'Ø': 778,  // Ø
	'Ù': 722,  // Ù
	'Ú': 722,  // Ú
	'Û': 722,  // Û
	'Ü': 722,  // Ü
	'Ý': 667,  // Ý
	'Þ': 667,  // Þ
	'ß': 611,  // ß
	'à': 556,  // à
	'á': 556,  // á
	'â': 556,  // â
	'ã': 556,  // ã
	'ä': 556,  // ä
	'å': 556,  // å
	'æ': 889,  // æ
	'ç': 500,  // ç
	'è': 556,  // è
	'é': 556,  // é
	'ê': 556,  // ê
	'ë': 556,  // ë
	'ì': 278,  // ì
	'í': 278,  // í
	'î': 278,  // î
	'ï': 278,  // ï
	'ð': 556,  // ð
	'ñ': 556,  // ñ
	'ò': 556,  // ò
	'ó': 556,  // ó
	'ô': 556,  // ô
	'õ': 556,  // õ
	'ö': 556,  // ö
	'÷': 584,  // ÷
	'ø': 611,  // ø
	'ù': 556,  // ù
	'ú': 556,  // ú
	'û': 556,  // û
	'ü': 556,  // ü
	'ý': 500,  // ý
	'þ': 556,  // þ
	'ÿ': 500,  // ÿ
}

// glyphWidth returns the Helvetica advance width for r in points at fontSize.
// Falls back to the width of 'n' (556/1000 em) for code points not in the table.
func glyphWidth(r rune, fontSize float64) float64 {
	if w, ok := helveticaWidths[r]; ok {
		return float64(w) / 1000 * fontSize
	}
	return 556.0 / 1000 * fontSize
}

// stringWidth returns the total advance width of s in points at fontSize.
func stringWidth(s string, fontSize float64) float64 {
	var w float64
	for _, r := range s {
		w += glyphWidth(r, fontSize)
	}
	return w
}

// wrapText splits s into lines that each fit within maxWidth at fontSize.
// Explicit '\n' characters create hard line breaks; within each paragraph
// words are greedily packed. A word wider than maxWidth is kept on its own
// line rather than broken mid-word.
func wrapText(s string, maxWidth, fontSize float64) []string {
	var result []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			candidate := current + " " + word
			if stringWidth(candidate, fontSize) <= maxWidth {
				current = candidate
			} else {
				result = append(result, current)
				current = word
			}
		}
		result = append(result, current)
	}
	return result
}
