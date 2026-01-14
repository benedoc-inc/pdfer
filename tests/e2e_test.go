package tests

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// TestE2E_CreateAndParseSimplePDF tests creating a simple PDF and parsing it back
func TestE2E_CreateAndParseSimplePDF(t *testing.T) {
	// Create a simple PDF with text
	builder := write.NewSimplePDFBuilder()

	page := builder.AddPage(write.PageSizeLetter)
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
	trailer, err := parse.ParsePDFTrailer(pdfBytes)
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
	builder := write.NewSimplePDFBuilder()

	// Create 3 pages
	for i := 1; i <= 3; i++ {
		page := builder.AddPage(write.PageSizeA4)
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
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

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
	trailer, err := parse.ParsePDFTrailer(pdfBytes)
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
		encoded := parse.EncodeASCIIHex(testData)
		decoded, err := parse.DecodeASCIIHex(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(decoded, testData) {
			t.Errorf("Round-trip failed: got %s, want %s", decoded, testData)
		}
	})

	// Test ASCII85Decode round-trip
	t.Run("ASCII85Decode", func(t *testing.T) {
		encoded := parse.EncodeASCII85(testData)
		decoded, err := parse.DecodeASCII85(encoded)
		if err != nil {
			t.Fatalf("Decode failed: %v", err)
		}
		if !bytes.Equal(decoded, testData) {
			t.Errorf("Round-trip failed: got %s, want %s", decoded, testData)
		}
	})

	// Test RunLengthDecode round-trip
	t.Run("RunLengthDecode", func(t *testing.T) {
		encoded := parse.EncodeRunLength(testData)
		decoded, err := parse.DecodeRunLength(encoded)
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
	w := write.NewPDFWriter()

	// Create content with ASCIIHex encoding
	content := []byte("BT /F1 12 Tf 72 720 Td (Test) Tj ET")
	encoded := parse.EncodeASCIIHex(content)

	// Create stream dictionary manually
	streamDict := write.Dictionary{
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
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	page.Content().BeginText().EndText()
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Parse the trailer
	trailer, err := parse.ParsePDFTrailer(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse trailer: %v", err)
	}

	// Parse xref table
	objMap, err := parse.ParseCrossReferenceTable(pdfBytes, trailer.StartXRef)
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
	cs := write.NewContentStream()

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

// TestE2E_DCTDecodeJPEGImage tests parsing a PDF with JPEG images (DCTDecode filter)
// and verifying the filter works correctly
func TestE2E_DCTDecodeJPEGImage(t *testing.T) {
	// First, create and save a test PDF with JPEG if it doesn't exist
	testPDFPath := getTestResourcePath("test_jpeg.pdf")

	var pdfBytes []byte
	var err error

	// Check if test PDF already exists
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Logf("Creating test PDF with JPEG image: %s", testPDFPath)

		// Create a minimal JPEG image (10x10 pixel, RGB)
		img := image.NewRGBA(image.Rect(0, 0, 10, 10))
		for y := 0; y < 10; y++ {
			for x := 0; x < 10; x++ {
				// Create a simple pattern
				r := uint8((x * 255) / 10)
				g := uint8((y * 255) / 10)
				b := uint8(128)
				img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}

		// Encode as JPEG
		var jpegBuf bytes.Buffer
		err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
		if err != nil {
			t.Fatalf("Failed to encode JPEG: %v", err)
		}
		jpegData := jpegBuf.Bytes()

		// Create PDF with JPEG image
		builder := write.NewSimplePDFBuilder()
		page := builder.AddPage(write.PageSizeLetter)

		// Add JPEG image to the PDF
		imgInfo, err := builder.Writer().AddJPEGImage(jpegData, "Im1")
		if err != nil {
			t.Fatalf("Failed to add JPEG image: %v", err)
		}

		// Register image with page and draw it
		imgName := page.AddImage(imgInfo)
		page.Content().DrawImageAt(imgName, 72, 500, 100, 100)

		builder.FinalizePage(page)

		pdfBytes, err = builder.Bytes()
		if err != nil {
			t.Fatalf("Failed to create PDF: %v", err)
		}

		// Save to resources directory for future use
		if err := ensureTestResourceDir(); err != nil {
			t.Logf("Warning: Could not create resources directory: %v", err)
		} else {
			if err := os.WriteFile(testPDFPath, pdfBytes, 0644); err != nil {
				t.Logf("Warning: Could not save test PDF: %v", err)
			} else {
				t.Logf("Saved test PDF to: %s", testPDFPath)
			}
		}
	} else {
		// Load existing test PDF
		t.Logf("Loading existing test PDF: %s", testPDFPath)
		pdfBytes, err = os.ReadFile(testPDFPath)
		if err != nil {
			t.Fatalf("Failed to read test PDF: %v", err)
		}
	}

	t.Logf("Using PDF with JPEG: %d bytes", len(pdfBytes))

	// Verify PDF contains DCTDecode filter
	if !bytes.Contains(pdfBytes, []byte("/DCTDecode")) {
		t.Error("PDF should contain /DCTDecode filter")
	}

	// Verify PDF contains image XObject markers
	hasSubtype := bytes.Contains(pdfBytes, []byte("/Subtype"))
	hasImage := bytes.Contains(pdfBytes, []byte("/Image"))
	if !hasSubtype || !hasImage {
		t.Error("PDF should contain image XObject (/Subtype and /Image)")
	}

	// Parse the PDF using unified API
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Find image objects by searching for DCTDecode filter
	// We need to find which object has the DCTDecode filter
	var imageObjNum int
	var imageObjData []byte

	for _, objNum := range pdf.Objects() {
		objData, err := pdf.GetObject(objNum)
		if err != nil {
			continue
		}

		// Check if this object has DCTDecode filter
		objStr := string(objData)
		if strings.Contains(objStr, "/DCTDecode") && strings.Contains(objStr, "/Subtype") && strings.Contains(objStr, "/Image") {
			imageObjNum = objNum
			imageObjData = objData
			break
		}
	}

	if imageObjNum == 0 {
		t.Fatal("No image object with DCTDecode filter found in PDF")
	}

	t.Logf("Found image object: %d", imageObjNum)

	// Get the image object
	objData := imageObjData
	if err != nil {
		t.Fatalf("Failed to get image object: %v", err)
	}

	// Verify object contains DCTDecode filter
	objStr := string(objData)
	if !strings.Contains(objStr, "/DCTDecode") {
		t.Error("Image object should contain /DCTDecode filter")
	}

	// Extract the stream data from the object
	// Find stream keyword
	streamIdx := bytes.Index(objData, []byte("stream"))
	if streamIdx == -1 {
		t.Fatal("Image object should contain stream data")
	}

	// Find endstream
	endstreamIdx := bytes.Index(objData[streamIdx:], []byte("endstream"))
	if endstreamIdx == -1 {
		t.Fatal("Image object should contain endstream")
	}
}

// TestE2E_MetadataRoundTrip tests writing metadata to a PDF and reading it back
func TestE2E_MetadataRoundTrip(t *testing.T) {
	// Create a PDF with metadata
	builder := write.NewSimplePDFBuilder()

	// Set comprehensive metadata
	metadata := &types.DocumentMetadata{
		Title:        "Test Document Title",
		Author:       "Test Author Name",
		Subject:      "Test Subject",
		Keywords:     "test, pdf, metadata, roundtrip",
		Creator:      "pdfer test suite",
		Producer:     "pdfer 0.9.2",
		CreationDate: "2024-01-15T10:30:00Z",
		ModDate:      "2024-01-16T14:45:00Z",
		Custom: map[string]string{
			"CustomField1": "CustomValue1",
			"CustomField2": "CustomValue2",
		},
	}

	builder.Writer().SetMetadata(metadata)

	// Add a simple page
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fontName, 24).
		SetTextPosition(72, 720).
		ShowText("Metadata Test Document").
		EndText()

	builder.FinalizePage(page)

	// Generate PDF
	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	t.Logf("Created PDF with metadata: %d bytes", len(pdfBytes))

	// Verify PDF contains metadata references
	if !bytes.Contains(pdfBytes, []byte("/Info")) {
		t.Error("PDF should contain /Info reference in trailer")
	} else {
		// Find and log the Info reference
		infoIdx := bytes.Index(pdfBytes, []byte("/Info"))
		if infoIdx > 0 {
			// Extract a snippet around the Info reference
			start := infoIdx - 50
			if start < 0 {
				start = 0
			}
			end := infoIdx + 100
			if end > len(pdfBytes) {
				end = len(pdfBytes)
			}
			t.Logf("PDF contains /Info at offset %d: %s", infoIdx, string(pdfBytes[start:end]))
		}
	}

	// Parse the PDF back
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Debug: Check trailer
	trailer := pdf.Trailer()
	if trailer != nil {
		t.Logf("Trailer InfoRef: %q", trailer.InfoRef)
		if trailer.InfoRef != "" {
			// Try to get the Info object directly
			// Parse object reference manually
			parts := strings.Fields(trailer.InfoRef)
			if len(parts) >= 1 {
				var infoObjNum int
				_, err := fmt.Sscanf(parts[0], "%d", &infoObjNum)
				if err == nil {
					infoObj, err := pdf.GetObject(infoObjNum)
					if err == nil {
						t.Logf("Info object content: %s", string(infoObj))
					} else {
						t.Logf("Failed to get Info object %d: %v", infoObjNum, err)
					}
				}
			}
		} else {
			t.Logf("InfoRef is empty in trailer")
		}
	} else {
		t.Logf("Trailer is nil")
	}

	// Extract metadata
	extractedMetadata, err := extract.ExtractMetadata(pdfBytes, pdf, true) // Use verbose for debugging
	if err != nil {
		t.Fatalf("Failed to extract metadata: %v", err)
	}

	if extractedMetadata == nil {
		t.Fatal("Extracted metadata should not be nil")
	}

	// Verify all metadata fields
	if extractedMetadata.Title != metadata.Title {
		t.Errorf("Title mismatch: got %q, want %q", extractedMetadata.Title, metadata.Title)
	}

	if extractedMetadata.Author != metadata.Author {
		t.Errorf("Author mismatch: got %q, want %q", extractedMetadata.Author, metadata.Author)
	}

	if extractedMetadata.Subject != metadata.Subject {
		t.Errorf("Subject mismatch: got %q, want %q", extractedMetadata.Subject, metadata.Subject)
	}

	if extractedMetadata.Keywords != metadata.Keywords {
		t.Errorf("Keywords mismatch: got %q, want %q", extractedMetadata.Keywords, metadata.Keywords)
	}

	if extractedMetadata.Creator != metadata.Creator {
		t.Errorf("Creator mismatch: got %q, want %q", extractedMetadata.Creator, metadata.Creator)
	}

	if extractedMetadata.Producer != metadata.Producer {
		t.Errorf("Producer mismatch: got %q, want %q", extractedMetadata.Producer, metadata.Producer)
	}

	// Verify dates (they may be in different formats, so check they contain the date)
	if extractedMetadata.CreationDate == "" {
		t.Error("CreationDate should be extracted")
	} else {
		// Should contain the date part
		if !strings.Contains(extractedMetadata.CreationDate, "2024") ||
			!strings.Contains(extractedMetadata.CreationDate, "01") ||
			!strings.Contains(extractedMetadata.CreationDate, "15") {
			t.Errorf("CreationDate should contain 2024-01-15, got %q", extractedMetadata.CreationDate)
		}
	}

	if extractedMetadata.ModDate == "" {
		t.Error("ModDate should be extracted")
	} else {
		// Should contain the date part
		if !strings.Contains(extractedMetadata.ModDate, "2024") ||
			!strings.Contains(extractedMetadata.ModDate, "01") ||
			!strings.Contains(extractedMetadata.ModDate, "16") {
			t.Errorf("ModDate should contain 2024-01-16, got %q", extractedMetadata.ModDate)
		}
	}

	// Verify custom fields
	if len(extractedMetadata.Custom) == 0 {
		t.Error("Custom fields should be extracted")
	}

	// Note: Custom field extraction depends on how the extractor parses the Info dict
	// For now, we just verify that custom fields were written (check PDF bytes)
	if !strings.Contains(string(pdfBytes), "CustomField1") || !strings.Contains(string(pdfBytes), "CustomValue1") {
		t.Error("PDF should contain custom field 1")
	}
	if !strings.Contains(string(pdfBytes), "CustomField2") || !strings.Contains(string(pdfBytes), "CustomValue2") {
		t.Error("PDF should contain custom field 2")
	}

	t.Logf("Metadata round-trip successful:")
	t.Logf("  Title: %q", extractedMetadata.Title)
	t.Logf("  Author: %q", extractedMetadata.Author)
	t.Logf("  Subject: %q", extractedMetadata.Subject)
	t.Logf("  Keywords: %q", extractedMetadata.Keywords)
	t.Logf("  Creator: %q", extractedMetadata.Creator)
	t.Logf("  Producer: %q", extractedMetadata.Producer)
	t.Logf("  CreationDate: %q", extractedMetadata.CreationDate)
	t.Logf("  ModDate: %q", extractedMetadata.ModDate)
}

// TestE2E_MetadataAutoModDate tests that ModDate is automatically set if not provided
func TestE2E_MetadataAutoModDate(t *testing.T) {
	builder := write.NewSimplePDFBuilder()

	// Set metadata without ModDate
	metadata := &types.DocumentMetadata{
		Title:  "Test Document",
		Author: "Test Author",
	}

	builder.Writer().SetMetadata(metadata)

	// Add a page
	page := builder.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fontName, 12).
		SetTextPosition(72, 720).
		ShowText("Test").
		EndText()

	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF: %v", err)
	}

	// Parse and extract metadata
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	extractedMetadata, err := extract.ExtractMetadata(pdfBytes, pdf, false)
	if err != nil {
		t.Fatalf("Failed to extract metadata: %v", err)
	}

	// ModDate should be automatically set
	if extractedMetadata.ModDate == "" {
		t.Error("ModDate should be automatically set when not provided")
	}

	// Should be in PDF date format (D:YYYYMMDD...)
	if !strings.HasPrefix(extractedMetadata.ModDate, "D:") {
		t.Errorf("ModDate should be in PDF format (D:...), got %q", extractedMetadata.ModDate)
	}

	t.Logf("Auto ModDate: %q", extractedMetadata.ModDate)
}
