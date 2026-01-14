package write

import (
	"fmt"
	"math"
)

// WatermarkOptions configures watermark appearance
type WatermarkOptions struct {
	Text      string  // Text to display (for text watermarks)
	FontName  string  // Font name (default: "Helvetica-Bold")
	FontSize  float64 // Font size in points (default: 48)
	Opacity   float64 // Opacity 0.0-1.0 (default: 0.3)
	Angle     float64 // Rotation angle in degrees (default: 45)
	Color     *Color  // Text color (default: gray)
	X         float64 // X position (default: center)
	Y         float64 // Y position (default: center)
	ImageName string  // XObject name for image watermark (if set, text is ignored)
}

// Color represents RGB color
type Color struct {
	R, G, B float64 // 0.0-1.0
}

// DefaultWatermarkOptions returns default watermark options
func DefaultWatermarkOptions() *WatermarkOptions {
	return &WatermarkOptions{
		FontName: "Helvetica-Bold",
		FontSize: 48,
		Opacity:  0.3,
		Angle:    45,
		Color:    &Color{R: 0.5, G: 0.5, B: 0.5}, // Gray
	}
}

// AddWatermark adds a watermark to a page
// Options can be nil to use defaults
func (pb *PageBuilder) AddWatermark(options *WatermarkOptions) error {
	if options == nil {
		options = DefaultWatermarkOptions()
	}

	// Save graphics state
	pb.content.SaveState()

	// Set opacity using ExtGState (if opacity < 1.0)
	if options.Opacity < 1.0 {
		// For simplicity, we'll use the gs operator with an ExtGState
		// For now, just set color with reduced opacity by adjusting RGB
		// Full ExtGState support would require adding it to resources
		opacity := options.Opacity
		pb.content.SetFillColorRGB(
			options.Color.R*opacity,
			options.Color.G*opacity,
			options.Color.B*opacity,
		)
	} else {
		pb.content.SetFillColorRGB(options.Color.R, options.Color.G, options.Color.B)
	}

	// Calculate center if not specified
	centerX := options.X
	centerY := options.Y
	if centerX == 0 && centerY == 0 {
		centerX = pb.size.Width / 2
		centerY = pb.size.Height / 2
	}

	// Translate to center position
	pb.content.Translate(centerX, centerY)

	// Rotate if angle specified
	if options.Angle != 0 {
		angleRad := options.Angle * math.Pi / 180.0
		pb.content.SetMatrix(
			math.Cos(angleRad), math.Sin(angleRad),
			-math.Sin(angleRad), math.Cos(angleRad),
			0, 0,
		)
	}

	if options.ImageName != "" {
		// Image watermark
		// Add image to resources if not already added
		if _, exists := pb.images[options.ImageName]; !exists {
			return fmt.Errorf("image %q not found in page resources", options.ImageName)
		}

		// Draw image centered using Do operator
		// Scale and position the image
		// For simplicity, scale to reasonable size and center
		// Image will be drawn at current position (0,0 after translate/rotate)
		pb.content.Translate(-pb.size.Width/4, -pb.size.Height/4) // Center the image
		pb.content.Scale(pb.size.Width/2, pb.size.Height/2)
		pb.content.DrawImage(options.ImageName)
	} else {
		// Text watermark
		if options.Text == "" {
			return fmt.Errorf("watermark text cannot be empty")
		}

		// Add font if not already added
		fontName := pb.AddStandardFont(options.FontName)

		// Begin text
		pb.content.BeginText()

		// Set font and size
		pb.content.SetFont(fontName, options.FontSize)

		// Calculate text position (center the text)
		// For simplicity, approximate text width (could be improved)
		textWidth := float64(len(options.Text)) * options.FontSize * 0.6
		textX := -textWidth / 2
		textY := -options.FontSize / 2

		pb.content.SetTextPosition(textX, textY)

		// Show text
		pb.content.ShowText(options.Text)

		pb.content.EndText()
	}

	// Restore graphics state
	pb.content.RestoreState()

	return nil
}

// AddTextWatermark is a convenience method for adding text watermarks
func (pb *PageBuilder) AddTextWatermark(text string, fontSize float64, angle float64) error {
	options := DefaultWatermarkOptions()
	options.Text = text
	if fontSize > 0 {
		options.FontSize = fontSize
	}
	if angle != 0 {
		options.Angle = angle
	}
	return pb.AddWatermark(options)
}
