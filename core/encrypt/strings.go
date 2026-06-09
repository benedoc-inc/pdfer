package encrypt

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// DecryptStringsInContent walks raw PDF object content (a non-stream dictionary
// body) and decrypts every hex-string value found. This is the read-side
// complement to the write package's encryptStringsInContent.
//
// Hex strings that are too short to be AES-encrypted (< 32 bytes decoded) or
// that fail decryption are left unchanged, providing backward compatibility
// with PDFs written before string encryption was implemented.
func DecryptStringsInContent(content []byte, objNum, genNum int, enc *types.PDFEncryption) ([]byte, error) {
	if enc == nil {
		return content, nil
	}
	out := make([]byte, 0, len(content))
	i := 0
	for i < len(content) {
		b := content[i]
		switch {
		case b == '%':
			for i < len(content) && content[i] != '\n' && content[i] != '\r' {
				out = append(out, content[i])
				i++
			}
		case b == '<' && i+1 < len(content) && content[i+1] == '<':
			out = append(out, '<', '<')
			i += 2
		case b == '>' && i+1 < len(content) && content[i+1] == '>':
			out = append(out, '>', '>')
			i += 2
		case b == '<':
			end, decoded, err := encParseHexString(content, i)
			if err != nil || len(decoded) < 32 {
				// Too short to be AES-encrypted or malformed — pass through as-is.
				out = append(out, b)
				i++
				continue
			}
			plain, decErr := DecryptObject(decoded, objNum, genNum, enc)
			if decErr != nil {
				// Decryption failed: likely not an encrypted string. Pass through.
				out = append(out, content[i:end]...)
				i = end
				continue
			}
			// Re-encode as PDF literal string so callers see (text) form.
			out = append(out, '(')
			out = encAppendLiteral(out, plain)
			out = append(out, ')')
			i = end
		default:
			out = append(out, b)
			i++
		}
	}
	return out, nil
}

// encParseHexString parses a hex string starting at content[pos] ('<', not '<<').
// Returns (end, decoded bytes, error). end is the index after '>'.
func encParseHexString(content []byte, pos int) (int, []byte, error) {
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
		if !encIsHexDigit(b) {
			return pos, nil, fmt.Errorf("invalid hex char %c", b)
		}
		hexChars = append(hexChars, b)
		i++
	}
	if i >= len(content) {
		return pos, nil, fmt.Errorf("unterminated hex string")
	}
	if len(hexChars)%2 != 0 {
		hexChars = append(hexChars, '0')
	}
	decoded := make([]byte, len(hexChars)/2)
	for k := range decoded {
		decoded[k] = encHexNibble(hexChars[k*2])<<4 | encHexNibble(hexChars[k*2+1])
	}
	return i + 1, decoded, nil
}

func encIsHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
}

func encHexNibble(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	default:
		return b - 'a' + 10
	}
}

// encAppendLiteral appends src as PDF literal string content (between the caller's
// parentheses), escaping special characters.
func encAppendLiteral(dst, src []byte) []byte {
	for _, b := range src {
		switch b {
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '(':
			dst = append(dst, '\\', '(')
		case ')':
			dst = append(dst, '\\', ')')
		case '\\':
			dst = append(dst, '\\', '\\')
		default:
			dst = append(dst, b)
		}
	}
	return dst
}
