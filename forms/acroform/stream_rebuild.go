// Package acroform provides object stream rebuilding for form filling
package acroform

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// StreamObjectUpdate tracks updates to objects in a stream
type StreamObjectUpdate struct {
	ObjNum     int
	Index      int
	NewContent []byte
}

// RebuildObjectStream rebuilds an object stream with updated objects
func RebuildObjectStream(pdfBytes []byte, streamObjNum int, updates []StreamObjectUpdate, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Get the object stream
	streamObjData, err := parse.GetObject(pdfBytes, streamObjNum, encryptInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get object stream %d: %w", streamObjNum, err)
	}

	// Find the object stream in the PDF
	pdfStr := string(pdfBytes)
	streamPattern := regexp.MustCompile(fmt.Sprintf(`%d\s+0\s+obj`, streamObjNum))
	streamMatch := streamPattern.FindStringIndex(pdfStr)
	if streamMatch == nil {
		return nil, fmt.Errorf("object stream %d not found", streamObjNum)
	}

	streamStart := streamMatch[0]

	// Parse the object stream to extract all objects
	streamDict, streamData, err := parseObjectStream(streamObjData, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse object stream: %w", err)
	}

	// Create update map
	updateMap := make(map[int]*StreamObjectUpdate)
	for i := range updates {
		updateMap[updates[i].ObjNum] = &updates[i]
	}

	// Rebuild stream with updates
	newStreamData, newHeader, err := rebuildStreamContent(streamDict, streamData, updateMap, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to rebuild stream content: %w", err)
	}

	// Compress the new stream
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	zw.Write(newStreamData)
	zw.Close()

	// Update /First to point to new header length
	newFirst := len(newHeader)
	streamDict["/First"] = newFirst
	streamDict["/Length"] = len(compressed.Bytes())

	// Rebuild object stream dictionary
	newDictStr := formatStreamDict(streamDict)

	// Find stream boundaries in PDF
	dictEnd := bytes.Index(pdfBytes[streamStart:], []byte("stream"))
	if dictEnd == -1 {
		return nil, fmt.Errorf("stream keyword not found")
	}
	dictEnd += streamStart

	// Find endstream
	endstreamPos := bytes.Index(pdfBytes[dictEnd:], []byte("endstream"))
	if endstreamPos == -1 {
		return nil, fmt.Errorf("endstream not found")
	}
	streamEnd := dictEnd + endstreamPos + 9

	// Reconstruct PDF
	before := pdfBytes[:streamStart]
	after := pdfBytes[streamEnd:]

	result := make([]byte, 0, len(before)+len(newDictStr)+len(compressed.Bytes())+len(after)+50)
	result = append(result, before...)
	result = append(result, []byte(fmt.Sprintf("%d 0 obj\n", streamObjNum))...)
	result = append(result, []byte(newDictStr)...)
	result = append(result, []byte("\nstream\n")...)
	result = append(result, compressed.Bytes()...)
	result = append(result, []byte("\nendstream\nendobj\n")...)
	result = append(result, after...)

	if verbose {
		fmt.Printf("Rebuilt object stream %d: %d objects, %d bytes compressed\n", streamObjNum, streamDict["/N"], len(compressed.Bytes()))
	}

	return result, nil
}

// parseObjectStream parses an object stream to extract dictionary and decompressed data
func parseObjectStream(streamObjData []byte, encryptInfo *types.PDFEncryption, verbose bool) (map[string]interface{}, []byte, error) {
	// Find dictionary
	dictStart := bytes.Index(streamObjData, []byte("<<"))
	if dictStart == -1 {
		return nil, nil, fmt.Errorf("dictionary not found")
	}

	dictEnd := bytes.Index(streamObjData[dictStart:], []byte(">>"))
	if dictEnd == -1 {
		return nil, nil, fmt.Errorf("dictionary end not found")
	}
	dictEnd += dictStart + 2

	dictStr := string(streamObjData[dictStart:dictEnd])
	dict := parseStreamDict(dictStr)

	// Find stream data
	streamKeyword := bytes.Index(streamObjData[dictEnd:], []byte("stream"))
	if streamKeyword == -1 {
		return nil, nil, fmt.Errorf("stream keyword not found")
	}

	streamDataStart := dictEnd + streamKeyword + 6
	// Skip EOL
	if streamDataStart < len(streamObjData) && (streamObjData[streamDataStart] == '\r' || streamObjData[streamDataStart] == '\n') {
		streamDataStart++
	}
	if streamDataStart < len(streamObjData) && streamObjData[streamDataStart] == '\n' {
		streamDataStart++
	}

	// Get stream length
	length, ok := dict["/Length"].(int)
	if !ok {
		return nil, nil, fmt.Errorf("/Length not found or invalid")
	}

	streamData := streamObjData[streamDataStart : streamDataStart+length]

	// Decrypt if needed
	if encryptInfo != nil {
		decrypted, err := encrypt.DecryptObject(streamData, 0, 0, encryptInfo)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decrypt stream: %w", err)
		}
		streamData = decrypted
	}

	// Decompress
	var decompressed bytes.Buffer
	zr, err := zlib.NewReader(bytes.NewReader(streamData))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	decompressed.ReadFrom(zr)
	zr.Close()

	return dict, decompressed.Bytes(), nil
}

// parseStreamDict parses a stream dictionary string
func parseStreamDict(dictStr string) map[string]interface{} {
	dict := make(map[string]interface{})

	// Extract /N
	if nMatch := regexp.MustCompile(`/N\s+(\d+)`).FindStringSubmatch(dictStr); nMatch != nil {
		if n, err := strconv.Atoi(nMatch[1]); err == nil {
			dict["/N"] = n
		}
	}

	// Extract /First
	if firstMatch := regexp.MustCompile(`/First\s+(\d+)`).FindStringSubmatch(dictStr); firstMatch != nil {
		if first, err := strconv.Atoi(firstMatch[1]); err == nil {
			dict["/First"] = first
		}
	}

	// Extract /Length
	if lenMatch := regexp.MustCompile(`/Length\s+(\d+)`).FindStringSubmatch(dictStr); lenMatch != nil {
		if length, err := strconv.Atoi(lenMatch[1]); err == nil {
			dict["/Length"] = length
		}
	}

	return dict
}

// rebuildStreamContent rebuilds stream content with updated objects
func rebuildStreamContent(streamDict map[string]interface{}, streamData []byte, updates map[int]*StreamObjectUpdate, verbose bool) ([]byte, []byte, error) {
	n, ok := streamDict["/N"].(int)
	if !ok {
		return nil, nil, fmt.Errorf("/N not found in stream dict")
	}

	first, ok := streamDict["/First"].(int)
	if !ok {
		return nil, nil, fmt.Errorf("/First not found in stream dict")
	}

	// Parse header
	headerData := streamData[:first]
	headerStr := string(headerData)
	fields := strings.Fields(headerStr)

	type objEntry struct {
		objNum int
		offset int
	}
	entries := make([]objEntry, 0, n)

	for i := 0; i < len(fields)-1 && len(entries) < n; i += 2 {
		on, _ := strconv.Atoi(fields[i])
		off, _ := strconv.Atoi(fields[i+1])
		entries = append(entries, objEntry{objNum: on, offset: off})
	}

	// Extract all objects
	objects := make(map[int][]byte)
	for i, entry := range entries {
		objStart := first + entry.offset
		var objEnd int
		if i < len(entries)-1 {
			objEnd = first + entries[i+1].offset
		} else {
			objEnd = len(streamData)
		}

		objData := streamData[objStart:objEnd]
		objData = bytes.TrimRight(objData, " \t\r\n")
		objects[entry.objNum] = objData
	}

	// Apply updates
	for objNum, update := range updates {
		objects[objNum] = update.NewContent
	}

	// Rebuild stream
	var newStream bytes.Buffer
	var newHeader bytes.Buffer

	offset := 0
	for _, entry := range entries {
		objData := objects[entry.objNum]
		newHeader.WriteString(fmt.Sprintf("%d %d ", entry.objNum, offset))
		newStream.Write(objData)
		newStream.WriteByte('\n')
		offset += len(objData) + 1
	}

	return newStream.Bytes(), newHeader.Bytes(), nil
}

// formatStreamDict formats a stream dictionary
func formatStreamDict(dict map[string]interface{}) string {
	var buf strings.Builder
	buf.WriteString("<<")

	if n, ok := dict["/N"].(int); ok {
		buf.WriteString(fmt.Sprintf(" /N %d", n))
	}

	if first, ok := dict["/First"].(int); ok {
		buf.WriteString(fmt.Sprintf(" /First %d", first))
	}

	if length, ok := dict["/Length"].(int); ok {
		buf.WriteString(fmt.Sprintf(" /Length %d", length))
	}

	buf.WriteString(" >>")
	return buf.String()
}
