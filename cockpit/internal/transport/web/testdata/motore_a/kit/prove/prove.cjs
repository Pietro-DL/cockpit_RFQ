// L1 — giro 5, A1d: la Distinta nuova in jsdom, sulle pagine rese dal rendering L1 (QFE-03; K-04; piano del frontend
// §9.2, raccordo RAC-02, RAC-03). Prova il COMPORTAMENTO della pagina (motore_a.mjs sul markup di motore_a.html), non il
// server: le risposte dei gesti le dà il ponte finto (prove/ponte_finto.mjs), i PDF un pdf.js finto
// (prove/pdfjs_finto.cjs) con i PDF sintetici ACME di pdf/. Nessuna rete, nessun DB, nessun browser.
//
// USO, dalla cartella cockpit/:
//   MA_HTML=<cartella> go test -count=1 -run '^TestMotoreAPaginaEFrammentoPerOgniCaso$' ./internal/transport/web/
//   NODE_PATH=<cartella con node_modules/jsdom> node internal/transport/web/testdata/motore_a/kit/prove/prove.cjs <cartella>
// <cartella> è quella scritta dal rendering L1 (QFE-02): le pagine, i frammenti e casi.json, un elenco di
// {pagina, frammento, caso, vista, prodotto, scrive}. Esce con 0 se ogni controllo passa, 1 se uno fallisce, 2 se mancano
// le pagine, jsdom o i file da provare; scrive i risultati in <cartella>/prove.json (mai nel repository).
// Variabili: MA_NEGATI=<file FUORI dal repository, un nome per riga> (i nomi da non trovare mai: si stampano solo
// mascherati); SOLO=<espressione regolare sui nomi dei gruppi>; MA_COCKPIT=<cartella cockpit/> (predefinita: sette
// livelli sopra questa cartella); MA_CASI=<cartella dei casi e*_*.json> (predefinita: testdata/motore_a/).
// Senza pagine: `node prove.cjs --solo-kit` controlla solo i dati privati nei file del kit e nei casi ACME.
//
// COME SI CARICANO I MODULI. jsdom non esegue <script type="module">: la pagina si apre con runScripts:
// "outside-only" (nessuno script della pagina parte da solo, né htmx né lo script del layout) e i due moduli si valutano
// con window.eval dopo una trasformazione del testo, nell'ordine del browser:
//   - il ponte finto: si tolgono gli `export` e le quattro funzioni dell'API vanno in window.__Gesti;
//   - motore_a.mjs: la riga `import * as Gesti from "./motore_a_gesti.mjs";` diventa `const Gesti = window.__Gesti;` e
//     l'unica import() con un argomento (il caricatore di pdf.js) diventa window.__importa(…), che dà il pdf.js finto.
// Se le forme attese non ci sono, la prova si ferma con l'uscita 2: mai un passaggio silenzioso.
//
// I DOPPI (jsdom 24 non li ha): IntersectionObserver finto (registra e fa scattare a comando: vedi()), ResizeObserver
// assente (il codice lo tollera), URL.createObjectURL/revokeObjectURL spie, HTMLCanvasElement.toBlob che dà un Blob,
// getContext che non disegna, HTMLDialogElement.showModal/close, scrollIntoView spia, CSS.escape assente (vale il
// ripiego del modulo), matchMedia e larghezze a comando (le prove le impostano: in jsdom ogni larghezza è 0), un
// orologio finto (Date.now e i setTimeout da 1 s in su), fetch spia che risponde alle sole GET /allegato/<id>/anteprima
// come il server (con HX-Request: 400), htmx.ajax spia che fa la rilettura: htmx:beforeSwap, #ma sostituita con il
// frammento dello stesso caso, htmx:afterSwap (sul body, come htmx 2 con htmx.ajax senza sorgente).
"use strict";

const fs = require("fs");
const path = require("path");

// ------------------------------------------------------------------ argomenti e percorsi

const USO = [
  "uso: NODE_PATH=<cartella con node_modules/jsdom> node prove.cjs <cartella delle pagine>",
  "  <cartella delle pagine>: quella di MA_HTML=<cartella> go test -count=1 -run '^TestMotoreAPaginaEFrammentoPerOgniCaso$' ./internal/transport/web/",
  "  (dalla cartella cockpit/), con casi.json: [{pagina, frammento, caso, vista, prodotto, scrive}, …]",
  "  variabili: MA_NEGATI=<file fuori dal repository>  SOLO=<regex dei gruppi>  MA_COCKPIT=<cartella cockpit/>  MA_CASI=<cartella dei casi>",
  "  oppure:    node prove.cjs --solo-kit   (solo i dati privati nei file del kit e nei casi ACME)",
].join("\n");

function esci(codice, ...righe) {
  for (const r of righe) console.error(r);
  process.exit(codice);
}

const ARGOMENTI = process.argv.slice(2);
const SOLO_KIT = ARGOMENTI.includes("--solo-kit");
const OUT = ARGOMENTI.find((a) => !a.startsWith("--")) || "";
if (!SOLO_KIT) {
  if (!OUT) esci(2, "prove.cjs: manca la cartella delle pagine rese.", USO);
  if (!fs.existsSync(OUT) || !fs.statSync(OUT).isDirectory()) esci(2, "prove.cjs: la cartella delle pagine non c'è: " + OUT, USO);
  if (!fs.existsSync(path.join(OUT, "casi.json"))) esci(2, "prove.cjs: in " + OUT + " manca casi.json: non è la cartella scritta da MA_HTML.", USO);
}

const KIT = path.resolve(__dirname, "..");
// da kit/prove/ sette livelli portano a cockpit/ (kit, motore_a, testdata, web, transport, internal, cockpit: RAC-03)
const COCKPIT = path.resolve(process.env.MA_COCKPIT || path.join(__dirname, "..", "..", "..", "..", "..", "..", ".."));
const CASI_DIR = path.resolve(process.env.MA_CASI || path.join(KIT, ".."));
const F_MJS = path.join(COCKPIT, "web", "static", "motore_a.mjs");
const F_PONTE = path.join(COCKPIT, "web", "static", "motore_a_gesti.mjs");
const F_FINTO = path.join(__dirname, "ponte_finto.mjs");
const F_PDF1 = path.join(KIT, "pdf", "acme_disegno_a4.pdf");
const F_PDF2 = path.join(KIT, "pdf", "acme_disegno_2pagine.pdf");

const { creaPdfjsFinto } = require("./pdfjs_finto.cjs");

// ------------------------------------------------------------------ costanti ACME e forme

const U = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const RE_UUID = new RegExp("^" + U + "$");
const RE_UUID_G = new RegExp(U, "g");
const RE_PDF = new RegExp("^/allegato/(" + U + ")/anteprima$");
const RE_SCELTA = new RegExp("^(aggiungi|sostituisci:" + U + ")$");
const ORIGINE = "http://cockpit.acme.example"; // RAC-14: acme.example, mai un host vero
// gli UUID dei casi ACME: 00000000-0000-4000-8000-0000000000nn (piano del frontend §8.1, D-PF-18)
const ACME = (nn) => "00000000-0000-4000-8000-0000000000" + nn;
const RE_UUID_ACME = /^00000000-0000-4000-8000-0000000000[0-9a-f]{2}$/i;
const UUID_ZERO = "00000000-0000-0000-0000-000000000000";
const FRASE_DB7_2 = /la struttura non è ancora verificata/i; // DB7-2, FE-12: solo con BOM.Verificata falso
const FRASE_CAND = /proposta dal file, non è la BOM/i;
const SELETTORI_VECCHI = [".ma-prossimo", ".ma-steps", ".ma-navsteps", ".ma-dviste", ".ma-conta-tipi", ".ma-percorso", ".ma-passi"];

const attendi = (ms = 15) => new Promise((r) => setTimeout(r, ms));
const testo = (e) => (e ? String(e.textContent || "").replace(/\s+/g, " ").trim() : "");
const conId = (s) => String(s || "").replace(RE_UUID_G, "{u}");
const uniche = (a) => [...new Set(a)];

// ------------------------------------------------------------------ i dati privati (anche senza pagine)

// I nomi di MA_NEGATI non si stampano mai: solo le prime due lettere, l'ultima e la lunghezza (come lo scanner dei
// privati). Il file sta fuori dal repository; senza la variabile il controllo dei nomi non si fa (e si dice).
function leggiNegati() {
  const f = process.env.MA_NEGATI;
  if (!f) return { nomi: [], nota: "MA_NEGATI non data: nessun elenco di nomi da cercare" };
  if (!fs.existsSync(f)) esci(2, "prove.cjs: MA_NEGATI indica un file che non c'è.");
  if (path.resolve(f).startsWith(path.resolve(COCKPIT, "..") + path.sep)) esci(2, "prove.cjs: il file di MA_NEGATI deve stare FUORI dal repository.");
  const nomi = fs.readFileSync(f, "utf8").split(/\r?\n/).map((x) => x.trim()).filter((x) => x.length > 2 && !x.startsWith("#"));
  return { nomi, nota: `MA_NEGATI: ${nomi.length} nomi` };
}
const maschera = (t) => (t.length <= 3 ? t[0] + "…(" + t.length + ")" : t.slice(0, 2) + "…" + t.slice(-1) + "(" + t.length + ")");

// cerca i nomi senza badare alle maiuscole, a parola intera (lettere e cifre ai bordi non contano come confine)
function trovaNegati(contenuto, nomi) {
  const basso = contenuto.toLowerCase();
  const fuori = [];
  for (const n of nomi) {
    const x = n.toLowerCase();
    let i = basso.indexOf(x);
    while (i >= 0) {
      const prima = i > 0 ? basso[i - 1] : " ";
      const dopo = i + x.length < basso.length ? basso[i + x.length] : " ";
      if (!/[a-z0-9]/.test(prima) && !/[a-z0-9]/.test(dopo)) {
        fuori.push(maschera(n));
        break;
      }
      i = basso.indexOf(x, i + 1);
    }
  }
  return fuori;
}

// testi che non devono stare in un file del kit, in un caso o in una pagina resa
const VIETATI = [
  [/\/home\/|\/tmp\/claude|scratchpad|\\Users\\|[A-Za-z]:\\\\/i, "un percorso di una macchina"],
  [/[A-Za-z0-9._%+-]+@(?!acme\.example\b)[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[a-z]{2,}/i, "un indirizzo email non ACME"],
  // ACME (anche un sottodominio di acme.example, come l'«altra origine» di ponte.cjs) e il loopback senza porta di
  // ponte.cjs («indirizzo non valido») non sono dati da non pubblicare (integrazione di A1d-Q3)
  [/\bhttps?:\/\/(?!(?:[a-z0-9-]+\.)*acme\.example\b|\[::1)[^\s"'<>)]+/i, "un indirizzo web"],
  [/\b(?!acme\b)[a-z0-9-]+\.(?:it|com|fr|de|eu|net|org|io)\b/i, "un dominio"],
  [/\bpromatec\b/i, "il nome dell'azienda"],
  // la cartella delle specifiche private; scritta «doc[s]» perché nemmeno questa riga ne contenga il percorso
  [/(^|[\s(`"'])doc[s]\//, "un percorso della cartella delle specifiche"],
];
function controllaTesto(contenuto) {
  const fuori = [];
  for (const [re, cosa] of VIETATI) {
    const m = contenuto.match(re);
    if (m) fuori.push(cosa);
  }
  return fuori;
}

function fileDelKit() {
  const elenco = [];
  const visita = (dir) => {
    for (const f of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, f.name);
      if (f.isDirectory()) {
        if (f.name !== "node_modules") visita(p);
      } else elenco.push(p);
    }
  };
  visita(KIT);
  return elenco.sort();
}

function fileDeiCasi() {
  if (!fs.existsSync(CASI_DIR)) return [];
  return fs.readdirSync(CASI_DIR).filter((f) => /^e\d+[a-z]?_.*\.json$/.test(f)).sort().map((f) => path.join(CASI_DIR, f));
}

// i valori dei casi che sono codici, file, clienti: solo ACME (famiglia 712xxxx, RAC-27) o norme
const CHIAVI_CODICE = /^(codice|codice_richiesto|codice_proposto|codice_manuale|nome_file|riferimento_cliente|cliente|oggetto)$/;
function codiciStrani(j) {
  const strani = [];
  const visita = (x, chiave) => {
    if (Array.isArray(x)) return x.forEach((y) => visita(y, chiave));
    if (x && typeof x === "object") return Object.entries(x).forEach(([k, v]) => visita(v, k));
    if (typeof x !== "string" || !x || !CHIAVI_CODICE.test(chiave || "")) return;
    // nei nomi e nei codici, ogni numero lungo è della famiglia ACME 712xxxx; ogni sigla con un numero è ACME o una norma
    for (const n of x.match(/\d{5,}/g) || []) if (!/^712\d*$/.test(n)) strani.push(chiave);
    for (const s of x.match(/\b[A-Z]{2,}[-_]?\d[\w-]*/g) || []) if (!/^(ACME|ISO|DIN|UNI|EN)/.test(s)) strani.push(chiave);
  };
  visita(j, "");
  return uniche(strani);
}

function uuidNonAcme(contenuto) {
  return (contenuto.match(RE_UUID_G) || []).filter((u) => !RE_UUID_ACME.test(u) && u !== UUID_ZERO).length;
}

// ------------------------------------------------------------------ il registro dei controlli

const risultati = [];
const note = new Set();
let gruppo = "";
function ok(cond, msg, extra) {
  const r = { gruppo, msg, ok: !!cond };
  if (!cond && extra !== undefined && extra !== "") r.extra = String(extra).slice(0, 500);
  risultati.push(r);
  if (!cond) console.log(`  FALLITA [${gruppo}] ${msg}${r.extra ? " · " + r.extra : ""}`);
  return !!cond;
}

function scriviEsito() {
  const falliti = risultati.filter((r) => !r.ok).length;
  const gruppi = uniche(risultati.map((r) => r.gruppo));
  const gruppiKo = gruppi.filter((g) => risultati.some((r) => r.gruppo === g && !r.ok)).length;
  console.log(`\nprove: ${gruppi.length - gruppiKo}/${gruppi.length} gruppi passati · controlli: ${risultati.length - falliti} passati, ${falliti} falliti`);
  if (note.size) console.log("note: " + [...note].slice(0, 12).join(" | "));
  const dove = SOLO_KIT ? null : path.join(OUT, "prove.json");
  if (dove) fs.writeFileSync(dove, JSON.stringify({ gruppi: gruppi.length, gruppiKo, controlli: risultati.length, falliti, note: [...note], risultati }, null, 1));
  return falliti;
}

// ------------------------------------------------------------------ privati: file del kit e casi ACME

function provaPrivatiDelKit(negati) {
  const kit = fileDelKit();
  ok(kit.length >= 8, `il kit ha i suoi file (${kit.length})`);
  for (const f of kit) {
    const rel = path.relative(KIT, f);
    ok(!/\.go$|(^|[\\/])go\.(mod|sum)$|(^|[\\/])package(-lock)?\.json$/.test(rel), `${rel}: nel kit nessun .go, go.mod, go.sum o package.json (RAC-03)`);
    const contenuto = fs.readFileSync(f, "utf8");
    // questo file contiene le espressioni stesse: per lui si cercano solo i nomi negati
    if (f !== __filename) {
      const v = controllaTesto(contenuto);
      ok(v.length === 0, `${rel}: nessun dato da non pubblicare`, v.join(", "));
    }
    const n = trovaNegati(contenuto, negati.nomi);
    ok(n.length === 0, `${rel}: nessun nome dell'elenco MA_NEGATI`, n.join(", "));
  }
  const casi = fileDeiCasi();
  if (!casi.length) note.add("nessun caso e*_*.json in " + path.basename(CASI_DIR) + "/: il controllo dei casi non si fa");
  for (const f of casi) {
    const nome = path.basename(f);
    const contenuto = fs.readFileSync(f, "utf8");
    const v = controllaTesto(contenuto);
    ok(v.length === 0, `${nome}: nessun dato da non pubblicare`, v.join(", "));
    ok(uuidNonAcme(contenuto) === 0, `${nome}: solo UUID ACME (…0000000000nn)`, `${uuidNonAcme(contenuto)} UUID diversi`);
    let j = null;
    try { j = JSON.parse(contenuto); } catch (e) { /* lo dice il controllo sotto */ }
    if (ok(!!j, `${nome}: JSON leggibile`)) {
      const strani = codiciStrani(j);
      ok(strani.length === 0, `${nome}: codici, file e cliente solo ACME (712xxxx) o norme`, strani.join(", "));
    }
    const n = trovaNegati(contenuto, negati.nomi);
    ok(n.length === 0, `${nome}: nessun nome dell'elenco MA_NEGATI`, n.join(", "));
  }
}

if (SOLO_KIT) {
  gruppo = "privati (solo kit)";
  const negati = leggiNegati();
  note.add(negati.nota);
  provaPrivatiDelKit(negati);
  process.exit(scriviEsito() ? 1 : 0);
}

// ------------------------------------------------------------------ jsdom, i casi, i moduli

let JSDOM;
let VirtualConsole;
try {
  ({ JSDOM, VirtualConsole } = require("jsdom"));
} catch (e) {
  esci(2, "prove.cjs: jsdom non si carica. Si installa FUORI dal repository e si indica con NODE_PATH=<cartella con node_modules>.");
}

for (const f of [F_MJS, F_FINTO, F_PDF1, F_PDF2]) if (!fs.existsSync(f)) esci(2, "prove.cjs: manca " + f);
const PDF1 = fs.readFileSync(F_PDF1);
const PDF2 = fs.readFileSync(F_PDF2);

function leggiCasi() {
  let elenco;
  try {
    elenco = JSON.parse(fs.readFileSync(path.join(OUT, "casi.json"), "utf8"));
  } catch (e) {
    esci(2, "prove.cjs: casi.json non si legge: " + e.message);
  }
  if (!Array.isArray(elenco) || !elenco.length) esci(2, "prove.cjs: casi.json deve essere un elenco non vuoto di {pagina, frammento, caso, vista, prodotto, scrive}.");
  elenco.forEach((v, i) => {
    const manca = ["pagina", "caso", "vista"].filter((k) => typeof v[k] !== "string" || !v[k]);
    if (manca.length || typeof v.scrive !== "boolean") esci(2, `prove.cjs: casi.json[${i}]: mancano ${manca.concat(typeof v.scrive === "boolean" ? [] : ["scrive"]).join(", ")}.`);
    if (!["distinta", "documenti", "rfq"].includes(v.vista)) esci(2, `prove.cjs: casi.json[${i}]: vista «${v.vista}» sconosciuta.`);
    if (!fs.existsSync(path.join(OUT, v.pagina))) esci(2, `prove.cjs: casi.json[${i}]: la pagina ${v.pagina} non c'è.`);
    if (v.frammento && !fs.existsSync(path.join(OUT, v.frammento))) esci(2, `prove.cjs: casi.json[${i}]: il frammento ${v.frammento} non c'è.`);
    if (typeof v.prodotto !== "string") v.prodotto = "";
    if (typeof v.frammento !== "string") v.frammento = "";
  });
  return elenco;
}
const CASI = leggiCasi();

const casiJSON = new Map();
function casoJSON(caso) {
  if (!casiJSON.has(caso)) {
    let j = null;
    try { j = JSON.parse(fs.readFileSync(path.join(CASI_DIR, caso + ".json"), "utf8")); } catch (e) { j = null; }
    casiJSON.set(caso, j);
  }
  return casiJSON.get(caso);
}
function prodottoDelCaso(caso, rif) {
  const j = casoJSON(caso);
  const pv = j && j.pagina && j.pagina.esito && j.pagina.esito.prodotti_valutati;
  return Array.isArray(pv) ? pv.find((p) => p && p.rif === rif) || null : null;
}
function bomVerificata(caso, rif) {
  const p = prodottoDelCaso(caso, rif);
  return p && p.bom ? !!p.bom.verificata : null;
}
function esitoNil(caso) {
  const j = casoJSON(caso);
  return j && j.pagina ? j.pagina.esito == null : null;
}

// il ponte finto, senza export, con l'API in window.__Gesti
function codicePonteFinto() {
  let c = fs.readFileSync(F_FINTO, "utf8");
  if (/^\s*import\s/m.test(c)) esci(2, "prove.cjs: il ponte finto importa qualcosa: la trasformazione non basta.");
  c = c.replace(/^export\s+(?=(async\s+)?function\b)/gm, "");
  if (/^\s*export\s/m.test(c)) esci(2, "prove.cjs: il ponte finto ha export che la trasformazione non toglie.");
  return `(function () {\n"use strict";\n${c}\nwindow.__Gesti = { esegui, leggi, confermaAlbero, aggiorna };\n})();\n//# sourceURL=ponte_finto.mjs`;
}

// motore_a.mjs: il ponte da window.__Gesti, la sola import() di pdf.js verso window.__importa
function codiceMotore() {
  const c = fs.readFileSync(F_MJS, "utf8");
  const riga = /^import \* as Gesti from "\.\/motore_a_gesti\.mjs";[ \t]*$/m;
  if (!riga.test(c)) esci(2, "prove.cjs: in motore_a.mjs manca la riga «import * as Gesti from \"./motore_a_gesti.mjs\";» (§7.2).");
  let t = c.replace(riga, "const Gesti = window.__Gesti;");
  // l'import() del caricatore ha un argomento (import(src)); «import()» nei commenti non conta
  const chiamate = t.match(/\bimport\s*\(\s*[A-Za-z_$][\w$.]*\s*\)/g) || [];
  if (chiamate.length !== 1) esci(2, `prove.cjs: in motore_a.mjs è attesa una sola import(<indirizzo>) (il caricatore di pdf.js, §7.4); trovate ${chiamate.length}.`);
  t = t.replace(/\bimport\s*\(\s*([A-Za-z_$][\w$.]*)\s*\)/, "window.__importa($1)");
  if (/^\s*(import|export)\s/m.test(t)) esci(2, "prove.cjs: motore_a.mjs ha altri import o export: la trasformazione non basta.");
  return `(function () {\n"use strict";\n${t}\n})();\n//# sourceURL=motore_a.mjs`;
}

// le liste chiuse del ponte vero, fra // ROTTE:inizio e // ROTTE:fine (e LETTURE): una fonte sola (RAC-13)
function listeDelPonte() {
  if (!fs.existsSync(F_PONTE)) return { rotte: null, letture: null, errore: "manca web/static/motore_a_gesti.mjs (il ponte vero, A1d-P2)" };
  const c = fs.readFileSync(F_PONTE, "utf8");
  const blocco = (nome) => {
    const m = c.match(new RegExp("//\\s*" + nome + ":inizio[\\s\\S]*?//\\s*" + nome + ":fine"));
    return m ? [...m[0].matchAll(/"(\/[^"\s]+)"/g)].map((x) => x[1]) : null;
  };
  const rotte = blocco("ROTTE");
  const letture = blocco("LETTURE");
  const errore = !rotte || !rotte.length ? "nel ponte vero non c'è il blocco ROTTE" : !letture || !letture.length ? "nel ponte vero non c'è il blocco LETTURE" : "";
  return { rotte, letture, errore };
}
// compila: «/thread/{id}/fascicolo/conferma» → la sorgente di ^/thread/(<uuid>)/fascicolo/conferma$ ({id} catturato,
// {u} un UUID qualunque). Si spezza sui segnaposto prima di proteggere i caratteri speciali: le graffe dei quantificatori
// di U ({8}, {4}, {12}) restano quantificatori.
function compila(s) {
  const protetto = (x) => x.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const pezzi = String(s).split(/(\{id\}|\{u\})/);
  return "^" + pezzi.map((x) => (x === "{id}" ? "(" + U + ")" : x === "{u}" ? U : protetto(x))).join("") + "$";
}
const LISTE = listeDelPonte();
const ROTTE = LISTE.rotte ? LISTE.rotte.map(compila) : null;
const LETTURE = LISTE.letture ? LISTE.letture.map(compila) : null;
function nellaLista(rotta, tid) {
  if (!ROTTE) return false;
  let p;
  try { p = new URL(rotta, ORIGINE + "/"); } catch (e) { return false; }
  if (p.origin !== ORIGINE || p.search) return false;
  return ROTTE.some((s) => {
    const m = p.pathname.match(new RegExp(s));
    return !!m && (!m[1] || m[1].toLowerCase() === String(tid).toLowerCase());
  });
}

const PONTE_FINTO = codicePonteFinto();
const MOTORE = codiceMotore();

// ------------------------------------------------------------------ la pagina in jsdom

function threadDi(voce, html) {
  const m = html.match(/data-ma-thread="([0-9a-fA-F-]{36})"/);
  if (m) return m[1];
  const j = casoJSON(voce.caso);
  return (j && j.pagina && j.pagina.thread_id) || ACME("01");
}

function intestazioniDi(h) {
  const out = {};
  if (!h) return out;
  if (typeof h.forEach === "function" && !Array.isArray(h)) h.forEach((v, k) => { out[String(k).toLowerCase()] = String(v); });
  else if (Array.isArray(h)) for (const [k, v] of h) out[String(k).toLowerCase()] = String(v);
  else for (const [k, v] of Object.entries(h)) out[String(k).toLowerCase()] = String(v);
  return out;
}

function risposta(s, stato, tipo, corpo, { redirected = false, url = "" } = {}) {
  const b = Buffer.isBuffer(corpo) ? corpo : Buffer.from(String(corpo || ""), "utf8");
  const h = { "content-type": tipo, "x-content-type-options": "nosniff" };
  const Uint8 = s.w.Uint8Array || s.w.eval("Uint8Array");
  return {
    ok: stato >= 200 && stato < 300,
    status: stato,
    statusText: String(stato),
    redirected,
    url,
    type: "basic",
    headers: { get: (k) => (String(k).toLowerCase() in h ? h[String(k).toLowerCase()] : null), has: (k) => String(k).toLowerCase() in h },
    arrayBuffer: () => {
      const a = new Uint8(b.length);
      a.set(b);
      return Promise.resolve(a.buffer);
    },
    text: () => Promise.resolve(b.toString("utf8")),
    json: () => Promise.resolve(JSON.parse(b.toString("utf8"))),
    clone() {
      return risposta(s, stato, tipo, b, { redirected, url });
    },
  };
}

// le risposte del server per GET /allegato/<a>/anteprima (anteprima.go): 200 PDF; 404, 409, 415 con il testo; il
// rinvio al login dopo una sessione scaduta; una pagina HTML al posto del PDF
const SERVER = {
  pdf: () => ({ stato: 200, tipo: "application/pdf", corpo: PDF1 }),
  pdf2: () => ({ stato: 200, tipo: "application/pdf", corpo: PDF2 }),
  404: () => ({ stato: 404, tipo: "text/plain; charset=utf-8", corpo: "il contenuto non e' su questo server: usa «Riscarica»\n" }),
  409: () => ({ stato: 409, tipo: "text/plain; charset=utf-8", corpo: "il file sul NAS ha una segnalazione aperta: risolvila in Admin prima di guardarlo\n" }),
  415: () => ({ stato: 415, tipo: "text/plain; charset=utf-8", corpo: "anteprima disponibile solo per i PDF\n" }),
  login: () => ({ stato: 200, tipo: "text/html; charset=utf-8", corpo: "<!doctype html><title>Accesso</title><form action=\"/login\"></form>", redirected: true, url: ORIGINE + "/login" }),
  html: () => ({ stato: 200, tipo: "text/html; charset=utf-8", corpo: "<!doctype html><p>non un PDF (ACME)</p>" }),
};

function fetchFinto(s) {
  return function fetch(entrata, init) {
    const o = init || {};
    const grezzo = typeof entrata === "string" ? entrata : entrata && entrata.url ? entrata.url : String(entrata);
    const metodo = String(o.method || (entrata && entrata.method) || "GET").toUpperCase();
    const intestazioni = intestazioniDi(o.headers || (entrata && entrata.headers));
    let u = null;
    try { u = new URL(grezzo, s.w.location.href); } catch (e) { u = null; }
    const m = u && u.origin === ORIGINE ? u.pathname.match(RE_PDF) : null;
    const voce = {
      url: u ? u.pathname + u.search : grezzo,
      metodo,
      intestazioni,
      credenziali: o.credentials,
      corpo: o.body !== undefined && o.body !== null,
      allegato: m ? m[1].toLowerCase() : "",
      v: u ? u.searchParams.get("v") || "" : "",
      soloV: u ? [...u.searchParams.keys()].every((k) => k === "v") : false,
    };
    s.spie.fetch.push(voce);
    if (typeof s.alFetch === "function") s.alFetch(voce);
    if (!m || metodo !== "GET" || voce.corpo) return Promise.reject(new TypeError("rete spenta nelle prove: " + metodo + " " + conId(voce.url)));
    if (Object.keys(intestazioni).some((k) => k.startsWith("hx-"))) {
      return Promise.resolve(risposta(s, 400, "text/plain; charset=utf-8", "l'anteprima si apre come documento, non dentro la pagina\n"));
    }
    const piano = (typeof s.server === "function" && s.server(voce)) || SERVER.pdf();
    return Promise.resolve(risposta(s, piano.stato, piano.tipo, piano.corpo, piano));
  };
}

// la rilettura di htmx.ajax("GET", location.href, {target: "#ma", swap: "innerHTML"}): il frammento dello stesso caso
async function rilettura(s, verbo, url, o) {
  const opz = o || {};
  const voce = {
    verbo: String(verbo).toUpperCase(),
    url: String(url),
    target: typeof opz.target === "string" ? opz.target : opz.target && opz.target.id ? "#" + opz.target.id : "",
    swap: opz.swap || "",
    daProva: !!s.daProva,
  };
  s.spie.ajax.push(voce);
  if (s.ajaxRifiuta) throw new Error("rilettura rifiutata dalla prova");
  const { w, d } = s;
  const ma = d.getElementById("ma");
  if (voce.verbo !== "GET" || voce.target !== "#ma" || !ma || !s.voce.frammento) {
    voce.esito = "inattesa";
    return;
  }
  const html = fs.readFileSync(path.join(OUT, s.voce.frammento), "utf8");
  const config = { verb: "get", path: voce.url, target: ma };
  if (s.eltRilettura) config.elt = s.eltRilettura;
  const dettaglio = { target: ma, elt: d.body, shouldSwap: true, serverResponse: html, isError: false, requestConfig: config };
  const prima = new w.CustomEvent("htmx:beforeSwap", { bubbles: true, cancelable: true, detail: dettaglio });
  d.body.dispatchEvent(prima);
  if (prima.defaultPrevented || dettaglio.shouldSwap === false) {
    voce.esito = "fermata";
    return;
  }
  const doc = new w.DOMParser().parseFromString(html, "text/html");
  ma.replaceChildren(...[...doc.body.childNodes].map((n) => d.importNode(n, true)));
  s.spie.swap++;
  d.body.dispatchEvent(new w.CustomEvent("htmx:afterSwap", { bubbles: true, detail: { target: ma, elt: d.body, requestConfig: config } }));
  await attendi(5);
  d.body.dispatchEvent(new w.CustomEvent("htmx:afterSettle", { bubbles: true, detail: { target: ma, elt: d.body, requestConfig: config } }));
  for (const el of [...ma.children]) el.dispatchEvent(new w.CustomEvent("htmx:load", { bubbles: true, detail: { elt: el } }));
  voce.esito = "fatta";
}

// la larghezza degli elementi che la prova imposta (in jsdom è sempre 0): la prima regola che corrisponde vince
function installaMisure(s) {
  const { w } = s;
  const misura = (el) => {
    for (const r of s.larghezze) if (el && el.matches && el.matches(r.sel)) return r.larghezza;
    return null;
  };
  const cw = Object.getOwnPropertyDescriptor(w.Element.prototype, "clientWidth");
  const ow = Object.getOwnPropertyDescriptor(w.HTMLElement.prototype, "offsetWidth");
  const rett = w.Element.prototype.getBoundingClientRect;
  Object.defineProperty(w.Element.prototype, "clientWidth", {
    configurable: true,
    get() { const l = misura(this); return l != null ? l : cw && cw.get ? cw.get.call(this) : 0; },
  });
  Object.defineProperty(w.HTMLElement.prototype, "offsetWidth", {
    configurable: true,
    get() { const l = misura(this); return l != null ? l : ow && ow.get ? ow.get.call(this) : 0; },
  });
  w.Element.prototype.getBoundingClientRect = function () {
    const l = misura(this);
    if (l == null) return rett.call(this);
    return { x: 0, y: 0, top: 0, left: 0, width: l, height: 300, right: l, bottom: 300, toJSON() { return this; } };
  };
  Object.defineProperty(w, "innerWidth", { configurable: true, get: () => s.finestra });
  w.matchMedia = (q) => {
    let vero = true;
    const min = String(q).match(/min-width:\s*([\d.]+)px/);
    const max = String(q).match(/max-width:\s*([\d.]+)px/);
    if (min && s.finestra < Number(min[1])) vero = false;
    if (max && s.finestra > Number(max[1])) vero = false;
    if (/prefers-reduced-motion:\s*reduce/.test(q)) vero = false;
    return { matches: vero, media: String(q), onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} };
  };
}

// l'orologio: Date.now e performance.now con uno scarto; i setTimeout da 1 s in su aspettano avanza()
function installaOrologio(s) {
  const { w } = s;
  const ora = w.Date.now.bind(w.Date);
  const st = w.setTimeout.bind(w);
  const ct = w.clearTimeout.bind(w);
  const pn = w.performance && w.performance.now ? w.performance.now.bind(w.performance) : null;
  s.orologio = { scarto: 0, coda: [], seq: 0 };
  w.Date.now = () => ora() + s.orologio.scarto;
  if (pn) w.performance.now = () => pn() + s.orologio.scarto;
  w.setTimeout = (fn, ms, ...a) => {
    const n = Number(ms) || 0;
    if (n >= 1000 && typeof fn === "function") {
      const id = 1e7 + ++s.orologio.seq;
      s.orologio.coda.push({ id, quando: s.orologio.scarto + n, fn, a });
      return id;
    }
    return st(fn, ms, ...a);
  };
  w.clearTimeout = (id) => {
    if (id >= 1e7) {
      s.orologio.coda = s.orologio.coda.filter((x) => x.id !== id);
      return;
    }
    ct(id);
  };
  s.avanza = async (ms) => {
    s.orologio.scarto += ms;
    const pronti = s.orologio.coda.filter((x) => x.quando <= s.orologio.scarto).sort((a, b) => a.quando - b.quando);
    s.orologio.coda = s.orologio.coda.filter((x) => !pronti.includes(x));
    for (const x of pronti) {
      try { x.fn(...x.a); } catch (e) { s.errori.push("timer: " + e.message); }
    }
    await attendi(20);
  };
}

function installaDoppi(s, opz) {
  const { w } = s;
  w.fetch = fetchFinto(s);
  const apriXhr = w.XMLHttpRequest.prototype.open;
  w.XMLHttpRequest.prototype.open = function (m, u, ...r) {
    s.spie.xhr.push([String(m), conId(u)]);
    return apriXhr.call(this, m, u, ...r);
  };
  w.XMLHttpRequest.prototype.send = function () { /* nessuna rete nelle prove */ };
  // revisione P3, S-01: le altre strade per mandare qualcosa al server, spiate (nessuna deve partire da motore_a.mjs)
  w.navigator.sendBeacon = (u) => { s.spie.altre.push("sendBeacon " + conId(u)); return true; };
  w.HTMLFormElement.prototype.submit = function () { s.spie.altre.push("form.submit " + conId(this.getAttribute("action") || "")); };
  w.htmx = {
    ajax: (verbo, url, o) => rilettura(s, verbo, url, o),
    process() {},
    trigger() {},
    on() {},
    off() {},
  };
  w.confirm = (t) => {
    s.spie.domande.push(String(t));
    return typeof s.conferma === "function" ? !!s.conferma(String(t)) : s.conferma;
  };
  w.alert = (t) => { s.spie.avvisi.push(String(t)); };
  w.prompt = () => null;
  w.scrollTo = () => {};
  w.scrollBy = () => {};
  if (!w.Element.prototype.scrollTo) w.Element.prototype.scrollTo = function () {};
  if (!w.Element.prototype.scrollBy) w.Element.prototype.scrollBy = function () {};
  w.Element.prototype.scrollIntoView = function (o) { s.spie.scorri.push({ id: this.id || "", el: this, opz: o }); };
  // dialog: jsdom 24 non ha showModal/close; Esc su una finestra modale la chiude, come nel browser
  const Dlg = w.HTMLDialogElement.prototype;
  if (!Dlg.showModal) {
    Dlg.showModal = function () { this.setAttribute("open", ""); this.__modale = true; };
    Dlg.show = function () { this.setAttribute("open", ""); };
    Dlg.close = function (v) {
      if (!this.hasAttribute("open")) return;
      if (v !== undefined) this.returnValue = String(v);
      this.removeAttribute("open");
      this.__modale = false;
      this.dispatchEvent(new w.Event("close"));
    };
  }
  if (!Object.getOwnPropertyDescriptor(Dlg, "open") || !Object.getOwnPropertyDescriptor(Dlg, "open").get) {
    Object.defineProperty(Dlg, "open", {
      configurable: true,
      get() { return this.hasAttribute("open"); },
      set(v) { if (v) this.setAttribute("open", ""); else this.removeAttribute("open"); },
    });
  }
  s.d.addEventListener("keydown", (e) => {
    if (e.key !== "Escape" || e.defaultPrevented) return;
    const aperte = [...s.d.querySelectorAll("dialog[open]")].filter((x) => x.__modale);
    const ultima = aperte[aperte.length - 1];
    if (!ultima) return;
    const ev = new w.Event("cancel", { cancelable: true });
    ultima.dispatchEvent(ev);
    if (!ev.defaultPrevented) ultima.close();
  });
  // le miniature
  s.io = [];
  w.IntersectionObserver = class IntersectionObserver {
    constructor(cb, o) { this.cb = cb; this.opzioni = o || {}; this.el = new Set(); s.io.push(this); }
    observe(el) { this.el.add(el); }
    unobserve(el) { this.el.delete(el); }
    disconnect() { this.el.clear(); }
    takeRecords() { return []; }
  };
  delete w.ResizeObserver;
  let blob = 0;
  w.URL.createObjectURL = (b) => {
    const u = "blob:" + ORIGINE + "/mini-" + ++blob;
    s.spie.blob.push({ url: u, tipo: b && b.type });
    return u;
  };
  w.URL.revokeObjectURL = (u) => { s.spie.revocati.push(String(u)); };
  w.HTMLCanvasElement.prototype.toBlob = function (cb, tipo) {
    const b = new w.Blob(["png finto ACME"], { type: tipo || "image/png" });
    Promise.resolve().then(() => cb(b));
  };
  w.HTMLCanvasElement.prototype.toDataURL = function () { return "data:image/png;base64,"; };
  w.HTMLCanvasElement.prototype.getContext = function () {
    const tela = this;
    return new Proxy({}, { get: (t, k) => (k === "canvas" ? tela : () => {}), set: () => true });
  };
  installaMisure(s);
  installaOrologio(s);
  // pdf.js finto e la sua import()
  s.pdf = creaPdfjsFinto({ modo: opz.pdfModo || "auto", perIndirizzo: (u) => (/2pagine/.test(String(s.visore || "")) ? PDF2 : PDF1) });
  w.__importa = (src) => {
    s.spie.importa.push(String(src));
    return Promise.resolve(s.pdf.lib);
  };
  // il ponte finto
  w.__copione = Object.assign(
    {
      rotte: ROTTE,
      letture_ammesse: LETTURE,
      gesti: [],
      letture: [],
      albero: null,
      predefinito: { esito: "no", testo: "Niente è cambiato: (prova) nessun esito preparato per questo gesto." },
    },
    opz.copione || {}
  );
  w.__gesti = [];
}

/**
 * apri(voce, opz) → la sessione di una pagina: {voce, w, d, tid, spie, pdf, orologio, …}
 * opz: copione (window.__copione in più dei predefiniti), conferma (risposta di confirm, true), finestra (larghezza
 * della finestra, 1440), larghezze ([{sel, larghezza}]), pdfModo ("auto" | "manuale"), server(voce della fetch) → una
 * risposta di SERVER, prima(s) (prima dei moduli: per esempio miniature in più).
 */
async function apri(voce, opz = {}) {
  const html = fs.readFileSync(path.join(OUT, voce.pagina), "utf8");
  const tid = threadDi(voce, html);
  const q = new URLSearchParams();
  if (voce.prodotto) q.set("prodotto", voce.prodotto);
  q.set("vista", voce.vista);
  const errori = [];
  const vc = new VirtualConsole();
  vc.on("jsdomError", (e) => {
    const m = String((e && e.message) || e);
    if (/^Not implemented/.test(m)) note.add(m);
    else errori.push(m + (e && e.detail && e.detail.stack ? " | " + String(e.detail.stack).split("\n").slice(0, 3).join(" | ") : ""));
  });
  const dom = new JSDOM(html, { url: `${ORIGINE}/thread/${tid}/motore-a?${q}`, runScripts: "outside-only", pretendToBeVisual: true, virtualConsole: vc });
  const s = {
    voce,
    tid,
    dom,
    w: dom.window,
    d: dom.window.document,
    errori,
    conferma: opz.conferma === undefined ? true : opz.conferma,
    finestra: opz.finestra || 1440,
    larghezze: [...(opz.larghezze || [])],
    server: opz.server || null,
    spie: { fetch: [], xhr: [], ajax: [], altre: [], domande: [], avvisi: [], scorri: [], importa: [], blob: [], revocati: [], swap: 0 },
  };
  installaDoppi(s, opz);
  if (typeof opz.prima === "function") opz.prima(s);
  try {
    s.w.eval(PONTE_FINTO);
    s.w.eval(MOTORE);
  } catch (e) {
    errori.push("il modulo non parte: " + (e && e.message));
  }
  await attendi(40);
  s.gesti = () => s.w.__gesti || [];
  s.scritture = () => s.gesti().filter((g) => g.metodo === "POST" && ["ok", "no", "errore", "login"].includes(g.esito));
  s.letture = () => s.gesti().filter((g) => g.metodo === "GET" && g.esito !== "rilettura");
  s.rilettureDelModulo = () => s.gesti().filter((g) => g.esito === "rilettura" && g.da === "modulo");
  s.rilettureDelPonte = () => s.gesti().filter((g) => g.esito === "rilettura" && g.da === "ponte");
  s.rileggi = async () => {
    s.daProva = true;
    try { await s.w.htmx.ajax("GET", s.w.location.href, { target: "#ma", swap: "innerHTML" }); } finally { s.daProva = false; }
    await attendi(40);
  };
  // la rilettura di una navigazione: requestConfig.elt è il collegamento hx-get che l'ha chiesta (come in htmx 2.0.4)
  s.rileggiNavigando = async (a) => {
    s.eltRilettura = a;
    try { await s.rileggi(); } finally { s.eltRilettura = null; }
  };
  s.memoria = (nome) => {
    try { return s.w.sessionStorage.getItem(`ma.${s.tid}.${nome}`); } catch (e) { return null; }
  };
  s.prodotto = () => {
    const r = s.d.querySelector("#ma > .ma");
    return r ? r.getAttribute("data-ma-prodotto") || "" : "";
  };
  return s;
}

// fa «vedere» gli elementi osservati (tutti, o quelli dentro o attorno agli elementi dati)
function vedi(s, elementi) {
  for (const o of s.io) {
    const voci = [...o.el]
      .filter((e) => e.isConnected && (!elementi || elementi.some((x) => x === e || e.contains(x) || x.contains(e))))
      .map((e) => ({ target: e, isIntersecting: true, intersectionRatio: 1, time: 0, boundingClientRect: {}, intersectionRect: {}, rootBounds: null }));
    if (voci.length) o.cb(voci, o);
  }
}

// i controlli che valgono per ogni pagina aperta, poi la chiusura
function chiudi(s, { controlli = true } = {}) {
  if (controlli) {
    const n = s.voce.pagina;
    const fuori = s.spie.fetch.filter((f) => !(f.metodo === "GET" && f.allegato && f.v && f.soloV && !f.corpo));
    ok(fuori.length === 0, `${n}: motore_a.mjs fa solo GET /allegato/<uuid>/anteprima?v=<sha> (nessuna fetch verso una POST)`, fuori.map((f) => f.metodo + " " + conId(f.url)).join(", "));
    const hx = s.spie.fetch.filter((f) => Object.keys(f.intestazioni).some((k) => k.startsWith("hx-")));
    ok(hx.length === 0, `${n}: la GET del PDF senza intestazioni HX-* (con HX-Request il server risponde 400)`);
    ok(s.spie.xhr.length === 0, `${n}: nessun XMLHttpRequest`);
    // revisione P3, S-01: htmx.ajax solo per rileggere (GET), mai sendBeacon o form.submit()
    const ajaxNonGet = s.spie.ajax.filter((a) => a.verbo !== "GET");
    ok(ajaxNonGet.length === 0, `${n}: ogni htmx.ajax è una GET (rilettura), mai una scrittura (S-01)`, ajaxNonGet.map((a) => a.verbo + " " + conId(a.url)).join(", "));
    ok(s.spie.altre.length === 0, `${n}: nessun sendBeacon e nessun form.submit() (S-01)`, s.spie.altre.join(", "));
    // revisione P3, S-02: nessun gestore scritto in un attributo on… (né dal template né da el())
    const conOn = [...s.d.querySelectorAll("*")].filter((e) => [...e.attributes].some((a) => /^on/i.test(a.name)));
    ok(conOn.length === 0, `${n}: nessun attributo on… nella pagina (S-02)`, conOn.slice(0, 3).map((e) => e.tagName + " " + [...e.attributes].filter((a) => /^on/i.test(a.name)).map((a) => a.name).join(" ")).join(", "));
    // revisione P3, S-03: in sessionStorage solo le chiavi del modulo, con id, chiavi e cammini (mai testi della pagina)
    const memorie = [];
    try {
      const ss = s.w.sessionStorage;
      for (let i = 0; i < ss.length; i++) memorie.push([ss.key(i), ss.getItem(ss.key(i))]);
    } catch (e) { /* senza sessionStorage non c'è niente da controllare */ }
    const reChiave = new RegExp("^ma\\." + s.tid.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "\\.(foglio|(chiusi|scheda|nodo|disegno|tela|aperti)\\.[\\w.:|>#-]*)$");
    const strane = memorie.filter(([k, v]) => !reChiave.test(k) || String(v).length > 2048 || !/^[\w.:|>#\-\[\]",{}]*$/.test(String(v)));
    ok(strane.length === 0, `${n}: sessionStorage con le sole chiavi ma.<thread>.* e valori corti di soli id e cammini (S-03)`, strane.slice(0, 3).map(([k, v]) => k + " = " + String(v).slice(0, 60)).join(" || "));
    ok(s.errori.length === 0, `${n}: nessuna eccezione nella pagina`, s.errori.slice(0, 3).join(" || "));
    ok(s.pdf.stato.lavoratori.length <= 1, `${n}: al più un PDFWorker per la pagina (§7.4)`, String(s.pdf.stato.lavoratori.length));
    const iframe = s.d.querySelectorAll("iframe");
    ok(iframe.length === 0, `${n}: nessun <iframe> (il PDF ha Content-Security-Policy: sandbox)`);
  }
  try { s.w.close(); } catch (e) { /* niente */ }
}

// la pagina senza script, solo da leggere
function leggi(voce, fn) {
  const html = fs.readFileSync(path.join(OUT, voce.pagina), "utf8");
  const dom = new JSDOM(html);
  try {
    return fn(dom.window.document, html);
  } finally {
    dom.window.close();
  }
}
// la prima pagina dell'elenco per cui fn(d, voce) dà qualcosa: {voce, r}
function cerca(elenco, fn) {
  for (const v of elenco) {
    const r = leggi(v, (d) => fn(d, v));
    if (r) return { voce: v, r };
  }
  return null;
}
const pagine = (f = {}) =>
  CASI.filter(
    (v) =>
      (!f.caso || v.caso === f.caso || v.caso.startsWith(f.caso + "_")) &&
      (!f.vista || v.vista === f.vista) &&
      (f.scrive === undefined || v.scrive === f.scrive)
  );

// ------------------------------------------------------------------ aiuti sul DOM

const spento = (b) => !!(b && (b.disabled || b.closest("fieldset[disabled]") || b.getAttribute("aria-disabled") === "true"));
function gestoSpento(g) {
  if (g.tagName !== "FORM") return spento(g);
  const invii = [...g.querySelectorAll("button:not([type=button]), input[type=submit]")];
  return invii.length > 0 && invii.every(spento);
}
function campiStatici(g) {
  const out = [];
  if (g.tagName === "FORM") for (const i of g.querySelectorAll("input[type=hidden][name]")) out.push([i.name, i.value]);
  const fissi = g.getAttribute("data-ma-campi");
  if (fissi) {
    try {
      for (const [k, v] of Object.entries(JSON.parse(fissi))) [].concat(v).forEach((x) => out.push([k, String(x)]));
    } catch (e) {
      out.push(["?", "data-ma-campi non JSON"]);
    }
  }
  return out;
}
const finestraAperta = (d) => {
  const f = [...d.querySelectorAll(".ma-finestra")].filter((x) => x.isConnected && (x.tagName === "DIALOG" ? x.hasAttribute("open") : !x.hidden && !x.closest("[hidden]")));
  return f[f.length - 1] || null;
};
const pannelloDi = (d, k) => [...d.querySelectorAll("#ma-anteprima [data-ma-pannello]")].find((p) => p.getAttribute("data-ma-pannello") === k) || null;
const caselleDi = (d, k) => [...d.querySelectorAll("#ma li[role=treeitem][data-ma-chiave]")].filter((li) => li.getAttribute("data-ma-chiave") === k);
const visibile = (el) => !!el && !el.hidden && !el.closest("[hidden]");
function casellaDi(li) {
  return li.querySelector(":scope > .ma-nodo-box .ma-nodo") || li.querySelector(":scope > .ma-nodo-box") || li.querySelector(":scope > .ma-nodo") || li;
}
async function clicca(s, el, ms = 30) {
  if (!el) return;
  if (typeof el.click === "function") el.click();
  else el.dispatchEvent(new s.w.MouseEvent("click", { bubbles: true, cancelable: true }));
  await attendi(ms);
}
async function tasto(s, el, key, ms = 20) {
  (el || s.d.body).dispatchEvent(new s.w.KeyboardEvent("keydown", { key, bubbles: true, cancelable: true }));
  await attendi(ms);
}
async function scegliValore(s, sel, valore) {
  sel.value = valore;
  sel.dispatchEvent(new s.w.Event("input", { bubbles: true }));
  sel.dispatchEvent(new s.w.Event("change", { bubbles: true }));
  await attendi(15);
}
// sceglie un'opzione come la sceglie una persona (per posizione, non per valore): «— diventa radice» ha il valore
// vuoto come il segnaposto spento, e sel.value = "" sceglierebbe il segnaposto (RAC-16; integrazione di A1d-Q3)
async function scegliOpzione(s, sel, o) {
  sel.selectedIndex = [...sel.options].indexOf(o);
  sel.dispatchEvent(new s.w.Event("input", { bubbles: true }));
  sel.dispatchEvent(new s.w.Event("change", { bubbles: true }));
  await attendi(15);
}
async function scrivi(s, inp, valore) {
  inp.value = valore;
  inp.dispatchEvent(new s.w.Event("input", { bubbles: true }));
  inp.dispatchEvent(new s.w.Event("change", { bubbles: true }));
  await attendi(5);
}
function inviatore(form) {
  return form.querySelector("button[type=submit], button:not([type]), input[type=submit]");
}
// il pulsante d'invio di una finestra del JS: il testo dato, e non «Chiudi», «Annulla» o «senza»
function bottoneDi(f, re) {
  const b = [...f.querySelectorAll("button")].filter((x) => re.test(testo(x)) && !/chiudi|annulla|senza/i.test(testo(x)));
  return b[b.length - 1] || null;
}
function casellaSpunta(f, nome, re) {
  return (
    f.querySelector(`input[type=checkbox][name=${nome}]`) ||
    [...f.querySelectorAll("input[type=checkbox]")].find(
      (x) => !/^(delega|raggruppamento|sorgente)$/.test(x.name) && re.test(testo(x.closest("label") || x.parentElement))
    ) ||
    null
  );
}
function opzioniConValore(sel) {
  return [...sel.options].filter((o) => o.value !== "" && !o.disabled);
}
const serializza = (s, el) => new s.w.XMLSerializer().serializeToString(el);
// apre la casella (clic) e aspetta l'anteprima
async function scegliCasella(s, k, ms = 60) {
  const li = caselleDi(s.d, k)[0];
  if (!li) return null;
  await clicca(s, casellaDi(li), ms);
  return li;
}

// ------------------------------------------------------------------ dati per le finestre (forme del server)

const RIEPILOGO = (confermabile) => ({
  formato: 1,
  albero_firma: "acme-albero-0001",
  firma: "acme-firma-albero-0001",
  nuovi: [{ nodo: "#20", fonte: "step", tipo: "sciolto", codice: "7120110", padri: ["7120100"], lettura: { base: "7120110", rev: "A" } }],
  ritrovati: [],
  tipi: [],
  revisioni: [],
  legami_nuovi: [{ padre: "7120100", figlio: "7120110", qta: 2 }],
  legami_tolti: [],
  quantita: [],
  cascata: [],
  fuori: [],
  scartate: [],
  rimozioni: [],
  commerciali: [],
  vicini: [],
  quantita_discordi: [],
  senza_codice: [],
  blocchi: confermabile ? [] : ["Due legami hanno quantità diverse nel file: si decide prima nel Fascicolo (prova ACME)."],
  confermabile,
});

const ANTEPRIMA_STEP = (extra = {}) =>
  Object.assign(
    {
      autorizzabile: true,
      inviabile: true,
      frasi: ["7120100_C.stp diventa lo STEP strutturale di 7120100 (prova ACME).", "I figli diretti del file diventano la BOM di lavoro."],
      figli: [{ chiave: "#20", nome: "7120110", codice: "7120110", qta: 2, figli: 0, deciso: "", scartato: false, stato: "aperta" }],
      bottone: "Autorizza",
      firma: "acme-firma-step-0001",
      presa_d_atto: "Le deleghe spente restano spente (prova ACME).",
      sorgenti: [],
      raggruppamenti: [],
      deleghe: [
        { chiave: "d1", etichetta: "7120110 (delega libera)", revoca: [] },
        { chiave: "d2", etichetta: "7120120 (delega spenta)", spenta: "è già delegata a un altro STEP (prova ACME)", revoca: ["7120120"] },
      ],
    },
    extra
  );

const ANTEPRIMA_TIPO = { frasi: ["Il tipo cambia: nessun altro effetto (prova ACME)."], bottone: "Cambia il tipo", firma: "acme-firma-tipo-0001" };

// ------------------------------------------------------------------ i gruppi (piano del frontend §9.2: 21 gruppi)

const GRUPPI = [];
const gruppoDi = (nome, fn) => GRUPPI.push([nome, fn]);

// 1. apertura: nessuna scrittura, nessuna rete, nessuna finestra; ogni gesto nella lista chiusa del ponte
gruppoDi("apertura", async () => {
  ok(!!ROTTE && !!LETTURE, "le liste chiuse si leggono dal ponte vero (// ROTTE:inizio … // ROTTE:fine e LETTURE, RAC-13)", LISTE.errore);
  let n = 0;
  for (const v of CASI) {
    const s = await apri(v);
    n++;
    const { d } = s;
    const nome = v.pagina;
    ok(s.gesti().length === 0, `${nome}: all'apertura nessuna chiamata al ponte (né POST né letture)`, s.gesti().map((g) => g.metodo + " " + conId(g.url)).join(", "));
    ok(s.spie.fetch.length === 0, `${nome}: all'apertura nessuna fetch (le miniature partono solo quando si vedono; nessuna casella scelta da sola)`, s.spie.fetch.map((f) => conId(f.url)).join(", "));
    ok(s.spie.ajax.length === 0 && s.spie.xhr.length === 0, `${nome}: all'apertura nessuna htmx.ajax e nessun XHR`);
    ok(!finestraAperta(d), `${nome}: nessuna finestra aperta all'apertura`);
    const avvisi = d.querySelectorAll("#ma-avviso");
    ok(avvisi.length === 1 && !avvisi[0].closest("#ma"), `${nome}: un solo #ma-avviso, fuori da #ma`);
    const radice = d.querySelectorAll("#ma > .ma[data-ma-thread][data-ma-prodotto][data-ma-vista]");
    ok(radice.length === 1 && d.querySelectorAll("#ma > *").length === 1, `${nome}: in #ma un solo elemento, la .ma con data-ma-thread, -prodotto, -vista (RAC-23)`);
    if (radice[0]) ok(radice[0].getAttribute("data-ma-vista") === v.vista, `${nome}: data-ma-vista = ${v.vista}`);
    const ma = d.getElementById("ma");
    ok(!!ma && ma.hasAttribute("data-ma-pdfjs") && ma.hasAttribute("data-ma-pdfjs-worker"), `${nome}: #ma con data-ma-pdfjs e data-ma-pdfjs-worker (C-01)`);
    const hx = d.querySelectorAll("#ma [hx-post], #ma [hx-put], #ma [hx-patch], #ma [hx-delete], #ma [hx-trigger], #ma-avviso[hx-trigger]");
    ok(hx.length === 0, `${nome}: in #ma nessun hx-post, hx-put, hx-patch, hx-delete o hx-trigger`);
    const fuori = [];
    const tecnici = [];
    for (const g of d.querySelectorAll("#ma [data-ma-gesto]")) {
      const r = g.getAttribute("data-ma-gesto");
      if (!nellaLista(r, s.tid)) fuori.push(conId(r));
      if (/nodo:|componente:/.test(r) || campiStatici(g).some(([, x]) => /nodo:|componente:/.test(x))) tecnici.push(conId(r));
    }
    ok(fuori.length === 0, `${nome}: ogni data-ma-gesto è nella lista chiusa del ponte, con l'id della pagina (§5.2)`, uniche(fuori).join(", "));
    ok(tecnici.length === 0, `${nome}: nessuna rotta o campo fisso con «nodo:» o «componente:» (contratto §1.9)`, uniche(tecnici).join(", "));
    if (esitoNil(v.caso) === true) ok(!d.querySelector("#ma .ma-ws"), `${nome}: senza Esito nessun workspace (e8)`);
    chiudi(s);
  }
  ok(n === CASI.length && n > 0, `aperte ${n} pagine su ${CASI.length}`);
});

// 2. workspace: uno solo con distinta e documenti, nessuno con rfq; le stesse regioni R3–R9
const REGIONI = ["#ma-fonte", "#ma-albero", "#ma-schede-albero", "#ma-conflitti-struttura", "#ma-anteprima", "#ma-da-smistare", "#ma-documenti", "#ma-documenti-t"];
gruppoDi("workspace", async () => {
  const ids = new Map();
  for (const v of CASI) {
    leggi(v, (d) => {
      const ws = d.querySelectorAll("#ma .ma-ws");
      if (v.vista === "rfq") {
        ok(ws.length === 0, `${v.pagina}: con vista=rfq nessun .ma-ws (c'è il riepilogo)`);
        return;
      }
      if (!v.prodotto || esitoNil(v.caso) === true) return;
      ok(ws.length === 1 && ws[0].hasAttribute("data-ma-ws"), `${v.pagina}: un solo .ma-ws[data-ma-ws] (un workspace, FE-01)`, String(ws.length));
      if (!ws[0]) return;
      const manca = REGIONI.filter((r) => !ws[0].querySelector(r));
      ok(manca.length === 0, `${v.pagina}: nel workspace le regioni R3–R9`, manca.join(", "));
      const albero = d.querySelector("#ma-albero");
      const schede = d.querySelector("#ma-schede-albero");
      if (albero && schede) ok(!!(albero.compareDocumentPosition(schede) & 4), `${v.pagina}: la conferma dell'albero (R4) sta sopra le schede (R5, FE-09)`);
      ids.set(v.pagina, [...ws[0].querySelectorAll("[id]")].map((x) => x.id).join(" "));
    });
  }
  // distinta e documenti dello stesso caso, prodotto e Scrive: gli stessi id, nello stesso ordine
  let coppie = 0;
  for (const v of CASI.filter((x) => x.vista === "distinta" && ids.has(x.pagina))) {
    const altra = CASI.find((x) => x.vista === "documenti" && x.caso === v.caso && x.prodotto === v.prodotto && x.scrive === v.scrive && ids.has(x.pagina));
    if (!altra) continue;
    coppie++;
    ok(ids.get(v.pagina) === ids.get(altra.pagina), `${v.pagina} e ${altra.pagina}: lo stesso workspace (stessi id, stesso ordine)`);
  }
  ok(coppie > 0, `confrontate ${coppie} coppie distinta/documenti`);
});

// 3. documenti: evidenza e fuoco solo con vista=documenti, anche dopo una rilettura
gruppoDi("documenti", async () => {
  let nd = 0;
  for (const v of CASI.filter((x) => x.vista !== "rfq" && x.prodotto && esitoNil(x.caso) !== true)) {
    const s = await apri(v);
    const { d } = s;
    const sez = d.getElementById("ma-documenti");
    if (!ok(!!sez, `${v.pagina}: c'è #ma-documenti`)) { chiudi(s); continue; }
    const scorsi = () => s.spie.scorri.filter((x) => x.id === "ma-documenti" || x.id === "ma-documenti-t").length;
    if (v.vista === "documenti") {
      nd++;
      ok(sez.classList.contains("ma-evidenza"), `${v.pagina}: #ma-documenti.ma-evidenza`);
      const t = d.getElementById("ma-documenti-t");
      ok(!!t && t.getAttribute("tabindex") === "-1", `${v.pagina}: #ma-documenti-t con tabindex=-1`);
      ok(d.activeElement === t, `${v.pagina}: il fuoco su #ma-documenti-t all'apertura`);
      const prima = scorsi();
      ok(prima > 0, `${v.pagina}: scrollIntoView sul riepilogo documentale all'apertura`);
      if (v.frammento) {
        // D-FEA-03 (aperto): la rilettura dopo un gesto rimette il fuoco dov'era e non porta via lo scorrimento; la
        // navigazione (un collegamento hx-get, requestConfig.elt) evidenzia di nuovo (integrazione di A1d-Q3)
        await s.rileggi();
        const t2 = d.getElementById("ma-documenti-t");
        ok(!!t2 && t2 !== t && d.activeElement === t2, `${v.pagina}: dopo la rilettura il fuoco torna su #ma-documenti-t (quello nuovo)`);
        ok(scorsi() === prima, `${v.pagina}: dopo la rilettura di un gesto nessuno scrollIntoView nuovo (D-FEA-03)`);
        ok(d.getElementById("ma-documenti").classList.contains("ma-evidenza"), `${v.pagina}: dopo la rilettura ancora .ma-evidenza`);
        const vista = d.querySelector('#ma-viste a[aria-current="page"][hx-get]');
        if (ok(!!vista, `${v.pagina}: la voce corrente delle viste è un collegamento hx-get`)) {
          await s.rileggiNavigando(vista);
          const t3 = d.getElementById("ma-documenti-t");
          ok(!!t3 && t3 !== t2 && d.activeElement === t3, `${v.pagina}: dopo una navigazione il fuoco su #ma-documenti-t (quello nuovo)`);
          ok(scorsi() > prima, `${v.pagina}: dopo una navigazione di nuovo scrollIntoView`);
          ok(d.getElementById("ma-documenti").classList.contains("ma-evidenza"), `${v.pagina}: dopo una navigazione ancora .ma-evidenza`);
        }
      }
    } else {
      ok(!sez.classList.contains("ma-evidenza"), `${v.pagina}: con distinta nessuna .ma-evidenza (MF09)`);
      ok(!d.activeElement || d.activeElement.id !== "ma-documenti-t", `${v.pagina}: con distinta il fuoco non va sul riepilogo documentale`);
      ok(scorsi() === 0, `${v.pagina}: con distinta nessuno scrollIntoView sul riepilogo documentale`);
    }
    chiudi(s);
  }
  ok(nd > 0, `pagine con vista=documenti provate: ${nd}`);
});

// 4. candidate: nessuna struttura candidata segnata come scelta; niente trascinamento nelle schede cand
gruppoDi("candidate", async () => {
  const elenco = ["e1", "e2", "e18"].flatMap((c) => pagine({ caso: c, vista: "distinta" }));
  ok(elenco.length > 0, "ci sono le pagine di e1, e2, e18", String(elenco.length));
  let candidate = 0;
  for (const v of elenco) {
    const s = await apri(v);
    const { d } = s;
    const n = v.pagina;
    ok(d.querySelectorAll("#ma li[role=treeitem][aria-selected=true]").length === 0, `${n}: all'apertura nessuna casella scelta (D-PF-06)`);
    ok([...d.querySelectorAll("#ma-anteprima [data-ma-pannello]")].every((p) => !visibile(p)), `${n}: all'apertura nessun pannello dell'anteprima aperto (lo stato vuoto)`);
    const segni = d.querySelectorAll("#ma-fonte input:checked, #ma-schede-albero input:checked, #ma-fonte [aria-checked=true], #ma-schede-albero [aria-checked=true]");
    ok(segni.length === 0, `${n}: in R3 e R5 nessun input spuntato e nessun aria-checked (MF12)`);
    ok(/nessuna è scelta/i.test(testo(d.getElementById("ma-fonte"))), `${n}: R3 dice «nessuna è scelta»`);
    for (const p of d.querySelectorAll("#ma [role=tabpanel][data-ma-gruppo^='cand:']")) {
      candidate++;
      const g = p.getAttribute("data-ma-gruppo");
      ok(FRASE_CAND.test(testo(p)), `${n} ${g}: «proposta dal file, non è la BOM»`);
      ok(!p.querySelector("[draggable=true], [data-ma-sposta], [data-ma-drop]"), `${n} ${g}: niente trascinamento in una scheda candidata (D-PF-05)`);
    }
    chiudi(s);
  }
  ok(candidate > 0, `schede candidate viste: ${candidate}`);
  // e1: Autorizza spento (manca il documento: P-14, D-RAC-02); e2: una candidata autorizzabile e una no
  for (const v of pagine({ caso: "e1", vista: "distinta", scrive: true })) {
    leggi(v, (d) => ok([...d.querySelectorAll("#ma-fonte [data-ma-azione=step]")].every(spento), `${v.pagina}: «Autorizza lo STEP strutturale…» spento (e1)`));
  }
  for (const v of pagine({ caso: "e2", vista: "distinta", scrive: true })) {
    leggi(v, (d) => {
      const b = [...d.querySelectorAll("#ma-fonte [data-ma-azione=step]")];
      ok(b.some((x) => !spento(x)), `${v.pagina}: una candidata con «Autorizza…» acceso (e2, A)`);
      ok(/autorizz/i.test(testo(d.getElementById("ma-fonte"))) && d.querySelectorAll("#ma-fonte .ma-letto, #ma-fonte li").length >= 2, `${v.pagina}: una riga per struttura candidata (e2)`);
    });
  }
});

// 5. schede: ← → in giro, Home, End; aria-selected, tabindex, hidden; ma.<t>.scheda.<prodotto>; dopo la rilettura
gruppoDi("schede", async () => {
  const trovata =
    cerca(pagine({ vista: "distinta" }), (d) => (d.querySelectorAll("#ma-schede [role=tab]").length >= 3 ? true : null)) ||
    cerca(pagine({ vista: "distinta" }), (d) => (d.querySelectorAll("#ma-schede [role=tab]").length >= 2 ? true : null));
  if (!ok(!!trovata, "una pagina con almeno due schede dell'albero (e2: A, B, senza)")) return;
  const v = trovata.voce;
  const s = await apri(v);
  const { d } = s;
  const lista = d.getElementById("ma-schede");
  const tab = () => [...d.querySelectorAll("#ma-schede > [role=tab]")];
  const corrente = () => tab().filter((t) => t.getAttribute("aria-selected") === "true");
  const controlla = (atteso, cosa) => {
    const t = tab();
    const sel = corrente();
    ok(sel.length === 1 && sel[0] === atteso, `${v.pagina}: ${cosa}: una sola scheda aria-selected, quella giusta`);
    ok(t.every((x) => x.getAttribute("tabindex") === (x === atteso ? "0" : "-1")), `${v.pagina}: ${cosa}: tabindex a rotazione (0 solo sulla scheda scelta)`);
    ok(d.activeElement === atteso, `${v.pagina}: ${cosa}: il fuoco sulla scheda`);
    for (const x of t) {
      const p = d.getElementById(x.getAttribute("aria-controls") || "");
      ok(!!p && p.getAttribute("role") === "tabpanel" && p.hidden === (x !== atteso), `${v.pagina}: ${cosa}: il pannello di «${testo(x)}» ${x === atteso ? "visibile" : "nascosto"}`);
    }
    ok(s.memoria("scheda." + s.prodotto()) === atteso.getAttribute("data-ma-gruppo"), `${v.pagina}: ${cosa}: ma.<t>.scheda.<prodotto> = il gruppo della scheda`);
  };
  ok(!!lista && lista.getAttribute("role") === "tablist", `${v.pagina}: #ma-schede[role=tablist]`);
  ok(corrente().length === 1, `${v.pagina}: all'apertura una sola scheda aria-selected`);
  const t = tab();
  ok(t.every((x) => x.hasAttribute("data-ma-gruppo") && x.hasAttribute("aria-controls")), `${v.pagina}: ogni scheda con data-ma-gruppo e aria-controls`);
  corrente()[0].focus();
  const i0 = t.indexOf(corrente()[0]);
  await tasto(s, d.activeElement, "ArrowRight");
  controlla(t[(i0 + 1) % t.length], "→");
  await tasto(s, d.activeElement, "Home");
  controlla(t[0], "Home");
  await tasto(s, d.activeElement, "ArrowLeft");
  controlla(t[t.length - 1], "← dalla prima (in giro, MF19)");
  await tasto(s, d.activeElement, "ArrowRight");
  controlla(t[0], "→ dall'ultima (in giro)");
  await tasto(s, d.activeElement, "End");
  controlla(t[t.length - 1], "End");
  // dopo la rilettura la scheda scelta torna (ripristinaScheda)
  const gruppo = corrente()[0].getAttribute("data-ma-gruppo");
  if (v.frammento) {
    await s.rileggi();
    const dopo = [...d.querySelectorAll("#ma-schede > [role=tab][aria-selected=true]")];
    ok(dopo.length === 1 && dopo[0].getAttribute("data-ma-gruppo") === gruppo, `${v.pagina}: dopo la rilettura la stessa scheda scelta`);
    const p = dopo[0] && d.getElementById(dopo[0].getAttribute("aria-controls") || "");
    ok(!!p && !p.hidden, `${v.pagina}: dopo la rilettura il suo pannello è visibile`);
  }
  chiudi(s);
});

// 6. chiavi gruppo|rif: e18 (stesso rif in due candidate) e due comparse della stessa chiave nello stesso gruppo
gruppoDi("chiavi", async () => {
  // controlli statici su ogni pagina con l'albero
  for (const v of pagine({ vista: "distinta" })) {
    leggi(v, (d) => {
      const pannelli = [...d.querySelectorAll("#ma-anteprima [data-ma-pannello]")].map((p) => p.getAttribute("data-ma-pannello"));
      ok(pannelli.length === uniche(pannelli).length, `${v.pagina}: data-ma-pannello unici`);
      ok(pannelli.every((k) => k.includes("|")), `${v.pagina}: ogni data-ma-pannello è «<gruppo>|<rif>» (MF03)`);
      const male = [...d.querySelectorAll("#ma li[role=treeitem][data-ma-chiave]")].filter((li) => {
        const p = li.closest("[role=tabpanel][data-ma-gruppo]");
        return !p || !li.getAttribute("data-ma-chiave").startsWith(p.getAttribute("data-ma-gruppo") + "|");
      });
      ok(male.length === 0, `${v.pagina}: ogni data-ma-chiave comincia con il gruppo del suo pannello`, String(male.length));
      const cammini = [...d.querySelectorAll("#ma li[role=treeitem]")].map((li) => li.getAttribute("data-ma-cammino"));
      ok(cammini.every(Boolean) && cammini.length === uniche(cammini).length, `${v.pagina}: data-ma-cammino presenti e unici`);
    });
  }
  // e18: lo stesso rif in due gruppi
  const doppio = cerca(pagine({ caso: "e18", vista: "distinta" }).concat(pagine({ vista: "distinta" })), (d) => {
    const perRif = new Map();
    for (const li of d.querySelectorAll("#ma li[role=treeitem][data-ma-chiave][data-ma-rif]")) {
      const p = li.closest("[role=tabpanel][data-ma-gruppo]");
      if (!p) continue;
      const r = li.getAttribute("data-ma-rif");
      if (!perRif.has(r)) perRif.set(r, new Set());
      perRif.get(r).add(p.getAttribute("data-ma-gruppo"));
    }
    for (const [r, g] of perRif) if (g.size >= 2) return { rif: r, gruppi: [...g].slice(0, 2) };
    return null;
  });
  if (ok(!!doppio, "una pagina con lo stesso rif in due gruppi (e18, D6)")) {
    const { voce: v, r } = doppio;
    const k1 = r.gruppi[0] + "|" + r.rif;
    const k2 = r.gruppi[1] + "|" + r.rif;
    const s = await apri(v);
    const { d } = s;
    ok(k1 !== k2 && !!pannelloDi(d, k1) && !!pannelloDi(d, k2), `${v.pagina}: due chiavi diverse e due pannelli per lo stesso rif`);
    for (const [ka, kb] of [[k1, k2], [k2, k1]]) {
      await scegliCasella(s, ka);
      ok(visibile(pannelloDi(d, ka)) && !visibile(pannelloDi(d, kb)), `${v.pagina}: scelta ${conId(ka)}: si vede solo il suo pannello (MF18)`);
      ok(caselleDi(d, ka).every((li) => li.getAttribute("aria-selected") === "true"), `${v.pagina}: scelta ${conId(ka)}: le sue caselle aria-selected`);
      ok(caselleDi(d, kb).every((li) => li.getAttribute("aria-selected") !== "true"), `${v.pagina}: scelta ${conId(ka)}: le caselle dell'altro gruppo no`);
      ok(s.memoria("nodo." + s.prodotto()) === ka, `${v.pagina}: ma.<t>.nodo.<prodotto> = la chiave intera`);
    }
    chiudi(s);
  }
  // la stessa chiave due volte nello stesso gruppo (un figlio sotto due padri, e3): tutte e due scelte
  const due = cerca(pagine({ vista: "distinta" }), (d) => {
    const conta = new Map();
    for (const li of d.querySelectorAll("#ma li[role=treeitem][data-ma-chiave]")) conta.set(li.getAttribute("data-ma-chiave"), (conta.get(li.getAttribute("data-ma-chiave")) || 0) + 1);
    for (const [k, n] of conta) if (n >= 2) return k;
    return null;
  });
  if (ok(!!due, "una pagina con la stessa chiave in due caselle dello stesso gruppo (e3: due padri nel file)")) {
    const s = await apri(due.voce);
    await scegliCasella(s, due.r);
    const tutte = caselleDi(s.d, due.r);
    ok(tutte.length >= 2 && tutte.every((li) => li.getAttribute("aria-selected") === "true"), `${due.voce.pagina}: le comparse della stessa chiave sono tutte aria-selected`);
    ok(visibile(pannelloDi(s.d, due.r)), `${due.voce.pagina}: il pannello della chiave è visibile`);
    chiudi(s);
  }
});

// 7. qta obbligatoria: Collega, Sposta nell'anteprima, dialogo del trascinamento (vuoto e 0: nessuna POST; 2: una)
async function provaQta(s, form, cosa) {
  const q = form.querySelector("input[name=qta]");
  if (!ok(!!q, `${cosa}: c'è il campo qta`)) return;
  ok(q.type === "number" && q.required && q.getAttribute("min") === "1" && q.getAttribute("step") === "1", `${cosa}: qta type=number required min=1 step=1 (MF01)`);
  ok(!q.hasAttribute("value") && q.value === "" && !q.hasAttribute("placeholder"), `${cosa}: qta senza valore e senza segnaposto (MF02)`);
  const b = inviatore(form);
  if (!ok(!!b && !spento(b), `${cosa}: il pulsante d'invio c'è ed è acceso`)) return;
  const n0 = s.scritture().length;
  await scrivi(s, q, "");
  await clicca(s, b);
  ok(s.scritture().length === n0, `${cosa}: qta vuota → nessuna POST`);
  await scrivi(s, q, "0");
  await clicca(s, b);
  ok(s.scritture().length === n0, `${cosa}: qta 0 → nessuna POST`);
  await scrivi(s, q, "2");
  await clicca(s, b, 50);
  const nuove = s.scritture().slice(n0);
  ok(nuove.length === 1 && (nuove[0].campi || []).some(([k, x]) => k === "qta" && x === "2"), `${cosa}: qta 2 → una POST con qta=2`, JSON.stringify(nuove.map((g) => g.campi)));
  return nuove[0];
}
gruppoDi("qta", async () => {
  const conScrive = pagine({ vista: "distinta", scrive: true });
  for (const [rotta, nome] of [["/collega", "Collega"], ["/sposta", "Sposta"]]) {
    const t = cerca(conScrive, (d) => {
      for (const f of d.querySelectorAll("#ma-anteprima form[data-ma-gesto]")) {
        if (!f.getAttribute("data-ma-gesto").endsWith(rotta) || gestoSpento(f)) continue;
        const p = f.closest("[data-ma-pannello]");
        const scelte = [...f.querySelectorAll("select[required]")];
        if (p && scelte.every((x) => opzioniConValore(x).length || [...x.options].some((o, i) => i > 0 && !o.disabled))) return { k: p.getAttribute("data-ma-pannello"), gesto: f.getAttribute("data-ma-gesto") };
      }
      return null;
    });
    if (!ok(!!t, `una pagina con «${nome}» acceso nell'anteprima`)) continue;
    const s = await apri(t.voce);
    await scegliCasella(s, t.r.k);
    const form = [...s.d.querySelectorAll("#ma-anteprima form[data-ma-gesto]")].find((f) => f.getAttribute("data-ma-gesto") === t.r.gesto && f.closest("[data-ma-pannello]").getAttribute("data-ma-pannello") === t.r.k);
    if (ok(!!form, `${t.voce.pagina}: il modulo ${nome} della casella scelta`)) {
      for (const sel of form.querySelectorAll("select[required]")) {
        const o = opzioniConValore(sel)[0] || [...sel.options].find((x, i) => i > 0 && !x.disabled);
        if (o) await scegliOpzione(s, sel, o);
      }
      if (rotta === "/collega") {
        const padre = form.querySelector("select[name=padre]");
        ok(!!padre && padre.required && [...padre.options][0].disabled, `${t.voce.pagina}: Collega: padre obbligatorio con «scegli» spento`);
      }
      await provaQta(s, form, `${t.voce.pagina} ${nome}`);
    }
    chiudi(s);
  }
  // il trascinamento apre il modulo Sposta in una finestra (solo bom e senza, solo con Scrive)
  const tr = cerca(conScrive, (d) => {
    for (const n of d.querySelectorAll("#ma .ma-nodo[data-ma-sposta]")) {
      const t = [...d.querySelectorAll("#ma .ma-nodo[data-ma-drop]")].find((x) => x !== n && x.getAttribute("data-ma-drop") !== (n.getAttribute("data-ma-drop") || "") && x.closest("[role=tabpanel]") === n.closest("[role=tabpanel]"));
      if (t) return { da: n.closest("li[role=treeitem]").getAttribute("data-ma-cammino"), a: t.closest("li[role=treeitem]").getAttribute("data-ma-cammino") };
    }
    return null;
  });
  if (ok(!!tr, "una pagina con una casella da trascinare e una dove lasciarla (e3, e4)")) {
    const s = await apri(tr.voce);
    const { w, d } = s;
    const li = (c) => [...d.querySelectorAll("#ma li[role=treeitem]")].find((x) => x.getAttribute("data-ma-cammino") === c);
    const n = li(tr.r.da).querySelector(".ma-nodo[data-ma-sposta]");
    const t = li(tr.r.a).querySelector(".ma-nodo[data-ma-drop]");
    ok(n.getAttribute("draggable") === "true", `${tr.voce.pagina}: la casella da spostare è draggable`);
    const trascina = (tipo, el) => {
      const ev = new w.Event(tipo, { bubbles: true, cancelable: true });
      Object.defineProperty(ev, "dataTransfer", { value: { effectAllowed: "", dropEffect: "", types: [], setData() {}, getData() { return ""; }, clearData() {} } });
      el.dispatchEvent(ev);
    };
    trascina("dragstart", n);
    trascina("dragover", t);
    trascina("drop", t);
    trascina("dragend", n);
    await attendi(40);
    const f = finestraAperta(d);
    const form = f && [...f.querySelectorAll("form[data-ma-gesto]")].find((x) => x.getAttribute("data-ma-gesto").endsWith("/sposta"));
    if (ok(!!form, `${tr.voce.pagina}: il trascinamento apre la finestra con il modulo Sposta (data-ma-gesto …/sposta)`)) {
      ok(form.getAttribute("data-ma-gesto") === n.getAttribute("data-ma-sposta"), `${tr.voce.pagina}: la rotta è quella della casella trascinata`);
      const da = form.querySelector("[name=da]");
      const a = form.querySelector("[name=a]");
      ok(!!da && da.value === (n.getAttribute("data-ma-da") || ""), `${tr.voce.pagina}: «da» = data-ma-da della casella`);
      ok(!!a && a.value === t.getAttribute("data-ma-drop"), `${tr.voce.pagina}: «a» = data-ma-drop della casella d'arrivo`);
      await provaQta(s, form, `${tr.voce.pagina} trascinamento`);
    }
    chiudi(s);
  }
});

// 8. miniature: solo [data-ma-mini][data-ma-allegato], mai HX-*, al massimo 2 insieme, cache, errori, nuovo tentativo
const chiaveMini = (x) => (x.getAttribute("data-ma-allegato") || "").toLowerCase() + "|" + (x.getAttribute("data-ma-sha") || "");
const spanDi = (s, k) => [...s.d.querySelectorAll("#ma [data-ma-mini]")].filter((x) => chiaveMini(x) === k);
const finita = (s, k) => spanDi(s, k).some((x) => x.querySelector("img") || /non disponibile|scaduta|non è un PDF/i.test(testo(x)));
// almeno `n` chiavi: se nessuna pagina ne ha tante, si copiano le miniature che ci sono con allegati ACME nuovi (fN)
function bastano(n) {
  return (s) => {
    const spans = [...s.d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")];
    const chiavi = uniche(spans.map(chiaveMini));
    for (let i = 0; chiavi.length + i < n && spans[0]; i++) {
      const c = spans[0].cloneNode(true);
      const nn = "f" + (i + 1);
      c.setAttribute("data-ma-allegato", ACME(nn));
      c.setAttribute("data-ma-sha", nn.repeat(32));
      spans[0].parentElement.append(c);
      note.add("miniature: aggiunte copie ACME delle miniature per averne " + n);
    }
  };
}
gruppoDi("miniature", async () => {
  let migliore = null;
  for (const v of pagine({ vista: "distinta" })) {
    const n = leggi(v, (d) => uniche([...d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini)).length);
    if (n && (!migliore || n > migliore.n)) migliore = { v, n };
  }
  if (!ok(!!migliore, "una pagina con miniature [data-ma-mini][data-ma-allegato] (e1, e5, e10, e13)")) return;
  const v = migliore.v;
  // statici: ogni miniatura ha allegato (UUID) e sha
  leggi(v, (d) => {
    const m = [...d.querySelectorAll("#ma [data-ma-mini]")];
    ok(m.every((x) => RE_UUID.test(x.getAttribute("data-ma-allegato") || "") && (x.getAttribute("data-ma-sha") || "").length > 0), `${v.pagina}: ogni [data-ma-mini] con data-ma-allegato (UUID) e data-ma-sha`);
  });
  // A. coda: al massimo 2 documenti insieme, poi tutte; ogni GET giusta; pdf.js dal template; cache dopo la rilettura
  {
    const s = await apri(v, { pdfModo: "manuale", prima: bastano(3) });
    const { d } = s;
    const chiavi = uniche([...d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini));
    const chiesti = new Set();
    let massimo = 0;
    s.alFetch = (f) => {
      if (!f.allegato) return;
      chiesti.add(f.allegato + "|" + f.v);
      massimo = Math.max(massimo, [...chiesti].filter((k) => !finita(s, k)).length);
    };
    ok(s.spie.fetch.length === 0, `${v.pagina}: prima di vederle nessuna miniatura chiesta`);
    vedi(s);
    await attendi(60);
    ok(s.spie.fetch.length >= 1 && s.spie.fetch.length <= 2, `${v.pagina}: con ${chiavi.length} miniature in vista partono al più 2 GET (coda di 2, MF15)`, String(s.spie.fetch.length));
    for (let giro = 0; giro < chiavi.length * 3 + 5 && (s.pdf.fermi() || chiavi.some((k) => !finita(s, k))); giro++) {
      s.pdf.rilascia(1);
      await attendi(40);
    }
    ok(massimo <= 2, `${v.pagina}: mai più di 2 miniature in lavorazione insieme`, String(massimo));
    ok(s.pdf.stato.massimoAperti <= 2, `${v.pagina}: mai più di 2 documenti pdf.js aperti insieme`, String(s.pdf.stato.massimoAperti));
    ok(chiavi.every((k) => finita(s, k)), `${v.pagina}: alla fine ogni miniatura ha la sua immagine`);
    const perChiave = new Map();
    for (const f of s.spie.fetch) perChiave.set(f.allegato + "|" + f.v, (perChiave.get(f.allegato + "|" + f.v) || 0) + 1);
    ok(chiavi.every((k) => perChiave.get(k) === 1) && perChiave.size === chiavi.length, `${v.pagina}: una GET per miniatura, e solo per le [data-ma-mini][data-ma-allegato]`);
    ok(s.spie.fetch.every((f) => f.credenziali === "same-origin" || f.credenziali === undefined), `${v.pagina}: la GET del PDF con credentials same-origin`);
    const img = [...d.querySelectorAll("#ma [data-ma-mini] img")];
    ok(img.length > 0 && img.every((x) => /^blob:/.test(x.getAttribute("src") || "") && x.getAttribute("alt") === ""), `${v.pagina}: le miniature sono <img alt=""> da URL.createObjectURL (T-A1d-FE-03)`);
    const docs = s.pdf.stato.documenti;
    ok(docs.length > 0 && docs.every((x) => x.dati && x.dati.subarray(0, 5).toString("latin1") === "%PDF-"), `${v.pagina}: getDocument riceve i byte della GET (data), non l'indirizzo (D-PF-08)`);
    ok(docs.every((x) => x.params.isEvalSupported === false && x.params.enableXfa === false), `${v.pagina}: getDocument con isEvalSupported e enableXfa falsi`);
    ok(docs.every((x) => x.params.worker && s.pdf.stato.lavoratori.includes(x.params.worker)), `${v.pagina}: getDocument con il PDFWorker condiviso`);
    ok(docs.every((x) => x.distrutto), `${v.pagina}: ogni documento delle miniature si chiude (task.destroy())`);
    ok(s.pdf.stato.disegni.every((x) => x.annotationMode === 0), `${v.pagina}: le miniature senza annotazioni (AnnotationMode.DISABLE)`);
    const ma = d.getElementById("ma");
    ok(s.spie.importa.length >= 1 && s.spie.importa.every((x) => x === ma.getAttribute("data-ma-pdfjs")), `${v.pagina}: pdf.js dall'indirizzo di data-ma-pdfjs ({{statico}})`);
    ok(s.pdf.lib.GlobalWorkerOptions.workerSrc === ma.getAttribute("data-ma-pdfjs-worker"), `${v.pagina}: il worker da data-ma-pdfjs-worker (C-01)`);
    // dopo la rilettura: dalla cache, nessuna GET nuova (MF17)
    if (v.frammento) {
      const nf = s.spie.fetch.length;
      const nd = s.pdf.stato.documenti.length;
      await s.rileggi();
      vedi(s);
      await attendi(60);
      ok(s.spie.fetch.length === nf && s.pdf.stato.documenti.length === nd, `${v.pagina}: dopo la rilettura nessuna GET e nessun getDocument nuovi (cache)`, `${s.spie.fetch.length - nf} GET in più`);
      const tornate = [...d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")];
      ok(tornate.length > 0 && tornate.every((x) => x.querySelector("img")), `${v.pagina}: dopo la rilettura le miniature tornano subito`);
    }
    chiudi(s);
  }
  // B. errori del server: il testo della risposta; nuovo tentativo solo dopo 60 s
  {
    let piano = {};
    const s = await apri(v, { prima: bastano(3), server: (f) => (piano[f.allegato] ? SERVER[piano[f.allegato]]() : SERVER.pdf()) });
    const originali = leggi(v, (d) => uniche([...d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini)));
    const tutte = uniche([...s.d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini));
    const ordine = originali.concat(tutte.filter((k) => !originali.includes(k)));
    const [k1, k2, k3] = ordine;
    const a = (k) => k.split("|")[0];
    piano = { [a(k1)]: "404", [a(k2)]: "409", [a(k3)]: "415" };
    vedi(s);
    await attendi(80);
    for (const [k, cod, frase] of [[k1, 404, /Riscarica/], [k2, 409, /segnalazione aperta/], [k3, 415, /solo per i PDF/]]) {
      const t = spanDi(s, k).map(testo).join(" ");
      ok(/anteprima non disponibile/i.test(t) && frase.test(t), `${v.pagina}: ${cod} → «anteprima non disponibile» con il testo della risposta`, t.slice(0, 120));
      ok(spanDi(s, k).every((x) => !x.querySelector("img")), `${v.pagina}: ${cod} → nessuna immagine inventata`);
    }
    ok(s.pdf.stato.documenti.length === 0, `${v.pagina}: con 404/409/415 nessun getDocument`);
    if (v.frammento && originali.includes(k1)) {
      const chiesteK1 = () => s.spie.fetch.filter((f) => f.allegato + "|" + f.v === k1).length;
      piano = { [a(k1)]: "pdf" };
      await s.avanza(10000);
      await s.rileggi();
      vedi(s);
      await attendi(60);
      ok(chiesteK1() === 1, `${v.pagina}: dopo 10 s (e una rilettura) la miniatura fallita non si richiede`, String(chiesteK1()));
      await s.avanza(51000);
      await s.rileggi();
      vedi(s);
      await attendi(80);
      ok(chiesteK1() === 2, `${v.pagina}: dopo 60 s la miniatura fallita si richiede (nuovo tentativo)`, String(chiesteK1()));
      ok(spanDi(s, k1).some((x) => x.querySelector("img")), `${v.pagina}: al nuovo tentativo riuscito l'immagine c'è`);
    } else note.add("miniature: nuovo tentativo dopo 60 s non provato (manca il frammento o la miniatura originale)");
    chiudi(s);
  }
  // C. sessione scaduta e una risposta che non è un PDF
  {
    let piano = {};
    const s = await apri(v, { prima: bastano(2), server: (f) => (piano[f.allegato] ? SERVER[piano[f.allegato]]() : SERVER.pdf()) });
    const [k1, k2] = uniche([...s.d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini));
    // le altre miniature della pagina rispondono 404: solo così «nessun getDocument» parla di questi due casi
    // (integrazione di A1d-Q3: e5 ha tre miniature)
    piano = {};
    for (const k of uniche([...s.d.querySelectorAll("#ma [data-ma-mini][data-ma-allegato]")].map(chiaveMini))) piano[k.split("|")[0]] = "404";
    piano[k1.split("|")[0]] = "login";
    piano[k2.split("|")[0]] = "html";
    vedi(s);
    await attendi(80);
    ok(/sessione scaduta/i.test(spanDi(s, k1).map(testo).join(" ")), `${v.pagina}: un rinvio a /login → «sessione scaduta»`);
    ok(/non è un PDF/i.test(spanDi(s, k2).map(testo).join(" ")), `${v.pagina}: un'altra risposta → «la risposta non è un PDF»`);
    ok(s.pdf.stato.documenti.length === 0, `${v.pagina}: in quei due casi nessun getDocument`);
    chiudi(s);
  }
});

// 9. anteprima persistente: il palco, la rilettura senza richieste nuove, l'alternativo, il visore
gruppoDi("anteprima", async () => {
  const conPalco = (d, conAlternativo) => {
    for (const p of d.querySelectorAll("#ma-anteprima [data-ma-pannello]")) {
      const palco = p.querySelector(".ma-palco[data-ma-palco][data-ma-allegato]");
      const alt = p.querySelectorAll("[data-ma-azione=disegno][data-ma-allegato]");
      const k = p.getAttribute("data-ma-pannello");
      if (palco && caselleDi(d, k).length && (!conAlternativo || alt.length >= 2)) return { k, allegato: palco.getAttribute("data-ma-allegato").toLowerCase(), alt: alt.length };
    }
    return null;
  };
  const t = cerca(pagine({ vista: "distinta" }), (d) => conPalco(d, true)) || cerca(pagine({ vista: "distinta" }), (d) => conPalco(d, false));
  if (!ok(!!t, "una pagina con una casella che ha il disegno (.ma-palco[data-ma-allegato])")) return;
  ok(t.r.alt >= 2, "una casella con il disegno principale e un alternativo (e13)", String(t.r.alt));
  const v = t.voce;
  const k = t.r.k;
  const s = await apri(v, { larghezze: [{ sel: ".ma-palco", larghezza: 360 }], server: (f) => (f.allegato === t.r.allegato ? SERVER.pdf2() : SERVER.pdf()) });
  const { d } = s;
  await scegliCasella(s, k, 100);
  const pann = pannelloDi(d, k);
  ok(visibile(pann), `${v.pagina}: scelta la casella, si vede il suo pannello`);
  ok([...d.querySelectorAll("#ma-anteprima [data-ma-pannello]")].filter(visibile).length === 1, `${v.pagina}: un solo pannello visibile`);
  ok(caselleDi(d, k).every((li) => li.getAttribute("aria-selected") === "true"), `${v.pagina}: la casella è aria-selected`);
  ok(s.memoria("nodo." + s.prodotto()) === k, `${v.pagina}: ma.<t>.nodo.<prodotto> = la chiave della casella`);
  const palcoFetch = s.spie.fetch.filter((f) => f.allegato === t.r.allegato);
  ok(palcoFetch.length === 1, `${v.pagina}: una GET del PDF per il palco`, String(palcoFetch.length));
  const palco = pann && pann.querySelector(".ma-palco");
  ok(!!palco && !!palco.querySelector("canvas, img"), `${v.pagina}: il palco si riempie (pagina 1 del PDF)`);
  ok(/1\s*di\s*2/.test(testo(pann)), `${v.pagina}: il pannello dice «pagina 1 di 2» (il PDF ha due pagine)`);
  const primo = pann && pann.querySelector("[data-ma-azione=pdf]");
  ok(!!primo, `${v.pagina}: nel pannello «Apri il visore» ([data-ma-azione=pdf])`);
  vedi(s);
  await attendi(80);
  const nf = s.spie.fetch.length;
  const ng = s.pdf.stato.documenti.length;
  if (v.frammento) {
    await s.rileggi();
    await attendi(60);
    const p2 = pannelloDi(d, k);
    ok(!!p2 && p2 !== pann && visibile(p2), `${v.pagina}: dopo la rilettura lo stesso pannello è visibile (anteprima persistente)`);
    ok(caselleDi(d, k).every((li) => li.getAttribute("aria-selected") === "true"), `${v.pagina}: dopo la rilettura la casella è ancora scelta`);
    ok(!!(p2 && p2.querySelector(".ma-palco canvas, .ma-palco img")), `${v.pagina}: dopo la rilettura il palco è pieno`);
    vedi(s);
    await attendi(60);
    ok(s.spie.fetch.length === nf && s.pdf.stato.documenti.length === ng, `${v.pagina}: dopo la rilettura nessuna GET e nessun getDocument nuovi (cache del palco e delle miniature, MF17)`, `${s.spie.fetch.length - nf} GET, ${s.pdf.stato.documenti.length - ng} getDocument in più`);
  }
  // l'alternativo: aria-pressed, ma.<t>.disegno.<prodotto>, ripristinato dopo la rilettura
  const bott = () => [...(pannelloDi(d, k) || d).querySelectorAll("[data-ma-azione=disegno][data-ma-allegato]")];
  const alt = bott().find((b) => b.getAttribute("aria-pressed") !== "true");
  if (alt) {
    const a = alt.getAttribute("data-ma-allegato");
    await clicca(s, alt, 80);
    ok(bott().every((b) => b.getAttribute("aria-pressed") === (b.getAttribute("data-ma-allegato") === a ? "true" : "false")), `${v.pagina}: l'alternativo scelto ha aria-pressed=true, gli altri false`);
    let m = null;
    try { m = JSON.parse(s.memoria("disegno." + s.prodotto()) || "null"); } catch (e) { m = null; }
    ok(!!m && m.nodo === k && String(m.allegato).toLowerCase() === a.toLowerCase(), `${v.pagina}: ma.<t>.disegno.<prodotto> = {nodo, allegato}`);
    const dl = [...(pannelloDi(d, k) || d).querySelectorAll("dl[data-ma-rev-di]")];
    if (dl.length) ok(dl.every((x) => visibile(x) === (x.getAttribute("data-ma-rev-di").toLowerCase() === a.toLowerCase())), `${v.pagina}: si vede la revisione del disegno mostrato`);
    if (v.frammento) {
      await s.rileggi();
      await attendi(60);
      const b2 = bott().find((b) => b.getAttribute("data-ma-allegato") === a);
      ok(!!b2 && b2.getAttribute("aria-pressed") === "true", `${v.pagina}: dopo la rilettura l'alternativo è ancora quello mostrato`);
    }
  }
  // il visore: una finestra senza iframe, pdf.js con l'indirizzo dell'anteprima
  const visore = (pannelloDi(d, k) || d).querySelector("[data-ma-azione=pdf]");
  // revisione P3, S-04: senza l'impronta «Apri il visore» non chiede niente (mai /allegato/{id}/anteprima senza ?v=)
  if (visore) {
    const sha = visore.getAttribute("data-ma-sha");
    visore.removeAttribute("data-ma-sha");
    const nf0 = s.spie.fetch.length;
    const nd0 = s.pdf.stato.documenti.length;
    await clicca(s, visore, 60);
    ok(s.spie.fetch.length === nf0 && s.pdf.stato.documenti.length === nd0 && !finestraAperta(d), `${v.pagina}: «Apri il visore» senza sha non chiede niente e non apre il visore (S-04)`);
    visore.setAttribute("data-ma-sha", sha);
  }
  if (visore) {
    s.visore = "2pagine";
    const ng2 = s.pdf.stato.documenti.length;
    await clicca(s, visore, 80);
    const f = finestraAperta(d);
    ok(!!f && !f.closest("#ma"), `${v.pagina}: «Apri il visore» apre una finestra .ma-finestra fuori da #ma (§7.6)`);
    const nuovi = s.pdf.stato.documenti.slice(ng2);
    ok(nuovi.length >= 1 && nuovi.every((x) => x.dati || RE_PDF.test(new URL(x.url || "/", ORIGINE).pathname)), `${v.pagina}: il visore apre il PDF di /allegato/<id>/anteprima con pdf.js`);
    ok(!d.querySelector("iframe"), `${v.pagina}: il visore non usa un <iframe>`);
  }
  chiudi(s);
});

// 10. foglio: sotto 860 px l'anteprima è un foglio dal basso; Esc lo chiude e il fuoco torna alla casella
gruppoDi("foglio", async () => {
  const t = cerca(pagine({ vista: "distinta" }), (d) => {
    const p = [...d.querySelectorAll("#ma-anteprima [data-ma-pannello]")].find((x) => caselleDi(d, x.getAttribute("data-ma-pannello")).length);
    return p ? p.getAttribute("data-ma-pannello") : null;
  });
  if (!ok(!!t, "una pagina con una casella e il suo pannello")) return;
  const v = t.voce;
  const k = t.r;
  const s = await apri(v, { finestra: 420, larghezze: [{ sel: ".ma-ws", larghezza: 400 }, { sel: ".ma-palco", larghezza: 380 }] });
  const { d } = s;
  const ws = () => d.querySelector("#ma .ma-ws");
  await scegliCasella(s, k, 80);
  ok(!!ws() && ws().classList.contains("ma-ws--foglio"), `${v.pagina}: a 400 px la casella apre il foglio (.ma-ws--foglio)`);
  const a = d.activeElement;
  ok(!!a && !!a.closest && a.closest("[data-ma-pannello]") === pannelloDi(d, k) && a.getAttribute("tabindex") === "-1", `${v.pagina}: il fuoco sul titolo del foglio (tabindex=-1)`);
  ok(s.memoria("foglio") === "1", `${v.pagina}: ma.<t>.foglio = 1`);
  if (v.frammento) {
    await s.rileggi();
    ok(ws().classList.contains("ma-ws--foglio"), `${v.pagina}: dopo la rilettura il foglio aperto si riapre`);
  }
  await tasto(s, d.activeElement, "Escape", 40);
  ok(!ws().classList.contains("ma-ws--foglio"), `${v.pagina}: Esc chiude il foglio`);
  const b = d.activeElement;
  ok(!!b && !!b.closest && !!b.closest("li[role=treeitem]") && b.closest("li[role=treeitem]").getAttribute("data-ma-chiave") === k, `${v.pagina}: dopo Esc il fuoco torna alla casella`);
  ok(s.memoria("foglio") !== "1", `${v.pagina}: chiuso il foglio, ma.<t>.foglio non è più 1`);
  if (v.frammento) {
    await s.rileggi();
    ok(!ws().classList.contains("ma-ws--foglio"), `${v.pagina}: dopo la rilettura il foglio chiuso resta chiuso`);
  }
  chiudi(s);
  // a 1440 px niente foglio
  const s2 = await apri(v, { larghezze: [{ sel: ".ma-ws", larghezza: 1300 }] });
  await scegliCasella(s2, k, 60);
  ok(!s2.d.querySelector("#ma .ma-ws--foglio"), `${v.pagina}: a 1300 px di contenitore nessun foglio`);
  chiudi(s2);
});

// 11. associa: e17 i prodotti del file, nessuna scelta; e15 solo il fieldset del componente scelto
gruppoDi("associa", async () => {
  const e17 = pagine({ caso: "e17", vista: "distinta", scrive: true });
  if (ok(e17.length > 0, "ci sono le pagine di e17")) {
    const v = e17[0];
    const s = await apri(v);
    const { d } = s;
    const forms = [...d.querySelectorAll("#ma-da-smistare form[data-ma-gesto]")].filter((f) => f.getAttribute("data-ma-gesto").endsWith("/fascicolo/assegna") && f.querySelector("select[name=componente]"));
    ok(forms.length >= 2, `${v.pagina}: «Associa a…» per i file da smistare`, String(forms.length));
    const etichette = [];
    for (const f of forms) {
      const sel = f.querySelector("select[name=componente]");
      const gruppi = [...sel.querySelectorAll("optgroup")];
      etichette.push(...gruppi.map((g) => g.getAttribute("label") || ""));
      ok(gruppi.length >= 1, `${v.pagina}: le opzioni sono raggruppate per prodotto (optgroup)`);
      ok(sel.required && sel.value === "" && sel.options[0] && sel.options[0].disabled, `${v.pagina}: nessun componente scelto in partenza (segnaposto spento)`);
      ok([...sel.options].every((o, i) => i === 0 || !o.defaultSelected), `${v.pagina}: nessuna opzione selected`);
      ok(!f.querySelector("[name=scelta]"), `${v.pagina}: con una proposta nessuna «scelta» (FE-11)`);
      ok(campiStatici(f).some(([k, x]) => k === "proposta" && RE_UUID.test(x)), `${v.pagina}: il modulo porta la proposta`);
    }
    ok(etichette.some((x) => /del file/i.test(x)), `${v.pagina}: un gruppo «del file»`);
    ok(etichette.some((x) => /dal messaggio/i.test(x)), `${v.pagina}: un gruppo «dal messaggio»`);
    ok(etichette.some((x) => /prodotto scelto/i.test(x)) && /il file non ha prodotti/i.test(testo(d.getElementById("ma-da-smistare"))), `${v.pagina}: senza prodotti, quelli del prodotto scelto con l'avviso (T-A1d-FE-06)`);
    const f = forms[0];
    if (f) {
      const sel = f.querySelector("select[name=componente]");
      const o = opzioniConValore(sel)[0];
      if (ok(!!o, `${v.pagina}: c'è un componente da scegliere`)) {
        await scegliValore(s, sel, o.value);
        const n0 = s.scritture().length;
        await clicca(s, inviatore(f), 50);
        const g = s.scritture().slice(n0);
        ok(g.length === 1 && g[0].campi.some(([k2, x]) => k2 === "componente" && x === o.value) && !g[0].campi.some(([k2]) => k2 === "scelta"), `${v.pagina}: «Associa» manda proposta e componente, senza scelta`, JSON.stringify(g.map((x) => x.campi)));
      }
    }
    chiudi(s);
  }
  const e15 = pagine({ caso: "e15", vista: "distinta", scrive: true }).concat(pagine({ caso: "e15", vista: "documenti", scrive: true }));
  if (ok(e15.length > 0, "ci sono le pagine di e15")) {
    const v = e15[0];
    const s = await apri(v);
    const { d } = s;
    const sel = [...d.querySelectorAll("#ma select[data-ma-scelte]")].find((x) => {
      const f = x.closest("form");
      return f && f.querySelector("fieldset[data-ma-scelta-per]");
    });
    if (ok(!!sel, `${v.pagina}: un select[data-ma-scelte] con le sue scelte`)) {
      const form = sel.closest("form");
      const fs_ = [...form.querySelectorAll("fieldset[data-ma-scelta-per]")];
      ok(campiStatici(form).some(([k2, x]) => k2 === "documento" && RE_UUID.test(x)), `${v.pagina}: le scelte stanno solo nel modulo con documento (FE-11)`);
      ok(fs_.every((x) => x.disabled && x.hidden), `${v.pagina}: all'apertura ogni fieldset di scelta spento e nascosto`);
      ok(sel.value === "", `${v.pagina}: nessun componente preselezionato`);
      const radio = [...form.querySelectorAll("input[name=scelta]")];
      ok(radio.length > 0 && radio.every((r) => !r.checked && RE_SCELTA.test(r.value) && r.required), `${v.pagina}: le scelte sono aggiungi o sostituisci:<uuid>, obbligatorie e non spuntate`);
      const apri_ = [...d.querySelectorAll("#ma [data-ma-azione=ispeziona]")].find((b) => {
        const dlg = d.getElementById(b.getAttribute("data-ma-cassetto") || "");
        return dlg && dlg.contains(sel);
      });
      if (apri_) await clicca(s, apri_, 30);
      const per = fs_[0].getAttribute("data-ma-scelta-per");
      await scegliValore(s, sel, per);
      ok(fs_.every((x) => (x.getAttribute("data-ma-scelta-per") === per ? !x.disabled && !x.hidden : x.disabled && x.hidden)), `${v.pagina}: scelto il componente, si accende solo il suo fieldset`);
      const n0 = s.scritture().length;
      await clicca(s, inviatore(form), 40);
      ok(s.scritture().length === n0, `${v.pagina}: senza una scelta il modulo non parte (radio obbligatorio)`);
      const r = [...fs_[0].querySelectorAll("input[name=scelta]")].find((x) => x.value === "aggiungi") || fs_[0].querySelector("input[name=scelta]");
      if (r) {
        await clicca(s, r, 10);
        await clicca(s, inviatore(form), 50);
        const g = s.scritture().slice(n0);
        ok(g.length === 1 && g[0].campi.some(([k2, x]) => k2 === "scelta" && x === r.value) && g[0].campi.some(([k2, x]) => k2 === "componente" && x === per), `${v.pagina}: con la scelta parte una POST con documento, componente e scelta`, JSON.stringify(g.map((x) => x.campi)));
      }
    }
    chiudi(s);
  }
});

// 12. conferma documento: sempre una sola voce; la frase DB7-2 solo con BOM.Verificata falso
gruppoDi("conferma documento", async () => {
  let n = 0;
  for (const v of CASI.filter((x) => x.scrive && x.vista !== "rfq")) {
    const ha = leggi(v, (d) => d.querySelectorAll("#ma [data-ma-gesto$='/fascicolo/conferma']").length);
    if (!ha) continue;
    const verificata = bomVerificata(v.caso, v.prodotto);
    if (verificata === null) note.add(`conferma documento: BOM.Verificata non letta dal caso ${v.caso}`);
    const s = await apri(v);
    for (const g of [...s.d.querySelectorAll("#ma [data-ma-gesto$='/fascicolo/conferma']")]) {
      if (gestoSpento(g)) continue;
      n++;
      const domanda = g.getAttribute("data-ma-domanda") || "";
      ok(/NAS/.test(domanda), `${v.pagina}: «Conferma il documento» chiede prima (la copia sul NAS parte subito)`, domanda.slice(0, 80));
      if (verificata !== null) ok(FRASE_DB7_2.test(domanda) === !verificata, `${v.pagina}: la frase DB7-2 c'è se e solo se la BOM non è verificata (${verificata ? "verificata" : "non verificata"})`, domanda.slice(0, 160));
      const n0 = s.scritture().length;
      await clicca(s, g.tagName === "FORM" ? inviatore(g) : g, 40);
      const nuove = s.scritture().slice(n0);
      ok(nuove.length === 1 && nuove[0].campi.length === 1 && nuove[0].campi[0][0] === "voce" && RE_UUID.test(nuove[0].campi[0][1]), `${v.pagina}: la conferma manda una sola voce, nient'altro (FE-12, MF08)`, JSON.stringify(nuove.map((x) => x.campi)));
    }
    chiudi(s);
  }
  ok(n > 0, `pulsanti «Conferma il documento» provati: ${n}`);
});

// 13. albero: confermabile falso → spento; 422 → errore nella finestra e riepilogo riletto; ok → nessuna aggiorna()
gruppoDi("albero", async () => {
  const t = cerca(pagine({ vista: "distinta", scrive: true }), (d) => [...d.querySelectorAll("#ma-albero [data-ma-azione=albero]")].some((b) => !spento(b)) || null);
  if (!ok(!!t, "una pagina con «Leggi il riepilogo e conferma l'albero…» acceso")) return;
  const v = t.voce;
  const leggiRiep = (s) => s.letture().filter((g) => /\/distinta\/albero\/riepilogo$/.test(g.url.split("?")[0]));
  const apriFinestra = async (copione) => {
    const s = await apri(v, { copione });
    await clicca(s, s.d.querySelector("#ma-albero [data-ma-azione=albero]"), 60);
    const f = finestraAperta(s.d);
    return { s, f, invia: f && bottoneDi(f, /conferma l.albero/i) };
  };
  {
    const { s, f, invia } = await apriFinestra({ letture: [{ url: "/distinta/albero/riepilogo", json: RIEPILOGO(false) }] });
    ok(!!f && !f.closest("#ma"), `${v.pagina}: il riepilogo si apre in una finestra fuori da #ma`);
    ok(leggiRiep(s).length === 1, `${v.pagina}: il riepilogo si legge con Gesti.leggi (GET …/distinta/albero/riepilogo)`);
    ok(!!invia && invia.disabled, `${v.pagina}: confermabile falso → «Conferma l'albero» spento`);
    // revisione P3, M6: una voce in una riga, prima il codice e il tipo, mai un oggetto scritto come JSON
    const voci = f ? [...f.querySelectorAll(".ma-riep li")].map((li) => li.textContent) : [];
    ok(voci.length > 0 && voci.every((x) => !x.includes('{"') && !x.includes('":')), `${v.pagina}: le voci del riepilogo senza JSON (M6)`, voci.slice(0, 2).join(" || "));
    ok(voci.some((x) => /^codice: 7120110 · tipo: sciolto · /.test(x) && /lettura: \(base: 7120110 · rev: A\)/.test(x)), `${v.pagina}: la voce comincia con codice e tipo; l'oggetto annidato fra parentesi (M6)`, voci.slice(0, 2).join(" || "));
    chiudi(s);
  }
  {
    const { s, f, invia } = await apriFinestra({ letture: [{ url: "/distinta/albero/riepilogo", json: RIEPILOGO(true) }], albero: { esito: "no", testo: "L'albero è cambiato mentre lo guardavi: rileggi il riepilogo (prova ACME)." } });
    if (ok(!!invia && !invia.disabled, `${v.pagina}: confermabile vero e firma → «Conferma l'albero» acceso`)) {
      await clicca(s, invia, 80);
      const post = s.scritture().filter((g) => /\/distinta\/albero\/conferma$/.test(g.url));
      ok(post.length === 1 && post[0].corpo && post[0].corpo.firma === "acme-firma-albero-0001" && post[0].corpo.bozza && post[0].corpo.bozza.formato === 1 && post[0].corpo.bozza.base === "acme-albero-0001", `${v.pagina}: confermaAlbero(firma, {formato: 1, base: albero_firma})`, JSON.stringify(post.map((x) => x.corpo)));
      const f2 = finestraAperta(s.d);
      ok(!!f2 && /cambiato mentre lo guardavi/.test(testo(f2)), `${v.pagina}: 422 → la finestra resta aperta con l'errore`);
      ok(leggiRiep(s).length === 2, `${v.pagina}: 422 → il riepilogo si rilegge`);
      ok(s.spie.ajax.length === 0 && s.rilettureDelModulo().length === 0, `${v.pagina}: 422 → nessuna rilettura della pagina`);
    }
    chiudi(s);
  }
  {
    const { s, invia } = await apriFinestra({ letture: [{ url: "/distinta/albero/riepilogo", json: RIEPILOGO(true) }], albero: { esito: "errore", testo: "Errore 500 (prova ACME)" } });
    if (invia) {
      await clicca(s, invia, 80);
      const f2 = finestraAperta(s.d);
      ok(!!f2 && /Errore 500/.test(testo(f2)), `${v.pagina}: un errore del server → la finestra resta con il testo dell'errore`);
      ok(s.spie.ajax.length === 0, `${v.pagina}: un errore → nessuna rilettura`);
    }
    chiudi(s);
  }
  {
    const { s, invia } = await apriFinestra({ letture: [{ url: "/distinta/albero/riepilogo", json: RIEPILOGO(true) }], albero: { esito: "ok", testo: "Albero confermato (prova ACME)." } });
    if (invia) {
      await clicca(s, invia, 100);
      ok(!finestraAperta(s.d), `${v.pagina}: ok → la finestra si chiude`);
      ok(s.rilettureDelPonte().length === 1, `${v.pagina}: ok → rilegge il ponte (X-08)`);
      ok(s.rilettureDelModulo().length === 0, `${v.pagina}: ok → motore_a.mjs non chiama Gesti.aggiorna() (X-08, MF14)`);
      ok(s.d.getElementById("ma-avviso").getAttribute("data-esito") === "ok", `${v.pagina}: ok → #ma-avviso[data-esito=ok]`);
    }
    chiudi(s);
  }
  for (const w of pagine({ vista: "distinta", scrive: false }).slice(0, 3)) {
    leggi(w, (d) => ok([...d.querySelectorAll("#ma [data-ma-azione=albero]")].every(spento), `${w.pagina}: senza Scrive «conferma l'albero» spento`));
  }
});

// 14. step e tipo: le anteprime con la firma; «autorizzo» non spuntata; Autorizza solo con inviabile, firma, spunte
gruppoDi("step e tipo", async () => {
  const t = cerca(pagine({ vista: "distinta", scrive: true }), (d) => [...d.querySelectorAll("#ma-fonte [data-ma-azione=step]")].some((b) => !spento(b)) || null);
  if (ok(!!t, "una pagina con «Autorizza lo STEP strutturale…» acceso (e2)")) {
    const v = t.voce;
    const apriStep = async (anteprima, gesti) => {
      const s = await apri(v, { copione: { letture: [{ url: "/motore-a/anteprima/step-strutturale", json: anteprima }], gesti: gesti || [] } });
      const b = [...s.d.querySelectorAll("#ma-fonte [data-ma-azione=step]")].find((x) => !spento(x));
      await clicca(s, b, 80);
      const f = finestraAperta(s.d);
      return { s, f, autorizzo: f && casellaSpunta(f, "autorizzo", /autorizz/i), presa: f && casellaSpunta(f, "presa_d_atto", /prendo atto|presa d.atto/i), invia: f && bottoneDi(f, /^autorizza/i) };
    };
    {
      const { s, f, autorizzo, presa, invia } = await apriStep(ANTEPRIMA_STEP(), [{ re: "/step-strutturale$", esito: "no", testo: "Niente è cambiato: la firma dell'anteprima è cambiata (prova ACME)." }]);
      const lett = s.letture().filter((g) => /\/motore-a\/anteprima\/step-strutturale$/.test(g.url.split("?")[0]));
      const q = lett[0] ? new URL(lett[0].url, ORIGINE).searchParams : new URLSearchParams();
      ok(lett.length === 1 && RE_UUID.test(q.get("componente") || ""), `${v.pagina}: l'anteprima si legge con componente`, lett.map((g) => conId(g.url)).join(", "));
      ok((q.has("documento") ? 1 : 0) + (q.has("nodo") ? 1 : 0) === 1 && (!q.has("documento") || RE_UUID.test(q.get("documento"))), `${v.pagina}: l'anteprima con documento oppure nodo (uno solo)`);
      ok(!!f && !f.closest("#ma"), `${v.pagina}: l'anteprima dello STEP in una finestra fuori da #ma`);
      ok(!!autorizzo && !autorizzo.checked, `${v.pagina}: «autorizzo» non spuntata in partenza`);
      ok(!!invia && invia.disabled, `${v.pagina}: «Autorizza» spento finché non si spunta`);
      const d2 = f && f.querySelector("input[name=delega][value=d2]");
      const d1 = f && f.querySelector("input[name=delega][value=d1]");
      ok(!!d2 && d2.disabled && !d2.checked, `${v.pagina}: la delega spenta non si può scegliere`);
      ok(!!d1 && !d1.disabled && !d1.checked, `${v.pagina}: la delega libera si può scegliere e parte non spuntata`);
      if (autorizzo && presa && invia) {
        await clicca(s, autorizzo, 15);
        ok(invia.disabled, `${v.pagina}: senza la presa d'atto «Autorizza» resta spento`);
        await clicca(s, presa, 15);
        ok(!invia.disabled, `${v.pagina}: con inviabile, firma, «autorizzo» e la presa d'atto «Autorizza» si accende`);
        if (d1) await clicca(s, d1, 10);
        await clicca(s, invia, 100);
        const post = s.scritture().filter((g) => /\/step-strutturale$/.test(g.url));
        const c = post[0] ? Object.fromEntries(post[0].campi) : {};
        const tutti = post[0] ? post[0].campi : [];
        ok(post.length === 1 && post[0].url === `/thread/${s.tid}/fascicolo/componente/${q.get("componente")}/step-strutturale`, `${v.pagina}: la POST va a …/componente/<il componente dell'anteprima>/step-strutturale`, post.map((g) => conId(g.url)).join(", "));
        ok(c.firma === "acme-firma-step-0001" && c.autorizza === "1" && c.presa_d_atto === "1", `${v.pagina}: con firma, autorizza=1 e presa_d_atto=1`, JSON.stringify(tutti));
        ok((q.has("documento") ? c.documento === q.get("documento") : c.nodo === q.get("nodo")), `${v.pagina}: con lo stesso documento o nodo dell'anteprima`);
        ok(tutti.filter(([k]) => k === "delega").map(([, x]) => x).join(",") === (d1 ? "d1" : ""), `${v.pagina}: la delega spenta non parte, quella scelta sì`);
        const f2 = finestraAperta(s.d);
        ok(!!f2 && /firma dell'anteprima è cambiata/.test(testo(f2)), `${v.pagina}: un «no» lascia la finestra aperta con il motivo`);
        const a2 = f2 && casellaSpunta(f2, "autorizzo", /autorizz/i);
        ok(s.letture().filter((g) => /\/anteprima\/step-strutturale$/.test(g.url.split("?")[0])).length === 2 && !!a2 && !a2.checked, `${v.pagina}: dopo il «no» l'anteprima si rilegge e «autorizzo» torna vuota`);
      } else ok(false, `${v.pagina}: nella finestra mancano «autorizzo», la presa d'atto o «Autorizza»`);
      chiudi(s);
    }
    for (const [extra, cosa] of [[{ inviabile: false }, "inviabile falso"], [{ firma: "" }, "senza firma"]]) {
      const { s, autorizzo, presa, invia } = await apriStep(ANTEPRIMA_STEP(extra));
      if (autorizzo) await clicca(s, autorizzo, 10);
      if (presa) await clicca(s, presa, 10);
      ok(!!invia && invia.disabled, `${v.pagina}: ${cosa} → «Autorizza» spento anche con le spunte`);
      chiudi(s);
    }
  }
  // tipo: nessuna preselezione; l'effetto prima; il cambio con la firma
  const tt = cerca(pagine({ vista: "distinta", scrive: true }), (d) => {
    const f = [...d.querySelectorAll("#ma-anteprima form[data-ma-azione=tipo]")].find((x) => !gestoSpento(x));
    return f ? f.closest("[data-ma-pannello]").getAttribute("data-ma-pannello") : null;
  });
  if (ok(!!tt, "una pagina con «Vedi l'effetto del cambio…» acceso")) {
    const v = tt.voce;
    const s = await apri(v, { copione: { letture: [{ url: "/distinta/tipo", json: ANTEPRIMA_TIPO }], gesti: [{ re: "/tipo$", esito: "no", testo: "Niente è cambiato (prova ACME)." }] } });
    await scegliCasella(s, tt.r, 60);
    const form = (pannelloDi(s.d, tt.r) || s.d).querySelector("form[data-ma-azione=tipo]");
    const sel = form && form.querySelector("select[name=tipo]");
    if (ok(!!sel && sel.required && sel.value === "" && sel.options[0].disabled, `${v.pagina}: tipo con «scegli» spento e selezionato (nessuna preselezione)`)) {
      // solo una nota, non un controllo: il cambio di tipo del componente può offrire «commerciale» (contratto §3.4 B)
      if ([...sel.options].some((o) => o.value === "commerciale")) note.add("tipo: «commerciale» offerto nel cambio di tipo del componente");
      await clicca(s, inviatore(form), 30);
      ok(s.letture().length === 0, `${v.pagina}: senza un tipo scelto nessuna anteprima`);
      const o = opzioniConValore(sel)[0];
      await scegliValore(s, sel, o.value);
      await clicca(s, inviatore(form), 80);
      const l = s.letture().filter((g) => /\/distinta\/tipo$/.test(g.url.split("?")[0]));
      const q = l[0] ? new URL(l[0].url, ORIGINE).searchParams : new URLSearchParams();
      ok(l.length === 1 && RE_UUID.test(q.get("componente") || "") && q.get("tipo") === o.value, `${v.pagina}: l'effetto si legge con componente e tipo`);
      const f = finestraAperta(s.d);
      const invia = f && bottoneDi(f, /cambia il tipo/i);
      if (ok(!!invia && !invia.disabled, `${v.pagina}: con la firma «Cambia il tipo» acceso`)) {
        await clicca(s, invia, 80);
        const post = s.scritture().filter((g) => /\/tipo$/.test(g.url));
        ok(post.length === 1 && post[0].url === `/thread/${s.tid}/fascicolo/componente/${q.get("componente")}/tipo`, `${v.pagina}: la POST va a …/componente/<cid>/tipo`);
        ok(post.length === 1 && post[0].campi.some(([k, x]) => k === "tipo" && x === o.value) && post[0].campi.some(([k, x]) => k === "firma" && x === "acme-firma-tipo-0001"), `${v.pagina}: con tipo e firma`);
      }
    }
    chiudi(s);
  }
});

// 15. esiti: un «no» non cambia la pagina (salvo #ma-avviso); un «ok» la rilegge; «Aggiorna» rilegge e basta
gruppoDi("esiti", async () => {
  const t = cerca(pagine({ vista: "distinta", scrive: true }), (d) => {
    const b = [...d.querySelectorAll("#ma button[data-ma-gesto]")].find((x) => !spento(x) && !x.hasAttribute("data-ma-domanda") && !x.closest("form"));
    return b ? b.getAttribute("data-ma-gesto") : null;
  });
  if (!ok(!!t, "una pagina con un gesto senza domanda (Rifai l'analisi)")) return;
  const v = t.voce;
  const bott = (s) => [...s.d.querySelectorAll("#ma button[data-ma-gesto]")].find((x) => x.getAttribute("data-ma-gesto") === t.r);
  {
    const s = await apri(v, { copione: { gesti: [{ url: t.r, esito: "no", testo: "Niente è cambiato: già in corso (prova ACME)." }] } });
    const prima = serializza(s, s.d.getElementById("ma"));
    await clicca(s, bott(s), 50);
    const av = s.d.getElementById("ma-avviso");
    ok(av.getAttribute("data-esito") === "no" && /già in corso/.test(testo(av)), `${v.pagina}: un «no» va in #ma-avviso[data-esito=no] con il testo`);
    ok(serializza(s, s.d.getElementById("ma")) === prima, `${v.pagina}: un «no» non cambia la pagina (#ma uguale)`);
    ok(s.spie.ajax.length === 0 && s.gesti().every((g) => g.esito !== "rilettura"), `${v.pagina}: un «no» non rilegge`);
    await clicca(s, av, 20);
    ok(testo(av) === "", `${v.pagina}: un clic su #ma-avviso lo svuota (§7.7)`);
    chiudi(s);
  }
  {
    const s = await apri(v, { copione: { gesti: [{ url: t.r, esito: "errore", testo: "Errore 500 (prova ACME)" }] } });
    const prima = serializza(s, s.d.getElementById("ma"));
    await clicca(s, bott(s), 50);
    ok(s.d.getElementById("ma-avviso").getAttribute("data-esito") === "errore" && serializza(s, s.d.getElementById("ma")) === prima, `${v.pagina}: un errore non cambia la pagina`);
    chiudi(s);
  }
  if (v.frammento) {
    const s = await apri(v, { copione: { gesti: [{ url: t.r, esito: "ok", testo: "Fatto (prova ACME)." }] } });
    const primo = s.d.querySelector("#ma > .ma");
    await clicca(s, bott(s), 80);
    ok(s.d.getElementById("ma-avviso").getAttribute("data-esito") === "ok", `${v.pagina}: un «ok» va in #ma-avviso[data-esito=ok]`);
    ok(s.spie.ajax.length === 1 && s.d.querySelector("#ma > .ma") !== primo, `${v.pagina}: un «ok» rilegge #ma una volta (dal ponte)`);
    chiudi(s);
  }
  const ag = cerca(pagine({ vista: "distinta" }), (d) => (d.querySelector("#ma [data-ma-azione=aggiorna]") ? true : null));
  if (ok(!!ag, "una pagina con «Aggiorna»") && ag.voce.frammento) {
    const s = await apri(ag.voce);
    await clicca(s, s.d.querySelector("#ma [data-ma-azione=aggiorna]"), 60);
    ok(s.rilettureDelModulo().length === 1 && s.scritture().length === 0, `${ag.voce.pagina}: «Aggiorna» = Gesti.aggiorna(): una rilettura, nessuna scrittura`);
    chiudi(s);
  }
});

// 16. tre operazioni da non confondere: scarto del nodo, scarto documentale, disassociazione
gruppoDi("tre operazioni", async () => {
  const tipi = { nodo: [], file: [], togli: [] };
  for (const v of CASI.filter((x) => x.scrive && x.vista !== "rfq")) {
    leggi(v, (d) => {
      for (const [i, g] of [...d.querySelectorAll("#ma [data-ma-gesto]")].entries()) {
        const r = g.getAttribute("data-ma-gesto");
        const voce = { v, r, i, etichetta: testo(g.tagName === "FORM" ? inviatore(g) : g), domanda: g.getAttribute("data-ma-domanda") || "" };
        if (/\/fascicolo\/nodo\/[^/]+\/scarta$/.test(r)) tipi.nodo.push(voce);
        else if (/^\/proposta\/[^/]+\/scarta$/.test(r)) tipi.file.push(voce);
        else if (/\/fascicolo\/assegna$/.test(r) && !g.querySelector("select[name=componente]") && campiStatici(g).some(([k, x]) => k === "componente" && x === "")) tipi.togli.push(voce);
      }
    });
  }
  for (const [k, nome] of [["nodo", "scarto del nodo"], ["file", "scarto documentale"], ["togli", "disassociazione"]]) ok(tipi[k].length > 0, `c'è almeno un gesto di ${nome}`, String(tipi[k].length));
  const etichette = (k) => new Set(tipi[k].map((x) => x.etichetta));
  const domande = (k) => new Set(tipi[k].map((x) => x.domanda.replace(/\{[^}]*\}|«[^»]*»/g, "")));
  for (const [a, b] of [["nodo", "file"], ["nodo", "togli"], ["file", "togli"]]) {
    ok([...etichette(a)].every((x) => !etichette(b).has(x)), `etichette diverse fra ${a} e ${b}`);
    ok([...domande(a)].every((x) => !domande(b).has(x)), `domande diverse fra ${a} e ${b}`);
  }
  ok(tipi.nodo.every((x) => /nodo/i.test(x.domanda) && /definitiv/i.test(x.domanda)), "lo scarto del nodo chiede «Scartare il nodo…? Lo scarto è definitivo…»");
  ok(tipi.file.every((x) => /file/i.test(x.domanda) && /definitiv/i.test(x.domanda)), "lo scarto del file chiede «Scartare il file…? Lo scarto è definitivo…» (DB7-8)");
  ok(tipi.togli.every((x) => /non viene scartato/i.test(x.domanda)), "togliere dal componente dice che il file non viene scartato");
  // un clic per tipo: la rotta e i campi che partono
  for (const k of ["nodo", "file", "togli"]) {
    const x = tipi[k][0];
    if (!x) continue;
    const s = await apri(x.v);
    // lo stesso elemento per posizione: «Associa» e «Togli dal componente» hanno la stessa rotta (integrazione di A1d-Q3)
    const g = [...s.d.querySelectorAll("#ma [data-ma-gesto]")][x.i];
    const n0 = s.scritture().length;
    await clicca(s, g.tagName === "FORM" ? inviatore(g) : g, 50);
    const p = s.scritture().slice(n0);
    ok(p.length === 1 && p[0].url === x.r, `${x.v.pagina}: ${k}: parte la sua rotta`);
    if (k === "file") ok(p.length === 1 && p[0].campi.some(([c, y]) => c === "ritorna_thread" && y === s.tid), `${x.v.pagina}: lo scarto del file porta ritorna_thread (lo aggiunge il ponte)`);
    if (k === "togli") ok(p.length === 1 && p[0].campi.some(([c, y]) => c === "componente" && y === "") && p[0].campi.some(([c]) => c === "proposta" || c === "documento"), `${x.v.pagina}: togliere manda componente vuoto con proposta o documento`);
    ok(s.spie.domande.length === 1, `${x.v.pagina}: ${k}: una domanda prima di partire`);
    chiudi(s);
  }
});

// 17. vecchio impianto: niente passi, binario, «Prossimo gesto», viste della distinta, conteggio dei tipi
gruppoDi("vecchio impianto", async () => {
  for (const v of CASI) {
    leggi(v, (d) => {
      const vecchi = SELETTORI_VECCHI.filter((x) => d.querySelector("#ma " + x));
      ok(vecchi.length === 0, `${v.pagina}: nessun ${SELETTORI_VECCHI.join(", ")}`, vecchi.join(", "));
      ok(!/Prossimo gesto/i.test(testo(d.getElementById("ma"))), `${v.pagina}: nessun «Prossimo gesto» (U-06)`);
      ok(!/dato non ancora disponibile \(EB7/i.test(testo(d.getElementById("ma"))), `${v.pagina}: nessuna scritta «dato non ancora disponibile (EB7…)» (X-07)`);
    });
  }
});

// 18. sola lettura: con Scrive falso ogni gesto spento, niente trascinamento, un clic non scrive
gruppoDi("sola lettura", async () => {
  const elenco = CASI.filter((x) => !x.scrive && x.vista !== "rfq" && x.prodotto && esitoNil(x.caso) !== true);
  ok(elenco.length > 0, "ci sono pagine con Scrive falso", String(elenco.length));
  let gesti = 0;
  for (const v of elenco) {
    const s = await apri(v);
    const { d } = s;
    const tutti = [...d.querySelectorAll("#ma [data-ma-gesto]")];
    gesti += tutti.length;
    const accesi = tutti.filter((g) => !gestoSpento(g));
    ok(accesi.length === 0, `${v.pagina}: ogni data-ma-gesto spento (MF10)`, uniche(accesi.map((g) => conId(g.getAttribute("data-ma-gesto")))).join(", "));
    const anteprime = [...d.querySelectorAll("#ma [data-ma-azione=step], #ma [data-ma-azione=albero], #ma form[data-ma-azione=tipo] button")].filter((b) => !spento(b));
    ok(anteprime.length === 0, `${v.pagina}: Autorizza, Albero e Tipo spenti`);
    ok(!d.querySelector("#ma [draggable=true], #ma [data-ma-drop], #ma [data-ma-sposta]"), `${v.pagina}: nessun draggable, data-ma-drop o data-ma-sposta`);
    for (const g of tutti.slice(0, 6)) await clicca(s, g.tagName === "FORM" ? inviatore(g) || g : g, 10);
    ok(s.scritture().length === 0, `${v.pagina}: i clic sui gesti spenti non scrivono`);
    if (pagine({ caso: "e11" }).includes(v)) ok(/sola lettura/i.test(testo(d.getElementById("ma-testa"))), `${v.pagina}: la striscia «sola lettura» (e11)`);
    chiudi(s);
  }
  ok(gesti > 0, `gesti controllati: ${gesti}`);
});

// 19. tastiera dell'albero: ↓ ↑ ← → Home End + - Invio Esc sui li[role=treeitem]
gruppoDi("tastiera albero", async () => {
  const t = cerca(pagine({ vista: "distinta" }), (d) => {
    for (const tab of d.querySelectorAll("#ma [role=tabpanel][data-ma-gruppo]:not([hidden])")) {
      const radice = tab.querySelector("[role=tree] > li[role=treeitem]");
      if (radice && radice.querySelectorAll(":scope > ul > li[role=treeitem], :scope > [role=group] > li[role=treeitem]").length >= 2) return radice.getAttribute("data-ma-cammino");
    }
    return null;
  });
  if (!ok(!!t, "una pagina con una radice che ha almeno due figli nella scheda visibile")) return;
  const v = t.voce;
  const s = await apri(v, { larghezze: [{ sel: ".ma-palco", larghezza: 360 }] });
  const { d } = s;
  const li = (c) => [...d.querySelectorAll("#ma li[role=treeitem]")].find((x) => x.getAttribute("data-ma-cammino") === c);
  const radice = li(t.r);
  const figli = [...radice.querySelectorAll(":scope > ul > li[role=treeitem], :scope > [role=group] > li[role=treeitem]")];
  ok(radice.getAttribute("tabindex") === "0", `${v.pagina}: la radice ha tabindex=0 (a rotazione)`);
  radice.focus();
  await tasto(s, radice, "ArrowDown");
  ok(d.activeElement === figli[0], `${v.pagina}: ↓ porta al primo figlio`);
  ok(figli[0].getAttribute("tabindex") === "0" && radice.getAttribute("tabindex") === "-1", `${v.pagina}: il tabindex segue il fuoco`);
  await tasto(s, d.activeElement, "ArrowRight");
  ok(d.activeElement === figli[1], `${v.pagina}: → porta al fratello dopo`);
  await tasto(s, d.activeElement, "ArrowLeft");
  ok(d.activeElement === figli[0], `${v.pagina}: ← porta al fratello prima`);
  await tasto(s, d.activeElement, "ArrowUp");
  ok(d.activeElement === radice, `${v.pagina}: ↑ porta al padre`);
  await tasto(s, d.activeElement, "End");
  const albero = radice.closest("[role=tree]");
  const tutte = [...albero.querySelectorAll("li[role=treeitem]")].filter((x) => !x.parentElement.closest("li[aria-expanded=false]"));
  ok(d.activeElement === tutte[tutte.length - 1], `${v.pagina}: End porta all'ultima casella visibile`);
  await tasto(s, d.activeElement, "Home");
  ok(d.activeElement === tutte[0], `${v.pagina}: Home porta alla prima`);
  await tasto(s, radice, "-");
  ok(radice.getAttribute("aria-expanded") === "false", `${v.pagina}: «-» chiude il ramo`);
  let chiusi = [];
  try { chiusi = JSON.parse(s.memoria("chiusi." + s.prodotto()) || "[]"); } catch (e) { chiusi = []; }
  ok(chiusi.includes(t.r), `${v.pagina}: il ramo chiuso va in ma.<t>.chiusi.<prodotto>`);
  await tasto(s, radice, "+");
  ok(radice.getAttribute("aria-expanded") === "true", `${v.pagina}: «+» lo riapre`);
  figli[0].focus();
  await tasto(s, figli[0], "Enter", 80);
  const k = figli[0].getAttribute("data-ma-chiave");
  ok(figli[0].getAttribute("aria-selected") === "true", `${v.pagina}: Invio sceglie la casella`);
  const a = d.activeElement;
  ok(!!a && !!a.closest && a.closest("[data-ma-pannello]") === pannelloDi(d, k) && a.getAttribute("tabindex") === "-1", `${v.pagina}: Invio porta il fuoco sul titolo dell'anteprima`);
  await tasto(s, d.activeElement, "Escape", 40);
  ok(!!d.activeElement && !!d.activeElement.closest && d.activeElement.closest("li[role=treeitem]") === figli[0], `${v.pagina}: Esc nell'anteprima riporta il fuoco alla casella`);
  chiudi(s);
});

// 20. e14 ed e16: «Da» dalle relazioni confermate; «Rev e descrizione» solo con la descrizione registrata; «Ripristina»
gruppoDi("e14, e16", async () => {
  const u10 = ACME("10");
  const u11 = ACME("11");
  const u12 = ACME("12");
  const u15 = ACME("15");
  const rottaDi = (g) => (g.getAttribute("data-ma-gesto") || "").toLowerCase();
  const e14 = pagine({ caso: "e14", vista: "distinta", scrive: true });
  if (ok(e14.length > 0, "ci sono le pagine di e14")) {
    const v = e14[0];
    const s = await apri(v);
    const { d } = s;
    const sposta = [...d.querySelectorAll("#ma form[data-ma-gesto]")].filter((g) => rottaDi(g).endsWith(`/componente/${u12}/sposta`));
    ok(sposta.length > 0, `${v.pagina}: c'è «Sposta» per ${u12.slice(-2)}`);
    for (const f of sposta) {
      const da = f.querySelector("select[name=da]");
      ok(!!da && opzioniConValore(da).map((o) => o.value.toLowerCase()).join(",") === u10 && da.value === "", `${v.pagina}: «Da» di Sposta = solo la relazione confermata (u10), nessuna preselezione (FE-08, MF06)`, da ? opzioniConValore(da).map((o) => conId(o.value)).join(",") : "nessun select");
    }
    const scollega = [...d.querySelectorAll("#ma [data-ma-gesto]")].filter((g) => rottaDi(g).endsWith(`/componente/${u12}/scollega`));
    const padri = scollega.map((g) => (campiStatici(g).find(([k]) => k === "padre") || [])[1]).map((x) => String(x).toLowerCase());
    ok(scollega.length > 0 && uniche(padri).join(",") === u10, `${v.pagina}: «Scollega» solo dalla relazione confermata (u10)`);
    const nodo = [...d.querySelectorAll("#ma .ma-nodo[data-ma-sposta]")].filter((n) => n.getAttribute("data-ma-sposta").toLowerCase().endsWith(`/componente/${u12}/sposta`));
    ok(nodo.length > 0 && nodo.every((n) => (n.getAttribute("data-ma-da") || "").toLowerCase() === u10), `${v.pagina}: data-ma-da = u10 sulla casella da trascinare`);
    const f = sposta[0];
    if (f) {
      const k = f.closest("[data-ma-pannello]") && f.closest("[data-ma-pannello]").getAttribute("data-ma-pannello");
      if (k) await scegliCasella(s, k, 50);
      const da = f.querySelector("select[name=da]");
      if (da) await scegliValore(s, da, opzioniConValore(da)[0].value);
      const a = f.querySelector("select[name=a]");
      if (a) {
        const o = opzioniConValore(a)[0] || [...a.options].find((x, i) => i > 0 && !x.disabled);
        if (o) await scegliValore(s, a, o.value);
      }
      const q = f.querySelector("input[name=qta]");
      if (q) await scrivi(s, q, "3");
      const n0 = s.scritture().length;
      await clicca(s, inviatore(f), 50);
      const p = s.scritture().slice(n0);
      ok(p.length === 1 && p[0].campi.some(([c, x]) => c === "da" && x.toLowerCase() === u10), `${v.pagina}: Sposta parte con da=u10`, JSON.stringify(p.map((x) => x.campi)));
    }
    chiudi(s);
  }
  const e16 = pagine({ caso: "e16", vista: "distinta", scrive: true });
  if (ok(e16.length > 0, "ci sono le pagine di e16")) {
    const v = e16[0];
    const s = await apri(v);
    const { d } = s;
    const mod = (u) => [...d.querySelectorAll("#ma form[data-ma-gesto]")].filter((g) => rottaDi(g).endsWith(`/componente/${u}/modifica`));
    const m11 = mod(u11).filter((g) => !gestoSpento(g));
    ok(m11.length > 0, `${v.pagina}: «Salva rev e descrizione» acceso per u11 (descrizione registrata, FE-08)`);
    ok(m11.every((f) => (f.querySelector("[name=rev]") || {}).value && (f.querySelector("[name=descrizione]") || {}).value), `${v.pagina}: precompilato con rev e descrizione registrate`);
    ok(mod(u12).every(gestoSpento), `${v.pagina}: per u12 (senza descrizione registrata) nessun «Salva rev e descrizione» acceso (MF07)`);
    const rip = [...d.querySelectorAll("#ma [data-ma-gesto]")].filter((g) => rottaDi(g).endsWith(`/componente/${u15}/ripristina`));
    ok(rip.length > 0 && rip.some((g) => !gestoSpento(g)), `${v.pagina}: «Ripristina» per u15 archiviato`);
    ok(![...d.querySelectorAll("#ma [data-ma-gesto]")].some((g) => rottaDi(g).endsWith(`/componente/${u15}/archivia`)), `${v.pagina}: nessun «Archivia» per u15`);
    const g = rip.find((x) => !gestoSpento(x));
    if (g) {
      const k = g.closest("[data-ma-pannello]") && g.closest("[data-ma-pannello]").getAttribute("data-ma-pannello");
      if (k) await scegliCasella(s, k, 50);
      const n0 = s.scritture().length;
      await clicca(s, g.tagName === "FORM" ? inviatore(g) : g, 50);
      const p = s.scritture().slice(n0);
      ok(p.length === 1 && p[0].url.toLowerCase() === `/thread/${s.tid}/fascicolo/componente/${u15}/ripristina`.toLowerCase(), `${v.pagina}: «Ripristina» manda POST …/componente/u15/ripristina`);
    }
    chiudi(s);
  }
});

// 21. privati: i file del kit, i casi ACME, le pagine rese; MA_NEGATI fuori dal repository
gruppoDi("privati", async () => {
  const negati = leggiNegati();
  note.add(negati.nota);
  provaPrivatiDelKit(negati);
  for (const v of CASI) {
    for (const f of [v.pagina, v.frammento].filter(Boolean)) {
      const contenuto = fs.readFileSync(path.join(OUT, f), "utf8");
      const x = controllaTesto(contenuto);
      ok(x.length === 0, `${f}: nessun dato da non pubblicare nella pagina resa`, x.join(", "));
      ok(uuidNonAcme(contenuto) === 0, `${f}: solo UUID ACME nella pagina resa`);
      const n = trovaNegati(contenuto, negati.nomi);
      ok(n.length === 0, `${f}: nessun nome dell'elenco MA_NEGATI`, n.join(", "));
    }
  }
});

// ------------------------------------------------------------------ l'esecuzione

(async () => {
  const solo = process.env.SOLO ? new RegExp(process.env.SOLO) : null;
  console.log(`prove.cjs: ${CASI.length} pagine da ${OUT}; motore_a.mjs di ${path.relative(path.resolve(COCKPIT, ".."), F_MJS) || F_MJS}`);
  for (const [nome, fn] of GRUPPI) {
    if (solo && !solo.test(nome)) continue;
    gruppo = nome;
    const prima = risultati.length;
    try {
      await fn();
    } catch (e) {
      ok(false, "eccezione nella prova", e && e.stack ? e.stack.split("\n").slice(0, 4).join(" | ") : String(e));
    }
    const mie = risultati.slice(prima);
    const ko = mie.filter((r) => !r.ok).length;
    console.log(`${ko ? "KO" : "ok"}  ${nome}  (${mie.length - ko}/${mie.length})`);
  }
  process.exit(scriviEsito() ? 1 : 0);
})();
