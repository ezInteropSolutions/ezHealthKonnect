<#
.SYNOPSIS
    Build a single self-contained Windows installer exe for ezHealthKonnect.

.DESCRIPTION
    Produces ONE file: ezHealthKonnect-Setup-Win64.exe.
    The app bundle (source + go-api.exe + HL7 schemas) is embedded directly
    inside the exe -- no sidecar zip, no GitHub release needed.

    Steps:
      1. Cross-compile go-api.exe for Windows via Docker  [skipped with -SkipGoCompile]
      2. Package the app source + go-api.exe + core schemas into a zip
      3. Embed the zip into the installer via -tags embedded
      4. Output: ..\dist\ezHealthKonnect-Setup-Win64.exe

    Cached go-api.exe is stored at dist\go-api-windows-amd64.exe and reused
    when -SkipGoCompile is set.  Use this flag when only JS/Node/SQL files
    changed -- saves ~3-4 minutes per build.

.PARAMETER SkipGoCompile
    Skip Step 1 and reuse the cached go-api.exe from dist\go-api-windows-amd64.exe.
    Use when only JavaScript, Node.js, HTML, CSS, or SQL migration files changed.

.EXAMPLE
    # Full build (Go source or dependencies changed)
    cd installer
    .\build-windows-release.ps1

.EXAMPLE
    # Fast build (only JS/Node/SQL files changed)
    cd installer
    .\build-windows-release.ps1 -SkipGoCompile
#>

[CmdletBinding()]
param(
    [switch]$SkipGoCompile
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot      = (Resolve-Path "$PSScriptRoot\..").Path
$DistDir       = Join-Path $RepoRoot "dist"
$AssetsDir     = Join-Path $PSScriptRoot "assets"
$BundleZip     = Join-Path $AssetsDir "bundle-windows.zip"
$CachedGoApi   = Join-Path $DistDir "go-api-windows-amd64.exe"   # persisted between builds
$TmpDir        = Join-Path $env:TEMP "ezhk-bundle-$PID"
$BundleDir     = Join-Path $TmpDir "ezhealthkonnect"

function Step([int]$n, [string]$msg) { Write-Host "`n-- Step ${n}: $msg" -ForegroundColor Cyan }
function Ok([string]$msg)            { Write-Host "   OK  $msg" -ForegroundColor Green }
function Warn([string]$msg)          { Write-Host " WARN  $msg" -ForegroundColor Yellow }
function Fail([string]$msg)          {
    Write-Host ""
    Write-Host " FAIL  $msg" -ForegroundColor Red
    Write-Host "       Build aborted." -ForegroundColor Red
    # Clean up temp dir if it exists
    Remove-Item $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
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
if ($SkipGoCompile) {
    Write-Host "  Mode  : Fast (skipping Go compile -- JS/Node/SQL changes only)" -ForegroundColor Yellow
} else {
    Write-Host "  Mode  : Full (compiling go-api.exe + installer)" -ForegroundColor DarkGray
}
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
New-Item -ItemType Directory -Force $BundleDir | Out-Null

# ---- Step 1: Cross-compile go-api.exe (or reuse cache) ----------------------
if ($SkipGoCompile) {
    Step 1 "Reusing cached go-api.exe (skipping Go compile)"
    if (-not (Test-Path $CachedGoApi)) {
        Fail "No cached go-api.exe found at dist\go-api-windows-amd64.exe.`n       Run without -SkipGoCompile first to build and cache it."
    }
    $GoApiInBundle = Join-Path $BundleDir "go-api.exe"
    Copy-Item $CachedGoApi $GoApiInBundle -Force
    Confirm-File $GoApiInBundle "go-api.exe (cached)"
} else {
    Step 1 "Cross-compiling go-api.exe for Windows (via Docker)"

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

    # Save to bundle dir
    Move-Item $tmpGoApi $GoApiInBundle -Force
    Confirm-File $GoApiInBundle "go-api.exe"

    # Cache for future -SkipGoCompile runs
    Copy-Item $GoApiInBundle $CachedGoApi -Force
    Ok "go-api.exe cached at dist\go-api-windows-amd64.exe"
}

# ---- Step 2: Assemble app bundle --------------------------------------------
Step 2 "Assembling app bundle (source + go-api.exe + core HL7 schemas)"

$ExcludeNames = @(
    '.git', '.github',
    'installer', 'dist', 'dist-go',
    'architecture', 'docs', 'connectivity',
    'tests', 'logs',
    'schemas',        # handled separately below -- only core versions included
    'downloads',
    'node_modules'    # installed by npm on the target machine
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

# Core HL7 schema versions -- already .gz compressed, zip adds no further reduction.
# v2.3 (68 MB): legacy HIS/EHR systems  v2.5/v2.5.1 (255 MB): modern standard
# Other versions (v2.4, v2.6, v2.7, v2.8) available via Settings > Schema Registry
$CoreSchemaVersions = @('v2.3', 'v2.5', 'v2.5.1')
$SchemaSrc = Join-Path $RepoRoot "schemas"
$SchemaDst = Join-Path $BundleDir "schemas"
New-Item -ItemType Directory -Force $SchemaDst | Out-Null

foreach ($ver in $CoreSchemaVersions) {
    $src = Join-Path $SchemaSrc "hl7\$ver"
    if (Test-Path $src) {
        New-Item -ItemType Directory -Force (Join-Path $SchemaDst "hl7") | Out-Null
        Copy-Item $src (Join-Path $SchemaDst "hl7\$ver") -Recurse -Force
        $szMB = '{0:N1}' -f ((Get-ChildItem $src -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB)
        Ok "  Schema hl7/$ver ($szMB MB)"
    } else {
        Warn "  Schema hl7/$ver not found in repo -- skipping"
    }
}
$fhirSrc = Join-Path $SchemaSrc "fhir"
if (Test-Path $fhirSrc) {
    Copy-Item $fhirSrc (Join-Path $SchemaDst "fhir") -Recurse -Force
    $szMB = '{0:N1}' -f ((Get-ChildItem $fhirSrc -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB)
    Ok "  Schema fhir ($szMB MB)"
}

$fileCount    = (Get-ChildItem $BundleDir -Recurse -File).Count
$bundleSizeMB = '{0:N1}' -f ((Get-ChildItem $BundleDir -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB)
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

# Cleanup: remove bundle zip from source tree (gitignored, rebuilt each time)
Remove-Item $BundleZip -Force -ErrorAction SilentlyContinue
Ok "Cleaned up installer/assets/bundle-windows.zip"

# ---- Done -------------------------------------------------------------------
$finalSizeMB = '{0:N1}' -f ((Get-Item $InstallerExe).Length / 1MB)

Write-Host ""
Write-Host "  =================================================" -ForegroundColor Green
Write-Host "  Build complete!" -ForegroundColor Green
Write-Host "  =================================================" -ForegroundColor Green
Write-Host "  dist\ezHealthKonnect-Setup-Win64.exe  ($finalSizeMB MB)" -ForegroundColor White
Write-Host ""
Write-Host "  Next build with only JS/Node/SQL changes:" -ForegroundColor DarkGray
Write-Host "    .\build-windows-release.ps1 -SkipGoCompile" -ForegroundColor DarkGray
Write-Host "  =================================================" -ForegroundColor Green
Write-Host ""
