package manipulate

import (
	"fmt"
	"strconv"
	"strings"
)

// MetadataUpdate holds the fields to update in the /Info dictionary.
// An empty string means "leave unchanged".
type MetadataUpdate struct {
	Title    string
	Author   string
	Subject  string
	Keywords string
	Creator  string
	Producer string
}

// Permissions describes the permission flags encoded in the /P field of an
// encrypted PDF's /Encrypt dictionary.
type Permissions struct {
	Print            bool
	Modify           bool
	Copy             bool
	AddAnnotations   bool
	FillForms        bool
	ExtractForAccess bool
	Assemble         bool
	PrintHighQuality bool
}

// escapePDFLiteralString wraps s in PDF literal string parentheses, escaping
// backslashes and parentheses as required by the PDF specification.
func escapePDFLiteralString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return "(" + s + ")"
}

// SetMetadata updates the /Info dictionary of a PDF.  For each field in meta
// that is non-empty the corresponding /Info entry is created or replaced.
// Fields that are empty strings are left unchanged.
func SetMetadata(pdfBytes []byte, meta MetadataUpdate, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("set metadata: %w", err)
	}

	trailer := m.pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("set metadata: no trailer found")
	}

	// Map of PDF key -> new value (only non-empty fields).
	updates := map[string]string{}
	if meta.Title != "" {
		updates["/Title"] = escapePDFLiteralString(meta.Title)
	}
	if meta.Author != "" {
		updates["/Author"] = escapePDFLiteralString(meta.Author)
	}
	if meta.Subject != "" {
		updates["/Subject"] = escapePDFLiteralString(meta.Subject)
	}
	if meta.Keywords != "" {
		updates["/Keywords"] = escapePDFLiteralString(meta.Keywords)
	}
	if meta.Creator != "" {
		updates["/Creator"] = escapePDFLiteralString(meta.Creator)
	}
	if meta.Producer != "" {
		updates["/Producer"] = escapePDFLiteralString(meta.Producer)
	}

	if len(updates) == 0 {
		// Nothing to do — return original bytes unchanged.
		return pdfBytes, nil
	}

	var infoObjNum int
	var infoStr string

	if trailer.InfoRef != "" {
		infoObjNum, err = parseObjectRef(trailer.InfoRef)
		if err != nil {
			return nil, fmt.Errorf("set metadata: bad Info ref %q: %w", trailer.InfoRef, err)
		}
		if raw, ok := m.objects[infoObjNum]; ok {
			infoStr = string(raw)
		} else {
			// Ref exists but object is missing — start with an empty dict.
			infoStr = "<<>>"
		}
	} else {
		// No /Info in trailer — we will create a new object.
		infoObjNum = 0
		infoStr = "<<>>"
	}

	// Apply each update to the dict string.
	for key, val := range updates {
		infoStr = setDictValue(infoStr, key, val)
	}

	if infoObjNum == 0 {
		// Allocate a new object number: one above the current maximum.
		maxObjNum := 0
		for n := range m.objects {
			if n > maxObjNum {
				maxObjNum = n
			}
		}
		infoObjNum = maxObjNum + 1
		// Point the trailer at the new object.  rebuildPDF picks up m.pdf.Trailer()
		// for RootRef / EncryptRef but we need to patch InfoRef separately.
		// We do this by updating the PDF's trailer through the writer: after
		// Rebuild() the writer calls SetInfo(infoObjNum) when it processes
		// trailer.InfoRef — but trailer.InfoRef is still empty at that point.
		// Instead we directly call m.writer.SetInfo after adding the object to
		// m.objects; rebuildPDF will honour it because it calls m.writer.SetInfo
		// only when trailer.InfoRef != "".  So we update the trailer ref here.
		//
		// The simplest approach: store the object and let rebuildPDF know by
		// mutating the trailer's InfoRef on the writer side.  Since we can't
		// mutate the parsed trailer, we store infoObjNum in m and override the
		// writer call inside a thin wrapper.
		m.objects[infoObjNum] = []byte(infoStr)
		// We need the writer to emit SetInfo for the new object.  Rebuild()
		// calls m.writer.SetInfo only if trailer.InfoRef != "".  We work around
		// this by rebuilding manually with an override.
		return m.rebuildWithInfo(infoObjNum, infoStr)
	}

	// Existing info object — just update in place.
	m.objects[infoObjNum] = []byte(infoStr)

	if verbose {
		fmt.Printf("SetMetadata: updated Info object %d\n", infoObjNum)
	}

	return m.Rebuild()
}

// rebuildWithInfo is like Rebuild() but forces a specific infoObjNum into the
// writer's trailer, overriding whatever the parsed trailer says.
func (m *PDFManipulator) rebuildWithInfo(infoObjNum int, infoContent string) ([]byte, error) {
	// Add all objects to writer (same as rebuildPDF).
	for objNum, content := range m.objects {
		m.writer.SetObject(objNum, content)
	}

	trailer := m.pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("no trailer found")
	}

	if trailer.RootRef != "" {
		rootObjNum, err := parseObjectRef(trailer.RootRef)
		if err == nil {
			m.writer.SetRoot(rootObjNum)
		}
	}
	// Use the new / updated info object number.
	m.writer.SetInfo(infoObjNum)

	if trailer.EncryptRef != "" {
		encryptObjNum, err := parseObjectRef(trailer.EncryptRef)
		if err == nil {
			m.writer.SetEncryptRef(encryptObjNum)
		}
	}
	if m.pdf.IsEncrypted() {
		m.writer.SetEncryption(m.pdf.Encryption(), nil)
	}

	return m.writer.Bytes()
}

// GetPermissions returns the permission flags for pdfBytes.  For unencrypted
// PDFs all flags are returned as true.  For encrypted PDFs the /P integer from
// the /Encrypt dictionary is decoded per the PDF specification.
func GetPermissions(pdfBytes []byte, password []byte) (*Permissions, error) {
	m, err := NewPDFManipulator(pdfBytes, password, false)
	if err != nil {
		return nil, fmt.Errorf("get permissions: %w", err)
	}

	if !m.pdf.IsEncrypted() {
		return &Permissions{
			Print:            true,
			Modify:           true,
			Copy:             true,
			AddAnnotations:   true,
			FillForms:        true,
			ExtractForAccess: true,
			Assemble:         true,
			PrintHighQuality: true,
		}, nil
	}

	trailer := m.pdf.Trailer()
	if trailer == nil || trailer.EncryptRef == "" {
		return nil, fmt.Errorf("get permissions: encrypted PDF has no /Encrypt reference")
	}

	encryptObjNum, err := parseObjectRef(trailer.EncryptRef)
	if err != nil {
		return nil, fmt.Errorf("get permissions: bad Encrypt ref %q: %w", trailer.EncryptRef, err)
	}

	encryptObj, ok := m.objects[encryptObjNum]
	if !ok {
		return nil, fmt.Errorf("get permissions: Encrypt object %d not found", encryptObjNum)
	}

	// Extract the /P value from the Encrypt dictionary.
	pStr := extractDictValue(string(encryptObj), "/P")
	if pStr == "" {
		return nil, fmt.Errorf("get permissions: /P not found in Encrypt dictionary")
	}

	pVal, err := strconv.ParseInt(strings.TrimSpace(pStr), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("get permissions: cannot parse /P value %q: %w", pStr, err)
	}

	p := int32(pVal)

	return &Permissions{
		Print:            (p & (1 << 2)) != 0,  // bit 3
		Modify:           (p & (1 << 3)) != 0,  // bit 4
		Copy:             (p & (1 << 4)) != 0,  // bit 5
		AddAnnotations:   (p & (1 << 5)) != 0,  // bit 6
		FillForms:        (p & (1 << 8)) != 0,  // bit 9
		ExtractForAccess: (p & (1 << 9)) != 0,  // bit 10
		Assemble:         (p & (1 << 10)) != 0, // bit 11
		PrintHighQuality: (p & (1 << 11)) != 0, // bit 12
	}, nil
}

// RedactMetadata removes document metadata from pdfBytes:
//   - Clears the /Info object (replaces its content with an empty dict).
//   - Removes the /Metadata stream from the document catalog.
//   - Drops the InfoRef from the rebuilt trailer.
func RedactMetadata(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	m, err := NewPDFManipulator(pdfBytes, password, verbose)
	if err != nil {
		return nil, fmt.Errorf("redact metadata: %w", err)
	}

	trailer := m.pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("redact metadata: no trailer found")
	}

	// 1. Zero out the /Info object.
	if trailer.InfoRef != "" {
		infoObjNum, err := parseObjectRef(trailer.InfoRef)
		if err == nil {
			if _, ok := m.objects[infoObjNum]; ok {
				m.objects[infoObjNum] = []byte("<<>>")
				if verbose {
					fmt.Printf("RedactMetadata: cleared Info object %d\n", infoObjNum)
				}
			}
		}
	}

	// 2. Remove /Metadata stream from the document catalog.
	if trailer.RootRef != "" {
		rootObjNum, err := parseObjectRef(trailer.RootRef)
		if err == nil {
			if catalogRaw, ok := m.objects[rootObjNum]; ok {
				catalogStr := string(catalogRaw)
				metaRef := extractDictValue(catalogStr, "/Metadata")
				if metaRef != "" {
					// Remove /Metadata entry from the catalog dict.
					catalogStr = removeDictKey(catalogStr, "/Metadata")
					m.objects[rootObjNum] = []byte(catalogStr)

					// Also clear the metadata stream object content.
					metaObjNum, err := parseObjectRef(metaRef)
					if err == nil {
						if _, ok := m.objects[metaObjNum]; ok {
							m.objects[metaObjNum] = []byte("<<>>")
							if verbose {
								fmt.Printf("RedactMetadata: cleared Metadata stream object %d\n", metaObjNum)
							}
						}
					}
				}
			}
		}
	}

	// 3. Rebuild without the InfoRef in the trailer.
	return m.rebuildWithoutInfo()
}

// rebuildWithoutInfo rebuilds the PDF, explicitly omitting the /Info trailer
// entry so that no Info reference is written.
func (m *PDFManipulator) rebuildWithoutInfo() ([]byte, error) {
	for objNum, content := range m.objects {
		m.writer.SetObject(objNum, content)
	}

	trailer := m.pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("no trailer found")
	}

	if trailer.RootRef != "" {
		rootObjNum, err := parseObjectRef(trailer.RootRef)
		if err == nil {
			m.writer.SetRoot(rootObjNum)
		}
	}
	// Intentionally do NOT call m.writer.SetInfo — this drops the /Info ref.

	if trailer.EncryptRef != "" {
		encryptObjNum, err := parseObjectRef(trailer.EncryptRef)
		if err == nil {
			m.writer.SetEncryptRef(encryptObjNum)
		}
	}
	if m.pdf.IsEncrypted() {
		m.writer.SetEncryption(m.pdf.Encryption(), nil)
	}

	return m.writer.Bytes()
}

// removeDictKey removes a key and its value from a PDF dictionary string.
// It handles the common cases of name values, reference values, and literal
// string values.
func removeDictKey(dictStr, key string) string {
	keyIdx := strings.Index(dictStr, key)
	if keyIdx == -1 {
		return dictStr
	}

	// Find where the value ends.  Skip whitespace after key.
	valueStart := keyIdx + len(key)
	for valueStart < len(dictStr) && isSpace(dictStr[valueStart]) {
		valueStart++
	}
	if valueStart >= len(dictStr) {
		// Just remove the key.
		return dictStr[:keyIdx] + dictStr[keyIdx+len(key):]
	}

	valueEnd := valueStart
	switch dictStr[valueStart] {
	case '(':
		// Literal string — find matching closing ')'.
		depth := 0
		for valueEnd < len(dictStr) {
			c := dictStr[valueEnd]
			if c == '\\' {
				valueEnd += 2
				continue
			}
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					valueEnd++
					break
				}
			}
			valueEnd++
		}
	case '[':
		// Array — find matching ']'.
		depth := 0
		for valueEnd < len(dictStr) {
			if dictStr[valueEnd] == '[' {
				depth++
			} else if dictStr[valueEnd] == ']' {
				depth--
				if depth == 0 {
					valueEnd++
					break
				}
			}
			valueEnd++
		}
	case '<':
		if valueStart+1 < len(dictStr) && dictStr[valueStart+1] == '<' {
			// Nested dict.
			depth := 0
			for valueEnd < len(dictStr)-1 {
				if dictStr[valueEnd] == '<' && dictStr[valueEnd+1] == '<' {
					depth++
					valueEnd += 2
				} else if dictStr[valueEnd] == '>' && dictStr[valueEnd+1] == '>' {
					depth--
					valueEnd += 2
					if depth == 0 {
						break
					}
				} else {
					valueEnd++
				}
			}
		} else {
			// Hex string.
			for valueEnd < len(dictStr) && dictStr[valueEnd] != '>' {
				valueEnd++
			}
			if valueEnd < len(dictStr) {
				valueEnd++ // consume '>'
			}
		}
	default:
		// Name or number: read until whitespace or delimiter.
		for valueEnd < len(dictStr) && !isSpace(dictStr[valueEnd]) && dictStr[valueEnd] != '/' && dictStr[valueEnd] != '>' {
			valueEnd++
		}
	}

	return dictStr[:keyIdx] + dictStr[valueEnd:]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}
