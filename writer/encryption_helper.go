package writer

import (
	"crypto/rand"
	"fmt"
)

// SetupEncryptionWithPasswords is a convenience function that sets up encryption
// and creates the encryption dictionary object
// Returns the encryption dictionary object number
func (w *PDFWriter) SetupEncryptionWithPasswords(userPassword, ownerPassword []byte, permissions int32, encryptMetadata bool) (int, error) {
	// Generate file ID if not already set
	if len(w.fileID) == 0 {
		fileID := make([]byte, 16)
		if _, err := rand.Read(fileID); err != nil {
			return 0, fmt.Errorf("failed to generate file ID: %v", err)
		}
		w.fileID = fileID
	}

	// Setup AES-256 encryption
	encrypt, err := SetupAES256Encryption(userPassword, ownerPassword, w.fileID, permissions, encryptMetadata)
	if err != nil {
		return 0, fmt.Errorf("failed to setup encryption: %v", err)
	}

	// Create encryption dictionary object
	encryptDict := CreateEncryptionDictionary(encrypt)
	encryptObjNum := w.AddObject(encryptDict)
	w.SetEncryptRef(encryptObjNum)
	w.SetEncryption(encrypt, w.fileID)

	return encryptObjNum, nil
}
