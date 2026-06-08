package manipulate

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// buildTwoPagePDF creates a two-page PDF with no content.
func buildTwoPagePDF(t *testing.T) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(write.PageSizeLetter))
	b.FinalizePage(b.AddPage(write.PageSizeLetter))
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

// buildOnePager creates a one-page PDF with no fonts or content.
func buildOnePager(t *testing.T) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(write.PageSizeLetter))
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

func TestStampText_ContentsBecomesArray(t *testing.T) {
	src := buildOnePager(t)
	m, err := NewPDFManipulator(src, nil, false)
	if err != nil {
		t.Fatalf("NewPDFManipulator: %v", err)
	}
	if err := m.StampText(1, TextStamp{Text: "DRAFT", X: 100, Y: 400}); err != nil {
		t.Fatalf("StampText: %v", err)
	}
	out, err := m.Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	// The rebuilt PDF must contain /Contents with an array (two streams: original + stamp).
	if !bytes.Contains(out, []byte("/Contents [")) && !bytes.Contains(out, []byte("/Contents[")) {
		t.Error("/Contents should become an array after stamp")
	}
	// The stamp text must appear in the content stream.
	if !bytes.Contains(out, []byte("DRAFT")) {
		t.Error("stamp text 'DRAFT' not found in output")
	}
}

func TestStampText_FontInjected(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "X", FontName: "Times-Roman", FontSize: 14, X: 50, Y: 50})
	out, _ := m.Rebuild()
	if !bytes.Contains(out, []byte("Times-Roman")) {
		t.Error("font 'Times-Roman' not found in rebuilt PDF")
	}
}

func TestStampText_ExistingFontPreserved(t *testing.T) {
	// Build a PDF that already has a Helvetica font reference.
	b := write.NewSimplePDFBuilder()
	p := b.AddPage(write.PageSizeLetter)
	fn := p.AddStandardFont("Helvetica")
	p.Content().BeginText().SetFont(fn, 12).ShowText("existing").EndText()
	b.FinalizePage(p)
	src, _ := b.Bytes()

	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "stamp", FontName: "Courier", FontSize: 10, X: 50, Y: 50})
	out, _ := m.Rebuild()

	if !bytes.Contains(out, []byte("Helvetica")) {
		t.Error("original Helvetica reference was lost")
	}
	if !bytes.Contains(out, []byte("Courier")) {
		t.Error("new Courier stamp font not found")
	}
	if !bytes.Contains(out, []byte("stamp")) {
		t.Error("stamp text not found")
	}
}

func TestStampAllPages_EveryPage(t *testing.T) {
	src := buildTwoPagePDF(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampAllPages(TextStamp{Text: "CONFIDENTIAL", X: 100, Y: 100})
	out, _ := m.Rebuild()

	// Count occurrences of "CONFIDENTIAL" — should appear twice (once per page stream).
	count := strings.Count(string(out), "CONFIDENTIAL")
	if count < 2 {
		t.Errorf("expected 'CONFIDENTIAL' on at least 2 pages, got %d occurrences", count)
	}
}

func TestStampText_Rotation(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "rotated", X: 306, Y: 396, Angle: 45})
	out, _ := m.Rebuild()
	// A 45-degree rotation should produce non-unit cos/sin values in the Tm operator.
	// cos(45°) ≈ 0.707107 — check for partial match.
	if !bytes.Contains(out, []byte("0.707107")) {
		t.Error("expected rotation matrix values in content stream")
	}
}

func TestStampText_Opacity(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "transparent", X: 100, Y: 400, Opacity: 0.3})
	out, _ := m.Rebuild()
	if !bytes.Contains(out, []byte("ExtGState")) {
		t.Error("expected ExtGState resource for opacity stamp")
	}
	if !bytes.Contains(out, []byte("0.3000")) {
		t.Error("expected opacity value 0.3000 in output")
	}
}

func TestStampPageNumbers_BottomCenter(t *testing.T) {
	src := buildTwoPagePDF(t)
	m, _ := NewPDFManipulator(src, nil, false)
	err := m.StampPageNumbers(PageNumberOptions{Format: "- %d -"})
	if err != nil {
		t.Fatalf("StampPageNumbers: %v", err)
	}
	out, _ := m.Rebuild()
	// Should contain page numbers 1 and 2.
	if !bytes.Contains(out, []byte("- 1 -")) {
		t.Error("page 1 number not found")
	}
	if !bytes.Contains(out, []byte("- 2 -")) {
		t.Error("page 2 number not found")
	}
}

func TestStampPageNumbers_TotalFormat(t *testing.T) {
	src := buildTwoPagePDF(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampPageNumbers(PageNumberOptions{Format: "%d of %d"})
	out, _ := m.Rebuild()
	if !bytes.Contains(out, []byte("1 of 2")) {
		t.Error("'1 of 2' not found in output")
	}
	if !bytes.Contains(out, []byte("2 of 2")) {
		t.Error("'2 of 2' not found in output")
	}
}

func TestStampText_ParsedByParser(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	_ = m.StampText(1, TextStamp{Text: "parseable", X: 72, Y: 720})
	out, _ := m.Rebuild()

	// The rebuilt PDF must be parseable by the parse package.
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("rebuilt PDF cannot be parsed: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("rebuilt PDF has no objects")
	}
}

func TestStampText_OOBPage(t *testing.T) {
	src := buildOnePager(t)
	m, _ := NewPDFManipulator(src, nil, false)
	err := m.StampText(5, TextStamp{Text: "x"})
	if err == nil {
		t.Error("expected error for out-of-bounds page number")
	}
}

// TestStampFontKey verifies font key sanitization.
func TestStampFontKey(t *testing.T) {
	cases := map[string]string{
		"Helvetica":           "F_Helvetica",
		"Helvetica-Bold":      "F_Helvetica_Bold",
		"Times-Roman":         "F_Times_Roman",
		"Courier-BoldOblique": "F_Courier_BoldOblique",
	}
	for in, want := range cases {
		if got := stampFontKey(in); got != want {
			t.Errorf("stampFontKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestStampParseMediaBox verifies MediaBox parsing.
func TestStampParseMediaBox(t *testing.T) {
	pageStr := `5 0 obj
<</Type/Page/MediaBox[0 0 612 792]/Contents 2 0 R>>
endobj`
	w, h := stampParseMediaBox(pageStr)
	if w != 612 || h != 792 {
		t.Errorf("want 612×792, got %v×%v", w, h)
	}
}

// TestStampFormatPageNumber verifies the format helper.
func TestStampFormatPageNumber(t *testing.T) {
	cases := []struct {
		fmt         string
		page, total int
		want        string
	}{
		{"%d", 3, 10, "3"},
		{"- %d -", 3, 10, "- 3 -"},
		{"%d of %d", 3, 10, "3 of 10"},
		{"Page %d/%d", 1, 5, "Page 1/5"},
	}
	for _, c := range cases {
		got := stampFormatPageNumber(c.fmt, c.page, c.total)
		if got != c.want {
			t.Errorf("format %q page=%d total=%d: got %q, want %q", c.fmt, c.page, c.total, got, c.want)
		}
	}
}

// TestStampAppendContents verifies the /Contents array update.
func TestStampAppendContents(t *testing.T) {
	single := `5 0 obj\n<</Type/Page/Contents 3 0 R>>\nendobj`
	got := stampAppendContents(single, "7 0 R")
	if !strings.Contains(got, "[3 0 R 7 0 R]") {
		t.Errorf("single ref not converted to array: %q", got)
	}

	array := `5 0 obj\n<</Type/Page/Contents [3 0 R 4 0 R]>>\nendobj`
	got2 := stampAppendContents(array, "8 0 R")
	if !strings.Contains(got2, "8 0 R]") {
		t.Errorf("array not extended: %q", got2)
	}
}
