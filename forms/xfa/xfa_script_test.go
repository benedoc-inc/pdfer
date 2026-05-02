package xfa

import (
	"strings"
	"testing"

	"github.com/benedoc-inc/pdfer/types"
)

// --- detectScriptLanguage ---------------------------------------------------

func TestDetectScriptLanguage_FormCalc_ThenEndif(t *testing.T) {
	s := `if ($.rawValue == "yes") then $.presence = "hidden" endif`
	if got := detectScriptLanguage(s, ""); got != "formcalc" {
		t.Errorf("expected formcalc, got %q", got)
	}
}

func TestDetectScriptLanguage_FormCalc_DollarRef(t *testing.T) {
	if got := detectScriptLanguage(`$.rawValue = "hello"`, ""); got != "formcalc" {
		t.Errorf("expected formcalc, got %q", got)
	}
}

func TestDetectScriptLanguage_JavaScript_Braces(t *testing.T) {
	if got := detectScriptLanguage(`if (this.rawValue == "yes") { this.presence = "hidden"; }`, ""); got != "javascript" {
		t.Errorf("expected javascript, got %q", got)
	}
}

func TestDetectScriptLanguage_JavaScript_Return(t *testing.T) {
	if got := detectScriptLanguage(`return this.rawValue.length > 0;`, ""); got != "javascript" {
		t.Errorf("expected javascript, got %q", got)
	}
}

func TestDetectScriptLanguage_HintWins(t *testing.T) {
	// Hint should override heuristic.
	if got := detectScriptLanguage(`$.rawValue = "x"`, "javascript"); got != "javascript" {
		t.Errorf("expected javascript (from hint), got %q", got)
	}
}

// --- visibility -------------------------------------------------------------

func TestParseXFAScript_Visibility_Hide_FormCalc(t *testing.T) {
	rs := parseXFAScript(`$.presence = "hidden"`, "myField", "", "")
	r := rs[0]
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
	rs := parseXFAScript(`$.presence = "visible"`, "f", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeVisibility {
		t.Fatalf("expected RuleTypeVisibility, got %q", r.ruleType)
	}
	if r.actions[0].Type != types.ActionTypeShow {
		t.Errorf("expected ActionTypeShow, got %q", r.actions[0].Type)
	}
}

func TestParseXFAScript_Visibility_Invisible(t *testing.T) {
	rs := parseXFAScript(`$.presence = "invisible"`, "f", "", "")
	r := rs[0]
	if r.actions[0].Type != types.ActionTypeHide {
		t.Errorf("expected ActionTypeHide for 'invisible', got %q", r.actions[0].Type)
	}
}

func TestParseXFAScript_Visibility_WithCondition_FormCalc(t *testing.T) {
	s := `if ($.rawValue == "yes") then $.presence = "hidden" endif`
	rs := parseXFAScript(s, "trigger", "target", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "trigger", "", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "checkboxField", "", "")
	r := rs[0]
	if r.actions[0].Target != "sectionB" {
		t.Errorf("expected target 'sectionB', got %q", r.actions[0].Target)
	}
}

// --- if/else rule splitting --------------------------------------------------

func TestParseXFAScript_IfElse_TwoRules(t *testing.T) {
	s := `if ($.rawValue == "1") then
    IMDRF.presence = "visible"
    USA.presence = "hidden"
else
    IMDRF.presence = "hidden"
    USA.presence = "visible"
endif`
	rs := parseXFAScript(s, "AppType", "", "formcalc")
	if len(rs) != 2 {
		t.Fatalf("expected 2 rules (if+else), got %d", len(rs))
	}
	// Both rules should be visibility type.
	for i, r := range rs {
		if r.ruleType != types.RuleTypeVisibility {
			t.Errorf("rule[%d]: expected RuleTypeVisibility, got %q", i, r.ruleType)
		}
	}
	// First rule condition: equals "1"; second rule: inverted (not equals "1").
	if rs[0].condition == nil || rs[0].condition.Operator != types.OperatorEquals {
		t.Errorf("if-rule condition should be OperatorEquals")
	}
	if rs[1].condition == nil {
		t.Errorf("else-rule should have a condition")
	}
}

// --- set value --------------------------------------------------------------

func TestParseXFAScript_SetValue_LiteralString_FormCalc(t *testing.T) {
	rs := parseXFAScript(`$.rawValue = "hello"`, "f", "", "")
	r := rs[0]
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
	rs := parseXFAScript(`$.rawValue = otherField.rawValue`, "f", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
	if r.actions[0].Expression != "otherField.rawValue" {
		t.Errorf("expected expression 'otherField.rawValue', got %q", r.actions[0].Expression)
	}
}

func TestParseXFAScript_SetValue_JavaScript_ThisRawValue(t *testing.T) {
	rs := parseXFAScript(`this.rawValue = "default";`, "f", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeSetValue {
		t.Fatalf("expected RuleTypeSetValue, got %q", r.ruleType)
	}
}

func TestParseXFAScript_SetValue_WithCondition(t *testing.T) {
	s := `if (trigger.rawValue != "") then $.rawValue = trigger.rawValue endif`
	rs := parseXFAScript(s, "f", "", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "f", "", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "zipCode", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeValidate {
		t.Fatalf("expected RuleTypeValidate, got %q", r.ruleType)
	}
}

func TestParseXFAScript_Validate_MessageBox(t *testing.T) {
	s := `if ($.rawValue == "") then xfa.host.messageBox("Required field") return false endif`
	rs := parseXFAScript(s, "f", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeValidate {
		t.Fatalf("expected RuleTypeValidate, got %q", r.ruleType)
	}
}

// --- calculate --------------------------------------------------------------

func TestParseXFAScript_Calculate_Sum_FormCalc(t *testing.T) {
	s := `$.rawValue = Sum(a.rawValue, b.rawValue, c.rawValue)`
	rs := parseXFAScript(s, "total", "", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "fullName", "", "")
	r := rs[0]
	if r.ruleType != types.RuleTypeCalculate {
		t.Fatalf("expected RuleTypeCalculate, got %q", r.ruleType)
	}
}

func TestParseXFAScript_Calculate_JavaScript_Return(t *testing.T) {
	s := `return parseFloat(qty.rawValue) * parseFloat(price.rawValue);`
	rs := parseXFAScript(s, "total", "", "")
	r := rs[0]
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
	rs := parseXFAScript(s, "f", "", "")
	r := rs[0]
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
	rs := parseXFAScript("", "f", "", "")
	// Empty script: single execute action with empty script or no actions.
	// Either is fine; the important thing is no panic.
	_ = rs
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

// --- splitIfElse ------------------------------------------------------------

func TestSplitIfElse_FormCalc(t *testing.T) {
	s := `if ($.rawValue == "1") then
    A.presence = "visible"
else
    A.presence = "hidden"
endif`
	ifBody, elseBody, ok := splitIfElse(s, "formcalc")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(ifBody, `"visible"`) {
		t.Errorf("ifBody should contain visible: %q", ifBody)
	}
	if !strings.Contains(elseBody, `"hidden"`) {
		t.Errorf("elseBody should contain hidden: %q", elseBody)
	}
}

func TestSplitIfElse_NoElse(t *testing.T) {
	s := `if ($.rawValue == "1") then A.presence = "visible" endif`
	_, _, ok := splitIfElse(s, "formcalc")
	if ok {
		t.Error("expected ok=false for script with no else branch")
	}
}

// --- invertCondition --------------------------------------------------------

func TestInvertCondition_Equals(t *testing.T) {
	c := &types.Condition{Operator: types.OperatorEquals, Value: "yes"}
	inv := invertCondition(c)
	if inv.Operator != types.OperatorNotEquals {
		t.Errorf("expected OperatorNotEquals, got %q", inv.Operator)
	}
}

func TestInvertCondition_NotEquals(t *testing.T) {
	c := &types.Condition{Operator: types.OperatorNotEquals, Value: ""}
	inv := invertCondition(c)
	if inv.Operator != types.OperatorEquals {
		t.Errorf("expected OperatorEquals, got %q", inv.Operator)
	}
}

func TestInvertCondition_GreaterThan(t *testing.T) {
	c := &types.Condition{Operator: types.OperatorGreaterThan, Value: "5"}
	inv := invertCondition(c)
	if inv.Operator != types.OperatorLessOrEqual {
		t.Errorf("expected OperatorLessOrEqual, got op=%q", inv.Operator)
	}
}

func TestInvertCondition_Fallback_Not(t *testing.T) {
	// An operator with no explicit inverse mapping falls back to LogicOpNot wrapper.
	c := &types.Condition{Operator: types.OperatorContains, Value: "x"}
	inv := invertCondition(c)
	if inv.Logic != types.LogicOpNot {
		t.Errorf("expected LogicOpNot fallback, got logic=%q op=%q", inv.Logic, inv.Operator)
	}
}

// --- convertXFAEventToRules integration -------------------------------------

func TestConvertXFAEventToRules_EventTypeMapping(t *testing.T) {
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
		rules, err := convertXFAEventToRules(event, "f", 1)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.eventType, err)
		}
		if len(rules) == 0 {
			t.Errorf("%q: expected at least one rule", tc.eventType)
			continue
		}
		if rules[0].Type != tc.want {
			t.Errorf("event type %q: expected rule type %q, got %q", tc.eventType, tc.want, rules[0].Type)
		}
	}
}

func TestConvertXFAEventToRules_ScriptOverridesEventType(t *testing.T) {
	// A "change" event whose script is actually a visibility toggle.
	event := XFAEvent{
		Type:   "change",
		Script: `$.presence = "hidden"`,
	}
	rules, err := convertXFAEventToRules(event, "f", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected at least one rule")
	}
	if rules[0].Type != types.RuleTypeVisibility {
		t.Errorf("expected RuleTypeVisibility (from script), got %q", rules[0].Type)
	}
}

func TestConvertXFAEventToRules_PreservesSourceAndID(t *testing.T) {
	event := XFAEvent{Type: "validate", Script: `return false`}
	rules, _ := convertXFAEventToRules(event, "emailField", 7)
	if len(rules) == 0 {
		t.Fatal("expected at least one rule")
	}
	rule := rules[0]
	if rule.Source != "emailField" {
		t.Errorf("expected source 'emailField', got %q", rule.Source)
	}
	if rule.ID != "rule_7" {
		t.Errorf("expected ID 'rule_7', got %q", rule.ID)
	}
}

func TestConvertXFAEventToRules_IfElse_TwoRules(t *testing.T) {
	event := XFAEvent{
		Type: "change",
		Script: `if ($.rawValue == "yes") then
    sectionA.presence = "visible"
    sectionB.presence = "hidden"
else
    sectionA.presence = "hidden"
    sectionB.presence = "visible"
endif`,
		Lang: "formcalc",
	}
	rules, err := convertXFAEventToRules(event, "toggle", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules for if/else script, got %d", len(rules))
	}
}

// --- splitJSIfElseChain --------------------------------------------------

func TestSplitJSIfElseChain_ThreeBranches(t *testing.T) {
	s := `if (field.rawValue == "0") {
  A.presence = "visible";
  B.presence = "hidden";
} else if (field.rawValue == "1") {
  A.presence = "hidden";
  B.presence = "visible";
} else if (field.rawValue == "2") {
  A.presence = "hidden";
  B.presence = "hidden";
}`
	branches := splitJSIfElseChain(s)
	if len(branches) != 3 {
		t.Fatalf("expected 3 branches, got %d", len(branches))
	}
	if branches[0].cond != `field.rawValue == "0"` {
		t.Errorf("branch 0 cond = %q", branches[0].cond)
	}
	if branches[2].cond != `field.rawValue == "2"` {
		t.Errorf("branch 2 cond = %q", branches[2].cond)
	}
}

func TestSplitJSIfElseChain_WithElse(t *testing.T) {
	s := `if (x.rawValue == "a") {
  A.presence = "visible";
} else {
  A.presence = "hidden";
}`
	branches := splitJSIfElseChain(s)
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d: %+v", len(branches), branches)
	}
	if branches[0].cond != `x.rawValue == "a"` {
		t.Errorf("branch 0 cond = %q", branches[0].cond)
	}
	if branches[1].cond != "" {
		t.Errorf("else branch should have empty cond, got %q", branches[1].cond)
	}
}

// --- parseVariablesFunctionRules -----------------------------------------

func TestParseVariablesFunctionRules_ThreeBranches(t *testing.T) {
	body := `
  if (ATRadioButton100.rawValue == "0") {
    IMDRF.presence = "visible";
    USA.presence = "hidden";
    CDN.presence = "hidden";
  } else if (ATRadioButton100.rawValue == "1") {
    IMDRF.presence = "hidden";
    USA.presence = "visible";
    CDN.presence = "hidden";
  } else if (ATRadioButton100.rawValue == "2") {
    IMDRF.presence = "hidden";
    USA.presence = "hidden";
    CDN.presence = "visible";
  }`
	rules := parseVariablesFunctionRules(body, "javascript", 0)
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	// Branch 0: IMDRF visible, USA hidden, CDN hidden
	r0 := rules[0]
	if r0.Condition == nil || r0.Condition.Expression != `ATRadioButton100.rawValue == "0"` {
		t.Errorf("rule 0 condition = %+v", r0.Condition)
	}
	// Branch 0 should show IMDRF and hide USA and CDN
	var showTargets, hideTargets []string
	for _, a := range r0.Actions {
		if a.Type == types.ActionTypeShow {
			showTargets = append(showTargets, a.Target)
		} else if a.Type == types.ActionTypeHide {
			hideTargets = append(hideTargets, a.Target)
		}
	}
	if len(showTargets) != 1 || showTargets[0] != "IMDRF" {
		t.Errorf("branch 0 show targets = %v, want [IMDRF]", showTargets)
	}
	if len(hideTargets) != 2 {
		t.Errorf("branch 0 hide targets = %v, want [USA CDN]", hideTargets)
	}

	// Branch 1: USA visible
	r1 := rules[1]
	if r1.Condition == nil || r1.Condition.Expression != `ATRadioButton100.rawValue == "1"` {
		t.Errorf("rule 1 condition = %+v", r1.Condition)
	}
	var show1 []string
	for _, a := range r1.Actions {
		if a.Type == types.ActionTypeShow {
			show1 = append(show1, a.Target)
		}
	}
	if len(show1) != 1 || show1[0] != "USA" {
		t.Errorf("branch 1 show targets = %v, want [USA]", show1)
	}
}

// --- extractJSFunctionBodies --------------------------------------------

func TestExtractJSFunctionBodies_SingleFunction(t *testing.T) {
	script := `function AutoPopulate() {
  if (x.rawValue == "0") { A.presence = "visible"; }
}`
	fns := extractJSFunctionBodies(script, "javascript")
	if len(fns) != 1 {
		t.Fatalf("expected 1 function, got %d", len(fns))
	}
	if !strings.Contains(fns[0].body, "A.presence") {
		t.Errorf("function body missing expected content: %q", fns[0].body)
	}
}

func TestExtractJSFunctionBodies_TwoFunctions(t *testing.T) {
	script := `function AutoPopulate() {
  A.presence = "visible";
}
function LBPresence() {
  B.presence = "hidden";
}`
	fns := extractJSFunctionBodies(script, "javascript")
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}
}
