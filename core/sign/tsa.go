package sign

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"io"
	"net/http"
)

// oidSHA256TSA is SHA-256 expressed as an AlgorithmIdentifier OID, used in TSA
// MessageImprint structures (same value as oidSHA256 but kept separate for clarity).
var oidSHA256TSA = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}

type tsaMsgImprint struct {
	HashAlgorithm pkix.AlgorithmIdentifier
	HashedMessage []byte
}

type tsaReqASN1 struct {
	Version        int
	MessageImprint tsaMsgImprint
	CertReq        bool `asn1:"optional"`
}

type tsaStatusASN1 struct {
	Status int
}

type tsaRespASN1 struct {
	Status         tsaStatusASN1
	TimeStampToken asn1.RawValue `asn1:"optional"`
}

// requestTSAToken sends an RFC 3161 timestamp request to endpoint for sigValue
// (the raw bytes of the SignerInfo.signature field) and returns the
// TimeStampToken (a DER-encoded ContentInfo) from the response.
//
// PKIStatus 0 (granted) and 1 (grantedWithMods) are both treated as success.
func requestTSAToken(endpoint string, sigValue []byte) ([]byte, error) {
	h := sha256.Sum256(sigValue)
	req := tsaReqASN1{
		Version: 1,
		MessageImprint: tsaMsgImprint{
			HashAlgorithm: pkix.AlgorithmIdentifier{Algorithm: oidSHA256TSA},
			HashedMessage: h[:],
		},
		CertReq: true,
	}
	reqDER, err := asn1.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal TSA request: %w", err)
	}

	resp, err := http.Post(endpoint, "application/timestamp-query", bytes.NewReader(reqDER))
	if err != nil {
		return nil, fmt.Errorf("TSA HTTP POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TSA returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read TSA response: %w", err)
	}

	var tsaResp tsaRespASN1
	if _, err := asn1.Unmarshal(body, &tsaResp); err != nil {
		return nil, fmt.Errorf("parse TSA response ASN.1: %w", err)
	}
	if s := tsaResp.Status.Status; s != 0 && s != 1 {
		return nil, fmt.Errorf("TSA PKIStatus %d — not granted", s)
	}
	if len(tsaResp.TimeStampToken.FullBytes) == 0 {
		return nil, fmt.Errorf("TSA response contains no TimeStampToken")
	}
	return tsaResp.TimeStampToken.FullBytes, nil
}
