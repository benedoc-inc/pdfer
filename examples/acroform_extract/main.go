// Example: Extract AcroForm fields from a PDF
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/forms/acroform"
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

	// Extract AcroForm
	acroForm, err := acroform.ExtractAcroForm(pdfBytes, password, true)
	if err != nil {
		log.Fatalf("Failed to extract AcroForm: %v", err)
	}

	fmt.Printf("Found AcroForm with %d fields\n\n", len(acroForm.Fields))

	// Convert to FormSchema
	schema := acroForm.ToFormSchema()
	fmt.Printf("Form Type: %s\n", schema.Metadata.FormType)
	fmt.Printf("Total Questions: %d\n\n", len(schema.Questions))

	// List all fields
	fmt.Println("Fields:")
	for i, q := range schema.Questions {
		fmt.Printf("  %d. %s (%s)", i+1, q.Name, q.Type)
		if q.Label != "" {
			fmt.Printf(" - %s", q.Label)
		}
		if q.Required {
			fmt.Printf(" [REQUIRED]")
		}
		if q.ReadOnly {
			fmt.Printf(" [READONLY]")
		}
		fmt.Println()
	}

	// Get field values
	fmt.Println("\nField Values:")
	values := acroForm.GetFieldValues()
	for name, value := range values {
		fmt.Printf("  %s = %v\n", name, value)
	}

	// Show field details
	if len(acroForm.Fields) > 0 {
		fmt.Println("\nField Details:")
		for _, field := range acroForm.Fields[:min(5, len(acroForm.Fields))] {
			fmt.Printf("  Name: %s\n", field.GetFullName())
			fmt.Printf("    Type: %s\n", field.FT)
			fmt.Printf("    Object: %d\n", field.ObjectNum)
			if field.V != nil {
				fmt.Printf("    Value: %v\n", field.V)
			}
			if len(field.Opt) > 0 {
				fmt.Printf("    Options: %v\n", field.Opt)
			}
			fmt.Println()
		}
		if len(acroForm.Fields) > 5 {
			fmt.Printf("  ... and %d more fields\n", len(acroForm.Fields)-5)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
