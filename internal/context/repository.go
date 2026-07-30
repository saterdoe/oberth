package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saterdoe/oberth/internal/repoanalyzer"
)

func (p *Pipeline) CompileRepository(ctx context.Context, root, query, taskType string, opts CompileOptions) (*CompileResult, error) {
	analysis, err := repoanalyzer.Analyze(root, repoanalyzer.Options{MaxFiles: 1000})
	if err != nil {
		return nil, err
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
	opts.RepoSources = append(sources, opts.RepoSources...)
	return p.CompileWithOptions(ctx, query, taskType, opts)
}
