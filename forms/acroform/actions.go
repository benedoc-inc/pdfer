// Package acroform provides action support for form fields
package acroform

import (
	"fmt"
	"strings"

	"github.com/benedoc-inc/pdfer/v2/core/write"
)

// removeDictKey removes a single "/key value" pair from a PDF dict string.
// The value may be a name token (/Name), an object reference (N G R), an
// inline dict (<< … >>), or a literal string ((…)).  Surrounding whitespace is
// also consumed so the result remains well-formed.
//
// The search is key-boundary aware: "/A" will not match "/Annot".
func removeDictKey(dict, key string) string {
	needle := "/" + key
	search := dict
	base := 0
	for {
		idx := strings.Index(search, needle)
		if idx == -1 {
			return dict
		}
		abs := base + idx
		afterKey := abs + len(needle)

		// Boundary check: the character immediately after the key name must be
		// whitespace or a PDF structural character, not a letter/digit that
		// would indicate a longer key (e.g. /Annot when searching for /A).
		if afterKey < len(dict) {
			c := dict[afterKey]
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' &&
				c != '<' && c != '(' && c != '[' && c != '/' && c != '>' {
				// Partial match — advance and keep searching.
				base = afterKey
				search = dict[base:]
				continue
			}
		}

		// Found a complete key token at abs.
		// Skip whitespace between key and value.
		after := afterKey
		for after < len(dict) && (dict[after] == ' ' || dict[after] == '\t' || dict[after] == '\r' || dict[after] == '\n') {
			after++
		}
		if after >= len(dict) {
			return dict[:abs]
		}

		valueEnd := after
		switch dict[after] {
		case '<': // Inline dict << … >>
			if after+1 < len(dict) && dict[after+1] == '<' {
				end := findDictEnd(dict, after)
				if end != -1 {
					valueEnd = end + 2 // skip past >>
				}
			}
		case '(': // Literal string (…)
			depth := 0
			for i := after; i < len(dict); i++ {
				if dict[i] == '\\' {
					i++ // skip escaped char
					continue
				}
				if dict[i] == '(' {
					depth++
				} else if dict[i] == ')' {
					depth--
					if depth == 0 {
						valueEnd = i + 1
						break
					}
				}
			}
		default:
			// Name token (/Name) or numeric token — read until whitespace or
			// structural character.
			for valueEnd < len(dict) {
				c := dict[valueEnd]
				if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '/' || c == '>' || c == '[' {
					break
				}
				valueEnd++
			}
			// Object reference "N G R": if the first token is numeric, consume
			// two more whitespace-separated tokens (generation number + "R").
			token := dict[after:valueEnd]
			if len(token) > 0 && token[0] >= '0' && token[0] <= '9' {
				for i := 0; i < 2; i++ {
					s2 := valueEnd
					for s2 < len(dict) && (dict[s2] == ' ' || dict[s2] == '\t') {
						s2++
					}
					e2 := s2
					for e2 < len(dict) {
						c := dict[e2]
						if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '/' || c == '>' {
							break
						}
						e2++
					}
					if e2 > s2 {
						valueEnd = e2
					}
				}
			}
		}

		// Consume trailing whitespace after the value.
		for valueEnd < len(dict) && (dict[valueEnd] == ' ' || dict[valueEnd] == '\t' || dict[valueEnd] == '\r' || dict[valueEnd] == '\n') {
			valueEnd++
		}
		return dict[:abs] + dict[valueEnd:]
	}
}

// ActionType represents the type of PDF action
type ActionType string

const (
	ActionTypeGoTo       ActionType = "GoTo"       // Navigate to a page/destination
	ActionTypeURI        ActionType = "URI"        // Open a URI
	ActionTypeJavaScript ActionType = "JavaScript" // Execute JavaScript
	ActionTypeSubmit     ActionType = "SubmitForm" // Submit form
	ActionTypeReset      ActionType = "ResetForm"  // Reset form
)

// Action represents a PDF action
type Action struct {
	Type        ActionType
	URI         string // For URI actions
	JavaScript  string // For JavaScript actions
	Destination string // For GoTo actions (e.g., "1 0 R" or page number)
	PageNum     int    // For GoTo actions (page number)
}

// AddActionToField adds an action to a field
func AddActionToField(fieldObjNum int, action *Action, w *write.PDFWriter) error {
	// Get existing field object
	fieldData, err := w.GetObject(fieldObjNum)
	if err != nil {
		return fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)

	// Create action dictionary
	var actionDict string
	switch action.Type {
	case ActionTypeURI:
		actionDict = fmt.Sprintf("<< /Type /Action /S /URI /URI (%s) >>", escapeActionString(action.URI))
	case ActionTypeJavaScript:
		actionDict = fmt.Sprintf("<< /Type /Action /S /JavaScript /JS (%s) >>", escapeActionString(action.JavaScript))
	case ActionTypeGoTo:
		if action.Destination != "" {
			actionDict = fmt.Sprintf("<< /Type /Action /S /GoTo /D %s >>", action.Destination)
		} else if action.PageNum > 0 {
			// Create destination reference
			actionDict = fmt.Sprintf("<< /Type /Action /S /GoTo /D [%d 0 R /Fit] >>", action.PageNum)
		} else {
			return fmt.Errorf("GoTo action requires either Destination or PageNum")
		}
	case ActionTypeSubmit:
		actionDict = "<< /Type /Action /S /SubmitForm /F << /F (submit.pdf) /FS /URL >> >>"
	case ActionTypeReset:
		actionDict = "<< /Type /Action /S /ResetForm >>"
	default:
		return fmt.Errorf("unsupported action type: %s", action.Type)
	}

	// Add /AA (additional actions) or /A (action) to field
	// /A is for the main action, /AA is for event-specific actions
	actionEntry := fmt.Sprintf("/A %s", actionDict)

	// Remove any existing /A entry before inserting the new one.
	if strings.Contains(fieldStr, "/A ") || strings.Contains(fieldStr, "/AA ") {
		fieldStr = removeDictKey(fieldStr, "A")
	}

	// Add action before the outermost closing >>
	dictEnd := findDictEnd(fieldStr, strings.Index(fieldStr, "<<"))
	if dictEnd == -1 {
		return fmt.Errorf("field dictionary not found")
	}
	fieldStr = fieldStr[:dictEnd] + actionEntry + " " + fieldStr[dictEnd:]

	// Update object
	w.SetObject(fieldObjNum, []byte(fieldStr))
	return nil
}

// AddMouseAction adds a mouse-triggered action (e.g., onClick)
func AddMouseAction(fieldObjNum int, event string, action *Action, w *write.PDFWriter) error {
	// Get existing field object
	fieldData, err := w.GetObject(fieldObjNum)
	if err != nil {
		return fmt.Errorf("failed to get field object: %w", err)
	}

	fieldStr := string(fieldData)

	// Create action dictionary
	var actionDict string
	switch action.Type {
	case ActionTypeURI:
		actionDict = fmt.Sprintf("<< /Type /Action /S /URI /URI (%s) >>", escapeActionString(action.URI))
	case ActionTypeJavaScript:
		actionDict = fmt.Sprintf("<< /Type /Action /S /JavaScript /JS (%s) >>", escapeActionString(action.JavaScript))
	default:
		return fmt.Errorf("unsupported action type for mouse event: %s", action.Type)
	}

	// Map event name to PDF action key
	eventMap := map[string]string{
		"onClick":     "U", // Mouse up
		"onMouseUp":   "U",
		"onMouseDown": "D",
		"onEnter":     "E", // Mouse enter
		"onExit":      "X", // Mouse exit
	}

	actionKey, ok := eventMap[event]
	if !ok {
		return fmt.Errorf("unsupported event: %s", event)
	}

	// Add to /AA dictionary
	aaEntry := fmt.Sprintf("/AA << /%s %s >>", actionKey, actionDict)

	// Remove any existing /AA entry, then re-insert the updated one.
	if strings.Contains(fieldStr, "/AA ") {
		fieldStr = removeDictKey(fieldStr, "AA")
	}

	// Add /AA before the outermost closing >>
	dictEnd := findDictEnd(fieldStr, strings.Index(fieldStr, "<<"))
	if dictEnd == -1 {
		return fmt.Errorf("field dictionary not found")
	}
	fieldStr = fieldStr[:dictEnd] + aaEntry + " " + fieldStr[dictEnd:]

	// Update object
	w.SetObject(fieldObjNum, []byte(fieldStr))
	return nil
}

// escapeActionString escapes strings for action dictionaries
func escapeActionString(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			result.WriteString("\\\\")
		case '(':
			result.WriteString("\\(")
		case ')':
			result.WriteString("\\)")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		default:
			if r > 127 {
				result.WriteString(fmt.Sprintf("\\%03o", r))
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}
