package encrypt

import (
	"testing"
)

func TestParseEncryptionDictionary(t *testing.T) {
	// Sample PDF with encryption dictionary
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Filter /Standard
/V 4
/R 4
/Length 128
/O (12345678901234567890123456789012)
/U (abcdefghijklmnopqrstuvwxyzabcdef)
/P -3084
/EncryptMetadata false
>>
endobj
trailer
<<
/Encrypt 1 0 R
/Root 2 0 R
>>
`)

	encrypt, err := ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("ParseEncryptionDictionary() error = %v", err)
	}

	if encrypt.V != 4 {
		t.Errorf("ParseEncryptionDictionary() V = %d, want 4", encrypt.V)
	}
	if encrypt.R != 4 {
		t.Errorf("ParseEncryptionDictionary() R = %d, want 4", encrypt.R)
	}
	if encrypt.KeyLength != 16 {
		t.Errorf("ParseEncryptionDictionary() KeyLength = %d, want 16", encrypt.KeyLength)
	}
	if len(encrypt.O) != 32 {
		t.Errorf("ParseEncryptionDictionary() O length = %d, want 32", len(encrypt.O))
	}
	if len(encrypt.U) != 32 {
		t.Errorf("ParseEncryptionDictionary() U length = %d, want 32", len(encrypt.U))
	}
	if encrypt.P != -3084 {
		t.Errorf("ParseEncryptionDictionary() P = %d, want -3084", encrypt.P)
	}
	if encrypt.EncryptMetadata {
		t.Error("ParseEncryptionDictionary() EncryptMetadata = true, want false")
	}
}

func TestParseEncryptionDictionary_Defaults(t *testing.T) {
	// PDF with minimal encryption dictionary
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Filter /Standard
/O (12345678901234567890123456789012)
/U (abcdefghijklmnopqrstuvwxyzabcdef)
>>
endobj
trailer
<<
/Encrypt 1 0 R
/Root 2 0 R
>>
`)

	encrypt, err := ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("ParseEncryptionDictionary() error = %v", err)
	}

	// Should use defaults
	if encrypt.V == 0 {
		t.Error("ParseEncryptionDictionary() V should have default value")
	}
	if encrypt.R == 0 {
		t.Error("ParseEncryptionDictionary() R should have default value")
	}
	if encrypt.KeyLength == 0 {
		t.Error("ParseEncryptionDictionary() KeyLength should have default value")
	}
}

func TestParseEncryptionDictionary_StrF(t *testing.T) {
	// Per ISO 32000-1 §7.6.5: with crypt filters in use (V ≥ 4) an absent /StrF
	// defaults to /Identity; for V 1/2 strings are always encrypted.
	build := func(body string) []byte {
		return []byte(`%PDF-1.4
1 0 obj
<<
/Filter /Standard
` + body + `
/O (12345678901234567890123456789012)
/U (abcdefghijklmnopqrstuvwxyzabcdef)
>>
endobj
trailer
<<
/Encrypt 1 0 R
/Root 2 0 R
>>
`)
	}

	cases := []struct {
		name string
		body string
		want bool
	}{
		{"V4 absent StrF defaults to Identity", "/V 4\n/R 4\n/Length 128", true},
		{"V4 explicit StdCF", "/V 4\n/R 4\n/Length 128\n/CF <</StdCF <</CFM /AESV2>>>>\n/StmF /StdCF\n/StrF /StdCF", false},
		{"V4 explicit Identity", "/V 4\n/R 4\n/Length 128\n/CF <</StdCF <</CFM /AESV2>>>>\n/StmF /StdCF\n/StrF /Identity", true},
		{"V2 strings always encrypted", "/V 2\n/R 3\n/Length 128", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := ParseEncryptionDictionary(build(tc.body), false)
			if err != nil {
				t.Fatalf("ParseEncryptionDictionary() error = %v", err)
			}
			if enc.StrFIdentity != tc.want {
				t.Errorf("StrFIdentity = %v, want %v", enc.StrFIdentity, tc.want)
			}
		})
	}
}

func TestParseEncryptionDictionary_NotFound(t *testing.T) {
	// PDF without encryption
	pdfBytes := []byte(`%PDF-1.4
trailer
<<
/Root 2 0 R
>>
`)

	_, err := ParseEncryptionDictionary(pdfBytes, false)
	if err == nil {
		t.Error("ParseEncryptionDictionary() should return error when /Encrypt not found")
	}
}

func TestParseEncryptionDictionary_NestedDicts(t *testing.T) {
	// PDF with nested dictionaries (CF structure)
	pdfBytes := []byte(`%PDF-1.4
1 0 obj
<<
/Filter /Standard
/V 4
/R 4
/Length 128
/CF <<
/StdCF <<
/Length 128
/V 4
>>
>>
/O (12345678901234567890123456789012)
/U (abcdefghijklmnopqrstuvwxyzabcdef)
/P -3084
>>
endobj
trailer
<<
/Encrypt 1 0 R
/Root 2 0 R
>>
`)

	encrypt, err := ParseEncryptionDictionary(pdfBytes, false)
	if err != nil {
		t.Fatalf("ParseEncryptionDictionary() error = %v", err)
	}

	// Should parse top-level V, not nested one
	if encrypt.V != 4 {
		t.Errorf("ParseEncryptionDictionary() V = %d, want 4", encrypt.V)
	}
}
