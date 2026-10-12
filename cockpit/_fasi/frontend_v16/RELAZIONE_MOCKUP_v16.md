# Relazione sul mockup v16 della Distinta

> ## ⚠ Mockup da visionare e collaudare da Stefano: non è ancora approvato; non va implementato prima della sua verifica e del suo collaudo.

*12/10/2026 · per Pietro · cartella `cockpit/_fasi/frontend_v16/` · base `smistamento-giro5` a `f864b48`.*

**Tutti i dati sono di fantasia (ACME).** Clienti, fornitori, persone, codici, nomi di file e indirizzi sono inventati. Tariffe, €/kg, prezzi dei terzisti, ricarichi e totali sono **valori di esempio**: la logica di calcolo è quella vera, i numeri no. Le persone dei ruoli sono Lino (ufficio tecnico), Franco (commerciale), Fabio (acquisti) e Marta (produzione).

---

## 1. Che cosa è e che cosa non è

**È:**
- un mockup del frontend del Cockpit RFQ, in un solo file HTML: `cockpit-frontend-mockup-v16.html`;
- i suoi pezzi in `sorgenti/`, con lo script che li ricompone e cinque prove automatiche (jsdom);
- una proposta di come la Distinta (Struttura, Come, Ciclo, Sequenza, Esito) e l'Offerta si appoggiano al tuo backend.

**Non è:**
- codice da mettere in produzione: il JS del mockup simula il server, non lo sostituisce;
- una modifica del backend. **Nessun file esistente del repository cambia.** Ci sono solo file nuovi, tutti in questa cartella;
- una decisione presa: le parti «Proposta · dopo A2 · R-xx» sono proposte, e le decisioni aperte sono al §8.

**Il binario non cambia.** `embed.go` incorpora solo `migrations/`, `web/templates/`, `web/static/` e `workers/`; `_fasi/` resta fuori. Niente Go, niente migrazioni, niente template.

---

## 2. Le zone

| Zona | Che cosa | Regola nel mockup |
|---|---|---|
| **A** | Struttura e documenti: la pagina A1d (`motore_a`). Dati e gesti veri. | Identica al contratto A1d v1: rotte, i 29 POST della lista chiusa più «Conferma l'albero», le 4 GET, le tre conferme distinte, le frasi fisse e le frasi di rifiuto, le ancore. Il finto server A (`17_server_a.js`) imita il contratto. |
| **B** | Lavorazione: Come, Ciclo, Sequenza, Esito, richieste ai fornitori, Costi, Offerta. Il backend non l'ha ancora. | Ogni parte porta la fascia «Proposta · dopo A2 · R-xx» con il numero della richiesta. Il finto server B (`18_server_a2.js`) scrive con rotte immaginate e un gesto per cosa. |
| **C** | Quello che non c'è e non si propone ora. | Il pulsante c'è, spento, con il motivo. |

L'unico cambiamento in zona A è **l'ordine visivo** della stazione Struttura (§4). Gesti, testi e significato restano.

---

## 3. Le nove richieste di Stefano e che cosa fa la v16

| # | Richiesta | Che cosa fa la v16 | Zona | Richieste a te |
|---|---|---|---|---|
| 1 | Solo tema chiaro | Tolti i token scuri, il pulsante del tema e la sua prova. `color-scheme: light`, come `motore_a.css`. Contrasto controllato su 63 schermate. | A e B | nessuna |
| 2 | Documenti prima della distinta | **Una pagina sola**, come il tuo workspace: in alto Documenti (R9) e «Da smistare» (R8); sotto Analisi (R3), Conferma dell'albero (R4), albero (R5) e conflitti (R7); a destra l'anteprima (R6). I link «Documenti» e «Distinta» scorrono all'ancora. Dopo «Conferma l'albero» la pagina resta dov'è. | A (solo ordine) | una modifica del template (§4) e due frasi del contratto |
| 3 | «Come» va bene | Resta com'è. La minuteria è spenta con il motivo (R-08). Il primario è «Apri le richieste da preparare ✉ (N)». | B | nessuna |
| 4 | PDF del prodotto finito per primo nel Ciclo | La colonna Disegno apre sulla scheda «Prodotto finito», con l'`AllegatoID` e lo `sha256` della voce 2D del prodotto. Mai un disegno inventato: senza PDF, «anteprima non disponibile». | dato A, posto B | nessuna |
| 5 | Sequenza scritta a mano; pre-unioni nel Ciclo e nella saldatura | La proposta è solo un **seme**. Lino aggiunge, toglie, rinomina, riordina e sposta i figli fra le fasi; il server non gli rifà il lavoro. La sequenza confermata è di ogni prodotto. Una pre-unione è una **fase** di unione del padre con una parte dei figli, mai un nodo della BOM. «Ripristina la proposta» è una rotta sola, con la firma. | B | R-10, R-32, R-35, R-36 precisate |
| 6 | Albero del processo | La Sequenza **è** l'albero del processo: stesso aspetto e stessi tasti dell'albero di Struttura, ma un componente separato (non sta in `#ma`, non usa `data-ma-*`). Il ramo si legge dal basso. Linee tratteggiate finché la sequenza non è confermata. | B (sola lettura) | nessuna nuova |
| 7 | Richiedere la maschera e le altre preparazioni | 8 tipi più «altra». «Richiedi» su ogni riga; «Fornitore» prepara la bozza con la tua rotta `POST /thread/{id}/richiesta` (una per fornitore e lavorazione, `lavorazione = 'attrezzature'`); «Interno» prepara una domanda all'attrezzeria. Ogni maschera ha la sua fase. | B | R-11 precisata, R-13 con le settimane |
| 8 | Meno caos, stesse funzioni | Pulsanti visibili da 539 a 482. Un primario per stazione, i secondari nel «⋯», un «?» per stazione, una fascia B per stazione. Nessuna funzione tolta. | A (poco) e B | nessuna |
| 9 | Fattibile e rispettoso del backend | Usa il Blocco 7B così com'è: rotte, stati della `0016`, frasi di rifiuto. Tolte le rotte immaginate al posto delle tue (`manda`, `scade`, `/rfq/{id}/richiesta`, `/registra`). Nessuna tabella nuova oltre a quella di R-11, nessun motore nuovo. | tutte | nessuna obbligatoria |

---

## 4. La modifica di template proposta a `motore_a.html` e la rilettura di DB7-2

**Che cosa.** Nel workspace della stazione ① il mockup mette i documenti in alto. Nel tuo codice vuol dire **spostare il markup** di R8 («Da smistare») e R9 («Documenti e NAS») **prima** di R3, dentro `ma_workspace`.

**Che cosa resta uguale:**
- rotte, gesti, guardie, testi fissi e prove;
- le tre conferme in tre posti, con tre testi; «Conferma il documento» resta una `voce` alla volta, con la domanda e la frase DB7-2 quando la BOM non è verificata;
- «confermato» e «copiato» in due celle diverse; «copiato» vuol dire solo `StatoNas == scritto`;
- le ancore `#ma-documenti`, `#ma-da-smistare`, `#ma-fonte`, `#ma-albero`, `#ma-schede-albero`;
- la prova d'ordine che c'è: `#ma-albero` prima di `#ma-schede`;
- i valori di `vista` e il default `distinta` del server. Nel mockup cambia solo l'ordine dei link in `nav#ma-viste`.

**Che cosa non si fa:** invertire l'ordine solo con il CSS (`order:`). Tastiera e lettori di schermo leggerebbero un altro ordine: si sposta il markup.

**La rilettura di DB7-2 (decisione S-01 di Stefano, da scrivere).** Proposta di formula: «DB7-2 A, ordine visivo: i documenti si mostrano per primi. Il significato non cambia: la copia sul NAS parte alla conferma di ogni voce, con la domanda; struttura e codici si sistemano prima di confermare i documenti incerti.»

**Perché va d'accordo con il backend.** Oggi lo STEP strutturale si autorizza solo se è già un documento confermato del componente. Con i documenti in alto, la pagina e il server vanno nella stessa direzione.

**Dopo la decisione:** due frasi del contratto A1d da correggere (v1, riga 438: «La scheda "Documenti" viene dopo "Distinta"»; delta v1.1, riga 127: «R9 sta in fondo»). Nessuna riga di codice e nessuna prova da cambiare.

---

## 5. Il modello dati minimo della zona B

Nessuna tabella nuova oltre a quella di R-11. Nessun numero calcolato nel browser che diventi un dato.

| Che cosa | Tabelle o campi | Richiesta |
|---|---|---|
| ciclo, fasi, chi entra in una fase e quanti | `ciclo` (con `stato` e `fonte`), `fase_ciclo` (`ordine`), `fase_componente` | R-10 |
| pre-unioni in fila | niente in più: più fasi di unione nello stesso ciclo del padre | R-10 |
| sequenza scritta a mano | `ciclo.fonte_sequenza` (proposta o scritta) | R-10 precisata |
| stato della sequenza per prodotto | prodotto, stato, firma, chi, quando; `POST /prodotto/{pid}/sequenza/conferma` e `…/ripristina` con la firma | R-32 precisata |
| un gesto per cosa nella Sequenza | `fase/{fid}` (ordine, riferimento), `fase/{fid}/componente` (quantità obbligatoria e vuota) | R-36 |
| preparazioni (maschere, set, stampi…) | 1 tabella, `attrezzatura_componente`: componente, fase facoltativa, tipo, stato, costo, settimane, fonte, nota | R-11 precisata |
| richiesta della preparazione | `richiesta_fornitore` com'è, `lavorazione = 'attrezzature'`; il legame con la preparazione si legge da lavorazione, codici e note (niente FK) | c'è già (0015, 0016) |
| risposta del fornitore | righe codice × lotto × €/pz, materiale dichiarato, data del giro, settimane, allegato | R-13 precisata |
| «da proteggere» | `POST /fase/{fid}/proteggi`: le fasi le sceglie il server | R-33, o una colonna di `fase_componente` |
| componente aggiunto a mano senza STEP | nessuna tabella: `InsertComponente` con origine `manuale`, `ControllaCodiceNuovo`, `Collega`; esce in `confermati_senza_step` | R-38 precisata |
| albero del processo | una lettura di R-10 | R-32, R-37 |
| PDF del prodotto nel Ciclo | niente: `GET /allegato/{AllegatoID}/anteprima?v=<sha256>` c'è già | — |

### 5.1 Le richieste R-xx: riusate, precisate, nuove

| R | Stato | Che cosa serve |
|---|---|---|
| R-10 | riusata, precisata | più fasi di unione nello stesso ciclo (la pre-unione); `ciclo.fonte_sequenza`; con fonte «scritta» il server non ripropone il ciclo |
| R-10, R-36 | precisata | `ciclo/conferma` con la firma di quello che si è letto |
| R-11 | precisata | la tabella delle preparazioni; niente FK verso `richiesta_fornitore` |
| R-13 | precisata | la risposta per codice e per scaglione, il materiale dichiarato (se diverso dal disegno: MAT-05, tenuta con riserva, fuori dalla scelta automatica), la data del giro, le settimane, l'allegato. Nessun numero nuovo |
| R-17 | precisata | nuovo giro sulla stessa riga: l'indice UNIQUE (RFQ, fornitore, lavorazione) non permette una seconda riga |
| R-27 | precisata | la scadenza della richiesta la calcola il server |
| R-28, R-29, R-41 | precisate | domande interne con la stessa forma delle richieste: bozza in Outlook con il marcatore; stati bozza · inviata · risposta · annullata |
| R-32 | precisata | stato della sequenza per prodotto; ripristino con la firma |
| R-33 | precisata | «da proteggere» con `/fase/{fid}/proteggi` |
| R-35 | precisata | righe nuove nel registro: PREP-04 (avviso), il seme ricalcolato |
| R-38 | precisata | «Aggiungi un componente sotto un padre» anche senza STEP: codice, rev, Q.tà, tipo. Il codice lo scrive una persona, mai precompilato. Due domande al §8 |
| R-42 | **nuova, facoltativa, non usata nella v16** | pre-unioni in parallelo: `fase_componente.da_fase_id`, `fase_ciclo.su_fase_id` (due colonne nullable) |
| R-43 | **nuova, facoltativa, rinviabile** | la tabella distinta del disegno 2D come struttura proposta. Solo se Stefano riapre R68: oggi la gerarchia viene solo dallo STEP |
| — | domanda | il testo del cartiglio (denominazione, rev, materiale, massa) esce già nell'esito del motore A? |

Scartata: `bom/applica` nella lista del ponte. Va contro DB7-5 A.

---

## 6. La prova da zero su un caso di media complessità

**Il caso** (anonimizzato): un **tirante saldato a 3 componenti con terzisti e scaglioni di lotto**.
- Assieme saldato con tre figli: un tubo (lo tagliamo noi a sega), una boccola (tornitura da terzi), una forcella (fresatura da terzi).
- Il cliente manda solo PDF: nessuno STEP.
- Due terzisti: uno risponde in due giri; l'altro quota un materiale diverso dal disegno.
- Due lotti: prima 20 pezzi, poi anche 5. Offerta rev 0 e rev 1.
- La mail del cliente è fra i messaggi nascosti del mockup (cliente non ancora censito).

**Come è stata fatta.** Due volte da zero, solo dall'interfaccia: dalla mail fino alla rev 1 dell'offerta e alla sua bozza di mail. La prima volta con la prova `test_tirante.cjs` (163 gesti); la seconda a mano, da un collaudatore nuovo, senza la prova (166 gesti). Errori JS: 0.

| Tratto (riprova a mano) | Gesti |
|---|---|
| Inbox: censisci il cliente, autorizza, crea la richiesta e l'RFQ | 16 |
| Struttura: prepara i file, 4 conferme, 3 componenti aggiunti a mano (R-38), albero | 27 |
| Come e Ciclo (4 componenti) | 33 |
| Richieste: 3 bozze, invio, 5 risposte in 2 giri | 42 |
| Sequenza, Esito, consegna del lotto 20 | 16 |
| Franco: foglio, costi, offerta rev 0, mail | 14 |
| Lotto 5: nuova consegna di Lino, costi, rev 1, mail | 18 |

### 6.1 Rilievi trovati e corretti (30 della prima prova, più la riprova)

**I bloccanti, risolti in modo conforme:**
- **RL-01 · Senza STEP la struttura non si faceva.** Zona B, R-38: una striscia «Proposta · dopo A2 · R-38» in Struttura. Codice, rev, Q.tà e tipo li scrive Lino; il PDF si apre accanto. La gerarchia resta «da verificare · fonte_non_confermata».
- **RL-02, RL-03 · I confermati senza STEP.** Il finto server A ora riempie `confermati_senza_step` nella forma del contratto (E4); tolto un blocco «prima autorizza lo STEP» che il riepilogo vero non ha. La zona B li legge.
- **RL-04 · Le risposte vere non si registravano.** Modulo «Registra la risposta del fornitore» per codice × lotto × €/pz, con materiale, tempi, data del giro e allegato (R-13). I giri precedenti restano.

**Gli altri, chiusi:**
- RL-05: i PDF dei figli vanno in «Da smistare» (`nessun_candidato`); la testata dice «perimetro aperto».
- RL-06: «Censisci come cliente» dalla mail (la rotta c'è già); i nascosti dello stesso dominio passano ad «Autorizza».
- RL-07, RL-08: la scelta automatica toglie le quotazioni sotto il lotto minimo, storiche o con il materiale discorde (MAT-05). Senza quotazioni: «manca · scegli tu», mai un prezzo basso.
- RL-09, RL-10: ogni consegna ha la sua chiave dei costi; i lotti si sommano. All'emissione l'offerta si fotografa; le revisioni passate si aprono in sola lettura; da rev 1 «Annulla e sostituisce…».
- RL-11 … RL-20, RL-23, RL-25 … RL-28: allegati e lotti giusti nella richiesta, modelli di «Come» per tondo, profilato e tubo, qualità e prezzo del tubo, storico separato dai giri nuovi, trasporti con il ricarico, avviso del margine sotto l'obiettivo, «NO · solo 2D» senza 3D, «Trattamento: nessuno», fornitori delle richieste nei Terzisti della fase, la mail al cliente come «bozza in Outlook», orologio del mockup.

**Trovati nella riprova e corretti:**
- **N6, difetto alto:** dopo «Emetti», un prezzo cambiato nei Costi cambiava l'offerta emessa, con lo stesso numero e la stessa data. Ora l'offerta emessa si legge dalla sua fotografia, con l'avviso «costi cambiati dopo l'emissione: crea una revisione».
- N1: nel modulo del cliente nuovo «Aggiungi» mittente cancellava i campi già scritti.
- N2: i componenti aggiunti a mano dicevano «Entra: radice: nessun padre»; ora mostrano il padre confermato.
- N3: gli allegati della richiesta non si vedevano.
- N4: la richiesta della maschera prometteva un 3D che non c'era.
- N5, N7, N8, N9: «niente da chiedere» al posto di «non la fa»; la riga dei tempi nella revisione in sola lettura; «1 mancante» al singolare; una quotazione per riga nel confronto dei terzisti.

### 6.2 Rilievi aperti

| Rilievo | Gravità | Che cosa |
|---|---|---|
| N10 | media · decisione | Le richieste sono una per fornitore **e per lavorazione**, come il tuo backend: il terzista che fa fresatura e tornitura riceve due mail. Va bene, o serve una richiesta per fornitore con più lavorazioni? |
| N11 | media | Se Lino apre le richieste da «Come» prima del Ciclo, la nota esce senza materiale, conto vendita e lotti della fase. Proposta: dire nella carta quali campi del Ciclo mancano. |
| N13 | media | La testata della fase e il Foglio per Franco mostrano un solo «€/pz» (il primo prezzo dell'ultimo giro), anche nel foglio dell'altro lotto. I Costi sono giusti. |
| N12, N14, N15, N16, N17 | basse | lotti della testata nella carta; riepilogo dell'albero «0 componenti nuovi» dopo R-38 (domanda 1 di R-38); «Nuovo giro» senza un campo per dire che cosa cambia; confronto dei terzisti stretto a 1440 px; codici abbreviati alle ultime 4 cifre ambigui |
| RL-22 | aperto | la domanda sul cartiglio (§5.1) e un campo per il codice del cliente (oggi resta solo nel titolo) |
| RL-29, in parte | basso | tasti del controllo dimensionale; scheda «Componente» tagliata a 1440 px con il cassetto aperto |

**I Costi del caso** (valori di esempio, ricarichi 4/8/8/6 %, obiettivo 35 %): lotto 20 → 41,48 €; lotto 5 → 58,16 €, prezzo calcolato 89,47 → 89,50 €. A 60,00 € il lotto 20 ha margine 30,9 %, sotto l'obiettivo: compare l'avviso. Con la lunghezza scritta da Lino (5,19 kg di tubo) i Costi danno 41,47 € e 58,14 €; la prova scrive 5,2 kg a mano. Non è un difetto.

---

## 7. I numeri

| Misura | Valore |
|---|---|
| `test_clic.cjs` | 437 superate, 0 fallite, 0 errori JS |
| `test_v16_c1.cjs` | 192 superate, 0 fallite, 0 errori JS |
| `test_v16_c2.cjs` | 215 superate, 0 fallite, 0 errori JS |
| `test_tirante.cjs` | 96 superate, 0 fallite, 163 gesti, 0 errori JS |
| `conta_passi.cjs` | OK |
| Pulsanti visibili, 13 schermate a 1440 px | 482 (erano 539) |
| Gesti del percorso del telaio skid, senza preparazioni | 59 (bersaglio ≤ 64) |
| Gesti dello stesso percorso con la maschera | 63 (bersaglio ≤ 69) |
| Caso 1, sequenza a mano | 12 (≤ 13) |
| Caso 2, tre livelli con un sottoassieme vero | 10 (≤ 10) |
| Prova del tirante da zero, a mano | 166 gesti fino alla rev 1 |

Le prove girano su questa copia ACME. Sono le stesse della copia di lavoro, adattate solo con la stessa tabella di sostituzione dei dati: stesso numero di prove, stesse condizioni.

---

## 8. Decisioni aperte

**Per Stefano:**
1. **S-01:** conferma per iscritto la nuova lettura di DB7-2 (documenti in alto, stessa pagina) e la modifica del template (§4).
2. Una RFQ solo 2D resta «non pronta» in zona A (K-02) mentre la zona B va avanti con la presa d'atto? Si riapre R68 per R-43?
3. Le pre-unioni in parallelo (R-42) servono già, o bastano quelle in fila?
4. 1024 px è una larghezza reale in ufficio? Da lì dipendono le colonne della Struttura e dell'Inbox.

**Per te (dentro R-38), con il default del mockup:**
5. «Conferma l'albero» con i soli confermati senza STEP è confermabile? *Default:* sì; la gerarchia resta «da verificare».
6. Un prodotto solo 2D non diventa mai «pronto» (K-02). Basta la deroga del fabbisogno `cad_3d`? *Default:* la zona A resta «non pronto».

**Per l'ufficio (Lino, Franco, Fabio):**
7. Domande interne: R-28 e R-41 con i loro stati (scelta del mockup), oppure messaggi senza stato?
8. Una seconda richiesta allo stesso fornitore per la stessa lavorazione passa da R-17 (nuovo giro). Va bene?
9. Minuteria spenta ovunque: si anticipa R-08 nella zona A?
10. Ripristino della sequenza: restano le unioni nate per prime. «Togli» su una pre-unione: le fasi che ci lavoravano si tolgono anche lì?
11. «Mai acquistato» vale DUBBIO o solo un avviso? Il 3D è un fabbisogno per questo cliente? Il pezzo si consegna grezzo?
12. Chi registra le quotazioni (Fabio nel cassetto o Franco nei Listini)? Una quotazione con il materiale discorde si scarta o si tiene con riserva? I lotti dei terzisti stanno solo nella fase? Chi firma l'offerta?
13. N10: una richiesta per fornitore e lavorazione, o una per fornitore con più codici e lavorazioni?

---

## 9. Come aprirlo e come lanciare le prove

**Aprire il mockup:** `cockpit-frontend-mockup-v16.html` si apre con un browser, senza server e senza rete (salvo i font di Google). Il ruolo si cambia dal menu ≡ › «Vedi come».

**Lanciare le prove** (Node 18 o più recente):
```
cd cockpit/_fasi/frontend_v16/sorgenti
npm install            # scarica jsdom; node_modules/ è in .gitignore
npm test               # le cinque prove, una dopo l'altra
```
Oppure una alla volta: `node test_clic.cjs`, `node test_v16_c1.cjs`, `node test_v16_c2.cjs`, `node test_tirante.cjs`, `node conta_passi.cjs`. Se manca memoria: `node --max-old-space-size=768 <prova>`.

**Rifare il file unico dai pezzi:** `sh assembla.sh` (scrive `../cockpit-frontend-mockup-v16.html`).

**Le foto della prova del tirante** (facoltative): `FOTO=1 node test_tirante.cjs` salva le fotografie statiche del DOM in `sorgenti/foto/` (anche questa in `.gitignore`).

**Nota sulle schermate.** Nelle vecchie schermate un blocco di pixel colorati copriva la testata in alto a sinistra. Veniva dallo strumento di screenshot (un canvas di controllo aggiunto alla pagina), non dal mockup: le foto nuove sono pulite.
