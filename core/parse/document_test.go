package parse

import (
	"bytes"
	"fmt"
	"testing"
)

// createTestPDF creates a simple PDF for testing
func createTestPDF() []byte {
	var buf bytes.Buffer

	// Header
	buf.WriteString("%PDF-1.7\n")
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A}) // Binary marker

	// Object 1: Catalog
	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")

	// Object 2: Pages
	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n")

	// Object 3: Page
	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>\nendobj\n")

	// Xref
	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("0 4\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))

	// Trailer
	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 4/Root 1 0 R>>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

// createTestPDFWithStream creates a PDF with a stream object
func createTestPDFWithStream() []byte {
	var buf bytes.Buffer

	// Header
	buf.WriteString("%PDF-1.7\n")
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A})

	// Object 1: Catalog
	obj1Offset := buf.Len()
	buf.WriteString("1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")

	// Object 2: Pages
	obj2Offset := buf.Len()
	buf.WriteString("2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n")

	// Object 3: Page
	obj3Offset := buf.Len()
	buf.WriteString("3 0 obj\n<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R>>\nendobj\n")

	// Object 4: Content stream
	streamData := "BT /F1 12 Tf 72 720 Td (Hello World) Tj ET"
	obj4Offset := buf.Len()
	buf.WriteString(fmt.Sprintf("4 0 obj\n<</Length %d>>\nstream\n%s\nendstream\nendobj\n", len(streamData), streamData))

	// Xref
	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString("0 5\n")
	buf.WriteString("0000000000 65535 f \n")
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj1Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj2Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj3Offset))
	buf.WriteString(fmt.Sprintf("%010d 00000 n \n", obj4Offset))

	buf.WriteString("trailer\n")
	buf.WriteString("<</Size 5/Root 1 0 R>>\n")
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func TestParsePDFHeader(t *testing.T) {
	pdf := createTestPDF()

	header, err := ParsePDFHeader(pdf)
	if err != nil {
		t.Fatalf("ParsePDFHeader failed: %v", err)
	}

	if header.Version != "1.7" {
		t.Errorf("Expected version 1.7, got %s", header.Version)
	}

	if header.MajorVersion != 1 {
		t.Errorf("Expected major version 1, got %d", header.MajorVersion)
	}

	if header.MinorVersion != 7 {
		t.Errorf("Expected minor version 7, got %d", header.MinorVersion)
	}

	if !bytes.HasPrefix(header.RawBytes, []byte("%PDF-1.7")) {
		t.Error("RawBytes should start with %PDF-1.7")
	}

	t.Logf("Header: %d bytes, version %s", len(header.RawBytes), header.Version)
}

func TestParsePDFDocument(t *testing.T) {
	pdf := createTestPDF()

	doc, err := ParsePDFDocument(pdf)
	if err != nil {
		t.Fatalf("ParsePDFDocument failed: %v", err)
	}

	if doc.Header == nil {
		t.Error("Document should have header")
	}

	if len(doc.Revisions) != 1 {
		t.Errorf("Expected 1 revision, got %d", len(doc.Revisions))
	}

	if doc.RevisionCount() != 1 {
		t.Errorf("RevisionCount should be 1, got %d", doc.RevisionCount())
	}

	// Check objects
	objCount := doc.ObjectCount()
	if objCount < 3 {
		t.Errorf("Expected at least 3 objects, got %d", objCount)
	}

	// Check specific object
	obj1 := doc.GetObject(1)
	if obj1 == nil {
		t.Error("Object 1 should exist")
	} else if obj1.Number != 1 {
		t.Errorf("Object 1 number should be 1, got %d", obj1.Number)
	}

	t.Logf("Document: %d revisions, %d objects", len(doc.Revisions), objCount)
}

func TestParsePDFDocument_BytePreservation(t *testing.T) {
	pdf := createTestPDF()

	doc, err := ParsePDFDocument(pdf)
	if err != nil {
		t.Fatalf("ParsePDFDocument failed: %v", err)
	}

	// Check that RawBytes is preserved
	if !bytes.Equal(doc.RawBytes, pdf) {
		t.Error("RawBytes should equal original PDF")
	}

	// Check that Bytes() returns the same
	if !bytes.Equal(doc.Bytes(), pdf) {
		t.Error("Bytes() should return original PDF bytes")
	}
}

func TestParseRawObject(t *testing.T) {
	pdf := createTestPDF()

	// Find object 1
	obj1Pattern := []byte("1 0 obj")
	obj1Offset := bytes.Index(pdf, obj1Pattern)
	if obj1Offset == -1 {
		t.Fatal("Could not find object 1")
	}

	obj, err := ParseRawObject(pdf, 1, 0, int64(obj1Offset))
	if err != nil {
		t.Fatalf("ParseRawObject failed: %v", err)
	}

	if obj.Number != 1 {
		t.Errorf("Object number should be 1, got %d", obj.Number)
	}

	if obj.Generation != 0 {
		t.Errorf("Generation should be 0, got %d", obj.Generation)
	}

	if !bytes.Contains(obj.RawBytes, []byte("1 0 obj")) {
		t.Error("RawBytes should contain object header")
	}

	if !bytes.HasSuffix(obj.RawBytes, []byte("endobj")) {
		t.Error("RawBytes should end with endobj")
	}

	// Check content extraction
	content := obj.Content()
	if !bytes.Contains(content, []byte("/Type/Catalog")) {
		t.Errorf("Content should contain /Type/Catalog, got: %s", string(content))
	}

	t.Logf("Object 1: %d raw bytes, content: %s", len(obj.RawBytes), string(content))
}

func TestParseRawObject_Stream(t *testing.T) {
	pdf := createTestPDFWithStream()

	// Find object 4 (stream)
	obj4Pattern := []byte("4 0 obj")
	obj4Offset := bytes.Index(pdf, obj4Pattern)
	if obj4Offset == -1 {
		t.Fatal("Could not find object 4")
	}

	obj, err := ParseRawObject(pdf, 4, 0, int64(obj4Offset))
	if err != nil {
		t.Fatalf("ParseRawObject failed: %v", err)
	}

	if !obj.IsStream {
		t.Error("Object 4 should be a stream")
	}

	if len(obj.DictRaw) == 0 {
		t.Error("Stream should have dictionary bytes")
	}

	if len(obj.StreamRaw) == 0 {
		t.Error("Stream should have stream data")
	}

	expectedStream := "BT /F1 12 Tf 72 720 Td (Hello World) Tj ET"
	if string(obj.StreamRaw) != expectedStream {
		t.Errorf("Stream data mismatch.\nExpected: %s\nGot: %s", expectedStream, string(obj.StreamRaw))
	}

	t.Logf("Stream object: dict=%d bytes, stream=%d bytes", len(obj.DictRaw), len(obj.StreamRaw))
}

func TestParseXRefDataRaw(t *testing.T) {
	pdf := createTestPDF()

	// Find xref offset
	xrefPattern := []byte("xref")
	xrefOffset := bytes.Index(pdf, xrefPattern)
	if xrefOffset == -1 {
		t.Fatal("Could not find xref")
	}

	xref, err := ParseXRefDataRaw(pdf, int64(xrefOffset))
	if err != nil {
		t.Fatalf("ParseXRefDataRaw failed: %v", err)
	}

	if xref.Type != XRefTypeTable {
		t.Errorf("Expected traditional xref table, got type %d", xref.Type)
	}

	if len(xref.Entries) < 4 {
		t.Errorf("Expected at least 4 xref entries, got %d", len(xref.Entries))
	}

	if !bytes.HasPrefix(xref.RawBytes, []byte("xref")) {
		t.Error("RawBytes should start with 'xref'")
	}

	t.Logf("Xref: %d entries, %d raw bytes", len(xref.Entries), len(xref.RawBytes))
}

func TestParseTrailerDataRaw(t *testing.T) {
	pdf := createTestPDF()

	// Find xref offset
	xrefPattern := []byte("xref")
	xrefOffset := bytes.Index(pdf, xrefPattern)
	if xrefOffset == -1 {
		t.Fatal("Could not find xref")
	}

	eofOffset := bytes.Index(pdf, []byte("%%EOF"))

	trailer, err := ParseTrailerDataRaw(pdf, int64(xrefOffset), eofOffset)
	if err != nil {
		t.Fatalf("ParseTrailerDataRaw failed: %v", err)
	}

	if trailer.Size != 4 {
		t.Errorf("Expected Size 4, got %d", trailer.Size)
	}

	if trailer.Root != "1 0 R" {
		t.Errorf("Expected Root '1 0 R', got '%s'", trailer.Root)
	}

	if !bytes.Contains(trailer.RawBytes, []byte("trailer")) {
		t.Error("RawBytes should contain 'trailer'")
	}

	t.Logf("Trailer: Size=%d, Root=%s, %d raw bytes", trailer.Size, trailer.Root, len(trailer.RawBytes))
}

func TestPDFDocument_AllObjects(t *testing.T) {
	pdf := createTestPDF()

	doc, err := ParsePDFDocument(pdf)
	if err != nil {
		t.Fatalf("ParsePDFDocument failed: %v", err)
	}

	allObjs := doc.AllObjects()

	// Should have objects 1, 2, 3
	for i := 1; i <= 3; i++ {
		if _, ok := allObjs[i]; !ok {
			t.Errorf("Missing object %d", i)
		}
	}

	t.Logf("AllObjects: %d objects", len(allObjs))
}

func TestPDFDocument_MultiRevision(t *testing.T) {
	pdf := createIncrementalPDF()

	doc, err := ParsePDFDocument(pdf)
	if err != nil {
		t.Fatalf("ParsePDFDocument failed: %v", err)
	}

	if len(doc.Revisions) != 2 {
		t.Errorf("Expected 2 revisions, got %d", len(doc.Revisions))
	}

	// Check that object 4 exists in both revisions
	obj4Rev1 := doc.GetObjectInRevision(4, 1)
	obj4Rev2 := doc.GetObjectInRevision(4, 2)

	if obj4Rev1 == nil {
		t.Error("Object 4 should exist in revision 1")
	}
	if obj4Rev2 == nil {
		t.Error("Object 4 should exist in revision 2")
	}

	// The offsets should be different (object was updated)
	if obj4Rev1 != nil && obj4Rev2 != nil {
		if obj4Rev1.Offset == obj4Rev2.Offset {
			t.Error("Object 4 should have different offsets in each revision")
		}
		t.Logf("Object 4: Rev1 offset=%d, Rev2 offset=%d", obj4Rev1.Offset, obj4Rev2.Offset)
	}

	// GetObject should return the latest version
	latestObj4 := doc.GetObject(4)
	if latestObj4 == nil {
		t.Error("GetObject(4) should return latest version")
	} else if latestObj4.Offset != obj4Rev2.Offset {
		t.Error("GetObject should return revision 2 version")
	}
}

func TestBytePreservation_Roundtrip(t *testing.T) {
	// Test with simple PDF
	t.Run("SimplePDF", func(t *testing.T) {
		original := createTestPDF()

		doc, err := ParsePDFDocument(original)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		reconstructed := doc.Bytes()

		if !bytes.Equal(original, reconstructed) {
			t.Error("Round-trip failed: bytes differ")
			if len(original) != len(reconstructed) {
				t.Errorf("Length mismatch: original=%d, reconstructed=%d", len(original), len(reconstructed))
			}
		}
	})

	// Test with stream PDF
	t.Run("StreamPDF", func(t *testing.T) {
		original := createTestPDFWithStream()

		doc, err := ParsePDFDocument(original)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		reconstructed := doc.Bytes()

		if !bytes.Equal(original, reconstructed) {
			t.Error("Round-trip failed: bytes differ")
		}
	})

	// Test with incremental PDF
	t.Run("IncrementalPDF", func(t *testing.T) {
		original := createIncrementalPDF()

		doc, err := ParsePDFDocument(original)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}

		reconstructed := doc.Bytes()

		if !bytes.Equal(original, reconstructed) {
			t.Error("Round-trip failed: bytes differ")
			if len(original) != len(reconstructed) {
				t.Errorf("Length mismatch: original=%d, reconstructed=%d", len(original), len(reconstructed))
			}
		}
	})
}
