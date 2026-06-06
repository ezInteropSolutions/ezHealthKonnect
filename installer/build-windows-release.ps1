<#
.SYNOPSIS
    Build a self-contained Windows standalone installer for development/testing.

.DESCRIPTION
    1. Cross-compiles go-api.exe for Windows using Docker
    2. Packages the app into ezhealthkonnect-windows-amd64.zip
    3. Cross-compiles the installer exe for Windows using Docker
    4. Copies both files to ../dist/ so you can run the installer with the
       bundle sitting beside it — no GitHub release required.

.EXAMPLE
    cd installer
    .\build-windows-release.ps1

    Output:
      ../dist/ezHealthKonnect-Setup-Win64.exe   — installer
      ../dist/ezhealthkonnect-windows-amd64.zip — app bundle (place next to installer)
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot  = (Resolve-Path "$PSScriptRoot\..").Path
$DistDir   = Join-Path $RepoRoot "dist"
$TmpBundle = Join-Path $env:TEMP "ezhk-bundle-build"
$BundleName = "ezhealthkonnect-windows-amd64"
$BundleDir  = Join-Path $TmpBundle $BundleName

function Step($n, $msg) { Write-Host "`n── Step $n`: $msg" -ForegroundColor Cyan }
function Ok($msg)        { Write-Host "  OK  $msg" -ForegroundColor Green }
function Fail($msg)      { Write-Host "  ERR $msg" -ForegroundColor Red; exit 1 }

# ── Check Docker ──────────────────────────────────────────────────────────────
Step 0 "Checking Docker"
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Fail "Docker not found. Install Docker Desktop or use the go-build-check skill."
}
docker info *>$null
if ($LASTEXITCODE -ne 0) { Fail "Docker daemon not running." }
Ok "Docker ready"

# ── Prepare dist dir ──────────────────────────────────────────────────────────
New-Item -ItemType Directory -Force $DistDir | Out-Null
New-Item -ItemType Directory -Force $BundleDir | Out-Null

# ── Step 1: Cross-compile go-api.exe for Windows ──────────────────────────────
Step 1 "Cross-compiling go-api.exe for Windows (via Docker)"

$GoApiExe = Join-Path $BundleDir "go-api.exe"

docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.25-alpine `
  go build -ldflags "-s -w" -o /work/dist/go-api.exe .

if ($LASTEXITCODE -ne 0) { Fail "go-api.exe cross-compile failed." }
Copy-Item (Join-Path $DistDir "go-api.exe") $GoApiExe
Ok "go-api.exe built"

# ── Step 2: Install production Node.js dependencies ───────────────────────────
Step 2 "Installing production Node.js dependencies"

Push-Location $RepoRoot
try {
    npm install --omit=dev --prefer-offline 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) { Fail "npm install failed." }
    Ok "npm install --omit=dev done"
} finally {
    Pop-Location
}

# ── Step 3: Package app bundle ────────────────────────────────────────────────
Step 3 "Packaging app bundle"

# Exclusion list mirrors the release workflow rsync excludes
$Exclude = @(
    '.git', '.github', 'logs', 'installer', 'architecture', 'docs',
    'connectivity', 'tests', 'dist', 'dist-go',
    '.env', '.env.*', '*.test.js',
    'go-api', 'go-api.exe', 'go-api-linux',
    'node_modules\.cache'
)

# Copy source files into bundle dir
Get-ChildItem $RepoRoot | Where-Object {
    $item = $_.Name
    -not ($Exclude | Where-Object { $item -like $_ })
} | ForEach-Object {
    Copy-Item $_.FullName $BundleDir -Recurse -Force
}

# Copy node_modules (already installed without .cache)
if (Test-Path (Join-Path $RepoRoot "node_modules")) {
    Copy-Item (Join-Path $RepoRoot "node_modules") $BundleDir -Recurse -Force
}

$ZipDest = Join-Path $DistDir "$BundleName.zip"
if (Test-Path $ZipDest) { Remove-Item $ZipDest -Force }

Push-Location $TmpBundle
try {
    Compress-Archive -Path $BundleName -DestinationPath $ZipDest -CompressionLevel Optimal
    Ok "Bundle: $ZipDest ($('{0:N1}' -f ((Get-Item $ZipDest).Length / 1MB)) MB)"
} finally {
    Pop-Location
}

# ── Step 4: Cross-compile installer exe for Windows ───────────────────────────
Step 4 "Cross-compiling installer exe for Windows (via Docker)"

$InstallerExe = Join-Path $DistDir "ezHealthKonnect-Setup-Win64.exe"

docker run --rm `
  -v "${RepoRoot}:/work" `
  -w /work/installer `
  -e GOOS=windows `
  -e GOARCH=amd64 `
  -e CGO_ENABLED=0 `
  golang:1.25-alpine `
  go build -ldflags "-s -w -H windowsgui" -o /work/dist/ezHealthKonnect-Setup-Win64.exe .

if ($LASTEXITCODE -ne 0) { Fail "Installer cross-compile failed." }
Ok "Installer: $InstallerExe ($('{0:N1}' -f ((Get-Item $InstallerExe).Length / 1MB)) MB)"

# ── Cleanup temp dir ──────────────────────────────────────────────────────────
Remove-Item $TmpBundle -Recurse -Force -ErrorAction SilentlyContinue

# ── Done ──────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "════════════════════════════════════════════════" -ForegroundColor Green
Write-Host "  Build complete!" -ForegroundColor Green
Write-Host ""
Write-Host "  Files in $DistDir`:``" -ForegroundColor White
Write-Host "    ezHealthKonnect-Setup-Win64.exe   (installer)" -ForegroundColor White
Write-Host "    ezhealthkonnect-windows-amd64.zip (app bundle)" -ForegroundColor White
Write-Host ""
Write-Host "  To install:" -ForegroundColor White
Write-Host "    1. Copy both files to the same folder" -ForegroundColor White
Write-Host "    2. Run ezHealthKonnect-Setup-Win64.exe" -ForegroundColor White
Write-Host "    3. The installer detects the zip — no internet needed" -ForegroundColor White
Write-Host "════════════════════════════════════════════════" -ForegroundColor Green
