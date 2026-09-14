# Esegue tutte le prove automatiche (L1-L4) e scrive il registro degli esiti simulati.
#
#   powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1 -SenzaDB   # solo L1-L3
#
# Regole di rendicontazione (piano di test §13):
#   - il registro distingue PASSATO, FALLITO, SALTATO e BLOCCATO DA PREREQUISITO;
#   - un test saltato NON conta come superato;
#   - qui finiscono solo gli esiti SIMULATI (L1-L4). Le prove su Outlook, Exchange e su due
#     postazioni (L5-L9) si annotano a mano in docs/esiti/esiti_reali.md e non si deducono mai da
#     un test simulato equivalente.
param([switch]$SenzaDB, [switch]$SenzaPython)

$ErrorActionPreference = "Continue"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

$esiti = @()
function Esegui($id, $descrizione, $blocco) {
    Write-Host "== $id  $descrizione" -ForegroundColor Cyan
    $uscita = & $blocco 2>&1 | Out-String
    $esito = if ($LASTEXITCODE -eq 0) { "PASSATO" } else { "FALLITO" }
    Write-Host $uscita
    $script:esiti += [pscustomobject]@{ ID = $id; Descrizione = $descrizione; Esito = $esito; Uscita = $uscita.Trim() }
}

Esegui "L1/L3 go build" "compilazione di tutti i pacchetti" { go build ./... }
Esegui "L1 go vet" "analisi statica" { go vet ./... }
Esegui "L1 go test" "unitari Go (dominio, zip, NAS, template, migrazioni statiche)" { go test ./... }

if (-not $SenzaDB) {
    $dsn = & "$PSScriptRoot\db-test.ps1" -Dsn
    & "$PSScriptRoot\db-test.ps1" -Avvia | Out-Null
    $env:COCKPIT_TEST_DSN = $dsn
    Esegui "L4 integrazione" "test su PostgreSQL di test, pacchetti in serie ($dsn)" { go test -tags integrazione -count=1 -p 1 ./... }
} else {
    $esiti += [pscustomobject]@{ ID = "L4 integrazione"; Descrizione = "test su PostgreSQL di test"; Esito = "SALTATO"; Uscita = "richiesto -SenzaDB" }
}

if (-not $SenzaPython) {
    Esegui "L2 pytest" "unitari Python (modulo comune, ciclo dei worker, analisi)" { python -m pytest -q workers }
} else {
    $esiti += [pscustomobject]@{ ID = "L2 pytest"; Descrizione = "unitari Python"; Esito = "SALTATO"; Uscita = "richiesto -SenzaPython" }
}

$cartella = Join-Path (Split-Path -Parent $radice) "docs\esiti"
New-Item -ItemType Directory -Force $cartella | Out-Null
$file = Join-Path $cartella "esiti_simulati.md"
$commit = (git rev-parse --short HEAD 2>$null)
$righe = @(
    "# Esiti simulati (L1-L4)",
    "",
    "Generato da ``scripts\prova-tutto.ps1`` il $(Get-Date -Format 'dd/MM/yyyy HH:mm') su $env:COMPUTERNAME, commit ``$commit``.",
    "",
    "| Prova | Che cosa copre | Esito |",
    "|---|---|---|"
)
foreach ($e in $esiti) { $righe += "| $($e.ID) | $($e.Descrizione) | **$($e.Esito)** |" }
$righe += @("", "Dettaglio:", "")
foreach ($e in $esiti) { $righe += @("### $($e.ID) - $($e.Esito)", "", '```', $e.Uscita, '```', "") }
$righe += @(
    "> I test **(EXCH)** e **(2PC)** non compaiono qui: vivono solo in ``esiti_reali.md`` e non si",
    "> danno per superati sulla base di un test simulato equivalente."
)
Set-Content -Path $file -Value $righe -Encoding utf8

$falliti = @($esiti | Where-Object { $_.Esito -eq "FALLITO" }).Count
$saltati = @($esiti | Where-Object { $_.Esito -eq "SALTATO" }).Count
Write-Host ""
Write-Host "registro: $file" -ForegroundColor Green
if ($falliti -gt 0) { Write-Host "$falliti prove FALLITE" -ForegroundColor Red; exit 1 }
if ($saltati -gt 0) { Write-Host "$saltati prove saltate: non contano come superate" -ForegroundColor Yellow }
Write-Host "tutte le prove eseguite sono passate" -ForegroundColor Green
