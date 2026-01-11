package xfa

import (
	"bytes"
	"fmt"
	"log"
	"regexp"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/types"
)

// parseObjectStructure parses a PDF object structure first, then decrypts encrypted parts
// This follows PyPDF's approach: parse structure, then decrypt values
func parseObjectStructure(pdfBytes []byte, objNum, genNum int, objOffset int64, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Step 1: Seek to object location (like PyPDF line 423)
	if objOffset < 0 || int(objOffset) >= len(pdfBytes) {
		return nil, fmt.Errorf("invalid object offset: %d", objOffset)
	}

	// Step 2: Read object header "212 0 obj" (like PyPDF line 425)
	// PyPDF seeks to xref offset and reads header directly
	// But if header is encrypted, we need to search for it
	// Try multiple patterns and search areas
	headerPatterns := []*regexp.Regexp{
		regexp.MustCompile(fmt.Sprintf(`%d\s+%d\s+obj`, objNum, genNum)),
		regexp.MustCompile(fmt.Sprintf(`%d\s+%d\s+obj`, objNum, genNum)),
		regexp.MustCompile(fmt.Sprintf(`\s%d\s+%d\s+obj`, objNum, genNum)), // With leading space
	}
	
	var headerMatch []int
	var searchStart, searchEnd int
	
	// Try searching near xref offset first (most common case)
	searchStart = types.Max(0, int(objOffset)-200)
	searchEnd = types.Min(len(pdfBytes), int(objOffset)+2000)
	searchArea := pdfBytes[searchStart:searchEnd]
	
	for _, pattern := range headerPatterns {
		headerMatch = pattern.FindIndex(searchArea)
		if headerMatch != nil {
			break
		}
	}
	
	// If not found, try wider search
	if headerMatch == nil {
		if verbose {
			log.Printf("Object header not found near offset %d, trying wider search", objOffset)
		}
		searchStart = types.Max(0, int(objOffset)-1000)
		searchEnd = types.Min(len(pdfBytes), int(objOffset)+5000)
		searchArea = pdfBytes[searchStart:searchEnd]
		
		for _, pattern := range headerPatterns {
			headerMatch = pattern.FindIndex(searchArea)
			if headerMatch != nil {
				break
			}
		}
	}
	
	if headerMatch == nil {
		// Last resort: search entire file (slow but might work)
		if verbose {
			log.Printf("Object header not found in search window, searching entire file")
		}
		searchArea = pdfBytes
		for _, pattern := range headerPatterns {
			headerMatch = pattern.FindIndex(searchArea)
			if headerMatch != nil {
				searchStart = 0
				break
			}
		}
	}
	
	if headerMatch == nil {
		return nil, fmt.Errorf("object header %d %d obj not found", objNum, genNum)
	}
	
	objStart := searchStart + headerMatch[1] // Position after "obj"
	if verbose {
		log.Printf("Found object header at offset %d (absolute: %d)", headerMatch[0], searchStart+headerMatch[0])
	}

	// Step 3: Parse object structure (like PyPDF's read_object)
	// Skip whitespace after "obj"
	pos := objStart
	for pos < len(pdfBytes) && (pdfBytes[pos] == ' ' || pdfBytes[pos] == '\r' || pdfBytes[pos] == '\n' || pdfBytes[pos] == '\t') {
		pos++
	}

	// Check if it's a dictionary (starts with "<<")
	if pos+2 > len(pdfBytes) || pdfBytes[pos] != '<' || pdfBytes[pos+1] != '<' {
		return nil, fmt.Errorf("object does not start with dictionary marker <<")
	}

	// Step 4: Find dictionary end ">>"
	dictStart := pos
	depth := 0
	dictEnd := -1
	for i := dictStart; i < len(pdfBytes) && i < dictStart+50000; i++ {
		if i+1 < len(pdfBytes) && pdfBytes[i] == '<' && pdfBytes[i+1] == '<' {
			depth++
			i++ // Skip second '<'
		} else if i+1 < len(pdfBytes) && pdfBytes[i] == '>' && pdfBytes[i+1] == '>' {
			depth--
			if depth == 0 {
				dictEnd = i + 2
				break
			}
			i++ // Skip second '>'
		}
	}

	if dictEnd == -1 {
		return nil, fmt.Errorf("dictionary end >> not found")
	}

	// Step 5: Check if there's a stream
	streamPos := dictEnd
	// Skip whitespace after ">>"
	for streamPos < len(pdfBytes) && (pdfBytes[streamPos] == ' ' || pdfBytes[streamPos] == '\r' || pdfBytes[streamPos] == '\n' || pdfBytes[streamPos] == '\t') {
		streamPos++
	}

	isStream := false
	streamDataStart := -1
	streamDataEnd := -1
	
	if streamPos+6 <= len(pdfBytes) && bytes.Equal(pdfBytes[streamPos:streamPos+6], []byte("stream")) {
		isStream = true
		streamPos += 6
		
		// Skip EOL after "stream" (PyPDF lines 611-619)
		if streamPos < len(pdfBytes) {
			if pdfBytes[streamPos] == '\r' {
				streamPos++
				if streamPos < len(pdfBytes) && pdfBytes[streamPos] == '\n' {
					streamPos++
				}
			} else if pdfBytes[streamPos] == '\n' {
				streamPos++
			}
		}
		
		streamDataStart = streamPos
		
		// Find "endstream"
		endstreamPos := bytes.Index(pdfBytes[streamDataStart:], []byte("endstream"))
		if endstreamPos == -1 {
			return nil, fmt.Errorf("endstream not found")
		}
		streamDataEnd = streamDataStart + endstreamPos
	}

	// Step 6: Decrypt dictionary content (between "<<" and ">>")
	// The dictionary structure itself is NOT encrypted, but string values are
	// For encrypted PDFs, the entire dictionary content between << and >> is encrypted
	dictContent := pdfBytes[dictStart+2 : dictEnd-2] // Remove "<< " and " >>"
	
	if encryptInfo != nil && len(dictContent) > 0 {
		// Try direct decryption first
		decryptedDict, err := encryption.DecryptObject(dictContent, objNum, genNum, encryptInfo)
		if err != nil {
			if verbose {
				log.Printf("Direct decryption failed, trying chunked decryption: %v", err)
			}
			// Try decrypting in chunks (for alignment) - uses existing function from xfa_utils.go
			decryptedDict, err = decryptInChunks(dictContent, objNum, genNum, encryptInfo, verbose)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt dictionary: %v", err)
			}
		}
		dictContent = decryptedDict
	}

	// Step 7: Reconstruct object
	result := make([]byte, 0, len(dictContent)+1000)
	
	// Object header
	result = append(result, []byte(fmt.Sprintf("%d %d obj\n", objNum, genNum))...)
	
	// Dictionary
	result = append(result, []byte("<<")...)
	result = append(result, dictContent...)
	result = append(result, []byte(">>")...)
	
	// Stream data (if present) - decrypt it
	if isStream {
		result = append(result, []byte("\nstream\n")...)
		
		if encryptInfo != nil {
			streamData := pdfBytes[streamDataStart:streamDataEnd]
			decryptedStream, err := encryption.DecryptObject(streamData, objNum, genNum, encryptInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt stream data: %v", err)
			}
			result = append(result, decryptedStream...)
		} else {
			result = append(result, pdfBytes[streamDataStart:streamDataEnd]...)
		}
		
		result = append(result, []byte("\nendstream")...)
	}
	
	result = append(result, []byte("\nendobj")...)
	
	return result, nil
}

