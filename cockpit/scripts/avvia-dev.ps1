# Avvia l'ambiente di sviluppo del Cockpit: compila cockpit.exe e apre le finestre
# di servizio (server e worker analisi). Rilanciarlo riavvia tutto: i worker sopravvivono al
# riavvio del server, ma il modo più semplice per essere sicuri è ripartire da qui.
#
#   powershell -ExecutionPolicy Bypass -File scripts\avvia-dev.ps1 [-NoBuild] [-ConOutlook]
#
# Il worker Outlook NON parte da solo. Si attacca via COM alla casella vera del profilo di
# questo PC: avviarlo è un accesso alla posta reale e va deciso ogni volta, non ereditato da
# un doppio clic. Con -ConOutlook parte anche lui.
param([switch]$NoBuild, [switch]$ConOutlook)

$ErrorActionPreference = "Stop"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

& "$PSScriptRoot\ferma-dev.ps1"

if (-not $NoBuild) {
    Write-Host "== go build" -ForegroundColor Cyan
    go build -o cockpit.exe .\cmd\cockpit
    if ($LASTEXITCODE -ne 0) { throw "compilazione fallita" }
}

if ($ConOutlook -and -not (Get-Process OUTLOOK -ErrorAction SilentlyContinue)) {
    Write-Warning "Outlook classico non è in esecuzione: il worker Outlook fallirà i job finché non lo apri."
}

$python = (Get-Command python).Source
$log = Join-Path $radice "..\_staging\log"
New-Item -ItemType Directory -Force $log | Out-Null

Write-Host "== cockpit.exe" -ForegroundColor Cyan
Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='cockpit.exe'; Set-Location '$radice'; .\cockpit.exe -config cockpit.toml 2>&1 | Tee-Object -FilePath '$log\cockpit.log'"
Start-Sleep -Seconds 2

if ($ConOutlook) {
    Write-Host "== worker_outlook.py (legge la casella vera di questo profilo)" -ForegroundColor Yellow
    Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='worker outlook'; Set-Location '$radice\workers'; & '$python' worker_outlook.py"
} else {
    Write-Host "== worker_outlook.py NON avviato: serve -ConOutlook" -ForegroundColor DarkGray
}

Write-Host "== worker_analisi.py" -ForegroundColor Cyan
Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='worker analisi'; Set-Location '$radice\workers'; & '$python' worker_analisi.py"

Write-Host ""
Write-Host "Cockpit: http://127.0.0.1:8080  (login PS / cockpit)" -ForegroundColor Green
Write-Host "Log worker: $log"
