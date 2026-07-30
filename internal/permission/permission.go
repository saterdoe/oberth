package permission

import (
	"strings"
)

const SchemaVersion = "1"

type Decision int

const (
	Allow Decision = iota
	Deny
	Ask
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	default:
		return "unknown"
	}
}

type Request struct {
	Operation string
	Target    string
	RepoPath  string
	TaskType  string
	UserID    string
	TaskID    string
	SessionID string
	RunID     string
	Risk      string
}

type Rule struct {
	Name          string
	Description   string
	Priority      int
	Operation     string
	TargetPattern string
	RepoPattern   *string
	TaskType      *string
	Risk          *string
	Decision      Decision
	IsActive      bool
}

type Engine struct {
	rules []Rule
}

func New() *Engine {
	return &Engine{}
}

func (e *Engine) AddRule(r Rule) {
	if r.Operation == "" {
		r.Operation = "*"
	}
	if r.TargetPattern == "" {
		r.TargetPattern = "*"
	}
	r.IsActive = true
	e.rules = append(e.rules, r)
}

// SetRuleActive updates IsActive on an existing rule matched by name.
func (e *Engine) SetRuleActive(name string, active bool) {
	for i := range e.rules {
		if e.rules[i].Name == name {
			e.rules[i].IsActive = active
			return
		}
	}
}

func (e *Engine) Evaluate(req Request) (Decision, *Rule) {
	var best *Rule
	for i := range e.rules {
		r := &e.rules[i]
		if !r.IsActive {
			continue
		}
		if !matchGlob(r.Operation, req.Operation) {
			continue
		}
		if !matchGlob(r.TargetPattern, req.Target) {
			continue
		}
		if r.RepoPattern != nil && !matchGlob(*r.RepoPattern, req.RepoPath) {
			continue
		}
		if r.TaskType != nil && *r.TaskType != req.TaskType {
			continue
		}
		if r.Risk != nil && *r.Risk != req.Risk {
			continue
		}
		if best == nil || r.Priority > best.Priority ||
			(r.Priority == best.Priority && decisionRank(r.Decision) > decisionRank(best.Decision)) ||
			(r.Priority == best.Priority && r.Decision == best.Decision && i > indexOf(e.rules, best)) {
			best = r
		}
	}
	if best != nil {
		return best.Decision, best
	}
	return defaultDecision(req.Operation), nil
}

func decisionRank(decision Decision) int {
	switch decision {
	case Deny:
		return 3
	case Ask:
		return 2
	default:
		return 1
	}
}

func indexOf(rules []Rule, r *Rule) int {
	for i := range rules {
		if &rules[i] == r {
			return i
		}
	}
	return -1
}

func defaultDecision(op string) Decision {
	switch {
	case op == "file.read" || op == "browse":
		return Allow
	case strings.HasPrefix(op, "cloud."):
		return Ask
	default:
		return Deny
	}
}

func matchGlob(pattern, value string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	parts := strings.Split(pattern, "**")
	if len(parts) == 1 {
		p := parts[0]
		if strings.HasPrefix(p, "*") {
			return strings.HasSuffix(value, strings.TrimPrefix(p, "*"))
		}
		if strings.HasSuffix(p, "*") {
			return strings.HasPrefix(value, strings.TrimSuffix(p, "*"))
		}
		return value == p
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(value, part) {
				return false
			}
			value = value[len(part):]
		} else if i == len(parts)-1 {
			if !strings.HasSuffix(value, part) {
				return false
			}
			value = value[:len(value)-len(part)]
		} else {
			idx := strings.Index(value, part)
			if idx < 0 {
				return false
			}
			value = value[idx+len(part):]
		}
	}
	return true
}
