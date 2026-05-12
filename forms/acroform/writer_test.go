package acroform

import (
	"strings"
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

func TestPushButton_HasAppearanceStream(t *testing.T) {
	w := write.NewPDFWriter()
	fb := NewFieldBuilder(w)
	fb.AddButton("btn_submit", []float64{72, 100, 200, 125}, 0)
	if _, err := fb.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Collect all objects from the writer via Bytes() and check for /AP.
	pdfBytes, err := func() ([]byte, error) {
		b := write.NewSimplePDFBuilder()
		page := b.AddPage(write.PageSizeLetter)
		fb2 := NewFieldBuilder(b.Writer())
		fb2.AddButton("submit", []float64{72, 100, 200, 125}, 0)
		b.FinalizePage(page)
		acroNum, err := fb2.Build()
		if err != nil {
			return nil, err
		}
		b.RegisterAcroForm(acroNum)
		return b.Bytes()
	}()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}

	raw := string(pdfBytes)
	if !strings.Contains(raw, "/AP") {
		t.Error("push button should have /AP entry")
	}
	if !strings.Contains(raw, "/MK") {
		t.Error("push button should have /MK entry for caption")
	}
}

func TestPasswordField_HasMaskedAP(t *testing.T) {
	pdfBytes, err := func() ([]byte, error) {
		b := write.NewSimplePDFBuilder()
		page := b.AddPage(write.PageSizeLetter)
		fb := NewFieldBuilder(b.Writer())
		pwField := fb.AddTextField("password", []float64{72, 680, 300, 700}, 0)
		pwField.SetPassword(true).SetValue("secret")
		b.FinalizePage(page)
		acroNum, err := fb.Build()
		if err != nil {
			return nil, err
		}
		b.RegisterAcroForm(acroNum)
		return b.Bytes()
	}()
	if err != nil {
		t.Fatalf("Bytes() failed: %v", err)
	}

	raw := string(pdfBytes)
	if !strings.Contains(raw, "/AP") {
		t.Error("password field should have /AP entry")
	}
	// Bullets are WinAnsi octal \225 — 6 chars per bullet × 6 chars in "secret"
	if !strings.Contains(raw, `\225`) {
		t.Error("password field AP should contain bullet chars (\\225)")
	}
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
