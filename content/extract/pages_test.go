package extract

import (
	"testing"
)

func TestExtractObjectBody(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "array body",
			input:    "3 0 obj\n[4 0 R 5 0 R]\nendobj",
			expected: "[4 0 R 5 0 R]",
		},
		{
			name:     "dictionary body",
			input:    "10 0 obj\n<< /Type /Page >>\nendobj",
			expected: "<< /Type /Page >>",
		},
		{
			name:     "no obj wrapper — contains 'obj' substring so strips prefix",
			input:    "no obj wrapper",
			expected: "wrapper",
		},
		{
			name:     "plain text without obj keyword",
			input:    "just some text",
			expected: "just some text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractObjectBody(tt.input)
			if got != tt.expected {
				t.Errorf("extractObjectBody(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
