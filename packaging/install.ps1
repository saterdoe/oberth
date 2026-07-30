$ErrorActionPreference = 'Stop'
$source = Split-Path -Parent $MyInvocation.MyCommand.Path
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\oberth'
$startMenu = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
Copy-Item -LiteralPath (Join-Path $source 'oberth.exe') -Destination $installDir -Force
Copy-Item -LiteralPath (Join-Path $source 'oberth-server.exe') -Destination $installDir -Force
Copy-Item -LiteralPath (Join-Path $source 'uninstall.cmd') -Destination $installDir -Force
Copy-Item -LiteralPath (Join-Path $source 'uninstall.ps1') -Destination $installDir -Force

$shell = New-Object -ComObject WScript.Shell
foreach ($shortcutPath in @(
    (Join-Path $startMenu 'oberth.lnk'),
    (Join-Path ([Environment]::GetFolderPath('Desktop')) 'oberth.lnk')
)) {
    $shortcut = $shell.CreateShortcut($shortcutPath)
    $shortcut.TargetPath = Join-Path $installDir 'oberth.exe'
    $shortcut.WorkingDirectory = $installDir
    $shortcut.Description = 'oberth — verified AI delivery workspace'
    $shortcut.Save()
}

Start-Process -FilePath (Join-Path $installDir 'oberth.exe')
