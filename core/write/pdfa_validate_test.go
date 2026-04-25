package write

import (
	"testing"
	"time"
)

// buildTestPDFA2b returns a minimal PDF/A-2b conformant PDF (no fonts = no
// embedded-font violations).
func buildTestPDFA2b(t *testing.T) []byte {
	t.Helper()
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	b.FinalizePage(p)
	b.SetPDFAMode(PDFAOptions{
		Conformance:  PDFA2b,
		Title:        "Test Document",
		CreationDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	return data
}

func TestValidatePDFA_ValidPDFA2b(t *testing.T) {
	data := buildTestPDFA2b(t)
	result := ValidatePDFA(data)
	if !result.Conformant {
		t.Errorf("expected conformant, got violations: %v", result.Violations)
	}
	if result.Part != 2 {
		t.Errorf("Part = %d, want 2", result.Part)
	}
	if result.ConformanceLevel != "B" {
		t.Errorf("ConformanceLevel = %q, want B", result.ConformanceLevel)
	}
}

func TestValidatePDFA_ValidPDFA1b(t *testing.T) {
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	b.FinalizePage(p)
	b.SetPDFAMode(PDFAOptions{Conformance: PDFA1b})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if !result.Conformant {
		t.Errorf("expected conformant, got violations: %v", result.Violations)
	}
	if result.Part != 1 {
		t.Errorf("Part = %d, want 1", result.Part)
	}
}

func TestValidatePDFA_ValidPDFA3b(t *testing.T) {
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	b.FinalizePage(p)
	b.SetPDFAMode(PDFAOptions{Conformance: PDFA3b})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if !result.Conformant {
		t.Errorf("expected conformant, got violations: %v", result.Violations)
	}
	if result.Part != 3 {
		t.Errorf("Part = %d, want 3", result.Part)
	}
}

func TestValidatePDFA_PlainPDFMissingXMP(t *testing.T) {
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	b.FinalizePage(p)
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if result.Conformant {
		t.Error("plain PDF should not be conformant")
	}
	if !hasViolation(result.Violations, ViolMissingXMP) && !hasViolation(result.Violations, ViolMissingPDFAID) {
		t.Errorf("expected XMP or PDFAID violation, got: %v", result.Violations)
	}
	if !hasViolation(result.Violations, ViolMissingOutputIntent) {
		t.Errorf("expected OutputIntent violation, got: %v", result.Violations)
	}
}

func TestValidatePDFA_EncryptedPDF(t *testing.T) {
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	b.FinalizePage(p)
	_, err := b.Writer().SetupEncryptionWithPasswords([]byte("pw"), []byte("owner"), -3904, true)
	if err != nil {
		t.Fatalf("SetupEncryptionWithPasswords: %v", err)
	}
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if result.Conformant {
		t.Error("encrypted PDF should not be conformant")
	}
	if !hasViolation(result.Violations, ViolEncrypted) {
		t.Errorf("expected ENCRYPTED violation, got: %v", result.Violations)
	}
}

func TestValidatePDFA_UnembeddedStdFont(t *testing.T) {
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	fn := p.AddStandardFont("Helvetica")
	p.Content().BeginText().SetFont(fn, 12).ShowText("hi").EndText()
	b.FinalizePage(p)
	b.SetPDFAMode(PDFAOptions{Conformance: PDFA2b})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if !hasViolation(result.Violations, ViolUnembeddedFont) {
		t.Errorf("expected UNEMBEDDED_FONT violation for Helvetica, got: %v", result.Violations)
	}
}

func TestValidatePDFA_NoViolations_NoPanic(t *testing.T) {
	result := ValidatePDFA([]byte("%PDF-1.7\n"))
	// Minimal/malformed PDF should return violations but not panic
	if result.Conformant {
		t.Error("truncated PDF should not be conformant")
	}
}

func TestValidatePDFA_ConformantExcludesUnembeddedFont(t *testing.T) {
	// A PDF/A-2b file without any fonts should be conformant.
	b := NewSimplePDFBuilder()
	p := b.AddPage(PageSizeLetter)
	// No fonts, no text — just an empty page.
	b.FinalizePage(p)
	b.SetPDFAMode(PDFAOptions{Conformance: PDFA2b})
	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	result := ValidatePDFA(data)
	if !result.Conformant {
		t.Errorf("empty-page PDF/A-2b should be conformant, got violations: %v", result.Violations)
	}
}

func TestValidatePDFA_ViolationCodes(t *testing.T) {
	// Verify violation code constants are stable strings (not iota).
	codes := map[PDFAViolationCode]string{
		ViolEncrypted:           "ENCRYPTED",
		ViolMissingDocumentID:   "MISSING_DOCUMENT_ID",
		ViolMissingXMP:          "MISSING_XMP",
		ViolMissingPDFAID:       "MISSING_PDFAID",
		ViolMissingOutputIntent: "MISSING_OUTPUT_INTENT",
		ViolMissingDestProfile:  "MISSING_DEST_PROFILE",
		ViolUnembeddedFont:      "UNEMBEDDED_FONT",
		ViolForbiddenAction:     "FORBIDDEN_ACTION",
	}
	for code, want := range codes {
		if string(code) != want {
			t.Errorf("code %q != %q", code, want)
		}
	}
}

// hasViolation reports whether viols contains a violation with the given code.
func hasViolation(viols []PDFAViolation, code PDFAViolationCode) bool {
	for _, v := range viols {
		if v.Code == code {
			return true
		}
	}
	return false
}
