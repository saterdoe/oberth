# Changelog

All notable changes follow Keep a Changelog and Semantic Versioning.

## Unreleased

### Added

- A versioned persisted-data migration policy, recoverable pre-migration
  backups, and automated prior-version result-bundle migration tests.
- Durable interrupted-run checkpoints and audited, idempotent lease recovery
  that preserves worktrees and skips confirmed external effects.

## 0.1.0-alpha.3

### Added

- Multi-project Code Workspace with CodeMirror tabs and directory navigation.
- Real-time session and Vault updates over WebSocket, plus progressive project
  index disclosure and safe closure of blocked or failed tasks.
- A bounded shared context envelope for task execution.
- Supported architecture matrix for Linux, macOS and Windows CLI/service builds.
- Reproducible GitHub Release assets with licenses, notices, checksums and SBOMs.
- Native packaged-artifact smoke tests and a reproducible, install-tested VSIX.
- A release runbook covering verification, publication, rollback and evidence.

### Changed

- The version contract now keeps application, extension, documentation and
  platform metadata synchronized from the canonical `VERSION` file.
- CI now exercises portable setup on Linux and macOS, quality gates, race and
  durable E2E tests, architecture smokes and VS Code extension tests.

### Fixed

- Project opening no longer leaves the explorer stuck loading.
- Locale switching translates application chrome without modifying user or
  model messages, repository paths, identifiers, code, or technical payloads.
- Clean GitHub Actions checkouts no longer require generated desktop assets.

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
