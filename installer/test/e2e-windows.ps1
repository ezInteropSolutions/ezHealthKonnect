# End-to-end test of the REAL Standalone installer on a fresh Windows host
# (GitHub Actions windows runner, administrator). Same scenario as
# e2e-linux.sh: install, setup token, data, upgrade, failing upgrade (must roll
# back), manual rollback, uninstall.
#
# -PostgresMode existing : reuse the PostgreSQL preinstalled on the runner
# -PostgresMode fresh    : remove it first, so the installer must download,
#                          verify and run the pinned PostgreSQL installer
param(
    [Parameter(Mandatory = $true)][string]$Installer,
    [Parameter(Mandatory = $true)][string]$BundleV1,
    [ValidateSet('existing', 'fresh')][string]$PostgresMode = 'fresh'
)
$ErrorActionPreference = 'Stop'
$Installer = (Resolve-Path $Installer).Path
$BundleV1 = (Resolve-Path $BundleV1).Path
$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Work = Join-Path $env:RUNNER_TEMP ("ezhk-e2e-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $Work | Out-Null
$Dir = 'C:\ezhk-e2e'
$AppPort = 3000
$AdminEmail = 'e2e-admin@example.org'
$AdminPassword = 'E2e-' + [guid]::NewGuid().ToString('N').Substring(0, 20)
$Base = "http://127.0.0.1:$AppPort"

function Pass($m) { Write-Host "  PASS  $m" }
function Fail($m) {
    Write-Host "  FAIL  $m" -ForegroundColor Red
    Write-Host '---- diagnostics'
    Get-Service ezhealthkonnect -ErrorAction SilentlyContinue | Format-List | Out-String | Write-Host
    Get-ChildItem "$Dir\logs" -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
        Write-Host "== $($_.FullName)"; Get-Content $_.FullName -Tail 40 -ErrorAction SilentlyContinue | Write-Host }
    exit 1
}
function Step($m) { Write-Host ''; Write-Host "==== $m" }
function EnvGet($k) { ((Get-Content "$Dir\.env" | Where-Object { $_ -like "$k=*" }) -split '=', 2)[1] }
function Run-Installer([string[]]$ArgList) {
    & $Installer @ArgList 2>&1 | Tee-Object -Variable out | Write-Host
    return @{ Code = $LASTEXITCODE; Out = ($out -join "`n") }
}
function Version { ((& $Installer status --dir $Dir) -join "`n" | ConvertFrom-Json).version }
function Healthy {
    try { (Invoke-RestMethod "$Base/api/system/health" -TimeoutSec 10).database.status -eq 'connected' } catch { $false }
}
function PsqlBin { (Get-ChildItem 'C:\Program Files\PostgreSQL\*\bin\psql.exe' | Sort-Object FullName | Select-Object -Last 1).FullName }
function Psql-App($sql) {
    $env:PGPASSWORD = EnvGet 'DB_PASSWORD'
    $r = & (PsqlBin) -X -q -t -A -h 127.0.0.1 -p (EnvGet 'DB_PORT') -U (EnvGet 'DB_USER') -d (EnvGet 'DB_NAME') -c $sql
    Remove-Item Env:PGPASSWORD
    return ($r | Out-String).Trim()
}
function Assert-ServiceAccount($when) {
    # The service runs as its own virtual account, not LocalSystem, with read
    # access to the application, write access to data, none to backups.
    $acct = 'NT SERVICE\ezhealthkonnect'
    $svc = Get-CimInstance Win32_Service -Filter "Name='ezhealthkonnect'"
    if ($svc.StartName -ne $acct) { Fail "$when`: service runs as '$($svc.StartName)', want $acct" }
    $has = { param($path, $right) (Get-Acl $path).Access | Where-Object {
        $_.IdentityReference -eq $acct -and $_.AccessControlType -eq 'Allow' -and ($_.FileSystemRights -band $right) -eq $right } }
    if (-not (& $has $Dir ([System.Security.AccessControl.FileSystemRights]::ReadAndExecute))) { Fail "$when`: service cannot read the application" }
    if (& $has $Dir ([System.Security.AccessControl.FileSystemRights]::Write)) { Fail "$when`: service can modify the application files" }
    if (-not (& $has "$Dir\storage" ([System.Security.AccessControl.FileSystemRights]::Modify))) { Fail "$when`: service cannot write storage" }
    if ((Get-Acl "$Dir\backups").Access | Where-Object { $_.IdentityReference -eq $acct }) { Fail "$when`: service has access to backups" }
    if (Test-Path "$Dir\tools\pg-superuser.secret") {
        if ((Get-Acl "$Dir\tools\pg-superuser.secret").Access | Where-Object { $_.IdentityReference -eq $acct }) { Fail "$when`: service can read the PostgreSQL superuser password" }
    }
}
function Login {
    $script:Session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    Invoke-RestMethod "$Base/api/auth/login" -Method Post -ContentType 'application/json' -WebSession $script:Session `
        -Body (@{ email = $AdminEmail; password = $AdminPassword } | ConvertTo-Json) | Out-Null
}

Step "Prepare PostgreSQL ($PostgresMode)"
$answers = @{ installDir = $Dir; appPort = "$AppPort"; apiPort = '8080'; dbMode = 'local' }
$preinstalled = Get-Service 'postgresql*' -ErrorAction SilentlyContinue | Select-Object -First 1
if ($PostgresMode -eq 'existing') {
    if (-not $preinstalled) { Write-Host 'No preinstalled PostgreSQL on this runner — skipping the existing-server scenario.'; exit 0 }
    Set-Service $preinstalled.Name -StartupType Manual
    Start-Service $preinstalled.Name
    if (-not $env:PGPASSWORD) { Fail 'runner does not expose the preinstalled superuser password (PGPASSWORD)' }
    $answers.pgAdminPassword = $env:PGPASSWORD
    Pass "using $($preinstalled.Name)"
} elseif ($preinstalled) {
    Stop-Service $preinstalled.Name -ErrorAction SilentlyContinue
    $uninstaller = Get-ChildItem 'C:\Program Files\PostgreSQL\*\uninstall-postgresql.exe' -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($uninstaller) { & $uninstaller.FullName --mode unattended | Out-Null }
    sc.exe delete $preinstalled.Name | Out-Null
    Remove-Item 'C:\Program Files\PostgreSQL' -Recurse -Force -ErrorAction SilentlyContinue
    reg delete 'HKLM\SOFTWARE\PostgreSQL' /f 2>$null | Out-Null
    Remove-Item Env:PGPASSWORD -ErrorAction SilentlyContinue
    Pass 'removed the preinstalled PostgreSQL — the installer must install its own'
}

Step 'Upgrade-test bundles'
python "$Here\make_variant_bundle.py" $BundleV1 "$Work\v2.zip" v0.0.2-ci --add database/migrations/V90001__E2E_Upgrade_Marker.sql 'CREATE TABLE e2e_upgrade_marker (id int PRIMARY KEY); INSERT INTO e2e_upgrade_marker VALUES (1);'
python "$Here\make_variant_bundle.py" "$Work\v2.zip" "$Work\v3.zip" v0.0.3-ci --add database/migrations/V90002__E2E_Failing_Migration.sql 'CREATE TABLE e2e_must_not_exist (id int); SELECT 1/0;'

Step 'Install (headless)'
$answers | ConvertTo-Json | Set-Content -Encoding ascii "$Work\answers.json"
$r = Run-Installer @('install', '--config', "$Work\answers.json")
if ($r.Code -ne 0) { Fail "install exited $($r.Code)" }
if (-not (Healthy)) { Fail 'not healthy after install' }
$svc = Get-Service ezhealthkonnect
if ($svc.Status -ne 'Running' -or $svc.StartType -ne 'Automatic') { Fail "service state $($svc.Status)/$($svc.StartType)" }
Pass "installed and healthy ($(Version))"

Step 'Security of the installation'
foreach ($p in @($Dir, "$Dir\.env", "$Dir\start-app.ps1")) {
    $bad = (Get-Acl $p).Access | Where-Object {
        $_.IdentityReference -match 'Users|Everyone|Authenticated' -and $_.AccessControlType -eq 'Allow' }
    if ($bad) { Fail "$p is accessible to $($bad.IdentityReference -join ', ')" }
}
if (-not (EnvGet 'SETUP_TOKEN')) { Fail 'no SETUP_TOKEN' }
if (-not (EnvGet 'APP_CREDENTIAL_KEY')) { Fail 'no APP_CREDENTIAL_KEY' }
if ($r.Out -notmatch [regex]::Escape("setup.html?token=$(EnvGet 'SETUP_TOKEN')")) { Fail 'installer did not print the setup link' }
if ($PostgresMode -eq 'fresh' -and (Test-Path "$Dir\tools\pg-superuser.secret")) {
    if ((Get-Content "$Dir\tools\pg-superuser.secret") -eq 'postgres') { Fail 'PostgreSQL superuser password is the default' }
}
Assert-ServiceAccount 'after install'
Pass 'install directory restricted; service runs as its virtual account; secrets present'

Step 'First-run setup requires the token'
$setup = @{ orgName = 'E2E'; firstName = 'E'; lastName = 'Tester'; email = $AdminEmail; password = $AdminPassword }
try {
    Invoke-RestMethod "$Base/api/setup/complete" -Method Post -ContentType 'application/json' -Body ($setup | ConvertTo-Json) | Out-Null
    Fail 'setup without token was accepted'
} catch { if ($_.Exception.Response.StatusCode.value__ -ne 403) { Fail "setup without token: $($_.Exception.Message)" } }
$setup.setupToken = EnvGet 'SETUP_TOKEN'
Invoke-RestMethod "$Base/api/setup/complete" -Method Post -ContentType 'application/json' -Body ($setup | ConvertTo-Json) | Out-Null
Login
Pass 'setup refused without token, completed with it, admin logs in'

Step 'Create data that upgrades must keep'
Invoke-RestMethod "$Base/api/interfaces" -Method Post -ContentType 'application/json' -WebSession $script:Session -Body (@{
    name = 'E2E Interface'; messageType = 'ADT^A01'; description = 'e2e'; sourceType = ''; targetType = ''; sourceConfig = @{}; targetConfig = @{}
} | ConvertTo-Json) | Out-Null
if ((Psql-App "SELECT count(*) FROM interfaces WHERE name='E2E Interface'") -ne '1') { Fail 'interface not stored' }
Set-Content "$Dir\storage\e2e-object.txt" 'patient-data'
$secretsBefore = (Get-Content "$Dir\.env" | Where-Object { $_ -match '^(DB_PASSWORD|SESSION_SECRET|JWT_SECRET|APP_CREDENTIAL_KEY)=' }) -join "`n"
Pass 'interface and stored object created'

Step 'Upgrade to v0.0.2-ci'
$r = Run-Installer @('upgrade', '--dir', $Dir, '--bundle', "$Work\v2.zip")
if ($r.Code -ne 0) { Fail 'upgrade to v2 failed' }
if ((Version) -ne 'v0.0.2-ci') { Fail "version after upgrade: $(Version)" }
if (-not (Healthy)) { Fail 'not healthy after upgrade' }
if ((Psql-App 'SELECT count(*) FROM e2e_upgrade_marker') -ne '1') { Fail 'new migration not applied' }
if ((Psql-App "SELECT count(*) FROM interfaces WHERE name='E2E Interface'") -ne '1') { Fail 'data lost in upgrade' }
if ((Get-Content "$Dir\storage\e2e-object.txt") -ne 'patient-data') { Fail 'stored object lost' }
$secretsAfter = (Get-Content "$Dir\.env" | Where-Object { $_ -match '^(DB_PASSWORD|SESSION_SECRET|JWT_SECRET|APP_CREDENTIAL_KEY)=' }) -join "`n"
if ($secretsAfter -ne $secretsBefore) { Fail 'secrets changed' }
Login
Assert-ServiceAccount 'after upgrade'
Pass 'upgraded with data, secrets and login intact'

Step 'Failing upgrade must roll back'
$r = Run-Installer @('upgrade', '--dir', $Dir, '--bundle', "$Work\v3.zip")
if ($r.Code -eq 0) { Fail 'an upgrade with a failing migration must fail' }
if ($r.Out -notmatch 'rolled back') { Fail 'failure was not rolled back' }
if ((Version) -ne 'v0.0.2-ci') { Fail "version after failed upgrade: $(Version)" }
if (-not (Healthy)) { Fail 'previous version not healthy after automatic rollback' }
if ((Psql-App "SELECT to_regclass('e2e_must_not_exist') IS NULL") -ne 't') { Fail 'failed migration left objects' }
Login
Pass 'failed upgrade rolled back automatically'

Step 'Manual rollback to v0.0.1-ci'
$r = Run-Installer @('rollback', '--dir', $Dir, '--yes')
if ($r.Code -ne 0) { Fail 'rollback failed' }
if ((Version) -ne 'v0.0.1-ci') { Fail "version after rollback: $(Version)" }
if (-not (Healthy)) { Fail 'not healthy after rollback' }
if ((Psql-App "SELECT to_regclass('e2e_upgrade_marker') IS NULL") -ne 't') { Fail 'database not restored' }
if ((Psql-App "SELECT count(*) FROM interfaces WHERE name='E2E Interface'") -ne '1') { Fail 'data from before the upgrade lost' }
Login
Pass 'rolled back to the previous version and database'

Step 'Uninstall'
$r = Run-Installer @('uninstall', '--dir', $Dir, '--yes')
if ($r.Code -ne 0) { Fail 'uninstall failed' }
# The database setup created is always dropped. On PostgreSQL that setup installed
# itself it uses the saved administrator password; on a PostgreSQL that already
# existed (where no administrator password is stored) it drops the database as
# its owner, the application user.
if ($r.Out -notmatch 'Database ezhealthkonnect removed') { Fail "database not removed: $($r.Out)" }
Start-Sleep -Seconds 8
if (Test-Path $Dir) { Fail 'install dir still present' }
if (Get-Service ezhealthkonnect -ErrorAction SilentlyContinue) { Fail 'service still registered' }
reg query 'HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\ezHealthKonnect' 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) { Fail 'Add/Remove Programs entry still present' }
Pass 'uninstalled'

# Uninstall followed by install must work in both scenarios, with no leftover
# database in the way ("database ezhealthkonnect already exists").
#   fresh:    PostgreSQL stays, and its generated superuser password (once kept
#             in the deleted install folder) must be found again without being asked.
#   existing: the customer's administrator password is supplied again, as in a real reinstall.
Step 'Reinstall after uninstall'
$r = Run-Installer @('install', '--config', "$Work\answers.json")
if ($r.Code -ne 0) { Fail "reinstall exited $($r.Code): $($r.Out)" }
if ($PostgresMode -eq 'fresh' -and $r.Out -notmatch 'earlier ezHealthKonnect install set up') { Fail 'reinstall did not reuse the saved PostgreSQL password' }
if (-not (Healthy)) { Fail 'not healthy after reinstall' }
Pass 'clean reinstall works, no leftover database'
$r = Run-Installer @('uninstall', '--dir', $Dir, '--yes')
if ($r.Code -ne 0) { Fail 'uninstall after reinstall failed' }
if ($r.Out -notmatch 'Database ezhealthkonnect removed') { Fail "database not removed after reinstall: $($r.Out)" }
Start-Sleep -Seconds 8
if (Test-Path $Dir) { Fail 'install dir still present after the second uninstall' }
Pass 'second uninstall removed the database too'

Write-Host ''
Write-Host 'ALL INSTALLER E2E CHECKS PASSED'
exit 0   # native commands above (reg query) leave a non-zero $LASTEXITCODE
