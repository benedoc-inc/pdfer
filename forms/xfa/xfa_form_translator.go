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

	"github.com/benedoc-inc/pdfer/v2/types"
)

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
	if len(resources) > 0 {
		resolveImageHRefs(schema, resources)
	}
	if verbose {
		log.Printf("Parsed XFA to FormSchema: %d questions, %d sections, %d scripts",
			len(schema.Questions), len(schema.Sections), len(schema.Scripts))
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
		log.Printf("Converting FormSchema to XFA XML: %d questions, %d scripts", len(formSchema.Questions), len(formSchema.Scripts))
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
	Caption          string // from <caption><value><text> or <label>
	CaptionPlacement string // from <caption placement="left|right|top|bottom|inline">
	ToolTip          string // from <assist><toolTip> — accessibility annotation (e.g. IMDRF TOC refs)
	SpeakLabel       string // from <assist><speak>
	BookmarkName     string // from <extras name="bookmark"><text name="name"> — PDF bookmark label

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

	// OptionEvents is parallel to Options on exclGroup nodes: one []XFAEvent per
	// flattened option <field>. Lets per-option event scripts (originally hung off
	// the individual <field> inside an <exclGroup>) survive the flatten so they
	// can still be surfaced as FormScripts.
	OptionEvents [][]XFAEvent

	// OptionFieldNames is parallel to Options on exclGroup nodes: the SOM-addressable
	// name attribute of each flattened option <field>. Used to build the per-option
	// script OwnerPath as "group.fieldName" (real SOM), independent of the option's
	// data value (which may contain arbitrary text from <items>).
	OptionFieldNames []string

	// UI-element-specific constraints
	AllowNeutral          bool   // checkButton allowNeutral="1" → tri-state checkbox
	MaxChars              *int   // textEdit maxChars → ValidationRules.MaxLength
	FracDigits            *int   // numericEdit fracDigits (decimal places)
	LeadDigits            *int   // numericEdit leadDigits (max integer digits)
	ChoiceListOpen        string // choiceList open: "always"=listbox, "userInteraction"=dropdown
	ChoiceListMultiSelect bool   // choiceList multiSelect="1"
	DateTimeSubType       string // "time" when picture starts with TIME{; "datetime" for combined

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
	Root             *xfaNode
	Title            string
	Description      string
	Version          string
	VariablesScripts []variablesScript
}

// variablesScript holds a <variables><script> block extracted verbatim from
// the template. OwnerStack is a snapshot of the subform/pageArea stack at the
// time the <variables> element opened; the SOM OwnerPath is resolved from it
// after parse completes (subformStackPath needs fully-populated Children to
// disambiguate same-named siblings). Empty stack ⇒ template-level block.
type variablesScript struct {
	OwnerStack []*xfaNode
	Name       string // <script name="...">
	Lang       string
	Body       string
	Properties map[string]interface{} // unknown <script> attrs (id, url, binding, stateless, …)
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

// XFAEvent represents a single <event> attached to a field, exclGroup, or subform.
// It captures the raw <event> and child <script> attributes; bodies are exposed
// verbatim via FormScript — pdfer does not interpret script semantics.
type XFAEvent struct {
	Type       string                 // <event activity="..."> — "initialize", "change", "click", etc.
	Name       string                 // <event name="..."> — Adobe convention is the same word as Type
	RunAt      string                 // <script runAt="..."> — "client" | "server" | "both"
	Lang       string                 // "formcalc" | "javascript" — derived from <script contentType="...">
	Body       string                 // verbatim <script> content
	Properties map[string]interface{} // unknown <event>/<script> attrs (listen, ref, id, binding, …)
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

	var currentLeaf *xfaNode
	var currentValue strings.Builder
	var currentCaption strings.Builder
	var currentLabel strings.Builder
	var currentDesc strings.Builder
	var currentToolTip strings.Builder
	var currentSpeak strings.Builder
	var currentImageData strings.Builder
	var imageContentType string

	// exData HTML extraction state
	var exDataContentType string
	var exDataHTMLBuf strings.Builder
	var exDataHasEmbed bool

	// picture element state (for dateTimeEdit subtype detection)
	var inPicture bool
	var currentPicture strings.Builder

	var inValue bool
	var inCaption bool
	var inExclGroupCaption bool // reading the exclGroup's own <caption>, not a child field's
	var inSubformCaption bool   // reading the <caption> that is a direct child of a subform
	var inLabel bool
	var inDescription bool
	var inItems bool
	var itemsIsSave bool
	var inImage bool
	var inToolTip bool
	var inAssist bool
	var inSpeak bool
	var inExData bool
	var inCaptionExData bool // exData nested inside a <caption> — routes to currentCaption
	var inScript bool
	var inVariables bool       // inside a <variables> element
	var inVariablesScript bool // inside a <variables><script> element
	var variablesScriptLang string
	var variablesScriptName string
	var variablesOwnerStack []*xfaNode
	var variablesScriptProperties map[string]interface{}

	// bookmark extras state
	var inBookmarkExtras bool
	var inBookmarkName bool
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
				variablesOwnerStack = append([]*xfaNode(nil), nodeStack...)

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
				ev := XFAEvent{}
				for _, attr := range se.Attr {
					switch attr.Name.Local {
					case "activity":
						ev.Type = attr.Value
					case "name":
						ev.Name = attr.Value
					default:
						putAttr(&ev.Properties, attr.Name.Local, attr.Value)
					}
				}
				if currentLeaf != nil {
					currentLeaf.Events = append(currentLeaf.Events, ev)
				} else if top := topOfStack(); top.Kind == xfaKindSubform || top.Kind == xfaKindPageArea || top.Kind == xfaKindExclGroup {
					top.Events = append(top.Events, ev)
				}

			case "script":
				if inVariables {
					inVariablesScript = true
					variablesScriptLang = "formcalc" // XFA default per spec
					variablesScriptName = ""
					variablesScriptProperties = nil
					currentValue.Reset()
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "contentType":
							if l := contentTypeToLang(attr.Value); l != "" {
								variablesScriptLang = l
							}
						case "name":
							variablesScriptName = attr.Value
						default:
							putAttr(&variablesScriptProperties, attr.Name.Local, attr.Value)
						}
					}
				} else if currentLeaf != nil && len(currentLeaf.Events) > 0 {
					inScript = true
					currentValue.Reset()
					last := &currentLeaf.Events[len(currentLeaf.Events)-1]
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "contentType":
							last.Lang = contentTypeToLang(attr.Value)
						case "runAt":
							last.RunAt = attr.Value
						default:
							putAttr(&last.Properties, attr.Name.Local, attr.Value)
						}
					}
				} else if top := topOfStack(); (top.Kind == xfaKindSubform || top.Kind == xfaKindPageArea || top.Kind == xfaKindExclGroup) && len(top.Events) > 0 {
					inScript = true
					currentValue.Reset()
					last := &top.Events[len(top.Events)-1]
					for _, attr := range se.Attr {
						switch attr.Name.Local {
						case "contentType":
							last.Lang = contentTypeToLang(attr.Value)
						case "runAt":
							last.RunAt = attr.Value
						default:
							putAttr(&last.Properties, attr.Name.Local, attr.Value)
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
						// Preserve per-option events and the field's SOM-addressable name
						// (parallel slices) so they can still surface as FormScripts with
						// a real SOM OwnerPath even though the option <field> is flattened.
						top.OptionEvents = append(top.OptionEvents, currentLeaf.Events)
						top.OptionFieldNames = append(top.OptionFieldNames, currentLeaf.Name)
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
					body := currentValue.String()
					if strings.TrimSpace(body) != "" {
						res.VariablesScripts = append(res.VariablesScripts, variablesScript{
							OwnerStack: variablesOwnerStack,
							Name:       variablesScriptName,
							Lang:       variablesScriptLang,
							Body:       body,
							Properties: variablesScriptProperties,
						})
					}
					inVariablesScript = false
					variablesScriptName = ""
					variablesScriptProperties = nil
				} else if currentLeaf != nil && len(currentLeaf.Events) > 0 {
					last := &currentLeaf.Events[len(currentLeaf.Events)-1]
					last.Body = currentValue.String()
				} else if top := topOfStack(); (top.Kind == xfaKindSubform || top.Kind == xfaKindPageArea || top.Kind == xfaKindExclGroup) && len(top.Events) > 0 {
					last := &top.Events[len(top.Events)-1]
					last.Body = currentValue.String()
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
// questions list, a hierarchical sections tree, and a flat Scripts slice
// indexed by Question.Scripts and FormSection.Scripts.
func buildFormSchema(result *xfaTemplateResult, verbose bool) *types.FormSchema {
	schema := &types.FormSchema{
		Metadata: types.FormMetadata{
			FormType:    "XFA",
			Title:       result.Title,
			Description: result.Description,
			Version:     result.Version,
		},
		Questions: make([]types.Question, 0),
	}

	// Pre-pass: claim preceding draw nodes as labels for unlabeled exclGroups.
	claimDrawLabels(result.Root)

	var qIdx int
	schema.Sections = walkSubformChildren(result.Root, nil, schema, &qIdx, false, verbose)

	// Extract non-question template nodes (bind="none" buttons, event-bearing
	// <draw>s, <pageArea>s) as FormElements so they have a typed owner that
	// downstream renderers and orphan FormScripts can dereference.
	extractAllElements(result.Root, schema)

	// Extract every <script> body in the template — including those on nodes
	// pdfer doesn't emit as a Question or FormSection (per-option events
	// inside an <exclGroup>). OwnerID is filled in by the back-ref pass below
	// when the owner happens to also be a Question/Section/Element; remaining
	// orphan scripts keep OwnerID empty and rely on OwnerPath for correlation.
	extractAllScripts(result.Root, schema)
	populateScriptBackRefs(schema)

	// Append <variables><script> blocks verbatim as FormScripts. These are
	// template-wide helper definitions (functions invoked from field events) and
	// are exposed as-is for callers to interpret.
	for i, vs := range result.VariablesScripts {
		ownerPath := subformStackPath(vs.OwnerStack)
		script := types.FormScript{
			ID:         formScriptID(ownerPath, "variables", i),
			OwnerPath:  ownerPath,
			Event:      "variables",
			Name:       vs.Name,
			Language:   vs.Lang,
			Body:       vs.Body,
			Properties: vs.Properties,
		}
		schema.Scripts = append(schema.Scripts, script)
	}

	return schema
}

// subformStackPath joins the names of subform/pageArea nodes on the stack into
// a dot-separated SOM-style path. Skips the synthetic "_root" node so the
// returned path matches FormSection.Path values. Returns "" when only _root
// is on the stack. Same-named siblings receive "[i]" suffixes via somSegment.
//
// Must be called after parsing completes — somSegment counts siblings against
// each parent's Children slice, which only finalizes on close-tag. Mid-parse
// callers should snapshot the stack and resolve here later.
func subformStackPath(stack []*xfaNode) string {
	parts := make([]string, 0, len(stack))
	for i, n := range stack {
		if n.Kind != xfaKindSubform && n.Kind != xfaKindPageArea {
			continue
		}
		if n.Name == "" || n.Name == "_root" {
			continue
		}
		var parent *xfaNode
		if i > 0 {
			parent = stack[i-1]
		}
		parts = append(parts, somSegment(parent, n))
	}
	return somJoin(parts)
}

// formScriptID builds a stable FormScript ID from the owner's SOM path,
// the event activity, and the event's declaration index.
func formScriptID(ownerPath, event string, index int) string {
	prefix := ownerPath
	if prefix == "" {
		prefix = "$template"
	}
	if event == "" {
		event = "event"
	}
	return fmt.Sprintf("%s#%s[%d]", prefix, event, index)
}

// buildFormScripts converts a node's []XFAEvent into []FormScript with stable
// IDs. ownerPath is the SOM path of the owner; ownerID is the Question.ID or
// FormSection.Path that callers can dereference. Events without a body are
// skipped — they're empty event declarations that carry no payload.
func buildFormScripts(events []XFAEvent, ownerPath, ownerID string) []types.FormScript {
	out := make([]types.FormScript, 0, len(events))
	for i, ev := range events {
		if strings.TrimSpace(ev.Body) == "" {
			continue
		}
		lang := ev.Lang
		if lang == "" {
			lang = "formcalc" // XFA default per spec when contentType is absent
		}
		out = append(out, types.FormScript{
			ID:         formScriptID(ownerPath, ev.Type, i),
			OwnerPath:  ownerPath,
			OwnerID:    ownerID,
			Event:      ev.Type,
			Name:       ev.Name,
			Language:   lang,
			RunAt:      ev.RunAt,
			Body:       ev.Body,
			Properties: ev.Properties,
		})
	}
	return out
}

// extractAllScripts walks the entire xfaNode tree and emits a FormScript for
// every event-bearing node — regardless of whether pdfer also surfaces that
// node as a Question or FormSection. OwnerPath is the SOM path of the owning
// node. OwnerID is left blank here and filled in by populateScriptBackRefs
// when the owner also became a Question/Section; it stays empty for orphan
// scripts (event-bearing <draw>s, bind="none" non-AddAttachment buttons,
// <pageArea>s, and per-option events flattened out of an <exclGroup>).
func extractAllScripts(root *xfaNode, schema *types.FormSchema) {
	var walk func(node, parent *xfaNode, parentPath []string)
	walk = func(node, parent *xfaNode, parentPath []string) {
		nodePath := parentPath
		if node.Name != "" && node.Name != "_root" {
			nodePath = somAppend(parentPath, somSegment(parent, node))
		}
		somPath := somJoin(nodePath)

		if len(node.Events) > 0 {
			schema.Scripts = append(schema.Scripts, buildFormScripts(node.Events, somPath, "")...)
		}

		// Per-option events on exclGroup — OptionEvents and OptionFieldNames are
		// parallel slices populated when child <field>s are flattened into the
		// group. The field's name is the SOM-addressable identifier ("group.optA"),
		// independent of the option's data value (which may contain arbitrary
		// text from <items>). Same-named option fields are disambiguated by
		// somSegmentFromSiblingNames (the underlying *xfaNode is discarded at
		// flatten time, so we recover the index from the parallel name slice).
		if node.Kind == xfaKindExclGroup {
			for i, optEvents := range node.OptionEvents {
				if len(optEvents) == 0 {
					continue
				}
				seg := somSegmentFromSiblingNames(node.OptionFieldNames, i)
				if seg == "" {
					continue
				}
				optPath := somJoin(somAppend(nodePath, seg))
				schema.Scripts = append(schema.Scripts, buildFormScripts(optEvents, optPath, "")...)
			}
		}

		for _, child := range node.Children {
			walk(child, node, nodePath)
		}
	}
	walk(root, nil, nil)
}

// extractAllElements walks the entire xfaNode tree and emits a FormElement for
// every non-question template node that downstream renderers need to address.
// Three roles are surfaced:
//   - "button" — bind="none" non-AddAttachment buttons (Help Text, Show Intro,
//     etc.). AddAttachment stays in Questions as ResponseTypeFile because it
//     accepts user input even though it doesn't bind to the dataset.
//   - "draw" — only draws with events (dynamic show/hide regions); static
//     label draws stay out and are surfaced as ResponseTypeDisplay Questions
//     by emitDraw.
//   - "pageArea" — surfaced unconditionally because pageAreas always carry
//     structural meaning (the page layout itself), independent of whether they
//     host any lifecycle events.
//
// Section/PageNumber/Hidden are populated to match Question's contract:
// section is the immediate enclosing subform's name (mirroring
// walkSubformChildren, which skips pageAreas and which only ever exposes the
// innermost subform via sectionName), and parentHidden accumulates static
// presence from enclosing subforms so a button in a hidden subform reports
// Hidden=true.
//
// The ID is the node's dot-joined ancestor path, dereferenceable by orphan
// FormScripts via OwnerPath equality.
func extractAllElements(root *xfaNode, schema *types.FormSchema) {
	var walk func(node *xfaNode, parentPath []string, section string, parentHidden bool)
	walk = func(node *xfaNode, parentPath []string, section string, parentHidden bool) {
		var nodePath []string
		if node.Name != "" && node.Name != "_root" {
			nodePath = make([]string, len(parentPath)+1)
			copy(nodePath, parentPath)
			nodePath[len(parentPath)] = node.Name
		} else {
			nodePath = parentPath
		}
		somPath := strings.Join(nodePath, ".")

		switch node.Kind {
		case xfaKindField:
			if node.Bind == "none" && node.UIType == "button" && !strings.Contains(node.Name, "AddAttachment") {
				el := types.FormElement{
					ID:         somPath,
					OwnerPath:  somPath,
					Role:       "button",
					Label:      resolveInteractiveLabel(node),
					Hidden:     parentHidden || node.Hidden,
					PageNumber: node.PageNumber,
					Section:    section,
				}
				if props := buildNodeProperties(node); len(props) > 0 {
					el.Properties = props
				}
				schema.Elements = append(schema.Elements, el)
			}
		case xfaKindDraw:
			if len(node.Events) > 0 {
				el := types.FormElement{
					ID:         somPath,
					OwnerPath:  somPath,
					Role:       "draw",
					Label:      resolveDrawText(node),
					Hidden:     parentHidden || node.Hidden,
					PageNumber: node.PageNumber,
					Section:    section,
				}
				props := buildNodeProperties(node)
				// Mirror emitDraw's image plumbing: event-bearing draws skip
				// emitDraw entirely (it short-circuits on len(Events) > 0),
				// so we attach image data here to keep dynamic stamps/signatures
				// from losing their bitmap.
				if node.ImageData != "" || node.ImageHRef != "" {
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
				}
				if len(props) > 0 {
					el.Properties = props
				}
				schema.Elements = append(schema.Elements, el)
			}
		case xfaKindPageArea:
			label := strings.TrimSpace(node.BookmarkName)
			if label == "" {
				label = strings.TrimSpace(node.Caption)
			}
			el := types.FormElement{
				ID:        somPath,
				OwnerPath: somPath,
				Role:      "pageArea",
				Label:     label,
				Hidden:    parentHidden || node.Hidden,
				// PageNumber and Section intentionally omitted: pageAreas are
				// page templates, not page-bound nodes, and walkSubformChildren
				// does not treat them as sections.
			}
			if props := buildNodeProperties(node); len(props) > 0 {
				el.Properties = props
			}
			schema.Elements = append(schema.Elements, el)
		}

		// Only subforms contribute to the section name and hidden accumulation,
		// matching walkSubformChildren / buildSection. Anonymous subforms shadow
		// with an empty string so descendant Section values stay in parity with
		// Question (buildSection writes node.Name into the path slot
		// unconditionally).
		childSection := section
		childHidden := parentHidden
		if node.Kind == xfaKindSubform {
			childSection = node.Name
			childHidden = parentHidden || node.Hidden
		}
		for _, child := range node.Children {
			walk(child, nodePath, childSection, childHidden)
		}
	}
	walk(root, nil, "", false)
}

// populateScriptBackRefs indexes schema.Scripts by OwnerPath and fills in the
// Question.Scripts / FormSection.Scripts / FormElement.Scripts back-references,
// plus the script's OwnerID, whenever its owner happens to be an emitted
// Question, Section, or Element. Orphan scripts keep an empty OwnerID and rely
// solely on OwnerPath.
func populateScriptBackRefs(schema *types.FormSchema) {
	if len(schema.Scripts) == 0 {
		return
	}
	byPath := make(map[string][]int)
	for i, s := range schema.Scripts {
		if s.OwnerPath == "" {
			continue
		}
		byPath[s.OwnerPath] = append(byPath[s.OwnerPath], i)
	}
	if len(byPath) == 0 {
		return
	}

	assign := func(ownerPath, ownerID string) []string {
		idxs, ok := byPath[ownerPath]
		if !ok {
			return nil
		}
		ids := make([]string, len(idxs))
		for i, idx := range idxs {
			schema.Scripts[idx].OwnerID = ownerID
			ids[i] = schema.Scripts[idx].ID
		}
		return ids
	}

	// First, assign section-owned scripts and record each question's containing
	// section path so the question pass can compute its full SOM path.
	qSectionPath := make(map[string]string, len(schema.Questions))
	var walkSections func([]types.FormSection)
	walkSections = func(secs []types.FormSection) {
		for i := range secs {
			sec := &secs[i]
			sec.Scripts = assign(sec.Path, sec.Path)
			for _, qID := range sec.Questions {
				qSectionPath[qID] = sec.Path
			}
			walkSections(sec.Children)
		}
	}
	walkSections(schema.Sections)

	// Single pass over questions: full SOM path = sectionPath + "." + SOMSegment,
	// or just SOMSegment for questions not enclosed in any section. SOMSegment
	// includes "[i]" disambiguation when same-named siblings exist; falling back
	// to Name preserves correlation for entries populated before XFA SOM tracking
	// (today: AcroForm questions, which never have scripts anyway).
	for i := range schema.Questions {
		q := &schema.Questions[i]
		seg := q.SOMSegment
		if seg == "" {
			seg = q.Name
		}
		somPath := seg
		if sp := qSectionPath[q.ID]; sp != "" {
			somPath = sp + "." + seg
		}
		q.Scripts = assign(somPath, q.ID)
	}

	// Assign element-owned scripts. ID == OwnerPath so the lookup is direct.
	for i := range schema.Elements {
		el := &schema.Elements[i]
		el.Scripts = assign(el.OwnerPath, el.ID)
	}
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
// node is the parent subform; its Children are walked here. SOM segments for
// each emitted question/section are derived via somSegment(node, child) so
// same-named siblings are disambiguated with "[i]".
func walkSubformChildren(node *xfaNode, path []string, schema *types.FormSchema, qIdx *int, parentHidden bool, verbose bool) []types.FormSection {
	var sections []types.FormSection
	for _, child := range node.Children {
		seg := somSegment(node, child)
		switch child.Kind {
		case xfaKindPageArea:
			// Page templates — isolated from main form content.

		case xfaKindSubform:
			sec := buildSection(child, node, path, schema, qIdx, parentHidden, verbose)
			sections = append(sections, sec)

		case xfaKindExclGroup:
			q := emitExclGroup(child, path, seg, qIdx)
			schema.Questions = append(schema.Questions, q)

		case xfaKindField:
			if q, ok := emitField(child, path, seg, qIdx, verbose); ok {
				schema.Questions = append(schema.Questions, q)
			}

		case xfaKindDraw:
			if q, ok := emitDraw(child, path, node, seg, qIdx, parentHidden); ok {
				schema.Questions = append(schema.Questions, q)
			}
		}
	}
	return sections
}

// buildSection creates a FormSection for a subform node and recurses into its children.
// parent is the subform that owns node, used to disambiguate same-named siblings.
func buildSection(node, parent *xfaNode, parentPath []string, schema *types.FormSchema, qIdx *int, parentHidden bool, verbose bool) types.FormSection {
	path := somAppend(parentPath, somSegment(parent, node))
	sectionPath := somJoin(path)

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
		Path:        sectionPath,
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
// somSeg is the node's SOM-formatted name segment (with "[i]" when needed),
// stored on the Question so script back-refs can stitch a unique SOM path.
func emitExclGroup(node *xfaNode, path []string, somSeg string, qIdx *int) types.Question {
	*qIdx++
	q := types.Question{
		ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
		Name:       node.Name,
		SOMSegment: somSeg,
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
// somSeg is the node's SOM-formatted name segment (with "[i]" when needed).
func emitField(node *xfaNode, path []string, somSeg string, qIdx *int, verbose bool) (types.Question, bool) {
	if node.Bind == "none" {
		// Non-data-bound field — UI trigger, display label, or file upload.
		if node.UIType == "button" {
			// AddAttachment buttons are bind="none" because XFA file uploads
			// don't bind to the dataset (attachments go into the XFA package's
			// own attachment stream), but they are still user input — render as
			// a file question. Other bind="none" buttons (Help Text, Show Intro,
			// etc.) are UI triggers and surface as FormElements instead.
			if strings.Contains(node.Name, "AddAttachment") {
				*qIdx++
				label := node.Caption
				if label == "" {
					label = "Add Attachment"
				}
				return types.Question{
					ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
					Name:       node.Name,
					SOMSegment: somSeg,
					Label:      label,
					Type:       types.ResponseTypeFile,
					Section:    sectionName(path),
					PageNumber: node.PageNumber,
					Hidden:     node.Hidden,
				}, true
			}
			return types.Question{}, false
		}
		displayText := resolveDrawText(node)
		if displayText == "" {
			return types.Question{}, false
		}
		*qIdx++
		return types.Question{
			ID:         sanitizeFieldIDWithIndex(node.Name, *qIdx),
			Name:       node.Name,
			SOMSegment: somSeg,
			Label:      displayText,
			Type:       types.ResponseTypeDisplay,
			ReadOnly:   true,
			Section:    sectionName(path),
			PageNumber: node.PageNumber,
		}, true
	}
	// Data-bound interactive field.
	*qIdx++
	q := convertNodeToQuestion(node, *qIdx, sectionName(path), verbose)
	q.SOMSegment = somSeg
	return q, true
}

// emitDraw emits a <draw> node as a Question. Returns (question, true) if the
// draw should be rendered, (zero, false) if it should be suppressed.
// parent is the subform node that owns this draw. somSeg is the node's
// SOM-formatted name segment (with "[i]" when needed).
func emitDraw(node *xfaNode, path []string, parent *xfaNode, somSeg string, qIdx *int, parentHidden bool) (types.Question, bool) {
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
			SOMSegment: somSeg,
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
			SOMSegment: somSeg,
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
		SOMSegment: somSeg,
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

// contentTypeToLang maps a <script contentType="..."> value to "formcalc" or "javascript".
// Returns "" when the contentType is unknown — callers default to "formcalc" per the XFA spec.
func contentTypeToLang(ct string) string {
	switch strings.ToLower(ct) {
	case "application/x-formcalc":
		return "formcalc"
	case "application/x-javascript", "text/javascript":
		return "javascript"
	}
	return ""
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

// putAttr lazily initialises m and writes value under name. Used to capture
// unknown <event>/<script> attributes into FormScript.Properties.
func putAttr(m *map[string]interface{}, name, value string) {
	if *m == nil {
		*m = make(map[string]interface{})
	}
	(*m)[name] = value
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
