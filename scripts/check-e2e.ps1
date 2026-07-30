$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) {
    throw "Go is required and was not found in PATH."
}

Push-Location $root
try {
    & $go test -tags e2e ./internal/api -run TestDurableRunHTTPHappyPath -count=1
    if ($LASTEXITCODE -ne 0) {
        throw "Durable E2E failed."
    }
    Write-Host "Durable E2E passed." -ForegroundColor Green
} finally {
    Pop-Location
}
