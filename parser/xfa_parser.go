package parser

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseXFAFields parses XFA XML and extracts form fields, returning FormField types
// This is general-purpose parsing - it extracts fields from XFA XML regardless of source
func ParseXFAFields(xfaXML string, verbose bool) ([]types.FormField, error) {
	var fields []types.FormField
	fieldIndex := 1

	// Count fields
	fieldCount := strings.Count(xfaXML, "<field")
	if verbose {
		log.Printf("Found %d <field> tags in XFA XML", fieldCount)
	}

	if fieldCount == 0 {
		return fields, nil
	}

	// Use XML decoder with error recovery
	decoder := xml.NewDecoder(strings.NewReader(xfaXML))
	decoder.Strict = false // Allow malformed XML

	var currentField *types.FormField
	var currentName string
	var currentValue string
	var currentRequired string
	var currentLabel string
	var inField bool
	var inValue bool
	var inName bool
	var inLabel bool

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			// Continue on errors - XFA XML can be malformed
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			// Handle field elements (ignore namespace)
			if se.Name.Local == "field" {
				inField = true
				currentField = &types.FormField{
					ID:         fmt.Sprintf("field_%d", fieldIndex),
					Type:       "text", // default
					Required:   false,
					ReadOnly:   false,
					Properties: make(map[string]interface{}),
				}

				// Get attributes
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						currentName = attr.Value
						currentField.Name = attr.Value
						currentField.FullName = attr.Value
						currentField.ID = sanitizeFieldID(attr.Value)
					case "type":
						currentField.Type = mapXFATypeToFieldType(attr.Value)
					case "required":
						currentRequired = attr.Value
					case "access":
						if attr.Value == "readOnly" {
							currentField.ReadOnly = true
						}
					case "h", "w", "x", "y":
						// Store layout properties
						currentField.Properties[attr.Name.Local] = attr.Value
					}
				}
			} else if se.Name.Local == "value" && inField {
				inValue = true
				currentValue = ""
			} else if se.Name.Local == "name" && inField {
				inName = true
			} else if se.Name.Local == "label" && inField {
				inLabel = true
			}
		case xml.EndElement:
			if se.Name.Local == "field" {
				if currentField != nil {
					// Finalize field
					if currentField.Name == "" {
						if currentName != "" {
							currentField.Name = currentName
							currentField.FullName = currentName
							currentField.ID = sanitizeFieldID(currentName)
						} else {
							currentField.Name = fmt.Sprintf("field_%d", fieldIndex)
							currentField.FullName = currentField.Name
							currentField.ID = fmt.Sprintf("field_%d", fieldIndex)
						}
					}

					if currentField.Type == "" {
						currentField.Type = "text"
					}

					if currentValue != "" {
						currentField.DefaultValue = strings.TrimSpace(currentValue)
					}

					if currentLabel != "" {
						// Store label in properties if needed
						currentField.Properties["label"] = strings.TrimSpace(currentLabel)
					}

					// Set required flag
					required := currentRequired == "1" || strings.ToLower(currentRequired) == "true" || strings.ToLower(currentRequired) == "required"
					currentField.Required = required

					fields = append(fields, *currentField)
					fieldIndex++
				}
				inField = false
				currentField = nil
				currentName = ""
				currentValue = ""
				currentRequired = ""
				currentLabel = ""
			} else if se.Name.Local == "value" {
				inValue = false
				if currentField != nil && currentValue != "" {
					currentField.DefaultValue = strings.TrimSpace(currentValue)
				}
			} else if se.Name.Local == "name" {
				inName = false
			} else if se.Name.Local == "label" {
				inLabel = false
			}
		case xml.CharData:
			if inValue && inField {
				currentValue += string(se)
			} else if inName && currentField != nil {
				currentName = strings.TrimSpace(string(se))
				if currentName != "" {
					currentField.Name = currentName
					currentField.FullName = currentName
					currentField.ID = sanitizeFieldID(currentName)
				}
			} else if inLabel && inField {
				currentLabel += string(se)
			}
		}
	}

	if verbose {
		log.Printf("Parsed %d form fields from XFA XML", len(fields))
	}

	return fields, nil
}

// mapXFATypeToFieldType maps XFA field types to FormField type strings
func mapXFATypeToFieldType(xfaType string) string {
	switch strings.ToLower(xfaType) {
	case "text", "textfield":
		return "text"
	case "textarea", "textedit":
		return "textarea"
	case "radio", "radiobutton":
		return "radio"
	case "checkbox", "checkbutton":
		return "checkbox"
	case "select", "choice", "dropdown":
		return "select"
	case "numeric", "decimal", "integer":
		return "number"
	case "date", "datefield":
		return "date"
	case "email", "emailfield":
		return "email"
	case "button":
		return "button"
	case "signature", "signaturefield":
		return "signature"
	default:
		return "text" // Default to text
	}
}

// sanitizeFieldID creates a valid ID from a field name
func sanitizeFieldID(name string) string {
	// Remove special characters, keep alphanumeric and underscores
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		} else if r == ' ' || r == '-' {
			result.WriteRune('_')
		}
	}
	id := result.String()
	if id == "" {
		id = "field"
	}
	// Ensure it doesn't start with a number
	if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
		id = "field_" + id
	}
	return id
}
