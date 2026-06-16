package types

// FormSchema represents the complete form structure
// that can be used to rebuild the form with questions, responses, and scripts.
type FormSchema struct {
	Metadata  FormMetadata  `json:"metadata"`
	Questions []Question    `json:"questions"`
	Sections  []FormSection `json:"sections,omitempty"` // hierarchical section tree (XFA only)
	Elements  []FormElement `json:"elements,omitempty"` // non-question template nodes (buttons, dynamic draws, pageAreas) — XFA only
	Scripts   []FormScript  `json:"scripts,omitempty"`  // raw <script> blocks extracted verbatim; callers parse them themselves
}

// FormElement is a non-question template node surfaced alongside Questions so
// orphan FormScripts have a typed owner to dereference and downstream renderers
// can render help-text buttons, dynamic draws, and pageArea events without
// consulting the raw XFA DOM. ID equals OwnerPath — SOM paths are unique within
// a template.
type FormElement struct {
	ID         string                 `json:"id"`
	OwnerPath  string                 `json:"owner_path"`
	Role       string                 `json:"role"` // "button" | "draw" | "pageArea"
	Label      string                 `json:"label,omitempty"`
	Hidden     bool                   `json:"hidden,omitempty"`      // rolled-up static presence: hidden on this node or an ancestor subform
	Presence   string                 `json:"presence,omitempty"`    // verbatim authored presence on THIS node only: "visible"|"invisible"|"hidden"|"inactive"; "" when unset. Unlike Hidden, never inherits from ancestors
	PageNumber int                    `json:"page_number,omitempty"` // field/draw "page" attribute; 0 for pageAreas and unannotated nodes
	Section    string                 `json:"section,omitempty"`     // nearest enclosing subform name; empty for pageAreas and top-level nodes
	Properties map[string]interface{} `json:"properties,omitempty"`
	Scripts    []string               `json:"scripts,omitempty"` // FormScript IDs
	Occur      *Occur                 `json:"occur,omitempty"`   // declared cardinality from <occur>; nil when absent
	Bind       *Bind                  `json:"bind,omitempty"`    // <bind> declaration; nil when absent
}

// FormSection is a node in the XFA subform hierarchy.
// Questions contains question IDs (not full Question objects) in document order;
// the flat Questions slice on FormSchema is the canonical store.
type FormSection struct {
	Name        string        `json:"name"`
	Path        string        `json:"path"`               // SOM-style dot-path from root, e.g. "form1.section2.sub"; same-named siblings carry "[i]" suffixes (XFA only)
	Label       string        `json:"label,omitempty"`    // human-readable label from XFA subform caption; empty when not specified
	Tooltip     string        `json:"tooltip,omitempty"`  // accessibility tooltip (e.g. IMDRF TOC chapter references)
	Interactive bool          `json:"interactive"`        // true if the section contains any data-bound fields
	Hidden      bool          `json:"hidden,omitempty"`   // rolled-up static presence: hidden on this subform or an ancestor subform
	Presence    string        `json:"presence,omitempty"` // verbatim authored presence on THIS subform only: "visible"|"invisible"|"hidden"|"inactive"; "" when unset. Unlike Hidden, never inherits from ancestors
	Layout      string        `json:"layout,omitempty"`   // XFA layout mode: position|tb|lr-tb|row|table
	Width       string        `json:"width,omitempty"`    // subform w attribute (mm/in/pt)
	Height      string        `json:"height,omitempty"`   // subform h attribute (mm/in/pt)
	Content     []string      `json:"content,omitempty"`  // static display text from non-interactive sections
	Children    []FormSection `json:"children,omitempty"`
	Questions   []string      `json:"questions,omitempty"` // question IDs in document order
	Scripts     []string      `json:"scripts,omitempty"`   // FormScript IDs for subform-level events, in declaration order
	Occur       *Occur        `json:"occur,omitempty"`     // declared cardinality from <occur>; nil when absent
	Bind        *Bind         `json:"bind,omitempty"`      // subform-level <bind> declaration; nil when absent
}

// Occur reflects the template's declared cardinality, normalized to XFA spec
// defaults (min=1, max=min, initial=min; Max=-1 means unbounded). Nil when the
// template has no <occur> element. The schema lists each repeating subform once
// regardless of how many instances the dataset carries; materializing instances
// is the runtime's responsibility.
type Occur struct {
	Min     int `json:"min"`
	Max     int `json:"max"` // -1 for unbounded
	Initial int `json:"initial"`
}

// Bind reflects the template's <bind> declaration. Match defaults to "once"
// per the XFA spec when the attribute is absent; Ref is the ref attribute
// verbatim (a SOM path, normally paired with match="dataRef").
type Bind struct {
	Match string `json:"match"`
	Ref   string `json:"ref,omitempty"`
}

// FormMetadata contains information about the form
type FormMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	FormType    string `json:"form_type"` // "XFA" or "AcroForm"
	TotalPages  int    `json:"total_pages"`
}

// Question represents a single question/field in the form
type Question struct {
	ID          string                 `json:"id"`                    // Unique identifier
	Name        string                 `json:"name"`                  // Field name from PDF
	SOMSegment  string                 `json:"som_segment,omitempty"` // XFA only: SOM-formatted name segment (includes "[i]" when same-named siblings exist). Append to parent section's Path to get the full SOM path.
	Label       string                 `json:"label,omitempty"`       // Display label/question text
	Description string                 `json:"description,omitempty"` // Help text or description
	Type        ResponseType           `json:"type"`                  // Response type
	Options     []Option               `json:"options,omitempty"`     // Options for choice types
	Validation  *ValidationRules       `json:"validation,omitempty"`  // Validation rules
	Default     interface{}            `json:"default,omitempty"`     // Default value
	Required    bool                   `json:"required"`              // Is field required?
	ReadOnly    bool                   `json:"read_only"`             // Is field read-only?
	Hidden      bool                   `json:"hidden"`                // Rolled-up static presence: hidden on this node or an ancestor subform
	Presence    string                 `json:"presence,omitempty"`    // verbatim authored presence on THIS node only: "visible"|"invisible"|"hidden"|"inactive"; "" when unset. Unlike Hidden, never inherits from ancestors
	Properties  map[string]interface{} `json:"properties,omitempty"`  // Additional properties (position, size, etc.)
	PageNumber  int                    `json:"page_number,omitempty"` // Which page the field appears on
	Section     string                 `json:"section,omitempty"`     // Parent subform / section name; matches the parent FormSection.Path's last segment (may include "[i]" for XFA same-named-sibling disambiguation)
	Scripts     []string               `json:"scripts,omitempty"`     // FormScript IDs for events on this field, in declaration order
	Occur       *Occur                 `json:"occur,omitempty"`       // declared cardinality from <occur>; nil when absent (XFA only)
	Bind        *Bind                  `json:"bind,omitempty"`        // <bind> declaration; nil when absent (XFA only)
}

// ResponseType represents the type of response expected
type ResponseType string

const (
	ResponseTypeText      ResponseType = "text"      // Single-line text input
	ResponseTypeTextarea  ResponseType = "textarea"  // Multi-line text input
	ResponseTypeRadio     ResponseType = "radio"     // Radio button group
	ResponseTypeCheckbox  ResponseType = "checkbox"  // Checkbox
	ResponseTypeSelect    ResponseType = "select"    // Dropdown/select
	ResponseTypeNumber    ResponseType = "number"    // Numeric input
	ResponseTypeDate      ResponseType = "date"      // Date picker
	ResponseTypeEmail     ResponseType = "email"     // Email input
	ResponseTypeButton    ResponseType = "button"    // Button (not a question, but action)
	ResponseTypePassword  ResponseType = "password"  // Password input
	ResponseTypeTime      ResponseType = "time"      // Time picker (XFA dateTimeEdit with TIME{} picture)
	ResponseTypeSignature ResponseType = "signature" // Signature field
	ResponseTypeDisplay   ResponseType = "display"   // Static display text (XFA draw / no-bind label)
	ResponseTypeImage     ResponseType = "image"     // Embedded image (XFA draw with <image> content)
	ResponseTypeSeparator ResponseType = "separator" // Horizontal/vertical rule (XFA draw with <line> value)
	ResponseTypeFile      ResponseType = "file"      // File attachment upload
	ResponseTypeUnknown   ResponseType = "unknown"   // Unknown type
)

// Option represents a choice option for radio, checkbox, or select types
type Option struct {
	Value       string `json:"value"`                 // Option value
	Label       string `json:"label"`                 // Display label
	Description string `json:"description,omitempty"` // Optional description
	Selected    bool   `json:"selected,omitempty"`    // Is this option selected by default?
}

// ValidationRules defines validation constraints for a question
type ValidationRules struct {
	MinLength    *int     `json:"min_length,omitempty"`
	MaxLength    *int     `json:"max_length,omitempty"`
	MinValue     *float64 `json:"min_value,omitempty"`
	MaxValue     *float64 `json:"max_value,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`       // Regex pattern
	CustomScript string   `json:"custom_script,omitempty"` // Custom validation script
	ErrorMessage string   `json:"error_message,omitempty"` // Error message to display
}

// FormScript represents a raw script block extracted from an XFA form.
// Bodies are exposed verbatim — pdfer does not interpret script semantics.
//
// OwnerID is non-empty iff the owning node is surfaced as a typed schema
// entity (Question, FormSection, or FormElement) that callers can dereference
// by ID. Scripts whose owner is not currently typed in the schema appear with
// OwnerPath set and OwnerID empty — at time of writing, the remaining orphan
// case is individual radio options collapsed into an <exclGroup> (the
// exclGroup itself is the typed owner). Callers should treat OwnerID empty as
// "not currently dereferenceable" rather than a permanent classification, and
// rely on OwnerPath when they need owner-keyed addressing in either case.
type FormScript struct {
	ID         string                 `json:"id"`                   // stable: SOM owner path + "#" + event + "[" + index + "]"
	OwnerPath  string                 `json:"owner_path,omitempty"` // SOM path of containing node (e.g. "form1.section.field"); empty for template-level
	OwnerID    string                 `json:"owner_id,omitempty"`   // matches Question.ID or FormSection.Path when the owner is a question or section
	Event      string                 `json:"event"`                // XFA activity: initialize|calculate|validate|change|exit|click|… ; "variables" for <variables><script> blocks
	Name       string                 `json:"name,omitempty"`       // <event name="..."> attribute, or <script name="..."> for variables scripts
	Language   string                 `json:"language"`             // "javascript" | "formcalc"; defaults to "formcalc" per XFA spec when contentType is absent
	RunAt      string                 `json:"run_at,omitempty"`     // client | server | both
	Body       string                 `json:"body"`                 // verbatim script source
	Properties map[string]interface{} `json:"properties,omitempty"` // unknown <event>/<script> attributes (listen, ref, id, binding, stateless, url, …); event and script attrs share this map, so a key set on both is last-write-wins
}
