package write

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestSetBookmarks_Basic(t *testing.T) {
	writer := NewPDFWriter()

	// Create pages
	catalogDict := Dictionary{
		"/Type":  "/Catalog",
		"/Pages": "2 0 R",
	}
	catalogNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogNum)

	pagesDict := Dictionary{
		"/Type":  "/Pages",
		"/Count": 2,
		"/Kids":  []interface{}{"3 0 R", "4 0 R"},
	}
	pagesNum := writer.AddObject(writer.formatDictionary(pagesDict))

	page1Dict := Dictionary{
		"/Type":     "/Page",
		"/Parent":   fmt.Sprintf("%d 0 R", pagesNum),
		"/MediaBox": []interface{}{0, 0, 612, 792},
	}
	page1Num := writer.AddObject(writer.formatDictionary(page1Dict))

	page2Dict := Dictionary{
		"/Type":     "/Page",
		"/Parent":   fmt.Sprintf("%d 0 R", pagesNum),
		"/MediaBox": []interface{}{0, 0, 612, 792},
	}
	page2Num := writer.AddObject(writer.formatDictionary(page2Dict))

	// Update pages dict with correct references
	pagesDict["/Kids"] = []interface{}{fmt.Sprintf("%d 0 R", page1Num), fmt.Sprintf("%d 0 R", page2Num)}
	writer.SetObject(pagesNum, writer.formatDictionary(pagesDict))

	// Create bookmarks
	bookmarks := []types.Bookmark{
		{
			Title:      "Chapter 1",
			PageNumber: 1,
		},
		{
			Title:      "Chapter 2",
			PageNumber: 2,
		},
	}

	pageObjNums := map[int]int{
		1: page1Num,
		2: page2Num,
	}

	_, err := writer.SetBookmarks(bookmarks, pageObjNums)
	if err != nil {
		t.Fatalf("SetBookmarks failed: %v", err)
	}

	// Write PDF
	var buf bytes.Buffer
	err = writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Verify PDF contains outline structure (formatDictionary adds spaces)
	if !bytes.Contains(pdfBytes, []byte("/Type")) || !bytes.Contains(pdfBytes, []byte("Outlines")) {
		t.Error("PDF should contain /Type and Outlines")
	}
	if !bytes.Contains(pdfBytes, []byte("Chapter 1")) {
		t.Error("PDF should contain bookmark title")
	}

	// Parse and verify PDF structure
	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		t.Fatalf("Failed to parse PDF: %v", err)
	}

	// Verify catalog has Outlines reference
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		t.Fatal("Trailer should have Root reference")
	}

	// Parse root object number from "N 0 R" format
	rootObjNum := 0
	fmt.Sscanf(trailer.RootRef, "%d 0 R", &rootObjNum)
	catalogObj, err := pdf.GetObject(rootObjNum)
	if err != nil {
		t.Fatalf("Failed to get catalog object: %v", err)
	}

	catalogStr := string(catalogObj)
	if !bytes.Contains([]byte(catalogStr), []byte("/Outlines")) {
		t.Error("Catalog should contain /Outlines reference")
	}

	// Verify bookmark titles are in PDF
	if !bytes.Contains(pdfBytes, []byte("Chapter 1")) {
		t.Error("PDF should contain bookmark title 'Chapter 1'")
	}
	if !bytes.Contains(pdfBytes, []byte("Chapter 2")) {
		t.Error("PDF should contain bookmark title 'Chapter 2'")
	}
}

func TestSetBookmarks_Hierarchical(t *testing.T) {
	writer := NewPDFWriter()

	// Create a simple page structure
	catalogDict := Dictionary{
		"/Type":  "/Catalog",
		"/Pages": "2 0 R",
	}
	catalogNum := writer.AddObject(writer.formatDictionary(catalogDict))
	writer.SetRoot(catalogNum)

	pagesDict := Dictionary{
		"/Type":  "/Pages",
		"/Count": 1,
	}
	pagesNum := writer.AddObject(writer.formatDictionary(pagesDict))

	page1Dict := Dictionary{
		"/Type":     "/Page",
		"/Parent":   fmt.Sprintf("%d 0 R", pagesNum),
		"/MediaBox": []interface{}{0, 0, 612, 792},
	}
	page1Num := writer.AddObject(writer.formatDictionary(page1Dict))
	pagesDict["/Kids"] = []interface{}{fmt.Sprintf("%d 0 R", page1Num)}
	writer.SetObject(pagesNum, writer.formatDictionary(pagesDict))

	// Create hierarchical bookmarks
	bookmarks := []types.Bookmark{
		{
			Title:      "Part 1",
			PageNumber: 1,
			Children: []types.Bookmark{
				{
					Title:      "Section 1.1",
					PageNumber: 1,
				},
				{
					Title:      "Section 1.2",
					PageNumber: 1,
				},
			},
		},
	}

	pageObjNums := map[int]int{
		1: page1Num,
	}

	_, err := writer.SetBookmarks(bookmarks, pageObjNums)
	if err != nil {
		t.Fatalf("SetBookmarks failed: %v", err)
	}

	// Write PDF
	var buf bytes.Buffer
	err = writer.Write(&buf)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	pdfBytes := buf.Bytes()

	// Verify bookmark titles are in PDF (titles are escaped with parentheses)
	if !bytes.Contains(pdfBytes, []byte("Part 1")) {
		t.Error("PDF should contain bookmark title 'Part 1'")
	}
	if !bytes.Contains(pdfBytes, []byte("Section 1.1")) {
		t.Error("PDF should contain bookmark title 'Section 1.1'")
	}
	if !bytes.Contains(pdfBytes, []byte("Section 1.2")) {
		t.Error("PDF should contain bookmark title 'Section 1.2'")
	}

	// Verify outline structure exists (formatDictionary adds spaces)
	if !bytes.Contains(pdfBytes, []byte("/Type")) || !bytes.Contains(pdfBytes, []byte("Outlines")) {
		t.Error("PDF should contain /Type and Outlines")
	}
	if !bytes.Contains(pdfBytes, []byte("/First")) {
		t.Error("PDF should contain /First reference in outlines")
	}
}
