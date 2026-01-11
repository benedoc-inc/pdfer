// Package parser provides PDF parsing functionality.
//
// # Quick Start
//
// Open a PDF:
//
//	pdf, err := parser.Open(pdfBytes)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Get an object:
//
//	obj, err := pdf.GetObject(5)
//
// For encrypted PDFs:
//
//	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
//	    Password: []byte("secret"),
//	})
//
// For byte-perfect reconstruction:
//
//	pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
//	    BytePerfect: true,
//	})
//	reconstructed := pdf.Bytes()
package parser

import (
	"fmt"

	"github.com/benedoc-inc/pdfer/encryption"
	"github.com/benedoc-inc/pdfer/types"
)

// ParseOptions configures PDF parsing behavior
type ParseOptions struct {
	Password    []byte // Password for encrypted PDFs (empty for unencrypted)
	Verbose     bool   // Enable verbose logging
	BytePerfect bool   // Preserve exact bytes for reconstruction
}

// PDF represents a parsed PDF document.
// This is the main entry point for working with PDF files.
type PDF struct {
	raw        []byte
	doc        *PDFDocument         // Populated when BytePerfect is true
	xref       *XRef                // Unified cross-reference data
	encryption *types.PDFEncryption // Encryption info (nil if unencrypted)
	trailer    *TrailerInfo         // Parsed trailer information
	opts       ParseOptions
}

// XRef represents consolidated cross-reference data for all objects in the PDF.
// It merges data from all revisions (for incremental updates) into a single view.
type XRef struct {
	Objects map[int]*ObjectRef // Object number -> reference info
	Size    int                // Total number of objects
}

// ObjectRef describes where a PDF object is located.
type ObjectRef struct {
	Number       int   // Object number
	Generation   int   // Generation number (usually 0)
	Offset       int64 // Byte offset in PDF (0 if in object stream)
	InStream     bool  // True if object is stored in an object stream
	StreamObjNum int   // Object stream number (if InStream is true)
	StreamIndex  int   // Index within object stream (if InStream is true)
}

// TrailerInfo contains parsed trailer dictionary information.
type TrailerInfo struct {
	Size       int    // Number of objects in the file
	RootRef    string // Reference to document catalog (e.g., "1 0 R")
	InfoRef    string // Reference to document info dictionary
	EncryptRef string // Reference to encryption dictionary
	IDArray    []byte // File identifier array
}

// Open parses a PDF from bytes with default options.
// For encrypted PDFs or byte-perfect parsing, use OpenWithOptions.
func Open(data []byte) (*PDF, error) {
	return OpenWithOptions(data, ParseOptions{})
}

// OpenWithOptions parses a PDF with custom options.
func OpenWithOptions(data []byte, opts ParseOptions) (*PDF, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("PDF too short: %d bytes", len(data))
	}

	pdf := &PDF{
		raw:  data,
		opts: opts,
		xref: &XRef{Objects: make(map[int]*ObjectRef)},
	}

	// Handle encryption first
	if err := pdf.handleEncryption(); err != nil {
		return nil, fmt.Errorf("encryption error: %w", err)
	}

	// Parse based on mode
	if opts.BytePerfect {
		if err := pdf.parseBytePerfect(); err != nil {
			return nil, fmt.Errorf("byte-perfect parse failed: %w", err)
		}
	} else {
		if err := pdf.parseStandard(); err != nil {
			return nil, fmt.Errorf("parse failed: %w", err)
		}
	}

	return pdf, nil
}

// handleEncryption checks for encryption and validates password
func (p *PDF) handleEncryption() error {
	// Try to parse encryption dictionary
	enc, err := encryption.ParseEncryptionDictionary(p.raw, p.opts.Verbose)
	if err != nil {
		// No encryption or parse error - assume unencrypted
		return nil
	}

	if enc != nil {
		// PDF is encrypted - validate password
		_, validatedEnc, err := encryption.DecryptPDF(p.raw, p.opts.Password, p.opts.Verbose)
		if err != nil {
			return fmt.Errorf("decryption failed (wrong password?): %w", err)
		}
		p.encryption = validatedEnc
	}

	return nil
}

// parseBytePerfect parses preserving all raw bytes
func (p *PDF) parseBytePerfect() error {
	doc, err := ParsePDFDocument(p.raw)
	if err != nil {
		return err
	}
	p.doc = doc

	// Build unified xref from document revisions
	for _, rev := range doc.Revisions {
		for objNum, obj := range rev.Objects {
			p.xref.Objects[objNum] = &ObjectRef{
				Number:     obj.Number,
				Generation: obj.Generation,
				Offset:     obj.Offset,
				InStream:   false, // Raw objects are always direct in this mode
			}
		}
		if rev.Trailer != nil && rev.Trailer.Size > p.xref.Size {
			p.xref.Size = rev.Trailer.Size
		}
	}

	// Build trailer info from last revision
	if len(doc.Revisions) > 0 {
		lastRev := doc.Revisions[len(doc.Revisions)-1]
		if lastRev.Trailer != nil {
			p.trailer = &TrailerInfo{
				Size:       lastRev.Trailer.Size,
				RootRef:    lastRev.Trailer.Root,
				InfoRef:    lastRev.Trailer.Info,
				EncryptRef: lastRev.Trailer.Encrypt,
			}
		}
	}

	return nil
}

// parseStandard parses for object access (not byte-perfect)
func (p *PDF) parseStandard() error {
	// Use incremental parser to handle all revision types
	incParser := newIncrementalParser(p.raw, p.opts.Verbose)
	if err := incParser.parse(); err != nil {
		// Fall back to simple trailer parsing
		trailer, err := ParsePDFTrailer(p.raw)
		if err != nil {
			return fmt.Errorf("failed to parse PDF structure: %w", err)
		}
		p.trailer = &TrailerInfo{
			RootRef:    trailer.RootRef,
			EncryptRef: trailer.EncryptRef,
			InfoRef:    trailer.InfoRef,
		}
		// Parse xref from startxref
		objMap, _ := ParseCrossReferenceTableWithEncryption(p.raw, trailer.StartXRef, p.encryption, p.opts.Verbose)
		for objNum, offset := range objMap {
			p.xref.Objects[objNum] = &ObjectRef{
				Number: objNum,
				Offset: offset,
			}
		}
		return nil
	}

	// Build xref from incremental parser results
	mergedObjs := incParser.getObjectMap()
	mergedStreams := incParser.getObjectStreamMap()

	for objNum, offset := range mergedObjs {
		p.xref.Objects[objNum] = &ObjectRef{
			Number: objNum,
			Offset: offset,
		}
	}

	for objNum, entry := range mergedStreams {
		p.xref.Objects[objNum] = &ObjectRef{
			Number:       objNum,
			InStream:     true,
			StreamObjNum: entry.StreamObjNum,
			StreamIndex:  entry.IndexInStream,
		}
	}

	// Get trailer info from last section
	sections := incParser.getSections()
	if len(sections) > 0 {
		lastSection := sections[len(sections)-1]
		p.trailer = &TrailerInfo{
			Size:       lastSection.Size,
			RootRef:    lastSection.Root,
			EncryptRef: lastSection.Encrypt,
		}
		p.xref.Size = lastSection.Size
	}

	return nil
}

// Version returns the PDF version string (e.g., "1.7")
func (p *PDF) Version() string {
	if p.doc != nil && p.doc.Header != nil {
		return p.doc.Header.Version
	}
	// Parse version from raw bytes
	header, err := ParsePDFHeader(p.raw)
	if err != nil {
		return "unknown"
	}
	return header.Version
}

// RevisionCount returns the number of revisions (1 for non-incremental PDFs)
func (p *PDF) RevisionCount() int {
	if p.doc != nil {
		return len(p.doc.Revisions)
	}
	// For standard parsing, count %%EOF markers
	return len(FindAllEOFMarkers(p.raw))
}

// ObjectCount returns the number of objects in the PDF
func (p *PDF) ObjectCount() int {
	return len(p.xref.Objects)
}

// Objects returns a list of all object numbers in the PDF
func (p *PDF) Objects() []int {
	result := make([]int, 0, len(p.xref.Objects))
	for objNum := range p.xref.Objects {
		result = append(result, objNum)
	}
	return result
}

// HasObject returns true if the object exists
func (p *PDF) HasObject(objNum int) bool {
	_, ok := p.xref.Objects[objNum]
	return ok
}

// GetObject returns the content of a PDF object by number.
// Returns the raw bytes between "N G obj" and "endobj".
func (p *PDF) GetObject(objNum int) ([]byte, error) {
	ref, ok := p.xref.Objects[objNum]
	if !ok {
		return nil, fmt.Errorf("object %d not found", objNum)
	}

	if ref.InStream {
		// Object is in an object stream - extract it
		return GetObjectFromStream(p.raw, objNum, ref.StreamObjNum, ref.StreamIndex, p.encryption, p.opts.Verbose)
	}

	// Direct object - get from byte offset
	return GetDirectObject(p.raw, objNum, ref.Offset, p.encryption, p.opts.Verbose)
}

// GetRawObject returns a PDFRawObject with full byte preservation.
// Only available when parsed with BytePerfect option.
func (p *PDF) GetRawObject(objNum int) (*PDFRawObject, error) {
	if p.doc == nil {
		return nil, fmt.Errorf("raw objects only available with BytePerfect parsing")
	}

	// Search all revisions (newest first for most recent version)
	for i := len(p.doc.Revisions) - 1; i >= 0; i-- {
		if obj, ok := p.doc.Revisions[i].Objects[objNum]; ok {
			return obj, nil
		}
	}

	return nil, fmt.Errorf("object %d not found", objNum)
}

// Trailer returns the trailer information
func (p *PDF) Trailer() *TrailerInfo {
	return p.trailer
}

// IsEncrypted returns true if the PDF is encrypted
func (p *PDF) IsEncrypted() bool {
	return p.encryption != nil
}

// Encryption returns encryption info (nil if unencrypted)
func (p *PDF) Encryption() *types.PDFEncryption {
	return p.encryption
}

// Bytes returns the PDF as bytes.
// If parsed with BytePerfect option, returns byte-identical reconstruction.
// Otherwise, returns the original input bytes.
func (p *PDF) Bytes() []byte {
	if p.doc != nil {
		return p.doc.Bytes()
	}
	return p.raw
}

// Raw returns the original input bytes (always available)
func (p *PDF) Raw() []byte {
	return p.raw
}

// Document returns the underlying PDFDocument (only for BytePerfect mode)
func (p *PDF) Document() *PDFDocument {
	return p.doc
}
