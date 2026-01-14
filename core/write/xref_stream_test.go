package write

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

func TestXRefStream_Basic(t *testing.T) {
	writer := NewPDFWriter()
	writer.UseXRefStream(true)

	// Create a simple PDF
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

	// Update catalog
	catalogDict["/Pages"] = fmt.Sprintf("%d 0 R", pagesNum)
	writer.SetObject(catalogNum, writer.formatDictionary(catalogDict))

	// Write PDF
	var buf bytes.Buffer
	err := writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Debug: show PDF content around xref area
	xrefIdx := bytes.LastIndex(pdfBytes, []byte("obj"))
	if xrefIdx > 0 && xrefIdx < len(pdfBytes)-100 {
		t.Logf("XRef area: %s", string(pdfBytes[xrefIdx:min(len(pdfBytes), xrefIdx+200)]))
	}

	// Verify PDF contains xref stream markers (check for /Type with XRef, may have spaces)
	if !bytes.Contains(pdfBytes, []byte("/Type")) || !bytes.Contains(pdfBytes, []byte("XRef")) {
		t.Error("PDF should contain /Type and XRef")
		t.Logf("PDF content: %s", string(pdfBytes))
	}
	if !bytes.Contains(pdfBytes, []byte("/W")) {
		t.Error("PDF should contain /W (field widths)")
	}
	if !bytes.Contains(pdfBytes, []byte("/FlateDecode")) {
		t.Error("PDF should contain /FlateDecode filter")
	}
	if !bytes.Contains(pdfBytes, []byte("stream")) {
		t.Error("PDF should contain stream keyword")
	}

	// Verify it does NOT contain traditional xref table
	// Check for "xref\n" as a standalone line (not "startxref")
	if bytes.Contains(pdfBytes, []byte("xref\n0 ")) || bytes.Contains(pdfBytes, []byte("xref\r\n0 ")) {
		t.Error("PDF should not contain traditional xref table when using xref stream")
	}

	// Verify it DOES contain xref stream markers
	if !bytes.Contains(pdfBytes, []byte("/Type")) || !bytes.Contains(pdfBytes, []byte("XRef")) {
		t.Error("PDF should contain xref stream")
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

	t.Logf("Generated PDF with xref stream: %d bytes", len(pdfBytes))
}

func TestXRefStream_WithMetadata(t *testing.T) {
	writer := NewPDFWriter()
	writer.UseXRefStream(true)

	// Set metadata
	metadata := &types.DocumentMetadata{
		Title:  "XRef Stream Test",
		Author: "Test Author",
	}
	writer.SetMetadata(metadata)

	// Create minimal PDF
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

	// Parse and verify
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Verify PDF is valid
	if pdf.ObjectCount() == 0 {
		t.Error("PDF should have objects")
	}

	// Verify metadata is in the PDF bytes
	if !strings.Contains(string(pdfBytes), "XRef Stream Test") {
		t.Error("PDF should contain metadata title")
	}
}

func TestXRefStream_DefaultBehavior(t *testing.T) {
	writer := NewPDFWriter()
	// Don't enable xref stream - should use traditional table

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

	// Should contain traditional xref table
	if !strings.Contains(string(pdfBytes), "xref\n") {
		t.Error("PDF should contain traditional xref table by default")
	}

	// Should NOT contain xref stream
	if bytes.Contains(pdfBytes, []byte("/Type/XRef")) {
		t.Error("PDF should not contain xref stream when not enabled")
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
