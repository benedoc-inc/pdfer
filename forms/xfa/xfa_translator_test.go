package xfa

import (
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
