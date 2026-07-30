package context_test

import (
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMemoryIndex_ThreeNotes(t *testing.T) {
	notes := []vault.Note{
		{Path: "architecture/microservices-decision", Metadata: map[string]any{"type": "decision", "date": "2026-05-24"}},
		{Path: "architecture/api-design", Metadata: map[string]any{"type": "architecture", "date": "2026-05-23"}},
		{Path: "bugs/login-error", Metadata: map[string]any{"type": "bug"}},
	}

	index := pkgctx.BuildMemoryIndex(notes)

	assert.Contains(t, index, "# Memory Index")
	assert.Contains(t, index, "> Generated automatically. Updated after each session.")
	assert.Contains(t, index, "## Summary")
	assert.Contains(t, index, "## Notes")
	assert.Contains(t, index, "- [microservices-decision](architecture/microservices-decision) — type: decision, 2026-05-24")
	assert.Contains(t, index, "- [api-design](architecture/api-design) — type: architecture, 2026-05-23")
	assert.Contains(t, index, "- [login-error](bugs/login-error) — type: bug")
}

func TestParseMemoryIndex(t *testing.T) {
	content := `# Memory Index
> Generated automatically. Updated after each session.

## Summary
<!-- Resumen LLM compilado del estado del vault -->

## Notes
- [microservices-decision](architecture/microservices-decision) — type: decision, 2026-05-24
- [api-design](architecture/api-design) — type: architecture, 2026-05-23
- [login-error](bugs/login-error) — type: bug
- [plain-note](plain-note)
`

	refs := pkgctx.ParseMemoryIndex(content)
	require.Len(t, refs, 4)

	assert.Equal(t, "architecture/microservices-decision", refs[0].Path)
	assert.Equal(t, "microservices-decision", refs[0].Title)
	assert.Equal(t, "decision", refs[0].Type)
	assert.Equal(t, "2026-05-24", refs[0].Date)

	assert.Equal(t, "architecture/api-design", refs[1].Path)
	assert.Equal(t, "api-design", refs[1].Title)
	assert.Equal(t, "architecture", refs[1].Type)
	assert.Equal(t, "2026-05-23", refs[1].Date)

	assert.Equal(t, "bugs/login-error", refs[2].Path)
	assert.Equal(t, "login-error", refs[2].Title)
	assert.Equal(t, "bug", refs[2].Type)
	assert.Equal(t, "", refs[2].Date)

	assert.Equal(t, "plain-note", refs[3].Path)
	assert.Equal(t, "plain-note", refs[3].Title)
	assert.Equal(t, "", refs[3].Type)
	assert.Equal(t, "", refs[3].Date)
}

func TestBuildMemoryIndex_RoundTrip(t *testing.T) {
	notes := []vault.Note{
		{Path: "architecture/microservices-decision", Metadata: map[string]any{"type": "decision", "date": "2026-05-24"}},
		{Path: "bugs/login-error", Metadata: map[string]any{"type": "bug"}},
		{Path: "plain-note", Metadata: map[string]any{}},
	}

	index := pkgctx.BuildMemoryIndex(notes)
	refs := pkgctx.ParseMemoryIndex(index)

	require.Len(t, refs, 3)
	assert.Equal(t, "architecture/microservices-decision", refs[0].Path)
	assert.Equal(t, "decision", refs[0].Type)
	assert.Equal(t, "2026-05-24", refs[0].Date)

	assert.Equal(t, "bugs/login-error", refs[1].Path)
	assert.Equal(t, "bug", refs[1].Type)
	assert.Equal(t, "", refs[1].Date)

	assert.Equal(t, "plain-note", refs[2].Path)
	assert.Equal(t, "", refs[2].Type)
	assert.Equal(t, "", refs[2].Date)
}

func TestBuildMemoryIndex_EmptyNotes(t *testing.T) {
	index := pkgctx.BuildMemoryIndex(nil)

	assert.Contains(t, index, "# Memory Index")
	assert.Contains(t, index, "## Notes\n")
	assert.NotContains(t, index, "- [")

	index2 := pkgctx.BuildMemoryIndex([]vault.Note{})
	assert.Contains(t, index2, "## Notes\n")
	assert.NotContains(t, index2, "- [")
}
