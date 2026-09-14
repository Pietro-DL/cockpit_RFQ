# Ambiente di test isolato del Cockpit (piano di correzione, voce 0.5).
#
# Un cluster PostgreSQL tutto suo, sotto %LOCALAPPDATA%, su una porta diversa da quella di sviluppo:
# i test possono distruggere e ricreare lo schema senza avvicinarsi ai dati di lavoro. Il database si
# chiama cockpit_test e il nome è un vincolo: internal/testutil rifiuta un DSN il cui database non
# contiene "test".
#
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Installa   # scarica i binari (una volta)
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Avvia
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Stato
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Ferma
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Ricrea     # svuota il database
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Dsn        # stampa il DSN e basta
#   powershell -ExecutionPolicy Bypass -File scripts\db-test.ps1 -Versione   # stampa la versione di PostgreSQL
#
# I binari sono quelli "senza installazione" di EnterpriseDB: non toccano il registro, non creano
# servizi e non interferiscono con un PostgreSQL già installato sul PC.
param(
    [switch]$Installa,
    [switch]$Avvia,
    [switch]$Ferma,
    [switch]$Stato,
    [switch]$Ricrea,
    [switch]$Dsn,
    [switch]$Versione,
    [int]$Porta = 5433,
    [string]$VersionePg = "16.10-1"
)

$ErrorActionPreference = "Stop"
$radice   = Join-Path $env:LOCALAPPDATA "cockpit_rfq_test"
$binari   = Join-Path $radice "pg\pgsql\bin"
$cluster  = Join-Path $radice "cluster"
$logFile  = Join-Path $radice "log\postgres.log"
$indirizzoDsn = "postgres://cockpit_test:cockpit_test@127.0.0.1:$Porta/cockpit_test"

function Assicura-Binari {
    if (Test-Path (Join-Path $binari "postgres.exe")) { return }
    $zip = Join-Path $radice "dl\postgresql.zip"
    New-Item -ItemType Directory -Force (Split-Path $zip) | Out-Null
    $url = "https://get.enterprisedb.com/postgresql/postgresql-$VersionePg-windows-x64-binaries.zip"
    Write-Host "scarico PostgreSQL $VersionePg (~320 MB, una volta sola)" -ForegroundColor Cyan
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
    Expand-Archive -Path $zip -DestinationPath (Join-Path $radice "pg") -Force
    Remove-Item $zip -Force
}

function Assicura-Cluster {
    if (Test-Path (Join-Path $cluster "PG_VERSION")) { return }
    New-Item -ItemType Directory -Force $cluster, (Split-Path $logFile) | Out-Null
    $pw = Join-Path $radice "pw.txt"
    Set-Content -Path $pw -Value "cockpit_test" -NoNewline -Encoding ascii
    & "$binari\initdb.exe" -D $cluster -U postgres --auth-local=trust --auth-host=scram-sha-256 --pwfile=$pw -E UTF8 --locale=C | Out-Null
    Remove-Item $pw -Force
    Write-Host "cluster creato in $cluster" -ForegroundColor Green
}

function In-Esecuzione {
    if (-not (Test-Path (Join-Path $cluster "PG_VERSION"))) { return $false }
    & "$binari\pg_ctl.exe" -D $cluster status *> $null
    return ($LASTEXITCODE -eq 0)
}

function Avvia-Cluster {
    Assicura-Binari; Assicura-Cluster
    if (In-Esecuzione) { Write-Host "già in esecuzione sulla porta $Porta" -ForegroundColor Yellow }
    else {
        # fsync spento: è un cluster usa e getta, ci interessa la velocità dei test, non la durabilità
        $opzioni = "-p $Porta -c listen_addresses=127.0.0.1 -c fsync=off -c synchronous_commit=off -c full_page_writes=off"
        & "$binari\pg_ctl.exe" -D $cluster -l $logFile -o $opzioni -w start | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "avvio fallito: vedi $logFile" }
        Write-Host "avviato sulla porta $Porta" -ForegroundColor Green
    }
    Assicura-Database
}

function Assicura-Database {
    $env:PGPASSWORD = "cockpit_test"
    $esiste = & "$binari\psql.exe" -h 127.0.0.1 -p $Porta -U postgres -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'cockpit_test'"
    if ($esiste -ne "1") {
        & "$binari\psql.exe" -h 127.0.0.1 -p $Porta -U postgres -d postgres -v ON_ERROR_STOP=1 -q `
            -c "DO `$`$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='cockpit_test') THEN CREATE ROLE cockpit_test LOGIN PASSWORD 'cockpit_test' CREATEDB; END IF; END `$`$;" `
            -c "CREATE DATABASE cockpit_test OWNER cockpit_test;"
        Write-Host "database cockpit_test creato" -ForegroundColor Green
    }
}

function Ricrea-Database {
    if (-not (In-Esecuzione)) { Avvia-Cluster }
    $env:PGPASSWORD = "cockpit_test"
    & "$binari\psql.exe" -h 127.0.0.1 -p $Porta -U cockpit_test -d cockpit_test -v ON_ERROR_STOP=1 -q `
        -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
    Write-Host "schema svuotato: le migrazioni ripartono da zero al prossimo test" -ForegroundColor Green
}

if ($Dsn)      { Write-Output $indirizzoDsn; exit 0 }
if ($Versione) {
    # Serve al registro degli esiti: la versione va scritta, non ricordata a memoria.
    if (Test-Path (Join-Path $binari "postgres.exe")) { & "$binari\postgres.exe" --version }
    else { Write-Output "PostgreSQL non installato in $binari (usare -Installa)" }
    exit 0
}
if ($Installa) { Assicura-Binari; Assicura-Cluster; exit 0 }
if ($Ferma)    { if (In-Esecuzione) { & "$binari\pg_ctl.exe" -D $cluster -m fast -w stop | Out-Null; Write-Host "fermato" -ForegroundColor Green } else { Write-Host "non era in esecuzione" }; exit 0 }
if ($Ricrea)   { Ricrea-Database; exit 0 }
if ($Stato) {
    if (In-Esecuzione) { Write-Host "in esecuzione sulla porta $Porta" -ForegroundColor Green; Write-Host "DSN: $indirizzoDsn" }
    else { Write-Host "fermo" -ForegroundColor Yellow }
    exit 0
}
if ($Avvia) {
    Avvia-Cluster
    Write-Host ""
    Write-Host "DSN: $indirizzoDsn"
    Write-Host 'Per i test:  $env:COCKPIT_TEST_DSN = "' -NoNewline; Write-Host "$indirizzoDsn`""
    exit 0
}

Write-Host "uso: db-test.ps1 -Installa | -Avvia | -Ferma | -Stato | -Ricrea | -Dsn | -Versione"
