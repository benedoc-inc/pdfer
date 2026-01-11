package parser

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParsePDFTrailer parses the PDF trailer to find object references
func ParsePDFTrailer(pdfBytes []byte) (*PDFTrailer, error) {
	pdfStr := string(pdfBytes)

	// Find trailer - search from the end (trailer is usually near EOF)
	// Try multiple patterns
	trailerPatterns := []*regexp.Regexp{
		regexp.MustCompile(`trailer\s*<<`),
		regexp.MustCompile(`trailer\s*<<\s*`),
	}

	var trailerMatch []int
	for _, pattern := range trailerPatterns {
		// Search from the end
		matches := pattern.FindAllStringIndex(pdfStr, -1)
		if len(matches) > 0 {
			trailerMatch = matches[len(matches)-1] // Take the last match (closest to EOF)
			break
		}
	}

	if trailerMatch == nil {
		return nil, fmt.Errorf("trailer not found")
	}

	// Find end of trailer dictionary (handle nested dictionaries)
	trailerStart := trailerMatch[1]

	// Find matching >> (handle nested dictionaries)
	// Start at depth 1 because trailerStart is AFTER the opening <<
	depth := 1
	trailerEnd := trailerStart
	for i := trailerStart; i < len(pdfStr) && i < trailerStart+5000; i++ {
		if i+1 < len(pdfStr) && pdfStr[i] == '<' && pdfStr[i+1] == '<' {
			depth++
			i++ // Skip second '<'
		} else if i+1 < len(pdfStr) && pdfStr[i] == '>' && pdfStr[i+1] == '>' {
			depth--
			if depth == 0 {
				trailerEnd = i + 2
				break
			}
			i++ // Skip second '>'
		}
	}

	if trailerEnd == trailerStart {
		return nil, fmt.Errorf("trailer end not found")
	}

	trailerSection := pdfStr[trailerStart:trailerEnd]

	trailer := &PDFTrailer{}

	// Extract Root reference
	rootPattern := regexp.MustCompile(`/Root\s+(\d+)\s+(\d+)\s+R`)
	rootMatch := rootPattern.FindStringSubmatch(trailerSection)
	if rootMatch != nil {
		trailer.RootRef = rootMatch[0]
	}

	// Extract Encrypt reference
	encryptPattern := regexp.MustCompile(`/Encrypt\s+(\d+)\s+(\d+)\s+R`)
	encryptMatch := encryptPattern.FindStringSubmatch(trailerSection)
	if encryptMatch != nil {
		trailer.EncryptRef = encryptMatch[0]
	}

	// Find startxref
	startXRefPattern := regexp.MustCompile(`startxref\s+(\d+)`)
	startXRefMatch := startXRefPattern.FindStringSubmatch(pdfStr)
	if startXRefMatch != nil {
		offset, err := strconv.ParseInt(startXRefMatch[1], 10, 64)
		if err == nil {
			trailer.StartXRef = offset
		}
	}

	return trailer, nil
}

// FindObjectByNumber finds a PDF object by its number
func FindObjectByNumber(pdfBytes []byte, objNum int, encryptInfo *types.PDFEncryption, verbose bool) (int, error) {
	// First, try direct search (works for unencrypted or if object header is visible)
	objPattern := []byte(fmt.Sprintf("%d 0 obj", objNum))
	objIndex := bytes.Index(pdfBytes, objPattern)
	if objIndex != -1 {
		if verbose {
			log.Printf("Found object %d at byte position %d (direct search)", objNum, objIndex)
		}
		return objIndex, nil
	}

	// If not found, try parsing cross-reference table (works for encrypted PDFs)
	// Find startxref directly (works even without trailer keyword)
	startxrefPattern := regexp.MustCompile(`startxref\s+(\d+)`)
	startxrefMatch := startxrefPattern.FindStringSubmatch(string(pdfBytes))

	if startxrefMatch != nil {
		startXRef, err := strconv.ParseInt(startxrefMatch[1], 10, 64)
		if err == nil && startXRef > 0 {
			if verbose {
				log.Printf("Found startxref at offset %d, attempting to parse cross-reference", startXRef)
			}
			objMap, xrefErr := ParseCrossReferenceTableWithEncryption(pdfBytes, startXRef, encryptInfo, verbose)
			if xrefErr != nil {
				if verbose {
					log.Printf("Error parsing xref table: %v", xrefErr)
				}
			} else {
				if verbose {
					log.Printf("Parsed xref table: found %d objects", len(objMap))
				}
				if offset, ok := objMap[objNum]; ok {
					if verbose {
						log.Printf("Found object %d via cross-reference table at offset %d", objNum, offset)
					}
					return int(offset), nil
				} else if verbose {
					log.Printf("Object %d not found in xref table (available objects: %v)", objNum, GetSampleObjectNumbers(objMap, 10))
				}
			}
		}
	} else if verbose {
		log.Printf("No startxref found in PDF")
	}

	return -1, fmt.Errorf("object %d not found", objNum)
}

// ParseCrossReferenceTable parses the PDF cross-reference table
// This allows finding objects even in encrypted PDFs
// Handles both traditional xref tables and cross-reference streams
func ParseCrossReferenceTable(pdfBytes []byte, startXRef int64) (map[int]int64, error) {
	return ParseCrossReferenceTableWithEncryption(pdfBytes, startXRef, nil, false)
}

// ParseCrossReferenceTableWithEncryption parses xref table with optional decryption
func ParseCrossReferenceTableWithEncryption(pdfBytes []byte, startXRef int64, encryptInfo *types.PDFEncryption, verbose bool) (map[int]int64, error) {
	if startXRef <= 0 || startXRef >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("invalid startxref offset: %d", startXRef)
	}

	// Read from startxref position to determine type
	xrefSection := pdfBytes[startXRef:]
	xrefStr := string(xrefSection[:min(5000, len(xrefSection))])

	// Check if it's a cross-reference stream (PDF 1.5+)
	// Cross-reference streams have /Type/XRef and /W[widths]
	if strings.Contains(xrefStr, "/Type/XRef") || strings.Contains(xrefStr, "/W[") {
		// Cross-reference stream - need to find the stream object and decompress it
		return ParseXRefStreamWithEncryption(pdfBytes, startXRef, encryptInfo, verbose)
	}

	// Traditional xref table
	return ParseTraditionalXRefTable(pdfBytes, startXRef)
}

// ParseTraditionalXRefTable parses a traditional PDF cross-reference table
func ParseTraditionalXRefTable(pdfBytes []byte, startXRef int64) (map[int]int64, error) {
	objMap := make(map[int]int64)

	// Read from startxref position
	xrefSection := pdfBytes[startXRef:]
	xrefStr := string(xrefSection[:min(10000, len(xrefSection))])

	// Find xref keyword
	xrefPos := strings.Index(xrefStr, "xref")
	if xrefPos == -1 {
		return nil, fmt.Errorf("xref keyword not found")
	}

	// Parse xref subsections
	// Format: xref\n0 N\nM K\n... where:
	// - First line after "xref": "0 N" means starting object 0, N entries
	// - Subsequent lines: "offset generation f" or "offset generation n"
	//   where f=free, n=in-use
	lines := strings.Split(xrefStr[xrefPos:], "\n")

	currentObjNum := 0
	inSubsection := false

	for i, line := range lines {
		if i == 0 {
			continue // Skip "xref" line
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 2 {
			// Subsection header: "first_obj_num count"
			firstObj, err1 := strconv.Atoi(fields[0])
			_, err2 := strconv.Atoi(fields[1])
			if err1 == nil && err2 == nil {
				currentObjNum = firstObj
				inSubsection = true
				continue
			}
		}

		if inSubsection && len(fields) >= 3 {
			// Entry: "offset generation flag"
			offset, err1 := strconv.ParseInt(fields[0], 10, 64)
			_, err2 := strconv.Atoi(fields[1])
			flag := fields[2]

			if err1 == nil && err2 == nil && flag == "n" {
				// In-use object
				objMap[currentObjNum] = offset
			}
			currentObjNum++
		}
	}

	return objMap, nil
}

// GetSampleObjectNumbers returns a sample of object numbers from the map (for debugging)
func GetSampleObjectNumbers(objMap map[int]int64, max int) []int {
	result := make([]int, 0, max)
	count := 0
	for objNum := range objMap {
		if count >= max {
			break
		}
		result = append(result, objNum)
		count++
	}
	return result
}
