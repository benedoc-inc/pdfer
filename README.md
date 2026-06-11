# pdfer

Pure Go PDF processing library — zero CGO, zero external dependencies.

[![Go Reference](https://pkg.go.dev/badge/github.com/benedoc-inc/pdfer/v2.svg)](https://pkg.go.dev/github.com/benedoc-inc/pdfer/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/benedoc-inc/pdfer/v2)](https://goreportcard.com/report/github.com/benedoc-inc/pdfer/v2)

## Installation

```bash
go get github.com/benedoc-inc/pdfer/v2
```

## Quick start

```go
import "github.com/benedoc-inc/pdfer/v2"

// Merge two PDFs
out, err := pdfer.MergePDFs([][]byte{a, b}, nil, false)

// Split into page ranges
parts, err := pdfer.SplitPDF(pdfBytes, []pdfer.PageRange{{1, 3}, {4, 6}}, nil, false)

// Fill a form and flatten it
form, err := pdfer.ExtractForm(pdfBytes, nil, false)
filled, err := form.Fill(pdfBytes, pdfer.FormData{"name": "Alice"}, nil, false)
flat, err := pdfer.FlattenForm(filled, nil, false)
```

## API reference

All operations are available from the root `pdfer` package. Import sub-packages only for lower-level control.

### Encryption

```go
out, err := pdfer.EncryptPDF(pdfBytes, []byte("user-pw"), []byte("owner-pw"), false)
out, err := pdfer.DecryptPDF(pdfBytes, []byte("password"), false)
perms, err := pdfer.GetPermissions(pdfBytes, []byte("password"))
// perms.Print, perms.Modify, perms.Copy, perms.AddAnnotations, …
```

### Page operations

```go
// Extract, delete, reorder
out, err := pdfer.ExtractPages(pdfBytes, []int{1, 3, 5}, nil, false)
out, err := pdfer.DeletePage(pdfBytes, 2, nil, false)
out, err := pdfer.DeletePages(pdfBytes, []int{2, 4}, nil, false)
out, err := pdfer.ReorderPages(pdfBytes, []int{3, 1, 2}, nil, false)

// Insert and duplicate
out, err := pdfer.InsertBlankPage(pdfBytes, 2, 612, 792, nil, false) // position, width, height (pts)
out, err := pdfer.DuplicatePage(pdfBytes, 1, 2, nil, false)          // page, copies

// Geometry
out, err := pdfer.RotatePage(pdfBytes, 1, 90, nil, false)   // angle: 90, 180, or 270
out, err := pdfer.RotateAllPages(pdfBytes, 180, nil, false)
out, err := pdfer.CropPage(pdfBytes, 1, [4]float64{36, 36, 576, 756}, nil, false) // [llx lly urx ury]
out, err := pdfer.SetPageSize(pdfBytes, 1, 612, 792, nil, false)                  // width, height (pts)
```

### Document operations

```go
out, err := pdfer.MergePDFs([][]byte{a, b, c}, nil, false)
parts, err := pdfer.SplitPDF(pdfBytes, []pdfer.PageRange{{1, 3}, {4, 6}}, nil, false)
parts, err := pdfer.SplitPDFByPageCount(pdfBytes, 10, nil, false)
out, err := pdfer.Redact(pdfBytes, []pdfer.RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}, nil)
out, err := pdfer.Repair(pdfBytes, nil)
out, err := pdfer.Linearize(pdfBytes, nil) // Fast Web View

// Embed file attachments (PDF Portfolio / eCTD)
out, err := pdfer.EmbedAttachments(pdfBytes, []pdfer.FileAttachment{
    {Name: "report.xlsx", Data: xlsxBytes, MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
    {Name: "photo.jpg",   Data: jpegBytes, MimeType: "image/jpeg"},
})

// For encrypted PDFs, supply the user/owner password: appended streams are
// encrypted with the document's keys and the file /ID is carried forward.
out, err = pdfer.EmbedAttachmentsWithPassword(encryptedBytes, files, []byte("password"))

// List / extract embedded file attachments (inverse of EmbedAttachments)
attachments, err := pdfer.ListAttachments(pdfBytes)
for _, a := range attachments {
    fmt.Printf("%s (%d bytes, %s)\n", a.Name, len(a.Data), a.MimeType)
    _ = os.WriteFile(a.Name, a.Data, 0644)
}
// For encrypted PDFs: pdfer.ListAttachmentsWithPassword(pdfBytes, []byte("password"))
```

### Stamping

```go
out, err := pdfer.StampText(pdfBytes, 1, pdfer.TextStamp{
    Text: "CONFIDENTIAL", FontSize: 14, X: 72, Y: 720, R: 1,
}, nil, false)
out, err := pdfer.StampAllPages(pdfBytes, pdfer.TextStamp{Text: "DRAFT", X: 72, Y: 36}, nil, false)
out, err := pdfer.StampPageNumbers(pdfBytes, pdfer.PageNumberOptions{
    Position: pdfer.BottomCenter, FontSize: 10,
}, nil, false)
```

### Metadata

```go
meta, err := pdfer.GetMetadata(pdfBytes, nil, false)
// meta.Title, meta.Author, meta.CreationDate, meta.PageCount, …

out, err := pdfer.SetMetadata(pdfBytes, pdfer.MetadataUpdate{
    Title:  "Annual Report",
    Author: "Alice",
}, nil, false)

out, err := pdfer.RedactMetadata(pdfBytes, nil, false) // strips /Info and XMP
```

### Annotations

```go
// Link to URL
out, err := pdfer.AddAnnotation(pdfBytes, 1, pdfer.AnnotationConfig{
    Type: pdfer.AnnotLink,
    Rect: [4]float64{72, 700, 200, 720},
    URI:  "https://example.com",
}, nil, false)

// Internal page link
out, err := pdfer.AddAnnotation(pdfBytes, 1, pdfer.AnnotationConfig{
    Type:     pdfer.AnnotLink,
    Rect:     [4]float64{72, 680, 200, 700},
    DestPage: 3,
}, nil, false)

// Text note, highlight, free-text, underline, strikeout also supported
out, err := pdfer.AddAnnotation(pdfBytes, 1, pdfer.AnnotationConfig{
    Type:     pdfer.AnnotHighlight,
    Rect:     [4]float64{72, 650, 300, 665},
    Contents: "Important passage",
    Color:    [3]float64{1, 1, 0}, // yellow
}, nil, false)
```

### Bookmarks

```go
bmarks, err := pdfer.GetBookmarks(pdfBytes, nil, false)

out, err := pdfer.SetBookmarks(pdfBytes, []pdfer.BookmarkEntry{
    {Title: "Introduction", Page: 1},
    {Title: "Chapter 1", Page: 3, Children: []pdfer.BookmarkEntry{
        {Title: "Background", Page: 3},
        {Title: "Methods",    Page: 7},
    }},
    {Title: "Appendix", Page: 42},
}, nil, false)
```

### Digital signatures

```go
// Sign
out, err := pdfer.SignPDF(pdfBytes, pdfer.SignOptions{
    Certificate: cert,   // *x509.Certificate
    PrivateKey:  key,    // crypto.Signer
    Reason:      "Approved",
    Location:    "New York",
})

// Validate
sigs, err := pdfer.ValidateSignatures(pdfBytes)
for _, s := range sigs {
    fmt.Printf("%s: valid=%v signer=%s\n", s.FieldName, s.Valid, s.SignerName)
}
```

### Forms (AcroForm and XFA)

```go
// Auto-detect form type
kind, err := pdfer.DetectForm(pdfBytes, nil, false) // "acroform", "xfa", or "unknown"

// Extract and fill
form, err := pdfer.ExtractForm(pdfBytes, nil, false)
schema := form.Schema()
filled, err := form.Fill(pdfBytes, pdfer.FormData{"FirstName": "Alice"}, nil, false)

// Flatten (make non-interactive)
out, err := pdfer.FlattenForm(filled, nil, false)
```

For XFA forms, `schema.Scripts` exposes raw `<script>` blocks verbatim:

```go
for _, s := range schema.Scripts {
    fmt.Printf("[%s] %s (%s) on %s\n%s\n", s.Event, s.Name, s.Language, s.OwnerPath, s.Body)
}
```

`FormScript.Body` is the unmodified source. pdfer does not interpret
FormCalc or JavaScript semantics — callers that need to evaluate scripts
should plug in their own parser. `Question.Scripts` and `FormSection.Scripts`
hold `FormScript.ID` references in declaration order.

For dynamic XFA, `<occur>` and `<bind>` declarations are likewise surfaced
as data: `Occur` (declared cardinality, normalized to XFA spec defaults,
`Max: -1` = unbounded) and `Bind` (match mode plus the `ref` SOM path) on
`Question`, `FormSection`, and `FormElement`. pdfer hands the renderer what
the template said; deciding which subforms get an "add another" control and
materializing instances is the caller's job. See
[xfa-web](https://github.com/benedoc-inc/xfa-web) for one example of an
interactive renderer built on this contract.

### Content extraction

```go
// Full structured extraction
doc, err := pdfer.ExtractContent(pdfBytes, nil, false)
// doc.Pages[0].Text, doc.Pages[0].Images, doc.Pages[0].Annotations, doc.Bookmarks, …

json, err := pdfer.ExtractContentToJSON(pdfBytes, nil, false)

// Images only
imgs, err := pdfer.ExtractAllImages(pdfBytes, nil, false)
// imgs[0].Data (raw bytes), imgs[0].Width, imgs[0].Height, imgs[0].Format

// Dump everything to disk
out, err := pdfer.ExtractToDirectory(pdfBytes, nil, "/tmp/extracted", false)
```

### Comparison

```go
result, err := pdfer.ComparePDFs(pdf1, pdf2, nil, nil, false)
fmt.Println(pdfer.CompareReport(result))

// With options
opts := pdfer.DefaultCompareOptions()
opts.IgnoreDates = true
result, err := pdfer.ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts)
```

### Image replacement

```go
// Replace an image by resource name or object number
out, err := pdfer.ReplaceImage(pdfBytes, "Im1", jpegBytes, "jpeg", nil, false)
out, err := pdfer.ReplaceImage(pdfBytes, "Im1", pngBytes,  "png",  nil, false)
```

### PDF/A conversion and validation

```go
// Convert to PDF/A (decrypts first if needed)
out, err := pdfer.ConvertToPDFA(pdfBytes, nil, "1b") // "1b", "2b", or "3b"

// Validate conformance
vr := pdfer.ValidatePDFA(pdfBytes)
if !vr.Conformant {
    for _, v := range vr.Violations {
        fmt.Println(v.Code, v.Message)
    }
}
```

## Creating PDFs from scratch

```go
import "github.com/benedoc-inc/pdfer/v2/core/write"

builder := write.NewSimplePDFBuilder()
page := builder.AddPage(write.PageSizeLetter)

font := page.AddStandardFont("Helvetica")
page.Content().
    BeginText().
    SetFont(font, 24).
    SetTextPosition(72, 720).
    ShowText("Hello, World!").
    EndText().
    SetFillColorRGB(0.9, 0.2, 0.2).
    Rectangle(72, 660, 200, 40).
    Fill()

builder.FinalizePage(page)
pdfBytes, err := builder.Bytes()
```

### Word-wrapped text boxes

```go
style := write.TextBoxStyle{FontName: font, FontSize: 11, Align: write.TextAlignLeft}
height := page.DrawTextBox(72, 700, 468, "Long paragraph text that wraps automatically...", style)
```

### Tables

```go
tbl := write.NewTableBuilder([]float64{150, 200, 118}, write.TableStyle{
    FontName: font, FontSize: 10,
})
hdr := tbl.AddRow(true)
hdr.AddCell("Field").WithAlign(write.TextAlignCenter)
hdr.AddCell("Value").WithAlign(write.TextAlignCenter)
hdr.AddCell("Notes").WithAlign(write.TextAlignCenter)

row := tbl.AddRow(false)
row.AddCell("Device Name")
row.AddCell("MyDevice 3000")
row.AddCell("")

height = page.DrawTable(72, 680, tbl)
```

### Multi-column layout

```go
layout := page.NewColumnLayout(72, 720, 468, 2, 12) // 2 columns, 12pt gutter

layout.DrawTextBox(0, "Left column content...", style, 6)
layout.DrawTextBox(1, "Right column content...", style, 6)

layout.BalanceColumns() // align cursors to the lowest column
```

## Parsing PDFs directly

```go
import "github.com/benedoc-inc/pdfer/v2/core/parse"

pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{
    Password: []byte("secret"),
    Verbose:  false,
})

fmt.Println(pdf.Version(), pdf.ObjectCount(), pdf.IsEncrypted())
obj, err := pdf.GetObject(5)
```

## Package layout

```
pdfer/
├── pdfer.go / api.go   — root package (start here)
├── core/
│   ├── parse/          — PDF structure parsing
│   ├── write/          — PDF generation and PDF/A validation
│   ├── encrypt/        — RC4/AES encryption primitives
│   ├── manipulate/     — all document-level operations
│   ├── sign/           — digital signature creation and validation
│   └── compare/        — structured PDF diffing
├── forms/
│   ├── acroform/       — AcroForm parsing, filling, flattening
│   └── xfa/            — XFA stream extraction and dataset updating
├── content/extract/    — text, image, annotation, bookmark extraction
├── resources/font/     — TrueType/OpenType font embedding
└── types/              — shared data structures
```

## Feature matrix

| Category | Feature | Status |
|---|---|---|
| **Encryption** | RC4 40/128-bit, AES 128/256-bit read | ✅ |
| | AES-128 write | ✅ |
| | Owner-password auth (R≤4) | ✅ |
| | Permission flags | ✅ |
| **Page ops** | Merge, split, extract, delete | ✅ |
| | Reorder, insert blank, duplicate | ✅ |
| | Rotate, crop, resize | ✅ |
| **Content** | Stamp text / page numbers | ✅ |
| | Redact content streams, annotations, image XObjects | ✅ |
| | Redact XMP/Info metadata | call `RedactMetadata` separately |
| | Linearize (Fast Web View) | ✅ |
| | Repair / rebuild | ✅ |
| **Metadata** | Read /Info + XMP | ✅ |
| | Write /Info | ✅ |
| | Strip metadata (privacy) | ✅ |
| **Annotations** | Link (URI + internal), Text, FreeText | ✅ |
| | Highlight, Underline, StrikeOut | ✅ |
| **Bookmarks** | Read and write outline tree | ✅ |
| **Signatures** | PKCS#7 / CMS detached signing | ✅ |
| | Signature validation (RSA + ECDSA) | ✅ |
| | Visible signature field appearance | ❌ |
| | RFC 3161 timestamp (TSA) | ❌ |
| | Long-term validation (LTV / OCSP / CRL) | ❌ |
| **File attachments** | Embed and extract files via `/EmbeddedFiles` name tree | ✅ |
| **Forms** | AcroForm parse, fill, flatten | ✅ |
| | XFA extract, fill, rebuild | ✅ |
| **Extraction** | Text, graphics, images, fonts | ✅ |
| | Annotations, bookmarks, metadata | ✅ |
| | Table detection (grid lines + rect-based, merged cells, header rows) | ✅ |
| | JSON serialization, directory dump | ✅ |
| | Text search / find-and-highlight | ❌ |
| | JPEG2000 (JPXDecode) decode | ❌ |
| | JBIG2 decode | ❌ |
| **Comparison** | Structural + text + image diff | ✅ |
| **PDF/A** | Conformance validation (heuristic, parts 1–3) | ✅ |
| | Conversion of arbitrary PDFs | ✅ |
| **Images** | Replace image XObject (JPEG/PNG/raw) | ✅ |
| **Write / layout** | Word-wrapped text boxes | ✅ |
| | Tables (header rows, col/row span, per-cell bg) | ✅ |
| | Multi-column layout helper | ✅ |
| **Parsing** | xref tables + streams (PDF 1.5+) | ✅ |
| | Object streams + Type-2 xref entries | ✅ |
| | Incremental updates | ✅ |

## Known limitations

See [GAPS.md](GAPS.md) for the full history and detailed file pointers.

**Redaction**
- `Redact` clears content streams, annotation objects, and image XObjects within the specified boxes. XMP metadata and `/Info` entries are not cleared — call `RedactMetadata` separately for document-level metadata.

**Digital signatures**
- Signatures are always invisible (no rendered appearance box).
- No RFC 3161 timestamps — signatures become unverifiable after the signing certificate expires.
- No long-term validation (no embedded OCSP responses or CRL data).

**Forms**
- `Form.Validate()` returns "not implemented" for XFA forms — structural extraction only.
- Calculated form fields are not re-evaluated on `Fill()`; dependent fields remain stale until opened in a viewer.
- XFA scripts are exposed verbatim via `FormSchema.Scripts` — pdfer does not interpret FormCalc or JavaScript. Every `<script>` in the template is extracted, including those on nodes not surfaced as questions; scripts whose owner is not a typed schema entity carry an empty `OwnerID` and are correlated via `OwnerPath`.
- XFA `<occur>` and `<bind>` declarations are surfaced as data (`Occur`/`Bind` on `Question`, `FormSection`, and `FormElement`) but never acted on — no instance expansion, no template/data merge. The schema lists each repeating subform once; materializing instances is the renderer's job.

**Images / encoding**
- JPEG2000 (`JPXDecode`) and JBIG2 image streams are detected but not decoded.
- CMYK images are returned with raw CMYK bytes; callers must convert to RGB.

**Other**
- Linearize does not emit a `/H` hint stream — object ordering is correct but byte-serving is not optimised.
- `StampText` emits a single `Tj` operator; text is not wrapped. Use `PageBuilder.DrawTextBox` for multi-line text in generated PDFs.
- Optional Content Groups (PDF layers) are not accessible via the API.
- Named destinations are not exposed.
- PDF/A validation is heuristic — it misses font subset tags, transparency groups, overprint settings, and annotation appearance requirements.

## Testing

```bash
go test ./...
```

## License

MIT — see [LICENSE](LICENSE) for details.
