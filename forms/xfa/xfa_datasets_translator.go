package xfa

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// ParseXFADatasets parses XFA datasets XML and converts it to XFADatasets type
func ParseXFADatasets(xfaDatasetsXML string, verbose bool) (*types.XFADatasets, error) {
	if verbose {
		log.Printf("Parsing XFA datasets XML (length: %d bytes)", len(xfaDatasetsXML))
	}

	datasets := &types.XFADatasets{
		Fields: make(map[string]interface{}),
		Groups: make([]types.XFAFieldGroup, 0),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaDatasetsXML))
	decoder.Strict = false

	var currentGroup *types.XFAFieldGroup
	var currentFieldName string
	var currentFieldValue strings.Builder
	var inData bool
	var inField bool
	var inValue bool
	var inGroup bool
	var groupStack []*types.XFAFieldGroup

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if verbose {
				log.Printf("XML parse error (continuing): %v", err)
			}
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			localName := se.Name.Local

			switch localName {
			case "data":
				inData = true
			case "field":
				inField = true
				currentFieldValue.Reset()
				for _, attr := range se.Attr {
					if attr.Name.Local == "name" {
						currentFieldName = attr.Value
					}
				}
			case "value":
				if inField {
					inValue = true
					currentFieldValue.Reset()
				}
			default:
				// Treat unknown elements as groups if they're not standard XFA elements
				if inData && !isStandardXFAElement(localName) {
					inGroup = true
					group := &types.XFAFieldGroup{
						Name:   localName,
						Fields: make(map[string]interface{}),
						Groups: make([]types.XFAFieldGroup, 0),
					}
					if currentGroup != nil {
						// Nested group
						groupStack = append(groupStack, currentGroup)
					}
					currentGroup = group
				}
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "data":
				inData = false
			case "field":
				if currentFieldName != "" {
					value := strings.TrimSpace(currentFieldValue.String())
					// Try to parse as number or boolean
					parsedValue := parseFieldValue(value)

					if currentGroup != nil {
						currentGroup.Fields[currentFieldName] = parsedValue
					} else {
						datasets.Fields[currentFieldName] = parsedValue
					}
					if verbose {
						log.Printf("Parsed field: %s = %v", currentFieldName, parsedValue)
					}
				}
				inField = false
				currentFieldName = ""
				currentFieldValue.Reset()
			case "value":
				inValue = false
			default:
				// End of group
				if inGroup && currentGroup != nil && localName == currentGroup.Name {
					if len(groupStack) > 0 {
						// Add to parent group
						parent := groupStack[len(groupStack)-1]
						parent.Groups = append(parent.Groups, *currentGroup)
						groupStack = groupStack[:len(groupStack)-1]
						currentGroup = parent
					} else {
						// Top-level group
						datasets.Groups = append(datasets.Groups, *currentGroup)
						currentGroup = nil
					}
					inGroup = false
				}
			}

		case xml.CharData:
			if inValue && inField {
				currentFieldValue.WriteString(string(se))
			}
		}
	}

	if verbose {
		log.Printf("Parsed XFA datasets: %d fields, %d groups", len(datasets.Fields), len(datasets.Groups))
	}

	return datasets, nil
}

// parseFieldValue attempts to parse a field value as number or boolean
func parseFieldValue(value string) interface{} {
	if value == "" {
		return ""
	}

	// Try boolean
	if value == "true" || value == "1" {
		return true
	}
	if value == "false" || value == "0" {
		return false
	}

	// Try integer
	if intVal, err := strconv.Atoi(value); err == nil {
		return intVal
	}

	// Try float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Return as string
	return value
}

// DatasetsToXFA converts XFADatasets to XFA XML
func DatasetsToXFA(datasets *types.XFADatasets, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting XFADatasets to XFA XML: %d fields, %d groups", len(datasets.Fields), len(datasets.Groups))
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start data element
	dataStart := xml.StartElement{Name: xml.Name{Local: "data"}}
	if err := enc.EncodeToken(dataStart); err != nil {
		return "", fmt.Errorf("failed to encode data start: %v", err)
	}

	// Encode top-level fields
	for name, value := range datasets.Fields {
		if err := encodeField(enc, name, value); err != nil {
			return "", fmt.Errorf("failed to encode field %s: %v", name, err)
		}
	}

	// Encode groups
	for _, group := range datasets.Groups {
		if err := encodeFieldGroup(enc, group); err != nil {
			return "", fmt.Errorf("failed to encode group %s: %v", group.Name, err)
		}
	}

	// End data element
	if err := enc.EncodeToken(dataStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode data end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted XFADatasets to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

// encodeField encodes a field as XFA XML
func encodeField(enc *xml.Encoder, name string, value interface{}) error {
	fieldStart := xml.StartElement{
		Name: xml.Name{Local: "field"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: name},
		},
	}

	if err := enc.EncodeToken(fieldStart); err != nil {
		return err
	}

	// Encode value
	valueStart := xml.StartElement{Name: xml.Name{Local: "value"}}
	if err := enc.EncodeToken(valueStart); err != nil {
		return err
	}

	valueStr := formatFieldValue(value)
	if err := enc.EncodeToken(xml.CharData(valueStr)); err != nil {
		return err
	}

	if err := enc.EncodeToken(valueStart.End()); err != nil {
		return err
	}

	if err := enc.EncodeToken(fieldStart.End()); err != nil {
		return err
	}

	return nil
}

// encodeFieldGroup encodes a field group as XFA XML
func encodeFieldGroup(enc *xml.Encoder, group types.XFAFieldGroup) error {
	groupStart := xml.StartElement{Name: xml.Name{Local: group.Name}}
	if err := enc.EncodeToken(groupStart); err != nil {
		return err
	}

	// Encode fields in group
	for name, value := range group.Fields {
		if err := encodeField(enc, name, value); err != nil {
			return err
		}
	}

	// Encode nested groups
	for _, nestedGroup := range group.Groups {
		if err := encodeFieldGroup(enc, nestedGroup); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(groupStart.End()); err != nil {
		return err
	}

	return nil
}

// formatFieldValue formats a value as a string for XML
func formatFieldValue(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// isStandardXFAElement checks if an element name is a standard XFA element
func isStandardXFAElement(name string) bool {
	standardElements := map[string]bool{
		"data":     true,
		"field":    true,
		"value":    true,
		"items":    true,
		"text":     true,
		"label":    true,
		"desc":     true,
		"bind":     true,
		"validate": true,
		"event":    true,
		"script":   true,
	}
	return standardElements[strings.ToLower(name)]
}
