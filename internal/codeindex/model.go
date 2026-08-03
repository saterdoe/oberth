// Package codeindex implements Oberth's private, repository-scoped code index.
package codeindex

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const SchemaVersion = "1"

type SymbolKind string

const (
	KindFunction      SymbolKind = "function"
	KindMethod        SymbolKind = "method"
	KindClass         SymbolKind = "class"
	KindStruct        SymbolKind = "struct"
	KindInterface     SymbolKind = "interface"
	KindType          SymbolKind = "type"
	KindModule        SymbolKind = "module"
	KindConfiguration SymbolKind = "configuration"
	KindFile          SymbolKind = "file"
	KindUnknown       SymbolKind = "unknown"
)

type Chunk struct {
	ID                  string     `json:"id"`
	RepoID              string     `json:"repo_id"`
	Path                string     `json:"path"`
	Language            string     `json:"language"`
	Content             string     `json:"content"`
	FileHash            string     `json:"file_hash"`
	ChunkHash           string     `json:"chunk_hash"`
	Ordinal             int        `json:"ordinal"`
	Symbol              string     `json:"symbol,omitempty"`
	SymbolKind          SymbolKind `json:"symbol_kind"`
	StartLine           int        `json:"start_line"`
	EndLine             int        `json:"end_line"` // one-based, inclusive
	IsTest              bool       `json:"is_test"`
	Commit              string     `json:"commit,omitempty"`
	SchemaVersion       string     `json:"schema_version"`
	EmbedderFingerprint string     `json:"embedder_fingerprint"`
}

type Signal struct {
	Kind  string  `json:"kind"`
	Score float64 `json:"score"`
	Rank  int     `json:"rank"`
}
type Result struct {
	Chunk   Chunk    `json:"chunk"`
	Score   float64  `json:"score"`
	Signals []Signal `json:"signals"`
	Reason  string   `json:"reason"`
}

type Metrics struct {
	Discovered      int           `json:"discovered"`
	Indexed         int           `json:"indexed"`
	Skipped         int           `json:"skipped"`
	Failed          int           `json:"failed"`
	Created         int           `json:"created"`
	Reused          int           `json:"reused"`
	Deleted         int           `json:"deleted"`
	EmbeddingHits   int           `json:"embedding_hits"`
	EmbeddingMisses int           `json:"embedding_misses"`
	Duration        time.Duration `json:"duration"`
}

type Status struct {
	SchemaVersion string    `json:"schema_version"`
	RepoID        string    `json:"repo_id"`
	IndexedFiles  int       `json:"indexed_files"`
	ChunkCount    int       `json:"chunk_count"`
	LastIndexed   time.Time `json:"last_indexed"`
	Fresh         bool      `json:"fresh"`
	LastError     string    `json:"last_error,omitempty"`
	Metrics       Metrics   `json:"metrics"`
}

func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
func RepositoryID(root string) (string, error) {
	a, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	a, e = filepath.EvalSymlinks(a)
	if e != nil {
		return "", e
	}
	return "repo:" + hash(strings.ToLower(filepath.Clean(a)))[:24], nil
}
func chunkID(repoID, contentHash, symbol string, start int) string {
	return "code:" + hash(repoID, contentHash, symbol, fmt.Sprint(start))
}
