package write

import "fmt"

// encryptStringsInContent walks raw PDF object content (a non-stream dictionary
// body) and encrypts every string value found. Literal strings (...) are decoded,
// encrypted, and replaced with hex strings <...>. Pre-existing hex strings <...>
// are decoded, encrypted, and replaced with new hex strings. Dictionary delimiters
// << and >> are passed through unchanged.
func (w *PDFWriter) encryptStringsInContent(content []byte, objNum, genNum int) ([]byte, error) {
	if w.encryptInfo == nil {
		return content, nil
	}
	out := make([]byte, 0, len(content)+32)
	i := 0
	for i < len(content) {
		b := content[i]
		switch {
		case b == '%':
			// Comment: copy through to end of line.
			for i < len(content) && content[i] != '\n' && content[i] != '\r' {
				out = append(out, content[i])
				i++
			}
		case b == '(':
			end, decoded, err := pdfParseLiteralString(content, i)
			if err != nil {
				out = append(out, b)
				i++
				continue
			}
			enc, err := w.encryptStream(decoded, objNum, genNum)
			if err != nil {
				return nil, fmt.Errorf("encrypt string in obj %d: %w", objNum, err)
			}
			out = append(out, '<')
			out = pdfAppendHexUpper(out, enc)
			out = append(out, '>')
			i = end
		case b == '<' && i+1 < len(content) && content[i+1] == '<':
			out = append(out, '<', '<')
			i += 2
		case b == '>' && i+1 < len(content) && content[i+1] == '>':
			out = append(out, '>', '>')
			i += 2
		case b == '<':
			end, decoded, err := pdfParseHexString(content, i)
			if err != nil {
				out = append(out, b)
				i++
				continue
			}
			enc, err := w.encryptStream(decoded, objNum, genNum)
			if err != nil {
				return nil, fmt.Errorf("encrypt hex string in obj %d: %w", objNum, err)
			}
			out = append(out, '<')
			out = pdfAppendHexUpper(out, enc)
			out = append(out, '>')
			i = end
		default:
			out = append(out, b)
			i++
		}
	}
	return out, nil
}

// pdfParseLiteralString parses a literal string starting at content[pos] (which
// must be '(') and returns (end, decoded bytes, error). end is the byte index
// immediately after the closing ')'. Escape sequences are decoded.
func pdfParseLiteralString(content []byte, pos int) (int, []byte, error) {
	if pos >= len(content) || content[pos] != '(' {
		return pos, nil, fmt.Errorf("not a literal string")
	}
	var out []byte
	depth := 1
	i := pos + 1
	for i < len(content) && depth > 0 {
		b := content[i]
		if b == '\\' && i+1 < len(content) {
			next := content[i+1]
			i += 2
			switch next {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, next)
			case '\r':
				if i < len(content) && content[i] == '\n' {
					i++
				}
			case '\n':
				// line continuation — consume, emit nothing
			default:
				if next >= '0' && next <= '7' {
					octal := int(next - '0')
					for k := 0; k < 2 && i < len(content) && content[i] >= '0' && content[i] <= '7'; k++ {
						octal = octal*8 + int(content[i]-'0')
						i++
					}
					out = append(out, byte(octal))
				} else {
					out = append(out, next)
				}
			}
		} else if b == '(' {
			depth++
			out = append(out, b)
			i++
		} else if b == ')' {
			depth--
			if depth > 0 {
				out = append(out, b)
			}
			i++
		} else {
			out = append(out, b)
			i++
		}
	}
	if depth != 0 {
		return pos, nil, fmt.Errorf("unterminated literal string at pos %d", pos)
	}
	return i, out, nil
}

// pdfParseHexString parses a PDF hex string starting at content[pos] ('<', not
// '<<') and returns (end, decoded bytes, error). end is the byte index after '>'.
func pdfParseHexString(content []byte, pos int) (int, []byte, error) {
	if pos >= len(content) || content[pos] != '<' {
		return pos, nil, fmt.Errorf("not a hex string")
	}
	var hexChars []byte
	i := pos + 1
	for i < len(content) && content[i] != '>' {
		b := content[i]
		if b == ' ' || b == '\n' || b == '\r' || b == '\t' {
			i++
			continue
		}
		if !pdfIsHexDigit(b) {
			return pos, nil, fmt.Errorf("invalid hex char %c", b)
		}
		hexChars = append(hexChars, b)
		i++
	}
	if i >= len(content) {
		return pos, nil, fmt.Errorf("unterminated hex string")
	}
	if len(hexChars)%2 != 0 {
		hexChars = append(hexChars, '0') // per PDF spec: pad with trailing zero nibble
	}
	decoded := make([]byte, len(hexChars)/2)
	for k := range decoded {
		decoded[k] = pdfHexNibble(hexChars[k*2])<<4 | pdfHexNibble(hexChars[k*2+1])
	}
	return i + 1, decoded, nil
}

func pdfIsHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}

func pdfHexNibble(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	default: // a-f
		return b - 'a' + 10
	}
}

func pdfAppendHexUpper(dst, src []byte) []byte {
	const h = "0123456789ABCDEF"
	for _, b := range src {
		dst = append(dst, h[b>>4], h[b&0xF])
	}
	return dst
}
