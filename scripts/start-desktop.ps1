$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$app = Join-Path $repo 'dist\oberth-desktop\oberth.exe'
$server = Join-Path $repo 'dist\oberth-desktop\oberth-server.exe'
$sourceRoots = @(
    (Join-Path $repo 'desktop'),
    (Join-Path $repo 'ui\src'),
    (Join-Path $repo 'ui\public'),
    (Join-Path $repo 'internal'),
    (Join-Path $repo 'cmd'),
    (Join-Path $repo 'pkg'),
    (Join-Path $repo 'packaging')
)
$sourceFiles = @(
    (Join-Path $repo 'go.mod'),
    (Join-Path $repo 'go.sum'),
    (Join-Path $repo 'ui\package.json'),
    (Join-Path $repo 'ui\package-lock.json'),
    (Join-Path $repo 'ui\vite.config.ts'),
    (Join-Path $repo 'ui\tsconfig.json')
)
$latestSource = @(
    $sourceRoots |
        Where-Object { Test-Path -LiteralPath $_ } |
        ForEach-Object { Get-ChildItem -LiteralPath $_ -Recurse -File }
    $sourceFiles |
        Where-Object { Test-Path -LiteralPath $_ } |
        ForEach-Object { Get-Item -LiteralPath $_ }
) | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1

$bundleMissing = -not (Test-Path -LiteralPath $app) -or -not (Test-Path -LiteralPath $server)
$bundleStale = -not $bundleMissing -and $latestSource -and (
    $latestSource.LastWriteTimeUtc -gt (Get-Item -LiteralPath $app).LastWriteTimeUtc -or
    $latestSource.LastWriteTimeUtc -gt (Get-Item -LiteralPath $server).LastWriteTimeUtc
)
if ($bundleMissing -or $bundleStale) {
    Write-Host 'Desktop bundle is missing or stale; rebuilding it...'
    & (Join-Path $PSScriptRoot 'build-desktop.ps1')
}
Start-Process -FilePath $app
