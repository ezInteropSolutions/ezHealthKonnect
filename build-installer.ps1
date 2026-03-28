#Requires -Version 5.1
<#
.SYNOPSIS
    Builds the ezHealthKonnect GUI installer for Windows, Linux, and macOS.

.DESCRIPTION
    Compiles the Go installer application for all three platforms and packages
    each one together with the application source files into a distributable zip.

.PARAMETER Platform
    Which platform to build for: All (default), Windows, Linux, macOS.

.PARAMETER OutDir
    Output directory for built installers. Default: .\dist
#>
param(
    [ValidateSet('All','Windows','Linux','macOS')]
    [string] $Platform = 'All',
    [string] $OutDir   = '.\dist'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$InstallerSrc = Join-Path $PSScriptRoot 'installer'
$OutDir       = Join-Path $PSScriptRoot $OutDir

function Build($goos, $goarch, $outName) {
    Write-Host "  Building $outName ..." -ForegroundColor Cyan

    $env:GOOS   = $goos
    $env:GOARCH = $goarch
    $outPath    = Join-Path $OutDir $outName

    Push-Location $InstallerSrc
    try {
        go build -ldflags="-s -w" -o $outPath .
        if ($LASTEXITCODE -ne 0) { throw "go build failed for $outName" }
    } finally {
        Pop-Location
        Remove-Item env:GOOS   -ErrorAction SilentlyContinue
        Remove-Item env:GOARCH -ErrorAction SilentlyContinue
    }

    Write-Host "  [OK] $outPath" -ForegroundColor Green
    return $outPath
}

function Package($installerExe, $zipName) {
    Write-Host "  Packaging $zipName ..." -ForegroundColor Cyan
    $zipPath = Join-Path $OutDir $zipName

    # Files and dirs to include alongside the installer
    $include = @(
        'Dockerfile', 'docker-compose.prod.yml', 'docker-compose.yml',
        'package.json', 'package-lock.json', 'go.mod', 'go.sum',
        'app.js', 'server.js', 'main.go', 'COMMERCIAL_LICENSE.md'
    )
    $includeDirs = @(
        'controllers', 'services', 'middleware', 'routes', 'config',
        'models', 'processing', 'utils', 'public', 'database',
        'fhir', 'hl7'
    )

    $tmp = Join-Path $OutDir "_pkg_tmp"
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null

    # Copy installer binary
    Copy-Item $installerExe $tmp

    # Copy source files
    foreach ($f in $include) {
        $src = Join-Path $PSScriptRoot $f
        if (Test-Path $src) { Copy-Item $src $tmp }
    }
    foreach ($d in $includeDirs) {
        $src = Join-Path $PSScriptRoot $d
        if (Test-Path $src) {
            Copy-Item -Recurse $src (Join-Path $tmp $d)
        }
    }

    Compress-Archive -Path "$tmp\*" -DestinationPath $zipPath -Force
    Remove-Item -Recurse -Force $tmp

    Write-Host "  [OK] $zipPath" -ForegroundColor Green
}

# ── Main ─────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ezHealthKonnect — Installer Builder" -ForegroundColor White
Write-Host "  =====================================" -ForegroundColor DarkCyan
Write-Host ""

New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

if ($Platform -eq 'All' -or $Platform -eq 'Windows') {
    $exe = Build 'windows' 'amd64' 'ezHealthKonnect-Setup-Win64.exe'
    Package $exe 'ezHealthKonnect-Win64.zip'
}

if ($Platform -eq 'All' -or $Platform -eq 'Linux') {
    $exe = Build 'linux' 'amd64' 'ezHealthKonnect-Setup-Linux-x64'
    Package $exe 'ezHealthKonnect-Linux-x64.tar.gz'
}

if ($Platform -eq 'All' -or $Platform -eq 'macOS') {
    $exe = Build 'darwin' 'amd64' 'ezHealthKonnect-Setup-macOS-x64'
    Package $exe 'ezHealthKonnect-macOS-x64.zip'
}

Write-Host ""
Write-Host "  All done! Installers are in: $OutDir" -ForegroundColor Green
Write-Host ""
