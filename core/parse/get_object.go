package parse

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/types"
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

	// Walk the full revision chain (last startxref, /Prev links, newest entry
	// wins). Anything else misreads incrementally-updated files: the first
	// startxref in the file belongs to the OLDEST revision.
	incParser := newIncrementalParser(pdfBytes, verbose)
	if err := incParser.parse(); err == nil {
		if entry, ok := incParser.getObjectStreamMap()[objNum]; ok {
			if verbose {
				log.Printf("Object %d is in object stream %d at index %d", objNum, entry.StreamObjNum, entry.IndexInStream)
			}
			return &ObjectLocation{
				IsDirect:      false,
				StreamObjNum:  entry.StreamObjNum,
				IndexInStream: entry.IndexInStream,
			}, nil
		}
		if offset, ok := incParser.getObjectMap()[objNum]; ok {
			if verbose {
				log.Printf("Object %d at byte offset %d (from merged xref chain)", objNum, offset)
			}
			return &ObjectLocation{IsDirect: true, ByteOffset: offset}, nil
		}
	} else if verbose {
		log.Printf("Incremental xref parse failed (%v), falling back to single-section parse", err)
	}

	// Fallback: parse only the section at the first startxref (legacy behavior
	// for files the chain walker cannot handle).
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
	// Take the LAST match: with incremental updates the newest definition of an
	// object appears later in the file.
	pattern := regexp.MustCompile(fmt.Sprintf(`(^|\s|[\r\n])%d\s+0\s+obj`, objNum))
	allMatches := pattern.FindAllIndex(pdfBytes, -1)
	if len(allMatches) > 0 {
		matches := allMatches[len(allMatches)-1]
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

// GetObjectContent returns the dict/stream body of an object, without the
// "N G obj" header or "endobj" footer. This is consistent with objects stored
// in object streams, which also have no per-object header bytes.
//
// Use this when you need to inspect or modify an object's content and then
// re-write it — typically in incremental-update paths where the caller supplies
// its own "N G obj\n" / "\nendobj\n" wrapper.
func GetObjectContent(pdfBytes []byte, objNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	loc, err := FindObjectLocation(pdfBytes, objNum, verbose)
	if err != nil {
		return nil, fmt.Errorf("object %d not found: %v", objNum, err)
	}
	if !loc.IsDirect {
		return GetObjectFromStream(pdfBytes, objNum, loc.StreamObjNum, loc.IndexInStream, encryptInfo, verbose)
	}
	content, _, err := extractDirectObjectContent(pdfBytes, objNum, loc.ByteOffset, encryptInfo, verbose)
	return content, err
}

// GetDirectObject reads a PDF object at a specific byte offset and returns the
// complete object bytes including the "N G obj\n" header and "endobj" footer.
//
// Most callers (parsers, extractors) should use this form. Code that needs to
// modify an object and re-write it in an incremental update should use
// GetObjectContent instead.
func GetDirectObject(pdfBytes []byte, objNum int, offset int64, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	content, genNum, err := extractDirectObjectContent(pdfBytes, objNum, offset, encryptInfo, verbose)
	if err != nil {
		return nil, err
	}
	// Reconstruct full "N G obj\n…\nendobj" bytes.
	var result []byte
	result = append(result, fmt.Sprintf("%d %d obj\n", objNum, genNum)...)
	result = append(result, content...)
	if !bytes.HasSuffix(content, []byte("\n")) {
		result = append(result, '\n')
	}
	result = append(result, "endobj"...)
	return result, nil
}

// extractDirectObjectContent is the shared implementation used by both
// GetDirectObject (returns full bytes) and GetObjectContent (returns body only).
// It returns the dict/stream body and the generation number.
func extractDirectObjectContent(pdfBytes []byte, objNum int, offset int64, encryptInfo *types.PDFEncryption, verbose bool) (content []byte, genNum int, err error) {
	if offset < 0 || offset >= int64(len(pdfBytes)) {
		return nil, 0, fmt.Errorf("invalid offset %d for object %d", offset, objNum)
	}

	objData := pdfBytes[offset:]

	// Verify object header
	headerPattern := regexp.MustCompile(fmt.Sprintf(`^%d\s+(\d+)\s+obj`, objNum))
	headerMatch := headerPattern.FindSubmatch(objData[:min(50, len(objData))])

	if headerMatch == nil {
		// Header not exactly at offset - try nearby
		searchArea := pdfBytes[max(0, int(offset)-100):min(len(pdfBytes), int(offset)+100)]
		pattern := regexp.MustCompile(fmt.Sprintf(`%d\s+(\d+)\s+obj`, objNum))
		match := pattern.FindIndex(searchArea)
		if match == nil {
			if verbose {
				log.Printf("Object %d header not found near offset %d", objNum, offset)
			}
		} else {
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
		return nil, 0, fmt.Errorf("endobj not found for object %d", objNum)
	}

	// Skip past "N G obj" header to the content
	contentStart := bytes.Index(objData[:endobjPos], []byte("obj"))
	if contentStart == -1 {
		contentStart = 0
	} else {
		contentStart += 3 // skip "obj"
		for contentStart < endobjPos && (objData[contentStart] == ' ' || objData[contentStart] == '\n' || objData[contentStart] == '\r' || objData[contentStart] == '\t') {
			contentStart++
		}
	}

	content = objData[contentStart:endobjPos]

	// Stream object: trim to endstream and optionally decrypt.
	streamStart := bytes.Index(content, []byte("stream"))
	if streamStart != -1 {
		endstreamPos := bytes.Index(content, []byte("endstream"))
		if endstreamPos == -1 {
			endstreamPos = len(content)
		}
		content = content[:endstreamPos+len("endstream")]

		if encryptInfo != nil {
			dictPart := content[:streamStart]
			// Stream dictionaries can carry string values too (e.g. embedded-file
			// /Desc or /Params dates); decrypt them like any other dict strings.
			// The /Encrypt dictionary itself is exempt: its strings (/O, /U, ...)
			// are never encrypted (ISO 32000-1 §7.6.5).
			if objNum != encryptInfo.EncryptObjNum {
				if decryptedDict, decErr := encrypt.DecryptStringsInContent(dictPart, objNum, genNum, encryptInfo); decErr == nil {
					dictPart = decryptedDict
				}
			}
			lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
			lengthMatch := lengthPattern.FindSubmatch(dictPart)

			var streamLength int
			if lengthMatch != nil {
				streamLength, _ = strconv.Atoi(string(lengthMatch[1]))
			}

			streamDataStart := streamStart + 6
			if streamDataStart < len(content) && content[streamDataStart] == '\r' {
				streamDataStart++
			}
			if streamDataStart < len(content) && content[streamDataStart] == '\n' {
				streamDataStart++
			}

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
			decryptedStream, decErr := encrypt.DecryptObject(streamData, objNum, genNum, encryptInfo)
			if decErr == nil {
				if verbose {
					log.Printf("Decryption successful: %d -> %d bytes", len(streamData), len(decryptedStream))
				}
				newLength := fmt.Sprintf("/Length %d", len(decryptedStream))
				dictPart = lengthPattern.ReplaceAll(dictPart, []byte(newLength))
				newContent := make([]byte, 0, len(dictPart)+len(decryptedStream)+20)
				newContent = append(newContent, dictPart...)
				newContent = append(newContent, []byte("stream\n")...)
				newContent = append(newContent, decryptedStream...)
				newContent = append(newContent, []byte("\nendstream")...)
				content = newContent
			} else if verbose {
				log.Printf("Decryption failed: %v", decErr)
			}
		}
	} else if encryptInfo != nil && objNum != encryptInfo.EncryptObjNum {
		// Dictionary object — decrypt individual string values. The /Encrypt
		// dictionary itself is exempt: its strings (/O, /U, ...) are never
		// encrypted (ISO 32000-1 §7.6.5).
		decrypted, decErr := encrypt.DecryptStringsInContent(content, objNum, genNum, encryptInfo)
		if decErr == nil {
			content = decrypted
		}
	}

	return content, genNum, nil
}
