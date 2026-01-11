package extract

import "strings"

// extractInlineDict extracts an inline dictionary value from a PDF dictionary string
// For example, from "/Resources<<...>>", extracts the "<<...>>" part
func extractInlineDict(dictStr, key string) string {
	keyIdx := strings.Index(dictStr, key)
	if keyIdx == -1 {
		return ""
	}

	// Find the dictionary after the key
	dictStart := keyIdx + len(key)
	for dictStart < len(dictStr) && (dictStr[dictStart] == ' ' || dictStr[dictStart] == '\n' || dictStr[dictStart] == '\r') {
		dictStart++
	}

	// Check if it's a dictionary <<...>>
	if dictStart+1 >= len(dictStr) || dictStr[dictStart] != '<' || dictStr[dictStart+1] != '<' {
		return ""
	}

	// Find matching >>
	dictEnd := dictStart + 2
	depth := 1
	for dictEnd+1 < len(dictStr) && depth > 0 {
		if dictStr[dictEnd] == '>' && dictStr[dictEnd+1] == '>' {
			depth--
			if depth == 0 {
				// Return the dictionary content (without << >>)
				return dictStr[dictStart+2 : dictEnd]
			}
			dictEnd += 2
		} else if dictEnd+1 < len(dictStr) && dictStr[dictEnd] == '<' && dictStr[dictEnd+1] == '<' {
			depth++
			dictEnd += 2
		} else {
			dictEnd++
		}
	}

	if depth == 0 {
		return dictStr[dictStart+2 : dictEnd]
	}

	return ""
}
