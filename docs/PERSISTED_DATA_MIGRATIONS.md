# Persisted-data migration policy

Oberth treats persisted configuration, database rows, task state, events, and
result bundles as operator-owned data. A release must never silently discard or
downgrade those formats.

## Format inventory

| Format | Owner | Current version | Storage | Compatibility and recovery |
| --- | --- | --- | --- | --- |
| Configuration | `internal/config` | 1 | `.oberth.yaml` and environment overrides | New keys are additive. Incompatible changes require a preserved pre-migration file and documented manual recovery. |
| Database | `internal/db` | 24 | PostgreSQL plus `schema_migrations` | Forward-only in production. Back up before upgrading; down migrations are for development, not data recovery. |
| Task and run state | `internal/api` | 1 | `tasks`, `sessions`, `task_runs` | Unknown versions fail closed and remain recoverable; they are never coerced into a terminal state. |
| Run events | `internal/api` | 1 | append-only `run_events` | Existing events are immutable. Recovery appends correlated evidence instead of rewriting history. |
| Result bundle | `internal/api` | 1 | `task_runs.result_bundle` and exports | Additive v1 fields are accepted. Incompatible versions require an explicit migration and byte-identical backup. |

The executable copy of this inventory lives in `internal/migration/policy.go` so
tests fail when an owner, version, storage location, or rollback policy is absent.

## Forward migration contract

1. Stop new executions and create a database or file backup.
2. Validate the source version before changing any bytes.
3. Write a byte-identical `*.pre-migration-v<version>` recovery file.
4. Produce and validate the complete target document before atomic replacement.
5. Run the release verification suite and retain migration evidence.
6. Record breaking format changes and recovery commands in `CHANGELOG.md`.

Unknown or malformed versions fail closed. A failed migration must leave the
source path unchanged, must not overwrite an existing recovery backup with
different bytes, and must return an actionable error.

## Rollback and recovery

Application rollback does not imply data downgrade. If an upgrade fails, stop
Oberth, retain logs and the current data directory, restore the database backup
or the relevant `*.pre-migration-v<version>` file, and restart the previous
application version. Never run a destructive down migration against the only
copy of operator data.

The fixture in `internal/migration/testdata/result-bundle-v0.json` exercises a
supported prior-version migration. Tests also cover future versions, malformed
documents, conflicting recovery backups, and byte preservation after failure.
