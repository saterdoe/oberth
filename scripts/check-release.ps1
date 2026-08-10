$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$node = (Get-Command node -ErrorAction SilentlyContinue).Source
if (-not $node) {
    throw "Node.js 22 or newer is required and was not found in PATH."
}

& $node (Join-Path $PSScriptRoot "release-check.mjs")
if ($LASTEXITCODE -ne 0) {
    throw "Release candidate verification failed."
}
