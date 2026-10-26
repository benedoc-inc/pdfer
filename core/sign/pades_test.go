package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

// ⚠⚠ THE ATTRIBUTE THAT SAYS WHICH CERTIFICATE SIGNED. SignerInfo names the
// signer by issuer-and-serial, but that lives OUTSIDE the signed attributes and
// so is not covered by the signature. Binding a hash of the certificate into
// the signed set is what stops a re-wrapped CMS blob being attributed to a
// different certificate — and it is what CAdES/PAdES require.
func TestSignedAttributesBindTheSigningCertificate(t *testing.T) {
	key, cert, err := GenerateTestCredentials()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("the document"))
	ctx, err := preparePKCS7(cert, key, digest[:], time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	// The attribute is present, and the hash inside it is this certificate's.
	want := sha256.Sum256(cert.Raw)
	if !bytes.Contains(ctx.signedAttrsImplicit, want[:]) {
		t.Fatal("signed attributes do not carry a hash of the signing certificate")
	}

	// And it is a well-formed signingCertificateV2 attribute, not just the
	// bytes appearing by coincidence: walk the DER and find the OID.
	oidDER, err := asn1.Marshal(oidSigningCertificateV2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ctx.signedAttrsImplicit, oidDER) {
		t.Error("id-aa-signingCertificateV2 OID is absent from the signed attributes")
	}

	// ⚠ hashAlgorithm must be OMITTED, not encoded as SHA-256. RFC 5035
	// defaults it, and DER forbids encoding a DEFAULT value — some validators
	// reject the structure outright if it is present.
	sha256AlgID := algID(derOIDs.sha256)
	idx := bytes.Index(ctx.signedAttrsImplicit, want[:])
	// The 4 bytes before the hash are the OCTET STRING header and the
	// enclosing SEQUENCE headers; an encoded AlgorithmIdentifier would sit
	// immediately before them and is 15 bytes, so check that window.
	start := idx - len(sha256AlgID) - 8
	if start < 0 {
		start = 0
	}
	if bytes.Contains(ctx.signedAttrsImplicit[start:idx], sha256AlgID) {
		t.Error("ESSCertIDv2 encodes hashAlgorithm, which DER forbids for a DEFAULT value")
	}
	// Also reject the no-parameters form of the AlgorithmIdentifier
	// (SEQUENCE { oid } with no NULL), so a regression that emitted the DEFAULT
	// in either spelling is caught, not only the NULL-parameters form.
	sha256AlgIDNoParams := algIDNoParams(derOIDs.sha256)
	if bytes.Contains(ctx.signedAttrsImplicit[start:idx], sha256AlgIDNoParams) {
		t.Error("ESSCertIDv2 encodes hashAlgorithm (no-params form), which DER forbids for a DEFAULT value")
	}
}

// The producer binding is only half the defense; the verifier must enforce it.
// Here we build genuine signed attributes (carrying signingCertificateV2 for
// `cert`) and confirm checkSigningCertificateV2 accepts the genuine certificate
// but rejects a DIFFERENT certificate that reuses the SAME public key — exactly
// the certificate-substitution the attribute exists to stop.
func TestVerifierEnforcesSigningCertificateBinding(t *testing.T) {
	key, cert, err := GenerateTestCredentials()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("the document"))
	ctx, err := preparePKCS7(cert, key, digest[:], time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	// The signed-attrs SET content is what a parsed SignerInfo exposes as
	// SignedAttrs.Bytes; recover it by stripping the [0] IMPLICIT header.
	attrsContent := stripTagAndLength(t, ctx.signedAttrsImplicit)
	si := signerInfoASN{SignedAttrs: asn1.RawValue{Bytes: attrsContent}}

	// The genuine certificate: the binding holds.
	if err := checkSigningCertificateV2(si, cert); err != nil {
		t.Fatalf("binding rejected the genuine signing certificate: %v", err)
	}

	// A re-wrap that swaps in a different certificate carrying the same public
	// key (so the signature over the attributes would still verify) must be
	// caught by the certHash comparison.
	swapped := reissueWithSameKey(t, cert, key)
	if bytes.Equal(swapped.Raw, cert.Raw) {
		t.Fatal("test setup: substituted certificate is byte-identical to the original")
	}
	if err := checkSigningCertificateV2(si, swapped); err == nil {
		t.Fatal("binding accepted a substituted certificate; the attack is not detected")
	}
}

// reissueWithSameKey mints a fresh self-signed certificate over the same public
// key as src but with a different serial and subject, so cert.Raw differs while
// the key does not — the shape of a certificate-substitution attack.
func reissueWithSameKey(t *testing.T, src *x509.Certificate, key crypto.Signer) *x509.Certificate {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "attacker-chosen name"},
		NotBefore:             src.NotBefore,
		NotAfter:              src.NotAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// X.690 §11.6: a SET OF is ordered by its members' encodings. Our own
// round-trip does not care, but a validator that re-encodes canonically before
// hashing does — and that failure only ever shows up in someone else's tool.
func TestSignedAttributesAreSortedAsDERRequires(t *testing.T) {
	key, cert, err := GenerateTestCredentials()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("the document"))
	ctx, err := preparePKCS7(cert, key, digest[:], time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	// Walk the [0] IMPLICIT content as a run of attributes and check each is
	// >= the one before it.
	rest := stripTagAndLength(t, ctx.signedAttrsImplicit)

	var prev []byte
	for len(rest) > 0 {
		elem, remainder := nextElement(t, rest)
		if prev != nil && bytes.Compare(prev, elem) > 0 {
			t.Fatalf("signed attributes are not in DER SET order")
		}
		prev, rest = elem, remainder
	}
	if prev == nil {
		t.Fatal("no attributes found")
	}
}

// stripTagAndLength drops a single DER tag+length header, returning the content.
func stripTagAndLength(t *testing.T, b []byte) []byte {
	t.Helper()
	if len(b) < 2 {
		t.Fatal("truncated DER")
	}
	i := 1
	n := int(b[i])
	i++
	if n&0x80 != 0 {
		count := n & 0x7F
		n = 0
		for c := 0; c < count; c++ {
			n = n<<8 | int(b[i])
			i++
		}
	}
	if i+n > len(b) {
		t.Fatalf("DER length %d exceeds %d remaining", n, len(b)-i)
	}
	return b[i : i+n]
}

// nextElement returns the next complete DER element and what follows it.
func nextElement(t *testing.T, b []byte) (elem, rest []byte) {
	t.Helper()
	if len(b) < 2 {
		t.Fatal("truncated DER element")
	}
	i := 1
	n := int(b[i])
	i++
	if n&0x80 != 0 {
		count := n & 0x7F
		n = 0
		for c := 0; c < count; c++ {
			n = n<<8 | int(b[i])
			i++
		}
	}
	return b[:i+n], b[i+n:]
}
