package parse

import (
	"github.com/benedoc-inc/pdfer/v2/types"
)

// ParseXRefStream parses a PDF cross-reference stream and returns a map of object
// numbers to byte offsets. Type-2 entries (objects stored inside object streams)
// cannot be represented as byte offsets and are therefore not included in the
// returned map. Callers that need full Type-2 support should use ParseXRefStreamFull
// directly.
func ParseXRefStream(pdfBytes []byte, startXRef int64) (map[int]int64, error) {
	return ParseXRefStreamWithEncryption(pdfBytes, startXRef, nil, false)
}

// ParseXRefStreamWithEncryption parses a PDF cross-reference stream with optional
// decryption context. It delegates to ParseXRefStreamFull so that both Type-1 and
// Type-2 entries are parsed using a single, consistent implementation.
//
// Note: xref streams are never encrypted (they must be readable before the
// encryption dictionary can be located), so encryptInfo is accepted for API
// compatibility but is not used here.
//
// Type-2 entries (compressed objects in object streams) cannot be represented as
// byte offsets. They are parsed by ParseXRefStreamFull but are not present in the
// returned map[int]int64. Callers needing full Type-2 support should call
// ParseXRefStreamFull directly.
func ParseXRefStreamWithEncryption(pdfBytes []byte, startXRef int64, encryptInfo *types.PDFEncryption, verbose bool) (map[int]int64, error) {
	result, err := ParseXRefStreamFull(pdfBytes, startXRef, verbose)
	if err != nil {
		return nil, err
	}
	// Type-2 entries (ObjectStreams) cannot be represented as byte offsets.
	// Callers needing full Type-2 support should use ParseXRefStreamFull directly.
	return result.Objects, nil
}
