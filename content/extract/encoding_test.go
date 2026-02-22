package extract

import (
	"testing"
)

func TestFontDecoder_SingleRuneToUnicode(t *testing.T) {
	fd := NewFontDecoder("TestFont")
	fd.SetToUnicode(65, 'A')
	fd.SetToUnicode(66, 'B')

	result := fd.Decode([]byte{65, 66})
	if result != "AB" {
		t.Errorf("expected 'AB', got %q", result)
	}
}

func TestFontDecoder_MultiRuneBfchar(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Simulate a CMap with a ligature: code 0x01 maps to "fi" (U+0066, U+0069)
	cmapData := `
beginbfchar
<01> <00660069>
<02> <00660066>
endbfchar
`
	fd.ParseToUnicodeCMap(cmapData)

	// Verify multi-rune mapping was stored
	if str, ok := fd.toUnicodeMulti[1]; !ok || str != "fi" {
		t.Errorf("expected multi-rune mapping for code 1 to be 'fi', got %q (ok=%v)", str, ok)
	}
	if str, ok := fd.toUnicodeMulti[2]; !ok || str != "ff" {
		t.Errorf("expected multi-rune mapping for code 2 to be 'ff', got %q (ok=%v)", str, ok)
	}

	// Verify single-rune mappings were NOT stored in toUnicode
	if _, ok := fd.toUnicode[1]; ok {
		t.Error("code 1 should not be in single-rune toUnicode map")
	}
}

func TestFontDecoder_MultiRuneDecode(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Set up: code 0x01 = "fi" ligature, code 0x41 = 'n', code 0x42 = 'd'
	fd.toUnicodeMulti[1] = "fi"
	fd.toUnicode[0x41] = 'n'
	fd.toUnicode[0x42] = 'd'

	result := fd.Decode([]byte{0x41, 0x01, 0x42})
	if result != "nfid" {
		t.Errorf("expected 'nfid', got %q", result)
	}
}

func TestFontDecoder_MultiRuneDecodeHex(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Set up 2-byte codes: code 0x0100 = "ffi" ligature, code 0x0041 = 'A'
	fd.toUnicodeMulti[0x0100] = "ffi"
	fd.toUnicode[0x0041] = 'A'

	result := fd.DecodeHex("<00410100>")
	if result != "Affi" {
		t.Errorf("expected 'Affi', got %q", result)
	}
}

func TestFontDecoder_MultiRuneBfrange(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Simulate a CMap with array range containing multi-rune mapping
	cmapData := `
beginbfrange
<10> <12> [<00660069> <00660066> <00660066006C>]
endbfrange
`
	fd.ParseToUnicodeCMap(cmapData)

	// Code 0x10 -> "fi"
	if str, ok := fd.toUnicodeMulti[0x10]; !ok || str != "fi" {
		t.Errorf("expected multi-rune mapping for code 0x10 to be 'fi', got %q (ok=%v)", str, ok)
	}
	// Code 0x11 -> "ff"
	if str, ok := fd.toUnicodeMulti[0x11]; !ok || str != "ff" {
		t.Errorf("expected multi-rune mapping for code 0x11 to be 'ff', got %q (ok=%v)", str, ok)
	}
	// Code 0x12 -> "ffl"
	if str, ok := fd.toUnicodeMulti[0x12]; !ok || str != "ffl" {
		t.Errorf("expected multi-rune mapping for code 0x12 to be 'ffl', got %q (ok=%v)", str, ok)
	}
}

func TestFontDecoder_SingleRuneBfchar(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Simulate a CMap with single-character mapping
	cmapData := `
beginbfchar
<41> <0041>
<42> <0042>
endbfchar
`
	fd.ParseToUnicodeCMap(cmapData)

	// Verify stored in single-rune map
	if r, ok := fd.toUnicode[0x41]; !ok || r != 'A' {
		t.Errorf("expected single-rune mapping for code 0x41 to be 'A', got %c (ok=%v)", r, ok)
	}
	if r, ok := fd.toUnicode[0x42]; !ok || r != 'B' {
		t.Errorf("expected single-rune mapping for code 0x42 to be 'B', got %c (ok=%v)", r, ok)
	}

	// Verify NOT in multi-rune map
	if _, ok := fd.toUnicodeMulti[0x41]; ok {
		t.Error("code 0x41 should not be in multi-rune map")
	}
}

func TestFontDecoder_LookupCodeStr(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// Single rune in toUnicode
	fd.toUnicode[1] = 'A'
	// Multi-rune in toUnicodeMulti
	fd.toUnicodeMulti[2] = "fi"
	// In differences
	fd.differences[3] = 'B'

	tests := []struct {
		code     int
		expected string
	}{
		{1, "A"},
		{2, "fi"},
		{3, "B"},
		{65, "A"}, // falls through to default (rune(65) = 'A')
	}

	for _, tt := range tests {
		result := fd.lookupCodeStr(tt.code)
		if result != tt.expected {
			t.Errorf("lookupCodeStr(%d): expected %q, got %q", tt.code, tt.expected, result)
		}
	}
}

func TestFontDecoder_Has2ByteMapping_Multi(t *testing.T) {
	fd := NewFontDecoder("TestFont")

	// No mappings > 255 yet
	if fd.has2ByteMapping() {
		t.Error("expected no 2-byte mapping")
	}

	// Add a multi-rune mapping with code > 255
	fd.toUnicodeMulti[0x0100] = "fi"
	if !fd.has2ByteMapping() {
		t.Error("expected 2-byte mapping after adding code 0x0100 to toUnicodeMulti")
	}
}
