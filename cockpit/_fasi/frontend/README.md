# Frontend del Cockpit RFQ · consegna per il merge

Per **Pietro**. È il frontend di Stefano, allineato a `smistamento-giro5` (`99fc9d9`, B6b): il mockup completo, il contratto dati che gli serve e le funzioni che mancano nel backend.

**Il merge lo fai tu:** qui sotto c'è tutto quello che serve sapere. Il §1 riguarda il merge; dal §2 in poi, il contratto e le richieste.

## 1. Il merge

### 1.1 Che cosa cambia nel repository

- **Solo file nuovi**, tutti in `cockpit/_fasi/frontend/`. Nessun file esistente è modificato, quindi nessun conflitto possibile con B6, B6b, Q10 o il resto di A1c.
- **Il binario non cambia.** `embed.go` incorpora solo `migrations/`, `web/templates/`, `web/static/` e `workers/`; `_fasi/` resta fuori, come il tuo `mockup_distinta.html`.
- **Niente Go.** Nessun `*_test.go`, nessuna migrazione, nessuna query sqlc, nessun template. `go build` e `go vet` non vedono la cartella.
- **Il test del mockup è in JavaScript** (`sorgenti/test_clic.cjs`, jsdom) e non entra in `prova-tutto.ps1`.

### 1.2 I passi suggeriti

1. `git fetch origin`, poi `git switch frontend-mockup-v10` (o la PR su GitHub).
2. **Il controllo dei dati privati sul ramo**, come per i tuoi push:
   ```
   scripts\controlla-privati.ps1 -Ramo frontend-mockup-v10
   ```
   - Tutti i dati del mockup sono di fantasia (ACME e simili).
   - Il controllo dei token è stato rifatto in locale con gli elenchi del tuo dataset, con la stessa regola (fisso, parola, sigla; confine «né lettera né cifra ASCII»), e dà **0 occorrenze**.
   - L'esito che vale resta il tuo, con il manifest.
3. **Il ramo QA.** Se vuoi tenere la coppia prodotto/QA allineata, il commit si porta su `smistamento-giro5-qa` con un cherry-pick: non ci sono prove Go da aggiungere.
4. **Il merge**, quando decidi tu. Non dipende da R-01…R-16: sono richieste per i blocchi che verranno.

### 1.3 Come si prova

- **Il mockup:** `cockpit-frontend-mockup.html` si apre con un browser, senza server e senza rete (salvo i font di Google).
- **Il test:** da `sorgenti/`, `npm install` (scarica jsdom; `node_modules` è in `.gitignore`), poi `node test_clic.cjs`. Oggi: **203 prove, 0 fallite, 0 errori JS**. Clicca tutti i percorsi: Inbox, smistamento, gestione RFQ, Documenti e NAS, Distinta (struttura, ciclo, conferma cumulativa), impostazioni.
- **Rifare il file unico dai pezzi:** `sh assembla.sh`.

### 1.4 Che cosa c'è

| File | Che cosa è |
|---|---|
| `cockpit-frontend-mockup.html` | il mockup in un solo file |
| `sorgenti/` | i pezzi; `15_contratto.js` è il contratto con il backend (§3) |
| `VERIFICA_mockup_backend.md` | il mockup confrontato con il ramo, schermata per schermata |
| `CONTRATTI_frontend_proposta.md` | la proposta per i DTO di B7, i gesti dello SV e il ciclo di produzione (niente codice) |
| `RICHIESTE_a_Pietro.md` | R-01…R-16, ognuna con che cosa c'è e una proposta |

## 2. Come il mockup segue il backend

**Il vocabolario è il tuo:**
- `fase` del thread, con il catalogo di `fase_catalogo` (ufficio, ordine) e le due transizioni automatiche (RICEVUTA/ATTESA_DISEGNI → FATTIBILITA quando `v_fascicolo` non ha righe bloccanti aperte);
- `stato_thread` APERTA/CHIUSA;
- `tipo_componente`, `tipo_documento` (`cad_3d`, `disegno_2d`, `sviluppo_dxf`), `stato_proposta`, `stato_richiesta_fornitore`, `esito_fattibilita`.

I gruppi della gestione (avviate, accettate, in produzione, terminate) sono **solo presentazione**, calcolati dalla fase. La fase «consegnata», che non esisteva, è stata tolta.

**Il contratto A1c:**
- i sette assi del §1.0 per prodotto e lo `StatoFascicolo` (pronti, congelabile, `FaseThread`);
- il 2D secondo **R103 C**: richiesto anche per i commerciali, con l'unica esenzione della minuteria confermata;
- la categoria distinta dal tipo (`Classificazione`);
- le tre situazioni dei file di **R108**: nessuna destinazione, proposta da confermare, analisi in corso o fallita;
- la **«Conferma fascicolo»** di R67 e R108: un riepilogo per gruppi (fonte, nomenclatura, relazioni e quantità, classificazioni, associazioni), le alternative irrisolte fuori, un esito unico, la versione esaminata. Non congela e non avvia la fattibilità.

**Le tue regole:**
- **una GET non scrive:** aprire la Distinta non chiama `prepara`, e «Cerca di nuovo» e «Rifai l'analisi» sono gesti;
- **il frontend non ricalcola il dominio:** gli assi del mockup sono simulati solo perché non c'è un server;
- **nessuna scelta silenziosa.**

**Il pannello «{ } Dati e chiamate»** nella pagina RFQ mostra:
- i DTO della pagina, con i nomi dei tuoi tipi (`ProdottoValutato`, `StatoFascicolo`, `CompletezzaDocumentale`, `NodoBOM`);
- il registro delle rotte che ogni gesto chiamerebbe.

È il modo più rapido per leggere il contratto dal vivo.

## 3. I gesti e le rotte

Generata dalla tabella `GESTI` di `sorgenti/15_contratto.js`:
- **esiste** = la rotta c'è nel ramo;
- **R-nn** = manca, vedi le richieste;
- **SV** = spazio di verifica (E2 §3.3).

| Gesto nel mockup | Metodo | Rotta | Stato | Nota |
|---|---|---|---|---|
| Aprire una RFQ | GET | `/thread/{id}/fascicolo` | esiste | sola lettura |
| Passo Documenti e NAS | GET | `/thread/{id}/fascicolo` | esiste | sola lettura |
| Passo Distinta (aprirla) | GET | `/thread/{id}/distinta` | esiste | sola lettura: niente POST prepara all’apertura |
| Passo Offerta | GET | `/thread/{id}` | esiste | offerta: solo lettura |
| «Crea l’RFQ» | JOB | `crea_cartella_thread` | R-02 | significato di «Crea l’RFQ» da confermare |
| Crea la richiesta da «Da smistare» | POST | `/messaggio/{id}/rfq` | esiste |  |
| Aggancia un messaggio | POST | `/messaggio/{id}/aggancia` | esiste |  |
| «Non è una richiesta» | POST | `/messaggio/{id}/ignora` | esiste |  |
| Conferma un file proposto | POST | `/thread/{id}/fascicolo/proposta/{pid}/decidi` | esiste | conferma |
| Conferma lo STEP strutturale | POST | `/thread/{id}/fascicolo/componente/{cid}/step-strutturale` | esiste | autorizza la fonte |
| Conferma tutti i riconosciuti | POST | `/thread/{id}/fascicolo/conferma` | esiste |  |
| Rimuovi un file dallo slot | POST | `/thread/{id}/fascicolo/proposta/{pid}/decidi` | esiste | scarta |
| Cambia con un altro candidato | POST | `/thread/{id}/fascicolo/documento/{did}/sostituisci` | esiste |  |
| Cerca di nuovo | POST | `/thread/{id}/fascicolo/rianalizza` | esiste |  |
| Carica a mano | POST | `/thread/{id}/fascicolo/carica` | R-06 | oggi a livello di thread: manca il caricamento dalla riga del componente |
| Associa un file senza destinazione | POST | `/thread/{id}/fascicolo/assegna` | esiste |  |
| Metti da parte | POST | `/thread/{id}/fascicolo/proposta/{pid}/decidi` | esiste | messo da parte = scarta |
| Conferma e copia sul NAS | POST | `/thread/{id}/fascicolo/conferma` | esiste | conferma e copia (job copia_nas) |
| Conferma minuteria | POST | `(spazio di verifica) conferma della categoria` | R-08 | gesto nuovo |
| Conferma fascicolo (cumulativa) | POST | `(spazio di verifica) conferma fascicolo cumulativa` | SV | E2 §3.3: riepilogo, impronta, esito unico |
| Conferma una casella | POST | `/thread/{id}/fascicolo/nodo/{pid}/accetta` | esiste |  |
| Scarta una casella proposta | POST | `/thread/{id}/fascicolo/nodo/{pid}/scarta` | esiste |  |
| Correggi il codice | POST | `/thread/{id}/fascicolo/nodo/{pid}/codice` | esiste |  |
| Accetta la struttura proposta | POST | `/thread/{id}/distinta/albero/conferma` | esiste |  |
| Salva la distinta | POST | `/thread/{id}/fascicolo/bom/applica` | esiste | tutto in una volta |
| Sposta un pezzo (anche trascinando) | POST | `/thread/{id}/fascicolo/componente/{cid}/sposta` | R-07 | con la regola dei contenitori |
| Cambia tipo | POST | `/thread/{id}/fascicolo/componente/{cid}/tipo` | esiste |  |
| Elimina un pezzo | POST | `/thread/{id}/fascicolo/componente/{cid}/rimuovi` | esiste |  |
| Rifai l’analisi | POST | `/thread/{id}/fascicolo/rianalizza` | esiste |  |
| Nota sul disegno | POST | `/thread/{id}/fascicolo/nota` | esiste | con la fase: R-14 |
| Togli una nota | POST | `/thread/{id}/fascicolo/nota/{nid}/elimina` | esiste |  |
| Ciclo di produzione (fasi) | POST | `(nuovo) ciclo di produzione` | R-10 | ciclo, fasi, componenti per fase |
| Preparazioni del ciclo | POST | `(nuovo) preparazioni del ciclo` | R-11 |  |
| Richiesta al terzista | POST | `/thread/{id}/richiesta` | esiste | istruzioni e allegati: R-13 |
| Aggiungi un mittente | POST | `/admin/anagrafica/{id}/buyer` | esiste |  |
| Togli un mittente | POST | `/admin/anagrafica/{id}/buyer/elimina` | esiste |  |
| Sospendi un mittente | POST | `/admin/anagrafica/{id}/buyer` | R-04 | oggi c’è solo «confermato» |
| Cliente attivo/sospeso | POST | `/admin/anagrafica/{id}` | esiste |  |
| Nuovo cliente | POST | `/admin/anagrafica` | esiste |  |

## 4. Il contratto dati che serve al frontend

Il dettaglio campo per campo è in `CONTRATTI_frontend_proposta.md`. In breve, per schermata:

| Schermata | Che cosa legge | Da dove, nel tuo backend |
|---|---|---|
| Inbox | richieste (thread) con prodotti, fase, scadenza; messaggi con l'aggancio (modo, regola, motivo) e **i prodotti che il messaggio riguarda, con il ruolo** (`richiesta`, `contesto`, `menzione`) | `thread_offerta`, `identificativo_thread`, `messaggio`, `messaggio_aggancio_log`; i prodotti da AM (`EsitoAnalisiMessaggio`, dopo il gate di A1c) |
| Gestione RFQ | riga per thread con fase, `StatoFascicolo` riassunto, scadenza | `thread_offerta`, `v_thread_fase`, l'esito di `valutazione` |
| Pagina RFQ, in testa | i sette assi e il fascicolo | `ProdottoValutato`, `StatoFascicolo` |
| Documenti e NAS | voci dei fabbisogni con esito e motivo, 2D con validità e cartiglio, file da smistare con disponibilità e motivo | `CompletezzaDocumentale`, `VoceFabbisogno`, `Disegno2D`, `GruppoDisegni2D`, `FileDaSmistare`, `AssociazioneFile` (B7) |
| Distinta, struttura | nodi e archi proposti e confermati, classificazione, usi in altri prodotti, conflitti, note | `NodoBOM`, `NodoProposto`, `ArcoProposto`, `IdentitaNodo`, `Conflitto`, `annotazione_pdf` |
| Conferma fascicolo | riepilogo per gruppi con evidenze, escluse con motivo, impronta | servizio dello SV (E2 §3.3) |
| Distinta, ciclo di produzione | ciclo, fasi, componenti per fase, preparazioni, storico delle lavorazioni esterne | **nuovo**: R-10…R-14 |

**Come lo chiediamo:**
- **un solo payload per pagina, già calcolato:** l'esito di `valutazione.Calcola` tradotto nei DTO di B7;
- **le scritture solo come gesti POST** con l'impronta della versione esaminata, dove la decisione può essere superata;
- **i frammenti htmx sempre 200 con HTML**, anche in errore, come già fai.

## 5. Le funzioni che servono (dettaglio in `RICHIESTE_a_Pietro.md`)

| Sigla | Che cosa | Dove la collocheremmo |
|---|---|---|
| R-01 | il mockup come riferimento visivo del contratto DTO di B7 | B7 |
| R-02 | «Crea l'RFQ» = cartella sul NAS e inizio del lavoro su un thread in RICEVUTA | decisione |
| R-03 | numero interno dell'RFQ quando il cliente non lo dà | decisione |
| R-04 | sospendere un singolo mittente (`buyer.attivo`, o il significato di `confermato`) | anagrafica |
| R-05 | nessuna fase di consegna: «consegnata» è stata tolta dal mockup (chiusa da noi, se ti va bene) | — |
| R-06 | caricare un file dalla riga di un componente | SV |
| R-07 | sotto un particolare o un commerciale non si mette niente (oggi l'editor promuove ad assieme) | SV |
| R-08 | il gesto di conferma della categoria minuteria | SV |
| R-09 | un formato del codice interno di un assieme aggiunto (vedi l'alias di RC-01) | decisione |
| R-10 | il ciclo di produzione: ciclo, fasi, componenti per fase | dopo A2 |
| R-11 | le preparazioni del ciclo (maschere, programma robot, attrezzature, stampo, dima, calibro) | dopo A2 |
| R-12 | il catalogo delle lavorazioni completo, con categoria e interna/esterna | dopo A2 |
| R-13 | la richiesta al terzista con istruzioni, allegati, materiale e l'offerta (prezzo, lotto, validità) | dopo A2 |
| R-14 | una nota sul disegno legata a una fase del ciclo | dopo A2 |
| R-15 | i prodotti del messaggio con il ruolo, per il filtro della conversazione | AM |
| R-16 | la cartella di questa consegna (se ne preferisci un'altra, si sposta) | merge |

## 6. Che cosa resta a noi

- Quando il contratto dei DTO di B7 è fissato, il mockup si allinea ai nomi definitivi: è un lavoro solo di `15_contratto.js` e delle viste.
- Poi si traduce nei template `web/templates/` con htmx, una schermata per volta, nei blocchi che deciderai.
- Nessuna scrittura di dominio nostra: i gesti restano le tue rotte e i tuoi servizi.
