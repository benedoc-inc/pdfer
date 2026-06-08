package sign_test

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/sign"
)

// fakeTSAServer returns an httptest.Server that responds to any request with a
// minimal RFC 3161 TimeStampResp: PKIStatus=0 (granted) + a fake TimeStampToken.
func fakeTSAServer(t *testing.T) *httptest.Server {
	t.Helper()

	type contentInfo struct {
		ContentType asn1.ObjectIdentifier
	}
	// Fake token: ContentInfo containing id-ct-TSTInfo OID (enough for the parser
	// to accept a non-empty FullBytes value).
	fakeToken, err := asn1.Marshal(contentInfo{
		ContentType: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4},
	})
	if err != nil {
		t.Fatalf("fakeTSA: marshal ContentInfo: %v", err)
	}

	type pkiStatus struct{ Status int }
	type tsaResponse struct {
		Status         pkiStatus
		TimeStampToken asn1.RawValue `asn1:"optional"`
	}
	respDER, err := asn1.Marshal(tsaResponse{
		Status:         pkiStatus{Status: 0},
		TimeStampToken: asn1.RawValue{FullBytes: fakeToken},
	})
	if err != nil {
		t.Fatalf("fakeTSA: marshal response: %v", err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/timestamp-reply")
		w.WriteHeader(http.StatusOK)
		w.Write(respDER)
	}))
}

// parseBytRange extracts the four integers from /ByteRange [a b c d] in raw.
func parseByteRange(raw string) (off1, len1, off2, len2 int64, ok bool) {
	idx := strings.Index(raw, "/ByteRange [")
	if idx < 0 {
		return
	}
	s := raw[idx+len("/ByteRange ["):]
	end := strings.Index(s, "]")
	if end < 0 {
		return
	}
	if _, err := fmt.Sscanf(s[:end], "%d %d %d %d", &off1, &len1, &off2, &len2); err != nil {
		return
	}
	ok = true
	return
}

// --- TSA tests ---

func TestSignPDF_TSA_Succeeds(t *testing.T) {
	tsaSrv := fakeTSAServer(t)
	defer tsaSrv.Close()

	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key,
		TSAEndpoint: tsaSrv.URL,
	})
	if err != nil {
		t.Fatalf("SignPDF with TSA: %v", err)
	}
	if !bytes.HasPrefix(signed, pdf) {
		t.Error("signed PDF does not start with original bytes")
	}
}

func TestSignPDF_TSA_ContentsLarger(t *testing.T) {
	// With a TSA endpoint, reservedSigBytesWithTSA is used so /Contents hex is longer.
	tsaSrv := fakeTSAServer(t)
	defer tsaSrv.Close()

	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	plain, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("plain sign: %v", err)
	}
	withTSA, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key, TSAEndpoint: tsaSrv.URL,
	})
	if err != nil {
		t.Fatalf("TSA sign: %v", err)
	}

	if len(withTSA) <= len(plain) {
		t.Errorf("TSA-signed PDF (%d B) should be larger than plain-signed (%d B)", len(withTSA), len(plain))
	}
}

func TestSignPDF_TSA_ByteRangeValid(t *testing.T) {
	tsaSrv := fakeTSAServer(t)
	defer tsaSrv.Close()

	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key, TSAEndpoint: tsaSrv.URL,
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}

	off1, len1, off2, len2, ok := parseByteRange(string(signed))
	if !ok {
		t.Fatal("could not parse /ByteRange")
	}
	if off1 != 0 {
		t.Errorf("ByteRange[0] should be 0, got %d", off1)
	}
	if off2 <= len1 {
		t.Errorf("ByteRange[2] (%d) should be > ByteRange[1] (%d)", off2, len1)
	}
	if off2+len2 != int64(len(signed)) {
		t.Errorf("ByteRange off2+len2=%d, file length=%d", off2+len2, len(signed))
	}
}

func TestSignPDF_TSA_SigDictPresent(t *testing.T) {
	tsaSrv := fakeTSAServer(t)
	defer tsaSrv.Close()

	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key, TSAEndpoint: tsaSrv.URL,
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	raw := string(signed)
	for _, want := range []string{"/Type/Sig", "/Filter/Adobe.PPKLite", "/ByteRange", "/Contents"} {
		if !strings.Contains(raw, want) {
			t.Errorf("TSA-signed PDF missing %q", want)
		}
	}
}

// --- LTV / DSS tests ---

func TestSignPDF_DSS_WithCertChain(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key,
		CertChain: []*x509.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("SignPDF with CertChain: %v", err)
	}
	raw := string(signed)
	if !strings.Contains(raw, "/DSS") {
		t.Error("expected /DSS in output")
	}
	if !strings.Contains(raw, "/Certs") {
		t.Error("expected /Certs in DSS")
	}
	if !strings.Contains(raw, "/VRI") {
		t.Error("expected /VRI in DSS")
	}
}

func TestSignPDF_DSS_WithOCSPResponse(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()
	fakeOCSP := []byte{0x30, 0x03, 0x0a, 0x01, 0x00} // minimal DER SEQUENCE

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key,
		OCSPResponse: fakeOCSP,
	})
	if err != nil {
		t.Fatalf("SignPDF with OCSPResponse: %v", err)
	}
	raw := string(signed)
	if !strings.Contains(raw, "/DSS") {
		t.Error("expected /DSS in output")
	}
	if !strings.Contains(raw, "/OCSPs") {
		t.Error("expected /OCSPs in DSS")
	}
}

func TestSignPDF_DSS_NoneWhenEmpty(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if strings.Contains(string(signed), "/DSS") {
		t.Error("no /DSS expected when CertChain and OCSPResponse are both empty")
	}
}

func TestSignPDF_DSS_IsIncrementalUpdate(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key,
		CertChain: []*x509.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}

	// Must start with original bytes (two incremental updates on top).
	if !bytes.HasPrefix(signed, pdf) {
		t.Error("output does not start with original PDF bytes")
	}
	// Signature update has /Prev → original; DSS update has /Prev → signature update.
	if strings.Count(string(signed), "/Prev") < 2 {
		t.Errorf("expected ≥2 /Prev entries, got %d", strings.Count(string(signed), "/Prev"))
	}
}

func TestSignPDF_DSS_CatalogHasDSSRef(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert, PrivateKey: key,
		CertChain: []*x509.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	// Catalog must contain /DSS N 0 R reference.
	if !strings.Contains(string(signed), "/DSS ") {
		t.Error("catalog does not contain /DSS reference")
	}
}
