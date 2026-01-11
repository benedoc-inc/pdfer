package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/writer"
)

// TestE2E_AES256_Decrypt tests decrypting a real AES-256 (V5/R5/R6) encrypted PDF
// This test requires a test PDF file to be present in tests/resources/
// You can create one using: qpdf --encrypt user-password owner-password 256 -- test.pdf test_aes256.pdf
func TestE2E_AES256_Decrypt(t *testing.T) {
	testPDFPath := getTestResourcePath("test_aes256.pdf")

	// Check if test PDF exists
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s. Create it using: qpdf --encrypt testpass ownerpass 256 -- test.pdf test_aes256.pdf", testPDFPath)
	}

	// Read the encrypted PDF
	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	t.Logf("Loaded encrypted PDF: %d bytes", len(pdfBytes))

	// Parse encryption dictionary
	encrypt, err := encryption.ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("Failed to parse encryption dictionary: %v", err)
	}

	// Verify it's V5/R5/R6
	if encrypt.V != 5 || encrypt.R < 5 {
		t.Skipf("PDF is not AES-256 (V5/R5/R6). Got V=%d, R=%d. This test requires AES-256 encryption.", encrypt.V, encrypt.R)
	}

	t.Logf("PDF encryption: V=%d, R=%d, KeyLength=%d bytes", encrypt.V, encrypt.R, encrypt.KeyLength)

	// Verify UE and OE are present
	if len(encrypt.UE) == 0 {
		t.Error("UE (encrypted user key) should be present for V5")
	}
	if len(encrypt.OE) == 0 {
		t.Error("OE (encrypted owner key) should be present for V5")
	}
	if len(encrypt.U) < 48 {
		t.Errorf("U value should be at least 48 bytes for V5, got %d", len(encrypt.U))
	}

	t.Logf("Encryption parameters: U=%d bytes, UE=%d bytes, OE=%d bytes", len(encrypt.U), len(encrypt.UE), len(encrypt.OE))

	// Try to decrypt with user password
	userPassword := []byte("testpass") // Default password from qpdf command
	decryptedBytes, decryptInfo, err := encryption.DecryptPDF(pdfBytes, userPassword, false)
	if err != nil {
		// Try empty password
		decryptedBytes, decryptInfo, err = encryption.DecryptPDF(pdfBytes, []byte(""), false)
		if err != nil {
			t.Fatalf("Failed to decrypt PDF with user password: %v", err)
		}
		t.Logf("Decrypted with empty password")
	} else {
		t.Logf("Decrypted with user password")
	}

	if decryptInfo == nil {
		t.Fatal("DecryptInfo should not be nil after successful decryption")
	}

	if len(decryptInfo.EncryptKey) != 32 {
		t.Errorf("Encryption key should be 32 bytes for AES-256, got %d", len(decryptInfo.EncryptKey))
	}

	t.Logf("Successfully decrypted PDF: %d bytes", len(decryptedBytes))

	// Verify decrypted PDF is valid
	if !bytes.HasPrefix(decryptedBytes, []byte("%PDF-")) {
		t.Error("Decrypted PDF should start with %PDF-")
	}

	// Try to parse the decrypted PDF
	pdf, err := parser.Open(decryptedBytes)
	if err != nil {
		t.Fatalf("Failed to parse decrypted PDF: %v", err)
	}

	t.Logf("Successfully parsed decrypted PDF: version=%s, objects=%d", pdf.Version(), pdf.ObjectCount())

	// Verify PDF is not encrypted anymore (in the decrypted bytes)
	if bytes.Contains(decryptedBytes, []byte("/Encrypt")) {
		t.Log("Note: Decrypted PDF still contains /Encrypt reference (this is normal)")
	}
}

// TestE2E_AES256_CreateAndDecrypt creates an AES-256 encrypted PDF using qpdf and then decrypts it
// This test requires qpdf to be installed: brew install qpdf (macOS) or apt-get install qpdf (Linux)
func TestE2E_AES256_CreateAndDecrypt(t *testing.T) {
	// Check if qpdf is available
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("qpdf not found. Install with: brew install qpdf (macOS) or apt-get install qpdf (Linux)")
	}

	// Create a simple unencrypted PDF first
	builder := writer.NewSimplePDFBuilder()
	page := builder.AddPage(writer.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fontName, 24).
		SetTextPosition(72, 720).
		ShowText("AES-256 Test PDF").
		EndText()
	builder.FinalizePage(page)

	unencryptedPDF, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Write temporary unencrypted PDF
	tempDir := filepath.Join("tests", "resources", "temp")
	os.MkdirAll(tempDir, 0755)
	tempInput := filepath.Join(tempDir, "input.pdf")
	tempEncrypted := filepath.Join(tempDir, "encrypted_aes256.pdf")

	if err := os.WriteFile(tempInput, unencryptedPDF, 0644); err != nil {
		t.Fatalf("Failed to write temp PDF: %v", err)
	}
	defer os.Remove(tempInput)
	defer os.Remove(tempEncrypted)

	// Encrypt with qpdf using AES-256 (V5/R5)
	// qpdf --encrypt user-password owner-password 256 -- input.pdf output.pdf
	cmd := exec.Command("qpdf", "--encrypt", "testpass", "ownerpass", "256", "--", tempInput, tempEncrypted)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to encrypt PDF with qpdf: %v", err)
	}

	t.Logf("Created AES-256 encrypted PDF: %s", tempEncrypted)

	// Read the encrypted PDF
	encryptedBytes, err := os.ReadFile(tempEncrypted)
	if err != nil {
		t.Fatalf("Failed to read encrypted PDF: %v", err)
	}

	// Verify encryption parameters
	encrypt, err := encryption.ParseEncryptionDictionary(encryptedBytes, false)
	if err != nil {
		t.Fatalf("Failed to parse encryption dictionary: %v", err)
	}

	if encrypt.V != 5 || encrypt.R < 5 {
		t.Errorf("Expected V=5, R>=5, got V=%d, R=%d", encrypt.V, encrypt.R)
	}

	if encrypt.KeyLength != 32 {
		t.Errorf("Expected KeyLength=32 for AES-256, got %d", encrypt.KeyLength)
	}

	// Decrypt with user password
	decryptedBytes, decryptInfo, err := encryption.DecryptPDF(encryptedBytes, []byte("testpass"), false)
	if err != nil {
		t.Fatalf("Failed to decrypt PDF: %v", err)
	}

	if decryptInfo == nil {
		t.Fatal("DecryptInfo should not be nil")
	}

	if len(decryptInfo.EncryptKey) != 32 {
		t.Errorf("Expected 32-byte encryption key, got %d bytes", len(decryptInfo.EncryptKey))
	}

	// Verify decrypted content
	if !bytes.HasPrefix(decryptedBytes, []byte("%PDF-")) {
		t.Error("Decrypted PDF should start with %PDF-")
	}

	// Verify we can parse it
	pdf, err := parser.Open(decryptedBytes)
	if err != nil {
		t.Fatalf("Failed to parse decrypted PDF: %v", err)
	}

	t.Logf("Successfully decrypted and parsed AES-256 PDF: version=%s, objects=%d", pdf.Version(), pdf.ObjectCount())

	// Also test owner password
	decryptedBytes2, _, err := encryption.DecryptPDF(encryptedBytes, []byte("ownerpass"), false)
	if err != nil {
		t.Fatalf("Failed to decrypt with owner password: %v", err)
	}

	if !bytes.Equal(decryptedBytes, decryptedBytes2) {
		t.Error("Decryption with user and owner passwords should produce same result")
	}
}

// TestE2E_AES256_VerifyUValue tests U value verification specifically
// This test verifies that password validation works correctly for AES-256
func TestE2E_AES256_VerifyUValue(t *testing.T) {
	testPDFPath := getTestResourcePath("test_aes256.pdf")

	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s. Create it using: qpdf --encrypt testpass ownerpass 256 -- test.pdf test_aes256.pdf", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	encrypt, err := encryption.ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("Failed to parse encryption dictionary: %v", err)
	}

	if encrypt.V != 5 || encrypt.R < 5 {
		t.Skip("PDF is not AES-256")
	}

	// Test with correct password - should succeed
	userPassword := []byte("testpass")
	_, decryptInfo, err := encryption.DecryptPDF(pdfBytes, userPassword, false)
	if err != nil {
		// Try empty password
		_, decryptInfo, err = encryption.DecryptPDF(pdfBytes, []byte(""), false)
		if err != nil {
			t.Fatalf("Failed to decrypt with correct password: %v", err)
		}
	}

	if decryptInfo == nil || len(decryptInfo.EncryptKey) != 32 {
		t.Error("Decryption should succeed with correct password and produce 32-byte key")
	}

	// Test with wrong password - should fail
	_, _, err = encryption.DecryptPDF(pdfBytes, []byte("wrongpass"), false)
	if err == nil {
		t.Error("Decryption should fail with wrong password")
	}
}
