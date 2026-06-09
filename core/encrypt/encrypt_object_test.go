package encrypt

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

// testEnc returns an AES-128 (V4) encryption context with a fixed master key.
func testEnc(strFIdentity bool) *types.PDFEncryption {
	return &types.PDFEncryption{
		V:            4,
		KeyLength:    16,
		EncryptKey:   bytes.Repeat([]byte{0xAB}, 16),
		StrFIdentity: strFIdentity,
	}
}

// TestEncryptObject_RoundTripsWithDecrypt verifies EncryptObject is the inverse
// of DecryptObject for the V4 (AES-128) handler pdfer emits.
func TestEncryptObject_RoundTripsWithDecrypt(t *testing.T) {
	enc := testEnc(false)
	plain := []byte("the quick brown fox\x00\x01\x02 binary stream data")

	ct, err := EncryptObject(plain, 7, 0, enc)
	if err != nil {
		t.Fatalf("EncryptObject: %v", err)
	}
	if bytes.Contains(ct, plain) {
		t.Fatal("ciphertext still contains plaintext")
	}
	pt, err := DecryptObject(ct, 7, 0, enc)
	if err != nil {
		t.Fatalf("DecryptObject: %v", err)
	}
	if !bytes.Equal(pt, plain) {
		t.Errorf("round-trip mismatch: got %q want %q", pt, plain)
	}
}

// TestEncryptStringsInContent_HonorsStrF checks that string encryption is a
// no-op when /StrF is /Identity, and otherwise encrypts to a hex string.
func TestEncryptStringsInContent_HonorsStrF(t *testing.T) {
	body := []byte("<</Type /Filespec /F (secret.txt)>>")

	// /StrF /Identity: strings must stay in the clear.
	got, err := EncryptStringsInContent(body, 3, 0, testEnc(true))
	if err != nil {
		t.Fatalf("EncryptStringsInContent (identity): %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("Identity StrF should leave strings unchanged; got %q", got)
	}

	// Non-Identity: the literal string is replaced by an encrypted hex string.
	got, err = EncryptStringsInContent(body, 3, 0, testEnc(false))
	if err != nil {
		t.Fatalf("EncryptStringsInContent (stdcf): %v", err)
	}
	if bytes.Contains(got, []byte("(secret.txt)")) {
		t.Error("string should have been encrypted, but cleartext remains")
	}
	if !bytes.Contains(got, []byte("/Type /Filespec")) {
		t.Error("non-string dict content should be preserved")
	}
	// And it must decrypt back via the read-side helper.
	back, err := DecryptStringsInContent(got, 3, 0, testEnc(false))
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Contains(back, []byte("(secret.txt)")) {
		t.Errorf("string did not round-trip; got %q", back)
	}
}

// TestParsePDFLiteralStringBytes exercises the literal-string parser.
func TestParsePDFLiteralStringBytes(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantEnd int
	}{
		{"(hello)", "hello", 7},
		{"(hi) extra", "hi", 4},
		{"(a\\nb)", "a\nb", 6},
		{"(a\\(b\\)c)", "a(b)c", 9},
		{"(nest(ed))", "nest(ed)", 10},
		{"(\\101)", "A", 6}, // octal \101 = 65 = 'A'
	}
	for _, tc := range cases {
		got, end, ok := parsePDFLiteralStringBytes([]byte(tc.input), 1)
		if !ok {
			t.Errorf("input %q: unexpectedly unterminated", tc.input)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("input %q: got %q, want %q", tc.input, got, tc.want)
		}
		if end != tc.wantEnd {
			t.Errorf("input %q: end=%d, want %d", tc.input, end, tc.wantEnd)
		}
	}
}

// TestEncParseHexString exercises the hex-string parser.
func TestEncParseHexString(t *testing.T) {
	cases := []struct {
		input   string
		want    []byte
		wantEnd int
	}{
		{"<48656C6C6F>", []byte("Hello"), 12},
		{"<4142>", []byte{0x41, 0x42}, 6},
		{"<41 42\n43>", []byte{0x41, 0x42, 0x43}, 10},
	}
	for _, tc := range cases {
		end, got, err := encParseHexString([]byte(tc.input), 0)
		if err != nil {
			t.Errorf("input %q: unexpected error %v", tc.input, err)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("input %q: got %v, want %v", tc.input, got, tc.want)
		}
		if end != tc.wantEnd {
			t.Errorf("input %q: end=%d, want %d", tc.input, end, tc.wantEnd)
		}
	}
}
