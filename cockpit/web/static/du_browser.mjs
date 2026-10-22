// Distinta unica, il browser ad albero della Revisione (richiesta [U] del 22/10, frontend): l'albero degli assiemi come
// quello di SolidWorks o di Fusion 360, prima dell'organigramma. Righe compatte e rientrate con il triangolo apri/chiudi,
// l'icona del tipo, codice e descrizione, la quantità del legame, lo stato con le parole della pagina e, sotto ogni pezzo,
// i suoi documenti come foglie. È solo presentazione: legge il grafo che la pagina ha già disegnato (lo stesso
// dell'organigramma) e i documenti che la pagina già conosce; non parla col server e non decide niente. Il clic su una
// riga sceglie il pezzo con la stessa funzione della tela (scegli), il doppio clic o Invio lo centra sulla tela (centraSel).
//
// Stabilità (richiesta esplicita [U]): aprire, chiudere e filtrare ridisegnano solo l'elenco del browser, mai la pagina;
// quando la pagina si ridisegna (dopo un clic su una riga) il riquadro che scorre e il fuoco tornano dov'erano
// (vistaCorpoPrima/Dopo della pagina, con la chiave del riquadro). Lo stato aperto/chiuso, il filtro e il pannello
// richiuso si ricordano per prodotto nella sessione (sessionStorage, con try/catch: senza memoria si riparte da capo).
//
// creaBrowser(o) → l'elemento del pannello. o:
//   g          il grafo { radice, nodi, archi, nonInclusi, modo } (grafoBozza, grafoWorking, grafoProposta)
//   chiave     la chiave della memoria (prodotto e vista)
//   scelto     la chiave del pezzo scelto (S.nodo)
//   scegli(k)  sceglie il pezzo (la funzione della tela)
//   centra(k)  centra il pezzo sulla tela, se c'è
//   icona(v)   "finito" | "assieme" | "particolare" | "commerciale" | "minuteria" | "cad" | "noninc"
//   documenti(v) → [{ id, nome, stato, tono }]   i documenti del pezzo, con lo stato a parole
//   statoDi(v)   → { testo, tono }              lo stato del pezzo a parole

const SVGNS = "http://www.w3.org/2000/svg";

function el(tag, props) {
  const e = document.createElement(tag);
  if (props) {
    for (const k of Object.keys(props)) {
      const v = props[k];
      if (v === null || v === undefined || v === false) continue;
      if (k === "class") e.className = v;
      else if (k.slice(0, 2) === "on" && typeof v === "function") e.addEventListener(k.slice(2), v);
      else if (v === true) e.setAttribute(k, "");
      else e.setAttribute(k, String(v));
    }
  }
  for (let i = 2; i < arguments.length; i++) metti(e, arguments[i]);
  return e;
}
function metti(e, c) {
  if (c === null || c === undefined || c === false || c === "") return;
  if (Array.isArray(c)) return c.forEach((x) => metti(e, x));
  e.append(c instanceof Node ? c : document.createTextNode(String(c)));
}
const sid = (k) => String(k).replace(/[^A-Za-z0-9_-]/g, "_");

// ---- le icone (SVG in linea, monocrome: prendono il colore del testo)

const PERCORSI = {
  finito: ["M8 1.5 14 4.8v6.4L8 14.5 2 11.2V4.8z", "M2 4.8 8 8l6-3.2M8 8v6.5"],
  assieme: ["M2 3.5h5v4H2zM9 3.5h5v4H9zM5.5 9.5h5v4h-5z"],
  particolare: ["M3 5.2 8 2.5l5 2.7v5.6L8 13.5l-5-2.7z", "M3 5.2 8 8l5-2.8M8 8v5.5"],
  commerciale: ["M8 2.2l1.2 1.3 1.7-.3.6 1.6 1.6.6-.3 1.7 1.3 1.2-1.3 1.2.3 1.7-1.6.6-.6 1.6-1.7-.3L8 13.8l-1.2-1.3-1.7.3-.6-1.6-1.6-.6.3-1.7L1.9 8l1.3-1.2-.3-1.7 1.6-.6.6-1.6 1.7.3z", "M8 6.2a1.8 1.8 0 1 0 0 3.6 1.8 1.8 0 0 0 0-3.6z"],
  minuteria: ["M5 2.5h6l-1 2H6z", "M8 4.5v9", "M6.3 6.5h3.4M6.3 8.5h3.4M6.3 10.5h3.4"],
  cad: ["M3 5.2 8 2.5l5 2.7v5.6L8 13.5l-5-2.7z", "M2.5 13.5 13.5 2.5"],
  noninc: ["M3 5.2 8 2.5l5 2.7v5.6L8 13.5l-5-2.7z"],
  doc: ["M4 1.8h5.2L12.5 5v9.2H4z", "M9 1.8V5.2h3.5"],
  cartella: ["M1.8 4h4.3l1.3 1.5h6.8v7.7H1.8z"],
  freccia: ["M5.5 3.5 10.5 8l-5 4.5"],
  pannello: ["M2 2.5h12v11H2z", "M6 2.5v11"],
};
function icona(nome, cls) {
  const s = document.createElementNS(SVGNS, "svg");
  s.setAttribute("viewBox", "0 0 16 16");
  s.setAttribute("width", "16");
  s.setAttribute("height", "16");
  s.setAttribute("aria-hidden", "true");
  s.setAttribute("focusable", "false");
  s.setAttribute("class", "du-br-ico" + (cls ? " " + cls : ""));
  for (const d of PERCORSI[nome] || PERCORSI.doc) {
    const p = document.createElementNS(SVGNS, "path");
    p.setAttribute("d", d);
    s.append(p);
  }
  return s;
}
const PAROLE_ICONA = { finito: "prodotto finito", assieme: "assieme", particolare: "particolare", commerciale: "commerciale", minuteria: "minuteria",
  cad: "elemento CAD segnalato", noninc: "non incluso" };

// il tipo del file dal nome: PDF, DXF, STEP, o la sua estensione
function tipoFile(nome) {
  const m = /\.([A-Za-z0-9]{1,5})$/.exec(String(nome || ""));
  const e = m ? m[1].toLowerCase() : "";
  if (e === "stp" || e === "step") return "STEP";
  return e ? e.toUpperCase() : "FILE";
}

// ---- la memoria per prodotto (nella sessione: una comodità, mai uno stato del server)

const MEM = new Map(); // chiave → { aperti: {percorso: bool}, filtro, attivo (la riga col tabindex 0, solo per questa pagina) }
function memoria(chiave) {
  if (MEM.has(chiave)) return MEM.get(chiave);
  let m = { aperti: {}, filtro: "" };
  try {
    const t = sessionStorage.getItem("du-br:" + chiave);
    const j = t ? JSON.parse(t) : null;
    if (j && typeof j === "object" && j.aperti && typeof j.aperti === "object") m = { aperti: j.aperti, filtro: typeof j.filtro === "string" ? j.filtro : "" };
  } catch (e) {
    // niente memoria: si riparte da capo
  }
  MEM.set(chiave, m);
  return m;
}
function salva(chiave) {
  try {
    sessionStorage.setItem("du-br:" + chiave, JSON.stringify(MEM.get(chiave)));
  } catch (e) {
    // niente memoria: resta per questa pagina
  }
}
// browserChiuso: la scelta di chi guarda (ricordata nella sessione); senza scelta, aperto dai 1600 px di larghezza in su.
// Sotto, la tela resta larga come prima (l'organigramma si apre adattato al riquadro) e il browser è una linguetta a
// sinistra che si apre con un clic, e da lì resta aperto.
export function browserChiuso() {
  let x = null;
  try {
    x = sessionStorage.getItem("du-br-chiuso");
  } catch (e) {
    x = null;
  }
  if (x === "1" || x === "0") return x === "1";
  return window.innerWidth < 1600;
}
function segnaChiuso(si) {
  try {
    sessionStorage.setItem("du-br-chiuso", si ? "1" : "0");
  } catch (e) {
    // niente memoria
  }
}

// ---- l'albero: dalla radice per i legami, un pezzo sotto ogni suo padre (come le istanze nel browser di un CAD)

function figli(g, k) {
  return g.archi.filter((a) => a.p === k && g.nodi[a.f]);
}
function costruisci(o) {
  const g = o.g;
  const visti = new Set();
  const nodo = (k, arco, percorso, antenati, livello) => {
    visti.add(k);
    const v = g.nodi[k];
    const n = { tipo: "pezzo", k, v, arco, percorso, livello, figli: [] };
    if (!antenati.has(k)) {
      const ant = new Set(antenati).add(k);
      for (const a of figli(g, k)) n.figli.push(nodo(a.f, a, percorso + ">" + a.f, ant, livello + 1));
    }
    arr(o.documenti(v)).forEach((d, i) => n.figli.push({ tipo: "doc", k, d, percorso: percorso + "#d" + i + ":" + d.id, livello: livello + 1, figli: [] }));
    return n;
  };
  const radici = [];
  if (g.radice && g.nodi[g.radice]) radici.push(nodo(g.radice, null, g.radice, new Set(), 1));
  // i pezzi che dalla radice non si raggiungono (senza padre) e i non inclusi: in fondo, in due gruppi richiusi
  const resto = Object.keys(g.nodi).filter((k) => !visti.has(k) && !g.nodi[k].noninc);
  const sciolti = resto.filter((k) => !g.archi.some((a) => a.f === k && g.nodi[a.p]));
  if (sciolti.length) {
    const gr = { tipo: "gruppo", percorso: "@senza-padre", testo: "Pezzi senza padre", livello: 1, figli: [] };
    sciolti.forEach((k) => gr.figli.push(nodo(k, null, "@senza-padre>" + k, new Set(), 2)));
    radici.push(gr);
  }
  const ni = Object.keys(g.nodi).filter((k) => g.nodi[k].noninc);
  if (ni.length) {
    const gr = { tipo: "gruppo", percorso: "@non-inclusi", testo: "Non inclusi nella BOM", livello: 1, figli: [] };
    ni.forEach((k) => gr.figli.push(nodo(k, null, "@non-inclusi>" + k, new Set(), 2)));
    radici.push(gr);
  }
  return radici;
}
function arr(v) {
  return Array.isArray(v) ? v : [];
}
function testoDi(n) {
  if (n.tipo === "doc") return String(n.d.nome || "");
  if (n.tipo === "gruppo") return n.testo;
  return ((n.v.cod || "") + " " + (n.v.desc || "")).trim();
}
// aperto: la scelta di chi guarda, o di partenza la radice e il primo livello aperti, i gruppi in fondo chiusi
function aperto(m, n) {
  const x = m.aperti[n.percorso];
  if (typeof x === "boolean") return x;
  return n.tipo === "pezzo" && n.livello <= 2;
}

// ---- il pannello

export function creaBrowser(o) {
  const m = memoria(o.chiave);
  const chiuso = browserChiuso();
  const radici = costruisci(o);
  const lista = el("div", { class: "du-br-albero", role: "tree", id: "du-br-albero", "aria-label": "Browser della distinta: assiemi, pezzi e documenti", "aria-multiselectable": "false" });
  const filtro = el("input", { type: "search", class: "du-br-filtro", id: "du-br-filtro", placeholder: "Filtra per codice o descrizione", "aria-label": "Filtra il browser per codice o descrizione",
    autocomplete: "off", spellcheck: "false" });
  filtro.value = m.filtro;
  const conto = el("span", { class: "du-br-conto", id: "du-br-conto", "aria-live": "polite" });

  // trovati: i percorsi delle righe che il filtro trova e dei loro antenati (null senza filtro)
  function trovati() {
    const q = m.filtro.trim().toLowerCase();
    if (!q) return null;
    const s = new Set();
    let n = 0;
    const giro = (x, antenati) => {
      let dentro = false;
      if (x.tipo !== "gruppo" && testoDi(x).toLowerCase().includes(q)) {
        dentro = true;
        if (x.tipo === "pezzo") n++;
      }
      for (const f of x.figli) if (giro(f, antenati.concat(x.percorso))) dentro = true;
      if (dentro) {
        s.add(x.percorso);
        antenati.forEach((p) => s.add(p));
      }
      return dentro;
    };
    radici.forEach((r) => giro(r, []));
    s.conto = n;
    return s;
  }

  function riga(n, tr) {
    const haFigli = n.figli.length > 0;
    const ap = haFigli && (tr ? true : aperto(m, n));
    const sel = n.tipo === "pezzo" && n.k === o.scelto;
    const id = "br-" + sid(n.percorso);
    const props = { class: "du-br-riga du-br-riga--" + n.tipo + (sel ? " du-br-riga--scelta" : ""), role: "treeitem", id, tabindex: "-1", "aria-level": n.livello,
      "aria-expanded": haFigli ? (ap ? "true" : "false") : null, "aria-selected": n.tipo === "pezzo" ? (sel ? "true" : "false") : null, "data-percorso": n.percorso,
      "data-k": n.tipo === "gruppo" ? null : n.k, style: "--du-br-liv:" + (n.livello - 1) };
    const tri = haFigli ? el("span", { class: "du-br-tri" + (ap ? " du-br-tri--aperto" : ""), "data-tri": "1", "aria-hidden": "true" }, icona("freccia")) : el("span", { class: "du-br-tri du-br-tri--vuoto", "aria-hidden": "true" });
    let corpo;
    if (n.tipo === "gruppo") {
      corpo = [icona("cartella", "du-br-ico--gruppo"), el("span", { class: "du-br-nome" }, n.testo), el("span", { class: "du-br-q du-br-q--neu" }, String(n.figli.length))];
    } else if (n.tipo === "doc") {
      const tf = tipoFile(n.d.nome);
      corpo = [el("span", { class: "du-br-file du-br-file--" + tf.toLowerCase(), "aria-hidden": "true" }, icona("doc"), el("b", {}, tf)),
        el("span", { class: "du-br-nome du-br-mono", title: n.d.nome }, n.d.nome),
        n.d.stato ? el("span", { class: "du-br-st du-br-st--" + (n.d.tono || "neu") }, n.d.stato) : null];
    } else {
      const v = n.v;
      const ic = o.icona(v);
      const st = o.statoDi(v);
      const q = n.arco ? n.arco.qta : null;
      corpo = [el("span", { class: "du-br-tipo du-br-tipo--" + ic, title: PAROLE_ICONA[ic] || "" }, icona(ic)),
        el("span", { class: "du-br-nome", title: testoDi(n) }, el("span", { class: "du-br-mono du-br-cod" }, v.cod || "(senza codice)"), v.desc ? el("span", { class: "du-br-desc" }, v.desc) : null),
        q !== null && q !== undefined ? el("span", { class: "du-br-q du-br-q--" + (n.arco.kind || "prop"), title: "quantità sul legame" }, "×" + q) : null,
        st && st.testo ? el("span", { class: "du-br-st du-br-st--" + (st.tono || "neu") }, st.testo) : null];
    }
    const r = el("div", props, tri, corpo);
    if (n.tipo === "pezzo") r.setAttribute("aria-label", (PAROLE_ICONA[o.icona(n.v)] || "pezzo") + " " + testoDi(n) + (n.arco && n.arco.qta !== undefined ? ", quantità " + n.arco.qta : ""));
    return { r, ap };
  }

  function disegna() {
    const tr = trovati();
    const frag = document.createDocumentFragment();
    let prima = null;
    let haAttivo = false;
    const giro = (n, gruppo) => {
      if (tr && !tr.has(n.percorso)) return;
      const { r, ap } = riga(n, tr);
      gruppo.append(r);
      if (!prima) prima = n.percorso;
      if (n.percorso === m.attivo) haAttivo = true;
      if (n.figli.length && ap) {
        const sotto = el("div", { role: "group", class: "du-br-gruppo" });
        n.figli.forEach((f) => giro(f, sotto));
        gruppo.append(sotto);
      }
    };
    radici.forEach((n) => giro(n, frag));
    lista.replaceChildren(frag);
    if (!lista.children.length) lista.append(el("p", { class: "du-br-vuoto", id: "du-br-vuoto" }, tr ? "Nessun pezzo trovato con «" + m.filtro.trim() + "»." : "Nessun pezzo da mostrare."));
    conto.textContent = tr ? tr.conto + (tr.conto === 1 ? " pezzo trovato" : " pezzi trovati") : "";
    // il tabindex 0 sulla riga attiva, o sulla riga del pezzo scelto, o sulla prima
    let ra = haAttivo ? lista.querySelector('[data-percorso="' + cssEsc(m.attivo) + '"]') : null;
    // il pezzo scelto è cambiato altrove (sulla tela, nell'Elenco): la riga attiva diventa la sua
    if (!ra || (o.scelto && ra.dataset.k !== o.scelto && !lista.contains(document.activeElement))) {
      const s = o.scelto ? lista.querySelector('.du-br-riga[aria-selected="true"]') : null;
      if (s || !ra) {
        m.attivo = s ? s.dataset.percorso : prima;
        ra = m.attivo && lista.querySelector('[data-percorso="' + cssEsc(m.attivo) + '"]');
      }
    }
    if (ra) ra.tabIndex = 0;
  }
  function cssEsc(s) {
    return window.CSS && CSS.escape ? CSS.escape(s) : String(s).replace(/["\\]/g, "\\$&");
  }
  function righe() {
    return Array.from(lista.querySelectorAll(".du-br-riga"));
  }
  function fuoco(r) {
    if (!r) return;
    righe().forEach((x) => (x.tabIndex = -1));
    r.tabIndex = 0;
    m.attivo = r.dataset.percorso;
    r.focus({ preventScroll: true });
    // la riga in vista dentro il riquadro, senza far scorrere la pagina
    const a = lista.getBoundingClientRect(), b = r.getBoundingClientRect();
    if (b.top < a.top) lista.scrollTop -= a.top - b.top + 4;
    else if (b.bottom > a.bottom) lista.scrollTop += b.bottom - a.bottom + 4;
  }
  function trovaNodo(percorso) {
    let t = null;
    const giro = (n) => {
      if (t) return;
      if (n.percorso === percorso) t = n;
      else n.figli.forEach(giro);
    };
    radici.forEach(giro);
    return t;
  }
  function apri(percorso, si) {
    m.aperti[percorso] = si;
    salva(o.chiave);
    const pr = document.activeElement && lista.contains(document.activeElement) ? document.activeElement.dataset.percorso : null;
    const top = lista.scrollTop;
    disegna();
    lista.scrollTop = top;
    if (pr) fuoco(lista.querySelector('[data-percorso="' + cssEsc(pr) + '"]'));
  }
  function tutti(si) {
    const giro = (n) => {
      if (n.figli.length && n.tipo !== "doc") m.aperti[n.percorso] = si;
      n.figli.forEach(giro);
    };
    radici.forEach(giro);
    salva(o.chiave);
    disegna();
  }
  function scegliRiga(r) {
    m.attivo = r.dataset.percorso;
    if (r.dataset.k && r.dataset.k !== o.scelto) o.scegli(r.dataset.k);
  }

  lista.addEventListener("click", (e) => {
    const r = e.target.closest(".du-br-riga");
    if (!r) return;
    const n = trovaNodo(r.dataset.percorso);
    if (!n) return;
    if (e.target.closest("[data-tri]") || n.tipo === "gruppo") {
      apri(n.percorso, !(r.getAttribute("aria-expanded") === "true"));
      return;
    }
    fuoco(r);
    scegliRiga(r);
  });
  lista.addEventListener("dblclick", (e) => {
    const r = e.target.closest(".du-br-riga");
    if (r && r.dataset.k && !e.target.closest("[data-tri]") && o.centra) o.centra(r.dataset.k);
  });
  lista.addEventListener("keydown", (e) => {
    const r = e.target.closest(".du-br-riga");
    if (!r) return;
    const tutte = righe();
    const i = tutte.indexOf(r);
    const n = trovaNodo(r.dataset.percorso);
    const esp = r.getAttribute("aria-expanded");
    let fatto = true;
    if (e.key === "ArrowDown") fuoco(tutte[i + 1]);
    else if (e.key === "ArrowUp") fuoco(tutte[i - 1]);
    else if (e.key === "Home") fuoco(tutte[0]);
    else if (e.key === "End") fuoco(tutte[tutte.length - 1]);
    else if (e.key === "ArrowRight") {
      if (esp === "false") apri(n.percorso, true);
      else if (esp === "true") fuoco(tutte[i + 1]);
    } else if (e.key === "ArrowLeft") {
      if (esp === "true") apri(n.percorso, false);
      else {
        const g = r.parentElement && r.parentElement.closest('[role="group"]');
        const p = g && g.previousElementSibling;
        if (p && p.classList.contains("du-br-riga")) fuoco(p);
      }
    } else if (e.key === "Enter") {
      if (n && n.tipo === "gruppo") apri(n.percorso, esp !== "true");
      else {
        scegliRiga(r);
        if (r.dataset.k && o.centra) o.centra(r.dataset.k);
      }
    } else if (e.key === " ") {
      if (n && n.tipo === "gruppo") apri(n.percorso, esp !== "true");
      else scegliRiga(r);
    } else fatto = false;
    if (fatto) {
      e.preventDefault();
      e.stopPropagation();
    }
  });
  let timerFiltro = null;
  filtro.addEventListener("input", () => {
    clearTimeout(timerFiltro);
    // il testo va subito nella memoria (un ridisegno della pagina negli ultimi istanti non lo perde); l'elenco dopo una pausa
    m.filtro = filtro.value;
    timerFiltro = setTimeout(() => {
      salva(o.chiave);
      lista.scrollTop = 0;
      disegna();
    }, 120);
  });
  filtro.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      fuoco(lista.querySelector(".du-br-riga"));
    } else if (e.key === "Escape" && filtro.value) {
      e.preventDefault();
      e.stopPropagation();
      filtro.value = "";
      m.filtro = "";
      salva(o.chiave);
      disegna();
    }
  });

  disegna();
  const pannello = el("aside", { class: "du-br" + (chiuso ? " du-br--chiuso" : ""), id: "du-br", "aria-label": "Browser della distinta" });
  const chiudi = el("button", { type: "button", class: "du-br-chiudi", id: "du-br-chiudi", "aria-expanded": chiuso ? "false" : "true", "aria-controls": "du-br-corpo",
    title: chiuso ? "Apri il browser della distinta" : "Richiudi il browser della distinta" }, icona("pannello"), el("span", { class: "du-br-chiudi-t" }, "Browser"));
  const corpo = el("div", { class: "du-br-corpo", id: "du-br-corpo", hidden: chiuso },
    el("div", { class: "du-br-testa" },
      el("div", { class: "du-br-gesti", role: "group", "aria-label": "Apri e chiudi i rami" },
        el("button", { type: "button", class: "du-br-btn", id: "du-br-espandi", onclick: () => tutti(true) }, "Espandi tutto"),
        el("button", { type: "button", class: "du-br-btn", id: "du-br-comprimi", onclick: () => tutti(false) }, "Comprimi tutto")),
      el("div", { class: "du-br-cerca" }, filtro, conto)),
    lista);
  chiudi.addEventListener("click", () => {
    const ora = !pannello.classList.contains("du-br--chiuso");
    segnaChiuso(ora);
    pannello.classList.toggle("du-br--chiuso", ora);
    pannello.parentElement && pannello.parentElement.classList.toggle("du-orgbr--chiuso", ora);
    corpo.hidden = ora;
    chiudi.setAttribute("aria-expanded", ora ? "false" : "true");
    chiudi.title = ora ? "Apri il browser della distinta" : "Richiudi il browser della distinta";
  });
  pannello.append(chiudi, corpo);
  return pannello;
}
