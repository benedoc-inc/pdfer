// Package parser provides PDF parsing with byte-perfect reconstruction support
package parse

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// ParsePDFDocument parses a complete PDF preserving all raw bytes for reconstruction
func ParsePDFDocument(pdfBytes []byte) (*PDFDocument, error) {
	if len(pdfBytes) < 8 {
		return nil, fmt.Errorf("PDF too short: %d bytes", len(pdfBytes))
	}

	doc := &PDFDocument{
		RawBytes:  pdfBytes,
		Revisions: make([]*PDFRevision, 0),
	}

	// Parse header
	header, err := ParsePDFHeader(pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse header: %v", err)
	}
	doc.Header = header

	// Find all revision boundaries (%%EOF markers)
	eofOffsets := FindAllEOFMarkers(pdfBytes)
	if len(eofOffsets) == 0 {
		return nil, fmt.Errorf("no %%EOF marker found")
	}

	// Parse each revision
	for i, eofOffset := range eofOffsets {
		rev, err := parseRevision(pdfBytes, i+1, eofOffset, doc)
		if err != nil {
			// Log warning but continue
			continue
		}
		doc.Revisions = append(doc.Revisions, rev)
	}

	if len(doc.Revisions) == 0 {
		return nil, fmt.Errorf("failed to parse any revisions")
	}

	return doc, nil
}

// ParsePDFHeader parses the PDF header with exact bytes preserved
func ParsePDFHeader(pdfBytes []byte) (*PDFHeader, error) {
	if len(pdfBytes) < 8 {
		return nil, fmt.Errorf("PDF too short for header")
	}

	// Check for %PDF-
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return nil, fmt.Errorf("not a PDF: missing %%PDF- header")
	}

	header := &PDFHeader{}

	// Parse version
	versionEnd := 5
	for versionEnd < len(pdfBytes) && versionEnd < 20 {
		if pdfBytes[versionEnd] == '\r' || pdfBytes[versionEnd] == '\n' {
			break
		}
		versionEnd++
	}

	versionStr := string(pdfBytes[5:versionEnd])
	header.Version = versionStr

	// Parse major.minor
	parts := bytes.Split([]byte(versionStr), []byte("."))
	if len(parts) >= 1 {
		header.MajorVersion, _ = strconv.Atoi(string(parts[0]))
	}
	if len(parts) >= 2 {
		header.MinorVersion, _ = strconv.Atoi(string(parts[1]))
	}

	// Find end of header (including binary marker line if present)
	headerEnd := versionEnd

	// Skip line ending after version
	for headerEnd < len(pdfBytes) && (pdfBytes[headerEnd] == '\r' || pdfBytes[headerEnd] == '\n') {
		headerEnd++
	}

	// Check for binary marker (line starting with % followed by high bytes)
	if headerEnd < len(pdfBytes) && pdfBytes[headerEnd] == '%' {
		// Skip binary marker line
		for headerEnd < len(pdfBytes) && pdfBytes[headerEnd] != '\r' && pdfBytes[headerEnd] != '\n' {
			headerEnd++
		}
		// Skip line ending
		for headerEnd < len(pdfBytes) && (pdfBytes[headerEnd] == '\r' || pdfBytes[headerEnd] == '\n') {
			headerEnd++
		}
	}

	header.RawBytes = pdfBytes[:headerEnd]

	return header, nil
}

// parseRevision parses a single revision ending at the given %%EOF offset
func parseRevision(pdfBytes []byte, revNum int, eofOffset int, doc *PDFDocument) (*PDFRevision, error) {
	rev := &PDFRevision{
		Number:    revNum,
		Objects:   make(map[int]*PDFRawObject),
		EOFOffset: int64(eofOffset),
	}

	// Calculate end offset (after %%EOF and trailing newlines)
	endOffset := eofOffset + 5 // len("%%EOF")
	for endOffset < len(pdfBytes) && (pdfBytes[endOffset] == '\r' || pdfBytes[endOffset] == '\n') {
		endOffset++
	}
	rev.EndOffset = int64(endOffset)

	// Find startxref before this %%EOF
	searchStart := eofOffset - 100
	if searchStart < 0 {
		searchStart = 0
	}

	searchSection := string(pdfBytes[searchStart:eofOffset])
	startxrefPattern := regexp.MustCompile(`startxref\s+(\d+)`)
	match := startxrefPattern.FindStringSubmatch(searchSection)
	if match == nil {
		return nil, fmt.Errorf("startxref not found before %%EOF at %d", eofOffset)
	}

	startXRef, _ := strconv.ParseInt(match[1], 10, 64)
	rev.StartXRef = startXRef

	// Parse xref data
	xref, err := ParseXRefDataRaw(pdfBytes, startXRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse xref: %v", err)
	}
	rev.XRef = xref

	// Parse trailer
	trailer, err := ParseTrailerDataRaw(pdfBytes, startXRef, eofOffset)
	if err != nil {
		// Trailer parsing is optional for xref streams
		if xref.Type != XRefTypeStream {
			return nil, fmt.Errorf("failed to parse trailer: %v", err)
		}
	}
	rev.Trailer = trailer

	// Determine which objects belong to this revision
	// For revision 1: all objects from xref that are before this %%EOF
	// For later revisions: objects that appear after previous %%EOF
	prevEnd := int64(0)
	if revNum > 1 && len(doc.Revisions) > 0 {
		prevEnd = doc.Revisions[len(doc.Revisions)-1].EndOffset
	} else {
		prevEnd = int64(len(doc.Header.RawBytes))
	}

	// Parse objects in this revision's xref
	for _, entry := range xref.Entries {
		if !entry.InUse || entry.InObjectStream {
			continue
		}

		// Check if this object belongs to this revision
		// (its offset is after previous revision's end)
		if entry.Offset >= prevEnd && entry.Offset < int64(eofOffset) {
			obj, err := ParseRawObject(pdfBytes, entry.ObjectNum, entry.Generation, entry.Offset)
			if err != nil {
				continue
			}
			rev.Objects[entry.ObjectNum] = obj
		}
	}

	return rev, nil
}

// ParseXRefDataRaw parses cross-reference data preserving raw bytes
func ParseXRefDataRaw(pdfBytes []byte, startXRef int64) (*XRefData, error) {
	if startXRef < 0 || startXRef >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("invalid startxref: %d", startXRef)
	}

	xref := &XRefData{
		Offset:  startXRef,
		Entries: make([]XRefEntry, 0),
	}

	section := pdfBytes[startXRef:]

	// Determine type
	if bytes.HasPrefix(section, []byte("xref")) {
		xref.Type = XRefTypeTable
		return parseTraditionalXRefRaw(pdfBytes, startXRef, xref)
	}

	// Cross-reference stream
	xref.Type = XRefTypeStream
	return parseXRefStreamRaw(pdfBytes, startXRef, xref)
}

// parseTraditionalXRefRaw parses a traditional xref table with raw bytes preserved
func parseTraditionalXRefRaw(pdfBytes []byte, startXRef int64, xref *XRefData) (*XRefData, error) {
	section := pdfBytes[startXRef:]

	// Find end of xref table (at "trailer" keyword)
	trailerIdx := bytes.Index(section, []byte("trailer"))
	if trailerIdx == -1 {
		// No trailer found - look for end of entries
		trailerIdx = min(10000, len(section))
	}

	xref.RawBytes = section[:trailerIdx]

	// Parse entries
	lines := bytes.Split(xref.RawBytes, []byte("\n"))

	currentObjNum := 0
	inSubsection := false

	for i, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Skip "xref" keyword
		if i == 0 && bytes.Equal(line, []byte("xref")) {
			continue
		}

		// Check for subsection header (two numbers)
		parts := bytes.Fields(line)
		if len(parts) == 2 {
			firstObj, err1 := strconv.Atoi(string(parts[0]))
			_, err2 := strconv.Atoi(string(parts[1]))
			if err1 == nil && err2 == nil {
				currentObjNum = firstObj
				inSubsection = true
				continue
			}
		}

		// Parse entry (20 bytes: offset generation flag)
		if inSubsection && len(parts) >= 3 {
			offset, err1 := strconv.ParseInt(string(parts[0]), 10, 64)
			gen, err2 := strconv.Atoi(string(parts[1]))
			flag := string(parts[2])

			if err1 == nil && err2 == nil {
				entry := XRefEntry{
					ObjectNum:  currentObjNum,
					Generation: gen,
					Offset:     offset,
					InUse:      flag == "n",
					RawBytes:   make([]byte, len(line)),
				}
				copy(entry.RawBytes, line)

				xref.Entries = append(xref.Entries, entry)
				currentObjNum++
			}
		}
	}

	return xref, nil
}

// parseXRefStreamRaw parses a cross-reference stream with raw bytes preserved
func parseXRefStreamRaw(pdfBytes []byte, startXRef int64, xref *XRefData) (*XRefData, error) {
	// Parse the xref stream object
	objPattern := regexp.MustCompile(`(\d+)\s+(\d+)\s+obj`)
	section := pdfBytes[startXRef:]
	match := objPattern.FindSubmatch(section[:min(100, len(section))])

	if match == nil {
		return nil, fmt.Errorf("xref stream object header not found")
	}

	objNum, _ := strconv.Atoi(string(match[1]))
	objGen, _ := strconv.Atoi(string(match[2]))

	// Parse the complete object
	obj, err := ParseRawObject(pdfBytes, objNum, objGen, startXRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse xref stream object: %v", err)
	}

	xref.StreamObject = obj
	xref.RawBytes = obj.RawBytes

	// Parse the xref entries from the stream using existing parser
	result, err := ParseXRefStreamFull(pdfBytes, startXRef, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse xref stream entries: %v", err)
	}

	// Convert to XRefEntry format
	for objNum, offset := range result.Objects {
		xref.Entries = append(xref.Entries, XRefEntry{
			ObjectNum: objNum,
			Offset:    offset,
			InUse:     true,
		})
	}

	for objNum, streamEntry := range result.ObjectStreams {
		xref.Entries = append(xref.Entries, XRefEntry{
			ObjectNum:      objNum,
			InUse:          true,
			InObjectStream: true,
			StreamObjNum:   streamEntry.StreamObjNum,
			IndexInStream:  streamEntry.IndexInStream,
		})
	}

	return xref, nil
}

// ParseTrailerDataRaw parses trailer data preserving raw bytes
func ParseTrailerDataRaw(pdfBytes []byte, startXRef int64, eofOffset int) (*TrailerData, error) {
	section := pdfBytes[startXRef:]
	sectionStr := string(section[:min(5000, len(section))])

	trailer := &TrailerData{}

	// Check if this is an xref stream (trailer info is in stream dictionary)
	if !bytes.HasPrefix(section, []byte("xref")) {
		// XRef stream - parse dictionary from stream object
		trailer.Offset = 0 // No separate trailer

		// Extract trailer-equivalent info from stream dictionary
		if match := regexp.MustCompile(`/Size\s+(\d+)`).FindStringSubmatch(sectionStr); match != nil {
			trailer.Size, _ = strconv.Atoi(match[1])
		}
		if match := regexp.MustCompile(`/Root\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(sectionStr); match != nil {
			trailer.Root = match[1]
		}
		if match := regexp.MustCompile(`/Encrypt\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(sectionStr); match != nil {
			trailer.Encrypt = match[1]
		}
		if match := regexp.MustCompile(`/Info\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(sectionStr); match != nil {
			trailer.Info = match[1]
		}
		if match := regexp.MustCompile(`/Prev\s+(\d+)`).FindStringSubmatch(sectionStr); match != nil {
			trailer.Prev, _ = strconv.ParseInt(match[1], 10, 64)
		}

		// Find dictionary boundaries for RawBytes
		dictStart := bytes.Index(section, []byte("<<"))
		if dictStart != -1 {
			// Find matching >>
			depth := 1
			dictEnd := dictStart + 2
			for dictEnd < len(section) && depth > 0 {
				if dictEnd+1 < len(section) && section[dictEnd] == '<' && section[dictEnd+1] == '<' {
					depth++
					dictEnd++
				} else if dictEnd+1 < len(section) && section[dictEnd] == '>' && section[dictEnd+1] == '>' {
					depth--
					dictEnd++
				}
				dictEnd++
			}
			trailer.RawBytes = section[dictStart:dictEnd]
		}

		return trailer, nil
	}

	// Traditional trailer
	trailerIdx := bytes.Index(section, []byte("trailer"))
	if trailerIdx == -1 {
		return nil, fmt.Errorf("trailer keyword not found")
	}

	trailer.Offset = startXRef + int64(trailerIdx)

	// Find end of trailer (at startxref keyword)
	startxrefIdx := bytes.Index(section[trailerIdx:], []byte("startxref"))
	if startxrefIdx == -1 {
		startxrefIdx = len(section) - trailerIdx
	}

	trailer.RawBytes = section[trailerIdx : trailerIdx+startxrefIdx]

	// Parse trailer values
	trailerStr := string(trailer.RawBytes)

	if match := regexp.MustCompile(`/Size\s+(\d+)`).FindStringSubmatch(trailerStr); match != nil {
		trailer.Size, _ = strconv.Atoi(match[1])
	}
	if match := regexp.MustCompile(`/Root\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerStr); match != nil {
		trailer.Root = match[1]
	}
	if match := regexp.MustCompile(`/Encrypt\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerStr); match != nil {
		trailer.Encrypt = match[1]
	}
	if match := regexp.MustCompile(`/Info\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerStr); match != nil {
		trailer.Info = match[1]
	}
	if match := regexp.MustCompile(`/Prev\s+(\d+)`).FindStringSubmatch(trailerStr); match != nil {
		trailer.Prev, _ = strconv.ParseInt(match[1], 10, 64)
	}

	return trailer, nil
}

// ParseRawObject parses a PDF object preserving all raw bytes
func ParseRawObject(pdfBytes []byte, objNum int, objGen int, offset int64) (*PDFRawObject, error) {
	if offset < 0 || offset >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("invalid offset: %d", offset)
	}

	obj := &PDFRawObject{
		Number:     objNum,
		Generation: objGen,
		Offset:     offset,
	}

	section := pdfBytes[offset:]

	// Verify object header
	expectedHeader := fmt.Sprintf("%d %d obj", objNum, objGen)
	if !bytes.HasPrefix(section, []byte(expectedHeader)) {
		// Try to find it nearby
		headerPattern := regexp.MustCompile(fmt.Sprintf(`%d\s+%d\s+obj`, objNum, objGen))
		if !headerPattern.Match(section[:min(50, len(section))]) {
			return nil, fmt.Errorf("object header not found at offset %d", offset)
		}
	}

	// Find endobj
	endobjIdx := bytes.Index(section, []byte("endobj"))
	if endobjIdx == -1 {
		return nil, fmt.Errorf("endobj not found for object %d", objNum)
	}

	// Include "endobj" in raw bytes
	endOffset := endobjIdx + 6 // len("endobj")

	obj.RawBytes = section[:endOffset]
	obj.EndOffset = offset + int64(endOffset)

	// Check if this is a stream object
	streamIdx := bytes.Index(obj.RawBytes, []byte("stream"))
	if streamIdx != -1 {
		obj.IsStream = true

		// Find dictionary (before "stream")
		dictStart := bytes.Index(obj.RawBytes, []byte("<<"))
		if dictStart != -1 {
			// Find matching >>
			depth := 1
			dictEnd := dictStart + 2
			for dictEnd < streamIdx && depth > 0 {
				if dictEnd+1 < len(obj.RawBytes) && obj.RawBytes[dictEnd] == '<' && obj.RawBytes[dictEnd+1] == '<' {
					depth++
					dictEnd++
				} else if dictEnd+1 < len(obj.RawBytes) && obj.RawBytes[dictEnd] == '>' && obj.RawBytes[dictEnd+1] == '>' {
					depth--
					dictEnd++
				}
				dictEnd++
			}

			obj.DictStart = dictStart
			obj.DictEnd = dictEnd
			obj.DictRaw = obj.RawBytes[dictStart:dictEnd]
		}

		// Find stream data boundaries
		// Stream keyword followed by single EOL
		streamDataStart := streamIdx + 6 // len("stream")
		if streamDataStart < len(obj.RawBytes) && obj.RawBytes[streamDataStart] == '\r' {
			streamDataStart++
		}
		if streamDataStart < len(obj.RawBytes) && obj.RawBytes[streamDataStart] == '\n' {
			streamDataStart++
		}

		// Find endstream
		endstreamIdx := bytes.Index(obj.RawBytes[streamDataStart:], []byte("endstream"))
		if endstreamIdx != -1 {
			streamDataEnd := streamDataStart + endstreamIdx
			// Trim trailing EOL before endstream
			if streamDataEnd > streamDataStart && obj.RawBytes[streamDataEnd-1] == '\n' {
				streamDataEnd--
			}
			if streamDataEnd > streamDataStart && obj.RawBytes[streamDataEnd-1] == '\r' {
				streamDataEnd--
			}

			obj.StreamStart = streamDataStart
			obj.StreamEnd = streamDataEnd
			obj.StreamRaw = obj.RawBytes[streamDataStart:streamDataEnd]
		}
	}

	return obj, nil
}

// ParseRawObjectAt is a convenience function to parse an object at a byte offset
// without knowing its object number (extracts from header)
func ParseRawObjectAt(pdfBytes []byte, offset int64) (*PDFRawObject, error) {
	if offset < 0 || offset >= int64(len(pdfBytes)) {
		return nil, fmt.Errorf("invalid offset: %d", offset)
	}

	section := pdfBytes[offset:]

	// Extract object number and generation from header
	headerPattern := regexp.MustCompile(`^(\d+)\s+(\d+)\s+obj`)
	match := headerPattern.FindSubmatch(section[:min(50, len(section))])
	if match == nil {
		return nil, fmt.Errorf("object header not found at offset %d", offset)
	}

	objNum, _ := strconv.Atoi(string(match[1]))
	objGen, _ := strconv.Atoi(string(match[2]))

	return ParseRawObject(pdfBytes, objNum, objGen, offset)
}
