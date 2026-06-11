package write

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// buildEncryptedDoc builds a one-page PDF with a doc-info /Title, applies the
// given writer tweaks, encrypts with SetupEncryptionWithPasswords, and returns
// the output bytes plus the Info object number.
func buildEncryptedDoc(t *testing.T, title string, password []byte, tweak func(w *PDFWriter)) ([]byte, int) {
	t.Helper()
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	fn := p.AddStandardFont("Helvetica")
	p.Content().BeginText().SetFont(fn, 12).SetTextPosition(72, 720).ShowText("body").EndText()
	b.FinalizePage(p)
	infoNum := b.Writer().SetMetadata(&types.DocumentMetadata{Title: title})
	if tweak != nil {
		tweak(b.Writer())
	}
	if _, err := b.Writer().SetupEncryptionWithPasswords(password, []byte("owner"), -3904, true); err != nil {
		t.Fatalf("setup encryption: %v", err)
	}
	out, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return out, infoNum
}

// TestStreamDictStrings_Encrypted is the regression test for the formatDictionary
// gap: string values inside a *stream object's* dictionary must be encrypted like
// any other string, and must decrypt on read.
func TestStreamDictStrings_Encrypted(t *testing.T) {
	const secret = "Stream Dict Secret Value"
	password := []byte("pw")
	var streamObjNum int

	out, _ := buildEncryptedDoc(t, "irrelevant", password, func(w *PDFWriter) {
		streamObjNum = w.AddStreamObject(Dictionary{
			"/Type": "/EmbeddedFile",
			"/Desc": secret,
		}, []byte("payload bytes"), false)
	})

	if bytes.Contains(out, []byte(secret)) {
		t.Error("stream-dict string appears in cleartext in encrypted output")
	}

	pdf, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("parse encrypted output: %v", err)
	}
	body, err := pdf.GetObjectContent(streamObjNum)
	if err != nil {
		t.Fatalf("GetObjectContent(%d): %v", streamObjNum, err)
	}
	if !bytes.Contains(body, []byte(secret)) {
		t.Errorf("stream-dict string did not decrypt on read; got %q", body)
	}
}

// TestObjectStreams_WithEncryption verifies the object-stream exclusions: member
// strings are not individually encrypted (the objstm is encrypted as a unit, so
// they survive the round-trip), and the /Encrypt dictionary stays a direct,
// readable object outside the object stream.
func TestObjectStreams_WithEncryption(t *testing.T) {
	const title = "ObjStm Member Title"
	password := []byte("pw")

	out, infoNum := buildEncryptedDoc(t, title, password, func(w *PDFWriter) {
		w.UseObjectStream(true)
	})

	// The Info dict lives inside the compressed+encrypted objstm: no cleartext.
	if bytes.Contains(out, []byte(title)) {
		t.Error("objstm member string appears in cleartext")
	}
	// The /Encrypt dictionary must be directly readable (not inside the objstm).
	if !bytes.Contains(out, []byte("/Filter /Standard")) {
		t.Error("/Encrypt dictionary is not a direct object")
	}

	pdf, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: password})
	if err != nil {
		t.Fatalf("parse encrypted objstm output: %v", err)
	}
	body, err := pdf.GetObjectContent(infoNum)
	if err != nil {
		t.Fatalf("GetObjectContent(%d): %v", infoNum, err)
	}
	if !bytes.Contains(body, []byte(title)) {
		t.Errorf("objstm member string did not round-trip; got %q", body)
	}
}

// TestEncryptDictDeclarationMatchesBehavior_V5 pins the single-source-of-truth
// property for the V5 helper path: the declared /StrF matches whether strings
// were actually encrypted.
func TestEncryptDictDeclarationMatchesBehavior_V5(t *testing.T) {
	const title = "V5 Declaration Title"
	out, _ := buildEncryptedDoc(t, title, []byte("pw"), nil)

	if !bytes.Contains(out, []byte("/StrF /StdCF")) {
		t.Errorf("V5 /Encrypt dict does not declare /StrF /StdCF")
	}
	if bytes.Contains(out, []byte(fmt.Sprintf("(%s)", title))) {
		t.Error("cleartext title found despite /StrF /StdCF declaration")
	}
}
