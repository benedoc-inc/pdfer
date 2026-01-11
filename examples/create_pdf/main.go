// Example: Create a simple PDF from scratch
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/core/write"
)

func main() {
	// Create a new PDF writer
	w := write.NewPDFWriter()
	w.SetVersion("1.7")

	// Create page content (a simple "Hello World")
	// Note: This is a minimal example - real content streams would need more setup
	pageContent := []byte("BT /F1 12 Tf 100 700 Td (Hello, World!) Tj ET")
	contentNum := w.AddStreamObject(
		write.Dictionary{"Length": len(pageContent)},
		pageContent,
		false, // Don't compress for readability
	)

	// Create font resource (reference to built-in font)
	fontDict := []byte("<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>")
	fontNum := w.AddObject(fontDict)

	// Create page
	pageDict := fmt.Sprintf("<</Type/Page/Parent 3 0 R/MediaBox[0 0 612 792]/Contents %d 0 R/Resources<</Font<</F1 %d 0 R>>>>>>", contentNum, fontNum)
	pageNum := w.AddObject([]byte(pageDict))

	// Create pages collection
	pagesDict := fmt.Sprintf("<</Type/Pages/Kids[%d 0 R]/Count 1>>", pageNum)
	pagesNum := w.AddObject([]byte(pagesDict))

	// Create catalog (root)
	catalogDict := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R>>", pagesNum)
	catalogNum := w.AddObject([]byte(catalogDict))
	w.SetRoot(catalogNum)

	// Generate PDF
	pdfBytes, err := w.Bytes()
	if err != nil {
		log.Fatalf("Failed to generate PDF: %v", err)
	}

	// Write to file
	outputPath := "hello.pdf"
	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Printf("Created PDF: %s (%d bytes)\n", outputPath, len(pdfBytes))
	fmt.Println("\nNote: This is a minimal PDF example.")
	fmt.Println("Opening it may show 'Hello, World!' but fonts/rendering may vary by viewer.")
}
