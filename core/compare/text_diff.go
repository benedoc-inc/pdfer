package compare

import (
	"github.com/benedoc-inc/pdfer/types"
)

// compareText compares text elements between two pages using advanced diff algorithms
// Granularity and sensitivity are controlled via CompareOptions
func compareText(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	tolerance := opts.TextTolerance
	moveTolerance := opts.MoveTolerance
	if moveTolerance == 0 {
		moveTolerance = tolerance * 10.0 // Auto: 10x base tolerance
	}
	diff := &TextDiff{
		Added:    []types.TextElement{},
		Removed:  []types.TextElement{},
		Modified: []TextModification{},
	}

	// Strategy: Use a hybrid approach
	// 1. First, match by position (handles exact matches and position-based modifications)
	// 2. Then, use content-based matching for text that moved or was reordered
	// 3. Finally, use Myers diff algorithm for semantic content comparison

	text1Matched := make(map[int]bool)
	text2Matched := make(map[int]bool)

	// Phase 1: Exact matches (same text, same position within tolerance)
	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			if t1.Text == t2.Text && positionsMatch(t1.X, t1.Y, t2.X, t2.Y, tolerance) {
				if t1.FontName == t2.FontName && abs(t1.FontSize-t2.FontSize) < 0.1 {
					text1Matched[i1] = true
					text2Matched[i2] = true
					break
				}
			}
		}
	}

	// Phase 2: Position-based modifications (same position, different text)
	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			if positionsMatch(t1.X, t1.Y, t2.X, t2.Y, tolerance) && t1.Text != t2.Text {
				diff.Modified = append(diff.Modified, TextModification{Old: t1, New: t2})
				text1Matched[i1] = true
				text2Matched[i2] = true
				break
			}
		}
	}

	// Phase 3: Content-based matching (same text, different position - moved text)
	// Only if DetectMoves is enabled
	if opts.DetectMoves {
		for i1, t1 := range text1 {
			if text1Matched[i1] {
				continue
			}
			for i2, t2 := range text2 {
				if text2Matched[i2] {
					continue
				}
				// Normalize text for comparison if options require it
				text1Norm := normalizeTextForComparison(t1.Text, opts)
				text2Norm := normalizeTextForComparison(t2.Text, opts)

				// Same text content but different position - likely moved
				// Only match if positions are still somewhat close (within moveTolerance)
				if text1Norm == text2Norm && t1.FontName == t2.FontName && abs(t1.FontSize-t2.FontSize) < 0.1 {
					// Check if positions are within move tolerance (allows for small moves)
					if positionsMatch(t1.X, t1.Y, t2.X, t2.Y, moveTolerance) {
						text1Matched[i1] = true
						text2Matched[i2] = true
						// Mark as modified since position changed (even if slightly)
						diff.Modified = append(diff.Modified, TextModification{Old: t1, New: t2})
						break
					}
				}
			}
		}
	}

	// Phase 4: Apply diff algorithm based on granularity for remaining unmatched text
	unmatched1 := make([]types.TextElement, 0)
	unmatched2 := make([]types.TextElement, 0)
	for i1, t1 := range text1 {
		if !text1Matched[i1] {
			unmatched1 = append(unmatched1, t1)
		}
	}
	for i2, t2 := range text2 {
		if !text2Matched[i2] {
			unmatched2 = append(unmatched2, t2)
		}
	}

	// Apply diff algorithm based on granularity
	if len(unmatched1) > 0 || len(unmatched2) > 0 {
		var textDiff *TextDiff
		switch opts.TextGranularity {
		case GranularityWord:
			textDiff = wordLevelDiff(unmatched1, unmatched2, opts)
		case GranularityChar:
			textDiff = charLevelDiff(unmatched1, unmatched2, opts)
		default: // GranularityElement
			textDiff = elementLevelDiff(unmatched1, unmatched2, opts)
		}

		// Merge results
		diff.Added = append(diff.Added, textDiff.Added...)
		diff.Removed = append(diff.Removed, textDiff.Removed...)
		diff.Modified = append(diff.Modified, textDiff.Modified...)
	}

	// Apply sensitivity filtering
	diff = applySensitivityFilter(diff, opts)

	if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Modified) == 0 {
		return nil
	}

	return diff
}

// myersDiffText applies Myers diff algorithm to text elements
// This finds the optimal matching between two sequences of text elements
func myersDiffText(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	tolerance := opts.TextTolerance
	diff := &TextDiff{
		Added:    []types.TextElement{},
		Removed:  []types.TextElement{},
		Modified: []TextModification{},
	}

	if len(text1) == 0 {
		diff.Added = text2
		return diff
	}
	if len(text2) == 0 {
		diff.Removed = text1
		return diff
	}

	// Build a map of text content to positions for efficient lookup
	// This helps us find similar text even if positions differ
	text1Map := make(map[string][]int) // text -> indices in text1
	for i, t := range text1 {
		key := normalizeTextForComparison(t.Text, opts)
		text1Map[key] = append(text1Map[key], i)
	}

	// Match text2 elements to text1 elements
	text1Matched := make(map[int]bool)
	text2Matched := make(map[int]bool)

	// First, try exact text matches
	for i2, t2 := range text2 {
		if text2Matched[i2] {
			continue
		}
		key := normalizeTextForComparison(t2.Text, opts)
		if indices, exists := text1Map[key]; exists {
			// Find best match (considering position similarity if available)
			bestMatch := -1
			bestScore := -1.0
			for _, i1 := range indices {
				if text1Matched[i1] {
					continue
				}
				// Score based on position similarity and font match
				score := 0.0
				if positionsMatch(text1[i1].X, text1[i1].Y, t2.X, t2.Y, tolerance*10) {
					score += 10.0 // Position match bonus
				}
				if text1[i1].FontName == t2.FontName {
					score += 5.0
				}
				if abs(text1[i1].FontSize-t2.FontSize) < 0.1 {
					score += 5.0
				}
				if score > bestScore {
					bestScore = score
					bestMatch = i1
				}
			}
			if bestMatch >= 0 {
				text1Matched[bestMatch] = true
				text2Matched[i2] = true
				// Check if it's a modification (different position or formatting)
				if !positionsMatch(text1[bestMatch].X, text1[bestMatch].Y, t2.X, t2.Y, tolerance) ||
					text1[bestMatch].FontName != t2.FontName ||
					abs(text1[bestMatch].FontSize-t2.FontSize) >= 0.1 {
					diff.Modified = append(diff.Modified, TextModification{
						Old: text1[bestMatch],
						New: t2,
					})
				}
				// Otherwise it's an exact match (already handled in Phase 1-3)
			}
		}
	}

	// Remaining unmatched elements
	for i2, t2 := range text2 {
		if !text2Matched[i2] {
			diff.Added = append(diff.Added, t2)
		}
	}
	for i1, t1 := range text1 {
		if !text1Matched[i1] {
			diff.Removed = append(diff.Removed, t1)
		}
	}

	return diff
}

// normalizeTextForComparison normalizes text based on comparison options
func normalizeTextForComparison(text string, opts CompareOptions) string {
	normalized := text
	if opts.IgnoreCase {
		normalized = ""
		for _, r := range text {
			if r >= 'A' && r <= 'Z' {
				normalized += string(r + 32) // to lowercase
			} else {
				normalized += string(r)
			}
		}
	}
	if opts.IgnoreWhitespace {
		result := ""
		for _, r := range normalized {
			if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
				result += string(r)
			}
		}
		normalized = result
	}
	return normalized
}

// normalizeText normalizes text for comparison (lowercase, trim whitespace)
// Deprecated: Use normalizeTextForComparison instead
func normalizeText(text string) string {
	normalized := ""
	for _, r := range text {
		if r >= 'A' && r <= 'Z' {
			normalized += string(r + 32) // to lowercase
		} else if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			normalized += string(r)
		}
		// Skip whitespace for comparison
	}
	return normalized
}

// elementLevelDiff compares text at element level (default behavior)
// Uses the full Myers O(ND) algorithm for optimal matching
func elementLevelDiff(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	return myersDiff(text1, text2, opts)
}

// wordLevelDiff compares text word-by-word within elements
func wordLevelDiff(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	diff := &TextDiff{
		Added:    []types.TextElement{},
		Removed:  []types.TextElement{},
		Modified: []TextModification{},
	}

	// For word-level, we compare the text content word-by-word
	// If elements have the same position, compare their words
	text1Matched := make(map[int]bool)
	text2Matched := make(map[int]bool)

	// Match elements by position first
	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			if positionsMatch(t1.X, t1.Y, t2.X, t2.Y, opts.TextTolerance) {
				// Same position - compare words
				words1 := splitWords(normalizeTextForComparison(t1.Text, opts))
				words2 := splitWords(normalizeTextForComparison(t2.Text, opts))

				if len(words1) == len(words2) && wordsEqual(words1, words2) {
					// All words match
					text1Matched[i1] = true
					text2Matched[i2] = true
				} else {
					// Words differ - mark as modified
					diff.Modified = append(diff.Modified, TextModification{Old: t1, New: t2})
					text1Matched[i1] = true
					text2Matched[i2] = true
				}
				break
			}
		}
	}

	// Remaining unmatched
	for i2, t2 := range text2 {
		if !text2Matched[i2] {
			diff.Added = append(diff.Added, t2)
		}
	}
	for i1, t1 := range text1 {
		if !text1Matched[i1] {
			diff.Removed = append(diff.Removed, t1)
		}
	}

	return diff
}

// charLevelDiff compares text character-by-character
func charLevelDiff(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	diff := &TextDiff{
		Added:    []types.TextElement{},
		Removed:  []types.TextElement{},
		Modified: []TextModification{},
	}

	// For character-level, compare text content character-by-character
	text1Matched := make(map[int]bool)
	text2Matched := make(map[int]bool)

	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			if positionsMatch(t1.X, t1.Y, t2.X, t2.Y, opts.TextTolerance) {
				// Same position - compare characters
				text1Norm := normalizeTextForComparison(t1.Text, opts)
				text2Norm := normalizeTextForComparison(t2.Text, opts)

				if text1Norm == text2Norm {
					text1Matched[i1] = true
					text2Matched[i2] = true
				} else {
					// Characters differ - mark as modified
					diff.Modified = append(diff.Modified, TextModification{Old: t1, New: t2})
					text1Matched[i1] = true
					text2Matched[i2] = true
				}
				break
			}
		}
	}

	// Remaining unmatched
	for i2, t2 := range text2 {
		if !text2Matched[i2] {
			diff.Added = append(diff.Added, t2)
		}
	}
	for i1, t1 := range text1 {
		if !text1Matched[i1] {
			diff.Removed = append(diff.Removed, t1)
		}
	}

	return diff
}

// applySensitivityFilter filters differences based on sensitivity level
func applySensitivityFilter(diff *TextDiff, opts CompareOptions) *TextDiff {
	if diff == nil {
		return nil
	}

	switch opts.DiffSensitivity {
	case SensitivityRelaxed:
		// Only keep significant changes (more than threshold)
		filtered := &TextDiff{
			Added:    []types.TextElement{},
			Removed:  []types.TextElement{},
			Modified: []TextModification{},
		}
		for _, mod := range diff.Modified {
			// Only keep if change is significant (e.g., >10% different)
			oldNorm := normalizeTextForComparison(mod.Old.Text, opts)
			newNorm := normalizeTextForComparison(mod.New.Text, opts)
			changeRatio := calculateChangeRatio(oldNorm, newNorm)
			if changeRatio > 0.1 { // More than 10% change
				filtered.Modified = append(filtered.Modified, mod)
			}
		}
		filtered.Added = diff.Added
		filtered.Removed = diff.Removed
		return filtered
	case SensitivityStrict:
		// Keep all changes
		return diff
	default: // SensitivityNormal
		// Keep all changes (could add filtering for very minor changes)
		return diff
	}
}

// Helper functions
func splitWords(text string) []string {
	words := []string{}
	current := ""
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			current += string(r)
		} else {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		}
	}
	if current != "" {
		words = append(words, current)
	}
	return words
}

func wordsEqual(words1, words2 []string) bool {
	if len(words1) != len(words2) {
		return false
	}
	for i := range words1 {
		if words1[i] != words2[i] {
			return false
		}
	}
	return true
}

func calculateChangeRatio(text1, text2 string) float64 {
	if len(text1) == 0 && len(text2) == 0 {
		return 0.0
	}
	maxLen := len(text1)
	if len(text2) > maxLen {
		maxLen = len(text2)
	}
	if maxLen == 0 {
		return 0.0
	}
	// Simple Levenshtein-like ratio
	diffs := 0
	minLen := len(text1)
	if len(text2) < minLen {
		minLen = len(text2)
	}
	for i := 0; i < minLen; i++ {
		if text1[i] != text2[i] {
			diffs++
		}
	}
	diffs += absInt(len(text1) - len(text2))
	return float64(diffs) / float64(maxLen)
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
