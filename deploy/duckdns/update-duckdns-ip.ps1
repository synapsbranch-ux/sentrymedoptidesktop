# Keeps the clinic's DuckDNS record pointed at this machine's current LAN
# address. Run once at Caddy startup (start-caddy.ps1) and again hourly (see
# scripts/install-caddy-task.ps1), so a DHCP reservation drift self-heals
# instead of breaking every phone in the clinic at once with no diagnostic.
$ErrorActionPreference = "Stop"

$domain = "CLINIC" # DuckDNS subdomain only, without ".duckdns.org"
$root = "C:\ProgramData\SentryMed"
$token = (Get-Content "$root\duckdns-token.txt" -Raw).Trim()

# The address on the interface that holds the default route -- the clinic
# LAN adapter, not a VPN/virtual adapter that might also be present.
$ifIndex = (Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Sort-Object RouteMetric | Select-Object -First 1).InterfaceIndex
$ip = (Get-NetIPAddress -InterfaceIndex $ifIndex -AddressFamily IPv4 | Select-Object -First 1).IPAddress
if (-not $ip) {
    throw "Could not determine this machine's LAN IPv4 address."
}

# The "ip=" parameter is mandatory: without it, DuckDNS records the clinic's
# PUBLIC internet address (auto-detected from the request), which no phone on
# the clinic LAN can reach. The record must point at the private LAN address.
$result = Invoke-RestMethod "https://www.duckdns.org/update?domains=$domain&token=$token&ip=$ip"
if ($result.Trim() -ne "OK") {
    throw "DuckDNS update failed for $domain -> $ip : $result"
}
Write-Host "DuckDNS $domain.duckdns.org -> $ip"
