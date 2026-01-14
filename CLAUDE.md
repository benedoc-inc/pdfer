# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run a single test
go test -v -run TestE2E_CreateAndParseSimplePDF ./tests

# Run tests in a specific package
go test ./core/parse/...
go test ./forms/acroform/...

# Build the CLI tool
go build -o pdfer ./cmd/pdfer

# Run the CLI
./pdfer -input form.pdf -data data.json -output filled.pdf
./pdfer -input form.pdf -extract-schema -output schema.json
```

## Architecture

pdfer is a pure Go PDF library with zero external dependencies. The codebase follows a layered architecture:

### Core Layer (`core/`)
- **`parse/`** - PDF parsing with unified API via `parse.Open()` and `parse.OpenWithOptions()`. Handles incremental updates, cross-reference streams, object streams, and byte-perfect reconstruction
- **`write/`** - PDF generation via `PDFWriter` (low-level) and `SimplePDFBuilder` (high-level). Supports content streams, image embedding, compression
- **`encrypt/`** - Decryption (RC4 40/128-bit, AES 128/256-bit) using "parse-then-decrypt" strategy

### Forms Layer (`forms/`)
- **`forms.go`** - Unified `Form` interface that auto-detects AcroForm vs XFA
- **`acroform/`** - AcroForm parsing, filling, validation, appearance stream generation
- **`xfa/`** - XFA stream extraction, template parsing, datasets updating

### Content Layer (`content/extract/`)
- Extracts text, graphics, images, fonts, annotations, bookmarks, metadata
- `ExtractContent()` returns structured `ContentDocument`
- `ExtractContentToJSON()` for JSON serialization

### Resources (`resources/font/`)
- TrueType/OpenType font embedding with subsetting

### Types (`types/`)
- Shared data structures: `PDFEncryption`, `FormData`, `FormSchema`, `ContentDocument`

## Key Entry Points

```go
// Parse PDF
pdf, err := parse.Open(pdfBytes)
pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
    Password:    []byte("secret"),
    BytePerfect: true,  // For reconstruction
})

// Unified form handling (auto-detects AcroForm/XFA)
form, err := forms.Extract(pdfBytes, password, verbose)
filledPDF, err := form.Fill(pdfBytes, formData, password, verbose)

// Create PDF from scratch
builder := write.NewSimplePDFBuilder()
page := builder.AddPage(write.PageSizeLetter)
fontName := page.AddStandardFont("Helvetica")
page.Content().BeginText().SetFont(fontName, 12).ShowText("Hello").EndText()
builder.FinalizePage(page)
pdfBytes, _ := builder.Bytes()

// Extract content
doc, _ := extract.ExtractContent(pdfBytes, nil, false)
```

## Code Patterns

- PDF object references use format "N G R" (e.g., "5 0 R")
- Encryption info (`*types.PDFEncryption`) is passed through decryption pipeline
- Verbose flag enables debug logging throughout
- Stream data is typically FlateDecode compressed
- Tests use `tests/resources/` and `content/extract/tests/resources/` for test PDFs
