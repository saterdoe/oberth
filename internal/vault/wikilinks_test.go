package vault

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractWikilinks(t *testing.T) {
	content := `# Note

This links to [[Another Note]] and also to [[Yet Another]].

But [[this]] is a short one.`

	links := ExtractWikilinks(content)
	expected := []string{"Another Note", "Yet Another", "this"}
	sort.Strings(links)
	sort.Strings(expected)
	assert.Equal(t, expected, links)
}

func TestExtractWikilinks_NoLinks(t *testing.T) {
	content := "Just plain text without any wikilinks."
	links := ExtractWikilinks(content)
	assert.Empty(t, links)
}

func TestExtractWikilinks_EmptyContent(t *testing.T) {
	links := ExtractWikilinks("")
	assert.Empty(t, links)
}

func TestExtractWikilinks_AdjacentLinks(t *testing.T) {
	content := "[[LinkA]][[LinkB]]"
	links := ExtractWikilinks(content)
	expected := []string{"LinkA", "LinkB"}
	assert.Equal(t, expected, links)
}

func TestExtractWikilinks_WithSpaces(t *testing.T) {
	content := "See [[My Great Note]] for details."
	links := ExtractWikilinks(content)
	assert.Equal(t, []string{"My Great Note"}, links)
}

func TestExtractWikilinks_Unclosed(t *testing.T) {
	content := "This has [[unclosed link"
	links := ExtractWikilinks(content)
	assert.Empty(t, links)
}

func TestResolveWikilink(t *testing.T) {
	assert.Equal(t, "Note.md", ResolveWikilink("Note"))
	assert.Equal(t, "Another Note.md", ResolveWikilink("Another Note"))
	assert.Equal(t, "subdir/Note.md", ResolveWikilink("subdir/Note"))
}

func TestResolveWikilink_TrimsSpace(t *testing.T) {
	assert.Equal(t, "Note.md", ResolveWikilink("  Note  "))
}

func TestResolveWikilink_Empty(t *testing.T) {
	assert.Equal(t, ".md", ResolveWikilink(""))
}
