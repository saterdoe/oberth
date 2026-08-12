package secrets

import (
	"encoding/json"
	"regexp"
	"strings"
)

type Match struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

type Result struct {
	HasSecrets bool    `json:"has_secrets"`
	Matches    []Match `json:"matches"`
	Redacted   string  `json:"redacted"`
}

var patterns = []struct {
	Name string
	Re   *regexp.Regexp
}{
	{"OpenAI API Key", regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`)},
	{"Anthropic API Key", regexp.MustCompile(`sk-ant-[A-Za-z0-9]{20,}`)},
	{"AWS Access Key", regexp.MustCompile(`(?:AKIA|ASIA)[A-Z0-9]{16,}`)},
	{"GitHub Token", regexp.MustCompile(`gh[pousry]_[A-Za-z0-9_]{20,}`)},
	{"GitLab Token", regexp.MustCompile(`glpat-[A-Za-z0-9\-_]{20,}`)},
	{"JWT Token", regexp.MustCompile(`eyJ[A-Za-z0-9\-_]+\.eyJ[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+`)},
	{"RSA Private Key", regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----`)},
	{"EC Private Key", regexp.MustCompile(`-----BEGIN EC PRIVATE KEY-----`)},
	{"OpenSSH Private Key", regexp.MustCompile(`-----BEGIN OPENSSH PRIVATE KEY-----`)},
	{"Generic Private Key", regexp.MustCompile(`-----BEGIN PRIVATE KEY-----`)},
	{"Password Assignment", regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*['\"][^'\"]+['\"]`)},
	{"Secret Assignment", regexp.MustCompile(`(?i)(secret|api_key|apikey|token)\s*[=:]\s*['\"][^'\"]+['\"]`)},
	{"Connection String", regexp.MustCompile(`[a-z]+://[^:]+:[^@]+@`)},
	{"Slack Token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{20,}`)},
	{"Google Service Account", regexp.MustCompile(`[A-Za-z0-9\-_]+@[A-Za-z0-9\-_]+\.iam\.gserviceaccount\.com`)},
}

func Scan(input string) Result {
	var matches []Match
	for _, p := range patterns {
		locs := p.Re.FindAllStringIndex(input, -1)
		for _, loc := range locs {
			start, end := loc[0], loc[1]
			overlap := false
			for _, m := range matches {
				if start < m.End && end > m.Start {
					if end-start > m.End-m.Start {
						m.Start = start
						m.End = end
						m.Type = p.Name
					}
					overlap = true
					break
				}
			}
			if !overlap {
				matches = append(matches, Match{Type: p.Name, Pattern: input[start:end], Start: start, End: end})
			}
		}
	}
	return Result{
		HasSecrets: len(matches) > 0,
		Matches:    matches,
		Redacted:   Redact(input),
	}
}

func HasSecrets(input string) bool {
	for _, p := range patterns {
		if p.Re.MatchString(input) {
			return true
		}
	}
	return false
}

func Redact(input string) string {
	result := input
	for _, p := range patterns {
		result = p.Re.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func RedactWithType(input string) string {
	result := input
	for _, p := range patterns {
		result = p.Re.ReplaceAllStringFunc(result, func(match string) string {
			return "[REDACTED:" + p.Name + "]"
		})
	}
	return result
}

// MarshalRedacted serializes a value and removes supported secret patterns
// recursively, including secrets nested inside maps, arrays and tool output.
func MarshalRedacted(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var structured any
	if err := json.Unmarshal(encoded, &structured); err != nil {
		return nil, err
	}
	redactStructured(structured)
	return json.Marshal(structured)
}

func redactStructured(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
				strings.Contains(lower, "token") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
				current[key] = "[REDACTED]"
				continue
			}
			if text, ok := child.(string); ok {
				current[key] = Redact(text)
			} else {
				redactStructured(child)
			}
		}
	case []any:
		for index, child := range current {
			if text, ok := child.(string); ok {
				current[index] = Redact(text)
			} else {
				redactStructured(child)
			}
		}
	}
}

func SanitizeKey(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[:2] + strings.Repeat("*", len(key)-4) + key[len(key)-2:]
}
