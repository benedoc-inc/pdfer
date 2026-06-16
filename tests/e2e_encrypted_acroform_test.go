package tests

import (
	"errors"
	"fmt"
	"testing"

	pdfer "github.com/benedoc-inc/pdfer/v2"
	"github.com/benedoc-inc/pdfer/v2/core/write"
	"github.com/benedoc-inc/pdfer/v2/forms/acroform"
	pdftypes "github.com/benedoc-inc/pdfer/v2/types"
)

// buildTinyAcroForm builds a minimal 3-field AcroForm PDF in-code. The bug under
// test (issue #26) is an auth/control-flow failure independent of document size,
// so a tiny synthetic fixture reproduces it just as well as a full-scale PDF but
// encrypts in well under a millisecond instead of tens of seconds.
func buildTinyAcroForm(t *testing.T) []byte {
	t.Helper()
	builder := write.NewSimplePDFBuilder()
	page := builder.AddPage(write.PageSizeLetter)

	fb := acroform.NewFieldBuilder(builder.Writer())
	fb.AddTextField("name", []float64{72, 700, 300, 720}, 0)
	fb.AddCheckbox("agree", []float64{72, 650, 90, 670}, 0)
	fb.AddChoiceField("country", []float64{72, 600, 200, 620}, 0, []string{"USA", "Canada"})

	builder.FinalizePage(page)

	acroFormNum, err := fb.Build()
	if err != nil {
		t.Fatalf("build AcroForm: %v", err)
	}
	w := builder.Writer()
	catalog := fmt.Sprintf("<</Type/Catalog/Pages %d 0 R/AcroForm %d 0 R>>",
		builder.PagesObjNum(), acroFormNum)
	w.SetRoot(w.AddObject([]byte(catalog)))

	pdfBytes, err := builder.Bytes()
	if err != nil {
		t.Fatalf("serialize PDF: %v", err)
	}
	return pdfBytes
}

// TestE2E_EncryptedAcroForm exercises ExtractForm against an encrypted AcroForm
// (issue #26). Encrypting once with a non-empty user password is what reproduces
// the old empty-password re-open failure in ParseAcroForm; both subtests share
// the encrypted bytes.
func TestE2E_EncryptedAcroForm(t *testing.T) {
	enc, err := pdfer.EncryptPDF(buildTinyAcroForm(t), []byte("user-pw"), []byte("owner-pw"), false)
	if err != nil {
		t.Fatalf("EncryptPDF: %v", err)
	}

	// Path A: parsing from the encrypted bytes with the already-derived,
	// password-verified encryptInfo resolves every object. The old code's
	// empty-password re-open failed here before any object parse ran.
	t.Run("ExtractWithPassword", func(t *testing.T) {
		form, err := pdfer.ExtractForm(enc, []byte("user-pw"), false)
		if err != nil {
			t.Fatalf("ExtractForm(encrypted, user-pw): %v", err)
		}
		if form.Type() != pdfer.FormTypeAcroForm {
			t.Errorf("Type() = %q, want %q", form.Type(), pdfer.FormTypeAcroForm)
		}
		if got := len(form.Schema().Questions); got != 3 {
			t.Errorf("Schema().Questions = %d, want 3", got)
		}
	})

	// A wrong password must surface an encryption error rather than the
	// misleading NO_FORMS the old error-swallowing produced.
	t.Run("WrongPasswordPropagates", func(t *testing.T) {
		_, err := pdfer.ExtractForm(enc, []byte("bad-pw"), false)
		if err == nil {
			t.Fatal("ExtractForm(encrypted, bad-pw): expected error, got nil")
		}
		var pe *pdftypes.PDFError
		if !errors.As(err, &pe) || !pdftypes.IsEncryptionError(pe) {
			t.Fatalf("expected an encryption error, got %v", err)
		}
		if pe.Code == pdftypes.ErrCodeNoForms {
			t.Fatalf("got misleading NO_FORMS instead of an auth error: %v", err)
		}
	})
}
