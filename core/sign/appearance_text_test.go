package sign

import (
	"strings"
	"testing"
)

// TestEscapeAppearanceStr_EncodesWinAnsi is the bug this encoder exists for.
//
// ⚠⚠ A content-stream string is indexed into the FONT's encoding, not UTF-8.
// Emitting the runes raw made "José" render as "JosÃ©" on the face of a signed
// document — a defect nobody would attribute to the signing code.
func TestEscapeAppearanceStr_EncodesWinAnsi(t *testing.T) {
	t.Parallel()

	got := escapeAppearanceStr("José")

	// é is 0xE9 in WinAnsi, written octal: \351.
	if want := `Jos\351`; got != want {
		t.Errorf("escapeAppearanceStr(José) = %q, want %q", got, want)
	}

	if strings.Contains(got, "Ã") {
		t.Error("the UTF-8 bytes leaked into the content stream")
	}
}

// TestEscapeAppearanceStr_KeepsWordPunctuation covers the CP1252 block, which
// Latin-1 leaves as control codes — and which every word processor produces.
func TestEscapeAppearanceStr_KeepsWordPunctuation(t *testing.T) {
	t.Parallel()

	// A right single quote (0x92) and an em dash (0x97).
	got := escapeAppearanceStr("O’Brien — CEO")

	if !strings.Contains(got, `\222`) {
		t.Errorf("curly apostrophe not encoded: %q", got)
	}

	if !strings.Contains(got, `\227`) {
		t.Errorf("em dash not encoded: %q", got)
	}
}

// TestEscapeAppearanceStr_SubstitutesWhatWinAnsiCannotHold keeps an
// unrepresentable name honest rather than mojibake.
func TestEscapeAppearanceStr_SubstitutesWhatWinAnsiCannotHold(t *testing.T) {
	t.Parallel()

	got := escapeAppearanceStr("署名")

	if want := "??"; got != want {
		t.Errorf("escapeAppearanceStr(署名) = %q, want %q", got, want)
	}
}

func TestEscapeAppearanceStr_EscapesDelimiters(t *testing.T) {
	t.Parallel()

	got := escapeAppearanceStr(`a(b)c\d`)

	if want := `a\(b\)c\\d`; got != want {
		t.Errorf("escapeAppearanceStr = %q, want %q", got, want)
	}
}

// TestFitAppearanceLine_TruncatesRatherThanOverflowing is the visual
// correctness guard.
//
// ⚠ A Form XObject's BBox does not clip reliably in every viewer, so an
// over-long line can print across the document underneath the box. A visible
// ellipsis is honest; overflow reads as a rendering fault.
func TestFitAppearanceLine_TruncatesRatherThanOverflowing(t *testing.T) {
	t.Parallel()

	const boxWidth = 200.0

	long := "Signed by: Bartholomew Montgomery-Fitzwilliam the Third, Chief Executive"
	got := fitAppearanceLine(long, boxWidth)

	if got == long {
		t.Fatal("an over-long line was not truncated")
	}

	if !strings.HasSuffix(got, appearanceEllipsis) {
		t.Errorf("a truncated line should end in %q, got %q", appearanceEllipsis, got)
	}

	if w := helveticaWidth(got, appearanceFontSize); w > boxWidth-2*appearancePadding {
		t.Errorf("truncated line is %.1fpt wide, box allows %.1fpt", w, boxWidth-2*appearancePadding)
	}
}

func TestFitAppearanceLine_LeavesAFittingLineAlone(t *testing.T) {
	t.Parallel()

	line := "Date: 2026-11-09"

	if got := fitAppearanceLine(line, 200); got != line {
		t.Errorf("fitAppearanceLine rewrote a line that fits: %q", got)
	}
}

// TestFitAppearanceLine_StripsControlCharacters keeps a newline from eating
// the rest of the line.
//
// ⚠ Inside a Tj string a newline is a control byte, not a line break — some
// viewers drop everything after it.
func TestFitAppearanceLine_StripsControlCharacters(t *testing.T) {
	t.Parallel()

	got := fitAppearanceLine("Signed by:\nDana Okafor", 300)

	if strings.ContainsAny(got, "\n\r\t") {
		t.Errorf("control characters survived: %q", got)
	}

	if !strings.Contains(got, "Dana Okafor") {
		t.Errorf("text after the newline was lost: %q", got)
	}
}

// TestAppearanceLines_EmptyMeansEmpty distinguishes "no lines" from "default
// lines".
//
// ⚠⚠ A caller that computes its own lines and ends up with none wants an empty
// box. Falling back to the default would print the CERTIFICATE's CommonName —
// the sealing platform — where a reader expects the person who signed.
func TestAppearanceLines_EmptyMeansEmpty(t *testing.T) {
	t.Parallel()

	if got := appearanceLines([]string{}, "BeneDoc Inc.", "2026-11-09", ""); len(got) != 0 {
		t.Errorf("an explicitly empty Lines produced %v", got)
	}

	got := appearanceLines(nil, "BeneDoc Inc.", "2026-11-09", "because")
	if len(got) != 3 || !strings.Contains(got[0], "BeneDoc Inc.") {
		t.Errorf("nil Lines should give the default body, got %v", got)
	}
}
