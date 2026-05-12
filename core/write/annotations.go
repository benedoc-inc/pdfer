package write

import (
	"fmt"
	"strings"
)

// AnnotationBuilder builds a PDF annotation object.
// Use a subtype constructor (e.g. NewLinkAnnotation), apply options via
// the With* methods, then attach to a page with PageBuilder.AddAnnotation.
type AnnotationBuilder struct {
	subtype        string
	rect           [4]float64 // x1, y1, x2, y2 in user space
	contents       string
	title          string     // author / creator shown in viewer UI
	color          [3]float64 // RGB 0–1
	hasColor       bool
	opacity        float64     // 0 means unset (viewer default = fully opaque)
	borderW        float64     // ≥0: border width; <0: invisible border
	open           bool        // Text: initial popup state
	icon           string      // Text/Stamp: icon or stamp name; Caret: /Sy symbol
	quadPoints     []float64   // Markup: 8 coords per quad [ul_x ul_y ur_x ur_y ll_x ll_y lr_x lr_y …]
	inkLists       [][]float64 // Ink: list of strokes, each a flat [x y …] slice
	da             string      // FreeText: default appearance string (/FontName sz Tf r g b rg)
	uri            string      // Link: target URI
	dest           string      // Link: named destination (alternative to uri)
	lineCoords     [4]float64  // Line: /L [x1 y1 x2 y2] endpoint coords
	vertices       []float64   // Polygon/PolyLine: /Vertices flat [x y …] slice
	lineEndings    [2]string   // Line/Polygon/PolyLine: /LE [start end] style names
	hasLineEndings bool
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

// NewSquigglyAnnotation creates a squiggly-underline markup annotation.
func NewSquigglyAnnotation(x1, y1, x2, y2 float64, quadPoints []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:    "Squiggly",
		rect:       [4]float64{x1, y1, x2, y2},
		quadPoints: quadPoints,
		borderW:    -1,
	}
}

// NewLineAnnotation creates a straight-line annotation.
// x1,y1 and x2,y2 are the line endpoints; the bounding rect is derived from them.
// Use WithLineEndings to set decorative end caps (e.g. "OpenArrow", "None").
func NewLineAnnotation(x1, y1, x2, y2 float64) *AnnotationBuilder {
	minX, maxX := x1, x2
	if x1 > x2 {
		minX, maxX = x2, x1
	}
	minY, maxY := y1, y2
	if y1 > y2 {
		minY, maxY = y2, y1
	}
	return &AnnotationBuilder{
		subtype:    "Line",
		rect:       [4]float64{minX, minY, maxX, maxY},
		lineCoords: [4]float64{x1, y1, x2, y2},
		borderW:    1,
	}
}

// NewPolygonAnnotation creates a closed polygon annotation.
// vertices is a flat [x y x y …] slice of vertex coordinates.
func NewPolygonAnnotation(x1, y1, x2, y2 float64, vertices []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:  "Polygon",
		rect:     [4]float64{x1, y1, x2, y2},
		vertices: vertices,
		borderW:  1,
	}
}

// NewPolylineAnnotation creates an open polyline annotation.
// vertices is a flat [x y x y …] slice of vertex coordinates.
func NewPolylineAnnotation(x1, y1, x2, y2 float64, vertices []float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype:  "PolyLine",
		rect:     [4]float64{x1, y1, x2, y2},
		vertices: vertices,
		borderW:  1,
	}
}

// NewStampAnnotation creates a rubber-stamp annotation.
// stampName is the standard PDF stamp name: Draft, NotApproved, Approved,
// AsIs, Confidential, Departmental, Experimental, Expired, Final,
// ForComment, ForPublicRelease, NotForPublicRelease, Sold, TopSecret.
func NewStampAnnotation(x1, y1, x2, y2 float64, stampName string) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype: "Stamp",
		rect:    [4]float64{x1, y1, x2, y2},
		icon:    stampName,
		borderW: -1,
	}
}

// NewCaretAnnotation creates a caret (text-insertion-point) annotation.
func NewCaretAnnotation(x1, y1, x2, y2 float64) *AnnotationBuilder {
	return &AnnotationBuilder{
		subtype: "Caret",
		rect:    [4]float64{x1, y1, x2, y2},
		borderW: -1,
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

// WithLineEndings sets decorative end caps for Line, Polygon, and PolyLine annotations.
// Standard style names: None, Square, Circle, Diamond, OpenArrow, ClosedArrow,
// Butt, ROpenArrow, RClosedArrow, Slash.
func (a *AnnotationBuilder) WithLineEndings(start, end string) *AnnotationBuilder {
	a.lineEndings = [2]string{start, end}
	a.hasLineEndings = true
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
					fmt.Fprintf(&b, "%.4f", v)
				}
				b.WriteByte(']')
			}
			b.WriteByte(']')
		}

	case "Line":
		fmt.Fprintf(&b, "/L[%.4f %.4f %.4f %.4f]",
			a.lineCoords[0], a.lineCoords[1], a.lineCoords[2], a.lineCoords[3])
		if a.hasLineEndings {
			fmt.Fprintf(&b, "/LE[/%s /%s]", a.lineEndings[0], a.lineEndings[1])
		}

	case "Polygon", "PolyLine":
		if len(a.vertices) > 0 {
			b.WriteString("/Vertices[")
			for i, v := range a.vertices {
				if i > 0 {
					b.WriteByte(' ')
				}
				fmt.Fprintf(&b, "%.4f", v)
			}
			b.WriteByte(']')
		}
		if a.hasLineEndings {
			fmt.Fprintf(&b, "/LE[/%s /%s]", a.lineEndings[0], a.lineEndings[1])
		}

	case "Stamp":
		if a.icon != "" {
			b.WriteString("/Name/")
			b.WriteString(a.icon)
		}

	case "Caret":
		if a.icon != "" {
			// /Sy: P = paragraph symbol, None = plain caret
			b.WriteString("/Sy/")
			b.WriteString(a.icon)
		}
	}

	// Appearance stream — generated before closing the dict so we can embed the ref.
	apObjNum := a.buildAppearance(w)
	if apObjNum > 0 {
		fmt.Fprintf(&b, "/AP<</N %d 0 R>>", apObjNum)
	}

	b.WriteString(">>")
	return w.AddObject([]byte(b.String()))
}
