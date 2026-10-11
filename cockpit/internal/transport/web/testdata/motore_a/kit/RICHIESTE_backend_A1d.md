# Richieste del frontend al backend (A1d v1.1)

Le richieste R-FE-01…R-FE-19 dell'analisi del frontend, con la risposta del piano del backend di A1d (A1d-P2): **risolta**
(con il nome che il template usa), **rinviata** (con dove resta aperta), **non accolta** o **facoltativa**. Le firme
esatte e le regole di ogni ricerca stanno nel piano del backend (§5.4-§5.7); qui c'è solo che cosa usa la pagina.

## Risolte dal backend di A1d

| R-FE | Che cosa chiedeva | Risposta (A1d-P2) | Dove la usa la pagina |
|---|---|---|---|
| R-FE-01 | `etichetta` con valori tipati e puntatori | `etichetta(dominio string, valore any) string` in `funzioni` (BE-10): segue i puntatori, legge ogni tipo stringa con nome; un valore ignoto torna `[valore]`. Tabella del server: 62 domini, 319 voci (RAC-11, con `origine_categoria`); i testi li firma il backend (D-RAC-01) | ovunque un valore di enum diventa testo |
| R-FE-02 | radici e figli per gruppo, senza scorrere `Parentela` | `Indice.Radici prodotto gruppo` e `Indice.Figli prodotto gruppo rif` (`{Nodo, Quantita, QuantitaConfermata, Decisa}`, X-04); un figlio con due padri compare sotto tutti e due | R5, l'albero di ogni scheda |
| R-FE-03 | nodo nel gruppo, stesso `Rif` in due strutture | `Indice.Gruppi prodotto` (chiavi opache `bom`, `cand:<allegato>:<radice>`, `senza`, X-02; con `Struttura` e `Nodi`) e `Indice.NodoIn prodotto gruppo rif` | R5 schede e caselle, R6 pannelli (chiave `gruppo\|rif`) |
| R-FE-04 | relazioni confermate correnti accanto ai legami del file | `Indice.Legami prodotto gruppo rif` → `DelFile` e `Confermati` (`PadreID`, `PadreRif`, `Codice`, `Qta`, `NelFile`), dalle relazioni della fotografia (X-05) | R5 linee piene e «confermato sotto Y»; R6 Posizione; «Da» di Sposta e Scollega |
| R-FE-05 | il componente registrato | `Indice.Componente v` → `{ID, Codice, Rev, Descrizione, Tipo, Archiviato, StepStrutturale}` | «Salva rev e descrizione» (solo con la descrizione registrata), «Ripristina», codici nei conflitti e nelle associazioni |
| R-FE-06 | i documenti correnti dello stesso tipo, per `scelta` | `Indice.Correnti componente tipo` → `{DocumentoID, NomeFile, Rev}` (X-06) | cassetto di R9: le risposte `aggiungi` / `sostituisci:<id>`, e la nota del caso e19 |
| R-FE-07 | anteprima di un allegato qualunque | `Indice.Anteprima allegato` con la forma di `Disegno2D.Anteprima` (X-09) | una sola miniatura per 2D, «Da smistare» e documenti |
| R-FE-08 | la voce del componente nella casella | `Indice.Voce prodotto componente tipo` (X-06) | R5, le celle 3D, 2D e DXF della casella |
| R-FE-09 | conflitti per prodotto e asse | `Indice.Conflitti prodotto asse` con `struttura` (tutti tranne lo smistamento) e `smistamento` (X-06, T-A1d-25); più `Indice.ConflittiDelThread` | R7 e R10 |
| R-FE-10 | copie sul NAS aperte | `Indice.CopieAperte prodotto` (X-06) | R9, accanto al solo «Riprova le copie» |
| R-FE-11 | gli altri candidati della fonte | `Indice.CandidatiFuori prodotto` (X-06, T-A1d-24) | R3, «Altri documenti candidati alla fonte» |
| R-FE-12 | segnale dei campi EB7 | non serve più: la pagina vuole l'esito `valutazione-3` (D-CHIUS-3; nel kit d'origine era `valutazione-2`); un'altra versione arriva come `Errore` (X-07). Spariscono le scritte «dato non ancora disponibile (EB7)» | — |
| R-FE-13 | anteprima JSON dello STEP più ricca | `autorizzabile`, `inviabile`, `figli` (le chiavi di `NodoEffetto`, mostrate come sono), `deleghe[].spenta`, `deleghe[].revoca`, `gia` (un testo, non un booleano: T-A1d-28); elenchi mai `null` (BE-11) | la finestra di «Autorizza lo STEP strutturale…» |
| R-FE-14 | impronta del ponte e del worker di pdf.js | importmap in `contenuto` prima del modulo, che porta `/static/motore_a_gesti.mjs` all'indirizzo con `?v=` (P-10, RAC-17); il worker da `{{statico}}` in `data-ma-pdfjs-worker`; nessun `no-cache` | `contenuto` di `motore_a.html` |
| R-FE-15 | esito del gesto deciso dal server, non dal testo | il ramo `motore-a` di `threadFrammento` scrive `data-ma-esito`; le frasi di rifiuto che oggi arrivano con HTTP 200 sono «no» per un elenco chiuso del server (P-07 b, T-A1d-11); il ponte non classifica il testo | `#ma-avviso` |

Altre richieste del kit vecchio, ora chiuse: le forme di ritorno di `AutorizzazioneDi`, `RelazioneProposta`,
`Rimozione` sono fissate (`autorizzazioneMA`, `relazioneMA`, `rimozioneMA`; in più `AutorizzazioneDiStruttura` per
allegato e radice); la rilettura dopo «Conferma l'albero» la fa il ponte (X-08); il ponte manda `HX-Request` e
`HX-Current-URL` su ogni richiesta (`esegui`, `leggi`, `confermaAlbero`); «Ripristina» è nella lista chiusa delle rotte
(v1.1 §4.6).

## Rinviate, non accolte, facoltative

| R-FE | Che cosa chiedeva | Stato | Dove resta |
|---|---|---|---|
| R-FE-16 | «Conferma il documento» con la domanda di revisione (aggiungere o sostituire) | **rinviata**: fuori dalla v1.1 con il motivo scritto (U-08). La pagina manda una sola `voce`; il server rifiuta («no») e la scelta si fa nel Fascicolo; nel cassetto la nota e i correnti dello stesso tipo (caso e19, D-PF-16) | U-08, decisione dell'utente |
| R-FE-17 | `ConfermatoIl` come data | **non accolta**: resta una stringa, la pagina ne mostra la data | — |
| R-FE-18 | frammento del solo pannello di una casella | **facoltativa**: solo se nelle L7 la pagina con tutti i pannelli resi pesa troppo (RK-10, T-A1d-FE-05) | misura sulla `_run` |
| R-FE-19 | conteggio dei tipi per gruppo | **facoltativa**: solo se l'utente vuole di nuovo il conteggio (tolto con U-06) | decisione dell'utente |

## Contrasti di dominio, aperti (P-14)

Non li decide la pagina né il kit: la pagina mostra il comportamento di oggi del server, con il motivo, e non lo giudica.

1. Lo STEP si autorizza solo con un documento confermato del componente con quel file, contro DB7-2 A: «Autorizza» è
   spento con il motivo quando manca (D-RAC-02).
2. «Conferma il documento» rifiutata dal piano del Fascicolo legacy, contro «sempre possibile».
3. I blocchi del riepilogo dell'albero che la bozza vuota della v1 non scioglie: «Conferma l'albero» resta spento.
4. «Completa» e «pronto» con una copia sul NAS in errore: la pagina mostra la copia aperta a parte (caso e13).
5. `accetta` che accetta anche `commerciale`: la pagina non lo offre.
6. `nodo/{pid}/codice` che riscrive sempre la rev: la pagina lo dice sotto il modulo.

Restano aperti anche U-08, i dubbi D-PF-nn e D-RAC-nn dei piani e la ratifica delle sigle T-A1d e T-A1d-FE.
