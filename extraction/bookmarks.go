package extraction

import (
	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// ExtractBookmarks extracts bookmarks/outlines from a PDF
func ExtractBookmarks(pdfBytes []byte, pdf *parser.PDF, verbose bool) ([]types.Bookmark, error) {
	// Get catalog to find outlines
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return []types.Bookmark{}, nil // No bookmarks
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return []types.Bookmark{}, nil
	}

	catalogObj, err := pdf.GetObject(rootObjNum)
	if err != nil {
		return []types.Bookmark{}, nil
	}

	// Find Outlines reference in catalog
	outlinesRef := extractDictValue(string(catalogObj), "/Outlines")
	if outlinesRef == "" {
		return []types.Bookmark{}, nil
	}

	outlinesObjNum, err := parseObjectRef(outlinesRef)
	if err != nil {
		return []types.Bookmark{}, nil
	}

	outlinesObj, err := pdf.GetObject(outlinesObjNum)
	if err != nil {
		return []types.Bookmark{}, nil
	}

	// Get First child
	firstRef := extractDictValue(string(outlinesObj), "/First")
	if firstRef == "" {
		return []types.Bookmark{}, nil
	}

	// Extract bookmarks recursively
	bookmarks, err := extractBookmarksRecursive(pdf, firstRef, verbose)
	if err != nil {
		return []types.Bookmark{}, nil
	}

	return bookmarks, nil
}

// extractBookmarksRecursive recursively extracts bookmarks from the outline tree
func extractBookmarksRecursive(pdf *parser.PDF, itemRef string, verbose bool) ([]types.Bookmark, error) {
	var bookmarks []types.Bookmark

	itemObjNum, err := parseObjectRef(itemRef)
	if err != nil {
		return bookmarks, err
	}

	itemObj, err := pdf.GetObject(itemObjNum)
	if err != nil {
		return bookmarks, err
	}

	itemStr := string(itemObj)

	// Extract title
	title := extractDictValue(itemStr, "/Title")
	if title != "" {
		title = unescapePDFString(title)
	}

	// Extract destination or action
	dest := extractDictValue(itemStr, "/Dest")
	uri := ""
	if dest == "" {
		// Check for action with URI
		actionRef := extractDictValue(itemStr, "/A")
		if actionRef != "" {
			actionObjNum, err := parseObjectRef(actionRef)
			if err == nil {
				actionObj, err := pdf.GetObject(actionObjNum)
				if err == nil {
					uri = extractDictValue(string(actionObj), "/URI")
					if uri != "" {
						uri = unescapePDFString(uri)
					}
				}
			}
		}
	}

	bookmark := types.Bookmark{
		Title:       title,
		Destination: dest,
		URI:         uri,
		Children:    []types.Bookmark{},
	}

	// Extract children (First/Next chain)
	firstRef := extractDictValue(itemStr, "/First")
	if firstRef != "" {
		children, err := extractBookmarksRecursive(pdf, firstRef, verbose)
		if err == nil {
			bookmark.Children = children
		}
	}

	bookmarks = append(bookmarks, bookmark)

	// Get next sibling
	nextRef := extractDictValue(itemStr, "/Next")
	if nextRef != "" {
		siblings, err := extractBookmarksRecursive(pdf, nextRef, verbose)
		if err == nil {
			bookmarks = append(bookmarks, siblings...)
		}
	}

	return bookmarks, nil
}
