# Release and rollback runbook

This runbook is the operational contract for cutting an Oberth alpha or beta
release. It deliberately separates preparing a candidate, approving an exact
commit, publishing artifacts from a tag, and advancing `main`.

## Release invariants

- `main` is the source of truth for productive code.
- Every feature branch starts from an updated `main` and uses `feat/<issue>-<slug>`.
- Feature branches integrate into `release/<version>`, never directly into
  `main` as part of a release cut.
- The approved release commit is immutable. The annotated `v<version>` tag,
  release artifacts, checksums, SBOM, and the commit advanced to `main` all
  refer to that exact commit.
- Artifacts are built by CI from the tag. Local build output is diagnostic and
  must not be published.
- A failed gate is a no-go. Fixes use a feature branch from `main`, are merged
  into the release branch, and restart verification.
- Persisted data is never downgraded unless the relevant format explicitly
  documents and tests that path.

## Ownership and approvals

With the current two-person team, one contributor acts as release operator and
the other as reviewer. The operator records commands and evidence; the reviewer
confirms scope, version, changelog, gates, tag target, and rollback decision.
The same person may prepare code and operate CI, but publishing still requires
an explicit go decision recorded in the release review.

## Dry run

A dry run is read-only with respect to Git history, tags, releases, and remote
artifacts. Run it before creating a release branch:

```sh
git fetch --prune origin
git status --short --branch
git rev-parse --verify origin/main
npm run test:docs
```

Confirm manually that the working tree is clean, the intended issues are in
review, no unresolved security or migration blocker applies, and the proposed
version is unused. Do not create a tag or upload an artifact during a dry run.

On Windows, execute the full candidate verification with:

```powershell
.\scripts\check-release.ps1
```

On Linux or macOS:

```sh
./scripts/check-release.sh
```

The full checks may install locked dependencies and produce ignored local build
output, but they must not modify Git history or publish anything.

## Cut the release branch

Choose a semantic version without the `v` prefix for the branch name. Starting
from a clean checkout:

```sh
git switch main
git pull --ff-only origin main
git switch -c release/<version>
git push -u origin release/<version>
```

Integrate reviewed feature branches into `release/<version>`. Preserve their
individual commits and record the included issue numbers. Resolve conflicts on
the release branch, then rerun all affected feature checks.

Version metadata and release notes are finalized on the release branch. Until
the coherent versioning automation is available, verify every user-visible
version location and ensure `CHANGELOG.md` describes migration and recovery
impact.

## Verify the candidate

Run the platform release check and require green CI for the release head. The
go/no-go review must confirm:

- intended issues and no unrelated changes;
- clean supported-platform CI and quality gates;
- version and changelog consistency;
- persisted-data compatibility or tested migration and recovery;
- security, secret-redaction, and dependency findings resolved;
- a documented rollback choice for code, artifacts, and data;
- the candidate commit SHA recorded in the review.

Any change after verification invalidates the approval and requires the gates
to run again. If `main` advances, merge or rebase it into the release branch,
resolve the result, and repeat verification before tagging.

## Tag and publish

After the explicit go decision, tag the recorded release-head SHA. Verify the
target before pushing:

```sh
git switch release/<version>
git rev-parse HEAD
git tag -a v<version> -m "Oberth <version>"
git rev-list -n 1 v<version>
git push origin v<version>
```

The pushed tag triggers the release workflow. Download the resulting artifacts
into a temporary directory and verify their checksums, SBOM presence, version
output, startup, and basic CLI/service behavior. Do not substitute artifacts
built from the branch or a local checkout.

If validation succeeds, advance `main` to the exact tagged commit:

```sh
git switch main
git pull --ff-only origin main
git merge --ff-only v<version>
git push origin main
```

If the fast-forward fails, stop. Do not create an unverified merge commit.
Rebuild the release branch from the new `main`, integrate the intended changes,
rerun the gates, and create a new candidate tag according to the rollback rules.

## Rollback before publishing

If a candidate fails before its tag is pushed, leave the remote release branch
for diagnosis or delete it only after its commits are confirmed recoverable.
Apply fixes through feature branches from `main`; do not rewrite `main` or reuse
an approved commit after changing it.

If a tag was created locally but not pushed, delete only that verified local
tag and repeat the candidate process:

```sh
git tag -d v<version>
```

## Rollback after publishing

Published tags are immutable. Never move or silently replace one.

1. Mark the release as withdrawn and stop distribution.
2. Preserve artifacts, checksums, SBOM, logs, and the failing commit for audit.
3. Restore the last known-good application version only when its persisted-data
   format can safely read the current data.
4. If data compatibility is uncertain, stop the application, preserve a backup,
   and follow the migration-specific recovery procedure instead of downgrading.
5. Implement the correction on a feature branch from current `main`, cut a new
   patch release branch, repeat every gate, and publish a new tag. Never reuse
   the withdrawn version.

If `main` already points at the withdrawn commit, preserve history and use a
reviewed revert or forward fix. Force-pushing or resetting shared `main` is not
an allowed rollback mechanism.

## Recovery and evidence

Retain the following for every release attempt, including no-go decisions:

- release branch and candidate SHA;
- included issues and reviewer decision;
- CI URLs and release-check output;
- version and changelog review;
- tag target and artifact checksums;
- SBOM and smoke-test result;
- rollback or recovery actions and their operator.

Secrets, tokens, private paths, and user data must be redacted before evidence
is attached to GitHub. A release is complete only when the exact tag is
published, its artifacts are verified, `main` contains that exact commit, and
the release record contains the evidence above.
