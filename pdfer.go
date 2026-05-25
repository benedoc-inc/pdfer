// Package pdfer provides pure Go PDF processing with zero external dependencies.
//
// # Common operations
//
// Encrypt, decrypt, and permissions:
//
//	out, err := pdfer.EncryptPDF(pdfBytes, []byte("password"), nil, false)
//	out, err := pdfer.DecryptPDF(pdfBytes, []byte("password"), false)
//	perms, err := pdfer.GetPermissions(pdfBytes, []byte("password"))
//
// Merge, split, extract, reorder, insert, delete:
//
//	out, err := pdfer.MergePDFs([][]byte{a, b}, nil, false)
//	parts, err := pdfer.SplitPDF(pdfBytes, []pdfer.PageRange{{1, 3}, {4, 6}}, nil, false)
//	out, err := pdfer.ExtractPages(pdfBytes, []int{1, 3, 5}, nil, false)
//	out, err := pdfer.ReorderPages(pdfBytes, []int{3, 1, 2}, nil, false)
//	out, err := pdfer.InsertBlankPage(pdfBytes, 2, 612, 792, nil, false)
//	out, err := pdfer.DuplicatePage(pdfBytes, 1, 2, nil, false)
//	out, err := pdfer.DeletePage(pdfBytes, 2, nil, false)
//
// Redact, repair, linearize, rotate, crop, stamp:
//
//	out, err := pdfer.Redact(pdfBytes, []pdfer.RedactBox{{Page: 1, Rect: [4]float64{50, 680, 200, 720}}}, nil)
//	out, err := pdfer.Repair(pdfBytes, nil)
//	out, err := pdfer.Linearize(pdfBytes, nil)
//	out, err := pdfer.RotatePage(pdfBytes, 1, 90, nil, false)
//	out, err := pdfer.CropPage(pdfBytes, 1, [4]float64{36, 36, 576, 756}, nil, false)
//	out, err := pdfer.SetPageSize(pdfBytes, 1, 612, 792, nil, false)
//	out, err := pdfer.StampAllPages(pdfBytes, pdfer.TextStamp{Text: "DRAFT", X: 72, Y: 72}, nil, false)
//	out, err := pdfer.StampPageNumbers(pdfBytes, pdfer.PageNumberOptions{Position: pdfer.BottomCenter}, nil, false)
//
// Metadata:
//
//	meta, err := pdfer.GetMetadata(pdfBytes, nil, false)
//	out, err := pdfer.SetMetadata(pdfBytes, pdfer.MetadataUpdate{Title: "Report", Author: "Alice"}, nil, false)
//	out, err := pdfer.RedactMetadata(pdfBytes, nil, false)
//
// Annotations and bookmarks:
//
//	out, err := pdfer.AddAnnotation(pdfBytes, 1, pdfer.AnnotationConfig{
//	    Type: pdfer.AnnotLink, Rect: [4]float64{72, 700, 200, 720}, URI: "https://example.com",
//	}, nil, false)
//	bmarks, err := pdfer.GetBookmarks(pdfBytes, nil, false)
//	out, err := pdfer.SetBookmarks(pdfBytes, []pdfer.BookmarkEntry{{Title: "Intro", Page: 1}}, nil, false)
//
// Digital signatures:
//
//	out, err := pdfer.SignPDF(pdfBytes, pdfer.SignOptions{Certificate: cert, PrivateKey: key})
//	sigs, err := pdfer.ValidateSignatures(pdfBytes)
//
// Fill and flatten forms (AcroForm and XFA auto-detected):
//
//	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
//	out, err := form.Fill(pdfBytes, pdfer.FormData{"name": "Alice"}, nil, false)
//	out, err = pdfer.FlattenForm(out, nil, false)
//
// Extract content, images, compare, and validate PDF/A:
//
//	doc, err := pdfer.ExtractContent(pdfBytes, nil, false)
//	imgs, err := pdfer.ExtractAllImages(pdfBytes, nil, false)
//	result, err := pdfer.ComparePDFs(pdf1, pdf2, nil, nil, false)
//	vr := pdfer.ValidatePDFA(pdfBytes)
//
// # Sub-packages
//
// Import sub-packages directly for lower-level control:
//
//   - core/encrypt    — RC4/AES decryption and AES-128/256 encryption primitives
//   - core/parse      — PDF structure parsing (xref, objects, streams)
//   - core/write      — PDF generation (SimplePDFBuilder, PDFWriter, ValidatePDFA)
//   - core/manipulate — document-level operations (all page/metadata/annotation ops)
//   - core/sign       — PDF digital signatures (create and validate PKCS#7/CMS)
//   - core/compare    — visual and structural PDF diffing
//   - forms/acroform  — AcroForm parsing, filling, appearance streams
//   - forms/xfa       — XFA stream extraction and dataset updating
//   - content/extract — text, image, annotation and metadata extraction
package pdfer

import (
	"runtime/debug"

	"github.com/benedoc-inc/pdfer/types"
)

// Re-export common types for convenience.
// Users can import just "github.com/benedoc-inc/pdfer" for basic usage.

// Encryption holds PDF encryption parameters and derived keys.
type Encryption = types.PDFEncryption

// FormSchema represents a parsed XFA form structure.
type FormSchema = types.FormSchema

// Question represents a single form field.
type Question = types.Question

// Rule represents a validation or calculation rule.
type Rule = types.Rule

// FormData is a map of field names to values for form filling.
type FormData = types.FormData

// XFADatasets represents parsed XFA datasets.
type XFADatasets = types.XFADatasets

// XFAConfig represents parsed XFA configuration.
type XFAConfig = types.XFAConfig

// XFALocaleSet represents parsed XFA localization data.
type XFALocaleSet = types.XFALocaleSet

const modulePath = "github.com/benedoc-inc/pdfer"

// Version returns the library version, derived from Go module metadata.
//
// When consumed via `go get` or `go install`, this returns the resolved
// module version (e.g., "v1.10.0"). When built from a working copy, it
// returns "(devel)". Falls back to "(unknown)" if build info is unavailable.
//
// Versioning is driven by git tags through the release workflow; no manual
// string updates are required.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	if info.Main.Path == modulePath {
		return info.Main.Version
	}
	for _, dep := range info.Deps {
		if dep != nil && dep.Path == modulePath {
			return dep.Version
		}
	}
	return "(unknown)"
}
