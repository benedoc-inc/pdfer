package parse

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// testEncRC4 returns a V2 (RC4) string-encrypting context whose /Encrypt
// dictionary lives at object 2.
func testEncRC4() *types.PDFEncryption {
	return &types.PDFEncryption{
		V:             2,
		R:             3,
		KeyLength:     16,
		EncryptKey:    bytes.Repeat([]byte{0xCD}, 16),
		EncryptObjNum: 2,
	}
}

// buildSingleObjectPDF wraps one object body in a minimal PDF with a
// traditional xref table so FindObjectLocation can resolve it.
func buildSingleObjectPDF(objNum int, body []byte) []byte {
	var pdf []byte
	pdf = append(pdf, "%PDF-1.4\n"...)
	objOffset := len(pdf)
	pdf = append(pdf, fmt.Sprintf("%d 0 obj\n", objNum)...)
	pdf = append(pdf, body...)
	pdf = append(pdf, "\nendobj\n"...)
	xrefOffset := len(pdf)
	pdf = append(pdf, fmt.Sprintf("xref\n%d 1\n%010d 00000 n \n", objNum, objOffset)...)
	pdf = append(pdf, fmt.Sprintf("trailer\n<</Size %d>>\nstartxref\n%d\n%%%%EOF\n", objNum+1, xrefOffset)...)
	return pdf
}

// TestGetObjectContent_EncryptDictExempt verifies that string decryption skips
// the /Encrypt dictionary itself: its /O and /U values are stored unencrypted
// (ISO 32000-1 §7.6.5), and RC4 "decryption" cannot fail, so without the
// exemption they come back deterministically garbled — and manipulate paths
// that round-trip every object then write the garbage back.
func TestGetObjectContent_EncryptDictExempt(t *testing.T) {
	enc := testEncRC4()

	// 32 paren-safe raw bytes standing in for a real /O hash.
	oValue := make([]byte, 32)
	for i := range oValue {
		oValue[i] = byte(0xA0 + i)
	}
	encryptDict := fmt.Sprintf("<</Filter/Standard/V 2/R 3/Length 128/P -44/O (%s)/U (%s)>>", oValue, oValue)

	pdf := buildSingleObjectPDF(2, []byte(encryptDict))

	got, err := GetObjectContent(pdf, 2, enc, false)
	if err != nil {
		t.Fatalf("GetObjectContent(encrypt dict): %v", err)
	}
	if !bytes.Contains(got, oValue) {
		t.Errorf("/O value of the /Encrypt dictionary was modified:\n got %q\nwant to contain %q", got, oValue)
	}
}

// TestGetObjectContent_OtherDictStringsStillDecrypt is the positive control:
// the exemption must not suppress string decryption for ordinary objects.
func TestGetObjectContent_OtherDictStringsStillDecrypt(t *testing.T) {
	enc := testEncRC4()

	body := []byte("<</Title (secret title)>>")
	ct, err := encrypt.EncryptStringsInContent(body, 5, 0, enc)
	if err != nil {
		t.Fatalf("EncryptStringsInContent: %v", err)
	}
	if bytes.Contains(ct, []byte("secret title")) {
		t.Fatal("test setup: string was not encrypted")
	}

	pdf := buildSingleObjectPDF(5, ct)

	got, err := GetObjectContent(pdf, 5, enc, false)
	if err != nil {
		t.Fatalf("GetObjectContent: %v", err)
	}
	if !bytes.Contains(got, []byte("(secret title)")) {
		t.Errorf("ordinary dict string was not decrypted; got %q", got)
	}
}
