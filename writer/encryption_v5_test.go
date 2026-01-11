package writer

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/encryption"
)

func TestSetupAES256Encryption(t *testing.T) {
	userPassword := []byte("testpass")
	ownerPassword := []byte("ownerpass")
	fileID := make([]byte, 16)
	rand.Read(fileID)

	encrypt, err := SetupAES256Encryption(userPassword, ownerPassword, fileID, -3904, true)
	if err != nil {
		t.Fatalf("SetupAES256Encryption() error = %v", err)
	}

	// Verify encryption parameters
	if encrypt.V != 5 {
		t.Errorf("V = %d, want 5", encrypt.V)
	}
	if encrypt.R != 5 {
		t.Errorf("R = %d, want 5", encrypt.R)
	}
	if encrypt.KeyLength != 32 {
		t.Errorf("KeyLength = %d, want 32", encrypt.KeyLength)
	}
	if len(encrypt.U) != 48 {
		t.Errorf("U length = %d, want 48", len(encrypt.U))
	}
	if len(encrypt.O) != 48 {
		t.Errorf("O length = %d, want 48", len(encrypt.O))
	}
	if len(encrypt.UE) == 0 {
		t.Error("UE should not be empty")
	}
	if len(encrypt.OE) == 0 {
		t.Error("OE should not be empty")
	}
	if len(encrypt.EncryptKey) != 32 {
		t.Errorf("EncryptKey length = %d, want 32", len(encrypt.EncryptKey))
	}
}

func TestCreateEncryptionDictionary(t *testing.T) {
	userPassword := []byte("testpass")
	ownerPassword := []byte("ownerpass")
	fileID := make([]byte, 16)
	rand.Read(fileID)

	encrypt, err := SetupAES256Encryption(userPassword, ownerPassword, fileID, -3904, true)
	if err != nil {
		t.Fatalf("SetupAES256Encryption() error = %v", err)
	}

	dict := CreateEncryptionDictionary(encrypt)
	dictStr := string(dict)

	// Verify dictionary contains required fields
	if !bytes.Contains(dict, []byte("/Filter /Standard")) {
		t.Error("Dictionary should contain /Filter /Standard")
	}
	if !bytes.Contains(dict, []byte("/V 5")) {
		t.Error("Dictionary should contain /V 5")
	}
	if !bytes.Contains(dict, []byte("/R 5")) {
		t.Error("Dictionary should contain /R 5")
	}
	if !bytes.Contains(dict, []byte("/Length 256")) {
		t.Error("Dictionary should contain /Length 256")
	}
	// Check for hex format (we use hex strings for binary data)
	if !bytes.Contains(dict, []byte("/U <")) {
		t.Error("Dictionary should contain /U <hex>")
	}
	if !bytes.Contains(dict, []byte("/O <")) {
		t.Error("Dictionary should contain /O <hex>")
	}
	if !bytes.Contains(dict, []byte("/UE <")) {
		t.Error("Dictionary should contain /UE <hex>")
	}
	if !bytes.Contains(dict, []byte("/OE <")) {
		t.Error("Dictionary should contain /OE <hex>")
	}

	t.Logf("Encryption dictionary: %s", dictStr)
}

func TestWriteAES256EncryptedPDF(t *testing.T) {
	// Create a simple PDF with AES-256 encryption (using same structure as roundtrip test)
	writer := NewPDFWriter()
	writer.SetVersion("1.7")

	userPassword := []byte("testpass")
	ownerPassword := []byte("ownerpass")

	// Setup encryption (before creating pages so fileID is set)
	encryptObjNum, err := writer.SetupEncryptionWithPasswords(userPassword, ownerPassword, -3904, true)
	if err != nil {
		t.Fatalf("SetupEncryptionWithPasswords() error = %v", err)
	}

	if encryptObjNum == 0 {
		t.Error("Encrypt object number should not be 0")
	}

	// Create minimal PDF structure (same as roundtrip test)
	catalogDict := Dictionary{
		"Type":  "/Catalog",
		"Pages": "2 0 R",
	}
	catalogObjNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogObjNum)

	pagesDict := Dictionary{
		"Type":  "/Pages",
		"Kids":  []interface{}{"3 0 R"},
		"Count": 1,
	}
	pagesObjNum := writer.AddObject(writer.formatDictionary(pagesDict))

	pageDict := Dictionary{
		"Type":     "/Page",
		"Parent":   fmt.Sprintf("%d 0 R", pagesObjNum),
		"MediaBox": []interface{}{0, 0, 612, 792},
	}
	_ = writer.AddObject(writer.formatDictionary(pageDict))

	// Generate PDF
	pdfBytes, err := writer.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	t.Logf("Created encrypted PDF: %d bytes", len(pdfBytes))

	// Verify PDF is encrypted
	if !bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		t.Error("PDF should contain /Encrypt in trailer")
	}
	if !bytes.Contains(pdfBytes, []byte("/V 5")) {
		t.Error("PDF should contain /V 5")
	}
	if !bytes.Contains(pdfBytes, []byte("/UE")) {
		t.Error("PDF should contain /UE")
	}
	if !bytes.Contains(pdfBytes, []byte("/OE")) {
		t.Error("PDF should contain /OE")
	}

	// Try to decrypt and parse
	decryptedBytes, decryptInfo, err := encryption.DecryptPDF(pdfBytes, userPassword, true)
	if err != nil {
		t.Fatalf("Failed to decrypt PDF: %v", err)
	}

	if decryptInfo == nil {
		t.Fatal("DecryptInfo should not be nil")
	}

	if len(decryptInfo.EncryptKey) != 32 {
		t.Errorf("Decryption key length = %d, want 32", len(decryptInfo.EncryptKey))
	}

	// Verify decrypted PDF is valid (starts with %PDF-)
	if !bytes.HasPrefix(decryptedBytes, []byte("%PDF-")) {
		t.Error("Decrypted PDF should start with %PDF-")
	}

	// Note: parser.Open will try to decrypt again if it finds /Encrypt
	// For this test, we just verify decryption worked
	t.Logf("Successfully decrypted PDF: %d bytes", len(decryptedBytes))

	// Test owner password
	decryptedBytes2, _, err := encryption.DecryptPDF(pdfBytes, ownerPassword, false)
	if err != nil {
		t.Fatalf("Failed to decrypt with owner password: %v", err)
	}

	if !bytes.Equal(decryptedBytes, decryptedBytes2) {
		t.Error("Decryption with user and owner passwords should produce same result")
	}
}

func TestWriteAES256EncryptedPDF_SimpleBuilder(t *testing.T) {
	// Test using SimplePDFBuilder with encryption
	builder := NewSimplePDFBuilder()

	page := builder.AddPage(PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fontName, 12).
		SetTextPosition(72, 720).
		ShowText("AES-256 Encrypted Test").
		EndText()

	builder.FinalizePage(page)

	// Manually add encryption to the underlying writer
	userPassword := []byte("testpass")
	ownerPassword := []byte("ownerpass")
	_, err := builder.Writer().SetupEncryptionWithPasswords(userPassword, ownerPassword, -3904, true)
	if err != nil {
		t.Fatalf("SetupEncryptionWithPasswords() error = %v", err)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	// Verify encryption
	if !bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		t.Error("PDF should be encrypted")
	}
	if !bytes.Contains(pdfBytes, []byte("/V 5")) {
		t.Error("PDF should use V5 encryption")
	}

	// Decrypt and verify
	decryptedBytes, decryptInfo, err := encryption.DecryptPDF(pdfBytes, userPassword, false)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if decryptInfo == nil || len(decryptInfo.EncryptKey) != 32 {
		t.Error("Decryption should produce 32-byte key")
	}

	if !bytes.HasPrefix(decryptedBytes, []byte("%PDF-")) {
		t.Error("Decrypted PDF should start with %PDF-")
	}

	t.Logf("Successfully created, encrypted, and decrypted PDF: %d bytes", len(decryptedBytes))
}
