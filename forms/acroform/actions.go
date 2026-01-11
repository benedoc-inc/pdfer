// Package acroform provides action support for form fields
package acroform

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/benedoc-inc/pdfer/core/write"
)

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

	// Check if /A or /AA already exists
	if strings.Contains(fieldStr, "/A ") || strings.Contains(fieldStr, "/AA ") {
		// Replace existing action (simplified - would need better parsing)
		// For now, just add /A
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("field dictionary not found")
		}
		fieldStr = fieldStr[:dictEnd] + actionEntry + " " + fieldStr[dictEnd:]
	} else {
		// Add action before closing >>
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("field dictionary not found")
		}
		fieldStr = fieldStr[:dictEnd] + actionEntry + " " + fieldStr[dictEnd:]
	}

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

	// Check if /AA exists
	if strings.Contains(fieldStr, "/AA ") {
		// Update existing /AA (simplified)
		aaPattern := regexp.MustCompile(`/AA\s*<<[^>]*>>`)
		fieldStr = aaPattern.ReplaceAllString(fieldStr, aaEntry)
	} else {
		// Add /AA before closing >>
		dictEnd := strings.LastIndex(fieldStr, ">>")
		if dictEnd == -1 {
			return fmt.Errorf("field dictionary not found")
		}
		fieldStr = fieldStr[:dictEnd] + aaEntry + " " + fieldStr[dictEnd:]
	}

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
