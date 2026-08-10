$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$go = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $go) { $go = 'C:\Program Files\Go\bin\go.exe' }
$npm = (Get-Command npm.cmd -ErrorAction SilentlyContinue).Source
if (-not $npm) { $npm = 'C:\Program Files\nodejs\npm.cmd' }
$env:Path = "$(Split-Path -Parent $npm);$(Split-Path -Parent $go);$env:Path"
Push-Location $root
try {
  & $go test ./...
  if ($LASTEXITCODE -ne 0) { throw 'Fallaron los tests de Go.' }
  & $go vet ./...
  if ($LASTEXITCODE -ne 0) { throw 'Fallo el analisis estatico de Go.' }
  & $npm --prefix ui run test
  if ($LASTEXITCODE -ne 0) { throw 'Fallaron los tests del frontend.' }
  & $npm run test:extension
  if ($LASTEXITCODE -ne 0) { throw 'Fallaron los tests de la extension de VS Code.' }
  & $npm --prefix ui run build
  if ($LASTEXITCODE -ne 0) { throw 'Fallo el build del frontend.' }
  & $npm run test:docs
  if ($LASTEXITCODE -ne 0) { throw 'Fallo el contrato de documentacion y release.' }
  Write-Host 'Todas las verificaciones pasaron.' -ForegroundColor Green
} finally { Pop-Location }
