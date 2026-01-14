// Package parser provides PDF parsing functionality including incremental updates
package parse

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// xrefSection represents a single cross-reference section from a PDF
type xrefSection struct {
	StartXRef int64                     // Byte offset where this xref section starts
	Objects   map[int]int64             // Object number -> byte offset (Type 1 entries)
	Streams   map[int]ObjectStreamEntry // Object number -> object stream info (Type 2 entries)
	Prev      int64                     // Offset of previous xref section (from /Prev in trailer)
	Root      string                    // /Root reference from trailer
	Info      string                    // /Info reference from trailer
	Encrypt   string                    // /Encrypt reference from trailer
	Size      int                       // /Size from trailer
}

// incrementalParser handles PDFs with multiple revisions (incremental updates)
type incrementalParser struct {
	pdfBytes      []byte
	sections      []*xrefSection            // Ordered from oldest to newest
	mergedObjs    map[int]int64             // Final merged object map
	mergedStreams map[int]ObjectStreamEntry // Final merged object stream entries
	verbose       bool
}

// newincrementalParser creates a new parser for handling incremental updates
func newIncrementalParser(pdfBytes []byte, verbose bool) *incrementalParser {
	return &incrementalParser{
		pdfBytes:      pdfBytes,
		sections:      make([]*xrefSection, 0),
		mergedObjs:    make(map[int]int64),
		mergedStreams: make(map[int]ObjectStreamEntry),
		verbose:       verbose,
	}
}

// parse parses all xref sections in the PDF and merges them
func (p *incrementalParser) parse() error {
	// Strategy 1: Find startxref before last %%EOF and follow /Prev chain
	// This is the standard approach per PDF spec

	// Find the last startxref
	lastStartXRef, err := p.findLastStartXRef()
	if err != nil {
		return fmt.Errorf("failed to find startxref: %v", err)
	}

	// Parse xref sections by following /Prev chain
	if err := p.parseXRefChain(lastStartXRef); err != nil {
		return fmt.Errorf("failed to parse xref chain: %v", err)
	}

	// Merge all sections (earlier sections first, later ones override)
	p.mergeSections()

	return nil
}

// getObjectMap returns the merged object map
func (p *incrementalParser) getObjectMap() map[int]int64 {
	return p.mergedObjs
}

// getObjectStreamMap returns the merged object stream entries
func (p *incrementalParser) getObjectStreamMap() map[int]ObjectStreamEntry {
	return p.mergedStreams
}

// getFullXRefResult returns both regular objects and object stream entries
func (p *incrementalParser) getFullXRefResult() *XRefResult {
	return &XRefResult{
		Objects:       p.mergedObjs,
		ObjectStreams: p.mergedStreams,
	}
}

// getSections returns all parsed xref sections (oldest first)
func (p *incrementalParser) getSections() []*xrefSection {
	return p.sections
}

// findLastStartXRef finds the startxref value before the last %%EOF
func (p *incrementalParser) findLastStartXRef() (int64, error) {
	// Search backwards from end for %%EOF
	pdfStr := string(p.pdfBytes)

	// Find all %%EOF markers
	eofPattern := regexp.MustCompile(`%%EOF`)
	eofMatches := eofPattern.FindAllStringIndex(pdfStr, -1)

	if len(eofMatches) == 0 {
		return 0, fmt.Errorf("no %%EOF marker found")
	}

	// Take the last %%EOF
	lastEOF := eofMatches[len(eofMatches)-1][0]

	// Search backwards from %%EOF for startxref
	searchStart := lastEOF - 100
	if searchStart < 0 {
		searchStart = 0
	}

	searchSection := pdfStr[searchStart:lastEOF]
	startxrefPattern := regexp.MustCompile(`startxref\s+(\d+)`)
	match := startxrefPattern.FindStringSubmatch(searchSection)

	if match == nil {
		return 0, fmt.Errorf("startxref not found before %%EOF")
	}

	offset, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid startxref value: %s", match[1])
	}

	if p.verbose {
		fmt.Printf("Found last startxref: %d (before %%EOF at %d)\n", offset, lastEOF)
	}

	return offset, nil
}

// parseXRefChain parses xref sections following the /Prev chain
func (p *incrementalParser) parseXRefChain(startXRef int64) error {
	visited := make(map[int64]bool) // Prevent infinite loops
	offsets := []int64{startXRef}

	// First, collect all offsets by following /Prev chain
	currentOffset := startXRef
	for currentOffset > 0 && !visited[currentOffset] {
		visited[currentOffset] = true

		prev, err := p.findPrevOffset(currentOffset)
		if err != nil {
			// No /Prev found, this is the first revision
			break
		}

		if prev > 0 && prev < int64(len(p.pdfBytes)) {
			offsets = append(offsets, prev)
			currentOffset = prev
		} else {
			break
		}
	}

	// Reverse to get oldest first
	for i, j := 0, len(offsets)-1; i < j; i, j = i+1, j-1 {
		offsets[i], offsets[j] = offsets[j], offsets[i]
	}

	if p.verbose {
		fmt.Printf("Found %d xref sections at offsets: %v\n", len(offsets), offsets)
	}

	// Parse each section
	for _, offset := range offsets {
		section, err := p.parsexrefSection(offset)
		if err != nil {
			if p.verbose {
				fmt.Printf("Warning: failed to parse xref at offset %d: %v\n", offset, err)
			}
			continue
		}
		p.sections = append(p.sections, section)
	}

	if len(p.sections) == 0 {
		return fmt.Errorf("no valid xref sections found")
	}

	return nil
}

// findPrevOffset finds the /Prev value in the trailer at the given xref offset
func (p *incrementalParser) findPrevOffset(startXRef int64) (int64, error) {
	if startXRef >= int64(len(p.pdfBytes)) {
		return 0, fmt.Errorf("startXRef out of bounds")
	}

	section := p.pdfBytes[startXRef:]

	// Check if this is an xref stream or traditional xref
	if bytes.HasPrefix(section, []byte("xref")) {
		// Traditional xref - find trailer and /Prev
		trailerIdx := bytes.Index(section, []byte("trailer"))
		if trailerIdx == -1 {
			return 0, fmt.Errorf("no trailer found")
		}

		// Find the end of the trailer dictionary (>>)
		trailerStart := trailerIdx + 7 // len("trailer")
		dictStart := bytes.Index(section[trailerStart:], []byte("<<"))
		if dictStart == -1 {
			return 0, fmt.Errorf("trailer dictionary not found")
		}
		dictStart += trailerStart

		// Find matching >> for trailer dict
		depth := 1
		dictEnd := dictStart + 2
		for dictEnd < len(section) && depth > 0 {
			if dictEnd+1 < len(section) && section[dictEnd] == '<' && section[dictEnd+1] == '<' {
				depth++
				dictEnd++
			} else if dictEnd+1 < len(section) && section[dictEnd] == '>' && section[dictEnd+1] == '>' {
				depth--
				if depth == 0 {
					dictEnd += 2
					break
				}
				dictEnd++
			}
			dictEnd++
		}

		trailerDict := string(section[dictStart:dictEnd])
		prevPattern := regexp.MustCompile(`/Prev\s+(\d+)`)
		match := prevPattern.FindStringSubmatch(trailerDict)
		if match == nil {
			return 0, fmt.Errorf("no /Prev in trailer")
		}

		return strconv.ParseInt(match[1], 10, 64)
	}

	// Cross-reference stream - /Prev is in the stream dictionary
	// Find the dictionary end (before "stream" keyword)
	streamIdx := bytes.Index(section, []byte("stream"))
	if streamIdx == -1 {
		streamIdx = min(2000, len(section))
	}

	dictSection := string(section[:streamIdx])
	prevPattern := regexp.MustCompile(`/Prev\s+(\d+)`)
	match := prevPattern.FindStringSubmatch(dictSection)
	if match == nil {
		return 0, fmt.Errorf("no /Prev in xref stream")
	}

	return strconv.ParseInt(match[1], 10, 64)
}

// parsexrefSection parses a single xref section at the given offset
func (p *incrementalParser) parsexrefSection(startXRef int64) (*xrefSection, error) {
	if startXRef >= int64(len(p.pdfBytes)) {
		return nil, fmt.Errorf("startXRef out of bounds: %d >= %d", startXRef, len(p.pdfBytes))
	}

	xrefData := p.pdfBytes[startXRef:]
	xrefStr := string(xrefData[:min(5000, len(xrefData))])

	// Determine type and parse
	if bytes.HasPrefix(xrefData, []byte("xref")) {
		// Traditional xref table
		return p.parseTraditionalxrefSection(startXRef)
	}

	// Cross-reference stream
	// Check for object header pattern
	if regexp.MustCompile(`^\d+\s+\d+\s+obj`).MatchString(xrefStr) ||
		bytes.Contains(xrefData[:min(100, len(xrefData))], []byte("obj")) {
		return p.parseXRefStreamSection(startXRef)
	}

	return nil, fmt.Errorf("unrecognized xref format at offset %d", startXRef)
}

// parseTraditionalxrefSection parses a traditional xref table
func (p *incrementalParser) parseTraditionalxrefSection(startXRef int64) (*xrefSection, error) {
	section := &xrefSection{
		StartXRef: startXRef,
		Objects:   make(map[int]int64),
		Streams:   make(map[int]ObjectStreamEntry),
	}

	xrefData := p.pdfBytes[startXRef:]
	xrefStr := string(xrefData[:min(10000, len(xrefData))])

	// Parse xref entries
	lines := regexp.MustCompile(`\r?\n`).Split(xrefStr, -1)

	currentObjNum := 0
	inSubsection := false

	for i, line := range lines {
		line = regexp.MustCompile(`^\s+|\s+$`).ReplaceAllString(line, "")
		if line == "" {
			continue
		}

		// First line should be "xref"
		if i == 0 && line == "xref" {
			continue
		}

		// Check for trailer
		if line == "trailer" || bytes.HasPrefix([]byte(line), []byte("trailer")) {
			break
		}

		fields := regexp.MustCompile(`\s+`).Split(line, -1)

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
				section.Objects[currentObjNum] = offset
			}
			currentObjNum++
		}
	}

	// Parse trailer for /Root, /Encrypt, /Prev, /Size
	trailerIdx := bytes.Index(xrefData, []byte("trailer"))
	if trailerIdx != -1 {
		// Find the trailer dictionary bounds
		trailerStart := trailerIdx + 7 // len("trailer")
		dictStart := bytes.Index(xrefData[trailerStart:], []byte("<<"))
		if dictStart != -1 {
			dictStart += trailerStart

			// Find matching >>
			depth := 1
			dictEnd := dictStart + 2
			for dictEnd < len(xrefData) && depth > 0 {
				if dictEnd+1 < len(xrefData) && xrefData[dictEnd] == '<' && xrefData[dictEnd+1] == '<' {
					depth++
					dictEnd++
				} else if dictEnd+1 < len(xrefData) && xrefData[dictEnd] == '>' && xrefData[dictEnd+1] == '>' {
					depth--
					if depth == 0 {
						dictEnd += 2
						break
					}
					dictEnd++
				}
				dictEnd++
			}

			trailerDict := string(xrefData[dictStart:dictEnd])

			if match := regexp.MustCompile(`/Root\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerDict); match != nil {
				section.Root = match[1]
			}
			if match := regexp.MustCompile(`/Info\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerDict); match != nil {
				section.Info = match[1]
			}
			if match := regexp.MustCompile(`/Encrypt\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(trailerDict); match != nil {
				section.Encrypt = match[1]
			}
			if match := regexp.MustCompile(`/Prev\s+(\d+)`).FindStringSubmatch(trailerDict); match != nil {
				section.Prev, _ = strconv.ParseInt(match[1], 10, 64)
			}
			if match := regexp.MustCompile(`/Size\s+(\d+)`).FindStringSubmatch(trailerDict); match != nil {
				section.Size, _ = strconv.Atoi(match[1])
			}
		}
	}

	if p.verbose {
		fmt.Printf("Parsed traditional xref at %d: %d objects, Prev=%d\n",
			startXRef, len(section.Objects), section.Prev)
	}

	return section, nil
}

// parseXRefStreamSection parses a cross-reference stream
func (p *incrementalParser) parseXRefStreamSection(startXRef int64) (*xrefSection, error) {
	section := &xrefSection{
		StartXRef: startXRef,
		Objects:   make(map[int]int64),
		Streams:   make(map[int]ObjectStreamEntry),
	}

	// Use existing full xref stream parser
	result, err := ParseXRefStreamFull(p.pdfBytes, startXRef, p.verbose)
	if err != nil {
		return nil, err
	}

	section.Objects = result.Objects
	section.Streams = result.ObjectStreams

	// Extract trailer info from stream dictionary
	xrefData := p.pdfBytes[startXRef:]
	xrefStr := string(xrefData[:min(2000, len(xrefData))])

	if match := regexp.MustCompile(`/Root\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(xrefStr); match != nil {
		section.Root = match[1]
	}
	if match := regexp.MustCompile(`/Info\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(xrefStr); match != nil {
		section.Info = match[1]
	}
	if match := regexp.MustCompile(`/Encrypt\s+(\d+\s+\d+\s+R)`).FindStringSubmatch(xrefStr); match != nil {
		section.Encrypt = match[1]
	}
	if match := regexp.MustCompile(`/Prev\s+(\d+)`).FindStringSubmatch(xrefStr); match != nil {
		section.Prev, _ = strconv.ParseInt(match[1], 10, 64)
	}
	if match := regexp.MustCompile(`/Size\s+(\d+)`).FindStringSubmatch(xrefStr); match != nil {
		section.Size, _ = strconv.Atoi(match[1])
	}

	if p.verbose {
		fmt.Printf("Parsed xref stream at %d: %d objects, %d in streams, Prev=%d\n",
			startXRef, len(section.Objects), len(section.Streams), section.Prev)
	}

	return section, nil
}

// mergeSections merges all xref sections (older entries first, newer override)
func (p *incrementalParser) mergeSections() {
	// sections are already ordered oldest to newest
	for _, section := range p.sections {
		// Regular objects
		for objNum, offset := range section.Objects {
			p.mergedObjs[objNum] = offset
		}
		// Object stream entries
		for objNum, entry := range section.Streams {
			p.mergedStreams[objNum] = entry
		}
	}

	if p.verbose {
		fmt.Printf("Merged %d sections: %d objects, %d in streams\n",
			len(p.sections), len(p.mergedObjs), len(p.mergedStreams))
	}
}

// FindAllEOFMarkers returns the byte offsets of all %%EOF markers in the PDF
func FindAllEOFMarkers(pdfBytes []byte) []int {
	var offsets []int
	pattern := []byte("%%EOF")

	pos := 0
	for {
		idx := bytes.Index(pdfBytes[pos:], pattern)
		if idx == -1 {
			break
		}
		offsets = append(offsets, pos+idx)
		pos = pos + idx + len(pattern)
	}

	return offsets
}

// CountRevisions returns the number of revisions in the PDF
func CountRevisions(pdfBytes []byte) int {
	return len(FindAllEOFMarkers(pdfBytes))
}

// parseWithIncrementalUpdates parses a PDF handling incremental updates
// Returns the merged object map with all revisions combined
func parseWithIncrementalUpdates(pdfBytes []byte, verbose bool) (*XRefResult, error) {
	parser := newIncrementalParser(pdfBytes, verbose)
	if err := parser.parse(); err != nil {
		return nil, err
	}
	return parser.getFullXRefResult(), nil
}

// parseCrossReferenceTableIncremental is a drop-in replacement for ParseCrossReferenceTable
// that handles incremental updates
func parseCrossReferenceTableIncremental(pdfBytes []byte, verbose bool) (map[int]int64, error) {
	result, err := parseWithIncrementalUpdates(pdfBytes, verbose)
	if err != nil {
		return nil, err
	}

	// Merge object streams into main map for backwards compatibility
	// Note: This loses the object stream information, use parseWithIncrementalUpdates for full info
	merged := make(map[int]int64)
	for k, v := range result.Objects {
		merged[k] = v
	}

	return merged, nil
}

// GetRevisionBoundaries returns the byte offsets where each revision ends (%%EOF positions)
func GetRevisionBoundaries(pdfBytes []byte) []int {
	offsets := FindAllEOFMarkers(pdfBytes)
	// Each %%EOF is followed by a newline, add 5 + 1 for the boundary
	for i := range offsets {
		offsets[i] += 5 // Length of "%%EOF"
	}
	sort.Ints(offsets)
	return offsets
}

// ExtractRevision extracts a specific revision (1-indexed) from the PDF
// Returns the PDF bytes up to and including that revision's %%EOF
func ExtractRevision(pdfBytes []byte, revisionNum int) ([]byte, error) {
	boundaries := GetRevisionBoundaries(pdfBytes)

	if revisionNum < 1 || revisionNum > len(boundaries) {
		return nil, fmt.Errorf("revision %d out of range (1-%d)", revisionNum, len(boundaries))
	}

	endPos := boundaries[revisionNum-1]
	if endPos > len(pdfBytes) {
		endPos = len(pdfBytes)
	}

	// Include trailing newline if present
	for endPos < len(pdfBytes) && (pdfBytes[endPos] == '\r' || pdfBytes[endPos] == '\n') {
		endPos++
	}

	return pdfBytes[:endPos], nil
}
