#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

if ! command -v node >/dev/null 2>&1; then
  echo "Node.js 22 or newer is required and was not found in PATH." >&2
  exit 1
fi

exec node "$SCRIPT_DIR/release-check.mjs"
