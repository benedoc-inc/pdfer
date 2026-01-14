package types

import (
	"errors"
	"fmt"
	"testing"
)

func TestPDFError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *PDFError
		expected string
	}{
		{
			name:     "simple error",
			err:      NewPDFError(ErrCodeInvalidPDF, "file is not a valid PDF"),
			expected: "[INVALID_PDF] file is not a valid PDF",
		},
		{
			name:     "error with cause",
			err:      WrapError(ErrCodeDecryptionFailed, "failed to decrypt", fmt.Errorf("wrong key")),
			expected: "[DECRYPTION_FAILED] failed to decrypt: wrong key",
		},
		{
			name:     "formatted error",
			err:      NewPDFErrorf(ErrCodeObjectNotFound, "object %d not found", 42),
			expected: "[OBJECT_NOT_FOUND] object 42 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPDFError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("underlying error")
	err := WrapError(ErrCodeStreamError, "stream decompression failed", cause)

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test with errors.Unwrap
	if unwrapped := errors.Unwrap(err); unwrapped != cause {
		t.Errorf("errors.Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestPDFError_Is(t *testing.T) {
	err := NewPDFError(ErrCodeWrongPassword, "incorrect password")

	// Should match sentinel
	if !errors.Is(err, ErrWrongPassword) {
		t.Error("errors.Is should match ErrWrongPassword sentinel")
	}

	// Should not match different sentinel
	if errors.Is(err, ErrInvalidPDF) {
		t.Error("errors.Is should not match ErrInvalidPDF sentinel")
	}

	// Wrapped error should still match
	wrapped := fmt.Errorf("outer: %w", err)
	if !errors.Is(wrapped, ErrWrongPassword) {
		t.Error("wrapped error should match ErrWrongPassword sentinel")
	}
}

func TestPDFError_WithContext(t *testing.T) {
	err := NewPDFError(ErrCodeObjectNotFound, "object not found").
		WithContext("objectNum", 42).
		WithContext("generation", 0)

	if err.Context["objectNum"] != 42 {
		t.Errorf("Context[objectNum] = %v, want 42", err.Context["objectNum"])
	}
	if err.Context["generation"] != 0 {
		t.Errorf("Context[generation] = %v, want 0", err.Context["generation"])
	}
}

func TestIsPDFError(t *testing.T) {
	pdfErr := NewPDFError(ErrCodeInvalidPDF, "invalid")
	stdErr := fmt.Errorf("standard error")

	if got, ok := IsPDFError(pdfErr); !ok || got.Code != ErrCodeInvalidPDF {
		t.Error("IsPDFError should return true for PDFError")
	}

	if _, ok := IsPDFError(stdErr); ok {
		t.Error("IsPDFError should return false for standard error")
	}
}

func TestGetErrorCode(t *testing.T) {
	pdfErr := NewPDFError(ErrCodeFieldNotFound, "field not found")

	code, ok := GetErrorCode(pdfErr)
	if !ok || code != ErrCodeFieldNotFound {
		t.Errorf("GetErrorCode() = %v, %v; want %v, true", code, ok, ErrCodeFieldNotFound)
	}

	_, ok = GetErrorCode(fmt.Errorf("standard error"))
	if ok {
		t.Error("GetErrorCode should return false for standard error")
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{NewPDFError(ErrCodeObjectNotFound, ""), true},
		{NewPDFError(ErrCodeFieldNotFound, ""), true},
		{NewPDFError(ErrCodeInvalidPDF, ""), false},
		{fmt.Errorf("standard error"), false},
	}

	for _, tt := range tests {
		if got := IsNotFound(tt.err); got != tt.expected {
			t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}

func TestIsEncryptionError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{NewPDFError(ErrCodeEncrypted, ""), true},
		{NewPDFError(ErrCodeDecryptionFailed, ""), true},
		{NewPDFError(ErrCodeWrongPassword, ""), true},
		{NewPDFError(ErrCodeUnsupportedCrypto, ""), true},
		{NewPDFError(ErrCodeInvalidPDF, ""), false},
		{fmt.Errorf("standard error"), false},
	}

	for _, tt := range tests {
		if got := IsEncryptionError(tt.err); got != tt.expected {
			t.Errorf("IsEncryptionError(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}

func TestIsValidationError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{NewPDFError(ErrCodeValidationError, ""), true},
		{NewPDFError(ErrCodeInvalidValue, ""), true},
		{NewPDFError(ErrCodeInvalidPDF, ""), false},
		{fmt.Errorf("standard error"), false},
	}

	for _, tt := range tests {
		if got := IsValidationError(tt.err); got != tt.expected {
			t.Errorf("IsValidationError(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}
