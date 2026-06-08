package manipulate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/parse"
)

func TestDecryptPDF_AlreadyPlaintext(t *testing.T) {
	src := buildPlainPDF(t, "no encryption")
	out, err := DecryptPDF(src, nil, false)
	if err != nil {
		t.Fatalf("DecryptPDF on plaintext: %v", err)
	}
	if !bytes.Equal(out, src) {
		t.Error("unencrypted PDF should be returned unchanged")
	}
}

func TestDecryptPDF_WrongPasswordRejected(t *testing.T) {
	src := buildPlainPDF(t, "secret content")
	encrypted, err := EncryptPDF(src, []byte("correct"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	_, err = DecryptPDF(encrypted, []byte("wrong"), false)
	if err == nil {
		t.Error("expected error with wrong password, got nil")
	}
}

func TestDecryptPDF_OutputIsNotEncrypted(t *testing.T) {
	src := buildPlainPDF(t, "hello")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("pass"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	pdf, err := parse.Open(decrypted)
	if err != nil {
		t.Fatalf("decrypted PDF not parseable: %v", err)
	}
	if pdf.IsEncrypted() {
		t.Error("decrypted PDF still reports IsEncrypted() = true")
	}
}

func TestDecryptPDF_NoEncryptDictInOutput(t *testing.T) {
	src := buildPlainPDF(t, "hello")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("pass"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	if bytes.Contains(decrypted, []byte("/Encrypt")) {
		t.Error("decrypted PDF still contains /Encrypt")
	}
}

func TestDecryptPDF_ParseableWithoutPassword(t *testing.T) {
	src := buildPlainPDF(t, "hello")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("pass"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	// Must be openable with no password at all.
	if _, err := parse.Open(decrypted); err != nil {
		t.Fatalf("decrypted PDF requires a password to open: %v", err)
	}
}

func TestDecryptPDF_RoundTrip(t *testing.T) {
	src := buildPlainPDF(t, "round trip")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("pass"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	// Re-encrypt the decrypted output — should work without error.
	reEncrypted, err := EncryptPDF(decrypted, []byte("new"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("re-EncryptPDF: %v", err)
	}
	if _, err := encrypt.DecryptPDF(reEncrypted, []byte("new"), false); err != nil {
		t.Fatalf("re-encrypted PDF not openable: %v", err)
	}
}

func TestDecryptPDF_EmptyUserPassword(t *testing.T) {
	src := buildPlainPDF(t, "no open password")
	encrypted, err := EncryptPDF(src, nil, []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, nil, false)
	if err != nil {
		t.Fatalf("DecryptPDF with empty password: %v", err)
	}
	if _, err := parse.Open(decrypted); err != nil {
		t.Fatalf("decrypted PDF not parseable: %v", err)
	}
}

func TestDecryptPDF_SingleEOF(t *testing.T) {
	src := buildPlainPDF(t, "eof")
	encrypted, err := EncryptPDF(src, []byte("p"), []byte("o"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("p"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	eofCount := strings.Count(string(decrypted), "%%EOF")
	if eofCount != 1 {
		t.Errorf("expected exactly one EOF marker, got %d", eofCount)
	}
}

func TestDecryptPDF_StructurePreserved(t *testing.T) {
	src := buildPlainPDF(t, "structure check")
	encrypted, err := EncryptPDF(src, []byte("pass"), []byte("owner"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}
	decrypted, err := DecryptPDF(encrypted, []byte("pass"), false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	pdf, err := parse.Open(decrypted)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("decrypted PDF has no objects")
	}
}
