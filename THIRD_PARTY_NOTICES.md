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

## github.com/gofrs/flock

Copyright (c) 2018-2024, The Gofrs
Copyright (c) 2015-2020, Tim Heckman
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this
  list of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice,
  this list of conditions and the following disclaimer in the documentation
  and/or other materials provided with the distribution.

* Neither the name of gofrs nor the names of its contributors may be used
  to endorse or promote products derived from this software without
  specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
