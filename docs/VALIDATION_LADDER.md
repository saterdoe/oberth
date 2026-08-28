# Deterministic validation ladder

The foundational oracle uses the real HTTP handlers, durable PostgreSQL schema,
typed agent runtime, tools, worktrees, evidence and review/promotion path. A tiny
dependency-free Go repository and scripted in-process provider replace personal
repositories and live models. This proves product invariants, not model quality.

## Gates and dependencies

| Gate | Depends on | Property |
| --- | --- | --- |
| contracts | dependency preparation | versioned actions and evidence reject invalid claims |
| components | contracts | isolated workspace, tool, provider and recovery contracts |
| durable-flow | components | real task plan → edit → verify → review → promotion |
| resilience (#45) | durable-flow | repeated failure, concurrency and cleanup scenarios |
| live conformance | foundational gates | actual selected model transport and behavior; optional |

The durable flow checks unchanged primary checkout before approval, automatic
evidence and memory promotion, idempotent starts, interrupted-run reconciliation,
fresh-attempt resume, event-cursor replay, read-only planning, correction, rejection,
dirty-checkout conflict, exact promoted content and rejected invented evidence.
Resume means a new isolated run with a preserved historical checkpoint; it does
not replay arbitrary side effects inside the interrupted worktree.

## Local commands

```sh
# Explicit online preparation, once per dependency/toolchain/distribution change:
node scripts/validation-ladder.mjs --prepare
# No external network or provider credentials are needed after preparation:
node scripts/validation-ladder.mjs
```

Preparation installs Go modules and extracts the embedded PostgreSQL distribution
into ignored `artifacts/hermetic-postgres`. The oracle sets GOPROXY/GOSUMDB off and
GOTOOLCHAIN local, ignores global/system Git configuration, removes inherited
database/provider environment settings, and uses only its own temporary database.
The HTTP transport in the durable fixture denies non-loopback destinations.
Loopback is required for PostgreSQL and the test HTTP server. This is an application
test guard, not an OS network sandbox for malicious subprocesses. Fixture commands
use no external modules or fetch operations. The application never contacts Ollama.

PostgreSQL binaries must already exist: offline startup fails before a download
attempt if they are missing. Windows preparation includes an extensionless pg_ctl
copy because the embedded library's cache probe expects that path. Preparation is
separate from the measured oracle; a fresh machine still needs dependencies.

## Reports and deterministic expectations

Each run creates a unique ignored `artifacts/validation/run-*` directory containing
`report.json`, per-gate JSONL/stderr, and fixture-only run bundles/events. Reports
stop at the first failed gate and include the last named product invariant. Both
successful and failed runs retain artifacts; CI uploads only this directory for
seven days, never the PostgreSQL distribution/database or personal runtime data.

Expected content, event order, state transitions, evidence references and decisions
are asserted. UUIDs, temporary directories and measured durations are intentionally
not golden values. Git dates are fixed by the runner; compare promoted content and
commit relationships rather than hashes containing a generated run ID.

The workflow runs natively on Linux amd64/arm64, macOS amd64/arm64 and Windows amd64
using the [standard GitHub runner labels](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
Component tests are useful while editing. Run the full ladder before a runtime PR;
run scheduled resilience before release; use live-provider conformance separately
when changing model integration. A live model success never replaces these gates.
