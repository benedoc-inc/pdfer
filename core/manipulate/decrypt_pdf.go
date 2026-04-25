package manipulate

import (
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// DecryptPDF produces a fully decrypted, plaintext PDF from an encrypted one.
// It verifies the password, decrypts every stream and string in the document,
// removes the /Encrypt dict and all encryption metadata, and returns clean bytes
// that can be opened by any PDF tool without a password.
//
// Returns an error if the password is wrong or the PDF cannot be parsed.
// If the PDF is already unencrypted the original bytes are returned unchanged.
func DecryptPDF(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	if !pdf.IsEncrypted() {
		return pdfBytes, nil
	}

	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("decrypt: no document catalog in trailer")
	}

	var rootNum, encryptObjNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)
	if trailer.EncryptRef != "" {
		fmt.Sscanf(trailer.EncryptRef, "%d", &encryptObjNum)
	}

	w := write.NewPDFWriter()

	for _, n := range pdf.Objects() {
		if n == encryptObjNum {
			continue // drop the /Encrypt dict from the output
		}
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		if n == rootNum {
			body = decryptRemoveEncryptRef(body)
		}
		dictBytes, streamBytes := encryptSplitContent(body)
		if streamBytes != nil {
			w.SetRawStreamObject(n, dictBytes, streamBytes)
		} else {
			w.SetObject(n, dictBytes)
		}
	}

	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}

	return w.Bytes()
}

var decryptEncryptRE = regexp.MustCompile(`/Encrypt\s+\d+\s+\d+\s+R`)

func decryptRemoveEncryptRef(catalogBytes []byte) []byte {
	return decryptEncryptRE.ReplaceAll(catalogBytes, nil)
}
