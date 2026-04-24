package manipulate

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/core/write"
)

// ExtractPages extracts specific pages from a PDF and returns a new PDF containing
// only those pages, with all their dependencies (fonts, images, content streams, etc.)
// fully copied and object references remapped. pageNumbers is 1-based.
func ExtractPages(pdfBytes []byte, pageNumbers []int, password []byte, verbose bool) ([]byte, error) {
	if len(pageNumbers) == 0 {
		return nil, fmt.Errorf("no pages specified for extraction")
	}

	manipulator, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to create manipulator: %w", err)
	}

	allPageObjNums, err := manipulator.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("failed to get page objects: %w", err)
	}

	for _, pageNum := range pageNumbers {
		if pageNum < 1 || pageNum > len(allPageObjNums) {
			return nil, fmt.Errorf("page number %d out of range (1-%d)", pageNum, len(allPageObjNums))
		}
	}

	// Build set of selected page object numbers.
	selectedPageObjNums := make(map[int]bool, len(pageNumbers))
	for _, pageNum := range pageNumbers {
		selectedPageObjNums[allPageObjNums[pageNum-1]] = true
	}

	// Collect all objects reachable from the selected pages, excluding the old
	// page tree (we build a fresh one below).
	deps := collectPageDependencies(manipulator.objects, selectedPageObjNums)

	// Assign new sequential object numbers to every dependency.
	objNumMap := make(map[int]int, len(deps))
	nextObjNum := 1
	for objNum := range deps {
		objNumMap[objNum] = nextObjNum
		nextObjNum++
	}

	// Reserve numbers for the new Pages node and Catalog.
	pagesObjNum := nextObjNum
	nextObjNum++
	catalogObjNum := nextObjNum

	// Write all dependency objects with remapped references.
	writer := write.NewPDFWriter()
	for oldObjNum, newObjNum := range objNumMap {
		obj := manipulator.objects[oldObjNum]

		// Update /Parent in page objects to point to the new Pages node.
		if selectedPageObjNums[oldObjNum] {
			objStr := setDictValue(string(obj), "/Parent", fmt.Sprintf("%d 0 R", pagesObjNum))
			obj = []byte(objStr)
		}

		writer.SetObject(newObjNum, updateObjectReferences(obj, objNumMap, false))
	}

	// Build Kids array in the caller-specified order.
	kids := "["
	for i, pageNum := range pageNumbers {
		if i > 0 {
			kids += " "
		}
		kids += fmt.Sprintf("%d 0 R", objNumMap[allPageObjNums[pageNum-1]])
	}
	kids += "]"

	writer.SetObject(pagesObjNum, []byte(fmt.Sprintf(
		"<</Type/Pages/Kids%s/Count %d>>", kids, len(pageNumbers),
	)))
	writer.SetObject(catalogObjNum, []byte(fmt.Sprintf(
		"<</Type/Catalog/Pages %d 0 R>>", pagesObjNum,
	)))
	writer.SetRoot(catalogObjNum)

	return writer.Bytes()
}

// collectPageDependencies returns the set of object numbers that are transitively
// reachable from the selected pages via object references, minus the old page tree
// (Pages nodes and Page objects that are not in the selection).
func collectPageDependencies(objects map[int][]byte, selectedPageObjNums map[int]bool) map[int]bool {
	refPattern := regexp.MustCompile(`(\d+)\s+\d+\s+R`)

	collected := make(map[int]bool, len(objects))
	queue := make([]int, 0, len(selectedPageObjNums))
	for objNum := range selectedPageObjNums {
		queue = append(queue, objNum)
	}

	for len(queue) > 0 {
		objNum := queue[0]
		queue = queue[1:]

		if collected[objNum] {
			continue
		}
		collected[objNum] = true

		objBytes, ok := objects[objNum]
		if !ok {
			continue
		}

		for _, m := range refPattern.FindAllStringSubmatch(string(objBytes), -1) {
			refNum, err := strconv.Atoi(m[1])
			if err == nil && refNum > 0 && !collected[refNum] {
				queue = append(queue, refNum)
			}
		}
	}

	// Remove old page-tree objects: /Pages nodes are replaced entirely, and any
	// /Page objects that are not in the selection are unwanted.
	toRemove := make([]int, 0)
	for objNum := range collected {
		objBytes, ok := objects[objNum]
		if !ok {
			toRemove = append(toRemove, objNum)
			continue
		}
		switch extractDictValue(string(objBytes), "/Type") {
		case "/Pages":
			toRemove = append(toRemove, objNum)
		case "/Page":
			if !selectedPageObjNums[objNum] {
				toRemove = append(toRemove, objNum)
			}
		}
	}
	for _, objNum := range toRemove {
		delete(collected, objNum)
	}

	return collected
}
