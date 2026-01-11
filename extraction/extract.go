// Package extraction provides comprehensive PDF content extraction
// This package extracts all content types from PDFs into structured data models
// that can be serialized to JSON
package extraction

import (
	"encoding/json"
	"fmt"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// ExtractContent extracts all content from a PDF into a ContentDocument
// This is the main entry point for content extraction
func ExtractContent(pdfBytes []byte, password []byte, verbose bool) (*types.ContentDocument, error) {
	// Parse PDF
	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	doc := &types.ContentDocument{
		Pages:       []types.Page{},
		Bookmarks:   []types.Bookmark{},
		Annotations: []types.Annotation{},
		Images:      []types.Image{},
		Fonts:       []types.FontInfo{},
	}

	// Extract metadata
	metadata, err := ExtractMetadata(pdfBytes, pdf, verbose)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to extract metadata: %v\n", err)
		}
	} else {
		doc.Metadata = metadata
	}

	// Extract pages
	pages, err := ExtractPages(pdfBytes, pdf, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to extract pages: %w", err)
	}
	doc.Pages = pages

	// Extract bookmarks/outlines
	bookmarks, err := ExtractBookmarks(pdfBytes, pdf, verbose)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to extract bookmarks: %v\n", err)
		}
	} else {
		doc.Bookmarks = bookmarks
	}

	// Extract annotations (from all pages)
	allAnnotations := []types.Annotation{}
	for _, page := range doc.Pages {
		allAnnotations = append(allAnnotations, page.Annotations...)
	}
	doc.Annotations = allAnnotations

	// Extract images (collect unique images from all pages)
	imageMap := make(map[string]types.Image)
	for _, page := range doc.Pages {
		if page.Resources != nil && page.Resources.Images != nil {
			for id, img := range page.Resources.Images {
				if _, exists := imageMap[id]; !exists {
					imageMap[id] = img
				}
			}
		}
	}
	for _, img := range imageMap {
		doc.Images = append(doc.Images, img)
	}

	// Extract fonts (collect unique fonts from all pages)
	fontMap := make(map[string]types.FontInfo)
	for _, page := range doc.Pages {
		if page.Resources != nil && page.Resources.Fonts != nil {
			for id, font := range page.Resources.Fonts {
				if _, exists := fontMap[id]; !exists {
					fontMap[id] = font
				}
			}
		}
	}
	for _, font := range fontMap {
		doc.Fonts = append(doc.Fonts, font)
	}

	return doc, nil
}

// ExtractContentToJSON extracts content and returns as JSON string
func ExtractContentToJSON(pdfBytes []byte, password []byte, verbose bool) (string, error) {
	doc, err := ExtractContent(pdfBytes, password, verbose)
	if err != nil {
		return "", err
	}

	// Use standard library JSON marshaling
	jsonBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	return string(jsonBytes), nil
}
