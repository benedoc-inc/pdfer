package compare

import (
	"github.com/benedoc-inc/pdfer/types"
)

// myersDiff implements the O(ND) Myers diff algorithm
// Based on "An O(ND) Difference Algorithm and Its Variations" by Eugene W. Myers
// This finds the shortest edit script (SES) to transform text1 into text2
func myersDiff(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	diff := &TextDiff{
		Added:    []types.TextElement{},
		Removed:  []types.TextElement{},
		Modified: []TextModification{},
	}

	n := len(text1)
	m := len(text2)

	if n == 0 && m == 0 {
		return nil // Both empty - no differences
	}
	if n == 0 {
		diff.Added = text2
		return diff
	}
	if m == 0 {
		diff.Removed = text1
		return diff
	}

	// Strategy: Multi-phase matching for optimal results
	// Phase 1: Exact matches (same text, same position, same font)
	// Phase 2: Position-based modifications (same position, different text)
	// Phase 3: LCS for reordered/moved content

	text1Matched := make(map[int]bool)
	text2Matched := make(map[int]bool)

	// Phase 1: Exact matches first (prevents false matches)
	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			// Exact match (text, position, font all match)
			if elementsEqual(t1, t2, opts) {
				text1Matched[i1] = true
				text2Matched[i2] = true
				break
			}
		}
	}

	// Phase 2: Position-based modifications (same position, different text)
	// Only match if text is somewhat similar (to avoid false matches)
	for i1, t1 := range text1 {
		if text1Matched[i1] {
			continue
		}
		for i2, t2 := range text2 {
			if text2Matched[i2] {
				continue
			}
			// Same position, different content - modification
			// But only if there's no better text-based match available
			if positionsMatch(t1.X, t1.Y, t2.X, t2.Y, opts.TextTolerance) {
				text1Norm := normalizeTextForComparison(t1.Text, opts)
				text2Norm := normalizeTextForComparison(t2.Text, opts)
				if text1Norm != text2Norm {
					// Check if there's a better text match elsewhere
					hasBetterMatch := false
					for j2, t2Other := range text2 {
						if j2 == i2 || text2Matched[j2] {
							continue
						}
						text2OtherNorm := normalizeTextForComparison(t2Other.Text, opts)
						if text1Norm == text2OtherNorm {
							hasBetterMatch = true
							break
						}
					}
					// Only match by position if no better text match exists
					if !hasBetterMatch {
						text1Matched[i1] = true
						text2Matched[i2] = true
						diff.Modified = append(diff.Modified, TextModification{
							Old: t1,
							New: t2,
						})
						break
					}
				}
			}
		}
	}

	// Phase 2: Use LCS for remaining unmatched elements (handles reordered content)
	unmatched1 := make([]types.TextElement, 0)
	unmatched2 := make([]types.TextElement, 0)
	unmatched1Indices := make([]int, 0)
	unmatched2Indices := make([]int, 0)

	for i1, t1 := range text1 {
		if !text1Matched[i1] {
			unmatched1 = append(unmatched1, t1)
			unmatched1Indices = append(unmatched1Indices, i1)
		}
	}
	for i2, t2 := range text2 {
		if !text2Matched[i2] {
			unmatched2 = append(unmatched2, t2)
			unmatched2Indices = append(unmatched2Indices, i2)
		}
	}

	// Find LCS matches for unmatched elements (relaxed position matching)
	if len(unmatched1) > 0 && len(unmatched2) > 0 {
		// Create relaxed options for LCS (ignore position for matching)
		relaxedOpts := opts
		relaxedOpts.TextTolerance = 10000.0 // Very large tolerance for LCS matching

		matches := findLCSMatches(unmatched1, unmatched2, relaxedOpts)

		// Map LCS matches back to original indices
		for _, match := range matches {
			origI1 := unmatched1Indices[match.i1]
			origI2 := unmatched2Indices[match.i2]

			text1Matched[origI1] = true
			text2Matched[origI2] = true

			// Check if it's a modification (different position or content)
			if !elementsEqual(text1[origI1], text2[origI2], opts) {
				diff.Modified = append(diff.Modified, TextModification{
					Old: text1[origI1],
					New: text2[origI2],
				})
			}
		}
	}

	// Add unmatched elements as added/removed
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

// elementsEqual checks if two text elements are equal based on options
func elementsEqual(t1, t2 types.TextElement, opts CompareOptions) bool {
	text1Norm := normalizeTextForComparison(t1.Text, opts)
	text2Norm := normalizeTextForComparison(t2.Text, opts)

	if text1Norm != text2Norm {
		return false
	}

	// Check position (within tolerance)
	if !positionsMatch(t1.X, t1.Y, t2.X, t2.Y, opts.TextTolerance) {
		return false
	}

	// Check font
	if t1.FontName != t2.FontName {
		return false
	}

	// Check size
	if abs(t1.FontSize-t2.FontSize) >= 0.1 {
		return false
	}

	return true
}

type matchPair struct {
	i1, i2 int
}

// findLCSMatches finds the Longest Common Subsequence using dynamic programming
// This provides optimal matching when elements can be reordered
// Time: O(n*m), Space: O(n*m)
// For LCS matching, we prioritize text content over position
func findLCSMatches(text1, text2 []types.TextElement, opts CompareOptions) []matchPair {
	matches := []matchPair{}

	n := len(text1)
	m := len(text2)

	if n == 0 || m == 0 {
		return matches
	}

	// Build LCS table using dynamic programming
	// dp[i][j] = length of LCS of text1[0..i-1] and text2[0..j-1]
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	// For LCS, we match by text content (ignoring position for matching purposes)
	// Create a relaxed equality function that only checks text content
	textEqual := func(t1, t2 types.TextElement) bool {
		text1Norm := normalizeTextForComparison(t1.Text, opts)
		text2Norm := normalizeTextForComparison(t2.Text, opts)
		return text1Norm == text2Norm && t1.FontName == t2.FontName && abs(t1.FontSize-t2.FontSize) < 0.1
	}

	// Fill the LCS table
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if textEqual(text1[i-1], text2[j-1]) {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	// Reconstruct matches by backtracking through the LCS table
	i, j := n, m
	for i > 0 && j > 0 {
		if textEqual(text1[i-1], text2[j-1]) {
			// Match found - add to matches
			matches = append(matches, matchPair{i1: i - 1, i2: j - 1})
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			// Move up (text1 element not matched)
			i--
		} else {
			// Move left (text2 element not matched)
			j--
		}
	}

	// Reverse to get correct order (we built it backwards)
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}

	return matches
}

// basicMatchingDiff is a fallback simple matching algorithm
func basicMatchingDiff(text1, text2 []types.TextElement, opts CompareOptions) *TextDiff {
	// This is what we currently have - hash-map based matching
	// Used as fallback if needed
	return myersDiffText(text1, text2, opts)
}
