# Avvio automatico e riavvio dei worker come attività pianificate (piano, voce 9.1).
#
# I worker devono ripartire da soli dopo un riavvio del PC, un crash o un arresto volontario
# (`os._exit(3)` quando il lease è perso, fase 1 voce 1.5): senza riavvio automatico il recupero del
# worker bloccato in COM resterebbe teoria. Le attività girano nella sessione dell'utente perché
# Outlook classico richiede un profilo interattivo: niente "Esegui anche se l'utente non ha eseguito
# l'accesso", che farebbe fallire ogni chiamata COM.
#
#   powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1 -Mostra
#   powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1 -Installa
#   powershell -ExecutionPolicy Bypass -File scripts\installa-attivita.ps1 -Disinstalla
#
# Il mutex per PC sta dentro il worker (un solo worker per tipo e per postazione): qui si evita solo
# che l'attività ne avvii un secondo mentre il primo è vivo (-MultipleInstances IgnoreNew).
param(
    [switch]$Installa,
    [switch]$Disinstalla,
    [switch]$Mostra,
    [string]$Prefisso = "Cockpit"
)

$ErrorActionPreference = "Stop"
$radice = Split-Path -Parent $PSScriptRoot
$workers = Join-Path $radice "workers"
$python = (Get-Command python -ErrorAction SilentlyContinue)?.Source
if (-not $python) { $python = (Get-Command py -ErrorAction SilentlyContinue)?.Source }

$attivita = @(
    @{ Nome = "$Prefisso - worker Outlook"; Script = "worker_outlook.py" },
    @{ Nome = "$Prefisso - worker analisi"; Script = "worker_analisi.py" }
)

if ($Mostra) {
    Get-ScheduledTask -TaskName "$Prefisso*" -ErrorAction SilentlyContinue |
        Select-Object TaskName, State, @{n = "UltimaEsecuzione"; e = { (Get-ScheduledTaskInfo $_).LastRunTime } },
        @{n = "UltimoEsito"; e = { (Get-ScheduledTaskInfo $_).LastTaskResult } } | Format-Table -AutoSize
    exit 0
}

if ($Disinstalla) {
    foreach ($a in $attivita) {
        if (Get-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue) {
            Unregister-ScheduledTask -TaskName $a.Nome -Confirm:$false
            Write-Host "rimossa: $($a.Nome)" -ForegroundColor Green
        }
    }
    exit 0
}

if (-not $Installa) {
    Write-Host "uso: installa-attivita.ps1 -Installa | -Disinstalla | -Mostra"
    Write-Host ""
    Write-Host "Cosa farebbe -Installa su questo PC:"
    Write-Host "  python  : $python"
    Write-Host "  workers : $workers"
    foreach ($a in $attivita) { Write-Host "  attività: $($a.Nome) → $($a.Script)" }
    exit 0
}

if (-not $python) { throw "python non trovato nel PATH" }
if (-not (Test-Path (Join-Path $workers "worker.toml"))) {
    throw "manca $workers\worker.toml: copiarlo da worker.toml.example e compilarlo prima di installare le attività"
}

foreach ($a in $attivita) {
    $azione = New-ScheduledTaskAction -Execute $python -Argument $a.Script -WorkingDirectory $workers
    $inneschi = @(
        (New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME),
        # rete di sicurezza: se l'attività è ferma per qualsiasi motivo, riparte entro 5 minuti
        (New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 5))
    )
    $impostazioni = New-ScheduledTaskSettingsSet `
        -MultipleInstances IgnoreNew `
        -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
        -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable
    $principale = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited

    Register-ScheduledTask -TaskName $a.Nome -Action $azione -Trigger $inneschi -Settings $impostazioni `
        -Principal $principale -Description "Cockpit RFQ: $($a.Script). Riavvio automatico; richiede la sessione interattiva perché usa Outlook classico." -Force | Out-Null
    Write-Host "installata: $($a.Nome)" -ForegroundColor Green
}

Write-Host ""
Write-Host "Verifica con: scripts\installa-attivita.ps1 -Mostra"
Write-Host "Il server cockpit.exe non è qui: in produzione va installato come servizio (nssm o sc.exe),"
Write-Host "perché non usa Outlook e non ha bisogno di una sessione interattiva."
