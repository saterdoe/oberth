# Contributing to Oberth

Oberth 0.1.0-alpha.3 is a Public Alpha. Focused bug reports, reproducible test
cases, documentation fixes and small improvements are especially valuable
while public interfaces are still taking shape.

## Development requirements

- Go 1.25 or newer
- Git
- Node.js 22 or newer for the interface

Follow the [Quickstart](docs/QUICKSTART.md) to set up the CLI and local service.
The measured coverage and static-analysis checks are documented in
[Quality gates](docs/QUALITY_GATES.md).

## Before submitting a change

Run the release checks for your platform.

Windows:

```powershell
.\scripts\check-release.ps1
```

Linux or macOS:

```sh
./scripts/check-release.sh
```

Release operators must follow the [release and rollback runbook](docs/RELEASE_RUNBOOK.md).
It defines the dry run, release-branch flow, go/no-go evidence, immutable tag,
artifact verification, advancement to `main`, and recovery procedure.

Changes to runtime states, events, result bundles, tools or structured CLI
output must update the corresponding contract and tests. Do not commit runtime
state, credentials, local tokens, databases, worktrees, build output or
dependency directories.

### Release versioning

`VERSION` is the canonical Oberth release version. Runtime, package and Windows
resource versions must agree with it. Validate the complete contract with:

```text
npm run version:check
```

To prepare a version change, first add the corresponding release section to
`CHANGELOG.md`, then run:

```text
npm run version:set -- <semver>
```

The update command synchronizes the root and UI package metadata, lockfiles,
the VS Code extension, Go runtime default, Windows resources and public version
references. CI rejects version drift, missing changelog sections and release
tags that do not equal `v` followed by the canonical version.

## Code and documentation

- Keep changes focused and preserve unrelated work.
- Prefer tests that describe externally observable behavior.
- Format Go with `gofmt`; follow the existing TypeScript checks.
- Comment invariants, security boundaries and non-obvious decisions.
- Change generated files through their source or generator.

### Interface languages

English (`en`) is the source locale and mandatory fallback. Spanish (`es`) is
bundled with the application. User-facing text belongs in locale catalogs, not
inline in React components; use stable semantic keys and the platform
internationalization APIs for dates, numbers and plurals.

A locale contribution should:

- add a complete catalog with the same keys as English;
- preserve placeholders and technical terms;
- include the locale in the language selector;
- add or update tests for fallback and persisted language selection;
- avoid changing application behavior in the translation commit.

Use semantic keys for every new application label. Do not pass task titles,
user or model messages, repository paths, identifiers, source code, command
output, or backend diagnostics through the translation layer. Mark those
dynamic values with `data-no-translate` when they are rendered inside localized
UI. Compatibility replacements exist only for migrating legacy labels; do not
add substring replacements. Test both bundled locales when changing UI copy.

Missing keys must fall back to English. A build or test should fail when a
bundled catalog has missing, extra or malformed keys.

See the [architecture overview](docs/ARCHITECTURE.md) before changing runtime
boundaries or public contracts.

## Pull requests

Explain:

- the user problem and visible outcome;
- the security and recovery impact;
- the verification performed;
- compatibility or migration implications.

Avoid mixing formatting changes or unrelated refactors into a functional
change.

## Security

Never commit credentials or private data. New filesystem, command, network or
cloud effects must pass through the permission system, redact secrets and emit
appropriate audit evidence.

Report suspected vulnerabilities using the private process described in
[SECURITY.md](SECURITY.md), not a public issue.

## License

Unless stated otherwise, contributions are submitted under the
[Apache License 2.0](LICENSE), the same license as the project. By submitting a
contribution, you confirm that you have the right to do so.
