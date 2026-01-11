package writer

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/benedoc-inc/pdfer/types"
)

// SetupAES256Encryption creates an encryption dictionary for AES-256 (V5/R5/R6)
// Based on ISO 32000-2 section 7.6.4
func SetupAES256Encryption(userPassword, ownerPassword []byte, fileID []byte, permissions int32, encryptMetadata bool) (*types.PDFEncryption, error) {
	// Prepare file ID (use first 8 bytes, pad/truncate to 8)
	fileID8 := make([]byte, 8)
	if len(fileID) >= 8 {
		copy(fileID8, fileID[:8])
	} else if len(fileID) > 0 {
		copy(fileID8, fileID)
	}

	// Derive user password key
	userPasswordKey, err := deriveEncryptionKeyV5(userPassword, fileID8)
	if err != nil {
		return nil, fmt.Errorf("failed to derive user password key: %v", err)
	}

	// Derive owner password key
	ownerPasswordKey, err := deriveEncryptionKeyV5(ownerPassword, fileID8)
	if err != nil {
		return nil, fmt.Errorf("failed to derive owner password key: %v", err)
	}

	// Generate random user encryption key (32 bytes for AES-256)
	userEncryptionKey := make([]byte, 32)
	if _, err := rand.Read(userEncryptionKey); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %v", err)
	}

	// Generate random owner encryption key (32 bytes)
	ownerEncryptionKey := make([]byte, 32)
	if _, err := rand.Read(ownerEncryptionKey); err != nil {
		return nil, fmt.Errorf("failed to generate owner encryption key: %v", err)
	}

	// Generate random salts
	validationSalt := make([]byte, 8)
	userKeySalt := make([]byte, 8)
	ownerValidationSalt := make([]byte, 8)
	ownerKeySalt := make([]byte, 8)

	if _, err := rand.Read(validationSalt); err != nil {
		return nil, fmt.Errorf("failed to generate validation salt: %v", err)
	}
	if _, err := rand.Read(userKeySalt); err != nil {
		return nil, fmt.Errorf("failed to generate user key salt: %v", err)
	}
	if _, err := rand.Read(ownerValidationSalt); err != nil {
		return nil, fmt.Errorf("failed to generate owner validation salt: %v", err)
	}
	if _, err := rand.Read(ownerKeySalt); err != nil {
		return nil, fmt.Errorf("failed to generate owner key salt: %v", err)
	}

	// Compute U value (user password validation)
	uValue, err := computeUValueV5(userPassword, userPasswordKey, validationSalt, userKeySalt)
	if err != nil {
		return nil, fmt.Errorf("failed to compute U value: %v", err)
	}

	// Compute O value (owner password validation)
	// For V5, O value uses the U value (48 bytes), not the user encryption key
	oValue, err := computeOValueV5(ownerPassword, ownerPasswordKey, ownerValidationSalt, ownerKeySalt, uValue)
	if err != nil {
		return nil, fmt.Errorf("failed to compute O value: %v", err)
	}

	// Wrap user encryption key with user password key
	ueValue, err := wrapKeyV5(userEncryptionKey, userPasswordKey)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap user encryption key: %v", err)
	}

	// Wrap user encryption key with owner password key (OE contains user key wrapped with owner key)
	oeValue, err := wrapKeyV5(userEncryptionKey, ownerPasswordKey)
	if err != nil {
		return nil, fmt.Errorf("failed to wrap user encryption key with owner key: %v", err)
	}

	encrypt := &types.PDFEncryption{
		V:               5,
		R:               5,
		KeyLength:       32, // 256 bits
		Filter:          "Standard",
		P:               permissions,
		EncryptMetadata: encryptMetadata,
		O:               oValue,
		U:               uValue,
		UE:              ueValue,
		OE:              oeValue,
		EncryptKey:      userEncryptionKey, // Store the actual encryption key
	}

	return encrypt, nil
}

// deriveEncryptionKeyV5 derives a 32-byte key from password using SHA-256
// Based on ISO 32000-2 section 7.6.4.3.3
func deriveEncryptionKeyV5(password []byte, fileID []byte) ([]byte, error) {
	// Concatenate password + fileID (8 bytes)
	input := make([]byte, len(password)+8)
	copy(input, password)
	copy(input[len(password):], fileID)

	// SHA-256 hash
	hash := sha256.Sum256(input)
	key := hash[:]

	// Iterate 64 times
	for i := 0; i < 64; i++ {
		hash = sha256.Sum256(key)
		key = hash[:]
	}

	return key, nil
}

// computeUValueV5 computes the U value for user password validation
// Based on ISO 32000-2 section 7.6.4.4.9
func computeUValueV5(password, passwordKey, validationSalt, userKeySalt []byte) ([]byte, error) {
	// Compute SHA-256(password + validation_salt)
	hash := sha256.New()
	hash.Write(password)
	hash.Write(validationSalt)
	hashed := hash.Sum(nil)

	// Encrypt with AES-128 ECB using first 16 bytes of password-derived key
	encryptKey := passwordKey[:16]
	block, err := aes.NewCipher(encryptKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	// Encrypt hash (32 bytes) in 2 blocks of 16 bytes each (ECB mode)
	encrypted := make([]byte, 32)
	block.Encrypt(encrypted[:16], hashed[:16])
	block.Encrypt(encrypted[16:], hashed[16:])

	// U value: validation_salt (8) + encrypted_hash (32) + user_key_salt (8) = 48 bytes
	result := make([]byte, 48)
	copy(result[:8], validationSalt)
	copy(result[8:40], encrypted)
	copy(result[40:48], userKeySalt)

	return result, nil
}

// computeOValueV5 computes the O value for owner password validation
// Based on ISO 32000-2 section 7.6.4.4.10
func computeOValueV5(ownerPassword, ownerPasswordKey, ownerValidationSalt, ownerKeySalt []byte, uValue []byte) ([]byte, error) {
	// Compute SHA-256(owner_password + owner_validation_salt + U_value)
	hash := sha256.New()
	hash.Write(ownerPassword)
	hash.Write(ownerValidationSalt)
	hash.Write(uValue) // U value is 48 bytes
	hashed := hash.Sum(nil)

	// Encrypt with AES-128 ECB using first 16 bytes of owner password-derived key
	encryptKey := ownerPasswordKey[:16]
	block, err := aes.NewCipher(encryptKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	// Encrypt hash (32 bytes) in 2 blocks
	encrypted := make([]byte, 32)
	block.Encrypt(encrypted[:16], hashed[:16])
	block.Encrypt(encrypted[16:], hashed[16:])

	// O value: owner_validation_salt (8) + encrypted_hash (32) + owner_key_salt (8) = 48 bytes
	result := make([]byte, 48)
	copy(result[:8], ownerValidationSalt)
	copy(result[8:40], encrypted)
	copy(result[40:48], ownerKeySalt)

	return result, nil
}

// wrapKeyV5 wraps an encryption key using AES-128 ECB mode
// Based on ISO 32000-2 - used for /UE and /OE
func wrapKeyV5(key, wrappingKey []byte) ([]byte, error) {
	if len(wrappingKey) < 16 {
		return nil, fmt.Errorf("wrapping key too short: %d bytes (need at least 16)", len(wrappingKey))
	}

	// Use first 16 bytes of wrapping key
	wrapKey := wrappingKey[:16]

	// Add PKCS#7 padding
	paddingLen := 16 - (len(key) % 16)
	padded := make([]byte, len(key)+paddingLen)
	copy(padded, key)
	for i := len(key); i < len(padded); i++ {
		padded[i] = byte(paddingLen)
	}

	// Encrypt in ECB mode (each 16-byte block independently)
	block, err := aes.NewCipher(wrapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	wrapped := make([]byte, len(padded))
	for i := 0; i < len(padded); i += 16 {
		block.Encrypt(wrapped[i:], padded[i:i+16])
	}

	return wrapped, nil
}

// CreateEncryptionDictionary creates the encryption dictionary object content for V5
func CreateEncryptionDictionary(encrypt *types.PDFEncryption) []byte {
	var buf bytes.Buffer
	buf.WriteString("<<\n")
	buf.WriteString("/Filter /Standard\n")
	buf.WriteString(fmt.Sprintf("/V %d\n", encrypt.V))
	buf.WriteString(fmt.Sprintf("/R %d\n", encrypt.R))
	buf.WriteString(fmt.Sprintf("/Length %d\n", encrypt.KeyLength*8)) // Length in bits
	buf.WriteString(fmt.Sprintf("/P %d\n", encrypt.P))

	// U value (48 bytes) - use hex string to avoid issues with binary data
	buf.WriteString("/U <")
	for _, b := range encrypt.U {
		buf.WriteString(fmt.Sprintf("%02X", b))
	}
	buf.WriteString(">\n")

	// O value (48 bytes) - use hex string
	buf.WriteString("/O <")
	for _, b := range encrypt.O {
		buf.WriteString(fmt.Sprintf("%02X", b))
	}
	buf.WriteString(">\n")

	// UE value (wrapped user encryption key) - use hex string
	if len(encrypt.UE) > 0 {
		buf.WriteString("/UE <")
		for _, b := range encrypt.UE {
			buf.WriteString(fmt.Sprintf("%02X", b))
		}
		buf.WriteString(">\n")
	}

	// OE value (wrapped user encryption key with owner key) - use hex string
	if len(encrypt.OE) > 0 {
		buf.WriteString("/OE <")
		for _, b := range encrypt.OE {
			buf.WriteString(fmt.Sprintf("%02X", b))
		}
		buf.WriteString(">\n")
	}

	if !encrypt.EncryptMetadata {
		buf.WriteString("/EncryptMetadata false\n")
	}

	buf.WriteString(">>")
	return buf.Bytes()
}
