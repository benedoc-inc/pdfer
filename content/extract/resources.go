package extract

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/types"
)

// extractResources extracts resources from a Resources dictionary
func extractResources(resourcesStr string, pdf *parse.PDF, verbose bool) *types.PageResources {
	resources := &types.PageResources{
		Fonts:       make(map[string]types.FontInfo),
		Images:      make(map[string]types.Image),
		XObjects:    make(map[string]types.XObject),
		ColorSpaces: make(map[string]string),
		Patterns:    make(map[string]interface{}),
		Shadings:    make(map[string]interface{}),
	}

	// Extract fonts
	fontsDict := extractFontsDict(resourcesStr, pdf, verbose)
	if fontsDict != nil {
		resources.Fonts = fontsDict
	}

	// Extract XObjects (images and forms)
	xobjectsDict := extractXObjectsDict(resourcesStr, pdf, verbose)
	if xobjectsDict != nil {
		resources.XObjects = xobjectsDict
		// Also populate Images from XObjects (for metadata)
		for name, xobj := range xobjectsDict {
			if xobj.Subtype == "/Image" {
				image := types.Image{
					ID:     "/" + name,
					Width:  int(xobj.Width),
					Height: int(xobj.Height),
					Format: "unknown", // Will be determined when extracting actual image data
				}
				resources.Images[name] = image
			}
		}
	}

	return resources
}

// extractFontsDict extracts font information from /Resources/Font dictionary
func extractFontsDict(resourcesStr string, pdf *parse.PDF, verbose bool) map[string]types.FontInfo {
	fonts := make(map[string]types.FontInfo)

	if verbose {
		fmt.Printf("Extracting fonts from Resources string: %s\n", resourcesStr[:min(200, len(resourcesStr))])
	}

	// Find /Font dictionary - handle both << >> and inline formats
	// Pattern: /Font<<...>> or /Font <<...>>
	// First try simple pattern
	fontPattern := regexp.MustCompile(`/Font\s*<<([^>]*)>>`)
	fontMatch := fontPattern.FindStringSubmatch(resourcesStr)

	if fontMatch == nil {
		// Try finding /Font followed by << with balanced brackets
		fontIdx := strings.Index(resourcesStr, "/Font")
		if fontIdx != -1 {
			// Find the << after /Font
			dictStart := fontIdx + 5
			for dictStart < len(resourcesStr) && (resourcesStr[dictStart] == ' ' || resourcesStr[dictStart] == '\n' || resourcesStr[dictStart] == '\r') {
				dictStart++
			}
			if dictStart < len(resourcesStr) && resourcesStr[dictStart] == '<' && dictStart+1 < len(resourcesStr) && resourcesStr[dictStart+1] == '<' {
				// Find matching >>
				dictEnd := dictStart + 2
				depth := 1
				for dictEnd < len(resourcesStr) && depth > 0 {
					if dictEnd+1 < len(resourcesStr) && resourcesStr[dictEnd] == '>' && resourcesStr[dictEnd+1] == '>' {
						depth--
						if depth == 0 {
							fontDictStr := resourcesStr[dictStart+2 : dictEnd]
							fontMatch = []string{"", fontDictStr}
							break
						}
						dictEnd += 2
					} else if dictEnd+1 < len(resourcesStr) && resourcesStr[dictEnd] == '<' && resourcesStr[dictEnd+1] == '<' {
						depth++
						dictEnd += 2
					} else {
						dictEnd++
					}
				}
			}
		}
	}

	if fontMatch == nil {
		if verbose {
			fmt.Printf("No /Font dictionary found in Resources\n")
		}
		return fonts
	}

	fontDictStr := fontMatch[1]
	if verbose {
		fmt.Printf("Found Font dictionary: %s\n", fontDictStr)
	}

	// Parse font entries: /F1 5 0 R /F2 6 0 R ...
	fontEntryPattern := regexp.MustCompile(`/(\w+)\s+(\d+)\s+\d+\s+R`)
	fontEntries := fontEntryPattern.FindAllStringSubmatch(fontDictStr, -1)

	if verbose {
		fmt.Printf("Found %d font entries\n", len(fontEntries))
	}

	for _, entry := range fontEntries {
		fontName := entry[1]
		fontObjNum, _ := parseObjectRef(entry[2] + " 0 R")

		fontInfo := extractFontInfo(fontObjNum, pdf, verbose)
		if fontInfo != nil {
			fontInfo.ID = "/" + fontName
			fonts[fontName] = *fontInfo
		}
	}

	return fonts
}

// extractFontInfo extracts information from a font dictionary object
func extractFontInfo(fontObjNum int, pdf *parse.PDF, verbose bool) *types.FontInfo {
	fontObj, err := pdf.GetObject(fontObjNum)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to get font object %d: %v\n", fontObjNum, err)
		}
		return nil
	}

	fontStr := string(fontObj)
	fontInfo := &types.FontInfo{
		Embedded: false,
	}

	// Extract Subtype (Type1, TrueType, Type0, etc.)
	// Try both /Subtype and Subtype (with and without leading /)
	subtype := extractDictValue(fontStr, "/Subtype")
	if subtype == "" {
		subtype = extractDictValue(fontStr, "Subtype")
	}
	if subtype != "" {
		fontInfo.Subtype = subtype
		// Ensure it starts with /
		if !strings.HasPrefix(fontInfo.Subtype, "/") {
			fontInfo.Subtype = "/" + fontInfo.Subtype
		}
	} else {
		fontInfo.Subtype = "Unknown"
	}

	// Extract BaseFont
	baseFont := extractDictValue(fontStr, "/BaseFont")
	if baseFont == "" {
		baseFont = extractDictValue(fontStr, "BaseFont")
	}
	if baseFont != "" {
		fontInfo.Name = baseFont
		// Remove leading / if present for family extraction
		familyName := strings.TrimPrefix(baseFont, "/")
		// Try to extract family name from BaseFont (e.g., "Helvetica-Bold" -> "Helvetica")
		parts := strings.Split(familyName, "-")
		if len(parts) > 0 {
			fontInfo.Family = parts[0]
		}
	}

	// Extract Encoding
	encoding := extractDictValue(fontStr, "/Encoding")
	if encoding != "" {
		fontInfo.Encoding = encoding
	}

	// Check if font is embedded (has FontDescriptor with FontFile, FontFile2, or FontFile3)
	if strings.Contains(fontStr, "/FontFile") || strings.Contains(fontStr, "/FontFile2") || strings.Contains(fontStr, "/FontFile3") {
		fontInfo.Embedded = true
	}

	// Check for ToUnicode CMap
	if strings.Contains(fontStr, "/ToUnicode") {
		fontInfo.ToUnicode = true
	}

	// Extract FontDescriptor for more info
	fontDescriptorRef := extractDictValue(fontStr, "/FontDescriptor")
	if fontDescriptorRef != "" {
		descObjNum, err := parseObjectRef(fontDescriptorRef)
		if err == nil {
			descObj, err := pdf.GetObject(descObjNum)
			if err == nil {
				descStr := string(descObj)
				// Check for embedded font files in descriptor
				if strings.Contains(descStr, "/FontFile") || strings.Contains(descStr, "/FontFile2") || strings.Contains(descStr, "/FontFile3") {
					fontInfo.Embedded = true
				}
				// Extract font family name if available
				fontFamily := extractDictValue(descStr, "/FontFamily")
				if fontFamily != "" {
					fontInfo.Family = fontFamily
				}
			}
		}
	}

	return fontInfo
}

// extractXObjectsDict extracts XObject information from /Resources/XObject dictionary
func extractXObjectsDict(resourcesStr string, pdf *parse.PDF, verbose bool) map[string]types.XObject {
	xobjects := make(map[string]types.XObject)

	// Find /XObject dictionary
	xobjPattern := regexp.MustCompile(`/XObject\s*<<([^>]*)>>`)
	xobjMatch := xobjPattern.FindStringSubmatch(resourcesStr)
	if xobjMatch == nil {
		// Try without << >> (inline dictionary)
		xobjPattern2 := regexp.MustCompile(`/XObject\s*<<([^>]+)`)
		xobjMatch2 := xobjPattern2.FindStringSubmatch(resourcesStr)
		if xobjMatch2 == nil {
			return xobjects
		}
		xobjMatch = xobjMatch2
	}

	xobjDictStr := xobjMatch[1]

	// Parse XObject entries: /Im1 5 0 R /Im2 6 0 R ...
	xobjEntryPattern := regexp.MustCompile(`/(\w+)\s+(\d+)\s+\d+\s+R`)
	xobjEntries := xobjEntryPattern.FindAllStringSubmatch(xobjDictStr, -1)

	for _, entry := range xobjEntries {
		xobjName := entry[1]
		xobjObjNum, _ := parseObjectRef(entry[2] + " 0 R")

		xobj := extractXObjectInfo(xobjObjNum, pdf, verbose)
		if xobj != nil {
			xobj.ID = "/" + xobjName
			xobjects[xobjName] = *xobj
		}
	}

	return xobjects
}

// extractXObjectInfo extracts information from an XObject dictionary
func extractXObjectInfo(xobjObjNum int, pdf *parse.PDF, verbose bool) *types.XObject {
	xobjObj, err := pdf.GetObject(xobjObjNum)
	if err != nil {
		if verbose {
			fmt.Printf("Warning: failed to get XObject %d: %v\n", xobjObjNum, err)
		}
		return nil
	}

	xobjStr := string(xobjObj)
	xobject := &types.XObject{
		Type: "XObject",
	}

	// Extract Subtype
	subtype := extractDictValue(xobjStr, "/Subtype")
	if subtype == "" {
		return nil
	}
	xobject.Subtype = subtype

	// For Image XObjects, extract dimensions
	if subtype == "/Image" {
		// Extract Width and Height
		widthStr := extractDictValue(xobjStr, "/Width")
		heightStr := extractDictValue(xobjStr, "/Height")
		if widthStr != "" {
			if w, err := strconv.ParseFloat(widthStr, 64); err == nil {
				xobject.Width = w
			}
		}
		if heightStr != "" {
			if h, err := strconv.ParseFloat(heightStr, 64); err == nil {
				xobject.Height = h
			}
		}
	}

	return xobject
}
