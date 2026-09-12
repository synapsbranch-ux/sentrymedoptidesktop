# Run elevated, once, on the clinic server after placing Caddyfile,
# start-caddy.ps1, update-duckdns-ip.ps1 and duckdns-token.txt under
# C:\ProgramData\SentryMed (see docs/DEPLOYMENT.md). Registers Caddy to run at
# boot, keeps the DuckDNS record current, and opens the firewall to the
# clinic subnet only.
param(
    [string]$Subnet = "192.168.1.0/24",
    [string]$RunAsUser = "SYSTEM"
)
$ErrorActionPreference = "Stop"

$root = "C:\ProgramData\SentryMed"

$caddyAction = New-ScheduledTaskAction -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$root\start-caddy.ps1`""
$caddyTrigger = New-ScheduledTaskTrigger -AtStartup
# ExecutionTimeLimit zero = never kill it; Caddy is meant to run forever.
$caddySettings = New-ScheduledTaskSettingsSet -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) -MultipleInstances IgnoreNew -StartWhenAvailable
$principal = New-ScheduledTaskPrincipal -UserId $RunAsUser -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -TaskName "SentryMed Caddy" -Action $caddyAction -Trigger $caddyTrigger `
    -Settings $caddySettings -Principal $principal -Force

$duckdnsAction = New-ScheduledTaskAction -Execute "powershell.exe" `
    -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$root\update-duckdns-ip.ps1`""
$duckdnsTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date) -RepetitionInterval (New-TimeSpan -Hours 1) -RepetitionDuration ([TimeSpan]::MaxValue)
Register-ScheduledTask -TaskName "SentryMed DuckDNS updater" -Action $duckdnsAction -Trigger $duckdnsTrigger `
    -Settings $caddySettings -Principal $principal -Force

New-NetFirewallRule -DisplayName "SentryMed HTTPS (clinic LAN)" -Direction Inbound `
    -Protocol TCP -LocalPort 443 -RemoteAddress $Subnet -Action Allow -Program "$root\caddy.exe" -Force
New-NetFirewallRule -DisplayName "SentryMed HTTP redirect (clinic LAN)" -Direction Inbound `
    -Protocol TCP -LocalPort 80 -RemoteAddress $Subnet -Action Allow -Program "$root\caddy.exe" -Force

Write-Host "Registered Scheduled Tasks and firewall rules. Start Caddy now with:"
Write-Host "  Start-ScheduledTask -TaskName `"SentryMed Caddy`""
