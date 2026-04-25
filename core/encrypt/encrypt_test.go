package encrypt_test

import (
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/write"
)

func buildEncryptedPDF(t *testing.T, userPwd, ownerPwd []byte) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	page.Content().
		BeginText().
		SetFont(page.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("Encrypted document").
		EndText()
	builder.FinalizePage(page)
	builder.SetPassword(userPwd, ownerPwd)
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}
	return b
}

func TestEncryption_RoundTrip(t *testing.T) {
	userPwd := []byte("hello")
	ownerPwd := []byte("owner")
	pdfBytes := buildEncryptedPDF(t, userPwd, ownerPwd)

	// Verify the password is accepted.
	if _, err := encrypt.DecryptPDF(pdfBytes, userPwd, false); err != nil {
		t.Fatalf("DecryptPDF failed: %v", err)
	}

	// ExtractContent handles decryption internally via the password.
	doc, err := extract.ExtractContent(pdfBytes, userPwd, false)
	if err != nil {
		t.Fatalf("ExtractContent failed: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(doc.Pages))
	}
}

func TestEncryption_WrongPasswordRejected(t *testing.T) {
	pdfBytes := buildEncryptedPDF(t, []byte("secret"), []byte("owner"))

	_, err := encrypt.DecryptPDF(pdfBytes, []byte("wrong"), false)
	if err == nil {
		t.Fatal("expected error with wrong password, got nil")
	}
}

func TestEncryption_EmptyUserPassword(t *testing.T) {
	// No user password — should open without a password.
	pdfBytes := buildEncryptedPDF(t, nil, []byte("owner"))

	if _, err := encrypt.DecryptPDF(pdfBytes, nil, false); err != nil {
		t.Fatalf("DecryptPDF with empty password failed: %v", err)
	}
	doc, err := extract.ExtractContent(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractContent failed: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(doc.Pages))
	}
}

func TestEncryption_PrepareEncryption(t *testing.T) {
	opts := encrypt.EncryptOptions{
		UserPassword:  []byte("test"),
		OwnerPassword: []byte("owner"),
	}
	enc, fileID, err := encrypt.PrepareEncryption(opts)
	if err != nil {
		t.Fatalf("PrepareEncryption failed: %v", err)
	}
	if enc.V != 4 {
		t.Errorf("expected V=4, got %d", enc.V)
	}
	if enc.R != 4 {
		t.Errorf("expected R=4, got %d", enc.R)
	}
	if len(enc.EncryptKey) != 16 {
		t.Errorf("expected 16-byte key, got %d", len(enc.EncryptKey))
	}
	if len(enc.O) != 32 {
		t.Errorf("expected 32-byte /O, got %d", len(enc.O))
	}
	if len(enc.U) != 32 {
		t.Errorf("expected 32-byte /U, got %d", len(enc.U))
	}
	if len(fileID) != 16 {
		t.Errorf("expected 16-byte fileID, got %d", len(fileID))
	}
}
