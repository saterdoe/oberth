package context

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

type ContextSource struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Content   string   `json:"content"`
	TaskTypes []string `json:"task_types,omitempty"`
	Priority  int      `json:"priority,omitempty"`
	Relevance float64  `json:"relevance,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

type CompileOptions struct {
	MaxTokens           int
	ReserveOutputTokens int
	RepoSources         []ContextSource
	Expand              func(context.Context, int) ([]ContextSource, error)
	MaxExpansionRounds  int
	MaxSourcesPerKind   int
	Mode                ContextMode
	Cache               *CompilationCache
}

type SourceSelection struct {
	ID        string  `json:"id"`
	Kind      string  `json:"kind"`
	Hash      string  `json:"hash"`
	Reason    string  `json:"reason"`
	Tokens    int     `json:"tokens"`
	Score     float64 `json:"score"`
	Compacted bool    `json:"compacted"`
}

type CompileMetrics struct {
	Candidates               int            `json:"candidates"`
	Selected                 int            `json:"selected"`
	Dropped                  int            `json:"dropped"`
	DuplicateDropped         int            `json:"duplicate_dropped"`
	CandidateTokens          int            `json:"candidate_tokens"`
	SelectedTokens           int            `json:"selected_tokens"`
	SavedTokens              int            `json:"saved_tokens"`
	SavingsPercent           float64        `json:"savings_percent"`
	BudgetUtilizationPercent float64        `json:"budget_utilization_percent"`
	ReservedOutputTokens     int            `json:"reserved_output_tokens"`
	ExpansionRounds          int            `json:"expansion_rounds"`
	Compacted                int            `json:"compacted"`
	CacheHit                 bool           `json:"cache_hit"`
	CacheHits                int            `json:"cache_hits"`
	CacheMisses              int            `json:"cache_misses"`
	Estimator                string         `json:"estimator"`
	SelectedByKind           map[string]int `json:"selected_by_kind"`
	CandidateByKind          map[string]int `json:"candidate_by_kind"`
}

type rankedSource struct {
	ContextSource
	score float64
}

func (p *Pipeline) CompileWithOptions(ctx context.Context, query, taskType string, opts CompileOptions) (*CompileResult, error) {
	if opts.Mode != "" {
		profile, err := ProfileForMode(opts.Mode)
		if err != nil {
			return nil, err
		}
		if opts.MaxTokens <= 0 {
			opts.MaxTokens = profile.MaxTokens
		}
		if opts.ReserveOutputTokens <= 0 {
			opts.ReserveOutputTokens = profile.ReserveOutputTokens
			if opts.MaxTokens > 0 && opts.ReserveOutputTokens >= opts.MaxTokens {
				opts.ReserveOutputTokens = opts.MaxTokens / 4
			}
		}
		if opts.MaxExpansionRounds <= 0 {
			opts.MaxExpansionRounds = profile.MaxExpansionRounds
		}
		if opts.MaxSourcesPerKind <= 0 {
			opts.MaxSourcesPerKind = profile.MaxSourcesPerKind
		}
	}
	max := opts.MaxTokens
	if max <= 0 {
		max = 2000
	}
	reserve := opts.ReserveOutputTokens
	if reserve < 0 {
		reserve = 0
	}
	budget := max - reserve
	if budget < 1 {
		budget = 1
	}

	candidates := make([]ContextSource, 0, len(opts.RepoSources)+1)
	if memory := p.readMemoryIndex(ctx); memory != "" {
		candidates = append(candidates, ContextSource{ID: "memory-index", Kind: "memory", Content: memory, Priority: 60, Relevance: .7, Reason: "project memory index"})
	}
	candidates = append(candidates, opts.RepoSources...)

	maxRounds := opts.MaxExpansionRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}
	expansionRounds := 0
	if opts.Expand != nil {
		seenExpansion := map[string]bool{}
		for round := 0; round < maxRounds; round++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			expanded, err := opts.Expand(ctx, budget)
			if err != nil {
				return nil, err
			}
			expansionRounds++
			added := 0
			for _, source := range expanded {
				fingerprint := source.ID + ":" + contentFingerprint(source.Content)
				if seenExpansion[fingerprint] {
					continue
				}
				seenExpansion[fingerprint] = true
				candidates = append(candidates, source)
				added++
			}
			if added == 0 {
				break
			}
		}
	}

	cacheKey := compilationCacheKey(query, taskType, opts, candidates)
	if opts.Expand == nil {
		if cached, ok := opts.Cache.get(cacheKey); ok {
			cached.Metrics.CacheHit = true
			cached.Metrics.CacheHits++
			return &cached, nil
		}
	}

	metrics := CompileMetrics{
		ReservedOutputTokens: reserve,
		ExpansionRounds:      expansionRounds,
		Estimator:            "chars/4-v1",
		SelectedByKind:       map[string]int{},
		CandidateByKind:      map[string]int{},
	}
	if opts.Cache != nil {
		metrics.CacheMisses = 1
	}
	ranked := rankAndDeduplicate(candidates, query, taskType, opts.Mode, &metrics)
	candidateParts := make([]string, 0, len(ranked))
	for _, source := range ranked {
		candidateParts = append(candidateParts, source.Content)
	}
	// Candidate and selected token counts must use the same serialized
	// representation, including separators, so savings cannot be distorted by
	// formatting overhead.
	metrics.CandidateTokens = estimateTokens(strings.Join(candidateParts, "\n\n---\n\n"))
	result := CompileResult{SchemaVersion: "1", Level: determineLevel(query, taskType)}
	parts := []string{}
	selectedKindCounts := map[string]int{}

	for _, source := range ranked {
		if opts.MaxSourcesPerKind > 0 && selectedKindCounts[source.Kind] >= opts.MaxSourcesPerKind {
			metrics.Dropped++
			result.Exclusions = append(result.Exclusions, ContextExclusion{ID: source.ID, Kind: source.Kind, Reason: "source-kind diversity limit"})
			continue
		}
		used := estimateTokens(strings.Join(parts, "\n\n---\n\n"))
		remaining := budget - used
		content := source.Content
		compacted := false
		if estimateTokens(content) > remaining {
			content = compactSource(content, remaining)
			compacted = content != ""
		}
		if content == "" {
			metrics.Dropped++
			result.Exclusions = append(result.Exclusions, ContextExclusion{ID: source.ID, Kind: source.Kind, Reason: "insufficient remaining token budget"})
			continue
		}
		proposed := strings.Join(append(append([]string{}, parts...), content), "\n\n---\n\n")
		if estimateTokens(proposed) > budget {
			metrics.Dropped++
			result.Exclusions = append(result.Exclusions, ContextExclusion{ID: source.ID, Kind: source.Kind, Reason: "token budget exceeded"})
			continue
		}
		parts = append(parts, content)
		result.Sources = append(result.Sources, source.ID)
		tokens := estimateTokens(content)
		reason := strings.TrimSpace(source.Reason)
		if reason == "" {
			reason = defaultSelectionReason(source.ContextSource, query)
		}
		result.Manifest = append(result.Manifest, SourceSelection{source.ID, source.Kind, contentFingerprint(content), reason, tokens, source.score, compacted})
		selectedKindCounts[source.Kind]++
		metrics.SelectedByKind[source.Kind]++
		metrics.Selected++
		if compacted {
			metrics.Compacted++
		}
	}

	result.Context = strings.TrimSpace(strings.Join(parts, "\n\n---\n\n"))
	result.Tokens = estimateTokens(result.Context)
	metrics.SelectedTokens = result.Tokens
	metrics.SavedTokens = metrics.CandidateTokens - metrics.SelectedTokens
	if metrics.SavedTokens < 0 {
		metrics.SavedTokens = 0
	}
	if metrics.CandidateTokens > 0 {
		metrics.SavingsPercent = float64(metrics.SavedTokens) * 100 / float64(metrics.CandidateTokens)
	}
	metrics.BudgetUtilizationPercent = float64(metrics.SelectedTokens) * 100 / float64(budget)
	result.Metrics = metrics
	if opts.Expand == nil {
		opts.Cache.put(cacheKey, result)
	}
	return &result, nil
}

func rankAndDeduplicate(sources []ContextSource, query, taskType string, mode ContextMode, metrics *CompileMetrics) []rankedSource {
	seenID := map[string]bool{}
	seenContent := map[string]bool{}
	terms := queryTermsForRanking(query)
	preferred := map[string]bool{}
	if mode != "" {
		if profile, err := ProfileForMode(mode); err == nil {
			for _, kind := range profile.PreferredKinds {
				preferred[kind] = true
			}
		}
	}
	ranked := make([]rankedSource, 0, len(sources))
	for _, source := range sources {
		metrics.Candidates++
		metrics.CandidateByKind[source.Kind]++
		if !sourceAllowed(source, taskType) || strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Content) == "" {
			metrics.Dropped++
			continue
		}
		fingerprint := contentFingerprint(source.Content)
		if seenID[source.ID] || seenContent[fingerprint] {
			metrics.DuplicateDropped++
			metrics.Dropped++
			continue
		}
		seenID[source.ID] = true
		seenContent[fingerprint] = true
		score := float64(source.Priority)*1000 + source.Relevance*100
		lower := strings.ToLower(source.Content)
		sourcePath := strings.ToLower(strings.Split(source.ID, ":")[0])
		if sourcePath != "" && strings.Contains(strings.ToLower(query), sourcePath) {
			score += 1_000_000
			if source.Reason == "" {
				source.Reason = "explicitly mentioned by the task"
			}
		}
		for _, term := range terms {
			score += float64(strings.Count(lower, term) * 10)
		}
		if preferred[source.Kind] {
			score += 50
		}
		ranked = append(ranked, rankedSource{source, score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].ID < ranked[j].ID
		}
		return ranked[i].score > ranked[j].score
	})
	return ranked
}

func sourceAllowed(source ContextSource, taskType string) bool {
	if len(source.TaskTypes) == 0 || taskType == "" {
		return true
	}
	for _, value := range source.TaskTypes {
		if value == taskType {
			return true
		}
	}
	return false
}

func contentFingerprint(content string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(content)), " ")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
}

func queryTermsForRanking(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	seen := map[string]bool{}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}\"'")
		if len(field) < 3 || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}

func defaultSelectionReason(source ContextSource, query string) string {
	if source.Relevance > 0 {
		return fmt.Sprintf("ranked relevance %.2f for query %q", source.Relevance, query)
	}
	if source.Priority > 0 {
		return fmt.Sprintf("source priority %d", source.Priority)
	}
	return "eligible source within token budget"
}

func compactSource(content string, maxTokens int) string {
	if maxTokens < 8 {
		return ""
	}
	maxChars := maxTokens*4 - 20
	if maxChars < 20 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}
	marker := "\n... compacted ...\n"
	room := maxChars - len(marker)
	if room < 2 {
		return ""
	}
	head := room * 2 / 3
	tail := room - head
	return content[:head] + marker + content[len(content)-tail:]
}
