package parse

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/types"
)

// ObjectStreamEntry represents an object stored in an object stream (Type 2 xref entry)
type ObjectStreamEntry struct {
	StreamObjNum  int // The object stream that contains this object
	IndexInStream int // Index within the object stream
}

// XRefResult contains both regular object offsets and object stream entries
type XRefResult struct {
	// Regular objects (Type 1): objNum -> byte offset
	Objects map[int]int64
	// Objects in object streams (Type 2): objNum -> ObjectStreamEntry
	ObjectStreams map[int]ObjectStreamEntry
}

// ParseXRefStreamFull parses a PDF cross-reference stream and returns both regular and compressed object info
func ParseXRefStreamFull(pdfBytes []byte, startXRef int64, verbose bool) (*XRefResult, error) {
	result := &XRefResult{
		Objects:       make(map[int]int64),
		ObjectStreams: make(map[int]ObjectStreamEntry),
	}

	// The xref stream object should be at startXRef
	xrefSection := pdfBytes[startXRef:]
	xrefStr := string(xrefSection[:min(500, len(xrefSection))])

	// Find object number - it should be near the start
	objPattern := regexp.MustCompile(`(\d+)\s+0\s+obj`)
	objMatch := objPattern.FindStringSubmatch(xrefStr)
	if objMatch == nil {
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

	// Find /Size entry
	sizePattern := regexp.MustCompile(`/Size\s+(\d+)`)
	sizeMatch := sizePattern.FindStringSubmatch(xrefStr[dictStart:])
	if sizeMatch == nil {
		return nil, fmt.Errorf("/Size not found in xref stream")
	}

	// Find /W entry (field widths)
	wPattern := regexp.MustCompile(`/W\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	wMatch := wPattern.FindStringSubmatch(xrefStr[dictStart:])
	if wMatch == nil {
		return nil, fmt.Errorf("/W not found in xref stream")
	}

	w1, _ := strconv.Atoi(wMatch[1])
	w2, _ := strconv.Atoi(wMatch[2])
	w3, _ := strconv.Atoi(wMatch[3])

	// Find /Index entry
	indexPattern := regexp.MustCompile(`/Index\s*\[\s*([^\]]+)\]`)
	indexMatch := indexPattern.FindStringSubmatch(xrefStr[dictStart:])

	var subsections []struct{ first, count int }
	if indexMatch != nil {
		indexFields := strings.Fields(indexMatch[1])
		for i := 0; i < len(indexFields)-1; i += 2 {
			first, _ := strconv.Atoi(indexFields[i])
			count, _ := strconv.Atoi(indexFields[i+1])
			subsections = append(subsections, struct{ first, count int }{first, count})
		}
	} else {
		size, _ := strconv.Atoi(sizeMatch[1])
		subsections = []struct{ first, count int }{{0, size}}
	}

	// Find and decompress stream content
	streamKeywordPos := bytes.Index(xrefSection[dictStart:], []byte("stream"))
	if streamKeywordPos == -1 {
		return nil, fmt.Errorf("stream keyword not found")
	}
	streamKeywordStart := dictStart + streamKeywordPos

	streamDataStart := streamKeywordStart + 6
	for streamDataStart < len(xrefSection) && (xrefSection[streamDataStart] == '\r' || xrefSection[streamDataStart] == '\n' || xrefSection[streamDataStart] == ' ' || xrefSection[streamDataStart] == '\t') {
		streamDataStart++
	}

	streamEndPos := bytes.Index(xrefSection[streamDataStart:], []byte("endstream"))
	if streamEndPos == -1 {
		return nil, fmt.Errorf("endstream not found")
	}

	streamContent := xrefSection[streamDataStart : streamDataStart+streamEndPos]

	// Decompress (xref streams are NOT encrypted)
	var decompressed []byte
	var err error

	zlibReader, zlibErr := zlib.NewReader(bytes.NewReader(streamContent))
	if zlibErr == nil {
		decompressed, err = io.ReadAll(zlibReader)
		zlibReader.Close()
		if err != nil {
			flateReader := flate.NewReader(bytes.NewReader(streamContent))
			decompressed, err = io.ReadAll(flateReader)
			flateReader.Close()
		}
	} else {
		flateReader := flate.NewReader(bytes.NewReader(streamContent))
		decompressed, err = io.ReadAll(flateReader)
		flateReader.Close()
	}

	if err != nil {
		return nil, fmt.Errorf("error decompressing xref stream: %v", err)
	}

	// Check for predictor (PNG filter)
	// Look for /DecodeParms with /Predictor
	decodeparmsPattern := regexp.MustCompile(`/DecodeParms\s*<<([^>]+)>>`)
	decodeparmsMatch := decodeparmsPattern.FindStringSubmatch(string(xrefSection[:min(500, len(xrefSection))]))

	if decodeparmsMatch != nil {
		predictorPattern := regexp.MustCompile(`/Predictor\s+(\d+)`)
		columnsPattern := regexp.MustCompile(`/Columns\s+(\d+)`)

		predictorMatch := predictorPattern.FindStringSubmatch(decodeparmsMatch[1])
		columnsMatch := columnsPattern.FindStringSubmatch(decodeparmsMatch[1])

		if predictorMatch != nil && columnsMatch != nil {
			predictor, _ := strconv.Atoi(predictorMatch[1])
			columns, _ := strconv.Atoi(columnsMatch[1])

			if predictor >= 10 && predictor <= 15 {
				// PNG predictor - apply filter
				rowSize := columns + 1 // +1 for filter byte
				numRows := len(decompressed) / rowSize

				decoded := make([]byte, numRows*columns)
				prevRow := make([]byte, columns)

				for row := 0; row < numRows; row++ {
					rowStart := row * rowSize
					if rowStart+rowSize > len(decompressed) {
						break
					}
					filterByte := decompressed[rowStart]
					rowData := decompressed[rowStart+1 : rowStart+rowSize]

					switch filterByte {
					case 0: // None
						copy(decoded[row*columns:], rowData)
					case 1: // Sub
						for i := 0; i < columns; i++ {
							left := byte(0)
							if i > 0 {
								left = decoded[row*columns+i-1]
							}
							decoded[row*columns+i] = rowData[i] + left
						}
					case 2: // Up (most common for Predictor 12)
						for i := 0; i < columns; i++ {
							decoded[row*columns+i] = rowData[i] + prevRow[i]
						}
					default:
						copy(decoded[row*columns:], rowData)
					}

					copy(prevRow, decoded[row*columns:row*columns+columns])
				}

				decompressed = decoded
			}
		}
	}

	// Parse entries
	entrySize := w1 + w2 + w3
	if entrySize == 0 {
		return nil, fmt.Errorf("invalid entry size")
	}

	entryIndex := 0
	for _, sub := range subsections {
		for objNum := sub.first; objNum < sub.first+sub.count; objNum++ {
			if entryIndex*entrySize+entrySize > len(decompressed) {
				break
			}

			entry := decompressed[entryIndex*entrySize : entryIndex*entrySize+entrySize]

			// Parse type (first w1 bytes)
			typeVal := 0
			for i := 0; i < w1 && i < len(entry); i++ {
				typeVal = typeVal<<8 | int(entry[i])
			}

			// Parse field 2 (offset for Type 1, stream object for Type 2)
			field2 := int64(0)
			for i := w1; i < w1+w2 && i < len(entry); i++ {
				field2 = field2<<8 | int64(entry[i])
			}

			// Parse field 3 (generation for Type 1, index for Type 2)
			field3 := int64(0)
			for i := w1 + w2; i < w1+w2+w3 && i < len(entry); i++ {
				field3 = field3<<8 | int64(entry[i])
			}

			switch typeVal {
			case 0:
				// Type 0: free object - skip
			case 1:
				// Type 1: uncompressed, in-use object
				// field2 = byte offset, field3 = generation
				if field2 > 0 {
					result.Objects[objNum] = field2
				}
			case 2:
				// Type 2: compressed object in object stream
				// field2 = object stream number, field3 = index within stream
				result.ObjectStreams[objNum] = ObjectStreamEntry{
					StreamObjNum:  int(field2),
					IndexInStream: int(field3),
				}
			}

			entryIndex++
		}
	}

	if verbose {
		fmt.Printf("Parsed xref stream: %d direct objects, %d objects in streams\n",
			len(result.Objects), len(result.ObjectStreams))
	}

	return result, nil
}

// GetObjectFromStream extracts an object from an object stream (ObjStm)
// This implements PyPDF's _get_object_from_stream method
func GetObjectFromStream(pdfBytes []byte, objNum int, streamObjNum int, indexInStream int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Step 1: Get the object stream itself
	// First, we need to find the object stream's byte offset
	// This requires having the xref table for direct objects

	// For now, search for the object stream in the PDF
	streamPattern := fmt.Sprintf(`%d\s+0\s+obj`, streamObjNum)
	re := regexp.MustCompile(streamPattern)
	matches := re.FindIndex(pdfBytes)
	if matches == nil {
		return nil, fmt.Errorf("object stream %d not found", streamObjNum)
	}

	streamObjStart := matches[0]

	// Find the object stream's dictionary and stream data
	objSection := pdfBytes[streamObjStart:]

	// Find dictionary
	dictStart := bytes.Index(objSection, []byte("<<"))
	if dictStart == -1 {
		return nil, fmt.Errorf("object stream dictionary not found")
	}

	// Find dictionary end to get dict content
	dictEnd := bytes.Index(objSection[dictStart:], []byte(">>"))
	if dictEnd == -1 {
		return nil, fmt.Errorf("dictionary end not found")
	}
	dictContent := string(objSection[dictStart : dictStart+dictEnd+2])

	// Get /Length from dictionary
	lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
	lengthMatch := lengthPattern.FindStringSubmatch(dictContent)
	if lengthMatch == nil {
		return nil, fmt.Errorf("/Length not found in object stream dictionary")
	}
	streamLength, _ := strconv.Atoi(lengthMatch[1])

	// Find stream keyword
	streamKeyword := bytes.Index(objSection[dictStart:], []byte("stream"))
	if streamKeyword == -1 {
		return nil, fmt.Errorf("stream keyword not found in object stream")
	}
	streamKeyword += dictStart

	// Skip "stream" and exactly one EOL marker (per PDF spec)
	streamDataStart := streamKeyword + 6
	if streamDataStart < len(objSection) && objSection[streamDataStart] == '\r' {
		streamDataStart++
	}
	if streamDataStart < len(objSection) && objSection[streamDataStart] == '\n' {
		streamDataStart++
	}

	// Use /Length to get exact stream data
	if streamDataStart+streamLength > len(objSection) {
		return nil, fmt.Errorf("stream length %d exceeds available data", streamLength)
	}

	streamData := objSection[streamDataStart : streamDataStart+streamLength]

	// Decrypt stream data if needed (object streams ARE encrypted)
	if encryptInfo != nil {
		decrypted, err := encrypt.DecryptObject(streamData, streamObjNum, 0, encryptInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt object stream: %v", err)
		}
		streamData = decrypted
	}

	// Decompress stream data (FlateDecode)
	var decompressed []byte
	var err error

	zlibReader, zlibErr := zlib.NewReader(bytes.NewReader(streamData))
	if zlibErr == nil {
		decompressed, err = io.ReadAll(zlibReader)
		zlibReader.Close()
	}

	if err != nil || zlibErr != nil {
		// Try raw deflate
		flateReader := flate.NewReader(bytes.NewReader(streamData))
		decompressed, err = io.ReadAll(flateReader)
		flateReader.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to decompress object stream: %v", err)
		}
	}

	// Parse object stream header
	// Object stream format:
	// - /N = number of objects
	// - /First = byte offset to first object's data
	// - Header format: "objnum1 offset1 objnum2 offset2 ..."

	// Find /N and /First from dictionary
	dictSection := string(objSection[dictStart:streamKeyword])

	nPattern := regexp.MustCompile(`/N\s+(\d+)`)
	nMatch := nPattern.FindStringSubmatch(dictSection)
	if nMatch == nil {
		return nil, fmt.Errorf("/N not found in object stream dictionary")
	}
	_ = nMatch[1] // Number of objects (for validation if needed)

	firstPattern := regexp.MustCompile(`/First\s+(\d+)`)
	firstMatch := firstPattern.FindStringSubmatch(dictSection)
	if firstMatch == nil {
		return nil, fmt.Errorf("/First not found in object stream dictionary")
	}
	firstOffset, _ := strconv.Atoi(firstMatch[1])

	// Parse header: pairs of "objnum offset"
	type objEntry struct {
		objNum int
		offset int
	}
	var entries []objEntry

	headerData := decompressed[:firstOffset]
	headerStr := string(headerData)
	fields := strings.Fields(headerStr)

	for i := 0; i < len(fields)-1; i += 2 {
		on, _ := strconv.Atoi(fields[i])
		off, _ := strconv.Atoi(fields[i+1])
		entries = append(entries, objEntry{objNum: on, offset: off})
	}

	if indexInStream >= len(entries) {
		return nil, fmt.Errorf("index %d out of range (stream has %d objects)", indexInStream, len(entries))
	}

	// Find the object at the specified index
	targetEntry := entries[indexInStream]
	if targetEntry.objNum != objNum {
		// Try to find by object number instead
		found := false
		for _, e := range entries {
			if e.objNum == objNum {
				targetEntry = e
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("object %d not found in object stream (expected at index %d)", objNum, indexInStream)
		}
	}

	// Calculate object data range
	objDataStart := firstOffset + targetEntry.offset
	var objDataEnd int

	// Find next object's offset or end of stream
	if indexInStream < len(entries)-1 {
		nextEntry := entries[indexInStream+1]
		objDataEnd = firstOffset + nextEntry.offset
	} else {
		objDataEnd = len(decompressed)
	}

	if objDataStart >= len(decompressed) || objDataEnd > len(decompressed) {
		return nil, fmt.Errorf("object data offset out of range")
	}

	objectData := decompressed[objDataStart:objDataEnd]

	// Trim trailing whitespace
	objectData = bytes.TrimRight(objectData, " \t\r\n")

	if verbose {
		fmt.Printf("Extracted object %d from stream %d: %d bytes\n", objNum, streamObjNum, len(objectData))
	}

	return objectData, nil
}
