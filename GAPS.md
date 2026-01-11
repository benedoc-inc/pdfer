# Implementation Gaps

This document details the current implementation gaps in pdfer and provides guidance for contributors.

## Summary

| Category | Implemented | Partial | Not Implemented |
|----------|-------------|---------|-----------------|
| Encryption | 3 | 1 | 1 |
| PDF Parsing | 5 | 2 | 4 |
| PDF Writing | 3 | 2 | 5 |
| XFA | 6 | 2 | 4 |

---

## Encryption (`encryption/`)

### ✅ Fully Implemented

| Feature | File | Notes |
|---------|------|-------|
| RC4 40-bit (V1, R2) | `decrypt.go` | Standard handler |
| RC4 128-bit (V2, R3) | `decrypt.go` | Standard handler |
| AES-128 CBC (V4, R4) | `decrypt.go` | With IV prefix handling |
| Password verification | `key_derivation.go` | User and owner passwords |
| Key derivation | `key_derivation.go` | Algorithm 2 per ISO 32000 |

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **AES-256 (V5, R5/R6)** | Partial | Key derivation uses V4 algorithm; needs SHA-256 based key derivation per ISO 32000-2 |

**To implement AES-256:**
```go
// In key_derivation.go, add:
func DeriveEncryptionKeyV5(password []byte, encrypt *PDFEncryption) ([]byte, error) {
    // 1. Compute hash using SHA-256 (not MD5)
    // 2. Use different algorithm per ISO 32000-2 section 7.6.4.3.3
    // 3. Handle /UE and /OE entries for key unwrapping
}
```

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

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Incremental updates** | High | Medium | Multiple xref sections, %%EOF markers |
| **Linearized PDFs** | Medium | High | First-page optimization, hint tables |
| **LZWDecode filter** | Low | Medium | Legacy compression, rarely used now |
| **ASCII85Decode** | Low | Low | Text encoding filter |
| **ASCIIHexDecode** | Low | Low | Hex text encoding |
| **RunLengthDecode** | Low | Low | Simple RLE compression |
| **CCITTFaxDecode** | Low | High | Fax image compression |
| **JBIG2Decode** | Low | High | Bi-level image compression |
| **JPXDecode** | Low | High | JPEG 2000 |
| **DCTDecode** | Medium | Medium | JPEG (for image extraction) |

**To implement incremental updates:**
```go
// In parser.go, add:
func ParseAllXRefSections(pdfBytes []byte) ([]*XRefSection, error) {
    // 1. Find all %%EOF markers
    // 2. Parse xref section before each
    // 3. Chain /Prev references
    // 4. Merge into final object map (later updates override earlier)
}
```

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

### ⚠️ Partial Implementation

| Feature | Status | What's Missing |
|---------|--------|----------------|
| **Dictionary writing** | Basic | Complex nested structures may not format correctly |
| **String encryption** | Partial | Only stream data encrypted, not string objects in dictionaries |

### ❌ Not Implemented

| Feature | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| **Font embedding** | High | High | TrueType/OpenType subsetting |
| **Image embedding** | High | Medium | JPEG, PNG to PDF image XObjects |
| **Page content streams** | High | High | Text operators, graphics state |
| **Encryption on write** | Medium | Medium | Generate new encrypted PDFs |
| **Cross-reference streams** | Medium | Medium | Write modern xref format |
| **Object streams** | Medium | Medium | Compress objects on write |
| **Digital signatures** | Low | Very High | PKCS#7, CMS signing |
| **Incremental save** | Medium | Medium | Append without rewriting |

**To implement font embedding:**
```go
// Create new package: font/
type Font struct {
    Name     string
    Subtype  string  // TrueType, Type1, etc.
    Data     []byte  // Raw font file
    Subset   []rune  // Characters to include
}

func (f *Font) ToXObjects() (fontDict, fontFile *PDFObject) {
    // 1. Parse font file (TTF/OTF)
    // 2. Subset to used glyphs
    // 3. Create /FontDescriptor
    // 4. Create font stream object
    // 5. Create /ToUnicode CMap
}
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
| `Font` | Font resource | High |
| `Image` | Image XObject | High |
| `Annotation` | Form fields, links | Medium |
| `Outline` | Bookmarks | Low |

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

## Contribution Priority

### High Priority (Core Functionality)
1. Incremental updates parsing
2. Font embedding
3. Image embedding  
4. Page content streams
5. AES-256 full support

### Medium Priority (Usability)
1. Encryption on write
2. Cross-reference stream writing
3. Subform repetition in XFA
4. Better error messages

### Low Priority (Nice to Have)
1. Linearized PDF support
2. Digital signatures
3. Script execution
4. LZW and other legacy filters

---

## How to Contribute

1. **Pick a gap** from this document
2. **Create an issue** describing your approach
3. **Write tests first** (TDD preferred)
4. **Implement** with clear comments
5. **Update GAPS.md** to mark as implemented
6. **Submit PR** with before/after examples

See [CONTRIBUTING.md](CONTRIBUTING.md) for code style guidelines.
