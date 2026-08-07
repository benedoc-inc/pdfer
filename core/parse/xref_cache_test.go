package parse

import (
	"testing"
)

func TestMergedXRefChain_CachesPerSlice(t *testing.T) {
	pdf := createIncrementalPDF()

	first, err := mergedXRefChain(pdf, false)
	if err != nil {
		t.Fatalf("first parse failed: %v", err)
	}
	second, err := mergedXRefChain(pdf, false)
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}
	if first != second {
		t.Error("same slice should return the cached XRefResult, got a fresh parse")
	}

	// A distinct buffer with the same content is a different document as far
	// as the identity-keyed cache is concerned — it must be parsed fresh.
	clone := append([]byte(nil), pdf...)
	third, err := mergedXRefChain(clone, false)
	if err != nil {
		t.Fatalf("clone parse failed: %v", err)
	}
	if third == first {
		t.Error("distinct slice should not share the cached XRefResult")
	}
	if len(third.Objects) != len(first.Objects) {
		t.Errorf("clone parse disagrees: %d objects vs %d", len(third.Objects), len(first.Objects))
	}
}

func TestMergedXRefChain_EvictionKeepsResultsCorrect(t *testing.T) {
	pdf := createIncrementalPDF()
	want, err := mergedXRefChain(pdf, false)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Flood the cache past its capacity with other documents.
	for i := 0; i < xrefCacheSize+2; i++ {
		other := createSimplePDF()
		if _, err := mergedXRefChain(other, false); err != nil {
			t.Fatalf("filler parse %d failed: %v", i, err)
		}
	}

	// The original entry has been evicted; a re-parse must still be correct.
	got, err := mergedXRefChain(pdf, false)
	if err != nil {
		t.Fatalf("re-parse after eviction failed: %v", err)
	}
	if len(got.Objects) != len(want.Objects) {
		t.Errorf("re-parse disagrees: %d objects vs %d", len(got.Objects), len(want.Objects))
	}
	for objNum, offset := range want.Objects {
		if got.Objects[objNum] != offset {
			t.Errorf("object %d: offset %d after eviction, want %d", objNum, got.Objects[objNum], offset)
		}
	}
}

func TestMergedXRefChain_EmptyInput(t *testing.T) {
	if _, err := mergedXRefChain(nil, false); err == nil {
		t.Error("expected error for empty input")
	}
}
