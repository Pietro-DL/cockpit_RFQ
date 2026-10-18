# `internal/core/propostabom` — dalla proposta del Motore A alla bozza della Distinta unica

## Scopo

- Il passaggio dalla proposta del Motore A alla bozza formato 3 della Distinta unica (V1: emendamento di V1 §6.1,
  §6.4.1; piano della 0023 §14 passo 3). È puro: legge l'esito già calcolato della valutazione, non legge il DB, non
  scrive e non decide.
- Oggi:
  - **la chiave dei gruppi di un prodotto** (`GruppoBOM`, `GruppoCandidata`, `GruppoSenzaSTEP`, `ChiaveCandidata`,
    `GruppiDelProdotto`): «bom», «cand:<allegato_id>:<radice>», «senza», con l'allegato, lo sha256 e la radice della
    struttura del gruppo. Prima stava nella pagina (`transport/web/motore_a`, `indice.go`), che ora la richiama: la pagina
    e il registro d'importazione usano la stessa chiave. Una chiave non si scompone mai;
  - **il gruppo corrente della sorgente** di una bozza (`GruppoCorrenteDellaSorgente`, `GruppoCorrenteDelProdotto`; la
    regola di O-01 con il secondo e il terzo giro): fra i gruppi del prodotto della bozza, sulle occorrenze attive
    (`NonAttivi`), per occorrenza, sha256 e radice (`SorgenteBozza`; senza la radice, il ripiego per una chiave
    «cand:…»), in quest'ordine: la candidata della stessa occorrenza, la BOM di lavoro della stessa occorrenza, la BOM di
    lavoro su un'altra occorrenza dello stesso contenuto; altrimenti `assente`, con il motivo `nessun_gruppo`,
    `occorrenza_non_attiva` o `senza_radice`. Lo usano la striscia della pagina, R1 `?confronto=1`, l'avviso
    `PROPOSTA_CAMBIATA` di R4 e `riallinea_proposta`.
- Arrivano con il DTO del registro d'importazione (corsia persistenza): l'adattatore che costruisce il registro
  (`ImportaGruppo`) e il confronto alla riapertura (`Confronta`).

## Non appartiene qui

- Il DTO della bozza e del registro, le query, le rotte: `core/rfq/distintabozza` e il web. `distintabozza` importa
  questo pacchetto, mai il contrario.
- Le regole di lettura del thread (partner attivi, contesto risolto, perimetro, impronta dell'indice):
  `valutazione.RegoleDiLettura`.

## File

- **`gruppi.go`** — commento `// Package`; le chiavi e i tipi dei gruppi, `ChiaveCandidata`, `Gruppo`,
  `GruppiDelProdotto`.
- **`gruppo_corrente.go`** — `SorgenteBozza`, `NonAttivi`, `GruppoCorrente` con i modi (`Come*`) e i motivi
  (`Motivo*`), `GruppoCorrenteDellaSorgente`, `GruppoCorrenteDelProdotto`, il ripiego per chiave.

## Invarianti

- Nessuna chiave si scompone: la regola guarda allegato, sha256 e radice dei gruppi; il ripiego confronta la chiave
  intera.
- Un gruppo di un altro prodotto non vale mai (la chiave non contiene il prodotto); una candidata di un'altra occorrenza
  non vale mai; una candidata su un'occorrenza non attiva non c'è.
- Pura e deterministica: niente DB, file, orologio, `uuid.New`; gli ingressi di chi chiama non cambiano.

## Dipendenze

- **Importa:** `core/valutazione` (il prodotto valutato e le sue candidate), `core/ancoraggio` (le strutture),
  `github.com/google/uuid`. La lista chiusa di `core/estrazione/evidenze/dipendenze_test.go` gli consente anche
  `motorea`, `grammatica`, `evidenze` e `jsoncanonico`, per l'adattatore.
- **È importato da:** `transport/web/motore_a` (la chiave dei gruppi).

## Test

- **`gruppi_test.go`** — L1 — le costanti; `GruppiDelProdotto` (ordine, dati della struttura, la BOM di un altro
  prodotto ignorata); T-09: i quattro esiti e i tre motivi di `assente` con due copie dello stesso STEP, la candidata
  della propria copia prima della BOM sull'altra, il passaggio inverso, le occorrenze non attive, il ripiego per chiave
  (anche con la chiave «bom», che non lo ammette), due prodotti con la stessa radice candidata, nessuna chiave scomposta;
  la forma JSON del gruppo corrente.
- A-09, la parità con `Indice.Gruppi`, in `transport/web/motore_a/indice_gruppi_core_test.go`.

## Leggi anche

- `internal/core/valutazione/README.md`
- `internal/core/ancoraggio/README.md`
- `internal/transport/web/README.md` (la pagina del Motore A, `motore_a`)
