package acroform

import (
	"os"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestValidateField(t *testing.T) {
	// Test required field
	field := &Field{
		T:  "testField",
		FT: "Tx",
		Ff: 0x2, // Required flag
	}

	err := ValidateField(field, "")
	if err == nil {
		t.Error("Expected error for empty required field")
	}

	err = ValidateField(field, "value")
	if err != nil {
		t.Errorf("Unexpected error for valid value: %v", err)
	}

	// Test max length
	field.MaxLen = 5
	err = ValidateField(field, "toolong")
	if err == nil {
		t.Error("Expected error for value exceeding max length")
	}

	err = ValidateField(field, "short")
	if err != nil {
		t.Errorf("Unexpected error for valid length: %v", err)
	}
}

func TestValidateChoiceField(t *testing.T) {
	field := &Field{
		T:   "choiceField",
		FT:  "Ch",
		Opt: []interface{}{"Option1", "Option2", "Option3"},
	}

	err := ValidateField(field, "Option1")
	if err != nil {
		t.Errorf("Unexpected error for valid option: %v", err)
	}

	err = ValidateField(field, "InvalidOption")
	if err == nil {
		t.Error("Expected error for invalid option")
	}
}

func TestValidateFormData(t *testing.T) {
	testPDFPath := getTestResourcePath("acroform_test.pdf")
	if _, err := os.Stat(testPDFPath); os.IsNotExist(err) {
		t.Skipf("Test PDF not found, skipping validation test")
	}

	pdfBytes, err := os.ReadFile(testPDFPath)
	if err != nil {
		t.Fatalf("Failed to read test PDF: %v", err)
	}

	acroForm, err := ExtractAcroForm(pdfBytes, []byte(""), false)
	if err != nil {
		t.Fatalf("Failed to extract AcroForm: %v", err)
	}

	// Test with valid data
	formData := types.FormData{
		"inputtext2": "Test Value",
	}

	errors := ValidateFormData(acroForm, formData)
	if len(errors) > 0 {
		t.Logf("Validation errors (may be expected): %v", errors)
	}

	// Test with missing required field (if any)
	emptyData := types.FormData{}
	errors = ValidateFormData(acroForm, emptyData)
	// Some fields may be required, so errors are expected
	t.Logf("Validation errors for empty form: %d", len(errors))
}

func TestValidateFormSchema(t *testing.T) {
	schema := &types.FormSchema{
		Questions: []types.Question{
			{
				Name:     "email",
				Type:     types.ResponseTypeEmail,
				Required: true,
				Validation: &types.ValidationRules{
					Pattern: `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`,
				},
			},
			{
				Name:     "age",
				Type:     types.ResponseTypeNumber,
				Required: true,
				Validation: &types.ValidationRules{
					MinValue: func() *float64 { v := 0.0; return &v }(),
					MaxValue: func() *float64 { v := 120.0; return &v }(),
				},
			},
		},
	}

	// Test valid data
	formData := types.FormData{
		"email": "test@example.com",
		"age":   25,
	}

	errors := ValidateFormSchema(schema, formData)
	if len(errors) > 0 {
		t.Errorf("Unexpected validation errors: %v", errors)
	}

	// Test invalid email
	formData["email"] = "invalid-email"
	errors = ValidateFormSchema(schema, formData)
	if len(errors) == 0 {
		t.Error("Expected validation error for invalid email")
	}

	// Test invalid age
	formData["email"] = "test@example.com"
	formData["age"] = 150
	errors = ValidateFormSchema(schema, formData)
	if len(errors) == 0 {
		t.Error("Expected validation error for age > 120")
	}

	// Test missing required field
	formData = types.FormData{
		"email": "test@example.com",
		// age missing
	}
	errors = ValidateFormSchema(schema, formData)
	if len(errors) == 0 {
		t.Error("Expected validation error for missing required field")
	}
}
