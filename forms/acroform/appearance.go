// Package acroform provides appearance stream creation for form fields
package acroform

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/benedoc-inc/pdfer/core/write"
)

// AppearanceBuilder helps create appearance streams for form fields
type AppearanceBuilder struct {
	writer *write.PDFWriter
}

// NewAppearanceBuilder creates a new appearance builder
func NewAppearanceBuilder(w *write.PDFWriter) *AppearanceBuilder {
	return &AppearanceBuilder{
		writer: w,
	}
}

// CreateCheckboxAppearance creates an appearance stream for a checkbox
func (ab *AppearanceBuilder) CreateCheckboxAppearance(checked bool, width, height float64) (int, error) {
	var content strings.Builder

	// Set up graphics state
	content.WriteString("q\n") // Save state

	// Set color (black)
	content.WriteString("0 0 0 rg\n") // Set fill color to black

	if checked {
		// Draw checkmark
		// Simple checkmark using lines
		lineWidth := width / 20
		content.WriteString(fmt.Sprintf("%.2f w\n", lineWidth)) // Set line width

		// Draw checkmark path
		checkX1 := width * 0.2
		checkY1 := height * 0.5
		checkX2 := width * 0.4
		checkY2 := height * 0.3
		checkX3 := width * 0.8
		checkY3 := height * 0.7

		content.WriteString(fmt.Sprintf("%.2f %.2f m\n", checkX1, checkY1)) // Move to start
		content.WriteString(fmt.Sprintf("%.2f %.2f l\n", checkX2, checkY2)) // Line to middle
		content.WriteString(fmt.Sprintf("%.2f %.2f l\n", checkX3, checkY3)) // Line to end
		content.WriteString("S\n")                                          // Stroke
	} else {
		// Draw empty box (border only)
		lineWidth := width / 20
		content.WriteString(fmt.Sprintf("%.2f w\n", lineWidth))
		content.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height)) // Rectangle
		content.WriteString("S\n")                                            // Stroke
	}

	content.WriteString("Q\n") // Restore state

	// Create appearance stream
	appearanceDict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0, 0, width, height},
		"/Matrix":  []interface{}{1, 0, 0, 1, 0, 0},
	}

	appearanceNum := ab.writer.AddStreamObject(appearanceDict, []byte(content.String()), true)
	return appearanceNum, nil
}

// CreateTextAppearance creates an appearance stream for a text field
func (ab *AppearanceBuilder) CreateTextAppearance(text string, width, height, fontSize float64, fontName string) (int, error) {
	var content strings.Builder

	content.WriteString("q\n") // Save state

	// Set up text
	content.WriteString("BT\n") // Begin text
	content.WriteString(fmt.Sprintf("/%s %.2f Tf\n", fontName, fontSize))
	content.WriteString("0 0 0 rg\n") // Black text

	// Position text (bottom-left of field)
	textY := height * 0.2 // Leave some margin
	content.WriteString(fmt.Sprintf("0 %.2f Td\n", textY))

	// Escape text for PDF
	escapedText := escapeAppearanceText(text)
	content.WriteString(fmt.Sprintf("(%s) Tj\n", escapedText))

	content.WriteString("ET\n") // End text
	content.WriteString("Q\n")  // Restore state

	// Create appearance stream
	appearanceDict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0, 0, width, height},
		"/Matrix":  []interface{}{1, 0, 0, 1, 0, 0},
		"/Resources": write.Dictionary{
			"/Font": write.Dictionary{
				fontName: fmt.Sprintf("%d 0 R", 0), // Font reference (would need actual font)
			},
		},
	}

	appearanceNum := ab.writer.AddStreamObject(appearanceDict, []byte(content.String()), true)
	return appearanceNum, nil
}

// CreateButtonAppearance creates an appearance stream for a button
func (ab *AppearanceBuilder) CreateButtonAppearance(label string, width, height, fontSize float64) (int, error) {
	var content strings.Builder

	content.WriteString("q\n") // Save state

	// Draw button background (light gray)
	content.WriteString("0.9 0.9 0.9 rg\n") // Light gray fill
	content.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height))
	content.WriteString("f\n") // Fill

	// Draw border
	content.WriteString("0 0 0 RG\n") // Black stroke
	content.WriteString("1 w\n")      // 1 point line width
	content.WriteString(fmt.Sprintf("0 0 %.2f %.2f re\n", width, height))
	content.WriteString("S\n") // Stroke

	// Draw text (centered)
	content.WriteString("BT\n") // Begin text
	content.WriteString("/Helvetica 12 Tf\n")
	content.WriteString("0 0 0 rg\n") // Black text

	// Center text
	textX := (width - float64(len(label))*6) / 2 // Rough centering
	textY := (height - 12) / 2
	content.WriteString(fmt.Sprintf("%.2f %.2f Td\n", textX, textY))
	content.WriteString(fmt.Sprintf("(%s) Tj\n", escapeAppearanceText(label)))
	content.WriteString("ET\n") // End text

	content.WriteString("Q\n") // Restore state

	// Create appearance stream
	appearanceDict := write.Dictionary{
		"/Type":    "/XObject",
		"/Subtype": "/Form",
		"/BBox":    []interface{}{0, 0, width, height},
		"/Matrix":  []interface{}{1, 0, 0, 1, 0, 0},
	}

	appearanceNum := ab.writer.AddStreamObject(appearanceDict, []byte(content.String()), true)
	return appearanceNum, nil
}

// escapeAppearanceText escapes text for appearance streams
func escapeAppearanceText(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '(':
			result.WriteString("\\(")
		case ')':
			result.WriteString("\\)")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			if r > 127 {
				result.WriteString(fmt.Sprintf("\\%03o", r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

// AddAppearanceToField adds an appearance stream to a field object
func AddAppearanceToField(fieldObjNum, appearanceNum int, w *write.PDFWriter) error {
	// Get existing field object
	fieldData, err := w.GetObject(fieldObjNum)
	if err != nil {
		return fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)

	// Add or update /AP dictionary
	apEntry := fmt.Sprintf("/AP << /N %d 0 R >>", appearanceNum)

	// Check if /AP already exists
	if strings.Contains(fieldStr, "/AP") {
		// Replace existing /AP
		apPattern := regexp.MustCompile(`/AP\s*<<[^>]*>>`)
		fieldStr = apPattern.ReplaceAllString(fieldStr, apEntry)
	} else {
		// Add /AP before closing >>
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("field dictionary not found")
		}
		fieldStr = fieldStr[:dictEnd] + apEntry + " " + fieldStr[dictEnd:]
	}

	// Update object
	w.SetObject(fieldObjNum, []byte(fieldStr))
	return nil
}
