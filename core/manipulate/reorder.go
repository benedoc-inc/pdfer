package manipulate

import (
	"fmt"
)

// getRootPagesObjNum returns the object number of the root /Pages node by following
// the catalog's /Pages reference.
func (m *PDFManipulator) getRootPagesObjNum() (int, error) {
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

	pagesRef := extractDictValue(string(catalogObj), "/Pages")
	if pagesRef == "" {
		return 0, fmt.Errorf("no /Pages reference in catalog")
	}

	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return 0, fmt.Errorf("failed to parse Pages reference: %w", err)
	}

	return pagesObjNum, nil
}

// maxObjectNumber returns the highest object number currently in m.objects.
func (m *PDFManipulator) maxObjectNumber() int {
	max := 0
	for objNum := range m.objects {
		if objNum > max {
			max = objNum
		}
	}
	return max
}

// ReorderPages reorders pages according to order, which is a 1-based slice of page
// numbers representing the desired output order.  order must be a permutation of
// [1..pageCount]: same length as the current page count, each page number appearing
// exactly once.
func ReorderPages(pdfBytes []byte, order []int, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("reorder pages: %w", err)
	}

	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("reorder pages: %w", err)
	}

	pageCount := len(pageObjNums)

	// Validate order.
	if len(order) != pageCount {
		return nil, fmt.Errorf("reorder pages: order length %d does not match page count %d", len(order), pageCount)
	}
	seen := make(map[int]bool, pageCount)
	for _, p := range order {
		if p < 1 || p > pageCount {
			return nil, fmt.Errorf("reorder pages: page number %d out of range (1-%d)", p, pageCount)
		}
		if seen[p] {
			return nil, fmt.Errorf("reorder pages: duplicate page number %d in order", p)
		}
		seen[p] = true
	}

	// Find the root /Pages object.
	pagesObjNum, err := m.getRootPagesObjNum()
	if err != nil {
		return nil, fmt.Errorf("reorder pages: %w", err)
	}

	pagesObj, ok := m.objects[pagesObjNum]
	if !ok {
		return nil, fmt.Errorf("reorder pages: root /Pages object %d not found", pagesObjNum)
	}

	// Build the reordered list of page object numbers.
	reorderedObjNums := make([]int, pageCount)
	for i, p := range order {
		reorderedObjNums[i] = pageObjNums[p-1]
	}

	// Flatten the page tree: point every page's /Parent directly at the root /Pages
	// node, then rebuild /Kids on that node.
	for _, pageObjNum := range reorderedObjNums {
		pageObj, ok := m.objects[pageObjNum]
		if !ok {
			continue
		}
		updated := setDictValue(string(pageObj), "/Parent", fmt.Sprintf("%d 0 R", pagesObjNum))
		m.objects[pageObjNum] = []byte(updated)
	}

	// Rebuild /Kids array in the new order.
	newKidsStr := "["
	for i, objNum := range reorderedObjNums {
		if i > 0 {
			newKidsStr += " "
		}
		newKidsStr += fmt.Sprintf("%d 0 R", objNum)
	}
	newKidsStr += " ]"

	updatedPages := setDictValue(string(pagesObj), "/Kids", newKidsStr)
	// /Count stays the same; ensure it is correct.
	updatedPages = setDictValue(updatedPages, "/Count", fmt.Sprintf("%d", pageCount))
	m.objects[pagesObjNum] = []byte(updatedPages)

	if verbose {
		fmt.Printf("Reordered %d pages\n", pageCount)
	}

	return m.Rebuild()
}

// InsertBlankPage inserts a blank page at position (1-based; 0 = append at end).
// width and height are in PDF user units (points).
func InsertBlankPage(pdfBytes []byte, position int, width, height float64, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("insert blank page: %w", err)
	}

	// Find the root /Pages object.
	pagesObjNum, err := m.getRootPagesObjNum()
	if err != nil {
		return nil, fmt.Errorf("insert blank page: %w", err)
	}

	pagesObj, ok := m.objects[pagesObjNum]
	if !ok {
		return nil, fmt.Errorf("insert blank page: root /Pages object %d not found", pagesObjNum)
	}
	pagesStr := string(pagesObj)

	// Allocate a new object number for the blank page.
	newObjNum := m.maxObjectNumber() + 1

	// Build the blank page dictionary.
	pageDict := fmt.Sprintf("<</Type/Page/Parent %d 0 R/MediaBox[0 0 %g %g]/Resources<<>>>>",
		pagesObjNum, width, height)
	m.objects[newObjNum] = []byte(pageDict)

	// Get current /Kids array and parse it.
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return nil, fmt.Errorf("insert blank page: no /Kids array found in root /Pages object")
	}
	kidsRefs := parseObjectRefArray(kidsStr)

	// Build new /Kids with the blank page inserted at position.
	newRef := fmt.Sprintf("%d 0 R", newObjNum)
	var newKidsRefs []string
	if position == 0 || position > len(kidsRefs) {
		// Append at end.
		newKidsRefs = append(newKidsRefs, kidsRefs...)
		newKidsRefs = append(newKidsRefs, newRef)
	} else {
		insertIdx := position - 1
		newKidsRefs = append(newKidsRefs, kidsRefs[:insertIdx]...)
		newKidsRefs = append(newKidsRefs, newRef)
		newKidsRefs = append(newKidsRefs, kidsRefs[insertIdx:]...)
	}

	newKidsStr := "["
	for i, ref := range newKidsRefs {
		if i > 0 {
			newKidsStr += " "
		}
		newKidsStr += ref
	}
	newKidsStr += " ]"

	updatedPages := setDictValue(pagesStr, "/Kids", newKidsStr)
	m.objects[pagesObjNum] = []byte(updatedPages)

	// Update /Count on this node and all ancestors.
	if err := m.updatePageCounts(pagesObjNum, +1); err != nil && verbose {
		fmt.Printf("Warning: failed to update page counts: %v\n", err)
	}

	if verbose {
		fmt.Printf("Inserted blank page (object %d) at position %d\n", newObjNum, position)
	}

	return m.Rebuild()
}

// CropPage sets /CropBox on the specified page (1-based).
// rect is [llx lly urx ury] in PDF user units (points).
func CropPage(pdfBytes []byte, pageNumber int, rect [4]float64, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("crop page: %w", err)
	}

	pageObjNum, err := m.getPageObjectNumber(pageNumber)
	if err != nil {
		return nil, fmt.Errorf("crop page: %w", err)
	}

	pageObj, ok := m.objects[pageObjNum]
	if !ok {
		return nil, fmt.Errorf("crop page: page object %d not found", pageObjNum)
	}

	cropBoxStr := fmt.Sprintf("[%g %g %g %g]", rect[0], rect[1], rect[2], rect[3])
	updated := setDictValue(string(pageObj), "/CropBox", cropBoxStr)
	m.objects[pageObjNum] = []byte(updated)

	if verbose {
		fmt.Printf("Set /CropBox %s on page %d (object %d)\n", cropBoxStr, pageNumber, pageObjNum)
	}

	return m.Rebuild()
}

// SetPageSize sets /MediaBox on the specified page (1-based).
// The new MediaBox is [0 0 width height].
func SetPageSize(pdfBytes []byte, pageNumber int, width, height float64, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("set page size: %w", err)
	}

	pageObjNum, err := m.getPageObjectNumber(pageNumber)
	if err != nil {
		return nil, fmt.Errorf("set page size: %w", err)
	}

	pageObj, ok := m.objects[pageObjNum]
	if !ok {
		return nil, fmt.Errorf("set page size: page object %d not found", pageObjNum)
	}

	mediaBoxStr := fmt.Sprintf("[0 0 %g %g]", width, height)
	updated := setDictValue(string(pageObj), "/MediaBox", mediaBoxStr)
	m.objects[pageObjNum] = []byte(updated)

	if verbose {
		fmt.Printf("Set /MediaBox %s on page %d (object %d)\n", mediaBoxStr, pageNumber, pageObjNum)
	}

	return m.Rebuild()
}

// DuplicatePage inserts copies copies of pageNumber immediately after it (1-based).
func DuplicatePage(pdfBytes []byte, pageNumber, copies int, password []byte, verbose bool) ([]byte, error) {
	if copies <= 0 {
		return nil, fmt.Errorf("duplicate page: copies must be >= 1, got %d", copies)
	}

	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("duplicate page: %w", err)
	}

	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("duplicate page: %w", err)
	}

	if pageNumber < 1 || pageNumber > len(pageObjNums) {
		return nil, fmt.Errorf("duplicate page: page number %d out of range (1-%d)", pageNumber, len(pageObjNums))
	}

	srcObjNum := pageObjNums[pageNumber-1]

	// Find the root /Pages object.
	pagesObjNum, err := m.getRootPagesObjNum()
	if err != nil {
		return nil, fmt.Errorf("duplicate page: %w", err)
	}

	pagesObj, ok := m.objects[pagesObjNum]
	if !ok {
		return nil, fmt.Errorf("duplicate page: root /Pages object %d not found", pagesObjNum)
	}
	pagesStr := string(pagesObj)

	// Parse the current /Kids array.
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return nil, fmt.Errorf("duplicate page: no /Kids array found in root /Pages object")
	}
	kidsRefs := parseObjectRefArray(kidsStr)

	// Allocate new object numbers for all copies.
	baseObjNum := m.maxObjectNumber()
	newRefs := make([]string, copies)
	for i := 0; i < copies; i++ {
		newObjNum := baseObjNum + 1 + i
		// Shallow copy: share the same content streams and resources.
		srcContent := m.objects[srcObjNum]
		dupContent := make([]byte, len(srcContent))
		copy(dupContent, srcContent)
		// Ensure /Parent points at the root /Pages node.
		dupUpdated := setDictValue(string(dupContent), "/Parent", fmt.Sprintf("%d 0 R", pagesObjNum))
		m.objects[newObjNum] = []byte(dupUpdated)
		newRefs[i] = fmt.Sprintf("%d 0 R", newObjNum)
	}

	// Build new /Kids: insert all copies immediately after pageNumber (0-based index pageNumber).
	insertAfter := pageNumber // 0-based index of the element after which we insert
	newKidsRefs := make([]string, 0, len(kidsRefs)+copies)
	newKidsRefs = append(newKidsRefs, kidsRefs[:insertAfter]...)
	newKidsRefs = append(newKidsRefs, newRefs...)
	newKidsRefs = append(newKidsRefs, kidsRefs[insertAfter:]...)

	newKidsStr := "["
	for i, ref := range newKidsRefs {
		if i > 0 {
			newKidsStr += " "
		}
		newKidsStr += ref
	}
	newKidsStr += " ]"

	updatedPages := setDictValue(pagesStr, "/Kids", newKidsStr)
	m.objects[pagesObjNum] = []byte(updatedPages)

	// Update /Count by +copies.
	if err := m.updatePageCounts(pagesObjNum, copies); err != nil && verbose {
		fmt.Printf("Warning: failed to update page counts: %v\n", err)
	}

	if verbose {
		fmt.Printf("Duplicated page %d (object %d) %d time(s)\n", pageNumber, srcObjNum, copies)
	}

	return m.Rebuild()
}
