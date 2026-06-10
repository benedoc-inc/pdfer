package encrypt

import (
	"bytes"
	"testing"

	"github.com/benedoc-inc/pdfer/v2/types"
)

// testEncRC4 returns a V2 (RC4) encryption context with a fixed master key.
func testEncRC4() *types.PDFEncryption {
	return &types.PDFEncryption{
		V:          2,
		R:          3,
		KeyLength:  16,
		EncryptKey: bytes.Repeat([]byte{0xCD}, 16),
	}
}

// TestDecryptStringsInContent_IdentityPassThrough verifies that a conformant
// /StrF /Identity document is returned byte-identical — including hex strings
// long enough that the old heuristic would have tried (and silently mangled)
// an AES decryption.
func TestDecryptStringsInContent_IdentityPassThrough(t *testing.T) {
	longHex := bytes.Repeat([]byte("AB"), 40) // 40-byte decoded value, > one AES block
	content := append([]byte("<</Title (cleartext title)/Custom <"), longHex...)
	content = append(content, []byte(">>>")...)

	got, err := DecryptStringsInContent(content, 4, 0, testEnc(true))
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Identity document was modified:\n got %q\nwant %q", got, content)
	}
}

// TestStringRoundTrip_ShortRC4 verifies that short RC4-encrypted strings round-trip.
// The old read path skipped hex strings shorter than 32 decoded bytes, so V1/V2
// short string values never decrypted.
func TestStringRoundTrip_ShortRC4(t *testing.T) {
	enc := testEncRC4()
	body := []byte("<</V (ok)>>")

	ct, err := EncryptStringsInContent(body, 9, 0, enc)
	if err != nil {
		t.Fatalf("EncryptStringsInContent: %v", err)
	}
	if bytes.Contains(ct, []byte("(ok)")) {
		t.Fatal("short RC4 string was not encrypted")
	}
	back, err := DecryptStringsInContent(ct, 9, 0, enc)
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Contains(back, []byte("(ok)")) {
		t.Errorf("short RC4 string did not round-trip; got %q", back)
	}
}

// TestDecryptStringsInContent_LiteralCiphertext verifies that encrypted string
// values stored as literal (...) strings — the form external StdCF producers
// emit — are decrypted, not just hex <...> strings.
func TestDecryptStringsInContent_LiteralCiphertext(t *testing.T) {
	enc := testEnc(false)
	plain := []byte("external literal value")

	ct, err := EncryptObject(plain, 12, 0, enc)
	if err != nil {
		t.Fatalf("EncryptObject: %v", err)
	}
	content := append([]byte("<</T ("), encAppendLiteral(nil, ct)...)
	content = append(content, []byte(")>>")...)

	got, err := DecryptStringsInContent(content, 12, 0, enc)
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Contains(got, plain) {
		t.Errorf("literal ciphertext was not decrypted; got %q", got)
	}
}

// TestStringsInContent_UnterminatedParenPassThrough verifies that a lone '('
// byte (as found inside binary content that was mistaken for a dict body) does
// not cause the walkers to consume and re-encode the rest of the input.
func TestStringsInContent_UnterminatedParenPassThrough(t *testing.T) {
	content := []byte("<</X 1>> binary \x28 tail with no closing paren")

	got, err := EncryptStringsInContent(content, 2, 0, testEnc(false))
	if err != nil {
		t.Fatalf("EncryptStringsInContent: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("encrypt mangled unterminated paren:\n got %q\nwant %q", got, content)
	}

	got, err = DecryptStringsInContent(content, 2, 0, testEnc(false))
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("decrypt mangled unterminated paren:\n got %q\nwant %q", got, content)
	}
}

// TestDecryptStringsInContent_EmptyAndMalformedPassThrough verifies that strings
// that cannot be ciphertext in an AES document (too short for an IV, not
// block-aligned) are passed through unchanged rather than corrupted.
func TestDecryptStringsInContent_EmptyAndMalformedPassThrough(t *testing.T) {
	enc := testEnc(false)
	content := []byte("<</A ()/B (tiny)/C <4142>>>")

	got, err := DecryptStringsInContent(content, 5, 0, enc)
	if err != nil {
		t.Fatalf("DecryptStringsInContent: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("malformed-ciphertext strings were modified:\n got %q\nwant %q", got, content)
	}
}
