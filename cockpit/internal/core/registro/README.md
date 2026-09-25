# `core/registro/` — lo schema delle regole di un cliente e i semi dell'anagrafica

Tre package: `regole` (lo schema di `cliente.regole` e delle convenzioni di codice, puro), `anagrafica`
(il seme dei clienti e il precompilato del buyer), `fornitori` (l'import dei fornitori con anteprima).
Le schermate che scrivono l'anagrafica a mano stanno in `transport/web` (`anagrafica*.go`,
`fornitori_admin.go`, `convenzioni_admin.go`, `censisci.go`) e usano questi package.

| Package | Che cosa fa | DB |
|---|---|---|
| [`regole`](#registroregole) | le due porte (scrittura che rifiuta, lettura che segna ✓/✗) per le regole del cliente e per le convenzioni codice → lavorazione | no |
| [`anagrafica`](#registroanagrafica) | il seme dei clienti da un file JSON, una volta e senza sovrascrivere; `NomeCognome` | sì |
| [`fornitori`](#registrofornitori) | l'import dei fornitori: `Leggi`, `Calcola` (anteprima), `Applica` (una transazione) | sì |

---

## `registro/regole`

Lo **schema** di ciò che un cliente dichiara di sé (voce 6.11, D17): famiglie di codice, riferimento della
richiesta, frasi del portale, suffissi decorativi e le altre voci di `cliente.regole`; più le convenzioni di
codice → lavorazione (blocco 7A, D39).

### Scopo

Due porte con la **stessa** verifica: `ValidaRegole` rifiuta in scrittura, `LeggiRegole` in lettura non rifiuta
niente e restituisce la diagnosi riga per riga. Lo stesso per le convenzioni: `ValidaConvenzione` e
`LeggiConvenzioni`. Una regola ✗ si vede in schermata e non entra nel motore.

### Non appartiene qui

Il motore che compila le regole e le applica a un testo (`core/inbox/classificazione/regole.go:Compila`),
la lettura e la scrittura del database (i chiamanti), le schermate.

### File

| File | Responsabilità |
|---|---|
| `regole.go` | `Regole`, `FamigliaCodice` (con `RevNelCodice` e `Ruolo` `prodotto`/`parte`), `Riferimento`, `Diagnostica`; `ValidaRegole` (JSON stretto, `DisallowUnknownFields`, un secondo valore dopo l'oggetto rifiutato), `LeggiRegole`, `Regole.Verifica` (il ✓/✗), `CampiRegole` (i nomi ammessi, letti dai tag); `verificaRegex`, `verificaSuffisso` con `codaDaRevisione` |
| `convenzioni.go` | `Convenzione`, `ModoSuffisso` / `ModoRegex`, `MaxSuffisso` (12, lo stesso numero del CHECK `ck_convenzione_suffisso`), `RegexDi`, `VerificaConvenzione`, `ValidaConvenzione`, `LeggiConvenzioni` → `*Convenzioni`, `Convenzioni.Lavorazioni(codice)` → `[]LavorazioneTrovata`, `Convenzioni.N` |

### Entry point

| Funzione | Chiamata da |
|---|---|
| `ValidaRegole` | `transport/web/anagrafica.go:salvaRegole` (riquadro JSON, poi riscrive il JSON normalizzato), `anagrafica_admin.go:scriviRegole` (form strutturato), `registro/anagrafica/seme.go:Leggi` |
| `LeggiRegole` | `core/inbox/ingest/ingest.go` (`Motori.Per`, `Motori.PerFornitore`), `core/rfq/fascicolo/classifica.go:MotoreDellaRfq`, `transport/web/anagrafica.go`, `triage.go`, `transport/workerapi/workerapi.go` — tutti poi chiamano `classificazione.Compila` |
| `Regole.Verifica` | `classificazione.Compila`, per decidere quali famiglie, riferimento, frasi e suffissi entrano nel motore |
| `ValidaConvenzione`, `LeggiConvenzioni`, `Convenzioni.Lavorazioni` | solo `transport/web/convenzioni_admin.go` (creazione, diagnosi, «prova un codice») |

### Dati

Nessun accesso al database. `Convenzione` è la forma di una riga di `convenzione_codice` con le sue
`convenzione_codice_lavorazione`, letta dal chiamante.

### Flussi principali

- **Scrittura delle regole** — testo JSON → `ValidaRegole`: vuoto è valido; campo sconosciuto, tipo sbagliato,
  un secondo valore JSON dopo l'oggetto → errore leggibile; poi il primo ✗ di `Verifica` → errore con il nome della riga.
- **Lettura delle regole** — `LeggiRegole`: JSON illeggibile → `Regole{}` e una diagnosi «JSON» ✗; altrimenti
  tutto il contenuto più `Verifica`.
- **`Verifica`**, riga per riga: ogni regex compila, ha un esempio e l'esempio corrisponde; con `rev_nel_codice`
  la regex ha il gruppo `rev`; `ruolo` vuoto, `prodotto` o `parte`; frasi del portale non vuote e di almeno 4
  caratteri; `lingua_risposta` di due lettere minuscole; `dati_richiesti` non vuoti e ≤ 120 byte;
  `risposta_entro_gg` 1–365; `finestra_aggancio_gg` 1–3650; suffissi decorativi con separatore iniziale
  (`_ - .`), almeno 3 caratteri, senza spazi, ≤ `MaxSuffisso`, e che non si leggono come revisione (`_1`,
  `-B`, `_R2`, `_REV1`); al più 20 famiglie, 20 frasi, 10 suffissi.
- **Convenzioni** — `RegexDi`: un suffisso diventa `(?i)<QuoteMeta>$`, una regex si usa com'è (senza ancore
  aggiunte). `VerificaConvenzione`: compila, esempio presente e corrispondente, controesempio (se c'è) diverso
  dall'esempio e non corrispondente, almeno una lavorazione non vuota. `LeggiConvenzioni` tiene solo le ✓
  attive; una spenta è ✓ con il motivo «spenta». `Lavorazioni` restituisce l'insieme delle lavorazioni di
  tutte le convenzioni che corrispondono, una volta per lavorazione (evidenza della prima), in ordine di codice.

### Invarianti

- La spunta verde e il salvataggio riuscito sono la stessa funzione: `ValidaRegole` usa `Verifica`,
  `ValidaConvenzione` usa `VerificaConvenzione`.
- La porta in lettura non rifiuta mai: le regole rotte restano nella struttura per essere mostrate.
- Una famiglia, un riferimento, una frase o un suffisso ✗ non entrano nel motore (applicato in
  `classificazione.Compila`, che guarda la diagnosi per nome di riga).
- Una convenzione ✗ o spenta non entra in `Convenzioni`.
- Una `finestra_aggancio_gg` fuori da 1–3650 non arriva all'aggancio nemmeno se è in database senza essere
  passata da `ValidaRegole`: `core/inbox/classificazione/regole.go:Motore.Finestra` ricontrolla lo stesso
  intervallo, restituisce 0 e l'aggancio usa la finestra predefinita.

### Dipendenze

Solo libreria standard e `github.com/google/uuid`: nessun package del progetto, come vuole la tabella di
`internal/README.md`. La importano `core/inbox/classificazione`, `core/inbox/ingest`, `core/rfq/fascicolo`,
`core/registro/anagrafica`, `transport/web`, `transport/workerapi`.

### Test

Tutti L1 (`go test ./internal/core/registro/regole/`):

| File | Che cosa prova |
|---|---|
| `regole_test.go` | AN1 (campo inesistente, regex che non compila, esempio mancante o sbagliato, `rev_nel_codice` senza gruppo, lingua, finestra negativa, frase corta, tipo sbagliato, non un oggetto); le regole buone passano intere e reggono il giro Marshal → `ValidaRegole`; vuoto, `{}` e spazi sono validi |
| `suffissi_test.go` | suffissi buoni interi; i rifiuti (vuoto, senza separatore, corto, con spazi, lungo, code da revisione); il limite di 10 |
| `convenzioni_test.go` | CP9 (porta in scrittura), CP10 (porta in lettura: la riga rotta ✗ e non usata, le altre funzionano), CP11 (l'insieme delle lavorazioni con l'evidenza) |

Il motore sulle regole (AN3, AN4) sta in `core/inbox/classificazione`. I vincoli del database sulle convenzioni
(CP13) stanno in `registro/fornitori/seme_db_test.go`.

### Stato dell'implementazione

Completo per D17 e D39; suffissi decorativi dal Fascicolo v3. `canale_atteso`, `lingua_risposta`,
`dati_richiesti`, `risposta_entro_gg`, `richiede_cbd` e `numero_ordine_anticipato` sono dati per l'operatore:
lo schema li controlla e la schermata Anagrafica li mostra, nessun ramo di codice li applica. `Convenzioni.Lavorazioni` oggi serve solo la prova in schermata
(«prova un codice»): nessun flusso — ingest, fascicolo, richieste ai fornitori — la usa ancora.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| un campo nuovo in `cliente.regole` | `regole.go:Regole` e `Verifica`, poi `core/inbox/classificazione/regole.go:Compila` se il motore lo usa |
| cambiare un limite o un messaggio di rifiuto | `regole.go:Verifica`, `verificaRegex`, `verificaSuffisso` |
| capire perché una regola è ✗ | `regole.go:Verifica` (il campo `Motivo`) |
| cambiare che cosa vale come suffisso di convenzione | `convenzioni.go:RegexDi` (e il CHECK della 0014 se cambia `MaxSuffisso`) |

---

## `registro/anagrafica`

Il seme dei clienti (voce 6.6, blocco 3) e il precompilato del buyer.

### Scopo

Leggere e **convalidare tutto** il file dei clienti prima di scrivere una riga; poi scrivere ciò che manca senza
toccare ciò che c'è. `NomeCognome` ricava nome e cognome dal display name (o dall'indirizzo) per il form.

### Non appartiene qui

I dati dei clienti: il file sta fuori dal repository, accanto a `cockpit.toml`. Le schermate dell'anagrafica
(`transport/web/anagrafica*.go`), il ricalcolo dei messaggi dopo il seme (`ingest.RitriageMolti`, chiamato da
`app/runtime/comandi.go`).

### File

| File | Responsabilità |
|---|---|
| `seme.go` | `Seme`, `ClienteSeme`, `BuyerSeme`, `Esito`; `Leggi` / `LeggiFile` (convalida), `Semina` (una transazione), `seminaDomini`, `senzaBOM` |
| `anagrafica.go` | `NomeCognome(display, email)`: toglie le code («- …», «(…)», «<…>», «\| …»), «Cognome, Nome» → invertito, altrimenti l'ultima parola è il cognome; senza display (o con un indirizzo al suo posto) usa la parte locale dell'indirizzo, divisa su `.`, `_`, `-` e con le iniziali maiuscole |

### Entry point

| Funzione | Chiamata da |
|---|---|
| `LeggiFile`, `Semina` | `app/runtime/comandi.go:SeminaAnagrafica` (flag `-semina-anagrafica`), che poi chiama `ingest.RitriageMolti` su `Esito.IndirizziScritti` e `Esito.DominiScritti` |
| `Leggi` | `LeggiFile`, i test |
| `NomeCognome` | `transport/web/triage.go:datiTriage` (nome e cognome precompilati del buyer) |

### Dati

`Semina` apre **una** transazione per tutto il seme (`pool.Begin` … `Commit`), senza lucchetti espliciti.

| Tabella | Letta | Scritta |
|---|---|---|
| `cliente` | `GetClientePerCartella` (esiste già?) | `InsertCliente`: cartella, ragione sociale, lingua, portale, peso, regole (grezze dal file, `{}` se assenti) |
| `dominio_cliente` | `GetClientePerDominio` | `InsertDominioCliente` (in minuscolo) |
| `buyer` | — | `InsertBuyer` con `origine = excel`, email in minuscolo; sul conflitto `(cliente_id, cognome, nome)` aggiorna email e telefono |

### Flussi principali

1. **`Leggi`** — toglie il BOM UTF-8, decodifica con campi sconosciuti vietati, poi: almeno un cliente;
   ragione sociale e cartella NAS obbligatorie; cartella unica nel file; peso 0–15; un dominio non contiene
   `@` e appartiene a un cliente solo nel file; le regole passano da `regole.ValidaRegole`. Un solo errore e
   non parte niente.
2. **`Semina`**, cliente per cliente: se la cartella NAS esiste già, il cliente non si tocca e si aggiungono
   solo i domini mancanti; altrimenti si crea il cliente, poi i domini, poi i buyer (quelli senza cognome si
   saltano; un `tipo` non valido ferma il seme).
3. **Domini** (`seminaDomini`) — un dominio già di un altro cliente non si sposta: diventa un avviso.
4. **`Esito`** — clienti creati e già presenti, domini e buyer scritti, avvisi, e le chiavi (domini, email dei
   buyer) su cui il chiamante ricalcola la posta già arrivata.

### Invarianti

- Nessuna scrittura prima che l'intero file sia convalidato (`Leggi` fa tutto, `Semina` riceve un `Seme` già
  letto).
- Un cliente già presente (per cartella NAS) non viene aggiornato: nemmeno ragione sociale, peso o regole.
- Un dominio già assegnato non cambia cliente.
- Il server non semina mai da solo: `Semina` parte solo dal flag della riga di comando.

### Dipendenze

Importa `core/registro/regole` e `platform/db`. È importato da `app/runtime` (il comando) e da `transport/web`
(`NomeCognome`). Coerente con la tabella di `internal/README.md`.

### Test

| File | Livello | Che cosa prova |
|---|---|---|
| `seme_test.go` | L1 | AN2 (un esempio che non corrisponde ferma il seme e dice quale cliente), il seme buono passa intero, i controlli del file (cartella doppia, dominio a due clienti, indirizzo al posto del dominio, peso, cartella mancante, campo inventato, nessun cliente) |
| `seme_db_test.go` | L4 (`integrazione`) | il seme crea ciò che manca (peso, lingua, regole, email del buyer in minuscolo, origine); rilanciato non sovrascrive le correzioni fatte a mano; non ruba un dominio a un altro cliente |

`NomeCognome` non ha test.

### Stato dell'implementazione

Completo per la voce 6.6. Limiti noti: il campo `note` del cliente si legge dal file ma non si scrive
(`InsertCliente` non ha la colonna); `lingua` del cliente non è controllata in `Leggi` (la colonna è
`char(2)`); i buyer di un cliente già presente non vengono aggiunti.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| un campo nuovo nel file dei clienti | `seme.go:ClienteSeme`, `creaCliente`, la query `InsertCliente` |
| un controllo nuovo sul file | `seme.go:Leggi` |
| capire perché un dominio non è stato scritto | `seme.go:seminaDomini` (l'avviso in `Esito.Avvisi`) |
| cambiare il precompilato di nome e cognome | `anagrafica.go:NomeCognome` |

---

## `registro/fornitori`

L'import dei fornitori, dei loro domini, contatti, lavorazioni e qualifiche per cliente (blocco 7A.4), con
**anteprima** e **conferma**.

### Scopo

Leggere il file, confrontarlo con l'anagrafica e dire che cosa verrebbe creato, che cosa c'è già e che cosa non
si riesce a risolvere; scrivere, solo alla conferma, ciò che è risolto.

### Non appartiene qui

Le schermate dei fornitori e dell'import (`transport/web/fornitori_admin.go`, `web/templates/importa.html`), il
ricalcolo della posta dopo l'import (`ingest.RitriageMolti`, chiamato da chi applica), le convenzioni di codice
(`registro/regole`).

### File

| File | Responsabilità |
|---|---|
| `seme.go` | `Seme`, `FornitoreSeme`, `ContattoSeme`, `QualificaSeme`; `Leggi` / `LeggiFile` (convalida e normalizza); `Riga`, `Anteprima` (con `Vuota`); `piano` e le operazioni tipizzate; `calcola` (il confronto), `Calcola` (anteprima senza scritture), `Applica` (una transazione); `senzaBOM` |

### Entry point

| Funzione | Chiamata da |
|---|---|
| `LeggiFile` + `Calcola` | `app/runtime/comandi.go:SemeFornitori` con `-anteprima-fornitori`, su un pool in sola lettura (`app/runtime/avvio.go:ApriDatabaseInLettura`: niente migrazioni né semi, e lo schema deve essere quello del binario) |
| `LeggiFile` + `Applica` | `app/runtime/comandi.go:SemeFornitori` con `-importa-fornitori` |
| `Leggi` + `Calcola` / `Applica` | `transport/web/fornitori_admin.go:importaFornitori` (`azione=anteprima` / `applica`) |
| `Anteprima.Vuota` | `web/templates/importa.html` (il pulsante «Applica» spento se non c'è niente da scrivere) |

### Dati

| Tabella | Letta (`calcola`) | Scritta (`Applica`) |
|---|---|---|
| `lavorazione` | `ListLavorazioni` | — |
| `fornitore` | `GetFornitorePerRagioneSociale` (senza maiuscole) | `InsertFornitore` |
| `dominio_fornitore` | `GetFornitorePerDominio` | `InsertDominioFornitore` |
| `dominio_cliente` | `GetClientePerDominio` (solo per l'avviso «ambigua») | — |
| `contatto_fornitore` | `ListContattiFornitore` | `InsertContattoFornitore` |
| `fornitore_lavorazione` | `ListLavorazioniFornitore` | `InsertLavorazioneFornitore` (`ON CONFLICT DO NOTHING`) |
| `cliente_fornitore_lavorazione` | `ListQualificheFornitore` | `InsertQualifica` (`ON CONFLICT DO NOTHING`) |
| `cliente` | `GetClientePerCartella` (la qualifica nomina il cliente per cartella NAS) | — |

`Calcola` lavora sul pool, senza transazione. `Applica` apre una transazione, **rifà** `calcola` dentro di
essa e scrive nell'ordine fornitori → domini → contatti → lavorazioni → qualifiche; il primo errore annulla
tutto.

### Flussi principali

1. **`Leggi`** — toglie il BOM, campi sconosciuti vietati, almeno un fornitore; ragione sociale obbligatoria e
   unica nel file (senza maiuscole); `tipo` fra `materie_prime`, `processi`, `verniciatore`; lingua di due
   lettere minuscole; domini con un punto e senza `@`, email con `@`, lavorazioni non vuote, qualifiche con
   cliente e lavorazione. Normalizza: spazi tolti, domini, email e lavorazioni in minuscolo.
2. **`calcola`**, fornitore per fornitore: presente (per ragione sociale) o da creare, con un avviso se il tipo
   del file è diverso; per ogni dominio: suo → presente, di un altro fornitore → non risolto, anche di un
   cliente → avviso e da aggiungere; contatti già suoi → presenti, gli altri da aggiungere; lavorazioni
   inesistenti → non risolte; qualifiche → non risolte se il cliente o la lavorazione non esistono o se il
   fornitore non ha (né riceve dal file) la capacità.
   I doppioni del file si risolvono qui, contro ciò che il piano ha già deciso di scrivere: un dominio già
   «da aggiungere» per un altro fornitore del file → non risolto (resta al primo che lo dichiara); lo stesso
   dominio o la stessa email ripetuti per lo stesso fornitore → un avviso «ripetuto nel file: si scrive una
   volta»; una qualifica ripetuta → la seconda è «presente».
3. **Anteprima** — `FornitoriDaCreare`, `FornitoriPresenti`, `DaAggiungere`, `Presenti`, `NonRisolti`,
   `Avvisi`, e le chiavi `DominiScritti` / `IndirizziScritti` per il ricalcolo della posta.
4. **`Applica`** — scrive il piano e restituisce la stessa anteprima; un secondo import dello stesso file non
   scrive niente.

### Invarianti

- Senza `Applica` non si scrive niente: `Calcola` fa solo letture.
- Un fornitore già presente non viene modificato (un tipo diverso è un avviso).
- Un dominio già di un altro fornitore non si sposta; un dominio dichiarato da due fornitori nello stesso file
  va al primo.
- Ciò che è in `NonRisolti` non viene scritto: una qualifica senza la capacità, una lavorazione inesistente,
  un cliente sconosciuto (il database lo rifiuterebbe comunque con la chiave esterna composta), il dominio
  ripetuto sotto un secondo fornitore.
- Ogni riga «da aggiungere» dell'anteprima compare una volta sola: `Applica` scrive quello che l'anteprima ha
  detto, senza fermarsi su una chiave primaria per un doppione del file.

### Dipendenze

Importa solo `platform/db`. È importato da `app/runtime` e da `transport/web`. Coerente con la tabella di
`internal/README.md`.

### Test

| File | Livello | Che cosa prova |
|---|---|---|
| `seme_db_test.go` | L4 (`integrazione`) | CP7/CP14: l'anteprima non scrive, l'applicazione scrive solo il risolto, il secondo import è vuoto, i non risolti restano tali, un tipo diverso non cambia il fornitore; i rifiuti di `Leggi` e il BOM (test puro, ma sotto il tag); CP13: i vincoli della 0014 (convenzioni, qualifica senza capacità, domini, ragione sociale doppia, controparte del messaggio) e la cascata delle convenzioni |
| `doppioni_db_test.go` | L4 (`integrazione`) | un dominio ripetuto sotto lo stesso fornitore e sotto un secondo, un'email ripetuta: l'anteprima li conta una volta e mette il secondo dominio fra i non risolti, `Applica` non si ferma, il dominio resta al primo fornitore, un contatto solo |

Senza `COCKPIT_TEST_DSN` e senza il tag il package non ha test che girano.

### Stato dell'implementazione

Completo per il 7A.4; il ricalcolo dopo l'import è del 7B.5 (nei chiamanti). Le celle del foglio senza un
significato noto non hanno un campo e non vengono lette.

### Dove intervenire

| Voglio… | Apri |
|---|---|
| un campo nuovo nel file dei fornitori | `seme.go:FornitoreSeme`, `Leggi`, `calcola`, `Applica` |
| capire perché una riga è «non risolta» | `seme.go:calcola` (il `Dettaglio` della riga) |
| capire che cosa scrive la conferma | `seme.go:Applica` |

---

## Leggi anche

`internal/core/README.md`, `internal/README.md` (i lavori amministrativi e il flusso «Censisci»),
`internal/core/inbox/classificazione` (il motore che usa le regole), `internal/transport/README.md` (le
schermate dell'anagrafica), `internal/app/README.md` (`comandi.go`).
