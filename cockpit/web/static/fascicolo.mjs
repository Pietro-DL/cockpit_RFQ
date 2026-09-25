// fascicolo.mjs — il Fascicolo v3 nel browser.
//
// Tre cose, tutte sopra htmx e senza toccare quello che htmx gia' fa:
//   - la vista Documenti: il disegno del componente scelto nello stage (pdf.js), il filmstrip con le
//     miniature, la scelta di un altro componente senza ricaricare la pagina, le note puntate sul disegno;
//   - la protezione di quello che l'operatore sta facendo: il poll non rifa' la vista mentre si scrive, i
//     collegamenti della pagina portano il componente scelto adesso e non quello di quando sono stati disegnati;
//   - l'editor della struttura: la BOM di un prodotto da trascinare, confermata in un colpo solo.
//
// Il server resta la verita': ogni gesto e' un POST che risponde con i pannelli rifatti; qui si tiene solo lo
// stato di chi guarda (quale componente, quale pagina, quanto zoom) e si rilegge tutto dopo ogni risposta.

const PDFJS = "/static/pdfjs-6.3.289/";
const $ = (sel, dentro) => (dentro || document).querySelector(sel);
const $$ = (sel, dentro) => Array.from((dentro || document).querySelectorAll(sel));

function prova(fn, altrimenti) {
  try { return fn(); } catch (e) { return altrimenti; }
}
function memoria(chiave, valore) {
  // localStorage puo' mancare o essere pieno (la cache di htmx): e' solo una comodita'
  if (valore === undefined) return prova(() => localStorage.getItem(chiave), null);
  prova(() => (valore === null ? localStorage.removeItem(chiave) : localStorage.setItem(chiave, valore)));
}
function sessione(chiave, valore) {
  if (valore === undefined) return prova(() => sessionStorage.getItem(chiave), null);
  prova(() => (valore === null ? sessionStorage.removeItem(chiave) : sessionStorage.setItem(chiave, valore)));
}
function el(tag, attr, ...figli) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attr || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k === "testo") e.textContent = v;
    else if (k.startsWith("on") && typeof v === "function") e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? "" : String(v));
  }
  for (const f of figli.flat()) {
    if (f === null || f === undefined || f === false) continue;
    e.append(f instanceof Node ? f : document.createTextNode(String(f)));
  }
  return e;
}
function inCampo(t) {
  if (!t || !t.tagName) return false;
  const n = t.tagName.toLowerCase();
  return n === "input" || n === "textarea" || n === "select" || t.isContentEditable;
}

// ------------------------------------------------------------------ pdf.js, caricato quando serve

let pdfjsPromessa = null;
function pdfjs() {
  if (!pdfjsPromessa) {
    pdfjsPromessa = import(PDFJS + "pdf.min.mjs")
      .then((lib) => {
        lib.GlobalWorkerOptions.workerSrc = PDFJS + "pdf.worker.min.mjs";
        return lib;
      })
      .catch(() => null); // senza pdf.js il disegno si apre nel visualizzatore del browser
  }
  return pdfjsPromessa;
}
function opzioniPdf(url, extra) {
  return Object.assign({
    url,
    isEvalSupported: false,
    enableXfa: false,
    wasmUrl: PDFJS + "wasm/",
    iccUrl: PDFJS + "iccs/",
    cMapUrl: PDFJS + "cmaps/",
    cMapPacked: true,
    standardFontDataUrl: PDFJS + "standard_fonts/",
    verbosity: 0,
  }, extra || {});
}
function motivoErrore(err) {
  const s = err && (err.status || (err.details && err.details.status));
  switch (s) {
    case 404: return "Il file non è su questo server: si riscarica («Riscarica» nel pannello).";
    case 409: return "Il file sul NAS non corrisponde al documento: va visto in Integrità NAS.";
    case 415: return "Non è un PDF.";
    case 503: return "Il NAS non risponde: si riprova fra poco.";
    case 401: case 403: return "La sessione è scaduta: si ricarica la pagina.";
  }
  if (err && err.name === "InvalidPDFException") return "Il PDF non si legge (è rovinato, o la sessione è scaduta).";
  if (err && err.name === "PasswordException") return "Il PDF è protetto da una password.";
  return "Il disegno non si è potuto aprire.";
}

// ------------------------------------------------------------------ lo stato di chi guarda

const S = {
  radice: null,
  base: "",
  scrive: false,
  bloccata: 0,
  indice: { elementi: [], note: {}, prec: {} },
  scelto: "",
  file: "",
  fileDi: {},       // elemento → ultimo file guardato
  pagina: 1,
  pagine: 0,
  zoom: 1,          // 1 = adatta il foglio allo stage
  zoomDi: {},       // allegato → zoom
  doc: null, docA: "",
  docs: new Map(),  // allegato → {task, promessa} di pdf.js (gli ultimi aperti)
  falliti: new Map(), // allegato → perche' non si e' aperto (si riprova solo con un gesto di chi guarda)
  apertura: null,   // la promessa dell'apertura in corso
  prec: "",         // la revisione precedente aperta con «Vedi», finche' non si sceglie altro
  sceltoServer: "", // l'elemento scelto dall'ultima risposta del server
  gen: 0,           // quante volte chi guarda ha scelto qui: una risposta chiesta prima di una scelta nuova non la tocca
  genRisposta: -1,  // la generazione della scelta quando e' partita la richiesta della risposta che sta entrando
  rispostaSelezione: false, // la risposta e' una navigazione (una linguetta, un gruppo): la sua scelta vale
  task: null, tok: 0,
  armato: false,
  pop: null,
  mini: new Map(),  // allegato → data URL della miniatura
  miniInCorso: new Set(), miniFallite: new Map(), // allegato → quando la miniatura non e' riuscita
  scroll: {},
  sezioneFetch: null,
  rispostaDelPoll: false,
};

function vistaDocumenti() { return document.getElementById("doc-vista"); }
function stage() { return document.getElementById("doc-stage"); }
function elemento(k) { return (S.indice.elementi || []).find((e) => e.k === k) || null; }
function fileDellElemento(k, a) {
  const e = elemento(k);
  return e ? (e.file || []).find((f) => f.a === a) || null : null;
}
function fileIniziale(e) {
  if (!e || !e.file || !e.file.length) return "";
  const ricordato = S.fileDi[e.k];
  if (ricordato && e.file.some((f) => f.a === ricordato)) return ricordato;
  const ok = e.file.find((f) => f.ok);
  return (ok || e.file[0]).a;
}

// ------------------------------------------------------------------ avvio e sincronizzazione

function avvia() {
  S.radice = document.getElementById("fascicolo");
  if (!S.radice) return;
  S.base = S.radice.dataset.base || "";
  S.scrive = S.radice.dataset.scrive === "1";
  S.bloccata = parseInt(S.radice.dataset.bloccata || "0", 10) || 0;
  if (memoria("cockpit.fascicolo.intero") === "1") document.body.classList.add("fasc-intero");
  document.addEventListener("click", clic, true);
  document.addEventListener("keydown", tasto);
  document.addEventListener("input", scrittura);
  document.body.addEventListener("htmx:configRequest", configuraRichiesta);
  document.body.addEventListener("htmx:oobBeforeSwap", primaDelloScambioFuoriBanda);
  document.body.addEventListener("htmx:beforeSwap", salvaScroll);
  document.body.addEventListener("htmx:beforeSwap", segnaRisposta);
  document.body.addEventListener("htmx:beforeRequest", (e) => { const rc = e.detail && e.detail.requestConfig; if (rc) rc.genScelta = S.gen; });
  document.body.addEventListener("htmx:afterSwap", () => { S.rispostaDelPoll = false; });
  document.body.addEventListener("htmx:afterSettle", () => sincronizza());
  document.body.addEventListener("htmx:historyRestore", () => { S.scelto = ""; S.file = ""; S.sceltoServer = ""; S.prec = ""; sincronizza(); });
  document.body.addEventListener("bom-esito", esitoEditor);
  window.addEventListener("resize", () => { clearTimeout(S.tResize); S.tResize = setTimeout(() => disegna(), 180); });
  window.addEventListener("beforeunload", (e) => { if (Editor.corrente && Editor.corrente.cambiato()) { e.preventDefault(); e.returnValue = ""; } });
  sincronizza();
  const q = new URLSearchParams(location.search);
  if (q.has("editor")) {
    const p = q.get("editor"), step = q.get("step") || "";
    q.delete("editor"); q.delete("step");
    history.replaceState(history.state, "", location.pathname + (q.toString() ? "?" + q : ""));
    if (S.scrive && !S.bloccata) Editor.apri(p, step);
  }
}

// sincronizza rilegge la pagina dopo ogni risposta: l'indice del viewer, la scelta, lo stage, le miniature.
function sincronizza() {
  // dopo un ritorno indietro htmx rifa' il corpo della pagina: la radice di prima non c'e' piu'
  S.radice = document.getElementById("fascicolo") || S.radice;
  const v = vistaDocumenti();
  if (!v) { chiudiPop(); return; }
  const j = $("#doc-indice", v);
  if (j) {
    const nuovo = prova(() => JSON.parse(j.textContent), null);
    if (nuovo) S.indice = nuovo;
  }
  if (!S.indice.note) S.indice.note = {};
  if (!S.indice.prec) S.indice.prec = {};
  // la scelta: se la risposta ha cambiato la sua (una linguetta, un gruppo, un collegamento con il suo nodo),
  // vale quella; altrimenti quella di adesso, se c'e' ancora; altrimenti quella dell'indirizzo o del server
  const q = new URLSearchParams(location.search);
  const srv = S.indice.scelto || "";
  let k = "", adottata = false;
  // vale la scelta del server se la risposta e' stata chiesta dopo l'ultima scelta fatta qui, e se e' una
  // navigazione o se il server ha cambiato idea; una risposta vecchia non riporta indietro chi guarda
  if (srv && S.genRisposta === S.gen && (S.rispostaSelezione || srv !== S.sceltoServer) && srv !== S.scelto && elemento(srv)) {
    k = srv; S.file = ""; S.prec = ""; adottata = true;
  }
  S.sceltoServer = srv;
  S.rispostaSelezione = false;
  S.genRisposta = -1;
  if (!k) k = S.scelto && elemento(S.scelto) ? S.scelto : "";
  if (!k) {
    if (q.get("nodo") && elemento("c:" + q.get("nodo"))) k = "c:" + q.get("nodo");
    else if (q.get("file") && elemento("f:" + q.get("file"))) k = "f:" + q.get("file");
    else k = S.indice.scelto || "";
  }
  S.scelto = k;
  const e = elemento(k);
  let a = S.file && e && e.file.some((f) => f.a === S.file) ? S.file : "";
  if (!a && q.get("file") && e && e.file.some((f) => f.a === q.get("file"))) a = q.get("file");
  if (!a) a = S.indice.file && e && e.file.some((f) => f.a === S.indice.file) ? S.indice.file : fileIniziale(e);
  S.file = a;
  // l'indirizzo dice quello che si vede
  const qn = q.get("nodo") ? "c:" + q.get("nodo") : "";
  if (adottata || (k.startsWith("c:") && qn !== k) || (a && q.get("file") !== a)) scriviIndirizzo();
  evidenzia();
  const sez = $(".docv-sez", v);
  if (k && (!sez || sez.dataset.k !== k)) chiediSezione();
  segnaFileAperto();
  montaStage();
  mostra();
  osservaMiniature();
  ripristinaScroll();
  filtra($(".docv-filtro", v) ? $(".docv-filtro", v).value : "");
}

function evidenzia() {
  for (const r of $$(".docv-riga[data-k], .docv-tile[data-k]")) {
    const s = r.dataset.k === S.scelto;
    r.classList.toggle("sel", s);
    if (s) r.setAttribute("aria-current", "true"); else r.removeAttribute("aria-current");
  }
  mostraDentro(document.getElementById("doc-albero"), $(`.docv-riga[data-k="${CSS.escape(S.scelto)}"]`));
  mostraDentro(document.getElementById("doc-film"), $(`.docv-tile[data-k="${CSS.escape(S.scelto)}"]`));
}

// mostraDentro porta un elemento in vista scorrendo solo il suo contenitore: scrollIntoView scorrerebbe anche
// i contenitori intorno (e quelli con overflow nascosto non tornano piu' indietro da soli).
function mostraDentro(c, e) {
  if (!c || !e) return;
  const rc = c.getBoundingClientRect(), r = e.getBoundingClientRect();
  if (r.left < rc.left) c.scrollLeft -= rc.left - r.left + 8;
  else if (r.right > rc.right) c.scrollLeft += r.right - rc.right + 8;
  if (r.top < rc.top) c.scrollTop -= rc.top - r.top + 4;
  else if (r.bottom > rc.bottom) c.scrollTop += r.bottom - rc.bottom + 4;
}

function segnaFileAperto() {
  for (const f of $$(".docv-file[data-a]")) f.classList.toggle("aperto", f.dataset.a === S.file);
}

// scegli apre un altro elemento (componente o file) senza ricaricare la pagina.
function scegli(k, a) {
  const e = elemento(k);
  if (!e) return false;
  S.gen++;
  S.prec = "";
  S.scelto = k;
  S.file = a && e.file.some((f) => f.a === a) ? a : fileIniziale(e);
  if (S.file) S.fileDi[k] = S.file;
  scriviIndirizzo();
  evidenzia();
  chiediSezione();
  mostra(true);
  return true;
}

function scriviIndirizzo() {
  const q = new URLSearchParams(location.search);
  q.delete("nodo"); q.delete("file"); q.delete("doc"); q.delete("scheda"); q.delete("prop");
  // il gruppo aperto: un componente condiviso da due prodotti resta in quello in cui lo si guarda
  if (S.indice.gruppo) q.set("gruppo", S.indice.gruppo);
  if (S.scelto.startsWith("c:")) q.set("nodo", S.scelto.slice(2));
  if (S.file) q.set("file", S.file);
  const url = location.pathname + (q.toString() ? "?" + q : "");
  if (url !== location.pathname + location.search) history.replaceState(history.state, "", url);
}

// chiediSezione porta il pannello dell'elemento scelto; se nel frattempo se ne sceglie un altro, la
// risposta vecchia si butta.
function chiediSezione() {
  const dove = document.getElementById("doc-sezione");
  if (!dove || !S.scelto) return;
  if (S.sezioneFetch) S.sezioneFetch.abort();
  const ac = new AbortController();
  S.sezioneFetch = ac;
  const k = S.scelto;
  clearTimeout(S.tSezione);
  S.tSezione = setTimeout(() => {
    fetch(S.base + "/sezione" + location.search, { signal: ac.signal, credentials: "same-origin", headers: { "HX-Request": "true" } })
      .then((r) => (r.ok ? r.text() : Promise.reject(r.status)))
      .then((html) => {
        if (ac.signal.aborted || k !== S.scelto) return;
        const tieni = dove.scrollTop;
        dove.innerHTML = html;
        if (window.htmx) htmx.process(dove);
        dove.scrollTop = tieni;
        segnaFileAperto();
      })
      .catch(() => {});
  }, 90);
}

// ------------------------------------------------------------------ lo stage

function montaStage() {
  const st = stage();
  if (!st || st.dataset.pronto) return;
  st.dataset.pronto = "1";
  S.docA = ""; S.doc = null; S.pop = null; S.armato = false; // uno stage nuovo: il disegno si riapre
  st.innerHTML = "";
  const barra = el("div", { class: "ds-barra" },
    el("button", { type: "button", class: "ds-prec", title: "Componente precedente (←)", "aria-label": "Componente precedente" }, "◀"),
    el("span", { class: "ds-pos k" }),
    el("button", { type: "button", class: "ds-succ", title: "Componente successivo (→)", "aria-label": "Componente successivo" }, "▶"),
    el("span", { class: "ds-cod" }),
    el("span", { class: "ds-desc" }),
    el("select", { class: "ds-file", "aria-label": "File del componente" }),
    el("span", { class: "ds-sp" }),
    el("button", { type: "button", class: "ds-pag-prec", title: "Pagina precedente (PagSu)", "aria-label": "Pagina precedente" }, "‹"),
    el("span", { class: "ds-pagine k" }),
    el("button", { type: "button", class: "ds-pag-succ", title: "Pagina successiva (PagGiù)", "aria-label": "Pagina successiva" }, "›"),
    el("button", { type: "button", class: "ds-meno", title: "Riduci (−)", "aria-label": "Riduci" }, "−"),
    el("button", { type: "button", class: "ds-adatta", title: "Adatta il foglio (0)" }, "Adatta"),
    el("button", { type: "button", class: "ds-piu", title: "Ingrandisci (+)", "aria-label": "Ingrandisci" }, "+"),
    S.scrive ? el("button", { type: "button", class: "ds-arma", title: "Poi un clic sul punto del disegno (N)" }, "✎ Aggiungi nota") : null,
    el("button", { type: "button", class: "ds-intero", title: "Schermo intero" }, "⛶"));
  const area = el("div", { class: "ds-area", tabindex: "0", "aria-label": "Disegno" },
    el("div", { class: "ds-pagina", hidden: true }, el("canvas", { class: "ds-canvas" }), el("div", { class: "ds-pins" })));
  st.append(barra, area, el("div", { class: "ds-hint", hidden: true }));
  $(".ds-prec", st).addEventListener("click", () => passo(-1));
  $(".ds-succ", st).addEventListener("click", () => passo(1));
  $(".ds-file", st).addEventListener("change", (e) => { S.prec = ""; S.file = e.target.value; S.fileDi[S.scelto] = S.file; scriviIndirizzo(); segnaFileAperto(); mostra(true); });
  $(".ds-pag-prec", st).addEventListener("click", () => pagina(-1));
  $(".ds-pag-succ", st).addEventListener("click", () => pagina(1));
  $(".ds-meno", st).addEventListener("click", () => zoom(1 / 1.25));
  $(".ds-piu", st).addEventListener("click", () => zoom(1.25));
  $(".ds-adatta", st).addEventListener("click", () => { S.zoom = 1; disegna(); });
  $(".ds-intero", st).addEventListener("click", schermoIntero);
  if ($(".ds-arma", st)) $(".ds-arma", st).addEventListener("click", () => arma(!S.armato));
  area.addEventListener("wheel", (e) => {
    if (!e.ctrlKey) return; // la rotella scorre il foglio; con Ctrl ingrandisce
    e.preventDefault();
    zoom(e.deltaY < 0 ? 1.15 : 1 / 1.15, e);
  }, { passive: false });
  area.addEventListener("click", clicSulFoglio);
}

function schermoIntero() {
  const on = !document.body.classList.contains("fasc-intero");
  document.body.classList.toggle("fasc-intero", on);
  memoria("cockpit.fascicolo.intero", on ? "1" : null);
  setTimeout(() => disegna(), 60);
}

// mostra aggiorna lo stage per l'elemento e il file scelti. nuovo = e' cambiata la scelta.
function mostra(nuovo) {
  const st = stage();
  if (!st || !st.dataset.pronto) return;
  const e = elemento(S.scelto);
  const idx = (S.indice.elementi || []).findIndex((x) => x.k === S.scelto);
  $(".ds-pos", st).textContent = idx >= 0 ? `${idx + 1}/${S.indice.elementi.length}` : "";
  $(".ds-cod", st).textContent = e ? e.codice : "";
  $(".ds-desc", st).textContent = e && e.desc ? e.desc : "";
  const sel = $(".ds-file", st);
  sel.innerHTML = "";
  for (const f of (e && e.file) || []) {
    sel.append(el("option", { value: f.a, selected: f.a === S.file }, `${f.tipo ? f.tipo + " · " : ""}${f.nome}${f.stato === "proposta" ? " (da confermare)" : ""}`));
  }
  sel.hidden = !(e && e.file && e.file.length > 1);
  const f = e ? fileDellElemento(e.k, S.file) : null;
  if (f && !f.ok) S.falliti.delete(f.a); // il file non c'e' piu': quando torna, si riprova
  if (!f) { S.prec = ""; vuoto("Disegno non presente", e && e.k.startsWith("c:") ? "Nessun file per " + e.codice + ": nessun documento confermato e nessun file in arrivo con il suo codice." : ""); return; }
  if (!f.pdf) { S.prec = ""; vuoto("Anteprima non disponibile per questo formato", f.nome + (f.tipo ? " · " + f.tipo : "") + ": la struttura di uno STEP si vede nella Struttura BOM."); return; }
  if (!f.ok) { S.prec = ""; vuoto("Il PDF non è ancora su questo server", f.nome + ": si scarica con la preparazione, oppure con «Riscarica»."); return; }
  // «Vedi»: la revisione precedente di questo file resta aperta finche' non si sceglie altro
  const pr = S.indice.prec && S.indice.prec[f.a];
  if (S.prec && !(pr && pr.a === S.prec)) S.prec = "";
  const a = S.prec || f.a;
  $(".ds-desc", st).textContent = S.prec ? "revisione precedente: " + (pr.nome || "") : (e && e.desc ? e.desc : "");
  const fl = S.falliti.get(a);
  // un gesto di chi guarda riprova; un NAS che non rispondeva si riprova da solo dopo un po'
  if (fl && (nuovo || (fl.transitorio && Date.now() - fl.quando > 20000))) S.falliti.delete(a);
  if (S.falliti.has(a)) { // gia' provato: si dice perche', senza chiederlo di nuovo al server a ogni risposta
    const gia = $(".ds-area .ds-vuoto", st);
    if (!gia || gia.dataset.a !== a) { vuoto("Il disegno non si apre", S.falliti.get(a).motivo); const v = $(".ds-area .ds-vuoto", st); if (v) v.dataset.a = a; }
    return;
  }
  if (nuovo || S.docA !== a) S.apertura = apriFile(a);
  else disegnaNote();
}

function vuoto(titolo, testo) {
  const st = stage();
  S.tok++;
  S.doc = null; S.docA = ""; S.pagine = 0;
  arma(false);
  chiudiPop();
  const area = $(".ds-area", st);
  $$(".ds-vuoto, .ds-iframe", area).forEach((x) => x.remove());
  $(".ds-pagina", area).hidden = true;
  area.append(el("div", { class: "ds-vuoto" }, el("b", { testo: titolo }), testo));
  aggiornaBarra();
}

async function apriFile(a) {
  const tok = ++S.tok;
  chiudiPop();
  const st = stage();
  const area = $(".ds-area", st);
  // mentre il file nuovo si apre il vecchio non c'e' piu': zoom, pagine e note non lo toccano, e una nota
  // non finisce sul file di prima con il componente di adesso
  const primaA = S.docA;
  S.doc = null; S.docA = ""; S.pagine = 0;
  arma(false);
  $$(".ds-vuoto, .ds-iframe", area).forEach((x) => x.remove());
  $(".ds-pagina", area).hidden = true;
  disegnaNote();
  area.append(el("div", { class: "ds-vuoto ds-carica" }, "Apertura del disegno…"));
  aggiornaBarra();
  const lib = await pdfjs();
  if (tok !== S.tok) return;
  if (!lib) {
    // senza pdf.js: il visualizzatore del browser, senza note
    $$(".ds-vuoto", area).forEach((x) => x.remove());
    $(".ds-pagina", area).hidden = true;
    area.append(el("iframe", { class: "ds-iframe", src: "/allegato/" + a + "/anteprima", title: "Disegno" }));
    S.docA = a; S.doc = null;
    aggiornaBarra();
    return;
  }
  let voce = S.docs.get(a);
  if (!voce) {
    const task = lib.getDocument(opzioniPdf("/allegato/" + a + "/anteprima"));
    voce = { task, promessa: task.promise };
    S.docs.set(a, voce);
    while (S.docs.size > 6) {
      const [vecchio, v] = S.docs.entries().next().value;
      S.docs.delete(vecchio);
      v.task.destroy().catch(() => {}); // chiude il documento e il suo lavoratore (la sua attesa non e' un errore)
    }
  } else { // l'ultimo usato va in fondo
    S.docs.delete(a); S.docs.set(a, voce);
  }
  let doc;
  try { doc = await voce.promessa; } catch (err) {
    if (S.docs.get(a) !== voce) return; // chiuso perche' uscito dagli ultimi aperti: non e' un errore del file
    S.docs.delete(a);
    voce.task.destroy().catch(() => {});
    const motivo = motivoErrore(err);
    const st = err && (err.status || (err.details && err.details.status));
    S.falliti.set(a, { motivo, transitorio: !st || st === 503, quando: Date.now() });
    if (tok === S.tok) vuoto("Il disegno non si apre", motivo);
    return;
  }
  if (tok !== S.tok) return;
  if (primaA !== a) { S.pagina = 1; S.zoom = S.zoomDi[a] || 1; }
  S.doc = doc; S.docA = a; S.pagine = doc.numPages;
  if (S.pagina > S.pagine) S.pagina = 1;
  await disegna();
}

function aggiornaBarra() {
  const st = stage();
  if (!st || !st.dataset.pronto) return;
  $(".ds-pagine", st).textContent = S.pagine ? `pag. ${S.pagina}/${S.pagine}` : "";
  $(".ds-pag-prec", st).disabled = !S.doc || S.pagina <= 1;
  $(".ds-pag-succ", st).disabled = !S.doc || S.pagina >= S.pagine;
  for (const b of $$(".ds-meno, .ds-piu, .ds-adatta", st)) b.disabled = !S.doc;
  const arm = $(".ds-arma", st);
  if (arm) { arm.disabled = !S.doc; arm.classList.toggle("on", S.armato); }
  $(".ds-area", st).classList.toggle("armato", S.armato);
  const hint = $(".ds-hint", st);
  hint.hidden = !S.armato;
  hint.textContent = S.armato ? "Clic sul punto del disegno dove va la nota · Esc annulla" : "";
}

async function disegna(ancora) {
  const st = stage();
  if (!st || !S.doc) { aggiornaBarra(); return; }
  const tok = ++S.tok;
  const lib = await pdfjs();
  const area = $(".ds-area", st);
  const foglio = $(".ds-pagina", area);
  const canvas = $(".ds-canvas", foglio);
  let page;
  try { page = await S.doc.getPage(S.pagina); } catch (err) { if (tok === S.tok) vuoto("La pagina non si legge", motivoErrore(err)); return; }
  if (tok !== S.tok) return;
  const vp1 = page.getViewport({ scale: 1 });
  const larg = Math.max(200, area.clientWidth - 24), alt = Math.max(200, area.clientHeight - 24);
  const adatta = Math.min(larg / vp1.width, alt / vp1.height);
  const scala = adatta * S.zoom;
  const dpr = Math.min(window.devicePixelRatio || 1, 2);
  // un foglio A0 ingrandito supera quello che un canvas regge: oltre sedici milioni di pixel si rende meno fitto
  const maxPix = 16e6;
  let pix = scala * dpr;
  if (vp1.width * vp1.height * pix * pix > maxPix) pix = Math.sqrt(maxPix / (vp1.width * vp1.height));
  const vp = page.getViewport({ scale: pix });
  const cssL = Math.round(vp1.width * scala), cssA = Math.round(vp1.height * scala);
  const prima = ancora ? posizioneRelativa(area, foglio, ancora) : null;
  if (S.task) { try { S.task.cancel(); } catch (e) { /* gia' finito */ } }
  const nuovo = document.createElement("canvas");
  nuovo.className = "ds-canvas";
  nuovo.width = Math.floor(vp.width); nuovo.height = Math.floor(vp.height);
  const task = page.render({ canvas: nuovo, viewport: vp, annotationMode: lib ? lib.AnnotationMode.DISABLE : 0 });
  S.task = task;
  try { await task.promise; } catch (err) {
    if (err && err.name === "RenderingCancelledException") return;
    if (tok === S.tok) vuoto("La pagina non si disegna", motivoErrore(err));
    return;
  }
  if (tok !== S.tok) return;
  canvas.replaceWith(nuovo);
  foglio.style.width = cssL + "px";
  foglio.style.height = cssA + "px";
  foglio.hidden = false;
  $$(".ds-vuoto, .ds-iframe", area).forEach((x) => x.remove());
  if (prima) ripristinaPosizione(area, foglio, prima);
  aggiornaBarra();
  disegnaNote();
}

function posizioneRelativa(area, foglio, e) {
  const r = foglio.getBoundingClientRect();
  return { fx: (e.clientX - r.left) / r.width, fy: (e.clientY - r.top) / r.height, cx: e.clientX, cy: e.clientY };
}
function ripristinaPosizione(area, foglio, p) {
  const r = foglio.getBoundingClientRect();
  area.scrollLeft += (r.left + p.fx * r.width) - p.cx;
  area.scrollTop += (r.top + p.fy * r.height) - p.cy;
}

function zoom(fattore, e) {
  if (!S.doc) return;
  S.zoom = Math.min(8, Math.max(1, S.zoom * fattore));
  if (Math.abs(S.zoom - 1) < 0.02) S.zoom = 1;
  S.zoomDi[S.docA] = S.zoom;
  clearTimeout(S.tZoom);
  S.tZoom = setTimeout(() => disegna(e ? { clientX: e.clientX, clientY: e.clientY } : null), 60);
}

function pagina(d) {
  if (!S.doc) return;
  const n = Math.min(S.pagine, Math.max(1, S.pagina + d));
  if (n === S.pagina) return;
  S.pagina = n;
  chiudiPop();
  disegna();
}

function passo(d) {
  const el = S.indice.elementi || [];
  if (!el.length) return;
  const visibili = el.filter((x) => {
    const t = $(`.docv-tile[data-k="${CSS.escape(x.k)}"]`);
    return !t || !t.hidden;
  });
  const lista = visibili.length ? visibili : el;
  let i = lista.findIndex((x) => x.k === S.scelto);
  i = i < 0 ? 0 : (i + d + lista.length) % lista.length;
  clearTimeout(S.tPasso);
  S.gen++;
  S.scelto = lista[i].k;
  evidenzia();
  // tenendo premuto il tasto si scorre la lista: il disegno si apre quando ci si ferma
  S.tPasso = setTimeout(() => scegli(lista[i].k), 150);
}

// ------------------------------------------------------------------ le note sul disegno

function noteDelFile(a) {
  return (S.indice.note && S.indice.note[a]) || [];
}
function componenteScelto() {
  return S.scelto.startsWith("c:") ? S.scelto.slice(2) : "";
}

function disegnaNote() {
  const st = stage();
  if (!st) return;
  const strato = $(".ds-pins", st);
  if (!strato) return;
  strato.innerHTML = "";
  if (!S.doc) return;
  for (const n of noteDelFile(S.docA)) {
    if (n.p !== S.pagina) continue;
    const b = el("button", { type: "button", class: "ds-pin", "data-nota": n.id, title: n.t, "aria-label": `Nota ${n.n}: ${n.t}`, testo: String(n.n) });
    b.style.left = (n.x * 100) + "%";
    b.style.top = (n.y * 100) + "%";
    b.addEventListener("click", (e) => { e.stopPropagation(); arma(false); apriNota(n, b); });
    strato.append(b);
  }
  if (S.tmp) {
    const t = el("div", { class: "ds-pin tmp", testo: "+" });
    t.style.left = (S.tmp.x * 100) + "%";
    t.style.top = (S.tmp.y * 100) + "%";
    strato.append(t);
  }
}

function arma(on) {
  if (on && (!S.scrive || !S.doc)) return;
  S.armato = !!on;
  if (!on) S.tmp = null;
  aggiornaBarra();
}

function clicSulFoglio(e) {
  if (S.pop && !S.pop.contains(e.target)) { chiudiPop(); return; }
  if (!S.armato) return;
  const foglio = $(".ds-pagina", stage());
  if (!foglio || foglio.hidden) return;
  const r = foglio.getBoundingClientRect();
  const x = (e.clientX - r.left) / r.width, y = (e.clientY - r.top) / r.height;
  if (!(x >= 0 && x <= 1 && y >= 0 && y <= 1)) return; // fuori dal foglio
  S.tmp = { x, y };
  disegnaNote();
  const pagina = S.pagina, allegato = S.docA, comp = componenteScelto();
  const testo = el("textarea", { "aria-label": "Testo della nota", placeholder: "Es. tolleranza ±0,2 da verificare, piega critica, cordone di saldatura…", maxlength: "2000" });
  const errore = el("div", { class: "ds-pop-errore", hidden: true });
  const salva = el("button", { type: "button", class: "btn small primary" }, "Salva nota");
  const annulla = el("button", { type: "button", class: "btn small" }, "Annulla");
  const pop = el("div", { class: "ds-pop", role: "dialog", "aria-label": "Nuova nota" },
    el("div", { class: "k" }, `Nuova nota · pagina ${pagina}`), testo, errore, el("div", { class: "ds-pop-azioni" }, annulla, salva));
  mettiPop(pop, e.clientX, e.clientY);
  testo.focus();
  annulla.addEventListener("click", () => { chiudiPop(); arma(false); });
  testo.addEventListener("keydown", (ev) => { if (ev.key === "Enter" && (ev.ctrlKey || ev.metaKey)) salva.click(); });
  salva.addEventListener("click", async () => {
    const t = testo.value.trim();
    if (!t) { testo.focus(); return; }
    salva.disabled = true;
    const esito = await gesto(S.base + "/nota", { componente: comp, allegato, pagina: String(pagina), x: x.toFixed(6), y: y.toFixed(6), testo: t });
    if (!esito.ok) { // la nota non c'e': il testo resta, e si dice perche'
      salva.disabled = false;
      errore.hidden = false;
      errore.textContent = esito.testo || "La nota non si è salvata.";
      return;
    }
    S.tmp = null;
    chiudiPop();
    arma(false);
  });
}

function apriNota(n, ancora) {
  chiudiPop();
  const r = ancora.getBoundingClientRect();
  const figli = [el("div", { class: "k" }, `Nota ${n.n} · ${n.chi} · ${n.quando}${n.su ? " · scritta su " + n.su : ""}`), el("div", { class: "ds-pop-testo", testo: n.t })];
  const azioni = el("div", { class: "ds-pop-azioni" });
  if (n.mia && S.scrive) {
    const cambia = el("button", { type: "button", class: "btn small" }, "Cambia");
    const togli = el("button", { type: "button", class: "btn small" }, "Togli");
    azioni.append(cambia, togli);
    cambia.addEventListener("click", () => cambiaNota(n, r));
    togli.addEventListener("click", async () => {
      togli.disabled = true;
      const esito = await gesto(S.base + "/nota/" + n.id + "/elimina", {});
      if (esito.ok) chiudiPop(); else { togli.disabled = false; mostraErrorePop(esito.testo); }
    });
  }
  azioni.append(el("button", { type: "button", class: "btn small", onclick: () => chiudiPop() }, "Chiudi"));
  const pop = el("div", { class: "ds-pop", role: "dialog", "aria-label": "Nota " + n.n }, ...figli, el("div", { class: "ds-pop-errore", hidden: true }), azioni);
  mettiPop(pop, r.right, r.bottom);
  ancora.classList.add("acceso");
  for (const x of $$(".docv-nota")) x.classList.toggle("acceso", x.dataset.nota === n.id);
}

function cambiaNota(n, r) {
  chiudiPop();
  const testo = el("textarea", { "aria-label": "Testo della nota", maxlength: "2000" });
  testo.value = n.t;
  const salva = el("button", { type: "button", class: "btn small primary" }, "Salva");
  const pop = el("div", { class: "ds-pop", role: "dialog", "aria-label": "Cambia la nota " + n.n },
    el("div", { class: "k" }, `Nota ${n.n}`), testo, el("div", { class: "ds-pop-errore", hidden: true }),
    el("div", { class: "ds-pop-azioni" }, el("button", { type: "button", class: "btn small", onclick: () => chiudiPop() }, "Annulla"), salva));
  mettiPop(pop, r.right, r.bottom);
  testo.focus();
  salva.addEventListener("click", async () => {
    const t = testo.value.trim();
    if (!t) return;
    salva.disabled = true;
    const esito = await gesto(S.base + "/nota/" + n.id + "/modifica", { testo: t });
    if (esito.ok) chiudiPop(); else { salva.disabled = false; mostraErrorePop(esito.testo); }
  });
}

function mostraErrorePop(t) {
  if (!S.pop) return;
  const e = $(".ds-pop-errore", S.pop);
  if (e) { e.hidden = false; e.textContent = t || "Non riuscito."; }
}

function mettiPop(pop, cx, cy) {
  const st = stage();
  const area = $(".ds-area", st);
  S.pop = pop;
  st.append(pop);
  const sr = st.getBoundingClientRect();
  const w = pop.offsetWidth, h = pop.offsetHeight;
  let l = cx - sr.left + 14, t = cy - sr.top + 14;
  if (l + w > sr.width - 8) l = Math.max(8, cx - sr.left - w - 14);
  if (t + h > sr.height - 8) t = Math.max(8, sr.height - h - 8);
  pop.style.left = l + "px";
  pop.style.top = t + "px";
  pop.addEventListener("click", (e) => e.stopPropagation());
  pop.addEventListener("wheel", (e) => e.stopPropagation());
  if (area) area.focus({ preventScroll: true });
}

function chiudiPop() {
  if (S.pop) { S.pop.remove(); S.pop = null; }
  $$(".ds-pin.acceso").forEach((x) => x.classList.remove("acceso"));
  if (S.tmp) { S.tmp = null; disegnaNote(); }
}

// vaiANota porta alla pagina della nota e la apre.
async function vaiANota(id, a) {
  if (a && (a !== S.file || S.prec)) {
    S.prec = "";
    S.file = a; S.fileDi[S.scelto] = a; scriviIndirizzo(); segnaFileAperto();
    mostra(true);
  }
  const n = noteDelFile(a || S.docA).find((x) => x.id === id);
  if (!n) return;
  if (S.apertura) await S.apertura;
  if (!S.doc || (a && S.docA !== a)) return; // non si e' aperto, o nel frattempo si e' scelto altro
  if (S.pagina !== n.p) { S.pagina = n.p; await disegna(); }
  const b = $(`.ds-pin[data-nota="${CSS.escape(id)}"]`, stage());
  if (b) { mostraDentro($(".ds-area", stage()), b); apriNota(n, b); }
}

// ------------------------------------------------------------------ i gesti mandati da qui

// gesto manda un POST come se fosse un bottone della pagina (stesso bersaglio, stessi pannelli rifatti) e dice
// se e' andato: un «no» del server arriva come avviso rosso, un errore HTTP non arriva affatto.
function gesto(url, valori, sorgente) {
  return new Promise((risolvi) => {
    const src = sorgente || stage() || S.radice;
    let esito = null;
    const ascolta = (e) => { if (e.detail && e.detail.elt === src) esito = e.detail; };
    document.body.addEventListener("htmx:afterRequest", ascolta);
    const fine = () => {
      document.body.removeEventListener("htmx:afterRequest", ascolta);
      const av = $("#fasc-avviso .avviso-f");
      if (!esito || !esito.successful) {
        const s = esito && esito.xhr ? esito.xhr.status : 0;
        risolvi({ ok: false, testo: s === 403 ? "Non autorizzato: chi consulta non cambia il fascicolo." : "Il server non ha risposto" + (s ? " (" + s + ")" : "") + ": riprova." });
        return;
      }
      if (av && av.classList.contains("no")) { risolvi({ ok: false, testo: av.textContent.trim() }); return; }
      risolvi({ ok: true, testo: av ? av.textContent.trim() : "" });
    };
    htmx.ajax("POST", url, { source: src, values: valori, target: "#fasc-avviso", swap: "innerHTML" }).then(fine, fine);
  });
}

// ------------------------------------------------------------------ le miniature del filmstrip

let osservatore = null, codaMini = [], miniInCorso = 0, lavoratoreMini = null;
function osservaMiniature() {
  const film = document.getElementById("doc-film");
  if (!film) return;
  if (osservatore) osservatore.disconnect();
  codaMini = [];
  for (const t of $$(".docv-tile[data-anteprima]", film)) {
    const a = t.dataset.anteprima;
    if (S.mini.has(a)) { mettiMini(t, S.mini.get(a)); continue; }
    if (S.miniFallite.has(a)) miniNonDisponibile(t);
  }
  osservatore = new IntersectionObserver((voci) => {
    for (const v of voci) {
      if (!v.isIntersecting) continue;
      const t = v.target;
      osservatore.unobserve(t);
      if (daFare(t.dataset.anteprima)) { codaMini.push(t); lavoraMini(); }
    }
  }, { root: film, rootMargin: "0px 200px" });
  for (const t of $$(".docv-tile[data-anteprima]", film)) if (daFare(t.dataset.anteprima)) osservatore.observe(t);
}
function daFare(a) {
  const fallita = S.miniFallite.get(a);
  if (fallita && Date.now() - fallita > 60000) S.miniFallite.delete(a);
  return !S.mini.has(a) && !S.miniInCorso.has(a) && !S.miniFallite.has(a);
}
function miniNonDisponibile(t) {
  const box = $(".docv-mini", t);
  if (box && !$("img", box)) box.innerHTML = '<span class="docv-mini-testo">anteprima non disponibile</span>';
}
function mettiMini(t, url) {
  const box = $(".docv-mini", t);
  if (!box || $("img", box)) return;
  box.innerHTML = "";
  box.append(el("img", { src: url, alt: "" }));
}
async function lavoraMini() {
  while (miniInCorso < 2 && codaMini.length) {
    const t = codaMini.shift();
    if (!t.isConnected || !daFare(t.dataset.anteprima)) continue;
    miniInCorso++;
    miniatura(t).finally(() => { miniInCorso--; lavoraMini(); });
  }
}
async function miniatura(t) {
  const a = t.dataset.anteprima;
  S.miniInCorso.add(a);
  try { await miniaturaDi(t, a); } finally { S.miniInCorso.delete(a); }
}
async function miniaturaDi(t, a) {
  const lib = await pdfjs();
  if (!lib) return;
  if (!lavoratoreMini) lavoratoreMini = new lib.PDFWorker({ name: "miniature" });
  let task;
  try {
    task = lib.getDocument(opzioniPdf("/allegato/" + a + "/anteprima", { worker: lavoratoreMini, disableRange: true, disableStream: true }));
    const doc = await task.promise;
    const page = await doc.getPage(1);
    const vp1 = page.getViewport({ scale: 1 });
    const s = Math.min(220 / vp1.width, 144 / vp1.height);
    const vp = page.getViewport({ scale: s });
    const c = document.createElement("canvas");
    c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
    await page.render({ canvas: c, viewport: vp, annotationMode: lib.AnnotationMode.DISABLE }).promise;
    const url = c.toDataURL("image/png");
    S.mini.set(a, url);
    for (const x of $$(`.docv-tile[data-anteprima="${CSS.escape(a)}"]`)) mettiMini(x, url);
  } catch (err) {
    S.miniFallite.set(a, Date.now());
    for (const x of $$(`.docv-tile[data-anteprima="${CSS.escape(a)}"]`)) miniNonDisponibile(x);
  } finally {
    if (task) task.destroy().catch(() => {});
  }
}

// ------------------------------------------------------------------ la pagina intorno

function filtra(testo) {
  const t = (testo || "").trim().toLowerCase();
  for (const r of $$(".docv-riga[data-k]")) r.hidden = !!t && !(r.dataset.cerca || "").toLowerCase().includes(t);
  for (const x of $$(".docv-tile[data-k]")) {
    const e = elemento(x.dataset.k);
    x.hidden = !!t && !(((e && e.codice) || "") + " " + ((e && e.desc) || "")).toLowerCase().includes(t);
  }
}

function scrittura(e) {
  if (e.target.classList && e.target.classList.contains("docv-filtro")) filtra(e.target.value);
}

function clic(e) {
  const t = e.target;
  if (!(t instanceof Element)) return;
  if (t.closest(".bomed-sfondo")) return; // l'editor ha i suoi
  const av = t.closest("#fasc-avviso");
  if (av && vistaDocumenti()) { av.innerHTML = ""; return; }
  const agg = t.closest(".docv-aggiorna");
  if (agg) { e.preventDefault(); agg.remove(); htmx.ajax("GET", S.base + "/parti" + location.search, { source: S.radice }); return; }
  const ed = t.closest("[data-editor]");
  if (ed && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
    if (!S.scrive || S.bloccata) return;
    e.preventDefault();
    Editor.apri(ed.dataset.editor, ed.dataset.step || "");
    return;
  }
  if (!vistaDocumenti()) return;
  const sel = t.closest(".docv-riga[data-k], .docv-tile[data-k]");
  if (sel && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.button === 0) {
    if (scegli(sel.dataset.k)) e.preventDefault();
    return;
  }
  const mostraF = t.closest(".docv-mostra[data-a]");
  if (mostraF && !e.ctrlKey && !e.metaKey) {
    e.preventDefault();
    S.prec = "";
    S.file = mostraF.dataset.a; S.fileDi[S.scelto] = S.file; scriviIndirizzo(); segnaFileAperto(); mostra(true);
    return;
  }
  const dnodo = t.closest("[data-doc-nodo]");
  if (dnodo && !e.ctrlKey && !e.metaKey) {
    // un componente di questo gruppo si apre qui; uno di un altro gruppo con l'indirizzo, che porta il suo
    if (scegli("c:" + dnodo.dataset.docNodo)) e.preventDefault();
    return;
  }
  const dfile = t.closest("[data-doc-file]");
  if (dfile) {
    const a = dfile.dataset.docFile;
    const e2 = (S.indice.elementi || []).find((x) => (x.file || []).some((f) => f.a === a));
    if (e2) { e.preventDefault(); scegli(e2.k, a); }
    return;
  }
  const nota = t.closest(".docv-nota[data-nota]");
  if (nota) { e.preventDefault(); vaiANota(nota.dataset.nota, nota.dataset.a); return; }
  const armaB = t.closest(".docv-arma[data-a]");
  if (armaB) {
    e.preventDefault();
    if (armaB.dataset.a !== S.file || S.prec) { S.prec = ""; S.file = armaB.dataset.a; S.fileDi[S.scelto] = S.file; scriviIndirizzo(); segnaFileAperto(); mostra(true); }
    (S.apertura || Promise.resolve()).then(() => arma(true));
    return;
  }
  const prec = t.closest(".docv-vedi-prec[data-a]");
  if (prec) {
    e.preventDefault();
    // la revisione precedente si guarda con le sue note, senza cambiare la scelta del pannello; resta aperta
    // finche' non si sceglie altro (una risposta del server non la richiude)
    // il file di cui e' la revisione precedente diventa quello aperto, poi la revisione si apre al suo posto
    const suo = (elemento(S.scelto) || { file: [] }).file.find((f) => S.indice.prec && S.indice.prec[f.a] && S.indice.prec[f.a].a === prec.dataset.a);
    if (suo && suo.a !== S.file) { S.file = suo.a; S.fileDi[S.scelto] = S.file; scriviIndirizzo(); segnaFileAperto(); }
    S.prec = prec.dataset.a;
    mostra(true);
  }
}

function tasto(e) {
  if (Editor.corrente) return; // l'editor ha la sua tastiera
  if (e.key === "Escape") {
    if (S.pop) { chiudiPop(); e.preventDefault(); return; }
    if (S.armato) { arma(false); e.preventDefault(); return; }
    return;
  }
  if (!vistaDocumenti() || inCampo(e.target) || e.altKey || e.ctrlKey || e.metaKey) return;
  if ($(".fasc-cassetto .cassetto-dentro")) return; // il cassetto aperto: i tasti sono suoi
  switch (e.key) {
    case "ArrowRight": case "ArrowDown": e.preventDefault(); passo(1); break;
    case "ArrowLeft": case "ArrowUp": e.preventDefault(); passo(-1); break;
    case "PageDown": e.preventDefault(); pagina(1); break;
    case "PageUp": e.preventDefault(); pagina(-1); break;
    case "+": zoom(1.25); break;
    case "-": zoom(1 / 1.25); break;
    case "0": S.zoom = 1; disegna(); break;
    case "n": case "N": if (S.scrive) arma(!S.armato); break;
  }
}

// configuraRichiesta: i collegamenti della pagina sono stati disegnati prima che l'operatore scegliesse un
// altro componente qui; portano il nodo e il file di adesso, non quelli di allora.
function configuraRichiesta(e) {
  const d = e.detail;
  if (!vistaDocumenti() || !d || (d.verb || "").toLowerCase() !== "get") return;
  if (d.elt && d.elt.closest && d.elt.closest("[data-selezione]")) return;
  if (!/\/fascicolo\/(parti|vista|anteprima)(\?|$)/.test(d.path || "")) return;
  const u = new URL(d.path, location.origin);
  const ora = new URLSearchParams(location.search);
  for (const k of ["nodo", "file", "gruppo"]) {
    if (ora.has(k)) u.searchParams.set(k, ora.get(k)); else u.searchParams.delete(k);
  }
  d.path = u.pathname + u.search;
}

// segnaRisposta dice, per la risposta che sta per entrare, se e' quella del poll: gli scambi fuori banda della
// stessa risposta vengono subito dopo
function segnaRisposta(e) {
  const rc = e.detail && e.detail.requestConfig;
  const elt = (rc && rc.elt) || (e.detail && e.detail.elt);
  S.rispostaDelPoll = !!(elt && elt.id === "fasc-avanzamento");
  S.rispostaSelezione = !!(elt && elt.closest && elt.closest("[data-selezione]"));
  S.genRisposta = rc && typeof rc.genScelta === "number" ? rc.genScelta : -1;
}

function occupato() {
  const v = vistaDocumenti();
  if (!v) return false;
  const a = document.activeElement;
  if (a && v.contains(a) && inCampo(a)) return true;
  if ($("details[open]", v)) return true;
  return !!(S.pop || S.armato);
}

// il poll rifa' la vista quando cambia qualcosa; se l'operatore sta scrivendo, la vista resta com'e' e un
// bottone dice che c'e' dell'altro.
function primaDelloScambioFuoriBanda(e) {
  const d = e.detail;
  if (!d || !d.target || d.target.id !== "vista") return;
  if (!S.rispostaDelPoll || !occupato()) { salvaScrollDi(); return; }
  d.shouldSwap = false;
  const radice = document.getElementById("fascicolo") || S.radice;
  if (radice && !$(".docv-aggiorna", radice)) {
    radice.append(el("button", { type: "button", class: "btn small primary docv-aggiorna" }, "Ci sono novità: aggiorna"));
  }
}

function salvaScroll(e) {
  if (e && e.detail && e.detail.target && e.detail.target.id === "doc-sezione") return;
  salvaScrollDi();
}
function salvaScrollDi() {
  const a = document.getElementById("doc-albero"), f = document.getElementById("doc-film"), s = document.getElementById("doc-sezione");
  S.scroll = { albero: a ? a.scrollTop : 0, film: f ? f.scrollLeft : 0, sezione: s ? s.scrollTop : 0 };
}
function ripristinaScroll() {
  const a = document.getElementById("doc-albero"), f = document.getElementById("doc-film"), s = document.getElementById("doc-sezione");
  if (a && S.scroll.albero) a.scrollTop = S.scroll.albero;
  if (f && S.scroll.film) f.scrollLeft = S.scroll.film;
  if (s && S.scroll.sezione) s.scrollTop = S.scroll.sezione;
}

// ------------------------------------------------------------------ l'editor della struttura

// Lo stato dell'editor sono gli archi voluti (padre → figlio, quantita'): la struttura del prodotto e' quello
// che la radice raggiunge; il resto sta nel vassoio. Ogni gesto e' un nuovo stato, e Annulla torna indietro.
class Editor {
  static corrente = null;
  static inApertura = false; // un doppio clic non apre due editor
  static inviante = null;    // l'editor che ha mandato la conferma: l'esito e' suo

  static async apri(prodotto, step) {
    if (Editor.corrente || Editor.inApertura) return;
    const posto = document.getElementById("fasc-editor");
    if (!posto) return;
    let dati;
    Editor.inApertura = true;
    try {
      const r = await fetch(S.base + "/bom/dati?prodotto=" + encodeURIComponent(prodotto || "") + "&step=" + encodeURIComponent(step || ""),
        { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!r.ok) throw new Error(r.status);
      dati = await r.json();
    } catch (err) {
      alert("I dati della struttura non si sono potuti leggere: riprova.");
      return;
    } finally {
      Editor.inApertura = false;
    }
    if (!dati.prodotti || !dati.prodotti.length) {
      alert("Non c'è ancora un prodotto finito: i codici della richiesta, confermati, diventano prodotti da soli.");
      return;
    }
    if (!dati.scrive || dati.bloccata) return;
    Editor.corrente = new Editor(dati, posto);
  }

  constructor(dati, posto) {
    this.d = dati;
    this.posto = posto;
    this.radice = dati.prodotto;
    this.nodi = dati.nodi || {};
    this.archi = new Map();   // "p|f" → {padre, figlio, qta, prop: bool, stepQta}
    for (const a of dati.archi || []) this.archi.set(a.padre + "|" + a.figlio, { padre: a.padre, figlio: a.figlio, qta: a.qta, prop: false });
    this.proposteDi = new Map();
    for (const p of dati.proposti || []) {
      const k = p.padre + "|" + p.figlio;
      this.proposteDi.set(k, p);
      const w = this.archi.get(k);
      if (w) { if (w.qta !== p.qta) w.stepQta = p.qta; }
      else this.archi.set(k, { padre: p.padre, figlio: p.figlio, qta: p.qta, prop: true });
    }
    this.rimozioni = new Set((dati.rimozioni || []).map((r) => r.padre + "|" + r.figlio));
    this.radici = new Set();
    this.scarta = new Set();
    this.codici = {};
    this.storia = [];
    this.scelte = new Set();
    this.menuAperto = null;
    this.iniziale = this.firma();
    // La bozza e' di chi la sta scrivendo: nella chiave c'e' l'utente, cosi' chi entra dopo sulla stessa
    // scheda non la trova (e non la conferma a nome suo). All'uscita layout.html butta le cockpit.editor.*.
    const ds = S.radice ? S.radice.dataset : {};
    this.chiaveBozza = "cockpit.editor." + (ds.utente || "") + "." + (ds.thread || "") + "." + this.radice;
    // l'impronta della BOM su cui si lavora: il contenuto (archi con le quantita', proposte), non la lunghezza.
    // Una bozza si riprende solo sulla stessa BOM; se nel frattempo e' cambiata anche di una quantita', no
    this.impronta = impronta(JSON.stringify([(dati.archi || []).map((a) => [a.padre, a.figlio, a.qta]).sort(),
      (dati.proposti || []).map((p) => [p.allegato, p.pk, p.fk, p.padre, p.figlio, p.qta]).sort(),
      Object.keys(dati.nodi || {}).sort()]));
    this.monta();
    this.riprendiBozza();
    this.disegna();
  }

  // ---- lo stato

  firma() {
    return JSON.stringify([[...this.archi.values()].map((a) => [a.padre, a.figlio, a.qta]).sort(), [...this.radici].sort(), [...this.scarta].sort(), this.codici]);
  }
  cambiato() { return this.firma() !== this.iniziale; }
  istantanea() {
    return { archi: [...this.archi.values()].map((a) => ({ ...a })), radici: [...this.radici], scarta: [...this.scarta], codici: { ...this.codici } };
  }
  ripristina(s) {
    this.archi = new Map(s.archi.map((a) => [a.padre + "|" + a.figlio, a]));
    this.radici = new Set(s.radici);
    this.scarta = new Set(s.scarta);
    this.codici = { ...s.codici };
    this.scelte.clear();
  }
  prima() {
    this.storia.push(this.istantanea());
    if (this.storia.length > 200) this.storia.shift();
    // la selezione multipla e' di archi che il gesto sta per cambiare: si ricomincia, e un selettore aperto su
    // quella selezione si chiude
    this.scelte.clear();
    for (const x of $$(".bomed-scegli", this.el)) x.remove();
  }
  annullaUltima() {
    const s = this.storia.pop();
    if (!s) return;
    this.ripristina(s);
    this.disegna("Ultima modifica annullata.");
  }
  salvaBozza() {
    if (!this.cambiato()) { sessione(this.chiaveBozza, null); return; }
    sessione(this.chiaveBozza, JSON.stringify({ impronta: this.impronta, stato: this.istantanea() }));
  }
  riprendiBozza() {
    const b = prova(() => JSON.parse(sessione(this.chiaveBozza) || "null"), null);
    if (!b) return;
    if (b.impronta === this.impronta) {
      this.ripristina(b.stato);
      this.avvisa("Riprese le modifiche non confermate di prima (la BOM non è cambiata nel frattempo). «Annulla» in alto per lasciarle.", "info");
    } else {
      sessione(this.chiaveBozza, null);
      this.avvisa("Le modifiche non confermate di prima erano su una BOM che nel frattempo è cambiata: non si riprendono.", "");
    }
  }

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
  raggiuntiDa(da, archi, proposti) {
    const figli = new Map();
    for (const a of [...archi, ...proposti]) { if (!figli.has(a.padre)) figli.set(a.padre, []); figli.get(a.padre).push(a.figlio); }
    const visti = new Set([da]);
    const coda = [da];
    while (coda.length) { const n = coda.shift(); for (const f of figli.get(n) || []) if (!visti.has(f)) { visti.add(f); coda.push(f); } }
    return visti;
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
  nome(ref) {
    const n = this.nodi[ref];
    if (!n) return ref;
    if (ref.startsWith("p:") && this.codici[ref.slice(2)] && this.codici[ref.slice(2)].codice) return this.codici[ref.slice(2)].codice;
    return n.codice || (n.nome ? "«" + n.nome + "»" : ref);
  }
  prodotti() { return new Set((this.d.prodotti || []).map((p) => p.ref)); }

  // i nodi del vassoio: non raggiunti dal prodotto, e non di un altro prodotto
  vassoio() {
    const qui = this.raggiunti(this.radice);
    const altrove = new Set();
    for (const p of this.prodotti()) if (p !== this.radice) for (const x of this.raggiunti(p)) altrove.add(x);
    const out = [];
    for (const ref of Object.keys(this.nodi)) {
      const n = this.nodi[ref];
      if (qui.has(ref) || altrove.has(ref) || n.finito || this.scarta.has(ref.slice(2)) || this.radici.has(ref)) continue;
      out.push(ref);
    }
    return out;
  }

  // ---- i gesti

  scendeDa(anc, ref) { return anc === ref || this.raggiunti(ref).has(anc); }

  // sposta: l'arco padre → figlio va sotto un altro padre (drag normale)
  sposta(padre, figlio, verso) {
    if (verso === padre) return;
    if (this.scendeDa(verso, figlio)) { this.avvisa(`${this.nome(figlio)} non può andare sotto ${this.nome(verso)}: chiuderebbe un ciclo.`, ""); return; }
    if (this.archi.has(verso + "|" + figlio)) { this.avvisa(`${this.nome(figlio)} è già sotto ${this.nome(verso)}.`, ""); return; }
    this.prima();
    const vecchio = padre ? this.archi.get(padre + "|" + figlio) : null;
    if (vecchio) this.archi.delete(padre + "|" + figlio);
    const prop = this.proposteDi.get(verso + "|" + figlio);
    this.archi.set(verso + "|" + figlio, { padre: verso, figlio, qta: vecchio ? vecchio.qta : prop ? prop.qta : 1, prop: !!prop && !vecchio });
    this.disegna(`${this.nome(figlio)} sotto ${this.nome(verso)}.`);
  }
  condividi(figlio, verso) {
    if (this.scendeDa(verso, figlio)) { this.avvisa(`${this.nome(figlio)} non può andare sotto ${this.nome(verso)}: chiuderebbe un ciclo.`, ""); return; }
    if (this.archi.has(verso + "|" + figlio)) { this.avvisa(`${this.nome(figlio)} è già sotto ${this.nome(verso)}.`, ""); return; }
    this.prima();
    this.archi.set(verso + "|" + figlio, { padre: verso, figlio, qta: 1, prop: false });
    this.disegna(`${this.nome(figlio)} anche sotto ${this.nome(verso)}: ora è condiviso.`);
  }
  togli(padre, figlio) {
    this.prima();
    this.archi.delete(padre + "|" + figlio);
    this.disegna(`${this.nome(figlio)} non è più sotto ${this.nome(padre)}${this.raggiunti(this.radice).has(figlio) ? "" : ": è nel vassoio"}.`);
  }
  quantita(padre, figlio, q) {
    const a = this.archi.get(padre + "|" + figlio);
    const n = parseInt(q, 10);
    if (!a || !(n >= 1 && n <= 100000) || n === a.qta) return;
    this.prima();
    a.qta = n;
    this.disegna();
  }
  scartaNodo(ref) {
    const sotto = [...this.raggiunti(ref)].filter((x) => x !== ref && x.startsWith("p:") && this.padriDi(x).every((a) => this.raggiunti(ref).has(a.padre) || a.padre === ref));
    let insieme = [ref];
    if (sotto.length && confirm(`Scartare anche ${sotto.length} nod${sotto.length === 1 ? "o" : "i"} che ${this.nome(ref)} ha sotto nello STEP?`)) insieme = insieme.concat(sotto);
    this.prima();
    for (const r of insieme) {
      this.scarta.add(r.slice(2));
      for (const k of [...this.archi.keys()]) { const a = this.archi.get(k); if (a.padre === r || a.figlio === r) this.archi.delete(k); }
    }
    this.disegna(`${this.nome(ref)} scartato: non è un pezzo della distinta.`);
  }
  eIlProdotto(ref) {
    this.prima();
    for (const a of this.figliDi(ref)) {
      this.archi.delete(a.padre + "|" + a.figlio);
      if (!this.archi.has(this.radice + "|" + a.figlio) && a.figlio !== this.radice) this.archi.set(this.radice + "|" + a.figlio, { padre: this.radice, figlio: a.figlio, qta: a.qta, prop: a.prop });
    }
    for (const a of this.padriDi(ref)) this.archi.delete(a.padre + "|" + a.figlio);
    this.radici.add(ref);
    this.disegna(`${this.nome(ref)} è il prodotto ${this.nome(this.radice)}: i suoi figli sono sotto il prodotto.`);
  }
  scriviCodice(ref, codice) {
    codice = (codice || "").trim();
    const id = ref.slice(2);
    if (codice && (codice.length > 40 || /\s/.test(codice))) { this.avvisa("Un codice ha al massimo 40 caratteri, senza spazi.", ""); return; }
    this.prima();
    if (codice) this.codici[id] = { codice, rev: "" }; else delete this.codici[id];
    const gia = Object.entries(this.nodi).find(([r, n]) => r.startsWith("c:") && (n.codice || "").toUpperCase() === codice.toUpperCase());
    this.disegna(gia ? `${codice} c'è già nella BOM: il nodo lo ritroverà (stesso pezzo).` : "");
  }

  // ---- la conferma

  payload() {
    const qui = this.raggiunti(this.radice);
    // quello che l'editor mostrava: la struttura di questo prodotto (adesso e all'apertura) e il vassoio. Le
    // proposte sotto un altro prodotto le decide chi guarda quello
    const nostri = new Set([...qui, ...this.raggiuntiDa(this.radice, this.d.archi || [], this.d.proposti || [])]);
    const altrove = new Set();
    for (const p of this.prodotti()) if (p !== this.radice) for (const x of this.raggiuntiDa(p, this.d.archi || [], this.d.proposti || [])) altrove.add(x);
    const visto = { has: (ref) => nostri.has(ref) || !altrove.has(ref) };
    const archi = [];
    for (const a of this.archi.values()) if (qui.has(a.padre)) archi.push({ padre: a.padre, figlio: a.figlio, qta: a.qta });
    const codici = {};
    for (const [id, c] of Object.entries(this.codici)) codici[id] = c;
    return {
      radice: this.radice.slice(2),
      archi,
      visti: (this.d.archi || []).map((a) => ({ padre: a.padre, figlio: a.figlio, qta: a.qta })),
      relazioni_viste: (this.d.proposti || []).filter((p) => visto.has(p.padre)).map((p) => ({ allegato: p.allegato, padre: p.pk, figlio: p.fk })),
      radici_proposte: [...this.radici].map((r) => r.slice(2)),
      scarta: [...this.scarta],
      codici,
    };
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
    return n + this.scarta.size + this.radici.size;
  }
  async conferma() {
    const qui = this.raggiunti(this.radice);
    for (const r of qui) {
      const n = this.nodi[r];
      if (n && n.senza_codice && !(this.codici[r.slice(2)] && this.codici[r.slice(2)].codice)) {
        this.avvisa(`${this.nome(r)} non ha un codice: lo si scrive nella sua riga, o lo si toglie dalla struttura.`, "");
        return;
      }
    }
    const b = $(".bomed-conferma", this.el);
    b.disabled = true;
    this.esito("Conferma in corso…", "");
    this.attesa(true);
    Editor.inviante = this;
    const esito = await gesto(S.base + "/bom/applica", { struttura: JSON.stringify(this.payload()) }, this.posto);
    this.attesa(false);
    if (this.chiuso) return;
    if (!esito.ok) { b.disabled = false; this.esito(esito.testo, "no"); }
  }
  // attesa: mentre la conferma e' in viaggio non si chiude l'editor e non si cambia prodotto
  attesa(on) {
    this.inAttesa = on;
    for (const x of $$(".bomed-prodotto, .bomed-annulla, .bomed-indietro", this.el)) x.disabled = on || (x.classList.contains("bomed-indietro") && !this.storia.length);
  }
  esitoServer(ok, testo) {
    this.attesa(false); // l'evento arriva prima che la richiesta finisca: l'esito e' questo
    if (ok) { this.confermato = true; sessione(this.chiaveBozza, null); this.chiudi(true); return; }
    const b = $(".bomed-conferma", this.el);
    if (b) b.disabled = false;
    this.esito(testo, "no");
  }

  chiudi(forza) {
    if (this.inAttesa) return;
    if (!forza && this.cambiato() && !confirm("Uscire senza confermare? Le modifiche restano come bozza in questa scheda del browser.")) return;
    if (!this.confermato) this.salvaBozza(); // confermata, la bozza non c'e' piu'
    this.chiuso = true;
    this.el.remove();
    document.removeEventListener("keydown", this.tasti, true);
    Editor.corrente = null;
    if (this.ritornoFuoco && this.ritornoFuoco.focus) this.ritornoFuoco.focus();
  }

  // ---- il disegno

  monta() {
    this.ritornoFuoco = document.activeElement;
    const prodotti = el("select", { class: "bomed-prodotto", "aria-label": "Prodotto" },
      (this.d.prodotti || []).map((p) => el("option", { value: p.ref, selected: p.ref === this.radice }, p.codice + (p.desc ? " — " + p.desc : ""))));
    prodotti.addEventListener("change", async () => {
      if (this.inAttesa) { prodotti.value = this.radice; return; }
      if (this.cambiato() && !confirm("Cambiare prodotto? Le modifiche di questo restano come bozza.")) { prodotti.value = this.radice; return; }
      const p = prodotti.value;
      this.chiudi(true);
      Editor.apri(p.slice(2));
    });
    this.el = el("div", { class: "bomed-sfondo" },
      el("div", { class: "bomed", role: "dialog", "aria-modal": "true", "aria-labelledby": "bomed-titolo" },
        el("header", { class: "bomed-testa" },
          el("h2", { id: "bomed-titolo", tabindex: "-1" }, "Struttura di "), prodotti,
          el("span", { class: "bomed-step k" }),
          el("span", { class: "sp" }),
          el("button", { type: "button", class: "bomed-sposta-scelti", hidden: true }, "Sposta i selezionati sotto…"),
          el("button", { type: "button", class: "bomed-indietro", title: "Annulla l'ultima modifica (Ctrl+Z)" }, "↶ Annulla modifica"),
          el("button", { type: "button", class: "bomed-annulla" }, "Chiudi"),
          el("button", { type: "button", class: "primario bomed-conferma" }, "Conferma struttura")),
        el("div", { class: "bomed-avvisi" }),
        el("div", { class: "bomed-corpo" },
          el("section", { class: "bomed-albero", "aria-label": "Struttura del prodotto" }),
          el("aside", { class: "bomed-vassoio", "aria-label": "Non posizionati" })),
        el("footer", { class: "bomed-piede" },
          el("span", { class: "bomed-esito", role: "status", "aria-live": "polite" }),
          el("span", { class: "sp" }),
          el("span", {}, "Trascinare = spostare · il tratteggio è quello che propone lo STEP ed entra con la conferma · un secondo padre si aggiunge dal menu ⋯ (Condividi)"))));
    this.posto.append(this.el);
    $(".bomed-annulla", this.el).addEventListener("click", () => this.chiudi(false));
    $(".bomed-indietro", this.el).addEventListener("click", () => this.annullaUltima());
    $(".bomed-conferma", this.el).addEventListener("click", () => this.conferma());
    $(".bomed-sposta-scelti", this.el).addEventListener("click", (e) => this.sceltaPadre(e.currentTarget, "scelti"));
    this.tasti = (e) => this.tasto(e);
    document.addEventListener("keydown", this.tasti, true);
    this.el.addEventListener("dragstart", (e) => this.inizioTrascina(e));
    this.el.addEventListener("dragover", (e) => this.sopra(e));
    this.el.addEventListener("dragleave", (e) => { const t = e.target.closest && e.target.closest(".bersaglio"); if (t) t.classList.remove("bersaglio"); });
    this.el.addEventListener("drop", (e) => this.lascia(e));
    this.el.addEventListener("click", (e) => this.clic(e));
    this.el.addEventListener("change", (e) => this.cambio(e));
    setTimeout(() => $("#bomed-titolo", this.el).focus(), 0);
    const st = this.d.step;
    const avv = $(".bomed-avvisi", this.el);
    if (this.d.analisi > 0) avv.append(el("div", { class: "bomed-avviso info" }, `Analisi in corso su ${this.d.analisi} file: la proposta può ancora cambiare. Si può lavorare lo stesso; la conferma dice se qualcosa è cambiato.`));
    if (st && st.esito === "presente_parziale") avv.append(el("div", { class: "bomed-avviso" }, "Lo STEP strutturale è letto in parte" + (st.motivo ? ": " + st.motivo : "") + ". La struttura proposta può essere incompleta."));
    if (st && st.etichetta) $(".bomed-step", this.el).textContent = "STEP: " + st.etichetta;
  }

  avvisa(t, tipo) { this.esito(t, tipo === "info" ? "ok" : "no"); }
  esito(t, cl) {
    const e = $(".bomed-esito", this.el);
    if (!e) return;
    e.textContent = t || "";
    e.className = "bomed-esito" + (cl ? " " + cl : "");
  }

  disegna(messaggio) {
    const albero = $(".bomed-albero", this.el);
    const vass = $(".bomed-vassoio", this.el);
    const tieni = [albero.scrollTop, vass.scrollTop];
    albero.innerHTML = "";
    vass.innerHTML = "";
    const aperti = new Set();
    const riga = (ref, arco, livello, dentro) => {
      const n = this.nodi[ref] || {};
      const ripetuto = aperti.has(ref);
      const padri = this.padriDi(ref).length;
      const cl = ["bomed-riga"];
      if (!arco && dentro === albero) cl.push("radice");
      if ((arco && arco.prop) || n.proposto || n.trovato) cl.push("proposto");
      if (ripetuto) cl.push("rimando");
      if (this.scelte.has(arco ? arco.padre + "|" + arco.figlio : ref)) cl.push("scelta");
      // nel vassoio si prende solo la carta in cima, con il suo sottoalbero: i legami sotto di lei non sono
      // della struttura del prodotto, e la conferma non li toccherebbe. Le righe del vassoio stanno nella sua
      // lista, non direttamente nel vassoio: si guarda dove stanno, non chi e' il contenitore
      const nelVassoio = !!(dentro.closest && dentro.closest(".bomed-vassoio"));
      const mobile = nelVassoio ? livello === 0 : !!arco;
      const r = el("div", { class: cl.join(" "), "data-ref": ref, "data-arco": arco && !nelVassoio ? arco.padre + "|" + arco.figlio : "", draggable: mobile ? "true" : "false", role: "treeitem", "aria-level": livello + 1 });
      r.style.paddingLeft = (6 + livello * 18) + "px";
      if (mobile) r.append(el("span", { class: "bomed-maniglia", "aria-hidden": "true" }, "⠿"));
      if (n.senza_codice) {
        const i = el("input", { class: "bomed-codice", placeholder: "codice di «" + (n.nome || "nodo") + "»", "aria-label": "codice", value: (this.codici[ref.slice(2)] || {}).codice || "", "data-codice": ref });
        r.append(i);
      } else r.append(el("span", { class: "bomed-cod" }, this.nome(ref)));
      if (n.rev) r.append(el("span", { class: "k" }, "rev " + n.rev));
      if (n.desc) r.append(el("span", { class: "bomed-desc", title: n.desc }, n.desc));
      r.append(el("span", { class: "bomed-tipo" }, n.finito ? "prodotto" : n.trovato ? "codice trovato" : ({ sottoassieme: "assieme", sciolto: "particolare", commerciale: "commerciale" }[n.tipo] || "")));
      if (n.proposto || (arco && arco.prop)) r.append(el("span", { class: "bomed-badge prop", title: n.file ? "dallo STEP " + n.file : "" }, "proposto"));
      if (padri > 1) r.append(el("span", { class: "bomed-badge info", title: "Un componente solo sotto più padri" }, `condiviso · ${padri} padri`));
      if (arco && arco.stepQta) r.append(el("span", { class: "bomed-badge warn" }, `lo STEP dice ×${arco.stepQta} `, el("button", { type: "button", "data-usa-step": arco.padre + "|" + arco.figlio }, "usa")));
      if (arco && this.rimozioni.has(arco.padre + "|" + arco.figlio)) r.append(el("span", { class: "bomed-badge urg", title: "Lo STEP strutturale non contiene più questo legame: confermando lo si tiene" }, "lo STEP lo toglie"));
      if (ripetuto) r.append(el("span", { class: "k" }, "↗ il suo sottoalbero è sopra"));
      r.append(el("span", { class: "sp" }));
      if (arco && !nelVassoio) r.append(el("input", { type: "number", min: "1", max: "100000", class: "bomed-qta", value: arco.qta, "aria-label": "quantità di " + this.nome(ref) + " in " + this.nome(arco.padre), "data-qta": arco.padre + "|" + arco.figlio }));
      else if (arco) r.append(el("span", { class: "k" }, "×" + arco.qta));
      if (mobile) r.append(el("span", { class: "bomed-menu" }, el("button", { type: "button", class: "bomed-apri-menu", "aria-label": "Azioni su " + this.nome(ref), "aria-haspopup": "true" }, "⋯")));
      dentro.append(r);
      if (ripetuto) return;
      aperti.add(ref);
      for (const a of this.figliDi(ref)) riga(a.figlio, a, livello + 1, dentro);
    };
    riga(this.radice, null, 0, albero);
    if (!this.figliDi(this.radice).length) albero.append(el("p", { class: "bomed-vuoto" }, "Struttura non definita: trascina qui sotto il prodotto i componenti del vassoio, oppure usa ⋯ › Metti sotto."));
    const vs = this.vassoio();
    const radiciVass = vs.filter((ref) => !this.padriDi(ref).some((a) => vs.includes(a.padre)));
    vass.append(el("h3", {}, `Non posizionati (${vs.length})`));
    const filtro = el("input", { type: "search", class: "bomed-filtro", placeholder: "Filtra…", "aria-label": "Filtra i non posizionati", value: this.filtro || "" });
    vass.append(filtro);
    const f = (this.filtro || "").toLowerCase();
    const scatola = el("div", { class: "bomed-lista" });
    vass.append(scatola);
    for (const ref of radiciVass.sort((a, b) => this.nome(a).localeCompare(this.nome(b)))) {
      if (f && !this.nome(ref).toLowerCase().includes(f) && !((this.nodi[ref] || {}).desc || "").toLowerCase().includes(f)) continue;
      riga(ref, null, 0, scatola);
    }
    if (!radiciVass.length) scatola.append(el("p", { class: "bomed-vuoto" }, "Niente fuori dalla struttura."));
    albero.scrollTop = tieni[0]; vass.scrollTop = tieni[1];
    const n = this.modifiche();
    const b = $(".bomed-conferma", this.el);
    b.textContent = n ? `Conferma struttura (${n} modific${n === 1 ? "a" : "he"})` : "Conferma struttura";
    $(".bomed-indietro", this.el).disabled = !this.storia.length;
    $(".bomed-sposta-scelti", this.el).hidden = this.scelte.size < 2;
    $("#bomed-titolo", this.el).textContent = "Struttura di";
    if (messaggio !== undefined) this.esito(messaggio, "ok");
    if (!this.confermato) this.salvaBozza();
  }

  // ---- trascinare

  inizioTrascina(e) {
    const r = e.target.closest && e.target.closest(".bomed-riga[draggable=true]");
    if (!r) return;
    this.trascinato = { ref: r.dataset.ref, arco: r.dataset.arco };
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", r.dataset.ref);
  }
  bersaglioDi(e) {
    return (e.target.closest && (e.target.closest(".bomed-albero .bomed-riga") || e.target.closest(".bomed-vassoio"))) || null;
  }
  sopra(e) {
    if (!this.trascinato) return;
    const t = this.bersaglioDi(e);
    if (!t) return;
    const ok = t.classList.contains("bomed-vassoio") ? !!this.trascinato.arco : !this.scendeDa(t.dataset.ref, this.trascinato.ref);
    if (ok) { e.preventDefault(); e.dataTransfer.dropEffect = "move"; }
    for (const x of $$(".bersaglio, .vietato", this.el)) x.classList.remove("bersaglio", "vietato");
    t.classList.add(ok ? "bersaglio" : "vietato");
  }
  lascia(e) {
    const tr = this.trascinato;
    this.trascinato = null;
    for (const x of $$(".bersaglio, .vietato", this.el)) x.classList.remove("bersaglio", "vietato");
    if (!tr) return;
    const t = this.bersaglioDi(e);
    if (!t) return;
    e.preventDefault();
    const [padre, figlio] = tr.arco ? tr.arco.split("|") : [null, tr.ref];
    if (t.classList.contains("bomed-vassoio")) { if (padre) this.togli(padre, figlio); return; }
    this.sposta(padre, figlio, t.dataset.ref);
  }

  // ---- menu, scelte, tastiera

  clic(e) {
    const t = e.target;
    const usa = t.closest("[data-usa-step]");
    if (usa) { const a = this.archi.get(usa.dataset.usaStep); if (a) { this.prima(); a.qta = a.stepQta; delete a.stepQta; this.disegna(); } return; }
    const apri = t.closest(".bomed-apri-menu");
    if (apri) { this.menu(apri.closest(".bomed-riga")); return; }
    const voce = t.closest("[data-voce]");
    if (voce) { this.voce(voce.dataset.voce, voce.closest(".bomed-menu") || voce); return; }
    if (this.menuAperto && !t.closest(".bomed-menu-dentro")) this.chiudiMenu();
    const r = t.closest(".bomed-riga");
    if (r && (e.ctrlKey || e.metaKey || e.shiftKey) && r.getAttribute("draggable") === "true") {
      const k = r.dataset.arco || r.dataset.ref;
      if (this.scelte.has(k)) this.scelte.delete(k); else this.scelte.add(k);
      r.classList.toggle("scelta", this.scelte.has(k));
      $(".bomed-sposta-scelti", this.el).hidden = this.scelte.size < 2;
    }
  }
  cambio(e) {
    const t = e.target;
    if (t.dataset.qta) this.quantita(...t.dataset.qta.split("|"), t.value);
    else if (t.dataset.codice) this.scriviCodice(t.dataset.codice, t.value);
    else if (t.classList.contains("bomed-filtro")) { this.filtro = t.value; this.disegna(); const i = $(".bomed-filtro", this.el); if (i) { i.focus(); i.setSelectionRange(i.value.length, i.value.length); } }
  }
  chiudiMenu() { if (this.menuAperto) { this.menuAperto.remove(); this.menuAperto = null; } }
  menu(riga) {
    this.chiudiMenu();
    const ref = riga.dataset.ref, arco = riga.dataset.arco;
    const n = this.nodi[ref] || {};
    const voci = [];
    if (arco) {
      voci.push(["sposta", "Sposta sotto…"], ["condividi", "Condividi anche sotto un altro assieme…"], ["togli", "Togli dalla struttura (va nel vassoio)"]);
    } else {
      voci.push(["metti", "Metti sotto…"]);
      if (ref.startsWith("p:") && this.figliDi(ref).length) voci.push(["prodotto", `È il prodotto ${this.nome(this.radice)} (i suoi figli vanno sotto il prodotto)`]);
    }
    if (ref.startsWith("p:") && !arco) voci.push(["scarta", "Scarta: non è un pezzo della distinta"]);
    const box = el("div", { class: "bomed-menu-dentro", role: "menu" }, voci.map(([k, t]) => el("button", { type: "button", role: "menuitem", "data-voce": k, "data-ref": ref, "data-arco": arco }, t)));
    riga.querySelector(".bomed-menu").append(box);
    this.menuAperto = box;
    const primo = $("button", box);
    if (primo) primo.focus();
    void n;
  }
  voce(k, dove) {
    const b = dove.querySelector ? dove.querySelector(`[data-voce="${k}"]`) || dove : dove;
    const ref = b.dataset.ref, arco = b.dataset.arco;
    this.chiudiMenu();
    const [padre, figlio] = arco ? arco.split("|") : [null, ref];
    switch (k) {
      case "togli": this.togli(padre, figlio); break;
      case "scarta": this.scartaNodo(ref); break;
      case "prodotto": this.eIlProdotto(ref); break;
      case "sposta": case "metti": this.sceltaPadre($(`.bomed-riga[data-ref="${CSS.escape(ref)}"][data-arco="${CSS.escape(arco || "")}"]`, this.el), "sposta", padre, figlio); break;
      case "condividi": this.sceltaPadre($(`.bomed-riga[data-ref="${CSS.escape(ref)}"][data-arco="${CSS.escape(arco || "")}"]`, this.el), "condividi", padre, figlio); break;
    }
  }
  // sceltaPadre apre sotto la riga un elenco dei padri possibili (senza i discendenti: niente cicli)
  sceltaPadre(dopo, modo, padre, figlio) {
    for (const x of $$(".bomed-scegli", this.el)) x.remove();
    const qui = [...this.raggiunti(this.radice)];
    let figli = [];
    if (modo === "scelti") figli = [...this.scelte].map((k) => (k.includes("|") ? k.split("|") : [null, k]));
    else figli = [[padre, figlio]];
    const ammessi = qui.filter((p) => figli.every(([, f]) => !this.scendeDa(p, f)) && !(this.nodi[p] || {}).senza_codice)
      .sort((a, b) => this.nome(a).localeCompare(this.nome(b)));
    const sel = el("select", { "aria-label": "nuovo padre" }, ammessi.map((p) => el("option", { value: p }, this.nome(p))));
    const ok = el("button", { type: "button", class: "primario" }, modo === "condividi" ? "Condividi" : "Sposta");
    const box = el("div", { class: "bomed-scegli" }, modo === "condividi" ? "Anche sotto" : "Sotto", sel, ok, el("button", { type: "button", onclick: () => box.remove() }, "Annulla"));
    (dopo || $(".bomed-albero", this.el)).after(box);
    sel.focus();
    ok.addEventListener("click", () => {
      const verso = sel.value;
      box.remove();
      if (modo === "condividi") { this.condividi(figlio, verso); return; }
      if (modo === "scelti") {
        this.prima();
        for (const [p, f] of figli) {
          if (this.scendeDa(verso, f) || this.archi.has(verso + "|" + f)) continue;
          const vecchio = p ? this.archi.get(p + "|" + f) : null;
          if (vecchio) this.archi.delete(p + "|" + f);
          this.archi.set(verso + "|" + f, { padre: verso, figlio: f, qta: vecchio ? vecchio.qta : 1, prop: false });
        }
        this.scelte.clear();
        this.disegna(`${figli.length} componenti sotto ${this.nome(verso)}.`);
        return;
      }
      this.sposta(padre, figlio, verso);
    });
  }
  tasto(e) {
    if (e.key === "Escape") {
      e.preventDefault();
      if (this.inAttesa) return;
      if (this.menuAperto) { this.chiudiMenu(); return; }
      const s = $(".bomed-scegli", this.el);
      if (s) { s.remove(); return; }
      this.chiudi(false);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && (e.key === "z" || e.key === "Z") && !inCampo(e.target)) { e.preventDefault(); this.annullaUltima(); return; }
    if (e.key === "Tab") { // il fuoco resta dentro l'editor
      const f = $$("button, input, select, [tabindex='-1']", this.el).filter((x) => !x.disabled && x.offsetParent !== null);
      if (!f.length) return;
      const i = f.indexOf(document.activeElement);
      if (e.shiftKey && i <= 0) { e.preventDefault(); f[f.length - 1].focus(); }
      else if (!e.shiftKey && i === f.length - 1) { e.preventDefault(); f[0].focus(); }
    }
  }
}

function esitoEditor(e) {
  const d = e.detail || {};
  const ed = Editor.inviante || Editor.corrente;
  Editor.inviante = null;
  if (ed && !ed.chiuso) ed.esitoServer(!!d.ok, d.testo || "");
}

// impronta e' un'impronta corta di un testo (cyrb53): basta a dire se due BOM sono la stessa.
function impronta(t) {
  let h1 = 0xdeadbeef, h2 = 0x41c6ce57;
  for (let i = 0; i < t.length; i++) {
    const c = t.charCodeAt(i);
    h1 = Math.imul(h1 ^ c, 2654435761);
    h2 = Math.imul(h2 ^ c, 1597334677);
  }
  h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
  h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
  return (h2 >>> 0).toString(16) + (h1 >>> 0).toString(16) + ":" + t.length;
}

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia);
else avvia();
