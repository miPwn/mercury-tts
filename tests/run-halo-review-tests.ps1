$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$haloScript = Join-Path $repoRoot 'halo'
$fallbackHaloScript = Join-Path $repoRoot 'halo.remote'
$testScript = Join-Path $PSScriptRoot 'test-halo-review.sh'

if (-not (Test-Path $haloScript) -and (Test-Path $fallbackHaloScript)) {
    $haloScript = $fallbackHaloScript
}

function Convert-ToWslPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WindowsPath
    )

    $resolved = (Resolve-Path $WindowsPath).Path
    if ($resolved -notmatch '^([A-Za-z]):\\(.*)$') {
        throw "Unsupported Windows path: $resolved"
    }

    $drive = $matches[1].ToLowerInvariant()
    $rest = $matches[2] -replace '\\', '/'
    return "/mnt/$drive/$rest"
}

$wslHaloScript = Convert-ToWslPath $haloScript
$wslTestScript = Convert-ToWslPath $testScript

wsl bash -n $wslHaloScript
if ($LASTEXITCODE -ne 0) {
    throw "Halo script syntax check failed."
}

wsl bash $wslTestScript
if ($LASTEXITCODE -ne 0) {
    throw "HAL review tests failed."
}