$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    & (Join-Path $PSScriptRoot "audit-release-tree.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Release tree audit failed." }
    & (Join-Path $PSScriptRoot "check.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Main verification failed." }
    & (Join-Path $PSScriptRoot "check-e2e.ps1")
    if ($LASTEXITCODE -ne 0) { throw "Durable E2E failed." }
    Write-Host "Release candidate verification passed." -ForegroundColor Green
} finally {
    Pop-Location
}
