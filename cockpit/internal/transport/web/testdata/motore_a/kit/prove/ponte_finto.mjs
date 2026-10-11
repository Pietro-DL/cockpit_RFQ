// L1 — giro 5, A1d: il DOPPIO DI PROVA del ponte dei gesti, usato solo da prove.cjs (QFE-03, K-05). NON è il ponte.
//
// Il ponte vero è cockpit/web/static/motore_a_gesti.mjs (BE-13; contratto A1d §3.2, §3.3; v1.1 §4.9): lo prova
// prove/ponte.cjs (QFE-04). Questo file è il vecchio ponte DEMO del kit con il solo modo «copione» (il modo HTTP è
// tolto) e non va MAI in cockpit/web/static (K-05): lì il server lo servirebbe come un modulo della pagina.
//
// Stessa API del ponte vero, perché motore_a.mjs non sappia con chi parla:
//   Gesti.esegui(url, campi)           → Promise<{esito: "ok"|"no"|"errore", testo}>; dopo "ok" rilegge #ma
//   Gesti.leggi(url)                   → l'oggetto JSON della lettura, o un'eccezione con il testo
//   Gesti.confermaAlbero(firma, bozza) → Promise<{esito, testo}>; dopo "ok" rilegge #ma lui (X-08)
//   Gesti.aggiorna()                   → Promise<boolean>: rilegge #ma senza gesto (true = riletta)
// e intercetta da solo click e submit degli elementi con data-ma-gesto (POST), con data-ma-campi (JSON di campi fissi;
// gli array diventano campi ripetuti) e data-ma-domanda (confirm() prima dell'invio), come il ponte vero. L'esito va in
// #ma-avviso con data-esito="ok|no|errore". Solo dopo "ok" la pagina si rilegge: htmx.ajax GET location.href in #ma
// (in jsdom lo fa la prova, con il frammento dello stesso caso).
//
// La differenza dal ponte vero è una sola: NIENTE RETE. Le risposte le prepara la prova in window.__copione e ogni
// chiamata si registra in window.__gesti:
//   window.__copione = {
//     rotte:   ["^/thread/(<uuid>)/fascicolo/conferma$", …],  // la lista chiusa del ponte vero (fra ROTTE:inizio e
//                                                             // ROTTE:fine), compilata dalla prova; senza: nessun controllo
//     letture_ammesse: ["^/thread/(<uuid>)/distinta/tipo$", …], // la lista LETTURE del ponte vero, compilata come sopra
//     gesti:   [{url: "/nodo/", esito: "no", testo: "Niente è cambiato: …"}, {re: "/conferma$", esito: "ok", testo: "…"}],
//     letture: [{url: "/distinta/albero/riepilogo", json: {…}}, {url: "/anteprima/", errore: "…"}, {url: "…", login: true}],
//     albero:  {esito: "ok", testo: "…"},          // la risposta di confermaAlbero
//     predefinito: {esito: "no", testo: "…"},     // un gesto senza voce: «no», mai un successo inventato
//   }
// Una voce può avere `risposte: [{…}, {…}]`: si usano in ordine e l'ultima si ripete. `login: true` simula la sessione
// scaduta (si registra in window.__login, nessuna navigazione). Le voci di window.__gesti:
//   {metodo, url, campi: [[nome, valore], …] | null, corpo, esito: "ok"|"no"|"errore"|"login"|"fuori"|"annullato"|
//    "rilettura", testo, da: "ponte"|"modulo" (solo per le riletture)}
//
// Vincoli, perché prove.cjs lo carichi togliendo gli `export` (README del kit): nessun `import`, nessun `await` al
// livello del modulo, solo le globali del DOM. Nessun innerHTML, outerHTML o insertAdjacentHTML: niente HTML entra
// nella pagina da qui.

const INSTALLATO = "__maGestiFinto";
const FATTO = "Fatto: il server non ha aggiunto altro."; // P-09, come il ponte vero
const NIENTE = "Niente è cambiato.";
const RILETTURA = "La pagina non si è riletta: ricaricala per vedere lo stato nuovo.";
const U = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";

function copione() {
  const c = typeof window !== "undefined" ? window.__copione : null;
  return c && typeof c === "object" ? c : {};
}

function registra(voce) {
  if (!Array.isArray(window.__gesti)) window.__gesti = [];
  window.__gesti.push(voce);
}

// il thread della pagina: solo dal percorso /thread/<uuid>/motore-a, come il ponte vero (T-A1d-32)
function threadID() {
  const m = location.pathname.match(new RegExp("^/thread/(" + U + ")/motore-a/?$"));
  return m ? m[1] : "";
}

// ammessa: la rotta è nella lista chiusa e parla della RFQ della pagina → {url}; altrimenti {motivo}
function ammessa(url, elenco) {
  let u;
  try { u = new URL(url, location.href); } catch (e) { return { motivo: "indirizzo non valido" }; }
  if (u.origin !== location.origin) return { motivo: "rotta di un'altra origine" };
  const t = threadID();
  if (!t) return { motivo: "il ponte vale solo nella pagina /thread/{id}/motore-a" };
  if (!Array.isArray(elenco)) return { url: u };
  for (const s of elenco) {
    const m = u.pathname.match(new RegExp(s));
    if (!m) continue;
    if (m[1] && m[1].toLowerCase() !== t.toLowerCase()) return { motivo: "rotta di un'altra RFQ" };
    return { url: u };
  }
  return { motivo: "rotta non prevista dal contratto" };
}

// aggiungi: un oggetto di campi in URLSearchParams (gli array diventano campi ripetuti; null e undefined si saltano)
function aggiungi(p, campi) {
  if (!campi) return p;
  if (campi instanceof URLSearchParams || (typeof FormData !== "undefined" && campi instanceof FormData)) {
    for (const [k, v] of campi.entries()) if (typeof v === "string") p.append(k, v);
    return p;
  }
  if (Array.isArray(campi)) {
    for (const [k, v] of campi) p.append(k, String(v));
    return p;
  }
  for (const [k, v] of Object.entries(campi)) {
    if (v === undefined || v === null) continue;
    if (Array.isArray(v)) v.forEach((x) => p.append(k, String(x)));
    else p.append(k, String(v));
  }
  return p;
}

function mostra(esito, testo) {
  const el = document.getElementById("ma-avviso");
  if (!el) return;
  el.textContent = testo || "";
  el.setAttribute("data-esito", esito);
  el.removeAttribute("data-rilettura");
  el.hidden = false;
}

function avvisaRilettura() {
  const el = document.getElementById("ma-avviso");
  if (!el) return;
  el.textContent = (el.textContent ? el.textContent + " " : "") + RILETTURA;
  el.setAttribute("data-rilettura", "no");
  el.hidden = false;
}

// scegli: la risposta di una voce del copione (con `risposte`, in ordine; l'ultima si ripete)
function scegli(v) {
  if (!v) return null;
  if (Array.isArray(v.risposte) && v.risposte.length) {
    v.__i = v.__i || 0;
    const r = v.risposte[Math.min(v.__i, v.risposte.length - 1)];
    v.__i++;
    return r;
  }
  return v;
}

function trova(elenco, url) {
  if (!Array.isArray(elenco)) return null;
  for (const v of elenco) {
    const ok = v.re ? new RegExp(v.re).test(url) : v.url ? url.indexOf(v.url) >= 0 : false;
    if (ok) return scegli(v);
  }
  return null;
}

function esitoDi(r) {
  const esito = r && (r.esito === "ok" || r.esito === "no") ? r.esito : "errore";
  let testo = (r && r.testo) || "";
  if (!testo && esito === "ok") testo = FATTO;
  if (!testo && esito === "no") testo = NIENTE;
  if (!testo) testo = "Il gesto non è riuscito (ponte finto).";
  return { esito, testo };
}

// rileggi: quello che fa aggiorna() del ponte vero; riuscita = il primo figlio di #ma è un nodo nuovo (T-A1d-31)
async function rileggi(da) {
  registra({ metodo: "GET", url: location.pathname + location.search, campi: null, corpo: null, esito: "rilettura", testo: "", da });
  const ma = document.getElementById("ma");
  if (!ma || typeof window.htmx === "undefined" || !window.htmx || typeof window.htmx.ajax !== "function") {
    avvisaRilettura();
    return false;
  }
  const prima = ma.firstElementChild;
  try {
    await window.htmx.ajax("GET", location.href, { target: "#ma", swap: "innerHTML" });
  } catch (e) {
    avvisaRilettura();
    return false;
  }
  const dopo = ma.firstElementChild;
  const riuscita = dopo !== null && dopo !== prima;
  if (!riuscita) avvisaRilettura();
  return riuscita;
}

// ---------------------------------------------------------------- l'API

export async function esegui(url, campi) {
  const p = aggiungi(new URLSearchParams(), campi);
  const a = ammessa(url, copione().rotte);
  if (!a.url) {
    registra({ metodo: "POST", url: String(url), campi: [...p.entries()], corpo: null, esito: "fuori", testo: a.motivo });
    const testo = "Il gesto non è partito: " + a.motivo + ".";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  const percorso = a.url.pathname;
  if (a.url.search) {
    registra({ metodo: "POST", url: percorso + a.url.search, campi: [...p.entries()], corpo: null, esito: "fuori", testo: "query" });
    const testo = "Il gesto non è partito: una rotta di scrittura non porta una query.";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  if (/\/fascicolo\/prepara$/.test(percorso) && p.has("auto")) {
    registra({ metodo: "POST", url: percorso, campi: [...p.entries()], corpo: null, esito: "fuori", testo: "auto" });
    const testo = "Il gesto non è partito: «Prepara i file» è solo il pulsante (DB7-6).";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  if (/^\/proposta\//.test(percorso)) p.set("ritorna_thread", threadID());
  const c = copione();
  const r = trova(c.gesti, percorso) || c.predefinito || { esito: "no", testo: "Niente è cambiato: (ponte finto) nessun esito preparato per questo gesto." };
  if (r.login) {
    registra({ metodo: "POST", url: percorso, campi: [...p.entries()], corpo: null, esito: "login", testo: "" });
    window.__login = "/login";
    const testo = "Sessione scaduta: si torna al login.";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  const e = esitoDi(r);
  registra({ metodo: "POST", url: percorso, campi: [...p.entries()], corpo: null, esito: e.esito, testo: e.testo });
  mostra(e.esito, e.testo);
  if (e.esito === "ok") await rileggi("ponte");
  return e;
}

export async function leggi(url) {
  const a = ammessa(url, copione().letture_ammesse);
  if (!a.url) {
    registra({ metodo: "GET", url: String(url), campi: null, corpo: null, esito: "fuori", testo: a.motivo });
    throw new Error("Lettura non partita: " + a.motivo + ".");
  }
  const dove = a.url.pathname + a.url.search;
  const r = trova(copione().letture, dove);
  if (!r) {
    registra({ metodo: "GET", url: dove, campi: null, corpo: null, esito: "errore", testo: "nessuna lettura preparata" });
    throw new Error("Nessuna lettura preparata per " + a.url.pathname + " (ponte finto).");
  }
  if (r.login) {
    registra({ metodo: "GET", url: dove, campi: null, corpo: null, esito: "login", testo: "" });
    window.__login = "/login";
    throw new Error("Sessione scaduta.");
  }
  if (r.errore) {
    registra({ metodo: "GET", url: dove, campi: null, corpo: null, esito: "errore", testo: r.errore });
    throw new Error(r.errore);
  }
  registra({ metodo: "GET", url: dove, campi: null, corpo: null, esito: "ok", testo: "" });
  // una copia nel mondo della pagina: chi legge può cambiarla senza toccare il copione
  return JSON.parse(JSON.stringify(r.json === undefined ? null : r.json));
}

export async function confermaAlbero(firma, bozza) {
  const t = threadID();
  if (!t) {
    const testo = "Il gesto non è partito: il ponte vale solo nella pagina /thread/{id}/motore-a.";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  const url = "/thread/" + t + "/distinta/albero/conferma";
  const corpo = JSON.parse(JSON.stringify({ firma, bozza }));
  const r = scegli(copione().albero) || { esito: "no", testo: "(ponte finto) nessun esito preparato per «Conferma l'albero»." };
  if (r.login) {
    registra({ metodo: "POST", url, campi: null, corpo, esito: "login", testo: "" });
    window.__login = "/login";
    const testo = "Sessione scaduta: si torna al login.";
    mostra("errore", testo);
    return { esito: "errore", testo };
  }
  const e = esitoDi(r);
  registra({ metodo: "POST", url, campi: null, corpo, esito: e.esito, testo: e.testo });
  mostra(e.esito, e.testo);
  // X-08: dopo «ok» rilegge il ponte; motore_a.mjs non chiama più Gesti.aggiorna() (la prova lo conta da `da`)
  if (e.esito === "ok") await rileggi("ponte");
  return e;
}

export function aggiorna() {
  return rileggi("modulo");
}

// ---------------------------------------------------------------- l'intercettazione (come il ponte vero)

async function lancia(el, fd, url) {
  if (!url || el.getAttribute("data-ma-in-corso") === "1") return;
  const domanda = el.getAttribute("data-ma-domanda");
  if (domanda && !window.confirm(domanda)) {
    registra({ metodo: "POST", url, campi: null, corpo: null, esito: "annullato", testo: "" });
    return;
  }
  const p = new URLSearchParams();
  if (fd) aggiungi(p, fd);
  const fissi = el.getAttribute("data-ma-campi");
  if (fissi) {
    try {
      aggiungi(p, JSON.parse(fissi));
    } catch (e) {
      mostra("errore", "I campi del gesto non si leggono (data-ma-campi): niente è partito.");
      return;
    }
  }
  const bottoni = el.tagName === "FORM" ? [...el.querySelectorAll("button, input[type=submit]")] : [el];
  const accesi = bottoni.filter((b) => !b.disabled);
  el.setAttribute("data-ma-in-corso", "1");
  el.setAttribute("aria-busy", "true");
  accesi.forEach((b) => { b.disabled = true; });
  try {
    await esegui(url, p);
  } finally {
    el.removeAttribute("data-ma-in-corso");
    el.removeAttribute("aria-busy");
    accesi.forEach((b) => { if (b.isConnected) b.disabled = false; });
  }
}

function alClick(e) {
  const el = e.target && e.target.closest ? e.target.closest("[data-ma-gesto]") : null;
  if (!el || el.tagName === "FORM") return; // i form passano dal submit
  e.preventDefault();
  if (el.disabled || el.closest("fieldset[disabled]") || el.getAttribute("aria-disabled") === "true") return;
  lancia(el, null, el.getAttribute("data-ma-gesto"));
}

function alSubmit(e) {
  const f = e.target;
  if (!f || f.tagName !== "FORM" || !f.hasAttribute("data-ma-gesto")) return;
  e.preventDefault();
  let fd;
  try { fd = new FormData(f, e.submitter || undefined); } catch (err) { fd = new FormData(f); }
  lancia(f, fd, f.getAttribute("data-ma-gesto"));
}

if (typeof window !== "undefined" && typeof document !== "undefined" && !window[INSTALLATO]) {
  window[INSTALLATO] = true;
  document.addEventListener("click", alClick);
  document.addEventListener("submit", alSubmit);
}
