package sign

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/parse"
)

// appendDSS appends a /DSS (Document Security Store) dictionary to an
// already-signed PDF as an incremental update. It embeds the provided
// certificate chain and optional OCSP response bytes so that validators can
// verify the signature chain offline without network access (long-term
// validation per ISO 32000-2 §12.8.4.3 / ETSI EN 319 102-2).
//
// pkcs7Bytes is the full CMS SignedData from the signature; its SHA-1 is used
// as the VRI dictionary key.
func appendDSS(signedPDF []byte, pkcs7Bytes []byte, certChain []*x509.Certificate, ocspResponse []byte) ([]byte, error) {
	if len(certChain) == 0 && len(ocspResponse) == 0 {
		return signedPDF, nil
	}

	pdf, err := parse.OpenWithOptions(signedPDF, parse.ParseOptions{})
	if err != nil {
		return nil, fmt.Errorf("DSS: parse: %w", err)
	}
	trailer := pdf.Trailer()
	if trailer == nil {
		return nil, fmt.Errorf("DSS: no trailer")
	}
	var catalogObjNum int
	if _, err := fmt.Sscanf(trailer.RootRef, "%d %d R", &catalogObjNum, new(int)); err != nil {
		return nil, fmt.Errorf("DSS: root ref: %w", err)
	}
	catalogData, err := pdf.GetObjectContent(catalogObjNum)
	if err != nil {
		return nil, fmt.Errorf("DSS: get catalog: %w", err)
	}
	prevXRef := findLastStartXRef(signedPDF)
	if prevXRef < 0 {
		return nil, fmt.Errorf("DSS: no startxref in signed PDF")
	}

	nextObjNum := trailer.Size

	var update bytes.Buffer
	offsets := make(map[int]int64)

	recordOff := func(n int) {
		offsets[n] = int64(len(signedPDF)) + int64(update.Len())
	}

	writeStreamObj := func(n int, data []byte) {
		recordOff(n)
		fmt.Fprintf(&update, "%d 0 obj\n<</Length %d>>\nstream\n", n, len(data))
		update.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			update.WriteByte('\n')
		}
		update.WriteString("endstream\nendobj\n")
	}

	writeDictObj := func(n int, dict string) {
		recordOff(n)
		fmt.Fprintf(&update, "%d 0 obj\n%s\nendobj\n", n, dict)
	}

	// --- Cert stream objects ---
	certObjNums := make([]int, 0, len(certChain))
	for _, cert := range certChain {
		n := nextObjNum
		nextObjNum++
		writeStreamObj(n, cert.Raw)
		certObjNums = append(certObjNums, n)
	}

	// --- OCSP stream object ---
	ocspObjNum := 0
	if len(ocspResponse) > 0 {
		ocspObjNum = nextObjNum
		nextObjNum++
		writeStreamObj(ocspObjNum, ocspResponse)
	}

	// --- VRI dict (key = SHA-1 of full CMS SignedData, uppercase hex) ---
	h := sha1.Sum(pkcs7Bytes)
	vriKey := strings.ToUpper(hex.EncodeToString(h[:]))

	var vri strings.Builder
	vri.WriteString("<<")
	if len(certObjNums) > 0 {
		vri.WriteString("/Cert[")
		for _, n := range certObjNums {
			fmt.Fprintf(&vri, "%d 0 R ", n)
		}
		vri.WriteString("]")
	}
	if ocspObjNum != 0 {
		fmt.Fprintf(&vri, "/OCSP[%d 0 R]", ocspObjNum)
	}
	vri.WriteString(">>")
	vriObjNum := nextObjNum
	nextObjNum++
	writeDictObj(vriObjNum, vri.String())

	// --- DSS dict ---
	var dss strings.Builder
	dss.WriteString("<<")
	if len(certObjNums) > 0 {
		dss.WriteString("/Certs[")
		for _, n := range certObjNums {
			fmt.Fprintf(&dss, "%d 0 R ", n)
		}
		dss.WriteString("]")
	}
	if ocspObjNum != 0 {
		fmt.Fprintf(&dss, "/OCSPs[%d 0 R]", ocspObjNum)
	}
	fmt.Fprintf(&dss, "/VRI<<%s %d 0 R>>", "/"+vriKey, vriObjNum)
	dss.WriteString(">>")
	dssObjNum := nextObjNum
	nextObjNum++
	writeDictObj(dssObjNum, dss.String())

	// --- Updated catalog ---
	recordOff(catalogObjNum)
	fmt.Fprintf(&update, "%d 0 obj\n%s\nendobj\n", catalogObjNum,
		dssInjectDSS(string(catalogData), dssObjNum))

	// --- xref + trailer ---
	xrefStart := int64(len(signedPDF)) + int64(update.Len())
	update.WriteString("xref\n")
	objNums := make([]int, 0, len(offsets))
	for n := range offsets {
		objNums = append(objNums, n)
	}
	sort.Ints(objNums)
	writeXRefSubsections(&update, offsets, objNums)

	update.WriteString("trailer\n<<")
	fmt.Fprintf(&update, "/Size %d/Root %s", nextObjNum, trailer.RootRef)
	if trailer.InfoRef != "" {
		fmt.Fprintf(&update, "/Info %s", trailer.InfoRef)
	}
	fmt.Fprintf(&update, "/Prev %d", prevXRef)
	update.WriteString(">>\n")
	fmt.Fprintf(&update, "startxref\n%d\n%%%%EOF\n", xrefStart)

	out := make([]byte, len(signedPDF)+update.Len())
	copy(out, signedPDF)
	copy(out[len(signedPDF):], update.Bytes())
	return out, nil
}

// dssInjectDSS adds /DSS N 0 R to a catalog dict body (no obj header).
var dssRE = regexp.MustCompile(`/DSS\s+\d+\s+\d+\s+R`)

func dssInjectDSS(catalogBody string, dssObjNum int) string {
	catalogBody = dssRE.ReplaceAllString(catalogBody, "")
	end := strings.LastIndex(catalogBody, ">>")
	if end == -1 {
		return catalogBody
	}
	return catalogBody[:end] + fmt.Sprintf("/DSS %d 0 R", dssObjNum) + catalogBody[end:]
}
