package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saterdoe/oberth/internal/codeindex"
	"github.com/saterdoe/oberth/internal/repoanalyzer"
)

func (p *Pipeline) CompileRepository(ctx context.Context, root, query, taskType string, opts CompileOptions) (*CompileResult, error) {
	analysis, err := repoanalyzer.Analyze(root, repoanalyzer.Options{MaxFiles: 1000})
	if err != nil {
		return nil, err
	}
	// The code index has its own repository-scoped store. Failure to initialize
	// embeddings never prevents the existing lexical repository path.
	var codeResults []codeindex.Result
	var embedder codeindex.Embedder
	if p != nil && p.searcher != nil {
		embedder = p.searcher.Embedder()
	}
	indexOptions := opts.CodeIndex
	if indexOptions.MaxFiles <= 0 {
		indexOptions = codeindex.DefaultOptions()
	}
	if !opts.DisableCodeIndex {
		identityRoot := opts.CodeIndexIdentityRoot
		if identityRoot == "" {
			identityRoot = root
		}
		if index, indexErr := codeindex.OpenLocalWithIdentity(root, identityRoot, embedder, indexOptions); indexErr == nil {
			_, _ = index.Update(ctx)
			codeResults, _ = index.Search(ctx, query, 24)
		}
	}
	matches, err := repoanalyzer.Search(root, query, repoanalyzer.SearchOptions{Limit: 12})
	if err != nil {
		return nil, err
	}
	metadata := fmt.Sprintf("Primary language: %s\nPackage manager: %s\nManifests: %s\nEntrypoints: %s", analysis.Metadata.PrimaryLanguage, analysis.Metadata.PackageManager, strings.Join(analysis.Metadata.Manifests, ", "), strings.Join(analysis.Metadata.Entrypoints, ", "))
	sources := []ContextSource{{ID: "repository-metadata", Kind: "metadata", Content: metadata}, {ID: "repository-map", Kind: "repo_map", Content: analysis.Map}}
	for _, rulePath := range []string{"AGENTS.md", "CLAUDE.md", ".cursorrules", filepath.Join(".cursor", "rules")} {
		fullPath := filepath.Join(root, rulePath)
		info, statErr := os.Stat(fullPath)
		if statErr != nil || info.IsDir() || info.Size() > 256*1024 {
			continue
		}
		if content, readErr := os.ReadFile(fullPath); readErr == nil {
			sources = append(sources, ContextSource{ID: filepath.ToSlash(rulePath), Kind: "rules", Content: string(content), Priority: 1000, Reason: "repository-native agent instructions"})
		}
	}
	for _, match := range matches {
		sources = append(sources, ContextSource{ID: fmt.Sprintf("%s:%d", match.Path, match.Line), Kind: "code", Content: fmt.Sprintf("%s:%d\n%s", match.Path, match.Line, match.Text), TaskTypes: []string{taskType}})
	}
	for _, result := range codeResults {
		c := result.Chunk
		kind, priority := "code", 20
		if c.Language == "markdown" {
			kind, priority = "documentation", 5
		} else if c.Language == "configuration" {
			kind, priority = "configuration", 10
		}
		sources = append(sources, ContextSource{ID: fmt.Sprintf("%s:%d-%d:%s", c.Path, c.StartLine, c.EndLine, c.Symbol), Kind: kind, Content: codeindex.FormatResult(result), TaskTypes: []string{taskType}, Priority: priority, Relevance: result.Score, Reason: result.Reason})
	}
	opts.RepoSources = append(sources, opts.RepoSources...)
	return p.CompileWithOptions(ctx, query, taskType, opts)
}
