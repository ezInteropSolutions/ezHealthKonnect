# rebuild-go.ps1
#
# Recompiles the Go backend WITHOUT docker compose build (no build cache growth).
# Uses a golang:1.25-alpine container with a persistent module-cache volume so
# modules are only downloaded once. Binary is copied into the running app container.
#
# Use this instead of "docker compose build app" for every Go code change.
#
# Usage:
#   .\scripts\rebuild-go.ps1
#   .\scripts\rebuild-go.ps1 -RestartNode   # also bounce the Node.js server
#
param(
    [switch]$RestartNode
)

$ErrorActionPreference = "Stop"
$AppContainer  = "ezhealthkonnect-app"
$GoImage       = "golang:1.25-alpine"
$ModCacheVol   = "ezhealthkonnect_go_module_cache"
$BuildCacheVol = "ezhealthkonnect_go_build_cache"
$ComposeDir    = "c:\Projects\ezHealthKonnect"
$TempBinary    = Join-Path $ComposeDir "go-api-rebuild-tmp"

function Write-Step([string]$msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-OK([string]$msg)   { Write-Host "    OK: $msg" -ForegroundColor Green }

Write-Host ""
Write-Host "ezHealthKonnect - Go Rebuild (zero build-cache)" -ForegroundColor Magenta

# Verify app container is running
& { $ErrorActionPreference = "SilentlyContinue"; docker inspect $AppContainer 2>&1 | Out-Null }
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Container '$AppContainer' is not running. Start the stack first." -ForegroundColor Red
    exit 1
}

# Ensure cache volumes exist (one-time creation)
foreach ($vol in @($ModCacheVol, $BuildCacheVol)) {
    & { $ErrorActionPreference = "SilentlyContinue"; docker volume inspect $vol 2>&1 | Out-Null }
    if ($LASTEXITCODE -ne 0) {
        docker volume create $vol | Out-Null
        Write-Host "    Created volume $vol"
    }
}

Write-Step "Compiling Go source"
$buildStart = Get-Date

# Convert Windows path to Docker-compatible path
$srcPath = $ComposeDir -replace '\\', '/' -replace '^([A-Za-z]):', '/$1'

docker run --rm `
    -v "${srcPath}:/src" `
    -v "${ModCacheVol}:/go/pkg/mod" `
    -v "${BuildCacheVol}:/root/.cache/go-build" `
    -e CGO_ENABLED=0 `
    -e GOOS=linux `
    -w /src `
    $GoImage `
    sh -c "go build -ldflags='-s -w' -o /src/go-api-rebuild-tmp ."

if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "BUILD FAILED -- app not restarted." -ForegroundColor Red
    if (Test-Path $TempBinary) { Remove-Item $TempBinary -Force }
    exit 1
}

$elapsed = [math]::Round(((Get-Date) - $buildStart).TotalSeconds, 1)
Write-OK "Build succeeded in ${elapsed}s"

Write-Step "Deploying binary to running container"
docker cp $TempBinary "${AppContainer}:/app/go-api"
Remove-Item $TempBinary -Force
Write-OK "Binary deployed"

Write-Step "Restarting go-api"
# The while-true loop in docker-compose CMD auto-restarts after kill
docker exec $AppContainer sh -c "pkill go-api || true"
Start-Sleep -Seconds 3

$goPid = (docker exec $AppContainer sh -c "pgrep -x go-api || echo ''").Trim()
if ($goPid) {
    Write-OK "go-api running (pid $goPid)"
} else {
    Write-Host "    WARN: go-api not yet visible -- restart loop still starting." -ForegroundColor Yellow
}

if ($RestartNode) {
    Write-Step "Restarting Node.js server"
    docker exec $AppContainer sh -c "pkill -f 'node server.js' || true"
    Start-Sleep -Seconds 2
    docker exec -d $AppContainer sh -c "node /app/server.js"
    Write-OK "Node.js restarted"
}

Write-Host ""
Write-Host "Done." -ForegroundColor Green
Write-Host "    API : http://localhost:8080"
Write-Host "    App : http://localhost:3000"
