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

	// The target overrides: every attribute delta Acrobat restored in r1. These
	// are the 296 presence reveals plus the handful of access/locale/h deltas the
	// capture also carries — the generalized builder round-trips all of them.
	r1Form := readFixtureBytes(t, r1FormFixture)
	want, err := xfa.ParseFormPacket(r1Form)
	if err != nil {
		t.Fatalf("ParseFormPacket: %v", err)
	}
	if n := countAttr(want, "presence"); n != 296 {
		t.Fatalf("expected 296 presence overrides from r1, got %d", n)
	}

	// Build the packet from the template + overrides alone — referencing none of
	// Acrobat's tree.
	built, err := xfa.BuildFormPacketWithOverrides(streams.Template.Data, want)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}
	t.Logf("template-built form packet: %d B (Acrobat's was %d B)", len(built), len(r1Form))

	// Round-trip: parsing the built packet must reproduce the input overrides
	// exactly (same paths, same attributes, same values).
	got, err := xfa.ParseFormPacket(built)
	if err != nil {
		t.Fatalf("ParseFormPacket(built): %v", err)
	}
	wantMap, gotMap := overrideMap(want), overrideMap(got)
	if len(gotMap) != len(wantMap) {
		t.Fatalf("round-trip size mismatch: got %d, want %d", len(gotMap), len(wantMap))
	}
	for path, attrs := range wantMap {
		g, ok := gotMap[path]
		if !ok {
			t.Errorf("path %s missing after round-trip", path)
			continue
		}
		// Equality, not containment: the built node must carry exactly the wanted
		// attributes and no extras (e.g. a leaked or duplicated attr Acrobat never
		// authored). Checking every wanted value below plus matching counts here
		// pins the attribute set exactly.
		if len(g) != len(attrs) {
			t.Errorf("path %s attr count mismatch: built %d %v, want %d %v", path, len(g), g, len(attrs), attrs)
		}
		for name, v := range attrs {
			if g[name] != v {
				t.Errorf("path %s attr %s: built %q, want %q", path, name, g[name], v)
			}
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

// countAttr counts overrides that set the named attribute.
func countAttr(overrides []xfa.NodeOverride, name string) int {
	n := 0
	for _, o := range overrides {
		if _, ok := o.Attrs[name]; ok {
			n++
		}
	}
	return n
}

// overrideMap keys overrides by their canonical path string for order-independent
// comparison of two override sets.
func overrideMap(overrides []xfa.NodeOverride) map[string]map[string]string {
	m := make(map[string]map[string]string, len(overrides))
	for _, o := range overrides {
		m[o.Path.String()] = o.Attrs
	}
	return m
}
