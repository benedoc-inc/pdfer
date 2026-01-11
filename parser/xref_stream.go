package parser

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseXRefStream parses a PDF cross-reference stream
// Cross-reference streams are compressed and may be encrypted
// They contain object offsets in binary format
func ParseXRefStream(pdfBytes []byte, startXRef int64) (map[int]int64, error) {
	return ParseXRefStreamWithEncryption(pdfBytes, startXRef, nil, false)
}

// ParseXRefStreamWithEncryption parses a PDF cross-reference stream with optional decryption
func ParseXRefStreamWithEncryption(pdfBytes []byte, startXRef int64, encryptInfo *types.PDFEncryption, verbose bool) (map[int]int64, error) {
	objMap := make(map[int]int64)

	// The xref stream object should be at startXRef
	// Find object number - it should be at or near startXRef
	xrefSection := pdfBytes[startXRef:]
	xrefStr := string(xrefSection[:min(500, len(xrefSection))])

	// Find object number - it should be near the start (usually the first thing)
	objPattern := regexp.MustCompile(`(\d+)\s+0\s+obj`)
	objMatch := objPattern.FindStringSubmatch(xrefStr)
	if objMatch == nil {
		// Try searching a bit before startXRef in case of offset issues
		var searchStart int64 = 0
		if startXRef > 100 {
			searchStart = startXRef - 100
		}
		searchSection := pdfBytes[searchStart : startXRef+500]
		searchStr := string(searchSection)
		objMatch = objPattern.FindStringSubmatch(searchStr)
		if objMatch == nil {
			return nil, fmt.Errorf("xref stream object not found")
		}
	}

	// Find the stream dictionary
	dictStart := bytes.Index(xrefSection, []byte("<<"))
	if dictStart == -1 {
		return nil, fmt.Errorf("xref stream dictionary not found")
	}

	// Find /Size entry (total number of objects)
	sizePattern := regexp.MustCompile(`/Size\s+(\d+)`)
	sizeMatch := sizePattern.FindStringSubmatch(xrefStr[dictStart:])
	if sizeMatch == nil {
		return nil, fmt.Errorf("/Size not found in xref stream")
	}

	// Find /W entry (field widths: [w1 w2 w3])
	wPattern := regexp.MustCompile(`/W\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	wMatch := wPattern.FindStringSubmatch(xrefStr[dictStart:])
	if wMatch == nil {
		return nil, fmt.Errorf("/W not found in xref stream")
	}

	w1, _ := strconv.Atoi(wMatch[1]) // Type field width
	w2, _ := strconv.Atoi(wMatch[2]) // Field 2 width (usually offset)
	w3, _ := strconv.Atoi(wMatch[3]) // Field 3 width (usually generation)

	// Find /Index entry if present (subsections)
	// Format: /Index [first1 count1 first2 count2 ...]
	// /Index defines which object numbers are in this xref stream
	indexPattern := regexp.MustCompile(`/Index\s*\[\s*([^\]]+)\]`)
	indexMatch := indexPattern.FindStringSubmatch(xrefStr[dictStart:])

	var subsections []struct{ first, count int }
	if indexMatch != nil {
		// Parse index array - this tells us which object numbers are in this stream
		indexFields := strings.Fields(indexMatch[1])
		for i := 0; i < len(indexFields)-1; i += 2 {
			first, _ := strconv.Atoi(indexFields[i])
			count, _ := strconv.Atoi(indexFields[i+1])
			subsections = append(subsections, struct{ first, count int }{first, count})
		}
	} else {
		// Default: single subsection starting at 0, up to /Size
		size, _ := strconv.Atoi(sizeMatch[1])
		subsections = []struct{ first, count int }{{0, size}}
	}

	// Find stream content
	// Look for "stream" after the dictionary
	streamKeywordPos := bytes.Index(xrefSection[dictStart:], []byte("stream"))
	if streamKeywordPos == -1 {
		return nil, fmt.Errorf("stream keyword not found")
	}
	streamKeywordStart := dictStart + streamKeywordPos

	// Skip "stream" keyword (6 bytes) and any whitespace
	streamDataStart := streamKeywordStart + 6
	for streamDataStart < len(xrefSection) && (xrefSection[streamDataStart] == '\r' || xrefSection[streamDataStart] == '\n' || xrefSection[streamDataStart] == ' ' || xrefSection[streamDataStart] == '\t') {
		streamDataStart++
	}

	// Find endstream
	streamEndPos := bytes.Index(xrefSection[streamDataStart:], []byte("endstream"))
	if streamEndPos == -1 {
		return nil, fmt.Errorf("endstream not found")
	}

	// Extract stream content (raw bytes, don't trim)
	streamContent := xrefSection[streamDataStart : streamDataStart+streamEndPos]

	// Note: Xref streams are typically NOT encrypted (they're needed to decrypt other objects)
	// The stream content should be FlateDecode compressed binary data
	// FlateDecode can be either raw deflate or zlib-wrapped

	// Try zlib decompression first (FlateDecode is usually zlib-wrapped)
	// If that fails, try raw deflate
	var decompressed []byte
	var err error

	zlibReader, zlibErr := zlib.NewReader(bytes.NewReader(streamContent))
	if zlibErr == nil {
		decompressed, err = io.ReadAll(zlibReader)
		zlibReader.Close()
		if err != nil {
			// zlib read failed, try raw deflate
			flateReader := flate.NewReader(bytes.NewReader(streamContent))
			decompressed, err = io.ReadAll(flateReader)
			flateReader.Close()
		}
	} else {
		// zlib.NewReader failed (not zlib format), try raw deflate
		flateReader := flate.NewReader(bytes.NewReader(streamContent))
		decompressed, err = io.ReadAll(flateReader)
		flateReader.Close()
	}

	if err != nil {
		return nil, fmt.Errorf("error decompressing xref stream (tried zlib and deflate): %v", err)
	}

	// Parse decompressed stream data
	// Each entry is w1+w2+w3 bytes
	entrySize := w1 + w2 + w3
	if entrySize == 0 {
		return nil, fmt.Errorf("invalid entry size: w1=%d, w2=%d, w3=%d", w1, w2, w3)
	}

	entryIndex := 0

	for _, sub := range subsections {
		for objNum := sub.first; objNum < sub.first+sub.count; objNum++ {
			if entryIndex*entrySize+entrySize > len(decompressed) {
				break
			}

			entry := decompressed[entryIndex*entrySize : entryIndex*entrySize+entrySize]

			// Parse entry based on type (first w1 bytes)
			typeVal := 0
			for i := 0; i < w1 && i < len(entry); i++ {
				typeVal = typeVal<<8 | int(entry[i])
			}

			// Calculate offset for all types (field 2)
			offset := int64(0)
			for i := w1; i < w1+w2 && i < len(entry); i++ {
				offset = offset<<8 | int64(entry[i])
			}

			if typeVal == 1 {
				// Type 1: uncompressed, in-use object
				if offset > 0 {
					objMap[objNum] = offset
				}
			} else if typeVal == 2 {
				// Type 2: compressed object (object stream)
				// Field 2 is object stream number, field 3 is index within stream
				// For compressed objects, we'd need to parse the object stream
				// For now, skip - would need object stream parsing
			} else if typeVal == 0 {
				// Type 0: free object
				// However, some PDFs incorrectly mark in-use objects as type 0
				// If offset looks valid, include it (object 212 is type 0 but has offset 66048)
				if offset > 100 && offset < int64(len(pdfBytes)) {
					objMap[objNum] = offset
				}
			}

			entryIndex++
		}
	}

	if len(objMap) == 0 {
		return nil, fmt.Errorf("no objects found in xref stream")
	}

	return objMap, nil
}
