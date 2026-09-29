// La Distinta (cockpit/_fasi/PROPOSTA_DISTINTA.md): lo schema della distinta, le miniature dei disegni e il
// visore con le note. Il resto della pagina e' HTML del server, che i gesti rifanno (#distinta).
//
// Lo schema lavora come l'editor della struttura del Fascicolo, sugli stessi dati (GET /fascicolo/bom/dati) e
// con la stessa conferma (POST /fascicolo/bom/applica, una StrutturaVoluta): gli archi voluti sono lo stato, le
// proposte degli STEP autorizzati si vedono tratteggiate e le mette l'operatore, un codice scritto e' una carta
// «n:». In piu': sotto un particolare e sotto un particolare commerciale non si mette niente (il server lo
// rifiuta comunque), e la miniatura del disegno di ogni pezzo apre il visore con le note.

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

// lo stato della pagina: il JSON del server (#dst-dati) e lo schema
const S = { dati: null, editor: null };

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
  c.innerHTML = html;
  if (window.htmx) window.htmx.process(c);
  avvia();
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
function urlPdf(a) { return "/allegato/" + a + "/anteprima"; }
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

const V = { file: null, comp: "", doc: null, task: null, pagina: 1, pagine: 1, zoom: 0, armato: false, sel: null, nuovo: null, modifica: null, conferma: null, da: null, gen: 0 };
const ZOOM = [1, 1.25, 1.5, 2, 3, 4];

function pdfDelComponente(comp) { return ((S.dati && S.dati.pdf && S.dati.pdf[comp]) || []); }
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

async function apriVisore(a, comp, da) {
  const vis = document.getElementById("dst-visore");
  if (!vis || !a) return;
  const p = nomePdf(a);
  if (p && !p.ok) { avvisa(p.nome + ": il file non è su questo server. Si riscarica da Documenti e NAS.", true); return; }
  V.file = a; V.comp = comp || (p && p.comp) || ""; V.pagina = 1; V.zoom = 0; V.armato = false; V.sel = null; V.nuovo = null; V.modifica = null; V.conferma = null;
  V.da = da || document.activeElement;
  // l'elenco dei disegni: prima quelli del pezzo, poi gli altri della richiesta
  const sel = document.getElementById("dst-vis-file");
  sel.replaceChildren();
  const suoi = V.comp ? pdfDelComponente(V.comp) : [];
  const altri = ((S.dati && S.dati.tutti) || []).filter((x) => !suoi.some((y) => y.a === x.a));
  const gruppo = (titolo, lista) => {
    if (!lista.length) return;
    const g = el("optgroup", { label: titolo });
    for (const x of lista) g.append(el("option", { value: x.a, selected: x.a === a }, x.nome + (x.ok ? "" : " (non disponibile)")));
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
  if (V.comp && S.editor && S.editor.nodi["c:" + V.comp]) {
    const n = S.editor.nodi["c:" + V.comp];
    pezzo = (TIPI[n.finito ? "finito" : n.tipo] || "Pezzo") + " " + n.codice + " · ";
  } else if (p && p.cc) {
    pezzo = "pezzo " + p.cc + " · ";
  } else if (p && !p.comp) {
    pezzo = "non ancora di un pezzo · ";
  }
  document.getElementById("dst-vis-sotto").textContent = pezzo + "pagina " + V.pagina + " di " + V.pagine;
  document.getElementById("dst-pag").textContent = V.pagina + "/" + V.pagine;
  document.getElementById("dst-pag-prec").disabled = V.pagina <= 1;
  document.getElementById("dst-pag-succ").disabled = V.pagina >= V.pagine;
  document.getElementById("dst-z").textContent = Math.round(ZOOM[V.zoom] * 100) + "%";
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
    const riga = el("div", {
      class: "dst-nota" + (V.sel === n.id ? " sel" : ""), role: "button", tabindex: "0",
      onclick: () => vaiANota(n), onkeydown: (e) => { if (e.key === "Enter" && e.target === riga) vaiANota(n); },
    }, el("span", { class: "n" }, n.n), el("span", {}, n.t),
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
  document.addEventListener("keydown", (e) => {
    if (vis.hidden || e.key !== "Escape") return;
    if (V.nuovo) { V.nuovo = null; disegnaNote(); } else if (V.armato) { V.armato = false; barraVisore(); } else chiudiVisore();
  });
  let tRidim = 0;
  window.addEventListener("resize", () => { if (vis.hidden) return; clearTimeout(tRidim); tRidim = setTimeout(disegnaPagina, 200); });
}

// ------------------------------------------------------------------ lo schema della distinta

class Distinta {
  static async avvia() {
    const tree = document.getElementById("dst-tree");
    if (!tree || !S.dati) return;
    const prodotti = S.dati.prodotti || [];
    if (!prodotti.length) {
      tree.replaceChildren(el("div", { class: "dst-box tono-2", style: "max-width:560px;margin:0 auto" },
        el("b", {}, "Non c'è ancora un prodotto."),
        el("p", { class: "k" }, "I codici della richiesta, confermati nella Inbox, diventano prodotti da soli. Poi qui si costruisce la distinta.")));
      $("#dst-analisi-corpo").replaceChildren(el("p", { class: "k" }, "Senza un prodotto non c'è una struttura da proporre."));
      return;
    }
    const chiave = "cockpit.distinta.prodotto." + S.dati.thread;
    let scelto = prova(() => sessionStorage.getItem(chiave), null);
    if (!prodotti.some((p) => p.id === scelto)) scelto = prodotti[0].id;
    let dati;
    try {
      const r = await fetch(S.dati.base + "/bom/dati?prodotto=" + encodeURIComponent(scelto), { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(r.status);
      dati = await r.json();
    } catch (e) {
      tree.replaceChildren(el("p", { class: "bad-t" }, "La distinta non si è potuta leggere: ricarica la pagina."));
      return;
    }
    if (!document.getElementById("dst-tree")) return; // la pagina e' cambiata nel frattempo
    S.editor = new Distinta(dati, chiave);
  }

  constructor(dati, chiaveProdotto) {
    this.d = dati;
    this.chiaveProdotto = chiaveProdotto;
    this.radice = dati.prodotto;
    this.nodi = dati.nodi || {};
    this.archi = new Map();
    for (const a of dati.archi || []) this.archi.set(a.padre + "|" + a.figlio, { padre: a.padre, figlio: a.figlio, qta: a.qta, prop: false });
    this.proposteDi = new Map();
    for (const p of dati.proposti || []) this.proposteDi.set(p.padre + "|" + p.figlio, p);
    this.ritrovati = new Map();
    for (const r of dati.ritrovati || []) { if (!this.ritrovati.has(r.ref)) this.ritrovati.set(r.ref, []); this.ritrovati.get(r.ref).push(r); }
    this.scarta = new Set();
    this.codici = {};
    this.nuovi = {};
    this.portati = new Set();
    this.storia = [];
    this.sel = this.radice;
    this.selPadre = "";
    this.conferma = null;   // il riquadro di conferma aperto nel dettaglio
    this.guidaBozza = null; // la creazione dei pezzi dalla guida, in attesa di conferma
    this.scrive = !!dati.scrive && !dati.bloccata;
    this.iniziale = this.firma();
    this.disegna();
  }

  // ---- lo stato

  firma() {
    return JSON.stringify([[...this.archi.values()].map((a) => [a.padre, a.figlio, a.qta]).sort(), [...this.scarta].sort(), this.codici, this.nuovi]);
  }
  cambiato() { return this.firma() !== this.iniziale; }
  istantanea() {
    return { archi: [...this.archi.values()].map((a) => ({ ...a })), scarta: [...this.scarta], codici: { ...this.codici },
      nuovi: JSON.parse(JSON.stringify(this.nuovi)), portati: [...this.portati], sel: this.sel };
  }
  ripristina(s) {
    this.archi = new Map(s.archi.map((a) => [a.padre + "|" + a.figlio, a]));
    this.scarta = new Set(s.scarta);
    this.codici = { ...s.codici };
    for (const r of Object.keys(this.nodi)) if (r.startsWith("n:")) delete this.nodi[r];
    this.nuovi = JSON.parse(JSON.stringify(s.nuovi || {}));
    for (const [r, n] of Object.entries(this.nuovi)) this.nodi[r] = { codice: n.codice, tipo: n.tipo, nuovo: true };
    this.portati = new Set(s.portati || []);
    this.sel = s.sel && this.nodi[s.sel] ? s.sel : this.radice;
  }
  prima() {
    this.storia.push(this.istantanea());
    if (this.storia.length > 200) this.storia.shift();
    this.conferma = null;
  }
  annullaUltima() {
    const s = this.storia.pop();
    if (!s) return;
    this.ripristina(s);
    this.disegna("Ultima modifica annullata.");
  }

  // ---- come si legge lo stato

  figliDi(ref) {
    const out = [];
    for (const a of this.archi.values()) if (a.padre === ref) out.push(a);
    return out.sort((x, y) => this.nome(x.figlio).localeCompare(this.nome(y.figlio)));
  }
  padriDi(ref) {
    const out = [];
    for (const a of this.archi.values()) if (a.figlio === ref) out.push(a);
    return out;
  }
  raggiunti(da) {
    const visti = new Set([da]);
    const coda = [da];
    while (coda.length) {
      const n = coda.shift();
      for (const a of this.archi.values()) if (a.padre === n && !visti.has(a.figlio)) { visti.add(a.figlio); coda.push(a.figlio); }
    }
    return visti;
  }
  raggiuntiDa(da, archi, proposti) {
    const figli = new Map();
    for (const a of [...archi, ...proposti]) { if (!figli.has(a.padre)) figli.set(a.padre, []); figli.get(a.padre).push(a.figlio); }
    const visti = new Set([da]);
    const coda = [da];
    while (coda.length) { const n = coda.shift(); for (const f of figli.get(n) || []) if (!visti.has(f)) { visti.add(f); coda.push(f); } }
    return visti;
  }
  scendeDa(anc, ref) { return anc === ref || this.raggiunti(ref).has(anc); }
  nome(ref) {
    const n = this.nodi[ref];
    if (!n) return ref;
    if (ref.startsWith("p:") && this.codici[ref.slice(2)] && this.codici[ref.slice(2)].codice) return this.codici[ref.slice(2)].codice;
    return n.codice || (n.nome ? "«" + n.nome + "»" : "senza codice");
  }
  tipoDi(ref) {
    if (ref.startsWith("n:")) return (this.nuovi[ref] && this.nuovi[ref].tipo) || "";
    const n = this.nodi[ref];
    if (!n) return "";
    if (n.finito) return "finito";
    return n.tipo || "";
  }
  // chi puo' avere dei pezzi sotto: il prodotto e l'assieme; un nodo proposto dallo STEP diventa assieme se ne ha
  contenitore(ref) {
    const t = this.tipoDi(ref);
    if (ref.startsWith("p:")) return t !== "commerciale";
    return t === "finito" || t === "sottoassieme";
  }
  prodotti() { return new Set((this.d.prodotti || []).map((p) => p.ref)); }
  comp(ref) { return ref.startsWith("c:") ? ref.slice(2) : ""; }

  // i legami che gli STEP autorizzati propongono sotto i pezzi della struttura, non ancora messi
  daMettere() {
    const qui = this.raggiunti(this.radice);
    return (this.d.proposti || []).filter((p) => qui.has(p.padre) && !this.archi.has(p.padre + "|" + p.figlio) &&
      !this.scarta.has(p.figlio.slice(2)) && !this.scendeDa(p.padre, p.figlio));
  }
  vassoio() {
    const qui = this.raggiunti(this.radice);
    const altrove = new Set();
    for (const p of this.prodotti()) if (p !== this.radice) for (const x of this.raggiunti(p)) altrove.add(x);
    const inProposta = new Set(this.daMettere().map((p) => p.figlio));
    const out = [];
    for (const ref of Object.keys(this.nodi)) {
      const n = this.nodi[ref];
      if (qui.has(ref) || (altrove.has(ref) && !this.portati.has(ref)) || n.finito || this.scarta.has(ref.slice(2)) || inProposta.has(ref)) continue;
      out.push(ref);
    }
    return out.sort((a, b) => this.nome(a).localeCompare(this.nome(b)));
  }
  modifiche() {
    const qui = this.raggiunti(this.radice);
    const prima = new Map();
    for (const a of this.d.archi || []) prima.set(a.padre + "|" + a.figlio, a.qta);
    let n = 0;
    for (const a of this.archi.values()) {
      if (!qui.has(a.padre)) continue;
      if (!prima.has(a.padre + "|" + a.figlio) || prima.get(a.padre + "|" + a.figlio) !== a.qta) n++;
    }
    for (const k of prima.keys()) { const [p] = k.split("|"); if (qui.has(p) && !this.archi.has(k)) n++; }
    return n + this.scarta.size + Object.keys(this.codici).length;
  }

  // ---- i gesti sullo schema (restano qui finche' non si salva)

  proteggi() {
    if (!this.scrive) { avvisa(this.d.bloccata ? "La distinta è congelata: si cambia aprendo una revisione." : "Chi consulta non cambia la distinta.", true); return false; }
    return true;
  }
  sposta(padre, figlio, verso) {
    if (!this.proteggi() || !verso || verso === padre) return;
    if (!this.contenitore(verso)) {
      avvisa(`${this.nome(verso)} è ${TIPI_FRASE[this.tipoDi(verso)] || "un pezzo"}: sotto non ci va niente. Mettilo sotto il prodotto o sotto un assieme.`, true);
      return;
    }
    if (this.scendeDa(verso, figlio)) { avvisa(`${this.nome(figlio)} non può andare sotto ${this.nome(verso)}: ${this.nome(verso)} sta già sotto di lui.`, true); return; }
    if (this.archi.has(verso + "|" + figlio)) { avvisa(`${this.nome(figlio)} è già sotto ${this.nome(verso)}.`, true); return; }
    this.prima();
    const vecchio = padre ? this.archi.get(padre + "|" + figlio) : null;
    if (vecchio) this.archi.delete(padre + "|" + figlio);
    const prop = this.proposteDi.get(verso + "|" + figlio);
    this.archi.set(verso + "|" + figlio, { padre: verso, figlio, qta: vecchio ? vecchio.qta : prop ? prop.qta : 1, prop: !!prop && !vecchio });
    this.portati.delete(figlio);
    this.sel = figlio;
    this.selPadre = verso;
    this.disegna(`${this.nome(figlio)} ora è sotto ${this.nome(verso)}.`);
  }
  condividi(figlio, verso) {
    if (!this.proteggi()) return;
    if (!this.contenitore(verso)) { avvisa(`${this.nome(verso)} è ${TIPI_FRASE[this.tipoDi(verso)] || "un pezzo"}: sotto non ci va niente.`, true); return; }
    if (this.scendeDa(verso, figlio)) { avvisa(`${this.nome(figlio)} non può andare sotto ${this.nome(verso)}: chiuderebbe un giro.`, true); return; }
    if (this.archi.has(verso + "|" + figlio)) { avvisa(`${this.nome(figlio)} è già sotto ${this.nome(verso)}.`, true); return; }
    this.prima();
    this.archi.set(verso + "|" + figlio, { padre: verso, figlio, qta: 1, prop: false });
    this.disegna(`${this.nome(figlio)} anche sotto ${this.nome(verso)}: ora è condiviso.`);
  }
  togli(padre, figlio) {
    if (!this.proteggi()) return;
    this.prima();
    this.archi.delete(padre + "|" + figlio);
    const fuori = !this.raggiunti(this.radice).has(figlio);
    if (fuori && figlio.startsWith("n:")) { delete this.nuovi[figlio]; delete this.nodi[figlio]; this.sel = padre; this.disegna(`${figlio.slice(2)} tolto: non nasce.`); return; }
    this.sel = fuori ? figlio : this.sel;
    this.disegna(`${this.nome(figlio)} non è più sotto ${this.nome(padre)}${fuori ? ": è fra i pezzi fuori dalla distinta, in fondo" : ""}.`);
  }
  quantita(padre, figlio, q) {
    if (!this.proteggi()) return;
    const a = this.archi.get(padre + "|" + figlio);
    const n = parseInt(q, 10);
    if (!a) return;
    if (!(n >= 1 && n <= 100000)) { avvisa("La quantità va da 1 a 100000.", true); this.disegna(); return; }
    if (n === a.qta) return;
    this.prima();
    a.qta = n;
    this.disegna(`${this.nome(figlio)} sotto ${this.nome(padre)}: quantità ${n}.`);
  }
  accetta(p) {
    if (!this.proteggi()) return;
    if (this.archi.has(p.padre + "|" + p.figlio)) return;
    this.prima();
    this.archi.set(p.padre + "|" + p.figlio, { padre: p.padre, figlio: p.figlio, qta: p.qta, prop: true });
    this.sel = p.figlio;
    this.selPadre = p.padre;
    this.disegna(`${this.nome(p.figlio)} ×${p.qta} sotto ${this.nome(p.padre)}: entra con «Salva la distinta».`);
  }
  accettaTutto() {
    if (!this.proteggi()) return;
    let da = this.daMettere();
    if (!da.length) { avvisa("Non ci sono legami proposti da mettere."); return; }
    this.prima();
    let n = 0;
    // anche i figli dei nodi appena messi: si ripete finche' ce ne sono
    while (da.length) {
      for (const p of da) { this.archi.set(p.padre + "|" + p.figlio, { padre: p.padre, figlio: p.figlio, qta: p.qta, prop: true }); n++; }
      da = this.daMettere();
    }
    this.disegna(`${n} legam${n === 1 ? "e" : "i"} dagli STEP nella distinta: si vedono qui ed entrano con «Salva la distinta».`);
  }
  scartaNodo(ref) {
    if (!this.proteggi() || !ref.startsWith("p:")) return;
    this.prima();
    this.scarta.add(ref.slice(2));
    for (const k of [...this.archi.keys()]) { const a = this.archi.get(k); if (a.padre === ref || a.figlio === ref) this.archi.delete(k); }
    this.sel = this.radice;
    this.disegna(`${this.nome(ref)} scartato: non è un pezzo della distinta.`);
  }
  cambiaTipoNuovo(ref, tipo) {
    if (!this.proteggi() || !this.nuovi[ref]) return;
    if (tipo !== "sottoassieme" && this.figliDi(ref).length) {
      avvisa(`${this.nome(ref)} ha dei pezzi sotto: per farlo diventare ${TIPI_FRASE[tipo]} sposta prima i suoi pezzi.`, true);
      this.disegna();
      return;
    }
    this.prima();
    this.nuovi[ref].tipo = tipo;
    this.nodi[ref].tipo = tipo;
    this.disegna(`${this.nome(ref)} è ${TIPI_FRASE[tipo]}.`);
  }
  scriviCodice(ref, codice) {
    if (!this.proteggi()) return;
    codice = (codice || "").trim();
    if (codice && (codice.length > 40 || /\s/.test(codice))) { avvisa("Un codice ha al massimo 40 caratteri, senza spazi.", true); return; }
    this.prima();
    if (codice) this.codici[ref.slice(2)] = { codice, rev: "" }; else delete this.codici[ref.slice(2)];
    this.disegna(codice ? `Il nodo si chiamerà ${codice}.` : "");
  }

  // il codice interno proposto per un assieme: <prodotto>-A01, -A02… il primo libero
  codiceInterno() {
    const base = this.nome(this.radice);
    const usati = new Set(Object.values(this.nodi).map((n) => (n.codice || "").toUpperCase()));
    for (let i = 1; i < 100; i++) {
      const c = base + "-A" + String(i).padStart(2, "0");
      if (!usati.has(c.toUpperCase())) return c;
    }
    return base + "-A";
  }

  // il modulo di «+ Assieme», «+ Particolare», «+ Particolare commerciale»
  formNuovo(tipo) {
    if (!this.proteggi()) return;
    const posto = document.getElementById("dst-nuovo");
    if (!posto) return;
    const contenitori = [...this.raggiunti(this.radice)].filter((r) => this.contenitore(r));
    const preferito = this.contenitore(this.sel) && contenitori.includes(this.sel) ? this.sel : this.radice;
    const cod = el("input", { id: "dst-nuovo-codice", maxlength: "40", autocomplete: "off", class: "mono" });
    this.codiceProposto = tipo === "sottoassieme" ? this.codiceInterno() : "";
    cod.value = this.codiceProposto;
    const aiuto = el("small", {}, tipo === "sottoassieme" ? "Codice interno proposto: lo puoi cambiare." : "Il codice del cliente. Se non c'è, scrivi un codice interno.");
    const padre = el("select", { id: "dst-nuovo-padre" }, contenitori.map((r) => el("option", { value: r, selected: r === preferito }, (TIPI[this.tipoDi(r)] || "Pezzo") + " " + this.nome(r))));
    const qta = el("input", { id: "dst-nuovo-qta", type: "number", min: "1", max: "100000", value: "1" });
    const diverso = el("div", { hidden: true });
    const errore = el("small", { class: "bad-t", hidden: true });
    const form = el("form", { class: "dst-form-nuovo", autocomplete: "off", onsubmit: (e) => { e.preventDefault(); this.aggiungiNuovo(tipo, cod.value, padre.value, qta.value, diverso, errore); } },
      el("label", { class: "dst-campo" }, el("span", {}, "Codice"), cod, aiuto),
      el("label", { class: "dst-campo" }, el("span", {}, "Sotto"), padre, el("small", {}, "Solo il prodotto o un assieme")),
      el("label", { class: "dst-campo" }, el("span", {}, "Quantità"), qta),
      el("div", { class: "dst-azioni" }, el("button", { class: "dst-btn primario", type: "submit" }, "Aggiungi"),
        el("button", { class: "dst-btn", type: "button", onclick: () => { posto.hidden = true; posto.replaceChildren(); } }, "Annulla")));
    posto.replaceChildren(el("span", { class: "dst-label" }, "Nuovo " + TIPI[tipo].toLowerCase()), form, diverso, errore);
    posto.hidden = false;
    cod.focus();
    cod.select();
  }
  async aggiungiNuovo(tipo, codice, padre, qta, diversoBox, errore) {
    codice = (codice || "").trim();
    const q = parseInt(qta, 10);
    const dice = (t) => { errore.textContent = t; errore.hidden = false; };
    errore.hidden = true;
    if (!codice) return dice("Scrivi il codice del pezzo.");
    if (codice.length > 40 || /\s/.test(codice)) return dice("Un codice ha al massimo 40 caratteri, senza spazi.");
    if (!(q >= 1 && q <= 100000)) return dice("La quantità va da 1 a 100000.");
    if (!this.contenitore(padre)) return dice("Sotto un particolare non si mette niente: scegli il prodotto o un assieme.");
    if (Object.keys(this.nuovi).some((r) => r.toUpperCase() === ("n:" + codice).toUpperCase())) return dice(codice + " l'hai già scritto: è fra i pezzi nuovi.");
    let e;
    try {
      const r = await fetch(S.dati.base + "/bom/codice?codice=" + encodeURIComponent(codice), { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(r.status);
      e = await r.json();
    } catch (err) {
      return dice("Il codice non si è potuto controllare: riprova.");
    }
    if (e.errore) return dice(e.errore);
    if (e.esiste) {
      if (e.esiste.archiviato) return dice(`${e.esiste.codice} c'è già ed è archiviato: si ripristina dal Fascicolo completo, non se ne crea un altro.`);
      if (e.esiste.tipo === "finito") return dice(`${e.esiste.codice} è un prodotto della richiesta: non va sotto un altro pezzo.`);
      document.getElementById("dst-nuovo").hidden = true;
      if (this.raggiunti(this.radice).has(e.esiste.ref)) {
        this.sel = e.esiste.ref;
        this.disegna(`${e.esiste.codice} c'è già ed è nella distinta: per metterlo anche sotto un altro assieme usa «Anche sotto…» nel riquadro qui sotto.`);
        return;
      }
      this.sposta("", e.esiste.ref, padre);
      return;
    }
    if (e.richiesta) return dice(`${codice} è un codice della richiesta: diventa un prodotto con la decisione nella Inbox, non un pezzo qui.`);
    let diverso = false;
    // il codice interno proposto (e lasciato com'e') e' un assieme nuovo per costruzione: e' diverso da ogni altro
    if (tipo === "sottoassieme" && this.codiceProposto && codice === this.codiceProposto) diverso = true;
    else if (e.vicini && e.vicini.length) {
      const spunta = $("input[type=checkbox]", diversoBox);
      if (!spunta || !spunta.checked) {
        const elenco = e.vicini.map((v) => `${v.codice} (${v.motivo})`).join(", ");
        diversoBox.replaceChildren(el("div", { class: "dst-conferma-box warn" },
          el("span", {}, `${codice} è quasi uguale a ${elenco}. Se è lo stesso pezzo, usa quello e non scriverne un altro.`),
          el("label", {}, el("input", { type: "checkbox" }), ` ${codice} è un pezzo diverso`)));
        diversoBox.hidden = false;
        return dice("Conferma che è un pezzo diverso, poi di nuovo «Aggiungi».");
      }
      diverso = true;
    }
    this.prima();
    const ref = "n:" + codice;
    this.nuovi[ref] = { codice, tipo, rev: "", diverso };
    this.nodi[ref] = { codice, tipo, nuovo: true };
    this.archi.set(padre + "|" + ref, { padre, figlio: ref, qta: q, prop: false });
    this.sel = ref;
    this.selPadre = padre;
    document.getElementById("dst-nuovo").hidden = true;
    this.disegna(`${codice} (${TIPI[tipo].toLowerCase()}) sotto ${this.nome(padre)}: nasce con «Salva la distinta».`);
  }

  // «Crea questi pezzi a mano» dalla guida di uno STEP non autorizzato: i codici dei nodi diventano carte «n:» (o i
  // componenti che ci sono gia'), sotto il prodotto come nel file. E' l'operatore che li scrive: li vede prima.
  async preparaGuida() {
    if (!this.proteggi()) return;
    const guida = this.d.guida || [];
    const figliDi = new Map();
    const eFiglio = new Set();
    for (const g of guida) { if (!figliDi.has(g.padre)) figliDi.set(g.padre, []); figliDi.get(g.padre).push(g); eFiglio.add(g.figlio); }
    const radiciGuida = [...figliDi.keys()].filter((p) => !eFiglio.has(p));
    const perCodice = new Map();
    for (const [r, n] of Object.entries(this.nodi)) if (r.startsWith("c:") && n.codice) perCodice.set(n.codice.toUpperCase(), r);
    const piano = { nuovi: [], archi: [], problemi: [], vicini: [] };
    const refDi = new Map();
    for (const r of radiciGuida) refDi.set(r, this.radice);
    const etichette = new Set(guida.map((g) => g.figlio));
    for (const etichetta of etichette) {
      if (refDi.has(etichetta)) continue;
      const codice = String(etichetta).replace(/^«|»$/g, "").trim();
      if (!codice || /\s/.test(codice) || codice.length > 40) { piano.problemi.push(`«${etichetta}»: non è un codice che si può scrivere`); continue; }
      const gia = perCodice.get(codice.toUpperCase());
      if (gia) { refDi.set(etichetta, gia); continue; }
      let e = null;
      try {
        const r = await fetch(S.dati.base + "/bom/codice?codice=" + encodeURIComponent(codice), { credentials: "same-origin", headers: { Accept: "application/json" } });
        if (r.ok) e = await r.json();
      } catch (err) { /* sotto */ }
      if (!e) { piano.problemi.push(`${codice}: non si è potuto controllare`); continue; }
      if (e.errore) { piano.problemi.push(`${codice}: ${e.errore}`); continue; }
      if (e.esiste) { if (e.esiste.archiviato || e.esiste.tipo === "finito") { piano.problemi.push(`${codice}: c'è già (${e.esiste.archiviato ? "archiviato" : "è un prodotto"})`); continue; } refDi.set(etichetta, e.esiste.ref); continue; }
      if (e.richiesta) { refDi.set(etichetta, this.radice); continue; }
      const ref = "n:" + codice;
      refDi.set(etichetta, ref);
      const conFigli = figliDi.has(etichetta);
      piano.nuovi.push({ ref, codice, tipo: conFigli ? "sottoassieme" : "sciolto", vicini: (e.vicini || []).map((v) => v.codice) });
    }
    for (const g of guida) {
      const p = refDi.get(g.padre), f = refDi.get(g.figlio);
      if (!p || !f || p === f) continue;
      piano.archi.push({ padre: p, figlio: f, qta: g.qta || 1 });
    }
    this.guidaBozza = piano;
    this.disegnaAnalisi();
    const box = document.getElementById("dst-analisi");
    if (box) box.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }
  applicaGuida() {
    const piano = this.guidaBozza;
    if (!piano) return;
    this.prima();
    for (const n of piano.nuovi) {
      if (this.nuovi[n.ref]) continue;
      this.nuovi[n.ref] = { codice: n.codice, tipo: n.tipo, rev: "", diverso: n.vicini.length > 0 };
      this.nodi[n.ref] = { codice: n.codice, tipo: n.tipo, nuovo: true };
    }
    let messi = 0;
    for (const a of piano.archi) {
      const k = a.padre + "|" + a.figlio;
      if (this.archi.has(k) || !this.contenitore(a.padre) || this.scendeDa(a.padre, a.figlio)) continue;
      this.archi.set(k, { padre: a.padre, figlio: a.figlio, qta: a.qta, prop: false });
      messi++;
    }
    this.guidaBozza = null;
    this.disegna(`${piano.nuovi.length} pezz${piano.nuovi.length === 1 ? "o" : "i"} nuov${piano.nuovi.length === 1 ? "o" : "i"} e ${messi} legam${messi === 1 ? "e" : "i"} dallo STEP: controlla i tipi (i dadi sono particolari commerciali?) e salva la distinta.`);
  }

  payload() {
    const qui = this.raggiunti(this.radice);
    const nostri = new Set([...qui, ...this.raggiuntiDa(this.radice, this.d.archi || [], this.d.proposti || [])]);
    const altrove = new Set();
    for (const p of this.prodotti()) if (p !== this.radice) for (const x of this.raggiuntiDa(p, this.d.archi || [], this.d.proposti || [])) altrove.add(x);
    const visto = (ref) => nostri.has(ref) || !altrove.has(ref);
    const archi = [];
    for (const a of this.archi.values()) if (qui.has(a.padre)) archi.push({ padre: a.padre, figlio: a.figlio, qta: a.qta });
    const usati = new Set();
    for (const a of archi) { usati.add(a.padre); usati.add(a.figlio); }
    const nuovi = Object.entries(this.nuovi).filter(([r]) => usati.has(r)).map(([, n]) => ({ codice: n.codice, tipo: n.tipo, rev: n.rev || "", diverso: !!n.diverso }));
    return {
      radice: this.radice.slice(2),
      archi,
      visti: (this.d.archi || []).map((a) => ({ padre: a.padre, figlio: a.figlio, qta: a.qta })),
      relazioni_viste: (this.d.proposti || []).filter((p) => visto(p.padre)).map((p) => ({ allegato: p.allegato, padre: p.pk, figlio: p.fk })),
      ritrovati_visti: (this.d.ritrovati || []).filter((r) => qui.has(r.ref)).map((r) => r.proposta),
      scarta: [...this.scarta],
      codici: { ...this.codici },
      nuovi,
    };
  }
  async salva() {
    if (!this.proteggi()) return;
    const qui = this.raggiunti(this.radice);
    for (const r of qui) {
      const n = this.nodi[r];
      if (n && n.senza_codice && !(this.codici[r.slice(2)] && this.codici[r.slice(2)].codice)) {
        this.sel = r;
        this.disegna();
        avvisa(`${this.nome(r)} non ha un codice: scrivilo nel riquadro qui sotto, o toglilo dalla distinta.`, true);
        return;
      }
    }
    const b = $("[data-azione=salva]");
    if (b) b.disabled = true;
    avvisa("Salvataggio della distinta…");
    this.salvando = true;
    const es = await gesto(S.dati.base + "/bom/applica", { struttura: JSON.stringify(this.payload()) });
    this.salvando = false;
    if (es.ok) { avvisa(es.testo || "Distinta salvata."); return; } // il corpo e' gia' rifatto (avvia)
    if (b) b.disabled = false;
    avvisa(es.testo || "La distinta non si è potuta salvare.", true);
  }
  async annullaTutto() {
    this.storia = [];
    await Distinta.avvia();
    avvisa("Modifiche annullate: la distinta è quella salvata.");
  }

  // ---- i gesti che vanno subito al server (fuori dallo schema): vogliono lo schema salvato

  liberoPerGesto() {
    if (this.cambiato()) { avvisa("Prima salva o annulla le modifiche alla distinta: questo gesto va subito al server e rifà la pagina.", true); return false; }
    return true;
  }
  async anteprimaTipo(ref, tipo) {
    if (!this.proteggi() || !this.liberoPerGesto()) { this.disegna(); return; }
    let e;
    try {
      const r = await fetch(S.dati.pagina + "/tipo?componente=" + encodeURIComponent(ref.slice(2)) + "&tipo=" + encodeURIComponent(tipo), { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(r.status);
      e = await r.json();
    } catch (err) {
      avvisa("Il cambio di tipo non si è potuto preparare: riprova.", true);
      this.disegna();
      return;
    }
    this.conferma = { tipo: "tipo", ref, nuovo: tipo, e };
    this.disegnaDettaglio();
  }
  async gestoComponente(percorso, valori, messaggio) {
    if (!this.liberoPerGesto()) return;
    const es = await gesto(S.dati.base + "/componente/" + this.comp(this.sel) + percorso, valori);
    if (!es.ok) avvisa(es.testo || messaggio, true);
  }

  // ---- il disegno

  disegna(messaggio) {
    if (!document.getElementById("dst-tree")) return;
    this.disegnaProdotti();
    this.disegnaAnalisi();
    this.disegnaAlbero();
    this.disegnaDettaglio();
    this.disegnaVassoio();
    this.disegnaSalva();
    miniature();
    if (messaggio) avvisa(messaggio);
  }

  disegnaProdotti() {
    const posto = document.getElementById("dst-prodotto-scelta");
    if (!posto) return;
    const pp = this.d.prodotti || [];
    if (pp.length < 2) { posto.replaceChildren(); return; }
    const sel = el("select", { "aria-label": "Prodotto", onchange: (e) => {
      if (this.cambiato()) { avvisa("Prima salva o annulla le modifiche a questo prodotto.", true); e.target.value = this.radice; return; }
      prova(() => sessionStorage.setItem(this.chiaveProdotto, e.target.value.slice(2)));
      Distinta.avvia();
    } }, pp.map((p) => el("option", { value: p.ref, selected: p.ref === this.radice }, p.codice + (p.desc ? " · " + p.desc : ""))));
    posto.replaceChildren(el("span", { class: "k" }, "Prodotto: "), sel);
  }

  disegnaAnalisi() {
    const corpo = document.getElementById("dst-analisi-corpo");
    const chip = document.getElementById("dst-analisi-chip");
    const bottone = $("[data-azione=accetta-tutto]");
    if (!corpo) return;
    const da = this.daMettere();
    const guida = this.d.guida || [];
    const parti = [];
    if (this.d.analisi > 0) parti.push(el("p", { class: "warn-t" }, `${this.d.analisi} analisi ancora in corso sui file della richiesta: la proposta può cambiare.`));
    if (da.length) {
      parti.push(el("p", {}, `Gli STEP autorizzati propongono ${da.length} legam${da.length === 1 ? "e" : "i"} sotto i pezzi della distinta: nello schema sono tratteggiati.`),
        el("ul", { class: "dst-proposta-lista" }, da.slice(0, 12).map((p) => el("li", {}, el("b", { class: "mono" }, this.nome(p.padre)), "→", el("b", { class: "mono" }, this.nome(p.figlio)), el("span", { class: "k" }, `×${p.qta} · ${p.file}`)))));
      if (da.length > 12) parti.push(el("p", { class: "k" }, `…e altri ${da.length - 12}.`));
    }
    if (guida.length) {
      const file = [...new Set(guida.map((g) => g.file))].join(", ");
      parti.push(el("p", {}, el("b", {}, "Lo STEP " + file), " descrive questa struttura, ma non è ancora autorizzato come STEP del prodotto: è una guida, non entra da sola."),
        el("ul", { class: "dst-proposta-lista" }, guida.map((g) => el("li", {}, el("b", { class: "mono" }, g.padre), "→", el("b", { class: "mono" }, g.figlio), el("span", { class: "k" }, `×${g.qta}${g.suggerito ? " · è " + g.suggerito : ""}`)))));
      if (this.scrive) {
        const azioni = el("div", { class: "dst-azioni" },
          el("button", { type: "button", class: "dst-btn piccolo primario", onclick: () => this.preparaGuida() }, "Crea questi pezzi nella distinta"),
          el("a", { class: "dst-btn piccolo", href: S.dati.base + "?vista=bom&nodo=" + this.radice.slice(2), title: "Nel Fascicolo completo: scheda del prodotto › STEP autorizzato" }, "Autorizza lo STEP (Fascicolo completo)"));
        parti.push(azioni);
      }
      if (this.guidaBozza) {
        const b = this.guidaBozza;
        const box = el("div", { class: "dst-conferma-box warn" },
          el("b", {}, "Prima di metterli nella distinta:"),
          b.nuovi.length ? el("ul", { class: "dst-elenco" }, b.nuovi.map((n) => el("li", {}, el("span", { class: "mono" }, n.codice), ` nasce come ${TIPI[n.tipo].toLowerCase()}`, n.vicini.length ? el("span", { class: "warn-t" }, ` · quasi uguale a ${n.vicini.join(", ")}: lo segno come pezzo diverso`) : ""))) : el("p", {}, "Nessun pezzo nuovo: i codici ci sono già."),
          el("p", {}, `${b.archi.length} legam${b.archi.length === 1 ? "e" : "i"} con le quantità dello STEP.`),
          b.problemi.length ? el("ul", { class: "dst-elenco bad-t" }, b.problemi.map((p) => el("li", {}, p))) : "",
          el("div", { class: "dst-azioni" },
            el("button", { type: "button", class: "dst-btn piccolo primario", onclick: () => this.applicaGuida() }, "Sì, mettili nella distinta"),
            el("button", { type: "button", class: "dst-btn piccolo", onclick: () => { this.guidaBozza = null; this.disegnaAnalisi(); } }, "Annulla")));
        parti.push(box);
      }
    }
    if (this.d.step && this.d.step.etichetta) parti.push(el("p", { class: "k" }, "STEP del prodotto: " + this.d.step.etichetta + (this.d.step.motivo ? " (" + this.d.step.motivo + ")" : "") + "."));
    if (!parti.length) parti.push(el("p", { class: "k" }, this.raggiunti(this.radice).size > 1
      ? "Nessuna proposta da decidere: la distinta è quella salvata. Si cambia con gli strumenti qui sotto."
      : "Nessuno STEP propone una struttura per questo prodotto. I pezzi si aggiungono con i pulsanti qui sotto."));
    corpo.replaceChildren(...parti);
    if (chip) {
      chip.replaceChildren(da.length ? el("span", { class: "dst-chip warn" }, `${da.length} da decidere`)
        : guida.length ? el("span", { class: "dst-chip neu" }, "guida da uno STEP") : el("span", { class: "dst-chip ok" }, "niente da decidere"));
    }
    const box = document.getElementById("dst-analisi");
    if (box) box.className = "dst-box" + (da.length || (guida.length && this.raggiunti(this.radice).size === 1) ? " tono-warn" : " tono-2");
    if (bottone) bottone.hidden = !(da.length && this.scrive);
  }

  // la casella di un pezzo
  casella(ref, padre, qta, modo) {
    const n = this.nodi[ref] || {};
    const tipo = this.tipoDi(ref);
    const c = this.comp(ref);
    const proposto = modo === "proposto";
    const classi = ["dst-nodo", tipo || "sciolto"];
    if (proposto || (ref.startsWith("p:"))) classi.push("proposto");
    if (ref.startsWith("n:")) classi.push("nuovo");
    if (ref === this.sel) classi.push("sel");
    if (modo === "rimando") classi.push("rimando");
    const arco = padre ? this.archi.get(padre + "|" + ref) : null;
    let tag = null;
    if (proposto) tag = el("span", { class: "dst-stato-tag warn" }, "proposto");
    else if (ref.startsWith("n:")) tag = el("span", { class: "dst-stato-tag acc" }, "nuovo · da salvare");
    else if (arco && arco.prop) tag = el("span", { class: "dst-stato-tag acc" }, "accettato · da salvare");
    else if (ref.startsWith("p:")) tag = el("span", { class: "dst-stato-tag warn" }, "dallo STEP");
    else if (this.ritrovati.has(ref)) tag = el("span", { class: "dst-stato-tag acc" }, "ritrovato per codice");
    const figli = [];
    figli.push(el("span", { class: "dst-nodo-testa" }, el("span", { class: "dst-tipo " + (tipo || "sciolto") }, TIPI[tipo] || "Da decidere"), tag));
    figli.push(el("span", { class: "cod" }, this.nome(ref)));
    if (n.desc) figli.push(el("span", { class: "nome" }, n.desc));
    else if (n.nome && n.codice && n.nome !== n.codice) figli.push(el("span", { class: "nome" }, "nel file: " + n.nome));
    if (modo !== "rimando") {
      const pdf = c ? pdfDelComponente(c).filter((p) => p.ok) : [];
      if (pdf.length) {
        const nn = noteDelComponente(c);
        figli.push(el("button", { class: "dst-mini", type: "button", "data-a": pdf[0].a, title: "Apri il disegno " + pdf[0].nome, "aria-label": "Apri il disegno " + pdf[0].nome,
          onclick: (e) => { e.stopPropagation(); apriVisore(pdf[0].a, c, e.currentTarget); } },
          el("span", { class: "attesa" }, "…"), el("span", { class: "apri" }, "Apri il disegno"), nn ? el("span", { class: "note-mini" }, "✎ " + nn) : null));
      } else {
        const celle = (c && S.dati.celle && S.dati.celle[c]) || [];
        const c3d = celle.find((x) => x.e === "3D");
        const solo3d = c3d && c3d.c !== "urg" && c3d.c !== "no";
        const testo = !c ? (proposto || ref.startsWith("p:") ? "Il disegno si vede quando il pezzo è salvato" : "Pezzo nuovo: il disegno si associa dopo il salvataggio")
          : solo3d ? "Nessun disegno 2D · solo 3D" : "Nessun disegno 2D";
        const dove = tipo === "commerciale" ? "per un particolare commerciale non serve" : c ? "si associa in «Documenti e NAS»" : "";
        figli.push(el("div", { class: "dst-mini vuota" }, el("span", {}, testo), dove ? el("small", {}, dove) : null));
      }
      const celle = (c && S.dati.celle && S.dati.celle[c]) || [];
      if (celle.length) figli.push(el("span", { class: "docs" }, celle.map((x) => el("span", { class: "dst-cella " + x.c, title: x.e + ": " + x.t }, x.e + " " + x.s))));
    } else {
      figli.push(el("span", { class: "k" }, "↗ lo stesso pezzo è già disegnato sopra"));
    }
    if (proposto && this.scrive) {
      const p = this.proposteDi.get(padre + "|" + ref);
      figli.push(el("span", { class: "dst-nodo-azioni" },
        el("button", { type: "button", class: "dst-btn piccolo primario", onclick: (e) => { e.stopPropagation(); if (p) this.accetta(p); } }, "✓ Accetta"),
        ref.startsWith("p:") ? el("button", { type: "button", class: "dst-btn piccolo", onclick: (e) => { e.stopPropagation(); this.scartaNodo(ref); } }, "Scarta") : null));
    }
    const box = el("div", {
      class: classi.join(" "), tabindex: "0", role: "button", "data-ref": ref,
      "aria-label": (TIPI[tipo] || "Pezzo") + " " + this.nome(ref) + (proposto ? ", proposto" : ""),
      draggable: this.scrive && padre && !proposto ? "true" : "false",
    }, figli);
    box.addEventListener("click", () => { this.sel = ref; this.selPadre = padre || ""; this.conferma = null; this.disegna(); });
    box.addEventListener("keydown", (e) => { if (e.target === box && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); this.sel = ref; this.selPadre = padre || ""; this.conferma = null; this.disegna(); } });
    if (this.scrive) {
      box.addEventListener("dragstart", (e) => { this.trascinato = { ref, padre }; e.dataTransfer.setData("text/plain", ref); e.dataTransfer.effectAllowed = "move"; });
      box.addEventListener("dragend", () => { this.trascinato = null; for (const x of $$(".dst-nodo.drop, .dst-nodo.no-drop")) x.classList.remove("drop", "no-drop"); });
      box.addEventListener("dragover", (e) => {
        const t = this.trascinato;
        if (!t || t.ref === ref || proposto) return;
        // anche sopra una casella vietata il rilascio si accetta: sposta() lo rifiuta e dice perche'
        e.preventDefault();
        if (this.contenitore(ref) && !this.scendeDa(ref, t.ref)) box.classList.add("drop"); else box.classList.add("no-drop");
      });
      box.addEventListener("dragleave", () => box.classList.remove("drop", "no-drop"));
      box.addEventListener("drop", (e) => {
        e.preventDefault();
        box.classList.remove("drop", "no-drop");
        const t = this.trascinato;
        this.trascinato = null;
        if (t) this.sposta(t.padre, t.ref, ref);
      });
    }
    const out = el("div", { class: "dst-nodo-box" });
    if (padre) out.append(el("span", { class: "dst-qta-arco", title: "quantità sotto " + this.nome(padre) }, "×" + qta));
    out.append(box);
    return out;
  }

  disegnaAlbero() {
    const tree = document.getElementById("dst-tree");
    const aperti = new Set();
    const pendentiDi = new Map();
    for (const p of this.daMettere()) { if (!pendentiDi.has(p.padre)) pendentiDi.set(p.padre, []); pendentiDi.get(p.padre).push(p); }
    const giu = (ref, padre, qta, modo, cammino) => {
      const ripetuto = aperti.has(ref) || cammino.has(ref);
      const li = el("li", { class: modo === "proposto" ? "proposto-arco" : "" }, this.casella(ref, padre, qta, ripetuto && modo !== "proposto" ? "rimando" : modo));
      if (ripetuto || modo === "proposto") return li;
      aperti.add(ref);
      const sotto = new Set(cammino).add(ref);
      const figli = this.figliDi(ref).map((a) => giu(a.figlio, ref, a.qta, "", sotto));
      for (const p of pendentiDi.get(ref) || []) figli.push(giu(p.figlio, ref, p.qta, "proposto", sotto));
      if (figli.length) li.append(el("ul", {}, figli));
      return li;
    };
    tree.replaceChildren(el("ul", {}, giu(this.radice, "", 0, "", new Set())));
    const indietro = $("[data-azione=indietro]");
    if (indietro) indietro.disabled = !this.storia.length;
  }

  disegnaDettaglio() {
    const d = document.getElementById("dst-dettaglio");
    if (!d) return;
    const ref = this.nodi[this.sel] ? this.sel : this.radice;
    const n = this.nodi[ref] || {};
    const tipo = this.tipoDi(ref);
    const c = this.comp(ref);
    const radice = ref === this.radice;
    const padri = this.padriDi(ref);
    const inAlbero = this.raggiunti(this.radice).has(ref);
    const pendente = [...this.proposteDi.values()].find((p) => p.figlio === ref && !this.archi.has(p.padre + "|" + p.figlio) && this.raggiunti(this.radice).has(p.padre));
    const sin = el("div", { style: "display:grid;gap:10px;align-content:start" });
    const des = el("div", { style: "display:grid;gap:12px;align-content:start" });
    const campo = (et, input, aiuto) => el("label", { class: "dst-campo" }, el("span", {}, et), input, aiuto ? el("small", {}, aiuto) : null);

    // il codice
    if (ref.startsWith("p:") && n.senza_codice) {
      const inp = el("input", { class: "mono", maxlength: "40", placeholder: "il codice del pezzo" });
      inp.value = (this.codici[ref.slice(2)] || {}).codice || "";
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Codice (lo STEP non lo dice)"),
        el("span", { class: "dst-azioni" }, inp, el("button", { type: "button", class: "dst-btn piccolo", disabled: !this.scrive, onclick: () => this.scriviCodice(ref, inp.value) }, "Scrivi il codice"))));
    } else {
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Codice"), el("b", { class: "mono", style: "font-size:1.1rem" }, this.nome(ref)),
        c ? el("small", {}, "Un codice sbagliato si corregge nel Fascicolo completo (Azioni sul componente › Modifica).") : null));
    }
    // la descrizione (solo per un componente che c'e': e' un gesto al server)
    if (c) {
      const inp = el("input", { maxlength: "200", placeholder: "per esempio: Staffa laterale" });
      inp.value = n.desc || "";
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Denominazione"),
        el("span", { class: "dst-azioni" }, inp, this.scrive ? el("button", { type: "button", class: "dst-btn piccolo", onclick: () => this.gestoComponente("/modifica", { rev: n.rev || "", descrizione: inp.value.trim() }, "La denominazione non si è potuta salvare.") }, "Salva") : null)));
    } else if (n.desc) sin.append(campo("Denominazione", el("span", {}, n.desc)));
    // il tipo
    if (!radice) {
      const opzioni = ["sottoassieme", "sciolto", "commerciale"];
      const sel = el("select", { disabled: !this.scrive || ref.startsWith("p:") }, opzioni.map((t) => el("option", { value: t, selected: t === tipo }, TIPI[t])));
      if (ref.startsWith("p:") && !tipo) sel.prepend(el("option", { value: "", selected: true }, "Da decidere"));
      sel.addEventListener("change", () => {
        if (ref.startsWith("n:")) this.cambiaTipoNuovo(ref, sel.value);
        else if (c) this.anteprimaTipo(ref, sel.value);
      });
      sin.append(campo("Tipo", sel, ref.startsWith("p:") ? "Il tipo di un pezzo proposto si decide dopo averlo accettato e salvato." : "Solo il prodotto e gli assiemi possono avere pezzi sotto."));
    }
    // dove sta
    if (!radice && inAlbero) {
      const righe = el("div", { style: "display:grid;gap:8px" });
      for (const a of padri) {
        const q = el("input", { type: "number", min: "1", max: "100000", value: a.qta, style: "width:6rem", "aria-label": "quantità sotto " + this.nome(a.padre), disabled: !this.scrive });
        q.addEventListener("change", () => this.quantita(a.padre, ref, q.value));
        righe.append(el("div", { class: "dst-azioni" }, el("span", {}, "sotto ", el("b", { class: "mono" }, this.nome(a.padre)), " ×"), q,
          this.scrive ? el("button", { type: "button", class: "dst-btn piccolo", onclick: () => this.togli(a.padre, ref) }, "Togli da qui") : null));
      }
      if (this.scrive) {
        const dove = [...this.raggiunti(this.radice)].filter((r) => r !== ref && this.contenitore(r) && !this.scendeDa(r, ref) && !this.archi.has(r + "|" + ref));
        if (dove.length) {
          const s1 = el("select", { "aria-label": "sposta sotto" }, dove.map((r) => el("option", { value: r }, (TIPI[this.tipoDi(r)] || "Pezzo") + " " + this.nome(r))));
          const s2 = s1.cloneNode(true);
          const daPadre = padri.length === 1 ? padri[0].padre : (padri.find((a) => a.padre === this.selPadre) || padri[0] || {}).padre;
          righe.append(el("div", { class: "dst-azioni" }, el("span", {}, "Sposta sotto"), s1, el("button", { type: "button", class: "dst-btn piccolo", onclick: () => this.sposta(daPadre, ref, s1.value) }, "Sposta")));
          righe.append(el("div", { class: "dst-azioni" }, el("span", {}, "Anche sotto"), s2, el("button", { type: "button", class: "dst-btn piccolo", onclick: () => this.condividi(ref, s2.value) }, "Condividi")));
        }
      }
      sin.append(el("div", { class: "dst-campo" }, el("span", {}, "Dove sta nella distinta"), righe));
    }

    // da dove viene
    const fonti = [];
    if (radice) fonti.push(["acc", "Richiesta", "è il prodotto della richiesta"]);
    if (c && !radice && !ref.startsWith("n:")) fonti.push(["ok", "Distinta", "pezzo salvato nella distinta"]);
    if (ref.startsWith("n:")) fonti.push(["acc", "A mano", "scritto adesso: nasce con «Salva la distinta»"]);
    if (ref.startsWith("p:")) fonti.push(["warn", "STEP", "proposto da " + (n.file || "uno STEP") + (n.nota ? " · " + n.nota : "")]);
    for (const r of this.ritrovati.get(ref) || []) fonti.push(["neu", "STEP", "anche nello STEP " + r.file + ", con lo stesso codice"]);
    if (pendente) fonti.push(["warn", "Proposta", `lo STEP ${pendente.file} lo mette sotto ${this.nome(pendente.padre)} ×${pendente.qta}`]);
    const pdf = c ? pdfDelComponente(c) : [];
    for (const p of pdf.slice(0, 3)) fonti.push(["neu", "Disegno", p.nome + (p.stato === "doc" ? " · confermato" : " · da confermare")]);
    if (fonti.length) des.append(el("div", {}, el("span", { class: "dst-label" }, "Da dove viene"),
      el("ul", { class: "dst-pulito dst-fonti", style: "margin-top:6px" }, fonti.map(([cl, a, b]) => el("li", {}, el("span", { class: "dst-chip " + cl }, a), el("span", {}, b))))));

    // le azioni
    const az = el("div", { class: "dst-azioni" });
    const disponibili = pdf.filter((p) => p.ok);
    if (disponibili.length) az.append(el("button", { type: "button", class: "dst-btn", onclick: (e) => apriVisore(disponibili[0].a, c, e.currentTarget) }, "Apri il disegno" + (noteDelComponente(c) ? " (✎ " + noteDelComponente(c) + ")" : "")));
    if (this.scrive) {
      if (pendente) az.append(el("button", { type: "button", class: "dst-btn primario", onclick: () => this.accetta(pendente) }, "✓ Accetta la proposta"));
      if (ref.startsWith("p:")) az.append(el("button", { type: "button", class: "dst-btn", onclick: () => this.scartaNodo(ref) }, "Scarta: non è un pezzo"));
      if (!radice && inAlbero && padri.length) az.append(el("button", { type: "button", class: "dst-btn pericolo", onclick: () => { this.conferma = { tipo: "togli", ref }; this.disegnaDettaglio(); } }, ref.startsWith("n:") ? "Non crearlo" : "Togli dalla distinta"));
      if (radice && (this.d.guida || []).length) az.append(el("a", { class: "dst-btn", href: S.dati.base + "?vista=bom&nodo=" + c }, "Autorizza lo STEP del prodotto"));
    }
    if (az.childNodes.length) des.append(az);

    // i riquadri di conferma
    const cf = this.conferma;
    if (cf && cf.ref === ref && cf.tipo === "togli") {
      const sotto = [...this.raggiunti(ref)].filter((x) => x !== ref).length;
      des.append(el("div", { class: "dst-conferma-box" },
        el("span", {}, "Togliere ", el("b", { class: "mono" }, this.nome(ref)), " dalla distinta", sotto ? ` (con i ${sotto} pezzi che ha sotto)` : "", "? ",
          ref.startsWith("n:") ? "Non nasce." : "Va fra i pezzi fuori dalla distinta, in fondo: da lì si rimette o si elimina."),
        el("span", { class: "dst-azioni" },
          el("button", { type: "button", class: "dst-btn pericolo", onclick: () => { this.conferma = null; for (const a of this.padriDi(ref)) if (this.raggiunti(this.radice).has(a.padre)) this.togli(a.padre, ref); } }, "Sì, togli"),
          el("button", { type: "button", class: "dst-btn", onclick: () => { this.conferma = null; this.disegnaDettaglio(); } }, "Annulla"))));
    }
    if (cf && cf.ref === ref && cf.tipo === "tipo") {
      const e = cf.e;
      const box = el("div", { class: "dst-conferma-box warn" }, el("b", {}, `${this.nome(ref)}: ${TIPI[tipo] || tipo} → ${TIPI[cf.nuovo] || cf.nuovo}`));
      if (e.errore || e.spento) box.append(el("span", { class: "bad-t" }, e.errore || e.spento));
      for (const f of e.frasi || []) box.append(el("span", {}, f));
      box.append(el("span", { class: "dst-azioni" },
        !(e.errore || e.spento) ? el("button", { type: "button", class: "dst-btn primario", onclick: () => this.gestoComponente("/tipo", { tipo: cf.nuovo, firma: e.firma }, "Il tipo non si è potuto cambiare.") }, e.bottone || "Cambia il tipo") : null,
        el("button", { type: "button", class: "dst-btn", onclick: () => { this.conferma = null; this.disegna(); } }, "Annulla")));
      des.append(box);
    }

    d.replaceChildren(
      el("div", { class: "dst-riga-testa" }, el("span", { class: "dst-label" }, radice ? "Il prodotto" : ref.startsWith("p:") ? "Pezzo proposto dallo STEP" : ref.startsWith("n:") ? "Pezzo nuovo, da salvare" : "Pezzo scelto"),
        el("span", { class: "dst-tipo " + (tipo || "sciolto") }, TIPI[tipo] || "Da decidere"), el("b", { class: "mono" }, this.nome(ref)),
        el("span", { class: "sp" }), el("span", { class: "k" }, "clic su una casella per sceglierla")),
      el("div", { class: "dst-dettaglio-griglia" }, sin, des));
    d.className = "dst-box" + (ref.startsWith("p:") || pendente ? " tono-warn" : "");
  }

  disegnaVassoio() {
    const v = document.getElementById("dst-vassoio");
    if (!v) return;
    const fuori = this.vassoio();
    if (!fuori.length) { v.hidden = true; v.replaceChildren(); return; }
    v.hidden = false;
    const contenitori = [...this.raggiunti(this.radice)].filter((r) => this.contenitore(r));
    const carte = fuori.map((ref) => {
      const sel = el("select", { "aria-label": "metti sotto" }, contenitori.map((r) => el("option", { value: r }, (TIPI[this.tipoDi(r)] || "Pezzo") + " " + this.nome(r))));
      const c = this.comp(ref);
      return el("div", { class: "dst-box", style: "padding:10px;gap:6px;min-width:230px" },
        el("span", { class: "dst-azioni" }, el("span", { class: "dst-tipo " + (this.tipoDi(ref) || "sciolto") }, TIPI[this.tipoDi(ref)] || "Da decidere"), el("b", { class: "mono" }, this.nome(ref))),
        this.nodi[ref] && this.nodi[ref].desc ? el("span", { class: "k" }, this.nodi[ref].desc) : null,
        this.scrive ? el("span", { class: "dst-azioni" }, "Metti sotto", sel, el("button", { type: "button", class: "dst-btn piccolo", onclick: () => this.sposta("", ref, sel.value) }, "Metti")) : null,
        this.scrive && c ? el("span", { class: "dst-azioni" },
          el("button", { type: "button", class: "dst-btn piccolo pericolo", title: "Riesce solo se il pezzo non ha storia (documenti, note, proposte decise)", onclick: () => { this.sel = ref; this.gestoComponente("/rimuovi", {}, "Il pezzo non si è potuto eliminare: se ha dei documenti si archivia dal Fascicolo completo."); } }, "Elimina definitivamente")) : null);
    });
    v.replaceChildren(el("span", { class: "dst-label" }, `Fuori dalla distinta (${fuori.length})`),
      el("p", { class: "k" }, "Pezzi della richiesta che non stanno sotto il prodotto. Si rimettono sotto un assieme, oppure si eliminano."),
      el("div", { class: "dst-vassoio-lista" }, carte));
  }

  disegnaSalva() {
    const b = document.getElementById("dst-salva");
    if (!b) return;
    const n = this.cambiato() ? Math.max(1, this.modifiche()) : 0;
    b.hidden = !n;
    const nn = document.getElementById("dst-n-modifiche");
    if (nn) nn.textContent = n;
  }
}

// ------------------------------------------------------------------ la pagina

function avvia() {
  leggiDati();
  legaVisore();
  S.editor = null;
  const passo = ($(".dst-pannello") || {}).dataset ? $(".dst-pannello").dataset.passo : "";
  if (passo === "distinta") Distinta.avvia();
  const av = $(".dst-avviso");
  if (av) avvisa(av.textContent.trim(), av.dataset.esito === "no");
}

// i clic della pagina: disegni, strumenti dello schema
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
  if (nuovo && S.editor) { S.editor.formNuovo(nuovo.dataset.nuovo); return; }
  const az = e.target.closest("[data-azione]");
  if (!az || !S.editor) return;
  switch (az.dataset.azione) {
    case "accetta-tutto": S.editor.accettaTutto(); break;
    case "salva": S.editor.salva(); break;
    case "annulla-tutto": S.editor.annullaTutto(); break;
    case "indietro": S.editor.annullaUltima(); break;
    case "elimina": {
      const ed = S.editor;
      if (ed.sel === ed.radice) { avvisa("Il prodotto non si toglie: scegli prima una casella sotto di lui.", true); break; }
      ed.conferma = { tipo: "togli", ref: ed.sel };
      ed.disegnaDettaglio();
      document.getElementById("dst-dettaglio").scrollIntoView({ block: "nearest", behavior: "smooth" });
      break;
    }
  }
});

// un gesto del server (i moduli htmx del corpo) con lo schema non salvato rifarebbe la pagina e perderebbe le
// modifiche: prima si salvano o si annullano
document.addEventListener("htmx:beforeRequest", (e) => {
  const elt = e.detail && e.detail.elt;
  if (!elt || !S.editor || !S.editor.cambiato() || S.editor.salvando) return;
  if (!elt.closest || !elt.closest("#distinta")) return;
  e.preventDefault();
  avvisa("Prima salva o annulla le modifiche alla distinta.", true);
});
document.addEventListener("htmx:afterSwap", (e) => {
  const t = e.detail && e.detail.target;
  if (t && t.id === "distinta") avvia();
});
window.addEventListener("beforeunload", (e) => {
  if (S.editor && S.editor.cambiato() && !S.editor.salvando) { e.preventDefault(); e.returnValue = ""; }
});

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia);
else avvia();
