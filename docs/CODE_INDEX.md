# Oberth repository code index

Oberth builds a private, repository-scoped code index while compiling task
context. It augments lexical and symbol search, repository instructions, and
Vault memory; it does not replace them.

## Architecture and persistence

`internal/codeindex` separates discovery, chunk extraction, embedding,
incremental state, vector persistence, and rank fusion. Each repository gets a
deterministic ID from its canonical root. Code vector IDs start with `code:`
and live in the OS user cache under `oberth/code-index/<repo-id>/`. Vault
vectors remain in their existing collection, so code reindexing cannot delete
or contaminate memory.

The state schema is versioned. Content hashes determine new, changed,
unchanged, and deleted files; timestamps are diagnostic only. Chunk IDs combine
repository identity, exact content hash, symbol, and starting line. State is
published through a temporary file and atomic replacement.

The zero-configuration embedder and local vector store are the default. The
built-in feature-hashing embedder is an offline baseline, not a specialized
code model. The index consumes Oberth's existing embedding and vector-store
contracts, allowing an explicitly configured alternative backend.

## Retrieval and context

Candidates come from explicit paths, mentioned symbols, lexical terms, and
vector similarity. Weighted reciprocal-rank fusion is deterministic. Stable
chunk IDs deduplicate results and a per-file cap maintains diversity. Each
result contains its contributing signals and a readable reason.

Complete chunks, paths, symbols, and one-based inclusive line ranges enter the
existing context compiler. Its token and per-kind budgets make the final
selection and record selection and exclusion reasons in the manifest. When
embedding or vector search fails, chunk metadata and lexical retrieval remain
available.

## Discovery and privacy

Discovery canonicalizes the repository root, never follows symlinks, and emits
portable relative paths. It excludes dependency, build, cache and coverage
directories; binary/media formats; generated and minified files; lockfiles;
local databases; `.env` variants; credentials; and private keys. Defaults limit
files to 512 KiB, repositories to 5,000 files and 20,000 chunks, and chunks to
240 lines with 20 lines of overlap.

Source is sensitive. Local embeddings are the safe default. Choosing an HTTP
or other remote embedder explicitly permits eligible chunk text to leave the
machine. Excluded secrets are never submitted. Logs and status contain counts
and hashes, not source or full vectors.

## Extraction and configuration

Isolated heuristic extractors recognize functions, methods, classes,
interfaces, structs, and types in Go, Java, C#, TypeScript, JavaScript,
TSX/JSX, and Python. Markdown and configuration files use document chunks;
unknown languages use a bounded textual fallback. This avoids native-parser
distribution risk in alpha.2 while leaving a small extractor contract to
replace later.

```yaml
code_index:
  enabled: true
  max_file_bytes: 524288
  max_files: 5000
  max_chunks: 20000
  max_chunk_lines: 240
  overlap_lines: 20
  exclude: []
```

Task compilation refreshes the index incrementally at its existing reliable
repository-context boundary. A permanent filesystem watcher is not required.

The Settings screen reports freshness, indexed file and chunk counts, the last
refresh and actionable errors for every authorized project. It also provides a
manual reindex action backed by:

To keep this section fast as the project catalog grows, the UI shows the first
four projects in the API's stable order and reveals four more at a time. “Show
less” returns to the initial view. This is presentation-only: status is fetched
for every authorized project and indexing behavior is unaffected.

- `GET /api/v1/projects/{id}/code-index`
- `POST /api/v1/projects/{id}/code-index/reindex`

Both routes resolve the repository from Oberth's project database; callers
cannot supply an arbitrary filesystem path.
