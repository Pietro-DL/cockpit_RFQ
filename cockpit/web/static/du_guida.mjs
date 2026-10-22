// du_guida.mjs — la guida interattiva e gli aiuti «(?)» della Distinta unica (frontend del 22/10 [U]).
//
// Che cosa fa: spiega la pagina, non la cambia. Due cose:
//  - le icone «(?)»: per le funzioni principali di Sorgente e Revisione (catalogo AIUTI qui sotto) un'icona piccola
//    accanto all'elemento; al passaggio del mouse o con il fuoco da tastiera compare un fumetto con una spiegazione breve.
//    Le icone stanno in uno strato fisso fuori dalla pagina (#du-aiuti, figlio di <body>) e si posizionano sopra
//    l'angolo dell'elemento: non entrano nel flusso, non cambiano altezze, testi, nomi accessibili o ordine del fuoco
//    della pagina. Si ricalcolano dopo ogni ridisegno (MutationObserver), allo scorrimento e al ridimensionamento;
//    l'interruttore «Mostra aiuti (?)» nella testata le spegne, e la scelta resta nella scheda del browser;
//  - la «Guida»: un percorso a passi (uno per la Sorgente, uno per la Revisione, secondo la scheda aperta) che evidenzia
//    l'elemento vero con un contorno luminoso e lo spiega in un fumetto compatto accanto a lui; la pagina non si vela e
//    resta leggibile intorno (seconda richiesta del 22/10 [U]); se l'elemento in quello stato non c'è, il passo dice che
//    cosa serve per vederlo.
//
// Regole: nessuna chiamata al server (niente ponte, niente fetch), nessun gesto della pagina, nessuno stato della pagina
// letto o scritto se non dal DOM; i testi dicono che cosa fa davvero la pagina, senza chiavi interne né gergo. Tutto il
// DOM con createElement e textContent. Lo stile sta in distinta_unica.css, blocco «Guida e aiuti».

const CHIAVE_MEMORIA = "cockpit.guida.aiuti";

// ---------------------------------------------------------------- il catalogo degli aiuti «(?)»
// sel: il selettore CSS dell'elemento; testo: (facoltativo) un filtro sul testo dell'elemento; la prima corrispondenza
// visibile riceve l'icona. Le spiegazioni dicono che cosa fa la pagina, non che cosa fa il server dietro.

export const AIUTI = [
  // testata e schede
  { k: "prodotti", sel: "#du-prodotti .du-ptab", t: "Prodotti della RFQ",
    s: "Ogni pulsante è un prodotto della richiesta. Scegline uno per vedere i suoi file, la sua sorgente e la sua bozza: la BOM si costruisce un prodotto alla volta." },
  { k: "passo-sorgente", sel: "#du-passo-sorgente", t: "Scheda Sorgente",
    s: "Primo passo: i file arrivati con la RFQ. Qui scegli lo STEP da cui nasce la struttura del prodotto e smisti gli altri file (associarli, metterli in Altri file, scartarli)." },
  { k: "passo-revisione", sel: "#du-passo-revisione", t: "Scheda Revisione",
    s: "Secondo passo: la bozza della distinta (BOM) del prodotto. Qui correggi pezzi, tipi e posizioni e alla fine confermi la BOM." },
  { k: "aggiorna", sel: "#du-aggiorna", t: "Aggiorna",
    s: "Rilegge dal sistema i dati della RFQ, dei file e della bozza. Non cambia niente: serve quando qualcun altro ha lavorato sulla stessa RFQ." },
  { k: "menu-rfq", sel: "#du-menu-rfq > summary", t: "Azioni della RFQ",
    s: "Gli atti che valgono per tutta la RFQ: preparare i file nuovi, rifare l'analisi, riprovare le copie sul NAS, il riepilogo e i limiti di questa versione." },

  // Sorgente
  { k: "sg-cerca", sel: "#du-sg-cerca", t: "Cerca nei file",
    s: "Filtra l'elenco per nome del file o codice. Scorciatoia: il tasto «/» porta qui." },
  { k: "sg-cartelle", sel: "#du-sg-albero", t: "Cartelle dei file",
    s: "I file della RFQ raggruppati per stato: da controllare, STEP, per prodotto, Altri file, scartati, tutti. Il numero accanto dice quanti file ci sono." },
  { k: "sg-c-dc", sel: "#du-sg-c-dc", t: "Da controllare",
    s: "I file che aspettano una decisione: senza proposta, ancora in analisi, in attesa di un prodotto, archivi ricevuti." },
  { k: "sg-c-step", sel: "#du-sg-c-step", t: "STEP della RFQ",
    s: "I modelli 3D (STEP) letti, con le candidature: per ogni prodotto, quale STEP potrebbe esserne la sorgente." },
  { k: "sg-c-prodotti", sel: "#du-sg-c-prodotti", t: "Cartelle dei prodotti",
    s: "Una cartella per prodotto: la sua sorgente e i file associati ai suoi pezzi." },
  { k: "sg-c-altri", sel: "#du-sg-c-altri", t: "Altri file",
    s: "I file che non entrano nella BOM ma vanno conservati: la loro copia sta nella cartella ALTRO della RFQ." },
  { k: "sg-c-scartati", sel: "#du-sg-c-scartati", t: "Scartati",
    s: "I file tolti dal lavoro. Resta la traccia di chi li ha scartati e quando." },
  { k: "sg-c-tutti", sel: "#du-sg-c-tutti", t: "Tutti i file",
    s: "L'elenco completo dei file attivi della RFQ, in qualunque stato." },
  { k: "sg-riga", sel: "#du-sg-elenco [role=row][id^='du-file-'], #du-sg-elenco li[id^='du-step-']", t: "Riga di un file",
    s: "Un clic apre il dettaglio a destra; la casella a sinistra lo spunta per un gesto su più file insieme. Le etichette colorate dicono lo stato." },
  { k: "st-candidata", sel: "#du-corpo .du-st", testo: /candidata per/i, t: "STEP candidato",
    s: "Il sistema propone questo STEP come possibile sorgente del prodotto indicato. È solo una proposta: decidi tu dopo aver guardato l'anteprima." },
  { k: "st-proposto", sel: "#du-corpo .du-st", testo: /^\W*proposto\b/i, t: "Proposto",
    s: "Il sistema propone dove va questo file (a quale pezzo). Non è ancora nella bozza: lo diventa quando lo salvi o lo accetti." },
  { k: "st-preassociato", sel: "#du-corpo .du-st", testo: /salvato nella bozza|preassociato/i, t: "Preassociato",
    s: "Il file è collegato a un pezzo nella bozza. Diventa definitivo solo quando la BOM viene confermata." },
  { k: "st-confermato", sel: "#du-corpo .du-st", testo: /sorgente confermata|^\W*confermat[oa]\b|già registrato/i, t: "Confermato",
    s: "Una decisione già registrata: la sorgente confermata del prodotto o un legame già presente nella BOM confermata." },
  { k: "guarda", sel: "[id^='du-guarda-']", testo: /Guarda l.anteprima/, t: "Guarda l'anteprima",
    s: "Mostra la struttura che il sistema legge in questo STEP per quel prodotto (assiemi, pezzi, quantità), prima di sceglierlo. Guardare non cambia niente." },
  { k: "conferma-sorgente", sel: "[id^='du-conferma-sorgente']", t: "Conferma questo STEP come sorgente",
    s: "Sceglie questo STEP come sorgente del prodotto: dalla sua struttura nasce la bozza della BOM da rivedere. Non conferma la BOM." },
  { k: "revoca-sorgente", sel: "#du-revoca-sorgente", t: "Revoca come sorgente",
    s: "Toglie a questo STEP il ruolo di sorgente del prodotto. Prima di farlo la finestra mostra che cosa succede alla bozza." },
  { k: "altri", sel: "[id^='du-altri-']", t: "Metti in Altri file",
    s: "Il file non entra nella BOM ma si conserva: la sua copia va nella cartella ALTRO della RFQ. Si può riattivare." },
  { k: "scarta", sel: "[id^='du-scarta-']", t: "Scarta",
    s: "Toglie il file dal lavoro. Prima la finestra elenca che cosa dipende da lui; resta comunque la traccia del gesto." },
  { k: "sg-aggiungi", sel: "#du-sg-aggiungi", t: "+ Aggiungi file",
    s: "Carica file nuovi nella RFQ dal tuo computer. Ogni file viene poi analizzato e compare nella cartella giusta." },
  { k: "sg-dettaglio", sel: "#du-sg-dettaglio", t: "Dettaglio del file",
    s: "Tutto sul file scelto: stato, candidature, a quale pezzo è associato, e i gesti possibili su di lui." },
  { k: "sg-mossa", sel: "#du-sg-mossa", t: "Ultima mossa",
    s: "L'ultimo gesto che hai fatto sui file, con l'esito. «Vai al file» lo ritrova; «Annulla la mossa» lo disfa finché non ne fai un'altra." },
  { k: "vai-revisione", sel: "#du-vai-revisione", t: "Vai alla Revisione",
    s: "Passa alla scheda Revisione del prodotto scelto, dove si modifica la bozza della BOM." },

  // Revisione
  { k: "vista-org", sel: "#du-vista-org", t: "Organigramma",
    s: "La bozza disegnata come un albero di schede: il prodotto in alto, sotto i suoi assiemi e pezzi, con le quantità sui legami." },
  { k: "vista-elenco", sel: "#du-vista-elenco", t: "Elenco",
    s: "La stessa bozza come tabella: utile per cercare, filtrare e accettare le proposte una riga alla volta." },
  { k: "br", sel: "#du-br-chiudi", t: "Browser della distinta",
    s: "Sopra l'organigramma, l'albero compatto di assiemi, pezzi e documenti, come il browser degli assiemi di un CAD. Un clic qui lo apre o lo richiude; la maniglia in basso a destra lo allunga." },
  { k: "br-espandi", sel: "#du-br-espandi", t: "Espandi e comprimi",
    s: "«Espandi tutto» apre tutti i rami dell'albero, «Comprimi tutto» li richiude. Cambia solo il browser, non la bozza." },
  { k: "br-filtro", sel: "#du-br-filtro", t: "Filtra il browser",
    s: "Scrivi parte del codice o della descrizione: restano le righe che corrispondono e i loro padri. Esc cancella il filtro." },
  { k: "br-albero", sel: "#du-br-albero", t: "Righe del browser",
    s: "Ogni riga è un pezzo, con icona del tipo, codice, descrizione, quantità e stato; sotto ci sono i suoi file. Clic per sceglierlo nel pannello del nodo, doppio clic o Invio per centrarlo sull'organigramma. Frecce per muoversi, destra e sinistra per aprire e chiudere." },
  { k: "proposta-step", sel: "#du-proposta-step", t: "Proposta dello STEP",
    s: "Confronta la bozza con la struttura letta nello STEP sorgente. «Importa dalla proposta…» prende solo gli elementi che scegli; «Riparti dalla proposta…» crea una bozza nuova da capo." },
  { k: "nuovo", sel: "#du-nuovo-ass", t: "Aggiungi un pezzo",
    s: "«+ Assieme», «+ Particolare» e «+ Commerciale» aggiungono alla bozza un pezzo nuovo del tipo scelto, sotto il nodo selezionato." },
  { k: "confronto", sel: "#du-confronto", t: "Confronto con la proposta",
    s: "Mette in evidenza sulle schede che cosa è diverso fra la bozza e la proposta del sistema." },
  { k: "segnalati", sel: "#du-segnalati-apri", t: "Nodi segnalati",
    s: "I pezzi su cui il sistema ha un dubbio (per esempio un possibile elemento CAD da non includere). Li apri in elenco e li decidi uno per uno o insieme." },
  { k: "zoom", sel: "#du-zoom-adatta", t: "Mostra tutto",
    s: "Centra l'organigramma e lo rimpicciolisce quanto basta per vederlo tutto (tasto 0). «Centra» porta al centro il pezzo scelto; − e + cambiano lo zoom." },
  { k: "schermo", sel: "#du-zoom-schermo", t: "Schermo intero",
    s: "Apre l'organigramma a tutta finestra: cerca un pezzo per codice o descrizione, clic su una scheda per vedere padri, figli e documenti. Esc torna alla pagina dov'era." },
  { k: "tela", sel: "#du-vp", t: "Organigramma della bozza",
    s: "Clic su una scheda per sceglierla e vederla nel pannello del nodo. Il bordo e i colori dicono il tipo e lo stato; le pillole sui legami dicono la quantità." },
  { k: "scheda", sel: "#du-vp .du-nc", t: "Scheda di un pezzo",
    s: "Codice, revisione e miniatura del disegno 2D del pezzo. Clic per sceglierla; la casellina in alto a destra la seleziona per un gesto su più pezzi." },
  { k: "oq", sel: "#du-vp .du-oq", t: "Quantità sul legame",
    s: "Quante volte il pezzo sta nel padre, e da dove viene il numero (dal file, deciso da te, confermato). Clic per cambiarla." },
  { k: "elenco", sel: "#du-elenco", t: "Elenco dei pezzi",
    s: "Tutti i pezzi della bozza in tabella, con filtri e ricerca. Da qui accetti le proposte dei file o escludi più pezzi insieme." },
  { k: "pannello", sel: "#du-pannello", t: "Pannello del nodo",
    s: "Tutto sul pezzo scelto: identità proposta e decisa, tipo, posizione, documenti, provenienza, e i gesti che puoi fare su di lui." },
  { k: "ling-dettaglio", sel: "#du-ling-dettaglio", t: "Dettaglio nodo",
    s: "La linguetta con l'identità del pezzo, i gesti sulla bozza e le sezioni di dettaglio." },
  { k: "ling-documenti", sel: "#du-ling-documenti", t: "Documenti del nodo",
    s: "La linguetta con i file collegati al pezzo (disegni PDF e TIFF, DXF, STEP) e il loro stato. «Apri» mostra i PDF e i TIFF nel visore della pagina." },
  { k: "gesti", sel: "#du-gesti > summary", t: "Gesti sulla bozza",
    s: "Le modifiche che puoi fare a questo pezzo. Ognuna si salva da sola nella bozza e si può annullare." },
  { k: "correggi", sel: "#du-correggi", t: "Correggi codice, rev e descrizione",
    s: "Cambia l'identità del pezzo nella bozza. La proposta del sistema resta visibile accanto alla tua decisione." },
  { k: "cambia-tipo", sel: "#du-cambia-tipo", t: "Cambia tipo",
    s: "Cambia il tipo del pezzo (assieme, particolare, commerciale…). La finestra mostra l'effetto prima di confermare." },
  { k: "sposta", sel: "#du-sposta", t: "Sposta sotto…",
    s: "Sposta il pezzo sotto un altro padre nella bozza, con la sua quantità." },
  { k: "quantita", sel: "#du-quantita", t: "Quantità",
    s: "Cambia quante volte il pezzo sta nel padre." },
  { k: "non-includere", sel: "#du-non-includere-nodo, #du-non-includere", t: "Non includere nella BOM",
    s: "Il pezzo resta visibile ma non entra nella BOM confermata (per esempio un elemento CAD di servizio). Si può includere di nuovo." },
  { k: "sez-proposta", sel: "#du-sez-proposta > summary", t: "Identità proposta",
    s: "Che cosa propone il sistema per questo pezzo e da dove l'ha letto (nome del file, testo dello STEP, disegno)." },
  { k: "sez-decisione", sel: "#du-sez-decisione > summary", t: "Decisione dell'ingegnere",
    s: "L'identità decisa nella bozza, chi l'ha decisa e quando." },
  { k: "sez-categoria", sel: "#du-sez-categoria > summary", t: "Tipo e categoria",
    s: "Il tipo del pezzo e la categoria proposta (per esempio minuteria), da confermare o correggere." },
  { k: "sez-posizione", sel: "#du-sez-posizione > summary", t: "Posizione",
    s: "Sotto quale padre sta il pezzo, con quale quantità, e quali pezzi ha sotto." },
  { k: "sez-riferimenti", sel: "#du-sez-riferimenti > summary", t: "Riferimenti e relazioni",
    s: "I legami del pezzo con altri elementi proposti dal sistema. Restano proposte da leggere." },
  { k: "sez-provenienza", sel: "#du-sez-provenienza > summary", t: "Provenienza",
    s: "Da quale file e da quale lettura arriva il pezzo." },
  { k: "salvataggio", sel: "#du-salvataggio", t: "Stato del salvataggio",
    s: "Ogni gesto si salva da solo nella bozza poco dopo. Qui leggi se è salvata (con la versione e l'ora), se sta salvando o se qualcosa non è andato, con il rimedio." },
  { k: "annulla", sel: "#du-annulla", t: "Annulla",
    s: "Disfa l'ultima modifica della bozza. Non va oltre la versione da cui la bozza è nata." },
  { k: "ripeti", sel: "#du-ripeti", t: "Ripeti",
    s: "Rifà la modifica appena annullata." },
  { k: "abbandona", sel: "#du-abbandona", t: "Abbandona la bozza",
    s: "Chiude questa bozza senza confermarla. La finestra chiede conferma prima." },
  { k: "cta-bom", sel: "#du-cta-bom", t: "Rivedi e conferma BOM",
    s: "Apre la revisione finale in due fasi: prima le identità dei componenti, poi gerarchie, quantità e tipi, con gli avvisi del sistema. Da lì si conferma la BOM." },
  { k: "cta-fascicolo", sel: "#du-cta-fascicolo", t: "Rivedi e conferma fascicolo",
    s: "La conferma dei documenti del prodotto, che si fa dopo una BOM confermata." },
  { k: "wz-fase1", sel: "#du-wz-fase1", t: "Fase 1 · Identità dei componenti",
    s: "Controlla codice, revisione e descrizione di ogni componente, con i dubbi e gli avvisi del sistema." },
  { k: "wz-fase2", sel: "#du-wz-fase2", t: "Fase 2 · Gerarchie, quantità e tipi",
    s: "Controlla chi sta sotto chi, le quantità, i tipi e i pezzi non inclusi, prima della conferma." },
  { k: "wz-conferma", sel: "#du-wz-conferma", t: "Conferma BOM",
    s: "Registra la BOM del prodotto («Conferma BOM»). Se qualcosa la blocca, il pulsante resta spento e il motivo è scritto accanto." },
  { k: "working", sel: "#du-working-testa", t: "BOM confermata",
    s: "L'ultima BOM confermata del prodotto, in sola lettura: è la BOM di lavoro. Per cambiarla si apre una bozza nuova." },
  { k: "da-conferma", sel: "#du-da-conferma", t: "Modifica la BOM confermata",
    s: "Apre una bozza nuova che parte dalla BOM confermata, da modificare e confermare di nuovo." },
  { k: "da-registrato", sel: "#du-da-registrato", t: "Parti dalla BOM registrata",
    s: "Apre una bozza nuova partendo dalla BOM già registrata per il prodotto." },
  { k: "nessuna-bozza", sel: "#du-nessuna-bozza", t: "Nessuna bozza",
    s: "Il prodotto non ha ancora una bozza: nasce quando confermi uno STEP come sorgente nella scheda Sorgente." },
];

// ---------------------------------------------------------------- i passi della guida

const PASSI = {
  sorgente: [
    { sel: ["#du-prodotti"], t: "I prodotti della RFQ",
      s: "La richiesta può contenere più prodotti. Scegline uno qui sopra: la Sorgente e la Revisione mostrano sempre il prodotto scelto." },
    { sel: ["#du-nav"], t: "Due passi: Sorgente e Revisione",
      s: "Si lavora in due passi. In «Sorgente» si decide da quale STEP nasce la struttura e si smistano i file; in «Revisione» si corregge e si conferma la BOM. Le etichette dicono a che punto sei." },
    { sel: ["#du-sg-albero"], t: "Le cartelle dei file",
      s: "A sinistra i file della RFQ divisi per stato. Comincia da «Da controllare» e da «STEP della RFQ»: il numero dice quanti file aspettano.",
      manca: "Le cartelle compaiono quando la RFQ ha dei file letti dal sistema." },
    { sel: ["#du-sg-cerca"], t: "Cerca",
      s: "Scrivi parte del nome o del codice per filtrare l'elenco. Il tasto «/» porta qui da qualunque punto della scheda." },
    { sel: ["#du-sg-elenco"], t: "L'elenco della cartella",
      s: "I file della cartella scelta. Un clic su una riga apre il suo dettaglio; con le caselle ne scegli più di uno per un gesto insieme." },
    { sel: ["[id^='du-guarda-']"], t: "Guarda l'anteprima",
      s: "Prima di scegliere uno STEP come sorgente, guarda che cosa ci legge il sistema: assiemi, pezzi e quantità. Guardare non cambia niente.",
      manca: "Il pulsante compare sulle righe degli STEP candidati, nella cartella «STEP della RFQ»." },
    { sel: ["[id^='du-conferma-sorgente']"], t: "Conferma lo STEP come sorgente",
      s: "Quando l'anteprima ti convince, «Conferma questo STEP come sorgente» crea la bozza della BOM da quello STEP. La BOM si conferma dopo, nella Revisione.",
      manca: "Il pulsante compare nel dettaglio di uno STEP candidato (apri la riga di uno STEP nella cartella «STEP della RFQ»), oppure il prodotto ha già la sua sorgente." },
    { sel: ["#du-sg-dettaglio"], t: "Il dettaglio del file",
      s: "A destra tutto sul file scelto e i gesti possibili: aprirlo, metterlo in Altri file se va solo conservato, scartarlo se non serve.",
      manca: "Il dettaglio compare quando scegli un file nell'elenco." },
    { sel: ["#du-sg-aggiungi"], t: "Aggiungi file",
      s: "Se manca un file, caricalo da qui: viene analizzato e finisce nella cartella giusta." },
    { sel: ["#du-barra"], t: "La barra in fondo",
      s: "Dice che cosa si può fare in questa scheda, l'«Ultima mossa» con il suo esito (e «Annulla la mossa»), e porta alla Revisione.", scorri: true },
    { sel: ["#du-passo-revisione"], t: "Poi, la Revisione",
      s: "Con la sorgente confermata, passa alla Revisione per rivedere la bozza. Apri di nuovo la Guida lì per il secondo percorso." },
  ],
  revisione: [
    { sel: ["#du-passo-revisione"], t: "La scheda Revisione",
      s: "Qui c'è la bozza della BOM del prodotto scelto. L'etichetta dice lo stato della bozza e la sua versione." },
    { sel: ["#du-nessuna-bozza"], t: "Ancora nessuna bozza", solo: true,
      s: "Il prodotto non ha una bozza: torna alla Sorgente e conferma uno STEP come sorgente, oppure usa i pulsanti di partenza che compaiono qui." },
    { sel: ["#du-working-testa"], t: "La BOM confermata", solo: true,
      s: "Questa è l'ultima BOM confermata, in sola lettura. Per cambiarla, «Modifica la BOM confermata» apre una bozza nuova che parte da lei." },
    { sel: ["#du-vista-org", "#du-vista-elenco"], t: "Organigramma o Elenco",
      s: "Due modi di guardare la stessa bozza: l'albero di schede o la tabella con filtri e ricerca. Il pannello del nodo resta lo stesso." },
    { sel: ["#du-br"], t: "Il browser della distinta",
      s: "Sopra l'organigramma, l'albero compatto della distinta: assiemi, pezzi, quantità e, sotto ogni pezzo, i suoi file. Clic su una riga per scegliere il pezzo; la linguetta «Browser» (nell'angolo della tela, se è richiuso) lo apre e lo richiude.",
      manca: "Il browser compare nell'Organigramma, con una bozza o una BOM confermata." },
    { sel: ["#du-proposta-step"], t: "La proposta dello STEP",
      s: "Dice se la bozza e la struttura letta nello STEP sono diverse. Puoi importare solo alcuni elementi o ripartire da capo.",
      manca: "Il riquadro compare con una bozza nata da uno STEP sorgente." },
    { sel: ["#du-nuovo-ass", "#du-zoom-centra"], t: "Gli strumenti",
      s: "Aggiungi un assieme, un particolare o un commerciale; confronta con la proposta; apri i nodi segnalati; regola lo zoom.",
      manca: "Gli strumenti compaiono nell'Organigramma, con una bozza aperta." },
    { sel: ["#du-vp", "#du-elenco"], t: "La bozza", scorri: true,
      s: "Ogni scheda è un pezzo: codice, revisione, miniatura del disegno. Clic su una scheda per sceglierla; trascina lo sfondo per spostarti.",
      manca: "La bozza compare quando il prodotto ne ha una." },
    { sel: ["#du-zoom-schermo"], t: "Schermo intero", scorri: true,
      s: "L'organigramma a tutta finestra: cerca un pezzo, clic su una scheda per evidenziare padri, figli e documenti. «Mostra tutto» lo rimette intero; Esc torna qui.",
      manca: "Il pulsante sta nell'angolo dell'organigramma, nella vista «Organigramma»." },
    { sel: ["#du-pannello"], t: "Il pannello del nodo", scorri: true,
      s: "Tutto sul pezzo scelto, diviso in sezioni: identità proposta e decisa, tipo e categoria, posizione, documenti, provenienza.",
      manca: "Il pannello si riempie quando scegli un pezzo sulla tela o nell'elenco." },
    { sel: ["#du-gesti"], t: "I gesti sul pezzo", scorri: true,
      s: "Correggi l'identità, cambia il tipo, sposta il pezzo, cambia la quantità o non includerlo nella BOM. Ogni gesto si salva da solo.",
      manca: "I gesti compaiono nel pannello quando scegli un pezzo della bozza." },
    { sel: ["#du-salvataggio"], t: "Il salvataggio", scorri: true,
      s: "Non serve un pulsante «Salva»: la bozza si salva da sola dopo ogni gesto. Qui leggi la versione salvata o il rimedio se qualcosa non va.",
      manca: "Lo stato del salvataggio compare con una bozza aperta." },
    { sel: ["#du-annulla", "#du-ripeti"], t: "Annulla e Ripeti", scorri: true,
      s: "Disfano e rifanno le modifiche della bozza, una alla volta.",
      manca: "Annulla e Ripeti compaiono con una bozza aperta." },
    { sel: ["#du-cta-bom"], t: "Rivedi e conferma BOM", scorri: true,
      s: "Quando la bozza è pronta, apri la revisione finale: due fasi, identità e poi gerarchie, quantità e tipi. Da lì «Conferma BOM» la registra.",
      manca: "Il pulsante compare con una bozza aperta." },
  ],
};

// ---------------------------------------------------------------- utilità

function el(tag, cls, testo) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (testo) e.textContent = testo;
  return e;
}
function leggiMemoria() {
  try {
    return window.localStorage.getItem(CHIAVE_MEMORIA);
  } catch (e) {
    return null;
  }
}
function scriviMemoria(v) {
  try {
    window.localStorage.setItem(CHIAVE_MEMORIA, v);
  } catch (e) {
    /* la scelta vale solo per questa visita */
  }
}
function visibile(e) {
  if (!e || !e.isConnected) return false;
  const r = e.getBoundingClientRect();
  return r.width > 0 && r.height > 0;
}
// trova: la prima corrispondenza visibile di una voce del catalogo
function trova(voce) {
  let lista;
  try {
    lista = document.querySelectorAll(voce.sel);
  } catch (e) {
    return null;
  }
  for (const x of lista) {
    if (voce.testo && !voce.testo.test((x.textContent || "").trim())) continue;
    if (visibile(x)) return x;
  }
  return null;
}
function dentroAiuti(x) {
  return !!(x && x.closest && x.closest(".du-aiuti, .du-aiuto-fumetto, .du-guida"));
}
// inVista: l'angolo dell'elemento dove va l'icona non è coperto da altro (un dialogo, la testata appiccicata di un
// elenco, il bordo di un riquadro che scorre)
function inVista(e, r) {
  const vw = window.innerWidth, vh = window.innerHeight;
  const x = r.right - Math.min(6, r.width / 2), y = r.top + Math.min(6, r.height / 2);
  if (x < 0 || y < 0 || x >= vw || y >= vh) return false;
  const pila = document.elementsFromPoint(x, y);
  for (const p of pila) {
    if (dentroAiuti(p)) continue;
    return p === e || e.contains(p);
  }
  return false;
}

// ---------------------------------------------------------------- le icone «(?)» e il fumetto

const A = { attivi: true, strato: null, fumetto: null, icone: new Map(), corrente: null, voce: null, rif: 0, oss: null };

function creaStrato() {
  const s = el("div", "du-aiuti");
  s.id = "du-aiuti";
  s.setAttribute("aria-label", "Aiuti della pagina");
  const f = el("div", "du-aiuto-fumetto");
  f.id = "du-aiuto-fumetto";
  f.setAttribute("role", "tooltip");
  f.hidden = true;
  document.body.append(s, f);
  A.strato = s;
  A.fumetto = f;
}
function icona(voce) {
  let b = A.icone.get(voce.k);
  if (b) return b;
  b = el("span", "du-aiuto");
  b.setAttribute("tabindex", "0");
  b.setAttribute("role", "button");
  b.setAttribute("aria-label", "Aiuto: " + voce.t);
  b.setAttribute("aria-describedby", "du-aiuto-fumetto");
  b.dataset.aiuto = voce.k;
  b.hidden = true;
  b.addEventListener("mouseenter", () => mostraFumetto(voce, b));
  b.addEventListener("mouseleave", () => nascondiFumetto(b));
  b.addEventListener("focus", () => mostraFumetto(voce, b));
  b.addEventListener("blur", () => nascondiFumetto(b));
  // l'icona spiega e basta: il clic non arriva alla pagina
  b.addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
    mostraFumetto(voce, b);
  });
  b.addEventListener("pointerdown", (e) => e.stopPropagation());
  A.icone.set(voce.k, b);
  A.strato.append(b);
  return b;
}
function mostraFumetto(voce, b) {
  const f = A.fumetto;
  f.replaceChildren(el("strong", "du-aiuto-fumetto-t", voce.t), el("span", "du-aiuto-fumetto-s", voce.s), el("span", "du-freccia"));
  f.dataset.aiuto = voce.k;
  f.hidden = false;
  A.corrente = b;
  A.voce = voce;
  posizionaFumetto(b);
}
// accanto: dove mettere un fumetto di w × hgt accanto al rettangolo r (l'elemento spiegato) senza coprirlo, dentro la
// finestra, con la freccia verso il punto (ax, ay). L'ordine dei lati è quello di «ordine»; se nessun lato ha posto, il
// fumetto va sotto il punto (e copre una parte dell'elemento: succede solo con gli elementi grandi quasi quanto la
// finestra). Torna {x, y, lato, fx, fy}.
function accanto(r, w, hgt, ax, ay, ordine) {
  const vw = document.documentElement.clientWidth, vh = window.innerHeight;
  const M = 8, D = 10;
  const cx = (x) => Math.max(M, Math.min(x, vw - w - M));
  const cy = (y) => Math.max(M, Math.min(y, vh - hgt - M));
  const prove = {
    sotto: () => (r.bottom + D + hgt <= vh - M ? { x: cx(ax - w / 2), y: r.bottom + D, lato: "sotto" } : null),
    sopra: () => (r.top - D - hgt >= M ? { x: cx(ax - w / 2), y: r.top - D - hgt, lato: "sopra" } : null),
    destra: () => (r.right + D + w <= vw - M ? { x: r.right + D, y: cy(ay - 22), lato: "destra" } : null),
    sinistra: () => (r.left - D - w >= M ? { x: r.left - D - w, y: cy(ay - 22), lato: "sinistra" } : null),
  };
  let p = null;
  for (const l of ordine) {
    p = prove[l]();
    if (p) break;
  }
  if (!p) p = { x: cx(ax - w / 2), y: cy(ay + 14), lato: "sotto" };
  p.fx = Math.max(12, Math.min(ax - p.x, w - 12));
  p.fy = Math.max(12, Math.min(ay - p.y, hgt - 12));
  return p;
}
function posizionaFumetto(b) {
  const f = A.fumetto;
  if (f.hidden || !b) return;
  const ri = b.getBoundingClientRect();
  // l'elemento spiegato e l'icona insieme: il fumetto non copre né l'uno né l'altra
  const t = A.voce ? trova(A.voce) : null;
  const rt = t ? t.getBoundingClientRect() : ri;
  const vh = window.innerHeight, vw = document.documentElement.clientWidth;
  // un elemento molto grande (la tela, un elenco) non si aggira: il fumetto sta accanto all'icona
  const grande = rt.width > vw * 0.6 || rt.height > vh * 0.5;
  const r = grande ? ri : { left: Math.min(rt.left, ri.left), top: Math.min(rt.top, ri.top), right: Math.max(rt.right, ri.right), bottom: Math.max(rt.bottom, ri.bottom) };
  f.style.left = "0px";
  f.style.top = "0px";
  const w = f.offsetWidth, hgt = f.offsetHeight;
  const p = accanto(r, w, hgt, ri.left + ri.width / 2, ri.top + ri.height / 2, ["sotto", "sopra", "destra", "sinistra"]);
  f.dataset.lato = p.lato;
  f.style.setProperty("--du-fx", Math.round(p.fx) + "px");
  f.style.setProperty("--du-fy", Math.round(p.fy) + "px");
  f.style.left = Math.round(p.x) + "px";
  f.style.top = Math.round(p.y) + "px";
}
function nascondiFumetto(b) {
  if (b && A.corrente !== b) return;
  A.corrente = null;
  if (!A.fumetto) return;
  A.fumetto.hidden = true;
  // il testo non resta nel DOM a fumetto chiuso
  A.fumetto.replaceChildren();
  delete A.fumetto.dataset.aiuto;
}
function aggiornaIcone() {
  A.rif = 0;
  if (!A.strato) return;
  // nello schermo intero dell'organigramma gli aiuti della pagina restano sotto: niente icone sopra la tela
  A.strato.hidden = !A.attivi || G.aperta || !!document.getElementById("du-fs-barra");
  if (A.strato.hidden) {
    nascondiFumetto();
    return;
  }
  const vw = document.documentElement.clientWidth, vh = window.innerHeight;
  for (const voce of AIUTI) {
    const t = trova(voce);
    let r = t && t.getBoundingClientRect();
    const b = t ? icona(voce) : A.icone.get(voce.k);
    if (!b) continue;
    if (!t || !inVista(t, r)) {
      if (!b.hidden) {
        if (A.corrente === b) nascondiFumetto();
        b.hidden = true;
      }
      continue;
    }
    const pos = posto(t, r, vw, vh);
    if (!pos) {
      if (!b.hidden) {
        if (A.corrente === b) nascondiFumetto();
        b.hidden = true;
      }
      continue;
    }
    const sx = pos[0] + "px", sy = pos[1] + "px";
    if (b.style.left !== sx) b.style.left = sx;
    if (b.style.top !== sy) b.style.top = sy;
    if (b.hidden) b.hidden = false;
  }
  if (A.corrente) posizionaFumetto(A.corrente);
}
// posto: dove va l'icona di t. Prima sull'angolo in alto a destra, poi su quello in alto a sinistra, poi fuori dal lato
// destro; un posto non va bene se l'icona coprirebbe il centro di un altro elemento cliccabile (la casella di una
// scheda, un pulsante vicino): il clic su quell'elemento deve restare suo. Nessun posto libero: niente icona.
const CLICCABILI = "button, input, select, textarea, a[href], summary, label, [role=button], [role=row], [role=treeitem], [role=tab], [tabindex]";
const LATO = 16;
function posto(t, r, vw, vh) {
  const cand = [[r.right - LATO / 2, r.top - LATO / 2], [r.left - LATO / 2, r.top - LATO / 2], [r.right + 2, r.top + r.height / 2 - LATO / 2]];
  const clip = ritaglio(t);
  for (const c of cand) {
    const x = Math.round(Math.max(2, Math.min(c[0], vw - LATO - 2)));
    const y = Math.round(Math.max(2, Math.min(c[1], vh - LATO - 2)));
    // il centro dell'icona dentro la parte visibile dei riquadri che tagliano l'elemento (la tela, un elenco che scorre)
    const mx = x + LATO / 2, my = y + LATO / 2;
    if (clip && (mx < clip.left || mx > clip.right || my < clip.top || my > clip.bottom)) continue;
    if (libero(t, x, y)) return [x, y];
  }
  return null;
}
// ritaglio: il rettangolo visibile dei riquadri antenati di t che tagliano il contenuto (overflow diverso da visible),
// allargato di un'icona perché l'icona può stare sul bordo del riquadro; null se nessuno taglia
function ritaglio(t) {
  let q = null;
  for (let e = t.parentElement; e && e !== document.body && e !== document.documentElement; e = e.parentElement) {
    const cs = getComputedStyle(e);
    if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
    const r = e.getBoundingClientRect();
    q = q ? { left: Math.max(q.left, r.left), top: Math.max(q.top, r.top), right: Math.min(q.right, r.right), bottom: Math.min(q.bottom, r.bottom) } : { left: r.left, top: r.top, right: r.right, bottom: r.bottom };
  }
  return q ? { left: q.left - LATO / 2, top: q.top - LATO / 2, right: q.right + LATO / 2, bottom: q.bottom + LATO / 2 } : null;
}
function libero(t, x, y) {
  const punti = [[x + LATO / 2, y + LATO / 2], [x + 1, y + 1], [x + LATO - 1, y + 1], [x + 1, y + LATO - 1], [x + LATO - 1, y + LATO - 1]];
  for (const [px, py] of punti) {
    let sopra = null;
    for (const p of document.elementsFromPoint(px, py)) {
      if (dentroAiuti(p)) continue;
      sopra = p;
      break;
    }
    const c = sopra && sopra.closest ? sopra.closest(CLICCABILI) : null;
    if (!c || c === t || c.contains(t)) continue;
    const q = c.getBoundingClientRect();
    const cx = q.left + q.width / 2, cy = q.top + q.height / 2;
    if (cx >= x - 2 && cx <= x + LATO + 2 && cy >= y - 2 && cy <= y + LATO + 2) return false;
  }
  return true;
}
function pianifica() {
  if (A.rif) return;
  A.rif = requestAnimationFrame(aggiornaIcone);
}
function osserva() {
  A.oss = new MutationObserver(pianifica);
  const opz = { childList: true, subtree: true, attributes: true, attributeFilter: ["class", "style", "hidden", "open", "aria-current", "aria-pressed", "aria-selected"] };
  for (const id of ["du", "du-finestra", "du-wizard", "du-velo"]) {
    const x = document.getElementById(id);
    if (x) A.oss.observe(x, opz);
  }
  window.addEventListener("scroll", pianifica, { capture: true, passive: true });
  window.addEventListener("resize", pianifica, { passive: true });
}
function impostaAiuti(si) {
  A.attivi = !!si;
  scriviMemoria(si ? "1" : "0");
  const c = document.getElementById("du-aiuti-mostra");
  if (c) c.checked = A.attivi;
  pianifica();
}

// ---------------------------------------------------------------- la guida

const G = { aperta: false, passi: [], i: 0, box: null, anello: null, fumetto: null, ritorno: null, rif: 0, tastiera: null, scorrimento: null };

function bersagli(passo) {
  const out = [];
  for (const s of passo.sel) {
    let lista;
    try {
      lista = document.querySelectorAll(s);
    } catch (e) {
      continue;
    }
    for (const x of lista) {
      if (visibile(x)) {
        out.push(x);
        break;
      }
    }
  }
  return out;
}
function rettangolo(lista) {
  let l = Infinity, t = Infinity, r = -Infinity, b = -Infinity;
  for (const x of lista) {
    const q = x.getBoundingClientRect();
    l = Math.min(l, q.left);
    t = Math.min(t, q.top);
    r = Math.max(r, q.right);
    b = Math.max(b, q.bottom);
  }
  return { left: l, top: t, right: r, bottom: b, width: r - l, height: b - t };
}
function schedaAperta() {
  return document.querySelector("#du-passo-revisione[aria-current='step']") ? "revisione" : "sorgente";
}
export function apriGuida() {
  if (G.aperta) return;
  const scheda = schedaAperta();
  // i passi «solo» valgono solo se il loro elemento c'è (per esempio «nessuna bozza» o la BOM confermata)
  G.passi = PASSI[scheda].filter((p) => !p.solo || bersagli(p).length);
  G.scheda = scheda;
  G.i = 0;
  G.aperta = true;
  G.ritorno = document.activeElement;
  G.scorrimento = new Map();
  nascondiFumetto();
  if (A.strato) A.strato.hidden = true;

  const box = el("div", "du-guida");
  box.id = "du-guida";
  const blocco = el("div", "du-guida-blocco");
  // la guida mostra e spiega: un clic sulla pagina velata non fa niente
  blocco.addEventListener("click", (e) => {
    e.preventDefault();
    e.stopPropagation();
  });
  const anello = el("div", "du-guida-anello");
  anello.id = "du-guida-anello";
  const f = el("section", "du-guida-fumetto");
  f.id = "du-guida-fumetto";
  f.setAttribute("role", "dialog");
  f.setAttribute("aria-modal", "true");
  f.setAttribute("aria-labelledby", "du-guida-titolo");
  box.append(blocco, anello, f);
  document.body.append(box);
  G.box = box;
  G.anello = anello;
  G.fumetto = f;
  G.tastiera = (e) => {
    if (!G.aperta) return;
    const k = e.key;
    if (k === "Escape") {
      e.preventDefault();
      chiudiGuida();
    } else if (k === "ArrowRight") {
      e.preventDefault();
      vai(1);
    } else if (k === "ArrowLeft") {
      e.preventDefault();
      vai(-1);
    } else if (k === "Tab") {
      // il fuoco resta nel fumetto della guida
      const bott = [...G.fumetto.querySelectorAll("button:not([disabled])")];
      if (bott.length) {
        e.preventDefault();
        const at = bott.indexOf(document.activeElement);
        const n = (at + (e.shiftKey ? -1 : 1) + bott.length) % bott.length;
        bott[n].focus({ preventScroll: true });
      }
    }
    // nessun tasto arriva alle scorciatoie della pagina mentre la guida è aperta
    e.stopPropagation();
  };
  window.addEventListener("keydown", G.tastiera, true);
  G.riposiziona = () => {
    if (G.rif) return;
    G.rif = requestAnimationFrame(() => {
      G.rif = 0;
      posizionaGuida();
    });
  };
  window.addEventListener("scroll", G.riposiziona, { capture: true, passive: true });
  window.addEventListener("resize", G.riposiziona, { passive: true });
  mostraPasso(true);
}
function vai(d) {
  const n = G.i + d;
  if (n < 0) return;
  if (n >= G.passi.length) return chiudiGuida();
  G.i = n;
  mostraPasso(true);
}
function mostraPasso(scorri) {
  const p = G.passi[G.i];
  const lista = bersagli(p);
  if (scorri && p.scorri && lista.length) {
    const r = rettangolo(lista);
    if (r.top < 0 || r.bottom > window.innerHeight) {
      // la guida ricorda dov'erano i riquadri che scorrono, e alla chiusura li rimette lì
      for (let e = lista[0].parentElement; e; e = e.parentElement) if (!G.scorrimento.has(e)) G.scorrimento.set(e, [e.scrollLeft, e.scrollTop]);
      const d = document.scrollingElement;
      if (d && !G.scorrimento.has(d)) G.scorrimento.set(d, [d.scrollLeft, d.scrollTop]);
      lista[0].scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }
  const f = G.fumetto;
  const tot = G.passi.length;
  const titolo = el("h2", "du-guida-t", p.t);
  titolo.id = "du-guida-titolo";
  const conta = el("span", "du-guida-conta", "Passo " + (G.i + 1) + " di " + tot + " · " + (G.scheda === "revisione" ? "Revisione" : "Sorgente"));
  conta.id = "du-guida-conta";
  const testo = el("p", "du-guida-s", p.s);
  const nota = lista.length ? null : el("p", "du-guida-manca", "Non c'è in questo momento. " + (p.manca || "Compare in un altro stato della pagina."));
  if (nota) nota.id = "du-guida-manca";
  const piede = el("div", "du-guida-piede");
  const esci = el("button", "du-guida-btn du-guida-btn--ghost", "Esci");
  esci.type = "button";
  esci.id = "du-guida-esci";
  esci.addEventListener("click", chiudiGuida);
  const indietro = el("button", "du-guida-btn", "Indietro");
  indietro.type = "button";
  indietro.id = "du-guida-indietro";
  indietro.disabled = G.i === 0;
  indietro.addEventListener("click", () => vai(-1));
  const avanti = el("button", "du-guida-btn du-guida-btn--pri", G.i === tot - 1 ? "Fine" : "Avanti");
  avanti.type = "button";
  avanti.id = "du-guida-avanti";
  avanti.addEventListener("click", () => vai(1));
  // «Esci» a sinistra, «Indietro» e «Avanti» insieme a destra, su una riga sola
  const destra = el("span", "du-guida-piede-d");
  destra.append(indietro, avanti);
  piede.append(esci, destra);
  f.replaceChildren(conta, titolo, testo, nota || "", piede, el("span", "du-freccia"));
  f.dataset.passo = String(G.i + 1);
  G.box.dataset.evidenzia = lista.length ? "1" : "0";
  posizionaGuida();
  avanti.focus({ preventScroll: true });
}
function posizionaGuida() {
  if (!G.aperta) return;
  const p = G.passi[G.i];
  const lista = bersagli(p);
  const a = G.anello, f = G.fumetto;
  const vw = document.documentElement.clientWidth, vh = window.innerHeight;
  let r = null;
  if (lista.length) {
    const q = rettangolo(lista);
    const pad = 6;
    r = { left: Math.max(4, q.left - pad), top: Math.max(4, q.top - pad), right: Math.min(vw - 4, q.right + pad), bottom: Math.min(vh - 4, q.bottom + pad) };
    if (r.right - r.left < 8 || r.bottom - r.top < 8) r = null;
  }
  G.box.dataset.evidenzia = r ? "1" : "0";
  if (r) {
    a.hidden = false;
    a.style.left = Math.round(r.left) + "px";
    a.style.top = Math.round(r.top) + "px";
    a.style.width = Math.round(r.right - r.left) + "px";
    a.style.height = Math.round(r.bottom - r.top) + "px";
  } else a.hidden = true;
  const w = f.offsetWidth, hgt = f.offsetHeight;
  // accanto all'elemento, non sopra (seconda richiesta del 22/10 [U]): prima a destra o a sinistra, poi sotto o sopra;
  // senza elemento il fumetto sta in basso al centro, senza freccia
  let pz;
  if (!r) pz = { x: (vw - w) / 2, y: vh - hgt - 24, lato: "no", fx: 0, fy: 0 };
  else {
    const ax = (r.left + r.right) / 2, ay = (Math.max(r.top, 0) + Math.min(r.bottom, vh)) / 2;
    const largo = r.right - r.left > vw * 0.55;
    pz = accanto(r, w, hgt, largo ? Math.min(r.left + 160, ax) : ax, largo ? ay : Math.min(ay, r.top + 24), largo ? ["sotto", "sopra", "destra", "sinistra"] : ["destra", "sinistra", "sotto", "sopra"]);
  }
  f.dataset.lato = pz.lato;
  f.style.setProperty("--du-fx", Math.round(pz.fx) + "px");
  f.style.setProperty("--du-fy", Math.round(pz.fy) + "px");
  f.style.left = Math.round(pz.x) + "px";
  f.style.top = Math.round(pz.y) + "px";
}
export function chiudiGuida() {
  if (!G.aperta) return;
  G.aperta = false;
  window.removeEventListener("keydown", G.tastiera, true);
  window.removeEventListener("scroll", G.riposiziona, { capture: true });
  window.removeEventListener("resize", G.riposiziona);
  if (G.rif) cancelAnimationFrame(G.rif);
  G.rif = 0;
  if (G.box) G.box.remove();
  G.box = G.anello = G.fumetto = null;
  // la pagina torna dov'era prima della guida
  if (G.scorrimento) for (const [e, [x, y]] of G.scorrimento) if (e.scrollLeft !== x || e.scrollTop !== y) e.scrollTo(x, y);
  G.scorrimento = null;
  const r = G.ritorno && G.ritorno.isConnected ? G.ritorno : document.getElementById("du-guida-apri");
  if (r && r.focus) r.focus({ preventScroll: true });
  pianifica();
}

// ---------------------------------------------------------------- la testata e l'avvio

function testata() {
  const testa = document.querySelector("#du .du-testa");
  if (!testa || document.getElementById("du-guida-testa")) return;
  const box = el("div", "du-guida-testa");
  box.id = "du-guida-testa";
  const guida = el("button", "du-btn du-btn--sm du-guida-apri", "Guida");
  guida.type = "button";
  guida.id = "du-guida-apri";
  guida.title = "Un percorso a passi che spiega la scheda aperta";
  guida.addEventListener("click", apriGuida);
  const lab = el("label", "du-aiuti-interr");
  const c = document.createElement("input");
  c.type = "checkbox";
  c.id = "du-aiuti-mostra";
  c.checked = A.attivi;
  c.addEventListener("change", () => impostaAiuti(c.checked));
  lab.append(c, el("span", "", "Mostra aiuti (?)"));
  box.append(guida, lab);
  const azioni = document.getElementById("du-testa-azioni");
  if (azioni && azioni.parentElement === testa) testa.insertBefore(box, azioni);
  else testa.append(box);
}

// avviaGuida: parte da sola quando il template carica questo modulo, dopo la pagina (i moduli girano nell'ordine del
// documento, quindi la testata della pagina c'è già). Niente rete, niente stato della pagina.
export function avviaGuida() {
  if (!document.getElementById("du") || A.strato) return;
  A.attivi = leggiMemoria() !== "0";
  creaStrato();
  testata();
  osserva();
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && A.fumetto && !A.fumetto.hidden) nascondiFumetto();
  });
  pianifica();
}

avviaGuida();
