package acroform

import (
	"strings"
	"testing"
)

// --- findDictEnd ------------------------------------------------------------

func TestFindDictEnd_Simple(t *testing.T) {
	s := `<< /Key /Value >>`
	// Start from the opening <<
	idx := findDictEnd(s, 0)
	if idx != len(s)-2 { // points at the first > of >>
		t.Errorf("expected idx %d, got %d (string: %q)", len(s)-2, idx, s)
	}
}

func TestFindDictEnd_Nested(t *testing.T) {
	s := `<< /Outer << /Inner /Val >> /More /Keys >>`
	idx := findDictEnd(s, 0)
	// Should point at the final >>
	if idx != len(s)-2 {
		t.Errorf("expected idx %d, got %d", len(s)-2, idx)
	}
}

func TestFindDictEnd_DeepNesting(t *testing.T) {
	s := `<< /A << /B << /C /D >> >> /E /F >>`
	idx := findDictEnd(s, 0)
	if s[idx:idx+2] != ">>" {
		t.Errorf("result %d is not '>>'", idx)
	}
	// Verify it's the outermost >> by checking nothing follows
	if idx != len(s)-2 {
		t.Errorf("expected outermost >> at %d, got %d (remainder: %q)", len(s)-2, idx, s[idx:])
	}
}

// --- removeDictKey ----------------------------------------------------------

func TestRemoveDictKey_NameValue(t *testing.T) {
	dict := `<< /Type /Annot /A /MyAction /Subtype /Widget >>`
	got := removeDictKey(dict, "A")
	if strings.Contains(got, "/A ") {
		t.Errorf("key /A not removed: %q", got)
	}
	if !strings.Contains(got, "/Type /Annot") {
		t.Errorf("surrounding keys disturbed: %q", got)
	}
	if !strings.Contains(got, "/Subtype /Widget") {
		t.Errorf("trailing key removed: %q", got)
	}
}

func TestRemoveDictKey_ObjRef(t *testing.T) {
	dict := `<< /Parent 3 0 R /A 7 0 R /Kids [1 0 R] >>`
	got := removeDictKey(dict, "A")
	if strings.Contains(got, "/A ") {
		t.Errorf("key /A not removed: %q", got)
	}
	if !strings.Contains(got, "/Parent 3 0 R") {
		t.Error("other ref removed")
	}
	if !strings.Contains(got, "/Kids") {
		t.Error("Kids removed")
	}
}

func TestRemoveDictKey_InlineDict(t *testing.T) {
	dict := `<< /Type /Annot /A << /S /URI /URI (https://x.com) >> /Rect [0 0 100 100] >>`
	got := removeDictKey(dict, "A")
	if strings.Contains(got, "/A ") {
		t.Errorf("key /A not removed: %q", got)
	}
	// The nested /URI inside /A should be gone too.
	if strings.Contains(got, "/S /URI") {
		t.Errorf("nested dict not removed: %q", got)
	}
	if !strings.Contains(got, "/Rect") {
		t.Errorf("Rect key disturbed: %q", got)
	}
}

func TestRemoveDictKey_DeeplyNestedInlineDict(t *testing.T) {
	dict := `<< /X /Val /A << /Outer << /Inner /Deep >> >> /Z /Last >>`
	got := removeDictKey(dict, "A")
	if strings.Contains(got, "/A ") {
		t.Errorf("/A not removed from deeply nested case: %q", got)
	}
	if !strings.Contains(got, "/Z /Last") {
		t.Errorf("trailing key disturbed: %q", got)
	}
}

func TestRemoveDictKey_LiteralString(t *testing.T) {
	dict := `<< /Type /Annot /A (some (nested) string) /Rect [0 0 10 10] >>`
	got := removeDictKey(dict, "A")
	if strings.Contains(got, "/A ") {
		t.Errorf("/A not removed: %q", got)
	}
	if !strings.Contains(got, "/Rect") {
		t.Errorf("Rect disturbed: %q", got)
	}
}

func TestRemoveDictKey_NotPresent(t *testing.T) {
	dict := `<< /Type /Annot /Subtype /Widget >>`
	got := removeDictKey(dict, "A")
	if got != dict {
		t.Errorf("dict changed when key not present: %q -> %q", dict, got)
	}
}

// --- AddActionToField replaces rather than appends -------------------------

func TestAddActionToField_ReplacesExistingA(t *testing.T) {
	// Simulate an object dict that already has an /A entry.
	oldActionDict := `<< /Type /Action /S /URI /URI (https://old.com) >>`
	objDict := `<< /Type /Annot /Subtype /Widget /A ` + oldActionDict + ` /Rect [0 0 100 20] >>`

	// Call AddActionToField via removeDictKey directly (testing the key logic).
	cleaned := removeDictKey(objDict, "A")

	if strings.Contains(cleaned, "old.com") {
		t.Errorf("old /A entry not removed: %q", cleaned)
	}
	if !strings.Contains(cleaned, "/Rect") {
		t.Errorf("/Rect key removed unexpectedly: %q", cleaned)
	}

	// After removing, we can insert a new /A.
	newAction := `/A << /Type /Action /S /URI /URI (https://new.com) >>`
	endIdx := findDictEnd(cleaned, 0)
	result := cleaned[:endIdx] + " " + newAction + cleaned[endIdx:]

	if !strings.Contains(result, "new.com") {
		t.Errorf("new action not present: %q", result)
	}
	if strings.Contains(result, "old.com") {
		t.Errorf("old action still present: %q", result)
	}
}

// --- flattenAddXObjectsToDict with nested dicts ----------------------------

func TestFlattenAddXObjectsToDict_NestedXObject(t *testing.T) {
	// /XObject dict that already contains a nested /Mask sub-dict.
	resStr := `<< /Font << /F1 3 0 R >> /XObject << /Im0 5 0 R /Mask << /ColorSpace /DeviceGray /BitsPerComponent 8 >> >> >>`
	got := flattenAddXObjectsToDict(resStr, map[string]int{"FO7": 7})

	if !strings.Contains(got, "/FO7 7 0 R") {
		t.Errorf("new entry not injected: %s", got)
	}
	if !strings.Contains(got, "/Im0 5 0 R") {
		t.Error("existing /Im0 entry removed")
	}
	if !strings.Contains(got, "/Mask <<") {
		t.Error("nested /Mask dict corrupted or removed")
	}
	if !strings.Contains(got, "/Font") {
		t.Error("/Font entry disturbed")
	}
}

func TestFlattenAddXObjectsToDict_NoExistingXObject(t *testing.T) {
	resStr := `<< /Font << /F1 3 0 R >> >>`
	got := flattenAddXObjectsToDict(resStr, map[string]int{"AP1": 9})
	if !strings.Contains(got, "/XObject<<") {
		t.Errorf("expected /XObject<< to be inserted: %s", got)
	}
	if !strings.Contains(got, "/AP1 9 0 R") {
		t.Errorf("expected /AP1 entry: %s", got)
	}
}

func TestFlattenAddXObjectsToDict_MultipleEntries(t *testing.T) {
	resStr := `<< /XObject << /Im0 1 0 R >> >>`
	got := flattenAddXObjectsToDict(resStr, map[string]int{"AP1": 2, "AP2": 3})
	if !strings.Contains(got, "/AP1 2 0 R") {
		t.Errorf("AP1 not injected: %s", got)
	}
	if !strings.Contains(got, "/AP2 3 0 R") {
		t.Errorf("AP2 not injected: %s", got)
	}
	if !strings.Contains(got, "/Im0 1 0 R") {
		t.Error("existing Im0 removed")
	}
}

func TestFlattenAddXObjectsToDict_Empty(t *testing.T) {
	resStr := `<< /Font << /F1 3 0 R >> >>`
	got := flattenAddXObjectsToDict(resStr, nil)
	if got != resStr {
		t.Errorf("empty xobjs should return unchanged: got %q", got)
	}
}
