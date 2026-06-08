package sign_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/core/sign"
)

func TestSignPDF_VisibleAppearance_RectIsNonZero(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Appearance:  &sign.SignAppearance{X: 72, Y: 100, Width: 200, Height: 50},
	})
	if err != nil {
		t.Fatalf("SignPDF with Appearance: %v", err)
	}
	raw := string(signed)
	if strings.Contains(raw, "Rect[0 0 0 0]") {
		t.Error("visible signature should not have Rect [0 0 0 0]")
	}
	if !strings.Contains(raw, "Rect[72.0000 100.0000 272.0000 150.0000]") {
		t.Errorf("expected Rect with x2=272 y2=150, raw contains: %v",
			extractRectLine(raw))
	}
}

func TestSignPDF_VisibleAppearance_HasAPDict(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Appearance:  &sign.SignAppearance{X: 72, Y: 100, Width: 200, Height: 50},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if !strings.Contains(string(signed), "/AP<<") {
		t.Error("visible signature widget should contain /AP dict")
	}
	if !strings.Contains(string(signed), "/N ") {
		t.Error("visible signature /AP dict should contain /N stream reference")
	}
}

func TestSignPDF_VisibleAppearance_StreamPresent(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Reason:      "Approved",
		Appearance:  &sign.SignAppearance{X: 72, Y: 100, Width: 200, Height: 50},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	raw := string(signed)
	if !strings.Contains(raw, "/Subtype/Form") {
		t.Error("expected Form XObject for appearance stream")
	}
	if !strings.Contains(raw, "/BBox") {
		t.Error("expected /BBox in appearance XObject")
	}
	if !strings.Contains(raw, "Signed by:") {
		t.Error("appearance stream should contain signer text")
	}
	if !strings.Contains(raw, "Reason: Approved") {
		t.Error("appearance stream should contain Reason text")
	}
}

func TestSignPDF_VisibleAppearance_IncrementalUpdate(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Appearance:  &sign.SignAppearance{X: 72, Y: 100, Width: 200, Height: 50},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if !bytes.HasPrefix(signed, pdf) {
		t.Error("visible-signature PDF should start with original bytes")
	}
}

func TestSignPDF_VisibleAppearance_ByteRangeConsistent(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{
		Certificate: cert,
		PrivateKey:  key,
		Appearance:  &sign.SignAppearance{X: 72, Y: 100, Width: 200, Height: 50},
	})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	off1, len1, off2, len2, ok := parseByteRange(string(signed))
	if !ok {
		t.Fatal("could not parse /ByteRange")
	}
	if off1 != 0 {
		t.Errorf("ByteRange[0] should be 0, got %d", off1)
	}
	if off2 <= len1 {
		t.Errorf("ByteRange[2] (%d) should be > ByteRange[1] (%d)", off2, len1)
	}
	if off2+len2 != int64(len(signed)) {
		t.Errorf("ByteRange off2+len2=%d, file length=%d", off2+len2, len(signed))
	}
}

func TestSignPDF_NoAppearance_RemainsInvisible(t *testing.T) {
	pdf := buildSimplePDF(t)
	key, cert, _ := sign.GenerateTestCredentials()

	signed, err := sign.SignPDF(pdf, sign.SignOptions{Certificate: cert, PrivateKey: key})
	if err != nil {
		t.Fatalf("SignPDF: %v", err)
	}
	if !strings.Contains(string(signed), "Rect[0 0 0 0]") {
		t.Error("default (no Appearance) should use invisible Rect [0 0 0 0]")
	}
	if strings.Contains(string(signed), "/AP<<") {
		t.Error("default (no Appearance) should not include /AP dict")
	}
}

// extractRectLine extracts the portion of the raw PDF string containing /Rect
// for diagnostic messages.
func extractRectLine(raw string) string {
	idx := strings.Index(raw, "/Rect[")
	if idx < 0 {
		return "(no /Rect found)"
	}
	end := strings.Index(raw[idx:], "]")
	if end < 0 {
		return raw[idx : idx+40]
	}
	return raw[idx : idx+end+1]
}
