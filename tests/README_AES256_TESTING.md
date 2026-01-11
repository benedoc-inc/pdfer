# Testing AES-256 (V5/R5/R6) Encryption

This document explains how to test the AES-256 encryption implementation with real PDF documents.

## Quick Start

### Option 1: Use qpdf to Create Test PDFs

If you have `qpdf` installed, the test will automatically create an AES-256 encrypted PDF:

```bash
# Install qpdf (if not already installed)
# macOS:
brew install qpdf

# Linux:
sudo apt-get install qpdf

# Run the test - it will create the PDF automatically
go test ./tests -run TestE2E_AES256_CreateAndDecrypt -v
```

### Option 2: Create Test PDF Manually

1. Create a simple unencrypted PDF (or use any existing PDF)

2. Encrypt it with AES-256 using qpdf:
```bash
qpdf --encrypt user-password owner-password 256 -- input.pdf tests/resources/test_aes256.pdf
```

Replace:
- `user-password`: The user password (e.g., "testpass")
- `owner-password`: The owner password (e.g., "ownerpass")
- `input.pdf`: Your source PDF file
- `tests/resources/test_aes256.pdf`: Output path

3. Run the test:
```bash
go test ./tests -run TestE2E_AES256_Decrypt -v
```

### Option 3: Use Adobe Acrobat

1. Open a PDF in Adobe Acrobat
2. Go to File > Properties > Security
3. Select "Password Security"
4. Choose "Encrypt all document contents"
5. Set "Compatibility" to "Acrobat 7.0 and later" (this uses AES-256)
6. Set user and owner passwords
7. Save the PDF

Place it in `tests/resources/test_aes256.pdf` and run the test.

## Test Functions

### `TestE2E_AES256_Decrypt`
Tests decrypting an existing AES-256 encrypted PDF from `tests/resources/test_aes256.pdf`.

**Requirements:**
- A PDF file at `tests/resources/test_aes256.pdf` encrypted with AES-256
- Default password: "testpass" (or empty password)

**Usage:**
```bash
go test ./tests -run TestE2E_AES256_Decrypt -v
```

### `TestE2E_AES256_CreateAndDecrypt`
Automatically creates an AES-256 encrypted PDF using qpdf, then tests decryption.

**Requirements:**
- `qpdf` must be installed and available in PATH

**Usage:**
```bash
go test ./tests -run TestE2E_AES256_CreateAndDecrypt -v
```

### `TestE2E_AES256_VerifyUValue`
Tests U value verification specifically (password validation).

**Requirements:**
- Same as `TestE2E_AES256_Decrypt`

**Usage:**
```bash
go test ./tests -run TestE2E_AES256_VerifyUValue -v
```

## Verifying Encryption Type

To verify a PDF uses AES-256 (V5/R5/R6):

```bash
# Using qpdf
qpdf --show-encryption test.pdf

# Look for:
# - Encryption method: AES-256
# - PDF version: 1.7 or higher
# - Encryption version (V): 5
# - Revision (R): 5 or 6
```

## Troubleshooting

### Test Skips with "Test PDF not found"
- Create the test PDF using one of the methods above
- Ensure it's placed at `tests/resources/test_aes256.pdf`

### Test Skips with "PDF is not AES-256"
- The PDF is encrypted but not with AES-256
- Re-encrypt using qpdf with `256` as the encryption level:
  ```bash
  qpdf --encrypt user owner 256 -- input.pdf output.pdf
  ```

### "qpdf not found"
- Install qpdf (see Quick Start section)
- Ensure it's in your PATH

### Decryption Fails
- Verify the password is correct
- Check that the PDF is actually encrypted with AES-256
- Try with an empty password if the PDF allows it

## Example: Complete Test Workflow

```bash
# 1. Create a simple test PDF (if you don't have one)
echo "Test PDF content" > test.txt
# (Use any PDF creation tool to convert to PDF)

# 2. Encrypt with AES-256
qpdf --encrypt testpass ownerpass 256 -- test.pdf tests/resources/test_aes256.pdf

# 3. Verify encryption
qpdf --show-encryption tests/resources/test_aes256.pdf

# 4. Run tests
go test ./tests -run TestE2E_AES256 -v

# 5. Clean up (optional)
rm tests/resources/test_aes256.pdf
```

## What the Tests Verify

1. **Encryption Dictionary Parsing**: Correctly extracts V, R, U, UE, OE values
2. **Key Derivation**: SHA-256 based key derivation (64 iterations)
3. **U Value Verification**: Password validation using validation salt
4. **Key Unwrapping**: Successfully unwraps /UE and /OE
5. **Decryption**: Decrypts PDF objects correctly
6. **Password Handling**: Works with both user and owner passwords

## Notes

- The test PDF password defaults to "testpass" for user password
- Owner password defaults to "ownerpass" (for qpdf-created PDFs)
- Tests will skip gracefully if test PDFs are not available
- Temporary files created by `TestE2E_AES256_CreateAndDecrypt` are automatically cleaned up
