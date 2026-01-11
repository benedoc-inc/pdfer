package extract

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// parseContentStream parses a PDF content stream and extracts text, graphics, and images
func parseContentStream(contentStr string, pdf *parse.PDF, pageNum int, verbose bool) ([]types.TextElement, []types.Graphic, []types.ImageRef) {
	var textElements []types.TextElement
	var graphics []types.Graphic
	var imageRefs []types.ImageRef

	// Decompress if needed (content streams are usually FlateDecode compressed)
	// For now, assume contentStr is already decompressed

	// Parse text state
	textState := &textState{
		fontName:    "",
		fontSize:    0,
		x:           0,
		y:           0,
		charSpacing: 0,
		wordSpacing: 0,
		textRise:    0,
		textMatrix:  [6]float64{1, 0, 0, 1, 0, 0},
		inText:      false,
	}

	// Parse graphics state
	graphicsState := &graphicsState{
		lineWidth:   1,
		fillColor:   &types.Color{R: 0, G: 0, B: 0}, // Default black
		strokeColor: &types.Color{R: 0, G: 0, B: 0},
	}

	// Track current transformation matrix (for images)
	// Default is identity matrix [1 0 0 1 0 0]
	currentMatrix := [6]float64{1, 0, 0, 1, 0, 0}
	matrixStack := [][6]float64{} // Stack for q/Q operators

	// Split content stream into tokens
	lines := strings.Split(contentStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse text operators
		if strings.HasPrefix(line, "BT") {
			textState.inText = true
			continue
		}
		if strings.HasPrefix(line, "ET") {
			textState.inText = false
			continue
		}

		// Text positioning
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+Td`).FindStringSubmatch(line); match != nil {
			if tx, err := strconv.ParseFloat(match[1], 64); err == nil {
				if ty, err := strconv.ParseFloat(match[2], 64); err == nil {
					textState.x += tx
					textState.y += ty
				}
			}
			continue
		}

		// Text matrix
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+Tm`).FindStringSubmatch(line); match != nil {
			matrix := [6]float64{}
			for i := 1; i <= 6; i++ {
				if val, err := strconv.ParseFloat(match[i], 64); err == nil {
					matrix[i-1] = val
				}
			}
			textState.textMatrix = matrix
			textState.x = matrix[4]
			textState.y = matrix[5]
			continue
		}

		// Font and size
		if match := regexp.MustCompile(`^([/\w]+)\s+([\d\.\-]+)\s+Tf`).FindStringSubmatch(line); match != nil {
			textState.fontName = match[1]
			if size, err := strconv.ParseFloat(match[2], 64); err == nil {
				textState.fontSize = size
			}
			continue
		}

		// Character spacing
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+Tc`).FindStringSubmatch(line); match != nil {
			if val, err := strconv.ParseFloat(match[1], 64); err == nil {
				textState.charSpacing = val
			}
			continue
		}

		// Word spacing
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+Tw`).FindStringSubmatch(line); match != nil {
			if val, err := strconv.ParseFloat(match[1], 64); err == nil {
				textState.wordSpacing = val
			}
			continue
		}

		// Text rise
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+Ts`).FindStringSubmatch(line); match != nil {
			if val, err := strconv.ParseFloat(match[1], 64); err == nil {
				textState.textRise = val
			}
			continue
		}

		// Show text (Tj operator) - literal string
		if match := regexp.MustCompile(`^\(([^)]*)\)\s+Tj`).FindStringSubmatch(line); match != nil {
			text := unescapePDFString(match[1])
			textElement := createTextElement(text, textState)
			textElements = append(textElements, textElement)
			continue
		}

		// Show text next line (') - literal string
		if match := regexp.MustCompile(`^\(([^)]*)\)\s+'`).FindStringSubmatch(line); match != nil {
			text := unescapePDFString(match[1])
			textElement := createTextElement(text, textState)
			textElements = append(textElements, textElement)
			// Move to next line (simplified - would need leading)
			textState.y -= textState.fontSize * 1.2
			continue
		}

		// Show text array (TJ operator)
		if match := regexp.MustCompile(`^\[([^\]]+)\]\s+TJ`).FindStringSubmatch(line); match != nil {
			text := parseTextArray(match[1])
			textElement := createTextElement(text, textState)
			textElements = append(textElements, textElement)
			continue
		}

		// Graphics operations
		// Rectangle (re operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+re`).FindStringSubmatch(line); match != nil {
			if x, err := strconv.ParseFloat(match[1], 64); err == nil {
				if y, err := strconv.ParseFloat(match[2], 64); err == nil {
					if w, err := strconv.ParseFloat(match[3], 64); err == nil {
						if h, err := strconv.ParseFloat(match[4], 64); err == nil {
							graphic := types.Graphic{
								Type:        types.GraphicTypeRectangle,
								FillColor:   graphicsState.fillColor,
								StrokeColor: graphicsState.strokeColor,
								LineWidth:   graphicsState.lineWidth,
								BoundingBox: &types.Rectangle{
									LowerX: x,
									LowerY: y,
									UpperX: x + w,
									UpperY: y + h,
								},
							}
							graphics = append(graphics, graphic)
						}
					}
				}
			}
			continue
		}

		// Move to (m operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+m`).FindStringSubmatch(line); match != nil {
			// Start of a path - we'll track this for line/path extraction
			// For now, just note it
			continue
		}

		// Line to (l operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+l`).FindStringSubmatch(line); match != nil {
			// Simplified - would need to track path start
			// For now, just note that a line was drawn
			graphic := types.Graphic{
				Type:        types.GraphicTypeLine,
				StrokeColor: graphicsState.strokeColor,
				LineWidth:   graphicsState.lineWidth,
			}
			graphics = append(graphics, graphic)
			continue
		}

		// Stroke (S operator)
		if strings.HasPrefix(line, "S") {
			// Stroke the current path
			continue
		}

		// Fill (f operator)
		if strings.HasPrefix(line, "f") {
			// Fill the current path
			continue
		}

		// Save graphics state (q operator) - push current matrix to stack
		if strings.TrimSpace(line) == "q" {
			matrixStack = append(matrixStack, currentMatrix)
			continue
		}

		// Restore graphics state (Q operator) - pop matrix from stack
		if strings.TrimSpace(line) == "Q" {
			if len(matrixStack) > 0 {
				currentMatrix = matrixStack[len(matrixStack)-1]
				matrixStack = matrixStack[:len(matrixStack)-1]
			}
			continue
		}

		// Set transformation matrix (cm operator)
		// Format: a b c d e f cm
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+cm`).FindStringSubmatch(line); match != nil {
			matrix := [6]float64{}
			for i := 1; i <= 6; i++ {
				if val, err := strconv.ParseFloat(match[i], 64); err == nil {
					matrix[i-1] = val
				}
			}
			// Multiply with current matrix (concatenate transformations)
			// For simplicity, we'll just replace (assuming q/Q handles stacking)
			// In full implementation, should multiply: newMatrix = currentMatrix * matrix
			// For now, replace since DrawImageAt uses q/cm/Do/Q pattern
			currentMatrix = matrix
			continue
		}

		// Set line width (w operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+w`).FindStringSubmatch(line); match != nil {
			if val, err := strconv.ParseFloat(match[1], 64); err == nil {
				graphicsState.lineWidth = val
			}
			continue
		}

		// Set fill color RGB (rg operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+rg`).FindStringSubmatch(line); match != nil {
			if r, err := strconv.ParseFloat(match[1], 64); err == nil {
				if g, err := strconv.ParseFloat(match[2], 64); err == nil {
					if b, err := strconv.ParseFloat(match[3], 64); err == nil {
						graphicsState.fillColor = &types.Color{R: r, G: g, B: b}
					}
				}
			}
			continue
		}

		// Set stroke color RGB (RG operator)
		if match := regexp.MustCompile(`^([\d\.\-]+)\s+([\d\.\-]+)\s+([\d\.\-]+)\s+RG`).FindStringSubmatch(line); match != nil {
			if r, err := strconv.ParseFloat(match[1], 64); err == nil {
				if g, err := strconv.ParseFloat(match[2], 64); err == nil {
					if b, err := strconv.ParseFloat(match[3], 64); err == nil {
						graphicsState.strokeColor = &types.Color{R: r, G: g, B: b}
					}
				}
			}
			continue
		}

		// Draw image (Do operator)
		// Extract position and size from current transformation matrix
		if match := regexp.MustCompile(`^([/\w]+)\s+Do`).FindStringSubmatch(line); match != nil {
			// Transformation matrix [a b c d e f] where:
			// - a, d are scale factors (width, height)
			// - e, f are translation (x, y position)
			// For DrawImageAt pattern: [width 0 0 height x y]
			x := currentMatrix[4]      // e (translation X)
			y := currentMatrix[5]      // f (translation Y)
			width := currentMatrix[0]  // a (scale X)
			height := currentMatrix[3] // d (scale Y)

			// Handle negative scales (flipped images)
			if width < 0 {
				width = -width
			}
			if height < 0 {
				height = -height
			}

			imageRef := types.ImageRef{
				ImageID:   match[1],
				X:         x,
				Y:         y,
				Width:     width,
				Height:    height,
				Transform: currentMatrix,
			}
			imageRefs = append(imageRefs, imageRef)
			continue
		}
	}

	return textElements, graphics, imageRefs
}

// textState tracks the current text rendering state
type textState struct {
	fontName    string
	fontSize    float64
	x           float64
	y           float64
	charSpacing float64
	wordSpacing float64
	textRise    float64
	textMatrix  [6]float64
	inText      bool
}

// graphicsState tracks the current graphics rendering state
type graphicsState struct {
	lineWidth   float64
	fillColor   *types.Color
	strokeColor *types.Color
}

// createTextElement creates a TextElement from text and current text state
func createTextElement(text string, state *textState) types.TextElement {
	// Calculate approximate width (simplified - would need font metrics)
	width := float64(len(text)) * state.fontSize * 0.6

	return types.TextElement{
		Text:        text,
		X:           state.x,
		Y:           state.y,
		Width:       width,
		Height:      state.fontSize,
		FontName:    state.fontName,
		FontSize:    state.fontSize,
		CharSpacing: state.charSpacing,
		WordSpacing: state.wordSpacing,
		TextRise:    state.textRise,
		TextMatrix:  state.textMatrix,
	}
}

// parseTextArray parses a TJ text array and concatenates strings
// Format: [ (text1) -50 (text2) ] where numbers are adjustments
func parseTextArray(arrStr string) string {
	var result strings.Builder

	// Remove brackets if present
	arrStr = strings.TrimSpace(arrStr)
	arrStr = strings.TrimPrefix(arrStr, "[")
	arrStr = strings.TrimSuffix(arrStr, "]")

	// Parse elements - strings in parentheses or numbers
	// Pattern: (text) or number
	pattern := regexp.MustCompile(`\(([^)]*)\)|([\d\.\-]+)`)
	matches := pattern.FindAllStringSubmatch(arrStr, -1)

	for _, match := range matches {
		if match[1] != "" {
			// It's a string
			result.WriteString(unescapePDFString(match[1]))
		}
		// Numbers are adjustments, we ignore them for text extraction
	}

	return result.String()
}

// unescapePDFString is defined in metadata.go
