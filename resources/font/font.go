// Package font provides TrueType/OpenType font embedding for PDFs
package font

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"unicode"
)

// Font represents an embedded font with subsetting support
type Font struct {
	Name    string
	Subtype string // "TrueType" or "CIDFontType2"
	Data    []byte // Raw font file (TTF/OTF)
	Subset  []rune // Characters to include in subset
	FontID  string // Unique identifier for this font instance
}

// TTF represents a parsed TrueType font
type TTF struct {
	Data               []byte
	Tables             map[string]*Table
	NumGlyphs          uint16
	UnitsPerEm         uint16
	Ascent             int16
	Descent            int16
	CapHeight          int16
	XHeight            int16
	ItalicAngle        float32
	UnderlinePosition  int16
	UnderlineThickness int16
	IsFixedPitch       bool
	FontName           string
	FamilyName         string
	FullName           string
	PostScriptName     string
}

// Table represents a TTF table
type Table struct {
	Tag      string
	Checksum uint32
	Offset   uint32
	Length   uint32
	Data     []byte
}

// NewFont creates a new Font from TTF/OTF data
func NewFont(name string, data []byte) (*Font, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("font data too short")
	}

	// Check TTF signature
	signature := binary.BigEndian.Uint32(data[0:4])
	if signature != 0x00010000 && signature != 0x4F54544F { // TTF or OTF
		return nil, fmt.Errorf("unsupported font format (expected TTF or OTF)")
	}

	return &Font{
		Name:    name,
		Subtype: "TrueType",
		Data:    data,
		Subset:  make([]rune, 0),
	}, nil
}

// ParseTTF parses a TTF/OTF font file
func ParseTTF(data []byte) (*TTF, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("font data too short")
	}

	ttf := &TTF{
		Data:   data,
		Tables: make(map[string]*Table),
	}

	// Read SFNT header
	signature := binary.BigEndian.Uint32(data[0:4])
	if signature != 0x00010000 && signature != 0x4F54544F {
		return nil, fmt.Errorf("unsupported font format")
	}

	numTables := binary.BigEndian.Uint16(data[4:6])
	offset := 12

	// Read table directory
	for i := 0; i < int(numTables); i++ {
		if offset+16 > len(data) {
			return nil, fmt.Errorf("invalid table directory")
		}

		tag := string(data[offset : offset+4])
		checksum := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		tableOffset := binary.BigEndian.Uint32(data[offset+8 : offset+12])
		length := binary.BigEndian.Uint32(data[offset+12 : offset+16])

		if int(tableOffset)+int(length) > len(data) {
			return nil, fmt.Errorf("table %s extends beyond font data", tag)
		}

		ttf.Tables[tag] = &Table{
			Tag:      tag,
			Checksum: checksum,
			Offset:   tableOffset,
			Length:   length,
			Data:     data[tableOffset : tableOffset+length],
		}

		offset += 16
	}

	// Parse required tables
	if err := ttf.parseHead(); err != nil {
		return nil, fmt.Errorf("failed to parse head table: %w", err)
	}
	if err := ttf.parseMaxp(); err != nil {
		return nil, fmt.Errorf("failed to parse maxp table: %w", err)
	}
	if err := ttf.parseHhea(); err != nil {
		return nil, fmt.Errorf("failed to parse hhea table: %w", err)
	}
	if err := ttf.parseName(); err != nil {
		return nil, fmt.Errorf("failed to parse name table: %w", err)
	}
	if err := ttf.parsePost(); err != nil {
		return nil, fmt.Errorf("failed to parse post table: %w", err)
	}

	return ttf, nil
}

// parseHead parses the 'head' table
func (ttf *TTF) parseHead() error {
	head, ok := ttf.Tables["head"]
	if !ok {
		return fmt.Errorf("missing head table")
	}
	if len(head.Data) < 54 {
		return fmt.Errorf("head table too short")
	}

	ttf.UnitsPerEm = binary.BigEndian.Uint16(head.Data[18:20])
	return nil
}

// parseMaxp parses the 'maxp' table
func (ttf *TTF) parseMaxp() error {
	maxp, ok := ttf.Tables["maxp"]
	if !ok {
		return fmt.Errorf("missing maxp table")
	}
	if len(maxp.Data) < 6 {
		return fmt.Errorf("maxp table too short")
	}

	ttf.NumGlyphs = binary.BigEndian.Uint16(maxp.Data[4:6])
	return nil
}

// parseHhea parses the 'hhea' table
func (ttf *TTF) parseHhea() error {
	hhea, ok := ttf.Tables["hhea"]
	if !ok {
		return fmt.Errorf("missing hhea table")
	}
	if len(hhea.Data) < 36 {
		return fmt.Errorf("hhea table too short")
	}

	ttf.Ascent = int16(binary.BigEndian.Uint16(hhea.Data[4:6]))
	ttf.Descent = int16(binary.BigEndian.Uint16(hhea.Data[6:8]))
	return nil
}

// parseName parses the 'name' table
func (ttf *TTF) parseName() error {
	name, ok := ttf.Tables["name"]
	if !ok {
		return fmt.Errorf("missing name table")
	}
	if len(name.Data) < 6 {
		return fmt.Errorf("name table too short")
	}

	count := binary.BigEndian.Uint16(name.Data[2:4])
	stringOffset := binary.BigEndian.Uint16(name.Data[4:6])
	offset := 6

	for i := 0; i < int(count); i++ {
		if offset+12 > len(name.Data) {
			break
		}

		platformID := binary.BigEndian.Uint16(name.Data[offset : offset+2])
		_ = binary.BigEndian.Uint16(name.Data[offset+2 : offset+4]) // encodingID
		_ = binary.BigEndian.Uint16(name.Data[offset+4 : offset+6]) // languageID
		nameID := binary.BigEndian.Uint16(name.Data[offset+6 : offset+8])
		length := binary.BigEndian.Uint16(name.Data[offset+8 : offset+10])
		startOffset := binary.BigEndian.Uint16(name.Data[offset+10 : offset+12])

		// We're interested in Unicode names (platform 3 or 0)
		if (platformID == 3 || platformID == 0) && int(stringOffset)+int(startOffset)+int(length) <= len(name.Data) {
			nameData := name.Data[stringOffset+startOffset : stringOffset+startOffset+length]

			var nameStr string
			if platformID == 3 {
				// UTF-16BE
				if len(nameData)%2 == 0 {
					runes := make([]rune, 0, len(nameData)/2)
					for j := 0; j < len(nameData); j += 2 {
						runes = append(runes, rune(binary.BigEndian.Uint16(nameData[j:j+2])))
					}
					nameStr = string(runes)
				}
			} else {
				// ASCII
				nameStr = string(nameData)
			}

			switch nameID {
			case 1:
				if ttf.FamilyName == "" {
					ttf.FamilyName = nameStr
				}
			case 4:
				if ttf.FullName == "" {
					ttf.FullName = nameStr
				}
			case 6:
				if ttf.PostScriptName == "" {
					ttf.PostScriptName = nameStr
				}
			}
		}

		offset += 12
	}

	// Use PostScript name as primary if available
	if ttf.PostScriptName != "" {
		ttf.FontName = ttf.PostScriptName
	} else if ttf.FullName != "" {
		ttf.FontName = ttf.FullName
	} else if ttf.FamilyName != "" {
		ttf.FontName = ttf.FamilyName
	}

	return nil
}

// parsePost parses the 'post' table
func (ttf *TTF) parsePost() error {
	post, ok := ttf.Tables["post"]
	if !ok {
		return fmt.Errorf("missing post table")
	}
	if len(post.Data) < 32 {
		return fmt.Errorf("post table too short")
	}

	ttf.ItalicAngle = float32(int32(binary.BigEndian.Uint32(post.Data[4:8]))) / 65536.0
	ttf.UnderlinePosition = int16(binary.BigEndian.Uint16(post.Data[8:10]))
	ttf.UnderlineThickness = int16(binary.BigEndian.Uint16(post.Data[10:12]))
	ttf.IsFixedPitch = binary.BigEndian.Uint32(post.Data[12:16]) != 0

	// Try to get CapHeight and XHeight from OS/2 table if available
	if os2, ok := ttf.Tables["OS/2"]; ok && len(os2.Data) >= 88 {
		ttf.CapHeight = int16(binary.BigEndian.Uint16(os2.Data[68:70]))
		ttf.XHeight = int16(binary.BigEndian.Uint16(os2.Data[70:72]))
	}

	return nil
}

// AddRune adds a rune to the subset
func (f *Font) AddRune(r rune) {
	// Check if already in subset
	for _, existing := range f.Subset {
		if existing == r {
			return
		}
	}
	f.Subset = append(f.Subset, r)
}

// AddString adds all runes from a string to the subset
func (f *Font) AddString(s string) {
	for _, r := range s {
		if unicode.IsPrint(r) || r == ' ' {
			f.AddRune(r)
		}
	}
}

// GetSubsetGlyphs returns the glyph IDs for the subset characters
func (f *Font) GetSubsetGlyphs() ([]uint16, error) {
	ttf, err := ParseTTF(f.Data)
	if err != nil {
		return nil, err
	}

	// Parse cmap to get glyph mappings
	cmap, ok := ttf.Tables["cmap"]
	if !ok {
		return nil, fmt.Errorf("missing cmap table")
	}

	glyphMap, err := parseCmap(cmap.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cmap: %w", err)
	}

	// Always include glyph 0 (notdef)
	glyphSet := map[uint16]bool{0: true}

	// Map runes to glyph IDs
	for _, r := range f.Subset {
		if gid, ok := glyphMap[r]; ok {
			glyphSet[gid] = true
		}
	}

	// Convert to sorted slice
	glyphs := make([]uint16, 0, len(glyphSet))
	for gid := range glyphSet {
		glyphs = append(glyphs, gid)
	}
	sort.Slice(glyphs, func(i, j int) bool {
		return glyphs[i] < glyphs[j]
	})

	return glyphs, nil
}

// parseCmap parses the cmap table to create a Unicode->glyph mapping
func parseCmap(data []byte) (map[rune]uint16, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("cmap table too short")
	}

	version := binary.BigEndian.Uint16(data[0:2])
	if version != 0 {
		return nil, fmt.Errorf("unsupported cmap version: %d", version)
	}

	numTables := binary.BigEndian.Uint16(data[2:4])
	offset := 4

	glyphMap := make(map[rune]uint16)

	// Find Unicode encoding (platform 3, encoding 1 or 10)
	var bestTable *Table
	var bestOffset uint32

	for i := 0; i < int(numTables); i++ {
		if offset+8 > len(data) {
			break
		}

		platformID := binary.BigEndian.Uint16(data[offset : offset+2])
		encodingID := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		subtableOffset := binary.BigEndian.Uint32(data[offset+4 : offset+8])

		// Prefer platform 3, encoding 1 (Unicode BMP) or 10 (Unicode full)
		if platformID == 3 && (encodingID == 1 || encodingID == 10) {
			if bestTable == nil || encodingID == 10 {
				bestTable = &Table{Offset: subtableOffset}
				bestOffset = subtableOffset
			}
		}

		offset += 8
	}

	if bestTable == nil {
		// Try platform 0 (Unicode)
		offset = 4
		for i := 0; i < int(numTables); i++ {
			if offset+8 > len(data) {
				break
			}

			platformID := binary.BigEndian.Uint16(data[offset : offset+2])
			_ = binary.BigEndian.Uint16(data[offset+2 : offset+4]) // encodingID
			subtableOffset := binary.BigEndian.Uint32(data[offset+4 : offset+8])

			if platformID == 0 {
				bestTable = &Table{Offset: subtableOffset}
				bestOffset = subtableOffset
				break
			}

			offset += 8
		}
	}

	if bestTable == nil {
		return nil, fmt.Errorf("no Unicode cmap subtable found")
	}

	// Parse the subtable
	subtableData := data[bestOffset:]
	if len(subtableData) < 2 {
		return nil, fmt.Errorf("cmap subtable too short")
	}

	format := binary.BigEndian.Uint16(subtableData[0:2])

	switch format {
	case 4:
		// Format 4: Segment mapping to delta values
		return parseCmapFormat4(subtableData, glyphMap)
	case 12:
		// Format 12: Segmented coverage
		return parseCmapFormat12(subtableData, glyphMap)
	default:
		// Try format 4 as fallback
		return parseCmapFormat4(subtableData, glyphMap)
	}
}

// parseCmapFormat4 parses cmap format 4 (most common)
func parseCmapFormat4(data []byte, glyphMap map[rune]uint16) (map[rune]uint16, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("cmap format 4 too short")
	}

	length := binary.BigEndian.Uint16(data[2:4])
	language := binary.BigEndian.Uint16(data[4:6])
	segCount := binary.BigEndian.Uint16(data[6:8]) / 2

	if len(data) < int(length) {
		return nil, fmt.Errorf("cmap format 4 length mismatch")
	}

	_ = language // unused

	endCodeOffset := 14
	startCodeOffset := endCodeOffset + 2 + int(segCount)*2
	idDeltaOffset := startCodeOffset + int(segCount)*2
	idRangeOffsetOffset := idDeltaOffset + int(segCount)*2

	for i := 0; i < int(segCount); i++ {
		startCode := binary.BigEndian.Uint16(data[startCodeOffset+i*2 : startCodeOffset+i*2+2])
		endCode := binary.BigEndian.Uint16(data[endCodeOffset+i*2 : endCodeOffset+i*2+2])
		idDelta := int16(binary.BigEndian.Uint16(data[idDeltaOffset+i*2 : idDeltaOffset+i*2+2]))
		idRangeOffset := binary.BigEndian.Uint16(data[idRangeOffsetOffset+i*2 : idRangeOffsetOffset+i*2+2])

		if endCode == 0xFFFF && startCode == 0xFFFF {
			break // Last segment
		}

		for code := startCode; code <= endCode && code != 0xFFFF; code++ {
			var glyphID uint16

			if idRangeOffset == 0 {
				glyphID = uint16(int16(code) + idDelta)
			} else {
				glyphOffset := idRangeOffsetOffset + int(i)*2 + int(idRangeOffset) + int(code-startCode)*2
				if glyphOffset+2 <= len(data) {
					glyphID = binary.BigEndian.Uint16(data[glyphOffset : glyphOffset+2])
					if glyphID != 0 {
						glyphID = uint16(int16(glyphID) + idDelta)
					}
				}
			}

			if glyphID != 0 {
				glyphMap[rune(code)] = glyphID
			}
		}
	}

	return glyphMap, nil
}

// parseCmapFormat12 parses cmap format 12 (32-bit coverage)
func parseCmapFormat12(data []byte, glyphMap map[rune]uint16) (map[rune]uint16, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("cmap format 12 too short")
	}

	length := binary.BigEndian.Uint32(data[4:8])
	numGroups := binary.BigEndian.Uint32(data[12:16])

	if len(data) < int(length) {
		return nil, fmt.Errorf("cmap format 12 length mismatch")
	}

	offset := 16
	for i := 0; i < int(numGroups); i++ {
		if offset+12 > len(data) {
			break
		}

		startCharCode := binary.BigEndian.Uint32(data[offset : offset+4])
		endCharCode := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		startGlyphID := binary.BigEndian.Uint32(data[offset+8 : offset+12])

		for code := startCharCode; code <= endCharCode; code++ {
			glyphID := uint16(startGlyphID + (code - startCharCode))
			glyphMap[rune(code)] = glyphID
		}

		offset += 12
	}

	return glyphMap, nil
}

// CreateSubsetFont creates a subset font file with only the specified glyphs
// This is a simplified version - full subsetting would require rebuilding all tables
func (f *Font) CreateSubsetFont() ([]byte, error) {
	// For now, we'll embed the full font
	// Full subsetting requires complex table rebuilding
	// This is a placeholder that can be enhanced later
	return f.Data, nil
}

// GetWidths returns the width array for the subset glyphs
func (f *Font) GetWidths() ([]int, error) {
	ttf, err := ParseTTF(f.Data)
	if err != nil {
		return nil, err
	}

	glyphs, err := f.GetSubsetGlyphs()
	if err != nil {
		return nil, err
	}

	// Parse hmtx table for widths
	hmtx, ok := ttf.Tables["hmtx"]
	if !ok {
		return nil, fmt.Errorf("missing hmtx table")
	}

	widths := make([]int, len(glyphs))
	for i, gid := range glyphs {
		if int(gid)*4+2 <= len(hmtx.Data) {
			widths[i] = int(binary.BigEndian.Uint16(hmtx.Data[gid*4 : gid*4+2]))
		} else {
			widths[i] = int(ttf.UnitsPerEm) // Default width
		}
	}

	return widths, nil
}

// Read reads font data from a reader
func Read(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
