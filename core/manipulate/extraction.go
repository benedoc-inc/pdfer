package manipulate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// maxParentChain bounds the /Parent walk when resolving inherited page
// attributes. A well-formed page tree is shallow; a longer chain means a cycle
// got into the file, and spinning on it is worse than losing an inherited key.
const maxParentChain = 64

// inheritableKeys are the page attributes a /Page may inherit from an ancestor
// /Pages node instead of carrying itself (ISO 32000-1 §7.7.3.4). Extraction
// discards the source page tree, so anything a page was inheriting has to be
// copied onto the page itself or it is silently lost — a page whose /MediaBox
// lived on the parent would come out with no size at all.
var inheritableKeys = []string{"/Resources", "/MediaBox", "/CropBox", "/Rotate"}

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

	// Resolve inherited attributes BEFORE the page tree is discarded — the
	// values may live on a /Pages node that is about to be dropped.
	inherited := make(map[int]map[string]string, len(selectedPageObjNums))
	for objNum := range selectedPageObjNums {
		inherited[objNum] = resolveInheritedAttrs(manipulator.objects, objNum)
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

		if selectedPageObjNums[oldObjNum] {
			objStr := setDictValue(string(obj), "/Parent", fmt.Sprintf("%d 0 R", pagesObjNum))
			// Materialize what this page used to inherit.
			for key, value := range inherited[oldObjNum] {
				objStr = setDictValue(objStr, key, value)
			}
			obj = []byte(objStr)
		}

		writer.SetObject(newObjNum, remapExtractedRefs(obj, objNumMap))
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

var refPattern = regexp.MustCompile(`(\d+)\s+\d+\s+R`)

// collectPageDependencies returns the set of object numbers transitively
// reachable from the selected pages, WITHOUT descending through the page tree.
//
// Not descending is the whole point. Every /Page carries /Parent pointing at
// its /Pages node, and that node's /Kids reference every other page in the
// document — so a walk that follows /Parent reaches the entire file, and
// extracting one page of a 500-page document collects all 500 pages' fonts,
// images and content streams. Deleting the page objects afterwards does not
// help: their content is already in the set, orphaned but still written, which
// is why an "extracted" page used to come out the same size as its source.
//
// The filter is therefore applied at ENQUEUE time, by object type: /Pages
// nodes are never entered, and a /Page is entered only if it was selected.
func collectPageDependencies(objects map[int][]byte, selectedPageObjNums map[int]bool) map[int]bool {
	collected := make(map[int]bool, len(objects))
	queue := make([]int, 0, len(selectedPageObjNums))
	for objNum := range selectedPageObjNums {
		if _, ok := objects[objNum]; ok {
			queue = append(queue, objNum)
		}
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
			if err != nil || refNum <= 0 || collected[refNum] {
				continue
			}
			// A reference to an object this file does not contain is dropped
			// rather than queued: it cannot be written, and leaving it in the
			// map would produce a mapping to nothing.
			refBytes, ok := objects[refNum]
			if !ok {
				continue
			}
			if isPageTreeObject(refBytes, refNum, selectedPageObjNums) {
				continue
			}
			queue = append(queue, refNum)
		}
	}

	return collected
}

// isPageTreeObject reports whether obj is part of the source page tree that
// extraction replaces: any /Pages node, or a /Page that was not selected.
func isPageTreeObject(obj []byte, objNum int, selected map[int]bool) bool {
	switch extractDictValue(string(obj), "/Type") {
	case "/Pages":
		return true
	case "/Page":
		return !selected[objNum]
	}
	return false
}

// remapExtractedRefs rewrites object references into the new numbering, and
// rewrites references to objects that were NOT carried over as `null`.
//
// The null part matters: extraction renumbers objects from 1, so a reference
// left at its original number does not dangle harmlessly — it silently points
// at whatever unrelated object now holds that number. A link annotation
// targeting page 40 of the source would resolve, in the extracted file, to a
// font or an image. `null` is the spec's own answer for a reference to a
// non-existent object (ISO 32000-1 §7.3.10), so readers already handle it.
//
// Deliberately separate from updateObjectReferences (merging.go), which leaves
// unmapped references alone: merge assigns numbers for every object up front,
// so an unmapped reference there means something different and this rewrite
// would mask it.
func remapExtractedRefs(obj []byte, objNumMap map[int]int) []byte {
	return []byte(refPattern.ReplaceAllStringFunc(string(obj), func(match string) string {
		var objNum, genNum int
		if _, err := fmt.Sscanf(match, "%d %d R", &objNum, &genNum); err != nil {
			return match
		}
		if newObjNum, ok := objNumMap[objNum]; ok {
			return fmt.Sprintf("%d 0 R", newObjNum)
		}
		return "null"
	}))
}

// resolveInheritedAttrs walks a page's /Parent chain and returns the values of
// inheritable attributes the page does not define itself. Nothing is returned
// for a page that defines everything, which is the common case.
func resolveInheritedAttrs(objects map[int][]byte, pageObjNum int) map[string]string {
	page, ok := objects[pageObjNum]
	if !ok {
		return nil
	}
	missing := make([]string, 0, len(inheritableKeys))
	for _, key := range inheritableKeys {
		if dictValue(string(page), key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	out := make(map[string]string, len(missing))
	seen := map[int]bool{pageObjNum: true}
	current := string(page)
	for i := 0; i < maxParentChain; i++ {
		// A page with no /Parent is the end of the chain — a one-page document
		// whose page hangs directly off the catalog, or a malformed file.
		fields := strings.Fields(extractDictValue(current, "/Parent"))
		if len(fields) == 0 {
			break
		}
		parentNum, err := strconv.Atoi(fields[0])
		if err != nil || seen[parentNum] {
			break
		}
		seen[parentNum] = true
		parent, ok := objects[parentNum]
		if !ok {
			break
		}
		parentStr := string(parent)
		remaining := missing[:0]
		for _, key := range missing {
			if v := dictValue(parentStr, key); v != "" {
				out[key] = v
			} else {
				remaining = append(remaining, key)
			}
		}
		missing = remaining
		if len(missing) == 0 {
			break
		}
		current = parentStr
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dictValue reads a dictionary entry, handling the inline-dictionary form that
// extractDictValue does not: /Resources is frequently written inline
// (`/Resources<</Font<<...>>>>`) rather than as an indirect reference, and
// extractDictValue stops at the `<`, so an inherited inline /Resources would
// otherwise read as absent and be dropped.
func dictValue(dictStr, key string) string {
	if idx := strings.Index(dictStr, key); idx >= 0 {
		i := idx + len(key)
		for i < len(dictStr) && (dictStr[i] == ' ' || dictStr[i] == '\t' || dictStr[i] == '\n' || dictStr[i] == '\r') {
			i++
		}
		if i+1 < len(dictStr) && dictStr[i] == '<' && dictStr[i+1] == '<' {
			if d := extractInlineDict(dictStr, i); d != "" {
				return d
			}
		}
	}
	return extractDictValue(dictStr, key)
}

// extractInlineDict returns the `<<...>>` beginning at start, matched by depth.
func extractInlineDict(s string, start int) string {
	depth := 0
	for i := start; i+1 < len(s); i++ {
		switch {
		case s[i] == '<' && s[i+1] == '<':
			depth++
			i++
		case s[i] == '>' && s[i+1] == '>':
			depth--
			i++
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
