package sign_test

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/sign"
)

// The PAdES SubFilter, and the fact that validation still finds the signature.
//
// ⚠ THE SECOND HALF IS THE EASY REGRESSION. ValidateSignatures locates
// signatures by matching the signature dictionary, and its matcher knew only
// /adbe.pkcs7.detached — so turning the flag on without widening it would make
// pdfer unable to read back the very signatures it had just written, reporting
// a correctly sealed document as unsigned.
func TestPAdESSubFilterIsWrittenAndStillValidates(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, err := sign.GenerateTestCredentials()
	if err != nil {
		t.Fatal(err)
	}

	legacy, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(legacy, []byte("/SubFilter/adbe.pkcs7.detached")) {
		t.Error("the default is no longer the legacy SubFilter")
	}

	pades, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key, PAdES: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pades, []byte("/SubFilter/ETSI.CAdES.detached")) {
		t.Fatal("PAdES: true did not write the ETSI SubFilter")
	}
	if bytes.Contains(pades, []byte("/SubFilter/adbe.pkcs7.detached")) {
		t.Error("both SubFilters are present")
	}

	for name, doc := range map[string][]byte{"legacy": legacy, "pades": pades} {
		sigs, err := sign.ValidateSignatures(doc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(sigs) != 1 {
			t.Fatalf("%s: found %d signatures, want 1", name, len(sigs))
		}
		if !sigs[0].Valid {
			t.Errorf("%s: signature does not verify: %s", name, sigs[0].Error)
		}
	}
}
