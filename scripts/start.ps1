param(
    [switch]$Stop,
    [switch]$NoOpen,
    [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$runtime = Join-Path $root "data\runtime"
$apiPidFile = Join-Path $runtime "api.pid"
$uiPidFile = Join-Path $runtime "ui.pid"
$apiOutLog = Join-Path $runtime "api.out.log"
$apiErrLog = Join-Path $runtime "api.err.log"
$uiOutLog = Join-Path $runtime "ui.out.log"
$uiErrLog = Join-Path $runtime "ui.err.log"

function Stop-ManagedProcess([string]$PidFile, [string]$Name, [switch]$GracefulAPI) {
    if (-not (Test-Path -LiteralPath $PidFile)) { return }
    $processID = [int](Get-Content -LiteralPath $PidFile -Raw)
    $process = Get-Process -Id $processID -ErrorAction SilentlyContinue
    if ($process) {
        if ($GracefulAPI) {
            try {
                $token = (Get-Content -LiteralPath (Join-Path $root "data\local-token") -Raw).Trim()
                Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:9090/api/v1/system/shutdown" -Headers @{ Authorization = "Bearer $token" } -ContentType "application/json" -Body "{}" -TimeoutSec 3 | Out-Null
                $process.WaitForExit(60000) | Out-Null
            } catch {
                Write-Warning "Graceful API shutdown failed; using process-tree fallback."
            }
        }
        if (-not $process.HasExited) {
            & taskkill.exe /PID $processID /T /F 2>$null | Out-Null
        }
        Write-Host "Stopped $Name (PID $processID)."
    }
    Remove-Item -LiteralPath $PidFile -Force -ErrorAction SilentlyContinue
}

function Test-LocalPort([int]$Port) {
    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $task = $client.ConnectAsync("127.0.0.1", $Port)
        return $task.Wait(250) -and $client.Connected
    } catch {
        return $false
    } finally {
        $client.Dispose()
    }
}

function Wait-Health([string]$URL, [int]$Seconds, [string]$LogPath) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri $URL -TimeoutSec 2
            if ($response.StatusCode -eq 200) { return }
        } catch {
            Start-Sleep -Milliseconds 300
        }
    }
    $tail = ""
    if (Test-Path -LiteralPath $LogPath) {
        $tail = (Get-Content -LiteralPath $LogPath -Tail 20) -join [Environment]::NewLine
    }
    throw "Service did not become healthy at $URL.`n$tail"
}

New-Item -ItemType Directory -Force -Path $runtime | Out-Null

if ($Stop) {
    Stop-ManagedProcess $uiPidFile "UI"
    Stop-ManagedProcess $apiPidFile "API" -GracefulAPI
    Write-Host "oberth stopped." -ForegroundColor Green
    exit 0
}

$go = (Get-Command go -ErrorAction SilentlyContinue).Source
$npm = (Get-Command npm.cmd -ErrorAction SilentlyContinue).Source
$git = (Get-Command git -ErrorAction SilentlyContinue).Source
if (-not $go) { throw "Go 1.25+ is required. Install Go and run .\start.cmd again." }
if (-not $npm) { throw "Node.js 22+ is required for the UI. Install Node.js and run .\start.cmd again." }
if (-not $git) { throw "Git is required. Install Git and run .\start.cmd again." }

Push-Location $root
try {
    if (-not (Test-Path -LiteralPath (Join-Path $root "ui\node_modules"))) {
        Write-Host "Installing UI dependencies (first start only)..."
        & $npm --prefix ui ci
        if ($LASTEXITCODE -ne 0) { throw "UI dependency installation failed." }
    }

    $server = Join-Path $root "bin\oberth-server.exe"
    $cli = Join-Path $root "bin\oberth.exe"
    $sourceLatest = Get-ChildItem -Path (Join-Path $root "cmd"),(Join-Path $root "internal"),(Join-Path $root "pkg") -Recurse -Filter *.go |
        Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
    $needsBuild = $Rebuild -or -not (Test-Path -LiteralPath $server) -or
        -not (Test-Path -LiteralPath $cli) -or
        $sourceLatest.LastWriteTimeUtc -gt (Get-Item -LiteralPath $server).LastWriteTimeUtc
    if ($needsBuild) {
        Write-Host "Building current oberth binaries..."
        & $go build -trimpath -o $cli ./cmd/oberth
        if ($LASTEXITCODE -ne 0) { throw "CLI build failed." }
        & $go build -trimpath -o $server ./cmd/oberth-server
        if ($LASTEXITCODE -ne 0) { throw "API build failed." }
    }

    $tokenFile = Join-Path $root "data\local-token"
    if (Test-Path -LiteralPath $tokenFile) {
        $token = (Get-Content -LiteralPath $tokenFile -Raw).Trim()
    } else {
        $tokenBytes = New-Object byte[] 32
        $tokenGenerator = [Security.Cryptography.RandomNumberGenerator]::Create()
        try {
            $tokenGenerator.GetBytes($tokenBytes)
        } finally {
            $tokenGenerator.Dispose()
        }
        $token = ([BitConverter]::ToString($tokenBytes) -replace '-', '').ToLowerInvariant()
        [System.IO.File]::WriteAllText($tokenFile, $token)
    }
    $env:OBERTH_AUTH_TOKEN = $token
    $env:VITE_API_TOKEN = $token
    $userStateDir = Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::ApplicationData)) "oberth"
    New-Item -ItemType Directory -Force -Path $userStateDir | Out-Null
    [System.IO.File]::WriteAllText((Join-Path $userStateDir "local-token"), $token)

    if (-not (Test-LocalPort 9090)) {
        $arguments = @()
        $config = Join-Path $root ".oberth.yaml"
        if (Test-Path -LiteralPath $config) { $arguments += @("--config", $config) }
        $startOptions = @{
            FilePath = $server
            WorkingDirectory = $root
            WindowStyle = "Hidden"
            PassThru = $true
            RedirectStandardOutput = $apiOutLog
            RedirectStandardError = $apiErrLog
        }
        if ($arguments.Count -gt 0) { $startOptions.ArgumentList = $arguments }
        $api = Start-Process @startOptions
        Set-Content -LiteralPath $apiPidFile -Value $api.Id -NoNewline
    }
    Wait-Health "http://127.0.0.1:9090/api/v1/health" 30 $apiErrLog

    if (-not (Test-LocalPort 5173)) {
        $ui = Start-Process -FilePath $npm -ArgumentList @("--prefix","ui","run","dev","--","--port","5173") -WorkingDirectory $root -WindowStyle Hidden -PassThru -RedirectStandardOutput $uiOutLog -RedirectStandardError $uiErrLog
        Set-Content -LiteralPath $uiPidFile -Value $ui.Id -NoNewline
    }
    Wait-Health "http://127.0.0.1:5173/" 30 $uiErrLog

    Write-Host ""
    Write-Host "oberth is ready." -ForegroundColor Green
    Write-Host "  App: http://127.0.0.1:5173/"
    Write-Host "  API: http://127.0.0.1:9090/"
    Write-Host "  Stop: .\start.cmd -Stop"
    Write-Host "  Logs: data\runtime\"

    if (-not $NoOpen) {
        Start-Process "http://127.0.0.1:5173/"
    }
} finally {
    Pop-Location
}
