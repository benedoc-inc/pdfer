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

// TestFDA3881_Fill_XRefStreamIncremental verifies that XFA fill works on PDFs
// whose xref is a cross-reference stream (PDF 1.5+). FDA Form 3881 is exactly
// that shape. Byte-rewrite cannot patch binary xref streams, so the fill must
// route through a PDF incremental update (issue #12): the original bytes are
// preserved verbatim as a prefix, a replacement datasets stream and an
// xref-stream section are appended, and the filled values are readable back.
func TestFDA3881_Fill_XRefStreamIncremental(t *testing.T) {
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

	// Incremental update: original revision preserved verbatim, new revision
	// appended with an xref stream (not a classical trailer, per §7.5.8.4).
	if !bytes.HasPrefix(filled, pdfBytes) {
		t.Error("filled PDF must preserve the original bytes as a prefix")
	}
	appended := filled[len(pdfBytes):]
	if !bytes.Contains(appended, []byte("/Type/XRef")) {
		t.Error("appended revision must use a cross-reference stream")
	}
	if bytes.Contains(appended, []byte("trailer")) {
		t.Error("appended revision must not use a classical trailer")
	}

	// The filled values must read back from the updated datasets.
	xfaXML, _, err := xfa.FindXFADatasetsStream(filled, nil, false)
	if err != nil {
		t.Fatalf("FindXFADatasetsStream on filled PDF: %v", err)
	}
	for field, want := range map[string]string{
		"number510":  "K241234",
		"devicename": "InVitro Glucose Monitor",
	} {
		if !bytes.Contains(xfaXML, []byte(want)) {
			t.Errorf("filled datasets missing value %q for field %q", want, field)
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
