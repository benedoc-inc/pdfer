package manipulate

import (
	"fmt"
	"strings"
)

// BookmarkEntry describes one entry in the PDF outline (bookmark) tree.
type BookmarkEntry struct {
	Title    string          // displayed text
	Page     int             // 1-based destination page (0 = no destination)
	Children []BookmarkEntry // nested entries
}

// SetBookmarks replaces all bookmarks in pdfBytes with the given tree.
// If bookmarks is empty or nil, all existing bookmarks are removed.
func SetBookmarks(pdfBytes []byte, bookmarks []BookmarkEntry, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("set bookmarks: %w", err)
	}

	// Get root catalog object number.
	trailer := m.pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("set bookmarks: no root reference found")
	}
	catalogObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("set bookmarks: parse root ref: %w", err)
	}

	catalogObj, ok := m.objects[catalogObjNum]
	if !ok {
		return nil, fmt.Errorf("set bookmarks: catalog object %d not found", catalogObjNum)
	}
	catalogStr := string(catalogObj)

	// Remove any existing /Outlines reference.
	catalogStr = removeOutlinesFromCatalog(catalogStr)

	if len(bookmarks) == 0 {
		m.objects[catalogObjNum] = []byte(catalogStr)
		return m.Rebuild()
	}

	// Collect page object numbers for destination mapping.
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err != nil {
		return nil, fmt.Errorf("set bookmarks: get pages: %w", err)
	}

	// Pass 1: assign object numbers to all entries depth-first.
	nextObjNum := m.maxObjectNumber() + 1
	// outlines root gets nextObjNum; items are assigned starting from nextObjNum+1.
	outlinesObjNum := nextObjNum
	nextObjNum++

	// assignedNums maps a flat index (assigned in DFS order) to its object number.
	// We build a parallel tree of (entry, assigned objNum) so we can do pass 2.
	type assignedEntry struct {
		entry    BookmarkEntry
		objNum   int
		children []assignedEntry
	}

	var assignNums func(entries []BookmarkEntry) []assignedEntry
	assignNums = func(entries []BookmarkEntry) []assignedEntry {
		result := make([]assignedEntry, len(entries))
		for i, e := range entries {
			result[i].entry = e
			result[i].objNum = nextObjNum
			nextObjNum++
			result[i].children = assignNums(e.Children)
		}
		return result
	}
	assigned := assignNums(bookmarks)

	// Pass 2: write all item dicts.
	// Returns firstObjNum, lastObjNum, total descendant count (for open count).
	var writeItems func(items []assignedEntry, parentObjNum int) (first, last, count int)
	writeItems = func(items []assignedEntry, parentObjNum int) (first, last, count int) {
		if len(items) == 0 {
			return 0, 0, 0
		}
		first = items[0].objNum
		last = items[len(items)-1].objNum

		for i, item := range items {
			var sb strings.Builder
			sb.WriteString("<</Title")
			sb.WriteString(escapePDFLiteralString(item.entry.Title))

			// Destination
			if item.entry.Page > 0 && item.entry.Page <= len(pageObjNums) {
				destPageObjNum := pageObjNums[item.entry.Page-1]
				sb.WriteString(fmt.Sprintf("/Dest[%d 0 R /XYZ null null null]", destPageObjNum))
			}

			// Parent ref
			sb.WriteString(fmt.Sprintf("/Parent %d 0 R", parentObjNum))

			// Prev / Next sibling refs
			if i > 0 {
				sb.WriteString(fmt.Sprintf("/Prev %d 0 R", items[i-1].objNum))
			}
			if i < len(items)-1 {
				sb.WriteString(fmt.Sprintf("/Next %d 0 R", items[i+1].objNum))
			}

			// Children
			if len(item.children) > 0 {
				childFirst, childLast, childCount := writeItems(item.children, item.objNum)
				sb.WriteString(fmt.Sprintf("/First %d 0 R/Last %d 0 R/Count %d",
					childFirst, childLast, childCount))
				count += childCount
			}

			sb.WriteString(">>")
			m.objects[item.objNum] = []byte(sb.String())
			count++
		}
		return first, last, count
	}

	firstObjNum, lastObjNum, totalCount := writeItems(assigned, outlinesObjNum)

	// Write the outlines root dict.
	outlinesDict := fmt.Sprintf("<</Type/Outlines/First %d 0 R/Last %d 0 R/Count %d>>",
		firstObjNum, lastObjNum, totalCount)
	m.objects[outlinesObjNum] = []byte(outlinesDict)

	// Update catalog with new /Outlines ref.
	catalogStr = setDictValue(catalogStr, "/Outlines", fmt.Sprintf("%d 0 R", outlinesObjNum))
	m.objects[catalogObjNum] = []byte(catalogStr)

	if verbose {
		fmt.Printf("SetBookmarks: wrote %d top-level bookmarks (%d total items), outlines object %d\n",
			len(bookmarks), totalCount, outlinesObjNum)
	}

	return m.Rebuild()
}

// removeOutlinesFromCatalog removes the /Outlines key and its value from a catalog dict string.
func removeOutlinesFromCatalog(catalogStr string) string {
	// Match /Outlines followed by an indirect ref like "N G R"
	// e.g. "/Outlines 5 0 R" or "/Outlines5 0 R"
	idx := strings.Index(catalogStr, "/Outlines")
	if idx == -1 {
		return catalogStr
	}

	// Find the end of the value. Values can be:
	//   "N G R"  — indirect ref, ends after R
	//   "<< ... >>" — inline dict (unlikely for Outlines but handle)
	// We look for the indirect ref pattern.
	after := idx + len("/Outlines")
	// skip whitespace
	for after < len(catalogStr) && isWhitespace(catalogStr[after]) {
		after++
	}
	// read until we hit R (end of "N G R") or another / or >>
	end := after
	for end < len(catalogStr) {
		if catalogStr[end] == 'R' && (end+1 >= len(catalogStr) || isWhitespace(catalogStr[end+1]) || catalogStr[end+1] == '/' || catalogStr[end+1] == '>') {
			end++ // include the R
			break
		}
		end++
	}

	return catalogStr[:idx] + catalogStr[end:]
}
