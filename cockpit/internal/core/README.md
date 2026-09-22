# `core/` — le regole del mestiere

## Scopo

Che cosa significa una mail, a chi appartiene, che cosa propone. Senza HTTP, senza TLS, senza lettura della
configurazione. Il database lo toccano solo i package che lo dichiarano qui sotto, e solo per leggere fatti e
scrivere proposte.

## Non appartiene qui

Handler e template, lettura del `cockpit.toml`, client verso servizi esterni, loop di processo, scritture sul
NAS (quelle passano da `platform/storage/nas`).

## Package posseduti

| Package | Che cosa fa | DB |
|---|---|---|
| `inbox/classificazione` | l'**interpretazione** pura: `codici.go` (famiglie del cliente, riferimento RFQ, triage con precedenza risposta > candidati > nuova RFQ), `atto.go` (7C.0: l'atto business e il legame operativo), `catena.go` (taglio della catena di risposta), `regole.go` (il **motore**: compila le regole del cliente e le applica a un testo), `controparte.go` (7A/D33: il resolver su un'interfaccia `Rubrica`; `DominioPubblico`), `proposta.go` (tipo del documento da nome ed estensione), `oggetto.go` (`OggettoPulito`: i prefissi RE:/FW: tolti dall'oggetto — lo usano aggancio e i percorsi), `aggancio.go` (punteggi R1–R5) | no |
| `inbox/ingest` | un lotto di messaggi → `messaggio`, `messaggio_casella`, `allegato`, `conversazione`, `riferimento_portale`, `proposta_triage`; una transazione per lotto con savepoint; scarti e replay; cursore; staging automatico deciso dal modo del sync (D30); `marcatori.go` (7B); `controparte.go` (7A): la controparte scritta sul messaggio, il ritriage mirato, il ricalcolo a lotti all'avvio | sì |
| `inbox/aggancio` | i **candidati** di aggancio con evidenza (In-Reply-To, conversazione, codici/articoli, buyer), scritti come proposte; `richieste.go` (7B): R0/R1/R3f verso una richiesta a un fornitore, `RichiesteManuali` (RF_oggetto) | sì |
| `rfq/documenti` | il **fascicolo** di una RFQ. `path.go`: i nomi sul NAS (`NomeSicuro`, `CartellaThread`, `PathDocumento`, `NomeFileSicuro`) — percorsi sempre RELATIVI alla radice, con `\` come separatore; il prefisso long-path sta in `platform/storage/nas`. `integrita.go`: il **ricognitore** che confronta `documento` con i file veri e scrive `nas_anomalia`, e `AllineaDocumento` (che su un conflitto rifiuta e basta: non apre l'anomalia, ed è il gesto di un amministratore che sta guardando). `nas_percorso.go`: `PercorsoSulNas`, il percorso composto dal solo database e verificato dentro la radice (relativo per le anomalie, assoluto per aprirlo). `verifica.go`: `VerificaFileAperto`, la verifica di chi sta per SERVIRE quei byte — hash ricalcolato sul file già aperto, anomalia `conflitto` aperta se non corrisponde, `verificato_il` scritto solo quando corrisponde — e `Segnala`, la stessa riga di anomalia che scrive il ricognitore (B8.1). `copia_nas.go`: che cosa SIGNIFICA copiare un documento sul NAS (`CopiaSulNas`) e dove se ne ritrova il contenuto (`SorgenteStaging`); `ripresa.go`: il contenuto sparito dalla cache che si riprende da solo (`RiprendiContenuto`); `cartella_thread.go`: la cartella di una RFQ (`CreaCartellaThread`). Chi le chiama e' l'esecutore, che sta in `app/runtime` | sì |
| `registro/regole` | lo **schema** di ciò che un cliente dichiara di sé: `regole.go` (`cliente.regole`, le due porte in scrittura e in lettura, la diagnosi ✓/✗), `convenzioni.go` (7A/D39: suffisso/regex → lavorazioni con l'evidenza). Non applica niente: lo legge e lo giudica | no |
| `registro/anagrafica` | il seme dei clienti da `seme_anagrafica.json`, una volta e senza sovrascrivere; `anagrafica.go`: `NomeCognome`, il precompilato del buyer dal display name o dall'indirizzo | sì |
| `registro/fornitori` | l'import del seme dei fornitori (7A.4): `Leggi` convalida, `Calcola` fa l'anteprima senza scrivere, `Applica` scrive in una transazione solo ciò che è risolto | sì |

## Dipendenze consentite

`core/registro/regole` non importa nulla del progetto: è lo schema, e uno schema non dipende da chi lo usa.
Nessun package di `platform` importa `core`: la freccia va in un verso solo.
Gli altri package di `core` importano gli altri `core/*` e `platform`. Le due catene che esistono davvero:
`inbox/classificazione` → `registro/regole` (il motore lavora sullo schema) e `rfq/documenti` →
`inbox/classificazione` (il nome della cartella nasce dall'oggetto ripulito).
Mai `transport`, mai `ai`, mai `app`.

## Entry point

`classificazione.Triage`, `classificazione.RisolviControparte`, `classificazione.TagliaCatena`,
`ingest.Servizio.Ingerisci`,
`ingest.Ritriage` / `RitriageMolti` / `RicalcolaControparti`, `aggancio.CalcolaESalva`,
`aggancio.CalcolaRichieste`, `fornitori.Leggi` / `Calcola` / `Applica`,
`regole.ValidaRegole` / `LeggiRegole` / `ValidaConvenzione` / `LeggiConvenzioni`,
`documenti.CartellaThread` / `PathDocumento` / `NomeSicuro` / `Ricognitore.Giro` / `AllineaDocumento` /
`CopiaSulNas` / `CreaCartellaThread` / `PercorsoSulNas` / `VerificaFileAperto` / `Segnala`,
`anagrafica.NomeCognome`.

## Flussi principali

L'ingest di un lotto: casella verificata prima della transazione, modo del sync letto dal **job** e non dal
lotto, identità del messaggio = Message-ID, controparte risolta e scritta, corpo tagliato, triage, proposte e
candidati, cursore nella stessa transazione. Il ritriage mirato tocca solo i messaggi **non decisi**.

## Invarianti

- I tre strati: fatto (che cosa è arrivato, e chi c'è dall'altra parte secondo l'anagrafica di quel momento) ·
  interpretazione (che cosa probabilmente significa, con punteggio ed evidenza) · decisione (che cosa
  l'operatore ha deciso). Un'interpretazione non scrive mai una decisione.
- Un fornitore, un ambiguo o uno sconosciuto **in entrata non produce mai `nuova_rfq`** (7B.3, D34): è per
  costruzione, non per un controllo aggiunto dopo.
- Precedenza del triage: risposta > candidati > nuova RFQ.
- Le regole del cliente marcate ✗ non entrano nel motore.
- Il corpo originale non si modifica: il taglio decide solo che cosa legge l'interpretazione.
- «Da validare» = sconosciuto + ambiguo. La newsletter di un cliente si censisce come Altro.
- Un fornitore senza domini e senza contatti non sposta nessuna mail: la posta si riconosce dall'indirizzo, non
  dalla ragione sociale.

## Effetti collaterali

`ingest` scrive i fatti e le proposte di triage; `aggancio` scrive i candidati; `registro/*` scrive le
anagrafiche; `rfq/documenti` scrive `nas_anomalia`, lo stato NAS di un documento e — in `CopiaSulNas` e
`CreaCartellaThread` — i file sul NAS attraverso `platform/storage/nas`.
Nessuno degli altri scrive `thread_id`, `documento` o sul NAS.

## Test

L1 sugli oracoli di `inbox/classificazione` (codici, atto, catena, controparte, il motore delle regole,
proposta, precedenza) e su `registro/regole` (le due porte, le convenzioni).
L4 per `ingest`, `aggancio`, `registro/fornitori` e `rfq/documenti` (la riconciliazione contro file veri),
compresi gli invarianti I4/I5 del contratto di classificazione.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una regola di triage nuova | `inbox/classificazione/codici.go:Triage`, `Motore.Codici` |
| un campo nuovo in `cliente.regole` | `registro/regole/regole.go`, poi il motore in `inbox/classificazione/regole.go` |
| un atto nuovo | `inbox/classificazione/atto.go` e l'enum `atto_business` in migrazione |
| una convenzione di codice | `registro/regole/convenzioni.go` |
| il nome di una cartella o di un file sul NAS | `rfq/documenti/path.go` |
| capire perché un documento risulta mancante o in conflitto | `rfq/documenti/integrita.go:controllo.esamina` |
| capire perché una copia sul NAS non parte o si ripete | `rfq/documenti/copia_nas.go:CopiaSulNas`, `ripresa.go:RiprendiContenuto` |
| capire perché un fornitore non apre una RFQ | `inbox/classificazione/controparte.go:RisolviControparte`, `inbox/classificazione/codici.go` (ramo `Controparte`) |
| capire perché un'offerta propone quella richiesta | `inbox/aggancio/richieste.go:CalcolaRichieste` |
| capire che cosa scrive (e non scrive) l'import dei fornitori | `registro/fornitori/seme.go:calcola`, `Applica` |

## Leggi anche

`internal/README.md` (i flussi per intero), `platform/README.md` (query e migrazioni),
`transport/README.md` (chi chiama questi entry point).
