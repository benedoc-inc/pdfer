package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"fmt"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// EncryptObject encrypts a stream or string payload belonging to object
// (objNum, genNum) using the document's encryption parameters. It is the exact
// inverse of DecryptObject:
//
//   - V1/V2: RC4 with the per-object MD5-derived key.
//   - V4/V5: AES-CBC with a random 16-byte IV prepended and PKCS#7 padding.
//
// When enc is nil or carries no master key the data is returned unchanged, so
// callers can use it unconditionally on both encrypted and plain documents.
//
// Note: the V5 (AES-256) branch mirrors DecryptObject's per-object MD5+sAlT
// derivation rather than the ISO 32000-2 scheme (which uses the file key
// directly). This round-trips within pdfer but is not interoperable with
// spec-compliant readers for genuine V5 documents; pdfer's own EncryptPDF only
// emits V4 (AES-128), which is spec-correct here.
func EncryptObject(data []byte, objNum, genNum int, enc *types.PDFEncryption) ([]byte, error) {
	if enc == nil || len(enc.EncryptKey) == 0 {
		return data, nil
	}

	// Per-object key: key[:n] + pack1(objNum) + pack2(genNum), MD5-hashed.
	pack1 := []byte{byte(objNum), byte(objNum >> 8), byte(objNum >> 16)}
	pack2 := []byte{byte(genNum), byte(genNum >> 8)}

	n := 5
	if enc.V > 1 {
		n = enc.KeyLength // already in bytes
	}
	if n > len(enc.EncryptKey) {
		return nil, fmt.Errorf("encrypt: key length %d exceeds master key %d", n, len(enc.EncryptKey))
	}

	keyData := make([]byte, n+5)
	copy(keyData, enc.EncryptKey[:n])
	copy(keyData[n:], pack1)
	copy(keyData[n+3:], pack2)

	keyHash := md5.New()
	keyHash.Write(keyData)

	switch enc.V {
	case 1, 2:
		rc4Key := keyHash.Sum(nil)[:min(n+5, 16)]
		c, err := rc4.NewCipher(rc4Key)
		if err != nil {
			return nil, err
		}
		out := make([]byte, len(data))
		c.XORKeyStream(out, data)
		return out, nil

	case 4, 5:
		keyHash.Write([]byte{0x73, 0x41, 0x6C, 0x54}) // "sAlT"
		aesKey := keyHash.Sum(nil)[:min(n+5, 16)]

		iv := make([]byte, 16)
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}

		// PKCS#7 pad to a multiple of the 16-byte block size. When the data is
		// already block-aligned a full padding block is added (padLen == 16),
		// matching DecryptObject's unpad logic.
		padLen := 16 - (len(data) % 16)
		padded := make([]byte, len(data)+padLen)
		copy(padded, data)
		for i := len(data); i < len(padded); i++ {
			padded[i] = byte(padLen)
		}

		block, err := aes.NewCipher(aesKey)
		if err != nil {
			return nil, err
		}
		out := make([]byte, 16+len(padded))
		copy(out, iv)
		cipher.NewCBCEncrypter(block, iv).CryptBlocks(out[16:], padded)
		return out, nil
	}

	return data, nil
}

// EncryptStringsInContent walks a (non-stream) dictionary body and encrypts every
// string value it contains, emitting each as a hex string <...> of the encrypted
// bytes. Both literal (...) and hex <...> source strings are handled; dictionary
// delimiters << >> and comments are passed through untouched. It is the write-side
// complement of DecryptStringsInContent.
//
// When enc is nil, or the document's string crypt filter is /Identity (strings
// stored in the clear), the content is returned unchanged.
func EncryptStringsInContent(content []byte, objNum, genNum int, enc *types.PDFEncryption) ([]byte, error) {
	if enc == nil || enc.StrFIdentity {
		return content, nil
	}
	out := make([]byte, 0, len(content)+32)
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
			decoded, end := parsePDFLiteralStringBytes(content, i+1)
			enc2, err := EncryptObject(decoded, objNum, genNum, enc)
			if err != nil {
				return nil, fmt.Errorf("encrypt literal string in obj %d: %w", objNum, err)
			}
			out = append(out, '<')
			out = encAppendHexUpper(out, enc2)
			out = append(out, '>')
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
				out = append(out, b)
				i++
				continue
			}
			enc2, encErr := EncryptObject(decoded, objNum, genNum, enc)
			if encErr != nil {
				return nil, fmt.Errorf("encrypt hex string in obj %d: %w", objNum, encErr)
			}
			out = append(out, '<')
			out = encAppendHexUpper(out, enc2)
			out = append(out, '>')
			i = end
		default:
			out = append(out, b)
			i++
		}
	}
	return out, nil
}

func encAppendHexUpper(dst, src []byte) []byte {
	const h = "0123456789ABCDEF"
	for _, b := range src {
		dst = append(dst, h[b>>4], h[b&0xF])
	}
	return dst
}
