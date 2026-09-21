# Installa (o aggiorna) i worker del Cockpit RFQ su QUESTA postazione (7C.1, P1).
#
# Sta nel pacchetto che si scarica dalla pagina Postazioni, accanto a worker.toml, e si lancia dalla
# cartella in cui il pacchetto e' stato scompattato:
#
#   powershell -ExecutionPolicy Bypass -File installa-postazione.ps1            installa/aggiorna e avvia
#   powershell -ExecutionPolicy Bypass -File installa-postazione.ps1 -Mostra    stato delle attivita'
#   powershell -ExecutionPolicy Bypass -File installa-postazione.ps1 -Ferma     ferma attivita' e worker
#   powershell -ExecutionPolicy Bypass -File installa-postazione.ps1 -Disinstalla
#
# Che cosa fa, nell'ordine:
#   1. ferma le attivita' pianificate e i processi worker di questa cartella (sempre: cosi'
#      aggiornare un pacchetto e' ferma -> sostituisci -> riparti, in un comando solo);
#   2. controlla Python (3.11 o piu', serve tomllib) e installa le dipendenze di requirements.txt;
#   3. crea la cartella `staging` dichiarata in worker.toml (e' la cartella DEL WORKER: log e file
#      temporanei, non lo staging del server);
#   4. se c'e' cert.pem, lo installa fra le autorita' radice dell'UTENTE (Cert:\CurrentUser\Root):
#      e' quello che fa accettare al browser il certificato autofirmato del server. Il worker non ne
#      ha bisogno, verifica l'impronta. Windows chiede una conferma: e' normale, e' l'unica domanda;
#   5. registra un'attivita' pianificata per ogni worker dichiarato in worker.toml ([outlook],
#      [analisi]): parte all'accesso dell'utente, istanza singola, riavvio automatico, senza finestra
#      (pythonw), con il log in <staging>\log\<worker>.log;
#   6. avvia le attivita' e mostra lo stato.
#
# Le attivita' girano nella SESSIONE DELL'UTENTE, non come servizio: Outlook classico e' COM e non
# esiste fuori da una sessione interattiva. Niente «Esegui anche se l'utente non ha eseguito l'accesso».
# Windows PowerShell 5.1: niente operatori ?. ?? e niente && fra comandi.
param(
    [switch]$Ferma,
    [switch]$Disinstalla,
    [switch]$Mostra,
    [switch]$SenzaCertificato,
    [switch]$SenzaDipendenze,
    [string]$Prefisso = "Cockpit"
)

$ErrorActionPreference = "Stop"
$qui = $PSScriptRoot
if (-not $qui) { $qui = Split-Path -Parent $MyInvocation.MyCommand.Path }
$tomlPath = Join-Path $qui "worker.toml"

function Leggi-Toml([string]$percorso) {
    # Un lettore minimo: sezioni [nome] e chiavi `chiave = "valore"` / 'valore' / true|false. Basta per
    # worker.toml, che scrive il server. Non e' un parser TOML: non deve esserlo.
    $dati = @{ "" = @{} }
    $sezione = ""
    foreach ($riga in Get-Content $percorso -Encoding UTF8) {
        $r = $riga.Trim()
        if ($r -eq "" -or $r.StartsWith("#")) { continue }
        if ($r -match '^\[([A-Za-z0-9_]+)\]$') { $sezione = $Matches[1]; if (-not $dati.ContainsKey($sezione)) { $dati[$sezione] = @{} }; continue }
        if ($r -match '^([A-Za-z0-9_]+)\s*=\s*(.*)$') {
            $chiave = $Matches[1]; $valore = $Matches[2].Trim()
            if ($valore -match '^"(.*)"\s*(#.*)?$') { $valore = $Matches[1] }
            elseif ($valore -match "^'(.*)'\s*(#.*)?$") { $valore = $Matches[1] }
            elseif ($valore -match '^(\S+)\s*(#.*)?$') { $valore = $Matches[1] }
            $dati[$sezione][$chiave] = $valore
        }
    }
    return $dati
}

$attivitaNote = @(
    @{ Sezione = "outlook"; Nome = "$Prefisso - worker Outlook"; Script = "worker_outlook.py" },
    @{ Sezione = "analisi"; Nome = "$Prefisso - worker analisi"; Script = "worker_analisi.py" }
)

function Ferma-Tutto {
    foreach ($a in $attivitaNote) {
        $t = Get-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue
        if ($t) { Stop-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue; Disable-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue | Out-Null }
    }
    # i processi worker di questo PC, avviati a mano o dall'attivita' (un solo worker per tipo e per PC)
    $fermati = 0
    Get-CimInstance Win32_Process | Where-Object {
        $_.Name -match '^python' -and $_.CommandLine -match 'worker_(outlook|analisi)\.py'
    } | ForEach-Object {
        try { Stop-Process -Id $_.ProcessId -Force -ErrorAction Stop; $fermati++ } catch {}
    }
    Write-Host ("worker fermati: {0}" -f $fermati)
}

if ($Mostra) {
    Get-ScheduledTask -TaskName "$Prefisso*" -ErrorAction SilentlyContinue |
        Select-Object TaskName, State, @{n = "UltimaEsecuzione"; e = { (Get-ScheduledTaskInfo $_).LastRunTime } },
        @{n = "UltimoEsito"; e = { (Get-ScheduledTaskInfo $_).LastTaskResult } } | Format-Table -AutoSize
    if (Test-Path $tomlPath) {
        $cfg = Leggi-Toml $tomlPath
        $staging = $cfg[""]["staging"]
        if ($staging) {
            if (-not [System.IO.Path]::IsPathRooted($staging)) { $staging = Join-Path $qui $staging }
            $log = Join-Path $staging "log"
            Write-Host "log dei worker: $log"
        }
    }
    exit 0
}

if ($Ferma) { Ferma-Tutto; Write-Host "Fermato. Per ripartire: installa-postazione.ps1" -ForegroundColor Green; exit 0 }

if ($Disinstalla) {
    Ferma-Tutto
    foreach ($a in $attivitaNote) {
        if (Get-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue) {
            Unregister-ScheduledTask -TaskName $a.Nome -Confirm:$false
            Write-Host "rimossa: $($a.Nome)" -ForegroundColor Green
        }
    }
    exit 0
}

# ---------------------------------------------------------------- installazione

if (-not (Test-Path $tomlPath)) {
    throw "manca ${tomlPath}: questo script va lanciato dalla cartella in cui e' stato scompattato il pacchetto della postazione (pagina Postazioni del Cockpit)"
}
$cfg = Leggi-Toml $tomlPath
$serverUrl = $cfg[""]["server_url"]
Write-Host "== postazione $($env:COMPUTERNAME), server $serverUrl" -ForegroundColor Cyan

Write-Host "== 1. fermo cio' che gira" -ForegroundColor Cyan
Ferma-Tutto

Write-Host "== 2. Python e dipendenze" -ForegroundColor Cyan
$cmd = Get-Command python -ErrorAction SilentlyContinue
if (-not $cmd) { $cmd = Get-Command py -ErrorAction SilentlyContinue }
if (-not $cmd) { throw "python non trovato nel PATH: installare Python 3.11 o superiore (python.org, con «Add to PATH»)" }
$python = $cmd.Source
$versione = & $python -c "import sys; print('%d.%d' % sys.version_info[:2])"
if ([version]$versione -lt [version]"3.11") { throw "Python $versione trovato in ${python}: serve 3.11 o superiore (tomllib)" }
Write-Host "   python $versione : $python"
if (-not $SenzaDipendenze) {
    & $python -m pip install --disable-pip-version-check -q -r (Join-Path $qui "requirements.txt")
    if ($LASTEXITCODE -ne 0) { throw "pip install fallito: vedere sopra" }
    Write-Host "   dipendenze installate (requirements.txt)"
}
# pythonw: stesso interprete, senza finestra. Il log e' su file (configura_log), la console non serve.
$pythonw = Join-Path (Split-Path -Parent $python) "pythonw.exe"
if (-not (Test-Path $pythonw)) { $pythonw = $python }

Write-Host "== 3. cartella del worker" -ForegroundColor Cyan
$staging = $cfg[""]["staging"]
if (-not $staging) { $staging = "staging" }
if (-not [System.IO.Path]::IsPathRooted($staging)) { $staging = Join-Path $qui $staging }
New-Item -ItemType Directory -Force (Join-Path $staging "log") | Out-Null
Write-Host "   staging del worker: $staging"

$certPath = Join-Path $qui "cert.pem"
if ((Test-Path $certPath) -and -not $SenzaCertificato) {
    Write-Host "== 4. certificato del server per il browser" -ForegroundColor Cyan
    $cert = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2
    $cert.Import($certPath)
    $gia = Get-ChildItem Cert:\CurrentUser\Root | Where-Object { $_.Thumbprint -eq $cert.Thumbprint }
    if ($gia) {
        Write-Host "   gia' installato (impronta SHA1 $($cert.Thumbprint))"
    } else {
        Write-Host "   Windows chiedera' conferma per aggiungere «$($cert.Subject)» alle autorita' radice dell'utente." -ForegroundColor Yellow
        Import-Certificate -FilePath $certPath -CertStoreLocation Cert:\CurrentUser\Root | Out-Null
        Write-Host "   installato: da ora Edge e Chrome accettano $serverUrl senza avvisi (se l'host e' fra i nomi del certificato)"
    }
} elseif ($SenzaCertificato) {
    Write-Host "== 4. certificato NON installato (-SenzaCertificato)" -ForegroundColor DarkGray
} else {
    Write-Host "== 4. nessun cert.pem nel pacchetto: il server gira in chiaro o il pacchetto e' vecchio" -ForegroundColor DarkGray
}

Write-Host "== 5. attivita' pianificate" -ForegroundColor Cyan
$installate = 0
foreach ($a in $attivitaNote) {
    if (-not $cfg.ContainsKey($a.Sezione)) {
        # un worker non dichiarato in worker.toml non va su questo PC; se c'era un'attivita' di un
        # pacchetto precedente, si toglie
        if (Get-ScheduledTask -TaskName $a.Nome -ErrorAction SilentlyContinue) {
            Unregister-ScheduledTask -TaskName $a.Nome -Confirm:$false
            Write-Host "   rimossa (non piu' in worker.toml): $($a.Nome)"
        }
        continue
    }
    $azione = New-ScheduledTaskAction -Execute $pythonw -Argument $a.Script -WorkingDirectory $qui
    $inneschi = @(
        (New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME),
        # rete di sicurezza: se l'attivita' e' ferma per qualsiasi motivo, riparte entro 5 minuti
        (New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 5))
    )
    $impostazioni = New-ScheduledTaskSettingsSet `
        -MultipleInstances IgnoreNew `
        -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
        -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
        -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable
    $principale = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Limited
    Register-ScheduledTask -TaskName $a.Nome -Action $azione -Trigger $inneschi -Settings $impostazioni `
        -Principal $principale -Description "Cockpit RFQ: $($a.Script) in $qui. Riavvio automatico; sessione interattiva (Outlook classico)." -Force | Out-Null
    Enable-ScheduledTask -TaskName $a.Nome | Out-Null
    Start-ScheduledTask -TaskName $a.Nome
    Write-Host "   installata e avviata: $($a.Nome)  ($($a.Script))" -ForegroundColor Green
    $installate++
}
if ($installate -eq 0) { throw "worker.toml non dichiara nessuna sezione [outlook] o [analisi]: nessun worker da installare" }

Write-Host ""
Write-Host "Fatto. Stato:      installa-postazione.ps1 -Mostra" -ForegroundColor Green
Write-Host "Log dei worker:    $(Join-Path $staging 'log')"
Write-Host "Prima di RIGENERARE il pacchetto dal Cockpit: installa-postazione.ps1 -Ferma (i token vecchi muoiono nel momento in cui si rigenera)."
