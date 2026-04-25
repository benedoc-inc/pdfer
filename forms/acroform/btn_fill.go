package acroform

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// isBtnPushButton reports whether Btn field flags indicate a push-button.
func isBtnPushButton(ff int) bool { return (ff & 0x10000) != 0 }

// isBtnRadio reports whether Btn field flags indicate a radio button group.
func isBtnRadio(ff int) bool { return (ff & 0x8000) != 0 }

// btnCheckboxState converts a fill value to a PDF appearance-state name.
// Bool true / "yes" / "on" / "true" / "checked" / "Yes" → "Yes"; anything else → "Off".
// For radio buttons pass the option name directly (it is returned unchanged if non-empty).
func btnCheckboxState(value interface{}) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "Yes"
		}
	case string:
		lower := strings.ToLower(v)
		if lower == "true" || lower == "yes" || lower == "on" || lower == "checked" {
			return "Yes"
		}
	}
	return "Off"
}

// withAS sets or replaces /AS in a field body (body-only bytes, no obj header/footer).
func withAS(body []byte, state string) []byte {
	s := string(body)
	newAS := fmt.Sprintf("/AS /%s", state)
	asPat := regexp.MustCompile(`/AS\s*/\w+`)
	if asPat.MatchString(s) {
		s = asPat.ReplaceAllString(s, newAS)
	} else {
		end := strings.LastIndex(s, ">>")
		if end == -1 {
			return body
		}
		s = s[:end] + newAS + " " + s[end:]
	}
	return []byte(s)
}

// withBtnAP adds or replaces /AP<</N<</Yes yesObj 0 R/Off offObj 0 R>>>> in body.
func withBtnAP(body []byte, yesObjNum, offObjNum int) []byte {
	s := string(body)
	newAP := fmt.Sprintf("/AP<</N<</Yes %d 0 R/Off %d 0 R>>>>", yesObjNum, offObjNum)
	apPat := regexp.MustCompile(`/AP\s*<<[^<>]*(?:<<[^<>]*>>[^<>]*)*>>`)
	if apPat.MatchString(s) {
		s = apPat.ReplaceAllString(s, newAP)
	} else {
		end := strings.LastIndex(s, ">>")
		if end == -1 {
			return body
		}
		s = s[:end] + newAP + s[end:]
	}
	return []byte(s)
}

// buildCheckboxAPStream returns the dict and raw stream bytes for a checkbox
// appearance XObject. checked=true draws a simple checkmark; checked=false is empty.
func buildCheckboxAPStream(w, h float64, checked bool) (dictBytes, streamBytes []byte) {
	if w <= 0 {
		w = 12
	}
	if h <= 0 {
		h = 12
	}

	var stream string
	if checked {
		lw := w / 15
		if lw < 0.5 {
			lw = 0.5
		}
		stream = fmt.Sprintf("q\n%.4g w\n0 g\n%.4g %.4g m\n%.4g %.4g l\n%.4g %.4g l\nS\nQ\n",
			lw,
			w*0.15, h*0.5,
			w*0.4, h*0.25,
			w*0.85, h*0.75,
		)
	} else {
		stream = "q\nQ\n"
	}

	dict := fmt.Sprintf("<</Type/XObject/Subtype/Form/BBox[0 0 %.4g %.4g]/Length %d>>",
		w, h, len(stream))
	return []byte(dict), []byte(stream)
}

// btnKidExportValue extracts the "on" state name from a radio-button widget body.
// It looks in /AP/N for a key that is not /Off; falls back to the existing /AS value.
func btnKidExportValue(body []byte) string {
	s := string(body)

	// Try /AP << /N << /SomeName ... /Off ... >> >> — grab first non-Off key.
	apPat := regexp.MustCompile(`(?i)/AP\s*<<`)
	if loc := apPat.FindStringIndex(s); loc != nil {
		sub := s[loc[0]:]
		nPat := regexp.MustCompile(`(?i)/N\s*<<`)
		if nloc := nPat.FindStringIndex(sub); nloc != nil {
			// Grab the chunk right after /N << until >>
			inner := sub[nloc[1]:]
			end := strings.Index(inner, ">>")
			if end > 0 {
				inner = inner[:end]
			}
			keyPat := regexp.MustCompile(`/([A-Za-z][A-Za-z0-9_]*)`)
			for _, m := range keyPat.FindAllStringSubmatch(inner, -1) {
				if m[1] != "Off" {
					return m[1]
				}
			}
		}
	}

	// Fallback: use current /AS if it is not /Off.
	if m := regexp.MustCompile(`/AS\s*/(\w+)`).FindStringSubmatch(s); m != nil && m[1] != "Off" {
		return m[1]
	}

	// Last resort: check if body contains just one non-Off appearance in /AP/N.
	if bytes.Contains(body, []byte("/Yes")) {
		return "Yes"
	}
	return ""
}
