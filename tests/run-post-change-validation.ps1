$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$haloScript = Join-Path $repoRoot 'halo'

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

function Invoke-Step {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Label,
        [Parameter(Mandatory = $true)]
        [scriptblock]$Action
    )

    Write-Host "==> $Label" -ForegroundColor Cyan
    & $Action
}

function Invoke-PythonPyCompile {
    param([string[]]$Files)

    if (Get-Command python -ErrorAction SilentlyContinue) {
        & python -m py_compile @Files
    } elseif (Get-Command py -ErrorAction SilentlyContinue) {
        & py -3 -m py_compile @Files
    } else {
        throw 'Python launcher not found. Expected python or py.'
    }

    if ($LASTEXITCODE -ne 0) {
        throw 'Python bytecode compilation failed.'
    }
}

Push-Location $repoRoot
try {
    Invoke-Step 'Validate halo shell syntax' {
        $wslHaloScript = Convert-ToWslPath $haloScript
        wsl bash -n $wslHaloScript
        if ($LASTEXITCODE -ne 0) {
            throw 'Halo shell syntax check failed.'
        }
    }

    Invoke-Step 'Compile halo Python helpers' {
        Invoke-PythonPyCompile @('halo_cache.py', 'halo_review.py')
    }

    Invoke-Step 'Run Go tests' {
        & go test ./...
        if ($LASTEXITCODE -ne 0) {
            throw 'go test ./... failed.'
        }
    }

    Invoke-Step 'Run HAL aware tests' {
        & pwsh -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'run-halo-aware-tests.ps1')
        if ($LASTEXITCODE -ne 0) {
            throw 'HAL aware tests failed.'
        }
    }

    Invoke-Step 'Run HAL review tests' {
        & pwsh -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'run-halo-review-tests.ps1')
        if ($LASTEXITCODE -ne 0) {
            throw 'HAL review tests failed.'
        }
    }

    Invoke-Step 'Run HAL sensory tests' {
        & pwsh -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'run-halo-sensory-tests.ps1')
        if ($LASTEXITCODE -ne 0) {
            throw 'HAL sensory tests failed.'
        }
    }

    Write-Host 'All mercury-tts post-change validation steps passed.' -ForegroundColor Green
} finally {
    Pop-Location
}