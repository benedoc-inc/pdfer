package xfa

import (
	"bytes"
	"compress/flate"
	"os"
	"path/filepath"
	"testing"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/types"
)

func TestDecompressStream(t *testing.T) {
	// Test with compressed data
	original := []byte("Hello, World!")
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	w.Write(original)
	w.Close()
	compressed := buf.Bytes()

	decompressed, wasCompressed, err := DecompressStream(compressed)
	if err != nil {
		t.Fatalf("DecompressStream() error = %v", err)
	}
	if !wasCompressed {
		t.Error("DecompressStream() wasCompressed = false, want true")
	}
	if !bytes.Equal(decompressed, original) {
		t.Errorf("DecompressStream() = %q, want %q", decompressed, original)
	}

	// Test with uncompressed data
	uncompressed, wasCompressed, err := DecompressStream(original)
	if err != nil {
		t.Fatalf("DecompressStream() error = %v", err)
	}
	if wasCompressed {
		t.Error("DecompressStream() wasCompressed = true, want false")
	}
	if !bytes.Equal(uncompressed, original) {
		t.Errorf("DecompressStream() = %q, want %q", uncompressed, original)
	}
}

func TestCompressStream(t *testing.T) {
	// Test with data that should compress well
	original := []byte("Hello, World! This is a test message. " +
		"Repeated text repeated text repeated text repeated text. " +
		"More repetition more repetition more repetition.")

	compressed, err := CompressStream(original)
	if err != nil {
		t.Fatalf("CompressStream() error = %v", err)
	}

	// Compression may not always reduce size for small data due to overhead,
	// but for larger repetitive data it should help
	if len(compressed) > len(original)*2 {
		t.Errorf("CompressStream() compressed size %d is unexpectedly large (original: %d)", len(compressed), len(original))
	}

	// Decompress to verify round-trip works correctly
	decompressed, wasCompressed, err := DecompressStream(compressed)
	if err != nil {
		t.Fatalf("DecompressStream() error = %v", err)
	}
	if !wasCompressed {
		t.Error("DecompressStream() wasCompressed = false, want true (data should be compressed)")
	}
	if !bytes.Equal(decompressed, original) {
		t.Errorf("Round-trip: decompressed = %q, want %q", decompressed, original)
	}

	// Test with small data (may not compress well, but should still work)
	smallOriginal := []byte("test")
	smallCompressed, err := CompressStream(smallOriginal)
	if err != nil {
		t.Fatalf("CompressStream() error with small data = %v", err)
	}

	smallDecompressed, _, err := DecompressStream(smallCompressed)
	if err != nil {
		t.Fatalf("DecompressStream() error with small data = %v", err)
	}
	if !bytes.Equal(smallDecompressed, smallOriginal) {
		t.Errorf("Round-trip with small data: decompressed = %q, want %q", smallDecompressed, smallOriginal)
	}
}

func TestUpdateXFAFieldValues(t *testing.T) {
	xfaXML := `<data>
<field name="testField">
<value>oldValue</value>
</field>
<field name="anotherField">
<value>anotherValue</value>
</field>
</data>`

	formData := types.FormData{
		"testField":    "newValue",
		"missingField": "shouldNotAppear",
	}

	updated, err := UpdateXFAFieldValues(xfaXML, formData, false)
	if err != nil {
		t.Fatalf("UpdateXFAFieldValues() error = %v", err)
	}

	// Check that testField was updated
	if !bytes.Contains([]byte(updated), []byte("newValue")) {
		t.Error("UpdateXFAFieldValues() did not update testField")
	}
	if bytes.Contains([]byte(updated), []byte("oldValue")) {
		t.Error("UpdateXFAFieldValues() still contains oldValue")
	}

	// Check that anotherField was not changed
	if !bytes.Contains([]byte(updated), []byte("anotherValue")) {
		t.Error("UpdateXFAFieldValues() changed anotherField")
	}
}

func TestUpdateXFAValues(t *testing.T) {
	xfaXML := `<xdp>
<data>
<field name="testField">
<value>oldValue</value>
</field>
</data>
</xdp>`

	formData := types.FormData{
		"testField": "newValue",
	}

	updated, err := UpdateXFAValues(xfaXML, formData, false)
	if err != nil {
		t.Fatalf("UpdateXFAValues() error = %v", err)
	}

	// Check that testField was updated
	if !bytes.Contains([]byte(updated), []byte("newValue")) {
		t.Error("UpdateXFAValues() did not update testField")
	}
	if bytes.Contains([]byte(updated), []byte("oldValue")) {
		t.Error("UpdateXFAValues() still contains oldValue")
	}
}

func TestUpdateXFAFieldValues_InsertValue(t *testing.T) {
	xfaXML := `<field name="testField">
</field>`

	formData := types.FormData{
		"testField": "newValue",
	}

	updated, err := UpdateXFAFieldValues(xfaXML, formData, false)
	if err != nil {
		t.Fatalf("UpdateXFAFieldValues() error = %v", err)
	}

	// Check that value was inserted
	if !bytes.Contains([]byte(updated), []byte("<value>newValue</value>")) {
		t.Error("UpdateXFAFieldValues() did not insert value element")
	}
}

// findTestPDF returns the path to the test PDF if it exists, or empty string
func findTestPDF() string {
	// Try common locations
	paths := []string{
		"/Users/b/repos/diligent/BureaucracyBuster/data/raw_data/estar/nIVD_eSTAR_5-5.pdf",
		"../../../../data/raw_data/estar/nIVD_eSTAR_5-5.pdf",
		"../../../data/raw_data/estar/nIVD_eSTAR_5-5.pdf",
	}
	for _, path := range paths {
		if absPath, err := filepath.Abs(path); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				return absPath
			}
		}
	}
	return ""
}

// TestFindXFADatasetsStream_Integration tests the full integration
func TestFindXFADatasetsStream_Integration(t *testing.T) {
	pdfPath := findTestPDF()
	if pdfPath == "" {
		t.Skip("Test PDF not found, skipping integration test")
	}

	// Read PDF
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	// Parse encryption
	encInfo, err := encrypt.ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("Failed to parse encryption dictionary: %v", err)
	}

	// Derive encryption key
	fileID := encrypt.ExtractFileID(pdfBytes, false)
	encryptKey, err := encrypt.DeriveEncryptionKey([]byte(""), encInfo, fileID, false)
	if err != nil {
		t.Fatalf("Failed to derive encryption key: %v", err)
	}
	encInfo.EncryptKey = encryptKey

	// Test FindXFADatasetsStream (should use incremental parser)
	xfaData, objNum, err := FindXFADatasetsStream(pdfBytes, encInfo, false)
	if err != nil {
		t.Fatalf("FindXFADatasetsStream() error = %v", err)
	}

	if len(xfaData) == 0 {
		t.Error("FindXFADatasetsStream() returned empty data")
	}

	// Verify XFA data structure
	if !bytes.Contains(xfaData, []byte("datasets")) {
		t.Error("XFA data does not contain 'datasets'")
	}

	if !bytes.Contains(xfaData, []byte("<")) {
		t.Error("XFA data does not appear to be XML")
	}

	t.Logf("Integration test: Found XFA stream, %d bytes, object number: %d", len(xfaData), objNum)
}
