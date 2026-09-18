# Carica le due anagrafiche nel database di SVILUPPO: clienti + domini + buyer, e fornitori.
#
#   powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1            # chiede conferma prima di scrivere i fornitori
#   powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1 -Conferma  # non chiede niente
#   powershell -ExecutionPolicy Bypass -File scripts\semina-anagrafiche.ps1 -SoloAnteprima
#
# PERCHE' ESISTE (checkpoint 7B.5)
#
# Dopo `azzera-dati.ps1` il database ha lo schema e le fondazioni (caselle, postazioni, worker,
# utenti) ma NON ha clienti ne' fornitori: il seme e' un file che sta fuori dal repository, e finche'
# nessuno lo carica ogni mail arriva da uno sconosciuto. Sul banco reale si vedeva Buyer 0,
# Fornitori 0, Da validare 310 — non un difetto del triage, un bootstrap mai fatto.
#
# CHE COSA FA E CHE COSA NON FA
#
# Qui dentro non c'e' nessun lettore di JSON e nessuna riga di SQL: il file lo legge, lo convalida e
# lo scrive `cockpit.exe`, con lo stesso codice della schermata Admin > Anagrafica > Fornitori >
# Importa. PowerShell decide l'ORDINE e chiede la conferma; Go decide che cosa e' valido.
#
# I fornitori si scrivono solo dopo aver mostrato l'ANTEPRIMA: il file e' la fotografia di un foglio
# scritto a mano, e cio' che non si riesce a risolvere (una lavorazione che non esiste, un cliente
# che in anagrafica non c'e') resta fuori e viene detto.
#
# Rilanciarlo non raddoppia niente: un cliente o un fornitore gia' presente viene lasciato com'e',
# nemmeno un campo toccato.
param(
    [string]$Config = "cockpit.toml",
    [string]$Clienti = "..\docs\seme_anagrafica.json",
    [string]$Fornitori = "..\docs\seme_fornitori.json",
    [switch]$SoloAnteprima,
    [switch]$Conferma,
    [switch]$SaltaMigrazioni,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$radice = Split-Path -Parent $PSScriptRoot
Set-Location $radice

function Percorso([string]$p) {
    if ([System.IO.Path]::IsPathRooted($p)) { return $p }
    return [System.IO.Path]::GetFullPath((Join-Path $radice $p))
}

$percorsoConfig = Percorso $Config
if (-not (Test-Path $percorsoConfig)) { throw "configurazione non trovata: $percorsoConfig" }

# 1. i due file devono esistere PRIMA di toccare il database: meta' anagrafica e' peggio di niente,
#    perche' sembra caricata.
$fileClienti = Percorso $Clienti
$fileFornitori = Percorso $Fornitori
$mancanti = @()
if (-not (Test-Path $fileClienti)) { $mancanti += $fileClienti }
if (-not (Test-Path $fileFornitori)) { $mancanti += $fileFornitori }
if ($mancanti.Count -gt 0) {
    throw "seme non trovato: $($mancanti -join ', '). I file delle anagrafiche stanno fuori dal repository (docs\): indicarli con -Clienti e -Fornitori."
}

# 2. dire dove si sta per scrivere, senza la password.
$testo = Get-Content -Raw $percorsoConfig
$mDsn = [regex]::Match($testo, '(?m)^\s*dsn\s*=\s*"([^"]+)"')
if (-not $mDsn.Success) { throw "[db].dsn non trovato in $Config" }
$dsn = $mDsn.Groups[1].Value
$mascherato = [regex]::Replace($dsn, '://[^@/]*@', '://***@')
$database = ([regex]::Match($dsn, '/([^/?]+)(\?|$)')).Groups[1].Value
if ($database -match 'prod') { throw "il database si chiama '$database': questo script e' solo per lo sviluppo" }

Write-Host "Configurazione : $percorsoConfig" -ForegroundColor Cyan
Write-Host "Database       : $mascherato" -ForegroundColor Cyan
Write-Host "Clienti        : $fileClienti" -ForegroundColor Cyan
Write-Host "Fornitori      : $fileFornitori" -ForegroundColor Cyan
Write-Host ""

# Si ricompila SEMPRE, salvo -NoBuild. Il 18/09/2026 questo script ha caricato i clienti e poi si e'
# fermato su "flag provided but not defined: -anteprima-fornitori": l'eseguibile sul disco era di
# due settimane prima. Un bootstrap che gira meta' con il codice nuovo e meta' con quello vecchio e'
# peggio di uno che non parte.
$exe = Join-Path $radice "cockpit.exe"
if ($NoBuild) {
    if (-not (Test-Path $exe)) { throw "cockpit.exe non c'e' e -NoBuild vieta di compilarlo" }
} else {
    Write-Host "== go build" -ForegroundColor Cyan
    go build -o cockpit.exe .\cmd\cockpit
    if ($LASTEXITCODE -ne 0) { throw "compilazione fallita" }
}

# 3. migrazioni: senza tabelle non si semina niente. `-migra` applica ed esce.
if (-not $SaltaMigrazioni) {
    Write-Host "== migrazioni e fondazioni" -ForegroundColor Cyan
    & $exe -config $Config -migra
    if ($LASTEXITCODE -ne 0) { throw "migrazioni fallite (codice $LASTEXITCODE)" }
}

# 4. clienti, domini, buyer. Un file invalido fa uscire il comando con codice != 0 e questo script
#    si ferma qui: non ha senso caricare i fornitori su un'anagrafica clienti che non c'e'.
Write-Host ""
Write-Host "== clienti, domini, buyer" -ForegroundColor Cyan
if ($SoloAnteprima) {
    Write-Host "  (anteprima: il seme dei clienti non ha una modalita' di sola lettura, quindi qui non si scrive niente)" -ForegroundColor DarkGray
} else {
    & $exe -config $Config -semina-anagrafica $fileClienti
    if ($LASTEXITCODE -ne 0) { throw "seme dei clienti rifiutato (codice $LASTEXITCODE): niente e' stato scritto" }
}

# 5. fornitori: prima l'anteprima, sempre.
Write-Host ""
Write-Host "== fornitori: anteprima (non scrive niente)" -ForegroundColor Cyan
& $exe -config $Config -anteprima-fornitori $fileFornitori
if ($LASTEXITCODE -ne 0) { throw "seme dei fornitori rifiutato (codice $LASTEXITCODE): niente e' stato scritto" }

$scrivo = $false
if (-not $SoloAnteprima) {
    if ($Conferma) {
        $scrivo = $true
    } else {
        Write-Host ""
        $risposta = Read-Host "Applicare l'import dei fornitori? (scrivi SI)"
        $scrivo = ($risposta -eq "SI")
        if (-not $scrivo) { Write-Host "Non applicato: i clienti sono stati caricati, i fornitori no." -ForegroundColor Yellow }
    }
}

if ($scrivo) {
    Write-Host ""
    Write-Host "== fornitori: applico" -ForegroundColor Cyan
    & $exe -config $Config -importa-fornitori $fileFornitori
    if ($LASTEXITCODE -ne 0) { throw "import dei fornitori fallito (codice $LASTEXITCODE)" }
}

# 6. i numeri finali. Li conta Go, qui si mostrano e basta.
Write-Host ""
Write-Host "== anagrafica in ${database}" -ForegroundColor Cyan
$conteggi = & $exe -config $Config -conta-anagrafiche
if ($LASTEXITCODE -ne 0) { throw "conteggio fallito (codice $LASTEXITCODE)" }
$etichette = @{
    clienti               = "clienti"
    domini_cliente        = "domini cliente"
    buyer                 = "buyer"
    fornitori             = "fornitori"
    domini_fornitore      = "domini fornitore"
    contatti_fornitore    = "contatti fornitore"
    lavorazioni_fornitore = "lavorazioni dei fornitori"
    qualifiche            = "qualifiche cliente/fornitore"
}
$vuoti = @()
# Il comando scrive anche le righe di log del server (stesso stdout): si tengono solo le voci del
# conteggio, che hanno la forma nome=numero.
foreach ($riga in ($conteggi | Where-Object { $_ -match '^[a-z_]+=[0-9]+$' })) {
    $k, $v = $riga -split "=", 2
    $nome = $etichette[$k]
    if (-not $nome) { $nome = $k }
    $colore = "Gray"
    if ([int]$v -eq 0) { $colore = "Yellow"; $vuoti += $nome }
    Write-Host ("  {0,-30} {1,6}" -f $nome, $v) -ForegroundColor $colore
}

if ($vuoti.Count -gt 0) {
    Write-Host ""
    Write-Warning "A zero: $($vuoti -join ', ')."
    Write-Host "Un fornitore senza domini e senza contatti non fa cambiare quadrante a nessuna mail:" -ForegroundColor Yellow
    Write-Host "la posta si riconosce dall'indirizzo, non dalla ragione sociale. Si censiscono" -ForegroundColor Yellow
    Write-Host "dall'Inbox con «Censisci come fornitore», oppure si aggiungono i domini al seme." -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Fatto. I messaggi gia' arrivati e non ancora decisi sono stati riguardati:" -ForegroundColor Green
Write-Host "cerca «messaggi gia' arrivati riguardati» nelle righe qui sopra." -ForegroundColor Green
