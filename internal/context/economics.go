package context

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type ContextMode string

const (
	ModeDev      ContextMode = "dev"
	ModeReview   ContextMode = "review"
	ModeResearch ContextMode = "research"
)

type ContextProfile struct {
	Mode                ContextMode `json:"mode"`
	MaxTokens           int         `json:"max_tokens"`
	ReserveOutputTokens int         `json:"reserve_output_tokens"`
	MaxExpansionRounds  int         `json:"max_expansion_rounds"`
	MaxSourcesPerKind   int         `json:"max_sources_per_kind"`
	PreferredKinds      []string    `json:"preferred_kinds"`
	AllowedTools        []string    `json:"allowed_tools"`
}

func ProfileForMode(mode ContextMode) (ContextProfile, error) {
	switch mode {
	case ModeDev:
		return ContextProfile{mode, 4000, 1000, 2, 6, []string{"code", "metadata", "repo_map", "memory", "dependency"}, []string{"file.read", "file.write", "search", "command.exec"}}, nil
	case ModeReview:
		return ContextProfile{mode, 6000, 1500, 3, 5, []string{"diff", "test", "security", "code", "memory"}, []string{"file.read", "search", "verification.run", "security.review"}}, nil
	case ModeResearch:
		return ContextProfile{mode, 8000, 2000, 3, 8, []string{"docs", "memory", "repo_map", "code"}, []string{"file.read", "search", "vault.search"}}, nil
	default:
		return ContextProfile{}, fmt.Errorf("unknown context mode %q", mode)
	}
}

type cacheEntry struct {
	result    CompileResult
	expiresAt time.Time
	usedAt    time.Time
}

type CompilationCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	entries    map[string]cacheEntry
}

func NewCompilationCache(maxEntries int, ttl time.Duration) *CompilationCache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CompilationCache{maxEntries: maxEntries, ttl: ttl, entries: map[string]cacheEntry{}}
}

func (c *CompilationCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]cacheEntry{}
}

func (c *CompilationCache) get(key string) (CompileResult, bool) {
	if c == nil {
		return CompileResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return CompileResult{}, false
	}
	entry.usedAt = time.Now()
	c.entries[key] = entry
	return cloneCompileResult(entry.result), true
}

func (c *CompilationCache) put(key string, result CompileResult) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if len(c.entries) >= c.maxEntries {
		keys := make([]string, 0, len(c.entries))
		for existing := range c.entries {
			keys = append(keys, existing)
		}
		sort.Slice(keys, func(i, j int) bool { return c.entries[keys[i]].usedAt.Before(c.entries[keys[j]].usedAt) })
		delete(c.entries, keys[0])
	}
	c.entries[key] = cacheEntry{result: cloneCompileResult(result), expiresAt: now.Add(c.ttl), usedAt: now}
}

func compilationCacheKey(query, taskType string, opts CompileOptions, sources []ContextSource) string {
	payload := struct {
		Query, TaskType string
		Mode            ContextMode
		Max, Reserve    int
		PerKind         int
		Sources         []ContextSource
	}{query, taskType, opts.Mode, opts.MaxTokens, opts.ReserveOutputTokens, opts.MaxSourcesPerKind, sources}
	data, _ := json.Marshal(payload)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func cloneCompileResult(source CompileResult) CompileResult {
	clone := source
	clone.Sources = append([]string(nil), source.Sources...)
	clone.Manifest = append([]SourceSelection(nil), source.Manifest...)
	clone.Metrics.SelectedByKind = cloneIntMap(source.Metrics.SelectedByKind)
	clone.Metrics.CandidateByKind = cloneIntMap(source.Metrics.CandidateByKind)
	return clone
}

func cloneIntMap(source map[string]int) map[string]int {
	result := map[string]int{}
	for key, value := range source {
		result[key] = value
	}
	return result
}
