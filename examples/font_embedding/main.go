// Example: Create a PDF with an embedded TrueType font
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/resources/font"
)

func main() {
	// Try to find a system font, or use a font file path provided as argument
	var fontPath string
	if len(os.Args) > 1 {
		fontPath = os.Args[1]
	} else {
		// Default to Arial Unicode if available
		fontPath = "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"
	}

	// Check if font file exists
	if _, err := os.Stat(fontPath); os.IsNotExist(err) {
		log.Fatalf("Font file not found: %s\n\nUsage: %s [font_path.ttf]\n\nExample system fonts on macOS:\n  /System/Library/Fonts/Supplemental/Arial Bold.ttf\n  /System/Library/Fonts/Supplemental/Times New Roman.ttf", fontPath, os.Args[0])
	}

	// Read font file
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		log.Fatalf("Failed to read font file: %v", err)
	}

	fmt.Printf("Loaded font file: %s (%d bytes)\n", fontPath, len(fontData))

	// Create font object
	f, err := font.NewFont("EmbeddedFont", fontData)
	if err != nil {
		log.Fatalf("Failed to create font: %v", err)
	}

	// Add the text we want to display (this creates a subset with only needed glyphs)
	text := "Hello, World! This is an embedded TrueType font. 123"
	f.AddString(text)

	fmt.Printf("Font subset will include %d unique characters\n", len(f.Subset))

	// Create PDF builder
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeA4)

	// Add embedded font to page
	fontName, err := page.AddEmbeddedFont(f)
	if err != nil {
		log.Fatalf("Failed to add embedded font: %v", err)
	}

	fmt.Printf("Font resource name: %s\n", fontName)

	// Add text content
	content := page.Content()
	content.
		BeginText().
		SetFont(fontName, 24).
		SetTextPosition(100, 750).
		ShowText(text).
		EndText()

	// Add another line with different size
	content.
		BeginText().
		SetFont(fontName, 18).
		SetTextPosition(100, 700).
		ShowText("Smaller text size").
		EndText()

	// Finalize page
	builder.FinalizePage(page)

	// Generate PDF
	pdfBytes, err := builder.Bytes()
	if err != nil {
		log.Fatalf("Failed to generate PDF: %v", err)
	}

	// Write to file
	outputPath := "font_embedding_example.pdf"
	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Printf("\nCreated PDF: %s (%d bytes)\n", outputPath, len(pdfBytes))
	fmt.Println("\nThe PDF contains an embedded TrueType font with subsetting.")
	fmt.Println("Only the glyphs needed for the displayed text are included.")
}
