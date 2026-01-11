package manipulate

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/core/write"
)

// ExtractPages extracts specific pages from a PDF and returns a new PDF
// pageNumbers is 1-based (first page is 1)
func ExtractPages(pdfBytes []byte, pageNumbers []int, password []byte, verbose bool) ([]byte, error) {
	if len(pageNumbers) == 0 {
		return nil, fmt.Errorf("no pages specified for extraction")
	}

	// Create manipulator
	manipulator, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to create manipulator: %w", err)
	}

	// Get all page object numbers
	allPageObjNums, err := manipulator.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("failed to get page objects: %w", err)
	}

	// Validate page numbers
	for _, pageNum := range pageNumbers {
		if pageNum < 1 || pageNum > len(allPageObjNums) {
			return nil, fmt.Errorf("page number %d out of range (1-%d)", pageNum, len(allPageObjNums))
		}
	}

	// Create a new PDF with only the extracted pages
	writer := write.NewPDFWriter()
	pagesObjNum := writer.AddObject([]byte("")) // Placeholder, will update later
	pageObjNums := make([]int, 0, len(pageNumbers))

	// Copy page objects and their dependencies
	for _, pageNum := range pageNumbers {
		pageObjNum := allPageObjNums[pageNum-1]

		// Get page object
		pageObj, ok := manipulator.objects[pageObjNum]
		if !ok {
			return nil, fmt.Errorf("page object %d not found", pageObjNum)
		}

		// Copy page object to new PDF
		writer.SetObject(pageObjNum, pageObj)
		pageObjNums = append(pageObjNums, pageObjNum)

		// Update page's Parent reference to point to new Pages object
		pageStr := string(pageObj)
		updatedPageStr := setDictValue(pageStr, "/Parent", fmt.Sprintf("%d 0 R", pagesObjNum))
		writer.SetObject(pageObjNum, []byte(updatedPageStr))

		// TODO: Copy page dependencies (content streams, resources, fonts, images, etc.)
		// For now, we'll copy the page object as-is and hope dependencies are included
	}

	// Build Kids array
	kids := "["
	for i, pageNum := range pageObjNums {
		if i > 0 {
			kids += " "
		}
		kids += fmt.Sprintf("%d 0 R ", pageNum)
	}
	kids += "]"

	// Create Pages object
	pagesDict := fmt.Sprintf("<</Type/Pages/Kids%s/Count %d>>", kids, len(pageObjNums))
	writer.SetObject(pagesObjNum, []byte(pagesDict))

	// Create Catalog
	catalogObjNum := writer.AddObject([]byte(fmt.Sprintf("<</Type/Catalog/Pages %d 0 R>>", pagesObjNum)))
	writer.SetRoot(catalogObjNum)

	return writer.Bytes()
}
