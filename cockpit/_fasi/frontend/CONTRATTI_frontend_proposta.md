# Il contratto dati del frontend: proposta per B7, lo SV e il ciclo di produzione

08/10/2026 · Stefano, con Claude · per Pietro.

**È una proposta di lavoro, non codice.** Non tocca A1c e non chiede niente prima del suo gate. Riusa i tipi e le rotte che esistono, e dice per ogni dato da dove viene. Le sigle sono le tue:
- **[V]** verificato sul ramo `smistamento-giro5` (`99fc9d9`) o nei documenti del giro 5;
- **[P]** proposta nostra, da decidere;
- **[R-nn]** una voce di `RICHIESTE_a_Pietro.md`.

## 0. Le regole che il contratto rispetta

1. **Il frontend mostra, non calcola.** Assi, completezza, fabbisogni, pertinenza e conflitti arrivano già calcolati da `valutazione.Calcola` (e da AM per i messaggi). Il frontend non ripete una regola di dominio [V: Architettura_db, «A1d / Front-end mostra DTO, NON ricalcola dominio»].
2. **Una GET non scrive.** Aprire la Distinta non accoda job e non riscrive proposte. «Cerca di nuovo» e «Rifai l'analisi» sono POST esplicite [V: rischio 73; Architettura_db].
3. **Ogni scrittura è un gesto con un autore.** Quelle che cambiano decisioni portano l'impronta della versione esaminata. Una versione superata dà un conflitto, mai una sovrascrittura [V: E2 §3.3, R108; piano 6.0.7].
4. **Nessuna scelta silenziosa.** Un'alternativa irrisolta resta fuori dal gesto, con il motivo [V: R108].
5. **Niente dati di clienti nel codice.** Esempi e prove con ACME; i nomi veri li ferma `controlla-privati.ps1` [V: R45, vademecum §12].
6. **htmx 2.0.4.** I frammenti rispondono 200 con HTML anche in errore [V: rischio 72].

## 1. I DTO in lettura, per schermata

### 1.1 Inbox

```
VistaInbox            caselle[] (contatori), clienti[] {cliente_id, ragione_sociale, attivo, n_richieste}
                      daSmistare n · senzaProdotto n · terzisti n · nascosti n
RichiestaInbox        thread_id · cliente · oggetto · riferimento_cliente · fase · data_scadenza
                      · prodotti[] (identificativo_thread confermati) · n_messaggi · ultimo_il · cartella_creata [V]
MessaggioConversazione  messaggio_id · direzione · data_evento · mittente {nome, indirizzo, buyer_id, ruolo}
                      · oggetto · testo (segmento corrente) · allegati[] {allegato_id, nome, tipo, provenienza}
                      · aggancio {modo, regola, da, il, motivo}                       [V: messaggio.aggancio, messaggio_aggancio_log]
                      · prodotti[] {codice, ruolo: richiesta|contesto|menzione, evidenze[]}  [V: AM0 §1.6; R-15]
```

- **Il filtro «un prodotto alla volta»** usa `prodotti[]`. Il messaggio iniziale di una richiesta congiunta compare in tutti i suoi prodotti, i successivi solo dove servono (R107).
- **I file della conversazione sono in sola lettura**, con la provenienza (mail, portale, caricato). Modificare e confermare i file si fa nella Distinta (R108).

### 1.2 Gestione delle RFQ

```
RigaRFQ               thread_id · numero (riferimento_cliente | progressivo [R-03]) · cliente · oggetto
                      · prodotti[] · fase · gruppo: avviata|accettata|produzione|terminata  [P: §2 della verifica]
                      · fascicolo {pronti, target, congelabile}                           [V: StatoFascicolo]
                      · data_scadenza · aggiornato_il
```

### 1.3 Pagina RFQ: gli assi (sempre in testa)

- `ProdottoValutato`: i sette assi, `Stato`, `Motivi[]` [V: §1.0].
- `StatoFascicolo`: `NumeroTarget`, `NumeroVerificati`, `Pronti`, `Bloccati`, `Congelabile`, `Congelato`, `FaseThread`, `Orfani`, `Avvisi` [V].
- Il mockup v10 mostra esattamente questi campi. Il composto non sostituisce mai gli assi.

### 1.4 Documenti e NAS

- `PV.Documenti` = `CompletezzaDocumentale` [V: §1.6]:
  - `Stato`, `PerimetroChiuso`;
  - `Voci[]` di `VoceFabbisogno` {componente, tipo, `Esito`, `Motivo`, `FileCandidato`, `Categoria`, `Invariante`, `NotaRegola`};
  - `Previsti[]`, `NonBloccanti[]`.
- `Disegno2D` {`Validita`, `Cartiglio`, `Anteprima`, `Formato`} e `GruppoDisegni2D` {`Primario`, alternativi, `RelazioniDaConfermare[]`} [V].
- `FileDaSmistare` e `AssociazioneFile` {`Disponibilita`, motivo, `Contesto`, `Perimetro`} [V: E2 §2.2]. Danno le tre situazioni di R108:
  - nessuna destinazione;
  - proposta da confermare;
  - analisi in corso o fallita.
- **NAS:** `documento.stato_nas`, `errore_nas`, `sha256` [V].

### 1.5 Distinta · Struttura

- `NodoBOM` {`Classificazione` {Ruolo, Categoria, Origine, Confermata, Motivo}, `AncheIn[]`} [V: E1 §2.5].
- `NodoProposto`, `ArcoProposto` (M2), `IdentitaNodo` {`StatoRevisione`, `CandidatiRevisione[]`} [V].
- `Conflitto` {Tipo, Asse, Decisione, Proposta, Evidenze} [V].
- **Le note:** `annotazione_pdf` {componente, allegato, pagina, x/y, testo, autore, quando}, più `fase_ciclo_id` facoltativa [R-14].

### 1.6 Riepilogo della «Conferma fascicolo» (SV)

```
RiepilogoGesto        prodotto · impronta (versione esaminata)
                      · gruppi[]: fonte_step | nomenclatura | relazioni | classificazioni | associazioni
                          decisioni[] {rif (chiave del nodo, dell'arco o dell'associazione), descrizione, evidenze[]}
                      · escluse[] {rif, motivo}   (alternative irrisolte, modifiche non salvate)
EsitoGesto            ok · confermate {per gruppo} · conflitti[] (impronta_cambiata, …) · nessun congelamento
```
È la forma del pannello del mockup v10 [P].

## 2. I gesti (scritture), con le rotte che ci sono

| Gesto del mockup | Rotta di oggi [V] | Dopo |
|---|---|---|
| Crea la richiesta / aggancia / non è una richiesta | `POST /messaggio/{id}/rfq`, `…/aggancia`, `…/ignora` | — |
| Crea l'RFQ | job `crea_cartella_thread` | significato da decidere [R-02] |
| Autorizza lo STEP come fonte | `POST …/componente/{cid}/step-strutturale` | — |
| Conferma, sostituisci o scarta un file | `…/proposta/{pid}/decidi`, `…/documento/{did}/sostituisci`, `…/file/{aid}/accetta` | — |
| Associa un file senza destinazione | `POST …/fascicolo/assegna` | — |
| Carica a mano | `POST …/fascicolo/carica` (thread) | dalla riga del componente [R-06] |
| Cerca di nuovo | `POST …/fascicolo/rianalizza` | — |
| Nodo: accetta, scarta, riapri, codice | `POST …/nodo/{pid}/accetta`, `scarta`, `riapri`, `codice` | con autore e valore di prima (SV, piano 3.8.4) |
| Sposta, tipo, rimuovi, archivia | `POST …/componente/{cid}/sposta`, `tipo`, `rimuovi`, `archivia` | regola dei contenitori [R-07] |
| Conferma la distinta | `POST /thread/{id}/distinta/albero/conferma` | — |
| Conferma minuteria | — | gesto dello SV [R-08] |
| Conferma fascicolo (cumulativa) | — | servizio dello SV con impronta (E2 §3.3) |
| Congela la V1 | `POST …/fascicolo/congela` | gesto del modello nuovo (LD-23) |
| Note sul disegno | `POST …/fascicolo/nota`, `…/nota/{nid}/modifica`, `elimina` | con la fase [R-14] |
| Richiesta al terzista | `POST /thread/{id}/richiesta`, `…/richiesta/{rid}/annulla` | con istruzioni e allegati [R-13] |
| Clienti e mittenti | `/admin/anagrafica…` (cliente, buyer, dominio, qualifiche) | mittente sospeso [R-04] |

## 3. Il ciclo di produzione: la persistenza nuova (dopo A2)

**Dove si colloca.**
- Fase FATTIBILITA («processi e materiali, Lino») [V: `fase_catalogo`].
- Dopo la 0022 di A2, perché tu hai già riservato la 0022.
- Un blocco suo, con le sue prove; non tocca le tabelle di A1c.

**Bozza indicativa, da non eseguire** [P], nello stile dei tuoi blocchi:

```
ciclo                 ciclo_id uuid PK · componente_id → componente (ON DELETE RESTRICT)
                      · stato (proposto | confermato) · fonte text · versione int
                      · materiale text · semilavorato text · dimensione text
                      · materiale_fornito_da (promatec | terzista)
                      · confermato_da → utente · confermato_il · creato_il
                      UNIQUE (componente_id, versione); righe solo aggiunte: una versione nuova a ogni conferma
fase_ciclo            fase_id uuid PK · ciclo_id → ciclo · ordine smallint · lavorazione → lavorazione(codice)
                      · in_maschera bool · norma text · filetti text · pieghe text · nota text
                      · (esterna) fornitore_id → fornitore · specifica text · istruzioni text · materiale_nostro bool
                      UNIQUE (ciclo_id, ordine)
fase_componente       fase_id → fase_ciclo · componente_figlio_id → componente · quantita int
                      UNIQUE (fase_id, componente_figlio_id)
preparazione_ciclo    ciclo_id → ciclo · tipo (maschera | programma_robot | piega | curvatura | stampo | dima | calibro)
                      · nota text · UNIQUE (ciclo_id, tipo)
lavorazione           + categoria (taglio | formatura | meccaniche | saldatura | assemblaggio | superficiali
                        | termici | finitura) · + interna bool                                    [R-12]
richiesta_fornitore   + istruzioni · + materiale_nostro · + prezzo, lotto, validita (offerta ricevuta)
richiesta_fornitore_allegato  richiesta_id · documento_id | allegato_id · con_note bool             [R-13]
annotazione_pdf       + fase_id → fase_ciclo (facoltativa)                                         [R-14]
```

**Regole del dominio del ciclo** [P]. Nel mockup sono controlli; nel backend sarebbero un esito calcolato:
- **ogni figlio diretto entra in almeno una fase del padre.** Altrimenti c'è un conflitto, e il ciclo non si conferma;
- **un trattamento affidato a un terzista** vuole un fornitore qualificato per il cliente della RFQ, da `cliente_fornitore_lavorazione`;
- **un commerciale non ha un ciclo**: lo compra l'ufficio acquisti;
- **il materiale lo fornisce Promatec**, salvo quando la prima fase è esterna;
- **«tutti i cicli confermati»** può essere il fatto richiesto della transizione FATTIBILITA → SCHEDA_COSTO.

**Lo storico delle lavorazioni esterne** è una vista sulle `richiesta_fornitore` con offerta ricevuta, per codice e per cliente. Non serve una tabella nuova, se R-13 entra.

## 4. Che cosa serve prima di scrivere il contratto vero

- Le tue risposte a R-01…R-16.
- Il contratto dei DTO di B7: dice quali campi del §1 sono già nell'esito di A1c e con quale nome. Il mockup v10 è pronto a seguirlo.
- La collocazione dello SV dopo A2, e di AM dopo il gate di A1c.
