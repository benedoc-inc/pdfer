package acroform

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestFieldBuilder(t *testing.T) {
	w := write.NewPDFWriter()
	fb := NewFieldBuilder(w)

	// Add fields
	textField := fb.AddTextField("name", []float64{72, 700, 300, 720}, 0)
	textField.SetDefault("Test").
		SetMaxLength(50).
		SetRequired(true)

	checkbox := fb.AddCheckbox("agree", []float64{72, 650, 90, 670}, 0)
	checkbox.SetDefault(false)

	dropdown := fb.AddChoiceField("country", []float64{72, 600, 200, 620}, 0,
		[]string{"USA", "Canada", "Mexico"})
	dropdown.SetDefault("USA")

	// Build
	acroFormNum, err := fb.Build()
	if err != nil {
		t.Fatalf("Failed to build AcroForm: %v", err)
	}

	if acroFormNum == 0 {
		t.Error("AcroForm object number should not be zero")
	}

	if len(fb.fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(fb.fields))
	}

	t.Logf("Created AcroForm object %d with %d fields", acroFormNum, len(fb.fields))
}

func TestFieldDef(t *testing.T) {
	fd := &FieldDef{
		Name: "test",
		Type: "Tx",
		Rect: []float64{0, 0, 100, 20},
	}

	// Test setters
	fd.SetValue("Hello").
		SetDefault("World").
		SetRequired(true).
		SetReadOnly(false).
		SetMaxLength(100)

	if fd.Value != "Hello" {
		t.Error("Value not set correctly")
	}

	if fd.DefaultValue != "World" {
		t.Error("Default value not set correctly")
	}

	if !fd.Required {
		t.Error("Required flag not set")
	}

	if fd.MaxLen != 100 {
		t.Error("MaxLen not set correctly")
	}
}
