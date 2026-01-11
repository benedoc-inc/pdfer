package manipulate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RotatePage rotates a specific page by the given angle (90, 180, or 270 degrees)
// pageNumber is 1-based (first page is 1)
func (m *PDFManipulator) RotatePage(pageNumber int, angle int) error {
	if angle != 90 && angle != 180 && angle != 270 {
		return fmt.Errorf("rotation angle must be 90, 180, or 270 degrees, got %d", angle)
	}

	// Get page object number
	pageObjNum, err := m.getPageObjectNumber(pageNumber)
	if err != nil {
		return fmt.Errorf("failed to get page object: %w", err)
	}

	// Get page object
	pageObj, ok := m.objects[pageObjNum]
	if !ok {
		return fmt.Errorf("page object %d not found", pageObjNum)
	}

	pageStr := string(pageObj)

	// Get current rotation
	currentRotStr := extractDictValue(pageStr, "/Rotate")
	currentRot := 0
	if currentRotStr != "" {
		var parseErr error
		currentRot, parseErr = strconv.Atoi(currentRotStr)
		if parseErr != nil {
			currentRot = 0
		}
	}

	// Calculate new rotation (add to current, normalize to 0-360)
	newRot := (currentRot + angle) % 360
	if newRot < 0 {
		newRot += 360
	}

	// Update the /Rotate field
	updatedPageStr := setDictValue(pageStr, "/Rotate", fmt.Sprintf("%d", newRot))
	m.objects[pageObjNum] = []byte(updatedPageStr)

	if m.verbose {
		fmt.Printf("Rotated page %d by %d degrees (old: %d, new: %d)\n", pageNumber, angle, currentRot, newRot)
	}

	return nil
}

// RotateAllPages rotates all pages by the given angle
func (m *PDFManipulator) RotateAllPages(angle int) error {
	// Get all page object numbers
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return fmt.Errorf("failed to get page objects: %w", err)
	}

	for i := range pageObjNums {
		pageNumber := i + 1
		if err := m.RotatePage(pageNumber, angle); err != nil {
			return fmt.Errorf("failed to rotate page %d: %w", pageNumber, err)
		}
	}

	return nil
}

// getPageObjectNumber gets the object number for a given page (1-based)
func (m *PDFManipulator) getPageObjectNumber(pageNumber int) (int, error) {
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return 0, err
	}

	if pageNumber < 1 || pageNumber > len(pageObjNums) {
		return 0, fmt.Errorf("page number %d out of range (1-%d)", pageNumber, len(pageObjNums))
	}

	return pageObjNums[pageNumber-1], nil
}

// getAllPageObjectNumbers recursively gets all page object numbers from the pages tree
func (m *PDFManipulator) getAllPageObjectNumbers() ([]int, error) {
	trailer := m.pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("no root reference found")
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root reference: %w", err)
	}

	catalogObj, err := m.pdf.GetObject(rootObjNum)
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

	return m.extractPageObjectNumbers(pagesObjNum)
}

// extractPageObjectNumbers recursively extracts page object numbers from the pages tree
func (m *PDFManipulator) extractPageObjectNumbers(pagesObjNum int) ([]int, error) {
	var pageObjNums []int

	pagesObj, err := m.pdf.GetObject(pagesObjNum)
	if err != nil {
		return nil, err
	}

	pagesStr := string(pagesObj)

	// Check if this is a /Page (leaf) or /Pages (intermediate)
	pageType := extractDictValue(pagesStr, "/Type")
	if pageType == "/Page" {
		// This is a page - add it
		pageObjNums = append(pageObjNums, pagesObjNum)
		return pageObjNums, nil
	}

	// This is a /Pages node - get Kids and recurse
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return pageObjNums, nil
	}

	// Parse kids array
	kidsRefs := parseObjectRefArray(kidsStr)
	for _, kidRef := range kidsRefs {
		kidObjNum, err := parseObjectRef(kidRef)
		if err != nil {
			continue
		}
		// Recurse
		childPages, err := m.extractPageObjectNumbers(kidObjNum)
		if err != nil {
			continue
		}
		pageObjNums = append(pageObjNums, childPages...)
	}

	return pageObjNums, nil
}

// parseObjectRefArray parses an array of object references like "[5 0 R 6 0 R]"
func parseObjectRefArray(arrStr string) []string {
	// Remove brackets if present
	arrStr = strings.TrimSpace(arrStr)
	arrStr = strings.TrimPrefix(arrStr, "[")
	arrStr = strings.TrimSuffix(arrStr, "]")

	// Split by references (pattern: "5 0 R")
	pattern := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	matches := pattern.FindAllStringSubmatch(arrStr, -1)

	var refs []string
	for _, match := range matches {
		if len(match) > 0 {
			refs = append(refs, match[0])
		}
	}

	return refs
}
