package codeindex

import (
	"regexp"
	"strings"
)

type declaration struct {
	re   *regexp.Regexp
	kind SymbolKind
}

var declarations = []declaration{
	{regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)`), KindFunction},
	{regexp.MustCompile(`^\s*(?:public|private|protected|static|final|abstract|async|export|default|internal|sealed|override|virtual|partial|\s)*\s*(?:class)\s+([A-Za-z_$][\w$]*)`), KindClass},
	{regexp.MustCompile(`^\s*(?:public|private|protected|export|internal|\s)*\s*interface\s+([A-Za-z_$][\w$]*)`), KindInterface},
	{regexp.MustCompile(`^\s*(?:public|private|export|internal|\s)*\s*(?:struct)\s+([A-Za-z_]\w*)`), KindStruct},
	{regexp.MustCompile(`^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)`), KindFunction},
	{regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)`), KindFunction},
	{regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][\w$]*)`), KindType},
	{regexp.MustCompile(`^\s*(?:public|private|protected|static|async|final|synchronized|override|virtual|\s)+[A-Za-z_<>,\[\]?]+\s+([A-Za-z_$][\w$]*)\s*\(`), KindMethod},
}

func ChunkFile(repoID string, f File, o Options, embedder string) []Chunk {
	lines := strings.Split(strings.ReplaceAll(string(f.Content), "\r\n", "\n"), "\n")
	starts := []int{0}
	names := map[int]string{}
	kinds := map[int]SymbolKind{}
	for i, line := range lines {
		for _, d := range declarations {
			if m := d.re.FindStringSubmatch(line); len(m) > 1 {
				declarationStart := i
				for declarationStart > 0 && isLeadingComment(lines[declarationStart-1]) {
					declarationStart--
				}
				if declarationStart > 0 {
					starts = append(starts, declarationStart)
				}
				names[declarationStart] = m[1]
				kinds[declarationStart] = d.kind
				break
			}
		}
	}
	starts = uniqueSorted(starts)
	var out []Chunk
	for n, start := range starts {
		end := len(lines)
		if n+1 < len(starts) {
			end = starts[n+1]
		}
		maxLines := o.MaxChunkLines
		if maxLines <= 0 {
			maxLines = 240
		}
		overlap := o.OverlapLines
		if overlap < 0 {
			overlap = 0
		}
		for pos := start; pos < end; {
			stop := min(pos+maxLines, end)
			content := strings.TrimSpace(strings.Join(lines[pos:stop], "\n"))
			if content != "" {
				ch := hash(content)
				kind := kinds[start]
				if kind == "" {
					if f.Language == "configuration" {
						kind = KindConfiguration
					} else {
						kind = KindFile
					}
				}
				out = append(out, Chunk{ID: chunkID(repoID, ch, names[start], pos+1), RepoID: repoID, Path: f.Path, Language: f.Language, Content: content, FileHash: f.Hash, ChunkHash: ch, Ordinal: len(out), Symbol: names[start], SymbolKind: kind, StartLine: pos + 1, EndLine: stop, IsTest: f.IsTest, SchemaVersion: SchemaVersion, EmbedderFingerprint: embedder})
			}
			if stop == end {
				break
			}
			pos = stop - overlap
		}
	}
	return out
}
func isLeadingComment(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "//") || strings.HasPrefix(s, "#") || strings.HasPrefix(s, "*") || strings.HasPrefix(s, "/*")
}
func uniqueSorted(v []int) []int {
	out := v[:0]
	last := -1
	for _, x := range v {
		if x != last {
			out = append(out, x)
			last = x
		}
	}
	return out
}
