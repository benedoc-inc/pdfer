// Example: Fill AcroForm fields in a PDF (including object stream support)
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/forms/acroform"
	"github.com/benedoc-inc/pdfer/types"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <pdf_file> [password]")
		fmt.Println("\nExample:")
		fmt.Println("  go run main.go form.pdf")
		fmt.Println("  go run main.go encrypted.pdf secret")
		os.Exit(1)
	}

	pdfPath := os.Args[1]
	password := []byte("")
	if len(os.Args) > 2 {
		password = []byte(os.Args[2])
	}

	// Read PDF
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		log.Fatalf("Failed to read PDF: %v", err)
	}

	fmt.Printf("Reading PDF: %s (%d bytes)\n\n", pdfPath, len(pdfBytes))

	// Extract AcroForm to see available fields
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, false)
	if err != nil {
		log.Fatalf("Failed to extract AcroForm: %v", err)
	}

	if len(acroForm.Fields) == 0 {
		fmt.Println("No AcroForm fields found in PDF")
		os.Exit(0)
	}

	fmt.Printf("Found %d fields:\n", len(acroForm.Fields))
	for i, field := range acroForm.Fields {
		if i < 10 {
			fmt.Printf("  %d. %s (%s)\n", i+1, field.GetFullName(), field.FT)
		}
	}
	if len(acroForm.Fields) > 10 {
		fmt.Printf("  ... and %d more fields\n", len(acroForm.Fields)-10)
	}

	// Get current values
	fmt.Println("\nCurrent field values:")
	values := acroForm.GetFieldValues()
	for name, value := range values {
		fmt.Printf("  %s = %v\n", name, value)
		if len(values) > 20 {
			break // Limit output
		}
	}

	// Create form data to fill
	formData := types.FormData{}

	// Fill first few text fields as example
	textFieldCount := 0
	for _, field := range acroForm.Fields {
		if field.FT == "Tx" && textFieldCount < 3 {
			formData[field.GetFullName()] = fmt.Sprintf("Filled Value %d", textFieldCount+1)
			textFieldCount++
		}
	}

	if len(formData) == 0 {
		fmt.Println("\nNo text fields found to fill")
		os.Exit(0)
	}

	fmt.Printf("\nFilling %d fields...\n", len(formData))
	for name, value := range formData {
		fmt.Printf("  %s = %v\n", name, value)
	}

	// Fill the form
	filledPDF, err := acroform.FillFormFieldsWithStreams(pdfBytes, formData, password, true)
	if err != nil {
		log.Fatalf("Failed to fill form: %v", err)
	}

	// Write filled PDF
	outputPath := pdfPath[:len(pdfPath)-4] + "_filled.pdf"
	if err := os.WriteFile(outputPath, filledPDF, 0644); err != nil {
		log.Fatalf("Failed to write filled PDF: %v", err)
	}

	fmt.Printf("\nCreated filled PDF: %s (%d bytes)\n", outputPath, len(filledPDF))
	fmt.Println("Form fields have been filled with new values.")
}
