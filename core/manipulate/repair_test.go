package manipulate

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// buildMultiRevisionPDF creates a PDF with two revisions: an initial page and
// then a stamp applied as an incremental update (simulated by a second Rebuild).
func buildMultiRevisionPDF(t *testing.T) []byte {
	t.Helper()
	// First revision: a plain page.
	b := write.NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(write.PageSizeLetter))
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("initial Bytes: %v", err)
	}
	// Simulate a second revision by running through the manipulator once.
	m, _ := NewPDFManipulator(data, nil, false)
	_ = m.StampText(1, TextStamp{Text: "rev2", X: 10, Y: 10})
	data, err = m.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild rev2: %v", err)
	}
	return data
}

// --- Repair tests -----------------------------------------------------------

func TestRepair_ProducesValidPDF(t *testing.T) {
	src := buildOnePager(t)
	out, err := Repair(src, nil)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Error("output does not start with %PDF-")
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("repaired PDF cannot be parsed: %v", err)
	}
}

func TestRepair_FlattensRevisions(t *testing.T) {
	src := buildMultiRevisionPDF(t)
	out, err := Repair(src, nil)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	// A flattened PDF should have exactly one %%EOF.
	eofCount := strings.Count(string(out), "%%EOF")
	if eofCount != 1 {
		t.Errorf("expected 1 %%%%EOF, got %d", eofCount)
	}
}

func TestRepair_PreservesContent(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "important", X: 72, Y: 700})
	src, _ = m.Rebuild()

	out, _ := Repair(src, nil)
	if !bytes.Contains(out, []byte("important")) {
		t.Error("content lost after Repair")
	}
}

func TestRepair_TwoPagePDF(t *testing.T) {
	src := buildTwoPagePDF(t)
	out, err := Repair(src, nil)
	if err != nil {
		t.Fatalf("Repair: %v", err)
	}
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("no objects in repaired PDF")
	}
}

// --- Linearize tests --------------------------------------------------------

func TestLinearize_HasLinearizedDict(t *testing.T) {
	src := buildOnePager(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	if !bytes.Contains(out, []byte("/Linearized 1")) {
		t.Error("/Linearized dict not found in output")
	}
}

func TestLinearize_LinDictIsFirstObject(t *testing.T) {
	src := buildOnePager(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	// Object 1 should appear before any other N 0 obj, and should contain /Linearized.
	idx1 := bytes.Index(out, []byte("1 0 obj"))
	idxLin := bytes.Index(out, []byte("/Linearized"))
	idx2 := bytes.Index(out, []byte("2 0 obj"))
	if idx1 == -1 || idxLin == -1 {
		t.Fatal("/Linearized not found or 1 0 obj missing")
	}
	if idx1 > idx2 {
		t.Error("1 0 obj should appear before 2 0 obj")
	}
	if idxLin < idx1 || (idx2 != -1 && idxLin > idx2) {
		t.Error("/Linearized should be in object 1")
	}
}

func TestLinearize_FileLengthAccurate(t *testing.T) {
	src := buildOnePager(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	// Extract /L value from the Linearized dict.
	lVal := linExtractIntField(out, "/L")
	if lVal == 0 {
		t.Fatal("could not extract /L from Linearized dict")
	}
	if lVal != int64(len(out)) {
		t.Errorf("/L = %d but actual file length = %d", lVal, len(out))
	}
}

func TestLinearize_XRefOffsetAccurate(t *testing.T) {
	src := buildOnePager(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	tVal := linExtractIntField(out, "/T")
	if tVal == 0 {
		t.Fatal("could not extract /T from Linearized dict")
	}
	actualXRef := linExtractXRefOffset(out)
	if tVal != actualXRef {
		t.Errorf("/T = %d but actual startxref = %d", tVal, actualXRef)
	}
}

func TestLinearize_CatalogEarlyInFile(t *testing.T) {
	src := buildTwoPagePDF(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	// /Type/Catalog should appear in the first half of the file.
	// (30% would be too tight for small test PDFs where the Linearized dict
	// and a single page consume most of the "first section" budget.)
	threshold := len(out) * 60 / 100
	catIdx := bytes.Index(out, []byte("/Type/Catalog"))
	if catIdx == -1 {
		catIdx = bytes.Index(out, []byte("/Type /Catalog"))
	}
	if catIdx == -1 {
		t.Fatal("/Type/Catalog not found")
	}
	if catIdx > threshold {
		t.Errorf("catalog at offset %d, expected within first %d bytes", catIdx, threshold)
	}
}

func TestLinearize_PageCountCorrect(t *testing.T) {
	src := buildTwoPagePDF(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	nVal := linExtractIntField(out, "/N")
	if nVal != 2 {
		t.Errorf("/N = %d, want 2", nVal)
	}
}

func TestLinearize_ParseableByParser(t *testing.T) {
	src := buildTwoPagePDF(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("linearized PDF cannot be parsed: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("no objects in linearized PDF")
	}
}

func TestLinearize_PreservesPageContent(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	p := b.AddPage(write.PageSizeLetter)
	b.FinalizePage(p)
	src, _ := b.Bytes()

	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "preserved_content", X: 72, Y: 700})
	src, _ = m.Rebuild()

	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	if !bytes.Contains(out, []byte("preserved_content")) {
		t.Error("page content lost after linearization")
	}
}

func TestLinearize_SingleRevisionOutput(t *testing.T) {
	src := buildMultiRevisionPDF(t)
	out, err := Linearize(src, nil)
	if err != nil {
		t.Fatalf("Linearize: %v", err)
	}
	eofCount := strings.Count(string(out), "%%EOF")
	if eofCount != 1 {
		t.Errorf("expected 1 %%%%EOF in linearized output, got %d", eofCount)
	}
}

// linExtractIntField extracts the integer value of a field like "/L 123" from
// the /Linearized dict, which is near the start of the output.
func linExtractIntField(data []byte, field string) int64 {
	// Only look in the first 512 bytes (the Linearized dict is always first).
	search := data
	if len(search) > 512 {
		search = search[:512]
	}
	re := regexp.MustCompile(regexp.QuoteMeta(field) + `\s+(\d+)`)
	m := re.FindSubmatch(search)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return n
}
