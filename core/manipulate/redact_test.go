package manipulate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// buildRedactPDF creates a one-page PDF with two text runs at different positions:
//
//	"hello" at (72, 700) and "world" at (72, 500).
func buildRedactPDF(t *testing.T) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fn := page.AddStandardFont("Helvetica")
	page.Content().
		BeginText().
		SetFont(fn, 12).
		SetTextPosition(72, 700).
		ShowText("hello").
		SetTextPosition(0, -200). // now at (72, 500)
		ShowText("world").
		EndText()
	b.FinalizePage(page)
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

// --- unit tests for helpers --------------------------------------------------

func TestRedactTokenize_Numbers(t *testing.T) {
	toks := redactTokenize("1 2.5 -3")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(toks))
	}
	for _, tok := range toks {
		if tok.kind != rtokOperand {
			t.Errorf("token %q should be operand", tok.value)
		}
	}
}

func TestRedactTokenize_Operator(t *testing.T) {
	toks := redactTokenize("72 700 Td")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d", len(toks))
	}
	if toks[2].kind != rtokOperator || toks[2].value != "Td" {
		t.Errorf("last token should be operator 'Td', got %+v", toks[2])
	}
}

func TestRedactTokenize_String(t *testing.T) {
	toks := redactTokenize("(hello world) Tj")
	if len(toks) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(toks))
	}
	if toks[0].kind != rtokOperand || toks[0].value != "(hello world)" {
		t.Errorf("unexpected string token: %+v", toks[0])
	}
}

func TestRedactTokenize_Array(t *testing.T) {
	toks := redactTokenize("[(hello) 20 (world)] TJ")
	if len(toks) != 2 {
		t.Fatalf("expected 2 tokens, got %d", len(toks))
	}
	if toks[0].kind != rtokOperand {
		t.Errorf("array should be operand: %+v", toks[0])
	}
}

func TestRedactBBoxOverlap(t *testing.T) {
	boxes := [][4]float64{{0, 0, 100, 100}}
	if !redactBBoxOverlap(10, 10, 50, 50, boxes) {
		t.Error("contained bbox should overlap")
	}
	if !redactBBoxOverlap(-10, -10, 10, 10, boxes) {
		t.Error("partially overlapping bbox should overlap")
	}
	if redactBBoxOverlap(110, 0, 200, 100, boxes) {
		t.Error("non-overlapping bbox should not overlap")
	}
}

func TestRedactContentStream_TextSuppressed(t *testing.T) {
	// A simple content stream with text at (72, 700).
	stream := "BT\n/F1 12 Tf\n72 700 Td\n(secret) Tj\nET\n"
	// Redaction box covers (50, 680) to (200, 720) — contains (72, 700).
	boxes := [][4]float64{{50, 680, 200, 720}}
	rewritten := redactContentStream(stream, boxes)
	if strings.Contains(rewritten, "secret") {
		t.Errorf("redacted content stream still contains 'secret': %q", rewritten)
	}
	// BT/ET and font operators must remain.
	if !strings.Contains(rewritten, "BT") {
		t.Error("BT operator removed")
	}
}

func TestRedactContentStream_TextOutsideBoxPreserved(t *testing.T) {
	stream := "BT\n/F1 12 Tf\n72 200 Td\n(visible) Tj\nET\n"
	// Box is far from (72, 200).
	boxes := [][4]float64{{300, 300, 500, 500}}
	rewritten := redactContentStream(stream, boxes)
	if !strings.Contains(rewritten, "visible") {
		t.Error("content outside redaction box should be preserved")
	}
}

func TestRedactContentStream_PathSuppressed(t *testing.T) {
	// A filled rectangle centred inside the redaction box.
	stream := "10 10 80 80 re\nf\n"
	boxes := [][4]float64{{0, 0, 200, 200}}
	rewritten := redactContentStream(stream, boxes)
	// The path should be suppressed (no re or f).
	if strings.Contains(rewritten, " re\n") || strings.Contains(rewritten, "\nre\n") {
		t.Error("path inside box should be suppressed")
	}
}

func TestRedactContentStream_PathOutsideBoxPreserved(t *testing.T) {
	stream := "300 300 100 100 re\nf\n"
	boxes := [][4]float64{{0, 0, 100, 100}}
	rewritten := redactContentStream(stream, boxes)
	if !strings.Contains(rewritten, "re") {
		t.Error("path outside redaction box should be preserved")
	}
}

func TestRedactContentStream_OverlayAdded(t *testing.T) {
	// The Redact function (not the stream rewriter) adds the overlay.
	// The stream rewriter itself does not add overlay — that's in redactPage.
	// Verify that redactPage result contains the filled rectangle.
	src := buildRedactPDF(t)
	box := RedactBox{Page: 1, Rect: [4]float64{50, 680, 200, 720}}
	out, err := Redact(src, []RedactBox{box}, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	// Output must contain a filled rectangle operator sequence.
	raw := string(out)
	if !strings.Contains(raw, " re\n") && !strings.Contains(raw, " re ") {
		t.Error("redacted PDF does not contain overlay rectangle operator")
	}
}

// --- integration tests -------------------------------------------------------

func TestRedact_TextRemoved(t *testing.T) {
	src := buildRedactPDF(t)
	// Redact the area around "hello" at (72, 700). Box: x:50-200, y:680-720.
	boxes := []RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}
	out, err := Redact(src, boxes, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	// "hello" must not appear in the content stream of the output.
	// We check the raw bytes of the stream sections (between stream/endstream).
	if streamContains(out, "hello") {
		t.Error("content stream still contains 'hello' after redaction")
	}
}

func TestRedact_UnredactedTextPreserved(t *testing.T) {
	src := buildRedactPDF(t)
	// Only redact the "hello" area; "world" at (72, 500) should survive.
	boxes := []RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}
	out, err := Redact(src, boxes, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	if !streamContains(out, "world") {
		t.Error("'world' outside redaction box should be preserved")
	}
}

func TestRedact_ParseableAfterRedact(t *testing.T) {
	src := buildRedactPDF(t)
	out, err := Redact(src, []RedactBox{{Page: 1, Rect: [4]float64{0, 0, 612, 792}}}, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("redacted PDF cannot be parsed: %v", err)
	}
}

func TestRedact_PageStructureIntact(t *testing.T) {
	src := buildRedactPDF(t)
	out, err := Redact(src, []RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("no objects in redacted PDF")
	}
}

func TestRedact_SingleEOF(t *testing.T) {
	src := buildRedactPDF(t)
	out, err := Redact(src, []RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	if strings.Count(string(out), "%%EOF") != 1 {
		t.Error("expected exactly one EOF marker in redacted output")
	}
}

func TestRedact_OOBPageReturnsError(t *testing.T) {
	src := buildRedactPDF(t)
	_, err := Redact(src, []RedactBox{{Page: 99, Rect: [4]float64{0, 0, 100, 100}}}, nil)
	if err == nil {
		t.Error("expected error for out-of-bounds page number")
	}
}

func TestRedact_EmptyBoxListIsNoop(t *testing.T) {
	src := buildRedactPDF(t)
	out, err := Redact(src, nil, nil)
	if err != nil {
		t.Fatalf("Redact with no boxes: %v", err)
	}
	// Output should still be a valid PDF.
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Error("output does not start with %PDF-")
	}
}

func TestRedact_TwoPagePDF(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	p1 := b.AddPage(write.PageSizeLetter)
	fn1 := p1.AddStandardFont("Helvetica")
	p1.Content().BeginText().SetFont(fn1, 12).SetTextPosition(72, 700).ShowText("page1secret").EndText()
	b.FinalizePage(p1)
	p2 := b.AddPage(write.PageSizeLetter)
	fn2 := p2.AddStandardFont("Helvetica")
	p2.Content().BeginText().SetFont(fn2, 12).SetTextPosition(72, 700).ShowText("page2visible").EndText()
	b.FinalizePage(p2)
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	out, err := Redact(src, []RedactBox{{Page: 1, Rect: [4]float64{50, 680, 300, 720}}}, nil)
	if err != nil {
		t.Fatalf("Redact: %v", err)
	}
	// Page 1 secret should be gone from the (decoded/rewritten) stream.
	if streamContains(out, "page1secret") {
		t.Error("page 1 secret text not redacted")
	}
	// Page 2 was not redacted; verify the PDF structure is still intact and parseable.
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("redacted two-page PDF not parseable: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("no objects in redacted two-page PDF")
	}
	// /Count 2 must still be present (both pages remain).
	if !bytes.Contains(out, []byte("/Count 2")) {
		t.Error("page count changed after single-page redaction")
	}
}

// streamContains searches for needle inside the decoded stream data of the PDF.
// It scans between "stream\n" and "endstream" markers in the raw bytes.
func streamContains(pdfBytes []byte, needle string) bool {
	s := string(pdfBytes)
	for {
		si := strings.Index(s, "stream")
		if si == -1 {
			break
		}
		// Skip past "stream" + EOL.
		end := strings.Index(s[si:], "endstream")
		if end == -1 {
			break
		}
		chunk := s[si : si+end]
		if strings.Contains(chunk, needle) {
			return true
		}
		s = s[si+end+9:]
	}
	return false
}
