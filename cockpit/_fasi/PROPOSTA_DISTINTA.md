# Proposta — la Distinta vista da chi la usa

**Stato: implementata in parte.** La schermata c'e' (`/thread/{id}/distinta`, vedi «Che cosa e' fatto» in fondo);
il mockup (`mockup_distinta.html`, si apre nel browser senza server e non salva niente) resta come riferimento. Qui:
che cosa non va oggi, che cosa si propone, che cosa serve nel backend e che cosa e' gia' fatto.

## Perimetro

Il mockup nasce da una RFQ reale aperta sul ramo `smistamento-giro3`, poi **anonimizzata**: codici,
cliente, buyer, cartella NAS e specifiche sono inventati. I disegni sono schizzi originali con
l'impaginazione di un disegno vero (elenco particolari, palline, cartiglio). Nessun PDF e nessun dato di
cliente entra in questo repository.

## Che cosa non va oggi

Visto aprendo la RFQ come la aprirebbe un operatore: un prodotto saldato, un particolare e tre dadi,
arrivati in uno zip con STEP, IGES e due PDF.

1. **La struttura resta chiusa.** Cinque STEP analizzati e nove nodi proposti, ma la Struttura BOM
   mostra solo il prodotto finché uno STEP non è autorizzato (Azioni sul componente › STEP autorizzato ›
   anteprima › conferma). L'editor si apre vuoto; la struttura dello STEP d'assieme sta nella «Guida dagli
   STEP», chiusa, in fondo.
2. **Lo stesso pezzo ha due o tre codici, e niente si aggancia da solo.**
   - Il dado in posizione 3 si chiama `3355400X1` nell'elenco del disegno, `3355400X` rev 1 nel nome del
     file `3355400X_1.STP` e `3355400X1_PRT` nel suo STEP.
   - La richiesta dice `5512300A1`, lo zip `5512300A_1` e la radice dello STEP d'assieme `5512300A`.
3. **File di un pezzo proposti sul prodotto.** `2244100X_1.IGS`, `2244100X_1.STP` e `1133200X_1.IGS`
   hanno la proposta con il codice del prodotto. Il primo risulta «Pronto»: «Conferma Fascicolo» lo
   scriverebbe sul NAS sotto il prodotto.
4. **Il linguaggio interno è a vista e i comandi sono sepolti.**
   - A vista: score, evidenze, autorità, guida, working, il testo della regex della famiglia.
   - I gesti stanno a due o tre livelli di `<details>`.
   - Per lo stesso lavoro ci sono due pagine, tre linguette, cinque viste di servizio, un cassetto e un
     piano in fondo.

## La proposta

Quattro linguette, nell'ordine del lavoro, con «Indietro» e «Avanti» fissi in fondo. Ogni linguetta
dice il suo stato («4 proposte da confermare», «sul NAS»).

1. **Richiesta.**
   - La mail, il prodotto, che cosa chiede il cliente (dalle regole dell'anagrafica).
   - A che punto sono gli altri passi.
2. **Distinta.**
   - In testa, l'«Analisi del prodotto»: che cosa ha letto il worker e **come ha deciso** la struttura:
     - un disegno con un elenco particolari è un assieme, uno senza è un particolare;
     - un dado senza 2D, con lo STEP del fornitore, è un particolare commerciale.
   - «Accetta la struttura proposta» conferma tutto; si corregge casella per casella.
   - Sotto, l'albero a tutta larghezza:
     - caselle grandi, con tipo, codice, nome e la miniatura del disegno;
     - un clic sulla miniatura apre il visore con le note (quello di oggi: pdf.js e `annotazione_pdf`);
     - «+ Assieme», «+ Particolare», «+ Particolare commerciale», «Elimina»;
     - per mettere un pezzo sotto un altro lo si trascina;
     - niente si scrive finché non si preme «Salva la distinta».
   - **Regola:** sotto un particolare e sotto un particolare commerciale non si mette niente.
3. **Documenti e NAS.**
   - Un blocco per pezzo, con le caselle 3D, 2D e DXF (obbligatorio o facoltativo).
   - Ogni file ha il suo motivo in chiaro, per esempio «`3355400X_1` = `3355400X1` · pos. 3 del disegno».
   - «Da sistemare» e «Messi da parte».
   - In fondo il controllo della completezza e «Conferma e copia sul NAS», che si abilita solo quando
     non manca niente di obbligatorio.
4. **Fattibilità** (anteprima della fase dopo).
   - Che cosa si produce e che cosa si compra, con le quantità totali.
   - Le lavorazioni lette dal disegno.
   - Qui di norma nascono gli assiemi interni.

Scelte di forma:

- Fondo bianco; colore solo per lo stato (fatto, da decidere, manca) e per il tipo di pezzo.
- Nessun punteggio a vista.
- Una conferma scritta nella pagina prima di ogni gesto che non torna indietro.

## Che cosa serve nel backend

- **1. L'elenco particolari del PDF d'assieme.**
  - Che cosa:
    - Il worker di analisi legge posizione, codice, quantità e denominazione dal testo con le
      coordinate dell'analisi 4 (`testo_pdf`).
    - Scende nei disegni che hanno a loro volta un elenco: quelli sono assiemi.
    - La proposta d'albero va nelle tabelle che ci sono già (`componente_proposta`,
      `relazione_proposta`), con una fonte nuova.
  - Dove: `workers/worker_analisi.py`, `contracts/`, `fascicolo.ApplicaStruttura`
  - Oggi: nuovo
- **2. Lo stesso pezzo sotto nomi diversi.**
  - Che cosa: `X_1` = `X1` = `X1_PRT`, e la radice dello STEP senza la cifra della versione. `_PRT` si
    può già dichiarare fra i suffissi decorativi del cliente; il resto no.
  - Dove: `classificazione` (`Canonico`), `fascicolo` (piano, candidati, `conAlias`)
  - Oggi: da fare
- **3. Niente sotto un particolare.**
  - Che cosa:
    - `ApplicaStrutturaVoluta` (passo 5) oggi trasforma un particolare in assieme quando riceve un
      figlio, e `Collega` e `Sposta` lo permettono.
    - Deve diventare un rifiuto, con la frase per l'operatore.
  - Dove: `fascicolo/voluta.go`, `fascicolo/modifiche.go`
  - Oggi: da cambiare
- **4. Un file con un codice suo non va sul prodotto.**
  - Che cosa: la proposta di un file dentro lo zip del prodotto prende il codice del prodotto invece del
    suo (punto 3 qui sopra).
  - Dove: `workerapi` (`propostaDaAnalisi`), il codice del contenitore
  - Oggi: da correggere
- **5. Assiemi interni.**
  - Che cosa: un codice automatico che si può cambiare (per esempio `<prodotto>-A01`), di norma in
    fattibilità. Oggi il codice di un componente è obbligatorio e lo scrive l'operatore.
  - Dove: `fascicolo`, `classificazione.CodiceAmmissibile`
  - Oggi: da decidere
- **6. I nomi dei tipi.**
  - Che cosa: «Particolare commerciale» per `commerciale`, «Particolare» per `sciolto`, «Assieme» per
    `sottoassieme`. Solo testo.
  - Dove: `fascicolo.NomeTipo`
  - Oggi: da fare

Si riusano così come sono:

- `GET /fascicolo/bom/dati` e `POST /fascicolo/bom/applica` (l'albero e il salvataggio in un gesto);
- tipo, sposta, collega, scollega, rimuovi, archivia;
- il visore pdf.js e le note `annotazione_pdf`;
- il fabbisogno per la completezza;
- «Conferma Fascicolo» con la copia sul NAS, e il congelamento.

## Non verificato

- Il mockup è stato controllato solo nella sintassi del suo script e guardato come pagina: nessuna prova L7.
- Il trascinamento funziona con il mouse. Da tastiera e su touch si usa il menu «Sotto» nel pannello
  della casella scelta.
- Le quantità, i tipi e le decisioni dell'«Analisi del prodotto» sono scritti a mano nel mockup: il
  worker che li produce non esiste ancora (punto 1).

## Che cosa e' fatto (29/09/2026)

- La Distinta in quattro passi (`internal/transport/web/distinta.go`, `web/templates/distinta.html`,
  `web/static/distinta.css`, `web/static/distinta.mjs`), con le rotte e i dati di sempre; la pagina della RFQ, la pagina
  Richieste e il Fascicolo portano li'.
- Punto 3 del backend: niente sotto un particolare (`collega`, `pianifica`, `MotivoTipoSpento`), e il particolare
  commerciale fra i tipi che l'operatore scrive (`TipiNuovo`).
- Punto 6: i nomi dei tipi («particolare commerciale»), anche nel Fascicolo.
- Punto 4: non era un difetto del backend. La proposta di un file non prende mai da sola il componente (la lettura del
  worker non lo scrive); i tre file erano stati assegnati al prodotto con un clic, in un menu che partiva gia' sul
  prodotto. Nella Distinta i menu partono vuoti e non offrono il pezzo a cui il file va gia'.
- La guida di uno STEP non autorizzato si puo' trasformare in pezzi scritti dall'operatore; l'autorizzazione dello STEP
  resta nel Fascicolo.

## Che cosa resta

- Punto 1 (l'elenco particolari del PDF d'assieme letto dal worker) e punto 2 (lo stesso pezzo sotto nomi diversi).
- Punto 5 (gli assiemi interni): la Distinta propone un codice interno modificabile per un assieme nuovo; manca la
  decisione su come si numerano.
- Gli archivi dentro un archivio non vengono aperti dalla preparazione: la Distinta li mostra fra i «Da sistemare».
- La denominazione di un pezzo nuovo si scrive dopo il salvataggio (la `StrutturaVoluta` non la porta).
- Le prove: quelle del ramo `-qa` sulla regola del tipo sono da aggiornare (vedi la descrizione della PR); nessuna prova
  L7 della Distinta.
