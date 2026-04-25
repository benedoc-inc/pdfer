package write

import (
	"bytes"
	"strings"
	"testing"
)

// buildPlainPDF returns a plain (non-PDF/A) PDF — one empty page, no fonts,
// no metadata. It will have MISSING_XMP, MISSING_OUTPUT_INTENT, and
// MISSING_DOCUMENT_ID violations.
func buildPlainPDF(t *testing.T) []byte {
	t.Helper()
	b := NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(PageSizeLetter))
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

func TestFixPDFA_AlreadyConformantPassthrough(t *testing.T) {
	src := buildTestPDFA2b(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	// Should return the original bytes unchanged.
	if !bytes.Equal(out, src) {
		t.Error("conformant PDF should be returned unchanged")
	}
}

func TestFixPDFA_PlainPDFBecomesConformant(t *testing.T) {
	src := buildPlainPDF(t)

	// Confirm it starts non-conformant.
	pre := ValidatePDFA(src)
	if pre.Conformant {
		t.Skip("plain PDF already conformant — cannot test fix")
	}

	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}

	post := ValidatePDFA(out)
	// UNEMBEDDED_FONT is the only unfixable violation. Plain PDF has no fonts,
	// so post-fix should be fully conformant.
	if !post.Conformant {
		t.Errorf("fixed PDF still has violations: %v", post.Violations)
	}
}

func TestFixPDFA_AddsDocumentID(t *testing.T) {
	src := buildPlainPDF(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	if !bytes.Contains(out, []byte("/ID [")) && !bytes.Contains(out, []byte("/ID[")) {
		t.Error("fixed PDF does not contain /ID array in trailer")
	}
}

func TestFixPDFA_AddsXMP(t *testing.T) {
	src := buildPlainPDF(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	if !bytes.Contains(out, []byte("pdfaid:part")) {
		t.Error("fixed PDF does not contain pdfaid:part in XMP")
	}
	if !bytes.Contains(out, []byte("pdfaid:conformance")) {
		t.Error("fixed PDF does not contain pdfaid:conformance in XMP")
	}
}

func TestFixPDFA_AddsOutputIntent(t *testing.T) {
	src := buildPlainPDF(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	if !bytes.Contains(out, []byte("/OutputIntents")) {
		t.Error("fixed PDF does not contain /OutputIntents")
	}
	if !bytes.Contains(out, []byte("/DestOutputProfile")) {
		t.Error("fixed PDF does not contain /DestOutputProfile")
	}
}

func TestFixPDFA_PreservesPageCount(t *testing.T) {
	b := NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(PageSizeLetter))
	b.FinalizePage(b.AddPage(PageSizeLetter))
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}

	// /Count 2 must appear in the output.
	if !bytes.Contains(out, []byte("/Count 2")) {
		t.Error("fixed PDF does not preserve /Count 2")
	}
}

func TestFixPDFA_ValidatesAfterFix(t *testing.T) {
	src := buildPlainPDF(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	result := ValidatePDFA(out)
	for _, v := range result.Violations {
		if v.Code != ViolUnembeddedFont {
			t.Errorf("unexpected violation after fix: %s: %s", v.Code, v.Message)
		}
	}
}

func TestFixPDFA_OutputHasSingleEOF(t *testing.T) {
	src := buildPlainPDF(t)
	out, err := FixPDFA(src, nil)
	if err != nil {
		t.Fatalf("FixPDFA: %v", err)
	}
	n := strings.Count(string(out), "%%EOF")
	if n != 1 {
		t.Errorf("fixed PDF has %d %%%%EOF markers, want 1", n)
	}
}

// --- helper patching logic tests --------------------------------------------

func TestPdfaRemoveCatalogRef(t *testing.T) {
	cat := "<</Type/Catalog/Metadata 5 0 R/Pages 3 0 R>>"
	got := pdfaRemoveCatalogRef(cat, "Metadata")
	if strings.Contains(got, "/Metadata") {
		t.Errorf("pdfaRemoveCatalogRef did not remove /Metadata: %s", got)
	}
	if !strings.Contains(got, "/Pages") {
		t.Error("pdfaRemoveCatalogRef removed /Pages unexpectedly")
	}
}

func TestPdfaRemoveCatalogArray(t *testing.T) {
	cat := "<</Type/Catalog/OutputIntents [7 0 R]/Pages 3 0 R>>"
	got := pdfaRemoveCatalogArray(cat, "OutputIntents")
	if strings.Contains(got, "/OutputIntents") {
		t.Errorf("pdfaRemoveCatalogArray did not remove /OutputIntents: %s", got)
	}
	if !strings.Contains(got, "/Pages") {
		t.Error("pdfaRemoveCatalogArray removed /Pages unexpectedly")
	}
}

func TestPdfaInjectEntry(t *testing.T) {
	cat := "<</Type/Catalog/Pages 3 0 R>>"
	got := pdfaInjectEntry(cat, "/Metadata 9 0 R")
	if !strings.Contains(got, "/Metadata 9 0 R") {
		t.Errorf("pdfaInjectEntry did not add entry: %s", got)
	}
	// Must still close properly.
	if !strings.HasSuffix(strings.TrimSpace(got), ">>") {
		t.Errorf("pdfaInjectEntry broke dict structure: %s", got)
	}
}

func TestPdfaStripJS(t *testing.T) {
	cases := []struct {
		cat  string
		want string
	}{
		{
			cat:  "<</Type/Catalog/JavaScript 8 0 R/Pages 3 0 R>>",
			want: "/JavaScript",
		},
		{
			cat:  "<</Type/Catalog/JS(alert(1))/Pages 3 0 R>>",
			want: "/JS",
		},
	}
	for _, c := range cases {
		got := pdfaStripJS(c.cat)
		if strings.Contains(got, c.want) {
			t.Errorf("pdfaStripJS(%q) still contains %q: %s", c.cat, c.want, got)
		}
	}
}
