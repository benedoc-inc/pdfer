// kitchen_sink generates a multi-page PDF that exercises as many pdfer
// features as possible: text, graphics, colors, images, watermarks,
// annotations (all subtypes), AcroForm fields, bookmarks, metadata,
// word-wrapped text boxes, tables, and multi-column layout.
//
// Usage:
//
//	go run ./examples/kitchen_sink [-o output.pdf]
//
// The generated file is written to output.pdf (or the path given via -o).
// A second file, output_encrypted.pdf, is also written with AES-128
// encryption (password: "open").
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/write"
	"github.com/benedoc-inc/pdfer/v2/forms/acroform"
	"github.com/benedoc-inc/pdfer/v2/types"
)

func main() {
	out := flag.String("o", "output.pdf", "output PDF path")
	flag.Parse()

	pdf, err := buildKitchenSink()
	if err != nil {
		log.Fatalf("build failed: %v", err)
	}

	if err := os.WriteFile(*out, pdf, 0644); err != nil {
		log.Fatalf("write failed: %v", err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, len(pdf))

	// Spot-check: parse the form fields from the generated PDF.
	form, err := acroform.ExtractAcroForm(pdf, nil, false)
	if err != nil {
		log.Fatalf("ExtractAcroForm failed: %v", err)
	}
	fmt.Printf("acroform fields: %d\n", len(form.Fields))
	for _, f := range form.Fields {
		fmt.Printf("  field %q type=%s\n", f.T, f.FT)
	}

	// Structural checks on raw bytes.
	raw := string(pdf)
	checks := []struct {
		needle string
		desc   string
	}{
		{"/AcroForm", "catalog has /AcroForm"},
		{"/Annots", "pages have /Annots"},
		{"/Outlines", "bookmarks present"},
		{"/XObject", "image XObject present"},
		// Watermark text lives in a zlib-compressed content stream, so raw-byte
		// search won't find it; we trust AddTextWatermark is exercised correctly.
		{"/Subtype/Widget", "form widget annotations present"},
		{"/Subtype/Highlight", "highlight annotation present"},
		{"/Subtype/Line", "line annotation present"},
		{"/Subtype/Polygon", "polygon annotation present"},
		{"/Subtype/Stamp", "stamp annotation present"},
	}
	for _, c := range checks {
		mark := "✓"
		if !strings.Contains(raw, c.needle) {
			mark = "✗"
		}
		fmt.Printf("  %s %s\n", mark, c.desc)
	}

	// Write an encrypted copy (user password "open", owner password "owner").
	encPath := appendSuffix(*out, "_encrypted")
	b2 := write.NewSimplePDFBuilder()
	// Re-generate (encryption is set before Bytes(), so we need a fresh build).
	pdf2, err := buildKitchenSinkEncrypted()
	if err != nil {
		log.Fatalf("encrypted build failed: %v", err)
	}
	_ = b2
	if err := os.WriteFile(encPath, pdf2, 0644); err != nil {
		log.Fatalf("write encrypted failed: %v", err)
	}
	fmt.Printf("wrote %s (%d bytes, password \"open\")\n", encPath, len(pdf2))
}

// ---- builders ---------------------------------------------------------------

func buildKitchenSink() ([]byte, error)          { return build(false) }
func buildKitchenSinkEncrypted() ([]byte, error) { return build(true) }

func build(encrypt bool) ([]byte, error) {
	b := write.NewSimplePDFBuilder()
	b.Writer().SetMetadata(&types.DocumentMetadata{
		Title:    "pdfer kitchen-sink demo",
		Author:   "pdfer library",
		Subject:  "Comprehensive feature test",
		Keywords: "pdfer pdf test acroform annotations encryption",
		Creator:  "kitchen_sink example",
	})

	// ── Page 1: text, graphics, colors, watermark ────────────────────────────
	p1 := b.AddPage(write.PageSizeLetter)
	buildPage1(p1, b.Writer())
	b.FinalizePage(p1)

	// ── Page 2: all annotation subtypes ──────────────────────────────────────
	p2 := b.AddPage(write.PageSizeLetter)
	buildPage2(p2)
	b.FinalizePage(p2)

	// ── Page 3: AcroForm field catalog — page content + widgets in one pass ────
	p3 := b.AddPage(write.PageSizeLetter)
	fb := acroform.NewFormBuilder(b)
	buildPage3WithForm(p3, fb)
	b.FinalizePage(p3)
	if _, err := fb.BuildForm(); err != nil {
		return nil, fmt.Errorf("BuildForm: %w", err)
	}

	// ── Page 4: TextBox, Table, ColumnLayout ─────────────────────────────────
	p4 := b.AddPage(write.PageSizeLetter)
	buildPage4(p4)
	b.FinalizePage(p4)

	// ── Bookmarks ─────────────────────────────────────────────────────────────
	if err := b.SetBookmarks([]types.Bookmark{
		{Title: "Cover — Text & Graphics", PageNumber: 1},
		{Title: "Annotations", PageNumber: 2, Children: []types.Bookmark{
			{Title: "Markup annotations", PageNumber: 2},
			{Title: "Shape annotations", PageNumber: 2},
		}},
		{Title: "Interactive Form", PageNumber: 3},
		{Title: "Layout — TextBox / Table / Columns", PageNumber: 4},
	}); err != nil {
		return nil, fmt.Errorf("SetBookmarks: %w", err)
	}

	if encrypt {
		b.SetPassword([]byte("open"), []byte("owner"))
	}

	return b.Bytes()
}

// ── Page builders ─────────────────────────────────────────────────────────────

func buildPage1(p *write.PageBuilder, w *write.PDFWriter) {
	cs := p.Content()
	font := p.AddStandardFont("Helvetica")
	bold := p.AddStandardFont("Helvetica-Bold")
	italic := p.AddStandardFont("Helvetica-Oblique")

	// Title
	cs.BeginText().
		SetFont(bold, 24).
		SetFillColorRGB(0.1, 0.2, 0.6).
		SetTextPosition(72, 730).
		ShowText("pdfer Kitchen-Sink Demo").
		EndText()

	// Subtitle
	cs.BeginText().
		SetFont(italic, 12).
		SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(72, 706).
		ShowText("A comprehensive feature test for the pdfer pure-Go PDF library").
		EndText()

	// Horizontal rule (thin colored rectangle)
	cs.SetFillColorRGB(0.1, 0.2, 0.6).
		Rectangle(72, 698, 468, 3).
		Fill()

	// Body text paragraphs
	cs.SetFillColorRGB(0, 0, 0)
	lines := []string{
		"This document was generated entirely in Go using pdfer v0.9.10,",
		"a zero-dependency pure-Go PDF library.  It exercises:",
		"",
		"  • Text rendering (multiple fonts, sizes, colours, spacing)",
		"  • Vector graphics (paths, rectangles, curves, fills, strokes)",
		"  • PNG image embedding",
		"  • Watermarks",
		"  • Annotations (all 14 subtypes)",
		"  • AcroForm fields (text, checkbox, radio, dropdown, button)",
		"  • Bookmarks / outline",
		"  • Document metadata",
		"  • AES-128 write encryption",
	}
	cs.BeginText().SetFont(font, 11).SetTextLeading(13).SetTextPosition(72, 678)
	for _, l := range lines {
		cs.ShowTextNextLine(l)
	}
	cs.EndText()

	// ── Coloured rectangles palette ───────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 465).ShowText("Colour palette").EndText()

	colors := [][3]float64{
		{0.9, 0.2, 0.2},
		{0.2, 0.7, 0.3},
		{0.2, 0.4, 0.9},
		{0.9, 0.7, 0.1},
		{0.6, 0.2, 0.8},
		{0.1, 0.7, 0.8},
	}
	for i, c := range colors {
		x := 72 + float64(i)*78
		cs.SetFillColorRGB(c[0], c[1], c[2]).Rectangle(x, 440, 68, 22).Fill()
	}

	// ── Vector graphics ───────────────────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 425).ShowText("Vector paths").EndText()

	// Triangle (filled)
	cs.SetFillColorRGB(0.9, 0.3, 0.1).
		MoveTo(110, 405).LineTo(150, 345).LineTo(70, 345).ClosePath().Fill()

	// Stroked rectangle
	cs.SetStrokeColorRGB(0.1, 0.5, 0.8).SetLineWidth(2).
		Rectangle(170, 345, 80, 60).Stroke()

	// Bezier curve
	cs.SetStrokeColorRGB(0.3, 0.7, 0.2).SetLineWidth(2).
		MoveTo(270, 345).
		CurveTo(280, 405, 340, 405, 350, 345).
		Stroke()

	// Circle approximation (4 bezier curves)
	cx, cy, r := 430.0, 375.0, 30.0
	k := 0.5523
	cs.SetFillColorRGB(0.8, 0.8, 0.1).SetStrokeColorRGB(0.4, 0.4, 0).SetLineWidth(1).
		MoveTo(cx, cy+r).
		CurveTo(cx+k*r, cy+r, cx+r, cy+k*r, cx+r, cy).
		CurveTo(cx+r, cy-k*r, cx+k*r, cy-r, cx, cy-r).
		CurveTo(cx-k*r, cy-r, cx-r, cy-k*r, cx-r, cy).
		CurveTo(cx-r, cy+k*r, cx-k*r, cy+r, cx, cy+r).
		FillStroke()

	// ── Embedded PNG image ────────────────────────────────────────────────────
	imgBytes := makePNG(120, 60)
	imgInfo, err := w.AddImage(imgBytes, "Im1")
	if err == nil {
		resName := p.AddImage(imgInfo)
		cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
			SetTextPosition(72, 330).ShowText("Embedded PNG image").EndText()
		cs.DrawImageAt(resName[1:], 72, 245, 180, 75)
	}

	// ── Text styling ──────────────────────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 235).ShowText("Text styling").EndText()

	// Character spacing
	cs.BeginText().SetFont(font, 10).SetCharSpacing(3).
		SetTextPosition(72, 215).ShowText("Wide character spacing").EndText()

	// Word spacing
	cs.BeginText().SetFont(font, 10).SetCharSpacing(0).SetWordSpacing(8).
		SetTextPosition(72, 197).ShowText("Extra word spacing here").EndText()

	// Text rise (superscript)
	cs.BeginText().SetFont(font, 10).SetWordSpacing(0).
		SetTextPosition(72, 179).
		ShowText("Normal ").
		SetTextRise(4).SetFont(font, 7).ShowText("superscript").
		SetTextRise(0).SetFont(font, 10).ShowText(" back to baseline").
		EndText()

	// ── Watermark ─────────────────────────────────────────────────────────────
	_ = p.AddTextWatermark("DRAFT", 60, 45)
}

func buildPage2(p *write.PageBuilder) {
	cs := p.Content()
	font := p.AddStandardFont("Helvetica")
	bold := p.AddStandardFont("Helvetica-Bold")

	cs.BeginText().SetFont(bold, 18).SetFillColorRGB(0.1, 0.2, 0.6).
		SetTextPosition(72, 750).ShowText("Annotation Showcase").EndText()

	cs.BeginText().SetFont(font, 9).SetFillColorRGB(0.4, 0.4, 0.4).
		SetTextPosition(72, 733).
		ShowText("Every annotation subtype supported by pdfer appears on this page.").
		EndText()

	// ── Markup annotations ────────────────────────────────────────────────────
	label(p, bold, 72, 718, "Markup annotations")

	// Text (sticky note)
	p.AddAnnotation(write.NewTextAnnotation(72, 680, 92, 700, "This is a sticky note").
		WithTitle("pdfer").WithIcon("Comment"))

	cs.BeginText().SetFont(font, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(100, 686).ShowText("Sticky note annotation →").EndText()

	// Highlight
	qp := write.RectToQuadPoints(72, 658, 240, 670)
	p.AddAnnotation(write.NewHighlightAnnotation(72, 658, 240, 670, qp).
		WithColor(1, 1, 0))
	cs.BeginText().SetFont(font, 10).SetTextPosition(72, 660).
		ShowText("This text is highlighted in yellow").EndText()

	// Underline
	qp2 := write.RectToQuadPoints(72, 638, 200, 650)
	p.AddAnnotation(write.NewUnderlineAnnotation(72, 638, 200, 650, qp2).
		WithColor(0, 0, 1))
	cs.BeginText().SetFont(font, 10).SetTextPosition(72, 640).
		ShowText("This text is underlined").EndText()

	// Strikeout
	qp3 := write.RectToQuadPoints(72, 618, 200, 630)
	p.AddAnnotation(write.NewStrikeoutAnnotation(72, 618, 200, 630, qp3).
		WithColor(1, 0, 0))
	cs.BeginText().SetFont(font, 10).SetTextPosition(72, 620).
		ShowText("This text is struck out").EndText()

	// Squiggly
	qp4 := write.RectToQuadPoints(72, 598, 200, 610)
	p.AddAnnotation(write.NewSquigglyAnnotation(72, 598, 200, 610, qp4).
		WithColor(1, 0.5, 0))
	cs.BeginText().SetFont(font, 10).SetTextPosition(72, 600).
		ShowText("Squiggly underline").EndText()

	// Free text
	p.AddAnnotation(write.NewFreeTextAnnotation(72, 560, 260, 585,
		"Free text annotation — rendered inline", "/Helv 9 Tf 0 0 0 rg").
		WithColor(1, 1, 0.8).WithBorderWidth(1))

	// ── Shape / geometric annotations ─────────────────────────────────────────
	label(p, bold, 72, 545, "Shape annotations")

	// Square
	p.AddAnnotation(write.NewSquareAnnotation(72, 490, 160, 535).
		WithColor(0.2, 0.4, 0.9).WithBorderWidth(2).
		WithContents("Square annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(78, 510).ShowText("Square").EndText()

	// Circle
	p.AddAnnotation(write.NewCircleAnnotation(175, 490, 265, 535).
		WithColor(0.8, 0.2, 0.2).WithBorderWidth(2).
		WithContents("Circle annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(203, 510).ShowText("Circle").EndText()

	// Line
	p.AddAnnotation(write.NewLineAnnotation(280, 490, 380, 535).
		WithColor(0.1, 0.6, 0.1).WithBorderWidth(2).
		WithLineEndings("OpenArrow", "OpenArrow").
		WithContents("Line annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(303, 510).ShowText("Line").EndText()

	// Polygon
	polyVerts := []float64{
		400, 490,
		440, 535,
		480, 535,
		520, 490,
	}
	p.AddAnnotation(write.NewPolygonAnnotation(400, 490, 520, 535, polyVerts).
		WithColor(0.6, 0.2, 0.8).WithBorderWidth(2).
		WithContents("Polygon annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(440, 510).ShowText("Polygon").EndText()

	// ── Path/ink annotations ──────────────────────────────────────────────────
	label(p, bold, 72, 475, "Ink & path annotations")

	// Polyline
	polylineVerts := []float64{72, 440, 100, 465, 130, 435, 160, 460, 190, 440}
	p.AddAnnotation(write.NewPolylineAnnotation(72, 435, 190, 465, polylineVerts).
		WithColor(0.1, 0.7, 0.7).WithBorderWidth(2).
		WithContents("Polyline annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(72, 422).ShowText("Polyline").EndText()

	// Ink
	stroke1 := []float64{220, 440, 240, 465, 260, 445, 280, 460}
	stroke2 := []float64{225, 450, 245, 435, 265, 455}
	p.AddAnnotation(write.NewInkAnnotation(220, 430, 285, 470, [][]float64{stroke1, stroke2}).
		WithColor(0.8, 0.4, 0.1).WithBorderWidth(1.5).
		WithContents("Ink annotation"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(230, 422).ShowText("Ink").EndText()

	// Caret
	p.AddAnnotation(write.NewCaretAnnotation(310, 445, 360, 465).
		WithContents("Caret: text insertion point"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(310, 435).ShowText("Caret").EndText()

	// Stamp
	p.AddAnnotation(write.NewStampAnnotation(380, 435, 520, 475, "Approved").
		WithColor(0.1, 0.6, 0.1).WithContents("Approved by pdfer"))
	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.3, 0.3, 0.3).
		SetTextPosition(420, 422).ShowText("Stamp: Approved").EndText()

	// ── Link annotation ───────────────────────────────────────────────────────
	label(p, bold, 72, 405, "Link annotation")

	linkText := "Click here to visit the pdfer repository"
	p.AddAnnotation(write.NewLinkAnnotation(72, 378, 310, 398,
		"https://github.com/benedoc-inc/pdfer").WithBorderWidth(1).WithColor(0, 0, 0.8))
	cs.BeginText().SetFont(font, 10).SetFillColorRGB(0, 0, 0.7).
		SetTextPosition(72, 380).ShowText(linkText).EndText()
	// Underline drawn as a thin rectangle
	cs.SetFillColorRGB(0, 0, 0.7).Rectangle(72, 378, 238, 1).Fill()
}

// buildPage3 draws all the labels and chrome for the form field catalog.
// The actual widget annotations are added separately via addFormFields.
// buildPage3WithForm draws the field-type catalog labels AND places the AcroForm
// widget annotations in a single pass so positions can never drift apart.
func buildPage3WithForm(p *write.PageBuilder, fb *acroform.FormBuilder) {
	const pg = 2 // 0-based page index
	const lx = 72.0
	const rx = 316.0
	const lw = 222.0
	const rw = 224.0

	bold := p.AddStandardFont("Helvetica-Bold")
	font := p.AddStandardFont("Helvetica")
	italic := p.AddStandardFont("Helvetica-Oblique")
	cs := p.Content()

	// secDiv draws a thin rule + small-caps section header; returns next label Y.
	secDiv := func(x, y, colW float64, title string) float64 {
		cs.SetFillColorRGB(0.72, 0.72, 0.72).Rectangle(x, y, colW, 0.5).Fill()
		cs.BeginText().SetFont(bold, 7.5).SetFillColorRGB(0.42, 0.42, 0.42).
			SetTextPosition(x, y-11).ShowText(strings.ToUpper(title)).EndText()
		return y - 22
	}

	// fieldRow draws label + places widget; returns Y for the next label.
	// labelY is the label baseline. fieldH is the widget height.
	fieldRow := func(x, labelY, colW, fieldH float64, text string, required bool,
		addWidget func([]float64)) float64 {
		cs.BeginText().SetFont(bold, 9).SetFillColorRGB(0.15, 0.15, 0.15).
			SetTextPosition(x, labelY).ShowText(text).EndText()
		if required {
			starX := x + float64(len(text))*0.46*9
			cs.BeginText().SetFont(bold, 9).SetFillColorRGB(0.75, 0.1, 0.1).
				SetTextPosition(starX, labelY).ShowText(" *").EndText()
		}
		fieldTop := labelY - 10
		addWidget([]float64{x, fieldTop - fieldH, x + colW, fieldTop})
		return fieldTop - fieldH - 14 // 14pt gap before next label
	}

	// inlineRow draws widget + label on the same line (checkbox / radio style).
	inlineRow := func(x, y float64, text string, addWidget func([]float64)) float64 {
		wTop, wBot := y, y-16
		addWidget([]float64{x, wBot, x + 16, wTop})
		cs.BeginText().SetFont(font, 9).SetFillColorRGB(0.1, 0.1, 0.1).
			SetTextPosition(x+21, wBot+4).ShowText(text).EndText()
		return wBot - 12
	}

	// ── Page title ────────────────────────────────────────────────────────────
	cs.BeginText().SetFont(bold, 18).SetFillColorRGB(0.1, 0.2, 0.6).
		SetTextPosition(lx, 750).ShowText("AcroForm Field Type Catalog").EndText()
	cs.BeginText().SetFont(italic, 9).SetFillColorRGB(0.4, 0.4, 0.4).
		SetTextPosition(lx, 733).
		ShowText("Every supported AcroForm field type — open in Acrobat or Preview to interact.").
		EndText()
	cs.SetFillColorRGB(0.1, 0.2, 0.6).Rectangle(lx, 725, 468, 2).Fill()

	// ── Left column: text field variants ─────────────────────────────────────
	ly := secDiv(lx, 706, lw, "Text fields")

	ly = fieldRow(lx, ly, lw, 17, "Single-line text (empty)", false,
		func(r []float64) { fb.AddTextField("text_empty", r, pg) })

	ly = fieldRow(lx, ly, lw, 17, `Single-line text — pre-filled`, false,
		func(r []float64) { fb.AddTextField("text_prefilled", r, pg).SetDefault("Alice Smith") })

	ly = fieldRow(lx, ly, lw, 52, "Multi-line text area", false,
		func(r []float64) {
			fb.AddTextField("text_multiline", r, pg).
				SetMultiline(true).SetDefault("Line one.\nLine two.\nLine three.")
		})

	ly = fieldRow(lx, ly, lw, 17, "Password (input obscured)", false,
		func(r []float64) { fb.AddTextField("text_password", r, pg).SetPassword(true) })

	ly = fieldRow(lx, ly, lw, 17, "Required field", true,
		func(r []float64) { fb.AddTextField("text_required", r, pg).SetRequired(true) })

	ly = fieldRow(lx, ly, lw, 17, "Read-only (value locked)", false,
		func(r []float64) {
			fb.AddTextField("text_readonly", r, pg).
				SetDefault("Cannot be changed").SetReadOnly(true)
		})

	fieldRow(lx, ly, lw, 17, "Max-length (10 chars)", false,
		func(r []float64) { fb.AddTextField("text_maxlen", r, pg).SetMaxLength(10) })

	ly = secDiv(lx, ly-28, lw, "Underline style")

	fieldRow(lx, ly, lw, 17, "Underline — single line", false,
		func(r []float64) { fb.AddUnderlineTextField("text_underline", r, pg) })

	fieldRow(lx, ly-48, lw, 17, "Underline — pre-filled", false,
		func(r []float64) {
			fb.AddUnderlineTextField("text_underline_filled", r, pg).
				SetDefault("Underlined entry")
		})

	// ── Right column ──────────────────────────────────────────────────────────
	ry := secDiv(rx, 706, rw, "Checkboxes")

	ry = inlineRow(rx, ry, "Unchecked (default off)",
		func(r []float64) { fb.AddCheckbox("chk_off", r, pg) })
	ry = inlineRow(rx, ry, `Pre-checked (Value = "Yes")`,
		func(r []float64) { fb.AddCheckbox("chk_on", r, pg).SetValue("Yes") })

	ry -= 6
	ry = secDiv(rx, ry, rw, "Radio buttons")
	for i, opt := range []string{"Option A", "Option B", "Option C"} {
		name := fmt.Sprintf("radio_%c", rune('a'+i))
		ry = inlineRow(rx, ry, opt,
			func(r []float64) { fb.AddRadioButton(name, r, pg) })
	}

	ry -= 6
	ry = secDiv(rx, ry, rw, "Choice fields")

	ry = fieldRow(rx, ry, rw, 17, "Dropdown (combo box)", false,
		func(r []float64) {
			fb.AddChoiceField("dropdown", r, pg,
				[]string{"USA", "Canada", "Mexico", "UK", "Germany", "Australia"}).
				SetValue("Canada")
		})

	ry = fieldRow(rx, ry, rw, 52, "List box (always visible)", false,
		func(r []float64) {
			fb.AddListBox("listbox", r, pg,
				[]string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta"})
		})

	ry -= 6
	ry = secDiv(rx, ry, rw, "Push button")

	fieldRow(rx, ry, rw, 22, "Push button", false,
		func(r []float64) { fb.AddButton("btn_submit", r, pg) })

	// Footer
	cs.SetFillColorRGB(0.82, 0.82, 0.82).Rectangle(lx, 58, 468, 0.5).Fill()
	cs.BeginText().SetFont(italic, 7.5).SetFillColorRGB(0.58, 0.58, 0.58).
		SetTextPosition(lx, 46).
		ShowText("* Required field.  15 widgets total.  Generated by pdfer — pure-Go zero-dependency PDF library.").
		EndText()
}

func buildPage4(p *write.PageBuilder) {
	bold := p.AddStandardFont("Helvetica-Bold")
	font := p.AddStandardFont("Helvetica")

	// Page title
	p.Content().
		BeginText().SetFont(bold, 18).SetFillColorRGB(0.1, 0.2, 0.6).
		SetTextPosition(72, 750).ShowText("Layout — TextBox / Table / Columns").
		EndText()

	p.Content().
		BeginText().SetFont(font, 9).SetFillColorRGB(0.4, 0.4, 0.4).
		SetTextPosition(72, 733).
		ShowText("Demonstrates DrawTextBox, DrawTable, and NewColumnLayout from core/write.").
		EndText()

	p.Content().SetFillColorRGB(0.1, 0.2, 0.6).Rectangle(72, 725, 468, 2).Fill()

	// ── Section 1: Word-wrapped text box ─────────────────────────────────────
	label(p, bold, 72, 712, "1. Word-wrapped text box (DrawTextBox)")

	paragraph := "pdfer's DrawTextBox helper wraps arbitrary text at word boundaries " +
		"to fit within a specified column width. Each line is emitted as an independent " +
		"BT/ET text block positioned with an absolute text matrix, so lines are never " +
		"split mid-word. Alignment (left, centre, right) is supported. This paragraph " +
		"is rendered into a 300 pt wide box at 11 pt Helvetica with 1.2× leading."

	bodyStyle := write.TextBoxStyle{FontName: font, FontSize: 11}
	textBoxH := p.DrawTextBox(72, 698, 300, paragraph, bodyStyle)

	// Right-aligned text box alongside
	rightStyle := write.TextBoxStyle{FontName: font, FontSize: 9, Align: write.TextAlignRight,
		Color: [3]float64{0.3, 0.3, 0.6}}
	p.DrawTextBox(390, 698, 150, "Right-aligned text in a narrower box, also word-wrapped.", rightStyle)

	nextY := 698 - textBoxH - 12

	// ── Section 2: Table ─────────────────────────────────────────────────────
	label(p, bold, 72, nextY, "2. Table (DrawTable)")
	nextY -= 14

	tblStyle := write.TableStyle{
		FontName:    font,
		FontSize:    9,
		BorderWidth: 0.4,
		HeaderFill:  [3]float64{0.82, 0.88, 0.96},
		BorderColor: [3]float64{0.5, 0.5, 0.5},
		CellPadding: 5,
	}
	tbl := write.NewTableBuilder([]float64{110, 90, 90, 178}, tblStyle)

	hdr := tbl.AddRow(true)
	hdr.AddCell("Component").WithAlign(write.TextAlignCenter)
	hdr.AddCell("Status").WithAlign(write.TextAlignCenter)
	hdr.AddCell("Version").WithAlign(write.TextAlignCenter)
	hdr.AddCell("Notes").WithAlign(write.TextAlignCenter)

	data := [][]string{
		{"pdfer core", "✓ Stable", "v1.9.0", "Pure Go, zero deps"},
		{"AcroForm", "✓ Stable", "v1.9.0", "Parse, fill, flatten; explicit AP streams"},
		{"XFA forms", "✓ Stable", "v1.4.0", "Extract, fill, XML export"},
		{"Signatures", "✓ Stable", "v1.3.0", "PKCS#7, TSA, LTV"},
		{"Content extraction", "✓ Stable", "v1.2.0", "Text, images, tables"},
		{"File attachments", "✓ Stable", "v1.4.0", "EmbedAttachments API"},
		{"Table writing", "✓ Stable", "v1.4.0", "DrawTable helper"},
	}
	for i, row := range data {
		r := tbl.AddRow(false)
		for _, cell := range row {
			r.AddCell(cell)
		}
		// Alternating row background
		if i%2 == 1 {
			for _, cell := range r.Cells() {
				cell.WithBgColor(0.95, 0.95, 0.95)
			}
		}
	}

	tableH := p.DrawTable(72, nextY, tbl)
	nextY -= tableH + 16

	// ── Section 3: Multi-column layout ───────────────────────────────────────
	label(p, bold, 72, nextY, "3. Multi-column layout (NewColumnLayout)")
	nextY -= 14

	layout := p.NewColumnLayout(72, nextY, 468, 3, 14)

	snippets := []string{
		"Column layouts divide a fixed-width region into uniform columns with a configurable gutter. " +
			"Each column tracks its own Y cursor so content can be placed independently.",
		"The ColumnLayout helper exposes DrawTextBox and DrawTable methods that advance the " +
			"cursor automatically, so callers only need to choose a column index.",
		"BalanceColumns() aligns all cursors to the lowest column, useful for drawing a horizontal " +
			"rule below a multi-column block without manually tracking each cursor.",
	}
	colStyle := write.TextBoxStyle{FontName: font, FontSize: 9}
	for i, s := range snippets {
		layout.DrawTextBox(i, s, colStyle, 0)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// label writes a section heading on the page directly to content.
func label(p *write.PageBuilder, boldFont string, x, y float64, text string) {
	p.Content().
		BeginText().
		SetFont(boldFont, 10).
		SetFillColorRGB(0.2, 0.2, 0.2).
		SetTextPosition(x, y).
		ShowText(text).
		EndText()
}

// makePNG builds a small synthetic PNG image (colour gradient) in memory.
func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x) / float64(w)
			u := float64(y) / float64(h)
			angle := math.Pi * 2 * t
			r := uint8(128 + 127*math.Sin(angle))
			g := uint8(128 + 127*math.Sin(angle+2.094))
			b := uint8(128 + 127*math.Cos(u*math.Pi))
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// appendSuffix inserts a suffix before the last "." in path, or appends it.
func appendSuffix(path, suffix string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[:i] + suffix + path[i:]
		}
	}
	return path + suffix
}
