package parse

import (
	"bytes"
	"fmt"
	"testing"
)

// createTestPDFForAPI creates a simple PDF for API testing
func createTestPDFForAPI() []byte {
	var buf bytes.Buffer

	buf.WriteString("%PDF-1.4\n")
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A}) // Binary marker

	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")

	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n")

	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>\nendobj\n")

	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))

	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 4/Root 1 0 R>>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func TestOpen(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if pdf == nil {
		t.Fatal("Open() returned nil")
	}

	// Check version
	version := pdf.Version()
	if version != "1.4" {
		t.Errorf("Version() = %q, want %q", version, "1.4")
	}

	// Check object count
	count := pdf.ObjectCount()
	if count < 3 {
		t.Errorf("ObjectCount() = %d, want at least 3", count)
	}

	// Check has object
	if !pdf.HasObject(1) {
		t.Error("HasObject(1) = false, want true")
	}
	if pdf.HasObject(999) {
		t.Error("HasObject(999) = true, want false")
	}

	// Get object
	obj, err := pdf.GetObject(1)
	if err != nil {
		t.Errorf("GetObject(1) error = %v", err)
	}
	if !bytes.Contains(obj, []byte("/Catalog")) {
		t.Error("GetObject(1) should contain /Catalog")
	}
}

func TestOpenWithOptions_BytePerfect(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := OpenWithOptions(pdfBytes, ParseOptions{BytePerfect: true})
	if err != nil {
		t.Fatalf("OpenWithOptions() error = %v", err)
	}

	// Check byte-perfect reconstruction
	reconstructed := pdf.Bytes()
	if !bytes.Equal(reconstructed, pdfBytes) {
		t.Error("Bytes() should return identical bytes in BytePerfect mode")
	}

	// Check raw object access
	rawObj, err := pdf.GetRawObject(1)
	if err != nil {
		t.Errorf("GetRawObject(1) error = %v", err)
	}
	if rawObj == nil {
		t.Error("GetRawObject(1) returned nil")
	} else if rawObj.Number != 1 {
		t.Errorf("GetRawObject(1).Number = %d, want 1", rawObj.Number)
	}
}

func TestOpen_TooShort(t *testing.T) {
	_, err := Open([]byte("short"))
	if err == nil {
		t.Error("Open() should fail for too-short input")
	}
}

func TestPDF_IsEncrypted(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if pdf.IsEncrypted() {
		t.Error("IsEncrypted() = true, want false for unencrypted PDF")
	}
}

func TestPDF_Objects(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	objs := pdf.Objects()
	if len(objs) < 3 {
		t.Errorf("Objects() returned %d objects, want at least 3", len(objs))
	}

	// Check that object 1 is in the list
	found := false
	for _, n := range objs {
		if n == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Objects() should include object 1")
	}
}

func TestPDF_Trailer(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	trailer := pdf.Trailer()
	if trailer == nil {
		t.Fatal("Trailer() returned nil")
	}

	// Our test PDF has /Root 1 0 R
	if trailer.RootRef == "" {
		t.Error("Trailer.RootRef is empty")
	}
}

func TestPDF_RevisionCount(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	count := pdf.RevisionCount()
	if count < 1 {
		t.Errorf("RevisionCount() = %d, want at least 1", count)
	}
}

func TestPDF_Raw(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	raw := pdf.Raw()
	if !bytes.Equal(raw, pdfBytes) {
		t.Error("Raw() should return original bytes")
	}
}

func TestPDF_GetRawObject_NotBytePerfect(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes) // Not byte-perfect
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	_, err = pdf.GetRawObject(1)
	if err == nil {
		t.Error("GetRawObject() should fail when not in BytePerfect mode")
	}
}

func TestPDF_GetObject_NotFound(t *testing.T) {
	pdfBytes := createTestPDFForAPI()

	pdf, err := Open(pdfBytes)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	_, err = pdf.GetObject(999)
	if err == nil {
		t.Error("GetObject(999) should fail for non-existent object")
	}
}
