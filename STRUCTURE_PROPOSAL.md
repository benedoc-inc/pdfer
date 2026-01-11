# Project Structure Reorganization Proposal

## Current Structure

```
pdfer/
├── acroform/        # AcroForm processing (parsing, filling, creation)
├── xfa/             # XFA form processing
├── extraction/      # Content extraction (NEW - text, images, graphics)
├── parser/          # Low-level PDF parsing
├── writer/          # PDF creation/writing
├── encryption/      # Encryption/decryption
├── font/            # Font embedding
├── types/           # Shared data structures
└── pdfer.go         # Root package with type aliases
```

## Issues Identified

### 1. **Forms Split Across Packages**
- `acroform/` and `xfa/` are separate packages but both handle forms
- Users must know which form type they're dealing with
- No unified form interface
- Duplication: both have extraction, filling, validation concepts

### 2. **Content Operations Scattered**
- `extraction/` is new and isolated
- Content manipulation (future) would be separate
- No clear "content" domain organization

### 3. **Core Operations Not Grouped**
- `parser/`, `writer/`, and `encryption/` are all fundamental operations
- No clear "core" or "foundation" grouping
- Hard to see the library's foundation layer

### 4. **Resources Not Organized**
- `font/` is isolated
- Future resources (images, etc.) would be scattered
- No clear "resources" domain

### 5. **Package Naming Inconsistency**
- Mix of nouns (`parser`, `writer`) and verbs (`extraction`)
- No clear organizational pattern

## Proposed Structure: Option B (Domain-Based)

### Design Principles

1. **Domain-Driven Organization**: Group by functional domain, not technical layer
2. **Scalability**: Structure supports future features naturally
3. **Discoverability**: Users can find related functionality easily
4. **Separation of Concerns**: Clear boundaries between domains
5. **Unified Interfaces**: Common interfaces where appropriate

### New Structure

```
pdfer/
├── core/                    # Core PDF operations (foundation layer)
│   ├── parse/              # PDF parsing (reading structure)
│   │   ├── api.go         # Main parsing API (Open, OpenWithOptions)
│   │   ├── document.go    # Document structure
│   │   ├── object.go      # Object retrieval
│   │   ├── xref.go        # Cross-reference parsing
│   │   ├── stream.go      # Stream handling
│   │   ├── incremental.go # Incremental updates
│   │   └── filters.go     # Stream filters
│   ├── write/             # PDF writing (creating/modifying)
│   │   ├── writer.go      # Main writer API
│   │   ├── page.go        # Page building
│   │   ├── content.go     # Content stream building
│   │   ├── image.go       # Image embedding
│   │   └── builder.go     # High-level builders
│   └── encrypt/           # Encryption/decryption
│       ├── decrypt.go     # Decryption
│       ├── encrypt.go     # Encryption (future)
│       └── key_derivation.go
│
├── forms/                  # Form processing (unified domain)
│   ├── forms.go           # Unified form interface
│   ├── detect.go          # Auto-detect form type
│   ├── acroform/          # AcroForm implementation
│   │   ├── parse.go       # Parsing
│   │   ├── extract.go     # Extraction
│   │   ├── fill.go        # Filling
│   │   ├── create.go      # Creation
│   │   ├── validate.go    # Validation
│   │   ├── appearance.go  # Appearance streams
│   │   ├── actions.go     # Field actions
│   │   └── stream.go      # Object stream handling
│   └── xfa/               # XFA implementation
│       ├── extract.go     # Stream extraction
│       ├── parse.go       # XML parsing
│       ├── translate.go   # Form translation
│       ├── update.go      # Form updates
│       └── build.go       # XFA PDF building
│
├── content/                # Content operations (reading/manipulating content)
│   ├── extract/           # Content extraction
│   │   ├── extract.go     # Main extraction API
│   │   ├── metadata.go    # Metadata extraction
│   │   ├── pages.go       # Page extraction
│   │   ├── text.go        # Text extraction
│   │   ├── graphics.go    # Graphics extraction
│   │   ├── images.go      # Image extraction
│   │   ├── annotations.go # Annotation extraction
│   │   └── bookmarks.go   # Bookmark extraction
│   ├── manipulate/        # Content manipulation (future)
│   │   ├── text.go        # Text manipulation
│   │   ├── graphics.go    # Graphics manipulation
│   │   └── pages.go       # Page manipulation
│   └── transform/         # Content transformation (future)
│       ├── merge.go       # PDF merging
│       ├── split.go       # PDF splitting
│       └── optimize.go    # PDF optimization
│
├── resources/             # Embeddable resources
│   ├── font/              # Font resources
│   │   ├── font.go        # Font parsing
│   │   ├── pdf.go         # PDF font objects
│   │   └── subset.go      # Font subsetting
│   └── image/             # Image resources (future)
│       ├── jpeg.go
│       ├── png.go
│       └── embed.go
│
├── types/                 # Shared data structures
│   ├── core.go           # Core types (PDFEncryption, etc.)
│   ├── forms.go          # Form types (FormSchema, Question, etc.)
│   ├── content.go        # Content types (ContentDocument, Page, etc.)
│   └── xfa.go            # XFA-specific types
│
├── pdfer.go              # Root package with type aliases and convenience functions
├── cmd/
│   └── pdfer/            # CLI tool
├── examples/             # Usage examples
└── tests/                # Integration tests
```

## Package Responsibilities

### `core/` - Foundation Layer

**Purpose**: Fundamental PDF operations that everything else builds on.

- **`core/parse/`**: Read and parse PDF structure
  - Object retrieval
  - Cross-reference parsing
  - Stream decompression
  - Incremental update handling
  
- **`core/write/`**: Create and modify PDFs
  - Object creation
  - Stream compression
  - PDF generation
  - High-level builders
  
- **`core/encrypt/`**: Security operations
  - Decryption (reading encrypted PDFs)
  - Encryption (writing encrypted PDFs - future)
  - Key derivation

**Key Insight**: These are the building blocks. Everything else uses these.

### `forms/` - Form Domain

**Purpose**: Unified form processing regardless of form type.

- **`forms/forms.go`**: Unified interface
  ```go
  type Form interface {
      Type() FormType
      Schema() *types.FormSchema
      Extract() (*types.FormSchema, error)
      Fill(data types.FormData) ([]byte, error)
      Validate(data types.FormData) []error
  }
  ```

- **`forms/detect.go`**: Auto-detect form type
  ```go
  func Detect(pdfBytes []byte) (FormType, error)
  func Extract(pdfBytes []byte, password []byte) (Form, error)
  ```

- **`forms/acroform/`**: AcroForm implementation
- **`forms/xfa/`**: XFA implementation

**Key Insight**: Users shouldn't need to know form type. Unified interface handles both.

### `content/` - Content Domain

**Purpose**: Operations on PDF content (text, graphics, images, etc.).

- **`content/extract/`**: Extract content into structured data
  - Text with positioning
  - Graphics/paths
  - Images
  - Annotations
  - Metadata
  - All serializable to JSON

- **`content/manipulate/`**: Modify existing content (future)
  - Edit text
  - Modify graphics
  - Transform pages

- **`content/transform/`**: Document-level transformations (future)
  - Merge PDFs
  - Split PDFs
  - Rotate pages
  - Optimize

**Key Insight**: Content operations are separate from form operations. Natural extension point.

### `resources/` - Resource Domain

**Purpose**: Embeddable resources (fonts, images, etc.).

- **`resources/font/`**: Font embedding
  - TTF/OTF parsing
  - Subsetting
  - PDF font object creation

- **`resources/image/`**: Image embedding (future)
  - JPEG/PNG support
  - Image optimization
  - PDF image object creation

**Key Insight**: Resources are reusable across documents. Separate domain makes sense.

### `types/` - Shared Types

**Purpose**: Data structures used across domains.

- **`types/core.go`**: Core PDF types (encryption, etc.)
- **`types/forms.go`**: Form types (FormSchema, Question, etc.)
- **`types/content.go`**: Content types (ContentDocument, Page, etc.)
- **`types/xfa.go`**: XFA-specific types

**Key Insight**: Types are organized by domain, not by technical layer.

## Import Path Changes

### Before → After

```go
// OLD
import "github.com/benedoc-inc/pdfer/parser"
import "github.com/benedoc-inc/pdfer/writer"
import "github.com/benedoc-inc/pdfer/encryption"
import "github.com/benedoc-inc/pdfer/acroform"
import "github.com/benedoc-inc/pdfer/xfa"
import "github.com/benedoc-inc/pdfer/font"
import "github.com/benedoc-inc/pdfer/extraction"

// NEW
import "github.com/benedoc-inc/pdfer/core/parse"
import "github.com/benedoc-inc/pdfer/core/write"
import "github.com/benedoc-inc/pdfer/core/encrypt"
import "github.com/benedoc-inc/pdfer/forms"
import "github.com/benedoc-inc/pdfer/forms/acroform"
import "github.com/benedoc-inc/pdfer/forms/xfa"
import "github.com/benedoc-inc/pdfer/resources/font"
import "github.com/benedoc-inc/pdfer/content/extract"
```

## Migration Strategy

### Phase 1: Create New Structure (v1.0.0-alpha)

1. Create new directory structure
2. Move files to new locations
3. Update all imports
4. Update package declarations
5. Ensure all tests pass

### Phase 2: Update Documentation (v1.0.0-beta)

1. Update README with new structure
2. Update all examples
3. Create migration guide
4. Update GAPS.md

### Phase 3: Release (v1.0.0)

1. Tag v1.0.0
2. Update CHANGELOG
3. Announce breaking changes
4. Provide migration examples

## Benefits of New Structure

### 1. **Clear Domain Boundaries**
- Forms are clearly separate from content
- Core operations are foundation layer
- Resources are reusable components

### 2. **Natural Extension Points**
- New form types → `forms/newtype/`
- New content operations → `content/newop/`
- New resources → `resources/newresource/`

### 3. **Better Discoverability**
- Users looking for forms → `forms/`
- Users looking for content → `content/`
- Users looking for core operations → `core/`

### 4. **Unified Interfaces**
- `forms/` provides unified form interface
- `content/` provides unified content interface
- Consistent patterns across domains

### 5. **Scalability**
- Structure supports growth
- Clear where new features belong
- No confusion about package location

## Example Usage After Reorganization

### Unified Form Interface

```go
import "github.com/benedoc-inc/pdfer/forms"

// Auto-detect and extract any form type
form, err := forms.Extract(pdfBytes, password, false)
if err != nil {
    log.Fatal(err)
}

// Work with unified interface
schema := form.Schema()
filled, err := form.Fill(formData)
```

### Content Extraction

```go
import "github.com/benedoc-inc/pdfer/content/extract"

// Extract all content
doc, err := extract.Content(pdfBytes, password, false)
if err != nil {
    log.Fatal(err)
}

// Serialize to JSON
jsonBytes, _ := json.MarshalIndent(doc, "", "  ")
```

### Core Operations

```go
import "github.com/benedoc-inc/pdfer/core/parse"
import "github.com/benedoc-inc/pdfer/core/write"

// Parse PDF
pdf, err := parse.Open(pdfBytes)

// Create PDF
builder := write.NewBuilder()
```

## Decision

**Implement Option B: Full Domain-Based Reorganization**

This structure:
- ✅ Groups related functionality logically
- ✅ Provides clear extension points
- ✅ Supports unified interfaces
- ✅ Scales naturally
- ✅ Improves discoverability
- ✅ Makes the library more general and maintainable

**Trade-off**: Breaking changes require v1.0.0 release, but the improved structure is worth it for long-term maintainability and usability.
