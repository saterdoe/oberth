# Quickstart

Oberth 0.1.0-alpha.2 is a Public Alpha. Use a test repository or one with a
clean backup, and review every generated change before approval.

## Requirements

- Go 1.25 or newer
- Git
- Node.js 22 or newer when building the interface

Windows is the primary desktop platform for this release.
The complete CLI, service, desktop, architecture, WSL, and shell support policy
is published in the [supported platform matrix](SUPPORT_MATRIX.md). The CLI and
local service are continuously built, tested, and smoke-tested on Linux and
macOS; the desktop application is not packaged or verified on those platforms.

## Start on Windows

From the repository root:

```powershell
.\start.cmd
```

This starts the local service and web interface. Stop managed processes with:

```powershell
.\start.cmd -Stop
```

To build and run the native desktop app:

```powershell
.\start-desktop.cmd
```

Build output is written under `dist`. User data is stored in the operating
system's per-user configuration directory and is preserved across application
updates.

## Start the CLI on Linux or macOS

```sh
./scripts/setup-cli.sh
./bin/oberth init
./bin/oberth-server --config ./oberth.yaml
```

The service binds to loopback and requires a generated local token. Do not
expose the service port to a network.

The supported source workflow uses the repository scripts above. CI exercises
`setup-cli.sh`, starts the resulting CLI and local-service binaries, and runs
the complete `check-release.sh` verification on both Linux and macOS. Desktop
packaging, browser launching, and OS-specific service installation remain
outside that portable coverage.

## Configure a provider

For Ollama:

```text
oberth provider add --name local --type ollama \
  --base-url http://localhost:11434 --model <model>
oberth provider list
oberth provider verify <provider-id>
```

For LM Studio or another OpenAI-compatible local server:

```text
oberth provider add --name local-server --type custom \
  --base-url http://localhost:1234 --model <model>
```

Keep cloud credentials out of tracked files and shell history. Use the provider
secret mechanism or an environment variable supported by the CLI. Run
`oberth doctor` and `oberth capabilities` to inspect the effective local
configuration before starting a task.

Provider compatibility varies by model. A successful chat response does not
guarantee reliable tool use or completion of a coding task.

## Run and review a task

From a Git repository:

```text
oberth run "fix the issue and run the relevant tests"
oberth status
oberth review
oberth diff
```

Oberth creates an isolated worktree and records the resulting diff, commands
and verification evidence. The main checkout remains unchanged while the task
runs.

After review:

```text
oberth approve --note "tests and diff reviewed"
```

Or keep the change isolated:

```text
oberth correct --note "handle the empty input case"
oberth reject --note "outside the intended scope"
```

Approval can fail if the main checkout is dirty or has diverged from the
recorded base. Oberth aborts a conflicting promotion and preserves the isolated
branch; it does not copy a partial result into the checkout.

## Verify a development checkout

Windows:

```powershell
.\check.cmd
```

Linux or macOS:

```sh
./scripts/check-release.sh
```

## Troubleshooting

- Service unavailable: run `oberth doctor` and confirm that the local process
  is running.
- Provider failure: verify the base URL, model availability and credentials.
- Interrupted task: restart the service and run `oberth resume`.
- Blocked or failed task: use **Close task** to cancel it while preserving its
  conversation, evidence and isolated worktree for later inspection.
- Promotion blocked: clean or commit unrelated checkout changes, then review
  the preserved task branch before retrying.
- Unexpected behavior after an upgrade: check [CHANGELOG.md](../CHANGELOG.md)
  for alpha compatibility notes.

For security boundaries and private vulnerability reporting, see
[SECURITY.md](../SECURITY.md).
