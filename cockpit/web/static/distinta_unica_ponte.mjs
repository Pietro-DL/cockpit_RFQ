// distinta_unica_ponte.mjs — il ponte della Distinta unica di V1 (giro 5, A1d, V1; direttiva 23 §6.3; matrice dei gesti
// di V1, MAT §1.3, §5; emendamento di V1, EM §9, §10.9).
//
// È l'unico modulo della pagina che chiama il server (D23:284). La pagina (distinta_unica.mjs) lo importa e non fa
// richieste da sé: presentazione, selezione, zoom, pan e interazione con la bozza stanno nella pagina, ogni regola sta
// nel server (RV3-05 A [U], DEC:84; D23:280).
//
// Deriva dal ponte della tappa del 13/10 (corsia P, web/static/motore_a_gesti.mjs della tappa: blocco DEDICATE,
// trasporto JSON, cataloghi dei codici, chiave del tentativo, confermaBom), riesaminato contro il contratto di V1
// (MAT §8). Che cosa cambia:
//   - una funzione per rotta: R1-R7, R11, il ramo JSON di POST /thread/{id}/carica (E1′), S1-S9 (S8 e S9 di U2), la revoca
//     della sorgente (direttiva 39 §2), R15 e il modello /motore-a/dati (MAT §1.3). Le cinque forme di R2 hanno ciascuna la
//     sua funzione; il riallineamento della proposta è riallineaProposta, mai «riallinea»; R11 è riallineaDocumenti;
//   - i tre atti esistenti della lista chiusa ridotta (EM §9.10 con il recepimento della direttiva 39; U2 §2.2) in ATTI, con
//     la risposta HTML data-ma-esito. Direttiva 39 §2 e D-TAB-02 B rivista [T-39]: «Autorizza» e «Revoca l'autorizzazione»
//     non sono più gesti della pagina nuova (la fonte della working la promuove solo R5; la revoca di uno STEP scelto per
//     errore è revocaSorgente), e con loro esce l'anteprima dell'autorizzazione. Nessun gesto legacy (MAT §7): né «Conferma
//     l'albero», né i gesti strutturali immediati, né «Scarta il file»;
//   - una scrittura alla volta PER RISORSA (la bozza di un prodotto, un file, lo STEP di un prodotto, il caricamento):
//     la coda sta nella pagina (MAT §1.3), che manda la seconda scrittura sulla stessa risorsa solo quando la prima ha
//     un esito. Se una arriva comunque mentre l'altra è in volo, il ponte non la manda e torna PONTE_IN_VOLO, con il
//     testo vero: «non è partita». Le letture non prendono lucchetti;
//   - la chiave del tentativo dall'esito incerto (R5, S2, S3, S4, S8, R15) sta nella memoria di sessione della scheda
//     (sessionStorage), con l'impronta del corpo, PRIMA dell'invio, e si toglie a esito certo: sopravvive a una ricarica
//     della pagina (EM §10.9, ultimo paragrafo; A4-Q05 A; MAT §5.5). Un'interruzione di rete non è una conferma fallita:
//     l'esito si recupera con R6 e la stessa chiave;
//   - la chiave_salvataggio di R2 (EM §9.2) la genera il ponte e la riusa per lo stesso corpo finché l'esito non è
//     certo: il reinvio dopo una risposta persa riceve «riproposto»;
//   - nessun numero di formato scritto qui: il formato lo dice R1 (MAT §1.3, V0-D49).
//
// Esiti delle funzioni (MAT §1.3):
//   - le letture tornano l'oggetto della risposta 200, oppure lanciano un ErroreDedicata {codice, stato, testo, dati};
//     /motore-a/dati, S6 e R1 ?confronto=1 dicono l'errore di calcolo con un 200 e «errore» (D-18): la pagina lo legge;
//   - le scritture con la versione attesa (R2, R3) tornano {esito: "ok"|"conflitto"|"no"|"errore", codice, motivo,
//     testo, dati}; "ok" anche per «riproposto»;
//   - le scritture con la chiave (R5, S2, S3, S4, S8, R15) tornano {esito: "ok"|"conflitto"|"no"|"da_verificare"|
//     "non_registrato"|"errore", codice, motivo, testo, chiave, dati}. "da_verificare" = 5xx, rete caduta, risposta che non
//     si legge, sessione scaduta dopo l'invio: la chiave resta in sospeso e si verifica con esitoIntento. "non_registrato"
//     = il 500 CONTROLLO_FINALE di R5 (EM §8.4): esito certo, niente «Riprova»;
//   - E1′ torna {esito: "caricati"|"parziale"|"no"|"errore", file, testo, codice, dati}: "parziale" non è mai un «ok»;
//     un caricamento fermo per inattività è "errore" con PONTE_INATTIVITA (come una rete caduta: si rimandano gli
//     stessi file, e quelli già entrati risultano «già presente»);
//   - gli atti tornano {esito: "ok"|"no"|"errore", testo}.
// I codici di trasporto: PONTE_RETE, PONTE_SESSIONE, PONTE_RISPOSTA, PONTE_ROTTA_ASSENTE, PONTE_IN_VOLO,
// PONTE_TENTATIVO_IN_SOSPESO, PONTE_ARGOMENTO, PONTE_NON_CONFIGURATO (MAT §1.3), e PONTE_INATTIVITA del caricamento.
//
// Vincoli del file: nessun import, nessun await fuori dalle funzioni, solo le globali del DOM (window, document,
// location, fetch, XMLHttpRequest, AbortSignal, setTimeout e clearTimeout, DOMParser, URL, URLSearchParams, FormData,
// sessionStorage, crypto, Uint8Array). Ogni chiamata JSON e ogni atto hanno un tempo massimo (AbortSignal.timeout): una
// richiesta appesa non tiene la risorsa «in volo» per sempre, e scaduto il tempo l'esito è quello di una rete caduta
// («da verificare» per le scritture con la chiave, «non arrivato» per R2). Il caricamento (E1′) no: ha un tempo
// d'inattività, cioè si ferma solo se per INATTIVITA nessun byte va al server e nessuno ne torna; un caricamento lento
// che avanza arriva in fondo (decisioni dell'ondata A, §5 n. 3). Per contare i byte inviati serve XMLHttpRequest
// (upload.onprogress: fetch non li dice), ed è il solo uso di setTimeout del file. Indirizzi costruiti solo da UUID
// controllati (RE_UUID) e dalla RFQ della pagina (D23:234): nessun percorso del disco va al server, nessuno ne torna.

const U = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const RE_UUID = new RegExp("^" + U + "$");
const RE_FIRMA = /^[0-9a-f]{64}$/;
// la firma dell'effetto di un cambio di tipo: esadecimale minuscolo di 24-64 cifre (decisioni dell'ondata A, P-16)
const RE_FIRMA_EFFETTO = /^[0-9a-f]{24,64}$/;
const TIPI_COMPONENTE = ["finito", "sottoassieme", "sciolto", "commerciale"];

const SESSIONE = "Sessione scaduta: si torna al login.";
const IN_VOLO = "Un'altra scrittura sulla stessa risorsa è in corso: questa non è partita. Rifalla quando la prima ha un esito.";
// il tempo massimo di una richiesta (AbortSignal.timeout, commento di testa): largo, perché i calcoli del motore e la
// copia sul NAS hanno già i loro tempi lato server (503 CALCOLO_OCCUPATO, 504 CALCOLO_SCADUTO)
const TEMPO_MASSIMO = 120000;
function segnale() {
  return typeof AbortSignal !== "undefined" && typeof AbortSignal.timeout === "function" ? AbortSignal.timeout(TEMPO_MASSIMO) : undefined;
}
// il tempo d'inattività del caricamento (E1′, commento di testa): nessun byte inviato né ricevuto per tanto, e il
// caricamento si ferma. Scelta [T] della corsia: non meno del tempo massimo delle chiamate JSON (TEMPO_MASSIMO), che
// prima valeva per tutto il caricamento. Un server che, ricevuto il corpo, tace mentre scrive i file prima arrivava in
// fondo entro 120 s, e deve arrivarci ancora: DECA §5 n. 3 toglie un limite, non ne aggiunge uno più stretto. Si ritara
// quando la corsia file dice il tempo peggiore del server dopo il corpo (RQ-F1); la L1 lo tiene >= TEMPO_MASSIMO
const INATTIVITA = 120000;
const FATTO = "Fatto: il server non ha aggiunto altro.";
const NIENTE = "Niente è cambiato.";
const SENZA_ESITO =
  "La risposta non dice l'esito (manca data-ma-esito): non si sa se il gesto è riuscito. Rileggi la pagina per vedere lo stato.";
const DA_VERIFICARE = "Non so se il gesto è registrato: verifico con la stessa chiave prima di qualunque altro tentativo.";
const SCRITTURA_ILLEGGIBILE =
  "Il server ha risposto, ma la risposta non si legge: la scrittura può essere avvenuta. Rileggi la bozza prima di riprovare.";

// DEDICATE:inizio — le rotte delle funzioni dedicate di V1, «METODO percorso», con i percorsi del C3 §1.2 e di EM §9 e
// §10.9 alla lettera. {id} = la RFQ della pagina; {u} = un UUID controllato dalla funzione. Le chiama solo la loro
// funzione.
const DEDICATE = {
  leggiModello: "GET /thread/{id}/motore-a/dati",
  leggiBozza: "GET /thread/{id}/motore-a/bozza",
  salvaBozza: "POST /thread/{id}/motore-a/bozza",
  creaBozza: "POST /thread/{id}/motore-a/bozza",
  cambiaSorgente: "POST /thread/{id}/motore-a/bozza",
  riallineaProposta: "POST /thread/{id}/motore-a/bozza",
  convertiBozza: "POST /thread/{id}/motore-a/bozza",
  abbandonaBozza: "POST /thread/{id}/motore-a/bozza/{u}/abbandona",
  riepilogoBom: "POST /thread/{id}/motore-a/bom/riepilogo",
  confermaBom: "POST /thread/{id}/motore-a/bom/conferma",
  esitoIntento: "GET /thread/{id}/motore-a/intento/{u}",
  storicoConferme: "GET /thread/{id}/motore-a/conferme",
  riallineaDocumenti: "POST /thread/{id}/motore-a/riallinea",
  carica: "POST /thread/{id}/carica",
  leggiSorgente: "GET /thread/{id}/sorgente",
  scartaFile: "POST /thread/{id}/sorgente/file/{u}/scarta",
  altriFile: "POST /thread/{id}/sorgente/file/{u}/altri-file",
  riattivaFile: "POST /thread/{id}/sorgente/file/{u}/riattiva",
  urlContenuto: "GET /thread/{id}/sorgente/file/{u}/contenuto",
  anteprimaStep: "GET /thread/{id}/sorgente/step/{u}/anteprima",
  // S7 («Associa lo STEP…») non c'è: il server non registra la rotta, e lo STEP non candidato si guarda con S6 e si
  // conferma con S8 (partenza dal solo prodotto o dal gruppo dell'anteprima): scelta [T-39] della corsia, rilievo del 20/10
  // le rotte di U2 (EM §9.5-bis, §10.9; MAT §1.3): S8 la conferma della sorgente, S9 lo smistamento di tutti i prodotti
  // (una variante di /dati, ?sorgente=1), R15 il rifiuto della proposta aggiornata
  confermaSorgente: "POST /thread/{id}/sorgente/step/{u}/conferma-sorgente",
  // la revoca della sorgente della bozza (direttiva 39 §2; EM App. I.4): lo STEP scelto per errore, con il motivo
  revocaSorgente: "POST /thread/{id}/sorgente/step/{u}/revoca-sorgente",
  leggiSmistamento: "GET /thread/{id}/motore-a/dati",
  rifiutaProposta: "POST /thread/{id}/motore-a/proposta-aggiornata/rifiuta",
};
// DEDICATE:fine

// I codici che ogni funzione legge come esito del contratto (EM §9.1-§9.7, §10.9, §10.10): un 409 o un 422 con un
// codice fuori da queste liste non è un esito del contratto, e torna "errore" con il codice com'è. Da generare
// dall'elenco unico dei codici del server quando la corsia persistenza lo pubblica (EM §10.10; prova T-05).
// CATALOGHI:inizio
const FORME_R2 = ["salvaBozza", "creaBozza", "cambiaSorgente", "riallineaProposta", "convertiBozza"];
const CONFLITTI = {
  salvaBozza: ["BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
  creaBozza: ["BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
  cambiaSorgente: ["BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
  riallineaProposta: ["BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
  convertiBozza: ["BOZZA_VERSIONE", "BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
  abbandonaBozza: ["BOZZA_VERSIONE"],
  confermaBom: ["BASE_SUPERATA", "ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA", "BOZZA_VERSIONE", "COPIA_IN_CORSO"],
  scartaFile: ["FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA"],
  altriFile: ["FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA", "COPIA_IN_CORSO"],
  riattivaFile: ["FILE_VERSIONE", "CHIAVE_RIUSATA", "ANTEPRIMA_SUPERATA"],
  // S8 al passo 0 (EM §10.9 «S8»): i rifiuti dei passi dopo arrivano dentro passi[] di un 200 «parziale».
  // RICONCILIAZIONE_NON_DISPONIBILE (CodiciServerS8, direttiva 39): bozza da convertire, solo_prodotto su una bozza o su
  // una conferma con un'altra sorgente
  confermaSorgente: ["ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA", "BOZZA_VERSIONE", "RICONCILIAZIONE_NON_DISPONIBILE"],
  // BOZZA_VERSIONE (giro G39, G39-03): la bozza dichiarata dalla revoca non è più quella (core/rfq/sorgente/revoca.go)
  revocaSorgente: ["CHIAVE_RIUSATA", "BOZZA_VERSIONE"],
  rifiutaProposta: ["ANTEPRIMA_SUPERATA", "CHIAVE_RIUSATA"],
};
// FONTE_NON_CONFERMATA (C-3, U2 §2.8): la forma step o cambia_sorgente su uno STEP senza il documento cad_3d confermato
// e non sostituito del prodotto; mai per la partenza vuota (direttiva 39 §1)
// STEP_CANDIDATO_DEL_FINITO (giro G39, G39-02 [U]; distintabozza/stepcandidati3.go): la PB nuova o spostata su uno STEP
// candidato sorgente di un prodotto finito (la lettura del Motore A), anche in una bozza manuale
const RIFIUTI_R2 = ["CORPO_NON_VALIDO", "FUORI_RFQ", "BOZZA_ALTRO_PRODOTTO", "FILE_NON_ATTIVO", "MOTIVO_OBBLIGATORIO",
  "PROPOSTA_NON_DISPONIBILE", "IMPORTAZIONE_TROPPO_GRANDE", "FONTE_NON_CONFERMATA", "STEP_CANDIDATO_DEL_FINITO"];
const RIFIUTI_FILE = ["CORPO_NON_VALIDO", "FUORI_RFQ", "NON_GESTITO", "RFQ_UNITA", "DIPENDENZE_ATTIVE", "RISOLUZIONE_RICHIESTA"];
const RIFIUTI = {
  leggiModello: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  leggiBozza: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  salvaBozza: RIFIUTI_R2,
  creaBozza: RIFIUTI_R2,
  cambiaSorgente: RIFIUTI_R2,
  riallineaProposta: RIFIUTI_R2,
  convertiBozza: RIFIUTI_R2,
  abbandonaBozza: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  riepilogoBom: ["CORPO_NON_VALIDO", "FUORI_RFQ"],
  // NESSUNA_MODIFICA: la bozza è uguale alla conferma di base formato 3 (PE-23); lo stato è 422 (U2-P55; DEC-B PD-01)
  confermaBom: ["BLOCCHI", "CONGELATA", "NESSUNA_MODIFICA", "CORPO_NON_VALIDO", "FUORI_RFQ"],
  esitoIntento: ["FUORI_RFQ"],
  storicoConferme: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  carica: ["NESSUN_FILE_CARICATO", "CORPO_NON_VALIDO"],
  leggiSorgente: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  scartaFile: RIFIUTI_FILE.concat(["MOTIVO_OBBLIGATORIO"]),
  altriFile: RIFIUTI_FILE.concat(["FILE_SENZA_BYTE", "FILE_SCARTATO", "CONSERVAZIONE_NON_RIUSCITA"]),
  riattivaFile: ["FILE_SCARTATO", "FUORI_RFQ", "CORPO_NON_VALIDO", "RFQ_UNITA", "CONSERVAZIONE_MANCANTE"],
  anteprimaStep: ["FUORI_RFQ", "NON_STEP", "FILE_NON_ATTIVO", "CORPO_NON_VALIDO"],
  // R11 (EM §9.8, invariato dal C3 §1.2): gli errori di appartenenza e di forma della busta comune; un 5xx resta
  // «riallineamento non riuscito: riprova» (richiesta RQ-3 della corsia persistenza, ondata B)
  riallineaDocumenti: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  // GIA_DOCUMENTO (CodiciServerS8): lo stesso contenuto è già un documento di un altro componente, senza componente o
  // sostituito
  confermaSorgente: ["FUORI_RFQ", "NON_STEP", "FILE_NON_ATTIVO", "RFQ_UNITA", "MOTIVO_OBBLIGATORIO", "CORPO_NON_VALIDO", "STEP_SORGENTE_DI_ALTRO_PRODOTTO", "GIA_DOCUMENTO"],
  // la revoca: il motivo obbligatorio; nessuna_sorgente_da_revocare e prodotto_non_ammesso sono motivi di CORPO_NON_VALIDO
  revocaSorgente: ["FUORI_RFQ", "MOTIVO_OBBLIGATORIO", "CORPO_NON_VALIDO"],
  leggiSmistamento: ["FUORI_RFQ", "CORPO_NON_VALIDO"],
  rifiutaProposta: ["FUORI_RFQ", "CORPO_NON_VALIDO", "MOTIVO_OBBLIGATORIO"],
};
// i 503 e 504 del calcolo e della configurazione (EM §10.10): un errore che si ritenta, niente è stato scritto
const OCCUPATO = ["CALCOLO_OCCUPATO", "NON_CONFIGURATO", "CALCOLO_SCADUTO"];
const STATI_BOZZA = ["aperta", "nessuna_bozza"];
const STATI_INTENTO = ["registrato", "non_registrato", "registrato_senza_esito", "in_corso"];
// CATALOGHI:fine

// ATTI:inizio — i tre atti esistenti della lista chiusa ridotta (EM §9.10 e App. I con la direttiva 39; MAT §1.4), campi
// di form, risposta HTML <div data-ma-esito="ok|no">. {id} = la RFQ della pagina. Nessun altro gesto legacy (MAT §7).
// «Conferma la fonte…» non c'è più: la fa S8 (U2 §2.2). «Autorizza» e «Revoca l'autorizzazione» non ci sono più
// (direttiva 39 §2, D-TAB-02 B rivista [T-39]): solo R5 promuove la fonte della working; la revoca è revocaSorgente.
const ATTI = {
  prepara: "/thread/{id}/fascicolo/prepara",
  rianalizza: "/thread/{id}/fascicolo/rianalizza",
  riprovaCopie: "/thread/{id}/riprova-copie",
};
// ATTI:fine

// GUARDIE: controlli sui NOMI dei campi degli atti prima dell'invio, come nel ponte di P3 (Q-P3-02, base di lavoro A):
// nessuna logica di dominio. Ognuna restituisce il motivo del rifiuto, o "".
const GUARDIE = {
  // «Prepara i file» è solo il pulsante (DB7-6): mai il ramo automatico
  prepara: (p) => (p.has("auto") ? "«Prepara i file» è solo il pulsante (DB7-6)" : ""),
};

// ---------------------------------------------------------------- configurazione della pagina

let RFQ = ""; // la RFQ della pagina (data-du-thread del guscio)
let UTENTE = ""; // l'utente della sessione (data-du-utente): entra nelle chiavi della memoria di sessione

// configura: la RFQ e l'utente della pagina. Il ponte non legge l'indirizzo: il guscio può stare su più percorsi
// (distinta_unica_pagina.go, commento di testa).
export function configura(o) {
  const t = o && typeof o.thread === "string" ? o.thread : "";
  if (!RE_UUID.test(t)) throw erroreDedicata("PONTE_NON_CONFIGURATO", 0, "La RFQ della pagina non è un UUID.");
  RFQ = t.toLowerCase();
  UTENTE = o && typeof o.utente === "string" ? o.utente.toLowerCase() : "";
}

// rfq: la RFQ configurata; "" prima di configura.
export function rfq() {
  return RFQ;
}

// ---------------------------------------------------------------- utilità

// erroreDedicata: un Error con codice (quello del contratto, o PONTE_… quando il problema è del ponte o del
// trasporto), stato HTTP (0 se la richiesta non è partita o non ha avuto risposta) e dati (il JSON della risposta).
function erroreDedicata(codice, stato, testo, dati) {
  const e = new Error(testo || codice);
  e.name = "ErroreDedicata";
  e.codice = codice;
  e.stato = stato;
  e.dati = dati === undefined ? null : dati;
  return e;
}

function oggetto(v) {
  return v !== null && typeof v === "object" && !Array.isArray(v);
}

function messaggio(err) {
  return (err && err.message) || String(err);
}

function uuid(v) {
  return typeof v === "string" && RE_UUID.test(v);
}

// nuovaChiave: un UUID v4 (crypto.randomUUID, o crypto.getRandomValues fuori da HTTPS).
export function nuovaChiave() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return h.slice(0, 8) + "-" + h.slice(8, 12) + "-" + h.slice(12, 16) + "-" + h.slice(16, 20) + "-" + h.slice(20);
}

// canonico: JSON con le chiavi in ordine, per riconoscere lo stesso corpo (non per firmarlo: le impronte sono del
// server). Da solo è più severo del JSON canonico del contratto: non ordina gli array e distingue null da un campo
// assente. Il verso è quello prudente: due corpi che il server riterrebbe uguali possono sembrare diversi, mai il
// contrario, cioè mai la stessa chiave su due corpi diversi. Vale così per R2 (lo stesso oggetto si rimanda com'è) e per
// i gesti dei file (il corpo del tentativo sta nella memoria della scheda). Per R5 l'impronta si calcola sulla forma del
// contratto (formaCanonica), perché dopo una ricarica il corpo si rifà dai byte di R1, ordinati dal server.
export function canonico(v) {
  if (Array.isArray(v)) return "[" + v.map(canonico).join(",") + "]";
  if (oggetto(v)) return "{" + Object.keys(v).sort().filter((k) => v[k] !== undefined).map((k) => JSON.stringify(k) + ":" + canonico(v[k])).join(",") + "}";
  return JSON.stringify(v === undefined ? null : v);
}

// formaCanonica: la forma del JSON canonico del contratto (EM §3.10), su cui il server calcola impronta_corpo di R5
// (EM §8.4): i facoltativi assenti, vuoti o null tolti (sono equivalenti); nodi[] per chiave_bozza, archi[] per (padre,
// figlio), decisioni[] per id, documenti_preassociati[] per allegato_id (direttiva 35, C-2), caselle[] per (gruppo, rif, allegato_id);
// descrizione e ogni motivo in NFC. Lo stesso contenuto in un altro ordine (lo snapshot della pagina, nell'ordine dei
// gesti, e quello riletto da R1 dopo una ricarica) ha la stessa forma: «Riprova la conferma…» riusa la chiave del
// tentativo in sospeso (MAT §5.2, §5.5 n. 4). Gli elementi degli array restano tutti: si ordinano, non si tolgono.
const ORDINI = {
  nodi: (x) => [x.chiave_bozza],
  archi: (x) => [x.padre, x.figlio],
  decisioni: (x) => [x.id],
  documenti_preassociati: (x) => [x.allegato_id],
  caselle: (x) => [x.gruppo, x.rif, x.allegato_id],
};
function vuoto(v) {
  return v === undefined || v === null || v === "" || (Array.isArray(v) && v.length === 0) || (oggetto(v) && Object.keys(v).length === 0);
}
function confrontaChiavi(a, b) {
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    const x = a[i] === undefined || a[i] === null ? "" : String(a[i]);
    const y = b[i] === undefined || b[i] === null ? "" : String(b[i]);
    if (x < y) return -1;
    if (x > y) return 1;
  }
  return 0;
}
export function formaCanonica(v, chiave) {
  if (Array.isArray(v)) {
    const out = v.map((x) => formaCanonica(x, ""));
    const k = ORDINI[chiave || ""];
    if (k) out.sort((a, b) => confrontaChiavi(k(oggetto(a) ? a : {}), k(oggetto(b) ? b : {})));
    return out;
  }
  if (oggetto(v)) {
    const out = {};
    for (const k of Object.keys(v)) {
      const x = formaCanonica(v[k], k);
      if (!vuoto(x)) out[k] = x;
    }
    return out;
  }
  if (typeof v === "string" && (chiave === "descrizione" || chiave === "motivo") && typeof v.normalize === "function") return v.normalize("NFC");
  return v;
}

// percorso: la rotta della funzione, con la RFQ della pagina e, dove c'è {u}, l'UUID passato (già controllato).
function percorso(nome, u) {
  const rotta = DEDICATE[nome].split(" ")[1];
  return rotta.replace("{id}", RFQ).replace("{u}", (u || "").toLowerCase());
}

function metodo(nome) {
  return DEDICATE[nome].split(" ")[0];
}

function percorsoDi(indirizzo) {
  try {
    const u = new URL(indirizzo, location.href);
    return u.origin === location.origin ? u.pathname : "";
  } catch (e) {
    return "";
  }
}

// rimandoAlLogin: la sessione scaduta. aLogin (server.go) risponde 401 con HX-Redirect: /login a una richiesta htmx,
// senza htmx un 302 che fetch segue fino a /login. Torna dove andare, oppure "".
function rimandoAlLogin(risp) {
  const red = risp.headers.get("HX-Redirect") || "";
  if (risp.status === 401) return red && percorsoDi(red) === "/login" ? red : "/login";
  if (red && percorsoDi(red) === "/login") return red;
  if (risp.redirected && percorsoDi(risp.url) === "/login") return "/login";
  return "";
}

// sessioneScaduta: la pagina va al login. Una chiave in sospeso resta nella memoria di sessione: al ritorno la pagina
// la verifica con R6 (MAT §5.4, prima riga).
function sessioneScaduta() {
  try {
    location.assign("/login");
  } catch (e) {
    // resta dov'è
  }
}

function testoPiano(corpo) {
  if (!corpo) return "";
  let t = String(corpo);
  if (/^\s*</.test(t) && typeof DOMParser !== "undefined") t = new DOMParser().parseFromString(t, "text/html").body.textContent || "";
  t = t.replace(/\s+/g, " ").trim();
  return t.length > 400 ? t.slice(0, 400) + "…" : t;
}

function testoDi(r, ripiego) {
  if (r.json && typeof r.json.testo === "string" && r.json.testo !== "") return r.json.testo;
  if (r.json && typeof r.json.errore === "string" && r.json.errore !== "") return r.json.errore;
  return testoPiano(r.testo) || ripiego || "Errore " + r.stato;
}

function codiceDi(r) {
  return r.json && typeof r.json.codice === "string" ? r.json.codice : "";
}

// motivoDi: il motivo di un 409 o di un 422 (motivo di BOZZA_VERSIONE e di CORPO_NON_VALIDO, anteprima di
// ANTEPRIMA_SUPERATA); i motivi[] di BASE_SUPERATA restano nei dati.
function motivoDi(r) {
  if (!r.json) return "";
  if (typeof r.json.motivo === "string") return r.json.motivo;
  if (typeof r.json.anteprima === "string") return r.json.anteprima;
  return "";
}

// chiama: la richiesta di una funzione dedicata. → {login} oppure {stato, json, testo}; json = l'oggetto della risposta
// se è application/json e si legge, altrimenti null. Una rete che non risponde lancia (lo decide chi chiama).
async function chiama(nome, u, query, corpo, extra) {
  const conCorpo = corpo !== undefined;
  const intest = { Accept: "application/json" };
  if (conCorpo && !(corpo instanceof FormData)) intest["Content-Type"] = "application/json";
  const risp = await fetch(percorso(nome, u) + (query || ""), {
    method: metodo(nome),
    credentials: "same-origin",
    headers: Object.assign(intest, extra || {}),
    body: !conCorpo ? undefined : corpo instanceof FormData ? corpo : JSON.stringify(corpo),
    signal: segnale(),
  });
  const login = rimandoAlLogin(risp);
  if (login) return { login };
  const testo = await risp.text();
  let json = null;
  if ((risp.headers.get("Content-Type") || "").toLowerCase().indexOf("json") >= 0) {
    try {
      json = JSON.parse(testo);
    } catch (e) {
      json = null;
    }
  }
  // X-Motivo: il codice che le rotte della Sorgente mettono su ogni risposta d'errore, anche senza busta (EM §10.9,
  // §10.10; DEC-B PF-25); lo legge motivoContenuto per il visore
  return { stato: risp.status, json: oggetto(json) ? json : null, testo, motivo: risp.headers.get("X-Motivo") || "" };
}

// ---------------------------------------------------------------- le scritture: una alla volta per risorsa

const inVolo = new Set();

// occupata: c'è una scrittura in volo sulla risorsa (la pagina usa questo per tenere la sua coda).
export function occupata(risorsa) {
  return inVolo.has(risorsa);
}

// inEsclusiva: fn con la risorsa occupata; null se era già occupata e fn non è partita.
async function inEsclusiva(risorsa, fn) {
  if (inVolo.has(risorsa)) return null;
  inVolo.add(risorsa);
  try {
    return await fn();
  } finally {
    inVolo.delete(risorsa);
  }
}

// ---------------------------------------------------------------- le letture

// leggi: una lettura dedicata (GET, o la POST di sola lettura di R4). L'oggetto della risposta 200 che passa valida,
// altrimenti un ErroreDedicata. Un 404 senza JSON è la rotta che non c'è (flag spento o non ancora nel server):
// PONTE_ROTTA_ASSENTE.
async function leggi(nome, u, query, corpo, valida) {
  if (!RFQ) throw erroreDedicata("PONTE_NON_CONFIGURATO", 0, "Il ponte non è configurato con la RFQ della pagina.");
  let r;
  try {
    r = await chiama(nome, u, query, corpo);
  } catch (err) {
    throw erroreDedicata("PONTE_RETE", 0, "La richiesta non è arrivata al server, o la risposta non si legge: " + messaggio(err));
  }
  if (r.login) {
    sessioneScaduta();
    throw erroreDedicata("PONTE_SESSIONE", 401, SESSIONE);
  }
  if (r.stato === 200 && r.json && (!valida || valida(r.json))) return r.json;
  if (r.stato === 200) throw erroreDedicata("PONTE_RISPOSTA", 200, "La risposta non ha la forma del contratto.", r.json);
  if (r.stato === 404 && !r.json) throw erroreDedicata("PONTE_ROTTA_ASSENTE", 404, testoPiano(r.testo) || "La rotta non c'è (non ancora nel server, o il motore A è spento).");
  const codice = codiceDi(r);
  if ((r.stato === 503 || r.stato === 504) && (OCCUPATO.includes(codice) || !codice)) {
    throw erroreDedicata(codice || (r.stato === 504 ? "CALCOLO_SCADUTO" : "CALCOLO_OCCUPATO"), r.stato, testoDi(r, "Il motore è occupato: riprova fra poco."), r.json);
  }
  if (r.stato === 422 && (RIFIUTI[nome] || []).includes(codice)) throw erroreDedicata(codice, 422, testoDi(r), r.json);
  throw erroreDedicata(codice || "PONTE_ERRORE_" + r.stato, r.stato, testoDi(r), r.json);
}

// leggiModello: GET /thread/{id}/motore-a/dati[?prodotto=<rif>]. Il modello della pagina (EM §9.9); l'errore di calcolo
// è un 200 con «errore» (D-18).
export async function leggiModello(prodotto) {
  const q = typeof prodotto === "string" && prodotto !== "" ? "?prodotto=" + encodeURIComponent(prodotto) : "";
  return leggi("leggiModello", "", q, undefined, null);
}

// effettoLeggibile: «effetto» della variante di R1 nella forma che la pagina legge (proposta PE-B05; RPC PD-05 ancora
// aperto): spento stringa, anche vuota, ed elenchi sospende, con_lei, resta_sospesa e avvisi, anche vuoti; rimozioni,
// autorizzabile, step_strutturale e svuota_step, se ci sono, del loro tipo. Un effetto con altri nomi (per esempio quelli
// della struct Go EffettoTipo senza tag json: «Spento», «Sospende»…) non si legge: senza questo controllo la pagina
// vedrebbe «nessun altro effetto» e accenderebbe «Cambia» per un cambio che il server dice spento.
function effettoLeggibile(e) {
  if (!oggetto(e) || typeof e.spento !== "string") return false;
  if (!["sospende", "con_lei", "resta_sospesa", "avvisi"].every((k) => Array.isArray(e[k]))) return false;
  if (e.rimozioni !== undefined && !(Number.isInteger(e.rimozioni) && e.rimozioni >= 0)) return false;
  if (e.autorizzabile !== undefined && typeof e.autorizzabile !== "boolean") return false;
  return ["step_strutturale", "svuota_step"].every((k) => e[k] === undefined || typeof e[k] === "string");
}

// leggiBozza: R1, GET …/motore-a/bozza?prodotto={uuid}[&eventi=1][&importazione=1][&registro=<impronta>][&confronto=1]
// (EM §9.1). L'oggetto con stato "aperta" o "nessuna_bozza".
// Con opz.effetto_cambio_tipo (un componente registrato) e opz.tipo_nuovo è la variante di lettura per l'effetto del
// cambio di tipo (MAT §1.3, BZ-06; decisioni dell'ondata A, PE-09): …&effetto_cambio_tipo={componente}&tipo_nuovo={tipo}
// → {componente_id, tipo_nuovo, effetto, firma_effetto}, senza scrivere; una variante come ?confronto=1 ed ?eventi=1,
// non una rotta nuova. La firma va com'è nella decisione cambia_tipo{componente_id, tipo_nuovo, firma_effetto} (EM §3.6
// riga 13; P-7, P-16). Una risposta senza la firma dell'effetto (una R1 che la variante non la conosce ancora), o con un
// «effetto» in una forma che la pagina non legge (effettoLeggibile), è PONTE_RISPOSTA: il gesto non parte.
export async function leggiBozza(prodotto, opz) {
  if (!uuid(prodotto)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Il prodotto non è un UUID.");
  const o = opz || {};
  let q = "?prodotto=" + prodotto.toLowerCase();
  if (o.effetto_cambio_tipo !== undefined) {
    if (!uuid(o.effetto_cambio_tipo)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Il componente del cambio di tipo non è un UUID.");
    if (!TIPI_COMPONENTE.includes(o.tipo_nuovo)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Il tipo nuovo non è uno dei tipi di componente.");
    const c = o.effetto_cambio_tipo.toLowerCase();
    q += "&effetto_cambio_tipo=" + c + "&tipo_nuovo=" + o.tipo_nuovo;
    return leggi("leggiBozza", "", q, undefined, (j) => typeof j.componente_id === "string" && j.componente_id.toLowerCase() === c && j.tipo_nuovo === o.tipo_nuovo &&
      typeof j.firma_effetto === "string" && RE_FIRMA_EFFETTO.test(j.firma_effetto) && effettoLeggibile(j.effetto));
  }
  // ?ingressi=1 (U2 §6.3.3; EM §9.1; MAT §1.5, WZ-11): la lettura leggera degli ingressi del riepilogo, solo database,
  // che risponde anche senza bozza. È una variante di lettura a sé, come effetto_cambio_tipo: niente altri parametri
  if (o.ingressi) {
    return leggi("leggiBozza", "", q + "&ingressi=1", undefined, (j) => typeof j.impronta_ingressi === "string" && oggetto(j.parti) &&
      ["bom", "bozza", "proposta", "analisi"].every((k) => oggetto(j.parti[k]) && typeof j.parti[k].impronta === "string") && Array.isArray(j.parti.analisi.in_analisi));
  }
  if (o.eventi) q += "&eventi=1";
  if (o.importazione) q += "&importazione=1";
  if (typeof o.registro === "string" && RE_FIRMA.test(o.registro)) q += "&registro=" + o.registro;
  if (o.confronto) q += "&confronto=1";
  // ?suggerimenti_motore=1 (giro G39, G39-01 e G39-02 [U]; distintabozza/adesso3.go): il calcolo del motore di adesso, con
  // preassociazioni_proposte[] anche dei file arrivati dopo l'apertura, suggerimenti_motore {calcolati, errore?} e
  // step_candidati[] {allegato_id, dal_motore}, anche per nessuna_bozza. Costa un calcolo: la pagina la chiede solo per
  // l'avviso della bozza manuale e dopo il 422 proposta_non_corrispondente di R2, mai a ogni lettura
  if (o.suggerimenti_motore) q += "&suggerimenti_motore=1";
  return leggi("leggiBozza", "", q, undefined, (j) => STATI_BOZZA.includes(j.stato));
}

// leggiSmistamento: S9, GET /thread/{id}/motore-a/dati?sorgente=1 (U2 §2.3; EM §10.9 «S9»): lo smistamento di tutti i
// prodotti, con le azioni per riga calcolate dal server. Costa come /dati: la pagina la rilegge solo dopo un gesto o un
// cambio della firma di S1, mai a intervalli (MAT §1.5). Il motore occupato è un 503 CALCOLO_OCCUPATO, niente scritto.
export async function leggiSmistamento() {
  return leggi("leggiSmistamento", "", "?sorgente=1", undefined, (j) => typeof j.firma === "string" && Array.isArray(j.prodotti) && Array.isArray(j.step));
}

// effettoS6: l'effetto sulla bozza di S6 (e gli elenchi della risposta di cambia_sorgente) con gli elenchi assenti letti
// come vuoti (DEC-B B-9: negli oggetti composti dal motore un elenco vuoto è assente). Solo presentazione, per la pagina.
const ELENCHI_EFFETTO = ["nodi_che_restano", "nodi_che_entrano", "fuori_per_cambio_fonte", "nuovi_nodi_proposti", "caselle_riscritte", "archi_riscritti",
  "identita_aggiornate", "da_rivedere", "decisioni_cadute", "preassociazioni_nel_vassoio"];
export function effettoS6(e) {
  const o = oggetto(e) ? Object.assign({}, e) : {};
  for (const k of ELENCHI_EFFETTO) o[k] = Array.isArray(o[k]) ? o[k] : [];
  o.differenze = oggetto(o.differenze) ? o.differenze : {};
  return o;
}

// motivoContenuto: il motivo per cui S5 non dà i byte di un file, letto dall'intestazione X-Motivo (EM §10.9: il visore
// pdf.js vede lo stato, non il corpo; DEC-B PF-25 e RF RQ-11 (c): anche le risposte senza busta portano X-Motivo). Una
// sola richiesta, di un byte (Range), fatta dalla pagina solo dopo che il visore non ha aperto il file: «Apri» resta un
// indirizzo, mai una fetch. Torna {stato, motivo}; motivo "" se la risposta non lo dice (per esempio un 2xx).
export async function motivoContenuto(allegato) {
  if (!RFQ || !uuid(allegato)) return { stato: 0, motivo: "" };
  try {
    const r = await chiama("urlContenuto", allegato, "?modo=anteprima", undefined, { Range: "bytes=0-0" });
    if (r.login) return { stato: 401, motivo: "SESSIONE_SCADUTA" };
    return { stato: r.stato, motivo: r.motivo || codiceDi(r) };
  } catch (err) {
    return { stato: 0, motivo: "" };
  }
}

// riepilogoBom: R4, POST di sola lettura …/motore-a/bom/riepilogo[?avvisi_motore=1] con la bozza formato 3 (EM §9.4).
// La forma minima: confermabile booleano e firma_anteprima di 64 esadecimali, la firma che poi va a confermaBom.
export async function riepilogoBom(bozza, opz) {
  if (!oggetto(bozza)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "La bozza non è un oggetto.");
  const q = opz && opz.motore ? "?avvisi_motore=1" : "";
  return leggi("riepilogoBom", "", q, bozza, (j) => typeof j.confermabile === "boolean" && typeof j.firma_anteprima === "string" && RE_FIRMA.test(j.firma_anteprima));
}

// esitoIntento: R6, GET …/motore-a/intento/{chiave} (EM §9.6). "registrato" e "registrato_senza_esito" chiudono il
// tentativo in sospeso con quella chiave; "non_registrato" lo lascia (verificato: lo stesso corpo si ritenta con la
// stessa chiave); "in_corso" lo lascia com'è (si richiede dopo qualche secondo).
export async function esitoIntento(chiave) {
  if (!uuid(chiave)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "La chiave non è un UUID.");
  const k = chiave.toLowerCase();
  const j = await leggi("esitoIntento", k, "", undefined, (x) => STATI_INTENTO.includes(x.stato));
  const s = sospesoPerChiave(k);
  if (s) {
    if (j.stato === "registrato" || j.stato === "registrato_senza_esito") togliSospeso(s.risorsa);
    else if (j.stato === "non_registrato") scriviSospeso(s.risorsa, Object.assign({}, s, { verificato: true }));
  }
  return j;
}

// storicoConferme: R7, GET …/motore-a/conferme?prodotto={uuid}[&conferma={uuid}][&importazione=1] (EM §9.7).
export async function storicoConferme(prodotto, opz) {
  if (!uuid(prodotto)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Il prodotto non è un UUID.");
  const o = opz || {};
  let q = "?prodotto=" + prodotto.toLowerCase();
  if (uuid(o.conferma)) q += "&conferma=" + o.conferma.toLowerCase();
  if (o.importazione) q += "&importazione=1";
  return leggi("storicoConferme", "", q, undefined, (j) => Array.isArray(j.conferme));
}

// leggiSorgente: S1, GET /thread/{id}/sorgente[?file={allegato}][&scartati=1] (EM §10.9).
export async function leggiSorgente(opz) {
  const o = opz || {};
  const p = new URLSearchParams();
  if (o.file !== undefined) {
    if (!uuid(o.file)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Il file non è un UUID.");
    p.set("file", o.file.toLowerCase());
  }
  if (o.scartati) p.set("scartati", "1");
  const q = p.toString() ? "?" + p.toString() : "";
  return leggi("leggiSorgente", "", q, undefined, (j) => Array.isArray(j.file));
}

// anteprimaStep: S6, GET …/sorgente/step/{allegato}/anteprima?prodotto=…[&gruppo=…][&bozza=…] (EM §10.9). Il gruppo è
// una stringa opaca: va com'è, mai scomposta.
export async function anteprimaStep(allegato, prodotto, opz) {
  if (!uuid(allegato) || !uuid(prodotto)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "Lo STEP o il prodotto non sono UUID.");
  const o = opz || {};
  const p = new URLSearchParams();
  p.set("prodotto", prodotto.toLowerCase());
  if (typeof o.gruppo === "string" && o.gruppo !== "" && o.gruppo.length <= 300) p.set("gruppo", o.gruppo);
  if (o.bozza !== undefined && o.bozza !== null) {
    if (!uuid(o.bozza)) throw erroreDedicata("PONTE_ARGOMENTO", 0, "La bozza non è un UUID.");
    p.set("bozza", o.bozza.toLowerCase());
  }
  return leggi("anteprimaStep", allegato, "?" + p.toString(), undefined, (j) => typeof j.stato === "string");
}

// urlContenuto: l'indirizzo di S5 (modo=anteprima|scarica), per pdf.js o per lo scaricamento: si apre come documento,
// mai con fetch (MAT §1.3). "" se l'allegato non è un UUID.
export function urlContenuto(allegato, modo) {
  if (!RFQ || !uuid(allegato)) return "";
  const m = modo === "scarica" ? "scarica" : "anteprima";
  return percorso("urlContenuto", allegato) + "?modo=" + m;
}

// ---------------------------------------------------------------- R2 e R3: la versione attesa e la chiave_salvataggio

// salvataggi: per ogni forma e bozza, il corpo in attesa di un esito certo con la sua chiave_salvataggio: lo stesso
// corpo riusa la chiave (il reinvio dopo una risposta persa riceve «riproposto», EM §9.2). Vive quanto la pagina: dopo
// una ricarica le modifiche non salvate non ci sono più, e R1 dice la verità.
const salvataggi = new Map();

function risorsaBozza(corpo) {
  return "bozza:" + String((corpo && corpo.prodotto_id) || "").toLowerCase();
}

// esitoScrittura: 200 con JSON → "ok"; 409 con un codice di CONFLITTI → "conflitto" (con motivo); 422 con un codice di
// RIFIUTI → "no"; 503/504 del calcolo → "errore" con il codice (niente scritto); ogni altra risposta → "errore". Un
// 200 che non si legge è "errore" con PONTE_RISPOSTA: la scrittura può essere avvenuta.
function esitoScrittura(nome, r) {
  const codice = codiceDi(r);
  if (r.stato === 200 && r.json) return { esito: "ok", codice: "", motivo: "", testo: testoDi(r, FATTO), dati: r.json };
  if (r.stato === 200) return { esito: "errore", codice: "PONTE_RISPOSTA", motivo: "", testo: SCRITTURA_ILLEGGIBILE, dati: null };
  if (r.stato === 409 && (CONFLITTI[nome] || []).includes(codice)) return { esito: "conflitto", codice, motivo: motivoDi(r), testo: testoDi(r), dati: r.json };
  if (r.stato === 422 && (RIFIUTI[nome] || []).includes(codice)) return { esito: "no", codice, motivo: motivoDi(r), testo: testoDi(r), dati: r.json };
  if ((r.stato === 503 || r.stato === 504) && OCCUPATO.includes(codice)) return { esito: "errore", codice, motivo: "", testo: testoDi(r, "Il motore è occupato: riprova fra poco. Niente è stato scritto."), dati: r.json };
  return { esito: "errore", codice: codice || "PONTE_ERRORE_" + r.stato, motivo: motivoDi(r), testo: testoDi(r), dati: r.json };
}

// scriviR2: una forma di R2. La chiave_salvataggio la mette il ponte: nuova per un corpo nuovo, la stessa per lo stesso
// corpo finché l'esito non è certo (200, 409, 422); una rete caduta, un 5xx o un 200 illeggibile la lasciano, e il
// reinvio dello stesso corpo la riusa.
async function scriviR2(nome, corpo) {
  if (!RFQ) return nonPartita("Il salvataggio non è partito: il ponte non è configurato.");
  if (!oggetto(corpo) || !uuid(corpo.prodotto_id)) return nonPartita("Il salvataggio non è partito: il corpo non ha il prodotto.");
  if (Object.prototype.hasOwnProperty.call(corpo, "chiave_salvataggio")) return nonPartita("La chiave_salvataggio la mette il ponte, non la pagina.");
  const risorsa = risorsaBozza(corpo);
  const impronta = canonico(corpo);
  const voce = nome + "|" + risorsa;
  const prima = salvataggi.get(voce);
  const chiave = prima && prima.impronta === impronta ? prima.chiave : nuovaChiave();
  salvataggi.set(voce, { impronta, chiave });
  let r;
  try {
    r = await inEsclusiva(risorsa, () => chiama(nome, "", "", Object.assign({}, corpo, { chiave_salvataggio: chiave })));
  } catch (err) {
    return { esito: "errore", codice: "PONTE_RETE", motivo: "", testo: "Il salvataggio non è arrivato al server, o la risposta non si legge: " + messaggio(err), dati: null, chiave_salvataggio: chiave };
  }
  if (r === null) return { esito: "errore", codice: "PONTE_IN_VOLO", motivo: "", testo: IN_VOLO, dati: null, chiave_salvataggio: chiave };
  if (r.login) {
    sessioneScaduta();
    return { esito: "errore", codice: "PONTE_SESSIONE", motivo: "", testo: SESSIONE, dati: null, chiave_salvataggio: chiave };
  }
  const e = esitoScrittura(nome, r);
  if (r.stato === 200 && r.json) salvataggi.delete(voce);
  else if (r.stato === 409 || r.stato === 422) salvataggi.delete(voce);
  e.chiave_salvataggio = chiave;
  return e;
}

function nonPartita(testo) {
  return { esito: "errore", codice: "PONTE_ARGOMENTO", motivo: "", testo, dati: null };
}

// salvaBozza: R2, forma salvataggio {bozza_id, versione_attesa, prodotto_id, vocabolario: 2, snapshot, azioni[], rebase?}
// (EM §9.2). Il ponte non compone e non cambia il corpo: aggiunge solo la chiave_salvataggio.
export async function salvaBozza(corpo) {
  if (!oggetto(corpo) || !uuid(corpo.bozza_id) || !oggetto(corpo.snapshot) || !Array.isArray(corpo.azioni)) {
    return nonPartita("Il salvataggio non è partito: servono la bozza, lo snapshot e le azioni.");
  }
  return scriviR2("salvaBozza", corpo);
}

// creaBozza: R2, forma creazione {bozza_id: null, versione_attesa: 0, prodotto_id, vocabolario: 2, crea: {partenza,
// sorgente?, impronta_contenuto?, motivo?, abbandona?}}. La versione 1 la costruisce il server, e la risposta porta lo
// snapshot (EM §9.2).
export async function creaBozza(corpo) {
  if (!oggetto(corpo) || !oggetto(corpo.crea)) return nonPartita("La creazione non è partita: manca «crea».");
  return scriviR2("creaBozza", corpo);
}

// cambiaSorgente: R2, forma cambia_sorgente {bozza_id, versione_attesa, …, cambia_sorgente: {sorgente,
// impronta_contenuto, motivo}} (EM §6.6).
export async function cambiaSorgente(corpo) {
  if (!oggetto(corpo) || !uuid(corpo.bozza_id) || !oggetto(corpo.cambia_sorgente)) return nonPartita("Il cambio della fonte non è partito: manca «cambia_sorgente».");
  return scriviR2("cambiaSorgente", corpo);
}

// riallineaProposta: R2, forma riallinea_proposta {bozza_id, versione_attesa, …, riallinea_proposta: {impronta_contenuto,
// motivo, scelte[]}} (EM §6.5). Mai chiamata «riallinea».
export async function riallineaProposta(corpo) {
  if (!oggetto(corpo) || !uuid(corpo.bozza_id) || !oggetto(corpo.riallinea_proposta)) return nonPartita("L'aggiornamento non è partito: manca «riallinea_proposta».");
  return scriviR2("riallineaProposta", corpo);
}

// convertiBozza: R2, forma converti_da {bozza_id: null, versione_attesa: 0, …, converti_da: {bozza_id, versione_attesa,
// motivo}} (EM §6.7).
export async function convertiBozza(corpo) {
  if (!oggetto(corpo) || !oggetto(corpo.converti_da) || !uuid(corpo.converti_da.bozza_id)) return nonPartita("La conversione non è partita: manca «converti_da».");
  return scriviR2("convertiBozza", corpo);
}

// abbandonaBozza: R3, POST …/motore-a/bozza/{bozza_id}/abbandona con {versione_attesa, motivo?} (EM §9.3).
export async function abbandonaBozza(id, corpo, prodotto) {
  if (!RFQ) return nonPartita("L'abbandono non è partito: il ponte non è configurato.");
  if (!uuid(id)) return nonPartita("L'abbandono non è partito: la bozza non è un UUID.");
  if (!oggetto(corpo)) return nonPartita("L'abbandono non è partito: il corpo non è un oggetto.");
  const risorsa = "bozza:" + String(prodotto || "").toLowerCase();
  let r;
  try {
    r = await inEsclusiva(risorsa, () => chiama("abbandonaBozza", id, "", corpo));
  } catch (err) {
    return { esito: "errore", codice: "PONTE_RETE", motivo: "", testo: "L'abbandono non è arrivato al server: " + messaggio(err), dati: null };
  }
  if (r === null) return { esito: "errore", codice: "PONTE_IN_VOLO", motivo: "", testo: IN_VOLO, dati: null };
  if (r.login) {
    sessioneScaduta();
    return { esito: "errore", codice: "PONTE_SESSIONE", motivo: "", testo: SESSIONE, dati: null };
  }
  return esitoScrittura("abbandonaBozza", r);
}

// ---------------------------------------------------------------- le scritture con la chiave: R5, S2, S3, S4, S8, R15

// La memoria di sessione della scheda (EM §10.9; MAT §5.5): per ogni risorsa (thread, prodotto o allegato) il tentativo
// dall'esito incerto {risorsa, nome, chiave, impronta, verificato, quando, etichetta} e, per i gesti dei file, l'allegato
// (u) e il corpo, per «Riprova» (riprovaSospeso). La chiave del valore porta l'utente: chi entra dopo sulla stessa scheda
// non trova il tentativo di un altro.
const PREFISSO = "cockpit.du.sospeso.";

function chiaveMemoria(risorsa) {
  return PREFISSO + UTENTE + "." + RFQ + "." + risorsa;
}

function leggiMemoria(k) {
  try {
    const v = sessionStorage.getItem(k);
    if (!v) return null;
    const o = JSON.parse(v);
    return oggetto(o) && uuid(o.chiave) ? o : null;
  } catch (e) {
    return null;
  }
}

function scriviSospeso(risorsa, s) {
  try {
    sessionStorage.setItem(chiaveMemoria(risorsa), JSON.stringify(s));
  } catch (e) {
    // senza memoria di sessione il tentativo vale quanto la pagina; R1, R7 e S1 fanno da ripiego (EM §10.9)
    memoriaVolatile.set(chiaveMemoria(risorsa), s);
  }
}

const memoriaVolatile = new Map();

function leggiSospeso(risorsa) {
  const k = chiaveMemoria(risorsa);
  return leggiMemoria(k) || memoriaVolatile.get(k) || null;
}

function togliSospeso(risorsa) {
  const k = chiaveMemoria(risorsa);
  memoriaVolatile.delete(k);
  try {
    sessionStorage.removeItem(k);
  } catch (e) {
    // niente
  }
}

function sospesoPerChiave(chiave) {
  for (const s of sospesi()) if (s.chiave === chiave) return s;
  return null;
}

// sospesi: i tentativi in sospeso di questa RFQ e di questo utente (la pagina li verifica con R6 all'apertura).
export function sospesi() {
  const out = [];
  const pre = PREFISSO + UTENTE + "." + RFQ + ".";
  try {
    for (let i = 0; i < sessionStorage.length; i++) {
      const k = sessionStorage.key(i);
      if (k && k.indexOf(pre) === 0) {
        const s = leggiMemoria(k);
        if (s) out.push(s);
      }
    }
  } catch (e) {
    // niente
  }
  for (const [k, s] of memoriaVolatile) if (k.indexOf(pre) === 0 && !out.some((x) => x.risorsa === s.risorsa)) out.push(s);
  return out;
}

// abbandonaSospeso: l'abbandono esplicito della chiave in sospeso da parte di chi guarda (MAT §5.5 n. 7), con l'avviso
// che la richiesta di prima potrebbe ancora arrivare. La pagina lo chiede prima di chiamarlo.
export function abbandonaSospeso(risorsa) {
  togliSospeso(risorsa);
}

// scriviConChiave: una scrittura con la chiave d'idempotenza. La chiave la mette il ponte, una per tentativo, e la
// riusa nei ritentativi dello stesso corpo finché l'esito non è certo. Un corpo diverso, con un tentativo in sospeso
// sulla stessa risorsa, non parte (PONTE_TENTATIVO_IN_SOSPESO), anche dopo un «non_registrato» di R6: la richiesta di
// prima potrebbe ancora arrivare, e la pagina fa abbandonare la chiave in modo esplicito (abbandonaSospeso; MAT §1.3,
// §5.5 n. 7). Fa eccezione il 409 COPIA_IN_CORSO, un esito certo del gestore (niente registrato, niente in volo):
// lì il tentativo è «certo» e un corpo diverso parte con una chiave nuova. Esiti:
//   - 200 → "ok" (chiude il tentativo);
//   - 409 di CONFLITTI → "conflitto" (chiude il tentativo, salvo COPIA_IN_CORSO, da ritentare con la stessa chiave);
//   - 422 di RIFIUTI → "no" (chiude);
//   - 500 con esito "non_registrato" (CONTROLLO_FINALE di R5) → "non_registrato" (chiude: esito certo); il 500
//     ERRORE_CALCOLO di R5 (il motore nella connessione S) e i 503/504 del calcolo → "errore", tentativo «certo» da
//     ritentare con la stessa chiave;
//   - 5xx, rete, 2xx illeggibile, sessione scaduta dopo l'invio → "da_verificare" (resta, non verificato);
//   - un rifiuto che non viene dal gestore (401 prima dell'invio, 403, 404, 405, 413, 409/422 con codici estranei) non
//     dice niente del tentativo di prima con la stessa chiave: torna com'era.
async function scriviConChiave(nome, risorsa, u, corpo, etichetta) {
  if (!RFQ) return Object.assign(nonPartita("Il gesto non è partito: il ponte non è configurato."), { chiave: "" });
  if (!oggetto(corpo)) return Object.assign(nonPartita("Il gesto non è partito: il corpo non è un oggetto."), { chiave: "" });
  const impronta = canonico({ gesto: nome, risorsa, u: u || "", corpo: impronteCorpo(nome, corpo) });
  const prima = leggiSospeso(risorsa);
  if (inVolo.has(risorsa)) return { esito: "errore", codice: "PONTE_IN_VOLO", motivo: "", testo: IN_VOLO, chiave: "", dati: null };
  if (prima && prima.impronta !== impronta && !prima.certo) {
    const testo = "Il gesto non è partito: un tentativo precedente su questa risorsa non ha un esito certo. Verificalo prima (chiave " + prima.chiave + ").";
    return { esito: "errore", codice: "PONTE_TENTATIVO_IN_SOSPESO", motivo: "", testo, chiave: prima.chiave, dati: null };
  }
  const riusato = prima && prima.impronta === impronta ? prima : null;
  const chiave = riusato ? riusato.chiave : nuovaChiave();
  // prima dell'invio: la chiave e l'impronta nella memoria di sessione della scheda; per i gesti dei file anche il corpo,
  // perché «Riprova» lo rimandi uguale anche dopo una ricarica (riprovaSospeso)
  const voce = { risorsa, nome, chiave, impronta, verificato: false, quando: new Date().toISOString(), etichetta: etichetta || "" };
  if (GESTI_CON_CORPO.includes(nome)) Object.assign(voce, { u: (u || "").toLowerCase(), corpo });
  scriviSospeso(risorsa, voce);
  const rimetti = () => {
    if (riusato) scriviSospeso(risorsa, riusato);
    else togliSospeso(risorsa);
  };
  let r;
  try {
    r = await inEsclusiva(risorsa, () => chiama(nome, u, "", conChiave(nome, corpo, chiave)));
  } catch (err) {
    return { esito: "da_verificare", codice: "PONTE_DA_VERIFICARE", motivo: "", testo: DA_VERIFICARE + " (" + messaggio(err) + ")", chiave, dati: null };
  }
  if (r === null) {
    rimetti();
    return { esito: "errore", codice: "PONTE_IN_VOLO", motivo: "", testo: IN_VOLO, chiave: "", dati: null };
  }
  if (r.login) {
    // la sessione è scaduta: il gestore non ha visto la richiesta, il tentativo di prima resta com'era; la pagina va al
    // login e al ritorno verifica le chiavi in sospeso con R6
    rimetti();
    sessioneScaduta();
    return { esito: "errore", codice: "PONTE_SESSIONE", motivo: "", testo: SESSIONE, chiave, dati: null };
  }
  const codice = codiceDi(r);
  if (r.stato === 200 && r.json) {
    // S8 «parziale» per un errore temporaneo di un passo (ritentabile: il motore occupato al passo 3, uno stallo): l'intento
    // non è scritto e lo stesso reinvio, con la stessa chiave e lo stesso corpo, riprende dal primo passo non fatto
    // (U2 §2.2; EM §10.9 «S8»). Il tentativo resta «certo» (niente in volo), da ritentare con la stessa chiave
    if (nome === "confermaSorgente" && r.json.esito === "parziale" && r.json.ritentabile === true) {
      scriviSospeso(risorsa, Object.assign({}, voce, { verificato: true, certo: true, codice: "S8_RITENTABILE", quando: new Date().toISOString() }));
    } else togliSospeso(risorsa);
    return { esito: "ok", codice: "", motivo: "", testo: testoDi(r, FATTO), chiave, dati: r.json };
  }
  if (r.stato === 409 && (CONFLITTI[nome] || []).includes(codice)) {
    // COPIA_IN_CORSO: esito certo (niente registrato, niente in volo), da ritentare con la stessa chiave; la pagina lo
    // dice come «copia in corso», non come un esito incerto (MAT §5.2, §5.4)
    if (codice === "COPIA_IN_CORSO") scriviSospeso(risorsa, Object.assign({}, voce, { verificato: true, certo: true, codice, motivo: motivoDi(r), quando: new Date().toISOString() }));
    else togliSospeso(risorsa);
    return { esito: "conflitto", codice, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
  }
  if (r.stato === 422 && (RIFIUTI[nome] || []).includes(codice)) {
    togliSospeso(risorsa);
    return { esito: "no", codice, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
  }
  // 500 ERRORE_CALCOLO di R5 con esito non_registrato (il calcolo del motore nella connessione S non è riuscito; blocco
  // «R4 completo» della persistenza): niente è scritto e non è il controllo finale; come il 503, il tentativo resta «certo»
  // e si ritenta con la stessa chiave
  if (nome === "confermaBom" && r.stato === 500 && r.json && r.json.esito === "non_registrato" && codice === "ERRORE_CALCOLO") {
    scriviSospeso(risorsa, Object.assign({}, voce, { verificato: true, certo: true, codice, motivo: motivoDi(r), quando: new Date().toISOString() }));
    return { esito: "errore", codice, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
  }
  if (r.stato === 500 && r.json && r.json.esito === "non_registrato") {
    togliSospeso(risorsa);
    return { esito: "non_registrato", codice: codice || "CONTROLLO_FINALE", motivo: "", testo: testoDi(r), chiave, dati: r.json };
  }
  if ((r.stato === 503 || r.stato === 504) && OCCUPATO.includes(codice)) {
    // un 503/504 del calcolo o della configurazione: il gestore ha detto che niente è stato scritto. R5 formato 3 risponde
    // 503 CALCOLO_OCCUPATO con esito non_registrato e la chiave: il tentativo resta «certo», da ritentare con la stessa
    // chiave (come COPIA_IN_CORSO; resoconto della persistenza, R5 §6)
    if (nome === "confermaBom" && r.json && r.json.esito === "non_registrato") {
      scriviSospeso(risorsa, Object.assign({}, voce, { verificato: true, certo: true, codice, motivo: motivoDi(r), quando: new Date().toISOString() }));
      return { esito: "errore", codice, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
    }
    togliSospeso(risorsa);
    return { esito: "errore", codice, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
  }
  if (r.stato >= 500 || (r.stato >= 200 && r.stato < 300)) {
    return { esito: "da_verificare", codice: codice || "PONTE_DA_VERIFICARE", motivo: "", testo: testoDi(r, DA_VERIFICARE), chiave, dati: r.json };
  }
  rimetti();
  return { esito: "errore", codice: codice || "PONTE_ERRORE_" + r.stato, motivo: motivoDi(r), testo: testoDi(r), chiave, dati: r.json };
}

// I gesti dei file con la chiave: il loro tentativo in sospeso porta anche il corpo (piccolo: ≤ 64 KiB, EM §10.9). Con
// U2 anche S8 (la conferma della sorgente) e R15 (il rifiuto della proposta aggiornata): «chiave e corpo nella memoria
// della scheda, come S2-S4» (MAT §1.3, §5.5).
const GESTI_FILE = ["scartaFile", "altriFile", "riattivaFile"];
const GESTI_CON_CORPO = GESTI_FILE.concat(["confermaSorgente", "revocaSorgente", "rifiutaProposta"]);

// riprovaSospeso: «Riprova» di un gesto dei file in sospeso, con il corpo e la chiave di allora, anche dopo una ricarica
// (EM §10.9, «Reinvio e risposta persa»: «non_registrato → si rimanda con la stessa chiave e lo stesso corpo»; MAT §5.5
// n. 4). Per la conferma della BOM no: si riprova dal wizard, che rilegge il riepilogo, e il ponte riusa la chiave se
// il corpo è lo stesso.
export async function riprovaSospeso(risorsa) {
  if (!RFQ) return Object.assign(nonPartita("Il gesto non è partito: il ponte non è configurato."), { chiave: "" });
  const s = leggiSospeso(String(risorsa || ""));
  const conU = !!(s && DEDICATE[s.nome] && DEDICATE[s.nome].indexOf("{u}") >= 0);
  if (!s || !GESTI_CON_CORPO.includes(s.nome) || !oggetto(s.corpo) || (conU && !uuid(s.u)) || s.risorsa !== risorsa) {
    return Object.assign(nonPartita("Il tentativo in sospeso non ha un corpo da rimandare: verificalo, o abbandonalo."), { chiave: "" });
  }
  return scriviConChiave(s.nome, risorsa, s.u, s.corpo, s.etichetta);
}

// impronteCorpo: la parte del corpo che decide se è «lo stesso corpo». R5: {firma_anteprima, bozza senza
// bozza_versione} nella forma del JSON canonico del contratto, come impronta_corpo del C3 §5.2 (EM §8.4, §3.10). Gli
// altri: il corpo com'è.
function impronteCorpo(nome, corpo) {
  if (nome !== "confermaBom") return corpo;
  const b = Object.assign({}, corpo.bozza || {});
  delete b.bozza_versione;
  const o = { firma_anteprima: corpo.firma_anteprima, bozza: b };
  // la presa d'atto dei documenti in analisi fa parte del corpo (U2 §6.3.7): un elenco diverso è un corpo diverso
  if (Array.isArray(corpo.presa_d_atto_in_analisi)) o.presa_d_atto_in_analisi = corpo.presa_d_atto_in_analisi.slice().sort();
  return formaCanonica(o, "");
}

// conChiave: il corpo con la chiave al suo posto (EM §9.5: dentro la bozza per R5; §10.9: al primo livello per S2-S4
// e S8, R15).
function conChiave(nome, corpo, chiave) {
  if (nome === "confermaBom") {
    const o = { firma_anteprima: corpo.firma_anteprima, bozza: Object.assign({}, corpo.bozza, { chiave_idempotenza: chiave }) };
    if (Array.isArray(corpo.presa_d_atto_in_analisi)) o.presa_d_atto_in_analisi = corpo.presa_d_atto_in_analisi;
    return o;
  }
  return Object.assign({}, corpo, { chiave_idempotenza: chiave });
}

// confermaBom: R5, POST …/motore-a/bom/conferma con {firma_anteprima, bozza} (EM §9.5). La bozza formato 3 è completa
// di bozza_id e bozza_versione, senza chiave_idempotenza: la mette il ponte (dentro la bozza). opz.presa = gli
// allegato_id dei documenti in analisi che il riepilogo ha mostrato e di cui chi conferma ha preso atto (U2 §6.3.7: il
// corpo porta presa_d_atto_in_analisi, al primo livello accanto a firma e bozza: scelta [T] della corsia, EM non dice
// dove sta); senza documenti in analisi il campo non c'è.
export async function confermaBom(firma, bozza, opz) {
  if (typeof firma !== "string" || !RE_FIRMA.test(firma)) return Object.assign(nonPartita("La conferma non è partita: serve la firma del riepilogo letto (64 esadecimali)."), { chiave: "" });
  if (!oggetto(bozza) || !uuid(bozza.prodotto_id) || !uuid(bozza.bozza_id)) return Object.assign(nonPartita("La conferma non è partita: la bozza non ha prodotto o bozza_id."), { chiave: "" });
  if (Object.prototype.hasOwnProperty.call(bozza, "chiave_idempotenza")) return Object.assign(nonPartita("La conferma non è partita: la chiave la mette il ponte, non la bozza."), { chiave: "" });
  const risorsa = "bozza:" + bozza.prodotto_id.toLowerCase();
  const corpo = { firma_anteprima: firma, bozza };
  const presa = opz && Array.isArray(opz.presa) ? opz.presa : null;
  if (presa && presa.length) {
    if (!presa.every(uuid)) return Object.assign(nonPartita("La conferma non è partita: la presa d'atto dei documenti in analisi non è un elenco di UUID."), { chiave: "" });
    corpo.presa_d_atto_in_analisi = presa.map((x) => x.toLowerCase());
  }
  return scriviConChiave("confermaBom", risorsa, "", corpo, "Conferma BOM");
}

function corpoFile(nome, allegato, corpo) {
  if (!uuid(allegato)) return null;
  if (!oggetto(corpo)) return null;
  if (Object.prototype.hasOwnProperty.call(corpo, "chiave_idempotenza")) return null;
  // nessun campo percorso dal browser (EM §10.9; D23:234): un nome che lo sembri non parte
  for (const k of Object.keys(corpo)) if (/percorso|path|staging/i.test(k)) return null;
  return corpo;
}

// scartaFile: S2, POST …/sorgente/file/{allegato}/scarta con {versione_attesa, motivo, impronta_dipendenze, n_voci?,
// impronta_voci?, scollega?, conferma_unica_copia?} (EM §10.9).
export async function scartaFile(allegato, corpo) {
  const c = corpoFile("scartaFile", allegato, corpo);
  if (!c) return Object.assign(nonPartita("Lo scarto non è partito: allegato o corpo non validi."), { chiave: "" });
  return scriviConChiave("scartaFile", "file:" + allegato.toLowerCase(), allegato, c, "Scarta");
}

// altriFile: S3, POST …/sorgente/file/{allegato}/altri-file con {versione_attesa, motivo?, impronta_dipendenze,
// n_voci?, impronta_voci?, scollega?} (EM §10.9).
export async function altriFile(allegato, corpo) {
  const c = corpoFile("altriFile", allegato, corpo);
  if (!c) return Object.assign(nonPartita("«Metti in Altri file» non è partito: allegato o corpo non validi."), { chiave: "" });
  return scriviConChiave("altriFile", "file:" + allegato.toLowerCase(), allegato, c, "Metti in Altri file");
}

// riattivaFile: S4, POST …/sorgente/file/{allegato}/riattiva con {versione_attesa, motivo?, n_voci?, impronta_voci?}
// (EM §10.9).
export async function riattivaFile(allegato, corpo) {
  const c = corpoFile("riattivaFile", allegato, corpo);
  if (!c) return Object.assign(nonPartita("«Riattiva» non è partito: allegato o corpo non validi."), { chiave: "" });
  return scriviConChiave("riattivaFile", "file:" + allegato.toLowerCase(), allegato, c, "Riattiva");
}

// confermaSorgente: S8, POST …/sorgente/step/{allegato}/conferma-sorgente (U2 §2.2; EM §10.9 «S8»; direttiva 39 §2-§3
// con la forma del server, motore_a_sorgente_v1.go e core/rfq/sorgente). Due partenze:
//   - "step" (predefinita): {prodotto_id, partenza?, gruppo, impronta_contenuto, sha256?, codice_atteso?, motivo?,
//     bozza?, scelte?, sostituisci?}, con il gruppo e l'impronta dell'anteprima (S6, V-7);
//   - "solo_prodotto": {prodotto_id, partenza: "solo_prodotto", stato_analisi, presa_d_atto: true, sha256?,
//     codice_atteso?, motivo, bozza?}, per uno STEP senza struttura importabile o con l'analisi in corso o fallita: lo
//     stato è quello mostrato (pronta | in_analisi | errore | non_valutato), la presa d'atto la spunta chi guarda; niente
//     gruppo, impronta né scelte.
// codice_atteso è il codice del PRODOTTO mostrato (il server risponde 409 ANTEPRIMA_SUPERATA anteprima «prodotto» se è
// cambiato), non il codice letto dal file. Una procedura del server a passi idempotenti: 200 «completata» o «parziale»
// (passi[], ritentabile), con l'esito di ogni passo. La risorsa è il prodotto («sorgente:<prodotto>»): il tentativo in
// sospeso porta chiave e corpo, e «Riprova» li rimanda uguali.
const STATI_ANALISI_S8 = ["pronta", "in_analisi", "errore", "non_valutato"];
export async function confermaSorgente(allegato, corpo) {
  const c = corpoFile("confermaSorgente", allegato, corpo);
  const no = (t) => Object.assign(nonPartita("La conferma della sorgente non è partita: " + t + "."), { chiave: "" });
  if (!c || !uuid(c.prodotto_id)) return no("STEP, prodotto o corpo non validi");
  const partenza = c.partenza === undefined ? "step" : c.partenza;
  if (partenza === "step") {
    if (typeof c.gruppo !== "string" || !c.gruppo || typeof c.impronta_contenuto !== "string" || !c.impronta_contenuto) return no("la partenza dallo STEP vuole gruppo e impronta dell'anteprima");
    if (c.stato_analisi !== undefined || c.presa_d_atto !== undefined) return no("stato dell'analisi e presa d'atto sono della partenza dal solo prodotto");
  } else if (partenza === "solo_prodotto") {
    if (c.gruppo !== undefined || c.impronta_contenuto !== undefined || c.scelte !== undefined) return no("la partenza dal solo prodotto non ha gruppo, impronta né scelte");
    if (!STATI_ANALISI_S8.includes(c.stato_analisi)) return no("manca lo stato dell'analisi mostrato");
    if (c.presa_d_atto !== true) return no("manca la presa d'atto dello stato dell'analisi");
  } else return no("la partenza è step oppure solo_prodotto");
  // G39-03 [U]: sposta_da, se c'è, è l'altro prodotto da cui lo STEP passa a questo; correzione_sorgente è un sì
  if (c.sposta_da !== undefined && (!uuid(c.sposta_da) || c.sposta_da.toLowerCase() === c.prodotto_id.toLowerCase())) return no("sposta_da è l'altro prodotto da cui la sorgente passa a questo");
  if (c.correzione_sorgente !== undefined && c.correzione_sorgente !== true) return no("correzione_sorgente, se c'è, è true");
  return scriviConChiave("confermaSorgente", "sorgente:" + c.prodotto_id.toLowerCase(), allegato, c, "Conferma della sorgente");
}

// revocaSorgente: la revoca della sorgente della bozza, POST …/sorgente/step/{allegato}/revoca-sorgente con {prodotto_id,
// motivo} (direttiva 39 §2; EM App. I.4; motore_a_sorgente_v1.go): per uno STEP scelto per errore, con il motivo. Non
// tocca la working, il documento, il NAS né la bozza; fino a una nuova S8 la conferma della BOM resta bloccata. Stessa
// risorsa di S8 («sorgente:<prodotto>»): una revoca e una conferma dello stesso prodotto non sono mai in volo insieme.
export async function revocaSorgente(allegato, corpo) {
  const c = corpoFile("revocaSorgente", allegato, corpo);
  if (!c || !uuid(c.prodotto_id) || typeof c.motivo !== "string" || !c.motivo.trim()) {
    return Object.assign(nonPartita("La revoca della sorgente non è partita: STEP, prodotto o motivo non validi."), { chiave: "" });
  }
  // G39-03 [U]: la causa sorgente_errata è un sì; la bozza dichiarata ha bozza_id e versione_attesa (almeno 1)
  if ((c.sorgente_errata !== undefined && c.sorgente_errata !== true) ||
    (c.bozza !== undefined && !(oggetto(c.bozza) && uuid(c.bozza.bozza_id) && Number.isInteger(c.bozza.versione_attesa) && c.bozza.versione_attesa >= 1))) {
    return Object.assign(nonPartita("La revoca della sorgente non è partita: la causa o la bozza dichiarata non sono valide."), { chiave: "" });
  }
  return scriviConChiave("revocaSorgente", "sorgente:" + c.prodotto_id.toLowerCase(), allegato, c, "Revoca della sorgente");
}

// rifiutaProposta: R15, POST …/motore-a/proposta-aggiornata/rifiuta con {prodotto_id, conferma_id, gruppo,
// impronta_contenuto, motivo} (U2 §5.7; EM §9.5-bis). Solo senza bozza: con la bozza il rifiuto è «Mantieni» per tutte
// nel riallineamento (BZ-17). Non è uno scarto (file) né un abbandono (bozza).
export async function rifiutaProposta(corpo) {
  if (!oggetto(corpo) || !uuid(corpo.prodotto_id) || !uuid(corpo.conferma_id) || typeof corpo.motivo !== "string" || !corpo.motivo.trim() ||
    Object.prototype.hasOwnProperty.call(corpo, "chiave_idempotenza")) {
    return Object.assign(nonPartita("Il rifiuto non è partito: prodotto, conferma o motivo non validi."), { chiave: "" });
  }
  return scriviConChiave("rifiutaProposta", "proposta:" + corpo.prodotto_id.toLowerCase(), "", corpo, "Rifiuta la proposta aggiornata");
}

// ---------------------------------------------------------------- R11 ed E1′

// riallineaDocumenti: R11, POST …/motore-a/riallinea (EM §9.8): rifà F8. Il nome non è «riallinea» da solo.
export async function riallineaDocumenti() {
  if (!RFQ) return nonPartita("Il riallineamento non è partito: il ponte non è configurato.");
  let r;
  try {
    r = await inEsclusiva("f8", () => chiama("riallineaDocumenti", "", "", {}));
  } catch (err) {
    return { esito: "errore", codice: "PONTE_RETE", motivo: "", testo: "Il riallineamento dei documenti non è arrivato al server: " + messaggio(err), dati: null };
  }
  if (r === null) return { esito: "errore", codice: "PONTE_IN_VOLO", motivo: "", testo: IN_VOLO, dati: null };
  if (r.login) {
    sessioneScaduta();
    return { esito: "errore", codice: "PONTE_SESSIONE", motivo: "", testo: SESSIONE, dati: null };
  }
  if (r.stato === 200 && r.json) return { esito: "ok", codice: "", motivo: "", testo: testoDi(r, "Documenti riallineati."), dati: r.json };
  if (r.stato === 422 && RIFIUTI.riallineaDocumenti.includes(codiceDi(r))) return { esito: "no", codice: codiceDi(r), motivo: motivoDi(r), testo: testoDi(r), dati: r.json };
  return { esito: "errore", codice: codiceDi(r) || "PONTE_ERRORE_" + r.stato, motivo: "", testo: testoDi(r, "Riallineamento dei documenti non riuscito: riprova."), dati: r.json };
}

// inviaConInattivita: il trasporto del caricamento (E1′, commento di testa). Una POST multipart con XMLHttpRequest, che
// dice i byte inviati (upload.onprogress) e quelli ricevuti (onprogress): ogni byte che si muove riarma il tempo
// d'inattività, e solo se per INATTIVITA non se ne muove nessuno la richiesta si ferma, con PONTE_INATTIVITA. Nessun
// tempo massimo totale: un caricamento lento che avanza arriva in fondo. progresso(inviati, totale), se c'è, riceve
// l'avanzamento dell'invio. → {stato, testo, tipo, hxRedirect, url}, oppure lancia (rete caduta, inattività).
function inviaConInattivita(indirizzo, corpo, progresso) {
  return new Promise((risolvi, rifiuta) => {
    const x = new XMLHttpRequest();
    let timer = 0;
    let chiuso = false;
    const fine = (fn, v) => {
      if (chiuso) return;
      chiuso = true;
      clearTimeout(timer);
      fn(v);
    };
    const riarma = () => {
      if (chiuso) return;
      clearTimeout(timer);
      timer = setTimeout(() => {
        fine(rifiuta, erroreDedicata("PONTE_INATTIVITA", 0, "nessun byte inviato né ricevuto per " + Math.round(INATTIVITA / 1000) + " s"));
        try {
          x.abort();
        } catch (e) {
          // già chiusa
        }
      }, INATTIVITA);
    };
    x.open("POST", indirizzo);
    x.setRequestHeader("Accept", "application/json");
    x.upload.addEventListener("loadstart", riarma);
    x.upload.addEventListener("progress", (e) => {
      riarma();
      if (progresso && e.lengthComputable) {
        try {
          progresso(e.loaded, e.total);
        } catch (err) {
          // l'avanzamento è solo presentazione
        }
      }
    });
    x.upload.addEventListener("load", riarma);
    x.addEventListener("readystatechange", () => {
      if (x.readyState === 2 || x.readyState === 3) riarma();
    });
    x.addEventListener("progress", riarma);
    x.addEventListener("load", () => fine(risolvi, { stato: x.status, testo: x.responseText || "", tipo: x.getResponseHeader("Content-Type") || "",
      hxRedirect: x.getResponseHeader("HX-Redirect") || "", url: x.responseURL || "" }));
    x.addEventListener("error", () => fine(rifiuta, new Error("la rete non risponde")));
    x.addEventListener("abort", () => fine(rifiuta, new Error("richiesta interrotta")));
    riarma();
    x.send(corpo);
  });
}

// rimandoAlLoginDi: come rimandoAlLogin, per la risposta di inviaConInattivita (XMLHttpRequest segue da sé i
// rimandi: un 302 verso /login si vede dall'indirizzo finale).
function rimandoAlLoginDi(r) {
  if (r.stato === 401) return r.hxRedirect && percorsoDi(r.hxRedirect) === "/login" ? r.hxRedirect : "/login";
  if (r.hxRedirect && percorsoDi(r.hxRedirect) === "/login") return r.hxRedirect;
  if (r.url && percorsoDi(r.url) === "/login") return "/login";
  return "";
}

// carica: E1′, il ramo JSON di POST /thread/{id}/carica (multipart, Accept: application/json; EM §10.9). origine =
// "portale" (del cliente) | "interna". Nessuna assegnazione a un componente all'ingresso (D23:276). Il reinvio dopo una
// risposta persa è idempotente per contenuto: dà «gia_presente». Il trasporto ha il tempo d'inattività, non il tempo
// massimo delle chiamate JSON (inviaConInattivita); progresso(inviati, totale) riceve l'avanzamento dell'invio.
export async function carica(files, origine, progresso) {
  if (!RFQ) return { esito: "errore", codice: "PONTE_NON_CONFIGURATO", testo: "Il caricamento non è partito: il ponte non è configurato.", file: [], dati: null };
  const elenco = Array.from(files || []).filter((f) => typeof File !== "undefined" && f instanceof File);
  if (elenco.length === 0) return { esito: "errore", codice: "PONTE_ARGOMENTO", testo: "Nessun file scelto.", file: [], dati: null };
  if (origine !== "portale" && origine !== "interna") return { esito: "errore", codice: "PONTE_ARGOMENTO", testo: "L'origine del file non è valida.", file: [], dati: null };
  const fd = new FormData();
  fd.append("origine", origine);
  for (const f of elenco) fd.append("file", f, f.name);
  let r;
  try {
    r = await inEsclusiva("carica", async () => {
      const x = await inviaConInattivita(percorso("carica", ""), fd, typeof progresso === "function" ? progresso : null);
      const login = rimandoAlLoginDi(x);
      if (login) return { login };
      let json = null;
      if (x.tipo.toLowerCase().indexOf("json") >= 0) {
        try {
          json = JSON.parse(x.testo);
        } catch (e) {
          json = null;
        }
      }
      return { stato: x.stato, json: oggetto(json) ? json : null, testo: x.testo };
    });
  } catch (err) {
    if (err && err.codice === "PONTE_INATTIVITA") {
      return { esito: "errore", codice: "PONTE_INATTIVITA", testo: "Il caricamento si è fermato: " + messaggio(err) + ". Non si sa se i file sono entrati: rimanda gli stessi file (quelli già entrati risulteranno «già presente»).", file: [], dati: null };
    }
    return { esito: "errore", codice: "PONTE_RETE", testo: "Il caricamento non è arrivato al server, o la risposta non si legge: rimanda gli stessi file (quelli già entrati risulteranno «già presente»). " + messaggio(err), file: [], dati: null };
  }
  if (r === null) return { esito: "errore", codice: "PONTE_IN_VOLO", testo: IN_VOLO, file: [], dati: null };
  if (r.login) {
    sessioneScaduta();
    return { esito: "errore", codice: "PONTE_SESSIONE", testo: SESSIONE, file: [], dati: null };
  }
  const codice = codiceDi(r);
  const fileRisp = r.json && Array.isArray(r.json.file) ? r.json.file : [];
  if (r.stato === 200 && r.json && (r.json.esito === "caricati" || r.json.esito === "parziale")) {
    return { esito: r.json.esito, codice: "", testo: testoDi(r, ""), file: fileRisp, dati: r.json };
  }
  if (r.stato === 200) return { esito: "errore", codice: "PONTE_RISPOSTA", testo: "La risposta del caricamento non si legge: rileggi la Sorgente prima di riprovare.", file: [], dati: r.json };
  if (r.stato === 422 && RIFIUTI.carica.includes(codice)) return { esito: "no", codice, testo: testoDi(r), file: fileRisp, dati: r.json };
  if (r.stato === 413) return { esito: "no", codice: codice || "TROPPO_GRANDE", testo: testoDi(r, "Troppo grande: dividi in più invii."), file: fileRisp, dati: r.json };
  if (r.stato === 503) return { esito: "errore", codice: codice || "NON_CONFIGURATO", testo: testoDi(r, "Il caricamento non è configurato su questo server."), file: [], dati: r.json };
  return { esito: "errore", codice: codice || "PONTE_ERRORE_" + r.stato, testo: testoDi(r, "Il caricamento non è riuscito: rileggi la Sorgente prima di riprovare."), file: fileRisp, dati: r.json };
}

// ---------------------------------------------------------------- i cinque atti esistenti (lista chiusa ridotta)

// leggiEsito: l'esito di un atto, letto come TESTO da un solo <div data-ma-esito> e nient'altro (rispondiMotoreA,
// motore_a.go). Un corpo d'altra forma → null.
function leggiEsito(corpo) {
  if (typeof DOMParser === "undefined") return null;
  const doc = new DOMParser().parseFromString(String(corpo || ""), "text/html");
  if (!doc.body || (doc.head && doc.head.childNodes.length > 0)) return null;
  const el = doc.body.firstElementChild;
  if (!el || doc.body.childElementCount !== 1 || el.tagName !== "DIV" || !el.hasAttribute("data-ma-esito") || el.childElementCount !== 0) return null;
  for (const n of doc.body.childNodes) if (n !== el && (n.nodeType !== 3 || n.textContent.trim() !== "")) return null;
  const v = el.getAttribute("data-ma-esito");
  const testo = (el.textContent || "").replace(/\s+/g, " ").trim();
  if (v === "ok") return { esito: "ok", testo: testo || FATTO };
  if (v === "no") return { esito: "no", testo: testo || NIENTE };
  return { esito: "errore", testo: "Esito sconosciuto «" + v + "»: non si sa se il gesto è riuscito. " + testo };
}

function valoreCampo(v) {
  if (typeof v === "string") return v;
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  throw new TypeError("valore di un campo non ammesso");
}

// esegui: uno dei tre atti di ATTI, con i campi di form. u = l'UUID del percorso, se la rotta lo vuole (nessuno dei tre
// atti di oggi). HX-Request e HX-Current-URL = la pagina /thread/{id}/motore-a: threadFrammento risponde con il solo
// <div data-ma-esito> (dalMotoreA, motore_a.go), qualunque sia l'indirizzo del guscio. Limite dichiarato (EM §9.10):
// gli atti rispondono 200 anche a un rifiuto e fanno girare F8 anche dopo un rifiuto.
export async function esegui(atto, campi, u) {
  if (!RFQ) return { esito: "errore", testo: "L'atto non è partito: il ponte non è configurato." };
  const rotta = ATTI[atto];
  if (!rotta) return { esito: "errore", testo: "L'atto non è partito: non è nella lista chiusa del ponte." };
  if (rotta.indexOf("{u}") >= 0 && !uuid(u)) return { esito: "errore", testo: "L'atto non è partito: il componente non è un UUID." };
  const p = new URLSearchParams();
  try {
    for (const [k, v] of Object.entries(campi || {})) {
      if (Array.isArray(v)) v.forEach((x) => p.append(k, valoreCampo(x)));
      else p.append(k, valoreCampo(v));
    }
  } catch (e) {
    return { esito: "errore", testo: "I campi dell'atto non si leggono (solo stringhe e numeri): niente è partito." };
  }
  const g = GUARDIE[atto];
  const motivo = g ? g(p) : "";
  if (motivo) return { esito: "errore", testo: "L'atto non è partito: " + motivo + "." };
  const indirizzo = rotta.replace("{id}", RFQ).replace("{u}", (u || "").toLowerCase());
  let r;
  try {
    r = await inEsclusiva("atto", async () => {
      const risp = await fetch(indirizzo, {
        method: "POST",
        credentials: "same-origin",
        headers: { "HX-Request": "true", "HX-Current-URL": location.origin + "/thread/" + RFQ + "/motore-a", "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body: p.toString(),
        signal: segnale(),
      });
      const login = rimandoAlLogin(risp);
      if (login) return { login };
      const corpo = await risp.text();
      if (!risp.ok) return { esito: "errore", testo: testoPiano(corpo) || "Errore " + risp.status };
      const html = (risp.headers.get("Content-Type") || "").trim().toLowerCase().startsWith("text/html");
      if (risp.status !== 200 || !html) return { esito: "errore", testo: SENZA_ESITO };
      return leggiEsito(corpo) || { esito: "errore", testo: SENZA_ESITO };
    });
  } catch (err) {
    return { esito: "errore", testo: "L'atto non è arrivato al server, o la risposta non si legge: " + messaggio(err) };
  }
  if (r === null) return { esito: "errore", testo: IN_VOLO };
  if (r.login) {
    sessioneScaduta();
    return { esito: "errore", testo: SESSIONE };
  }
  return r;
}

// ---------------------------------------------------------------- per le prove di contratto (lettura sola)

// catalogo: le rotte, i cataloghi e gli atti del ponte, in sola lettura (le prove L1 e la prova nel browser li
// confrontano con il contratto).
export function catalogo() {
  return JSON.parse(JSON.stringify({ dedicate: DEDICATE, conflitti: CONFLITTI, rifiuti: RIFIUTI, atti: ATTI, forme_r2: FORME_R2 }));
}
