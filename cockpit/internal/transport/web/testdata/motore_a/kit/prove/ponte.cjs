// L1 — il ponte dei gesti della Distinta nuova, vero, in una pagina jsdom con un fetch finto (QFE-04; giro 5, A1d;
// contratto A1d §3.1-§3.3; piano di A1d backend §5.7 e §7.3; piano del frontend §9.3; T-A1d-08, T-A1d-31, T-A1d-32;
// RAC-14, RAC-15, RAC-25). Si lancia a mano, fuori da go test: Node e jsdom non stanno nel repository.
//
// Uso, dalla cartella cockpit/:
//   NODE_PATH=<cartella con node_modules/jsdom> node internal/transport/web/testdata/motore_a/kit/prove/ponte.cjs
// Esce con 0 se ogni controllo passa, con 1 altrimenti (e scrive i controlli caduti). MA_COCKPIT=<cartella cockpit>
// prova il ponte di un'altra copia (le mutazioni MB01…MB08 si fanno così, mai nel repository).
//
// COME SI CARICA IL PONTE. jsdom non esegue <script type="module">. La pagina si apre con runScripts: "outside-only"
// (nessuno script della pagina parte da solo) e web/static/motore_a_gesti.mjs si valuta nella finestra con
// window.eval, togliendo le parole «export» e mettendo le quattro funzioni dell'API in window.__Gesti: il ponte non ha
// import né await fuori dalle funzioni (piano di A1d backend §5.7.3), quindi basta. L'indirizzo della pagina è
// /thread/00000000-0000-4000-8000-000000000001/motore-a su cockpit.acme.example (RAC-14): il ponte prende la RFQ solo
// da lì.
//
// LE SPIE. fetch risponde con le forme di risposte_acme.json (quelle del server, RAC-25); htmx.ajax registra la
// rilettura e, a seconda della prova, sostituisce il figlio di #ma come fa htmx (nodi nuovi, mai HTML in stringa),
// lascia tutto com'è (una 4xx non sostituisce) o rifiuta; confirm risponde come la prova chiede; i setter di
// innerHTML e outerHTML e insertAdjacentHTML contano le scritture di HTML; la navigazione verso /login, che jsdom non
// fa, si legge dal VirtualConsole («Not implemented: navigation»).
//
// I testi si confrontano come pezzi delle costanti del ponte (RAC-14): FATTO (P-09), RILETTURA, «Sessione scaduta»,
// «rotta non prevista dal contratto». Solo dati inventati: ACME, 712xxxx, UUID …0000000000nn, acme.example.
"use strict";

const fs = require("fs");
const path = require("path");
const { JSDOM, VirtualConsole } = require("jsdom");

// da kit/prove sette «..» portano a cockpit/ (RAC-03)
const COCKPIT = process.env.MA_COCKPIT ? path.resolve(process.env.MA_COCKPIT) : path.resolve(__dirname, "..", "..", "..", "..", "..", "..", "..");
const F_PONTE = path.join(COCKPIT, "web", "static", "motore_a_gesti.mjs");
const KIT = JSON.parse(fs.readFileSync(path.join(__dirname, "risposte_acme.json"), "utf8"));
const SORGENTE = fs.readFileSync(F_PONTE, "utf8");

const ID = KIT.thread; // la RFQ della pagina
const ALTRO = "00000000-0000-4000-8000-000000000002"; // un'altra RFQ
const AB = "00000000-0000-4000-8000-0000000000ab"; // una RFQ con delle lettere nell'UUID (RAC-15)
const N10 = "00000000-0000-4000-8000-000000000010"; // una riga di componente_proposta (un nodo)
const C11 = "00000000-0000-4000-8000-000000000011"; // un componente
const V20 = "00000000-0000-4000-8000-000000000020"; // una voce (documento_proposta)
const P30 = "00000000-0000-4000-8000-000000000030"; // una proposta di file
const PAGINA = KIT.pagina;
const ORIGINE = new URL(PAGINA).origin;

const P09 = "Fatto: il server non ha aggiunto altro.";
const RILETTA = "La pagina non si è riletta";
const SESSIONE = "Sessione scaduta";
const FUORI_LISTA = "rotta non prevista dal contratto";
const SENZA_ESITO = "non si sa se il gesto è riuscito";

// ------------------------------------------------------------------ il ponte, trasformato per jsdom

function codicePonte(src) {
  if (/^\s*import[\s{*"']/m.test(src) || /\bimport\s*\(/.test(src)) throw new Error("il ponte importa qualcosa: la trasformazione non basta");
  const c = src.replace(/^export\s+/gm, "");
  if (/^\s*export\b/m.test(c)) throw new Error("il ponte esporta in un modo che la trasformazione non toglie");
  return `(function(){\n${c}\nwindow.__Gesti = { esegui, leggi, confermaAlbero, aggiorna };\n})();`;
}
const PONTE = codicePonte(SORGENTE);

// ------------------------------------------------------------------ la pagina

function gestiHTML() {
  return `
<button type="button" id="b-scarta-nodo" data-ma-gesto="/thread/${ID}/fascicolo/nodo/${N10}/scarta"
        data-ma-domanda="Scartare il nodo 7120010? Lo scarto è definitivo.">Scarta il nodo</button>
<button type="button" id="b-prepara" data-ma-gesto="/thread/${ID}/fascicolo/prepara">Prepara i file</button>
<button type="button" id="b-conferma" data-ma-gesto="/thread/${ID}/fascicolo/conferma" data-ma-campi='{"voce":"${V20}"}'>Conferma il documento</button>
<button type="button" id="b-campi-rotti" data-ma-gesto="/thread/${ID}/fascicolo/conferma" data-ma-campi="{voce: ${V20}}">Conferma</button>
<button type="button" id="b-scarta-file" data-ma-gesto="/proposta/${P30}/scarta"
        data-ma-domanda="Scartare il file 7120110.pdf? Lo scarto è definitivo.">Scarta il file</button>
<button type="button" id="b-applica" data-ma-gesto="/thread/${ID}/fascicolo/bom/applica">Applica la bozza</button>
<button type="button" id="b-spento" data-ma-gesto="/thread/${ID}/riprova-copie" disabled>Riprova le copie</button>
<a href="#" id="a-riprova" data-ma-gesto="/thread/${ID}/riprova-copie">Riprova le copie sul NAS</a>
<form id="f-codice" data-ma-gesto="/thread/${ID}/fascicolo/nodo/${N10}/codice">
  <input name="codice" value="7120011"><input name="rev" value="B"><button>Correggi il codice</button>
</form>
<form id="f-collega" data-ma-gesto="/thread/${ID}/fascicolo/componente/${C11}/collega" data-ma-campi='{"padre":"${N10}"}'>
  <input name="qta" type="number" value="2"><button>Collega</button>
</form>`;
}

function paginaHTML() {
  return `<!doctype html><html lang="it"><head><meta charset="utf-8"><title>Distinta · Cockpit RFQ</title></head><body>
<div id="ma-avviso" role="status" hidden></div>
<div id="ma"><div class="ma" id="ma-prima" data-ma-thread="${ID}" data-ma-vista="distinta">${gestiHTML()}
</div></div>
</body></html>`;
}

const attendi = (ms = 15) => new Promise((r) => setTimeout(r, ms));

// risposta: una voce di risposte_acme.json come la vede il ponte (status, ok, redirected, url, headers.get, text).
function risposta(spec) {
  const h = {};
  for (const [k, v] of Object.entries(spec.intestazioni || {})) h[k.toLowerCase()] = v;
  const stato = spec.stato || 200;
  return {
    status: stato,
    ok: stato >= 200 && stato < 300,
    redirected: !!spec.rediretto,
    url: spec.url || "",
    headers: { get: (n) => (Object.prototype.hasOwnProperty.call(h, String(n).toLowerCase()) ? h[String(n).toLowerCase()] : null) },
    text: async () => spec.corpo || "",
  };
}

/**
 * apri({url, risposte, htmx, conferma}) → {w, d, G, spie}
 * risposte: un elenco (in ordine; l'ultima si ripete) o una funzione (voce) → spec | Promise<spec>; {rete: true} = la
 *   fetch rifiuta. htmx: "sostituisce" (predefinito), "resta", "rifiuta", "assente", "coda" (come htmx 2.0.4 con la
 *   coda predefinita «last»: la rilettura dura un poco; una seconda chiesta mentre la prima è in volo va in coda, la sua
 *   promessa si risolve subito e parte dopo la prima; R-05 della revisione di A1d-P2). conferma: che cosa dice confirm().
 */
async function apri({ url = PAGINA, risposte = [], htmx = "sostituisce", conferma = true } = {}) {
  const spie = { fetch: [], ajax: [], domande: [], navigazioni: 0, altri: [], innerHTML: 0, outerHTML: 0, insertAdjacentHTML: 0 };
  const vc = new VirtualConsole();
  vc.on("jsdomError", (e) => {
    const m = String((e && e.message) || e);
    if (/navigation/i.test(m)) spie.navigazioni++;
    else spie.altri.push(m);
  });
  const dom = new JSDOM(paginaHTML(), { url, runScripts: "outside-only", pretendToBeVisual: true, virtualConsole: vc });
  const w = dom.window;
  const P = w.Element.prototype;
  for (const nome of ["innerHTML", "outerHTML"]) {
    const d = Object.getOwnPropertyDescriptor(P, nome);
    Object.defineProperty(P, nome, {
      configurable: true,
      get() { return d.get.call(this); },
      set(v) { spie[nome]++; d.set.call(this, v); },
    });
  }
  const insAdj = P.insertAdjacentHTML;
  P.insertAdjacentHTML = function (...a) { spie.insertAdjacentHTML++; return insAdj.apply(this, a); };
  const coda = Array.isArray(risposte) ? risposte.slice() : null;
  w.fetch = (u, o = {}) => {
    const voce = {
      url: String(u),
      metodo: String(o.method || "GET").toUpperCase(),
      intestazioni: Object.assign({}, o.headers || {}),
      corpo: o.body === undefined ? null : String(o.body),
      credenziali: o.credentials,
    };
    spie.fetch.push(voce);
    const spec = coda ? (coda.length > 1 ? coda.shift() : coda[0]) : risposte(voce);
    return Promise.resolve(spec).then((s) => {
      if (!s) return Promise.reject(new w.TypeError("nessuna risposta preparata per " + voce.url));
      if (s.rete) return Promise.reject(new w.TypeError("Failed to fetch"));
      return risposta(s);
    });
  };
  if (htmx !== "assente") {
    const sostituisci = (n) => {
      const ma = w.document.getElementById("ma");
      const nuovo = w.document.createElement("div");
      nuovo.className = "ma";
      nuovo.id = "ma-riletta-" + n;
      nuovo.setAttribute("data-ma-thread", ID);
      while (ma.firstChild) ma.removeChild(ma.firstChild);
      ma.appendChild(nuovo);
    };
    let inVolo = false;
    let inCoda = null;
    const parti = (n) => {
      inVolo = true;
      return attendi(5).then(() => {
        sostituisci(n);
        inVolo = false;
        if (inCoda) {
          const c = inCoda;
          inCoda = null;
          c();
        }
      });
    };
    w.htmx = {
      ajax: (metodo, dove, opzioni) => {
        spie.ajax.push({ metodo, url: dove, opzioni });
        const n = spie.ajax.length;
        if (htmx === "rifiuta") return Promise.reject(new Error("la rilettura non è riuscita (prova)"));
        if (htmx === "sostituisce") sostituisci(n);
        if (htmx === "coda") {
          if (!inVolo) return parti(n);
          inCoda = () => parti(n); // «last»: prende il posto di quella già in coda
          return Promise.resolve();
        }
        return Promise.resolve();
      },
    };
  }
  w.confirm = (t) => {
    spie.domande.push(String(t));
    return conferma;
  };
  w.eval(PONTE);
  await attendi(1);
  return { w, d: w.document, G: w.__Gesti, spie };
}

const avviso = (p) => p.d.getElementById("ma-avviso");
const elementi = (p) => p.d.getElementsByTagName("*").length;

async function clic(p, el, ms = 15) {
  el.dispatchEvent(new p.w.MouseEvent("click", { bubbles: true, cancelable: true }));
  await attendi(ms);
}
async function invia(p, form, ms = 15) {
  form.dispatchEvent(new p.w.Event("submit", { bubbles: true, cancelable: true }));
  await attendi(ms);
}

// ------------------------------------------------------------------ prove

const risultati = [];
let caso = "";
function ok(cond, msg, extra) {
  risultati.push({ caso, msg, ok: !!cond });
  if (!cond) console.log(`  FALLITA [${caso}] ${msg}${extra !== undefined ? " · " + extra : ""}`);
}
const casi = [];
function prova(numero, titolo, fn) {
  casi.push({ numero, titolo, fn });
}

// --- 1…20: i casi del piano del frontend (§9.3); 21, 22 del backend (§7.3); 23 di RAC-15

prova(1, "200 con data-ma-esito=ok: ok, il testo, una rilettura", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok] });
  const prima = p.d.getElementById("ma").firstElementChild;
  const r = await p.G.esegui(`/thread/${ID}/fascicolo/nodo/${N10}/accetta`, { tipo: "finito" });
  ok(r.esito === "ok" && r.testo === "Proposta scartata.", "esito ok con il testo del server", JSON.stringify(r));
  const av = avviso(p);
  ok(av.textContent === "Proposta scartata." && av.getAttribute("data-esito") === "ok" && !av.hidden, "#ma-avviso: il testo e data-esito=ok");
  ok(p.spie.fetch.length === 1 && p.spie.fetch[0].metodo === "POST" && p.spie.fetch[0].url === `/thread/${ID}/fascicolo/nodo/${N10}/accetta`, "una POST alla rotta", JSON.stringify(p.spie.fetch));
  ok(p.spie.fetch[0] && p.spie.fetch[0].corpo === "tipo=finito", "il corpo è il form", p.spie.fetch[0] && p.spie.fetch[0].corpo);
  ok(p.spie.ajax.length === 1, "una sola rilettura", p.spie.ajax.length);
  const a = p.spie.ajax[0] || {};
  ok(a.metodo === "GET" && a.url === p.w.location.href && a.opzioni && a.opzioni.target === "#ma" && a.opzioni.swap === "innerHTML",
    "la rilettura è htmx.ajax(GET, location.href, {target: #ma, swap: innerHTML}) (contratto §3.3)", JSON.stringify(a));
  ok(p.d.getElementById("ma").firstElementChild !== prima && !av.hasAttribute("data-rilettura"), "#ma riletta, nessun avviso di rilettura");
});

prova(2, "200 con data-ma-esito=ok e nessun testo: il testo di P-09", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok_vuoto] });
  const r = await p.G.esegui(`/thread/${ID}/fascicolo/prepara`);
  ok(r.esito === "ok" && r.testo.includes(P09), "ok con «" + P09 + "»", JSON.stringify(r));
  ok(avviso(p).textContent.includes(P09) && avviso(p).getAttribute("data-esito") === "ok", "#ma-avviso con P-09");
  ok(p.spie.ajax.length === 1, "una rilettura");
});

prova(3, "200 con data-ma-esito=no: no, il testo, nessuna rilettura", async () => {
  for (const nome of ["no", "no_gia_decisa"]) {
    const v = KIT.gesti[nome];
    const p = await apri({ risposte: [v] });
    const prima = p.d.getElementById("ma").firstElementChild;
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/nodo/${N10}/scarta`);
    ok(r.esito === "no" && r.testo === v.avviso, `${nome}: esito no con il testo`, JSON.stringify(r));
    ok(avviso(p).getAttribute("data-esito") === "no" && avviso(p).textContent === v.avviso, `${nome}: #ma-avviso con data-esito=no`);
    ok(p.spie.ajax.length === 0 && p.d.getElementById("ma").firstElementChild === prima, `${nome}: nessuna rilettura`);
  }
  // un «no» senza testo
  const p = await apri({ risposte: [{ stato: 200, intestazioni: KIT.gesti.no.intestazioni, corpo: '<div data-ma-esito="no"></div>' }] });
  const r = await p.G.esegui(`/thread/${ID}/riprova-copie`);
  ok(r.esito === "no" && r.testo === "Niente è cambiato.", "un no vuoto dice che niente è cambiato", JSON.stringify(r));
});

prova(4, "le 9 frasi di rifiuto dell'elenco chiuso, marcate no dal server: no; il ponte non classifica", async () => {
  ok(KIT.rifiuti.length === 9, "nel kit le 9 frasi di T-A1d-11", KIT.rifiuti.length);
  for (const v of KIT.rifiuti) {
    const p = await apri({ risposte: [v] });
    const r = await p.G.esegui(`/thread/${ID}/riprova-copie`);
    ok(r.esito === "no" && r.testo === v.avviso, "no: " + v.avviso, JSON.stringify(r));
    ok(p.spie.ajax.length === 0 && avviso(p).getAttribute("data-esito") === "no", "nessuna rilettura: " + v.avviso);
    // la stessa frase marcata ok: il ponte riporta l'esito del server, non legge il testo (la classificazione è di
    // rispondiMotoreA, T-A1d-11)
    const q = await apri({ risposte: [Object.assign({}, v, { corpo: v.corpo.replace('data-ma-esito="no"', 'data-ma-esito="ok"') })] });
    const s = await q.G.esegui(`/thread/${ID}/riprova-copie`);
    ok(s.esito === "ok" && q.spie.ajax.length === 1, "il ponte non classifica: " + v.avviso, JSON.stringify(s));
  }
});

prova(5, "2xx senza un solo data-ma-esito: errore, mai un successo, nessuna rilettura", async () => {
  for (const nome of ["senza_esito", "vuoto_200", "senza_corpo_204", "due_div", "testo_fuori", "esito_in_uno_span", "pagina_intera"]) {
    const p = await apri({ risposte: [KIT.gesti[nome]] });
    const n = elementi(p);
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/rianalizza`);
    ok(r.esito === "errore" && r.testo.includes(SENZA_ESITO), `${nome}: errore «${SENZA_ESITO}»`, JSON.stringify(r));
    ok(avviso(p).getAttribute("data-esito") === "errore", `${nome}: data-esito=errore`);
    ok(p.spie.ajax.length === 0, `${nome}: nessuna rilettura`);
    ok(elementi(p) === n, `${nome}: niente entra nella pagina`);
  }
});

prova(6, "200 con l'HTML legacy di thread_corpo: errore, il corpo non entra mai nella pagina", async () => {
  const p = await apri({ risposte: [KIT.gesti.legacy_thread_corpo] });
  const n = elementi(p);
  const r = await p.G.esegui(`/proposta/${P30}/scarta`);
  await attendi(20);
  ok(r.esito === "errore" && r.testo.includes(SENZA_ESITO), "errore: l'esito annidato nella vecchia interfaccia non vale", JSON.stringify(r));
  ok(elementi(p) === n, "il numero degli elementi della pagina non cambia", `${n} → ${elementi(p)}`);
  ok(!p.d.querySelector(".thread-corpo, #thread-corpo-acme, table.documenti, form[action]"), "nessun pezzo della vecchia interfaccia nella pagina");
  ok(p.d.querySelectorAll("[data-ma-esito]").length === 0, "nessun data-ma-esito nella pagina");
  ok(p.spie.innerHTML === 0 && p.spie.outerHTML === 0 && p.spie.insertAdjacentHTML === 0, "nessuna scrittura di HTML (innerHTML, outerHTML, insertAdjacentHTML)",
    JSON.stringify({ i: p.spie.innerHTML, o: p.spie.outerHTML, a: p.spie.insertAdjacentHTML }));
  ok(p.w.__iniettato === undefined, "niente della risposta è stato eseguito");
  ok(!/[<>]/.test(avviso(p).textContent) && avviso(p).children.length === 0, "#ma-avviso ha solo testo");
  ok(p.spie.ajax.length === 0, "nessuna rilettura");
});

prova(7, "200 con data-ma-esito=forse: errore", async () => {
  const p = await apri({ risposte: [KIT.gesti.forse] });
  const r = await p.G.esegui(`/thread/${ID}/smistamento/aggiorna`);
  ok(r.esito === "errore" && r.testo.includes("Esito sconosciuto") && r.testo.includes(SENZA_ESITO), "errore con l'esito sconosciuto", JSON.stringify(r));
  ok(p.spie.ajax.length === 0, "nessuna rilettura");
});

prova(8, "401 con HX-Redirect (e senza): errore, sessione scaduta, si va al login, nessuna rilettura", async () => {
  for (const nome of ["login_401", "login_401_nudo"]) {
    const p = await apri({ risposte: [KIT.gesti[nome]] });
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/rimozione/accetta`, { step: V20, padre: C11, figlio: N10 });
    ok(r.esito === "errore" && r.testo.includes(SESSIONE), `${nome}: errore «${SESSIONE}»`, JSON.stringify(r));
    ok(p.spie.navigazioni >= 1, `${nome}: la pagina prova ad andare al login`, p.spie.navigazioni);
    ok(p.spie.ajax.length === 0, `${nome}: nessuna rilettura`);
  }
});

prova(9, "302 seguito fino a /login: come il 401", async () => {
  const p = await apri({ risposte: [KIT.gesti.login_302] });
  const n = elementi(p);
  const r = await p.G.esegui(`/thread/${ID}/fascicolo/relazione/scarta`, { allegato: P30, padre: "#1", figlio: "#2" });
  ok(r.esito === "errore" && r.testo.includes(SESSIONE), "errore «" + SESSIONE + "»", JSON.stringify(r));
  ok(p.spie.navigazioni >= 1, "la pagina prova ad andare al login", p.spie.navigazioni);
  ok(p.spie.ajax.length === 0 && elementi(p) === n, "nessuna rilettura, la pagina del login non entra");
});

prova(10, "403, 422, 500, 400, 404 e la rete giù: errore con il testo, nessuna rilettura", async () => {
  const attesi = {
    vietato_403: "Non autorizzato. Il ruolo «consultazione» vede il Cockpit e non lo cambia.",
    rifiuto_422: "Niente è cambiato: la firma del riepilogo non coincide",
    errore_500: "errore del database: la connessione si è chiusa",
    errore_400: "id non valido",
    errore_404: "proposta non trovata",
  };
  for (const [nome, testo] of Object.entries(attesi)) {
    const p = await apri({ risposte: [KIT.gesti[nome]] });
    const n = elementi(p);
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/assegna`, { proposta: P30, componente: C11 });
    ok(r.esito === "errore" && r.testo.includes(testo), `${nome}: errore con il testo`, JSON.stringify(r));
    ok(avviso(p).getAttribute("data-esito") === "errore" && avviso(p).textContent === r.testo, `${nome}: #ma-avviso con data-esito=errore`);
    ok(p.spie.ajax.length === 0 && elementi(p) === n, `${nome}: nessuna rilettura, niente entra nella pagina`);
  }
  const p = await apri({ risposte: [{ rete: true }] });
  const r = await p.G.esegui(`/thread/${ID}/fascicolo/assegna`, { proposta: P30, componente: C11 });
  ok(r.esito === "errore" && r.testo.startsWith("Il gesto non è partito"), "rete giù: errore «Il gesto non è partito»", JSON.stringify(r));
  ok(p.spie.ajax.length === 0, "rete giù: nessuna rilettura");
});

prova(11, "rotte fuori dalla lista, di un'altra origine o di un'altra RFQ: errore, nessuna fetch", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok] });
  const fuori = [
    `/thread/${ID}/fascicolo/bom/applica`,
    `/thread/${ID}/fascicolo/congela`,
    `/thread/${ID}/fascicolo/revisione/apri`,
    `/thread/${ID}/fascicolo/revisione/abbandona`,
    `/proposta/${P30}/conferma`,
    `/proposta/${P30}/decidi`,
    `/thread/${ID}/fascicolo/file/${P30}/accetta`,
    `/thread/${ID}/fascicolo/carica`,
    `/thread/${ID}/fascicolo/componente/${C11}/step-strutturale/riattiva`,
    `/thread/${ID}/distinta/albero/conferma`,
    `/thread/${ID}/fascicolo/nodo/${N10}/../../bom/applica`,
    `/thread/${ID}/fascicolo%2Fprepara`,
    `/thread/${ID}/fascicolo/nodo/7120010/accetta`,
    `/thread/${ID}/motore-a/dati`,
  ];
  for (const u of fuori) {
    const r = await p.G.esegui(u);
    ok(r.esito === "errore" && r.testo.includes(FUORI_LISTA), "fuori lista: " + u, JSON.stringify(r));
  }
  const motivi = [
    [`http://altro.acme.example/thread/${ID}/fascicolo/prepara`, "rotta di un'altra origine"],
    [`https://cockpit.acme.example/thread/${ID}/fascicolo/prepara`, "rotta di un'altra origine"],
    [`/thread/${ALTRO}/fascicolo/prepara`, "rotta di un'altra RFQ"],
    [`/thread/${ALTRO}/fascicolo/nodo/${N10}/accetta`, "rotta di un'altra RFQ"],
    ["http://[::1", "indirizzo non valido"],
    [`/thread/${ID}/smistamento/aggiorna?ritorna=1`, "non hanno parametri"],
  ];
  for (const [u, motivo] of motivi) {
    const r = await p.G.esegui(u);
    ok(r.esito === "errore" && r.testo.includes(motivo), `${motivo}: ${u}`, JSON.stringify(r));
  }
  ok(p.spie.fetch.length === 0, "nessuna fetch", JSON.stringify(p.spie.fetch));
  await clic(p, p.d.getElementById("b-applica"));
  ok(p.spie.fetch.length === 0 && avviso(p).getAttribute("data-esito") === "errore" && avviso(p).textContent.includes(FUORI_LISTA),
    "il pulsante con una rotta fuori lista non fa partire niente e lo dice");
  ok(p.spie.ajax.length === 0, "nessuna rilettura");
});

prova(12, "doppio clic mentre la prima richiesta è aperta: una sola fetch", async () => {
  let sblocca;
  const attesa = new Promise((r) => { sblocca = r; });
  // htmx «resta»: i pulsanti stanno in #ma, e una rilettura riuscita li toglierebbe dalla pagina
  const p = await apri({ risposte: () => attesa.then(() => KIT.gesti.ok), htmx: "resta" });
  const b = p.d.getElementById("b-scarta-nodo");
  await clic(p, b, 5);
  await clic(p, b, 5);
  ok(p.spie.fetch.length === 1, "un pulsante: una fetch", p.spie.fetch.length);
  ok(b.disabled && b.getAttribute("data-ma-in-corso") === "1", "il pulsante è spento e in corso");
  const f = p.d.getElementById("f-codice");
  await invia(p, f, 5);
  await invia(p, f, 5);
  ok(p.spie.fetch.length === 2, "un form: una fetch", p.spie.fetch.length);
  const a = p.d.getElementById("a-riprova");
  await clic(p, a, 5);
  await clic(p, a, 5);
  ok(p.spie.fetch.length === 3, "un collegamento: una fetch", p.spie.fetch.length);
  sblocca();
  await attendi(30);
  ok(!b.disabled && !b.hasAttribute("data-ma-in-corso"), "finita la richiesta il pulsante si riaccende");
  await clic(p, b);
  ok(p.spie.fetch.length === 4, "un clic dopo la fine parte", p.spie.fetch.length);
  await clic(p, p.d.getElementById("b-spento"));
  ok(p.spie.fetch.length === 4, "un pulsante spento non parte");
});

prova(13, "/proposta/{u}/scarta: ritorna_thread è sempre la RFQ della pagina", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok], htmx: "resta" });
  await p.G.esegui(`/proposta/${P30}/scarta`, { rumore: "1" });
  await p.G.esegui(`/proposta/${P30}/scarta`, { ritorna_thread: ALTRO });
  await clic(p, p.d.getElementById("b-scarta-file"));
  await p.G.esegui(`/thread/${ID}/fascicolo/conferma`, { voce: V20 });
  const q = p.spie.fetch.map((f) => new p.w.URLSearchParams(f.corpo || ""));
  ok(q.length === 4, "quattro fetch", q.length);
  ok(q[0] && q[0].getAll("ritorna_thread").join() === ID && q[0].get("rumore") === "1", "esegui: ritorna_thread e i campi", p.spie.fetch[0] && p.spie.fetch[0].corpo);
  ok(q[1] && q[1].getAll("ritorna_thread").join() === ID, "un ritorna_thread di un'altra RFQ si sostituisce", p.spie.fetch[1] && p.spie.fetch[1].corpo);
  ok(q[2] && q[2].getAll("ritorna_thread").join() === ID, "il pulsante: ritorna_thread", p.spie.fetch[2] && p.spie.fetch[2].corpo);
  ok(q[3] && !q[3].has("ritorna_thread") && q[3].get("voce") === V20, "le rotte del thread non lo hanno", p.spie.fetch[3] && p.spie.fetch[3].corpo);
});

prova(14, "ogni richiesta (esegui, leggi, confermaAlbero) porta HX-Request e HX-Current-URL", async () => {
  const p = await apri({
    risposte: (v) => (v.url.includes("/distinta/albero/conferma") ? KIT.albero.ok : v.metodo === "GET" ? KIT.letture.dati : KIT.gesti.ok),
  });
  await p.G.esegui(`/thread/${ID}/fascicolo/componente/${C11}/modifica`, { rev: "B", descrizione: "Gruppo ACME" });
  await p.G.leggi(`/thread/${ID}/motore-a/dati`);
  await p.G.confermaAlbero("f-acme", { formato: 1, base: "b-acme" });
  ok(p.spie.fetch.length === 3, "tre richieste", p.spie.fetch.length);
  for (const f of p.spie.fetch) {
    ok(f.intestazioni["HX-Request"] === "true" && f.intestazioni["HX-Current-URL"] === p.w.location.href, `${f.metodo} ${f.url}: HX-Request e HX-Current-URL`, JSON.stringify(f.intestazioni));
    ok(f.credenziali === "same-origin", `${f.url}: credentials same-origin`);
    ok(f.url.startsWith("/"), `${f.url}: indirizzo della stessa origine, senza host`);
  }
  ok((p.spie.fetch[0] || {}).intestazioni["Content-Type"] === "application/x-www-form-urlencoded;charset=UTF-8", "esegui: campi di form");
  ok((p.spie.fetch[1] || {}).metodo === "GET" && (p.spie.fetch[1] || { intestazioni: {} }).intestazioni.Accept === "application/json", "leggi: GET JSON");
  ok((p.spie.fetch[2] || { intestazioni: {} }).intestazioni["Content-Type"] === "application/json", "confermaAlbero: JSON");
});

prova(15, "data-ma-domanda: confirm falso non fa partire niente, vero sì", async () => {
  const no = await apri({ risposte: [KIT.gesti.ok], conferma: false });
  await clic(no, no.d.getElementById("b-scarta-nodo"));
  ok(no.spie.fetch.length === 0, "confirm falso: nessuna fetch", no.spie.fetch.length);
  ok(no.spie.domande[0] === "Scartare il nodo 7120010? Lo scarto è definitivo.", "la domanda è quella del markup", no.spie.domande[0]);
  ok(no.d.getElementById("b-scarta-nodo").disabled === false, "il pulsante resta acceso");
  const si = await apri({ risposte: [KIT.gesti.ok], conferma: true });
  await clic(si, si.d.getElementById("b-scarta-nodo"));
  ok(si.spie.fetch.length === 1 && si.spie.fetch[0].url === `/thread/${ID}/fascicolo/nodo/${N10}/scarta`, "confirm vero: una fetch", JSON.stringify(si.spie.fetch));
  const senza = await apri({ risposte: [KIT.gesti.ok] });
  await clic(senza, senza.d.getElementById("b-prepara"));
  ok(senza.spie.domande.length === 0 && senza.spie.fetch.length === 1, "senza data-ma-domanda: nessuna domanda");
});

prova(16, "data-ma-campi che non è JSON: errore, nessuna fetch; i campi fissi si aggiungono al form", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok], htmx: "resta" });
  await clic(p, p.d.getElementById("b-campi-rotti"));
  ok(p.spie.fetch.length === 0, "nessuna fetch", p.spie.fetch.length);
  ok(avviso(p).getAttribute("data-esito") === "errore" && avviso(p).textContent.includes("data-ma-campi"), "#ma-avviso dice che i campi non si leggono");
  await clic(p, p.d.getElementById("b-conferma"));
  ok(p.spie.fetch.length === 1 && p.spie.fetch[0].corpo === "voce=" + V20, "una sola voce dai campi fissi", p.spie.fetch[0] && p.spie.fetch[0].corpo);
  await invia(p, p.d.getElementById("f-collega"));
  const q = new p.w.URLSearchParams((p.spie.fetch[1] || {}).corpo || "");
  ok(q.get("qta") === "2" && q.get("padre") === N10, "form più data-ma-campi", p.spie.fetch[1] && p.spie.fetch[1].corpo);
  const r = await p.G.esegui(`/thread/${ID}/fascicolo/conferma`, "voce=" + V20);
  ok(r.esito === "errore" && p.spie.fetch.length === 2, "campi che non sono un oggetto: errore, nessuna fetch", JSON.stringify(r));
  await p.G.esegui(`/thread/${ID}/fascicolo/componente/${C11}/step-strutturale`, { documento: V20, firma: "f", autorizza: "1", delega: ["a", "b"] });
  const s = new p.w.URLSearchParams((p.spie.fetch[2] || {}).corpo || "");
  ok(s.getAll("delega").join() === "a,b" && s.get("autorizza") === "1", "gli array diventano campi ripetuti", p.spie.fetch[2] && p.spie.fetch[2].corpo);
});

prova(17, "leggi: JSON, non JSON, 401, 500, 404 e una lettura fuori lista", async () => {
  const p = await apri({ risposte: [KIT.letture.dati] });
  const j = await p.G.leggi(`/thread/${ID}/motore-a/dati`);
  ok(j && j.thread_id === ID && j.testa && j.testa.cliente === "ACME S.p.A.", "200 JSON: l'oggetto", JSON.stringify(j));
  const t = await apri({ risposte: [KIT.letture.tipo] });
  await t.G.leggi(`/thread/${ID}/distinta/tipo?componente=${C11}&tipo=sottoassieme`);
  ok(t.spie.fetch[0] && t.spie.fetch[0].url === `/thread/${ID}/distinta/tipo?componente=${C11}&tipo=sottoassieme` && t.spie.fetch[0].metodo === "GET", "la query resta", t.spie.fetch[0] && t.spie.fetch[0].url);
  const casi = [
    ["non_json", (e) => e.message === "La risposta non è JSON."],
    ["errore_500", (e) => e.message === "dati non disponibili"],
    ["assente_404", (e) => e.message === "RFQ non trovata"],
  ];
  for (const [nome, giusto] of casi) {
    const q = await apri({ risposte: [KIT.letture[nome]] });
    let errore = null;
    try { await q.G.leggi(`/thread/${ID}/distinta/albero/riepilogo`); } catch (e) { errore = e; }
    ok(errore && giusto(errore), `${nome}: un'eccezione con il testo`, errore && errore.message);
    ok(q.spie.ajax.length === 0 && avviso(q).hidden, `${nome}: nessuna rilettura, #ma-avviso non toccato`);
  }
  const l = await apri({ risposte: [KIT.gesti.login_401] });
  let e401 = null;
  try { await l.G.leggi(`/thread/${ID}/motore-a/anteprima/step-strutturale?componente=${C11}&documento=${V20}`); } catch (e) { e401 = e; }
  ok(e401 && e401.message.includes(SESSIONE) && l.spie.navigazioni >= 1, "401: eccezione e login", e401 && e401.message);
  const f = await apri({ risposte: [KIT.letture.dati] });
  let efuori = null;
  try { await f.G.leggi(`/thread/${ID}/fascicolo`); } catch (e) { efuori = e; }
  ok(efuori && efuori.message.includes(FUORI_LISTA) && f.spie.fetch.length === 0, "fuori da LETTURE: eccezione, nessuna fetch", efuori && efuori.message);
  let ealtra = null;
  try { await f.G.leggi(`/thread/${ALTRO}/motore-a/dati`); } catch (e) { ealtra = e; }
  ok(ealtra && ealtra.message.includes("rotta di un'altra RFQ") && f.spie.fetch.length === 0, "un'altra RFQ: eccezione, nessuna fetch", ealtra && ealtra.message);
});

prova(18, "confermaAlbero: 200 ok e la rilettura del ponte; 422 no; 413, 500, 200 non JSON errore; 401", async () => {
  const p = await apri({ risposte: [KIT.albero.ok] });
  const r = await p.G.confermaAlbero("f-acme", { formato: 1, base: "b-acme" });
  ok(r.esito === "ok" && r.testo === "Albero confermato: 3 componenti e 2 legami nella BOM verificata.", "200: ok con il testo", JSON.stringify(r));
  ok(p.spie.ajax.length === 1, "200: il ponte rilegge #ma (X-08)", p.spie.ajax.length);
  const f = p.spie.fetch[0] || {};
  let corpo = null;
  try { corpo = JSON.parse(f.corpo); } catch (e) { corpo = null; }
  ok(f.metodo === "POST" && f.url === `/thread/${ID}/distinta/albero/conferma` && corpo && corpo.firma === "f-acme" && corpo.bozza.formato === 1 && corpo.bozza.base === "b-acme",
    "la POST JSON {firma, bozza} alla RFQ della pagina", JSON.stringify(f));
  const v = await apri({ risposte: [KIT.albero.ok_vuoto] });
  const rv = await v.G.confermaAlbero("f", { formato: 1, base: "b" });
  ok(rv.esito === "ok" && rv.testo.includes(P09) && v.spie.ajax.length === 1, "200 senza testo: P-09 e la rilettura", JSON.stringify(rv));
  const n = await apri({ risposte: [KIT.albero.rifiuto_422] });
  const rn = await n.G.confermaAlbero("f", { formato: 1, base: "b" });
  ok(rn.esito === "no" && rn.testo === "il riepilogo è cambiato mentre lo guardavi: rileggilo" && n.spie.ajax.length === 0, "422: no, nessuna rilettura", JSON.stringify(rn));
  ok(avviso(n).getAttribute("data-esito") === "no", "422: #ma-avviso con data-esito=no");
  for (const [nome, testo] of [["troppo_413", "la bozza mandata è troppo grande"], ["errore_500", "l'albero non si legge"], ["non_json_200", "Albero confermato."]]) {
    const q = await apri({ risposte: [KIT.albero[nome]] });
    const rq = await q.G.confermaAlbero("f", { formato: 1, base: "b" });
    ok(rq.esito === "errore" && rq.testo.includes(testo) && q.spie.ajax.length === 0, `${nome}: errore, nessuna rilettura`, JSON.stringify(rq));
  }
  const l = await apri({ risposte: [KIT.gesti.login_401] });
  const rl = await l.G.confermaAlbero("f", { formato: 1, base: "b" });
  ok(rl.esito === "errore" && rl.testo.includes(SESSIONE) && l.spie.navigazioni >= 1 && l.spie.ajax.length === 0, "401: login", JSON.stringify(rl));
});

prova(19, "la rilettura che non riesce: #ma-avviso lo dice", async () => {
  for (const modo of ["rifiuta", "resta", "assente"]) {
    const p = await apri({ risposte: [KIT.gesti.ok], htmx: modo });
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/nodo/${N10}/riapri`);
    const av = avviso(p);
    ok(r.esito === "ok", `${modo}: il gesto resta ok`, JSON.stringify(r));
    ok(av.textContent.startsWith("Proposta scartata.") && av.textContent.includes(RILETTA) && av.getAttribute("data-rilettura") === "no" && av.getAttribute("data-esito") === "ok",
      `${modo}: l'avviso dice che la pagina non si è riletta`, av.textContent);
    const ancora = await p.G.aggiorna();
    ok(ancora === false && av.textContent.split(RILETTA).length === 2, `${modo}: aggiorna() dà false e la frase c'è una volta`, av.textContent);
  }
  const p = await apri({ risposte: [KIT.gesti.ok] });
  ok((await p.G.aggiorna()) === true && !avviso(p).hasAttribute("data-rilettura"), "una rilettura riuscita: true, nessun avviso");
  await p.G.esegui(`/thread/${ID}/fascicolo/nodo/${N10}/riapri`);
  ok(!avviso(p).hasAttribute("data-rilettura"), "dopo un gesto nuovo l'avviso di rilettura sparisce");
});

prova(20, "il sorgente del ponte: niente che inserisca HTML, nessun modo demo", async () => {
  const swap = 'hx.ajax("GET", location.href, { target: "#ma", swap: "innerHTML" })';
  ok(SORGENTE.split(swap).length === 2, "la sola eccezione: lo stile della rilettura di htmx, una volta (contratto §3.3)");
  const resto = SORGENTE.replace(swap, "").toLowerCase();
  const vietate = ["innerhtml", "outerhtml", "insertadjacenthtml", "insertadjacentelement", "createcontextualfragment", "document.write", "eval(", "new function",
    "appendchild", "insertbefore", "replacechild", "replacewith", "importnode", "adoptnode", ".prepend(", ".before(", ".after(",
    "__esitidemo", "__gesti", "__login", "copione", "location.reload", "srcdoc", "http://", "https://", "analisi" + "_messaggio"];
  for (const v of vietate) ok(!resto.includes(v), "nel ponte non c'è «" + v + "»");
  const esportate = [...SORGENTE.matchAll(/^export\s+(?:async\s+)?function\s+(\w+)/gm)].map((m) => m[1]).sort();
  ok(JSON.stringify(esportate) === JSON.stringify(["aggiorna", "confermaAlbero", "esegui", "leggi"]), "esporta solo l'API del contratto", esportate.join());
  ok((SORGENTE.match(/^export\b/gm) || []).length === 4, "nessun altro export");
  ok(!/^\s*import[\s{*"']/m.test(SORGENTE) && !/\bimport\s*\(/.test(SORGENTE), "nessun import");
  ok(SORGENTE.includes('const INSTALLATO = "__maGesti";'), "la guardia è window.__maGesti");
  ok(SORGENTE.includes('"Fatto: il server non ha aggiunto altro."'), "il testo di P-09 è nel ponte");
});

prova(21, "fuori dalla pagina /thread/{id}/motore-a nessun gesto parte", async () => {
  for (const u of [`${ORIGINE}/thread/${ID}/distinta`, `${ORIGINE}/thread/${ID}/motore-a/dati`, `${ORIGINE}/thread/${ID}/fascicolo`, `${ORIGINE}/inbox`]) {
    const p = await apri({ url: u, risposte: [KIT.gesti.ok] });
    const r = await p.G.esegui(`/thread/${ID}/fascicolo/prepara`);
    ok(r.esito === "errore" && r.testo.includes("il ponte vale solo nella pagina"), `${u}: esegui non parte`, JSON.stringify(r));
    let e = null;
    try { await p.G.leggi(`/thread/${ID}/motore-a/dati`); } catch (x) { e = x; }
    ok(e && e.message.includes("il ponte vale solo nella pagina"), `${u}: leggi non parte`, e && e.message);
    const ra = await p.G.confermaAlbero("f", { formato: 1, base: "b" });
    ok(ra.esito === "errore", `${u}: confermaAlbero non parte`, JSON.stringify(ra));
    await clic(p, p.d.getElementById("b-prepara"));
    ok(p.spie.fetch.length === 0 && p.spie.ajax.length === 0, `${u}: nessuna fetch, nessuna rilettura`, p.spie.fetch.length);
  }
  const barra = await apri({ url: PAGINA + "/?vista=documenti", risposte: [KIT.gesti.ok] });
  const rb = await barra.G.esegui(`/thread/${ID}/fascicolo/prepara`);
  ok(rb.esito === "ok" && barra.spie.fetch.length === 1, "la / finale e la query della pagina vanno bene", JSON.stringify(rb));
});

prova(22, "«Prepara i file» con auto: errore, nessuna fetch; senza auto parte", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok] });
  const r1 = await p.G.esegui(`/thread/${ID}/fascicolo/prepara`, { auto: "1" });
  ok(r1.esito === "errore" && r1.testo.includes("Prepara i file") && r1.testo.includes("DB7-6"), "auto nei campi: errore", JSON.stringify(r1));
  const r2 = await p.G.esegui(`/thread/${ID}/fascicolo/prepara?auto=1`);
  ok(r2.esito === "errore", "auto nell'indirizzo: errore", JSON.stringify(r2));
  const fd = new p.w.URLSearchParams("auto=");
  const r3 = await p.G.esegui(`/thread/${ID}/fascicolo/prepara`, fd);
  ok(r3.esito === "errore", "auto vuoto: errore lo stesso", JSON.stringify(r3));
  ok(p.spie.fetch.length === 0, "nessuna fetch", p.spie.fetch.length);
  const r4 = await p.G.esegui(`/thread/${ID}/fascicolo/prepara`);
  ok(r4.esito === "ok" && p.spie.fetch.length === 1 && p.spie.fetch[0].corpo === "", "senza auto: una POST senza campi", JSON.stringify(p.spie.fetch));
});

prova(23, "RAC-15: la pagina con l'UUID in maiuscolo è la stessa RFQ", async () => {
  const su = AB.toUpperCase();
  ok(su !== AB, "l'UUID della prova ha delle lettere");
  const p = await apri({ url: `${ORIGINE}/thread/${su}/motore-a`, risposte: [KIT.gesti.ok] });
  const r = await p.G.esegui(`/thread/${AB}/fascicolo/rianalizza`);
  ok(r.esito === "ok" && p.spie.fetch.length === 1, "una rotta con l'UUID in minuscolo parte", JSON.stringify(r));
  ok((p.spie.fetch[0] || { intestazioni: {} }).intestazioni["HX-Current-URL"] === p.w.location.href, "HX-Current-URL è l'indirizzo della pagina com'è");
  const r2 = await p.G.esegui(`/thread/${ALTRO.toUpperCase()}/fascicolo/rianalizza`);
  ok(r2.esito === "errore" && r2.testo.includes("rotta di un'altra RFQ") && p.spie.fetch.length === 1, "un'altra RFQ resta fuori", JSON.stringify(r2));
  await p.G.esegui(`/proposta/${P30}/scarta`);
  const q = new p.w.URLSearchParams((p.spie.fetch[1] || {}).corpo || "");
  ok((q.get("ritorna_thread") || "").toLowerCase() === AB, "ritorna_thread è la RFQ della pagina", p.spie.fetch[1] && p.spie.fetch[1].corpo);
  const mi = await apri({ url: `${ORIGINE}/thread/${AB}/motore-a`, risposte: [KIT.gesti.ok] });
  const r3 = await mi.G.esegui(`/thread/${su}/fascicolo/rianalizza`);
  ok(r3.esito === "ok" && mi.spie.fetch.length === 1, "e al contrario: la rotta in maiuscolo nella pagina in minuscolo", JSON.stringify(r3));
});

prova(24, "R-05: due gesti finiti vicini, una rilettura alla volta: nessun falso «non si è riletta»", async () => {
  const p = await apri({ risposte: [KIT.gesti.ok], htmx: "coda" });
  const r = await Promise.all([p.G.esegui(`/thread/${ID}/fascicolo/prepara`), p.G.esegui(`/thread/${ID}/fascicolo/rianalizza`)]);
  await attendi(30);
  ok(r[0].esito === "ok" && r[1].esito === "ok" && p.spie.fetch.length === 2, "due gesti, tutti e due ok", JSON.stringify(r));
  ok(!(avviso(p).textContent || "").includes(RILETTA), "nessun avviso di rilettura", avviso(p).textContent);
  ok(p.spie.ajax.length === 2, "due riletture, la seconda dopo la prima", p.spie.ajax.length);
  const tre = await Promise.all([p.G.aggiorna(), p.G.aggiorna(), p.G.aggiorna()]);
  ok(tre.every((x) => x === true), "tre aggiorna insieme: tutte riuscite", JSON.stringify(tre));
  ok(p.spie.ajax.length === 4, "una rilettura e poi una sola, condivisa dalle due arrivate nel frattempo", p.spie.ajax.length);
  ok(!(avviso(p).textContent || "").includes(RILETTA), "ancora nessun avviso di rilettura", avviso(p).textContent);
  ok(p.spie.innerHTML + p.spie.outerHTML + p.spie.insertAdjacentHTML === 0, "nessuna scrittura di HTML");
});

// ------------------------------------------------------------------ esecuzione

(async () => {
  for (const c of casi) {
    caso = `${c.numero} ${c.titolo}`;
    try {
      await c.fn();
    } catch (e) {
      ok(false, "eccezione nella prova", (e && e.stack) || String(e));
    }
  }
  const caduti = risultati.filter((r) => !r.ok);
  const casiCaduti = new Set(caduti.map((r) => r.caso));
  console.log(`ponte.cjs: ${casi.length} casi, ${risultati.length} controlli, ${caduti.length} falliti (${casiCaduti.size} casi)`);
  for (const c of casiCaduti) console.log("  caso caduto: " + c);
  process.exit(caduti.length === 0 ? 0 : 1);
})();
