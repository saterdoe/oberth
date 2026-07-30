package vault

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var validTypes = map[string]bool{
	"architecture":      true,
	"bug":               true,
	"code_map":          true,
	"conventions":       true,
	"decision":          true,
	"decisions":         true,
	"features":          true,
	"fixes":             true,
	"gotchas":           true,
	"memory_index":      true,
	"notebooklm_export": true,
	"pattern":           true,
	"project":           true,
	"session":           true,
	"task":              true,
}

var frontmatterRe = regexp.MustCompile(`^---\n([\s\S]*?)\n---(?:\n|$)`)

type validationError string

func (e validationError) Error() string { return string(e) }

var emptyFrontmatterRe = regexp.MustCompile(`^---\n---(\n|$)`)

// ParseFrontmatter extracts YAML frontmatter and body from markdown content.
// If no frontmatter is found, metadata is nil and body is the entire content.
func ParseFrontmatter(content string) (map[string]any, string, error) {
	matches := frontmatterRe.FindStringSubmatch(content)
	if matches == nil {
		if m := emptyFrontmatterRe.FindString(content); m != "" {
			body := strings.TrimPrefix(content, m)
			return map[string]any{}, body, nil
		}
		return nil, content, nil
	}

	yamlBlock := matches[1]
	body := strings.TrimPrefix(content, matches[0])

	metadata := make(map[string]any)
	if err := yaml.Unmarshal([]byte(yamlBlock), &metadata); err != nil {
		return nil, "", fmt.Errorf("vault: invalid frontmatter YAML: %w", err)
	}

	if err := validateMetadata(metadata); err != nil {
		return nil, "", err
	}

	return metadata, body, nil
}

// WriteFrontmatter writes markdown content with YAML frontmatter.
func WriteFrontmatter(metadata map[string]any, body string) (string, error) {
	if metadata == nil {
		metadata = make(map[string]any)
	}

	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("---\n")

	if len(metadata) > 0 {
		yamlBytes, err := yaml.Marshal(metadata)
		if err != nil {
			return "", fmt.Errorf("vault: failed to marshal frontmatter: %w", err)
		}
		sb.Write(yamlBytes)
	}

	sb.WriteString("---\n")
	sb.WriteString(body)

	return sb.String(), nil
}

func validateMetadata(metadata map[string]any) error {
	if t, ok := metadata["type"]; ok {
		typeStr, ok := t.(string)
		if !ok || !validTypes[typeStr] {
			return validationError(
				fmt.Sprintf("vault: invalid note type %q", t),
			)
		}
	}

	if d, ok := metadata["date"]; ok {
		dateStr, ok := d.(string)
		if !ok {
			return validationError("vault: date must be a string in ISO 8601 format")
		}
		if _, err := time.Parse(time.RFC3339, dateStr); err != nil {
			if _, err := time.Parse("2006-01-02", dateStr); err != nil {
				return validationError(fmt.Sprintf("vault: date %q is not valid ISO 8601", dateStr))
			}
		}
	}

	return nil
}
