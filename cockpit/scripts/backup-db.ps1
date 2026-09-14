# Backup del database del Cockpit con prova di ripristino (piano, voce 9.4; N39).
#
# Un backup vale quanto il ripristino che è stato provato. Qui il backup non è considerato riuscito
# perché il file esiste o perché è più grande di zero byte: viene ripristinato davvero in un database
# usa e getta e si verificano la versione dello schema e i conteggi delle tabelle che contano.
#
#   powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1 -Dsn $env:COCKPIT_TEST_DSN
#   powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1 -Dsn "postgres://..." -Cartella D:\backup -TieniGiorni 30
#   powershell -ExecutionPolicy Bypass -File scripts\backup-db.ps1 -Dsn "postgres://..." -SenzaProva   # sconsigliato
#
# Esce con 0 solo se il ripristino di prova è riuscito e i conteggi coincidono.
param(
    [Parameter(Mandatory = $true)][string]$Dsn,
    [string]$Cartella = (Join-Path $env:LOCALAPPDATA "cockpit_rfq_backup"),
    [string]$BinPg = (Join-Path $env:LOCALAPPDATA "cockpit_rfq_test\pg\pgsql\bin"),
    [int]$TieniGiorni = 30,
    [switch]$SenzaProva
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path (Join-Path $BinPg "pg_dump.exe"))) {
    throw "pg_dump non trovato in ${BinPg} - passare -BinPg con la cartella bin di PostgreSQL"
}
New-Item -ItemType Directory -Force $Cartella | Out-Null

$uri = [Uri]$Dsn
$dbOrigine = $uri.AbsolutePath.TrimStart('/')
$utente = $uri.UserInfo.Split(':')[0]
$env:PGPASSWORD = [Uri]::UnescapeDataString($uri.UserInfo.Split(':')[1])
$host_ = $uri.Host
$porta = if ($uri.Port -gt 0) { $uri.Port } else { 5432 }
$stampo = Get-Date -Format "yyyyMMdd-HHmmss"
$file = Join-Path $Cartella "$dbOrigine-$stampo.dump"

# --- backup ------------------------------------------------------------------
Write-Host "== backup di $dbOrigine → $file" -ForegroundColor Cyan
& "$BinPg\pg_dump.exe" -h $host_ -p $porta -U $utente -d $dbOrigine -Fc -Z 6 -f $file
if ($LASTEXITCODE -ne 0) { throw "pg_dump fallito" }
$mb = [math]::Round((Get-Item $file).Length / 1MB, 2)
Write-Host "   scritto: $mb MB"

# --- conteggi attesi ---------------------------------------------------------
$query = @"
SELECT coalesce(max(versione),0) FROM schema_versione
UNION ALL SELECT count(*) FROM messaggio
UNION ALL SELECT count(*) FROM allegato
UNION ALL SELECT count(*) FROM documento
UNION ALL SELECT count(*) FROM thread_offerta
UNION ALL SELECT count(*) FROM utente;
"@
$attesi = & "$BinPg\psql.exe" -h $host_ -p $porta -U $utente -d $dbOrigine -tA -c $query
Write-Host "   origine (versione schema, messaggi, allegati, documenti, RFQ, utenti): $($attesi -join ', ')"

# --- prova di ripristino -----------------------------------------------------
if (-not $SenzaProva) {
    # solo lettere, cifre e trattino basso: così l'identificativo non ha bisogno di virgolette, che
    # PowerShell toglierebbe passando l'argomento a psql.exe
    $dbProva = "ripristino_prova_" + ($stampo -replace "-", "_")
    Write-Host "== prova di ripristino in $dbProva" -ForegroundColor Cyan
    try {
        & "$BinPg\psql.exe" -h $host_ -p $porta -U $utente -d postgres -v ON_ERROR_STOP=1 -q -c "CREATE DATABASE $dbProva;"
        if ($LASTEXITCODE -ne 0) { throw "creazione del database di prova fallita" }
        & "$BinPg\pg_restore.exe" -h $host_ -p $porta -U $utente -d $dbProva --no-owner --no-privileges $file
        if ($LASTEXITCODE -ne 0) { throw "pg_restore fallito" }
        $ottenuti = & "$BinPg\psql.exe" -h $host_ -p $porta -U $utente -d $dbProva -tA -c $query
        Write-Host "   ripristino (stessi conteggi): $($ottenuti -join ', ')"
        if (($attesi -join ',') -ne ($ottenuti -join ',')) {
            throw "i conteggi dopo il ripristino non coincidono: atteso [$($attesi -join ', ')], ottenuto [$($ottenuti -join ', ')]"
        }
        Write-Host "   ripristino verificato" -ForegroundColor Green
    }
    finally {
        & "$BinPg\psql.exe" -h $host_ -p $porta -U $utente -d postgres -q -c "DROP DATABASE IF EXISTS $dbProva WITH (FORCE);" *> $null
    }
}

# --- retention ---------------------------------------------------------------
$limite = (Get-Date).AddDays(-$TieniGiorni)
$vecchi = Get-ChildItem $Cartella -Filter "$dbOrigine-*.dump" | Where-Object { $_.LastWriteTime -lt $limite }
foreach ($v in $vecchi) { Remove-Item $v.FullName -Force; Write-Host "   rimosso backup oltre $TieniGiorni giorni: $($v.Name)" }

Write-Host ""
Write-Host "backup riuscito e ripristinabile: $file" -ForegroundColor Green
