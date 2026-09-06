$ErrorActionPreference = "Stop"

if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Wails build tool..."
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
}

Push-Location (Join-Path $PSScriptRoot "..")
try {
    npm --prefix apps/web ci
    npm --prefix apps/web run build
    wails build -clean -platform windows/amd64 -nsis -installscope user
    Write-Host "Installer created under build/bin."
} finally {
    Pop-Location
}
