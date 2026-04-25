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

// CreateSubsetFont creates a subset font file with only the glyphs in f.Subset.
// It rebuilds cmap, glyf, loca, hmtx, head, hhea, and maxp tables and copies all
// other tables as-is. On any parsing failure it falls back to the full font data.
func (f *Font) CreateSubsetFont() ([]byte, error) {
	result, err := buildSubsetFont(f.Data, f.Subset)
	if err != nil {
		return f.Data, nil
	}
	return result, nil
}

// buildSubsetFont performs the actual TrueType subsetting and returns an error on failure.
func buildSubsetFont(data []byte, subsetRunes []rune) ([]byte, error) {
	ttf, err := ParseTTF(data)
	if err != nil {
		return nil, err
	}

	// Parse cmap to get rune->glyphID mapping.
	cmapTable, ok := ttf.Tables["cmap"]
	if !ok {
		return nil, fmt.Errorf("missing cmap table")
	}
	runeToGlyph, err := parseCmap(cmapTable.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cmap: %w", err)
	}

	// Collect glyph IDs for the requested runes; always include .notdef (0).
	glyphSet := map[uint16]bool{0: true}
	for _, r := range subsetRunes {
		if gid, ok2 := runeToGlyph[r]; ok2 {
			glyphSet[gid] = true
		}
	}

	// Parse head to get indexToLocFormat.
	headTable, ok := ttf.Tables["head"]
	if !ok {
		return nil, fmt.Errorf("missing head table")
	}
	if len(headTable.Data) < 52 {
		return nil, fmt.Errorf("head table too short")
	}
	locFormat := int16(binary.BigEndian.Uint16(headTable.Data[50:52]))

	// Parse loca table into uint32 offsets.
	locaTable, ok := ttf.Tables["loca"]
	if !ok {
		return nil, fmt.Errorf("missing loca table")
	}
	numGlyphs := int(ttf.NumGlyphs)
	offsets := make([]uint32, numGlyphs+1)
	if locFormat == 0 {
		if len(locaTable.Data) < (numGlyphs+1)*2 {
			return nil, fmt.Errorf("loca table too short (short format)")
		}
		for i := 0; i <= numGlyphs; i++ {
			offsets[i] = uint32(binary.BigEndian.Uint16(locaTable.Data[i*2:i*2+2])) * 2
		}
	} else {
		if len(locaTable.Data) < (numGlyphs+1)*4 {
			return nil, fmt.Errorf("loca table too short (long format)")
		}
		for i := 0; i <= numGlyphs; i++ {
			offsets[i] = binary.BigEndian.Uint32(locaTable.Data[i*4 : i*4+4])
		}
	}

	glyfTable, ok := ttf.Tables["glyf"]
	if !ok {
		return nil, fmt.Errorf("missing glyf table")
	}
	glyfData := glyfTable.Data

	// Expand composite glyphs one level deep: add referenced component glyph IDs.
	for gid := range glyphSet {
		gi := int(gid)
		if gi >= numGlyphs {
			continue
		}
		st, en := offsets[gi], offsets[gi+1]
		if en <= st || int(en) > len(glyfData) || int(st) > len(glyfData) {
			continue
		}
		gb := glyfData[st:en]
		if len(gb) >= 2 && int16(binary.BigEndian.Uint16(gb[0:2])) < 0 {
			for _, cgid := range subsetParseCompositeGlyphs(gb) {
				glyphSet[cgid] = true
			}
		}
	}

	// Build sorted list of old glyph IDs and old->new remapping.
	oldGIDs := make([]uint16, 0, len(glyphSet))
	for gid := range glyphSet {
		oldGIDs = append(oldGIDs, gid)
	}
	sort.Slice(oldGIDs, func(i, j int) bool { return oldGIDs[i] < oldGIDs[j] })

	oldToNew := make(map[uint16]uint16, len(oldGIDs))
	for newID, oldID := range oldGIDs {
		oldToNew[oldID] = uint16(newID)
	}
	newNumGlyphs := len(oldGIDs)

	// Build per-glyph byte slices for the new glyf table (with remapped composite GIDs).
	newGlyfSlices := make([][]byte, newNumGlyphs)
	for i, oldGID := range oldGIDs {
		gi := int(oldGID)
		if gi >= numGlyphs {
			continue
		}
		st, en := offsets[gi], offsets[gi+1]
		if en <= st || int(en) > len(glyfData) || int(st) > len(glyfData) {
			continue
		}
		gb := make([]byte, en-st)
		copy(gb, glyfData[st:en])
		if len(gb) >= 2 && int16(binary.BigEndian.Uint16(gb[0:2])) < 0 {
			subsetRemapCompositeGlyphs(gb, oldToNew)
		}
		newGlyfSlices[i] = gb
	}

	// Assemble new glyf bytes with 4-byte padding and record per-glyph offsets.
	var newGlyfBuf bytes.Buffer
	newGlyfOffsets := make([]uint32, newNumGlyphs+1)
	for i, gb := range newGlyfSlices {
		newGlyfOffsets[i] = uint32(newGlyfBuf.Len())
		if len(gb) > 0 {
			newGlyfBuf.Write(gb)
			if pad := len(gb) % 4; pad != 0 {
				newGlyfBuf.Write(make([]byte, 4-pad))
			}
		}
	}
	newGlyfOffsets[newNumGlyphs] = uint32(newGlyfBuf.Len())
	newGlyfBytes := newGlyfBuf.Bytes()

	// Build new loca table (long format, uint32).
	newLocaBytes := make([]byte, (newNumGlyphs+1)*4)
	for i, off := range newGlyfOffsets {
		binary.BigEndian.PutUint32(newLocaBytes[i*4:], off)
	}

	// Build new hmtx table.
	hmtxTable, ok := ttf.Tables["hmtx"]
	if !ok {
		return nil, fmt.Errorf("missing hmtx table")
	}
	hheaTable, ok := ttf.Tables["hhea"]
	if !ok {
		return nil, fmt.Errorf("missing hhea table")
	}
	if len(hheaTable.Data) < 36 {
		return nil, fmt.Errorf("hhea table too short")
	}
	numHMetrics := int(binary.BigEndian.Uint16(hheaTable.Data[34:36]))

	getHmtx := func(gid uint16) (advanceWidth uint16, lsb int16) {
		gi := int(gid)
		if numHMetrics > 0 && gi >= numHMetrics {
			advanceWidth = binary.BigEndian.Uint16(hmtxTable.Data[(numHMetrics-1)*4:])
			trailing := gi - numHMetrics
			off := numHMetrics*4 + trailing*2
			if off+2 <= len(hmtxTable.Data) {
				lsb = int16(binary.BigEndian.Uint16(hmtxTable.Data[off:]))
			}
		} else if gi*4+4 <= len(hmtxTable.Data) {
			advanceWidth = binary.BigEndian.Uint16(hmtxTable.Data[gi*4:])
			lsb = int16(binary.BigEndian.Uint16(hmtxTable.Data[gi*4+2:]))
		}
		return
	}

	newHmtxBytes := make([]byte, newNumGlyphs*4)
	for i, oldGID := range oldGIDs {
		aw, lb := getHmtx(oldGID)
		binary.BigEndian.PutUint16(newHmtxBytes[i*4:], aw)
		binary.BigEndian.PutUint16(newHmtxBytes[i*4+2:], uint16(lb))
	}

	// Build new cmap (format 4) for the subset runes.
	var subsetMappings []runeGIDPair
	for _, r := range subsetRunes {
		if oldGID, ok2 := runeToGlyph[r]; ok2 {
			if newGID, ok3 := oldToNew[oldGID]; ok3 {
				subsetMappings = append(subsetMappings, runeGIDPair{r, newGID})
			}
		}
	}
	sort.Slice(subsetMappings, func(i, j int) bool { return subsetMappings[i].r < subsetMappings[j].r })

	newCmapBytes := subsetBuildCmapFormat4(subsetMappings)

	// Build updated head: set indexToLocFormat=1, zero checkSumAdjustment.
	newHeadBytes := make([]byte, len(headTable.Data))
	copy(newHeadBytes, headTable.Data)
	binary.BigEndian.PutUint16(newHeadBytes[50:52], 1) // indexToLocFormat = long
	binary.BigEndian.PutUint32(newHeadBytes[8:12], 0)  // checkSumAdjustment = 0

	// Build updated hhea: set numberOfHMetrics.
	newHheaBytes := make([]byte, len(hheaTable.Data))
	copy(newHheaBytes, hheaTable.Data)
	binary.BigEndian.PutUint16(newHheaBytes[34:36], uint16(newNumGlyphs))

	// Build updated maxp: set numGlyphs.
	maxpTable, ok := ttf.Tables["maxp"]
	if !ok {
		return nil, fmt.Errorf("missing maxp table")
	}
	newMaxpBytes := make([]byte, len(maxpTable.Data))
	copy(newMaxpBytes, maxpTable.Data)
	if len(newMaxpBytes) >= 6 {
		binary.BigEndian.PutUint16(newMaxpBytes[4:6], uint16(newNumGlyphs))
	}

	// Map of rebuilt tables.
	rebuildMap := map[string][]byte{
		"cmap": newCmapBytes,
		"glyf": newGlyfBytes,
		"loca": newLocaBytes,
		"hmtx": newHmtxBytes,
		"head": newHeadBytes,
		"hhea": newHheaBytes,
		"maxp": newMaxpBytes,
	}

	// Canonical TTF table order.
	canonicalOrder := []string{
		"head", "hhea", "maxp", "OS/2", "name", "cmap", "post",
		"cvt ", "fpgm", "prep", "loca", "glyf", "hmtx",
		"kern", "GDEF", "GPOS", "GSUB",
	}
	canonicalIdx := make(map[string]int, len(canonicalOrder))
	for i, t := range canonicalOrder {
		canonicalIdx[t] = i
	}

	// Collect all table tags (rebuilt + originals).
	tagSet := make(map[string]bool)
	for tag := range rebuildMap {
		tagSet[tag] = true
	}
	for tag := range ttf.Tables {
		tagSet[tag] = true
	}

	type tableEntry struct {
		tag  string
		data []byte
	}
	var tables []tableEntry
	for tag := range tagSet {
		var td []byte
		if d, ok2 := rebuildMap[tag]; ok2 {
			td = d
		} else if t, ok2 := ttf.Tables[tag]; ok2 {
			td = t.Data
		}
		if len(td) == 0 {
			continue
		}
		tables = append(tables, tableEntry{tag: tag, data: td})
	}
	sort.Slice(tables, func(i, j int) bool {
		ci, cj := 999, 999
		if idx, ok2 := canonicalIdx[tables[i].tag]; ok2 {
			ci = idx
		}
		if idx, ok2 := canonicalIdx[tables[j].tag]; ok2 {
			cj = idx
		}
		if ci != cj {
			return ci < cj
		}
		return tables[i].tag < tables[j].tag
	})

	numTablesOut := len(tables)

	// Lay out output: offset table (12) + directory (numTables*16) + data.
	dataStart := 12 + numTablesOut*16

	var out bytes.Buffer
	sr, es, rs := subsetSFNTSearchParams(numTablesOut)
	subsetWriteU32(&out, 0x00010000)
	subsetWriteU16(&out, uint16(numTablesOut))
	subsetWriteU16(&out, uint16(sr))
	subsetWriteU16(&out, uint16(es))
	subsetWriteU16(&out, uint16(rs))

	// Compute per-table offsets.
	type tableRecord struct {
		tag    string
		cs     uint32
		offset uint32
		length uint32
	}
	records := make([]tableRecord, numTablesOut)
	curOff := uint32(dataStart)
	for i, t := range tables {
		records[i] = tableRecord{
			tag:    t.tag,
			cs:     subsetTableChecksum(t.data),
			offset: curOff,
			length: uint32(len(t.data)),
		}
		curOff += uint32(len(t.data))
		if pad := len(t.data) % 4; pad != 0 {
			curOff += uint32(4 - pad)
		}
	}

	// Write table directory.
	for _, rec := range records {
		out.WriteString(rec.tag)
		subsetWriteU32(&out, rec.cs)
		subsetWriteU32(&out, rec.offset)
		subsetWriteU32(&out, rec.length)
	}

	// Write table data (4-byte aligned).
	for _, t := range tables {
		out.Write(t.data)
		if pad := len(t.data) % 4; pad != 0 {
			out.Write(make([]byte, 4-pad))
		}
	}

	// Patch head.checkSumAdjustment = 0xB1B0AFBA - wholeFileChecksum.
	fontBytes := out.Bytes()
	fontCS := subsetTableChecksum(fontBytes)
	adjustment := uint32(0xB1B0AFBA) - fontCS
	for _, rec := range records {
		if rec.tag == "head" {
			patchOff := int(rec.offset) + 8
			if patchOff+4 <= len(fontBytes) {
				binary.BigEndian.PutUint32(fontBytes[patchOff:patchOff+4], adjustment)
			}
			break
		}
	}

	return fontBytes, nil
}

// subsetParseCompositeGlyphs extracts referenced glyph IDs from a composite glyph record.
func subsetParseCompositeGlyphs(data []byte) []uint16 {
	const (
		flagArg1And2AreWords   = 0x0001
		flagMoreComponents     = 0x0020
		flagWeHaveAScale       = 0x0008
		flagWeHaveAnXAndYScale = 0x0080
		flagWeHaveATwoByTwo    = 0x0040
	)
	var result []uint16
	// Composite glyph: 10-byte header (numberOfContours + bbox), then components.
	offset := 10
	if len(data) < offset {
		return result
	}
	for {
		if offset+4 > len(data) {
			break
		}
		flags := binary.BigEndian.Uint16(data[offset : offset+2])
		glyphIndex := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		result = append(result, glyphIndex)
		offset += 4
		if flags&flagArg1And2AreWords != 0 {
			offset += 4
		} else {
			offset += 2
		}
		if flags&flagWeHaveATwoByTwo != 0 {
			offset += 8
		} else if flags&flagWeHaveAnXAndYScale != 0 {
			offset += 4
		} else if flags&flagWeHaveAScale != 0 {
			offset += 2
		}
		if flags&flagMoreComponents == 0 {
			break
		}
	}
	return result
}

// subsetRemapCompositeGlyphs rewrites component glyphIndex fields using oldToNew mapping.
func subsetRemapCompositeGlyphs(data []byte, oldToNew map[uint16]uint16) {
	const (
		flagArg1And2AreWords   = 0x0001
		flagMoreComponents     = 0x0020
		flagWeHaveAScale       = 0x0008
		flagWeHaveAnXAndYScale = 0x0080
		flagWeHaveATwoByTwo    = 0x0040
	)
	offset := 10
	if len(data) < offset {
		return
	}
	for {
		if offset+4 > len(data) {
			break
		}
		flags := binary.BigEndian.Uint16(data[offset : offset+2])
		oldGID := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		if newGID, ok := oldToNew[oldGID]; ok {
			binary.BigEndian.PutUint16(data[offset+2:offset+4], newGID)
		}
		offset += 4
		if flags&flagArg1And2AreWords != 0 {
			offset += 4
		} else {
			offset += 2
		}
		if flags&flagWeHaveATwoByTwo != 0 {
			offset += 8
		} else if flags&flagWeHaveAnXAndYScale != 0 {
			offset += 4
		} else if flags&flagWeHaveAScale != 0 {
			offset += 2
		}
		if flags&flagMoreComponents == 0 {
			break
		}
	}
}

// runeGIDPair is used internally by subsetBuildCmapFormat4.
type runeGIDPair struct {
	r   rune
	gid uint16
}

// subsetBuildCmapFormat4 builds a complete cmap table with a single format 4 subtable
// (platform 3, encoding 1) covering only the provided rune->glyph mappings.
func subsetBuildCmapFormat4(mappings []runeGIDPair) []byte {
	// Build segments: runs where both code and GID are consecutive share one segment.
	type seg struct {
		startCode, endCode uint16
		idDelta            int16
	}
	var segs []seg
	for i := 0; i < len(mappings); {
		start := i
		for i+1 < len(mappings) &&
			mappings[i+1].r == mappings[i].r+1 &&
			mappings[i+1].gid == mappings[i].gid+1 {
			i++
		}
		sc := uint16(mappings[start].r)
		ec := uint16(mappings[i].r)
		delta := int16(int(mappings[start].gid) - int(sc))
		segs = append(segs, seg{startCode: sc, endCode: ec, idDelta: delta})
		i++
	}
	// Spec requires a terminator segment.
	segs = append(segs, seg{startCode: 0xFFFF, endCode: 0xFFFF, idDelta: 1})

	segCount := len(segs)
	// Format 4 subtable size: 14-byte fixed header + 4 arrays * segCount*2 + 2 (reservedPad).
	subtableLen := 14 + segCount*8 + 2
	sub := make([]byte, subtableLen)

	binary.BigEndian.PutUint16(sub[0:], 4)                   // format
	binary.BigEndian.PutUint16(sub[2:], uint16(subtableLen)) // length
	binary.BigEndian.PutUint16(sub[4:], 0)                   // language
	binary.BigEndian.PutUint16(sub[6:], uint16(segCount*2))  // segCountX2
	sr, es, rs := subsetSFNTSearchParams(segCount)
	binary.BigEndian.PutUint16(sub[8:], uint16(sr))
	binary.BigEndian.PutUint16(sub[10:], uint16(es))
	binary.BigEndian.PutUint16(sub[12:], uint16(rs))

	endCodeBase := 14
	// reservedPad sits between endCode array and startCode array
	startCodeBase := endCodeBase + segCount*2 + 2
	idDeltaBase := startCodeBase + segCount*2
	idRangeOffBase := idDeltaBase + segCount*2

	binary.BigEndian.PutUint16(sub[endCodeBase+segCount*2:], 0) // reservedPad

	for i, s := range segs {
		binary.BigEndian.PutUint16(sub[endCodeBase+i*2:], s.endCode)
		binary.BigEndian.PutUint16(sub[startCodeBase+i*2:], s.startCode)
		binary.BigEndian.PutUint16(sub[idDeltaBase+i*2:], uint16(s.idDelta))
		binary.BigEndian.PutUint16(sub[idRangeOffBase+i*2:], 0) // idRangeOffset=0
	}

	// Wrap in a cmap table: 4-byte header + 8-byte encoding record + subtable.
	headerLen := 4 + 8
	cmap := make([]byte, headerLen+subtableLen)
	binary.BigEndian.PutUint16(cmap[0:], 0)                     // version
	binary.BigEndian.PutUint16(cmap[2:], 1)                     // numTables
	binary.BigEndian.PutUint16(cmap[4:], 3)                     // platformID = Windows
	binary.BigEndian.PutUint16(cmap[6:], 1)                     // encodingID = Unicode BMP
	binary.BigEndian.PutUint32(cmap[8:], uint32(headerLen))     // offset to subtable
	copy(cmap[headerLen:], sub)
	return cmap
}

// subsetSFNTSearchParams computes searchRange/entrySelector/rangeShift for an SFNT table.
func subsetSFNTSearchParams(n int) (searchRange, entrySelector, rangeShift int) {
	if n == 0 {
		return 0, 0, 0
	}
	es := 0
	for (1 << (es + 1)) <= n {
		es++
	}
	sr := (1 << es) * 16
	rs := n*16 - sr
	return sr, es, rs
}

// subsetTableChecksum computes the standard TTF checksum for a table (sum of uint32 BE words).
func subsetTableChecksum(data []byte) uint32 {
	var sum uint32
	i := 0
	for ; i+3 < len(data); i += 4 {
		sum += binary.BigEndian.Uint32(data[i : i+4])
	}
	if rem := len(data) - i; rem > 0 {
		var tmp [4]byte
		copy(tmp[:], data[i:])
		sum += binary.BigEndian.Uint32(tmp[:])
	}
	return sum
}

// subsetWriteU16 writes a big-endian uint16 to a buffer.
func subsetWriteU16(buf *bytes.Buffer, v uint16) {
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
}

// subsetWriteU32 writes a big-endian uint32 to a buffer.
func subsetWriteU32(buf *bytes.Buffer, v uint32) {
	buf.WriteByte(byte(v >> 24))
	buf.WriteByte(byte(v >> 16))
	buf.WriteByte(byte(v >> 8))
	buf.WriteByte(byte(v))
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
