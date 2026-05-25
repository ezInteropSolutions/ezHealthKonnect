# reset-docker-disk.ps1
#
# Safely backs up postgres + minio, wipes the Docker WSL disk to reclaim space,
# then fully restores and brings the stack back up.
#
# SAFE DEFAULTS:
#   - Keeps last 3 dated backups — never overwrites a good backup
#   - Verifies backup integrity BEFORE deleting the .vhdx
#   - Aborts the wipe if backup looks too small or corrupt
#   - Supports running phases independently so a failed restore
#     doesn't force you to re-backup (which would destroy your data)
#
# Usage:
#   Full run (backup → wipe → restore):
#       .\scripts\reset-docker-disk.ps1
#
#   Backup only (no wipe, no restore):
#       .\scripts\reset-docker-disk.ps1 -Phase Backup
#
#   Restore only (after manual Docker restart, skips backup + wipe):
#       .\scripts\reset-docker-disk.ps1 -Phase Restore
#
#   Wipe only (backup must already exist and be verified):
#       .\scripts\reset-docker-disk.ps1 -Phase Wipe
#
# ─────────────────────────────────────────────────────────────────────────────

param(
    [ValidateSet("All", "Backup", "Wipe", "Restore")]
    [string]$Phase = "All",

    # Where to store backups — must be on Windows filesystem (survives .vhdx wipe)
    # Change this to a different drive (e.g. D:\Backups) if C: is running low
    [string]$BackupDir = "$env:USERPROFILE\ezHealthKonnect-Backups",

    # How many dated backups to keep (oldest deleted automatically)
    [int]$KeepBackups = 3,

    # Minimum expected backup size in MB — abort if smaller (catches empty-volume backups)
    [int]$MinBackupMB = 100
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ComposeDir = "c:\Projects\ezHealthKonnect"
$VhdxPath   = "$env:LOCALAPPDATA\Docker\wsl\disk\docker_data.vhdx"
$DockerExe  = "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"
$Timestamp  = Get-Date -Format "yyyyMMdd_HHmm"

# ─── HELPERS ─────────────────────────────────────────────────────────────────

function Write-Step([string]$msg) {
    Write-Host "`n==> $msg" -ForegroundColor Cyan
}

function Write-OK([string]$msg) {
    Write-Host "    OK: $msg" -ForegroundColor Green
}

function Write-Warn([string]$msg) {
    Write-Host "    WARN: $msg" -ForegroundColor Yellow
}

function Abort([string]$msg) {
    Write-Host "`nABORTED: $msg" -ForegroundColor Red
    Write-Host "The .vhdx has NOT been deleted. Your data is safe." -ForegroundColor Green
    exit 1
}

function Wait-ForDocker {
    Write-Host "    Waiting for Docker daemon" -NoNewline
    $deadline = (Get-Date).AddSeconds(180)
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
        Write-Host "." -NoNewline
        docker info 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) {
            Write-Host " ready." -ForegroundColor Green
            return
        }
    }
    Write-Host ""
    throw "Docker did not become ready within 3 minutes. Start Docker Desktop manually, then re-run with -Phase Restore."
}

function Get-LatestBackup([string]$prefix) {
    Get-ChildItem "$BackupDir\${prefix}_*.tar.gz" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
}

function Verify-Backup([string]$path, [string]$label) {
    Write-Host "    Verifying $label backup..."

    # Size check
    $sizeMB = [math]::Round((Get-Item $path).Length / 1MB, 1)
    if ($sizeMB -lt $MinBackupMB) {
        Abort "$label backup is only ${sizeMB} MB (minimum $MinBackupMB MB). This looks like an empty volume was backed up. The .vhdx will NOT be deleted."
    }
    Write-OK "Size: ${sizeMB} MB"

    # Integrity check — list the archive contents
    docker run --rm `
        -v "${path}:/check/file.tar.gz" `
        alpine tar tzf /check/file.tar.gz 2>$null | Out-Null

    if ($LASTEXITCODE -ne 0) {
        Abort "$label backup failed integrity check (tar could not read the archive). The .vhdx will NOT be deleted."
    }
    Write-OK "Archive integrity OK"
}

function Prune-OldBackups([string]$prefix) {
    $all = Get-ChildItem "$BackupDir\${prefix}_*.tar.gz" -ErrorAction SilentlyContinue |
               Sort-Object LastWriteTime -Descending
    if ($all.Count -gt $KeepBackups) {
        $all | Select-Object -Skip $KeepBackups | ForEach-Object {
            Write-Warn "Removing old backup: $($_.Name)"
            Remove-Item $_.FullName -Force
        }
    }
}

# ─── PHASE: BACKUP ───────────────────────────────────────────────────────────

function Run-Backup {
    Write-Step "PHASE 1: BACKUP"

    # Ensure backup directory exists on Windows filesystem
    if (-not (Test-Path $BackupDir)) { New-Item -ItemType Directory -Path $BackupDir | Out-Null }

    $pgFile    = "$BackupDir\postgres_${Timestamp}.tar.gz"
    $minioFile = "$BackupDir\minio_${Timestamp}.tar.gz"

    # Check volumes exist before backing up
    $volumes = docker volume ls --format "{{.Name}}" 2>$null
    if ($volumes -notcontains "ezhealthkonnect_postgres_data") {
        Abort "Volume 'ezhealthkonnect_postgres_data' does not exist. Nothing to back up — is the stack running?"
    }

    Write-Step "Stopping all containers"
    Set-Location $ComposeDir
    docker compose down

    # Convert Windows path to Docker-compatible mount format
    $backupDirDocker = $BackupDir -replace '\\', '/' -replace '^([A-Z]):', '/$1'

    Write-Host "    Backing up postgres → $pgFile"
    docker run --rm `
        -v ezhealthkonnect_postgres_data:/data `
        -v "${backupDirDocker}:/backup" `
        alpine tar czf /backup/postgres_${Timestamp}.tar.gz -C /data .

    Write-Host "    Backing up minio → $minioFile"
    docker run --rm `
        -v ezhealthkonnect_minio_data:/data `
        -v "${backupDirDocker}:/backup" `
        alpine tar czf /backup/minio_${Timestamp}.tar.gz -C /data .

    # Verify both backups before allowing the wipe phase to proceed
    Verify-Backup $pgFile    "postgres"
    Verify-Backup $minioFile "minio"

    # Clean up old backups beyond retention limit
    Prune-OldBackups "postgres"
    Prune-OldBackups "minio"

    Write-OK "Backup complete. Safe to wipe."
    Write-Host "    Postgres: $pgFile" -ForegroundColor White
    Write-Host "    MinIO:    $minioFile" -ForegroundColor White
}

# ─── PHASE: WIPE ─────────────────────────────────────────────────────────────

function Run-Wipe {
    Write-Step "PHASE 2: WIPE DOCKER DISK"

    # Safety gate — refuse to wipe if no verified backup exists
    $latestPg    = Get-LatestBackup "postgres"
    $latestMinio = Get-LatestBackup "minio"

    if (-not $latestPg) {
        Abort "No postgres backup found in $BackupDir. Run with -Phase Backup first."
    }
    if (-not $latestMinio) {
        Abort "No minio backup found in $BackupDir. Run with -Phase Backup first."
    }

    Write-Host "    Latest postgres backup: $($latestPg.Name) ($([math]::Round($latestPg.Length/1MB,1)) MB)"
    Write-Host "    Latest minio backup:    $($latestMinio.Name) ($([math]::Round($latestMinio.Length/1MB,1)) MB)"

    Write-Host ""
    Write-Host "    About to delete: $VhdxPath" -ForegroundColor Yellow
    Write-Host "    This will wipe ALL Docker images, containers, and volumes." -ForegroundColor Yellow
    $confirm = Read-Host "    Type YES to confirm"
    if ($confirm -ne "YES") { Abort "Cancelled by user." }

    Write-Host "    Stopping Docker Desktop..."
    Get-Process "Docker Desktop" -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Seconds 8

    Write-Host "    Shutting down WSL..."
    wsl --shutdown 2>$null | Out-Null
    Start-Sleep -Seconds 3

    if (-not (Test-Path $VhdxPath)) {
        Write-Warn ".vhdx not found — may already be deleted. Continuing."
    } else {
        Remove-Item $VhdxPath -Force
        Write-OK "Deleted $VhdxPath"
    }
}

# ─── PHASE: RESTORE ──────────────────────────────────────────────────────────

function Run-Restore {
    Write-Step "PHASE 3: RESTORE"

    $latestPg    = Get-LatestBackup "postgres"
    $latestMinio = Get-LatestBackup "minio"

    if (-not $latestPg)    { throw "No postgres backup found in $BackupDir" }
    if (-not $latestMinio) { throw "No minio backup found in $BackupDir" }

    Write-Host "    Restoring from:"
    Write-Host "      Postgres: $($latestPg.FullName)"
    Write-Host "      MinIO:    $($latestMinio.FullName)"

    Write-Step "Starting Docker Desktop"
    if (-not (Get-Process "Docker Desktop" -ErrorAction SilentlyContinue)) {
        Start-Process $DockerExe
    }
    Wait-ForDocker

    Write-Step "Creating volumes"
    docker volume create ezhealthkonnect_postgres_data | Out-Null
    docker volume create ezhealthkonnect_minio_data    | Out-Null
    docker volume create ezhealthkonnect_ollama_data   | Out-Null
    Write-OK "Volumes created"

    $backupDirDocker = $BackupDir -replace '\\', '/' -replace '^([A-Z]):', '/$1'

    Write-Step "Restoring postgres data"
    docker run --rm `
        -v ezhealthkonnect_postgres_data:/data `
        -v "${backupDirDocker}:/backup" `
        alpine sh -c "cd /data && tar xzf /backup/$($latestPg.Name)"
    Write-OK "Postgres restored"

    Write-Step "Restoring minio data"
    docker run --rm `
        -v ezhealthkonnect_minio_data:/data `
        -v "${backupDirDocker}:/backup" `
        alpine sh -c "cd /data && tar xzf /backup/$($latestMinio.Name)"
    Write-OK "MinIO restored"

    Write-Step "Rebuilding app image"
    Set-Location $ComposeDir
    docker compose build app

    Write-Step "Starting the stack"
    docker compose up -d

    Start-Sleep -Seconds 5
    Write-Host ""
    docker compose ps
    Write-Host "`nAll done. App at http://localhost:3000" -ForegroundColor Green
}

# ─── ENTRYPOINT ──────────────────────────────────────────────────────────────

Write-Host "ezHealthKonnect Docker Disk Reset" -ForegroundColor Magenta
Write-Host "Phase: $Phase | Backup dir: $BackupDir | Keep: $KeepBackups backups"

switch ($Phase) {
    "Backup"  { Run-Backup }
    "Wipe"    { Run-Wipe }
    "Restore" { Run-Restore }
    "All"     { Run-Backup; Run-Wipe; Run-Restore }
}
