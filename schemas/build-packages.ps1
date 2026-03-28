# build-packages.ps1
# Run this from inside the cloned ezhealthkonnect-schemas repo.
# Point $SchemaSource at your ezHealthKonnect/schemas folder.
# Outputs zip files to ./dist/ ready for upload as GitHub Release assets.
#
# Usage:
#   cd C:\path\to\ezhealthkonnect-schemas
#   .\build-packages.ps1 -SchemaSource "C:\Projects\ezHealthKonnect\schemas"

param(
    [Parameter(Mandatory=$true)]
    [string]$SchemaSource
)

$dist = Join-Path $PSScriptRoot "dist"
if (-not (Test-Path $dist)) { New-Item -ItemType Directory -Path $dist | Out-Null }

function Build-Pack {
    param([string]$PackId, [string[]]$SourceDirs, [string]$OutputZip)

    $zip = Join-Path $dist "$PackId.zip"
    Write-Host "Building $PackId ..." -ForegroundColor Cyan

    # Remove existing zip if present
    if (Test-Path $zip) { Remove-Item $zip }

    $missing = $false
    foreach ($dir in $SourceDirs) {
        $full = Join-Path $SchemaSource $dir
        if (-not (Test-Path $full)) {
            Write-Warning "  Source not found: $full — skipping $PackId"
            $missing = $true
            break
        }
    }
    if ($missing) { return }

    # Compress-Archive can't handle multiple sources in one call,
    # so we stage to a temp dir first.
    $tmp = Join-Path $env:TEMP "ezhk-schema-stage-$PackId"
    if (Test-Path $tmp) { Remove-Item $tmp -Recurse -Force }
    New-Item -ItemType Directory -Path $tmp | Out-Null

    foreach ($dir in $SourceDirs) {
        $full   = Join-Path $SchemaSource $dir
        $target = Join-Path $tmp $dir
        New-Item -ItemType Directory -Path (Split-Path $target) -Force | Out-Null
        Copy-Item $full $target -Recurse
    }

    Compress-Archive -Path "$tmp\*" -DestinationPath $zip -CompressionLevel NoCompression
    Remove-Item $tmp -Recurse -Force

    $sizeMB = [math]::Round((Get-Item $zip).Length / 1MB, 1)
    Write-Host "  Done: $PackId.zip ($sizeMB MB)" -ForegroundColor Green
}

# ── FHIR Packs ───────────────────────────────────────────────────────────────
Build-Pack "fhir-r4-base"    @("fhir\R4\resources")            "fhir-r4-base.zip"
Build-Pack "fhir-r4-us-core" @("fhir\R4\profiles\us-core")     "fhir-r4-us-core.zip"

# ── HL7 Packs ────────────────────────────────────────────────────────────────
# Bundle minor versions together (v2.3 + v2.3.1, etc.) so users install one pack
Build-Pack "hl7-v2-legacy"   @("hl7\v2.1", "hl7\v2.2")                    "hl7-v2-legacy.zip"
Build-Pack "hl7-v2-23"       @("hl7\v2.3", "hl7\v2.3.1")                  "hl7-v2-23.zip"
Build-Pack "hl7-v2-24"       @("hl7\v2.4")                                 "hl7-v2-24.zip"
Build-Pack "hl7-v2-25"       @("hl7\v2.5", "hl7\v2.5.1")                  "hl7-v2-25.zip"
Build-Pack "hl7-v2-26"       @("hl7\v2.6")                                 "hl7-v2-26.zip"
Build-Pack "hl7-v2-27"       @("hl7\v2.7", "hl7\v2.7.1")                  "hl7-v2-27.zip"
Build-Pack "hl7-v2-28"       @("hl7\v2.8")                                 "hl7-v2-28.zip"

Write-Host ""
Write-Host "All packages built in: $dist" -ForegroundColor Yellow
Write-Host "Next: upload these files as assets to a GitHub Release (v1.0.0)" -ForegroundColor Yellow
Write-Host "Then update download_url values in catalog.json with your GitHub username." -ForegroundColor Yellow
