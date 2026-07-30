package vault

import (
	"regexp"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// ExtractWikilinks extracts [[wikilink]] references from markdown content.
// Returns the link targets without the [[ ]] delimiters.
func ExtractWikilinks(content string) []string {
	matches := wikilinkRe.FindAllStringSubmatch(content, -1)
	links := make([]string, 0, len(matches))
	for _, m := range matches {
		links = append(links, m[1])
	}
	return links
}

// ResolveWikilink resolves a wikilink to a file path by trimming
// surrounding whitespace and appending the .md extension.
func ResolveWikilink(wikilink string) string {
	return strings.TrimSpace(wikilink) + ".md"
}
