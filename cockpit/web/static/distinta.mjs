// La Distinta (cockpit/_fasi/PROPOSTA_DISTINTA.md): l'albero proposto del passo 2, le miniature dei disegni e il
// visore con le note. Il resto della pagina e' HTML del server, che i gesti rifanno (#distinta).
//
// Il passo 2 (giro 4, fase 4.4a.3; domande 27, 28, 29, 30 e 10) lavora sull'albero proposto dal server (GET
// .../distinta/albero): tutti i livelli degli STEP del prodotto, con lo stato di ogni pezzo. Luigi lo corregge — rinomina
// un codice, toglie un pezzo (con la cascata: un figlio in comune resta), ne aggiunge uno sotto il padre che sceglie,
// cambia un tipo, risponde ✓ o ✗ alla minuteria proposta — e ogni correzione va in una BOZZA: il JSON che il riepilogo e
// la conferma leggono (fascicolo.BozzaAlbero, formato 1). La bozza non va al server finche' non la si rivede: resta in
// questo browser (localStorage, per RFQ e per utente, con la firma dell'albero da cui e' partita), e al ricaricamento
// la pagina chiede se riprenderla. «Rivedi e conferma» mostra il riepilogo (una POST che legge soltanto); «Conferma
// l'albero», spento finche' c'e' una domanda aperta, e' il solo gesto che scrive. Gli stessi comandi stanno nel menu
// del tasto destro sui pezzi, e da tastiera (il tasto menu o Maiusc+F10), con il fuoco che torna dov'era.

const PDFJS = "/static/pdfjs-6.3.289/";

// ------------------------------------------------------------------ piccoli attrezzi

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
function el(tag, attr, ...figli) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attr || {})) {
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k.startsWith("on") && typeof v === "function") e.addEventListener(k.slice(2), v);
    else if (k === "html") e.innerHTML = v;
    else e.setAttribute(k, v === true ? "" : v);
  }
  for (const f of figli.flat(Infinity)) if (f != null && f !== false) e.append(f.nodeType ? f : document.createTextNode(String(f)));
  return e;
}
function prova(fn, altrimenti) { try { return fn(); } catch (e) { return altrimenti; } }
const cssId = (s) => (window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, "\\$&"));

let tToast = 0;
function avvisa(testo, no) {
  const t = document.getElementById("dst-toast");
  if (!t || !testo) return;
  t.textContent = testo;
  t.classList.toggle("no", !!no);
  t.hidden = false;
  clearTimeout(tToast);
  tToast = setTimeout(() => { t.hidden = true; }, no ? 7000 : 4200);
}

const TIPI = { finito: "Prodotto", sottoassieme: "Assieme", sciolto: "Particolare", commerciale: "Particolare commerciale" };
const TIPI_FRASE = { finito: "un prodotto", sottoassieme: "un assieme", sciolto: "un particolare", commerciale: "un particolare commerciale" };
const FONTI = { step: "dallo STEP", operatore: "scritto sulla riga dello STEP da una persona", componente: "il componente della distinta", richiesta: "il codice della richiesta" };
const conta = (n, uno, tanti) => n + " " + (n === 1 ? uno : tanti);

// lo stato della pagina: il JSON del server (#dst-dati) e l'albero; fuocoDopo: il pezzo su cui torna il fuoco quando
// l'albero si rilegge dopo un gesto che ha rifatto la pagina
const S = { dati: null, editor: null, fuocoDopo: null };

function leggiDati() {
  const x = document.getElementById("dst-dati");
  S.dati = x ? prova(() => JSON.parse(x.textContent), null) : null;
}
async function rileggiDati() {
  if (!S.dati) return;
  try {
    const r = await fetch(S.dati.pagina + "/dati", { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (r.ok) S.dati = await r.json();
  } catch (e) { /* resta quello di prima */ }
}

// ------------------------------------------------------------------ il posto e il fuoco dopo un gesto

// Un gesto rifa' il corpo (#distinta). Prima si ricorda la riga del file (o il comando) su cui si e' agito, dove
// stava nella finestra e quale bottone aveva il fuoco; dopo, la riga torna allo stesso punto della finestra e il fuoco
// sul bottone equivalente, o sulla riga se il bottone non c'e' piu' (un file confermato non ha piu' «✓ Conferma").
// Senza, il browser ancorava lo scorrimento al corpo vecchio e la pagina saltava di migliaia di pixel, con il fuoco
// sul BODY (bug 4 del 29/09).
let postoSalvato = null;
function ricordaPosto(elt) {
  const att = document.activeElement;
  const da = (elt && elt.closest && elt.closest("[data-riga]")) || (att && att.closest && att.closest("[data-riga]"));
  const fuoco = (att && att.dataset && att.dataset.fuoco) || (elt && elt.dataset && elt.dataset.fuoco) ||
    (elt && elt.querySelector && elt.querySelector("[data-fuoco]") ? elt.querySelector("[data-fuoco]").dataset.fuoco : "");
  return { riga: da ? da.dataset.riga : "", fuoco, alto: da ? da.getBoundingClientRect().top : null, y: window.scrollY };
}
function rimettiPosto(p) {
  if (!p) return;
  const riga = p.riga ? document.querySelector('[data-riga="' + cssId(p.riga) + '"]') : null;
  if (riga && p.alto != null) window.scrollBy(0, riga.getBoundingClientRect().top - p.alto);
  else window.scrollTo(0, p.y);
  const x = riga && p.fuoco ? riga.querySelector('[data-fuoco="' + cssId(p.fuoco) + '"]') : null;
  const bersaglio = x || riga;
  if (bersaglio) bersaglio.focus({ preventScroll: true });
}

// ------------------------------------------------------------------ i gesti

// gesto manda una rotta di sempre come la manderebbe htmx da questa pagina: la risposta e' il corpo nuovo con
// l'avviso (threadFrammento → rispondiDistinta). Con swap il corpo si rifa' solo se il gesto e' riuscito: uno
// rifiutato lascia com'e' quello che l'operatore stava facendo.
async function gesto(url, valori, opz = {}) {
  const corpo = new URLSearchParams();
  for (const [k, v] of Object.entries(valori || {})) {
    if (Array.isArray(v)) v.forEach((x) => corpo.append(k, x));
    else if (v != null) corpo.append(k, v);
  }
  let r;
  try {
    r = await fetch(url, {
      method: "POST", credentials: "same-origin", body: corpo,
      headers: { "HX-Request": "true", "HX-Current-URL": location.href, "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8" },
    });
  } catch (e) {
    return { ok: false, testo: "Il server non ha risposto: controlla la rete e riprova." };
  }
  if (r.status === 403) return { ok: false, testo: "Non autorizzato: chi consulta non cambia la distinta." };
  if (!r.ok) return { ok: false, testo: "Il server ha risposto " + r.status + ": riprova." };
  const html = await r.text();
  let ok = true;
  let testo = "";
  const trig = r.headers.get("HX-Trigger");
  if (trig) {
    const j = prova(() => JSON.parse(trig), null);
    if (j && j["bom-esito"]) { ok = !!j["bom-esito"].ok; testo = j["bom-esito"].testo || ""; }
  }
  const doc = new DOMParser().parseFromString(html, "text/html");
  const av = doc.querySelector(".dst-avviso");
  if (av) {
    if (!testo) testo = av.textContent.trim();
    if (av.dataset.esito === "no") ok = false;
  }
  if (ok && opz.swap !== false) sostituisciCorpo(html);
  return { ok, testo };
}

function sostituisciCorpo(html) {
  const c = document.getElementById("distinta");
  if (!c) return;
  const p = ricordaPosto(document.activeElement);
  c.innerHTML = html;
  if (window.htmx) window.htmx.process(c);
  avvia();
  rimettiPosto(p);
}

// ricaricaCorpo rilegge il corpo della pagina (lo stesso passo) come lo rifarebbe un gesto: dopo «Conferma l'albero»
// le linguette, l'albero e i documenti sono quelli di adesso.
async function ricaricaCorpo() {
  try {
    const r = await fetch(location.pathname + location.search, { credentials: "same-origin", headers: { "HX-Request": "true" } });
    if (r.ok) sostituisciCorpo(await r.text());
  } catch (e) { /* resta la pagina di prima: la si ricarica a mano */ }
}

// ------------------------------------------------------------------ pdf.js, caricato quando serve

let pdfjsPromessa = null;
function pdfjs() {
  if (!pdfjsPromessa) {
    pdfjsPromessa = import(PDFJS + "pdf.min.mjs")
      .then((lib) => { lib.GlobalWorkerOptions.workerSrc = PDFJS + "pdf.worker.min.mjs"; return lib; })
      .catch(() => null);
  }
  return pdfjsPromessa;
}
function opzioniPdf(url) {
  return {
    url, isEvalSupported: false, enableXfa: false, verbosity: 0,
    wasmUrl: PDFJS + "wasm/", iccUrl: PDFJS + "iccs/", cMapUrl: PDFJS + "cmaps/", cMapPacked: true,
    standardFontDataUrl: PDFJS + "standard_fonts/",
  };
}
// l'indirizzo del PDF di un allegato. Con l'impronta del contenuto (v, lo sha256 che la pagina riceve) il server puo'
// dire al browser di tenerlo senza richiederlo alla prossima apertura (cache C1); senza, si rivalida
function urlPdf(a) {
  const p = nomePdf(a);
  const v = p && p.v;
  return "/allegato/" + a + "/anteprima" + (v ? "?v=" + encodeURIComponent(v) : "");
}
function motivoPdf(err) {
  const s = err && (err.status || (err.details && err.details.status));
  switch (s) {
    case 404: return "Il file non è su questo server: va riscaricato (Documenti e NAS › Riscarica).";
    case 409: return "Il file sul NAS non corrisponde al documento: va visto in Integrità NAS.";
    case 415: return "Non è un PDF.";
    case 503: return "Il NAS non risponde: si riprova fra poco.";
    case 401: case 403: return "La sessione è scaduta: ricarica la pagina.";
  }
  if (err && err.name === "PasswordException") return "Il PDF è protetto da una password.";
  return "Il disegno non si è potuto aprire.";
}

// ------------------------------------------------------------------ le miniature

// la prima pagina di ogni disegno, una volta sola per file (cache per allegato) e un file per volta
const miniCache = new Map(); // allegato → dataURL, oppure "" = non disponibile
let osservatore = null;
const codaMini = [];
let lavoro = false;

function miniature() {
  if (osservatore) osservatore.disconnect();
  const tutte = $$(".dst-mini[data-a]");
  const daFare = [];
  for (const m of tutte) {
    if (miniCache.has(m.dataset.a)) mettiMini(m, miniCache.get(m.dataset.a));
    else daFare.push(m);
  }
  if (!daFare.length) return;
  if (!("IntersectionObserver" in window)) { codaMini.push(...daFare); lavoraMini(); return; }
  osservatore = new IntersectionObserver((voci) => {
    for (const v of voci) if (v.isIntersecting) { osservatore.unobserve(v.target); codaMini.push(v.target); }
    lavoraMini();
  }, { rootMargin: "200px" });
  for (const m of daFare) osservatore.observe(m);
}
function mettiMini(m, url) {
  const att = $(".attesa", m);
  if (att) att.remove();
  if (!url) {
    if (!$(".attesa", m) && !$("canvas, img", m)) m.prepend(el("span", { class: "attesa" }, "anteprima non disponibile"));
    return;
  }
  if ($("img", m)) return;
  m.prepend(el("img", { src: url, alt: "", style: "display:block;width:100%;height:100%;object-fit:contain" }));
}
async function lavoraMini() {
  if (lavoro) return;
  lavoro = true;
  while (codaMini.length) {
    const m = codaMini.shift();
    if (!m.isConnected) continue;
    const a = m.dataset.a;
    if (!miniCache.has(a)) miniCache.set(a, await disegnaMini(a));
    for (const x of $$(`.dst-mini[data-a="${a}"]`)) mettiMini(x, miniCache.get(a));
  }
  lavoro = false;
}
async function disegnaMini(a) {
  const lib = await pdfjs();
  if (!lib) return "";
  const task = lib.getDocument(opzioniPdf(urlPdf(a)));
  try {
    const doc = await task.promise;
    const pag = await doc.getPage(1);
    const v1 = pag.getViewport({ scale: 1 });
    const scala = 520 / Math.max(v1.width, v1.height);
    const vp = pag.getViewport({ scale: scala });
    const c = document.createElement("canvas");
    c.width = Math.ceil(vp.width);
    c.height = Math.ceil(vp.height);
    const ctx = c.getContext("2d");
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, c.width, c.height);
    await pag.render({ canvasContext: ctx, viewport: vp, background: "#ffffff" }).promise;
    return c.toDataURL("image/png");
  } catch (e) {
    return "";
  } finally {
    task.destroy().catch(() => {}); // chiude il documento e il suo lavoratore
  }
}

// ------------------------------------------------------------------ il visore dei disegni con le note

const V = { file: null, comp: "", cod: "", doc: null, task: null, pagina: 1, pagine: 1, zoom: 0, armato: false, sel: null, nuovo: null, modifica: null, conferma: null, da: null, gen: 0 };
const ZOOM = [1, 1.25, 1.5, 2, 3, 4];

function pdfDelComponente(comp) { return ((S.dati && S.dati.pdf && S.dati.pdf[comp]) || []); }
// i PDF che dicono un codice e non sono ancora di un componente: il disegno di un pezzo proposto (con la scritta
// «proposto», A5.3.10)
function pdfDelCodice(codice) {
  const c = (codice || "").toUpperCase();
  if (!c) return [];
  return ((S.dati && S.dati.tutti) || []).filter((p) => !p.comp && (p.cod || "").toUpperCase() === c);
}
function nomePdf(a) {
  for (const p of (S.dati && S.dati.tutti) || []) if (p.a === a) return p;
  return null;
}
function noteDi(a) { return ((S.dati && S.dati.note && S.dati.note[a]) || []); }
function noteDelComponente(comp) {
  const visti = new Set();
  let n = 0;
  for (const p of pdfDelComponente(comp)) if (!visti.has(p.a)) { visti.add(p.a); n += noteDi(p.a).length; }
  return n;
}

async function apriVisore(a, comp, da, cod) {
  const vis = document.getElementById("dst-visore");
  if (!vis || !a) return;
  const p = nomePdf(a);
  if (p && !p.ok) { avvisa(p.nome + ": il file non è su questo server. Si riscarica da Documenti e NAS.", true); return; }
  V.file = a; V.comp = comp || (p && p.comp) || ""; V.cod = cod || ""; V.pagina = 1; V.zoom = 0; V.armato = false; V.sel = null; V.nuovo = null; V.modifica = null; V.conferma = null;
  V.da = da || document.activeElement;
  // l'elenco dei disegni: prima quelli del pezzo, poi gli altri della richiesta
  const sel = document.getElementById("dst-vis-file");
  sel.replaceChildren();
  const suoi = V.comp ? pdfDelComponente(V.comp) : pdfDelCodice(V.cod);
  const altri = ((S.dati && S.dati.tutti) || []).filter((x) => !suoi.some((y) => y.a === x.a));
  const gruppo = (titolo, lista) => {
    if (!lista.length) return;
    const g = el("optgroup", { label: titolo });
    for (const x of lista) g.append(el("option", { value: x.a, selected: x.a === a }, x.nome + (x.ok ? "" : " (non disponibile)") + (x.stato === "doc" ? "" : " · proposto")));
    sel.append(g);
  };
  gruppo("Disegni del pezzo", suoi);
  gruppo("Altri PDF della richiesta", altri);
  vis.hidden = false;
  document.body.style.overflow = "hidden";
  await caricaPdf();
  document.getElementById("dst-vis-chiudi").focus();
}

function chiudiVisore() {
  const vis = document.getElementById("dst-visore");
  if (!vis || vis.hidden) return;
  vis.hidden = true;
  document.body.style.overflow = "";
  chiudiDocumento();
  if (S.editor) S.editor.disegna();
  if (V.da && V.da.isConnected) V.da.focus();
  else if (S.editor) S.editor.fuocoSu(S.editor.sel);
}

function chiudiDocumento() {
  if (V.task) V.task.destroy().catch(() => {});
  V.task = null;
  V.doc = null;
}

async function caricaPdf() {
  const carta = document.getElementById("dst-carta");
  const gen = ++V.gen;
  chiudiDocumento();
  carta.replaceChildren(el("p", { class: "messaggio" }, "Apertura del disegno…"));
  barraVisore();
  const lib = await pdfjs();
  if (gen !== V.gen) return;
  if (!lib) { carta.replaceChildren(el("p", { class: "messaggio" }, "Il visore non si è caricato: ricarica la pagina.")); return; }
  const task = lib.getDocument(opzioniPdf(urlPdf(V.file)));
  V.task = task;
  try {
    const doc = await task.promise;
    if (gen !== V.gen) { task.destroy().catch(() => {}); return; }
    V.doc = doc;
    V.pagine = doc.numPages;
    await disegnaPagina();
  } catch (e) {
    if (gen === V.gen) carta.replaceChildren(el("p", { class: "messaggio" }, motivoPdf(e)));
  }
}

async function disegnaPagina() {
  if (!V.doc) return;
  const gen = V.gen;
  const scena = document.getElementById("dst-scena");
  const carta = document.getElementById("dst-carta");
  const pag = await V.doc.getPage(V.pagina);
  if (gen !== V.gen) return;
  const v1 = pag.getViewport({ scale: 1 });
  const largo = Math.max(200, scena.clientWidth - 48);
  const alto = Math.max(200, scena.clientHeight - 48);
  const adatta = Math.min(largo / v1.width, alto / v1.height);
  const scala = adatta * ZOOM[V.zoom];
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  const vp = pag.getViewport({ scale: scala * dpr });
  const c = document.createElement("canvas");
  c.width = Math.ceil(vp.width);
  c.height = Math.ceil(vp.height);
  c.style.width = Math.round(vp.width / dpr) + "px";
  c.style.height = Math.round(vp.height / dpr) + "px";
  const ctx = c.getContext("2d");
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, c.width, c.height);
  await pag.render({ canvasContext: ctx, viewport: vp, background: "#ffffff" }).promise;
  if (gen !== V.gen) return;
  carta.replaceChildren(c);
  carta.style.width = c.style.width;
  carta.style.height = c.style.height;
  disegnaNote();
  barraVisore();
}

function barraVisore() {
  const p = nomePdf(V.file);
  document.getElementById("dst-vis-titolo").textContent = p ? p.nome : "Disegno";
  let pezzo = "";
  if (p && p.cc) pezzo = "pezzo " + p.cc + " · ";
  else if (V.cod) pezzo = "pezzo proposto " + V.cod + " · ";
  else if (p && !p.comp) pezzo = "non ancora di un pezzo · ";
  if (p && p.stato !== "doc") pezzo += "file proposto, non ancora confermato · ";
  document.getElementById("dst-vis-sotto").textContent = pezzo + "pagina " + V.pagina + " di " + V.pagine;
  document.getElementById("dst-pag").textContent = V.pagina + "/" + V.pagine;
  document.getElementById("dst-pag-prec").disabled = V.pagina <= 1;
  document.getElementById("dst-pag-succ").disabled = V.pagina >= V.pagine;
  document.getElementById("dst-z").textContent = Math.round(ZOOM[V.zoom] * 100) + "%";
  const finestra = document.getElementById("dst-vis-finestra");
  if (finestra) finestra.href = V.file ? urlPdf(V.file) : "#";
  const arma = document.getElementById("dst-vis-arma");
  const scrive = S.dati && S.dati.scrive;
  arma.hidden = !scrive;
  arma.setAttribute("aria-pressed", V.armato ? "true" : "false");
  arma.textContent = V.armato ? "Fai clic sul disegno… (Esc annulla)" : "+ Aggiungi nota";
  document.getElementById("dst-carta").classList.toggle("armata", V.armato);
}

function disegnaNote() {
  const carta = document.getElementById("dst-carta");
  for (const x of $$(".dst-pin, .dst-pop-nota", carta)) x.remove();
  const tutte = noteDi(V.file);
  const qui = tutte.filter((n) => n.p === V.pagina);
  for (const n of qui) {
    carta.append(el("button", {
      class: "dst-pin" + (V.sel === n.id ? " sel" : ""), type: "button", style: `left:${n.x * 100}%;top:${n.y * 100}%`,
      "aria-label": "Nota " + n.n + ": " + n.t, title: n.t,
      onclick: (e) => { e.stopPropagation(); V.sel = n.id; disegnaNote(); },
    }, n.n));
  }
  if (V.nuovo) {
    carta.append(el("span", { class: "dst-pin nuovo", style: `left:${V.nuovo.x * 100}%;top:${V.nuovo.y * 100}%` }, "+"));
    const testo = el("textarea", { id: "dst-nota-testo", maxlength: "2000", placeholder: "Che cosa c'è da sapere su questo punto?", "aria-label": "Testo della nota" });
    const errore = el("span", { class: "errore", hidden: true });
    const salva = el("button", { class: "dst-btn piccolo primario", type: "submit" }, "Salva la nota");
    const pop = el("form", {
      class: "dst-pop-nota",
      style: `left:min(calc(${V.nuovo.x * 100}% + 18px), calc(100% - 280px));top:min(calc(${V.nuovo.y * 100}% - 10px), calc(100% - 170px))`,
      onclick: (e) => e.stopPropagation(),
      onsubmit: async (e) => {
        e.preventDefault();
        const t = testo.value.trim();
        if (!t) { errore.textContent = "Scrivi il testo della nota."; errore.hidden = false; return; }
        salva.disabled = true;
        const es = await gesto(S.dati.base + "/nota", { componente: V.comp, allegato: V.file, pagina: V.pagina, x: V.nuovo.x.toFixed(6), y: V.nuovo.y.toFixed(6), testo: t }, { swap: false });
        salva.disabled = false;
        if (!es.ok) { errore.textContent = es.testo || "La nota non si è potuta salvare."; errore.hidden = false; return; }
        V.nuovo = null;
        await rileggiDati();
        const ultime = noteDi(V.file);
        V.sel = ultime.length ? ultime[ultime.length - 1].id : null;
        avvisa("Nota salvata sul disegno.");
        disegnaNote();
      },
    },
      el("label", { class: "dst-campo" }, el("span", {}, "Nota nuova, pagina " + V.pagina), testo),
      errore,
      el("span", { class: "dst-azioni", style: "justify-content:flex-end" },
        el("button", { class: "dst-btn piccolo", type: "button", onclick: () => { V.nuovo = null; disegnaNote(); } }, "Annulla"), salva));
    carta.append(pop);
    setTimeout(() => testo.focus(), 0);
  }
  // l'elenco a lato
  document.getElementById("dst-vis-n-note").textContent = "Note sul disegno · " + tutte.length;
  document.getElementById("dst-vis-aiuto").textContent = S.dati && S.dati.scrive
    ? "Premi «+ Aggiungi nota», poi fai clic sul punto del disegno."
    : "Chi consulta legge le note ma non ne scrive.";
  const lista = document.getElementById("dst-lista-note");
  lista.replaceChildren();
  if (!tutte.length) lista.append(el("p", { class: "k" }, "Nessuna nota su questo disegno."));
  for (const n of tutte) {
    // la riga non e' un bottone (dentro ha i bottoni della nota): il bottone e' il numero, che porta alla nota; un clic
    // sulla riga fa lo stesso
    const riga = el("div", { class: "dst-nota" + (V.sel === n.id ? " sel" : ""), onclick: () => vaiANota(n) },
      el("button", { type: "button", class: "n", "aria-label": "Vai alla nota " + n.n + ": " + n.t, onclick: (e) => { e.stopPropagation(); vaiANota(n); } }, n.n),
      el("span", {}, n.t),
      el("span", { class: "meta" }, n.chi + " · " + n.quando + " · pag. " + n.p + (n.su ? " · scritta su " + n.su : "")));
    if (n.mia && S.dati.scrive) {
      if (V.modifica === n.id) {
        const ta = el("textarea", { maxlength: "2000", "aria-label": "Testo della nota", onclick: (e) => e.stopPropagation() });
        ta.value = n.t;
        riga.append(ta, el("span", { class: "azioni" },
          el("button", { class: "dst-btn piccolo primario", type: "button", onclick: async (e) => {
            e.stopPropagation();
            const es = await gesto(S.dati.base + "/nota/" + n.id + "/modifica", { testo: ta.value.trim() }, { swap: false });
            if (!es.ok) { avvisa(es.testo, true); return; }
            V.modifica = null; await rileggiDati(); avvisa("Nota cambiata."); disegnaNote();
          } }, "Salva"),
          el("button", { class: "dst-btn piccolo", type: "button", onclick: (e) => { e.stopPropagation(); V.modifica = null; disegnaNote(); } }, "Annulla")));
        setTimeout(() => ta.focus(), 0);
      } else if (V.conferma === n.id) {
        riga.append(el("span", { class: "azioni" }, el("span", { class: "k" }, "Togliere la nota?"),
          el("button", { class: "dst-btn piccolo pericolo", type: "button", onclick: async (e) => {
            e.stopPropagation();
            const es = await gesto(S.dati.base + "/nota/" + n.id + "/elimina", {}, { swap: false });
            if (!es.ok) { avvisa(es.testo, true); return; }
            V.conferma = null; V.sel = null; await rileggiDati(); avvisa("Nota tolta."); disegnaNote();
          } }, "Sì, togli"),
          el("button", { class: "dst-btn piccolo", type: "button", onclick: (e) => { e.stopPropagation(); V.conferma = null; disegnaNote(); } }, "No")));
      } else {
        riga.append(el("span", { class: "azioni" },
          el("button", { class: "dst-btn piccolo", type: "button", onclick: (e) => { e.stopPropagation(); V.modifica = n.id; V.conferma = null; disegnaNote(); } }, "Cambia"),
          el("button", { class: "dst-btn piccolo pericolo", type: "button", onclick: (e) => { e.stopPropagation(); V.conferma = n.id; V.modifica = null; disegnaNote(); } }, "Togli")));
      }
    }
    lista.append(riga);
  }
}

async function vaiANota(n) {
  V.sel = n.id;
  if (n.p !== V.pagina) { V.pagina = n.p; await disegnaPagina(); }
  else disegnaNote();
  const pin = $(".dst-pin.sel", document.getElementById("dst-carta"));
  if (pin) pin.scrollIntoView({ block: "center", inline: "center", behavior: "smooth" });
}

function legaVisore() {
  const vis = document.getElementById("dst-visore");
  if (!vis || vis.dataset.legato) return;
  vis.dataset.legato = "1";
  document.getElementById("dst-vis-chiudi").addEventListener("click", chiudiVisore);
  vis.addEventListener("click", (e) => { if (e.target === vis) chiudiVisore(); });
  document.getElementById("dst-vis-file").addEventListener("change", (e) => {
    const p = nomePdf(e.target.value);
    if (p && !p.ok) { avvisa(p.nome + ": il file non è su questo server.", true); e.target.value = V.file; return; }
    V.file = e.target.value; V.comp = (p && p.comp) || V.comp; V.pagina = 1; V.sel = null; V.nuovo = null; V.armato = false;
    caricaPdf();
  });
  document.getElementById("dst-pag-prec").addEventListener("click", () => { if (V.pagina > 1) { V.pagina--; V.nuovo = null; disegnaPagina(); } });
  document.getElementById("dst-pag-succ").addEventListener("click", () => { if (V.pagina < V.pagine) { V.pagina++; V.nuovo = null; disegnaPagina(); } });
  document.getElementById("dst-z-piu").addEventListener("click", () => { if (V.zoom < ZOOM.length - 1) { V.zoom++; disegnaPagina(); } });
  document.getElementById("dst-z-meno").addEventListener("click", () => { if (V.zoom > 0) { V.zoom--; disegnaPagina(); } });
  document.getElementById("dst-z-adatta").addEventListener("click", () => { V.zoom = 0; disegnaPagina(); });
  document.getElementById("dst-vis-arma").addEventListener("click", () => { V.armato = !V.armato; V.nuovo = null; barraVisore(); disegnaNote(); });
  document.getElementById("dst-carta").addEventListener("click", (e) => {
    const c = $("canvas", e.currentTarget);
    if (!c) return;
    if (!V.armato) { if (V.sel) { V.sel = null; disegnaNote(); } return; }
    const r = c.getBoundingClientRect();
    const x = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    const y = Math.min(1, Math.max(0, (e.clientY - r.top) / r.height));
    V.nuovo = { x, y };
    V.armato = false;
    barraVisore();
    disegnaNote();
  });
  // il fuoco resta nel visore aperto: Tab gira fra i suoi comandi, non esce sulla pagina di sotto
  vis.addEventListener("keydown", (e) => {
    if (e.key !== "Tab" || vis.hidden) return;
    const giri = $$("button:not([disabled]):not([hidden]), select, a[href], textarea, [tabindex='0']", vis).filter((x) => x.offsetParent !== null);
    if (!giri.length) return;
    const primo = giri[0], ultimo = giri[giri.length - 1];
    if (e.shiftKey && document.activeElement === primo) { e.preventDefault(); ultimo.focus(); }
    else if (!e.shiftKey && document.activeElement === ultimo) { e.preventDefault(); primo.focus(); }
  });
  document.addEventListener("keydown", (e) => {
    if (vis.hidden || e.key !== "Escape") return;
    if (V.nuovo) { V.nuovo = null; disegnaNote(); } else if (V.armato) { V.armato = false; barraVisore(); } else chiudiVisore();
  });
  let tRidim = 0;
  window.addEventListener("resize", () => { if (vis.hidden) return; clearTimeout(tRidim); tRidim = setTimeout(disegnaPagina, 200); });
}

// ------------------------------------------------------------------ la finestra dei comandi e il menu

// Dialogo e' la finestra dei comandi dell'albero (un <dialog> vero: tiene il fuoco dentro, Esc la chiude). Quando si
// chiude il fuoco torna al pezzo dell'albero su cui si lavorava (ritorno: la sua chiave), o a chi l'aveva aperta.
const Dialogo = {
  da: null,
  ritorno: null,
  el() { return document.getElementById("dst-dialogo"); },
  apri(titolo, corpo, azioni, opz = {}) {
    const d = this.el();
    if (!d) return null;
    if (!d.open) this.da = opz.da || document.activeElement;
    this.ritorno = opz.ritorno || null;
    d.className = "dst-dialogo" + (opz.largo ? " largo" : "");
    d.replaceChildren(...[
      el("div", { class: "dst-dialogo-testa" }, el("h2", { id: "dst-dialogo-titolo" }, titolo),
        el("button", { type: "button", class: "dst-btn piccolo", "aria-label": "Chiudi", onclick: () => this.chiudi() }, "✕")),
      el("div", { class: "dst-dialogo-corpo" }, corpo),
      azioni && azioni.length ? el("div", { class: "dst-azioni dst-dialogo-azioni" }, azioni) : null].filter(Boolean));
    if (!d.open) d.showModal();
    const primo = opz.fuoco || $("[autofocus], .dst-dialogo-corpo input:not([type=hidden]), .dst-dialogo-corpo select", d) || $(".dst-dialogo-azioni button", d);
    if (primo) primo.focus();
    return d;
  },
  chiudi(ritorno) {
    const d = this.el();
    if (ritorno !== undefined) this.ritorno = ritorno;
    if (d && d.open) d.close();
  },
  lega() {
    const d = this.el();
    if (!d || d.dataset.legato) return;
    d.dataset.legato = "1";
    d.addEventListener("close", () => {
      const k = this.ritorno;
      this.ritorno = null;
      // l'evento arriva dopo la chiusura: se nel frattempo il fuoco e' andato altrove (un menu aperto subito dopo), non
      // lo si ruba. Si rimette solo se e' perso (sul body), ancora nella finestra, o su chi l'aveva aperta (il browser
      // lo riporta li' da solo, e il pezzo su cui si e' agito e' un posto migliore)
      const a = document.activeElement;
      if (a && a !== document.body && !d.contains(a) && a !== this.da) return;
      if (k && S.editor && S.editor.fuocoSu(k)) return;
      if (this.da && this.da.isConnected) this.da.focus();
      else if (S.editor) S.editor.fuocoSu(S.editor.sel);
    });
  },
};

// Menu e' il menu del tasto destro sui pezzi (domanda 29a), lo stesso da tastiera: le frecce lo percorrono, Invio
// sceglie, Esc (o Tab) lo chiude e il fuoco torna al pezzo.
const Menu = {
  da: null,
  apertoIl: 0,
  // daTastiera: quando il menu e' stato aperto dal tasto menu o da Maiusc+F10. Il browser manda anche il suo
  // «contextmenu» per quei tasti, a volte dopo che il menu e' gia' stato chiuso con Esc: non deve riaprirlo
  daTastiera: 0,
  el() { return document.getElementById("dst-menu"); },
  appenaAperto() { const m = this.el(); return !!m && !m.hidden && Date.now() - this.apertoIl < 600; },
  ecoDellaTastiera() { return Date.now() - this.daTastiera < 1000; },
  apri(voci, x, y, da) {
    const m = this.el();
    if (!m) return;
    this.da = da || document.activeElement;
    this.apertoIl = Date.now();
    m.replaceChildren(...voci.map((v) => v === "-" ? el("div", { class: "sep", role: "separator" }) : el("button", {
      type: "button", role: "menuitem", class: "voce" + (v.pericolo ? " pericolo" : ""), tabindex: "-1",
      "aria-disabled": v.spento ? "true" : null, title: v.spento || null,
      onclick: () => {
        if (v.spento) { avvisa(v.spento, true); return; }
        // il fuoco torna al pezzo prima del comando: una finestra che si apre lo restituisce li', un gesto lo ritrova
        this.chiudi(true);
        v.fai();
      },
    }, v.testo, v.spento ? el("small", {}, " · " + v.spento) : null)));
    m.hidden = false;
    const r = m.getBoundingClientRect();
    m.style.left = Math.max(4, Math.min(x, window.innerWidth - r.width - 4)) + "px";
    m.style.top = Math.max(4, Math.min(y, window.innerHeight - r.height - 4)) + "px";
    const primo = $(".voce:not([aria-disabled])", m) || $(".voce", m);
    if (primo) primo.focus();
  },
  chiudi(ridai = true) {
    const m = this.el();
    if (!m || m.hidden) return;
    m.hidden = true;
    if (ridai && this.da && this.da.isConnected) this.da.focus();
  },
  lega() {
    const m = this.el();
    if (!m || m.dataset.legato) return;
    m.dataset.legato = "1";
    m.addEventListener("keydown", (e) => {
      const voci = $$(".voce", m);
      const i = voci.indexOf(document.activeElement);
      switch (e.key) {
        case "ArrowDown": e.preventDefault(); voci[(i + 1) % voci.length].focus(); break;
        case "ArrowUp": e.preventDefault(); voci[(i - 1 + voci.length) % voci.length].focus(); break;
        case "Home": e.preventDefault(); voci[0].focus(); break;
        case "End": e.preventDefault(); voci[voci.length - 1].focus(); break;
        case "Escape": case "Tab": e.preventDefault(); this.chiudi(true); break;
      }
    });
    // dentro il menu il tasto destro non apre il menu del browser
    m.addEventListener("contextmenu", (e) => e.preventDefault());
    document.addEventListener("mousedown", (e) => { if (!m.hidden && !m.contains(e.target)) this.chiudi(false); }, true);
    window.addEventListener("resize", () => this.chiudi(false));
    // lo scorrimento chiude il menu, ma non quello che arriva mentre si apre (il riquadro del pezzo che si ridisegna
    // cambia l'altezza della pagina, e il browser riassesta lo scorrimento)
    window.addEventListener("scroll", () => { if (!this.appenaAperto()) this.chiudi(false); }, true);
  },
};

// ------------------------------------------------------------------ la bozza dell'albero, nel browser

const FORMATO_BOZZA = 1;
function bozzaVuota(base) {
  return { formato: FORMATO_BOZZA, base, rinomine: [], tolti: [], aggiunti: [], legami: [], quantita: [], tipi: [], commerciali: [], diversi: [] };
}
const LISTE = ["rinomine", "tolti", "aggiunti", "legami", "quantita", "tipi", "commerciali", "diversi"];
function vociBozza(b) { return LISTE.reduce((n, k) => n + ((b && b[k]) || []).length, 0); }
function bozzaValida(b) { return !!b && b.formato === FORMATO_BOZZA && typeof b.base === "string" && LISTE.every((k) => Array.isArray(b[k])); }

// Memoria e' la bozza in localStorage: per RFQ e per utente (su una postazione condivisa un collega non riprende la
// bozza di un altro), con la firma dell'albero su cui e' disegnata e l'ora. La memoria del browser puo' mancare (una
// finestra privata, la pulizia): la bozza allora vive solo nella pagina, come prima.
const Memoria = {
  chiave() { return "cockpit.distinta.bozza." + (S.dati ? S.dati.thread : "") + "." + ((S.dati && S.dati.utente) || ""); },
  leggi() {
    const x = prova(() => JSON.parse(localStorage.getItem(this.chiave()) || "null"), null);
    return x && bozzaValida(x.bozza) && vociBozza(x.bozza) ? x : null;
  },
  scrivi(b) {
    prova(() => {
      if (!vociBozza(b)) localStorage.removeItem(this.chiave());
      else localStorage.setItem(this.chiave(), JSON.stringify({ formato: FORMATO_BOZZA, quando: Date.now(), bozza: b }));
    });
  },
  togli() { prova(() => localStorage.removeItem(this.chiave())); },
};
const ora = (t) => prova(() => new Date(t).toLocaleTimeString("it-IT", { hour: "2-digit", minute: "2-digit" }), "");

// ------------------------------------------------------------------ l'albero proposto

class Distinta {
  static async avvia() {
    const tree = document.getElementById("dst-tree");
    if (!tree || !S.dati) return;
    let a;
    try {
      const r = await fetch(S.dati.pagina + "/albero", { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(r.status);
      a = await r.json();
    } catch (e) {
      tree.replaceChildren(el("p", { class: "bad-t" }, "L'albero proposto non si è potuto leggere: ricarica la pagina."));
      return;
    }
    if (!document.getElementById("dst-tree")) return; // la pagina e' cambiata nel frattempo
    S.editor = new Distinta(a);
  }

  constructor(a) {
    this.a = a;
    this.nodi = new Map((a.nodi || []).map((n) => [n.chiave, n]));
    this.prodotti = a.prodotti || [];
    this.scrive = !!S.dati.scrive && !S.dati.bloccata;
    this.bozza = bozzaVuota(a.firma);
    this.storia = [];
    this.chiaveProdotto = "cockpit.distinta.prodotto." + S.dati.thread;
    let scelto = prova(() => sessionStorage.getItem(this.chiaveProdotto), null);
    if (!this.prodotti.some((p) => p.chiave === scelto)) scelto = this.prodotti.length ? this.prodotti[0].chiave : "";
    this.radice = scelto;
    this.sel = this.radice;
    this.selPadre = "";
    // la bozza trovata in questo browser: si chiede se riprenderla, non si applica da sola
    this.trovata = this.scrive ? Memoria.leggi() : null;
    this.vista = this.calcola(this.bozza);
    this.disegna();
    // dopo un gesto che ha rifatto la pagina (la conferma, «Riapri»), il fuoco torna sull'albero
    if (S.fuocoDopo) {
      const k = S.fuocoDopo;
      S.fuocoDopo = null;
      this.fuocoSu(this.nodi.has(k) ? k : this.radice);
    }
  }

  // ---- la bozza sull'albero: le stesse regole del riepilogo del server (fascicolo.applica), per disegnare

  calcola(b) {
    const pezzi = new Map();
    for (const n of this.a.nodi || []) {
      if (n.stato === "scartato") continue;
      pezzi.set(n.chiave, { k: n.chiave, n, codice: n.codice || "", tipo: n.tipo, rev: n.rev || "" });
    }
    const legami = new Map();
    for (const l of this.a.archi || []) {
      if (l.stato === "scartato") continue;
      legami.set(l.padre + "|" + l.figlio, { padre: l.padre, figlio: l.figlio, qta: l.qta, discordi: !!l.qta_discordi, arco: l });
    }
    for (const x of b.aggiunti) pezzi.set(x.id, { k: x.id, ag: x, codice: x.codice, tipo: x.tipo, rev: x.rev || "" });
    for (const x of b.aggiunti) if (pezzi.has(x.padre)) legami.set(x.padre + "|" + x.id, { padre: x.padre, figlio: x.id, qta: x.qta, scritto: true });
    for (const t of b.tolti) {
      if (t.padre) { legami.delete(t.padre + "|" + t.nodo); continue; }
      const p = pezzi.get(t.nodo);
      if (p) p.tolto = true;
      for (const [k, l] of [...legami]) if (l.padre === t.nodo || l.figlio === t.nodo) legami.delete(k);
    }
    for (const l of b.legami) if (pezzi.has(l.padre) && pezzi.has(l.figlio)) legami.set(l.padre + "|" + l.figlio, { padre: l.padre, figlio: l.figlio, qta: l.qta, scritto: true });
    for (const q of b.quantita) { const l = legami.get(q.padre + "|" + q.figlio); if (l) { l.qta = q.qta; l.scelta = true; l.discordi = false; } }
    for (const r of b.rinomine) { const p = pezzi.get(r.nodo); if (p) { p.codice = r.codice; p.rinominato = true; if (r.rev) p.rev = r.rev; } }
    for (const t of b.tipi) { const p = pezzi.get(t.nodo); if (p) { p.tipo = t.tipo; p.tipoScelto = true; } }
    for (const r of b.commerciali) { const p = pezzi.get(r.nodo); if (p) p.risposta = r.risposta; }
    for (const k of b.diversi) { const p = pezzi.get(k); if (p) p.diverso = true; }
    const figli = new Map(), padri = new Map();
    for (const l of legami.values()) {
      if (!figli.has(l.padre)) figli.set(l.padre, []);
      figli.get(l.padre).push(l);
      if (!padri.has(l.figlio)) padri.set(l.figlio, []);
      padri.get(l.figlio).push(l);
    }
    const raggiunti = new Set(this.prodotti.map((p) => p.chiave));
    const coda = [...raggiunti];
    while (coda.length) {
      const k = coda.shift();
      for (const l of figli.get(k) || []) if (!raggiunti.has(l.figlio)) { raggiunti.add(l.figlio); coda.push(l.figlio); }
    }
    return { pezzi, legami, figli, padri, raggiunti };
  }

  // pulisci toglie dalla bozza quello che non regge piu' dopo un gesto (un legame o una quantita' di un pezzo tolto, un
  // pezzo aggiunto sotto un padre tolto): il riepilogo lo rifiuterebbe. La finestra della cascata lo ha gia' detto.
  pulisci(b) {
    for (let giro = 0; giro < 20; giro++) {
      const v = this.calcola(b);
      const vivo = (k) => { const p = v.pezzi.get(k); return !!p && !p.tolto; };
      const n = vociBozza(b);
      // un pezzo aggiunto sotto un padre che non si raggiunge piu' da un prodotto va via con la cascata (il riepilogo lo
      // rifiuterebbe: «non si raggiunge più da un prodotto»)
      b.aggiunti = b.aggiunti.filter((x) => vivo(x.padre) && v.raggiunti.has(x.padre));
      const ids = new Set(b.aggiunti.map((x) => x.id));
      const esiste = (k) => this.nodi.has(k) || ids.has(k);
      b.legami = b.legami.filter((l) => vivo(l.padre) && vivo(l.figlio));
      b.quantita = b.quantita.filter((q) => v.legami.has(q.padre + "|" + q.figlio));
      b.rinomine = b.rinomine.filter((r) => esiste(r.nodo));
      b.tipi = b.tipi.filter((t) => esiste(t.nodo));
      b.commerciali = b.commerciali.filter((r) => esiste(r.nodo));
      b.diversi = b.diversi.filter((k) => esiste(k));
      b.tolti = b.tolti.filter((t) => esiste(t.nodo) && (!t.padre || esiste(t.padre)));
      if (vociBozza(b) === n) break;
    }
    return b;
  }

  // ---- come si legge l'albero con la bozza

  pezzo(k) { return this.vista.pezzi.get(k) || null; }
  nome(k) {
    const p = this.pezzo(k);
    const n = p ? p.n : this.nodi.get(k);
    if (p && p.codice) return p.codice;
    if (n && n.codice) return n.codice;
    if (n && n.nome) return "«" + n.nome + "»";
    return "senza codice";
  }
  tipoDi(k) {
    const p = this.pezzo(k);
    if (!p) { const n = this.nodi.get(k); return n ? n.tipo : ""; }
    if (p.n && p.n.prodotto) return "finito";
    if (p.risposta === "si") return "commerciale";
    return p.tipo || "";
  }
  // chi puo' avere dei pezzi sotto: il prodotto e l'assieme (un particolare non ha figli, 6b; il commerciale e' una
  // foglia, 6a)
  contenitore(k) { const t = this.tipoDi(k); return t === "finito" || t === "sottoassieme"; }
  prodotto(k) { const p = this.pezzo(k); return !!(p && p.n && p.n.prodotto); }
  figliDi(k) { return (this.vista.figli.get(k) || []).slice().sort((x, y) => this.nome(x.figlio).localeCompare(this.nome(y.figlio))); }
  padriDi(k) { return (this.vista.padri.get(k) || []).slice().sort((x, y) => this.nome(x.padre).localeCompare(this.nome(y.padre))); }
  // scendeDa: anc sta sotto k (o e' k): mettere k sotto anc chiuderebbe un giro
  scendeDa(anc, k) {
    if (anc === k) return true;
    const visti = new Set([k]);
    const coda = [k];
    while (coda.length) {
      const x = coda.shift();
      for (const l of this.vista.figli.get(x) || []) {
        if (l.figlio === anc) return true;
        if (!visti.has(l.figlio)) { visti.add(l.figlio); coda.push(l.figlio); }
      }
    }
    return false;
  }
  // rinominabile: il codice si scrive solo per un pezzo proposto (non ancora un componente della distinta), non per un
  // prodotto (viene dal triage) ne' per un aggancio per codice di prima dello Smistamento (si corregge nel Fascicolo)
  rinominabile(k) {
    const p = this.pezzo(k);
    if (!p || p.tolto) return false;
    if (p.ag) return true;
    return !p.n.prodotto && p.n.stato === "proposto" && !(p.n.ritrovato && p.n.ritrovato.agganciato);
  }
  stato(k) {
    const p = this.pezzo(k);
    if (!p) return "scartato";
    if (p.ag) return "nuovo";
    if (p.n.prodotto) return "prodotto";
    return p.n.stato;
  }
  cambiato() { return vociBozza(this.bozza) > 0; }
  // i pezzi che «Conferma l'albero» decide: quelli proposti e quelli ritrovati per codice con le righe dei file ancora
  // da decidere (il riepilogo li elenca), come conta la linguetta del passo 2
  daConfermare() {
    let n = 0;
    for (const p of this.vista.pezzi.values()) if (p.n && !p.n.prodotto && this.vista.raggiunti.has(p.k) && (p.n.stato === "proposto" || p.n.ritrovato)) n++;
    return n;
  }
  // i codici che l'albero usa, per il codice interno libero
  codiciUsati() {
    const out = new Set();
    for (const p of this.vista.pezzi.values()) if (p.codice) out.add(p.codice.toUpperCase());
    for (const n of this.nodi.values()) if (n.codice) out.add(n.codice.toUpperCase());
    for (const f of S.dati.fuori || []) out.add(f.codice.toUpperCase());
    return out;
  }
  // il codice interno di un assieme che nasce qui (domanda 10 = A): <prodotto>-A01, -A02… il primo libero. Lo si usa
  // solo con il suo bottone: il campo del codice parte vuoto (P5)
  codiceInterno() {
    const base = this.nome(this.radice);
    const usati = this.codiciUsati();
    for (let i = 1; i < 100; i++) {
      const c = base + "-A" + String(i).padStart(2, "0");
      if (!usati.has(c.toUpperCase())) return c;
    }
    return base + "-A";
  }

  // ---- i gesti sulla bozza (restano in questo browser finche' non si conferma)

  proteggi() {
    if (!this.scrive) { avvisa(S.dati.bloccata ? "La distinta è congelata: si cambia aprendo una revisione." : "Chi consulta non cambia la distinta.", true); return false; }
    if (this.trovata) { avvisa("Prima scegli che cosa fare della bozza trovata qui sopra: riprenderla o scartarla.", true); return false; }
    return true;
  }
  // cambia applica un gesto alla bozza: prima una copia per «Annulla l'ultima», poi il gesto, la pulizia, la memoria del
  // browser e il disegno, con il fuoco sul pezzo (fuoco) o dov'era
  cambia(fn, messaggio, fuoco) {
    if (!this.proteggi()) return false;
    const prima = JSON.stringify(this.bozza);
    const b = JSON.parse(prima);
    const esito = fn(b);
    if (esito === false) return false;
    this.pulisci(b);
    if (JSON.stringify(b) === prima) return false;
    this.storia.push(prima);
    if (this.storia.length > 200) this.storia.shift();
    this.bozza = b;
    this.dopo(messaggio, fuoco);
    return true;
  }
  dopo(messaggio, fuoco) {
    Memoria.scrivi(this.bozza);
    this.vista = this.calcola(this.bozza);
    if (!this.pezzo(this.sel) || !this.vista.raggiunti.has(this.sel)) this.sel = this.vista.raggiunti.has(fuoco) ? fuoco : this.radice;
    this.disegna(messaggio, fuoco);
  }
  annullaUltima() {
    const s = this.storia.pop();
    if (!s) return;
    this.bozza = JSON.parse(s);
    this.dopo("Ultima modifica annullata.");
  }

  rinomina(k, codice, rev) {
    codice = (codice || "").trim();
    rev = (rev || "").trim().toUpperCase();
    if (!codice) { avvisa("Scrivi il codice.", true); return false; }
    if (codice.length > 40 || /\s/.test(codice)) { avvisa("Un codice ha al massimo 40 caratteri, senza spazi.", true); return false; }
    if (rev.length > 10 || /\s/.test(rev)) { avvisa("Una revisione ha al massimo 10 caratteri, senza spazi.", true); return false; }
    const p = this.pezzo(k);
    return this.cambia((b) => {
      if (p.ag) {
        const x = b.aggiunti.find((y) => y.id === k);
        x.codice = codice;
        x.rev = rev;
        return;
      }
      b.rinomine = b.rinomine.filter((r) => r.nodo !== k);
      if (codice !== (p.n.codice || "") || (rev && rev !== (p.n.rev || "").toUpperCase())) b.rinomine.push({ nodo: k, codice, rev });
    }, codice === (p.codice || "") ? "" : `${this.nome(k)} si chiamerà ${codice}: con la conferma dell'albero.`, k);
  }

  // togli: il pezzo intero (padre vuoto: da tutti i padri, con la cascata) o un legame solo («togli da qui»). Un pezzo
  // aggiunto nella bozza non nasce e basta; un legame messo nella bozza si toglie dalla bozza.
  togli(k, padre) {
    const p = this.pezzo(k);
    if (!p) return false;
    return this.cambia((b) => {
      if (p.ag && !padre) {
        b.aggiunti = b.aggiunti.filter((x) => x.id !== k);
        return;
      }
      if (padre) {
        const ag = b.aggiunti.find((x) => x.id === k && x.padre === padre);
        if (ag) { b.aggiunti = b.aggiunti.filter((x) => x.id !== k); return; }
        const l = b.legami.find((x) => x.padre === padre && x.figlio === k);
        if (l) { b.legami = b.legami.filter((x) => x !== l); return; }
        if (!b.tolti.some((t) => t.nodo === k && t.padre === padre)) b.tolti.push({ nodo: k, padre });
        return;
      }
      b.tolti = b.tolti.filter((t) => t.nodo !== k);
      b.tolti.push({ nodo: k });
    }, padre ? `${this.nome(k)} non è più sotto ${this.nome(padre)}.` : `${this.nome(k)} tolto dall'albero${p.ag ? ": non nasce" : ""}.`, padre || this.radice);
  }
  ripristina(k, padre) {
    return this.cambia((b) => { b.tolti = b.tolti.filter((t) => !(t.nodo === k && (t.padre || "") === (padre || ""))); },
      padre ? `${this.nome(k)} torna sotto ${this.nome(padre)}.` : `${this.nome(k)} torna nell'albero.`, k);
  }

  aggiungi(x) {
    const ids = this.bozza.aggiunti.map((y) => parseInt(y.id.slice(6), 10) || 0);
    const id = "nuovo:" + (Math.max(0, ...ids) + 1);
    return this.cambia((b) => {
      b.aggiunti.push({ id, codice: x.codice, rev: x.rev || "", tipo: x.tipo, padre: x.padre, qta: x.qta });
      if (x.diverso) b.diversi.push(id);
    }, `${x.codice} (${TIPI[x.tipo].toLowerCase()}) sotto ${this.nome(x.padre)}: nasce con la conferma dell'albero.`, id);
  }

  // vietato: perche' figlio non puo' andare sotto verso (niente, se puo')
  vietato(figlio, verso) {
    if (!this.pezzo(verso) || this.pezzo(verso).tolto) return `${this.nome(verso)} non è nell'albero.`;
    if (!this.contenitore(verso)) return `${this.nome(verso)} è ${TIPI_FRASE[this.tipoDi(verso)] || "un pezzo"}: sotto non ci va niente. Mettilo sotto il prodotto o sotto un assieme.`;
    if (this.prodotto(figlio)) return `${this.nome(figlio)} è un prodotto della richiesta: non va sotto un altro pezzo.`;
    if (this.scendeDa(verso, figlio)) return `${this.nome(figlio)} non può andare sotto ${this.nome(verso)}: ${this.nome(verso)} sta già sotto di lui.`;
    if (this.vista.legami.has(verso + "|" + figlio)) return `${this.nome(figlio)} è già sotto ${this.nome(verso)}.`;
    return "";
  }
  // legame: «anche sotto» (un padre in piu')
  legame(figlio, verso, qta) {
    const no = this.vietato(figlio, verso);
    if (no) { avvisa(no, true); return false; }
    return this.cambia((b) => this.metti(b, figlio, verso, qta || 1), `${this.nome(figlio)} anche sotto ${this.nome(verso)}.`, figlio);
  }
  // metti porta figlio sotto verso nella bozza: se era un legame dell'albero tolto nella bozza, lo rimette com'era
  metti(b, figlio, verso, qta) {
    const tolto = b.tolti.find((t) => t.nodo === figlio && t.padre === verso);
    if (tolto) {
      b.tolti = b.tolti.filter((t) => t !== tolto);
      b.quantita = b.quantita.filter((x) => !(x.padre === verso && x.figlio === figlio));
      const a = (this.a.archi || []).find((l) => l.padre === verso && l.figlio === figlio);
      if (a && (qta !== a.qta || a.qta_discordi)) b.quantita.push({ padre: verso, figlio, qta });
      return;
    }
    b.legami.push({ padre: verso, figlio, qta });
  }
  sposta(padre, figlio, verso) {
    if (!verso || verso === padre) return false;
    const no = this.vietato(figlio, verso);
    if (no) { avvisa(no, true); return false; }
    const l = padre ? this.vista.legami.get(padre + "|" + figlio) : null;
    const qta = l ? (l.discordi ? 1 : l.qta) : 1;
    return this.cambia((b) => {
      const ag = b.aggiunti.find((x) => x.id === figlio && x.padre === padre);
      if (ag) { ag.padre = verso; return; }
      if (padre) {
        const scritto = b.legami.find((x) => x.padre === padre && x.figlio === figlio);
        if (scritto) b.legami = b.legami.filter((x) => x !== scritto);
        else b.tolti.push({ nodo: figlio, padre });
      }
      this.metti(b, figlio, verso, qta);
    }, `${this.nome(figlio)} ora è sotto ${this.nome(verso)}.`, figlio);
  }
  quantita(padre, figlio, q) {
    const n = parseInt(q, 10);
    if (!(n >= 1 && n <= 100000)) { avvisa("La quantità va da 1 a 100000.", true); this.disegna(); return false; }
    const l = this.vista.legami.get(padre + "|" + figlio);
    if (!l || (n === l.qta && !l.discordi)) return false;
    return this.cambia((b) => {
      const ag = b.aggiunti.find((x) => x.id === figlio && x.padre === padre);
      if (ag) { ag.qta = n; return; }
      const scritto = b.legami.find((x) => x.padre === padre && x.figlio === figlio);
      if (scritto) { scritto.qta = n; return; }
      b.quantita = b.quantita.filter((x) => !(x.padre === padre && x.figlio === figlio));
      if (!l.arco || n !== l.arco.qta || l.arco.qta_discordi) b.quantita.push({ padre, figlio, qta: n });
    }, `${this.nome(figlio)} sotto ${this.nome(padre)}: quantità ${n}.`, figlio);
  }
  // tipo: il tipo scelto. Per un pezzo con la proposta commerciale, «particolare commerciale» e' il suo ✓ (la stessa
  // cosa per il server); un altro tipo lascia la domanda aperta (non e' il ✗)
  tipo(k, t) {
    const p = this.pezzo(k);
    if (!p || p.n && p.n.prodotto) return false;
    if (t !== "sottoassieme" && (this.vista.figli.get(k) || []).length) {
      avvisa(`${this.nome(k)} ha dei pezzi sotto: per farlo diventare ${TIPI_FRASE[t]} sposta prima i suoi pezzi (un particolare non ha figli, un commerciale è una foglia).`, true);
      this.disegna();
      return false;
    }
    return this.cambia((b) => {
      if (p.ag) { b.aggiunti.find((x) => x.id === k).tipo = t; return; }
      b.tipi = b.tipi.filter((x) => x.nodo !== k);
      if (p.n.commerciale) {
        b.commerciali = b.commerciali.filter((x) => x.nodo !== k);
        if (t === "commerciale") { b.commerciali.push({ nodo: k, risposta: "si" }); return; }
      }
      if (t !== p.n.tipo) b.tipi.push({ nodo: k, tipo: t });
    }, `${this.nome(k)} è ${TIPI_FRASE[t]}.`, k);
  }
  // risposta: il ✓ o il ✗ dell'ingegnere alla minuteria proposta (domanda 30); la stessa risposta di nuovo la toglie
  risposta(k, r) {
    const p = this.pezzo(k);
    if (!p || !p.n || !p.n.commerciale) return false;
    if (r === "si" && (this.vista.figli.get(k) || []).length) { avvisa(`${this.nome(k)} ha dei pezzi sotto: un particolare commerciale è una foglia (6a).`, true); return false; }
    const via = p.risposta === r;
    return this.cambia((b) => {
      b.commerciali = b.commerciali.filter((x) => x.nodo !== k);
      b.tipi = b.tipi.filter((x) => x.nodo !== k);
      if (!via) b.commerciali.push({ nodo: k, risposta: r });
    }, via ? `${this.nome(k)}: la domanda sulla minuteria torna aperta.` : r === "si" ? `${this.nome(k)}: particolare commerciale (✓).` : `${this.nome(k)}: non è minuteria (✗).`, k);
  }
  diverso(k, si) {
    return this.cambia((b) => {
      b.diversi = b.diversi.filter((x) => x !== k);
      if (si) b.diversi.push(k);
    }, si ? `${this.nome(k)} è un pezzo diverso dai codici quasi uguali.` : "", k);
  }

  // ---- la bozza trovata nel browser

  riprendi() {
    const t = this.trovata;
    if (!t) return;
    let b = JSON.parse(JSON.stringify(t.bozza));
    let persi = 0;
    if (b.base !== this.a.firma) {
      // una bozza vecchia: si tiene quello che riguarda ancora pezzi dell'albero di adesso, e la si rivede
      const n = vociBozza(b);
      const ids = new Set(b.aggiunti.map((x) => x.id));
      const c = (k) => this.nodi.has(k) || ids.has(k);
      b.rinomine = b.rinomine.filter((r) => this.nodi.has(r.nodo) && this.nodi.get(r.nodo).stato === "proposto");
      b.commerciali = b.commerciali.filter((r) => this.nodi.has(r.nodo) && this.nodi.get(r.nodo).commerciale);
      b.tolti = b.tolti.filter((x) => c(x.nodo) && (!x.padre || c(x.padre)));
      b.legami = b.legami.filter((x) => c(x.padre) && c(x.figlio));
      b.aggiunti = b.aggiunti.filter((x) => c(x.padre));
      b.base = this.a.firma;
      this.pulisci(b);
      persi = n - vociBozza(b);
    }
    this.trovata = null;
    this.bozza = b;
    this.storia = [];
    this.dopo(persi ? `Bozza ripresa: ${conta(persi, "modifica non vale", "modifiche non valgono")} più sull'albero di adesso. Rivedila.` : "Bozza ripresa.", this.radice);
  }
  scartaTrovata() {
    this.trovata = null;
    Memoria.togli();
    this.disegna("Bozza scartata: l'albero è quello proposto.", this.radice);
  }

  // ---- le finestre

  dialogoRinomina(k) {
    if (!this.proteggi() || !this.rinominabile(k)) return;
    const p = this.pezzo(k);
    const n = p.n || {};
    const cod = el("input", { class: "mono", maxlength: "40", autocomplete: "off", value: p.codice || "", "aria-describedby": "dst-rin-aiuto" });
    const rev = el("input", { class: "mono", maxlength: "10", autocomplete: "off", value: p.rinominato || p.ag ? (p.rev || "") : "" });
    const fonte = p.ag ? "scritto nella bozza" : n.codice ? (FONTI[n.fonte_codice] || "dallo STEP") + (n.codice_nel_file ? ` (nel file: ${n.codice_nel_file})` : "") : "lo STEP non dice un codice";
    const fai = () => { if (this.rinomina(k, cod.value, rev.value) !== false) Dialogo.chiudi(k); };
    Dialogo.apri(`Il codice di ${this.nome(k)}`, [
      el("p", { class: "k", id: "dst-rin-aiuto" }, "Il codice proposto: " + (n.codice || "nessuno") + " · " + fonte + ". Diventa il codice del pezzo solo con la conferma dell'albero."),
      el("form", { class: "dst-form-nuovo", onsubmit: (e) => { e.preventDefault(); fai(); } },
        el("label", { class: "dst-campo" }, el("span", {}, "Codice"), cod),
        el("label", { class: "dst-campo" }, el("span", {}, "Revisione (se la sai)"), rev)),
    ], [el("button", { type: "button", class: "dst-btn primario", onclick: fai }, "Rinomina"),
      el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi(k) }, "Annulla")], { fuoco: cod, ritorno: k });
    cod.select();
  }

  // la cascata di un «togli», come la dira' il riepilogo (29b = A): che cosa va via e che cosa resta sotto altri padri
  cascata(k, padre) {
    const b = JSON.parse(JSON.stringify(this.bozza));
    const p = this.pezzo(k);
    if (p && p.ag && !padre) b.aggiunti = b.aggiunti.filter((x) => x.id !== k);
    else if (padre) b.tolti.push({ nodo: k, padre });
    else b.tolti.push({ nodo: k });
    this.pulisci(b);
    const dopo = this.calcola(b);
    const vanno = [...this.vista.raggiunti].filter((x) => !dopo.raggiunti.has(x));
    // restano: i discendenti di k che un altro padre tiene
    const restano = [];
    const visti = new Set([k]);
    const coda = [k];
    while (coda.length) {
      const x = coda.shift();
      for (const l of this.vista.figli.get(x) || []) {
        if (visti.has(l.figlio)) continue;
        visti.add(l.figlio);
        if (dopo.raggiunti.has(l.figlio)) restano.push({ k: l.figlio, padri: (dopo.padri.get(l.figlio) || []).map((y) => this.nome(y.padre)).sort() });
        else coda.push(l.figlio);
      }
    }
    return { vanno, restano };
  }
  dialogoTogli(k, padre) {
    if (!this.proteggi()) return;
    if (this.prodotto(k)) { avvisa("Il prodotto non si toglie dall'albero: viene dalla richiesta.", true); return; }
    const padri = this.padriDi(k).map((l) => l.padre);
    const riga = (x) => {
      const p = this.pezzo(x);
      const nella = p && p.n && (p.n.stato === "nella_distinta" || p.n.stato === "tolto_dallo_step");
      return el("li", {}, el("span", { class: "mono" }, this.nome(x)), " ", el("span", { class: "k" }, TIPI_FRASE[this.tipoDi(x)] || ""),
        nella ? el("span", { class: "warn-t" }, " · è nella distinta: con la conferma si archivia o si elimina (lo dice il riepilogo)") : null,
        p && p.ag ? el("span", { class: "k" }, " · aggiunto nella bozza: non nasce") : null);
    };
    const corpo = (c, frase) => [
      el("p", {}, frase),
      c.vanno.length ? el("div", {}, el("span", { class: "dst-label" }, `Vanno via (${c.vanno.length})`), el("ul", { class: "dst-elenco" }, c.vanno.map(riga))) : el("p", { class: "k" }, "Non va via nessun pezzo: resta tutto sotto altri padri."),
      c.restano.length ? el("div", {}, el("span", { class: "dst-label" }, `Restano, sotto altri padri (${c.restano.length})`),
        el("ul", { class: "dst-elenco" }, c.restano.map((r) => el("li", {}, el("span", { class: "mono" }, this.nome(r.k)), el("span", { class: "k" }, " · sotto " + r.padri.join(", ")))))) : null,
      el("p", { class: "k" }, "Resta una modifica della bozza: si annulla con «↶ Annulla l'ultima», e l'albero cambia solo con la conferma."),
    ];
    const annulla = el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi(k) }, "Annulla");
    // un pezzo sotto piu' padri: lo si toglie da tutti o solo da qui, e nessuna delle due e' scelta prima (studio § 2.5)
    if (!padre && padri.length > 1 && this.selPadre && padri.includes(this.selPadre)) padre = "?";
    if (padre === "?") {
      const qui = this.selPadre;
      const tutti = this.cascata(k, "");
      Dialogo.apri(`Togliere ${this.nome(k)}?`, [
        el("p", {}, `${this.nome(k)} sta sotto ${padri.map((x) => this.nome(x)).join(", ")}: lo togli da tutti, o solo da sotto ${this.nome(qui)}?`),
        ...corpo(tutti, "Togliendolo da tutti:"),
      ], [el("button", { type: "button", class: "dst-btn pericolo", onclick: () => { Dialogo.chiudi(qui); this.togli(k, ""); } }, "Togli da tutti"),
        el("button", { type: "button", class: "dst-btn pericolo", onclick: () => { Dialogo.chiudi(k); this.togli(k, qui); } }, `Togli solo da sotto ${this.nome(qui)}`), annulla],
      { fuoco: annulla, ritorno: k });
      return;
    }
    const c = this.cascata(k, padre || "");
    const frase = padre ? `Togliere ${this.nome(k)} da sotto ${this.nome(padre)}?` : `Togliere ${this.nome(k)} dall'albero, da tutti i padri?`;
    Dialogo.apri(padre ? `Togliere ${this.nome(k)} da qui?` : `Togliere ${this.nome(k)}?`, corpo(c, frase),
      [el("button", { type: "button", class: "dst-btn pericolo", onclick: () => { Dialogo.chiudi(padre || this.radice); this.togli(k, padre || ""); } }, "Sì, togli"), annulla],
      { fuoco: annulla, ritorno: k });
  }

  // aggiungi: il codice parte vuoto (P5, domanda 10), con il bottone del codice interno per un assieme; il padre si
  // sceglie fra il prodotto e gli assiemi; un codice quasi uguale a un altro vuole la risposta di una persona (P4)
  dialogoAggiungi(tipo, padre, codiceDato) {
    if (!this.proteggi()) return;
    const contenitori = [...this.vista.raggiunti].filter((r) => this.contenitore(r) && !this.pezzo(r).tolto)
      .sort((a, b) => (this.prodotto(b) - this.prodotto(a)) || this.nome(a).localeCompare(this.nome(b)));
    const preferito = padre && contenitori.includes(padre) ? padre : this.contenitore(this.sel) && contenitori.includes(this.sel) ? this.sel : this.radice;
    const cod = el("input", { class: "mono", maxlength: "40", autocomplete: "off", value: codiceDato || "", placeholder: "il codice del pezzo" });
    if (codiceDato) cod.readOnly = true;
    const interno = el("button", { type: "button", class: "dst-btn piccolo", onclick: () => { cod.value = this.codiceInterno(); cod.focus(); } }, "usa il codice interno " + this.codiceInterno());
    const tipoSel = el("select", {}, ["sottoassieme", "sciolto", "commerciale"].map((t) => el("option", { value: t, selected: t === tipo }, TIPI[t])));
    const padreSel = el("select", {}, contenitori.map((r) => el("option", { value: r, selected: r === preferito }, (TIPI[this.tipoDi(r)] || "Pezzo") + " " + this.nome(r))));
    const qta = el("input", { type: "number", min: "1", max: "100000", value: "1" });
    const rev = el("input", { class: "mono", maxlength: "10", autocomplete: "off", placeholder: "rev" });
    const vicini = el("div", { hidden: true });
    const errore = el("p", { class: "bad-t", hidden: true, role: "alert" });
    const aggiornaInterno = () => { interno.hidden = tipoSel.value !== "sottoassieme" || !!codiceDato; };
    tipoSel.addEventListener("change", aggiornaInterno);
    aggiornaInterno();
    const dice = (t) => { errore.textContent = t; errore.hidden = false; };
    const fai = async () => {
      errore.hidden = true;
      const codice = cod.value.trim();
      const q = parseInt(qta.value, 10);
      if (!codice) return dice(tipoSel.value === "sottoassieme" ? "Scrivi il codice, o usa il codice interno." : "Scrivi il codice del pezzo.");
      if (codice.length > 40 || /\s/.test(codice)) return dice("Un codice ha al massimo 40 caratteri, senza spazi.");
      if (!(q >= 1 && q <= 100000)) return dice("La quantità va da 1 a 100000.");
      if (!this.contenitore(padreSel.value)) return dice("Sotto un particolare non si mette niente: scegli il prodotto o un assieme.");
      for (const p of this.vista.pezzi.values()) {
        if (!p.tolto && (p.codice || "").toUpperCase() === codice.toUpperCase() && this.vista.raggiunti.has(p.k)) {
          return dice(`${codice} è già nell'albero: per metterlo anche sotto ${this.nome(padreSel.value)} usa «Anche sotto…» sul pezzo.`);
        }
      }
      let e;
      try {
        const r = await fetch(S.dati.base + "/bom/codice?codice=" + encodeURIComponent(codice), { credentials: "same-origin", headers: { Accept: "application/json" } });
        if (!r.ok) throw new Error(r.status);
        e = await r.json();
      } catch (err) {
        return dice("Il codice non si è potuto controllare: riprova.");
      }
      if (e.errore) return dice(e.errore);
      if (e.esiste && e.esiste.tipo === "finito") return dice(`${e.esiste.codice} è un prodotto della richiesta: non va sotto un altro pezzo.`);
      if (e.richiesta) return dice(`${codice} è un codice della richiesta: diventa un prodotto con la decisione nella Inbox, non un pezzo qui.`);
      let diverso = false;
      if (e.vicini && e.vicini.length && !(e.esiste)) {
        const spunta = $("input[type=checkbox]", vicini);
        if (!spunta) {
          const elenco = e.vicini.map((v) => `${v.codice} (${v.motivo})`).join(", ");
          vicini.replaceChildren(el("div", { class: "dst-conferma-box warn" },
            el("span", {}, `${codice} è quasi uguale a ${elenco}. Se è lo stesso pezzo, usa quello e non scriverne un altro.`),
            el("label", {}, el("input", { type: "checkbox" }), ` ${codice} è un pezzo diverso`),
            el("small", { class: "k" }, "Se non lo spunti, il pezzo entra nella bozza con la domanda aperta: la conferma aspetta la risposta.")));
          vicini.hidden = false;
          return dice("Guarda i codici quasi uguali qui sopra, poi di nuovo «Aggiungi».");
        }
        diverso = spunta.checked;
      }
      if (this.aggiungi({ codice, rev: rev.value.trim().toUpperCase(), tipo: tipoSel.value, padre: padreSel.value, qta: q, diverso }) !== false) {
        const nuovo = this.bozza.aggiunti[this.bozza.aggiunti.length - 1];
        Dialogo.chiudi(nuovo ? nuovo.id : this.radice);
      }
    };
    Dialogo.apri(codiceDato ? `Rimetti ${codiceDato} nell'albero` : "Aggiungi un pezzo", [
      el("p", { class: "k" }, codiceDato ? "Il componente c'è già nella richiesta: torna nell'albero sotto il padre che scegli, con la conferma." :
        "Il pezzo nasce solo con la conferma dell'albero. Il codice è quello del cliente; per un assieme interno c'è il codice interno."),
      el("form", { class: "dst-form-nuovo", autocomplete: "off", onsubmit: (ev) => { ev.preventDefault(); fai(); } },
        el("label", { class: "dst-campo" }, el("span", {}, "Codice"), cod, interno),
        el("label", { class: "dst-campo" }, el("span", {}, "Tipo"), tipoSel),
        el("label", { class: "dst-campo" }, el("span", {}, "Sotto"), padreSel, el("small", {}, "Solo il prodotto o un assieme")),
        el("label", { class: "dst-campo" }, el("span", {}, "Quantità"), qta),
        el("label", { class: "dst-campo" }, el("span", {}, "Revisione"), rev)),
      vicini, errore,
    ], [el("button", { type: "button", class: "dst-btn primario", onclick: fai }, "Aggiungi"),
      el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Annulla")], { fuoco: codiceDato ? padreSel : cod });
  }

  dialogoSposta(k, padre, anche) {
    if (!this.proteggi()) return;
    const dove = [...this.vista.raggiunti].filter((r) => !this.vietato(k, r));
    if (!dove.length) { avvisa(`Non c'è un assieme dove ${this.nome(k)} possa andare.`, true); return; }
    const sel = el("select", {}, dove.sort((a, b) => this.nome(a).localeCompare(this.nome(b))).map((r) => el("option", { value: r }, (TIPI[this.tipoDi(r)] || "Pezzo") + " " + this.nome(r))));
    const qta = el("input", { type: "number", min: "1", max: "100000", value: "1" });
    const fai = () => {
      const ok = anche ? this.legame(k, sel.value, parseInt(qta.value, 10) || 1) : this.sposta(padre, k, sel.value);
      if (ok !== false) Dialogo.chiudi(k);
    };
    Dialogo.apri(anche ? `${this.nome(k)} anche sotto…` : `Sposta ${this.nome(k)}${padre ? " da sotto " + this.nome(padre) : ""}`, [
      el("form", { class: "dst-form-nuovo", onsubmit: (e) => { e.preventDefault(); fai(); } },
        el("label", { class: "dst-campo" }, el("span", {}, anche ? "Anche sotto" : "Sotto"), sel),
        anche ? el("label", { class: "dst-campo" }, el("span", {}, "Quantità"), qta) : null),
    ], [el("button", { type: "button", class: "dst-btn primario", onclick: fai }, anche ? "Metti anche lì" : "Sposta"),
      el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi(k) }, "Annulla")], { fuoco: sel, ritorno: k });
  }

  // riapri un pezzo scartato da una persona (la rotta di «Riapri il nodo»): e' un gesto che scrive subito, e cambia
  // l'albero; con una bozza aperta no
  async riapri(k) {
    if (!this.scrive) return;
    if (this.cambiato()) { avvisa("Prima conferma o scarta la bozza: riaprire un pezzo cambia l'albero proposto.", true); return; }
    const n = this.nodi.get(k);
    const righe = ((n && n.righe) || []).filter((r) => r.stato === "scartata");
    if (!righe.length) { avvisa("Nessuna riga scartata da riaprire per " + this.nome(k) + ".", true); return; }
    S.fuocoDopo = k;
    for (let i = 0; i < righe.length; i++) {
      const es = await gesto(S.dati.base + "/nodo/" + righe[i].proposta + "/riapri", {}, { swap: i === righe.length - 1 });
      if (!es.ok) { avvisa(es.testo || "Il pezzo non si è potuto riaprire.", true); return; }
      if (i === righe.length - 1) avvisa(es.testo || "Pezzo riaperto.");
    }
  }

  // elimina un componente fuori dalla distinta (la rotta di sempre): scrive subito, con la conferma scritta
  eliminaFuori(f, da) {
    if (!this.scrive) return;
    if (this.cambiato()) { avvisa("Prima conferma o scarta la bozza: eliminare un pezzo cambia l'albero proposto.", true); return; }
    const annulla = el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Annulla");
    Dialogo.apri(`Eliminare ${f.codice}?`, [
      el("p", {}, `${f.codice} non sta sotto nessun prodotto. Eliminarlo definitivamente? Non si torna indietro.`),
      el("p", { class: "k" }, "Riesce solo se il pezzo non ha storia (documenti, note, proposte decise): se ce l'ha, si archivia dal Fascicolo completo."),
    ], [el("button", { type: "button", class: "dst-btn pericolo", onclick: async () => {
      Dialogo.chiudi();
      const es = await gesto(S.dati.base + "/componente/" + f.id + "/rimuovi", {});
      if (!es.ok) avvisa(es.testo || "Il pezzo non si è potuto eliminare: se ha dei documenti si archivia dal Fascicolo completo.", true);
    } }, "Sì, elimina definitivamente"), annulla], { fuoco: annulla, da });
  }

  // ---- il riepilogo e la conferma

  async rivedi(da) {
    if (!this.proteggi()) return;
    Dialogo.apri("Rivedi e conferma l'albero", [el("p", { class: "k" }, "Lettura del riepilogo…")], [], { largo: true, da });
    let r, j;
    try {
      r = await fetch(S.dati.pagina + "/albero/riepilogo", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(this.bozza) });
      j = await r.json();
    } catch (e) {
      this.erroreRiepilogo("Il riepilogo non si è potuto leggere: controlla la rete e riprova.");
      return;
    }
    if (r.status === 422) { this.erroreRiepilogo("La bozza non si legge: " + (j.errore || "") + ". Correggila nell'albero (o annulla l'ultima modifica)."); return; }
    if (!r.ok) { this.erroreRiepilogo("Il riepilogo non si è potuto leggere (" + r.status + "): riprova."); return; }
    this.riepilogo = j;
    try {
      this.disegnaRiepilogo(j);
    } catch (e) {
      // un riepilogo che non si disegna non lascia la finestra ferma su «Lettura…»: si dice, e niente si conferma
      this.erroreRiepilogo("Il riepilogo non si è potuto mostrare: ricarica la pagina (la bozza resta in questo browser).");
      throw e;
    }
  }
  erroreRiepilogo(testo) {
    Dialogo.apri("Rivedi e conferma l'albero", [el("p", { class: "bad-t", role: "alert" }, testo)],
      [el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Torna all'albero")], { largo: true });
  }
  disegnaRiepilogo(r) {
    const sez = (titolo, lista, riga, classe) => lista && lista.length ? el("section", { class: "dst-riep-sezione" + (classe ? " " + classe : "") },
      el("h3", {}, titolo + " (" + lista.length + ")"), el("ul", { class: "dst-elenco" }, lista.map((x) => el("li", {}, riga(x))))) : null;
    const mono = (t) => el("span", { class: "mono" }, t);
    const vai = (k) => k ? el("button", { type: "button", class: "dst-btn piccolo", onclick: () => { this.sel = k; this.disegna(); Dialogo.chiudi(k); } }, "Vai al pezzo") : null;
    const nodoDi = (codice) => { for (const p of this.vista.pezzi.values()) if (p.codice === codice) return p.k; return ""; };
    const statoComm = { si: "✓ particolare commerciale", no: "✗ non è minuteria", aperta: "senza risposta", decaduta: "non vale più (il pezzo è cambiato)" };
    const corpo = [];
    if (r.vecchia) corpo.push(el("p", { class: "bad-t" }, "L'albero è cambiato da quando la bozza è stata disegnata: rivedila sull'albero di adesso."));
    if (r.blocchi.length) {
      corpo.push(el("div", { class: "dst-conferma-box", role: "alert" }, el("b", {}, "Prima della conferma, " + conta(r.blocchi.length, "domanda aperta:", "domande aperte:")),
        el("ul", { class: "dst-elenco" }, r.blocchi.map((b) => el("li", {}, b)))));
    } else {
      corpo.push(el("p", { class: "ok-t" }, "Nessuna domanda aperta: l'albero si può confermare."));
    }
    corpo.push(
      sez("Pezzi che nascono", r.nuovi, (x) => [mono(x.codice), x.rev ? " rev " + x.rev : "", " · ", TIPI[x.tipo] || x.tipo, x.commerciale ? " · commerciale (✓)" : "",
        el("span", { class: "k" }, " · codice " + ({ step: "dallo STEP", operatore: "scritto sulla riga dello STEP", scritto: "scritto nella bozza", richiesta: "della richiesta" }[x.fonte] || x.fonte) +
          (x.origine ? " (" + x.origine + ")" : "") + ((x.padri || []).length ? " · sotto " + x.padri.join(", ") : ""))]),
      sez("Ritrovati per codice: il componente c'è già", r.ritrovati, (x) => [mono(x.codice), el("span", { class: "k" }, " · " + x.perche + (x.archiviato ? " · è archiviato: la conferma lo ripristina" : ""))]),
      sez("Tipi che cambiano", r.tipi, (x) => [mono(x.codice), ": " + (TIPI[x.da] || x.da) + " → " + (TIPI[x.a] || x.a), el("span", { class: "k" }, " · " + x.motivo),
        x.spento ? el("span", { class: "bad-t" }, " · " + x.spento) : null, (x.effetti || []).length ? el("span", { class: "k" }, " · " + x.effetti.join(" ")) : null]),
      sez("Revisioni", r.revisioni, (x) => [mono(x.codice), x.discordi && x.discordi.length ? ": i file dicono revisioni diverse (" + x.discordi.join(", ") + "): resta " + (x.da || "senza revisione") : ": rev " + (x.da || "—") + " → " + x.a]),
      sez("Legami nuovi", r.legami_nuovi, (x) => [mono(x.padre), " → ", mono(x.figlio), " × " + (x.qta || "?"), el("span", { class: "k" }, (x.file && x.file.length ? " · da " + x.file.join(", ") : "") + (x.motivo ? " · " + x.motivo : ""))]),
      sez("Legami tolti", r.legami_tolti, (x) => [mono(x.padre), " → ", mono(x.figlio), " × " + x.qta, el("span", { class: "k" }, " · " + x.motivo)]),
      sez("Quantità che cambiano", r.quantita, (x) => [mono(x.padre), " → ", mono(x.figlio), ": " + x.prima + " → " + x.qta]),
      sez("La cascata dei pezzi tolti", r.cascata, (x) => [mono(x.codice), x.padre ? " da sotto " + x.padre : " da tutti i padri",
        el("span", { class: "k" }, " · vanno via: " + ((x.vanno || []).length ? x.vanno.join(", ") : "nessuno") + ((x.restano || []).length ? " · restano: " + x.restano.map((y) => y.codice + " (sotto " + (y.padri || []).join(", ") + ")").join(", ") : ""))]),
      sez("Componenti che restano senza padri", r.fuori, (x) => [mono(x.codice), x.esito === "si_archivia" ? ": si archivia" : ": si elimina", el("span", { class: "k" }, " · " + (x.perche || []).join(", "))]),
      sez("Righe degli STEP che si chiudono (scartate)", r.scartate, (x) => [mono(x.codice), el("span", { class: "k" }, " · " + conta(x.righe, "riga", "righe"))]),
      sez("Rimozioni proposte dallo STEP", r.rimozioni, (x) => [mono(x.padre), " → ", mono(x.figlio), x.stato === "tolta" ? ": accettata, il legame va via" : ": il legame resta"]),
      sez("Proposte di minuteria", r.commerciali, (x) => [mono(x.codice), ": " + (statoComm[x.stato] || x.stato) + (x.tendina ? " (con la tendina del tipo)" : ""), el("span", { class: "k" }, " · " + x.motivo), x.stato === "aperta" ? vai(x.nodo) : null],
        "da-guardare"),
      sez("Codici quasi uguali", r.vicini, (x) => [mono(x.codice), " è quasi uguale a " + (x.vicini || []).map((v) => v.codice).join(", "), x.diverso ? el("span", { class: "k" }, " · detto diverso da una persona") : el("span", { class: "warn-t" }, " · lo stesso pezzo o un pezzo diverso?"),
        // la risposta di una persona (P4), anche da qui: il riepilogo si rilegge con la bozza nuova. Se e' lo stesso
        // pezzo, lo si rinomina dall'albero («Vai al pezzo»)
        x.diverso ? null : el("button", { type: "button", class: "dst-btn piccolo", onclick: async () => { if (this.diverso(x.nodo, true) !== false) await this.rivedi(); } }, "È un pezzo diverso"),
        x.diverso ? null : vai(x.nodo)]),
      sez("Quantità discordi fra i file", r.quantita_discordi, (x) => [mono(x.padre), " → ", mono(x.figlio), ": " + ((x.fonti || []).map((f) => f.file + " ×" + f.qta).join(", ") || "i file dicono quantità diverse"),
        x.scelta ? " · scelta: " + x.scelta : el("span", { class: "warn-t" }, " · scegline una"), x.scelta ? null : vai(nodoDi(x.figlio))]),
      sez("Pezzi senza codice", r.senza_codice, (x) => [x, " ", vai(nodoDi(""))]));
    if (!r.nuovi.length && !r.ritrovati.length && !r.legami_nuovi.length && !r.legami_tolti.length && !r.tipi.length && !r.quantita.length && !r.cascata.length && !r.rimozioni.length) {
      corpo.push(el("p", { class: "k" }, "La conferma non cambia niente della distinta."));
    }
    const conferma = el("button", { type: "button", class: "dst-btn primario", disabled: !r.confermabile,
      title: r.confermabile ? null : "Si conferma quando non ci sono più domande aperte", onclick: () => this.conferma(conferma) }, "Conferma l'albero");
    const perche = r.confermabile ? null : el("span", { class: "k" }, "«Conferma l'albero» è spento finché ci sono domande aperte.");
    Dialogo.apri("Rivedi e conferma l'albero", corpo, [conferma, perche, el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Torna all'albero")],
      { largo: true, fuoco: r.confermabile ? conferma : $(".dst-dialogo-azioni .dst-btn:last-child", Dialogo.el()) });
  }
  async conferma(bottone) {
    if (!this.riepilogo || this.inConferma) return;
    this.inConferma = true;
    bottone.disabled = true;
    bottone.textContent = "Conferma in corso…";
    let r, j;
    try {
      r = await fetch(S.dati.pagina + "/albero/conferma", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify({ firma: this.riepilogo.firma, bozza: this.bozza }) });
      j = await r.json().catch(() => ({}));
    } catch (e) {
      r = null;
    }
    this.inConferma = false;
    if (r && r.ok) {
      Memoria.togli();
      this.bozza = bozzaVuota(this.a.firma);
      this.storia = [];
      S.fuocoDopo = this.radice;
      Dialogo.chiudi();
      await ricaricaCorpo();
      avvisa(j.testo || "Albero confermato.");
      return;
    }
    const testo = !r ? "Il server non ha risposto: niente è cambiato. Riprova." : r.status === 403 ? "Non autorizzato: chi consulta non conferma l'albero." :
      "Niente è cambiato: " + ((j && j.errore) || "la conferma non è riuscita (" + r.status + ")") + ".";
    Dialogo.apri("Rivedi e conferma l'albero", [el("p", { class: "bad-t", role: "alert" }, testo)],
      [el("button", { type: "button", class: "dst-btn primario", onclick: () => this.rivedi() }, "Rileggi il riepilogo"),
        el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Torna all'albero")], { largo: true });
  }
  scartaBozza(da) {
    if (!this.scrive || !this.cambiato()) return;
    const annulla = el("button", { type: "button", class: "dst-btn", onclick: () => Dialogo.chiudi() }, "Annulla");
    Dialogo.apri("Scartare la bozza?", [el("p", {}, `Le ${conta(vociBozza(this.bozza), "modifica", "modifiche")} della bozza vanno via, e l'albero torna quello proposto. Niente era stato scritto.`)],
      [el("button", { type: "button", class: "dst-btn pericolo", onclick: () => {
        this.storia = [];
        this.bozza = bozzaVuota(this.a.firma);
        Dialogo.chiudi(this.radice);
        this.dopo("Bozza scartata: l'albero è quello proposto.", this.radice);
      } }, "Sì, scartala"), annulla], { fuoco: annulla, da });
  }

  // ---- il menu del tasto destro (lo stesso da tastiera)

  vociMenu(k, padre) {
    const voci = [];
    const p = this.pezzo(k);
    const n = p ? p.n : this.nodi.get(k);
    const pdf = this.pdfDi(k);
    if (pdf.length) voci.push({ testo: "Apri il disegno", fai: () => apriVisore(pdf[0].a, pdf[0].comp || "", this.bottoneDi(k), pdf[0].comp ? "" : this.nome(k)) });
    if (!p) {
      if (this.scrive && n && (n.righe || []).some((r) => r.stato === "scartata")) voci.push({ testo: "Riapri il pezzo scartato", fai: () => this.riapri(k), spento: this.cambiato() ? "prima conferma o scarta la bozza" : "" });
      return voci;
    }
    if (!this.scrive) return voci;
    if (voci.length) voci.push("-");
    if (this.rinominabile(k)) voci.push({ testo: p.codice ? "Rinomina il codice…" : "Scrivi il codice…", fai: () => this.dialogoRinomina(k) });
    if (this.contenitore(k)) voci.push({ testo: "Aggiungi un pezzo sotto…", fai: () => this.dialogoAggiungi("sciolto", k) });
    if (!this.prodotto(k)) {
      if (padre) voci.push({ testo: "Sposta sotto un altro assieme…", fai: () => this.dialogoSposta(k, padre, false) });
      voci.push({ testo: "Anche sotto un altro assieme…", fai: () => this.dialogoSposta(k, "", true) });
      voci.push("-");
      const figli = (this.vista.figli.get(k) || []).length;
      for (const t of ["sottoassieme", "sciolto", "commerciale"]) {
        if (t === this.tipoDi(k)) continue;
        voci.push({ testo: "È " + TIPI_FRASE[t], fai: () => this.tipo(k, t), spento: t !== "sottoassieme" && figli ? "ha dei pezzi sotto" : "" });
      }
      if (n && n.commerciale) {
        voci.push({ testo: (p.risposta === "si" ? "Togli il ✓" : "✓ È minuteria: particolare commerciale"), fai: () => this.risposta(k, "si") });
        voci.push({ testo: (p.risposta === "no" ? "Togli il ✗" : "✗ Non è minuteria"), fai: () => this.risposta(k, "no") });
      }
      if (n && n.vicini && n.vicini.length) voci.push({ testo: p.diverso ? "Non è più detto diverso dai codici quasi uguali" : "È un pezzo diverso da " + n.vicini.map((v) => v.codice).join(", "), fai: () => this.diverso(k, !p.diverso) });
      voci.push("-");
      const padri = this.padriDi(k);
      if (padre && padri.length > 1) voci.push({ testo: "Togli da sotto " + this.nome(padre) + "…", fai: () => this.dialogoTogli(k, padre), pericolo: true });
      voci.push({ testo: "Togli dall'albero…", fai: () => this.dialogoTogli(k, ""), pericolo: true });
    }
    return voci;
  }
  apriMenu(k, padre, x, y, da) {
    this.sel = k;
    this.selPadre = padre || "";
    this.disegnaDettaglio();
    for (const c of $$("#dst-tree .dst-nodo.sel")) c.classList.remove("sel");
    for (const c of $$(`#dst-tree .dst-nodo[data-chiave="${cssId(k)}"]`)) c.classList.add("sel");
    const voci = this.vociMenu(k, padre);
    if (!voci.length) { avvisa("Per " + this.nome(k) + " qui non ci sono comandi.", true); return; }
    Menu.apri(voci, x, y, this.bottoneDi(k, padre) || da);
  }

  // ---- il disegno

  bottoneDi(k, padre) {
    const tutti = $$(`#dst-tree .cod[data-chiave="${cssId(k)}"]`);
    return tutti.find((b) => (b.dataset.padre || "") === (padre || "")) || tutti[0] || null;
  }
  // fuocoSu mette il fuoco sul pezzo k dell'albero (sulla sua casella piena, se ce n'e' piu' d'una): true se c'e'
  fuocoSu(k) {
    const b = $$(`#dst-tree .dst-nodo:not(.rimando) .cod[data-chiave="${cssId(k || "")}"]`)[0] || this.bottoneDi(k);
    if (!b) return false;
    b.focus();
    return true;
  }
  pdfDi(k) {
    const p = this.pezzo(k);
    const n = p ? p.n : this.nodi.get(k);
    const comp = n && n.componente ? n.componente : "";
    // il codice che dicono i file e' quello proposto, anche se nella bozza il pezzo e' rinominato
    const lista = comp ? pdfDelComponente(comp) : pdfDelCodice((n && n.codice) || (p && p.codice));
    return lista.filter((x) => x.ok);
  }

  disegna(messaggio, fuoco) {
    if (!document.getElementById("dst-tree")) return;
    const att = document.activeElement;
    const eraQui = att && att.closest && (att.closest("#dst-tree") || att.closest("#dst-dettaglio") || att.closest("#dst-vassoio") || att.closest("#dst-ripresa"));
    const chiaveFuoco = eraQui && att.dataset ? att.dataset.fuoco : "";
    this.disegnaProdotti();
    this.disegnaAnalisi();
    this.disegnaRipresa();
    this.disegnaAlbero();
    this.disegnaDettaglio();
    this.disegnaVassoio();
    this.disegnaSalva();
    miniature();
    if (messaggio) avvisa(messaggio);
    // il fuoco torna dov'era: lo stesso comando, o il pezzo su cui si e' agito
    const d = document.getElementById("dst-dialogo");
    if (d && d.open) return;
    if (chiaveFuoco) {
      const x = document.querySelector(`[data-fuoco="${cssId(chiaveFuoco)}"]`);
      if (x && !x.disabled) { x.focus(); return; }
    }
    if (fuoco || eraQui) this.fuocoSu(fuoco || this.sel);
  }

  disegnaProdotti() {
    const posto = document.getElementById("dst-prodotto-scelta");
    if (!posto) return;
    const pp = this.prodotti;
    if (pp.length < 2) { posto.replaceChildren(); return; }
    const sel = el("select", { "aria-label": "Prodotto", onchange: (e) => {
      // la bozza e' della RFQ intera: cambiare prodotto non la perde
      this.radice = e.target.value;
      this.sel = this.radice;
      prova(() => sessionStorage.setItem(this.chiaveProdotto, this.radice));
      this.disegna("", this.radice);
    } }, pp.map((p) => el("option", { value: p.chiave, selected: p.chiave === this.radice }, p.codice)));
    posto.replaceChildren(el("span", { class: "k" }, "Prodotto: "), sel);
  }

  disegnaAnalisi() {
    const corpo = document.getElementById("dst-analisi-corpo");
    const chip = document.getElementById("dst-analisi-chip");
    if (!corpo) return;
    const parti = [];
    if (S.dati.analisi > 0) parti.push(el("p", { class: "warn-t" }, `${S.dati.analisi} analisi ancora in corso sui file della richiesta: la proposta può cambiare.`));
    if (!this.prodotti.length) parti.push(el("p", { class: "k" }, "Non c'è ancora un prodotto: i codici della richiesta, confermati nella Inbox, diventano prodotti. Poi qui si propone l'albero."));
    const livelli = { autorizzata: "dallo STEP autorizzato del prodotto", piena: "dallo STEP del prodotto (la radice è il suo codice)", da_confermare: "da uno STEP che sembra del prodotto (da confermare)" };
    for (const p of this.prodotti) {
      if (p.solo_prodotto) {
        // senza STEP del prodotto l'albero ha solo il prodotto: la frase, il caricamento dei file e l'aggiunta a mano
        parti.push(el("div", { class: "dst-conferma-box warn" },
          el("span", {}, el("b", { class: "mono" }, p.codice), ": " + (p.perche || "nessuno STEP del prodotto") + ". L'albero ha solo il prodotto."),
          el("span", {}, S.dati.caricamento ? ["I file si caricano nel ", el("a", { href: S.dati.base }, "Fascicolo completo"), " (+ Aggiungi file › Carica dal PC); poi l'albero si propone da solo."]
            : "Il caricamento dei file non è disponibile su questo server: i file arrivano con la posta."),
          this.scrive ? el("span", { class: "k" }, "Oppure costruisci l'albero a mano, con «+ Assieme» e «+ Particolare».") : null));
        continue;
      }
      parti.push(el("p", {}, el("b", { class: "mono" }, p.codice), ": l'albero viene " + (livelli[p.ancora] || "dallo STEP del prodotto") + (p.file && p.file.length ? " (" + p.file.join(", ") + ")" : "") + ", a tutti i livelli, con gli STEP degli assiemi che si trovano per codice."),
        (p.discordanze || []).length ? el("p", { class: "warn-t" }, "Da guardare: " + p.discordanze.join("; ")) : null);
    }
    const senza = this.a.senza_posto || [];
    if (senza.length) {
      parti.push(el("div", {}, el("span", { class: "dst-label" }, `File che non stanno nell'albero (${senza.length})`),
        el("ul", { class: "dst-elenco" }, senza.map((f) => el("li", {}, el("span", { class: "mono" }, f.file), (f.radici && f.radici.length ? " (radice " + f.radici.join(", ") + ")" : ""), el("span", { class: "k" }, " · " + f.motivo))))));
    }
    corpo.replaceChildren(...parti.filter(Boolean));
    const proposti = this.daConfermare();
    const domande = [...this.vista.pezzi.values()].filter((p) => p.n && p.n.commerciale && !p.risposta && this.vista.raggiunti.has(p.k)).length;
    if (chip) {
      chip.replaceChildren(proposti ? el("span", { class: "dst-chip warn" }, `${conta(proposti, "pezzo", "pezzi")} da confermare`) : el("span", { class: "dst-chip ok" }, "niente da confermare"),
        domande ? el("span", { class: "dst-chip warn" }, `${conta(domande, "domanda", "domande")} sulla minuteria`) : "");
    }
    const box = document.getElementById("dst-analisi");
    if (box) box.className = "dst-box" + (proposti || this.prodotti.some((p) => p.solo_prodotto) ? " tono-warn" : " tono-2");
  }

  // la bozza trovata in questo browser: si riprende o si scarta. Con l'albero cambiato (una rianalisi, la conferma di un
  // collega) e' vecchia, e non si applica da sola
  disegnaRipresa() {
    const box = document.getElementById("dst-ripresa");
    if (!box) return;
    const t = this.trovata;
    if (!t) { box.hidden = true; box.replaceChildren(); return; }
    const vecchia = t.bozza.base !== this.a.firma;
    const n = vociBozza(t.bozza);
    box.hidden = false;
    box.dataset.stato = vecchia ? "vecchia" : "uguale";
    box.replaceChildren(
      el("span", { class: "dst-label" }, vecchia ? "Bozza vecchia" : "Bozza non confermata"),
      el("p", {}, vecchia
        ? `C'è una bozza delle ${ora(t.quando)} (${conta(n, "modifica", "modifiche")}), disegnata su un albero che nel frattempo è cambiato: è vecchia, e non si applica da sola.`
        : `C'è una bozza non confermata delle ${ora(t.quando)} (${conta(n, "modifica", "modifiche")}): riprendila o scartala.`),
      el("span", { class: "dst-azioni" },
        el("button", { type: "button", class: "dst-btn primario", "data-fuoco": "ripresa|si", onclick: () => this.riprendi() }, vecchia ? "Riprendi quello che vale ancora" : "Riprendila"),
        el("button", { type: "button", class: "dst-btn", "data-fuoco": "ripresa|no", onclick: () => this.scartaTrovata() }, "Scartala")));
  }

  // la casella di un pezzo. Non e' un bottone: dentro ha dei bottoni (il codice, che la sceglie; la miniatura; il ✓ e
  // il ✗ della minuteria)
  casella(k, padre, legame, modo) {
    const p = this.pezzo(k);
    const n = p ? p.n : this.nodi.get(k);
    const tipo = modo === "scartato" ? (n && n.tipo) || "sciolto" : this.tipoDi(k) || "sciolto";
    const stato = modo === "scartato" ? "scartato" : this.stato(k);
    const classi = ["dst-nodo", tipo];
    if (stato === "proposto" || stato === "tolto_dallo_step") classi.push("proposto");
    if (stato === "nuovo") classi.push("nuovo");
    if (stato === "scartato") classi.push("scartato");
    if (k === this.sel && modo !== "rimando") classi.push("sel");
    if (modo === "rimando") classi.push("rimando");
    const tag = {
      proposto: ["warn", "proposto"], nuovo: ["acc", "aggiunto · nella bozza"], scartato: ["no", "scartato"], tolto_dallo_step: ["bad", "lo STEP lo toglie"],
    }[stato];
    const figli = [];
    figli.push(el("span", { class: "dst-nodo-testa" }, el("span", { class: "dst-tipo " + tipo }, TIPI[tipo] || "Da decidere"),
      tag ? el("span", { class: "dst-stato-tag " + tag[0] }, tag[1]) : null));
    const etichetta = (TIPI[tipo] || "Pezzo") + " " + this.nome(k) + (tag ? ", " + tag[1] : "") + (padre ? ", sotto " + this.nome(padre) : "");
    const cod = el("button", {
      type: "button", class: "cod", "data-chiave": k, "data-padre": padre || "", "data-fuoco": "nodo|" + k + "|" + (padre || "") + (modo === "rimando" ? "|r" : ""),
      "aria-label": etichetta, title: "Clic: i dettagli · clic destro, tasto menu o Maiusc+F10: i comandi",
    }, this.nome(k));
    cod.addEventListener("click", () => { this.sel = k; this.selPadre = padre || ""; this.disegna("", k); });
    cod.addEventListener("keydown", (e) => {
      if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
        e.preventDefault();
        Menu.daTastiera = Date.now();
        const r = cod.getBoundingClientRect();
        this.apriMenu(k, padre, r.left, r.bottom + 2, cod);
      } else if (e.key === "Delete" && this.scrive && p && !this.prodotto(k)) {
        e.preventDefault();
        this.sel = k; this.selPadre = padre || "";
        this.dialogoTogli(k, "");
      }
    });
    figli.push(cod);
    if (modo === "rimando") {
      figli.push(el("span", { class: "k" }, "↗ lo stesso pezzo è già disegnato sopra"));
    } else {
      const sotto = [];
      if (p && p.rinominato && n && n.codice) sotto.push("era " + n.codice);
      else if (p && p.ag) sotto.push("scritto nella bozza");
      else if (n && n.codice && stato === "proposto") sotto.push(FONTI[n.fonte_codice] || "dallo STEP");
      if (n && n.codice_nel_file && !(p && p.rinominato)) sotto.push("nel file: " + n.codice_nel_file);
      if (p && p.rev) sotto.push("rev " + p.rev);
      if (sotto.length) figli.push(el("span", { class: "fonte" }, sotto.join(" · ")));
      if (n && n.nome && n.nome !== this.nome(k)) figli.push(el("span", { class: "nome" }, n.nome));
      const note = [];
      if (n && n.ritrovato && stato !== "scartato") note.push(el("span", { class: "dst-stato-tag acc" }, "ritrovato per codice: c'è già" + (n.ritrovato.archiviato ? " (archiviato)" : "")));
      if (n && n.vicini && n.vicini.length && !(p && p.diverso) && stato !== "scartato") note.push(el("span", { class: "dst-stato-tag warn" }, "quasi uguale a " + n.vicini.map((v) => v.codice).join(", ")));
      if (p && p.tipoScelto) note.push(el("span", { class: "dst-stato-tag acc" }, "tipo scelto nella bozza"));
      const altri = padre ? this.padriDi(k).filter((l) => l.padre !== padre) : [];
      if (altri.length) note.push(el("span", { class: "dst-stato-tag neu" }, "anche sotto " + altri.map((l) => this.nome(l.padre)).join(", ")));
      if (n && n.prodotti && n.prodotti.length > 1) note.push(el("span", { class: "dst-stato-tag neu" }, "in " + conta(n.prodotti.length, "prodotto", "prodotti") + ": " + n.prodotti.join(", ")));
      if (note.length) figli.push(el("span", { class: "tags" }, note));
      // la minuteria proposta: il ✓ o il ✗ dell'ingegnere, con il motivo in parole (domanda 30)
      if (n && n.commerciale && p && stato !== "scartato") {
        const r = p.risposta || "";
        figli.push(el("span", { class: "dst-minuteria" + (r ? " " + r : "") },
          el("span", { class: "m" }, r === "si" ? "minuteria: particolare commerciale" : r === "no" ? "non è minuteria" : "minuteria?"),
          this.scrive ? el("span", { class: "dst-gruppo", role: "group", "aria-label": "È minuteria? " + n.commerciale.motivo },
            el("button", { type: "button", "aria-pressed": r === "si" ? "true" : "false", title: "È un particolare commerciale", "data-fuoco": "si|" + k + "|" + (padre || ""), onclick: () => this.risposta(k, "si") }, "✓"),
            el("button", { type: "button", "aria-pressed": r === "no" ? "true" : "false", title: "Non è minuteria", "data-fuoco": "no|" + k + "|" + (padre || ""), onclick: () => this.risposta(k, "no") }, "✗")) : null,
          el("small", { class: "k" }, n.commerciale.motivo)));
      }
      if (stato !== "scartato") {
        const pdf = this.pdfDi(k);
        const comp = n && n.componente ? n.componente : "";
        if (pdf.length) {
          const nn = comp ? noteDelComponente(comp) : 0;
          const proposto = pdf[0].stato !== "doc";
          figli.push(el("button", { class: "dst-mini", type: "button", "data-a": pdf[0].a, title: "Apri il disegno " + pdf[0].nome, "aria-label": "Apri il disegno " + pdf[0].nome + (proposto ? ", proposto" : ""),
            onclick: (e) => { e.stopPropagation(); apriVisore(pdf[0].a, comp, e.currentTarget, comp ? "" : this.nome(k)); } },
          el("span", { class: "attesa" }, "…"), el("span", { class: "apri" }, "Apri il disegno"), proposto ? el("span", { class: "proposto" }, "proposto") : null,
          nn ? el("span", { class: "note-mini" }, "✎ " + nn) : null));
        } else {
          const testo = comp ? "Nessun disegno 2D" : stato === "nuovo" ? "Pezzo nuovo: il disegno si associa dopo la conferma" : "Nessun disegno con questo codice";
          figli.push(el("div", { class: "dst-mini vuota" }, el("span", {}, testo), tipo === "commerciale" ? el("small", {}, "per un particolare commerciale non serve") : null));
        }
        const celle = (comp && S.dati.celle && S.dati.celle[comp]) || [];
        if (celle.length) figli.push(el("span", { class: "docs" }, celle.map((x) => el("span", { class: "dst-cella " + x.c, title: x.e + ": " + x.t }, x.e + " " + (x.c === "coda" ? "✓ in coda" : x.s)))));
      }
    }
    const box = el("div", { class: classi.join(" "), "data-chiave": k, "data-padre": padre || "",
      draggable: this.scrive && padre && p && !this.prodotto(k) && modo !== "rimando" && modo !== "scartato" ? "true" : "false" }, figli);
    box.addEventListener("contextmenu", (e) => {
      e.preventDefault();
      // il tasto menu e Maiusc+F10 aprono il menu da keydown; il browser manda poi anche il suo «contextmenu»
      // (quello della tastiera non e' del tasto destro: button 0). Il tasto destro del mouse (button 2) apre sempre
      if (e.button !== 2 && (Menu.appenaAperto() || Menu.ecoDellaTastiera())) return;
      const r = cod.getBoundingClientRect();
      const x = e.clientX || r.left, y = e.clientY || r.bottom + 2;
      this.apriMenu(k, padre, x, y, cod);
    });
    if (this.scrive) {
      box.addEventListener("dragstart", (e) => { this.trascinato = { k, padre }; e.dataTransfer.setData("text/plain", k); e.dataTransfer.effectAllowed = "move"; });
      box.addEventListener("dragend", () => { this.trascinato = null; for (const x of $$(".dst-nodo.drop, .dst-nodo.no-drop")) x.classList.remove("drop", "no-drop"); });
      box.addEventListener("dragover", (e) => {
        const t = this.trascinato;
        if (!t || t.k === k || modo === "scartato") return;
        // anche sopra una casella vietata il rilascio si accetta: sposta() lo rifiuta e dice perche'
        e.preventDefault();
        box.classList.add(this.vietato(t.k, k) ? "no-drop" : "drop");
      });
      box.addEventListener("dragleave", () => box.classList.remove("drop", "no-drop"));
      box.addEventListener("drop", (e) => {
        e.preventDefault();
        box.classList.remove("drop", "no-drop");
        const t = this.trascinato;
        this.trascinato = null;
        if (t) this.sposta(t.padre, t.k, k);
      });
    }
    const out = el("div", { class: "dst-nodo-box" });
    if (legame) {
      const discordi = legame.discordi;
      out.append(el("span", { class: "dst-qta-arco" + (discordi ? " discordi" : ""), title: discordi ? "i file dicono quantità diverse: scegline una" : "quantità sotto " + this.nome(padre) },
        "×" + (discordi ? "?" : legame.qta)));
    }
    out.append(box);
    return out;
  }

  disegnaAlbero() {
    const tree = document.getElementById("dst-tree");
    if (!this.radice) {
      tree.replaceChildren(el("div", { class: "dst-box tono-2", style: "max-width:560px;margin:0 auto" },
        el("b", {}, "Non c'è ancora un prodotto."),
        el("p", { class: "k" }, "I codici della richiesta, confermati nella Inbox, diventano prodotti. Poi qui si propone l'albero.")));
      return;
    }
    const aperti = new Set();
    const giu = (k, padre, legame, modo, cammino) => {
      const ripetuto = aperti.has(k) || cammino.has(k);
      let classe = "";
      if (legame && (legame.scritto || (legame.arco && legame.arco.stato === "proposto"))) classe = "proposto-arco";
      if (legame && legame.arco && legame.arco.stato === "tolto_dallo_step") classe = "rimozione-arco";
      if (modo === "scartato") classe = "scartato-arco";
      const li = el("li", { class: classe }, this.casella(k, padre, legame, ripetuto && modo !== "scartato" ? "rimando" : modo));
      if (ripetuto || modo === "scartato") return li;
      aperti.add(k);
      const sotto = new Set(cammino).add(k);
      const figli = this.figliDi(k).map((l) => giu(l.figlio, k, l, "", sotto));
      // i pezzi scartati da una persona sotto questo pezzo: si vedono, grigi, e non si scende
      for (const l of this.a.archi || []) {
        const f = this.nodi.get(l.figlio);
        if (l.padre === k && l.stato === "scartato" && f && f.stato === "scartato") figli.push(giu(l.figlio, k, { qta: l.qta, arco: l }, "scartato", sotto));
      }
      if (figli.length) li.append(el("ul", {}, figli));
      return li;
    };
    tree.replaceChildren(el("ul", {}, giu(this.radice, "", null, "", new Set())));
    const indietro = $("[data-azione=indietro]");
    if (indietro) indietro.disabled = !this.storia.length;
  }

  disegnaDettaglio() {
    const d = document.getElementById("dst-dettaglio");
    if (!d) return;
    const k = this.pezzo(this.sel) || this.nodi.has(this.sel) ? this.sel : this.radice;
    if (!k) { d.replaceChildren(); return; }
    const p = this.pezzo(k);
    const n = p ? p.n : this.nodi.get(k);
    const tipo = this.tipoDi(k);
    const stato = p ? this.stato(k) : "scartato";
    const radice = this.prodotto(k);
    const comp = n && n.componente ? n.componente : "";
    const sin = el("div", { style: "display:grid;gap:10px;align-content:start" });
    const des = el("div", { style: "display:grid;gap:12px;align-content:start" });
    const campo = (et, input, aiuto) => el("label", { class: "dst-campo" }, el("span", {}, et), input, aiuto ? el("small", {}, aiuto) : null);
    const scrive = this.scrive && p && !p.tolto;

    // il codice, con la sua fonte
    const fonte = p && p.ag ? "scritto nella bozza" : p && p.rinominato ? "rinominato nella bozza (era " + (n.codice || "senza codice") + ")" : n && n.codice ? FONTI[n.fonte_codice] || "dallo STEP" : "lo STEP non dice un codice";
    sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Codice"),
      el("span", { class: "dst-azioni" }, el("b", { class: "mono", style: "font-size:1.1rem" }, this.nome(k)),
        scrive && this.rinominabile(k) ? el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "det|rinomina", onclick: () => this.dialogoRinomina(k) }, p.codice ? "Rinomina…" : "Scrivi il codice…") : null),
      el("small", {}, fonte + (n && n.codice_nel_file ? " · nel file: " + n.codice_nel_file : "") + (n && n.origine_codice ? " · " + n.origine_codice : "")),
      comp && !radice ? el("small", {}, "Un codice sbagliato di un componente che c'è già si corregge nel Fascicolo completo.") : null));
    if (p && p.rev) sin.append(campo("Revisione", el("span", { class: "mono" }, p.rev)));
    // la denominazione di un componente che c'e': un gesto al server, con la bozza vuota
    if (comp && stato === "nella_distinta") {
      const inp = el("input", { maxlength: "200", placeholder: "per esempio: Staffa laterale", "data-fuoco": "det|desc" });
      inp.value = n.descrizione || "";
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Denominazione"),
        el("span", { class: "dst-azioni" }, inp, this.scrive ? el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "det|desc-salva", onclick: async () => {
          if (this.cambiato()) { avvisa("Prima conferma o scarta la bozza: la denominazione va subito al server e rifà la pagina.", true); return; }
          const es = await gesto(S.dati.base + "/componente/" + comp + "/modifica", { rev: n.rev || "", descrizione: inp.value.trim() });
          if (!es.ok) avvisa(es.testo || "La denominazione non si è potuta salvare.", true);
        } }, "Salva") : null)));
    } else if (n && (n.descrizione || n.nome)) sin.append(campo("Nel file", el("span", {}, [n.nome, n.descrizione].filter(Boolean).join(" · "))));
    // il tipo
    if (!radice && p) {
      const sel = el("select", { disabled: !scrive, "data-fuoco": "det|tipo" }, ["sottoassieme", "sciolto", "commerciale"].map((t) => el("option", { value: t, selected: t === tipo }, TIPI[t])));
      sel.addEventListener("change", () => this.tipo(k, sel.value));
      sin.append(campo("Tipo", sel, (p.tipoScelto ? "scelto nella bozza" : n && n.tipo_motivo ? "proposto: " + n.tipo_motivo : "") +
        (n && n.tipo_attuale && n.tipo_attuale !== tipo ? " · oggi è " + (TIPI_FRASE[n.tipo_attuale] || n.tipo_attuale) : "")));
    }
    // la minuteria proposta
    if (n && n.commerciale && p) {
      const r = p.risposta || "";
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Minuteria proposta dal nome"),
        el("span", {}, n.commerciale.motivo + (n.commerciale.nome ? " (" + n.commerciale.nome + ")" : "")),
        scrive ? el("span", { class: "dst-azioni" },
          el("button", { type: "button", class: "dst-btn piccolo" + (r === "si" ? " primario" : ""), "aria-pressed": r === "si" ? "true" : "false", "data-fuoco": "det|si", onclick: () => this.risposta(k, "si") }, "✓ È un particolare commerciale"),
          el("button", { type: "button", class: "dst-btn piccolo" + (r === "no" ? " primario" : ""), "aria-pressed": r === "no" ? "true" : "false", "data-fuoco": "det|no", onclick: () => this.risposta(k, "no") }, "✗ Non è minuteria")) : null,
        el("small", {}, r ? "Risposta data: con la conferma resta scritto chi l'ha data." : "Senza una risposta, «Conferma l'albero» resta spento.")));
    }
    // i codici quasi uguali (P4): la risposta di una persona, mai spuntata da sola. Per un pezzo aggiunto li dice il
    // riepilogo (o la finestra dell'aggiunta)
    const viciniRiep = p && p.ag && this.riepilogo ? (this.riepilogo.vicini || []).find((x) => x.nodo === k) : null;
    if (p && p.ag && (viciniRiep || p.diverso)) {
      const spunta = el("input", { type: "checkbox", checked: !!p.diverso, disabled: !scrive, "data-fuoco": "det|diverso" });
      spunta.addEventListener("change", () => this.diverso(k, spunta.checked));
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Codici quasi uguali"),
        viciniRiep ? el("span", {}, viciniRiep.vicini.map((v) => v.codice + " (" + v.motivo + ")").join(", ")) : null,
        el("label", {}, spunta, ` ${this.nome(k)} è un pezzo diverso`)));
    }
    if (n && n.vicini && n.vicini.length && p && !p.tolto) {
      const spunta = el("input", { type: "checkbox", checked: !!p.diverso, disabled: !scrive, "data-fuoco": "det|diverso" });
      spunta.addEventListener("change", () => this.diverso(k, spunta.checked));
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Codici quasi uguali"),
        el("span", {}, n.vicini.map((v) => v.codice + " (" + v.motivo + ")").join(", ")),
        el("label", {}, spunta, ` ${this.nome(k)} è un pezzo diverso`),
        el("small", {}, "Se è lo stesso pezzo, rinominalo con quel codice.")));
    }
    // dove sta, con le quantita'
    if (!radice && p && this.vista.raggiunti.has(k)) {
      const righe = el("div", { style: "display:grid;gap:8px" });
      for (const l of this.padriDi(k)) {
        let q;
        if (l.discordi && l.arco) {
          q = el("select", { disabled: !scrive, "aria-label": "quantità sotto " + this.nome(l.padre), "data-fuoco": "det|qta|" + l.padre },
            el("option", { value: "" }, "— i file dicono quantità diverse —"),
            [...new Set((l.arco.fonti || []).filter((f) => !f.scartata).map((f) => f.qta))].map((x) => el("option", { value: x }, x + " (" + (l.arco.fonti || []).filter((f) => f.qta === x).map((f) => f.file).join(", ") + ")")));
          q.addEventListener("change", () => { if (q.value) this.quantita(l.padre, k, q.value); });
        } else {
          q = el("input", { type: "number", min: "1", max: "100000", value: l.qta, style: "width:6rem", "aria-label": "quantità sotto " + this.nome(l.padre), disabled: !scrive, "data-fuoco": "det|qta|" + l.padre });
          q.addEventListener("change", () => this.quantita(l.padre, k, q.value));
        }
        righe.append(el("div", { class: "dst-azioni" }, el("span", {}, "sotto ", el("b", { class: "mono" }, this.nome(l.padre)), " ×"), q,
          l.arco && l.arco.stato === "tolto_dallo_step" ? el("span", { class: "bad-t" }, "lo STEP lo toglie") : null,
          scrive ? el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "det|togli|" + l.padre, onclick: () => this.dialogoTogli(k, l.padre) }, l.arco && l.arco.stato === "tolto_dallo_step" ? "Togli (come dice lo STEP)" : "Togli da qui") : null));
      }
      if (scrive) {
        const padre = this.selPadre && this.vista.legami.has(this.selPadre + "|" + k) ? this.selPadre : (this.padriDi(k)[0] || {}).padre;
        righe.append(el("span", { class: "dst-azioni" },
          padre ? el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "det|sposta", onclick: () => this.dialogoSposta(k, padre, false) }, "Sposta sotto…") : null,
          el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "det|anche", onclick: () => this.dialogoSposta(k, "", true) }, "Anche sotto…")));
      }
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Dove sta nell'albero"), righe));
    }

    // da dove viene
    const fonti = [];
    if (radice) fonti.push(["acc", "Richiesta", "è il prodotto della richiesta"]);
    if (stato === "nella_distinta" && !radice) fonti.push(["ok", "Distinta", "pezzo già nella distinta"]);
    if (stato === "nuovo") fonti.push(["acc", "Bozza", "aggiunto adesso: nasce con la conferma dell'albero"]);
    if (stato === "scartato") fonti.push(["no", "Scartato", "una persona ha scartato le sue righe dello STEP: si vede, non si propone"]);
    if (n && n.ritrovato) fonti.push(["acc", "Ritrovato", `c'è già il componente ${n.ritrovato.codice} (${TIPI_FRASE[n.ritrovato.tipo] || n.ritrovato.tipo})` + (n.ritrovato.archiviato ? ", archiviato: la conferma lo ripristina" : "") + (n.ritrovato.agganciato ? "; agganciato per codice prima dello Smistamento, senza una persona" : "")]);
    for (const r of (n && n.righe) || []) fonti.push([r.stato === "scartata" ? "no" : "warn", "STEP", r.file + (r.rev ? " · rev " + r.rev : "") + " · " + r.stato]);
    if (n && n.step) fonti.push(["neu", "Suo STEP", n.step.file.join(", ") + " (" + n.step.livello + "): i suoi figli vengono anche da lì"]);
    for (const x of (n && n.note) || []) fonti.push(["neu", "Nota", x]);
    for (const x of (n && n.discordanze) || []) fonti.push(["warn", "Da guardare", x]);
    const pdf = comp ? pdfDelComponente(comp) : pdfDelCodice((n && n.codice) || (p && p.codice));
    for (const x of pdf.slice(0, 3)) fonti.push(["neu", "Disegno", x.nome + (x.stato === "doc" ? " · confermato" : " · proposto, da confermare")]);
    if (fonti.length) des.append(el("div", {}, el("span", { class: "dst-label" }, "Da dove viene"),
      el("ul", { class: "dst-pulito dst-fonti", style: "margin-top:6px" }, fonti.map(([cl, a, b]) => el("li", {}, el("span", { class: "dst-chip " + cl }, a), el("span", {}, b))))));

    // le azioni
    const az = el("div", { class: "dst-azioni" });
    const disponibili = pdf.filter((x) => x.ok);
    if (disponibili.length) az.append(el("button", { type: "button", class: "dst-btn", "data-fuoco": "det|disegno", onclick: (e) => apriVisore(disponibili[0].a, comp, e.currentTarget, comp ? "" : this.nome(k)) }, "Apri il disegno" + (comp && noteDelComponente(comp) ? " (✎ " + noteDelComponente(comp) + ")" : "")));
    if (scrive && this.contenitore(k)) az.append(el("button", { type: "button", class: "dst-btn", "data-fuoco": "det|aggiungi", onclick: () => this.dialogoAggiungi("sciolto", k) }, "+ Un pezzo sotto…"));
    if (scrive && !radice) az.append(el("button", { type: "button", class: "dst-btn pericolo", "data-fuoco": "det|toglitutto", onclick: () => this.dialogoTogli(k, "") }, stato === "nuovo" ? "Non crearlo" : "Togli dall'albero…"));
    if (this.scrive && stato === "scartato") az.append(el("button", { type: "button", class: "dst-btn", "data-fuoco": "det|riapri", onclick: () => this.riapri(k) }, "Riapri il pezzo"));
    if (az.childNodes.length) des.append(az);

    d.replaceChildren(
      el("div", { class: "dst-riga-testa" }, el("span", { class: "dst-label" }, radice ? "Il prodotto" : { proposto: "Pezzo proposto", nuovo: "Pezzo aggiunto nella bozza", scartato: "Pezzo scartato", tolto_dallo_step: "Pezzo che lo STEP toglie" }[stato] || "Pezzo nella distinta"),
        el("span", { class: "dst-tipo " + (tipo || "sciolto") }, TIPI[tipo] || "Da decidere"), el("b", { class: "mono" }, this.nome(k)),
        el("span", { class: "sp" }), el("span", { class: "k" }, "clic su un pezzo per sceglierlo · clic destro per i comandi")),
      el("div", { class: "dst-dettaglio-griglia" }, sin, des));
    d.className = "dst-box" + (stato === "proposto" || stato === "tolto_dallo_step" ? " tono-warn" : "");
  }

  // il vassoio: i pezzi tolti nella bozza (con quello che va via a cascata) e i componenti fuori dalla distinta
  disegnaVassoio() {
    const v = document.getElementById("dst-vassoio");
    if (!v) return;
    const parti = [];
    const tolti = this.bozza.tolti;
    if (tolti.length) {
      parti.push(el("span", { class: "dst-label" }, `Tolti nella bozza (${tolti.length})`));
      const carte = tolti.map((t) => {
        // che cosa va via a cascata per questo «togli»: quello che senza di lui si raggiungerebbe ancora
        const senza = JSON.parse(JSON.stringify(this.bozza));
        senza.tolti = senza.tolti.filter((x) => !(x.nodo === t.nodo && (x.padre || "") === (t.padre || "")));
        const cascata = [...this.calcola(senza).raggiunti].filter((x) => x !== t.nodo && !this.vista.raggiunti.has(x));
        return el("div", { class: "dst-box", style: "padding:10px;gap:6px;min-width:230px" },
          el("span", { class: "dst-azioni" }, el("span", { class: "dst-tipo " + (this.tipoDi(t.nodo) || "sciolto") }, TIPI[this.tipoDi(t.nodo)] || "Pezzo"), el("b", { class: "mono" }, this.nome(t.nodo))),
          el("span", { class: "k" }, t.padre ? "non più sotto " + this.nome(t.padre) : "tolto dall'albero"),
          cascata.length ? el("span", { class: "k" }, "a cascata: " + cascata.map((x) => this.nome(x)).join(", ")) : null,
          this.scrive ? el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "rimetti|" + t.nodo + "|" + (t.padre || ""), onclick: () => this.ripristina(t.nodo, t.padre || "") }, t.padre ? "Rimetti sotto " + this.nome(t.padre) : "Rimetti nell'albero") : null);
      });
      parti.push(el("div", { class: "dst-vassoio-lista" }, carte));
    }
    const fuori = S.dati.fuori || [];
    const rimessi = new Set(this.bozza.aggiunti.map((x) => (x.codice || "").toUpperCase()));
    const restano = fuori.filter((f) => !rimessi.has(f.codice.toUpperCase()));
    if (restano.length) {
      parti.push(el("span", { class: "dst-label" }, `Fuori dalla distinta (${restano.length})`),
        el("p", { class: "k" }, "Componenti della richiesta che non stanno sotto nessun prodotto. Si rimettono nell'albero (con la conferma), oppure si eliminano."));
      parti.push(el("div", { class: "dst-vassoio-lista" }, restano.map((f) => el("div", { class: "dst-box", style: "padding:10px;gap:6px;min-width:230px" },
        el("span", { class: "dst-azioni" }, el("span", { class: "dst-tipo " + f.tipo }, TIPI[f.tipo] || "Pezzo"), el("b", { class: "mono" }, f.codice)),
        f.desc ? el("span", { class: "k" }, f.desc) : null,
        this.scrive ? el("span", { class: "dst-azioni" },
          el("button", { type: "button", class: "dst-btn piccolo", "data-fuoco": "fuori|" + f.id, onclick: () => this.dialogoAggiungi(f.tipo === "finito" ? "sottoassieme" : f.tipo, "", f.codice) }, "Rimetti nell'albero…"),
          el("button", { type: "button", class: "dst-btn piccolo pericolo", "data-fuoco": "elimina|" + f.id, onclick: (e) => this.eliminaFuori(f, e.currentTarget) }, "Elimina definitivamente…")) : null))));
    }
    v.hidden = !parti.length;
    v.replaceChildren(...parti);
  }

  disegnaSalva() {
    const b = document.getElementById("dst-salva");
    if (!b) return;
    const n = vociBozza(this.bozza);
    const proposti = this.daConfermare();
    const legami = [...this.vista.legami.values()].filter((l) => l.arco && (l.arco.stato === "proposto" || l.arco.stato === "tolto_dallo_step")).length;
    b.hidden = !this.scrive || !!this.trovata || (!n && !proposti && !legami);
    const t = document.getElementById("dst-salva-testo");
    if (t) {
      t.replaceChildren(n
        ? el("span", {}, el("b", { id: "dst-n-modifiche" }, n), ` ${n === 1 ? "modifica" : "modifiche"} nella bozza, tenute in questo browser: nessun altro le vede, e la distinta cambia solo con «Conferma l'albero».`)
        : el("span", {}, "L'albero proposto ha " + [proposti ? conta(proposti, "pezzo", "pezzi") : "", legami ? conta(legami, "legame", "legami") : ""].filter(Boolean).join(" e ") + " da confermare."));
    }
    const scarta = $("[data-azione=annulla-tutto]", b);
    if (scarta) scarta.hidden = !n;
  }
}

// ------------------------------------------------------------------ la pagina

function avvia() {
  leggiDati();
  legaVisore();
  Dialogo.lega();
  Menu.lega();
  S.editor = null;
  const pan = $(".dst-pannello");
  const passo = pan && pan.dataset ? pan.dataset.passo : "";
  if (passo === "distinta") Distinta.avvia();
  // nel passo 3: una bozza dell'albero non ancora confermata, lo si dice (i file si smistano sui pezzi confermati)
  const nota = document.getElementById("dst-bozza-nota");
  if (nota && S.dati && S.dati.scrive) {
    const t = Memoria.leggi();
    nota.hidden = !t;
    if (t) nota.textContent = `Hai una bozza dell'albero non confermata (${conta(vociBozza(t.bozza), "modifica", "modifiche")}, delle ${ora(t.quando)}): i file si smistano sui pezzi già confermati. La bozza si conferma nel passo 2.`;
  }
  const av = $(".dst-avviso");
  if (av) avvisa(av.textContent.trim(), av.dataset.esito === "no");
}

// i clic della pagina: disegni, strumenti dell'albero
document.addEventListener("click", (e) => {
  const comp = e.target.closest("[data-apri-comp]");
  if (comp) {
    const lista = pdfDelComponente(comp.dataset.apriComp).filter((p) => p.ok);
    if (!lista.length) { avvisa("Nessun disegno per " + (comp.dataset.codice || "questo pezzo") + " su questo server.", true); return; }
    apriVisore(lista[0].a, comp.dataset.apriComp, comp);
    return;
  }
  const file = e.target.closest("[data-apri-file]");
  if (file) {
    const p = nomePdf(file.dataset.apriFile);
    apriVisore(file.dataset.apriFile, p ? p.comp : "", file);
    return;
  }
  const nuovo = e.target.closest("[data-nuovo]");
  if (nuovo && S.editor) { S.editor.dialogoAggiungi(nuovo.dataset.nuovo, ""); return; }
  const az = e.target.closest("[data-azione]");
  if (!az || !S.editor) return;
  const ed = S.editor;
  switch (az.dataset.azione) {
    case "rivedi": ed.rivedi(az); break;
    case "annulla-tutto": ed.scartaBozza(az); break;
    case "indietro": ed.annullaUltima(); break;
    case "elimina": {
      if (!ed.sel || ed.prodotto(ed.sel)) { avvisa("Il prodotto non si toglie: scegli prima un pezzo sotto di lui.", true); break; }
      ed.dialogoTogli(ed.sel, "");
      break;
    }
  }
});

// un gesto del server nel passo 2 (Rianalizza) cambia l'albero proposto: con una bozza aperta la bozza diventerebbe
// vecchia. Prima si conferma o si scarta
document.addEventListener("htmx:beforeRequest", (e) => {
  const elt = e.detail && e.detail.elt;
  if (!elt || !S.editor || !S.editor.cambiato()) return;
  if (!elt.closest || !elt.closest("#distinta")) return;
  e.preventDefault();
  avvisa("Prima conferma o scarta la bozza dell'albero: questo gesto cambia l'albero proposto.", true);
});
// il posto e il fuoco prima e dopo lo scambio del corpo (bug 4)
document.addEventListener("htmx:beforeSwap", (e) => {
  const t = e.detail && e.detail.target;
  if (!t || t.id !== "distinta") return;
  postoSalvato = ricordaPosto(e.detail.elt || (e.detail.requestConfig && e.detail.requestConfig.elt));
});
document.addEventListener("htmx:afterSwap", (e) => {
  const t = e.detail && e.detail.target;
  if (!t || t.id !== "distinta") return;
  avvia();
  rimettiPosto(postoSalvato);
  postoSalvato = null;
});

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia);
else avvia();
