package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SignatureInfo describes one digital signature found in a PDF.
type SignatureInfo struct {
	FieldName   string            // AcroForm field name
	SignerName  string            // CN from the signer certificate, or SubjectCommonName
	SignedAt    time.Time         // signing time from SigningTime attribute (zero if absent)
	Reason      string            // /Reason from sig dict
	Location    string            // /Location from sig dict
	ByteRange   [4]int64          // the four byte-range values
	Valid       bool              // true if signature cryptographically verifies
	Error       string            // non-empty if validation failed
	Certificate *x509.Certificate // signer's leaf certificate (nil if unparseable)
}

// ---- ASN.1 structures for PKCS#7 / CMS parsing ----

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,tag:0,optional"`
}

type signedDataASN struct {
	Version          int
	DigestAlgorithms []pkix.AlgorithmIdentifier `asn1:"set"`
	EncapContentInfo asn1.RawValue
	Certificates     asn1.RawValue `asn1:"optional,tag:0"`
	CRLs             asn1.RawValue `asn1:"optional,tag:1"`
	SignerInfos      []signerInfoASN `asn1:"set"`
}

type signerInfoASN struct {
	Version            int
	IssuerAndSerial    issuerAndSerial
	DigestAlgorithm    pkix.AlgorithmIdentifier
	SignedAttrs        asn1.RawValue        `asn1:"optional,tag:0"`
	SignatureAlgorithm pkix.AlgorithmIdentifier
	Signature          []byte
	UnsignedAttrs      asn1.RawValue `asn1:"optional,tag:1"`
}

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type pkcs9Attr struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

// ---- regex patterns for raw PDF scanning ----

var (
	reSigBlock    = regexp.MustCompile(`(?s)/Type\s*/Sig|/SubFilter\s*/adbe\.pkcs7\.detached`)
	reByteRange   = regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	reContents    = regexp.MustCompile(`/Contents\s*<([0-9A-Fa-f\s]+)>`)
	reReason      = regexp.MustCompile(`/Reason\s*\(`)
	reLocation    = regexp.MustCompile(`/Location\s*\(`)
	reFieldName   = regexp.MustCompile(`/T\s*\(([^)]*)\)`)
)

// ValidateSignatures scans pdfBytes for digital signature dictionaries and
// cryptographically verifies each one. It returns one SignatureInfo per
// signature found. An error is returned only for fatal problems that prevent
// any scanning; per-signature failures are reported in SignatureInfo.Error.
func ValidateSignatures(pdfBytes []byte) ([]SignatureInfo, error) {
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("empty PDF")
	}

	raw := string(pdfBytes)
	sigOffsets := findSigDictOffsets(raw)
	if len(sigOffsets) == 0 {
		return nil, nil
	}

	var results []SignatureInfo
	for _, start := range sigOffsets {
		info := validateOneSig(pdfBytes, raw, start)
		results = append(results, info)
	}
	return results, nil
}

// findSigFieldName scans from sigDictStart to end of file looking for a widget
// annotation dict (/Subtype/Widget with /FT/Sig) and returns the /T value.
// Returns empty string if not found.
func findSigFieldName(raw string, sigDictStart int) string {
	// Search forward from the sig dict position.
	searchArea := raw[sigDictStart:]

	// Find all widget annotation dicts that are signature fields.
	// Look for << ... /Subtype/Widget ... /FT/Sig ... /T (name) ... >>
	// We walk through looking for << openings and check each dict.
	reWidgetOpen := regexp.MustCompile(`<<`)
	for _, loc := range reWidgetOpen.FindAllStringIndex(searchArea, -1) {
		dictStart := loc[0]
		dictEnd := dictStart + 4096
		if dictEnd > len(searchArea) {
			dictEnd = len(searchArea)
		}
		chunk := searchArea[dictStart:dictEnd]

		// Quick pre-filter: must contain Widget and FT/Sig and /T.
		if !strings.Contains(chunk, "/Subtype") || !strings.Contains(chunk, "Widget") {
			continue
		}
		if !strings.Contains(chunk, "/FT") || !strings.Contains(chunk, "/Sig") {
			continue
		}

		// Look for /T (fieldname) in this chunk.
		if fm := reFieldName.FindStringSubmatch(chunk); fm != nil {
			return unescapePDFString(fm[1])
		}
	}
	return ""
}

// findSigDictOffsets returns the start position of each signature dict in the
// raw PDF string. We look for `/Type/Sig` or `/SubFilter/adbe.pkcs7.detached`
// and then walk back to the nearest `<<` to anchor the dict window.
func findSigDictOffsets(raw string) []int {
	var offsets []int
	seen := make(map[int]bool)

	for _, loc := range reSigBlock.FindAllStringIndex(raw, -1) {
		// Walk back to find the opening << of this dict.
		anchor := loc[0]
		start := strings.LastIndex(raw[:anchor+1], "<<")
		if start == -1 {
			start = 0
		}
		if !seen[start] {
			seen[start] = true
			offsets = append(offsets, start)
		}
	}
	return offsets
}

// dictWindow extracts a reasonable window of text around the signature dict
// start. Signature dicts can be large (they contain the hex-encoded /Contents)
// so we use a generous window and cap it at a closing ">>" boundary.
func dictWindow(raw string, start int) string {
	end := start + 65536
	if end > len(raw) {
		end = len(raw)
	}
	// Try to find the closing >> of the dict. A simple heuristic: find the
	// last ">>" within the window that is followed by either whitespace,
	// "endobj", or end-of-window.
	window := raw[start:end]
	closeIdx := strings.LastIndex(window, ">>")
	if closeIdx != -1 && closeIdx+2 < len(window) {
		window = window[:closeIdx+2]
	}
	return window
}

// validateOneSig extracts fields from one signature dict and verifies it.
func validateOneSig(pdfBytes []byte, raw string, start int) SignatureInfo {
	var info SignatureInfo

	window := dictWindow(raw, start)

	// ---- ByteRange ----
	brMatch := reByteRange.FindStringSubmatch(window)
	if brMatch == nil {
		info.Error = "ByteRange not found in signature dict"
		return info
	}
	var br [4]int64
	for i := 0; i < 4; i++ {
		n, parseErr := strconv.ParseInt(strings.TrimSpace(brMatch[i+1]), 10, 64)
		if parseErr != nil {
			info.Error = fmt.Sprintf("invalid ByteRange value[%d]: %v", i, parseErr)
			return info
		}
		br[i] = n
	}
	info.ByteRange = br

	// Validate byte range bounds.
	fileLen := int64(len(pdfBytes))
	a, b, c, d := br[0], br[1], br[2], br[3]
	if a < 0 || b < 0 || c < 0 || d < 0 ||
		a+b > fileLen || c+d > fileLen || a+b > c {
		info.Error = "invalid byte range"
		return info
	}

	// ---- /Contents hex ----
	cMatch := reContents.FindStringSubmatch(window)
	if cMatch == nil {
		info.Error = "/Contents not found in signature dict"
		return info
	}
	hexStr := strings.ReplaceAll(cMatch[1], " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\r", "")
	// Trim trailing zeros that are padding (odd-length guard).
	derBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		info.Error = fmt.Sprintf("hex decode /Contents: %v", err)
		return info
	}

	// ---- Optional metadata ----
	info.Reason = extractPDFLiteralString(window, reReason)
	info.Location = extractPDFLiteralString(window, reLocation)

	// Field name: look in a widget annotation dict associated with this
	// signature. The widget dict is typically written after the sig dict in
	// the incremental update and contains /Subtype/Widget plus /FT/Sig and
	// /T (fieldname). Search from the sig dict start to end-of-file.
	info.FieldName = findSigFieldName(raw, start)

	// ---- Parse PKCS#7 ----
	cert, signedAt, valid, verifyErr := verifyPKCS7(pdfBytes, a, b, c, d, derBytes)
	info.Certificate = cert
	info.SignedAt = signedAt
	info.Valid = valid
	if verifyErr != nil {
		info.Error = verifyErr.Error()
	}
	if cert != nil {
		info.SignerName = cert.Subject.CommonName
	}

	return info
}

// verifyPKCS7 parses and cryptographically verifies a PKCS#7 / CMS detached
// signature over the byte ranges [a:a+b] ++ [c:c+d] of pdfBytes.
func verifyPKCS7(
	pdfBytes []byte,
	a, b, c, d int64,
	derBytes []byte,
) (cert *x509.Certificate, signedAt time.Time, valid bool, err error) {
	// Strip trailing zero padding (the /Contents field is zero-padded to a
	// fixed reservation size). Real DER always starts 0x30 and its length
	// field tells us the exact size.
	derBytes = trimPKCS7Padding(derBytes)
	if len(derBytes) == 0 {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: empty after padding trim")
	}

	// ---- Parse ContentInfo ----
	var ci contentInfo
	rest, asn1Err := asn1.Unmarshal(derBytes, &ci)
	if asn1Err != nil || len(rest) > 0 {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: %v", asn1Err)
	}
	if !ci.ContentType.Equal(oidSignedData) {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: unexpected ContentType %v", ci.ContentType)
	}

	// ---- Parse SignedData ----
	var sd signedDataASN
	if _, err2 := asn1.Unmarshal(ci.Content.Bytes, &sd); err2 != nil {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: SignedData: %v", err2)
	}
	if len(sd.SignerInfos) == 0 {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: no SignerInfo")
	}

	// ---- Extract signer certificate ----
	// The certificates field is [0] IMPLICIT (context-constructed, tag 0).
	// Its bytes are the concatenated DER certificates.
	cert, parseErr := extractSignerCert(sd)
	if parseErr != nil {
		return nil, time.Time{}, false, fmt.Errorf("PKCS#7 parse failed: cert: %v", parseErr)
	}

	si := sd.SignerInfos[0]

	// ---- Extract signing time from signed attributes ----
	signedAt = extractSigningTime(si)

	// ---- Verify the signature ----
	verifyErr := verifySigInfo(pdfBytes, a, b, c, d, si, cert)
	if verifyErr != nil {
		return cert, signedAt, false, verifyErr
	}
	return cert, signedAt, true, nil
}

// trimPKCS7Padding removes zero bytes appended as padding after the actual DER.
// Real DER is self-describing: byte[0] must be 0x30 (SEQUENCE) and the length
// bytes tell us exactly how long the structure is.
func trimPKCS7Padding(b []byte) []byte {
	if len(b) < 2 || b[0] != 0x30 {
		return b
	}
	_, headerLen, err := decodeDERLength(b[1:])
	if err != nil {
		return b
	}
	total := 1 + headerLen
	if total > len(b) {
		return b
	}
	return b[:total]
}

// decodeDERLength reads a BER/DER length from the bytes starting at p,
// returning (length_value, bytes_consumed_including_length_octets, error).
func decodeDERLength(p []byte) (int, int, error) {
	if len(p) == 0 {
		return 0, 0, fmt.Errorf("empty")
	}
	if p[0] < 0x80 {
		// Short form: the byte itself is the length.
		// total consumed = 1 (length byte) + length value
		return int(p[0]), 1 + int(p[0]), nil
	}
	numBytes := int(p[0] & 0x7f)
	if numBytes == 0 || numBytes > 4 || len(p) < 1+numBytes {
		return 0, 0, fmt.Errorf("invalid DER length encoding")
	}
	var n int
	for i := 0; i < numBytes; i++ {
		n = (n << 8) | int(p[1+i])
	}
	// total consumed = 1 (initial byte) + numBytes (length octets) + n (content)
	return n, 1 + numBytes + n, nil
}

// extractSignerCert parses all certificates from the [0] IMPLICIT certificates
// field of SignedData and returns the first one (the signer's leaf cert).
func extractSignerCert(sd signedDataASN) (*x509.Certificate, error) {
	// sd.Certificates is the raw [0] context tag. Its bytes are the
	// concatenated DER certificates (no extra wrapper).
	if sd.Certificates.FullBytes == nil && sd.Certificates.Bytes == nil {
		return nil, fmt.Errorf("no certificates in SignedData")
	}

	// The Bytes field holds the content of the [0] tag.
	certBytes := sd.Certificates.Bytes
	if len(certBytes) == 0 {
		return nil, fmt.Errorf("no certificates in SignedData")
	}

	// There may be multiple DER-encoded certificates back to back.
	// Parse the first one.
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		// Try parsing as a sequence of certs (the content might be multiple
		// certs concatenated).
		certs, err2 := x509.ParseCertificates(certBytes)
		if err2 != nil || len(certs) == 0 {
			return nil, fmt.Errorf("parse cert: %v", err)
		}
		return certs[0], nil
	}
	return cert, nil
}

// extractSigningTime looks for the id-signingTime attribute in SignerInfo's
// signedAttrs and parses the time value. Returns zero time if absent.
func extractSigningTime(si signerInfoASN) time.Time {
	if si.SignedAttrs.Bytes == nil {
		return time.Time{}
	}

	// Walk through the signedAttrs SET content.
	rest := si.SignedAttrs.Bytes
	for len(rest) > 0 {
		var attr pkcs9Attr
		remaining, err := asn1.Unmarshal(rest, &attr)
		if err != nil {
			break
		}
		rest = remaining

		if attr.Type.Equal(oidSigningTime) {
			// Values is the SET content; the actual time is inside.
			var t time.Time
			if _, err2 := asn1.Unmarshal(attr.Values.Bytes, &t); err2 == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// verifySigInfo verifies the cryptographic signature in si against pdfBytes
// using cert's public key.
func verifySigInfo(
	pdfBytes []byte,
	a, b, c, d int64,
	si signerInfoASN,
	cert *x509.Certificate,
) error {
	if si.SignedAttrs.Bytes == nil {
		return fmt.Errorf("no signed attributes in SignerInfo")
	}

	// ---- Hash the signed content (byte ranges) and check messageDigest ----
	h := sha256.New()
	h.Write(pdfBytes[a : a+b])
	h.Write(pdfBytes[c : c+d])
	contentDigest := h.Sum(nil)

	// Verify that the messageDigest signed attribute matches.
	if err := checkMessageDigest(si, contentDigest); err != nil {
		return err
	}

	// ---- Build the SET-of-signedAttrs bytes for signature verification ----
	// SignedAttrs is stored with tag [0] (0xa0) in the DER. For signing,
	// the SET tag (0x31) replaces the [0] tag. We take the raw bytes of the
	// [0] tag and swap the first byte.
	rawSignedAttrs := si.SignedAttrs.FullBytes
	if len(rawSignedAttrs) == 0 {
		return fmt.Errorf("empty signedAttrs raw bytes")
	}
	setBytes := make([]byte, len(rawSignedAttrs))
	copy(setBytes, rawSignedAttrs)
	setBytes[0] = 0x31 // replace [0] with SET tag

	sigHash := sha256.Sum256(setBytes)

	// ---- Verify the signature ----
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sigHash[:], si.Signature); err != nil {
			return fmt.Errorf("RSA signature verification failed: %v", err)
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pub, sigHash[:], si.Signature) {
			return fmt.Errorf("ECDSA signature verification failed")
		}
	default:
		return fmt.Errorf("unsupported public key type %T", cert.PublicKey)
	}

	return nil
}

// checkMessageDigest verifies the messageDigest signed attribute against the
// provided content digest.
func checkMessageDigest(si signerInfoASN, contentDigest []byte) error {
	rest := si.SignedAttrs.Bytes
	for len(rest) > 0 {
		var attr pkcs9Attr
		remaining, err := asn1.Unmarshal(rest, &attr)
		if err != nil {
			break
		}
		rest = remaining

		if attr.Type.Equal(oidMessageDigest) {
			// Values.Bytes is the SET content; inside is an OCTET STRING.
			var digest []byte
			if _, err2 := asn1.Unmarshal(attr.Values.Bytes, &digest); err2 != nil {
				return fmt.Errorf("parse messageDigest attribute: %v", err2)
			}
			if string(digest) != string(contentDigest) {
				return fmt.Errorf("messageDigest mismatch: signature covers different content")
			}
			return nil
		}
	}
	return fmt.Errorf("messageDigest attribute not found in signedAttrs")
}

// ---- PDF string helpers ----

// extractPDFLiteralString finds a key like `/Reason (` in window and returns
// the decoded literal string content.
func extractPDFLiteralString(window string, re *regexp.Regexp) string {
	loc := re.FindStringIndex(window)
	if loc == nil {
		return ""
	}
	// Scan past the opening '(' to collect the literal string.
	pos := loc[1] // points just past the '('
	var sb strings.Builder
	depth := 1
	for pos < len(window) && depth > 0 {
		ch := window[pos]
		if ch == '\\' && pos+1 < len(window) {
			pos++
			switch window[pos] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '(':
				sb.WriteByte('(')
			case ')':
				sb.WriteByte(')')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte(window[pos])
			}
		} else if ch == '(' {
			depth++
			sb.WriteByte(ch)
		} else if ch == ')' {
			depth--
			if depth > 0 {
				sb.WriteByte(ch)
			}
		} else {
			sb.WriteByte(ch)
		}
		pos++
	}
	return sb.String()
}

// unescapePDFString decodes escape sequences in a PDF literal string (the
// content between the outer parentheses, after the regex already stripped them).
func unescapePDFString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case '(':
				sb.WriteByte('(')
			case ')':
				sb.WriteByte(')')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte(s[i])
			}
		} else {
			sb.WriteByte(s[i])
		}
	}
	return sb.String()
}
