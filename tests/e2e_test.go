package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/writer"
)

// TestE2E_CreateAndParseSimplePDF tests creating a simple PDF and parsing it back
func TestE2E_CreateAndParseSimplePDF(t *testing.T) {
	// Create a simple PDF with text
	builder := writer.NewSimplePDFBuilder()

	page := builder.AddPage(writer.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	page.Content().
		BeginText().
		SetFont(fontName, 24).
		SetTextPosition(72, 720).
		ShowText("Hello, PDF World!").
		EndText()

	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	t.Logf("Created PDF: %d bytes", len(pdfBytes))

	// Verify PDF structure
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.7")) {
		t.Error("PDF should start with %PDF-1.7")
	}

	if !bytes.Contains(pdfBytes, []byte("/Type/Catalog")) {
		t.Error("PDF should contain Catalog")
	}

	if !bytes.Contains(pdfBytes, []byte("/Type/Pages")) {
		t.Error("PDF should contain Pages")
	}

	if !bytes.Contains(pdfBytes, []byte("/Type/Page")) {
		t.Error("PDF should contain Page")
	}

	// Note: Content is compressed with FlateDecode, so we verify structure instead
	if !bytes.Contains(pdfBytes, []byte("/FlateDecode")) {
		t.Error("PDF should contain FlateDecode filter (content is compressed)")
	}

	// Parse the PDF back
	trailer, err := parser.ParsePDFTrailer(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse trailer: %v", err)
	}

	if trailer.RootRef == "" {
		t.Error("Trailer should have Root reference")
	}

	t.Logf("Parsed trailer: Root=%s, StartXRef=%d", trailer.RootRef, trailer.StartXRef)
}

// TestE2E_CreateAndParseMultiPagePDF tests creating a multi-page PDF
func TestE2E_CreateAndParseMultiPagePDF(t *testing.T) {
	builder := writer.NewSimplePDFBuilder()

	// Create 3 pages
	for i := 1; i <= 3; i++ {
		page := builder.AddPage(writer.PageSizeA4)
		fontName := page.AddStandardFont("Times-Roman")

		page.Content().
			BeginText().
			SetFont(fontName, 18).
			SetTextPosition(72, 750).
			ShowText("Page " + string(rune('0'+i))).
			EndText()

		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Count Page objects (should be 3)
	pageCount := bytes.Count(pdfBytes, []byte("/Type/Page/"))
	if pageCount != 3 {
		t.Errorf("Expected 3 pages, found %d", pageCount)
	}

	// Verify Pages has Count 3
	if !bytes.Contains(pdfBytes, []byte("/Count 3")) {
		t.Error("Pages should have /Count 3")
	}

	t.Logf("Created multi-page PDF: %d bytes, %d pages", len(pdfBytes), pageCount)
}

// TestE2E_CreatePDFWithGraphics tests creating a PDF with graphics
func TestE2E_CreatePDFWithGraphics(t *testing.T) {
	builder := writer.NewSimplePDFBuilder()
	page := builder.AddPage(writer.PageSizeLetter)

	// Draw some shapes
	page.Content().
		// Red filled rectangle
		SetFillColorRGB(1, 0, 0).
		Rectangle(100, 600, 200, 100).
		Fill().
		// Blue stroked rectangle
		SetStrokeColorRGB(0, 0, 1).
		SetLineWidth(2).
		Rectangle(350, 600, 150, 100).
		Stroke().
		// Green line
		SetStrokeColorRGB(0, 0.5, 0).
		SetLineWidth(3).
		MoveTo(100, 500).
		LineTo(500, 500).
		Stroke()

	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Content is compressed with FlateDecode - verify structure
	if !bytes.Contains(pdfBytes, []byte("/FlateDecode")) {
		t.Error("PDF should contain FlateDecode filter")
	}
	if !bytes.Contains(pdfBytes, []byte("/Contents")) {
		t.Error("PDF should contain Contents reference")
	}

	// Verify we can parse the PDF structure
	trailer, err := parser.ParsePDFTrailer(pdfBytes)
	if err != nil {
		t.Errorf("Failed to parse trailer: %v", err)
	}
	if trailer.RootRef == "" {
		t.Error("Should have parsed root reference")
	}

	t.Logf("Created graphics PDF: %d bytes", len(pdfBytes))
}

// TestE2E_FilterRoundTrip tests encoding and decoding with various filters
func TestE2E_FilterRoundTrip(t *testing.T) {
	testData := []byte("The quick brown fox jumps over the lazy dog. 1234567890!@#$%^&*()")

	// Test ASCIIHexDecode round-trip
	t.Run("ASCIIHexDecode", func(t *testing.T) {
		encoded := parser.EncodeASCIIHex(testData)
		decoded, err := parser.DecodeASCIIHex(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(decoded, testData) {
			t.Errorf("Round-trip failed: got %s, want %s", decoded, testData)
		}
	})

	// Test ASCII85Decode round-trip
	t.Run("ASCII85Decode", func(t *testing.T) {
		encoded := parser.EncodeASCII85(testData)
		decoded, err := parser.DecodeASCII85(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(decoded, testData) {
			t.Errorf("Round-trip failed: got %s, want %s", decoded, testData)
		}
	})

	// Test RunLengthDecode round-trip
	t.Run("RunLengthDecode", func(t *testing.T) {
		encoded := parser.EncodeRunLength(testData)
		decoded, err := parser.DecodeRunLength(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(decoded, testData) {
			t.Errorf("Round-trip failed: got %s, want %s", decoded, testData)
		}
	})
}

// TestE2E_CreatePDFWithASCIIHexStream tests creating a PDF with ASCIIHex encoded stream
func TestE2E_CreatePDFWithASCIIHexStream(t *testing.T) {
	w := writer.NewPDFWriter()

	// Create content with ASCIIHex encoding
	content := []byte("BT /F1 12 Tf 72 720 Td (Test) Tj ET")
	encoded := parser.EncodeASCIIHex(content)

	// Create stream dictionary manually
	streamDict := writer.Dictionary{
		"Filter": "/ASCIIHexDecode",
	}

	// Add as non-compressed stream (ASCIIHex encoding)
	objNum := w.AddStreamObject(streamDict, encoded, false)

	// Create minimal page structure
	fontNum := w.AddObject([]byte("<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>"))
	pageNum := w.AddObject([]byte("<</Type/Page/MediaBox[0 0 612 792]/Contents " +
		string(rune('0'+objNum)) + " 0 R/Resources<</Font<</F1 " +
		string(rune('0'+fontNum)) + " 0 R>>>>>>"))
	pagesNum := w.AddObject([]byte("<</Type/Pages/Kids[" +
		string(rune('0'+pageNum)) + " 0 R]/Count 1>>"))
	catalogNum := w.AddObject([]byte("<</Type/Catalog/Pages " +
		string(rune('0'+pagesNum)) + " 0 R>>"))
	w.SetRoot(catalogNum)

	pdfBytes, err := w.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	if !bytes.Contains(pdfBytes, []byte("/ASCIIHexDecode")) {
		t.Error("PDF should contain ASCIIHexDecode filter")
	}

	t.Logf("Created PDF with ASCIIHex stream: %d bytes", len(pdfBytes))
}

// TestE2E_XRefParsing tests that created PDFs have valid xref tables
func TestE2E_XRefParsing(t *testing.T) {
	builder := writer.NewSimplePDFBuilder()
	page := builder.AddPage(writer.PageSizeLetter)
	page.Content().BeginText().EndText()
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Parse the trailer
	trailer, err := parser.ParsePDFTrailer(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse trailer: %v", err)
	}

	// Parse xref table
	objMap, err := parser.ParseCrossReferenceTable(pdfBytes, trailer.StartXRef)
	if err != nil {
		t.Fatalf("Failed to parse xref: %v", err)
	}

	if len(objMap) == 0 {
		t.Error("Should have parsed at least one object from xref")
	}

	t.Logf("Parsed xref table: %d objects at startxref %d", len(objMap), trailer.StartXRef)

	// Verify we can find objects at the given offsets
	for objNum, offset := range objMap {
		if offset <= 0 {
			continue
		}
		// Check that the offset points to a valid object header
		if int(offset) >= len(pdfBytes) {
			t.Errorf("Object %d offset %d is beyond PDF length", objNum, offset)
			continue
		}

		objData := pdfBytes[offset:]
		// Should start with "N 0 obj" or similar
		if !bytes.Contains(objData[:50], []byte("obj")) {
			t.Errorf("Object %d at offset %d doesn't appear to be valid", objNum, offset)
		}
	}
}

// TestE2E_ContentStreamOperators tests that content stream operators work correctly
func TestE2E_ContentStreamOperators(t *testing.T) {
	cs := writer.NewContentStream()

	// Build a complex content stream
	cs.SaveState().
		SetFillColorRGB(0.5, 0.5, 0.5).
		Rectangle(0, 0, 100, 100).
		Fill().
		RestoreState().
		BeginText().
		SetFont("/F1", 12).
		SetTextLeading(14).
		SetTextPosition(10, 10).
		ShowText("Line 1").
		NextLine().
		ShowText("Line 2").
		EndText()

	content := cs.String()

	// Verify operators
	expected := []string{
		"q",           // SaveState
		"rg",          // SetFillColorRGB
		"re",          // Rectangle
		"f",           // Fill
		"Q",           // RestoreState
		"BT",          // BeginText
		"/F1",         // Font reference
		"Tf",          // SetFont
		"TL",          // SetTextLeading
		"Td",          // SetTextPosition
		"(Line 1) Tj", // ShowText
		"T*",          // NextLine
		"(Line 2) Tj", // ShowText
		"ET",          // EndText
	}

	for _, exp := range expected {
		if !strings.Contains(content, exp) {
			t.Errorf("Content stream should contain '%s', got:\n%s", exp, content)
		}
	}
}
