package xfa

import (
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

// TestSOMSegment_NoCollision_BareName: when a parent has no same-named
// siblings, somSegment returns the bare name (no "[i]" suffix). This keeps
// emitted SOM paths backward-compatible for the overwhelmingly common case.
func TestSOMSegment_NoCollision_BareName(t *testing.T) {
	parent := &xfaNode{Name: "form1"}
	a := &xfaNode{Name: "alpha"}
	b := &xfaNode{Name: "beta"}
	parent.Children = []*xfaNode{a, b}

	if got := somSegment(parent, a); got != "alpha" {
		t.Errorf("somSegment(form1, alpha) = %q, want %q", got, "alpha")
	}
	if got := somSegment(parent, b); got != "beta" {
		t.Errorf("somSegment(form1, beta) = %q, want %q", got, "beta")
	}
}

// TestSOMSegment_Collision_IndexedBoth: when two or more siblings share a name,
// every collider — including the first — receives a "[i]" suffix. The all-or-
// none rule (rather than skipping i=0) keeps paths unambiguous and consistent.
func TestSOMSegment_Collision_IndexedBoth(t *testing.T) {
	parent := &xfaNode{Name: "form1"}
	a := &xfaNode{Name: "addr"}
	b := &xfaNode{Name: "addr"}
	c := &xfaNode{Name: "phone"}
	parent.Children = []*xfaNode{a, b, c}

	if got := somSegment(parent, a); got != "addr[0]" {
		t.Errorf("first addr: got %q, want %q", got, "addr[0]")
	}
	if got := somSegment(parent, b); got != "addr[1]" {
		t.Errorf("second addr: got %q, want %q", got, "addr[1]")
	}
	if got := somSegment(parent, c); got != "phone" {
		t.Errorf("unique phone: got %q, want %q", got, "phone")
	}
}

// TestSOMSegment_AnonymousSiblingsIgnored: anonymous (Name=="") siblings are
// not SOM-addressable and must not perturb the index of named siblings.
func TestSOMSegment_AnonymousSiblingsIgnored(t *testing.T) {
	parent := &xfaNode{Name: "form1"}
	anon := &xfaNode{Name: ""}
	a := &xfaNode{Name: "field"}
	parent.Children = []*xfaNode{anon, a}

	if got := somSegment(parent, a); got != "field" {
		t.Errorf("got %q, want %q (anonymous sibling should not trigger indexing)", got, "field")
	}
}

// TestSOMSegment_NilParent_BareName: top-level nodes have no parent context;
// the bare name is returned and no count is attempted.
func TestSOMSegment_NilParent_BareName(t *testing.T) {
	a := &xfaNode{Name: "root"}
	if got := somSegment(nil, a); got != "root" {
		t.Errorf("got %q, want %q", got, "root")
	}
}

// TestSOMSegmentFromSiblingNames_Collision: the name-slice variant (used for
// flattened exclGroup option fields, where the *xfaNode references are gone)
// applies the same all-or-none indexing rule.
func TestSOMSegmentFromSiblingNames_Collision(t *testing.T) {
	siblings := []string{"opt", "opt", "other"}

	if got := somSegmentFromSiblingNames(siblings, 0); got != "opt[0]" {
		t.Errorf("i=0: got %q, want %q", got, "opt[0]")
	}
	if got := somSegmentFromSiblingNames(siblings, 1); got != "opt[1]" {
		t.Errorf("i=1: got %q, want %q", got, "opt[1]")
	}
	if got := somSegmentFromSiblingNames(siblings, 2); got != "other" {
		t.Errorf("i=2: got %q, want %q", got, "other")
	}
}

// TestSOMJoin_DropsEmpty: somJoin drops empty segments so an anonymous frame
// in the middle of a path doesn't produce a stray "..".
func TestSOMJoin_DropsEmpty(t *testing.T) {
	if got := somJoin([]string{"a", "", "b"}); got != "a.b" {
		t.Errorf("got %q, want %q", got, "a.b")
	}
	if got := somJoin(nil); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

// TestSiblingDisambiguation_EndToEnd_QuestionAndScripts is the regression test
// for the original bug: two <field>s with the same name in the same subform
// must produce distinct OwnerPaths and distinct SOMSegments, and a script
// attached to the second field must not fan out to the first.
func TestSiblingDisambiguation_EndToEnd_QuestionAndScripts(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="addr">
      <field name="line1"><ui><textEdit/></ui></field>
    </subform>
    <subform name="addr">
      <field name="line1">
        <ui><textEdit/></ui>
        <event activity="change">
          <script contentType="application/x-javascript">app.alert("hi");</script>
        </event>
      </field>
    </subform>
    <subform name="phone">
      <field name="number"><ui><textEdit/></ui></field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	// FormSection.Path: both addr subforms must be distinct; phone bare.
	gotSectionPaths := collectSectionPaths(form.Sections)
	wantPaths := map[string]bool{
		"form1":         true,
		"form1.addr[0]": true,
		"form1.addr[1]": true,
		"form1.phone":   true,
	}
	for want := range wantPaths {
		if !gotSectionPaths[want] {
			t.Errorf("missing section Path %q (got: %v)", want, keys(gotSectionPaths))
		}
	}
	if gotSectionPaths["form1.addr"] {
		t.Errorf("found ambiguous section Path %q — same-named siblings must be indexed", "form1.addr")
	}

	// Question.SOMSegment: both "line1" fields under colliding parents are
	// themselves unique within their respective parents, so their segment is
	// bare "line1". The disambiguation lives in their section Path.
	for _, q := range form.Questions {
		if q.Name == "line1" && q.SOMSegment != "line1" {
			t.Errorf("line1 question SOMSegment = %q, want %q", q.SOMSegment, "line1")
		}
	}

	// Script back-ref: the change event must attach uniquely to the second
	// addr's line1, never to the first.
	if s := findScript(form.Scripts, "form1.addr[1].line1", "change"); s == nil {
		t.Fatalf("missing script with OwnerPath=form1.addr[1].line1 event=change; got %d scripts", len(form.Scripts))
	}
	if s := findScript(form.Scripts, "form1.addr[0].line1", "change"); s != nil {
		t.Errorf("script erroneously attributed to form1.addr[0].line1")
	}
	if s := findScript(form.Scripts, "form1.addr.line1", "change"); s != nil {
		t.Errorf("script emitted with ambiguous (un-indexed) OwnerPath form1.addr.line1")
	}
}

// TestSiblingDisambiguation_FieldLevel: collision at the field level (two
// fields with the same name sharing one parent subform) produces indexed
// SOMSegments on the Questions themselves, and the script back-ref attributes
// to the right one.
func TestSiblingDisambiguation_FieldLevel(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <field name="dup"><ui><textEdit/></ui></field>
    <field name="dup">
      <ui><textEdit/></ui>
      <event activity="change">
        <script contentType="application/x-javascript">noop();</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var segs []string
	for _, q := range form.Questions {
		if q.Name == "dup" {
			segs = append(segs, q.SOMSegment)
		}
	}
	wantSegs := []string{"dup[0]", "dup[1]"}
	if len(segs) != len(wantSegs) {
		t.Fatalf("expected %d dup questions, got %d (%v)", len(wantSegs), len(segs), segs)
	}
	for i, want := range wantSegs {
		if segs[i] != want {
			t.Errorf("dup[%d].SOMSegment = %q, want %q", i, segs[i], want)
		}
	}

	if s := findScript(form.Scripts, "form1.dup[1]", "change"); s == nil {
		t.Fatalf("missing script OwnerPath=form1.dup[1]; got scripts: %+v", form.Scripts)
	}
	if s := findScript(form.Scripts, "form1.dup[0]", "change"); s != nil {
		t.Errorf("script erroneously attached to form1.dup[0]")
	}
}

func collectSectionPaths(secs []types.FormSection) map[string]bool {
	out := map[string]bool{}
	var walk func([]types.FormSection)
	walk = func(s []types.FormSection) {
		for _, sec := range s {
			out[sec.Path] = true
			walk(sec.Children)
		}
	}
	walk(secs)
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
