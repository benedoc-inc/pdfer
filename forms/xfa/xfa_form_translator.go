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

// ParseXFAForm parses raw XFA XML and converts it to a strongly-typed FormSchema
func ParseXFAForm(xfaXML string, verbose bool) (*types.FormSchema, error) {
	if verbose {
		log.Printf("Parsing XFA XML to FormSchema (length: %d bytes)", len(xfaXML))
	}

	// Parse XFA XML structure
	xfaData, err := parseXFAStructure(xfaXML, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XFA structure: %v", err)
	}

	// Convert to FormSchema
	formSchema := &types.FormSchema{
		Metadata: types.FormMetadata{
			FormType:   "XFA",
			TotalPages: extractPageCount(xfaXML),
		},
		Questions: make([]types.Question, 0),
		Rules:     make([]types.Rule, 0),
	}

	// Extract metadata
	if xfaData.Title != "" {
		formSchema.Metadata.Title = xfaData.Title
	}
	if xfaData.Description != "" {
		formSchema.Metadata.Description = xfaData.Description
	}
	if xfaData.Version != "" {
		formSchema.Metadata.Version = xfaData.Version
	}

	// Convert fields to questions
	for i, field := range xfaData.Fields {
		question := convertXFAFieldToQuestion(field, i+1, verbose)
		formSchema.Questions = append(formSchema.Questions, question)
	}

	// Extract rules from XFA scripts and events
	rules, err := extractXFARules(xfaXML, xfaData, verbose)
	if err != nil {
		if verbose {
			log.Printf("Warning: Failed to extract XFA rules: %v", err)
		}
		// Continue without rules
	} else {
		formSchema.Rules = rules
	}

	if verbose {
		log.Printf("Parsed XFA to FormSchema: %d questions, %d rules", len(formSchema.Questions), len(formSchema.Rules))
	}

	return formSchema, nil
}

// FormToXFA converts a FormSchema to XFA XML
func FormToXFA(formSchema *types.FormSchema, verbose bool) (string, error) {
	if verbose {
		log.Printf("Converting FormSchema to XFA XML: %d questions, %d rules", len(formSchema.Questions), len(formSchema.Rules))
	}

	var buf strings.Builder
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")

	// Start template element
	templateStart := xml.StartElement{
		Name: xml.Name{Local: "template"},
		Attr: []xml.Attr{},
	}
	if formSchema.Metadata.Title != "" {
		templateStart.Attr = append(templateStart.Attr, xml.Attr{
			Name:  xml.Name{Local: "title"},
			Value: formSchema.Metadata.Title,
		})
	}
	if err := enc.EncodeToken(templateStart); err != nil {
		return "", fmt.Errorf("failed to encode template start: %v", err)
	}

	// Convert questions to fields
	for _, question := range formSchema.Questions {
		if err := encodeQuestionAsField(enc, question); err != nil {
			return "", fmt.Errorf("failed to encode question %s: %v", question.ID, err)
		}
	}

	// Convert rules to events/scripts
	for _, rule := range formSchema.Rules {
		if err := encodeRuleAsEvent(enc, rule); err != nil {
			if verbose {
				log.Printf("Warning: Failed to encode rule %s: %v", rule.ID, err)
			}
		}
	}

	// End template element
	if err := enc.EncodeToken(templateStart.End()); err != nil {
		return "", fmt.Errorf("failed to encode template end: %v", err)
	}

	if err := enc.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush encoder: %v", err)
	}

	result := buf.String()
	if verbose {
		log.Printf("Converted FormSchema to XFA XML: %d bytes", len(result))
	}

	return result, nil
}

// encodeQuestionAsField encodes a Question as an XFA field element
func encodeQuestionAsField(enc *xml.Encoder, question types.Question) error {
	fieldStart := xml.StartElement{
		Name: xml.Name{Local: "field"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: question.Name},
		},
	}

	// Add type attribute
	if question.Type != "" {
		xfaType := mapResponseTypeToXFA(question.Type)
		if xfaType != "" {
			fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
				Name:  xml.Name{Local: "type"},
				Value: xfaType,
			})
		}
	}

	// Add required attribute
	if question.Required {
		fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
			Name:  xml.Name{Local: "required"},
			Value: "1",
		})
	}

	// Add access attribute
	if question.ReadOnly {
		fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
			Name:  xml.Name{Local: "access"},
			Value: "readOnly",
		})
	} else if question.Hidden {
		fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
			Name:  xml.Name{Local: "access"},
			Value: "hidden",
		})
	}

	// Add layout properties
	if question.Properties != nil {
		for key, val := range question.Properties {
			if key == "x" || key == "y" || key == "w" || key == "h" {
				fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
					Name:  xml.Name{Local: key},
					Value: fmt.Sprintf("%v", val),
				})
			}
		}
		if pageNum, ok := question.Properties["page_number"].(int); ok && pageNum > 0 {
			fieldStart.Attr = append(fieldStart.Attr, xml.Attr{
				Name:  xml.Name{Local: "page"},
				Value: strconv.Itoa(pageNum),
			})
		}
	}

	if err := enc.EncodeToken(fieldStart); err != nil {
		return err
	}

	// Add label
	if question.Label != "" {
		labelStart := xml.StartElement{Name: xml.Name{Local: "label"}}
		if err := enc.EncodeToken(labelStart); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(question.Label)); err != nil {
			return err
		}
		if err := enc.EncodeToken(labelStart.End()); err != nil {
			return err
		}
	}

	// Add description
	if question.Description != "" {
		descStart := xml.StartElement{Name: xml.Name{Local: "desc"}}
		if err := enc.EncodeToken(descStart); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(question.Description)); err != nil {
			return err
		}
		if err := enc.EncodeToken(descStart.End()); err != nil {
			return err
		}
	}

	// Add value/default
	if question.Default != nil {
		valueStart := xml.StartElement{Name: xml.Name{Local: "value"}}
		if err := enc.EncodeToken(valueStart); err != nil {
			return err
		}
		valueStr := fmt.Sprintf("%v", question.Default)
		if err := enc.EncodeToken(xml.CharData(valueStr)); err != nil {
			return err
		}
		if err := enc.EncodeToken(valueStart.End()); err != nil {
			return err
		}
	}

	// Add options for choice types
	if len(question.Options) > 0 {
		itemsStart := xml.StartElement{Name: xml.Name{Local: "items"}}
		if err := enc.EncodeToken(itemsStart); err != nil {
			return err
		}
		for _, opt := range question.Options {
			textStart := xml.StartElement{Name: xml.Name{Local: "text"}}
			if err := enc.EncodeToken(textStart); err != nil {
				return err
			}
			if err := enc.EncodeToken(xml.CharData(opt.Value)); err != nil {
				return err
			}
			if err := enc.EncodeToken(textStart.End()); err != nil {
				return err
			}
		}
		if err := enc.EncodeToken(itemsStart.End()); err != nil {
			return err
		}
	}

	// Add validation
	if question.Validation != nil {
		if err := encodeValidation(enc, question.Validation); err != nil {
			return err
		}
	}

	// End field
	if err := enc.EncodeToken(fieldStart.End()); err != nil {
		return err
	}

	return nil
}

// encodeValidation encodes validation rules as XFA validation elements
func encodeValidation(enc *xml.Encoder, validation *types.ValidationRules) error {
	validateStart := xml.StartElement{Name: xml.Name{Local: "validate"}}
	attrs := []xml.Attr{}

	if validation.Pattern != "" {
		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: "formatTest"},
			Value: "pattern",
		})
	}
	if validation.CustomScript != "" {
		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: "scriptTest"},
			Value: validation.CustomScript,
		})
	}
	if validation.ErrorMessage != "" {
		attrs = append(attrs, xml.Attr{
			Name:  xml.Name{Local: "messageText"},
			Value: validation.ErrorMessage,
		})
	}

	validateStart.Attr = attrs
	if err := enc.EncodeToken(validateStart); err != nil {
		return err
	}

	if validation.Pattern != "" {
		patternStart := xml.StartElement{Name: xml.Name{Local: "pattern"}}
		if err := enc.EncodeToken(patternStart); err != nil {
			return err
		}
		if err := enc.EncodeToken(xml.CharData(validation.Pattern)); err != nil {
			return err
		}
		if err := enc.EncodeToken(patternStart.End()); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(validateStart.End()); err != nil {
		return err
	}

	return nil
}

// encodeRuleAsEvent encodes a Rule as an XFA event element
func encodeRuleAsEvent(enc *xml.Encoder, rule types.Rule) error {
	// Map rule type to event activity
	activity := "change" // default
	switch rule.Type {
	case types.RuleTypeVisibility:
		activity = "initialize"
	case types.RuleTypeCalculate:
		activity = "change"
	case types.RuleTypeValidate:
		activity = "validate"
	case types.RuleTypeSetValue:
		activity = "enter"
	}

	eventStart := xml.StartElement{
		Name: xml.Name{Local: "event"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "activity"}, Value: activity},
		},
	}

	if err := enc.EncodeToken(eventStart); err != nil {
		return err
	}

	// Encode actions as script
	if len(rule.Actions) > 0 {
		scriptStart := xml.StartElement{Name: xml.Name{Local: "script"}}
		if err := enc.EncodeToken(scriptStart); err != nil {
			return err
		}

		var scriptBuf strings.Builder
		for _, action := range rule.Actions {
			if action.Script != "" {
				scriptBuf.WriteString(action.Script)
			} else if action.Expression != "" {
				scriptBuf.WriteString(action.Expression)
			}
		}

		if scriptBuf.Len() > 0 {
			if err := enc.EncodeToken(xml.CharData(scriptBuf.String())); err != nil {
				return err
			}
		}

		if err := enc.EncodeToken(scriptStart.End()); err != nil {
			return err
		}
	}

	if err := enc.EncodeToken(eventStart.End()); err != nil {
		return err
	}

	return nil
}

// mapResponseTypeToXFA maps ResponseType enum to XFA field type string
func mapResponseTypeToXFA(responseType types.ResponseType) string {
	switch responseType {
	case types.ResponseTypeText:
		return "text"
	case types.ResponseTypeTextarea:
		return "textEdit"
	case types.ResponseTypeRadio:
		return "radioButton"
	case types.ResponseTypeCheckbox:
		return "checkButton"
	case types.ResponseTypeSelect:
		return "choiceList"
	case types.ResponseTypeNumber:
		return "numeric"
	case types.ResponseTypeDate:
		return "date"
	case types.ResponseTypeEmail:
		return "email"
	case types.ResponseTypeButton:
		return "button"
	case types.ResponseTypeSignature:
		return "signature"
	default:
		return "text"
	}
}

// XFAStructure represents the parsed XFA XML structure
type XFAStructure struct {
	Title       string
	Description string
	Version     string
	Fields      []XFAFieldData
	Subforms    []XFASubformData
}

// XFAFieldData represents a field extracted from XFA XML
type XFAFieldData struct {
	Name        string
	FullName    string
	Type        string
	Value       string
	Default     string
	Label       string
	Description string
	Required    bool
	ReadOnly    bool
	Hidden      bool
	PageNumber  int
	Options     []XFAOption
	Validation  *XFAValidation
	Properties  map[string]interface{}
	Events      []XFAEvent
}

// XFAOption represents an option for choice fields
type XFAOption struct {
	Value    string
	Label    string
	Selected bool
}

// XFAValidation represents validation rules from XFA
type XFAValidation struct {
	MinLength    *int
	MaxLength    *int
	MinValue     *float64
	MaxValue     *float64
	Pattern      string
	Script       string
	ErrorMessage string
}

// XFAEvent represents an event in XFA (for rules extraction)
type XFAEvent struct {
	Type   string // "initialize", "change", "enter", "exit", etc.
	Script string
	Target string
	Action string
}

// XFASubformData represents a subform in XFA
type XFASubformData struct {
	Name   string
	Fields []XFAFieldData
}

// parseXFAStructure parses the XFA XML and extracts the structure
func parseXFAStructure(xfaXML string, verbose bool) (*XFAStructure, error) {
	structure := &XFAStructure{
		Fields:   make([]XFAFieldData, 0),
		Subforms: make([]XFASubformData, 0),
	}

	decoder := xml.NewDecoder(strings.NewReader(xfaXML))
	decoder.Strict = false // Allow malformed XML

	var currentField *XFAFieldData
	var currentSubform *XFASubformData
	var currentValue strings.Builder
	var currentLabel strings.Builder
	var currentDescription strings.Builder
	var inField bool
	var inSubform bool
	var inValue bool
	var inLabel bool
	var inDescription bool
	var inItems bool // For choice options
	var fieldIndex int

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			if verbose {
				log.Printf("XML parse error (continuing): %v", err)
			}
			// Continue on errors - XFA XML can be malformed
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			localName := se.Name.Local

			// Handle top-level elements
			switch localName {
			case "template":
				// Extract title/description from template attributes or children
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "title":
						structure.Title = attr.Value
					}
				}
			case "subform":
				inSubform = true
				currentSubform = &XFASubformData{
					Fields: make([]XFAFieldData, 0),
				}
				for _, attr := range se.Attr {
					if attr.Name.Local == "name" {
						currentSubform.Name = attr.Value
					}
				}
			case "field":
				inField = true
				fieldIndex++
				currentField = &XFAFieldData{
					Properties: make(map[string]interface{}),
					Options:    make([]XFAOption, 0),
					Events:     make([]XFAEvent, 0),
					PageNumber: 1, // Default
				}

				// Extract field attributes
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						currentField.Name = attr.Value
						currentField.FullName = attr.Value
					case "type":
						currentField.Type = attr.Value
					case "required":
						currentField.Required = parseBool(attr.Value)
					case "access":
						if attr.Value == "readOnly" {
							currentField.ReadOnly = true
						} else if attr.Value == "hidden" {
							currentField.Hidden = true
						}
					case "h", "w", "x", "y":
						// Store layout properties
						if val, err := parseFloat(attr.Value); err == nil {
							currentField.Properties[attr.Name.Local] = val
						} else {
							currentField.Properties[attr.Name.Local] = attr.Value
						}
					case "page":
						if pageNum, err := strconv.Atoi(attr.Value); err == nil {
							currentField.PageNumber = pageNum
						}
					}
				}
			case "value":
				if inField {
					inValue = true
					currentValue.Reset()
				}
			case "label":
				if inField {
					inLabel = true
					currentLabel.Reset()
				}
			case "desc", "description":
				if inField {
					inDescription = true
					currentDescription.Reset()
				}
			case "items":
				if inField {
					inItems = true
				}
			case "text":
				// Option text in items
				if inItems && inField {
					currentValue.Reset()
				}
			case "event":
				if inField {
					event := XFAEvent{}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "activity":
							event.Type = attr.Value
						case "name":
							event.Action = attr.Value
						}
					}
					currentField.Events = append(currentField.Events, event)
				}
			case "script":
				if inField && len(currentField.Events) > 0 {
					// Script content will be in CharData
					currentValue.Reset()
				}
			case "validate":
				if inField {
					if currentField.Validation == nil {
						currentField.Validation = &XFAValidation{}
					}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "scriptTest":
							currentField.Validation.Script = attr.Value
						case "messageText":
							currentField.Validation.ErrorMessage = attr.Value
						}
					}
				}
			case "pattern":
				if inField && currentField.Validation != nil {
					currentValue.Reset()
				}
			case "bind":
				if inField {
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "match":
							// Can indicate required, etc.
							if attr.Value == "none" {
								currentField.Required = false
							}
						}
					}
				}
			}

		case xml.EndElement:
			localName := se.Name.Local

			switch localName {
			case "field":
				if currentField != nil {
					// Finalize field
					if currentValue.Len() > 0 {
						currentField.Value = strings.TrimSpace(currentValue.String())
						if currentField.Default == "" {
							currentField.Default = currentField.Value
						}
					}
					if currentLabel.Len() > 0 {
						currentField.Label = strings.TrimSpace(currentLabel.String())
					}
					if currentDescription.Len() > 0 {
						currentField.Description = strings.TrimSpace(currentDescription.String())
					}

					if inSubform && currentSubform != nil {
						currentSubform.Fields = append(currentSubform.Fields, *currentField)
					} else {
						structure.Fields = append(structure.Fields, *currentField)
					}
				}
				inField = false
				currentField = nil
				currentValue.Reset()
				currentLabel.Reset()
				currentDescription.Reset()
			case "subform":
				if currentSubform != nil {
					structure.Subforms = append(structure.Subforms, *currentSubform)
					// Also add subform fields to top-level fields
					structure.Fields = append(structure.Fields, currentSubform.Fields...)
				}
				inSubform = false
				currentSubform = nil
			case "value":
				if inField && currentField != nil {
					val := strings.TrimSpace(currentValue.String())
					if val != "" {
						currentField.Value = val
						if currentField.Default == "" {
							currentField.Default = val
						}
					}
				}
				inValue = false
			case "label":
				if inField && currentField != nil {
					currentField.Label = strings.TrimSpace(currentLabel.String())
				}
				inLabel = false
			case "desc", "description":
				if inField && currentField != nil {
					currentField.Description = strings.TrimSpace(currentDescription.String())
				}
				inDescription = false
			case "items":
				inItems = false
			case "text":
				if inItems && inField && currentField != nil {
					// This is an option value
					option := XFAOption{
						Value: strings.TrimSpace(currentValue.String()),
						Label: strings.TrimSpace(currentValue.String()),
					}
					currentField.Options = append(currentField.Options, option)
				}
			case "script":
				if inField && len(currentField.Events) > 0 {
					// Add script to last event
					lastEvent := &currentField.Events[len(currentField.Events)-1]
					lastEvent.Script = strings.TrimSpace(currentValue.String())
				}
			case "pattern":
				if inField && currentField.Validation != nil {
					currentField.Validation.Pattern = strings.TrimSpace(currentValue.String())
				}
			}

		case xml.CharData:
			data := string(se)
			if inValue && inField {
				currentValue.WriteString(data)
			} else if inLabel && inField {
				currentLabel.WriteString(data)
			} else if inDescription && inField {
				currentDescription.WriteString(data)
			}
		}
	}

	return structure, nil
}

// convertXFAFieldToQuestion converts an XFAFieldData to a Question
func convertXFAFieldToQuestion(field XFAFieldData, index int, verbose bool) types.Question {
	question := types.Question{
		ID:          sanitizeFieldIDWithIndex(field.Name, index),
		Name:        field.Name,
		Label:       field.Label,
		Description: field.Description,
		Type:        mapXFATypeToResponseTypeEnum(field.Type),
		Required:    field.Required,
		ReadOnly:    field.ReadOnly,
		Hidden:      field.Hidden,
		PageNumber:  field.PageNumber,
		Properties:  field.Properties,
	}

	// Set default value
	if field.Default != "" {
		question.Default = field.Default
	} else if field.Value != "" {
		question.Default = field.Value
	}

	// Convert options
	if len(field.Options) > 0 {
		question.Options = make([]types.Option, len(field.Options))
		for i, opt := range field.Options {
			question.Options[i] = types.Option{
				Value:    opt.Value,
				Label:    opt.Label,
				Selected: opt.Selected,
			}
		}
	}

	// Convert validation
	if field.Validation != nil {
		question.Validation = &types.ValidationRules{
			MinLength:    field.Validation.MinLength,
			MaxLength:    field.Validation.MaxLength,
			MinValue:     field.Validation.MinValue,
			MaxValue:     field.Validation.MaxValue,
			Pattern:      field.Validation.Pattern,
			CustomScript: field.Validation.Script,
			ErrorMessage: field.Validation.ErrorMessage,
		}
	}

	return question
}

// mapXFATypeToResponseTypeEnum maps XFA field types to ResponseType enum
func mapXFATypeToResponseTypeEnum(xfaType string) types.ResponseType {
	switch strings.ToLower(xfaType) {
	case "text", "textfield":
		return types.ResponseTypeText
	case "textarea", "textedit":
		return types.ResponseTypeTextarea
	case "radio", "radiobutton":
		return types.ResponseTypeRadio
	case "checkbox", "checkbutton":
		return types.ResponseTypeCheckbox
	case "select", "choice", "dropdown", "listbox":
		return types.ResponseTypeSelect
	case "numeric", "decimal", "integer", "float":
		return types.ResponseTypeNumber
	case "date", "datefield":
		return types.ResponseTypeDate
	case "email", "emailfield":
		return types.ResponseTypeEmail
	case "button":
		return types.ResponseTypeButton
	case "signature", "signaturefield":
		return types.ResponseTypeSignature
	default:
		return types.ResponseTypeText // Default to text
	}
}

// extractXFARules extracts control flow rules from XFA events and scripts
func extractXFARules(xfaXML string, xfaData *XFAStructure, verbose bool) ([]types.Rule, error) {
	rules := make([]types.Rule, 0)
	ruleIndex := 1

	// Extract rules from field events
	for _, field := range xfaData.Fields {
		for _, event := range field.Events {
			rule, err := convertXFAEventToRule(event, field.Name, ruleIndex)
			if err != nil {
				if verbose {
					log.Printf("Warning: Failed to convert event to rule: %v", err)
				}
				continue
			}
			if rule != nil {
				rules = append(rules, *rule)
				ruleIndex++
			}
		}
	}

	return rules, nil
}

// convertXFAEventToRule converts an XFA event to a Rule, parsing the script
// body to extract structured conditions and actions where possible.
func convertXFAEventToRule(event XFAEvent, sourceField string, index int) (*types.Rule, error) {
	rule := &types.Rule{
		ID:     fmt.Sprintf("rule_%d", index),
		Source: sourceField,
		Type:   types.RuleTypeCalculate,
	}

	// Map event activity to rule type.
	switch strings.ToLower(event.Type) {
	case "validate":
		rule.Type = types.RuleTypeValidate
	case "calculate":
		rule.Type = types.RuleTypeCalculate
	case "initialize", "docReady":
		rule.Type = types.RuleTypeSetValue
	case "change", "exit", "enter":
		rule.Type = types.RuleTypeCalculate
	}

	if event.Script == "" {
		return rule, nil
	}

	parsed := parseXFAScript(event.Script, sourceField, event.Target)

	// Use parsed rule type if more specific than the event-derived default.
	if parsed.ruleType != "" {
		rule.Type = parsed.ruleType
	}
	if parsed.description != "" {
		rule.Description = parsed.description
	}
	rule.Condition = parsed.condition
	rule.Actions = parsed.actions

	return rule, nil
}

// scriptParseResult holds the structured result of parsing an XFA script body.
type scriptParseResult struct {
	ruleType    types.RuleType
	condition   *types.Condition
	actions     []types.Action
	description string
}

// parseXFAScript analyses an XFA script (FormCalc or JavaScript) and returns
// whatever structured information can be extracted. It always succeeds — for
// scripts it cannot understand it returns a single ActionTypeExecute action.
func parseXFAScript(script, sourceField, targetField string) scriptParseResult {
	s := strings.TrimSpace(script)
	lang := detectScriptLanguage(s)

	// Try each pattern family in order of specificity.
	// Validation is tried before set-value so that scripts containing both
	// a rawValue reference and return true/false are classified as validation.
	if r, ok := tryParseVisibilityScript(s, lang, sourceField, targetField); ok {
		return r
	}
	if r, ok := tryParseValidationScript(s, lang, sourceField); ok {
		return r
	}
	if r, ok := tryParseSetValueScript(s, lang, sourceField, targetField); ok {
		return r
	}
	if r, ok := tryParseCalculateScript(s, lang, sourceField); ok {
		return r
	}

	// Fallback: wrap raw script in an execute action.
	return scriptParseResult{
		actions: []types.Action{{
			Type:       types.ActionTypeExecute,
			Target:     targetField,
			Script:     script,
			Expression: script,
		}},
	}
}

// detectScriptLanguage returns "formcalc" or "javascript".
// FormCalc uses `then`/`endif` keywords; JavaScript uses `{`/`}` blocks.
func detectScriptLanguage(s string) string {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "then") && (strings.Contains(lower, "endif") || strings.Contains(lower, "end if")) {
		return "formcalc"
	}
	if strings.Contains(s, "{") && strings.Contains(s, "}") {
		return "javascript"
	}
	// FormCalc uses $ for field references and has no braces.
	if strings.Contains(s, "$.") || strings.Contains(s, "$parent.") {
		return "formcalc"
	}
	return "javascript"
}

// tryParseVisibilityScript detects scripts that show or hide a field.
//
// FormCalc patterns:
//   $.presence = "visible" / "hidden" / "invisible"
//   if (cond) then $.presence = "hidden" else $.presence = "visible" endif
//
// JavaScript patterns:
//   this.presence = "visible"
//   xfa.resolveNode("fieldName").presence = "hidden"
func tryParseVisibilityScript(s, lang, sourceField, targetField string) (scriptParseResult, bool) {
	lower := strings.ToLower(s)
	if !strings.Contains(lower, "presence") {
		return scriptParseResult{}, false
	}

	// Determine which visibility action is being set.
	actionType := types.ActionTypeShow
	if strings.Contains(lower, `"hidden"`) || strings.Contains(lower, `"invisible"`) ||
		strings.Contains(lower, "'hidden'") || strings.Contains(lower, "'invisible'") {
		actionType = types.ActionTypeHide
	}

	// Extract target field name.
	target := extractResolveNodeTarget(s)
	if target == "" {
		target = targetField
	}
	if target == "" {
		target = sourceField
	}

	// Check if the assignment is conditional.
	cond := extractSimpleCondition(s, lang, sourceField)

	action := types.Action{
		Type:   actionType,
		Target: target,
	}

	return scriptParseResult{
		ruleType:    types.RuleTypeVisibility,
		condition:   cond,
		actions:     []types.Action{action},
		description: fmt.Sprintf("%s field %q", actionType, target),
	}, true
}

// tryParseSetValueScript detects scripts that assign a value to a field.
//
// FormCalc: $.rawValue = "literal" or $.rawValue = fieldRef.rawValue
// JavaScript: this.rawValue = expr  or  xfa.resolveNode("x").rawValue = expr
func tryParseSetValueScript(s, lang, sourceField, targetField string) (scriptParseResult, bool) {
	lower := strings.ToLower(s)
	if !strings.Contains(lower, "rawvalue") && !strings.Contains(lower, ".value") {
		return scriptParseResult{}, false
	}
	// Must contain an assignment.
	if !strings.Contains(s, "=") {
		return scriptParseResult{}, false
	}
	// Skip comparisons (== / != / <= / >=).
	assignIdx := findAssignmentOp(s)
	if assignIdx == -1 {
		return scriptParseResult{}, false
	}

	rhs := strings.TrimSpace(s[assignIdx+1:])
	// Trim trailing semicolons or statement ends.
	rhs = strings.TrimRight(rhs, "; \t\n\r")

	target := extractResolveNodeTarget(s)
	if target == "" {
		target = targetField
	}
	if target == "" {
		target = sourceField
	}

	cond := extractSimpleCondition(s, lang, sourceField)

	action := types.Action{
		Type:       types.ActionTypeSetValue,
		Target:     target,
		Expression: rhs,
		Value:      unquoteLiteral(rhs),
	}

	return scriptParseResult{
		ruleType:    types.RuleTypeSetValue,
		condition:   cond,
		actions:     []types.Action{action},
		description: fmt.Sprintf("set %q = %s", target, rhs),
	}, true
}

// tryParseValidationScript detects scripts used for validation (return true/false).
func tryParseValidationScript(s, lang, sourceField string) (scriptParseResult, bool) {
	lower := strings.ToLower(s)
	hasReturn := strings.Contains(lower, "return true") || strings.Contains(lower, "return false") ||
		strings.Contains(lower, "return 1") || strings.Contains(lower, "return 0")
	hasMessage := strings.Contains(lower, "xfa.host.messageBox") || strings.Contains(lower, "app.alert") ||
		strings.Contains(lower, "console.print") || strings.Contains(lower, "messagebox")
	if !hasReturn && !hasMessage {
		return scriptParseResult{}, false
	}

	cond := extractSimpleCondition(s, lang, sourceField)

	action := types.Action{
		Type:       types.ActionTypeValidate,
		Target:     sourceField,
		Script:     s,
		Expression: s,
	}

	return scriptParseResult{
		ruleType:    types.RuleTypeValidate,
		condition:   cond,
		actions:     []types.Action{action},
		description: "validate " + sourceField,
	}, true
}

// tryParseCalculateScript detects scripts that compute a value (Sum, arithmetic, concat).
func tryParseCalculateScript(s, lang, sourceField string) (scriptParseResult, bool) {
	lower := strings.ToLower(s)
	// FormCalc built-in functions or obvious arithmetic.
	calcKeywords := []string{"sum(", "count(", "avg(", "min(", "max(", "concat(", "len(", "substr(", "num2str(", "str2num("}
	isCalc := false
	for _, kw := range calcKeywords {
		if strings.Contains(lower, kw) {
			isCalc = true
			break
		}
	}
	// JavaScript: typical calculate pattern involves return of an expression.
	if !isCalc && lang == "javascript" && strings.Contains(lower, "return ") {
		isCalc = true
	}
	if !isCalc {
		return scriptParseResult{}, false
	}

	// Extract the expression: look for last `return <expr>` or the whole script.
	expr := extractReturnExpression(s)
	if expr == "" {
		expr = s
	}

	action := types.Action{
		Type:       types.ActionTypeCalculate,
		Target:     sourceField,
		Expression: expr,
		Script:     s,
	}

	return scriptParseResult{
		ruleType:    types.RuleTypeCalculate,
		actions:     []types.Action{action},
		description: fmt.Sprintf("calculate %s", sourceField),
	}, true
}

// extractSimpleCondition parses an `if` guard from the script and returns a
// *Condition for simple comparisons (field == value, field != value, etc.).
// Returns nil if no parseable condition is present.
func extractSimpleCondition(s, lang, sourceField string) *types.Condition {
	// Extract the condition expression from an if-statement.
	var condExpr string
	if lang == "formcalc" {
		// if (expr) then ... endif  OR  if expr then ... endif
		if i := strings.Index(strings.ToLower(s), "if "); i >= 0 {
			rest := s[i+3:]
			// Find the matching "then"
			thenIdx := strings.Index(strings.ToLower(rest), " then")
			if thenIdx > 0 {
				condExpr = strings.TrimSpace(rest[:thenIdx])
				condExpr = strings.TrimPrefix(condExpr, "(")
				condExpr = strings.TrimSuffix(condExpr, ")")
			}
		}
	} else {
		// JavaScript: if (expr) { ... }
		if i := strings.Index(s, "if ("); i >= 0 {
			rest := s[i+4:]
			depth := 1
			for j := 0; j < len(rest); j++ {
				if rest[j] == '(' {
					depth++
				} else if rest[j] == ')' {
					depth--
					if depth == 0 {
						condExpr = strings.TrimSpace(rest[:j])
						break
					}
				}
			}
		} else if i := strings.Index(s, "if("); i >= 0 {
			rest := s[i+3:]
			depth := 1
			for j := 0; j < len(rest); j++ {
				if rest[j] == '(' {
					depth++
				} else if rest[j] == ')' {
					depth--
					if depth == 0 {
						condExpr = strings.TrimSpace(rest[:j])
						break
					}
				}
			}
		}
	}

	if condExpr == "" {
		return nil
	}

	// Try to parse a simple binary comparison: lhs op rhs
	op, lhs, rhs := parseSimpleComparison(condExpr)
	if op == "" {
		// Return the condition as a raw expression.
		return &types.Condition{Expression: condExpr}
	}

	// Resolve lhs to a field reference where possible.
	fieldRef := extractFieldRef(lhs, sourceField)

	return &types.Condition{
		Operator:   op,
		Expression: condExpr,
		Value:      unquoteLiteral(rhs),
		Children: []types.Condition{{
			Expression: fieldRef,
		}},
	}
}

// parseSimpleComparison splits "lhs op rhs" into parts.
// Returns empty op if the expression is not a simple binary comparison.
func parseSimpleComparison(expr string) (types.Operator, string, string) {
	ops := []struct {
		sym string
		op  types.Operator
	}{
		{"==", types.OperatorEquals},
		{"!=", types.OperatorNotEquals},
		{"<>", types.OperatorNotEquals},
		{">=", types.OperatorGreaterOrEqual},
		{"<=", types.OperatorLessOrEqual},
		{">", types.OperatorGreaterThan},
		{"<", types.OperatorLessThan},
	}
	for _, candidate := range ops {
		idx := strings.Index(expr, candidate.sym)
		if idx < 0 {
			continue
		}
		lhs := strings.TrimSpace(expr[:idx])
		rhs := strings.TrimSpace(expr[idx+len(candidate.sym):])
		if lhs != "" && rhs != "" {
			return candidate.op, lhs, rhs
		}
	}
	return "", "", ""
}

// extractFieldRef converts an XFA field reference token (e.g. "$.rawValue",
// "this.rawValue", "fieldName.rawValue") into a plain field name.
func extractFieldRef(token, currentField string) string {
	t := strings.TrimSpace(token)
	// $.rawValue / $.value → current field
	if strings.HasPrefix(t, "$.") || t == "$" {
		return currentField
	}
	// this.rawValue / this.value → current field
	if strings.HasPrefix(strings.ToLower(t), "this.") || t == "this" {
		return currentField
	}
	// Strip .rawValue / .value suffixes.
	for _, suffix := range []string{".rawValue", ".value", ".rawvalue"} {
		if strings.HasSuffix(strings.ToLower(t), suffix) {
			return t[:len(t)-len(suffix)]
		}
	}
	return t
}

// extractResolveNodeTarget parses `xfa.resolveNode("fieldName")` and returns fieldName.
func extractResolveNodeTarget(s string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "resolvenode(")
	if idx == -1 {
		return ""
	}
	rest := s[idx+12:]
	end := strings.IndexByte(rest, ')')
	if end == -1 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(rest[:end]), `"'`)
}

// extractReturnExpression finds the last `return <expr>` in a script and
// returns the expression part.
func extractReturnExpression(s string) string {
	lower := strings.ToLower(s)
	idx := strings.LastIndex(lower, "return ")
	if idx == -1 {
		return ""
	}
	expr := strings.TrimSpace(s[idx+7:])
	// Trim trailing statement terminators.
	expr = strings.TrimRight(expr, "; \t\n\r")
	// Take only the first line.
	if nl := strings.IndexAny(expr, "\n\r"); nl > 0 {
		expr = strings.TrimSpace(expr[:nl])
	}
	return strings.TrimRight(expr, ";")
}

// findAssignmentOp returns the index of a bare `=` that is not part of
// `==`, `!=`, `<=`, or `>=`. Returns -1 if none found.
func findAssignmentOp(s string) int {
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '=' && s[i-1] != '!' && s[i-1] != '<' && s[i-1] != '>' && s[i-1] != '=' && s[i+1] != '=' {
			return i
		}
	}
	return -1
}

// unquoteLiteral strips surrounding " or ' from a string literal.
// Returns nil if the value is not a string literal.
func unquoteLiteral(s string) interface{} {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return nil
}

// Helper functions

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "1" || s == "true" || s == "yes" || s == "required"
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func sanitizeFieldIDWithIndex(name string, index int) string {
	if name == "" {
		return fmt.Sprintf("field_%d", index)
	}

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
		id = fmt.Sprintf("field_%d", index)
	}
	// Ensure it doesn't start with a number
	if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
		id = "field_" + id
	}
	return id
}

func extractPageCount(xfaXML string) int {
	// Try to find page count in XFA
	// This is a simplified version - could be improved
	pageCount := 1
	if strings.Contains(xfaXML, "<pageSet") {
		// Count pageSet occurrences or extract page count attribute
		pageCount = strings.Count(xfaXML, "<pageSet")
	}
	return pageCount
}
