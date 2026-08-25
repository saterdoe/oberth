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
gzip CSS, and 500 KiB total uncompressed assets. CI uploads both
`go-performance.json` and `ui-bundle.json` even when a gate fails.

Run locally:

```text
$env:OBERTH_PERF_REPORT='artifacts/performance/go-performance.json'
go test ./internal/perfbench -run TestRegressionBudgets -count=1 -v
npm --prefix ui run build
node scripts/ui-budget.mjs
```

These are regression budgets, not universal hardware promises. Release-specific
benchmarks such as Code Map query/render limits remain additional gates.
