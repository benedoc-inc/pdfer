# pdfer

A pure Go library for PDF processing with comprehensive XFA (XML Forms Architecture) support.

[![Go Reference](https://pkg.go.dev/badge/github.com/benedoc-inc/pdfer.svg)](https://pkg.go.dev/github.com/benedoc-inc/pdfer)
[![Go Report Card](https://goreportcard.com/badge/github.com/benedoc-inc/pdfer)](https://goreportcard.com/report/github.com/benedoc-inc/pdfer)

## Features

- **Pure Go** - No CGO, no external dependencies
- **Unified API** - Clean `parse.Open()` entry point for all PDF operations
- **Unified Forms** - Auto-detect and work with AcroForm or XFA forms
- **PDF Decryption** - RC4 (40/128-bit) and AES (128/256-bit)
- **XFA Processing** - Extract, parse, modify, and rebuild XFA forms
- **PDF Generation** - Create PDFs from scratch with text, graphics, and images
- **Image Embedding** - JPEG and PNG images with alpha channel support
- **Content Streams** - Full text and graphics operators for page content
- **Object Streams** - Full support for compressed object storage
- **Cross-Reference Streams** - Parse modern PDF xref streams with predictor filters
- **Stream Filters** - FlateDecode, ASCIIHexDecode, ASCII85Decode, RunLengthDecode
- **Incremental Updates** - Parse PDFs with multiple revisions, follow /Prev chains
- **Byte-Perfect Parsing** - Preserve exact bytes for reconstruction of original PDF
- **Structured Error Handling** - Categorized error types with codes, context, and standard library compatibility
- **Warning System** - Collect and manage non-fatal warnings during PDF processing

## Installation

```bash
go get github.com/benedoc-inc/pdfer
```

## Quick Start

### Open and Parse a PDF (Unified API)

```go
import "github.com/benedoc-inc/pdfer/core/parse"

// Open a PDF
pdf, err := parse.Open(pdfBytes)
if err != nil {
    log.Fatal(err)
}

// Get basic info
log.Printf("Version: %s", pdf.Version())
log.Printf("Objects: %d", pdf.ObjectCount())
log.Printf("Revisions: %d", pdf.RevisionCount())

// Get an object
obj, err := pdf.GetObject(1)
if err != nil {
    log.Fatal(err)
}
log.Printf("Object 1: %s", string(obj))

// List all objects
for _, num := range pdf.Objects() {
    log.Printf("Object %d exists", num)
}
```

### Open an Encrypted PDF

```go
pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
    Password: []byte("secret"),
    Verbose:  true,
})
```

### Byte-Perfect PDF Parsing

```go
// Parse preserving exact bytes for reconstruction
pdf, err := parser.OpenWithOptions(pdfBytes, parser.ParseOptions{
    BytePerfect: true,
})

// Reconstruct identical PDF
reconstructed := pdf.Bytes()
// bytes.Equal(reconstructed, pdfBytes) == true

// Access raw object data with byte offsets
rawObj, _ := pdf.GetRawObject(1)
log.Printf("Object at offset %d, raw bytes: %d", rawObj.Offset, len(rawObj.RawBytes))
```

### Extract and Fill Forms (Unified Interface)

```go
import "github.com/benedoc-inc/pdfer/forms"

// Auto-detect and extract any form type (AcroForm or XFA)
form, err := forms.Extract(pdfBytes, password, false)
if err != nil {
    log.Fatal(err)
}

// Work with unified interface
schema := form.Schema()
log.Printf("Form type: %s, Fields: %d", form.Type(), len(schema.Questions))

// Fill the form
formData := types.FormData{
    "FirstName": "John",
    "LastName":  "Doe",
}
filled, err := form.Fill(pdfBytes, formData, password, false)
```

### Extract XFA from an Encrypted PDF

```go
package main

import (
    "log"
    "os"

    "github.com/benedoc-inc/pdfer"
    "github.com/benedoc-inc/pdfer/core/encrypt"
    "github.com/benedoc-inc/pdfer/forms/xfa"
)

func main() {
    // Read PDF
    pdfBytes, _ := os.ReadFile("form.pdf")

    // Decrypt (empty password for many government forms)
    _, encryptInfo, err := encrypt.DecryptPDF(pdfBytes, []byte(""), false)
    if err != nil {
        log.Fatal(err)
    }

    // Extract XFA streams
    streams, err := xfa.ExtractAllXFAStreams(pdfBytes, encryptInfo, false)
    if err != nil {
        log.Fatal(err)
    }

    // Access template (form structure)
    log.Printf("Template: %d bytes", len(streams.Template.Data))
    
    // Access datasets (form data)
    log.Printf("Datasets: %d bytes", len(streams.Datasets.Data))
    
    // Use convenient type aliases from root package
    var form *pdfer.FormSchema
    form, _ = xfa.ParseXFAForm(string(streams.Template.Data), false)
    log.Printf("Found %d questions", len(form.Questions))
}
```

### Parse XFA Form Structure

```go
// Parse the template to get form fields
form, err := xfa.ParseXFAForm(string(streams.Template.Data), false)
if err != nil {
    log.Fatal(err)
}

log.Printf("Found %d questions", len(form.Questions))
for _, q := range form.Questions {
    log.Printf("  %s: %s (%s)", q.ID, q.Label, q.Type)
}
```

### Update Form Field Values

```go
// Create form data (using type alias from root package)
formData := pdfer.FormData{
    "FirstName": "John",
    "LastName":  "Doe",
    "Date":      "2024-01-15",
}

// Update XFA in PDF
updatedPDF, err := xfa.UpdateXFAInPDF(pdfBytes, formData, encryptInfo, false)
if err != nil {
    log.Fatal(err)
}

os.WriteFile("filled.pdf", updatedPDF, 0644)
```

### Create a PDF from Scratch

```go
import "github.com/benedoc-inc/pdfer/core/write"

// Create a simple PDF with text and graphics
builder := write.NewSimplePDFBuilder()

// Add a page
page := builder.AddPage(writer.PageSizeLetter)

// Add a font and get its resource name
fontName := page.AddStandardFont("Helvetica")

// Draw content
page.Content().
    // Add text
    BeginText().
    SetFont(fontName, 24).
    SetTextPosition(72, 720).
    ShowText("Hello, PDF World!").
    EndText().
    // Draw a red rectangle
    SetFillColorRGB(1, 0, 0).
    Rectangle(72, 650, 200, 50).
    Fill()

builder.FinalizePage(page)

// Generate PDF bytes
pdfBytes, err := builder.Bytes()
```

### Embed Images in a PDF

```go
import "github.com/benedoc-inc/pdfer/core/write"

builder := write.NewSimplePDFBuilder()
page := builder.AddPage(write.PageSizeLetter)

// Add a JPEG image
jpegData, _ := os.ReadFile("photo.jpg")
imgInfo, err := builder.Writer().AddJPEGImage(jpegData, "Im1")
if err != nil {
    log.Fatal(err)
}

// Register image with page and draw it
imgName := page.AddImage(imgInfo)
page.Content().DrawImageAt(imgName, 72, 500, 200, 150)

builder.FinalizePage(page)
pdfBytes, _ := builder.Bytes()
```

### Extract All Content from a PDF

```go
import "github.com/benedoc-inc/pdfer/content/extract"

// Extract all content (text, graphics, images, fonts, annotations)
doc, err := extract.ExtractContent(pdfBytes, nil, false)
if err != nil {
    log.Fatal(err)
}

// Access extracted content
log.Printf("Pages: %d", len(doc.Pages))
for _, page := range doc.Pages {
    log.Printf("  Page %d: %d text elements, %d graphics, %d images",
        page.PageNumber, len(page.Text), len(page.Graphics), len(page.Images))
    
    // Text with positioning and font info
    for _, text := range page.Text {
        log.Printf("    Text: '%s' at (%.2f, %.2f) font: %s size: %.2f",
            text.Text, text.X, text.Y, text.FontName, text.FontSize)
    }
    
    // Resources (fonts, images)
    if page.Resources != nil {
        log.Printf("    Fonts: %d, Images: %d", 
            len(page.Resources.Fonts), len(page.Resources.Images))
    }
    
    // Annotations
    for _, annot := range page.Annotations {
        log.Printf("    Annotation: %s", annot.Type)
    }
}

// Extract as JSON
jsonStr, err := extract.ExtractContentToJSON(pdfBytes, nil, false)
if err != nil {
    log.Fatal(err)
}
log.Printf("JSON: %s", jsonStr)

// Extract all images with binary data
images, err := extract.ExtractAllImages(pdfBytes, nil, false)
if err != nil {
    log.Fatal(err)
}
for _, img := range images {
    log.Printf("Image: %s, %dx%d, Format: %s, Data: %d bytes", 
        img.ID, img.Width, img.Height, img.Format, len(img.Data))
    // Image binary data is in img.Data (and img.DataBase64 for JSON)
}
```

**Extraction Flow:**
```
ExtractContent()
  ├─→ ExtractMetadata() → Document info (title, author, dates)
  ├─→ ExtractPages() → For each page:
  │     ├─→ parseContentStream() → Text, graphics, image refs
  │     ├─→ extractResources() → Fonts, XObjects, images
  │     └─→ extractAnnotations() → Links, comments, highlights
  ├─→ ExtractBookmarks() → Document outline
  └─→ Aggregate → Unique fonts/images from all pages
```

### Parse PDFs with Incremental Updates

```go
import "github.com/benedoc-inc/pdfer/core/parse"

// Parse a PDF that has been edited multiple times
pdfBytes, _ := os.ReadFile("edited.pdf")

// Check how many revisions the PDF has
revisions := parse.CountRevisions(pdfBytes)
log.Printf("PDF has %d revisions", revisions)

// Parse all revisions and merge object tables
result, err := parser.ParseWithIncrementalUpdates(pdfBytes, false)
if err != nil {
    log.Fatal(err)
}

log.Printf("Found %d objects", len(result.Objects))

// Extract a specific revision (e.g., the original before edits)
originalPDF, _ := parse.ExtractRevision(pdfBytes, 1)
```

### Byte-Perfect PDF Parsing

```go
import "github.com/benedoc-inc/pdfer/core/parse"

// Parse PDF with full byte preservation
pdfBytes, _ := os.ReadFile("document.pdf")
doc, err := parser.ParsePDFDocument(pdfBytes)
if err != nil {
    log.Fatal(err)
}

// Access individual revisions and objects
log.Printf("PDF has %d revisions, %d objects", doc.RevisionCount(), doc.ObjectCount())

// Get raw bytes of any object
obj := doc.GetObject(1)
log.Printf("Object 1: %d bytes", len(obj.RawBytes))

// Stream objects have parsed components
if obj.IsStream {
    log.Printf("Dictionary: %s", string(obj.DictRaw))
    log.Printf("Stream data: %d bytes", len(obj.StreamRaw))
}

// Reconstruct the PDF (byte-identical to original)
reconstructed := doc.Bytes()
// reconstructed == pdfBytes
```

### Build XFA PDF from XML Streams

```go
builder := write.NewXFABuilder(false)

streams := []writer.XFAStreamData{
    {Name: "template", Data: templateXML, Compress: true},
    {Name: "datasets", Data: datasetsXML, Compress: true},
    {Name: "config", Data: configXML, Compress: true},
}

pdfBytes, err := builder.BuildFromXFA(streams)
```

## Package Structure

```
github.com/benedoc-inc/pdfer/
├── pdfer.go         # Root package with type aliases
├── core/            # Foundation layer
│   ├── parse/       # PDF parsing (reading structure)
│   ├── write/       # PDF writing (creating/modifying)
│   └── encrypt/     # Encryption/decryption
├── forms/           # Form processing (unified domain)
│   ├── forms.go     # Unified form interface
│   ├── acroform/    # AcroForm implementation
│   └── xfa/         # XFA implementation
├── content/         # Content operations
│   └── extract/     # Content extraction
├── resources/       # Embeddable resources
│   └── font/        # Font embedding
├── types/           # Shared data structures
├── cmd/pdfer/       # CLI tool
└── examples/        # Usage examples
```

## Type Aliases

For convenience, common types are re-exported from the root package:

```go
import "github.com/benedoc-inc/pdfer"

var enc *pdfer.Encryption      // = types.PDFEncryption
var form *pdfer.FormSchema     // = types.FormSchema
var q pdfer.Question           // = types.Question
var data pdfer.FormData        // = types.FormData
```

## Document Manipulation

The library provides comprehensive document manipulation capabilities:

```go
import "github.com/benedoc-inc/pdfer/core/manipulate"

// Rotate pages
manipulator, _ := manipulate.NewPDFManipulator(pdfBytes, nil, false)
manipulator.RotatePage(1, 90)  // Rotate page 1 by 90 degrees
manipulator.RotateAllPages(180) // Rotate all pages by 180 degrees
rotatedPDF, _ := manipulator.Rebuild()

// Delete pages
manipulator, _ := manipulate.NewPDFManipulator(pdfBytes, nil, false)
manipulator.DeletePage(2)  // Delete page 2
manipulator.DeletePages([]int{3, 5})  // Delete pages 3 and 5
modifiedPDF, _ := manipulator.Rebuild()

// Extract pages
extractedPDF, _ := manipulate.ExtractPages(pdfBytes, []int{1, 3, 5}, nil, false)

// Merge PDFs
mergedPDF, _ := manipulate.MergePDFs([][]byte{pdf1, pdf2, pdf3}, nil, false)

// Split PDF
ranges := []manipulate.PageRange{
    {Start: 1, End: 5},
    {Start: 6, End: 10},
}
splitPDFs, _ := manipulate.SplitPDF(pdfBytes, ranges, nil, false)

// Split by page count
splitPDFs, _ := manipulate.SplitPDFByPageCount(pdfBytes, 5, nil, false) // 5 pages per PDF
```

### Compare PDFs

```go
import "github.com/benedoc-inc/pdfer/core/compare"

// Compare two PDFs
result, err := compare.ComparePDFs(pdf1Bytes, pdf2Bytes, nil, nil, false)
if err != nil {
    log.Fatal(err)
}

if result.Identical {
    log.Println("PDFs are identical")
} else {
    log.Printf("Found %d differences", result.Summary.TotalDifferences)
    
    // Generate human-readable report
    report := compare.GenerateReport(result)
    log.Println(report)
    
    // Or get JSON report
    jsonReport, _ := compare.GenerateJSONReport(result)
    log.Println(jsonReport)
}

// Advanced comparison options with granularity and sensitivity control
opts := compare.CompareOptions{
    // Metadata options
    IgnoreProducer: true,  // Ignore Producer differences
    IgnoreDates:    true,  // Ignore date differences
    
    // Position tolerance
    TextTolerance:  5.0,   // Position tolerance for text matching (points)
    GraphicTolerance: 5.0,  // Position tolerance for graphics/images
    
    // Text comparison granularity
    TextGranularity: compare.GranularityElement, // element, word, or char
    DiffSensitivity: compare.SensitivityNormal,    // strict, normal, or relaxed
    
    // Move detection
    DetectMoves:    true,  // Detect when text/images move positions
    MoveTolerance: 50.0,  // Position tolerance for detecting moves
    
    // Filtering
    MinChangeThreshold: 0.0, // Minimum change percentage to report (0.0 = all)
    IgnoreWhitespace: false, // Ignore whitespace differences
    IgnoreCase:       false, // Case-insensitive comparison
}
result, _ := compare.ComparePDFsWithOptions(pdf1Bytes, pdf2Bytes, nil, nil, opts)
```

**Comparison Features:**
- **Best-in-class diffing algorithm**: Uses LCS (Longest Common Subsequence) for optimal matching
- **Multi-phase matching**: Exact matches → Position-based modifications → Content-based matching
- **Configurable granularity**: Compare at element, word, or character level
- **Sensitivity control**: Strict, normal, or relaxed change detection
- **Move detection**: Identifies when content moves between positions
- **Image comparison**: Binary comparison of image data, position tracking, and move detection
- **Text extraction**: Full text with position, font, and size information
- **Comprehensive reports**: Human-readable and JSON output formats

## Error Handling

The library provides structured error handling with categorized error types:

```go
import "github.com/benedoc-inc/pdfer/types"
import "errors"

pdf, err := parse.Open(pdfBytes)
if err != nil {
    // Check for specific error types
    if errors.Is(err, types.ErrWrongPassword) {
        log.Println("Incorrect password")
    } else if errors.Is(err, types.ErrObjectNotFound) {
        log.Println("Object not found")
    }
    
    // Get error code
    if code, ok := types.GetErrorCode(err); ok {
        log.Printf("Error code: %s", code)
    }
    
    // Check error category
    if types.IsEncryptionError(err) {
        log.Println("Encryption-related error")
    } else if types.IsNotFound(err) {
        log.Println("Resource not found")
    }
    
    // Access structured error details
    if pdfErr, ok := types.IsPDFError(err); ok {
        log.Printf("Code: %s, Message: %s", pdfErr.Code, pdfErr.Message)
        if pdfErr.Context != nil {
            log.Printf("Context: %v", pdfErr.Context)
        }
    }
}
```

**Error Categories:**
- **Parsing errors**: Invalid PDF, malformed structure, object not found
- **Encryption errors**: Wrong password, decryption failed, unsupported crypto
- **Form errors**: No forms, invalid form, field not found, validation errors
- **Content errors**: Extraction errors, font errors, image errors
- **Write errors**: Write failures, invalid input
- **I/O errors**: File system errors

All errors implement `errors.Is()` and `errors.Unwrap()` for compatibility with the standard library.

### Warning System

The library provides a warning system for collecting non-fatal issues during PDF processing:

```go
import "github.com/benedoc-inc/pdfer/types"
import "github.com/benedoc-inc/pdfer/core/parse"

// Create a warning collector
warnings := types.NewWarningCollector(true)

// Use it in ParseOptions
pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
    Warnings: warnings,
})

// Check for warnings after processing
if warnings.HasWarnings() {
    log.Printf("Found %d warnings", warnings.Count())
    
    // Get all warnings
    for _, warning := range warnings.Warnings() {
        log.Printf("[%s] %s", warning.Level, warning.Message)
    }
    
    // Filter by level
    errors := warnings.FilterByLevel(types.WarningLevelError)
    for _, warning := range errors {
        log.Printf("Error-level warning: %s", warning.Message)
    }
    
    // Filter by code
    fontWarnings := warnings.GetByCode("MISSING_FONT")
    for _, warning := range fontWarnings {
        log.Printf("Font warning: %s", warning.Message)
    }
}
```

**Warning Levels:**
- `WarningLevelInfo` - Informational messages
- `WarningLevelWarning` - Non-critical issues
- `WarningLevelError` - Non-fatal errors that should be reported

**Warning Features:**
- Collect warnings during PDF operations
- Filter by level or code
- Add context to warnings
- Enable/disable collection
- Access warnings programmatically

## Supported PDF Features

### Encryption
| Feature | Status |
|---------|--------|
| RC4 40-bit (V1) | ✅ |
| RC4 128-bit (V2) | ✅ |
| AES-128 (V4) | ✅ |
| AES-256 (V5) | ✅ |
| User password | ✅ |
| Owner password | ✅ |

### PDF Structure
| Feature | Status |
|---------|--------|
| Cross-reference tables | ✅ |
| Cross-reference streams | ✅ |
| Object streams (ObjStm) | ✅ |
| FlateDecode filter | ✅ |
| ASCIIHexDecode filter | ✅ |
| ASCII85Decode filter | ✅ |
| RunLengthDecode filter | ✅ |
| PNG predictor filters | ✅ |
| Image embedding (JPEG/PNG) | ✅ |
| Page content streams | ✅ |
| Incremental updates | ✅ |
| Linearized PDFs | ❌ |

### Content Extraction
| Feature | Status |
|---------|--------|
| Text extraction | ✅ |
| Graphics extraction | ✅ |
| Image extraction | ✅ |
| Font extraction | ✅ |
| Annotation extraction | ✅ |
| Bookmark extraction | ✅ |
| Metadata extraction | ✅ |
| JSON serialization | ✅ |

### Document Manipulation
| Feature | Status |
|---------|--------|
| Page rotation | ✅ |
| Page deletion | ✅ |
| Page insertion | ✅ |
| Page extraction | ✅ |
| PDF merging | ✅ |
| PDF splitting | ✅ |
| PDF comparison | ✅ (Best-in-class LCS diffing algorithm) |

### XFA Forms
| Feature | Status |
|---------|--------|
| Template extraction | ✅ |
| Datasets extraction | ✅ |
| Config extraction | ✅ |
| LocaleSet extraction | ✅ |
| Form field parsing | ✅ |
| Validation rules | ✅ |
| Calculation rules | ✅ |
| Field value update | ✅ |
| PDF rebuild | ✅ |
| Dynamic XFA | ⚠️ Limited |

## Implementation Status

See [GAPS.md](GAPS.md) for detailed implementation status and contribution opportunities.

### High Priority Gaps
- [x] **Incremental updates** - Parse PDFs with multiple revisions ✅
- [x] **Font embedding** - TrueType/OpenType font subsetting ✅
- [x] **Image embedding** - JPEG, PNG image objects ✅
- [x] **Page content streams** - Text and graphics operators ✅
- [x] **AES-256 full support** - Complete V5/R6 encryption ✅
- [x] **Structured error handling** - Categorized error types with codes and context ✅
- [x] **Warning system** - Non-fatal warning collection and management ✅

### Not Planned
- Dynamic XFA rendering (requires full layout engine)
- Script execution (FormCalc/JavaScript)
- Digital signatures (complex PKI requirements)

## Testing

```bash
go test ./...
```

Run with verbose output:
```bash
go test -v ./...
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Acknowledgments

This library's PDF parsing approach is inspired by [pypdf](https://github.com/py-pdf/pypdf), 
implementing the "parse-then-decrypt" strategy for handling encrypted PDFs with object streams.
