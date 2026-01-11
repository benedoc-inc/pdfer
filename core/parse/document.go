// Package parser provides PDF parsing with byte-perfect reconstruction support
package parse

// PDFDocument represents a complete PDF with all revisions and raw bytes preserved
// This enables byte-perfect reconstruction of the original PDF
type PDFDocument struct {
	RawBytes  []byte         // Original complete PDF bytes
	Header    *PDFHeader     // PDF header info
	Revisions []*PDFRevision // All revisions, oldest first
}

// PDFHeader contains PDF header information with exact bytes preserved
type PDFHeader struct {
	Version      string // PDF version (e.g., "1.7")
	MajorVersion int    // Major version number (e.g., 1)
	MinorVersion int    // Minor version number (e.g., 7)
	RawBytes     []byte // Exact header bytes including binary marker (up to first object)
}

// PDFRevision represents a single revision of the PDF
// A PDF can have multiple revisions when it's been incrementally updated
type PDFRevision struct {
	Number    int                   // Revision number (1-indexed)
	Objects   map[int]*PDFRawObject // Objects added/modified in this revision (keyed by object number)
	XRef      *XRefData             // Cross-reference data for this revision
	Trailer   *TrailerData          // Trailer dictionary for this revision
	StartXRef int64                 // startxref value for this revision
	EOFOffset int64                 // Byte offset where %%EOF starts
	EndOffset int64                 // Byte offset after %%EOF (and any trailing newlines)
}

// PDFRawObject contains the raw bytes of a PDF object
// This preserves exact formatting for byte-perfect reconstruction
type PDFRawObject struct {
	Number     int    // Object number
	Generation int    // Generation number
	Offset     int64  // Byte offset in file where object starts
	EndOffset  int64  // Byte offset where object ends (after "endobj")
	RawBytes   []byte // Complete raw bytes from "N G obj" through "endobj" (inclusive)

	// Parsed stream components (populated for stream objects)
	IsStream    bool   // True if this is a stream object
	DictRaw     []byte // Raw dictionary bytes (including << >>)
	StreamRaw   []byte // Raw stream data (between "stream\n" and "\nendstream", excluding keywords)
	DictStart   int    // Offset within RawBytes where dictionary starts
	DictEnd     int    // Offset within RawBytes where dictionary ends
	StreamStart int    // Offset within RawBytes where stream data starts
	StreamEnd   int    // Offset within RawBytes where stream data ends
}

// XRefData represents cross-reference data with exact bytes preserved
type XRefData struct {
	Type     XRefType // Traditional table or stream
	Offset   int64    // Byte offset where xref section starts
	RawBytes []byte   // Complete raw bytes of xref section (for traditional: "xref" through entries)

	// Parsed entries for convenience
	Entries []XRefEntry

	// For xref streams: the object containing the stream
	StreamObject *PDFRawObject
}

// XRefType indicates the type of cross-reference section
type XRefType int

const (
	XRefTypeTable  XRefType = iota // Traditional "xref" table
	XRefTypeStream                 // Cross-reference stream (PDF 1.5+)
)

// XRefEntry represents a single cross-reference entry
type XRefEntry struct {
	ObjectNum  int   // Object number
	Generation int   // Generation number
	Offset     int64 // For type 1: byte offset in file
	InUse      bool  // true = 'n' (in use), false = 'f' (free)

	// For type 2 entries (object in object stream)
	InObjectStream bool // True if this object is in an object stream
	StreamObjNum   int  // Object stream number (if InObjectStream)
	IndexInStream  int  // Index within object stream (if InObjectStream)

	// Raw entry bytes for byte-perfect reconstruction (20 bytes for traditional)
	RawBytes []byte
}

// TrailerData represents trailer information with exact bytes preserved
type TrailerData struct {
	Offset   int64  // Byte offset where "trailer" keyword starts (0 for xref stream)
	RawBytes []byte // Raw bytes of trailer dictionary (including "trailer\n<<...>>")

	// Parsed values for convenience
	Size    int      // /Size value
	Root    string   // /Root reference (e.g., "1 0 R")
	Encrypt string   // /Encrypt reference (if encrypted)
	Info    string   // /Info reference (if present)
	Prev    int64    // /Prev value (offset of previous xref, 0 if none)
	ID      [][]byte // /ID array (two byte strings)
}

// Bytes returns the complete PDF bytes for reconstruction
// This should produce output identical to the original RawBytes
func (d *PDFDocument) Bytes() []byte {
	if d == nil {
		return nil
	}

	// If we have raw bytes and no modifications, return them directly
	if len(d.RawBytes) > 0 {
		return d.RawBytes
	}

	// Reconstruct from components
	return d.reconstruct()
}

// reconstruct rebuilds the PDF from its components
func (d *PDFDocument) reconstruct() []byte {
	// TODO: Implement full reconstruction
	// For now, return raw bytes if available
	return d.RawBytes
}

// GetObject returns an object by number, searching from newest to oldest revision
func (d *PDFDocument) GetObject(objNum int) *PDFRawObject {
	// Search from newest to oldest (last revision first)
	for i := len(d.Revisions) - 1; i >= 0; i-- {
		if obj, ok := d.Revisions[i].Objects[objNum]; ok {
			return obj
		}
	}
	return nil
}

// GetObjectInRevision returns an object from a specific revision
func (d *PDFDocument) GetObjectInRevision(objNum int, revisionNum int) *PDFRawObject {
	if revisionNum < 1 || revisionNum > len(d.Revisions) {
		return nil
	}
	return d.Revisions[revisionNum-1].Objects[objNum]
}

// RevisionCount returns the number of revisions in the PDF
func (d *PDFDocument) RevisionCount() int {
	return len(d.Revisions)
}

// LatestRevision returns the most recent revision
func (d *PDFDocument) LatestRevision() *PDFRevision {
	if len(d.Revisions) == 0 {
		return nil
	}
	return d.Revisions[len(d.Revisions)-1]
}

// ObjectCount returns the total number of unique objects across all revisions
func (d *PDFDocument) ObjectCount() int {
	seen := make(map[int]bool)
	for _, rev := range d.Revisions {
		for objNum := range rev.Objects {
			seen[objNum] = true
		}
	}
	return len(seen)
}

// AllObjects returns all objects from the merged view (latest version of each)
func (d *PDFDocument) AllObjects() map[int]*PDFRawObject {
	result := make(map[int]*PDFRawObject)
	// Process oldest to newest, so newer overwrites older
	for _, rev := range d.Revisions {
		for objNum, obj := range rev.Objects {
			result[objNum] = obj
		}
	}
	return result
}

// Bytes returns the raw bytes of this object
func (o *PDFRawObject) Bytes() []byte {
	return o.RawBytes
}

// Content returns the content between "N G obj" and "endobj"
func (o *PDFRawObject) Content() []byte {
	if len(o.RawBytes) == 0 {
		return nil
	}

	// Find "obj" and skip past it
	start := 0
	for i := 0; i < len(o.RawBytes)-3; i++ {
		if o.RawBytes[i] == 'o' && o.RawBytes[i+1] == 'b' && o.RawBytes[i+2] == 'j' {
			start = i + 3
			// Skip whitespace after "obj"
			for start < len(o.RawBytes) && isWhitespace(o.RawBytes[start]) {
				start++
			}
			break
		}
	}

	// Find "endobj" from the end
	end := len(o.RawBytes)
	for i := len(o.RawBytes) - 6; i >= 0; i-- {
		if i+6 <= len(o.RawBytes) &&
			o.RawBytes[i] == 'e' && o.RawBytes[i+1] == 'n' && o.RawBytes[i+2] == 'd' &&
			o.RawBytes[i+3] == 'o' && o.RawBytes[i+4] == 'b' && o.RawBytes[i+5] == 'j' {
			end = i
			// Trim trailing whitespace before endobj
			for end > start && isWhitespace(o.RawBytes[end-1]) {
				end--
			}
			break
		}
	}

	if start >= end {
		return nil
	}

	return o.RawBytes[start:end]
}

// StreamData returns the decompressed stream data (if this is a stream object)
func (o *PDFRawObject) StreamData() []byte {
	if !o.IsStream {
		return nil
	}
	return o.StreamRaw
}

// Bytes returns the raw bytes of this xref section
func (x *XRefData) Bytes() []byte {
	if x.Type == XRefTypeStream && x.StreamObject != nil {
		return x.StreamObject.RawBytes
	}
	return x.RawBytes
}

// Bytes returns the raw bytes of this trailer
func (t *TrailerData) Bytes() []byte {
	return t.RawBytes
}

// RevisionBytes returns the complete bytes for this revision
// (all objects + xref + trailer + startxref + %%EOF)
func (r *PDFRevision) RevisionBytes(doc *PDFDocument) []byte {
	if doc == nil || len(doc.RawBytes) == 0 {
		return nil
	}

	// Determine start offset (first object in this revision, or xref if no objects)
	startOffset := r.XRef.Offset
	for _, obj := range r.Objects {
		if obj.Offset < startOffset {
			startOffset = obj.Offset
		}
	}

	// End at EOFOffset + length of "%%EOF" + any trailing newlines
	endOffset := r.EndOffset
	if endOffset == 0 {
		endOffset = r.EOFOffset + 5 // len("%%EOF")
		// Include trailing newlines
		for int(endOffset) < len(doc.RawBytes) &&
			(doc.RawBytes[endOffset] == '\r' || doc.RawBytes[endOffset] == '\n') {
			endOffset++
		}
	}

	if int(startOffset) >= len(doc.RawBytes) || int(endOffset) > len(doc.RawBytes) {
		return nil
	}

	return doc.RawBytes[startOffset:endOffset]
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f'
}

// PDFTrailer represents simplified PDF trailer information
// This is a lightweight type for quick trailer parsing without byte preservation.
// For byte-perfect reconstruction, use TrailerData instead.
type PDFTrailer struct {
	RootRef    string // Root reference (e.g., "/Root 204 0 R")
	EncryptRef string // Encrypt reference if present
	InfoRef    string // Info reference if present
	StartXRef  int64  // Byte offset from startxref
}
