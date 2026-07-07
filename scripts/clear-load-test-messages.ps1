# clear-load-test-messages.ps1
#
# Truncates bloated interface message tables after load/stress testing.
# Targets any messages_intf_* table above -ThresholdMB (default 1 MB).
# Safe: never touches schema, config, pipelines, or user data.
#
# Usage:
#   .\scripts\clear-load-test-messages.ps1
#   .\scripts\clear-load-test-messages.ps1 -ThresholdMB 10   # only tables > 10 MB
#   .\scripts\clear-load-test-messages.ps1 -DryRun            # preview; no changes
#
param(
    [int]$ThresholdMB = 1,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$container = "ezhealthkonnect-postgres"

function Write-Step([string]$msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-OK([string]$msg)   { Write-Host "    OK: $msg" -ForegroundColor Green }
function Write-Info([string]$msg) { Write-Host "    $msg" -ForegroundColor White }

function Invoke-PsqlPipe([string]$sql) {
    # Pipe SQL via stdin to avoid Windows quoting issues with -c flag
    return ($sql | docker exec -i $container psql -U ezhealth_user -d ezhealthkonnect -t -A -F "|" 2>&1)
}

Write-Host ""
Write-Host "ezHealthKonnect - Load Test Message Cleanup" -ForegroundColor Magenta
Write-Host "    DryRun=$DryRun  ThresholdMB=${ThresholdMB}"

# Verify the postgres container is up
& { $ErrorActionPreference = "SilentlyContinue"; docker inspect $container 2>&1 | Out-Null }
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Container '$container' is not running. Start the stack first." -ForegroundColor Red
    exit 1
}

Write-Step "Finding bloated message tables (above ${ThresholdMB} MB)"

$thresholdBytes = $ThresholdMB * 1024 * 1024
$sql = "SELECT c.relname, pg_total_relation_size(c.oid), pg_size_pretty(pg_total_relation_size(c.oid)), COALESCE(i.name, '(unknown)') FROM pg_class c LEFT JOIN interfaces i ON c.relname = 'messages_intf_' || replace(i.id::text, '-', '_') WHERE c.relname LIKE 'messages_intf_%' AND c.relkind = 'r' AND pg_total_relation_size(c.oid) > $thresholdBytes ORDER BY pg_total_relation_size(c.oid) DESC;"

$rows = Invoke-PsqlPipe $sql
$targets = [System.Collections.Generic.List[PSCustomObject]]::new()

foreach ($row in $rows) {
    $r = "$row".Trim()
    if ([string]::IsNullOrEmpty($r)) { continue }
    $parts = $r -split '\|'
    if ($parts.Count -lt 4) { continue }
    $sizeLong = 0L
    if ([long]::TryParse($parts[1].Trim(), [ref]$sizeLong)) {
        $targets.Add([PSCustomObject]@{
            Table         = $parts[0].Trim()
            SizeBytes     = $sizeLong
            SizePretty    = $parts[2].Trim()
            InterfaceName = $parts[3].Trim()
        })
    }
}

if ($targets.Count -eq 0) {
    Write-OK "No message tables above ${ThresholdMB} MB -- nothing to clear."
    exit 0
}

$totalMB = [math]::Round(($targets | Measure-Object -Property SizeBytes -Sum).Sum / 1MB, 1)

Write-Host ""
Write-Host ("    {0,-52} {1,8}  {2}" -f "Table", "Size", "Interface") -ForegroundColor Gray
Write-Host ("    {0,-52} {1,8}  {2}" -f "-----", "----", "---------") -ForegroundColor Gray
foreach ($t in $targets) {
    Write-Host ("    {0,-52} {1,8}  {2}" -f $t.Table, $t.SizePretty, $t.InterfaceName)
}
Write-Host ""
Write-Host "    Total to reclaim: ~${totalMB} MB across $($targets.Count) table(s)" -ForegroundColor Yellow

if ($DryRun) {
    Write-Host ""
    Write-Host "    DRY RUN complete -- rerun without -DryRun to apply changes." -ForegroundColor Yellow
    exit 0
}

Write-Step "Truncating tables"
foreach ($t in $targets) {
    Write-Info "Truncating $($t.Table) ($($t.SizePretty))..."
    "TRUNCATE TABLE $($t.Table);" | docker exec -i $container psql -U ezhealth_user -d ezhealthkonnect -t | Out-Null
    Write-OK "$($t.InterfaceName)"
}

Write-Step "Running VACUUM ANALYZE"
foreach ($t in $targets) {
    "VACUUM ANALYZE $($t.Table);" | docker exec -i $container psql -U ezhealth_user -d ezhealthkonnect -t | Out-Null
}
Write-OK "Done"

Write-Step "Pruning Docker build cache"
& { $ErrorActionPreference = "SilentlyContinue"; docker builder prune -f 2>&1 | Out-Null }
Write-OK "Build cache cleared"

# Final summary
$afterSql = "SELECT pg_size_pretty(SUM(pg_total_relation_size(c.oid))) FROM pg_class c WHERE c.relname LIKE 'messages_intf_%' AND c.relkind = 'r';"
$afterSize = ((Invoke-PsqlPipe $afterSql) | Where-Object { "$_".Trim() -ne "" } | Select-Object -First 1).ToString().Trim()
$freeGB = [math]::Round((Get-PSDrive C).Free / 1GB, 1)

Write-Host ""
Write-Host "    Reclaimed   : ~${totalMB} MB" -ForegroundColor Green
Write-Host "    Msg tables  : $afterSize remaining" -ForegroundColor Green
Write-Host "    C: free     : ${freeGB} GB" -ForegroundColor Green
Write-Host ""
Write-Host "Done." -ForegroundColor Green
