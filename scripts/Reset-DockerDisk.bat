@echo off
:: ============================================================
:: Reset-DockerDisk.bat  —  ONE-CLICK Docker disk reset
::
:: What it does (fully automatic, no further prompts):
::   1. Backs up postgres + minio volumes to %USERPROFILE%\ezHealthKonnect-Backups
::   2. Verifies both backups are valid before touching anything
::   3. Shuts down Docker Desktop + WSL
::   4. Deletes %LOCALAPPDATA%\Docker\wsl\disk\docker_data.vhdx
::   5. Starts Docker Desktop, recreates volumes, restores data
::   6. Rebuilds the app image and brings the full stack up
::
:: A 10-second countdown is shown before the delete — press Ctrl+C to abort.
:: ============================================================

:: --- Self-elevate to Administrator if not already running elevated --------
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting Administrator privileges...
    powershell -NoProfile -Command ^
        "Start-Process -FilePath '%~f0' -Verb RunAs -Wait"
    exit /b
)

:: --- Run from the project root so docker compose commands work -----------
cd /d "c:\Projects\ezHealthKonnect"

echo.
echo ============================================================
echo  ezHealthKonnect Docker Disk Reset  [ONE-CLICK]
echo ============================================================
echo.

powershell.exe -NoProfile -ExecutionPolicy Bypass -File ^
    "c:\Projects\ezHealthKonnect\scripts\reset-docker-disk.ps1" ^
    -Phase All ^
    -AutoConfirm

echo.
if %errorlevel% equ 0 (
    echo  DONE. Stack is up at http://localhost:3000
) else (
    echo  FAILED with exit code %errorlevel%. Scroll up for the error.
    echo  Your .vhdx was NOT deleted if the failure was in the Backup phase.
)

echo.
pause
