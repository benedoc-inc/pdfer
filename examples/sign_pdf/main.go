// Example: digitally sign a PDF using the ByteRange / PKCS#7 mechanism.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/benedoc-inc/pdfer/core/sign"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/acroform"
	"github.com/benedoc-inc/pdfer/types"
)

func main() {
	// ---- Build a simple PDF with a filled text field ----
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	helvetica := page.AddStandardFont("Helvetica")
	helveticaBold := page.AddStandardFont("Helvetica-Bold")

	c := page.Content()

	// Title
	c.BeginText().
		SetFont(helveticaBold, 24).
		SetTextPosition(72, 720).
		ShowText("Document Signature Demo").
		EndText()

	// Subtitle
	c.BeginText().
		SetFont(helvetica, 12).
		SetTextPosition(72, 690).
		ShowText("This PDF was signed using pdfer's built-in PKCS#7 / ByteRange digital signature support.").
		EndText()

	// Separator line
	c.MoveTo(72, 678).LineTo(540, 678).SetLineWidth(0.5).Stroke()

	// Body text — 8 lines, 18pt leading, starting at y=650
	lines := []string{
		"What you are looking at:",
		"  1. A PDF form field filled with the author name below.",
		"  2. An invisible digital signature field on this page.",
		"  3. A PKCS#7 detached signature covering the entire file",
		"     via the /ByteRange mechanism (ISO 32000 §12.8).",
		"The signature was produced in pure Go with no external",
		"libraries. Open in Adobe Acrobat Reader > Tools > Signatures",
		"to inspect the embedded certificate and byte ranges.",
	}
	y := 650.0
	for _, line := range lines {
		c.BeginText().
			SetFont(helvetica, 11).
			SetTextPosition(72, y).
			ShowText(line).
			EndText()
		y -= 18
	}
	// y is now 650 - 7*18 = 524 after the last line

	// Field label
	c.BeginText().
		SetFont(helveticaBold, 11).
		SetTextPosition(72, 460).
		ShowText("Author:").
		EndText()

	builder.FinalizePage(page)

	// Add an AcroForm text field — placed well below the body text
	fb := acroform.NewFormBuilder(builder)
	fb.AddTextField("author", []float64{72, 436, 350, 454}, 0).
		SetDefault("Enter author name")

	if _, err := fb.BuildForm(); err != nil {
		log.Fatalf("BuildForm: %v", err)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		log.Fatalf("build PDF: %v", err)
	}

	// ---- Fill the text field ----
	pdfBytes, err = acroform.FillFormFields(pdfBytes, types.FormData{
		"author": "Alice Example",
	}, nil, false)
	if err != nil {
		log.Fatalf("fill form: %v", err)
	}

	// ---- Generate a self-signed test certificate ----
	key, cert, err := sign.GenerateTestCredentials()
	if err != nil {
		log.Fatalf("generate credentials: %v", err)
	}
	fmt.Printf("Signer:  %s\n", cert.Subject.CommonName)
	fmt.Printf("Serial:  %s\n", cert.SerialNumber)
	fmt.Printf("Valid:   %s → %s\n", cert.NotBefore.Format("2006-01-02"), cert.NotAfter.Format("2006-01-02"))

	// ---- Sign the PDF ----
	signed, err := sign.SignPDF(pdfBytes, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Reason:      "Demonstration of pdfer digital signature",
		Location:    "pdfer library",
		FieldName:   "Signature1",
		Page:        0,
	})
	if err != nil {
		log.Fatalf("sign PDF: %v", err)
	}

	outPath := "/tmp/signed_demo.pdf"
	if err := os.WriteFile(outPath, signed, 0644); err != nil {
		log.Fatalf("write: %v", err)
	}

	fmt.Printf("Written: %s (%d bytes)\n", outPath, len(signed))
	fmt.Println("Open in Adobe Acrobat Reader → Tools → Signatures to inspect.")
}
