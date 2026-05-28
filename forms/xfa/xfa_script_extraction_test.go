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

// findScript returns the first FormScript matching the given OwnerPath and
// event activity, or nil if none is found.
func findScript(scripts []types.FormScript, ownerPath, event string) *types.FormScript {
	for i := range scripts {
		if scripts[i].OwnerPath == ownerPath && scripts[i].Event == event {
			return &scripts[i]
		}
	}
	return nil
}

// TestPageAreaEventExtracted verifies that <pageArea> events are surfaced as
// FormScripts with the pageArea's SOM path as OwnerPath. pageAreas surface as
// FormElements (role "pageArea"), so OwnerID resolves to the element ID.
func TestPageAreaEventExtracted(t *testing.T) {
	body := `xfa.host.messageBox("page rendered");`
	xfaXML := `<template>
  <subform name="form1">
    <pageArea name="Master">
      <event activity="ready">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </pageArea>
    <subform name="Page1">
      <field name="anchor"><ui><textEdit/></ui></field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	s := findScript(form.Scripts, "form1.Master", "ready")
	if s == nil {
		t.Fatalf("no script with OwnerPath=form1.Master event=ready; got %+v", form.Scripts)
	}
	if s.Body != body {
		t.Errorf("body = %q, want %q", s.Body, body)
	}
	if s.OwnerID != "form1.Master" {
		t.Errorf("OwnerID = %q, want form1.Master (pageArea is a typed Element)", s.OwnerID)
	}
}

// TestBindNoneButtonScriptExtracted verifies that a bind="none" button (e.g.
// "Help Text" / "Show Intro" UI triggers) contributes its click-handler script
// to FormSchema.Scripts. The button itself surfaces as a FormElement, so the
// script's OwnerID resolves to the element ID rather than staying empty.
func TestBindNoneButtonScriptExtracted(t *testing.T) {
	body := `xfa.host.messageBox("?-hint");`
	xfaXML := `<template>
  <subform name="Page1">
    <field name="helpBtn">
      <ui><button/></ui>
      <caption><value><text>Help Text</text></value></caption>
      <bind match="none"/>
      <event activity="click">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "helpBtn" {
			t.Fatalf("bind=none button should not be emitted as Question; got %+v", q)
		}
	}
	s := findScript(form.Scripts, "Page1.helpBtn", "click")
	if s == nil {
		t.Fatalf("no script for Page1.helpBtn click; got %+v", form.Scripts)
	}
	if s.Body != body {
		t.Errorf("body = %q, want %q", s.Body, body)
	}
	if s.OwnerID != "Page1.helpBtn" {
		t.Errorf("OwnerID = %q, want Page1.helpBtn (button is a typed Element)", s.OwnerID)
	}
}

// TestDrawEventScriptExtracted verifies that event-bearing <draw> elements
// (status indicators with dynamic show/hide handlers) still surface their
// scripts even though the draw itself is suppressed from Questions. The draw
// surfaces as a FormElement, so the script's OwnerID resolves to the element.
func TestDrawEventScriptExtracted(t *testing.T) {
	body := `this.presence = "hidden";`
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="statusOk">
      <value><text>Required Question Complete</text></value>
      <event activity="initialize">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "statusOk" {
			t.Fatalf("event-bearing draw should not be emitted as Question; got %+v", q)
		}
	}
	s := findScript(form.Scripts, "Page1.statusOk", "initialize")
	if s == nil {
		t.Fatalf("no script for Page1.statusOk initialize; got %+v", form.Scripts)
	}
	if s.Body != body {
		t.Errorf("body = %q, want %q", s.Body, body)
	}
	if s.OwnerID != "Page1.statusOk" {
		t.Errorf("OwnerID = %q, want Page1.statusOk (draw is a typed Element)", s.OwnerID)
	}
}

// TestExclGroupOptionScriptsExtracted verifies that per-option <event>
// blocks (defined on the individual radio-option <field>s inside an
// <exclGroup>) are preserved as distinct FormScripts. Each option's script
// must have its own OwnerPath (exclGroup SOM path + "." + option field name)
// and remain an orphan, while the exclGroup itself (which IS a Question) gets
// its own script back-ref via the standard Question.Scripts mechanism.
//
// The field <items> values ("a", "b") deliberately differ from the field
// names ("optA", "optB") to assert that the SOM OwnerPath is keyed by the
// field's name (real SOM) rather than the option's data value (which can
// contain arbitrary text).
func TestExclGroupOptionScriptsExtracted(t *testing.T) {
	bodyA := `xfa.host.messageBox("A selected");`
	bodyB := `xfa.host.messageBox("B selected");`
	groupBody := `xfa.host.messageBox("group changed");`
	xfaXML := `<template>
  <subform name="Page1">
    <exclGroup name="choice">
      <event activity="change">
        <script contentType="application/x-javascript">` + groupBody + `</script>
      </event>
      <field name="optA">
        <ui><radioButton/></ui>
        <caption><value><text>A</text></value></caption>
        <items><text>a</text></items>
        <event activity="click">
          <script contentType="application/x-javascript">` + bodyA + `</script>
        </event>
      </field>
      <field name="optB">
        <ui><radioButton/></ui>
        <caption><value><text>B</text></value></caption>
        <items><text>b</text></items>
        <event activity="click">
          <script contentType="application/x-javascript">` + bodyB + `</script>
        </event>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var choiceQ *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "choice" {
			choiceQ = &form.Questions[i]
		}
	}
	if choiceQ == nil {
		t.Fatalf("exclGroup 'choice' question missing")
	}

	groupScript := findScript(form.Scripts, "Page1.choice", "change")
	if groupScript == nil {
		t.Fatalf("no script with OwnerPath=Page1.choice event=change; got %+v", form.Scripts)
	}
	if groupScript.Body != groupBody {
		t.Errorf("group body = %q, want %q", groupScript.Body, groupBody)
	}
	if groupScript.OwnerID != choiceQ.ID {
		t.Errorf("group OwnerID = %q, want question ID %q", groupScript.OwnerID, choiceQ.ID)
	}

	scriptA := findScript(form.Scripts, "Page1.choice.optA", "click")
	if scriptA == nil {
		t.Fatalf("no per-option script with OwnerPath=Page1.choice.optA; got %+v", form.Scripts)
	}
	if scriptA.Body != bodyA {
		t.Errorf("option A body = %q, want %q", scriptA.Body, bodyA)
	}
	if scriptA.OwnerID != "" {
		t.Errorf("option A OwnerID = %q, want empty (orphan)", scriptA.OwnerID)
	}

	scriptB := findScript(form.Scripts, "Page1.choice.optB", "click")
	if scriptB == nil {
		t.Fatalf("no per-option script with OwnerPath=Page1.choice.optB; got %+v", form.Scripts)
	}
	if scriptB.Body != bodyB {
		t.Errorf("option B body = %q, want %q", scriptB.Body, bodyB)
	}
	if scriptB.OwnerID != "" {
		t.Errorf("option B OwnerID = %q, want empty (orphan)", scriptB.OwnerID)
	}

	// Negative assertion: the OLD path (keyed by option <items> value) must
	// NOT appear, to lock in the SOM-correct field-name keying.
	if s := findScript(form.Scripts, "Page1.choice.a", "click"); s != nil {
		t.Errorf("per-option script should NOT use option value as path key; got %+v", s)
	}
}

// TestNestedSubformFieldBackRef verifies that a field inside a deeply nested
// subform gets its Question.Scripts back-reference populated, and the script's
// OwnerID resolves to the Question.ID. Specifically locks in that the SOM
// path computed by extractAllScripts (parent walk of named nodes) matches the
// path computed by populateScriptBackRefs (sec.Path + "." + q.Name) for
// arbitrary nesting depth.
func TestNestedSubformFieldBackRef(t *testing.T) {
	body := `xfa.host.messageBox("deep");`
	xfaXML := `<template>
  <subform name="form1">
    <subform name="outer">
      <subform name="inner">
        <field name="deepField">
          <ui><textEdit/></ui>
          <event activity="change">
            <script contentType="application/x-javascript">` + body + `</script>
          </event>
        </field>
      </subform>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var deepQ *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "deepField" {
			deepQ = &form.Questions[i]
		}
	}
	if deepQ == nil {
		t.Fatalf("deepField question missing; got %+v", form.Questions)
	}

	wantPath := "form1.outer.inner.deepField"
	s := findScript(form.Scripts, wantPath, "change")
	if s == nil {
		t.Fatalf("no script with OwnerPath=%s; got %+v", wantPath, form.Scripts)
	}
	if s.OwnerID != deepQ.ID {
		t.Errorf("OwnerID = %q, want question ID %q (back-ref must resolve through nested sections)", s.OwnerID, deepQ.ID)
	}
	if len(deepQ.Scripts) != 1 || deepQ.Scripts[0] != s.ID {
		t.Errorf("Question.Scripts = %v, want [%q]", deepQ.Scripts, s.ID)
	}
}

// findElement returns the first FormElement matching the given OwnerPath, or
// nil if none is found.
func findElement(elements []types.FormElement, ownerPath string) *types.FormElement {
	for i := range elements {
		if elements[i].OwnerPath == ownerPath {
			return &elements[i]
		}
	}
	return nil
}

// TestBindNoneButtonElement verifies that a bind="none" non-AddAttachment button
// (a Help Text / Show Intro UI trigger) surfaces as a FormElement with role
// "button" and that its click-handler script's OwnerID resolves to the element.
func TestBindNoneButtonElement(t *testing.T) {
	body := `xfa.host.messageBox("?-hint");`
	xfaXML := `<template>
  <subform name="Page1">
    <field name="helpBtn">
      <ui><button/></ui>
      <caption><value><text>Help Text</text></value></caption>
      <bind match="none"/>
      <event activity="click">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	el := findElement(form.Elements, "Page1.helpBtn")
	if el == nil {
		t.Fatalf("no element with OwnerPath=Page1.helpBtn; got %+v", form.Elements)
	}
	if el.Role != "button" {
		t.Errorf("Role = %q, want button", el.Role)
	}
	if el.Label != "Help Text" {
		t.Errorf("Label = %q, want %q", el.Label, "Help Text")
	}
	if _, ok := el.Properties["xfa_role"]; ok {
		t.Errorf("non-AddAttachment button should not carry xfa_role=attachment; got %+v", el.Properties)
	}

	s := findScript(form.Scripts, "Page1.helpBtn", "click")
	if s == nil {
		t.Fatalf("no script for Page1.helpBtn click; got %+v", form.Scripts)
	}
	if s.OwnerID != el.ID {
		t.Errorf("script OwnerID = %q, want element ID %q", s.OwnerID, el.ID)
	}
	if len(el.Scripts) != 1 || el.Scripts[0] != s.ID {
		t.Errorf("element.Scripts = %v, want [%q]", el.Scripts, s.ID)
	}
}

// TestEventBearingDrawElement verifies that an event-bearing <draw> (status
// indicator with a dynamic show/hide handler) surfaces as a FormElement with
// role "draw" and its initialize script resolves to the element.
func TestEventBearingDrawElement(t *testing.T) {
	body := `this.presence = "hidden";`
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="statusOk">
      <value><text>Required Question Complete</text></value>
      <event activity="initialize">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	el := findElement(form.Elements, "Page1.statusOk")
	if el == nil {
		t.Fatalf("no element with OwnerPath=Page1.statusOk; got %+v", form.Elements)
	}
	if el.Role != "draw" {
		t.Errorf("Role = %q, want draw", el.Role)
	}

	s := findScript(form.Scripts, "Page1.statusOk", "initialize")
	if s == nil {
		t.Fatalf("no script for Page1.statusOk initialize; got %+v", form.Scripts)
	}
	if s.OwnerID != el.ID {
		t.Errorf("script OwnerID = %q, want element ID %q", s.OwnerID, el.ID)
	}
	if len(el.Scripts) != 1 || el.Scripts[0] != s.ID {
		t.Errorf("element.Scripts = %v, want [%q]", el.Scripts, s.ID)
	}
}

// TestPageAreaElement verifies that <pageArea> nodes surface as FormElements
// with role "pageArea" and that pageArea events resolve to the element ID.
func TestPageAreaElement(t *testing.T) {
	body := `xfa.host.messageBox("page rendered");`
	xfaXML := `<template>
  <subform name="form1">
    <pageArea name="Master">
      <event activity="ready">
        <script contentType="application/x-javascript">` + body + `</script>
      </event>
    </pageArea>
    <subform name="Page1">
      <field name="anchor"><ui><textEdit/></ui></field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	el := findElement(form.Elements, "form1.Master")
	if el == nil {
		t.Fatalf("no element with OwnerPath=form1.Master; got %+v", form.Elements)
	}
	if el.Role != "pageArea" {
		t.Errorf("Role = %q, want pageArea", el.Role)
	}

	s := findScript(form.Scripts, "form1.Master", "ready")
	if s == nil {
		t.Fatalf("no script for form1.Master ready; got %+v", form.Scripts)
	}
	if s.OwnerID != el.ID {
		t.Errorf("script OwnerID = %q, want element ID %q", s.OwnerID, el.ID)
	}
	if len(el.Scripts) != 1 || el.Scripts[0] != s.ID {
		t.Errorf("element.Scripts = %v, want [%q]", el.Scripts, s.ID)
	}
}

// TestAddAttachmentStaysQuestion verifies that AddAttachment buttons remain in
// Questions as ResponseTypeFile and do NOT appear in Elements. File uploads are
// semantically user input (just on a non-dataset binding path), and consumers
// iterating Questions to render input controls expect to find them there.
func TestAddAttachmentStaysQuestion(t *testing.T) {
	xfaXML := `<template>
  <subform name="CoverLetter">
    <field name="CLAddAttachment110">
      <ui><button/></ui>
      <caption><value><text>Add Attachment</text></value></caption>
      <bind match="none"/>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var fileQ *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "CLAddAttachment110" {
			fileQ = &form.Questions[i]
		}
	}
	if fileQ == nil {
		t.Fatalf("AddAttachment button missing from Questions; got %+v", form.Questions)
	}
	if fileQ.Type != types.ResponseTypeFile {
		t.Errorf("Type = %q, want %q", fileQ.Type, types.ResponseTypeFile)
	}

	if el := findElement(form.Elements, "CoverLetter.CLAddAttachment110"); el != nil {
		t.Errorf("AddAttachment should not appear in Elements; got %+v", el)
	}
}
