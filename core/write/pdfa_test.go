package write_test

import (
	"strings"
	"testing"
	"time"

	"github.com/benedoc-inc/pdfer/v2/core/write"
)

func buildOnePage(t *testing.T) *write.SimplePDFBuilder {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	p := b.AddPage(write.PageSizeLetter)
	p.Content().
		BeginText().
		SetFont(p.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("PDF/A test document").
		EndText()
	b.FinalizePage(p)
	return b
}

func TestPDFA2b_XMPPresent(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{
		Conformance: write.PDFA2b,
		Title:       "Test Document",
		Author:      "Test Author",
	})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	raw := string(pdfBytes)

	checks := []string{
		"pdfaid:part",
		"<pdfaid:part>2</pdfaid:part>",
		"pdfaid:conformance",
		"<pdfaid:conformance>B</pdfaid:conformance>",
		"Test Document",
		"Test Author",
		"application/pdf",
	}
	for _, want := range checks {
		if !strings.Contains(raw, want) {
			t.Errorf("PDF/A-2b output missing %q", want)
		}
	}
}

func TestPDFA2b_OutputIntentPresent(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA2b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	raw := string(pdfBytes)

	checks := []string{
		"/OutputIntents",
		"GTS_PDFA1",
		"sRGB IEC61966-2.1",
		"/DestOutputProfile",
	}
	for _, want := range checks {
		if !strings.Contains(raw, want) {
			t.Errorf("PDF/A-2b output missing %q", want)
		}
	}
}

func TestPDFA2b_CatalogEntries(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA2b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	raw := string(pdfBytes)

	if !strings.Contains(raw, "/Metadata") {
		t.Error("catalog missing /Metadata reference")
	}
	if !strings.Contains(raw, "/MarkInfo") {
		t.Error("catalog missing /MarkInfo")
	}
}

func TestPDFA2b_PDFVersion(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA2b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF-1.7") {
		t.Errorf("expected %%PDF-1.7 header for PDF/A-2b")
	}
}

func TestPDFA1b_PDFVersion(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA1b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.HasPrefix(string(pdfBytes), "%PDF-1.4") {
		t.Errorf("expected %%PDF-1.4 header for PDF/A-1b")
	}
}

func TestPDFA2b_XMPPartIsOne_ForPDFA1b(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA1b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(pdfBytes), "<pdfaid:part>1</pdfaid:part>") {
		t.Error("PDF/A-1b should set pdfaid:part to 1")
	}
}

func TestPDFA2b_CreationDate(t *testing.T) {
	fixed := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{
		Conformance:  write.PDFA2b,
		CreationDate: fixed,
	})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(pdfBytes), "2024-06-15T12:00:00Z") {
		t.Error("XMP metadata should contain the specified creation date")
	}
}

func TestPDFA2b_ICCProfileInOutput(t *testing.T) {
	b := buildOnePage(t)
	b.SetPDFAMode(write.PDFAOptions{Conformance: write.PDFA2b})
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	// The ICC profile is FlateDecode-compressed, so check for the
	// OutputIntent object and DestOutputProfile reference instead.
	raw := string(pdfBytes)
	if !strings.Contains(raw, "DestOutputProfile") {
		t.Error("missing DestOutputProfile in OutputIntent")
	}
	// The compressed stream should be present (non-zero length).
	if len(pdfBytes) < 1000 {
		t.Error("output too small to contain an embedded ICC profile")
	}
}

func TestPDFA2b_NoModeNoXMP(t *testing.T) {
	b := buildOnePage(t)
	// No SetPDFAMode call.
	pdfBytes, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if strings.Contains(string(pdfBytes), "pdfaid:part") {
		t.Error("non-PDF/A document should not contain pdfaid namespace")
	}
}
