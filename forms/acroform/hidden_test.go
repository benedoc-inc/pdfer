package acroform_test

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/forms/acroform"
)

// buildSingleFieldForm creates a one-page PDF with exactly one text field, so
// the generated widget contains exactly one "/F 4" annotation-flag token.
func buildSingleFieldForm(t *testing.T) []byte {
	t.Helper()

	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)
	builder.FinalizePage(page)

	fb := acroform.NewFormBuilder(builder)
	fb.AddTextField("solo", []float64{72, 700, 300, 720}, 0)
	if _, err := fb.BuildForm(); err != nil {
		t.Fatalf("BuildForm failed: %v", err)
	}
	b, err := builder.Bytes()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}
	return b
}

// TestHiddenWidgetFlagParsed exercises the real /F annotation-flag parse path.
// The writer emits "/F 4" (Print) on every widget, so a freshly built form is
// visible. Flipping that one token to "/F 6" (Print|Hidden) — same byte length,
// so xref offsets stay valid — must surface as Question.Hidden=true. This also
// guards the regex against colliding with the /Ff and /FT keys in the same dict.
func TestHiddenWidgetFlagParsed(t *testing.T) {
	pdf := buildSingleFieldForm(t)

	form, err := acroform.ExtractAcroForm(pdf, nil, false)
	if err != nil {
		t.Fatalf("ExtractAcroForm (visible) failed: %v", err)
	}
	if len(form.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(form.Fields))
	}
	if q := form.Fields[0].ToQuestion(); q.Hidden {
		t.Errorf("widget with /F 4 (Print) should not be Hidden, got Hidden=true")
	}

	hiddenPDF := bytes.Replace(pdf, []byte("/F 4"), []byte("/F 6"), 1)
	if bytes.Equal(hiddenPDF, pdf) {
		t.Fatal("expected to find a /F 4 token to flip; writer output changed?")
	}

	form, err = acroform.ExtractAcroForm(hiddenPDF, nil, false)
	if err != nil {
		t.Fatalf("ExtractAcroForm (hidden) failed: %v", err)
	}
	if len(form.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(form.Fields))
	}
	if q := form.Fields[0].ToQuestion(); !q.Hidden {
		t.Errorf("widget with /F 6 (Print|Hidden) should be Hidden, got Hidden=false")
	}
}

// TestFieldHiddenKidsPolicy covers the hidden-flag semantics on the Field model
// directly, including the radio/kids rule: the Hidden flag lives on each kid
// widget, so a field counts as hidden only when every kid widget is hidden.
func TestFieldHiddenKidsPolicy(t *testing.T) {
	const (
		flagHidden = 0x2
		flagPrint  = 0x4
	)
	tests := []struct {
		name  string
		field *acroform.Field
		want  bool
	}{
		{"merged hidden widget", &acroform.Field{FT: "Tx", F: flagHidden}, true},
		{"merged visible widget", &acroform.Field{FT: "Tx", F: flagPrint}, false},
		{"no flags", &acroform.Field{FT: "Tx"}, false},
		{
			name:  "radio all kids hidden",
			field: &acroform.Field{FT: "Btn", Kids: []*acroform.Field{{F: flagHidden}, {F: flagHidden}}},
			want:  true,
		},
		{
			name:  "radio one kid visible",
			field: &acroform.Field{FT: "Btn", Kids: []*acroform.Field{{F: flagHidden}, {F: flagPrint}}},
			want:  false,
		},
		{
			name:  "parent hidden overrides visible kids",
			field: &acroform.Field{FT: "Btn", F: flagHidden, Kids: []*acroform.Field{{F: flagPrint}}},
			want:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.field.ToQuestion().Hidden; got != tt.want {
				t.Errorf("Hidden = %v, want %v", got, tt.want)
			}
		})
	}
}
