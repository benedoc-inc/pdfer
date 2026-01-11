package extract

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// extractImageData extracts actual image binary data from an image XObject
func extractImageData(imageObjNum int, pdf *parse.PDF, verbose bool) (*types.Image, error) {
	imageObj, err := pdf.GetObject(imageObjNum)
	if err != nil {
		return nil, fmt.Errorf("failed to get image object %d: %w", imageObjNum, err)
	}

	imageObjBytes := imageObj
	imageStr := string(imageObjBytes)
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

	// Extract stream data - handle binary data properly
	streamIdx := bytes.Index(imageObjBytes, []byte("stream"))
	if streamIdx != -1 {
		// Get /Length from dictionary for exact stream size
		dictPart := imageObjBytes[:streamIdx]
		lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
		lengthMatch := lengthPattern.FindSubmatch(dictPart)

		var streamLength int
		if lengthMatch != nil {
			streamLength, _ = strconv.Atoi(string(lengthMatch[1]))
		}

		// Find actual stream data (after "stream\r\n" or "stream\n")
		streamDataStart := streamIdx + 6 // len("stream")
		// Skip exactly one EOL per PDF spec
		if streamDataStart < len(imageObjBytes) && imageObjBytes[streamDataStart] == '\r' {
			streamDataStart++
		}
		if streamDataStart < len(imageObjBytes) && imageObjBytes[streamDataStart] == '\n' {
			streamDataStart++
		}

		// Use /Length if available, otherwise find endstream
		var streamData []byte
		if streamLength > 0 && streamDataStart+streamLength <= len(imageObjBytes) {
			streamData = imageObjBytes[streamDataStart : streamDataStart+streamLength]
		} else {
			endstreamIdx := bytes.Index(imageObjBytes[streamDataStart:], []byte("endstream"))
			if endstreamIdx != -1 {
				streamData = imageObjBytes[streamDataStart : streamDataStart+endstreamIdx]
			}
		}

		if len(streamData) > 0 {
			// Decompress if needed
			if filter != "" {
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
					// Other filters - use raw data
					image.Data = streamData
				}
			} else {
				// No filter - use raw data
				image.Data = streamData
			}

			// Set base64 encoded version for JSON serialization
			if len(image.Data) > 0 {
				image.DataBase64 = base64.StdEncoding.EncodeToString(image.Data)
			}
		}
	}

	return image, nil
}

// ExtractAllImages extracts all images from a PDF document with binary data
func ExtractAllImages(pdfBytes []byte, password []byte, verbose bool) ([]types.Image, error) {
	// Parse PDF
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
		Password: password,
		Verbose:  verbose,
	})
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// First get metadata to find all images
	doc, err := ExtractContent(pdfBytes, password, verbose)
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to extract content: %w", err)
	}

	allImages := make([]types.Image, 0)
	imageMap := make(map[string]bool)      // Track unique images by ID
	imageObjNumMap := make(map[string]int) // Map image name to object number

	// Collect image object numbers by re-parsing Resources from pages
	// We need to get the actual page object numbers to extract Resources
	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return []types.Image{}, fmt.Errorf("no root reference found")
	}

	rootObjNum, err := parseObjectRef(trailer.RootRef)
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to parse root reference: %w", err)
	}

	catalogObj, err := pdf.GetObject(rootObjNum)
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to get catalog: %w", err)
	}

	catalogStr := string(catalogObj)
	pagesRef := extractDictValue(catalogStr, "/Pages")
	if pagesRef == "" {
		return []types.Image{}, fmt.Errorf("no /Pages reference in catalog")
	}

	pagesObjNum, err := parseObjectRef(pagesRef)
	if err != nil {
		return []types.Image{}, fmt.Errorf("failed to parse Pages reference: %w", err)
	}

	// Recursively find all page objects
	pageObjNums := extractPageObjectNumbers(pdf, pagesObjNum, verbose)

	// For each page, extract Resources and get image object numbers
	for i, pageObjNum := range pageObjNums {
		if i >= len(doc.Pages) {
			break
		}

		pageObj, err := pdf.GetObject(pageObjNum)
		if err != nil {
			continue
		}

		pageStr := string(pageObj)
		// Extract Resources
		resourcesRef := extractDictValue(pageStr, "/Resources")
		var resourcesStr string
		if resourcesRef != "" {
			resourcesObjNum, err := parseObjectRef(resourcesRef)
			if err == nil {
				resourcesObj, err := pdf.GetObject(resourcesObjNum)
				if err == nil {
					resourcesStr = string(resourcesObj)
				}
			}
		} else {
			// Inline Resources
			resourcesStr = extractInlineDict(pageStr, "/Resources")
		}

		if resourcesStr != "" {
			// Extract XObject object numbers
			_, objNums := extractXObjectsDictWithObjNums(resourcesStr, pdf, verbose)
			for name, objNum := range objNums {
				imageID := "/" + name
				if !imageMap[imageID] {
					imageObjNumMap[name] = objNum
					imageMap[imageID] = true
				}
			}
		}
	}

	// Extract full image data with binary for each unique image
	for name, objNum := range imageObjNumMap {
		image, err := extractImageData(objNum, pdf, verbose)
		if err != nil {
			if verbose {
				fmt.Printf("Warning: failed to extract image data for %s (obj %d): %v\n", name, objNum, err)
			}
			// Fall back to metadata-only image from Resources
			for _, page := range doc.Pages {
				if page.Resources != nil {
					if img, ok := page.Resources.Images[name]; ok {
						allImages = append(allImages, img)
						break
					}
				}
			}
		} else {
			allImages = append(allImages, *image)
		}
	}

	// Also add any images we found but couldn't get object numbers for (metadata only)
	for _, page := range doc.Pages {
		if page.Resources != nil {
			for name, image := range page.Resources.Images {
				imageID := "/" + name
				if !imageMap[imageID] {
					allImages = append(allImages, image)
					imageMap[imageID] = true
				}
			}
		}
	}

	return allImages, nil
}

// extractPageObjectNumbers recursively extracts page object numbers from the pages tree
func extractPageObjectNumbers(pdf *parse.PDF, pagesObjNum int, verbose bool) []int {
	var pageObjNums []int

	pagesObj, err := pdf.GetObject(pagesObjNum)
	if err != nil {
		return pageObjNums
	}

	pagesStr := string(pagesObj)

	// Check if this is a /Page (leaf) or /Pages (intermediate)
	pageType := extractDictValue(pagesStr, "/Type")
	if pageType == "/Page" {
		// This is a page - add it
		pageObjNums = append(pageObjNums, pagesObjNum)
		return pageObjNums
	}

	// This is a /Pages node - get Kids and recurse
	kidsStr := extractDictValue(pagesStr, "/Kids")
	if kidsStr == "" {
		return pageObjNums
	}

	// Parse kids array
	kidsRefs := parseObjectRefArray(kidsStr)
	for _, kidRef := range kidsRefs {
		kidObjNum, err := parseObjectRef(kidRef)
		if err != nil {
			continue
		}
		// Recurse
		childPages := extractPageObjectNumbers(pdf, kidObjNum, verbose)
		pageObjNums = append(pageObjNums, childPages...)
	}

	return pageObjNums
}
