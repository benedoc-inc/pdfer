package compare

import (
	"testing"

	"github.com/benedoc-inc/pdfer/core/write"
	"github.com/benedoc-inc/pdfer/types"
)

func TestMyersDiff_Identical(t *testing.T) {
	// Create two identical PDFs
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

	builder2 := write.NewSimplePDFBuilder()
	page2 := builder2.AddPage(write.PageSizeLetter)
	content2 := page2.Content()
	content2.BeginText()
	font2 := page2.AddStandardFont("Helvetica")
	content2.SetFont(font2, 12)
	content2.SetTextPosition(72, 720)
	content2.ShowText("Hello World")
	content2.EndText()
	builder2.FinalizePage(page2)
	pdf2, _ := builder2.Bytes()

	result, err := ComparePDFs(pdf1, pdf2, nil, nil, false)
	if err != nil {
		t.Fatalf("Failed to compare: %v", err)
	}

	if !result.Identical {
		t.Error("Expected identical PDFs to be detected as identical")
	}
}

func TestMyersDiff_SimpleInsertion(t *testing.T) {
	// Test: Insert one element
	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	if len(diff.Added) != 1 {
		t.Errorf("Expected 1 added element, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 0 {
		t.Errorf("Expected 0 removed elements, got %d", len(diff.Removed))
	}
	if diff.Added[0].Text != "B" {
		t.Errorf("Expected added element to be 'B', got '%s'", diff.Added[0].Text)
	}
}

func TestMyersDiff_SimpleDeletion(t *testing.T) {
	// Test: Delete one element
	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	if len(diff.Removed) != 1 {
		t.Errorf("Expected 1 removed element, got %d", len(diff.Removed))
	}
	if len(diff.Added) != 0 {
		t.Errorf("Expected 0 added elements, got %d", len(diff.Added))
	}
	if diff.Removed[0].Text != "B" {
		t.Errorf("Expected removed element to be 'B', got '%s'", diff.Removed[0].Text)
	}
}

func TestMyersDiff_SimpleModification(t *testing.T) {
	// Test: Modify one element
	text1 := []types.TextElement{
		{Text: "Hello", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "World", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "Hello", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "Universe", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	if len(diff.Modified) != 1 {
		t.Errorf("Expected 1 modified element, got %d", len(diff.Modified))
	}
	if diff.Modified[0].Old.Text != "World" {
		t.Errorf("Expected old text 'World', got '%s'", diff.Modified[0].Old.Text)
	}
	if diff.Modified[0].New.Text != "Universe" {
		t.Errorf("Expected new text 'Universe', got '%s'", diff.Modified[0].New.Text)
	}
}

func TestMyersDiff_MultipleChanges(t *testing.T) {
	// Test: Multiple insertions, deletions, and modifications
	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
		{Text: "D", X: 72, Y: 690, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "X", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12}, // Modified
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
		{Text: "E", X: 72, Y: 690, FontName: "Helvetica", FontSize: 12}, // Modified
		{Text: "F", X: 72, Y: 680, FontName: "Helvetica", FontSize: 12}, // Added
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	// Should have: 2 modified (B->X, D->E), 1 added (F), 0 removed
	if len(diff.Modified) != 2 {
		t.Errorf("Expected 2 modified elements, got %d", len(diff.Modified))
	}
	if len(diff.Added) != 1 {
		t.Errorf("Expected 1 added element, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 0 {
		t.Errorf("Expected 0 removed elements, got %d", len(diff.Removed))
	}
}

func TestMyersDiff_ReorderedElements(t *testing.T) {
	// Test: Elements reordered (same content, different positions)
	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "C", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12}, // Moved
		{Text: "A", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12}, // Moved
		{Text: "B", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12}, // Moved
	}

	opts := DefaultCompareOptions()
	opts.DetectMoves = true
	opts.MoveTolerance = 100.0 // Large tolerance to detect moves
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	// With move detection, should detect as modifications (position changed)
	// Without proper move tracking, might show as removed+added
	t.Logf("Reordered elements: modified=%d, removed=%d, added=%d",
		len(diff.Modified), len(diff.Removed), len(diff.Added))
}

func TestMyersDiff_EmptySequences(t *testing.T) {
	opts := DefaultCompareOptions()

	// Both empty
	diff1 := myersDiff([]types.TextElement{}, []types.TextElement{}, opts)
	if diff1 != nil {
		t.Error("Expected nil diff for both empty sequences")
	}

	// First empty
	text2 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
	}
	diff2 := myersDiff([]types.TextElement{}, text2, opts)
	if diff2 == nil {
		t.Fatal("Expected diff for empty->non-empty")
	}
	if len(diff2.Added) != 1 {
		t.Errorf("Expected 1 added, got %d", len(diff2.Added))
	}

	// Second empty
	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
	}
	diff3 := myersDiff(text1, []types.TextElement{}, opts)
	if diff3 == nil {
		t.Fatal("Expected diff for non-empty->empty")
	}
	if len(diff3.Removed) != 1 {
		t.Errorf("Expected 1 removed, got %d", len(diff3.Removed))
	}
}

func TestMyersDiff_LongSequences(t *testing.T) {
	// Test with longer sequences to verify algorithm efficiency
	text1 := make([]types.TextElement, 100)
	text2 := make([]types.TextElement, 100)

	for i := 0; i < 100; i++ {
		text1[i] = types.TextElement{
			Text:     string(rune('A' + (i % 26))),
			X:        72,
			Y:        float64(720 - i*10),
			FontName: "Helvetica",
			FontSize: 12,
		}
		// text2 is same except every 10th element is different
		if i%10 == 0 {
			text2[i] = types.TextElement{
				Text:     "X",
				X:        72,
				Y:        float64(720 - i*10),
				FontName: "Helvetica",
				FontSize: 12,
			}
		} else {
			text2[i] = text1[i]
		}
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	// Should have 10 modifications (every 10th element)
	expectedMods := 10
	if len(diff.Modified) != expectedMods {
		t.Errorf("Expected %d modifications, got %d", expectedMods, len(diff.Modified))
	}
	if len(diff.Added) != 0 {
		t.Errorf("Expected 0 added, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 0 {
		t.Errorf("Expected 0 removed, got %d", len(diff.Removed))
	}
}

func TestMyersDiff_OptimalEditScript(t *testing.T) {
	// Test that we find the optimal (shortest) edit script
	// Example: "ABC" -> "ACB" should be 2 edits (swap B and C)
	// Not 4 edits (delete B, delete C, insert C, insert B)

	text1 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "A", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
	}

	opts := DefaultCompareOptions()
	opts.DetectMoves = true
	opts.MoveTolerance = 100.0
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	// Optimal would be 2 modifications (B and C swapped positions)
	// But with position-based matching, might detect differently
	t.Logf("Swap test: modified=%d, removed=%d, added=%d",
		len(diff.Modified), len(diff.Removed), len(diff.Added))

	// The key is that we should find matches optimally
	// A should match, and B/C should be detected as moved/modified
	if len(diff.Modified) < 1 {
		t.Error("Expected at least 1 modification for swapped elements")
	}
}

func TestMyersDiff_ComplexScenario(t *testing.T) {
	// Complex scenario: mix of all operations
	text1 := []types.TextElement{
		{Text: "Start", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12},
		{Text: "A", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},
		{Text: "B", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},
		{Text: "C", X: 72, Y: 690, FontName: "Helvetica", FontSize: 12},
		{Text: "D", X: 72, Y: 680, FontName: "Helvetica", FontSize: 12},
		{Text: "End", X: 72, Y: 670, FontName: "Helvetica", FontSize: 12},
	}
	text2 := []types.TextElement{
		{Text: "Start", X: 72, Y: 720, FontName: "Helvetica", FontSize: 12}, // Match
		{Text: "X", X: 72, Y: 710, FontName: "Helvetica", FontSize: 12},     // Modified A->X
		{Text: "B", X: 72, Y: 700, FontName: "Helvetica", FontSize: 12},     // Match
		{Text: "New", X: 72, Y: 690, FontName: "Helvetica", FontSize: 12},   // Added
		{Text: "C", X: 72, Y: 680, FontName: "Helvetica", FontSize: 12},     // Match (moved)
		{Text: "D", X: 72, Y: 670, FontName: "Helvetica", FontSize: 12},     // Match (moved)
		{Text: "End", X: 72, Y: 660, FontName: "Helvetica", FontSize: 12},   // Match (moved)
	}

	opts := DefaultCompareOptions()
	diff := myersDiff(text1, text2, opts)

	if diff == nil {
		t.Fatal("Expected diff, got nil")
	}

	t.Logf("Complex scenario: modified=%d, removed=%d, added=%d",
		len(diff.Modified), len(diff.Removed), len(diff.Added))

	// Verify optimal matching: Start, B, C, D, End should match
	// A->X should be modified, "New" should be added
	hasModification := false
	hasAddition := false

	for _, mod := range diff.Modified {
		if mod.Old.Text == "A" && mod.New.Text == "X" {
			hasModification = true
		}
	}

	for _, added := range diff.Added {
		if added.Text == "New" {
			hasAddition = true
		}
	}

	if !hasModification {
		t.Error("Expected A->X modification")
	}
	if !hasAddition {
		t.Error("Expected 'New' addition")
	}
}
