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

# Source directory: where the Dockerfile and source code live.
# - If REPO_URL is set, clone/pull from git into InstallDir.
# - Otherwise assume the installer was distributed as a zip and is
#   running from inside the extracted source folder (PSScriptRoot).
$sourceDir = $PSScriptRoot

if ($env:REPO_URL) {
    # Git-based install (set REPO_URL env var before running)
    if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
        Fail "git is required for a repo-based install. Install Git from https://git-scm.com"
    }
    if (Test-Path (Join-Path $InstallDir ".git")) {
        Info "Updating existing repo in $InstallDir ..."
        git -C $InstallDir pull
    } else {
        Info "Cloning $env:REPO_URL into $InstallDir ..."
        git clone $env:REPO_URL $InstallDir
    }
    $sourceDir = $InstallDir
} elseif ($sourceDir -ne $InstallDir) {
    Info "Copying source files to $InstallDir ..."
    # Exclude large/unnecessary directories from the copy
    $exclude = @(".git", "node_modules", "schemas", "tests", "test-results",
                 "logs", "uploads", "*.tar.gz", "*.zip")
    $robocopyArgs = @($sourceDir, $InstallDir, "/E", "/XD") + $exclude
    & robocopy @robocopyArgs | Out-Null
    OK "Files copied."
    $sourceDir = $InstallDir
}

Set-Location $sourceDir

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

    function Read-Port($label, $default) {
        $raw = Read-Host "  $label [$default]"
        return if ([string]::IsNullOrWhiteSpace($raw)) { $default } else { $raw.Trim() }
    }

    Write-Host "  -- Ports -----------------------------------------------" -ForegroundColor DarkGray
    $appPort      = Read-Port "Web UI port"           "3000"
    $apiPort      = Read-Port "Go API port"           "8080"
    $dbHostPort   = Read-Port "PostgreSQL port"       "5432"
    $minioApiPort = Read-Port "MinIO API port"        "9000"
    $minioConPort = Read-Port "MinIO console port"    "9001"

    Write-Host ""
    Write-Host "  -- Passwords -------------------------------------------" -ForegroundColor DarkGray
    Write-Host "  Choose a strong database password:" -ForegroundColor White
    $dbPassword = Prompt-Secret "Database password"

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
API_PORT=$apiPort

DB_USER=ezhealth_user
DB_PASSWORD=$dbPassword
DB_HOST_PORT=$dbHostPort
DB_SSL=false

MINIO_USER=ezhealth_user
MINIO_PASSWORD=$minioPassword
MINIO_API_PORT=$minioApiPort
MINIO_CONSOLE_PORT=$minioConPort

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
docker build -t ezhealthkonnect/app:latest $sourceDir
$buildExit = $LASTEXITCODE
$ErrorActionPreference = 'Stop'
if ($buildExit -ne 0) { Fail "Image build failed. Check the output above." }
OK "Image built successfully."

Write-Host ""
Info "Starting services..."
Write-Host ""

$composeArgs = @(
    "compose",
    "-f", (Join-Path $sourceDir "docker-compose.prod.yml"),
    "-f", (Join-Path $sourceDir "docker-compose.listeners.yml"),
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

# Register as a Windows Service (visible in services.msc) using NSSM
$svcName     = "ezHealthKonnect"
$nssmDir     = Join-Path $sourceDir "tools"
$nssmExe     = Join-Path $nssmDir "nssm.exe"
$composeFile = Join-Path $sourceDir "docker-compose.prod.yml"

$existingSvc = Get-Service -Name $svcName -ErrorAction SilentlyContinue
if ($existingSvc) {
    OK "Windows service '$svcName' already registered."
} else {
    Write-Host ""
    $registerSvc = Read-Host "  Register as a Windows service (services.msc)? [Y/n]"
    if ($registerSvc -ne 'n' -and $registerSvc -ne 'N') {
        try {
            # Download NSSM if not already present
            if (-not (Test-Path $nssmExe)) {
                New-Item -ItemType Directory -Path $nssmDir -Force | Out-Null
                Info "Downloading NSSM service manager..."
                $nssmZip = Join-Path $nssmDir "nssm.zip"
                Invoke-WebRequest -Uri "https://nssm.cc/release/nssm-2.24.zip" `
                    -OutFile $nssmZip -UseBasicParsing
                Expand-Archive -Path $nssmZip -DestinationPath $nssmDir -Force
                # nssm extracts into nssm-2.24\win64\nssm.exe
                $extracted = Get-ChildItem "$nssmDir\nssm-*\win64\nssm.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
                if ($extracted) {
                    Copy-Item $extracted.FullName $nssmExe -Force
                }
                Remove-Item $nssmZip -Force -ErrorAction SilentlyContinue
                OK "NSSM downloaded."
            }

            $dockerExe = (Get-Command docker).Source

            # Install service: runs "docker compose up" in foreground so
            # Windows can start/stop it properly via services.msc
            $listenersFile = Join-Path $sourceDir "docker-compose.listeners.yml"
            & $nssmExe install $svcName $dockerExe `
                "compose -f `"$composeFile`" -f `"$listenersFile`" --env-file `"$envFile`" up" | Out-Null

            # Service settings
            & $nssmExe set $svcName DisplayName  "ezHealthKonnect"          | Out-Null
            & $nssmExe set $svcName Description  "ezHealthKonnect Healthcare Integration Platform" | Out-Null
            & $nssmExe set $svcName Start        SERVICE_AUTO_START         | Out-Null
            & $nssmExe set $svcName AppStopMethodConsole 10000              | Out-Null
            & $nssmExe set $svcName AppStopMethodWindow  5000               | Out-Null
            & $nssmExe set $svcName AppStopMethodThreads 5000               | Out-Null

            # Depend on Docker so we start after it
            & $nssmExe set $svcName DependOnService "com.docker.service"    | Out-Null

            # Stdout/stderr logs
            $logDir = Join-Path $sourceDir "logs"
            New-Item -ItemType Directory -Path $logDir -Force | Out-Null
            & $nssmExe set $svcName AppStdout (Join-Path $logDir "service.log") | Out-Null
            & $nssmExe set $svcName AppStderr (Join-Path $logDir "service.log") | Out-Null
            & $nssmExe set $svcName AppRotateFiles 1                        | Out-Null

            Start-Service $svcName -ErrorAction SilentlyContinue
            OK "Service '$svcName' registered and set to auto-start."
            OK "Manage it from services.msc or: Start-Service $svcName"
        } catch {
            Warn "Could not register service: $_"
            Warn "Re-run as Administrator, or start manually:"
            Warn "  docker compose -f `"$composeFile`" up -d"
        }
    }
}

Write-Host ""
Write-Host "  To stop:      Stop-Service $svcName" -ForegroundColor DarkGray
Write-Host "  To start:     Start-Service $svcName" -ForegroundColor DarkGray
Write-Host "  Logs:         $sourceDir\logs\service.log" -ForegroundColor DarkGray
Write-Host "  Uninstall:    & `"$nssmExe`" remove $svcName confirm" -ForegroundColor DarkGray
Write-Host ""

Start-Sleep 5
Start-Process $url
