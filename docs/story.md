# User Story: Text Extraction from 510(k) Summary PDFs

## Story
**As a** data analyst working with FDA 510(k) summary documents  
**I want to** extract readable text content from PDF files  
**So that** I can analyze device descriptions, indications for use, and regulatory information programmatically

## Current Behavior

### What Works
The `inspect_510k_summaries` script successfully:
- ✅ Parses PDF structure (version, encryption status, object count)
- ✅ Detects XFA forms and AcroForm presence
- ✅ Identifies PDF metadata
- ✅ Extracts some text content (but it's not readable)

### What's Broken
The text extraction returns binary/encoded data instead of readable text.

### Precise Output Example

**Input:** `K162922_summary_1.pdf` (95,235 bytes, PDF 1.1, unencrypted)

**Output:**
```json
{
  "file_name": "K162922_summary_1.pdf",
  "pdf_version": "1.1 ",
  "is_encrypted": false,
  "has_xfa": false,
  "structure": {
    "has_acro_form": false,
    "has_xfa": false,
    "has_metadata": false
  },
  "text_preview": "p^7]^%'\u0017\u0001V!ZZׄSlqPO\u0015\u0011_:ַKʿU\u001fU\no֨]8B\u001fV}\u001c/UN\u0010cMUL\u001f\u000bkj?fb֔\u007fm\u001f&ASLh\u0018T\u0004[R\nhB~ﰽpz\u0003\b0M\u0010F\b3tPӆ\u0013ӆΩ}\u0007aSiXJ頕}R\u0007+0\u0019\u00190G@\f\u0016A",
  "text_content_length": 3361
}
```

**Another Example:** `K161085_summary_1.pdf`
```json
{
  "file_name": "K161085_summary_1.pdf",
  "text_preview": "X\u0007\\룳[sBy6`\u000ebrf\",\u0007~\u0006\u000b<eo\u0000d{-5p\u0012H#\u00165c~\u0014O=v>Y\u00146D!\nPhSb\u0006<#6}<HzGvXݶ孮❇'Q\rZSѧJLNz2kE4¥\u0001zH}6HFS,MD=ԙ>2؝+\u0006\u0007#\u001cc\u0006z\u001agOowXwOi`х\u00035V-6|`bz>qB\\=(H1=F@\u0013\u0005jq\u001d.90#z&W\u0014`/z<Gz\u0006\u0006!fO14kKE5.\u001b\u000b$"
}
```

## Root Cause Analysis

### Technical Issue
The current text extraction implementation (`extractTextContent` function) uses a naive approach:

1. **Pattern Matching**: Searches for `(text) Tj` patterns in raw PDF bytes
2. **No Decompression**: PDFs use compressed content streams (FlateDecode, LZW, etc.) that must be decompressed first
3. **No Font Decoding**: Even decompressed text requires font encoding/CMap decoding to convert character codes to Unicode
4. **Binary Data Extraction**: The function is extracting binary/encoded stream data instead of actual text content

### Current Implementation
```go
// extractTextContent attempts to extract text from PDF content streams
func extractTextContent(pdfBytes []byte, verbose bool) (string, error) {
    // Looks for text between parentheses: (text) Tj
    // Pattern: (text content) Tj
    // This approach fails because:
    // 1. Content streams are compressed (FlateDecode, etc.)
    // 2. Text requires font encoding/CMap decoding
    // 3. Binary data is being extracted instead of readable text
}
```

## Expected Behavior

### Success Criteria
When extracting text from a 510(k) summary PDF, the output should contain:
- ✅ Readable English text
- ✅ Device names and descriptions
- ✅ Indications for use
- ✅ Regulatory information
- ✅ Proper Unicode character encoding

### Example Expected Output
```json
{
  "text_preview": "510(k) SUMMARY\n\nDevice Name: Example Medical Device\n\nIndications for Use:\nThe Example Medical Device is indicated for...\n\nDevice Description:\nThe device consists of...",
  "text_content": "[full readable text content]"
}
```

## Solution Requirements

### Technical Approach
To properly extract text from PDFs, we need to:

1. **Parse Content Streams**
   - Identify page content streams
   - Decompress using appropriate filters (FlateDecode, LZW, ASCIIHexDecode, etc.)
   - Handle stream dictionaries and `/Filter` entries

2. **Decode Text Operators**
   - Parse PDF text operators: `Tj`, `TJ`, `'`, `"`
   - Extract text strings from operands
   - Handle text positioning and formatting

3. **Font Decoding**
   - Identify fonts used in each text block
   - Apply font encoding (StandardEncoding, WinAnsiEncoding, custom encodings)
   - Use CMap/ToUnicode for proper Unicode conversion
   - Handle CID fonts and composite fonts

4. **Text Reconstruction**
   - Combine text fragments in reading order
   - Handle text positioning (Td, Tm operators)
   - Preserve paragraph structure where possible

### Implementation Options

**Option 1: Use pdfer's Content Extraction (if available)**
- Check if pdfer v0.8.0 has content stream parsing capabilities
- Leverage existing PDF parsing infrastructure

**Option 2: Integrate Specialized PDF Text Extraction Library**
- Consider libraries like `github.com/gen2brain/go-fitz` (MuPDF bindings)
- Or `github.com/ledongthuc/pdf` for Go-native extraction
- Or use external tools like `pdftotext` (poppler-utils)

**Option 3: Implement Full Content Stream Parser**
- Build on top of pdfer's stream extraction
- Add decompression, text operator parsing, and font decoding
- Most complex but most control

## Acceptance Criteria

- [ ] Text extraction returns readable English text (not binary data)
- [ ] Device names and descriptions are extractable
- [ ] Indications for use sections are readable
- [ ] Text preview shows actual content (not encoded characters)
- [ ] Works for multiple 510(k) summary PDF samples
- [ ] Handles compressed content streams (FlateDecode)
- [ ] Properly decodes font encodings to Unicode

## Related Files
- `/backend/src/scripts/inspect_510k_summaries/main.go` - Current implementation
- `/data/raw_data/fda/summaries/summaries/` - Sample PDF files

## Notes
- Current text extraction finds 3,361 characters but they're all binary/encoded
- PDFs are not encrypted, so decryption is not the issue
- The problem is specifically with content stream decompression and text decoding
- This is a common challenge with PDF text extraction - requires proper stream parsing
