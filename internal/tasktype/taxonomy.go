package tasktype

import "strings"

const SchemaVersion = "1"

const (
	Implementation = "implementation"
	BugFix         = "bug_fix"
	Review         = "review"
	Testing        = "testing"
	Docs           = "docs"
	Architecture   = "architecture"
	Research       = "research"
)

func Normalize(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "code", "dev", "development", "implement", "implementation":
		return Implementation
	case "debug", "bug", "bugfix", "bug_fix", "fix":
		return BugFix
	case "review":
		return Review
	case "test", "tests", "testing":
		return Testing
	case "doc", "docs", "documentation":
		return Docs
	case "architecture", "design":
		return Architecture
	case "research":
		return Research
	default:
		return Implementation
	}
}

func Infer(intention string) string {
	lower := strings.ToLower(intention)
	switch {
	case strings.Contains(lower, "review") || strings.Contains(lower, "revis"):
		return Review
	case strings.Contains(lower, "bug") || strings.Contains(lower, "fix") || strings.Contains(lower, "corrige"):
		return BugFix
	case strings.Contains(lower, "test") || strings.Contains(lower, "prueba"):
		return Testing
	case strings.Contains(lower, "document") || strings.Contains(lower, "readme"):
		return Docs
	case strings.Contains(lower, "architect") || strings.Contains(lower, "diseñ"):
		return Architecture
	case strings.Contains(lower, "research") || strings.Contains(lower, "investig"):
		return Research
	default:
		return Implementation
	}
}

func InferRisk(intention string) string {
	lower := strings.ToLower(intention)
	for _, marker := range []string{"delete", "elimina", "migration", "migraci", "credential", "secret", "deploy", "push", "production", "producci"} {
		if strings.Contains(lower, marker) {
			return "high"
		}
	}
	for _, marker := range []string{"docs", "readme", "comentario", "typo", "formato"} {
		if strings.Contains(lower, marker) {
			return "low"
		}
	}
	return "medium"
}

func InferStrategy(intention string) string {
	lower := strings.ToLower(intention)
	if strings.Contains(lower, "solo analiza") || strings.Contains(lower, "no cambies") || strings.Contains(lower, "read only") {
		return "ask"
	}
	return "guided"
}
