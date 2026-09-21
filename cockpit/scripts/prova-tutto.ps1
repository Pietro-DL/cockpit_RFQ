# Esegue le prove automatiche disponibili e scrive il registro degli esiti simulati.
#
#   powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\prova-tutto.ps1 -SenzaDB   # senza L4
#
# Regole di rendicontazione (piano di test §13):
#   - il registro distingue PASSATO, FALLITO, SALTATO, NON ESEGUITO e BLOCCATO DA PREREQUISITO;
#   - un test saltato NON conta come superato: conta come non verificato;
#   - `go build` dimostra che il codice compila. NON dimostra la conformità agli schemi JSON di
#     contracts\ (livello L3): i tipi Go e i modelli pydantic possono compilare benissimo ed essere
#     comunque incoerenti con lo schema. L3 ha ora un test suo, in due metà (Go e Python), ed è quello
#     che decide: senza, un contratto può essere cambiato da una parte sola senza che nulla protesti;
#   - qui finiscono solo gli esiti SIMULATI. Le prove su Outlook, Exchange e su due postazioni
#     (L5-L9) si annotano a mano in docs\esiti\esiti_reali.md e non si deducono mai da un test
#     simulato equivalente.
param([switch]$SenzaDB, [switch]$SenzaPython)

$ErrorActionPreference = "Continue"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

$esiti = @()
function Esegui($id, $descrizione, $comando, $blocco) {
    Write-Host "== $id  $descrizione" -ForegroundColor Cyan
    $uscita = & $blocco 2>&1 | Out-String
    $esito = if ($LASTEXITCODE -eq 0) { "PASSATO" } else { "FALLITO" }
    Write-Host $uscita
    $script:esiti += [pscustomobject]@{ ID = $id; Descrizione = $descrizione; Comando = $comando; Esito = $esito; Uscita = $uscita.Trim() }
}
function Annota($id, $descrizione, $comando, $esito, $nota) {
    $script:esiti += [pscustomobject]@{ ID = $id; Descrizione = $descrizione; Comando = $comando; Esito = $esito; Uscita = $nota }
}

Esegui "L1 go build"  "compilazione di tutti i pacchetti (NON copre L3)" "go build ./..." { go build ./... }
Esegui "L1 go vet"    "analisi statica" "go vet ./..." { go vet ./... }

# Il server va su una VM Linux (D23, blocco 2): che compili per Linux è una condizione, non un
# dettaglio, e si scopre qui invece che sulla VM. NON dimostra che funzioni: il NAS su una share SMB
# montata e il banco a due PC sono prove reali (L8), e stanno in esiti_reali.md.
Esegui "L1 build Linux" "il server compila per la VM Linux (D23)" "GOOS=linux go build ./..." {
    $vecchio = $env:GOOS
    $env:GOOS = "linux"
    go build ./...
    $codice = $LASTEXITCODE
    if ($vecchio) { $env:GOOS = $vecchio } else { Remove-Item Env:\GOOS -ErrorAction SilentlyContinue }
    $global:LASTEXITCODE = $codice
}
Esegui "L1 go test"   "unitari Go (dominio, zip, NAS, template, migrazioni statiche, configurazione, modalita, rete e TLS, testata dell Inbox)" "go test ./..." { go test ./... }

# Gli script di servizio girano su Windows PowerShell 5.1, non sulla 7: un operatore della 7
# (per esempio ?.) rende il file illeggibile gia in fase di parsing. Qui si controlla che
# ognuno sia almeno analizzabile dalla versione installata.
Esegui "L1 script PS" "sintassi degli script di servizio sulla PowerShell installata" "Parser::ParseFile su scripts\*.ps1" {
    $problemi = 0
    foreach ($f in Get-ChildItem "$PSScriptRoot\*.ps1") {
        $errori = $null
        [void][System.Management.Automation.Language.Parser]::ParseFile($f.FullName, [ref]$null, [ref]$errori)
        if ($errori) {
            $problemi += $errori.Count
            foreach ($e in $errori) { Write-Output ("{0}: {1} (riga {2})" -f $f.Name, $e.Message, $e.Extent.StartLineNumber) }
        } else { Write-Output "$($f.Name): sintassi ok" }
    }
    $global:LASTEXITCODE = if ($problemi -gt 0) { 1 } else { 0 }
}

Esegui "L3 contratti (Go)" "i tipi Go corrispondono agli schemi di contracts (K1, K2, K3, K4)" "go test ./internal/platform/contratti/api" { go test -count=1 ./internal/platform/contratti/api/ }

if (-not $SenzaPython) {
    # La metà Python verifica la premessa dell'altra: che gli schemi su disco descrivano i modelli
    # pydantic di oggi. Senza, il confronto Go girerebbe contro uno schema vecchio e sarebbe verde
    # proprio mentre le due parti si allontanano.
    Esegui "L3 contratti (Python)" "rigenerare gli schemi non cambia nessun file" "python -m pytest -q workers/tests/test_contratti.py" { python -m pytest -q workers/tests/test_contratti.py }
} else {
    Annota "L3 contratti (Python)" "rigenerare gli schemi non cambia nessun file" "python -m pytest -q workers/tests/test_contratti.py" "SALTATO" "richiesto -SenzaPython: non verificato"
}

if (-not $SenzaDB) {
    $dsn = & "$PSScriptRoot\db-test.ps1" -Dsn
    & "$PSScriptRoot\db-test.ps1" -Avvia | Out-Null
    $env:COCKPIT_TEST_DSN = $dsn
    # Dentro L4 c'e' anche la prova end-to-end, che avvia il worker VERO: e' Python, e
    # senza interprete non si puo' eseguire. Saltarla in silenzio la farebbe sparire dentro un
    # "L4 integrazione: PASSATO", quindi si annota a parte come non verificata.
    $env:COCKPIT_TEST_SENZA_PYTHON = ""
    if ($SenzaPython) {
        $env:COCKPIT_TEST_SENZA_PYTHON = "1"
        Annota "L4 E2E worker" "il client vero (worker Python) contro il server vero" `
            "go test -tags integrazione -run TestE2E ./internal/transport/workerapi" "SALTATO" `
            "richiesto -SenzaPython: non verificato"
    }
    Esegui "L4 integrazione" "test su PostgreSQL di test, pacchetti in serie (E2E, guardie sul futuro, W4/W10/W11, PK1)" "go test -tags integrazione -count=1 -p 1 ./..." { go test -tags integrazione -count=1 -p 1 ./... }
} else {
    Annota "L4 integrazione" "test su PostgreSQL di test" "go test -tags integrazione -count=1 -p 1 ./..." "SALTATO" "richiesto -SenzaDB: non verificato"
}

if (-not $SenzaPython) {
    Esegui "L2 pytest" "unitari Python (modulo comune, ciclo dei worker, analisi, finestra del sync, ora di Outlook, credenziali e impronta)" "python -m pytest -q workers" { python -m pytest -q workers }
} else {
    Annota "L2 pytest" "unitari Python" "python -m pytest -q workers" "SALTATO" "richiesto -SenzaPython: non verificato"
}

Annota "L5-L9 reali" "Outlook via COM, Exchange, due postazioni, shadow" "-" "NON ESEGUITO" `
    "Fuori dal perimetro di questo script: si annotano a mano in docs\esiti\esiti_reali.md."

# ------------------------------------------------------------------ versioni degli strumenti
function Versione($etichetta, $blocco) {
    # PowerShell 5.1 non ammette try/catch come espressione: serve la forma estesa.
    $v = "non rilevata"
    try {
        $righe = @(& $blocco 2>&1 | ForEach-Object { "$_" } | Where-Object { $_.Trim() })
        if ($righe.Count -gt 0) { $v = $righe[0].Trim() }
    } catch { $v = "non rilevata" }
    [pscustomobject]@{ Strumento = $etichetta; Versione = $v }
}

$versioni = @(
    Versione "Go"         { go version }
    Versione "Python"     { python --version }
    Versione "sqlc"       { $e = Get-Command sqlc -ErrorAction SilentlyContinue
                            if ($e) { & $e.Source version }
                            else {
                                $alt = Join-Path $env:LOCALAPPDATA "cockpit_rfq_test\sqlc\sqlc.exe"
                                if (Test-Path $alt) { & $alt version } else { "non installato" }
                            } }
    Versione "PostgreSQL" { & "$PSScriptRoot\db-test.ps1" -Versione }
    [pscustomobject]@{ Strumento = "Sistema"; Versione = "$([Environment]::OSVersion.VersionString) su $env:COMPUTERNAME" }
)

# ------------------------------------------------------------------ registro
$cartella = Join-Path (Split-Path -Parent $radice) "docs\esiti"
New-Item -ItemType Directory -Force $cartella | Out-Null
$file = Join-Path $cartella "esiti_simulati.md"
$sha    = (git rev-parse HEAD 2>$null)
$ramo   = (git rev-parse --abbrev-ref HEAD 2>$null)
$sporco = (git status --porcelain 2>$null | Out-String).Trim()
$stato  = if ($sporco) { "**con modifiche non committate**" } else { "pulito" }

$righe = @(
    "# Esiti simulati (prove automatiche)",
    "",
    "Generato da ``scripts\prova-tutto.ps1`` il $(Get-Date -Format 'dd/MM/yyyy HH:mm').",
    "",
    "| | |",
    "|---|---|",
    "| Commit | ``$sha`` |",
    "| Ramo | ``$ramo`` |",
    "| Albero di lavoro | $stato |",
    "| Copia verificata | ``$radice`` |",
    ""
)
$righe += @("| Strumento | Versione |", "|---|---|")
foreach ($v in $versioni) { $righe += "| $($v.Strumento) | $($v.Versione) |" }
$righe += @("", "| Prova | Comando | Che cosa copre | Esito |", "|---|---|---|---|")
foreach ($e in $esiti) { $righe += "| $($e.ID) | ``$($e.Comando)`` | $($e.Descrizione) | **$($e.Esito)** |" }
$righe += @("", "Dettaglio:", "")
foreach ($e in $esiti) { $righe += @("### $($e.ID) - $($e.Esito)", "", '```', $e.Uscita, '```', "") }
$righe += @(
    "> Un esito SALTATO o NON ESEGUITO non è un esito positivo: è una verifica che non è stata fatta.",
    "> I test **(EXCH)** e **(2PC)** non compaiono qui: vivono solo in ``esiti_reali.md`` e non si",
    "> danno per superati sulla base di un test simulato equivalente."
)
Set-Content -Path $file -Value $righe -Encoding utf8

$falliti = @($esiti | Where-Object { $_.Esito -eq "FALLITO" }).Count
$aperti  = @($esiti | Where-Object { $_.Esito -in @("SALTATO", "NON ESEGUITO") }).Count
Write-Host ""
Write-Host "registro: $file" -ForegroundColor Green
if ($falliti -gt 0) { Write-Host "$falliti prove FALLITE" -ForegroundColor Red; exit 1 }
if ($aperti -gt 0) { Write-Host "$aperti voci non verificate (saltate o non eseguite): non contano come superate" -ForegroundColor Yellow }
Write-Host "tutte le prove eseguite sono passate" -ForegroundColor Green
