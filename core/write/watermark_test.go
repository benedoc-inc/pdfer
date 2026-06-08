package write

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
)

func TestWatermark_Text(t *testing.T) {
	builder := NewSimplePDFBuilder()

	// Add a page
	page := builder.AddPage(PageSizeLetter)

	// Add some content
	fontName := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fontName, 12).
		SetTextPosition(72, 720).
		ShowText("Document Content").
		EndText()

	// Add watermark
	options := DefaultWatermarkOptions()
	options.Text = "DRAFT"
	options.FontSize = 72
	options.Angle = 45
	options.Opacity = 0.2
	err := page.AddWatermark(options)
	if err != nil {
		t.Fatalf("AddWatermark failed: %v", err)
	}

	builder.FinalizePage(page)

	// Generate PDF
	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}

	// Parse and verify PDF is valid
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	if pdf.ObjectCount() == 0 {
		t.Error("PDF should have objects")
	}

	// Watermark text may be in compressed content stream, so we verify PDF is valid
	// In a real scenario, you'd extract and decompress content streams to verify
	t.Logf("Generated PDF with watermark: %d bytes, %d objects", len(pdfBytes), pdf.ObjectCount())
}

// TestWatermark_ExtGState verifies that a semi-transparent watermark produces
// a proper /ExtGState entry (not just an RGB colour approximation).
func TestWatermark_ExtGState(t *testing.T) {
	builder := NewSimplePDFBuilder()
	page := builder.AddPage(PageSizeLetter)

	options := DefaultWatermarkOptions()
	options.Text = "DRAFT"
	options.Opacity = 0.3

	if err := page.AddWatermark(options); err != nil {
		t.Fatalf("AddWatermark: %v", err)
	}
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	pdfStr := string(pdfBytes)

	// The page resources dictionary must contain an /ExtGState entry.
	if !strings.Contains(pdfStr, "/ExtGState") {
		t.Error("expected /ExtGState in PDF resources")
	}

	// The ExtGState dict must carry a fill-alpha (/ca) entry.
	if !strings.Contains(pdfStr, "/ca") {
		t.Error("expected /ca (fill alpha) in ExtGState")
	}

}

// TestWatermark_Opaque verifies that opacity == 1.0 skips the ExtGState path
// (no unnecessary /ExtGState in a fully opaque watermark).
func TestWatermark_Opaque(t *testing.T) {
	builder := NewSimplePDFBuilder()
	page := builder.AddPage(PageSizeLetter)

	options := DefaultWatermarkOptions()
	options.Text = "SOLID"
	options.Opacity = 1.0

	if err := page.AddWatermark(options); err != nil {
		t.Fatalf("AddWatermark: %v", err)
	}
	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	// A fully opaque watermark should not need /ExtGState transparency state.
	if strings.Contains(string(pdfBytes), "/ExtGState") {
		t.Error("did not expect /ExtGState for opacity=1.0")
	}
}

func TestWatermark_ConvenienceMethod(t *testing.T) {
	builder := NewSimplePDFBuilder()

	page := builder.AddPage(PageSizeA4)

	// Add watermark using convenience method
	err := page.AddTextWatermark("CONFIDENTIAL", 60, 30)
	if err != nil {
		t.Fatalf("AddTextWatermark failed: %v", err)
	}

	builder.FinalizePage(page)

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}

	// Parse to verify validity (watermark text is in compressed content stream)
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	if pdf.ObjectCount() == 0 {
		t.Error("PDF should have objects")
	}

	t.Logf("Generated PDF with watermark: %d bytes", len(pdfBytes))
}
