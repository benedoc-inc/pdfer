package manipulate

import (
	"fmt"
	"regexp"
)

// InsertPage inserts a page at a specific position
// pageNumber is 1-based (first page is 1, 0 means insert at end)
// The page to insert must be from another PDF (use ExtractPages first)
func (m *PDFManipulator) InsertPage(pageNumber int, pageObjNum int, pageContent []byte) error {
	// Get all page object numbers
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return fmt.Errorf("failed to get page objects: %w", err)
	}

	// Validate page number
	if pageNumber < 0 || pageNumber > len(pageObjNums)+1 {
		return fmt.Errorf("page number %d out of range (0-%d)", pageNumber, len(pageObjNums)+1)
	}

	// Find the parent Pages object
	parentPagesObjNum, err := m.findParentPagesObjectForInsertion()
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

	// Parse kids array
	kidsRefs := parseObjectRefArray(kidsStr)

	// Insert the new page reference at the specified position
	newKidsRefs := make([]string, 0, len(kidsRefs)+1)
	if pageNumber == 0 || pageNumber > len(kidsRefs) {
		// Insert at end
		newKidsRefs = append(newKidsRefs, kidsRefs...)
		newKidsRefs = append(newKidsRefs, fmt.Sprintf("%d 0 R", pageObjNum))
	} else {
		// Insert at specific position (1-based, convert to 0-based)
		insertIdx := pageNumber - 1
		newKidsRefs = append(newKidsRefs, kidsRefs[:insertIdx]...)
		newKidsRefs = append(newKidsRefs, fmt.Sprintf("%d 0 R", pageObjNum))
		newKidsRefs = append(newKidsRefs, kidsRefs[insertIdx:]...)
	}

	// Rebuild Kids array string
	newKidsStr := "["
	for i, ref := range newKidsRefs {
		if i > 0 {
			newKidsStr += " "
		}
		newKidsStr += ref
	}
	newKidsStr += " ]"

	// Add the page object to our objects map
	m.objects[pageObjNum] = pageContent

	// Update /Kids and /Count in parent Pages object
	updatedPagesStr := setDictValue(parentPagesStr, "/Kids", newKidsStr)
	newCount := len(newKidsRefs)
	countPattern := regexp.MustCompile(`/Count\s+\d+`)
	updatedPagesStr = countPattern.ReplaceAllString(updatedPagesStr, fmt.Sprintf("/Count %d", newCount))

	m.objects[parentPagesObjNum] = []byte(updatedPagesStr)

	if m.verbose {
		fmt.Printf("Inserted page (object %d) at position %d\n", pageObjNum, pageNumber)
	}

	return nil
}

// findParentPagesObjectForInsertion finds the root Pages object for insertion
func (m *PDFManipulator) findParentPagesObjectForInsertion() (int, error) {
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

	return pagesObjNum, nil
}
