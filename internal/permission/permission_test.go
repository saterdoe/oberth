package permission

import (
	"testing"
)

func TestDenyByDefaultForWriteOperations(t *testing.T) {
	e := New()
	d, _ := e.Evaluate(Request{Operation: "file.write", Target: "C:\\repo\\main.go"})
	if d != Deny {
		t.Errorf("expected Deny for file.write, got %v", d)
	}
}

func TestDenyByDefaultForCommandExec(t *testing.T) {
	e := New()
	d, _ := e.Evaluate(Request{Operation: "command.exec", Target: "go test ./..."})
	if d != Deny {
		t.Errorf("expected Deny for command.exec, got %v", d)
	}
}

func TestDefaultAllowForReadAndBrowse(t *testing.T) {
	e := New()
	for _, op := range []string{"file.read", "browse"} {
		d, _ := e.Evaluate(Request{Operation: op, Target: "C:\\repo\\main.go"})
		if d != Allow {
			t.Errorf("expected Allow for %s, got %v", op, d)
		}
	}
}

func TestAllowRuleOverridesDefaultDeny(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "allow writes in repo",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "C:\\repo\\**",
		Decision:      Allow,
	})
	d, r := e.Evaluate(Request{Operation: "file.write", Target: "C:\\repo\\main.go"})
	if d != Allow {
		t.Errorf("expected Allow, got %v", d)
	}
	if r == nil || r.Name != "allow writes in repo" {
		t.Errorf("expected matching rule, got %v", r)
	}
}

func TestPriorityOrdering(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "low allow",
		Priority:      10,
		Operation:     "file.write",
		TargetPattern: "**",
		Decision:      Allow,
	})
	e.AddRule(Rule{
		Name:          "high deny",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "C:\\secret\\**",
		Decision:      Deny,
	})
	d, r := e.Evaluate(Request{Operation: "file.write", Target: "C:\\secret\\token.txt"})
	if d != Deny || r == nil || r.Name != "high deny" {
		t.Errorf("expected Deny from high priority rule, got %v (%v)", d, r)
	}
}

func TestAskDecision(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "ask for new files",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "**",
		Decision:      Ask,
	})
	d, r := e.Evaluate(Request{Operation: "file.write", Target: "C:\\repo\\new.go"})
	if d != Ask || r == nil || r.Decision != Ask {
		t.Errorf("expected Ask, got %v", d)
	}
}

func TestRepoPatternFiltering(t *testing.T) {
	e := New()
	repo := "C:\\projects\\myapp"
	e.AddRule(Rule{
		Name:          "allow for myapp",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "**",
		RepoPattern:   &repo,
		Decision:      Allow,
	})
	d, _ := e.Evaluate(Request{Operation: "file.write", Target: "C:\\other\\file.go"})
	if d != Deny {
		t.Errorf("expected Deny for different repo, got %v", d)
	}
	d, _ = e.Evaluate(Request{Operation: "file.write", Target: "C:\\projects\\myapp\\file.go", RepoPath: "C:\\projects\\myapp"})
	if d != Allow {
		t.Errorf("expected Allow for matching repo, got %v", d)
	}
}

func TestHigherPriorityWinsOverLower(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "deny src",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "**\\src\\**",
		Decision:      Deny,
	})
	e.AddRule(Rule{
		Name:          "allow src/test",
		Priority:      200,
		Operation:     "file.write",
		TargetPattern: "**\\src\\test\\**",
		Decision:      Allow,
	})
	d, r := e.Evaluate(Request{Operation: "file.write", Target: "C:\\repo\\src\\test\\util_test.go"})
	if d != Allow || r == nil || r.Name != "allow src/test" {
		t.Errorf("expected Allow from higher priority rule, got %v (%v)", d, r)
	}
	d, _ = e.Evaluate(Request{Operation: "file.write", Target: "C:\\repo\\src\\main.go"})
	if d != Deny {
		t.Errorf("expected Deny for src/main, got %v", d)
	}
}

func TestOperationGlobMatching(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "allow all file ops",
		Priority:      100,
		Operation:     "file.*",
		TargetPattern: "**",
		Decision:      Allow,
	})
	d, _ := e.Evaluate(Request{Operation: "file.write", Target: "C:\\a.go"})
	if d != Allow {
		t.Errorf("expected Allow via file.* match, got %v", d)
	}
	d, _ = e.Evaluate(Request{Operation: "file.read", Target: "C:\\a.go"})
	if d != Allow {
		t.Errorf("expected Allow via file.* match, got %v", d)
	}
	d, _ = e.Evaluate(Request{Operation: "command.exec", Target: "go test"})
	if d != Deny {
		t.Errorf("expected Deny for command not matching file.*, got %v", d)
	}
}

func TestDefaultDenyWhenNoRuleMatches(t *testing.T) {
	e := New()
	d, r := e.Evaluate(Request{Operation: "unknown.op", Target: "something"})
	if d != Deny {
		t.Errorf("expected Deny for unmatched operation, got %v", d)
	}
	if r != nil {
		t.Errorf("expected no rule for default deny, got %v", r)
	}
}

func TestInactiveRuleIgnored(t *testing.T) {
	e := New()
	e.AddRule(Rule{
		Name:          "inactive allow",
		Priority:      100,
		Operation:     "file.write",
		TargetPattern: "**",
		Decision:      Allow,
	})
	e.SetRuleActive("inactive allow", false)
	d, _ := e.Evaluate(Request{Operation: "file.write", Target: "any.go"})
	if d != Deny {
		t.Errorf("expected Deny because rule is inactive, got %v", d)
	}
}

func TestMultipleRulesSamePriorityUsesSafetyPrecedence(t *testing.T) {
	e := New()
	e.AddRule(Rule{Name: "first", Priority: 100, Operation: "op", TargetPattern: "**", Decision: Deny})
	e.AddRule(Rule{Name: "second", Priority: 100, Operation: "op", TargetPattern: "**", Decision: Allow})
	d, r := e.Evaluate(Request{Operation: "op", Target: "any"})
	if d != Deny || r == nil || r.Name != "first" {
		t.Errorf("expected deny to win at the same priority, got %v (%v)", d, r)
	}
}
