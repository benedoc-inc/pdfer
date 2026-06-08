package acroform

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/core/write"
	"github.com/benedoc-inc/pdfer/v2/types"
)

// buildCheckboxPDF creates a one-page PDF with a single unchecked checkbox
// field named "agree".
func buildCheckboxPDF(t *testing.T) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	builder.FinalizePage(page)

	fb := NewFormBuilder(builder)
	fb.AddCheckbox("agree", []float64{72, 650, 90, 668}, 0)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm: %v", err)
	}
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return b
}

// --- unit tests for helpers -------------------------------------------------

func TestBtnCheckboxState(t *testing.T) {
	cases := []struct {
		input interface{}
		want  string
	}{
		{true, "Yes"},
		{false, "Off"},
		{"true", "Yes"},
		{"yes", "Yes"},
		{"Yes", "Yes"},
		{"on", "Yes"},
		{"checked", "Yes"},
		{"off", "Off"},
		{"false", "Off"},
		{"no", "Off"},
		{"", "Off"},
	}
	for _, c := range cases {
		if got := btnCheckboxState(c.input); got != c.want {
			t.Errorf("btnCheckboxState(%v) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestWithAS_SetsNewEntry(t *testing.T) {
	body := []byte("<</FT/Btn/V/Off>>")
	got := withAS(body, "Yes")
	if !bytes.Contains(got, []byte("/AS /Yes")) {
		t.Errorf("withAS did not add /AS: %s", got)
	}
}

func TestWithAS_ReplacesExisting(t *testing.T) {
	body := []byte("<</FT/Btn/V/Yes/AS/Off>>")
	got := withAS(body, "Yes")
	if !bytes.Contains(got, []byte("/AS /Yes")) {
		t.Errorf("withAS did not update /AS: %s", got)
	}
	if bytes.Contains(got, []byte("/AS /Off")) {
		t.Errorf("withAS left old /AS /Off in body: %s", got)
	}
}

func TestWithBtnAP_AddsEntry(t *testing.T) {
	body := []byte("<</FT/Btn/V/Off>>")
	got := withBtnAP(body, 5, 6)
	if !bytes.Contains(got, []byte("/AP")) {
		t.Errorf("withBtnAP did not add /AP: %s", got)
	}
	if !bytes.Contains(got, []byte("/Yes 5 0 R")) {
		t.Errorf("withBtnAP missing /Yes ref: %s", got)
	}
	if !bytes.Contains(got, []byte("/Off 6 0 R")) {
		t.Errorf("withBtnAP missing /Off ref: %s", got)
	}
}

func TestBtnKidExportValue_FromAP(t *testing.T) {
	body := []byte("<</Subtype/Widget/AS/Off/AP<</N<</Yes 10 0 R/Off 11 0 R>>>>>>")
	got := btnKidExportValue(body)
	if got != "Yes" {
		t.Errorf("btnKidExportValue = %q, want %q", got, "Yes")
	}
}

func TestBtnKidExportValue_FallbackAS(t *testing.T) {
	// No /AP but has non-Off /AS
	body := []byte("<</Subtype/Widget/AS/Option2>>")
	got := btnKidExportValue(body)
	if got != "Option2" {
		t.Errorf("btnKidExportValue = %q, want %q", got, "Option2")
	}
}

func TestIsBtnPushButton(t *testing.T) {
	if isBtnPushButton(0) {
		t.Error("0 should not be pushbutton")
	}
	if !isBtnPushButton(0x10000) {
		t.Error("0x10000 should be pushbutton")
	}
}

func TestIsBtnRadio(t *testing.T) {
	if isBtnRadio(0) {
		t.Error("0 should not be radio")
	}
	if !isBtnRadio(0x8000) {
		t.Error("0x8000 should be radio")
	}
}

// --- integration tests -------------------------------------------------------

func TestFillCheckbox_SetsAS(t *testing.T) {
	src := buildCheckboxPDF(t)

	filled, err := FillFormFields(src, types.FormData{"agree": true}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}

	raw := string(filled)
	// The filled PDF must contain /AS /Yes for the checked state.
	if !strings.Contains(raw, "/AS /Yes") {
		t.Error("filled PDF does not contain /AS /Yes for checked checkbox")
	}
	// Must contain /V /Yes (the semantic value).
	if !strings.Contains(raw, "/V/Yes") && !strings.Contains(raw, "/V /Yes") {
		t.Error("filled PDF does not contain /V /Yes")
	}
}

func TestFillCheckbox_UncheckedSetsASOff(t *testing.T) {
	src := buildCheckboxPDF(t)

	filled, err := FillFormFields(src, types.FormData{"agree": false}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}

	raw := string(filled)
	// /AS /Off must appear in the incremental update section (after original).
	// Find the "agree" field section in the incremental update.
	if !strings.Contains(raw, "/AS /Off") {
		t.Error("filled PDF does not contain /AS /Off for unchecked checkbox")
	}
}

func TestFillCheckbox_GeneratesAPWhenMissing(t *testing.T) {
	// Build a raw PDF with a checkbox field that has NO /AP.
	// We craft a minimal PDF with a Btn field without appearance streams.
	src := buildCheckboxPDF(t)

	// Strip /AP from the source so the fill path generates it.
	stripped := bytes.ReplaceAll(src, []byte("/AP"), []byte("/XP"))

	filled, err := FillFormFields(stripped, types.FormData{"agree": true}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields on AP-stripped PDF: %v", err)
	}
	// The incremental update should have added /AP with new XObject refs.
	if !bytes.Contains(filled, []byte("/AP")) {
		t.Error("fill did not generate /AP for checkbox without appearance streams")
	}
}

func TestFillCheckbox_PreservesExistingAP(t *testing.T) {
	src := buildCheckboxPDF(t)

	filled, err := FillFormFields(src, types.FormData{"agree": true}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	// /AP must still appear (either original or updated).
	if !bytes.Contains(filled, []byte("/AP")) {
		t.Error("filled PDF lost /AP dict")
	}
}

func TestFillCheckbox_ReparseableAfterFill(t *testing.T) {
	src := buildCheckboxPDF(t)

	filled, err := FillFormFields(src, types.FormData{"agree": true}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	form, err := ExtractAcroForm(filled, nil, false)
	if err != nil {
		t.Fatalf("ExtractAcroForm on filled PDF: %v", err)
	}
	found := false
	for _, f := range form.Fields {
		if f.T == "agree" {
			found = true
			if f.V != "Yes" {
				t.Errorf("agree field value after fill: got %q, want \"Yes\"", f.V)
			}
		}
	}
	if !found {
		t.Error("agree field not found after fill")
	}
}
