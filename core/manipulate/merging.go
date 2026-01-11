package manipulate

import (
	"fmt"
	"regexp"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// MergePDFs merges multiple PDFs into a single PDF
// Returns a new PDF with all pages from all input PDFs
func MergePDFs(pdfBytesList [][]byte, passwords [][]byte, verbose bool) ([]byte, error) {
	if len(pdfBytesList) == 0 {
		return nil, fmt.Errorf("no PDFs to merge")
	}

	writer := write.NewPDFWriter()
	var allPageObjNums []int
	nextObjNum := 1

	// Process each PDF
	for pdfIdx, pdfBytes := range pdfBytesList {
		var password []byte
		if pdfIdx < len(passwords) {
			password = passwords[pdfIdx]
		}

		pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
			Password: password,
			Verbose:  verbose,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse PDF %d: %w", pdfIdx+1, err)
		}

		// Get all page object numbers from this PDF
		pageObjNums, err := getAllPageObjectNumbersFromPDF(pdf)
		if err != nil {
			return nil, fmt.Errorf("failed to get pages from PDF %d: %w", pdfIdx+1, err)
		}

		// Copy all objects from this PDF to the merged PDF
		// Need to remap object numbers to avoid conflicts
		objNumMap := make(map[int]int) // old objNum -> new objNum

		for _, objNum := range pdf.Objects() {
			obj, err := pdf.GetObject(objNum)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: failed to get object %d from PDF %d: %v\n", objNum, pdfIdx+1, err)
				}
				continue
			}

			newObjNum := nextObjNum
			nextObjNum++
			objNumMap[objNum] = newObjNum

			// Update object references in the content
			updatedObj := updateObjectReferences(obj, objNumMap, pdfIdx == 0)

			writer.SetObject(newObjNum, updatedObj)

			// If this is a page object, add it to our pages list
			for _, pageObjNum := range pageObjNums {
				if objNum == pageObjNum {
					allPageObjNums = append(allPageObjNums, newObjNum)
					break
				}
			}
		}
	}

	// Create Pages object with all pages
	pagesObjNum := nextObjNum
	nextObjNum++

	kids := "["
	for i, pageNum := range allPageObjNums {
		if i > 0 {
			kids += " "
		}
		kids += fmt.Sprintf("%d 0 R ", pageNum)
	}
	kids += "]"

	pagesDict := fmt.Sprintf("<</Type/Pages/Kids%s/Count %d>>", kids, len(allPageObjNums))
	writer.SetObject(pagesObjNum, []byte(pagesDict))

	// Create Catalog
	catalogObjNum := nextObjNum
	catalogDict := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R>>", pagesObjNum)
	writer.SetObject(catalogObjNum, []byte(catalogDict))
	writer.SetRoot(catalogObjNum)

	return writer.Bytes()
}

// getAllPageObjectNumbersFromPDF gets all page object numbers from a parsed PDF
func getAllPageObjectNumbersFromPDF(pdf *parse.PDF) ([]int, error) {
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("no root reference found")
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root reference: %w", err)
	}

	catalogObj, err := pdf.GetObject(rootObjNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog: %w", err)
	}

	catalogStr := string(catalogObj)
	pagesRef := extractDictValue(catalogStr, "/Pages")
	if pagesRef == "" {
		return nil, fmt.Errorf("no /Pages reference in catalog")
	}

	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Pages reference: %w", err)
	}

	return extractPageObjectNumbersFromPDF(pdf, pagesObjNum)
}

// extractPageObjectNumbersFromPDF recursively extracts page object numbers
func extractPageObjectNumbersFromPDF(pdf *parse.PDF, pagesObjNum int) ([]int, error) {
	var pageObjNums []int

	pagesObj, err := pdf.GetObject(pagesObjNum)
	if err != nil {
		return nil, err
	}

	pagesStr := string(pagesObj)
	pageType := extractDictValue(pagesStr, "/Type")
	if pageType == "/Page" {
		pageObjNums = append(pageObjNums, pagesObjNum)
		return pageObjNums, nil
	}

	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return pageObjNums, nil
	}

	kidsRefs := parseObjectRefArray(kidsStr)
	for _, kidRef := range kidsRefs {
		kidObjNum, err := parseObjectRef(kidRef)
		if err != nil {
			continue
		}
		childPages, err := extractPageObjectNumbersFromPDF(pdf, kidObjNum)
		if err != nil {
			continue
		}
		pageObjNums = append(pageObjNums, childPages...)
	}

	return pageObjNums, nil
}

// updateObjectReferences updates object references in a PDF object string
// objNumMap maps old object numbers to new object numbers
func updateObjectReferences(obj []byte, objNumMap map[int]int, isFirstPDF bool) []byte {
	objStr := string(obj)

	// Find all object references (pattern: "5 0 R")
	refPattern := regexp.MustCompile(`(\d+)\s+(\d+)\s+R`)

	updatedStr := refPattern.ReplaceAllStringFunc(objStr, func(match string) string {
		// Extract object number
		var objNum, genNum int
		fmt.Sscanf(match, "%d %d R", &objNum, &genNum)

		// Check if we need to remap this reference
		if newObjNum, ok := objNumMap[objNum]; ok {
			return fmt.Sprintf("%d 0 R", newObjNum)
		}

		// For first PDF, keep original references (they're already in the map)
		// For subsequent PDFs, references to objects not in the map should be updated
		// For now, return as-is (this is a simplified version)
		return match
	})

	return []byte(updatedStr)
}
