# Azzera i dati dell'ambiente di SVILUPPO: schema ricreato da zero e log cancellati.
#
#   powershell -ExecutionPolicy Bypass -File scripts\azzera-dati.ps1            # dice solo che cosa farebbe
#   powershell -ExecutionPolicy Bypass -File scripts\azzera-dati.ps1 -Conferma  # lo fa
#
# Serve a ripartire da una riga di partenza pulita quando si sta cercando un difetto: una coda con
# dentro quattro sync storici della settimana scorsa e tre job in corso di un worker che non esiste
# più non permette di capire se il passo che si sta provando ha funzionato.
#
# CHE COSA CANCELLA
#   - tutto lo schema `public` del database indicato da [db].dsn in cockpit.toml: messaggi, job,
#     cursori, presenze, scarti, allegati, proposte, sessioni. Anche le fondazioni (caselle,
#     postazioni, credenziali dei worker, utenti), che però rinascono da cockpit.toml al primo
#     riavvio di cockpit.exe, insieme alle migrazioni.
#   - i log di <nas.staging>\log (server e worker).
#   - con -AncheAllegati, anche i file già scaricati in <nas.staging>: senza le righe di `allegato`
#     nel database sono comunque irraggiungibili dalla UI.
#
# CHE COSA NON TOCCA MAI
#   - il NAS ([nas].radice) e la posta: da qui non si scrive né sull'uno né sull'altra.
#   - il database di test (cockpit_test, porta 5433): quello lo gestisce scripts\db-test.ps1.
#   - cockpit.toml e worker.toml.
param(
    [string]$Config = "cockpit.toml",
    [string]$Psql,
    [switch]$AncheAllegati,
    [switch]$Conferma
)

$ErrorActionPreference = "Stop"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

$percorsoConfig = Join-Path $radice $Config
if (-not (Test-Path $percorsoConfig)) { throw "configurazione non trovata: $percorsoConfig" }
$testo = Get-Content -Raw $percorsoConfig

$mDsn = [regex]::Match($testo, '(?m)^\s*dsn\s*=\s*"([^"]+)"')
if (-not $mDsn.Success) { throw "[db].dsn non trovato in $Config" }
$dsn = $mDsn.Groups[1].Value
$mascherato = [regex]::Replace($dsn, '://[^@/]*@', '://***@')
$database = ([regex]::Match($dsn, '/([^/?]+)(\?|$)')).Groups[1].Value

$mStaging = [regex]::Match($testo, "(?m)^\s*staging\s*=\s*['`"]([^'`"]+)['`"]")
$staging = ""
if ($mStaging.Success) { $staging = $mStaging.Groups[1].Value }

# Guardia: questo script è per lo sviluppo. Un database che si chiama come la produzione non si
# azzera con un doppio clic.
if ($database -match 'prod') { throw "il database si chiama '$database': questo script è solo per lo sviluppo" }

# Guardia: con il server o un worker in esecuzione, ricreare lo schema sotto di loro produce errori
# incomprensibili invece di una riga di partenza pulita.
$vivi = @()
$vivi += @(Get-Process cockpit -ErrorAction SilentlyContinue | ForEach-Object { "cockpit.exe (PID $($_.Id))" })
$vivi += @(Get-CimInstance Win32_Process -Filter "Name = 'python.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -match 'worker_(outlook|analisi)\.py' } |
    ForEach-Object { "$($_.Name) (PID $($_.ProcessId))" })
if ($vivi.Count -gt 0) {
    throw "fermare prima server e worker (scripts\ferma-dev.ps1): in esecuzione $($vivi -join ', ')"
}

if (-not $Psql) {
    $candidati = @(
        (Join-Path $env:LOCALAPPDATA "cockpit_rfq_test\pg\pgsql\bin\psql.exe"),
        "C:\Program Files\PostgreSQL\18\bin\psql.exe"
    )
    $Psql = $candidati | Where-Object { Test-Path $_ } | Select-Object -First 1
    if (-not $Psql) {
        $cmd = Get-Command psql -ErrorAction SilentlyContinue
        if ($cmd) { $Psql = $cmd.Source }
    }
}
if (-not $Psql -or -not (Test-Path $Psql)) { throw "psql.exe non trovato: indicarlo con -Psql" }

$log = ""
if ($staging) { $log = Join-Path $staging "log" }

Write-Host "Database : $mascherato" -ForegroundColor Cyan
Write-Host "Schema   : DROP SCHEMA public CASCADE; CREATE SCHEMA public;" -ForegroundColor Cyan
if ($log) { Write-Host "Log      : $log\*.log*" -ForegroundColor Cyan }
if ($AncheAllegati -and $staging) { Write-Host "Allegati : tutto il contenuto di $staging tranne log" -ForegroundColor Cyan }

if (-not $Conferma) {
    Write-Host ""
    Write-Host "Nessuna modifica: rilanciare con -Conferma per eseguire davvero." -ForegroundColor Yellow
    return
}

Write-Host ""
Write-Host "== schema" -ForegroundColor Cyan
# -d $dsn e NON il DSN come primo argomento: psql prende il primo positional come nome del database e
# poi IGNORA le opzioni che seguono, avvisando su stderr e uscendo con codice 0. Il 16/09/2026 questo
# script ha detto «schema public ricreato» senza aver eseguito niente, e le 43 tabelle erano ancora
# lì. Uno script di azzeramento che riesce senza azzerare è peggio di uno che non c'è: da qui anche
# la verifica qui sotto.
& $Psql -v ON_ERROR_STOP=1 -q -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" -d $dsn
if ($LASTEXITCODE -ne 0) { throw "psql ha restituito ${LASTEXITCODE}: schema NON azzerato" }

$rimaste = (& $Psql -t -A -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'" -d $dsn)
if ($LASTEXITCODE -ne 0) { throw "psql ha restituito ${LASTEXITCODE}: impossibile verificare l'azzeramento" }
if ([int]$rimaste -ne 0) { throw "lo schema public contiene ancora $rimaste tabelle: azzeramento NON riuscito" }
Write-Host "schema public ricreato su ${database}: 0 tabelle"

if ($log -and (Test-Path $log)) {
    Write-Host "== log" -ForegroundColor Cyan
    Get-ChildItem -Path $log -Filter "*.log*" -File -ErrorAction SilentlyContinue | ForEach-Object {
        Remove-Item $_.FullName -Force
        Write-Host "  rimosso $($_.Name)"
    }
}

if ($AncheAllegati -and $staging -and (Test-Path $staging)) {
    Write-Host "== allegati in staging" -ForegroundColor Cyan
    Get-ChildItem -Path $staging -Force | Where-Object { $_.Name -ne "log" } | ForEach-Object {
        Remove-Item $_.FullName -Recurse -Force
        Write-Host "  rimosso $($_.Name)"
    }
}

Write-Host ""
Write-Host "Fatto. Al prossimo avvio cockpit.exe riapplica le migrazioni e risemina le fondazioni" -ForegroundColor Green
Write-Host "(caselle, postazioni, worker, utenti) da $Config. Poi si riavvia il worker." -ForegroundColor Green
