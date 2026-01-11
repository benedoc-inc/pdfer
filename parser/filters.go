// Package parser provides PDF parsing functionality including stream filters
package parser

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// DecodeFilter applies the appropriate filter to decode stream data
// Supports: FlateDecode, ASCIIHexDecode, ASCII85Decode, RunLengthDecode, DCTDecode
func DecodeFilter(data []byte, filterName string) ([]byte, error) {
	switch filterName {
	case "/FlateDecode", "FlateDecode":
		return DecodeFlateDecode(data)
	case "/ASCIIHexDecode", "ASCIIHexDecode":
		return DecodeASCIIHex(data)
	case "/ASCII85Decode", "ASCII85Decode":
		return DecodeASCII85(data)
	case "/RunLengthDecode", "RunLengthDecode":
		return DecodeRunLength(data)
	case "/DCTDecode", "DCTDecode":
		return DecodeDCTDecode(data)
	default:
		return nil, fmt.Errorf("unsupported filter: %s", filterName)
	}
}

// DecodeFlateDecode decompresses zlib/deflate compressed data
func DecodeFlateDecode(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("zlib error: %v", err)
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// DecodeASCIIHex decodes ASCIIHexDecode filter data
// ASCIIHexDecode converts pairs of hex digits to bytes
// Whitespace is ignored, '>' marks end of data
func DecodeASCIIHex(data []byte) ([]byte, error) {
	var result bytes.Buffer
	var hexByte byte
	var haveNibble bool

	for _, b := range data {
		// Skip whitespace
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f' {
			continue
		}

		// End of data marker
		if b == '>' {
			break
		}

		// Convert hex character to nibble
		var nibble byte
		switch {
		case b >= '0' && b <= '9':
			nibble = b - '0'
		case b >= 'A' && b <= 'F':
			nibble = b - 'A' + 10
		case b >= 'a' && b <= 'f':
			nibble = b - 'a' + 10
		default:
			return nil, fmt.Errorf("invalid hex character: %c", b)
		}

		if haveNibble {
			// Complete the byte
			result.WriteByte(hexByte<<4 | nibble)
			haveNibble = false
		} else {
			// First nibble of pair
			hexByte = nibble
			haveNibble = true
		}
	}

	// Handle odd number of hex digits (pad with 0)
	if haveNibble {
		result.WriteByte(hexByte << 4)
	}

	return result.Bytes(), nil
}

// DecodeASCII85 decodes ASCII85Decode (also known as btoa) filter data
// ASCII85 encodes 4 bytes as 5 ASCII characters (base 85)
// Special cases: 'z' represents 4 zero bytes, '~>' marks end
func DecodeASCII85(data []byte) ([]byte, error) {
	var result bytes.Buffer

	// Remove leading <~ if present
	if bytes.HasPrefix(data, []byte("<~")) {
		data = data[2:]
	}

	// Process until ~>
	var tuple [5]byte
	tupleIndex := 0

	for i := 0; i < len(data); i++ {
		b := data[i]

		// Skip whitespace
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f' {
			continue
		}

		// End marker
		if b == '~' {
			if i+1 < len(data) && data[i+1] == '>' {
				break
			}
		}

		// 'z' is special: represents 4 zero bytes
		if b == 'z' {
			if tupleIndex != 0 {
				return nil, fmt.Errorf("'z' inside ascii85 tuple")
			}
			result.Write([]byte{0, 0, 0, 0})
			continue
		}

		// Valid ASCII85 character: '!' (33) to 'u' (117)
		if b < '!' || b > 'u' {
			return nil, fmt.Errorf("invalid ascii85 character: %c (0x%02x)", b, b)
		}

		tuple[tupleIndex] = b - '!'
		tupleIndex++

		// Process complete tuple of 5 characters
		if tupleIndex == 5 {
			decoded := decodeASCII85Tuple(tuple[:], 5)
			result.Write(decoded)
			tupleIndex = 0
		}
	}

	// Handle partial tuple at end
	if tupleIndex > 0 {
		// Pad with 'u' (84) characters
		for i := tupleIndex; i < 5; i++ {
			tuple[i] = 84 // 'u' - '!' = 84
		}
		decoded := decodeASCII85Tuple(tuple[:], tupleIndex)
		// Only write the actual bytes (not the padding)
		bytesToWrite := tupleIndex - 1
		if bytesToWrite > 0 {
			result.Write(decoded[:bytesToWrite])
		}
	}

	return result.Bytes(), nil
}

// decodeASCII85Tuple decodes a 5-character ASCII85 tuple to 4 bytes
func decodeASCII85Tuple(tuple []byte, count int) []byte {
	// Calculate 32-bit value from base-85 digits
	var value uint32
	value = uint32(tuple[0])*85*85*85*85 +
		uint32(tuple[1])*85*85*85 +
		uint32(tuple[2])*85*85 +
		uint32(tuple[3])*85 +
		uint32(tuple[4])

	// Extract 4 bytes (big-endian)
	return []byte{
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	}
}

// DecodeRunLength decodes RunLengthDecode filter data
// Format: length byte followed by data
// - length 0-127: copy next (length+1) bytes literally
// - length 129-255: repeat next byte (257-length) times
// - length 128: end of data
func DecodeRunLength(data []byte) ([]byte, error) {
	var result bytes.Buffer
	i := 0

	for i < len(data) {
		length := int(data[i])
		i++

		if length == 128 {
			// End of data
			break
		} else if length < 128 {
			// Copy next (length+1) bytes literally
			count := length + 1
			if i+count > len(data) {
				return nil, fmt.Errorf("runlength: not enough data for literal run")
			}
			result.Write(data[i : i+count])
			i += count
		} else {
			// Repeat next byte (257-length) times
			count := 257 - length
			if i >= len(data) {
				return nil, fmt.Errorf("runlength: not enough data for repeat")
			}
			repeatByte := data[i]
			i++
			for j := 0; j < count; j++ {
				result.WriteByte(repeatByte)
			}
		}
	}

	return result.Bytes(), nil
}

// EncodeASCIIHex encodes data using ASCIIHexDecode format
// Returns hex string with '>' terminator
func EncodeASCIIHex(data []byte) []byte {
	result := make([]byte, len(data)*2+1)
	hexChars := "0123456789ABCDEF"

	for i, b := range data {
		result[i*2] = hexChars[b>>4]
		result[i*2+1] = hexChars[b&0x0F]
	}
	result[len(data)*2] = '>'

	return result
}

// EncodeASCII85 encodes data using ASCII85Decode format
// Returns data wrapped in <~ ... ~>
func EncodeASCII85(data []byte) []byte {
	var result bytes.Buffer
	result.WriteString("<~")

	// Process 4 bytes at a time
	for i := 0; i < len(data); i += 4 {
		remaining := len(data) - i
		if remaining >= 4 {
			// Full 4-byte tuple
			value := uint32(data[i])<<24 | uint32(data[i+1])<<16 | uint32(data[i+2])<<8 | uint32(data[i+3])

			// Special case: all zeros
			if value == 0 {
				result.WriteByte('z')
				continue
			}

			// Encode to 5 base-85 digits
			var encoded [5]byte
			for j := 4; j >= 0; j-- {
				encoded[j] = byte(value%85) + '!'
				value /= 85
			}
			result.Write(encoded[:])
		} else {
			// Partial tuple at end
			var value uint32
			for j := 0; j < remaining; j++ {
				value |= uint32(data[i+j]) << (24 - j*8)
			}

			// Encode to (remaining+1) base-85 digits
			var encoded [5]byte
			for j := 4; j >= 0; j-- {
				encoded[j] = byte(value%85) + '!'
				value /= 85
			}
			result.Write(encoded[:remaining+1])
		}
	}

	result.WriteString("~>")
	return result.Bytes()
}

// EncodeRunLength encodes data using RunLengthDecode format
func EncodeRunLength(data []byte) []byte {
	if len(data) == 0 {
		return []byte{128} // Just EOD marker
	}

	var result bytes.Buffer
	i := 0

	for i < len(data) {
		// Look for runs of repeated bytes
		runStart := i
		for i < len(data)-1 && data[i] == data[i+1] && i-runStart < 127 {
			i++
		}
		runLength := i - runStart + 1

		if runLength >= 2 {
			// Encode as repeated byte
			result.WriteByte(byte(257 - runLength))
			result.WriteByte(data[runStart])
			i++
		} else {
			// Encode as literal bytes
			literalStart := runStart
			for i < len(data) && (i == len(data)-1 || data[i] != data[i+1]) && i-literalStart < 127 {
				i++
			}
			literalLength := i - literalStart
			result.WriteByte(byte(literalLength - 1))
			result.Write(data[literalStart:i])
		}
	}

	// End of data marker
	result.WriteByte(128)

	return result.Bytes()
}

// DecodeDCTDecode decodes DCTDecode filter data
// DCTDecode is a pass-through filter for JPEG-compressed image data
// The data is already in JPEG format, so we just return it as-is
func DecodeDCTDecode(data []byte) ([]byte, error) {
	// DCTDecode is a pass-through - JPEG data is already compressed
	// Just validate it's valid JPEG by checking for JPEG markers
	if len(data) < 2 {
		return nil, fmt.Errorf("DCTDecode: data too short for JPEG")
	}

	// Check for JPEG SOI (Start of Image) marker: 0xFF 0xD8
	if data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("DCTDecode: invalid JPEG header (expected 0xFF 0xD8, got 0x%02X 0x%02X)", data[0], data[1])
	}

	// Return data as-is (JPEG is already compressed)
	return data, nil
}

// EncodeDCTDecode encodes data using DCTDecode filter
// This is a pass-through - assumes data is already valid JPEG
func EncodeDCTDecode(data []byte) ([]byte, error) {
	// Validate JPEG header
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("EncodeDCTDecode: data is not valid JPEG (missing SOI marker)")
	}

	// Return as-is (JPEG is already compressed)
	return data, nil
}
