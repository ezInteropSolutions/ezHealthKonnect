<#
.SYNOPSIS
    Run Linux installer smoke tests locally using Docker.

.DESCRIPTION
    Runs the Ubuntu 22.04 and Rocky Linux 9 smoke tests in Docker containers.
    No Linux machine needed — Docker Desktop on Windows is enough.

.EXAMPLE
    # Run all smoke tests
    cd installer\test
    .\Run-SmokeTests.ps1

.EXAMPLE
    # Run Ubuntu only
    .\Run-SmokeTests.ps1 -Ubuntu

.EXAMPLE
    # Run Rocky only
    .\Run-SmokeTests.ps1 -Rocky
#>
[CmdletBinding()]
param(
    [switch]$Ubuntu,
    [switch]$Rocky
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Continue'   # don't abort on test failures

$TestDir  = $PSScriptRoot
$BothMode = -not $Ubuntu -and -not $Rocky

function RunTest([string]$name, [string]$image, [string]$script) {
    Write-Host ""
    Write-Host "  Running: $name" -ForegroundColor Cyan
    Write-Host "  Image:   $image" -ForegroundColor DarkGray
    Write-Host "  ─────────────────────────────────────────" -ForegroundColor DarkGray

    $scriptContent = Get-Content $script -Raw
    $result = $scriptContent | docker run --rm -i $image bash
    $exit = $LASTEXITCODE

    if ($exit -eq 0) {
        Write-Host ""
        Write-Host "  $name: PASSED" -ForegroundColor Green
    } else {
        Write-Host ""
        Write-Host "  $name: FAILED (exit $exit)" -ForegroundColor Red
    }
    return $exit
}

# ---- Preflight ---------------------------------------------------------------
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: Docker not found. Install Docker Desktop and try again." -ForegroundColor Red
    exit 1
}
try { docker info > $null 2>&1 } catch {}
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Docker daemon is not running. Start Docker Desktop first." -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "  ezHealthKonnect Linux Installer Smoke Tests" -ForegroundColor White
Write-Host "  ============================================" -ForegroundColor DarkGray
Write-Host "  These tests run in Docker containers — no Linux machine needed." -ForegroundColor DarkGray
Write-Host "  Each test takes ~3–5 minutes (PostgreSQL and Node.js download)." -ForegroundColor DarkGray

$results = @{}

if ($BothMode -or $Ubuntu) {
    $results["Ubuntu 22.04"] = RunTest `
        "Ubuntu 22.04 (apt-get + PGDG)" `
        "ubuntu:22.04" `
        "$TestDir\smoke-linux-ubuntu.sh"
}

if ($BothMode -or $Rocky) {
    $results["Rocky Linux 9"] = RunTest `
        "Rocky Linux 9 (dnf + PGDG RPM)" `
        "rockylinux:9" `
        "$TestDir\smoke-linux-rocky.sh"
}

# ---- Summary -----------------------------------------------------------------
Write-Host ""
Write-Host "  ============================================" -ForegroundColor White
Write-Host "  Summary" -ForegroundColor White
Write-Host "  ============================================" -ForegroundColor White

$allPassed = $true
foreach ($name in $results.Keys) {
    if ($results[$name] -eq 0) {
        Write-Host "    PASS  $name" -ForegroundColor Green
    } else {
        Write-Host "    FAIL  $name" -ForegroundColor Red
        $allPassed = $false
    }
}

Write-Host "  ============================================" -ForegroundColor White
Write-Host ""

if ($allPassed) {
    Write-Host "  All tests passed. Linux installer is ready to ship." -ForegroundColor Green
    exit 0
} else {
    Write-Host "  Some tests failed. Review the output above before shipping." -ForegroundColor Red
    exit 1
}
