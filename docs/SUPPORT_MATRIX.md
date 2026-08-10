# Supported platforms and architectures

This matrix applies to the Oberth Public Alpha. “Supported” means the
combination is built and smoke-tested on native GitHub-hosted hardware on every
change or release. Cross-compilation alone does not establish runtime support.

| Operating system | Architecture | CLI and local service | Desktop app | CI evidence |
| --- | --- | --- | --- | --- |
| Windows 11 / Server 2022+ | amd64 | Supported | Primary alpha platform | Native build, tests, and smoke test |
| Ubuntu 22.04+ | amd64 | Supported | Not packaged | Native build, tests, and smoke test |
| Ubuntu 22.04+ | arm64 | Supported | Not packaged | Native build and smoke test |
| macOS 14+ | amd64 | Supported | Not packaged | Native build and smoke test |
| macOS 14+ | arm64 | Supported | Not packaged | Native build and smoke test |
| Windows | arm64 | Not supported | Not supported | No release asset; runner remains preview |
| Other operating systems or CPU architectures | any | Not supported | Not supported | No release asset |

Release artifact names contain both operating system and architecture, for
example `oberth-linux-arm64` and `oberth-windows-amd64.exe`. Absence from the
release matrix is intentional and the release build rejects unknown target
combinations with an explicit error.

## Tooling compatibility

- Git: the version available on the supported OS must provide worktrees and the
  standard `status`, `diff`, `apply`, `merge`, `tag`, and `revert` operations.
- Go: Go 1.25 or newer is required to build the CLI, local service, and desktop
  source.
- Node.js: Node.js 22 or newer is required to build or test the web interface.
- Shell: PowerShell is the supported automation shell on Windows; POSIX `sh` is
  supported on Linux and macOS. Git Bash is used only inside release CI.
- VS Code: the extension is source-build and development supported wherever its
  declared Node.js and VS Code requirements are met; packaged extension support
  is tracked separately.
- WSL: the Linux CLI and local service may run inside WSL 2, but the Windows
  desktop app and a service inside WSL are separate environments. WSL-specific
  filesystem, browser-launch, and interop behavior is not yet a supported
  release configuration.

## Support policy

Native smoke tests execute `oberth version` and `oberth-server -h` from the
built binaries. The broader Go suite continues to run on the primary runners.
If a native architecture runner is unavailable, its required check must fail or
be explicitly waived with a linked issue; a successful cross-build must not be
reported as equivalent evidence.

Support can only be added by updating this document, the CI smoke matrix, and
the release artifact allowlist together. Preview runners are not used to claim
stable support. Platform-specific defects should include OS version,
architecture, shell, filesystem type, and whether execution occurred inside
WSL or another compatibility layer.
