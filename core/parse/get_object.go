package parse

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/types"
)

// ObjectLocation describes where an object is located
type ObjectLocation struct {
	IsDirect      bool  // True if object is at a direct byte offset
	ByteOffset    int64 // For direct objects: byte offset in PDF
	StreamObjNum  int   // For object stream objects: containing stream's object number
	IndexInStream int   // For object stream objects: index within the stream
}

// FindObjectLocation finds where an object is located (direct or in object stream)
func FindObjectLocation(pdfBytes []byte, objNum int, verbose bool) (*ObjectLocation, error) {
	// ALWAYS use xref table first - it's the authoritative source
	// Direct search using bytes.Index is dangerous because "5 0 obj" matches inside "265 0 obj"

	// Parse xref to find object
	startxrefPattern := regexp.MustCompile(`startxref\s+(\d+)`)
	startxrefMatch := startxrefPattern.FindStringSubmatch(string(pdfBytes))
	if startxrefMatch == nil {
		return nil, fmt.Errorf("startxref not found")
	}

	startXRef, err := strconv.ParseInt(startxrefMatch[1], 10, 64)
	if err != nil || startXRef <= 0 {
		return nil, fmt.Errorf("invalid startxref: %s", startxrefMatch[1])
	}

	// Bounds check
	if startXRef >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("startxref offset %d is beyond PDF length %d", startXRef, len(pdfBytes))
	}

	// Determine if this is a traditional xref or xref stream
	xrefSection := pdfBytes[startXRef:]

	// Check if it's an xref stream (PDF 1.5+)
	// Xref streams start with "N 0 obj" instead of "xref"
	if bytes.Contains(xrefSection[:min(100, len(xrefSection))], []byte("obj")) {
		// It's an xref stream - use full parsing
		result, err := ParseXRefStreamFull(pdfBytes, startXRef, verbose)
		if err != nil {
			if verbose {
				log.Printf("Failed to parse xref stream: %v", err)
			}
		} else {
			// Check if object is in object stream
			if entry, ok := result.ObjectStreams[objNum]; ok {
				if verbose {
					log.Printf("Object %d is in object stream %d at index %d", objNum, entry.StreamObjNum, entry.IndexInStream)
				}
				return &ObjectLocation{
					IsDirect:      false,
					StreamObjNum:  entry.StreamObjNum,
					IndexInStream: entry.IndexInStream,
				}, nil
			}

			// Check regular objects
			if offset, ok := result.Objects[objNum]; ok {
				if verbose {
					log.Printf("Object %d at byte offset %d (from xref stream)", objNum, offset)
				}
				return &ObjectLocation{IsDirect: true, ByteOffset: offset}, nil
			}
		}
	} else if bytes.HasPrefix(xrefSection, []byte("xref")) {
		// Traditional xref table
		objMap, err := ParseTraditionalXRefTable(pdfBytes, startXRef)
		if err == nil {
			if offset, ok := objMap[objNum]; ok {
				if verbose {
					log.Printf("Object %d at byte offset %d (from traditional xref)", objNum, offset)
				}
				return &ObjectLocation{IsDirect: true, ByteOffset: offset}, nil
			}
		}
	}

	// Not found in xref - try regex search for object header with word boundary
	// Use negative lookbehind equivalent: require whitespace or start of file before objNum
	pattern := regexp.MustCompile(fmt.Sprintf(`(^|\s|[\r\n])%d\s+0\s+obj`, objNum))
	matches := pattern.FindIndex(pdfBytes)
	if matches != nil {
		// Find the actual start of the object number
		offset := int64(matches[0])
		// Skip the preceding whitespace/newline
		for offset < int64(len(pdfBytes)) && (pdfBytes[offset] == ' ' || pdfBytes[offset] == '\r' || pdfBytes[offset] == '\n' || pdfBytes[offset] == '\t') {
			offset++
		}
		if verbose {
			log.Printf("Object %d found via regex at offset %d", objNum, offset)
		}
		return &ObjectLocation{IsDirect: true, ByteOffset: offset}, nil
	}

	return nil, fmt.Errorf("object %d not found", objNum)
}

// GetObject retrieves a PDF object, handling both direct objects and objects in object streams
// This is the equivalent of PyPDF's get_object() method
func GetObject(pdfBytes []byte, objNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Find where the object is located
	loc, err := FindObjectLocation(pdfBytes, objNum, verbose)
	if err != nil {
		return nil, fmt.Errorf("object %d not found: %v", objNum, err)
	}

	if !loc.IsDirect {
		// Object is in an object stream - extract it
		if verbose {
			log.Printf("Extracting object %d from object stream %d (index %d)", objNum, loc.StreamObjNum, loc.IndexInStream)
		}
		return GetObjectFromStream(pdfBytes, objNum, loc.StreamObjNum, loc.IndexInStream, encryptInfo, verbose)
	}

	// Direct object - read it from the byte offset
	if verbose {
		log.Printf("Reading direct object %d from offset %d", objNum, loc.ByteOffset)
	}
	return GetDirectObject(pdfBytes, objNum, loc.ByteOffset, encryptInfo, verbose)
}

// GetDirectObject reads a PDF object at a specific byte offset
func GetDirectObject(pdfBytes []byte, objNum int, offset int64, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	if offset < 0 || offset >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("invalid offset %d for object %d", offset, objNum)
	}

	objData := pdfBytes[offset:]

	// Verify object header
	headerPattern := regexp.MustCompile(fmt.Sprintf(`^%d\s+(\d+)\s+obj`, objNum))
	headerMatch := headerPattern.FindSubmatch(objData[:min(50, len(objData))])

	var genNum int
	if headerMatch == nil {
		// Header not exactly at offset - try to find it nearby
		searchArea := pdfBytes[max(0, int(offset)-100):min(len(pdfBytes), int(offset)+100)]
		pattern := regexp.MustCompile(fmt.Sprintf(`%d\s+(\d+)\s+obj`, objNum))
		match := pattern.FindIndex(searchArea)
		if match == nil {
			if verbose {
				log.Printf("Object %d header not found near offset %d", objNum, offset)
			}
			// Continue anyway with content at offset
			genNum = 0
		} else {
			// Adjust offset
			newOffset := max(0, int(offset)-100) + match[0]
			offset = int64(newOffset)
			objData = pdfBytes[offset:]
			headerMatch = headerPattern.FindSubmatch(objData[:min(50, len(objData))])
			if headerMatch != nil {
				genNum, _ = strconv.Atoi(string(headerMatch[1]))
			}
		}
	} else {
		genNum, _ = strconv.Atoi(string(headerMatch[1]))
	}

	// Find endobj
	endobjPos := bytes.Index(objData, []byte("endobj"))
	if endobjPos == -1 {
		return nil, fmt.Errorf("endobj not found for object %d", objNum)
	}

	// Extract content between header and endobj
	// Skip past "N G obj" to get to the content
	contentStart := bytes.Index(objData[:endobjPos], []byte("obj"))
	if contentStart == -1 {
		contentStart = 0
	} else {
		contentStart += 3 // Skip "obj"
		// Skip whitespace
		for contentStart < endobjPos && (objData[contentStart] == ' ' || objData[contentStart] == '\n' || objData[contentStart] == '\r' || objData[contentStart] == '\t') {
			contentStart++
		}
	}

	content := objData[contentStart:endobjPos]

	// Check if this is a stream object
	streamStart := bytes.Index(content, []byte("stream"))
	if streamStart != -1 {
		// This is a stream object
		// Find "endstream" to get the full stream
		endstreamPos := bytes.Index(content, []byte("endstream"))
		if endstreamPos == -1 {
			endstreamPos = len(content)
		}
		content = content[:endstreamPos+len("endstream")]

		// Decrypt stream data if needed
		if encryptInfo != nil {
			// Get /Length from dictionary for exact stream data size
			dictPart := content[:streamStart]
			lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
			lengthMatch := lengthPattern.FindSubmatch(dictPart)

			var streamLength int
			if lengthMatch != nil {
				streamLength, _ = strconv.Atoi(string(lengthMatch[1]))
			}

			// Find actual stream data (after "stream\r\n" or "stream\n")
			streamDataStart := streamStart + 6
			// Skip exactly one EOL per PDF spec
			if streamDataStart < len(content) && content[streamDataStart] == '\r' {
				streamDataStart++
			}
			if streamDataStart < len(content) && content[streamDataStart] == '\n' {
				streamDataStart++
			}

			// Use /Length if available, otherwise find endstream
			var streamData []byte
			if streamLength > 0 && streamDataStart+streamLength <= len(content) {
				streamData = content[streamDataStart : streamDataStart+streamLength]
			} else {
				streamDataEnd := bytes.Index(content[streamDataStart:], []byte("endstream"))
				if streamDataEnd == -1 {
					streamDataEnd = len(content) - streamDataStart
				}
				streamData = content[streamDataStart : streamDataStart+streamDataEnd]
			}

			if verbose {
				log.Printf("Decrypting stream: %d bytes (from /Length %d), objNum=%d, genNum=%d", len(streamData), streamLength, objNum, genNum)
			}
			decryptedStream, err := encrypt.DecryptObject(streamData, objNum, genNum, encryptInfo)
			if err == nil {
				if verbose {
					log.Printf("Decryption successful: %d -> %d bytes", len(streamData), len(decryptedStream))
				}
				// Update /Length in the dictionary to reflect decrypted size
				newLength := fmt.Sprintf("/Length %d", len(decryptedStream))
				dictPart = lengthPattern.ReplaceAll(dictPart, []byte(newLength))

				// Reconstruct content with updated dictionary and decrypted stream
				newContent := make([]byte, 0, len(dictPart)+len(decryptedStream)+20)
				newContent = append(newContent, dictPart...)
				newContent = append(newContent, []byte("stream\n")...)
				newContent = append(newContent, decryptedStream...)
				newContent = append(newContent, []byte("\nendstream")...)
				content = newContent

				if verbose {
					log.Printf("Reconstructed content: %d bytes, new dict: %s", len(content), string(dictPart[:min(100, len(dictPart))]))
				}
			} else if verbose {
				log.Printf("Decryption failed: %v", err)
			}
		}
	} else if encryptInfo != nil {
		// Not a stream - decrypt string values in dictionary
		// For now, just decrypt the entire content (simplified approach)
		// TODO: Parse dictionary properly and decrypt only string values
		decrypted, err := encrypt.DecryptObject(content, objNum, genNum, encryptInfo)
		if err == nil {
			content = decrypted
		}
	}

	// Reconstruct full object
	result := fmt.Sprintf("%d %d obj\n", objNum, genNum)
	result += string(content)
	if !bytes.HasSuffix(content, []byte("\n")) {
		result += "\n"
	}
	result += "endobj"

	return []byte(result), nil
}
