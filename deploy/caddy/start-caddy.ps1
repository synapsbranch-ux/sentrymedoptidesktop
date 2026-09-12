# Launcher invoked by the "SentryMed Caddy" Scheduled Task (see
# scripts/install-caddy-task.ps1). Task Scheduler has no per-task environment
# variable, and a machine-level DUCKDNS_API_TOKEN would expose it to every
# account on the box, so the token lives only in this ACL'd file.
$ErrorActionPreference = "Stop"

$root = "C:\ProgramData\SentryMed"
$env:DUCKDNS_API_TOKEN = (Get-Content "$root\duckdns-token.txt" -Raw).Trim()

$updater = "$root\update-duckdns-ip.ps1"
if (Test-Path $updater) {
    & $updater
}

& "$root\caddy.exe" run --config "$root\Caddyfile"
