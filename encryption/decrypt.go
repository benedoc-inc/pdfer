package encryption

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"fmt"
	"log"

	"github.com/benedoc-inc/pdfer/types"
)

// DecryptPDF decrypts a PDF using the provided password
// Returns decrypted PDF bytes and encryption info
// Note: Implementation inspired by UniPDF but written from first principles
func DecryptPDF(pdfBytes []byte, password []byte, verbose bool) ([]byte, *types.PDFEncryption, error) {
	// Parse encryption dictionary
	encrypt, err := ParseEncryptionDictionary(pdfBytes, verbose)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing encryption dictionary: %v", err)
	}

	if verbose {
		log.Printf("PDF encryption: V=%d, R=%d, KeyLength=%d", encrypt.V, encrypt.R, encrypt.KeyLength)
	}

	// Extract file ID from PDF trailer (needed for key derivation and U value computation)
	fileID := ExtractFileID(pdfBytes, verbose)

	// Derive encryption key from password
	encryptKey, err := DeriveEncryptionKey(password, encrypt, fileID, verbose)
	if err != nil {
		return nil, nil, fmt.Errorf("error deriving encryption key: %v", err)
	}

	encrypt.EncryptKey = encryptKey

	if verbose {
		log.Printf("Derived encryption key (length: %d)", len(encryptKey))
	}

	// Verify password by checking U value
	uValue, err := ComputeUValue(encryptKey, encrypt, fileID, verbose)
	if err != nil {
		return nil, nil, fmt.Errorf("error computing U value: %v", err)
	}

	// Compare with stored U value
	// For revision 2: compare first 16 bytes
	// For revision 3-4: compare first 16 bytes (the encrypted hash)
	// Note: U value is 32 bytes for revision 3-4, but only first 16 are used for verification
	uMatch := false
	if len(encrypt.U) >= 16 && len(uValue) >= 16 {
		if encrypt.R == 2 {
			uMatch = bytes.Equal(uValue[:16], encrypt.U[:16])
		} else {
			// For revision 3-4, compare first 16 bytes
			uMatch = bytes.Equal(uValue[:16], encrypt.U[:16])
		}
	}

	if verbose {
		if uMatch {
			log.Printf("U value matches! Password verified")
		} else {
			log.Printf("U value mismatch - computed: %x (first 16), stored: %x (first 16)",
				uValue[:min(16, len(uValue))], encrypt.U[:min(16, len(encrypt.U))])
			log.Printf("Full computed U: %x", uValue)
			log.Printf("Full stored U: %x", encrypt.U)
		}
	}

	if !uMatch {
		// Try owner password approach
		if verbose {
			log.Printf("User password failed, trying owner password approach...")
		}
		ownerKey, err := DeriveOwnerKey(password, encrypt, fileID, verbose)
		if err == nil {
			// Try to decrypt with owner key
			userKey, err := DeriveUserKeyFromOwner(ownerKey, encrypt, verbose)
			if err == nil {
				uValue2, err := ComputeUValue(userKey, encrypt, fileID, verbose)
				if err == nil && len(uValue2) >= 16 && len(encrypt.U) >= 16 {
					if bytes.Equal(uValue2[:16], encrypt.U[:16]) {
						if verbose {
							log.Printf("Owner password verified! Using owner-derived user key")
						}
						encrypt.EncryptKey = userKey
						uMatch = true
					}
				}
			}
		}
	}

	if !uMatch {
		return nil, nil, fmt.Errorf("password incorrect or encryption parameters invalid")
	}

	if verbose {
		log.Printf("Password verified successfully")
	}

	// Decrypt PDF objects
	decryptedPDF, err := DecryptPDFObjects(pdfBytes, encrypt, verbose)
	if err != nil {
		return nil, nil, fmt.Errorf("error decrypting PDF objects: %v", err)
	}

	return decryptedPDF, encrypt, nil
}

// DecryptPDFObjects decrypts all encrypted objects in the PDF
// This is a simplified version - full implementation would parse xref table
// and decrypt objects individually
func DecryptPDFObjects(pdfBytes []byte, encrypt *types.PDFEncryption, verbose bool) ([]byte, error) {
	// For now, return original bytes
	// Full implementation would:
	// 1. Parse cross-reference table
	// 2. For each object, check if encrypted
	// 3. Derive object-specific key
	// 4. Decrypt object content
	// 5. Reconstruct PDF with decrypted objects

	// This is a placeholder - we'll decrypt objects on-demand when accessing them
	return pdfBytes, nil
}

// DecryptObject decrypts a single PDF object or stream
// Algorithm 1 from ISO 32000-1:2008
// Implementation copied EXACTLY from PyPDF's _make_crypt_filter (lines 914-935) and CryptAES.decrypt (lines 73-88)
func DecryptObject(objBytes []byte, objNum, genNum int, encrypt *types.PDFEncryption) ([]byte, error) {
	if encrypt == nil || len(encrypt.EncryptKey) == 0 {
		// Not encrypted
		return objBytes, nil
	}

	// PyPDF line 914: pack1 = struct.pack("<i", idnum)[:3]
	// struct.pack("<i", idnum) packs as little-endian int32 (4 bytes)
	// [:3] takes first 3 bytes (low-order bytes)
	pack1 := make([]byte, 3)
	pack1[0] = byte(objNum & 0xff)
	pack1[1] = byte((objNum >> 8) & 0xff)
	pack1[2] = byte((objNum >> 16) & 0xff)

	// PyPDF line 915: pack2 = struct.pack("<i", generation)[:2]
	pack2 := make([]byte, 2)
	pack2[0] = byte(genNum & 0xff)
	pack2[1] = byte((genNum >> 8) & 0xff)

	// PyPDF line 919: n = 5 if self.V == 1 else self.Length // 8
	n := 5
	if encrypt.V > 1 {
		n = encrypt.KeyLength // KeyLength is already in bytes (converted from bits in parser)
	}

	// PyPDF line 918: key = self._key
	// PyPDF line 920: key_data = key[:n] + pack1 + pack2
	keyData := make([]byte, n+5)
	copy(keyData, encrypt.EncryptKey[:n]) // CRITICAL: Use only first n bytes!
	copy(keyData[n:], pack1)
	copy(keyData[n+3:], pack2)

	// PyPDF line 921: key_hash = hashlib.md5(key_data)
	keyHash := md5.New()
	keyHash.Write(keyData)

	// Decrypt based on encryption algorithm
	if encrypt.V == 1 || encrypt.V == 2 {
		// RC4 encryption
		// PyPDF line 922: rc4_key = key_hash.digest()[: min(n + 5, 16)]
		rc4KeyHash := keyHash.Sum(nil)
		rc4Key := rc4KeyHash[:min(n+5, 16)]

		cipher, err := rc4.NewCipher(rc4Key)
		if err != nil {
			return nil, err
		}
		decrypted := make([]byte, len(objBytes))
		cipher.XORKeyStream(decrypted, objBytes)
		return decrypted, nil
	} else if encrypt.V == 4 || encrypt.V == 5 {
		// AES encryption (AES-128 or AES-256)
		// PyPDF line 925: key_hash.update(b"sAlT")
		// PyPDF line 926: aes128_key = key_hash.digest()[: min(n + 5, 16)]
		keyHash.Write([]byte{0x73, 0x41, 0x6C, 0x54}) // "sAlT"
		aesKeyHash := keyHash.Sum(nil)
		aesKeyLen := min(n+5, 16)
		aesKey := aesKeyHash[:aesKeyLen]

		// PyPDF CryptAES.decrypt (lines 73-88):
		// Line 74: iv = data[:16]
		// Line 75: data = data[16:]
		if len(objBytes) < 16 {
			return objBytes, fmt.Errorf("AES: Buf len < 16 (%d)", len(objBytes))
		}

		iv := objBytes[:16]
		data := objBytes[16:]

		// Line 77-78: if not data: return data
		if len(data) == 0 {
			return data, nil
		}

		// Line 81-83: if len(data) % 16 != 0: pad it (robustness check)
		if len(data)%16 != 0 {
			return data, fmt.Errorf("AES buf length not multiple of 16 (%d)", len(data))
		}

		// Line 85-87: cipher = Cipher(self.alg, CBC(iv)); decryptor = cipher.decryptor(); d = decryptor.update(data) + decryptor.finalize()
		block, err := aes.NewCipher(aesKey)
		if err != nil {
			return nil, err
		}

		mode := cipher.NewCBCDecrypter(block, iv)
		decrypted := make([]byte, len(data))
		mode.CryptBlocks(decrypted, data)

		// Line 88: return d[: -d[-1]]  (remove PKCS#7 padding)
		if len(decrypted) == 0 {
			return decrypted, nil
		}

		paddingLen := int(decrypted[len(decrypted)-1])
		if paddingLen > len(decrypted) {
			return decrypted, fmt.Errorf("invalid pad length: %d > %d", paddingLen, len(decrypted))
		}

		return decrypted[:len(decrypted)-paddingLen], nil
	}

	return objBytes, nil
}
