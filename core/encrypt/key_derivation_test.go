package encrypt

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestParseHexString(t *testing.T) {
	tests := []struct {
		name     string
		hexStr   string
		expected []byte
	}{
		{"simple", "0123456789ABCDEF", []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF}},
		{"with spaces", "01 23 45 67", []byte{0x01, 0x23, 0x45, 0x67}},
		{"with newlines", "01\n23\r45", []byte{0x01, 0x23, 0x45}},
		{"empty", "", []byte{}},
		{"odd length", "123", []byte{0x12}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHexString(tt.hexStr)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("parseHexString(%q) = %x, want %x", tt.hexStr, result, tt.expected)
			}
		})
	}
}

func TestExtractFileID(t *testing.T) {
	tests := []struct {
		name     string
		pdfBytes []byte
		expected []byte
	}{
		{
			name:     "hex format",
			pdfBytes: []byte("/ID [ <7FB157EB1234567890ABCDEF> <other> ]"),
			expected: []byte{0x7F, 0xB1, 0x57, 0xEB, 0x12, 0x34, 0x56, 0x78, 0x90, 0xAB, 0xCD, 0xEF},
		},
		{
			name:     "hex format no space",
			pdfBytes: []byte("/ID[<7FB157EB>"),
			expected: []byte{0x7F, 0xB1, 0x57, 0xEB},
		},
		{
			name:     "binary format",
			pdfBytes: []byte("/ID [ (\x7F\xB1\x57\xEB) (other) ]"),
			expected: []byte{0x7F, 0xB1, 0x57, 0xEB},
		},
		{
			name:     "not found",
			pdfBytes: []byte("some other content"),
			expected: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFileID(tt.pdfBytes, false)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("ExtractFileID() = %x, want %x", result, tt.expected)
			}
		})
	}
}

func TestDeriveEncryptionKey(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R:               4,
		KeyLength:       16,
		O:               []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20},
		P:               -3084,
		EncryptMetadata: false,
	}
	fileID := []byte{0x7F, 0xB1, 0x57, 0xEB}

	// Test with empty password
	key, err := DeriveEncryptionKey([]byte{}, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() error = %v", err)
	}
	if len(key) != encrypt.KeyLength {
		t.Errorf("DeriveEncryptionKey() key length = %d, want %d", len(key), encrypt.KeyLength)
	}

	// Test with password
	key2, err := DeriveEncryptionKey([]byte("test"), encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() error = %v", err)
	}
	if len(key2) != encrypt.KeyLength {
		t.Errorf("DeriveEncryptionKey() key length = %d, want %d", len(key2), encrypt.KeyLength)
	}

	// Keys should be different
	if bytes.Equal(key, key2) {
		t.Error("DeriveEncryptionKey() empty password and test password produced same key")
	}
}

func TestComputeUValue(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R:         4,
		KeyLength: 16,
	}
	encryptKey := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	fileID := []byte{0x7F, 0xB1, 0x57, 0xEB}

	uValue, err := ComputeUValue(encryptKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("ComputeUValue() error = %v", err)
	}
	if len(uValue) != 32 {
		t.Errorf("ComputeUValue() length = %d, want 32", len(uValue))
	}
}

func TestDeriveOwnerKey(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R:               4,
		KeyLength:       16,
		O:               []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20},
		P:               -3084,
		EncryptMetadata: false,
	}
	fileID := []byte{0x7F, 0xB1, 0x57, 0xEB}

	ownerKey, err := DeriveOwnerKey([]byte("owner"), encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveOwnerKey() error = %v", err)
	}
	if len(ownerKey) != encrypt.KeyLength {
		t.Errorf("DeriveOwnerKey() key length = %d, want %d", len(ownerKey), encrypt.KeyLength)
	}
}
