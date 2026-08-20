# Security policy

## Supported versions

Oberth 0.1.0-alpha.8 is a local, single-user Public Alpha. Security fixes are
applied to the latest alpha release only. Older alpha releases are unsupported.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability. Use GitHub private
vulnerability reporting in the public repository. Include:

- affected commit or version;
- reproduction steps;
- expected and observed security boundary;
- whether credentials, files outside a worktree or remote systems are exposed.

Do not include real API keys, access tokens, private source code or customer
data. Maintainers aim to acknowledge a complete report within three business
days and provide a remediation status within seven business days. These are
response targets, not a service-level agreement.

## Security boundaries

- The local service binds to loopback and requires a bearer token.
- Agent mutations belong in an isolated Git worktree.
- Provider credentials are encrypted at rest and omitted from API responses.
- Model output and repository content are untrusted input.
- Code Map is local and metadata-only. It never sends source, graph topology or
  embeddings to a remote layout/parser service and never executes a repository
  to resolve relationships.
- Code Map labels are untrusted text. The desktop content-security policy
  blocks remote scripts, frames, objects and unsolicited navigation.
- Remote extensions, remote tool servers and multi-user deployments are not
  part of the supported alpha security model.

Isolation reduces risk; it is not a sandbox for arbitrary hostile code. A
permitted command runs with the operating-system privileges of the Oberth
process. Review requested permissions and use a disposable repository or
stronger OS-level isolation when evaluating untrusted projects.

Code topology can still reveal sensitive architecture, package names and
internal boundaries even without source snippets. Avoid screenshots or copied
reports from private projects. Code Map data is excluded from diagnostics and
telemetry; export and clipboard workflows are not supported in this release.
