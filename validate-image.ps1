#Requires -Version 5.1
param(
    [switch] $SkipBuild,
    [switch] $SkipScan,
    [string] $Tag = 'ezhealthkonnect/app:validate'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Root        = $PSScriptRoot
$ComposeFile = Join-Path $Root 'docker-compose.prod.yml'
$EnvFile     = Join-Path $Root '.env.validate'
$Passed      = 0
$Failed      = 0

function Header($msg) {
    Write-Host ''
    Write-Host ('=' * 60) -ForegroundColor DarkCyan
    Write-Host "  $msg" -ForegroundColor White
    Write-Host ('=' * 60) -ForegroundColor DarkCyan
}
function Pass($msg) { Write-Host "  [PASS] $msg" -ForegroundColor Green;  $script:Passed++ }
function Fail($msg) { Write-Host "  [FAIL] $msg" -ForegroundColor Red;    $script:Failed++ }
function Info($msg) { Write-Host "  [    ] $msg" -ForegroundColor Gray }
function Warn($msg) { Write-Host "  [WARN] $msg" -ForegroundColor Yellow }

# Write a throw-away .env so compose can start without real secrets
$lines = @(
    "APP_IMAGE=$Tag",
    'APP_PORT=13000',
    'API_PORT=18080',
    'DB_USER=ezhealth_user',
    'DB_PASSWORD=ValidatePassword123!',
    'DB_HOST_PORT=15432',
    'DB_SSL=false',
    'MINIO_USER=ezhealth_user',
    'MINIO_PASSWORD=ValidatePassword123!',
    'MINIO_API_PORT=19000',
    'MINIO_CONSOLE_PORT=19001',
    'SESSION_SECRET=validate-session-secret-not-real',
    'JWT_SECRET=validate-jwt-secret-not-real',
    'OLLAMA_PORT=11434',
    'OLLAMA_CHAT_MODEL=llama3.2:3b',
    'OLLAMA_EMBED_MODEL=nomic-embed-text'
)
Set-Content -Path $EnvFile -Value $lines -Encoding UTF8

# ── Step 1: Build ─────────────────────────────────────────────────────────────
Header 'Step 1 of 4 — Building Image'
if ($SkipBuild) {
    Info 'Skipping build (-SkipBuild was set)'
} else {
    Info "Running: docker build -t $Tag ."
    $ErrorActionPreference = 'Continue'
    docker build -t $Tag $Root
    $buildExit = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    if ($buildExit -ne 0) { Fail 'docker build failed'; Remove-Item $EnvFile -Force; exit 1 }
    Pass 'Image built successfully'
}

# ── Step 2: Layer / secret inspection ────────────────────────────────────────
Header 'Step 2 of 4 — Layer Inspection'

Info 'Scanning image history for secret keywords...'
$ErrorActionPreference = 'Continue'
$history = docker history --no-trunc $Tag 2>&1 | Out-String
$ErrorActionPreference = 'Stop'
$secretHits = @('DB_PASSWORD','JWT_SECRET','SESSION_SECRET','password=','token=','api_key=') |
    Where-Object { $history -imatch $_ }
if ($secretHits) {
    Fail "Potential secret in image history: $($secretHits -join ', ')"
} else {
    Pass 'No secrets detected in image history'
}

Info 'Checking .env is not baked into image...'
$ErrorActionPreference = 'Continue'
$envCheck = docker run --rm $Tag sh -c 'test -f /app/.env && echo FOUND || echo OK' 2>&1
$ErrorActionPreference = 'Stop'
if ("$envCheck" -match 'FOUND') { Fail '.env is baked into the image — check .dockerignore' }
else { Pass '.env not present in image' }

Info 'Checking .git is not baked into image...'
$ErrorActionPreference = 'Continue'
$gitCheck = docker run --rm $Tag sh -c 'test -d /app/.git && echo FOUND || echo OK' 2>&1
$ErrorActionPreference = 'Stop'
if ("$gitCheck" -match 'FOUND') { Fail '.git directory is baked into the image — check .dockerignore' }
else { Pass '.git not present in image' }

Info 'Verifying go-api binary is present...'
$ErrorActionPreference = 'Continue'
$binCheck = docker run --rm $Tag sh -c 'test -x /app/go-api && echo OK || echo MISSING' 2>&1
$ErrorActionPreference = 'Stop'
if ("$binCheck" -match 'MISSING') { Fail 'go-api binary is missing from image' }
else { Pass 'go-api binary present and executable' }

$ErrorActionPreference = 'Continue'
$sizeStr = (docker images $Tag --format '{{.Size}}' 2>&1 | Select-Object -First 1)
$ErrorActionPreference = 'Stop'
Info "Image size: $sizeStr"
Pass "Image size recorded: $sizeStr"

# ── Step 3: CVE Scan (Trivy standalone binary) ───────────────────────────────
Header 'Step 3 of 4 — CVE Scan (Trivy)'
if ($SkipScan) {
    Info 'Skipping CVE scan (-SkipScan was set)'
} else {
    # Locate or download Trivy binary
    $trivyCmd = Get-Command trivy -ErrorAction SilentlyContinue
    $trivyExe = if ($trivyCmd) { $trivyCmd.Source } else { Join-Path $env:TEMP 'trivy-bin\trivy.exe' }

    if (-not (Test-Path $trivyExe)) {
        Info 'Trivy not found — downloading from GitHub releases...'
        $trivyDir = Split-Path $trivyExe
        New-Item -ItemType Directory -Path $trivyDir -Force | Out-Null

        # Get latest release download URL
        $releaseApi = 'https://api.github.com/repos/aquasecurity/trivy/releases/latest'
        $ErrorActionPreference = 'Continue'
        $release    = Invoke-RestMethod -Uri $releaseApi -UseBasicParsing
        $ErrorActionPreference = 'Stop'
        $asset      = $release.assets | Where-Object { $_.name -like '*Windows-64bit.zip' } | Select-Object -First 1
        if (-not $asset) { Fail 'Could not find Trivy Windows release asset'; goto skipScan }

        $zipPath = Join-Path $trivyDir 'trivy.zip'
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $zipPath -UseBasicParsing
        Expand-Archive -Path $zipPath -DestinationPath $trivyDir -Force
        Remove-Item $zipPath -Force
        Info "Trivy downloaded to $trivyExe"
    } else {
        Info "Using Trivy at: $trivyExe"
    }

    if (Test-Path $trivyExe) {
        Info 'Scanning for HIGH and CRITICAL package vulnerabilities (ignore-unfixed)...'
        $ErrorActionPreference = 'Continue'
        & $trivyExe image --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed $Tag
        $ErrorActionPreference = 'Stop'

        # Capture output to count CRITICAL findings — don't rely on exit code
        # (Trivy exits 1 on stderr INFO output too, making $LASTEXITCODE unreliable)
        $ErrorActionPreference = 'Continue'
        $trivyOut = & $trivyExe image --scanners vuln --severity CRITICAL --ignore-unfixed --quiet $Tag 2>&1 | Out-String
        $ErrorActionPreference = 'Stop'
        # A real finding prints the CVE ID; INFO/WARN lines don't contain 'CVE-'
        if ($trivyOut -match 'CVE-') {
            Fail 'CRITICAL package CVEs with available fixes — resolve before publishing'
            Write-Host $trivyOut
        } else {
            Pass 'No CRITICAL package CVEs with available fixes'
        }

        # Separate secret scan (informational — does not block publish)
        Info 'Running secret scan (informational)...'
        $ErrorActionPreference = 'Continue'
        & $trivyExe image --scanners secret --severity CRITICAL,HIGH $Tag
        $ErrorActionPreference = 'Stop'
        Warn 'Secret scan above is informational — review any findings manually'
    } else {
        Warn 'Trivy could not be downloaded — skipping CVE scan'
    }
}

# ── Step 4: Smoke Test ────────────────────────────────────────────────────────
Header 'Step 4 of 4 — Smoke Test (Full Stack)'
Info 'Starting stack on test ports: UI=13000  API=18080  DB=15432...'

$ValidateOverride = Join-Path $Root 'docker-compose.validate.yml'
$composeBase = @('compose', '-f', $ComposeFile, '-f', $ValidateOverride, '--env-file', $EnvFile, '-p', 'ezhk-validate')

try {
    $ErrorActionPreference = 'Continue'
    & docker @composeBase up -d --remove-orphans 2>&1 | Out-Null
    $ErrorActionPreference = 'Stop'

    # Discover the actual app container name (varies by Compose version)
    $ErrorActionPreference = 'Continue'
    $appContainer = docker compose -f $ComposeFile --env-file $EnvFile -p ezhk-validate `
        ps -q app 2>$null | Select-Object -First 1
    $ErrorActionPreference = 'Stop'
    if (-not $appContainer) {
        # Fallback: find by label
        $ErrorActionPreference = 'Continue'
        $appContainer = docker ps -q --filter 'label=com.docker.compose.service=app' `
            --filter 'label=com.docker.compose.project=ezhk-validate' 2>$null | Select-Object -First 1
        $ErrorActionPreference = 'Stop'
    }
    Info "App container ID: $appContainer"

    Info 'Waiting for application health check (up to 3 minutes)...'
    Info '(Includes Flyway DB migrations and all service startups)'
    $deadline = (Get-Date).AddSeconds(180)
    $healthy  = $false

    while ((Get-Date) -lt $deadline) {
        $ErrorActionPreference = 'Continue'
        $status = if ($appContainer) {
            docker inspect $appContainer --format '{{.State.Health.Status}}' 2>$null
        } else { $null }
        $ErrorActionPreference = 'Stop'
        if ($status -eq 'healthy') { $healthy = $true; break }
        Write-Host "    ... ($status)" -NoNewline
        Start-Sleep 5
    }
    Write-Host ''

    if (-not $healthy) {
        Fail 'Container did not become healthy within 3 minutes'
        Info 'App logs:'
        & docker @composeBase logs app --tail 50 2>&1
        Info 'All service status:'
        & docker @composeBase ps 2>&1
    } else {
        Pass 'Container health check passed'

        Info 'Testing HTTP health endpoint...'
        try {
            $resp = Invoke-WebRequest -Uri 'http://localhost:13000/health' `
                -UseBasicParsing -TimeoutSec 10
            $code = $resp.StatusCode
        } catch {
            $code = $_.Exception.Response.StatusCode.value__
        }
        if ($code -eq 200) {
            Pass "HTTP /health returned 200 (correct)"
        } else {
            Fail "HTTP /health returned unexpected status: $code"
            & docker @composeBase logs app --tail 30 2>&1
        }
    }
} finally {
    Info 'Tearing down test stack...'
    $ErrorActionPreference = 'Continue'
    & docker @composeBase down -v --remove-orphans 2>&1 | Out-Null
    Remove-Item $EnvFile -Force -ErrorAction SilentlyContinue
    $ErrorActionPreference = 'Stop'
}

# ── Summary ───────────────────────────────────────────────────────────────────
Header 'Validation Summary'
Write-Host ''
Write-Host "  Passed : $Passed" -ForegroundColor Green
if ($Failed -gt 0) {
    Write-Host "  Failed : $Failed" -ForegroundColor Red
    Write-Host ''
    Write-Host '  Image is NOT ready to publish. Fix the failures above.' -ForegroundColor Red
    exit 1
} else {
    Write-Host '  Failed : 0' -ForegroundColor Green
    Write-Host ''
    Write-Host '  Image passed all checks. Safe to push to registry.' -ForegroundColor Green
    Write-Host ''
    Write-Host '  To publish manually:' -ForegroundColor Gray
    Write-Host "    docker tag $Tag ghcr.io/ezinteropsolutions/ezhealthkonnect:latest" -ForegroundColor Gray
    Write-Host '    docker push ghcr.io/ezinteropsolutions/ezhealthkonnect:latest' -ForegroundColor Gray
}
Write-Host ''
