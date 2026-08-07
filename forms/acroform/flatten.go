package acroform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// FlattenForm converts all filled AcroForm widget annotations to static page
// content. For each widget that has a normal appearance stream (/AP/N):
//   - The appearance is placed on the page as a Form XObject reference.
//   - The page /Resources /XObject dict is updated to include the AP stream.
//   - Widget annotations are removed from /Annots.
//
// After flattening, the PDF has no interactive form fields — /AcroForm is
// removed from the catalog. Unfilled fields (no AP stream) are silently
// dropped; they leave no visual trace.
//
// Both inline and indirect /Resources dicts are handled.
func FlattenForm(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password})
	if err != nil {
		return nil, fmt.Errorf("flatten: parse failed: %w", err)
	}

	// Load the content body of every object (dict for plain objects, dict+stream
	// for stream objects). This is what PDFWriter.SetObject expects.
	contents := make(map[int][]byte)
	maxNum := 0
	for _, n := range pdf.Objects() {
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		// Track the true max object number even for skipped containers so a
		// dropped container's number is never reused when allocating new objects.
		if n > maxNum {
			maxNum = n
		}
		if parse.IsCrossRefContainerObject(body) {
			continue // xref/objstm containers; the writer regenerates them
		}
		contents[n] = body
	}

	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("flatten: no document catalog")
	}
	var rootNum int
	fmt.Sscanf(trailer.RootRef, "%d", &rootNum)

	// If there is no /AcroForm, the PDF is already flat.
	catalogStr := string(contents[rootNum])
	if !strings.Contains(catalogStr, "/AcroForm") {
		return pdfBytes, nil
	}

	pageNums, err := flattenGetPageNums(contents, rootNum)
	if err != nil {
		return nil, fmt.Errorf("flatten: %w", err)
	}

	nextObjNum := maxNum + 1

	for _, pageNum := range pageNums {
		pageDict := string(contents[pageNum])
		annotNums := flattenGetAnnotNums(pageDict)

		type widgetAP struct {
			rect     [4]float64
			apObjNum int
			bbox     [4]float64
		}
		var widgets []widgetAP

		for _, annotNum := range annotNums {
			annotBody, ok := contents[annotNum]
			if !ok {
				continue
			}
			annotDict := string(annotBody)
			if !strings.Contains(annotDict, "/Widget") {
				continue
			}
			rect := flattenParseRect(annotDict)
			apNum := flattenGetAPNObjNum(annotDict)
			if apNum <= 0 {
				continue
			}
			apContent, ok := contents[apNum]
			if !ok {
				continue
			}
			bbox := flattenParseBBox(string(apContent))
			bw := bbox[2] - bbox[0]
			bh := bbox[3] - bbox[1]
			if bw <= 0 || bh <= 0 {
				continue
			}
			widgets = append(widgets, widgetAP{rect, apNum, bbox})
		}

		pageDict = flattenClearAnnots(pageDict)

		if len(widgets) == 0 {
			contents[pageNum] = []byte(pageDict)
			continue
		}

		// Build an overlay content stream that places each AP XObject at the
		// widget's position using a cm + Do pair.
		var overlay strings.Builder
		xobjs := make(map[string]int)

		for _, w := range widgets {
			bw := w.bbox[2] - w.bbox[0]
			bh := w.bbox[3] - w.bbox[1]
			rw := w.rect[2] - w.rect[0]
			rh := w.rect[3] - w.rect[1]
			sx := rw / bw
			sy := rh / bh
			tx := w.rect[0] - sx*w.bbox[0]
			ty := w.rect[1] - sy*w.bbox[1]

			alias := fmt.Sprintf("FO%d", w.apObjNum)
			xobjs[alias] = w.apObjNum
			fmt.Fprintf(&overlay, "q\n%s 0 0 %s %s %s cm\n/%s Do\nQ\n",
				flattenFmtF(sx), flattenFmtF(sy), flattenFmtF(tx), flattenFmtF(ty), alias)
		}

		if overlay.Len() == 0 {
			contents[pageNum] = []byte(pageDict)
			continue
		}

		// Store the overlay as a new stream object.
		overlayStr := overlay.String()
		overlayNum := nextObjNum
		nextObjNum++
		contents[overlayNum] = []byte(fmt.Sprintf(
			"<</Length %d>>\nstream\n%sendstream", len(overlayStr), overlayStr))

		pageDict = flattenAppendContents(pageDict, overlayNum)

		// If /Resources is an indirect reference, update that object directly;
		// otherwise inject into the inline /Resources dict in the page dict.
		if resObjNum := flattenIndirectResourcesObjNum(pageDict); resObjNum > 0 {
			if resBody, ok := contents[resObjNum]; ok {
				contents[resObjNum] = []byte(flattenAddXObjectsToDict(string(resBody), xobjs))
			}
		} else {
			pageDict = flattenAddXObjects(pageDict, xobjs)
		}
		contents[pageNum] = []byte(pageDict)
	}

	// Remove /AcroForm from catalog.
	catStr := string(contents[rootNum])
	catStr = regexp.MustCompile(`/AcroForm\s+\d+\s+\d+\s+R`).ReplaceAllString(catStr, "")
	contents[rootNum] = []byte(catStr)

	w := write.NewPDFWriter()
	for n, body := range contents {
		w.SetObject(n, body)
	}
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d", &infoNum)
		w.SetInfo(infoNum)
	}
	return w.Bytes()
}

// flattenFmtF formats a float64 for content stream use, dropping redundant zeros.
func flattenFmtF(f float64) string {
	s := strconv.FormatFloat(f, 'f', 6, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	return s
}

func flattenGetPageNums(contents map[int][]byte, rootNum int) ([]int, error) {
	pagesRE := regexp.MustCompile(`/Pages\s+(\d+)\s+\d+\s+R`)
	m := pagesRE.FindStringSubmatch(string(contents[rootNum]))
	if m == nil {
		return nil, fmt.Errorf("no /Pages reference in catalog")
	}
	pagesNum, _ := strconv.Atoi(m[1])
	return flattenWalkTree(contents, pagesNum), nil
}

func flattenWalkTree(contents map[int][]byte, num int) []int {
	s := string(contents[num])
	// Match /Type/Page but NOT /Type/Pages (both have /Type/Page as a prefix).
	if (strings.Contains(s, "/Type/Page") || strings.Contains(s, "/Type /Page")) &&
		!strings.Contains(s, "/Type/Pages") && !strings.Contains(s, "/Type /Pages") {
		return []int{num}
	}
	kidsRE := regexp.MustCompile(`/Kids\s*\[([^\]]*)\]`)
	km := kidsRE.FindStringSubmatch(s)
	if km == nil {
		return nil
	}
	refRE := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	var pages []int
	for _, r := range refRE.FindAllStringSubmatch(km[1], -1) {
		n, _ := strconv.Atoi(r[1])
		pages = append(pages, flattenWalkTree(contents, n)...)
	}
	return pages
}

func flattenGetAnnotNums(pageDict string) []int {
	annotsRE := regexp.MustCompile(`/Annots\s*\[([^\]]*)\]`)
	m := annotsRE.FindStringSubmatch(pageDict)
	if m == nil {
		return nil
	}
	refRE := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	nums := make([]int, 0)
	for _, r := range refRE.FindAllStringSubmatch(m[1], -1) {
		n, _ := strconv.Atoi(r[1])
		nums = append(nums, n)
	}
	return nums
}

// flattenGetAPNObjNum returns the object number of the widget's /AP/N appearance
// stream, or 0 if there is no renderable appearance.
//
// Two forms are handled:
//   - Direct: /AP<</N N G R>> — used by text/choice fields.
//   - Sub-dict: /AP<</N<</State N G R/Off M G R>>>> with /AS /State — used by
//     checkboxes and radio buttons. "Off" state returns 0 (nothing to draw).
func flattenGetAPNObjNum(widgetDict string) int {
	// Case 1: /AP<</N N G R [/D ...]>>
	directRE := regexp.MustCompile(`/AP\s*<<[^<>]*/N\s+(\d+)\s+\d+\s+R`)
	if m := directRE.FindStringSubmatch(widgetDict); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	// Case 2: /AP<</N<</State N G R/Off M G R>>>>
	asRE := regexp.MustCompile(`/AS\s*/([^/<>\s]+)`)
	asM := asRE.FindStringSubmatch(widgetDict)
	if asM == nil {
		return 0
	}
	state := asM[1]
	if state == "Off" || state == "" {
		return 0
	}
	apNRE := regexp.MustCompile(`/N\s*<<([^<>]*)>>`)
	apNM := apNRE.FindStringSubmatch(widgetDict)
	if apNM == nil {
		return 0
	}
	stateRE := regexp.MustCompile(`/` + regexp.QuoteMeta(state) + `\s+(\d+)\s+\d+\s+R`)
	stateM := stateRE.FindStringSubmatch(apNM[1])
	if stateM == nil {
		return 0
	}
	n, _ := strconv.Atoi(stateM[1])
	return n
}

func flattenParseRect(widgetDict string) [4]float64 {
	rectRE := regexp.MustCompile(`/Rect\s*\[\s*([^\]]*)\]`)
	m := rectRE.FindStringSubmatch(widgetDict)
	if m == nil {
		return [4]float64{}
	}
	nums := strings.Fields(m[1])
	var rect [4]float64
	for i, s := range nums {
		if i >= 4 {
			break
		}
		rect[i], _ = strconv.ParseFloat(s, 64)
	}
	return rect
}

func flattenParseBBox(xobjContent string) [4]float64 {
	bboxRE := regexp.MustCompile(`/BBox\s*\[\s*([^\]]*)\]`)
	m := bboxRE.FindStringSubmatch(xobjContent)
	if m == nil {
		return [4]float64{0, 0, 1, 1}
	}
	nums := strings.Fields(m[1])
	var bbox [4]float64
	for i, s := range nums {
		if i >= 4 {
			break
		}
		bbox[i], _ = strconv.ParseFloat(s, 64)
	}
	return bbox
}

func flattenClearAnnots(pageDict string) string {
	return regexp.MustCompile(`/Annots\s*\[[^\]]*\]`).ReplaceAllString(pageDict, "")
}

func flattenAppendContents(pageDict string, newObjNum int) string {
	contentsRE := regexp.MustCompile(`/Contents\s*(\[[^\]]*\]|\d+\s+\d+\s+R)`)
	m := contentsRE.FindStringSubmatch(pageDict)
	if m == nil {
		return pageDict
	}
	existing := strings.TrimSpace(m[1])
	var newContents string
	if strings.HasPrefix(existing, "[") {
		inner := strings.TrimSpace(existing[1 : len(existing)-1])
		newContents = fmt.Sprintf("[%s %d 0 R]", inner, newObjNum)
	} else {
		newContents = fmt.Sprintf("[%s %d 0 R]", existing, newObjNum)
	}
	return strings.Replace(pageDict, m[0], "/Contents "+newContents, 1)
}

// flattenIndirectResourcesObjNum returns the object number if the page dict's
// /Resources entry is an indirect reference (e.g. "/Resources 10 0 R"), or 0
// if /Resources is an inline dict or absent.
func flattenIndirectResourcesObjNum(pageDict string) int {
	re := regexp.MustCompile(`/Resources\s+(\d+)\s+\d+\s+R`)
	m := re.FindStringSubmatch(pageDict)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// findDictEnd returns the index of the first byte of the closing ">>" that
// matches the opening "<<" at position start in s.  It counts nested "<<"/">> "
// pairs so that embedded sub-dicts are skipped correctly.  Returns -1 if no
// matching ">>" is found.
func findDictEnd(s string, start int) int {
	depth := 0
	i := start
	for i < len(s)-1 {
		if s[i] == '<' && s[i+1] == '<' {
			depth++
			i += 2
		} else if s[i] == '>' && s[i+1] == '>' {
			depth--
			if depth == 0 {
				return i
			}
			i += 2
		} else {
			i++
		}
	}
	return -1
}

// flattenAddXObjectsToDict injects XObject resource entries into a standalone
// resources dict body (the raw << … >> bytes of the /Resources object, not a
// full page dict).  It mirrors the behaviour of flattenAddXObjects but operates
// on the resources dict directly.
func flattenAddXObjectsToDict(resStr string, xobjs map[string]int) string {
	if len(xobjs) == 0 {
		return resStr
	}
	var entries strings.Builder
	for alias, num := range xobjs {
		fmt.Fprintf(&entries, "/%s %d 0 R", alias, num)
	}
	newEntries := entries.String()

	// Inject into existing /XObject if present.
	if xobjIdx := strings.Index(resStr, "/XObject"); xobjIdx != -1 {
		// Find the opening << of the /XObject value.
		openIdx := strings.Index(resStr[xobjIdx:], "<<")
		if openIdx != -1 {
			openIdx += xobjIdx
			closeIdx := findDictEnd(resStr, openIdx)
			if closeIdx != -1 {
				return resStr[:closeIdx] + newEntries + resStr[closeIdx:]
			}
		}
	}

	// No /XObject yet: insert before the outermost closing >> of the dict.
	depth := 0
	for i := 0; i < len(resStr)-1; i++ {
		if resStr[i] == '<' && resStr[i+1] == '<' {
			depth++
			i++
		} else if resStr[i] == '>' && resStr[i+1] == '>' {
			depth--
			if depth == 0 {
				return resStr[:i] + "/XObject<<" + newEntries + ">>" + resStr[i:]
			}
			i++
		}
	}
	return resStr
}

// flattenAddXObjects injects XObject resource entries into the page dict's
// /Resources /XObject sub-dict, creating the sub-dict if it does not exist.
// Call flattenAddXObjectsToDict instead when /Resources is an indirect object.
func flattenAddXObjects(pageDict string, xobjs map[string]int) string {
	if len(xobjs) == 0 {
		return pageDict
	}
	var entries strings.Builder
	for alias, num := range xobjs {
		fmt.Fprintf(&entries, "/%s %d 0 R", alias, num)
	}
	newEntries := entries.String()

	// If /XObject already exists, inject into it.
	xobjRE := regexp.MustCompile(`/XObject\s*<<([^<>]*)>>`)
	if xobjRE.MatchString(pageDict) {
		return xobjRE.ReplaceAllStringFunc(pageDict, func(match string) string {
			idx := strings.LastIndex(match, ">>")
			if idx == -1 {
				return match
			}
			return match[:idx] + newEntries + ">>"
		})
	}

	// No existing /XObject: find the end of the /Resources dict and inject before it.
	resIdx := strings.Index(pageDict, "/Resources")
	if resIdx == -1 {
		return pageDict
	}
	ddStart := strings.Index(pageDict[resIdx:], "<<")
	if ddStart == -1 {
		return pageDict
	}
	i := resIdx + ddStart + 2
	depth := 1
	for i < len(pageDict)-1 && depth > 0 {
		if pageDict[i] == '<' && pageDict[i+1] == '<' {
			depth++
			i += 2
		} else if pageDict[i] == '>' && pageDict[i+1] == '>' {
			depth--
			if depth == 0 {
				return pageDict[:i] + "/XObject<<" + newEntries + ">>" + pageDict[i:]
			}
			i += 2
		} else {
			i++
		}
	}
	return pageDict
}
