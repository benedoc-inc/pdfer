package writer

import (
	"bytes"
	"testing"
)

func TestPDFWriter_BasicPDF(t *testing.T) {
	w := NewPDFWriter()
	
	// Add a simple catalog object
	catalogNum := w.AddObject([]byte("<</Type/Catalog/Pages 2 0 R>>"))
	w.SetRoot(catalogNum)
	
	// Add pages object
	w.AddObject([]byte("<</Type/Pages/Kids[]/Count 0>>"))
	
	// Generate PDF
	pdfBytes, err := w.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}
	
	// Verify PDF structure
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.7")) {
		t.Errorf("PDF should start with %%PDF-1.7")
	}
	
	if !bytes.Contains(pdfBytes, []byte("xref")) {
		t.Errorf("PDF should contain xref table")
	}
	
	if !bytes.Contains(pdfBytes, []byte("trailer")) {
		t.Errorf("PDF should contain trailer")
	}
	
	if !bytes.Contains(pdfBytes, []byte("startxref")) {
		t.Errorf("PDF should contain startxref")
	}
	
	if !bytes.HasSuffix(pdfBytes, []byte("%%EOF\n")) {
		t.Errorf("PDF should end with EOF marker")
	}
	
	t.Logf("Generated PDF: %d bytes", len(pdfBytes))
}

func TestPDFWriter_StreamObject(t *testing.T) {
	w := NewPDFWriter()
	
	// Add a stream object with compression
	streamData := []byte("This is the stream content to be compressed.")
	dict := Dictionary{
		"Type": "/Test",
	}
	objNum := w.AddStreamObject(dict, streamData, true)
	
	// Add catalog
	catalogNum := w.AddObject([]byte("<</Type/Catalog>>"))
	w.SetRoot(catalogNum)
	
	// Generate PDF
	pdfBytes, err := w.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}
	
	// Verify stream object is in PDF
	if !bytes.Contains(pdfBytes, []byte("/FlateDecode")) {
		t.Errorf("PDF should contain FlateDecode filter for compressed stream")
	}
	
	if !bytes.Contains(pdfBytes, []byte("/Length")) {
		t.Errorf("PDF should contain Length in stream dictionary")
	}
	
	t.Logf("Stream object number: %d, PDF: %d bytes", objNum, len(pdfBytes))
}

func TestPDFWriter_SetObject(t *testing.T) {
	w := NewPDFWriter()
	
	// Set objects at specific numbers (useful for rebuild)
	w.SetObject(5, []byte("<</Type/Test1>>"))
	w.SetObject(10, []byte("<</Type/Test2>>"))
	w.SetRoot(5)
	
	// Generate PDF
	pdfBytes, err := w.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}
	
	// Verify objects are present
	if !bytes.Contains(pdfBytes, []byte("5 0 obj")) {
		t.Errorf("PDF should contain object 5")
	}
	
	if !bytes.Contains(pdfBytes, []byte("10 0 obj")) {
		t.Errorf("PDF should contain object 10")
	}
	
	// Verify xref has correct size
	if !bytes.Contains(pdfBytes, []byte("/Size 11")) {
		t.Errorf("PDF should have /Size 11 (0-10 inclusive)")
	}
	
	t.Logf("Generated PDF: %d bytes", len(pdfBytes))
}

func TestPDFWriter_XRefTable(t *testing.T) {
	w := NewPDFWriter()
	
	// Add some objects
	w.AddObject([]byte("<</Test 1>>"))
	w.AddObject([]byte("<</Test 2>>"))
	w.AddObject([]byte("<</Test 3>>"))
	catalogNum := w.AddObject([]byte("<</Type/Catalog>>"))
	w.SetRoot(catalogNum)
	
	// Generate PDF
	pdfBytes, err := w.Bytes()
	if err != nil {
		t.Fatalf("Failed to generate PDF: %v", err)
	}
	
	// Find xref section
	xrefIdx := bytes.Index(pdfBytes, []byte("xref\n"))
	if xrefIdx == -1 {
		t.Fatalf("xref not found")
	}
	
	// Verify xref entries format
	xrefSection := string(pdfBytes[xrefIdx:])
	if !bytes.Contains([]byte(xrefSection), []byte("0000000000 65535 f ")) {
		t.Errorf("xref should start with free entry 0")
	}
	
	t.Logf("xref section starts at offset %d", xrefIdx)
}

func TestDictionary_Formatting(t *testing.T) {
	w := NewPDFWriter()
	
	dict := Dictionary{
		"Type":   "/Catalog",
		"Length": 42,
		"Name":   "/TestName",
		"Ref":    "5 0 R",
	}
	
	formatted := w.formatDictionary(dict)
	
	if !bytes.Contains(formatted, []byte("/Type /Catalog")) {
		t.Errorf("Dictionary should contain /Type /Catalog, got: %s", formatted)
	}
	
	if !bytes.Contains(formatted, []byte("/Length 42")) {
		t.Errorf("Dictionary should contain /Length 42, got: %s", formatted)
	}
	
	if !bytes.Contains(formatted, []byte("/Ref 5 0 R")) {
		t.Errorf("Dictionary should contain /Ref 5 0 R, got: %s", formatted)
	}
	
	t.Logf("Formatted dictionary: %s", formatted)
}
