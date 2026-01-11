package encryption

import (
	"bytes"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// DeriveEncryptionKey derives the encryption key from password
// Based on PDF encryption algorithm (ISO 32000) - Algorithm 2
func DeriveEncryptionKey(password []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
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
			log.Printf("Using file ID in key derivation: %d bytes, hex: %x", len(fileID), fileID[:types.Min(16, len(fileID))])
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
	} else {
		// Revision 5+ uses different algorithm (SHA-256 based)
		// For now, return error
		return nil, fmt.Errorf("revision %d not yet supported", encrypt.R)
	}
}

// DeriveOwnerKey derives the encryption key from owner password
// Algorithm 3 from ISO 32000-1:2008
func DeriveOwnerKey(ownerPassword []byte, encrypt *types.PDFEncryption, fileID []byte, verbose bool) ([]byte, error) {
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
		cipher.XORKeyStream(decryptedO, encrypt.O[:types.Min(32, len(encrypt.O))])
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
					log.Printf("Extracted file ID[0] (binary): %d bytes, hex: %x", len(fileID), fileID[:types.Min(16, len(fileID))])
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
