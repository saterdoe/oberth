$ErrorActionPreference = 'Stop'
$installDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$startMenuLink = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\oberth.lnk'
$desktopLink = Join-Path ([Environment]::GetFolderPath('Desktop')) 'oberth.lnk'
Remove-Item -LiteralPath $startMenuLink -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath $desktopLink -Force -ErrorAction SilentlyContinue

$cleanup = Join-Path $env:TEMP 'oberth-uninstall-cleanup.cmd'
@"
@echo off
timeout /t 2 /nobreak >nul
rmdir /s /q "$installDir"
del "%~f0"
"@ | Set-Content -LiteralPath $cleanup -Encoding ASCII
Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', "`"$cleanup`"" -WindowStyle Hidden
