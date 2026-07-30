package context

import (
	"fmt"
	"strings"

	"github.com/saterdoe/oberth/internal/vault"
)

// NoteRef represents a reference to a note in the memory index.
type NoteRef struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Date  string `json:"date"`
}

// BuildMemoryIndex generates the memory-index.md content from a list of notes.
// Format: resumen LLM + lista plana de notas.
func BuildMemoryIndex(notes []vault.Note) string {
	var b strings.Builder
	b.WriteString("# Memory Index\n")
	b.WriteString("> Generated automatically. Updated after each session.\n\n")
	b.WriteString("## Summary\n")
	b.WriteString("<!-- Resumen LLM compilado del estado del vault -->\n\n")
	b.WriteString("## Notes\n")

	for _, note := range notes {
		noteType, _ := note.Metadata["type"].(string)
		noteDate, _ := note.Metadata["date"].(string)

		display := note.Path
		if idx := strings.LastIndex(note.Path, "/"); idx >= 0 {
			display = note.Path[idx+1:]
		}

		line := fmt.Sprintf("- [%s](%s)", display, note.Path)
		if noteType != "" || noteDate != "" {
			line += " —"
			if noteType != "" {
				line += fmt.Sprintf(" type: %s", noteType)
			}
			if noteDate != "" {
				if noteType != "" {
					line += ","
				}
				line += " " + noteDate
			}
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

// ParseMemoryIndex reads the memory-index content and returns a list of note references.
func ParseMemoryIndex(content string) []NoteRef {
	var refs []NoteRef
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		line = strings.TrimPrefix(line, "- [")
		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}
		title := line[:closeBracket]
		rest := line[closeBracket+1:]
		if !strings.HasPrefix(rest, "(") {
			continue
		}
		rest = rest[1:]
		closeParen := strings.Index(rest, ")")
		if closeParen < 0 {
			continue
		}
		path := rest[:closeParen]
		rest = strings.TrimSpace(rest[closeParen+1:])

		ref := NoteRef{Path: path, Title: title}

		if strings.HasPrefix(rest, "—") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "—"))
			parts := strings.Split(rest, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "type: ") {
					ref.Type = strings.TrimPrefix(p, "type: ")
				} else if p != "" {
					ref.Date = p
				}
			}
		}

		refs = append(refs, ref)
	}
	return refs
}
