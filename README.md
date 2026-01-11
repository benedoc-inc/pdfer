# pdfer

A pure Go library for PDF processing with comprehensive XFA (XML Forms Architecture) support.

[![Go Reference](https://pkg.go.dev/badge/github.com/benedoc-inc/pdfer.svg)](https://pkg.go.dev/github.com/benedoc-inc/pdfer)
[![Go Report Card](https://goreportcard.com/badge/github.com/benedoc-inc/pdfer)](https://goreportcard.com/report/github.com/benedoc-inc/pdfer)

## Features

- **Pure Go** - No CGO, no external dependencies
- **PDF Decryption** - RC4 (40/128-bit) and AES (128/256-bit)
- **XFA Processing** - Extract, parse, modify, and rebuild XFA forms
- **PDF Generation** - Create PDFs from scratch or modify existing ones
- **Object Streams** - Full support for compressed object storage
- **Cross-Reference Streams** - Parse modern PDF xref streams with predictor filters

## Installation

```bash
go get github.com/benedoc-inc/pdfer
```

## Quick Start

### Extract XFA from an Encrypted PDF

```go
package main

import (
    "log"
    "os"

    "github.com/benedoc-inc/pdfer"
    "github.com/benedoc-inc/pdfer/encryption"
    "github.com/benedoc-inc/pdfer/xfa"
)

func main() {
    // Read PDF
    pdfBytes, _ := os.ReadFile("form.pdf")

    // Decrypt (empty password for many government forms)
    _, encryptInfo, err := encryption.DecryptPDF(pdfBytes, []byte(""), false)
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
import "github.com/benedoc-inc/pdfer/writer"

// Create PDF writer
w := writer.NewPDFWriter()

// Add objects
catalogNum := w.AddObject([]byte("<</Type/Catalog/Pages 2 0 R>>"))
pagesNum := w.AddObject([]byte("<</Type/Pages/Kids[]/Count 0>>"))
w.SetRoot(catalogNum)

// Generate PDF bytes
pdfBytes, err := w.Bytes()
```

### Build XFA PDF from XML Streams

```go
builder := writer.NewXFABuilder(false)

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
├── encryption/      # PDF decryption (RC4, AES)
├── parser/          # Low-level PDF parsing
├── types/           # Core data structures
├── writer/          # PDF generation
├── xfa/             # XFA form processing
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

## Supported PDF Features

### Encryption
| Feature | Status |
|---------|--------|
| RC4 40-bit (V1) | ✅ |
| RC4 128-bit (V2) | ✅ |
| AES-128 (V4) | ✅ |
| AES-256 (V5) | ⚠️ Partial |
| User password | ✅ |
| Owner password | ✅ |

### PDF Structure
| Feature | Status |
|---------|--------|
| Cross-reference tables | ✅ |
| Cross-reference streams | ✅ |
| Object streams (ObjStm) | ✅ |
| FlateDecode filter | ✅ |
| PNG predictor filters | ✅ |
| Incremental updates | ❌ |
| Linearized PDFs | ❌ |

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
- [ ] **Incremental updates** - Parse PDFs with multiple revisions
- [ ] **Font embedding** - TrueType/OpenType font subsetting
- [ ] **Image embedding** - JPEG, PNG image objects
- [ ] **Page content streams** - Text and graphics operators
- [ ] **AES-256 full support** - Complete V5/R6 encryption

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
