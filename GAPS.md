# Known Gaps

Concrete, verified gaps with file pointers. Update this file when a gap is
closed. "Workaround" describes what currently happens instead of the correct
behaviour.

---

## P1 — High impact

### ~~AES-128 (V=4/R=4) passwords rejected by all PDF readers~~ ✅ Fixed in v1.7.0
`DeriveEncryptionKey` and `DeriveOwnerKey` were padding short passwords by
cycling password bytes (e.g. "open" → "openopenopen…") instead of appending
the standard 32-byte PDF padding string (ISO 32000-1 §7.6.3.3 step a).
`ComputeOValue` (in `encrypt.go`) already used the correct `padPassword` helper,
so the /O entry was valid but the derived encryption key did not match what any
external PDF reader computes. Fixed by calling `padPassword` in both functions.

**File**: `core/encrypt/key_derivation.go`

### ~~`DecryptPDF` returns encrypted bytes unchanged~~ ✅ Fixed in v0.9.27–v0.9.28
`manipulate.DecryptPDF` / `pdfer.DecryptPDF` now produces a fully decrypted
plaintext PDF. `encrypt.DecryptPDF` signature changed to `(*PDFEncryption,
error)` — callers that want fully decrypted bytes use `manipulate.DecryptPDF`.
`PDFDocument.reconstruct()` is now implemented (rebuilds xref + trailer from
revision components instead of returning nil).

### ~~Owner-password authentication is broken for R≤4~~ ✅ Fixed in v0.9.30
`DeriveOwnerKey` now applies Algorithm 3 step-e inverse correctly for R≥3:
20 RC4 passes in descending order (i=19→0), each XOR-ing key bytes with `i`,
before calling `DeriveEncryptionKey` to produce the file key. R=2 still uses a
single pass (correct per spec). `DeriveUserKeyFromOwner` is a correct pass-through
since `DeriveOwnerKey` already returns the file encryption key.

---

## P2 — Medium impact

### ~~PDF comparison text diff is not wired up~~ ✅ Already wired (stale gap)
`compareText` is implemented in `text_diff.go` and called from
`compareSinglePage` at line 453. The stub comment at line 521 was an artefact;
it has been removed. `PageDifference.TextDiff` is populated correctly.

### ~~`FlattenForm` silently skips pages with indirect `/Resources`~~ ✅ Fixed in v0.9.30
`flattenIndirectResourcesObjNum` detects `/Resources N G R` patterns; when found,
`flattenAddXObjectsToDict` injects XObject entries directly into the resources
object. Both inline and indirect `/Resources` dicts are now handled.

### ~~PDF merge leaves dangling references for cross-document refs~~ ✅ Fixed in v0.9.30
`MergePDFs` now uses a two-pass approach: pass 1 assigns all new object numbers
before any content is read, so pass 2 has a complete remap table when rewriting
references. Forward references (object A → B where B appears later) are now
resolved correctly.

### ~~Inline AcroForm dict not supported~~ ✅ Fixed in v0.9.29
`parseAcroFormFromBytes` now extracts the inline `<< ... >>` dict using a
bracket-counting helper (`extractBracketedDict`) and passes it directly to
`parseAcroFormDict`. Nested dicts (e.g. `/DR`) and string literals with angle
brackets are handled correctly.

### ~~Page deletion does not update ancestor `/Count`~~ ✅ Fixed in v0.9.30
`updatePageCounts` now walks the full `/Parent` chain from the immediate parent
up to the root `/Pages` node, decrementing `/Count` at every level. Multi-level
page trees produce correct counts throughout the hierarchy.

### ~~`FlattenForm` appearance streams with indirect `/Resources` may have nested XObject issues~~ ✅ Fixed in v1.1.0
`flattenAddXObjectsToDict` now uses a `findDictEnd` bracket-counter instead of
the regex `[^<>]*` character class. Arbitrarily nested dicts inside an existing
`/XObject` entry are handled correctly.

### ~~`Form.Validate()` silently succeeds for XFA forms~~ ✅ Fixed in v1.1.0
`XFAFormWrapper.Validate()` now returns `[]error{fmt.Errorf("XFA form validation
is not implemented")}`, so callers can distinguish "no validation errors" from
"validation not supported."

### ~~xref stream Type-2 entries partially skipped~~ ✅ Fixed in v1.1.0
`ParseXRefStreamWithEncryption` and `ParseCrossReferenceTableWithEncryption` now
delegate to `ParseXRefStreamFull`, which handles both Type-1 and Type-2 entries
using a single implementation. The duplicate parsing logic has been removed.
Type-2 entries (compressed objects in object streams) are fully parsed; callers
that receive the `map[int]int64` return type still see only Type-1 byte offsets —
callers needing full Type-2 support should call `ParseXRefStreamFull` directly.

### ~~AcroForm field actions use a simplified replace~~ ✅ Fixed in v1.1.0
`AddActionToField` and `AddMouseAction` now use a `removeDictKey` helper that
bracket-counts through nested dicts and literal strings to cleanly remove any
existing `/A` or `/AA` entry before injecting the new action. String substitution
and `strings.LastIndex` have been replaced throughout.

---

## P3 — Low impact

### ~~Font subsetting embeds the full font~~ ✅ Fixed in v1.1.0
`CreateSubsetFont` now builds a real TTF subset: it rebuilds `glyf`, `loca`,
`hmtx`, and `cmap` (format 4) tables containing only the glyphs in `f.Subset`
plus any composite components they reference. All other tables (`head`, `hhea`,
`maxp`, `name`, `OS/2`, `post`, etc.) are copied as-is. Output is a valid TTF
with a correct offset table, table directory, and `head.checkSumAdjustment`.
Falls back to the full font on any parse error.

### ~~Watermark opacity uses colour blending instead of `ExtGState`~~ ✅ Fixed in v1.1.0
`AddWatermark` now registers a `WMgs` ExtGState resource with `/ca` (fill alpha)
and `/CA` (stroke alpha) at the requested opacity and applies it via the `gs`
operator. `PageBuilder` gained an `extgstates` map and a `Build()` block that
emits `/ExtGState << ... >>` in the page resources dictionary.

### ~~XFA script parsing is approximate~~ ✅ Superseded in v2.0.0
The structured `parseXFAScript` analyser shipped in v1.2.0 (regex-based
visibility / set-value / validate / calculate classification, populating
`Rule`/`Condition`/`Action`) was lossy and incorrect on non-trivial scripts —
faithful interpretation needs a real ES5/FormCalc AST, which is out of scope
for a PDF library. v2.0.0 removes the heuristic and the `Rule`/`Action` type
surface entirely; `FormSchema.Scripts []FormScript` now exposes verbatim
script bodies with their event activity, language (`javascript` | `formcalc`),
SOM owner path, and owner ID, leaving interpretation to the caller. See the
**Forms** section of the README for usage and the type comment on
`types.FormScript` for owner-reference semantics. (The follow-on gap — scripts
attached to nodes pdfer did not surface, such as decorative `<draw>`s,
`bind="none"` non-AddAttachment buttons, `<pageArea>` events, and per-option
`<field>`s collapsed into an `<exclGroup>` — was closed by the orphan-script
extraction in `88b6989`; such scripts are now extracted with `OwnerPath` set
and an empty `OwnerID` when the owner is not a typed schema entity.)

---

## Open gaps

### P1 — High impact

#### ~~Redaction is incomplete (content streams only)~~ ✅ Fixed in v1.3.0
`Redact` now handles three of the four previously missing surfaces:
- **Annotation objects** — every annotation object whose `/Rect` overlaps a
  redaction box is zeroed out (replaced with an empty dict), so that URIs,
  note text, and tooltip strings are unrecoverable.
- **Image XObjects** — the placement bounding box is computed by transforming
  the unit square through the current CTM, not just checking the translation
  anchor. Image objects whose footprint overlaps a box are replaced with a 1×1
  black pixel stream so no image data remains.
- **Content streams** — existing behaviour (text/path operator suppression)
  plus an opaque overlay rectangle.

**Remaining gap**: XMP metadata and `/Info` dictionary entries are not cleared
by `Redact`. Call `RedactMetadata` separately for document-level metadata.

---

### P2 — Medium impact

#### ~~Annotation appearance streams missing for most subtypes~~ ✅ Fixed in v1.7.0
`AnnotationBuilder.build()` produced structurally valid annotation dicts but no
`/AP` (appearance) entry.  Adobe Acrobat synthesises appearances from the
annotation dictionary; Preview and Skim do not for: Squiggly, Polygon, PolyLine,
Ink, Line, Square, Circle, and FreeText.  A new `buildAppearance()` method now
generates a Form XObject for each of these subtypes and wires it in as
`/AP<</N N 0 R>>`.

**File**: `core/write/annotations_ap.go` (new), `core/write/annotations.go`

#### ~~AcroForm push button is invisible~~ ✅ Fixed in v1.7.0
`writeBtnKeys` now generates a raised-bevel gray appearance stream
(`pushButtonAppearance`) and adds `/MK<</CA(label)/TP 0>>` so viewers display
the field name as the button caption.

**File**: `forms/acroform/writer.go` (`writeBtnKeys`, `pushButtonAppearance`)

#### ~~AcroForm password field shows plain text~~ ✅ Fixed in v1.8.0
`SetPassword(true)` sets bit 13 of `/Ff` correctly. When the password flag is
set, `writeTxKeys` now calls `passwordFieldAppearance` to emit an explicit `/AP`
Form XObject containing a white background, border (solid or underline-only per
`BorderStyle`), and masked bullet chars (`\225` WinAnsi) for any pre-filled value.
Vertical centering uses cap-height ≈ 0.72 × fontSize. Font resources (`/Helv`)
are embedded in the XObject's `/Resources` dict so the stream is self-contained.

**File**: `forms/acroform/writer.go` (`writeTxKeys`, `passwordFieldAppearance`)

#### ~~AcroForm underline-style field shows no underline~~ ✅ Fixed in v1.9.0
`AddUnderlineTextField` sets `/BS<</W 1/S/U>>` correctly. All `Tx` fields now
get an explicit `/AP` Form XObject from `textFieldAppearance`. The border branch
checks `BorderStyle == "U"` and strokes only the bottom edge (`0 0 m w 0 l S`);
any other style strokes the full inset box. For non-password fields with a value,
`textFieldAppearance` also renders the value using Helvetica AFM glyph widths
(`helvetica_widths.go`): single-line fields are vertically centred and
right-edge-clipped; multiline fields are word-wrapped with `wrapText` (which
also respects explicit `\n` hard breaks) and overflow-guarded. The rendering
path uses `write.EscapePDFString` for correct WinAnsiEncoding of non-ASCII chars.

**File**: `forms/acroform/writer.go` (`textFieldAppearance`),
`forms/acroform/helvetica_widths.go` (`wrapText`, `stringWidth`, `glyphWidth`),
`core/write/content.go` (`EscapePDFString` export)

#### JavaScript action removal API missing
`core/parse` can detect JavaScript in a PDF (e.g. scanning `/S /JavaScript`
objects), but the public API exposes no way to strip embedded JavaScript from
action dictionaries (`/AA`, `/A`, `/OpenAction`). This matters for security
sandboxing and PDF/A conformance.

**Workaround**: none — callers must implement their own object-walk and
rewrite.

**File**: `core/manipulate/` (new file needed), `api.go`

#### PDF/A validation is heuristic
`ValidatePDFA` uses text-scanning heuristics rather than a full structural
traversal. Violations it misses:
- **Type0/CIDFont subset tag** — embedded CID fonts must carry a 6-character
  subset prefix; this is not checked
- **Transparency and blend modes** — `/Group`, `/BM`, `/SMask` in content
  streams
- **Overprint** — `/op`, `/OP`, `/OPM` operator sequences
- **Annotation appearances** — every annotation must have an `/AP` stream
- **Action types** — `Launch`, `Sound`, `Movie`, `ResetForm` are forbidden in
  PDF/A-1b

**File**: `core/write/pdfa_validate.go`

#### ~~Digital signatures lack timestamps (TSA / RFC 3161)~~ ✅ Fixed in v1.3.0
Set `SignOptions.TSAEndpoint` to any RFC 3161 URL. `SignPDF` signs once, hashes
the raw `signerInfo.signature` bytes with SHA-256, POSTs a `TimeStampReq` to
the endpoint, and embeds the `TimeStampToken` (ContentInfo) as the
`id-aa-signatureTimeStampToken` unsigned attribute in SignerInfo. The
`/Contents` reservation is automatically increased to 20 KB when a TSA is
configured. Leaving `TSAEndpoint` empty is a no-op — fully backward compatible.

**Files**: `core/sign/tsa.go`, `core/sign/pkcs7.go`, `core/sign/sign.go`

#### ~~No long-term validation (LTV) support~~ ✅ Fixed in v1.3.0
Set `SignOptions.CertChain` and/or `SignOptions.OCSPResponse`. After signing,
`appendDSS` adds a `/DSS` incremental update containing:
- Stream objects for each cert in the chain (`/Certs`)
- A stream object for the pre-fetched OCSP response (`/OCSPs`), if provided
- A `/VRI` dict keyed by SHA-1 of the CMS SignedData bytes
- `/DSS N 0 R` injected into the catalog

The library embeds whatever bytes the caller provides; callers are responsible
for fetching OCSP responses from the URL in the signing cert's `OCSPServer`
field (use `golang.org/x/crypto/ocsp` or any RFC 2560 client).

**Files**: `core/sign/ltv.go`, `core/sign/sign.go`

#### ~~Signature fields are always invisible~~ ✅ Fixed in v1.3.0
Set `SignOptions.Appearance` to a `*SignAppearance{X, Y, Width, Height}`. When
set, `SignPDF` allocates a Form XObject appearance stream containing a light
border, the signer name (from the certificate CommonName), the signing date,
and the Reason (if provided) in 8pt Helvetica. The widget `/Rect` is set to the
requested bounding box and `/AP << /N N 0 R >>` points to the stream. Omitting
`Appearance` (nil) is fully backward compatible — the widget remains invisible
with `Rect [0 0 0 0]`.

**File**: `core/sign/sign.go`

---

### P3 — Low impact

#### Optional Content Groups (PDF layers) not supported
PDFs can carry an `/OCProperties` dictionary that defines named layers
(`/OCGs`). There is no API to list layer names, toggle visibility, or flatten a
layer (burn its content into the page permanently).

**File**: new `core/manipulate/layers.go` + `api.go`

#### Named destinations not exposed
The document catalog may contain a `/Dests` dictionary mapping human-readable
names to page destinations. `GetBookmarks` does not resolve named destination
strings, and there is no `GetNamedDestinations()` API.

**File**: `content/extract/bookmarks.go`, `api.go`

#### ~~Embedded file attachments not supported~~ ✅ Fixed
`pdfer.EmbedAttachments(pdfBytes, []pdfer.FileAttachment{...})` writes each
attachment as an incremental update: an `EmbeddedFile` stream object per file,
a Filespec dict per file, and a `/Names` object that extends (or creates) the
catalog's `/EmbeddedFiles` name tree. The original PDF bytes are never
modified — only appended to.

For encrypted PDFs, use `EmbedAttachmentsWithPassword(pdfBytes, files, password)`:
the appended streams are encrypted with the document's per-object keys (strings
follow the document's `/StrF` crypt filter) and the file `/ID` is carried into
the new trailer (the standard security handler derives the key from `/ID[0]`, so
dropping it breaks decryption). The shared envelope lives in `core/incremental`,
used by both attachments and AcroForm incremental fills.

**Files**: `attach.go`, `core/incremental/incremental.go`, `core/encrypt/encrypt_object.go`

#### JPEG2000 (JPXDecode) images not decoded
`images.go` identifies `JPXDecode`-filtered streams and reports the format, but
returns the raw compressed bytes without decoding. Standard `image/jpeg` cannot
parse JPEG2000; a pure-Go JPEG2000 decoder would be needed (or `golang.org/x/image/jpeg2000`).

**File**: `content/extract/images.go`

#### JBIG2 images not decoded
`JBig2Decode`-filtered streams (common in scanned documents and fax) are
completely unhandled — no format detection and no decoding path.

**File**: `content/extract/images.go`, `core/parse/decode.go`

#### CMYK images not converted on extraction
Images in `DeviceCMYK` color space are extracted with raw CMYK bytes. Callers
expecting RGB data (e.g. to re-embed as JPEG or PNG) must perform the
conversion themselves.

**File**: `content/extract/images.go`

#### Linearize omits the `/H` hint stream
`Linearize` writes a `/Linearized` parameter dict but leaves the `/H` field
set to a zero placeholder. Strictly, a linearized PDF should include a hint
stream so that byte-serving HTTP clients can fetch only the first page's objects
without reading the entire file. Current output still benefits from object
ordering but not from byte-serving optimisation.

**File**: `core/manipulate/linearize.go:20`

#### ~~`StampText` does not support multi-line text~~ ✅ Addressed via `DrawTextBox`
`PageBuilder.DrawTextBox(x, y, width, text, TextBoxStyle)` in `core/write/textbox.go`
provides word-wrapped, per-line text rendering for generated PDFs. `StampText`
(which modifies existing PDFs via incremental update) still emits a single `Tj`
and is not wrapped — use `DrawTextBox` in generated content instead.

**File**: `core/write/textbox.go`

#### No text search / find-and-highlight API
There is no `FindText(pdfBytes, query string)` function that returns match
coordinates. Building this on top of the existing text extractor + bounding-box
data is straightforward but not yet wired up.

**File**: new `core/manipulate/search.go` + `api.go`

#### Calculated form fields are not auto-evaluated on fill
`form.Fill()` writes raw values directly into field objects. Fields that carry
a `/AA /C` (Calculate action) are not re-evaluated, so dependent computed fields
remain stale until the PDF is opened in a viewer.

**File**: `forms/acroform/fill.go`

#### CID font / Type0 vertical writing not supported
`CreateSubsetFont` only handles horizontal TrueType fonts (glyph advances in
the `hmtx` table). CIDFont Type0 (OpenType CFF) and vertical-writing fonts
(`vmtx` table, `vhea` table) are not subsetted or embedded. Affected scripts:
CJK vertical text, some Arabic/Hebrew layout engines.

**File**: `resources/font/font.go`, `resources/font/subset.go`

---

### ~~XFA browser rendering gaps~~ ✅ Fixed in v2.0.0

Fourteen improvements to the XFA → `FormSchema` translation for browser rendering fidelity:

1. **Position/size data** — `x`, `y`, `w`, `h`, `minH` on `<field>`, `<draw>`, `<subform>`, `<exclGroup>` captured into `Question.Properties` and `FormSection.Width`/`Height`.
2. **Subform layout mode** — `layout` attribute (`position`, `tb`, `lr-tb`, `row`, `table`) captured into `FormSection.Layout`.
3. **`maxChars` → `ValidationRules.MaxLength`** — `<textEdit maxChars="N">` now wires directly to `ValidationRules.MaxLength`.
4. **Caption placement** — `<caption placement="left|right|top|bottom|inline">` captured into `Question.Properties["caption_placement"]`.
5. **Listbox vs dropdown** — `<choiceList open="always">` sets `Properties["listbox"]=true`; `multiSelect="1"` sets `Properties["multi_select"]=true`.
6. **`dateTimeEdit` subtype** — `<picture>TIME{...}</picture>` maps to `ResponseTypeTime`; `DATE{...}` keeps `ResponseTypeDate`.
7. **`passwordEdit` → `ResponseTypePassword`** — was incorrectly mapped to `ResponseTypeText`.
8. **Tri-state checkbox** — `<checkButton allowNeutral="1">` sets `Properties["allow_neutral"]=true`.
9. **`exclGroup` layout** — `<exclGroup layout="lr-tb">` sets `Properties["layout"]`.
10. **Rich-text exData extraction** — `<exData contentType="text/html">` without `xfa:embed` markers now extracts plain text for display instead of being blanket-suppressed.
11. **Non-interactive section content** — static draw text in non-interactive subforms (headers, instructions) is collected into `FormSection.Content` instead of being silently dropped.
12. **Separator draws** — `<draw><value><line>` pattern emits `ResponseTypeSeparator` questions for `<hr>`-style rendering.
13. **Text alignment** — `<para hAlign="center|left|right|justify">` captured into `Properties["text_align"]`.
14. **Font hints** — `<font size="9pt" weight="bold">` captured into `Properties["font_size"]` and `Properties["font_weight"]`.

**Files**: `types/form_types.go`, `forms/xfa/xfa_form_translator.go`
