# Verifica: il mockup del frontend contro il backend di Pietro

08/10/2026 · Stefano, con Claude · per Pietro.

**Su che cosa.**
- Il mockup `02_Mockup_frontend/cockpit-inbox-promatec.html` (versione 11).
- Il ramo `smistamento-giro5` (`99fc9d9`, B6b), installato e avviato su schema 21.
- I documenti privati del giro 5 (chiavetta, `archivio privato del giro 5`): il contratto A1c con E1, E1R ed E2 (proposta), le risposte R106–R117, AM0 (proposta congelata `8d4edcd7`), RC-01, il piano A.

Le verifiche sono fatte sul codice e sullo schema veri:
- **le tabelle e gli enum dal DB migrato** (80 tabelle, 21 migrazioni);
- **le rotte da `internal/transport/web`.**

## In breve

- **Il modello di base coincide.**
  - **Conversazione:** è una per richiesta (RFQ/thread), con un filtro per prodotto. È la tua D-AM3 del 07/10 («chat per RFQ/thread; il "riguarda i prodotti" è un risultato dell'analisi»).
  - **Distinta:** la usa Lino; il frontend mostra i DTO e non ricalcola il dominio. La GET non scrive niente: è il punto che tu stesso vuoi togliere dal vecchio telaio (la `POST prepara` all'apertura).
- **Quasi ogni gesto del mockup ha già una rotta.** L'elenco è al §2.
- **Il mockup è stato corretto in quattro punti per stare al contratto** (versione 10, §1):
  1. R103 C sulla minuteria;
  2. i sette assi del §1.0;
  3. le tre situazioni dei file di R108;
  4. «Conferma fascicolo» come gesto cumulativo con riepilogo (R67, R108).
- **Restano tre famiglie di cose che il backend oggi non ha.** Sono in `RICHIESTE_a_Pietro.md`:
  - il ciclo di produzione e le preparazioni, cioè tabelle nuove;
  - i dati dello storico delle lavorazioni esterne, cioè prezzo, istruzioni e allegati di `richiesta_fornitore`;
  - qualche decisione di significato: la «richiesta senza RFQ», il numero interno dell'RFQ, il mittente sospeso.

## 1. Che cosa abbiamo corretto nel mockup

| Punto | Prima | Ora | Fonte |
|---|---|---|---|
| 2D dei commerciali | non richiesto (avevo seguito R82 di B0+E1) | richiesto; **l'unica esenzione è la minuteria confermata**. Nella tabella delle sotto-parti e nella scheda del pezzo c'è la categoria (fabbricato, commerciale, minuteria) con «Conferma minuteria». Finché è solo proposta, il 2D resta richiesto | R103 C (E1R), K-01 A, T-E1-18 |
| Stati | quattro riquadri (fonte, BOM, documenti, composto) | **i sette assi per prodotto**: target, fonte, nomenclatura, gerarchia, smistamento, completezza, stato del prodotto (`non_pronto`, `pronto_fattibilita`). In più il fascicolo con «N su M pronti» e «congelabile». Gerarchia `da_verificare` finché la fonte non è confermata | contratto §1.0, R79, R88, R89, K-02 A |
| File senza destinazione | un solo elenco | tre situazioni distinte: **nessuna destinazione**, **proposta da confermare**, **analisi in corso o fallita** | R108, E2 §2.2 |
| Conferma | conferme separate nel passo 1 e nella Distinta | in più **«Conferma fascicolo»**, che apre un riepilogo dei gruppi di decisione del prodotto, da spuntare: fonte STEP, nomenclatura, relazioni e quantità, classificazioni, associazioni. Poi un solo gesto e un esito unico; le alternative irrisolte restano fuori, con il motivo; c'è la versione esaminata (impronta). Non congela e non avvia la fattibilità | R67 A, R108, E2 §3.3 |

**In più, dalla versione 11:**
- il mockup usa il vocabolario del backend: `fase` con le transizioni automatiche, `stato_thread`, gli enum dei tipi;
- dichiara per ogni gesto la rotta vera (`sorgenti/15_contratto.js`);
- mostra i DTO con i nomi dei tipi di `valutazione` nel pannello «Dati e chiamate» della pagina RFQ.

## 2. Schermata per schermata

Legenda:
- ✅ c'è già;
- 🟡 c'è, con un adattamento di significato;
- ❌ manca nel backend (→ richiesta).

### Inbox

| Funzione del mockup | Backend | |
|---|---|---|
| Clienti autorizzati, sospesi | `cliente.attivo`; *Admin › Anagrafica* (`/admin/anagrafica…`) | ✅ |
| Mittenti autorizzati per cliente, con il dominio | `buyer` (`email`, `tipo`, `confermato`), `dominio_cliente`; rotte `…/buyer`, `…/dominio` | ✅ |
| Sospendere un singolo mittente | `buyer` non ha `attivo`; c'è solo `confermato` | 🟡 → R-04 |
| Messaggi nascosti (mittenti fuori anagrafica) | `messaggio.controparte_tipo` (`sconosciuto`, `altro`), `soggetto_altro`, `recapito_altro` | ✅ |
| Da smistare: crea la richiesta, aggancia, non è una richiesta | `proposta_triage` (`nuova_rfq`, `aggancia`, `ignora`), `POST /messaggio/{id}/rfq`, `…/aggancia`, `…/ignora` | ✅ |
| Una conversazione per richiesta | `thread_offerta` + `messaggio.thread_id` + `conversazione` | ✅ (D-AM3) |
| La mail di un'altra catena agganciata alla richiesta, con il motivo | `messaggio.aggancio` (`operatore`, `auto_*`), `messaggio_aggancio_log`; regole R0–R5 | ✅ |
| «Guarda un prodotto alla volta» | il risultato di AM0: i prodotti che il messaggio riguarda, con il ruolo `richiesta` / `contesto` / `menzione` | 🟡 dopo il gate di A1c (AM1–AM3); fino ad allora il filtro non ha dati |
| Senza prodotto (posta non RFQ del cliente) | `messaggio.thread_id` nullo con controparte `cliente`; `atto_business` per l'atto | ✅ |
| Terzisti: le chat per lavorazione | `richiesta_fornitore` (`thread_id`, `fornitore_id`, `lavorazione`, `codici`, `messaggio_id`, `stato`), `messaggio.richiesta_fornitore_id` | ✅ |

### Richieste RFQ (gestione)

| Funzione | Backend | |
|---|---|---|
| Avviate, accettate, in produzione, terminate | `fase` di `thread_offerta`: avviate = RICEVUTA…OFFERTA_INVIATA; accettate = ACCETTATA, DISTINTA_ERP, ORDINE; in produzione = PRODUZIONE; terminate = PERSA, RESPINTA, SCADUTA, più `stato = CHIUSA` | 🟡 raggruppamento solo di presentazione |
| «Consegnata» fra le terminate | non c'è una fase di consegna | ✅ tolta dal mockup: un ordine evaso è un thread CHIUSO (R-05) |
| Numero dell'RFQ: quello del cliente o un nostro progressivo | `thread_offerta.riferimento_cliente`; nessun progressivo interno | 🟡 → R-03 |
| Richiesta nell'Inbox **senza** RFQ, e «Crea l'RFQ» | il thread nasce al triage `nuova_rfq`: non esiste una richiesta senza thread | 🟡 → R-02 (proposta: «Crea l'RFQ» = cartella sul NAS + inizio del lavoro) |

### Pagina RFQ · Documenti e NAS

| Funzione | Backend | |
|---|---|---|
| STEP strutturale e PDF del prodotto proposti, da confermare | `documento_proposta`, `v_step_prodotto`, `POST …/step-strutturale`, `…/fascicolo/conferma` | ✅ |
| Sotto-parti con 3D, 2D e DXF, fabbisogni | `fabbisogno_documento`, `v_fascicolo`, `ListFabbisognoEffettivo`; DTO `VoceFabbisogno`, `CompletezzaDocumentale` | ✅ |
| Ispeziona, cambia con un altro candidato, rimuovi | `POST …/documento/{did}/sostituisci`, `…/proposta/{pid}/decidi`, `…/file/{aid}/accetta` | ✅ |
| Cerca di nuovo | `POST …/fascicolo/rianalizza` | ✅ (deve restare un gesto, mai all'apertura) |
| Carica a mano | `POST …/fascicolo/carica` (a livello di thread) | 🟡 dalla riga di un componente non c'è (piano 6.0.7) → R-06 |
| Indicato sul portale | `riferimento_portale` (0 righe sul dump) | ✅ |
| Associa un file senza destinazione | `POST …/fascicolo/assegna` | ✅ |
| Copia sul NAS con le impronte | job `copia_nas`, `documento.sha256`, `stato_nas`, integrità (0013) | ✅ |
| Le tre situazioni di un file (R108) | E2 §2.2: `FileDaSmistare.Disponibilita` e il motivo in B6b/B7 | 🟡 in arrivo con B7 |

### Pagina RFQ · Distinta, vista Struttura

| Funzione | Backend | |
|---|---|---|
| Albero con proposte tratteggiate e confermate | `componente_proposta`, `relazione_proposta`, `componente`, `componente_relazione`; DTO `NodoProposto`, `ArcoProposto`, `NodoBOM` | ✅ |
| Accetta, scarta, riapri, correggi il codice | `POST …/nodo/{pid}/accetta`, `scarta`, `riapri`, `codice` | ✅ (la correzione del codice oggi non tiene autore e valore di prima → piano 3.8.4, SV) |
| Aggiungi, sposta (anche trascinando), elimina, cambia tipo | editor e `bom/applica`; `…/componente/{cid}/sposta`, `tipo`, `rimuovi`, `archivia` | ✅ |
| Sotto un particolare o un commerciale non si mette niente | oggi l'editor promuove il particolare ad assieme | 🟡 → R-07 |
| Conferma l'albero / la distinta | `POST /thread/{id}/distinta/albero/conferma`; nomenclatura e gerarchia del §1.0 | ✅ |
| Categoria fabbricato, commerciale, minuteria, e la sua conferma | `Classificazione` (calcolo, E1 §6); la conferma della minuteria è del modello nuovo | 🟡 il gesto di conferma è dello SV → R-08 |
| Codice interno di un assieme aggiunto a mano (es. `…-A01`) | RC-01 propone alias sintetici (`codiceProdotto_P1`), ancora aperto | 🟡 → R-09 |
| Visore con le note sul disegno, condivise | `annotazione_pdf` (componente, allegato, pagina, x/y normalizzati, autore); `POST …/fascicolo/nota…` | ✅ |
| Componente usato in più prodotti | `NodoBOM.AncheIn` (E1) | ✅ |
| Congela la V1 | `POST …/fascicolo/congela`, `bom_versione` (`contesto` preventivo/tecnica) | ✅ (il gesto del modello nuovo, LD-23, è dello SV) |
| Conferma fascicolo cumulativa | E2 §3.3: servizio dello SV | 🟡 il mockup ne mostra la UI; il servizio è tuo, dopo A2 |

### Pagina RFQ · Distinta, vista Ciclo di produzione

| Funzione | Backend | |
|---|---|---|
| Materiale, spessore, peso del pezzo | `componente.materiale_testo`, `spessore_mm`, `peso_kg` | 🟡 mancano semilavorato, dimensione e chi lo fornisce |
| Esito della fattibilità | `componente.esito_fattibilita` (OK, KO, DUBBIO), `note_fattibilita`; fase FATTIBILITA («processi e materiali (Lino)») | ✅ |
| Fasi in ordine, per categoria, interne ed esterne | nessuna tabella | ❌ → R-10 |
| Figli e commerciali che entrano in una fase (es. dado a saldare) | nessuna tabella | ❌ → R-10 |
| Spunta «in maschera» sulle saldature; preparazioni (maschera, programma robot, attrezzature, stampo, dima, calibro) | nessuna tabella; la lavorazione `attrezzature` esiste solo come capacità di un fornitore | ❌ → R-11 |
| Catalogo delle lavorazioni | `lavorazione` (13 codici: taglio_laser, piega, saldatura, tornitura, sabbiatura, cataforesi, verniciatura_polvere, zincatura, lavaggio_zinco, trattamento_termico, curvatura_tubi, fresatura, attrezzature) | 🟡 servono le lavorazioni interne mancanti e le voci per categoria → R-12 |
| Terzisti qualificati per cliente | `fornitore`, `fornitore_lavorazione`, `cliente_fornitore_lavorazione` | ✅ |
| Prepara la richiesta al terzista, bozza nella sua chat | `POST /thread/{id}/richiesta`, `richiesta_fornitore` (`bozza` → `inviata` → `offerta_ricevuta`), `bozza` di Outlook | 🟡 mancano istruzioni, allegati scelti, materiale nostro o suo, prezzo e lotto dell'offerta → R-13 |
| Storico delle lavorazioni esterne, riuso | le `richiesta_fornitore` passate con i loro `codici` e messaggi | 🟡 senza prezzo e istruzioni non si riusa davvero → R-13 |
| Archivio dei disegni per cliente | `documento` + `thread_offerta.cliente_id` | ✅ (solo una query) |
| Nota legata a una fase | `annotazione_pdf` non ha la fase | ❌ → R-14 |
| Commerciali «da acquistare», gestiti da Fabio | `tipo = commerciale`, fase OFFERTE_FORN | ✅ (niente acquisti nella Distinta) |

### Offerta
- Solo un segnaposto nel mockup.
- Il backend ha le fasi SCHEDA_COSTO, OFFERTE_FORN e OFFERTA_INVIATA. I costi li fa Franco con il suo strumento.

## 3. I punti in cui il frontend deve rispettare un vincolo tuo

- **Aprire una pagina non scrive.** La Distinta del mockup non chiama niente all'apertura; «Cerca di nuovo» e «Rifai l'analisi» sono gesti.
- **Il frontend non ricalcola il dominio.** I sette assi, la completezza e i fabbisogni del mockup sono simulati solo perché non c'è il server. Nel frontend vero arrivano da `ProdottoValutato` e `StatoFascicolo`.
- **Mai una scelta silenziosa.** Le alternative irrisolte restano fuori dal gesto cumulativo, con il motivo.
- **Nessun dato reale nel repository.** Il pacchetto per la PR usa dati ACME (vedi `PR_frontend_nota.md`).
