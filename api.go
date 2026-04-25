package pdfer

import (
	"github.com/benedoc-inc/pdfer/core/compare"
	"github.com/benedoc-inc/pdfer/core/encrypt"
	"github.com/benedoc-inc/pdfer/core/manipulate"
	"github.com/benedoc-inc/pdfer/forms"
	"github.com/benedoc-inc/pdfer/forms/acroform"
)

// --- Type aliases for sub-package types that appear in the top-level API ----

// RedactBox specifies a rectangular region on a page to permanently redact.
type RedactBox = manipulate.RedactBox

// PageRange specifies an inclusive page range for splitting.
type PageRange = manipulate.PageRange

// FormType distinguishes AcroForm from XFA.
type FormType = forms.FormType

// Form is the unified interface returned by ExtractForm.
type Form = forms.Form

const (
	FormTypeAcroForm FormType = "acroform"
	FormTypeXFA      FormType = "xfa"
	FormTypeUnknown  FormType = "unknown"
)

// --- Encryption / decryption -------------------------------------------------

// DecryptPDF decrypts an encrypted PDF using the provided password.
// It accepts both user and owner passwords and supports RC4 (40/128-bit) and
// AES (128/256-bit). Returns the decrypted bytes and the encryption parameters.
func DecryptPDF(pdfBytes []byte, password []byte, verbose bool) ([]byte, *Encryption, error) {
	return encrypt.DecryptPDF(pdfBytes, password, verbose)
}

// EncryptPDF applies AES-128 encryption to an existing unencrypted PDF.
// userPassword is required to open the document; ownerPassword grants full
// control. Either may be nil (no password for that role).
func EncryptPDF(pdfBytes []byte, userPassword, ownerPassword []byte, verbose bool) ([]byte, error) {
	return manipulate.EncryptPDF(pdfBytes, userPassword, ownerPassword, verbose)
}

// --- Document manipulation ---------------------------------------------------

// MergePDFs concatenates multiple PDFs into a single document. Passwords
// correspond to each input PDF (nil if unencrypted).
func MergePDFs(pdfBytesList [][]byte, passwords [][]byte, verbose bool) ([]byte, error) {
	return manipulate.MergePDFs(pdfBytesList, passwords, verbose)
}

// SplitPDF extracts page ranges from a PDF, returning one output PDF per range.
func SplitPDF(pdfBytes []byte, pageRanges []PageRange, password []byte, verbose bool) ([][]byte, error) {
	return manipulate.SplitPDF(pdfBytes, pageRanges, password, verbose)
}

// SplitPDFByPageCount splits a PDF into chunks of at most pagesPerPDF pages each.
func SplitPDFByPageCount(pdfBytes []byte, pagesPerPDF int, password []byte, verbose bool) ([][]byte, error) {
	return manipulate.SplitPDFByPageCount(pdfBytes, pagesPerPDF, password, verbose)
}

// Redact permanently removes content within the specified regions and covers
// each area with an opaque black rectangle. Content is unrecoverable.
func Redact(pdfBytes []byte, boxes []RedactBox, password []byte) ([]byte, error) {
	return manipulate.Redact(pdfBytes, boxes, password)
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

// ComparisonResult holds the full diff between two PDFs.
type ComparisonResult = compare.ComparisonResult

// CompareOptions configures PDF comparison behaviour.
type CompareOptions = compare.CompareOptions

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
