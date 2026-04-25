package acroform

import (
	"os"
	"path/filepath"
	"testing"
)

func getTestResourcePath(filename string) string {
	possiblePaths := []string{
		filepath.Join("tests", "resources", filename),
		filepath.Join("resources", filename),
		filepath.Join("..", "tests", "resources", filename),
		filepath.Join(".", "tests", "resources", filename),
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return filepath.Join("tests", "resources", filename)
}

func TestExtractAcroForm(t *testing.T) {
	testPDFPath := getTestResourcePath("acroform_test.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s, skipping AcroForm test", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	acroForm, err := ExtractAcroForm(pdfBytes, []byte(""), false)
	if err != nil {
		t.Fatalf("Failed to extract AcroForm: %v", err)
	}

	if acroForm == nil {
		t.Fatal("ExtractAcroForm returned nil")
	}

	t.Logf("Found AcroForm with %d top-level fields", len(acroForm.Fields))

	// Check if we found any fields
	if len(acroForm.Fields) == 0 {
		t.Log("Warning: No fields found in AcroForm (may be XFA-only form)")
	} else {
		for i, field := range acroForm.Fields {
			t.Logf("Field %d: %s (type: %s, name: %s)", i+1, field.T, field.FT, field.GetFullName())
		}
	}
}

func TestExtractFormSchema(t *testing.T) {
	testPDFPath := getTestResourcePath("acroform_test.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s, skipping FormSchema test", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	schema, err := ExtractFormSchema(pdfBytes, []byte(""), false)
	if err != nil {
		t.Fatalf("Failed to extract FormSchema: %v", err)
	}

	if schema == nil {
		t.Fatal("ExtractFormSchema returned nil")
	}

	if schema.Metadata.FormType != "AcroForm" {
		t.Errorf("Expected FormType 'AcroForm', got '%s'", schema.Metadata.FormType)
	}

	t.Logf("Extracted FormSchema with %d questions", len(schema.Questions))

	for i, q := range schema.Questions {
		t.Logf("Question %d: %s (%s) - %s", i+1, q.Name, q.Type, q.Label)
	}
}

func TestGetFieldValues(t *testing.T) {
	testPDFPath := getTestResourcePath("acroform_test.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found at %s, skipping field values test", testPDFPath)
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	acroForm, err := ExtractAcroForm(pdfBytes, []byte(""), false)
	if err != nil {
		t.Fatalf("Failed to extract AcroForm: %v", err)
	}

	values := acroForm.GetFieldValues()
	t.Logf("Found %d field values", len(values))

	for name, value := range values {
		t.Logf("  %s = %v", name, value)
	}
}

func TestExtractBracketedDict(t *testing.T) {
	cases := []struct {
		input string
		start int
		want  string
	}{
		{"<< /Fields [] >>", 0, "<< /Fields [] >>"},
		{"<< /A << /B 1 >> >>", 0, "<< /A << /B 1 >> >>"},
		{"prefix << /X 1 >> suffix", 7, "<< /X 1 >>"},
		{"<< /S (has (parens) inside) >>", 0, "<< /S (has (parens) inside) >>"},
	}
	for _, tc := range cases {
		got := extractBracketedDict(tc.input, tc.start)
		if string(got) != tc.want {
			t.Errorf("extractBracketedDict(%q, %d) = %q, want %q", tc.input, tc.start, got, tc.want)
		}
	}
}

func TestParseAcroForm_InlineDict(t *testing.T) {
	// PDF bytes where /AcroForm is an inline dict rather than an indirect reference.
	// parseAcroFormFromBytes searches the raw bytes, so we only need the relevant content.
	pdf := []byte("/Type /Catalog /AcroForm << /Fields [] /NeedAppearances true >>")

	acroForm, err := parseAcroFormFromBytes(pdf, nil, false)
	if err != nil {
		t.Fatalf("parseAcroFormFromBytes with inline dict: %v", err)
	}
	if acroForm == nil {
		t.Fatal("expected non-nil AcroForm")
	}
	if len(acroForm.Fields) != 0 {
		t.Errorf("expected 0 fields for empty inline dict, got %d", len(acroForm.Fields))
	}
}

func TestParseAcroForm_InlineDictNestedResources(t *testing.T) {
	// Inline AcroForm with nested /DR dict (typical real-world shape)
	pdf := []byte("/Type /Catalog /AcroForm << /Fields [] /DR << /Font << /Helv 12 0 R >> >> >>")

	acroForm, err := parseAcroFormFromBytes(pdf, nil, false)
	if err != nil {
		t.Fatalf("parseAcroFormFromBytes with nested inline dict: %v", err)
	}
	if acroForm == nil {
		t.Fatal("expected non-nil AcroForm")
	}
}
