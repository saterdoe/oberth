package context

import (
	"fmt"
	"strings"

	"github.com/saterdoe/oberth/internal/vault"
)

type ResumeVault interface {
	ListNotes(string) ([]vault.Note, error)
}

type ResumeContext struct {
	Task       *vault.Note  `json:"task,omitempty"`
	Session    *vault.Note  `json:"session,omitempty"`
	Summary    string       `json:"summary"`
	Decisions  []vault.Note `json:"decisions"`
	Characters int          `json:"characters"`
}

func BuildResumeContext(source ResumeVault, taskID string, maxCharacters int) (ResumeContext, error) {
	if maxCharacters <= 0 {
		return ResumeContext{}, fmt.Errorf("resume context budget must be positive")
	}
	result := ResumeContext{Decisions: []vault.Note{}}
	tasks, err := source.ListNotes("tasks")
	if err != nil {
		return result, err
	}
	for index := range tasks {
		if noteMatches(tasks[index], taskID) {
			result.Task = &tasks[index]
			break
		}
	}
	sessions, err := source.ListNotes("sessions")
	if err != nil {
		return result, err
	}
	for index := range sessions {
		if noteMatches(sessions[index], taskID) {
			result.Session = &sessions[index]
			break
		}
	}
	parts := []string{}
	if result.Task != nil {
		parts = append(parts, extractSummary(result.Task.Content))
	}
	if result.Session != nil {
		parts = append(parts, extractSummary(result.Session.Content))
	}
	result.Summary = strings.TrimSpace(strings.Join(parts, "\n\n"))
	decisions, err := source.ListNotes("decisions")
	if err != nil {
		return result, err
	}
	for _, decision := range decisions {
		if strings.Contains(strings.ToLower(decision.Content), strings.ToLower(taskID)) {
			result.Decisions = append(result.Decisions, decision)
		}
	}
	var combined strings.Builder
	combined.WriteString(result.Summary)
	for _, decision := range result.Decisions {
		combined.WriteString("\n\n")
		combined.WriteString(decision.Path)
		combined.WriteString("\n")
		combined.WriteString(extractSummary(decision.Content))
	}
	text := combined.String()
	if len(text) > maxCharacters {
		text = text[:maxCharacters]
	}
	result.Characters = len(text)
	if len(result.Summary) > maxCharacters {
		result.Summary = result.Summary[:maxCharacters]
	}
	return result, nil
}

func noteMatches(note vault.Note, id string) bool {
	if strings.Contains(strings.ToLower(note.Path), strings.ToLower(id)) {
		return true
	}
	for _, key := range []string{"id", "task_id", "session_id"} {
		if value, ok := note.Metadata[key].(string); ok && strings.EqualFold(value, id) {
			return true
		}
	}
	return false
}

func extractSummary(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if marker := strings.Index(strings.ToLower(normalized), "## summary"); marker >= 0 {
		summary := normalized[marker+len("## summary"):]
		if next := strings.Index(summary, "\n## "); next >= 0 {
			summary = summary[:next]
		}
		return strings.TrimSpace(summary)
	}
	if len(normalized) > 500 {
		return strings.TrimSpace(normalized[:500])
	}
	return strings.TrimSpace(normalized)
}
