package acroform

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/parse"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

// buildFilledTextFieldPDF returns a one-page PDF with a single text field
// "name" pre-filled with "Alice".
func buildFilledTextFieldPDF(t *testing.T) []byte {
	t.Helper()
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	b.FinalizePage(page)

	fb := NewFormBuilder(b)
	fb.AddTextField("name", []float64{72, 680, 300, 700}, 0)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm: %v", err)
	}
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	filled, err := FillFormFields(src, types.FormData{"name": "Alice"}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	return filled
}

// --- helper unit tests -------------------------------------------------------

func TestFlattenGetAPNObjNum_DirectRef(t *testing.T) {
	widget := "<</Type/Annot/Subtype/Widget/AP<</N 7 0 R>>>>"
	if got := flattenGetAPNObjNum(widget); got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

func TestFlattenGetAPNObjNum_SubDictChecked(t *testing.T) {
	widget := "<</Type/Annot/Subtype/Widget/AS /Yes/AP<</N<</Yes 8 0 R/Off 9 0 R>>>>>>"
	if got := flattenGetAPNObjNum(widget); got != 8 {
		t.Errorf("expected 8 (Yes state), got %d", got)
	}
}

func TestFlattenGetAPNObjNum_SubDictUnchecked(t *testing.T) {
	widget := "<</Type/Annot/Subtype/Widget/AS /Off/AP<</N<</Yes 8 0 R/Off 9 0 R>>>>>>"
	if got := flattenGetAPNObjNum(widget); got != 0 {
		t.Errorf("expected 0 (Off = nothing to draw), got %d", got)
	}
}

func TestFlattenGetAPNObjNum_NoAP(t *testing.T) {
	widget := "<</Type/Annot/Subtype/Widget/FT/Tx/T(empty)>>"
	if got := flattenGetAPNObjNum(widget); got != 0 {
		t.Errorf("expected 0 for widget with no AP, got %d", got)
	}
}

func TestFlattenParseRect(t *testing.T) {
	widget := "<</Rect [72 680 300 700]>>"
	rect := flattenParseRect(widget)
	if rect[0] != 72 || rect[1] != 680 || rect[2] != 300 || rect[3] != 700 {
		t.Errorf("unexpected rect: %v", rect)
	}
}

func TestFlattenParseBBox(t *testing.T) {
	xobj := "<</Type/XObject/Subtype/Form/BBox [0 0 228 20]/Length 10>>"
	bbox := flattenParseBBox(xobj)
	if bbox[0] != 0 || bbox[1] != 0 || bbox[2] != 228 || bbox[3] != 20 {
		t.Errorf("unexpected bbox: %v", bbox)
	}
}

func TestFlattenClearAnnots(t *testing.T) {
	pageDict := "<</Type/Page/Annots[5 0 R 6 0 R]/Contents 3 0 R>>"
	got := flattenClearAnnots(pageDict)
	if strings.Contains(got, "/Annots") {
		t.Errorf("flattenClearAnnots did not remove /Annots: %s", got)
	}
	if !strings.Contains(got, "/Contents") {
		t.Error("flattenClearAnnots removed /Contents unexpectedly")
	}
}

func TestFlattenAppendContents_SingleRef(t *testing.T) {
	pageDict := "<</Type/Page/Contents 4 0 R>>"
	got := flattenAppendContents(pageDict, 99)
	if !strings.Contains(got, "[4 0 R 99 0 R]") {
		t.Errorf("expected [4 0 R 99 0 R], got: %s", got)
	}
}

func TestFlattenAppendContents_ArrayRef(t *testing.T) {
	pageDict := "<</Type/Page/Contents [4 0 R 5 0 R]>>"
	got := flattenAppendContents(pageDict, 99)
	if !strings.Contains(got, "[4 0 R 5 0 R 99 0 R]") {
		t.Errorf("expected [4 0 R 5 0 R 99 0 R], got: %s", got)
	}
}

func TestFlattenAddXObjects_NoExistingXObject(t *testing.T) {
	pageDict := "<</Type/Page/Resources<</Font<</F1 3 0 R>>>>>>"
	got := flattenAddXObjects(pageDict, map[string]int{"FO7": 7})
	if !strings.Contains(got, "/XObject<<") {
		t.Errorf("expected /XObject<< to be added: %s", got)
	}
	if !strings.Contains(got, "/FO7 7 0 R") {
		t.Errorf("expected /FO7 7 0 R in XObject dict: %s", got)
	}
	if !strings.Contains(got, "/Font<<") {
		t.Error("flattenAddXObjects removed /Font unexpectedly")
	}
}

func TestFlattenAddXObjects_ExistingXObject(t *testing.T) {
	pageDict := "<</Type/Page/Resources<</XObject<</Im0 5 0 R>>/Font<</F1 3 0 R>>>>>>"
	got := flattenAddXObjects(pageDict, map[string]int{"FO7": 7})
	if !strings.Contains(got, "/FO7 7 0 R") {
		t.Errorf("expected /FO7 7 0 R: %s", got)
	}
	if !strings.Contains(got, "/Im0 5 0 R") {
		t.Error("existing XObject entry /Im0 was removed")
	}
}

func TestFlattenFmtF(t *testing.T) {
	cases := [][2]string{
		{"1.000000", "1"},
		{"0.500000", "0.5"},
		{"72.000000", "72"},
		{"-0.500000", "-0.5"},
	}
	for _, c := range cases {
		f, _ := strconv.ParseFloat(c[0], 64)
		if got := flattenFmtF(f); got != c[1] {
			t.Errorf("flattenFmtF(%s) = %q, want %q", c[0], got, c[1])
		}
	}
}

// --- integration tests -------------------------------------------------------

func TestFlattenForm_NoAcroForm(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	b.FinalizePage(b.AddPage(write.PageSizeLetter))
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if !bytes.Equal(out, src) {
		t.Error("PDF with no AcroForm should be returned unchanged")
	}
}

func TestFlattenForm_ParseableAfterFlatten(t *testing.T) {
	src := buildFilledTextFieldPDF(t)
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("flattened PDF not parseable: %v", err)
	}
}

func TestFlattenForm_NoAcroFormInOutput(t *testing.T) {
	src := buildFilledTextFieldPDF(t)
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if bytes.Contains(out, []byte("/AcroForm")) {
		t.Error("flattened PDF still contains /AcroForm")
	}
}

func TestFlattenForm_NoAnnotsInOutput(t *testing.T) {
	src := buildFilledTextFieldPDF(t)
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if bytes.Contains(out, []byte("/Annots")) {
		t.Error("flattened PDF still contains /Annots")
	}
}

func TestFlattenForm_OverlayOperatorsPresent(t *testing.T) {
	src := buildFilledTextFieldPDF(t)
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	// The flattened PDF must have a cm + Do sequence from the overlay stream.
	if !bytes.Contains(out, []byte(" cm\n")) {
		t.Error("flattened PDF missing cm operator in overlay")
	}
	if !bytes.Contains(out, []byte(" Do\n")) {
		t.Error("flattened PDF missing Do operator in overlay")
	}
}

func TestFlattenForm_SingleEOF(t *testing.T) {
	src := buildFilledTextFieldPDF(t)
	out, err := FlattenForm(src, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if strings.Count(string(out), "%%EOF") != 1 {
		t.Error("expected exactly one EOF marker in flattened output")
	}
}

func TestFlattenForm_CheckedCheckbox(t *testing.T) {
	src := buildCheckboxPDF(t) // defined in btn_fill_test.go
	filled, err := FillFormFields(src, types.FormData{"agree": true}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	out, err := FlattenForm(filled, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("flattened checked-checkbox PDF not parseable: %v", err)
	}
	if bytes.Contains(out, []byte("/AcroForm")) {
		t.Error("flattened PDF still contains /AcroForm")
	}
}

func TestFlattenForm_UncheckedCheckbox(t *testing.T) {
	src := buildCheckboxPDF(t)
	filled, err := FillFormFields(src, types.FormData{"agree": false}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	out, err := FlattenForm(filled, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("flattened unchecked-checkbox PDF not parseable: %v", err)
	}
	if bytes.Contains(out, []byte("/AcroForm")) {
		t.Error("flattened PDF still contains /AcroForm")
	}
	// Unchecked → no overlay drawn, so no Do operator expected.
	if bytes.Contains(out, []byte(" Do\n")) {
		t.Error("unchecked checkbox should produce no Do operator in flattened output")
	}
}

func TestFlattenForm_MultipleFields(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	page := b.AddPage(write.PageSizeLetter)
	b.FinalizePage(page)

	fb := NewFormBuilder(b)
	fb.AddTextField("first", []float64{72, 700, 250, 720}, 0)
	fb.AddTextField("last", []float64{72, 660, 250, 680}, 0)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm: %v", err)
	}
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	filled, err := FillFormFields(src, types.FormData{"first": "Jane", "last": "Doe"}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	out, err := FlattenForm(filled, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	if _, err := parse.Open(out); err != nil {
		t.Fatalf("flattened multi-field PDF not parseable: %v", err)
	}
	if bytes.Contains(out, []byte("/AcroForm")) {
		t.Error("flattened PDF still contains /AcroForm")
	}
	if bytes.Contains(out, []byte("/Annots")) {
		t.Error("flattened PDF still contains /Annots")
	}
}

func TestFlattenForm_TwoPageForm(t *testing.T) {
	b := write.NewSimplePDFBuilder()
	p1 := b.AddPage(write.PageSizeLetter)
	b.FinalizePage(p1)
	p2 := b.AddPage(write.PageSizeLetter)
	b.FinalizePage(p2)

	fb := NewFormBuilder(b)
	fb.AddTextField("p1field", []float64{72, 700, 250, 720}, 0)
	fb.AddTextField("p2field", []float64{72, 700, 250, 720}, 1)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm: %v", err)
	}
	src, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	filled, err := FillFormFields(src, types.FormData{"p1field": "one", "p2field": "two"}, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields: %v", err)
	}
	out, err := FlattenForm(filled, nil, false)
	if err != nil {
		t.Fatalf("FlattenForm: %v", err)
	}
	pdf, err := parse.Open(out)
	if err != nil {
		t.Fatalf("flattened two-page PDF not parseable: %v", err)
	}
	if len(pdf.Objects()) == 0 {
		t.Error("no objects in flattened two-page PDF")
	}
	// Both pages must remain (/Count 2).
	if !bytes.Contains(out, []byte("/Count 2")) {
		t.Error("page count changed after flattening two-page form")
	}
}
