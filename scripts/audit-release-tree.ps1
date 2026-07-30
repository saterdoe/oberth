$ErrorActionPreference = "Stop"

$trackedRuntime = @(git ls-files -- data .tmp-qa)
if ($trackedRuntime.Count -gt 0) {
    Write-Error "Release blocked: $($trackedRuntime.Count) runtime/QA files are tracked under data/ or .tmp-qa/."
}

if ($LASTEXITCODE -ne 0) {
    throw "Unable to inspect the Git working tree."
}

Write-Host "Release tree audit passed. No runtime or QA download files are tracked."
