# Known Gaps

Concrete, verified gaps with file pointers. Update this file when a gap is
closed. "Workaround" describes what currently happens instead of the correct
behaviour.

---

## P1 — High impact

### ~~`DecryptPDF` returns encrypted bytes unchanged~~ ✅ Fixed in v0.9.27
`manipulate.DecryptPDF` / `pdfer.DecryptPDF` now produces a fully decrypted
plaintext PDF. `encrypt.DecryptPDFObjects` remains a stub but is no longer
called by the public API.

### Owner-password authentication is broken for R≤4
**File:** `core/encrypt/key_derivation.go:268` — `DeriveUserKeyFromOwner`  
The function returns the owner key unchanged instead of implementing Algorithm 3
inverse (ISO 32000-1 §7.6.3.4): RC4-decrypt `/O` with the owner-derived key to
recover the padded user password, then derive the file encryption key from
that. Opening an encrypted PDF with the *owner* password (rather than the user
password) fails or accepts wrong passwords for R=3/4 documents.

---

## P2 — Medium impact

### PDF comparison text diff is not wired up
**File:** `core/compare/compare.go:521`  
`compareText` is declared as a placeholder; the actual implementation is in
`text_diff.go` but `compareSinglePage` never calls it. `PageDifference.TextDiff`
is always nil, making `ComparePDFs` / `ComparePDFsWithOptions` blind to text
changes between pages.  
**To fix:** Call the text diff logic from `text_diff.go` inside
`compareSinglePage`.

### `FlattenForm` silently skips pages with indirect `/Resources`
**File:** `forms/acroform/flatten.go:24`  
Pages whose `/Resources` is an indirect object reference (rather than an inline
dict) are passed through without flattening. Widget annotations on those pages
are silently dropped from the output.  
**To fix:** Dereference the indirect resource object, inject XObject entries,
write back the modified object.

### PDF merge leaves dangling references for cross-document refs
**File:** `core/manipulate/merging.go:188`  
Object numbers are remapped when merging PDFs to avoid collisions, but
references to objects not in the remap table are passed through unchanged.
Complex PDFs with object streams or shared resources may have dangling
references in the merged output.

### Inline AcroForm dict not supported
**File:** `forms/acroform/parser.go:78`  
PDFs where `/AcroForm` is an inline dict in the catalog (rather than an
indirect object reference) return `ErrCodeUnsupportedPDF`. Rare in practice but
produced by some older generators.

### Page deletion does not update ancestor `/Count`
**File:** `core/manipulate/deletion.go:233`  
Deleting a page updates the `/Count` of its immediate parent `/Pages` node only.
Ancestor nodes in multi-level page trees retain stale counts, which confuses
some PDF readers on large documents.

---

## P3 — Low impact

### Font subsetting embeds the full font
**File:** `resources/font/font.go:517` — `CreateSubsetFont`  
The function embeds the full font program rather than a subset containing only
the glyphs used in the document. File size is larger than necessary, sometimes
significantly for CJK or large symbol fonts.

### Watermark opacity uses colour blending instead of `ExtGState`
**File:** `core/write/watermark.go:50`  
Transparency is approximated by blending the requested colour toward white
rather than using a proper `/ca` fill-alpha entry in a graphics state
dictionary. The result is a lighter colour, not true transparency, and does not
composite correctly over non-white backgrounds.

### XFA form validation always returns nil
**File:** `forms/forms.go:98`  
`Form.Validate()` on XFA forms returns nil unconditionally. XFA validation
rules (mandatory fields, patterns, value ranges) are not evaluated.

### XFA script parsing is approximate
**File:** `forms/xfa/xfa_form_translator.go:866,916`  
FormCalc/JavaScript script bodies are extracted with a simplified parser that
may misread complex expressions. Calculation and validation scripts may produce
incorrect results.

### xref stream Type-2 entries partially skipped
**File:** `core/parse/xref_stream.go:185`  
When the xref is a cross-reference stream (PDF 1.5+), Type-2 entries pointing
into object streams are skipped during xref construction. Object streams are
parsed separately so most objects are reachable, but objects that appear
*exclusively* via a Type-2 entry in a file with no traditional xref table may
be inaccessible.

### AcroForm field actions use a simplified replace
**File:** `forms/acroform/actions.go:72`  
Replacing an existing `/A` or `/AA` entry uses a simple string substitution
rather than a proper dict parser, which may corrupt the dict if the existing
action value contains nested structures.
