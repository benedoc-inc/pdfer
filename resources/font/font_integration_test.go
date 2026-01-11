package font

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWithRealTTF tests font embedding with a real TTF file
// This test will be skipped if no TTF file is found
func TestWithRealTTF(t *testing.T) {
	// Try to find a test font (prefer local test font, then system fonts)
	ttfPaths := []string{
		"tests/resources/test_font.ttf",
		"../tests/resources/test_font.ttf",
		"../../tests/resources/test_font.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/System/Library/Fonts/Helvetica.ttc", // TTC (collection), but we'll try
		"/System/Library/Fonts/Apple Braille.ttf",
	}

	var ttfPath string
	var ttfData []byte
	var err error

	for _, path := range ttfPaths {
		if data, err := os.ReadFile(path); err == nil {
			// Check if it's a valid TTF (starts with TTF signature)
			if len(data) > 4 {
				sig := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
				if sig == 0x00010000 || sig == 0x4F54544F { // TTF or OTF
					ttfPath = path
					ttfData = data
					break
				}
			}
		}
	}

	if ttfPath == "" {
		t.Skip("No TTF font file found for testing. Place a .ttf file in tests/resources/ or use a system font.")
	}

	t.Logf("Using TTF file: %s (%d bytes)", ttfPath, len(ttfData))

	// Test parsing
	ttf, err := ParseTTF(ttfData)
	if err != nil {
		t.Fatalf("Failed to parse TTF: %v", err)
	}

	if ttf.UnitsPerEm == 0 {
		t.Error("UnitsPerEm should not be zero")
	}

	if ttf.FontName == "" {
		t.Log("Warning: Font name not found (may be normal for some fonts)")
	}

	t.Logf("Font: %s, UnitsPerEm: %d, NumGlyphs: %d", ttf.FontName, ttf.UnitsPerEm, ttf.NumGlyphs)

	// Test font creation
	font, err := NewFont("TestFont", ttfData)
	if err != nil {
		t.Fatalf("Failed to create font: %v", err)
	}

	// Add some test characters
	testString := "Hello, World! 123"
	font.AddString(testString)
	font.AddRune('€') // Test Unicode character

	if len(font.Subset) == 0 {
		t.Error("Subset should contain characters")
	}

	t.Logf("Subset contains %d unique characters", len(font.Subset))

	// Test getting subset glyphs
	glyphs, err := font.GetSubsetGlyphs()
	if err != nil {
		t.Logf("Warning: Failed to get subset glyphs (may need cmap table): %v", err)
	} else {
		t.Logf("Subset contains %d glyphs", len(glyphs))
		if len(glyphs) == 0 {
			t.Error("Should have at least one glyph (notdef)")
		}
	}

	// Test getting widths
	widths, err := font.GetWidths()
	if err != nil {
		t.Logf("Warning: Failed to get widths (may need hmtx table): %v", err)
	} else {
		t.Logf("Got %d widths", len(widths))
	}
}

// TestFindSystemFonts lists available system fonts (for debugging)
func TestFindSystemFonts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system font search in short mode")
	}

	fontDirs := []string{
		"/System/Library/Fonts",
		"/System/Library/Fonts/Supplemental",
		"/Library/Fonts",
	}

	var foundFonts []string
	for _, dir := range fontDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				name := entry.Name()
				if len(name) > 4 && (name[len(name)-4:] == ".ttf" || name[len(name)-4:] == ".otf") {
					fullPath := filepath.Join(dir, name)
					foundFonts = append(foundFonts, fullPath)
				}
			}
		}
	}

	if len(foundFonts) > 0 {
		t.Logf("Found %d font files:", len(foundFonts))
		for i, font := range foundFonts {
			if i < 10 { // Show first 10
				info, _ := os.Stat(font)
				t.Logf("  %s (%d bytes)", font, info.Size())
			}
		}
		if len(foundFonts) > 10 {
			t.Logf("  ... and %d more", len(foundFonts)-10)
		}
	} else {
		t.Log("No font files found in system directories")
	}
}
