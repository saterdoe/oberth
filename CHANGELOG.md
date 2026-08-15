# Changelog

All notable changes follow Keep a Changelog and Semantic Versioning.

## Unreleased

## 0.1.0-alpha.7 - 2026-08-14

### Added

- Sandboxed command execution with a minimal inherited environment, resource
  quotas and verification evidence.
- Provider egress allowlists and SSRF defenses for local, private and
  metadata-service destinations.
- A dedicated provider-secret encryption key lifecycle independent from the
  local daemon authentication token.
- Serializable, crash-recoverable cost reservations that prevent concurrent
  runs from overspending a shared budget.
- Durable WebSocket event replay, explicit resynchronization and bounded
  backpressure for reconnecting clients.
- Keyboard workspace navigation, a localized shortcut reference and automated
  WCAG 2.2 AA checks for critical journeys and dialogs.

### Changed

- The local HTTP and WebSocket boundary now enforces stricter timeouts, request
  limits, origin validation and graceful shutdown behavior.
- Navigation, task composition, sessions, routes and settings now adapt to
  compact desktop windows without global horizontal scrolling.

### Migration and recovery

- Database migrations 27 and 28 add durable cost reservations and replayable
  event streams. They are additive and run through the existing transactional
  migration path; preserve the automatic pre-migration backup before rollback.
- Existing provider secrets remain readable while the dedicated key is created
  and persisted. Recovery must restore the provider-secret key together with
  the database rather than substituting the daemon token.

## 0.1.0-alpha.6 - 2026-08-12

### Added

- A versioned persisted-data migration policy, recoverable pre-migration
  backups, and automated prior-version result-bundle migration tests.
- Durable interrupted-run checkpoints and audited, idempotent lease recovery
  that preserves worktrees and skips confirmed external effects.
- Crash-recoverable workspace transactions with durable before-images,
  metadata restoration and fault-injected create, replace, rename and delete tests.
- Classified worktree lifecycle reconciliation with retention, dry-run reports,
  dirty-worktree quarantine and fail-closed protection for recoverable runs.
- Tamper-evident, correlated security audit chains with structural secret
  redaction and fail-closed recording for sensitive user decisions.
- Deterministic Chromium E2E coverage for critical English and Spanish user
  journeys, with screenshots and traces retained on failure.
- A confidence-oriented review workspace with explicit readiness, blocker and
  reviewed-file signals before repository promotion.
- Contextual recovery and provider guidance in English and Spanish, with
  versioned links to the relevant operating documentation.

### Changed

- Reviewed Git changes now promote through a repository-serialized, strict
  fast-forward that revalidates the base commit and approved diff hash.

### Fixed

- Release packaging now resolves the generated VSIX before activation and
  computes artifact checksums with the native SHA-256 tool on macOS.
- Release asset collection now ignores auxiliary dependency artifacts instead
  of interpreting them as incomplete platform packages.

## 0.1.0-alpha.5 - 2026-08-12 [withdrawn]

- Withdrawn before publication after the final collector misclassified an
  auxiliary dependency SBOM artifact as a platform package.

## 0.1.0-alpha.4 - 2026-08-12 [withdrawn]

- Withdrawn before artifact publication after cross-platform packaging checks
  exposed a literal VSIX glob and an unavailable macOS checksum command.

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
