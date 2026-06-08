package manipulate

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/write"
)

func makeSimplePDF(t *testing.T) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	font := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(font, 12).
		SetTextPosition(72, 720).
		ShowText("Hello PDF/A").
		EndText()
	builder.FinalizePage(page)
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("makeSimplePDF: %v", err)
	}
	return b
}

func TestConvertToPDFA_ProducesValidPDF(t *testing.T) {
	src := makeSimplePDF(t)
	out, err := ConvertToPDFA(src, nil, "1b")
	if err != nil {
		t.Fatalf("ConvertToPDFA: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
	if !strings.HasPrefix(string(out), "%PDF-") {
		t.Error("output does not start with %PDF-")
	}
}

func TestConvertToPDFA_HasXMPMetadata(t *testing.T) {
	src := makeSimplePDF(t)
	out, err := ConvertToPDFA(src, nil, "1b")
	if err != nil {
		t.Fatalf("ConvertToPDFA: %v", err)
	}
	pdfStr := string(out)
	if !strings.Contains(pdfStr, "pdfaid") {
		t.Error("expected pdfaid XMP namespace in output")
	}
	if !strings.Contains(pdfStr, "Metadata") {
		t.Error("expected /Metadata reference in catalog")
	}
}

func TestConvertToPDFA_HasOutputIntents(t *testing.T) {
	src := makeSimplePDF(t)
	out, err := ConvertToPDFA(src, nil, "1b")
	if err != nil {
		t.Fatalf("ConvertToPDFA: %v", err)
	}
	if !strings.Contains(string(out), "OutputIntents") {
		t.Error("expected /OutputIntents in output")
	}
}

func TestConvertToPDFA_ConformanceLevels(t *testing.T) {
	src := makeSimplePDF(t)
	for _, level := range []string{"1b", "2b", "3b"} {
		out, err := ConvertToPDFA(src, nil, level)
		if err != nil {
			t.Errorf("conformance %q: unexpected error: %v", level, err)
			continue
		}
		if len(out) == 0 {
			t.Errorf("conformance %q: empty output", level)
		}
	}
}

func TestConvertToPDFA_InvalidConformance(t *testing.T) {
	src := makeSimplePDF(t)
	_, err := ConvertToPDFA(src, nil, "99z")
	if err == nil {
		t.Error("expected error for unknown conformance level")
	}
}

func TestConvertToPDFA_DefaultConformance(t *testing.T) {
	src := makeSimplePDF(t)
	// Empty string should default to 1b without error.
	out, err := ConvertToPDFA(src, nil, "")
	if err != nil {
		t.Fatalf("ConvertToPDFA with empty conformance: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
}
