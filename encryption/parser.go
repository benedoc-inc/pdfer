package encryption

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseEncryptionDictionary parses the /Encrypt dictionary from PDF
func ParseEncryptionDictionary(pdfBytes []byte, verbose bool) (*types.PDFEncryption, error) {
	pdfStr := string(pdfBytes)

	// Find /Encrypt reference in trailer
	encryptPattern := regexp.MustCompile(`/Encrypt\s+(\d+)\s+(\d+)\s+R`)
	encryptMatch := encryptPattern.FindStringSubmatchIndex(pdfStr)

	var encryptObjNum int
	var err error

	if encryptMatch == nil {
		return nil, fmt.Errorf("/Encrypt dictionary not found")
	}

	encryptObjNum, err = strconv.Atoi(pdfStr[encryptMatch[2]:encryptMatch[3]])
	if err != nil {
		return nil, fmt.Errorf("invalid Encrypt object number: %v", err)
	}

	if verbose {
		log.Printf("Found /Encrypt dictionary: object %d", encryptObjNum)
	}

	// Find the Encrypt object
	objPattern := []byte(fmt.Sprintf("%d 0 obj", encryptObjNum))
	objIndex := bytes.Index(pdfBytes, objPattern)
	if objIndex == -1 {
		return nil, fmt.Errorf("Encrypt object %d not found", encryptObjNum)
	}

	// Find dictionary content - need to handle nested dictionaries
	dictStart := bytes.Index(pdfBytes[objIndex:], []byte("<<"))
	if dictStart == -1 {
		return nil, fmt.Errorf("Encrypt dictionary start not found")
	}
	dictStart += objIndex

	// Find matching >> (handle nested dictionaries)
	depth := 0
	dictEnd := dictStart
	for i := dictStart; i < len(pdfBytes) && i < dictStart+2000; i++ {
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

	if dictEnd == dictStart {
		return nil, fmt.Errorf("Encrypt dictionary end not found")
	}

	dictContent := pdfStr[dictStart:dictEnd]

	if verbose {
		log.Printf("Encrypt dictionary content (first 200 chars): %s", dictContent[:types.Min(200, len(dictContent))])
	}

	encrypt := &types.PDFEncryption{}

	// Parse /Filter
	filterPattern := regexp.MustCompile(`/Filter\s+/(\w+)`)
	if match := filterPattern.FindStringSubmatch(dictContent); match != nil {
		encrypt.Filter = match[1]
	}

	// Parse /V (encryption version) - must be at top level, not in nested dicts
	vPattern := regexp.MustCompile(`/V\s+(\d+)`)
	matches := vPattern.FindAllStringSubmatch(dictContent, -1)
	// Take the first match that's not inside a nested dictionary structure
	for _, match := range matches {
		// Check if it's at top level (simple heuristic: not immediately after /CF or /StdCF)
		matchPos := strings.Index(dictContent, match[0])
		beforeMatch := dictContent[:matchPos]
		// If we see /CF or /StdCF before this, it's nested - skip
		if !strings.Contains(beforeMatch[types.Max(0, len(beforeMatch)-50):], "/CF") &&
			!strings.Contains(beforeMatch[types.Max(0, len(beforeMatch)-50):], "/StdCF") {
			encrypt.V, _ = strconv.Atoi(match[1])
			break
		}
	}

	// Parse /R (revision) - must be at top level
	rPattern := regexp.MustCompile(`/R\s+(\d+)`)
	matches = rPattern.FindAllStringSubmatch(dictContent, -1)
	for _, match := range matches {
		matchPos := strings.Index(dictContent, match[0])
		beforeMatch := dictContent[:matchPos]
		if !strings.Contains(beforeMatch[types.Max(0, len(beforeMatch)-50):], "/CF") &&
			!strings.Contains(beforeMatch[types.Max(0, len(beforeMatch)-50):], "/StdCF") {
			encrypt.R, _ = strconv.Atoi(match[1])
			break
		}
	}

	// Parse /Length (key length in bits) - look for top-level /Length
	// Note: There may be nested /Length in /CF dictionaries, we want the top-level one
	lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
	matches = lengthPattern.FindAllStringSubmatch(dictContent, -1)
	for _, match := range matches {
		matchPos := strings.Index(dictContent, match[0])
		beforeMatch := dictContent[:matchPos]
		// Top-level /Length should not be inside /CF
		if !strings.Contains(beforeMatch[types.Max(0, len(beforeMatch)-50):], "/CF") {
			keyBits, _ := strconv.Atoi(match[1])
			if keyBits > 0 {
				encrypt.KeyLength = keyBits / 8
				break
			}
		}
	}

	// Set defaults if not found
	if encrypt.V == 0 {
		encrypt.V = 4 // Default to AES encryption
	}
	if encrypt.R == 0 {
		encrypt.R = 4 // Default revision
	}
	if encrypt.KeyLength == 0 {
		// Default key length based on revision
		if encrypt.R == 2 {
			encrypt.KeyLength = 5 // 40 bits
		} else {
			encrypt.KeyLength = 16 // 128 bits
		}
	}

	// Parse /O (owner password hash) - binary data in parentheses
	// Find /O in the dictionary, then extract the binary data between ( and )
	oPattern := regexp.MustCompile(`/O\s*\(`)
	oMatch := oPattern.FindStringIndex(dictContent)
	if oMatch != nil {
		oStartInDict := dictStart + oMatch[1] - 1 // Position of '('
		parenStart := bytes.Index(pdfBytes[oStartInDict:], []byte("("))
		if parenStart != -1 {
			parenStart += oStartInDict + 1
			parenEnd := bytes.Index(pdfBytes[parenStart:], []byte(")"))
			if parenEnd != -1 {
				encrypt.O = make([]byte, parenEnd)
				copy(encrypt.O, pdfBytes[parenStart:parenStart+parenEnd])
				if verbose {
					log.Printf("Extracted O value: %d bytes", len(encrypt.O))
				}
			}
		}
	}

	// Parse /U (user password hash) - binary data in parentheses
	uPattern := regexp.MustCompile(`/U\s*\(`)
	uMatch := uPattern.FindStringIndex(dictContent)
	if uMatch != nil {
		uStartInDict := dictStart + uMatch[1] - 1 // Position of '('
		parenStart := bytes.Index(pdfBytes[uStartInDict:], []byte("("))
		if parenStart != -1 {
			parenStart += uStartInDict + 1
			parenEnd := bytes.Index(pdfBytes[parenStart:], []byte(")"))
			if parenEnd != -1 {
				encrypt.U = make([]byte, parenEnd)
				copy(encrypt.U, pdfBytes[parenStart:parenStart+parenEnd])
				if verbose {
					log.Printf("Extracted U value: %d bytes", len(encrypt.U))
				}
			}
		}
	}

	// Parse /P (permissions)
	pPattern := regexp.MustCompile(`/P\s+(-?\d+)`)
	if match := pPattern.FindStringSubmatch(dictContent); match != nil {
		pVal, _ := strconv.ParseInt(match[1], 10, 32)
		encrypt.P = int32(pVal)
	}

	// Parse /EncryptMetadata
	if strings.Contains(dictContent, "/EncryptMetadata false") {
		encrypt.EncryptMetadata = false
	} else {
		encrypt.EncryptMetadata = true
	}

	return encrypt, nil
}
