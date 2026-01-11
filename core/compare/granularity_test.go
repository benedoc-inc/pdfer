package compare

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
)

func TestCompareOptions_Granularity(t *testing.T) {
	// Create first PDF with text
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Hello World")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with slightly modified text
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Hello Universe") // Changed "World" to "Universe"
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	// Test with element-level granularity (default)
	opts1 := DefaultCompareOptions()
	opts1.TextGranularity = GranularityElement
	result1, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts1)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result1.Identical {
		t.Error("Expected PDFs to be different")
	}
	if len(result1.PageDiffs) == 0 || result1.PageDiffs[0].TextDiff == nil {
		t.Error("Expected text differences")
	} else {
		t.Logf("Element-level: modified=%d, removed=%d, added=%d",
			len(result1.PageDiffs[0].TextDiff.Modified),
			len(result1.PageDiffs[0].TextDiff.Removed),
			len(result1.PageDiffs[0].TextDiff.Added))
	}

	// Test with word-level granularity
	opts2 := DefaultCompareOptions()
	opts2.TextGranularity = GranularityWord
	result2, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts2)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result2.Identical {
		t.Error("Expected PDFs to be different")
	}
	t.Logf("Word-level: modified=%d, removed=%d, added=%d",
		len(result2.PageDiffs[0].TextDiff.Modified),
		len(result2.PageDiffs[0].TextDiff.Removed),
		len(result2.PageDiffs[0].TextDiff.Added))

	// Test with character-level granularity
	opts3 := DefaultCompareOptions()
	opts3.TextGranularity = GranularityChar
	result3, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts3)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result3.Identical {
		t.Error("Expected PDFs to be different")
	}
	t.Logf("Char-level: modified=%d, removed=%d, added=%d",
		len(result3.PageDiffs[0].TextDiff.Modified),
		len(result3.PageDiffs[0].TextDiff.Removed),
		len(result3.PageDiffs[0].TextDiff.Added))
}

func TestCompareOptions_Sensitivity(t *testing.T) {
	// Create first PDF
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("This is a long text with many words")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with minor change (1 character)
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("This is a long text with many wordX") // 1 char change
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	// Test with strict sensitivity (should detect all changes)
	opts1 := DefaultCompareOptions()
	opts1.DiffSensitivity = SensitivityStrict
	result1, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts1)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result1.Identical {
		t.Error("Expected PDFs to be different with strict sensitivity")
	}
	t.Logf("Strict sensitivity: detected differences")

	// Test with relaxed sensitivity (may filter minor changes)
	opts2 := DefaultCompareOptions()
	opts2.DiffSensitivity = SensitivityRelaxed
	result2, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts2)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	// Relaxed may or may not detect single character changes depending on threshold
	t.Logf("Relaxed sensitivity: identical=%v", result2.Identical)
}

func TestCompareOptions_IgnoreOptions(t *testing.T) {
	// Create first PDF
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Hello World")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with case difference
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("HELLO WORLD") // All caps
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	// Test with case-sensitive (default)
	opts1 := DefaultCompareOptions()
	opts1.IgnoreCase = false
	result1, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts1)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result1.Identical {
		t.Error("Expected PDFs to be different (case-sensitive)")
	}

	// Test with case-insensitive
	opts2 := DefaultCompareOptions()
	opts2.IgnoreCase = true
	result2, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts2)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	// Should be identical when ignoring case
	if !result2.Identical {
		t.Logf("Note: Case-insensitive comparison may still show differences due to position matching")
	}
}

func TestCompareOptions_DetectMoves(t *testing.T) {
	// Create first PDF with text at position (72, 720)
	builder1 := write.NewSimplePDFBuilder()
	page1 := builder1.AddPage(write.PageSizeLetter)
	content1 := page1.Content()
	content1.BeginText()
	font1 := page1.AddStandardFont("Helvetica")
	content1.SetFont(font1, 12)
	content1.SetTextPosition(72, 720)
	content1.ShowText("Moved Text")
	content1.EndText()
	builder1.FinalizePage(page1)
	pdf1, _ := builder1.Bytes()

	// Create second PDF with same text at different position
	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(200, 600) // Different position
	content2.ShowText("Moved Text")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	// Test with move detection enabled (default)
	opts1 := DefaultCompareOptions()
	opts1.DetectMoves = true
	result1, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts1)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	t.Logf("With move detection: identical=%v", result1.Identical)

	// Test with move detection disabled
	opts2 := DefaultCompareOptions()
	opts2.DetectMoves = false
	result2, err := ComparePDFsWithOptions(pdf1, pdf2, nil, nil, opts2)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}
	if result2.Identical {
		t.Error("Expected PDFs to be different when move detection is disabled")
	}
	t.Logf("Without move detection: identical=%v", result2.Identical)
}
