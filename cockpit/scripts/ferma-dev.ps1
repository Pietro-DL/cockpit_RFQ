# Ferma server e worker del Cockpit avviati da avvia-dev.ps1 (o a mano).
Get-Process cockpit -ErrorAction SilentlyContinue | Stop-Process -Force
Get-CimInstance Win32_Process |
    Where-Object { $_.Name -match '^python' -and $_.CommandLine -match 'worker_(outlook|analisi)\.py' } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Write-Host "cockpit.exe e worker fermati."
