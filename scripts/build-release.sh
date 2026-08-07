#!/usr/bin/env sh
set -eu

GOOS_VALUE=${1:-}
GOARCH_VALUE=${2:-}
OUTPUT_DIR=${3:-dist}

case "$GOOS_VALUE/$GOARCH_VALUE" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
  *)
    echo "Unsupported Oberth release target: $GOOS_VALUE/$GOARCH_VALUE" >&2
    exit 2
    ;;
esac

case "$GOOS_VALUE" in
  windows) SUFFIX=.exe ;;
  *) SUFFIX= ;;
esac

mkdir -p "$OUTPUT_DIR"
GOOS=$GOOS_VALUE GOARCH=$GOARCH_VALUE go build -trimpath -ldflags="-s -w" -o "$OUTPUT_DIR/oberth-$GOOS_VALUE-$GOARCH_VALUE$SUFFIX" ./cmd/oberth
GOOS=$GOOS_VALUE GOARCH=$GOARCH_VALUE go build -trimpath -ldflags="-s -w" -o "$OUTPUT_DIR/oberth-server-$GOOS_VALUE-$GOARCH_VALUE$SUFFIX" ./cmd/oberth-server
