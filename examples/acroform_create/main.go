// Example: Create a PDF with AcroForm fields
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/acroform"
	"github.com/benedoc-inc/pdfer/writer"
)

func main() {
	// Create a page builder
	builder := writer.NewSimplePDFBuilder()
	page := builder.AddPage(writer.PageSizeLetter)

	// Add some content
	content := page.Content()
	content.
		BeginText().
		SetFont(page.AddStandardFont("Helvetica"), 16).
		SetTextPosition(72, 750).
		ShowText("Sample AcroForm").
		EndText()

	// Create AcroForm fields
	fieldBuilder := acroform.NewFieldBuilder(builder.Writer())

	// Add a text field
	textField := fieldBuilder.AddTextField("name", []float64{72, 700, 300, 720}, 0)
	textField.SetDefault("Enter your name").
		SetMaxLength(50).
		SetRequired(true)

	// Add a checkbox
	checkbox := fieldBuilder.AddCheckbox("agree", []float64{72, 650, 90, 670}, 0)
	checkbox.SetDefault(false)

	// Add a dropdown
	dropdown := fieldBuilder.AddChoiceField("country", []float64{72, 600, 200, 620}, 0,
		[]string{"USA", "Canada", "Mexico", "Other"})
	dropdown.SetDefault("USA")

	// Finalize page first
	builder.FinalizePage(page)

	// Build the AcroForm (after pages are created)
	acroFormNum, err := fieldBuilder.Build()
	if err != nil {
		log.Fatalf("Failed to build AcroForm: %v", err)
	}

	// Get the writer and update catalog to include AcroForm
	w := builder.Writer()
	pagesObjNum := builder.PagesObjNum()

	// Create catalog with AcroForm reference
	catalogDict := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>",
		pagesObjNum, acroFormNum)
	catalogNum := w.AddObject([]byte(catalogDict))
	w.SetRoot(catalogNum)

	// Generate PDF using builder (which will use the updated catalog)
	pdfBytes, err := builder.Bytes()
	if err != nil {
		log.Fatalf("Failed to generate PDF: %v", err)
	}

	// Write to file
	outputPath := "acroform_example.pdf"
	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		log.Fatalf("Failed to write PDF: %v", err)
	}

	fmt.Printf("Created PDF with AcroForm: %s (%d bytes)\n", outputPath, len(pdfBytes))
	fmt.Println("The PDF contains:")
	fmt.Println("  - Text field: 'name'")
	fmt.Println("  - Checkbox: 'agree'")
	fmt.Println("  - Dropdown: 'country'")
}
