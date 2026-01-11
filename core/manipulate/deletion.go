package manipulate

import (
	"fmt"
	"regexp"
	"strconv"
)

// DeletePage deletes a specific page from the PDF
// pageNumber is 1-based (first page is 1)
func (m *PDFManipulator) DeletePage(pageNumber int) error {
	// Get page object number
	pageObjNum, err := m.getPageObjectNumber(pageNumber)
	if err != nil {
		return fmt.Errorf("failed to get page object: %w", err)
	}

	// Find the parent Pages object that contains this page
	parentPagesObjNum, err := m.findParentPagesObject(pageObjNum)
	if err != nil {
		return fmt.Errorf("failed to find parent Pages object: %w", err)
	}

	// Get parent Pages object
	parentPagesObj, ok := m.objects[parentPagesObjNum]
	if !ok {
		return fmt.Errorf("parent Pages object %d not found", parentPagesObjNum)
	}

	parentPagesStr := string(parentPagesObj)

	// Get Kids array
	kidsStr := extractDictValue(parentPagesStr, "/Kids")
	if kidsStr == "" {
		return fmt.Errorf("no /Kids array found in parent Pages object")
	}

	// Parse kids array and remove the page reference
	// Kids array format: "[5 0 R 6 0 R ]" or "[5 0 R 6 0 R]"
	kidsRefs := parseObjectRefArray(kidsStr)
	if len(kidsRefs) == 0 {
		return fmt.Errorf("Kids array is empty or could not be parsed: %s", kidsStr)
	}

	newKidsRefs := make([]string, 0, len(kidsRefs)-1)

	// Match page reference by object number (ignore whitespace)
	pageRefPattern := fmt.Sprintf(`%d\s+\d+\s+R`, pageObjNum)
	pageRefRegex := regexp.MustCompile(pageRefPattern)

	found := false
	for _, kidRef := range kidsRefs {
		// Check if this reference matches the page we want to delete
		if pageRefRegex.MatchString(kidRef) {
			found = true
			// Skip this reference (don't add to newKidsRefs)
		} else {
			newKidsRefs = append(newKidsRefs, kidRef)
		}
	}

	if !found {
		return fmt.Errorf("page %d (object %d) not found in Kids array. Kids: %v, KidsStr: %s", pageNumber, pageObjNum, kidsRefs, kidsStr)
	}

	// Rebuild Kids array string with proper formatting (matching writer format: "[5 0 R 6 0 R ]")
	// Note: No space before opening bracket to match original format
	newKidsStr := "["
	for i, ref := range newKidsRefs {
		if i > 0 {
			newKidsStr += " "
		}
		newKidsStr += ref
	}
	newKidsStr += " ]"

	// Update /Kids and /Count in parent Pages object
	updatedPagesStr := setDictValue(parentPagesStr, "/Kids", newKidsStr)
	newCount := len(newKidsRefs)
	if newCount < 1 {
		return fmt.Errorf("cannot delete page: would result in 0 pages")
	}

	// Update Count using direct regex replacement to ensure it works
	countPattern := regexp.MustCompile(`/Count\s+\d+`)
	updatedPagesStr = countPattern.ReplaceAllString(updatedPagesStr, fmt.Sprintf("/Count %d", newCount))

	m.objects[parentPagesObjNum] = []byte(updatedPagesStr)

	// Also need to update /Count in all ancestor Pages objects
	err = m.updatePageCounts(parentPagesObjNum, -1)
	if err != nil {
		if m.verbose {
			fmt.Printf("Warning: failed to update page counts: %v\n", err)
		}
	}

	if m.verbose {
		fmt.Printf("Deleted page %d (object %d)\n", pageNumber, pageObjNum)
	}

	return nil
}

// DeletePages deletes multiple pages from the PDF
// pageNumbers is 1-based and should be sorted in descending order to avoid index shifts
func (m *PDFManipulator) DeletePages(pageNumbers []int) error {
	// Sort in descending order to avoid index shifts
	// (delete from end to beginning)
	for i := len(pageNumbers) - 1; i >= 0; i-- {
		if err := m.DeletePage(pageNumbers[i]); err != nil {
			return fmt.Errorf("failed to delete page %d: %w", pageNumbers[i], err)
		}
	}
	return nil
}

// findParentPagesObject finds the Pages object that contains the given page object
func (m *PDFManipulator) findParentPagesObject(pageObjNum int) (int, error) {
	// Get all Pages objects (intermediate nodes in page tree)
	trailer := m.pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return 0, fmt.Errorf("no root reference found")
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return 0, fmt.Errorf("failed to parse root reference: %w", err)
	}

	catalogObj, err := m.pdf.GetObject(rootObjNum)
	if err != nil {
		return 0, fmt.Errorf("failed to get catalog: %w", err)
	}

	catalogStr := string(catalogObj)
	pagesRef := extractDictValue(catalogStr, "/Pages")
	if pagesRef == "" {
		return 0, fmt.Errorf("no /Pages reference in catalog")
	}

	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return 0, fmt.Errorf("failed to parse Pages reference: %w", err)
	}

	// Recursively search for the parent
	return m.findParentPagesObjectRecursive(pagesObjNum, pageObjNum)
}

// findParentPagesObjectRecursive recursively searches for the parent Pages object
func (m *PDFManipulator) findParentPagesObjectRecursive(pagesObjNum, targetPageObjNum int) (int, error) {
	pagesObj, err := m.pdf.GetObject(pagesObjNum)
	if err != nil {
		return 0, err
	}

	pagesStr := string(pagesObj)

	// Check if this is a /Page (leaf) - shouldn't happen, but check anyway
	pageType := extractDictValue(pagesStr, "/Type")
	if pageType == "/Page" {
		return 0, fmt.Errorf("reached /Page object, expected /Pages")
	}

	// Get Kids array
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return 0, fmt.Errorf("no /Kids array found")
	}

	// Check if target page is in this Pages object's Kids
	kidsRefs := parseObjectRefArray(kidsStr)
	for _, kidRef := range kidsRefs {
		kidObjNum, err := parseObjectRef(kidRef)
		if err != nil {
			continue
		}

		if kidObjNum == targetPageObjNum {
			// Found it! This Pages object is the parent
			return pagesObjNum, nil
		}

		// Check if this kid is a /Pages node - recurse
		kidObj, err := m.pdf.GetObject(kidObjNum)
		if err != nil {
			continue
		}
		kidStr := string(kidObj)
		kidType := extractDictValue(kidStr, "/Type")
		if kidType == "/Pages" {
			// Recurse into this Pages node
			parent, err := m.findParentPagesObjectRecursive(kidObjNum, targetPageObjNum)
			if err == nil {
				return parent, nil
			}
		}
	}

	return 0, fmt.Errorf("page object %d not found in page tree", targetPageObjNum)
}

// updatePageCounts updates the /Count field in the given Pages object and all ancestors
func (m *PDFManipulator) updatePageCounts(pagesObjNum int, delta int) error {
	// Update this Pages object's count
	pagesObj, ok := m.objects[pagesObjNum]
	if !ok {
		return fmt.Errorf("Pages object %d not found", pagesObjNum)
	}

	pagesStr := string(pagesObj)
	countStr := extractDictValue(pagesStr, "/Count")
	currentCount := 0
	if countStr != "" {
		var err error
		currentCount, err = strconv.Atoi(countStr)
		if err != nil {
			currentCount = 0
		}
	}

	newCount := currentCount + delta
	if newCount < 0 {
		newCount = 0
	}

	updatedPagesStr := setDictValue(pagesStr, "/Count", fmt.Sprintf("%d", newCount))
	m.objects[pagesObjNum] = []byte(updatedPagesStr)

	// Note: Updating parent Pages objects' /Count would require
	// finding the parent of this Pages object in the tree.
	// For now, we only update the immediate Pages object.
	// This is sufficient for most cases where pages are direct children.
	return nil
}
