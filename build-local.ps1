#Requires -Version 5.1
param([string]$Version = 'v1.0.4-beta')

$ErrorActionPreference = 'Stop'
$ROOT   = $PSScriptRoot
$DIST   = Join-Path $ROOT 'dist'
$ASSETS = Join-Path $ROOT 'installer\assets'

$env:PATH += ';C:\Program Files\Docker\Docker\resources\bin'

New-Item -ItemType Directory -Force -Path $DIST   | Out-Null
New-Item -ItemType Directory -Force -Path $ASSETS | Out-Null

Write-Host ""
Write-Host "ezHealthKonnect Local Build - $Version" -ForegroundColor Cyan
Write-Host "=======================================" -ForegroundColor Cyan

# -- Step 1: Build go-api.exe ----------------------------------------------------
Write-Host "`n-- Step 1: Build go-api.exe" -ForegroundColor Yellow

$s1 = "/src/dist/go-api.exe"
$script1 = "GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags `"-s -w -X main.version=$Version`" -o $s1 ."
[System.IO.File]::WriteAllText("$DIST\_s1.sh", $script1)

docker run --rm `
    -v "${ROOT}:/src" `
    -w /src `
    golang:1.25-alpine `
    sh /src/dist/_s1.sh

Remove-Item "$DIST\_s1.sh" -ErrorAction SilentlyContinue

if (-not (Test-Path "$DIST\go-api.exe")) {
    Write-Host "ERROR: go-api.exe not built" -ForegroundColor Red; exit 1
}
$sz = [math]::Round((Get-Item "$DIST\go-api.exe").Length / 1MB, 1)
Write-Host "   OK: go-api.exe ($sz MB)" -ForegroundColor Green

# -- Step 2: Package app bundle zip --------------------------------------------
Write-Host "`n-- Step 2: Package bundle zip" -ForegroundColor Yellow

$TMP        = Join-Path $env:TEMP "ezhealthkonnect-$Version"
$BUNDLE_ZIP = Join-Path $ASSETS 'bundle-windows.zip'

if (Test-Path $TMP) { Remove-Item -Recurse -Force $TMP }
New-Item -ItemType Directory -Path $TMP -Force | Out-Null

# Runtime files needed by the application
$files = @(
    'app.js', 'server.js', 'main.go',
    'package.json', 'package-lock.json',
    'go.mod', 'go.sum',
    'Dockerfile', 'docker-compose.prod.yml', 'docker-compose.yml',
    'COMMERCIAL_LICENSE.md', 'THIRD-PARTY-NOTICES.txt'
)
# Runtime directories — no dev/test/build/docs dirs
$dirs = @(
    'controllers', 'services', 'middleware', 'routes', 'config',
    'models', 'processing', 'utils', 'public',
    'fhir', 'hl7',
    'templates'
)
# NOTE: schemas/ excluded — 1.5 GB of FHIR JSON schemas, too large to embed.
# App degrades gracefully: logs warning but runs without schema validation.
# Excluded (not in $dirs above):
#   installer/       - build tooling only
#   dist/            - build output
#   tests/           - test files
#   test-results/    - test output
#   mapping_generator/ - dev tool
#   scripts/         - dev/seed scripts
#   migrations/      - separate from database/migrations (not used at runtime)
#   tools/           - dev Python script
#   node-api/        - standalone service, not imported
#   cmd/             - build tooling
#   logs/            - runtime-generated
#   connectivity/    - docs only
#   architecture/    - docs only
#   .claude/ .vscode/ .claire/ - IDE tooling

# Only ship database/migrations — not backups or init scripts
$migDir = Join-Path $TMP 'database\migrations'
New-Item -ItemType Directory -Path $migDir -Force | Out-Null
Copy-Item (Join-Path $ROOT 'database\migrations\*') $migDir -Recurse -Force

foreach ($f in $files) {
    $src = Join-Path $ROOT $f
    if (Test-Path $src) { Copy-Item $src $TMP -Force }
}
foreach ($d in $dirs) {
    $src = Join-Path $ROOT $d
    if (Test-Path $src) { Copy-Item $src (Join-Path $TMP $d) -Recurse -Force }
}
Copy-Item "$DIST\go-api.exe" (Join-Path $TMP 'go-api.exe') -Force

# Install production node_modules into the bundle so the installer is self-contained
# (no npm registry access needed on the target machine).
Write-Host "   Installing production node_modules (this takes a minute)..." -ForegroundColor DarkYellow
$npmResult = & npm install --omit=dev --silent --prefix $TMP 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "   WARN: npm install had issues: $npmResult" -ForegroundColor Yellow
} else {
    $nmSz = [math]::Round((Get-ChildItem (Join-Path $TMP 'node_modules') -Recurse | Measure-Object -Property Length -Sum).Sum / 1MB, 0)
    Write-Host "   OK: node_modules bundled ($nmSz MB)" -ForegroundColor Green
}

if (Test-Path $BUNDLE_ZIP) { Remove-Item $BUNDLE_ZIP -Force }
# Compress $TMP itself (not its contents) so the zip has a root dir matching CI format:
#   ezhealthkonnect-v1.0.2-beta/app.js
#   ezhealthkonnect-v1.0.2-beta/database/migrations/...
# extractZip(stripRoot:true) then strips that root dir correctly.
$parentDir = Split-Path $TMP -Parent
Push-Location $parentDir
Compress-Archive -Path $TMP -DestinationPath $BUNDLE_ZIP -CompressionLevel Optimal
Pop-Location
Remove-Item -Recurse -Force $TMP

$sz = [math]::Round((Get-Item $BUNDLE_ZIP).Length / 1MB, 1)
Write-Host "   OK: bundle-windows.zip ($sz MB)" -ForegroundColor Green

# -- Step 3: Build installer.exe with embedded bundle --------------------------
Write-Host "`n-- Step 3: Build installer.exe (embedded)" -ForegroundColor Yellow

$s3 = "/src/dist/ezHealthKonnect-Setup-Win64.exe"
$script3 = "GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags embedded -ldflags `"-s -w -X main.version=$Version`" -o $s3 ."
[System.IO.File]::WriteAllText("$DIST\_s3.sh", $script3)

docker run --rm `
    -v "${ROOT}:/src" `
    -w /src/installer `
    golang:1.25-alpine `
    sh /src/dist/_s3.sh

Remove-Item "$DIST\_s3.sh" -ErrorAction SilentlyContinue

if (-not (Test-Path "$DIST\ezHealthKonnect-Setup-Win64.exe")) {
    Write-Host "ERROR: installer.exe not built" -ForegroundColor Red; exit 1
}
$sz = [math]::Round((Get-Item "$DIST\ezHealthKonnect-Setup-Win64.exe").Length / 1MB, 1)
Write-Host "   OK: ezHealthKonnect-Setup-Win64.exe ($sz MB)" -ForegroundColor Green

# -- Done ----------------------------------------------------------------------
Write-Host ""
Write-Host "Build complete!" -ForegroundColor Green
Write-Host "  dist\ezHealthKonnect-Setup-Win64.exe - copy this to the target machine" -ForegroundColor Green
Write-Host ""
