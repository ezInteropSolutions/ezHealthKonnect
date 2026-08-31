# start-app.ps1
#
# Single-process wrapper so the standalone Windows install runs both the Go
# backend and the Node.js frontend under ONE Windows service instead of two
# independently-restartable ones — mirroring the Docker image's own startup
# command (Dockerfile: `CMD ["sh", "-c", "./go-api & sleep 3 && node server.js"]`).
# The two-service model was the root cause of a real bug: each half could be
# restarted independently, so after an .env change only one side would pick
# up the new value, leaving Node and Go disagreeing on the shared internal-
# proxy secret until BOTH were restarted together. One service removes that
# failure mode entirely, and gives the uninstaller a single thing to stop
# and remove instead of two.
#
# Run by WinSW as: powershell.exe -NoProfile -ExecutionPolicy Bypass
#                  -File start-app.ps1 -NodePath "<resolved node.exe path>"
param(
    [Parameter(Mandatory = $true)]
    [string]$NodePath
)

$ErrorActionPreference = 'Stop'
$here     = Split-Path -Parent $MyInvocation.MyCommand.Path
$goApiExe = Join-Path $here 'go-api.exe'
$serverJs = Join-Path $here 'server.js'

$apiProc = Start-Process -FilePath $goApiExe -WorkingDirectory $here -PassThru -WindowStyle Hidden

try {
    Start-Sleep -Seconds 3

    # Foreground, blocking: this wrapper (and therefore the Windows service)
    # stays alive exactly as long as the frontend does. WinSW's normal stop
    # request is expected to reach PowerShell as a break signal here, which
    # would unwind into the finally block below — based on WinSW's documented
    # behavior for a wrapped console process, NOT verified against a real
    # installed service in this session (no elevated access available to
    # install/stop a real Windows service). Confirm this on first real use.
    & $NodePath $serverJs
}
finally {
    # Always take the backend down with the frontend, however this exits
    # (normal stop, a Node crash, Ctrl+Break) — /T also catches anything
    # go-api.exe itself spawned. This can't run if the wrapper process
    # itself is hard-killed from outside (Task Manager "End process tree",
    # a forced TerminateProcess) rather than stopped through the service —
    # a real, accepted limitation, but no worse than today's two-service
    # model, where each half already has that same exposure individually.
    if ($apiProc -and -not $apiProc.HasExited) {
        Start-Process -FilePath 'taskkill.exe' `
            -ArgumentList '/PID', $apiProc.Id, '/T', '/F' `
            -WindowStyle Hidden -Wait -ErrorAction SilentlyContinue
    }
}
