# Local runtime observability

`GET /api/v1/health` is **liveness only**. `GET /api/v1/ready` uses the normal local
API authentication and returns HTTP 503 when draining, when the runtime is absent,
when the database cannot perform writes against the run schema, or when no provider
is active. Its reason is a stable code, not a raw database error. A ready response
does not certify provider credentials, network reachability or sufficient model
capacity; those remain execution-time checks. Shutdown closes admission before
HTTP draining. Existing runs keep their durable recovery contract.

`GET /api/v1/diagnostics/runtime` remains available while readiness is false. It
returns content-free stage/provider latency metrics, correlated run/task/session
traces, and stuck-run signals. Metrics cover context compilation, advisory workflow
stages, run duration, individual chat attempts and streaming attempts. They include
count, errors, sum, maximum and non-cumulative buckets (100ms, 1s, 10s, 60s, overflow).
Retries are individual provider samples; provider timing excludes concurrency-queue
wait. A run span includes finalization. Stages and provider IDs are local labels;
no model prompts, source paths, source code, output or raw errors are collected.

Retention is process-local: at most 256 metric series and the latest 256 traces.
Excess series are counted, not allocated without a bound. Restart resets telemetry;
durable run events remain the historical source of truth. Diagnostics scans the
oldest 100 running runs and exposes this limit. Expired leases or no durable progress
for five minutes are **signals**, not automatic failure or cancellation: a slow
local model can legitimately trip the progress signal. A database query failure is
reported explicitly rather than treated as zero stuck runs.

## Initial service objectives

These are initial operational targets, not claims about measured compliance:

| Signal | Initial objective | Response |
| --- | --- | --- |
| Readiness probe | complete within 2 seconds | inspect stable reason; retry after recovery |
| Progress gap | investigate after 5 minutes | correlate run ID with durable events |
| Provider latency | review attempts exceeding 60 seconds | inspect local resource use and model budget |
| Diagnostic retention | bounded at 256 traces/series | export before restart if evidence is needed |

Hardware/model-dependent inference is deliberately not assigned a universal
latency promise. Use bucket counts to establish a baseline before tightening SLOs.

## Doctor bundle

`oberth doctor --bundle ./diagnostics.zip` exports health, versions, error summaries
and `runtime.json` with these metrics. It can still export when the daemon is down;
that absence is recorded. The ZIP is create-only (never overwrites), owner-readable
where filesystem permissions support it, and never uploaded automatically. Raw
configuration is omitted rather than trusting secret-name heuristics. Log/error
redaction covers credentials and complete private-key blocks; JSON remains valid.
Review the bundle before sharing: run identifiers and locally produced diagnostic
messages are operational metadata, and redaction is not permission to publish it.

Regression checks: `go test ./internal/observability ./internal/doctor ./internal/api
./internal/gateway` and `go test -tags e2e ./internal/api -run
TestDurableRunHTTPHappyPath -count=1`. The latter exercises a real writable and
read-only PostgreSQL connection and correlated provider/context samples.
