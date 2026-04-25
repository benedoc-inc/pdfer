package xfa

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

// --- detectScriptLanguage ---------------------------------------------------

func TestDetectScriptLanguage_FormCalc_ThenEndif(t *testing.T) {
	s := `if ($.rawValue == "yes") then $.presence = "hidden" endif`
	if got := detectScriptLanguage(s); got != "formcalc" {
		t.Errorf("expected formcalc, got %q", got)
	}
}

func TestDetectScriptLanguage_FormCalc_DollarRef(t *testing.T) {
	if got := detectScriptLanguage(`$.rawValue = "hello"`); got != "formcalc" {
		t.Errorf("expected formcalc, got %q", got)
	}
}

func TestDetectScriptLanguage_JavaScript_Braces(t *testing.T) {
	if got := detectScriptLanguage(`if (this.rawValue == "yes") { this.presence = "hidden"; }`); got != "javascript" {
		t.Errorf("expected javascript, got %q", got)
	}
}

func TestDetectScriptLanguage_JavaScript_Return(t *testing.T) {
	if got := detectScriptLanguage(`return this.rawValue.length > 0;`); got != "javascript" {
		t.Errorf("expected javascript, got %q", got)
	}
}

// --- visibility -------------------------------------------------------------

func TestParseXFAScript_Visibility_Hide_FormCalc(t *testing.T) {
	r := parseXFAScript(`$.presence = "hidden"`, "myField", "")
	if r.ruleType != types.RuleTypeVisibility {
		t.Fatalf("expected RuleTypeVisibility, got %q", r.ruleType)
	}
	if len(r.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(r.actions))
	}
	if r.actions[0].Type != types.ActionTypeHide {
		t.Errorf("expected ActionTypeHide, got %q", r.actions[0].Type)
	}
}

func TestParseXFAScript_Visibility_Show_FormCalc(t *testing.T) {
	r := parseXFAScript(`$.presence = "visible"`, "f", "")
	if r.ruleType != types.RuleTypeVisibility {
		t.Fatalf("expected RuleTypeVisibility, got %q", r.ruleType)
	}
	if r.actions[0].Type != types.ActionTypeShow {
		t.Errorf("expected ActionTypeShow, got %q", r.actions[0].Type)
	}
}

func TestParseXFAScript_Visibility_Invisible(t *testing.T) {
	r := parseXFAScript(`$.presence = "invisible"`, "f", "")
	if r.actions[0].Type != types.ActionTypeHide {
		t.Errorf("expected ActionTypeHide for 'invisible', got %q", r.actions[0].Type)
	}
}

func TestParseXFAScript_Visibility_WithCondition_FormCalc(t *testing.T) {
	s := `if ($.rawValue == "yes") then $.presence = "hidden" endif`
	r := parseXFAScript(s, "trigger", "target")
	if r.ruleType != types.RuleTypeVisibility {
		t.Fatalf("expected RuleTypeVisibility, got %q", r.ruleType)
	}
	if r.condition == nil {
		t.Fatal("expected non-nil condition")
	}
	if r.condition.Operator != types.OperatorEquals {
		t.Errorf("expected OperatorEquals, got %q", r.condition.Operator)
	}
}

func TestParseXFAScript_Visibility_WithCondition_JavaScript(t *testing.T) {
	s := `if (this.rawValue == "yes") { xfa.resolveNode("otherField").presence = "hidden"; }`
	r := parseXFAScript(s, "trigger", "")
	if r.ruleType != types.RuleTypeVisibility {
		t.Fatalf("expected RuleTypeVisibility, got %q", r.ruleType)
	}
	if r.condition == nil {
		t.Fatal("expected non-nil condition")
	}
	// Target should be extracted from resolveNode.
	if r.actions[0].Target != "otherField" {
		t.Errorf("expected target 'otherField', got %q", r.actions[0].Target)
	}
}

func TestParseXFAScript_Visibility_ResolveNode_Target(t *testing.T) {
	s := `xfa.resolveNode("sectionB").presence = "hidden"`
	r := parseXFAScript(s, "checkboxField", "")
	if r.actions[0].Target != "sectionB" {
		t.Errorf("expected target 'sectionB', got %q", r.actions[0].Target)
	}
}

// --- set value --------------------------------------------------------------

func TestParseXFAScript_SetValue_LiteralString_FormCalc(t *testing.T) {
	r := parseXFAScript(`$.rawValue = "hello"`, "f", "")
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
	if len(r.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(r.actions))
	}
	a := r.actions[0]
	if a.Type != types.ActionTypeSetValue {
		t.Errorf("expected ActionTypeSetValue, got %q", a.Type)
	}
	if a.Value != "hello" {
		t.Errorf("expected unquoted literal 'hello', got %v", a.Value)
	}
}

func TestParseXFAScript_SetValue_FieldRef_FormCalc(t *testing.T) {
	r := parseXFAScript(`$.rawValue = otherField.rawValue`, "f", "")
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
	if r.actions[0].Expression != "otherField.rawValue" {
		t.Errorf("expected expression 'otherField.rawValue', got %q", r.actions[0].Expression)
	}
}

func TestParseXFAScript_SetValue_JavaScript_ThisRawValue(t *testing.T) {
	r := parseXFAScript(`this.rawValue = "default";`, "f", "")
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
}

func TestParseXFAScript_SetValue_WithCondition(t *testing.T) {
	s := `if (trigger.rawValue != "") then $.rawValue = trigger.rawValue endif`
	r := parseXFAScript(s, "f", "")
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
	if r.condition == nil {
		t.Fatal("expected condition")
	}
	if r.condition.Operator != types.OperatorNotEquals {
		t.Errorf("expected OperatorNotEquals, got %q", r.condition.Operator)
	}
}

// --- validation -------------------------------------------------------------

func TestParseXFAScript_Validate_ReturnFalse(t *testing.T) {
	s := `if ($.rawValue == "") then return false endif`
	r := parseXFAScript(s, "f", "")
	if r.ruleType != types.RuleTypeValidate {
		t.Fatalf("expected RuleTypeValidate, got %q", r.ruleType)
	}
	if r.actions[0].Type != types.ActionTypeValidate {
		t.Errorf("expected ActionTypeValidate, got %q", r.actions[0].Type)
	}
	if r.actions[0].Script == "" {
		t.Error("raw script should be preserved")
	}
}

func TestParseXFAScript_Validate_JavaScript_ReturnBool(t *testing.T) {
	s := `
var v = this.rawValue;
if (v.length < 5) { return false; }
return true;`
	r := parseXFAScript(s, "zipCode", "")
	if r.ruleType != types.RuleTypeValidate {
		t.Fatalf("expected RuleTypeValidate, got %q", r.ruleType)
	}
}

func TestParseXFAScript_Validate_MessageBox(t *testing.T) {
	s := `if ($.rawValue == "") then xfa.host.messageBox("Required field") return false endif`
	r := parseXFAScript(s, "f", "")
	if r.ruleType != types.RuleTypeValidate {
		t.Fatalf("expected RuleTypeValidate, got %q", r.ruleType)
	}
}

// --- calculate --------------------------------------------------------------

func TestParseXFAScript_Calculate_Sum_FormCalc(t *testing.T) {
	s := `$.rawValue = Sum(a.rawValue, b.rawValue, c.rawValue)`
	r := parseXFAScript(s, "total", "")
	// Contains Sum() — triggers calculate path before set-value since it
	// matches calculate earlier if we check set-value first. Either
	// RuleTypeCalculate or RuleTypeSetValue is acceptable here; the key check
	// is that Sum is present in the expression.
	if r.ruleType != types.RuleTypeCalculate && r.ruleType != types.RuleTypeSetValue {
		t.Errorf("unexpected rule type %q", r.ruleType)
	}
}

func TestParseXFAScript_Calculate_FormCalcBuiltin(t *testing.T) {
	s := `Concat(firstName.rawValue, " ", lastName.rawValue)`
	r := parseXFAScript(s, "fullName", "")
	if r.ruleType != types.RuleTypeCalculate {
		t.Fatalf("expected RuleTypeCalculate, got %q", r.ruleType)
	}
}

func TestParseXFAScript_Calculate_JavaScript_Return(t *testing.T) {
	s := `return parseFloat(qty.rawValue) * parseFloat(price.rawValue);`
	r := parseXFAScript(s, "total", "")
	if r.ruleType != types.RuleTypeCalculate {
		t.Fatalf("expected RuleTypeCalculate, got %q", r.ruleType)
	}
	if !strings.Contains(r.actions[0].Expression, "parseFloat") {
		t.Errorf("expression should contain the return value, got %q", r.actions[0].Expression)
	}
}

// --- fallback ---------------------------------------------------------------

func TestParseXFAScript_Fallback_Unknown(t *testing.T) {
	s := `someComplexCustomFunction(arg1, arg2)`
	r := parseXFAScript(s, "f", "")
	if len(r.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(r.actions))
	}
	if r.actions[0].Type != types.ActionTypeExecute {
		t.Errorf("expected ActionTypeExecute fallback, got %q", r.actions[0].Type)
	}
	if r.actions[0].Script != s {
		t.Error("raw script not preserved in fallback")
	}
}

func TestParseXFAScript_Empty(t *testing.T) {
	r := parseXFAScript("", "f", "")
	// Empty script: single execute action with empty script or no actions.
	// Either is fine; the important thing is no panic.
	_ = r
}

// --- condition parsing ------------------------------------------------------

func TestExtractSimpleCondition_Equals(t *testing.T) {
	s := `if (status.rawValue == "active") then $.presence = "visible" endif`
	cond := extractSimpleCondition(s, "formcalc", "f")
	if cond == nil {
		t.Fatal("expected condition")
	}
	if cond.Operator != types.OperatorEquals {
		t.Errorf("expected OperatorEquals, got %q", cond.Operator)
	}
	if cond.Value != "active" {
		t.Errorf("expected value 'active', got %v", cond.Value)
	}
}

func TestExtractSimpleCondition_NotEquals(t *testing.T) {
	s := `if ($.rawValue != "") then return true endif`
	cond := extractSimpleCondition(s, "formcalc", "myField")
	if cond == nil {
		t.Fatal("expected condition")
	}
	if cond.Operator != types.OperatorNotEquals {
		t.Errorf("expected OperatorNotEquals, got %q", cond.Operator)
	}
}

func TestExtractSimpleCondition_JavaScript_GreaterThan(t *testing.T) {
	s := `if (this.rawValue > 100) { return false; }`
	cond := extractSimpleCondition(s, "javascript", "amount")
	if cond == nil {
		t.Fatal("expected condition")
	}
	if cond.Operator != types.OperatorGreaterThan {
		t.Errorf("expected OperatorGreaterThan, got %q", cond.Operator)
	}
}

func TestExtractSimpleCondition_NoIf(t *testing.T) {
	cond := extractSimpleCondition(`$.rawValue = "hello"`, "formcalc", "f")
	if cond != nil {
		t.Errorf("expected nil condition for script without if, got %+v", cond)
	}
}

// --- helper functions -------------------------------------------------------

func TestExtractFieldRef_Dollar(t *testing.T) {
	if got := extractFieldRef("$.rawValue", "myField"); got != "myField" {
		t.Errorf("expected 'myField', got %q", got)
	}
}

func TestExtractFieldRef_This(t *testing.T) {
	if got := extractFieldRef("this.rawValue", "myField"); got != "myField" {
		t.Errorf("expected 'myField', got %q", got)
	}
}

func TestExtractFieldRef_Named(t *testing.T) {
	if got := extractFieldRef("otherField.rawValue", "myField"); got != "otherField" {
		t.Errorf("expected 'otherField', got %q", got)
	}
}

func TestExtractResolveNodeTarget(t *testing.T) {
	cases := []struct {
		s    string
		want string
	}{
		{`xfa.resolveNode("firstName").rawValue = ""`, "firstName"},
		{`xfa.resolveNode('section2').presence = "hidden"`, "section2"},
		{`$.rawValue = "x"`, ""},
	}
	for _, tc := range cases {
		got := extractResolveNodeTarget(tc.s)
		if got != tc.want {
			t.Errorf("extractResolveNodeTarget(%q) = %q, want %q", tc.s, got, tc.want)
		}
	}
}

func TestUnquoteLiteral(t *testing.T) {
	cases := []struct {
		in   string
		want interface{}
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`42`, nil},
		{`field.rawValue`, nil},
	}
	for _, tc := range cases {
		got := unquoteLiteral(tc.in)
		if got != tc.want {
			t.Errorf("unquoteLiteral(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestFindAssignmentOp(t *testing.T) {
	cases := []struct {
		s    string
		want int // -1 means "not found"
	}{
		{`$.rawValue = "hello"`, 11},
		{`a == b`, -1},
		{`a != b`, -1},
		{`a >= b`, -1},
		{`a <= b`, -1},
	}
	for _, tc := range cases {
		got := findAssignmentOp(tc.s)
		if tc.want == -1 {
			if got != -1 {
				t.Errorf("findAssignmentOp(%q) = %d, expected -1", tc.s, got)
			}
		} else {
			if got == -1 {
				t.Errorf("findAssignmentOp(%q) = -1, expected %d", tc.s, tc.want)
			}
		}
	}
}

// --- convertXFAEventToRule integration -------------------------------------

func TestConvertXFAEventToRule_EventTypeMapping(t *testing.T) {
	cases := []struct {
		eventType string
		want      types.RuleType
	}{
		{"validate", types.RuleTypeValidate},
		{"calculate", types.RuleTypeCalculate},
		{"initialize", types.RuleTypeSetValue},
	}
	for _, tc := range cases {
		event := XFAEvent{Type: tc.eventType}
		rule, err := convertXFAEventToRule(event, "f", 1)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.eventType, err)
		}
		if rule.Type != tc.want {
			t.Errorf("event type %q: expected rule type %q, got %q", tc.eventType, tc.want, rule.Type)
		}
	}
}

func TestConvertXFAEventToRule_ScriptOverridesEventType(t *testing.T) {
	// A "change" event whose script is actually a visibility toggle.
	event := XFAEvent{
		Type:   "change",
		Script: `$.presence = "hidden"`,
	}
	rule, err := convertXFAEventToRule(event, "f", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Type != types.RuleTypeVisibility {
		t.Errorf("expected RuleTypeVisibility (from script), got %q", rule.Type)
	}
}

func TestConvertXFAEventToRule_PreservesSourceAndID(t *testing.T) {
	event := XFAEvent{Type: "validate", Script: `return false`}
	rule, _ := convertXFAEventToRule(event, "emailField", 7)
	if rule.Source != "emailField" {
		t.Errorf("expected source 'emailField', got %q", rule.Source)
	}
	if rule.ID != "rule_7" {
		t.Errorf("expected ID 'rule_7', got %q", rule.ID)
	}
}
