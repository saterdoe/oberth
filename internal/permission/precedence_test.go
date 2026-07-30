package permission

import "testing"

func TestEqualPriorityUsesDenyAskAllowPrecedence(t *testing.T) {
	engine := New()
	engine.AddRule(Rule{Name: "allow", Priority: 10, Operation: "command.exec", TargetPattern: "*", Decision: Allow})
	engine.AddRule(Rule{Name: "ask", Priority: 10, Operation: "command.exec", TargetPattern: "*", Decision: Ask})
	engine.AddRule(Rule{Name: "deny", Priority: 10, Operation: "command.exec", TargetPattern: "*", Decision: Deny})
	decision, rule := engine.Evaluate(Request{Operation: "command.exec", Target: "danger"})
	if decision != Deny || rule == nil || rule.Name != "deny" {
		t.Fatalf("expected deny precedence, got %s (%v)", decision, rule)
	}
}
