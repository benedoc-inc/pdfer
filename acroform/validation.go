// Package acroform provides AcroForm field validation
package acroform

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/benedoc-inc/pdfer/types"
)

// ValidationError represents a validation error for a field
type ValidationError struct {
	FieldName string
	Message   string
	Value     interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s (value: %v)", e.FieldName, e.Message, e.Value)
}

// ValidateField validates a single field value against its constraints
func ValidateField(field *Field, value interface{}) error {
	if field == nil {
		return fmt.Errorf("field is nil")
	}

	// Check required
	if (field.Ff & 0x2) != 0 { // Required flag
		if value == nil || value == "" {
			return &ValidationError{
				FieldName: field.GetFullName(),
				Message:   "field is required",
				Value:     value,
			}
		}
	}

	// Check read-only
	if (field.Ff & 0x1) != 0 { // ReadOnly flag
		// Read-only fields can't be changed, but we'll allow it for now
		// (some PDFs allow programmatic changes)
	}

	// Type-specific validation
	switch field.FT {
	case "Tx": // Text field
		return validateTextField(field, value)
	case "Btn": // Button/Checkbox/Radio
		return validateButtonField(field, value)
	case "Ch": // Choice field
		return validateChoiceField(field, value)
	case "Sig": // Signature field
		return validateSignatureField(field, value)
	}

	return nil
}

// validateTextField validates a text field
func validateTextField(field *Field, value interface{}) error {
	valueStr, ok := value.(string)
	if !ok {
		valueStr = fmt.Sprintf("%v", value)
	}

	// Check max length
	if field.MaxLen > 0 && len(valueStr) > field.MaxLen {
		return &ValidationError{
			FieldName: field.GetFullName(),
			Message:   fmt.Sprintf("value exceeds maximum length of %d", field.MaxLen),
			Value:     valueStr,
		}
	}

	// Check if field has format validation (would be in /Ff flags or /F)
	// Format flag 0x1000 = Multiline
	// Format flag 0x2000 = Password
	// Format flag 0x100000 = FileSelect
	// Format flag 0x200000 = DoNotSpellCheck
	// Format flag 0x400000 = DoNotScroll

	return nil
}

// validateButtonField validates a button field (checkbox/radio/button)
func validateButtonField(field *Field, value interface{}) error {
	// Checkbox validation
	if (field.Ff & 0x8000) != 0 { // Checkbox flag
		// Checkbox values are typically "Yes", "On", "Off", or appearance state names
		valueStr := fmt.Sprintf("%v", value)
		if valueStr != "Yes" && valueStr != "On" && valueStr != "Off" && valueStr != "No" {
			// Allow custom appearance state names
			return nil
		}
	}

	// Radio button validation
	if (field.Ff & 0x10000) != 0 { // Radio button flag
		// Radio buttons should match one of the option values
		if len(field.Opt) > 0 {
			valueStr := fmt.Sprintf("%v", value)
			found := false
			for _, opt := range field.Opt {
				if fmt.Sprintf("%v", opt) == valueStr {
					found = true
					break
				}
			}
			if !found {
				return &ValidationError{
					FieldName: field.GetFullName(),
					Message:   "value does not match any radio button option",
					Value:     value,
				}
			}
		}
	}

	return nil
}

// validateChoiceField validates a choice field (dropdown/list)
func validateChoiceField(field *Field, value interface{}) error {
	if len(field.Opt) == 0 {
		return nil // No options to validate against
	}

	valueStr := fmt.Sprintf("%v", value)

	// Check if value is in options
	found := false
	for _, opt := range field.Opt {
		optStr := fmt.Sprintf("%v", opt)
		if optStr == valueStr {
			found = true
			break
		}
	}

	if !found {
		return &ValidationError{
			FieldName: field.GetFullName(),
			Message:   "value is not in the list of valid options",
			Value:     value,
		}
	}

	return nil
}

// validateSignatureField validates a signature field
func validateSignatureField(field *Field, value interface{}) error {
	// Signature fields typically contain signature dictionaries or certificate data
	// For now, just check it's not empty if required
	if value == nil || value == "" {
		if (field.Ff & 0x2) != 0 { // Required
			return &ValidationError{
				FieldName: field.GetFullName(),
				Message:   "signature field is required",
				Value:     value,
			}
		}
	}

	return nil
}

// ValidateFormData validates all fields in form data against the AcroForm
func ValidateFormData(acroForm *AcroForm, formData types.FormData) []error {
	var errors []error

	for fieldName, value := range formData {
		field := acroForm.FindFieldByName(fieldName)
		if field == nil {
			errors = append(errors, fmt.Errorf("field '%s' not found in form", fieldName))
			continue
		}

		if err := ValidateField(field, value); err != nil {
			errors = append(errors, err)
		}
	}

	// Check for missing required fields
	for _, field := range acroForm.Fields {
		if (field.Ff & 0x2) != 0 { // Required flag
			fieldName := field.GetFullName()
			if _, found := formData[fieldName]; !found {
				errors = append(errors, &ValidationError{
					FieldName: fieldName,
					Message:   "required field is missing",
					Value:     nil,
				})
			}
		}

		// Check child fields too
		for _, kid := range field.Kids {
			if (kid.Ff & 0x2) != 0 {
				kidName := kid.GetFullName()
				if _, found := formData[kidName]; !found {
					errors = append(errors, &ValidationError{
						FieldName: kidName,
						Message:   "required field is missing",
						Value:     nil,
					})
				}
			}
		}
	}

	return errors
}

// ValidateFieldValue validates a field value using FormSchema validation rules
func ValidateFieldValue(question *types.Question, value interface{}) error {
	if question.Validation == nil {
		return nil
	}

	valueStr := ""
	switch v := value.(type) {
	case string:
		valueStr = v
	case int:
		valueStr = strconv.Itoa(v)
	case float64:
		valueStr = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		valueStr = fmt.Sprintf("%v", v)
	}

	// Check min length
	if question.Validation.MinLength != nil && len(valueStr) < *question.Validation.MinLength {
		return &ValidationError{
			FieldName: question.Name,
			Message:   fmt.Sprintf("value is too short (minimum %d characters)", *question.Validation.MinLength),
			Value:     value,
		}
	}

	// Check max length
	if question.Validation.MaxLength != nil && len(valueStr) > *question.Validation.MaxLength {
		return &ValidationError{
			FieldName: question.Name,
			Message:   fmt.Sprintf("value is too long (maximum %d characters)", *question.Validation.MaxLength),
			Value:     value,
		}
	}

	// Check pattern
	if question.Validation.Pattern != "" {
		matched, err := regexp.MatchString(question.Validation.Pattern, valueStr)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
		if !matched {
			msg := question.Validation.ErrorMessage
			if msg == "" {
				msg = fmt.Sprintf("value does not match required pattern: %s", question.Validation.Pattern)
			}
			return &ValidationError{
				FieldName: question.Name,
				Message:   msg,
				Value:     value,
			}
		}
	}

	// Check numeric ranges
	if question.Type == types.ResponseTypeNumber {
		numValue, err := strconv.ParseFloat(valueStr, 64)
		if err == nil {
			if question.Validation.MinValue != nil && numValue < *question.Validation.MinValue {
				return &ValidationError{
					FieldName: question.Name,
					Message:   fmt.Sprintf("value is below minimum of %.2f", *question.Validation.MinValue),
					Value:     value,
				}
			}
			if question.Validation.MaxValue != nil && numValue > *question.Validation.MaxValue {
				return &ValidationError{
					FieldName: question.Name,
					Message:   fmt.Sprintf("value is above maximum of %.2f", *question.Validation.MaxValue),
					Value:     value,
				}
			}
		}
	}

	return nil
}

// ValidateFormSchema validates form data against a FormSchema
func ValidateFormSchema(schema *types.FormSchema, formData types.FormData) []error {
	var errors []error

	// Build a map of questions by name
	questionsByName := make(map[string]*types.Question)
	for i := range schema.Questions {
		q := &schema.Questions[i]
		questionsByName[q.Name] = q
	}

	// Validate provided values
	for fieldName, value := range formData {
		question, found := questionsByName[fieldName]
		if !found {
			errors = append(errors, fmt.Errorf("field '%s' not found in schema", fieldName))
			continue
		}

		if err := ValidateFieldValue(question, value); err != nil {
			errors = append(errors, err)
		}
	}

	// Check for missing required fields
	for _, question := range schema.Questions {
		if question.Required {
			if _, found := formData[question.Name]; !found {
				errors = append(errors, &ValidationError{
					FieldName: question.Name,
					Message:   "required field is missing",
					Value:     nil,
				})
			}
		}
	}

	return errors
}
