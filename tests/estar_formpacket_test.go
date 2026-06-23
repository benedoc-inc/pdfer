package tests

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	encrypt "github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/forms/xfa"
)

// Revealed-state form packet captured from nonivd_estar.pdf after a user revealed
// sections in Acrobat (r1): 153 presence="visible" deltas the as-distributed packet
// lacks. Used here as a realistic packet to insert. Computing a real, Acrobat-honored
// digest is out of pdfer's scope, so these tests exercise the insertion seam with a
// stub FormChecksumFunc.
const r1FormFixture = "resources/nonivd_estar_r1_form.bin"

var storedChecksumRE = regexp.MustCompile(`checksum="([^"]*)"`)

func readFixtureBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// TestESTAR_InjectRevealedFormPacket replaces the as-distributed eSTAR's form
// packet with the r1 revealed-state delta through pdfer and verifies the insertion
// mechanics: the incremental-update invariant, that the checksummer is handed the
// PDF's real template+datasets, that the swap took, that its returned digest is
// stamped onto <form>, and that the datasets are left untouched.
func TestESTAR_InjectRevealedFormPacket(t *testing.T) {
	pristine := readESTARPDF(t, "nonivd_estar.pdf")
	r1Form := readFixtureBytes(t, r1FormFixture)

	const stubChecksum = "STUBCHECKSUMAAAAAAAAAAAAAAA="
	var gotTemplate, gotDatasets []byte
	stub := func(template, datasets []byte) (string, error) {
		gotTemplate, gotDatasets = template, datasets
		return stubChecksum, nil
	}

	out, err := xfa.SetXFAFormPacket(pristine, r1Form, nil, false, stub)
	if err != nil {
		t.Fatalf("SetXFAFormPacket: %v", err)
	}

	// Incremental-update invariant: the original bytes are an exact prefix of the
	// result (we appended a new revision, we did not rewrite the document).
	if !bytes.HasPrefix(out, pristine) {
		t.Errorf("output is not an incremental update: original bytes are not a prefix")
	}
	if len(out) <= len(pristine) {
		t.Errorf("output (%d B) did not grow over original (%d B)", len(out), len(pristine))
	}

	encInfo, err := encrypt.DecryptPDF(out, nil, false)
	if err != nil {
		t.Fatalf("DecryptPDF(result): %v", err)
	}
	streams, err := xfa.ExtractAllXFAStreams(out, encInfo, false)
	if err != nil {
		t.Fatalf("ExtractAllXFAStreams(result): %v", err)
	}

	// The checksummer was invoked with the PDF's actual template + datasets packets.
	if !bytes.Equal(gotTemplate, streams.Template.Data) {
		t.Errorf("checksum func got template %d B, want %d B", len(gotTemplate), len(streams.Template.Data))
	}
	if !bytes.Equal(gotDatasets, streams.Datasets.Data) {
		t.Errorf("checksum func got datasets %d B, want %d B", len(gotDatasets), len(streams.Datasets.Data))
	}

	form := streams.Form
	if form == nil {
		t.Fatal("result has no form packet")
	}
	// The revealed deltas are present.
	if n := bytes.Count(form.Data, []byte(`presence="visible"`)); n < 100 {
		t.Errorf("expected the revealed form packet (>=100 visible deltas), got %d", n)
	}
	// The injected digest is stamped onto the form packet.
	m := storedChecksumRE.FindSubmatch(form.Data)
	if m == nil {
		t.Fatal("result form packet has no checksum attribute")
	}
	if got := string(m[1]); got != stubChecksum {
		t.Errorf("stamped checksum\n got: %s\nwant: %s", got, stubChecksum)
	}

	// Datasets must be untouched (we only swapped the form packet).
	pEnc, _ := encrypt.DecryptPDF(pristine, nil, false)
	pStreams, err := xfa.ExtractAllXFAStreams(pristine, pEnc, false)
	if err != nil {
		t.Fatalf("ExtractAllXFAStreams(pristine): %v", err)
	}
	if !bytes.Equal(streams.Datasets.Data, pStreams.Datasets.Data) {
		t.Errorf("datasets packet changed; expected it untouched")
	}
}

// TestESTAR_InjectFormPacket_NilChecksum verifies the nil-checksummer path: pdfer
// carries no algorithm to recompute the digest, so it must leave whatever checksum
// the form packet already carries untouched.
func TestESTAR_InjectFormPacket_NilChecksum(t *testing.T) {
	pristine := readESTARPDF(t, "nonivd_estar.pdf")
	r1Form := readFixtureBytes(t, r1FormFixture)

	m := storedChecksumRE.FindSubmatch(r1Form)
	if m == nil {
		t.Fatal("r1 form fixture has no checksum attribute")
	}
	want := string(m[1])

	out, err := xfa.SetXFAFormPacket(pristine, r1Form, nil, false, nil)
	if err != nil {
		t.Fatalf("SetXFAFormPacket(nil checksum): %v", err)
	}
	encInfo, err := encrypt.DecryptPDF(out, nil, false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	streams, err := xfa.ExtractAllXFAStreams(out, encInfo, false)
	if err != nil {
		t.Fatalf("ExtractAllXFAStreams: %v", err)
	}
	form := streams.Form
	if form == nil {
		t.Fatal("result has no form packet")
	}
	m2 := storedChecksumRE.FindSubmatch(form.Data)
	if m2 == nil {
		t.Fatal("result form packet has no checksum attribute")
	}
	if got := string(m2[1]); got != want {
		t.Errorf("nil checksummer should preserve the existing checksum\n got: %s\nwant: %s", got, want)
	}
}
