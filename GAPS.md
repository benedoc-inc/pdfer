# Implementation Gaps

This document details the current implementation gaps in pdfer and provides guidance for contributors.

## Summary

| Category | Implemented | Partial | Not Implemented |
|----------|-------------|---------|-----------------|
| Encryption | 4 | 0 | 1 |
| PDF Parsing | 13 | 2 | 20+ |
| PDF Writing | 8 | 2 | 25+ |
| XFA | 6 | 2 | 4 |
| Document Manipulation | 0 | 0 | 8 |
| Content Extraction | 0 | 0 | 7 |
| Advanced Features | 0 | 0 | 10+ |
| Form Handling | 0 | 0 | 5 |
| Font Features | 1 | 0 | 5 |
| Image Features | 2 | 0 | 6 |
| Error Handling | 0 | 0 | 5 |

---

## Encryption (`encryption/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| RC4 40-bit (V1, R2) | `decrypt.go` | Standard handler |
| RC4 128-bit (V2, R3) | `decrypt.go` | Standard handler |
| AES-128 CBC (V4, R4) | `decrypt.go` | With IV prefix handling |
| AES-256 CBC (V5, R5/R6) | `decrypt.go`, `key_derivation.go` | SHA-256 key derivation, /UE/OE unwrapping |
| Password verification | `key_derivation.go` | User and owner passwords (V1-V5) |
| Key derivation | `key_derivation.go` | Algorithm 2 (V1-V4), 7.6.4.3.3 (V5+) |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **AES-256 (V5, R5/R6)** | `key_derivation.go` | SHA-256 based key derivation per ISO 32000-2 |
| **Key unwrapping (/UE, /OE)** | `key_derivation.go` | AES-128 ECB mode for unwrapping encrypted keys |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| Public key encryption | Low | High | Certificate-based, rarely used |
| Crypt filters per stream | Medium | Medium | Different encryption per stream type |

---

## PDF Parsing (`parser/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| Cross-reference tables | `parser.go` | Traditional xref format |
| Cross-reference streams | `xref_stream.go` | PDF 1.5+ compressed xref |
| Object streams (ObjStm) | `object_stream.go` | Compressed object storage |
| PNG predictor filters | `object_stream.go` | Predictors 10-15 |
| FlateDecode | Multiple | zlib/deflate decompression |
| Object retrieval | `get_object.go` | Unified direct + ObjStm access |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Trailer parsing** | Works for most | Hybrid-reference files (xref table + stream) not fully supported |
| **Indirect object references** | Basic | Doesn't resolve nested references automatically |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **ASCIIHexDecode** | `filters.go` | Encode and decode hex text |
| **ASCII85Decode** | `filters.go` | Encode and decode base-85 |
| **RunLengthDecode** | `filters.go` | Simple RLE compression |
| **DCTDecode** | `filters.go` | JPEG image pass-through filter |
| **Incremental updates** | `incremental.go` | Parse PDFs with multiple revisions, /Prev chain |
| **Byte-perfect parsing** | `document.go`, `document_parser.go` | Full PDF structure with raw bytes preserved |
| **Unified API** | `api.go` | Clean `Open()`/`OpenWithOptions()` entry point with `PDF` type |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Linearized PDFs** | Medium | High | First-page optimization, hint tables |
| **LZWDecode filter** | Low | Medium | Legacy compression, rarely used now |
| **CCITTFaxDecode** | Low | High | Fax image compression |
| **JBIG2Decode** | Low | High | Bi-level image compression |
| **JPXDecode** | Low | High | JPEG 2000 |
| **Text extraction** | High | Medium | Extract text from content streams with positioning |
| **Metadata extraction** | Medium | Low | Document info, XMP metadata parsing |
| **Page extraction** | High | Medium | Extract individual pages as new PDFs |
| **PDF merging** | High | Medium | Combine multiple PDFs into one |
| **PDF splitting** | High | Medium | Split PDF into multiple files by pages |
| **Page rotation** | Medium | Low | Rotate pages 90/180/270 degrees |
| **Page deletion** | Medium | Medium | Remove pages from PDF |
| **Page insertion** | Medium | Medium | Insert pages at specific positions |
| **Bookmark/outline extraction** | Medium | Medium | Extract document navigation structure |
| **Annotation extraction** | Medium | High | Links, comments, highlights, form fields |
| **Form field extraction (AcroForm)** | High | High | ✅ Implemented - Extract AcroForm fields |
| **Image extraction** | Medium | Medium | Extract embedded images from PDF |
| **PDF/A compliance** | Low | Very High | PDF/A-1, PDF/A-2, PDF/A-3 validation |
| **PDF/X support** | Low | Very High | PDF/X-1a, PDF/X-3, PDF/X-4 |
| **Error recovery** | Medium | High | Graceful handling of corrupted PDFs |
| **Streaming parser** | Low | High | Parse large PDFs without loading entire file |

---

## PDF Writing (`writer/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| Basic PDF structure | `writer.go` | Header, objects, xref, trailer |
| Stream objects | `writer.go` | With FlateDecode compression |
| XFA PDF building | `xfa_builder.go` | From XFA streams |
| PDF rebuilding | `xfa_builder.go` | Modify existing PDFs |
| Object encryption | `writer.go` | AES-128 for streams |
| **Image embedding** | `image.go` | JPEG (DCTDecode), PNG to RGB XObjects |
| **Page content streams** | `content.go` | Text, graphics, and image operators |
| **Page building** | `page.go` | SimplePDFBuilder for easy page creation |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Dictionary writing** | Basic | Complex nested structures may not format correctly |
| **String encryption** | Partial | Only stream data encrypted, not string objects in dictionaries |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **Font embedding** | `font/font.go`, `font/pdf.go` | TrueType/OpenType font embedding with subsetting support |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **AcroForm parsing** | `acroform/parser.go` | Extract AcroForm structure and fields |
| **Form field extraction** | `acroform/extract.go` | Extract field values and convert to FormSchema |
| **Form field filling** | `acroform/fill.go`, `acroform/replace.go` | Fill AcroForm fields with values (basic structure) |
| **Form field creation** | `acroform/writer.go` | Create AcroForm fields programmatically |
| **Form field validation** | `acroform/validation.go` | Validate field values against constraints |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Encryption on write** | Medium | Medium | Generate new encrypted PDFs |
| **Cross-reference streams** | Medium | Medium | Write modern xref format |
| **Object streams** | Medium | Medium | Compress objects on write |
| **Digital signatures** | Low | Very High | PKCS#7, CMS signing |
| **Incremental save** | Medium | Medium | Append without rewriting |
| **AcroForm creation** | High | High | Create new AcroForm fields programmatically |
| **Form field validation** | Medium | Medium | Validate field values before filling |
| **Form flattening** | Medium | High | Convert form fields to static content |
| **Advanced graphics** | Medium | High | Curves (bezier), arcs, gradients, patterns |
| **Transparency/alpha** | Medium | High | Alpha channels, blend modes, soft masks |
| **Annotations (write)** | High | High | Links, comments, highlights, form fields |
| **Bookmarks/outlines (write)** | Medium | Medium | Create document navigation structure |
| **Form fields (AcroForm)** | High | High | Create and fill AcroForm fields |
| **Watermarks** | Medium | Medium | Add text/image watermarks to pages |
| **Page manipulation** | High | Medium | Rotate, delete, reorder, insert pages |
| **PDF optimization** | Medium | High | Remove unused objects, compress streams |
| **Metadata (write)** | Medium | Low | Set document info, XMP metadata |
| **Multiple image formats** | Medium | Medium | TIFF, GIF, BMP, WebP support |
| **Font subsetting (advanced)** | Low | High | Full TTF subsetting with table rebuilding |
| **Type 1 fonts** | Low | Medium | PostScript Type 1 font support |
| **CID fonts** | Low | High | CID-keyed fonts for CJK languages |
| **Color spaces** | Medium | Medium | CMYK, Lab, ICC profiles, spot colors |
| **Layers/OCGs** | Low | High | Optional content groups, layer visibility |
| **3D content** | Low | Very High | 3D annotations, U3D, PRC |
| **Multimedia** | Low | Very High | Video, audio, rich media annotations |
| **Accessibility (write)** | Medium | High | Tagged PDF, structure tree, alt text |
| **PDF repair** | Low | Very High | Fix corrupted PDFs, recover content |

**Font embedding implementation:**
```go
// Package font/ provides TrueType/OpenType font embedding
// Usage example:
fontData, _ := os.ReadFile("font.ttf")
font, _ := font.NewFont("MyFont", fontData)
font.AddString("Hello, World!") // Subset to used characters

// Add to page
fontName, _ := page.AddEmbeddedFont(font)
content.SetFont(fontName, 12).ShowText("Hello, World!")
```

---

## XFA Processing (`xfa/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| Template extraction | `xfa.go` | Form structure |
| Datasets extraction | `xfa.go` | Form data |
| Config extraction | `xfa.go` | Rendering config |
| LocaleSet extraction | `xfa.go` | Localization |
| Form field parsing | `xfa_form_translator.go` | Text, numeric, choice, date |
| Validation rules | `xfa_form_translator.go` | Required, patterns, ranges |
| Calculation rules | `xfa_form_translator.go` | Basic field calculations |
| Field value updates | `xfa.go` | Modify datasets |
| PDF rebuild | `xfa.go` | With updated XFA |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Subform parsing** | Basic | Nested subforms may not fully resolve |
| **Rich text** | Not handled | XHTML content in text fields |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Dynamic XFA rendering** | Low | Very High | Form layout calculation |
| **Script execution** | Low | Very High | FormCalc, JavaScript |
| **Subform repetition** | Medium | High | min/max occurs, dynamic rows |
| **Barcode fields** | Low | Medium | Generate barcode images |
| **Signature fields** | Low | High | XFA signature handling |
| **Accessibility** | Low | Medium | Role maps, structure |

**To implement subform repetition:**
```go
// In xfa_form_translator.go, add:
type SubformInstance struct {
    Template  *Subform
    Index     int
    Data      map[string]string
}

func (s *Subform) CreateInstances(data []map[string]string) []SubformInstance {
    // 1. Check minOccur/maxOccur
    // 2. Clone subform template for each data row
    // 3. Bind data to cloned fields
}
```

---

## Types (`types/`)

### Current Types

| Type | Purpose | Status |
|------|---------|--------|
| `PDFEncryption` | Encryption parameters | ✅ Complete |
| `FormSchema` | Parsed form structure | ✅ Complete |
| `Question` | Form field | ✅ Complete |
| `Rule` | Validation/calculation | ✅ Complete |
| `XFADatasets` | Parsed datasets | ⚠️ Basic |
| `XFAConfig` | Parsed config | ⚠️ Basic |

### Missing Types

| Type | Purpose | Priority |
|------|---------|----------|
| `Page` | Page object with content | High |
| `Font` | Font resource | ✅ Complete |
| `Image` | Image XObject | High |
| `Annotation` | Form fields, links | Medium |
| `Outline` | Bookmarks | Low |
| `Metadata` | Document metadata (Info, XMP) | Medium |
| `ColorSpace` | Color space definitions | Medium |
| `Pattern` | Tiling and shading patterns | Low |
| `Gradient` | Gradient definitions | Low |
| `FormXObject` | Reusable form XObjects | Medium |
| `Action` | PDF actions (GoTo, URI, etc.) | Medium |
| `Destination` | Named destinations for navigation | Low |

---

## Testing Gaps

### Current Test Coverage

| Package | Coverage | Notes |
|---------|----------|-------|
| `encryption` | ~70% | Missing edge cases |
| `parser` | ~60% | Missing malformed PDF handling |
| `writer` | ~50% | Missing complex structures |
| `xfa` | ~65% | Good roundtrip tests |
| `types` | ~80% | Mostly complete |

### Needed Tests

1. **Malformed PDF handling** - Graceful failures for corrupted files
2. **Edge cases** - Empty streams, zero-length objects
3. **Large files** - Performance with 100MB+ PDFs
4. **Concurrent access** - Thread safety
5. **Fuzz testing** - Random input validation

---

## Additional PDF Library Features

### Document Manipulation
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **PDF merging** | High | Medium | Combine multiple PDFs, handle conflicts |
| **PDF splitting** | High | Medium | Split by pages, bookmarks, or custom logic |
| **Page extraction** | High | Medium | Extract pages as new PDFs |
| **Page rotation** | Medium | Low | Rotate individual or all pages |
| **Page deletion** | Medium | Medium | Remove pages, update references |
| **Page insertion** | Medium | Medium | Insert pages at specific positions |
| **Page reordering** | Medium | Medium | Reorder pages in document |
| **PDF comparison** | Low | High | Diff two PDFs, highlight differences |

### Content Extraction
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Text extraction** | High | Medium | Extract text with position, font, size info |
| **Image extraction** | Medium | Medium | Extract all embedded images |
| **Font extraction** | Low | Medium | Extract embedded fonts from PDF |
| **Metadata extraction** | Medium | Low | Document info, XMP, custom metadata |
| **Bookmark extraction** | Medium | Medium | Extract outline/bookmark structure |
| **Annotation extraction** | Medium | High | Links, comments, highlights, form fields |
| **Form data extraction** | High | High | ✅ Implemented - Extract AcroForm field values |

### Content Creation
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Advanced graphics** | Medium | High | Bezier curves, arcs, ellipses |
| **Gradients** | Medium | Medium | Linear and radial gradients |
| **Patterns** | Low | Medium | Tiling patterns, shading |
| **Transparency** | Medium | High | Alpha channels, blend modes |
| **Watermarks** | Medium | Medium | Text/image watermarks |
| **Annotations (write)** | High | High | Create links, comments, highlights |
| **Bookmarks (write)** | Medium | Medium | Create navigation structure |
| **Form fields (AcroForm)** | High | High | ⚠️ Partial - Parsing done, creation pending |
| **Actions** | Medium | Medium | GoTo, URI, JavaScript actions |

### Advanced Features
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **PDF optimization** | Medium | High | Remove unused objects, compress, linearize |
| **PDF repair** | Low | Very High | Fix corrupted PDFs, recover content |
| **Streaming parser** | Low | High | Parse large PDFs without full memory load |
| **Concurrent parsing** | Low | High | Parallel object parsing for performance |
| **PDF/A compliance** | Low | Very High | Generate PDF/A-1, PDF/A-2, PDF/A-3 |
| **PDF/X support** | Low | Very High | Generate PDF/X-1a, PDF/X-3, PDF/X-4 |
| **Accessibility (tagged PDF)** | Medium | High | Structure tree, alt text, reading order |
| **Layers/OCGs** | Low | High | Optional content groups, layer control |
| **Color management** | Medium | Medium | CMYK, Lab, ICC profiles, spot colors |
| **3D content** | Low | Very High | 3D annotations, U3D, PRC support |
| **Multimedia** | Low | Very High | Video, audio, rich media annotations |

### Form Handling
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **AcroForm support** | High | High | Full AcroForm parsing and creation |
| **Form field filling** | High | Medium | Fill AcroForm fields programmatically |
| **Form field validation** | Medium | Medium | Validate form field values |
| **Form flattening** | Medium | High | Convert form fields to static content |
| **Digital signatures (forms)** | Low | Very High | Sign form fields, signature fields |

### Font Features
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Advanced subsetting** | Low | High | Full TTF subsetting with table rebuilding |
| **Type 1 fonts** | Low | Medium | PostScript Type 1 font support |
| **CID fonts** | Low | High | CID-keyed fonts for CJK languages |
| **Font metrics** | Medium | Low | Get font metrics, character widths |
| **Font fallback** | Low | Medium | Automatic font substitution |

### Image Features
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **TIFF support** | Medium | Medium | TIFF image embedding |
| **GIF support** | Low | Low | GIF image embedding |
| **BMP support** | Low | Low | BMP image embedding |
| **WebP support** | Low | Medium | WebP image embedding |
| **Image compression** | Medium | Medium | Recompress images, quality control |
| **Image scaling** | Medium | Low | Scale images before embedding |

### Error Handling & Validation
| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Better error messages** | High | Medium | Detailed, actionable error messages |
| **Error recovery** | Medium | High | Graceful handling of corrupted PDFs |
| **PDF validation** | Medium | High | Validate PDF structure, compliance |
| **Warning system** | Medium | Low | Non-fatal warnings for issues |
| **Diagnostic mode** | Low | Medium | Detailed diagnostic information |

## Contribution Priority

### High Priority (Core Functionality)
1. Incremental updates parsing ✅
2. Font embedding ✅
3. Image embedding ✅
4. Page content streams ✅
5. AES-256 full support ✅
6. **Text extraction** - Extract text from PDFs
7. **PDF merging/splitting** - Basic document manipulation
8. **AcroForm support** - ✅ Parsing implemented, creation pending
9. **Form field filling** - ⚠️ Basic implementation, needs object replacement
10. **Page manipulation** - Rotate, delete, reorder pages

### Medium Priority (Usability)
1. Encryption on write
2. Cross-reference stream writing
3. Subform repetition in XFA
4. Better error messages
5. **Advanced graphics** - Curves, gradients, patterns
6. **Annotations** - Create links, comments, highlights
7. **Bookmarks** - Create navigation structure
8. **Metadata handling** - Read/write document metadata
9. **PDF optimization** - Remove unused objects, compress
10. **Accessibility** - Tagged PDF, structure tree

### Low Priority (Nice to Have)
1. Linearized PDF support
2. Digital signatures
3. Script execution
4. LZW and other legacy filters
5. **PDF/A compliance** - Generate compliant PDFs
6. **3D content** - 3D annotations support
7. **Multimedia** - Video/audio support
8. **PDF repair** - Fix corrupted PDFs
9. **Streaming parser** - Handle very large PDFs
10. **Color management** - Advanced color spaces

---

## How to Contribute

1. **Pick a gap** from this document
2. **Create an issue** describing your approach
3. **Write tests first** (TDD preferred)
4. **Implement** with clear comments
5. **Update GAPS.md** to mark as implemented
6. **Submit PR** with before/after examples

See [CONTRIBUTING.md](CONTRIBUTING.md) for code style guidelines.
