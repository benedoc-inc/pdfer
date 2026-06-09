package manipulate

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"testing"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// buildTitledPDF builds a one-page PDF with a document-info /Title string.
func buildTitledPDF(t *testing.T, title string) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fn := page.AddStandardFont("Helvetica")
	page.Content().BeginText().SetFont(fn, 12).SetTextPosition(72, 720).ShowText("body text").EndText()
	b.FinalizePage(page)
	b.Writer().SetMetadata(&types.DocumentMetadata{Title: title})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

// TestEncryptPDF_StrFConformance checks the encrypted output the way a
// spec-compliant external reader would: the /Encrypt dict must declare
// /StrF /StdCF, string values must be ciphertext in the raw bytes, and that
// ciphertext must decrypt with the standard V4 per-object algorithm.
func TestEncryptPDF_StrFConformance(t *testing.T) {
	const title = "Conformance Title 42"
	src := buildTitledPDF(t, title)
	password := []byte("user-pw")

	out, err := EncryptPDF(src, password, []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	// Declaration: strings are encrypted with the standard crypt filter.
	if !bytes.Contains(out, []byte("/StrF/StdCF")) {
		t.Error("/Encrypt dict does not declare /StrF/StdCF")
	}
	// Behavior: the title must not survive in cleartext anywhere.
	if bytes.Contains(out, []byte(title)) {
		t.Error("cleartext /Title found in encrypted output")
	}

	// Decrypt the /Title value exactly as a conformant reader would: parse the
	// /Encrypt dict, derive the file key from the password and /ID, then apply
	// the standard V4 per-object algorithm to the string in the Info object.
	enc, err := encrypt.ParseEncryptionDictionary(out, false)
	if err != nil {
		t.Fatalf("ParseEncryptionDictionary: %v", err)
	}
	if enc.StrFIdentity {
		t.Fatal("parsed encryption reports /StrF /Identity for StdCF output")
	}
	fileID := encrypt.ExtractFileID(out, false)
	key, err := encrypt.DeriveEncryptionKey(password, enc, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey: %v", err)
	}
	enc.EncryptKey = key

	infoNumMatch := regexp.MustCompile(`/Info (\d+) 0 R`).FindSubmatch(out)
	if infoNumMatch == nil {
		t.Fatal("no /Info reference in trailer")
	}
	var infoNum int
	fmt.Sscanf(string(infoNumMatch[1]), "%d", &infoNum)

	infoBody := regexp.MustCompile(fmt.Sprintf(`(?s)\n%d 0 obj\n(.*?)endobj`, infoNum)).FindSubmatch(out)
	if infoBody == nil {
		t.Fatalf("Info object %d not found in raw bytes", infoNum)
	}
	titleHex := regexp.MustCompile(`/Title\s*<([0-9A-Fa-f]+)>`).FindSubmatch(infoBody[1])
	if titleHex == nil {
		t.Fatalf("no hex /Title ciphertext in Info object; body: %q", infoBody[1])
	}
	ct, err := hex.DecodeString(string(titleHex[1]))
	if err != nil {
		t.Fatalf("decode title hex: %v", err)
	}
	plain, err := encrypt.DecryptObject(ct, infoNum, 0, enc)
	if err != nil {
		t.Fatalf("DecryptObject on /Title ciphertext: %v", err)
	}
	if string(plain) != title {
		t.Errorf("decrypted /Title = %q, want %q", plain, title)
	}

	// And pdfer's own reader must agree.
	pdf, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("parse encrypted output: %v", err)
	}
	got, err := pdf.GetObjectContent(infoNum)
	if err != nil {
		t.Fatalf("GetObjectContent(%d): %v", infoNum, err)
	}
	if !bytes.Contains(got, []byte(title)) {
		t.Errorf("pdfer reader did not decrypt /Title; got %q", got)
	}
}

// TestEncryptPDF_IdentityDeclaresAndStoresCleartext exercises the /StrF /Identity
// policy end-to-end: when StrFIdentity is set, the /Encrypt dict declares
// Identity, string values stay cleartext in the raw bytes, and the reader
// passes them through untouched.
func TestEncryptPDF_IdentityDeclaresAndStoresCleartext(t *testing.T) {
	const title = "Identity Cleartext Title"
	src := buildTitledPDF(t, title)
	password := []byte("user-pw")

	// Replicate EncryptPDF with the Identity policy (no public knob for it).
	pdf, err := parse.Open(src)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	enc, fileID, err := encrypt.PrepareEncryption(encrypt.EncryptOptions{UserPassword: password})
	if err != nil {
		t.Fatalf("PrepareEncryption: %v", err)
	}
	enc.StrFIdentity = true

	w := write.NewPDFWriter()
	w.SetEncryption(enc, fileID)
	maxNum := 0
	for _, n := range pdf.Objects() {
		if n > maxNum {
			maxNum = n
		}
	}
	encDictObjNum := maxNum + 1
	w.SetObject(encDictObjNum, encrypt.EncryptDictString(enc))
	w.SetEncryptRef(encDictObjNum)
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
	var rootNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}
	out, err := w.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	if !bytes.Contains(out, []byte("/StrF/Identity")) {
		t.Error("/Encrypt dict does not declare /StrF/Identity")
	}
	if !bytes.Contains(out, []byte(title)) {
		t.Error("cleartext /Title missing: strings were encrypted despite Identity")
	}

	// The reader must honor Identity and not mangle the cleartext strings.
	reparsed, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("parse Identity output: %v", err)
	}
	var infoNum int
	fmt.Sscanf(reparsed.Trailer().InfoRef, "%d", &infoNum)
	got, err := reparsed.GetObjectContent(infoNum)
	if err != nil {
		t.Fatalf("GetObjectContent(%d): %v", infoNum, err)
	}
	if !bytes.Contains(got, []byte(title)) {
		t.Errorf("reader mangled cleartext /Title in Identity document; got %q", got)
	}
}
