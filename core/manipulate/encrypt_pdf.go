package manipulate

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// EncryptPDF applies AES-128 (V=4, R=4) encryption to an existing unencrypted
// PDF. It is the write-side counterpart to encrypt.DecryptPDF.
//
// userPassword is required to open the document; ownerPassword grants full
// control. Either may be nil or empty (no password for that role).
//
// Returns an error if the input PDF is already encrypted or cannot be parsed.
func EncryptPDF(pdfBytes []byte, userPassword, ownerPassword []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Verbose: verbose})
	if err != nil {
		return nil, fmt.Errorf("encrypt: parse failed: %w", err)
	}
	if pdf.IsEncrypted() {
		return nil, fmt.Errorf("encrypt: pdf is already encrypted; decrypt first")
	}

	enc, fileID, err := encrypt.PrepareEncryption(encrypt.EncryptOptions{
		UserPassword:  userPassword,
		OwnerPassword: ownerPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("encrypt: prepare failed: %w", err)
	}

	w := write.NewPDFWriter()
	w.SetEncryption(enc, fileID)

	// Allocate the /Encrypt dict at maxObjNum+1 to avoid collision with existing
	// object numbers.
	maxNum := 0
	for _, n := range pdf.Objects() {
		if n > maxNum {
			maxNum = n
		}
	}
	encDictObjNum := maxNum + 1
	w.SetObject(encDictObjNum, encrypt.EncryptDictString(enc))
	w.SetEncryptRef(encDictObjNum)

	// Copy all existing objects. Stream objects are stored with separate dict
	// and stream bytes so the writer can AES-encrypt the stream during Bytes().
	for _, n := range pdf.Objects() {
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		dictBytes, streamBytes := encryptSplitContent(body)
		if streamBytes != nil {
			w.SetRawStreamObject(n, dictBytes, streamBytes)
		} else {
			w.SetObject(n, dictBytes)
		}
	}

	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("encrypt: no document catalog in trailer")
	}
	var rootNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}

	return w.Bytes()
}

// encryptStreamKeyword matches the stream keyword between the dictionary's
// closing >> and the stream data: optional whitespace, "stream", then the
// spec-mandated CRLF or LF. Anchoring on >> means a dict ending ">>stream\n"
// (no newline before the keyword) is still recognized as a stream object.
var encryptStreamKeyword = regexp.MustCompile(`>>[ \t\r\n]*stream\r?\n`)

// encryptSplitContent splits GetObjectContent output into the dict bytes and
// raw stream bytes for a stream object. Returns (content, nil) for plain dicts.
func encryptSplitContent(content []byte) (dictBytes, streamBytes []byte) {
	loc := encryptStreamKeyword.FindIndex(content)
	if loc == nil {
		return content, nil
	}
	dict := content[:loc[0]+2] // keep the closing >>
	rest := content[loc[1]:]
	for _, endSep := range [][]byte{
		[]byte("\nendstream"),
		[]byte("endstream"),
	} {
		if j := bytes.Index(rest, endSep); j >= 0 {
			return dict, rest[:j]
		}
	}
	return content, nil
}
