package tests

import (
	"bytes"
	"os"
	"testing"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	"github.com/benedoc-inc/pdfer/v2/forms"
	"github.com/benedoc-inc/pdfer/v2/forms/xfa"
)

// TestFDA3881_Detection verifies that Form 3881 is correctly identified as an XFA form.
// Form 3881 ("Indications for Use") is a pure XFA form with no AcroForm fields.
func TestFDA3881_Detection(t *testing.T) {
	pdfBytes := readFDA3881(t)

	formType, err := forms.Detect(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if formType != forms.FormTypeXFA {
		t.Errorf("expected form type %q, got %q", forms.FormTypeXFA, formType)
	}
}

// TestFDA3881_Schema verifies that pdfer correctly extracts the five fields
// defined in Form 3881's XFA template.
func TestFDA3881_Schema(t *testing.T) {
	pdfBytes := readFDA3881(t)

	schema, _, err := forms.ExtractXFA(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractXFA: %v", err)
	}
	if schema == nil {
		t.Fatal("ExtractXFA returned nil schema")
	}

	// Form 3881's five interactive fields must all be present.
	// Additional display elements from the same sections are also expected.
	want := []string{"number510", "devicename", "indications", "prescript", "overthe"}
	byName := make(map[string]string, len(schema.Questions))
	for _, q := range schema.Questions {
		byName[q.Name] = string(q.Type)
	}
	for _, name := range want {
		if _, ok := byName[name]; !ok {
			t.Errorf("field %q missing from schema", name)
		}
	}
}

// TestFDA3881_Fill fills Form 3881 with sample data and verifies the field
// values are written into the XFA datasets stream of the output PDF.
func TestFDA3881_Fill(t *testing.T) {
	pdfBytes := readFDA3881(t)

	form, err := pdfer.ExtractForm(pdfBytes, nil, false)
	if err != nil {
		t.Fatalf("ExtractForm: %v", err)
	}
	if form.Type() != forms.FormTypeXFA {
		t.Fatalf("expected XFA form, got %v", form.Type())
	}

	fillData := pdfer.FormData{
		"number510":   "K241234",
		"devicename":  "InVitro Glucose Monitor",
		"indications": "For professional use in measuring blood glucose in adults.",
		"prescript":   "1",
		"overthe":     "0",
	}

	filled, err := form.Fill(pdfBytes, fillData, nil, false)
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if len(filled) == 0 {
		t.Fatal("Fill returned empty PDF")
	}
	if !bytes.HasPrefix(filled, []byte("%PDF-")) {
		t.Error("Fill output does not start with %PDF- header")
	}

	// Re-extract datasets from the filled PDF and verify field values.
	streams, err := xfa.ExtractAllXFAStreams(filled, nil, false)
	if err != nil {
		t.Fatalf("re-extract XFA streams: %v", err)
	}
	if streams.Datasets == nil || len(streams.Datasets.Data) == 0 {
		t.Fatal("datasets stream missing from filled PDF")
	}

	datasetsXML := string(streams.Datasets.Data)

	wantSubstrings := map[string]string{
		"number510":   "<number510>K241234</number510>",
		"devicename":  "<devicename>InVitro Glucose Monitor</devicename>",
		"indications": "<indications>For professional use in measuring blood glucose in adults.</indications>",
		"prescript":   "<prescript>1</prescript>",
		"overthe":     "<overthe>0</overthe>",
	}
	for field, want := range wantSubstrings {
		if !bytes.Contains([]byte(datasetsXML), []byte(want)) {
			t.Errorf("field %q: expected %q in datasets XML\ngot:\n%s", field, want, datasetsXML)
		}
	}
}

// readFDA3881 loads the Form 3881 fixture and skips if it is not present.
func readFDA3881(t *testing.T) []byte {
	t.Helper()
	path := getTestResourcePath("fda_3881.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("fda_3881.pdf not found at %s — add it to tests/resources/ to run this test", path)
	}
	return data
}
