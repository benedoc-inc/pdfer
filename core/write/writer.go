// Package writer provides PDF writing capabilities
package write

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// PDFObject represents a PDF object with its content
type PDFObject struct {
	Number     int
	Generation int
	Content    []byte     // Raw content (dictionary, stream, etc.)
	Stream     []byte     // Stream data (if this is a stream object)
	Dict       Dictionary // Parsed dictionary (for convenience)
	IsFree     bool
}

// Dictionary represents a PDF dictionary
type Dictionary map[string]interface{}

// PDFWriter builds PDF files from scratch
type PDFWriter struct {
	objects         map[int]*PDFObject
	nextObjNum      int
	rootRef         string
	infoRef         string
	encryptRef      string
	outlinesRef     string // Outlines/bookmarks reference
	encryptInfo     *types.PDFEncryption
	fileID          []byte
	pdfVersion      string
	useXRefStream   bool // If true, use cross-reference stream instead of table
	useObjectStream bool // If true, compress objects into object streams
}

// NewPDFWriter creates a new PDF writer
func NewPDFWriter() *PDFWriter {
	return &PDFWriter{
		objects:    make(map[int]*PDFObject),
		nextObjNum: 1,
		pdfVersion: "1.7",
	}
}

// SetVersion sets the PDF version (e.g., "1.7")
func (w *PDFWriter) SetVersion(version string) {
	w.pdfVersion = version
}

// SetEncryption enables encryption for the output PDF
func (w *PDFWriter) SetEncryption(encryptInfo *types.PDFEncryption, fileID []byte) {
	w.encryptInfo = encryptInfo
	w.fileID = fileID
}

// UseXRefStream enables cross-reference stream writing (PDF 1.5+)
// If true, writes a compressed cross-reference stream instead of traditional xref table
func (w *PDFWriter) UseXRefStream(enable bool) {
	w.useXRefStream = enable
}

// UseObjectStream enables object stream writing (PDF 1.5+)
// If true, compresses objects into object streams (ObjStm) for smaller file sizes
// Note: Object streams require xref streams, so this automatically enables xref streams
func (w *PDFWriter) UseObjectStream(enable bool) {
	w.useObjectStream = enable
	if enable {
		// Object streams require xref streams (Type 2 entries)
		w.useXRefStream = true
	}
}

// AddObject adds a new object and returns its object number
func (w *PDFWriter) AddObject(content []byte) int {
	objNum := w.nextObjNum
	w.nextObjNum++

	w.objects[objNum] = &PDFObject{
		Number:     objNum,
		Generation: 0,
		Content:    content,
	}

	return objNum
}

// AddStreamObject adds a stream object with dictionary and data
func (w *PDFWriter) AddStreamObject(dict Dictionary, data []byte, compress bool) int {
	objNum := w.nextObjNum
	w.nextObjNum++

	streamData := data
	if compress && len(data) > 0 {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		zw.Write(data)
		zw.Close()
		streamData = buf.Bytes()
		dict["Filter"] = "/FlateDecode"
	}

	dict["Length"] = len(streamData)

	w.objects[objNum] = &PDFObject{
		Number:     objNum,
		Generation: 0,
		Dict:       dict,
		Stream:     streamData,
	}

	return objNum
}

// SetObject sets or replaces an object at a specific number
func (w *PDFWriter) SetObject(objNum int, content []byte) {
	w.objects[objNum] = &PDFObject{
		Number:     objNum,
		Generation: 0,
		Content:    content,
	}
	if objNum >= w.nextObjNum {
		w.nextObjNum = objNum + 1
	}
}

// SetStreamObject sets a stream object at a specific number
func (w *PDFWriter) SetStreamObject(objNum int, dict Dictionary, data []byte, compress bool) {
	streamData := data
	if compress && len(data) > 0 {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		zw.Write(data)
		zw.Close()
		streamData = buf.Bytes()
		dict["Filter"] = "/FlateDecode"
	}

	dict["Length"] = len(streamData)

	w.objects[objNum] = &PDFObject{
		Number:     objNum,
		Generation: 0,
		Dict:       dict,
		Stream:     streamData,
	}
	if objNum >= w.nextObjNum {
		w.nextObjNum = objNum + 1
	}
}

// SetRoot sets the root (catalog) object reference
func (w *PDFWriter) SetRoot(objNum int) {
	w.rootRef = fmt.Sprintf("%d 0 R", objNum)
}

// SetInfo sets the info dictionary object reference
func (w *PDFWriter) SetInfo(objNum int) {
	w.infoRef = fmt.Sprintf("%d 0 R", objNum)
}

// updateCatalogWithOutlines updates the catalog object to include /Outlines reference
func (w *PDFWriter) updateCatalogWithOutlines() {
	if w.outlinesRef == "" || w.rootRef == "" {
		return
	}

	// Parse root object number from rootRef (format: "N 0 R")
	rootObjNum := 0
	fmt.Sscanf(w.rootRef, "%d 0 R", &rootObjNum)
	if rootObjNum == 0 {
		return
	}

	// Get catalog object
	catalogObj, ok := w.objects[rootObjNum]
	if !ok || catalogObj == nil {
		return
	}

	// Parse catalog dictionary if it's a Dictionary, otherwise parse as string
	var catalogDict Dictionary
	if catalogObj.Dict != nil {
		catalogDict = catalogObj.Dict
	} else {
		// Try to parse from Content
		catalogStr := string(catalogObj.Content)
		// Simple parsing - find dictionary entries
		// For now, rebuild the dictionary properly
		// Extract existing entries and rebuild
		if strings.Contains(catalogStr, "/Type") && strings.Contains(catalogStr, "/Pages") {
			// Rebuild with /Outlines
			catalogDict = Dictionary{
				"/Type":  "/Catalog",
				"/Pages": extractDictValueFromString(catalogStr, "/Pages"),
			}
			if pagesRef := extractDictValueFromString(catalogStr, "/Pages"); pagesRef != "" {
				catalogDict["/Pages"] = pagesRef
			}
		} else {
			// Fallback: just add /Outlines to string
			catalogStr := string(catalogObj.Content)
			if !strings.Contains(catalogStr, "/Outlines") {
				lastIdx := strings.LastIndex(catalogStr, ">>")
				if lastIdx > 0 {
					catalogStr = catalogStr[:lastIdx] + fmt.Sprintf("/Outlines %s ", w.outlinesRef) + catalogStr[lastIdx:]
					catalogObj.Content = []byte(catalogStr)
				}
			}
			return
		}
	}

	// Add /Outlines to dictionary
	catalogDict["/Outlines"] = w.outlinesRef

	// Update catalog object
	catalogObj.Content = w.formatDictionary(catalogDict)
	catalogObj.Dict = catalogDict
}

// extractDictValueFromString extracts a dictionary value from a string representation
func extractDictValueFromString(dictStr, key string) string {
	// Simple regex-like extraction
	keyPattern := key + " "
	idx := strings.Index(dictStr, keyPattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(keyPattern)
	// Find the value (could be a reference, array, etc.)
	// For simplicity, extract until space or >
	end := start
	for end < len(dictStr) && dictStr[end] != ' ' && dictStr[end] != '>' && dictStr[end] != '\n' {
		end++
	}
	return dictStr[start:end]
}

// SetEncryptRef sets the encrypt dictionary object reference
func (w *PDFWriter) SetEncryptRef(objNum int) {
	w.encryptRef = fmt.Sprintf("%d 0 R", objNum)
}

// Write outputs the complete PDF to the given writer
func (w *PDFWriter) Write(out io.Writer) error {
	// Update catalog with outlines before writing
	if w.outlinesRef != "" {
		w.updateCatalogWithOutlines()
	}

	var buf bytes.Buffer

	// Write header
	buf.WriteString(fmt.Sprintf("%%PDF-%s\n", w.pdfVersion))
	buf.Write([]byte{0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A}) // Binary marker

	// Create object streams if enabled (before writing objects)
	// This may add new object stream objects to w.objects
	objStreamMap, _, err := w.createObjectStreams()
	if err != nil {
		return fmt.Errorf("failed to create object streams: %v", err)
	}

	// Collect and sort object numbers (after object streams are created)
	var objNums []int
	for num := range w.objects {
		objNums = append(objNums, num)
	}
	sort.Ints(objNums)

	// Write objects and track positions
	positions := make(map[int]int64)

	for _, objNum := range objNums {
		obj := w.objects[objNum]
		if obj.IsFree {
			continue
		}

		// Skip objects that are in object streams (they'll be referenced via Type 2)
		if objStreamMap != nil {
			if _, inStream := objStreamMap[objNum]; inStream {
				continue
			}
		}

		positions[objNum] = int64(buf.Len())

		// Write object header
		buf.WriteString(fmt.Sprintf("%d %d obj\n", objNum, obj.Generation))

		// Write content
		if obj.Stream != nil {
			// Stream object
			content := w.formatDictionary(obj.Dict)

			// Encrypt stream if needed
			streamData := obj.Stream
			if w.encryptInfo != nil {
				encrypted, err := w.encryptStream(streamData, objNum, obj.Generation)
				if err == nil {
					streamData = encrypted
					// Update length in dictionary
					obj.Dict["Length"] = len(streamData)
					content = w.formatDictionary(obj.Dict)
				}
			}

			buf.Write(content)
			buf.WriteString("\nstream\n")
			buf.Write(streamData)
			buf.WriteString("\nendstream")
		} else if obj.Content != nil {
			// Encrypt content if needed (for non-stream objects like strings in dicts)
			content := obj.Content
			// Note: For non-stream objects, encryption is more complex
			// (need to encrypt individual strings, not the whole object)
			// For simplicity, we skip encryption of non-stream content here
			buf.Write(content)
		}

		buf.WriteString("\nendobj\n")
	}

	var xrefPos int64

	// Write cross-reference (stream or table)
	if w.useXRefStream {
		// Write cross-reference stream
		xrefPos, err = w.writeXRefStream(&buf, positions, objStreamMap)
		if err != nil {
			return fmt.Errorf("failed to write xref stream: %v", err)
		}

		// Write trailer (xref streams still need a trailer with /Root, /Info, etc.)
		buf.WriteString("trailer\n<<\n")
		buf.WriteString(fmt.Sprintf("/Size %d\n", w.nextObjNum))
		if w.rootRef != "" {
			buf.WriteString(fmt.Sprintf("/Root %s\n", w.rootRef))
		}
		if w.infoRef != "" {
			buf.WriteString(fmt.Sprintf("/Info %s\n", w.infoRef))
		}
		if w.outlinesRef != "" {
			// Outlines reference goes in the catalog, not trailer
			// We'll update the catalog object if it exists
			w.updateCatalogWithOutlines()
		}
		if w.encryptRef != "" {
			buf.WriteString(fmt.Sprintf("/Encrypt %s\n", w.encryptRef))
		}
		if len(w.fileID) > 0 {
			hexID := fmt.Sprintf("%X", w.fileID)
			buf.WriteString(fmt.Sprintf("/ID [<%s><%s>]\n", hexID, hexID))
		}
		buf.WriteString(">>\n")
	} else {
		// Write traditional xref table
		xrefPos = int64(buf.Len())
		buf.WriteString("xref\n")
		buf.WriteString(fmt.Sprintf("0 %d\n", w.nextObjNum))

		// Entry for object 0 (always free, points to next free object)
		buf.WriteString(fmt.Sprintf("%010d %05d f \n", 0, 65535))

		// Entries for each object
		for i := 1; i < w.nextObjNum; i++ {
			if pos, ok := positions[i]; ok {
				buf.WriteString(fmt.Sprintf("%010d %05d n \n", pos, 0))
			} else {
				// Free object
				buf.WriteString(fmt.Sprintf("%010d %05d f \n", 0, 1))
			}
		}

		// Write trailer
		buf.WriteString("trailer\n<<\n")
		buf.WriteString(fmt.Sprintf("/Size %d\n", w.nextObjNum))
		if w.rootRef != "" {
			buf.WriteString(fmt.Sprintf("/Root %s\n", w.rootRef))
		}
		if w.infoRef != "" {
			buf.WriteString(fmt.Sprintf("/Info %s\n", w.infoRef))
		}
		if w.outlinesRef != "" {
			// Outlines reference goes in the catalog, not trailer
			// We'll update the catalog object if it exists
			w.updateCatalogWithOutlines()
		}
		if w.encryptRef != "" {
			buf.WriteString(fmt.Sprintf("/Encrypt %s\n", w.encryptRef))
		}
		if len(w.fileID) > 0 {
			hexID := fmt.Sprintf("%X", w.fileID)
			buf.WriteString(fmt.Sprintf("/ID [<%s><%s>]\n", hexID, hexID))
		}
		buf.WriteString(">>\n")
	}

	// Write startxref
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefPos))

	// Write to output
	_, err = out.Write(buf.Bytes())
	return err
}

// formatDictionary formats a Dictionary as PDF syntax
func (w *PDFWriter) formatDictionary(dict Dictionary) []byte {
	var buf bytes.Buffer
	buf.WriteString("<<")

	// Sort keys for consistent output
	var keys []string
	for k := range dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := dict[key]
		// Ensure key starts with /
		if !strings.HasPrefix(key, "/") {
			key = "/" + key
		}
		buf.WriteString(key)
		buf.WriteString(" ")
		buf.WriteString(w.formatValue(value))
		buf.WriteString(" ")
	}

	buf.WriteString(">>")
	return buf.Bytes()
}

// formatValue formats a value for PDF output
func (w *PDFWriter) formatValue(value interface{}) string {
	switch v := value.(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		// If it looks like a name or reference, use as-is
		if strings.HasPrefix(v, "/") || strings.HasSuffix(v, " R") {
			return v
		}
		// If it's already escaped (contains \( or \)), use as-is
		// Otherwise, it's a string - format as PDF string
		// Note: escapePDFString in metadata.go returns escaped content without parentheses
		return "(" + v + ")"
	case []byte:
		return "<" + fmt.Sprintf("%X", v) + ">"
	case []interface{}:
		var items []string
		for _, item := range v {
			items = append(items, w.formatValue(item))
		}
		return "[" + strings.Join(items, " ") + "]"
	case Dictionary:
		return string(w.formatDictionary(v))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// encryptStream encrypts stream data using the PDF's encryption settings
func (w *PDFWriter) encryptStream(data []byte, objNum, genNum int) ([]byte, error) {
	if w.encryptInfo == nil || len(w.encryptInfo.EncryptKey) == 0 {
		return data, nil
	}

	// Derive object-specific key (same as decrypt)
	pack1 := []byte{byte(objNum & 0xff), byte((objNum >> 8) & 0xff), byte((objNum >> 16) & 0xff)}
	pack2 := []byte{byte(genNum & 0xff), byte((genNum >> 8) & 0xff)}

	n := w.encryptInfo.KeyLength
	if n == 0 {
		n = 5
	}

	keyData := make([]byte, n+5)
	copy(keyData, w.encryptInfo.EncryptKey[:n])
	copy(keyData[n:], pack1)
	copy(keyData[n+3:], pack2)

	keyHash := md5.New()
	keyHash.Write(keyData)

	// AES encryption
	if w.encryptInfo.V == 4 || w.encryptInfo.V == 5 {
		keyHash.Write([]byte{0x73, 0x41, 0x6C, 0x54}) // "sAlT"
		aesKeyHash := keyHash.Sum(nil)
		aesKeyLen := min(n+5, 16)
		aesKey := aesKeyHash[:aesKeyLen]

		// Generate random IV
		iv := make([]byte, 16)
		if _, err := rand.Read(iv); err != nil {
			return nil, err
		}

		// Pad data to multiple of 16 (PKCS#7)
		padLen := 16 - (len(data) % 16)
		padded := make([]byte, len(data)+padLen)
		copy(padded, data)
		for i := len(data); i < len(padded); i++ {
			padded[i] = byte(padLen)
		}

		// Encrypt
		block, err := aes.NewCipher(aesKey)
		if err != nil {
			return nil, err
		}

		encrypted := make([]byte, len(padded))
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(encrypted, padded)

		// Prepend IV
		result := make([]byte, 16+len(encrypted))
		copy(result[:16], iv)
		copy(result[16:], encrypted)

		return result, nil
	}

	// RC4 encryption (V1, V2)
	// Not implemented for now
	return data, nil
}

// Bytes returns the complete PDF as a byte slice
func (w *PDFWriter) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := w.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// NextObjectNumber returns the next available object number
func (w *PDFWriter) NextObjectNumber() int {
	return w.nextObjNum
}

// GetObject returns an object's content (for reading existing objects)
func (w *PDFWriter) GetObject(objNum int) ([]byte, error) {
	obj, ok := w.objects[objNum]
	if !ok {
		return nil, fmt.Errorf("object %d not found", objNum)
	}
	if obj.Content != nil {
		return obj.Content, nil
	}
	if obj.Stream != nil {
		return obj.Stream, nil
	}
	return nil, fmt.Errorf("object %d has no content", objNum)
}
