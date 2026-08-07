package tests

import (
	"bytes"
	"testing"

	encrypt "github.com/benedoc-inc/pdfer/v2/core/encrypt"
	"github.com/benedoc-inc/pdfer/v2/forms/xfa"
)

// TestESTAR_BuildFormPacketFromTemplate is the template-walk de-risk. It takes
// r1's presence set (parsed from Acrobat's captured form packet), rebuilds the
// form packet PURELY by walking the template — referencing none of Acrobat's tree
// — and checks the result round-trips and inserts cleanly. (Acrobat acceptance of
// the template-built reveal was proven during the spike; the ongoing guard here is
// the byte-level round-trip plus insertion mechanics.)
func TestESTAR_BuildFormPacketFromTemplate(t *testing.T) {
	pristine := readESTARPDF(t, "nonivd_estar.pdf")
	encInfo, err := encrypt.DecryptPDF(pristine, nil, false)
	if err != nil {
		t.Fatalf("DecryptPDF: %v", err)
	}
	streams, err := xfa.ExtractAllXFAStreams(pristine, encInfo, false)
	if err != nil {
		t.Fatalf("ExtractAllXFAStreams: %v", err)
	}

	// The target presence set: what Acrobat restored in r1.
	r1Form := readFixtureBytes(t, r1FormFixture)
	want, err := xfa.ParseFormPacketPresence(r1Form)
	if err != nil {
		t.Fatalf("ParseFormPacketPresence: %v", err)
	}
	if len(want) != 296 {
		t.Fatalf("expected 296 presence entries from r1, got %d", len(want))
	}

	// Build the packet from the template + presence set alone.
	built, err := xfa.BuildFormPacket(streams.Template.Data, want)
	if err != nil {
		t.Fatalf("BuildFormPacket: %v", err)
	}
	t.Logf("template-built form packet: %d B (Acrobat's was %d B)", len(built), len(r1Form))

	// Round-trip: parsing the built packet's presence must reproduce the input set
	// exactly (same paths, same presence values).
	got, err := xfa.ParseFormPacketPresence(built)
	if err != nil {
		t.Fatalf("ParseFormPacketPresence(built): %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("round-trip size mismatch: got %d, want %d", len(got), len(want))
	}
	for path, p := range want {
		if got[path] != p {
			t.Errorf("path %s: built has presence %q, want %q", path, got[path], p)
		}
	}

	// The template-built packet inserts cleanly as an incremental update. A real,
	// Acrobat-honored checksum is stamped by the caller's export path when it
	// supplies a FormChecksumFunc; here we pass nil and verify only the mechanics.
	out, err := xfa.SetXFAFormPacket(pristine, built, nil, false, nil)
	if err != nil {
		t.Fatalf("SetXFAFormPacket: %v", err)
	}
	if !bytes.HasPrefix(out, pristine) || len(out) <= len(pristine) {
		t.Errorf("template-built packet did not insert as an incremental update")
	}
}
