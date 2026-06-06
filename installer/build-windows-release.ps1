<#
.SYNOPSIS
    Build a single self-contained Windows installer exe for ezHealthKonnect.

.DESCRIPTION
    Produces ONE file: ezHealthKonnect-Setup-Win64.exe (~35 MB).
    The app bundle is embedded directly inside the exe -- no sidecar zip,
    no GitHub release, no internet required for bundle extraction.

    Steps:
      1. Cross-compile go-api.exe for Windows via Docker
      2. Package the app source + go-api.exe into a zip (no node_modules)
      3. Embed the zip into the installer via -tags embedded
      4. Output: ..\dist\ezHealthKonnect-Setup-Win64.exe

    Node.js dependencies (node_modules) are NOT bundled -- the installer
    runs "npm install --omit=dev" during installation.

.EXAMPLE
    cd installer
    .\build-windows-release.ps1
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot   = (Resolve-Path "$PSScriptRoot\..").Path
$DistDir    = Join-Path $RepoRoot "dist"
$AssetsDir  = Join-Path $PSScriptRoot "assets"
$BundleZip  = Join-Path $AssetsDir "bundle-windows.zip"
$TmpDir     = Join-Path $env:TEMP "ezhk-bundle-$PID"
$BundleDir  = Join-Path $TmpDir "ezhealthkonnect"

function Step([int]$n, [string]$msg) { Write-Host "`n-- Step ${n}: $msg" -ForegroundColor Cyan }
function Ok([string]$msg)            { Write-Host "   OK  $msg" -ForegroundColor Green }
function Warn([string]$msg)          { Write-Host " WARN  $msg" -ForegroundColor Yellow }
function Fail([string]$msg)          {
    Write-Host ""
    Write-Host " FAIL  $msg" -ForegroundColor Red
    Write-Host "       Build aborted." -ForegroundColor Red
    exit 1
}

function Confirm-File([string]$path, [string]$label) {
    if (-not (Test-Path $path)) { Fail "$label not found at: $path" }
    $size = '{0:N1}' -f ((Get-Item $path).Length / 1MB)
    Ok "$label ready ($size MB)"
}

# ---- Preflight ---------------------------------------------------------------
Write-Host ""
Write-Host "  ezHealthKonnect -- Windows Installer Build" -ForegroundColor White
Write-Host "  ===========================================" -ForegroundColor DarkGray
Write-Host "  Repo  : $RepoRoot" -ForegroundColor DarkGray
Write-Host "  Output: $DistDir\ezHealthKonnect-Setup-Win64.exe" -ForegroundColor DarkGray
Write-Host ""

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Fail "Docker not found. Install Docker Desktop and try again."
}
try { docker info > $null } catch {}
if ($LASTEXITCODE -ne 0) {
    Fail "Docker daemon is not running. Start Docker Desktop and try again."
}
Ok "Docker ready"

New-Item -ItemType Directory -Force $DistDir   | Out-Null
New-Item -ItemType Directory -Force $AssetsDir | Out-Null

# ---- Step 1: Cross-compile go-api.exe for Windows ---------------------------
Step 1 "Cross-compiling go-api.exe for Windows (via Docker)"

New-Item -ItemType Directory -Force $BundleDir | Out-Null

docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.25-alpine `
  go build -ldflags "-s -w" -o /work/installer/assets/_go-api-tmp.exe .

if ($LASTEXITCODE -ne 0) {
    Fail "go-api.exe cross-compile failed. Check Go source for errors above."
}

$tmpGoApi = Join-Path $AssetsDir "_go-api-tmp.exe"
$GoApiInBundle = Join-Path $BundleDir "go-api.exe"
Move-Item $tmpGoApi $GoApiInBundle -Force
Confirm-File $GoApiInBundle "go-api.exe"

# ---- Step 2: Assemble app bundle --------------------------------------------
Step 2 "Assembling app bundle (source + go-api.exe, no node_modules)"

$ExcludeNames = @(
    '.git', '.github',
    'installer', 'dist', 'dist-go',
    'architecture', 'docs', 'connectivity',
    'tests', 'logs',
    'schemas',
    'downloads',
    'node_modules'
)
$ExcludeFiles = @(
    '.env', '.env.production',
    'go-api', 'go-api-linux', 'go-api-siu', 'go-api-new', 'ezhealthkonnect'
)

Get-ChildItem $RepoRoot | Where-Object {
    $name = $_.Name
    $excluded = $false
    foreach ($ex in $ExcludeNames) { if ($name -ieq $ex) { $excluded = $true; break } }
    foreach ($ex in $ExcludeFiles) { if ($name -ieq $ex) { $excluded = $true; break } }
    -not $excluded
} | ForEach-Object {
    Copy-Item $_.FullName $BundleDir -Recurse -Force
}

$fileCount     = (Get-ChildItem $BundleDir -Recurse -File).Count
$bundleSizeMB  = '{0:N1}' -f ((Get-ChildItem $BundleDir -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB)
Ok "Bundle staged: $fileCount files, $bundleSizeMB MB uncompressed"

# ---- Step 3: Zip bundle into installer/assets/bundle-windows.zip ------------
Step 3 "Compressing bundle -> installer/assets/bundle-windows.zip"

if (Test-Path $BundleZip) { Remove-Item $BundleZip -Force }

Push-Location $TmpDir
try {
    Compress-Archive -Path "ezhealthkonnect" -DestinationPath $BundleZip -CompressionLevel Optimal
} finally {
    Pop-Location
}

Confirm-File $BundleZip "bundle-windows.zip"

Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue

# ---- Step 4: Cross-compile installer with embedded bundle -------------------
Step 4 "Cross-compiling installer exe with embedded bundle (via Docker)"

$InstallerExe = Join-Path $DistDir "ezHealthKonnect-Setup-Win64.exe"

docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work/installer `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.25-alpine `
  go build -tags embedded -ldflags "-s -w -H windowsgui" -o /work/dist/ezHealthKonnect-Setup-Win64.exe .

if ($LASTEXITCODE -ne 0) {
    Remove-Item $BundleZip -Force -ErrorAction SilentlyContinue
    Fail "Installer cross-compile failed. Check the Go errors above."
}

Confirm-File $InstallerExe "ezHealthKonnect-Setup-Win64.exe"

# ---- Cleanup: remove bundle zip from source tree ----------------------------
Remove-Item $BundleZip -Force -ErrorAction SilentlyContinue
Ok "Cleaned up installer/assets/bundle-windows.zip"

# ---- Done -------------------------------------------------------------------
$finalSizeMB = '{0:N1}' -f ((Get-Item $InstallerExe).Length / 1MB)

Write-Host ""
Write-Host "  =================================================" -ForegroundColor Green
Write-Host "  Build complete!" -ForegroundColor Green
Write-Host "  =================================================" -ForegroundColor Green
Write-Host "  dist\ezHealthKonnect-Setup-Win64.exe" -ForegroundColor White
Write-Host "  Size: $finalSizeMB MB (self-contained)" -ForegroundColor White
Write-Host ""
Write-Host "  Distribute just this ONE file." -ForegroundColor White
Write-Host "  No sidecar zip, no GitHub release needed." -ForegroundColor White
Write-Host "  =================================================" -ForegroundColor Green
Write-Host ""
