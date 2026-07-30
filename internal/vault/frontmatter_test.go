package vault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter_WithFrontmatter(t *testing.T) {
	content := `---
title: My Note
type: architecture
date: "2024-01-15"
---
This is the body content.
`
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, "My Note", metadata["title"])
	assert.Equal(t, "architecture", metadata["type"])
	assert.Equal(t, "2024-01-15", metadata["date"])
	assert.Equal(t, "This is the body content.\n", body)
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just a plain markdown file.\n"
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, content, body)
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	content := `---
invalid: [yaml: broken
---
body
`
	_, _, err := ParseFrontmatter(content)
	assert.Error(t, err)
}

func TestParseFrontmatter_InvalidType(t *testing.T) {
	content := `---
type: invalid_type
---
body
`
	_, _, err := ParseFrontmatter(content)
	assert.Error(t, err)
}

func TestParseFrontmatter_ValidTypes(t *testing.T) {
	types := []string{"architecture", "decision", "pattern", "bug", "session", "task"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			content := "---\ntype: " + typ + "\n---\nbody"
			_, _, err := ParseFrontmatter(content)
			assert.NoError(t, err, "type %q should be valid", typ)
		})
	}
}

func TestParseFrontmatter_InvalidDate(t *testing.T) {
	content := `---
date: not-a-date
---
body
`
	_, _, err := ParseFrontmatter(content)
	assert.Error(t, err)
}

func TestRoundTrip(t *testing.T) {
	metadata := map[string]any{
		"title": "Test Note",
		"type":  "decision",
		"date":  "2024-06-15",
		"tags":  []any{"go", "testing"},
	}
	body := "# Hello\n\nThis is a test note.\n"

	written, err := WriteFrontmatter(metadata, body)
	require.NoError(t, err)

	parsedMeta, parsedBody, err := ParseFrontmatter(written)
	require.NoError(t, err)

	assert.Equal(t, metadata["title"], parsedMeta["title"])
	assert.Equal(t, metadata["type"], parsedMeta["type"])
	assert.Equal(t, metadata["date"], parsedMeta["date"])
	assert.Equal(t, body, parsedBody)
}

func TestWriteFrontmatter_InvalidType(t *testing.T) {
	_, err := WriteFrontmatter(map[string]any{"type": "invalid"}, "body")
	assert.Error(t, err)
}

func TestWriteFrontmatter_InvalidDate(t *testing.T) {
	_, err := WriteFrontmatter(map[string]any{"date": "bad-date"}, "body")
	assert.Error(t, err)
}

func TestParseFrontmatter_EmptyFrontmatter(t *testing.T) {
	content := "---\n---\nbody content"
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.NotNil(t, metadata)
	assert.Equal(t, "body content", body)
}

func TestParseFrontmatter_NoClosingDelimiter(t *testing.T) {
	content := "---\ntitle: test\nno closing delimiter here"
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Nil(t, metadata)
	assert.Equal(t, content, body)
}

func TestParseFrontmatter_BodyWithEmbeddedDelimiters(t *testing.T) {
	content := `---
title: test
type: task
---
Code block with --- in it.
`
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, "test", metadata["title"])
	assert.Equal(t, "Code block with --- in it.\n", body)
}

func TestParseFrontmatter_MetadataAsNonString(t *testing.T) {
	content := `---
count: 42
enabled: true
type: task
---
body`
	metadata, body, err := ParseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, 42, metadata["count"])
	assert.Equal(t, true, metadata["enabled"])
	assert.Equal(t, "body", body)
}
