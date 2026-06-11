package pdfer

import (
	"github.com/benedoc-inc/pdfer/v2/content/extract"
	"github.com/benedoc-inc/pdfer/v2/core/compare"
	"github.com/benedoc-inc/pdfer/v2/core/manipulate"
	"github.com/benedoc-inc/pdfer/v2/core/sign"
	"github.com/benedoc-inc/pdfer/v2/core/write"
	"github.com/benedoc-inc/pdfer/v2/forms"
	"github.com/benedoc-inc/pdfer/v2/forms/acroform"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// --- Type aliases -----------------------------------------------------------

// RedactBox specifies a rectangular region on a page to permanently redact.
type RedactBox = manipulate.RedactBox

// PageRange specifies an inclusive page range for splitting.
type PageRange = manipulate.PageRange

// TextStamp describes a text overlay to paint onto one or more pages.
type TextStamp = manipulate.TextStamp

// PageNumberOptions configures automatic page-number stamping.
type PageNumberOptions = manipulate.PageNumberOptions

// PageNumberPosition controls where page numbers are placed.
type PageNumberPosition = manipulate.PageNumberPosition

// PageNumberPosition values.
const (
	BottomCenter = manipulate.BottomCenter
	BottomRight  = manipulate.BottomRight
	BottomLeft   = manipulate.BottomLeft
	TopCenter    = manipulate.TopCenter
	TopRight     = manipulate.TopRight
	TopLeft      = manipulate.TopLeft
)

// MetadataUpdate holds fields to write into the PDF /Info dictionary.
// An empty string means "leave unchanged".
type MetadataUpdate = manipulate.MetadataUpdate

// Permissions describes the access-control flags of an encrypted PDF.
type Permissions = manipulate.Permissions

// AnnotationType identifies the PDF annotation subtype.
type AnnotationType = manipulate.AnnotationType

// AnnotationConfig describes an annotation to be added to a page.
type AnnotationConfig = manipulate.AnnotationConfig

// Annotation subtype constants.
const (
	AnnotText      = manipulate.AnnotText
	AnnotLink      = manipulate.AnnotLink
	AnnotFreeText  = manipulate.AnnotFreeText
	AnnotHighlight = manipulate.AnnotHighlight
	AnnotUnderline = manipulate.AnnotUnderline
	AnnotStrikeOut = manipulate.AnnotStrikeOut
)

// BookmarkEntry describes one entry in the PDF outline (bookmark) tree.
type BookmarkEntry = manipulate.BookmarkEntry

// FormType distinguishes AcroForm from XFA.
type FormType = forms.FormType

// Form is the unified interface returned by ExtractForm.
type Form = forms.Form

const (
	FormTypeAcroForm FormType = "acroform"
	FormTypeXFA      FormType = "xfa"
	FormTypeUnknown  FormType = "unknown"
)

// ComparisonResult holds the full diff between two PDFs.
type ComparisonResult = compare.ComparisonResult

// CompareOptions configures PDF comparison behaviour.
type CompareOptions = compare.CompareOptions

// SignOptions configures PDF digital signature creation.
type SignOptions = sign.SignOptions

// SignatureInfo describes one digital signature found in a PDF.
type SignatureInfo = sign.SignatureInfo

// PDFAValidationResult contains the outcome of a PDF/A conformance check.
type PDFAValidationResult = write.PDFAValidationResult

// PDFAViolation describes a single PDF/A conformance failure.
type PDFAViolation = write.PDFAViolation

// PDFAViolationCode identifies the category of a PDF/A violation.
type PDFAViolationCode = write.PDFAViolationCode

// DocumentMetadata holds title, author, and other /Info dict fields.
type DocumentMetadata = types.DocumentMetadata

// Bookmark describes one entry in the document outline.
type Bookmark = types.Bookmark

// ContentDocument is the structured extraction result from ExtractContent.
type ContentDocument = types.ContentDocument

// Image holds raw image data and metadata extracted from a PDF.
type Image = types.Image

// DirectoryOutput is the result of extracting a PDF to a filesystem directory.
type DirectoryOutput = extract.DirectoryOutput

// --- Encryption / decryption -------------------------------------------------

// DecryptPDF produces a fully decrypted, plaintext PDF from an encrypted one.
// Accepts both user and owner passwords; supports RC4 (40/128-bit) and AES (128/256-bit).
func DecryptPDF(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	return manipulate.DecryptPDF(pdfBytes, password, verbose)
}

// EncryptPDF applies AES-128 encryption to a PDF.
// userPassword is required to open; ownerPassword grants full control. Either may be nil.
func EncryptPDF(pdfBytes []byte, userPassword, ownerPassword []byte, verbose bool) ([]byte, error) {
	return manipulate.EncryptPDF(pdfBytes, userPassword, ownerPassword, verbose)
}

// GetPermissions returns the access-control flags of an encrypted PDF.
// For unencrypted PDFs all flags are true.
func GetPermissions(pdfBytes []byte, password []byte) (*Permissions, error) {
	return manipulate.GetPermissions(pdfBytes, password)
}

// --- Document manipulation ---------------------------------------------------

// MergePDFs concatenates multiple PDFs into one. Passwords correspond to each
// input PDF (nil if unencrypted).
func MergePDFs(pdfBytesList [][]byte, passwords [][]byte, verbose bool) ([]byte, error) {
	return manipulate.MergePDFs(pdfBytesList, passwords, verbose)
}

// SplitPDF extracts page ranges from a PDF, returning one output per range.
func SplitPDF(pdfBytes []byte, pageRanges []PageRange, password []byte, verbose bool) ([][]byte, error) {
	return manipulate.SplitPDF(pdfBytes, pageRanges, password, verbose)
}

// SplitPDFByPageCount splits a PDF into chunks of at most pagesPerPDF pages each.
func SplitPDFByPageCount(pdfBytes []byte, pagesPerPDF int, password []byte, verbose bool) ([][]byte, error) {
	return manipulate.SplitPDFByPageCount(pdfBytes, pagesPerPDF, password, verbose)
}

// Redact permanently removes content within the specified regions, covering
// each area with an opaque black rectangle. Content is unrecoverable.
func Redact(pdfBytes []byte, boxes []RedactBox, password []byte) ([]byte, error) {
	return manipulate.Redact(pdfBytes, boxes, password)
}

// Repair rebuilds a PDF into a clean single-revision form: flattens incremental
// updates, removes orphaned xref entries, and rebuilds the xref table from scratch.
func Repair(pdfBytes []byte, password []byte) ([]byte, error) {
	return manipulate.Repair(pdfBytes, password)
}

// Linearize rewrites a PDF for fast web delivery (Fast Web View), reordering
// objects so the catalog and first page appear early in the file.
func Linearize(pdfBytes []byte, password []byte) ([]byte, error) {
	return manipulate.Linearize(pdfBytes, password)
}

// --- Page operations --------------------------------------------------------

// ExtractPages returns a new PDF containing only the specified pages (1-based),
// with all referenced resources fully copied.
func ExtractPages(pdfBytes []byte, pageNumbers []int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.ExtractPages(pdfBytes, pageNumbers, password, verbose)
}

// DeletePage removes a single page from a PDF (1-based page number).
func DeletePage(pdfBytes []byte, pageNumber int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.DeletePage(pdfBytes, pageNumber, password, verbose)
}

// DeletePages removes multiple pages from a PDF (1-based page numbers).
func DeletePages(pdfBytes []byte, pageNumbers []int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.DeletePages(pdfBytes, pageNumbers, password, verbose)
}

// ReorderPages rearranges pages according to order, a 1-based permutation of
// current page numbers. len(order) must equal the page count.
func ReorderPages(pdfBytes []byte, order []int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.ReorderPages(pdfBytes, order, password, verbose)
}

// InsertBlankPage inserts an empty page at position (1-based; 0 = append).
// width and height are in PDF points (1 pt = 1/72 inch).
func InsertBlankPage(pdfBytes []byte, position int, width, height float64, password []byte, verbose bool) ([]byte, error) {
	return manipulate.InsertBlankPage(pdfBytes, position, width, height, password, verbose)
}

// DuplicatePage inserts copies copies of page pageNumber immediately after it (1-based).
func DuplicatePage(pdfBytes []byte, pageNumber, copies int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.DuplicatePage(pdfBytes, pageNumber, copies, password, verbose)
}

// CropPage sets the /CropBox of a page, clipping the visible area.
// rect is [llx lly urx ury] in page coordinates (points).
func CropPage(pdfBytes []byte, pageNumber int, rect [4]float64, password []byte, verbose bool) ([]byte, error) {
	return manipulate.CropPage(pdfBytes, pageNumber, rect, password, verbose)
}

// SetPageSize changes the /MediaBox (physical size) of a page (1-based).
// width and height are in PDF points.
func SetPageSize(pdfBytes []byte, pageNumber int, width, height float64, password []byte, verbose bool) ([]byte, error) {
	return manipulate.SetPageSize(pdfBytes, pageNumber, width, height, password, verbose)
}

// RotatePage rotates a single page by angle degrees (must be 90, 180, or 270).
func RotatePage(pdfBytes []byte, pageNumber, angle int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.RotatePage(pdfBytes, pageNumber, angle, password, verbose)
}

// RotateAllPages rotates every page by angle degrees (must be 90, 180, or 270).
func RotateAllPages(pdfBytes []byte, angle int, password []byte, verbose bool) ([]byte, error) {
	return manipulate.RotateAllPages(pdfBytes, angle, password, verbose)
}

// --- Stamping ----------------------------------------------------------------

// StampText adds a text overlay to one page of a PDF (1-based page number).
func StampText(pdfBytes []byte, pageNumber int, stamp TextStamp, password []byte, verbose bool) ([]byte, error) {
	return manipulate.StampText(pdfBytes, pageNumber, stamp, password, verbose)
}

// StampAllPages adds the same text overlay to every page of a PDF.
func StampAllPages(pdfBytes []byte, stamp TextStamp, password []byte, verbose bool) ([]byte, error) {
	return manipulate.StampAllPages(pdfBytes, stamp, password, verbose)
}

// StampPageNumbers adds automatic page numbers to all pages of a PDF.
func StampPageNumbers(pdfBytes []byte, opts PageNumberOptions, password []byte, verbose bool) ([]byte, error) {
	return manipulate.StampPageNumbers(pdfBytes, opts, password, verbose)
}

// --- Metadata ----------------------------------------------------------------

// GetMetadata extracts document metadata (title, author, dates, etc.) from a PDF.
func GetMetadata(pdfBytes []byte, password []byte, verbose bool) (*DocumentMetadata, error) {
	return extract.GetMetadata(pdfBytes, password, verbose)
}

// SetMetadata writes metadata fields into the PDF /Info dictionary.
// Fields left empty in MetadataUpdate are not modified.
func SetMetadata(pdfBytes []byte, meta MetadataUpdate, password []byte, verbose bool) ([]byte, error) {
	return manipulate.SetMetadata(pdfBytes, meta, password, verbose)
}

// RedactMetadata strips the /Info dictionary and any /Metadata XMP stream,
// removing author, title, creation dates, and other identifying information.
func RedactMetadata(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	return manipulate.RedactMetadata(pdfBytes, password, verbose)
}

// --- Annotations & bookmarks ------------------------------------------------

// AddAnnotation adds an annotation to a page (1-based page number).
func AddAnnotation(pdfBytes []byte, pageNumber int, cfg AnnotationConfig, password []byte, verbose bool) ([]byte, error) {
	return manipulate.AddAnnotation(pdfBytes, pageNumber, cfg, password, verbose)
}

// GetBookmarks returns the document outline (bookmark tree) of a PDF.
func GetBookmarks(pdfBytes []byte, password []byte, verbose bool) ([]Bookmark, error) {
	return extract.GetBookmarks(pdfBytes, password, verbose)
}

// SetBookmarks replaces the entire document outline with the given bookmark tree.
// Pass nil or an empty slice to remove all bookmarks.
func SetBookmarks(pdfBytes []byte, bookmarks []BookmarkEntry, password []byte, verbose bool) ([]byte, error) {
	return manipulate.SetBookmarks(pdfBytes, bookmarks, password, verbose)
}

// --- Digital signatures -----------------------------------------------------

// SignPDF appends a digital signature to pdfBytes using an incremental update.
// The signature covers all bytes except the /Contents hex value.
func SignPDF(pdfBytes []byte, opts SignOptions) ([]byte, error) {
	return sign.SignPDF(pdfBytes, opts)
}

// ValidateSignatures verifies all digital signatures present in a PDF and
// returns one SignatureInfo per signature found.
func ValidateSignatures(pdfBytes []byte) ([]SignatureInfo, error) {
	return sign.ValidateSignatures(pdfBytes)
}

// --- Content extraction -----------------------------------------------------

// ExtractContent extracts text, images, graphics, annotations, bookmarks, and
// metadata from a PDF into a structured ContentDocument.
func ExtractContent(pdfBytes []byte, password []byte, verbose bool) (*ContentDocument, error) {
	return extract.ExtractContent(pdfBytes, password, verbose)
}

// ExtractContentToJSON extracts all PDF content and returns it as a JSON string.
func ExtractContentToJSON(pdfBytes []byte, password []byte, verbose bool) (string, error) {
	return extract.ExtractContentToJSON(pdfBytes, password, verbose)
}

// ExtractAllImages extracts all images from a PDF with their raw pixel data.
func ExtractAllImages(pdfBytes []byte, password []byte, verbose bool) ([]Image, error) {
	return extract.ExtractAllImages(pdfBytes, password, verbose)
}

// ExtractToDirectory extracts all PDF content to a filesystem directory,
// writing images as files and producing a structure JSON manifest.
func ExtractToDirectory(pdfBytes []byte, password []byte, outputDir string, verbose bool) (*DirectoryOutput, error) {
	return extract.ExtractToDirectory(pdfBytes, password, outputDir, verbose)
}

// --- PDF/A compliance -------------------------------------------------------

// ValidatePDFA checks pdfBytes for PDF/A conformance and returns a detailed
// result listing any violations found.
func ValidatePDFA(pdfBytes []byte) PDFAValidationResult {
	return write.ValidatePDFA(pdfBytes)
}

// --- Forms ------------------------------------------------------------------

// DetectForm returns the form type (AcroForm, XFA, or unknown) for a PDF.
func DetectForm(pdfBytes []byte, password []byte, verbose bool) (FormType, error) {
	return forms.Detect(pdfBytes, password, verbose)
}

// ExtractForm parses the form in a PDF and returns a unified Form interface
// that works for both AcroForm and XFA documents.
func ExtractForm(pdfBytes []byte, password []byte, verbose bool) (Form, error) {
	return forms.Extract(pdfBytes, password, verbose)
}

// FlattenForm converts all filled AcroForm widget annotations to static page
// content, removing all interactive form fields from the output PDF.
func FlattenForm(pdfBytes []byte, password []byte, verbose bool) ([]byte, error) {
	return acroform.FlattenForm(pdfBytes, password, verbose)
}

// --- Comparison -------------------------------------------------------------

// DefaultCompareOptions returns sensible comparison defaults.
func DefaultCompareOptions() CompareOptions {
	return compare.DefaultCompareOptions()
}

// ComparePDFs compares two PDFs and returns a structured diff covering
// metadata, page content, text, images, annotations, and form fields.
func ComparePDFs(pdf1, pdf2 []byte, password1, password2 []byte, verbose bool) (*ComparisonResult, error) {
	return compare.ComparePDFs(pdf1, pdf2, password1, password2, verbose)
}

// ComparePDFsWithOptions compares two PDFs with custom comparison options.
func ComparePDFsWithOptions(pdf1, pdf2 []byte, password1, password2 []byte, opts CompareOptions) (*ComparisonResult, error) {
	return compare.ComparePDFsWithOptions(pdf1, pdf2, password1, password2, opts)
}

// CompareReport generates a human-readable text report from a ComparisonResult.
func CompareReport(result *ComparisonResult) string {
	return compare.GenerateReport(result)
}

// CompareReportJSON generates a JSON report from a ComparisonResult.
func CompareReportJSON(result *ComparisonResult) (string, error) {
	return compare.GenerateJSONReport(result)
}

// --- Image manipulation -----------------------------------------------------

// ReplaceImage replaces an image XObject in a PDF.
//
// imageRef identifies the image by its XObject resource name (e.g. "Im1") or
// by the raw object number as a decimal string (e.g. "7").
//
// format must be "jpeg", "png", or "raw":
//   - "jpeg": newImageData is a JPEG file; stored with /Filter/DCTDecode.
//   - "png":  newImageData is a PNG file; decoded and stored as deflate-compressed RGB.
//   - "raw":  stream bytes replaced as-is; existing filter preserved.
func ReplaceImage(pdfBytes []byte, imageRef string, newImageData []byte, format string, password []byte, verbose bool) ([]byte, error) {
	return manipulate.ReplaceImage(pdfBytes, imageRef, newImageData, format, password, verbose)
}

// --- PDF/A conversion -------------------------------------------------------

// ConvertToPDFA converts a PDF to PDF/A format.
//
// conformance selects the conformance level: "1b" (default), "2b", or "3b".
// Encrypted PDFs are decrypted before conversion (PDF/A prohibits encryption).
func ConvertToPDFA(pdfBytes []byte, password []byte, conformance string) ([]byte, error) {
	return manipulate.ConvertToPDFA(pdfBytes, password, conformance)
}
