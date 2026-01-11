package parser

import (
	"bytes"
	"testing"
)

func TestDecodeASCIIHex(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		wantErr  bool
	}{
		{
			name:     "simple hex",
			input:    []byte("48656C6C6F>"),
			expected: []byte("Hello"),
		},
		{
			name:     "lowercase hex",
			input:    []byte("48656c6c6f>"),
			expected: []byte("Hello"),
		},
		{
			name:     "with whitespace",
			input:    []byte("48 65 6C 6C 6F>"),
			expected: []byte("Hello"),
		},
		{
			name:     "odd number of digits",
			input:    []byte("123>"),
			expected: []byte{0x12, 0x30},
		},
		{
			name:     "empty with terminator",
			input:    []byte(">"),
			expected: []byte{},
		},
		{
			name:     "binary data",
			input:    []byte("00FF7F80>"),
			expected: []byte{0x00, 0xFF, 0x7F, 0x80},
		},
		{
			name:    "invalid character",
			input:   []byte("GG>"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeASCIIHex(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDecodeASCII85(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		wantErr  bool
	}{
		{
			name:     "hello world",
			input:    []byte("<~87cURD]j7BEbo80~>"),
			expected: []byte("Hello world!"),
		},
		{
			name:     "with z compression",
			input:    []byte("<~z~>"),
			expected: []byte{0, 0, 0, 0},
		},
		{
			name:     "Man",
			input:    []byte("<~9jqo~>"),
			expected: []byte("Man"),
		},
		{
			name:     "empty",
			input:    []byte("<~~>"),
			expected: []byte{},
		},
		{
			name:     "without delimiters",
			input:    []byte("9jqo~>"),
			expected: []byte("Man"),
		},
		{
			name:     "test string",
			input:    []byte("<~FCfN8~>"),
			expected: []byte("test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeASCII85(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("got %v (%s), want %v (%s)", result, string(result), tt.expected, string(tt.expected))
			}
		})
	}
}

func TestDecodeRunLength(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		wantErr  bool
	}{
		{
			name:     "literal run",
			input:    []byte{4, 'H', 'e', 'l', 'l', 'o', 128},
			expected: []byte("Hello"),
		},
		{
			name:     "repeated bytes",
			input:    []byte{252, 'A', 128}, // 257-252=5 A's
			expected: []byte("AAAAA"),
		},
		{
			name:     "mixed",
			input:    []byte{2, 'H', 'i', '!', 253, ' ', 128}, // "Hi!" + 4 spaces
			expected: []byte("Hi!    "),
		},
		{
			name:     "empty",
			input:    []byte{128},
			expected: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DecodeRunLength(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("got %v (%s), want %v (%s)", result, string(result), tt.expected, string(tt.expected))
			}
		})
	}
}

func TestEncodeASCIIHex(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "hello",
			input:    []byte("Hello"),
			expected: []byte("48656C6C6F>"),
		},
		{
			name:     "binary",
			input:    []byte{0x00, 0xFF, 0x7F},
			expected: []byte("007FFF7F>"),
		},
		{
			name:     "empty",
			input:    []byte{},
			expected: []byte(">"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeASCIIHex(tt.input)
			// Verify it decodes back to original
			decoded, err := DecodeASCIIHex(result)
			if err != nil {
				t.Fatalf("round-trip decode failed: %v", err)
			}
			if !bytes.Equal(decoded, tt.input) {
				t.Errorf("round-trip failed: got %v, want %v", decoded, tt.input)
			}
		})
	}
}

func TestEncodeASCII85(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "hello world",
			input: []byte("Hello World!"),
		},
		{
			name:  "zeros",
			input: []byte{0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name:  "binary",
			input: []byte{0xFF, 0x00, 0x7F, 0x80, 0x01},
		},
		{
			name:  "partial",
			input: []byte("Man"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeASCII85(tt.input)
			decoded, err := DecodeASCII85(encoded)
			if err != nil {
				t.Fatalf("round-trip decode failed: %v", err)
			}
			if !bytes.Equal(decoded, tt.input) {
				t.Errorf("round-trip failed: encoded=%s, decoded=%v, want %v", string(encoded), decoded, tt.input)
			}
		})
	}
}

func TestEncodeRunLength(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "no repetition",
			input: []byte("Hello"),
		},
		{
			name:  "all same",
			input: []byte("AAAAAAAAAA"),
		},
		{
			name:  "mixed",
			input: []byte("Hello    World"),
		},
		{
			name:  "empty",
			input: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeRunLength(tt.input)
			decoded, err := DecodeRunLength(encoded)
			if err != nil {
				t.Fatalf("round-trip decode failed: %v", err)
			}
			if !bytes.Equal(decoded, tt.input) {
				t.Errorf("round-trip failed: decoded=%v, want %v", decoded, tt.input)
			}
		})
	}
}

func TestDecodeDCTDecode(t *testing.T) {
	// Minimal valid JPEG: SOI (0xFF 0xD8) + some data
	validJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	tests := []struct {
		name    string
		data    []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "valid JPEG",
			data:    validJPEG,
			want:    validJPEG,
			wantErr: false,
		},
		{
			name:    "too short",
			data:    []byte{0xFF},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid header",
			data:    []byte{0xFF, 0xD9}, // EOI instead of SOI
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty",
			data:    []byte{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeDCTDecode(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeDCTDecode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Errorf("DecodeDCTDecode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeDCTDecode(t *testing.T) {
	// Minimal valid JPEG
	validJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}

	tests := []struct {
		name    string
		data    []byte
		want    []byte
		wantErr bool
	}{
		{
			name:    "valid JPEG",
			data:    validJPEG,
			want:    validJPEG,
			wantErr: false,
		},
		{
			name:    "invalid header",
			data:    []byte{0xFF, 0xD9},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "too short",
			data:    []byte{0xFF},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodeDCTDecode(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeDCTDecode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.want) {
				t.Errorf("EncodeDCTDecode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDecodeFilter(t *testing.T) {
	// Test that DecodeFilter dispatches correctly
	hexEncoded := []byte("48656C6C6F>")
	result, err := DecodeFilter(hexEncoded, "/ASCIIHexDecode")
	if err != nil {
		t.Fatalf("DecodeFilter failed: %v", err)
	}
	if string(result) != "Hello" {
		t.Errorf("got %s, want Hello", string(result))
	}

	// Test DCTDecode
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	result, err = DecodeFilter(jpegData, "/DCTDecode")
	if err != nil {
		t.Fatalf("DecodeFilter with DCTDecode failed: %v", err)
	}
	if !bytes.Equal(result, jpegData) {
		t.Errorf("DCTDecode should return data as-is, got %v, want %v", result, jpegData)
	}

	// Test unsupported filter
	_, err = DecodeFilter([]byte{}, "/Unknown")
	if err == nil {
		t.Error("expected error for unsupported filter")
	}
}
