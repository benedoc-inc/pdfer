package write

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
)

// PDFAValidationResult contains the outcome of a PDF/A conformance check.
type PDFAValidationResult struct {
	Conformant       bool            // true when no violations found
	Part             int             // PDF/A part number (1, 2, or 3); 0 if not identified
	ConformanceLevel string          // "B", "A", or "U"; empty if not identified
	Violations       []PDFAViolation // every spec violation found
}

// PDFAViolationCode identifies the category of a PDF/A violation.
type PDFAViolationCode string

const (
	ViolEncrypted           PDFAViolationCode = "ENCRYPTED"
	ViolMissingDocumentID   PDFAViolationCode = "MISSING_DOCUMENT_ID"
	ViolMissingXMP          PDFAViolationCode = "MISSING_XMP"
	ViolMissingPDFAID       PDFAViolationCode = "MISSING_PDFAID"
	ViolMissingOutputIntent PDFAViolationCode = "MISSING_OUTPUT_INTENT"
	ViolMissingDestProfile  PDFAViolationCode = "MISSING_DEST_PROFILE"
	ViolUnembeddedFont      PDFAViolationCode = "UNEMBEDDED_FONT"
	ViolForbiddenAction     PDFAViolationCode = "FORBIDDEN_ACTION"
)

// PDFAViolation describes a single PDF/A conformance failure.
type PDFAViolation struct {
	Code    PDFAViolationCode
	Message string
}

// ValidatePDFA checks pdfBytes for PDF/A conformance and returns a detailed
// result. It verifies:
//   - no encryption (§6.1.3)
//   - trailer /ID array present (§6.7.3)
//   - catalog /Metadata XMP stream with pdfaid:part/conformance (§6.7.2)
//   - /OutputIntents with at least one /DestOutputProfile ICC stream (§6.2.2)
//   - no unembedded fonts (heuristic: Type1/TrueType without /FontFile* flagged)
//   - no JavaScript actions (§6.6.1)
//
// Font-embedding detection is heuristic; it reports standard Type1 fonts
// (Helvetica, Times-Roman, etc.) used without embedded programs as violations.
func ValidatePDFA(pdfBytes []byte) PDFAValidationResult {
	var result PDFAValidationResult
	var viols []PDFAViolation
	add := func(code PDFAViolationCode, msg string) {
		viols = append(viols, PDFAViolation{Code: code, Message: msg})
	}

	pdf, err := parse.Open(pdfBytes)
	if err != nil {
		add(ViolEncrypted, "cannot parse PDF (may be encrypted or malformed): "+err.Error())
		result.Violations = viols
		return result
	}

	// Encryption
	if pdf.IsEncrypted() {
		add(ViolEncrypted, "PDF/A documents must not be encrypted (ISO 19005 §6.1.3)")
	}

	// Trailer /ID — the parse API does not populate IDArray, so search the raw bytes.
	trailer := pdf.Trailer()
	idRE := regexp.MustCompile(`/ID\s*\[`)
	if !idRE.Match(pdfBytes) {
		add(ViolMissingDocumentID, "trailer /ID array is absent (ISO 19005 §6.7.3)")
	}

	if trailer == nil || trailer.RootRef == "" {
		result.Violations = viols
		result.Conformant = len(viols) == 0
		return result
	}

	// Catalog
	var rootNum int
	fmt.Sscanf(trailer.RootRef, "%d 0 R", &rootNum)
	catalogBytes, err := pdf.GetObjectContent(rootNum)
	if err != nil {
		add(ViolMissingXMP, "cannot read catalog object")
		result.Violations = viols
		result.Conformant = len(viols) == 0
		return result
	}
	catalog := string(catalogBytes)

	// XMP metadata
	metaNum := pdfaExtractRef(catalog, "Metadata")
	if metaNum == 0 {
		add(ViolMissingXMP, "catalog /Metadata entry absent (ISO 19005 §6.7.2)")
	} else {
		xmpData, err := pdfaExtractStreamContent(pdf, metaNum)
		if err != nil || len(xmpData) == 0 {
			add(ViolMissingXMP, "cannot read /Metadata stream")
		} else {
			part, level, ok := pdfaParsePDFAID(xmpData)
			if !ok {
				add(ViolMissingPDFAID, "XMP stream missing pdfaid:part and/or pdfaid:conformance (ISO 19005 §6.7.2)")
			} else {
				result.Part = part
				result.ConformanceLevel = level
			}
		}
	}

	// OutputIntents
	if !strings.Contains(catalog, "/OutputIntents") {
		add(ViolMissingOutputIntent, "catalog /OutputIntents absent (ISO 19005 §6.2.2)")
	} else {
		refs := pdfaExtractArrayRefs(catalog, "OutputIntents")
		hasProfile := false
		for _, ref := range refs {
			body, err := pdf.GetObjectContent(ref)
			if err == nil && strings.Contains(string(body), "/DestOutputProfile") {
				hasProfile = true
				break
			}
		}
		if !hasProfile {
			add(ViolMissingDestProfile, "no OutputIntent carries a /DestOutputProfile ICC profile")
		}
	}

	// Font embedding
	pdfaCheckFonts(pdf, &viols)

	// Forbidden actions (JavaScript)
	pdfaCheckActions(catalog, &viols)

	result.Violations = viols
	result.Conformant = len(viols) == 0
	return result
}

// pdfaExtractRef finds the first /Key N 0 R reference in content and returns N.
func pdfaExtractRef(content, key string) int {
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s+(\d+)\s+\d+\s+R`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// pdfaExtractArrayRefs finds /Key[N1 G R N2 G R ...] and returns the object numbers.
func pdfaExtractArrayRefs(content, key string) []int {
	// Match /Key [ ... ] tolerating whitespace.
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s*\[([^\]]*)\]`)
	m := re.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	numRE := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	matches := numRE.FindAllStringSubmatch(m[1], -1)
	out := make([]int, 0, len(matches))
	for _, mm := range matches {
		n, _ := strconv.Atoi(mm[1])
		out = append(out, n)
	}
	return out
}

// pdfaExtractStreamContent returns the raw (uncompressed) bytes of a stream
// object. XMP streams must not be compressed per PDF/A; other streams may be.
func pdfaExtractStreamContent(pdf *parse.PDF, objNum int) ([]byte, error) {
	obj, err := pdf.GetObject(objNum)
	if err != nil {
		return nil, err
	}
	s := string(obj)
	start := strings.Index(s, "stream")
	if start == -1 {
		return nil, fmt.Errorf("object %d has no stream", objNum)
	}
	start += len("stream")
	if start < len(s) && s[start] == '\r' {
		start++
	}
	if start < len(s) && s[start] == '\n' {
		start++
	}
	end := strings.LastIndex(s, "endstream")
	if end == -1 || end <= start {
		return nil, fmt.Errorf("object %d: endstream not found", objNum)
	}
	// Trim trailing whitespace before endstream.
	data := s[start:end]
	for len(data) > 0 && (data[len(data)-1] == '\n' || data[len(data)-1] == '\r') {
		data = data[:len(data)-1]
	}
	return []byte(data), nil
}

// pdfaParsePDFAID extracts pdfaid:part and pdfaid:conformance from XMP bytes.
func pdfaParsePDFAID(xmp []byte) (part int, level string, ok bool) {
	xmpStr := string(xmp)
	// Match either <pdfaid:part>N</pdfaid:part> or pdfaid:part="N"
	partRE := regexp.MustCompile(`(?i)<pdfaid:part>\s*(\d+)\s*</pdfaid:part>|pdfaid:part\s*=\s*"(\d+)"`)
	if m := partRE.FindStringSubmatch(xmpStr); m != nil {
		val := m[1]
		if val == "" {
			val = m[2]
		}
		part, _ = strconv.Atoi(val)
	}
	// Match <pdfaid:conformance>B</pdfaid:conformance> or pdfaid:conformance="B"
	confRE := regexp.MustCompile(`(?i)<pdfaid:conformance>\s*([A-Za-z])\s*</pdfaid:conformance>|pdfaid:conformance\s*=\s*"([A-Za-z])"`)
	if m := confRE.FindStringSubmatch(xmpStr); m != nil {
		level = strings.ToUpper(m[1])
		if level == "" {
			level = strings.ToUpper(m[2])
		}
	}
	ok = part > 0 && level != ""
	return part, level, ok
}

// pdfaCheckFonts scans all font objects in the PDF and flags any Type1 or
// TrueType font without an embedded font program (/FontFile, /FontFile2,
// /FontFile3 in its FontDescriptor).
func pdfaCheckFonts(pdf *parse.PDF, viols *[]PDFAViolation) {
	// Standard 14 Type1 fonts — never embedded, always a violation in PDF/A.
	standard14 := map[string]bool{
		"Helvetica": true, "Helvetica-Bold": true, "Helvetica-Oblique": true,
		"Helvetica-BoldOblique": true, "Times-Roman": true, "Times-Bold": true,
		"Times-Italic": true, "Times-BoldItalic": true, "Courier": true,
		"Courier-Bold": true, "Courier-Oblique": true, "Courier-BoldOblique": true,
		"Symbol": true, "ZapfDingbats": true,
	}

	fontBaseRE := regexp.MustCompile(`/BaseFont\s*/([^\s/<>\[\]()]+)`)
	subtypeRE := regexp.MustCompile(`/Subtype\s*/(\w+)`)
	fontFileRE := regexp.MustCompile(`/FontFile[23]?\s+\d+\s+\d+\s+R`)
	descriptorRE := regexp.MustCompile(`/FontDescriptor\s+(\d+)\s+\d+\s+R`)

	reported := map[string]bool{}

	for _, objNum := range pdf.Objects() {
		body, err := pdf.GetObjectContent(objNum)
		if err != nil {
			continue
		}
		s := string(body)
		if !strings.Contains(s, "/Type/Font") && !strings.Contains(s, "/Type /Font") {
			continue
		}

		// Only check Type1 and TrueType (not Type0/CIDFont composite fonts which
		// reference embedded sub-fonts separately).
		sm := subtypeRE.FindStringSubmatch(s)
		if sm == nil {
			continue
		}
		subtype := sm[1]
		if subtype != "Type1" && subtype != "TrueType" && subtype != "MMType1" {
			continue
		}

		bm := fontBaseRE.FindStringSubmatch(s)
		baseName := ""
		if bm != nil {
			baseName = bm[1]
		}

		// Standard 14 fonts are never embedded.
		if standard14[baseName] {
			if !reported[baseName] {
				reported[baseName] = true
				*viols = append(*viols, PDFAViolation{
					Code:    ViolUnembeddedFont,
					Message: fmt.Sprintf("standard Type1 font %q has no embedded program (ISO 19005 §6.3.3)", baseName),
				})
			}
			continue
		}

		// Non-standard font: check FontDescriptor for /FontFile*.
		dm := descriptorRE.FindStringSubmatch(s)
		if dm == nil {
			continue // no descriptor — assume composite, skip
		}
		descNum, _ := strconv.Atoi(dm[1])
		descBody, err := pdf.GetObjectContent(descNum)
		if err != nil {
			continue
		}
		if !fontFileRE.MatchString(string(descBody)) {
			name := baseName
			if name == "" {
				name = fmt.Sprintf("obj %d", objNum)
			}
			if !reported[name] {
				reported[name] = true
				*viols = append(*viols, PDFAViolation{
					Code:    ViolUnembeddedFont,
					Message: fmt.Sprintf("font %q lacks /FontFile* in its FontDescriptor (ISO 19005 §6.3.3)", name),
				})
			}
		}
	}
}

// pdfaCheckActions looks for JavaScript-related entries that are forbidden in PDF/A.
func pdfaCheckActions(catalog string, viols *[]PDFAViolation) {
	// /AA (additional actions) and /OpenAction with /S/JavaScript are forbidden.
	// A simple heuristic: flag any /JavaScript or /JS key in the document.
	if strings.Contains(catalog, "/JavaScript") || strings.Contains(catalog, "/JS ") || strings.Contains(catalog, "/JS\n") || strings.Contains(catalog, "/JS\r") {
		*viols = append(*viols, PDFAViolation{
			Code:    ViolForbiddenAction,
			Message: "document catalog contains /JavaScript or /JS action (ISO 19005 §6.6.1)",
		})
	}
}
