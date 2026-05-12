package write

import (
	"fmt"
	"strings"
)

// buildAppearance generates a Normal (/N) appearance Form XObject for the
// annotation and returns its object number.  Returns 0 for subtypes that
// either don't need an explicit /AP (e.g. Link, Text) or whose appearance
// cannot be generated without external information (e.g. Stamp).
//
// The Form XObject uses BBox [0 0 width height] in annotation-local coordinates
// (origin at the annotation's lower-left corner).  PDF viewers map this to the
// annotation's /Rect when rendering.
func (a *AnnotationBuilder) buildAppearance(w *PDFWriter) int {
	width := a.rect[2] - a.rect[0]
	height := a.rect[3] - a.rect[1]
	if width <= 0 || height <= 0 {
		return 0
	}

	var cs strings.Builder
	rx, ry := a.rect[0], a.rect[1]

	// effective line width
	lw := a.borderW
	if lw <= 0 {
		lw = 1
	}

	switch a.subtype {

	case "Square":
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		inset := lw / 2
		fmt.Fprintf(&cs, "%.4f %.4f %.4f %.4f re\nS\n",
			inset, inset, width-lw, height-lw)

	case "Circle":
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		inset := lw / 2
		apEllipse(&cs, inset, inset, width-lw, height-lw)
		cs.WriteString("S\n")

	case "Line":
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		lx1 := a.lineCoords[0] - rx
		ly1 := a.lineCoords[1] - ry
		lx2 := a.lineCoords[2] - rx
		ly2 := a.lineCoords[3] - ry
		fmt.Fprintf(&cs, "%.4f %.4f m\n%.4f %.4f l\nS\n", lx1, ly1, lx2, ly2)

	case "Polygon":
		if len(a.vertices) < 4 {
			return 0
		}
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		for i := 0; i+1 < len(a.vertices); i += 2 {
			vx := a.vertices[i] - rx
			vy := a.vertices[i+1] - ry
			if i == 0 {
				fmt.Fprintf(&cs, "%.4f %.4f m\n", vx, vy)
			} else {
				fmt.Fprintf(&cs, "%.4f %.4f l\n", vx, vy)
			}
		}
		cs.WriteString("h\nS\n")

	case "PolyLine":
		if len(a.vertices) < 4 {
			return 0
		}
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		for i := 0; i+1 < len(a.vertices); i += 2 {
			vx := a.vertices[i] - rx
			vy := a.vertices[i+1] - ry
			if i == 0 {
				fmt.Fprintf(&cs, "%.4f %.4f m\n", vx, vy)
			} else {
				fmt.Fprintf(&cs, "%.4f %.4f l\n", vx, vy)
			}
		}
		cs.WriteString("S\n")

	case "Ink":
		if len(a.inkLists) == 0 {
			return 0
		}
		apSetStrokeColor(&cs, a)
		fmt.Fprintf(&cs, "%.4f w\n", lw)
		for _, stroke := range a.inkLists {
			if len(stroke) < 4 {
				continue
			}
			for i := 0; i+1 < len(stroke); i += 2 {
				px := stroke[i] - rx
				py := stroke[i+1] - ry
				if i == 0 {
					fmt.Fprintf(&cs, "%.4f %.4f m\n", px, py)
				} else {
					fmt.Fprintf(&cs, "%.4f %.4f l\n", px, py)
				}
			}
			cs.WriteString("S\n")
		}

	case "Squiggly":
		// Draw a wavy underline along the bottom edge of each quad.
		// QuadPoints order per quad: ul_x ul_y ur_x ur_y ll_x ll_y lr_x lr_y
		if len(a.quadPoints) < 8 {
			return 0
		}
		apSetStrokeColor(&cs, a)
		cs.WriteString("0.7500 w\n")
		for i := 0; i+7 < len(a.quadPoints); i += 8 {
			llx := a.quadPoints[i+4] - rx
			lly := a.quadPoints[i+5] - ry
			lrx := a.quadPoints[i+6] - rx
			apSquigglyWave(&cs, llx, lrx, lly)
		}
		cs.WriteString("S\n")

	case "FreeText":
		// Render background fill and border; text is left to the viewer via /DA+/Contents.
		if a.hasColor {
			fmt.Fprintf(&cs, "%.4f %.4f %.4f rg\n", a.color[0], a.color[1], a.color[2])
			fmt.Fprintf(&cs, "0 0 %.4f %.4f re\nf\n", width, height)
		}
		if a.borderW > 0 {
			cs.WriteString("0 0 0 RG\n")
			fmt.Fprintf(&cs, "%.4f w\n", a.borderW)
			inset := a.borderW / 2
			fmt.Fprintf(&cs, "%.4f %.4f %.4f %.4f re\nS\n",
				inset, inset, width-a.borderW, height-a.borderW)
		}

	default:
		return 0
	}

	if cs.Len() == 0 {
		return 0
	}

	stream := []byte(cs.String())
	dict := Dictionary{
		"Type":      "/XObject",
		"Subtype":   "/Form",
		"BBox":      []interface{}{float64(0), float64(0), width, height},
		"Resources": Dictionary{},
	}
	return w.AddStreamObject(dict, stream, false)
}

// apSetStrokeColor writes RG/rg operators for the annotation color.
func apSetStrokeColor(cs *strings.Builder, a *AnnotationBuilder) {
	if a.hasColor {
		fmt.Fprintf(cs, "%.4f %.4f %.4f RG\n", a.color[0], a.color[1], a.color[2])
	} else {
		cs.WriteString("0 0 0 RG\n")
	}
}

// apEllipse emits four cubic bezier arcs that approximate an ellipse inscribed
// in [x, y, x+w, y+h] (local coordinates, with corner at origin).
// k ≈ 0.5523 is the standard bezier circle approximation constant.
func apEllipse(cs *strings.Builder, x, y, w, h float64) {
	const k = 0.5523
	cx := x + w/2
	cy := y + h/2
	rx := w / 2
	ry := h / 2
	fmt.Fprintf(cs, "%.4f %.4f m\n", cx, cy+ry)
	fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
		cx+k*rx, cy+ry, cx+rx, cy+k*ry, cx+rx, cy)
	fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
		cx+rx, cy-k*ry, cx+k*rx, cy-ry, cx, cy-ry)
	fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
		cx-k*rx, cy-ry, cx-rx, cy-k*ry, cx-rx, cy)
	fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
		cx-rx, cy+k*ry, cx-k*rx, cy+ry, cx, cy+ry)
	cs.WriteString("h\n")
}

// apSquigglyWave emits a wavy path (not stroked) from (xStart, y) to (xEnd, y)
// using pairs of cubic bezier arcs — one arch up, one arch down per cycle.
// The path must be followed by an S operator to actually stroke it.
func apSquigglyWave(cs *strings.Builder, xStart, xEnd, y float64) {
	if xEnd <= xStart {
		return
	}
	const period = 4.0 // full cycle width in points
	const amp = 1.5    // peak amplitude in points
	// control-point offset to make bezier peak ≈ amp
	const cp = amp * 4 / 3

	x := xStart
	fmt.Fprintf(cs, "%.4f %.4f m\n", x, y)

	for x < xEnd {
		half := period / 2
		// Arch up
		if x+half > xEnd {
			half = xEnd - x
		}
		fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
			x+half/3, y+cp, x+2*half/3, y+cp, x+half, y)
		x += half
		if x >= xEnd {
			break
		}
		// Arch down
		half = period / 2
		if x+half > xEnd {
			half = xEnd - x
		}
		fmt.Fprintf(cs, "%.4f %.4f %.4f %.4f %.4f %.4f c\n",
			x+half/3, y-cp, x+2*half/3, y-cp, x+half, y)
		x += half
	}
}
