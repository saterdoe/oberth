[CmdletBinding()]
param(
    [switch]$Run
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$ui = Join-Path $repo 'ui'
$frontend = Join-Path $repo 'desktop\frontend\dist'
$desktop = Join-Path $repo 'desktop'
$icon = Join-Path $desktop 'build\appicon.png'
$output = Join-Path $repo 'dist\oberth-desktop'
$package = Join-Path $repo 'dist\oberth-windows-x64.zip'
$version = (Get-Content -LiteralPath (Join-Path $repo 'VERSION') -Raw).Trim()
$versionParts = ($version -split '-', 2)[0] -split '\.'
$windowsVersion = (@($versionParts) + @('0', '0', '0', '0'))[0..3] -join '.'

& node (Join-Path $repo 'scripts\version.mjs')
if ($LASTEXITCODE -ne 0) { throw 'Version contract failed.' }

Write-Host 'Building oberth desktop UI...'
& npm.cmd --prefix $ui run build
if ($LASTEXITCODE -ne 0) { throw 'UI build failed.' }

New-Item -ItemType Directory -Path $frontend -Force | Out-Null
Remove-Item -LiteralPath $frontend -Recurse -Force
New-Item -ItemType Directory -Path $frontend -Force | Out-Null
Copy-Item -Path (Join-Path $ui 'dist\*') -Destination $frontend -Recurse -Force

New-Item -ItemType Directory -Path $output -Force | Out-Null
Write-Host 'Building local service...'
& go build -trimpath -ldflags '-s -w' -o (Join-Path $output 'oberth-server.exe') ./cmd/oberth-server
if ($LASTEXITCODE -ne 0) { throw 'Local service build failed.' }

Write-Host 'Building native application...'
if (-not (Test-Path -LiteralPath $icon)) { throw "Application icon not found: $icon" }
& go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64 --in (Join-Path $desktop 'winres.json') --out (Join-Path $desktop 'rsrc') --product-version $windowsVersion --file-version $windowsVersion
if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
& go build -trimpath -tags 'production,desktop,webkit2_41' -ldflags '-s -w -H windowsgui' -o (Join-Path $output 'oberth.exe') ./desktop
if ($LASTEXITCODE -ne 0) { throw 'Desktop application build failed.' }

Copy-Item -Path (Join-Path $repo 'packaging\*') -Destination $output -Force
Copy-Item -LiteralPath (Join-Path $repo 'LICENSE') -Destination $output -Force
Copy-Item -LiteralPath (Join-Path $repo 'NOTICE') -Destination $output -Force
Copy-Item -LiteralPath (Join-Path $repo 'THIRD_PARTY_NOTICES.md') -Destination $output -Force
if (Test-Path -LiteralPath $package) { Remove-Item -LiteralPath $package -Force }
Compress-Archive -Path (Join-Path $output '*') -DestinationPath $package -CompressionLevel Optimal

Write-Host "Desktop application ready: $output"
Write-Host "Installable package ready: $package"
if ($Run) {
    Start-Process -FilePath (Join-Path $output 'oberth.exe')
}
