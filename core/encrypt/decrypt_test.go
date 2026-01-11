package encrypt

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

func TestDecryptObject_RC4(t *testing.T) {
	encrypt := &types.PDFEncryption{
		V:         2,
		R:         3,
		KeyLength: 16,
		EncryptKey: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
	}

	plaintext := []byte("Hello, World!")
	objNum := 1
	genNum := 0

	// Encrypt using RC4
	objKey := deriveObjectKey(encrypt.EncryptKey, objNum, genNum, encrypt.R, encrypt.KeyLength)
	cipher, err := rc4.NewCipher(objKey)
	if err != nil {
		t.Fatalf("Failed to create RC4 cipher: %v", err)
	}
	encrypted := make([]byte, len(plaintext))
	cipher.XORKeyStream(encrypted, plaintext)

	// Decrypt using DecryptObject
	decrypted, err := DecryptObject(encrypted, objNum, genNum, encrypt)
	if err != nil {
		t.Fatalf("DecryptObject() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("DecryptObject() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptObject_AES(t *testing.T) {
	encrypt := &types.PDFEncryption{
		V:         4,
		R:         4,
		KeyLength: 16,
		EncryptKey: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10},
	}

	plaintext := []byte("Hello, World! This is a test message.")
	// Pad to multiple of block size (PKCS#7)
	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	paddedPlaintext := make([]byte, len(plaintext)+padding)
	copy(paddedPlaintext, plaintext)
	for i := len(plaintext); i < len(paddedPlaintext); i++ {
		paddedPlaintext[i] = byte(padding)
	}

	objNum := 1
	genNum := 0

	// Derive object-specific key (must match DecryptObject's algorithm)
	// This is the AES key derivation: hash(key[:n] + objNum + genNum + "sAlT")
	n := encrypt.KeyLength
	keyData := make([]byte, n+5)
	copy(keyData, encrypt.EncryptKey[:n])
	keyData[n] = byte(objNum & 0xff)
	keyData[n+1] = byte((objNum >> 8) & 0xff)
	keyData[n+2] = byte((objNum >> 16) & 0xff)
	keyData[n+3] = byte(genNum & 0xff)
	keyData[n+4] = byte((genNum >> 8) & 0xff)
	
	keyHash := md5.New()
	keyHash.Write(keyData)
	keyHash.Write([]byte{0x73, 0x41, 0x6C, 0x54}) // "sAlT"
	aesKeyHash := keyHash.Sum(nil)
	aesKeyLen := n + 5
	if aesKeyLen > 16 {
		aesKeyLen = 16
	}
	aesKey := aesKeyHash[:aesKeyLen]

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatalf("Failed to create AES cipher: %v", err)
	}

	// Generate IV (use a fixed IV for reproducibility)
	iv := make([]byte, aes.BlockSize)
	for i := range iv {
		iv[i] = byte(i + 1)
	}

	// Encrypt using AES-CBC
	mode := cipher.NewCBCEncrypter(block, iv)
	encryptedData := make([]byte, len(paddedPlaintext))
	mode.CryptBlocks(encryptedData, paddedPlaintext)

	// PDF AES format: IV prepended to encrypted data
	encrypted := make([]byte, aes.BlockSize+len(encryptedData))
	copy(encrypted[:aes.BlockSize], iv)
	copy(encrypted[aes.BlockSize:], encryptedData)

	// Decrypt using DecryptObject
	decrypted, err := DecryptObject(encrypted, objNum, genNum, encrypt)
	if err != nil {
		t.Fatalf("DecryptObject() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("DecryptObject() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptObject_NoEncryption(t *testing.T) {
	plaintext := []byte("Hello, World!")

	// No encryption
	decrypted, err := DecryptObject(plaintext, 1, 0, nil)
	if err != nil {
		t.Fatalf("DecryptObject() error = %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("DecryptObject() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptPDFObjects(t *testing.T) {
	pdfBytes := []byte("%PDF-1.4\n")
	encrypt := &types.PDFEncryption{
		EncryptKey: []byte{0x01, 0x02, 0x03},
	}

	// Should return original bytes (placeholder implementation)
	result, err := DecryptPDFObjects(pdfBytes, encrypt, false)
	if err != nil {
		t.Fatalf("DecryptPDFObjects() error = %v", err)
	}

	if !bytes.Equal(result, pdfBytes) {
		t.Errorf("DecryptPDFObjects() = %q, want %q", result, pdfBytes)
	}
}

// deriveObjectKey is a helper function for tests to derive object key
func deriveObjectKey(encryptKey []byte, objNum, genNum, r, keyLength int) []byte {
	key := make([]byte, len(encryptKey)+5)
	copy(key, encryptKey)
	key[len(encryptKey)] = byte(objNum & 0xFF)
	key[len(encryptKey)+1] = byte((objNum >> 8) & 0xFF)
	key[len(encryptKey)+2] = byte((objNum >> 16) & 0xFF)
	key[len(encryptKey)+3] = byte(genNum & 0xFF)
	key[len(encryptKey)+4] = byte((genNum >> 8) & 0xFF)

	hash := md5.New()
	hash.Write(key)
	objKey := hash.Sum(nil)

	if r >= 3 {
		if len(objKey) > keyLength {
			objKey = objKey[:keyLength]
		}
	} else {
		if len(objKey) > 5 {
			objKey = objKey[:5]
		}
	}

	return objKey
}
