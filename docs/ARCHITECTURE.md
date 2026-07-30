# Architecture

Oberth is a local-first system. The desktop application, web interface and CLI
all use the same local Go service and versioned runtime contracts.

## Main components

- `cmd/oberth-server`: local HTTP and WebSocket service.
- `cmd/oberth`: command-line client and automation interface.
- `desktop`: native Wails host for the desktop experience.
- `ui`: React interface shared by desktop and browser-based development.
- `internal`: runtime, persistence, providers, permissions, tools, verification
  and recovery.
- `pkg`: Go packages intended for reuse outside their implementation package.
- `extensions`: optional editor integrations that delegate to the local
  runtime.

## Task lifecycle

1. A user selects a repository and describes an intended change.
2. Oberth creates a task and isolated Git worktree.
3. The runtime builds context and invokes the selected provider.
4. Tool calls pass through permission and repository-boundary checks.
5. Commands, events, diffs and verification results are recorded as evidence.
6. The user reviews the result and approves, requests a correction or rejects
   it.
7. Only explicit approval promotes the isolated change to the main checkout.

Interrupted work is recovered from durable task and event state. Each task
starts from a recorded base commit on an isolated branch. Approval commits the
reviewed result there and applies that exact commit to a clean main checkout.
If promotion conflicts, Oberth aborts it and preserves the isolated branch for
inspection or correction.

## Trust boundaries

Repository content, prompts, model responses and tool arguments are untrusted.
Filesystem and command effects must remain scoped to the selected repository
and active worktree. Provider secrets are stored outside tracked configuration
and redacted from logs and evidence.

The local API binds to loopback and requires a generated token. Oberth is a
single-user local application in this alpha; it does not provide
organization-level roles, remote administration or multi-tenant isolation.

See [SECURITY.md](../SECURITY.md) for the supported trust model and
vulnerability reporting process.

## Compatibility

Runtime events, result bundles, schemas and structured CLI output are explicit
contracts and require tests when changed. During Public Alpha these contracts
may evolve between releases; incompatible changes must be called out in the
changelog and accompanied by migration or recovery guidance where persisted
data is affected.

## Development boundaries

- Keep UI clients thin; execution, permission and promotion rules belong in the
  local service.
- Treat repository content, model output and tool arguments as untrusted.
- Route filesystem, command, network and provider effects through their
  permission and audit boundaries.
- Keep handlers cancellable and make interrupted state recoverable.
- Preserve versioned events and evidence when changing runtime behavior.
