package extract

import (
	"testing"
)

func TestTokenizeDifferences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "concatenated names",
			input:    "2/fi/fl",
			expected: []string{"2", "/fi", "/fl"},
		},
		{
			name:     "mixed spaces and concatenated",
			input:    "2/fi 39/quoteright",
			expected: []string{"2", "/fi", "39", "/quoteright"},
		},
		{
			name:     "leading/trailing whitespace and spaces",
			input:    " 128 /Euro ",
			expected: []string{"128", "/Euro"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "newlines as separators",
			input:    "2/fi/fl\n39/quoteright",
			expected: []string{"2", "/fi", "/fl", "39", "/quoteright"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeDifferences(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("tokenizeDifferences(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.expected, len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("tokenizeDifferences(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParseCIDWidthArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[int]int
	}{
		{
			name:  "consecutive format",
			input: "[1 [500 600 700]]",
			expected: map[int]int{
				1: 500, 2: 600, 3: 700,
			},
		},
		{
			name:  "range format",
			input: "[10 20 300]",
			expected: func() map[int]int {
				m := make(map[int]int)
				for i := 10; i <= 20; i++ {
					m[i] = 300
				}
				return m
			}(),
		},
		{
			name:  "mixed formats",
			input: "[1 [500 600] 10 20 300]",
			expected: func() map[int]int {
				m := map[int]int{1: 500, 2: 600}
				for i := 10; i <= 20; i++ {
					m[i] = 300
				}
				return m
			}(),
		},
		{
			name:     "empty",
			input:    "[]",
			expected: map[int]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCIDWidthArray(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("parseCIDWidthArray(%q) returned %d entries, want %d", tt.input, len(got), len(tt.expected))
			}
			for k, v := range tt.expected {
				if got[k] != v {
					t.Errorf("parseCIDWidthArray(%q)[%d] = %d, want %d", tt.input, k, got[k], v)
				}
			}
		})
	}
}

func TestHexToUnicodeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single BMP char A",
			input:    "0041",
			expected: "A",
		},
		{
			name:     "multi-char fi ligature decomposed",
			input:    "00660069",
			expected: "fi",
		},
		{
			name:     "single ligature char U+FB01",
			input:    "FB01",
			expected: "ﬁ",
		},
		{
			name:     "2-char hex",
			input:    "41",
			expected: "A",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hexToUnicodeString(tt.input)
			if got != tt.expected {
				t.Errorf("hexToUnicodeString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
