package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/resources/font"
)

func main() {
	outputDir := filepath.Join("..", "resources")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create output directory: %v", err))
	}

	// Test PDF 1: Fonts (standard and embedded)
	createFontTestPDF(filepath.Join(outputDir, "test_fonts.pdf"))

	// Test PDF 2: Images (JPEG and PNG)
	createImageTestPDF(filepath.Join(outputDir, "test_images.pdf"))

	// Test PDF 3: Annotations
	createAnnotationTestPDF(filepath.Join(outputDir, "test_annotations.pdf"))

	// Test PDF 4: Combined (fonts, images, annotations)
	createCombinedTestPDF(filepath.Join(outputDir, "test_combined.pdf"))

	fmt.Println("All test PDFs created successfully!")
}

func createFontTestPDF(outputPath string) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	content := page.Content()

	// Standard font
	font1 := page.AddStandardFont("Helvetica")
	content.BeginText()
	content.SetFont(font1, 24)
	content.SetTextPosition(72, 750)
	content.ShowText("Standard Helvetica Font")
	content.EndText()

	// Another standard font
	font2 := page.AddStandardFont("Times-Roman")
	content.BeginText()
	content.SetFont(font2, 18)
	content.SetTextPosition(72, 700)
	content.ShowText("Standard Times-Roman Font")
	content.EndText()

	// Try to add embedded font if available
	fontPath := filepath.Join("..", "resources", "test_font.ttf")
	if fontData, err := os.ReadFile(fontPath); err == nil {
		if f, err := font.NewFont("TestFont", fontData); err == nil {
			f.AddString("Embedded Font Test")
			if fontName, err := page.AddEmbeddedFont(f); err == nil {
				content.BeginText()
				content.SetFont(fontName, 16)
				content.SetTextPosition(72, 650)
				content.ShowText("Embedded Font Test")
				content.EndText()
			}
		}
	}

	builder.FinalizePage(page)
	pdfBytes, err := builder.Bytes()
	if err != nil {
		panic(fmt.Sprintf("Failed to create font test PDF: %v", err))
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write font test PDF: %v", err))
	}
	fmt.Printf("Created: %s\n", outputPath)
}

func createImageTestPDF(outputPath string) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	content := page.Content()
	writer := builder.Writer()

	// Create a simple JPEG image (1x1 red pixel)
	jpegData := createSimpleJPEG()
	imgInfo, err := writer.AddJPEGImage(jpegData, "Im1")
	if err != nil {
		panic(fmt.Sprintf("Failed to add JPEG image: %v", err))
	}
	imageName := page.AddImage(imgInfo)

	// Draw the image
	content.SaveState()
	content.Translate(100, 600)
	content.Scale(float64(imgInfo.Width), float64(imgInfo.Height))
	content.DrawImage(imageName)
	content.RestoreState()

	// Add text label
	font1 := page.AddStandardFont("Helvetica")
	content.BeginText()
	content.SetFont(font1, 12)
	content.SetTextPosition(100, 580)
	content.ShowText("JPEG Image (1x1 red pixel)")
	content.EndText()

	// Create a simple PNG image (using AddImage which handles PNG)
	pngData := createSimplePNG()
	imgInfo2, err := writer.AddImage(pngData, "Im2")
	if err != nil {
		panic(fmt.Sprintf("Failed to add PNG image: %v", err))
	}
	imageName2 := page.AddImage(imgInfo2)

	// Draw the PNG image
	content.SaveState()
	content.Translate(100, 500)
	content.Scale(float64(imgInfo2.Width), float64(imgInfo2.Height))
	content.DrawImage(imageName2)
	content.RestoreState()

	// Add text label
	content.BeginText()
	content.SetFont(font1, 12)
	content.SetTextPosition(100, 480)
	content.ShowText("PNG Image (2x2 blue square)")
	content.EndText()

	builder.FinalizePage(page)
	pdfBytes, err := builder.Bytes()
	if err != nil {
		panic(fmt.Sprintf("Failed to create image test PDF: %v", err))
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write image test PDF: %v", err))
	}
	fmt.Printf("Created: %s\n", outputPath)
}

func createAnnotationTestPDF(outputPath string) {
	// Annotations require lower-level PDF manipulation
	// For now, create a simple PDF and we'll add annotations manually or via a helper
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	content := page.Content()

	// Add some text
	font1 := page.AddStandardFont("Helvetica")
	content.BeginText()
	content.SetFont(font1, 16)
	content.SetTextPosition(72, 750)
	content.ShowText("Annotation Test PDF")
	content.EndText()

	content.BeginText()
	content.SetFont(font1, 12)
	content.SetTextPosition(72, 700)
	content.ShowText("This PDF will have annotations added via low-level manipulation")
	content.EndText()

	builder.FinalizePage(page)

	// Add annotations to the page object
	writer := builder.Writer()
	pages := builder.Pages()
	if len(pages) > 0 {
		pageObjNum := pages[0]
		pageObj, err := writer.GetObject(pageObjNum)
		if err == nil {
			// Add a link annotation
			linkAnnot := createLinkAnnotation(writer)

			// Modify page object to include /Annots
			pageStr := string(pageObj)
			// Find the end of the page dictionary (before >>)
			annotRef := fmt.Sprintf("%d 0 R", linkAnnot)
			// Insert /Annots before the closing >>
			annotInsert := fmt.Sprintf("/Annots[%s]", annotRef)
			pageStr = insertBeforeClosing(pageStr, annotInsert)

			writer.SetObject(pageObjNum, []byte(pageStr))
		}
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		panic(fmt.Sprintf("Failed to create annotation test PDF: %v", err))
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write annotation test PDF: %v", err))
	}
	fmt.Printf("Created: %s\n", outputPath)
}

func createCombinedTestPDF(outputPath string) {
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	content := page.Content()
	writer := builder.Writer()

	// Fonts
	font1 := page.AddStandardFont("Helvetica")
	content.BeginText()
	content.SetFont(font1, 20)
	content.SetTextPosition(72, 750)
	content.ShowText("Combined Test PDF")
	content.EndText()

	// Image
	jpegData := createSimpleJPEG()
	imgInfo, _ := writer.AddJPEGImage(jpegData, "Im1")
	imageName := page.AddImage(imgInfo)
	content.SaveState()
	content.Translate(72, 650)
	content.Scale(float64(imgInfo.Width), float64(imgInfo.Height))
	content.DrawImage(imageName)
	content.RestoreState()

	// Graphics
	content.SetStrokeColorRGB(1.0, 0.0, 0.0)
	content.SetLineWidth(2.0)
	content.Rectangle(72, 500, 200, 50)
	content.Stroke()

	builder.FinalizePage(page)
	pdfBytes, err := builder.Bytes()
	if err != nil {
		panic(fmt.Sprintf("Failed to create combined test PDF: %v", err))
	}

	if err := os.WriteFile(outputPath, pdfBytes, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write combined test PDF: %v", err))
	}
	fmt.Printf("Created: %s\n", outputPath)
}

// Helper functions

func createSimpleJPEG() []byte {
	// Minimal valid JPEG (1x1 red pixel)
	// JPEG header + minimal data
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x14, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x08, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00,
		0xFF, 0xD9,
	}
}

func createSimplePNG() []byte {
	// Create a 2x2 blue PNG programmatically
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	blue := color.RGBA{0, 0, 255, 255}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, blue)
		}
	}

	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(fmt.Sprintf("Failed to encode PNG: %v", err))
	}
	return buf.Bytes()
}

func createLinkAnnotation(writer *write.PDFWriter) int {
	// Create a link annotation dictionary
	annotDict := `<</Type/Annot/Subtype/Link/Rect[72 600 200 650]/Border[0 0 2]/A<</Type/Action/S/URI/URI(https://example.com)>>>>`
	return writer.AddObject([]byte(annotDict))
}

func insertBeforeClosing(str, insert string) string {
	// Find the last >> and insert before it
	lastIdx := strings.LastIndex(str, ">>")
	if lastIdx == -1 {
		return str + insert
	}
	return str[:lastIdx] + insert + " " + str[lastIdx:]
}
