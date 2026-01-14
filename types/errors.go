package types

import (
	"fmt"
)

// PDFErrorCode represents categorized error codes for PDF operations
type PDFErrorCode string

const (
	// Parsing errors
	ErrCodeInvalidPDF     PDFErrorCode = "INVALID_PDF"
	ErrCodeMalformedPDF   PDFErrorCode = "MALFORMED_PDF"
	ErrCodeUnsupportedPDF PDFErrorCode = "UNSUPPORTED_PDF"
	ErrCodeObjectNotFound PDFErrorCode = "OBJECT_NOT_FOUND"
	ErrCodeInvalidObject  PDFErrorCode = "INVALID_OBJECT"
	ErrCodeStreamError    PDFErrorCode = "STREAM_ERROR"
	ErrCodeXRefError      PDFErrorCode = "XREF_ERROR"

	// Encryption errors
	ErrCodeEncrypted         PDFErrorCode = "ENCRYPTED"
	ErrCodeDecryptionFailed  PDFErrorCode = "DECRYPTION_FAILED"
	ErrCodeWrongPassword     PDFErrorCode = "WRONG_PASSWORD"
	ErrCodeUnsupportedCrypto PDFErrorCode = "UNSUPPORTED_CRYPTO"

	// Form errors
	ErrCodeNoForms         PDFErrorCode = "NO_FORMS"
	ErrCodeInvalidForm     PDFErrorCode = "INVALID_FORM"
	ErrCodeFieldNotFound   PDFErrorCode = "FIELD_NOT_FOUND"
	ErrCodeInvalidValue    PDFErrorCode = "INVALID_VALUE"
	ErrCodeValidationError PDFErrorCode = "VALIDATION_ERROR"

	// Content errors
	ErrCodeNoContent       PDFErrorCode = "NO_CONTENT"
	ErrCodeExtractionError PDFErrorCode = "EXTRACTION_ERROR"
	ErrCodeFontError       PDFErrorCode = "FONT_ERROR"
	ErrCodeImageError      PDFErrorCode = "IMAGE_ERROR"

	// Write errors
	ErrCodeWriteError   PDFErrorCode = "WRITE_ERROR"
	ErrCodeInvalidInput PDFErrorCode = "INVALID_INPUT"

	// I/O errors
	ErrCodeIOError PDFErrorCode = "IO_ERROR"
)

// PDFError is a structured error type for PDF operations
type PDFError struct {
	Code    PDFErrorCode           // Error category code
	Message string                 // Human-readable message
	Cause   error                  // Underlying error (if any)
	Context map[string]interface{} // Additional context (object number, field name, etc.)
}

// Error implements the error interface
func (e *PDFError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for errors.Is/As support
func (e *PDFError) Unwrap() error {
	return e.Cause
}

// Is checks if this error matches a target PDFError by code
func (e *PDFError) Is(target error) bool {
	if t, ok := target.(*PDFError); ok {
		return e.Code == t.Code
	}
	return false
}

// WithContext adds context to the error and returns the same error for chaining
func (e *PDFError) WithContext(key string, value interface{}) *PDFError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// NewPDFError creates a new PDFError with the given code and message
func NewPDFError(code PDFErrorCode, message string) *PDFError {
	return &PDFError{
		Code:    code,
		Message: message,
	}
}

// NewPDFErrorf creates a new PDFError with a formatted message
func NewPDFErrorf(code PDFErrorCode, format string, args ...interface{}) *PDFError {
	return &PDFError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// WrapError wraps an existing error with a PDFError
func WrapError(code PDFErrorCode, message string, cause error) *PDFError {
	return &PDFError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// WrapErrorf wraps an existing error with a PDFError and formatted message
func WrapErrorf(code PDFErrorCode, cause error, format string, args ...interface{}) *PDFError {
	return &PDFError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   cause,
	}
}

// Sentinel errors for use with errors.Is()
var (
	// Parsing sentinels
	ErrInvalidPDF     = &PDFError{Code: ErrCodeInvalidPDF}
	ErrMalformedPDF   = &PDFError{Code: ErrCodeMalformedPDF}
	ErrUnsupportedPDF = &PDFError{Code: ErrCodeUnsupportedPDF}
	ErrObjectNotFound = &PDFError{Code: ErrCodeObjectNotFound}
	ErrInvalidObject  = &PDFError{Code: ErrCodeInvalidObject}
	ErrStreamError    = &PDFError{Code: ErrCodeStreamError}
	ErrXRefError      = &PDFError{Code: ErrCodeXRefError}

	// Encryption sentinels
	ErrEncrypted         = &PDFError{Code: ErrCodeEncrypted}
	ErrDecryptionFailed  = &PDFError{Code: ErrCodeDecryptionFailed}
	ErrWrongPassword     = &PDFError{Code: ErrCodeWrongPassword}
	ErrUnsupportedCrypto = &PDFError{Code: ErrCodeUnsupportedCrypto}

	// Form sentinels
	ErrNoForms         = &PDFError{Code: ErrCodeNoForms}
	ErrInvalidForm     = &PDFError{Code: ErrCodeInvalidForm}
	ErrFieldNotFound   = &PDFError{Code: ErrCodeFieldNotFound}
	ErrInvalidValue    = &PDFError{Code: ErrCodeInvalidValue}
	ErrValidationError = &PDFError{Code: ErrCodeValidationError}

	// Content sentinels
	ErrNoContent       = &PDFError{Code: ErrCodeNoContent}
	ErrExtractionError = &PDFError{Code: ErrCodeExtractionError}
	ErrFontError       = &PDFError{Code: ErrCodeFontError}
	ErrImageError      = &PDFError{Code: ErrCodeImageError}

	// Write sentinels
	ErrWriteError   = &PDFError{Code: ErrCodeWriteError}
	ErrInvalidInput = &PDFError{Code: ErrCodeInvalidInput}

	// I/O sentinels
	ErrIOError = &PDFError{Code: ErrCodeIOError}
)

// IsPDFError checks if an error is a PDFError and returns it
func IsPDFError(err error) (*PDFError, bool) {
	if pdfErr, ok := err.(*PDFError); ok {
		return pdfErr, true
	}
	return nil, false
}

// GetErrorCode extracts the error code from an error if it's a PDFError
func GetErrorCode(err error) (PDFErrorCode, bool) {
	if pdfErr, ok := err.(*PDFError); ok {
		return pdfErr.Code, true
	}
	return "", false
}

// IsNotFound checks if the error is an object not found error
func IsNotFound(err error) bool {
	if pdfErr, ok := err.(*PDFError); ok {
		return pdfErr.Code == ErrCodeObjectNotFound || pdfErr.Code == ErrCodeFieldNotFound
	}
	return false
}

// IsEncryptionError checks if the error is related to encryption
func IsEncryptionError(err error) bool {
	if pdfErr, ok := err.(*PDFError); ok {
		switch pdfErr.Code {
		case ErrCodeEncrypted, ErrCodeDecryptionFailed, ErrCodeWrongPassword, ErrCodeUnsupportedCrypto:
			return true
		}
	}
	return false
}

// IsValidationError checks if the error is a validation error
func IsValidationError(err error) bool {
	if pdfErr, ok := err.(*PDFError); ok {
		return pdfErr.Code == ErrCodeValidationError || pdfErr.Code == ErrCodeInvalidValue
	}
	return false
}
