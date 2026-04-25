package write

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/core/parse"
)

// FixPDFA attempts to repair common PDF/A conformance violations and return a
// PDF/A-2b conformant document. It rebuilds the PDF into a single revision,
// then adds or replaces:
//   - trailer /ID (§6.7.3)
//   - catalog /Metadata XMP stream with pdfaid identifiers (§6.7.2)
//   - catalog /OutputIntents with an sRGB ICC profile (§6.2.2)
//
// It also strips /JavaScript and /JS catalog entries (§6.6.1).
//
// UNEMBEDDED_FONT violations cannot be fixed automatically; those fonts will
// still appear in the repaired file. Supply a password for encrypted source
// files; the output will be unencrypted (PDF/A cannot be encrypted).
func FixPDFA(pdfBytes []byte, password []byte) ([]byte, error) {
	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{Password: password})
	if err != nil {
		return nil, fmt.Errorf("fixpdfa: parse failed: %w", err)
	}

	result := ValidatePDFA(pdfBytes)
	if result.Conformant {
		return pdfBytes, nil
	}

	needXMP := false
	needOutputIntent := false
	needStripJS := false
	for _, v := range result.Violations {
		switch v.Code {
		case ViolEncrypted:
			// We can still proceed — parse above succeeded with password, and we
			// output unencrypted, which is what PDF/A requires.
		case ViolMissingXMP, ViolMissingPDFAID:
			needXMP = true
		case ViolMissingOutputIntent, ViolMissingDestProfile:
			needOutputIntent = true
		case ViolForbiddenAction:
			needStripJS = true
		}
	}

	trailer := pdf.Trailer()
	if trailer == nil || trailer.RootRef == "" {
		return nil, fmt.Errorf("fixpdfa: no document catalog in trailer")
	}
	var rootNum int
	fmt.Sscanf(trailer.RootRef, "%d 0 R", &rootNum)

	// Copy all objects into a fresh writer (flattens incremental revisions).
	w := NewPDFWriter()
	for _, n := range pdf.Objects() {
		body, objErr := pdf.GetObjectContent(n)
		if objErr != nil {
			continue
		}
		w.SetObject(n, body)
	}

	// Set trailer references.
	w.SetRoot(rootNum)
	if trailer.InfoRef != "" {
		var infoNum int
		fmt.Sscanf(trailer.InfoRef, "%d 0 R", &infoNum)
		w.SetInfo(infoNum)
	}

	// preparePDFA allocates XMP metadata, sRGB ICC stream, and OutputIntent
	// objects; it also sets /ID when none exists.
	metaObjNum, outputIntentObjNum, err := w.preparePDFA(PDFAOptions{Conformance: PDFA2b}, time.Now())
	if err != nil {
		return nil, fmt.Errorf("fixpdfa: prepare failed: %w", err)
	}

	// Patch the catalog to reference the newly created objects.
	catalogBody, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		return nil, fmt.Errorf("fixpdfa: cannot read catalog: %w", err)
	}
	cat := string(catalogBody)

	if needXMP {
		cat = pdfaRemoveCatalogRef(cat, "Metadata")
		cat = pdfaInjectEntry(cat, fmt.Sprintf("/Metadata %d 0 R", metaObjNum))
	}
	if needOutputIntent {
		cat = pdfaRemoveCatalogArray(cat, "OutputIntents")
		cat = pdfaInjectEntry(cat, fmt.Sprintf("/OutputIntents [%d 0 R]", outputIntentObjNum))
	}
	if needStripJS {
		cat = pdfaStripJS(cat)
	}

	w.SetObject(rootNum, []byte(cat))

	return w.Bytes()
}

// pdfaRemoveCatalogRef removes "/Key N G R" from a catalog dict body.
func pdfaRemoveCatalogRef(cat, key string) string {
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s+\d+\s+\d+\s+R`)
	return re.ReplaceAllString(cat, "")
}

// pdfaRemoveCatalogArray removes "/Key [...]" from a catalog dict body.
func pdfaRemoveCatalogArray(cat, key string) string {
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s*\[[^\]]*\]`)
	return re.ReplaceAllString(cat, "")
}

// pdfaInjectEntry inserts entry (already formatted as "/Key value") just before
// the closing ">>" of the outermost dict.
func pdfaInjectEntry(cat, entry string) string {
	end := strings.LastIndex(cat, ">>")
	if end == -1 {
		return cat
	}
	return cat[:end] + entry + cat[end:]
}

// pdfaStripJS removes /JavaScript and /JS entries from a catalog dict body.
func pdfaStripJS(cat string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`/JavaScript\s+\d+\s+\d+\s+R`),
		regexp.MustCompile(`/JavaScript\s*<<[^>]*>>`),
		regexp.MustCompile(`/JS\s*\([^)]*\)`),
		regexp.MustCompile(`/JS\s+\d+\s+\d+\s+R`),
	}
	for _, re := range patterns {
		cat = re.ReplaceAllString(cat, "")
	}
	return cat
}
