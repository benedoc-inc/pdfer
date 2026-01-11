package manipulate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
)

// PDFManipulator provides functions to modify existing PDFs
type PDFManipulator struct {
	pdf      *parse.PDF
	pdfBytes []byte
	writer   *write.PDFWriter
	objects  map[int][]byte // object number -> content
	verbose  bool
}

// NewPDFManipulator creates a new PDF manipulator from existing PDF bytes
func NewPDFManipulator(pdfBytes []byte, password []byte, verbose bool) (*PDFManipulator, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// Create a new writer for rebuilding
	writer := write.NewPDFWriter()

	// Copy all objects from original PDF
	objects := make(map[int][]byte)
	for _, objNum := range pdf.Objects() {
		obj, err := pdf.GetObject(objNum)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to get object %d: %v\n", objNum, err)
			}
			continue
		}
		objects[objNum] = obj
	}

	return &PDFManipulator{
		pdf:      pdf,
		pdfBytes: pdfBytes,
		writer:   writer,
		objects:  objects,
		verbose:  verbose,
	}, nil
}

// Rebuild rebuilds the PDF with modified objects and returns the new PDF bytes
func (m *PDFManipulator) Rebuild() ([]byte, error) {
	return m.rebuildPDF()
}

// rebuildPDF rebuilds the PDF with modified objects
func (m *PDFManipulator) rebuildPDF() ([]byte, error) {
	// Add all objects to writer
	for objNum, content := range m.objects {
		m.writer.SetObject(objNum, content)
	}

	// Get trailer info
	trailer := m.pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("no trailer found")
	}

	// Set root, info, encrypt references
	if trailer.RootRef != "" {
		rootObjNum, err := parseObjectRef(trailer.RootRef)
		if err == nil {
			m.writer.SetRoot(rootObjNum)
		}
	}
	if trailer.InfoRef != "" {
		infoObjNum, err := parseObjectRef(trailer.InfoRef)
		if err == nil {
			m.writer.SetInfo(infoObjNum)
		}
	}
	if trailer.EncryptRef != "" {
		encryptObjNum, err := parseObjectRef(trailer.EncryptRef)
		if err == nil {
			m.writer.SetEncryptRef(encryptObjNum)
		}
	}

	// Set encryption if present
	if m.pdf.IsEncrypted() {
		m.writer.SetEncryption(m.pdf.Encryption(), nil) // fileID will be regenerated
	}

	return m.writer.Bytes()
}

// parseObjectRef parses an object reference like "5 0 R"
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

// extractDictValue extracts a value from a PDF dictionary string
func extractDictValue(dictStr, key string) string {
	keyIdx := strings.Index(dictStr, key)
	if keyIdx == -1 {
		return ""
	}

	// Check if the value after the key starts with '[' (array)
	valueStart := keyIdx + len(key)
	// Skip whitespace
	for valueStart < len(dictStr) && (dictStr[valueStart] == ' ' || dictStr[valueStart] == '\t' || dictStr[valueStart] == '\n' || dictStr[valueStart] == '\r') {
		valueStart++
	}

	if valueStart < len(dictStr) && dictStr[valueStart] == '[' {
		// Array value - find matching closing bracket
		arrayStart := valueStart
		depth := 0
		arrayEnd := arrayStart
		for i := arrayStart; i < len(dictStr); i++ {
			if dictStr[i] == '[' {
				depth++
			} else if dictStr[i] == ']' {
				depth--
				if depth == 0 {
					arrayEnd = i + 1
					break
				}
			}
		}
		if arrayEnd > arrayStart {
			return dictStr[arrayStart:arrayEnd]
		}
	}

	// Try to match simple value with space (e.g., "/Pages 2 0 R")
	pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+([^\s<>]+)`)
	match := pattern.FindStringSubmatch(dictStr)
	if len(match) > 1 {
		return match[1]
	}

	// Try to match value without space (e.g., "/Subtype/Type1")
	noSpacePattern := regexp.MustCompile(regexp.QuoteMeta(key) + `/([^/\s<>]+)`)
	noSpaceMatch := noSpacePattern.FindStringSubmatch(dictStr)
	if len(noSpaceMatch) > 1 {
		return "/" + noSpaceMatch[1]
	}

	return ""
}

// setDictValue sets or updates a value in a PDF dictionary string
func setDictValue(dictStr, key, value string) string {
	// Check if key already exists
	existingValue := extractDictValue(dictStr, key)
	if existingValue != "" {
		// Replace existing value
		// Try to match and replace with space (e.g., "/Count 3" -> "/Count 2")
		// Use word boundary or whitespace to ensure we match the full value
		pattern := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+([^\s<>]+)`)
		if pattern.MatchString(dictStr) {
			replaced := pattern.ReplaceAllString(dictStr, key+" "+value)
			// Verify the replacement worked
			newValue := extractDictValue(replaced, key)
			if newValue == value {
				return replaced
			}
		}
		// Try without space
		noSpacePattern := regexp.MustCompile(regexp.QuoteMeta(key) + `/([^/\s<>]+)`)
		if noSpacePattern.MatchString(dictStr) {
			return noSpacePattern.ReplaceAllString(dictStr, key+"/"+strings.TrimPrefix(value, "/"))
		}
		// Try array value - need to handle brackets properly
		keyIdx := strings.Index(dictStr, key)
		if keyIdx != -1 {
			valueStart := keyIdx + len(key)
			// Skip whitespace
			for valueStart < len(dictStr) && (dictStr[valueStart] == ' ' || dictStr[valueStart] == '\t' || dictStr[valueStart] == '\n' || dictStr[valueStart] == '\r') {
				valueStart++
			}
			if valueStart < len(dictStr) && dictStr[valueStart] == '[' {
				// Find matching closing bracket
				arrayStart := valueStart
				depth := 0
				arrayEnd := arrayStart
				for i := arrayStart; i < len(dictStr); i++ {
					if dictStr[i] == '[' {
						depth++
					} else if dictStr[i] == ']' {
						depth--
						if depth == 0 {
							arrayEnd = i + 1
							break
						}
					}
				}
				if arrayEnd > arrayStart {
					// Replace the array - preserve original spacing (no space after key if original had none)
					// Check if there was a space after the key in the original
					originalValueStart := keyIdx + len(key)
					hadSpace := originalValueStart < len(dictStr) && (dictStr[originalValueStart] == ' ' || dictStr[originalValueStart] == '\t' || dictStr[originalValueStart] == '\n' || dictStr[originalValueStart] == '\r')
					if hadSpace {
						return dictStr[:keyIdx] + key + " " + value + " " + dictStr[arrayEnd:]
					} else {
						// No space in original, don't add one
						return dictStr[:keyIdx] + key + value + " " + dictStr[arrayEnd:]
					}
				}
			}
		}
	}

	// Add new key-value pair before closing >>
	lastIdx := strings.LastIndex(dictStr, ">>")
	if lastIdx == -1 {
		return dictStr
	}
	// Insert before the closing >>, but preserve any existing spacing
	return dictStr[:lastIdx] + key + " " + value + " " + dictStr[lastIdx:]
}
