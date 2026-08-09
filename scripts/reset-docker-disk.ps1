# reset-docker-disk.ps1
#
# Safely backs up postgres + minio, wipes the Docker WSL disk to reclaim space,
# then fully restores and brings the stack back up.
#
# Usage:
#   Full run (backup, wipe, restore):
#       .\scripts\reset-docker-disk.ps1
#
#   Backup only:
#       .\scripts\reset-docker-disk.ps1 -Phase Backup
#
#   Restore only (after a failed restore or manual Docker restart):
#       .\scripts\reset-docker-disk.ps1 -Phase Restore
#
# ---------------------------------------------------------------------------

param(
    [ValidateSet("All","Backup","Wipe","Restore")]
    [string]$Phase = "All",

    [string]$BackupDir = "$env:USERPROFILE\ezHealthKonnect-Backups",

    [int]$KeepBackups = 3,

    [int]$MinBackupMB = 100,

    # Skip the interactive "Type YES" confirmation in the Wipe phase.
    # Used by the one-click .bat launcher; do NOT pass this flag manually
    # unless you are certain a valid backup already exists.
    [switch]$AutoConfirm
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$ComposeDir = "c:\Projects\ezHealthKonnect"
$VhdxPath   = "$env:LOCALAPPDATA\Docker\wsl\disk\docker_data.vhdx"
$DockerExe  = "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"
$Timestamp  = Get-Date -Format "yyyyMMdd_HHmm"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Write-Step([string]$msg) {
    Write-Host ""
    Write-Host "==> $msg" -ForegroundColor Cyan
}

function Write-OK([string]$msg) {
    Write-Host "    OK: $msg" -ForegroundColor Green
}

function Write-Warn([string]$msg) {
    Write-Host "    WARN: $msg" -ForegroundColor Yellow
}

function Safe-Abort([string]$msg) {
    Write-Host ""
    Write-Host "ABORTED: $msg" -ForegroundColor Red
    Write-Host "The .vhdx has NOT been deleted. Your data is safe." -ForegroundColor Green
    exit 1
}

function Wait-ForDocker {
    Write-Host "    Waiting for Docker daemon" -NoNewline
    $deadline = (Get-Date).AddSeconds(180)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Seconds 5
        Write-Host "." -NoNewline
        & { $ErrorActionPreference = "SilentlyContinue"; docker info 2>&1 | Out-Null }
        if ($LASTEXITCODE -eq 0) {
            $ready = $true
            break
        }
    }
    if ($ready) {
        Write-Host " ready." -ForegroundColor Green
    } else {
        Write-Host ""
        throw "Docker did not start within 3 minutes. Start Docker Desktop manually then re-run with -Phase Restore."
    }
}

function Get-LatestBackup([string]$prefix) {
    $pattern = Join-Path $BackupDir "${prefix}_*.tar.gz"
    $files = Get-ChildItem $pattern -ErrorAction SilentlyContinue |
             Sort-Object LastWriteTime -Descending
    if ($files) { return $files[0] } else { return $null }
}

function Verify-Backup([string]$filePath, [string]$label) {
    Write-Host "    Verifying $label backup..."
    $sizeMB = [math]::Round((Get-Item $filePath).Length / 1MB, 1)
    if ($sizeMB -lt $MinBackupMB) {
        Safe-Abort "$label backup is only ${sizeMB} MB (minimum $MinBackupMB MB). Looks like an empty volume was backed up. The .vhdx will NOT be deleted."
    }
    Write-OK "Size OK: ${sizeMB} MB"

    $mountPath = $filePath -replace '\\','/' -replace '^([A-Za-z]):','/$1'
    & { $ErrorActionPreference = "SilentlyContinue"; docker run --rm -v "${mountPath}:/check/file.tar.gz" alpine tar tzf /check/file.tar.gz 2>&1 | Out-Null }
    if ($LASTEXITCODE -ne 0) {
        Safe-Abort "$label backup failed integrity check (tar could not read the archive). The .vhdx will NOT be deleted."
    }
    Write-OK "Archive integrity OK"
}

function Prune-OldBackups([string]$prefix) {
    $pattern = Join-Path $BackupDir "${prefix}_*.tar.gz"
    $files = Get-ChildItem $pattern -ErrorAction SilentlyContinue |
             Sort-Object LastWriteTime -Descending
    if ($files.Count -gt $KeepBackups) {
        $toDelete = $files | Select-Object -Skip $KeepBackups
        foreach ($f in $toDelete) {
            Write-Warn "Removing old backup: $($f.Name)"
            Remove-Item $f.FullName -Force
        }
    }
}

function Convert-ToDockerPath([string]$winPath) {
    return $winPath -replace '\\','/' -replace '^([A-Za-z]):','/$1'
}

# ---------------------------------------------------------------------------
# Phase: Backup
# ---------------------------------------------------------------------------

function Run-Backup {
    Write-Step "PHASE 1: BACKUP"

    if (-not (Test-Path $BackupDir)) {
        New-Item -ItemType Directory -Path $BackupDir | Out-Null
    }

    # Ensure Docker is running before we do anything
    & { $ErrorActionPreference = "SilentlyContinue"; docker info 2>&1 | Out-Null }
    if ($LASTEXITCODE -ne 0) {
        Write-Host "    Docker is not running -- starting Docker Desktop..."
        Start-Process $DockerExe
        Wait-ForDocker
    }

    # Check the postgres volume actually exists before backing up
    & { $ErrorActionPreference = "SilentlyContinue"; docker volume inspect ezhealthkonnect_postgres_data 2>&1 | Out-Null }
    if ($LASTEXITCODE -ne 0) {
        Safe-Abort "Volume ezhealthkonnect_postgres_data does not exist. Is the stack running?"
    }

    Write-Step "Stopping all containers"
    Set-Location $ComposeDir
    docker compose down

    $pgFile    = Join-Path $BackupDir "postgres_${Timestamp}.tar.gz"
    $minioFile = Join-Path $BackupDir "minio_${Timestamp}.tar.gz"
    $backupDockerPath = Convert-ToDockerPath $BackupDir

    Write-Host "    Backing up postgres -> $pgFile"
    docker run --rm `
        -v "ezhealthkonnect_postgres_data:/data" `
        -v "${backupDockerPath}:/backup" `
        alpine tar czf "/backup/postgres_${Timestamp}.tar.gz" -C /data .

    Write-Host "    Backing up minio -> $minioFile"
    docker run --rm `
        -v "ezhealthkonnect_minio_data:/data" `
        -v "${backupDockerPath}:/backup" `
        alpine tar czf "/backup/minio_${Timestamp}.tar.gz" -C /data .

    Verify-Backup $pgFile    "postgres"
    Verify-Backup $minioFile "minio"

    Prune-OldBackups "postgres"
    Prune-OldBackups "minio"

    Write-OK "Backup complete. Safe to wipe."
    Write-Host "    Postgres : $pgFile" -ForegroundColor White
    Write-Host "    MinIO    : $minioFile" -ForegroundColor White
}

# ---------------------------------------------------------------------------
# Phase: Wipe
# ---------------------------------------------------------------------------

function Run-Wipe {
    Write-Step "PHASE 2: WIPE DOCKER DISK"

    $latestPg    = Get-LatestBackup "postgres"
    $latestMinio = Get-LatestBackup "minio"

    if (-not $latestPg) {
        Safe-Abort "No postgres backup found in $BackupDir. Run with -Phase Backup first."
    }
    if (-not $latestMinio) {
        Safe-Abort "No minio backup found in $BackupDir. Run with -Phase Backup first."
    }

    $pgMB    = [math]::Round($latestPg.Length / 1MB, 1)
    $minioMB = [math]::Round($latestMinio.Length / 1MB, 1)
    Write-Host "    Latest postgres backup : $($latestPg.Name) (${pgMB} MB)"
    Write-Host "    Latest minio backup    : $($latestMinio.Name) (${minioMB} MB)"
    Write-Host ""
    Write-Host "    About to delete: $VhdxPath" -ForegroundColor Yellow
    Write-Host "    This will wipe ALL Docker images, containers, and volumes." -ForegroundColor Yellow

    if ($AutoConfirm) {
        Write-Host ""
        Write-Host "    AUTO-CONFIRM mode -- proceeding in 10 seconds. Press Ctrl+C to abort." -ForegroundColor Yellow
        for ($i = 10; $i -gt 0; $i--) {
            Write-Host "    $i..." -NoNewline
            Start-Sleep -Seconds 1
        }
        Write-Host ""
    } else {
        $confirm = Read-Host "    Type YES to confirm"
        if ($confirm -ne "YES") {
            Safe-Abort "Cancelled by user."
        }
    }

    Write-Host "    Stopping Docker Desktop..."
    $proc = Get-Process "Docker Desktop" -ErrorAction SilentlyContinue
    if ($proc) {
        $proc | Stop-Process -Force
        Start-Sleep -Seconds 8
    }

    Write-Host "    Shutting down WSL..."
    & { $ErrorActionPreference = "SilentlyContinue"; wsl --shutdown 2>&1 | Out-Null }
    Start-Sleep -Seconds 3

    if (-not (Test-Path $VhdxPath)) {
        Write-Warn ".vhdx not found at $VhdxPath -- may already be deleted. Continuing."
    } else {
        Remove-Item $VhdxPath -Force
        Write-OK "Deleted $VhdxPath"
    }
}

# ---------------------------------------------------------------------------
# Phase: Restore
# ---------------------------------------------------------------------------

function Run-Restore {
    Write-Step "PHASE 3: RESTORE"

    $latestPg    = Get-LatestBackup "postgres"
    $latestMinio = Get-LatestBackup "minio"

    if (-not $latestPg)    { throw "No postgres backup found in $BackupDir" }
    if (-not $latestMinio) { throw "No minio backup found in $BackupDir" }

    Write-Host "    Restoring from:"
    Write-Host "      Postgres : $($latestPg.FullName)"
    Write-Host "      MinIO    : $($latestMinio.FullName)"

    Write-Step "Starting Docker Desktop"
    $running = Get-Process "Docker Desktop" -ErrorAction SilentlyContinue
    if (-not $running) {
        Start-Process $DockerExe
    }
    Wait-ForDocker

    Write-Step "Creating volumes"
    docker volume create ezhealthkonnect_postgres_data | Out-Null
    docker volume create ezhealthkonnect_minio_data    | Out-Null
    # Ollama models live on the Windows host at $HOME\.ollama (bind mount, survives wipes)
    if (-not (Test-Path "$env:USERPROFILE\.ollama")) {
        New-Item -ItemType Directory -Path "$env:USERPROFILE\.ollama" | Out-Null
    }
    Write-OK "Volumes created"

    $backupDockerPath = Convert-ToDockerPath $BackupDir

    Write-Step "Restoring postgres"
    docker run --rm `
        -v "ezhealthkonnect_postgres_data:/data" `
        -v "${backupDockerPath}:/backup" `
        alpine sh -c "cd /data && tar xzf /backup/$($latestPg.Name)"
    Write-OK "Postgres restored"

    Write-Step "Restoring minio"
    docker run --rm `
        -v "ezhealthkonnect_minio_data:/data" `
        -v "${backupDockerPath}:/backup" `
        alpine sh -c "cd /data && tar xzf /backup/$($latestMinio.Name)"
    Write-OK "MinIO restored"

    Write-Step "Rebuilding app image"
    Set-Location $ComposeDir
    docker compose build app

    Write-Step "Starting the stack"
    docker compose up -d

    Write-Step "Pruning build cache"
    & { $ErrorActionPreference = "SilentlyContinue"; docker builder prune -f 2>&1 | Out-Null }
    Write-OK "Build cache cleared"

    Start-Sleep -Seconds 5
    Write-Host ""
    docker compose ps
    Write-Host ""
    Write-Host "All done. App at http://localhost:3000" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

Write-Host "ezHealthKonnect Docker Disk Reset" -ForegroundColor Magenta
Write-Host "Phase: $Phase | Backup dir: $BackupDir | Keep: $KeepBackups backups"

if ($Phase -eq "All" -or $Phase -eq "Backup") { Run-Backup }
if ($Phase -eq "All" -or $Phase -eq "Wipe")   { Run-Wipe }
if ($Phase -eq "All" -or $Phase -eq "Restore") { Run-Restore }
