package write_test

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/write"
)

// buildAnnotationPDF is a helper that creates a single-page PDF, adds the
// given annotations, and returns the raw PDF bytes.
func buildAnnotationPDF(t *testing.T, annotations ...*write.AnnotationBuilder) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	page.Content().
		BeginText().
		SetFont(page.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("Annotation test page").
		EndText()
	for _, a := range annotations {
		page.AddAnnotation(a)
	}
	builder.FinalizePage(page)
	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("failed to build PDF: %v", err)
	}
	return pdfBytes
}

func TestAnnotation_Link(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewLinkAnnotation(72, 700, 300, 720, "https://example.com"),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(doc.Pages))
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "link" {
		t.Errorf("expected type link, got %s", a.Type)
	}
	if !strings.Contains(a.URI, "example.com") {
		t.Errorf("expected URI to contain example.com, got %q", a.URI)
	}
}

func TestAnnotation_Text(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewTextAnnotation(72, 680, 200, 700, "My comment").
			WithTitle("Reviewer").
			WithOpen(true),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "text" {
		t.Errorf("expected type text, got %s", a.Type)
	}
	if a.Contents != "My comment" {
		t.Errorf("expected contents 'My comment', got %q", a.Contents)
	}
	if a.Title != "Reviewer" {
		t.Errorf("expected title 'Reviewer', got %q", a.Title)
	}
}

func TestAnnotation_Highlight(t *testing.T) {
	qp := write.RectToQuadPoints(72, 710, 300, 722)
	pdf := buildAnnotationPDF(t,
		write.NewHighlightAnnotation(72, 710, 300, 722, qp).
			WithContents("Important").
			WithColor(1, 1, 0),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "highlight" {
		t.Errorf("expected type highlight, got %s", a.Type)
	}
	if len(a.QuadPoints) == 0 {
		t.Error("expected QuadPoints to be populated")
	}
}

func TestAnnotation_Square(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewSquareAnnotation(72, 600, 300, 700).
			WithContents("Box").
			WithColor(1, 0, 0).
			WithBorderWidth(2),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "square" {
		t.Errorf("expected type square, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

func TestAnnotation_Circle(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewCircleAnnotation(72, 500, 200, 580).
			WithColor(0, 0, 1),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "circle" {
		t.Errorf("expected type circle, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

func TestAnnotation_Underline(t *testing.T) {
	qp := write.RectToQuadPoints(72, 710, 300, 722)
	pdf := buildAnnotationPDF(t,
		write.NewUnderlineAnnotation(72, 710, 300, 722, qp),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "underline" {
		t.Errorf("expected type underline, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

func TestAnnotation_Strikeout(t *testing.T) {
	qp := write.RectToQuadPoints(72, 710, 300, 722)
	pdf := buildAnnotationPDF(t,
		write.NewStrikeoutAnnotation(72, 710, 300, 722, qp),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "strikeout" {
		t.Errorf("expected type strikeout, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

func TestAnnotation_FreeText(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewFreeTextAnnotation(72, 400, 300, 450, "Text box content", "/Helvetica 12 Tf 0 g"),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "freetext" {
		t.Errorf("expected type freetext, got %s", a.Type)
	}
	if a.Contents != "Text box content" {
		t.Errorf("expected contents 'Text box content', got %q", a.Contents)
	}
}

func TestAnnotation_Ink(t *testing.T) {
	strokes := [][]float64{
		{100, 100, 150, 200, 200, 150},
		{210, 100, 250, 180},
	}
	pdf := buildAnnotationPDF(t,
		write.NewInkAnnotation(100, 100, 250, 200, strokes).
			WithColor(0, 0, 0).
			WithBorderWidth(1.5),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "ink" {
		t.Errorf("expected type ink, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

func TestAnnotation_Multiple(t *testing.T) {
	qp := write.RectToQuadPoints(72, 710, 200, 722)
	pdf := buildAnnotationPDF(t,
		write.NewLinkAnnotation(72, 700, 300, 720, "https://example.com"),
		write.NewHighlightAnnotation(72, 710, 200, 722, qp),
		write.NewTextAnnotation(300, 700, 340, 740, "See this"),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 3 {
		t.Errorf("expected 3 annotations, got %d", len(doc.Pages[0].Annotations))
	}
}

func TestAnnotation_RectToQuadPoints(t *testing.T) {
	qp := write.RectToQuadPoints(10, 20, 100, 50)
	// Expect: upper-left, upper-right, lower-left, lower-right
	// ul: (10, 50), ur: (100, 50), ll: (10, 20), lr: (100, 20)
	expected := []float64{10, 50, 100, 50, 10, 20, 100, 20}
	if len(qp) != len(expected) {
		t.Fatalf("expected %d points, got %d", len(expected), len(qp))
	}
	for i, v := range expected {
		if qp[i] != v {
			t.Errorf("qp[%d] = %v, want %v", i, qp[i], v)
		}
	}
}
