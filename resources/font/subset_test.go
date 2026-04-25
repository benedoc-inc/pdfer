package font

import (
	"os"
	"testing"
)

// loadTestFont returns the bytes of a real TTF, or skips the test if none is available.
func loadTestFont(t *testing.T) []byte {
	t.Helper()
	candidates := []string{
		"/Library/Fonts/Arial Unicode.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/Apple Braille.ttf",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) >= 4 {
			sig := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
			if sig == 0x00010000 || sig == 0x4F54544F {
				t.Logf("using font: %s (%d bytes)", path, len(data))
				return data
			}
		}
	}
	t.Skip("no suitable TTF found — skipping subset test")
	return nil
}

// TestCreateSubsetFont_SmallerThanFull verifies that subsetting a large font
// with a small character set produces a smaller output than the original.
func TestCreateSubsetFont_SmallerThanFull(t *testing.T) {
	data := loadTestFont(t)

	f, err := NewFont("Test", data)
	if err != nil {
		t.Fatalf("NewFont: %v", err)
	}

	f.AddString("Hello") // 5 distinct glyphs

	subset, err := f.CreateSubsetFont()
	if err != nil {
		t.Fatalf("CreateSubsetFont: %v", err)
	}
	if len(subset) == 0 {
		t.Fatal("empty subset output")
	}

	// A 5-glyph subset must be smaller than the full font (which may have 10k+ glyphs).
	if len(subset) >= len(data) {
		t.Errorf("subset (%d bytes) is not smaller than full font (%d bytes)", len(subset), len(data))
	}
	t.Logf("full=%d bytes, subset=%d bytes (ratio %.1f%%)",
		len(data), len(subset), 100*float64(len(subset))/float64(len(data)))
}

// TestCreateSubsetFont_IsValidTTF verifies that the output is parseable as a TTF.
func TestCreateSubsetFont_IsValidTTF(t *testing.T) {
	data := loadTestFont(t)

	f, err := NewFont("Test", data)
	if err != nil {
		t.Fatalf("NewFont: %v", err)
	}
	f.AddString("ABC")

	subset, err := f.CreateSubsetFont()
	if err != nil {
		t.Fatalf("CreateSubsetFont: %v", err)
	}

	ttf, err := ParseTTF(subset)
	if err != nil {
		t.Fatalf("ParseTTF on subset output: %v", err)
	}
	if ttf.NumGlyphs == 0 {
		t.Error("subset has 0 glyphs")
	}
	// Subset must contain at most the number of requested glyphs (+1 for .notdef).
	if int(ttf.NumGlyphs) > len(f.Subset)+2 {
		t.Errorf("subset has %d glyphs but only %d were requested", ttf.NumGlyphs, len(f.Subset))
	}
	t.Logf("subset glyph count: %d (requested %d)", ttf.NumGlyphs, len(f.Subset))
}

// TestCreateSubsetFont_RequiredTablesPresent checks that the subset TTF
// contains the mandatory tables.
func TestCreateSubsetFont_RequiredTablesPresent(t *testing.T) {
	data := loadTestFont(t)

	f, err := NewFont("Test", data)
	if err != nil {
		t.Fatalf("NewFont: %v", err)
	}
	f.AddString("Hi")

	subset, err := f.CreateSubsetFont()
	if err != nil {
		t.Fatalf("CreateSubsetFont: %v", err)
	}

	ttf, err := ParseTTF(subset)
	if err != nil {
		t.Fatalf("ParseTTF: %v", err)
	}

	required := []string{"head", "hhea", "maxp", "cmap", "hmtx", "glyf", "loca"}
	for _, tbl := range required {
		if _, ok := ttf.Tables[tbl]; !ok {
			t.Errorf("required table %q missing from subset", tbl)
		}
	}
}

// TestCreateSubsetFont_EmptySubset verifies that an empty subset falls back
// gracefully (returns data, no panic, no error).
func TestCreateSubsetFont_EmptySubset(t *testing.T) {
	data := loadTestFont(t)

	f, err := NewFont("Test", data)
	if err != nil {
		t.Fatalf("NewFont: %v", err)
	}
	// No characters added — Subset is empty.

	out, err := f.CreateSubsetFont()
	if err != nil {
		t.Fatalf("CreateSubsetFont with empty subset: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty fallback output")
	}
}

// TestCreateSubsetFont_FallbackOnMinimalTTF verifies that CreateSubsetFont
// returns the full font (graceful fallback) when the TTF lacks the tables
// needed for subsetting (e.g. the minimal test fixture has no cmap/glyf).
func TestCreateSubsetFont_FallbackOnMinimalTTF(t *testing.T) {
	minimal := createMinimalTTF()

	f, err := NewFont("Minimal", minimal)
	if err != nil {
		t.Fatalf("NewFont: %v", err)
	}
	f.AddString("ABC")

	out, err := f.CreateSubsetFont()
	if err != nil {
		t.Fatalf("CreateSubsetFont: %v", err)
	}
	// Should fall back to the full (minimal) font data.
	if len(out) == 0 {
		t.Error("expected non-empty fallback")
	}
}
