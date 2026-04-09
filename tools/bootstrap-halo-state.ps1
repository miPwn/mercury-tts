param(
    [string]$ProjectRoot,
    [int]$PostgresPort = 15433,
    [int]$QdrantPort = 16333,
    [switch]$SkipComposeUp,
    [switch]$SkipIngestion,
    [string]$DocumentRoot = "Z:/hal-system-monitor/learning_matrial"
)

$ErrorActionPreference = 'Stop'

$scriptRepoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path

if ([string]::IsNullOrWhiteSpace($ProjectRoot)) {
    $workspaceRoot = $env:HALO_WORKSPACE_ROOT
    if (-not [string]::IsNullOrWhiteSpace($workspaceRoot)) {
        $workspaceRepoRoot = Join-Path $workspaceRoot 'mercury-tts'
        if (Test-Path -LiteralPath $workspaceRepoRoot) {
            $ProjectRoot = (Resolve-Path -LiteralPath $workspaceRepoRoot).Path
        }
    }

    if ([string]::IsNullOrWhiteSpace($ProjectRoot)) {
        $ProjectRoot = $scriptRepoRoot
    }
}
else {
    $ProjectRoot = (Resolve-Path -LiteralPath $ProjectRoot).Path
}

Set-Location $ProjectRoot

if (-not $SkipComposeUp) {
    docker compose -f docker-compose.halo-state.yml up -d
}

$env:PGPASSWORD = 'halo_state_dev'

for ($i = 0; $i -lt 30; $i++) {
    & pg_isready -h localhost -p $PostgresPort -U halo_state | Out-Null
    if ($LASTEXITCODE -eq 0) {
        break
    }
    Start-Sleep -Milliseconds 500
}

& psql -h localhost -p $PostgresPort -U halo_state -d halo_state -f db/halo_state_postgres.sql

$env:PYTHONPATH = $ProjectRoot
$env:HALO_STATE_POSTGRES_DSN = "postgresql://halo_state:halo_state_dev@localhost:$PostgresPort/halo_state"
$env:HALO_STATE_POSTGRES_SCHEMA = 'halo'
$env:HALO_STATE_PROFILE_KEY = 'hal-9000'
$env:HALO_STATE_QDRANT_URL = "http://localhost:$QdrantPort"
$env:HALO_STATE_QDRANT_COLLECTION = 'halo-memory'
$env:HALO_STATE_DOCUMENT_ROOT = $DocumentRoot

if (-not $SkipIngestion -and (Test-Path $DocumentRoot)) {
    & python -m halo_state.cli ingest-learning-material --document-root $DocumentRoot
}
