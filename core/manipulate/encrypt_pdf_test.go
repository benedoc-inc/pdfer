package manipulate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// buildPlainPDF builds a one-page PDF with a single text run (no encryption).
func buildPlainPDF(t *testing.T, text string) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fn := page.AddStandardFont("Helvetica")
	page.Content().BeginText().SetFont(fn, 12).SetTextPosition(72, 720).ShowText(text).EndText()
	b.FinalizePage(page)
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

// --- helper unit tests --------------------------------------------------------

func TestEncryptSplitContent_Plain(t *testing.T) {
	content := []byte("<</Type/Page/MediaBox[0 0 612 792]>>")
	d, s := encryptSplitContent(content)
	if !bytes.Equal(d, content) {
		t.Errorf("expected dict to be the whole content")
	}
	if s != nil {
		t.Errorf("expected nil stream for plain dict")
	}
}

func TestEncryptSplitContent_Stream(t *testing.T) {
	raw := "<</Filter/FlateDecode/Length 5>>\nstream\nHello\nendstream"
	d, s := encryptSplitContent([]byte(raw))
	if !strings.Contains(string(d), "/Length") {
		t.Errorf("dict bytes should contain /Length: %q", d)
	}
	if string(s) != "Hello" {
		t.Errorf("stream bytes = %q, want %q", s, "Hello")
	}
}

func TestEncryptSplitContent_StreamCRLF(t *testing.T) {
	raw := "<</Length 5>>\nstream\r\nHello\nendstream"
	_, s := encryptSplitContent([]byte(raw))
	if string(s) != "Hello" {
		t.Errorf("stream bytes = %q, want %q", s, "Hello")
	}
}

// --- integration tests --------------------------------------------------------

func TestEncryptPDF_ParseableAfterEncrypt(t *testing.T) {
	src := buildPlainPDF(t, "hello encrypt")
	out, err := EncryptPDF(src, []byte("user"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	// Parse with the user password — required because the PDF is now encrypted.
	if _, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: []byte("user")}); err != nil {
		t.Fatalf("encrypted PDF not parseable with correct password: %v", err)
	}
}

func TestEncryptPDF_OutputIsEncrypted(t *testing.T) {
	src := buildPlainPDF(t, "hello encrypt")
	out, err := EncryptPDF(src, []byte("secret"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	// Even without a password, the parser reports the PDF as encrypted before
	// attempting decryption — we just need to reach IsEncrypted().
	pdf, parseErr := parse.OpenWithOptions(out, parse.ParseOptions{Password: []byte("secret")})
	if parseErr != nil {
		t.Fatalf("parse: %v", parseErr)
	}
	if !pdf.IsEncrypted() {
		t.Error("output PDF is not marked as encrypted")
	}
}

func TestEncryptPDF_CorrectPasswordAccepted(t *testing.T) {
	src := buildPlainPDF(t, "hello encrypt")
	out, err := EncryptPDF(src, []byte("mypassword"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	if _, err := encrypt.DecryptPDF(out, []byte("mypassword"), false); err != nil {
		t.Fatalf("DecryptPDF with correct password failed: %v", err)
	}
}

func TestEncryptPDF_WrongPasswordRejected(t *testing.T) {
	src := buildPlainPDF(t, "hello encrypt")
	out, err := EncryptPDF(src, []byte("secret"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	if _, err := encrypt.DecryptPDF(out, []byte("wrong"), false); err == nil {
		t.Error("expected error with wrong password, got nil")
	}
}

func TestEncryptPDF_RoundTrip(t *testing.T) {
	src := buildPlainPDF(t, "round trip content")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	// Verify the correct password is accepted.
	if _, err := encrypt.DecryptPDF(encrypted, []byte("pass"), false); err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	// Verify the encrypted PDF is still structurally intact by parsing with password.
	if _, err := parse.OpenWithOptions(encrypted, parse.ParseOptions{Password: []byte("pass")}); err != nil {
		t.Fatalf("encrypted PDF not parseable with password: %v", err)
	}
}

func TestEncryptPDF_EmptyUserPassword(t *testing.T) {
	src := buildPlainPDF(t, "no open password")
	out, err := EncryptPDF(src, nil, []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	if _, err := encrypt.DecryptPDF(out, nil, false); err != nil {
		t.Fatalf("DecryptPDF with empty password failed: %v", err)
	}
}

func TestEncryptPDF_AlreadyEncryptedReturnsError(t *testing.T) {
	src := buildPlainPDF(t, "will be encrypted")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("first EncryptPDF: %v", err)
	}
	_, err = EncryptPDF(encrypted, []byte("pass2"), []byte("owner"), false)
	if err == nil {
		t.Error("expected error when encrypting already-encrypted PDF")
	}
}

func TestEncryptPDF_SingleEOF(t *testing.T) {
	src := buildPlainPDF(t, "eof test")
	out, err := EncryptPDF(src, []byte("p"), []byte("o"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	eofCount := strings.Count(string(out), "%%EOF")
	if eofCount != 1 {
		t.Errorf("expected exactly one EOF marker in encrypted output, got %d", eofCount)
	}
}

func TestEncryptPDF_TwoPage(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	p1 := b.AddPage(write.PageSizeLetter)
	fn1 := p1.AddStandardFont("Helvetica")
	p1.Content().BeginText().SetFont(fn1, 12).SetTextPosition(72, 720).ShowText("page one").EndText()
	b.FinalizePage(p1)
	p2 := b.AddPage(write.PageSizeLetter)
	fn2 := p2.AddStandardFont("Helvetica")
	p2.Content().BeginText().SetFont(fn2, 12).SetTextPosition(72, 720).ShowText("page two").EndText()
	b.FinalizePage(p2)
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	out, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	if _, err := parse.OpenWithOptions(out, parse.ParseOptions{Password: []byte("pass")}); err != nil {
		t.Fatalf("two-page encrypted PDF not parseable: %v", err)
	}
}
