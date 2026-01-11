package font

import (
	"bytes"
	"testing"
)

// TestParseTTF tests basic TTF parsing
func TestParseTTF(t *testing.T) {
	// Create a minimal TTF structure for testing
	// This is a simplified test - real TTF files would be more complex
	ttfData := createMinimalTTF()

	ttf, err := ParseTTF(ttfData)
	if err != nil {
		t.Fatalf("Failed to parse TTF: %v", err)
	}

	if ttf == nil {
		t.Fatal("ParseTTF returned nil")
	}

	if ttf.UnitsPerEm == 0 {
		t.Error("UnitsPerEm should not be zero")
	}
}

// TestFontSubset tests font subsetting
// Note: This test requires a full TTF with cmap table to work properly
// For now, we test the basic AddString/AddRune functionality
func TestFontSubset(t *testing.T) {
	ttfData := createMinimalTTF()

	font, err := NewFont("TestFont", ttfData)
	if err != nil {
		t.Fatalf("Failed to create font: %v", err)
	}

	// Add some characters
	font.AddString("Hello, World!")
	font.AddRune('!')

	// Verify characters were added
	if len(font.Subset) == 0 {
		t.Error("Subset should contain characters")
	}

	// Test that duplicates aren't added
	originalLen := len(font.Subset)
	font.AddString("Hello") // Should not add duplicates
	if len(font.Subset) != originalLen {
		t.Error("Duplicate characters should not be added")
	}
}

// TestFontWidths tests width calculation
// Note: This test requires a full TTF with cmap and hmtx tables
func TestFontWidths(t *testing.T) {
	ttfData := createMinimalTTF()

	font, err := NewFont("TestFont", ttfData)
	if err != nil {
		t.Fatalf("Failed to create font: %v", err)
	}

	font.AddString("Test")

	// This will fail without cmap table, which is expected for minimal TTF
	_, err = font.GetWidths()
	if err == nil {
		t.Error("Expected error for minimal TTF without cmap table")
	}
}

// createMinimalTTF creates a minimal valid TTF structure for testing
// This is a very simplified TTF - real fonts would be much more complex
func createMinimalTTF() []byte {
	// This is a placeholder - creating a full valid TTF is complex
	// In real usage, you would load an actual TTF file
	// For now, we'll create a minimal structure that passes basic validation

	var buf bytes.Buffer

	// SFNT header
	buf.Write([]byte{0x00, 0x01, 0x00, 0x00}) // TTF signature
	buf.Write([]byte{0x00, 0x05})             // numTables = 5
	buf.Write([]byte{0x00, 0x00})             // searchRange
	buf.Write([]byte{0x00, 0x00})             // entrySelector
	buf.Write([]byte{0x00, 0x00})             // rangeShift

	// Table directory entries (simplified)
	offset := 12 + 5*16 // Start of table data

	// 'head' table
	buf.WriteString("head")
	buf.Write([]byte{0, 0, 0, 0}) // checksum
	writeUint32(&buf, uint32(offset))
	writeUint32(&buf, 54) // length
	headData := make([]byte, 54)
	writeUint32ToBytes(headData[0:4], 0x00010000)   // version
	writeUint32ToBytes(headData[4:8], 0x00010000)   // fontRevision
	writeUint32ToBytes(headData[8:12], 0)           // checksumAdjustment
	writeUint32ToBytes(headData[12:16], 0x5F0F3CF5) // magicNumber
	writeUint16ToBytes(headData[16:18], 0)          // flags
	writeUint16ToBytes(headData[18:20], 1000)       // unitsPerEm
	writeUint64ToBytes(headData[20:28], 0)          // created
	writeUint64ToBytes(headData[28:36], 0)          // modified
	writeInt16(headData[36:38], -100)               // xMin
	writeInt16(headData[38:40], -100)               // yMin
	writeInt16(headData[40:42], 1100)               // xMax
	writeInt16(headData[42:44], 1100)               // yMax
	writeUint16ToBytes(headData[44:46], 0)          // macStyle
	writeUint16ToBytes(headData[46:48], 0)          // lowestRecPPEM
	writeInt16(headData[48:50], 0)                  // fontDirectionHint
	writeInt16(headData[50:52], 0)                  // indexToLocFormat
	writeInt16(headData[52:54], 0)                  // glyphDataFormat
	offset += 54

	// 'maxp' table
	buf.WriteString("maxp")
	buf.Write([]byte{0, 0, 0, 0}) // checksum
	writeUint32(&buf, uint32(offset))
	writeUint32(&buf, 6) // length
	maxpData := make([]byte, 6)
	writeUint32ToBytes(maxpData[0:4], 0x00005000) // version
	writeUint16ToBytes(maxpData[4:6], 1)          // numGlyphs
	offset += 6

	// 'hhea' table
	buf.WriteString("hhea")
	buf.Write([]byte{0, 0, 0, 0}) // checksum
	writeUint32(&buf, uint32(offset))
	writeUint32(&buf, 36) // length
	hheaData := make([]byte, 36)
	writeUint32ToBytes(hheaData[0:4], 0x00010000) // version
	writeInt16(hheaData[4:6], 800)                // ascent
	writeInt16(hheaData[6:8], -200)               // descent
	writeInt16(hheaData[8:10], 0)                 // lineGap
	writeUint16ToBytes(hheaData[10:12], 1000)     // advanceWidthMax
	writeInt16(hheaData[12:14], 0)                // minLeftSideBearing
	writeInt16(hheaData[14:16], 0)                // minRightSideBearing
	writeInt16(hheaData[16:18], 0)                // xMaxExtent
	writeInt16(hheaData[18:20], 0)                // caretSlopeRise
	writeInt16(hheaData[20:22], 0)                // caretSlopeRun
	writeInt16(hheaData[22:24], 0)                // caretOffset
	writeInt16(hheaData[24:26], 0)                // reserved
	writeInt16(hheaData[26:28], 0)                // reserved
	writeInt16(hheaData[28:30], 0)                // reserved
	writeInt16(hheaData[30:32], 0)                // reserved
	writeInt16(hheaData[32:34], 0)                // metricDataFormat
	writeUint16ToBytes(hheaData[34:36], 0)        // numberOfHMetrics
	offset += 36

	// 'name' table
	buf.WriteString("name")
	buf.Write([]byte{0, 0, 0, 0}) // checksum
	writeUint32(&buf, uint32(offset))
	nameLen := 6 + 12 + 20 // header + 1 entry + string data
	writeUint32(&buf, uint32(nameLen))
	nameData := make([]byte, nameLen)
	writeUint16ToBytes(nameData[0:2], 0)      // format
	writeUint16ToBytes(nameData[2:4], 1)      // count
	writeUint16ToBytes(nameData[4:6], 38)     // stringOffset
	writeUint16ToBytes(nameData[6:8], 3)      // platformID (Windows)
	writeUint16ToBytes(nameData[8:10], 1)     // encodingID (Unicode BMP)
	writeUint16ToBytes(nameData[10:12], 0)    // languageID
	writeUint16ToBytes(nameData[12:14], 6)    // nameID (PostScript)
	writeUint16ToBytes(nameData[14:16], 10)   // length
	writeUint16ToBytes(nameData[16:18], 0)    // offset (relative to stringOffset)
	copy(nameData[18:28], []byte("TestFont")) // String data
	offset += nameLen

	// 'post' table
	buf.WriteString("post")
	buf.Write([]byte{0, 0, 0, 0}) // checksum
	writeUint32(&buf, uint32(offset))
	writeUint32(&buf, 32) // length
	postData := make([]byte, 32)
	writeUint32ToBytes(postData[0:4], 0x00020000) // version
	writeUint32ToBytes(postData[4:8], 0)          // italicAngle
	writeInt16(postData[8:10], -100)              // underlinePosition
	writeInt16(postData[10:12], 50)               // underlineThickness
	writeUint32ToBytes(postData[12:16], 0)        // isFixedPitch
	writeUint32ToBytes(postData[16:20], 0)        // minMemType42
	writeUint32ToBytes(postData[20:24], 0)        // maxMemType42
	writeUint32ToBytes(postData[24:28], 0)        // minMemType1
	writeUint32ToBytes(postData[28:32], 0)        // maxMemType1
	offset += 32

	// Write table data
	buf.Write(headData)
	buf.Write(maxpData)
	buf.Write(hheaData)
	buf.Write(nameData)
	buf.Write(postData)

	return buf.Bytes()
}

func writeUint16(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
}

func writeUint16ToBytes(data []byte, v uint16) {
	data[0] = byte(v >> 8)
	data[1] = byte(v)
}

func writeUint32(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v >> 24))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
}

func writeUint32ToBytes(data []byte, v uint32) {
	data[0] = byte(v >> 24)
	data[1] = byte(v >> 16)
	data[2] = byte(v >> 8)
	data[3] = byte(v)
}

func writeInt16(data []byte, v int16) {
	data[0] = byte(v >> 8)
	data[1] = byte(v)
}

func writeUint64ToBytes(data []byte, v uint64) {
	for i := 0; i < 8; i++ {
		data[i] = byte(v >> (56 - i*8))
	}
}
