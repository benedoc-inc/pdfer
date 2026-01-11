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
	catalogStr := string(catalogObj)
	if verbose {
		fmt.Printf("Catalog object: %s\n", catalogStr)
	}
	pagesRef := extractDictValue(catalogStr, "/Pages")
	if pagesRef == "" {
		return nil, fmt.Errorf("no /Pages reference found in catalog: %s", catalogStr)
	}

	if verbose {
		fmt.Printf("Found Pages reference: %s\n", pagesRef)
	}
	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Pages reference %s: %w", pagesRef, err)
	}

	if verbose {
		fmt.Printf("Pages object number: %d\n", pagesObjNum)
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
	if verbose {
		fmt.Printf("Pages object %d: %s\n", pagesObjNum, pagesStr)
	}

	// Check if this is a page or a pages node
	// Look for /Type/Page or /Type/Pages in the dictionary
	// Need to check for exact match, not substring (e.g., /Type/Page should not match /Type/Pages)
	pageType := ""
	if strings.Contains(pagesStr, "/Type/Pages") || strings.Contains(pagesStr, "/Type /Pages") {
		pageType = "/Pages"
	} else if strings.Contains(pagesStr, "/Type/Page") || strings.Contains(pagesStr, "/Type /Page") {
		// Make sure it's not /Type/Pages
		if !strings.Contains(pagesStr, "/Type/Pages") && !strings.Contains(pagesStr, "/Type /Pages") {
			pageType = "/Page"
		}
	}
	if verbose {
		fmt.Printf("Type: %s (from string check)\n", pageType)
	}
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
	if verbose {
		fmt.Printf("Kids string: %s\n", kidsStr)
	}
	if kidsStr == "" {
		if verbose {
			fmt.Printf("No Kids array found in pages object\n")
		}
		return result, nil
	}

	// Parse Kids array (e.g., "[5 0 R 6 0 R 7 0 R]" or "5 0 R 6 0 R")
	// If kidsStr already starts with [, use it as-is, otherwise wrap it
	if !strings.HasPrefix(kidsStr, "[") {
		kidsStr = "[" + kidsStr + "]"
	}
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
	if len(mediaBox) >= 4 {
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
	if len(cropBox) >= 4 {
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
		// Decompress and parse content stream
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
			// Content streams are usually stored as stream objects with FlateDecode
			// We need to get the actual stream data, not the object wrapper
			contentStr := string(contentObj)

			// Check if this is a stream object and extract/decompress the stream data
			if strings.Contains(contentStr, "stream") {
				streamIdx := strings.Index(contentStr, "stream")
				if streamIdx != -1 {
					// Check for FlateDecode filter
					isCompressed := strings.Contains(contentStr, "/FlateDecode")

					// Extract stream data
					dataStart := streamIdx + 6 // Skip "stream"
					// Skip EOL after "stream"
					if dataStart < len(contentStr) && (contentStr[dataStart] == '\r' || contentStr[dataStart] == '\n') {
						dataStart++
					}
					if dataStart < len(contentStr) && contentStr[dataStart] == '\n' {
						dataStart++
					}

					endstreamIdx := strings.Index(contentStr[dataStart:], "endstream")
					if endstreamIdx != -1 {
						streamData := []byte(contentStr[dataStart : dataStart+endstreamIdx])

						// Decompress if needed using parse package
						if isCompressed {
							// Use parse.DecodeFlateDecode which handles both zlib and raw deflate
							decompressed, err := parse.DecodeFlateDecode(streamData)
							if err == nil {
								contentStr = string(decompressed)
							} else {
								// Fallback to raw if decompression fails
								contentStr = string(streamData)
							}
						} else {
							contentStr = string(streamData)
						}
					}
				}
			}

			textElements, graphics, images := parseContentStream(contentStr, pdf, pageObjNum, verbose)
			page.Text = append(page.Text, textElements...)
			page.Graphics = append(page.Graphics, graphics...)
			page.Images = append(page.Images, images...)
		}
	}

	// Extract resources
	resourcesRef := extractDictValue(pageStr, "/Resources")
	if resourcesRef != "" {
		// Resources is a reference to another object
		resourcesObjNum, err := parseObjectRef(resourcesRef)
		if err == nil {
			resourcesObj, err := pdf.GetObject(resourcesObjNum)
			if err == nil {
				page.Resources = extractResources(string(resourcesObj), pdf, verbose)
			}
		}
	} else {
		// Resources might be inline - extract the Resources dictionary from page string
		resourcesDictStr := extractInlineDict(pageStr, "/Resources")
		if resourcesDictStr != "" {
			page.Resources = extractResources(resourcesDictStr, pdf, verbose)
		}
	}

	// Extract annotations
	// extractDictValue already handles arrays and returns them as "[...]" strings
	annotsRef := extractDictValue(pageStr, "/Annots")
	if annotsRef != "" {
		annotations := extractAnnotations(annotsRef, pdf, pageObjNum, verbose)
		page.Annotations = annotations
	}

	return page, nil
}

// Helper functions for parsing PDF dictionaries and arrays

func extractDictValue(dictStr, key string) string {
	// Try to match simple value with space (e.g., "/Pages 2 0 R")
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+([^\s<>]+)`)
	match := pattern.FindStringSubmatch(dictStr)
	if len(match) > 1 {
		return match[1]
	}

	// Try to match value without space (e.g., "/Subtype/Type1")
	// This handles PDF dictionaries where values immediately follow keys
	noSpacePattern := regexp.MustCompile(regexp.QuoteMeta(key) + `/([^/\s<>]+)`)
	noSpaceMatch := noSpacePattern.FindStringSubmatch(dictStr)
	if len(noSpaceMatch) > 1 {
		return "/" + noSpaceMatch[1]
	}

	// Try to match array value (e.g., "/Kids[4 0 R ]")
	arrayPattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*\[([^\]]+)\]`)
	arrayMatch := arrayPattern.FindStringSubmatch(dictStr)
	if len(arrayMatch) > 1 {
		return "[" + arrayMatch[1] + "]"
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
	arrStr = strings.TrimPrefix(arrStr, "[")
	arrStr = strings.TrimSuffix(arrStr, "]")

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

// parseContentStream is implemented in content_stream.go

// extractResources is implemented in resources.go

// extractAnnotations is implemented in annotations.go
