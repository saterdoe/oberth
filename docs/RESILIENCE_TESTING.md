# Durable-runtime resilience testing

Run `node scripts/resilience-stress.mjs` from the repository after installing Go,
Node and Git. The default is three **sequential** iterations (optional argument
1–20). Each iteration uses fresh temporary storage and a fixture provider: no
installed daemon, user repository or model credentials. Go and embedded
PostgreSQL dependencies may download on first use; this is not the offline
validation ladder.

## Failure contracts

- The existing durable HTTP scenario exercises provider failures, concurrent
  idempotent starts, reviewed promotion, invalid evidence and recovery replay.
- `TestResilienceDatabaseRestart` waits at a provider-entry barrier, stops its
  real PostgreSQL child, cancels the worker while persistence is unavailable,
  then starts PostgreSQL over the same data and creates a fresh HTTP server.
  It advances only the fixture lease to avoid a wall-clock expiry delay.
- Startup reconciliation must record exactly one interruption across two calls,
  preserve its worktree, and retain the durable event cursor. A retry must use a
  different run and complete verification.
- A held PostgreSQL advisory lock deterministically tests decision contention;
  two concurrent rejection requests then yield exactly one success, one conflict
  and one outcome event. The guard is acquired **before** any Git side effect,
  including when requests reach separate daemon instances.
- Rejected worktrees disappear; interrupted worktrees are intentionally retained
  for recovery, not classified as leaks. The fixture explicitly removes its
  retained worktree and verifies only the primary registry entry remains. Worker
  maps must empty and the PostgreSQL listener must close after each shutdown.

This simulates daemon replacement and actual database unavailability; it does
not claim to test power loss, arbitrary crash points inside Git promotion, disk
corruption, or production-scale multi-daemon operation. Advisory locking does
not make Git and PostgreSQL one atomic transaction. A database loss after a Git
operation can still require operator recovery. Each decision opens a dedicated
short-lived database connection for its guard so held locks cannot exhaust the
query pool; closing that session releases the lock even on an error return.

## Scheduling and evidence

The `Resilience stress` GitHub workflow runs three iterations on PRs and main,
and ten every Monday at 04:19 UTC after it is merged into the default branch.
Linux and Windows run independently; a failure is never retried into green.
Each iteration has a three-minute Go test deadline and a four-minute driver
deadline. The outer deadline terminates only its own process tree/group.
Tests use normal cleanup for PostgreSQL and temporary worktrees. Forced OS or
runner termination cannot guarantee deferred cleanup; use isolated CI runners
for destructive timeout investigation, not a production daemon.

`artifacts/resilience/run-*/report.json` records the first failed iteration and
each iteration's JSONL log is retained even on failure (14 days in GitHub).
Artifacts are ignored by Git and contain only synthetic fixtures. The runner's
own tests check failure propagation, credential filtering and bounded process
termination. These checks are evidence for covered scenarios, not a claim that
all possible process leaks have been eliminated.

Performance-budget failures also retain partial measurements. CI passes absolute
report paths because Go tests run from their package directory, not the checkout
root. Existing latency thresholds are unchanged; stress does not waive them.
