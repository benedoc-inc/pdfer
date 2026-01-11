package manipulate

import (
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/write"
)

func TestExtractPages(t *testing.T) {
	// Create a test PDF with 5 pages
	builder := write.NewSimplePDFBuilder()
	for i := 1; i <= 5; i++ {
		page := builder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Extract pages 2, 3, and 4
	extractedPDF, err := ExtractPages(pdfBytes, []int{2, 3, 4}, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract pages: %v", err)
	}

	// Verify extracted PDF
	doc, err := extract.ExtractContent(extractedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content from extracted PDF: %v", err)
	}

	if len(doc.Pages) != 3 {
		t.Errorf("Expected 3 pages in extracted PDF, got %d", len(doc.Pages))
	}
}

func TestMergePDFs(t *testing.T) {
	// Create first PDF with 2 pages
	builder1 := write.NewSimplePDFBuilder()
	for i := 1; i <= 2; i++ {
		page := builder1.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("PDF1 Page %d", i))
		content.EndText()
		builder1.FinalizePage(page)
	}
	pdf1, err := builder1.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF1: %v", err)
	}

	// Create second PDF with 2 pages
	builder2 := write.NewSimplePDFBuilder()
	for i := 1; i <= 2; i++ {
		page := builder2.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("PDF2 Page %d", i))
		content.EndText()
		builder2.FinalizePage(page)
	}
	pdf2, err := builder2.Bytes()
	if err != nil {
		t.Fatalf("Failed to create PDF2: %v", err)
	}

	// Merge PDFs
	mergedPDF, err := MergePDFs([][]byte{pdf1, pdf2}, nil, false)
	if err != nil {
		t.Fatalf("Failed to merge PDFs: %v", err)
	}

	// Verify merged PDF
	doc, err := extract.ExtractContent(mergedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content from merged PDF: %v", err)
	}

	if len(doc.Pages) != 4 {
		t.Errorf("Expected 4 pages in merged PDF, got %d", len(doc.Pages))
	}
}

func TestSplitPDF(t *testing.T) {
	// Create a test PDF with 6 pages
	builder := write.NewSimplePDFBuilder()
	for i := 1; i <= 6; i++ {
		page := builder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Split into ranges: 1-2, 3-4, 5-6
	ranges := []PageRange{
		{Start: 1, End: 2},
		{Start: 3, End: 4},
		{Start: 5, End: 6},
	}

	splitPDFs, err := SplitPDF(pdfBytes, ranges, nil, false)
	if err != nil {
		t.Fatalf("Failed to split PDF: %v", err)
	}

	if len(splitPDFs) != 3 {
		t.Fatalf("Expected 3 split PDFs, got %d", len(splitPDFs))
	}

	// Verify each split PDF
	for i, splitPDF := range splitPDFs {
		doc, err := extract.ExtractContent(splitPDF, nil, false)
		if err != nil {
			t.Fatalf("Failed to extract content from split PDF %d: %v", i+1, err)
		}

		expectedPages := 2
		if len(doc.Pages) != expectedPages {
			t.Errorf("Split PDF %d: Expected %d pages, got %d", i+1, expectedPages, len(doc.Pages))
		}
	}
}

func TestSplitPDFByPageCount(t *testing.T) {
	// Create a test PDF with 7 pages
	builder := write.NewSimplePDFBuilder()
	for i := 1; i <= 7; i++ {
		page := builder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Page %d", i))
		content.EndText()
		builder.FinalizePage(page)
	}

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create test PDF: %v", err)
	}

	// Split into PDFs with 3 pages each
	splitPDFs, err := SplitPDFByPageCount(pdfBytes, 3, nil, false)
	if err != nil {
		t.Fatalf("Failed to split PDF: %v", err)
	}

	// Should have 3 PDFs: 3 pages, 3 pages, 1 page
	if len(splitPDFs) != 3 {
		t.Fatalf("Expected 3 split PDFs, got %d", len(splitPDFs))
	}

	expectedCounts := []int{3, 3, 1}
	for i, splitPDF := range splitPDFs {
		doc, err := extract.ExtractContent(splitPDF, nil, false)
		if err != nil {
			t.Fatalf("Failed to extract content from split PDF %d: %v", i+1, err)
		}

		if len(doc.Pages) != expectedCounts[i] {
			t.Errorf("Split PDF %d: Expected %d pages, got %d", i+1, expectedCounts[i], len(doc.Pages))
		}
	}
}

func TestInsertPage(t *testing.T) {
	// Create source PDF with 1 page
	sourceBuilder := write.NewSimplePDFBuilder()
	sourcePage := sourceBuilder.AddPage(write.PageSizeLetter)
	sourceContent := sourcePage.Content()
	sourceContent.BeginText()
	sourceFont := sourcePage.AddStandardFont("Helvetica")
	sourceContent.SetFont(sourceFont, 12)
	sourceContent.SetTextPosition(72, 720)
	sourceContent.ShowText("Source Page")
	sourceContent.EndText()
	sourceBuilder.FinalizePage(sourcePage)
	sourcePDF, err := sourceBuilder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create source PDF: %v", err)
	}

	// Extract the page from source PDF
	extractedPDF, err := ExtractPages(sourcePDF, []int{1}, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract page: %v", err)
	}

	// Get the page object from extracted PDF
	sourceManipulator, err := NewPDFManipulator(extractedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to create source manipulator: %v", err)
	}
	sourcePageObjNums, err := sourceManipulator.getAllPageObjectNumbers()
	if err != nil {
		t.Fatalf("Failed to get source page objects: %v", err)
	}
	if len(sourcePageObjNums) != 1 {
		t.Fatalf("Expected 1 page in source, got %d", len(sourcePageObjNums))
	}
	sourcePageObjNum := sourcePageObjNums[0]
	sourcePageContent := sourceManipulator.objects[sourcePageObjNum]

	// Create target PDF with 2 pages
	targetBuilder := write.NewSimplePDFBuilder()
	for i := 1; i <= 2; i++ {
		page := targetBuilder.AddPage(write.PageSizeLetter)
		content := page.Content()
		content.BeginText()
		font := page.AddStandardFont("Helvetica")
		content.SetFont(font, 12)
		content.SetTextPosition(72, 720)
		content.ShowText(fmt.Sprintf("Target Page %d", i))
		content.EndText()
		targetBuilder.FinalizePage(page)
	}
	targetPDF, err := targetBuilder.Bytes()
	if err != nil {
		t.Fatalf("Failed to create target PDF: %v", err)
	}

	// Insert the source page at position 2 (between pages 1 and 2)
	targetManipulator, err := NewPDFManipulator(targetPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to create target manipulator: %v", err)
	}

	// Need to assign a new object number for the inserted page to avoid conflicts
	// Get the next available object number
	allObjNums := targetManipulator.pdf.Objects()
	maxObjNum := 0
	for _, objNum := range allObjNums {
		if objNum > maxObjNum {
			maxObjNum = objNum
		}
	}
	newPageObjNum := maxObjNum + 1

	// Update the page's Parent reference in the content
	pageStr := string(sourcePageContent)
	// Update Parent to point to the target's Pages object
	targetPagesObjNum, _ := targetManipulator.findParentPagesObjectForInsertion()
	updatedPageStr := setDictValue(pageStr, "/Parent", fmt.Sprintf("%d 0 R", targetPagesObjNum))
	sourcePageContent = []byte(updatedPageStr)

	err = targetManipulator.InsertPage(2, newPageObjNum, sourcePageContent)
	if err != nil {
		t.Fatalf("Failed to insert page: %v", err)
	}

	// Rebuild and verify
	modifiedPDF, err := targetManipulator.Rebuild()
	if err != nil {
		t.Fatalf("Failed to rebuild PDF: %v", err)
	}

	doc, err := extract.ExtractContent(modifiedPDF, nil, false)
	if err != nil {
		t.Fatalf("Failed to extract content: %v", err)
	}

	if len(doc.Pages) != 3 {
		t.Errorf("Expected 3 pages after insertion, got %d", len(doc.Pages))
	}
}
