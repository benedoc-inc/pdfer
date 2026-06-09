package encrypt

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"fmt"

	"github.com/benedoc-inc/pdfer/types"
)

var pdfPadding = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// padPassword pads or truncates p to exactly 32 bytes using the PDF padding string.
func padPassword(p []byte) []byte {
	out := make([]byte, 32)
	n := copy(out, p)
	copy(out[n:], pdfPadding)
	return out
}

// ComputeOValue computes the /O entry for a new Standard encryption dictionary.
// This is Algorithm 3 from ISO 32000-1:2008 §7.6.3.4.
// keyLen is the number of bytes in the encryption key (16 for AES-128).
// revision must be 3 or 4 for the key-lengths this library supports.
func ComputeOValue(ownerPassword, userPassword []byte, keyLen, revision int) ([]byte, error) {
	opwd := ownerPassword
	if len(opwd) == 0 {
		opwd = userPassword
	}

	// Step b: MD5 of padded owner password.
	h := md5.New()
	h.Write(padPassword(opwd))
	key := h.Sum(nil)

	// Step c (R≥3): 50 MD5 iterations.
	if revision >= 3 {
		for i := 0; i < 50; i++ {
			h.Reset()
			h.Write(key[:keyLen])
			key = h.Sum(nil)
		}
	}
	key = key[:keyLen]

	// Step e+f: RC4 encrypt the padded user password.
	data := padPassword(userPassword)
	stream, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream.XORKeyStream(data, data)

	// Step g (R≥3): 19 more RC4 passes with XOR'd key.
	if revision >= 3 {
		for i := 1; i <= 19; i++ {
			xorKey := make([]byte, keyLen)
			for j := range xorKey {
				xorKey[j] = key[j] ^ byte(i)
			}
			stream, err = rc4.NewCipher(xorKey)
			if err != nil {
				return nil, err
			}
			stream.XORKeyStream(data, data)
		}
	}

	return data, nil
}

// EncryptOptions configures PDF encryption for a new document.
type EncryptOptions struct {
	// UserPassword is the password required to open the document.
	// Leave nil or empty for no open-password (anyone can open).
	UserPassword []byte

	// OwnerPassword is the password that grants full control.
	// If empty, UserPassword is used for the owner role as well.
	OwnerPassword []byte

	// Permissions is the P flags bitmask (ISO 32000-1 §7.6.3.2).
	// Default (0) is interpreted as -4 (all permissions, only AES protection).
	Permissions int32
}

// PrepareEncryption computes all parameters needed to add AES-128 (V=4, R=4)
// encryption to a new PDF. It returns the populated encryption struct (with
// EncryptKey set) and a fresh 16-byte file ID.
//
// Pass the result to PDFWriter.SetEncryption and write the /Encrypt dict with
// EncryptDictString.
func PrepareEncryption(opts EncryptOptions) (*types.PDFEncryption, []byte, error) {
	const keyLen = 16 // AES-128

	p := opts.Permissions
	if p == 0 {
		p = -4 // all bits set except reserved bits 1-2
	}

	// Random 16-byte file ID (used in key derivation).
	fileID := make([]byte, 16)
	if _, err := rand.Read(fileID); err != nil {
		return nil, nil, fmt.Errorf("generating file ID: %w", err)
	}

	// /O value.
	oValue, err := ComputeOValue(opts.OwnerPassword, opts.UserPassword, keyLen, 4)
	if err != nil {
		return nil, nil, fmt.Errorf("computing /O: %w", err)
	}

	enc := &types.PDFEncryption{
		V:               4,
		R:               4,
		KeyLength:       keyLen,
		O:               oValue,
		P:               p,
		EncryptMetadata: true,
		// Strings are encrypted alongside streams (/StrF /StdCF). Leaving string
		// values cleartext inside an "encrypted" PDF is a weak default.
		StrFIdentity: false,
	}

	// Derive the file encryption key.
	encKey, err := DeriveEncryptionKey(opts.UserPassword, enc, fileID, false)
	if err != nil {
		return nil, nil, fmt.Errorf("deriving encryption key: %w", err)
	}
	enc.EncryptKey = encKey

	// /U value.
	uValue, err := ComputeUValue(encKey, enc, fileID, false)
	if err != nil {
		return nil, nil, fmt.Errorf("computing /U: %w", err)
	}
	enc.U = uValue

	return enc, fileID, nil
}

// EncryptDictString returns the raw PDF dictionary bytes for the /Encrypt object
// produced by PrepareEncryption. Streams are always AES-128 encrypted via /StmF;
// the declared /StrF is derived from enc.StrFIdentity so the dictionary always
// matches what the write paths actually do with string values.
func EncryptDictString(enc *types.PDFEncryption) []byte {
	oHex := fmt.Sprintf("%X", enc.O)
	uHex := fmt.Sprintf("%X", enc.U)
	strF := "StdCF"
	if enc.StrFIdentity {
		strF = "Identity"
	}
	return []byte(fmt.Sprintf(
		"<</Filter/Standard/V 4/R 4/Length 128/P %d"+
			"/O <%s>/U <%s>"+
			"/EncryptMetadata true"+
			"/CF<</StdCF<</AuthEvent/DocOpen/CFM/AESV2/Length 16>>>>"+
			"/StmF/StdCF/StrF/%s>>",
		enc.P, oHex, uHex, strF,
	))
}
