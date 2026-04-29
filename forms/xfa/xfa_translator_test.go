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

func TestMultiTargetVisibilityScript(t *testing.T) {
	script := `if (this.rawValue == "1") {
	IMDRF.presence = "visible";
	USA.presence = "hidden";
	CDN.presence = "hidden";
}`
	results, ok := tryParseVisibilityScript(script, "javascript", "AppType", "")
	if !ok {
		t.Fatal("tryParseVisibilityScript should have matched")
	}
	result := results[0]
	if result.ruleType != types.RuleTypeVisibility {
		t.Errorf("ruleType = %q, want visibility", result.ruleType)
	}
	if len(result.actions) != 3 {
		t.Fatalf("actions count = %d, want 3; got %+v", len(result.actions), result.actions)
	}

	byTarget := map[string]types.ActionType{}
	for _, a := range result.actions {
		byTarget[a.Target] = a.Type
	}
	if byTarget["IMDRF"] != types.ActionTypeShow {
		t.Errorf("IMDRF action = %q, want show", byTarget["IMDRF"])
	}
	if byTarget["USA"] != types.ActionTypeHide {
		t.Errorf("USA action = %q, want hide", byTarget["USA"])
	}
	if byTarget["CDN"] != types.ActionTypeHide {
		t.Errorf("CDN action = %q, want hide", byTarget["CDN"])
	}
}

func TestThisPresenceFallback(t *testing.T) {
	// Script only uses "this.presence" — no named external targets.
	// Should fall back to sourceField.
	script := `this.presence = "hidden";`
	results, ok := tryParseVisibilityScript(script, "javascript", "myField", "")
	if !ok {
		t.Fatal("tryParseVisibilityScript should have matched")
	}
	result := results[0]
	if len(result.actions) != 1 {
		t.Fatalf("actions count = %d, want 1", len(result.actions))
	}
	if result.actions[0].Target != "myField" {
		t.Errorf("action target = %q, want myField", result.actions[0].Target)
	}
	if result.actions[0].Type != types.ActionTypeHide {
		t.Errorf("action type = %q, want hide", result.actions[0].Type)
	}
}

func TestPerTargetActionType(t *testing.T) {
	script := `IMDRF.presence = "visible"; USA.presence = "hidden";`
	if got := perTargetActionType(script, "IMDRF", types.ActionTypeHide); got != types.ActionTypeShow {
		t.Errorf("IMDRF: got %q, want show", got)
	}
	if got := perTargetActionType(script, "USA", types.ActionTypeShow); got != types.ActionTypeHide {
		t.Errorf("USA: got %q, want hide", got)
	}
	// Unknown target falls back to the provided default.
	if got := perTargetActionType(script, "OTHER", types.ActionTypeShow); got != types.ActionTypeShow {
		t.Errorf("OTHER: got %q, want show (fallback)", got)
	}
}
