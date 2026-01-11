// Package font provides PDF font object creation
package font

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sort"
)

// FontObjects represents the PDF objects needed for an embedded font
type FontObjects struct {
	FontDictNum       int    // Font dictionary object number
	FontDescriptorNum int    // FontDescriptor object number
	FontFileNum       int    // Font file stream object number
	ToUnicodeNum      int    // ToUnicode CMap object number (optional)
	ResourceName      string // Resource name like "F1"
	SubsetPrefix      string // Subset prefix like "ABCDEF+"
}

// ToPDFObjects creates PDF objects for an embedded font
// Returns the font dictionary, font descriptor, font file stream, and ToUnicode CMap
func (f *Font) ToPDFObjects(writer PDFWriter) (*FontObjects, error) {
	ttf, err := ParseTTF(f.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font: %w", err)
	}

	// Generate subset prefix from font name hash
	if f.FontID == "" {
		hash := md5.Sum([]byte(ttf.FontName))
		f.FontID = hex.EncodeToString(hash[:6])
	}
	subsetPrefix := f.FontID + "+"

	// Get subset glyphs
	glyphs, err := f.GetSubsetGlyphs()
	if err != nil {
		return nil, fmt.Errorf("failed to get subset glyphs: %w", err)
	}

	// Get widths
	widths, err := f.GetWidths()
	if err != nil {
		return nil, fmt.Errorf("failed to get widths: %w", err)
	}

	// Create font file stream (subset font)
	fontFileData, err := f.CreateSubsetFont()
	if err != nil {
		return nil, fmt.Errorf("failed to create subset font: %w", err)
	}

	// Create font file stream object first
	fontFileDict := map[string]interface{}{
		"/Length1": len(fontFileData),
		"/Length":  len(fontFileData),
	}
	fontFileNum := writer.AddStreamObject(fontFileDict, fontFileData, true)

	// Create FontDescriptor (references fontFileNum)
	fontDescriptor := f.createFontDescriptor(ttf, fontFileNum)
	fontDescriptorNum := writer.AddObject(fontDescriptor)

	// Create ToUnicode CMap (as stream object)
	toUnicodeData := f.createToUnicodeCMapData(glyphs)
	toUnicodeDict := map[string]interface{}{
		"/Type":     "/CMap",
		"/CMapName": "/Adobe-Identity-UCS",
		"/CIDSystemInfo": map[string]interface{}{
			"/Registry":   "(Adobe)",
			"/Ordering":   "(UCS)",
			"/Supplement": 0,
		},
	}
	toUnicodeNum := writer.AddStreamObject(toUnicodeDict, toUnicodeData, false) // Don't compress CMap

	// Create Font dictionary (references fontDescriptorNum and toUnicodeNum)
	fontDict := f.createFontDict(ttf, fontDescriptorNum, toUnicodeNum, glyphs, widths, subsetPrefix)
	fontDictNum := writer.AddObject(fontDict)

	// Generate resource name
	resourceName := fmt.Sprintf("F%d", writer.NextObjectNumber())

	return &FontObjects{
		FontDictNum:       fontDictNum,
		FontDescriptorNum: fontDescriptorNum,
		FontFileNum:       fontFileNum,
		ToUnicodeNum:      toUnicodeNum,
		ResourceName:      resourceName,
		SubsetPrefix:      subsetPrefix,
	}, nil
}

// PDFWriter interface for creating PDF objects
type PDFWriter interface {
	AddObject(content []byte) int
	AddStreamObject(dict map[string]interface{}, data []byte, compress bool) int
	NextObjectNumber() int
}

// createFontDict creates the Font dictionary
func (f *Font) createFontDict(ttf *TTF, fontDescriptorNum, toUnicodeNum int, glyphs []uint16, widths []int, subsetPrefix string) []byte {
	var buf bytes.Buffer

	// Base font name with subset prefix
	baseFontName := subsetPrefix + ttf.FontName

	buf.WriteString("<<\n")
	buf.WriteString("/Type /Font\n")
	buf.WriteString("/Subtype /TrueType\n")
	buf.WriteString(fmt.Sprintf("/BaseFont /%s\n", escapeName(baseFontName)))
	buf.WriteString(fmt.Sprintf("/FirstChar %d\n", 0))
	buf.WriteString(fmt.Sprintf("/LastChar %d\n", len(glyphs)-1))

	// Width array
	buf.WriteString("/Widths [")
	for i, width := range widths {
		if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(fmt.Sprintf("%d", width))
	}
	buf.WriteString("]\n")

	// FontDescriptor reference
	buf.WriteString(fmt.Sprintf("/FontDescriptor %d 0 R\n", fontDescriptorNum))

	// ToUnicode reference
	buf.WriteString(fmt.Sprintf("/ToUnicode %d 0 R\n", toUnicodeNum))

	buf.WriteString(">>")

	return buf.Bytes()
}

// createFontDescriptor creates the FontDescriptor dictionary
func (f *Font) createFontDescriptor(ttf *TTF, fontFileNum int) []byte {
	var buf bytes.Buffer

	buf.WriteString("<<\n")
	buf.WriteString("/Type /FontDescriptor\n")
	buf.WriteString(fmt.Sprintf("/FontName /%s\n", escapeName(ttf.FontName)))
	buf.WriteString(fmt.Sprintf("/FontFamily (%s)\n", escapeString(ttf.FamilyName)))
	buf.WriteString(fmt.Sprintf("/Flags %d\n", f.getFontFlags(ttf)))
	buf.WriteString(fmt.Sprintf("/FontBBox [%d %d %d %d]\n",
		-100, -100, int(ttf.UnitsPerEm)+100, int(ttf.UnitsPerEm)+100)) // Simplified bbox
	buf.WriteString(fmt.Sprintf("/ItalicAngle %.4f\n", ttf.ItalicAngle))
	buf.WriteString(fmt.Sprintf("/Ascent %d\n", ttf.Ascent))
	buf.WriteString(fmt.Sprintf("/Descent %d\n", ttf.Descent))
	buf.WriteString(fmt.Sprintf("/CapHeight %d\n", ttf.CapHeight))
	buf.WriteString(fmt.Sprintf("/StemV %d\n", 80)) // Default stem width
	buf.WriteString(fmt.Sprintf("/FontFile2 %d 0 R\n", fontFileNum))
	buf.WriteString(">>")

	return buf.Bytes()
}

// createToUnicodeCMapData creates ToUnicode CMap stream data for text extraction
func (f *Font) createToUnicodeCMapData(glyphs []uint16) []byte {
	// Get glyph to Unicode mapping
	ttf, err := ParseTTF(f.Data)
	if err != nil {
		return []byte("<<\n/Type /CMap\n>>")
	}

	cmap, ok := ttf.Tables["cmap"]
	if !ok {
		return []byte("<<\n/Type /CMap\n>>")
	}

	glyphMap, err := parseCmap(cmap.Data)
	if err != nil {
		return []byte("<<\n/Type /CMap\n>>")
	}

	// Create reverse mapping: glyph ID -> Unicode
	glyphToUnicode := make(map[uint16][]rune)
	for unicode, gid := range glyphMap {
		glyphToUnicode[gid] = append(glyphToUnicode[gid], unicode)
	}

	// Build CMap content
	var buf bytes.Buffer
	buf.WriteString("/CIDInit /ProcSet findresource begin\n")
	buf.WriteString("12 dict begin\n")
	buf.WriteString("begincmap\n")
	buf.WriteString("/CIDSystemInfo\n")
	buf.WriteString("<< /Registry (Adobe)\n")
	buf.WriteString("   /Ordering (UCS)\n")
	buf.WriteString("   /Supplement 0\n")
	buf.WriteString(">> def\n")
	buf.WriteString("/CMapName /Adobe-Identity-UCS def\n")
	buf.WriteString("/CMapVersion 1.0 def\n")
	buf.WriteString("/CMapType 2 def\n")
	buf.WriteString("1 begincodespacerange\n")
	buf.WriteString("<00> <FF>\n")
	buf.WriteString("endcodespacerange\n")

	// Write bfchar entries (glyph -> Unicode)
	sort.Slice(glyphs, func(i, j int) bool {
		return glyphs[i] < glyphs[j]
	})

	buf.WriteString(fmt.Sprintf("%d beginbfchar\n", len(glyphs)))
	for _, gid := range glyphs {
		unicodes := glyphToUnicode[gid]
		if len(unicodes) > 0 {
			buf.WriteString(fmt.Sprintf("<%02X> <%04X>\n", gid, unicodes[0]))
		} else {
			// Map to space if no Unicode found
			buf.WriteString(fmt.Sprintf("<%02X> <0020>\n", gid))
		}
	}
	buf.WriteString("endbfchar\n")

	buf.WriteString("endcmap\n")
	buf.WriteString("CMapName currentdict /CMap defineresource pop\n")
	buf.WriteString("end\n")
	buf.WriteString("end\n")

	return buf.Bytes()
}

// getFontFlags calculates font flags
func (f *Font) getFontFlags(ttf *TTF) int {
	flags := 0

	// Bit 0: FixedPitch
	if ttf.IsFixedPitch {
		flags |= 1
	}

	// Bit 1: Serif
	// Bit 2: Symbolic
	// Bit 3: Script
	// Bit 5: Nonsymbolic (we assume TrueType fonts are nonsymbolic)
	flags |= 32

	// Bit 6: Italic
	if ttf.ItalicAngle != 0 {
		flags |= 64
	}

	// Bit 17: AllCap (not set by default)
	// Bit 18: SmallCap (not set by default)

	return flags
}

// escapeName escapes a PDF name
func escapeName(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		if r == ' ' || r == '#' || r == '/' || r == '(' || r == ')' || r == '<' || r == '>' || r == '[' || r == ']' || r == '{' || r == '}' || r == '%' {
			buf.WriteString(fmt.Sprintf("#%02X", r))
		} else if r > 127 {
			buf.WriteString(fmt.Sprintf("#%02X", r))
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// escapeString escapes a PDF string
func escapeString(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '\\':
			buf.WriteString("\\\\")
		case '(':
			buf.WriteString("\\(")
		case ')':
			buf.WriteString("\\)")
		case '\n':
			buf.WriteString("\\n")
		case '\r':
			buf.WriteString("\\r")
		case '\t':
			buf.WriteString("\\t")
		default:
			if r > 127 {
				buf.WriteString(fmt.Sprintf("\\%03o", r))
			} else {
				buf.WriteRune(r)
			}
		}
	}
	return buf.String()
}
