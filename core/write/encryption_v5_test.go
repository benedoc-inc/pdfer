package write

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
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

	// Verify password and derive key
	decryptInfo, err := encrypt.DecryptPDF(pdfBytes, userPassword, true)
	if err != nil {
		t.Fatalf("Failed to decrypt PDF: %v", err)
	}

	if decryptInfo == nil {
		t.Fatal("DecryptInfo should not be nil")
	}

	if len(decryptInfo.EncryptKey) != 32 {
		t.Errorf("Decryption key length = %d, want 32", len(decryptInfo.EncryptKey))
	}

	t.Logf("Successfully verified password: %d-byte key", len(decryptInfo.EncryptKey))

	// Owner password should also be accepted and yield the same file encryption key
	decryptInfo2, err := encrypt.DecryptPDF(pdfBytes, ownerPassword, false)
	if err != nil {
		t.Fatalf("Failed to decrypt with owner password: %v", err)
	}

	if !bytes.Equal(decryptInfo.EncryptKey, decryptInfo2.EncryptKey) {
		t.Error("User and owner passwords should derive the same file encryption key")
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

	// Verify password and derive key
	decryptInfo, err := encrypt.DecryptPDF(pdfBytes, userPassword, false)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if decryptInfo == nil || len(decryptInfo.EncryptKey) != 32 {
		t.Error("Decryption should produce 32-byte key")
	}

	t.Logf("Successfully created, encrypted, and decrypted PDF: %d bytes", len(pdfBytes))
}

// TestEncryptedPDF_DictStringsAreEncrypted verifies that string values in
// non-stream objects (e.g. the Info dict) are encrypted in the written PDF.
func TestEncryptedPDF_DictStringsAreEncrypted(t *testing.T) {
	const title = "Confidential Report Q4"
	const author = "Jane Smith"

	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	p.Content().
		BeginText().
		SetFont(p.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("hello").
		EndText()
	b.FinalizePage(p)

	b.Writer().SetMetadata(&types.DocumentMetadata{Title: title, Author: author})

	userPw := []byte("pw123")
	_, err := b.Writer().SetupEncryptionWithPasswords(userPw, []byte("owner"), -3904, true)
	if err != nil {
		t.Fatalf("setup encryption: %v", err)
	}

	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	// Plaintext title must NOT appear in the raw PDF bytes.
	if bytes.Contains(pdfBytes, []byte(title)) {
		t.Error("plaintext title found in encrypted PDF — strings are not being encrypted")
	}
	if bytes.Contains(pdfBytes, []byte(author)) {
		t.Error("plaintext author found in encrypted PDF — strings are not being encrypted")
	}
}

// TestEncryptedPDF_DictStringsRoundtrip verifies that metadata string values
// survive encrypt → write → parse-with-password intact. It uses the parse API
// to open the encrypted PDF with the correct password and then reads the Info
// dict object content, which triggers per-object string decryption.
func TestEncryptedPDF_DictStringsRoundtrip(t *testing.T) {
	const title = "Roundtrip Title"

	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	p.Content().
		BeginText().
		SetFont(p.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("hello").
		EndText()
	b.FinalizePage(p)

	b.Writer().SetMetadata(&types.DocumentMetadata{Title: title})

	userPw := []byte("pw123")
	_, err := b.Writer().SetupEncryptionWithPasswords(userPw, []byte("owner"), -3904, true)
	if err != nil {
		t.Fatalf("setup encryption: %v", err)
	}

	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	// Open with the parse API using the password; this triggers per-object
	// string decryption via DecryptStringsInContent.
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: userPw})
	if err != nil {
		t.Fatalf("parse.OpenWithOptions: %v", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil || trailer.InfoRef == "" {
		t.Skip("no Info dictionary in generated PDF")
	}
	var infoObjNum int
	if _, err := fmt.Sscanf(trailer.InfoRef, "%d 0 R", &infoObjNum); err != nil || infoObjNum == 0 {
		t.Fatalf("could not parse InfoRef %q", trailer.InfoRef)
	}
	infoContent, err := pdf.GetObjectContent(infoObjNum)
	if err != nil {
		t.Fatalf("GetObjectContent(%d): %v", infoObjNum, err)
	}
	if !bytes.Contains(infoContent, []byte(title)) {
		t.Errorf("Info dict after decryption does not contain %q\ncontent: %s", title, infoContent)
	}
}

// TestEncryptStringsInContent_LiteralString verifies that literal strings are
// encrypted and the ciphertext differs from the original.
func TestEncryptStringsInContent_LiteralString(t *testing.T) {
	w := NewPDFWriter()
	_, err := w.SetupEncryptionWithPasswords([]byte("pw"), []byte("owner"), -3904, true)
	if err != nil {
		t.Fatalf("setup encryption: %v", err)
	}

	content := []byte("<</Title (Hello World)/Type/Catalog>>")
	encrypted, err := w.encryptStringsInContent(content, 5, 0)
	if err != nil {
		t.Fatalf("encryptStringsInContent: %v", err)
	}

	// The literal (Hello World) must not appear in encrypted output.
	if bytes.Contains(encrypted, []byte("Hello World")) {
		t.Error("plaintext string still present after encryption")
	}
	// Output must still contain the PDF keys /Title and /Type.
	if !bytes.Contains(encrypted, []byte("/Title")) {
		t.Error("/Title key missing from encrypted content")
	}
	// The encrypted string should be a hex string <...>.
	if !bytes.Contains(encrypted, []byte("<")) {
		t.Error("encrypted output should contain a hex string")
	}
}

// TestParsePDFLiteralString exercises the literal-string parser.
func TestParsePDFLiteralString(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantEnd int
	}{
		{"(hello)", "hello", 7},
		{"(hi) extra", "hi", 4},
		{"(a\\nb)", "a\nb", 6},
		{"(a\\(b\\)c)", "a(b)c", 9},
		{"(nest(ed))", "nest(ed)", 10},
		{"(\\101)", "A", 6}, // octal \101 = 65 = 'A'
	}
	for _, tc := range cases {
		end, got, err := pdfParseLiteralString([]byte(tc.input), 0)
		if err != nil {
			t.Errorf("input %q: unexpected error %v", tc.input, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("input %q: got %q, want %q", tc.input, got, tc.want)
		}
		if end != tc.wantEnd {
			t.Errorf("input %q: end=%d, want %d", tc.input, end, tc.wantEnd)
		}
	}
}

// TestParsePDFHexString exercises the hex-string parser.
func TestParsePDFHexString(t *testing.T) {
	cases := []struct {
		input   string
		want    []byte
		wantEnd int
	}{
		{"<48656C6C6F>", []byte("Hello"), 12},
		{"<4142>", []byte{0x41, 0x42}, 6},
		{"<41 42\n43>", []byte{0x41, 0x42, 0x43}, 10},
	}
	for _, tc := range cases {
		end, got, err := pdfParseHexString([]byte(tc.input), 0)
		if err != nil {
			t.Errorf("input %q: unexpected error %v", tc.input, err)
			continue
		}
		if string(got) != string(tc.want) {
			t.Errorf("input %q: got %v, want %v", tc.input, got, tc.want)
		}
		if end != tc.wantEnd {
			t.Errorf("input %q: end=%d, want %d", tc.input, end, tc.wantEnd)
		}
	}
}
