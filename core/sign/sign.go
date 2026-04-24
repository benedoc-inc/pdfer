package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/benedoc-inc/pdfer/core/parse"
)

const (
	// byteRangePlaceholder is exactly 45 bytes — overwritten in-place after the
	// file is assembled and the exact /Contents position is known.
	byteRangePlaceholder = "[0000000000 0000000000 0000000000 0000000000]"

	// reservedSigBytes is the number of bytes reserved for the PKCS#7 DER
	// encoding (hex-encoded → reservedSigBytes*2 characters in /Contents).
	reservedSigBytes = 8192
)

// SignOptions configures PDF digital signature creation.
type SignOptions struct {
	Certificate *x509.Certificate
	PrivateKey  crypto.Signer

	// Optional metadata embedded in the signature dictionary.
	Reason   string
	Location string

	// FieldName is the PDF field name for the signature widget. Defaults to "Signature1".
	FieldName string

	// Page is the 0-indexed page the invisible widget is wired to. Defaults to 0.
	Page int
}

// SignPDF appends a digital signature to pdfBytes using an incremental update.
//
// The signature covers all bytes of the file except the /Contents hex value,
// as required by the PDF ByteRange mechanism (ISO 32000 §12.8.1).
// Original bytes are never modified.
func SignPDF(pdfBytes []byte, opts SignOptions) ([]byte, error) {
	if opts.Certificate == nil || opts.PrivateKey == nil {
		return nil, fmt.Errorf("Certificate and PrivateKey are required")
	}
	if opts.FieldName == "" {
		opts.FieldName = "Signature1"
	}

	pdf, err := parse.OpenWithOptions(pdfBytes, parse.ParseOptions{})
	if err != nil {
		return nil, fmt.Errorf("parse PDF: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("could not read PDF trailer")
	}

	prevXRef := findLastStartXRef(pdfBytes)
	if prevXRef < 0 {
		return nil, fmt.Errorf("could not locate startxref in PDF")
	}

	// ---- Discover existing structures ----
	catalogObjNum, catalogData, err := getCatalog(pdf)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	pageObjNum, pageData, err := getPage(pdf, catalogData, opts.Page)
	if err != nil {
		return nil, fmt.Errorf("find page %d: %w", opts.Page, err)
	}
	existingAcroFormObjNum, existingAcroFormData, acroFormExists := findAcroForm(catalogData, pdf)

	// ---- Assign fresh object numbers ----
	nextObjNum := trailer.Size
	sigDictObjNum := nextObjNum
	nextObjNum++
	widgetObjNum := nextObjNum
	nextObjNum++
	var newAcroFormObjNum int
	if !acroFormExists {
		newAcroFormObjNum = nextObjNum
		nextObjNum++
	}

	signingTime := time.Now()

	// ---- Build incremental update ----
	var buf bytes.Buffer
	buf.Write(pdfBytes)
	buf.WriteByte('\n')

	offsets := make(map[int]int64)

	// --- SigDict object (ByteRange and /Contents are placeholders) ---
	var sigDictBuf bytes.Buffer
	sigDictBuf.WriteString("<<")
	sigDictBuf.WriteString("/Type/Sig")
	sigDictBuf.WriteString("/Filter/Adobe.PPKLite")
	sigDictBuf.WriteString("/SubFilter/adbe.pkcs7.detached")
	sigDictBuf.WriteString("/ByteRange ")
	byteRangeRelPos := sigDictBuf.Len() // position of '[' within sigDictBuf
	sigDictBuf.WriteString(byteRangePlaceholder)
	sigDictBuf.WriteString("/Contents <")
	contentsHexRelPos := sigDictBuf.Len() // position of first hex char
	sigDictBuf.WriteString(strings.Repeat("0", reservedSigBytes*2))
	contentsHexEndRelPos := sigDictBuf.Len() // position just past last hex char (= position of '>')
	sigDictBuf.WriteByte('>')
	if opts.Reason != "" {
		fmt.Fprintf(&sigDictBuf, "/Reason (%s)", escapePDFStr(opts.Reason))
	}
	if opts.Location != "" {
		fmt.Fprintf(&sigDictBuf, "/Location (%s)", escapePDFStr(opts.Location))
	}
	fmt.Fprintf(&sigDictBuf, "/M (%s)", formatPDFDate(signingTime))
	sigDictBuf.WriteString(">>")

	sigDictObjHeader := fmt.Sprintf("%d 0 obj\n", sigDictObjNum)
	// Compute absolute byte positions before writing anything.
	sigDictAbsBase := int64(buf.Len()) + int64(len(sigDictObjHeader))
	byteRangeAbsPos := int(sigDictAbsBase) + byteRangeRelPos
	contentsHexAbsPos := int(sigDictAbsBase) + contentsHexRelPos
	contentsHexEndAbsPos := int(sigDictAbsBase) + contentsHexEndRelPos

	offsets[sigDictObjNum] = int64(buf.Len())
	buf.WriteString(sigDictObjHeader)
	buf.Write(sigDictBuf.Bytes())
	buf.WriteString("\nendobj\n")

	// --- Widget annotation (invisible: /Rect [0 0 0 0]) ---
	widgetDict := fmt.Sprintf(
		"<</Type/Annot/Subtype/Widget/FT/Sig/Rect[0 0 0 0]/F 132/T(%s)/V %d 0 R/P %d 0 R>>",
		escapePDFStr(opts.FieldName), sigDictObjNum, pageObjNum,
	)
	offsets[widgetObjNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n", widgetObjNum)
	buf.WriteString(widgetDict)
	buf.WriteString("\nendobj\n")

	// --- Updated page dict (widget added to /Annots) ---
	// pageData is body-only (from GetObjectContent); write with explicit header.
	offsets[pageObjNum] = int64(buf.Len())
	fmt.Fprintf(&buf, "%d 0 obj\n", pageObjNum)
	buf.Write(addToAnnots(pageData, widgetObjNum))
	buf.WriteString("\nendobj\n")

	// --- AcroForm ---
	if acroFormExists {
		offsets[existingAcroFormObjNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", existingAcroFormObjNum)
		buf.Write(addSigFieldToAcroForm(existingAcroFormData, widgetObjNum))
		buf.WriteString("\nendobj\n")
	} else {
		// New AcroForm — created from scratch.
		newAcroForm := fmt.Sprintf("<</Fields[%d 0 R]/SigFlags 3>>", widgetObjNum)
		offsets[newAcroFormObjNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", newAcroFormObjNum)
		buf.WriteString(newAcroForm)
		buf.WriteString("\nendobj\n")

		// Updated catalog — body-only from GetObjectContent.
		offsets[catalogObjNum] = int64(buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n", catalogObjNum)
		buf.Write(setCatalogAcroForm(catalogData, newAcroFormObjNum))
		buf.WriteString("\nendobj\n")
	}

	// --- xref + trailer ---
	xrefStart := int64(buf.Len())
	buf.WriteString("xref\n")
	objNums := make([]int, 0, len(offsets))
	for n := range offsets {
		objNums = append(objNums, n)
	}
	sort.Ints(objNums)
	writeXRefSubsections(&buf, offsets, objNums)

	buf.WriteString("trailer\n<<")
	fmt.Fprintf(&buf, "/Size %d", nextObjNum)
	fmt.Fprintf(&buf, "/Root %s", trailer.RootRef)
	if trailer.InfoRef != "" {
		fmt.Fprintf(&buf, "/Info %s", trailer.InfoRef)
	}
	fmt.Fprintf(&buf, "/Prev %d", prevXRef)
	buf.WriteString(">>\n")
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefStart)

	// ---- Finalise ByteRange and /Contents in-place ----
	result := buf.Bytes()
	fileLen := len(result)

	// '<' is at contentsHexAbsPos-1; '>' is at contentsHexEndAbsPos.
	posLt := contentsHexAbsPos - 1
	posGt := contentsHexEndAbsPos

	byteRange := fmt.Sprintf("[%010d %010d %010d %010d]",
		0, posLt, posGt+1, fileLen-(posGt+1))
	if len(byteRange) != len(byteRangePlaceholder) {
		return nil, fmt.Errorf("ByteRange string length mismatch: %d != %d", len(byteRange), len(byteRangePlaceholder))
	}
	copy(result[byteRangeAbsPos:], byteRange)

	// Hash the two signed byte ranges (everything except /Contents hex).
	h := sha256.New()
	h.Write(result[0:posLt])
	h.Write(result[posGt+1:])
	digest := h.Sum(nil)

	pkcs7Bytes, err := buildDetachedPKCS7(opts.Certificate, opts.PrivateKey, digest, signingTime)
	if err != nil {
		return nil, fmt.Errorf("build PKCS#7: %w", err)
	}
	if len(pkcs7Bytes) > reservedSigBytes {
		return nil, fmt.Errorf("PKCS#7 size %d exceeds reserved %d bytes", len(pkcs7Bytes), reservedSigBytes)
	}

	hexSig := strings.ToUpper(hex.EncodeToString(pkcs7Bytes))
	hexSig += strings.Repeat("0", reservedSigBytes*2-len(hexSig))
	copy(result[contentsHexAbsPos:], hexSig)

	return result, nil
}

// GenerateTestCredentials creates a self-signed RSA-2048 certificate and key
// suitable for unit tests. Not for production use.
func GenerateTestCredentials() (*rsa.PrivateKey, *x509.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "pdfer test signer",
			Organization: []string{"pdfer"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parse cert: %w", err)
	}
	return key, cert, nil
}

// GenerateTestCredentialsECDSA creates a self-signed ECDSA P-256 certificate
// and key suitable for unit tests. Not for production use.
func GenerateTestCredentialsECDSA() (*ecdsa.PrivateKey, *x509.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "pdfer test signer (ECDSA)",
			Organization: []string{"pdfer"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create cert: %w", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("parse cert: %w", err)
	}
	return key, cert, nil
}

// ---- internal helpers ----

func findLastStartXRef(pdfBytes []byte) int64 {
	searchLen := 1024
	if len(pdfBytes) < searchLen {
		searchLen = len(pdfBytes)
	}
	tail := pdfBytes[len(pdfBytes)-searchLen:]
	pat := regexp.MustCompile(`startxref\s+(\d+)`)
	match := pat.FindSubmatch(tail)
	if match != nil {
		if val, err := strconv.ParseInt(string(match[1]), 10, 64); err == nil {
			return val
		}
	}
	return -1
}

func getCatalog(pdf *parse.PDF) (int, []byte, error) {
	trailer := pdf.Trailer()
	if trailer == nil {
		return 0, nil, fmt.Errorf("no trailer")
	}
	var objNum, gen int
	if _, err := fmt.Sscanf(trailer.RootRef, "%d %d R", &objNum, &gen); err != nil {
		return 0, nil, fmt.Errorf("parse Root ref %q: %w", trailer.RootRef, err)
	}
	data, err := pdf.GetObjectContent(objNum)
	if err != nil {
		return 0, nil, fmt.Errorf("get catalog %d: %w", objNum, err)
	}
	return objNum, data, nil
}

func getPage(pdf *parse.PDF, catalogData []byte, pageIdx int) (int, []byte, error) {
	pagesObjNum, err := extractObjRef(string(catalogData), "/Pages")
	if err != nil {
		return 0, nil, fmt.Errorf("catalog /Pages: %w", err)
	}
	pagesData, err := pdf.GetObjectContent(pagesObjNum)
	if err != nil {
		return 0, nil, fmt.Errorf("get pages tree %d: %w", pagesObjNum, err)
	}
	kids := parseRefArray(extractArray(string(pagesData), "/Kids"))
	if pageIdx >= len(kids) {
		return 0, nil, fmt.Errorf("page %d out of range (%d in /Kids)", pageIdx, len(kids))
	}
	pageObjNum := kids[pageIdx]
	pageData, err := pdf.GetObjectContent(pageObjNum)
	if err != nil {
		return 0, nil, fmt.Errorf("get page %d: %w", pageObjNum, err)
	}
	return pageObjNum, pageData, nil
}

// findAcroForm returns (objNum, data, exists) for the existing AcroForm, if any.
func findAcroForm(catalogData []byte, pdf *parse.PDF) (int, []byte, bool) {
	objNum, err := extractObjRef(string(catalogData), "/AcroForm")
	if err != nil || objNum == 0 {
		return 0, nil, false
	}
	data, err := pdf.GetObjectContent(objNum)
	if err != nil {
		return 0, nil, false
	}
	return objNum, data, true
}

// addToAnnots adds widgetObjNum to the /Annots array in pageData.
func addToAnnots(pageData []byte, widgetObjNum int) []byte {
	s := string(pageData)
	newRef := fmt.Sprintf("%d 0 R", widgetObjNum)
	annotsRE := regexp.MustCompile(`/Annots\s*\[([^\]]*)\]`)
	if annotsRE.MatchString(s) {
		return []byte(annotsRE.ReplaceAllStringFunc(s, func(m string) string {
			idx := strings.LastIndex(m, "]")
			return m[:idx] + " " + newRef + m[idx:]
		}))
	}
	end := strings.LastIndex(s, ">>")
	if end == -1 {
		return pageData
	}
	return []byte(s[:end] + fmt.Sprintf("/Annots[%s]", newRef) + s[end:])
}

// addSigFieldToAcroForm adds widgetObjNum to /Fields and sets /SigFlags 3.
func addSigFieldToAcroForm(acroFormData []byte, widgetObjNum int) []byte {
	s := string(acroFormData)
	newRef := fmt.Sprintf("%d 0 R", widgetObjNum)

	fieldsRE := regexp.MustCompile(`/Fields\s*\[([^\]]*)\]`)
	if fieldsRE.MatchString(s) {
		s = fieldsRE.ReplaceAllStringFunc(s, func(m string) string {
			idx := strings.LastIndex(m, "]")
			return m[:idx] + " " + newRef + m[idx:]
		})
	} else {
		if end := strings.LastIndex(s, ">>"); end != -1 {
			s = s[:end] + fmt.Sprintf("/Fields[%s]", newRef) + s[end:]
		}
	}

	sigFlagsRE := regexp.MustCompile(`/SigFlags\s+\d+`)
	if sigFlagsRE.MatchString(s) {
		s = sigFlagsRE.ReplaceAllString(s, "/SigFlags 3")
	} else {
		if end := strings.LastIndex(s, ">>"); end != -1 {
			s = s[:end] + "/SigFlags 3" + s[end:]
		}
	}
	return []byte(s)
}

// setCatalogAcroForm inserts /AcroForm N 0 R into the catalog dict.
func setCatalogAcroForm(catalogData []byte, acroFormObjNum int) []byte {
	s := string(catalogData)
	// Remove any stale /AcroForm entry.
	s = regexp.MustCompile(`/AcroForm\s+\d+\s+\d+\s+R`).ReplaceAllString(s, "")
	if end := strings.LastIndex(s, ">>"); end != -1 {
		s = s[:end] + fmt.Sprintf("/AcroForm %d 0 R", acroFormObjNum) + s[end:]
	}
	return []byte(s)
}

// extractObjRef extracts the object number from "KEY N G R" in a dict string.
func extractObjRef(dictStr, key string) (int, error) {
	pat := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+(\d+)\s+\d+\s+R`)
	m := pat.FindStringSubmatch(dictStr)
	if m == nil {
		return 0, fmt.Errorf("%s not found", key)
	}
	return strconv.Atoi(m[1])
}

// extractArray returns the content inside "/KEY [ ... ]" from a dict string.
func extractArray(dictStr, key string) string {
	pat := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*\[([^\]]*)\]`)
	m := pat.FindStringSubmatch(dictStr)
	if m == nil {
		return ""
	}
	return m[1]
}

// parseRefArray parses "N G R ..." tokens into a slice of object numbers.
func parseRefArray(s string) []int {
	pat := regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	var result []int
	for _, m := range pat.FindAllStringSubmatch(s, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil {
			result = append(result, n)
		}
	}
	return result
}

// formatPDFDate formats a time as a PDF date string.
func formatPDFDate(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("D:%04d%02d%02d%02d%02d%02d+00'00'",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

// escapePDFStr escapes a string for use inside PDF parenthesised string literals.
func escapePDFStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '(':
			b.WriteString(`\(`)
		case ')':
			b.WriteString(`\)`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeXRefSubsections writes xref entries grouped into contiguous subsections.
func writeXRefSubsections(buf *bytes.Buffer, offsets map[int]int64, objNums []int) {
	i := 0
	for i < len(objNums) {
		j := i + 1
		for j < len(objNums) && objNums[j] == objNums[j-1]+1 {
			j++
		}
		fmt.Fprintf(buf, "%d %d\n", objNums[i], j-i)
		for k := i; k < j; k++ {
			fmt.Fprintf(buf, "%010d %05d n \n", offsets[objNums[k]], 0)
		}
		i = j
	}
}
