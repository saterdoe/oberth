$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) {
    throw "Go 1.25+ is required and was not found in PATH."
}

$version = (& $go version)
if ($LASTEXITCODE -ne 0) {
    throw "Unable to execute Go."
}

$bin = Join-Path $root "bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

Push-Location $root
try {
    & $go mod download
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to download Go dependencies."
    }
    & $go build -trimpath -o (Join-Path $bin "oberth.exe") ./cmd/oberth
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to build oberth."
    }
    & $go build -trimpath -o (Join-Path $bin "oberth-server.exe") ./cmd/oberth-server
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to build oberth-server."
    }
} finally {
    Pop-Location
}

Write-Host "CLI installation ready." -ForegroundColor Green
Write-Host "  $bin\oberth.exe"
Write-Host "  $bin\oberth-server.exe"
Write-Host "Node.js and Docker were not required."
