package manipulate

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ReplaceImage replaces an image XObject in a PDF.
//
// imageRef identifies the image either by its XObject resource name (e.g. "Im1",
// "Img0") or by the raw object number as a decimal string (e.g. "7").
//
// format must be "jpeg", "png", or "raw".
//   - "jpeg": newImageData must be a JPEG file; the stream is stored with /Filter/DCTDecode.
//   - "png":  newImageData must be a PNG file; the image is decoded and stored as raw
//     deflate-compressed RGB pixels with /Filter/FlateDecode.
//   - "raw":  the stream bytes are replaced as-is; the existing filter is preserved.
func ReplaceImage(pdfBytes []byte, imageRef string, newImageData []byte, format string, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("replace image: parse failed: %w", err)
	}

	objNum, err := m.findImageObjectNum(imageRef)
	if err != nil {
		return nil, fmt.Errorf("replace image: %w", err)
	}

	if err := m.replaceImageStream(objNum, newImageData, format); err != nil {
		return nil, fmt.Errorf("replace image: %w", err)
	}

	return m.Rebuild()
}

// findImageObjectNum returns the object number for the image identified by imageRef.
// imageRef can be a decimal object-number string or an XObject resource name.
func (m *PDFManipulator) findImageObjectNum(imageRef string) (int, error) {
	// If imageRef is a plain decimal integer, use it directly.
	if n, err := strconv.Atoi(imageRef); err == nil {
		if _, ok := m.objects[n]; !ok {
			return 0, fmt.Errorf("object %d not found", n)
		}
		return n, nil
	}

	// Otherwise scan all objects for an image XObject with a matching /Name.
	for objNum, objBytes := range m.objects {
		s := string(objBytes)
		if !strings.Contains(s, "/XObject") && !strings.Contains(s, "/Image") {
			continue
		}
		if strings.Contains(s, "/Subtype/Image") || strings.Contains(s, "/Subtype /Image") {
			if imgNameMatches(s, imageRef) {
				return objNum, nil
			}
		}
	}

	// Scan page resource dictionaries for /XObject entries that map the name.
	pageObjNums, err := m.getAllPageObjectNumbers()
	if err == nil {
		for _, pageObjNum := range pageObjNums {
			pageStr := string(m.objects[pageObjNum])
			if n := m.findImageInResources(pageStr, imageRef); n != 0 {
				return n, nil
			}
		}
	}

	return 0, fmt.Errorf("image %q not found", imageRef)
}

// imgNameMatches returns true if the object dict contains "/Name /imageRef" or "/Name/imageRef".
func imgNameMatches(objStr, name string) bool {
	re := regexp.MustCompile(`/Name\s*/` + regexp.QuoteMeta(name) + `[\s/]`)
	return re.MatchString(objStr)
}

// findImageInResources searches page resource dicts for /XObject/<name> and returns its object number.
func (m *PDFManipulator) findImageInResources(pageStr, name string) int {
	// Locate /XObject dict inside /Resources.
	resIdx := strings.Index(pageStr, "/XObject")
	if resIdx == -1 {
		return 0
	}
	// Find the << after /XObject.
	open, close := stampFindDictBounds(pageStr, resIdx+len("/XObject"))
	if open == -1 {
		return 0
	}
	xobjDict := pageStr[open:close]

	// Look for /name N 0 R
	pattern := regexp.MustCompile(`/` + regexp.QuoteMeta(name) + `\s+(\d+)\s+\d+\s+R`)
	m2 := pattern.FindStringSubmatch(xobjDict)
	if len(m2) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m2[1])
	if err != nil {
		return 0
	}
	return n
}

// replaceImageStream replaces the stream bytes (and updates the dict) for an image object.
func (m *PDFManipulator) replaceImageStream(objNum int, newData []byte, format string) error {
	objBytes, ok := m.objects[objNum]
	if !ok {
		return fmt.Errorf("object %d not found", objNum)
	}

	s := string(objBytes)

	// Locate the stream/endstream boundary.
	si := strings.Index(s, "stream")
	if si == -1 {
		return fmt.Errorf("object %d is not a stream object", objNum)
	}
	header := s[:si]

	// Extract generation number.
	genNum := redactGenNum(objBytes)

	var streamBytes []byte
	var newDictEntries map[string]string

	switch strings.ToLower(format) {
	case "jpeg":
		w, h := jpegDimensions(newData)
		streamBytes = newData
		newDictEntries = map[string]string{
			"/Filter": "/DCTDecode",
			"/Length": strconv.Itoa(len(streamBytes)),
		}
		if w > 0 {
			newDictEntries["/Width"] = strconv.Itoa(w)
		}
		if h > 0 {
			newDictEntries["/Height"] = strconv.Itoa(h)
		}

	case "png":
		w, h, pixelData, err := pngDecodeRGB(newData)
		if err != nil {
			// Fall back to raw mode if PNG decode fails.
			streamBytes = newData
			newDictEntries = map[string]string{
				"/Length": strconv.Itoa(len(streamBytes)),
			}
		} else {
			// Compress raw pixel data with zlib for /FlateDecode.
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			if _, werr := zw.Write(pixelData); werr != nil {
				return fmt.Errorf("zlib compress: %w", werr)
			}
			if cerr := zw.Close(); cerr != nil {
				return fmt.Errorf("zlib close: %w", cerr)
			}
			streamBytes = buf.Bytes()
			newDictEntries = map[string]string{
				"/Filter":           "/FlateDecode",
				"/ColorSpace":       "/DeviceRGB",
				"/BitsPerComponent": "8",
				"/Length":           strconv.Itoa(len(streamBytes)),
				"/Width":            strconv.Itoa(w),
				"/Height":           strconv.Itoa(h),
			}
		}

	case "raw", "":
		streamBytes = newData
		newDictEntries = map[string]string{
			"/Length": strconv.Itoa(len(streamBytes)),
		}

	default:
		return fmt.Errorf("unsupported format %q (must be jpeg, png, or raw)", format)
	}

	// Update the header dict.
	// Remove any existing inline dict inside the raw object string (between "obj" and "stream").
	dictStart := strings.Index(header, "<<")
	dictEnd := strings.LastIndex(header, ">>")
	var dictStr string
	if dictStart != -1 && dictEnd != -1 && dictEnd > dictStart {
		dictStr = header[dictStart : dictEnd+2]
	} else {
		dictStr = "<<>>"
	}

	for k, v := range newDictEntries {
		dictStr = setDictValue(dictStr, k, v)
	}

	// Rebuild the full object.
	objHeader := fmt.Sprintf("%d %d obj\n", objNum, genNum)
	newObj := objHeader + dictStr + "\nstream\n" + string(streamBytes) + "\nendstream\nendobj\n"
	m.objects[objNum] = []byte(newObj)

	if m.verbose {
		fmt.Printf("Replaced image stream in object %d (%d bytes, format=%s)\n", objNum, len(streamBytes), format)
	}
	return nil
}

// jpegDimensions reads JPEG SOF marker to return (width, height). Returns (0,0) on failure.
func jpegDimensions(data []byte) (int, int) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0
	}
	i := 2
	for i+3 < len(data) {
		if data[i] != 0xFF {
			break
		}
		marker := data[i+1]
		i += 2
		if marker == 0xD9 { // EOI
			break
		}
		if i+1 >= len(data) {
			break
		}
		segLen := int(data[i])<<8 | int(data[i+1])
		// SOF markers: 0xC0–0xC3, 0xC5–0xC7, 0xC9–0xCB, 0xCD–0xCF
		if (marker >= 0xC0 && marker <= 0xC3) || (marker >= 0xC5 && marker <= 0xC7) ||
			(marker >= 0xC9 && marker <= 0xCB) || (marker >= 0xCD && marker <= 0xCF) {
			if i+8 < len(data) {
				h := int(data[i+3])<<8 | int(data[i+4])
				w := int(data[i+5])<<8 | int(data[i+6])
				return w, h
			}
		}
		i += segLen
	}
	return 0, 0
}

// pngDecodeRGB decodes a PNG file and returns (width, height, raw RGB bytes, error).
// It decompresses IDAT chunks and strips the per-row filter byte, converting to
// flat RGB pixels for use in a PDF FlateDecode image stream.
func pngDecodeRGB(data []byte) (int, int, []byte, error) {
	// PNG signature
	if len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n" {
		return 0, 0, nil, fmt.Errorf("not a PNG file")
	}

	var width, height int
	bitDepth := 0
	colorType := 0
	var idatBuf bytes.Buffer

	i := 8 // skip signature
	for i+8 <= len(data) {
		chunkLen := int(binary.BigEndian.Uint32(data[i:]))
		chunkType := string(data[i+4 : i+8])
		chunkData := data[i+8:]
		if chunkLen > len(chunkData) {
			break
		}
		chunkData = chunkData[:chunkLen]

		switch chunkType {
		case "IHDR":
			if len(chunkData) < 13 {
				return 0, 0, nil, fmt.Errorf("IHDR too short")
			}
			width = int(binary.BigEndian.Uint32(chunkData[0:]))
			height = int(binary.BigEndian.Uint32(chunkData[4:]))
			bitDepth = int(chunkData[8])
			colorType = int(chunkData[9])
		case "IDAT":
			idatBuf.Write(chunkData)
		case "IEND":
			// done
		}
		i += 8 + chunkLen + 4 // length + type + data + CRC
	}

	if width == 0 || height == 0 {
		return 0, 0, nil, fmt.Errorf("invalid PNG dimensions")
	}
	if bitDepth != 8 {
		return 0, 0, nil, fmt.Errorf("PNG bit depth %d not supported (only 8-bit)", bitDepth)
	}

	// Determine channels per pixel.
	var channels int
	switch colorType {
	case 2: // RGB
		channels = 3
	case 6: // RGBA
		channels = 4
	case 0: // Grayscale
		channels = 1
	default:
		return 0, 0, nil, fmt.Errorf("PNG color type %d not supported", colorType)
	}

	// Decompress the concatenated IDAT data.
	zr, err := zlib.NewReader(bytes.NewReader(idatBuf.Bytes()))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("PNG zlib open: %w", err)
	}
	defer zr.Close()

	stride := channels * width
	var rawBuf bytes.Buffer
	if _, err := rawBuf.ReadFrom(zr); err != nil {
		return 0, 0, nil, fmt.Errorf("PNG decompress: %w", err)
	}
	raw := rawBuf.Bytes()
	_ = stride // stride used below

	// Un-filter rows and extract RGB pixels.
	outBuf := make([]byte, 0, width*height*3)
	prev := make([]byte, stride)

	for row := 0; row < height; row++ {
		rowStart := row * (1 + stride)
		if rowStart >= len(raw) {
			break
		}
		filterType := raw[rowStart]
		rowData := raw[rowStart+1:]
		if len(rowData) < stride {
			break
		}
		rowData = rowData[:stride]

		cur := make([]byte, stride)
		pngUndoFilter(filterType, rowData, prev, cur, channels)

		// Convert to RGB (strip alpha if RGBA, or expand grayscale to RGB).
		for x := 0; x < width; x++ {
			switch colorType {
			case 2: // RGB
				outBuf = append(outBuf, cur[x*3], cur[x*3+1], cur[x*3+2])
			case 6: // RGBA → RGB (drop alpha)
				outBuf = append(outBuf, cur[x*4], cur[x*4+1], cur[x*4+2])
			case 0: // Grayscale → RGB
				g := cur[x]
				outBuf = append(outBuf, g, g, g)
			}
		}
		copy(prev, cur)
	}

	return width, height, outBuf, nil
}

// pngUndoFilter reverses a PNG filter for one row.
func pngUndoFilter(filter byte, row, prev, out []byte, bpp int) {
	n := len(out)
	switch filter {
	case 0: // None
		copy(out, row)
	case 1: // Sub
		for i := range out {
			a := byte(0)
			if i >= bpp {
				a = out[i-bpp]
			}
			out[i] = row[i] + a
		}
	case 2: // Up
		for i := 0; i < n; i++ {
			out[i] = row[i] + prev[i]
		}
	case 3: // Average
		for i := range out {
			a := byte(0)
			if i >= bpp {
				a = out[i-bpp]
			}
			out[i] = row[i] + byte((int(a)+int(prev[i]))/2)
		}
	case 4: // Paeth
		for i := range out {
			a := byte(0)
			if i >= bpp {
				a = out[i-bpp]
			}
			b := prev[i]
			c := byte(0)
			if i >= bpp {
				c = prev[i-bpp]
			}
			out[i] = row[i] + pngPaethPredictor(a, b, c)
		}
	default:
		copy(out, row)
	}
}

func pngPaethPredictor(a, b, c byte) byte {
	ia, ib, ic := int(a), int(b), int(c)
	p := ia + ib - ic
	pa := p - ia
	if pa < 0 {
		pa = -pa
	}
	pb := p - ib
	if pb < 0 {
		pb = -pb
	}
	pc := p - ic
	if pc < 0 {
		pc = -pc
	}
	if pa <= pb && pa <= pc {
		return a
	} else if pb <= pc {
		return b
	}
	return c
}
