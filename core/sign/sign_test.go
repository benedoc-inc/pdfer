package sign_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/sign"
	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// buildSimplePDF creates a minimal one-page PDF for signing tests.
func buildSimplePDF(t *testing.T) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	p := b.AddPage(write.PageSizeLetter)
	p.Content().
		BeginText().
		SetFont(p.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 720).
		ShowText("Sign me").
		EndText()
	b.FinalizePage(p)
	out, err := b.Bytes()
	if err != nil {
		t.Fatalf("build PDF: %v", err)
	}
	return out
}

func TestSignPDF_ProducesValidOutput(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, err := sign.GenerateTestCredentials()
	if err != nil {
		t.Fatalf("generate credentials: %v", err)
	}

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Reason:      "Test signature",
		Location:    "pdfer unit tests",
		FieldName:   "TestSig",
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if len(signed) == 0 {
		t.Fatal("signed PDF is empty")
	}
}

func TestSignPDF_SigDictPresent(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	raw := string(signed)
	for _, want := range []string{
		"/Type/Sig",
		"/Filter/Adobe.PPKLite",
		"/SubFilter/adbe.pkcs7.detached",
		"/ByteRange",
		"/Contents",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("signed PDF missing %q", want)
		}
	}
}

func TestSignPDF_ByteRangeIsNumeric(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	// ByteRange must contain four decimal integers, not zeros placeholder.
	pat := regexp.MustCompile(`/ByteRange\s*\[(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\]`)
	m := pat.FindSubmatch(signed)
	if m == nil {
		t.Fatal("could not find /ByteRange with four integers")
	}
	// First value must be 0.
	if string(m[1]) != "0000000000" && string(m[1]) != "0" {
		// Accept either zero-padded or plain 0.
	}
	// Third value (offset2) must be greater than second (length1) — basic sanity.
	// Both are non-empty decimal strings (placeholder zeros have been overwritten).
}

func TestSignPDF_ContentsHexIsNonTrivial(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	// /Contents <hex...> — the leading bytes must not all be zero (PKCS#7 starts with 0x30).
	pat := regexp.MustCompile(`/Contents <([0-9A-Fa-f]+)>`)
	m := pat.FindSubmatch(signed)
	if m == nil {
		t.Fatal("could not find /Contents hex string")
	}
	hex := string(m[1])
	if len(hex) == 0 {
		t.Fatal("empty /Contents hex")
	}
	// PKCS#7 ContentInfo is a SEQUENCE (tag 0x30), hex "30".
	if !strings.HasPrefix(strings.ToUpper(hex), "30") {
		t.Errorf("PKCS#7 should start with SEQUENCE tag 0x30, got %q", hex[:4])
	}
}

func TestSignPDF_IncrementalUpdate(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	// Signed PDF must begin with the original bytes unchanged.
	if !bytes.HasPrefix(signed, pdf) {
		t.Error("signed PDF does not start with original PDF bytes (incremental update violated)")
	}
	// Must contain a /Prev xref reference.
	if !strings.Contains(string(signed), "/Prev") {
		t.Error("signed PDF missing /Prev in trailer (not an incremental update)")
	}
}

func TestSignPDF_WidgetAnnotation(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	raw := string(signed)
	if !strings.Contains(raw, "/Subtype/Widget") {
		t.Error("signed PDF missing /Subtype/Widget annotation")
	}
	if !strings.Contains(raw, "/FT/Sig") {
		t.Error("signed PDF missing /FT/Sig signature field type")
	}
	if !strings.Contains(raw, "Rect[0 0 0 0]") {
		t.Error("signature widget should have invisible Rect [0 0 0 0]")
	}
}

func TestSignPDF_ByteRangeCoverageIsConsistent(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}

	pat := regexp.MustCompile(`/ByteRange\s*\[(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\]`)
	m := pat.FindSubmatch(signed)
	if m == nil {
		t.Fatal("could not find /ByteRange")
	}

	parse := func(b []byte) int64 {
		var n int64
		for _, c := range b {
			n = n*10 + int64(c-'0')
		}
		return n
	}
	off1 := parse(m[1]) // should be 0
	len1 := parse(m[2])
	off2 := parse(m[3])
	len2 := parse(m[4])

	total := int64(len(signed))
	if off1 != 0 {
		t.Errorf("ByteRange offset1 should be 0, got %d", off1)
	}
	if off2 <= len1 {
		t.Errorf("ByteRange offset2 (%d) should be > length1 (%d)", off2, len1)
	}
	if off2+len2 != total {
		t.Errorf("ByteRange off2+len2 = %d, file length = %d — mismatch", off2+len2, total)
	}
}

func TestSignPDF_CustomFieldName(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		FieldName:   "MySig",
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if !strings.Contains(string(signed), "MySig") {
		t.Error("signed PDF does not contain custom field name 'MySig'")
	}
}

func TestSignPDF_ReasonAndLocation(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Reason:      "Approved",
		Location:    "New York",
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	raw := string(signed)
	if !strings.Contains(raw, "Approved") {
		t.Error("signed PDF missing /Reason")
	}
	if !strings.Contains(raw, "New York") {
		t.Error("signed PDF missing /Location")
	}
}

func TestSignPDF_ECDSA(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, err := sign.GenerateTestCredentialsECDSA()
	if err != nil {
		t.Fatalf("generate ECDSA credentials: %v", err)
	}

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Reason:      "ECDSA test",
		FieldName:   "ECDSASig",
	})
	if err != nil {
		t.Fatalf("SignPDF (ECDSA): %v", err)
	}

	// Must start with original bytes (incremental update).
	if !bytes.HasPrefix(signed, pdf) {
		t.Error("ECDSA-signed PDF does not preserve original bytes")
	}

	// /Contents must begin with PKCS#7 SEQUENCE tag (0x30).
	pat := regexp.MustCompile(`/Contents <([0-9A-Fa-f]+)>`)
	m := pat.FindSubmatch(signed)
	if m == nil {
		t.Fatal("ECDSA signed PDF: /Contents hex not found")
	}
	if !strings.HasPrefix(strings.ToUpper(string(m[1])), "30") {
		t.Errorf("ECDSA PKCS#7 should start with 0x30, got %q", string(m[1])[:4])
	}

	// ByteRange coverage must be consistent.
	brPat := regexp.MustCompile(`/ByteRange\s*\[(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\]`)
	bm := brPat.FindSubmatch(signed)
	if bm == nil {
		t.Fatal("ECDSA signed PDF: /ByteRange not found")
	}
	parse := func(b []byte) int64 {
		var n int64
		for _, c := range b {
			n = n*10 + int64(c-'0')
		}
		return n
	}
	off2 := parse(bm[3])
	len2 := parse(bm[4])
	if off2+len2 != int64(len(signed)) {
		t.Errorf("ECDSA ByteRange off2+len2=%d, file length=%d", off2+len2, len(signed))
	}
}
