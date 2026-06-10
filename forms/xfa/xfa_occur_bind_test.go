package xfa

import (
	"os"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

// findSectionByName returns the first section with the given Name via
// depth-first search, or nil.
func findSectionByName(secs []types.FormSection, name string) *types.FormSection {
	for i := range secs {
		if secs[i].Name == name {
			return &secs[i]
		}
		if found := findSectionByName(secs[i].Children, name); found != nil {
			return found
		}
	}
	return nil
}

// findQuestionByName returns the first question with the given Name, or nil.
func findQuestionByName(qs []types.Question, name string) *types.Question {
	for i := range qs {
		if qs[i].Name == name {
			return &qs[i]
		}
	}
	return nil
}

// TestOccurNormalization verifies that <occur> attributes are normalized to
// XFA spec defaults (min=1; max=min; initial=min; max="-1" unbounded) and that
// sections without an <occur> element report nil.
func TestOccurNormalization(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="full">
      <occur min="2" max="5" initial="3"/>
      <field name="a"><ui><textEdit/></ui></field>
    </subform>
    <subform name="maxOnly">
      <occur max="-1"/>
      <field name="b"><ui><textEdit/></ui></field>
    </subform>
    <subform name="minZero">
      <occur min="0"/>
      <field name="c"><ui><textEdit/></ui></field>
    </subform>
    <subform name="plain">
      <field name="d"><ui><textEdit/></ui></field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	cases := []struct {
		section string
		want    *types.Occur
	}{
		{"full", &types.Occur{Min: 2, Max: 5, Initial: 3}},
		{"maxOnly", &types.Occur{Min: 1, Max: -1, Initial: 1}},
		{"minZero", &types.Occur{Min: 0, Max: 0, Initial: 0}},
		{"plain", nil},
	}
	for _, c := range cases {
		sec := findSectionByName(form.Sections, c.section)
		if sec == nil {
			t.Errorf("section %q not found", c.section)
			continue
		}
		if c.want == nil {
			if sec.Occur != nil {
				t.Errorf("section %q Occur = %+v, want nil", c.section, *sec.Occur)
			}
			continue
		}
		if sec.Occur == nil {
			t.Errorf("section %q Occur = nil, want %+v", c.section, *c.want)
			continue
		}
		if *sec.Occur != *c.want {
			t.Errorf("section %q Occur = %+v, want %+v", c.section, *sec.Occur, *c.want)
		}
	}
}

// TestOccurOnField verifies that an <occur> inside a <field> attaches to the
// field's Question rather than the enclosing subform.
func TestOccurOnField(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <field name="repeating">
      <occur max="3"/>
      <ui><textEdit/></ui>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	q := findQuestionByName(form.Questions, "repeating")
	if q == nil {
		t.Fatal("question 'repeating' not found")
	}
	want := types.Occur{Min: 1, Max: 3, Initial: 1}
	if q.Occur == nil || *q.Occur != want {
		t.Errorf("question Occur = %+v, want %+v", q.Occur, want)
	}
	sec := findSectionByName(form.Sections, "form1")
	if sec == nil {
		t.Fatal("section 'form1' not found")
	}
	if sec.Occur != nil {
		t.Errorf("field-level <occur> leaked onto section: %+v", *sec.Occur)
	}
}

// TestBindMetadataOnFields verifies that <bind> match/ref attributes surface
// on Questions, with Match defaulting to "once" when the attribute is absent.
func TestBindMetadataOnFields(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <field name="dataRefField">
      <ui><textEdit/></ui>
      <bind match="dataRef" ref="$record.applicant.name"/>
    </field>
    <field name="bareRefField">
      <ui><textEdit/></ui>
      <bind ref="$record.other"/>
    </field>
    <field name="globalField">
      <ui><textEdit/></ui>
      <bind match="global"/>
    </field>
    <field name="plainField">
      <ui><textEdit/></ui>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	cases := []struct {
		field string
		want  *types.Bind
	}{
		{"dataRefField", &types.Bind{Match: "dataRef", Ref: "$record.applicant.name"}},
		{"bareRefField", &types.Bind{Match: "once", Ref: "$record.other"}},
		{"globalField", &types.Bind{Match: "global"}},
		{"plainField", nil},
	}
	for _, c := range cases {
		q := findQuestionByName(form.Questions, c.field)
		if q == nil {
			t.Errorf("question %q not found", c.field)
			continue
		}
		if c.want == nil {
			if q.Bind != nil {
				t.Errorf("question %q Bind = %+v, want nil", c.field, *q.Bind)
			}
			continue
		}
		if q.Bind == nil {
			t.Errorf("question %q Bind = nil, want %+v", c.field, *c.want)
			continue
		}
		if *q.Bind != *c.want {
			t.Errorf("question %q Bind = %+v, want %+v", c.field, *q.Bind, *c.want)
		}
	}
}

// TestBindNoneFieldStillFiltered verifies that a bind="none" leaf keeps its
// existing emission behavior (surfaced as a display question when it has
// text) and additionally carries the Bind metadata.
func TestBindNoneFieldStillFiltered(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <field name="labelField">
      <ui><textEdit/></ui>
      <bind match="none"/>
      <caption><value><text>Static label</text></value></caption>
    </field>
    <field name="real">
      <ui><textEdit/></ui>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	q := findQuestionByName(form.Questions, "labelField")
	if q == nil {
		t.Fatal("bind='none' field with caption should surface as a display question")
	}
	if q.Type != types.ResponseTypeDisplay {
		t.Errorf("type = %q, want display (bind='none' filter must stay keyed to the leaf Bind string)", q.Type)
	}
	if q.Bind == nil || q.Bind.Match != "none" {
		t.Errorf("Bind = %+v, want Match 'none'", q.Bind)
	}
}

// TestSubformBindNoneRecordedNotFiltered verifies that a subform-level
// <bind match="none"> is recorded as metadata only — the section and its
// child questions are still emitted (the bind="none" emission filter is
// leaf-only and must not leak to containers).
func TestSubformBindNoneRecordedNotFiltered(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="noBind">
      <bind match="none"/>
      <field name="inner">
        <ui><textEdit/></ui>
      </field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	sec := findSectionByName(form.Sections, "noBind")
	if sec == nil {
		t.Fatal("subform with bind='none' must still be emitted as a section")
	}
	if sec.Bind == nil || sec.Bind.Match != "none" {
		t.Errorf("section Bind = %+v, want Match 'none'", sec.Bind)
	}
	q := findQuestionByName(form.Questions, "inner")
	if q == nil {
		t.Fatal("field inside bind='none' subform must still be emitted as a question")
	}
	if q.Type != types.ResponseTypeText {
		t.Errorf("inner question type = %q, want text", q.Type)
	}
	if q.Bind != nil {
		t.Errorf("subform-level bind leaked onto child question: %+v", *q.Bind)
	}
}

// TestExclGroupBindAndOccur verifies that <bind> and <occur> declared directly
// under an <exclGroup> surface on the emitted radio question.
func TestExclGroupBindAndOccur(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <exclGroup name="choice">
      <bind match="global"/>
      <occur max="-1"/>
      <field name="yes">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>Yes</text></value></caption>
        <value><text>1</text></value>
      </field>
      <field name="no">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>No</text></value></caption>
        <value><text>2</text></value>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	q := findQuestionByName(form.Questions, "choice")
	if q == nil {
		t.Fatal("exclGroup question 'choice' not found")
	}
	if q.Bind == nil || q.Bind.Match != "global" {
		t.Errorf("Bind = %+v, want Match 'global'", q.Bind)
	}
	want := types.Occur{Min: 1, Max: -1, Initial: 1}
	if q.Occur == nil || *q.Occur != want {
		t.Errorf("Occur = %+v, want %+v", q.Occur, want)
	}
	if len(q.Options) != 2 {
		t.Errorf("options = %d, want 2 (bind/occur parsing must not disturb option flattening)", len(q.Options))
	}
}

// TestOccurUnderPageSet verifies the pageSet guard: <pageSet> is not pushed
// onto the parser's node stack, so an <occur> directly under it must attach
// nowhere (NOT to the enclosing subform), while an <occur> under a <pageArea>
// lands on the pageArea's FormElement.
func TestOccurUnderPageSet(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <pageSet>
      <occur min="1" max="4"/>
      <pageArea name="Page1">
        <occur max="2"/>
        <contentArea x="0" y="0" w="612pt" h="792pt"/>
      </pageArea>
    </pageSet>
    <field name="f">
      <ui><textEdit/></ui>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	sec := findSectionByName(form.Sections, "form1")
	if sec == nil {
		t.Fatal("section 'form1' not found")
	}
	if sec.Occur != nil {
		t.Errorf("pageSet-level <occur> misattributed to enclosing subform: %+v", *sec.Occur)
	}

	var pageEl *types.FormElement
	for i := range form.Elements {
		if form.Elements[i].Role == "pageArea" && form.Elements[i].OwnerPath == "form1.Page1" {
			pageEl = &form.Elements[i]
			break
		}
	}
	if pageEl == nil {
		t.Fatal("pageArea element 'form1.Page1' not found")
	}
	want := types.Occur{Min: 1, Max: 2, Initial: 1}
	if pageEl.Occur == nil || *pageEl.Occur != want {
		t.Errorf("pageArea Occur = %+v, want %+v", pageEl.Occur, want)
	}
}

// TestFDA3881OccurBindMetadata checks the real FDA 3881 template: it declares
// no <occur> elements (so Occur must be nil everywhere — guarding against
// spurious defaults), and its only <bind> declarations are match="none" — on
// the Page1 subform (previously dropped, now surfaced as section metadata)
// and on page-counter fields.
func TestFDA3881OccurBindMetadata(t *testing.T) {
	data, err := os.ReadFile("testdata/fda_3881_template.xml")
	if err != nil {
		t.Skip("testdata/fda_3881_template.xml not found:", err)
	}

	form, err := ParseXFAForm(string(data), false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	for _, q := range form.Questions {
		if q.Occur != nil {
			t.Errorf("question %q has Occur %+v, want nil (template declares no <occur>)", q.Name, *q.Occur)
		}
		if q.Bind != nil && q.Bind.Match != "none" {
			t.Errorf("question %q has Bind %+v; the fixture only declares match='none'", q.Name, *q.Bind)
		}
	}
	var walk func([]types.FormSection)
	walk = func(secs []types.FormSection) {
		for _, s := range secs {
			if s.Occur != nil {
				t.Errorf("section %q has Occur %+v, want nil", s.Path, *s.Occur)
			}
			walk(s.Children)
		}
	}
	walk(form.Sections)
	for _, el := range form.Elements {
		if el.Occur != nil {
			t.Errorf("element %q has Occur %+v, want nil", el.ID, *el.Occur)
		}
	}

	page1 := findSectionByName(form.Sections, "Page1")
	if page1 == nil {
		t.Fatal("section 'Page1' not found")
	}
	if page1.Bind == nil || page1.Bind.Match != "none" {
		t.Errorf("Page1 section Bind = %+v, want Match 'none' (subform-level <bind> must now be surfaced)", page1.Bind)
	}
}
