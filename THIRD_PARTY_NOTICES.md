# Third-party notices

Oberth incorporates open-source dependencies under permissive and
file-level copyleft licenses. Their original copyright notices and license
terms remain with their respective authors.

The authoritative dependency inventories are `go.mod`, `go.sum`,
`package-lock.json`, and `ui/package-lock.json`. Release artifacts must be
checked against those lock files because the effective dependency set can
change between releases.

Notable license families currently present include:

- Apache-2.0: BAML, Cobra, TypeScript, and supporting packages.
- MIT, BSD, ISC, 0BSD, and MIT-0: Wails, React, Vite, PostgreSQL clients,
  embedded-postgres helpers, Lucide, and supporting packages.
- MPL-2.0: HashiCorp HCL, errwrap, go-multierror, and build-time
  `lightningcss` packages.

MPL-2.0 applies at file level. Corresponding source is available from the
upstream projects identified by the module and package lock files. Oberth
does not distribute `node_modules`; build-only packages are not included in
the native application bundle.

The embedded PostgreSQL binaries downloaded at runtime are distributed by
the PostgreSQL project under the PostgreSQL License. See
<https://www.postgresql.org/about/licence/>.

Before publishing a release, maintainers must regenerate and review the
third-party inventory, preserve required upstream notices, and investigate
unknown, GPL, AGPL, SSPL, LGPL, or other reciprocal licenses.
