package write

import (
	"fmt"
	"strings"
)

// AnnotationBuilder builds a PDF annotation object.
// Use a subtype constructor (e.g. NewLinkAnnotation), apply options via
// the With* methods, then attach to a page with PageBuilder.AddAnnotation.
type AnnotationBuilder struct {
	subtype    string
	rect       [4]float64 // x1, y1, x2, y2 in user space
	contents   string
	title      string     // author / creator shown in viewer UI
	color      [3]float64 // RGB 0–1
	hasColor   bool
	opacity    float64     // 0 means unset (viewer default = fully opaque)
	borderW    float64     // ≥0: border width; <0: invisible border
	open       bool        // Text: initial popup state
	icon       string      // Text: icon name (/Note, /Comment, /Help, …)
	quadPoints []float64   // Markup: 8 coords per quad [ul_x ul_y ur_x ur_y ll_x ll_y lr_x lr_y …]
	inkLists   [][]float64 // Ink: list of strokes, each a flat [x y …] slice
	da         string      // FreeText: default appearance string (/FontName sz Tf r g b rg)
	uri        string      // Link: target URI
	dest       string      // Link: named destination (alternative to uri)
}

// --- Constructors ---

// NewLinkAnnotation creates a link annotation pointing to a URI.
// The border is invisible by default; use WithBorderWidth to add a visible one.
func NewLinkAnnotation(x1, y1, x2, y2 float64, uri string) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype: "Link",
		rect:    [4]float64{x1, y1, x2, y2},
		uri:     uri,
		borderW: -1,
	}
}

// NewTextAnnotation creates a sticky-note annotation.
func NewTextAnnotation(x1, y1, x2, y2 float64, contents string) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:  "Text",
		rect:     [4]float64{x1, y1, x2, y2},
		contents: contents,
		icon:     "Note",
		borderW:  -1,
	}
}

// NewHighlightAnnotation creates a highlight markup annotation.
// quadPoints is a flat slice of 8 coords per highlighted quad (use RectToQuadPoints
// for a simple rectangular highlight). Default color is yellow.
func NewHighlightAnnotation(x1, y1, x2, y2 float64, quadPoints []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:    "Highlight",
		rect:       [4]float64{x1, y1, x2, y2},
		quadPoints: quadPoints,
		color:      [3]float64{1, 1, 0},
		hasColor:   true,
		borderW:    -1,
	}
}

// NewUnderlineAnnotation creates an underline markup annotation.
func NewUnderlineAnnotation(x1, y1, x2, y2 float64, quadPoints []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:    "Underline",
		rect:       [4]float64{x1, y1, x2, y2},
		quadPoints: quadPoints,
		borderW:    -1,
	}
}

// NewStrikeoutAnnotation creates a strikeout markup annotation.
func NewStrikeoutAnnotation(x1, y1, x2, y2 float64, quadPoints []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:    "StrikeOut",
		rect:       [4]float64{x1, y1, x2, y2},
		quadPoints: quadPoints,
		borderW:    -1,
	}
}

// NewSquareAnnotation creates a rectangle annotation.
func NewSquareAnnotation(x1, y1, x2, y2 float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype: "Square",
		rect:    [4]float64{x1, y1, x2, y2},
		borderW: 1,
	}
}

// NewCircleAnnotation creates an ellipse annotation.
func NewCircleAnnotation(x1, y1, x2, y2 float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype: "Circle",
		rect:    [4]float64{x1, y1, x2, y2},
		borderW: 1,
	}
}

// NewFreeTextAnnotation creates a free-text (text-box) annotation.
// da is the PDF default-appearance string, e.g. "/Helvetica 12 Tf 0 g".
func NewFreeTextAnnotation(x1, y1, x2, y2 float64, contents, da string) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:  "FreeText",
		rect:     [4]float64{x1, y1, x2, y2},
		contents: contents,
		da:       da,
		borderW:  1,
	}
}

// NewInkAnnotation creates a freehand-drawing annotation.
// inkLists is a slice of strokes; each stroke is a flat [x y x y …] coordinate slice.
func NewInkAnnotation(x1, y1, x2, y2 float64, inkLists [][]float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:  "Ink",
		rect:     [4]float64{x1, y1, x2, y2},
		inkLists: inkLists,
		borderW:  1,
	}
}

// RectToQuadPoints converts a bounding rectangle to a QuadPoints array in the
// PDF order expected by markup annotations: upper-left, upper-right, lower-left, lower-right.
func RectToQuadPoints(x1, y1, x2, y2 float64) []float64 {
	return []float64{x1, y2, x2, y2, x1, y1, x2, y1}
}

// --- Fluent option methods ---

// WithContents sets the annotation's text content (shown in comment/tooltip).
func (a *AnnotationBuilder) WithContents(s string) *AnnotationBuilder {
	a.contents = s
	return a
}

// WithTitle sets the annotation author/creator shown in the viewer UI.
func (a *AnnotationBuilder) WithTitle(s string) *AnnotationBuilder {
	a.title = s
	return a
}

// WithColor sets the annotation color (RGB, components 0–1).
func (a *AnnotationBuilder) WithColor(r, g, b float64) *AnnotationBuilder {
	a.color = [3]float64{r, g, b}
	a.hasColor = true
	return a
}

// WithOpacity sets the annotation opacity (0=transparent … 1=opaque).
func (a *AnnotationBuilder) WithOpacity(v float64) *AnnotationBuilder {
	a.opacity = v
	return a
}

// WithBorderWidth sets the annotation border line width in points.
func (a *AnnotationBuilder) WithBorderWidth(w float64) *AnnotationBuilder {
	a.borderW = w
	return a
}

// WithNoBorder removes the annotation border.
func (a *AnnotationBuilder) WithNoBorder() *AnnotationBuilder {
	a.borderW = -1
	return a
}

// WithOpen controls whether a Text annotation's popup is initially open.
func (a *AnnotationBuilder) WithOpen(open bool) *AnnotationBuilder {
	a.open = open
	return a
}

// WithIcon sets the icon for a Text annotation.
// Standard values: Note (default), Comment, Key, Help, NewParagraph, Paragraph, Insert.
func (a *AnnotationBuilder) WithIcon(icon string) *AnnotationBuilder {
	a.icon = icon
	return a
}

// --- Internal ---

// build serializes the annotation into w and returns the allocated object number.
func (a *AnnotationBuilder) build(w *PDFWriter) int {
	var b strings.Builder

	b.WriteString("<</Type/Annot/Subtype/")
	b.WriteString(a.subtype)
	b.WriteString(fmt.Sprintf("/Rect[%.4f %.4f %.4f %.4f]",
		a.rect[0], a.rect[1], a.rect[2], a.rect[3]))

	if a.contents != "" {
		b.WriteString(fmt.Sprintf("/Contents(%s)", escapePDFString(a.contents)))
	}
	if a.title != "" {
		b.WriteString(fmt.Sprintf("/T(%s)", escapePDFString(a.title)))
	}
	if a.hasColor {
		b.WriteString(fmt.Sprintf("/C[%.4f %.4f %.4f]", a.color[0], a.color[1], a.color[2]))
	}
	if a.opacity > 0 {
		b.WriteString(fmt.Sprintf("/CA %.4f", a.opacity))
	}

	// Border: /Border[h-radius v-radius line-width] for simple borders;
	// /BS for border style dict (needed for non-zero width with style).
	switch {
	case a.borderW < 0:
		b.WriteString("/Border[0 0 0]")
	case a.borderW > 0:
		b.WriteString(fmt.Sprintf("/BS<</W %.4f/S/S>>", a.borderW))
	}

	// Print flag: bit 3 (value 4) — annotation is printed with the page.
	b.WriteString("/F 4")

	// Subtype-specific keys.
	switch a.subtype {
	case "Link":
		if a.uri != "" {
			b.WriteString(fmt.Sprintf("/A<</Type/Action/S/URI/URI(%s)>>",
				escapePDFString(a.uri)))
		} else if a.dest != "" {
			b.WriteString(fmt.Sprintf("/Dest(%s)", escapePDFString(a.dest)))
		}
		b.WriteString("/H/I") // highlight mode: invert on click

	case "Text":
		if a.open {
			b.WriteString("/Open true")
		} else {
			b.WriteString("/Open false")
		}
		if a.icon != "" {
			b.WriteString("/Name/")
			b.WriteString(a.icon)
		}

	case "Highlight", "Underline", "StrikeOut", "Squiggly":
		if len(a.quadPoints) > 0 {
			b.WriteString("/QuadPoints[")
			for i, v := range a.quadPoints {
				if i > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(fmt.Sprintf("%.4f", v))
			}
			b.WriteByte(']')
		}

	case "FreeText":
		if a.da != "" {
			b.WriteString(fmt.Sprintf("/DA(%s)", escapePDFString(a.da)))
		}
		b.WriteString("/Q 0") // left-justified

	case "Ink":
		if len(a.inkLists) > 0 {
			b.WriteString("/InkList[")
			for _, stroke := range a.inkLists {
				b.WriteByte('[')
				for i, v := range stroke {
					if i > 0 {
						b.WriteByte(' ')
					}
					b.WriteString(fmt.Sprintf("%.4f", v))
				}
				b.WriteByte(']')
			}
			b.WriteByte(']')
		}
	}

	b.WriteString(">>")
	return w.AddObject([]byte(b.String()))
}
