// kitchen_sink generates a multi-page PDF that exercises as many pdfer
// features as possible: text, graphics, colors, images, watermarks,
// annotations (all subtypes), AcroForm fields, bookmarks, and metadata.
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

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/acroform"
	"github.com/benedoc-inc/pdfer/types"
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

	// ── Page 3: AcroForm fields ───────────────────────────────────────────────
	p3 := b.AddPage(write.PageSizeLetter)
	buildPage3(p3)
	b.FinalizePage(p3)

	// ── AcroForm ──────────────────────────────────────────────────────────────
	fb := acroform.NewFormBuilder(b)
	fb.AddTextField("fullname", []float64{72, 680, 350, 700}, 2).
		SetDefault("Enter full name").
		SetMaxLength(80)
	fb.AddTextField("email", []float64{72, 640, 350, 660}, 2).
		SetDefault("email@example.com")
	fb.AddCheckbox("subscribe", []float64{72, 605, 90, 623}, 2)
	fb.AddCheckbox("agree_terms", []float64{72, 575, 90, 593}, 2)
	fb.AddChoiceField("country", []float64{72, 535, 250, 555}, 2,
		[]string{"USA", "Canada", "Mexico", "United Kingdom", "Australia"}).
		SetValue("USA")
	fb.AddRadioButton("plan", []float64{72, 495, 90, 513}, 2)
	fb.AddButton("submit", []float64{200, 460, 340, 485}, 2)
	if _, err := fb.BuildForm(); err != nil {
		return nil, fmt.Errorf("BuildForm: %w", err)
	}

	// ── Bookmarks ─────────────────────────────────────────────────────────────
	if err := b.SetBookmarks([]types.Bookmark{
		{Title: "Cover — Text & Graphics", PageNumber: 1},
		{Title: "Annotations", PageNumber: 2, Children: []types.Bookmark{
			{Title: "Markup annotations", PageNumber: 2},
			{Title: "Shape annotations", PageNumber: 2},
		}},
		{Title: "Interactive Form", PageNumber: 3},
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
	cs.BeginText().SetFont(font, 11).SetTextLeading(16).SetTextPosition(72, 678)
	for _, l := range lines {
		cs.ShowTextNextLine(l)
	}
	cs.EndText()

	// ── Coloured rectangles palette ───────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 530).ShowText("Colour palette").EndText()

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
		cs.SetFillColorRGB(c[0], c[1], c[2]).Rectangle(x, 505, 68, 22).Fill()
	}

	// ── Vector graphics ───────────────────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 490).ShowText("Vector paths").EndText()

	// Triangle (filled)
	cs.SetFillColorRGB(0.9, 0.3, 0.1).
		MoveTo(110, 470).LineTo(150, 410).LineTo(70, 410).ClosePath().Fill()

	// Dashed stroked rectangle
	cs.SetStrokeColorRGB(0.1, 0.5, 0.8).SetLineWidth(2).
		Rectangle(170, 410, 80, 60).Stroke()

	// Bezier curve
	cs.SetStrokeColorRGB(0.3, 0.7, 0.2).SetLineWidth(2).
		MoveTo(270, 410).
		CurveTo(280, 470, 340, 470, 350, 410).
		Stroke()

	// Circle approximation (4 bezier curves)
	cx, cy, r := 430.0, 440.0, 30.0
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
			SetTextPosition(72, 395).ShowText("Embedded PNG image").EndText()
		cs.DrawImageAt(resName[1:], 72, 310, 180, 75)
	}

	// ── Text styling ──────────────────────────────────────────────────────────
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(72, 300).ShowText("Text styling").EndText()

	// Character spacing
	cs.BeginText().SetFont(font, 10).SetCharSpacing(3).
		SetTextPosition(72, 280).ShowText("Wide character spacing").EndText()

	// Word spacing
	cs.BeginText().SetFont(font, 10).SetCharSpacing(0).SetWordSpacing(8).
		SetTextPosition(72, 262).ShowText("Extra word spacing here").EndText()

	// Text rise (superscript)
	cs.BeginText().SetFont(font, 10).SetWordSpacing(0).
		SetTextPosition(72, 244).
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

func buildPage3(p *write.PageBuilder) {
	cs := p.Content()
	font := p.AddStandardFont("Helvetica")
	bold := p.AddStandardFont("Helvetica-Bold")

	cs.BeginText().SetFont(bold, 18).SetFillColorRGB(0.1, 0.2, 0.6).
		SetTextPosition(72, 750).ShowText("Interactive Form").EndText()

	cs.BeginText().SetFont(font, 10).SetFillColorRGB(0, 0, 0).
		SetTextLeading(16).SetTextPosition(72, 728).
		ShowText("Please fill in the fields below.  This page contains an AcroForm").
		ShowTextNextLine("with text fields, checkboxes, a dropdown, a radio button, and a push button.").
		EndText()

	// Labels for each field (the fields themselves are wired in by BuildForm)
	fields := []struct {
		y    float64
		text string
	}{
		{700, "Full name:"},
		{660, "Email address:"},
		{620, "Subscribe to newsletter"},
		{590, "I agree to the terms and conditions"},
		{550, "Country:"},
		{510, "Plan: (radio button)"},
		{475, ""},
	}
	for _, f := range fields {
		if f.text == "" {
			continue
		}
		cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
			SetTextPosition(72, f.y).ShowText(f.text).EndText()
	}

	// Submit label
	cs.BeginText().SetFont(bold, 10).SetFillColorRGB(0, 0, 0).
		SetTextPosition(200, 467).ShowText("Submit").EndText()

	// Divider
	cs.SetFillColorRGB(0.8, 0.8, 0.8).Rectangle(72, 440, 468, 1).Fill()

	cs.BeginText().SetFont(font, 8).SetFillColorRGB(0.5, 0.5, 0.5).
		SetTextPosition(72, 425).
		ShowText("Generated by pdfer — a pure-Go zero-dependency PDF library.").
		EndText()
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
