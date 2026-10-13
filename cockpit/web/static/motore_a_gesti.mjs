// motore_a_gesti.mjs — il ponte dei gesti della Distinta nuova del motore A (giro 5, A1d-P3; direttiva A1d dell'11/10
// §4; mappa dei gesti A1d-P3, §0 e §2).
//
// Deriva dal ponte della PR #18 di Stefano Valo (c543e89, web/static/motore_a_gesti.mjs), con queste differenze:
//   - la lista chiusa ROTTE ha 27 POST (mappa §2.1): fuori «Accetta la relazione» (Q-A1d-06 = A, decisione
//     dell'utente: la BOM si conferma dal riepilogo con «Conferma l'albero») e «Scarta la relazione» (sospesa: base di
//     lavoro di Q-P3-01, fuori finché non è decisa);
//   - le guardie di invio sui NOMI dei campi (GUARDIE; Q-P3-02, base di lavoro A), sullo schema della guardia «auto»
//     che c'era già: controlli prima dell'invio, nessuna logica di dominio. Un campo che il gestore legge con
//     r.FormValue (il primo valore) si controlla com'è letto: una volta sola, e non vuoto dove vuoto non è ammesso;
//   - HX-Current-URL è sempre l'indirizzo della pagina /thread/{id}/motore-a, senza query;
//   - «Conferma l'albero» manda la bozza vuota della v1, {formato: 1, base}: confermaAlbero(firma, base);
//   - un solo stato «occupato» in tutta la pagina: un gesto in volo o la rilettura di #ma che lo segue;
//   - la sessione scaduta è un 401, un HX-Redirect verso /login o un rimando seguito fino a /login: un HX-Redirect
//     verso altro non porta da nessuna parte;
//   - l'esito lo dice il server (rispondiMotoreA, internal/transport/web/motore_a.go) con avvisoNegativo così com'è
//     (Q-A1d-09 = A): il ponte non classifica le frasi.
//
// API (la pagina /thread/{id}/motore-a, che arriva con A1d-P4, lo importa e non lo modifica):
//
//   esegui(url, campi)          → Promise<{esito: "ok"|"no"|"errore", testo}>; dopo "ok" rilegge #ma
//   leggi(url)                  → GET JSON di sola lettura della lista LETTURE → l'oggetto; un Error con il testo
//   confermaAlbero(firma, base) → POST JSON di «Conferma l'albero» con la bozza vuota → {esito, testo}; dopo "ok" rilegge
//   aggiorna()                  → Promise<boolean>: rilegge #ma con htmx; true = riletta
//
// e intercetta da solo click e submit degli elementi con data-ma-gesto (POST), con data-ma-campi (JSON di campi fissi:
// solo stringhe e numeri; gli array diventano campi ripetuti, senza []) e data-ma-domanda (confirm() prima dell'invio).
//
// Le regole (direttiva §4.1 e §4.3):
// - il frontend trasmette l'intenzione dell'operatore, il backend decide e registra l'esito. Il ponte non conferma
//   niente da sé, non ricostruisce gerarchie, non trasforma tipi o quantità, non mette valori di quantità impliciti,
//   non simula un esito e non ha un modo dimostrativo;
// - è l'unico punto della pagina che chiama le rotte che scrivono, e solo quelle della lista chiusa ROTTE (più ALBERO,
//   a parte): stessa origine, l'indirizzo senza parametri, il {id} uguale alla RFQ della pagina (senza badare alle
//   maiuscole). La RFQ si prende solo dal percorso della pagina: fuori da /thread/{id}/motore-a nessun gesto parte;
// - ogni richiesta, anche le GET JSON, porta HX-Request: true e HX-Current-URL = la pagina: così threadFrammento
//   risponde con il solo <div data-ma-esito="ok|no">testo</div>, mai con la vecchia interfaccia. Il corpo dei gesti è
//   application/x-www-form-urlencoded, con i campi ripetuti senza [];
// - il corpo della risposta di un gesto non entra mai nella pagina: DOMParser lo legge in un documento a parte, che non
//   esegue niente e non si inserisce; nella pagina va solo il testo, con textContent, in #ma-avviso;
// - gli esiti: un 200 text/html con un solo <div data-ma-esito="ok|no"> → "ok" o "no"; ogni altro 2xx, o un 200 senza
//   quel div o d'altro tipo → "errore" (non si sa se il gesto è riuscito: mai un successo); 403, ogni altro 4xx e i 5xx
//   → "errore" con il testo; la sessione scaduta lo dice e porta al login;
// - dopo un "ok" verificato, anche di confermaAlbero, rilegge #ma con htmx (GET della pagina); dopo "no" o "errore" no,
//   anche se il rismistamento legacy può aver scritto dopo un rifiuto con 200 (TEC-03 della mappa: dichiarato, non
//   corretto qui). Se la rilettura non riesce, #ma-avviso lo dice: mai ricaricare la pagina da sé;
// - la rilettura è l'unico punto in cui entra nella pagina HTML del server, e solo a queste condizioni (l'eccezione
//   della revisione di P3, P3-PONTE-02; il contratto del template è di A1d-P4): la GET della pagina stessa, con un limite
//   di tempo (RILETTURA_MS); la risposta dello stesso percorso, senza rimandi seguiti altrove; un frammento con un solo
//   elemento data-ma-pagina = l'id della RFQ, senza #ma né #ma-avviso, senza <script>, senza attributi htmx che
//   chiamino il server (hx-get, hx-post, hx-put, hx-patch, hx-delete), che sostituiscano fuori banda (hx-swap-oob) o
//   che eseguano codice (hx-on). Altrimenti niente sostituzione e l'avviso;
// - un gesto alla volta: mentre uno è in volo, o la pagina si sta rileggendo, un secondo non parte; i pulsanti
//   dell'elemento restano spenti fino alla fine della rilettura.
//
// Vincoli del file: nessun import, nessun await fuori dalle funzioni, nessun timer, solo le globali del DOM (window,
// document, location, fetch, DOMParser, URL, URLSearchParams, FormData, confirm) e window.htmx.

const INSTALLATO = "__maGesti";
const FATTO = "Fatto: il server non ha aggiunto altro."; // un "ok" senza testo
const NIENTE = "Niente è cambiato.";
const RILETTURA = "La pagina non si è riletta: ricaricala per vedere lo stato nuovo.";
const SESSIONE = "Sessione scaduta: si torna al login.";
const IN_VOLO = "Il gesto non è partito: un altro gesto, o la rilettura della pagina che lo segue, è in corso: aspetta il suo esito.";
const SENZA_ESITO =
  "La risposta non dice l'esito (manca data-ma-esito): non si sa se il gesto è riuscito. Ricarica la pagina per vedere lo stato.";
// RILETTURA_MS [T]: il limite di tempo della rilettura di #ma (htmx non ne ha uno suo). Più del limite di ripiego delle
// letture del motore A (30 s): allo scadere la rilettura fallisce, l'avviso lo dice e la pagina torna libera.
const RILETTURA_MS = 60000;

// ROTTE:inizio — le 27 POST della mappa dei gesti A1d-P3, §2.1, nello stesso ordine. Campi di form. {id} = la RFQ
// della pagina, {u} = un UUID. Fuori: relazione/accetta (Q-A1d-06 [U]), relazione/scarta (Q-P3-01), bom/applica,
// file/{aid}/accetta, proposta/{pid}/decidi, /proposta/{id}/conferma, step-strutturale/riattiva, note, congela,
// revisione/*, deroga-struttura, annulla-sostituzione (mappa §1.F, §2.2).
const ROTTE = [
  "/thread/{id}/fascicolo/componente/{u}/step-strutturale",
  "/thread/{id}/fascicolo/componente/{u}/step-strutturale/revoca",
  "/thread/{id}/fascicolo/nodo/{u}/accetta",
  "/thread/{id}/fascicolo/nodo/{u}/scarta",
  "/thread/{id}/fascicolo/nodo/{u}/codice",
  "/thread/{id}/fascicolo/nodo/{u}/riapri",
  "/thread/{id}/fascicolo/componente/{u}/codice",
  "/thread/{id}/fascicolo/componente/{u}/modifica",
  "/thread/{id}/fascicolo/componente/{u}/tipo",
  "/thread/{id}/fascicolo/componente/{u}/collega",
  "/thread/{id}/fascicolo/componente/{u}/sposta",
  "/thread/{id}/fascicolo/componente/{u}/scollega",
  "/thread/{id}/fascicolo/componente/{u}/archivia",
  "/thread/{id}/fascicolo/componente/{u}/rimuovi",
  "/thread/{id}/fascicolo/componente/{u}/ripristina",
  "/thread/{id}/fascicolo/rimozione/accetta",
  "/thread/{id}/fascicolo/rimozione/scarta",
  "/thread/{id}/fascicolo/conferma",
  "/proposta/{u}/scarta",
  "/thread/{id}/fascicolo/assegna",
  "/thread/{id}/fascicolo/documento/{u}/sostituisci",
  "/thread/{id}/fascicolo/componente/{u}/deroga",
  "/thread/{id}/fascicolo/deroga/{u}/revoca",
  "/thread/{id}/riprova-copie",
  "/thread/{id}/fascicolo/prepara",
  "/thread/{id}/fascicolo/rianalizza",
  "/thread/{id}/smistamento/aggiorna",
];
// ROTTE:fine

// LETTURE:inizio — le GET JSON di sola lettura della pagina (con la query).
const LETTURE = [
  "/thread/{id}/motore-a/dati",
  "/thread/{id}/motore-a/anteprima/step-strutturale",
  "/thread/{id}/distinta/tipo",
  "/thread/{id}/distinta/albero/riepilogo",
];
// LETTURE:fine

// «Conferma l'albero»: fuori da ROTTE, la chiama solo confermaAlbero, con la RFQ della pagina. L'esito viene dallo stato
// HTTP (200 / 422 / altro), non da avvisoNegativo: la rotta non passa da threadFrammento.
const ALBERO = "/thread/{id}/distinta/albero/conferma";

// presente: il primo dei nomi che c'è fra i campi; "" se nessuno.
function presente(p, nomi) {
  for (const n of nomi) if (p.has(n)) return n;
  return "";
}

// conPrefisso: il primo campo il cui nome comincia con il prefisso; "" se nessuno.
function conPrefisso(p, prefisso) {
  for (const k of p.keys()) if (k.startsWith(prefisso)) return k;
  return "";
}

// unico: un campo come lo legge il gestore con r.FormValue, cioè il PRIMO valore. Deve esserci una volta sola e, se
// vuoto non è ammesso, non vuoto dopo trim: la guardia e il server guardano così lo stesso valore (P3-PONTE-01: con
// qta=&qta=3 il server leggeva il primo, vuoto, e scriveva ×1). Restituisce il motivo, o "" se va bene.
function unico(p, n, ancheVuoto) {
  const v = p.getAll(n);
  if (v.length === 0) return "manca «" + n + "»";
  if (v.length > 1) return "«" + n + "» è ripetuto";
  if (!ancheVuoto && v[0].trim() === "") return "«" + n + "» è vuoto";
  return "";
}

// GUARDIE:inizio — le guardie di invio della mappa §2.1 (ultima colonna; Q-P3-02, base di lavoro A): controlli sui
// NOMI dei campi prima dell'invio, sullo schema della guardia «auto». Ognuna restituisce il motivo del rifiuto, o "".
// Non decidono niente del dominio: con un campo vietato, mancante o ripetuto il gesto non parte, e lo dice.
const GUARDIE = {
  // Q-A1d-07 (raccomandazione A): «Accetta il nodo» non offre commerciale; il commerciale passa da «Cambia il tipo»,
  // con anteprima e firma. Il ponte non filtra finito (Q-P3-06): lo decide il template di P4. Più stretta del server
  // (guarda tutti i valori di tipo, non solo il primo).
  "/thread/{id}/fascicolo/nodo/{u}/accetta": (p) =>
    p.getAll("tipo").some((v) => v.trim().toLowerCase() === "commerciale")
      ? "«Accetta il nodo» non offre il tipo commerciale: si cambia con «Cambia il tipo», con anteprima e firma"
      : "",
  // rev e descrizione sempre tutti e due, una volta sola (omesso = vuoto = NULL nel gestore); niente tipo: i tipi
  // passano solo da componente/{u}/tipo
  "/thread/{id}/fascicolo/componente/{u}/modifica": (p) => {
    if (p.has("tipo")) return "«Rev e descrizione» non cambia il tipo: si cambia con «Cambia il tipo»";
    const m = unico(p, "rev", true) || unico(p, "descrizione", true);
    return m ? "«Rev e descrizione» manda sempre rev e descrizione insieme, una volta sola (" + m + ")" : "";
  },
  // nessuna quantità implicita: il gestore legge il primo valore di qta, e uno vuoto come 1 (TEC-13); il ponte la vuole
  // scritta, una volta sola
  "/thread/{id}/fascicolo/componente/{u}/collega": (p) => {
    const m = unico(p, "qta", false);
    return m ? "la quantità va scritta, una volta sola: il ponte non ne mette una da sé (" + m + ")" : "";
  },
  // da e a sempre presenti, una volta sola, anche vuoti come scelta esplicita, mai un campo dimenticato: da vuoto = il
  // componente era una radice (aggiunge un padre senza staccarlo da un altro, TEC-13), a vuoto = diventa una radice. La
  // quantità come in «Collega», ma solo con un padre di arrivo: con a vuoto il gestore la legge e non la usa, e il ponte
  // non la chiede (SEM-03 della revisione di P3: allineata al legacy, [T])
  "/thread/{id}/fascicolo/componente/{u}/sposta": (p) => {
    const m = unico(p, "da", true) || unico(p, "a", true);
    if (m) return "«Sposta» manda sempre da e a, una volta sola, anche vuoti come scelta esplicita (" + m + ")";
    if (p.get("a").trim() === "") return "";
    const q = unico(p, "qta", false);
    return q ? "con un padre di arrivo la quantità va scritta, una volta sola: il ponte non ne mette una da sé (" + q + ")" : "";
  },
  // una sola voce, mai in catena (D2), e nessun campo della «Conferma Fascicolo» completa
  "/thread/{id}/fascicolo/conferma": (p) => {
    if (p.getAll("voce").length !== 1) return "«Conferma il documento» conferma esattamente una voce";
    const v = presente(p, ["struttura", "strutturale", "firma", "conferma", "selezione"]);
    return v ? "«Conferma il documento» non manda «" + v + "»" : "";
  },
  // lo scarto del file non segna il rumore dalla pagina nuova
  "/proposta/{u}/scarta": (p) => (p.has("rumore") ? "lo scarto del file dalla Distinta nuova non segna il rumore" : ""),
  // correggi_codice solo insieme a correzione_confermata, una volta sola e non vuoti: li manda soltanto «Usa come STEP
  // del prodotto…» (motore_a.mjs, apriUsaStep), dopo la finestra che elenca che cosa si scrive (giro 5, A1d, notte
  // 14/10, WP-U: decisione autonoma dell'orchestratore su Q-P3-03 [T, da ratificare]); il server ignora
  // correzione_confermata e decide come sempre (codiceDaComponente). Da soli, nessuno dei due; nuovo_riferimento* fuori
  // (DOM-20). codice_atteso e componente_atteso (revisione della notte 14/10) passano: li controlla il server contro la
  // proposta bloccata (attesaProposta, fascicolo.go)
  "/thread/{id}/fascicolo/assegna": (p) => {
    const confermata = p.has("correggi_codice") && !unico(p, "correggi_codice", false) && !unico(p, "correzione_confermata", false);
    const v = (confermata ? "" : presente(p, ["correggi_codice", "correzione_confermata"])) || conPrefisso(p, "nuovo_riferimento");
    return v ? "l'associazione dalla Distinta nuova non manda «" + v + "»" : "";
  },
  "/thread/{id}/fascicolo/documento/{u}/sostituisci": (p) => {
    const v = conPrefisso(p, "nuovo_riferimento");
    return v ? "«Sostituisci» dalla Distinta nuova non manda «" + v + "»: lo STEP si autorizza con la sua anteprima" : "";
  },
  // «Prepara i file» è solo il pulsante (DB7-6): mai il ramo automatico
  "/thread/{id}/fascicolo/prepara": (p) => (p.has("auto") ? "“Prepara i file” è solo il pulsante (DB7-6)" : ""),
};
// GUARDIE:fine

const U = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const PAGINA = new RegExp("^/thread/(" + U + ")/motore-a/?$");

// compila: una rotta della lista in una RegExp ancorata; {id} = un UUID catturato (la RFQ), {u} = un UUID qualunque, il
// resto alla lettera.
function compila(rotta) {
  let re = "^";
  for (const pezzo of rotta.split(/(\{id\}|\{u\})/)) {
    if (pezzo === "{id}") re += "(" + U + ")";
    else if (pezzo === "{u}") re += U;
    else re += pezzo.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }
  return new RegExp(re + "$");
}

const RE_ROTTE = ROTTE.map((r) => ({ rotta: r, re: compila(r) }));
const RE_LETTURE = LETTURE.map((r) => ({ rotta: r, re: compila(r) }));

// threadID: la RFQ della pagina, solo dal percorso /thread/<uuid>/motore-a (la / finale tollerata); "" altrove.
function threadID() {
  const m = PAGINA.exec(location.pathname);
  return m ? m[1] : "";
}

// percorsoPagina: il percorso della pagina, /thread/{id}/motore-a, senza la / finale.
function percorsoPagina() {
  return "/thread/" + threadID() + "/motore-a";
}

// pagina: l'indirizzo della pagina, per HX-Current-URL: /thread/{id}/motore-a della stessa origine, senza query.
function pagina() {
  return location.origin + percorsoPagina();
}

// ammessa: l'indirizzo di un gesto o di una lettura, se è nella lista, con la sua rotta; altrimenti il motivo. Le rotte
// si confrontano sul percorso già normalizzato da URL.
function ammessa(url, elenco) {
  let u;
  try {
    u = new URL(String(url), location.href);
  } catch (e) {
    return { motivo: "indirizzo non valido" };
  }
  if (u.origin !== location.origin) return { motivo: "rotta di un'altra origine" };
  const t = threadID();
  if (!t) return { motivo: "il ponte vale solo nella pagina /thread/{id}/motore-a" };
  for (const { rotta, re } of elenco) {
    const m = re.exec(u.pathname);
    if (!m) continue;
    if (m[1] !== undefined && m[1].toLowerCase() !== t.toLowerCase()) return { motivo: "rotta di un'altra RFQ" };
    return { url: u, rotta };
  }
  return { motivo: "rotta non prevista dalla lista chiusa" };
}

// valoreDi: il valore di un campo, solo una stringa o un numero finito. null, undefined, booleani, oggetti, NaN e
// Infinity non diventano testo ("null", "undefined", "[object Object]": valori che l'operatore non ha scritto):
// TypeError, e il gesto non parte (P3-PONTE-04).
function valoreDi(v) {
  if (typeof v === "string") return v;
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  throw new TypeError("valore di un campo non ammesso");
}

// aggiungi: i campi in URLSearchParams. Un oggetto (gli array diventano campi ripetuti, senza []), delle coppie
// [nome, valore], URLSearchParams o FormData (i file si saltano). I valori: solo stringhe e numeri finiti (valoreDi).
// Altro: TypeError, e il gesto non parte.
function aggiungi(p, campi) {
  if (campi === undefined || campi === null) return p;
  if (typeof campi !== "object") throw new TypeError("campi non validi");
  if (campi instanceof URLSearchParams || (typeof FormData !== "undefined" && campi instanceof FormData)) {
    for (const [k, v] of campi.entries()) if (typeof v === "string") p.append(k, v);
    return p;
  }
  if (Array.isArray(campi)) {
    for (const c of campi) {
      if (!Array.isArray(c) || c.length !== 2 || typeof c[0] !== "string") throw new TypeError("coppia di campi non valida");
      p.append(c[0], valoreDi(c[1]));
    }
    return p;
  }
  for (const [k, v] of Object.entries(campi)) {
    if (Array.isArray(v)) v.forEach((x) => p.append(k, valoreDi(x)));
    else p.append(k, valoreDi(v));
  }
  return p;
}

function intestazioni(extra) {
  return Object.assign({ "HX-Request": "true", "HX-Current-URL": pagina() }, extra || {});
}

function messaggio(err) {
  return (err && err.message) || String(err);
}

// percorsoDi: il percorso di un indirizzo della stessa origine; "" se non si legge o è di un'altra origine.
function percorsoDi(indirizzo) {
  try {
    const u = new URL(indirizzo, location.href);
    return u.origin === location.origin ? u.pathname : "";
  } catch (e) {
    return "";
  }
}

// tipoHTML: la risposta dichiara text/html.
function tipoHTML(risp) {
  return (risp.headers.get("Content-Type") || "").trim().toLowerCase().startsWith("text/html");
}

// rimandoAlLogin: la sessione scaduta. aLogin (server.go) risponde 401 con HX-Redirect: /login a una richiesta htmx;
// senza htmx un 302 che fetch segue fino a /login. Ritorna dove andare, oppure "". Un HX-Redirect che non porta a
// /login, con uno stato diverso da 401, non è la sessione scaduta: il ponte non lo segue.
function rimandoAlLogin(risp) {
  const red = risp.headers.get("HX-Redirect") || "";
  if (risp.status === 401) return red && percorsoDi(red) === "/login" ? red : "/login";
  if (red && percorsoDi(red) === "/login") return red;
  if (risp.redirected && percorsoDi(risp.url) === "/login") return "/login";
  return "";
}

// vaiAlLogin: solo un percorso della stessa origine; un indirizzo di fuori diventa /login.
function vaiAlLogin(dove) {
  let destinazione = "/login";
  try {
    const u = new URL(dove || "/login", location.href);
    if (u.origin === location.origin) destinazione = u.pathname + u.search;
  } catch (e) {
    // resta /login
  }
  location.assign(destinazione);
}

// sessioneScaduta: lo dice in #ma-avviso (resta finché la pagina del login non arriva) e va al login.
function sessioneScaduta(dove) {
  mostra("errore", SESSIONE);
  vaiAlLogin(dove);
  return { esito: "errore", testo: SESSIONE };
}

// testoPiano: il testo di una risposta d'errore, per #ma-avviso. Un corpo HTML si legge con DOMParser (solo per
// leggere); spazi compattati, al più 400 caratteri.
function testoPiano(corpo) {
  if (!corpo) return "";
  let t = String(corpo);
  if (/^\s*</.test(t) && typeof DOMParser !== "undefined") {
    t = new DOMParser().parseFromString(t, "text/html").body.textContent || "";
  }
  t = t.replace(/\s+/g, " ").trim();
  return t.length > 400 ? t.slice(0, 400) + "…" : t;
}

// leggiEsito: l'esito di un gesto, letto come TESTO. Il corpo deve essere un solo <div data-ma-esito> e nient'altro:
// niente testo o elementi fuori, niente nella testa del documento letto. Un esito vuoto: "ok" → FATTO, "no" → NIENTE.
// Un altro valore → "errore". Un corpo d'altra forma (la vecchia interfaccia, una pagina intera) → null.
function leggiEsito(corpo) {
  if (typeof DOMParser === "undefined") return null;
  const doc = new DOMParser().parseFromString(String(corpo || ""), "text/html");
  if (!doc.body || (doc.head && doc.head.childNodes.length > 0)) return null;
  const el = doc.body.firstElementChild;
  if (!el || doc.body.childElementCount !== 1 || el.tagName !== "DIV" || !el.hasAttribute("data-ma-esito")) return null;
  if (el.childElementCount !== 0) return null;
  for (const n of doc.body.childNodes) {
    if (n !== el && (n.nodeType !== 3 || n.textContent.trim() !== "")) return null;
  }
  const v = el.getAttribute("data-ma-esito");
  const testo = (el.textContent || "").replace(/\s+/g, " ").trim();
  if (v === "ok") return { esito: "ok", testo: testo || FATTO };
  if (v === "no") return { esito: "no", testo: testo || NIENTE };
  return { esito: "errore", testo: "Esito sconosciuto «" + v + "»: non si sa se il gesto è riuscito. " + testo };
}

// regioneAvviso: #ma-avviso, la regione dell'esito. Se il template non l'ha, il ponte ne crea una vuota in fondo al
// body (un elemento suo, mai HTML del server). È sempre una regione viva: role="status" e aria-live="polite" se il
// template non dice altro (P3-PONTE-06).
function regioneAvviso() {
  let el = document.getElementById("ma-avviso");
  if (!el && document.body) {
    el = document.createElement("div");
    el.id = "ma-avviso";
    document.body.append(el);
  }
  if (!el) return null;
  if (!el.hasAttribute("role")) el.setAttribute("role", "status");
  if (!el.hasAttribute("aria-live")) el.setAttribute("aria-live", "polite");
  return el;
}

// mostra: l'esito in #ma-avviso, solo testo. Svuotata prima: lo stesso testo due volte di seguito si annuncia di nuovo.
function mostra(esito, testo) {
  const el = regioneAvviso();
  if (!el) return;
  el.textContent = "";
  el.textContent = testo || "";
  el.setAttribute("data-esito", esito);
  el.removeAttribute("data-rilettura");
  el.hidden = false;
}

function finisci(esito, testo) {
  mostra(esito, testo);
  return { esito, testo };
}

// avvisaRilettura: la pagina non si è riletta. L'avviso di prima resta, con la frase in più una volta sola.
function avvisaRilettura() {
  const el = regioneAvviso();
  if (!el) return;
  const prima = (el.textContent || "").trim();
  if (!prima.endsWith(RILETTURA)) el.textContent = prima ? prima + " " + RILETTURA : RILETTURA;
  el.setAttribute("data-rilettura", "no");
  el.hidden = false;
}

// postHTTP: un gesto con campi di form. Mai il corpo nella pagina: o il login, o {esito, testo}.
async function postHTTP(percorso, p) {
  const risp = await fetch(percorso, {
    method: "POST",
    credentials: "same-origin",
    headers: intestazioni({ "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" }),
    body: p.toString(),
  });
  const login = rimandoAlLogin(risp);
  if (login) return { login };
  const corpo = await risp.text();
  if (!risp.ok) return { esito: "errore", testo: testoPiano(corpo) || "Errore " + risp.status };
  // l'esito si legge solo da un 200 text/html (rispondiMotoreA): un altro 2xx, o un corpo d'altro tipo, non dice
  // l'esito anche se contiene un <div data-ma-esito> (P3-PONTE-05)
  if (risp.status !== 200 || !tipoHTML(risp)) return { esito: "errore", testo: SENZA_ESITO };
  return leggiEsito(corpo) || { esito: "errore", testo: SENZA_ESITO };
}

// Lo stato della pagina: un gesto in volo (esegui, confermaAlbero) e la rilettura di #ma in volo o in coda (aggiorna).
let inVolo = false;
let rilettura = null; // la rilettura in volo
let prossima = null; // quella che parte dopo, condivisa

// occupato: un gesto in volo o una rilettura in corso. Un gesto nuovo parte solo a pagina libera: durante la rilettura
// la vista è ancora quella di prima, e un secondo gesto agirebbe su uno stato vecchio (P3-PONTE-03). Il gesto libera la
// pagina solo dopo la sua rilettura, che ha un limite di tempo (RILETTURA_MS).
function occupato() {
  return inVolo || rilettura !== null || prossima !== null;
}

// ---------------------------------------------------------------- l'API

export async function esegui(url, campi) {
  const a = ammessa(url, RE_ROTTE);
  if (!a.url) return finisci("errore", "Il gesto non è partito: " + a.motivo + ".");
  if (a.url.search !== "") return finisci("errore", "Il gesto non è partito: le rotte dei gesti non hanno parametri nell'indirizzo.");
  let p;
  try {
    p = aggiungi(new URLSearchParams(), campi);
  } catch (e) {
    return finisci("errore", "I campi del gesto non si leggono (solo stringhe e numeri): niente è partito.");
  }
  const guardia = GUARDIE[a.rotta];
  const motivo = guardia ? guardia(p) : "";
  if (motivo) return finisci("errore", "Il gesto non è partito: " + motivo + ".");
  // le rotte /proposta/ rispondono con il frammento del thread solo con ritorna_thread: sempre la RFQ della pagina
  if (a.url.pathname.startsWith("/proposta/")) p.set("ritorna_thread", threadID());
  if (occupato()) return finisci("errore", IN_VOLO);
  inVolo = true;
  try {
    let r;
    try {
      r = await postHTTP(a.url.pathname, p);
    } catch (err) {
      r = { esito: "errore", testo: "Il gesto non è arrivato al server, o la risposta non si legge: " + messaggio(err) };
    }
    if (r.login) return sessioneScaduta(r.login);
    mostra(r.esito, r.testo);
    if (r.esito === "ok") await aggiorna();
    return { esito: r.esito, testo: r.testo };
  } finally {
    inVolo = false;
  }
}

export async function leggi(url) {
  const a = ammessa(url, RE_LETTURE);
  if (!a.url) throw new Error("Lettura non partita: " + a.motivo + ".");
  const risp = await fetch(a.url.pathname + a.url.search, {
    credentials: "same-origin",
    headers: intestazioni({ Accept: "application/json" }),
  });
  const login = rimandoAlLogin(risp);
  if (login) {
    sessioneScaduta(login);
    throw new Error(SESSIONE);
  }
  const corpo = await risp.text();
  if (!risp.ok) throw new Error(testoPiano(corpo) || "Errore " + risp.status);
  if ((risp.headers.get("Content-Type") || "").toLowerCase().indexOf("json") < 0) throw new Error("La risposta non è JSON.");
  return JSON.parse(corpo);
}

// confermaAlbero: «Conferma l'albero» con la firma del riepilogo (GET /distinta/albero/riepilogo) e la bozza vuota della
// v1, {formato: 1, base: <albero_firma>}. Le due stringhe vengono dal riepilogo letto: il ponte non le calcola.
export async function confermaAlbero(firma, base) {
  const t = threadID();
  if (!t) return finisci("errore", "La conferma non è partita: il ponte vale solo nella pagina /thread/{id}/motore-a.");
  if (typeof firma !== "string" || firma === "" || typeof base !== "string" || base === "") {
    return finisci("errore", "La conferma non è partita: servono la firma e la base del riepilogo letto.");
  }
  if (occupato()) return finisci("errore", IN_VOLO);
  inVolo = true;
  try {
    let r;
    try {
      const risp = await fetch(ALBERO.replace("{id}", t), {
        method: "POST",
        credentials: "same-origin",
        headers: intestazioni({ "Content-Type": "application/json", Accept: "application/json" }),
        body: JSON.stringify({ firma, bozza: { formato: 1, base } }),
      });
      const login = rimandoAlLogin(risp);
      if (login) {
        r = { login };
      } else {
        const testo = await risp.text();
        const json = (risp.headers.get("Content-Type") || "").toLowerCase().indexOf("json") >= 0;
        let j = null;
        try {
          j = json ? JSON.parse(testo) : null;
        } catch (e) {
          j = null;
        }
        if (j === null || typeof j !== "object") j = null;
        if (risp.status === 200 && j) r = { esito: "ok", testo: String(j.testo || "") || FATTO };
        else if (risp.status === 422 && j) r = { esito: "no", testo: String(j.errore || "") || NIENTE };
        else r = { esito: "errore", testo: (j && String(j.errore || j.testo || "")) || testoPiano(testo) || "Errore " + risp.status };
      }
    } catch (err) {
      r = { esito: "errore", testo: "La conferma non è arrivata al server, o la risposta non si legge: " + messaggio(err) };
    }
    if (r.login) return sessioneScaduta(r.login);
    mostra(r.esito, r.testo);
    if (r.esito === "ok") await aggiorna();
    return { esito: r.esito, testo: r.testo };
  } finally {
    inVolo = false;
  }
}

// aggiorna: rilegge #ma con htmx (GET della pagina, che htmx manda con le sue intestazioni). Riuscita = la risposta
// passa le condizioni della rilettura (alPrimaDelloSwap) e il primo figlio di #ma è un nodo nuovo: htmx inserisce i
// nodi nuovi e toglie i vecchi; una 4xx o 5xx non sostituisce niente; un 401 con HX-Redirect lo segue htmx; allo
// scadere di RILETTURA_MS la richiesta si interrompe. Altrimenti l'avviso. È l'unico punto in cui la pagina cambia
// struttura, ed è la pagina del server riletta, mai la risposta di un gesto.
//
// Una rilettura alla volta: htmx, con la coda predefinita «last», mette in coda una seconda richiesta sullo stesso #ma
// mentre la prima è in volo e ne risolve subito la promessa; chi chiama mentre una rilettura è in volo aspetta che
// finisca e poi ne parte una sola, condivisa da tutti quelli che sono arrivati nel frattempo.
export function aggiorna() {
  if (!rilettura) {
    rilettura = rileggi().finally(() => {
      rilettura = null;
    });
    return rilettura;
  }
  if (!prossima) {
    prossima = rilettura.then(() => {
      prossima = null;
      return aggiorna();
    });
  }
  return prossima;
}

async function rileggi() {
  const ma = document.getElementById("ma");
  const hx = typeof window !== "undefined" ? window.htmx : undefined;
  if (!ma || !hx || typeof hx.ajax !== "function" || !threadID()) {
    avvisaRilettura();
    return false;
  }
  const prima = ma.firstElementChild;
  ma.setAttribute("aria-busy", "true");
  try {
    await hx.ajax("GET", pagina(), { target: "#ma", swap: "innerHTML" });
  } catch (e) {
    avvisaRilettura();
    return false;
  } finally {
    ma.removeAttribute("aria-busy");
  }
  const dopo = ma.firstElementChild;
  const riuscita = dopo !== null && dopo !== prima;
  if (!riuscita) avvisaRilettura();
  return riuscita;
}

// ---------------------------------------------------------------- le condizioni della rilettura (P3-PONTE-02)

// laRilettura: la richiesta htmx è la rilettura di #ma del ponte: una GET della pagina /thread/{id}/motore-a verso #ma.
// Guarda solo i dati dell'evento e la pagina, non lo stato del modulo.
function laRilettura(cfg, target) {
  return (
    !!cfg &&
    String(cfg.verb || "").toLowerCase() === "get" &&
    !!target &&
    target.id === "ma" &&
    threadID() !== "" &&
    percorsoDi(String(cfg.path || "")) === percorsoPagina()
  );
}

// alConfigRequest: il limite di tempo della rilettura (htmx lo legge dal campo timeout dell'evento htmx:configRequest).
function alConfigRequest(e) {
  const d = e.detail;
  if (d && laRilettura(d, d.target)) d.timeout = RILETTURA_MS;
}

// ATTRIBUTO_VIETATO: gli attributi htmx che nel frammento riletto chiamerebbero il server fuori dalla lista chiusa,
// sostituirebbero fuori banda o eseguirebbero codice.
const ATTRIBUTO_VIETATO = /^(data-)?hx-(get|post|put|patch|delete|swap-oob|on)/i;

// frammentoNonAmmesso: perché la risposta della rilettura non entra in #ma; "" se entra. Si legge con DOMParser, in un
// documento a parte che non esegue niente.
function frammentoNonAmmesso(corpo) {
  if (typeof DOMParser === "undefined") return "nessun DOMParser";
  const doc = new DOMParser().parseFromString(corpo, "text/html");
  const marcati = doc.querySelectorAll("[data-ma-pagina]");
  if (marcati.length !== 1 || (marcati[0].getAttribute("data-ma-pagina") || "").toLowerCase() !== threadID().toLowerCase()) {
    return "senza il marcatore data-ma-pagina di questa RFQ";
  }
  if (doc.getElementById("ma") || doc.getElementById("ma-avviso")) return "con #ma o #ma-avviso dentro";
  for (const el of doc.querySelectorAll("*")) {
    if (el.tagName === "SCRIPT") return "con uno script";
    for (const at of el.attributes) if (ATTRIBUTO_VIETATO.test(at.name)) return "con l'attributo " + at.name;
  }
  return "";
}

// alPrimaDelloSwap: la rilettura entra in #ma solo se la risposta è la pagina chiesta (lo stesso percorso: nessun
// rimando seguito altrove) e il frammento è ammesso (frammentoNonAmmesso). Altrimenti niente sostituzione e l'avviso.
// La guardia del layout (risposte non HTML scartate) resta prima di questa.
function alPrimaDelloSwap(e) {
  const d = e.detail;
  if (!d || !d.shouldSwap || !laRilettura(d.requestConfig, d.target)) return;
  const x = d.xhr;
  const altrove = !x || percorsoDi(String(x.responseURL || "")) !== percorsoPagina();
  if (altrove || frammentoNonAmmesso(String(d.serverResponse || "")) !== "") {
    d.shouldSwap = false;
    avvisaRilettura();
  }
}

// ---------------------------------------------------------------- l'intercettazione

async function lancia(el, fd, url) {
  if (!url || el.getAttribute("data-ma-in-corso") === "1") return; // doppio clic: il primo è ancora in corso
  if (occupato()) {
    mostra("errore", IN_VOLO);
    return;
  }
  const domanda = el.getAttribute("data-ma-domanda");
  if (domanda && !window.confirm(domanda)) return;
  const p = new URLSearchParams();
  if (fd) aggiungi(p, fd);
  const fissi = el.getAttribute("data-ma-campi");
  if (fissi) {
    try {
      aggiungi(p, JSON.parse(fissi));
    } catch (e) {
      mostra("errore", "I campi del gesto non si leggono (data-ma-campi: solo stringhe e numeri): niente è partito.");
      return;
    }
  }
  const bottoni = el.tagName === "FORM" ? [...el.querySelectorAll("button, input[type=submit]")] : [el];
  const spenti = bottoni.filter((b) => !b.disabled);
  el.setAttribute("data-ma-in-corso", "1");
  el.setAttribute("aria-busy", "true");
  spenti.forEach((b) => {
    b.disabled = true;
  });
  try {
    await esegui(url, p);
  } finally {
    el.removeAttribute("data-ma-in-corso");
    el.removeAttribute("aria-busy");
    spenti.forEach((b) => {
      if (b.isConnected) b.disabled = false;
    });
  }
}

function alClick(e) {
  const el = e.target && e.target.closest ? e.target.closest("[data-ma-gesto]") : null;
  if (!el || el.tagName === "FORM") return; // i form passano dal submit
  e.preventDefault();
  if (el.disabled || el.getAttribute("aria-disabled") === "true") return;
  lancia(el, null, el.getAttribute("data-ma-gesto"));
}

function alSubmit(e) {
  const f = e.target;
  if (!f || f.tagName !== "FORM" || !f.hasAttribute("data-ma-gesto")) return;
  e.preventDefault();
  let fd;
  try {
    fd = new FormData(f, e.submitter || undefined);
  } catch (err) {
    fd = new FormData(f);
  }
  lancia(f, fd, f.getAttribute("data-ma-gesto"));
}

// Gli ascoltatori una volta sola per finestra. Quelli della rilettura guardano solo l'evento e la pagina: valgono anche
// se il modulo fosse caricato due volte (e il frammento riletto non può portare uno <script> che lo ricarichi).
if (typeof window !== "undefined" && typeof document !== "undefined" && !window[INSTALLATO]) {
  window[INSTALLATO] = true;
  document.addEventListener("click", alClick);
  document.addEventListener("submit", alSubmit);
  document.addEventListener("htmx:configRequest", alConfigRequest);
  document.addEventListener("htmx:beforeSwap", alPrimaDelloSwap);
}
