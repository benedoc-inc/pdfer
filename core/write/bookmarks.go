package write

import (
	"fmt"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// SetBookmarks sets the document bookmarks/outlines
// bookmarks is a slice of top-level bookmarks (can have children)
// pageObjNums is a map from page number (1-based) to page object number
// Returns the outlines object number
func (w *PDFWriter) SetBookmarks(bookmarks []types.Bookmark, pageObjNums map[int]int) (int, error) {
	if len(bookmarks) == 0 {
		return 0, nil // No bookmarks
	}

	// Create outline items recursively
	firstItemNum, lastItemNum, count, err := w.createOutlineItems(bookmarks, pageObjNums, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to create outline items: %w", err)
	}

	// Create Outlines dictionary
	outlinesDict := Dictionary{
		"/Type":  "/Outlines",
		"/First": fmt.Sprintf("%d 0 R", firstItemNum),
		"/Last":  fmt.Sprintf("%d 0 R", lastItemNum),
		"/Count": count,
	}

	outlinesObjNum := w.AddObject(w.formatDictionary(outlinesDict))

	// Store outlines reference for catalog update
	w.outlinesRef = fmt.Sprintf("%d 0 R", outlinesObjNum)

	return outlinesObjNum, nil
}

// createOutlineItems creates outline item objects recursively
// Returns: firstItemNum, lastItemNum, count (total visible items including descendants)
func (w *PDFWriter) createOutlineItems(bookmarks []types.Bookmark, pageObjNums map[int]int, parentNum int) (int, int, int, error) {
	if len(bookmarks) == 0 {
		return 0, 0, 0, nil
	}

	var firstItemNum, lastItemNum int
	totalCount := 0

	for i, bookmark := range bookmarks {
		// Create destination string
		dest := ""
		if bookmark.PageNumber > 0 {
			pageObjNum, ok := pageObjNums[bookmark.PageNumber]
			if ok {
				// Create destination array: [pageObjNum /XYZ left top zoom]
				// Default: top of page, fit to page width
				dest = fmt.Sprintf("[%d 0 R /XYZ 0 792 null]", pageObjNum)
			}
		} else if bookmark.Destination != "" {
			// Use provided destination string (should be in format like "[pageObjNum /XYZ ...]")
			dest = bookmark.Destination
		}

		// Create action for URI if provided
		actionRef := ""
		if bookmark.URI != "" {
			actionDict := Dictionary{
				"/Type": "/Action",
				"/S":    "/URI",
				"/URI":  fmt.Sprintf("(%s)", escapePDFString(bookmark.URI)),
			}
			actionObjNum := w.AddObject(w.formatDictionary(actionDict))
			actionRef = fmt.Sprintf("%d 0 R", actionObjNum)
		}

		// Create children first (they need to know parent object number)
		// Reserve parent object number now
		itemObjNum := w.nextObjNum
		w.nextObjNum++ // Reserve it

		// Create children if any (they will use object numbers after the reserved one)
		var firstChildNum, lastChildNum int
		childCount := 0
		if len(bookmark.Children) > 0 {
			var err error
			firstChildNum, lastChildNum, childCount, err = w.createOutlineItems(bookmark.Children, pageObjNums, itemObjNum)
			if err != nil {
				return 0, 0, 0, fmt.Errorf("failed to create child items: %w", err)
			}
		}

		// Build outline item dictionary
		itemDict := Dictionary{
			"/Title": fmt.Sprintf("(%s)", escapePDFString(bookmark.Title)),
		}

		if dest != "" {
			itemDict["/Dest"] = dest
		}
		if actionRef != "" {
			itemDict["/A"] = actionRef
		}

		// Set parent reference
		if parentNum > 0 {
			itemDict["/Parent"] = fmt.Sprintf("%d 0 R", parentNum)
		}

		// Set sibling references
		if i > 0 {
			itemDict["/Prev"] = fmt.Sprintf("%d 0 R", lastItemNum)
		}
		if i < len(bookmarks)-1 {
			// Will set /Next after creating next item
		}

		// Set children references
		if len(bookmark.Children) > 0 {
			itemDict["/First"] = fmt.Sprintf("%d 0 R", firstChildNum)
			itemDict["/Last"] = fmt.Sprintf("%d 0 R", lastChildNum)
			// Count is positive if open, negative if closed
			// Default to open (positive)
			itemDict["/Count"] = childCount
		}

		// Create the outline item object (using reserved number)
		w.objects[itemObjNum] = &PDFObject{
			Number:     itemObjNum,
			Generation: 0,
			Content:    w.formatDictionary(itemDict),
		}
		// Object number already reserved above

		// Update previous item's /Next reference
		if i > 0 {
			prevObj := w.objects[lastItemNum]
			if prevObj != nil {
				prevDictStr := string(prevObj.Content)
				// Add /Next to previous item
				updatedDict := addDictEntry(prevDictStr, "/Next", fmt.Sprintf("%d 0 R", itemObjNum))
				w.objects[lastItemNum].Content = []byte(updatedDict)
			}
		}

		// Track first and last
		if i == 0 {
			firstItemNum = itemObjNum
		}
		lastItemNum = itemObjNum

		// Count this item plus its descendants
		totalCount++
		if childCount > 0 {
			totalCount += childCount
		}
	}

	return firstItemNum, lastItemNum, totalCount, nil
}

// addDictEntry adds or updates a dictionary entry
func addDictEntry(dictStr, key, value string) string {
	// Simple approach: append before closing >>
	// Find last >>
	lastIdx := strings.LastIndex(dictStr, ">>")
	if lastIdx == -1 {
		return dictStr
	}

	// Check if key already exists
	if strings.Contains(dictStr, key+" ") || strings.Contains(dictStr, key+"/") {
		// Replace existing entry (simplified - just append new one, PDF readers will use last)
		// For proper implementation, would need to parse and rebuild
		return dictStr[:lastIdx] + fmt.Sprintf("%s %s ", key, value) + dictStr[lastIdx:]
	}

	// Add new entry
	return dictStr[:lastIdx] + fmt.Sprintf("%s %s ", key, value) + dictStr[lastIdx:]
}
