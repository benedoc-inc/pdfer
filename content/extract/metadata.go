package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// ExtractMetadata extracts document metadata
func ExtractMetadata(pdfBytes []byte, pdf *parse.PDF, verbose bool) (*types.DocumentMetadata, error) {
	metadata := &types.DocumentMetadata{
		PDFVersion: pdf.Version(),
		PageCount:  countPages(pdf, verbose),
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
// pdfStringValue strips outer PDF string parentheses from s and returns the
// unescaped content. Pass the raw value returned by extractDictValue.
func pdfStringValue(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		s = s[1 : len(s)-1]
	}
	return unescapePDFString(s)
}

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

// maxPageTreeDepth bounds the page-tree walk. A well-formed tree is shallow;
// a deeper one means a /Kids cycle, and spinning on it is worse than
// under-counting.
const maxPageTreeDepth = 64

// countPages returns the document's real page count by walking the page tree.
//
// This used to report pdf.ObjectCount() with a "will be updated when we parse
// pages" comment that was never honoured, so DocumentMetadata.PageCount was
// the number of OBJECTS in the file — off by whatever ratio of objects to
// pages a document happened to have. A 7-page PDF with 73 objects reported 73
// pages; a 5-page extract reported 52.
//
// Prefers the /Count on the root /Pages node (the value the spec requires to
// be correct) and falls back to counting /Page leaves when /Count is missing
// or implausible.
func countPages(pdf *parse.PDF, verbose bool) int {
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return 0
	}
	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return 0
	}
	catalog, err := pdf.GetObjectContent(rootObjNum)
	if err != nil {
		return 0
	}
	pagesRef := findDictValue(string(catalog), "/Pages")
	if pagesRef == "" {
		return 0
	}
	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return 0
	}
	pagesObj, err := pdf.GetObjectContent(pagesObjNum)
	if err != nil {
		return 0
	}
	if n, err := strconv.Atoi(strings.TrimSpace(findDictValue(string(pagesObj), "/Count"))); err == nil && n > 0 {
		return n
	}
	if verbose {
		fmt.Printf("metadata: root /Pages has no usable /Count; counting leaves\n")
	}
	return countPageLeaves(pdf, pagesObjNum, 0, make(map[int]bool))
}

// countPageLeaves counts /Page nodes under a /Pages subtree.
func countPageLeaves(pdf *parse.PDF, objNum, depth int, seen map[int]bool) int {
	if depth > maxPageTreeDepth || seen[objNum] {
		return 0
	}
	seen[objNum] = true
	obj, err := pdf.GetObjectContent(objNum)
	if err != nil {
		return 0
	}
	body := string(obj)
	if strings.Contains(body, "/Type/Page") && !strings.Contains(body, "/Type/Pages") {
		return 1
	}
	kids := findDictValue(body, "/Kids")
	if kids == "" {
		return 0
	}
	total := 0
	for _, m := range regexp.MustCompile(`(\d+)\s+\d+\s+R`).FindAllStringSubmatch(kids, -1) {
		if kidNum, err := strconv.Atoi(m[1]); err == nil {
			total += countPageLeaves(pdf, kidNum, depth+1, seen)
		}
	}
	return total
}

// findDictValue reads a dictionary entry, handling the array form (/Kids [...])
// and the reference/number forms (/Pages 2 0 R, /Count 7).
func findDictValue(dict, key string) string {
	idx := strings.Index(dict, key)
	if idx < 0 {
		return ""
	}
	i := idx + len(key)
	for i < len(dict) && (dict[i] == ' ' || dict[i] == '\t' || dict[i] == '\r' || dict[i] == '\n') {
		i++
	}
	if i < len(dict) && dict[i] == '[' {
		depth := 0
		for j := i; j < len(dict); j++ {
			switch dict[j] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					return dict[i : j+1]
				}
			}
		}
		return ""
	}
	if m := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*(\d+\s+\d+\s+R|\d+)`).FindStringSubmatch(dict); m != nil {
		return m[1]
	}
	return ""
}
