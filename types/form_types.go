package types

// FormSchema represents the complete form structure
// that can be used to rebuild the form with questions, responses, and control flow
type FormSchema struct {
	Metadata  FormMetadata `json:"metadata"`
	Questions []Question   `json:"questions"`
	Rules     []Rule       `json:"rules"` // Control flow rules (dependencies, conditions)
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
	Label       string                 `json:"label,omitempty"`       // Display label/question text
	Description string                 `json:"description,omitempty"` // Help text or description
	Type        ResponseType           `json:"type"`                  // Response type
	Options     []Option               `json:"options,omitempty"`     // Options for choice types
	Validation  *ValidationRules       `json:"validation,omitempty"`  // Validation rules
	Default     interface{}            `json:"default,omitempty"`     // Default value
	Required    bool                   `json:"required"`              // Is field required?
	ReadOnly    bool                   `json:"read_only"`             // Is field read-only?
	Hidden      bool                   `json:"hidden"`                // Is field initially hidden?
	Properties  map[string]interface{} `json:"properties,omitempty"`  // Additional properties (position, size, etc.)
	PageNumber  int                    `json:"page_number,omitempty"` // Which page the field appears on
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
	ResponseTypeSignature ResponseType = "signature" // Signature field
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

// Rule represents a control flow rule (dependency/condition)
type Rule struct {
	ID          string     `json:"id"`                    // Unique rule identifier
	Type        RuleType   `json:"type"`                  // Type of rule
	Source      string     `json:"source"`                // Source question ID
	Condition   *Condition `json:"condition,omitempty"`   // Condition to evaluate
	Actions     []Action   `json:"actions"`               // Actions to perform when condition is met
	Priority    int        `json:"priority,omitempty"`    // Rule priority (for ordering)
	Description string     `json:"description,omitempty"` // Human-readable description
}

// RuleType represents the type of control flow rule
type RuleType string

const (
	RuleTypeVisibility RuleType = "visibility" // Show/hide fields
	RuleTypeEnable     RuleType = "enable"     // Enable/disable fields
	RuleTypeCalculate  RuleType = "calculate"  // Calculate field value
	RuleTypeValidate   RuleType = "validate"   // Custom validation
	RuleTypeSetValue   RuleType = "set_value"  // Set field value
	RuleTypeNavigate   RuleType = "navigate"   // Navigate to page/section
)

// Condition represents a condition to evaluate
type Condition struct {
	Operator   Operator      `json:"operator"`             // Comparison operator
	Value      interface{}   `json:"value,omitempty"`      // Value to compare against
	Values     []interface{} `json:"values,omitempty"`     // Multiple values (for IN operator)
	Expression string        `json:"expression,omitempty"` // Custom expression/script
	Logic      LogicOp       `json:"logic,omitempty"`      // Logic operator for compound conditions
	Children   []Condition   `json:"children,omitempty"`   // Nested conditions
}

// Operator represents comparison operators
type Operator string

const (
	OperatorEquals         Operator = "equals"           // ==
	OperatorNotEquals      Operator = "not_equals"       // !=
	OperatorGreaterThan    Operator = "greater_than"     // >
	OperatorLessThan       Operator = "less_than"        // <
	OperatorGreaterOrEqual Operator = "greater_or_equal" // >=
	OperatorLessOrEqual    Operator = "less_or_equal"    // <=
	OperatorContains       Operator = "contains"         // String contains
	OperatorNotContains    Operator = "not_contains"     // String does not contain
	OperatorIn             Operator = "in"               // Value in array
	OperatorNotIn          Operator = "not_in"           // Value not in array
	OperatorIsEmpty        Operator = "is_empty"         // Field is empty
	OperatorIsNotEmpty     Operator = "is_not_empty"     // Field is not empty
	OperatorMatches        Operator = "matches"          // Regex match
)

// LogicOp represents logical operators for combining conditions
type LogicOp string

const (
	LogicOpAnd LogicOp = "and"
	LogicOpOr  LogicOp = "or"
	LogicOpNot LogicOp = "not"
)

// Action represents an action to perform when a condition is met
type Action struct {
	Type        ActionType  `json:"type"`                  // Type of action
	Target      string      `json:"target"`                // Target question ID
	Value       interface{} `json:"value,omitempty"`       // Value to set (for set_value)
	Expression  string      `json:"expression,omitempty"`  // Expression to evaluate (for calculate)
	Script      string      `json:"script,omitempty"`      // Custom script to execute
	Description string      `json:"description,omitempty"` // Human-readable description
}

// ActionType represents the type of action
type ActionType string

const (
	ActionTypeShow      ActionType = "show"      // Show field
	ActionTypeHide      ActionType = "hide"      // Hide field
	ActionTypeEnable    ActionType = "enable"    // Enable field
	ActionTypeDisable   ActionType = "disable"   // Disable field
	ActionTypeSetValue  ActionType = "set_value" // Set field value
	ActionTypeCalculate ActionType = "calculate" // Calculate field value
	ActionTypeValidate  ActionType = "validate"  // Trigger validation
	ActionTypeNavigate  ActionType = "navigate"  // Navigate to page/section
	ActionTypeExecute   ActionType = "execute"   // Execute custom script
)
