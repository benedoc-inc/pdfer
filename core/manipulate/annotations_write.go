package manipulate

import (
	"fmt"
	"regexp"
	"strings"
)

// AnnotationType identifies the PDF annotation subtype.
type AnnotationType string

const (
	AnnotText      AnnotationType = "Text"
	AnnotLink      AnnotationType = "Link"
	AnnotFreeText  AnnotationType = "FreeText"
	AnnotHighlight AnnotationType = "Highlight"
	AnnotUnderline AnnotationType = "Underline"
	AnnotStrikeOut AnnotationType = "StrikeOut"
)

// AnnotationConfig describes an annotation to be added to a page.
type AnnotationConfig struct {
	Type     AnnotationType
	Rect     [4]float64 // [llx lly urx ury] in page coordinates
	Contents string     // visible text (Text, FreeText) or tooltip (Link)

	// Link-specific: set URI for external links, or DestPage (1-based) for internal.
	URI      string
	DestPage int

	// Colour (RGB 0–1); zero value = default for type.
	Color [3]float64

	// Opacity 0–1; 0 means use default (fully opaque).
	Opacity float64
}

// AddAnnotation adds a single annotation to the given page (1-based) of pdfBytes.
func AddAnnotation(pdfBytes []byte, pageNumber int, cfg AnnotationConfig, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("add annotation: %w", err)
	}

	pageObjNum, err := m.getPageObjectNumber(pageNumber)
	if err != nil {
		return nil, fmt.Errorf("add annotation: %w", err)
	}

	newAnnotNum := m.maxObjectNumber() + 1

	// Build the annotation dictionary.
	var b strings.Builder
	b.WriteString("<</Type/Annot")
	b.WriteString("/Subtype/")
	b.WriteString(string(cfg.Type))
	b.WriteString(fmt.Sprintf("/Rect[%g %g %g %g]",
		cfg.Rect[0], cfg.Rect[1], cfg.Rect[2], cfg.Rect[3]))

	if cfg.Contents != "" {
		b.WriteString("/Contents")
		b.WriteString(escapePDFLiteralString(cfg.Contents))
	}

	if cfg.Type == AnnotLink {
		if cfg.URI != "" {
			b.WriteString("/A<</S/URI/URI")
			b.WriteString(escapePDFLiteralString(cfg.URI))
			b.WriteString(">>")
		} else if cfg.DestPage > 0 {
			allPageObjNums, err := m.getAllPageObjectNumbers()
			if err != nil {
				return nil, fmt.Errorf("add annotation: get page objects: %w", err)
			}
			if cfg.DestPage < 1 || cfg.DestPage > len(allPageObjNums) {
				return nil, fmt.Errorf("add annotation: dest page %d out of range (1-%d)", cfg.DestPage, len(allPageObjNums))
			}
			destPageObjNum := allPageObjNums[cfg.DestPage-1]
			b.WriteString(fmt.Sprintf("/Dest[%d 0 R /XYZ null null null]", destPageObjNum))
		}
	}

	zeroColor := [3]float64{}
	if cfg.Color != zeroColor {
		b.WriteString(fmt.Sprintf("/C[%g %g %g]", cfg.Color[0], cfg.Color[1], cfg.Color[2]))
	}

	if cfg.Opacity > 0 {
		b.WriteString(fmt.Sprintf("/CA %g", cfg.Opacity))
	}

	b.WriteString(">>")
	annotDict := b.String()

	m.objects[newAnnotNum] = []byte(annotDict)

	// Update the page's /Annots array.
	pageObj, ok := m.objects[pageObjNum]
	if !ok {
		return nil, fmt.Errorf("add annotation: page object %d not found", pageObjNum)
	}
	pageStr := string(pageObj)

	newRef := fmt.Sprintf("%d 0 R", newAnnotNum)

	annotsIdx := strings.Index(pageStr, "/Annots")
	if annotsIdx == -1 {
		// No /Annots key — insert one before closing >>
		pageStr = insertBeforeClosingDict(pageStr, "/Annots ["+newRef+"]")
	} else {
		// Determine whether the value is inline [...] or an indirect ref.
		valueStart := annotsIdx + len("/Annots")
		for valueStart < len(pageStr) && isWhitespace(pageStr[valueStart]) {
			valueStart++
		}

		if valueStart < len(pageStr) && pageStr[valueStart] == '[' {
			// Inline array — append the new ref before the closing ].
			inlineRe := regexp.MustCompile(`(/Annots\s*\[[^\]]*\])`)
			pageStr = inlineRe.ReplaceAllStringFunc(pageStr, func(match string) string {
				// Insert newRef before the final ]
				closeIdx := strings.LastIndex(match, "]")
				existing := strings.TrimSpace(match[:closeIdx])
				if existing == "/Annots [" || existing == "/Annots[" {
					return existing + newRef + "]"
				}
				return existing + " " + newRef + "]"
			})
		} else {
			// Indirect ref — parse the object number and append into that object.
			refStr := ""
			i := valueStart
			for i < len(pageStr) && !isWhitespace(pageStr[i]) && pageStr[i] != '/' && pageStr[i] != '>' {
				refStr += string(pageStr[i])
				i++
			}
			// refStr should be the first digit of "N 0 R"
			// collect until R
			for i < len(pageStr) && pageStr[i] != 'R' && i-valueStart < 20 {
				refStr += string(pageStr[i])
				i++
			}
			if i < len(pageStr) && pageStr[i] == 'R' {
				refStr += "R"
			}
			annotsObjNum, parseErr := parseObjectRef(strings.TrimSpace(refStr))
			if parseErr == nil {
				if annotsObj, exists := m.objects[annotsObjNum]; exists {
					annotsStr := strings.TrimSpace(string(annotsObj))
					// The object is an array like [1 0 R 2 0 R]
					if strings.HasSuffix(annotsStr, "]") {
						annotsStr = annotsStr[:len(annotsStr)-1] + " " + newRef + "]"
					} else {
						annotsStr = annotsStr + " " + newRef
					}
					m.objects[annotsObjNum] = []byte(annotsStr)
				}
			}
		}
	}

	m.objects[pageObjNum] = []byte(pageStr)

	if verbose {
		fmt.Printf("Added %s annotation (object %d) to page %d (object %d)\n",
			cfg.Type, newAnnotNum, pageNumber, pageObjNum)
	}

	return m.Rebuild()
}

// insertBeforeClosingDict inserts extra into dictStr just before the last >>
func insertBeforeClosingDict(dictStr, extra string) string {
	lastIdx := strings.LastIndex(dictStr, ">>")
	if lastIdx == -1 {
		return dictStr + extra
	}
	return dictStr[:lastIdx] + extra + " " + dictStr[lastIdx:]
}

// isWhitespace returns true for PDF whitespace characters.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}
