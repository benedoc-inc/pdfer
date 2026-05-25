package xfa

import (
	"os"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestParseXFAForm(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<template>
  <field name="testField" type="text" required="1">
    <label>Test Label</label>
    <value>Default Value</value>
  </field>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	if form == nil {
		t.Fatal("ParseXFAForm() returned nil")
	}

	if len(form.Questions) != 1 {
		t.Fatalf("ParseXFAForm() questions count = %d, want 1", len(form.Questions))
	}

	question := form.Questions[0]
	if question.Name != "testField" {
		t.Errorf("ParseXFAForm() question name = %q, want %q", question.Name, "testField")
	}
	if question.Label != "Test Label" {
		t.Errorf("ParseXFAForm() question label = %q, want %q", question.Label, "Test Label")
	}
	if question.Type != types.ResponseTypeText {
		t.Errorf("ParseXFAForm() question type = %q, want %q", question.Type, types.ResponseTypeText)
	}
	if !question.Required {
		t.Error("ParseXFAForm() question required = false, want true")
	}
}

// TestToolTipLabel verifies that <toolTip> text is used as the field label
// when no <caption> is present — the pattern used by FDA forms.
func TestToolTipLabel(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="number510" x="0mm" y="24.9mm" w="190mm" h="6.7mm">
      <ui><textEdit/></ui>
      <toolTip>5 10 (k) number</toolTip>
      <assist><speak>Enter 5 10 (k) number.</speak></assist>
      <bind match="none"/>
    </field>
    <field name="devicename" x="0mm" y="36.6mm" w="190mm" h="13.5mm">
      <ui><textEdit multiLine="1"/></ui>
      <toolTip>device name</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	if q, ok := byName["devicename"]; !ok {
		t.Error("devicename field missing")
	} else if q.Label != "device name" {
		t.Errorf("devicename label = %q, want %q", q.Label, "device name")
	}
}

// TestAssistSpeakFallback verifies that <assist><speak> is used as the label
// fallback when neither <caption> nor <toolTip> is present.
func TestAssistSpeakFallback(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="myField">
      <ui><textEdit/></ui>
      <assist><speak>My field label</speak></assist>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions parsed")
	}
	q := form.Questions[0]
	if q.Label != "My field label" {
		t.Errorf("label = %q, want %q", q.Label, "My field label")
	}
}

// TestExDataSuppression verifies that draw elements containing <exData>
// (page-counter embeds and similar template machinery) are not emitted.
func TestExDataSuppression(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <draw name="Pages">
      <value>
        <exData contentType="text/html">
          <body><p>Page <span xfa:embed="#p"/> of <span xfa:embed="#n"/></p></body>
        </exData>
      </value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "Pages" {
			t.Errorf("exData draw 'Pages' should have been suppressed, got type=%s label=%q", q.Type, q.Label)
		}
		if q.Type == types.ResponseTypeDisplay && strings.Contains(q.Label, "xfa:embed") {
			t.Errorf("exData HTML leaked into display question: %q", q.Label)
		}
	}
}

// TestButtonWithBindNoneSkipped verifies that button fields with bind=none
// (UI triggers like "Help Text", "Show Introduction") are not emitted as display items.
func TestButtonWithBindNoneSkipped(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <field name="helpBtn" access="readOnly">
      <ui><button/></ui>
      <caption><value><text>Help Text</text></value></caption>
      <bind match="none"/>
    </field>
    <field name="showIntro" access="readOnly">
      <ui><button/></ui>
      <caption><value><text>Show Application Introduction</text></value></caption>
      <bind match="none"/>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "helpBtn" || q.Name == "showIntro" {
			t.Errorf("button field %q should not appear in output, got type=%s label=%q", q.Name, q.Type, q.Label)
		}
	}
}

// TestDrawWithEventsSkipped verifies that draw elements with events (status indicators,
// scripted show/hide toggles) are not emitted as display items.
func TestDrawWithEventsSkipped(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <draw name="statusOk">
      <value><text>Required Question Complete</text></value>
      <event activity="initialize">
        <script>this.presence = "hidden";</script>
      </event>
    </draw>
    <draw name="statusBad">
      <value><text>Required Question Incomplete</text></value>
      <event activity="change">
        <script>this.presence = "visible";</script>
      </event>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "statusOk" || q.Name == "statusBad" {
			t.Errorf("draw with events %q should not appear in output", q.Name)
		}
	}
}

// TestExclGroupLabelFromPrecedingDraw verifies that an unlabeled exclGroup gets its
// label from the nearest preceding draw in the same section.
func TestExclGroupLabelFromPrecedingDraw(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="groupLabel">
      <value><text>Application Type</text></value>
    </draw>
    <exclGroup name="appType">
      <field name="traditional">
        <caption><value><text>Traditional</text></value></caption>
        <items><text>Traditional</text></items>
      </field>
      <field name="abbreviated">
        <caption><value><text>Abbreviated</text></value></caption>
        <items><text>Abbreviated</text></items>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	var radioQ *types.Question
	for i := range form.Questions {
		if form.Questions[i].Type == types.ResponseTypeRadio {
			radioQ = &form.Questions[i]
		}
	}
	if radioQ == nil {
		t.Fatal("no radio question found")
	}
	if radioQ.Label != "Application Type" {
		t.Errorf("radio group label = %q, want %q", radioQ.Label, "Application Type")
	}
	// The draw should be consumed — not a standalone display item
	for _, q := range form.Questions {
		if q.Name == "groupLabel" {
			t.Errorf("draw 'groupLabel' should have been consumed as radio group label")
		}
	}
}

// TestPresenceManagedDrawsSkipped verifies that draw elements with an explicit
// presence= attribute are filtered out. These are script-managed status indicators
// (like "Required Question Complete/Incomplete") whose visibility is toggled by form
// logic — not static labels. Static labels never need a presence attribute.
func TestPresenceManagedDrawsSkipped(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <draw name="sectionHeader">
      <value><text>Carcinogenicity</text></value>
    </draw>
    <draw name="statusComplete" presence="hidden">
      <value><text>Required Question Complete</text></value>
    </draw>
    <draw name="statusIncomplete" presence="visible">
      <value><text>Required Question Incomplete</text></value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	found := make(map[string]bool)
	for _, q := range form.Questions {
		found[q.Name] = true
	}

	if found["statusComplete"] {
		t.Error("statusComplete (presence=hidden) should be filtered")
	}
	if found["statusIncomplete"] {
		t.Error("statusIncomplete (presence=visible) should be filtered — explicit presence marks it as dynamic")
	}
	if !found["sectionHeader"] {
		t.Error("sectionHeader (no presence attr) should be kept")
	}
	if !found["realField"] {
		t.Error("realField should be kept")
	}
}

// TestSectionsTree verifies that the Sections field on FormSchema contains a
// proper hierarchy with Interactive correctly set per section.
func TestSectionsTree(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="interactiveSection">
      <field name="userInput">
        <ui><textEdit/></ui>
        <toolTip>Enter value</toolTip>
      </field>
      <draw name="label"><value><text>Some label</text></value></draw>
    </subform>
    <subform name="displayOnlySection">
      <draw name="introText"><value><text>Introduction paragraph.</text></value></draw>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	if len(form.Sections) == 0 {
		t.Fatal("expected non-empty Sections, got none")
	}

	// Navigate to form1
	var form1 *types.FormSection
	for i := range form.Sections {
		if form.Sections[i].Name == "form1" {
			form1 = &form.Sections[i]
			break
		}
	}
	if form1 == nil {
		t.Fatalf("section 'form1' not found in %v", sectionNames(form.Sections))
	}

	byName := make(map[string]*types.FormSection)
	for i := range form1.Children {
		byName[form1.Children[i].Name] = &form1.Children[i]
	}

	interactive, ok := byName["interactiveSection"]
	if !ok {
		t.Fatalf("'interactiveSection' not found among form1's children: %v", sectionChildNames(form1))
	}
	if !interactive.Interactive {
		t.Error("interactiveSection.Interactive should be true")
	}

	displayOnly, ok := byName["displayOnlySection"]
	if !ok {
		t.Fatalf("'displayOnlySection' not found among form1's children: %v", sectionChildNames(form1))
	}
	if displayOnly.Interactive {
		t.Error("displayOnlySection.Interactive should be false")
	}

	// Draws in the display-only section must not appear in flat questions[].
	for _, q := range form.Questions {
		if q.Name == "introText" {
			t.Error("draw in display-only section must not appear in questions[]")
		}
	}

	// The label draw in the interactive section should appear in questions[].
	found := false
	for _, q := range form.Questions {
		if q.Name == "label" {
			found = true
			break
		}
	}
	if !found {
		t.Error("draw 'label' in interactive section should appear in questions[]")
	}
}

func sectionNames(ss []types.FormSection) []string {
	var names []string
	for _, s := range ss {
		names = append(names, s.Name)
	}
	return names
}

func sectionChildNames(s *types.FormSection) []string {
	return sectionNames(s.Children)
}

// TestFDA3881Template parses the real FDA 3881 template fixture and checks
// that key fields have correct labels from <toolTip>/<assist>.
func TestFDA3881Template(t *testing.T) {
	data, err := os.ReadFile("testdata/fda_3881_template.xml")
	if err != nil {
		t.Skip("testdata/fda_3881_template.xml not found:", err)
	}

	form, err := ParseXFAForm(string(data), false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	cases := []struct{ name, wantLabel string }{
		{"number510", "5 10 (k) number"},
		{"devicename", "device name"},
		{"indications", "indications for use"},
	}
	for _, c := range cases {
		q, ok := byName[c.name]
		if !ok {
			t.Errorf("field %q not found in parsed form", c.name)
			continue
		}
		if q.Label != c.wantLabel {
			t.Errorf("field %q label = %q, want %q", c.name, q.Label, c.wantLabel)
		}
	}

	// Verify no page-counter exData draw leaked through
	for _, q := range form.Questions {
		if q.Name == "Pages" {
			t.Errorf("exData 'Pages' draw should be suppressed")
		}
	}
}

// TestImageEditDrawsSuppressed verifies that draws with <ui><imageEdit/></ui>
// and no embedded image data (colored status blocks) are not emitted as
// questions and are not claimed as labels by adjacent exclGroups.
func TestImageEditDrawsSuppressed(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="YesIndicator">
      <ui><imageEdit/></ui>
      <assist><toolTip>Required Question Incomplete</toolTip></assist>
    </draw>
    <exclGroup name="LanguageGroup">
      <field name="English">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>English</text></value></caption>
        <value><text>1</text></value>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	found := make(map[string]types.Question)
	for _, q := range form.Questions {
		found[q.Name] = q
	}

	if _, ok := found["YesIndicator"]; ok {
		t.Error("imageEdit draw 'YesIndicator' should be suppressed, not emitted as a question")
	}
	if q, ok := found["LanguageGroup"]; ok {
		if q.Label == "Required Question Incomplete" {
			t.Error("exclGroup 'LanguageGroup' should not inherit label from imageEdit draw")
		}
	} else {
		t.Error("exclGroup 'LanguageGroup' should be present in questions")
	}
}

// TestLabelScanStopsAtField verifies that the backward label scan stops when
// it encounters an interactive field, preventing cross-cluster label stealing.
func TestLabelScanStopsAtField(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="labelA"><value><text>Group A Label</text></value></draw>
    <exclGroup name="GroupA">
      <field name="optA1">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>Option 1</text></value></caption>
        <value><text>1</text></value>
      </field>
    </exclGroup>
    <field name="helpButton">
      <ui><button/></ui>
      <bind match="none"/>
    </field>
    <exclGroup name="GroupB">
      <field name="optB1">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>Option A</text></value></caption>
        <value><text>1</text></value>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	// GroupA should claim labelA (it's directly before GroupA with no boundary).
	if q, ok := byName["GroupA"]; ok {
		if q.Label != "Group A Label" {
			t.Errorf("GroupA label = %q, want %q", q.Label, "Group A Label")
		}
	} else {
		t.Error("GroupA should be present")
	}

	// GroupB scan hits helpButton (field boundary) before reaching labelA.
	// It must NOT claim "Group A Label" (wrong cluster). With the Name fallback it
	// will have label "GroupB" — the important invariant is ≠ "Group A Label".
	if q, ok := byName["GroupB"]; ok {
		if q.Label == "Group A Label" {
			t.Errorf("GroupB incorrectly claimed 'Group A Label' across field boundary")
		}
	} else {
		t.Error("GroupB should be present")
	}
}

// TestPresenceDrawClaimedAsLabel verifies that a presence-attributed textEdit
// draw is claimed as a label by an adjacent unlabeled exclGroup, but is NOT
// emitted as a standalone question.
func TestPresenceDrawClaimedAsLabel(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="someField">
      <ui><textEdit/></ui>
      <toolTip>Other field</toolTip>
    </field>
    <draw name="languageLabel" presence="hidden">
      <ui><textEdit/></ui>
      <value><text>Choose your language</text></value>
    </draw>
    <exclGroup name="LanguageGroup">
      <field name="English">
        <ui><checkButton shape="round"/></ui>
        <caption><value><text>English</text></value></caption>
        <value><text>1</text></value>
      </field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	if _, ok := byName["languageLabel"]; ok {
		t.Error("presence-hidden draw 'languageLabel' should not appear as a standalone question")
	}
	if q, ok := byName["LanguageGroup"]; ok {
		if q.Label != "Choose your language" {
			t.Errorf("LanguageGroup label = %q, want %q", q.Label, "Choose your language")
		}
	} else {
		t.Error("LanguageGroup should be present in questions")
	}
}

// TestPlaceholderCaptionOverridden verifies that an Adobe LiveCycle
// "The caption is in the textbox to the left" placeholder caption is replaced
// by the text from the immediately preceding sibling draw.
func TestPlaceholderCaptionOverridden(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <draw name="questionLabel">
      <ui><textEdit/></ui>
      <value><text>What does this rely on?</text></value>
    </draw>
    <field name="dropdown">
      <ui><choiceList/></ui>
      <caption><value><text>  The caption is in the textbox to the left of this dropdown</text></value></caption>
      <assist><toolTip>Abbreviated Type</toolTip></assist>
      <items><text>Option A</text><text>Option B</text></items>
      <items save="1"><text>a</text><text>b</text></items>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	if _, ok := byName["questionLabel"]; ok {
		t.Error("draw 'questionLabel' should be consumed and not emitted as a standalone question")
	}
	q, ok := byName["dropdown"]
	if !ok {
		t.Fatal("field 'dropdown' should be present in questions")
	}
	if q.Label == "  The caption is in the textbox to the left of this dropdown" ||
		q.Label == "The caption is in the textbox to the left of this dropdown" {
		t.Errorf("dropdown label is still the placeholder; want the sibling draw text")
	}
	if q.Label != "What does this rely on?" {
		t.Errorf("dropdown label = %q, want %q", q.Label, "What does this rely on?")
	}
}

func TestFormToXFA(t *testing.T) {
	form := &types.FormSchema{
		Metadata: types.FormMetadata{
			FormType:   "XFA",
			TotalPages: 1,
			Title:      "Test Form",
		},
		Questions: []types.Question{
			{
				ID:       "field1",
				Name:     "testField",
				Label:    "Test Label",
				Type:     types.ResponseTypeText,
				Required: true,
				Default:  "Default Value",
			},
		},
	}

	xfaXML, err := FormToXFA(form, false)
	if err != nil {
		t.Fatalf("FormToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "testField") {
		t.Error("FormToXFA() output does not contain field name")
	}
	if !strings.Contains(xfaXML, "Test Label") {
		t.Error("FormToXFA() output does not contain label")
	}
}

func TestFormRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<template>
  <field name="testField" type="text" required="1">
    <label>Test Label</label>
    <value>Default Value</value>
  </field>
</template>`

	// Parse to FormSchema
	form, err := ParseXFAForm(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := FormToXFA(form, false)
	if err != nil {
		t.Fatalf("FormToXFA() error = %v", err)
	}

	// Parse again to verify
	form2, err := ParseXFAForm(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFAForm() error = %v", err)
	}

	if len(form2.Questions) != len(form.Questions) {
		t.Errorf("Round-trip questions count = %d, want %d", len(form2.Questions), len(form.Questions))
	}
}

func TestParseXFADatasets(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<data>
  <field name="field1">
    <value>value1</value>
  </field>
  <field name="field2">
    <value>123</value>
  </field>
</data>`

	datasets, err := ParseXFADatasets(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFADatasets() error = %v", err)
	}

	if datasets == nil {
		t.Fatal("ParseXFADatasets() returned nil")
	}

	if len(datasets.Fields) != 2 {
		t.Fatalf("ParseXFADatasets() fields count = %d, want 2", len(datasets.Fields))
	}

	if datasets.Fields["field1"] != "value1" {
		t.Errorf("ParseXFADatasets() field1 = %v, want %q", datasets.Fields["field1"], "value1")
	}
}

func TestDatasetsToXFA(t *testing.T) {
	datasets := &types.XFADatasets{
		Fields: map[string]interface{}{
			"field1": "value1",
			"field2": 123,
			"field3": true,
		},
	}

	xfaXML, err := DatasetsToXFA(datasets, false)
	if err != nil {
		t.Fatalf("DatasetsToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "field1") {
		t.Error("DatasetsToXFA() output does not contain field1")
	}
	if !strings.Contains(xfaXML, "value1") {
		t.Error("DatasetsToXFA() output does not contain value1")
	}
}

func TestDatasetsRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<data>
  <field name="field1">
    <value>value1</value>
  </field>
  <field name="field2">
    <value>123</value>
  </field>
</data>`

	// Parse to XFADatasets
	datasets, err := ParseXFADatasets(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFADatasets() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := DatasetsToXFA(datasets, false)
	if err != nil {
		t.Fatalf("DatasetsToXFA() error = %v", err)
	}

	// Parse again to verify
	datasets2, err := ParseXFADatasets(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFADatasets() error = %v", err)
	}

	if len(datasets2.Fields) != len(datasets.Fields) {
		t.Errorf("Round-trip fields count = %d, want %d", len(datasets2.Fields), len(datasets.Fields))
	}
}

func TestParseXFAConfig(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<config>
  <present renderPolicy="client">
    <pdf version="1.7" />
  </present>
  <submit format="pdf" target="http://example.com" method="post" />
  <acrobat autoSave="1" autoSaveTime="30" />
</config>`

	config, err := ParseXFAConfig(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAConfig() error = %v", err)
	}

	if config == nil {
		t.Fatal("ParseXFAConfig() returned nil")
	}

	if config.Present == nil {
		t.Fatal("ParseXFAConfig() Present is nil")
	}

	if config.Present.RenderPolicy != "client" {
		t.Errorf("ParseXFAConfig() RenderPolicy = %q, want %q", config.Present.RenderPolicy, "client")
	}

	if config.Submit == nil {
		t.Fatal("ParseXFAConfig() Submit is nil")
	}

	if config.Submit.Format != "pdf" {
		t.Errorf("ParseXFAConfig() Submit Format = %q, want %q", config.Submit.Format, "pdf")
	}

	if config.Acrobat == nil {
		t.Fatal("ParseXFAConfig() Acrobat is nil")
	}

	if !config.Acrobat.AutoSave {
		t.Error("ParseXFAConfig() Acrobat AutoSave = false, want true")
	}

	if config.Acrobat.AutoSaveTime != 30 {
		t.Errorf("ParseXFAConfig() Acrobat AutoSaveTime = %d, want 30", config.Acrobat.AutoSaveTime)
	}
}

func TestConfigToXFA(t *testing.T) {
	config := &types.XFAConfig{
		Present: &types.XFAConfigPresent{
			RenderPolicy: "client",
			PDF: &types.XFAConfigPDF{
				Version: "1.7",
			},
		},
		Submit: &types.XFAConfigSubmit{
			Format: "pdf",
			Target: "http://example.com",
			Method: "post",
		},
		Acrobat: &types.XFAConfigAcrobat{
			AutoSave:     true,
			AutoSaveTime: 30,
		},
	}

	xfaXML, err := ConfigToXFA(config, false)
	if err != nil {
		t.Fatalf("ConfigToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "config") {
		t.Error("ConfigToXFA() output does not contain config")
	}
	if !strings.Contains(xfaXML, "present") {
		t.Error("ConfigToXFA() output does not contain present")
	}
	if !strings.Contains(xfaXML, "submit") {
		t.Error("ConfigToXFA() output does not contain submit")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<config>
  <present renderPolicy="client">
    <pdf version="1.7" />
  </present>
  <submit format="pdf" target="http://example.com" method="post" />
</config>`

	// Parse to XFAConfig
	config, err := ParseXFAConfig(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFAConfig() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := ConfigToXFA(config, false)
	if err != nil {
		t.Fatalf("ConfigToXFA() error = %v", err)
	}

	// Parse again to verify
	config2, err := ParseXFAConfig(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFAConfig() error = %v", err)
	}

	if config2.Present == nil || config.Present == nil {
		t.Fatal("Present is nil in round-trip")
	}

	if config2.Present.RenderPolicy != config.Present.RenderPolicy {
		t.Errorf("Round-trip RenderPolicy = %q, want %q", config2.Present.RenderPolicy, config.Present.RenderPolicy)
	}
}

func TestParseXFALocaleSet(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<localeSet default="en_US">
  <locale code="en_US" name="English (US)">
    <currency symbol="$" name="USD" precision="2" />
    <datePattern format="MM/DD/YYYY" />
  </locale>
</localeSet>`

	localeSet, err := ParseXFALocaleSet(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFALocaleSet() error = %v", err)
	}

	if localeSet == nil {
		t.Fatal("ParseXFALocaleSet() returned nil")
	}

	if localeSet.Default != "en_US" {
		t.Errorf("ParseXFALocaleSet() Default = %q, want %q", localeSet.Default, "en_US")
	}

	if len(localeSet.Locales) != 1 {
		t.Fatalf("ParseXFALocaleSet() locales count = %d, want 1", len(localeSet.Locales))
	}

	locale := localeSet.Locales[0]
	if locale.Code != "en_US" {
		t.Errorf("ParseXFALocaleSet() locale code = %q, want %q", locale.Code, "en_US")
	}

	if locale.Currency == nil {
		t.Fatal("ParseXFALocaleSet() locale Currency is nil")
	}

	if locale.Currency.Symbol != "$" {
		t.Errorf("ParseXFALocaleSet() Currency Symbol = %q, want %q", locale.Currency.Symbol, "$")
	}
}

func TestLocaleSetToXFA(t *testing.T) {
	localeSet := &types.XFALocaleSet{
		Default: "en_US",
		Locales: []types.XFALocale{
			{
				Code: "en_US",
				Name: "English (US)",
				Currency: &types.XFACurrency{
					Symbol:    "$",
					Name:      "USD",
					Precision: 2,
				},
			},
		},
	}

	xfaXML, err := LocaleSetToXFA(localeSet, false)
	if err != nil {
		t.Fatalf("LocaleSetToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "localeSet") {
		t.Error("LocaleSetToXFA() output does not contain localeSet")
	}
	if !strings.Contains(xfaXML, "en_US") {
		t.Error("LocaleSetToXFA() output does not contain locale code")
	}
}

func TestLocaleSetRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<localeSet default="en_US">
  <locale code="en_US" name="English (US)">
    <currency symbol="$" name="USD" precision="2" />
  </locale>
</localeSet>`

	// Parse to XFALocaleSet
	localeSet, err := ParseXFALocaleSet(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFALocaleSet() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := LocaleSetToXFA(localeSet, false)
	if err != nil {
		t.Fatalf("LocaleSetToXFA() error = %v", err)
	}

	// Parse again to verify
	localeSet2, err := ParseXFALocaleSet(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFALocaleSet() error = %v", err)
	}

	if localeSet2.Default != localeSet.Default {
		t.Errorf("Round-trip Default = %q, want %q", localeSet2.Default, localeSet.Default)
	}

	if len(localeSet2.Locales) != len(localeSet.Locales) {
		t.Errorf("Round-trip locales count = %d, want %d", len(localeSet2.Locales), len(localeSet.Locales))
	}
}

func TestParseXFAConnectionSet(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<connectionSet>
  <connection name="httpConn" type="http">
    <uri>http://example.com/api</uri>
  </connection>
</connectionSet>`

	connectionSet, err := ParseXFAConnectionSet(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAConnectionSet() error = %v", err)
	}

	if connectionSet == nil {
		t.Fatal("ParseXFAConnectionSet() returned nil")
	}

	if len(connectionSet.Connections) != 1 {
		t.Fatalf("ParseXFAConnectionSet() connections count = %d, want 1", len(connectionSet.Connections))
	}

	conn := connectionSet.Connections[0]
	if conn.Name != "httpConn" {
		t.Errorf("ParseXFAConnectionSet() connection name = %q, want %q", conn.Name, "httpConn")
	}

	if conn.Type != "http" {
		t.Errorf("ParseXFAConnectionSet() connection type = %q, want %q", conn.Type, "http")
	}
}

func TestConnectionSetToXFA(t *testing.T) {
	connectionSet := &types.XFAConnectionSet{
		Connections: []types.XFAConnection{
			{
				Name: "httpConn",
				Type: "http",
				Properties: map[string]interface{}{
					"uri": "http://example.com/api",
				},
			},
		},
	}

	xfaXML, err := ConnectionSetToXFA(connectionSet, false)
	if err != nil {
		t.Fatalf("ConnectionSetToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "connectionSet") {
		t.Error("ConnectionSetToXFA() output does not contain connectionSet")
	}
	if !strings.Contains(xfaXML, "httpConn") {
		t.Error("ConnectionSetToXFA() output does not contain connection name")
	}
}

func TestConnectionSetRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<connectionSet>
  <connection name="httpConn" type="http">
    <uri>http://example.com/api</uri>
  </connection>
</connectionSet>`

	// Parse to XFAConnectionSet
	connectionSet, err := ParseXFAConnectionSet(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFAConnectionSet() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := ConnectionSetToXFA(connectionSet, false)
	if err != nil {
		t.Fatalf("ConnectionSetToXFA() error = %v", err)
	}

	// Parse again to verify
	connectionSet2, err := ParseXFAConnectionSet(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFAConnectionSet() error = %v", err)
	}

	if len(connectionSet2.Connections) != len(connectionSet.Connections) {
		t.Errorf("Round-trip connections count = %d, want %d", len(connectionSet2.Connections), len(connectionSet.Connections))
	}
}

func TestParseXFAStylesheet(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<stylesheet>
  <style name="defaultStyle" type="font">
    <font family="Arial" size="12" />
  </style>
</stylesheet>`

	stylesheet, err := ParseXFAStylesheet(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAStylesheet() error = %v", err)
	}

	if stylesheet == nil {
		t.Fatal("ParseXFAStylesheet() returned nil")
	}

	if len(stylesheet.Styles) != 1 {
		t.Fatalf("ParseXFAStylesheet() styles count = %d, want 1", len(stylesheet.Styles))
	}

	style := stylesheet.Styles[0]
	if style.Name != "defaultStyle" {
		t.Errorf("ParseXFAStylesheet() style name = %q, want %q", style.Name, "defaultStyle")
	}
}

func TestStylesheetToXFA(t *testing.T) {
	stylesheet := &types.XFAStylesheet{
		Styles: []types.XFAStyle{
			{
				Name: "defaultStyle",
				Type: "font",
				Properties: map[string]interface{}{
					"font": &types.XFAStyleFont{
						Family: "Arial",
						Size:   12,
					},
				},
			},
		},
	}

	xfaXML, err := StylesheetToXFA(stylesheet, false)
	if err != nil {
		t.Fatalf("StylesheetToXFA() error = %v", err)
	}

	if !strings.Contains(xfaXML, "stylesheet") {
		t.Error("StylesheetToXFA() output does not contain stylesheet")
	}
	if !strings.Contains(xfaXML, "defaultStyle") {
		t.Error("StylesheetToXFA() output does not contain style name")
	}
}

func TestStylesheetRoundTrip(t *testing.T) {
	originalXML := `<?xml version="1.0"?>
<stylesheet>
  <style name="defaultStyle" type="font">
    <font family="Arial" size="12" />
  </style>
</stylesheet>`

	// Parse to XFAStylesheet
	stylesheet, err := ParseXFAStylesheet(originalXML, false)
	if err != nil {
		t.Fatalf("ParseXFAStylesheet() error = %v", err)
	}

	// Convert back to XML
	roundTripXML, err := StylesheetToXFA(stylesheet, false)
	if err != nil {
		t.Fatalf("StylesheetToXFA() error = %v", err)
	}

	// Parse again to verify
	stylesheet2, err := ParseXFAStylesheet(roundTripXML, false)
	if err != nil {
		t.Fatalf("Round-trip ParseXFAStylesheet() error = %v", err)
	}

	if len(stylesheet2.Styles) != len(stylesheet.Styles) {
		t.Errorf("Round-trip styles count = %d, want %d", len(stylesheet2.Styles), len(stylesheet.Styles))
	}
}

func TestPresenceHiddenField(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<template>
  <subform name="root">
    <field name="visibleField">
      <ui><textEdit/></ui>
      <caption><value><text>Visible</text></value></caption>
    </field>
    <field name="hiddenField" presence="hidden">
      <ui><textEdit/></ui>
      <caption><value><text>Hidden</text></value></caption>
    </field>
    <field name="invisibleField" presence="invisible">
      <ui><textEdit/></ui>
      <caption><value><text>Invisible</text></value></caption>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	byName := map[string]*types.Question{}
	for i := range form.Questions {
		byName[form.Questions[i].Name] = &form.Questions[i]
	}

	if q := byName["visibleField"]; q == nil {
		t.Fatal("visibleField not found")
	} else if q.Hidden {
		t.Error("visibleField should not be hidden")
	}

	if q := byName["hiddenField"]; q == nil {
		t.Fatal("hiddenField not found")
	} else if !q.Hidden {
		t.Error("hiddenField with presence='hidden' should have q.Hidden=true")
	}

	if q := byName["invisibleField"]; q == nil {
		t.Fatal("invisibleField not found")
	} else if !q.Hidden {
		t.Error("invisibleField with presence='invisible' should have q.Hidden=true")
	}
}

func TestPresenceVisibleField(t *testing.T) {
	xfaXML := `<?xml version="1.0"?>
<template>
  <subform name="root">
    <field name="explicitVisible" presence="visible">
      <ui><textEdit/></ui>
      <caption><value><text>Explicit Visible</text></value></caption>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("expected at least 1 question")
	}
	q := form.Questions[0]
	if q.Hidden {
		t.Errorf("field with presence='visible' should not be hidden, got Hidden=%v", q.Hidden)
	}
}

// ── Rendering improvement tests ───────────────────────────────────────────────

// TestPositionProperties verifies that x/y/w/h attributes on fields and subforms
// are captured and surfaced in Question.Properties and FormSection.
func TestPositionProperties(t *testing.T) {
	xfaXML := `<template>
  <subform name="Sect" layout="position" w="190mm" h="100mm">
    <field name="nameField" x="10mm" y="5mm" w="80mm" h="6mm">
      <ui><textEdit/></ui>
      <toolTip>Name</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions parsed")
	}
	q := form.Questions[0]
	for _, key := range []string{"x", "y", "w", "h"} {
		if q.Properties == nil || q.Properties[key] == nil {
			t.Errorf("question Properties[%q] missing", key)
		}
	}
	if q.Properties["x"] != "10mm" {
		t.Errorf("x = %v, want 10mm", q.Properties["x"])
	}
	if q.Properties["y"] != "5mm" {
		t.Errorf("y = %v, want 5mm", q.Properties["y"])
	}

	// Subform layout and dimensions on FormSection.
	if len(form.Sections) == 0 {
		t.Fatal("no sections")
	}
	sec := form.Sections[0]
	if sec.Layout != "position" {
		t.Errorf("section Layout = %q, want position", sec.Layout)
	}
	if sec.Width != "190mm" {
		t.Errorf("section Width = %q, want 190mm", sec.Width)
	}
	if sec.Height != "100mm" {
		t.Errorf("section Height = %q, want 100mm", sec.Height)
	}
}

// TestMaxCharsToValidation verifies that textEdit maxChars maps to ValidationRules.MaxLength.
func TestMaxCharsToValidation(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="shortText">
      <ui><textEdit maxChars="50"/></ui>
      <toolTip>Short text</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Validation == nil {
		t.Fatal("Validation is nil, want MaxLength=50")
	}
	if q.Validation.MaxLength == nil || *q.Validation.MaxLength != 50 {
		t.Errorf("MaxLength = %v, want 50", q.Validation.MaxLength)
	}
}

// TestCaptionPlacement verifies that <caption placement="right"> is surfaced in Properties.
func TestCaptionPlacement(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="agree">
      <ui><checkButton/></ui>
      <caption placement="right"><value><text>I agree</text></value></caption>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Label != "I agree" {
		t.Errorf("label = %q, want I agree", q.Label)
	}
	if q.Properties == nil || q.Properties["caption_placement"] != "right" {
		t.Errorf("caption_placement = %v, want right", q.Properties["caption_placement"])
	}
}

// TestChoiceListListbox verifies that open="always" is surfaced as Properties["listbox"]=true.
func TestChoiceListListbox(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="multiChoice">
      <ui><choiceList open="always" multiSelect="1"/></ui>
      <toolTip>Pick options</toolTip>
      <items><text>A</text><text>B</text></items>
    </field>
    <field name="dropdown">
      <ui><choiceList open="userInteraction"/></ui>
      <toolTip>Pick one</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	if q, ok := byName["multiChoice"]; !ok {
		t.Error("multiChoice missing")
	} else {
		if q.Properties == nil || q.Properties["listbox"] != true {
			t.Errorf("multiChoice listbox = %v, want true", q.Properties["listbox"])
		}
		if q.Properties["multi_select"] != true {
			t.Errorf("multiChoice multi_select = %v, want true", q.Properties["multi_select"])
		}
	}

	if q, ok := byName["dropdown"]; !ok {
		t.Error("dropdown missing")
	} else {
		if q.Properties != nil && q.Properties["listbox"] == true {
			t.Error("dropdown should not have listbox=true")
		}
	}
}

// TestPasswordEditType verifies that passwordEdit fields map to ResponseTypePassword.
func TestPasswordEditType(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="pwd">
      <ui><passwordEdit/></ui>
      <toolTip>Password</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Type != types.ResponseTypePassword {
		t.Errorf("type = %q, want password", q.Type)
	}
}

// TestCheckButtonAllowNeutral verifies that allowNeutral="1" is surfaced in Properties.
func TestCheckButtonAllowNeutral(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="tristate">
      <ui><checkButton allowNeutral="1"/></ui>
      <caption placement="right"><value><text>Optional flag</text></value></caption>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Properties == nil || q.Properties["allow_neutral"] != true {
		t.Errorf("allow_neutral = %v, want true", q.Properties["allow_neutral"])
	}
}

// TestNumericEditConstraints verifies that fracDigits and leadDigits are surfaced.
func TestNumericEditConstraints(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="amount">
      <ui><numericEdit fracDigits="2" leadDigits="6"/></ui>
      <toolTip>Amount</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Properties == nil {
		t.Fatal("Properties nil")
	}
	if q.Properties["frac_digits"] != 2 {
		t.Errorf("frac_digits = %v, want 2", q.Properties["frac_digits"])
	}
	if q.Properties["lead_digits"] != 6 {
		t.Errorf("lead_digits = %v, want 6", q.Properties["lead_digits"])
	}
}

// TestExclGroupLayout verifies that exclGroup layout is surfaced in Properties.
func TestExclGroupLayout(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <exclGroup name="choice" layout="lr-tb">
      <field name="optA"><ui><radioButton/></ui><caption><value><text>A</text></value></caption></field>
      <field name="optB"><ui><radioButton/></ui><caption><value><text>B</text></value></caption></field>
    </exclGroup>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	if len(form.Questions) == 0 {
		t.Fatal("no questions")
	}
	q := form.Questions[0]
	if q.Type != types.ResponseTypeRadio {
		t.Errorf("type = %q, want radio", q.Type)
	}
	if q.Properties == nil || q.Properties["layout"] != "lr-tb" {
		t.Errorf("layout = %v, want lr-tb", q.Properties["layout"])
	}
}

// TestDateTimeEditTimeSubtype verifies that a picture with TIME{} maps to ResponseTypeTime.
func TestDateTimeEditTimeSubtype(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="startTime">
      <ui><dateTimeEdit/></ui>
      <toolTip>Start time</toolTip>
      <format><picture>TIME{HH:MM:SS}</picture></format>
    </field>
    <field name="startDate">
      <ui><dateTimeEdit/></ui>
      <toolTip>Start date</toolTip>
      <format><picture>DATE{MM/DD/YYYY}</picture></format>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	if q, ok := byName["startTime"]; !ok {
		t.Error("startTime missing")
	} else if q.Type != types.ResponseTypeTime {
		t.Errorf("startTime type = %q, want time", q.Type)
	}

	if q, ok := byName["startDate"]; !ok {
		t.Error("startDate missing")
	} else if q.Type != types.ResponseTypeDate {
		t.Errorf("startDate type = %q, want date", q.Type)
	}
}

// TestExDataHTMLExtraction verifies that text/html exData without xfa:embed markers
// is extracted as display text rather than suppressed.
func TestExDataHTMLExtraction(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <draw name="richLabel">
      <value>
        <exData contentType="text/html">
          <body xmlns="http://www.w3.org/1999/xhtml">
            <p>510(k) Number <span style="font-style:italic">(if known)</span></p>
          </body>
        </exData>
      </value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	q, ok := byName["richLabel"]
	if !ok {
		t.Fatal("richLabel draw should appear as a display question when in an interactive section")
	}
	if q.Type != types.ResponseTypeDisplay {
		t.Errorf("type = %q, want display", q.Type)
	}
	if !strings.Contains(q.Label, "510(k) Number") {
		t.Errorf("label = %q, should contain '510(k) Number'", q.Label)
	}
	if strings.Contains(q.Label, "<") || strings.Contains(q.Label, ">") {
		t.Errorf("label should not contain HTML tags: %q", q.Label)
	}
}

// TestExDataPageCounterSuppressed verifies page-counter draws (xfa:embed) stay suppressed.
func TestExDataPageCounterSuppressed(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="realField">
      <ui><textEdit/></ui>
      <toolTip>Real field</toolTip>
    </field>
    <draw name="pageCounter">
      <value>
        <exData contentType="text/html">
          <body><p>Page <span xfa:embed="#floatingField1"/> of <span xfa:embed="#floatingField2"/></p></body>
        </exData>
      </value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	for _, q := range form.Questions {
		if q.Name == "pageCounter" {
			t.Errorf("page-counter draw should be suppressed, got type=%s label=%q", q.Type, q.Label)
		}
	}
}

// TestNonInteractiveSectionContent verifies that static text in non-interactive subforms
// is collected into FormSection.Content rather than the flat Questions list.
func TestNonInteractiveSectionContent(t *testing.T) {
	xfaXML := `<template>
  <subform name="Header">
    <draw name="title"><value><text>Indications for Use</text></value></draw>
    <draw name="subtitle"><value><text>Form FDA 3881</text></value></draw>
  </subform>
  <subform name="Body">
    <field name="indication">
      <ui><textEdit multiLine="1"/></ui>
      <toolTip>Describe the indication</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	// Header draws should NOT appear in flat Questions list.
	for _, q := range form.Questions {
		if q.Name == "title" || q.Name == "subtitle" {
			t.Errorf("non-interactive draw %q should not be in flat Questions list", q.Name)
		}
	}

	// Header section should have Content populated.
	var headerSec *types.FormSection
	for i := range form.Sections {
		if form.Sections[i].Name == "Header" {
			headerSec = &form.Sections[i]
		}
	}
	if headerSec == nil {
		t.Fatal("Header section missing")
	}
	if len(headerSec.Content) == 0 {
		t.Fatal("Header section Content is empty, want static text items")
	}
	found := false
	for _, c := range headerSec.Content {
		if strings.Contains(c, "Indications for Use") {
			found = true
		}
	}
	if !found {
		t.Errorf("'Indications for Use' not found in Header.Content: %v", headerSec.Content)
	}
}

// TestSeparatorDraw verifies that <draw><value><line> is emitted as ResponseTypeSeparator.
func TestSeparatorDraw(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="aboveField">
      <ui><textEdit/></ui>
      <toolTip>Above</toolTip>
    </field>
    <draw name="divider" w="190mm" h="0pt">
      <value><line><edge thickness="1pt"/></line></value>
    </draw>
    <field name="belowField">
      <ui><textEdit/></ui>
      <toolTip>Below</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	q, ok := byName["divider"]
	if !ok {
		t.Fatal("divider draw should be emitted as separator question")
	}
	if q.Type != types.ResponseTypeSeparator {
		t.Errorf("divider type = %q, want separator", q.Type)
	}
}

// TestParaHAlignAndFontProperties verifies that <para hAlign> and <font size/weight>
// are captured in Question.Properties for display draws.
func TestParaHAlignAndFontProperties(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="anchor">
      <ui><textEdit/></ui>
      <toolTip>Anchor field</toolTip>
    </field>
    <draw name="heading">
      <value><text>Section Title</text></value>
      <para hAlign="center"/>
      <font size="14pt" weight="bold"/>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	q, ok := byName["heading"]
	if !ok {
		t.Fatal("heading draw missing from Questions")
	}
	if q.Properties == nil {
		t.Fatal("heading Properties nil")
	}
	if q.Properties["text_align"] != "center" {
		t.Errorf("text_align = %v, want center", q.Properties["text_align"])
	}
	if q.Properties["font_size"] != "14pt" {
		t.Errorf("font_size = %v, want 14pt", q.Properties["font_size"])
	}
	if q.Properties["font_weight"] != "bold" {
		t.Errorf("font_weight = %v, want bold", q.Properties["font_weight"])
	}
}

// TestImageHRefParsing verifies that an <image href="..."> attribute is captured
// as image_href in properties and that contentType is not corrupted by it.
func TestImageHRefParsing(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="SomeField">
      <ui><textEdit/></ui>
      <caption><value><text>Name</text></value></caption>
    </field>
    <draw name="LogoDraw">
      <value>
        <image contentType="image/png" href="$rr:logo.png"/>
      </value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	var img *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "LogoDraw" {
			img = &form.Questions[i]
			break
		}
	}
	if img == nil {
		t.Fatal("LogoDraw missing — href-only image should still be emitted")
	}
	if img.Type != types.ResponseTypeImage {
		t.Errorf("type = %v, want image", img.Type)
	}
	if img.Properties == nil {
		t.Fatal("Properties nil")
	}
	if img.Properties["image_href"] != "$rr:logo.png" {
		t.Errorf("image_href = %v, want $rr:logo.png", img.Properties["image_href"])
	}
	if img.Properties["content_type"] != "image/png" {
		t.Errorf("content_type = %v, want image/png (href must not overwrite contentType)", img.Properties["content_type"])
	}
	if _, hasData := img.Properties["image_data"]; hasData {
		t.Error("image_data should be absent when no inline data is present")
	}
}

// TestImageHRefResolution verifies that ParseXFAFormWithResources fills in
// image_data when a matching resource is supplied.
func TestImageHRefResolution(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="SomeField">
      <ui><textEdit/></ui>
      <caption><value><text>Name</text></value></caption>
    </field>
    <draw name="LogoDraw">
      <value>
        <image contentType="image/png" href="$rr:logo.png"/>
      </value>
    </draw>
  </subform>
</template>`

	fakeImageBytes := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	resources := map[string][]byte{
		"logo.png": fakeImageBytes,
	}

	form, err := ParseXFAFormWithResources(xfaXML, resources, false)
	if err != nil {
		t.Fatalf("ParseXFAFormWithResources() error = %v", err)
	}
	var img *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "LogoDraw" {
			img = &form.Questions[i]
			break
		}
	}
	if img == nil {
		t.Fatal("LogoDraw missing")
	}
	if img.Properties["image_data"] == "" {
		t.Error("image_data should be filled in after resource resolution")
	}
	want := "iVBORw=="
	if img.Properties["image_data"] != want {
		t.Errorf("image_data = %v, want %v", img.Properties["image_data"], want)
	}
}

// TestPresenceAttrImageEmitted verifies that image draws with a presence= attribute
// are still emitted (as hidden questions), not silently suppressed. This covers
// conditional logo images like the FDA/HC/IMDRFRF monograms in the eSTAR form.
func TestPresenceAttrImageEmitted(t *testing.T) {
	xfaXML := `<template>
  <subform name="Page1">
    <field name="SomeField">
      <ui><textEdit/></ui>
      <caption><value><text>Name</text></value></caption>
    </field>
    <draw name="CondLogo" presence="hidden">
      <value>
        <image contentType="image/png">iVBORw0KGgo=</image>
      </value>
    </draw>
    <draw name="ActiveLogo" presence="visible">
      <value>
        <image contentType="image/jpeg">iVBORw0KGgo=</image>
      </value>
    </draw>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	condLogo, ok := byName["CondLogo"]
	if !ok {
		t.Fatal("CondLogo missing — presence=hidden image draw should still be emitted")
	}
	if condLogo.Type != types.ResponseTypeImage {
		t.Errorf("CondLogo type = %v, want image", condLogo.Type)
	}
	if !condLogo.Hidden {
		t.Error("CondLogo should be hidden (presence=hidden)")
	}

	activeLogo, ok := byName["ActiveLogo"]
	if !ok {
		t.Fatal("ActiveLogo missing")
	}
	if activeLogo.Hidden {
		t.Error("ActiveLogo should not be hidden (presence=visible)")
	}
}

// TestVariablesBlockExtracted verifies that <variables><script> blocks are
// exposed verbatim as FormScripts on the schema. pdfer no longer interprets
// the body; callers can parse the JavaScript themselves.
func TestVariablesBlockExtracted(t *testing.T) {
	body := `
      function AutoPopulate() {
        if (AppType.rawValue == "0") {
          SectionA.presence = "visible";
        } else if (AppType.rawValue == "1") {
          SectionB.presence = "visible";
        }
      }
    `
	xfaXML := `<template>
  <variables>
    <script name="Functions" contentType="application/x-javascript">` + body + `</script>
  </variables>
  <subform name="Page1">
    <field name="anchor"><ui><textEdit/></ui></field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var variablesScripts []types.FormScript
	for _, s := range form.Scripts {
		if s.Event == "variables" {
			variablesScripts = append(variablesScripts, s)
		}
	}
	if len(variablesScripts) != 1 {
		t.Fatalf("expected 1 variables script, got %d: %+v", len(variablesScripts), variablesScripts)
	}
	s := variablesScripts[0]
	if s.Name != "Functions" {
		t.Errorf("script name = %q, want %q", s.Name, "Functions")
	}
	if s.Language != "javascript" {
		t.Errorf("language = %q, want javascript", s.Language)
	}
	if s.Body != body {
		t.Errorf("body not preserved verbatim:\n got: %q\nwant: %q", s.Body, body)
	}
}

// TestAddAttachmentSectionInteractive verifies that a subform containing only
// AddAttachment button fields (bind="none") is treated as interactive. These
// buttons are emitted as file questions and must cause their containing section
// to appear in the navigation — they are not data-bound but they are user-facing.
func TestAddAttachmentSectionInteractive(t *testing.T) {
	xfaXML := `<template>
  <subform name="CoverLetter">
    <subform name="AttachmentSlot">
      <field name="CLAddAttachment110">
        <ui><button/></ui>
        <caption><value><text>Add Attachment</text></value></caption>
        <bind match="none"/>
      </field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	// CLAddAttachment110 must be emitted as a file question.
	var fileQ *types.Question
	for i := range form.Questions {
		if form.Questions[i].Name == "CLAddAttachment110" {
			fileQ = &form.Questions[i]
		}
	}
	if fileQ == nil {
		t.Fatal("CLAddAttachment110 not found in Questions")
	}
	if fileQ.Type != types.ResponseTypeFile {
		t.Errorf("CLAddAttachment110 type = %q, want %q", fileQ.Type, types.ResponseTypeFile)
	}

	// CoverLetter section must be marked interactive so it appears in nav.
	var coverLetterSec *types.FormSection
	for i := range form.Sections {
		if form.Sections[i].Name == "CoverLetter" {
			coverLetterSec = &form.Sections[i]
		}
	}
	if coverLetterSec == nil {
		t.Fatal("CoverLetter section not found")
	}
	if !coverLetterSec.Interactive {
		t.Error("CoverLetter.Interactive = false, want true (contains AddAttachment file question)")
	}
}

// TestSectionContentNoiseFiltered verifies that graphical status indicator draws
// (imageEdit elements with accessibility captions like "Required Question Incomplete")
// are excluded from FormSection.Content, and that draws whose only text is in their
// Caption — not in their Value — are likewise excluded. Only draws with actual visible
// text (from <value><text>) are collected.
func TestSectionContentNoiseFiltered(t *testing.T) {
	xfaXML := `<template>
  <subform name="Header">
    <draw name="statusBar">
      <ui><imageEdit/></ui>
      <caption><value><text>Required Question Incomplete</text></value></caption>
    </draw>
    <draw name="statusDot">
      <ui><imageEdit/></ui>
      <caption><value><text>Optional Question</text></value></caption>
    </draw>
    <draw name="captionOnlyDraw">
      <caption><value><text>Top Border</text></value></caption>
    </draw>
    <draw name="instruction">
      <value><text>Please complete all required fields before submitting.</text></value>
    </draw>
    <draw name="reference">
      <value><text>IMDRF TOC CH1.04</text></value>
    </draw>
  </subform>
  <subform name="Body">
    <field name="deviceName">
      <ui><textEdit/></ui>
      <toolTip>Device name</toolTip>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var headerSec *types.FormSection
	for i := range form.Sections {
		if form.Sections[i].Name == "Header" {
			headerSec = &form.Sections[i]
		}
	}
	if headerSec == nil {
		t.Fatal("Header section not found")
	}

	for _, c := range headerSec.Content {
		switch c {
		case "Required Question Incomplete", "Optional Question", "Top Border":
			t.Errorf("noise string %q must not appear in Content", c)
		}
	}

	want := map[string]bool{
		"Please complete all required fields before submitting.": false,
		"IMDRF TOC CH1.04": false,
	}
	for _, c := range headerSec.Content {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for text, found := range want {
		if !found {
			t.Errorf("expected %q in Content, not found; Content = %v", text, headerSec.Content)
		}
	}
}

// TestHTMLCaptionExtraction verifies that a field whose <caption> contains
// <exData contentType="text/html"> gets the stripped plain text as its label,
// not the field name (which is what happens when Caption remains empty).
func TestHTMLCaptionExtraction(t *testing.T) {
	xfaXML := `<template>
  <subform name="Section1">
    <field name="BCDropDownList105">
      <ui><choiceList><listOpen>true</listOpen></choiceList></ui>
      <caption>
        <value>
          <exData contentType="text/html">
            <body xmlns="http://www.w3.org/1999/xhtml">
              <p>How many tissue contacting products/components/materials are there?</p>
            </body>
          </exData>
        </value>
      </caption>
      <items>
        <text>1</text>
        <text>2</text>
      </items>
    </field>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}
	byName := make(map[string]types.Question)
	for _, q := range form.Questions {
		byName[q.Name] = q
	}

	q, ok := byName["BCDropDownList105"]
	if !ok {
		t.Fatal("BCDropDownList105 not found in questions")
	}
	wantLabel := "How many tissue contacting products/components/materials are there?"
	if q.Label != wantLabel {
		t.Errorf("label = %q, want %q", q.Label, wantLabel)
	}
	if strings.Contains(q.Label, "<") || strings.Contains(q.Label, ">") {
		t.Errorf("label must not contain HTML tags: %q", q.Label)
	}
}

// TestDrawTextValueBeatsToolTip verifies that for draw/heading elements, the
// visible <value><text> content is used as the section label rather than
// <assist><toolTip>, which XFA authors use for accessibility annotations
// (e.g. IMDRF TOC chapter references) that are not intended as display text.
// Matches the real eSTAR structure: CoverLetter contains imageEdit draws (which
// are skipped by claimDrawLabels) and then CLHeading (which the first-child scan
// claims). The field inside has bind=none so it doesn't steal the draw label.
func TestDrawTextValueBeatsToolTip(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <subform name="CoverLetter">
      <draw name="YesIndicator"><ui><imageEdit/></ui></draw>
      <draw name="CLHeading">
        <ui><textEdit/></ui>
        <value><text>Cover Letter / Letters of Reference</text></value>
        <assist role="H1"><toolTip>IMDRF TOC CH1.01, CH1.13</toolTip></assist>
      </draw>
      <field name="CLAddAttachment" w="34.925mm">
        <ui><button/></ui>
        <caption><value><text>Add Attachment</text></value></caption>
        <bind match="none"/>
      </field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var coverLetter *types.FormSection
	if len(form.Sections) > 0 {
		for _, child := range form.Sections[0].Children {
			c := child
			if c.Name == "CoverLetter" {
				coverLetter = &c
				break
			}
		}
	}
	if coverLetter == nil {
		t.Fatal("CoverLetter section not found")
	}

	wantLabel := "Cover Letter / Letters of Reference"
	if coverLetter.Label != wantLabel {
		t.Errorf("section label = %q, want %q (ToolTip must not override visible Value text)", coverLetter.Label, wantLabel)
	}

	wantTooltip := "IMDRF TOC CH1.01, CH1.13"
	if coverLetter.Tooltip != wantTooltip {
		t.Errorf("section tooltip = %q, want %q", coverLetter.Tooltip, wantTooltip)
	}
}

// TestBookmarkNameAsLabel verifies that <extras name="bookmark"><text name="name">
// is used as the authoritative section label, overriding any instruction-text draw
// that claimDrawLabels would otherwise claim as the label.
func TestBookmarkNameAsLabel(t *testing.T) {
	xfaXML := `<template>
  <subform name="form1">
    <draw name="BCText1">
      <value><text>If this section is not applicable, please attach a statement explaining why.</text></value>
    </draw>
    <subform name="PatientMaterials">
      <extras name="bookmark">
        <text name="name">Tissue Contacting Materials</text>
      </extras>
      <field name="MaterialsList">
        <ui><textEdit/></ui>
      </field>
    </subform>
  </subform>
</template>`

	form, err := ParseXFAForm(xfaXML, false)
	if err != nil {
		t.Fatalf("ParseXFAForm() error = %v", err)
	}

	var patientMaterials *types.FormSection
	if len(form.Sections) > 0 {
		for _, child := range form.Sections[0].Children {
			c := child
			if c.Name == "PatientMaterials" {
				patientMaterials = &c
				break
			}
		}
	}
	if patientMaterials == nil {
		t.Fatal("PatientMaterials section not found")
	}

	wantLabel := "Tissue Contacting Materials"
	if patientMaterials.Label != wantLabel {
		t.Errorf("section label = %q, want %q (bookmark name must override claimed draw text)", patientMaterials.Label, wantLabel)
	}
}
