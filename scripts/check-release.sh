#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

TRACKED_RUNTIME=$(git ls-files -- data .tmp-qa)
if [ -n "$TRACKED_RUNTIME" ]; then
  echo "Release blocked: runtime/QA files are tracked:" >&2
  echo "$TRACKED_RUNTIME" >&2
  exit 1
fi

go test ./...
go test -race ./internal/api ./internal/gateway ./internal/toolrunner ./pkg/git
go test -tags e2e ./internal/api -run TestDurableRunHTTPHappyPath -count=1

if command -v npm >/dev/null 2>&1; then
  npm ci
  npm --prefix ui ci
  npm test
  npm run build:all
  npm run test:docs
else
  echo "npm not found: optional web checks skipped."
fi

echo "Release candidate verification passed."
