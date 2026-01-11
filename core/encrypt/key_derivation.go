package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"crypto/rc4"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// DeriveEncryptionKey derives the encryption key from password
// Based on PDF encryption algorithm (ISO 32000) - Algorithm 2 (V1-V4) or 7.6.4.3.3 (V5+)
func DeriveEncryptionKey(password []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	// For revision 5+ (AES-256), use SHA-256 based key derivation
	if encrypt.R >= 5 {
		return DeriveEncryptionKeyV5(password, encrypt, fileID, verbose)
	}
	// Pad or truncate password to 32 bytes
	// According to PDF spec Algorithm 2:
	// - If password is shorter than 32 bytes, pad by appending bytes from the password cyclically
	// - For empty password, the padding string itself is used
	paddingString := []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}

	paddedPassword := make([]byte, 32)
	if len(password) == 0 {
		// Empty password: use padding string directly
		copy(paddedPassword, paddingString)
	} else {
		copy(paddedPassword, password)
		if len(password) < 32 {
			// Pad with password bytes cyclically
			for i := len(password); i < 32; i++ {
				paddedPassword[i] = password[i%len(password)]
			}
		}
	}

	// Step 1: Compute hash of padded password + O + P (as 32-bit int, little-endian) + ID[0]
	hash := md5.New()
	hash.Write(paddedPassword)
	hash.Write(encrypt.O)

	// P as 32-bit unsigned integer, little-endian
	pBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(pBytes, uint32(encrypt.P))
	hash.Write(pBytes)

	// ID[0] - first element of file ID
	// Use the full fileID (it's already the first element)
	if len(fileID) > 0 {
		hash.Write(fileID)
		if verbose {
			log.Printf("Using file ID in key derivation: %d bytes, hex: %x", len(fileID), fileID[:min(16, len(fileID))])
		}
	} else {
		if verbose {
			log.Printf("Warning: No file ID, using empty in key derivation")
		}
	}

	// If EncryptMetadata is false, add 4 bytes of 0xFFFFFFFF
	if !encrypt.EncryptMetadata {
		metadataBytes := []byte{0xFF, 0xFF, 0xFF, 0xFF}
		hash.Write(metadataBytes)
		if verbose {
			log.Printf("Added EncryptMetadata=false flag to key derivation")
		}
	}

	key := hash.Sum(nil)

	// Step 2: For revision 3+, iterate 50 times
	if encrypt.R >= 3 {
		for i := 0; i < 50; i++ {
			hash := md5.New()
			hash.Write(key[:encrypt.KeyLength])
			key = hash.Sum(nil)
		}
	}

	// Truncate to key length
	if len(key) > encrypt.KeyLength {
		key = key[:encrypt.KeyLength]
	}

	return key, nil
}

// ComputeUValue computes the U value for password verification
// For revision 4 (AES), the U value is 48 bytes: 32-byte hash + 16-byte validation salt
// Algorithm 5 from ISO 32000-1:2008
func ComputeUValue(encryptKey []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	if encrypt.R == 2 {
		// Revision 2: RC4 encrypt padding string
		padding := []byte{
			0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
			0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
			0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
			0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
		}

		cipher, err := rc4.NewCipher(encryptKey)
		if err != nil {
			return nil, err
		}

		encrypted := make([]byte, 32)
		cipher.XORKeyStream(encrypted, padding)
		return encrypted, nil
	} else if encrypt.R >= 3 && encrypt.R < 5 {
		// Revision 3-4: MD5 hash of padding + file ID, then RC4 encrypt
		// Algorithm 5 from ISO 32000-1:2008
		padding := []byte{
			0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
			0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
			0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
			0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
		}

		hash := md5.New()
		hash.Write(padding)
		if len(fileID) > 0 {
			hash.Write(fileID)
			if verbose {
				log.Printf("Using file ID in U computation: %d bytes", len(fileID))
			}
		} else {
			if verbose {
				log.Printf("Warning: No file ID found, using empty file ID")
			}
		}
		hashed := hash.Sum(nil)

		if verbose {
			log.Printf("MD5 hash of padding+fileID: %x", hashed)
		}

		// Encrypt the 16-byte hash with RC4
		cipher, err := rc4.NewCipher(encryptKey)
		if err != nil {
			return nil, err
		}

		encrypted := make([]byte, 16)
		cipher.XORKeyStream(encrypted, hashed[:16])

		if verbose {
			log.Printf("After first RC4 encryption: %x", encrypted)
		}

		// For revision 3-4, do 19 additional iterations (not 16)
		// Each iteration: XOR key with iteration number (1-19), then encrypt
		for i := 1; i <= 19; i++ {
			newKey := make([]byte, len(encryptKey))
			for j := 0; j < len(encryptKey); j++ {
				newKey[j] = encryptKey[j] ^ byte(i)
			}
			cipher, _ := rc4.NewCipher(newKey)
			cipher.XORKeyStream(encrypted, encrypted)
		}

		if verbose {
			log.Printf("After 19 RC4 iterations: %x", encrypted)
		}

		// Append 16 bytes of arbitrary padding to make 32 bytes total
		// (PDF spec says to append padding, but for verification we only need first 16)
		result := make([]byte, 32)
		copy(result, encrypted)
		// Padding bytes can be arbitrary - we'll just use zeros

		return result, nil
	} else if encrypt.R >= 5 {
		// Revision 5+: SHA-256 based (V5/R5/R6)
		// Note: For V5, ComputeUValue requires the actual password, not just the key
		// This function signature doesn't have password, so we return an error
		// The caller should use VerifyUValueV5 directly for V5 verification
		return nil, fmt.Errorf("V5 U value computation requires password; use VerifyUValueV5 instead")
	} else {
		return nil, fmt.Errorf("unsupported revision: %d", encrypt.R)
	}
}

// DeriveOwnerKey derives the encryption key from owner password
// Algorithm 3 from ISO 32000-1:2008 (V1-V4) or V5 algorithm (V5+)
func DeriveOwnerKey(ownerPassword []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	// For revision 5+, use V5 algorithm
	if encrypt.R >= 5 {
		return DeriveOwnerKeyV5(ownerPassword, encrypt, fileID, verbose)
	}
	// Pad owner password to 32 bytes
	paddingString := []byte{
		0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
		0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
		0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
		0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
	}

	paddedOwnerPassword := make([]byte, 32)
	if len(ownerPassword) > 0 {
		copy(paddedOwnerPassword, ownerPassword)
		if len(ownerPassword) < 32 {
			for i := len(ownerPassword); i < 32; i++ {
				paddedOwnerPassword[i] = ownerPassword[i%len(ownerPassword)]
			}
		}
	} else {
		// Empty password - use padding string
		copy(paddedOwnerPassword, paddingString)
	}

	// MD5 hash of padded owner password
	hash := md5.New()
	hash.Write(paddedOwnerPassword)
	ownerHash := hash.Sum(nil)

	// For revision 3+, iterate 50 times
	if encrypt.R >= 3 {
		for i := 0; i < 50; i++ {
			hash := md5.New()
			hash.Write(ownerHash[:encrypt.KeyLength])
			ownerHash = hash.Sum(nil)
		}
	}

	// Truncate to key length
	if len(ownerHash) > encrypt.KeyLength {
		ownerHash = ownerHash[:encrypt.KeyLength]
	}

	// Decrypt O value using owner key
	// O value is encrypted with RC4 using owner key
	decryptedO := make([]byte, len(encrypt.O))
	if encrypt.R == 2 {
		cipher, err := rc4.NewCipher(ownerHash)
		if err != nil {
			return nil, err
		}
		cipher.XORKeyStream(decryptedO, encrypt.O)
	} else {
		// Revision 3+: decrypt O with RC4, then use it to derive user key
		cipher, err := rc4.NewCipher(ownerHash)
		if err != nil {
			return nil, err
		}
		cipher.XORKeyStream(decryptedO, encrypt.O[:min(32, len(encrypt.O))])
	}

	// Now derive user key from decrypted O (which is the user password hash)
	// Use decrypted O as if it were the user password
	return DeriveEncryptionKey(decryptedO, encrypt, fileID, verbose)
}

// DeriveUserKeyFromOwner derives the user encryption key from owner key
func DeriveUserKeyFromOwner(ownerKey []byte, encrypt *types.PDFEncryption, verbose bool) ([]byte, error) {
	// The owner key approach: decrypt O value, then use it to derive user key
	// This is a simplified version - full implementation would follow Algorithm 3

	// For now, return ownerKey as userKey (this needs proper implementation)
	return ownerKey, nil
}

// ExtractFileID extracts the file ID from PDF trailer
// Returns the first element of the ID array (ID[0])
// File ID can be in hex format: <7FB157EB...> or binary: (binary data)
func ExtractFileID(pdfBytes []byte, verbose bool) []byte {
	pdfStr := string(pdfBytes)

	// Find /ID in trailer - format: /ID [ <hex1> <hex2> ] or /ID [ (binary1) (binary2) ]
	// Try hex format first: /ID[<hex><hex>] or /ID [<hex><hex>]
	hexIDPattern := regexp.MustCompile(`/ID\s*\[\s*<([0-9A-Fa-f]+)>`)
	hexMatch := hexIDPattern.FindStringSubmatch(pdfStr)
	if hexMatch != nil {
		// Decode hex string
		hexStr := hexMatch[1]
		fileID := parseHexString(hexStr)
		if len(fileID) > 0 {
			if verbose {
				log.Printf("Extracted file ID[0] (hex): %d bytes, hex: %x", len(fileID), fileID)
			}
			return fileID
		}
	}

	// Try alternative pattern without space: /ID[<hex>
	hexIDPattern2 := regexp.MustCompile(`/ID\[<([0-9A-Fa-f]+)>`)
	hexMatch2 := hexIDPattern2.FindStringSubmatch(pdfStr)
	if hexMatch2 != nil {
		hexStr := hexMatch2[1]
		fileID := parseHexString(hexStr)
		if len(fileID) > 0 {
			if verbose {
				log.Printf("Extracted file ID[0] (hex, alt pattern): %d bytes, hex: %x", len(fileID), fileID)
			}
			return fileID
		}
	}

	// Try binary format: /ID [ (binary1) (binary2) ]
	idPattern := regexp.MustCompile(`/ID\s*\[\s*\(`)
	match := idPattern.FindStringIndex(pdfStr)
	if match != nil {
		// Find the first binary string in parentheses
		idStart := match[1] - 1 // Position of '('
		parenStart := bytes.Index(pdfBytes[idStart:], []byte("("))
		if parenStart != -1 {
			parenStart += idStart + 1
			parenEnd := bytes.Index(pdfBytes[parenStart:], []byte(")"))
			if parenEnd != -1 {
				fileID := pdfBytes[parenStart : parenStart+parenEnd]
				if verbose {
					log.Printf("Extracted file ID[0] (binary): %d bytes, hex: %x", len(fileID), fileID[:min(16, len(fileID))])
				}
				return fileID
			}
		}
	}

	if verbose {
		log.Printf("No file ID found in trailer")
	}

	// Return empty if not found (some PDFs work without file ID)
	return []byte{}
}

// parseHexString parses a hex string like "0123456789ABCDEF" to bytes
func parseHexString(hexStr string) []byte {
	// Remove whitespace
	hexStr = strings.ReplaceAll(hexStr, " ", "")
	hexStr = strings.ReplaceAll(hexStr, "\n", "")
	hexStr = strings.ReplaceAll(hexStr, "\r", "")

	result := make([]byte, 0, len(hexStr)/2)
	for i := 0; i < len(hexStr)-1; i += 2 {
		val, err := strconv.ParseUint(hexStr[i:i+2], 16, 8)
		if err != nil {
			continue
		}
		result = append(result, byte(val))
	}
	return result
}

// DeriveEncryptionKeyV5 derives the encryption key for V5/R5/R6 (AES-256)
// Based on ISO 32000-2 section 7.6.4.3.3 - SHA-256 based key derivation
func DeriveEncryptionKeyV5(password []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	// Step 1: Prepare file ID (use first 8 bytes, or pad/truncate to 8 bytes)
	fileID8 := make([]byte, 8)
	if len(fileID) >= 8 {
		copy(fileID8, fileID[:8])
	} else if len(fileID) > 0 {
		copy(fileID8, fileID)
		// Pad with zeros if needed
	} else {
		// No file ID - use zeros
		if verbose {
			log.Printf("Warning: No file ID for V5 key derivation, using zeros")
		}
	}

	// Step 2: Concatenate password (UTF-8) with 8-byte file ID
	// Password should be UTF-8 encoded (already is for byte slice)
	input := make([]byte, len(password)+8)
	copy(input, password)
	copy(input[len(password):], fileID8)

	if verbose {
		log.Printf("V5 key derivation: password=%d bytes, fileID=8 bytes", len(password))
	}

	// Step 3: Compute SHA-256 hash
	hash := sha256.Sum256(input)
	key := hash[:]

	// Step 4: Iterate 64 times with SHA-256
	for i := 0; i < 64; i++ {
		hash := sha256.Sum256(key)
		key = hash[:]
	}

	if verbose {
		log.Printf("V5 key derivation: final key length=%d bytes (AES-256)", len(key))
	}

	// Key is always 32 bytes for AES-256
	return key, nil
}

// UnwrapUserKeyV5 unwraps the user encryption key from /UE using the password-derived key
// Based on ISO 32000-2 - uses AES-128 in ECB mode for key unwrapping
func UnwrapUserKeyV5(passwordKey []byte, encrypt *types.PDFEncryption, verbose bool) ([]byte, error) {
	if len(encrypt.UE) == 0 {
		return nil, fmt.Errorf("UE (encrypted user key) not found")
	}

	if len(passwordKey) < 16 {
		return nil, fmt.Errorf("password key too short: %d bytes (need at least 16 for AES-128)", len(passwordKey))
	}

	// UE is encrypted with AES-128 in ECB mode using the password-derived key
	// The key for unwrapping is the first 16 bytes of the password-derived key
	unwrapKey := passwordKey[:16]

	// Create AES cipher
	block, err := aes.NewCipher(unwrapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	// ECB mode - decrypt each 16-byte block independently
	if len(encrypt.UE)%16 != 0 {
		return nil, fmt.Errorf("UE length (%d) is not a multiple of 16", len(encrypt.UE))
	}

	decrypted := make([]byte, len(encrypt.UE))
	for i := 0; i < len(encrypt.UE); i += 16 {
		block.Decrypt(decrypted[i:], encrypt.UE[i:i+16])
	}

	// Remove PKCS#7 padding
	if len(decrypted) == 0 {
		return nil, fmt.Errorf("decrypted UE is empty")
	}

	paddingLen := int(decrypted[len(decrypted)-1])
	if paddingLen > len(decrypted) || paddingLen > 16 {
		return nil, fmt.Errorf("invalid padding length: %d", paddingLen)
	}

	unwrappedKey := decrypted[:len(decrypted)-paddingLen]

	if verbose {
		log.Printf("Unwrapped user key: %d bytes", len(unwrappedKey))
	}

	return unwrappedKey, nil
}

// UnwrapOwnerKeyV5 unwraps the owner encryption key from /OE
// Similar to UnwrapUserKeyV5 but uses owner password derived key
func UnwrapOwnerKeyV5(ownerPasswordKey []byte, encrypt *types.PDFEncryption, verbose bool) ([]byte, error) {
	if len(encrypt.OE) == 0 {
		return nil, fmt.Errorf("OE (encrypted owner key) not found")
	}

	if len(ownerPasswordKey) < 16 {
		return nil, fmt.Errorf("owner password key too short: %d bytes (need at least 16 for AES-128)", len(ownerPasswordKey))
	}

	// OE is encrypted with AES-128 in ECB mode
	unwrapKey := ownerPasswordKey[:16]

	block, err := aes.NewCipher(unwrapKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	if len(encrypt.OE)%16 != 0 {
		return nil, fmt.Errorf("OE length (%d) is not a multiple of 16", len(encrypt.OE))
	}

	decrypted := make([]byte, len(encrypt.OE))
	for i := 0; i < len(encrypt.OE); i += 16 {
		block.Decrypt(decrypted[i:], encrypt.OE[i:i+16])
	}

	// Remove PKCS#7 padding
	if len(decrypted) == 0 {
		return nil, fmt.Errorf("decrypted OE is empty")
	}

	paddingLen := int(decrypted[len(decrypted)-1])
	if paddingLen > len(decrypted) || paddingLen > 16 {
		return nil, fmt.Errorf("invalid padding length: %d", paddingLen)
	}

	unwrappedKey := decrypted[:len(decrypted)-paddingLen]

	if verbose {
		log.Printf("Unwrapped owner key: %d bytes", len(unwrappedKey))
	}

	return unwrappedKey, nil
}

// DeriveOwnerKeyV5 derives the owner password key for V5/R5/R6
// Similar to DeriveEncryptionKeyV5 but uses owner password
func DeriveOwnerKeyV5(ownerPassword []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	// Same algorithm as user password, but with owner password
	return DeriveEncryptionKeyV5(ownerPassword, encrypt, fileID, verbose)
}

// ComputeUValueV5 computes the U value for V5/R5/R6 password verification
// Based on ISO 32000-2 section 7.6.4.4.9 - uses SHA-256 and AES-128
// This function computes the U value from a password for verification purposes.
// The actual password (not the derived key) is required for proper verification.
func ComputeUValueV5(password []byte, passwordKey []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
	// For V5, U value structure (48 bytes total):
	// - Bytes 0-7: Validation salt (8 bytes)
	// - Bytes 8-39: Encrypted hash (32 bytes) - SHA-256(password + validation_salt) encrypted with AES-128 ECB
	// - Bytes 40-47: User key salt (8 bytes) - used for deriving user encryption key

	if len(encrypt.U) < 48 {
		return nil, fmt.Errorf("U value too short for V5: %d bytes (expected at least 48)", len(encrypt.U))
	}

	// Extract validation salt (first 8 bytes)
	validationSalt := encrypt.U[:8]

	// Step 1: Compute SHA-256(password + validation_salt)
	// Password should be UTF-8 encoded (already is for byte slice)
	hash := sha256.New()
	hash.Write(password)
	hash.Write(validationSalt)
	hashed := hash.Sum(nil)

	if verbose {
		log.Printf("V5 U computation: password=%d bytes, validation_salt=%x, hash=%x", len(password), validationSalt, hashed)
	}

	// Step 2: Encrypt the hash with AES-128 ECB using first 16 bytes of password-derived key
	if len(passwordKey) < 16 {
		return nil, fmt.Errorf("password key too short: %d bytes (need at least 16 for AES-128)", len(passwordKey))
	}

	encryptKey := passwordKey[:16]
	block, err := aes.NewCipher(encryptKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %v", err)
	}

	// Encrypt hash (32 bytes) in 2 blocks of 16 bytes each (ECB mode)
	encrypted := make([]byte, 32)
	block.Encrypt(encrypted[:16], hashed[:16])
	block.Encrypt(encrypted[16:], hashed[16:])

	// Step 3: Construct U value: validation_salt (8) + encrypted_hash (32) + user_key_salt (8) = 48 bytes
	result := make([]byte, 48)
	copy(result[:8], validationSalt)
	copy(result[8:40], encrypted)
	// User key salt is already in encrypt.U[40:48], copy it
	if len(encrypt.U) >= 48 {
		copy(result[40:48], encrypt.U[40:48])
	}

	if verbose {
		log.Printf("V5 U value computed: validation_salt=%x, encrypted_hash=%x, user_key_salt=%x",
			result[:8], result[8:40], result[40:48])
	}

	return result, nil
}

// VerifyUValueV5 verifies a password by comparing computed U value with stored U value
// Returns true if password is correct, false otherwise
func VerifyUValueV5(password []byte, passwordKey []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) (bool, error) {
	if len(encrypt.U) < 48 {
		return false, fmt.Errorf("U value too short for V5: %d bytes (expected at least 48)", len(encrypt.U))
	}

	// Compute U value from password
	computedU, err := ComputeUValueV5(password, passwordKey, encrypt, fileID, verbose)
	if err != nil {
		return false, err
	}

	// Compare encrypted hash portion (bytes 8-39) - this is the actual verification
	// The validation salt and user key salt are stored values, not computed
	storedEncryptedHash := encrypt.U[8:40]
	computedEncryptedHash := computedU[8:40]

	match := bytes.Equal(storedEncryptedHash, computedEncryptedHash)

	if verbose {
		if match {
			log.Printf("V5 U value verification: PASSED")
		} else {
			log.Printf("V5 U value verification: FAILED")
			log.Printf("  Stored encrypted hash:   %x", storedEncryptedHash)
			log.Printf("  Computed encrypted hash: %x", computedEncryptedHash)
		}
	}

	return match, nil
}
