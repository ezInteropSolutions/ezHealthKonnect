#Requires -Version 5.1
<#
.SYNOPSIS
    ezHealthKonnect - Docker Edition Installer (Windows)

.DESCRIPTION
    Installs ezHealthKonnect on any Windows machine that has Docker Desktop.
    The script:
      1. Checks prerequisites (Docker, Docker Compose)
      2. Prompts for configuration (ports, passwords)
      3. Generates a secure .env file
      4. Builds the application image
      5. Starts the stack
      6. Opens the browser to the setup wizard

.PARAMETER InstallDir
    Where to install ezHealthKonnect files. Default: C:\ezHealthKonnect

.PARAMETER Reconfigure
    Force re-entry of all configuration values (regenerates .env).

.PARAMETER WithAI
    Also start the Ollama AI service and pull the LLM model (~2.3 GB download).
#>
param(
    [string] $InstallDir  = "C:\ezHealthKonnect",
    [switch] $Reconfigure,
    [switch] $WithAI
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Info  ($msg) { Write-Host "  $msg" -ForegroundColor Cyan   }
function OK    ($msg) { Write-Host "  [OK] $msg" -ForegroundColor Green  }
function Warn  ($msg) { Write-Host "  [!]  $msg" -ForegroundColor Yellow }
function Fail  ($msg) { Write-Host "  [X]  $msg" -ForegroundColor Red; exit 1 }
function Header($msg) {
    Write-Host ""
    Write-Host ("-" * 60) -ForegroundColor DarkCyan
    Write-Host "  $msg" -ForegroundColor White
    Write-Host ("-" * 60) -ForegroundColor DarkCyan
}

function Prompt-Secret($prompt) {
    do {
        $ss = Read-Host "  $prompt" -AsSecureString
        $plain = [System.Runtime.InteropServices.Marshal]::PtrToStringAuto(
                    [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($ss))
    } while ([string]::IsNullOrWhiteSpace($plain))
    return $plain
}

function New-RandomSecret([int]$bytes = 48) {
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    $buf = New-Object byte[] $bytes
    $rng.GetBytes($buf)
    return [Convert]::ToBase64String($buf)
}

# Banner
Clear-Host
Write-Host ""
Write-Host "  +--------------------------------------------------+" -ForegroundColor DarkCyan
Write-Host "  |      ezHealthKonnect - Docker Edition            |" -ForegroundColor White
Write-Host "  |      Healthcare Integration Platform             |" -ForegroundColor Gray
Write-Host "  |      Copyright 2025-2026 ezInterop Solutions     |" -ForegroundColor Gray
Write-Host "  +--------------------------------------------------+" -ForegroundColor DarkCyan
Write-Host ""

# Step 1: Prerequisites
Header "Step 1 of 4 - Checking Prerequisites"

try {
    $dockerVersion = (docker version --format '{{.Server.Version}}' 2>$null)
    if (-not $dockerVersion) { throw "not running" }
    OK "Docker Engine $dockerVersion"
} catch {
    Fail "Docker is not running or not installed. Install from: https://www.docker.com/products/docker-desktop/"
}

try {
    $composeVersion = (docker compose version --short 2>$null)
    if (-not $composeVersion) { throw "not found" }
    OK "Docker Compose $composeVersion"
} catch {
    Fail "Docker Compose v2 not found. Please update Docker Desktop."
}

# Step 2: Install directory
Header "Step 2 of 4 - Install Location"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
    OK "Created: $InstallDir"
} else {
    OK "Install directory: $InstallDir"
}

$scriptDir = $PSScriptRoot
if ($scriptDir -ne $InstallDir) {
    Info "Copying application files to $InstallDir ..."
    $filesToCopy = @(
        "Dockerfile", "docker-compose.prod.yml",
        "package.json", "package-lock.json", "go.mod", "go.sum",
        "app.js", "server.js", "main.go"
    )
    $dirsToCopy = @(
        "controllers", "services", "middleware", "routes", "config",
        "models", "processing", "utils", "public", "database"
        # schemas/ is intentionally excluded - downloaded on demand via Schema Package Manager
    )
    foreach ($f in $filesToCopy) {
        $src = Join-Path $scriptDir $f
        if (Test-Path $src) { Copy-Item $src $InstallDir -Force }
    }
    foreach ($d in $dirsToCopy) {
        $src = Join-Path $scriptDir $d
        if (Test-Path $src) { Copy-Item $src (Join-Path $InstallDir $d) -Recurse -Force }
    }
    OK "Files copied."
}

Set-Location $InstallDir

# Step 3: Configuration
Header "Step 3 of 4 - Configuration"

$envFile = Join-Path $InstallDir ".env"

if ((Test-Path $envFile) -and -not $Reconfigure) {
    OK ".env already exists - skipping (use -Reconfigure to change)."
} else {
    Write-Host ""
    Write-Host "  A few questions to configure your installation." -ForegroundColor White
    Write-Host "  Press Enter to accept the default shown in [brackets]." -ForegroundColor Gray
    Write-Host ""

    $appPortRaw = Read-Host "  Web UI port [3000]"
    $appPort = if ([string]::IsNullOrWhiteSpace($appPortRaw)) { "3000" } else { $appPortRaw.Trim() }

    Write-Host ""
    Write-Host "  Choose a strong database password:" -ForegroundColor White
    $dbPassword = Prompt-Secret "Database password"

    Write-Host ""
    Write-Host "  Choose a storage password (MinIO - for message archiving):" -ForegroundColor White
    $minioPassword = Prompt-Secret "Storage password"

    $sessionSecret = New-RandomSecret 48
    $jwtSecret     = New-RandomSecret 48

    $envContent = @"
# ezHealthKonnect - Production Environment
# Generated by install.ps1 on $(Get-Date -Format 'yyyy-MM-dd HH:mm')
# Keep this file private - it contains secrets.

APP_IMAGE=ezhealthkonnect/app:latest
APP_PORT=$appPort
API_PORT=8080

DB_USER=ezhealth_user
DB_PASSWORD=$dbPassword
DB_PORT=5432
DB_SSL=false

MINIO_USER=ezhealth_user
MINIO_PASSWORD=$minioPassword
MINIO_API_PORT=9000
MINIO_CONSOLE_PORT=9001

SESSION_SECRET=$sessionSecret
JWT_SECRET=$jwtSecret

OLLAMA_PORT=11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
"@

    Set-Content -Path $envFile -Value $envContent -Encoding UTF8
    OK ".env written to $envFile"
}

# Step 4: Build and Start
Header "Step 4 of 4 - Building and Starting ezHealthKonnect"

Write-Host ""
Info "Building application image (this takes 3-6 minutes on first run)..."
Write-Host ""

$ErrorActionPreference = 'Continue'
docker build -t ezhealthkonnect/app:latest $InstallDir
$buildExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($buildExit -ne 0) { Fail "Image build failed. Check the output above." }
OK "Image built successfully."

Write-Host ""
Info "Starting services..."
Write-Host ""

$composeArgs = @(
    "compose",
    "-f", (Join-Path $InstallDir "docker-compose.prod.yml"),
    "--env-file", $envFile
)
if ($WithAI) { $composeArgs += @("--profile", "ai") }
$composeArgs += @("up", "-d", "--remove-orphans")

$ErrorActionPreference = 'Continue'
& docker @composeArgs
$composeExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($composeExit -ne 0) { Fail "Failed to start services. Check the output above." }

# Done
$port = "3000"
Get-Content $envFile | ForEach-Object {
    if ($_ -match '^APP_PORT=(.+)') { $port = $Matches[1].Trim() }
}
$url = "http://localhost:$port"

Write-Host ""
Write-Host ("-" * 60) -ForegroundColor Green
Write-Host "  ezHealthKonnect is starting up!" -ForegroundColor Green
Write-Host ("-" * 60) -ForegroundColor Green
Write-Host ""
Write-Host "  Platform URL : $url" -ForegroundColor Cyan
Write-Host "  Install dir  : $InstallDir" -ForegroundColor Cyan
Write-Host "  Config file  : $envFile" -ForegroundColor Cyan
Write-Host ""
Write-Host "  On first visit you will be guided through a short setup" -ForegroundColor White
Write-Host "  wizard to create your administrator account." -ForegroundColor White
Write-Host ""

if ($WithAI) {
    Write-Host "  AI is enabled. Pulling Ollama model in background (~2.3 GB)..." -ForegroundColor Yellow
    Start-Job -ScriptBlock {
        Start-Sleep 30
        docker exec ezhk-ollama ollama pull llama3.2:3b
        docker exec ezhk-ollama ollama pull nomic-embed-text
    } | Out-Null
}

Write-Host "  To stop:    docker compose -f docker-compose.prod.yml down" -ForegroundColor DarkGray
Write-Host "  To restart: docker compose -f docker-compose.prod.yml up -d" -ForegroundColor DarkGray
Write-Host "  Logs:       docker compose -f docker-compose.prod.yml logs -f app" -ForegroundColor DarkGray
Write-Host ""

Start-Sleep 5
Start-Process $url
