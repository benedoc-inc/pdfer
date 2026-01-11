package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// ExtractPages extracts all pages from a PDF
func ExtractPages(pdfBytes []byte, pdf *parse.PDF, verbose bool) ([]types.Page, error) {
	var pages []types.Page

	// Get catalog to find pages tree
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("no root reference found in trailer")
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root reference: %w", err)
	}

	catalogObj, err := pdf.GetObject(rootObjNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog object: %w", err)
	}

	// Find Pages reference in catalog
	pagesRef := extractDictValue(string(catalogObj), "/Pages")
	if pagesRef == "" {
		return nil, fmt.Errorf("no /Pages reference found in catalog")
	}

	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Pages reference: %w", err)
	}

	// Extract pages from pages tree
	pages, err = extractPagesFromTree(pdfBytes, pdf, pagesObjNum, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract pages from tree: %w", err)
	}

	// Update page numbers
	for i := range pages {
		pages[i].PageNumber = i + 1
	}

	return pages, nil
}

// extractPagesFromTree recursively extracts pages from a pages tree
func extractPagesFromTree(pdfBytes []byte, pdf *parse.PDF, pagesObjNum int, verbose bool) ([]types.Page, error) {
	var result []types.Page

	pagesObj, err := pdf.GetObject(pagesObjNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get pages object %d: %w", pagesObjNum, err)
	}

	pagesStr := string(pagesObj)

	// Check if this is a page or a pages node
	pageType := extractDictValue(pagesStr, "/Type")
	if pageType == "/Page" {
		// This is a page object
		page, err := extractPage(pdfBytes, pdf, pagesObjNum, pagesStr, verbose)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to extract page %d: %v\n", pagesObjNum, err)
			}
			return result, nil
		}
		return []types.Page{page}, nil
	}

	// This is a pages node - get Kids array
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return result, nil
	}

	// Parse Kids array (e.g., "[5 0 R 6 0 R 7 0 R]")
	kids := parseObjectRefArray(kidsStr)
	for _, kidRef := range kids {
		kidObjNum, err := parseObjectRef(kidRef)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to parse kid reference %s: %v\n", kidRef, err)
			}
			continue
		}

		// Recursively extract from child
		childPages, err := extractPagesFromTree(pdfBytes, pdf, kidObjNum, verbose)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to extract from child %d: %v\n", kidObjNum, err)
			}
			continue
		}

		result = append(result, childPages...)
	}

	return result, nil
}

// extractPage extracts a single page
func extractPage(pdfBytes []byte, pdf *parse.PDF, pageObjNum int, pageStr string, verbose bool) (types.Page, error) {
	page := types.Page{
		PageNumber:  0, // Will be set by caller
		Text:        []types.TextElement{},
		Graphics:    []types.Graphic{},
		Images:      []types.ImageRef{},
		Annotations: []types.Annotation{},
	}

	// Extract media box
	mediaBox := extractArrayValue(pageStr, "/MediaBox")
	if mediaBox != nil && len(mediaBox) >= 4 {
		page.MediaBox = &types.Rectangle{
			LowerX: mediaBox[0],
			LowerY: mediaBox[1],
			UpperX: mediaBox[2],
			UpperY: mediaBox[3],
		}
		page.Width = mediaBox[2] - mediaBox[0]
		page.Height = mediaBox[3] - mediaBox[1]
	}

	// Extract crop box
	cropBox := extractArrayValue(pageStr, "/CropBox")
	if cropBox != nil && len(cropBox) >= 4 {
		page.CropBox = &types.Rectangle{
			LowerX: cropBox[0],
			LowerY: cropBox[1],
			UpperX: cropBox[2],
			UpperY: cropBox[3],
		}
	}

	// Extract rotation
	rotationStr := extractDictValue(pageStr, "/Rotate")
	if rotationStr != "" {
		if rot, err := strconv.Atoi(rotationStr); err == nil {
			page.Rotation = rot
		}
	}

	// Extract content streams
	contents := extractDictValue(pageStr, "/Contents")
	if contents != "" {
		// Contents can be a single reference or an array
		contentRefs := parseObjectRefArray(contents)
		if len(contentRefs) == 0 {
			// Try as single reference
			contentRefs = []string{contents}
		}

		// Extract text and graphics from content streams
		for _, contentRef := range contentRefs {
			contentObjNum, err := parseObjectRef(contentRef)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: failed to parse content reference %s: %v\n", contentRef, err)
				}
				continue
			}

			contentObj, err := pdf.GetObject(contentObjNum)
			if err != nil {
				if verbose {
					fmt.Printf("Warning: failed to get content object %d: %v\n", contentObjNum, err)
				}
				continue
			}

			// Parse content stream
			textElements, graphics, images := parseContentStream(string(contentObj), pdf, pageObjNum, verbose)
			page.Text = append(page.Text, textElements...)
			page.Graphics = append(page.Graphics, graphics...)
			page.Images = append(page.Images, images...)
		}
	}

	// Extract resources
	resourcesRef := extractDictValue(pageStr, "/Resources")
	if resourcesRef != "" {
		resourcesObjNum, err := parseObjectRef(resourcesRef)
		if err == nil {
			resourcesObj, err := pdf.GetObject(resourcesObjNum)
			if err == nil {
				page.Resources = extractResources(string(resourcesObj), pdf, verbose)
			}
		}
	} else {
		// Resources might be inline
		if strings.Contains(pageStr, "/Resources") {
			page.Resources = extractResources(pageStr, pdf, verbose)
		}
	}

	// Extract annotations
	annotsRef := extractDictValue(pageStr, "/Annots")
	if annotsRef != "" {
		annotations := extractAnnotations(annotsRef, pdf, pageObjNum, verbose)
		page.Annotations = annotations
	}

	return page, nil
}

// Helper functions for parsing PDF dictionaries and arrays

func extractDictValue(dictStr, key string) string {
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+([^\s]+)`)
	match := pattern.FindStringSubmatch(dictStr)
	if match != nil && len(match) > 1 {
		return match[1]
	}
	return ""
}

func extractArrayValue(dictStr, key string) []float64 {
	// Find the array after the key (e.g., "/MediaBox [0 0 612 792]")
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*\[\s*([^\]]+)\]`)
	match := pattern.FindStringSubmatch(dictStr)
	if match == nil || len(match) < 2 {
		return nil
	}

	// Parse numbers from array
	parts := strings.Fields(match[1])
	var values []float64
	for _, part := range parts {
		if val, err := strconv.ParseFloat(part, 64); err == nil {
			values = append(values, val)
		}
	}
	return values
}

func parseObjectRefArray(arrStr string) []string {
	// Remove brackets if present
	arrStr = strings.TrimSpace(arrStr)
	if strings.HasPrefix(arrStr, "[") {
		arrStr = arrStr[1:]
	}
	if strings.HasSuffix(arrStr, "]") {
		arrStr = arrStr[:len(arrStr)-1]
	}

	// Split by references (pattern: "5 0 R")
	pattern := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	matches := pattern.FindAllStringSubmatch(arrStr, -1)

	var refs []string
	for _, match := range matches {
		if len(match) > 0 {
			refs = append(refs, match[0])
		}
	}

	return refs
}

// parseContentStream, extractResources, extractAnnotations will be implemented in separate files
// These are stubs for now
func parseContentStream(contentStr string, pdf *parse.PDF, pageNum int, verbose bool) ([]types.TextElement, []types.Graphic, []types.ImageRef) {
	// Will be implemented in content_stream.go
	return []types.TextElement{}, []types.Graphic{}, []types.ImageRef{}
}

func extractResources(resourcesStr string, pdf *parse.PDF, verbose bool) *types.PageResources {
	// Will be implemented in resources.go
	return &types.PageResources{
		Fonts:       make(map[string]types.FontInfo),
		Images:      make(map[string]types.Image),
		XObjects:    make(map[string]types.XObject),
		ColorSpaces: make(map[string]string),
		Patterns:    make(map[string]interface{}),
		Shadings:    make(map[string]interface{}),
	}
}

func extractAnnotations(annotsStr string, pdf *parse.PDF, pageNum int, verbose bool) []types.Annotation {
	// Will be implemented in annotations.go
	return []types.Annotation{}
}
