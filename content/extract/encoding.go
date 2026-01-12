package extract

import (
	"regexp"
	"strconv"
	"strings"
)

// FontDecoder decodes character codes to Unicode for a specific font
type FontDecoder struct {
	// ToUnicode mapping from character codes to Unicode (highest priority)
	toUnicode map[int]rune

	// Base encoding (WinAnsiEncoding, MacRomanEncoding, etc.)
	baseEncoding map[int]rune

	// Differences array overlays the base encoding
	differences map[int]rune

	// Font name for debugging
	fontName string
}

// NewFontDecoder creates a new font decoder
func NewFontDecoder(fontName string) *FontDecoder {
	return &FontDecoder{
		toUnicode:    make(map[int]rune),
		baseEncoding: make(map[int]rune),
		differences:  make(map[int]rune),
		fontName:     fontName,
	}
}

// SetBaseEncoding sets the base encoding (WinAnsi, MacRoman, etc.)
func (fd *FontDecoder) SetBaseEncoding(encodingName string) {
	encodingName = strings.TrimPrefix(encodingName, "/")
	switch encodingName {
	case "WinAnsiEncoding":
		fd.baseEncoding = winAnsiEncoding
	case "MacRomanEncoding":
		fd.baseEncoding = macRomanEncoding
	case "StandardEncoding":
		fd.baseEncoding = standardEncoding
	case "MacExpertEncoding":
		fd.baseEncoding = macExpertEncoding
	case "Identity-H", "Identity-V":
		// Identity encoding - character codes are Unicode values
		// Don't set a base encoding, rely on ToUnicode
	}
}

// SetDifferences sets differences array entries
func (fd *FontDecoder) SetDifferences(code int, glyphName string) {
	if unicode := glyphNameToUnicode(glyphName); unicode != 0 {
		fd.differences[code] = unicode
	}
}

// SetToUnicode adds a ToUnicode mapping
func (fd *FontDecoder) SetToUnicode(code int, unicode rune) {
	fd.toUnicode[code] = unicode
}

// Decode decodes a byte slice to a Unicode string using the font's encoding
func (fd *FontDecoder) Decode(data []byte) string {
	var result strings.Builder

	for _, b := range data {
		code := int(b)
		var char rune

		// Priority: ToUnicode > Differences > BaseEncoding > Identity
		if unicode, ok := fd.toUnicode[code]; ok {
			char = unicode
		} else if unicode, ok := fd.differences[code]; ok {
			char = unicode
		} else if unicode, ok := fd.baseEncoding[code]; ok {
			char = unicode
		} else {
			// Default: treat as Latin-1 (ISO-8859-1)
			char = rune(code)
		}

		result.WriteRune(char)
	}

	return result.String()
}

// DecodeHex decodes a hex string to Unicode using the font's encoding
func (fd *FontDecoder) DecodeHex(hexStr string) string {
	// Remove < and > if present
	hexStr = strings.TrimPrefix(hexStr, "<")
	hexStr = strings.TrimSuffix(hexStr, ">")
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\r", "")

	// Pad with 0 if odd length
	if len(hexStr)%2 != 0 {
		hexStr += "0"
	}

	var result strings.Builder

	// Check if this is a 2-byte encoding (common for CID fonts)
	// Heuristic: if we have ToUnicode entries for codes > 255, use 2-byte decoding
	is2Byte := fd.has2ByteMapping()

	if is2Byte && len(hexStr) >= 4 {
		// 2-byte character codes
		for i := 0; i < len(hexStr); i += 4 {
			if i+4 > len(hexStr) {
				// Remaining single byte
				if val, err := strconv.ParseInt(hexStr[i:], 16, 32); err == nil {
					result.WriteRune(fd.lookupCode(int(val)))
				}
				break
			}
			if val, err := strconv.ParseInt(hexStr[i:i+4], 16, 32); err == nil {
				result.WriteRune(fd.lookupCode(int(val)))
			}
		}
	} else {
		// 1-byte character codes
		for i := 0; i < len(hexStr); i += 2 {
			if val, err := strconv.ParseInt(hexStr[i:i+2], 16, 32); err == nil {
				result.WriteRune(fd.lookupCode(int(val)))
			}
		}
	}

	return result.String()
}

// has2ByteMapping checks if the font has 2-byte ToUnicode mappings
func (fd *FontDecoder) has2ByteMapping() bool {
	for code := range fd.toUnicode {
		if code > 255 {
			return true
		}
	}
	return false
}

// lookupCode looks up a character code in all encoding tables
func (fd *FontDecoder) lookupCode(code int) rune {
	if unicode, ok := fd.toUnicode[code]; ok {
		return unicode
	}
	if unicode, ok := fd.differences[code]; ok {
		return unicode
	}
	if code < 256 {
		if unicode, ok := fd.baseEncoding[code]; ok {
			return unicode
		}
	}
	// Default: treat as Unicode code point (for Identity-H)
	if code < 0x10000 {
		return rune(code)
	}
	return '?'
}

// ParseToUnicodeCMap parses a ToUnicode CMap stream and populates the decoder
func (fd *FontDecoder) ParseToUnicodeCMap(cmapData string) {
	// Parse beginbfchar...endbfchar sections (single character mappings)
	// Format: <srcCode> <dstUnicode>
	bfcharPattern := regexp.MustCompile(`beginbfchar\s*([\s\S]*?)\s*endbfchar`)
	bfcharMatches := bfcharPattern.FindAllStringSubmatch(cmapData, -1)
	for _, match := range bfcharMatches {
		fd.parseBfcharBlock(match[1])
	}

	// Parse beginbfrange...endbfrange sections (range mappings)
	// Format: <srcCodeLo> <srcCodeHi> <dstUnicode> or <srcCodeLo> <srcCodeHi> [<dst1> <dst2> ...]
	bfrangePattern := regexp.MustCompile(`beginbfrange\s*([\s\S]*?)\s*endbfrange`)
	bfrangeMatches := bfrangePattern.FindAllStringSubmatch(cmapData, -1)
	for _, match := range bfrangeMatches {
		fd.parseBfrangeBlock(match[1])
	}
}

// parseBfcharBlock parses a bfchar block
func (fd *FontDecoder) parseBfcharBlock(block string) {
	// Match pairs of hex values: <srcCode> <dstUnicode>
	linePattern := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	matches := linePattern.FindAllStringSubmatch(block, -1)
	for _, match := range matches {
		srcCode, err := strconv.ParseInt(match[1], 16, 32)
		if err != nil {
			continue
		}
		// Destination can be multi-byte Unicode
		dstHex := match[2]
		unicodeRunes := hexToUnicodeRunes(dstHex)
		if len(unicodeRunes) == 1 {
			fd.toUnicode[int(srcCode)] = unicodeRunes[0]
		} else if len(unicodeRunes) > 1 {
			// For ligatures/sequences, store the first rune
			// TODO: Handle multi-rune mappings properly
			fd.toUnicode[int(srcCode)] = unicodeRunes[0]
		}
	}
}

// parseBfrangeBlock parses a bfrange block
func (fd *FontDecoder) parseBfrangeBlock(block string) {
	// Match range definitions: <srcLo> <srcHi> <dstStart> or <srcLo> <srcHi> [array]
	// Simple range: <0020> <007E> <0020>
	simpleRangePattern := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	simpleMatches := simpleRangePattern.FindAllStringSubmatch(block, -1)
	for _, match := range simpleMatches {
		srcLo, err1 := strconv.ParseInt(match[1], 16, 32)
		srcHi, err2 := strconv.ParseInt(match[2], 16, 32)
		dstStart, err3 := strconv.ParseInt(match[3], 16, 32)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		for i := srcLo; i <= srcHi; i++ {
			fd.toUnicode[int(i)] = rune(dstStart + (i - srcLo))
		}
	}

	// Array range: <srcLo> <srcHi> [<dst1> <dst2> ...]
	arrayRangePattern := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\[([^\]]+)\]`)
	arrayMatches := arrayRangePattern.FindAllStringSubmatch(block, -1)
	for _, match := range arrayMatches {
		srcLo, err1 := strconv.ParseInt(match[1], 16, 32)
		srcHi, err2 := strconv.ParseInt(match[2], 16, 32)
		if err1 != nil || err2 != nil {
			continue
		}
		// Parse array elements
		arrayHexPattern := regexp.MustCompile(`<([0-9A-Fa-f]+)>`)
		arrayElements := arrayHexPattern.FindAllStringSubmatch(match[3], -1)
		for i, elem := range arrayElements {
			code := int(srcLo) + i
			if code > int(srcHi) {
				break
			}
			unicodeRunes := hexToUnicodeRunes(elem[1])
			if len(unicodeRunes) > 0 {
				fd.toUnicode[code] = unicodeRunes[0]
			}
		}
	}
}

// hexToUnicodeRunes converts a hex string to Unicode runes
// Each pair of hex digits represents a byte in UTF-16BE
func hexToUnicodeRunes(hex string) []rune {
	var runes []rune

	// Handle UTF-16BE encoding (most common in PDFs)
	if len(hex) == 4 {
		// Single BMP character
		if val, err := strconv.ParseInt(hex, 16, 32); err == nil {
			runes = append(runes, rune(val))
		}
	} else if len(hex) == 2 {
		// Single byte - treat as code point
		if val, err := strconv.ParseInt(hex, 16, 32); err == nil {
			runes = append(runes, rune(val))
		}
	} else if len(hex) > 4 {
		// Multi-character or surrogate pair
		for i := 0; i < len(hex); i += 4 {
			end := i + 4
			if end > len(hex) {
				end = len(hex)
			}
			if val, err := strconv.ParseInt(hex[i:end], 16, 32); err == nil {
				runes = append(runes, rune(val))
			}
		}
	}

	return runes
}

// ParseDifferencesArray parses a PDF Differences array
// Format: [ code1 /name1 /name2 code2 /name3 ... ]
func (fd *FontDecoder) ParseDifferencesArray(diffStr string) {
	// Remove brackets
	diffStr = strings.TrimSpace(diffStr)
	diffStr = strings.TrimPrefix(diffStr, "[")
	diffStr = strings.TrimSuffix(diffStr, "]")

	// Parse tokens
	tokens := strings.Fields(diffStr)
	currentCode := 0

	for _, token := range tokens {
		if strings.HasPrefix(token, "/") {
			// Glyph name
			glyphName := strings.TrimPrefix(token, "/")
			if unicode := glyphNameToUnicode(glyphName); unicode != 0 {
				fd.differences[currentCode] = unicode
			}
			currentCode++
		} else {
			// Character code
			if code, err := strconv.Atoi(token); err == nil {
				currentCode = code
			}
		}
	}
}

// glyphNameToUnicode converts a PostScript glyph name to Unicode
func glyphNameToUnicode(name string) rune {
	// Check Adobe Glyph List first
	if unicode, ok := adobeGlyphList[name]; ok {
		return unicode
	}

	// Handle uniXXXX format (e.g., uni0041 = A)
	if strings.HasPrefix(name, "uni") && len(name) == 7 {
		if val, err := strconv.ParseInt(name[3:], 16, 32); err == nil {
			return rune(val)
		}
	}

	// Handle uXXXX or uXXXXX format
	if strings.HasPrefix(name, "u") && (len(name) == 5 || len(name) == 6) {
		if val, err := strconv.ParseInt(name[1:], 16, 32); err == nil {
			return rune(val)
		}
	}

	return 0
}

// Standard PDF encodings

// WinAnsiEncoding (Windows Latin 1)
var winAnsiEncoding = map[int]rune{
	// Control characters and basic ASCII are same as Latin-1
	32: ' ', 33: '!', 34: '"', 35: '#', 36: '$', 37: '%', 38: '&', 39: '\'',
	40: '(', 41: ')', 42: '*', 43: '+', 44: ',', 45: '-', 46: '.', 47: '/',
	48: '0', 49: '1', 50: '2', 51: '3', 52: '4', 53: '5', 54: '6', 55: '7',
	56: '8', 57: '9', 58: ':', 59: ';', 60: '<', 61: '=', 62: '>', 63: '?',
	64: '@', 65: 'A', 66: 'B', 67: 'C', 68: 'D', 69: 'E', 70: 'F', 71: 'G',
	72: 'H', 73: 'I', 74: 'J', 75: 'K', 76: 'L', 77: 'M', 78: 'N', 79: 'O',
	80: 'P', 81: 'Q', 82: 'R', 83: 'S', 84: 'T', 85: 'U', 86: 'V', 87: 'W',
	88: 'X', 89: 'Y', 90: 'Z', 91: '[', 92: '\\', 93: ']', 94: '^', 95: '_',
	96: '`', 97: 'a', 98: 'b', 99: 'c', 100: 'd', 101: 'e', 102: 'f', 103: 'g',
	104: 'h', 105: 'i', 106: 'j', 107: 'k', 108: 'l', 109: 'm', 110: 'n', 111: 'o',
	112: 'p', 113: 'q', 114: 'r', 115: 's', 116: 't', 117: 'u', 118: 'v', 119: 'w',
	120: 'x', 121: 'y', 122: 'z', 123: '{', 124: '|', 125: '}', 126: '~',
	// Extended characters (128-159 are Windows-specific)
	128: '\u20AC', // Euro sign
	130: '\u201A', // Single low-9 quotation mark
	131: '\u0192', // Latin small letter f with hook
	132: '\u201E', // Double low-9 quotation mark
	133: '\u2026', // Horizontal ellipsis
	134: '\u2020', // Dagger
	135: '\u2021', // Double dagger
	136: '\u02C6', // Modifier letter circumflex accent
	137: '\u2030', // Per mille sign
	138: '\u0160', // Latin capital letter S with caron
	139: '\u2039', // Single left-pointing angle quotation mark
	140: '\u0152', // Latin capital ligature OE
	142: '\u017D', // Latin capital letter Z with caron
	145: '\u2018', // Left single quotation mark
	146: '\u2019', // Right single quotation mark
	147: '\u201C', // Left double quotation mark
	148: '\u201D', // Right double quotation mark
	149: '\u2022', // Bullet
	150: '\u2013', // En dash
	151: '\u2014', // Em dash
	152: '\u02DC', // Small tilde
	153: '\u2122', // Trade mark sign
	154: '\u0161', // Latin small letter s with caron
	155: '\u203A', // Single right-pointing angle quotation mark
	156: '\u0153', // Latin small ligature oe
	158: '\u017E', // Latin small letter z with caron
	159: '\u0178', // Latin capital letter Y with diaeresis
	// 160-255 are same as Latin-1
	160: '\u00A0', 161: '\u00A1', 162: '\u00A2', 163: '\u00A3', 164: '\u00A4',
	165: '\u00A5', 166: '\u00A6', 167: '\u00A7', 168: '\u00A8', 169: '\u00A9',
	170: '\u00AA', 171: '\u00AB', 172: '\u00AC', 173: '\u00AD', 174: '\u00AE',
	175: '\u00AF', 176: '\u00B0', 177: '\u00B1', 178: '\u00B2', 179: '\u00B3',
	180: '\u00B4', 181: '\u00B5', 182: '\u00B6', 183: '\u00B7', 184: '\u00B8',
	185: '\u00B9', 186: '\u00BA', 187: '\u00BB', 188: '\u00BC', 189: '\u00BD',
	190: '\u00BE', 191: '\u00BF', 192: '\u00C0', 193: '\u00C1', 194: '\u00C2',
	195: '\u00C3', 196: '\u00C4', 197: '\u00C5', 198: '\u00C6', 199: '\u00C7',
	200: '\u00C8', 201: '\u00C9', 202: '\u00CA', 203: '\u00CB', 204: '\u00CC',
	205: '\u00CD', 206: '\u00CE', 207: '\u00CF', 208: '\u00D0', 209: '\u00D1',
	210: '\u00D2', 211: '\u00D3', 212: '\u00D4', 213: '\u00D5', 214: '\u00D6',
	215: '\u00D7', 216: '\u00D8', 217: '\u00D9', 218: '\u00DA', 219: '\u00DB',
	220: '\u00DC', 221: '\u00DD', 222: '\u00DE', 223: '\u00DF', 224: '\u00E0',
	225: '\u00E1', 226: '\u00E2', 227: '\u00E3', 228: '\u00E4', 229: '\u00E5',
	230: '\u00E6', 231: '\u00E7', 232: '\u00E8', 233: '\u00E9', 234: '\u00EA',
	235: '\u00EB', 236: '\u00EC', 237: '\u00ED', 238: '\u00EE', 239: '\u00EF',
	240: '\u00F0', 241: '\u00F1', 242: '\u00F2', 243: '\u00F3', 244: '\u00F4',
	245: '\u00F5', 246: '\u00F6', 247: '\u00F7', 248: '\u00F8', 249: '\u00F9',
	250: '\u00FA', 251: '\u00FB', 252: '\u00FC', 253: '\u00FD', 254: '\u00FE',
	255: '\u00FF',
}

// MacRomanEncoding (Mac OS Roman)
var macRomanEncoding = map[int]rune{
	// Basic ASCII same as standard
	32: ' ', 33: '!', 34: '"', 35: '#', 36: '$', 37: '%', 38: '&', 39: '\'',
	40: '(', 41: ')', 42: '*', 43: '+', 44: ',', 45: '-', 46: '.', 47: '/',
	48: '0', 49: '1', 50: '2', 51: '3', 52: '4', 53: '5', 54: '6', 55: '7',
	56: '8', 57: '9', 58: ':', 59: ';', 60: '<', 61: '=', 62: '>', 63: '?',
	64: '@', 65: 'A', 66: 'B', 67: 'C', 68: 'D', 69: 'E', 70: 'F', 71: 'G',
	72: 'H', 73: 'I', 74: 'J', 75: 'K', 76: 'L', 77: 'M', 78: 'N', 79: 'O',
	80: 'P', 81: 'Q', 82: 'R', 83: 'S', 84: 'T', 85: 'U', 86: 'V', 87: 'W',
	88: 'X', 89: 'Y', 90: 'Z', 91: '[', 92: '\\', 93: ']', 94: '^', 95: '_',
	96: '`', 97: 'a', 98: 'b', 99: 'c', 100: 'd', 101: 'e', 102: 'f', 103: 'g',
	104: 'h', 105: 'i', 106: 'j', 107: 'k', 108: 'l', 109: 'm', 110: 'n', 111: 'o',
	112: 'p', 113: 'q', 114: 'r', 115: 's', 116: 't', 117: 'u', 118: 'v', 119: 'w',
	120: 'x', 121: 'y', 122: 'z', 123: '{', 124: '|', 125: '}', 126: '~',
	// Mac-specific extended characters
	128: '\u00C4', // A with diaeresis
	129: '\u00C5', // A with ring above
	130: '\u00C7', // C with cedilla
	131: '\u00C9', // E with acute
	132: '\u00D1', // N with tilde
	133: '\u00D6', // O with diaeresis
	134: '\u00DC', // U with diaeresis
	135: '\u00E1', // a with acute
	136: '\u00E0', // a with grave
	137: '\u00E2', // a with circumflex
	138: '\u00E4', // a with diaeresis
	139: '\u00E3', // a with tilde
	140: '\u00E5', // a with ring above
	141: '\u00E7', // c with cedilla
	142: '\u00E9', // e with acute
	143: '\u00E8', // e with grave
	144: '\u00EA', // e with circumflex
	145: '\u00EB', // e with diaeresis
	146: '\u00ED', // i with acute
	147: '\u00EC', // i with grave
	148: '\u00EE', // i with circumflex
	149: '\u00EF', // i with diaeresis
	150: '\u00F1', // n with tilde
	151: '\u00F3', // o with acute
	152: '\u00F2', // o with grave
	153: '\u00F4', // o with circumflex
	154: '\u00F6', // o with diaeresis
	155: '\u00F5', // o with tilde
	156: '\u00FA', // u with acute
	157: '\u00F9', // u with grave
	158: '\u00FB', // u with circumflex
	159: '\u00FC', // u with diaeresis
	160: '\u2020', // Dagger
	161: '\u00B0', // Degree sign
	162: '\u00A2', // Cent sign
	163: '\u00A3', // Pound sign
	164: '\u00A7', // Section sign
	165: '\u2022', // Bullet
	166: '\u00B6', // Pilcrow sign
	167: '\u00DF', // German sharp s
	168: '\u00AE', // Registered sign
	169: '\u00A9', // Copyright sign
	170: '\u2122', // Trade mark sign
	171: '\u00B4', // Acute accent
	172: '\u00A8', // Diaeresis
	173: '\u2260', // Not equal to
	174: '\u00C6', // AE
	175: '\u00D8', // O with stroke
	176: '\u221E', // Infinity
	177: '\u00B1', // Plus-minus sign
	178: '\u2264', // Less-than or equal to
	179: '\u2265', // Greater-than or equal to
	180: '\u00A5', // Yen sign
	181: '\u00B5', // Micro sign
	182: '\u2202', // Partial differential
	183: '\u2211', // N-ary summation
	184: '\u220F', // N-ary product
	185: '\u03C0', // Greek small letter pi
	186: '\u222B', // Integral
	187: '\u00AA', // Feminine ordinal indicator
	188: '\u00BA', // Masculine ordinal indicator
	189: '\u03A9', // Greek capital letter omega
	190: '\u00E6', // ae
	191: '\u00F8', // o with stroke
	192: '\u00BF', // Inverted question mark
	193: '\u00A1', // Inverted exclamation mark
	194: '\u00AC', // Not sign
	195: '\u221A', // Square root
	196: '\u0192', // Latin small letter f with hook
	197: '\u2248', // Almost equal to
	198: '\u2206', // Increment
	199: '\u00AB', // Left-pointing double angle quotation mark
	200: '\u00BB', // Right-pointing double angle quotation mark
	201: '\u2026', // Horizontal ellipsis
	202: '\u00A0', // No-break space
	203: '\u00C0', // A with grave
	204: '\u00C3', // A with tilde
	205: '\u00D5', // O with tilde
	206: '\u0152', // OE
	207: '\u0153', // oe
	208: '\u2013', // En dash
	209: '\u2014', // Em dash
	210: '\u201C', // Left double quotation mark
	211: '\u201D', // Right double quotation mark
	212: '\u2018', // Left single quotation mark
	213: '\u2019', // Right single quotation mark
	214: '\u00F7', // Division sign
	215: '\u25CA', // Lozenge
	216: '\u00FF', // y with diaeresis
	217: '\u0178', // Y with diaeresis
	218: '\u2044', // Fraction slash
	219: '\u20AC', // Euro sign
	220: '\u2039', // Single left-pointing angle quotation mark
	221: '\u203A', // Single right-pointing angle quotation mark
	222: '\uFB01', // fi ligature
	223: '\uFB02', // fl ligature
	224: '\u2021', // Double dagger
	225: '\u00B7', // Middle dot
	226: '\u201A', // Single low-9 quotation mark
	227: '\u201E', // Double low-9 quotation mark
	228: '\u2030', // Per mille sign
	229: '\u00C2', // A with circumflex
	230: '\u00CA', // E with circumflex
	231: '\u00C1', // A with acute
	232: '\u00CB', // E with diaeresis
	233: '\u00C8', // E with grave
	234: '\u00CD', // I with acute
	235: '\u00CE', // I with circumflex
	236: '\u00CF', // I with diaeresis
	237: '\u00CC', // I with grave
	238: '\u00D3', // O with acute
	239: '\u00D4', // O with circumflex
	240: '\uF8FF', // Apple logo (Private Use Area)
	241: '\u00D2', // O with grave
	242: '\u00DA', // U with acute
	243: '\u00DB', // U with circumflex
	244: '\u00D9', // U with grave
	245: '\u0131', // Dotless i
	246: '\u02C6', // Modifier letter circumflex accent
	247: '\u02DC', // Small tilde
	248: '\u00AF', // Macron
	249: '\u02D8', // Breve
	250: '\u02D9', // Dot above
	251: '\u02DA', // Ring above
	252: '\u00B8', // Cedilla
	253: '\u02DD', // Double acute accent
	254: '\u02DB', // Ogonek
	255: '\u02C7', // Caron
}

// StandardEncoding (PostScript Standard Encoding)
var standardEncoding = map[int]rune{
	32: ' ', 33: '!', 34: '"', 35: '#', 36: '$', 37: '%', 38: '&', 39: '\u2019', // quoteright
	40: '(', 41: ')', 42: '*', 43: '+', 44: ',', 45: '-', 46: '.', 47: '/',
	48: '0', 49: '1', 50: '2', 51: '3', 52: '4', 53: '5', 54: '6', 55: '7',
	56: '8', 57: '9', 58: ':', 59: ';', 60: '<', 61: '=', 62: '>', 63: '?',
	64: '@', 65: 'A', 66: 'B', 67: 'C', 68: 'D', 69: 'E', 70: 'F', 71: 'G',
	72: 'H', 73: 'I', 74: 'J', 75: 'K', 76: 'L', 77: 'M', 78: 'N', 79: 'O',
	80: 'P', 81: 'Q', 82: 'R', 83: 'S', 84: 'T', 85: 'U', 86: 'V', 87: 'W',
	88: 'X', 89: 'Y', 90: 'Z', 91: '[', 92: '\\', 93: ']', 94: '^', 95: '_',
	96: '\u2018', // quoteleft
	97: 'a', 98: 'b', 99: 'c', 100: 'd', 101: 'e', 102: 'f', 103: 'g',
	104: 'h', 105: 'i', 106: 'j', 107: 'k', 108: 'l', 109: 'm', 110: 'n', 111: 'o',
	112: 'p', 113: 'q', 114: 'r', 115: 's', 116: 't', 117: 'u', 118: 'v', 119: 'w',
	120: 'x', 121: 'y', 122: 'z', 123: '{', 124: '|', 125: '}', 126: '~',
	161: '\u00A1', // exclamdown
	162: '\u00A2', // cent
	163: '\u00A3', // sterling
	164: '\u2044', // fraction
	165: '\u00A5', // yen
	166: '\u0192', // florin
	167: '\u00A7', // section
	168: '\u00A4', // currency
	169: '\u0027', // quotesingle
	170: '\u201C', // quotedblleft
	171: '\u00AB', // guillemotleft
	172: '\u2039', // guilsinglleft
	173: '\u203A', // guilsinglright
	174: '\uFB01', // fi
	175: '\uFB02', // fl
	177: '\u2013', // endash
	178: '\u2020', // dagger
	179: '\u2021', // daggerdbl
	180: '\u00B7', // periodcentered
	182: '\u00B6', // paragraph
	183: '\u2022', // bullet
	184: '\u201A', // quotesinglbase
	185: '\u201E', // quotedblbase
	186: '\u201D', // quotedblright
	187: '\u00BB', // guillemotright
	188: '\u2026', // ellipsis
	189: '\u2030', // perthousand
	191: '\u00BF', // questiondown
	193: '\u0060', // grave
	194: '\u00B4', // acute
	195: '\u02C6', // circumflex
	196: '\u02DC', // tilde
	197: '\u00AF', // macron
	198: '\u02D8', // breve
	199: '\u02D9', // dotaccent
	200: '\u00A8', // dieresis
	202: '\u02DA', // ring
	203: '\u00B8', // cedilla
	205: '\u02DD', // hungarumlaut
	206: '\u02DB', // ogonek
	207: '\u02C7', // caron
	208: '\u2014', // emdash
	225: '\u00C6', // AE
	227: '\u00AA', // ordfeminine
	232: '\u0141', // Lslash
	233: '\u00D8', // Oslash
	234: '\u0152', // OE
	235: '\u00BA', // ordmasculine
	241: '\u00E6', // ae
	245: '\u0131', // dotlessi
	248: '\u0142', // lslash
	249: '\u00F8', // oslash
	250: '\u0153', // oe
	251: '\u00DF', // germandbls
}

// MacExpertEncoding (Mac Expert Encoding - for expert fonts)
var macExpertEncoding = map[int]rune{
	32:  ' ',
	33:  '\uF721', // exclamsmall
	34:  '\uF6F8', // Hungarumlautsmall
	35:  '\uF7A2', // centoldstyle
	36:  '\uF724', // dollaroldstyle
	37:  '\uF6E4', // dollarsuperior
	38:  '\uF726', // ampersandsmall
	39:  '\uF7B4', // Acutesmall
	40:  '\u207D', // parenleftsuperior
	41:  '\u207E', // parenrightsuperior
	42:  '\u2025', // twodotenleader
	43:  '\u2024', // onedotenleader
	44:  ',',
	45:  '-',
	46:  '.',
	47:  '\u2044', // fraction
	48:  '\uF730', // zerooldstyle
	49:  '\uF731', // oneoldstyle
	50:  '\uF732', // twooldstyle
	51:  '\uF733', // threeoldstyle
	52:  '\uF734', // fouroldstyle
	53:  '\uF735', // fiveoldstyle
	54:  '\uF736', // sixoldstyle
	55:  '\uF737', // sevenoldstyle
	56:  '\uF738', // eightoldstyle
	57:  '\uF739', // nineoldstyle
	58:  ':',
	59:  ';',
	61:  '\uF6DE', // threequartersemdash
	63:  '\uF73F', // questionsmall
	68:  '\uF7F0', // Ethsmall
	71:  '\u00BC', // onequarter
	72:  '\u00BD', // onehalf
	73:  '\u00BE', // threequarters
	74:  '\u215B', // oneeighth
	75:  '\u215C', // threeeighths
	76:  '\u215D', // fiveeighths
	77:  '\u215E', // seveneighths
	78:  '\u2153', // onethird
	79:  '\u2154', // twothirds
	80:  '\uFB00', // ff
	81:  '\uFB01', // fi
	82:  '\uFB02', // fl
	83:  '\uFB03', // ffi
	84:  '\uFB04', // ffl
	85:  '\u2070', // zerosuperior
	86:  '\u2074', // foursuperior
	87:  '\u2075', // fivesuperior
	88:  '\u2076', // sixsuperior
	89:  '\u2077', // sevensuperior
	90:  '\u2078', // eightsuperior
	91:  '\u2079', // ninesuperior
	92:  '\u2080', // zeroinferior
	93:  '\u2081', // oneinferior
	94:  '\u2082', // twoinferior
	95:  '\u2083', // threeinferior
	96:  '\u2084', // fourinferior
	97:  '\u2085', // fiveinferior
	98:  '\u2086', // sixinferior
	99:  '\u2087', // seveninferior
	100: '\u2088', // eightinferior
	101: '\u2089', // nineinferior
	102: '\u00B9', // onesuperior
	103: '\u00B2', // twosuperior
	104: '\u00B3', // threesuperior
	105: '\u2071', // isuperior
	106: '\u207F', // nsuperior
	109: '\u2219', // centinferior
	110: '\uF6E0', // dollarinferior
	111: '\uF6E1', // periodinferior
	112: '\uF6E2', // commainferior
}

// Adobe Glyph List (partial - most common glyphs)
var adobeGlyphList = map[string]rune{
	"space":          ' ',
	"exclam":         '!',
	"quotedbl":       '"',
	"numbersign":     '#',
	"dollar":         '$',
	"percent":        '%',
	"ampersand":      '&',
	"quotesingle":    '\'',
	"parenleft":      '(',
	"parenright":     ')',
	"asterisk":       '*',
	"plus":           '+',
	"comma":          ',',
	"hyphen":         '-',
	"period":         '.',
	"slash":          '/',
	"zero":           '0',
	"one":            '1',
	"two":            '2',
	"three":          '3',
	"four":           '4',
	"five":           '5',
	"six":            '6',
	"seven":          '7',
	"eight":          '8',
	"nine":           '9',
	"colon":          ':',
	"semicolon":      ';',
	"less":           '<',
	"equal":          '=',
	"greater":        '>',
	"question":       '?',
	"at":             '@',
	"A":              'A',
	"B":              'B',
	"C":              'C',
	"D":              'D',
	"E":              'E',
	"F":              'F',
	"G":              'G',
	"H":              'H',
	"I":              'I',
	"J":              'J',
	"K":              'K',
	"L":              'L',
	"M":              'M',
	"N":              'N',
	"O":              'O',
	"P":              'P',
	"Q":              'Q',
	"R":              'R',
	"S":              'S',
	"T":              'T',
	"U":              'U',
	"V":              'V',
	"W":              'W',
	"X":              'X',
	"Y":              'Y',
	"Z":              'Z',
	"bracketleft":    '[',
	"backslash":      '\\',
	"bracketright":   ']',
	"asciicircum":    '^',
	"underscore":     '_',
	"grave":          '`',
	"a":              'a',
	"b":              'b',
	"c":              'c',
	"d":              'd',
	"e":              'e',
	"f":              'f',
	"g":              'g',
	"h":              'h',
	"i":              'i',
	"j":              'j',
	"k":              'k',
	"l":              'l',
	"m":              'm',
	"n":              'n',
	"o":              'o',
	"p":              'p',
	"q":              'q',
	"r":              'r',
	"s":              's',
	"t":              't',
	"u":              'u',
	"v":              'v',
	"w":              'w',
	"x":              'x',
	"y":              'y',
	"z":              'z',
	"braceleft":      '{',
	"bar":            '|',
	"braceright":     '}',
	"asciitilde":     '~',
	"exclamdown":     '\u00A1',
	"cent":           '\u00A2',
	"sterling":       '\u00A3',
	"currency":       '\u00A4',
	"yen":            '\u00A5',
	"brokenbar":      '\u00A6',
	"section":        '\u00A7',
	"dieresis":       '\u00A8',
	"copyright":      '\u00A9',
	"ordfeminine":    '\u00AA',
	"guillemotleft":  '\u00AB',
	"logicalnot":     '\u00AC',
	"registered":     '\u00AE',
	"macron":         '\u00AF',
	"degree":         '\u00B0',
	"plusminus":      '\u00B1',
	"twosuperior":    '\u00B2',
	"threesuperior":  '\u00B3',
	"acute":          '\u00B4',
	"mu":             '\u00B5',
	"paragraph":      '\u00B6',
	"periodcentered": '\u00B7',
	"cedilla":        '\u00B8',
	"onesuperior":    '\u00B9',
	"ordmasculine":   '\u00BA',
	"guillemotright": '\u00BB',
	"onequarter":     '\u00BC',
	"onehalf":        '\u00BD',
	"threequarters":  '\u00BE',
	"questiondown":   '\u00BF',
	"Agrave":         '\u00C0',
	"Aacute":         '\u00C1',
	"Acircumflex":    '\u00C2',
	"Atilde":         '\u00C3',
	"Adieresis":      '\u00C4',
	"Aring":          '\u00C5',
	"AE":             '\u00C6',
	"Ccedilla":       '\u00C7',
	"Egrave":         '\u00C8',
	"Eacute":         '\u00C9',
	"Ecircumflex":    '\u00CA',
	"Edieresis":      '\u00CB',
	"Igrave":         '\u00CC',
	"Iacute":         '\u00CD',
	"Icircumflex":    '\u00CE',
	"Idieresis":      '\u00CF',
	"Eth":            '\u00D0',
	"Ntilde":         '\u00D1',
	"Ograve":         '\u00D2',
	"Oacute":         '\u00D3',
	"Ocircumflex":    '\u00D4',
	"Otilde":         '\u00D5',
	"Odieresis":      '\u00D6',
	"multiply":       '\u00D7',
	"Oslash":         '\u00D8',
	"Ugrave":         '\u00D9',
	"Uacute":         '\u00DA',
	"Ucircumflex":    '\u00DB',
	"Udieresis":      '\u00DC',
	"Yacute":         '\u00DD',
	"Thorn":          '\u00DE',
	"germandbls":     '\u00DF',
	"agrave":         '\u00E0',
	"aacute":         '\u00E1',
	"acircumflex":    '\u00E2',
	"atilde":         '\u00E3',
	"adieresis":      '\u00E4',
	"aring":          '\u00E5',
	"ae":             '\u00E6',
	"ccedilla":       '\u00E7',
	"egrave":         '\u00E8',
	"eacute":         '\u00E9',
	"ecircumflex":    '\u00EA',
	"edieresis":      '\u00EB',
	"igrave":         '\u00EC',
	"iacute":         '\u00ED',
	"icircumflex":    '\u00EE',
	"idieresis":      '\u00EF',
	"eth":            '\u00F0',
	"ntilde":         '\u00F1',
	"ograve":         '\u00F2',
	"oacute":         '\u00F3',
	"ocircumflex":    '\u00F4',
	"otilde":         '\u00F5',
	"odieresis":      '\u00F6',
	"divide":         '\u00F7',
	"oslash":         '\u00F8',
	"ugrave":         '\u00F9',
	"uacute":         '\u00FA',
	"ucircumflex":    '\u00FB',
	"udieresis":      '\u00FC',
	"yacute":         '\u00FD',
	"thorn":          '\u00FE',
	"ydieresis":      '\u00FF',
	"OE":             '\u0152',
	"oe":             '\u0153',
	"Scaron":         '\u0160',
	"scaron":         '\u0161',
	"Ydieresis":      '\u0178',
	"Zcaron":         '\u017D',
	"zcaron":         '\u017E',
	"florin":         '\u0192',
	"circumflex":     '\u02C6',
	"caron":          '\u02C7',
	"breve":          '\u02D8',
	"dotaccent":      '\u02D9',
	"ring":           '\u02DA',
	"ogonek":         '\u02DB',
	"tilde":          '\u02DC',
	"hungarumlaut":   '\u02DD',
	"endash":         '\u2013',
	"emdash":         '\u2014',
	"quoteleft":      '\u2018',
	"quoteright":     '\u2019',
	"quotesinglbase": '\u201A',
	"quotedblleft":   '\u201C',
	"quotedblright":  '\u201D',
	"quotedblbase":   '\u201E',
	"dagger":         '\u2020',
	"daggerdbl":      '\u2021',
	"bullet":         '\u2022',
	"ellipsis":       '\u2026',
	"perthousand":    '\u2030',
	"guilsinglleft":  '\u2039',
	"guilsinglright": '\u203A',
	"fraction":       '\u2044',
	"Euro":           '\u20AC',
	"trademark":      '\u2122',
	"minus":          '\u2212',
	"fi":             '\uFB01',
	"fl":             '\uFB02',
	"ff":             '\uFB00',
	"ffi":            '\uFB03',
	"ffl":            '\uFB04',
	"dotlessi":       '\u0131',
	"Lslash":         '\u0141',
	"lslash":         '\u0142',
}
