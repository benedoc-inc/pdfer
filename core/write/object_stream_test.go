package write

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
)

func TestObjectStream_Basic(t *testing.T) {
	writer := NewPDFWriter()
	writer.UseObjectStream(true) // This also enables xref streams

	// Create a simple PDF with multiple objects
	catalogDict := Dictionary{
		"/Type":  "/Catalog",
		"/Pages": "2 0 R",
	}
	catalogNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogNum)

	pagesDict := Dictionary{
		"/Type":  "/Pages",
		"/Count": 2,
		"/Kids":  []interface{}{"3 0 R", "4 0 R"},
	}
	pagesNum := writer.AddObject(writer.formatDictionary(pagesDict))

	page1Dict := Dictionary{
		"/Type":     "/Page",
		"/Parent":   fmt.Sprintf("%d 0 R", pagesNum),
		"/MediaBox": []interface{}{0, 0, 612, 792},
	}
	page1Num := writer.AddObject(writer.formatDictionary(page1Dict))

	page2Dict := Dictionary{
		"/Type":     "/Page",
		"/Parent":   fmt.Sprintf("%d 0 R", pagesNum),
		"/MediaBox": []interface{}{0, 0, 612, 792},
	}
	page2Num := writer.AddObject(writer.formatDictionary(page2Dict))

	// Update pages dict with correct references
	pagesDict["/Kids"] = []interface{}{fmt.Sprintf("%d 0 R", page1Num), fmt.Sprintf("%d 0 R", page2Num)}
	writer.SetObject(pagesNum, writer.formatDictionary(pagesDict))

	// Write PDF
	var buf bytes.Buffer
	err := writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Verify PDF contains object stream markers
	if !bytes.Contains(pdfBytes, []byte("/Type")) || !bytes.Contains(pdfBytes, []byte("ObjStm")) {
		t.Error("PDF should contain /Type and ObjStm")
	}
	if !bytes.Contains(pdfBytes, []byte("/N")) {
		t.Error("PDF should contain /N (object count)")
	}
	if !bytes.Contains(pdfBytes, []byte("/First")) {
		t.Error("PDF should contain /First (header offset)")
	}

	// Parse the PDF back to verify it's valid
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse generated PDF: %v", err)
	}

	// Verify we can access objects
	if !pdf.HasObject(catalogNum) {
		t.Error("Should be able to access catalog object")
	}
	if !pdf.HasObject(pagesNum) {
		t.Error("Should be able to access pages object")
	}

	t.Logf("Generated PDF with object stream: %d bytes", len(pdfBytes))
}

func TestObjectStream_Disabled(t *testing.T) {
	writer := NewPDFWriter()
	writer.UseObjectStream(false) // Explicitly disable

	catalogDict := Dictionary{
		"/Type":  "/Catalog",
		"/Pages": "2 0 R",
	}
	catalogNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogNum)

	pagesDict := Dictionary{
		"/Type":  "/Pages",
		"/Count": 0,
		"/Kids":  []interface{}{},
	}
	pagesNum := writer.AddObject(writer.formatDictionary(pagesDict))
	catalogDict["/Pages"] = fmt.Sprintf("%d 0 R", pagesNum)
	writer.SetObject(catalogNum, writer.formatDictionary(catalogDict))

	var buf bytes.Buffer
	err := writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Should NOT contain object stream
	if bytes.Contains(pdfBytes, []byte("/Type/ObjStm")) {
		t.Error("PDF should not contain object stream when disabled")
	}

	// Parse to verify it's valid
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	if !pdf.HasObject(catalogNum) {
		t.Error("Should be able to access catalog object")
	}
}
