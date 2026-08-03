package codeindex

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

func (i *Index) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		return []Result{}, nil
	}
	i.mu.RLock()
	chunks := make([]Chunk, 0, len(i.state.Chunks))
	for _, c := range i.state.Chunks {
		chunks = append(chunks, c)
	}
	i.mu.RUnlock()
	sort.Slice(chunks, func(a, b int) bool { return chunks[a].ID < chunks[b].ID })
	queryTerms := terms(query)
	lists := map[string][]Chunk{"path": {}, "symbol": {}, "lexical": {}, "semantic": {}}
	q := strings.ToLower(query)
	for _, c := range chunks {
		path, sym, body := strings.ToLower(c.Path), strings.ToLower(c.Symbol), strings.ToLower(c.Content)
		if mentioned(q, path) || mentioned(q, strings.ToLower(baseName(path))) {
			lists["path"] = append(lists["path"], c)
		}
		if sym != "" && mentioned(q, sym) {
			lists["symbol"] = append(lists["symbol"], c)
		}
		hits := 0
		for _, term := range queryTerms {
			hits += strings.Count(body, term) + 2*strings.Count(sym, term)
		}
		if hits > 0 {
			lists["lexical"] = append(lists["lexical"], c)
		}
	}
	sort.SliceStable(lists["lexical"], func(a, b int) bool {
		return lexicalScore(lists["lexical"][a], queryTerms) > lexicalScore(lists["lexical"][b], queryTerms)
	})
	if i.embedder != nil && i.store != nil {
		if v, err := i.embedder.Embed(ctx, query); err == nil {
			if found, err := i.store.Search(ctx, v, limit*5); err == nil {
				byID := map[string]Chunk{}
				for _, c := range chunks {
					byID[c.ID] = c
				}
				for _, r := range found {
					if c, ok := byID[r.ID]; ok {
						lists["semantic"] = append(lists["semantic"], c)
					}
				}
			}
		}
	}
	type fused struct {
		r     Result
		score float64
	}
	all := map[string]*fused{}
	weights := map[string]float64{"path": 5, "symbol": 4, "lexical": 2, "semantic": 1}
	for _, kind := range []string{"path", "symbol", "lexical", "semantic"} {
		list := lists[kind]
		for rank, c := range list {
			f := all[c.ID]
			if f == nil {
				f = &fused{r: Result{Chunk: c}}
				all[c.ID] = f
			}
			score := weights[kind] / float64(61+rank)
			f.score += score
			f.r.Signals = append(f.r.Signals, Signal{Kind: kind, Score: score, Rank: rank + 1})
		}
	}
	out := make([]Result, 0, len(all))
	storageIntent := false
	for _, term := range terms(q) {
		if term == "storage" || term == "stored" || term == "persist" || term == "persistence" || term == "database" {
			storageIntent = true
			break
		}
	}
	for _, f := range all {
		if f.r.Chunk.Language == "markdown" {
			f.score *= .8
		} else if !f.r.Chunk.IsTest {
			f.score *= 1.2
		}
		if storageIntent && strings.Contains(strings.ToLower(f.r.Chunk.Path), "storage/") {
			f.score *= 2.5
		}
		f.r.Score = f.score
		why := make([]string, len(f.r.Signals))
		for n, s := range f.r.Signals {
			why[n] = s.Kind
		}
		f.r.Reason = "selected by " + strings.Join(why, " + ")
		out = append(out, f.r)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Score == out[b].Score {
			return out[a].Chunk.ID < out[b].Chunk.ID
		}
		return out[a].Score > out[b].Score
	})
	perFile := i.options.PerFileLimit
	if perFile <= 0 {
		perFile = 4
	}
	counts := map[string]int{}
	selected := make([]Result, 0, limit)
	for _, r := range out {
		if counts[r.Chunk.Path] >= perFile && !mentioned(q, strings.ToLower(r.Chunk.Path)) {
			continue
		}
		counts[r.Chunk.Path]++
		selected = append(selected, r)
		if len(selected) >= limit {
			break
		}
	}
	return selected, nil
}

func terms(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' }) {
		if len([]rune(x)) > 2 && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	synonyms := map[string][]string{"guardado": {"storage", "store", "save", "persist", "persistence"}, "guardar": {"storage", "store", "save", "persist"}, "datos": {"data", "database", "storage"}, "donde": {"where"}, "dónde": {"where"}, "contexto": {"context"}, "fuentes": {"sources"}}
	base := append([]string(nil), out...)
	for _, term := range base {
		for _, extra := range synonyms[term] {
			if !seen[extra] {
				seen[extra] = true
				out = append(out, extra)
			}
		}
	}
	return out
}
func mentioned(q, s string) bool { return s != "" && strings.Contains(q, s) }
func baseName(p string) string   { parts := strings.Split(p, "/"); return parts[len(parts)-1] }
func lexicalScore(c Chunk, t []string) int {
	v := strings.ToLower(c.Content + " " + c.Symbol + " " + c.Symbol)
	path := strings.ToLower(c.Path)
	n := 0
	for _, x := range t {
		n += strings.Count(v, x)
		// A matching directory or filename is a stronger navigation signal than
		// an incidental word in a large source body.
		n += strings.Count(path, x) * 100
	}
	return n
}
func FormatResult(r Result) string {
	return fmt.Sprintf("%s:%d-%d [%s %s]\n%s", r.Chunk.Path, r.Chunk.StartLine, r.Chunk.EndLine, r.Chunk.SymbolKind, r.Chunk.Symbol, r.Chunk.Content)
}
