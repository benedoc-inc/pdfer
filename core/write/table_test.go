package write_test

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/write"
)

func TestDrawTextBox_Basic(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	style := write.TextBoxStyle{
		FontName: fontName,
		FontSize: 12,
	}
	h := page.DrawTextBox(72, 700, 468, "Hello, world!", style)
	if h <= 0 {
		t.Fatalf("expected positive height, got %f", h)
	}

	b.FinalizePage(page)
	pdf, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
	// Streams are compressed; just verify we got a valid non-trivial PDF.
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Error("output is not a valid PDF")
	}
	if len(pdf) < 500 {
		t.Errorf("PDF too small (%d bytes)", len(pdf))
	}
}

func TestDrawTextBox_WordWrap(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	style := write.TextBoxStyle{
		FontName: fontName,
		FontSize: 12,
	}
	// Narrow box forces wrapping
	h := page.DrawTextBox(72, 700, 60, "This is a longer sentence that should wrap across multiple lines", style)
	if h <= 14 {
		t.Fatalf("expected multi-line height, got %f", h)
	}
	_ = h

	b.FinalizePage(page)
	_, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
}

func TestDrawTable_Basic(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	style := write.TableStyle{
		FontName: fontName,
		FontSize: 10,
	}
	tbl := write.NewTableBuilder([]float64{150, 150, 150}, style)

	hdr := tbl.AddRow(true)
	hdr.AddCell("Name")
	hdr.AddCell("Value")
	hdr.AddCell("Notes")

	row1 := tbl.AddRow(false)
	row1.AddCell("Alpha")
	row1.AddCell("1")
	row1.AddCell("First item")

	row2 := tbl.AddRow(false)
	row2.AddCell("Beta")
	row2.AddCell("2")
	row2.AddCell("Second item")

	h := page.DrawTable(72, 700, tbl)
	if h <= 0 {
		t.Fatalf("expected positive height, got %f", h)
	}

	b.FinalizePage(page)
	pdf, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Error("output is not a valid PDF")
	}
	if len(pdf) < 500 {
		t.Errorf("PDF too small (%d bytes)", len(pdf))
	}
}

func TestDrawTable_ColSpan(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	style := write.TableStyle{FontName: fontName, FontSize: 10}
	tbl := write.NewTableBuilder([]float64{100, 100, 100}, style)

	hdr := tbl.AddRow(true)
	hdr.AddCell("Spanning Header").WithColSpan(3)

	row := tbl.AddRow(false)
	row.AddCell("A")
	row.AddCell("B")
	row.AddCell("C")

	h := page.DrawTable(72, 700, tbl)
	if h <= 0 {
		t.Fatalf("expected positive height, got %f", h)
	}
	b.FinalizePage(page)
	if _, err := b.Bytes(); err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
}

func TestColumnLayout(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	fontName := page.AddStandardFont("Helvetica")

	layout := page.NewColumnLayout(72, 720, 468, 2, 12)
	if layout.Width() <= 0 {
		t.Fatalf("column width should be positive, got %f", layout.Width())
	}

	style := write.TextBoxStyle{FontName: fontName, FontSize: 10}
	layout.DrawTextBox(0, "Left column content for the first column.", style, 6)
	layout.DrawTextBox(1, "Right column content for the second column.", style, 6)

	if layout.CurrentY(0) >= 720 {
		t.Error("left column cursor should have advanced")
	}
	if layout.CurrentY(1) >= 720 {
		t.Error("right column cursor should have advanced")
	}

	b.FinalizePage(page)
	if _, err := b.Bytes(); err != nil {
		t.Fatalf("Bytes() error: %v", err)
	}
}
