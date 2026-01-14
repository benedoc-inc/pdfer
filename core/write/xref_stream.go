package write

import (
	"bytes"
	"compress/zlib"
	"fmt"
)

// writeXRefStream writes a cross-reference stream instead of a traditional xref table
// Returns the position where the xref stream object starts
func (w *PDFWriter) writeXRefStream(buf *bytes.Buffer, positions map[int]int64) (int64, error) {
	// Reserve object number for xref stream
	xrefStreamObjNum := w.nextObjNum
	totalObjects := w.nextObjNum + 1 // Include the xref stream itself

	// Calculate field widths
	// w1: type field (0 or 1, so 1 byte is enough)
	// w2: offset field (need enough bytes for max offset)
	// w3: generation field (usually 0, so 1 byte is enough)
	maxOffset := int64(0)
	for _, pos := range positions {
		if pos > maxOffset {
			maxOffset = pos
		}
	}
	// Estimate xref stream position (will be after all objects)
	// Rough estimate: current buffer size + some overhead for xref stream
	estimatedXrefPos := int64(buf.Len()) + 200 // Add buffer for xref stream overhead
	if estimatedXrefPos > maxOffset {
		maxOffset = estimatedXrefPos
	}

	// Calculate bytes needed for offset
	w1 := 1 // Type field: 1 byte (0=free, 1=in-use, 2=compressed)
	w2 := calculateBytesNeeded(maxOffset)
	w3 := 1 // Generation: 1 byte (usually 0)

	// Build binary stream data
	// Include entry for xref stream object itself
	streamData := make([]byte, 0, totalObjects*(w1+w2+w3))

	// Entry for object 0 (always free)
	entry0 := make([]byte, w1+w2+w3)
	entry0[0] = 0 // Type 0 = free
	// Field 2 (offset) = 0 (free objects don't need offset)
	// Field 3 (generation) = 0
	streamData = append(streamData, entry0...)

	// Entries for each existing object
	for i := 1; i < w.nextObjNum; i++ {
		entry := make([]byte, w1+w2+w3)
		if pos, ok := positions[i]; ok {
			// Type 1: in-use object
			entry[0] = 1
			// Write offset in big-endian
			writeBigEndian(entry[w1:], pos, w2)
			// Generation = 0
			entry[w1+w2] = 0
		} else {
			// Free object
			entry[0] = 0
			// Offset = 0
			// Generation = 0
		}
		streamData = append(streamData, entry...)
	}

	// Entry for xref stream object itself (type 1, offset will be set)
	xrefEntry := make([]byte, w1+w2+w3)
	xrefEntry[0] = 1 // Type 1 = in-use
	// Offset will be set after we know the actual position
	xrefEntry[w1+w2] = 0 // Generation = 0
	streamData = append(streamData, xrefEntry...)

	// Get actual position where xref stream will be written
	xrefPos := int64(buf.Len())

	// Update xref stream entry with its own offset
	// The xref stream entry is the last entry in streamData
	xrefEntryStart := len(streamData) - (w1 + w2 + w3)
	writeBigEndian(streamData[xrefEntryStart+w1:], xrefPos, w2)

	// Compress stream data with FlateDecode (zlib)
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(streamData); err != nil {
		return 0, fmt.Errorf("failed to compress xref stream: %v", err)
	}
	if err := zw.Close(); err != nil {
		return 0, fmt.Errorf("failed to close zlib writer: %v", err)
	}
	compressedData := compressed.Bytes()

	// Create xref stream dictionary
	// Note: /Root, /Info, /Encrypt, /ID go in the trailer, not the stream dict
	xrefStreamDict := Dictionary{
		"/Type":   "/XRef",
		"/Size":   totalObjects,
		"/W":      []interface{}{w1, w2, w3},
		"/Filter": "/FlateDecode",
		"/Length": len(compressedData),
	}

	// Increment object counter
	w.nextObjNum++

	// Write object header
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", xrefStreamObjNum))

	// Write dictionary
	dictBytes := w.formatDictionary(xrefStreamDict)
	buf.Write(dictBytes)
	buf.WriteString("\nstream\n")

	// Write compressed stream data
	buf.Write(compressedData)

	buf.WriteString("\nendstream\nendobj\n")

	return xrefPos, nil
}

// calculateBytesNeeded calculates how many bytes are needed to represent a number
func calculateBytesNeeded(n int64) int {
	if n == 0 {
		return 1
	}
	bytes := 0
	for n > 0 {
		bytes++
		n >>= 8
	}
	return bytes
}

// writeBigEndian writes a number in big-endian format to a byte slice
func writeBigEndian(dst []byte, value int64, width int) {
	for i := width - 1; i >= 0; i-- {
		dst[i] = byte(value & 0xff)
		value >>= 8
	}
}
