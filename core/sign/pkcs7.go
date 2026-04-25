// Package sign provides PDF digital signature creation.
package sign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"time"
)

// OIDs required for CMS/PKCS#7 detached signatures.
var (
	oidData                  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidSHA256                = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidECDSAWithSHA256       = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidContentType           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidSigTimeStampToken     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
)

// derOIDs holds pre-computed DER encodings of the OIDs used in PKCS#7
// structures. Populated once at init; asn1.Marshal on a valid, non-nil OID
// cannot fail, so any error here indicates a compile-time constant bug.
var derOIDs struct {
	data, signedData, sha256, rsaEncryption, ecdsaWithSHA256 []byte
	contentType, messageDigest, signingTime, sigTimeStampToken []byte
}

func init() {
	enc := func(oid asn1.ObjectIdentifier) []byte {
		b, err := asn1.Marshal(oid)
		if err != nil {
			panic(fmt.Sprintf("pkcs7 init: marshal OID %v: %v", oid, err))
		}
		return b
	}
	derOIDs.data = enc(oidData)
	derOIDs.signedData = enc(oidSignedData)
	derOIDs.sha256 = enc(oidSHA256)
	derOIDs.rsaEncryption = enc(oidRSAEncryption)
	derOIDs.ecdsaWithSHA256 = enc(oidECDSAWithSHA256)
	derOIDs.contentType = enc(oidContentType)
	derOIDs.messageDigest = enc(oidMessageDigest)
	derOIDs.signingTime = enc(oidSigningTime)
	derOIDs.sigTimeStampToken = enc(oidSigTimeStampToken)
}

// derLen encodes a DER length field.
func derLen(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	if n < 0x100 {
		return []byte{0x81, byte(n)}
	}
	if n < 0x10000 {
		return []byte{0x82, byte(n >> 8), byte(n)}
	}
	return []byte{0x83, byte(n >> 16), byte(n >> 8), byte(n)}
}

// derWrap prepends a tag byte and DER length to content.
func derWrap(tag byte, content []byte) []byte {
	out := make([]byte, 0, 1+len(derLen(len(content)))+len(content))
	out = append(out, tag)
	out = append(out, derLen(len(content))...)
	out = append(out, content...)
	return out
}

func derSequence(parts ...[]byte) []byte { return derWrap(0x30, cat(parts...)) }
func derSet(parts ...[]byte) []byte      { return derWrap(0x31, cat(parts...)) }

// derContextImplicit wraps content with a context-specific constructed tag [n].
func derContextImplicit(n int, content []byte) []byte {
	return derWrap(byte(0xa0|n), content)
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// algID encodes AlgorithmIdentifier { oid, NULL }.
// RSA algorithm identifiers conventionally include a NULL parameters field.
func algID(derOID []byte) []byte {
	return derSequence(derOID, []byte{0x05, 0x00})
}

// algIDNoParams encodes AlgorithmIdentifier { oid } with no parameters field.
// ECDSA algorithm identifiers (ecdsa-with-SHA256 etc.) MUST omit parameters.
func algIDNoParams(derOID []byte) []byte {
	return derSequence(derOID)
}

// attr encodes a PKCS#9 attribute: SEQUENCE { OID, SET { value... } }.
func attr(derOID []byte, values ...[]byte) []byte {
	return derSequence(derOID, derSet(values...))
}

// pkcs7Context holds the intermediate state produced by preparePKCS7.
// Use assemblePKCS7 to build the final DER bytes.
type pkcs7Context struct {
	signedAttrsImplicit []byte // [0] IMPLICIT signed attributes (for SignerInfo)
	sigAlgDER           []byte // AlgorithmIdentifier for the signature algorithm
	sigValue            []byte // raw signature bytes (RSA PKCS#1 or ECDSA DER)
	issuerAndSerial     []byte // IssuerAndSerialNumber for SignerIdentifier
	certsDER            []byte // [0] IMPLICIT certificates block
}

// preparePKCS7 computes the signed attributes and signature value exactly once.
// The returned context can be passed to assemblePKCS7, optionally with a TSA token.
func preparePKCS7(cert *x509.Certificate, key crypto.Signer, digest []byte, t time.Time) (*pkcs7Context, error) {
	// ---- signed attributes ----
	contentTypeAttrVal, _ := asn1.Marshal(oidData)
	digestAttrVal, _ := asn1.Marshal(digest)
	signingTimeAttrVal, _ := asn1.Marshal(t.UTC())

	attrsContent := cat(
		attr(derOIDs.contentType, contentTypeAttrVal),
		attr(derOIDs.messageDigest, digestAttrVal),
		attr(derOIDs.signingTime, signingTimeAttrVal),
	)

	// Hash the SET-wrapped attrs (per PKCS#7 §9.3).
	h := sha256.Sum256(derWrap(0x31, attrsContent))
	sig, err := key.Sign(rand.Reader, h[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	var sigAlgDER []byte
	switch key.Public().(type) {
	case *ecdsa.PublicKey:
		sigAlgDER = algIDNoParams(derOIDs.ecdsaWithSHA256)
	default:
		sigAlgDER = algID(derOIDs.rsaEncryption)
	}

	serialDER, _ := asn1.Marshal(cert.SerialNumber)
	return &pkcs7Context{
		signedAttrsImplicit: derContextImplicit(0, attrsContent),
		sigAlgDER:           sigAlgDER,
		sigValue:            sig,
		issuerAndSerial:     derSequence(cert.RawIssuer, serialDER),
		certsDER:            derContextImplicit(0, cert.Raw),
	}, nil
}

// assemblePKCS7 builds the final CMS ContentInfo DER from a prepared signing
// context. tsaToken, if non-nil, is embedded as the id-aa-signatureTimeStampToken
// unsigned attribute in SignerInfo (RFC 3161 §3.4).
func assemblePKCS7(ctx *pkcs7Context, tsaToken []byte) []byte {
	version1, _ := asn1.Marshal(1)
	sigOctet, _ := asn1.Marshal(ctx.sigValue)

	signerInfoParts := [][]byte{
		version1,
		ctx.issuerAndSerial,
		algID(derOIDs.sha256),
		ctx.signedAttrsImplicit,
		ctx.sigAlgDER,
		sigOctet,
	}
	if len(tsaToken) > 0 {
		// unsignedAttrs [1] IMPLICIT containing the timestamp token attribute.
		tsaAttr := attr(derOIDs.sigTimeStampToken, tsaToken)
		signerInfoParts = append(signerInfoParts, derContextImplicit(1, tsaAttr))
	}

	sdVersion, _ := asn1.Marshal(1)
	signedData := derSequence(
		sdVersion,
		derSet(algID(derOIDs.sha256)),
		derSequence(derOIDs.data), // encapContentInfo (detached)
		ctx.certsDER,
		derSet(derSequence(signerInfoParts...)),
	)
	return derSequence(derOIDs.signedData, derContextImplicit(0, signedData))
}

