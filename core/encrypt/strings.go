package encrypt

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/types"
)

// DecryptStringsInContent walks raw PDF object content (a non-stream dictionary
// body) and decrypts every string value found — both hex <...> and literal (...)
// forms — re-encoding each as a literal string. It is the read-side complement
// of EncryptStringsInContent.
//
// The document's /StrF crypt filter is honored deterministically: when it is
// /Identity (strings stored in the clear) the content is returned unchanged.
// There is no heuristic fallback — in a non-Identity document every string is
// ciphertext by definition.
func DecryptStringsInContent(content []byte, objNum, genNum int, enc *types.PDFEncryption) ([]byte, error) {
	if enc == nil || enc.StrFIdentity {
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
		case b == '(':
			decoded, end, ok := parsePDFLiteralStringBytes(content, i+1)
			if !ok {
				// No balanced ')': not a real string — pass the byte through.
				out = append(out, b)
				i++
				continue
			}
			plain, decErr := DecryptObject(decoded, objNum, genNum, enc)
			if decErr != nil {
				// Malformed ciphertext — pass through as-is.
				out = append(out, content[i:end]...)
				i = end
				continue
			}
			out = append(out, '(')
			out = encAppendLiteral(out, plain)
			out = append(out, ')')
			i = end
		case b == '<' && i+1 < len(content) && content[i+1] == '<':
			out = append(out, '<', '<')
			i += 2
		case b == '>' && i+1 < len(content) && content[i+1] == '>':
			out = append(out, '>', '>')
			i += 2
		case b == '<':
			end, decoded, err := encParseHexString(content, i)
			if err != nil {
				// Malformed hex string — pass through as-is.
				out = append(out, b)
				i++
				continue
			}
			plain, decErr := DecryptObject(decoded, objNum, genNum, enc)
			if decErr != nil {
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
