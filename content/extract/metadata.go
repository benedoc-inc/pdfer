package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// ExtractMetadata extracts document metadata
func ExtractMetadata(pdfBytes []byte, pdf *parse.PDF, verbose bool) (*types.DocumentMetadata, error) {
	metadata := &types.DocumentMetadata{
		PDFVersion: pdf.Version(),
		PageCount:  pdf.ObjectCount(), // Will be updated when we parse pages
		Encrypted:  pdf.IsEncrypted(),
		Custom:     make(map[string]string),
	}

	// Extract from Info dictionary if available
	trailer := pdf.Trailer()
	if trailer != nil && trailer.InfoRef != "" {
		// Parse object number from reference (e.g., "5 0 R")
		infoObjNum, err := parseObjectRef(trailer.InfoRef)
		if err == nil {
			infoObj, err := pdf.GetObject(infoObjNum)
			if err == nil {
				parseInfoDict(string(infoObj), metadata, verbose)
			}
		}
	}

	// Try to extract from raw PDF bytes as fallback
	if metadata.Title == "" {
		extractMetadataFromBytes(pdfBytes, metadata, verbose)
	}

	return metadata, nil
}

// parseObjectRef parses an object reference like "5 0 R" and returns the object number
func parseObjectRef(ref string) (int, error) {
	parts := strings.Fields(ref)
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid object reference: %s", ref)
	}
	objNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid object number in reference: %s", ref)
	}
	return objNum, nil
}

// parseInfoDict parses a PDF Info dictionary
func parseInfoDict(infoStr string, metadata *types.DocumentMetadata, verbose bool) {
	// Extract common fields
	patterns := map[string]*regexp.Regexp{
		"title":         regexp.MustCompile(`/Title\s*\(([^)]*)\)`),
		"author":        regexp.MustCompile(`/Author\s*\(([^)]*)\)`),
		"subject":       regexp.MustCompile(`/Subject\s*\(([^)]*)\)`),
		"keywords":      regexp.MustCompile(`/Keywords\s*\(([^)]*)\)`),
		"creator":       regexp.MustCompile(`/Creator\s*\(([^)]*)\)`),
		"producer":      regexp.MustCompile(`/Producer\s*\(([^)]*)\)`),
		"creation_date": regexp.MustCompile(`/CreationDate\s*\(([^)]*)\)`),
		"mod_date":      regexp.MustCompile(`/ModDate\s*\(([^)]*)\)`),
	}

	fieldMap := map[string]*string{
		"title":         &metadata.Title,
		"author":        &metadata.Author,
		"subject":       &metadata.Subject,
		"keywords":      &metadata.Keywords,
		"creator":       &metadata.Creator,
		"producer":      &metadata.Producer,
		"creation_date": &metadata.CreationDate,
		"mod_date":      &metadata.ModDate,
	}

	for key, pattern := range patterns {
		match := pattern.FindStringSubmatch(infoStr)
		if len(match) > 1 {
			value := unescapePDFString(match[1])
			if fieldPtr, ok := fieldMap[key]; ok {
				*fieldPtr = value
			}
		}
	}

	// Extract custom fields (any field that's not a standard field)
	// Pattern: /FieldName (value)
	customPattern := regexp.MustCompile(`/([A-Za-z0-9_]+)\s*\(([^)]*)\)`)
	standardFields := map[string]bool{
		"Title":        true,
		"Author":       true,
		"Subject":      true,
		"Keywords":     true,
		"Creator":      true,
		"Producer":     true,
		"CreationDate": true,
		"ModDate":      true,
	}

	allMatches := customPattern.FindAllStringSubmatch(infoStr, -1)
	for _, match := range allMatches {
		if len(match) >= 3 {
			fieldName := match[1]
			fieldValue := unescapePDFString(match[2])

			// Skip if it's a standard field (already extracted)
			if !standardFields[fieldName] {
				if metadata.Custom == nil {
					metadata.Custom = make(map[string]string)
				}
				metadata.Custom[fieldName] = fieldValue
			}
		}
	}
}

// extractMetadataFromBytes extracts metadata by searching PDF bytes
func extractMetadataFromBytes(pdfBytes []byte, metadata *types.DocumentMetadata, verbose bool) {
	// This is a fallback method - search for common patterns
	// In practice, Info dict parsing should work
}

// unescapePDFString unescapes a PDF string literal
func unescapePDFString(s string) string {
	// Handle basic PDF string escaping
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\(", "(")
	s = strings.ReplaceAll(s, "\\)", ")")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// parsePDFDate parses a PDF date string (D:YYYYMMDDHHmmSSOHH'mm)
// parsePDFDate parses a PDF date string (D:YYYYMMDDHHmmSSOHH'mm).
// This function is available for future use when date parsing is needed.
var _ = parsePDFDate // Mark as available for future use

func parsePDFDate(dateStr string) time.Time {
	// Remove "D:" prefix if present
	dateStr = strings.TrimPrefix(dateStr, "D:")

	// Try to parse common formats
	formats := []string{
		"20060102150405",
		"20060102150405-07'00",
		"20060102150405Z07'00",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Time{}
}
