// Example: Extract XFA from an encrypted PDF
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer"
	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/xfa"
)

func main() {
	inputPDF := flag.String("input", "", "Path to input PDF file")
	password := flag.String("password", "", "PDF password (empty for none)")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	if *inputPDF == "" {
		fmt.Println("Usage: extract_xfa -input <pdf_file> [-password <password>] [-verbose]")
		os.Exit(1)
	}

	// Read PDF
	pdfBytes, err := os.ReadFile(*inputPDF)
	if err != nil {
		log.Fatalf("Error reading PDF: %v", err)
	}

	fmt.Printf("Read PDF: %d bytes\n", len(pdfBytes))

	// Check for encryption
	var encryptInfo *pdfer.Encryption
	if bytes.Contains(pdfBytes, []byte("/Encrypt")) {
		fmt.Println("PDF is encrypted, decrypting...")

		_, encryptInfo, err = encryption.DecryptPDF(pdfBytes, []byte(*password), *verbose)
		if err != nil {
			log.Fatalf("Decryption failed: %v", err)
		}
		fmt.Println("Decryption successful")
	}

	// Extract XFA streams
	streams, err := xfa.ExtractAllXFAStreams(pdfBytes, encryptInfo, *verbose)
	if err != nil {
		log.Fatalf("Failed to extract XFA: %v", err)
	}

	// Print extracted streams
	fmt.Println("\nExtracted XFA Streams:")
	if streams.Template != nil {
		fmt.Printf("  Template: %d bytes (object %d)\n", len(streams.Template.Data), streams.Template.ObjectNumber)
	}
	if streams.Datasets != nil {
		fmt.Printf("  Datasets: %d bytes (object %d)\n", len(streams.Datasets.Data), streams.Datasets.ObjectNumber)
	}
	if streams.Config != nil {
		fmt.Printf("  Config: %d bytes (object %d)\n", len(streams.Config.Data), streams.Config.ObjectNumber)
	}
	if streams.LocaleSet != nil {
		fmt.Printf("  LocaleSet: %d bytes (object %d)\n", len(streams.LocaleSet.Data), streams.LocaleSet.ObjectNumber)
	}

	// Parse form structure if template exists
	if streams.Template != nil && len(streams.Template.Data) > 0 {
		form, err := xfa.ParseXFAForm(string(streams.Template.Data), false)
		if err != nil {
			fmt.Printf("\nCould not parse form: %v\n", err)
		} else {
			fmt.Printf("\nParsed Form Structure:\n")
			fmt.Printf("  Questions: %d\n", len(form.Questions))
			fmt.Printf("  Rules: %d\n", len(form.Rules))

			if len(form.Questions) > 0 {
				fmt.Println("\nFirst 10 questions:")
				for i, q := range form.Questions {
					if i >= 10 {
						fmt.Printf("  ... and %d more\n", len(form.Questions)-10)
						break
					}
					fmt.Printf("  - %s: %s (%s)\n", q.ID, q.Label, q.Type)
				}
			}
		}
	}
}
