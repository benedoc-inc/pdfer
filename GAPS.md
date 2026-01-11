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
| Content Extraction | 10 | 0 | 1 |
| Advanced Features | 0 | 0 | 10+ |
| Form Handling | 5 | 0 | 1 |
| Font Features | 1 | 0 | 5 |
| Image Features | 6 | 0 | 2 |
| Error Handling | 0 | 0 | 5 |

---

## Encryption (`core/encrypt/`)

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
| Cross-reference tables | `core/parse/parser.go` | Traditional xref format |
| Cross-reference streams | `core/parse/xref_stream.go` | PDF 1.5+ compressed xref |
| Object streams (ObjStm) | `core/parse/object_stream.go` | Compressed object storage |
| PNG predictor filters | `core/parse/object_stream.go` | Predictors 10-15 |
| FlateDecode | Multiple | zlib/deflate decompression |
| Object retrieval | `core/parse/get_object.go` | Unified direct + ObjStm access |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Trailer parsing** | Works for most | Hybrid-reference files (xref table + stream) not fully supported |
| **Indirect object references** | Basic | Doesn't resolve nested references automatically |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **ASCIIHexDecode** | `core/parse/filters.go` | Encode and decode hex text |
| **ASCII85Decode** | `core/parse/filters.go` | Encode and decode base-85 |
| **RunLengthDecode** | `core/parse/filters.go` | Simple RLE compression |
| **DCTDecode** | `core/parse/filters.go` | JPEG image pass-through filter |
| **Incremental updates** | `core/parse/incremental.go` | Parse PDFs with multiple revisions, /Prev chain |
| **Byte-perfect parsing** | `core/parse/document.go`, `core/parse/document_parser.go` | Full PDF structure with raw bytes preserved |
| **Unified API** | `core/parse/api.go` | Clean `Open()`/`OpenWithOptions()` entry point with `PDF` type |

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

## PDF Writing (`core/write/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| Basic PDF structure | `core/write/writer.go` | Header, objects, xref, trailer |
| Stream objects | `core/write/writer.go` | With FlateDecode compression |
| XFA PDF building | `core/write/xfa_builder.go` | From XFA streams |
| PDF rebuilding | `core/write/xfa_builder.go` | Modify existing PDFs |
| Object encryption | `core/write/writer.go` | AES-128 for streams |
| **Image embedding** | `core/write/image.go` | JPEG (DCTDecode), PNG to RGB XObjects |
| **Page content streams** | `core/write/content.go` | Text, graphics, and image operators |
| **Page building** | `core/write/page.go` | SimplePDFBuilder for easy page creation |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Dictionary writing** | Basic | Complex nested structures may not format correctly |
| **String encryption** | Partial | Only stream data encrypted, not string objects in dictionaries |

### ✅ Newly Implemented

| Feature | File | Notes |
|---------|------|-------|
| **Font embedding** | `resources/font/font.go`, `resources/font/pdf.go` | TrueType/OpenType font embedding with subsetting support |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Encryption on write** | Medium | Medium | Generate new encrypted PDFs |
| **Cross-reference streams** | Medium | Medium | Write modern xref format |
| **Object streams** | Medium | Medium | Compress objects on write |
| **Digital signatures** | Low | Very High | PKCS#7, CMS signing |
| **Incremental save** | Medium | Medium | Append without rewriting |
| **Advanced graphics** | Medium | High | Curves (bezier), arcs, gradients, patterns |
| **Transparency/alpha** | Medium | High | Alpha channels, blend modes, soft masks |
| **Annotations (write)** | High | High | Links, comments, highlights, form fields |
| **Bookmarks/outlines (write)** | Medium | Medium | Create document navigation structure |
| **Watermarks** | Medium | Medium | Add text/image watermarks to pages |
| **Page manipulation** | High | Medium | Rotate, delete, reorder, insert pages |
| **PDF optimization** | Medium | High | Remove unused objects, compress streams |
| **Metadata (write)** | Medium | Low | Set document info, XMP metadata |
| **WebP support** | Low | Medium | WebP image embedding (requires external decoder) |
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

## XFA Processing (`forms/xfa/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| Template extraction | `forms/xfa/xfa.go` | Form structure |
| Datasets extraction | `forms/xfa/xfa.go` | Form data |
| Config extraction | `forms/xfa/xfa.go` | Rendering config |
| LocaleSet extraction | `forms/xfa/xfa.go` | Localization |
| Form field parsing | `forms/xfa/xfa_form_translator.go` | Text, numeric, choice, date |
| Validation rules | `forms/xfa/xfa_form_translator.go` | Required, patterns, ranges |
| Calculation rules | `forms/xfa/xfa_form_translator.go` | Basic field calculations |
| Field value updates | `forms/xfa/xfa.go` | Modify datasets |
| PDF rebuild | `forms/xfa/xfa.go` | With updated XFA |

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

### Content Extraction (`content/extract/`)

#### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| **Metadata extraction** | `content/extract/metadata.go` | Document info (title, author, dates, etc.) |
| **Page extraction** | `content/extract/pages.go` | Extract page structure, dimensions, rotation |
| **Bookmark extraction** | `content/extract/bookmarks.go` | Extract outline/bookmark hierarchy |
| **Form data extraction** | `forms/acroform/extract.go`, `forms/xfa/` | ✅ Implemented - Extract AcroForm and XFA field values |

#### ✅ Fully Implemented (Additional)

| Feature | File | Notes |
|---------|------|-------|
| **Text extraction** | `content/extract/content_stream.go` | Extract text with position, font, size, spacing, matrix |
| **Graphics extraction** | `content/extract/content_stream.go` | Extract rectangles, lines, colors, line width |
| **Image XObject extraction** | `content/extract/content_stream.go`, `content/extract/resources.go` | Extract image XObject references and metadata |
| **Font extraction** | `content/extract/resources.go` | Extract font dictionaries, subtypes, embedded status |
| **Resource extraction** | `content/extract/resources.go` | Extract fonts, XObjects, images from Resources |
| **Annotation extraction** | `content/extract/annotations.go` | Extract links, text annotations, markup annotations |

#### ✅ Fully Implemented (Additional)

| Feature | File | Notes |
|---------|------|-------|
| **Image binary data extraction** | `content/extract/images.go` | Extract actual image binary data (JPEG bytes for DCTDecode, raw pixel data for FlateDecode) |

#### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Font binary data extraction** | Low | Medium | Extract embedded font file data |

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
| **Form fields (AcroForm)** | High | High | ✅ Implemented - Full parsing and creation |
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

## Form Handling (`forms/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| **Unified form interface** | `forms/forms.go` | Auto-detect and work with AcroForm or XFA forms |
| **AcroForm parsing** | `forms/acroform/parser.go` | Extract AcroForm structure and fields recursively |
| **Form field extraction** | `forms/acroform/extract.go` | Extract field values and convert to FormSchema |
| **Form field filling** | `forms/acroform/fill_streams.go`, `forms/acroform/replace.go` | Fill AcroForm fields with values, handles direct objects and object streams |
| **Form field creation** | `forms/acroform/writer.go` | Create AcroForm fields programmatically (text, checkbox, radio, choice, button) |
| **Form field validation** | `forms/acroform/validation.go` | Validate field values against constraints (required, max length, patterns, ranges) |
| **Appearance streams** | `forms/acroform/appearance.go` | Generate appearance streams for checkboxes, text fields, buttons |
| **Field actions** | `forms/acroform/actions.go` | Add actions to fields (URI, JavaScript, GoTo, Submit, Reset) |
| **Form flattening** | `forms/acroform/flatten.go` | Convert form fields to static content (removes interactivity) |
| **Object stream support** | `forms/acroform/stream_rebuild.go`, `forms/acroform/stream_finder.go` | Handle form fields within compressed object streams |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Digital signatures (forms)** | Low | Very High | Sign form fields, signature fields |

### Font Features (`resources/font/`)

#### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| **Font embedding** | `resources/font/font.go`, `resources/font/pdf.go` | TrueType/OpenType font embedding with subsetting support |

#### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Advanced subsetting** | Low | High | Full TTF subsetting with table rebuilding |
| **Type 1 fonts** | Low | Medium | PostScript Type 1 font support |
| **CID fonts** | Low | High | CID-keyed fonts for CJK languages |
| **Font metrics** | Medium | Low | Get font metrics, character widths |
| **Font fallback** | Low | Medium | Automatic font substitution |

### Image Features (`core/write/image.go`)

#### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| **JPEG embedding** | `core/write/image.go` | JPEG images with DCTDecode filter (direct embedding, no re-encoding) |
| **PNG embedding** | `core/write/image.go` | PNG images converted to RGB/Gray with FlateDecode compression |
| **Generic image support** | `core/write/image.go` | Any format supported by Go's image package (PNG, GIF, BMP, etc.) |
| **Alpha channel support** | `core/write/image.go` | Soft masks for images with transparency |
| **Color space support** | `core/write/image.go` | RGB, Gray, CMYK (from JPEG) |
| **Image drawing** | `core/write/content.go` | DrawImage, DrawImageAt operators |

#### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **WebP support** | Low | Medium | WebP image embedding (requires external decoder) |
| **Image compression** | Medium | Medium | Recompress images, quality control |
| **Image scaling** | Medium | Low | Scale images before embedding (currently done via DrawImageAt) |

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
8. **AcroForm support** - ✅ Complete - Parsing, creation, validation, filling, flattening
9. **Form field filling** - ✅ Implemented - Basic filling with object stream structure
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
