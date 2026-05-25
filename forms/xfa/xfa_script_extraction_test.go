package xfa

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// TestFieldEventBodyPreservedVerbatim verifies that a field-attached event's
// script body is emitted on FormSchema.Scripts exactly as written — including
// patterns the old heuristic layer would have mangled.
func TestFieldEventBodyPreservedVerbatim(t *testing.T) {
	body := `if ($.rawValue == "yes") then
    IMDRF.presence = "visible"
    USA.presence = "hidden"
else
    IMDRF.presence = "hidden"
    USA.presence = "visible"
endif`
	xfaXML := `<template>
  <subform name="Page1">
    <field name="AppType">
      <ui><textEdit/></ui>
      <event activity="change">
        <script contentType="application/x-formcalc">` + body + `</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	if len(form.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(form.Scripts))
	}
	s := form.Scripts[0]
	if s.Body != body {
		t.Errorf("body not preserved verbatim:\n got: %q\nwant: %q", s.Body, body)
	}
	if s.Event != "change" {
		t.Errorf("event = %q, want change", s.Event)
	}
	if s.Language != "formcalc" {
		t.Errorf("language = %q, want formcalc", s.Language)
	}
}

// TestLanguageDefaultsToFormCalc verifies that a <script> without contentType
// is reported as formcalc — that's the XFA spec's default.
func TestLanguageDefaultsToFormCalc(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="f">
      <ui><textEdit/></ui>
      <event activity="initialize">
        <script>$.rawValue = "hi"</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(form.Scripts))
	}
	if got := form.Scripts[0].Language; got != "formcalc" {
		t.Errorf("language = %q, want formcalc (XFA default)", got)
	}
}

// TestLanguageJavaScriptContentType verifies that contentType maps to javascript.
func TestLanguageJavaScriptContentType(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="f">
      <ui><textEdit/></ui>
      <event activity="change">
        <script contentType="application/x-javascript">this.presence = "hidden";</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(form.Scripts))
	}
	if got := form.Scripts[0].Language; got != "javascript" {
		t.Errorf("language = %q, want javascript", got)
	}
}

// TestQuestionScriptsIndex verifies that Question.Scripts holds IDs that resolve
// to entries in FormSchema.Scripts.
func TestQuestionScriptsIndex(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="trigger">
      <ui><textEdit/></ui>
      <event activity="initialize">
        <script contentType="application/x-javascript">this.rawValue = "init";</script>
      </event>
      <event activity="exit">
        <script contentType="application/x-javascript">this.rawValue = "exit";</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	if len(form.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(form.Questions))
	}
	q := form.Questions[0]
	if len(q.Scripts) != 2 {
		t.Fatalf("question.Scripts = %v, want 2 IDs", q.Scripts)
	}

	scriptsByID := make(map[string]types.FormScript, len(form.Scripts))
	for _, s := range form.Scripts {
		scriptsByID[s.ID] = s
	}
	for _, id := range q.Scripts {
		s, ok := scriptsByID[id]
		if !ok {
			t.Fatalf("question.Scripts ID %q not found in FormSchema.Scripts", id)
		}
		if s.OwnerID != q.ID {
			t.Errorf("script %s OwnerID = %q, want %q", id, s.OwnerID, q.ID)
		}
		if !strings.HasPrefix(s.OwnerPath, "Page1.trigger") {
			t.Errorf("script %s OwnerPath = %q, want prefix Page1.trigger", id, s.OwnerPath)
		}
	}
}

// TestSectionScriptsIndex verifies that subform-attached events surface as
// FormSection.Scripts IDs that resolve to FormSchema.Scripts.
func TestSectionScriptsIndex(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="Page1">
      <event activity="initialize">
        <script contentType="application/x-javascript">log("page init");</script>
      </event>
      <field name="f"><ui><textEdit/></ui></field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	if len(form.Sections) == 0 || len(form.Sections[0].Children) == 0 {
		t.Fatalf("expected nested section tree, got %+v", form.Sections)
	}
	page1 := form.Sections[0].Children[0]
	if page1.Name != "Page1" {
		t.Fatalf("expected Page1 section, got %q", page1.Name)
	}
	if len(page1.Scripts) != 1 {
		t.Fatalf("section.Scripts = %v, want 1 ID", page1.Scripts)
	}

	scriptsByID := make(map[string]types.FormScript, len(form.Scripts))
	for _, s := range form.Scripts {
		scriptsByID[s.ID] = s
	}
	s, ok := scriptsByID[page1.Scripts[0]]
	if !ok {
		t.Fatalf("section.Scripts ID %q not found in FormSchema.Scripts", page1.Scripts[0])
	}
	if s.OwnerPath != "form1.Page1" {
		t.Errorf("script OwnerPath = %q, want form1.Page1", s.OwnerPath)
	}
	if s.OwnerID != "form1.Page1" {
		t.Errorf("script OwnerID = %q, want form1.Page1", s.OwnerID)
	}
	if s.Event != "initialize" {
		t.Errorf("script Event = %q, want initialize", s.Event)
	}
}

// TestScriptEventNameAndRunAt verifies that <event name> and <script runAt>
// flow into FormScript.Name and FormScript.RunAt.
func TestScriptEventNameAndRunAt(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="f">
      <ui><button/></ui>
      <event activity="click" name="click">
        <script contentType="application/x-javascript" runAt="client">app.alert("hi");</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(form.Scripts))
	}
	s := form.Scripts[0]
	if s.Name != "click" {
		t.Errorf("Name = %q, want click", s.Name)
	}
	if s.RunAt != "client" {
		t.Errorf("RunAt = %q, want client", s.RunAt)
	}
}

// TestEmptyEventSkipped verifies that an <event> declaration with no <script>
// body produces no FormScript — there's no payload to expose.
func TestEmptyEventSkipped(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="f">
      <ui><textEdit/></ui>
      <event activity="change"/>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 0 {
		t.Errorf("expected 0 scripts for empty event, got %d: %+v", len(form.Scripts), form.Scripts)
	}
	if len(form.Questions) != 1 || len(form.Questions[0].Scripts) != 0 {
		t.Errorf("question.Scripts should be empty, got %+v", form.Questions[0].Scripts)
	}
}

// TestScriptIDStability verifies that script IDs include the SOM path, event
// activity, and event declaration index — making them stable across regeneration
// when the underlying XFA doesn't change.
func TestScriptIDStability(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="trigger">
      <ui><textEdit/></ui>
      <event activity="initialize">
        <script>$.rawValue = "a"</script>
      </event>
      <event activity="change">
        <script>$.rawValue = "b"</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 2 {
		t.Fatalf("expected 2 scripts, got %d", len(form.Scripts))
	}
	wantIDs := []string{"Page1.trigger#initialize[0]", "Page1.trigger#change[1]"}
	for i, want := range wantIDs {
		if form.Scripts[i].ID != want {
			t.Errorf("script[%d].ID = %q, want %q", i, form.Scripts[i].ID, want)
		}
	}
}

// TestUnknownEventAndScriptAttrsCaptured verifies that <event> and <script>
// attributes outside the typed-field set (activity, name, contentType, runAt)
// land in FormScript.Properties so callers parsing the script body have
// access to ref/listen targeting and other XFA-spec attrs.
func TestUnknownEventAndScriptAttrsCaptured(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="watcher">
      <ui><textEdit/></ui>
      <event activity="change" name="change" listen="refOnly" ref="someField" id="evt1">
        <script contentType="application/x-javascript" runAt="client" binding="this" stateless="0" id="scr1">this.rawValue = "x";</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Scripts) != 1 {
		t.Fatalf("expected 1 script, got %d", len(form.Scripts))
	}
	s := form.Scripts[0]

	wantPresent := map[string]string{
		"listen":    "refOnly",
		"ref":       "someField",
		"binding":   "this",
		"stateless": "0",
	}
	for k, want := range wantPresent {
		got, ok := s.Properties[k]
		if !ok {
			t.Errorf("Properties[%q] missing", k)
			continue
		}
		if got != want {
			t.Errorf("Properties[%q] = %v, want %q", k, got, want)
		}
	}

	// Both <event> and <script> carry id; share a single flat map, so the
	// later-seen <script id> wins. The map must at least contain one of them.
	if _, ok := s.Properties["id"]; !ok {
		t.Error("Properties[\"id\"] missing")
	}

	wantAbsent := []string{"activity", "name", "contentType", "runAt"}
	for _, k := range wantAbsent {
		if _, ok := s.Properties[k]; ok {
			t.Errorf("Properties[%q] should not be duplicated from typed field", k)
		}
	}
}

// TestVariablesScriptAttrsCaptured verifies that unknown attributes on a
// <variables><script> element land in FormScript.Properties.
func TestVariablesScriptAttrsCaptured(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <variables>
      <script name="helpers" contentType="application/x-javascript" id="vars1" url="http://example/lib.js" binding="this">function hi() { return 1; }</script>
    </variables>
    <field name="f"><ui><textEdit/></ui></field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var vs *struct{ id, url, binding string }
	for _, s := range form.Scripts {
		if s.Event == "variables" {
			id, _ := s.Properties["id"].(string)
			url, _ := s.Properties["url"].(string)
			binding, _ := s.Properties["binding"].(string)
			vs = &struct{ id, url, binding string }{id, url, binding}
			if _, ok := s.Properties["contentType"]; ok {
				t.Error("Properties[\"contentType\"] should not be duplicated; it's surfaced as Language")
			}
			if _, ok := s.Properties["name"]; ok {
				t.Error("Properties[\"name\"] should not be duplicated; it's surfaced as Name")
			}
			break
		}
	}
	if vs == nil {
		t.Fatal("no variables script found")
	}
	if vs.id != "vars1" {
		t.Errorf("Properties[\"id\"] = %q, want vars1", vs.id)
	}
	if vs.url != "http://example/lib.js" {
		t.Errorf("Properties[\"url\"] = %q, want http://example/lib.js", vs.url)
	}
	if vs.binding != "this" {
		t.Errorf("Properties[\"binding\"] = %q, want this", vs.binding)
	}
}
