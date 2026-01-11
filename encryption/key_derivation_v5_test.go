package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestDeriveEncryptionKeyV5(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
	}

	password := []byte("test")
	fileID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	key, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	// Key should be 32 bytes for AES-256
	if len(key) != 32 {
		t.Errorf("Key length = %d, want 32", len(key))
	}

	// Verify it's deterministic
	key2, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	if !bytes.Equal(key, key2) {
		t.Error("Key derivation should be deterministic")
	}

	// Verify different passwords produce different keys
	key3, err := DeriveEncryptionKeyV5([]byte("different"), encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	if bytes.Equal(key, key3) {
		t.Error("Different passwords should produce different keys")
	}
}

func TestDeriveEncryptionKeyV5_EmptyFileID(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
	}

	password := []byte("test")
	fileID := []byte{} // Empty file ID

	key, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Key length = %d, want 32", len(key))
	}
}

func TestDeriveEncryptionKeyV5_ShortFileID(t *testing.T) {
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
	}

	password := []byte("test")
	fileID := []byte{0x01, 0x02} // Short file ID (should be padded to 8 bytes)

	key, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Key length = %d, want 32", len(key))
	}
}

func TestUnwrapUserKeyV5(t *testing.T) {
	// Create a test scenario: encrypt a key with AES-128 ECB, then unwrap it
	passwordKey := make([]byte, 32)
	for i := range passwordKey {
		passwordKey[i] = byte(i)
	}

	// Create a test encryption key to wrap (32 bytes for AES-256)
	testKey := make([]byte, 32)
	for i := range testKey {
		testKey[i] = byte(i + 100)
	}

	// Wrap the key: encrypt with AES-128 ECB using first 16 bytes of password key
	wrapKey := passwordKey[:16]
	block, err := aes.NewCipher(wrapKey)
	if err != nil {
		t.Fatalf("Failed to create AES cipher: %v", err)
	}

	// Add PKCS#7 padding
	paddingLen := 16 - (len(testKey) % 16)
	paddedKey := make([]byte, len(testKey)+paddingLen)
	copy(paddedKey, testKey)
	for i := len(testKey); i < len(paddedKey); i++ {
		paddedKey[i] = byte(paddingLen)
	}

	// Encrypt in ECB mode (each 16-byte block independently)
	wrapped := make([]byte, len(paddedKey))
	for i := 0; i < len(paddedKey); i += 16 {
		block.Encrypt(wrapped[i:], paddedKey[i:i+16])
	}

	// Create encryption struct with wrapped key
	encrypt := &types.PDFEncryption{
		R:  5,
		V:  5,
		UE: wrapped,
	}

	// Unwrap
	unwrapped, err := UnwrapUserKeyV5(passwordKey, encrypt, false)
	if err != nil {
		t.Fatalf("UnwrapUserKeyV5() error = %v", err)
	}

	if !bytes.Equal(unwrapped, testKey) {
		t.Errorf("Unwrapped key doesn't match original")
		t.Errorf("Original: %x", testKey)
		t.Errorf("Unwrapped: %x", unwrapped)
	}
}

func TestDeriveEncryptionKey_DispatchToV5(t *testing.T) {
	// Test that DeriveEncryptionKey dispatches to V5 for R>=5
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
	}

	password := []byte("test")
	fileID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	key, err := DeriveEncryptionKey(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() error = %v", err)
	}

	// Should be 32 bytes for V5
	if len(key) != 32 {
		t.Errorf("Key length = %d, want 32 for V5", len(key))
	}

	// Verify it uses SHA-256 (not MD5) by checking it's different from V4
	encryptV4 := &types.PDFEncryption{
		R: 4,
		V: 4,
	}

	keyV4, err := DeriveEncryptionKey(password, encryptV4, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKey() error = %v", err)
	}

	// V4 key should be different from V5 (different algorithms)
	if bytes.Equal(key, keyV4) {
		t.Error("V5 key should be different from V4 key (SHA-256 vs MD5)")
	}
}

func TestDeriveEncryptionKeyV5_Algorithm(t *testing.T) {
	// Verify the algorithm matches ISO 32000-2 spec:
	// 1. Concatenate password + fileID (8 bytes)
	// 2. SHA-256 hash
	// 3. Iterate 64 times with SHA-256

	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
	}

	password := []byte("test")
	fileID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	key, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	// Manually verify first step: SHA-256(password + fileID)
	input := make([]byte, len(password)+8)
	copy(input, password)
	copy(input[len(password):], fileID)

	hash := sha256.Sum256(input)
	expected := hash[:]

	// Iterate 64 times
	for i := 0; i < 64; i++ {
		hash = sha256.Sum256(expected)
		expected = hash[:]
	}

	if !bytes.Equal(key, expected) {
		t.Error("Key derivation doesn't match expected algorithm")
		t.Errorf("Got:      %x", key)
		t.Errorf("Expected: %x", expected)
	}
}

func TestComputeUValueV5(t *testing.T) {
	// Test U value computation for V5
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
		U: make([]byte, 48), // 48-byte U value
	}

	// Set up a test U value with validation salt and user key salt
	validationSalt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	userKeySalt := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	copy(encrypt.U[:8], validationSalt)
	copy(encrypt.U[40:48], userKeySalt)

	password := []byte("test")
	fileID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	// Derive password key
	passwordKey, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	// Compute U value
	uValue, err := ComputeUValueV5(password, passwordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("ComputeUValueV5() error = %v", err)
	}

	// Verify structure
	if len(uValue) != 48 {
		t.Errorf("U value length = %d, want 48", len(uValue))
	}

	// Validation salt should match
	if !bytes.Equal(uValue[:8], validationSalt) {
		t.Error("Validation salt mismatch")
	}

	// User key salt should match
	if !bytes.Equal(uValue[40:48], userKeySalt) {
		t.Error("User key salt mismatch")
	}

	// Encrypted hash should be 32 bytes
	if len(uValue[8:40]) != 32 {
		t.Errorf("Encrypted hash length = %d, want 32", len(uValue[8:40]))
	}
}

func TestVerifyUValueV5(t *testing.T) {
	// Test U value verification for V5
	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
		U: make([]byte, 48),
	}

	password := []byte("test")
	fileID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

	// Set up validation salt and user key salt
	validationSalt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	userKeySalt := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	copy(encrypt.U[:8], validationSalt)
	copy(encrypt.U[40:48], userKeySalt)

	// Derive password key
	passwordKey, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	// Compute correct U value and store encrypted hash
	correctU, err := ComputeUValueV5(password, passwordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("ComputeUValueV5() error = %v", err)
	}
	copy(encrypt.U, correctU)

	// Verify with correct password
	match, err := VerifyUValueV5(password, passwordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("VerifyUValueV5() error = %v", err)
	}
	if !match {
		t.Error("Correct password should verify successfully")
	}

	// Verify with wrong password
	wrongPassword := []byte("wrong")
	wrongPasswordKey, err := DeriveEncryptionKeyV5(wrongPassword, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	match, err = VerifyUValueV5(wrongPassword, wrongPasswordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("VerifyUValueV5() error = %v", err)
	}
	if match {
		t.Error("Wrong password should not verify successfully")
	}
}

func TestVerifyUValueV5_RealWorldScenario(t *testing.T) {
	// Simulate a real-world scenario:
	// 1. Generate U value from password
	// 2. Store it in encrypt.U
	// 3. Verify it later

	encrypt := &types.PDFEncryption{
		R: 5,
		V: 5,
		U: make([]byte, 48),
	}

	password := []byte("MySecurePassword123!")
	fileID := []byte{0x7F, 0xB1, 0x57, 0xEB, 0x4A, 0x3C, 0x2D, 0x1E}

	// Generate random validation salt and user key salt (in real PDF, these are random)
	validationSalt := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	userKeySalt := []byte{0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA}
	copy(encrypt.U[:8], validationSalt)
	copy(encrypt.U[40:48], userKeySalt)

	// Derive password key
	passwordKey, err := DeriveEncryptionKeyV5(password, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("DeriveEncryptionKeyV5() error = %v", err)
	}

	// Compute and store U value (as if PDF was created)
	uValue, err := ComputeUValueV5(password, passwordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("ComputeUValueV5() error = %v", err)
	}
	copy(encrypt.U, uValue)

	// Now verify (as if decrypting PDF)
	match, err := VerifyUValueV5(password, passwordKey, encrypt, fileID, false)
	if err != nil {
		t.Fatalf("VerifyUValueV5() error = %v", err)
	}

	if !match {
		t.Error("Password verification should succeed for correct password")
		t.Errorf("Stored U:   %x", encrypt.U)
		t.Errorf("Computed U: %x", uValue)
	}
}
