package extract

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// extractImageData extracts actual image binary data from an image XObject.
// This function is available for extracting full image binary data when needed.
// Currently not called in the main extraction flow (which extracts metadata only).
// To use: call this function with an image XObject number to get full binary data.
var _ = extractImageData // Mark as available for future use

func extractImageData(imageObjNum int, pdf *parse.PDF, verbose bool) (*types.Image, error) {
	imageObj, err := pdf.GetObject(imageObjNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get image object %d: %w", imageObjNum, err)
	}

	imageStr := string(imageObj)
	image := &types.Image{
		ID:       fmt.Sprintf("/Im%d", imageObjNum),
		Metadata: make(map[string]interface{}),
	}

	// Extract Width and Height
	widthStr := extractDictValue(imageStr, "/Width")
	heightStr := extractDictValue(imageStr, "/Height")
	if widthStr != "" {
		if w, err := strconv.Atoi(widthStr); err == nil {
			image.Width = w
		}
	}
	if heightStr != "" {
		if h, err := strconv.Atoi(heightStr); err == nil {
			image.Height = h
		}
	}

	// Extract ColorSpace
	colorSpace := extractDictValue(imageStr, "/ColorSpace")
	if colorSpace != "" {
		image.ColorSpace = colorSpace
	}

	// Extract BitsPerComponent
	bitsPerCompStr := extractDictValue(imageStr, "/BitsPerComponent")
	if bitsPerCompStr != "" {
		if b, err := strconv.Atoi(bitsPerCompStr); err == nil {
			image.BitsPerComponent = b
		}
	}

	// Extract Filter to determine format and decompression method
	filter := extractDictValue(imageStr, "/Filter")
	if filter != "" {
		image.Filter = filter
		// Determine format from filter
		if filter == "/DCTDecode" || strings.Contains(filter, "DCTDecode") {
			image.Format = "jpeg"
		} else if filter == "/FlateDecode" || strings.Contains(filter, "FlateDecode") {
			image.Format = "png" // Could be PNG or other FlateDecode image
		} else if filter == "/CCITTFaxDecode" || strings.Contains(filter, "CCITTFaxDecode") {
			image.Format = "tiff"
		} else if filter == "/JPXDecode" || strings.Contains(filter, "JPXDecode") {
			image.Format = "jpeg2000"
		} else {
			image.Format = "unknown"
		}
	}

	// Extract stream data
	if strings.Contains(imageStr, "stream") {
		streamIdx := strings.Index(imageStr, "stream")
		if streamIdx != -1 {
			// Get stream data (after "stream\n")
			dataStart := streamIdx + 6
			if dataStart < len(imageStr) && (imageStr[dataStart] == '\r' || imageStr[dataStart] == '\n') {
				dataStart++
			}
			if dataStart < len(imageStr) && imageStr[dataStart] == '\n' {
				dataStart++
			}

			endstreamIdx := strings.Index(imageStr[dataStart:], "endstream")
			if endstreamIdx != -1 {
				streamData := []byte(imageStr[dataStart : dataStart+endstreamIdx])

				// Decompress if needed
				if filter != "" && (strings.Contains(filter, "FlateDecode") || strings.Contains(filter, "DCTDecode")) {
					if strings.Contains(filter, "FlateDecode") {
						// Decompress FlateDecode
						decompressed, err := parse.DecodeFlateDecode(streamData)
						if err == nil {
							image.Data = decompressed
						} else {
							if verbose {
								fmt.Printf("Warning: failed to decompress FlateDecode image: %v\n", err)
							}
							image.Data = streamData
						}
					} else if strings.Contains(filter, "DCTDecode") {
						// DCTDecode (JPEG) - data is already JPEG, no decompression needed
						image.Data = streamData
					} else {
						image.Data = streamData
					}
				} else {
					// No filter or unknown filter - use raw data
					image.Data = streamData
				}

				// Set base64 encoded version for JSON serialization
				if len(image.Data) > 0 {
					image.DataBase64 = base64.StdEncoding.EncodeToString(image.Data)
				}
			}
		}
	}

	return image, nil
}

// ExtractAllImages extracts all images from a PDF document
func ExtractAllImages(pdfBytes []byte, password []byte, verbose bool) ([]types.Image, error) {
	doc, err := ExtractContent(pdfBytes, password, verbose)
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to extract content: %w", err)
	}

	allImages := make([]types.Image, 0)
	imageMap := make(map[string]bool) // Track unique images by ID

	// Collect images from all pages
	for _, page := range doc.Pages {
		// Images from Resources
		if page.Resources != nil {
			for _, image := range page.Resources.Images {
				if !imageMap[image.ID] {
					allImages = append(allImages, image)
					imageMap[image.ID] = true
				}
			}
		}

		// ImageRefs from content streams (references to XObjects)
		for _, imageRef := range page.Images {
			if !imageMap[imageRef.ImageID] {
				// Try to find the actual image in Resources
				if page.Resources != nil {
					if img, ok := page.Resources.Images[strings.TrimPrefix(imageRef.ImageID, "/")]; ok {
						if !imageMap[img.ID] {
							allImages = append(allImages, img)
							imageMap[img.ID] = true
						}
					}
				}
			}
		}
	}

	return allImages, nil
}
