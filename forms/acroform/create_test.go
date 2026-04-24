package acroform_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/content/extract"
	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/acroform"
	"github.com/benedoc-inc/pdfer/types"
)

// buildTestForm creates a one-page PDF with a text field, a checkbox, and a
// dropdown, returning the raw PDF bytes.
func buildTestForm(t *testing.T) []byte {
	t.Helper()

	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	page.Content().
		BeginText().
		SetFont(page.AddStandardFont("Helvetica"), 12).
		SetTextPosition(72, 750).
		ShowText("Sample Form").
		EndText()
	builder.FinalizePage(page)

	fb := acroform.NewFormBuilder(builder)
	fb.AddTextField("fullname", []float64{72, 700, 300, 720}, 0).
		SetDefault("Enter name").
		SetMaxLength(100)
	fb.AddCheckbox("agree", []float64{72, 660, 90, 678}, 0)
	fb.AddChoiceField("country", []float64{72, 620, 250, 640}, 0,
		[]string{"USA", "Canada", "Mexico"}).
		SetValue("USA")

	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm failed: %v", err)
	}

	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}
	return b
}

func TestCreateForm_ExtractFindsFields(t *testing.T) {
	pdf := buildTestForm(t)

	form, err := acroform.ExtractAcroForm(pdf, nil, false)
	if err != nil {
		t.Fatalf("ExtractAcroForm failed: %v", err)
	}

	if len(form.Fields) != 3 {
		var names []string
		for _, f := range form.Fields {
			names = append(names, f.T)
		}
		t.Fatalf("expected 3 fields, got %d: %v", len(form.Fields), names)
	}

	byName := make(map[string]*acroform.Field)
	for _, f := range form.Fields {
		byName[f.T] = f
	}
	for _, want := range []string{"fullname", "agree", "country"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("field %q not found", want)
		}
	}
	if f := byName["fullname"]; f != nil && f.FT != "Tx" {
		t.Errorf("fullname type: want Tx, got %s", f.FT)
	}
	if f := byName["agree"]; f != nil && f.FT != "Btn" {
		t.Errorf("agree type: want Btn, got %s", f.FT)
	}
	if f := byName["country"]; f != nil && f.FT != "Ch" {
		t.Errorf("country type: want Ch, got %s", f.FT)
	}
}

func TestCreateForm_AnnotsWiredToPage(t *testing.T) {
	pdf := buildTestForm(t)

	doc, err := extract.ExtractContent(pdf, nil, false)
	if err != nil {
		t.Fatalf("ExtractContent failed: %v", err)
	}
	if len(doc.Pages) == 0 {
		t.Fatal("no pages extracted")
	}
	if len(doc.Pages[0].Annotations) != 3 {
		t.Errorf("expected 3 widget annotations on page 1, got %d",
			len(doc.Pages[0].Annotations))
	}
}

func TestCreateForm_FillRoundTrip(t *testing.T) {
	pdf := buildTestForm(t)

	data := types.FormData{
		"fullname": "Jane Smith",
	}
	filled, err := acroform.FillFormFields(pdf, data, nil, false)
	if err != nil {
		t.Fatalf("FillFormFields failed: %v", err)
	}
	if len(filled) == 0 {
		t.Fatal("FillFormFields returned empty bytes")
	}
	// The filled value should appear in the raw bytes as a PDF string literal.
	if !strings.Contains(string(filled), "Jane Smith") {
		t.Error("filled PDF does not contain 'Jane Smith'")
	}
}

func TestCreateForm_CatalogHasAcroForm(t *testing.T) {
	pdf := buildTestForm(t)
	if !strings.Contains(string(pdf), "/AcroForm") {
		t.Error("PDF bytes do not contain /AcroForm")
	}
}

func TestCreateForm_ChoiceFieldValue(t *testing.T) {
	pdf := buildTestForm(t)

	form, err := acroform.ExtractAcroForm(pdf, nil, false)
	if err != nil {
		t.Fatalf("ExtractAcroForm failed: %v", err)
	}
	for _, f := range form.Fields {
		if f.T == "country" {
			if !strings.Contains(fmt.Sprint(f.V), "USA") {
				t.Errorf("country value: want 'USA', got %q", f.V)
			}
			if len(f.Opt) != 3 {
				t.Errorf("country options: want 3, got %d", len(f.Opt))
			}
			return
		}
	}
	t.Error("country field not found")
}
