<#
.SYNOPSIS
    Build a single self-contained Windows installer exe for ezHealthKonnect.

.DESCRIPTION
    Produces ONE file: ezHealthKonnect-Setup-Win64.exe (~35 MB).
    The app bundle is embedded directly inside the exe — no sidecar zip,
    no GitHub release, no internet required at install time (except npm).

    Steps:
      1. Cross-compile go-api.exe for Windows via Docker
      2. Package the app source + go-api.exe into a zip (no node_modules)
      3. Embed the zip into the installer via -tags embedded
      4. Output: ../dist/ezHealthKonnect-Setup-Win64.exe

    Node.js dependencies (node_modules) are NOT bundled — the installer
    runs "npm install --omit=dev" during installation. This requires
    internet access on the target machine for that one step.

.EXAMPLE
    cd installer
    .\build-windows-release.ps1

    Then distribute only:
      dist\ezHealthKonnect-Setup-Win64.exe
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot   = (Resolve-Path "$PSScriptRoot\..").Path
$DistDir    = Join-Path $RepoRoot "dist"
$AssetsDir  = Join-Path $PSScriptRoot "assets"
$BundleZip  = Join-Path $AssetsDir "bundle-windows.zip"  # embed source
$TmpDir     = Join-Path $env:TEMP "ezhk-bundle-$$"
$BundleDir  = Join-Path $TmpDir "ezhealthkonnect"        # strip-root dir name

function Step($n, $msg) { Write-Host "`n── Step $n: $msg" -ForegroundColor Cyan }
function Ok($msg)        { Write-Host "   OK  $msg" -ForegroundColor Green }
function Warn($msg)      { Write-Host "  WARN $msg" -ForegroundColor Yellow }
function Fail($msg)      { Write-Host ""
                           Write-Host "  FAIL $msg" -ForegroundColor Red
                           Write-Host "       Build aborted." -ForegroundColor Red
                           exit 1 }

function Require-Docker {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        Fail "Docker not found. Install Docker Desktop and try again."
    }
    $info = docker info 2>&1
    if ($LASTEXITCODE -ne 0) {
        Fail "Docker daemon is not running. Start Docker Desktop and try again."
    }
}

function Confirm-File($path, $label) {
    if (-not (Test-Path $path)) { Fail "$label not found at: $path" }
    $size = '{0:N1}' -f ((Get-Item $path).Length / 1MB)
    Ok "$label ready ($size MB)"
}

# ── Preflight ─────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ezHealthKonnect — Windows Installer Build" -ForegroundColor White
Write-Host "  ==========================================" -ForegroundColor DarkGray
Write-Host "  Repo  : $RepoRoot" -ForegroundColor DarkGray
Write-Host "  Output: $DistDir\ezHealthKonnect-Setup-Win64.exe" -ForegroundColor DarkGray
Write-Host ""

Require-Docker

New-Item -ItemType Directory -Force $DistDir   | Out-Null
New-Item -ItemType Directory -Force $AssetsDir | Out-Null

# ── Step 1: Cross-compile go-api.exe for Windows ─────────────────────────────
Step 1 "Cross-compiling go-api.exe for Windows (via Docker)"

New-Item -ItemType Directory -Force $BundleDir | Out-Null
$GoApiInBundle = Join-Path $BundleDir "go-api.exe"

# Build directly into the bundle staging dir
docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.24-alpine `
  go build -ldflags "-s -w" -o /work/installer/assets/_go-api-tmp.exe .

if ($LASTEXITCODE -ne 0) {
    Fail "go-api.exe cross-compile failed. Check Go source for errors."
}

Move-Item (Join-Path $AssetsDir "_go-api-tmp.exe") $GoApiInBundle -Force
Confirm-File $GoApiInBundle "go-api.exe"

# ── Step 2: Assemble app bundle ───────────────────────────────────────────────
Step 2 "Assembling app bundle (source + go-api.exe, no node_modules)"

# Directories and files to exclude from the bundle
$ExcludeNames = @(
    '.git', '.github',
    'installer', 'dist', 'dist-go',
    'architecture', 'docs', 'connectivity',
    'tests', 'logs',
    'schemas',        # 1.5 GB — schema packages downloaded separately
    'downloads',      # dev download cache
    'node_modules'    # installed by npm on the target machine
)
# Exact filenames to exclude at root level
$ExcludeFiles = @(
    '.env', '.env.production',
    'go-api', 'go-api-linux', 'go-api-siu', 'go-api-new', 'ezhealthkonnect'
)

# Copy root items that aren't excluded
Get-ChildItem $RepoRoot | Where-Object {
    $name = $_.Name
    $excluded = $false
    foreach ($ex in $ExcludeNames) { if ($name -ieq $ex) { $excluded = $true; break } }
    foreach ($ex in $ExcludeFiles) { if ($name -ieq $ex) { $excluded = $true; break } }
    -not $excluded
} | ForEach-Object {
    Copy-Item $_.FullName $BundleDir -Recurse -Force
}

# go-api.exe was already placed in $BundleDir above
$fileCount = (Get-ChildItem $BundleDir -Recurse -File).Count
$bundleSizeMB = '{0:N1}' -f ((Get-ChildItem $BundleDir -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB)
Ok "Bundle staged: $fileCount files, $bundleSizeMB MB uncompressed"

# ── Step 3: Zip bundle into installer/assets/bundle-windows.zip ──────────────
Step 3 "Compressing bundle → installer/assets/bundle-windows.zip"

if (Test-Path $BundleZip) { Remove-Item $BundleZip -Force }

Push-Location $TmpDir
try {
    Compress-Archive -Path "ezhealthkonnect" -DestinationPath $BundleZip -CompressionLevel Optimal
} finally {
    Pop-Location
}

Confirm-File $BundleZip "bundle-windows.zip"

# Clean up staging dir — no longer needed
Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue

# ── Step 4: Cross-compile installer with embedded bundle ──────────────────────
Step 4 "Cross-compiling installer exe with embedded bundle (via Docker)"

$InstallerExe = Join-Path $DistDir "ezHealthKonnect-Setup-Win64.exe"

docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work/installer `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.24-alpine `
  go build -tags embedded -ldflags "-s -w -H windowsgui" -o /work/dist/ezHealthKonnect-Setup-Win64.exe .

if ($LASTEXITCODE -ne 0) {
    # Clean up the bundle zip so it doesn't get committed
    Remove-Item $BundleZip -Force -ErrorAction SilentlyContinue
    Fail "Installer cross-compile failed. Check the output above for Go errors."
}

Confirm-File $InstallerExe "ezHealthKonnect-Setup-Win64.exe"

# ── Cleanup embedded asset (don't leave 26 MB zip in source tree) ─────────────
Remove-Item $BundleZip -Force -ErrorAction SilentlyContinue
Ok "Cleaned up installer/assets/bundle-windows.zip"

# ── Done ──────────────────────────────────────────────────────────────────────
$finalSizeMB = '{0:N1}' -f ((Get-Item $InstallerExe).Length / 1MB)

Write-Host ""
Write-Host "  ╔══════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "  ║  Build complete!                                 ║" -ForegroundColor Green
Write-Host "  ╠══════════════════════════════════════════════════╣" -ForegroundColor Green
Write-Host "  ║  dist\ezHealthKonnect-Setup-Win64.exe            ║" -ForegroundColor White
Write-Host ("  ║  Size: {0,-43}║" -f "$finalSizeMB MB (self-contained)") -ForegroundColor White
Write-Host "  ╠══════════════════════════════════════════════════╣" -ForegroundColor Green
Write-Host "  ║  To distribute: copy just this ONE file.         ║" -ForegroundColor White
Write-Host "  ║  Run it on any Windows 10/11 machine.            ║" -ForegroundColor White
Write-Host "  ║  No sidecar zip, no GitHub release needed.       ║" -ForegroundColor White
Write-Host "  ╚══════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
