#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

if ! command -v go >/dev/null 2>&1; then
  echo "Go 1.25+ is required and was not found in PATH." >&2
  exit 1
fi

GO_VERSION=$(go env GOVERSION)
case "$GO_VERSION" in
  go1.2[4-9]*|go1.[3-9][0-9]*) ;;
  *)
    echo "Go 1.25+ is required; found $GO_VERSION." >&2
    exit 1
    ;;
esac

mkdir -p "$ROOT_DIR/bin"
cd "$ROOT_DIR"
go mod download
go build -trimpath -o "$ROOT_DIR/bin/oberth" ./cmd/oberth
go build -trimpath -o "$ROOT_DIR/bin/oberth-server" ./cmd/oberth-server

echo "CLI installation ready."
echo "  $ROOT_DIR/bin/oberth"
echo "  $ROOT_DIR/bin/oberth-server"
echo "Node.js and Docker were not required."
