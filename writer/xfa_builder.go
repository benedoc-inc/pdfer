package writer

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/parser"
	"github.com/benedoc-inc/pdfer/types"
)

// XFABuilder builds PDFs with XFA content
type XFABuilder struct {
	writer      *PDFWriter
	catalogNum  int
	acroFormNum int
	xfaStreams  map[string]int // stream name -> object number
	verbose     bool
}

// NewXFABuilder creates a new XFA PDF builder
func NewXFABuilder(verbose bool) *XFABuilder {
	return &XFABuilder{
		writer:     NewPDFWriter(),
		xfaStreams: make(map[string]int),
		verbose:    verbose,
	}
}

// XFAStreamData represents XFA stream data with its name
type XFAStreamData struct {
	Name     string
	Data     []byte
	Compress bool
}

// BuildFromXFA creates a complete PDF from XFA stream data
func (b *XFABuilder) BuildFromXFA(streams []XFAStreamData) ([]byte, error) {
	if b.verbose {
		log.Printf("Building PDF from %d XFA streams", len(streams))
	}
	
	// Create XFA stream objects
	var xfaArrayParts []string
	
	for _, stream := range streams {
		objNum := b.writer.AddStreamObject(
			Dictionary{"Type": "/EmbeddedFile"},
			stream.Data,
			stream.Compress,
		)
		b.xfaStreams[stream.Name] = objNum
		
		// Add to XFA array: (name) objNum 0 R
		xfaArrayParts = append(xfaArrayParts, fmt.Sprintf("(%s)%d 0 R", stream.Name, objNum))
		
		if b.verbose {
			log.Printf("Added XFA stream '%s': object %d, %d bytes", stream.Name, objNum, len(stream.Data))
		}
	}
	
	// Create AcroForm dictionary with XFA array
	acroFormContent := fmt.Sprintf("<</Fields[]/XFA[%s]>>", 
		joinStrings(xfaArrayParts, ""))
	b.acroFormNum = b.writer.AddObject([]byte(acroFormContent))
	
	// Create catalog (root) with AcroForm reference
	catalogContent := fmt.Sprintf("<</Type/Catalog/AcroForm %d 0 R>>", b.acroFormNum)
	b.catalogNum = b.writer.AddObject([]byte(catalogContent))
	
	// Set root
	b.writer.SetRoot(b.catalogNum)
	
	if b.verbose {
		log.Printf("Created catalog (object %d) with AcroForm (object %d)", b.catalogNum, b.acroFormNum)
	}
	
	return b.writer.Bytes()
}

// BuildFromOriginal rebuilds a PDF preserving structure but updating XFA streams
func (b *XFABuilder) BuildFromOriginal(originalPDF []byte, updatedStreams map[string][]byte, encryptInfo *types.PDFEncryption) ([]byte, error) {
	if b.verbose {
		log.Printf("Rebuilding PDF with %d updated streams", len(updatedStreams))
	}
	
	// Parse the original PDF structure
	objects, err := parseAllObjects(originalPDF, encryptInfo, b.verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse original PDF: %v", err)
	}
	
	if b.verbose {
		log.Printf("Parsed %d objects from original PDF", len(objects))
	}
	
	// Find trailer info
	rootObjNum, infoObjNum, encryptObjNum, fileID := parseTrailerInfo(originalPDF)
	
	if b.verbose {
		log.Printf("Trailer: Root=%d, Info=%d, Encrypt=%d", rootObjNum, infoObjNum, encryptObjNum)
	}
	
	// Find XFA stream references in AcroForm
	xfaStreamMap, err := findXFAStreamMap(originalPDF, encryptInfo, b.verbose)
	if err != nil {
		if b.verbose {
			log.Printf("Warning: could not find XFA stream map: %v", err)
		}
	}
	
	// Update stream objects with new data
	for streamName, newData := range updatedStreams {
		if objNum, ok := xfaStreamMap[streamName]; ok {
			if obj, objOk := objects[objNum]; objOk {
				// Compress the new data
				var compressed bytes.Buffer
				zw := zlib.NewWriter(&compressed)
				zw.Write(newData)
				zw.Close()
				
				// Update the object's stream
				obj.Stream = compressed.Bytes()
				if obj.Dict == nil {
					obj.Dict = make(Dictionary)
				}
				obj.Dict["Filter"] = "/FlateDecode"
				obj.Dict["Length"] = len(obj.Stream)
				
				if b.verbose {
					log.Printf("Updated stream '%s' (object %d): %d bytes compressed", streamName, objNum, len(obj.Stream))
				}
			}
		}
	}
	
	// Add all objects to writer
	for objNum, obj := range objects {
		if obj.Stream != nil {
			b.writer.SetStreamObject(objNum, obj.Dict, obj.Stream, false) // Already compressed
		} else if obj.Content != nil {
			b.writer.SetObject(objNum, obj.Content)
		}
	}
	
	// Set trailer references
	if rootObjNum > 0 {
		b.writer.SetRoot(rootObjNum)
	}
	if infoObjNum > 0 {
		b.writer.SetInfo(infoObjNum)
	}
	if encryptObjNum > 0 && encryptInfo != nil {
		b.writer.SetEncryptRef(encryptObjNum)
		b.writer.SetEncryption(encryptInfo, fileID)
	}
	
	return b.writer.Bytes()
}

// parseAllObjects parses all objects from a PDF including those in object streams
func parseAllObjects(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) (map[int]*PDFObject, error) {
	objects := make(map[int]*PDFObject)
	
	// First, parse direct objects (those with "N G obj" pattern)
	objPattern := regexp.MustCompile(`(\d+)\s+(\d+)\s+obj`)
	matches := objPattern.FindAllSubmatchIndex(pdfBytes, -1)
	
	for _, match := range matches {
		objNumStr := string(pdfBytes[match[2]:match[3]])
		genNumStr := string(pdfBytes[match[4]:match[5]])
		
		objNum, _ := strconv.Atoi(objNumStr)
		genNum, _ := strconv.Atoi(genNumStr)
		
		// Find endobj
		objStart := match[1] // After "obj"
		endObjPos := bytes.Index(pdfBytes[objStart:], []byte("endobj"))
		if endObjPos == -1 {
			continue
		}
		
		objContent := pdfBytes[objStart : objStart+endObjPos]
		
		// Check for stream
		streamPos := bytes.Index(objContent, []byte("stream"))
		var streamData []byte
		var dict Dictionary
		
		if streamPos != -1 {
			// Parse dictionary
			dictContent := objContent[:streamPos]
			dict = parseDictionary(dictContent)
			
			// Find stream data
			streamStart := streamPos + 6
			// Skip EOL
			for streamStart < len(objContent) && (objContent[streamStart] == '\r' || objContent[streamStart] == '\n') {
				streamStart++
			}
			
			// Find endstream
			endstreamPos := bytes.Index(objContent[streamStart:], []byte("endstream"))
			if endstreamPos == -1 {
				endstreamPos = len(objContent) - streamStart
			}
			
			// Use Length from dictionary if available
			if length, ok := dict["Length"].(int); ok && streamStart+length <= len(objContent) {
				streamData = objContent[streamStart : streamStart+length]
			} else {
				streamData = objContent[streamStart : streamStart+endstreamPos]
			}
		}
		
		// Trim whitespace from content
		content := bytes.TrimSpace(objContent)
		if streamPos != -1 {
			content = bytes.TrimSpace(objContent[:streamPos])
		}
		
		objects[objNum] = &PDFObject{
			Number:     objNum,
			Generation: genNum,
			Content:    content,
			Stream:     streamData,
			Dict:       dict,
		}
	}
	
	// Now extract objects from object streams using the parser
	startXRef := findStartXRef(pdfBytes)
	if startXRef > 0 {
		xrefResult, err := parser.ParseXRefStreamFull(pdfBytes, startXRef, false)
		if err == nil {
			// Process objects stored in object streams
			for objNum, entry := range xrefResult.ObjectStreams {
				if _, exists := objects[objNum]; exists {
					continue // Already have this object as a direct object
				}
				
				// Extract object from the object stream
				objData, err := parser.GetObjectFromStream(pdfBytes, objNum, entry.StreamObjNum, entry.IndexInStream, encryptInfo, false)
				if err != nil {
					if verbose {
						log.Printf("Warning: failed to extract object %d from stream %d: %v", objNum, entry.StreamObjNum, err)
					}
					continue
				}
				
				// Parse the extracted object content
				objects[objNum] = &PDFObject{
					Number:     objNum,
					Generation: 0,
					Content:    bytes.TrimSpace(objData),
				}
				
				if verbose {
					log.Printf("Extracted object %d from object stream %d", objNum, entry.StreamObjNum)
				}
			}
		} else if verbose {
			log.Printf("Warning: could not parse xref stream: %v", err)
		}
	}
	
	return objects, nil
}

// findStartXRef finds the startxref position in a PDF
func findStartXRef(pdfBytes []byte) int64 {
	// Search from end of file
	searchLen := types.Min(1024, len(pdfBytes))
	tailStart := len(pdfBytes) - searchLen
	tail := string(pdfBytes[tailStart:])
	
	pattern := regexp.MustCompile(`startxref\s+(\d+)`)
	match := pattern.FindStringSubmatch(tail)
	if match != nil {
		val, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil {
			return val
		}
	}
	return -1
}

// parseDictionary parses a PDF dictionary into our Dictionary type
func parseDictionary(data []byte) Dictionary {
	dict := make(Dictionary)
	
	// Simple parsing - extract key-value pairs
	s := string(data)
	
	// Remove << and >>
	s = strings.TrimPrefix(strings.TrimSpace(s), "<<")
	s = strings.TrimSuffix(strings.TrimSpace(s), ">>")
	
	// Find /Key Value pairs
	keyPattern := regexp.MustCompile(`/(\w+)\s*`)
	matches := keyPattern.FindAllStringSubmatchIndex(s, -1)
	
	for i, match := range matches {
		key := s[match[2]:match[3]]
		
		// Value is from end of key to start of next key (or end)
		valueStart := match[1]
		var valueEnd int
		if i+1 < len(matches) {
			valueEnd = matches[i+1][0]
		} else {
			valueEnd = len(s)
		}
		
		value := strings.TrimSpace(s[valueStart:valueEnd])
		
		// Parse value type
		if num, err := strconv.Atoi(value); err == nil {
			dict[key] = num
		} else {
			dict[key] = value
		}
	}
	
	return dict
}

// parseTrailerInfo extracts trailer references
func parseTrailerInfo(pdfBytes []byte) (rootNum, infoNum, encryptNum int, fileID []byte) {
	pdfStr := string(pdfBytes)
	
	// Find Root
	rootPattern := regexp.MustCompile(`/Root\s+(\d+)\s+\d+\s+R`)
	if match := rootPattern.FindStringSubmatch(pdfStr); match != nil {
		rootNum, _ = strconv.Atoi(match[1])
	}
	
	// Find Info
	infoPattern := regexp.MustCompile(`/Info\s+(\d+)\s+\d+\s+R`)
	if match := infoPattern.FindStringSubmatch(pdfStr); match != nil {
		infoNum, _ = strconv.Atoi(match[1])
	}
	
	// Find Encrypt
	encryptPattern := regexp.MustCompile(`/Encrypt\s+(\d+)\s+\d+\s+R`)
	if match := encryptPattern.FindStringSubmatch(pdfStr); match != nil {
		encryptNum, _ = strconv.Atoi(match[1])
	}
	
	// Find ID
	idPattern := regexp.MustCompile(`/ID\s*\[\s*<([0-9A-Fa-f]+)>`)
	if match := idPattern.FindStringSubmatch(pdfStr); match != nil {
		fileID, _ = hexDecode(match[1])
	}
	
	return
}

// findXFAStreamMap finds the mapping of XFA stream names to object numbers
func findXFAStreamMap(pdfBytes []byte, encryptInfo *types.PDFEncryption, verbose bool) (map[string]int, error) {
	result := make(map[string]int)
	
	// First find AcroForm reference
	pdfStr := string(pdfBytes)
	acroFormPattern := regexp.MustCompile(`/AcroForm\s+(\d+)\s+\d+\s+R`)
	acroFormMatch := acroFormPattern.FindStringSubmatch(pdfStr)
	
	if acroFormMatch == nil {
		// Try inline AcroForm
		return findXFAStreamMapInline(pdfBytes)
	}
	
	acroFormObjNum, err := strconv.Atoi(acroFormMatch[1])
	if err != nil {
		return result, fmt.Errorf("invalid AcroForm object number")
	}
	
	// Get the AcroForm object using proper parser (handles object streams)
	acroFormData, err := parser.GetObject(pdfBytes, acroFormObjNum, encryptInfo, false)
	if err != nil {
		return result, fmt.Errorf("failed to get AcroForm object: %v", err)
	}
	
	// Find XFA array in AcroForm content
	return parseXFAArrayFromContent(acroFormData, verbose)
}

// findXFAStreamMapInline finds XFA array when AcroForm is inline
func findXFAStreamMapInline(pdfBytes []byte) (map[string]int, error) {
	result := make(map[string]int)
	pdfStr := string(pdfBytes)
	
	xfaPattern := regexp.MustCompile(`/XFA\s*\[([^\]]+)\]`)
	match := xfaPattern.FindStringSubmatch(pdfStr)
	if match == nil {
		return result, fmt.Errorf("XFA array not found")
	}
	
	return parseXFAArrayContent(match[1])
}

// parseXFAArrayFromContent parses XFA array from object content
func parseXFAArrayFromContent(content []byte, verbose bool) (map[string]int, error) {
	result := make(map[string]int)
	
	// Find XFA array
	xfaIdx := bytes.Index(content, []byte("/XFA"))
	if xfaIdx == -1 {
		return result, fmt.Errorf("XFA not found in AcroForm")
	}
	
	// Find array start
	arrayStart := bytes.Index(content[xfaIdx:], []byte("["))
	if arrayStart == -1 {
		return result, fmt.Errorf("XFA array start not found")
	}
	arrayStart += xfaIdx
	
	// Find array end
	depth := 0
	arrayEnd := arrayStart
	for i := arrayStart; i < len(content); i++ {
		if content[i] == '[' {
			depth++
		} else if content[i] == ']' {
			depth--
			if depth == 0 {
				arrayEnd = i + 1
				break
			}
		}
	}
	
	arrayContent := string(content[arrayStart+1 : arrayEnd-1])
	if verbose {
		log.Printf("Found XFA array content: %s", arrayContent)
	}
	
	return parseXFAArrayContent(arrayContent)
}

// parseXFAArrayContent parses XFA array content string
func parseXFAArrayContent(arrayContent string) (map[string]int, error) {
	result := make(map[string]int)
	
	refPattern := regexp.MustCompile(`\(([^)]+)\)\s*(\d+)\s+\d+\s+R`)
	refs := refPattern.FindAllStringSubmatch(arrayContent, -1)
	
	for _, ref := range refs {
		name := ref[1]
		objNum, _ := strconv.Atoi(ref[2])
		result[name] = objNum
	}
	
	return result, nil
}

// hexDecode decodes a hex string to bytes
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		b, err := strconv.ParseUint(s[i:i+2], 16, 8)
		if err != nil {
			return nil, err
		}
		result[i/2] = byte(b)
	}
	return result, nil
}

// joinStrings joins strings without separator
func joinStrings(parts []string, sep string) string {
	var buf bytes.Buffer
	for i, p := range parts {
		if i > 0 {
			buf.WriteString(sep)
		}
		buf.WriteString(p)
	}
	return buf.String()
}

