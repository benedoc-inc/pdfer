package xfa

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
	"github.com/benedoc-inc/pdfer/writer"
)

// extractStreamDataFromObject extracts stream data from raw object bytes
// The object should already be decrypted
func extractStreamDataFromObject(objData []byte, objNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Find stream keyword
	streamIdx := bytes.Index(objData, []byte("stream"))
	if streamIdx == -1 {
		// Not a stream object - might be a simple value
		// Look for dictionary or value content
		dictStart := bytes.Index(objData, []byte("<<"))
		if dictStart != -1 {
			return objData[dictStart:], nil
		}
		return objData, nil
	}

	// Find endstream
	endstreamIdx := bytes.Index(objData[streamIdx:], []byte("endstream"))
	if endstreamIdx == -1 {
		return nil, fmt.Errorf("endstream not found in object %d", objNum)
	}
	endstreamIdx += streamIdx

	// Get /Length from dictionary if available
	dictEnd := streamIdx
	dictStr := string(objData[:dictEnd])
	lengthPattern := regexp.MustCompile(`/Length\s+(\d+)`)
	lengthMatch := lengthPattern.FindStringSubmatch(dictStr)

	// Skip "stream" and EOL
	dataStart := streamIdx + 6
	if dataStart < len(objData) && objData[dataStart] == '\r' {
		dataStart++
	}
	if dataStart < len(objData) && objData[dataStart] == '\n' {
		dataStart++
	}

	var streamData []byte
	if lengthMatch != nil {
		length, _ := strconv.Atoi(lengthMatch[1])
		if dataStart+length <= len(objData) {
			streamData = objData[dataStart : dataStart+length]
		} else {
			streamData = objData[dataStart:endstreamIdx]
		}
	} else {
		streamData = objData[dataStart:endstreamIdx]
	}

	// Check if FlateDecode filter is present
	if bytes.Contains(objData[:streamIdx], []byte("/FlateDecode")) ||
		bytes.Contains(objData[:streamIdx], []byte("/Filter/FlateDecode")) ||
		bytes.Contains(objData[:streamIdx], []byte("/Filter /FlateDecode")) {
		// Decompress
		zlibReader, err := zlib.NewReader(bytes.NewReader(streamData))
		if err == nil {
			decompressed, err := io.ReadAll(zlibReader)
			zlibReader.Close()
			if err == nil {
				return decompressed, nil
			}
		}
		// Try raw deflate
		flateReader := flate.NewReader(bytes.NewReader(streamData))
		decompressed, err := io.ReadAll(flateReader)
		flateReader.Close()
		if err == nil {
			return decompressed, nil
		}
		// Return raw data if decompression fails
		if verbose {
			log.Printf("Decompression failed for object %d, returning raw stream data", objNum)
		}
	}

	return streamData, nil
}

// XFAStreamInfo contains stream data and metadata
type XFAStreamInfo struct {
	Data         []byte `json:"data"`
	ObjectNumber int    `json:"objectNumber"`
	Compressed   bool   `json:"compressed"`
}

// XFAStreams represents all XFA streams extracted from a PDF
type XFAStreams struct {
	Template      *XFAStreamInfo `json:"template,omitempty"`
	Datasets      *XFAStreamInfo `json:"datasets,omitempty"`
	Config        *XFAStreamInfo `json:"config,omitempty"`
	LocaleSet     *XFAStreamInfo `json:"localeSet,omitempty"`
	ConnectionSet *XFAStreamInfo `json:"connectionSet,omitempty"`
	Stylesheet    *XFAStreamInfo `json:"stylesheet,omitempty"`
	XMP           *XFAStreamInfo `json:"xmp,omitempty"`
	Signature     *XFAStreamInfo `json:"signature,omitempty"`
	SourceSet     *XFAStreamInfo `json:"sourceSet,omitempty"`
}

// ExtractAllXFAStreams extracts all XFA streams from a PDF without using UniPDF
func ExtractAllXFAStreams(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) (*XFAStreams, error) {
	if verbose {
		log.Printf("Extracting all XFA streams from PDF (no UniPDF)")
	}

	streams := &XFAStreams{}
	pdfStr := string(pdfBytes)

	// Find AcroForm reference
	acroFormPattern := regexp.MustCompile(`/AcroForm\s+(\d+)\s+(\d+)\s+R`)
	acroFormMatch := acroFormPattern.FindStringSubmatchIndex(pdfStr)

	var acroFormObjNum int
	var err error

	if acroFormMatch != nil {
		// AcroForm is an indirect reference
		acroFormObjNum, err = strconv.Atoi(pdfStr[acroFormMatch[2]:acroFormMatch[3]])
		if err != nil {
			return nil, fmt.Errorf("invalid AcroForm object number: %v", err)
		}

		if verbose {
			log.Printf("Found AcroForm indirect reference: object %d", acroFormObjNum)
		}
	} else {
		// Try inline AcroForm dictionary
		acroFormInlinePattern := regexp.MustCompile(`/AcroForm\s*<<`)
		acroFormInlineMatch := acroFormInlinePattern.FindStringIndex(pdfStr)
		if acroFormInlineMatch == nil {
			return nil, fmt.Errorf("AcroForm not found (neither inline nor indirect reference)")
		}
		// For inline, we'll search from that position
		acroFormObjNum = 0
	}

	// Find and decrypt AcroForm object to get XFA array using PyPDF approach
	var xfaArrayContent string
	if acroFormObjNum > 0 {
		// Use new GetObject function that handles both direct objects and object streams
		// This is the equivalent of PyPDF's get_object() method
		decryptedContent, err := parser.GetObject(pdfBytes, acroFormObjNum, encryptInfo, verbose)
		if err != nil {
			if verbose {
				log.Printf("GetObject failed for AcroForm %d: %v, trying fallback", acroFormObjNum, err)
			}
			// Fallback to old method
			decryptedContent, err = findAndDecryptAcroForm(pdfBytes, acroFormObjNum, encryptInfo, verbose)
			if err != nil {
				return nil, err
			}
		}

		xfaArrayContent, err = findXFAArrayContent(decryptedContent, verbose)
		if err != nil {
			return nil, err
		}
	} else {
		// Inline AcroForm - find XFA directly
		xfaPattern := regexp.MustCompile(`/XFA\s*\[`)
		xfaMatch := xfaPattern.FindStringIndex(pdfStr)
		if xfaMatch == nil {
			return nil, fmt.Errorf("XFA entry not found")
		}

		arrayStart := xfaMatch[1] - 1
		depth := 0
		arrayEnd := arrayStart
		for i := arrayStart; i < len(pdfStr) && i < arrayStart+10000; i++ {
			if pdfStr[i] == '[' {
				depth++
			} else if pdfStr[i] == ']' {
				depth--
				if depth == 0 {
					arrayEnd = i
					break
				}
			}
		}

		if arrayEnd == arrayStart {
			return nil, fmt.Errorf("could not find end of XFA array")
		}

		xfaArrayContent = pdfStr[arrayStart+1 : arrayEnd]
	}

	if verbose {
		log.Printf("Found XFA array content: %s", xfaArrayContent[:min(200, len(xfaArrayContent))])
	}

	// Parse XFA array: format is (name) N M R (name) N M R ...
	// Pattern: (name) followed by object number, generation, and R
	// Note: whitespace between ) and object number is optional
	streamPattern := regexp.MustCompile(`\(([^)]+)\)\s*(\d+)\s+(\d+)\s+R`)
	matches := streamPattern.FindAllStringSubmatch(xfaArrayContent, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no stream references found in XFA array")
	}

	if verbose {
		log.Printf("Found %d stream references in XFA array", len(matches))
	}

	// Extract each stream
	for _, match := range matches {
		streamName := match[1]
		objNum, err := strconv.Atoi(match[2])
		if err != nil {
			if verbose {
				log.Printf("Invalid stream object number for %s: %s", streamName, match[2])
			}
			continue
		}

		// Use GetObject which properly handles both direct objects and objects in streams
		objData, err := parser.GetObject(pdfBytes, objNum, encryptInfo, verbose)
		if err != nil {
			if verbose {
				log.Printf("Failed to get object %d for stream %s: %v, trying fallback", objNum, streamName, err)
			}
			// Fallback to old method
			objData, _, err = extractStreamFromPDF(pdfBytes, objNum, encryptInfo, verbose)
			if err != nil {
				if verbose {
					log.Printf("Fallback also failed for %s (object %d): %v", streamName, objNum, err)
				}
				continue
			}
		}

		// Extract stream data from the object
		streamData, err := extractStreamDataFromObject(objData, objNum, encryptInfo, verbose)
		if err != nil {
			if verbose {
				log.Printf("Failed to extract stream data from object %d: %v", objNum, err)
			}
			continue
		}

		// Decompress if needed
		decompressed, wasCompressed, err := DecompressStream(streamData)
		if err != nil {
			if verbose {
				log.Printf("Failed to decompress stream %s: %v", streamName, err)
			}
			decompressed = streamData // Use as-is if decompression fails
		}

		streamInfo := &XFAStreamInfo{
			Data:         decompressed,
			ObjectNumber: objNum,
			Compressed:   wasCompressed,
		}

		if verbose {
			log.Printf("Extracted %s stream: %d bytes (object %d, compressed: %v)", streamName, len(decompressed), objNum, wasCompressed)
		}

		// Store in appropriate field
		switch streamName {
		case "template":
			streams.Template = streamInfo
		case "datasets":
			streams.Datasets = streamInfo
		case "config":
			streams.Config = streamInfo
		case "localeSet":
			streams.LocaleSet = streamInfo
		case "connectionSet":
			streams.ConnectionSet = streamInfo
		case "stylesheet":
			streams.Stylesheet = streamInfo
		case "xmp":
			streams.XMP = streamInfo
		case "signature":
			streams.Signature = streamInfo
		case "sourceSet":
			streams.SourceSet = streamInfo
		default:
			if verbose {
				log.Printf("Unknown XFA stream type: %s", streamName)
			}
		}
	}

	return streams, nil
}

// FindXFADatasetsStream finds and extracts the XFA datasets stream
// This is a convenience function that uses ExtractAllXFAStreams and returns only the datasets stream
func FindXFADatasetsStream(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, int, error) {
	// Extract all XFA streams
	streams, err := ExtractAllXFAStreams(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to extract XFA streams: %v", err)
	}

	// Return datasets stream if found
	if streams.Datasets != nil {
		return streams.Datasets.Data, streams.Datasets.ObjectNumber, nil
	}

	return nil, 0, fmt.Errorf("datasets stream not found in XFA")
}

// RebuildPDFFromXFAStreams rebuilds a PDF from extracted XFA streams using proper PDF writer
func RebuildPDFFromXFAStreams(originalPDFBytes []byte, streams *XFAStreams, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	if verbose {
		log.Printf("Rebuilding PDF from XFA streams using PDFWriter")
	}

	// Map stream names to stream data
	updatedStreams := make(map[string][]byte)
	if streams.Template != nil && len(streams.Template.Data) > 0 {
		updatedStreams["template"] = streams.Template.Data
	}
	if streams.Datasets != nil && len(streams.Datasets.Data) > 0 {
		updatedStreams["datasets"] = streams.Datasets.Data
	}
	if streams.Config != nil && len(streams.Config.Data) > 0 {
		updatedStreams["config"] = streams.Config.Data
	}
	if streams.LocaleSet != nil && len(streams.LocaleSet.Data) > 0 {
		updatedStreams["localeSet"] = streams.LocaleSet.Data
	}
	if streams.ConnectionSet != nil && len(streams.ConnectionSet.Data) > 0 {
		updatedStreams["connectionSet"] = streams.ConnectionSet.Data
	}
	if streams.Stylesheet != nil && len(streams.Stylesheet.Data) > 0 {
		updatedStreams["stylesheet"] = streams.Stylesheet.Data
	}
	if streams.XMP != nil && len(streams.XMP.Data) > 0 {
		updatedStreams["xmp"] = streams.XMP.Data
	}
	if streams.Signature != nil && len(streams.Signature.Data) > 0 {
		updatedStreams["signature"] = streams.Signature.Data
	}
	if streams.SourceSet != nil && len(streams.SourceSet.Data) > 0 {
		updatedStreams["sourceSet"] = streams.SourceSet.Data
	}

	// Use XFABuilder to rebuild
	builder := writer.NewXFABuilder(verbose)
	return builder.BuildFromOriginal(originalPDFBytes, updatedStreams, encryptInfo)
}

// BuildPDFFromXFAStreams creates a new PDF entirely from XFA stream data
// This is useful when you need a clean PDF with only the XFA content
func BuildPDFFromXFAStreams(streams *XFAStreams, verbose bool) ([]byte, error) {
	if verbose {
		log.Printf("Building new PDF from XFA streams")
	}

	// Collect all non-nil streams
	var xfaStreams []writer.XFAStreamData

	if streams.Template != nil && len(streams.Template.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "template",
			Data:     streams.Template.Data,
			Compress: true,
		})
	}
	if streams.Datasets != nil && len(streams.Datasets.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "datasets",
			Data:     streams.Datasets.Data,
			Compress: true,
		})
	}
	if streams.Config != nil && len(streams.Config.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "config",
			Data:     streams.Config.Data,
			Compress: true,
		})
	}
	if streams.LocaleSet != nil && len(streams.LocaleSet.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "localeSet",
			Data:     streams.LocaleSet.Data,
			Compress: true,
		})
	}
	if streams.ConnectionSet != nil && len(streams.ConnectionSet.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "connectionSet",
			Data:     streams.ConnectionSet.Data,
			Compress: true,
		})
	}
	if streams.Stylesheet != nil && len(streams.Stylesheet.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "stylesheet",
			Data:     streams.Stylesheet.Data,
			Compress: true,
		})
	}
	if streams.XMP != nil && len(streams.XMP.Data) > 0 {
		xfaStreams = append(xfaStreams, writer.XFAStreamData{
			Name:     "xmp",
			Data:     streams.XMP.Data,
			Compress: true,
		})
	}

	builder := writer.NewXFABuilder(verbose)
	return builder.BuildFromXFA(xfaStreams)
}

// UpdateXFAInPDF updates XFA field values in PDF bytes
func UpdateXFAInPDF(pdfBytes []byte, formData types.FormData, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Find XFA datasets stream
	datasetsStream, streamObjNum, err := FindXFADatasetsStream(pdfBytes, encryptInfo, verbose)
	if err != nil {
		return nil, fmt.Errorf("error finding XFA datasets stream: %v", err)
	}

	if verbose {
		log.Printf("Found XFA datasets stream at object %d", streamObjNum)
		log.Printf("Stream size: %d bytes", len(datasetsStream))
	}

	// Decompress if needed and parse XML
	xfaXML, wasCompressed, err := DecompressStream(datasetsStream)
	if err != nil {
		return nil, fmt.Errorf("error decompressing stream: %v", err)
	}

	if verbose {
		log.Printf("Decompressed XFA XML: %d bytes (was compressed: %v)", len(xfaXML), wasCompressed)
	}

	// Update field values in XFA XML
	updatedXML, err := UpdateXFAValues(string(xfaXML), formData, verbose)
	if err != nil {
		return nil, fmt.Errorf("error updating XFA values: %v", err)
	}

	// Re-compress if it was compressed
	updatedStream := []byte(updatedXML)
	if wasCompressed {
		compressed, err := CompressStream(updatedStream)
		if err != nil {
			return nil, fmt.Errorf("error compressing stream: %v", err)
		}
		updatedStream = compressed
		if verbose {
			log.Printf("Re-compressed stream: %d bytes", len(updatedStream))
		}
	}

	// Update PDF with new stream
	updatedPDF, err := ReplaceStreamInPDF(pdfBytes, streamObjNum, updatedStream, verbose)
	if err != nil {
		return nil, fmt.Errorf("error replacing stream: %v", err)
	}

	return updatedPDF, nil
}

// UpdateXFAValues updates field values in XFA XML
func UpdateXFAValues(xfaXML string, formData types.FormData, verbose bool) (string, error) {
	// Find data section
	dataStart := strings.Index(xfaXML, "<data")
	if dataStart == -1 {
		// If no data section, update individual fields
		return UpdateXFAFieldValues(xfaXML, formData, verbose)
	}

	// Find end of data section
	dataEnd := strings.LastIndex(xfaXML, "</data>")
	if dataEnd == -1 {
		return UpdateXFAFieldValues(xfaXML, formData, verbose)
	}

	// Extract data section
	dataSection := xfaXML[dataStart : dataEnd+7]

	// Update field values in data section
	updatedData, err := UpdateXFAFieldValues(dataSection, formData, verbose)
	if err != nil {
		return "", err
	}

	// Replace data section in full XML
	result := xfaXML[:dataStart] + updatedData + xfaXML[dataEnd+7:]

	return result, nil
}

// UpdateXFAFieldValues updates individual field values in XFA XML
func UpdateXFAFieldValues(xfaXML string, formData types.FormData, verbose bool) (string, error) {
	resultXML := xfaXML

	for fieldName, newValue := range formData {
		// Find field in XFA XML
		fieldPattern := fmt.Sprintf(`<field name="%s"`, fieldName)
		fieldIndex := strings.Index(resultXML, fieldPattern)

		if fieldIndex == -1 {
			if verbose {
				log.Printf("Warning: Field '%s' not found in XFA XML", fieldName)
			}
			continue
		}

		// Find the value element for this field
		fieldEnd := strings.Index(resultXML[fieldIndex:], "</field>")
		if fieldEnd == -1 {
			continue
		}

		fieldSection := resultXML[fieldIndex : fieldIndex+fieldEnd]

		// Check if value element exists
		valueStart := strings.Index(fieldSection, "<value>")
		valueEnd := strings.Index(fieldSection, "</value>")

		valueStr := fmt.Sprintf("%v", newValue)

		if valueStart != -1 && valueEnd != -1 {
			// Replace existing value
			beforeValue := resultXML[:fieldIndex+valueStart+7]
			afterValue := resultXML[fieldIndex+valueEnd:]
			resultXML = beforeValue + valueStr + afterValue
		} else {
			// Insert new value element before </field>
			beforeFieldEnd := resultXML[:fieldIndex+fieldEnd]
			afterFieldEnd := resultXML[fieldIndex+fieldEnd:]
			resultXML = beforeFieldEnd + "<value>" + valueStr + "</value>" + afterFieldEnd
		}

		if verbose {
			log.Printf("Updated field '%s' = '%s'", fieldName, valueStr)
		}
	}

	return resultXML, nil
}
