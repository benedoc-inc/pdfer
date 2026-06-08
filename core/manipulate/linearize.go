package manipulate

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// Linearize rewrites a PDF for fast web delivery ("Fast Web View"). It:
//   - flattens all incremental updates into a single revision
//   - reorders objects so the catalog and first page appear early in the file
//   - adds a /Linearized parameter dictionary as the first object
//
// The /H (hint stream) field is omitted; PDF readers fall back to sequential
// reading gracefully when hints are absent. For encrypted PDFs supply the
// password; the output will be unencrypted (linearization requires plaintext).
func Linearize(pdfBytes []byte, password []byte) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password})
	if err != nil {
		return nil, fmt.Errorf("linearize: parse failed: %w", err)
	}

	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("linearize: no document catalog")
	}

	// Locate the catalog and first page.
	catalogObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("linearize: bad RootRef %q: %w", trailer.RootRef, err)
	}
	firstPageObjNum, err := linFirstPageObjNum(pdf, catalogObjNum)
	if err != nil {
		return nil, fmt.Errorf("linearize: cannot find first page: %w", err)
	}
	pageCount := linPageCount(pdf, catalogObjNum)

	// BFS from catalog + first page to collect the "first section" objects.
	firstGroup := linBFSReachable(pdf, []int{catalogObjNum, firstPageObjNum})

	// Build old→new object number mapping.
	// Object 1 is reserved for the Linearized parameter dict.
	// Objects 2..k  → first-page group (deterministic order).
	// Objects k+1..n → remaining objects.
	allObjs := pdf.Objects()
	sort.Ints(allObjs)

	oldToNew := make(map[int]int, len(allObjs)+1)
	nextNum := 2

	firstList := linSortedKeys(firstGroup)
	for _, n := range firstList {
		oldToNew[n] = nextNum
		nextNum++
	}
	for _, n := range allObjs {
		if !firstGroup[n] {
			oldToNew[n] = nextNum
			nextNum++
		}
	}

	// Set up the writer.
	w := write.NewPDFWriter()

	// Object 1: Linearized parameter dictionary with placeholder values.
	// We use distinct placeholder integers that are fixed-width (10 digits) so
	// we can patch them in-place after measuring the output length and xref
	// position. The file size stays constant because replacements are same-width.
	newFirstPageObj := oldToNew[firstPageObjNum]
	linBody := fmt.Sprintf(
		"<</Linearized 1/O %d/N %d/L %d/T %d>>",
		newFirstPageObj, pageCount,
		linPlaceholderL, linPlaceholderT,
	)
	w.SetObject(1, []byte(linBody))

	// Copy and remap every object.
	for _, oldNum := range allObjs {
		newNum, ok := oldToNew[oldNum]
		if !ok {
			continue
		}
		body, err := pdf.GetObjectContent(oldNum)
		if err != nil {
			continue
		}
		remapped := updateObjectReferences(body, oldToNew)
		w.SetObject(newNum, remapped)
	}

	// Set trailer references (remapped).
	w.SetRoot(oldToNew[catalogObjNum])
	if trailer.InfoRef != "" {
		if infoNum, err := parseObjectRef(trailer.InfoRef); err == nil {
			if newNum, ok := oldToNew[infoNum]; ok {
				w.SetInfo(newNum)
			}
		}
	}

	// First write — objects land in numeric order (1, 2, …).
	raw, err := w.Bytes()
	if err != nil {
		return nil, fmt.Errorf("linearize: write failed: %w", err)
	}

	// Patch /L (file length) and /T (main xref offset) in-place.
	xrefOffset := linExtractXRefOffset(raw)
	fileLen := int64(len(raw))

	raw = linPatchPlaceholder(raw, linPlaceholderL, fileLen)
	raw = linPatchPlaceholder(raw, linPlaceholderT, xrefOffset)

	return raw, nil
}

// placeholder values — 10 digits each, chosen to be unique within the file.
const (
	linPlaceholderL int64 = 9000000001
	linPlaceholderT int64 = 9000000002
)

// linPatchPlaceholder replaces the FIRST occurrence of placeholder (as a
// decimal string) with actual, right-aligned to the same character width so
// the file length stays constant.
func linPatchPlaceholder(data []byte, placeholder, actual int64) []byte {
	old := []byte(strconv.FormatInt(placeholder, 10))
	padded := fmt.Sprintf("%*d", len(old), actual) // right-align, same width
	return bytes.Replace(data, old, []byte(padded), 1)
}

// linExtractXRefOffset reads the startxref value from the end of the file.
func linExtractXRefOffset(data []byte) int64 {
	re := regexp.MustCompile(`startxref\s+(\d+)`)
	m := re.FindSubmatch(data)
	if m == nil {
		return 0
	}
	n, _ := strconv.ParseInt(string(m[1]), 10, 64)
	return n
}

// linFirstPageObjNum traverses catalog → Pages tree and returns the object
// number of the very first Page dictionary.
func linFirstPageObjNum(pdf *parse.PDF, catalogObjNum int) (int, error) {
	catalogBody, err := pdf.GetObjectContent(catalogObjNum)
	if err != nil {
		return 0, fmt.Errorf("cannot read catalog: %w", err)
	}
	pagesVal := extractDictValue(string(catalogBody), "/Pages")
	if pagesVal == "" {
		return 0, fmt.Errorf("catalog has no /Pages")
	}
	pagesNum, err := parseObjectRef(pagesVal)
	if err != nil {
		return 0, fmt.Errorf("bad /Pages ref %q: %w", pagesVal, err)
	}
	return linFirstKid(pdf, pagesNum)
}

// linFirstKid recurses through the Pages tree and returns the first leaf Page.
func linFirstKid(pdf *parse.PDF, pagesObjNum int) (int, error) {
	body, err := pdf.GetObjectContent(pagesObjNum)
	if err != nil {
		return 0, err
	}
	s := string(body)
	kidsVal := extractDictValue(s, "/Kids")
	if kidsVal == "" {
		// No /Kids — this node itself may be a Page.
		if strings.Contains(s, "/Type/Page") || strings.Contains(s, "/Type /Page") {
			return pagesObjNum, nil
		}
		return 0, fmt.Errorf("object %d has no /Kids", pagesObjNum)
	}
	re := regexp.MustCompile(`(\d+)\s+0\s+R`)
	m := re.FindStringSubmatch(kidsVal)
	if m == nil {
		return 0, fmt.Errorf("no references found in /Kids")
	}
	kidNum, _ := strconv.Atoi(m[1])
	kidBody, err := pdf.GetObjectContent(kidNum)
	if err != nil {
		return 0, err
	}
	kidStr := string(kidBody)
	if strings.Contains(kidStr, "/Type/Pages") || strings.Contains(kidStr, "/Type /Pages") {
		return linFirstKid(pdf, kidNum)
	}
	return kidNum, nil
}

// linPageCount reads /Count from the Pages tree root.
func linPageCount(pdf *parse.PDF, catalogObjNum int) int {
	catalogBody, _ := pdf.GetObjectContent(catalogObjNum)
	pagesVal := extractDictValue(string(catalogBody), "/Pages")
	pagesNum, err := parseObjectRef(pagesVal)
	if err != nil {
		return 0
	}
	pagesBody, _ := pdf.GetObjectContent(pagesNum)
	countStr := extractDictValue(string(pagesBody), "/Count")
	n, _ := strconv.Atoi(countStr)
	return n
}

// linBFSReachable performs a breadth-first search from startNums through all
// object references found in object bodies, returning the set of reachable
// object numbers.
func linBFSReachable(pdf *parse.PDF, startNums []int) map[int]bool {
	visited := make(map[int]bool, 32)
	queue := append([]int(nil), startNums...)
	refRE := regexp.MustCompile(`(\d+)\s+0\s+R`)

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if visited[n] {
			continue
		}
		visited[n] = true

		body, err := pdf.GetObjectContent(n)
		if err != nil {
			continue
		}
		for _, m := range refRE.FindAllSubmatch(body, -1) {
			ref, _ := strconv.Atoi(string(m[1]))
			if !visited[ref] && pdf.HasObject(ref) {
				queue = append(queue, ref)
			}
		}
	}
	return visited
}

// linSortedKeys returns the keys of a bool map in ascending order.
func linSortedKeys(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
