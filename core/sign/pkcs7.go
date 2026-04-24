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
	oidData            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidSHA256          = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidECDSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidContentType     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
)

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

// marshalOID returns the DER encoding of an OID.
func marshalOID(oid asn1.ObjectIdentifier) []byte {
	b, err := asn1.Marshal(oid)
	if err != nil {
		panic(fmt.Sprintf("marshal oid: %v", err))
	}
	return b
}

// algID encodes AlgorithmIdentifier { oid, NULL }.
// RSA algorithm identifiers conventionally include a NULL parameters field.
func algID(oid asn1.ObjectIdentifier) []byte {
	return derSequence(marshalOID(oid), []byte{0x05, 0x00})
}

// algIDNoParams encodes AlgorithmIdentifier { oid } with no parameters field.
// ECDSA algorithm identifiers (ecdsa-with-SHA256 etc.) MUST omit parameters.
func algIDNoParams(oid asn1.ObjectIdentifier) []byte {
	return derSequence(marshalOID(oid))
}

// attr encodes a PKCS#9 attribute: SEQUENCE { OID, SET { value... } }.
func attr(oid asn1.ObjectIdentifier, values ...[]byte) []byte {
	return derSequence(marshalOID(oid), derSet(values...))
}

// buildDetachedPKCS7 produces a CMS SignedData detached signature.
//
// digest is SHA-256 of the byte ranges to be signed (the two regions outside
// the /Contents hex string). The signature is detached — no content is
// embedded in the structure.
func buildDetachedPKCS7(cert *x509.Certificate, key crypto.Signer, digest []byte, t time.Time) ([]byte, error) {
	// ---- signed attributes ----
	// id-contentType { id-data }
	contentTypeAttrVal, _ := asn1.Marshal(oidData)
	contentTypeAttr := attr(oidContentType, contentTypeAttrVal)

	// id-messageDigest { OCTET STRING(digest) }
	digestAttrVal, _ := asn1.Marshal(digest)
	digestAttr := attr(oidMessageDigest, digestAttrVal)

	// id-signingTime { UTCTime }
	signingTimeAttrVal, _ := asn1.Marshal(t.UTC())
	signingTimeAttr := attr(oidSigningTime, signingTimeAttrVal)

	// The inner content of signedAttrs (concatenated attribute encodings).
	// This content gets wrapped in two different outer tags:
	//   0x31  (SET)   — for computing the hash that RSA signs
	//   0xa0  ([0])   — for embedding in SignerInfo
	attrsContent := cat(contentTypeAttr, digestAttr, signingTimeAttr)

	// Hash the SET-wrapped attrs (per PKCS#7 §9.3).
	signedAttrsForSigning := derWrap(0x31, attrsContent)
	h := sha256.Sum256(signedAttrsForSigning)
	sig, err := key.Sign(rand.Reader, h[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("RSA sign: %w", err)
	}

	// [0] IMPLICIT tag for embedding in SignerInfo.
	signedAttrsImplicit := derContextImplicit(0, attrsContent)

	// ---- SignerInfo ----
	// Choose signature AlgorithmIdentifier based on key type.
	// ECDSA: ecdsa-with-SHA256 with no parameters (RFC 5754 §3.2).
	// RSA:   rsaEncryption with NULL parameters (PKCS#1).
	var sigAlgDER []byte
	switch key.Public().(type) {
	case *ecdsa.PublicKey:
		sigAlgDER = algIDNoParams(oidECDSAWithSHA256)
	default:
		sigAlgDER = algID(oidRSAEncryption)
	}

	version1, _ := asn1.Marshal(1)
	serialDER, _ := asn1.Marshal(cert.SerialNumber)
	issuerAndSerial := derSequence(cert.RawIssuer, serialDER)
	sigOctet, _ := asn1.Marshal(sig)

	signerInfo := derSequence(
		version1,
		issuerAndSerial,
		algID(oidSHA256),
		signedAttrsImplicit,
		sigAlgDER,
		sigOctet,
	)

	// ---- SignedData ----
	sdVersion, _ := asn1.Marshal(1)

	// encapContentInfo: SEQUENCE { id-data }  (no content — detached)
	encapContentInfo := derSequence(marshalOID(oidData))

	// certificates: [0] IMPLICIT containing one DER certificate
	certsDER := derContextImplicit(0, cert.Raw)

	signedData := derSequence(
		sdVersion,
		derSet(algID(oidSHA256)), // digestAlgorithms
		encapContentInfo,
		certsDER,
		derSet(signerInfo), // signerInfos
	)

	// ---- ContentInfo ----
	oidSignedDataDER := marshalOID(oidSignedData)
	contentExplicit := derContextImplicit(0, signedData)
	return derSequence(oidSignedDataDER, contentExplicit), nil
}
