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
| `domain` | l'**interpretazione** pura: `codici.go` (famiglie del cliente, riferimento RFQ, triage con precedenza risposta > candidati > nuova RFQ), `atto.go` (7C.0: l'atto business e il legame operativo), `catena.go` (taglio della catena di risposta), `regole.go` (schema di `cliente.regole`, ✓/✗), `controparte.go` (7A/D33: il resolver su un'interfaccia `Rubrica`; `DominioPubblico`), `convenzioni.go` (7A/D39: suffisso/regex → lavorazioni con l'evidenza), `proposta.go` (tipo del documento da nome ed estensione), `path.go` (nome della cartella NAS), `anagrafica.go`, `aggancio.go` (punteggi R1–R5) | no |
| `inbox/ingest` | un lotto di messaggi → `messaggio`, `messaggio_casella`, `allegato`, `conversazione`, `riferimento_portale`, `proposta_triage`; una transazione per lotto con savepoint; scarti e replay; cursore; staging automatico deciso dal modo del sync (D30); `marcatori.go` (7B); `controparte.go` (7A): la controparte scritta sul messaggio, il ritriage mirato, il ricalcolo a lotti all'avvio | sì |
| `inbox/aggancio` | i **candidati** di aggancio con evidenza (In-Reply-To, conversazione, codici/articoli, buyer), scritti come proposte; `richieste.go` (7B): R0/R1/R3f verso una richiesta a un fornitore, `RichiesteManuali` (RF_oggetto) | sì |
| `registro/anagrafica` | il seme dei clienti da `seme_anagrafica.json`, una volta e senza sovrascrivere | sì |
| `registro/fornitori` | l'import del seme dei fornitori (7A.4): `Leggi` convalida, `Calcola` fa l'anteprima senza scrivere, `Applica` scrive in una transazione solo ciò che è risolto | sì |

## Dipendenze consentite

`core/domain` non importa nulla del progetto. Gli altri package di `core` importano `core/domain` e `platform`.
Mai `transport`, mai `ai`, mai `jobs`.

## Entry point

`domain.Triage`, `domain.RisolviControparte`, `domain.TagliaCatena`, `ingest.Servizio.Ingerisci`,
`ingest.Ritriage` / `RitriageMolti` / `RicalcolaControparti`, `aggancio.CalcolaESalva`,
`aggancio.CalcolaRichieste`, `fornitori.Leggi` / `Calcola` / `Applica`.

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
anagrafiche. Nessuno di questi scrive `thread_id`, `documento` o sul NAS.

## Test

L1 sugli oracoli di `domain` (codici, atto, catena, controparte, convenzioni, regole, proposta, precedenza).
L4 per `ingest`, `aggancio` e `registro/fornitori`, compresi gli invarianti I4/I5 del contratto di
classificazione.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| una regola di triage nuova | `domain/codici.go:Triage`, `Motore.Codici` |
| un atto nuovo | `domain/atto.go` e l'enum `atto_business` in migrazione |
| una convenzione di codice | `domain/convenzioni.go` |
| capire perché un fornitore non apre una RFQ | `domain/controparte.go:RisolviControparte`, `domain/codici.go` (ramo `Controparte`) |
| capire perché un'offerta propone quella richiesta | `inbox/aggancio/richieste.go:CalcolaRichieste` |
| capire che cosa scrive (e non scrive) l'import dei fornitori | `registro/fornitori/seme.go:calcola`, `Applica` |

## Leggi anche

`internal/README.md` (i flussi per intero), `platform/README.md` (query e migrazioni),
`transport/README.md` (chi chiama questi entry point).

---

**Cambia in B**: `domain` si divide — `inbox/classificazione` (triage, motore, atto, catena, controparte,
candidati, proposta dell'allegato), `registro/regole`, `rfq/documenti` — e `domain` sparisce come package unico.
