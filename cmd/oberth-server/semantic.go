package main

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/saterdoe/oberth/internal/config"
	semcontext "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/mcp"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/saterdoe/oberth/pkg/vector"
)

func buildEmbedder(cfg *config.Config) (semcontext.Embedder, int) {
	dimensions := cfg.VectorDB.Embedder.Dimensions
	if dimensions <= 0 {
		dimensions = 384
	}

	var embedder semcontext.Embedder
	switch cfg.VectorDB.Embedder.Provider {
	case "", "builtin":
		embedder = semcontext.NewBuiltinEmbedder(dimensions)
	case "http":
		embedder = semcontext.NewHTTPEmbedder(cfg.Python.URL, dimensions)
	case "disabled":
		return nil, dimensions
	default:
		slog.Warn("unknown embedder provider, using built-in embedder", "provider", cfg.VectorDB.Embedder.Provider)
		embedder = semcontext.NewBuiltinEmbedder(dimensions)
	}
	if cfg.VectorDB.Embedder.CachePath == "" {
		return embedder, dimensions
	}
	cached, err := semcontext.NewCachedEmbedder(embedder, cfg.VectorDB.Embedder.CachePath)
	if err != nil {
		slog.Warn("embedding cache unavailable; continuing without persistence", "error", err)
		return embedder, dimensions
	}
	return cached, dimensions
}

func buildVectorStore(cfg *config.Config, dimensions int) vector.VectorStore {
	switch cfg.VectorDB.Engine {
	case "", "builtin", "local":
		store, err := vector.NewLocalStore(vectorStorePath(cfg), dimensions)
		if err != nil {
			slog.Error("failed to initialize built-in vector store", "error", err)
			return nil
		}
		slog.Info("vector store initialized", "engine", "builtin", "path", cfg.VectorDB.Local.Path)
		return store
	case "qdrant":
		store := vector.NewQdrantStore(cfg.VectorDB.Qdrant.Collection, dimensions, vector.WithBaseURL(cfg.VectorDB.Qdrant.URL))
		slog.Info("vector store initialized", "engine", "qdrant", "collection", cfg.VectorDB.Qdrant.Collection)
		return store
	case "chromadb":
		var options []vector.ChromaDBOption
		if cfg.VectorDB.ChromaDB.Path != "" {
			options = append(options, vector.WithChromaBaseURL(cfg.VectorDB.ChromaDB.Path))
		}
		store := vector.NewChromaDBStore(cfg.VectorDB.ChromaDB.Collection, dimensions, options...)
		slog.Info("vector store initialized", "engine", "chromadb", "collection", cfg.VectorDB.ChromaDB.Collection)
		return store
	case "disabled":
		// Keep a standby local store for status and future re-enablement, while
		// callers deliberately exclude it from active semantic search.
		store, err := vector.NewLocalStore(vectorStorePath(cfg), dimensions)
		if err != nil {
			slog.Warn("standby vector store unavailable", "error", err)
			return nil
		}
		return store
	default:
		slog.Warn("unknown vector store engine, disabling semantic search", "engine", cfg.VectorDB.Engine)
		return nil
	}
}

func vectorStorePath(cfg *config.Config) string {
	if cfg.VectorDB.Local.Path != "" {
		return cfg.VectorDB.Local.Path
	}
	return filepath.Join(filepath.Dir(cfg.Vault.Path), "vector", "index.json")
}

func buildMCPServer(vaultConn *vault.Vault, searcher *semcontext.Searcher, pipeline *semcontext.Pipeline) *mcp.Server {
	server := mcp.NewServer()
	options := &mcp.VaultToolsOptions{}
	if searcher != nil {
		options.SearchFn = func(ctx context.Context, query string, limit int, taskType string) ([]map[string]any, error) {
			results, err := searcher.Search(ctx, query, limit, taskType)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, len(results))
			for i, result := range results {
				out[i] = map[string]any{"note": result.Note, "score": result.Score}
			}
			return out, nil
		}
	}
	if pipeline != nil {
		options.CompileContextFn = func(ctx context.Context, query string, taskType string) (any, error) {
			return pipeline.Compile(ctx, query, taskType)
		}
	}
	tools := mcp.NewVaultTools(vaultConn, options)
	for _, tool := range tools {
		server.RegisterTool(tool)
	}
	slog.Info("mcp server initialized", "tool_count", len(tools))
	return server
}
