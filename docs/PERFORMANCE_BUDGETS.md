# Performance budgets

The alpha baseline uses deterministic small, medium, and large fixtures for
startup/state loading, incremental indexing, API serialization, context
compilation, and source chunking. Each scenario records 25 samples after
warm-up and reports p50 and p95. CI fails when p95 exceeds these deliberately
generous regression ceilings:

| Fixture | Inputs/lines | p95 ceiling |
| --- | ---: | ---: |
| Small | 25 | 20 ms |
| Medium | 250 | 75 ms |
| Large | 1,000 | 250 ms |

Incremental indexing receives twice the listed ceiling because it includes
filesystem discovery and hashing. The fixtures contain identical deterministic
Go files and never call an embedding service or network endpoint.

The frontend production bundle is limited to 100 KiB gzip JavaScript, 25 KiB
gzip CSS, and 500 KiB total uncompressed assets. CI uploads the available
reports even when a gate fails; the Go report includes partial measurements.
UI measurements are produced only if execution reaches the UI build.

The Go latency test uses the `performance` build tag so a broad parallel
`go test ./...` cannot compete with the measurement. Both the shared release
runner and the dedicated performance job explicitly execute that tagged gate
in isolation. Thresholds are unchanged; this isolates load, not failures.

Run locally:

```text
$env:OBERTH_PERF_REPORT=(Join-Path (Get-Location) 'artifacts/performance/go-performance.json')
go test -tags performance ./internal/perfbench -run TestRegressionBudgets -count=1 -v
npm --prefix ui run build
node scripts/ui-budget.mjs
```

These are regression budgets, not universal hardware promises. Release-specific
benchmarks such as Code Map query/render limits remain additional gates.
