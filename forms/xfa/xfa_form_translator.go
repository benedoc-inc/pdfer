package xfa

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/benedoc-inc/pdfer/types"
)

// presenceTargetRe matches "Name.presence =" in XFA JavaScript scripts.
var presenceTargetRe = regexp.MustCompile(`\b([A-Za-z_]\w*)\.presence\s*=`)

// xfaFuncDeclRe matches "function Name(...) {" in JavaScript.
var xfaFuncDeclRe = regexp.MustCompile(`function\s+(\w+)\s*\([^)]*\)\s*\{`)

// xfaRawValueCondRe extracts a field name from "FieldName.rawValue ==" patterns.
var xfaRawValueCondRe = regexp.MustCompile(`\b([A-Za-z_]\w*)\.rawValue\s*==`)

// htmlTagRe matches HTML tags for stripping caption exData HTML to plain text.
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// stripHTMLTags removes all HTML tags and normalises whitespace to a single space.
func stripHTMLTags(s string) string {
	plain := htmlTagRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(plain), " ")
}

// ParseXFAForm parses raw XFA XML and converts it to a strongly-typed FormSchema.
func ParseXFAForm(xfaXML string, verbose bool) (*types.FormSchema, error) {
	return ParseXFAFormWithResources(xfaXML, nil, verbose)
}

// ParseXFAFormWithResources parses XFA XML and resolves any href-based image
// references against the provided resource map. Keys in resources are the bare
// resource name (e.g. "logo.jpeg"); values are the raw image bytes.
// Pass nil when no external resources are available.
func ParseXFAFormWithResources(xfaXML string, resources map[string][]byte, verbose bool) (*types.FormSchema, error) {
	if verbose {
		log.Printf("Parsing XFA XML to FormSchema (length: %d bytes, resources: %d)", len(xfaXML), len(resources))
	}
	result, err := parseXFATemplate(xfaXML, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XFA structure: %v", err)
	}
	schema := buildFormSchema(result, verbose)
	schema.Metadata.TotalPages = extractPageCount(xfaXML)
	rules, err := extractXFARules(result.Root, verbose)
	if err != nil && verbose {
		log.Printf("Warning: Failed to extract XFA rules: %v", err)
	}
	// Append field-event rules to any rules already built by buildFormSchema
	// (subform events, variables function bodies).
	schema.Rules = append(schema.Rules, rules...)
	if len(resources) > 0 {
		resolveImageHRefs(schema, resources)
	}
	if verbose {
		log.Printf("Parsed XFA to FormSchema: %d questions, %d sections, %d rules",
			len(schema.Questions), len(schema.Sections), len(schema.Rules))
	}
	return schema, nil
}

// resolveImageHRefs fills in image_data for questions that carry an image_href
// pointing to a $rr: resource. The href is stripped of its $rr: prefix and
// matched against the resource map keys case-insensitively.
func resolveImageHRefs(schema *types.FormSchema, resources map[string][]byte) {
	// Build a lowercase lookup to handle case mismatches.
	lower := make(map[string][]byte, len(resources))
	for k, v := range resources {
		lower[strings.ToLower(k)] = v
	}
	for i := range schema.Questions {
		q := &schema.Questions[i]
		if q.Type != types.ResponseTypeImage || q.Properties == nil {
			continue
		}
		href, _ := q.Properties["image_href"].(string)
		if href == "" {
			continue
		}
		// Strip $rr: or # prefix.
		name := href
		if strings.HasPrefix(name, "$rr:") {
			name = name[4:]
		} else if strings.HasPrefix(name, "#") {
			name = name[1:]
		}
		data, ok := lower[strings.ToLower(name)]
		if !ok {
			continue
		}
		q.Properties["image_data"] = base64.StdEncoding.EncodeToString(data)
		// Infer content type from extension if not already set.
		if ct, _ := q.Properties["content_type"].(string); ct == "image/jpeg" {
			ext := strings.ToLower(name)
			switch {
			case strings.HasSuffix(ext, ".png"):
				q.Properties["content_type"] = "image/png"
			case strings.HasSuffix(ext, ".gif"):
				q.Properties["content_type"] = "image/gif"
			case strings.HasSuffix(ext, ".bmp"):
				q.Properties["content_type"] = "image/bmp"
			case strings.HasSuffix(ext, ".webp"):
				q.Properties["content_type"] = "image/webp"
			}
		}
	}
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
	case types.ResponseTypeDate, types.ResponseTypeTime:
		return "dateTimeEdit"
	case types.ResponseTypeEmail:
		return "email"
	case types.ResponseTypePassword:
		return "passwordEdit"
	case types.ResponseTypeButton:
		return "button"
	case types.ResponseTypeSignature:
		return "signature"
	default:
		return "text"
	}
}

// ── Internal parse-tree types ─────────────────────────────────────────────────

// xfaNodeKind identifies the XFA element type of a parse-tree node.
type xfaNodeKind string

const (
	xfaKindSubform   xfaNodeKind = "subform"
	xfaKindPageArea  xfaNodeKind = "pageArea"
	xfaKindField     xfaNodeKind = "field"
	xfaKindDraw      xfaNodeKind = "draw"
	xfaKindExclGroup xfaNodeKind = "exclGroup"
)

// xfaNode is an internal parse-tree node for one XFA template element.
// Container nodes (subform, pageArea, exclGroup) carry child nodes in document order.
// Leaf nodes (field, draw) carry all parsed attributes.
type xfaNode struct {
	Kind     xfaNodeKind
	Name     string
	Children []*xfaNode // document order; populated for containers

	// Label sources, resolved by resolveInteractiveLabel / resolveDrawText.
	Caption         string // from <caption><value><text> or <label>
	CaptionPlacement string // from <caption placement="left|right|top|bottom|inline">
	ToolTip         string // from <assist><toolTip> — accessibility annotation (e.g. IMDRF TOC refs)
	SpeakLabel      string // from <assist><speak>
	BookmarkName    string // from <extras name="bookmark"><text name="name"> — PDF bookmark label

	// Content
	Value       string
	Default     string
	Description string
	UIType      string // from <ui> child: textEdit | checkButton | radioButton | choiceList | …
	Bind        string // "none" when <bind match="none">; "" means auto-bind

	// Position / dimensions (present on field, draw, subform, exclGroup)
	X, Y, W, H, MinH string

	// Layout (subform and exclGroup layout mode: position|tb|lr-tb|row|table)
	Layout string

	// Field interaction
	Required     bool
	ReadOnly     bool
	Hidden       bool
	Options      []XFAOption
	OptionValues []string // data values from <items save="1">
	Validation   *XFAValidation
	Events       []XFAEvent

	// UI-element-specific constraints
	AllowNeutral         bool   // checkButton allowNeutral="1" → tri-state checkbox
	MaxChars             *int   // textEdit maxChars → ValidationRules.MaxLength
	FracDigits           *int   // numericEdit fracDigits (decimal places)
	LeadDigits           *int   // numericEdit leadDigits (max integer digits)
	ChoiceListOpen       string // choiceList open: "always"=listbox, "userInteraction"=dropdown
	ChoiceListMultiSelect bool  // choiceList multiSelect="1"
	DateTimeSubType      string // "time" when picture starts with TIME{; "datetime" for combined

	// Visual / display hints
	IsLine     bool   // draw <value><line> → separator element
	TextAlign  string // from <para hAlign="left|right|center|justify">
	FontSize   string // from <font size="9pt">
	FontWeight string // from <font weight="bold|normal">

	// Draw classification flags — set at parse time, used in emitDraw.
	ImageData        string
	ImageContentType string
	ImageHRef        string // href attribute value (e.g. "$rr:logo.jpeg"); empty for inline images
	HasPresenceAttr  bool   // explicit presence= attr → script-managed status indicator, suppress
	HasExData        bool   // contains <exData>
	ExDataHTML       string // plain text extracted from text/html exData without xfa:embed markers

	PageNumber int
}

// xfaTemplateResult bundles the parse tree with top-level metadata.
type xfaTemplateResult struct {
	Root               *xfaNode
	Title              string
	Description        string
	Version            string
	VariablesFunctions []variablesFuncDef
}

// variablesFuncDef holds the body of a JavaScript function extracted from a
// <variables> script block. These function bodies contain presence-based
// visibility logic that cannot be attributed to individual field events.
type variablesFuncDef struct {
	body string
	lang string
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
	Lang   string // "formcalc" | "javascript" | "" (auto-detect)
	Target string
	Action string
}

// ── Label resolution helpers ──────────────────────────────────────────────────

// resolveInteractiveLabel returns the best label for an interactive field or exclGroup.
// Priority: non-placeholder caption > toolTip > speak.
// Placeholder captions (e.g. "The caption is in the textbox to the left…") are
// treated as absent so the toolTip can surface.
func resolveInteractiveLabel(n *xfaNode) string {
	caption := n.Caption
	if isPlaceholderCaption(caption) {
		caption = ""
	}
	return firstNonEmpty(caption, n.ToolTip, n.SpeakLabel)
}

// resolveDisplayLabel extends resolveInteractiveLabel with a Name fallback so no
// question is emitted with a blank label. NOT used inside claimDrawLabels — that
// check must remain Name-free so fields with no caption/tooltip still trigger the
// sibling-draw scan.
func resolveDisplayLabel(n *xfaNode) string {
	return firstNonEmpty(resolveInteractiveLabel(n), n.Name)
}

// resolveDrawText returns the best display text for a draw/display element.
// Priority: caption > rich HTML > default/value text > toolTip > speak.
// ToolTip is placed after visible content because XFA authors use it for
// accessibility annotations (e.g. IMDRF TOC references) that are not
// intended as display labels.
func resolveDrawText(n *xfaNode) string {
	return firstNonEmpty(n.Caption, n.ExDataHTML, n.Default, n.Value, n.ToolTip, n.SpeakLabel)
}

// isInteractiveSubtree reports whether node or any descendant produces a user-facing
// interactive element. This covers:
//   - Data-bound fields (bind != "none")
//   - exclGroups (always interactive)
//   - AddAttachment buttons (bind="none", UIType="button") — these are emitted as
//     file questions by emitField and must be counted as interactive so that
//     sections containing only file attachment inputs appear in the navigation.
func isInteractiveSubtree(n *xfaNode) bool {
	switch n.Kind {
	case xfaKindField:
		if n.Bind != "none" {
			return true
		}
		return n.UIType == "button" && strings.Contains(n.Name, "AddAttachment")
	case xfaKindExclGroup:
		return true
	}
	for _, child := range n.Children {
		if isInteractiveSubtree(child) {
			return true
		}
	}
	return false
}

// ── XML parser ────────────────────────────────────────────────────────────────

// parseXFATemplate parses XFA template XML and builds a typed xfaNode tree.
func parseXFATemplate(xfaXML string, verbose bool) (*xfaTemplateResult, error) {
	res := &xfaTemplateResult{}

	// Synthetic root holds all top-level subforms.
	root := &xfaNode{Kind: xfaKindSubform, Name: "_root"}
	nodeStack := []*xfaNode{root}
	topOfStack := func() *xfaNode { return nodeStack[len(nodeStack)-1] }

	decoder := xml.NewDecoder(strings.NewReader(xfaXML))
	decoder.Strict = false

	var currentLeaf        *xfaNode
	var currentValue       strings.Builder
	var currentCaption     strings.Builder
	var currentLabel       strings.Builder
	var currentDesc        strings.Builder
	var currentToolTip     strings.Builder
	var currentSpeak       strings.Builder
	var currentImageData   strings.Builder
	var imageContentType   string

	// exData HTML extraction state
	var exDataContentType string
	var exDataHTMLBuf     strings.Builder
	var exDataHasEmbed    bool

	// picture element state (for dateTimeEdit subtype detection)
	var inPicture     bool
	var currentPicture strings.Builder

	var inValue           bool
	var inCaption         bool
	var inExclGroupCaption bool // reading the exclGroup's own <caption>, not a child field's
	var inSubformCaption  bool // reading the <caption> that is a direct child of a subform
	var inLabel           bool
	var inDescription     bool
	var inItems           bool
	var itemsIsSave       bool
	var inImage           bool
	var inToolTip         bool
	var inAssist          bool
	var inSpeak           bool
	var inExData          bool
	var inCaptionExData   bool // exData nested inside a <caption> — routes to currentCaption
	var inScript          bool
	var inVariables       bool   // inside a <variables> element
	var inVariablesScript bool   // inside a <variables><script> element
	var variablesScriptLang string

	// bookmark extras state
	var inBookmarkExtras bool
	var inBookmarkName   bool
	var currentBookmarkName strings.Builder

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
			// When inside exData HTML content, only detect xfa:embed markers and skip
			// all XFA element processing to avoid false-positive state transitions.
			if inExData {
				for _, attr := range se.Attr {
					if attr.Name.Local == "embed" {
						exDataHasEmbed = true
					}
				}
				break
			}
			localName := se.Name.Local
			switch localName {

			case "template":
				for _, attr := range se.Attr {
					if attr.Name.Local == "title" {
						res.Title = attr.Value
					}
				}

			case "variables":
				inVariables = true

			case "subform", "pageArea":
				kind := xfaKindSubform
				if localName == "pageArea" {
					kind = xfaKindPageArea
				}
				node := &xfaNode{Kind: kind, Events: make([]XFAEvent, 0)}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						node.Name = attr.Value
					case "id":
						if node.Name == "" {
							node.Name = attr.Value
						}
					case "presence":
						if attr.Value != "visible" {
							node.Hidden = true
						}
					case "access":
						if attr.Value == "readOnly" || attr.Value == "protected" || attr.Value == "nonInteractive" {
							node.ReadOnly = true
						}
					case "layout":
						node.Layout = attr.Value
					case "x":
						node.X = attr.Value
					case "y":
						node.Y = attr.Value
					case "w":
						node.W = attr.Value
					case "h":
						node.H = attr.Value
					case "minH":
						node.MinH = attr.Value
					}
				}
				nodeStack = append(nodeStack, node)

			case "exclGroup":
				node := &xfaNode{
					Kind:    xfaKindExclGroup,
					Options: make([]XFAOption, 0),
					Events:  make([]XFAEvent, 0),
				}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						node.Name = attr.Value
					case "layout":
						node.Layout = attr.Value
					case "x":
						node.X = attr.Value
					case "y":
						node.Y = attr.Value
					case "w":
						node.W = attr.Value
					case "h":
						node.H = attr.Value
					}
				}
				nodeStack = append(nodeStack, node)

			case "field", "draw":
				kind := xfaKindField
				if localName == "draw" {
					kind = xfaKindDraw
				}
				leaf := &xfaNode{
					Kind:         kind,
					PageNumber:   1,
					Options:      make([]XFAOption, 0),
					OptionValues: make([]string, 0),
					Events:       make([]XFAEvent, 0),
				}
				if kind == xfaKindDraw {
					leaf.Bind = "none"
				}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "name":
						leaf.Name = attr.Value
					case "required":
						leaf.Required = parseBool(attr.Value)
					case "mandatory":
						if attr.Value == "error" || attr.Value == "warning" || parseBool(attr.Value) {
							leaf.Required = true
						}
					case "presence":
						leaf.Hidden = attr.Value != "visible"
						if kind == xfaKindDraw {
							leaf.HasPresenceAttr = true
						}
					case "access":
						if attr.Value == "readOnly" || attr.Value == "protected" || attr.Value == "nonInteractive" {
							leaf.ReadOnly = true
						}
					case "page":
						if p, e := strconv.Atoi(attr.Value); e == nil {
							leaf.PageNumber = p
						}
					case "x":
						leaf.X = attr.Value
					case "y":
						leaf.Y = attr.Value
					case "w":
						leaf.W = attr.Value
					case "h":
						leaf.H = attr.Value
					case "minH":
						leaf.MinH = attr.Value
					}
				}
				currentLeaf = leaf

			case "caption":
				if currentLeaf != nil {
					inCaption = true
					currentCaption.Reset()
					for _, attr := range se.Attr {
						if attr.Name.Local == "placement" {
							currentLeaf.CaptionPlacement = attr.Value
						}
					}
				} else if topOfStack().Kind == xfaKindExclGroup {
					inExclGroupCaption = true
					currentCaption.Reset()
				} else if topOfStack().Kind == xfaKindSubform {
					inSubformCaption = true
					currentCaption.Reset()
				}

			case "value":
				// Suppress inValue inside caption (caption has its own accumulator).
				if currentLeaf != nil && !inCaption && !inExclGroupCaption && !inSubformCaption {
					inValue = true
					currentValue.Reset()
				}

			case "label":
				if currentLeaf != nil {
					inLabel = true
					currentLabel.Reset()
				}

			case "desc", "description":
				if currentLeaf != nil {
					inDescription = true
					currentDesc.Reset()
				}

			case "items":
				if currentLeaf != nil {
					inItems = true
					itemsIsSave = false
					for _, attr := range se.Attr {
						if attr.Name.Local == "save" && attr.Value == "1" {
							itemsIsSave = true
						}
					}
				}

			case "text":
				if inItems && currentLeaf != nil {
					currentValue.Reset()
				} else if inBookmarkExtras && currentLeaf == nil {
					// Detect <text name="name"> inside a bookmark extras block.
					for _, attr := range se.Attr {
						if attr.Name.Local == "name" && attr.Value == "name" {
							inBookmarkName = true
							currentBookmarkName.Reset()
							break
						}
					}
				}

			// UI element type detection
			case "textEdit":
				if currentLeaf != nil {
					isML := false
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "multiLine":
							if attr.Value == "1" {
								isML = true
							}
						case "maxChars":
							if v, e := strconv.Atoi(attr.Value); e == nil && v > 0 {
								currentLeaf.MaxChars = &v
							}
						}
					}
					if isML {
						currentLeaf.UIType = "textEditMultiLine"
					} else if currentLeaf.UIType == "" {
						currentLeaf.UIType = "textEdit"
					}
				}
			case "checkButton":
				if currentLeaf != nil {
					currentLeaf.UIType = "checkButton"
					for _, attr := range se.Attr {
						if attr.Name.Local == "allowNeutral" && (attr.Value == "1" || attr.Value == "true") {
							currentLeaf.AllowNeutral = true
						}
					}
				}
			case "choiceList":
				if currentLeaf != nil {
					currentLeaf.UIType = "choiceList"
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "open":
							currentLeaf.ChoiceListOpen = attr.Value
						case "multiSelect":
							if attr.Value == "1" || attr.Value == "true" {
								currentLeaf.ChoiceListMultiSelect = true
							}
						}
					}
				}
			case "radioButton":
				if currentLeaf != nil {
					currentLeaf.UIType = "radioButton"
				}
			case "dateTimeEdit":
				if currentLeaf != nil {
					currentLeaf.UIType = "dateTimeEdit"
				}
			case "numericEdit":
				if currentLeaf != nil {
					currentLeaf.UIType = "numericEdit"
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "fracDigits":
							if v, e := strconv.Atoi(attr.Value); e == nil {
								currentLeaf.FracDigits = &v
							}
						case "leadDigits":
							if v, e := strconv.Atoi(attr.Value); e == nil {
								currentLeaf.LeadDigits = &v
							}
						}
					}
				}
			case "passwordEdit":
				if currentLeaf != nil {
					currentLeaf.UIType = "passwordEdit"
				}
			case "signature":
				if currentLeaf != nil {
					currentLeaf.UIType = "signature"
				}
			case "imageEdit":
				if currentLeaf != nil {
					currentLeaf.UIType = "imageEdit"
				}
			case "button":
				if currentLeaf != nil && currentLeaf.UIType == "" {
					currentLeaf.UIType = "button"
				}

			// Visual / layout hints
			case "para":
				if currentLeaf != nil {
					for _, attr := range se.Attr {
						if attr.Name.Local == "hAlign" {
							currentLeaf.TextAlign = attr.Value
						}
					}
				}

			case "font":
				if currentLeaf != nil {
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "size":
							currentLeaf.FontSize = attr.Value
						case "weight":
							currentLeaf.FontWeight = attr.Value
						}
					}
				}

			case "line":
				// <draw><value><line> → separator element
				if currentLeaf != nil && currentLeaf.Kind == xfaKindDraw && inValue {
					currentLeaf.IsLine = true
				}

			case "picture":
				// Inside <format><picture> — used to detect dateTimeEdit subtype.
				if currentLeaf != nil && currentLeaf.UIType == "dateTimeEdit" {
					inPicture = true
					currentPicture.Reset()
				}

			case "event":
				if currentLeaf != nil {
					ev := XFAEvent{}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "activity":
							ev.Type = attr.Value
						case "name":
							ev.Action = attr.Value
						}
					}
					currentLeaf.Events = append(currentLeaf.Events, ev)
				} else if top := topOfStack(); top.Kind == xfaKindSubform {
					ev := XFAEvent{}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "activity":
							ev.Type = attr.Value
						case "name":
							ev.Action = attr.Value
						}
					}
					top.Events = append(top.Events, ev)
				}

			case "script":
				if inVariables {
					inVariablesScript = true
					variablesScriptLang = "javascript"
					currentValue.Reset()
					for _, attr := range se.Attr {
						if attr.Name.Local == "contentType" {
							if l := contentTypeToLang(attr.Value); l != "" {
								variablesScriptLang = l
							}
						}
					}
				} else if currentLeaf != nil && len(currentLeaf.Events) > 0 {
					inScript = true
					currentValue.Reset()
					for _, attr := range se.Attr {
						if attr.Name.Local == "contentType" {
							last := &currentLeaf.Events[len(currentLeaf.Events)-1]
							last.Lang = contentTypeToLang(attr.Value)
						}
					}
				} else if top := topOfStack(); top.Kind == xfaKindSubform && len(top.Events) > 0 {
					inScript = true
					currentValue.Reset()
					for _, attr := range se.Attr {
						if attr.Name.Local == "contentType" {
							last := &top.Events[len(top.Events)-1]
							last.Lang = contentTypeToLang(attr.Value)
						}
					}
				}

			case "validate":
				if currentLeaf != nil {
					if currentLeaf.Validation == nil {
						currentLeaf.Validation = &XFAValidation{}
					}
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "scriptTest":
							currentLeaf.Validation.Script = attr.Value
						case "messageText":
							currentLeaf.Validation.ErrorMessage = attr.Value
						}
					}
				}

			case "pattern":
				if currentLeaf != nil && currentLeaf.Validation != nil {
					currentValue.Reset()
				}

			case "image":
				if currentLeaf != nil {
					inImage = true
					currentImageData.Reset()
					imageContentType = "image/jpeg"
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "contentType":
							imageContentType = attr.Value
						case "href":
							currentLeaf.ImageHRef = attr.Value
						}
					}
				}

			case "toolTip":
				if currentLeaf != nil {
					inToolTip = true
					currentToolTip.Reset()
				}
			case "assist":
				if currentLeaf != nil {
					inAssist = true
				}
			case "speak":
				if currentLeaf != nil && inAssist {
					inSpeak = true
					currentSpeak.Reset()
				}

			case "exData":
				if currentLeaf != nil {
					inExData = true
					currentLeaf.HasExData = true
					exDataContentType = ""
					exDataHTMLBuf.Reset()
					exDataHasEmbed = false
					inCaptionExData = inCaption || inExclGroupCaption || inSubformCaption
					for _, attr := range se.Attr {
						if attr.Name.Local == "contentType" {
							exDataContentType = attr.Value
						}
					}
				}

			case "bind":
				if currentLeaf != nil {
					for _, attr := range se.Attr {
						if attr.Name.Local == "match" && attr.Value == "none" {
							currentLeaf.Bind = "none"
						}
					}
				}

			case "extras":
				// Track <extras name="bookmark"> on subforms for bookmark label extraction.
				if currentLeaf == nil {
					for _, attr := range se.Attr {
						if attr.Name.Local == "name" && attr.Value == "bookmark" {
							inBookmarkExtras = true
							break
						}
					}
				}
			}

		case xml.EndElement:
			// When inside exData HTML content, only process the closing </exData> tag.
			if inExData && se.Name.Local != "exData" {
				break
			}
			localName := se.Name.Local
			switch localName {

			case "caption":
				text := strings.TrimSpace(currentCaption.String())
				if inExclGroupCaption {
					if topOfStack().Kind == xfaKindExclGroup {
						topOfStack().Caption = text
					}
				} else if inSubformCaption {
					if topOfStack().Kind == xfaKindSubform && text != "" && topOfStack().Caption == "" {
						topOfStack().Caption = text
					}
				} else if currentLeaf != nil && text != "" && currentLeaf.Caption == "" {
					currentLeaf.Caption = text
				}
				inCaption = false
				inExclGroupCaption = false
				inSubformCaption = false

			case "label":
				if currentLeaf != nil {
					text := strings.TrimSpace(currentLabel.String())
					if text != "" && currentLeaf.Caption == "" {
						currentLeaf.Caption = text // treat <label> as caption
					}
				}
				inLabel = false

			case "field", "draw":
				if currentLeaf != nil {
					if inDescription && currentDesc.Len() > 0 {
						currentLeaf.Description = strings.TrimSpace(currentDesc.String())
					}
					top := topOfStack()
					if top.Kind == xfaKindExclGroup && localName == "field" {
						// Radio option inside exclGroup → accumulate as XFAOption.
						optLabel := firstNonEmpty(currentLeaf.Caption, currentLeaf.ToolTip, currentLeaf.Name)
						optValue := ""
						if len(currentLeaf.Options) > 0 {
							optValue = currentLeaf.Options[0].Value
						}
						if optValue == "" {
							optValue = currentLeaf.Name
						}
						top.Options = append(top.Options, XFAOption{Label: optLabel, Value: optValue})
						// draws inside exclGroup are decorative labels — fall through and discard
					} else if top.Kind != xfaKindExclGroup {
						top.Children = append(top.Children, currentLeaf)
					}
				}
				currentLeaf = nil
				inCaption = false
				inExclGroupCaption = false
				inLabel = false
				inDescription = false
				inImage = false
				inToolTip = false
				inAssist = false
				inSpeak = false
				inExData = false
				inValue = false
				inItems = false
				inScript = false
				currentValue.Reset()
				currentCaption.Reset()
				currentLabel.Reset()
				currentDesc.Reset()
				currentToolTip.Reset()
				currentSpeak.Reset()
				currentImageData.Reset()

			case "exclGroup":
				if len(nodeStack) > 1 {
					node := nodeStack[len(nodeStack)-1]
					nodeStack = nodeStack[:len(nodeStack)-1]
					topOfStack().Children = append(topOfStack().Children, node)
				}

			case "subform", "pageArea":
				if len(nodeStack) > 1 {
					node := nodeStack[len(nodeStack)-1]
					nodeStack = nodeStack[:len(nodeStack)-1]
					topOfStack().Children = append(topOfStack().Children, node)
				}

			case "value":
				if currentLeaf != nil {
					val := strings.TrimSpace(currentValue.String())
					if val != "" {
						currentLeaf.Value = val
						if currentLeaf.Default == "" {
							currentLeaf.Default = val
						}
					}
				}
				inValue = false

			case "image":
				if currentLeaf != nil {
					if currentImageData.Len() > 0 {
						currentLeaf.ImageData = strings.TrimSpace(currentImageData.String())
					}
					// Always store content type — relevant for both inline and href images.
					if currentLeaf.ImageHRef != "" || currentImageData.Len() > 0 {
						currentLeaf.ImageContentType = imageContentType
					}
				}
				inImage = false
				currentImageData.Reset()

			case "items":
				if itemsIsSave && currentLeaf != nil {
					for i, sv := range currentLeaf.OptionValues {
						if i < len(currentLeaf.Options) {
							currentLeaf.Options[i].Value = sv
						}
					}
				}
				inItems = false
				itemsIsSave = false

			case "text":
				if inBookmarkName {
					if text := strings.TrimSpace(currentBookmarkName.String()); text != "" {
						topOfStack().BookmarkName = text
					}
					currentBookmarkName.Reset()
					inBookmarkName = false
				} else if inItems && currentLeaf != nil {
					text := strings.TrimSpace(currentValue.String())
					currentValue.Reset()
					if itemsIsSave {
						currentLeaf.OptionValues = append(currentLeaf.OptionValues, text)
					} else {
						currentLeaf.Options = append(currentLeaf.Options, XFAOption{Value: text, Label: text})
					}
				}

			case "extras":
				inBookmarkExtras = false

			case "script":
				if inVariablesScript {
					body := strings.TrimSpace(currentValue.String())
					if body != "" {
						res.VariablesFunctions = append(res.VariablesFunctions,
							extractJSFunctionBodies(body, variablesScriptLang)...)
					}
					inVariablesScript = false
				} else if currentLeaf != nil && len(currentLeaf.Events) > 0 {
					last := &currentLeaf.Events[len(currentLeaf.Events)-1]
					last.Script = strings.TrimSpace(currentValue.String())
				} else if top := topOfStack(); top.Kind == xfaKindSubform && len(top.Events) > 0 {
					last := &top.Events[len(top.Events)-1]
					last.Script = strings.TrimSpace(currentValue.String())
				}
				inScript = false
				currentValue.Reset()

			case "variables":
				inVariables = false

			case "pattern":
				if currentLeaf != nil && currentLeaf.Validation != nil {
					currentLeaf.Validation.Pattern = strings.TrimSpace(currentValue.String())
				}

			case "toolTip":
				if currentLeaf != nil {
					currentLeaf.ToolTip = strings.TrimSpace(currentToolTip.String())
				}
				inToolTip = false
				currentToolTip.Reset()

			case "speak":
				if currentLeaf != nil && inAssist {
					currentLeaf.SpeakLabel = strings.TrimSpace(currentSpeak.String())
				}
				inSpeak = false
				currentSpeak.Reset()

			case "assist":
				inAssist = false

			case "exData":
				// Extract plain text from text/html exData that has no xfa:embed page-counter markers.
				if currentLeaf != nil && exDataContentType == "text/html" && !exDataHasEmbed {
					text := strings.TrimSpace(exDataHTMLBuf.String())
					if text != "" {
						if inCaptionExData {
							// Caption exData: route stripped plain text into currentCaption so
							// resolveInteractiveLabel can pick it up as Caption.
							currentCaption.WriteString(stripHTMLTags(text))
						} else {
							currentLeaf.ExDataHTML = text
						}
					}
				}
				inExData = false
				inCaptionExData = false
				exDataContentType = ""
				exDataHasEmbed = false
				exDataHTMLBuf.Reset()

			case "picture":
				// Detect dateTimeEdit subtype from picture pattern.
				if currentLeaf != nil && inPicture {
					pic := strings.ToUpper(strings.TrimSpace(currentPicture.String()))
					switch {
					case strings.HasPrefix(pic, "TIME{"):
						currentLeaf.DateTimeSubType = "time"
					case strings.HasPrefix(pic, "DATETIME{"), strings.Contains(pic, "}{TIME{"):
						currentLeaf.DateTimeSubType = "datetime"
					}
				}
				inPicture = false
				currentPicture.Reset()

			case "desc", "description":
				if currentLeaf != nil {
					currentLeaf.Description = strings.TrimSpace(currentDesc.String())
				}
				inDescription = false
			}

		case xml.CharData:
			data := string(se)
			switch {
			case inImage:
				currentImageData.WriteString(data)
			case inExData:
				// Accumulate text/html content; page-counter machinery is filtered at </exData>.
				if exDataContentType == "text/html" {
					exDataHTMLBuf.WriteString(data)
				}
			case inPicture:
				currentPicture.WriteString(data)
			case inCaption || inExclGroupCaption || inSubformCaption:
				currentCaption.WriteString(data)
			case inLabel:
				currentLabel.WriteString(data)
			case inToolTip:
				currentToolTip.WriteString(data)
			case inSpeak && inAssist:
				currentSpeak.WriteString(data)
			case inBookmarkName:
				currentBookmarkName.WriteString(data)
			case inScript || inVariablesScript:
				currentValue.WriteString(data)
			case inItems:
				currentValue.WriteString(data)
			case inValue:
				currentValue.WriteString(data)
			case inDescription:
				currentDesc.WriteString(data)
			}
		}
	}

	res.Root = root
	return res, nil
}

// ── Schema builder ────────────────────────────────────────────────────────────

// buildFormSchema walks the xfaNode tree and builds a FormSchema with a flat
// questions list and a hierarchical sections tree.
func buildFormSchema(result *xfaTemplateResult, verbose bool) *types.FormSchema {
	schema := &types.FormSchema{
		Metadata: types.FormMetadata{
			FormType:    "XFA",
			Title:       result.Title,
			Description: result.Description,
			Version:     result.Version,
		},
		Questions: make([]types.Question, 0),
		Rules:     make([]types.Rule, 0),
	}

	// Pre-pass: claim preceding draw nodes as labels for unlabeled exclGroups.
	claimDrawLabels(result.Root)

	var qIdx int
	schema.Sections = walkSubformChildren(result.Root, nil, schema, &qIdx, false, verbose)

	// Extract visibility rules from <variables> function bodies. These functions
	// contain presence-based logic that XFA field events delegate to rather than
	// implementing inline, so they can't be attributed to a single field event.
	for _, fn := range result.VariablesFunctions {
		newRules := parseVariablesFunctionRules(fn.body, fn.lang, len(schema.Rules))
		schema.Rules = append(schema.Rules, newRules...)
	}

	return schema
}

// subformHasInteractive reports whether node contains at least one field or
// exclGroup anywhere in its subtree (direct or nested).
func subformHasInteractive(node *xfaNode) bool {
	for _, c := range node.Children {
		if c.Kind == xfaKindField || c.Kind == xfaKindExclGroup {
			return true
		}
		if c.Kind == xfaKindSubform && subformHasInteractive(c) {
			return true
		}
	}
	return false
}

// claimDrawLabels performs a depth-first pre-pass over the tree. For each
// unlabeled exclGroup or data-bound field, it scans backwards through its
// sibling list for the nearest eligible draw and claims its text as the
// node's caption, marking the draw consumed so it is not double-emitted.
//
// Scan rules:
//   - Stops at subform, pageArea, field, or exclGroup boundaries — a label
//     draw is always in the same visual cluster as its field.
//   - Skips imageEdit draws (colored indicator blocks, not text labels).
//   - Skips event-bearing draws (dynamic).
//   - Presence-attributed text draws ARE eligible — they are valid static
//     labels whose visibility is toggled by scripts.
func claimDrawLabels(node *xfaNode) {
	for i, child := range node.Children {
		needsLabel := resolveInteractiveLabel(child) == "" || isPlaceholderCaption(child.Caption)
		shouldClaim := false
		switch child.Kind {
		case xfaKindExclGroup:
			shouldClaim = needsLabel
		case xfaKindField:
			shouldClaim = child.Bind != "none" && needsLabel
		case xfaKindSubform:
			shouldClaim = needsLabel
		}
		if shouldClaim {
			// Scan preceding siblings for a draw label.
			for j := i - 1; j >= 0; j-- {
				prev := node.Children[j]
				// Stop at any structural or interactive boundary.
				if prev.Kind == xfaKindSubform || prev.Kind == xfaKindPageArea ||
					prev.Kind == xfaKindField || prev.Kind == xfaKindExclGroup {
					break
				}
				if prev.Kind != xfaKindDraw {
					continue
				}
				// Skip colored indicator blocks and event-driven draws.
				if prev.UIType == "imageEdit" || len(prev.Events) > 0 {
					continue
				}
				if prev.Caption == "\x00consumed" {
					continue
				}
				// Skip variable-height content blocks.
				if prev.MinH != "" {
					continue
				}
				label := resolveDrawText(prev)
				if label == "" {
					continue
				}
				// Content blocks (HTML or positioned) with substantial text are not labels.
				// Short draws — even those with explicit h or exData — are row-aligned
				// question titles and are valid label candidates.
				if len(label) > 150 && (prev.HasExData || prev.H != "") {
					continue
				}
				child.Caption = label
				// Propagate the draw's tooltip (e.g. IMDRF TOC refs) to the subform.
				if child.ToolTip == "" && prev.ToolTip != "" {
					child.ToolTip = prev.ToolTip
				}
				prev.Caption = "\x00consumed"
				break
			}
		}
		// Recurse into subforms BEFORE checking first-child draws so that fields
		// within the subform can claim their sibling draws first.
		if child.Kind == xfaKindSubform {
			claimDrawLabels(child)
		}
		// For a subform still without a label after recursion: scan its leading draw
		// children (before any interactive content) as a fallback. This covers
		// group containers (e.g. *CheckboxGroup*) whose instruction/label draw is
		// the first child rather than a preceding sibling. Because we recurse first,
		// interactive-field draws are already consumed and won't be stolen.
		//
		// Only claim from a child draw when the subform has interactive descendants;
		// purely-static subforms have no label need — their draws become section Content.
		if shouldClaim && child.Kind == xfaKindSubform && child.Caption == "" && subformHasInteractive(child) {
			for _, gc := range child.Children {
				if gc.Kind == xfaKindField || gc.Kind == xfaKindExclGroup || gc.Kind == xfaKindSubform {
					break // stop at interactive content
				}
				if gc.Kind != xfaKindDraw || gc.UIType == "imageEdit" || len(gc.Events) > 0 || gc.Caption == "\x00consumed" {
					continue
				}
				// Same checks as the sibling-scan path above.
				if gc.MinH != "" {
					continue
				}
				label := resolveDrawText(gc)
				if label == "" {
					continue
				}
				if len(label) > 150 && (gc.HasExData || gc.H != "") {
					continue
				}
				child.Caption = label
				if child.ToolTip == "" && gc.ToolTip != "" {
					child.ToolTip = gc.ToolTip
				}
				gc.Caption = "\x00consumed"
				break
			}
		}
	}
}

// isPlaceholderCaption returns true for captions that are Adobe LiveCycle
// placeholder messages pointing to an adjacent draw for the real label.
func isPlaceholderCaption(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(s)), "the caption is in the textbox")
}

// walkSubformChildren iterates node.Children, emits questions to schema, and
// returns the FormSection slice for sections found at this level.
// parentHidden=true propagates hidden state from ancestor subforms.
func walkSubformChildren(node *xfaNode, path []string, schema *types.FormSchema, qIdx *int, parentHidden bool, verbose bool) []types.FormSection {
	var sections []types.FormSection
	for _, child := range node.Children {
		switch child.Kind {
		case xfaKindPageArea:
			// Page templates — isolated from main form content.

		case xfaKindSubform:
			sec := buildSection(child, path, schema, qIdx, parentHidden, verbose)
			sections = append(sections, sec)

		case xfaKindExclGroup:
			q := emitExclGroup(child, path, qIdx)
			schema.Questions = append(schema.Questions, q)

		case xfaKindField:
			if q, ok := emitField(child, path, qIdx, verbose); ok {
				schema.Questions = append(schema.Questions, q)
			}

		case xfaKindDraw:
			if q, ok := emitDraw(child, path, node, qIdx, parentHidden); ok {
				schema.Questions = append(schema.Questions, q)
			}
		}
	}
	return sections
}

// buildSection creates a FormSection for a subform node and recurses into its children.
func buildSection(node *xfaNode, parentPath []string, schema *types.FormSchema, qIdx *int, parentHidden bool, verbose bool) types.FormSection {
	path := make([]string, len(parentPath)+1)
	copy(path, parentPath)
	path[len(path)-1] = node.Name

	// Collect any events on the subform node itself as rules.
	for _, event := range node.Events {
		if newRules, err := convertXFAEventToRules(event, node.Name, len(schema.Rules)+1); err == nil {
			schema.Rules = append(schema.Rules, newRules...)
		}
	}

	startLen := len(schema.Questions)
	nodeHidden := parentHidden || node.Hidden
	interactive := isInteractiveSubtree(node)
	// Prefer bookmark name (from <extras name="bookmark">) as the authoritative label;
	// fall back to caption claimed from sibling/child draws.
	label := ""
	if bk := strings.TrimSpace(node.BookmarkName); bk != "" {
		label = bk
	} else if cap := strings.TrimSpace(node.Caption); cap != "" && !isPlaceholderCaption(cap) {
		label = cap
	}
	sec := types.FormSection{
		Name:        node.Name,
		Path:        strings.Join(path, "."),
		Label:       label,
		Tooltip:     node.ToolTip,
		Interactive: interactive,
		Layout:      node.Layout,
		Width:       node.W,
		Height:      node.H,
	}
	sec.Children = walkSubformChildren(node, path, schema, qIdx, nodeHidden, verbose)
	// Record all question IDs added during this section's subtree walk.
	if endLen := len(schema.Questions); endLen > startLen {
		sec.Questions = make([]string, 0, endLen-startLen)
		for _, q := range schema.Questions[startLen:endLen] {
			sec.Questions = append(sec.Questions, q.ID)
		}
	}
	// Collect static display text from non-interactive sections (headers, instructions, etc.)
	if !interactive {
		sec.Content = collectSectionContent(node)
	}
	return sec
}

// collectSectionContent returns static display text from a non-interactive subform.
// It walks draw children (and non-interactive sub-subforms) and extracts visible text.
//
// Filtering rules mirror emitDraw's suppression logic:
//   - Consumed draws (claimed as labels) and line-separator draws are skipped.
//   - imageEdit draws without image data are graphical status indicators (colored
//     bars/dots used for completion tracking in Adobe LiveCycle forms). Their
//     Caption is an accessibility label, not visible text — skip them.
//   - Event-bearing draws are script-managed — skip them.
//
// Text source: only n.Value / n.ExDataHTML / n.Default — the actual rendered text
// of the draw element. n.Caption is an XFA accessibility label set on graphical
// elements and must NOT be used here; it produces noise like "Required Question
// Incomplete" or "Part of a meter display" from status indicator widgets.
func collectSectionContent(node *xfaNode) []string {
	var result []string
	for _, child := range node.Children {
		switch child.Kind {
		case xfaKindDraw:
			if child.Caption == "\x00consumed" || child.IsLine {
				continue
			}
			if child.UIType == "imageEdit" && child.ImageData == "" && child.ImageHRef == "" {
				continue // graphical status block — accessibility caption is not content
			}
			if len(child.Events) > 0 {
				continue // script-managed draw
			}
			text := firstNonEmpty(child.ExDataHTML, child.Default, child.Value)
			if text != "" {
				result = append(result, text)
			}
		case xfaKindSubform:
			if !isInteractiveSubtree(child) {
				result = append(result, collectSectionContent(child)...)
			}
		}
	}
	return result
}

// emitExclGroup emits an XFA exclGroup as a radio-button question.
func emitExclGroup(node *xfaNode, path []string, qIdx *int) types.Question {
	*qIdx++
	q := types.Question{
		ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
		Name:       node.Name,
		Label:      resolveDisplayLabel(node),
		Type:       types.ResponseTypeRadio,
		Section:    sectionName(path),
		PageNumber: node.PageNumber,
	}
	if len(node.Options) > 0 {
		q.Options = make([]types.Option, len(node.Options))
		for i, opt := range node.Options {
			q.Options[i] = types.Option{Value: opt.Value, Label: opt.Label, Selected: opt.Selected}
		}
	}
	props := buildNodeProperties(node)
	if len(props) > 0 {
		q.Properties = props
	}
	return q
}

// emitField emits a <field> node as a Question. Returns (question, true) if the
// field should appear in the output, (zero, false) if it should be skipped.
func emitField(node *xfaNode, path []string, qIdx *int, verbose bool) (types.Question, bool) {
	if node.Bind == "none" {
		// Non-data-bound field — UI trigger or display label.
		if node.UIType == "button" {
			// Attachment upload buttons — render as file inputs.
			if strings.Contains(node.Name, "AddAttachment") {
				*qIdx++
				label := node.Caption
				if label == "" {
					label = "Add Attachment"
				}
				return types.Question{
					ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
					Name:       node.Name,
					Label:      label,
					Type:       types.ResponseTypeFile,
					Section:    sectionName(path),
					PageNumber: node.PageNumber,
					Hidden:     node.Hidden,
				}, true
			}
			return types.Question{}, false // UI trigger (Help Text, Show Intro, etc.)
		}
		displayText := resolveDrawText(node)
		if displayText == "" {
			return types.Question{}, false
		}
		*qIdx++
		return types.Question{
			ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
			Name:       node.Name,
			Label:      displayText,
			Type:       types.ResponseTypeDisplay,
			ReadOnly:   true,
			Section:    sectionName(path),
			PageNumber: node.PageNumber,
		}, true
	}
	// Data-bound interactive field.
	*qIdx++
	return convertNodeToQuestion(node, *qIdx, sectionName(path), verbose), true
}

// emitDraw emits a <draw> node as a Question. Returns (question, true) if the
// draw should be rendered, (zero, false) if it should be suppressed.
// parent is the subform node that owns this draw.
func emitDraw(node *xfaNode, path []string, parent *xfaNode, qIdx *int, parentHidden bool) (types.Question, bool) {
	// Structural classification — each check corresponds to a distinct entity type.
	if node.Caption == "\x00consumed" {
		return types.Question{}, false // claimed as exclGroup label
	}
	// Suppress exData draws only when no plain text could be extracted.
	// Draws with ExDataHTML set have their content recovered from text/html exData.
	if node.HasExData && node.ExDataHTML == "" {
		return types.Question{}, false // page counter / embedded reference with no text
	}
	if len(node.Events) > 0 {
		return types.Question{}, false // has its own event handlers → dynamic
	}
	if node.UIType == "imageEdit" && node.ImageData == "" && node.ImageHRef == "" {
		return types.Question{}, false // colored status block (YesIndicator / NoIndicator)
	}

	// Separator draw (<line> value).
	if node.IsLine {
		if !isInteractiveSubtree(parent) {
			return types.Question{}, false
		}
		*qIdx++
		props := buildNodeProperties(node)
		return types.Question{
			ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
			Name:       node.Name,
			Type:       types.ResponseTypeSeparator,
			ReadOnly:   true,
			Hidden:     parentHidden || node.Hidden,
			Section:    sectionName(path),
			PageNumber: node.PageNumber,
			Properties: props,
		}, true
	}

	// Image draw — inline base64 data or an href reference to an external resource.
	// These are emitted even when presence-attributed, so visibility rules can show/hide them.
	if node.ImageData != "" || node.ImageHRef != "" {
		*qIdx++
		props := buildNodeProperties(node)
		if props == nil {
			props = make(map[string]interface{})
		}
		if node.ImageData != "" {
			props["image_data"] = node.ImageData
		}
		if node.ImageHRef != "" {
			props["image_href"] = node.ImageHRef
		}
		props["content_type"] = node.ImageContentType
		return types.Question{
			ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
			Name:       node.Name,
			Label:      resolveInteractiveLabel(node),
			Type:       types.ResponseTypeImage,
			ReadOnly:   true,
			Hidden:     parentHidden || node.Hidden,
			Section:    sectionName(path),
			PageNumber: node.PageNumber,
			Properties: props,
		}, true
	}

	// Non-image draws: suppress if script-managed (presence= attr).
	// These are UI status indicators (colored blocks, dynamic labels) controlled by scripts.
	if node.HasPresenceAttr {
		return types.Question{}, false
	}

	// Static display draw (text or exData HTML).
	displayText := resolveDrawText(node)
	if displayText == "" {
		return types.Question{}, false
	}
	// Include static draws only when the parent section contains interactive fields.
	// Draws in pure-display sections (intro text, FAQ, help banners) are structural
	// content that the frontend renders via the Sections tree, not the flat list.
	if !isInteractiveSubtree(parent) {
		return types.Question{}, false
	}
	*qIdx++
	props := buildNodeProperties(node)
	return types.Question{
		ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
		Name:       node.Name,
		Label:      displayText,
		Type:       types.ResponseTypeDisplay,
		ReadOnly:   true,
		Section:    sectionName(path),
		PageNumber: node.PageNumber,
		Properties: props,
	}, true
}

// sectionName returns the nearest parent section name from a path slice.
func sectionName(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

// convertNodeToQuestion converts a data-bound xfaNode (Kind == xfaKindField) to a Question.
func convertNodeToQuestion(node *xfaNode, index int, section string, verbose bool) types.Question {
	q := types.Question{
		ID:          sanitizeFieldIDWithIndex(node.Name, index),
		Name:        node.Name,
		Label:       resolveDisplayLabel(node),
		Description: node.Description,
		Type:        mapXFAUITypeToResponseType(node.UIType, ""),
		Required:    node.Required,
		ReadOnly:    node.ReadOnly,
		Hidden:      node.Hidden,
		PageNumber:  node.PageNumber,
		Section:     section,
	}
	// Adjust dateTimeEdit type based on picture format.
	if node.UIType == "dateTimeEdit" && node.DateTimeSubType == "time" {
		q.Type = types.ResponseTypeTime
	}
	if node.Default != "" {
		q.Default = node.Default
	} else if node.Value != "" {
		q.Default = node.Value
	}
	if len(node.Options) > 0 {
		q.Options = make([]types.Option, len(node.Options))
		for i, opt := range node.Options {
			q.Options[i] = types.Option{Value: opt.Value, Label: opt.Label, Selected: opt.Selected}
		}
	}
	// Build validation — merge XFA validate element with textEdit maxChars.
	var val *types.ValidationRules
	if node.Validation != nil {
		val = &types.ValidationRules{
			MinLength:    node.Validation.MinLength,
			MaxLength:    node.Validation.MaxLength,
			MinValue:     node.Validation.MinValue,
			MaxValue:     node.Validation.MaxValue,
			Pattern:      node.Validation.Pattern,
			CustomScript: node.Validation.Script,
			ErrorMessage: node.Validation.ErrorMessage,
		}
	}
	if node.MaxChars != nil && *node.MaxChars > 0 {
		if val == nil {
			val = &types.ValidationRules{}
		}
		if val.MaxLength == nil {
			val.MaxLength = node.MaxChars
		}
	}
	q.Validation = val
	// Populate extended properties.
	if props := buildNodeProperties(node); len(props) > 0 {
		q.Properties = props
	}
	return q
}

// buildNodeProperties builds the Properties map for a Question from node metadata.
// Only non-default / non-zero values are included to keep the JSON lean.
func buildNodeProperties(node *xfaNode) map[string]interface{} {
	props := make(map[string]interface{})
	// Position / dimensions
	if node.X != "" {
		props["x"] = node.X
	}
	if node.Y != "" {
		props["y"] = node.Y
	}
	if node.W != "" {
		props["w"] = node.W
	}
	h := node.H
	if h == "" {
		h = node.MinH
	}
	if h != "" {
		props["h"] = h
	}
	// Layout (exclGroup / subform)
	if node.Layout != "" {
		props["layout"] = node.Layout
	}
	// Caption placement
	if node.CaptionPlacement != "" {
		props["caption_placement"] = node.CaptionPlacement
	}
	// Text display
	if node.TextAlign != "" {
		props["text_align"] = node.TextAlign
	}
	if node.FontSize != "" {
		props["font_size"] = node.FontSize
	}
	if node.FontWeight != "" && node.FontWeight != "normal" {
		props["font_weight"] = node.FontWeight
	}
	// UI constraints
	if node.AllowNeutral {
		props["allow_neutral"] = true
	}
	if node.ChoiceListOpen == "always" {
		props["listbox"] = true
	}
	if node.ChoiceListMultiSelect {
		props["multi_select"] = true
	}
	if node.FracDigits != nil {
		props["frac_digits"] = *node.FracDigits
	}
	if node.LeadDigits != nil {
		props["lead_digits"] = *node.LeadDigits
	}
	if len(props) == 0 {
		return nil
	}
	return props
}

// mapXFAUITypeToResponseType maps a UI element name (from <ui> children) to a ResponseType.
// This is more reliable than the field's type attribute.
func mapXFAUITypeToResponseType(uiType, fieldType string) types.ResponseType {
	switch strings.ToLower(uiType) {
	case "textedit":
		return types.ResponseTypeText
	case "textedit multiline", "texteditmultiline":
		return types.ResponseTypeTextarea
	case "checkbutton":
		return types.ResponseTypeCheckbox
	case "choicelist":
		return types.ResponseTypeSelect
	case "radiobutton":
		return types.ResponseTypeRadio
	case "datetimeedit":
		return types.ResponseTypeDate
	case "numericedit":
		return types.ResponseTypeNumber
	case "passwordedit":
		return types.ResponseTypePassword
	case "button":
		return types.ResponseTypeButton
	case "signature":
		return types.ResponseTypeSignature
	default:
		return mapXFATypeToResponseTypeEnum(fieldType)
	}
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

// ── Rules extraction ──────────────────────────────────────────────────────────

// collectFieldEvents collects all field/exclGroup nodes that have events.
func collectFieldEvents(node *xfaNode, out *[]*xfaNode) {
	if (node.Kind == xfaKindField || node.Kind == xfaKindExclGroup) && len(node.Events) > 0 {
		*out = append(*out, node)
	}
	for _, child := range node.Children {
		collectFieldEvents(child, out)
	}
}

// extractXFARules extracts control flow rules from field events in the xfaNode tree.
func extractXFARules(root *xfaNode, verbose bool) ([]types.Rule, error) {
	var fieldNodes []*xfaNode
	collectFieldEvents(root, &fieldNodes)

	rules := make([]types.Rule, 0)
	ruleIndex := 1
	for _, node := range fieldNodes {
		for _, event := range node.Events {
			newRules, err := convertXFAEventToRules(event, node.Name, ruleIndex)
			if err != nil {
				if verbose {
					log.Printf("Warning: Failed to convert event to rule: %v", err)
				}
				continue
			}
			rules = append(rules, newRules...)
			ruleIndex += len(newRules)
		}
	}
	return rules, nil
}

// convertXFAEventToRules converts an XFA event to one or more Rules.
// Visibility scripts with if/else branches generate two rules (if-branch + else-branch).
func convertXFAEventToRules(event XFAEvent, sourceField string, index int) ([]types.Rule, error) {
	defaultType := types.RuleTypeCalculate
	switch strings.ToLower(event.Type) {
	case "validate":
		defaultType = types.RuleTypeValidate
	case "calculate":
		defaultType = types.RuleTypeCalculate
	case "initialize", "docready":
		defaultType = types.RuleTypeSetValue
	}

	if event.Script == "" {
		return []types.Rule{{
			ID:      fmt.Sprintf("rule_%d", index),
			Source:  sourceField,
			Type:    defaultType,
			Actions: []types.Action{},
		}}, nil
	}

	results := parseXFAScript(event.Script, sourceField, event.Target, event.Lang)
	out := make([]types.Rule, 0, len(results))
	for i, parsed := range results {
		ruleType := defaultType
		if parsed.ruleType != "" {
			ruleType = parsed.ruleType
		}
		rule := types.Rule{
			ID:          fmt.Sprintf("rule_%d", index+i),
			Source:      sourceField,
			Type:        ruleType,
			Description: parsed.description,
			Condition:   parsed.condition,
			Actions:     parsed.actions,
		}
		if rule.Actions == nil {
			rule.Actions = []types.Action{}
		}
		out = append(out, rule)
	}
	return out, nil
}

// scriptParseResult holds the structured result of parsing an XFA script body.
type scriptParseResult struct {
	ruleType    types.RuleType
	condition   *types.Condition
	actions     []types.Action
	description string
}

// parseXFAScript analyses an XFA script (FormCalc or JavaScript) and returns
// one or more structured results. Visibility scripts with if/else produce two.
// It always succeeds — unknown scripts return a single ActionTypeExecute action.
// langHint is from the script element's contentType attribute ("formcalc" | "javascript" | "").
func parseXFAScript(script, sourceField, targetField, langHint string) []scriptParseResult {
	s := strings.TrimSpace(script)
	lang := detectScriptLanguage(s, langHint)

	// Try each pattern family in order of specificity.
	// Validation is tried before set-value so that scripts containing both
	// a rawValue reference and return true/false are classified as validation.
	if rs, ok := tryParseVisibilityScript(s, lang, sourceField, targetField); ok {
		return rs
	}
	if r, ok := tryParseValidationScript(s, lang, sourceField); ok {
		return []scriptParseResult{r}
	}
	if r, ok := tryParseSetValueScript(s, lang, sourceField, targetField); ok {
		return []scriptParseResult{r}
	}
	if r, ok := tryParseCalculateScript(s, lang, sourceField); ok {
		return []scriptParseResult{r}
	}

	// Fallback: wrap raw script in an execute action.
	return []scriptParseResult{{
		actions: []types.Action{{
			Type:       types.ActionTypeExecute,
			Target:     targetField,
			Script:     script,
			Expression: script,
		}},
	}}
}

// contentTypeToLang maps a <script contentType="..."> value to "formcalc" or "javascript".
func contentTypeToLang(ct string) string {
	switch strings.ToLower(ct) {
	case "application/x-formcalc":
		return "formcalc"
	case "application/x-javascript", "text/javascript":
		return "javascript"
	}
	return ""
}

// detectScriptLanguage returns "formcalc" or "javascript".
// hint is from the script element's contentType attribute; if non-empty it wins.
// FormCalc uses `then`/`endif` keywords; JavaScript uses `{`/`}` blocks.
func detectScriptLanguage(s, hint string) string {
	if hint == "formcalc" || hint == "javascript" {
		return hint
	}
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

// tryParseVisibilityScript detects scripts that show or hide one or more fields.
// Returns 1 result for plain visibility scripts, 2 for if/else (one per branch).
//
// FormCalc patterns:
//
//	$.presence = "visible" / "hidden" / "invisible"
//	if (cond) then $.presence = "hidden" else $.presence = "visible" endif
//
// JavaScript patterns:
//
//	this.presence = "visible"
//	xfa.resolveNode("fieldName").presence = "hidden"
//	Name.presence = "hidden"  (dotted-path, eSTAR style)
func tryParseVisibilityScript(s, lang, sourceField, targetField string) ([]scriptParseResult, bool) {
	lower := strings.ToLower(s)
	if !strings.Contains(lower, "presence") {
		return nil, false
	}

	cond := extractSimpleCondition(s, lang, sourceField)

	// Attempt if/else split for accurate bidirectional visibility.
	if cond != nil {
		if ifBody, elseBody, ok := splitIfElse(s, lang); ok {
			return buildIfElseVisibilityRules(ifBody, elseBody, cond, sourceField, targetField), true
		}
	}

	// Single-branch: extract all targets from the full script.
	targets := extractAllPresenceTargets(s)
	if len(targets) == 0 {
		if targetField != "" {
			targets = []string{targetField}
		} else {
			targets = []string{sourceField}
		}
	}

	dominant := dominantActionType(lower)
	actions := presenceActionsForTargets(s, targets, dominant)

	return []scriptParseResult{{
		ruleType:  types.RuleTypeVisibility,
		condition: cond,
		actions:   actions,
	}}, true
}

// buildIfElseVisibilityRules creates two visibility rules from an if/else script.
func buildIfElseVisibilityRules(ifBody, elseBody string, cond *types.Condition, sourceField, targetField string) []scriptParseResult {
	ifTargets := extractAllPresenceTargets(ifBody)
	elseTargets := extractAllPresenceTargets(elseBody)

	if len(ifTargets) == 0 {
		if targetField != "" {
			ifTargets = []string{targetField}
		} else {
			ifTargets = []string{sourceField}
		}
	}
	if len(elseTargets) == 0 {
		// Mirror the if-branch targets with inverted action types.
		elseTargets = ifTargets
	}

	ifActions := presenceActionsForTargets(ifBody, ifTargets, dominantActionType(strings.ToLower(ifBody)))
	elseActions := presenceActionsForTargets(elseBody, elseTargets, dominantActionType(strings.ToLower(elseBody)))

	return []scriptParseResult{
		{ruleType: types.RuleTypeVisibility, condition: cond, actions: ifActions},
		{ruleType: types.RuleTypeVisibility, condition: invertCondition(cond), actions: elseActions},
	}
}

// dominantActionType returns ActionTypeHide if "hidden"/"invisible" appears, else ActionTypeShow.
func dominantActionType(lower string) types.ActionType {
	if strings.Contains(lower, `"hidden"`) || strings.Contains(lower, `"invisible"`) ||
		strings.Contains(lower, "'hidden'") || strings.Contains(lower, "'invisible'") {
		return types.ActionTypeHide
	}
	return types.ActionTypeShow
}

// presenceActionsForTargets builds an Action slice with per-target show/hide.
func presenceActionsForTargets(s string, targets []string, dominant types.ActionType) []types.Action {
	actions := make([]types.Action, 0, len(targets))
	for _, t := range targets {
		actions = append(actions, types.Action{
			Type:   perTargetActionType(s, t, dominant),
			Target: t,
		})
	}
	return actions
}

// splitIfElse extracts the if-body and else-body from a conditional script.
// Returns (ifBody, elseBody, true) if a parseable if/else is found.
func splitIfElse(s, lang string) (ifBody, elseBody string, ok bool) {
	lower := strings.ToLower(s)
	if lang == "formcalc" {
		thenIdx := strings.Index(lower, " then")
		if thenIdx < 0 {
			thenIdx = strings.Index(lower, "\tthen")
		}
		endIdx := strings.LastIndex(lower, "endif")
		if thenIdx < 0 || endIdx < 0 {
			return
		}
		body := s[thenIdx+5 : endIdx]
		bodyLower := strings.ToLower(body)
		elseIdx := strings.Index(bodyLower, "\nelse")
		if elseIdx < 0 {
			elseIdx = strings.Index(bodyLower, "\r\nelse")
		}
		if elseIdx < 0 {
			return
		}
		return strings.TrimSpace(body[:elseIdx]), strings.TrimSpace(body[elseIdx+5:]), true
	}
	// JavaScript: find the end of the first { } block then look for "else {"
	braceOpen := strings.Index(s, "{")
	if braceOpen < 0 {
		return
	}
	depth := 0
	for i := braceOpen; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				rest := strings.TrimSpace(s[i+1:])
				if strings.HasPrefix(strings.ToLower(rest), "else") {
					// Find { and } of the else block.
					elseOpen := strings.Index(rest, "{")
					elseClose := strings.LastIndex(rest, "}")
					if elseOpen >= 0 && elseClose > elseOpen {
						return s[braceOpen+1 : i], rest[elseOpen+1 : elseClose], true
					}
				}
				return
			}
		}
	}
	return
}

// invertCondition returns the logical inverse of a Condition.
func invertCondition(c *types.Condition) *types.Condition {
	if c == nil {
		return nil
	}
	switch c.Operator {
	case types.OperatorEquals:
		return &types.Condition{Operator: types.OperatorNotEquals, Expression: c.Expression, Value: c.Value, Children: c.Children}
	case types.OperatorNotEquals:
		return &types.Condition{Operator: types.OperatorEquals, Expression: c.Expression, Value: c.Value, Children: c.Children}
	case types.OperatorGreaterThan:
		return &types.Condition{Operator: types.OperatorLessOrEqual, Expression: c.Expression, Value: c.Value, Children: c.Children}
	case types.OperatorLessThan:
		return &types.Condition{Operator: types.OperatorGreaterOrEqual, Expression: c.Expression, Value: c.Value, Children: c.Children}
	}
	return &types.Condition{Logic: types.LogicOpNot, Children: []types.Condition{*c}}
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

// extractAllPresenceTargets collects every field name that a script assigns
// .presence to, handling both xfa.resolveNode("name") and Name.presence= patterns.
func extractAllPresenceTargets(s string) []string {
	seen := map[string]bool{}
	var out []string

	// resolveNode("name").presence pattern — only static string arguments.
	lower := strings.ToLower(s)
	for offset := 0; ; {
		idx := strings.Index(lower[offset:], "resolvenode(")
		if idx == -1 {
			break
		}
		abs := offset + idx
		rest := strings.TrimSpace(s[abs+12:])
		// Skip dynamic expressions — argument must start with a quote.
		if len(rest) == 0 || (rest[0] != '"' && rest[0] != '\'') {
			offset = abs + 12
			continue
		}
		quote := rest[0]
		rest = rest[1:]
		end := strings.IndexByte(rest, quote)
		if end == -1 {
			offset = abs + 12
			continue
		}
		name := rest[:end]
		// Strip leading SOM path (e.g. "xfa.form.root.Section.field" → "field").
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			name = name[dot+1:]
		}
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
		offset = abs + 12
	}

	// Name.presence = pattern (identifier followed by .presence)
	for _, m := range presenceTargetRe.FindAllStringSubmatch(s, -1) {
		name := m[1]
		// Exclude XFA built-ins that are never field names.
		if name == "this" || name == "xfa" || name == "event" || name == "xfaForm" {
			continue
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// perTargetActionType determines whether a script shows or hides a specific
// named target by looking for "target.presence = '...'" nearby.
func perTargetActionType(s, target string, fallback types.ActionType) types.ActionType {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(target) + `\.presence\s*=\s*["'](\w+)["']`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return fallback
	}
	v := strings.ToLower(m[1])
	if v == "hidden" || v == "invisible" || v == "inactive" {
		return types.ActionTypeHide
	}
	return types.ActionTypeShow
}

// extractResolveNodeTarget parses `xfa.resolveNode("fieldName")` and returns fieldName.
// Only handles static string arguments; returns "" for dynamic expressions.
func extractResolveNodeTarget(s string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, "resolvenode(")
	if idx == -1 {
		return ""
	}
	rest := strings.TrimSpace(s[idx+12:])
	if len(rest) == 0 || (rest[0] != '"' && rest[0] != '\'') {
		return ""
	}
	quote := rest[0]
	rest = rest[1:]
	end := strings.IndexByte(rest, quote)
	if end == -1 {
		return ""
	}
	name := rest[:end]
	// Strip leading SOM path.
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	return name
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

// firstNonEmpty returns the first non-blank string from vals.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

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

// ── Variables function rule extraction ───────────────────────────────────────

// extractJSFunctionBodies parses function definitions from a JavaScript block
// and returns each function's body (content between the outer braces).
func extractJSFunctionBodies(script, lang string) []variablesFuncDef {
	var out []variablesFuncDef
	matches := xfaFuncDeclRe.FindAllStringSubmatchIndex(script, -1)
	for _, m := range matches {
		// m[1]-1 is the position of '{' ending the function signature.
		braceStart := m[1] - 1
		if braceStart < 0 || braceStart >= len(script) || script[braceStart] != '{' {
			continue
		}
		end := matchingBraceJS(script, braceStart)
		if end < 0 {
			continue
		}
		body := script[braceStart+1 : end]
		if strings.TrimSpace(body) != "" {
			out = append(out, variablesFuncDef{body: body, lang: lang})
		}
	}
	return out
}

// matchingBraceJS returns the index of the closing '}' that matches the '{' at
// position start. Returns -1 if not found.
func matchingBraceJS(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// matchingParenJS returns the index of the closing ')' matching the '(' at start.
func matchingParenJS(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ifChainBranch is a single branch of an if/else-if*/else? chain.
type ifChainBranch struct {
	cond string // raw JS condition expression; empty for the final else branch
	body string // body of the branch (content between { and })
}

// splitJSIfElseChain splits a JavaScript if/else-if*/else? chain into branches.
// Returns nil if the script doesn't start with an if statement.
func splitJSIfElseChain(s string) []ifChainBranch {
	var branches []ifChainBranch
	rest := strings.TrimSpace(s)

	for {
		lower := strings.ToLower(rest)
		if !strings.HasPrefix(lower, "if") {
			break
		}
		// Require "if (" or "if(" — not "ifdef" etc.
		if len(rest) > 2 && rest[2] != '(' && rest[2] != ' ' && rest[2] != '\t' {
			break
		}

		// Find condition in ( ... )
		parenStart := strings.Index(rest, "(")
		if parenStart < 0 {
			break
		}
		parenEnd := matchingParenJS(rest, parenStart)
		if parenEnd < 0 {
			break
		}
		cond := strings.TrimSpace(rest[parenStart+1 : parenEnd])

		// Find the body block { ... }
		braceStart := strings.Index(rest[parenEnd:], "{")
		if braceStart < 0 {
			break
		}
		braceStart += parenEnd
		braceEnd := matchingBraceJS(rest, braceStart)
		if braceEnd < 0 {
			break
		}
		body := rest[braceStart+1 : braceEnd]
		branches = append(branches, ifChainBranch{cond: cond, body: body})
		rest = strings.TrimSpace(rest[braceEnd+1:])

		// Check for else / else if
		lower = strings.ToLower(rest)
		if !strings.HasPrefix(lower, "else") {
			break
		}
		rest = strings.TrimSpace(rest[4:]) // consume "else"
		lower = strings.ToLower(rest)
		if strings.HasPrefix(lower, "if") {
			// else if — continue the loop
			continue
		}
		// Final else { ... }
		if strings.HasPrefix(lower, "{") {
			braceEnd2 := matchingBraceJS(rest, 0)
			if braceEnd2 >= 0 {
				branches = append(branches, ifChainBranch{body: rest[1:braceEnd2]})
			}
		}
		break
	}

	return branches
}

// parseVariablesFunctionRules extracts visibility rules from a <variables>
// function body. The function body typically contains an if/else-if chain
// keyed on a radio-button or select field's rawValue.
func parseVariablesFunctionRules(funcBody, lang string, baseIndex int) []types.Rule {
	lower := strings.ToLower(funcBody)
	if !strings.Contains(lower, "presence") {
		return nil
	}

	var branches []ifChainBranch
	if lang == "javascript" || lang == "" {
		branches = splitJSIfElseChain(strings.TrimSpace(funcBody))
	}

	var source string
	if len(branches) > 0 && branches[0].cond != "" {
		if m := xfaRawValueCondRe.FindStringSubmatch(branches[0].cond); m != nil {
			source = m[1]
		}
	}

	var rules []types.Rule

	if len(branches) > 0 {
		// Generate one rule per branch with the exact condition.
		for i, branch := range branches {
			targets := extractAllPresenceTargets(branch.body)
			if len(targets) == 0 {
				continue
			}
			actions := presenceActionsForTargets(branch.body, targets,
				dominantActionType(strings.ToLower(branch.body)))
			if len(actions) == 0 {
				continue
			}
			var cond *types.Condition
			if branch.cond != "" {
				cond = &types.Condition{Expression: branch.cond}
			}
			rules = append(rules, types.Rule{
				ID:      fmt.Sprintf("rule_var_%d", baseIndex+i+1),
				Source:  source,
				Type:    types.RuleTypeVisibility,
				Actions: actions,
				Condition: cond,
			})
		}
	} else {
		// Fallback: no multi-branch parse — use tryParseVisibilityScript on the whole body.
		results := parseXFAScript(funcBody, source, "", lang)
		for i, parsed := range results {
			if parsed.ruleType != types.RuleTypeVisibility {
				continue
			}
			rule := types.Rule{
				ID:      fmt.Sprintf("rule_var_%d", baseIndex+i+1),
				Source:  source,
				Type:    types.RuleTypeVisibility,
				Condition: parsed.condition,
				Actions: parsed.actions,
			}
			if rule.Actions == nil {
				rule.Actions = []types.Action{}
			}
			rules = append(rules, rule)
		}
	}

	return rules
}
