# `internal/core/inbox/ingest` — dal lotto del worker ai fatti e alle prime proposte

## Scopo

Riceve un lotto di messaggi Outlook già convertiti dal worker e lo scrive in una transazione sola: i
**fatti** (messaggio, copia nella casella, conversazione, allegati, controparte) e le prime
**interpretazioni** (proposte sugli allegati dal nome del file, riferimenti al portale, candidati di aggancio
e di codice, proposta di triage). Tiene gli scarti e il loro replay, fa avanzare il cursore del sync, decide
se gli allegati scendono da soli nello staging (D30), applica i marcatori che il Cockpit stesso ha scritto
sulle bozze (7B) e ricalcola controparte e proposta dei messaggi non decisi quando cambia l'anagrafica (7A).
Scrive solo il lotto che il job del tentativo autorizza: un `sync_outlook` o un `rileggi_elemento` della
stessa casella.

Non decide niente: `messaggio.thread_id` si scrive qui in un caso solo, il marcatore
`CockpitRichiestaFornitore` sulla nostra posta in uscita (`marcatori.go`). Non scrive sul NAS.

## Non appartiene qui

- Il protocollo HTTP, l'autenticazione del worker, la credenziale che autorizza la casella, la verifica del
  tentativo prima della chiamata, la frontiera `coperto_fino_a`: `transport/workerapi` (`ingest`,
  `casellaAutorizzata`, `applicaRisultato`).
- Che cosa significa un testo: resolver della controparte, taglio della catena, triage, atto, estrazione dei
  codici, proposta dal nome del file, `Motore` delle regole del cliente: `core/inbox/classificazione`.
- Il calcolo e il salvataggio dei candidati di aggancio, di codice e verso una richiesta: `core/inbox/aggancio`.
- L'accodamento dei job (`stage_allegato`, `rileggi_elemento`) e la guardia del doppio download:
  `platform/coda`.
- Le decisioni dell'operatore (agganciare, creare la RFQ, ignorare, censire): `transport/web`.

## File

| File | Responsabilità |
|---|---|
| `ingest.go` | Il lotto. `Servizio` (con `PrimaDelCommit`, `StagingAutomatico`, `StagingMaxByte`, `StagingBootstrap`), `Lotto`, `Tentativo`, gli errori `ErrTentativoNonValido`, `ErrCasellaNonCensita`, `ErrLottoNonDelJob`; `CasellaDelJob` (la casella che un job nomina: colonna `casella_id`, altrimenti il payload) e `lottoDelJob` (il job del tentativo può consegnare questo lotto?); `Ingerisci` (transazione + savepoint per elemento + cursore), `uno` (un elemento, nel suo savepoint), `stageAutomatico` (un accodamento D30 in un savepoint annidato) ed `errStagingIrrecuperabile`; `scarta` / `scartaLettura` / `payloadScarto` / `jsonSenzaNul` / `senzaNul` / `senzaNulTesto` (scarti senza byte NUL, nei campi e nel payload), `RisolviCasella`, `Nostri` / `CaricaNostri` / `DirezioneEInterno` (direzione e `interno` dalle caselle censite), `Motori` / `NuoviMotori` / `Per` / `PerFornitore` (cache per lotto dei motori delle regole), `scendonoDaSoli` (staging automatico secondo il modo del job), `sogliaStaging`, `RicevutoIn`, `MaxIdentificativo`, `forzaErrore`. Commento `// Package` |
| `controparte.go` | 7A: `rubricaDB` (la rubrica del resolver sul database), `risolviControparte`, `clienteDallaControparte`, `parametriControparte`; `interpretazione` e `interpreta` (candidati, triage, candidati di codice, `UpsertTriage`), `daInterpretare`, `motorePer`; il ricalcolo: `Ritriage`, `RitriageMolti`, `ritriageChiavi`, `ritriageUno`, `EsitoRitriage`; `RicalcolaControparti` (avvio); `IndirizzoDaCensire` |
| `marcatori.go` | 7B: `MarcatoreBozza`, `MarcatoreRichiesta`, `applicaMarcatori` (solo sulla posta in uscita: richiesta `inviata`, legame messaggio → richiesta, aggancio alla RFQ con riga nel log; bozza partita), `avvisa` (il `Warn` che tollera un log nil) |
| `replay.go` | `Riprova`: uno scarto `ingest` si reingerisce dal payload, uno scarto `lettura` diventa un job `rileggi_elemento` |

## Entry point

| Simbolo | Chi lo chiama |
|---|---|
| `Servizio.Ingerisci`, `Lotto`, `Tentativo`, `ErrTentativoNonValido`, `ErrLottoNonDelJob` | `transport/workerapi/workerapi.go:ingest` (`POST` del lotto; 409 sul tentativo non valido, 403 su `ErrLottoNonDelJob`, 500 su ogni altro errore) |
| `CasellaDelJob` | `transport/workerapi/workerapi.go:ingest`, per un lotto che non dichiara `casella_id`: vale la casella del job, e `[outlook].casella_default` solo se il job non ne nomina nessuna |
| `RisolviCasella`, `ErrCasellaNonCensita` | `transport/workerapi/workerapi.go:ingest`, prima della transazione (422 e job fallito senza nuovi tentativi) |
| `Servizio` (costruzione) | `app/runtime/servizi.go:CostruisciServizi` (con `[staging]` dalla configurazione), passato a `workerapi.Server.Ingest` e `web.Server.Ingest` |
| `Servizio.Riprova` | `transport/web/routes_admin.go:riprovaScarto` (pagina degli scarti) |
| `Servizio.Ritriage` | `transport/web/censisci.go:censisci` (dopo «Censisci»), e `censisci.go:ritriagePer` dopo un dominio o un contatto aggiunto in `anagrafica.go`, `anagrafica_admin.go`, `fornitori_admin.go`; tutti e due con un `Servizio` costruito al volo (solo `Pool` e `Log`) |
| `Servizio.RitriageMolti` | `transport/web/fornitori_admin.go` (import del seme fornitori), `app/runtime/comandi.go:ricalcola` (`-semina-anagrafica`, `-importa-fornitori`) |
| `RicalcolaControparti` | `app/runtime/esegui.go:Esegui`, a ogni avvio dopo il seed |
| `IndirizzoDaCensire` | `transport/web/censisci.go:datiCensisci` |
| `NuoviMotori().PerFornitore` | `transport/workerapi/workerapi.go:motoreDelFile` (i suffissi decorativi tolti dal codice letto in un file di un fornitore, come all'arrivo) |
| `MarcatoreRichiesta` | `transport/web/richieste.go` (payload di `crea_bozza_outlook`); `CockpitBozza` lo scrive il worker su ogni bozza |
| `CaricaNostri`, `Nostri`, `RicevutoIn`, `MaxIdentificativo`, `Motori.Per` | esportati, usati solo dentro il package e dalle sue prove |

## Dati

**Scrive, in `Ingerisci` (una transazione per lotto, un savepoint per elemento):**
`conversazione` (`UpsertConversazione`), `messaggio` (`UpsertMessaggio`, `SetControparteMessaggio`),
`messaggio_outlook`, `messaggio_casella` (`UpsertPresenza`), `allegato`, `documento_proposta`
(`InsertPropostaSeAssente`), `riferimento_portale`, `proposta_triage` (`UpsertTriage`, fonte `deterministico`),
`ingest_scarto` (`UpsertIngestScarto`, `EliminaScartoPerElemento`), `sync_cursore` (`UpsertSyncCursore`).
Attraverso `aggancio`: `candidato_aggancio`, `candidato_codice`, `candidato_richiesta`. Attraverso
`coda.AccodaStage`, ciascuno in un savepoint annidato in quello dell'elemento (`stageAutomatico`): `job`
(`stage_allegato`, chiave `stage:<allegato_id>`) e lo stato di `allegato`.
Dai marcatori, solo su una mail in uscita: `richiesta_fornitore` (`SetRichiestaInviata`),
`messaggio.richiesta_fornitore_id`, `messaggio.thread_id` con `aggancio = 'operatore'` e `agganciato_da` = chi
ha creato la richiesta, `messaggio_aggancio_log`, `bozza` (`SetBozzaInviata`).

**Scrive fuori dal lotto:** `ritriageUno` — una transazione per messaggio — `messaggio` (controparte, `buyer_id`
se mancava), `candidato_aggancio`, `candidato_codice` e `candidato_richiesta` (cancellati e ricalcolati),
`proposta_triage`. `RicalcolaControparti` — senza transazione, a lotti di 500 — solo la controparte.
`Riprova` — `job` `rileggi_elemento` (chiave `rileggi:<casella>:<entry_id>`) oppure un `Ingerisci` intero.

**Legge:** `casella` (`GetCasella`, `GetCasellaPerIndirizzo`, `DominiNostri`: anche le caselle disattivate),
`cliente`, `dominio_cliente`, `buyer`, `fornitore`, `dominio_fornitore`, `contatto_fornitore`, i recapiti
«altro» (7C.0), `richiesta_fornitore` con `thread_offerta` (le regole dei clienti che hanno richieste aperte a
un fornitore), `messaggio` (parent per chiave, `ListMessaggiDaRitriage`, `ListMessaggiSenzaControparte`),
`messaggio_outlook`, `allegato`, `proposta_triage`, `ingest_scarto`, `job` (tipo, `casella_id` e payload del
job del tentativo, per `lottoDelJob` e per il modo dello staging).

**Lucchetti:** `BloccaTentativo` tiene `FOR UPDATE` la riga del job per tutta la transazione del lotto: due
lotti dello stesso job si serializzano, e un tentativo non scade a metà scrittura. Nient'altro si blocca
esplicitamente. Il package non tocca il disco.

## Flussi principali

**Un lotto dal worker.**
1. `workerapi.ingest` verifica worker e tentativo dichiarato, sceglie la casella (quella del lotto; se manca,
   quella del job con `CasellaDelJob`; altrimenti `[outlook].casella_default`) e chiama `RisolviCasella` fuori
   dalla transazione: sconosciuta o disattivata = `ErrCasellaNonCensita`. Poi controlla che la credenziale
   autorizzi quella casella (403, in `workerapi`).
2. `Ingerisci` apre la transazione, legge `Nostri`, prepara `Motori`, blocca il job (`BloccaTentativo`; nessuna
   riga = `ErrTentativoNonValido`) e lo confronta con il lotto (`lottoDelJob`): il job deve essere
   `sync_outlook` o `rileggi_elemento` e, se nomina una casella, la stessa del lotto; altrimenti
   `ErrLottoNonDelJob`, prima di ogni scrittura. Decide lo staging automatico dal **payload del job**, non dal
   lotto (`scendonoDaSoli`: spento → no; nessun job o job diverso dal sync → sì; storico → no; bootstrap → solo
   con `StagingBootstrap`; payload illeggibile → no). Il replay (`Riprova`) passa senza tentativo e salta i due
   controlli sul job.
3. Per ogni elemento: savepoint → `uno`. Un errore (o `forzaErrore`) annulla fino al savepoint e registra lo
   scarto `ingest` con il payload intero; un elemento entrato cancella il suo scarto per (casella, entry_id).
   Ogni campo di testo dello scarto (entry_id, cartella, Message-ID, oggetto, errore) e il JSON del payload
   passano da `senzaNulTesto` / `jsonSenzaNul`: un byte NUL diventa U+FFFD. Un errore nello scrivere lo scarto
   non ha un savepoint sotto e ferma il lotto.
4. I `Saltati` con `entry_id` diventano scarti `lettura` (payload: l'elemento saltato, senza NUL); uno senza
   `entry_id` non è rileggibile: un avviso nel log e niente scarto. Tutti contano fra i `Falliti`.
5. Il cursore di (casella, cartella) avanza nella stessa transazione, salvo che sia nel futuro
   (`worker.NelFuturo`, tolleranza 5 minuti): allora resta dov'è e lo si scrive nel log.
6. `PrimaDelCommit` (solo prove), commit. Un commit fallito restituisce l'errore e una risposta con i conteggi a
   zero.

**Un elemento (`uno`).**
1. Controlli: Message-ID non vuoto e non oltre `MaxIdentificativo` (niente troncamento); direzione dichiarata
   nell'enum; `RicevutoIn` (ReceivedTime, altrimenti `data_evento`) non nel futuro.
2. Direzione e `interno` li decide `DirezioneEInterno`: mittente di un nostro dominio = uscita, e interno se lo
   sono anche tutti i destinatari; senza caselle o senza chiocciola vale la direzione del worker.
3. Conversazione per `ConversationID`, o `msg:<Message-ID>` se manca.
4. Controparte dal resolver di `classificazione` con `rubricaDB`; cliente e buyer solo in entrata e solo per una
   controparte cliente, il buyer solo se riconosciuto dall'indirizzo esatto (`clienteDallaControparte`).
5. `UpsertMessaggio` (parent risolto per `ParentMessageID` se già noto), `SetControparteMessaggio`,
   `applicaMarcatori`, `UpsertMessaggioOutlook`, `UpsertPresenza` (entry_id obbligatorio).
6. Allegati: due con lo stesso indice = errore dell'elemento. Per ogni file o elemento Outlook:
   `PropostaDaNome`, codice ripulito dai suffissi decorativi con `Motore.Canonico` / `CanonicoNome`,
   `InsertPropostaSeAssente`.
7. Staging automatico (D30) se il lotto lo consente, il messaggio è nuovo, il cliente è riconosciuto, e per i
   file pre-spuntati sotto la soglia (`StagingMaxByte`, altrimenti `classificazione.SogliaStagingAutomatico`).
   Ogni accodamento sta in un savepoint suo (`stageAutomatico`): un errore, anche SQL, annulla solo quello
   staging (compreso lo stato «in coda» scritto sull'allegato) e va nel log; l'elemento cade solo se lo
   staging non si riesce nemmeno ad annullare (`errStagingIrrecuperabile`).
8. Solo alla prima vista: riferimenti al portale (`RilevaPortale` sul corpo intero, codici canonici) e, se il
   messaggio è orfano e `daInterpretare` (entrata, interno, o nostra mail a un fornitore), `interpreta`.

**`interpreta`.** Estrazione con `Motore.Estrai` sui testi di `IngressoTriage.Testi()` (corpo tagliato dalla
catena, storia citata a parte, nomi degli allegati) → `aggancio.CalcolaESalva` con i soli codici di famiglia →
per un fornitore in entrata `CalcolaRichieste` + `SalvaCandidatiRichiesta`; in ogni altro caso
`SalvaCandidatiRichiesta` senza candidati, cioè i candidati verso una richiesta di una lettura precedente si
tolgono → per una nostra mail a un fornitore `RichiesteManuali` → `classificazione.Triage` →
`SalvaCandidatiCodice` → il motivo della controparte in testa se è fornitore o ambigua → `RilevaScadenza` →
`UpsertTriage`, che non tocca una proposta già decisa.

**Ritriage mirato.** `Ritriage(indirizzo, dominio)` e `RitriageMolti(indirizzi, domini)` raccolgono le chiavi
(senza doppioni), leggono i messaggi non decisi che le citano da mittente o destinatari
(`ListMessaggiDaRitriage`: `thread_id` vuoto e nessuna proposta accettata o rifiutata) e li rifanno uno per
transazione: controparte, buyer se mancava e, se orfano e da interpretare, candidati cancellati e
`interpreta` con gli allegati di primo livello e In-Reply-To / Riferimenti da `messaggio_outlook`. Un messaggio
che fallisce finisce in `EsitoRitriage.Dettagli` e non ferma gli altri. Non ricalcola `documento_proposta` né
`riferimento_portale`.

**All'avvio.** `RicalcolaControparti` risolve la controparte dei messaggi che non l'hanno (`controparte_il`
vuoto, quelli di prima della 0014), 500 per volta; un lotto che non aggiorna nessuna riga è un errore, e
l'avvio si ferma.

**Replay.** `Riprova(scartoID)`: origine `ingest` → `Ingerisci` senza tentativo con il payload salvato («acquisito»,
«già presente: aggiornato», «ancora in errore: …»); origine `lettura` → job `rileggi_elemento` sulla casella dello
scarto, oppure «già in coda».

## Invarianti

- **200 ⇔ tutto durevole.** Messaggi, scarti e cursore di un lotto entrano con un solo commit; se il commit
  non riesce la risposta è un errore e non resta niente. Un lotto ripetuto identico non duplica niente
  (upsert per Message-ID, per (messaggio, casella), per (messaggio, contenitore, indice)).
- Scrive solo il tentativo vivo: il job si verifica e si blocca dentro la transazione.
- Il lease autorizza il lotto del **suo** job: consegnano posta solo `sync_outlook` e `rileggi_elemento`, e solo
  per la casella che il job nomina. Un lotto rifiutato (`ErrLottoNonDelJob`) non scrive niente: né messaggi,
  né presenze, né cursori.
- Un elemento che il database o i controlli rifiutano si annulla fino al suo savepoint e va in
  `ingest_scarto`; gli altri entrano e il cursore avanza. Né i campi dello scarto né il suo payload contengono
  byte NUL, quindi lo scarto non viene rifiutato per lo stesso motivo dell'elemento.
- Un elemento saltato senza `entry_id` non ferma il lotto: si conta fra i falliti e si scrive nel log.
- Lo staging automatico è una comodità: un suo errore non fa cadere l'elemento, salvo che la transazione non
  si possa più riportare al savepoint annidato.
- Identità del messaggio = Message-ID (`chiave_esterna`), mai troncato. Ciò che è della copia (entry_id,
  cartella, letto, flag, categorie) sta in `messaggio_casella`, una riga per casella.
- La direzione la decide il server dalle caselle censite; nella query «uscita» vince fra due copie.
- Il cursore è per (casella, cartella), non va nel futuro e non arretra (`GREATEST` nella query).
- `messaggio.thread_id` si scrive solo con il marcatore `CockpitRichiestaFornitore`, solo su una mail in
  uscita e solo se il messaggio non sta già in un'altra RFQ; ogni aggancio così lascia una riga in
  `messaggio_aggancio_log`. Anche il marcatore `CockpitBozza` vale solo su una mail in uscita: in entrata si
  ignora, con un avviso.
- Il triage e i riferimenti al portale si scrivono solo alla prima vista del messaggio; `UpsertTriage` non
  sovrascrive una proposta accettata o rifiutata, `InsertPropostaSeAssente` non tocca una proposta esistente.
- I candidati di un messaggio sono una fotografia dell'ultima interpretazione: aggancio, codice e richiesta
  si sostituiscono tutti e tre, e quelli verso una richiesta restano solo se il messaggio è ancora posta in
  entrata di un fornitore.
- Il cliente arriva al triage solo per una controparte cliente in entrata: un fornitore, un ambiguo o uno
  sconosciuto non hanno cliente proposto (e da lì, in `classificazione`, niente `nuova_rfq`).
- Lo staging automatico non parte mai per lo storico, per un bootstrap non dichiarato, per un payload
  illeggibile, per un mittente non riconosciuto o per un messaggio già visto.
- Il ritriage legge solo messaggi non decisi; una controparte `manuale` non si sovrascrive (clausola della
  query `SetControparteMessaggio`).
- `messaggio.corpo_testo` si scrive com'è arrivato: il taglio della catena avviene solo nei testi dati
  all'interpretazione.

## Dipendenze

Importa: `core/inbox/classificazione`, `core/inbox/aggancio`, `core/registro/regole`, `platform/db`,
`platform/coda`, `platform/contratti/worker`, `platform/storage/staging` (solo il tipo `FileStaging`).
È importato da: `transport/workerapi`, `transport/web`, `app/runtime`. Tutto dentro la tabella di
`internal/README.md` (`core/*` → altri `core/*` e `platform`).

## Test

Tutte le prove con il database hanno il tag `integrazione` (L4): `go test ./...` senza tag non tocca il
database nemmeno con `COCKPIT_TEST_DSN` impostata. Il pool viene da `testutil.Pool`, che salta senza DSN e
rifiuta un database il cui nome, letto come lo legge pgx (`testutil.DatabaseDiTest`, con
`pgconn.ParseConfig`), non contiene «test». Comando:
`COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/core/inbox/ingest/`.

| File | Livello | Che cosa prova |
|---|---|---|
| `modo_test.go` | L1 | `scendonoDaSoli` in tutti i rami (spento, storico, bootstrap, payload vecchi, rilettura, riprova, payload illeggibile) e `ModoEffettivo` |
| `ingest_test.go` | L4 | idempotenza del lotto, presenza aggiornata quando l'elemento cambia cartella, proposta di triage; definisce `pool`, `casellaProva`, `clienteDiProva` |
| `candidati_db_test.go` | L4 | checkpoint 3R: l'ingest propone e non aggancia (T1, T22, T3/T4, T16), il ruolo dei candidati di codice, D30 solo per i riconosciuti e sotto soglia |
| `lotto_db_test.go` | L4 | poison pill e varianti (direzione, natura, indice doppio, Message-ID lungo, byte NUL nel corpo), replay dal payload e rilettura, commit fallito, cursore (interruzione, non arretra), casella non censita, tentativo scaduto, lotti concorrenti, cursore ≠ copertura |
| `robustezza_db_test.go` | L4 | byte NUL nell'oggetto, nella cartella o nel Message-ID: scarto e lotto che prosegue; NUL in un elemento saltato; elemento saltato senza `entry_id`; un errore SQL vero nello staging automatico che non scarta l'elemento; descrizione di una famiglia con lettere accentate oltre 120 byte; il ricalcolo che toglie i candidati verso una richiesta di una lettura vecchia |
| `lease_db_test.go` | L4 | il lease di un job che non consegna posta (download, apri elemento, segna letto) non scrive niente; nemmeno quello di un sync o di una rilettura di un'altra casella, con la casella nella colonna o solo nel payload; lo stesso lotto per la casella del job entra |
| `caselle_db_test.go` | L4 | stessa mail in due e in quattro caselle, risync che non perde il corpo, messaggio annidato, cursore su `ricevuto_il`, direzione dalle caselle |
| `decisioni_db_test.go` | L4 | il risync non annulla aggancio né proposte decise |
| `futuro_db_test.go` | L4 | elemento nel futuro in scarto e cursore fermo; due minuti di orologio avanti accettati |
| `controparte_db_test.go` | L4 | CP1–CP5, CP8, controparte `manuale` intatta dopo il ritriage |
| `invarianti_7c_db_test.go` | L4 | I4 (R0/R1 verso una richiesta sono candidati) e I5 (il marcatore della richiesta solo in uscita, verso una richiesta esistente, non su un messaggio già in un'altra RFQ; il marcatore della bozza solo in uscita) |
| `storico_db_test.go` | L4 | staging automatico dal modo del job, con un job vero preso in carico |
| `suffissi_db_test.go` | L4 | suffissi decorativi del cliente, e dei clienti che hanno richieste aperte a un fornitore |

Altrove: `transport/workerapi/copertura_db_test.go` (il lotto dentro il protocollo),
`transport/workerapi/ingest_db_test.go` (casella non autorizzata dalla credenziale, lease di un altro job,
lotto senza casella che prende quella del suo job),
`transport/web/{anagrafica,fornitori,richieste,bootstrap}_db_test.go` (censimento e ritriage, marcatori
richiesta e bozza, import del seme).

## Stato dell'implementazione

Completo: lotto transazionale con scarti e replay (fase 1), presenze per casella e direzione dal server (2.1),
guardia sul futuro (16/09), proposte al posto degli agganci automatici (checkpoint 3R), staging automatico per
modo (D30, 4A), controparte e ritriage (7A), marcatori e posta dei fornitori (7B), atto e legame nella
proposta (7C.0), suffissi decorativi (Fascicolo v3), lotto legato al job del tentativo e alla sua casella.

Parziale o in attesa:
- Il form che crea una richiesta dalla pagina della RFQ è tolto dal blocco 8: la strada dei marcatori resta e
  si usa quando la rotta viene chiamata.
- La controparte `manuale` è protetta dalla query, ma nessun codice la scrive ancora; il ritriage, comunque,
  ricalcola la proposta con la controparte risolta, non con quella manuale.
- Solo il canale Outlook (`CanaleOutlook` fisso in tutte le scritture).
- `forzaErrore` (`COCKPIT_INGEST_FORZA_ERRORE`) è un gancio di prova che nessuna prova imposta.

Limiti noti:
- Scadenza proposta e riferimenti al portale si cercano sul corpo intero, storia citata compresa.
- Un risync riscrive la controparte anche dei messaggi decisi; il ritriage invece non li tocca.
- Il ritriage non riguarda `documento_proposta` né `riferimento_portale`.
- Un errore transitorio del database dentro un elemento (per esempio un deadlock) si tratta come un dato
  sbagliato: l'elemento va in scarto e il cursore avanza.

## Dove intervenire

| Voglio… | Apri |
|---|---|
| capire perché un elemento è finito in scarto | `ingest.go:uno` (i controlli in testa), poi `ingest_scarto.errore` |
| capire perché un lotto è stato rifiutato con 403 | `ingest.go:lottoDelJob`, `ingest.go:CasellaDelJob`; per la credenziale `transport/workerapi/workerapi.go:ingest` |
| capire perché il cursore non avanza | `ingest.go:Ingerisci` (ramo `worker.NelFuturo`), `UpsertSyncCursore` in `platform/db/queries/messaggi.sql` |
| capire perché una mail risulta in uscita, o interna | `ingest.go:Nostri.DirezioneEInterno`, `DominiNostri` |
| capire perché un allegato è (o non è) sceso da solo | `ingest.go:scendonoDaSoli`, la condizione D30 in `uno`, `ingest.go:stageAutomatico`, `classificazione.PropostaDaNome` (`PreSpunta`) |
| capire perché un messaggio non ha una proposta di triage | `controparte.go:daInterpretare`, `row.Inserito` e `threadID` in `ingest.go:uno` |
| capire perché una controparte è quella | `controparte.go:rubricaDB`, `classificazione.RisolviControparte` |
| capire che cosa cambia «Censisci» sui messaggi già arrivati | `controparte.go:ritriageUno`, `ListMessaggiDaRitriage` in `platform/db/queries/fornitori.sql` |
| aggiungere un campo al messaggio in ingresso | `platform/contratti/worker/tipi.go:MessaggioIn` (e `workers/contratti.py`), poi `ingest.go:uno` |
| cambiare che cosa fa un marcatore | `marcatori.go:applicaMarcatori` |
| cambiare il modo di riprovare uno scarto | `replay.go:Riprova` |
| cambiare le regole usate per la posta di un fornitore | `ingest.go:Motori.PerFornitore` |

## Leggi anche

`internal/README.md` (il flusso Sync per intero), `internal/core/README.md` (i tre strati),
`internal/transport/README.md` (l'API dei worker), `internal/transport/workerapi/README.md` (la rotta del
lotto), `internal/platform/README.md` (query e migrazioni), `workers/workers_README.md` (chi manda i lotti).
