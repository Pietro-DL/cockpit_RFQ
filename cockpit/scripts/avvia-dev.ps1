# Avvia l'ambiente di SVILUPPO del Cockpit su un PC solo: compila cockpit.exe e apre le finestre
# di servizio (server e worker analisi). Rilanciarlo riavvia tutto: i worker sopravvivono al
# riavvio del server, ma il modo più semplice per essere sicuri è ripartire da qui.
#
# NON è il launcher delle postazioni (7C.1, P1). Su un PC di lavoro i worker si installano con il
# pacchetto scaricato dalla pagina Postazioni e il suo `installa-postazione.ps1`: attività
# pianificate all'accesso dell'utente, certificato del server, riavvio automatico. Qui i worker
# girano in finestre aperte a mano, contro il server di QUESTO PC, con l'indirizzo letto da
# cockpit.toml.
#
#   powershell -ExecutionPolicy Bypass -File scripts\avvia-dev.ps1 [-NoBuild] [-ConOutlook] [-NoSemina]
#
# Il worker Outlook NON parte da solo. Si attacca via COM alla casella vera del profilo di
# questo PC: avviarlo è un accesso alla posta reale e va deciso ogni volta, non ereditato da
# un doppio clic. Con -ConOutlook parte anche lui.
#
# LE ANAGRAFICHE SI CARICANO QUI (checkpoint 7B.5). Prima di mettere in piedi server e worker questo
# script semina clienti, domini, buyer e fornitori dai due file di docs\. Senza, dopo un
# azzera-dati.ps1 il database ha lo schema ma non sa chi è nessuno: ogni mail arriva da uno
# sconosciuto, e l'Inbox mostra Buyer 0, Fornitori 0 e tutto il resto in Da validare.
#
# Non è il server a seminare: è questo script, prima di avviarlo. cockpit.exe non tocca le
# anagrafiche da solo nemmeno adesso, e il seed resta un'operazione che si vede passare. Con
# -NoSemina si salta; se i due file non ci sono (un altro PC, un repository appena clonato) lo
# script lo dice e prosegue, senza fermare l'avvio.
param([switch]$NoBuild, [switch]$ConOutlook, [switch]$NoSemina)

$ErrorActionPreference = "Stop"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

& "$PSScriptRoot\ferma-dev.ps1"

if (-not $NoBuild) {
    Write-Host "== go build" -ForegroundColor Cyan
    go build -o cockpit.exe .\cmd\cockpit
    if ($LASTEXITCODE -ne 0) { throw "compilazione fallita" }
}

# Le anagrafiche PRIMA del server: così il ricalcolo dei messaggi già arrivati (che il seed fa in
# coda) gira su un database che nessuno sta scrivendo, e la prima Inbox che si apre è già quella
# giusta. Lo script del seed applica anche le migrazioni, quindi va bene anche su un database
# appena azzerato.
if (-not $NoSemina) {
    $semeClienti = Join-Path $radice "..\docs\seme_anagrafica.json"
    $semeFornitori = Join-Path $radice "..\docs\seme_fornitori.json"
    if ((Test-Path $semeClienti) -and (Test-Path $semeFornitori)) {
        Write-Host "== anagrafiche: clienti, domini, buyer, fornitori" -ForegroundColor Cyan
        & "$PSScriptRoot\semina-anagrafiche.ps1" -Conferma
        if ($LASTEXITCODE -ne 0) { throw "seed delle anagrafiche fallito: il server non viene avviato" }
    } else {
        Write-Warning "seme delle anagrafiche non trovato in docs\: avvio senza clienti e senza fornitori, e l'Inbox mostrerà tutto in «Da validare»."
    }
} else {
    Write-Host "== anagrafiche NON caricate (-NoSemina)" -ForegroundColor DarkGray
}

if ($ConOutlook -and -not (Get-Process OUTLOOK -ErrorAction SilentlyContinue)) {
    Write-Warning "Outlook classico non è in esecuzione: il worker Outlook fallirà i job finché non lo apri."
}

$python = (Get-Command python).Source
$log = Join-Path $radice "..\_staging\log"
New-Item -ItemType Directory -Force $log | Out-Null

Write-Host "== cockpit.exe" -ForegroundColor Cyan
# Niente Tee-Object: da oggi il log su file lo scrive il server ([server].log_file, di default
# <nas.staging>\log\cockpit.log). Due scrittori sullo stesso file si contendono l'handle e si
# mescolano le righe; qui resta solo la finestra.
Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='cockpit.exe'; Set-Location '$radice'; .\cockpit.exe -config cockpit.toml"
Start-Sleep -Seconds 2

if ($ConOutlook) {
    Write-Host "== worker_outlook.py (legge la casella vera di questo profilo)" -ForegroundColor Yellow
    Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='worker outlook'; Set-Location '$radice\workers'; & '$python' worker_outlook.py"
} else {
    Write-Host "== worker_outlook.py NON avviato: serve -ConOutlook" -ForegroundColor DarkGray
}

Write-Host "== worker_analisi.py" -ForegroundColor Cyan
Start-Process powershell -ArgumentList "-NoExit", "-Command", "`$host.UI.RawUI.WindowTitle='worker analisi'; Set-Location '$radice\workers'; & '$python' worker_analisi.py"

# L'indirizzo si legge da cockpit.toml, non si scrive qui (7C.1, P1): questo script stampava
# «http://127.0.0.1:8080 (login PS / cockpit)» qualunque cosa dicesse il file, password compresa.
# Niente password: sta in cockpit.toml, e chi lancia il banco lo sa leggere.
$toml = Get-Content (Join-Path $radice "cockpit.toml") -Raw
$urlPubblico = if ($toml -match '(?m)^\s*url_pubblico\s*=\s*"([^"]+)"') { $Matches[1] } else { $null }
$indirizzo = if ($toml -match '(?m)^\s*indirizzo\s*=\s*"([^"]+)"') { $Matches[1] } else { "127.0.0.1:8080" }
$conTls = ($toml -match '(?m)^\s*tls_cert\s*=') -and ($toml -match '(?m)^\s*tls_key\s*=')
$schema = if ($conTls) { "https" } else { "http" }
$url = if ($urlPubblico) { $urlPubblico } else { "${schema}://$($indirizzo -replace '^0\.0\.0\.0', $env:COMPUTERNAME)" }
Write-Host ""
Write-Host "Cockpit (banco di sviluppo): $url" -ForegroundColor Green
Write-Host "Log (server e worker): $log"
Write-Host "Questo e' il banco di SVILUPPO su un PC solo. Una postazione vera si installa con il pacchetto" -ForegroundColor DarkGray
Write-Host "della pagina Postazioni e il suo installa-postazione.ps1, non con questo script." -ForegroundColor DarkGray
