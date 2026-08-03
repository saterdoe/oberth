# Changelog

All notable changes follow Keep a Changelog and Semantic Versioning.

## Unreleased

### Added

- Multi-project Code Workspace with CodeMirror tabs and directory navigation.
- Real-time session and Vault updates over WebSocket.
- Progressive disclosure for project code-index status: four projects are
  shown initially and additional projects load in small batches.
- Blocked and failed tasks can be closed without losing their recorded history.

### Fixed

- Project opening no longer leaves the explorer stuck loading.
- Locale switching now translates application chrome without modifying user or
  model messages, repository paths, identifiers, code, or technical payloads.
- Clean GitHub Actions checkouts no longer require generated desktop assets for
  the cross-platform Go test job.

## 0.1.0-alpha.2

### Added

- Private, incremental repository code index with symbol-aware chunking.
- Hybrid path, symbol, lexical and semantic retrieval for task context.
- Repository-scoped vector persistence, explainable ranking and lexical fallback.
- Secret, binary, generated-file and repository exclusion policies for indexing.

### Changed

- Repository context compilation now incorporates complete ranked code chunks
  under the existing token and source-diversity budgets.

## 0.1.0-alpha.1

First Public Alpha of the local-first coding agent runtime.

### Included

- Isolated Git worktrees for agent tasks.
- Reviewable diffs, command history and verification evidence.
- Explicit approve, correct and reject decisions.
- Native Windows desktop app plus CLI and local service.
- Local and OpenAI-compatible provider support.

### Alpha limitations

- Windows receives the most desktop release testing.
- Public interfaces and persisted data may change between alpha releases.
- Multi-user, remote administration and hosted operation are out of scope.
