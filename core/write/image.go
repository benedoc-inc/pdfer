// Package writer provides PDF writing capabilities including image embedding
package write

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"

	// Import for JPEG decoding side effects
	_ "image/jpeg"
	_ "image/png"
)

// ImageInfo contains information about an embedded image
type ImageInfo struct {
	ObjectNum  int    // Object number of the image XObject
	Width      int    // Image width in pixels
	Height     int    // Image height in pixels
	ColorSpace string // PDF color space name (e.g., "/DeviceRGB")
	Name       string // Resource name (e.g., "/Im1")
}

// AddJPEGImage adds a JPEG image to the PDF and returns its info
// JPEG images are embedded directly without re-encoding (DCTDecode)
func (w *PDFWriter) AddJPEGImage(jpegData []byte, name string) (*ImageInfo, error) {
	// Parse JPEG header to get dimensions and color info
	width, height, colorSpace, err := parseJPEGHeader(jpegData)
	if err != nil {
		return nil, fmt.Errorf("invalid JPEG: %v", err)
	}

	// Create image XObject dictionary
	dict := Dictionary{
		"Type":             "/XObject",
		"Subtype":          "/Image",
		"Width":            width,
		"Height":           height,
		"ColorSpace":       colorSpace,
		"BitsPerComponent": 8,
		"Filter":           "/DCTDecode",
		"Length":           len(jpegData),
	}

	// Add as stream object (don't compress - JPEG is already compressed)
	objNum := w.nextObjNum
	w.nextObjNum++

	w.objects[objNum] = &PDFObject{
		Number:     objNum,
		Generation: 0,
		Dict:       dict,
		Stream:     jpegData,
	}

	return &ImageInfo{
		ObjectNum:  objNum,
		Width:      width,
		Height:     height,
		ColorSpace: colorSpace,
		Name:       name,
	}, nil
}

// AddImage adds a generic image (PNG, etc.) to the PDF
// The image is converted to raw RGB/Gray data and compressed with FlateDecode
func (w *PDFWriter) AddImage(imgData []byte, name string) (*ImageInfo, error) {
	// Decode image
	img, format, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	// If it's a JPEG, use the direct embedding method
	if format == "jpeg" {
		return w.AddJPEGImage(imgData, name)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Determine color space and extract raw pixel data
	var rawData []byte
	var colorSpace string
	var hasAlpha bool

	// Check if image has alpha channel
	switch img.(type) {
	case *image.NRGBA, *image.RGBA:
		hasAlpha = true
	}

	// Convert to RGB or Gray
	switch img.ColorModel() {
	case color.GrayModel, color.Gray16Model:
		colorSpace = "/DeviceGray"
		rawData = make([]byte, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				gray := color.GrayModel.Convert(img.At(x+bounds.Min.X, y+bounds.Min.Y)).(color.Gray)
				rawData[y*width+x] = gray.Y
			}
		}
	default:
		colorSpace = "/DeviceRGB"
		rawData = make([]byte, width*height*3)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				r, g, b, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
				idx := (y*width + x) * 3
				rawData[idx] = uint8(r >> 8)
				rawData[idx+1] = uint8(g >> 8)
				rawData[idx+2] = uint8(b >> 8)
			}
		}
	}

	// Create image XObject with FlateDecode compression
	dict := Dictionary{
		"Type":             "/XObject",
		"Subtype":          "/Image",
		"Width":            width,
		"Height":           height,
		"ColorSpace":       colorSpace,
		"BitsPerComponent": 8,
	}

	objNum := w.AddStreamObject(dict, rawData, true)

	info := &ImageInfo{
		ObjectNum:  objNum,
		Width:      width,
		Height:     height,
		ColorSpace: colorSpace,
		Name:       name,
	}

	// If image has alpha, create a soft mask
	if hasAlpha {
		alphaMask := make([]byte, width*height)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				_, _, _, a := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
				alphaMask[y*width+x] = uint8(a >> 8)
			}
		}

		maskDict := Dictionary{
			"Type":             "/XObject",
			"Subtype":          "/Image",
			"Width":            width,
			"Height":           height,
			"ColorSpace":       "/DeviceGray",
			"BitsPerComponent": 8,
		}
		maskObjNum := w.AddStreamObject(maskDict, alphaMask, true)

		// Update main image to reference soft mask
		w.objects[objNum].Dict["SMask"] = fmt.Sprintf("%d 0 R", maskObjNum)
	}

	return info, nil
}

// parseJPEGHeader parses a JPEG header to extract width, height, and color space
func parseJPEGHeader(data []byte) (width, height int, colorSpace string, err error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, "", fmt.Errorf("not a valid JPEG (missing SOI)")
	}

	pos := 2
	for pos < len(data)-1 {
		if data[pos] != 0xFF {
			pos++
			continue
		}

		marker := data[pos+1]
		pos += 2

		// Skip padding
		if marker == 0xFF {
			continue
		}

		// SOF0-SOF15 (except SOF4, SOF8, SOF12 which are not frame markers)
		// We're interested in SOF0 (baseline), SOF1, SOF2 (progressive)
		if marker >= 0xC0 && marker <= 0xC3 {
			if pos+7 > len(data) {
				return 0, 0, "", fmt.Errorf("truncated SOF segment")
			}

			// Skip length (2 bytes), precision (1 byte)
			height = int(binary.BigEndian.Uint16(data[pos+3 : pos+5]))
			width = int(binary.BigEndian.Uint16(data[pos+5 : pos+7]))
			components := int(data[pos+7])

			switch components {
			case 1:
				colorSpace = "/DeviceGray"
			case 3:
				colorSpace = "/DeviceRGB"
			case 4:
				colorSpace = "/DeviceCMYK"
			default:
				colorSpace = "/DeviceRGB"
			}

			return width, height, colorSpace, nil
		}

		// Skip this segment
		if pos+1 >= len(data) {
			break
		}
		segmentLength := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += segmentLength
	}

	return 0, 0, "", fmt.Errorf("no SOF marker found")
}
