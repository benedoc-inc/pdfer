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

func TestAnnotation_Squiggly(t *testing.T) {
	qp := write.RectToQuadPoints(72, 710, 300, 722)
	pdf := buildAnnotationPDF(t,
		write.NewSquigglyAnnotation(72, 710, 300, 722, qp),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "squiggly" {
		t.Errorf("expected type squiggly, got %s", a.Type)
	}
	if len(a.QuadPoints) == 0 {
		t.Error("expected QuadPoints to be populated")
	}
}

func TestAnnotation_Line(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewLineAnnotation(72, 700, 300, 720).
			WithColor(1, 0, 0).
			WithBorderWidth(2).
			WithLineEndings("OpenArrow", "None"),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "line" {
		t.Errorf("expected type line, got %s", a.Type)
	}
	if len(a.LineEndpoints) != 2 {
		t.Errorf("expected 2 line endpoints, got %d", len(a.LineEndpoints))
	}
}

func TestAnnotation_Polygon(t *testing.T) {
	verts := []float64{100, 100, 200, 200, 150, 300}
	pdf := buildAnnotationPDF(t,
		write.NewPolygonAnnotation(100, 100, 200, 300, verts).
			WithColor(0, 1, 0).
			WithBorderWidth(1.5),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "polygon" {
		t.Errorf("expected type polygon, got %s", a.Type)
	}
	if len(a.Vertices) != 3 {
		t.Errorf("expected 3 vertices, got %d", len(a.Vertices))
	}
}

func TestAnnotation_Polyline(t *testing.T) {
	verts := []float64{72, 700, 150, 720, 300, 700}
	pdf := buildAnnotationPDF(t,
		write.NewPolylineAnnotation(72, 700, 300, 720, verts).
			WithBorderWidth(1),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "polyline" {
		t.Errorf("expected type polyline, got %s", a.Type)
	}
	if len(a.Vertices) != 3 {
		t.Errorf("expected 3 vertices, got %d", len(a.Vertices))
	}
}

func TestAnnotation_Stamp(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewStampAnnotation(72, 650, 250, 700, "Draft"),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	a := doc.Pages[0].Annotations[0]
	if a.Type != "stamp" {
		t.Errorf("expected type stamp, got %s", a.Type)
	}
	if a.Icon != "Draft" {
		t.Errorf("expected stamp name 'Draft', got %q", a.Icon)
	}
}

func TestAnnotation_Caret(t *testing.T) {
	pdf := buildAnnotationPDF(t,
		write.NewCaretAnnotation(100, 700, 110, 715),
	)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(doc.Pages[0].Annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
	}
	if doc.Pages[0].Annotations[0].Type != "caret" {
		t.Errorf("expected type caret, got %s", doc.Pages[0].Annotations[0].Type)
	}
}

// TestAnnotation_AppearanceStreams verifies that subtypes which were previously
// invisible in Preview/Skim now carry /AP entries in the generated PDF bytes.
func TestAnnotation_AppearanceStreams(t *testing.T) {
	verts := []float64{100, 100, 200, 200, 150, 300}
	strokes := [][]float64{{100, 100, 150, 150, 200, 100}}
	qp := write.RectToQuadPoints(72, 710, 300, 722)

	cases := []struct {
		name   string
		annot  *write.AnnotationBuilder
		hasAP  bool
	}{
		{"Square", write.NewSquareAnnotation(72, 600, 300, 700).WithColor(1, 0, 0).WithBorderWidth(2), true},
		{"Circle", write.NewCircleAnnotation(72, 500, 200, 580).WithColor(0, 0, 1), true},
		{"Line", write.NewLineAnnotation(72, 700, 300, 720).WithBorderWidth(2), true},
		{"Polygon", write.NewPolygonAnnotation(100, 100, 200, 300, verts).WithBorderWidth(1.5), true},
		{"PolyLine", write.NewPolylineAnnotation(72, 700, 300, 720, verts).WithBorderWidth(1), true},
		{"Ink", write.NewInkAnnotation(100, 100, 250, 200, strokes).WithBorderWidth(1.5), true},
		{"Squiggly", write.NewSquigglyAnnotation(72, 710, 300, 722, qp).WithColor(1, 0.5, 0), true},
		{"FreeText", write.NewFreeTextAnnotation(72, 400, 300, 450, "hi", "/Helvetica 9 Tf 0 g").WithColor(1, 1, 0.8).WithBorderWidth(1), true},
		{"Link", write.NewLinkAnnotation(72, 700, 300, 720, "https://example.com"), false},
		{"Text", write.NewTextAnnotation(72, 680, 200, 700, "note"), false},
		{"Stamp", write.NewStampAnnotation(72, 650, 250, 700, "Draft"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pdf := buildAnnotationPDF(t, tc.annot)
			raw := string(pdf)
			hasAP := strings.Contains(raw, "/AP")
			if hasAP != tc.hasAP {
				t.Errorf("hasAP=%v, want %v", hasAP, tc.hasAP)
			}
			// Structural sanity: must still parse.
			doc, err := extract.ExtractContent(pdf, nil, false)
			if err != nil {
				t.Fatalf("extract failed: %v", err)
			}
			if len(doc.Pages[0].Annotations) != 1 {
				t.Fatalf("expected 1 annotation, got %d", len(doc.Pages[0].Annotations))
			}
		})
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
