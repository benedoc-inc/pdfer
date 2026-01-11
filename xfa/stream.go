package xfa

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/types"
)

// PDFStream represents a stream-like interface for reading PDF bytes
type PDFStream struct {
	data   []byte
	offset int
}

func NewPDFStream(data []byte) *PDFStream {
	return &PDFStream{data: data, offset: 0}
}

func (s *PDFStream) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0: // io.SeekStart
		s.offset = int(offset)
	case 1: // io.SeekCurrent
		s.offset += int(offset)
	case 2: // io.SeekEnd
		s.offset = len(s.data) + int(offset)
	}
	if s.offset < 0 {
		s.offset = 0
	}
	if s.offset > len(s.data) {
		s.offset = len(s.data)
	}
	return int64(s.offset), nil
}

func (s *PDFStream) Tell() int64 {
	return int64(s.offset)
}

func (s *PDFStream) Read(n int) ([]byte, error) {
	if s.offset >= len(s.data) {
		return nil, io.EOF
	}
	end := s.offset + n
	if end > len(s.data) {
		end = len(s.data)
	}
	result := s.data[s.offset:end]
	s.offset = end
	return result, nil
}

func (s *PDFStream) ReadByte() (byte, error) {
	if s.offset >= len(s.data) {
		return 0, io.EOF
	}
	b := s.data[s.offset]
	s.offset++
	return b, nil
}

func (s *PDFStream) Peek(n int) []byte {
	if s.offset >= len(s.data) {
		return nil
	}
	end := s.offset + n
	if end > len(s.data) {
		end = len(s.data)
	}
	return s.data[s.offset:end]
}

// skipOverWhitespace skips whitespace characters, returns true if any were skipped
// Matches PyPDF's skip_over_whitespace function
func skipOverWhitespace(stream *PDFStream) bool {
	skipped := false
	for {
		b, err := stream.ReadByte()
		if err != nil {
			stream.Seek(-1, 1) // Seek back
			break
		}
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			skipped = true
		} else {
			stream.Seek(-1, 1) // Seek back
			break
		}
	}
	return skipped
}

// readUntilWhitespace reads until whitespace is encountered
// Matches PyPDF's read_until_whitespace function
func readUntilWhitespace(stream *PDFStream) ([]byte, error) {
	var result []byte
	for {
		b, err := stream.ReadByte()
		if err != nil {
			break
		}
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			stream.Seek(-1, 1) // Seek back
			break
		}
		result = append(result, b)
	}
	return result, nil
}

// skipOverComment skips PDF comments (lines starting with %)
// Matches PyPDF's skip_over_comment function
func skipOverComment(stream *PDFStream) {
	b, err := stream.ReadByte()
	if err != nil {
		return
	}
	if b == '%' {
		// Skip until newline
		for {
			b, err := stream.ReadByte()
			if err != nil || b == '\n' || b == '\r' {
				break
			}
		}
	} else {
		stream.Seek(-1, 1) // Seek back
	}
}

// readObjectHeader reads "212 0 obj" from stream
// Matches PyPDF's read_object_header function (lines 540-565)
func readObjectHeader(stream *PDFStream) (int, int, error) {
	// Skip comments
	skipOverComment(stream)
	
	// Skip whitespace
	skipOverWhitespace(stream)
	stream.Seek(-1, 1) // PyPDF seeks back after skip
	
	// Read object number
	idnumBytes, err := readUntilWhitespace(stream)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read object number: %v", err)
	}
	
	// Skip whitespace
	skipOverWhitespace(stream)
	stream.Seek(-1, 1) // PyPDF seeks back (line 550)
	
	// Read generation number
	// At this point, we're at the space after the object number
	// We need to skip whitespace first, then read
	skipOverWhitespace(stream) // Skip the space to get to generation number
	genBytes, err := readUntilWhitespace(stream)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read generation number: %v", err)
	}
	
	// Skip whitespace (PyPDF line 552)
	skipOverWhitespace(stream)
	stream.Seek(-1, 1) // PyPDF seeks back (line 553)
	// After seeking back, we're at the space before "obj", so skip again
	skipOverWhitespace(stream)
	
	// Read "obj" keyword (3 bytes) - PyPDF line 556: _obj = stream.read(3)
	objBytes, err := stream.Read(3)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read 'obj' keyword: %v", err)
	}
	if !bytes.Equal(objBytes, []byte("obj")) {
		return 0, 0, fmt.Errorf("expected 'obj' keyword, got %q", string(objBytes))
	}
	
	// Read non-whitespace (PyPDF line 558) - this reads one char to verify we're past "obj"
	// PyPDF: read_non_whitespace(stream); stream.seek(-1, 1)
	skipOverWhitespace(stream)
	stream.Seek(-1, 1)
	
	idnum, err := strconv.Atoi(string(idnumBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid object number: %v", err)
	}
	
	generation, err := strconv.Atoi(string(genBytes))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid generation number: %v", err)
	}
	
	return idnum, generation, nil
}

// findObjectHeaderByRegex searches for object header using regex (fallback method)
// Matches PyPDF's fallback approach (lines 432-452)
// PyPDF searches the ENTIRE file buffer, not just a window
func findObjectHeaderByRegex(pdfBytes []byte, objNum, genNum int) (int, error) {
	// PyPDF line 439: searches entire buffer
	// Pattern: \s{objNum}\s+{genNum}\s+obj
	pattern := regexp.MustCompile(fmt.Sprintf(`\s%d\s+%d\s+obj`, objNum, genNum))
	matches := pattern.FindIndex(pdfBytes)
	if matches == nil {
		return -1, fmt.Errorf("object header %d %d obj not found", objNum, genNum)
	}
	// PyPDF line 451: uses m.start(0) + 1
	// This positions stream at the character AFTER the leading whitespace
	return matches[0] + 1, nil
}

// ReadObjectFromXRef reads an object using xref offset (PyPDF approach)
// Matches PyPDF's get_object method (lines 413-490)
func ReadObjectFromXRef(pdfBytes []byte, objNum, genNum int, xrefOffset int64, encryptInfo *types.PDFEncryption, verbose bool) ([]byte, error) {
	// Step 1: Create stream and seek to xref offset (PyPDF line 423)
	stream := NewPDFStream(pdfBytes)
	
	// Step 2: Try to find object header
	// The xref offset might point to encrypted content, so we need to search for the header
	// Per PDF spec, object headers are NOT encrypted, so we should find "212 0 obj" in plaintext
	if verbose {
		log.Printf("ReadObjectFromXRef: Looking for object %d %d, xref offset: %d", objNum, genNum, xrefOffset)
	}
	
	// First, try reading header at xref offset (PyPDF line 425)
	_, err := stream.Seek(xrefOffset, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to xref offset: %v", err)
	}
	
	idnum, generation, err := readObjectHeader(stream)
	if err != nil {
		// Header not found at xref offset - search backwards and forwards
		// The xref offset might point to the object content, not the header
		if verbose {
			log.Printf("readObjectHeader failed at offset %d, searching for header: %v", xrefOffset, err)
		}
		
		// Search in a window around xref offset (headers are usually close)
		searchStart := types.Max(0, int(xrefOffset)-500)
		searchEnd := types.Min(len(pdfBytes), int(xrefOffset)+500)
		searchArea := pdfBytes[searchStart:searchEnd]
		
		pattern := []byte(fmt.Sprintf("%d %d obj", objNum, genNum))
		patternPos := bytes.Index(searchArea, pattern)
		if patternPos != -1 {
			headerOffset := searchStart + patternPos
			if verbose {
				log.Printf("Found object header at offset %d (searched window around xref offset)", headerOffset)
			}
			_, err = stream.Seek(int64(headerOffset), 0)
			if err != nil {
				return nil, fmt.Errorf("failed to seek to found header: %v", err)
			}
			idnum, generation, err = readObjectHeader(stream)
			if err != nil {
				return nil, fmt.Errorf("failed to read header at found offset: %v", err)
			}
		} else {
			// Fallback: search entire file with regex (PyPDF lines 432-452)
			if verbose {
				log.Printf("Header not found in window, trying full file regex search")
			}
			offset, err := findObjectHeaderByRegex(pdfBytes, objNum, genNum)
			if err != nil {
			if verbose {
				log.Printf("Regex search also failed: %v", err)
				log.Printf("Object header not found in plaintext - object may be in object stream or header is encrypted")
				log.Printf("Attempting to parse dictionary structure from encrypted content at xref offset")
			}
			// If header not found, try to parse from xref offset anyway
			// The structure markers might still be parseable
			_, err = stream.Seek(xrefOffset, 0)
			if err != nil {
				return nil, fmt.Errorf("object header not found and cannot seek to xref: %v", err)
			}
			// We'll try to parse the dictionary structure even without the header
			// Set idnum/generation from parameters
			idnum = objNum
			generation = genNum
			}
			if verbose {
				log.Printf("Regex found object header at offset %d", offset)
			}
			_, err = stream.Seek(int64(offset), 0)
			if err != nil {
				return nil, fmt.Errorf("failed to seek to found offset: %v", err)
			}
			idnum, generation, err = readObjectHeader(stream)
			if err != nil {
				// Even if header read fails, continue with xref offset
				// The structure might still be parseable
				if verbose {
					log.Printf("Failed to read header after regex find, continuing from xref offset: %v", err)
				}
				_, err = stream.Seek(xrefOffset, 0)
				if err != nil {
					return nil, fmt.Errorf("failed to seek back to xref offset: %v", err)
				}
				idnum = objNum
				generation = genNum
			}
		}
	}
	
	if verbose {
		log.Printf("ReadObjectFromXRef: Proceeding with idnum=%d, generation=%d, stream pos=%d", idnum, generation, stream.Tell())
	}
	
	if verbose {
		log.Printf("ReadObjectFromXRef: Successfully read header - idnum=%d, generation=%d, stream pos=%d", idnum, generation, stream.Tell())
	}
	
	// Step 3: Verify object number matches (PyPDF lines 426-472)
	if idnum != objNum {
		return nil, fmt.Errorf("object number mismatch: expected %d, got %d", objNum, idnum)
	}
	if generation != genNum {
		if verbose {
			log.Printf("Generation number mismatch: expected %d, got %d", genNum, generation)
		}
		// Continue anyway (non-strict mode)
	}
	
	// Step 4: Parse object structure FIRST, then decrypt (PyPDF approach)
	// PyPDF's read_object function parses the structure first (line 1441-1491)
	// It checks the first byte to determine object type
	// For dictionaries, it expects "<<" (line 1448-1449)
	// Then PyPDF decrypts the parsed object's values (line 488-490)
	
	objStart := int(stream.Tell())
	
	// Check if we're at dictionary start
	peek := stream.Peek(2)
	if len(peek) < 2 || peek[0] != '<' || peek[1] != '<' {
		// Not at dictionary start - the content might be encrypted
		// Try to decrypt a small chunk first to see if we can find "<<" after decryption
		if encryptInfo != nil && verbose {
			log.Printf("Not at dictionary start (got %q), content appears encrypted", peek)
			log.Printf("Attempting to decrypt and find dictionary structure")
		}
		
		// Try decrypting a window to find dictionary start
		// Read a reasonable chunk (first 200 bytes should contain "<<" if it's a dictionary)
		testWindow := types.Min(200, len(pdfBytes)-objStart)
		if testWindow > 0 {
			testContent := pdfBytes[objStart : objStart+testWindow]
			decryptedTest, err := encryption.DecryptObject(testContent, objNum, generation, encryptInfo)
			if err == nil && bytes.Contains(decryptedTest, []byte("<<")) {
				// Found dictionary after decryption - the entire content needs decryption first
				if verbose {
					log.Printf("Found '<<' in decrypted test window - decrypting full content first")
				}
				// Read full object content
				endobjPos := bytes.Index(pdfBytes[objStart:], []byte("endobj"))
				if endobjPos == -1 {
					return nil, fmt.Errorf("endobj not found")
				}
				objEnd := objStart + endobjPos
				objContent := pdfBytes[objStart:objEnd]
				
				// Decrypt full content
				decrypted, err := encryption.DecryptObject(objContent, objNum, generation, encryptInfo)
				if err != nil {
					decrypted, err = decryptInChunks(objContent, objNum, generation, encryptInfo, verbose)
					if err != nil {
						return nil, fmt.Errorf("failed to decrypt object: %v", err)
					}
				}
				
				// Now parse dictionary from decrypted content
				decryptedStream := NewPDFStream(decrypted)
				parsedDict, _, err := parseDictionaryFromStream(decryptedStream, decrypted, verbose)
				if err != nil {
					// If parsing fails, return decrypted content as-is
					if verbose {
						log.Printf("Failed to parse dictionary from decrypted content: %v", err)
					}
					objContent = decrypted
				} else {
					// Dictionary parsed successfully - reconstruct it
					objContent = reconstructDictionary(parsedDict)
				}
				
				// Reconstruct
				result := make([]byte, 0, len(objContent)+50)
				result = append(result, []byte(fmt.Sprintf("%d %d obj\n", objNum, genNum))...)
				result = append(result, objContent...)
				result = append(result, []byte("\nendobj")...)
				return result, nil
			}
		}
		
		// Fall back to old approach
		if verbose {
			log.Printf("Not a dictionary (peek=%q), trying to find endobj", peek)
		}
		endobjPos := bytes.Index(pdfBytes[objStart:], []byte("endobj"))
		if endobjPos == -1 {
			return nil, fmt.Errorf("endobj not found")
		}
		objEnd := objStart + endobjPos
		objContent := pdfBytes[objStart:objEnd]
		
		// Decrypt if needed
		if encryptInfo != nil {
			decrypted, err := encryption.DecryptObject(objContent, objNum, generation, encryptInfo)
			if err != nil {
				decrypted, err = decryptInChunks(objContent, objNum, generation, encryptInfo, verbose)
				if err != nil {
					return nil, fmt.Errorf("failed to decrypt object: %v", err)
				}
			}
			objContent = decrypted
		}
		
		// Reconstruct
		result := make([]byte, 0, len(objContent)+50)
		result = append(result, []byte(fmt.Sprintf("%d %d obj\n", objNum, genNum))...)
		result = append(result, objContent...)
		result = append(result, []byte("\nendobj")...)
		return result, nil
	}
	
	// We're at dictionary start - PARSE STRUCTURE FIRST (PyPDF's DictionaryObject.read_from_stream)
	// Parse the dictionary from raw bytes (structure markers like "<<" are in plaintext)
	parsedDict, _, err := parseDictionaryFromStream(stream, pdfBytes, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dictionary: %v", err)
	}
	
	// Step 5: Decrypt the parsed dictionary's values (PyPDF lines 482-490)
	// PyPDF decrypts the parsed object, not raw bytes
	if encryptInfo != nil {
		decryptedDict, err := decryptParsedDictionary(parsedDict, objNum, generation, encryptInfo, verbose)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt dictionary values: %v", err)
		}
		parsedDict = decryptedDict
	}
	
	// Reconstruct the decrypted dictionary
	objContent := reconstructDictionary(parsedDict)
	
	// Reconstruct full object
	result := make([]byte, 0, len(objContent)+50)
	result = append(result, []byte(fmt.Sprintf("%d %d obj\n", objNum, genNum))...)
	result = append(result, objContent...)
	result = append(result, []byte("\nendobj")...)
	
	return result, nil
}

// hexDump creates a hex dump of bytes (for debugging)
func hexDump(data []byte) string {
	var result strings.Builder
	for i := 0; i < len(data); i += 16 {
		result.WriteString(fmt.Sprintf("%04x: ", i))
		for j := 0; j < 16 && i+j < len(data); j++ {
			result.WriteString(fmt.Sprintf("%02x ", data[i+j]))
		}
		result.WriteString("\n")
	}
	return result.String()
}

// asciiDump creates an ASCII dump (non-printable as .) for debugging
func asciiDump(data []byte) string {
	var result strings.Builder
	for i, b := range data {
		if i > 0 && i%80 == 0 {
			result.WriteString("\n")
		}
		if b >= 32 && b < 127 {
			result.WriteByte(b)
		} else {
			result.WriteString(".")
		}
	}
	return result.String()
}

// ParsedDictEntry represents a key-value pair in a parsed dictionary
type ParsedDictEntry struct {
	Key   []byte // Key (e.g., "/XFA")
	Value []byte // Value (raw bytes, may be encrypted)
}

// parseDictionaryFromStream parses a dictionary from the stream (PyPDF's DictionaryObject.read_from_stream)
// This parses the STRUCTURE from raw bytes, even if values are encrypted
// Returns: parsed entries, end position, error
func parseDictionaryFromStream(stream *PDFStream, pdfBytes []byte, verbose bool) ([]ParsedDictEntry, int, error) {
	// Read "<<" marker (PyPDF line 549-557)
	tmp, err := stream.Read(2)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read dictionary start: %v", err)
	}
	if !bytes.Equal(tmp, []byte("<<")) {
		return nil, 0, fmt.Errorf("dictionary must begin with '<<', got %q", tmp)
	}
	
	var entries []ParsedDictEntry
	
	// Parse key-value pairs (PyPDF lines 560-600)
	for {
		// Skip whitespace and comments
		skipOverWhitespace(stream)
		skipOverComment(stream)
		
		// Check for dictionary end ">>"
		peek := stream.Peek(2)
		if len(peek) >= 2 && peek[0] == '>' && peek[1] == '>' {
			stream.Read(2) // Consume ">>"
			break
		}
		
		// Read key (should be a name object starting with "/")
		keyStart := int(stream.Tell())
		keyBytes, err := readNameObject(stream)
		if err != nil {
			// Try to find ">>" as fallback
			remaining := pdfBytes[keyStart:]
			endPos := bytes.Index(remaining, []byte(">>"))
			if endPos != -1 {
				stream.Seek(int64(keyStart+endPos+2), 0)
				break
			}
			return nil, 0, fmt.Errorf("failed to read dictionary key: %v", err)
		}
		
		// Skip whitespace
		skipOverWhitespace(stream)
		
		// Read value (could be any PDF object)
		valueBytes, err := readPDFObject(stream, pdfBytes)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to read dictionary value for key %q: %v", string(keyBytes), err)
		}
		
		entries = append(entries, ParsedDictEntry{
			Key:   keyBytes,
			Value: valueBytes,
		})
		
		if verbose {
			log.Printf("Parsed dict entry: %s = %d bytes", string(keyBytes), len(valueBytes))
		}
	}
	
	endPos := int(stream.Tell())
	return entries, endPos, nil
}

// readNameObject reads a PDF name object (e.g., "/XFA")
func readNameObject(stream *PDFStream) ([]byte, error) {
	b, err := stream.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != '/' {
		return nil, fmt.Errorf("name object must start with '/', got %c", b)
	}
	
	var result []byte
	result = append(result, b)
	
	for {
		b, err := stream.ReadByte()
		if err != nil {
			break
		}
		// Name objects end at whitespace or special characters
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '/' || b == '[' || b == ']' || b == '<' || b == '>' || b == '(' || b == ')' {
			stream.Seek(-1, 1)
			break
		}
		result = append(result, b)
	}
	
	return result, nil
}

// readPDFObject reads a PDF object value (simplified - reads until next key or dict end)
// For our use case, we read the value as raw bytes until we encounter:
// - Next dictionary key (starts with "/" after whitespace)
// - Dictionary end (">>")
// - End of object ("endobj")
func readPDFObject(stream *PDFStream, pdfBytes []byte) ([]byte, error) {
	startPos := int(stream.Tell())
	currentPos := startPos
	
	// Look ahead to find where this value ends
	// Strategy: scan forward until we find:
	// 1. "/" followed by whitespace (next key)
	// 2. ">>" (dictionary end)
	// 3. "endobj" (object end)
	
	// We need to handle nested structures, so track depth for dicts/arrays
	dictDepth := 0
	arrayDepth := 0
	stringDepth := 0
	
	for currentPos < len(pdfBytes) {
		// Check for dictionary end
		if currentPos+1 < len(pdfBytes) && pdfBytes[currentPos] == '>' && pdfBytes[currentPos+1] == '>' {
			if dictDepth == 0 && arrayDepth == 0 && stringDepth == 0 {
				// Found dictionary end - value ends before this
				stream.Seek(int64(currentPos), 0)
				return pdfBytes[startPos:currentPos], nil
			}
		}
		
		// Check for next key (starts with "/" after whitespace)
		if currentPos > 0 && (pdfBytes[currentPos-1] == ' ' || pdfBytes[currentPos-1] == '\t' || pdfBytes[currentPos-1] == '\n' || pdfBytes[currentPos-1] == '\r') {
			if pdfBytes[currentPos] == '/' && dictDepth == 0 && arrayDepth == 0 && stringDepth == 0 {
				// Found next key - value ends before this
				stream.Seek(int64(currentPos), 0)
				return pdfBytes[startPos:currentPos], nil
			}
		}
		
		// Track nested structures
		if stringDepth == 0 { // Only track depth when not in string
			if pdfBytes[currentPos] == '<' && currentPos+1 < len(pdfBytes) && pdfBytes[currentPos+1] == '<' {
				dictDepth++
				currentPos++ // Skip second '<'
			} else if pdfBytes[currentPos] == '>' && currentPos+1 < len(pdfBytes) && pdfBytes[currentPos+1] == '>' {
				dictDepth--
				currentPos++ // Skip second '>'
			} else if pdfBytes[currentPos] == '[' {
				arrayDepth++
			} else if pdfBytes[currentPos] == ']' {
				arrayDepth--
			}
		}
		
		// Track strings (handle escape sequences)
		if pdfBytes[currentPos] == '(' && (currentPos == 0 || pdfBytes[currentPos-1] != '\\') {
			stringDepth++
		} else if pdfBytes[currentPos] == ')' && stringDepth > 0 && (currentPos == 0 || pdfBytes[currentPos-1] != '\\') {
			stringDepth--
		}
		
		currentPos++
	}
	
	// Reached end - return what we have
	stream.Seek(int64(currentPos), 0)
	return pdfBytes[startPos:currentPos], nil
}

// decryptParsedDictionary decrypts the values in a parsed dictionary (PyPDF's decrypt_object)
func decryptParsedDictionary(entries []ParsedDictEntry, objNum, genNum int, encryptInfo *types.PDFEncryption, verbose bool) ([]ParsedDictEntry, error) {
	result := make([]ParsedDictEntry, len(entries))
	
	for i, entry := range entries {
		result[i].Key = entry.Key // Keys are not encrypted
		
		// Decrypt the value
		decrypted, err := encryption.DecryptObject(entry.Value, objNum, genNum, encryptInfo)
		if err != nil {
			// Try chunked decryption for large values
			decrypted, err = decryptInChunks(entry.Value, objNum, genNum, encryptInfo, verbose)
			if err != nil {
				if verbose {
					log.Printf("Failed to decrypt value for key %s: %v", string(entry.Key), err)
				}
				// Keep original value if decryption fails
				result[i].Value = entry.Value
				continue
			}
		}
		result[i].Value = decrypted
		
		if verbose && bytes.Equal(entry.Key, []byte("/XFA")) {
			log.Printf("Decrypted /XFA value: %d bytes -> %d bytes", len(entry.Value), len(decrypted))
		}
	}
	
	return result, nil
}

// reconstructDictionary reconstructs a parsed dictionary back to PDF bytes
func reconstructDictionary(entries []ParsedDictEntry) []byte {
	var result bytes.Buffer
	result.WriteString("<<\n")
	
	for _, entry := range entries {
		result.Write(entry.Key)
		result.WriteByte(' ')
		result.Write(entry.Value)
		result.WriteByte('\n')
	}
	
	result.WriteString(">>")
	return result.Bytes()
}
