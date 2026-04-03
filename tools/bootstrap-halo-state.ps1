param(
    [string]$ProjectRoot = "f:/DEVELOPMENT/FALCON_LOCAL/mercury-tts",
    [int]$PostgresPort = 15433,
    [int]$QdrantPort = 16333,
    [switch]$SkipComposeUp,
    [switch]$SkipMigration,
    [switch]$SkipIngestion,
    [string]$AwareDbPath = "Z:/hal-system-monitor/state/halo/aware-memory.sqlite3",
    [string]$SensoryDbPath = "Z:/hal-system-monitor/state/halo/sensory/knowledge.sqlite3",
    [string]$CommentaryDbPath = "Z:/hal-system-monitor/state/halo/commentary-history.sqlite3",
    [string]$DocumentRoot = "Z:/hal-system-monitor/learning_matrial"
)

$ErrorActionPreference = 'Stop'

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
$env:HALO_STATE_BACKEND = 'postgres'
$env:HALO_STATE_POSTGRES_DSN = "postgresql://halo_state:halo_state_dev@localhost:$PostgresPort/halo_state"
$env:HALO_STATE_POSTGRES_SCHEMA = 'halo'
$env:HALO_STATE_PROFILE_KEY = 'hal-9000'
$env:HALO_STATE_QDRANT_URL = "http://localhost:$QdrantPort"
$env:HALO_STATE_QDRANT_COLLECTION = 'halo-memory'
$env:HALO_STATE_DOCUMENT_ROOT = $DocumentRoot

if (-not $SkipMigration) {
    $migrationArgs = @('-m', 'halo_state.cli', 'migrate-legacy-state')
    if (Test-Path $AwareDbPath) {
        $migrationArgs += @('--aware-db', $AwareDbPath)
    }
    if (Test-Path $SensoryDbPath) {
        $migrationArgs += @('--sensory-db', $SensoryDbPath)
    }
    if (Test-Path $CommentaryDbPath) {
        $migrationArgs += @('--commentary-db', $CommentaryDbPath)
    }
    & python @migrationArgs
}

if (-not $SkipIngestion -and (Test-Path $DocumentRoot)) {
    & python -m halo_state.cli ingest-learning-material --document-root $DocumentRoot
}
