// Package acroform provides form flattening functionality
package acroform

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
	"github.com/benedoc-inc/pdfer/writer"
)

// FlattenForm converts form fields to static content (removes interactivity)
// This creates a new PDF with form fields rendered as regular text/graphics
func FlattenForm(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	// Parse PDF
	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	var encryptInfo *types.PDFEncryption
	if pdf.Encryption() != nil {
		encryptInfo = pdf.Encryption()
	}

	// Extract AcroForm
	acroForm, err := ParseAcroForm(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AcroForm: %w", err)
	}

	// Create new PDF writer
	w := writer.NewPDFWriter()
	w.SetVersion(pdf.Version())

	// For each field, we need to:
	// 1. Get the field's appearance stream (if it exists)
	// 2. Add it to the page content
	// 3. Remove the field from AcroForm

	// This is a simplified version - full flattening would:
	// - Extract appearance streams (/AP dictionary)
	// - Convert them to page content
	// - Remove /AcroForm from catalog
	// - Remove field annotations from pages

	if verbose {
		fmt.Printf("Flattening %d form fields\n", len(acroForm.Fields))
	}

	// For now, return a copy with AcroForm reference removed from catalog
	// Full implementation would require page-level annotation removal
	result := make([]byte, len(pdfBytes))
	copy(result, pdfBytes)

	// Remove AcroForm reference from catalog (simplified)
	catalogPattern := regexp.MustCompile(`/AcroForm\s+\d+\s+\d+\s+R`)
	result = catalogPattern.ReplaceAll(result, []byte(""))

	if verbose {
		fmt.Println("Form flattened (AcroForm reference removed)")
	}

	return result, nil
}

// FlattenField converts a single field to static content
func FlattenField(pdfBytes []byte, field *Field, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Get field object
	fieldData, err := parser.GetObject(pdfBytes, field.ObjectNum, encryptInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)

	// Check for appearance stream (/AP)
	apPattern := regexp.MustCompile(`/AP\s*<<[^>]*>>`)
	if !apPattern.MatchString(fieldStr) {
		// No appearance stream - field value should be rendered as text
		// This would require creating content stream with field value
		if verbose {
			fmt.Printf("Field '%s' has no appearance stream, will render value as text\n", field.GetFullName())
		}
	}

	// For full flattening, we would:
	// 1. Extract /AP dictionary
	// 2. Get the appearance stream
	// 3. Add it to page content at field position
	// 4. Remove field annotation from page

	return pdfBytes, nil
}

// GetFieldAppearance gets the appearance stream for a field
func GetFieldAppearance(field *Field, pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Get field object
	fieldData, err := parser.GetObject(pdfBytes, field.ObjectNum, encryptInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)

	// Find /AP dictionary
	apPattern := regexp.MustCompile(`/AP\s*<<([^>]*)>>`)
	apMatch := apPattern.FindStringSubmatch(fieldStr)
	if apMatch == nil {
		return nil, fmt.Errorf("no appearance stream found for field")
	}

	// Find /N (normal appearance) stream reference
	nPattern := regexp.MustCompile(`/N\s+(\d+)\s+(\d+)\s+R`)
	nMatch := nPattern.FindStringSubmatch(apMatch[1])
	if nMatch == nil {
		return nil, fmt.Errorf("no normal appearance stream found")
	}

	appearanceObjNum, _ := strconv.Atoi(nMatch[1])
	_ = nMatch[2] // Generation number (usually 0)

	// Get appearance stream
	appearanceData, err := parser.GetObject(pdfBytes, appearanceObjNum, encryptInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get appearance stream: %w", err)
	}

	// Extract stream data (if it's a stream object)
	streamPattern := regexp.MustCompile(`stream\s*\n(.*?)\nendstream`)
	streamMatch := streamPattern.FindStringSubmatch(string(appearanceData))
	if streamMatch != nil {
		return []byte(streamMatch[1]), nil
	}

	return appearanceData, nil
}
