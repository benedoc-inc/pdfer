package xfa

import (
	"strings"
	"testing"
)

// synthTemplate is a minimal XFA template with the two structures a SOM dot-path
// cannot address but a NodePath can: a repeating subform (two "Rep" instances,
// each with a field "A") and two anonymous draws. The form-packet builder must
// keep these distinct by index.
const synthTemplate = `<template xmlns="http://www.xfa.org/schema/xfa-template/4.0/">
  <subform name="root">
    <subform name="Rep"><field name="A"></field></subform>
    <subform name="Rep"><field name="A"></field></subform>
    <draw></draw>
    <draw></draw>
  </subform>
</template>`

func path(segs ...NodeSeg) NodePath { return NodePath(segs) }

func sub(name string, idx int) NodeSeg { return NodeSeg{Name: name, Type: "subform", Index: idx} }
func fld(name string, idx int) NodeSeg { return NodeSeg{Name: name, Type: "field", Index: idx} }
func draw(idx int) NodeSeg             { return NodeSeg{Type: "draw", Index: idx} }

// TestBuildFormPacket_DistinguishesRepeatingInstances is the correctness proof:
// two instances of the same repeating subform get different presence, and the
// builder keeps them apart by NodeSeg.Index. A SOM dot-path ("root.Rep.A") would
// collide both onto one key and lose one of the deltas.
func TestBuildFormPacket_DistinguishesRepeatingInstances(t *testing.T) {
	overrides := []NodeOverride{
		{Path: path(sub("root", 0), sub("Rep", 0), fld("A", 0)), Attrs: map[string]string{"presence": "hidden"}},
		{Path: path(sub("root", 0), sub("Rep", 1), fld("A", 0)), Attrs: map[string]string{"presence": "visible"}},
	}

	built, err := BuildFormPacketWithOverrides([]byte(synthTemplate), overrides)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}

	got, err := ParseFormPacket(built)
	if err != nil {
		t.Fatalf("ParseFormPacket: %v", err)
	}
	byPath := map[string]string{}
	for _, o := range got {
		byPath[o.Path.String()] = o.Attrs["presence"]
	}
	if byPath["root[0]/Rep[0]/A[0]"] != "hidden" {
		t.Errorf("Rep[0].A presence = %q, want hidden", byPath["root[0]/Rep[0]/A[0]"])
	}
	if byPath["root[0]/Rep[1]/A[0]"] != "visible" {
		t.Errorf("Rep[1].A presence = %q, want visible", byPath["root[0]/Rep[1]/A[0]"])
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 overrides round-tripped, got %d: %v", len(got), byPath)
	}
}

// TestBuildFormPacket_MultiAttrSortedAndAnonymous covers two things in one packet:
// multiple attributes on a single node are all emitted (deterministically
// sorted), and an anonymous node is addressable purely by its type + index.
func TestBuildFormPacket_MultiAttrSortedAndAnonymous(t *testing.T) {
	overrides := []NodeOverride{
		{Path: path(sub("root", 0), sub("Rep", 0), fld("A", 0)), Attrs: map[string]string{
			"presence": "visible",
			"access":   "readOnly",
		}},
		{Path: path(sub("root", 0), draw(1)), Attrs: map[string]string{"presence": "visible"}},
	}

	built, err := BuildFormPacketWithOverrides([]byte(synthTemplate), overrides)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}
	s := string(built)

	// Both attributes present, and emitted in sorted (access before presence) order.
	if !strings.Contains(s, `<field name="A" access="readOnly" presence="visible">`) {
		t.Errorf("multi-attr field not emitted as expected; got:\n%s", s)
	}
	// The anonymous draw at index 1 carries the override. Index 0 is emitted as an
	// empty placeholder (no attributes) so draw[1] keeps its position on re-parse.
	if !strings.Contains(s, `<draw presence="visible">`) {
		t.Errorf("anonymous draw[1] not emitted; got:\n%s", s)
	}
	if !strings.Contains(s, `<draw></draw><draw presence="visible">`) {
		t.Errorf("expected an empty placeholder draw[0] before draw[1]; got:\n%s", s)
	}

	got, err := ParseFormPacket(built)
	if err != nil {
		t.Fatalf("ParseFormPacket: %v", err)
	}
	byPath := map[string]map[string]string{}
	for _, o := range got {
		byPath[o.Path.String()] = o.Attrs
	}
	if a := byPath["root[0]/Rep[0]/A[0]"]; a["access"] != "readOnly" || a["presence"] != "visible" {
		t.Errorf("field A attrs round-trip mismatch: %v", a)
	}
	if _, ok := byPath["root[0]/#draw[1]"]; !ok {
		t.Errorf("anonymous draw[1] missing after round-trip; got paths %v", byPath)
	}
}

// TestDeprecatedPresenceShims guards the v2.8.0 source-compatible surface:
// PresenceSet + BuildFormPacket + ParseFormPacketPresence must still build and
// round-trip a presence-only packet, and produce the same bytes as the general
// builder fed equivalent presence overrides.
func TestDeprecatedPresenceShims(t *testing.T) {
	set := PresenceSet{
		"root[0]/Rep[0]/A[0]": "hidden",
		"root[0]/Rep[1]/A[0]": "visible",
		"root[0]/#draw[1]":    "invisible",
	}

	built, err := BuildFormPacket([]byte(synthTemplate), set)
	if err != nil {
		t.Fatalf("BuildFormPacket (deprecated): %v", err)
	}

	got, err := ParseFormPacketPresence(built)
	if err != nil {
		t.Fatalf("ParseFormPacketPresence (deprecated): %v", err)
	}
	if len(got) != len(set) {
		t.Fatalf("round-trip size mismatch: got %d, want %d", len(got), len(set))
	}
	for p, presence := range set {
		if got[p] != presence {
			t.Errorf("path %s: got presence %q, want %q", p, got[p], presence)
		}
	}

	// The shim must agree byte-for-byte with the general builder on presence-only input.
	overrides := []NodeOverride{
		{Path: path(sub("root", 0), sub("Rep", 0), fld("A", 0)), Attrs: map[string]string{"presence": "hidden"}},
		{Path: path(sub("root", 0), sub("Rep", 1), fld("A", 0)), Attrs: map[string]string{"presence": "visible"}},
		{Path: path(sub("root", 0), draw(1)), Attrs: map[string]string{"presence": "invisible"}},
	}
	viaGeneral, err := BuildFormPacketWithOverrides([]byte(synthTemplate), overrides)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}
	if string(built) != string(viaGeneral) {
		t.Errorf("deprecated shim diverged from general builder:\n   shim: %s\ngeneral: %s", built, viaGeneral)
	}
}

// TestFormPacket_NamespacedAttrRoundTrips guards that a namespace-prefixed
// attribute keeps its prefix through parse and build. Keying by local name alone
// would rewrite xml:lang as lang — colliding it with a bare lang on the same node
// (last-write-wins silent loss) and breaking the documented inverse round-trip.
func TestFormPacket_NamespacedAttrRoundTrips(t *testing.T) {
	const formPacket = `<form xmlns="http://www.xfa.org/schema/xfa-form/2.8/">` +
		`<subform name="root"><field name="A" xml:lang="en" lang="x" presence="hidden"></field></subform>` +
		`</form>`

	got, err := ParseFormPacket([]byte(formPacket))
	if err != nil {
		t.Fatalf("ParseFormPacket: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 override, got %d: %v", len(got), got)
	}
	if a := got[0].Attrs; a["xml:lang"] != "en" || a["lang"] != "x" || a["presence"] != "hidden" {
		t.Fatalf("namespaced and bare attrs not both preserved: %v", a)
	}

	// The namespaced override must build back out (validateAttrName allows the
	// prefix colon) and survive a rebuild + re-parse unchanged.
	const tmpl = `<template xmlns="http://www.xfa.org/schema/xfa-template/4.0/">` +
		`<subform name="root"><field name="A"></field></subform></template>`
	built, err := BuildFormPacketWithOverrides([]byte(tmpl), got)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}
	if !strings.Contains(string(built), `xml:lang="en"`) {
		t.Errorf("namespaced attr not emitted on rebuild: %s", built)
	}
	round, err := ParseFormPacket(built)
	if err != nil {
		t.Fatalf("ParseFormPacket(built): %v", err)
	}
	if len(round) != 1 || round[0].Attrs["xml:lang"] != "en" || round[0].Attrs["lang"] != "x" {
		t.Errorf("namespaced attr did not round-trip through build: %v", round)
	}
}

func TestBuildFormPacket_Errors(t *testing.T) {
	tests := []struct {
		name      string
		overrides []NodeOverride
		wantErr   string
	}{
		{
			name:      "unmatched path",
			overrides: []NodeOverride{{Path: path(sub("root", 0), sub("Nope", 0)), Attrs: map[string]string{"presence": "visible"}}},
			wantErr:   "did not match any template node",
		},
		{
			name:      "instance beyond template",
			overrides: []NodeOverride{{Path: path(sub("root", 0), draw(5)), Attrs: map[string]string{"presence": "visible"}}},
			wantErr:   "did not match any template node",
		},
		{
			name: "duplicate path",
			overrides: []NodeOverride{
				{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{"presence": "visible"}},
				{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{"presence": "hidden"}},
			},
			wantErr: "duplicate path",
		},
		{
			name:      "empty attrs",
			overrides: []NodeOverride{{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{}}},
			wantErr:   "no attributes to set",
		},
		{
			name:      "reserved name attr",
			overrides: []NodeOverride{{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{"name": "x"}}},
			wantErr:   "reserved",
		},
		{
			name:      "invalid attr name",
			overrides: []NodeOverride{{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{"pre sence": "visible"}}},
			wantErr:   "invalid character",
		},
		{
			name:      "empty path",
			overrides: []NodeOverride{{Path: NodePath{}, Attrs: map[string]string{"presence": "visible"}}},
			wantErr:   "empty node path",
		},
		{
			name:      "malformed segment",
			overrides: []NodeOverride{{Path: path(NodeSeg{Index: 0}), Attrs: map[string]string{"presence": "visible"}}},
			wantErr:   "neither Name nor Type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildFormPacketWithOverrides([]byte(synthTemplate), tc.overrides)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestBuildFormPacket_AttributeValuesEscaped guards that special characters in an
// attribute value are XML-escaped so the emitted packet stays well-formed.
func TestBuildFormPacket_AttributeValuesEscaped(t *testing.T) {
	overrides := []NodeOverride{
		{Path: path(sub("root", 0), draw(0)), Attrs: map[string]string{"presence": `a"<&b`}},
	}
	built, err := BuildFormPacketWithOverrides([]byte(synthTemplate), overrides)
	if err != nil {
		t.Fatalf("BuildFormPacketWithOverrides: %v", err)
	}
	if strings.Contains(string(built), `presence="a"<&b"`) {
		t.Errorf("attribute value not escaped: %s", built)
	}
	got, err := ParseFormPacket(built)
	if err != nil {
		t.Fatalf("ParseFormPacket: %v", err)
	}
	if len(got) != 1 || got[0].Attrs["presence"] != `a"<&b` {
		t.Errorf("escaped value did not round-trip: %v", got)
	}
}
