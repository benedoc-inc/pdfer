package extract

import (
	"fmt"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// extractAnnotations extracts annotations from a page's /Annots array
func extractAnnotations(annotsStr string, pdf *parse.PDF, pageNum int, verbose bool) []types.Annotation {
	var annotations []types.Annotation

	// Parse annotations array (e.g., "[5 0 R 6 0 R]")
	annotRefs := parseObjectRefArray(annotsStr)
	if len(annotRefs) == 0 {
		// Try as single reference
		if annotRefs = []string{annotsStr}; len(annotsStr) > 0 {
			// Already set
		} else {
			return annotations
		}
	}

	for _, annotRef := range annotRefs {
		annotObjNum, err := parseObjectRef(annotRef)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to parse annotation reference %s: %v\n", annotRef, err)
			}
			continue
		}

		annot := extractAnnotation(annotObjNum, pdf, pageNum, verbose)
		if annot != nil {
			annotations = append(annotations, *annot)
		}
	}

	return annotations
}

// extractAnnotation extracts a single annotation object
func extractAnnotation(annotObjNum int, pdf *parse.PDF, pageNum int, verbose bool) *types.Annotation {
	annotObj, err := pdf.GetObject(annotObjNum)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to get annotation object %d: %v\n", annotObjNum, err)
		}
		return nil
	}

	annotStr := string(annotObj)
	annotation := &types.Annotation{
		ID:         fmt.Sprintf("%d", annotObjNum),
		PageNumber: pageNum,
		Properties: make(map[string]interface{}),
	}

	// Extract Subtype (annotation type)
	subtype := extractDictValue(annotStr, "/Subtype")
	if subtype == "" {
		// Not a valid annotation
		return nil
	}

	// Map PDF subtype to our AnnotationType
	switch subtype {
	case "/Link":
		annotation.Type = types.AnnotationTypeLink
	case "/Text":
		annotation.Type = types.AnnotationTypeText
	case "/Highlight":
		annotation.Type = types.AnnotationTypeHighlight
	case "/Underline":
		annotation.Type = types.AnnotationTypeUnderline
	case "/StrikeOut":
		annotation.Type = types.AnnotationTypeStrikeout
	case "/Squiggly":
		annotation.Type = types.AnnotationTypeSquiggly
	case "/Circle":
		annotation.Type = types.AnnotationTypeCircle
	case "/Square":
		annotation.Type = types.AnnotationTypeSquare
	case "/Line":
		annotation.Type = types.AnnotationTypeLine
	case "/Polygon":
		annotation.Type = types.AnnotationTypePolygon
	case "/Polyline":
		annotation.Type = types.AnnotationTypePolyline
	case "/Ink":
		annotation.Type = types.AnnotationTypeInk
	case "/Stamp":
		annotation.Type = types.AnnotationTypeStamp
	case "/Caret":
		annotation.Type = types.AnnotationTypeCaret
	case "/FreeText":
		annotation.Type = types.AnnotationTypeFreeText
	case "/Popup":
		annotation.Type = types.AnnotationTypePopup
	default:
		// Unknown type, but still extract it
		annotation.Type = types.AnnotationType(strings.TrimPrefix(subtype, "/"))
	}

	// Extract Rect (bounding rectangle)
	rect := extractArrayValue(annotStr, "/Rect")
	if len(rect) >= 4 {
		annotation.Rect = &types.Rectangle{
			LowerX: rect[0],
			LowerY: rect[1],
			UpperX: rect[2],
			UpperY: rect[3],
		}
	}

	// Extract Contents (annotation text/contents)
	contents := extractDictValue(annotStr, "/Contents")
	if contents != "" {
		// Unescape PDF string
		annotation.Contents = unescapePDFString(contents)
	}

	// Extract Title
	title := extractDictValue(annotStr, "/T")
	if title != "" {
		annotation.Title = unescapePDFString(title)
	}

	// Extract Subject
	subject := extractDictValue(annotStr, "/Subj")
	if subject != "" {
		annotation.Subject = unescapePDFString(subject)
	}

	// Extract Color
	color := extractArrayValue(annotStr, "/C")
	if len(color) >= 3 {
		annotation.Color = &types.Color{
			Space: types.ColorSpaceRGB,
			R:     color[0],
			G:     color[1],
			B:     color[2],
		}
		if len(color) >= 4 {
			// CMYK
			annotation.Color.Space = types.ColorSpaceCMYK
			annotation.Color.C = color[0]
			annotation.Color.M = color[1]
			annotation.Color.Y = color[2]
			annotation.Color.K = color[3]
		}
	}

	// Extract Border
	borderWidthStr := extractDictValue(annotStr, "/Border")
	if borderWidthStr != "" {
		// Border can be [width] or [width dashArray dashPhase]
		borderArray := extractArrayValue(annotStr, "/Border")
		if len(borderArray) > 0 {
			annotation.Border = &types.Border{
				Width: borderArray[0],
			}
			if len(borderArray) > 2 {
				annotation.Border.Dash = borderArray[1 : len(borderArray)-1]
			}
		}
	}

	// Link-specific: Extract URI or Destination
	if annotation.Type == types.AnnotationTypeLink {
		// Check for URI
		uriRef := extractDictValue(annotStr, "/URI")
		if uriRef != "" {
			// URI might be a string or a reference to a URI object
			if strings.HasPrefix(uriRef, "(") {
				// It's a string
				annotation.URI = unescapePDFString(uriRef[1 : len(uriRef)-1])
			} else {
				// It's a reference - get the URI object
				uriObjNum, err := parseObjectRef(uriRef)
				if err == nil {
					uriObj, err := pdf.GetObject(uriObjNum)
					if err == nil {
						uriStr := string(uriObj)
						uriValue := extractDictValue(uriStr, "/URI")
						if uriValue != "" {
							annotation.URI = unescapePDFString(uriValue[1 : len(uriValue)-1])
						}
					}
				}
			}
		}

		// Check for Destination (named destination or array)
		dest := extractDictValue(annotStr, "/Dest")
		if dest != "" {
			annotation.Destination = dest
		}
	}

	// Text annotation-specific
	if annotation.Type == types.AnnotationTypeText {
		// Extract Open (is annotation open/popup visible)
		openStr := extractDictValue(annotStr, "/Open")
		if openStr == "true" {
			annotation.Open = true
		}

		// Extract Icon
		icon := extractDictValue(annotStr, "/Name")
		if icon != "" {
			annotation.Icon = icon
		}
	}

	// Markup annotations: Extract QuadPoints (for highlight, underline, etc.)
	if annotation.Type == types.AnnotationTypeHighlight ||
		annotation.Type == types.AnnotationTypeUnderline ||
		annotation.Type == types.AnnotationTypeStrikeout ||
		annotation.Type == types.AnnotationTypeSquiggly {
		quadPoints := extractArrayValue(annotStr, "/QuadPoints")
		if len(quadPoints) >= 8 {
			// QuadPoints is array of [x1 y1 x2 y2 x3 y3 x4 y4] for each quad
			// For simplicity, extract first quad
			annotation.QuadPoints = []types.Point{
				{X: quadPoints[0], Y: quadPoints[1]},
				{X: quadPoints[2], Y: quadPoints[3]},
				{X: quadPoints[4], Y: quadPoints[5]},
				{X: quadPoints[6], Y: quadPoints[7]},
			}
		}
	}

	return annotation
}
