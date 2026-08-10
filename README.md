# Oberth

**Coding agents, under control.**

Oberth is a local runtime for delegating work to coding agents without handing
them your main checkout. Each task runs in an isolated Git worktree, records the
commands and checks it performed, and waits for a human decision before its
changes can be promoted.

> **Public Alpha · 0.1.0-alpha.2**
>
> Oberth is ready for evaluation, not unattended production use. Interfaces,
> configuration and storage formats may change before beta. Start with a test
> repository or one that has a recoverable remote copy.

## Why use Oberth?

Agent output is more useful when it is inspectable and reversible. Oberth makes
the review boundary part of the workflow:

1. You describe a task and choose a model.
2. Oberth creates an isolated worktree.
3. The agent edits, runs commands and records evidence there.
4. You review the session, checks and color-coded diff.
5. You approve, request another change, or reject the result.

The main checkout is not modified while the task runs. Approval can still be
blocked when the checkout is dirty, has diverged, or the change conflicts.
Oberth preserves the isolated result instead of applying a partial promotion.

## What is included

- A native Windows app and local web interface.
- A CLI and local service for Windows, Linux and macOS.
- Local and OpenAI-compatible model providers.
- Isolated Git worktrees for task execution.
- Reviewable diffs, command logs and verification evidence.
- Explicit approve, correct, reject and cancel decisions.
- Local persistence and recovery for interrupted work.

The interface is available in English and Spanish. Localization applies to
Oberth's application chrome; user and model messages, repository paths,
identifiers, source code and technical evidence are always preserved verbatim.

Windows is the primary tested desktop platform in this alpha. The CLI and
service run on Linux and macOS with less release coverage. Oberth is currently
single-user and local-first; hosted administration, team roles and
multi-tenant isolation are intentionally outside the public alpha.

## Quick start

This alpha is currently distributed from source. Clone or download the public
repository, open a terminal in its root directory, and use the Windows desktop
app for the most thoroughly tested experience.

Requirements:

- Git
- Go 1.25 or newer
- Node.js 22 or newer when building the interface

Build and launch the native app (recommended) with:

```powershell
.\start-desktop.cmd
```

The first run installs the locked interface dependencies and builds the local
service and desktop application. Wait until Oberth reports that the local
service, memory and semantic search are ready before creating a task.

In the app, open **Settings → Language** to choose English or Español, then
configure or detect a provider in **Settings**. Select **New task**, choose a
local Git repository and describe the result you want. When the run finishes,
review its diff and evidence before choosing approve, request changes or
reject.

To use the browser interface instead, run:

```powershell
.\start.cmd
```

Stop its managed processes with `.\start.cmd -Stop`.

For the CLI on Linux or macOS:

```sh
./scripts/setup-cli.sh
./bin/oberth init
./bin/oberth-server --config ./oberth.yaml
```

Then configure a provider and run the health checks:

```text
oberth provider add --name local --type ollama --base-url http://localhost:11434 --model <model>
oberth provider verify <provider-id>
oberth doctor
```

Run a task from a Git repository:

```text
oberth run "fix the issue and run the relevant tests"
oberth status
oberth review
oberth diff
```

Finish with one explicit decision:

```text
oberth approve --note "tests and diff reviewed"
oberth correct --note "handle the empty input case"
oberth reject --note "outside the intended scope"
```

The [Quickstart](docs/QUICKSTART.md) includes provider alternatives,
troubleshooting and recovery behavior.

## Data and privacy

Oberth is local-first, but prompts, selected context and model requests are sent
to the provider you configure. Local desktop state and the generated
authentication token are stored under `%APPDATA%\oberth`; source-development
logs and runtime files are stored under `data/` in the checkout. Neither
location belongs in Git. Back up anything you need before deleting those
directories or uninstalling an alpha build.

## Safety model

Oberth isolates repository changes; it is not a security sandbox. A configured
agent may execute commands, access files available to its process and contact
allowed services. Keep permissions narrow, do not place secrets in prompts or
tracked files, inspect the recorded evidence, and review every diff before
approval.

The local service binds to loopback and uses a generated token. Do not expose
its port to a network. See [Security](SECURITY.md) for trust boundaries and
private vulnerability reporting.

See the [supported platform matrix](docs/SUPPORT_MATRIX.md) for the exact OS,
CPU architecture, shell, WSL, and desktop support boundaries.

## Development

Run the release checks before submitting a change:

```powershell
.\scripts\check-release.ps1
```

On Linux or macOS:

```sh
./scripts/check-release.sh
```

Focused contributions are welcome, especially reproducible bug reports, tests,
documentation fixes and small improvements. Start with
[Contributing](CONTRIBUTING.md) and read the
[architecture overview](docs/ARCHITECTURE.md) before changing runtime
boundaries or public contracts.

## Interface languages

English is the default and fallback language. Spanish is bundled with the
application. Change the language in **Settings → Language**; the choice
persists across restarts. To contribute another locale, follow
[Interface languages](CONTRIBUTING.md#interface-languages).

## Known alpha limitations

- Windows is the primary tested desktop platform.
- The supported deployment is local and single-user.
- Configuration and storage formats may change before beta.
- Coding-agent reliability and tool use vary by provider and model.
- Worktree isolation is a review boundary, not an operating-system sandbox.

## Project status and documentation

- [Quickstart](docs/QUICKSTART.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Repository code index](docs/CODE_INDEX.md)
- [Contributing](CONTRIBUTING.md)
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Third-party notices](THIRD_PARTY_NOTICES.md)

There is no compatibility guarantee before beta. Breaking changes and required
recovery steps are documented in the changelog.

## License

Oberth is licensed under the [Apache License 2.0](LICENSE) (`Apache-2.0`).

## The name

The Oberth effect describes how a precisely placed impulse can decisively
change a trajectory. That is the product principle: agents generate the
impulse; isolation, evidence and human approval keep the trajectory under
control.
