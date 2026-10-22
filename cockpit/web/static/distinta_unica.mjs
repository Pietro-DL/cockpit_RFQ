// distinta_unica.mjs — la pagina della Distinta unica di V1 (giro 5, A1d, V1; direttiva 23 §6; mockup v3,
// 07-frontend/mockup-v3; matrice dei gesti di V1 = MAT; emendamento di V1 = EM).
//
// Che cosa fa: presentazione, selezione, zoom, pan e interazione con la bozza (D23:280; RV3-05 A [U], DEC:84). Ogni
// regola sta nel server: blocchi, avvisi, stati delle associazioni, dipendenze dei file, identità e conferme li dicono
// le rotte di V1, e la pagina li mostra. Le sole cose che la pagina calcola sono di presentazione (impaginazione
// dell'organigramma, filtri, la pila d'annullamento) e lo stato della bozza mentre l'ingegnere la modifica: le azioni
// del vocabolario 2 applicate allo snapshot salvato, che il server riconcilia a ogni salvataggio (C3 §6.6; EM §9.2).
//
// Il solo modulo che chiama il server è il ponte (distinta_unica_ponte.mjs, D23:284): qui nessuna fetch, nessun htmx,
// nessun HTML del server. Il DOM si costruisce con createElement e textContent.
//
// Zone (mockup v3, LEGGIMI-v3 §1): testata, prodotti, passi «1 Sorgente» e «2 Revisione», viste «Organigramma» ed
// «Elenco», strisce, workspace (vista a sinistra, pannello del nodo a destra), barra fissa, finestre e wizard «Rivedi e
// conferma BOM». «Altri file» è una cartella della Sorgente, non un passo (D23:290).
//
// Nessun gesto legacy (MAT §7): né «Conferma l'albero», né i gesti strutturali immediati, né «Scarta il file». Le azioni
// che V1 non ha restano visibili e spente, con il motivo scritto accanto (MAT §6.1).

import * as P from "./distinta_unica_ponte.mjs";
// la guida interattiva e gli aiuti «(?)» (frontend del 22/10 [U]): solo presentazione, nessuna chiamata al server
import { avviaGuida } from "./du_guida.mjs";

// ---------------------------------------------------------------- utilità del DOM

const $ = (s, r) => (r || document).querySelector(s);
const SVGNS = "http://www.w3.org/2000/svg";
const RE_UUID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function h(tag, props) {
  const e = document.createElement(tag);
  if (props) {
    for (const k of Object.keys(props)) {
      const v = props[k];
      if (v === null || v === undefined || v === false) continue;
      if (k === "class") e.className = v;
      else if (k === "text") e.textContent = v;
      else if (k === "value") e.value = v;
      else if (k === "checked") e.checked = !!v;
      else if (k === "style") e.setAttribute("style", v);
      else if (k.slice(0, 2) === "on" && typeof v === "function") e.addEventListener(k.slice(2), v);
      else if (v === true) e.setAttribute(k, "");
      else e.setAttribute(k, String(v));
    }
  }
  for (let i = 2; i < arguments.length; i++) aggiungi(e, arguments[i]);
  return e;
}
function aggiungi(e, c) {
  if (c === null || c === undefined || c === false || c === "") return;
  if (Array.isArray(c)) {
    c.forEach((x) => aggiungi(e, x));
    return;
  }
  e.append(c instanceof Node ? c : document.createTextNode(String(c)));
}
function sv(tag, attrs) {
  const e = document.createElementNS(SVGNS, tag);
  for (const k of Object.keys(attrs || {})) e.setAttribute(k, attrs[k]);
  return e;
}
const clona = (o) => (o === undefined ? undefined : JSON.parse(JSON.stringify(o)));
const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
const sid = (k) => String(k).replace(/[^A-Za-z0-9_-]/g, "_");
const arr = (v) => (Array.isArray(v) ? v : []);
const ogg = (v) => v !== null && typeof v === "object" && !Array.isArray(v);
const breve = (s, n) => {
  s = String(s || "");
  return s.length > n ? s.slice(0, n) + "…" : s;
};
function ora(iso) {
  const d = iso ? new Date(iso) : new Date();
  if (isNaN(d.getTime())) return "";
  return d.toLocaleTimeString("it-IT", { hour: "2-digit", minute: "2-digit" });
}
function dataOra(iso) {
  const d = new Date(iso);
  if (!iso || isNaN(d.getTime())) return "";
  return d.toLocaleDateString("it-IT") + " " + d.toLocaleTimeString("it-IT", { hour: "2-digit", minute: "2-digit" });
}
function testoErrore(e) {
  return (e && e.message) || String(e);
}
function pill(tono, testo, titolo) {
  return h("span", { class: "du-st du-st--" + tono, title: titolo || null }, testo);
}
function btn(testo, fai, o) {
  o = o || {};
  const b = h("button", { type: "button", class: "du-btn" + (o.cls ? " " + o.cls : ""), id: o.id || null, disabled: !!o.no, title: o.no || o.t || null,
    "aria-pressed": o.pressed === undefined ? null : o.pressed ? "true" : "false", onclick: o.no ? null : fai }, testo);
  return o.no && o.perche !== false ? h("span", { class: "du-gesto" }, b, h("span", { class: "du-perche" }, o.no)) : b;
}

// «Aggiorna proposta…» e «Vedi le differenze» (R1 ?confronto=1) finché il server non calcola il confronto con la proposta di
// oggi (R1 confronto_disponibile false): spenti con il perché invece di offrirli e fallire (collaudo UI46, giro 3, F3-I1 [T])
const CONFRONTO_NON_V1 = "il confronto con la proposta di oggi non è ancora disponibile: arriva con l'adattatore del Motore A";
function perchéNoAggiorna() {
  return S.confrontoNo ? CONFRONTO_NON_V1 : perchéNoModifica();
}

// I motivi delle azioni che V1 non ha (MAT §6.1): restano visibili e spente, con il motivo scritto accanto (D23:294).
const NON_V1 = {
  documento: "I documenti si confermano con «Rivedi e conferma fascicolo», non da questa pagina",
  r10: "La correzione di un file non è in questa pagina: qui vale solo per lo STEP del prodotto",
  deroga: "Le deroghe non sono in questa pagina",
  sostituisci: "La sostituzione dei documenti non è in questa pagina",
  revisione: "Revisione e congelamento non sono in questa pagina",
  relazioni: "Le relazioni restano proposte: la loro ratifica arriva in una tappa dopo",
  candidata: "Il rifiuto di una candidata non è in questa pagina: intanto si sceglie un altro STEP",
  fascicolo: "I documenti si confermano con il lotto, dopo una BOM confermata e verificata: non ancora in questa pagina",
};

// ---------------------------------------------------------------- stato della pagina

const S = {
  thread: "",
  utente: "",
  prodotto: "", // componente_id del prodotto scelto
  passo: "", // "sorgente" | "revisione"
  vista: "org", // "org" | "elenco"
  zona: "vista", // sotto i 900 px: "vista" | "nodo"
  nodo: "", // la chiave del nodo scelto: chiave_bozza nella Revisione, voce del registro nella Sorgente
  modello: null,
  erroreModello: "",
  sorgente: null,
  erroreSorgente: "",
  smist: null, // S9, lo smistamento di tutti i prodotti (U2 §2.3)
  erroreSmist: "",
  esitiCarica: null,
  anteprima: null, // S6
  anteprimaAllegato: "",
  anteprimaGruppo: "",
  anteprimaPasso: "", // il passo da cui è stata chiesta l'anteprima (collaudo UI46, giro 2: dalla Revisione si mostra lì)
  erroreAnteprima: "",
  bozza: null, // R1
  erroreBozza: "",
  registri: {}, // impronta → contenuto del registro d'importazione
  storico: null, // R7
  erroreStorico: "",
  working: null, // la BOM confermata nella Revisione senza bozza (R7 &conferma=; messaggio 47): {chiave, stato, d, errore}
  salvato: null, // lo snapshot salvato (quello di R1 o dell'ultimo R2 riuscito)
  invio: null, // {voci, azioni, snapshot, corpo}: il salvataggio in volo o da rimandare
  coda: [], // voci non ancora inviate: {etichetta, inviare, locali}
  lavoro: null, // lo snapshot che si vede: salvato + invio + coda
  pila: [], // l'annullamento: {etichetta, voci, salvata}
  rifai: [],
  totaleEventi: 0,
  salvataggio: { stato: "pulita", testo: "", versione: 0, ora: "" },
  tentativi: 0,
  riepilogo: null, // R4 in sottofondo, senza gli avvisi del motore
  riepilogoStato: "",
  riepilogoVersione: 0, // la versione della bozza di cui è il riepilogo in sottofondo
  selezione: new Set(), // chiavi_bozza scelte per «Non includere»
  segnalatiAperto: false,
  compatte: false,
  camere: {},
  compressi: {},
  wizard: null,
  filtro: "tutti", // il filtro dell'Elenco (MAT EL-01)
  cerca: "", // la ricerca dell'Elenco (MAT EL-01)
  confronto: false, // «Confronto con la proposta» sulle schede (MAT BZ-25)
  // le modifiche non salvate tenute da parte dopo un conflitto o un rifiuto (MAT §5.1): {voci, perche, testo, prodotto,
  // codice, bozza_id, versione}. Ricordano il prodotto e la bozza su cui sono nate: si riapplicano solo lì (decisioni
  // dell'ondata A, §5 n. 2)
  daParte: null,
  daRileggere: "", // un gesto registrato dopo il quale la pagina non si è riletta (MAT §1.3, §5.2: «↻ Rileggi»)
  linguettaNodo: "dettaglio", // la linguetta del pannello del nodo: "dettaglio" | "documenti" (MAT PN-02)
  barraAltezza: null, // { larga, alta }: l'altezza più grande della barra in fondo a questa larghezza (renderBarraDentro)
  verifiche: new Set(), // le chiavi che la pagina sta verificando da sola con R6 (MAT §5.5 n. 2)
  avvisoDi: null, // il legame della striscia dell'esito: {legame: "anteprima" | "prodotto", prodotto} (avvisa)
  sezioni: {}, // le sezioni del pannello del nodo aperte o chiuse da chi guarda (messaggio 46: sezioni richiudibili)
};
let L = null; // la tela in vista: {lay, chiave, vp, mondo, g}

// ---------------------------------------------------------------- avvisi, strisce

// avvisa: la striscia dell'esito in testa. legame (messaggio 46 [U], difetti D22 e D24 [T]): «anteprima» per un testo che
// parla dell'anteprima di uno STEP (sparisce quando l'anteprima si chiude o si lascia la Sorgente), «prodotto» per un
// esito di un gesto sulla sorgente di un prodotto (sparisce quando si passa a un altro prodotto). Senza legame resta
// finché un altro esito non lo sostituisce.
function avvisa(esito, testo, legame) {
  const el = $("#du-avviso");
  if (!el) return;
  el.textContent = "";
  el.textContent = testo || "";
  el.setAttribute("data-esito", esito);
  el.hidden = !testo;
  // ogni striscia parla del prodotto in vista quando è nata: cambiando prodotto se ne va (collaudo UI46, I5)
  S.avvisoDi = testo ? { legame: legame || "prodotto", prodotto: S.prodotto } : null;
}
// avvisoScaduto: la striscia legata che non vale più per quello che si vede (render e cambio di prodotto)
function avvisoScaduto() {
  const el = $("#du-avviso");
  const a = S.avvisoDi;
  if (!el || el.hidden || !a) return;
  const via = (a.legame === "anteprima" && (!S.anteprima || (S.passo !== "sorgente" && !anteprimaInRevisione()))) || (a.prodotto && a.prodotto !== S.prodotto);
  if (via) {
    el.hidden = true;
    el.textContent = "";
    S.avvisoDi = null;
  }
}

// ---------------------------------------------------------------- indirizzo (la scelta di chi guarda, mai uno stato del server)

function leggiIndirizzo() {
  const q = new URLSearchParams(location.search);
  const p = q.get("prodotto") || "";
  S.prodotto = RE_UUID.test(p) ? p.toLowerCase() : "";
  const passo = q.get("passo");
  S.passo = passo === "revisione" || passo === "sorgente" ? passo : "";
  S.vista = q.get("vista") === "elenco" ? "elenco" : "org";
  S.nodo = q.get("nodo") || "";
}
function scriviIndirizzo() {
  const q = new URLSearchParams();
  if (S.prodotto) q.set("prodotto", S.prodotto);
  if (S.passo) q.set("passo", S.passo);
  q.set("vista", S.vista);
  if (S.nodo) q.set("nodo", S.nodo);
  try {
    history.replaceState(null, "", location.pathname + "?" + q.toString());
  } catch (e) {
    // niente: l'indirizzo è una comodità
  }
}

// ---------------------------------------------------------------- il modello della pagina (/motore-a/dati)

// modelloDa: i campi di /motore-a/dati che la pagina legge. In /dati «esito» è l'esito del thread (EsitoThread di
// valutazione: prodotti_valutati, cliente_id, fascicolo, associazioni, diagnostiche), non l'esito con thread[]: la
// lettura di thread[0] dell'ondata A lasciava vuoti prodotti, cliente e congelamento sul server vero (richiesta RQ-I1
// della corsia motore; DEC-B §6.3; prova OB-18). Del prodotto valutato: rif, componente_id, codice_richiesto, e il
// «calcolo di oggi» della verifica della BOM da bom.verificata, non da prodotto_verificato, che comprende smistamento e
// documenti (U2 §5.4). Le aggiunte di V1 (EM §9.9): gruppi[] (con importazione, diagnostiche, nodi per voce), fonte con
// date e autori, motivo_senza_gruppi, bozza_del_prodotto, f8_stato, conferma_corrente (U2 §5.3). È l'unica funzione da
// adeguare se il modello cambia nome a un campo; esportata per la prova OB-18 sul JSON vero di /dati.
export function modelloDa(j) {
  const th = j && ogg(j.esito) ? j.esito : {};
  const prodotti = arr(th.prodotti_valutati)
    .filter((p) => p && typeof p.componente_id === "string" && RE_UUID.test(p.componente_id))
    .map((p) => ({ rif: p.rif || "", id: p.componente_id.toLowerCase(), codice: p.codice_richiesto || "", descrizione: p.descrizione || "",
      verificatoOggi: !!(p.bom && p.bom.verificata === true), criterioOggi: (p.bom && p.bom.criterio) || "", motivi: arr(p.motivi), disegni: disegniDelProdotto(p),
      registrati: registratiSottoIlProdotto(p) }));
  return {
    errore: typeof j.errore === "string" ? j.errore : "",
    testa: ogg(j.testa) ? j.testa : {},
    prodotti,
    rif: typeof j.prodotto === "string" ? j.prodotto : "",
    clienteId: typeof th.cliente_id === "string" ? th.cliente_id.toLowerCase() : "",
    congelata: !!(th.fascicolo && th.fascicolo.congelato),
    // il contesto partner dichiarato e non applicato (U2 §4.7, U2-P49): la diagnostica contesto.non_valido del thread
    contestoNonValido: arr(th.diagnostiche).filter((d) => d && d.codice === "contesto.non_valido"),
    bozzaDelProdotto: ogg(j.bozza_del_prodotto) ? j.bozza_del_prodotto : null,
    gruppi: arr(j.gruppi),
    motivoSenzaGruppi: ogg(j.motivo_senza_gruppi) ? j.motivo_senza_gruppi : null,
    fonte: ogg(j.fonte) ? j.fonte : null,
    f8: typeof j.f8_stato === "string" ? j.f8_stato : "",
    associazioni: arr(th.associazioni),
    confermaCorrente: ogg(j.conferma_corrente) ? j.conferma_corrente : null,
    strutturaCondivisa: ogg(j.struttura_condivisa) ? j.struttura_condivisa : null,
    calcolato: j.calcolato || "",
    radiciStep: radiciLetteDegliStep(th),
  };
}
// radiciLetteDegliStep: il codice letto dalla radice di ogni STEP dell'esito (file[].documento.unita[], selettore
// radice_step.id, se no radice_step.nome), per allegato. Solo da mostrare: lo STEP la cui radice non è il prodotto dice
// quale radice ha (collaudo UI46, giro 3, F3-I4 [T]); nessuna regola, il confronto lo fa il motore.
function radiciLetteDegliStep(th) {
  const out = {};
  for (const f of arr(th && th.file)) {
    const aid = ogg(f) && typeof f.allegato_id === "string" ? f.allegato_id.toLowerCase() : "";
    const un = aid && ogg(f.documento) ? arr(f.documento.unita) : [];
    const di = (sel) => un.find((u) => ogg(u) && u.selettore === sel && typeof u.testo === "string" && u.testo.trim());
    const u = di("radice_step.id") || di("radice_step.nome");
    if (u) out[aid] = u.testo.trim();
  }
  return out;
}
// radiceLettaDiversa: il codice letto dalla radice dello STEP quando non è il codice del prodotto (maiuscole, spazi e
// separatori a parte), "" altrimenti o se il modello non lo dà
function radiceLettaDiversa(allegato, codice) {
  const r = S.modello && S.modello.radiciStep ? S.modello.radiciStep[String(allegato || "").toLowerCase()] || "" : "";
  const n = (x) => String(x || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
  return r && codice && n(r) !== n(codice) ? r : "";
}
// registratiSottoIlProdotto: i componenti registrati del perimetro del prodotto, la radice esclusa, che il motore espone
// fuori dalla BOM di lavoro (confermati_senza_step: senza uno STEP sono tutti). Serve solo a non offrire «Parti dalla BOM
// registrata» a un prodotto che non ne ha una (collaudo UI46, I6/D41 [T]).
function registratiSottoIlProdotto(p) {
  const radice = "componente:" + String(p.componente_id).toLowerCase();
  return arr(p.confermati_senza_step).filter((n) => ogg(n) && ogg(n.nodo) && typeof n.nodo.rif === "string" && n.nodo.rif.toLowerCase() !== radice).length;
}
// disegniDelProdotto: il 2D primario di ogni nodo del prodotto valutato, come lo sceglie il motore per l'organigramma
// della pagina di prima (valutazione.NodoBOM.disegni.primario, con l'anteprima: solo un PDF che si apre, LD-03), dalla
// BOM di lavoro (nodi), dalle strutture candidate e dai confermati senza STEP. Messaggio 46 [U]: le anteprime dei 2D nei
// nodi. La chiave è «r:<rif del nodo>» (lo stesso rif della casella della bozza: nodo:<sha>:<chiave>) e, per un nodo con
// un componente deciso, «c:<componente_id>». Solo presentazione: nessuna regola, il primario lo sceglie il server.
function disegniDelProdotto(p) {
  const out = {};
  const metti = (n) => {
    if (!ogg(n) || !ogg(n.nodo)) return;
    const pr = n.disegni && n.disegni.primario;
    const an = pr && pr.anteprima;
    if (!ogg(an) || an.disponibile !== true || typeof an.allegato_id !== "string" || !RE_UUID.test(an.allegato_id)) return;
    const x = { aid: an.allegato_id.toLowerCase(), nome: (pr && pr.nome_file) || "" };
    const rif = typeof n.nodo.rif === "string" ? n.nodo.rif : "";
    if (rif && !out["r:" + rif]) out["r:" + rif] = x;
    const cid = (ogg(n.nodo.decisione) && n.nodo.decisione.componente_id) || (rif.indexOf("componente:") === 0 ? rif.slice(11) : "");
    if (typeof cid === "string" && RE_UUID.test(cid) && !out["c:" + cid.toLowerCase()]) out["c:" + cid.toLowerCase()] = x;
  };
  arr(p && p.nodi).forEach(metti);
  arr(p && p.strutture_candidate).forEach((s) => arr(s && s.nodi).forEach(metti));
  arr(p && p.confermati_senza_step).forEach(metti);
  return out;
}
// nodoDelModello: i campi per nodo di adesso che /dati dà per la voce di un gruppo (EM §9.9; PE-10, PE-28; B-3):
// completezza {requisito, esito, deroga?}, diagnostiche[], decisione_esistente {componente_id, identita_registrata}. La
// voce è quella del registro (n:<allegato>:<rif>); null se il modello non la dà.
function nodoDelModello(voce) {
  if (!voce || !S.modello) return null;
  for (const g of S.modello.gruppi) for (const n of arr(g.nodi)) if (n && (n.voce === voce || n.rif === voce)) return n;
  return null;
}
function prodottoScelto() {
  return (S.modello && S.modello.prodotti.find((p) => p.id === S.prodotto)) || null;
}
function codiceProdotto() {
  const p = prodottoScelto();
  return p ? p.codice || p.rif : "";
}
function emittenteCliente() {
  const r = S.lavoro && arr(S.lavoro.nodi).find((n) => n.componente_id === S.prodotto);
  if (r && r.identita && typeof r.identita.emittente === "string" && r.identita.emittente.indexOf("cliente:") === 0) return r.identita.emittente;
  return S.modello && S.modello.clienteId ? "cliente:" + S.modello.clienteId : "";
}

// ---------------------------------------------------------------- letture

async function caricaModello() {
  try {
    let j = await P.leggiModello("");
    S.modello = modelloDa(j);
    if (!S.modello.errore) {
      if (!S.prodotto || !prodottoScelto()) S.prodotto = S.modello.prodotti.length ? S.modello.prodotti[0].id : "";
      const p = prodottoScelto();
      if (p && S.modello.rif && p.rif !== S.modello.rif) {
        j = await P.leggiModello(p.rif);
        S.modello = modelloDa(j);
      }
    }
    S.erroreModello = S.modello.errore;
  } catch (e) {
    S.modello = null;
    S.erroreModello = testoErrore(e);
  }
}
let pollSorgente = null;
let pollFine = 0;
async function caricaSorgente() {
  try {
    const prima = S.sorgente;
    S.sorgente = await P.leggiSorgente({ scartati: true });
    S.erroreSorgente = "";
    // le letture del Motore A arrivate intanto (U2 §2.3: S9 si rilegge quando cambia la firma di S1, quando finisce
    // un'analisi): un messaggio del sistema, non una mossa dell'utente (K-1 del mockup v5)
    if (prima && prima.firma && S.sorgente.firma && prima.firma !== S.sorgente.firma) {
      const letti = arr(S.sorgente.file).filter((f) => letto(f) && arr(prima.file).some((g) => g.allegato_id === f.allegato_id && g.analisi && IN_ANALISI.includes(g.analisi.stato)));
      if (letti.length) sistema(letti.length === 1 ? letti[0].nome_file + " letto dal Motore A" : letti.length + " file letti dal Motore A", letti[0].allegato_id);
      SG.firmaS1 = S.sorgente.firma;
      if (S.smist || S.erroreSmist) caricaSmistamentoPoi();
    }
  } catch (e) {
    S.erroreSorgente = testoErrore(e);
  }
  programmaPoll();
}
// S1 si rilegge ogni 5 s finché un file è in coda o in analisi, una rimozione aspetta o una copia è in corso, per al più
// 10 minuti; poi «↻ Aggiorna» (MAT §1.5; A4-Q15 A).
function inLavorazione() {
  return arr(S.sorgente && S.sorgente.file).some((f) => {
    const a = f.analisi && f.analisi.stato;
    const rim = f.rimozione && f.rimozione.stato;
    const con = f.conservazione && f.conservazione.stato;
    return ["download_in_coda", "in_staging", "estrazione_in_coda", "in_analisi"].includes(a) || rim === "in_attesa" || con === "in_copia";
  });
}
function programmaPoll() {
  clearTimeout(pollSorgente);
  if (!inLavorazione()) {
    pollFine = 0;
    return;
  }
  if (!pollFine) pollFine = Date.now() + 10 * 60 * 1000;
  if (Date.now() > pollFine) return;
  pollSorgente = setTimeout(async () => {
    // UI46 [U] (controllo di stabilità del 21/10, S1 bloccante): la rilettura ogni 5 s ridisegnava sempre tutto
    // #du-corpo (l'elenco tornava in cima, la ricerca perdeva i tasti). Come la rilettura del wizard (firmaVistaWizard)
    // [T]: si ridisegna solo se cambia quello che la Sorgente mostra (la firma di S1 o l'errore di lettura).
    const prima = firmaVistaS1();
    await caricaSorgente();
    if (S.passo === "sorgente" && firmaVistaS1() !== prima) renderCorpo();
  }, 5000);
}
// firmaVistaS1: quello di S1 che la Sorgente mostra. La firma del server cambia se e solo se cambia un campo mostrato
// (U2 §2.3, firmaDellaSorgente); senza firma conta la risposta intera, senza l'ora del calcolo.
function firmaVistaS1() {
  const s = S.sorgente;
  return JSON.stringify([S.erroreSorgente || "", s ? s.firma || Object.assign({}, s, { calcolato: "" }) : null]);
}
// caricaBozza: R1. Con modifiche non salvate (in coda o in volo) R1 non si rilegge: impostaBozza le butterebbe, e MAT
// §3.9 e §5.1 vogliono che non si perdano finché la pagina è aperta. Chi deve rileggere con modifiche aperte le mette
// prima da parte (rileggiBozzaSalvata). Una lettura non riuscita non è «nessuna bozza» (MAT §5.1, §5.4): erroreBozza
// spegne la tela, i gesti e i pulsanti di creazione, e dice «↻ Aggiorna». Torna true se R1 è stato riletto.
async function caricaBozza() {
  if (S.coda.length || S.invio) return false;
  if (!S.prodotto) {
    impostaBozza(null);
    S.erroreBozza = "";
    return true;
  }
  try {
    impostaBozza(await P.leggiBozza(S.prodotto, { eventi: true, importazione: true }));
    S.erroreBozza = "";
    return true;
  } catch (e) {
    impostaBozza(null);
    S.erroreBozza = testoErrore(e);
    return false;
  }
}
async function caricaStorico() {
  if (!S.prodotto) {
    S.storico = null;
    return;
  }
  try {
    S.storico = await P.storicoConferme(S.prodotto);
    S.erroreStorico = "";
  } catch (e) {
    S.storico = null;
    S.erroreStorico = testoErrore(e);
  }
}
async function leggiTutto(prima) {
  const passoChiesto = S.passo;
  await caricaModello();
  await Promise.all([caricaSorgente(), caricaBozza(), caricaStorico(), caricaSmistamento()]);
  if (prima && !passoChiesto) S.passo = bozzaAperta() ? "revisione" : "sorgente";
  if (!S.passo) S.passo = "sorgente";
  render();
}
// aggiorna (MAT TS-03): rilegge il modello e, sul passo aperto, R1 o S1. Il guscio resta, la bozza non si perde: con
// modifiche non salvate R1 non si rilegge.
async function aggiorna() {
  avvisa("ok", "Rilettura in corso…");
  await caricaModello();
  await Promise.all([caricaSorgente(), caricaStorico(), caricaBozza(), caricaSmistamento()]);
  if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
  const non = letturaNonRiuscita();
  if (!non) S.daRileggere = "";
  sistema(non ? "↻ Aggiorna: lettura non riuscita (" + non + ")" : "↻ Aggiorna: pagina riletta alle " + ora());
  avvisa(non ? "errore" : "ok", non ? "Lettura non riuscita (" + non + "): ↻ Aggiorna per riprovare." : "Pagina riletta alle " + ora() + ".");
  render();
}
// letturaNonRiuscita: quali letture dell'ultima rilettura non sono riuscite, o "" (MAT §5.4, «letture: 5xx»).
function letturaNonRiuscita() {
  const out = [];
  if (!S.modello || S.erroreModello) out.push("modello");
  if (S.erroreBozza) out.push("bozza");
  if (S.erroreSorgente) out.push("file");
  if (S.erroreStorico) out.push("conferme");
  return out.join(", ");
}
// rileggiDopoUnGesto: dopo un «ok» si rileggono il modello e le letture toccate; se la rilettura non riesce, lo stato è
// «da rileggere», mai «non riuscito» (MAT §1.3, §5.2): la striscia dice che il gesto è registrato e offre «↻ Rileggi».
async function rileggiDopoUnGesto(cosa, testoRegistrato) {
  await caricaModello();
  const lavori = [];
  if (cosa.includes("sorgente")) lavori.push(caricaSorgente());
  if (cosa.includes("bozza")) lavori.push(caricaBozza());
  if (cosa.includes("storico")) lavori.push(caricaStorico());
  // lo smistamento (S9) cambia con i gesti dei file e della bozza (U2 §2.3)
  if (cosa.includes("sorgente") || cosa.includes("bozza")) lavori.push(caricaSmistamento());
  await Promise.all(lavori);
  const non = letturaNonRiuscita();
  S.daRileggere = non ? testoRegistrato + " La pagina non si è riletta (" + non + "): ↻ Rileggi." : "";
  return !non;
}
async function rileggiTutto() {
  await aggiorna();
}

// ---------------------------------------------------------------- la bozza: snapshot, azioni, pila

function bozzaAperta() {
  return !!(S.bozza && S.bozza.stato === "aperta");
}
// bozzaDaConvertire: R1 dice che la bozza aperta va convertita (EM §9.1): la pagina non confronta numeri di formato.
function bozzaDaConvertire() {
  return bozzaAperta() && !!(S.bozza.conversione && S.bozza.conversione.necessaria);
}
function bozzaLavorabile() {
  return bozzaAperta() && !bozzaDaConvertire() && ogg(S.salvato);
}
function normalizza(s) {
  const o = clona(s) || {};
  o.nodi = arr(o.nodi);
  o.archi = arr(o.archi);
  o.decisioni = arr(o.decisioni);
  o.documenti_preassociati = arr(o.documenti_preassociati);
  return o;
}
function numeroEventi(j) {
  if (Array.isArray(j.eventi)) return j.eventi.reduce((m, e) => Math.max(m, Number(e.n) || 0), 0);
  return typeof j.eventi === "number" ? j.eventi : 0;
}
function impostaBozza(j) {
  S.bozza = j;
  // il server dice se calcola il confronto con la proposta di oggi (R1 confronto_disponibile; collaudo UI46, giro 3, F3-I1)
  if (j && j.confronto_disponibile === false) S.confrontoNo = true;
  S.coda = [];
  S.invio = null;
  S.rifai = [];
  S.riepilogo = null;
  S.riepilogoStato = "";
  S.riepilogoVersione = 0;
  S.tentativi = 0;
  S.selezione.clear();
  if (j && j.stato === "aperta" && ogg(j.snapshot) && !(j.conversione && j.conversione.necessaria)) {
    S.salvato = normalizza(j.snapshot);
    S.totaleEventi = numeroEventi(j);
    S.pila = pilaDalGiornale(arr(j.eventi));
    const imp = j.importazione;
    if (imp && typeof imp.impronta === "string" && ogg(imp.contenuto)) S.registri[imp.impronta] = imp.contenuto;
    S.salvataggio = { stato: "salvata", testo: "", versione: j.versione, ora: ora(j.aggiornata_il) };
    // R4 non parte all'apertura: in sottofondo solo dopo un R2 riuscito, e con gli avvisi del motore all'apertura del
    // wizard (MAT §1.5; EM §9.4, «Uso dalla pagina»)
  } else {
    S.salvato = null;
    S.pila = [];
    S.salvataggio = { stato: "pulita", testo: "", versione: 0, ora: "" };
  }
  ricalcola();
}

// Le azioni del server: dopo l'ultima di queste la pila non torna indietro (EM §9.2, OV-06).
const AZIONI_SERVER = ["bozza_aperta", "sorgente_scelta", "sorgente_cambiata", "proposta_riallineata"];

// eventoAzione: un evento del giornale (R1 ?eventi=1) come azione del vocabolario 2.
function eventoAzione(e) {
  return { azione: e.azione, chiave_bozza: e.chiave === undefined ? null : e.chiave, prima: e.prima === undefined ? null : e.prima, dopo: e.dopo === undefined ? null : e.dopo,
    origine: e.origine || "utente" };
}
// pilaDalGiornale: la pila ricostruita dal giornale dopo una rilettura (MAT BZ-18): gli eventi dell'ingegnere dopo
// l'ultima versione costruita dal server, un passo per salvataggio, salvo il gesto cumulativo «Non includere», un passo
// per azione_id (MAT NI-08). Gli eventi annullati e non rifatti non tornano nella pila.
function pilaDalGiornale(eventi) {
  const ev = eventi.filter((e) => ogg(e) && typeof e.azione === "string").sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  let da = 0;
  ev.forEach((e, i) => {
    if (AZIONI_SERVER.includes(e.azione) || e.origine === "server") da = i + 1;
  });
  const dopo = ev.slice(da);
  const annullati = new Set();
  for (const e of dopo) {
    const n = e.dopo && Number(e.dopo.n);
    if (e.azione === "annulla" && n) annullati.add(n);
    if (e.azione === "rifai" && n) annullati.delete(n);
  }
  const passi = [];
  const perChiave = new Map();
  for (const e of dopo) {
    // il riconoscimento di una PB legacy non si annulla (R39-15: per tornare indietro la PB si toglie): non è un passo
    if (e.azione === "annulla" || e.azione === "rifai" || e.azione === "file_riconosciuto" || annullati.has(Number(e.n))) continue;
    const az = eventoAzione(e);
    const gruppo = (e.dopo && e.dopo.azione_id) || (e.prima && e.prima.azione_id) || "v" + e.versione;
    let passo = perChiave.get(gruppo);
    if (!passo) {
      passo = { etichetta: etichettaEvento(e), voci: [{ etichetta: etichettaEvento(e), inviare: [], locali: [], numeri: [] }], salvata: true };
      perChiave.set(gruppo, passo);
      passi.push(passo);
    }
    passo.voci[0].inviare.push(az);
    passo.voci[0].locali.push(az);
    passo.voci[0].numeri.push(Number(e.n));
  }
  return passi;
}
function etichettaEvento(e) {
  const nomi = { nodo_aggiunto: "pezzo aggiunto", nodo_tolto: "pezzo tolto", nodo_modificato: "pezzo modificato", arco_aggiunto: "legame aggiunto",
    arco_tolto: "legame tolto", arco_spostato: "pezzo spostato", qta_modificata: "quantità cambiata", decisione_aggiunta: "decisione", decisione_tolta: "decisione tolta",
    file_preassociato: "file preassociato", file_svincolato: "file svincolato", file_lasciato: "file lasciato senza associazione", file_riconosciuto: "preassociazione legacy riconosciuta",
    rebase: "rebase" };
  return (nomi[e.azione] || e.azione) + (e.versione ? " (versione " + e.versione + ")" : "");
}

// applica: un'azione del vocabolario 2 sullo snapshot (C3 §6.6; EM §9.2). È la stessa regola che il server rifà a ogni
// salvataggio per riconciliare (DB/azioni.go): se le due regole divergono, R2 risponde 422 azioni_non_riconciliate.
export function applica(snap, a) {
  const s = snap;
  const d = a.dopo;
  const p = a.prima;
  const togliId = (o) => {
    if (!ogg(o)) return o;
    const c = clona(o);
    delete c.azione_id;
    return c;
  };
  switch (a.azione) {
    case "nodo_aggiunto":
      s.nodi = s.nodi.filter((n) => n.chiave_bozza !== d.chiave_bozza).concat([togliId(d)]);
      break;
    case "nodo_tolto": {
      const k = a.chiave_bozza || (p && p.chiave_bozza);
      s.nodi = s.nodi.filter((n) => n.chiave_bozza !== k);
      break;
    }
    case "nodo_modificato": {
      const n = s.nodi.find((x) => x.chiave_bozza === a.chiave_bozza);
      if (n && ogg(d)) for (const k of Object.keys(d)) {
        if (d[k] === null || d[k] === undefined) delete n[k];
        else n[k] = clona(d[k]);
      }
      break;
    }
    case "arco_aggiunto":
      s.archi = s.archi.filter((x) => !(x.padre === d.padre && x.figlio === d.figlio)).concat([togliId(d)]);
      break;
    case "arco_tolto":
      s.archi = s.archi.filter((x) => !(x.padre === p.padre && x.figlio === p.figlio));
      break;
    case "arco_spostato":
      s.archi = s.archi.filter((x) => !(x.padre === p.padre && x.figlio === p.figlio)).concat([togliId(d)]);
      break;
    case "qta_modificata": {
      const x = s.archi.find((y) => y.padre === d.padre && y.figlio === d.figlio);
      if (x) x.qta = d.qta;
      break;
    }
    case "decisione_aggiunta":
      s.decisioni = s.decisioni.filter((x) => x.id !== d.id).concat([clona(d)]);
      break;
    case "decisione_tolta":
      s.decisioni = s.decisioni.filter((x) => x.id !== p.id);
      break;
    // la PB del formato 3 è {allegato_id, chiave_bozza} (direttiva 35, C-2). La regola è quella del server (applica2.go):
    // un file_preassociato di un allegato già preassociato non sostituisce (il server lo rifiuta: per spostare si mandano
    // file_svincolato e file_preassociato, azioniPreassocia); file_svincolato toglie la PB solo se è sul nodo che dice
    case "file_preassociato":
      // il registro dichiarato di un suggerimento accettato sta solo nella copia locale dell'azione (preassocia)
      if (!s.documenti_preassociati.some((x) => x.allegato_id === d.allegato_id)) s.documenti_preassociati = s.documenti_preassociati.concat([typeof d.importazione === "string" && d.importazione ? { allegato_id: d.allegato_id, chiave_bozza: d.chiave_bozza, importazione: d.importazione } : { allegato_id: d.allegato_id, chiave_bozza: d.chiave_bozza }]);
      break;
    case "file_svincolato":
      s.documenti_preassociati = s.documenti_preassociati.filter((x) => !(x.allegato_id === p.allegato_id && x.chiave_bozza === p.chiave_bozza));
      break;
    case "rebase":
      if (d && ogg(d.base)) {
        s.versione_bom_base = { conferma_id: d.base.conferma_id === undefined ? null : d.base.conferma_id, numero: d.base.numero, contatore: d.base.contatore };
        if (typeof d.base.firma_struttura === "string") s.firma_base = d.base.firma_struttura;
      }
      break;
    default:
      // file_lasciato non cambia lo snapshot (EM §3.7); annulla e rifai si applicano con le loro azioni locali
      break;
  }
  return s;
}
const INVERSI = { nodo_aggiunto: "nodo_tolto", nodo_tolto: "nodo_aggiunto", arco_aggiunto: "arco_tolto", arco_tolto: "arco_aggiunto",
  decisione_aggiunta: "decisione_tolta", decisione_tolta: "decisione_aggiunta", file_preassociato: "file_svincolato", file_svincolato: "file_preassociato" };
// inverso: l'azione che disfa a (per l'annullamento in locale; al server va «annulla{n}»).
function inverso(a) {
  if (INVERSI[a.azione]) {
    const aggiunge = /aggiunt|preassociato/.test(a.azione);
    const o = aggiunge ? a.dopo : a.prima;
    return aggiunge ? { azione: INVERSI[a.azione], chiave_bozza: a.chiave_bozza, prima: o, dopo: null } : { azione: INVERSI[a.azione], chiave_bozza: a.chiave_bozza, prima: null, dopo: o };
  }
  if (["nodo_modificato", "arco_spostato", "qta_modificata"].includes(a.azione)) return { azione: a.azione, chiave_bozza: a.chiave_bozza, prima: a.dopo, dopo: a.prima };
  return { azione: "nessuna", chiave_bozza: null, prima: null, dopo: null };
}
function applicaVoci(base, voci) {
  const s = normalizza(base);
  for (const v of voci) for (const a of v.locali) applica(s, a);
  return s;
}
function ricalcola() {
  S.lavoro = S.salvato ? applicaVoci(S.salvato, (S.invio ? S.invio.voci : []).concat(S.coda)) : null;
}
let contatoreVoci = 0;
function nuovaChiaveNodo() {
  contatoreVoci++;
  return "k-u" + Date.now().toString(36) + "-" + contatoreVoci.toString(36) + Math.random().toString(36).slice(2, 6);
}
function nuovoIdDecisione(pref) {
  contatoreVoci++;
  return pref + "-" + Date.now().toString(36) + contatoreVoci.toString(36);
}
// chiaveEvento: la chiave_bozza dell'evento di una decisione (decisione_aggiunta, decisione_tolta) è quella della
// decisione, se il suo tipo ha il campo (identita, d1_*, senza_padre, riusa_*, da_valutare); altrimenti null (EM §9.2,
// tabella delle forme: il server la deduce dalla decisione, come nel vocabolario 1, e una chiave dichiarata diversa da
// quella dedotta non si riconcilia). L'ondata A mandava la chiave del nodo anche per escludi, scollega_documento,
// togli_arco, non_includere. null è la regola del vocabolario 1 e la raccomandata di RPC PD-08 (opzione a); se si decide
// l'opzione b (la chiave del nodo), cambia solo questa funzione.
function chiaveEvento(dec) {
  if (dec && typeof dec.chiave_bozza === "string" && dec.chiave_bozza) return dec.chiave_bozza;
  // conferma_categoria: il nodo sta nel soggetto (U2 §6.4.2)
  if (dec && ogg(dec.soggetto) && typeof dec.soggetto.chiave_bozza === "string" && dec.soggetto.chiave_bozza) return dec.soggetto.chiave_bozza;
  return null;
}

// Una conferma in sospeso per il prodotto (R5 con l'esito incerto): finché c'è, la bozza non si modifica, salvo
// l'abbandono esplicito della chiave (MAT §5.2, «verificato: non registrata»; §5.5 n. 7). Un tentativo «certo» (409
// COPIA_IN_CORSO: niente registrato, niente in volo) non ferma le modifiche: il ponte ammette un corpo diverso con una
// chiave nuova, e «Riprova» con lo stesso corpo riusa la chiave (MAT §5.2, «copia in corso»).
function confermaInSospeso() {
  return P.sospesi().find((s) => s.nome === "confermaBom" && s.risorsa === "bozza:" + S.prodotto) || null;
}
const D18 = "il modello della pagina non si legge: niente tela, bozza né wizard (D-18)";
const BOZZA_NON_LETTA = "la bozza non si legge: ↻ Aggiorna";
const CONGELATA = "la BOM è congelata: revisione e congelamento non sono in questa pagina";
// i testi dei 422 di R2 che la matrice o il contratto nominano per motivo (MAT §5.4, riga R2): gli altri hanno il testo
// del server. natura_non_preassociabile e svincolo_senza_associazione: direttiva 35, C-2 (provvisorio, base [T] D-2 (a)
// e D-4 A)
const TESTI_R2_MOTIVO = {
  annullamento_non_ammesso: "Questo passo è prima di un aggiornamento fatto dal server: per tornare indietro usa «Includi di nuovo» o «Ripristina».",
  natura_non_preassociabile: "Un file di queste modifiche non si preassocia: solo file ed elementi di Outlook, esclusi STEP e archivi (ZIP, 7z, RAR). Togli quella modifica (Annulla) e salva di nuovo.",
  svincolo_senza_associazione: "Lo scollegamento cita un file che non ha un'associazione registrata attiva con quel componente: togli quella modifica (Annulla) e rileggi la pagina.",
  // G39-01 (distintabozza/provenienza3.go, rilievo 7 della revisione G39 parte A): il suggerimento accettato è superato,
  // il calcolo del motore è cambiato dopo la lettura. «Rileggi la bozza salvata» rilegge anche i suggerimenti (R1 con
  // ?suggerimenti_motore=1) e tiene da parte tutte le modifiche, senza i registri dichiarati; «Riapplica» li ricalcola
  // con i suggerimenti riletti (rilievo importante della revisione indipendente 3 della corsia interfaccia)
  proposta_non_corrispondente: "Un suggerimento del motore accettato in queste modifiche non vale più: il calcolo del motore è cambiato dopo la lettura. «Rileggi la bozza salvata» rilegge la bozza e i suggerimenti e tiene da parte tutte le modifiche: riapplicandole, la pagina le manda con il calcolo di adesso, e il server dice se ogni file è ancora una proposta del motore.",
};
// i testi dei 422 di R2 per codice (giro G39): STEP_CANDIDATO_DEL_FINITO (G39-02 [U], distintabozza/stepcandidati3.go) è la
// PB nuova o spostata su uno STEP che il Motore A riconosce come candidato sorgente di un prodotto finito, anche in una
// bozza manuale: quello STEP si conferma con S8, non si preassocia
const TESTI_R2_CODICE = {
  STEP_CANDIDATO_DEL_FINITO: "Uno STEP di queste modifiche è, per il Motore A, lo STEP strutturale di un prodotto finito: non si preassocia come documento di un pezzo, si conferma come sorgente dalla Sorgente («Conferma questo STEP come sorgente»). Togli quella modifica (Annulla) e salva di nuovo.",
};
// perchéNoModifica: perché un gesto sulla bozza non parte, o "". Con o.riprova una conferma in sospeso già verificata
// «non registrata» non ferma il wizard: è la strada di «Riprova la conferma», lo stesso corpo con la stessa chiave, anche
// dopo una ricarica della pagina (MAT §5.2, «verificato: non registrata»; §5.5 n. 4). Le modifiche restano ferme. Con
// o.daParte le modifiche tenute da parte non fermano: è la strada di «Riapplica», che le decide.
function perchéNoModifica(o) {
  if (!S.modello || S.erroreModello) return D18;
  if (S.erroreBozza) return BOZZA_NON_LETTA;
  if (!bozzaAperta()) return "nessuna bozza: la BOM di lavoro nasce dalla scelta dello STEP nella Sorgente";
  if (bozzaDaConvertire()) return "la bozza è nel formato di prima: per lavorarla va convertita";
  if (S.modello.congelata) return CONGELATA;
  if (S.salvataggio.stato === "conflitto") return "la bozza è cambiata altrove: rileggi la bozza salvata";
  if (S.salvataggio.stato === "rifiutata") return "il server ha rifiutato l'ultimo salvataggio: rileggi la bozza salvata";
  if (S.daParte && !(o && o.daParte)) return "prima decidi le modifiche tenute da parte: riapplicale o scartale";
  const sosp = confermaInSospeso();
  if (sosp && !sosp.certo && !(o && o.riprova && sosp.verificato)) return sosp.verificato ? "una conferma è in sospeso (non ancora registrata): riprovala con lo stesso corpo, o abbandona il tentativo" : "una conferma è in sospeso: verificala, o abbandona il tentativo";
  if (S.wizard && S.wizard.stato === "conferma") return "conferma in corso";
  return "";
}
// perchéNoFormaBozza: perché «Abbandona la bozza…» (R3) o «Converti…» non partono, o "". Valgono anche per una bozza
// del formato di prima (MAT §5.1: «solo conversione o abbandono»); come ogni modifica della bozza si fermano con una
// conferma in sospeso, salvo l'abbandono esplicito della chiave (MAT §5.2, §5.5 n. 7), con la BOM congelata e con un
// salvataggio in volo, che deve prima avere un esito (MAT §1.3).
function perchéNoFormaBozza() {
  if (!S.modello || S.erroreModello) return D18;
  if (S.erroreBozza) return BOZZA_NON_LETTA;
  if (!bozzaAperta()) return "nessuna bozza aperta";
  if (S.modello.congelata) return CONGELATA;
  const sosp = confermaInSospeso();
  if (sosp && !sosp.certo) return "una conferma è in sospeso: verificala, o abbandona prima il tentativo";
  if (S.wizard && S.wizard.stato === "conferma") return "conferma in corso";
  if (S.invio) return "un salvataggio è in corso: aspetta il suo esito";
  return "";
}
// ultimaConferma: l'ultima conferma dello storico (R7), o null.
function ultimaConferma() {
  return arr(S.storico && S.storico.conferme).find((c) => c.ultima) || null;
}
// storicoNonLetto: lo storico delle conferme (R7) non si è letto. Allora la pagina non sa se il prodotto ha una conferma
// né di che formato: le regole che ne dipendono vanno nel verso sicuro, cioè spengono con STORICO_NON_LETTO (le due
// partenze, PE-B03, e la regola del giornale di WZ-01, nessunaModifica), e niente dice «nessuna conferma» (MAT §5.4).
function storicoNonLetto() {
  return !!S.erroreStorico || !S.storico;
}
const STORICO_NON_LETTO = "lo storico delle conferme non si legge: ↻ Aggiorna";
// confermaFormato3: la conferma è del formato 3. Lo dice R7 (EM §9.7: «formato» di ogni conferma), la pagina non lo
// calcola. Serve alle regole di PE-23 (decisioni dell'ondata A), che distinguono una conferma di base formato 3 da una
// formato 1 o 2; non è il formato della bozza, che dice R1 (MAT §1.3).
function confermaFormato3(c) {
  return !!(c && c.formato === 3);
}
// haConfermaFormato3: il prodotto ha una conferma formato 3 (R7). «Parti dalla BOM registrata» allora non si offre:
// si parte da quella conferma, con «Modifica la BOM confermata» (PE-23; EM §6.12, «la partenza registrato: working senza
// una conferma formato 3»).
function haConfermaFormato3() {
  return arr(S.storico && S.storico.conferme).some(confermaFormato3);
}
const PARTENZA_REGISTRATO_NO = "il prodotto ha già una BOM confermata: si parte da quella, con «Modifica la BOM confermata»";
// nessunaModifica: la bozza è la conferma di base come l'ha aperta il server e dopo non è cambiato niente; allora «Rivedi
// e conferma BOM» e «Conferma BOM» sono spenti con il motivo, perché una conferma n+1 uguale alla n sarebbe un atto in
// più che non si toglie (MAT WZ-01; decisioni dell'ondata A, PE-23). r4: il riepilogo da guardare (quello del wizard);
// senza, quello in sottofondo, se è della versione di adesso.
//   - Il segno del server vale per primo: R4 dà uguale_alla_base, vero quando lo snapshot canonico della bozza è uguale a
//     quello della conferma di base formato 3 (e R5 allora rifiuta con NESSUNA_MODIFICA).
//   - Finché R4 non lo dà (passo 6), la pagina decide dal giornale (R1 ?eventi=1): l'ultimo evento è bozza_aperta con la
//     partenza «conferma» (da-conferma-1) o «registrato», nessun gesto in coda né in volo, e la base è l'ultima conferma
//     valida, formato 3. La prima conferma formato 3 di un prodotto resta ammessa anche se è uguale a una conferma formato
//     1 o 2, perché aggiunge identità e caselle. Quando il giornale non basta (un annullamento netto), la pagina non
//     spegne niente: decide R4.
//   - Con la stessa bozza senza gesti e lo storico (R7) che non si legge, la pagina non sa di che formato è la base: nel
//     dubbio spegne, con «↻ Aggiorna», come le due partenze (PE-B03). L'unica eccezione è R4 che dà uguale_alla_base
//     falso, perché il segno del server vale per primo. Per la partenza «registrato» il giornale resta l'unica guardia
//     anche dopo il passo 6 se PD-01 non sceglie il punto 3 (a): R5 ferma NESSUNA_MODIFICA solo quando R4 dice «uguale».
function nessunaModifica(r4) {
  if (!bozzaLavorabile() || S.coda.length || S.invio) return "";
  const base = S.salvato.versione_bom_base;
  const testo = "nessuna modifica: la v" + (base && base.numero ? base.numero : "?") + " è già confermata";
  const r = r4 !== undefined ? r4 : S.riepilogoStato === "aggiornato" && S.riepilogoVersione === S.bozza.versione ? S.riepilogo : null;
  if (r && typeof r.uguale_alla_base === "boolean") return r.uguale_alla_base ? testo : "";
  if (!Array.isArray(S.bozza.eventi)) return "";
  const ev = S.bozza.eventi.filter((e) => ogg(e) && typeof e.azione === "string").sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  if (!ev.length) return "";
  const ultimo = ev[ev.length - 1];
  if (ultimo.azione !== "bozza_aperta" || !ogg(ultimo.dopo) || !["conferma", "registrato"].includes(ultimo.dopo.partenza)) return "";
  if (storicoNonLetto()) return STORICO_NON_LETTO;
  const u = ultimaConferma();
  if (!u || !u.valida || !confermaFormato3(u) || !base || Number(base.numero) !== Number(u.numero)) return "";
  return testo;
}
function puoModificare() {
  return !perchéNoModifica();
}

// registroDelFile: il registro d'importazione con cui il Motore A propone il file, con una sola destinazione, per il
// prodotto della pagina (G39-01 [U], 20/10 sera; D-PB01 A, D-PB03 A [U]): prima i suggerimenti del calcolo di adesso (S9 e
// R1 preassociazioni_proposte[].importazione, sempre univoci: ClassificaProposte), poi il registro della versione della
// bozza (associazioni_proposte delle sue voci: un solo nodo, ProposteDelRegistro). "" senza una proposta univoca: una
// proposta ambigua non è un suggerimento.
const RE_IMPRONTA_REG = /^[0-9a-f]{64}$/;
function registroDelFile(aid) {
  const a = String(aid || "").toLowerCase();
  if (!a) return "";
  const p = s9P(S.prodotto);
  for (const x of arr(p && p.preassociazioni_proposte).concat(arr(S.bozza && S.bozza.preassociazioni_proposte))) {
    if (ogg(x) && String(x.allegato_id || "").toLowerCase() === a && typeof x.importazione === "string" && RE_IMPRONTA_REG.test(x.importazione)) return x.importazione;
  }
  const reg = registroBozza();
  const imp = S.bozza && S.bozza.importazione && S.bozza.importazione.impronta;
  if (!reg || typeof imp !== "string" || !RE_IMPRONTA_REG.test(imp)) return "";
  const rif = new Set();
  for (const v of arr(reg.proposta && reg.proposta.nodi)) {
    if (ogg(v) && arr(v.associazioni_proposte).some((y) => ogg(y) && String(y.allegato_id || "").toLowerCase() === a)) rif.add(String((v.origine && v.origine.rif) || ""));
  }
  return rif.size === 1 ? imp : "";
}
// conRegistriDelMotore: la copia locale delle azioni di un gesto (lo snapshot che si manda), con il registro del motore
// sulla PB nuova o spostata di un file che il motore propone (registroDelFile). Le azioni restano {allegato_id,
// chiave_bozza}. Decide il server (distintabozza/provenienza3.go): sul nodo proposto «motore», su un altro nodo
// «corretta» (G39-01 [U]: la proposta del motore spostata dall'ingegnere su un altro componente); senza registro
// «manuale». Vale per ogni gesto che preassocia (Accetta, l'Elenco, «Associa a un nodo…», le mosse della Sorgente, il
// trascinamento): la pagina dichiara solo da dove viene la proposta, mai la provenienza [T-39].
// Una PB legacy (le pagine di prima, la conversione P1) spostata non porta il registro: il server la registra «manuale»
// (provenienza3.go, «legacy → manuale», il rimedio di R39-15 [T-39]; il riepilogo lo dice: «Spostate su un altro nodo
// diventano manuali»), anche se il motore propone il file (rilievo minore della revisione indipendente 3 della corsia).
function conRegistriDelMotore(azioni) {
  if (!arr(azioni).some((a) => a && a.azione === "file_preassociato")) return azioni;
  return azioni.map((a) => {
    if (!a || a.azione !== "file_preassociato" || !ogg(a.dopo) || a.dopo.importazione) return a;
    if (pbLegacy(a.dopo.allegato_id)) return a;
    const r = registroDelFile(a.dopo.allegato_id);
    return r ? Object.assign({}, a, { dopo: Object.assign({}, a.dopo, { importazione: r }) }) : a;
  });
}
// pbLegacy: il file ha nella versione salvata una PB legacy (la provenienza che il server ha letto da R1).
function pbLegacy(aid) {
  const a = String(aid || "").toLowerCase();
  return !!a && arr(S.salvato && S.salvato.documenti_preassociati).some((d) => ogg(d) && d.provenienza === "legacy" && String(d.allegato_id || "").toLowerCase() === a);
}
// senzaRegistriDelMotore: la copia locale delle azioni senza i registri dichiarati (dopo il 422 proposta_non_corrispondente:
// un registro letto prima può essere superato). Le azioni da inviare non li hanno mai.
function senzaRegistriDelMotore(locali) {
  return arr(locali).map((a) => {
    if (!a || a.azione !== "file_preassociato" || !ogg(a.dopo) || !a.dopo.importazione) return a;
    const dopo = Object.assign({}, a.dopo);
    delete dopo.importazione;
    return Object.assign({}, a, { dopo });
  });
}
// fai: un gesto della bozza. Le azioni vanno in coda (debounce 1,5 s, MAT §3.4), si applicano subito allo snapshot che
// si vede e diventano un passo della pila. Niente arriva alla working: la scrive solo R5 (D23:254).
function fai(etichetta, azioni, o) {
  const no = perchéNoModifica();
  if (no) {
    avvisa("errore", "Il gesto non parte: " + no + ".");
    return false;
  }
  const voce = { etichetta, inviare: azioni, locali: o && o.locali ? o.locali : conRegistriDelMotore(azioni), numeri: null, rebase: o && o.rebase ? o.rebase : null };
  S.coda.push(voce);
  S.pila.push({ etichetta, voci: [voce], salvata: false });
  S.rifai = [];
  ricalcola();
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: "modifiche" });
  programmaSalvataggio(voce.rebase ? 0 : 1500);
  avvisa("info", "Nella bozza: " + etichetta + ". Si salva da sola; «Annulla» la toglie.");
  render();
  return true;
}

// idGesto: l'azione_id comune a tutte le azioni di un passo (il gesto cumulativo «Non includere»), o "".
function idGesto(voce) {
  const ids = new Set(voce.locali.map((a) => (ogg(a.dopo) && a.dopo.azione_id) || (ogg(a.prima) && a.prima.azione_id) || ""));
  const id = ids.size === 1 ? [...ids][0] : "";
  return typeof id === "string" ? id : "";
}
// annulla: l'ultimo passo. Un passo non ancora inviato esce dalla coda; uno salvato manda «annulla{n}» per i suoi
// eventi, in ordine inverso (EM §7.6: per il gesto cumulativo l'ordine fisso decisioni, nodi, archi, preassociazioni,
// che è l'inverso dell'ordine con cui il gesto le ha scritte). Mai oltre l'ultima versione del server (EM §9.2).
function annulla() {
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "Annulla non parte: " + no + ".");
  if (S.invio) return avvisa("errore", "Annulla non parte: un salvataggio è in corso, aspetta il suo esito.");
  const passo = S.pila.pop();
  if (!passo) return;
  const voce = passo.voci[0];
  const i = S.coda.indexOf(voce);
  if (i >= 0) {
    S.coda.splice(i, 1);
    S.rifai.push(passo);
  } else if (voce.numeri && voce.numeri.length) {
    const numeri = voce.numeri.slice().sort((a, b) => b - a);
    // l'annullamento di un gesto cumulativo porta il suo azione_id nel «prima» (EM §7.1, ultimo capoverso)
    const id = idGesto(voce);
    const inviare = numeri.map((n) => ({ azione: "annulla", chiave_bozza: null, prima: id ? { azione_id: id } : null, dopo: { n }, origine: "utente" }));
    const locali = voce.locali.slice().reverse().map(inverso);
    const v2 = { etichetta: "annulla: " + passo.etichetta, inviare, locali, numeri: null, annullaDi: passo };
    S.coda.push(v2);
    S.rifai.push(passo);
    programmaSalvataggio(800);
  } else {
    S.pila.push(passo);
    return avvisa("errore", "Questo passo non si annulla da qui: rileggi la bozza.");
  }
  ricalcola();
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: S.coda.length ? "modifiche" : "salvata" });
  avvisa("ok", "Annullato: " + passo.etichetta + ".");
  render();
}
function ripeti() {
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "Ripeti non parte: " + no + ".");
  if (S.invio) return avvisa("errore", "Ripeti non parte: un salvataggio è in corso, aspetta il suo esito.");
  const passo = S.rifai.pop();
  if (!passo) return;
  const voce = passo.voci[0];
  const iAnn = S.coda.findIndex((v) => v.annullaDi === passo);
  if (iAnn >= 0) {
    S.coda.splice(iAnn, 1);
  } else if (voce.numeri && voce.numeri.length) {
    // la ripetizione di un gesto cumulativo porta il suo azione_id nel «prima», come l'annullamento, e va nell'ordine del
    // gesto (EM §7.1; DECA PE-05: simmetrico)
    const id = idGesto(voce);
    const inviare = voce.numeri.slice().sort((a, b) => a - b).map((n) => ({ azione: "rifai", chiave_bozza: null, prima: id ? { azione_id: id } : null, dopo: { n }, origine: "utente" }));
    S.coda.push({ etichetta: "ripeti: " + passo.etichetta, inviare, locali: voce.locali.slice(), numeri: null });
    programmaSalvataggio(800);
  } else {
    S.coda.push(voce);
    programmaSalvataggio();
  }
  S.pila.push(passo);
  ricalcola();
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: S.coda.length ? "modifiche" : "salvata" });
  avvisa("ok", "Ripetuto: " + passo.etichetta + ".");
  render();
}

// ---------------------------------------------------------------- il salvataggio (R2) e il riepilogo in sottofondo (R4)

// snapshotDaInviare: i campi dello snapshot salvato (EM §3.2): mai bozza_id, bozza_versione, chiave_idempotenza.
function senzaRegistriDichiarati(s) {
  const o = Object.assign({}, s);
  o.documenti_preassociati = arr(s.documenti_preassociati).map((d) => (ogg(d) && !d.provenienza && d.importazione ? { allegato_id: d.allegato_id, chiave_bozza: d.chiave_bozza } : d));
  return o;
}
function snapshotDaInviare(s) {
  const o = {};
  for (const k of ["formato", "prodotto_id", "sorgente", "versione_bom_base", "firma_base", "nodi", "archi", "decisioni", "documenti_preassociati"]) if (s[k] !== undefined) o[k] = clona(s[k]);
  return o;
}
function corpoBozza() {
  return Object.assign(snapshotDaInviare(S.salvato), { bozza_id: S.bozza.bozza_id, bozza_versione: S.bozza.versione });
}
let timerSalva = null;
function programmaSalvataggio(ms) {
  clearTimeout(timerSalva);
  timerSalva = setTimeout(salva, ms === undefined ? 1500 : ms);
}
async function salva() {
  if (S.invio || !S.coda.length || !bozzaLavorabile()) return;
  if (["conflitto", "rifiutata"].includes(S.salvataggio.stato)) return;
  // un rebase si salva da solo (il corpo porta «rebase»)
  let n = S.coda.findIndex((v) => v.rebase);
  if (n === 0) n = 1;
  else if (n < 0) n = S.coda.length;
  const voci = S.coda.splice(0, n);
  const azioni = [];
  voci.forEach((v) => v.inviare.forEach((a) => azioni.push(a)));
  const snapshot = applicaVoci(S.salvato, voci);
  const corpo = { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione, prodotto_id: S.prodotto, vocabolario: 2, snapshot: snapshotDaInviare(snapshot), azioni: clona(azioni) };
  if (voci[0].rebase) corpo.rebase = clona(voci[0].rebase);
  S.invio = { voci, azioni, snapshot, corpo };
  S.tentativi = 0;
  await invia();
}
async function invia() {
  if (!S.invio) return;
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: "in_corso", testo: "" });
  renderBarra();
  renderNav();
  const r = await P.salvaBozza(S.invio.corpo);
  esitoSalvataggio(r);
}
function esitoSalvataggio(r) {
  const inv = S.invio;
  if (!inv) return;
  if (r.esito === "ok") {
    // il registro dichiarato con un suggerimento accettato vale per quel salvataggio: dopo, la PB ha la provenienza decisa
    // dal server, che la pagina non rilegge; si omette (il server la rimette dalla versione prima), mai si rimanda diversa
    S.salvato = senzaRegistriDichiarati(inv.snapshot);
    S.bozza.versione = r.dati && typeof r.dati.versione === "number" ? r.dati.versione : S.bozza.versione + 1;
    S.bozza.impronta = r.dati && r.dati.impronta;
    let n = S.totaleEventi;
    inv.voci.forEach((v) => {
      v.numeri = v.inviare.map(() => ++n);
    });
    S.totaleEventi = n;
    // il giornale della pagina segue il salvataggio, con i numeri che il server ha dato agli eventi (in ordine): il vassoio
    // «Da ricollocare», «↺ Ripristina» e «Includi di nuovo» leggono il giornale, e senza questi eventi li perderebbero
    // fino alla prossima rilettura di R1, che resta la fonte vera
    if (Array.isArray(S.bozza.eventi)) {
      const quando = new Date().toISOString();
      inv.voci.forEach((v) => v.inviare.forEach((a, j) => S.bozza.eventi.push({ n: v.numeri[j], versione: S.bozza.versione, vocabolario: 2, azione: a.azione,
        chiave: a.chiave_bozza === undefined ? null : a.chiave_bozza, prima: a.prima === undefined ? null : a.prima, dopo: a.dopo === undefined ? null : a.dopo,
        origine: a.origine || "utente", motivo: a.motivo || null, autore: "", quando })));
    }
    S.invio = null;
    S.tentativi = 0;
    S.salvataggio = { stato: S.coda.length ? "modifiche" : "salvata", testo: r.dati && r.dati.riproposto ? "riproposto" : "", versione: S.bozza.versione, ora: ora() };
    ricalcola();
    if (S.coda.length) programmaSalvataggio(300);
    programmaRiepilogo();
    renderBarra();
    // la pillola della Revisione dice la versione salvata, come la barra (collaudo UI46, I1: restava una versione indietro
    // con «1 da salvare» fino alla lettura dopo)
    renderNav();
    // le righe dello smistamento (S9) cambiano con la bozza: si rilegge, raggruppato (U2 §2.3)
    caricaSmistamentoPoi();
    // dopo un rebase lo stato della base lo dice il server: R1 si rilegge (senza modifiche in coda, caricaBozza)
    if (inv.voci.some((v) => v.rebase)) caricaBozza().then(() => render());
    return;
  }
  if (r.codice === "PONTE_SESSIONE") {
    // la sessione è scaduta prima del gestore: niente è scritto. Il corpo e la chiave_salvataggio restano nella pagina
    // (S.invio, e nel ponte la stessa chiave per lo stesso corpo): se chi guarda resta sulla pagina, dopo il login «Riprova
    // ora» li rimanda uguali (MAT §5.4, prima riga). Dopo una ricarica non ci sono più: stanno solo in memoria (C3 §6.2)
    S.tentativi = 3;
    S.salvataggio = Object.assign({}, S.salvataggio, { stato: "non_arrivato", testo: "Sessione scaduta: dopo il login, «Riprova ora» rimanda le modifiche con la stessa chiave.", codice: r.codice });
    renderBarra();
    return;
  }
  if (r.esito === "conflitto" || r.esito === "no") {
    S.coda = inv.voci.concat(S.coda);
    S.invio = null;
    // il 422 annullamento_non_ammesso ha il suo testo (MAT §5.4, riga R2; BZ-18); così i motivi delle preassociazioni per
    // allegato_id (direttiva 35, C-2: i nomi sono scelte [T] della corsia persistenza)
    const testo = TESTI_R2_MOTIVO[r.motivo] || TESTI_R2_CODICE[r.codice] || r.testo;
    S.salvataggio = { stato: r.esito === "conflitto" ? "conflitto" : "rifiutata", codice: r.codice, motivo: r.motivo, testo, dati: r.dati, versione: S.bozza.versione, ora: S.salvataggio.ora };
    ricalcola();
    render();
    return;
  }
  // rete, 5xx, risposta illeggibile: si rimanda lo STESSO corpo (la stessa chiave_salvataggio: se il primo era arrivato,
  // R2 risponde «riproposto»), fino a tre volte con attese crescenti, poi «Riprova ora» (MAT §5.1)
  S.tentativi++;
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: "non_arrivato", testo: r.testo, codice: r.codice });
  renderBarra();
  if (S.tentativi < 3) setTimeout(invia, [1000, 3000, 9000][S.tentativi - 1]);
}
async function salvaOra() {
  clearTimeout(timerSalva);
  if (S.invio) {
    if (S.salvataggio.stato === "non_arrivato") await invia();
    else await attendi(() => !S.invio || S.salvataggio.stato === "non_arrivato", 30000);
  }
  if (S.coda.length) await salva();
  await attendi(() => (!S.invio && !S.coda.length) || ["conflitto", "rifiutata"].includes(S.salvataggio.stato) || (S.salvataggio.stato === "non_arrivato" && S.tentativi >= 3), 30000);
  return !S.invio && !S.coda.length && S.salvataggio.stato === "salvata";
}
function attendi(condizione, ms) {
  return new Promise((ok) => {
    const fine = Date.now() + ms;
    const giro = () => {
      if (condizione() || Date.now() > fine) ok();
      else setTimeout(giro, 80);
    };
    giro();
  });
}
// La coda per risorsa della pagina (MAT §1.3): una seconda scrittura sulla stessa risorsa (gli atti, un file, la bozza
// per R3, F8, il caricamento) parte quando la prima ha un esito, e non si perde con PONTE_IN_VOLO. R2 ha la sua coda
// (S.coda, S.invio); R5 parte dal wizard dopo il salvataggio.
const codeRisorse = new Map();
const inCodaPer = new Map();
function inSerie(risorsa, fn) {
  const prima = codeRisorse.get(risorsa) || Promise.resolve();
  inCodaPer.set(risorsa, (inCodaPer.get(risorsa) || 0) + 1);
  const p = prima.then(fn, fn);
  const fine = p.then(() => {}, () => {}).then(() => inCodaPer.set(risorsa, inCodaPer.get(risorsa) - 1));
  codeRisorse.set(risorsa, fine);
  return p;
}
// inVoloSu: quante scritture della pagina sono in volo o in coda sulla risorsa.
function inVoloSu(risorsa) {
  return inCodaPer.get(risorsa) || 0;
}
let timerRiep = null;
function programmaRiepilogo() {
  clearTimeout(timerRiep);
  S.riepilogoStato = "da_aggiornare";
  timerRiep = setTimeout(riepilogoSfondo, 1500);
}
// riepilogoSfondo: R4 dopo un R2 riuscito, senza gli avvisi del motore (EM §9.4; MAT §1.5): i conti della barra e gli
// stati delle associazioni. Finché non risponde i conti dicono «da aggiornare».
async function riepilogoSfondo() {
  if (!bozzaLavorabile() || S.invio || S.coda.length) return;
  const v = S.bozza.versione;
  let nuovo = false;
  try {
    const r = await P.riepilogoBom(corpoBozza());
    if (bozzaLavorabile() && S.bozza.versione === v && !S.coda.length) {
      S.riepilogo = r;
      S.riepilogoStato = "aggiornato";
      S.riepilogoVersione = v;
      nuovo = true;
    }
  } catch (e) {
    S.riepilogoStato = "errore";
    S.riepilogoErrore = testoErrore(e);
  }
  // la barra: i conti e il CTA, che segue uguale_alla_base di R4 quando c'è (PE-23)
  renderBarra();
  // l'Elenco e la linguetta «Documenti del nodo» mostrano gli stati delle preassociazioni che dà R4: si ridisegnano. Il
  // pannello del nodo mostra l'id. cliente che dà R4: si ridisegna se quello a schermo non è più quello di R4 (UI46,
  // controllo di stabilità del 21/10: dopo Correggi o Annulla restava l'id. cliente di prima)
  if (nuovo && S.passo === "revisione" && (S.vista === "elenco" || (S.nodo && S.linguettaNodo === "documenti") || idcPannelloVecchio())) ridisegnaCorpoFermo();
}
// idcPannelloVecchio: il pannello del nodo in vista mostra un id. cliente diverso da quello che dà ora il riepilogo
function idcPannelloVecchio() {
  const p = S.passo === "revisione" && $("#du-pannello");
  if (!p || !S.nodo || p.dataset.k !== S.nodo) return false;
  // l'id. cliente sta nella sezione «Decisione dell'ingegnere», non per un pezzo non incluso
  const sez = p.querySelector("#du-sez-decisione");
  if (!sez || sez.querySelector("#du-noninc-deciso")) return false;
  const e = sez.querySelector("[data-du-idc]");
  const n = idClienteNodo(S.nodo);
  return (e ? e.dataset.duIdc : null) !== (n ? n.dataset.duIdc : null);
}
// ridisegnaCorpoFermo: renderCorpo, ma non durante un trascinamento, un pan o un pinch della tela (lo rimanda a gesto finito)
let timerCorpoFermo = null;
function ridisegnaCorpoFermo() {
  clearTimeout(timerCorpoFermo);
  if (V.punti.size || V.drag || V.pan || V.pinch) {
    timerCorpoFermo = setTimeout(ridisegnaCorpoFermo, 400);
    return;
  }
  renderCorpo();
}

// ---------------------------------------------------------------- registro d'importazione, voci, caselle

function registroBozza() {
  const imp = S.bozza && S.bozza.importazione;
  return imp && S.registri[imp.impronta] ? S.registri[imp.impronta] : null;
}
function indiceVoci(reg) {
  if (!reg) return null;
  if (reg.__indice) return reg.__indice;
  const m = new Map();
  for (const v of arr(reg.proposta && reg.proposta.nodi)) {
    if (v && v.voce) m.set(v.voce, v);
    if (v && v.origine && v.origine.rif) m.set("rif:" + v.origine.allegato_id + ":" + v.origine.rif, v);
  }
  Object.defineProperty(reg, "__indice", { value: m, enumerable: false });
  return m;
}
function voceDiCasella(reg, c) {
  const ix = indiceVoci(reg);
  if (!ix || !c) return null;
  return ix.get("n:" + c.allegato_id + ":" + c.rif) || ix.get("rif:" + c.allegato_id + ":" + c.rif) || null;
}
function casellaDi(n) {
  return arr(n && n.caselle)[0] || null;
}
function segnaliDi(v) {
  return arr(v && v.segnali);
}

// ---------------------------------------------------------------- i file della Sorgente

function fileSorgente() {
  return arr(S.sorgente && S.sorgente.file);
}
function fileDi(allegato) {
  return fileSorgente().find((f) => f.allegato_id === allegato) || null;
}
function nomeFile(allegato) {
  const f = fileDi(allegato);
  return f ? f.nome_file : allegato ? allegato.slice(0, 8) : "";
}
// le preassociazioni (PB), gli svincoli, gli eventi dei file e il vassoio citano l'occorrenza, allegato_id, non la
// classificazione (direttiva 35, C-2): il file si trova con fileDi, e un file senza proposta di documento si preassocia
function pbDi(allegato) {
  return S.lavoro ? S.lavoro.documenti_preassociati.find((d) => d.allegato_id === allegato) || null : null;
}
// azioniPreassocia: le azioni per preassociare l'allegato al nodo k. Il server rifiuta un file_preassociato su un
// allegato già preassociato (applica2.go; provvisorio, base [T] D-ASS-04 A): se è già preassociato a un altro nodo,
// prima lo svincolo (motivo utente), poi la preassociazione; sullo stesso nodo nessuna azione.
function azioniPreassocia(allegato, k) {
  const cur = pbDi(allegato);
  if (cur && cur.chiave_bozza === k) return [];
  const out = [];
  if (cur) out.push({ azione: "file_svincolato", chiave_bozza: cur.chiave_bozza, prima: clona(cur), dopo: { allegato_id: allegato, motivo: "utente" }, origine: "utente" });
  out.push({ azione: "file_preassociato", chiave_bozza: k, prima: null, dopo: { allegato_id: allegato, chiave_bozza: k }, origine: "utente" });
  return out;
}
// Le associazioni registrate (AR) lette dalle righe di S9 (direttiva 35 §1, DIR35-DP [U]: documento_proposta.componente_id
// non è più la fonte delle AR nel percorso V1; D-TAB-04 A [T-39]): una riga gia_registrato o da_scollegare con la
// destinazione {componente_id}, di qualunque sezione (S9 la dà nella sezione del prodotto che contiene il componente e in
// quelle per cui il file è pertinente). documento: la riga del documento confermato senza AR (motivo
// documento_confermato, D-AR04 (ii) B [U]). Senza S9 letto la pagina non sa quali AR ci sono: nessuna (la Sorgente dice
// «smistamento non letto»), mai la copia.
function associazioniRegistrate() {
  const out = new Map();
  for (const p of arr(S.smist && S.smist.prodotti)) {
    for (const r of arr(p.righe)) {
      if (!ogg(r) || (r.stato !== "gia_registrato" && r.stato !== "da_scollegare")) continue;
      const c = r.destinazione && r.destinazione.componente_id;
      if (typeof c !== "string" || !c || !r.allegato_id) continue;
      const a = String(r.allegato_id).toLowerCase();
      if (!out.has(a)) out.set(a, { componente_id: c.toLowerCase(), documento: r.motivo === "documento_confermato" });
    }
  }
  return out;
}
function arDi(allegato) {
  return associazioniRegistrate().get(String(allegato || "").toLowerCase()) || null;
}
const ANALISI = { non_scaricato: "non scaricato", download_in_coda: "in analisi", in_staging: "in analisi", estrazione_in_coda: "in analisi", in_analisi: "in analisi",
  letto: "letto", errore: "errore" };
function testoAnalisi(f) {
  const a = (f.analisi && f.analisi.stato) || "";
  if (a === "errore") return "errore: " + ((f.analisi && f.analisi.motivo) || "motivo non dato");
  if (a === "non_scaricato") return "non scaricato (posta)";
  return ANALISI[a] || a || "—";
}
function azioneFile(f, nome) {
  const a = f.azioni && f.azioni[nome];
  if (a === undefined || a === null) return { ok: false, motivo: "il server non dice se il gesto è ammesso" };
  if (typeof a === "boolean") return { ok: a, motivo: a ? "" : "non ammesso per questo file" };
  return { ok: !!a.disponibile, motivo: a.motivo || "" };
}
const TESTI_RIMOZIONE = {
  in_attesa: "Scarto registrato; rimozione dallo staging ancora da completare.",
  errore: "Scarto registrato; rimozione dallo staging ancora da completare.",
};
function testoRimozione(f) {
  const r = f.rimozione;
  if (!r) return "";
  if (r.testo) return r.testo;
  return TESTI_RIMOZIONE[r.stato] || "";
}

// ---------------------------------------------------------------- il grafo da disegnare (organigramma ed elenco)

// La scheda porta l'anteprima del 2D (A3 orizzontale, 420/297) come nella pagina di prima (messaggio 46 [U]): CH è
// l'altezza fissa che l'impaginazione usa; SW e SH sono la scheda «stesso pezzo» di un figlio sotto un secondo padre.
const CW = 236, CH = 252, MARGINE = 24, GXC = 26, BUS1 = 18, BUS2 = 34, SW = 218, SH = 30, KMIN = 0.2, KMAX = 2.2;
const TIPI = { finito: "Finito", sottoassieme: "Assieme", sciolto: "Particolare", commerciale: "Commerciale" };

function emittenteParola(id) {
  if (!id || typeof id.emittente !== "string") return "";
  if (id.emittente.indexOf("partner:") === 0) return "fornitore " + id.emittente.slice(8);
  return id.emittente_presunto ? "cliente, presunto" : "cliente";
}
// grafoBozza: la BOM di lavoro dalla bozza che si vede (S.lavoro), con il registro della bozza per le identità
// proposte, i segnali e le proposte d'associazione. I nodi non inclusi stanno solo nelle decisioni (EM §7.1): si
// disegnano in fondo, attenuati.
function grafoBozza() {
  const s = S.lavoro;
  const reg = registroBozza();
  const nodi = {};
  const rivedere = daRivedere();
  let radice = "";
  for (const n of s.nodi) {
    const id = n.identita || {};
    const c = casellaDi(n);
    const v = c ? voceDiCasella(reg, c) : null;
    const stato = n.op === "esclude" ? { c: "bozza", t: "escluso" }
      : n.origine === "utente" && !c ? { c: "bozza", t: "nuovo" }
      : id.modo === "proposta" ? { c: "prop", t: "proposto" }
      : id.modo === "decisione" ? { c: "bozza", t: "decisa in bozza" }
      : id.modo === "da_decidere" || (n.op === "crea" && !id.codice) ? { c: "bozza", t: "senza codice" }
      : { c: "reg", t: "registrato" };
    nodi[n.chiave_bozza] = { k: n.chiave_bozza, cod: id.codice || "", rev: id.rev || "", desc: n.descrizione || "", tipo: n.tipo || "sciolto", stato, nodo: n, voce: v,
      fornitore: typeof id.emittente === "string" && id.emittente.indexOf("partner:") === 0, cad: segnaliDi(v).length > 0, rivedere: rivedere.get(n.chiave_bozza) || [],
      escluso: n.op === "esclude", nuovo: n.origine === "utente" && !c, proposto: id.modo === "proposta" };
    if (n.componente_id && n.componente_id === s.prodotto_id) radice = n.chiave_bozza;
  }
  const qtaRegistro = new Map();
  for (const a of arr(reg && reg.proposta && reg.proposta.archi)) qtaRegistro.set(a.padre + ">" + a.figlio, a.quantita);
  const archi = [];
  for (const a of s.archi) {
    if (!nodi[a.padre] || !nodi[a.figlio]) continue;
    const cp = casellaDi(nodi[a.padre].nodo);
    const cf = casellaDi(nodi[a.figlio].nodo);
    const qtaStep = cp && cf ? qtaRegistro.get(cp.rif + ">" + cf.rif) : undefined;
    const kind = a.origine === "registrato" || a.origine === "conferma" ? "conf" : a.origine === "utente" || (qtaStep !== undefined && qtaStep !== a.qta) ? "bozza" : "prop";
    archi.push({ id: a.padre + ">" + a.figlio, p: a.padre, f: a.figlio, qta: a.qta, qtaStep, origine: a.origine, kind, arco: a });
  }
  const nonInclusi = [];
  for (const d of s.decisioni) {
    if (d.tipo !== "non_includere") continue;
    const v = voceDiCasella(reg, d.casella);
    const k = "ni:" + d.id;
    nodi[k] = { k, cod: (v && v.codice_proposto) || "", rev: "", desc: (v && v.grezzi && (v.grezzi.nome || v.grezzi.id)) || (d.casella && d.casella.rif) || "",
      tipo: (v && v.tipo_proposto && v.tipo_proposto.valore) || "sciolto", stato: { c: "bozza", t: "non incluso (bozza)" }, decisione: d, voce: v, noninc: true,
      cad: segnaliDi(v).length > 0, rivedere: rivedere.get(d.id) || [] };
    nonInclusi.push(k);
  }
  return { radice, nodi, archi, nonInclusi, modo: "bozza" };
}
// grafoProposta: la proposta di uno STEP in vista (S6, in lettura): lo stato di partenza che la bozza avrebbe e il
// registro del gruppo. Guardare non conferma niente (RAT05-FONTE-SOSTITUIBILE [U]).
function grafoProposta(anteprima) {
  const a = anteprima || S.anteprima;
  const pr = a && a.proposta;
  if (!pr) return null;
  const reg = pr.contenuto || null;
  const sp = ogg(pr.stato_di_partenza) ? normalizza(pr.stato_di_partenza) : null;
  const nodi = {};
  let radice = "";
  const archi = [];
  if (sp) {
    for (const n of sp.nodi) {
      const id = n.identita || {};
      const c = casellaDi(n);
      const v = c ? voceDiCasella(reg, c) : null;
      nodi[n.chiave_bozza] = { k: n.chiave_bozza, cod: id.codice || "", rev: id.rev || "", desc: n.descrizione || "", tipo: n.tipo || "sciolto", nodo: n, voce: v,
        stato: id.modo === "registrato" ? { c: "reg", t: "registrato" } : { c: "prop", t: id.codice ? "proposto" : "senza codice" },
        fornitore: typeof id.emittente === "string" && id.emittente.indexOf("partner:") === 0, cad: segnaliDi(v).length > 0, rivedere: [], proposto: true };
      if (n.componente_id && n.componente_id === sp.prodotto_id) radice = n.chiave_bozza;
    }
    for (const x of sp.archi) if (nodi[x.padre] && nodi[x.figlio]) archi.push({ id: x.padre + ">" + x.figlio, p: x.padre, f: x.figlio, qta: x.qta, qtaStep: x.qta, origine: x.origine, kind: "prop", arco: x });
  }
  return { radice, nodi, archi, nonInclusi: [], modo: "proposta", registro: reg };
}
function figliA(g, k) {
  return g.archi.filter((a) => a.p === k && g.nodi[a.f]);
}
function padriA(g, k) {
  return g.archi.filter((a) => a.f === k && g.nodi[a.p]);
}
function discendenti(g, k) {
  const s = new Set();
  const giro = (x) => figliA(g, x).forEach((a) => {
    if (!s.has(a.f)) {
      s.add(a.f);
      giro(a.f);
    }
  });
  giro(k);
  return s;
}
function contenitore(v) {
  return v && (v.tipo === "finito" || v.tipo === "sottoassieme");
}

// ---------------------------------------------------------------- impaginazione dell'organigramma (dal mockup v3, §4.1)

function alberoPrimario(g) {
  const kids = {}, rim = {}, padrePrim = {}, visti = new Set();
  function dfs(k) {
    figliA(g, k).forEach((a) => {
      if (!visti.has(a.f)) {
        visti.add(a.f);
        padrePrim[a.f] = k;
        (kids[k] = kids[k] || []).push(a);
        dfs(a.f);
      } else (rim[k] = rim[k] || []).push(a);
    });
  }
  if (g.radice) {
    visti.add(g.radice);
    dfs(g.radice);
  }
  const altre = [];
  const resto = Object.keys(g.nodi).filter((k) => !visti.has(k) && !g.nodi[k].noninc);
  resto.filter((k) => padriA(g, k).length === 0).concat(resto).forEach((k) => {
    if (!visti.has(k)) {
      visti.add(k);
      altre.push(k);
      dfs(k);
    }
  });
  return { kids, rim, padrePrim, altre };
}
function compressiCorr() {
  const c = S.prodotto + ":" + S.passo;
  return (S.compressi[c] = S.compressi[c] || {});
}
// impagina: l'organigramma della pagina di prima (messaggio 46 [U], «preferivo la disposizione come era prima»): ogni padre
// ha i suoi figli in una riga sola sotto di sé, con il padre centrato sopra di loro, a ogni livello; l'albero si allarga in
// orizzontale e si guarda con il pan e lo zoom (prima con lo scorrimento). Le linee: il gambo del padre e la barra fra i
// fratelli neutri, la caduta su ogni figlio con il tratto del suo legame (file, confermato, bozza). Un figlio che sta già
// sotto un altro padre è una scheda «stesso pezzo». In fondo, i pezzi senza padre e i non inclusi, ognuno con il suo
// sottoalbero. Solo presentazione: la parentela la dice la bozza.
function impagina(g, ap) {
  const comp = compressiCorr();
  const pos = {}, stubs = [], tratti = [], pills = [], etichette = [];
  const ext = { X: 0, Y: 0 };
  const cresci = (x2, y2) => {
    ext.X = Math.max(ext.X, x2);
    ext.Y = Math.max(ext.Y, y2);
  };
  const voci = (k) => (comp[k] ? [] : (ap.kids[k] || []).map((a) => ({ a })).concat((ap.rim[k] || []).map((a) => ({ a, stub: true }))));
  const larghezze = {};
  const somma = (it) => it.reduce((w, x, i) => w + (i ? GXC : 0) + (x.stub ? SW : largh(x.a.f)), 0);
  function largh(k) {
    if (larghezze[k] === undefined) larghezze[k] = Math.max(CW, somma(voci(k)));
    return larghezze[k];
  }
  // metti: il sottoalbero di k nella striscia [x0, x0 + largh(k)], con la scheda di k in cima a y; torna il fondo
  function metti(k, x0, y) {
    const it = voci(k);
    if (!it.length) {
      pos[k] = { x: x0 + (largh(k) - CW) / 2, y };
      cresci(pos[k].x + CW, y + CH);
      return y + CH;
    }
    let x = x0 + (largh(k) - somma(it)) / 2;
    const yb = y + CH + BUS1;
    const yf = yb + BUS2;
    let fondo = y + CH;
    const cadute = [];
    it.forEach((z) => {
      if (z.stub) {
        stubs.push({ a: z.a, x, y: yf, w: SW });
        cresci(x + SW, yf + SH);
        const dx = x + SW / 2;
        cadute.push(dx);
        tratti.push({ d: "M" + dx + " " + yb + " V" + yf, a: z.a, secondo: true });
        pills.push({ a: z.a, x: dx, y: yf - BUS2 / 2, secondo: true });
        fondo = Math.max(fondo, yf + SH);
        x += SW + GXC;
        return;
      }
      const w = largh(z.a.f);
      fondo = Math.max(fondo, metti(z.a.f, x, yf));
      const dx = pos[z.a.f].x + CW / 2;
      cadute.push(dx);
      tratti.push({ d: "M" + dx + " " + yb + " V" + yf, a: z.a });
      pills.push({ a: z.a, x: dx, y: yf - BUS2 / 2 });
      x += w + GXC;
    });
    const cx = (cadute[0] + cadute[cadute.length - 1]) / 2;
    pos[k] = { x: cx - CW / 2, y };
    cresci(pos[k].x + CW, y + CH);
    tratti.push({ d: "M" + cx + " " + (y + CH) + " V" + yb, neutro: true });
    if (cadute.length > 1) tratti.push({ d: "M" + cadute[0] + " " + yb + " H" + cadute[cadute.length - 1], neutro: true });
    return fondo;
  }
  let yE = MARGINE;
  if (g.radice) yE = metti(g.radice, MARGINE, MARGINE);
  const fuori = [["Senza padre · da ricollocare", ap.altre], ["Non inclusi nella BOM (bozza)", g.nonInclusi]];
  for (const [titolo, elenco] of fuori) {
    if (!elenco.length) continue;
    yE = Math.max(yE, ext.Y) + 40;
    etichette.push({ x: MARGINE, y: yE, t: titolo });
    const yC = yE + 28;
    let x = MARGINE;
    let yMax = yC;
    elenco.forEach((k) => {
      const w = largh(k);
      yMax = Math.max(yMax, metti(k, x, yC));
      x += w + GXC;
    });
    yE = yMax;
  }
  return { W: ext.X + MARGINE, H: Math.max(ext.Y, yE) + MARGINE, cw: CW, pos, stubs, tratti, pills, etichette, kids: ap.kids, padrePrim: ap.padrePrim, modo: "albero" };
}
function layout(g) {
  return impagina(g, alberoPrimario(g));
}

// ---------------------------------------------------------------- le schede e la tela

// diversaDallaProposta: in che cosa la scheda non è come l'ha importata la bozza (MAT BZ-25): codice, rev e tipo contro la
// voce del registro della bozza (R1 ?importazione=1), mai contro la proposta di oggi. "" se è uguale o non c'è la voce.
function diversaDallaProposta(v) {
  const voce = v.voce;
  if (!voce || !v.nodo || v.noninc) return "";
  const id = v.nodo.identita || {};
  if (id.modo === "registrato") return "";
  const diff = [];
  if ((id.codice || "") !== (voce.codice_proposto || "")) diff.push("codice " + (voce.codice_proposto || "(vuoto)") + " → " + (id.codice || "(vuoto)"));
  if ((id.rev || "") !== (voce.rev_proposta || "")) diff.push("rev " + (voce.rev_proposta || "(vuota)") + " → " + (id.rev || "(vuota)"));
  const tipo = voce.tipo_proposto && voce.tipo_proposto.valore;
  if (tipo && v.tipo !== tipo) diff.push("tipo " + (TIPI[tipo] || tipo) + " → " + (TIPI[v.tipo] || v.tipo));
  return diff.join("; ");
}
function segniScheda(g, v) {
  const out = [];
  const diff = g.modo === "bozza" && S.confronto ? diversaDallaProposta(v) : "";
  if (diff) out.push(h("span", { class: "du-segno du-segno--bozza", title: "Diverso dalla proposta importata: " + diff }, "≠ proposta"));
  if (v.cad) out.push(h("span", { class: "du-segno du-segno--cad", title: "Segnale del motore: possibile elemento CAD, non un articolo. È solo un segnale: il nodo resta e il suo 2D resta richiesto finché non decidi." }, "possibile elemento CAD"));
  if (v.fornitore) out.push(h("span", { class: "du-segno du-segno--fornitore", title: "L'emittente del codice non è il cliente" }, "fornitore"));
  if (v.rivedere && v.rivedere.length) out.push(h("span", { class: "du-segno du-segno--rivedere", title: "Da rivedere: " + v.rivedere.join(", ") }, "da rivedere"));
  if (g.modo === "bozza" && !v.noninc && v.k !== g.radice && padriA(g, v.k).length === 0) out.push(h("span", { class: "du-segno du-segno--bozza", title: "Nessun padre: da ricollocare" }, "⤴ ricollocare"));
  const pa = padriA(g, v.k);
  if (pa.length > 1) out.push(h("span", { class: "du-segno du-segno--padri", title: "Stesso componente sotto " + pa.length + " padri" }, "↗ " + pa.length + " padri"));
  return out.slice(0, 3);
}
function selezionabile(g, v) {
  if (g.modo !== "bozza" || v.noninc) return { ok: false, motivo: "" };
  if (v.k === g.radice) return { ok: false, motivo: "è il prodotto" };
  if (v.nodo && v.nodo.componente_id) return { ok: false, motivo: "ha un componente: usa Escludi" };
  if (!casellaDi(v.nodo)) return { ok: false, motivo: "è un pezzo aggiunto a mano: usa «Non crearlo (togli)»" };
  return { ok: true, motivo: "" };
}
// tipoProposto: il tipo della scheda è quello proposto dal file e nessuno l'ha confermato o cambiato (la pagina di prima lo
// scriveva «PARTICOLARE ?»). Un registrato, un pezzo aggiunto a mano e un tipo confermato o cambiato non lo sono.
function tipoProposto(v) {
  // la BOM confermata (working): il tipo è quello della conferma, mai «proposto» (collaudo UI46, giro 3, F3-I3 [T])
  if (!v || v.nuovo || v.noninc || v.wk) return false;
  const n = v.nodo;
  if (n && (n.componente_id || n.tipo_confermato)) return false;
  const tp = v.voce && v.voce.tipo_proposto && v.voce.tipo_proposto.valore;
  return !!tp && tp === v.tipo;
}
function chipTipo(v) {
  const prop = tipoProposto(v);
  return h("span", { class: "du-tipo du-tipo--" + v.tipo + (prop ? " du-tipo--proposto" : ""), title: prop ? "tipo proposto dal file, non ancora confermato" : null },
    (TIPI[v.tipo] || v.tipo) + (prop ? " ?" : ""));
}
function scheda(g, v, ps) {
  const cls = ["du-nc", "du-nc--" + v.tipo];
  if (v.nuovo) cls.push("du-nc--nuovo");
  else if (v.proposto || g.modo === "proposta") cls.push("du-nc--proposto");
  if (v.escluso || v.noninc) cls.push("du-nc--escluso");
  if (S.nodo === v.k) cls.push("du-nc--scelto");
  if (S.selezione.has(v.k)) cls.push("du-nc--selezionato");
  const lungo = String(v.cod || "").length + (v.rev ? 4 + String(v.rev).length : 0);
  const card = h("div", { class: cls.join(" "), id: "nc-" + sid(v.k), "data-k": v.k, "data-du-stato": v.stato.t, style: "left:" + ps.x + "px;top:" + ps.y + "px;width:" + CW + "px;height:" + CH + "px",
    title: (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "") + (v.desc ? " · " + v.desc : "") });
  card.append(h("div", { class: "du-nc-chips" }, chipTipo(v), h("span", { class: "du-nc-st du-nc-st--" + v.stato.c }, v.stato.t)));
  card.append(h("button", { type: "button", class: "du-nc-cod" + (lungo > 17 ? " du-nc-cod--l" : ""), id: "oc-" + sid(v.k),
    "aria-label": (TIPI[v.tipo] || v.tipo) + " " + (v.cod || "senza codice") + (v.rev ? " rev " + v.rev : "") + ", " + v.stato.t + ". Invio: apri nel pannello; menu contestuale: i gesti del pezzo", "aria-current": S.nodo === v.k ? "true" : null },
  h("span", { class: "du-nc-c" }, v.cod || "(senza codice)"), v.rev ? h("span", { class: "du-nc-rev" }, "rev " + v.rev) : null));
  // senza descrizione la riga resta vuota: un «—» sembrava un dato vuoto (messaggio 46, D07)
  card.append(h("span", { class: "du-nc-desc" }, v.desc || ""));
  card.append(miniatura(v));
  card.append(h("div", { class: "du-nc-piede" }, segniScheda(g, v)));
  const sel = selezionabile(g, v);
  if (g.modo === "bozza" && sel.ok) {
    card.append(h("input", { type: "checkbox", class: "du-nc-sel", id: "sel-" + sid(v.k), checked: S.selezione.has(v.k), "aria-label": "Seleziona " + (v.cod || v.desc || "il pezzo") + " per «Non includere»",
      onclick: (e) => {
        e.stopPropagation();
        cambiaSelezione(v.k, e.target.checked);
      } }));
  } else if (g.modo === "bozza" && !v.noninc && sel.motivo) {
    // un nodo che non si può selezionare: la casella c'è, spenta, con il motivo (MAT NI-06); sulla scheda il motivo sta nel
    // suggerimento e nell'etichetta accessibile, nel cassetto «Nodi segnalati» è scritto accanto
    card.append(h("input", { type: "checkbox", class: "du-nc-sel", id: "sel-" + sid(v.k), disabled: true, title: "Non selezionabile: " + sel.motivo,
      "aria-label": "Non selezionabile per «Non includere»: " + sel.motivo, "data-du-motivo": sel.motivo }));
  }
  card.addEventListener("click", (e) => {
    if (Date.now() - V.ultimoMov < 300) return;
    if (e.target.closest(".du-nc-sel")) return;
    if ((e.ctrlKey || e.metaKey) && sel.ok) {
      cambiaSelezione(v.k, !S.selezione.has(v.k));
      return;
    }
    scegli(v.k);
  });
  // il menu del nodo (messaggio 46, D38): il clic destro, il tasto del menu o Maiusc+F10 aprono i gesti del pezzo, non il
  // menu del browser
  card.addEventListener("contextmenu", (e) => {
    e.preventDefault();
    e.stopPropagation();
    apriMenuNodo(v.k, e.clientX, e.clientY);
  });
  card.addEventListener("keydown", (e) => {
    if (e.key !== "ContextMenu" && !(e.key === "F10" && e.shiftKey)) return;
    e.preventDefault();
    const r = card.getBoundingClientRect();
    apriMenuNodo(v.k, r.left + 24, r.top + 24);
  });
  return card;
}
function cambiaSelezione(k, si) {
  if (si) S.selezione.add(k);
  else S.selezione.delete(k);
  render();
}
function scegli(k) {
  // la stessa scheda già scelta e già nel pannello: niente da ridisegnare (UI46, controllo di stabilità del 21/10), se
  // il pannello mostra già quello che dà il riepilogo (l'id. cliente)
  if (k === S.nodo && S.zona === "nodo" && !idcPannelloVecchio()) return;
  // il fuoco su un gesto del pannello di un altro nodo non passa al gesto con lo stesso id del nodo nuovo: va sulla scheda
  const zn = $(".du-zona--nodo");
  const dalPannello = k !== S.nodo && zn && zn.contains(document.activeElement);
  S.nodo = k;
  S.zona = "nodo";
  scriviIndirizzo();
  render();
  const c = dalPannello && document.getElementById("oc-" + sid(k));
  if (c && (document.activeElement === document.body || !document.activeElement)) c.focus({ preventScroll: true });
}
// qPill: la quantità del legame con le parole della pagina di prima (messaggio 46, D07): «×q dal file» e «da confermare»
// per un legame proposto dallo STEP, «✎×q in bozza» per uno deciso nella bozza, «✓×q confermata» per uno registrato.
function qPill(a, g) {
  if (g.modo === "proposta") return h("span", { class: "du-q du-q--prop" }, "×" + a.qta + " dal file");
  if (a.kind === "bozza") return h("span", { class: "du-q du-q--bozza" }, "✎×" + a.qta + " in bozza");
  if (a.kind === "conf") return h("span", { class: "du-q du-q--conf" }, "✓×" + a.qta + " confermata");
  return [h("span", { class: "du-q du-q--prop" }, "×" + a.qta + " dal file"), h("span", { class: "du-q du-q--da" }, "da confermare")];
}
function titoloQta(a, g) {
  const cod = (k) => (g.nodi[k] && (g.nodi[k].cod || g.nodi[k].desc)) || k;
  let t = "Quantità sul legame " + cod(a.p) + " → " + cod(a.f) + ": ×" + a.qta;
  if (a.qtaStep !== undefined && a.qtaStep !== a.qta) t += " (STEP ×" + a.qtaStep + ")";
  return t;
}
// La tela: viewport con pan e zoom, senza scrollbar (mockup v3 §4.4); la camera per prodotto, passo e vista.
const V = { punti: new Map(), pan: null, pinch: null, drag: null, target: null, ultimoMov: 0 };
function chiaveCamera() {
  return S.prodotto + ":" + S.passo + ":" + (S.passo === "sorgente" || anteprimaInRevisione() ? S.anteprimaAllegato + "|" + S.anteprimaGruppo : "bozza");
}
function organigramma(g) {
  const lay = layout(g);
  const vp = h("div", { class: "du-vp", id: "du-vp", tabindex: "0", role: "region", "data-modo": lay.modo,
    "aria-label": "Organigramma " + (g.modo === "proposta" ? "della proposta" : "della BOM di lavoro") + ". Frecce: sposta la vista; più e meno: zoom; 0: adatta" });
  const mondo = h("div", { class: "du-vp-mondo", id: "du-vp-mondo", style: "width:" + lay.W + "px;height:" + lay.H + "px" });
  const svg = sv("svg", { class: "du-vp-archi", width: lay.W, height: lay.H, viewBox: "0 0 " + lay.W + " " + lay.H, "aria-hidden": "true" });
  lay.tratti.forEach((t) => {
    svg.append(sv("path", { d: t.d, class: "du-ar " + (t.neutro ? "du-ar--neutro" : "du-ar--" + t.a.kind + (t.secondo ? " du-ar--secondo" : "")) }));
  });
  mondo.append(svg);
  lay.etichette.forEach((e) => mondo.append(h("div", { class: "du-vp-etic", style: "left:" + e.x + "px;top:" + e.y + "px" }, e.t)));
  Object.keys(lay.pos).forEach((k) => mondo.append(scheda(g, g.nodi[k], lay.pos[k])));
  lay.stubs.forEach((s) => {
    const v = g.nodi[s.a.f];
    mondo.append(h("button", { type: "button", class: "du-nc du-nc--stub du-nc--" + v.tipo, style: "left:" + s.x + "px;top:" + s.y + "px;width:" + s.w + "px;height:" + SH + "px",
      title: "Stesso componente della scheda sotto il primo padre", onclick: () => scegli(v.k) }, "↗ ", h("span", { class: "du-mono" }, v.cod || "(senza codice)"), " stesso pezzo"));
  });
  lay.pills.forEach((pl) => {
    const a = pl.a;
    const no = g.modo !== "bozza" ? "nella Sorgente la quantità è quella del file" : perchéNoModifica();
    mondo.append(h("button", { type: "button", class: "du-oq", id: "oq-" + sid(a.id), style: "left:" + pl.x + "px;top:" + pl.y + "px", disabled: !!no,
      "aria-label": titoloQta(a, g), title: titoloQta(a, g) + (no ? " (" + no + ")" : " · clic per cambiarla"),
      onclick: (e) => {
        e.stopPropagation();
        if (Date.now() - V.ultimoMov < 300) return;
        apriQuantita(a.f, a.p);
      } }, qPill(a, g)));
  });
  vp.append(mondo);
  L = { lay, chiave: chiaveCamera(), vp, mondo, g };
  vp.addEventListener("pointerdown", vpDown);
  vp.addEventListener("wheel", vpWheel, { passive: false });
  vp.addEventListener("keydown", vpKey);
  return vp;
}
// legendaOrg: la legenda sotto l'organigramma, come nella pagina di prima (messaggio 46, D07): i tratti dei legami, le
// schede, i segni e il trascinamento.
function legendaOrg(g) {
  const linea = (c) => h("i", { class: "du-leg-linea du-leg-linea--" + c, "aria-hidden": "true" });
  return h("div", { class: "du-legenda", id: "du-legenda", role: "group", "aria-label": "Legenda dell'organigramma" },
    h("span", {}, linea("prop"), "legame del file (da confermare)"),
    h("span", {}, linea("conf"), "legame confermato"),
    g.modo === "bozza" ? h("span", {}, linea("bozza"), "legame deciso nella bozza") : null,
    h("span", {}, h("i", { class: "du-leg-box du-leg-box--tratt", "aria-hidden": "true" }), "scheda tratteggiata: proposta dal file"),
    h("span", {}, h("b", {}, "?"), " tipo proposto, non confermato"),
    h("span", {}, h("b", {}, "↗"), " stesso pezzo sotto un altro padre"),
    g.modo === "bozza" ? h("span", { class: "du-muto" }, "trascina un pezzo sulla scheda del nuovo padre per spostarlo · clic destro su una scheda: i gesti del pezzo") : null);
}
function cam() {
  return L ? S.camere[L.chiave] : null;
}
function vpDim() {
  const r = L.vp.getBoundingClientRect();
  return { w: r.width || 800, h: r.height || 500 };
}
function applicaCam() {
  if (!L || !L.vp || !document.body.contains(L.vp)) return;
  const c = cam();
  if (!c) return;
  L.mondo.style.transform = "translate(" + c.tx + "px, " + c.ty + "px) scale(" + c.k + ")";
  L.vp.classList.toggle("du-vp--lontano", c.k < 0.6);
  L.vp.dataset.scala = String(Math.round(c.k * 100));
  const z = $("#du-zoom-val");
  if (z) z.textContent = Math.round(c.k * 100) + " %";
}
function setCam(c) {
  if (!L || !c) return;
  c.k = clamp(c.k, KMIN, KMAX);
  // messaggio 46 (D37): l'albero non esce dal riquadro: ne resta in vista almeno metà, o copre almeno metà del riquadro
  // (in larghezza e in altezza); si sposta, ma non si perde
  const d = vpDim();
  const dentro = (t, lato, vista) => {
    const m = Math.min(lato, vista) / 2;
    return clamp(t, m - lato, vista - m);
  };
  c.tx = dentro(c.tx, L.lay.W * c.k, d.w);
  c.ty = dentro(c.ty, L.lay.H * c.k, d.h);
  S.camere[L.chiave] = c;
  applicaCam();
}
// altoVisibile: quanta tela si vede adesso in altezza, fra il suo bordo alto e il fondo della finestra (o la barra in fondo,
// se la copre): la tela della Revisione è alta 100vh - 300 px e comincia sotto testata, linguette e strumenti, quindi una
// parte resta sotto la piega. «Adatta» e l'apertura adattano l'albero alla parte che si vede (collaudo UI46, giro 3,
// F3-I5: il terzo livello restava sotto il bordo finché non si premeva «Adatta») [T]. Almeno 200 px: sotto, la tela intera.
function altoVisibile(d) {
  const r = L.vp.getBoundingClientRect();
  // la barra della bozza è appiccicata in fondo (.du-salva, sticky): copre la tela che le sta dietro
  const barra = $("#du-barra .du-salva") || $("#du-barra");
  const rb = barra ? barra.getBoundingClientRect() : null;
  let fondo = rb && rb.height && rb.top < window.innerHeight && rb.top > r.top ? rb.top : window.innerHeight;
  // l'anteprima nella Sorgente sta nel riquadro del dettaglio, che scorre: si vede fino al suo bordo
  const riq = L.vp.closest("#du-sg-dettaglio");
  if (riq) fondo = Math.min(fondo, riq.getBoundingClientRect().bottom);
  const vis = Math.min(r.bottom, fondo) - Math.max(r.top, 0);
  return vis >= 200 && vis < d.h ? vis : d.h;
}
function vistaAdatta() {
  const d = vpDim();
  const l = L.lay;
  const hv = altoVisibile(d);
  const k = clamp(Math.min((d.w - 24) / l.W, (hv - 24) / l.H), KMIN, 1);
  // in alto, come la pagina di prima (il prodotto sempre in vista, anche in un riquadro più alto della parte visibile)
  return { k, tx: (d.w - l.W * k) / 2, ty: 12 };
}
// vistaIniziale: all'apertura l'albero adattato al riquadro che si vede (F3-I5 [T]); solo un albero così grande che
// adattato non si leggerebbe più (sotto il 35 %) si apre al 60 % sul prodotto, come prima
function vistaIniziale() {
  const a = vistaAdatta();
  if (a.k >= 0.35) return { k: a.k, tx: a.tx, ty: 12 };
  const d = vpDim();
  const k = 0.6;
  const r = L.lay.pos[L.g.radice] || { x: 0 };
  return { k, tx: d.w / 2 - (r.x + CW / 2) * k, ty: 12 };
}
function zoomA(k2, cx, cy) {
  const c = cam();
  if (!c) return;
  k2 = clamp(k2, KMIN, KMAX);
  const wx = (cx - c.tx) / c.k, wy = (cy - c.ty) / c.k;
  setCam({ k: k2, tx: cx - wx * k2, ty: cy - wy * k2 });
}
function zoomCentro(f) {
  if (!L) return;
  const d = vpDim();
  zoomA(cam().k * f, d.w / 2, d.h / 2);
}
function centraSel() {
  if (!L || !S.nodo || !L.lay.pos[S.nodo]) return;
  const d = vpDim();
  const ps = L.lay.pos[S.nodo];
  const k = Math.max(cam().k, 0.8);
  setCam({ k, tx: d.w / 2 - (ps.x + CW / 2) * k, ty: d.h / 2 - (ps.y + CH / 2) * k });
}
function puoTrascinare(k) {
  return !!L && L.g.modo === "bozza" && k !== L.g.radice && !!L.g.nodi[k] && !L.g.nodi[k].noninc && puoModificare() && S.vista === "org";
}
function vpDown(e) {
  if (!L) return;
  if (e.pointerType === "mouse" && e.button !== 0) return;
  V.punti.set(e.pointerId, { x: e.clientX, y: e.clientY });
  if (V.punti.size === 2) {
    V.pan = null;
    V.drag = null;
    const pp = Array.from(V.punti.values());
    const r = L.vp.getBoundingClientRect();
    V.pinch = { d0: Math.hypot(pp[0].x - pp[1].x, pp[0].y - pp[1].y) || 1, c0: Object.assign({}, cam()), cx: (pp[0].x + pp[1].x) / 2 - r.left, cy: (pp[0].y + pp[1].y) / 2 - r.top };
    return;
  }
  const card = e.target.closest(".du-nc");
  if (card && e.pointerType === "mouse") {
    if (e.target.closest(".du-nc-sel")) return;
    if (puoTrascinare(card.dataset.k)) {
      // niente selezione del testo della pagina mentre si trascina (il clic resta: lo dà il browser dopo pointerup)
      e.preventDefault();
      V.drag = { k: card.dataset.k, x0: e.clientX, y0: e.clientY, attivo: false, card };
    }
    return;
  }
  if (!card && e.target.closest("button, input, select, a")) return;
  // trascinare lo sfondo non seleziona il testo della pagina; la tela prende il fuoco per i tasti (frecce, più, meno, 0)
  e.preventDefault();
  L.vp.focus({ preventScroll: true });
  V.pan = { x0: e.clientX, y0: e.clientY, c0: Object.assign({}, cam()), mosso: false };
  L.vp.classList.add("du-vp--pan");
}
function vpMove(e) {
  if (V.punti.has(e.pointerId)) V.punti.set(e.pointerId, { x: e.clientX, y: e.clientY });
  if (V.pinch && V.punti.size >= 2 && L) {
    const pp = Array.from(V.punti.values());
    const r = L.vp.getBoundingClientRect();
    const d = Math.hypot(pp[0].x - pp[1].x, pp[0].y - pp[1].y) || 1;
    const c0 = V.pinch.c0;
    const k2 = clamp(c0.k * d / V.pinch.d0, KMIN, KMAX);
    const wx = (V.pinch.cx - c0.tx) / c0.k, wy = (V.pinch.cy - c0.ty) / c0.k;
    const mx = (pp[0].x + pp[1].x) / 2 - r.left, my = (pp[0].y + pp[1].y) / 2 - r.top;
    setCam({ k: k2, tx: mx - wx * k2, ty: my - wy * k2 });
    V.ultimoMov = Date.now();
    return;
  }
  if (V.pan && L) {
    const dx = e.clientX - V.pan.x0, dy = e.clientY - V.pan.y0;
    if (Math.abs(dx) + Math.abs(dy) > 4) V.pan.mosso = true;
    if (V.pan.mosso) setCam({ k: V.pan.c0.k, tx: V.pan.c0.tx + dx, ty: V.pan.c0.ty + dy });
    return;
  }
  if (V.drag) {
    const d = V.drag;
    if (!d.attivo && Math.hypot(e.clientX - d.x0, e.clientY - d.y0) > 6) {
      d.attivo = true;
      d.card.classList.add("du-nc--trascinato");
    }
    if (!d.attivo) return;
    const t = schedaVicina(e.clientX, e.clientY, d.k);
    if (V.target && V.target !== t) V.target.classList.remove("du-nc--drop", "du-nc--nodrop");
    V.target = t;
    const no = V.target ? vietatoSotto(d.k, V.target.dataset.k) : "";
    if (V.target) {
      V.target.classList.toggle("du-nc--drop", !no);
      V.target.classList.toggle("du-nc--nodrop", !!no);
    }
    fantasma(d, e, V.target ? (no ? "Non qui: " + no : "Sposta sotto " + nomeScheda(V.target.dataset.k)) : "Sposta " + nomeScheda(d.k) + ": rilascia sulla scheda del nuovo padre");
  }
}
// schedaVicina: la scheda sotto il puntatore durante un trascinamento, con un margine attorno al bordo (messaggio 46, D36:
// il rilascio sul bordo della scheda andava a vuoto); se le schede vicine sono più d'una, la più vicina. Le posizioni sono
// quelle dell'impaginazione, nella camera di adesso.
const TOLLERANZA = 24;
function schedaVicina(cx, cy, escludi) {
  if (!L || !cam()) return null;
  const c = cam();
  const r = L.vp.getBoundingClientRect();
  if (cx < r.left || cx > r.right || cy < r.top || cy > r.bottom) return null;
  const wx = (cx - r.left - c.tx) / c.k, wy = (cy - r.top - c.ty) / c.k;
  const tol = TOLLERANZA / c.k;
  let scelta = "", distanza = Infinity;
  for (const k of Object.keys(L.lay.pos)) {
    if (k === escludi) continue;
    const p = L.lay.pos[k];
    const d = Math.hypot(Math.max(p.x - wx, 0, wx - p.x - CW), Math.max(p.y - wy, 0, wy - p.y - CH));
    if (d <= tol && d < distanza) {
      scelta = k;
      distanza = d;
    }
  }
  return scelta ? document.getElementById("nc-" + sid(scelta)) : null;
}
function nomeScheda(k) {
  const v = L && L.g.nodi[k];
  return (v && (v.cod || v.desc)) || "il pezzo";
}
// fantasma: l'etichetta che segue il puntatore durante il trascinamento: che cosa succede se si rilascia lì
function fantasma(d, e, testo) {
  if (!d.fantasma) {
    d.fantasma = h("div", { class: "du-fantasma", id: "du-fantasma", "aria-hidden": "true" });
    const radice = $("#du");
    if (radice) radice.append(d.fantasma);
  }
  d.fantasma.textContent = testo;
  d.fantasma.style.left = e.clientX + 16 + "px";
  d.fantasma.style.top = e.clientY + 14 + "px";
}
function vpUp(e, annullato) {
  V.punti.delete(e.pointerId);
  if (V.pinch && V.punti.size < 2) {
    V.pinch = null;
    V.ultimoMov = Date.now();
  }
  if (V.pan) {
    if (V.pan.mosso) V.ultimoMov = Date.now();
    V.pan = null;
    if (L) L.vp.classList.remove("du-vp--pan");
  }
  if (V.drag) {
    const d = V.drag;
    V.drag = null;
    d.card.classList.remove("du-nc--trascinato");
    const t = V.target;
    V.target = null;
    if (t) t.classList.remove("du-nc--drop", "du-nc--nodrop");
    if (d.fantasma) d.fantasma.remove();
    if (!d.attivo || annullato) return;
    V.ultimoMov = Date.now();
    if (!t) return avvisa("no", "Per spostare un pezzo rilascialo sulla scheda del nuovo padre: niente è cambiato.");
    const no = vietatoSotto(d.k, t.dataset.k);
    if (no) return avvisa("no", "Lo spostamento non si fa: " + no);
    conRitorno("oc-" + sid(d.k), () => apriSposta(d.k, t.dataset.k));
  }
}
function vpWheel(e) {
  if (!L) return;
  e.preventDefault();
  const r = L.vp.getBoundingClientRect();
  const unit = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? r.height : 1;
  if (e.ctrlKey || e.metaKey) {
    zoomA(cam().k * Math.exp(-e.deltaY * unit * 0.0016), e.clientX - r.left, e.clientY - r.top);
    return;
  }
  const c = cam();
  setCam({ k: c.k, tx: c.tx - (e.shiftKey && !e.deltaX ? e.deltaY : e.deltaX) * unit, ty: c.ty - (e.shiftKey && !e.deltaX ? 0 : e.deltaY) * unit });
}
function vpKey(e) {
  if (!L || e.target !== L.vp) return;
  const c = cam();
  const m = { ArrowLeft: [60, 0], ArrowRight: [-60, 0], ArrowUp: [0, 60], ArrowDown: [0, -60] }[e.key];
  if (m) {
    e.preventDefault();
    setCam({ k: c.k, tx: c.tx + m[0], ty: c.ty + m[1] });
  } else if (e.key === "+" || e.key === "=") {
    e.preventDefault();
    zoomCentro(1.25);
  } else if (e.key === "-") {
    e.preventDefault();
    zoomCentro(0.8);
  } else if (e.key === "0") {
    e.preventDefault();
    setCam(vistaAdatta());
  }
}
// vietatoSotto: perché il pezzo k non va sotto «verso» (presentazione del rifiuto prima del dialogo; la regola che
// decide resta nel server: R4 dà CICLO ed E37_NON_CONTENITORE).
function vietatoSotto(k, verso) {
  const g = L ? L.g : grafoBozza();
  const v = g.nodi[verso];
  if (!v) return "il pezzo di arrivo non c'è.";
  if (k === verso) return "è lo stesso pezzo.";
  if (k === g.radice) return "il prodotto della RFQ non va sotto un altro pezzo.";
  if (v.noninc || v.escluso) return "il pezzo di arrivo è fuori dalla BOM nella bozza.";
  if (!contenitore(v)) return (v.cod || "il pezzo") + " è " + (v.tipo === "commerciale" ? "un commerciale" : "un particolare") + ": non ha figli. Se deve averne, cambia prima il suo tipo in Assieme.";
  if (discendenti(g, k).has(verso)) return (v.cod || "il pezzo") + " sta già sotto il pezzo che sposti (ciclo).";
  return "";
}

// ---------------------------------------------------------------- le anteprime dei 2D nei nodi (messaggio 46 [U])

// disegnoDelNodo: il 2D da mostrare nella scheda e nel pannello del nodo, come nella pagina di prima (il primario del
// motore, «nessun 2D associato» senza), con le decisioni della bozza davanti: il PDF preassociato al nodo nella bozza, poi
// il primario del motore per la casella o per il componente (disegniDelProdotto, solo con l'anteprima disponibile), poi
// il PDF associato al componente registrato, poi il PDF che il registro propone per la voce. Solo presentazione: quale
// documento vale lo decide il server alla conferma. {aid, nome, chiuso} o null; null vuol dire che il pezzo un 2D non
// ce l'ha.
function disegnoDelNodo(v) {
  if (!v) return null;
  const d = primoDisegnoDelNodo(v);
  if (!d) return null;
  // i byte che S5 non darebbe (azioni.contenuto.disponibile = false di S1: scartato, non nello staging) non si chiedono;
  // il 2D però c'è, e la scheda lo dice con il motivo di S1, mai «nessun 2D associato» (revisione della corsia ALBERO,
  // rilievo 2 [T])
  const f = fileDi(d.aid);
  const ct = f && f.azioni && f.azioni.contenuto;
  if (ct && ct.disponibile === false) d.chiuso = ct.motivo || "il contenuto del file non si apre su questo server";
  return d;
}
function primoDisegnoDelNodo(v) {
  const pdf = (aid) => {
    const f = fileDi(aid);
    return !!f && ePdf(f) && (!f.stato || f.stato === "attivo");
  };
  if (S.lavoro && v.nodo && !v.noninc) {
    const pb = arr(S.lavoro.documenti_preassociati).find((d) => d.chiave_bozza === v.k && pdf(d.allegato_id));
    if (pb) return { aid: String(pb.allegato_id).toLowerCase(), nome: nomeFile(pb.allegato_id) };
  }
  const p = prodottoScelto();
  const m = (p && p.disegni) || {};
  const c = v.nodo ? casellaDi(v.nodo) : v.decisione ? v.decisione.casella : null;
  const cid = v.nodo && v.nodo.componente_id ? String(v.nodo.componente_id).toLowerCase() : "";
  const dm = (c && m["r:" + c.rif]) || (cid && m["c:" + cid]);
  // il primario del motore vale solo se il file è attivo nella Sorgente (o S1 non lo elenca): uno scartato o messo in
  // Altri file non è più il 2D del pezzo, e lascia il posto al 2D attivo che viene dopo (seconda revisione della corsia
  // ALBERO, rilievo A [T]); i byte non si controllano qui (lo fa disegnoDelNodo, «anteprima non disponibile»)
  const fdm = dm ? fileDi(dm.aid) : null;
  if (dm && (!fdm || !fdm.stato || fdm.stato === "attivo")) return { aid: dm.aid, nome: dm.nome || nomeFile(dm.aid) };
  if (cid && S.lavoro) {
    for (const [aid, ar] of associazioniRegistrate()) if (ar.componente_id === cid && pdf(aid)) return { aid: String(aid).toLowerCase(), nome: nomeFile(aid) };
  }
  for (const a of arr(v.voce && v.voce.associazioni_proposte)) if (pdf(a.allegato_id)) return { aid: String(a.allegato_id).toLowerCase(), nome: nomeFile(a.allegato_id) };
  return null;
}
// miniatura: lo spazio dell'anteprima: la pagina 1 del PDF, disegnata da pdf.js dall'indirizzo di S5 che costruisce il
// ponte (come il visore), una volta per file e tenuta in memoria per la pagina; «nessun 2D associato» senza un 2D.
// «grande»: quella del pannello del nodo, che apre il visore.
function miniatura(v, grande) {
  const d = disegnoDelNodo(v);
  const cls = "du-nc-mini" + (grande ? " du-nc-mini--grande" : "");
  if (!d) return h("span", { class: cls + " du-nc-mini--na", id: grande ? "du-nodo-2d" : null, "data-du-stato": "nessuno" }, grande ? "nessun 2D associato al pezzo" : "nessun 2D associato");
  // il 2D c'è ma i suoi byte su questo server non si aprono (S1): lo si dice con il nome del file e il motivo, che sta nel
  // suggerimento della scheda e per intero nel pannello
  if (d.chiuso) return h("span", { class: cls + " du-nc-mini--na du-nc-mini--chiuso", id: grande ? "du-nodo-2d" : null, "data-du-stato": "chiuso", "data-du-mini-chiuso": d.aid,
    title: (d.nome || "2D") + ": anteprima non disponibile · " + d.chiuso },
  h("span", { class: "du-nc-mini-nome" }, d.nome || "2D"), h("span", {}, grande ? "anteprima non disponibile: " + d.chiuso : "anteprima non disponibile"));
  const el = h(grande ? "button" : "span", { class: cls, id: grande ? "du-nodo-2d" : null, type: grande ? "button" : null, "data-du-mini": d.aid,
    title: (d.nome || "2D") + (grande ? ": apri nel visore" : ""), "aria-label": grande ? "Apri il 2D " + (d.nome || "") + " nel visore" : null, "aria-hidden": grande ? null : "true" });
  if (grande) el.addEventListener("click", () => apriVisore(fileDi(d.aid) || { allegato_id: d.aid, nome_file: d.nome || "2D.pdf", estensione: "pdf" }));
  riempiMini(el, d.aid);
  return el;
}
const MINI = { cache: new Map(), fallite: new Map(), coda: [], inCorso: new Set(), attive: 0, lavoratore: null };
function riempiMini(el, aid) {
  const url = MINI.cache.get(aid);
  if (url) {
    el.classList.remove("du-nc-mini--na");
    el.replaceChildren(h("img", { src: url, alt: "", draggable: "false" }));
    el.dataset.duStato = "pronta";
    return;
  }
  const ko = MINI.fallite.get(aid);
  if (ko && Date.now() - ko.quando < 60000) {
    el.classList.add("du-nc-mini--na");
    el.textContent = "anteprima non disponibile";
    el.title = ko.testo;
    el.dataset.duStato = "errore";
    return;
  }
  el.dataset.duStato = "attesa";
  el.textContent = "anteprima…";
  if (!MINI.coda.includes(aid) && !MINI.inCorso.has(aid)) MINI.coda.push(aid);
  setTimeout(lavoraMini, 0);
}
// lavoraMini: al più due PDF insieme, nell'ordine in cui le schede li chiedono (dalla radice in giù)
function lavoraMini() {
  while (MINI.attive < 2 && MINI.coda.length) {
    const aid = MINI.coda.shift();
    if (MINI.cache.has(aid) || MINI.inCorso.has(aid)) continue;
    if (!document.querySelector("[data-du-mini='" + aid + "']")) continue;
    MINI.attive++;
    MINI.inCorso.add(aid);
    disegnaMini(aid).finally(() => {
      MINI.inCorso.delete(aid);
      MINI.attive--;
      lavoraMini();
    });
  }
}
async function disegnaMini(aid) {
  let task = null;
  try {
    const p = await pdfjs();
    if (!p) throw new Error("il visore dei PDF non si carica");
    if (!MINI.lavoratore) MINI.lavoratore = new p.lib.PDFWorker({ name: "du-miniature" });
    task = p.lib.getDocument({ url: P.urlContenuto(aid, "anteprima"), worker: MINI.lavoratore, isEvalSupported: false, enableXfa: false, verbosity: 0, wasmUrl: p.cartella + "wasm/",
      iccUrl: p.cartella + "iccs/", cMapUrl: p.cartella + "cmaps/", cMapPacked: true, standardFontDataUrl: p.cartella + "standard_fonts/" });
    const doc = await task.promise;
    const pag = await doc.getPage(1);
    const v1 = pag.getViewport({ scale: 1 });
    const viewport = pag.getViewport({ scale: Math.min(640 / v1.width, 452 / v1.height) });
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.ceil(viewport.width));
    canvas.height = Math.max(1, Math.ceil(viewport.height));
    await pag.render({ canvas, viewport, annotationMode: p.lib.AnnotationMode ? p.lib.AnnotationMode.DISABLE : undefined }).promise;
    // un data: URL, non un blob: la pagina non fa richieste oltre a quelle del ponte e di pdf.js (DU-B34)
    MINI.cache.set(aid, canvas.toDataURL("image/png"));
  } catch (e) {
    MINI.fallite.set(aid, { quando: Date.now(), testo: "anteprima non disponibile: " + testoErrore(e) });
  } finally {
    if (task) task.destroy().catch(() => {});
  }
  document.querySelectorAll("[data-du-mini='" + aid + "']").forEach((el) => riempiMini(el, aid));
}

// ---------------------------------------------------------------- il menu del nodo (messaggio 46, D38)

// apriMenuNodo: il menu dei gesti del pezzo k, dove si è fatto clic destro: gli stessi gesti del pannello (vociGesti),
// spenti con il motivo se oggi non si fanno, più «Apri il 2D». Sceglie il pezzo; si chiude con Esc, con un clic fuori o
// con un gesto.
function apriMenuNodo(k, x, y) {
  chiudiMenuNodo();
  if (S.nodo !== k) scegli(k);
  const g = L ? L.g : null;
  const v = g && g.nodi[k];
  const radice = $("#du");
  if (!v || !radice) return;
  const voci = g.modo === "bozza" ? vociGesti(g, v) : [];
  const d = disegnoDelNodo(v);
  if (d && !d.chiuso) voci.unshift({ id: "du-menu-2d", testo: "Apri il 2D", fai: () => apriVisore(fileDi(d.aid) || { allegato_id: d.aid, nome_file: d.nome || "2D.pdf", estensione: "pdf" }) });
  voci.unshift({ id: "du-menu-pannello", testo: "Mostra nel pannello", fai: () => {
    S.zona = "nodo";
    render();
    const pn = $("#du-pannello");
    if (pn) pn.scrollIntoView({ block: "nearest" });
  } });
  const menu = h("div", { class: "du-menu-nodo", id: "du-menu-nodo", role: "menu", "aria-label": "Gesti di " + (v.cod || v.desc || "il pezzo") },
    h("div", { class: "du-menu-nodo-t", "aria-hidden": "true" }, v.cod || v.desc || "(senza codice)"),
    voci.map((o) => h("button", { type: "button", role: "menuitem", class: "du-menu-voce" + (o.cls ? " " + o.cls : ""), id: o.id ? o.id + "-menu" : null, disabled: !!o.no, title: o.no || null,
      onclick: () => {
        chiudiMenuNodo();
        conRitorno("oc-" + sid(k), o.fai);
      } }, o.testo, o.no ? h("span", { class: "du-perche" }, o.no) : null)));
  radice.append(menu);
  const r = menu.getBoundingClientRect();
  menu.style.left = Math.max(4, Math.min(x, window.innerWidth - r.width - 8)) + "px";
  menu.style.top = Math.max(4, Math.min(y, window.innerHeight - r.height - 8)) + "px";
  const prima = menu.querySelector("button:not([disabled])");
  if (prima) prima.focus();
  menu.addEventListener("keydown", (e) => {
    const tutte = [...menu.querySelectorAll("button:not([disabled])")];
    const i = tutte.indexOf(document.activeElement);
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const j = (i + (e.key === "ArrowDown" ? 1 : tutte.length - 1)) % tutte.length;
      if (tutte[j]) tutte[j].focus();
    } else if (e.key === "Escape" || e.key === "Tab") {
      e.preventDefault();
      e.stopPropagation();
      chiudiMenuNodo();
      const c = document.getElementById("oc-" + sid(k));
      if (c) c.focus({ preventScroll: true });
    }
  });
  setTimeout(() => {
    document.addEventListener("pointerdown", fuoriDalMenu, true);
    window.addEventListener("resize", chiudiMenuNodo);
    window.addEventListener("wheel", chiudiMenuNodo, { passive: true });
  }, 0);
}
function fuoriDalMenu(e) {
  const m = $("#du-menu-nodo");
  if (m && !m.contains(e.target)) chiudiMenuNodo();
}
function chiudiMenuNodo() {
  const m = $("#du-menu-nodo");
  if (m) m.remove();
  document.removeEventListener("pointerdown", fuoriDalMenu, true);
  window.removeEventListener("resize", chiudiMenuNodo);
  window.removeEventListener("wheel", chiudiMenuNodo);
}

// ---------------------------------------------------------------- render

function render() {
  avvisoScaduto();
  renderTestaAzioni();
  renderProdotti();
  renderNav();
  renderStrisce();
  renderCorpo();
  renderBarra();
  scriviIndirizzo();
}
function renderTestaAzioni() {
  const box = $("#du-testa-azioni");
  if (!box) return;
  const f8 = S.modello && S.modello.f8 === "errore";
  const menu = h("details", { class: "du-menu", id: "du-menu-rfq" },
    h("summary", { class: "du-btn du-btn--sm" }, "Azioni della RFQ"),
    h("div", { class: "du-zona", style: "position:absolute;z-index:30;padding:8px;display:grid;gap:6px;max-width:min(420px,90vw)" },
      h("div", { class: "du-gesto" }, btn("Prepara i file", () => atto("prepara", {}, "", "Prepara i file"), { cls: "du-btn--sm", id: "du-prepara", perche: false }),
        h("span", { class: "du-perche" }, "Mette in coda scaricamento, estrazione e analisi dei file che non le hanno ancora (al più 20 per volta). Non rilegge gli STEP già analizzati: per quelli c'è «Rifai l'analisi».")),
      h("div", { class: "du-gesto" }, btn("Rifai l'analisi", () => atto("rianalizza", {}, "", "Rifai l'analisi"), { cls: "du-btn--sm", id: "du-rianalizza", perche: false }),
        h("span", { class: "du-perche" }, "Rilegge gli STEP che hanno già i fatti correnti e accoda gli altri, compresi gli STEP con la struttura da rileggere (LS-01) e i PDF letti da un analizzatore precedente.")),
      h("div", { class: "du-gesto" }, btn("Riprova le copie sul NAS", () => atto("riprovaCopie", {}, "", "Riprova le copie sul NAS"), { cls: "du-btn--sm", id: "du-riprova-copie", perche: false })),
      f8 ? h("div", { class: "du-gesto" }, btn("Riallinea i documenti", riallineaDocumenti, { cls: "du-btn--sm", id: "du-riallinea-documenti", perche: false }),
        h("span", { class: "du-perche" }, "Il riallineamento dei documenti dopo l'ultima conferma è in errore (F8): rifallo.")) : null,
      btn("Riepilogo RFQ", apriRiepilogoRfq, { cls: "du-btn--sm du-btn--ghost", id: "du-riepilogo-rfq", perche: false }),
      btn("Limiti di questa versione", apriLimiti, { cls: "du-btn--sm du-btn--ghost", id: "du-limiti", perche: false })));
  box.replaceChildren(btn("↻ Aggiorna", aggiorna, { cls: "du-btn--sm", id: "du-aggiorna", perche: false }), h("span", { style: "position:relative" }, menu));
}
function renderProdotti() {
  const box = $("#du-prodotti");
  if (!box) return;
  box.replaceChildren();
  if (!S.modello || S.erroreModello) return;
  for (const p of S.modello.prodotti) {
    box.append(h("button", { type: "button", class: "du-ptab", "aria-current": p.id === S.prodotto ? "true" : "false", "data-prodotto": p.id,
      onclick: () => scegliProdotto(p.id, { daTab: true }) }, h("span", { class: "du-mono" }, p.codice || p.rif), p.descrizione ? h("span", { class: "du-muto du-piccolo" }, p.descrizione) : null));
  }
}
async function scegliProdotto(id, o) {
  if (id === S.prodotto) return;
  // le modifiche tenute da parte valgono solo per il prodotto e la bozza su cui sono nate: la scelta si fa prima del
  // cambio di prodotto (decisioni dell'ondata A, §5 n. 2). Il prodotto non cambia finché non sono riapplicate o
  // scartate; il dialogo offre di scartarle e cambiare
  if (S.daParte && S.daParte.prodotto === S.prodotto) return chiediPrimaDiCambiare(id);
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "Il prodotto non cambia: le modifiche di questo prodotto non sono salvate.");
  }
  S.prodotto = id;
  S.nodo = "";
  S.anteprima = null;
  S.anteprimaAllegato = "";
  S.anteprimaGruppo = "";
  // il prodotto scelto dalle linguette in alto porta con sé la cartella del prodotto della Sorgente (messaggio 46, difetto
  // D21 [T]): la testata e il corpo parlano dello stesso prodotto; una cartella di un altro prodotto (la sua sezione, i
  // file che attendono la sua sorgente, i suoi file da ricollocare) passa a quella di questo
  // Collaudo UI46, giro 2 (C2-I6): con il tab, anche da una cartella di tutti i file («Tutti i file», «STEP della
  // RFQ»…) il corpo restava sull'altro prodotto (il dettaglio di un suo STEP, con «Revoca come sorgente di» quello). Il
  // tab porta la Sorgente nella cartella del prodotto scelto: il dettaglio è un file di quel prodotto, o nessuno [T]. Una
  // ricerca in corso resta (la cartella dei risultati vale per tutti i prodotti). Gli altri cambi di prodotto (un gesto su
  // una candidatura, una cartella) restano dove sono, tranne le cartelle di un altro prodotto
  const k = SG.cartella || "";
  if (o && o.daTab && k !== "ris") SG.cartella = "p:" + id;
  else if (k.indexOf("p:") === 0 && k.slice(2) !== id) SG.cartella = "p:" + id;
  else if ((k.indexOf("dc-att:") === 0 && k.slice(7) !== id) || (k.indexOf("dc-ric:") === 0 && k.slice(7) !== id)) SG.cartella = "dc";
  if (SG.cartella !== k) SG.torna = "";
  await caricaModello();
  await Promise.all([caricaBozza(), caricaStorico()]);
  render();
}
// chiediPrimaDiCambiare: il cambio di prodotto con modifiche tenute da parte (decisioni dell'ondata A, §5 n. 2). Le
// modifiche portano le chiavi dei nodi di questo prodotto: non passano a un altro. O si resta e si decide dalla
// striscia («Riapplica alla bozza riletta», «Scartale…»), o si scartano qui e il prodotto cambia.
function chiediPrimaDiCambiare(id) {
  const dp = S.daParte;
  const altro = (S.modello && S.modello.prodotti.find((p) => p.id === id)) || null;
  const nomeAltro = altro ? altro.codice || altro.rif : "l'altro prodotto";
  apriFinestra("Prima di passare a " + nomeAltro, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, dp.voci.length + " modifiche non salvate di " + (dp.codice || "questo prodotto") + " sono tenute da parte: valgono solo per la sua bozza e non passano a " + nomeAltro + "."),
    h("ul", { class: "du-elenco-p", id: "f-da-parte-cambio" }, dp.voci.map((v) => h("li", {}, v.etichetta))),
    h("p", { class: "du-nota" }, "Resta e decidi dalla striscia («Riapplica alla bozza riletta» o «Scartale…»), oppure scartale qui: si perdono, e il prodotto cambia.")),
  [{ testo: "Resta su " + (dp.codice || "questo prodotto"), chiudi: true }, { testo: "Scartale e passa a " + nomeAltro, danger: true, id: "f-ok", fai: async () => {
    if (S.daParte === dp) S.daParte = null;
    avvisa("ok", "Modifiche non salvate di " + (dp.codice || "questo prodotto") + " scartate.");
    chiudiFinestra();
    await scegliProdotto(id);
  } }]);
}
// chipSorgente: la sorgente del prodotto scelto (direttiva 39 §2): confermata da S8 e valida, oppure da confermare (con il
// perché: revocata, superata…, o il legame storico da riconfermare); e lo STEP della working, che cambia solo con la
// conferma della BOM (R5). Nessun «autorizzato»: nella pagina nuova l'autorizzazione non è più un gesto (D-TAB-02 B rivista).
function chipSorgente() {
  const f = S.modello && S.modello.fonte;
  if (!f) return pill("neu", "fonte: non letta");
  const s8 = ogg(f.sorgente_s8) ? f.sorgente_s8 : null;
  const t = s8 && s8.valida ? "sorgente confermata" : s8 ? "sorgente " + testoMotivoSorgente(s8.motivo) : ogg(f.legame_storico) ? "legame storico: da confermare" : "sorgente da confermare";
  return h("span", { class: "du-ptab-r" }, pill(s8 && s8.valida ? "ok" : "ver", t),
    pill(f.autorizzata ? "ok" : "neu", f.autorizzata ? "STEP della BOM confermata" : "nessuno STEP nella BOM confermata"));
}
// i motivi di una conferma di S8 che non vale più (confermabom.SorgenteValida; /dati.fonte.sorgente_s8.motivo)
const MOTIVI_SORGENTE = { revocata: "revocata", rifiutata: "rifiutata", superata: "superata", non_coerente: "non coerente con il documento",
  non_confermata: "non confermata", senza_sorgente: "assente" };
function testoMotivoSorgente(m) {
  return MOTIVI_SORGENTE[m] || "non valida" + (m ? " (" + m + ")" : "");
}
function chipRevisione() {
  if (S.erroreBozza) return pill("ko", "bozza: lettura non riuscita");
  // senza bozza la Revisione mostra la BOM confermata, se c'è (messaggio 47 [U]): il chip dice quella, non «nessuna bozza»
  if (!bozzaAperta()) {
    const u = ultimaConferma();
    return u ? pill(u.valida ? "ok" : "ver", "BOM confermata · v" + u.numero) : pill("neu", "nessuna bozza");
  }
  if (bozzaDaConvertire()) return pill("ver", "bozza da convertire");
  const base = S.salvato && S.salvato.versione_bom_base;
  const n = S.coda.length + (S.invio ? S.invio.voci.length : 0);
  // una bozza da rivedere è lo stato atteso, non un errore: niente colore d'allarme (messaggio 46, D35)
  return pill("info", "bozza " + (base && base.numero ? "su v" + base.numero : "da rivedere") + " · versione " + S.bozza.versione + (n ? " · " + n + " da salvare" : ""));
}
function renderNav() {
  const box = $("#du-nav");
  if (!box) return;
  const noRev = !S.prodotto ? "nessun prodotto valutato" : "";
  box.replaceChildren(
    h("div", { class: "du-passi", role: "group", "aria-label": "Passi" },
      h("button", { type: "button", class: "du-passo", id: "du-passo-sorgente", "aria-current": S.passo === "sorgente" ? "step" : null, onclick: () => vaiPasso("sorgente") },
        h("span", { class: "du-passo-t" }, h("span", { class: "du-passo-n" }, "1"), "Sorgente"), chipSorgente()),
      h("button", { type: "button", class: "du-passo", id: "du-passo-revisione", "aria-current": S.passo === "revisione" ? "step" : null, disabled: !!noRev, title: noRev || null,
        onclick: () => vaiPasso("revisione") }, h("span", { class: "du-passo-t" }, h("span", { class: "du-passo-n" }, "2"), "Revisione"), chipRevisione())),
    h("div", { class: "du-seg", role: "group", "aria-label": "Viste" },
      h("button", { type: "button", id: "du-vista-org", "aria-pressed": S.vista === "org" ? "true" : "false", onclick: () => vaiVista("org") }, "Organigramma"),
      h("button", { type: "button", id: "du-vista-elenco", "aria-pressed": S.vista === "elenco" ? "true" : "false", onclick: () => vaiVista("elenco") }, "Elenco")));
}
function vaiPasso(p) {
  S.passo = p;
  S.nodo = "";
  render();
}
function vaiVista(v) {
  S.vista = v;
  render();
}

// le strisce: errori del modello, bozza da convertire, base superata, proposta cambiata, rif scaduti, sospesi (MAT §5)
function renderStrisce() {
  const box = $("#du-strisce");
  if (!box) return;
  box.replaceChildren();
  const add = (tono, testo, extra, id) => box.append(h("div", { class: "du-striscia du-striscia--" + tono, id: id || null, role: tono === "ko" ? "alert" : "status" }, h("span", {}, testo), extra || null));
  if (S.erroreModello) add("ko", "Il calcolo del motore non è riuscito: " + S.erroreModello + " Nessun esito valido: la pagina non mostra tela, bozza né wizard finché il calcolo non riesce. «↻ Aggiorna» lo rifà.", null, "du-errore-modello");
  if (S.erroreBozza && S.passo === "revisione") add("ko", "Lettura non riuscita: la bozza non si legge (" + S.erroreBozza + "). ↻ Aggiorna per riprovare.", null, "du-errore-bozza");
  if (S.daRileggere) add("ver", S.daRileggere, btn("↻ Rileggi", rileggiTutto, { cls: "du-btn--sm du-btn--pri", id: "du-rileggi", perche: false }), "du-da-rileggere");
  // la BOM congelata (MAT TS-11, §6.1): modifiche spente; «Chiedi una revisione autorizzata…» visibile e spento, con il motivo
  if (S.modello && !S.erroreModello && S.modello.congelata) add("info", "BOM congelata: le modifiche della bozza sono spente.",
    btn("Chiedi una revisione autorizzata…", null, { cls: "du-btn--sm", id: "du-chiedi-revisione", no: "Revisione e congelamento non sono in questa pagina" }), "du-congelata");
  // il contesto partner dichiarato e non applicato (U2 §4.7; [U] Q24): una striscia nella Sorgente (e nel wizard), mai
  // «contesto applicato» finché le regole del contesto non ci sono
  const ctx = testoContestoNonApplicato();
  if (ctx && S.passo === "sorgente") add("ver", ctx, null, "du-contesto-non-applicato");
  const sosp = P.sospesi();
  for (const s of sosp) {
    const file = String(s.risorsa).indexOf("file:") === 0;
    const etichetta = "«" + (s.etichetta || s.nome) + "»" + (file ? " su " + fileDelSospeso(s).nome_file : "");
    if (s.certo) {
      // un esito certo da ritentare con la stessa chiave (409 COPIA_IN_CORSO): niente è registrato e niente è in volo, non
      // c'è niente da verificare (MAT §5.2, «copia in corso»; §5.4, S3)
      const riprovaC = s.nome === "confermaBom" && s.risorsa === "bozza:" + S.prodotto && bozzaLavorabile();
      const s8 = s.nome === "confermaSorgente";
      add("ver", etichetta + ": " + (s8 ? "la sorgente è confermata ma la bozza non è ancora nata (il motore era occupato). Riprova con lo stesso gesto: la stessa chiave riprende dal passo che manca." :
        (s.nome === "confermaBom" ? (s.codice === "CALCOLO_OCCUPATO" ? "il server era occupato e la conferma non è partita" : "una copia sul NAS è in corso") : "una copia di conservazione è già in corso per questo file") + ", niente è registrato. Riprova fra poco con la stessa chiave."),
        h("span", { class: "du-azioni" },
          riprovaC ? btn("Riprova la conferma…", riprovaConfermaSospesa, { cls: "du-btn--sm du-btn--pri", id: "du-riprova-sospesa", perche: false }) : null,
          s8 && ogg(s.corpo) ? btn("Riprova la conferma della sorgente", () => riprovaS8(s), { cls: "du-btn--sm du-btn--pri", id: "du-riprova-" + sid(s.risorsa), perche: false }) : null,
          file && ogg(s.corpo) ? btn("Riprova «" + (s.etichetta || s.nome) + "»", () => riprovaGestoFile(s), { cls: "du-btn--sm du-btn--pri", id: "du-riprova-" + sid(s.risorsa), perche: false }) : null,
          btn("Lascia perdere…", () => abbandonaTentativo(s), { cls: "du-btn--sm du-btn--ghost", perche: false })), "du-sospeso-" + sid(s.risorsa));
      continue;
    }
    // la conferma di questo prodotto verificata «non registrata»: «Riprova la conferma» riapre il wizard, che rilegge il
    // riepilogo e manda lo stesso corpo; il ponte riusa la chiave se il corpo è lo stesso (MAT §5.2; §5.5 n. 4)
    const riprova = s.nome === "confermaBom" && s.verificato && s.risorsa === "bozza:" + S.prodotto && bozzaLavorabile();
    // un gesto di un file verificato «non registrato»: il ponte ha il corpo di allora, e «Riprova» lo rimanda uguale
    const gf = file && s.verificato && ogg(s.corpo);
    const stato = s.verificato ? ": il server dice che non è ancora registrato." : S.verifiche.has(s.chiave) ? ": lo verifico con la stessa chiave." : ": «Verifica ora» lo chiede al server con la stessa chiave.";
    add("ver", "Un tentativo (" + etichetta + ") non ha un esito certo" + stato,
      h("span", { class: "du-azioni" }, btn("Verifica ora", () => verificaSospeso(s), { cls: "du-btn--sm", id: "du-verifica-" + sid(s.risorsa), perche: false }),
        riprova ? btn("Riprova la conferma…", riprovaConfermaSospesa, { cls: "du-btn--sm du-btn--pri", id: "du-riprova-sospesa", perche: false }) : null,
        gf ? btn("Riprova «" + (s.etichetta || s.nome) + "»", () => riprovaGestoFile(s), { cls: "du-btn--sm du-btn--pri", id: "du-riprova-" + sid(s.risorsa), perche: false }) : null,
        s.nome === "confermaSorgente" && s.verificato && ogg(s.corpo) ? btn("Riprova la conferma della sorgente", () => riprovaS8(s), { cls: "du-btn--sm du-btn--pri", id: "du-riprova-" + sid(s.risorsa), perche: false }) : null,
        s.nome === "revocaSorgente" && s.verificato && ogg(s.corpo) ? btn("Riprova la revoca della sorgente", () => riprovaRevocaSorgente(s), { cls: "du-btn--sm du-btn--pri", id: "du-riprova-" + sid(s.risorsa), perche: false }) : null,
        btn("Abbandona il tentativo…", () => abbandonaTentativo(s), { cls: "du-btn--sm du-btn--ghost", perche: false })), "du-sospeso-" + sid(s.risorsa));
  }
  const dp = rigaDaParte();
  if (dp) box.append(dp);
  if (!bozzaAperta()) return;
  if (bozzaDaConvertire()) {
    const noF = perchéNoFormaBozza();
    add("ver", "Bozza nel formato di prima: per lavorarla va convertita (la bozza vecchia si chiude, la nuova parte dalla stessa struttura).",
      h("span", { class: "du-azioni" }, btn("Converti…", apriConverti, { cls: "du-btn--sm du-btn--pri", id: "du-converti", no: noF }),
        btn("Abbandona la bozza…", apriAbbandona, { cls: "du-btn--sm", id: "du-abbandona-f2", no: noF })), "du-formato-vecchio");
    return;
  }
  const b = S.bozza;
  if (b.base && b.base.stato === "superata") {
    add("ver", "La BOM o lo STEP sono cambiati dopo la tua base (" + arr(b.base.motivi).join(", ") + "): serve il rebase. La bozza si salva comunque; la conferma no.",
      btn("Fai il rebase…", apriRebase, { cls: "du-btn--sm", id: "du-rebase", no: perchéNoModifica() }), "du-base-superata");
  }
  if (arr(b.rif_scaduti).length) add("ver", "Riferimenti scaduti nella bozza: " + arr(b.rif_scaduti).map((r) => (r.chiave_bozza || r.decisione || "?") + " (" + r.motivo + ")").join("; ") + ".", null, "du-rif-scaduti");
  const sp = strisciaProposta();
  if (sp) add(sp.tono, sp.testo, sp.extra, "du-proposta-cambiata");
  const uf = S.ultimaForma;
  if (uf) {
    const conta = (t, x) => (arr(x).length ? t + ": " + arr(x).length : "");
    const parti = [conta("identità aggiornate", uf.identita_aggiornate), conta("da rivedere", uf.da_rivedere), conta("caselle riscritte", uf.caselle_riscritte),
      conta("archi riscritti", uf.archi_riscritti), conta("decisioni cadute", uf.decisioni_cadute), conta("nodi usciti", uf.fuori_per_cambio_fonte),
      conta("voci nuove proposte (non entrano da sole)", uf.nuovi_nodi_proposti), conta("decisioni scritte dal server", uf.decisioni_scritte)].filter(Boolean);
    if (parti.length) add("info", "Il server ha costruito la versione nuova della bozza: " + parti.join("; ") + ".", btn("Chiudi", () => {
      S.ultimaForma = null;
      renderStrisce();
    }, { cls: "du-btn--sm du-btn--ghost", perche: false }), "du-ultima-forma");
  }
}
// testoContestoNonApplicato: il testo di U2 §4.7 per le diagnostiche contesto.non_valido del thread (/dati) o della
// testata del registro della bozza (proposta.diagnostiche, U2-P49): «‹id›» è il riferimento della diagnostica (rif), il
// motivo il suo messaggio. "" se non ce ne sono.
function testoContestoNonApplicato() {
  const reg = registroBozza();
  const diag = arr(S.modello && S.modello.contestoNonValido).concat(arr(reg && reg.proposta && reg.proposta.diagnostiche).filter((d) => d && d.codice === "contesto.non_valido"));
  if (!diag.length) return "";
  const d = diag[0];
  const id = arr(d.rif)[0] || "";
  return "Il contesto partner" + (id ? " " + id : "") + " dichiarato per questa RFQ non è applicato" + (d.messaggio ? " (" + d.messaggio + ")" : "") + ": le regole del partner non sono state usate.";
}
function strisciaProposta() {
  const b = S.bozza;
  const bp = S.modello && S.modello.bozzaDelProdotto;
  if (!b || !b.importazione || !bp || bp.bozza_id !== b.bozza_id || !ogg(bp.gruppo_corrente)) return null;
  const g = bp.gruppo_corrente;
  const prodotto = codiceProdotto();
  if (g.come === "stessa_occorrenza") {
    const gr = S.modello.gruppi.find((x) => x.chiave === g.chiave);
    if (!gr || !gr.impronta_contenuto || gr.impronta_contenuto === b.importazione.impronta_contenuto) return null;
    return { tono: "ver", testo: "La proposta del Motore A è cambiata dopo l'importazione.",
      extra: h("span", { class: "du-azioni" }, btn("Vedi le differenze", apriDifferenze, { cls: "du-btn--sm", id: "du-differenze", perche: false, no: S.confrontoNo ? CONFRONTO_NON_V1 : "" }),
        btn("Aggiorna proposta…", apriRiallinea, { cls: "du-btn--sm", id: "du-riallinea", no: perchéNoAggiorna() })) };
  }
  if (g.come === "altra_occorrenza") {
    return { tono: "info", testo: "La BOM di lavoro di " + prodotto + " sta su un'altra copia dello stesso file (" + nomeFile(g.allegato_id) + "): la bozza resta valida sulla sua copia; per seguirla, «Usa nuovo STEP…».",
      extra: btn("Guarda l'altra copia", () => {
        S.passo = "sorgente";
        guarda(g.allegato_id, "bom");
      }, { cls: "du-btn--sm", perche: false }) };
  }
  if (g.come === "assente") {
    // il testo dipende dallo stato del file (RIC-I7; DEC-B B-3): in Altri file «prima riattiva il file», con «Riattiva»;
    // scartato «è stato scartato: per seguirla ricaricalo», mai «Riattiva» (lo scarto non si presenta come reversibile,
    // Q20 [U])
    if (g.motivo === "occorrenza_non_attiva") {
      const f = fileDi(g.allegato_id);
      const stato = (f && f.stato) || "";
      if (stato === "scartato") return { tono: "info", testo: "La struttura della bozza di " + prodotto + " sta su un file che è stato scartato (" + nomeFile(g.allegato_id) + "): la bozza resta valida; per seguirla ricaricalo." };
      return { tono: "info", testo: "La struttura della bozza di " + prodotto + " sta su un file che non è attivo (" + nomeFile(g.allegato_id) + (stato === "altri_file" ? ", in Altri file" : "") + "): la bozza resta valida; per seguirla, prima riattiva il file.",
        extra: f && stato === "altri_file" ? btn("Riattiva…", () => apriRiattiva(f), { cls: "du-btn--sm", id: "du-riattiva-sorgente", no: azioneFile(f, "riattiva").ok ? "" : azioneFile(f, "riattiva").motivo }) : null };
    }
    return { tono: "info", testo: "Il Motore A non propone più questa struttura per " + prodotto + ": la bozza resta valida." };
  }
  return null;
}

// UI46 [U] (controllo di stabilità del 21/10: «quando si clicca un box o bottone la UI fa dei refresh che spostano la
// pagina»): renderCorpo ricrea tutto #du-corpo, e i riquadri che scorrono nascevano in cima, il fuoco finiva sul body,
// i <details> aperti si chiudevano. Come nel wizard (vistaWizardPrima/Dopo) [T]: lo scorrimento di ogni riquadro si
// conserva finché resta la stessa vista di quel riquadro (la chiave scritta in data-du-vista quando è nato: la
// cartella e la ricerca per l'elenco, il file per il dettaglio, il nodo e la linguetta per il pannello del nodo); una
// vista diversa (altra cartella, altro file, altro nodo, altra linguetta) riparte dall'inizio. Tornano il fuoco (con la
// selezione del testo nei campi) e i <details> aperti che hanno un id. I gesti che spostano il fuoco di proposito
// (fuocoSuRiga, l'esito di S8) lo fanno dopo renderCorpo e restano come sono. Oltre allo scorrimento si tiene ferma
// un'ancora (la prima riga o sezione con un id in vista nel riquadro): se sopra compare o sparisce qualcosa (la barra
// delle azioni di gruppo alla prima spunta, una striscia nel pannello dopo un salvataggio), l'ancora resta dov'era.
const SCORREVOLI_CORPO = ["#du-sg-albero", "#du-sg-elenco", "#du-sg-dettaglio", ".du-zona--nodo", ".du-segnalati-c"];
const ANCORE_CORPO = { "#du-sg-elenco": "[role=row][id]", "#du-sg-albero": "[id]", "#du-sg-dettaglio": "[id]", ".du-zona--nodo": "[id]", ".du-segnalati-c": "" };
function ancoraIn(el, sel) {
  if (!sel || !el.scrollTop) return null;
  const b = el.getBoundingClientRect();
  for (const x of el.querySelectorAll(sel)) {
    const r = x.getBoundingClientRect();
    if (r.height && r.top >= b.top - 1 && r.top < b.bottom) return { id: x.id, dy: r.top - b.top };
  }
  return null;
}
function chiaveScorrevole(sel) {
  const sg = "sorgente|";
  if (sel === "#du-sg-albero") return sg;
  if (sel === "#du-sg-elenco") return sg + SG.cartella + "|" + SG.cerca;
  if (sel === "#du-sg-dettaglio") return sg + SG.cartella + "|" + (SG.memo[SG.cartella] ? SG.memo[SG.cartella].file || "" : "") + "|" + (S.anteprima ? S.anteprimaAllegato + "|" + S.anteprimaGruppo + "|" + (SG.larga ? 1 : 0) : "");
  if (sel === ".du-zona--nodo") return S.passo + "|" + S.vista + "|" + (S.nodo || "") + "|" + S.linguettaNodo + "|" + (S.passo === "sorgente" ? S.anteprimaAllegato + "|" + S.anteprimaGruppo : "");
  return S.passo + "|" + S.vista;
}
function vistaCorpoPrima(box) {
  const scorri = {};
  for (const sel of SCORREVOLI_CORPO) {
    const el = box.querySelector(sel);
    if (el && el.dataset.duVista !== undefined) scorri[sel] = { chiave: el.dataset.duVista, top: el.scrollTop, ancora: ancoraIn(el, ANCORE_CORPO[sel]) };
  }
  const a = document.activeElement;
  let fuoco = null;
  // un <summary> senza id: si ricorda il <details> che lo contiene, se ha un id
  const det = a && a.tagName === "SUMMARY" && !a.id && a.parentElement && a.parentElement.tagName === "DETAILS" && a.parentElement.id ? a.parentElement.id : "";
  if (a && a !== box && box.contains(a) && (a.id || det)) {
    fuoco = { id: a.id, summaryDi: det, valore: null, da: null, a: null, riquadro: "", chiave: "" };
    // il riquadro con la sua vista: se la vista cambia (un altro nodo, un altro file) il fuoco non torna
    for (const sel of SCORREVOLI_CORPO) {
      const el = box.querySelector(sel);
      if (el && el.contains(a) && el.dataset.duVista !== undefined) {
        fuoco.riquadro = sel;
        fuoco.chiave = el.dataset.duVista;
        break;
      }
    }
    if (a.tagName === "INPUT" || a.tagName === "TEXTAREA") {
      fuoco.valore = a.value;
      try {
        fuoco.da = a.selectionStart;
        fuoco.a = a.selectionEnd;
      } catch (e) {
        fuoco.da = null;
      }
    }
  }
  return { scorri, fuoco, aperti: Array.from(box.querySelectorAll("details[open][id]"), (d) => d.id) };
}
function vistaCorpoDopo(box, v) {
  for (const id of v.aperti) {
    const d = document.getElementById(id);
    if (d && box.contains(d)) d.open = true;
  }
  // lo spazio di lavoro della Sorgente prende l'altezza subito (non al frame dopo): senza, i riquadri non scorrono ancora
  if (S.passo === "sorgente" && $("#du-sg")) adattaAltezzaSg();
  for (const sel of SCORREVOLI_CORPO) {
    const el = box.querySelector(sel);
    if (!el) continue;
    const chiave = chiaveScorrevole(sel);
    el.dataset.duVista = chiave;
    const p = v.scorri[sel];
    if (!p || p.chiave !== chiave || !p.top) continue;
    el.scrollTop = p.top;
    const x = p.ancora && document.getElementById(p.ancora.id);
    if (x && el.contains(x)) {
      const d = x.getBoundingClientRect().top - el.getBoundingClientRect().top - p.ancora.dy;
      if (Math.abs(d) > 1) el.scrollTop += d;
    }
  }
  const f = v.fuoco;
  if (!f) return;
  if (f.riquadro && chiaveScorrevole(f.riquadro) !== f.chiave) return;
  const d = f.summaryDi && document.getElementById(f.summaryDi);
  const el = f.summaryDi ? d && d.querySelector(":scope > summary") : document.getElementById(f.id);
  if (!el || !box.contains(el) || el === document.activeElement) return;
  // la ricerca della Sorgente: il testo scritto negli ultimi istanti non è ancora in SG.cerca (lo prende il debounce)
  if (f.id === "du-sg-cerca" && f.valore !== null && el.value !== f.valore && f.valore !== SG.cerca) el.value = f.valore;
  el.focus({ preventScroll: true });
  if (f.da !== null && f.valore !== null && el.value === f.valore) {
    try {
      el.setSelectionRange(f.da, f.a);
    } catch (e) {
      // i campi senza selezione (numeri, caselle) non la ripristinano
    }
  }
}
function renderCorpo() {
  const box = $("#du-corpo");
  if (!box) return;
  box.removeAttribute("aria-busy");
  // messaggio 46: dopo un gesto l'impaginazione cambia (un pezzo spostato, un tipo cambiato) e le schede si spostano; la
  // vista resta ferma sul pezzo scelto (o sul prodotto), che rimane dov'era sullo schermo
  let ancora = null;
  if (L && L.lay && cam()) {
    const k = S.nodo && L.lay.pos[S.nodo] ? S.nodo : L.g.radice;
    const ps = k && L.lay.pos[k];
    const c = cam();
    if (ps) ancora = { chiave: L.chiave, k, sx: ps.x * c.k + c.tx, sy: ps.y * c.k + c.ty };
  }
  L = null;
  const vista = vistaCorpoPrima(box);
  box.replaceChildren(S.passo === "revisione" ? corpoRevisione() : corpoSorgente());
  vistaCorpoDopo(box, vista);
  // la tela dell'anteprima prende la sua altezza prima della camera, che la misura (C2-I4)
  if (L && S.passo === "sorgente" && $("#du-sg-dettaglio #du-vp")) adattaAltezzaSg();
  if (L) {
    const c = cam();
    const ps = ancora && ancora.chiave === L.chiave ? L.lay.pos[ancora.k] : null;
    if (!c) setCam(vistaIniziale());
    else if (ps) setCam({ k: c.k, tx: ancora.sx - ps.x * c.k, ty: ancora.sy - ps.y * c.k });
    else applicaCam();
  }
}

// ---------------------------------------------------------------- Sorgente (mockup v5, direzione A)

// Il tab «1 Sorgente» è lo spazio di lavoro dei file della RFQ, senza scegliere prima un prodotto (U2 §2.1, X-01 [U]),
// in un tab che non fa scorrere la pagina ([U] M32), con l'esploratore a tre riquadri della direzione A ([U] M33, versione
// 5.1 approvata per disposizione e azioni ammesse: [U] M34, UX-S3): a sinistra le cartelle con i conti, al centro
// l'elenco della cartella, a destra il dettaglio del file con le sole azioni valide (spente con la ragione). Testi e
// regole delle mosse vengono dal manuale del mockup (07-frontend/mockup-v5-sorgente, LEGGIMI §2-§6); le funzioni sono
// quelle della v4.2 e dei contratti (S1, S6, S8, S9; EM §10.9; U2 §2-§3).
//
// I dati: S1 (i file, le loro azioni {disponibile, motivo} e i contatori), S9 (/motore-a/dati?sorgente=1: le candidate
// degli STEP, lo smistamento di ogni prodotto con le azioni delle righe, i file da associare con il motivo, i file in
// attesa della sorgente), S6 (l'anteprima di uno STEP per un prodotto) e /dati del prodotto scelto (fonte, gruppi).
// Nessun conto che il server non dia: i conti dell'albero sono le lunghezze degli elenchi del server.
//
// SG: lo stato di presentazione della Sorgente (mai uno stato del server): la cartella aperta, la memoria di ogni
// cartella (file attivo, spunte), la ricerca, la vista sotto i 600 px, l'ultima mossa dell'utente e i messaggi del
// sistema.
const SG = {
  cartella: "",
  memo: {},
  cerca: "",
  vista: "cartelle", // sotto i 600 px: "cartelle" | "elenco" | "dettaglio"
  mossa: null, // l'ultima mossa dell'utente: {testo, esito, allegato, cartella, passo, fantasma, contiPrima}
  sistema: [], // i messaggi del sistema, il più recente in testa: {testo, allegato, quando}
  registroAperto: false,
  esitiS8: {}, // l'esito dell'ultima «Conferma questo STEP come sorgente», per prodotto
  inCorsoS8: {}, // la conferma della sorgente in corso, per prodotto
  larga: true, // l'anteprima di uno STEP nel dettaglio allargato
  torna: "", // la cartella da cui si è saltati («‹ Torna a …»)
  apertiProdotto: {}, // l'intestazione del prodotto aperta (true) o chiusa (false), per cartella
  firmaS1: "",
  caricaAperto: false,
};

// caricaSmistamento: S9 (U2 §2.3). Si rilegge all'apertura, dopo S8, dopo i gesti dei file e della bozza, quando cambia
// la firma di S1 e con «↻ Aggiorna»; mai a intervalli fissi (costa come /dati). Una lettura non riuscita non è «nessun
// abbinamento»: la Sorgente lo dice e i conti che vengono da S9 diventano «—».
async function caricaSmistamento() {
  try {
    S.smist = await P.leggiSmistamento();
    S.erroreSmist = "";
  } catch (e) {
    S.smist = null;
    S.erroreSmist = testoErrore(e);
  }
}
// caricaSmistamentoPoi: la rilettura di S9 dopo un salvataggio della bozza, raggruppata (un salvataggio dopo l'altro
// non fa una lettura per ciascuno)
let timerSmist = null;
function caricaSmistamentoPoi() {
  clearTimeout(timerSmist);
  timerSmist = setTimeout(async () => {
    await caricaSmistamento();
    if (S.passo === "sorgente") renderCorpo();
  }, 600);
}

// ---- i dati delle cartelle

const IN_ANALISI = ["download_in_coda", "in_staging", "estrazione_in_coda", "in_analisi"];
function eStep(f) {
  return /^(stp|step)$/i.test((f && f.estensione) || "") || /\.(stp|step)$/i.test((f && f.nome_file) || "");
}
function eArchivio(f) {
  return /^(zip|7z)$/i.test((f && f.estensione) || "") || /\.(zip|7z)$/i.test((f && f.nome_file) || "");
}
function eAttivo(f) {
  return !!f && (!f.stato || f.stato === "attivo");
}
// preassociabile: un file che si può preassociare a un pezzo (direttiva 35, C-2; D-2 (c) [U], direttiva 39 §5): attivo e
// non scartato col vecchio «Scarta il file», né contenitore (estensione zip, 7z, rar, o con voci estratte), né lo STEP
// legato come sorgente di un prodotto finito; gli altri STEP (di un particolare o di un sottoassieme) si preassociano come
// un PDF. Una voce estratta da un archivio sì. S1 elenca solo file ed elementi di Outlook; R2 ricontrolla tutto
// (FILE_NON_ATTIVO, natura_non_preassociabile, legato_come_sorgente). La proposta di documento non serve.
function preassociabile(f) {
  return eAttivo(f) && !f.scartato_legacy && !(eStep(f) && stepSorgenteDelFinito(f)) && !eArchivio(f) && !/^rar$/i.test(f.estensione || "") && !arr(f.voci).length;
}
// stepSorgenteDelFinito: lo STEP che la pagina sa legato come sorgente di un prodotto finito (direttiva 39 §5; il predicato
// del server è allegato_sorgente_del_finito): la riga di S9 «legato come sorgente» o «legame storico», la sorgente S8 (anche
// revocata: nel dubbio), la fonte della working, la sorgente di una bozza aperta. Senza S9 letto la pagina non lo sa: nel
// dubbio ogni STEP vale come sorgente e non si offre fra i file da preassociare [T-39]; R2 resta la guardia.
// Anche uno STEP candidato di un prodotto finito (S9 step[].candidate) non si offre [T-39]: è uno STEP del finito, che si
// conferma come sorgente con S8; associato come documento diventerebbe una AR che poi impedisce a S8 di confermarlo
// (gia_associata_altrove, provato sul server vero). Gli STEP di particolari e sottoassiemi non hanno candidate per i finiti.
function stepSorgenteDelFinito(f) {
  if (!S.smist) return true;
  const a = String(f.allegato_id || "").toLowerCase();
  if (arr(S.smist.step).some((s) => String(s.allegato_id || "").toLowerCase() === a && arr(s.candidate).length)) return true;
  // G39-02 [U]: lo STEP proposto come sorgente di un finito (S9 prodotti[].fonte.proposte_sorgente[]) è un candidato
  // sorgente, mai un documento di un nodo (R2 lo rifiuta con STEP_CANDIDATO_DEL_FINITO, R4 lo blocca: la pagina non lo offre)
  if (arr(S.smist.prodotti).some((p) => ogg(p.fonte) && arr(p.fonte.proposte_sorgente).some((x) => ogg(x) && String(x.allegato_id || "").toLowerCase() === a))) return true;
  for (const p of arr(S.smist.prodotti)) {
    if (arr(p.righe).some((r) => ogg(r) && String(r.allegato_id || "").toLowerCase() === a && (r.stato === "legato_come_sorgente" || r.stato === "legame_storico"))) return true;
    const fo = ogg(p.fonte) ? p.fonte : null;
    if (fo && typeof fo.allegato_id === "string" && fo.allegato_id.toLowerCase() === a && (fo.confermata || fo.autorizzata)) return true;
  }
  for (const p of prodottiSg()) {
    const s8 = sorgenteS8(p.id);
    if (s8 && String(s8.allegato_id || "").toLowerCase() === a) return true;
    const sb = sorgenteDellaBozza(p.id);
    if (sb && sb.allegato_id === a) return true;
  }
  return false;
}
// ---- lo stato di uno STEP come sorgente dei prodotti finiti (messaggio 46 [U] punto 4; tabella degli stati [T])
// Un solo stato per prodotto, detto allo stesso modo nella riga, nel dettaglio e nell'intestazione del prodotto; mai
// «confermato» e «da confermare» (o «proposto») insieme:
//   proposto      lo STEP è candidato o proposto dal motore e il prodotto non ha una sorgente confermata su di lui:
//                 «◇ candidata per ‹P›» / il riquadro «STEP proposto come sorgente» con «Conferma questo STEP…»
//   confermata    la conferma di S8 su questo STEP vale: «◆ sorgente di ‹P›», nessun «proposto», nessun «✓ confermato»
//                 (il documento confermato è la conseguenza della sorgente, non un secondo stato)
//   revocata      l'ultima conferma di S8 su questo STEP è revocata: «sorgente revocata per ‹P›», con «resta un documento
//                 di ‹codice›» se il documento c'è; il riquadro dice la revoca e offre «Conferma di nuovo come sorgente»
//   sostituita    l'ultima conferma su questo STEP è superata da un'altra: «sostituita come sorgente di ‹P›» (+ documento)
//   non_valida    la conferma non vale per un altro motivo del server: «sorgente di ‹P› non valida (‹motivo›)»
//   (documento ordinario: uno STEP o un file confermato come documento e mai sorgente: «✓ confermato», come prima)
// statiSorgente: gli stati dello STEP per i prodotti che hanno su di lui l'ultima conferma di S8 (fonte.sorgente_s8)
function statiSorgente(allegato) {
  const a = String(allegato || "").toLowerCase();
  const out = [];
  if (!a) return out;
  for (const p of prodottiSg()) {
    const s8 = sorgenteS8(p.id);
    if (!s8 || String(s8.allegato_id || "").toLowerCase() !== a) continue;
    const stato = s8.valida ? "confermata" : s8.motivo === "revocata" ? "revocata" : s8.motivo === "superata" ? "sostituita" : "non_valida";
    out.push({ pid: p.id, codice: p.codice, stato, s8 });
  }
  return out;
}
function statoSorgentePer(allegato, pid) {
  return statiSorgente(allegato).find((x) => x.pid === pid) || null;
}
// pillStatoSorgente: la pillola dello stato (la tabella sopra); f per «resta un documento di ‹codice›»
function pillStatoSorgente(x, f) {
  const doc = f && f.documento ? pill("neu", "resta un documento di " + (f.documento.codice || x.codice), "il documento confermato non cambia con la revoca: cambia solo la sorgente della BOM") : null;
  if (x.stato === "confermata") return pill("ok", "◆ sorgente di " + x.codice, x.s8.confermata_il ? "confermata il " + dataOra(x.s8.confermata_il) : "sorgente confermata");
  if (x.stato === "revocata") return [pill("ver", "sorgente revocata per " + x.codice, "da confermare di nuovo, questo STEP o un altro"), doc];
  if (x.stato === "sostituita") return [pill("neu", "sostituita come sorgente di " + x.codice), doc];
  return [pill("ver", "sorgente di " + x.codice + " non valida" + (x.s8.motivo ? " (" + testoMotivoSorgente(x.s8.motivo) + ")" : "")), doc];
}
function inAnalisiF(f) {
  return IN_ANALISI.includes(f && f.analisi && f.analisi.stato);
}
function letto(f) {
  return !!(f && f.analisi && f.analisi.stato === "letto");
}
function formatoDi(f) {
  return eArchivio(f) ? String(f.estensione || "zip").toUpperCase() : "";
}
// i prodotti della RFQ: quelli del modello, con la sezione di S9 se c'è
function prodottiSg() {
  if (!S.modello || S.erroreModello) return [];
  return S.modello.prodotti.map((p) => ({ id: p.id, codice: p.codice || p.rif, rif: p.rif, descrizione: p.descrizione || "", s9: s9P(p.id) }));
}
function s9P(id) {
  return arr(S.smist && S.smist.prodotti).find((p) => String(p.prodotto_id || "").toLowerCase() === id) || null;
}
function codiceDi(id) {
  const p = s9P(id);
  if (p && p.codice) return p.codice;
  const m = S.modello && S.modello.prodotti.find((x) => x.id === id);
  return m ? m.codice || m.rif : id ? id.slice(0, 8) : "";
}
// haBozza: il prodotto ha una bozza aperta (S9; per il prodotto scelto, R1 se S9 non si legge)
function haBozza(id) {
  if (id === S.prodotto && S.bozza) return bozzaAperta();
  const p = s9P(id);
  return !!(p && p.bozza);
}
function nessunaBozza() {
  return !prodottiSg().some((p) => haBozza(p.id));
}
function fonteDi(id) {
  if (id === S.prodotto && S.modello && S.modello.fonte) return S.modello.fonte;
  const p = s9P(id);
  return p && ogg(p.fonte) ? p.fonte : null;
}
// lo stato di una riga dello smistamento: quello di S9 e, per il prodotto scelto, le modifiche della bozza non ancora
// rilette (il server lo ridice alla rilettura di S9 che segue ogni salvataggio)
const STATI_RIGA = { proposto: ["◇", "proposto", "prop"], salvato_nella_bozza: ["◈", "salvato nella bozza", "bozza"], gia_registrato: ["●", "già registrato", "info"],
  da_scollegare: ["⊘", "da scollegare alla conferma", "ver"], in_conflitto: ["!", "in conflitto", "ko"], tolto: ["◇", "tolto dalla bozza (da rileggere)", "neu"],
  // direttiva 39 §5 (S9): lo STEP sorgente del finito in sola lettura, e il legame storico univoco da confermare con S8
  da_ricollocare: ["↺", "da ricollocare", "ver"], legato_come_sorgente: ["◆", "sorgente confermata", "ok"], legame_storico: ["◇", "legame storico: da confermare come sorgente", "ver"] };
function statoRiga(r, id) {
  if (id !== S.prodotto || !bozzaLavorabile() || !S.lavoro) return r.stato;
  const k = r.destinazione && r.destinazione.chiave_bozza;
  const pb = pbDi(r.allegato_id);
  if (r.stato === "proposto" && pb) return "salvato_nella_bozza";
  if ((r.stato === "salvato_nella_bozza" || (r.stato === "in_conflitto" && r.origine === "bozza")) && !pb) return "tolto";
  if (r.stato === "gia_registrato" && svincoloDi(r.allegato_id)) return "da_scollegare";
  if (r.stato === "da_scollegare" && !svincoloDi(r.allegato_id)) return "gia_registrato";
  if (r.stato === "salvato_nella_bozza" && pb && k && pb.chiave_bozza !== k) return "salvato_nella_bozza";
  return r.stato;
}
// la chiave del nodo della riga: quella della preassociazione della bozza che si vede, se c'è, o quella del server
function chiaveRiga(r, id) {
  if (id === S.prodotto && S.lavoro) {
    const pb = pbDi(r.allegato_id);
    if (pb) return pb.chiave_bozza;
  }
  return (r.destinazione && r.destinazione.chiave_bozza) || "";
}
// le righe di un prodotto: quelle di S9 e, per il prodotto scelto, le preassociazioni della bozza che S9 non ha ancora
// (un «Associa a un nodo…» appena fatto), finché S9 non si rilegge
function righeDi(id) {
  const p = s9P(id);
  const out = arr(p && p.righe).map((r) => ({ f: fileDi(r.allegato_id), r, pid: id, stato: statoRiga(r, id), grp: "smist:" + id })).filter((x) => x.f);
  if (id === S.prodotto && bozzaLavorabile() && S.lavoro) {
    for (const d of S.lavoro.documenti_preassociati) {
      if (out.some((x) => x.r.allegato_id === d.allegato_id)) continue;
      const f = fileDi(d.allegato_id);
      if (!f) continue;
      const n = S.lavoro.nodi.find((x) => x.chiave_bozza === d.chiave_bozza);
      const r = { allegato_id: f.allegato_id, destinazione: { chiave_bozza: d.chiave_bozza }, codice_mostrato: (n && n.identita && n.identita.codice) || "",
        stato: "salvato_nella_bozza", origine: "bozza", condiviso_con: [], azioni: {}, locale: true };
      out.push({ f, r, pid: id, stato: "salvato_nella_bozza", grp: "smist:" + id });
    }
  }
  return out;
}
function collegamentiDi(f) {
  const out = [];
  for (const p of prodottiSg()) for (const x of righeDi(p.id)) if (x.f.allegato_id === f.allegato_id) out.push(x);
  return out;
}
const MOTIVI_DA_ASSOCIARE = { nessun_candidato: "nessun nodo candidato", associazione_ambigua: "più nodi candidati: associazione ambigua",
  associazione_discordante: "le letture del file non concordano", collocazione_non_determinabile: "collocazione non determinabile",
  fuori_richiesta_non_confermato: "fuori dalla richiesta, non confermato" };
function motivoDaAssociare(allegato) {
  const x = arr(S.smist && S.smist.da_associare).find((d) => d.allegato_id === allegato);
  return x ? MOTIVI_DA_ASSOCIARE[x.motivo] || x.motivo || "" : "";
}
// i prodotti senza bozza per cui il file attende la sorgente (U2 §2.3, la chip «attende la sorgente», G-6)
function attesaDi(allegato) {
  // D-AR11 B [U]: senza una sorgente confermata da S8 il motore non propone righe, anche con una bozza senza STEP aperta;
  // i file restano qui, e il server dice quali (S9 in_attesa_sorgente, motivo sorgente_non_confermata)
  return prodottiSg().filter((p) => p.s9 && !sorgenteValida(p.id) && arr(p.s9.in_attesa_sorgente).some((x) => x.allegato_id === allegato));
}
// le candidate di uno STEP (S9 step[].candidate; senza S9, i gruppi di /dati del prodotto scelto)
function candidateDi(allegato) {
  const st = arr(S.smist && S.smist.step).find((s) => s.allegato_id === allegato);
  if (st) return arr(st.candidate).map((c) => Object.assign({ allegato_id: allegato }, c, { prodotto_id: String(c.prodotto_id || "").toLowerCase() }));
  if (S.smist || !S.modello || S.erroreModello || !S.prodotto) return [];
  return gruppiDelloStep(allegato).map((g) => ({ allegato_id: allegato, prodotto_id: S.prodotto, gruppo: g.chiave, compatibilita: g.compatibilita, radice_letterale: g.radice_letterale,
    struttura: g.struttura, segnalati: g.segnalati, impronta_contenuto: g.impronta_contenuto }));
}
function candidatePer(id) {
  const out = [];
  for (const s of stepDellaRfq()) for (const c of candidateDi(s.allegato_id)) if (c.prodotto_id === id) out.push(c);
  return out;
}

// ---- lo STEP proposto come sorgente del prodotto finito (G39-02 [U], 20/10 sera)
// Lo STEP del prodotto finito non è un file ordinario: è un candidato sorgente, riconosciuto con il Motore A (il codice del
// prodotto della RFQ e il codice letto dal nome del file, con il marcatore: PM-01 «Z = marcatore»), proposto qui PRIMA di
// S8 («il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di ‹P›: lo confermi?») e registrato
// solo con S8 (fonte_step_evento); mai PB né AR ordinaria; senza S8 niente R5. La proposta la dice il server, con la
// forma della corsia motore (motore_a/proposta_sorgente.go, propostabom.CandidatureSorgenteDelFinito): S9
// prodotti[].fonte.proposte_sorgente[] e /dati fonte.proposte_sorgente[] {allegato_id, nome_file, codice_prodotto,
// evidenze[] {lettura, selettore, codice_letto, base_letta, marcatore_letto, esito_base, esito_marcatore}, sorgente_di?,
// testo}, solo le candidature univoche e solo prima di S8. Nessun ripiego della pagina [T-39, rilievo bloccante della
// revisione G39]: la regola è una, quella del server; senza proposte del server la pagina non ne mostra. sorgente_di dice
// che lo STEP è adesso la sorgente confermata di un altro prodotto: il gesto è «Sposta la sorgente su ‹P›…» (G39-03).
function proposteSorgente(id) {
  if (!id || sorgenteValida(id)) return [];
  const p = s9P(id);
  const out = [];
  const viste = new Set();
  for (const fo of [fonteDi(id), p && ogg(p.fonte) ? p.fonte : null]) {
    for (const x of arr(fo && fo.proposte_sorgente)) {
      if (!ogg(x) || typeof x.allegato_id !== "string" || !x.allegato_id) continue;
      const a = x.allegato_id.toLowerCase();
      if (viste.has(a)) continue;
      viste.add(a);
      const f = fileDi(a);
      if (!f || !eAttivo(f)) continue;
      const altro = x.sorgente_di && String(x.sorgente_di).toLowerCase() !== id ? String(x.sorgente_di).toLowerCase() : "";
      const c = Object.assign({}, candidateDi(a).find((y) => y.prodotto_id === id) || {}, { allegato_id: a, prodotto_id: id });
      if (altro) c.sorgente_di = altro;
      out.push({ c, f, altro, evidenze: arr(x.evidenze).filter(ogg), codice: typeof x.codice_prodotto === "string" && x.codice_prodotto ? x.codice_prodotto : codiceDi(id),
        testo: typeof x.testo === "string" ? x.testo : "" });
    }
  }
  return out;
}
// stepProposto: la proposta del server per il prodotto (la prima; con un allegato, quella di quello STEP), o null
function stepProposto(id, allegato) {
  const pp = proposteSorgente(id);
  if (!allegato) return pp[0] || null;
  const a = String(allegato).toLowerCase();
  return pp.find((x) => x.c.allegato_id === a) || null;
}
// le evidenze della lettura del Motore A, come le dice il server (evidenze[] della proposta): da dove viene il codice letto
// (il selettore), il codice, la base e il marcatore letti e l'esito del confronto con il prodotto. Solo presentazione.
function etichettaSelettore(s) {
  const x = String(s || "");
  if (x === "struttura_candidata") return "radice di una struttura candidata";
  if (x.indexOf("fonte.") === 0) return "candidato della fonte";
  if (x.indexOf("nome_file") === 0) return "codice letto dal nome del file";
  if (x.indexOf("archivio") >= 0) return "codice letto dalla voce d'archivio";
  if (x.indexOf("step") >= 0 || x.indexOf("radice") >= 0) return "codice letto dallo STEP";
  return "codice letto";
}
const ESITI_EVIDENZA = { uguale: "uguale", equivalente: "equivalente", compatibile: "compatibile", compatibile_parziale: "compatibile in parte", discordante: "diverso", assente: "assente",
  incerta: "incerta" };
function evidenzeProposta(sp, dove) {
  const li = [h("li", { "data-evidenza": "codice_prodotto" }, "codice del prodotto nella RFQ: ", h("span", { class: "du-mono" }, sp.codice))];
  for (const e of sp.evidenze) {
    const sel = typeof e.selettore === "string" ? e.selettore : "";
    // la struttura candidata e i candidati della fonte: il server dà la radice o una chiave interna del nodo come
    // codice_letto; una chiave (con «:», o un riferimento «#n» dello STEP) non si mostra (trovato sul server vero, giro G39)
    if (sel === "struttura_candidata" || sel.indexOf("fonte.") === 0) {
      const cod = typeof e.codice_letto === "string" && e.codice_letto && e.codice_letto.indexOf(":") < 0 && e.codice_letto[0] !== "#" ? e.codice_letto : "";
      li.push(h("li", { "data-evidenza": sel, "data-codice-letto": cod || null }, sel === "struttura_candidata" ? "una struttura candidata del prodotto ha la radice di questo STEP" : etichettaSelettore(sel),
        cod ? [": ", h("span", { class: "du-mono" }, cod)] : null, e.esito_base ? " · compatibilità: " + (ESITI_EVIDENZA[e.esito_base] || e.esito_base) : ""));
      continue;
    }
    const parti = [];
    if (e.base_letta && e.base_letta !== e.codice_letto) parti.push("base " + e.base_letta);
    parti.push(e.marcatore_letto ? "marcatore " + e.marcatore_letto : "nessun marcatore scritto");
    if (e.esito_base) parti.push("base: " + (ESITI_EVIDENZA[e.esito_base] || e.esito_base));
    if (e.esito_marcatore) parti.push("marcatore: " + (ESITI_EVIDENZA[e.esito_marcatore] || e.esito_marcatore));
    li.push(h("li", { "data-evidenza": sel || "lettura", "data-codice-letto": e.codice_letto || "", "data-marcatore": e.marcatore_letto || "" }, etichettaSelettore(sel) + ": ",
      h("span", { class: "du-mono" }, e.codice_letto || "?"), " · " + parti.join(" · ")));
  }
  if (sp.c.compatibilita) li.push(h("li", { "data-evidenza": "compatibilita" }, "compatibilità della struttura con il prodotto: " + sp.c.compatibilita));
  const st = ogg(sp.c.struttura) ? (sp.c.struttura.completa === false ? "struttura non completa" + (sp.c.struttura.motivo ? ": " + sp.c.struttura.motivo : "") : "struttura completa") : "";
  if (st) li.push(h("li", { "data-evidenza": "struttura" }, st + (typeof sp.c.segnalati === "number" ? " · nodi segnalati: " + sp.c.segnalati : "")));
  return h("ul", { class: "du-elenco-p du-piccolo", id: "du-step-proposto-evidenze" + (dove ? "-" + dove : "") }, li);
}
// boxStepProposto: la proposta, con le evidenze e i gesti: la conferma (S8, lo stesso gesto della candidatura, con i suoi
// motivi di spegnimento) e l'anteprima. dove distingue gli id (sg: cartella del prodotto, d: dettaglio dello STEP, rev:
// Revisione).
// Più proposte (più STEP univoci per lo stesso prodotto) danno più riquadri: il primo con gli id senza numero, gli altri
// con -2, -3… allegato: solo la proposta di quello STEP (il dettaglio dello STEP).
function boxStepProposto(id, dove, allegato) {
  const pp = allegato ? [stepProposto(id, allegato)].filter(Boolean) : proposteSorgente(id);
  if (!pp.length) return null;
  // lo STEP proposto che ha già su di sé la conferma di S8 di questo prodotto (revocata, sostituita): il riquadro dello
  // stato, non la domanda «lo confermi?» (messaggio 46 punto 4, D10)
  const box = pp.map((sp, i) => {
    const suf = (dove ? "-" + dove : "") + (i ? "-" + (i + 1) : "");
    const xs = statoSorgentePer(sp.c.allegato_id, id);
    return xs ? riquadroStatoSorgente(xs, sp.f, suf) : riquadroStepProposto(id, sp, suf);
  });
  return box.length === 1 ? box[0] : h("div", { style: "display:grid;gap:6px" }, box);
}
// sintesiProposta: la proposta in una riga (messaggio 46, D30): il codice letto, la compatibilità con il codice del
// prodotto, la struttura; il dettaglio delle letture sta nel «Perché lo propone» ripiegato
function sintesiProposta(sp) {
  const leggibile = (e) => e.codice_letto && String(e.codice_letto).indexOf(":") < 0 && String(e.codice_letto)[0] !== "#";
  const ev = sp.evidenze.find((e) => /step|radice/.test(String(e.selettore || "")) && leggibile(e)) || sp.evidenze.find(leggibile);
  const compat = compatCodice(sp);
  const st = ogg(sp.c.struttura) ? (sp.c.struttura.completa === false ? "struttura parziale" : "struttura completa") : "";
  // la candidata «incerta»: il perché in una riga, al posto del codice letto e della sua compatibilità (che, letti dal nome
  // del file, dicevano «marcatore uguale» accanto a «incerta»: collaudo UI46, giro 2, C2-I7) [T]
  const inc = perchéIncerta(sp.c, sp.evidenze, sp.codice);
  // F3-I4 (collaudo UI46, giro 3) [T]: il codice letto dal nome del file si dice tale, e lo STEP senza struttura per il
  // prodotto dice la radice che ha davvero (la 9170099A di uno STEP chiamato 9170001A_1.stp)
  const dal = ev && String(ev.selettore || "").indexOf("nome_file") === 0 ? "dal nome del file " : "";
  const rd = !sp.c.gruppo ? radiceLettaDiversa(sp.c.allegato_id, sp.codice) : "";
  const parti = [inc ? [h("span", { "data-du-incerta": sp.c.allegato_id || "" }, inc)] : ev ? ["codice letto " + dal, h("span", { class: "du-mono" }, codiceSenzaRev(ev)), compat ? " (" + compat + ")" : ""] : compat,
    rd ? [h("span", { "data-du-radice-letta": rd }, "la radice dello STEP è ", h("span", { class: "du-mono" }, rd), ", non ", h("span", { class: "du-mono" }, sp.codice))] : "", st,
    typeof sp.c.segnalati === "number" && sp.c.segnalati ? sp.c.segnalati + (sp.c.segnalati === 1 ? " pezzo da verificare" : " pezzi da verificare") : ""].filter((x) => x && x.length);
  const out = [];
  parti.forEach((x, i) => out.push(i ? " · " : "", x));
  return out;
}
// codiceSenzaRev: il codice letto senza la revisione che il nome del file porta in coda («9150001A_1» → «9150001A»), solo
// quando base e marcatore letti dal Motore A dicono che il resto è la revisione; altrimenti il codice com'è (C2-I7)
function codiceSenzaRev(e) {
  const cl = String((e && e.codice_letto) || "");
  const norm = (x) => String(x || "").toUpperCase().replace(/[^A-Z0-9]/g, "");
  const pre = norm(e && e.base_letta) + norm(e && e.marcatore_letto);
  const m = cl.match(/^(.*?)[\s._-]+(?:rev\.?\s*)?[A-Za-z0-9]{1,3}$/i);
  return pre && m && norm(m[1]) === pre && norm(cl) !== pre ? m[1] : cl;
}
// perchéIncerta: perché la candidatura di uno STEP è «incerta», in una frase per chi sceglie la sorgente: la radice dello
// STEP non porta il codice del prodotto (o ne porta un altro) e, se è così, il codice lo dice solo il nome del file. ""
// se la candidatura non è incerta (C2-I7) [T].
function perchéIncerta(c, evidenze, codice) {
  if (!c || c.compatibilita !== "incerta") return "";
  const cod = codice || codiceDi(c.prodotto_id);
  const rad = c.radice_letterale && c.radice_letterale.indexOf(":") < 0 && c.radice_letterale[0] !== "#" ? c.radice_letterale : "";
  const ev = arr(evidenze).length ? arr(evidenze) : arr(c.evidenze);
  const nome = ev.find((e) => ogg(e) && String(e.selettore || "").indexOf("nome_file") === 0 && e.codice_letto);
  return "incerta: la radice dello STEP " + (rad && rad !== cod ? "dice " + rad + ", non " + cod : "non porta il codice di " + cod) +
    (nome ? "; " + codiceSenzaRev(nome) + " lo dice solo il nome del file" : "") + ". Guarda l'anteprima prima di confermarlo.";
}
// riquadroStatoSorgente: lo STEP che ha su di sé l'ultima conferma di S8 del prodotto x.pid, con lo stato della tabella di
// statiSorgente e il gesto che serve in quello stato (messaggio 46 punto 4): confermata (nessun gesto qui: «Vai alla
// Revisione» e «Revoca come sorgente…» stanno fra le azioni del file), revocata («Conferma di nuovo come sorgente di ‹P›»
// e l'anteprima), sostituita o non valida (l'anteprima).
function riquadroStatoSorgente(x, f, suf) {
  suf = suf === undefined ? "-" + sid(x.pid) : suf;
  const cod = x.codice;
  const a = f ? f.allegato_id : String(x.s8.allegato_id || "").toLowerCase();
  const nome = (f && f.nome_file) || nomeFile(a);
  const doc = f && f.documento ? " Il file resta un documento di " + (f.documento.codice || cod) + ": cambia solo la sorgente della BOM." : "";
  const sp = stepProposto(x.pid, a);
  const c = (sp && sp.c && sp.c.gruppo ? sp.c : null) || candidateDi(a).find((y) => y.prodotto_id === x.pid) || { allegato_id: a, prodotto_id: x.pid, gruppo: "" };
  const az = azioniCandidatura(c);
  const conf = az.find((y) => y.id.indexOf("conferma-") === 0);
  const guarda = az.find((y) => y.id.indexOf("guarda-") === 0);
  let titolo = "";
  let testo = "";
  const gesti = [];
  if (x.stato === "confermata") {
    titolo = "Sorgente di " + cod;
    testo = "Confermata" + (x.s8.confermata_il ? " il " + dataOra(x.s8.confermata_il) : "") + (bomConfermataSu(x.pid, a) ? ". È lo STEP della BOM confermata." : ". Diventa lo STEP della BOM con «Conferma BOM».");
    // l'anteprima e «Aggiorna proposta…» (la bozza già su questa sorgente) della sua candidatura, con gli id di prima
    for (const y of az) gesti.push(bottoneAz(y, y.domId + "-d"));
  } else if (x.stato === "revocata") {
    titolo = "Sorgente revocata per " + cod;
    testo = "Questo STEP non è più la sorgente di " + cod + "." + doc + " Se la revoca era un errore, confermalo di nuovo; altrimenti conferma lo STEP giusto.";
    if (conf && c.gruppo) {
      const b = bottoneAz(Object.assign({}, conf, { t: "Conferma di nuovo come sorgente di " + cod }), "du-riconferma" + suf);
      if (!conf.no) (b.querySelector("button") || b).classList.add("du-btn--pri");
      gesti.push(b);
    }
  } else if (x.stato === "sostituita") {
    titolo = "Sostituita come sorgente di " + cod;
    // il server dice «superata» sia per un altro STEP confermato sia per il documento sostituito (stesso valore): un testo
    // che vale per tutti e due (rilievo della revisione della corsia)
    testo = "Questo STEP non è più la sorgente valida di " + cod + ": la sua conferma è stata superata." + doc;
  } else {
    titolo = "Sorgente di " + cod + " non valida";
    testo = "La conferma di questo STEP come sorgente non vale più (" + testoMotivoSorgente(x.s8.motivo) + ")." + doc;
  }
  if (x.stato !== "confermata" && guarda) gesti.push(bottoneAz(guarda, "du-guarda-stato" + suf));
  return h("div", { class: "du-striscia du-striscia--" + (x.stato === "confermata" ? "ok" : "ver") + " du-stato-sorgente", id: "du-stato-sorgente" + suf, "data-stato": x.stato, "data-allegato": a, "data-prodotto": x.pid },
    h("div", { style: "display:grid;gap:4px;min-width:0" }, h("span", {}, h("b", {}, titolo + ": "), h("span", { class: "du-mono" }, nome)), h("span", { class: "du-piccolo" }, testo)),
    gesti.length ? h("div", { class: "du-azioni" }, gesti) : null);
}
function riquadroStepProposto(id, sp, suf) {
  const cod = codiceDi(id);
  const az = azioniCandidatura(sp.c);
  const conf = az.find((a) => a.id.indexOf("conferma-") === 0);
  const guarda = az.find((a) => a.id.indexOf("guarda-") === 0);
  // nella Revisione, con l'anteprima di questo STEP aperta sotto, la conferma è quella dell'anteprima: un pulsante solo
  // (collaudo UI46, giro 3, minore) [T]
  const doppio = suf.indexOf("-rev") === 0 && anteprimaInRevisione() && S.anteprimaAllegato === sp.c.allegato_id;
  const bc = conf && !doppio ? bottoneAz(conf, "du-conferma-proposto" + suf) : null;
  if (bc && !conf.no) (bc.querySelector("button") || bc).classList.add("du-btn--pri");
  const domanda = sp.testo || "Il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di " + cod + ": lo confermi come sorgente?";
  return h("div", { class: "du-striscia du-striscia--info du-step-proposto", id: "du-step-proposto" + suf, "data-allegato": sp.c.allegato_id, "data-prodotto": id, "data-sorgente-di": sp.altro || null },
    h("div", { style: "display:grid;gap:4px;min-width:0" },
      h("span", {}, h("b", {}, "STEP proposto come sorgente: "), h("span", { class: "du-mono" }, sp.f.nome_file || nomeFile(sp.c.allegato_id))),
      h("span", { id: "du-step-proposto-domanda" + suf }, domanda),
      h("span", { class: "du-piccolo", id: "du-step-proposto-sintesi" + suf }, sintesiProposta(sp)),
      h("details", { class: "du-ripiega du-piccolo", id: "du-step-proposto-perche" + suf }, h("summary", {}, "Perché lo propone"), evidenzeProposta(sp, suf.slice(1))),
      sp.altro ? h("span", { class: "du-esito du-esito--ver", id: "du-step-proposto-altro" + suf }, "Adesso è la sorgente confermata di " + codiceDi(sp.altro) +
        ". Se quell'associazione è sbagliata, «Sposta la sorgente su " + cod + "…» la revoca là come «sorgente errata» e annulla la bozza di " + codiceDi(sp.altro) +
        " nata da questo STEP (la sua storia resta). Se " + codiceDi(sp.altro) + " ha già confermato la BOM con questo STEP, lo spostamento non si fa: si apre la sua revisione.") : null,
      // senza un gruppo importabile la bozza nasce dal solo prodotto (collaudo UI46, I3): il testo non promette una struttura
      h("span", { class: "du-nota" }, "Non è ancora una sorgente: lo diventa solo con la tua conferma, che crea la bozza " + (sp.c.gruppo ? "dalla sua struttura" : "con il solo prodotto: il motore non ne ha letto una struttura") +
        ". Senza uno STEP confermato non si arriva a «Conferma BOM».")),
    h("div", { class: "du-azioni" }, bc, guarda ? bottoneAz(guarda, "du-guarda-proposto" + suf) : null));
}
// avvisoStepProposto: nel dialogo di una partenza senza lo STEP (bozza manuale, «solo prodotto» da un altro STEP), lo STEP
// proposto e non confermato (G39-02 [U]: «molto sconsigliata»): l'avviso, la scelta di confermarlo prima e la presa d'atto
// per partire lo stesso [T-39: la spunta, mai spuntata]. escluso: lo STEP che il dialogo sta già confermando.
function avvisoStepProposto(id, escluso) {
  const sp = stepProposto(id);
  if (!sp) return avvisoStepCandidati(id, escluso);
  if (sp.c.allegato_id === String(escluso || "").toLowerCase()) return null;
  const cod = codiceDi(id);
  return h("div", { class: "du-esito du-esito--ko", id: "f-step-proposto", "data-allegato": sp.c.allegato_id },
    h("b", {}, "C'è uno STEP proposto come sorgente: è molto sconsigliato partire senza."),
    h("span", {}, " Il motore ha trovato che " + (sp.f.nome_file || nomeFile(sp.c.allegato_id)) + " è molto probabilmente lo STEP strutturale di " + cod +
      ". Confermato, la bozza nasce dalla sua struttura; senza, i pezzi e le quantità li scrivi a mano, e «Conferma BOM» resterà bloccata finché non confermi uno STEP come sorgente."),
    h("div", { class: "du-azioni" }, btn("Conferma prima lo STEP proposto…", () => {
      chiudiFinestra();
      conProdotto(id, () => apriConfermaSorgente(sp.c));
    }, { cls: "du-btn--sm du-btn--pri", id: "f-conferma-proposto", perche: false })),
    h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-senza-proposto" }), "Parto lo stesso senza lo STEP proposto."));
}
// avvisoStepCandidati: senza una proposta univoca, gli STEP che il Motore A riconosce come candidati sorgente del prodotto
// e non ancora confermati (R1 ?suggerimenti_motore=1, step_candidati[]: G39-02 [U], distintabozza/stepcandidati3.go),
// anche più d'uno: la bozza manuale resta «molto sconsigliata» (rilievo della revisione 2 della corsia). La lettura costa
// un calcolo del motore e arriva dopo l'apertura del dialogo: il posto c'è subito, l'avviso (con la sua presa d'atto, lo
// stesso id di avvisoStepProposto) entra quando R1 risponde. Finché la lettura è in corso il dialogo non parte
// (mancaPresaProposto: data-stato «in_lettura»), al più per ATTESA_STEP_CANDIDATI_MS: poi resta la nota «il motore non ha
// potuto controllare» e si parte; se R1 risponde dopo, l'avviso entra lo stesso (rilievo minore della revisione
// indipendente 3 della corsia: un clic veloce apriva la bozza manuale senza avviso). Un motore che non calcola o una
// lettura che non riesce lasciano una nota, mai un blocco: la regola vera la tengono R2, R4 e R5 del server.
const ATTESA_STEP_CANDIDATI_MS = 12000;
function avvisoStepCandidati(id, escluso) {
  const posto = h("div", { id: "f-step-candidati", "data-stato": "in_lettura" }, h("span", { class: "du-muto du-piccolo" }, "Controllo se il motore riconosce uno STEP sorgente di " + codiceDi(id) + "…"));
  const esc = String(escluso || "").toLowerCase();
  const aggiornaDialogo = () => {
    const fin = $("#du-finestra");
    if (fin && fin.__aggiorna && fin.contains(posto)) fin.__aggiorna();
  };
  const scadenza = setTimeout(() => {
    if (!posto.isConnected || posto.dataset.stato !== "in_lettura") return;
    posto.dataset.stato = "scaduto";
    posto.replaceChildren(h("span", { class: "du-nota" }, "Il motore non ha potuto controllare in tempo gli STEP candidati sorgente di " + codiceDi(id) + ": se ne trova uno, l'avviso compare qui."));
    aggiornaDialogo();
  }, ATTESA_STEP_CANDIDATI_MS);
  P.leggiBozza(id, { suggerimenti_motore: true }).then((j) => {
    clearTimeout(scadenza);
    if (!posto.isConnected) return;
    const err = j && ogg(j.suggerimenti_motore) ? j.suggerimenti_motore.errore : "";
    const l = arr(j && j.step_candidati).filter((x) => ogg(x) && typeof x.allegato_id === "string").map((x) => x.allegato_id.toLowerCase())
      .filter((a) => a !== esc && (!fileDi(a) || eAttivo(fileDi(a))));
    if (!l.length) {
      posto.dataset.stato = err ? "non_calcolato" : "nessuno";
      posto.replaceChildren(err ? h("span", { class: "du-nota" }, "Il motore non ha potuto controllare gli STEP candidati sorgente adesso (" + err + ").") : "");
      aggiornaDialogo();
      return;
    }
    const cod = codiceDi(id);
    posto.dataset.stato = "candidati";
    posto.replaceChildren(h("div", { class: "du-esito du-esito--ko", id: "f-step-proposto", "data-candidati": l.join(" ") },
      h("b", {}, l.length === 1 ? "C'è uno STEP che il motore riconosce come sorgente: è molto sconsigliato partire senza." : "Ci sono " + l.length + " STEP che il motore riconosce come sorgente: è molto sconsigliato partire senza."),
      h("span", {}, " Per " + cod + " il Motore A riconosce come candidati sorgente " + l.map((a) => (fileDi(a) ? fileDi(a).nome_file : nomeFile(a))).join(", ") +
        (l.length > 1 ? ", senza sceglierne uno" : "") + ". Guardali e conferma quello giusto dalla Sorgente: la bozza nasce dalla sua struttura. Senza, i pezzi e le quantità li scrivi a mano, e «Conferma BOM» resterà bloccata finché non confermi uno STEP come sorgente."),
      h("div", { class: "du-azioni" }, btn("Scegli lo STEP nella Sorgente", () => {
        chiudiFinestra();
        conProdotto(id, () => vaiPasso("sorgente"));
      }, { cls: "du-btn--sm du-btn--pri", id: "f-vai-sorgente", perche: false })),
      h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-senza-proposto" }), "Parto lo stesso senza uno STEP sorgente.")));
    aggiornaDialogo();
  }).catch((e) => {
    clearTimeout(scadenza);
    if (!posto.isConnected) return;
    posto.dataset.stato = "errore";
    posto.replaceChildren(h("span", { class: "du-nota" }, "Gli STEP candidati sorgente non si leggono adesso: " + testoErrore(e)));
    aggiornaDialogo();
  });
  return posto;
}
// la presa d'atto dell'avviso, se il dialogo lo mostra; con la lettura degli STEP candidati in corso il dialogo aspetta
// (avvisoStepCandidati, al più ATTESA_STEP_CANDIDATI_MS)
function mancaPresaProposto(fin) {
  if (fin && fin.querySelector("#f-step-candidati[data-stato='in_lettura']")) return "il controllo degli STEP candidati sorgente del motore (in corso)";
  return fin && fin.querySelector("#f-step-proposto") && !(fin.querySelector("#f-senza-proposto") || {}).checked ? "la scelta di partire senza lo STEP proposto" : "";
}
function gruppiDelloStep(allegato) {
  return arr(S.modello && S.modello.gruppi).filter((x) => x.allegato_id === allegato);
}
// gli STEP della RFQ: S9 step[] (attivi e letti), o S1 step[]
function stepDellaRfq() {
  const st = S.smist ? arr(S.smist.step) : arr(S.sorgente && S.sorgente.step);
  const out = (st.length ? st : fileSorgente().filter((f) => eStep(f) && eAttivo(f))).slice();
  // gli STEP che il motore propone come sorgente (o che sono già la sorgente) di un prodotto senza un gruppo importabile:
  // S9 step[] non li elenca, ma sono STEP della RFQ (collaudo UI46, I3: la stessa card diceva «STEP proposto» e «0 STEP»)
  const visti = new Set(out.map((x) => String(x.allegato_id || "").toLowerCase()));
  for (const a of stepSenzaGruppo()) if (!visti.has(a)) {
    visti.add(a);
    out.push({ allegato_id: a, nome_file: nomeFile(a), candidate: [] });
  }
  return out.filter((s) => {
    const f = fileDi(s.allegato_id);
    return !f || eAttivo(f);
  });
}
// stepSenzaGruppo: gli STEP proposti come sorgente (fonte.proposte_sorgente) o già sorgente (fonte.sorgente_s8) di un
// prodotto, da /dati e da S9, attivi; quelli con un gruppo del motore li dà già S9 step[]
function stepSenzaGruppo() {
  const out = [];
  const visti = new Set();
  const metti = (a) => {
    const x = typeof a === "string" ? a.toLowerCase() : "";
    const f = x ? fileDi(x) : null;
    if (!x || visti.has(x) || !f || !eAttivo(f) || !eStep(f)) return;
    visti.add(x);
    out.push(x);
  };
  for (const p of prodottiSg()) for (const fo of [fonteDi(p.id), p.s9 && ogg(p.s9.fonte) ? p.s9.fonte : null]) {
    if (!ogg(fo)) continue;
    for (const x of arr(fo.proposte_sorgente)) if (ogg(x)) metti(x.allegato_id);
    if (ogg(fo.sorgente_s8)) metti(fo.sorgente_s8.allegato_id);
  }
  return out;
}
// i prodotti per cui il motore propone uno STEP come sorgente (proposte_sorgente), anche senza un gruppo importabile
function prodottiConProposta(allegato) {
  const a = String(allegato || "").toLowerCase();
  return prodottiSg().filter((p) => proposteSorgente(p.id).some((x) => x.c.allegato_id === a)).map((p) => p.id);
}
// lo STEP sorgente di un prodotto confermato da S8 e valido (direttiva 39 §2; la fonte di S9, o di /dati per il prodotto
// scelto: sorgente_s8 {allegato_id, valida, motivo}). Il documento confermato da una pagina legacy (confermata) e il
// legame storico univoco (legame_storico, D-AR04 (iii)) non bastano: si riconfermano da S8.
function sorgenteS8(id) {
  const fo = fonteDi(id);
  return fo && ogg(fo.sorgente_s8) ? fo.sorgente_s8 : null;
}
function sorgenteDelProdotto(id) {
  const s8 = sorgenteS8(id);
  return s8 && s8.valida && s8.allegato_id ? String(s8.allegato_id).toLowerCase() : "";
}
function sorgenteValida(id) {
  return !!sorgenteDelProdotto(id);
}
// bomConfermataSu: la BOM confermata del prodotto ha come STEP la sua sorgente (allegato, se dato): dopo «Conferma BOM» la
// sorgente è lo STEP della BOM (fonte.autorizzata) e la bozza non c'è più; non manca niente (collaudo UI46, B3)
function bomConfermataSu(id, allegato) {
  const fo = fonteDi(id);
  if (!fo || fo.autorizzata !== true) return false;
  const a = sorgenteDelProdotto(id);
  return !!a && (!allegato || a === String(allegato).toLowerCase());
}
// sorgenteDellaBozza: {allegato_id, gruppo} della sorgente della bozza aperta di un prodotto (R1 per il prodotto scelto,
// S9 bozza.sorgente per gli altri), o null per una bozza senza STEP (partenza vuota, direttiva 39 §1). Il gruppo
// riservato «solo_prodotto» dice uno STEP confermato senza struttura importata.
function sorgenteDellaBozza(id) {
  if (id === S.prodotto && bozzaLavorabile() && S.salvato) {
    const s = S.salvato.sorgente;
    return ogg(s) && s.allegato_id ? { allegato_id: String(s.allegato_id).toLowerCase(), gruppo: s.gruppo || "" } : null;
  }
  const p = s9P(id);
  const s = p && p.bozza && p.bozza.sorgente;
  if (typeof s === "string" && s) return { allegato_id: s.toLowerCase(), gruppo: "" };
  return null;
}
function prodottiConSorgente(allegato) {
  return prodottiSg().filter((p) => sorgenteDelProdotto(p.id) === allegato);
}
// la sintesi delle candidate di uno STEP per una riga (MAT SG-01): radice letta, compatibilità, struttura (LS-01) e
// segnalati (LS-02), per prodotto. Solo presentazione: le dice il server.
// La riga dice per ogni prodotto uno stato solo (messaggio 46, D47 e punto 4): lo stato della sorgente se il prodotto ha su
// questo STEP la conferma di S8 (statiSorgente), altrimenti «◇ candidata per ‹P› · ‹compatibilità›»; la diagnostica del
// motore (radice letta, struttura, segnalati) sta nel titolo e nel dettaglio, non nella riga.
function sintesiCandidate(allegato) {
  const f = fileDi(allegato);
  const ss = statiSorgente(allegato);
  const cc = candidateDi(allegato).filter((c) => !ss.some((x) => x.pid === c.prodotto_id));
  // lo stato della sorgente lo dicono le pillole del file (metaFile): qui solo le candidate per gli altri prodotti
  if (!cc.length) {
    if (ss.length) return null;
    // lo STEP proposto come sorgente senza una struttura importabile (collaudo UI46, I3): lo dice, non «nessuna candidata»
    const pp = prodottiConProposta(allegato);
    if (pp.length) return h("div", { class: "du-fr-meta du-piccolo", "data-du-candidate": "0", "data-du-proposto": pp.join(" ") },
      pp.map((id) => pill("prop", "◇ proposto come sorgente di " + codiceDi(id) + " · nessuna struttura importabile")));
    return h("div", { class: "du-fr-meta du-piccolo", "data-du-candidate": "0" }, pill("neu", S.erroreSmist ? "candidate non lette" : "nessuna candidata"));
  }
  return h("div", { class: "du-fr-meta du-piccolo", "data-du-candidate": String(cc.length) }, cc.map((x) => pillCandidata(x, f)));
}
// pillCandidata: una candidata dello STEP per un prodotto, in breve, con la diagnostica nel titolo; null se il prodotto ha
// su questo STEP la conferma di S8 (lo stato lo dicono le pillole del file)
function pillCandidata(x, f) {
  if (f && statoSorgentePer(f.allegato_id, x.prodotto_id)) return null;
  const st = ogg(x.struttura) ? (x.struttura.completa === false ? "struttura non completa" + (x.struttura.motivo ? ": " + x.struttura.motivo : "") : "struttura completa") : "";
  const di = x.sorgente_di && String(x.sorgente_di).toLowerCase() !== x.prodotto_id ? " · già sorgente di " + codiceDi(String(x.sorgente_di).toLowerCase()) : "";
  const titolo = perchéIncerta(x, null, "") || ["radice letta " + (x.radice_letterale || "?"), x.compatibilita ? "compatibilità: " + x.compatibilita : "", st, typeof x.segnalati === "number" ? "nodi segnalati: " + x.segnalati : ""].filter(Boolean).join(" · ");
  return h("span", { class: "du-st du-st--wrap du-st--" + (x.compatibilita === "compatibile" ? "ok" : "ver"), title: titolo, "data-du-candidata": x.prodotto_id, "data-du-radice": x.radice_letterale || "" },
    "◇ candidata per " + codiceDi(x.prodotto_id) + (x.compatibilita ? " · " + x.compatibilita : "") + di);
}

// ---- le cartelle: chiave, nome, conto, seconda riga

function cartellaNome(k) {
  if (k === "dc") return "Da controllare";
  if (k === "dc-senza") return "Senza proposta";
  if (k === "dc-analisi") return "In analisi";
  if (k === "dc-arch") return "Archivi ricevuti";
  if (k.indexOf("dc-att:") === 0) return "Attendono " + codiceDi(k.slice(7));
  if (k.indexOf("dc-ric:") === 0) return "Da ricollocare · " + codiceDi(k.slice(7));
  if (k === "step") return "STEP della RFQ";
  if (k === "prodotti") return "Prodotti";
  if (k.indexOf("p:") === 0) return codiceDi(k.slice(2));
  if (k === "altri") return "Altri file · ALTRO";
  if (k === "scartati") return "Scartati";
  if (k === "tutti") return "Tutti i file";
  if (k === "ris") return "Risultati";
  return k;
}
const DEFINIZIONI = {
  dc: "I file ancora da associare: senza proposta, in analisi, in attesa della sorgente di un prodotto, gli archivi ricevuti e i file da ricollocare. Un file in attesa di due prodotti si conta una volta.",
  "dc-senza": "File letti senza destinazione né proposta: il motivo è quello del Motore A. «Associa a un nodo…» li mette nella bozza di un prodotto.",
  "dc-analisi": "File non ancora letti, STEP compresi: aspetta la lettura per associarli.",
  "dc-arch": "Contenitori ZIP e 7z attivi: le voci estratte sono file a sé.",
  step: "Gli STEP attivi e letti, con le loro candidature per prodotto. Guardare un'anteprima non scrive niente.",
  prodotti: "Tutti i collegamenti dei file, a sezioni per prodotto.",
  altri: "Cartella ALTRO della RFQ sul NAS. Nella stessa cartella stanno i documenti confermati di tipo \"altro\": che cosa è confermato e che cosa è solo conservato lo dice il database.",
  scartati: "I file scartati, con la traccia: nome, SHA, origine, chi e quando. Lo scarto è terminale.",
  tutti: "Ogni file una volta, per arrivo, con le voci sotto l'archivio.",
  ris: "I file che corrispondono alla ricerca, in tutte le cartelle.",
};
function definizioneDi(k) {
  if (DEFINIZIONI[k]) return DEFINIZIONI[k];
  if (k.indexOf("dc-att:") === 0) return "File che il Motore A propone per " + codiceDi(k.slice(7)) + ", che non ha ancora una sorgente: la strada è «Scegli la sorgente di " + codiceDi(k.slice(7)) + " →».";
  if (k.indexOf("dc-ric:") === 0) return "Collegamenti salvati per nodi usciti dalla bozza di " + codiceDi(k.slice(7)) + ": ricollocali, o lasciali senza associazione. Niente è cancellato.";
  if (k.indexOf("p:") === 0) return "Lo smistamento di " + codiceDi(k.slice(2)) + ": i collegamenti proposti (◇), salvati nella bozza (◈) e già registrati (●). Sono proposte finché non le salvi; la BOM diventa ufficiale solo con «Conferma BOM».";
  return "";
}
function corrisponde(f, extra) {
  const q = SG.cerca.trim().toLowerCase();
  if (!q) return true;
  const testo = [f.nome_file, f.proposta && f.proposta.codice, f.proposta && f.proposta.rev, extra].filter(Boolean).join(" ").toLowerCase();
  return testo.indexOf(q) >= 0;
}
// contenuto: le righe di una cartella, senza la ricerca: {f, grp, pid?, r?, stato?, c?}
function contenuto(k) {
  const tutti = fileSorgente();
  const attivi = tutti.filter(eAttivo);
  if (k === "dc-senza") {
    if (!S.smist) return [];
    return arr(S.smist.da_associare).map((d) => fileDi(d.allegato_id)).filter((f) => eAttivo(f) && !inAnalisiF(f) && !eArchivio(f) && !attesaDi(f.allegato_id).length && !collegamentiDi(f).length)
      .map((f) => ({ f, grp: "senza" }));
  }
  if (k === "dc-analisi") return attivi.filter(inAnalisiF).map((f) => ({ f, grp: "analisi" }));
  if (k === "dc-arch") return attivi.filter(eArchivio).map((f) => ({ f, grp: "archivi" }));
  if (k.indexOf("dc-att:") === 0) {
    const id = k.slice(7);
    const p = s9P(id);
    if (!p || sorgenteValida(id)) return [];
    return arr(p.in_attesa_sorgente).map((x) => ({ f: fileDi(x.allegato_id), x, pid: id, grp: "attesa:" + id })).filter((y) => eAttivo(y.f));
  }
  if (k.indexOf("dc-ric:") === 0) {
    const id = k.slice(7);
    if (id !== S.prodotto || !bozzaLavorabile()) return [];
    return vassoioCorrente().map((x) => ({ f: fileDi(x.allegato_id), v: x, pid: id, grp: "ric:" + id })).filter((y) => y.f && eAttivo(y.f));
  }
  if (k === "dc") {
    const visti = new Set();
    const out = [];
    for (const c of sottoDc()) for (const y of contenuto(c)) if (!visti.has(y.f.allegato_id)) {
      visti.add(y.f.allegato_id);
      out.push(y);
    }
    return out;
  }
  if (k === "step") return stepDellaRfq().map((s) => fileDi(s.allegato_id)).filter((f) => f && letto(f)).map((f) => ({ f, grp: "step" }));
  if (k.indexOf("p:") === 0) {
    const id = k.slice(2);
    // senza bozza la cartella del prodotto mostra le sue candidate (gli STEP che il motore propone per lui)
    // e, sotto, i file con un'associazione registrata (le righe gia_registrato di S9, in sola lettura: per esempio le AR
    // appena scritte da «Conferma BOM», quando la bozza si è chiusa) [T-39]
    if (!haBozza(id)) {
      const cand = candidatePer(id).map((c) => ({ f: fileDi(c.allegato_id), c, pid: id, grp: "cand:" + id })).filter((y) => y.f);
      // lo STEP proposto come sorgente senza gruppo sta fra le candidate del prodotto (collaudo UI46, I3)
      for (const sp of proposteSorgente(id)) if (sp.f && !cand.some((y) => y.f.allegato_id === sp.c.allegato_id)) cand.push({ f: sp.f, c: sp.c, pid: id, grp: "cand:" + id });
      const visti = new Set(cand.map((y) => y.f.allegato_id));
      return cand.concat(righeDi(id).filter((x) => x.stato === "gia_registrato" && !visti.has(x.f.allegato_id)));
    }
    return righeDi(id);
  }
  if (k === "prodotti") {
    const out = [];
    for (const p of prodottiSg()) if (haBozza(p.id)) for (const y of righeDi(p.id)) out.push(y);
    return out;
  }
  if (k === "altri") return tutti.filter((f) => f.stato === "altri_file").map((f) => ({ f, grp: "altri" }));
  if (k === "scartati") return tutti.filter((f) => f.stato === "scartato").map((f) => ({ f, grp: "scartati" }));
  if (k === "tutti" || k === "ris") {
    const ord = tutti.slice().sort((a, b) => String(a.ricevuto_il || "").localeCompare(String(b.ricevuto_il || "")) || String(a.nome_file).localeCompare(String(b.nome_file)));
    const out = [];
    for (const f of ord) {
      if (f.contenitore_id && fileDi(f.contenitore_id)) continue;
      out.push({ f, grp: k });
      for (const v of ord) if (v.contenitore_id === f.allegato_id) out.push({ f: v, grp: k, voce: true });
    }
    return k === "ris" ? out.filter((y) => corrisponde(y.f)) : out;
  }
  return [];
}
function sottoDc() {
  const out = ["dc-senza", "dc-analisi"];
  for (const p of prodottiSg()) if (p.s9 && !sorgenteValida(p.id) && arr(p.s9.in_attesa_sorgente).length) out.push("dc-att:" + p.id);
  out.push("dc-arch");
  if (S.prodotto && bozzaLavorabile() && vassoioCorrente().length) out.push("dc-ric:" + S.prodotto);
  return out;
}
function righeVisibili(k) {
  return contenuto(k).filter((y) => corrisponde(y.f, y.r && y.r.codice_mostrato));
}
function distinti(righe) {
  return new Set(righe.map((y) => y.f.allegato_id)).size;
}
// conto: il conto della voce dell'albero; null = «—» (il server non lo dice: S9 non letto o prodotto senza bozza)
function conto(k) {
  if (k.indexOf("p:") === 0) {
    if (!haBozza(k.slice(2))) return null;
    return righeDi(k.slice(2)).length;
  }
  if ((k === "dc-senza" || k.indexOf("dc-att:") === 0 || k === "prodotti" || k === "dc") && !S.smist) return null;
  return distinti(contenuto(k));
}
function albero() {
  const v = [];
  v.push({ k: "dc", liv: 1, n: "1", seconda: S.smist ? "ancora da associare" : "smistamento non letto" });
  for (const c of sottoDc()) {
    if (c.indexOf("dc-ric:") === 0 && !conto(c)) continue;
    let seconda = "";
    if (c.indexOf("dc-att:") === 0) {
      const id = c.slice(7);
      const anche = {};
      for (const y of contenuto(c)) for (const p of attesaDi(y.f.allegato_id)) if (p.id !== id) anche[p.codice] = (anche[p.codice] || 0) + 1;
      seconda = Object.keys(anche).map((x) => anche[x] + " valgono anche per " + x).join(" · ");
    }
    v.push({ k: c, liv: 2, seconda });
  }
  const conSorg = prodottiSg().filter((p) => sorgenteValida(p.id)).length;
  v.push({ k: "step", liv: 1, n: "2", seconda: "sorgenti: " + conSorg + " di " + prodottiSg().length });
  v.push({ k: "prodotti", liv: 1, n: "3" });
  for (const p of prodottiSg()) v.push({ k: "p:" + p.id, liv: 2, seconda: rigaStatoProdotto(p.id).t, prodotto: true });
  v.push({ sep: "conservati, non da lavorare" });
  v.push({ k: "altri", liv: 1 });
  v.push({ k: "scartati", liv: 1 });
  const nAtt = fileSorgente().filter(eAttivo).length;
  const nAn = fileSorgente().filter((f) => eAttivo(f) && inAnalisiF(f)).length;
  v.push({ k: "tutti", liv: 1, seconda: nAtt + " attivi" + (nAn ? " · " + nAn + " in analisi" : "") });
  if (SG.cerca.trim()) v.push({ k: "ris", liv: 1 });
  for (const x of v) if (x.k) x.conto = conto(x.k);
  return v;
}
// la seconda riga del prodotto nell'albero (LEGGIMI-v5 §3): stato della sorgente, autorizzazione, bozza, conti per stato
function rigaStatoProdotto(id) {
  const p = s9P(id);
  const e = SG.esitiS8[id];
  if (SG.inCorsoS8[id]) return { t: "⟳ conferma della sorgente in corso", tono: "info" };
  if (e && e.esito === "da_verificare") return { t: "? esito da verificare", tono: "ver" };
  const fo = fonteDi(id);
  if (!p && !fo) return { t: S.erroreSmist ? "smistamento non letto" : "—", tono: "neu" };
  const bz = id === S.prodotto && S.bozza ? S.bozza.versione : p && p.bozza && p.bozza.versione;
  const v = bz ? "v" + bz : "";
  if (!sorgenteValida(id)) {
    // direttiva 39 §1: una bozza senza STEP si lavora; la conferma della BOM aspetta una sorgente confermata da S8
    const s8 = sorgenteS8(id);
    const perche = s8 ? "! sorgente " + testoMotivoSorgente(s8.motivo) : fo && ogg(fo.legame_storico) ? "○ legame storico da confermare" : "○ da confermare";
    if (haBozza(id)) return { t: perche + " · bozza " + v + " senza sorgente confermata", tono: "ver" };
    if (S.anteprima && S.prodotto === id && S.anteprimaAllegato) return { t: "◌ in anteprima · " + nomeFile(S.anteprimaAllegato), tono: "prop" };
    // gli STEP candidati e quelli proposti come sorgente senza gruppo, contati una volta (collaudo UI46, I3)
    const nStep = new Set(candidatePer(id).map((c) => c.allegato_id).concat(proposteSorgente(id).map((x) => x.c.allegato_id))).size;
    return { t: perche + " · " + nStep + " STEP", tono: "ver" };
  }
  if (!haBozza(id) && bomConfermataSu(id)) return { t: "✓ sorgente · BOM confermata", tono: "ok" };
  if (!haBozza(id)) return { t: e && e.bozza409 ? "✓ sorgente · ! bozza non creata: proposta cambiata" : "✓ sorgente · ! bozza non creata", tono: "ver" };
  if (!p) return { t: "✓ sorgente · bozza · smistamento non letto", tono: "ver" };
  const calc = (p.calcolo && p.calcolo.stato) || "";
  if (calc && calc !== "pronto") return { t: "✓ sorgente · bozza " + v + " · " + (calc === "errore" ? "smistamento in errore" : "smistamento in calcolo"), tono: calc === "errore" ? "ko" : "info" };
  const righe = righeDi(id);
  const c = { proposto: 0, salvato_nella_bozza: 0, gia_registrato: 0 };
  for (const y of righe) if (c[y.stato] !== undefined) c[y.stato]++;
  const nc = righe.filter((y) => y.stato === "in_conflitto").length;
  const nr = id === S.prodotto && bozzaLavorabile() ? vassoioCorrente().length : 0;
  if (!righe.length) return { t: "✓ sorgente · bozza " + v + " · nessun abbinamento", tono: "ok" };
  return { t: "✓ sorgente · bozza " + v + " · ◇" + c.proposto + " ◈" + c.salvato_nella_bozza + " ●" + c.gia_registrato + (nc ? " · ! " + nc + " in conflitto" : "") + (nr ? " · ↺ " + nr : ""), tono: "ok" };
}
// la cartella iniziale (D-V5-01 senza risposta: vale la raccomandata A′, base [T]): finché nessun prodotto ha la bozza
// «STEP della RFQ»; poi «Da controllare» se ci sono file senza proposta, in analisi o da ricollocare; altrimenti «STEP
// della RFQ» se manca una sorgente; altrimenti il primo prodotto con proposte da salvare
function cartellaIniziale() {
  if (nessunaBozza() && stepDellaRfq().length) return "step";
  if (conto("dc-senza") || conto("dc-analisi") || (S.prodotto && bozzaLavorabile() && vassoioCorrente().length)) return "dc";
  if (prodottiSg().some((p) => !sorgenteValida(p.id))) return "step";
  const p = prodottiSg().find((x) => righeDi(x.id).some((y) => y.stato === "proposto"));
  return p ? "p:" + p.id : "dc";
}
function memo(k) {
  k = k || SG.cartella;
  if (!SG.memo[k]) SG.memo[k] = { file: "", spunte: new Set() };
  return SG.memo[k];
}
function cartellaValida(k) {
  if (!k) return false;
  if (k.indexOf("p:") === 0 || k.indexOf("dc-att:") === 0 || k.indexOf("dc-ric:") === 0) return prodottiSg().some((p) => p.id === k.slice(k.indexOf(":") + 1));
  if (k === "ris") return !!SG.cerca.trim();
  return ["dc", "dc-senza", "dc-analisi", "dc-arch", "step", "prodotti", "altri", "scartati", "tutti"].includes(k);
}
// vaiCartella: apre una cartella; con o.salto ricorda da dove («‹ Torna a …»); con o.file rende attivo quel file. La
// cartella di un prodotto lavora sulla sua bozza: il prodotto scelto la segue (R1, R7 e /dati di quel prodotto).
async function vaiCartella(k, o) {
  o = o || {};
  if (o.salto && SG.cartella && SG.cartella !== k) SG.torna = SG.cartella;
  else if (!o.salto) SG.torna = "";
  SG.cartella = k;
  // cambiare cartella rimette l'anteprima allargata nel riquadro stretto (come «Riduci ⟩»): l'elenco torna in vista
  SG.larga = false;
  SG.caricaAperto = false;
  if (o.file) memo(k).file = o.file;
  if (o.vista) SG.vista = o.vista;
  else if (o.daAlbero && SG.vista === "cartelle") SG.vista = "elenco";
  if (k.indexOf("p:") === 0 && k.slice(2) !== S.prodotto) await scegliProdotto(k.slice(2));
  if (S.passo !== "sorgente") return;
  renderCorpo();
  if (o.fuoco === "elenco") fuocoSuRiga(memo().file);
  annuncia(cartellaNome(k) + ": " + (conto(k) === null ? "conto non disponibile" : conto(k) + " file") + ".");
}
function annuncia(t) {
  const el = $("#du-sg-annuncio");
  if (el) el.textContent = t;
}
// fuocoSuRiga: il fuoco sulla riga, che si porta in vista solo se non lo è già (UI46, controllo di stabilità del 21/10
// [T]: il focus() semplice arrivava prima che lo spazio di lavoro avesse l'altezza, e l'elenco ripartiva da capo)
function fuocoSuRiga(allegato) {
  const el = (allegato && document.querySelector("#du-sg-elenco [role=row][data-allegato='" + allegato + "']")) || document.querySelector("#du-sg-elenco [role=row][tabindex='0']");
  if (!el) return;
  const sg = $("#du-sg");
  if (sg && !sg.style.height) adattaAltezzaSg();
  el.focus({ preventScroll: true });
  const box = $("#du-sg-elenco");
  if (!box) return;
  const r = el.getBoundingClientRect();
  const b = box.getBoundingClientRect();
  const testa = box.querySelector(".du-sg-elenco-t");
  const su = b.top + (testa ? testa.getBoundingClientRect().height : 0);
  if (r.top < su || r.bottom > b.bottom) {
    el.scrollIntoView({ block: "nearest" });
    // la testata dell'elenco resta appiccicata in cima: la riga non ci finisce sotto
    const r2 = el.getBoundingClientRect();
    if (r2.top < su) box.scrollTop -= su - r2.top;
  }
}

// ---- la matrice stato del file → azioni (LEGGIMI-v5 §4): una sola fonte per il dettaglio, il gesto in riga e le azioni
// di gruppo. Un'azione è nascosta se non ha senso in quello stato, spenta con la ragione se qualcosa la blocca adesso. Le
// ragioni dei gesti dei file le dà il server (S1 azioni, S9 azioni delle righe, {disponibile, motivo}).

const NO_BOZZA_SG = "nessun prodotto ha ancora la bozza: conferma prima una sorgente, o parti dal solo prodotto nella Revisione";
function azA(id, t, fai, no, o) {
  return Object.assign({ id, t, fai, no: no || "" }, o || {});
}
// conProdotto: un gesto sulla bozza di un prodotto che non è quello scelto passa prima a quel prodotto (R1 di quella
// bozza), poi fa il gesto; il nome del gesto dice il prodotto
async function conProdotto(id, fn) {
  if (id && id !== S.prodotto) {
    await scegliProdotto(id);
    if (S.prodotto !== id) return;
  }
  return fn();
}
function nomeConProdotto(t, id, piu) {
  if (!piu) return t;
  const c = codiceDi(id);
  return /…$/.test(t) ? t.slice(0, -1) + " di " + c + "…" : t + " di " + c;
}
function srvAz(r, nome) {
  const a = r && r.azioni && r.azioni[nome];
  if (a === undefined || a === null) return "";
  if (typeof a === "boolean") return a ? "" : "non ammesso per questa riga";
  return a.disponibile === false ? senzaGergo(a.motivo) || "non ammesso per questa riga" : "";
}
// apriFile: «Apri» (MAT FL-03): un PDF nel visore della pagina; gli altri formati all'indirizzo di S5. Un indirizzo, mai
// una fetch della pagina
function apriFile(f) {
  if (ePdf(f)) return apriVisore(f);
  const a = h("a", { href: P.urlContenuto(f.allegato_id, "anteprima"), target: "_blank", rel: "noopener" });
  document.body.append(a);
  a.click();
  a.remove();
}
// la scrittura sul NAS spenta (S1 configurazione.altri_file, U2 §3.5): «Metti in Altri file» spento con il motivo
function noAltriFileConfig() {
  const c = S.sorgente && S.sorgente.configurazione;
  const af = c && c.altri_file;
  if (ogg(af) && af.disponibile === false) return af.motivo || "La scrittura sul NAS è spenta su questo server: la cartella ALTRO sta sul NAS";
  return "";
}
function azioniFile(f, ctx) {
  ctx = ctx || {};
  const A = { prin: null, altre: [], spente: [], step: false };
  const cont = f.azioni && f.azioni.contenuto;
  const apri = azA("apri", "Apri", () => apriFile(f), cont && cont.disponibile === false ? "contenuto non disponibile: " + (cont.motivo || "") : "");
  const sc = azioneFile(f, "scarta"), af = azioneFile(f, "altri_file"), ri = azioneFile(f, "riattiva");
  const afNo = af.ok ? noAltriFileConfig() : af.motivo;
  if (f.stato === "scartato") {
    A.spente.push(azA("riattiva", "Riattiva…", null, "Lo scarto è terminale: per riavere il file serve un nuovo caricamento."));
    // la copia in ALTRO ancora da togliere con la scrittura sul NAS spenta (U2 §3.5): un gesto visibile, con una chiave nuova
    if (f.conservazione && f.conservazione.stato === "da_rilasciare" && f.azioni && f.azioni.riprova_rimozione) {
      const rr = azioneFile(f, "riprova_rimozione");
      A.altre.push(azA("riprova-rimozione", "Riprova la rimozione dalla cartella ALTRO", null, rr.ok ? "non ancora in questa pagina: la rotta non è nel contratto del ponte" : rr.motivo));
    }
    return finiscilo(A);
  }
  const arch = eArchivio(f);
  if (f.stato === "altri_file") {
    A.prin = azA("riattiva", "Riattiva…", () => apriRiattiva(f, "contenitore"), ri.ok ? "" : ri.motivo, { g: "riattiva", sub: arch ? "riattiva solo il contenitore" : "" });
    A.altre.push(apri);
    if (arch) A.altre.push(azA("riattiva-voci", "Riattiva anche i contenuti…", () => apriRiattiva(f, "contenitore_e_voci"), ri.ok ? "" : ri.motivo, { sub: "gesto separato, con il perimetro elencato" }));
    A.altre.push(azA("scarta", "Scarta…", () => apriScarta(f), sc.ok ? "" : sc.motivo, { g: "scarta", pericolo: true, sub: "elimina anche la copia del cockpit nella cartella ALTRO; resta la traccia" }));
    if (!arch) A.spente.push(azA("associa", "Associa a un nodo…", null, "È in Altri file: prima riattivalo."));
    return finiscilo(A);
  }
  const links = collegamentiDi(f);
  const piu = links.length > 1 || !ctx.pid || !links.some((x) => x.pid === ctx.pid);
  const prinPer = {};
  const confermato = !!f.documento;
  for (const x of links) {
    const id = x.pid;
    const r = x.r;
    const k = chiaveRiga(r, id);
    const modifica = id === S.prodotto ? perchéNoModifica() : "";
    const suf = id === ctx.pid || links.length === 1 ? "" : "-" + sid(id);
    if (x.stato === "proposto" || x.stato === "tolto") {
      // G39-02 [U]: lo STEP del prodotto finito (proposto o legato come sorgente) non si salva come documento di un nodo,
      // anche se una riga di S9 lo propone: si conferma come sorgente (R2 lo rifiuta comunque)
      const npa = !preassociabile(f) ? (eStep(f) ? "è lo STEP del prodotto finito: si conferma come sorgente, non si preassocia a un nodo" : "il file non si preassocia (non attivo o archivio)") : "";
      const s = azA("salva" + suf, nomeConProdotto("Salva nella bozza", id, piu), () => conProdotto(id, () => mossaBozza("salva", f, x)),
        npa || (x.stato === "proposto" ? srvAz(r, "salva_nella_bozza") : "") || (k ? "" : "il nodo della proposta non è nella bozza: «Associa a un altro nodo…»") || modifica, { pid: id, g: "salva", corta: "Salva" });
      A.altre.push(s, azA("sposta" + suf, nomeConProdotto("Associa a un altro nodo…", id, piu), () => conProdotto(id, () => apriAssociaNodo(f, { pid: id })), npa || srvAz(r, "sposta") || modifica, { pid: id }));
      prinPer[id] = s;
    } else if (x.stato === "salvato_nella_bozza" || (x.stato === "in_conflitto" && r.origine !== "working")) {
      const t = azA("togli" + suf, nomeConProdotto("Togli dalla bozza", id, piu), () => conProdotto(id, () => mossaBozza("togli", f, x)), (r.locale ? "" : srvAz(r, "togli_dalla_bozza")) || modifica, { pid: id, g: "togli", corta: "Togli" });
      const sp = azA("sposta" + suf, nomeConProdotto("Sposta a un altro nodo…", id, piu), () => conProdotto(id, () => apriAssociaNodo(f, { pid: id, da: k })), (r.locale ? "" : srvAz(r, "sposta")) || modifica, { pid: id });
      A.altre.push(t, sp);
      prinPer[id] = x.stato === "in_conflitto" ? sp : t;
    } else if (x.stato === "legato_come_sorgente" || x.stato === "legame_storico") {
      // lo STEP sorgente del finito in sola lettura (direttiva 39 §5; S9): nessun gesto documentale da qui; la sorgente si
      // cambia con S8 o con la revoca, il legame storico si conferma con S8
      prinPer[id] = prinPer[id] || "apri";
    } else if (confermato) {
      A.spente.push(azA("scollega" + suf, nomeConProdotto("Scollega alla conferma della BOM…", id, piu), null, "È un documento confermato: si sostituisce o si revisiona, non da questa pagina.", { pid: id }));
      prinPer[id] = prinPer[id] || "apri";
    } else if (x.stato === "da_scollegare") {
      const u = azA("scollega" + suf, nomeConProdotto("Annulla lo scollegamento alla conferma", id, piu), () => conProdotto(id, () => annullaScollegamento(f, x)), modifica, { pid: id });
      A.altre.push(u);
      prinPer[id] = prinPer[id] || "apri";
    } else {
      const u = azA("scollega" + suf, nomeConProdotto("Scollega alla conferma della BOM…", id, piu), () => conProdotto(id, () => apriScollega(f, { nodo: { componente_id: r.destinazione && r.destinazione.componente_id } })),
        srvAz(r, "scollega_alla_conferma") || modifica, { pid: id, sub: "decisione della bozza; l'associazione resta fino a «Conferma BOM»" });
      A.altre.push(u);
      prinPer[id] = x.stato === "in_conflitto" ? u : prinPer[id] || "apri";
    }
    if (id !== ctx.pid && links.length > 1) A.altre.push(azA("vai" + suf, "Vai alla riga in " + codiceDi(id), () => vaiCartella("p:" + id, { salto: true, file: f.allegato_id })));
  }
  const kP = ctx.pid && prinPer[ctx.pid] ? ctx.pid : links.length === 1 ? links[0].pid : "";
  const ric = S.prodotto && bozzaLavorabile() ? vassoioCorrente().find((x) => x.allegato_id === f.allegato_id) : null;
  const att = attesaDi(f.allegato_id);
  if (links.length) {
    if (kP) A.prin = prinPer[kP] === "apri" ? apri : prinPer[kP];
  } else if (ric) {
    A.prin = azA("associa", "Associa a un nodo…", () => apriAssociaNodo(f, { pid: S.prodotto, ricolloca: true }), perchéNoModifica(), { corta: "Associa…" });
    A.altre.push(azA("lascia", "Lascia senza associazione", () => mossaBozza("lascia", f, { pid: S.prodotto }), perchéNoModifica()));
  } else if (arch) {
    const nv = arr(f.voci).length;
    A.prin = azA("altri", "Metti lo " + formatoDi(f) + " originale in Altri file…", () => apriAltriFile(f, "contenitore"), afNo,
      { g: "altri", corta: formatoDi(f) + " in Altri file…", sub: "solo il contenitore: i file estratti restano operativi" });
    A.altre.push(azA("altri-voci", "Metti in Altri file anche i contenuti" + (nv ? " (" + nv + ")" : "") + "…", () => apriAltriFile(f, "contenitore_e_voci"), afNo, { sub: "gesto separato, con il perimetro elencato" }));
  } else if (!letto(f) && eStep(f)) {
    // uno STEP in analisi o con l'analisi fallita (direttiva 39 §3): l'anteprima dice lo stato, mai come prova geometrica,
    // e offre la conferma come sorgente con la partenza dal solo prodotto, con la presa d'atto
    for (const p of prodottiSg()) A.altre.push(azA("guarda-" + sid(p.id), "Guarda l'anteprima per " + p.codice, () => guardaPer({ allegato_id: f.allegato_id, prodotto_id: p.id, gruppo: "" }), "",
      { sub: "lo STEP non è ancora letto: l'anteprima dice lo stato dell'analisi" }));
  } else if (!letto(f)) {
    A.spente.push(azA("associa", "Associa a un nodo…", null, "Aspetta la lettura del file", { corta: "Associa…" }));
  } else if (eStep(f)) {
    A.step = true;
  } else {
    const assoc = azA("associa", "Associa a un nodo…", () => apriAssociaNodo(f, { pid: att.length ? "" : S.prodotto }), nessunaBozza() ? NO_BOZZA_SG : "", { corta: "Associa…" });
    if (att.length) {
      const a0 = att.find((p) => p.id === ctx.pid) || att[0];
      A.prin = azA("sorgente-di", "Scegli la sorgente di " + a0.codice + " →", () => vaiCartella("p:" + a0.id, { salto: true, vista: "elenco" }), "", { corta: "Sorgente di " + a0.codice + " →" });
      if (assoc.no) A.spente.push(assoc);
      else A.altre.push(Object.assign(assoc, { sub: "a un altro prodotto: scelta tua, con la traccia" }));
    } else if (assoc.no) A.spente.push(assoc);
    else A.prin = assoc;
  }
  // lo STEP sorgente confermato da S8 (direttiva 39 §2, §5): con la bozza «Vai alla Revisione di ‹P› →»; «Revoca come
  // sorgente di ‹P›…» per uno STEP scelto per errore. Niente «Autorizza»: la fonte della working la
  // promuove solo la conferma della BOM (R5; D-TAB-02 B rivista [T-39])
  for (const p of prodottiConSorgente(f.allegato_id)) {
    const corso = SG.inCorsoS8[p.id] ? "È in corso la conferma della sorgente di " + p.codice + "." : "";
    if (haBozza(p.id)) A.prin = azA("revisione", "Vai alla Revisione di " + p.codice + " →", () => conProdotto(p.id, () => vaiPasso("revisione")));
    A.altre.push(azA("revoca-sorgente", "Revoca come sorgente di " + p.codice + "…", () => conProdotto(p.id, () => apriRevocaSorgente(f)), corso,
      { pericolo: true, sub: "per uno STEP scelto per errore" }));
  }
  if (A.prin !== apri) A.altre.unshift(apri);
  const sorgenteDi = prodottiConSorgente(f.allegato_id);
  // messaggio 46 (D18, D29): il perché in parole dell'ingegnere, senza le versioni del progetto
  const confNo = sorgenteDi.length ? "è la sorgente di " + sorgenteDi.map((p) => p.codice).join(", ") + ": prima revocala" :
    confermato ? "è un documento confermato: si toglie solo con una revisione o una sostituzione, non da questa pagina" : "";
  if (!arch) A.altre.push(azA("altri", "Metti in Altri file…", () => apriAltriFile(f, ""), confNo || afNo, { g: "altri", sub: "la copia va nella cartella ALTRO della RFQ sul NAS" }));
  A.altre.push(azA("scarta", "Scarta…", () => apriScarta(f), confNo || (sc.ok ? "" : sc.motivo), { g: "scarta", pericolo: true, sub: arch ? "solo il contenitore: le voci estratte restano attive" : "toglie la copia gestita dal cockpit; resta la traccia" }));
  // la copia in Altri file in corso: tutto spento tranne «Apri»
  if (f.conservazione && f.conservazione.stato === "in_copia") {
    const bl = "La copia in Altri file è in corso: aspetta la risposta.";
    for (const a of [A.prin].concat(A.altre)) if (a && a !== apri) A.spente.push(Object.assign(a, { no: bl }));
    A.prin = null;
    A.altre = [apri];
  }
  return finiscilo(A);
}
// finiscilo: le azioni spente vanno in «Non disponibili (n)»; il principale spento non resta principale
function finiscilo(A) {
  if (A.prin && A.prin.no) {
    A.spente.unshift(A.prin);
    A.prin = null;
  }
  const visti = new Set(A.prin ? [A.prin.id] : []);
  const spente = [];
  A.altre = A.altre.filter((a) => {
    if (!a || visti.has(a.id)) return false;
    if (a.no) {
      spente.push(a);
      return false;
    }
    visti.add(a.id);
    return true;
  });
  A.spente = A.spente.concat(spente);
  const vs = new Set();
  A.spente = A.spente.filter((a) => a && !visti.has(a.id) && !vs.has(a.id) && vs.add(a.id));
  return A;
}
// i gesti di una candidatura (U2 §2.2; LEGGIMI-v5 §4; direttiva 39 §2-§3): «Guarda l'anteprima per ‹P›» e un solo gesto
// di S8, con il nome del prodotto (X-01 [U]): «Conferma questo STEP come sorgente di ‹P›» (anche su una bozza senza
// STEP, che il server riconcilia), «Usa nuovo STEP per ‹P›…» (cambia_sorgente via S8), «Aggiorna proposta…»
// (riallinea_proposta, stesso STEP), «Conferma di nuovo come sorgente di ‹P›» (dopo una revoca, la bozza già su questo
// STEP). Nessun «Autorizza» e nessuna revoca preliminare: la working cambia solo con la conferma della BOM.
function azioniCandidatura(c) {
  const id = c.prodotto_id;
  const cod = codiceDi(id);
  const out = [];
  const f = fileDi(c.allegato_id);
  const suf = sid(c.allegato_id) + (id === S.prodotto ? "" : "-" + sid(id));
  const inVista = S.anteprima && S.prodotto === id && S.anteprimaAllegato === c.allegato_id;
  const bl = f && f.conservazione && f.conservazione.stato === "in_copia" ? "La copia in Altri file di questo STEP è in corso." : "";
  out.push(azA("guarda-" + suf, inVista ? "Chiudi l'anteprima" : "Guarda l'anteprima per " + cod, () => (inVista ? chiudiAnteprima() : guardaPer(c)), bl, { domId: "du-guarda-" + suf }));
  const sorg = sorgenteDelProdotto(id);
  const bozza = haBozza(id);
  const sb = bozza ? sorgenteDellaBozza(id) : null;
  let t = "Conferma questo STEP come sorgente di " + cod;
  let no = bl;
  if (SG.inCorsoS8[id]) no = "È in corso la conferma della sorgente di " + cod + ".";
  else if (S.modello && S.modello.congelata) no = CONGELATA;
  // G39-03 [U]: lo STEP è la sorgente S8 valida di un altro prodotto: in V1 uno STEP è la sorgente di un solo prodotto, ma
  // l'associazione sbagliata (del motore o dell'utente) si slega e si riassocia: «Sposta la sorgente su ‹P›…» è S8 con
  // sposta_da (core/rfq/sorgente/sposta.go: la revoca motivata «sorgente errata» là, l'annullamento della bozza là e S8 qui,
  // atomici). Se l'altro prodotto ha già confermato la BOM con lui il server rifiuta (bom_confermata) con il rimedio della
  // sua revisione: la pagina non lo indovina, lo dice il server.
  const altro = c.sorgente_di && String(c.sorgente_di).toLowerCase() !== id ? String(c.sorgente_di).toLowerCase() : "";
  if (altro) {
    out.push(azA("conferma-" + suf, "Sposta la sorgente su " + cod + "…", () => conProdotto(id, () => apriConfermaSorgente(c, { spostaDa: altro })), no, { domId: "du-sposta-sorgente-" + suf }));
    return out;
  }
  const stessaDellaBozza = !!(sb && sb.allegato_id === c.allegato_id && sb.gruppo !== "solo_prodotto");
  if (sorg && sorg === c.allegato_id && bozza && stessaDellaBozza) {
    // caso (e): la bozza è già su questa sorgente; al posto di S8 «Aggiorna proposta…» (riallinea_proposta: stesso STEP,
    // evidenze, regole o file nuovi; direttiva 39 §3)
    out.push(azA("riallinea-" + suf, "Aggiorna proposta…", () => conProdotto(id, apriRiallinea), S.confrontoNo ? CONFRONTO_NON_V1 : id === S.prodotto ? perchéNoModifica() : "", { domId: "du-riallinea-" + suf }));
    return out;
  }
  if (sorg && sorg === c.allegato_id && !bozza && bomConfermataSu(id, sorg)) {
    // la BOM è confermata con questo STEP e la bozza è chiusa: niente da riprendere; le modifiche partono dalla BOM
    // confermata, nella Revisione (collaudo UI46, B3)
    out.push(azA("conferma-" + suf, "Vai alla Revisione di " + cod + " →", () => conProdotto(id, () => vaiPasso("revisione")), "", { domId: "du-vai-revisione-" + suf }));
    return out;
  }
  if (sorg && sorg === c.allegato_id && !bozza) {
    const e = SG.esitiS8[id];
    t = e && e.bozza409 ? "Rivedi l'anteprima" : "Riprendi: crea la bozza di " + cod;
  } else if (bozza && stessaDellaBozza) {
    // la bozza è su questo STEP ma la sua conferma di S8 non vale più (revocata, superata): S8 la conferma di nuovo, il
    // passo 3 trova la bozza già sulla sorgente
    t = "Conferma di nuovo come sorgente di " + cod;
  } else if (bozza && sb && sb.allegato_id === c.allegato_id) {
    // la bozza parte dallo STEP senza struttura (solo_prodotto): la struttura arriva con S8 sulla partenza step
    t = "Usa la struttura di questo STEP per " + cod + "…";
  } else if ((sorg && sorg !== c.allegato_id) || (bozza && sb)) {
    t = "Usa nuovo STEP per " + cod + "…";
  } else if (bozza) {
    // una bozza senza STEP (partenza vuota): S8 conferma la fonte e il server riconcilia la stessa bozza
    t = "Conferma questo STEP come sorgente di " + cod + "…";
  }
  const fai = t === "Rivedi l'anteprima" ? () => guardaPer(c) : () => conProdotto(id, () => apriConfermaSorgente(c));
  out.push(azA("conferma-" + suf, t, fai, no, { domId: "du-conferma-sorgente-" + suf }));
  return out;
}
// gli STEP che il motore non propone per un prodotto (il caso di RV3-32 [U]: lo STEP scelto a mano, senza bloccare
// l'ingegnere): l'anteprima per quel prodotto (S6) e poi S8, che associa lo STEP al prodotto al suo passo 1, con la
// partenza dal gruppo dell'anteprima o dal solo prodotto. S7 («Associa lo STEP a ‹P›…», U2 §2.2) non si offre: il server
// non registra la rotta (scelta [T-39] della corsia, rilievo del revisore del 20/10; richiesta alla corsia contratto)
function azioniStepNonCandidato(f, esclusi) {
  const out = [];
  const cand = new Set(candidateDi(f.allegato_id).map((c) => c.prodotto_id));
  for (const p of prodottiSg()) {
    if (cand.has(p.id) || (esclusi && esclusi.has(p.id))) continue;
    // direttiva 39 §3: anche uno STEP senza struttura per il prodotto si guarda (S6 dice nessuna struttura o l'analisi
    // in corso) e si conferma come sorgente con la partenza dal solo prodotto
    out.push(azA("guarda-nc-" + sid(p.id), "Guarda l'anteprima per " + p.codice, () => guardaPer({ allegato_id: f.allegato_id, prodotto_id: p.id, gruppo: "" }), ""));
  }
  return out;
}

// ---- il disegno: tab, barra dello spazio di lavoro, albero, elenco, dettaglio

function corpoSorgente() {
  if (SG.cartella && !cartellaValida(SG.cartella)) SG.cartella = "";
  if (!SG.cartella && S.sorgente) SG.cartella = cartellaIniziale();
  if (!SG.cartella) SG.cartella = "step";
  const larga = !!(S.anteprima && SG.larga);
  const sg = h("div", { class: "du-sg", id: "du-sg", role: "tabpanel", "aria-labelledby": "du-passo-sorgente", "data-vista": SG.vista, "data-larga": larga ? "1" : null },
    h("a", { class: "du-sg-salta", href: "#du-sg-elenco", onclick: (e) => {
      e.preventDefault();
      fuocoSuRiga(memo().file);
    } }, "Vai all'elenco dei file"),
    barraSg(),
    h("div", { class: "du-sg-spazio" }, alberoSg(), elencoSg(), dettaglioSg()),
    h("div", { class: "du-sg-annuncio", id: "du-sg-annuncio", "aria-live": "polite" }));
  requestAnimationFrame(adattaAltezzaSg);
  return sg;
}
// adattaAltezzaSg: lo spazio di lavoro prende l'altezza che resta sotto testata, linguette e strisce, sopra la barra di
// stato: la pagina non scorre, scorrono i riquadri ([U] M32, R1 del mockup v5). Sotto i 600 px le tre viste si alternano
// e l'altezza segue la finestra allo stesso modo.
function adattaAltezzaSg() {
  const sg = $("#du-sg");
  if (!sg) return;
  const barra = $("#du-barra");
  const top = sg.getBoundingClientRect().top + window.scrollY;
  const hb = barra ? barra.getBoundingClientRect().height : 0;
  // la striscia dell'esito vuota tiene il suo posto (UI46, controllo di stabilità del 21/10): il minimo dello spazio di
  // lavoro (360 px, anche nel CSS) cala di quel posto con lo spazio fra le righe del guscio, così la pagina della
  // Sorgente non scorre a un'altezza dove prima non scorreva ([U] M32)
  const av = $("#du-avviso");
  const posto = av && av.hidden ? av.getBoundingClientRect().height + (parseFloat(getComputedStyle(av.parentElement).rowGap) || 0) : 0;
  const minimo = Math.round(360 - posto);
  sg.style.minHeight = minimo + "px";
  let alt = Math.max(minimo, window.innerHeight - top - hb - 24);
  sg.style.height = alt + "px";
  // i margini del guscio (padding, barra di stato appiccicata) li misura la pagina stessa: se scorre ancora, si toglie l'eccesso
  const eccesso = document.documentElement.scrollHeight - window.innerHeight;
  if (eccesso > 0 && alt - eccesso >= minimo) {
    alt -= eccesso;
    sg.style.height = alt + "px";
  }
  adattaTelaAnteprima();
}
// adattaTelaAnteprima: la tela dell'anteprima allargata prende l'altezza che resta nel riquadro del dettaglio sotto la
// proposta, così sta tutta nello spazio che si vede e «Adatta» (vistaAdatta, che misura la tela) adatta l'albero a quello
// spazio (collaudo UI46, giro 2, C2-I4: a 1600x1000 la tela di 700 px stava in un riquadro di 655 px, e i figli restavano
// tagliati sotto). Almeno 240 px: sotto, il riquadro scorre come prima [T].
function adattaTelaAnteprima() {
  const det = $("#du-sg-dettaglio");
  const vp = det && det.querySelector("#du-anteprima .du-vp");
  if (!vp) return;
  const rd = det.getBoundingClientRect();
  const sotto = (el) => {
    const cs = getComputedStyle(el);
    return (parseFloat(cs.paddingBottom) || 0) + (parseFloat(cs.borderBottomWidth) || 0);
  };
  const riq = vp.closest(".du-riq");
  const sopra = vp.getBoundingClientRect().top - rd.top - det.clientTop + det.scrollTop;
  const alt = Math.floor(det.clientHeight - sopra - (parseFloat(getComputedStyle(det).paddingBottom) || 0) - (riq ? sotto(riq) : 0) - 4);
  vp.style.height = Math.max(240, alt) + "px";
}
window.addEventListener("resize", () => {
  if (S.passo === "sorgente") adattaAltezzaSg();
});
function barraSg() {
  const cerca = h("input", { type: "search", id: "du-sg-cerca", class: "du-sg-cerca", placeholder: "Cerca in tutte le cartelle (nome, codice, rev)", "aria-label": "Cerca nei file della RFQ", value: SG.cerca });
  let t = null;
  cerca.addEventListener("input", () => {
    clearTimeout(t);
    t = setTimeout(() => {
      SG.cerca = cerca.value;
      if (SG.cerca.trim() && SG.cartella !== "ris") {
        SG.torna = SG.cartella;
        SG.cartella = "ris";
      }
      if (!SG.cerca.trim() && SG.cartella === "ris") SG.cartella = SG.torna || "tutti";
      renderCorpo();
      const c = $("#du-sg-cerca");
      if (c) {
        c.focus();
        c.setSelectionRange(c.value.length, c.value.length);
      }
    }, 200);
  });
  cerca.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && cerca.value) {
      e.stopPropagation();
      cerca.value = "";
      cerca.dispatchEvent(new Event("input"));
    }
  });
  const ult = SG.sistema[0];
  const sistema = h("div", { class: "du-sg-sistema", id: "du-sg-sistema", "aria-live": "polite" },
    ult ? [h("span", { class: "du-lab" }, "Dal sistema"), h("span", { class: "du-sg-sistema-t" }, ult.testo),
      ult.allegato && fileDi(ult.allegato) ? btn("Vai al file", () => vaiAlFile(ult.allegato), { cls: "du-btn--sm du-btn--ghost", perche: false }) : null,
      btn("Registro (" + SG.sistema.length + ")", () => {
        SG.registroAperto = !SG.registroAperto;
        renderCorpo();
      }, { cls: "du-btn--sm du-btn--ghost", id: "du-sg-registro-apri", pressed: SG.registroAperto, perche: false })] : null,
    SG.registroAperto ? h("ol", { class: "du-sg-registro", id: "du-sg-registro" }, SG.sistema.map((m) => h("li", {}, h("span", { class: "du-muto" }, ora(m.quando) + " "), m.testo,
      m.allegato && fileDi(m.allegato) ? btn("Vai al file", () => vaiAlFile(m.allegato), { cls: "du-btn--sm du-btn--ghost", perche: false }) : null))) : null);
  return h("div", { class: "du-sg-barra" },
    h("h2", {}, "File della RFQ"), cerca,
    SG.torna && cartellaValida(SG.torna) ? btn("‹ Torna a " + cartellaNome(SG.torna), () => vaiCartella(SG.torna), { cls: "du-btn--sm du-btn--ghost", id: "du-sg-torna", perche: false }) : null,
    sistema,
    h("span", { class: "du-sg-carica" }, btn("+ Aggiungi file", () => {
      SG.caricaAperto = !SG.caricaAperto;
      renderCorpo();
    }, { cls: "du-btn--sm du-btn--pri", id: "du-sg-aggiungi", pressed: SG.caricaAperto, perche: false }), SG.caricaAperto ? h("div", { class: "du-sg-pop" }, moduloCarica()) : null));
}
function vaiAlFile(allegato) {
  const f = fileDi(allegato);
  if (!f) return;
  const dove = cartellaDelFile(f);
  vaiCartella(dove, { salto: dove !== SG.cartella, file: allegato, fuoco: "elenco", vista: "dettaglio" });
}
// cartellaDelFile: la cartella dove il file sta adesso (per «Vai al file»)
function cartellaDelFile(f) {
  if (f.stato === "scartato") return "scartati";
  if (f.stato === "altri_file") return "altri";
  const l = collegamentiDi(f);
  if (l.length) return "p:" + (l.find((x) => x.pid === S.prodotto) || l[0]).pid;
  if (eStep(f) && letto(f)) return "step";
  for (const c of sottoDc()) if (contenuto(c).some((y) => y.f.allegato_id === f.allegato_id)) return c;
  return "tutti";
}
function alberoSg() {
  const voci = albero();
  const nav = h("nav", { class: "du-sg-albero", id: "du-sg-albero", "aria-label": "Cartelle" });
  const ul = h("ul", { role: "tree", "aria-label": "Cartelle dei file" });
  const vere = voci.filter((x) => x.k);
  const mossa = SG.mossa && SG.mossa.contiPrima;
  for (const x of voci) {
    if (x.sep) {
      ul.append(h("li", { role: "presentation", class: "du-sg-sep" }, "·· " + x.sep + " ··"));
      continue;
    }
    const sel = x.k === SG.cartella;
    const n = x.conto;
    const q = SG.cerca.trim() ? distinti(righeVisibili(x.k)) : null;
    const delta = mossa && n !== null && typeof mossa[x.k] === "number" ? n - mossa[x.k] : 0;
    const li = h("li", { role: "treeitem", id: "du-sg-c-" + sid(x.k), class: "du-sg-voce du-sg-liv" + x.liv + (sel ? " du-sg-voce--sel" : "") + (q === 0 ? " du-sg-voce--attenuata" : ""),
      "aria-level": String(x.liv), "aria-selected": sel ? "true" : "false", "aria-setsize": String(vere.length), "aria-posinset": String(vere.indexOf(x) + 1),
      tabindex: sel ? "0" : "-1", "data-k": x.k, onclick: () => vaiCartella(x.k, { daAlbero: true }) },
    h("span", { class: "du-sg-voce-t" }, x.n ? h("span", { class: "du-passo-n" }, x.n) : null, h("span", { class: "du-sg-voce-nome" }, cartellaNome(x.k)),
      h("span", { class: "du-sg-conto", "data-conto": n === null ? "" : String(n) }, n === null ? "—" : q !== null ? q + "/" + n : String(n)),
      delta ? h("span", { class: "du-sg-delta", "data-delta": String(delta) }, (delta > 0 ? "+" : "") + delta) : null),
    x.seconda ? h("span", { class: "du-sg-voce-2" + (x.prodotto ? " du-mono" : "") }, x.seconda) : null);
    ul.append(li);
  }
  ul.addEventListener("keydown", tastiAlbero);
  nav.append(ul);
  return nav;
}
let timerAlbero = null;
function tastiAlbero(e) {
  const voci = [...document.querySelectorAll("#du-sg-albero [role=treeitem]")];
  const i = voci.indexOf(document.activeElement);
  if (i < 0) return;
  let j = -1;
  if (e.key === "ArrowDown") j = Math.min(voci.length - 1, i + 1);
  else if (e.key === "ArrowUp") j = Math.max(0, i - 1);
  else if (e.key === "Home") j = 0;
  else if (e.key === "End") j = voci.length - 1;
  else if (e.key === "Enter") {
    e.preventDefault();
    vaiCartella(voci[i].dataset.k, { daAlbero: true, fuoco: "elenco" });
    return;
  } else if (e.key.length === 1 && /\S/.test(e.key) && !e.ctrlKey && !e.metaKey) {
    const l = e.key.toLowerCase();
    j = voci.findIndex((v, n) => n > i && v.textContent.trim().toLowerCase().replace(/^\d/, "").trim().startsWith(l));
    if (j < 0) j = voci.findIndex((v) => v.textContent.trim().toLowerCase().replace(/^\d/, "").trim().startsWith(l));
  } else return;
  if (j < 0) return;
  e.preventDefault();
  voci.forEach((v) => v.setAttribute("tabindex", "-1"));
  voci[j].setAttribute("tabindex", "0");
  voci[j].focus();
  // da 600 px in su la scelta segue il fuoco dopo 150 ms; sotto, solo con il tocco o con Invio (critica I-10)
  clearTimeout(timerAlbero);
  if (window.innerWidth >= 600) {
    const k = voci[j].dataset.k;
    timerAlbero = setTimeout(() => {
      if (SG.cartella !== k) {
        vaiCartella(k).then(() => {
          const v = document.getElementById("du-sg-c-" + sid(k));
          if (v) v.focus();
        });
      }
    }, 150);
  }
}
// fasciaCartella: la fascia in testa all'elenco (LEGGIMI-v5 §2): l'intestazione del prodotto, la cartella ALTRO,
// «Prima conferma una sorgente», «Scegli la sorgente di ‹P› →»
function fasciaCartella(k) {
  const out = [];
  if ((k === "dc" || k.indexOf("dc-") === 0) && nessunaBozza() && k !== "dc-analisi") {
    out.push(h("div", { class: "du-striscia du-striscia--info", id: "du-sg-prima-sorgente" }, h("span", {}, "Prima conferma una sorgente: finché nessun prodotto ha la bozza i file non si associano."),
      btn("Vai a «STEP della RFQ» →", () => vaiCartella("step", { salto: true }), { cls: "du-btn--sm", perche: false })));
  }
  if (k.indexOf("dc-att:") === 0) {
    const id = k.slice(7);
    out.push(h("div", { class: "du-striscia du-striscia--info" }, h("span", {}, codiceDi(id) + " non ha ancora una sorgente: questi file aspettano la sua bozza."),
      btn("Scegli la sorgente di " + codiceDi(id) + " →", () => vaiCartella("p:" + id, { salto: true }), { cls: "du-btn--sm", perche: false })));
  }
  if (k === "altri") out.push(h("p", { class: "du-nota", id: "du-sg-altro" }, DEFINIZIONI.altri));
  if (k.indexOf("p:") === 0) out.push(intestazioneProdotto(k.slice(2)));
  if (S.erroreSmist && (k === "dc" || k === "dc-senza" || k.indexOf("p:") === 0 || k === "prodotti" || k === "step")) {
    out.push(h("div", { class: "du-striscia du-striscia--ko", id: "du-sg-smist-non-letto" }, "Lettura non riuscita: lo smistamento dei prodotti non si legge (" + S.erroreSmist + "). Non vuol dire «nessun abbinamento»: ↻ Aggiorna per riprovare."));
  }
  return out;
}
// intestazioneProdotto: la sorgente del prodotto, con data e autore (U2 §2.4), l'autorizzazione, l'esito dell'ultima
// conferma della sorgente e le azioni del prodotto («Revoca come sorgente…», «Salva nella bozza le N proposte…»). Ripiegata
// per difetto con poco spazio e dopo la conferma della sorgente (K-7); la scelta resta per la cartella.
// intestazioneAperta: l'intestazione della cartella del prodotto è aperta (scelta dell'utente, o aperta da sola con lo
// spazio per farlo e finché il prodotto non ha sorgente e bozza)
function intestazioneAperta(id) {
  const k = "p:" + id;
  const poco = window.innerWidth < 1280 || window.innerHeight < 1000;
  return SG.apertiProdotto[k] !== undefined ? SG.apertiProdotto[k] : !(poco || (sorgenteValida(id) && haBozza(id)));
}
// cardNellaCartella: la card dello STEP proposto (o dello stato della sorgente) di id per quell'allegato sta già
// nell'intestazione aperta della cartella del prodotto in vista: il dettaglio del file e la riga non la ripetono, e
// «Guarda l'anteprima» c'è una volta sola (collaudo UI46, giro 2, C2-I5, D28) [T]
function cardNellaCartella(id, allegato) {
  return SG.cartella === "p:" + id && !SG.cerca.trim() && intestazioneAperta(id) && !sorgenteDelProdotto(id) && !!stepProposto(id, allegato);
}
function intestazioneProdotto(id) {
  const k = "p:" + id;
  const fo = fonteDi(id) || {};
  const st = rigaStatoProdotto(id);
  const e = SG.esitiS8[id];
  const aperta = intestazioneAperta(id);
  const autore = (x) => (ogg(x) ? x.nome || x.utente_id || "" : x || "");
  const sorg = sorgenteDelProdotto(id);
  // G39-02 [U]: lo STEP del prodotto finito non entra fra le proposte da salvare come preassociazioni
  const prop = id === S.prodotto ? righeDi(id).filter((y) => y.stato === "proposto" && chiaveRiga(y.r, id) && preassociabile(y.f)) : [];
  const box = h("div", { class: "du-sg-prodotto", id: "du-sg-prodotto", "data-prodotto": id },
    h("div", { class: "du-sg-prodotto-t" }, h("b", { class: "du-mono" }, codiceDi(id)), pill(st.tono, st.t),
      btn(aperta ? "▴" : "▾", () => {
        SG.apertiProdotto[k] = !aperta;
        renderCorpo();
      }, { cls: "du-btn--sm du-btn--ghost", id: "du-sg-prodotto-apri", t: aperta ? "Chiudi l'intestazione" : "Apri l'intestazione", perche: false })));
  if (e && e.passi) box.append(esitoS8Riga(id, e));
  if (!aperta) return box;
  const corpo = h("div", { class: "du-sg-prodotto-c" });
  const s8 = sorgenteS8(id);
  if (sorg) {
    // la sorgente confermata da S8 (direttiva 39 §2), con data e autore della conferma; lo STEP della working è un'altra
    // cosa: cambia solo con la conferma della BOM (R5), e qui si dice soltanto
    const quando = s8.confermata_il || fo.confermata_il;
    const chi = autore(fo.confermata_da) || (typeof s8.confermata_da === "string" ? s8.confermata_da.slice(0, 8) : "");
    corpo.append(h("p", { class: "du-piccolo", id: "du-sg-fonte" }, "Sorgente: ", h("span", { class: "du-mono" }, nomeFile(sorg)),
      quando ? " · confermata il " + dataOra(quando) + (chi ? " da " + chi : "") : " · confermata",
      fo.autorizzata ? " · STEP della BOM confermata" + (fo.autorizzata_il ? " dal " + dataOra(fo.autorizzata_il) : "") : " · diventa lo STEP della BOM con «Conferma BOM»"));
    const az = h("div", { class: "du-azioni" });
    az.append(btn("Revoca come sorgente…", () => conProdotto(id, () => apriRevocaSorgente(fileDi(sorg) || { allegato_id: sorg, nome_file: nomeFile(sorg) })),
      { cls: "du-btn--sm du-btn--danger", id: "du-revoca-sorgente", no: SG.inCorsoS8[id] ? "è in corso la conferma della sorgente" : "" }));
    if (prop.length) az.append(btn("Salva nella bozza le " + prop.length + " proposte…", () => apriSalvaProposte(id, prop), { cls: "du-btn--sm", id: "du-sg-salva-tutte", no: perchéNoModifica() }));
    corpo.append(az);
    const s2 = fo.step_secondario;
    if (s2) corpo.append(secondario(fo));
    const sug = suggerimentiDopoApertura(id);
    if (sug) corpo.append(sug);
  } else if (haBozza(id)) {
    // una bozza senza sorgente confermata (partenza vuota, o la sorgente revocata): si lavora, ma la conferma della BOM
    // aspetta uno STEP confermato da S8 (direttiva 39 §1: R4 sì, R5 no)
    // aggiungi, non append: boxStepProposto e legameStorico possono non dare niente, e Element.append(null) scrive «null»
    aggiungi(corpo, [boxStepProposto(id, "sg"), h("p", { class: "du-piccolo", id: "du-sg-fonte" }, s8 ? "La sorgente confermata di " + codiceDi(id) + " è " + testoMotivoSorgente(s8.motivo) + ". " : "La bozza di " + codiceDi(id) + " non ha uno STEP confermato. ",
      "Si modifica e si salva; «Conferma BOM» richiederà uno STEP confermato con «Conferma questo STEP come sorgente di " + codiceDi(id) + "»."),
    legameStorico(fo)]);
  } else {
    // G39-02 [U]: prima della scelta fra le candidate, la proposta del motore con le sue evidenze e la conferma
    aggiungi(corpo, boxStepProposto(id, "sg"));
    const proposti = proposteSorgente(id);
    aggiungi(corpo, [h("p", { class: "du-piccolo", id: candidatePer(id).length || proposti.length ? null : "du-nessuna-candidata" }, candidatePer(id).length ? "Scegli lo STEP sorgente fra le candidate: «Guarda l'anteprima», poi «Conferma questo STEP come sorgente di " + codiceDi(id) + "». La conferma crea la bozza: non conferma la BOM." :
      // lo STEP proposto qui sopra non ha una struttura importabile: non si dice «nessuno STEP… caricalo» (collaudo UI46, I3)
      proposti.length ? "Il motore non ha una struttura da importare da " + (proposti.length === 1 ? nomeFile(proposti[0].c.allegato_id) : "questi STEP") + " per " + codiceDi(id) +
        ": confermandolo come sorgente, la bozza parte dal solo prodotto e i pezzi li aggiungi tu." :
      "Il Motore A non propone nessuno STEP per " + codiceDi(id) + ((S.modello && S.modello.motivoSenzaGruppi && id === S.prodotto && S.modello.motivoSenzaGruppi.testo) ? ": " + S.modello.motivoSenzaGruppi.testo : "") +
      ". Se lo STEP c'è, guardalo dal suo dettaglio (STEP della RFQ) e confermalo come sorgente partendo dal solo prodotto; altrimenti caricalo con «+ Aggiungi file», o parti dal solo prodotto nella Revisione."),
    legameStorico(fo)]);
  }
  if (haBozza(id) && righeDi(id).some((y) => y.stato === "in_conflitto")) corpo.append(h("p", { class: "du-nota" }, "! Le righe in conflitto bloccano «Conferma BOM» finché non le sposti o le scolleghi."));
  // con la BOM confermata e nessuna bozza non ci sono proposte da salvare: la nota non vale (collaudo UI46, B3)
  if (!(bomConfermataSu(id) && !haBozza(id))) corpo.append(h("p", { class: "du-nota" }, "Sono proposte finché non le salvi nella bozza; la BOM diventa ufficiale solo con «Conferma BOM»."));
  box.append(corpo);
  return box;
}
function esitoS8Riga(id, e) {
  // un passo che non serve non è «riuscito» (collaudo UI46: «Riuscito: associazione» con associazione non_serve)
  const fatti = arr(e.passi).filter((p) => p.esito === "fatto" || p.esito === "gia_fatto");
  const ko = arr(e.passi).filter((p) => p.esito === "rifiutato");
  const manca = arr(e.passi).filter((p) => p.esito === "non_tentato");
  const nomi = { associazione: "associazione", conferma_documento: "conferma della sorgente", bozza: "bozza" };
  const s = (l) => l.map((p) => nomi[p.passo] || p.passo).join(", ");
  const det = h("details", { class: "du-ripiega", id: "du-sg-esito-s8-dettagli" }, h("summary", {}, "Dettagli"), h("ul", { class: "du-elenco-p" }, arr(e.passi).map((p) => h("li", { "data-passo": p.passo, "data-esito": p.esito },
    (nomi[p.passo] || p.passo) + ": " + p.esito + (p.codice ? " (" + p.codice + ")" : "") + (p.testo ? " — " + p.testo : "")))));
  // completata: il testo dell'esito sta già in «Ultima mossa»; qui solo i passi (messaggio 46, D23)
  const completa = e.esito === "completata";
  return h("div", { class: "du-striscia du-striscia--" + (completa ? "ok" : "ver"), id: "du-sg-esito-s8", "data-esito": e.esito },
    h("span", {}, completa ? "✓ Sorgente confermata." : senzaGergo(e.testo) || "Conferma della sorgente parziale.",
      (fatti.length ? " Riuscito: " + s(fatti) + "." : "") + (ko.length ? " Non riuscito: " + s(ko) + "." : "") + (manca.length ? " Manca: " + s(manca) + "." : "")),
    det, btn("Chiudi", () => {
      delete SG.esitiS8[id];
      renderCorpo();
    }, { cls: "du-btn--sm du-btn--ghost", perche: false }));
}
// legameStorico: lo STEP storico univoco del prodotto (D-AR04 (iii) [U]; /dati.fonte.legame_storico), solo per un prodotto
// senza conferme di S8: si mostra distinto, non vale come sorgente confermata, e l'ingegnere lo conferma con S8
function legameStorico(fo) {
  const l = fo && ogg(fo.legame_storico) ? fo.legame_storico : null;
  if (!l) return null;
  return h("p", { class: "du-nota", id: "du-sg-legame-storico" }, pill("info", "legame storico"), " ", h("span", { class: "du-mono" }, l.allegato_id ? nomeFile(String(l.allegato_id).toLowerCase()) : "STEP del prodotto"),
    ": è lo STEP del prodotto nei dati di prima, ma non è una sorgente confermata. Per usarlo, «Conferma questo STEP come sorgente» dal suo dettaglio.");
}
// suggerimentiDopoApertura: le proposte del motore arrivate dopo l'apertura della bozza (D-PB02 A [U]; S9
// preassociazioni_proposte[]): mai applicate da sole e mai fra «da rivedere»; ognuna si accetta con un gesto esplicito (una
// preassociazione salvata con R2) o si lascia dov'è. Le proposte ambigue non ci sono: restano file da associare.
const MOTIVI_SUGGERIMENTO = { dopo_apertura: "proposto dopo l'apertura della bozza", pb_toccata: "la bozza lo preassocia a un altro nodo", ar_attiva: "registrato su un altro componente",
  conflitto: "in conflitto" };
function suggerimentiDopoApertura(id) {
  const p = s9P(id);
  const l = arr(p && p.preassociazioni_proposte);
  if (!l.length || !haBozza(id)) return null;
  const box = h("div", { class: "du-sez", id: "du-sg-suggerimenti" }, h("h4", { class: "du-lab" }, "Suggerimenti del motore (" + l.length + ")"),
    h("p", { class: "du-nota" }, "Arrivati dopo l'apertura della bozza: nessuno entra da solo. «Accetta» lo salva nella bozza sul nodo proposto (il server lo registra «dal motore»); «Su un altro nodo…» lo salva dove scegli tu (il server lo registra «corretta dall'ingegnere»); altrimenti resta un suggerimento."));
  for (const x of l) {
    const f = fileDi(x.allegato_id);
    const k = x.chiave_bozza || (x.destinazione && x.destinazione.chiave_bozza) || "";
    const nodo = k && S.lavoro && id === S.prodotto ? S.lavoro.nodi.find((n) => n.chiave_bozza === k) : null;
    const no = !k ? "il nodo della proposta non è nella bozza: «Associa a un nodo…» dal dettaglio del file" :
      id === S.prodotto && S.lavoro && !nodo ? "il nodo della proposta non è più nella bozza" :
        f && !preassociabile(f) ? "il file non si preassocia (non attivo, STEP del prodotto finito o archivio)" : id === S.prodotto ? perchéNoModifica() : "";
    box.append(h("div", { class: "du-gesto", "data-du-suggerimento": x.allegato_id, "data-motivo": x.motivo || "" },
      h("span", { class: "du-mono" }, f ? f.nome_file : nomeFile(x.allegato_id)), " → ", h("span", { class: "du-mono" }, k ? voceEffetto(k) : "?"),
      pill("prop", MOTIVI_SUGGERIMENTO[x.motivo] || x.motivo || "suggerimento"),
      btn("Accetta", () => conProdotto(id, () => preassocia(x.allegato_id, k, registroDelSuggerimento(x, k))), { cls: "du-btn--sm", id: "du-accetta-sugg-" + sid(x.allegato_id), no }),
      // G39-01 [U]: la proposta del motore spostata dall'ingegnere su un altro componente è «corretta»; il dialogo è quello di
      // «Associa a un nodo…», e il registro del suggerimento lo dichiara conRegistriDelMotore (decide il server)
      btn("Su un altro nodo…", () => conProdotto(id, () => apriAssociaNodo(f || { allegato_id: x.allegato_id, nome_file: nomeFile(x.allegato_id) }, { pid: id, suggerito: k })),
        { cls: "du-btn--sm du-btn--ghost", id: "du-altro-nodo-sugg-" + sid(x.allegato_id), no: f && !preassociabile(f) ? "il file non si preassocia (non attivo, STEP del prodotto finito o archivio)" : id === S.prodotto ? perchéNoModifica() : "" })));
  }
  return box;
}
// registroDelSuggerimento: il registro d'importazione che propone il file sul nodo, se il server lo dà con il suggerimento
// (S9, o la lettura della bozza R1: preassociazioni_proposte[].importazione, l'impronta, R39-02). Con il registro dichiarato
// e corrispondente il server dice la PB «motore» (D-PB03 A [U]); senza, «manuale». La pagina non lo inventa [T-39].
function registroDelSuggerimento(x, k) {
  if (typeof x.importazione === "string" && x.importazione) return x.importazione;
  const r1 = arr(S.bozza && S.bozza.preassociazioni_proposte).find((y) => ogg(y) && y.allegato_id === x.allegato_id && (y.chiave_bozza || "") === k);
  return r1 && typeof r1.importazione === "string" ? r1.importazione : "";
}
// apriRevocaSorgente: «Revoca come sorgente di ‹P›…» (direttiva 39 §2; EM App. I.4; G39-03 [U]): per uno STEP scelto per
// errore, del motore o dell'utente, con la causa «sorgente errata» (sorgente_errata nel corpo [T-39]). Dal messaggio 46
// [U] nessun motivo: il dialogo dice che cosa succede e la revoca parte; il motivo della riga lo scrive il server. Prima
// della conferma della BOM con quello STEP la bozza aperta nata da lui si annulla (abbandonata, la storia resta: il server
// lo fa nella stessa transazione, core/rfq/sorgente/revoca.go); dopo, la bozza e la working restano e il cambio di sorgente
// è una revisione interna («correzione della sorgente»). La pagina dichiara la bozza che mostra (bozza_id,
// versione_attesa): se è cambiata, 409 BOZZA_VERSIONE e niente scritto. Non è la sostituzione per una revisione del
// cliente («Usa nuovo STEP»); non tocca il documento né il NAS; fino a una nuova conferma della sorgente «Conferma BOM»
// resta bloccata.
async function apriRevocaSorgente(f) {
  const id = S.prodotto;
  const cod = codiceProdotto();
  if (!f || !f.allegato_id) return avvisa("errore", "Lo STEP sorgente non è fra i file della Sorgente: ↻ Aggiorna.");
  const conBozza = bozzaLavorabile();
  if (conBozza && (S.coda.length || S.invio)) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "La revoca non parte: le modifiche della bozza non sono salvate.");
  }
  const sb = conBozza ? sorgenteDellaBozza(id) : null;
  const nata = !!(sb && sb.allegato_id === String(f.allegato_id).toLowerCase());
  const versione = conBozza ? S.bozza.versione : 0;
  apriFinestra("Revoca " + (f.nome_file || nomeFile(f.allegato_id)) + " come sorgente di " + cod, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Serve a correggere uno STEP scelto per errore, dal motore o da te: la revoca si registra con la causa «sorgente errata». Non cambia la BOM confermata, il documento né la copia sul NAS: dice che questo STEP non è più la sorgente confermata di " + cod + "."),
    // collaudo UI46: il dialogo dice il caso che vale adesso (la BOM confermata o no con questo STEP), non tutti e due
    nata ? h("p", { class: "du-esito du-esito--ver", id: "f-revoca-bozza", "data-nata": "1" }, "La bozza aperta di " + cod + " (versione " + versione + ") è nata da questo STEP. " +
      (bomConfermataSu(id, f.allegato_id) ? "La BOM è già confermata con questo STEP: la bozza e la BOM confermata restano, e il cambio di sorgente sarà una revisione interna (correzione della sorgente), non una revisione del cliente." :
        "La BOM non è ancora confermata con lui: la revoca annulla la bozza (resta nella storia), e la bozza nuova nascerà dalla sorgente giusta che confermerai.")) :
      h("p", { class: "du-nota", id: "f-revoca-bozza", "data-nata": "0" }, conBozza ? "La bozza aperta di " + cod + " non è nata da questo STEP: la revoca non la cambia." : "Nessuna bozza aperta di " + cod + "."),
    h("p", { class: "du-nota" }, "Finché uno STEP non è confermato di nuovo come sorgente, «Conferma BOM» resta bloccata. Per uno STEP nuovo del cliente non si revoca: si usa «Usa nuovo STEP per " + cod + "…».")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Revoca come sorgente", danger: true, id: "f-ok", spento: () => {
    if (conBozza && S.bozza && S.bozza.versione !== versione) return "la bozza è cambiata: riapri il dialogo";
    return "";
  }, fai: async () => {
    const corpo = { prodotto_id: id, sorgente_errata: true };
    if (conBozza) corpo.bozza = { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione };
    const r = await inSerie("sorgente:" + id, () => P.revocaSorgente(f.allegato_id, corpo));
    return esitoRevocaSorgente(r, id, f);
  } }]);
}
// riprovaRevocaSorgente: la revoca verificata «non registrata», rimandata dalla striscia con lo stesso corpo e la stessa
// chiave (il ponte ha il corpo di allora)
async function riprovaRevocaSorgente(s) {
  const id = String(s.risorsa || "").slice("sorgente:".length);
  const a = s.u || "";
  const r = await inSerie(s.risorsa, () => P.riprovaSospeso(s.risorsa));
  await esitoRevocaSorgente(r, id, fileDi(a) || { allegato_id: a, nome_file: nomeFile(a) });
  renderStrisce();
}
async function esitoRevocaSorgente(r, id, f) {
  if (r.esito === "ok") {
    const d = r.dati || {};
    const testo = senzaGergo(d.testo) || "Sorgente revocata per " + codiceDi(id) + "." + (d.bozza_abbandonata ? " La bozza nata da questo STEP è annullata (resta nella storia)." : "");
    const riletta = await rileggiDopoUnGesto(["sorgente", "bozza"], "La revoca della sorgente è registrata.");
    // la bozza annullata (G39-03) cambia anche lo smistamento
    await caricaSmistamento();
    togliTutteLeSpunte();
    // l'esito della conferma di prima non vale più: niente «Sorgente confermata» accanto a «sorgente revocata» (D10)
    delete SG.esitiS8[id];
    registraMossa(testo, { allegato: f.allegato_id, esito: "ok" });
    avvisa("ok", testo + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."), "prodotto");
    render();
    return true;
  }
  if (r.esito === "da_verificare") {
    avvisa("da_verificare", "Esito della revoca da verificare: verifico con la stessa chiave.");
    verificaConRiprova(r.chiave, async (j) => {
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        await esitoRevocaSorgente({ esito: "ok", dati: j.esito || {} }, id, f);
        return true;
      }
      return false;
    });
    renderStrisce();
    return true;
  }
  if (r.codice === "BOZZA_VERSIONE") {
    // la bozza dichiarata non è più quella (G39-03: la revoca la annullerebbe): niente scritto, si rilegge
    chiudiFinestra();
    await rileggiTenendoDaParte();
    await caricaSmistamento();
    render();
    avvisa("conflitto", "La bozza è cambiata altrove: la rileggo; niente è stato revocato. Ripeti la revoca se serve ancora.");
    return true;
  }
  const testi = { MOTIVO_OBBLIGATORIO: TESTO_MOTIVO_VECCHIO, CHIAVE_RIUSATA: "La chiave del tentativo è già usata per un altro corpo: rileggi e riprova." };
  const nessuna = r.motivo === "nessuna_sorgente_da_revocare" ? "Lo STEP non è (più) una sorgente confermata e valida di " + codiceDi(id) + ": non c'è niente da revocare. ↻ Aggiorna." : "";
  const t = nessuna || testi[r.codice] || r.testo || "La revoca non è riuscita.";
  const err = $("#du-finestra-errore");
  if (err) err.textContent = t + (r.codice ? " (" + r.codice + ")" : "");
  avvisa(r.esito === "no" || r.esito === "conflitto" ? "no" : "errore", t);
  return false;
}
function elencoSg() {
  const k = SG.cartella;
  const righe = righeVisibili(k);
  const tutte = contenuto(k);
  const n = conto(k);
  const mm = memo(k);
  // le spunte valgono solo per la cartella e solo per i file in vista; una spunta nascosta non riceve gesti (I-12)
  for (const a of [...mm.spunte]) if (!tutte.some((y) => y.f.allegato_id === a)) mm.spunte.delete(a);
  const nascoste = [...mm.spunte].filter((a) => !righe.some((y) => y.f.allegato_id === a));
  const sec = h("section", { class: "du-sg-elenco", id: "du-sg-elenco", "aria-label": "Elenco della cartella" });
  if (S.erroreSorgente) {
    // niente elenco vecchio mostrato come attuale (MAT §5.4, «letture: 5xx»): la lista non c'è finché S1 non si rilegge
    sec.append(h("div", { class: "du-striscia du-striscia--ko", id: "du-file-non-letti" }, "Lettura non riuscita: l'elenco dei file non è aggiornato (" + S.erroreSorgente + "). ↻ Aggiorna per riprovare."),
      h("div", { class: "du-striscia du-striscia--ko", id: "du-step-non-letti" }, "Lettura non riuscita: gli STEP della RFQ non si leggono. ↻ Aggiorna per riprovare."));
    return sec;
  }
  sec.append(h("div", { class: "du-sg-elenco-t" },
    h("button", { type: "button", class: "du-btn du-btn--sm du-btn--ghost du-sg-indietro", onclick: () => {
      SG.vista = "cartelle";
      renderCorpo();
    } }, "‹ Cartelle"),
    h("h3", { id: "du-sg-titolo" }, cartellaNome(k), " ", h("span", { class: "du-muto" }, n === null ? "" : SG.cerca.trim() ? righe.length + "/" + tutte.length : String(n))),
    h("span", { class: "du-muto du-piccolo du-sg-def", title: definizioneDi(k) }, "ⓘ ", breve(definizioneDi(k), 140))));
  for (const x of fasciaCartella(k)) sec.append(x);
  const spuntate = righe.filter((y) => mm.spunte.has(y.f.allegato_id));
  if (spuntate.length || nascoste.length) sec.append(barraGruppo(spuntate, nascoste));
  const ul = h("ul", { class: "du-sg-righe", role: "grid", "aria-multiselectable": "true", "aria-labelledby": "du-sg-titolo", "aria-rowcount": String(righe.length) });
  // le righe a gruppi: nella cartella «Da controllare» e in «Prodotti» un'intestazione per gruppo
  let grp = "";
  const visti = new Set();
  let primo = true;
  for (const y of righe) {
    if ((k === "dc" || k === "prodotti") && y.grp !== grp) {
      grp = y.grp;
      ul.append(h("li", { role: "presentation", class: "du-sg-gruppo" }, nomeGruppo(grp)));
    }
    const id = visti.has(y.f.allegato_id) ? "du-file-" + sid(y.f.allegato_id) + "-" + sid(y.pid || grp) : "du-file-" + sid(y.f.allegato_id);
    visti.add(y.f.allegato_id);
    const attivo = mm.file ? mm.file === y.f.allegato_id : primo;
    ul.append(rigaSg(y, k, id, attivo, mm));
    primo = false;
  }
  // la riga fantasma dell'ultima mossa: il file uscito dalla cartella, con «Vai al file» (R4)
  const fm = SG.mossa && SG.mossa.fantasma;
  if (fm && fm.cartella === k && !righe.some((y) => y.f.allegato_id === fm.allegato) && fileDi(fm.allegato)) {
    ul.append(h("li", { role: "row", class: "du-sg-riga du-sg-riga--fantasma", id: "du-sg-fantasma", tabindex: "-1", "data-allegato": fm.allegato,
      onclick: () => vaiAlFile(fm.allegato), onkeydown: (e) => {
        if (e.key === "Enter") vaiAlFile(fm.allegato);
      } }, h("span", { role: "gridcell" }, "↪ " + nomeFile(fm.allegato) + " → " + fm.dove), h("span", { role: "gridcell" }, btn("Vai al file", () => vaiAlFile(fm.allegato), { cls: "du-btn--sm du-btn--ghost", perche: false }))));
  }
  if (!righe.length && !(fm && fm.cartella === k)) {
    const vuoti = { "dc-senza": "Nessun file senza proposta.", "dc-analisi": "Nessun file in analisi.", "dc-arch": "Nessun archivio ricevuto.", altri: "Nessun file in Altri file.", scartati: "Nessun file scartato.",
      step: "Nessuno STEP attivo e letto: carica lo STEP del prodotto con «+ Aggiungi file».", dc: S.smist ? "Niente da controllare." : "Lo smistamento non è letto.", ris: "Nessun file corrisponde alla ricerca." };
    let t = vuoti[k] || "Nessun file.";
    if (k.indexOf("p:") === 0) {
      const p = s9P(k.slice(2));
      t = !haBozza(k.slice(2)) ? "Nessuna candidata per " + codiceDi(k.slice(2)) + "." : p && p.calcolo && p.calcolo.stato !== "pronto" ? "Bozza creata; smistamento in calcolo." : !p ? "Smistamento non letto." : "Nessun abbinamento.";
    }
    ul.append(h("li", { role: "presentation", class: "du-muto du-piccolo du-sg-vuoto" }, t));
  }
  ul.addEventListener("keydown", tastiElenco);
  sec.append(ul);
  // in fondo alla cartella di un prodotto con la bozza, gli «Altri STEP candidati» ripiegati (K-8)
  if (k.indexOf("p:") === 0 && haBozza(k.slice(2))) {
    const id = k.slice(2);
    const altri = candidatePer(id).filter((c) => c.allegato_id !== sorgenteDelProdotto(id));
    if (altri.length) sec.append(h("details", { class: "du-ripiega du-sg-altri-step", id: "du-sg-altri-step" }, h("summary", {}, "Altri STEP candidati (" + altri.length + ")"),
      h("ul", { class: "du-sg-righe" }, altri.map((c) => h("li", { class: "du-sg-riga", "data-allegato": c.allegato_id }, h("span", { class: "du-mono" }, nomeFile(c.allegato_id)),
        h("span", { class: "du-azioni" }, azioniCandidatura(c).map((a) => bottoneAz(a, a.domId))))))));
  }
  return sec;
}
function nomeGruppo(g) {
  if (g === "senza") return "Senza proposta";
  if (g === "analisi") return "In analisi";
  if (g === "archivi") return "Archivi ricevuti";
  if (g.indexOf("attesa:") === 0) return "Attendono " + codiceDi(g.slice(7));
  if (g.indexOf("ric:") === 0) return "Da ricollocare · " + codiceDi(g.slice(4));
  if (g.indexOf("smist:") === 0) return codiceDi(g.slice(6));
  return g;
}
// rigaSg: una riga dell'elenco (role=row): casella, nome e formato, segni dello stato, destinazione, gesto principale.
// Le pillole di prima (origine, analisi, codice, «da ricodificare», preassociazione, proposta) e la traccia di Altri file
// e Scartati restano nella riga: a 1440 px ci stanno (K-6), e il dettaglio le ripete.
// la provenienza di una riga di S9 o di una PB (D-PB03 A [U]: motore, corretta, manuale, legacy)
const PROVENIENZE = { motore: "dal motore", corretta: "corretta dall'ingegnere", manuale: "messa a mano", legacy: "dai dati di prima" };
// statoSorgenteRiga: per la riga di S9 di uno STEP legato come sorgente (o legame storico) di pid, lo stato della sorgente
// di statiSorgente; null per le altre righe. testoStatoSorgenteRiga: la pillola, come STATI_RIGA ([segno, testo, tono]).
function statoSorgenteRiga(f, pid, statoRiga) {
  return statoRiga === "legato_come_sorgente" || statoRiga === "legame_storico" ? statoSorgentePer(f.allegato_id, pid) : null;
}
function testoStatoSorgenteRiga(xs) {
  if (xs.stato === "confermata") return STATI_RIGA.legato_come_sorgente;
  return ["", xs.stato === "revocata" ? "sorgente revocata" : xs.stato === "sostituita" ? "sostituita come sorgente" : "sorgente non valida", "ver"];
}
function rigaSg(y, k, id, attivo, mm) {
  const f = y.f;
  const spunt = mm.spunte.has(f.allegato_id);
  const cls = "du-sg-riga du-fr" + (f.stato === "altri_file" ? " du-fr--altri" : f.stato === "scartato" ? " du-fr--scartato" : "") + (attivo ? " du-sg-riga--attiva" : "") + (y.voce ? " du-sg-riga--voce" : "");
  // lo STEP sorgente: lo stato della sorgente per questo prodotto, non quello della riga (statiSorgente, messaggio 46);
  // detto dalla pillola della riga, non anche fra le pillole del file (rilievo della revisione: «◆ sorgente confermata» e
  // «◆ sorgente di ‹P›» insieme)
  const xs = y.r && eStep(f) ? statoSorgenteRiga(f, y.pid, y.stato) : null;
  const meta = metaFile(f, xs ? y.pid : "");
  let stato = null;
  if (y.r) {
    const s = STATI_RIGA[y.stato] || ["", y.stato, "neu"];
    const cid = testoIdc(y.r.identificativo_cliente);
    stato = h("span", { class: "du-sg-dest" }, xs ? pillStatoSorgente(xs, f) : pill(s[2], (s[0] ? s[0] + " " : "") + s[1]), " → ", h("span", { class: "du-mono" }, y.r.codice_mostrato || "(senza codice)"),
      y.r.rev_mostrata ? " rev " + y.r.rev_mostrata : "", cid ? h("span", { class: "du-idc", title: TITOLO_IDC, "data-du-idc": cid }, " · id. cliente ", h("span", { class: "du-mono" }, cid)) : null,
      arr(y.r.condiviso_con).length ? pill("info", "⇄ anche " + arr(y.r.condiviso_con).map((p) => codiceDi(String(p).toLowerCase())).join(", ")) : null,
      // S9 (direttiva 39): la proposta con più destinazioni non diventa un suggerimento; la provenienza della riga
      y.r.motivo === "proposta_ambigua" ? pill("ver", "proposta ambigua: più destinazioni") : null,
      // G39-02 [U]: la PB su uno STEP candidato sorgente del finito (S9 in_conflitto, motivo step_candidato_del_finito)
      y.r.motivo === "step_candidato_del_finito" ? pill("ko", "STEP candidato sorgente del finito: si conferma come sorgente, non è un documento del pezzo") : null,
      y.r.provenienza ? h("span", { class: "du-muto du-piccolo", "data-du-provenienza": y.r.provenienza }, " · " + (PROVENIENZE[y.r.provenienza] || y.r.provenienza)) : null,
      k === "prodotti" || (k.indexOf("p:") !== 0 && collegamentiDi(f).length > 1) ? pill("neu", codiceDi(y.pid)) : null);
  } else if (y.x) stato = h("span", { class: "du-sg-dest" }, pill("att", "attende la sorgente di " + codiceDi(y.pid)), y.x.codice_mostrato ? [" → ", h("span", { class: "du-mono" }, y.x.codice_mostrato)] : null);
  else if (y.v) stato = h("span", { class: "du-sg-dest" }, pill("bozza", "↺ da ricollocare"), " " + motivoSvincolo(y.v.motivo));
  else if (y.c) stato = h("span", { class: "du-sg-dest" }, pillCandidata(y.c, f));
  else if (k === "dc-senza" || (k === "dc" && y.grp === "senza")) {
    const mot = motivoDaAssociare(f.allegato_id);
    if (mot) stato = h("span", { class: "du-sg-dest du-muto" }, "motivo: " + mot);
  }
  for (const p of attesaDi(f.allegato_id)) if (!y.x) meta.push(pill("att", "attende la sorgente di " + p.codice));
  const A = azioniFile(f, { pid: y.pid, cartella: k });
  let gesto = null;
  if (k === "step" || y.c) {
    // nelle cartelle degli STEP il gesto in riga è la prima candidatura: «Guarda l'anteprima per ‹P›»
    const cc = y.c ? [y.c] : candidateDi(f.allegato_id);
    const c0 = cc.find((c) => c.prodotto_id === S.prodotto) || cc[0];
    if (c0 && !cardNellaCartella(c0.prodotto_id, f.allegato_id)) gesto = bottoneAz(azioniCandidatura(c0)[0], azioniCandidatura(c0)[0].domId);
  } else if (A.prin) {
    gesto = bottoneAz(A.prin, "du-r-" + A.prin.id + "-" + sid(f.allegato_id), true);
  }
  const traccia = tracciaFile(f);
  const idRiga = k === "step" ? "du-step-" + sid(f.allegato_id) : id;
  const riga = h("li", { role: "row", class: cls, id: idRiga, "data-allegato": f.allegato_id, "data-stato": f.stato || "attivo",
    "aria-selected": spunt ? "true" : "false", "aria-current": attivo ? "true" : null, tabindex: attivo ? "0" : "-1",
    onclick: (e) => {
      if (e.target.closest("button, a, input")) return;
      scegliFile(f.allegato_id);
    } },
  h("span", { role: "gridcell", class: "du-sg-cella-spunta" }, h("input", { type: "checkbox", class: "du-sg-spunta", id: idRiga + "-spunta", "aria-label": "Seleziona " + f.nome_file, checked: spunt, "data-allegato": f.allegato_id,
    onchange: (e) => {
      if (e.target.checked) mm.spunte.add(f.allegato_id);
      else mm.spunte.delete(f.allegato_id);
      renderCorpo();
      annuncia(mm.spunte.size + " file selezionati: le azioni di gruppo sono nella barra sopra l'elenco e nel dettaglio.");
    } })),
  h("span", { role: "gridcell", class: "du-sg-cella-file" }, h("div", { class: "du-fr-nome" }, (y.voce ? "↳ " : "") + f.nome_file, eArchivio(f) ? " " : null, eArchivio(f) ? pill("neu", formatoDi(f)) : null),
    stato, k === "step" ? sintesiCandidate(f.allegato_id) : null, h("div", { class: "du-fr-meta" }, meta), traccia ? h("div", { class: "du-fr-traccia", "data-du-traccia": "1" }, traccia) : null),
  h("span", { role: "gridcell", class: "du-sg-cella-gesto" }, gesto));
  return riga;
}
// metaFile: le pillole del file (origine, analisi, codice letto, stato dell'associazione registrata, «da ricodificare»,
// preassociazione e proposte della bozza che si vede)
// salta: il prodotto il cui stato della sorgente la riga dice già (rigaSg)
function metaFile(f, salta) {
  const meta = [pill("neu", f.origine === "interna" ? "interna" : f.canale === "posta" ? "posta" : "del cliente"), pill(f.analisi && f.analisi.stato === "errore" ? "ko" : inAnalisiF(f) ? "info" : "neu", (inAnalisiF(f) ? "⟳ " : "") + testoAnalisi(f))];
  if (f.proposta && f.proposta.codice) meta.push(h("span", { class: "du-mono" }, f.proposta.codice + (f.proposta.rev ? " rev " + f.proposta.rev : "")));
  if (f.scartato_legacy) meta.push(pill("neu", "Scartato (gesto precedente)"));
  const dove = destinazioneFile(f);
  // lo STEP sorgente di un prodotto finito: un solo stato per prodotto (la tabella di statiSorgente, messaggio 46 punto
  // 4), al posto di «✓ confermato» e senza le proposte del registro che lo citano come 3D della radice (D10, D11)
  const ss = eStep(f) ? statiSorgente(f.allegato_id) : [];
  for (const x of ss) if (!salta || x.pid !== salta) meta.push(pillStatoSorgente(x, f));
  // l'associazione registrata dalle righe di S9 (mai la copia di S1); un documento confermato si dice comunque
  const ar = arDi(f.allegato_id);
  if ((ar || f.documento) && !ss.length) meta.push(pill("info", f.documento || (ar && ar.documento) ? "✓ confermato" : "● associato"));
  if (dove.ricodifica) meta.push(pill("ver", "da ricodificare", "il componente del file ha una ricodifica nella bozza: alla conferma il file segue il codice nuovo (MAT §4.3 riga 7)"));
  if (dove.pb) meta.push(pill("bozza", "◈ preassociato a " + dove.pb));
  // uno STEP candidato sorgente di un finito non è un documento da preassociare: la proposta del registro non si mostra
  const delFinito = ss.length || (eStep(f) && S.smist && stepSorgenteDelFinito(f));
  if (!delFinito) for (const x of dove.pa) meta.push(pill("prop", "◇ proposto per " + x));
  for (const x of dove.noninc) meta.push(pill("neu", "◇ proposto per " + x + ": non richiesta, nodo non incluso (bozza)"));
  if (f.conservazione && f.conservazione.stato === "in_copia") meta.push(pill("info", "copia in corso…"));
  if (f.conservazione && f.conservazione.stato === "trattenuta") meta.push(pill("neu", "c'è una copia del cockpit nella cartella ALTRO" + (f.altri_file && f.altri_file.nome_sul_nas ? ": " + f.altri_file.nome_sul_nas : "")));
  return meta;
}
// tracciaFile: chi e quando per Scartati e Altri file, lo stato della rimozione e della copia in ALTRO (U2 §3.7, §3.12)
const TESTI_RIMOZIONE_NAS = {
  scrittura_nas_spenta: "Scarto registrato; la copia in ALTRO sul NAS è ancora da togliere: la scrittura sul NAS è spenta su questo server. Quando sarà accesa, usa «Riprova la rimozione dalla cartella ALTRO».",
  nas_irraggiungibile: "Scarto registrato; la copia in ALTRO sul NAS è ancora da togliere: il NAS non risponde. Si riprova da solo.",
  file_occupato: "Scarto registrato; la copia in ALTRO sul NAS è aperta da un altro programma: si toglie appena si chiude.",
  copia_in_volo: "Scarto registrato; la copia verso ALTRO è ancora in corso: si toglie appena finisce.",
  radice_cambiata: "Scarto registrato; il file in ALTRO non è stato tolto: serve una bonifica dell'amministratore.",
  fuori_cartella: "Scarto registrato; il file in ALTRO non è stato tolto: serve una bonifica dell'amministratore.",
  nome_ripreso: "Scarto registrato; il file in ALTRO non è stato tolto: il suo nome di prima è stato ripreso da un altro file e la copia è nascosta nella stessa cartella con un nome di servizio. Serve una bonifica dell'amministratore.",
  rimosso: "File scartato ed eliminato anche dalla cartella ALTRO della RFQ.",
  gia_assente: "File scartato. La copia non è stata trovata al percorso registrato: la cartella può essere stata spostata o rinominata.",
  non_nostro: "File scartato. Il file in ALTRO non l'ha creato il cockpit: resta dov'è.",
  percorso_di_un_documento: "File scartato. Il file in ALTRO è un documento confermato: resta dov'è.",
  file_cambiato: "File scartato. Il file in ALTRO è stato cambiato da una persona: il cockpit non lo tocca.",
};
function tracciaFile(f) {
  const t = [];
  if (f.stato === "scartato") {
    const ev = arr(f.eventi).find((e) => e.azione === "scarta") || f.scarto || null;
    // messaggio 46 [U]: il motivo non si mostra (né quello scritto nelle pagine di prima né la traccia del gesto): chi e quando
    if (ev) t.push("Scartato da " + (chiE(ev.autore || ev.autore_id) || "?") + " il " + dataOra(ev.quando) + ".");
    t.push(testoRimozione(f));
    const rn = f.rimozione_nas || (f.conservazione && f.conservazione.rimozione);
    if (ogg(rn) && TESTI_RIMOZIONE_NAS[rn.motivo]) t.push(TESTI_RIMOZIONE_NAS[rn.motivo]);
    t.push(f.canale === "posta" ? "Per riaverlo: salvalo dalla mail e ricaricalo." : "Per riaverlo: ricaricalo.");
  }
  if (f.stato === "altri_file") {
    const af = ogg(f.altri_file) ? f.altri_file : arr(f.eventi).find((e) => e.azione === "altri_file") || null;
    if (af && (af.autore || af.autore_id || af.quando)) t.push("In Altri file da " + (chiE(af.autore || af.autore_id) || "?") + " il " + dataOra(af.quando) + ".");
    const c = f.conservazione;
    if (c) {
      const motivo = c.motivo || c.causa || "";
      t.push(c.stato === "copiata" ? "Copia nella cartella ALTRO della RFQ verificata" + (af && af.nome_sul_nas ? " (" + af.nome_sul_nas + ")" : "") + "." : c.stato === "in_copia" ? "Copia verso la cartella ALTRO in corso." :
        c.stato === "cambiata" ? "La copia nella cartella ALTRO è stata cambiata sul NAS: non è più il file conservato." : c.stato === "mancante" ? "La copia non si trova al percorso registrato nella cartella ALTRO: la cartella può essere stata spostata o rinominata." :
          c.stato === "errore" ? "Copia nella cartella ALTRO non riuscita: " + (motivo || "motivo non dato") + "." : "Copia: " + c.stato + ".");
    }
  }
  return t.filter(Boolean).join(" ").trim();
}
function tastiElenco(e) {
  const righe = [...document.querySelectorAll("#du-sg-elenco [role=row]")];
  const i = righe.indexOf(document.activeElement);
  if (i < 0) {
    if (e.key === "Escape") togliSpunte();
    return;
  }
  let j = -1;
  if (e.key === "ArrowDown") j = Math.min(righe.length - 1, i + 1);
  else if (e.key === "ArrowUp") j = Math.max(0, i - 1);
  else if (e.key === "Home") j = 0;
  else if (e.key === "End") j = righe.length - 1;
  else if (e.key === "PageDown") j = Math.min(righe.length - 1, i + 10);
  else if (e.key === "PageUp") j = Math.max(0, i - 10);
  else if (e.key === " ") {
    e.preventDefault();
    const c = righe[i].querySelector(".du-sg-spunta");
    if (c) c.click();
    return;
  } else if (e.key === "Enter") {
    e.preventDefault();
    const a = righe[i].dataset.allegato;
    if (righe[i].id === "du-sg-fantasma") return vaiAlFile(a);
    scegliFile(a, true);
    return;
  } else if (e.key === "Escape") {
    togliSpunte();
    return;
  } else if ((e.key === "a" || e.key === "A") && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    const mm = memo();
    for (const r of righe) if (r.dataset.allegato && r.id !== "du-sg-fantasma") mm.spunte.add(r.dataset.allegato);
    renderCorpo();
    return;
  } else return;
  e.preventDefault();
  righe.forEach((r) => r.setAttribute("tabindex", "-1"));
  righe[j].setAttribute("tabindex", "0");
  righe[j].focus();
  if (righe[j].dataset.allegato && righe[j].id !== "du-sg-fantasma") {
    memo().file = righe[j].dataset.allegato;
    const d = $("#du-sg-dettaglio");
    if (d) d.replaceWith(dettaglioSg());
  }
}
// togliTutteLeSpunte: dopo un gesto riuscito sulla sorgente le spunte di ogni cartella si tolgono (messaggio 46, D20)
function togliTutteLeSpunte() {
  for (const k of Object.keys(SG.memo)) SG.memo[k].spunte.clear();
}
let spunteTolte = null;
function togliSpunte() {
  const mm = memo();
  if (!mm.spunte.size) return;
  spunteTolte = { cartella: SG.cartella, spunte: new Set(mm.spunte) };
  mm.spunte.clear();
  renderCorpo();
  // il pulsante sparisce con le spunte: il fuoco va sulla riga attiva, senza scorrere (UI46, controllo di stabilità)
  if (document.activeElement === document.body || !document.activeElement) {
    const r = (mm.file && document.querySelector("#du-sg-elenco [role=row][data-allegato='" + mm.file + "']")) || document.querySelector("#du-sg-elenco [role=row][tabindex='0']");
    if (r) r.focus({ preventScroll: true });
  }
  avvisa("ok", "Spunte tolte.");
  setTimeout(() => {
    spunteTolte = null;
  }, 5000);
}
function scegliFile(allegato, dettaglio) {
  memo().file = allegato;
  SG.caricaAperto = false;
  if (S.anteprima && SG.larga) SG.larga = false;
  if (window.innerWidth < 600 || dettaglio) SG.vista = "dettaglio";
  renderCorpo();
  fuocoSuRiga(allegato);
  if (dettaglio) {
    const d = $("#du-sg-dettaglio h3");
    if (d) d.focus();
  }
}
// barraGruppo: le azioni di gruppo (LEGGIMI-v5 §4): valgono solo se ammesse per tutti i file spuntati in vista;
// altrimenti spente con la ragione che nomina i file. Non si fanno in gruppo «Associa», «Sposta», «Apri», «Scollega» e
// i gesti sulle sorgenti.
// Messaggio 46 (D18, D19 [T]): la barra mostra solo i gesti che valgono per tutti i file spuntati; gli altri non diventano
// un muro di pulsanti spenti: stanno in «n gesti non disponibili», ripiegato, con il perché in breve (e nel titolo, al
// passaggio del mouse). Un gesto che non riguarda nessuno dei file spuntati non si nomina. Con un file solo, in testa il
// suo gesto principale (per uno STEP candidato sorgente del prodotto, la conferma come sorgente).
function barraGruppo(spuntate, nascoste, suf) {
  suf = suf || "";
  const k = SG.cartella;
  const ctx = (y) => ({ pid: y.pid, cartella: k });
  const nonVale = [];
  const ogni = (gruppo) => {
    const ok = [];
    const no = [];
    let nessuno = true;
    for (const y of spuntate) {
      const A = azioniFile(y.f, ctx(y));
      const a = [A.prin].concat(A.altre).concat(A.spente).find((x) => x && x.g === gruppo && (!x.pid || !y.pid || x.pid === y.pid));
      if (a) nessuno = false;
      if (a && !a.no) ok.push({ y, a });
      else no.push({ nome: y.f.nome_file, motivo: senzaGergo(a && a.no ? a.no : "non si applica a questo file") });
    }
    return { ok, no, nessuno };
  };
  const breveNo = (no) => {
    const per = new Map();
    for (const x of no) per.set(x.motivo, (per.get(x.motivo) || []).concat(x.nome));
    return [...per.entries()].map(([m, nomi]) => (spuntate.length === 1 ? m : (nomi.length === 1 ? nomi[0] : nomi.length + " file") + ": " + m)).join("; ");
  };
  const g = (gruppo, testo, fai, id) => {
    const r = ogni(gruppo);
    if (r.nessuno) return null;
    if (r.no.length) {
      nonVale.push({ t: testo, perche: breveNo(r.no), id });
      return null;
    }
    return btn(testo + (spuntate.length > 1 ? " (" + spuntate.length + ")" : ""), () => fai(r.ok), { cls: "du-btn--sm" + (gruppo === "scarta" ? " du-btn--danger" : ""), id, perche: false });
  };
  const gesti = [g("salva", "Salva nella bozza", (l) => mossaGruppo("salva", l), "du-g-salva" + suf),
    g("togli", "Togli dalla bozza", (l) => mossaGruppo("togli", l), "du-g-togli" + suf),
    g("altri", "Metti in Altri file…", (l) => apriGruppoFile("altri", l.map((x) => x.y.f)), "du-g-altri" + suf),
    g("riattiva", "Riattiva…", (l) => apriGruppoFile("riattiva", l.map((x) => x.y.f)), "du-g-riattiva" + suf),
    g("scarta", "Scarta…", (l) => apriGruppoFile("scarta", l.map((x) => x.y.f)), "du-g-scarta" + suf)];
  const princ = spuntate.length === 1 ? principaleDi(spuntate[0], ctx(spuntate[0])) : null;
  const perche = nonVale.map((x) => x.t + ": " + x.perche).join("\n");
  return h("div", { class: "du-sg-gruppo-barra", id: "du-sg-gruppo" + suf, role: "toolbar", "aria-label": "Azioni di gruppo" },
    h("span", { class: "du-lab" }, spuntate.length + " selezionati"),
    princ && !princ.no ? btn(princ.t, princ.fai, { cls: "du-btn--sm du-btn--pri", id: "du-g-principale" + suf, perche: false }) : null,
    gesti,
    nonVale.length ? h("details", { class: "du-ripiega du-sg-nonvale", id: "du-sg-nonvale" + suf }, h("summary", { title: perche }, nonVale.length + (nonVale.length === 1 ? " gesto non disponibile" : " gesti non disponibili")),
      h("ul", { class: "du-elenco-p du-piccolo" }, nonVale.map((x) => h("li", { "data-gesto": x.id }, h("b", {}, x.t), ": ", x.perche)))) : null,
    btn("Togli le spunte", togliSpunte, { cls: "du-btn--sm du-btn--ghost", id: "du-sg-togli-spunte" + (suf || ""), perche: false }),
    nascoste.length ? h("span", { class: "du-perche", id: "du-sg-nascoste" }, nascoste.length + " spuntati nascosti dal filtro", " ",
      btn("Mostrali", () => {
        SG.cerca = "";
        renderCorpo();
      }, { cls: "du-btn--sm du-btn--ghost", perche: false }),
      btn("Togli le loro spunte", () => {
        for (const a of nascoste) memo().spunte.delete(a);
        renderCorpo();
      }, { cls: "du-btn--sm du-btn--ghost", perche: false })) : null);
}
// principaleDi: il gesto principale di un file spuntato da solo, per la barra (D19): per uno STEP che ha la conferma di S8
// valida del prodotto scelto il suo principale («Vai alla Revisione…»); per uno STEP candidato o proposto per il prodotto
// scelto la conferma come sorgente; per gli altri file il principale della matrice, se non è già un gesto di gruppo
function principaleDi(y, ctx) {
  const f = y.f;
  const A = azioniFile(f, ctx);
  if (eStep(f) && eAttivo(f) && !statoSorgentePer(f.allegato_id, S.prodotto)) {
    const sp = stepProposto(S.prodotto, f.allegato_id);
    const cc = candidateDi(f.allegato_id);
    const c0 = (sp && sp.c && sp.c.gruppo ? sp.c : null) || cc.find((c) => c.prodotto_id === S.prodotto);
    const a = c0 ? azioniCandidatura(c0).find((x) => x.id.indexOf("conferma-") === 0) : null;
    if (a) return a;
  }
  return A.prin && !A.prin.g && A.prin.id !== "apri" ? A.prin : null;
}
// senzaGergo: i testi del server mostrati in una barra o in un esito, senza i nomi delle rotte e delle versioni del
// progetto («(S8)», «(non in V1)»; messaggio 46, D29 [T]). Solo presentazione: il testo resta del server.
// aParole: un testo del server per chi guarda (collaudo UI46, I4 e I7): gli accenti scritti con l'apostrofo nel codice
// del server («e'», «piu'»), i nomi delle rotte («da S8») e le frasi di metodo («senza dedurre un mono-pezzo») diventano
// parole. Solo presentazione: il testo del server resta quello.
const ACCENTI = { "e'": "è", "E'": "È", "piu'": "più", "gia'": "già", "puo'": "può", "perche'": "perché", "cioe'": "cioè", "pero'": "però", "finche'": "finché", "qualita'": "qualità",
  "quantita'": "quantità", "identita'": "identità", "unicita'": "unicità", "validita'": "validità", "attivita'": "attività" };
function aParole(t) {
  return String(t || "").replace(/\b([A-Za-z]+)'(?=\s|[.,;:)]|$)/g, (m, p) => ACCENTI[p + "'"] || m)
    .replace(/\s*\(confermat[oa] da S8\)/g, "").replace(/\bdopo il commit\b/g, "dopo la conferma").replace(/\bconfermat([oa]) da S8\b/g, "confermat$1 come sorgente")
    .replace(/,\s*senza dedurre un mono-pezzo/g, "").replace(/\s*\(nessun mono-pezzo dedotto\)/g, "").replace(/\s*Nessuna struttura inventata\.?/g, "")
    .replace(/l'assenza di struttura non è una prova/g, "il risultato non è definitivo").replace(/\.$/, "");
}
function senzaGergo(t) {
  return String(t || "").replace(/\s*\((?:S\d{1,2}|non in V\d)\)/g, "").replace(/,\s*non in V\d\b/g, "").replace(/\s*\(non in V\d\.?\)/g, "");
}
// bottoneAz: il pulsante di un'azione della matrice; spento con la ragione accanto (o nel titolo, nella riga)
function bottoneAz(a, id, riga) {
  if (!a) return null;
  const b = btn(riga && a.corta ? a.corta : a.t, a.fai, { cls: "du-btn--sm" + (a.pericolo ? " du-btn--danger" : "") + (riga ? "" : ""), id: id || null, no: a.no, perche: riga ? false : undefined });
  return a.sub && !riga && !a.no ? h("span", { class: "du-gesto" }, b, h("span", { class: "du-perche" }, a.sub)) : b;
}
// RIFIUTO_CANDIDATA: il perché del «Rifiuta la candidata…» spento nel dettaglio dello STEP (al posto di NON_V1.candidata,
// che diceva la versione del progetto in cui arriva)
const RIFIUTO_CANDIDATA = "Il rifiuto di una candidata non è ancora in questa pagina: per un'altra sorgente, conferma un altro STEP";
function dettaglioSg() {
  const aside = h("aside", { class: "du-sg-dettaglio", id: "du-sg-dettaglio", "aria-label": "Dettaglio" });
  aside.append(h("button", { type: "button", class: "du-btn du-btn--sm du-btn--ghost du-sg-indietro", onclick: () => {
    SG.vista = "elenco";
    renderCorpo();
  } }, "‹ Elenco"));
  if (S.anteprima && SG.larga) {
    aside.append(dettaglioAnteprima());
    return aside;
  }
  const mm = memo();
  const k = SG.cartella;
  const righe = righeVisibili(k);
  const spuntate = righe.filter((y) => mm.spunte.has(y.f.allegato_id));
  if (spuntate.length > 1) {
    aside.append(h("h3", { tabindex: "-1" }, "Selezione · " + spuntate.length + " file"), h("ul", { class: "du-elenco-p" }, spuntate.map((y) => h("li", {}, y.f.nome_file))), barraGruppo(spuntate, [], "-d"));
    return aside;
  }
  const y = righe.find((x) => x.f.allegato_id === mm.file) || righe[0];
  if (!y) {
    aside.append(h("h3", { tabindex: "-1" }, cartellaNome(k)), h("p", { class: "du-nota" }, definizioneDi(k)));
    if (S.anteprima) aside.append(btn("‹ Allarga l'anteprima", () => {
      SG.larga = true;
      renderCorpo();
    }, { cls: "du-btn--sm", id: "du-sg-allarga", perche: false }));
    return aside;
  }
  if (!mm.file) mm.file = y.f.allegato_id;
  aside.append(schedaFile(y, k));
  return aside;
}
function schedaFile(y, k) {
  const f = y.f;
  const A = azioniFile(f, { pid: y.pid, cartella: k });
  const box = h("div", { class: "du-sg-file", id: "du-sg-file", "data-allegato": f.allegato_id });
  box.append(h("h3", { tabindex: "-1", class: "du-mono" }, f.nome_file), h("div", { class: "du-fr-meta" }, metaFile(f)));
  const tr = tracciaFile(f);
  if (tr) box.append(h("p", { class: "du-nota", id: "du-sg-traccia" }, tr));
  // le azioni: la principale, le altre, «Non disponibili (n)» (aperto da solo quando è spenta quella che di solito si cerca)
  const az = h("div", { class: "du-sg-azioni", id: "du-sg-azioni" });
  const idDi = (a) => "du-" + a.id + "-" + sid(f.allegato_id);
  // «Apri» è un indirizzo (S5) o il visore per un PDF (MAT FL-03): un link, mai una fetch della pagina
  const el = (a) => (a.id === "apri" && !a.no ? bottoneApri(f, idDi(a)) : bottoneAz(a, idDi(a)));
  if (A.prin) {
    const p0 = el(A.prin);
    (p0.querySelector ? p0.querySelector("button, a") || p0 : p0).classList.add("du-btn--pri");
    az.append(p0);
  }
  for (const a of A.altre) az.append(el(a));
  box.append(az);
  // lo STEP: le candidature, una per prodotto, con i loro due gesti; e l'anteprima per i prodotti che il motore non propone
  if (eStep(f) && eAttivo(f)) {
    // messaggio 46 (D10, D11, D27, D28): per ogni prodotto un riquadro solo. Lo stato della sorgente (confermata, revocata,
    // sostituita) se il prodotto ha su questo STEP la conferma di S8; altrimenti la proposta del motore (G39-02 [U]); poi
    // le candidature dei prodotti che non hanno né l'uno né l'altra; infine, ripiegata, l'anteprima per gli altri prodotti
    const coperti = new Set();
    // la card che la cartella del prodotto mostra già in alto non si ripete qui (C2-I5): una riga che lo dice
    for (const p of prodottiSg()) {
      if (!cardNellaCartella(p.id, f.allegato_id)) continue;
      box.append(h("p", { class: "du-nota", id: "du-sg-card-sopra", "data-prodotto": p.id }, "La proposta di questo STEP come sorgente di " + codiceDi(p.id) + " è nell'intestazione della cartella, a sinistra."));
      coperti.add(p.id);
    }
    for (const x of statiSorgente(f.allegato_id)) {
      if (coperti.has(x.pid)) continue;
      box.append(riquadroStatoSorgente(x, f));
      coperti.add(x.pid);
    }
    for (const p of prodottiSg()) {
      if (coperti.has(p.id) || !stepProposto(p.id, f.allegato_id)) continue;
      box.append(boxStepProposto(p.id, p.id === S.prodotto ? "d" : "d-" + sid(p.id), f.allegato_id));
      coperti.add(p.id);
    }
    const cc = candidateDi(f.allegato_id).filter((c) => !coperti.has(c.prodotto_id));
    const cand = h("div", { class: "du-sg-candidature", id: "du-sg-candidature" }, cc.length || !coperti.size ? h("h4", { class: "du-lab" }, "Candidature") : null);
    if (!cc.length && !coperti.size) cand.append(h("p", { class: "du-muto du-piccolo" }, S.erroreSmist ? "Le candidate non si leggono: ↻ Aggiorna." : "Il Motore A non propone questo STEP per nessun prodotto."));
    for (const c of cc) {
      const st = ogg(c.struttura) ? (c.struttura.completa === false ? "struttura non completa" : "struttura completa") : "";
      const inc = perchéIncerta(c, null, "");
      cand.append(h("div", { class: "du-sg-cand", "data-prodotto": c.prodotto_id }, h("div", {}, h("b", { class: "du-mono" }, codiceDi(c.prodotto_id)), " · radice ", h("span", { class: "du-mono" }, c.radice_letterale || "?"),
        " · " + (c.compatibilita || "?") + (st ? " · " + st : "") + (typeof c.segnalati === "number" ? " · segnalati: " + c.segnalati : "")),
      inc ? h("p", { class: "du-nota", "data-du-incerta": c.allegato_id || "" }, inc) : null,
      h("div", { class: "du-azioni" }, azioniCandidatura(c).map((a) => bottoneAz(a, a.domId + "-d")),
        // il rifiuto di una candidata non è in questa pagina: visibile e spento, con il perché (MAT §6.1), senza le
        // versioni del progetto e senza «motivato» (messaggio 46 [U] punti 1 e 5; rilievo della revisione della corsia)
        btn("Rifiuta la candidata…", null, { cls: "du-btn--sm", no: RIFIUTO_CANDIDATA }))));
    }
    // gli altri prodotti: l'anteprima per ciascuno, ripiegata (D27: niente elenco lungo che inviti a confermare lo STEP di
    // un componente come sorgente di un finito qualsiasi)
    const altri = azioniStepNonCandidato(f, coperti);
    if (altri.length) cand.append(h("details", { class: "du-ripiega", id: "du-sg-altri-prodotti" }, h("summary", {}, "Guardalo per un altro prodotto (" + altri.length + ")"),
      h("p", { class: "du-nota" }, "Il motore non propone questo STEP per questi prodotti: l'anteprima dice che cosa sa della sua struttura."),
      h("div", { class: "du-azioni" }, altri.map((a) => bottoneAz(a, "du-" + a.id + "-" + sid(f.allegato_id))))));
    if (cand.childNodes.length) box.append(cand);
  }
  if (A.spente.length) {
    const cercata = A.spente.some((a) => ["associa", "salva", "togli", "riattiva", "revisione"].includes(a.id.replace(/-.*/, "")));
    box.append(h("details", { class: "du-ripiega du-sg-nondisp", id: "du-sg-nondisp", open: cercata ? true : null }, h("summary", {}, "Non disponibili (" + A.spente.length + ")"),
      h("ul", { class: "du-elenco-p" }, A.spente.map((a) => h("li", { "data-azione": a.id }, h("b", {}, a.t), ": ", a.no)))));
  }
  // destinazioni (S9) e motivo del mancato abbinamento
  const links = collegamentiDi(f);
  if (links.length) box.append(h("div", { class: "du-sez" }, h("h4", { class: "du-lab" }, "Destinazioni"), h("ul", { class: "du-elenco-p", id: "du-sg-destinazioni" }, links.map((x) => {
    const xs = eStep(f) ? statoSorgenteRiga(f, x.pid, x.stato) : null;
    const s = xs ? testoStatoSorgenteRiga(xs) : STATI_RIGA[x.stato] || ["", x.stato];
    const idc = idClienteDa(x.r.identificativo_cliente, Object.prototype.hasOwnProperty.call(x.r, "identificativo_cliente"));
    return h("li", { "data-prodotto": x.pid }, codiceDi(x.pid) + " · " + (s[0] ? s[0] + " " : "") + s[1] + " → ", h("span", { class: "du-mono" }, x.r.codice_mostrato || "(senza codice)"), x.r.rev_mostrata ? " rev " + x.r.rev_mostrata : "",
      idc && !idc.hidden ? [" · ", idc] : null);
  }))));
  const mot = motivoDaAssociare(f.allegato_id);
  if (mot && !links.length) box.append(h("p", { class: "du-nota", id: "du-sg-motivo" }, "Il Motore A non lo abbina: " + mot + "."));
  for (const p of attesaDi(f.allegato_id)) box.append(h("p", { class: "du-nota" }, "Attende la sorgente di " + p.codice + ": il Motore A lo propone in tutte le sue candidate."));
  box.append(h("div", { class: "du-azioni" }, f.stato === "scartato" ? null : h("a", { class: "du-btn du-btn--sm du-btn--ghost", href: P.urlContenuto(f.allegato_id, "scarica"), download: f.nome_file || "", id: "du-scarica-" + sid(f.allegato_id) }, "Scarica")));
  return box;
}
// dettaglioAnteprima: l'anteprima di uno STEP nel dettaglio allargato (T-14): la proposta, l'organigramma o l'elenco, il
// pannello del nodo accanto; «Riduci ⟩» la rimette nel riquadro stretto, «Chiudi» la toglie
function dettaglioAnteprima() {
  return h("div", { class: "du-sg-anteprima" },
    h("div", { class: "du-azioni du-sg-anteprima-t" }, btn("Riduci l'anteprima ›", () => {
      SG.larga = false;
      renderCorpo();
    }, { cls: "du-btn--sm du-btn--ghost", id: "du-sg-riduci", perche: false })),
    anteprimaConPannello());
}
// anteprimaConPannello: il riquadro dell'anteprima (riquadroAnteprima: la proposta, l'organigramma o l'elenco, i gesti) con
// il pannello del nodo della proposta accanto. Lo stesso nella Sorgente (dettaglio allargato) e nella Revisione.
function anteprimaConPannello(id) {
  const g = S.anteprima ? grafoProposta() : null;
  const nodo = g && g.nodi[S.nodo] ? pannelloNodo(g, g.nodi[S.nodo]) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo della proposta per vedere identità proposta, riferimenti e provenienza.");
  return h("div", { class: "du-sg-anteprima-c", id: id || null }, riquadroAnteprima(g), h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, S.nodo && g && g.nodi[S.nodo] ? "Nodo della proposta" : "Pannello del nodo")), h("div", { class: "du-zona-corpo" }, nodo)));
}
// anteprimaInRevisione: l'anteprima di uno STEP chiesta dalla Revisione (il riquadro dello STEP proposto, o della sorgente
// revocata): si mostra lì, con lo stesso riquadro della Sorgente, finché non la si chiude (collaudo UI46, giro 2, C2-B1:
// «Guarda l'anteprima» dalla Revisione senza bozza non mostrava niente, e «Conferma questo STEP» stava accanto) [T]
function anteprimaInRevisione() {
  return !!(S.anteprima && S.passo === "revisione" && S.anteprimaPasso === "revisione");
}
// revisioneAnteprima: il posto dell'anteprima nella Revisione, a tutta larghezza
function revisioneAnteprima() {
  return h("div", { class: "du-rev-anteprima", id: "du-rev-anteprima" }, anteprimaConPannello());
}

// ---- le mosse (R4 del mockup v5): ogni mossa lascia la riga fantasma, il ±1 sui conti, «Ultima mossa» con «Vai al file»
// e, se l'inverso è esatto, «Annulla la mossa»

function contiAlbero() {
  const o = {};
  for (const x of albero()) if (x.k && typeof x.conto === "number") o[x.k] = x.conto;
  return o;
}
function registraMossa(testo, o) {
  o = o || {};
  SG.mossa = { testo, allegato: o.allegato || "", cartella: SG.cartella, passo: o.annullabile ? S.pila[S.pila.length - 1] || null : null, esito: o.esito || "ok", contiPrima: o.contiPrima || null,
    fantasma: o.fantasma || null, prodotto: S.prodotto };
  annuncia(testo);
  renderBarra();
}
// sistema: un messaggio del sistema (letture del Motore A, risposte che non vengono da un gesto, «↻ Aggiorna»), in «Dal
// sistema»; non sposta il fuoco
function sistema(testo, allegato) {
  SG.sistema.unshift({ testo, allegato: allegato || "", quando: new Date().toISOString() });
  SG.sistema = SG.sistema.slice(0, 50);
}
function mossaAnnullabile() {
  const m = SG.mossa;
  return !!(m && m.passo && S.pila[S.pila.length - 1] === m.passo && !perchéNoModifica());
}
function annullaMossa() {
  if (!mossaAnnullabile()) return;
  const m = SG.mossa;
  const prima = contiAlbero();
  annulla();
  SG.mossa = { testo: "Annullata: " + m.testo, allegato: m.allegato, cartella: SG.cartella, passo: null, esito: "ok", contiPrima: prima, fantasma: null };
  if (m.allegato) memo().file = m.allegato;
  renderCorpo();
  renderBarra();
  fuocoSuRiga(m.allegato);
}
// mossaBozza: le mosse della bozza dalla Sorgente (MAT FL-12, EL-02…EL-04; vocabolario 2): «Salva nella bozza» (la
// preassociazione della proposta), «Togli dalla bozza», «Lascia senza associazione». Un salvataggio R2 normale.
function mossaBozza(tipo, f, x) {
  const prima = contiAlbero();
  const k0 = SG.cartella;
  const aid = f.allegato_id;
  let ok = false;
  let testo = "";
  if (tipo === "salva") {
    const k = chiaveRiga(x.r, x.pid);
    const azioni = azioniPreassocia(aid, k);
    if (!azioni.length) return avvisa("no", f.nome_file + " è già preassociato a quel nodo.");
    ok = fai("salvato nella bozza " + f.nome_file, azioni);
    testo = f.nome_file + " → salvato nella bozza di " + codiceDi(x.pid) + " (" + (x.r.codice_mostrato || k) + ")";
  } else if (tipo === "togli") {
    const k = chiaveRiga(x.r, x.pid);
    ok = fai("tolto dalla bozza " + f.nome_file, [{ azione: "file_svincolato", chiave_bozza: k, prima: { allegato_id: aid, chiave_bozza: k }, dopo: { allegato_id: aid, motivo: "utente" }, origine: "utente" }]);
    testo = f.nome_file + " → tolto dalla bozza di " + codiceDi(x.pid);
  } else if (tipo === "lascia") {
    ok = fai("lasciato senza associazione " + f.nome_file, [{ azione: "file_lasciato", chiave_bozza: null, prima: null, dopo: { allegato_id: aid }, origine: "utente" }]);
    testo = f.nome_file + " → senza associazione";
  }
  if (!ok) return;
  const esce = !contenuto(k0).some((y) => y.f.allegato_id === f.allegato_id);
  registraMossa(testo, { allegato: f.allegato_id, annullabile: tipo !== "lascia", contiPrima: prima, fantasma: esce ? { allegato: f.allegato_id, cartella: k0, dove: tipo === "lascia" ? "Senza proposta" : codiceDi(x.pid) } : null });
  if (esce) prossimaRiga(k0, f.allegato_id);
  renderCorpo();
  fuocoSuRiga(memo(k0).file);
}
// mossaGruppo: «Salva nella bozza (n)» e «Togli dalla bozza (n)»: un gesto, una versione (R2), con un azione_id comune
function mossaGruppo(tipo, l) {
  if (!l.length) return;
  const pid = l[0].y.pid;
  if (l.some((x) => x.y.pid !== pid)) return avvisa("no", "Il gesto di gruppo vale per un prodotto alla volta.");
  return conProdotto(pid, () => {
    const prima = contiAlbero();
    const azioni = l.flatMap(({ y }) => {
      const k = chiaveRiga(y.r, y.pid);
      const aid = y.f.allegato_id;
      return tipo === "salva" ? azioniPreassocia(aid, k) :
        [{ azione: "file_svincolato", chiave_bozza: k, prima: { allegato_id: aid, chiave_bozza: k }, dopo: { allegato_id: aid, motivo: "utente" }, origine: "utente" }];
    });
    if (!azioni.length) return avvisa("no", "I file scelti sono già preassociati a quei nodi.");
    const ok = fai((tipo === "salva" ? "salvati nella bozza " : "tolti dalla bozza ") + l.length + " file", azioni);
    if (!ok) return;
    for (const { y } of l) memo().spunte.delete(y.f.allegato_id);
    registraMossa(l.length + " file → " + (tipo === "salva" ? "salvati nella bozza di " : "tolti dalla bozza di ") + codiceDi(pid), { allegato: l[0].y.f.allegato_id, annullabile: true, contiPrima: prima });
    renderCorpo();
  });
}
// «Salva nella bozza le N proposte…» (intestazione del prodotto): un dialogo con l'elenco, poi un gesto solo
function apriSalvaProposte(id, prop) {
  apriFinestra("Salva nella bozza le " + prop.length + " proposte di " + codiceDi(id), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Le proposte del Motore A diventano collegamenti salvati nella bozza di " + codiceDi(id) + ". La BOM non è confermata: lo diventa solo con «Conferma BOM»."),
    h("ul", { class: "du-elenco-p", id: "f-proposte" }, prop.map((y) => h("li", {}, y.f.nome_file + " → " + (y.r.codice_mostrato || chiaveRiga(y.r, id)))))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Salva nella bozza (" + prop.length + ")", pri: true, id: "f-ok", fai: () => {
    mossaGruppo("salva", prop.map((y) => ({ y })));
  } }]);
}
// prossimaRiga: dopo una mossa che toglie il file dall'elenco il fuoco va sulla riga successiva e il dettaglio la segue
// (T-11, regola generale del 5.1)
function prossimaRiga(k, allegato) {
  const righe = righeVisibili(k);
  const i = righe.findIndex((y) => y.f.allegato_id === allegato);
  const dopo = righe.slice(i + 1).find((y) => y.f.allegato_id !== allegato) || righe.slice(0, Math.max(0, i)).reverse().find((y) => y.f.allegato_id !== allegato);
  memo(k).file = dopo ? dopo.f.allegato_id : "";
}
// annullaScollegamento: toglie la decisione scollega_documento della bozza (il collegamento resta registrato)
function annullaScollegamento(f, x) {
  const dec = svincoloDi(f.allegato_id);
  if (!dec) return;
  const prima = contiAlbero();
  const ok = fai("annullato lo scollegamento di " + f.nome_file, [{ azione: "decisione_tolta", chiave_bozza: chiaveEvento(dec), prima: clona(dec), dopo: null, origine: "utente" }]);
  if (ok) registraMossa(f.nome_file + " → resta collegato a " + codiceDi(x.pid), { allegato: f.allegato_id, annullabile: true, contiPrima: prima });
}
// apriAssociaNodo: «Associa a un nodo…» / «Associa a un altro nodo…» / «Sposta a un altro nodo…» (il dialogo della v4.2,
// [U] M32: preselezione del prodotto, nodo con lo stesso codice, riquadro dell'attesa, conflitto, id. cliente, nota senza
// codice). D-V5-03 senza risposta: vale la raccomandata A, base [T]: il testo di ogni voce comincia con il codice e il
// livello sta dopo. Il prodotto scelto deve avere la bozza aperta (il nodo è della sua bozza).
function apriAssociaNodo(f, o) {
  o = o || {};
  const conBozza = prodottiSg().filter((p) => haBozza(p.id));
  const pid = o.pid && haBozza(o.pid) ? o.pid : conBozza.find((p) => p.id === S.prodotto) ? S.prodotto : conBozza.length ? conBozza[0].id : "";
  if (!pid) return avvisa("no", "«Associa a un nodo…» non parte: " + NO_BOZZA_SG + ".");
  if (pid !== S.prodotto) return conProdotto(pid, () => apriAssociaNodo(f, Object.assign({}, o, { pid })));
  const g = grafoBozza();
  const liv = {};
  const giro = (k, n) => {
    if (liv[k] !== undefined) return;
    liv[k] = n;
    for (const a of arr(S.lavoro && S.lavoro.archi)) if (a.padre === k) giro(a.figlio, n + 1);
  };
  const radice = S.lavoro && S.lavoro.nodi.find((n) => n.componente_id === S.prodotto);
  if (radice) giro(radice.chiave_bozza, 0);
  const pezzi = Object.values(g.nodi).filter((v) => !v.noninc && v.nodo && !v.escluso && v.k !== o.da).sort((a, b) => (liv[a.k] || 0) - (liv[b.k] || 0) || String(a.cod || a.desc).localeCompare(String(b.cod || b.desc)));
  const codFile = f.proposta && f.proposta.codice;
  const stesso = codFile ? pezzi.find((v) => v.cod === codFile) : null;
  const attesa = attesaDi(f.allegato_id);
  const selP = h("select", { id: "f-prodotto" }, prodottiSg().map((p) => h("option", { value: p.id, selected: p.id === pid ? true : null, disabled: haBozza(p.id) ? null : true },
    p.codice + (haBozza(p.id) ? "" : " — prima conferma la sorgente di " + p.codice))));
  selP.addEventListener("change", () => {
    const nuovo = selP.value;
    chiudiFinestra();
    conProdotto(nuovo, () => apriAssociaNodo(f, Object.assign({}, o, { pid: nuovo })));
  });
  const sel = h("select", { id: "f-pezzo" }, h("option", { value: "" }, "— scegli il nodo —"), pezzi.map((v) => h("option", { value: v.k, selected: stesso && stesso.k === v.k ? true : null },
    (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "") + " · " + (v.desc || "") + " · liv. " + (liv[v.k] !== undefined ? liv[v.k] : "?"))));
  const idc = h("p", { class: "du-nota", id: "f-idc" });
  const conflitto = h("p", { class: "du-nota", id: "f-conflitto" });
  const aggiornaNodo = () => {
    const v = g.nodi[sel.value];
    idc.replaceChildren(v ? idClienteNodo(v.k) || h("span", { class: "du-muto" }, "id. cliente: lo calcola il riepilogo") : "");
    conflitto.textContent = v && codFile && v.cod && v.cod !== codFile ? "Il codice letto dal file (" + codFile + ") non è quello del nodo (" + v.cod + "): il riepilogo dirà il collegamento «in conflitto»." :
      v && f.proposta && f.proposta.rev && v.rev && f.proposta.rev !== v.rev ? "La rev del file (" + f.proposta.rev + ") non è quella del nodo (" + v.rev + "): il riepilogo dirà il collegamento «in conflitto»." : "";
  };
  sel.addEventListener("change", aggiornaNodo);
  const titolo = o.da ? "Sposta «" + f.nome_file + "» a un altro nodo" : "Associa «" + f.nome_file + "» a un nodo";
  apriFinestra(titolo, h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "È un collegamento salvato nella bozza: la conferma della BOM lo applica. Il codice e la rev del file restano quelli letti."),
    attesa.length && attesa.some((p) => p.id !== pid) ? h("p", { class: "du-nota", id: "f-attesa" }, "Il Motore A lo propone per " + attesa.map((p) => p.codice).join(", ") + ", che non ha ancora una sorgente: associarlo a " + codiceDi(pid) + " è una scelta tua, con la traccia.") : null,
    codFile ? null : h("p", { class: "du-nota", id: "f-senza-codice" }, "Il file non ha un codice letto: scegli il nodo guardando il contenuto (Apri)."),
    // G39-01 [U]: una proposta del motore con una sola destinazione; la provenienza la decide il server. Una PB legacy
    // spostata diventa manuale (conRegistriDelMotore, regola [T-39] di provenienza3.go)
    pbLegacy(f.allegato_id) ? h("p", { class: "du-nota", id: "f-provenienza-legacy" }, "È una pre-assegnazione delle pagine di prima (legacy): spostata, il server la registra «manuale», anche sul nodo che il motore propone.") : null,
    !pbLegacy(f.allegato_id) && registroDelFile(f.allegato_id) ? h("p", { class: "du-nota", id: "f-provenienza-motore" }, "Il Motore A propone questo file" + (o.suggerito ? " su " + voceEffetto(o.suggerito) : "") +
      ": sul nodo proposto il server registra la preassociazione «dal motore», su un altro nodo «corretta dall'ingegnere».") : null,
    campo("Prodotto", selP), campo("Nodo destinatario", sel), idc, conflitto,
    h("p", { class: "du-perche", id: "f-spento" })),
  [{ testo: "Annulla", chiudi: true }, { testo: o.da ? "Sposta" : "Associa", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-pezzo") ? "" : "Spento: scegli il prodotto e il nodo."), fai: () => {
    const k = valore($("#du-finestra"), "f-pezzo");
    const prima = contiAlbero();
    const k0 = SG.cartella;
    // «Sposta» (o.da) e un'associazione di un file già preassociato altrove: svincolo e preassociazione (azioniPreassocia)
    const azioni = azioniPreassocia(f.allegato_id, k);
    if (!azioni.length) return avvisa("no", f.nome_file + " è già preassociato a quel nodo.");
    const v = g.nodi[k];
    const ok = fai((o.da ? "spostato " : "associato ") + f.nome_file + " a " + (v && v.cod ? v.cod : k), azioni);
    if (!ok) return false;
    const esce = !contenuto(k0).some((y) => y.f.allegato_id === f.allegato_id);
    registraMossa(f.nome_file + " → " + codiceDi(pid) + " (" + ((v && v.cod) || k) + ")", { allegato: f.allegato_id, annullabile: !o.ricolloca, contiPrima: prima,
      fantasma: esce ? { allegato: f.allegato_id, cartella: k0, dove: codiceDi(pid) } : null });
    if (esce) prossimaRiga(k0, f.allegato_id);
    renderCorpo();
  } }], { dopo: aggiornaNodo });
}
// apriGruppoFile: «Metti in Altri file… (n)», «Riattiva… (n)», «Scarta… (n)» (D-V5-02 senza risposta: vale la
// raccomandata A, base [T]: un dialogo solo con l'elenco file per file e le dipendenze, un solo «Scollega e …», una sola
// spunta per le uniche copie, l'esito file per file; dal messaggio 46 [U] nessun motivo). I gesti partono uno dopo l'altro, ognuno
// con la sua chiave, il suo corpo e le impronte del suo dettaglio.
async function apriGruppoFile(gesto, files) {
  if (!files.length) return;
  avvisa("ok", "Lettura delle dipendenze di " + files.length + " file…");
  const det = [];
  for (const f of files) {
    const d = await dettaglioFile(f);
    if (!d) return;
    det.push({ f, d });
  }
  const unica = det.reduce((n, x) => n + richiede(x.d, "conferma_unica_copia"), 0);
  const scol = det.reduce((n, x) => n + richiede(x.d, "scollega"), 0);
  const blocchi = det.filter((x) => blocchiFile(x.d).length);
  const nomi = { altri: "Metti in Altri file", riattiva: "Riattiva", scarta: "Scarta" };
  const testi = {
    altri: "I file restano nella RFQ: la copia di ciascuno va nella cartella ALTRO della RFQ sul NAS. Stare lì non vuol dire che è un documento confermato: lo dice il database. Non entrano nella proposta della BOM finché non li riattivi.",
    riattiva: "I file tornano fra i file attivi: il motore li rilegge e può proporli di nuovo. Non conferma nessun documento e non autorizza nessuno STEP.",
    scarta: "I file escono dai file attivi della RFQ e la loro copia gestita dal cockpit viene eliminata. La mail originale, i documenti confermati e la storia non si toccano. Se un file è in Altri file, anche la sua copia nella cartella ALTRO della RFQ sul NAS viene eliminata. Non si annulla: per riaverli dovrai ricaricarli.",
  };
  apriFinestra(nomi[gesto] + " · " + files.length + " file", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, testi[gesto]),
    h("ul", { class: "du-dip", id: "f-gruppo" }, det.map((x) => h("li", { "data-allegato": x.f.allegato_id }, h("b", {}, x.f.nome_file), h("span", {}, arr(x.d.dipendenze).map((d) => (d.codice || "") + (d.testo ? ": " + d.testo : "")).join(" · ") || "nessuna dipendenza")))),
    gesto === "scarta" && unica ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-unica" }), "Ho capito: per " + unica + " file è l'unica copia. Scartandoli non ne resta nessuna: per riaverli dovrò ricaricarli.") : null,
    gesto !== "riattiva" && scol ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-scollega" }), (gesto === "scarta" ? "Scollega e scarta" : "Scollega e metti in Altri file") + " (" + scol + " file associati)") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: nomi[gesto] + " (" + files.length + ")", pri: gesto !== "scarta", danger: gesto === "scarta", id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    const manca = [];
    if (blocchi.length) manca.push("dipendenze che bloccano: " + blocchi.map((x) => x.f.nome_file).join(", "));
    if (gesto === "scarta" && unica && !fin.querySelector("#f-unica").checked) manca.push("la spunta «Ho capito»");
    if (gesto !== "riattiva" && scol && !fin.querySelector("#f-scollega").checked) manca.push("la spunta «Scollega e …»");
    return manca.length ? "Spento: mancano: " + manca.join(", ") : "";
  }, fai: async () => {
    const prima = contiAlbero();
    const k0 = SG.cartella;
    const esiti = [];
    for (const { f, d } of det) {
      const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0 };
      if (gesto !== "riattiva") corpo.impronta_dipendenze = d.impronta_dipendenze || "";
      if (gesto !== "riattiva" && richiede(d, "scollega")) corpo.scollega = true;
      if (gesto === "scarta" && richiede(d, "conferma_unica_copia")) corpo.conferma_unica_copia = true;
      if (eArchivio(f) && gesto !== "scarta") corpo.perimetro = "contenitore";
      else if (d.n_voci !== undefined && gesto === "scarta") {
        corpo.n_voci = d.n_voci;
        corpo.impronta_voci = d.impronta_voci;
      }
      const fn = gesto === "altri" ? P.altriFile : gesto === "riattiva" ? P.riattivaFile : P.scartaFile;
      const r = await inSerie("file:" + f.allegato_id, () => fn(f.allegato_id, corpo));
      esiti.push({ f, r });
      if (r.esito === "ok") memo(k0).spunte.delete(f.allegato_id);
    }
    await rileggiDopoUnGesto(["sorgente"], "I gesti sui file sono registrati.");
    await caricaSmistamento();
    const ok = esiti.filter((x) => x.r.esito === "ok");
    const ko = esiti.filter((x) => x.r.esito !== "ok");
    const dove = { altri: "Altri file", riattiva: "In analisi", scarta: "Scartati" }[gesto];
    registraMossa(ok.length + " file → " + dove + (ko.length ? " · " + ko.length + " non riusciti: " + ko.map((x) => x.f.nome_file + " (" + (TESTI_FILE[x.r.codice] || x.r.testo || x.r.codice || "errore") + ")").join(", ") : ""),
      { allegato: ok.length ? ok[0].f.allegato_id : "", esito: ko.length ? "parziale" : "ok", contiPrima: prima });
    avvisa(ko.length ? "no" : "ok", SG.mossa.testo);
    render();
    return true;
  } }]);
}

// ---- l'anteprima e la conferma della sorgente (S6, S8)

// guardaPer: «Guarda l'anteprima per ‹P›» di una candidatura: il prodotto scelto segue la candidatura (S6 è per prodotto)
async function guardaPer(c) {
  if (c.prodotto_id && c.prodotto_id !== S.prodotto) {
    await scegliProdotto(c.prodotto_id);
    if (S.prodotto !== c.prodotto_id) return;
  }
  SG.larga = true;
  // messaggio 46 [U]: l'anteprima dello STEP compare anche con la candidata «incerta». Il gesto «Guarda l'anteprima per
  // ‹P›» di una candidatura (o della proposta) mette in vista la sua struttura: è la scelta esplicita di chi guarda, e
  // guardare non conferma niente (supera, per l'anteprima, la regola di prima SG-02 «un'incerta non si mette in vista da
  // sola»; la conferma resta il gesto separato di S8). Senza gruppo, guarda sceglie la struttura se è una sola.
  await guarda(c.allegato_id, c.gruppo || "");
}
function chiudiAnteprima() {
  S.anteprima = null;
  S.anteprimaAllegato = "";
  S.anteprimaGruppo = "";
  S.nodo = "";
  render();
}
// apriConfermaSorgente: l'unico gesto di S8 (U2 §2.2; direttiva 39 §2-§3): «Conferma questo STEP come sorgente di ‹P›»,
// «Usa nuovo STEP per ‹P›…», «Conferma di nuovo…», «Riprendi: crea la bozza di ‹P›» e, con opz.partenza «solo_prodotto»,
// la conferma di uno STEP senza struttura importabile (apriSoloProdotto). Il prodotto della candidatura è il prodotto
// scelto (conProdotto). Il gruppo e l'impronta sono quelli che la pagina ha mostrato (S6 o la candidatura di S9); con una
// bozza aperta su un'altra sorgente o senza STEP il dialogo legge prima l'effetto con S6 ?bozza= (caso (d)), dichiara la
// bozza nel corpo (dal messaggio 46 [U] mai il motivo: dove serve lo scrive il server). codice_atteso è il codice del prodotto mostrato; sha256 quello del file di S1.
// opz.spostaDa (G39-03 [U]): lo STEP è legato a quell'altro prodotto e passa a questo (S8 con sposta_da): come sorgente S8
// non usata da una R5 (la revoca «sorgente errata» e l'annullamento della bozza là) o, con opz.riassocia, come documento
// o copia P1 senza S8 (si riassocia con sposta_da, opz.testo è il testo del server). Nessun motivo (messaggio 46 [U]).
async function apriConfermaSorgente(c, opz) {
  const o = opz || {};
  const id = S.prodotto;
  const cod = codiceProdotto();
  const f = fileDi(c.allegato_id);
  const sposta = o.spostaDa ? String(o.spostaDa).toLowerCase() : "";
  if (!f) return avvisa("errore", "Lo STEP non è fra i file della Sorgente: ↻ Aggiorna.");
  // la partenza dal solo prodotto (direttiva 39 §3): lo STEP senza struttura importabile, o con l'analisi in corso o
  // fallita; lo stato è quello che la pagina mostra (struttura_step di S6), con la presa d'atto di chi guarda
  const solo = o.partenza === "solo_prodotto";
  const ss = solo ? o.struttura || {} : null;
  const sorg = sorgenteDelProdotto(id);
  const conBozza = bozzaLavorabile();
  if (S.erroreBozza) return avvisa("errore", "La conferma della sorgente non parte: la bozza del prodotto non si legge (↻ Aggiorna).");
  if (bozzaAperta() && !conBozza) return avvisa("no", "La conferma della sorgente non parte: prima converti la bozza.");
  const sb = conBozza ? sorgenteDellaBozza(id) : null;
  // la bozza già su questo STEP (dopo una revoca): S8 conferma di nuovo, il passo 3 trova la bozza sulla sorgente
  const stessaDellaBozza = !!(sb && sb.allegato_id === c.allegato_id && (solo ? sb.gruppo === "solo_prodotto" : sb.gruppo !== "solo_prodotto"));
  if (solo && sb && !stessaDellaBozza) return avvisa("no", "La partenza dal solo prodotto non si offre su una bozza che ha già uno STEP: usa uno STEP con struttura, o abbandona la bozza.");
  if (conBozza && !stessaDellaBozza) {
    const no = perchéNoModifica();
    if (no) return avvisa("errore", "La conferma della sorgente non parte: " + no + ".");
    if (S.coda.length || S.invio) {
      const ok = await salvaOra();
      if (!ok) return avvisa("errore", "La conferma della sorgente non parte: le modifiche della bozza non sono salvate.");
    }
  }
  // il gruppo e l'impronta da dichiarare: quelli dell'anteprima in vista (S6) se è questa, altrimenti della candidatura
  let gruppo = c.gruppo || "";
  let impronta = c.impronta_contenuto || "";
  let radice = c.radice_letterale || "";
  let effetto = null;
  let conBozzaS6 = null; // S6 ?bozza= letta per l'effetto: la struttura del dialogo la riusa
  if (!solo && S.anteprima && S.anteprimaAllegato === c.allegato_id) {
    const gr = arr(S.anteprima.gruppi).find((x) => x.gruppo === (S.anteprimaGruppo || gruppo));
    if (gr) {
      gruppo = gr.gruppo;
      impronta = gr.impronta_contenuto || impronta;
      radice = (gr.radice && gr.radice.identita_proposta && gr.radice.identita_proposta.letterale) || radice;
    }
  }
  // con una bozza aperta su un'altra sorgente, o senza STEP (riconciliazione, direttiva 39 §3), il dialogo legge prima
  // l'effetto con S6 ?bozza= (caso (d))
  if (conBozza && !stessaDellaBozza && !solo) {
    try {
      const a2 = await P.anteprimaStep(c.allegato_id, id, { gruppo: gruppo || undefined, bozza: S.bozza.bozza_id });
      if (a2.errore) return avvisa("errore", "L'effetto sulla bozza non si calcola: " + a2.errore + ". La conferma non parte.");
      conBozzaS6 = a2;
      const gr2 = arr(a2.gruppi).find((x) => x.gruppo === gruppo) || arr(a2.gruppi)[0];
      if (gr2) {
        gruppo = gr2.gruppo;
        impronta = gr2.impronta_contenuto || impronta;
      }
      effetto = P.effettoS6(a2.effetto_sulla_bozza);
      if (effetto.stessa_sorgente && sorg === c.allegato_id) return avvisa("no", S.confrontoNo ? "È già la sorgente della bozza." : "È la sorgente della bozza: per prendere la proposta di oggi usa «Aggiorna proposta…».");
    } catch (e) {
      return avvisa("errore", "L'effetto sulla bozza non si legge: " + testoErrore(e) + ". La conferma non parte.");
    }
  }
  if (!solo && (!gruppo || !impronta)) {
    // collaudo UI46, giro 3 (F3-B1): uno STEP senza struttura importabile per il prodotto (la radice diversa, o senza il
    // marcatore) non ha gruppo né impronta; il pulsante principale della card apre lo stesso dialogo «solo prodotto»
    // dell'anteprima, con lo stato che dice S6 (struttura_step.partenza = solo_prodotto), invece di fermarsi [T]
    let ss2 = null;
    const vista0 = S.anteprima && S.anteprimaAllegato === c.allegato_id ? S.anteprima : conBozzaS6;
    if (vista0 && ogg(vista0.struttura_step)) ss2 = vista0.struttura_step;
    else {
      try {
        const a4 = await P.anteprimaStep(c.allegato_id, id, {});
        if (a4 && !a4.errore && ogg(a4.struttura_step)) ss2 = a4.struttura_step;
      } catch (e) {
        ss2 = null;
      }
    }
    if (ss2 && ss2.partenza === "solo_prodotto") return apriConfermaSorgente(c, Object.assign({}, o, { partenza: "solo_prodotto", struttura: ss2 }));
    return avvisa("no", "Per " + cod + " il Motore A non dà una struttura da confermare in questo STEP: «Guarda l'anteprima per " + cod + "» dice che cosa legge.");
  }
  // che cosa si conferma (messaggio 46 [U], difetti D09 e D17): la struttura proposta del gruppo che si dichiara, dalla
  // anteprima in vista se è questa, altrimenti letta adesso con S6 (una lettura: guardare non scrive niente). Se la
  // lettura non riesce il dialogo lo dice e la conferma resta possibile: la regola la tiene S8
  let vista = null;
  if (!solo && S.anteprima && S.anteprimaAllegato === c.allegato_id && S.anteprimaGruppo === gruppo && S.anteprima.proposta) vista = S.anteprima;
  else if (!solo && conBozzaS6 && conBozzaS6.proposta && arr(conBozzaS6.gruppi).some((x) => x.gruppo === gruppo)) vista = conBozzaS6;
  else if (!solo) {
    try {
      const a3 = await P.anteprimaStep(c.allegato_id, id, conBozza ? { gruppo, bozza: S.bozza.bozza_id } : { gruppo });
      if (a3 && !a3.errore && a3.proposta) vista = a3;
    } catch (e) {
      vista = null;
    }
  }
  if (vista && !radice) radice = radiceDi(arr(vista.gruppi).find((x) => x.gruppo === gruppo));
  const confronto = vista && vista.confronto ? vista.confronto : radice && cod ? (radice === cod ? "coincide" : "diverso") : "";
  // G39-03 [U] prima di R5 (giro G39-ter): la bozza aperta è nata da un'altra sorgente S8 e la BOM non è mai stata
  // confermata: S8 la sostituisce (annullata con la sua storia, sorgente revocata, bozza nuova dalla proposta) invece di
  // cambiarne la fonte; lo dice S6 (effetto_sulla_bozza.sostituzione), con la presa d'atto se la bozza ha modifiche
  const sostBozza = effetto && ogg(effetto.sostituzione) ? effetto.sostituzione : null;
  const diverso = !solo && !!(radice && cod && radice !== cod);
  // il codice letto dal file (S1 proposta.codice) che non è quello del prodotto: S8 registra il documento con il codice del
  // prodotto (core/rfq/sorgente/s8.go passo 2, con qualunque candidatura dal messaggio 46 [U]), e il dialogo lo dice prima
  // del gesto, perché l'utente veda che cosa conferma [T]. Un documento già confermato non cambia codice. Il confronto è
  // quello del server (confermadoc.stessoCodice: spazi ai lati e maiuscole non contano). Il corpo non lo porta.
  const letto = !f.documento ? codiceLettoDelFile(f) : "";
  const lettoDiverso = !!(letto && cod && letto.toUpperCase() !== String(cod).trim().toUpperCase());
  const manuale = conBozza && !sb;
  const cambio = conBozza && !!sb && !stessaDellaBozza;
  // un'altra sorgente confermata e valida: la conferma nuova la supera per la bozza; per S8 è una presa d'atto
  // (sostituisci {conferma: true}, R39-05), il documento di prima resta e la working cambia solo con «Conferma BOM»
  const sostituisci = !!(sorg && sorg !== c.allegato_id);
  const nas = S.sorgente && S.sorgente.configurazione && ogg(S.sorgente.configurazione.altri_file) && S.sorgente.configurazione.altri_file.disponibile === false;
  const versione = conBozza ? S.bozza.versione : 0;
  const gpEff = vista ? grafoProposta(vista) : null;
  const elenco = (t, x) => (arr(x).length ? h("li", {}, t + ": " + arr(x).length + " — " + arr(x).map((y) => voceEffetto(y, gpEff)).join(", ")) : null);
  const statoTesto = { pronta: "analisi conclusa, nessuna struttura importabile per il prodotto", in_analisi: "analisi ancora in corso", errore: "analisi fallita", non_valutato: "analisi non valutata" };
  const cosa = manuale ? "riconcilia la bozza senza STEP che hai già (la stessa bozza: i pezzi uguali si abbinano, gli incerti restano da decidere)" :
    sostBozza ? "sostituisce la bozza aperta, nata da un altro STEP (la bozza di adesso è annullata e resta nella storia, la sua sorgente è revocata, la bozza nuova nasce dalla proposta)" :
    cambio ? "cambia la sorgente della bozza aperta (una versione nuova della stessa bozza)" : stessaDellaBozza ? "la bozza resta quella che hai, già su questo STEP" :
      solo ? "crea la bozza con il solo prodotto, senza struttura dallo STEP" : "crea la bozza dalla proposta del Motore A che hai visto";
  const titolo = sposta ? (o.riassocia ? "Riassocia " + f.nome_file + " a " + cod : "Sposta la sorgente su " + cod) : cambio ? "Usa nuovo STEP per " + cod :
    solo ? "Conferma " + f.nome_file + " come sorgente di " + cod + " · solo prodotto" : "Conferma " + f.nome_file + " come sorgente di " + cod;
  const codA = sposta ? codiceDi(sposta) : "";
  apriFinestra(titolo, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il server registra la conferma di " + f.nome_file + " come sorgente di " + cod + " e " + cosa + ". Non è la conferma della BOM né del fascicolo: la BOM confermata cambia solo con «Conferma BOM». Lo STEP confermato va anche nella cartella della RFQ sul NAS" +
      (nas ? " quando la scrittura sul NAS sarà accesa" : "") + "."),
    sposta ? h("div", { class: "du-esito du-esito--ver", id: "f-sposta", "data-sposta-da": sposta }, h("b", {}, (o.riassocia ? "Riassocia lo STEP da " : "Sposta la sorgente da ") + codA + " a " + cod + "."),
      h("span", {}, o.riassocia ? " " + (o.testo || "Lo STEP è un documento di " + codA + ", non la sua sorgente confermata: lascerà " + codA + ".") :
        " Il server lo fa in un solo gesto: revoca la sorgente di " + codA + " come «sorgente errata»; annulla la bozza di " + codA + " nata da questo STEP (abbandonata, la sua storia resta); " +
        "conferma lo STEP come sorgente di " + cod + ". Se " + codA + " ha già confermato la BOM con questo STEP, lo spostamento non si fa: si apre la sua revisione. Uno STEP è la sorgente di un solo prodotto.")) : null,
    solo ? h("div", { class: "du-esito du-esito--ver", id: "f-solo-prodotto" }, h("b", {}, "Stato dello STEP: " + (statoTesto[ss.stato_analisi] || ss.stato_analisi || "?") + "."),
      h("span", {}, " " + aParole(ss.testo) + (ss.testo ? ". " : "") + "La bozza parte dal solo prodotto: i pezzi li aggiungi tu."),
      radiceLettaDiversa(c.allegato_id, cod) ? h("span", { id: "f-radice-letta" }, " La radice dello STEP è ", h("span", { class: "du-mono" }, radiceLettaDiversa(c.allegato_id, cod)), ", non " + cod + ".") : null,
      h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-presa" }), "Ho preso atto dello stato dell'analisi: procedo dal solo prodotto.")) :
      h("div", { class: "du-riass-box", id: "f-cosa" }, h("p", { class: "du-nota", id: "f-codici" }, righeCodici(cod, radice, confronto, stepProposto(id, c.allegato_id))),
        riassuntoStruttura(vista ? grafoProposta(vista) : null) || h("p", { class: "du-nota", id: "f-struttura-non-letta" }, "La struttura proposta non si legge adesso: la vedi con «Guarda l'anteprima per " + cod + "».")),
    sostituisci ? h("p", { class: "du-nota", id: "f-sostituisci" }, "La sorgente confermata di adesso (" + nomeFile(sorg) + ") resta un documento confermato: questo STEP la supera come sorgente della bozza. La BOM confermata non cambia finché non confermi la BOM nuova.") : null,
    // G39-03 [U]: dopo la conferma della BOM il cambio di sorgente per uno STEP sbagliato è una revisione interna, distinta
    // da una revisione del cliente: la spunta (mai spuntata) lo dichiara, correzione_sorgente nel corpo di S8 [T-39]
    sostBozza ? h("div", { class: "du-esito du-esito--ver", id: "f-sostituzione-bozza" }, h("b", {}, "La bozza aperta viene sostituita."), h("span", {}, " " + (sostBozza.testo || "")),
      sostBozza.modifiche_utente ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-presa-sostituzione" }),
        "Ho preso atto: le modifiche della bozza di adesso restano nella sua storia e non passano alla bozza nuova.") : null) : null,
    sostituisci && !sposta && !sostBozza ? h("label", { class: "du-spunta", id: "f-correzione-riga" }, h("input", { type: "checkbox", id: "f-correzione" }),
      "La sorgente di adesso era sbagliata: è una correzione della sorgente (una revisione interna, non una revisione del cliente, anche se cambiano dei codici).") : null,
    effetto && !sostBozza ? h("div", {}, h("p", {}, "L'effetto sulla bozza (versione " + versione + "), calcolato dal server adesso" + (manuale ? " (l'anteprima non mostra ancora gli abbinamenti incerti: li vedrai nella Revisione)" : "") + ":"),
      h("ul", { class: "du-elenco-p", id: "f-effetto" }, elenco("nodi che restano", effetto.nodi_che_restano), elenco("nodi che escono", effetto.fuori_per_cambio_fonte), elenco("nodi che entrano", effetto.nodi_che_entrano),
        elenco("caselle riscritte su un'altra copia", effetto.caselle_riscritte), elenco("archi riscritti", effetto.archi_riscritti), elenco("identità aggiornate", effetto.identita_aggiornate),
        elenco("da rivedere", effetto.da_rivedere), elenco("decisioni che cadono", effetto.decisioni_cadute), elenco("collegamenti da ricollocare", effetto.preassociazioni_nel_vassoio),
        elenco("abbinamenti da decidere", effetto.abbinamenti_proposti))) : null,
    diverso ? h("p", { class: "du-nota", id: "f-radice-diversa" }, "Il codice letto dalla radice dello STEP (", h("span", { class: "du-mono" }, radice), ") non è quello del prodotto (", h("span", { class: "du-mono" }, cod), "): confermando, vale il codice del prodotto.") : null,
    lettoDiverso ? h("p", { class: "du-nota", id: "f-codice-letto", "data-letto": letto }, "Il file dice ", h("span", { class: "du-mono" }, letto), ", non ", h("span", { class: "du-mono" }, cod),
      ": confermando, il documento prende il codice del prodotto (", h("span", { class: "du-mono" }, cod), ").") : null,
    // G39-02 [U]: la partenza «solo prodotto» da un altro STEP mentre il motore ne propone uno come sorgente
    solo ? avvisoStepProposto(id, c.allegato_id) : null,
    // l'attesa si vede nel dialogo, non solo nella striscia dietro il velo (messaggio 46, D26)
    h("p", { class: "du-esito du-esito--info", id: "f-in-corso", role: "status", hidden: true })),
  [{ testo: "Annulla", chiudi: true }, { testo: cambio ? "Usa questo STEP" : solo ? "Conferma e parti dal solo prodotto" : "Conferma la sorgente", pri: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    const manca = [];
    if (conBozza && S.bozza && S.bozza.versione !== versione) return "la bozza è cambiata: riapri il dialogo";
    if (solo && !(fin.querySelector("#f-presa") || {}).checked) manca.push("la presa d'atto dello stato dell'analisi");
    if (sostBozza && sostBozza.modifiche_utente && !(fin.querySelector("#f-presa-sostituzione") || {}).checked) manca.push("la presa d'atto della bozza sostituita");
    if (mancaPresaProposto(fin)) manca.push(mancaPresaProposto(fin));
    return manca.length ? "mancano: " + manca.join(", ") : "";
  }, fai: async () => {
    const corpo = { prodotto_id: id, codice_atteso: cod || undefined };
    if (f.sha256) corpo.sha256 = f.sha256;
    if (solo) Object.assign(corpo, { partenza: "solo_prodotto", stato_analisi: ss.stato_analisi, presa_d_atto: true });
    else Object.assign(corpo, { gruppo, impronta_contenuto: impronta });
    if (!corpo.codice_atteso) delete corpo.codice_atteso;
    if (sostituisci || sostBozza) corpo.sostituisci = { conferma: true };
    if (sposta) corpo.sposta_da = sposta;
    if (sostituisci && !sposta && !sostBozza && ($("#du-finestra").querySelector("#f-correzione") || {}).checked) corpo.correzione_sorgente = true;
    // con una bozza aperta il corpo la dichiara (anche la bozza senza STEP: senza, 409 BOZZA_VERSIONE con il conflitto)
    if (conBozza) corpo.bozza = { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione };
    SG.inCorsoS8[id] = true;
    avvisa("ok", "Conferma della sorgente in corso: conferma del documento, bozza…", "prodotto");
    const attesa = $("#f-in-corso");
    if (attesa) {
      attesa.textContent = "⟳ Conferma in corso: registro la sorgente e preparo la bozza di " + cod + "…";
      attesa.hidden = false;
    }
    renderCorpo();
    let r;
    try {
      r = await inSerie("sorgente:" + id, () => P.confermaSorgente(c.allegato_id, corpo));
    } catch (e) {
      if (attesa) attesa.hidden = true;
      throw e;
    } finally {
      delete SG.inCorsoS8[id];
    }
    if (attesa && r && r.esito === "ok") attesa.textContent = "⟳ Sorgente registrata: rileggo la bozza e i file…";
    const fatto = await esitoS8(r, id, c);
    if (!fatto && attesa) attesa.hidden = true;
    return fatto;
  } }]);
}
// codiceLettoDelFile: il codice letto dal file (S1 proposta.codice), "" se non c'è; solo da mostrare (il dialogo di S8)
function codiceLettoDelFile(f) {
  return f && ogg(f.proposta) && typeof f.proposta.codice === "string" ? f.proposta.codice.trim() : "";
}
// apriSoloProdotto: «Conferma come sorgente e parti dal solo prodotto…» dall'anteprima di uno STEP senza struttura per il
// prodotto, o con l'analisi in corso o fallita (direttiva 39 §3; struttura_step di S6 con partenza solo_prodotto)
function apriSoloProdotto(a) {
  const ss = ogg(a && a.struttura_step) ? a.struttura_step : null;
  if (!ss || ss.partenza !== "solo_prodotto") return avvisa("no", "Il server non offre la partenza dal solo prodotto per questo STEP: guarda di nuovo l'anteprima.");
  return apriConfermaSorgente({ allegato_id: a.allegato_id, prodotto_id: S.prodotto }, { partenza: "solo_prodotto", struttura: ss });
}
// esitoS8: l'esito di S8 (U2 §2.2): «completata» o «parziale» (Riuscito · Non riuscito · Manca), con le riletture
// (modello, S1, R1, R7, S9); la vista passa alla cartella del prodotto (T-15) e resta nella Sorgente (U2 §2.6). Torna true
// quando il dialogo va chiuso.
async function esitoS8(r, id, c) {
  if (r.esito === "ok") {
    const d = r.dati || {};
    const ko = arr(d.passi).find((p) => p.esito === "rifiutato");
    SG.esitiS8[id] = { esito: d.esito || "completata", passi: arr(d.passi), testo: d.testo || "", bozza409: !!(ko && ko.codice === "ANTEPRIMA_SUPERATA"), ritentabile: !!d.ritentabile };
    S.anteprima = null;
    S.anteprimaAllegato = "";
    S.anteprimaGruppo = "";
    S.nodo = "";
    const riletta = await rileggiDopoUnGesto(["sorgente", "bozza", "storico"], "La conferma della sorgente è registrata.");
    await caricaSmistamento();
    // la riconciliazione della bozza senza STEP (direttiva 39 §3): gli abbinamenti incerti stanno nella Revisione
    const nAbb = id === S.prodotto && bozzaLavorabile() ? abbinamentiProposti().length : 0;
    // e le differenze di quantità, legami e tipi con lo STEP (direttiva 39 §3: si mostrano, «recepisci/mantieni»)
    const nDiff = id === S.prodotto && bozzaLavorabile() ? differenzeRiconciliazione().length : 0;
    const testo = (d.esito === "completata" ? senzaGergo(d.testo) || "Sorgente confermata: la bozza di " + codiceDi(id) + " è pronta da rivedere. La BOM non è confermata." :
      (senzaGergo(d.testo) || "Conferma della sorgente parziale.") + (d.ritentabile ? " «Riprova» dalla striscia riprende con la stessa chiave." : ko && ko.codice === "ANTEPRIMA_SUPERATA" ? " Rivedi l'anteprima e conferma di nuovo." : "")) +
      (nAbb ? " Abbinamenti da decidere nella Revisione: " + nAbb + "." : "") + (nDiff ? " Differenze con lo STEP da vedere nella Revisione: " + nDiff + "." : "") +
      // G39-03: che cosa lo spostamento ha fatto sull'altro prodotto (la revoca, la bozza annullata), detto dal server
      (ogg(d.spostamento) && d.spostamento.testo ? " " + senzaGergo(d.spostamento.testo) : "") +
      // G39-03 prima di R5 (giro G39-ter): la bozza precedente sostituita (annullata, sorgente revocata), detto dal server
      (ogg(d.sostituzione) && d.sostituzione.testo ? " " + senzaGergo(d.sostituzione.testo) : "");
    registraMossa(testo, { allegato: c.allegato_id, esito: d.esito === "completata" ? "ok" : "parziale" });
    SG.apertiProdotto["p:" + id] = undefined;
    // dopo il gesto riuscito le spunte non valgono più (messaggio 46, D20)
    togliTutteLeSpunte();
    await vaiCartella("p:" + id, { salto: true, vista: "elenco" });
    // l'esito riuscito sta nell'intestazione del prodotto e in «Ultima mossa»: niente terza copia nella striscia (D23);
    // la striscia resta per un esito parziale, per la pagina che non si è riletta, per quello che resta da fare nella
    // Revisione (abbinamenti, differenze) e per gli effetti su un altro prodotto o su una bozza di prima (spostamento,
    // sostituzione: detti dal server)
    if (d.esito === "completata" && riletta && !nAbb && !nDiff && !(ogg(d.spostamento) && d.spostamento.testo) && !(ogg(d.sostituzione) && d.sostituzione.testo)) avvisa("ok", "");
    else avvisa(d.esito === "completata" ? "ok" : "no", testo + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."), "prodotto");
    render();
    const t = $("#du-sg-esito-s8");
    if (t) t.setAttribute("tabindex", "-1"), t.focus();
    return true;
  }
  if (r.esito === "da_verificare") {
    SG.esitiS8[id] = { esito: "da_verificare", passi: [], testo: "Esito da verificare: verifico con la stessa chiave." };
    avvisa("da_verificare", "Esito della conferma della sorgente da verificare: verifico con la stessa chiave.");
    verificaConRiprova(r.chiave, async (j) => {
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        await esitoS8({ esito: "ok", dati: Object.assign({ esito: "completata" }, j.esito || {}) }, id, c);
        return true;
      }
      if (j.stato === "non_registrato") {
        SG.esitiS8[id] = { esito: "non_registrato", passi: [], testo: "La conferma della sorgente non è ancora registrata: «Riprova» dalla striscia la rimanda con la stessa chiave." };
        avvisa("no", SG.esitiS8[id].testo);
        renderCorpo();
        return true;
      }
      return false;
    });
    renderStrisce();
    renderCorpo();
    return true;
  }
  if (r.codice === "ANTEPRIMA_SUPERATA") {
    // anteprima: «proposta_step» (gruppo o impronta), «prodotto» (il codice del prodotto è cambiato), «stato_analisi» (lo
    // stato dello STEP mostrato non è più quello, o una struttura importabile c'è): si rilegge e si rivede
    const quale = (r.dati && r.dati.anteprima) || "";
    chiudiFinestra();
    await caricaModello();
    await caricaSmistamento();
    if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, "", true);
    render();
    const t = quale === "prodotto" ? "Il codice del prodotto è cambiato dopo l'anteprima: la pagina si è riletta, conferma di nuovo (ANTEPRIMA_SUPERATA)." :
      quale === "stato_analisi" ? "Lo stato dell'analisi dello STEP è cambiato mentre guardavi: rivedi l'anteprima, poi conferma di nuovo (ANTEPRIMA_SUPERATA)." :
        "La proposta è cambiata mentre guardavi: rivedi l'anteprima, poi conferma di nuovo (ANTEPRIMA_SUPERATA).";
    avvisa("conflitto", t);
    return true;
  }
  if (r.codice === "RICONCILIAZIONE_NON_DISPONIBILE") {
    const mot = { bozza_da_convertire: "la bozza aperta è nel formato di prima: prima convertila", solo_prodotto_su_bozza: "la bozza aperta ha già uno STEP con struttura: la partenza dal solo prodotto non si applica",
      solo_prodotto_su_conferma: "il prodotto ha una BOM confermata: partire dal solo prodotto con uno STEP senza struttura non è ancora disponibile",
      bozza_manuale: "la riconciliazione della bozza senza STEP non è disponibile su questo server" };
    const t = "La conferma non parte: " + (mot[r.motivo] || r.testo || r.motivo) + " (RICONCILIAZIONE_NON_DISPONIBILE).";
    const err = $("#du-finestra-errore");
    if (err) err.textContent = t;
    avvisa("conflitto", t);
    renderCorpo();
    return false;
  }
  if (r.codice === "BOZZA_VERSIONE") {
    chiudiFinestra();
    await rileggiTenendoDaParte();
    await caricaSmistamento();
    render();
    avvisa("conflitto", "La bozza è cambiata altrove: la rileggo; poi ripeti il gesto.");
    return true;
  }
  // G39-03 [U]: lo STEP è legato a un altro prodotto (altro_prodotto del rifiuto, core/rfq/sorgente/sposta.go); il testo è
  // del server, la pagina aggiunge il rimedio: sorgente_spostabile → «Sposta la sorgente su ‹P›…»; riassociazione (un
  // documento o una copia P1 dell'altro, senza S8) → «Riassocia a ‹P›…» (sposta_da; messaggio 46 [U]: senza motivo); bom_confermata → «Apri la revisione di
  // ‹altro›» (dopo R5 niente spostamento diretto: il cambio di sorgente dell'altro è una sua revisione interna)
  const altroP = r.dati && typeof r.dati.altro_prodotto === "string" ? r.dati.altro_prodotto.toLowerCase() : "";
  if (altroP && ["sorgente_spostabile", "bom_confermata", "riassociazione"].includes(r.motivo)) {
    const codA = codiceDi(altroP);
    const t = r.testo || (r.motivo === "bom_confermata" ? "Questo STEP è nella BOM confermata di " + codA + ": non si sposta." : "Questo STEP è legato a " + codA + ".");
    const rimedio = r.motivo === "bom_confermata" ?
      btn("Apri la revisione di " + codA, () => {
        chiudiFinestra();
        conProdotto(altroP, () => vaiPasso("revisione"));
      }, { cls: "du-btn--sm du-btn--pri", id: "f-rimedio-revisione", perche: false }) :
      btn(r.motivo === "riassociazione" ? "Riassocia a " + codiceDi(id) + "…" : "Sposta la sorgente su " + codiceDi(id) + "…", () => {
        chiudiFinestra();
        conProdotto(id, () => apriConfermaSorgente(c, { spostaDa: altroP, riassocia: r.motivo === "riassociazione", testo: r.testo }));
      }, { cls: "du-btn--sm du-btn--pri", id: "f-rimedio-sposta", perche: false });
    const err = $("#du-finestra-errore");
    if (err) {
      err.textContent = t + " (" + r.codice + ", " + r.motivo + ") ";
      err.append(rimedio);
    }
    avvisa("no", t);
    renderCorpo();
    return false;
  }
  // G39-03 dopo R5 (core/rfq/sorgente/s8.go, sorgente.MotivoSostituitoInAltroProdotto): lo STEP è stato sostituito nella
  // storia di un altro prodotto finito; resta lì e non diventa la sorgente di questo. Nessun gesto di rimedio: il server non
  // ne ha uno che funzioni (G39-05, domanda aperta all'utente), e la pagina non ne inventa
  if (r.codice === "GIA_DOCUMENTO" && r.motivo === "sostituito_in_altro_prodotto") {
    const codA = altroP ? codiceDi(altroP) : "un altro prodotto";
    const t = r.testo || "Questo STEP è stato sostituito nella storia di " + codA + ": resta lì e non diventa la sorgente di " + codiceDi(id) + ".";
    const err = $("#du-finestra-errore");
    if (err) err.textContent = t + " (" + r.codice + ", " + r.motivo + ")";
    avvisa("no", t);
    renderCorpo();
    return false;
  }
  const testi = { STEP_SORGENTE_DI_ALTRO_PRODOTTO: "Questo STEP è già la sorgente di un altro prodotto: uno STEP è la sorgente di un solo prodotto.", MOTIVO_OBBLIGATORIO: TESTO_MOTIVO_VECCHIO,
    FILE_NON_ATTIVO: "Lo STEP non è fra i file attivi della RFQ: riattivalo prima.", NON_STEP: "Il file non è uno STEP.", FONTE_NON_CONFERMATA: "La sorgente non risulta confermata: ↻ Aggiorna.",
    GIA_DOCUMENTO: "Lo stesso STEP è già un documento del Fascicolo (di un altro componente, senza componente o sostituito): non diventa la sorgente di questo prodotto.",
    CALCOLO_OCCUPATO: "Il motore è occupato: niente è stato scritto, riprova fra poco.", CALCOLO_SCADUTO: "Il motore non ha risposto in tempo: niente è stato scritto, riprova.", RFQ_UNITA: TESTI_FILE.RFQ_UNITA };
  // i motivi di CORPO_NON_VALIDO di S8 (core/rfq/sorgente): detti con le parole della pagina
  const motivi = { presa_d_atto_mancante: "serve la presa d'atto dello stato dell'analisi", gia_associata_altrove: "lo STEP è associato come documento: lo STEP sorgente del finito non è un documento associato",
    sha_diverso: "il file è cambiato dopo la lettura: ↻ Aggiorna", prodotto_non_ammesso: "il prodotto non è un finito attivo", gruppo_non_importabile: "questo gruppo non si importa: per uno STEP senza struttura si parte dal solo prodotto" };
  const t = (r.codice === "CORPO_NON_VALIDO" && motivi[r.motivo] ? "La conferma non parte: " + motivi[r.motivo] + "." : "") || testi[r.codice] || r.testo || "La conferma della sorgente non è riuscita.";
  const err = $("#du-finestra-errore");
  if (err) err.textContent = t + (r.codice ? " (" + r.codice + ")" : "");
  avvisa(r.esito === "no" ? "no" : "errore", t);
  renderCorpo();
  return false;
}
// riprovaS8: «Riprova la conferma della sorgente» dalla striscia (un «parziale» ritentabile o un esito verificato «non
// registrato»): il ponte rimanda lo stesso corpo con la stessa chiave
async function riprovaS8(s) {
  const id = String(s.risorsa || "").slice("sorgente:".length);
  const a = s.u || s.allegato || (ogg(s.corpo) && s.corpo.allegato_id) || "";
  avvisa("ok", "Conferma della sorgente: la rimando con lo stesso corpo e la stessa chiave…");
  SG.inCorsoS8[id] = true;
  let r;
  try {
    r = await inSerie(s.risorsa, () => P.riprovaSospeso(s.risorsa));
  } finally {
    delete SG.inCorsoS8[id];
  }
  await esitoS8(r, id, { allegato_id: a });
  renderStrisce();
}

// ---- l'anteprima di uno STEP (S6)

async function guarda(allegato, gruppo, silenzioso) {
  if (!S.prodotto) return;
  // F4-I1: l'anteprima di questo STEP era già aperta nella Revisione (un'altra struttura scelta dentro il riquadro): la
  // pagina non si sposta
  const giaInRevisione = anteprimaInRevisione() && S.anteprimaAllegato === allegato;
  S.anteprimaAllegato = allegato;
  S.anteprimaGruppo = gruppo || "";
  // il passo da cui l'anteprima è stata chiesta: dalla Revisione si mostra lì (collaudo UI46, giro 2, C2-B1); le letture
  // silenziose (dopo un gesto) non lo cambiano
  if (!silenzioso) S.anteprimaPasso = S.passo;
  S.erroreAnteprima = "";
  if (!silenzioso) avvisa("ok", "Lettura della proposta dello STEP…", "anteprima");
  try {
    const opz = { gruppo: gruppo || undefined };
    if (bozzaLavorabile()) opz.bozza = S.bozza.bozza_id;
    let a = await P.anteprimaStep(allegato, S.prodotto, opz);
    if (!gruppo) {
      const inVista = arr(a.gruppi).find((x) => x.in_vista);
      // messaggio 46 [U]: una sola struttura candidata (anche «incerta», che il server non mette in vista da solo) si
      // mostra subito: S6 di nuovo con quel gruppo, una lettura
      const unica = !inVista && !a.errore && arr(a.gruppi).length === 1 ? arr(a.gruppi)[0].gruppo : "";
      if (unica) {
        a = await P.anteprimaStep(allegato, S.prodotto, Object.assign({}, opz, { gruppo: unica }));
        S.anteprimaGruppo = unica;
      } else S.anteprimaGruppo = inVista ? inVista.gruppo : "";
    }
    S.anteprima = a;
    if (!silenzioso) avvisa(a.errore ? "errore" : "ok", a.errore ? "Il calcolo della proposta non è riuscito: " + a.errore : a.proposta ? "Proposta dello STEP in vista, non confermata." :
      "Anteprima di " + (a.nome_file || nomeFile(allegato)) + ": il motore non ha una struttura da proporre per " + codiceProdotto() + ".", "anteprima");
  } catch (e) {
    S.anteprima = null;
    S.erroreAnteprima = testoErrore(e);
    avvisa("errore", "La proposta dello STEP non si legge: " + S.erroreAnteprima);
  }
  S.nodo = "";
  render();
  // collaudo finale, giro 4 (F4-I1) [T]: dalla Revisione l'anteprima nasce sotto la card dello STEP, sotto la piega. Il clic
  // l'ha chiesta: la si porta in cima alla finestra e la si adatta alla parte che ora si vede (all'apertura altoVisibile
  // misurava meno di 200 px e lasciava lo zoom al 60 %). Solo dopo il gesto che la apre, mai dopo una lettura silenziosa né
  // quando era già aperta qui
  if (!silenzioso && !giaInRevisione && anteprimaInRevisione()) {
    const ra = $("#du-rev-anteprima");
    if (ra) {
      ra.scrollIntoView({ block: "start", behavior: "instant" });
      if (L && L.vp && ra.contains(L.vp)) setCam(vistaIniziale());
    }
  }
  // UI46 (controllo di stabilità del 21/10 [T]): l'anteprima allargata nasconde l'elenco, e il fuoco che era sulla riga
  // dello STEP finiva sul body; va sul «Chiudi» dell'anteprima (o sul «Riduci»), senza far scorrere niente
  if (!silenzioso) {
    const a = document.activeElement;
    if (!a || a === document.body || !a.isConnected || !a.offsetParent || getComputedStyle(a).visibility === "hidden") {
      const dove = $("#du-anteprima-chiudi") || $("#du-sg-riduci");
      if (dove) dove.focus({ preventScroll: true });
    }
  }
}
function riquadroAnteprima(g) {
  const a = S.anteprima;
  const box = h("div", { class: "du-riq du-riq--cand", id: "du-anteprima" });
  box.append(h("div", { class: "du-riq-testa" }, h("h3", {}, "Anteprima di " + (a.nome_file || nomeFile(a.allegato_id)) + " per " + codiceProdotto()), pill("prop", "in vista, non confermata"),
    btn("Chiudi", chiudiAnteprima, { cls: "du-btn--sm du-btn--ghost", id: "du-anteprima-chiudi", perche: false })));
  if (a.errore) {
    box.append(h("div", { class: "du-striscia du-striscia--ko" }, "Il calcolo della proposta non è riuscito: " + a.errore));
    return box;
  }
  // che cosa il motore sa della struttura di questo STEP per il prodotto (direttiva 39 §3; struttura_step di S6): con
  // figli, la sola radice (il vero gruppo, senza dedurre un mono-pezzo), nessuna struttura importabile, oppure non
  // determinabile perché l'analisi è in corso, fallita o non valutata (mai presentata come prova geometrica)
  const ss = ogg(a.struttura_step) ? a.struttura_step : null;
  const fascia = ss ? fasciaStruttura(ss) : null;
  if (a.stato !== "pronta") {
    // F3-I4 (collaudo UI46, giro 3) [T]: lo STEP analizzato la cui radice non è il prodotto non è «non determinabile»: è
    // di un altro codice, e la striscia dice quale
    const rd = a.stato === "struttura_non_determinabile" && ss && ss.stato_analisi === "pronta" ? radiceLettaDiversa(a.allegato_id, a.codice_prodotto || codiceProdotto()) : "";
    const testi = { in_analisi: "Lo STEP è ancora in analisi.", non_scaricato: "Lo STEP non è scaricato.", errore: "L'analisi dello STEP è in errore.",
      struttura_non_determinabile: "La struttura dello STEP non è determinabile" + (a.struttura && a.struttura.motivo ? ": " + aParole(a.struttura.motivo) : "") + "." };
    box.append(h("div", { class: "du-striscia du-striscia--ver", id: "du-anteprima-stato", "data-du-radice-letta": rd || null }, rd ?
      h("span", {}, "La radice dello STEP è ", h("span", { class: "du-mono" }, rd), ", non ", h("span", { class: "du-mono" }, a.codice_prodotto || codiceProdotto()),
        ": per " + (a.codice_prodotto || codiceProdotto()) + " il Motore A non ha una struttura da importare da questo STEP.") :
      testi[a.stato] || a.testo || "Proposta non disponibile (" + a.stato + ")."));
    if (fascia) box.append(fascia);
    box.append(azioniAnteprima(a, null));
    return box;
  }
  if (fascia) box.append(fascia);
  const gruppi = arr(a.gruppi);
  const scelto = gruppi.find((x) => x.gruppo === S.anteprimaGruppo) || null;
  // la scelta della struttura: con più candidate, e con una sola che non è in vista (messaggio 46 [U]: guarda la mette in
  // vista da sola; qui resta il caso di una lettura che non l'ha portata)
  if (gruppi.length > 1 || (gruppi.length === 1 && !scelto)) {
    box.append(h("div", { class: "du-cartelle", role: "group", "aria-label": "Strutture dello STEP", id: "du-strutture" }, gruppi.map((x) => h("button", { type: "button", "aria-pressed": x.gruppo === S.anteprimaGruppo ? "true" : "false",
      id: "du-struttura-" + sid(x.gruppo), title: "compatibilità con il prodotto: " + (x.compatibilita || "?"), onclick: () => guarda(a.allegato_id, x.gruppo) },
    (x.radice && x.radice.identita_proposta && x.radice.identita_proposta.letterale) || "struttura " + (gruppi.indexOf(x) + 1), " · ", x.compatibilita || ""))));
    if (!scelto) box.append(h("p", { class: "du-nota", id: "du-nessuna-in-vista" }, gruppi.length > 1 ? "Lo STEP ha " + gruppi.length + " strutture candidate per " + codiceProdotto() + ": scegline una qui sopra per guardarla." : "Scegli la struttura qui sopra per guardarla."));
  }
  if (!gruppi.length) {
    box.append(h("div", { class: "du-striscia du-striscia--ver", id: "du-anteprima-nessuna" }, "Nessuna candidata per " + codiceProdotto() + " in questo STEP" +
      (a.testo ? ": " + a.testo : a.struttura && a.struttura.motivo ? ": " + a.struttura.motivo : "") + ". Se lo STEP del prodotto è un altro file, caricalo con «+ Aggiungi file»."));
    box.append(azioniAnteprima(a, null));
    return box;
  }
  const gr = gruppi.find((x) => x.gruppo === S.anteprimaGruppo) || null;
  if (gr) box.append(h("p", { class: "du-nota", id: "du-anteprima-codici" }, righeCodici(a.codice_prodotto || codiceProdotto(), radiceDi(gr), a.confronto, stepProposto(S.prodotto, a.allegato_id))));
  box.append(azioniAnteprima(a, gr));
  if (S.vista === "org" && g && g.radice) box.append(strumentiOrg(g), organigramma(g));
  else if (g && g.radice) box.append(elencoRighe(g));
  else box.append(h("p", { class: "du-muto du-piccolo" }, "Scegli una struttura dello STEP per vederla."));
  return box;
}
// radiceDi: il codice letto dalla radice di una struttura di S6 (gruppi[].radice.identita_proposta.letterale), "" se non c'è
function radiceDi(gr) {
  return (gr && gr.radice && gr.radice.identita_proposta && gr.radice.identita_proposta.letterale) || "";
}
// compatCodice: che cosa la lettura del Motore A dice del codice letto rispetto a quello del prodotto (le evidenze della
// proposta del server: base e marcatore), in parole; "" senza evidenze. Solo presentazione (messaggio 46, D02 e D31 [T]).
function compatCodice(sp) {
  const ev = arr(sp && sp.evidenze).filter((e) => ogg(e) && e.esito_base);
  const e = ev.find((x) => /step|radice/.test(String(x.selettore || "")) && x.esito_marcatore) || ev.find((x) => x.esito_marcatore) || ev[0];
  if (!e) return "";
  const base = e.esito_base === "uguale" || e.esito_base === "equivalente" ? "stesso codice di base" : "codice di base " + (ESITI_EVIDENZA[e.esito_base] || e.esito_base);
  return base + (e.esito_marcatore ? ", marcatore " + (ESITI_EVIDENZA[e.esito_marcatore] || e.esito_marcatore) : "");
}
// righeCodici: il codice del prodotto e quello letto dalla radice dello STEP, una riga sola e senza allarmi: un codice
// letto senza il marcatore del prodotto è un caso previsto, e lo dice la compatibilità del Motore A (messaggio 46, D02
// e D31 [T]). Lo stesso testo nell'anteprima e nel dialogo di conferma.
function righeCodici(codProdotto, radice, confronto, sp) {
  const compat = compatCodice(sp);
  const esito = confronto === "coincide" ? " · uguale" : radice && confronto === "diverso" ? " · " + (compat || "diverso dal codice del prodotto") : "";
  return ["Codice del prodotto: ", h("span", { class: "du-mono" }, codProdotto || "?"), " · codice letto dalla radice dello STEP: ",
    radice ? h("span", { class: "du-mono", "data-du-radice": radice }, radice) : "non letto", esito];
}
// riassuntoStruttura: che cosa si conferma (messaggio 46 [U]: «il dialogo di conferma mostra cosa si conferma»): la radice
// e i pezzi della struttura proposta, con le quantità, fino a max righe. g: il grafo di grafoProposta.
function riassuntoStruttura(g, max) {
  if (!g || !g.radice || !g.nodi[g.radice]) return null;
  max = max || 14;
  const righe = [];
  const visti = new Set();
  const giro = (k, liv, qta) => {
    if (visti.has(k)) return;
    visti.add(k);
    const v = g.nodi[k];
    if (righe.length < max) {
      righe.push(h("li", { class: "du-riass-riga", style: "padding-left:" + liv * 16 + "px", "data-k": k },
        qta !== undefined ? h("span", { class: "du-muto" }, "×" + qta + " ") : null, h("span", { class: "du-mono" }, v.cod || "(senza codice)"),
        v.rev ? " rev " + v.rev : "", " ", h("span", { class: "du-muto du-piccolo" }, TIPI[v.tipo] || v.tipo || ""), v.desc ? h("span", { class: "du-muto du-piccolo" }, " · " + breve(v.desc, 40)) : null));
    }
    for (const a of figliA(g, k)) giro(a.f, liv + 1, a.qta);
  };
  giro(g.radice, 0);
  const n = Object.keys(g.nodi).length;
  const figli = figliA(g, g.radice).length;
  return h("div", { class: "du-riass", id: "f-struttura", "data-nodi": String(n) },
    h("p", {}, h("b", {}, "Struttura che diventa la bozza: "), n + (n === 1 ? " pezzo" : " pezzi") + (figli ? ", " + figli + (figli === 1 ? " pezzo" : " pezzi") + " sotto la radice" : ", la sola radice")),
    h("ul", { class: "du-riass-elenco" }, righe), n > righe.length ? h("p", { class: "du-muto du-piccolo" }, "… e altri " + (n - righe.length) + " pezzi: li vedi nell'anteprima.") : null);
}
// fasciaStruttura: la riga dello stato della struttura dello STEP (struttura_step di S6), con il testo del server
const STRUTTURE_STEP = { con_struttura: "struttura dallo STEP", sola_radice: "sola radice: il motore non ha estratto figli", nessuna_struttura: "nessuna struttura importabile per il prodotto",
  non_determinabile: "struttura non determinabile adesso" };
const ANALISI_STEP = { pronta: "analisi conclusa", in_analisi: "analisi in corso", errore: "analisi fallita", non_valutato: "analisi non valutata" };
function fasciaStruttura(ss) {
  const tono = ss.struttura === "con_struttura" ? "info" : ss.struttura === "non_determinabile" ? "ver" : "ver";
  return h("div", { class: "du-striscia du-striscia--" + tono, id: "du-struttura-step", "data-struttura": ss.struttura || "", "data-analisi": ss.stato_analisi || "", "data-partenza": ss.partenza || "" },
    h("span", {}, (STRUTTURE_STEP[ss.struttura] || ss.struttura || "?") + " · " + (ANALISI_STEP[ss.stato_analisi] || ss.stato_analisi || "?") +
      (ss.struttura === "con_struttura" && typeof ss.figli_estratti === "number" ? " · figli estratti: " + ss.figli_estratti : "") + (ss.testo ? ". " + ss.testo : "") +
      (ss.struttura === "sola_radice" ? ". Se il prodotto è un pezzo unico, lo dichiari tu." : "")));
}
// azioniAnteprima: i gesti della proposta in vista (U2 §2.2; direttiva 39 §2-§3). Con la struttura (con figli o la sola
// radice, sul vero gruppo), S8 con la partenza dallo STEP: «Conferma questo STEP come sorgente di ‹P›», «Usa nuovo STEP
// per ‹P›…» o «Aggiorna proposta…» (bozza già su questa sorgente, caso (e)). Senza struttura importabile, o con l'analisi
// in corso o fallita (struttura_step.partenza = solo_prodotto), S8 con la partenza dal solo prodotto, con la presa d'atto.
// S7 («Associa lo STEP a ‹P›…») non c'è: S8 associa lo STEP al prodotto al suo passo 1 (rilievo del 20/10, [T-39]).
function azioniAnteprima(a, gr) {
  const prodotto = codiceProdotto();
  const az = h("div", { class: "du-azioni" });
  const f = fileDi(a.allegato_id);
  const ss = ogg(a.struttura_step) ? a.struttura_step : null;
  const sorg = sorgenteDelProdotto(S.prodotto);
  if (!gr) {
    if (ss && ss.partenza === "solo_prodotto") {
      const sb = bozzaLavorabile() ? sorgenteDellaBozza(S.prodotto) : null;
      const no = !f ? "lo STEP non è fra i file della Sorgente" : S.erroreBozza ? "la bozza del prodotto non si legge: ↻ Aggiorna" : bozzaAperta() && bozzaDaConvertire() ? "prima converti la bozza" :
        sb && !(sb.allegato_id === a.allegato_id && sb.gruppo === "solo_prodotto") ? "la bozza ha già uno STEP: la partenza dal solo prodotto non si applica" :
          sorg === a.allegato_id && sb ? "è già la sorgente confermata della bozza" : SG.inCorsoS8[S.prodotto] ? "è in corso la conferma della sorgente" : S.modello && S.modello.congelata ? CONGELATA : "";
      az.append(btn("Conferma come sorgente e parti dal solo prodotto…", () => apriSoloProdotto(a), { cls: "du-btn--sm du-btn--pri", id: "du-conferma-solo-prodotto", no }));
    }
    const associato = f && f.proposta && f.proposta.componente_id === S.prodotto;
    if (a.stato === "pronta" && associato) az.append(pill("ok", "STEP associato a " + prodotto));
    return az;
  }
  const c = { allegato_id: a.allegato_id, prodotto_id: S.prodotto, gruppo: gr.gruppo, impronta_contenuto: gr.impronta_contenuto, compatibilita: gr.compatibilita,
    radice_letterale: (gr.radice && gr.radice.identita_proposta && gr.radice.identita_proposta.letterale) || "" };
  const noGr = !gr.impronta_contenuto ? "il server non dà l'impronta della proposta" : "";
  if (S.erroreBozza) {
    az.append(btn("Conferma questo STEP come sorgente di " + prodotto, null, { cls: "du-btn--sm", id: "du-conferma-sorgente", no: "la bozza del prodotto non si legge: ↻ Aggiorna" }));
  } else if (bozzaAperta() && bozzaDaConvertire()) {
    az.append(btn("Usa nuovo STEP per " + prodotto + "…", null, { cls: "du-btn--sm", no: "prima converti la bozza" }));
  } else if (bozzaAperta() && a.effetto_sulla_bozza && a.effetto_sulla_bozza.stessa_sorgente && sorg === a.allegato_id) {
    az.append(btn("Aggiorna proposta…", apriRiallinea, { cls: "du-btn--sm", id: "du-riallinea-da-step", no: perchéNoAggiorna() }));
  } else {
    const ac = azioniCandidatura(c).find((x) => x.id.indexOf("conferma-") === 0 || x.id.indexOf("riallinea-") === 0);
    const id = ac && /^(Usa|Aggiorna)/.test(ac.t) ? (/^Aggiorna/.test(ac.t) ? "du-riallinea-da-step" : "du-cambia-sorgente") : "du-conferma-sorgente";
    if (ac) az.append(btn(ac.t, ac.fai, { cls: "du-btn--sm du-btn--pri", id, no: noGr || ac.no }));
  }
  return az;
}
// secondario: l'avviso «STEP secondario disponibile» (MAT SG-12): una riga, senza gesti; l'accettazione degli archi
// arriva in V5. La forma di step_secondario la dice il modello (nome del file, oppure l'allegato).
function secondario(f) {
  const s2 = f && f.step_secondario;
  if (!s2) return null;
  const nome = typeof s2 === "string" ? s2 : s2.nome_file || nomeFile(s2.allegato_id) || "uno STEP";
  return h("p", { class: "du-nota", id: "du-step-secondario" }, pill("info", "STEP secondario disponibile"), " ", h("span", { class: "du-mono" }, nome),
    typeof s2 === "object" && s2.motivo ? " · " + s2.motivo : "", ". I suoi archi non si accettano ancora da questa pagina.");
}

// ---- «+ Aggiungi file» (E1′)

let modCarica = null; // il modulo di caricamento resta lo stesso fra un ridisegno e l'altro (i file scelti non si perdono)
function moduloCarica() {
  if (modCarica) return modCarica;
  const input = h("input", { type: "file", id: "du-carica-file", multiple: true, "aria-label": "File da aggiungere" });
  const origine = h("select", { id: "du-carica-origine", "aria-label": "Origine del file" }, h("option", { value: "portale" }, "del cliente"), h("option", { value: "interna" }, "interna"));
  const esiti = h("ul", { class: "du-esiti-file", id: "du-carica-esiti", "aria-live": "polite" });
  const invia = h("button", { type: "button", class: "du-btn du-btn--sm du-btn--pri", id: "du-carica-invia" }, "Carica");
  invia.addEventListener("click", async () => {
    if (!input.files || !input.files.length) {
      avvisa("errore", "Scegli almeno un file da aggiungere.");
      return;
    }
    invia.disabled = true;
    try {
      await carica(input.files, origine.value);
      input.value = "";
    } finally {
      invia.disabled = false;
    }
  });
  modCarica = h("div", { class: "du-caricamento", id: "du-caricamento" }, h("b", {}, "+ Aggiungi file"),
    h("span", { class: "du-nota" }, "File, zip o 7z. Il file entra nella RFQ e viene analizzato: non si assegna da solo a un componente."),
    h("div", { class: "du-azioni" }, input, h("label", { class: "du-piccolo" }, "Origine ", origine), invia), esiti);
  modCarica.__esiti = esiti;
  return modCarica;
}
// mega: i byte in MB con una cifra decimale, per l'avanzamento del caricamento.
function mega(n) {
  return (Math.round((Number(n) || 0) / 104857.6) / 10).toLocaleString("it-IT", { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + " MB";
}
// carica: E1′ dalla coda della risorsa «carica». Il ponte non ha un tempo massimo totale per il caricamento, solo un
// tempo d'inattività: l'avviso dice quanto è stato inviato, così un caricamento lento si vede avanzare (decisioni
// dell'ondata A, §5 n. 3).
async function carica(files, origine) {
  avvisa("ok", "Caricamento in corso…");
  const progresso = (inviati, totale) => {
    if (!totale) return;
    const pct = Math.min(100, Math.floor((100 * inviati) / totale));
    avvisa("ok", pct >= 100 ? "Caricamento inviato (" + mega(totale) + "): il server scrive i file…" : "Caricamento in corso… " + pct + " % (" + mega(inviati) + " di " + mega(totale) + ")");
    const el = $("#du-avviso");
    if (el) el.setAttribute("data-du-caricamento", String(pct));
  };
  const r = await inSerie("carica", () => P.carica(files, origine, progresso));
  const el = $("#du-avviso");
  if (el) el.removeAttribute("data-du-caricamento");
  const ul = moduloCarica().__esiti;
  ul.replaceChildren();
  for (const f of arr(r.file)) {
    const testo = f.esito === "caricato" ? (f.ricaricato_dopo_scarto ? "ricaricato: è un file nuovo della RFQ; lo scarto di prima resta nella storia" : "caricato · " + (ANALISI[f.analisi] || f.analisi || "in analisi"))
      : f.esito === "gia_presente" ? (f.byte_ripristinati ? "già presente: i byte che mancavano sono stati ripristinati" : "già presente" + (f.stato_file === "altri_file" ? " (in Altri file: Riattiva)" : ""))
      : "rifiutato: " + ((f.motivo && f.motivo.testo) || "motivo non dato");
    ul.append(h("li", { "data-esito": f.esito }, h("span", { class: "du-mono" }, f.nome_file), pill(f.esito === "rifiutato" ? "ko" : f.esito === "gia_presente" ? "neu" : "ok", testo)));
  }
  // i rifiuti di E1′ (MAT §5.4): 413 «troppo grande: dividi in più invii» (anche prima del corpo: «richiesta troppo
  // grande», RF RQ-11 (d)); 503 «caricamento non configurato su questo server»; 422 NESSUN_FILE_CARICATO con i motivi
  const testi = { TROPPO_GRANDE: "Troppo grande: dividi in più invii.", PONTE_ERRORE_413: "Richiesta troppo grande: dividi in più invii.", NON_CONFIGURATO: "Caricamento non configurato su questo server.",
    NESSUN_FILE_CARICATO: "Nessun file è stato caricato: i motivi sono file per file, qui sotto." };
  if (r.esito === "caricati") avvisa("ok", r.testo || "File caricati.");
  else if (r.esito === "parziale") avvisa("no", "Caricamento parziale: " + (r.testo || "alcuni file non sono stati caricati") + ". Non caricati: " + arr(r.file).filter((f) => f.esito === "rifiutato").map((f) => f.nome_file).join(", ") + ".");
  else avvisa(r.esito === "no" ? "no" : "errore", testi[r.codice] || r.testo || "Il caricamento non è riuscito.");
  await caricaSorgente();
  await caricaModello();
  await caricaSmistamento();
  const nuovi = arr(r.file).filter((f) => f.esito === "caricato");
  // l'ultima mossa dice il gesto, non uno stato che poi cambia (collaudo UI46, giro 3, minore) [T]
  if (nuovi.length) registraMossa(nuovi.length === 1 ? "1 file caricato e mandato all'analisi" : nuovi.length + " file caricati e mandati all'analisi", { allegato: nuovi[0].allegato_id || "" });
  render();
}

// zoneTabs: sotto i 900 px la Revisione alterna la vista e il pannello del nodo (le zone)
function zoneTabs() {
  return h("div", { class: "du-zone-tabs", role: "group", "aria-label": "Zone" },
    h("button", { type: "button", "aria-pressed": S.zona === "vista" ? "true" : "false", onclick: () => {
      S.zona = "vista";
      render();
    } }, S.vista === "org" ? "Organigramma" : "Elenco"),
    h("button", { type: "button", "aria-pressed": S.zona === "nodo" ? "true" : "false", onclick: () => {
      S.zona = "nodo";
      render();
    } }, "Nodo scelto"));
}
// ---------------------------------------------------------------- il visore dei PDF (MAT FL-03, PN-04)

// pdfjs: pdf.js si carica una volta, dagli statici che il guscio dice (data-du-pdfjs e data-du-pdfjs-worker), come nella
// pagina vecchia. È il solo import dinamico della pagina. Il PDF lo legge pdf.js dall'indirizzo di S5 (urlContenuto,
// modo=anteprima), che costruisce il ponte: la pagina non fa richieste (MAT §1.3: «un indirizzo per pdf.js»).
let pdfjsPromessa = null;
function pdfjs() {
  const m = $("#du");
  const src = m && m.dataset.duPdfjs;
  const worker = m && m.dataset.duPdfjsWorker;
  if (!src || !worker) return Promise.resolve(null);
  if (!pdfjsPromessa) {
    const cartella = new URL(".", new URL(src, location.href)).href; // cmaps/, wasm/, iccs/, standard_fonts/
    pdfjsPromessa = import(src).then((lib) => {
      lib.GlobalWorkerOptions.workerSrc = worker;
      return { lib, cartella };
    }).catch(() => {
      pdfjsPromessa = null;
      return null;
    });
  }
  return pdfjsPromessa;
}
function ePdf(f) {
  return /^pdf$/i.test((f && f.estensione) || "") || /\.pdf$/i.test((f && f.nome_file) || "");
}
let visoreAttivo = null;
// apriVisore: il PDF nel visore della pagina, una pagina alla volta con ‹ ›. Un byte che non c'è (410 FILE_SCARTATO,
// BYTE_NON_DISPONIBILI) è «Contenuto non disponibile» con il motivo che la risposta permette di dire (MAT §5.4, S5).
async function apriVisore(f) {
  if (visoreAttivo) {
    try {
      visoreAttivo.destroy();
    } catch (e) {
      // niente
    }
    visoreAttivo = null;
  }
  const stato = { doc: null, n: 1 };
  const tela = h("div", { class: "du-visore-tela", id: "du-visore-tela" }, h("p", { class: "du-muto du-piccolo" }, "Apertura del PDF…"));
  const pagina = h("span", { class: "du-piccolo", id: "du-visore-pagina" }, "");
  const prima = h("button", { type: "button", class: "du-btn du-btn--sm", id: "du-visore-prima", disabled: true, "aria-label": "Pagina precedente" }, "‹");
  const dopo = h("button", { type: "button", class: "du-btn du-btn--sm", id: "du-visore-dopo", disabled: true, "aria-label": "Pagina seguente" }, "›");
  const disegna = async () => {
    const pag = await stato.doc.getPage(stato.n);
    const v1 = pag.getViewport({ scale: 1 });
    const larghezza = Math.max(220, Math.min(760, (window.innerWidth || 800) - 96));
    const viewport = pag.getViewport({ scale: larghezza / v1.width });
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.ceil(viewport.width));
    canvas.height = Math.max(1, Math.ceil(viewport.height));
    await pag.render({ canvas, viewport }).promise;
    tela.replaceChildren(canvas);
    pagina.textContent = "pagina " + stato.n + " di " + stato.doc.numPages;
    prima.disabled = stato.n <= 1;
    dopo.disabled = stato.n >= stato.doc.numPages;
  };
  prima.addEventListener("click", () => {
    if (stato.doc && stato.n > 1) {
      stato.n--;
      disegna();
    }
  });
  dopo.addEventListener("click", () => {
    if (stato.doc && stato.n < stato.doc.numPages) {
      stato.n++;
      disegna();
    }
  });
  apriFinestra("Visore: " + f.nome_file, h("div", { id: "du-visore", style: "display:grid;gap:8px" },
    h("div", { class: "du-azioni" }, prima, pagina, dopo,
      h("a", { class: "du-btn du-btn--sm du-btn--ghost", href: P.urlContenuto(f.allegato_id, "scarica"), download: f.nome_file || "" }, "Scarica")), tela),
  [{ testo: "Chiudi", chiudi: true }]);
  const p = await pdfjs();
  if (!p) {
    tela.replaceChildren(h("p", { class: "du-err", id: "du-visore-errore" }, "Il visore dei PDF non si carica: usa «Scarica»."));
    return;
  }
  try {
    const task = p.lib.getDocument({ url: P.urlContenuto(f.allegato_id, "anteprima"), isEvalSupported: false, enableXfa: false, verbosity: 0, wasmUrl: p.cartella + "wasm/",
      iccUrl: p.cartella + "iccs/", cMapUrl: p.cartella + "cmaps/", cMapPacked: true, standardFontDataUrl: p.cartella + "standard_fonts/" });
    visoreAttivo = task;
    stato.doc = await task.promise;
    await disegna();
  } catch (e) {
    // il visore pdf.js vede lo stato, non il corpo: il motivo lo dice l'intestazione X-Motivo di S5, anche sulle risposte
    // senza busta (EM §10.9, §10.10; DEC-B PF-25; richiesta RF RQ-11 (c)), letta dal ponte con una richiesta di un byte
    const st = e && (e.status || (e.details && e.details.status));
    const m = await P.motivoContenuto(f.allegato_id);
    tela.replaceChildren(h("p", { class: "du-err", id: "du-visore-errore", "data-du-motivo": m.motivo || "" }, "Contenuto non disponibile: " + testoMotivoContenuto(m.motivo, f, m.stato || st) + "."));
  }
}
// testoMotivoContenuto: il motivo per cui S5 non dà i byte, dal codice di X-Motivo (EM §10.9, §10.10). Un file in Altri
// file non si «ricarica»: la sua copia sta nella cartella ALTRO (RF RQ-11 (b)).
function testoMotivoContenuto(motivo, f, stato) {
  const ricarica = f && f.stato === "altri_file" ? "" : f && f.canale === "posta" ? " (per riaverlo, salvalo dalla mail e ricaricalo)" : " (per riaverlo, ricaricalo)";
  const t = {
    FILE_SCARTATO: "il file è stato scartato" + ricarica,
    BYTE_NON_DISPONIBILI: "i byte del file non ci sono più" + ricarica,
    COPIA_NAS_MANCANTE: "la copia nella cartella ALTRO della RFQ sul NAS non si trova al percorso registrato: la cartella può essere stata spostata o rinominata",
    COPIA_NAS_CAMBIATA: "la copia nella cartella ALTRO della RFQ sul NAS è stata cambiata: non è più il file conservato",
    SESSIONE_SCADUTA: "la sessione è scaduta: rientra e riapri il file",
    NON_AUTORIZZATO: "non hai i permessi per questo file",
    NON_TROVATO: "il file non c'è più in questa RFQ",
    RANGE_NON_VALIDO: "intervallo dei byte non valido",
    RICHIESTA_NON_VALIDA: "richiesta non valida",
    METODO_NON_AMMESSO: "richiesta non ammessa",
    PRECONDIZIONE_FALLITA: "il file è cambiato mentre lo aprivi: riprova",
    FUORI_RFQ: "il file non è di questa RFQ",
    ERRORE: "errore del server nel leggere il file",
  };
  if (motivo && t[motivo]) return t[motivo];
  if (motivo) return motivo;
  return stato ? "la rotta del contenuto risponde " + stato : "il file non si apre";
}
// bottoneApri: «Apri» di un file (MAT FL-03): un PDF nel visore della pagina, gli altri formati come documento dalla
// rotta S5 (il browser li scarica: S5 li dà come allegato).
function bottoneApri(f, id) {
  if (ePdf(f)) return btn("Apri", () => apriVisore(f), { cls: "du-btn--sm", id: id || null, perche: false });
  return h("a", { class: "du-btn du-btn--sm", href: P.urlContenuto(f.allegato_id, "anteprima"), target: "_blank", rel: "noopener", id: id || null }, "Apri");
}

// destinazioneFile: la preassociazione del file nella bozza che si vede (codice del pezzo e chiave) e i pezzi per cui il
// registro della bozza lo propone (associazioni_proposte delle voci). Solo presentazione: lo stato vero lo dà R4.
function destinazioneFile(f) {
  const out = { pb: "", pbChiave: "", pa: [], noninc: [], ricodifica: false };
  if (!S.lavoro || !f.allegato_id) return out;
  const nodoDi = (k) => S.lavoro.nodi.find((n) => n.chiave_bozza === k);
  const cod = (n) => (n && n.identita && n.identita.codice) || (n && n.descrizione) || (n && n.chiave_bozza) || "?";
  // un'associazione registrata di un componente con una ricodifica nella bozza: alla conferma il file segue il codice
  // nuovo, e la pagina lo segna «da ricodificare» (MAT §4.3 riga 7)
  const ar = arDi(f.allegato_id);
  const cid = ar && ar.componente_id;
  if (cid) out.ricodifica = S.lavoro.decisioni.some((d) => d.tipo === "ricodifica" && String(d.componente_id).toLowerCase() === cid);
  const pb = pbDi(f.allegato_id);
  if (pb) {
    out.pbChiave = pb.chiave_bozza;
    out.pb = cod(nodoDi(pb.chiave_bozza));
    return out;
  }
  const reg = registroBozza();
  const nonInc = S.lavoro.decisioni.filter((d) => d.tipo === "non_includere" && d.casella);
  for (const v of arr(reg && reg.proposta && reg.proposta.nodi)) {
    if (!arr(v.associazioni_proposte).some((a) => a.allegato_id === f.allegato_id)) continue;
    const rif = v.origine && v.origine.rif;
    const n = S.lavoro.nodi.find((x) => casellaDi(x) && casellaDi(x).rif === rif);
    if (n) out.pa.push(cod(n));
    // la proposta d'associazione di un nodo non incluso: «non richiesta: nodo non incluso (bozza)» (MAT §4.3 riga 5)
    else if (nonInc.some((d) => d.casella.rif === rif)) out.noninc.push(nomeVoce(v));
  }
  return out;
}

// ---------------------------------------------------------------- Revisione

function corpoRevisione() {
  if (!S.prodotto) return h("div", { class: "du-vuoto" }, "Nessun prodotto valutato: la Revisione non ha una BOM da mostrare.");
  if (S.erroreModello) return h("div", { class: "du-vuoto" }, "Il calcolo del motore non è riuscito: niente tela, bozza né wizard finché non riesce (D-18).");
  if (!bozzaAperta()) return revisioneSenzaBozza();
  if (bozzaDaConvertire()) return h("div", { class: "du-vuoto" }, "La bozza aperta è nel formato di prima: convertila per lavorarla (striscia qui sopra).");
  if (anteprimaInRevisione()) return h("div", { style: "display:grid;gap:10px" }, riquadroSenzaSorgente(), revisioneAnteprima());
  const g = grafoBozza();
  if (S.nodo && !g.nodi[S.nodo]) S.nodo = "";
  const vista = h("section", { class: "du-zona du-zona--vista", "aria-label": "BOM di lavoro" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, S.vista === "org" ? "Organigramma della bozza" : "Elenco della bozza"), statoConferma()),
    h("div", { class: "du-zona-corpo" }, S.vista === "org" ? [riquadroSenzaSorgente(), riquadroProposta(), riquadroAbbinamenti(), riquadroDifferenze(), strumentiOrg(g), cassettoSegnalati(g), postoSelezione(g), organigramma(g), legendaOrg(g)] :
      [riquadroSenzaSorgente(), riquadroProposta(), riquadroAbbinamenti(), riquadroDifferenze(), vassoio(), elencoRighe(g)]));
  const nodo = h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, "Pannello del nodo")),
    h("div", { class: "du-zona-corpo" }, g.nodi[S.nodo] ? pannelloNodo(g, g.nodi[S.nodo]) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo: identità proposta, riferimenti, provenienza e decisione dell'ingegnere stanno qui, non sulle schede.")));
  return h("div", {}, zoneTabs(), h("div", { class: "du-ws", "data-zona": S.zona }, vista, nodo));
}
// riquadroSenzaSorgente: la bozza senza una sorgente confermata da S8 (partenza vuota, o la sorgente revocata o superata;
// direttiva 39 §1): si lavora e si rivede, ma «Conferma BOM» richiederà uno STEP confermato (R4 lo dice con il blocco
// STEP_SORGENTE_NON_CONFERMATA, R5 rifiuta)
function riquadroSenzaSorgente() {
  if (!bozzaLavorabile() || sorgenteValida(S.prodotto)) return null;
  const sb = sorgenteDellaBozza(S.prodotto);
  const s8 = sorgenteS8(S.prodotto);
  const t = !sb ? "Bozza senza STEP: parte dal solo prodotto." : sb.gruppo === "solo_prodotto" ? "Bozza dal solo prodotto, con lo STEP " + nomeFile(sb.allegato_id) + " senza struttura." :
    "La sorgente della bozza (" + nomeFile(sb.allegato_id) + ") non è più confermata" + (s8 ? " (" + testoMotivoSorgente(s8.motivo) + ")" : "") + ".";
  const riga = h("div", { class: "du-striscia du-striscia--ver", id: "du-senza-sorgente" }, h("span", {}, t + " Si modifica e si salva; «Conferma BOM» richiederà uno STEP confermato come sorgente."),
    btn("Vai alla Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm", perche: false }));
  // G39-02 [U]: una bozza senza STEP mentre il motore propone uno STEP come sorgente: la proposta, con la conferma
  const sp = boxStepProposto(S.prodotto, "rev");
  return sp ? h("div", { style: "display:grid;gap:6px" }, riga, sp) : riga;
}
function revisioneSenzaBozza() {
  if (S.erroreBozza) {
    // una lettura non riuscita non è «nessuna bozza» (MAT §5.1, §5.4): nessun gesto di creazione, solo «↻ Aggiorna»
    return h("div", { class: "du-vuoto", id: "du-bozza-non-letta" },
      h("p", {}, "Lettura non riuscita: la bozza di " + (codiceProdotto() || "questo prodotto") + " non si legge (" + S.erroreBozza + "). Niente è mostrato come attuale finché la lettura non riesce."),
      h("div", { class: "du-azioni" }, btn("↻ Aggiorna", aggiorna, { cls: "du-btn--sm du-btn--pri", id: "du-aggiorna-bozza", perche: false })));
  }
  // messaggio 47 [U]: dopo la conferma la BOM resta visibile, come BOM confermata (la working), in sola lettura
  const u = ultimaConferma();
  if (u) return revisioneWorking(u);
  // con lo storico delle conferme non letto «Modifica la BOM confermata» resta, spento (corsia ALBERO, messaggio 46 D41)
  const gesti = gestiPartenza(storicoNonLetto());
  // le partenze che non si possono fare non si mostrano (lettura [T] dei messaggi 46-47); un motivo comune a tutte (la BOM
  // congelata) si dice in una riga sola. Lo storico non letto lo dice già la riga in fondo
  const ferma = !gesti.length && perchéNoPartenza("vuota") === CONGELATA ? CONGELATA : "";
  return h("div", { class: "du-vuoto", id: "du-nessuna-bozza" },
    statoConferma(),
    h("p", {}, "Nessuna bozza per " + codiceProdotto() + ". La BOM di lavoro nasce di solito dallo STEP confermato come sorgente nella Sorgente; senza STEP si può partire dal solo prodotto."),
    // G39-02 [U]: lo STEP proposto dal motore, da confermare prima di partire senza
    boxStepProposto(S.prodotto, "rev"),
    h("div", { class: "du-azioni" }, btn("Vai alla Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm", perche: false }), gesti),
    ferma ? h("p", { class: "du-muto du-piccolo", id: "du-partenze-ferme" }, "Una bozza nuova non parte: " + ferma + ".") : null,
    S.erroreStorico ? h("p", { class: "du-muto du-piccolo" }, "Lo storico delle conferme non si legge: " + S.erroreStorico) : null,
    anteprimaInRevisione() ? revisioneAnteprima() : null);
}
// gestiPartenza: i gesti che aprono una bozza nuova. «Modifica la BOM confermata» (con una conferma) è il gesto principale
// e resta anche spento, con il motivo; «Parti dalla BOM registrata» e «Parti dal solo prodotto…» compaiono solo quando si
// possono fare (lettura [T] dei messaggi 46-47: niente gesti spenti con la spiegazione lunga). Le regole sono quelle di
// perchéNoPartenza, invariate; il server ricontrolla.
function gestiPartenza(conConferma) {
  return [
    conConferma ? btn("Modifica la BOM confermata", () => creaDa("conferma"), { cls: "du-btn--sm du-btn--pri", id: "du-da-conferma", no: perchéNoPartenza("conferma") }) : null,
    perchéNoPartenza("registrato") ? null : btn("Parti dalla BOM registrata", () => creaDa("registrato"), { cls: "du-btn--sm", id: "du-da-registrato" }),
    perchéNoPartenza("vuota") ? null : btn("Parti dal solo prodotto…", apriSoloProdottoSenzaStep, { cls: "du-btn--sm", id: "du-da-vuota" }),
  ].filter(Boolean);
}

// ---------------------------------------------------------------- la BOM confermata (working) nella Revisione

// Messaggio 47 [U]: «la BOM scompare dopo che viene confermata. In realtà dobbiamo continuare a mostrarla facendo notare
// che sia la working». Senza bozza aperta e con una conferma, la Revisione mostra la BOM confermata in sola lettura
// (lettura [T]), con la stessa tela (organigramma) e lo stesso Elenco della bozza e il pannello del nodo senza gesti di
// scrittura; per cambiarla, «Modifica la BOM confermata» apre una bozza nuova da questa conferma (R2, partenza
// «conferma»). La struttura viene dal dettaglio di R7 (&conferma=, con i registri citati): una lettura che il ponte ha già,
// nessuna rotta nuova.
//
// leggiWorking: il dettaglio dell'ultima conferma, letto una volta per conferma (una conferma non cambia); alla risposta
// la Revisione si ridisegna. Una lettura non riuscita si dice, con «↻ Riprova», e non è «nessuna struttura».
function leggiWorking(u) {
  const chiave = S.prodotto + ":" + u.conferma_id;
  if (S.working && S.working.chiave === chiave) return S.working;
  const w = { chiave, stato: "in_lettura", d: null, errore: "" };
  S.working = w;
  P.storicoConferme(S.prodotto, { conferma: u.conferma_id, importazione: true }).then((j) => {
    w.d = ogg(j.dettaglio) ? j.dettaglio : null;
    w.stato = w.d ? "letta" : "errore";
    if (!w.d) w.errore = "lo storico non dà il dettaglio della conferma";
  }, (e) => {
    w.stato = "errore";
    w.errore = testoErrore(e);
  }).then(() => {
    if (S.working === w && S.passo === "revisione" && !bozzaAperta()) renderCorpo();
  });
  return w;
}
// grafoWorking: il grafo della BOM confermata dal dettaglio di R7, nella forma di grafoBozza, con modo «working»: le
// funzioni della tela, dell'Elenco e del pannello scrivono solo con modo «bozza», quindi qui niente trascinamento,
// selezione, quantità né gesti del nodo. Nodi e legami sono le righe certificate della conferma (componenti e archi); il
// nodo dello snapshot formato 3 con la stessa chiave_bozza dà la casella (la provenienza nel pannello) e, con i registri
// citati, la voce del registro. Il componente sta in cid (non in nodo.componente_id, che per le funzioni della bozza è
// un nodo di S.lavoro).
function grafoWorking(d) {
  const snap = !d.contenuto_non_leggibile && ogg(d.snapshot) ? normalizza(d.snapshot) : null;
  const perChiave = new Map(snap ? snap.nodi.filter((n) => n && n.chiave_bozza).map((n) => [n.chiave_bozza, n]) : []);
  const registri = arr(d.registri).map((r) => r && r.contenuto).filter(ogg);
  const voceDi = (c) => {
    for (const reg of registri) {
      const v = voceDiCasella(reg, c);
      if (v) return { v, reg };
    }
    return null;
  };
  const nodi = {};
  const chiaveDi = {};
  let radice = "";
  let registro = null;
  for (const c of arr(d.componenti)) {
    const cid = String((c && c.componente_id) || "").toLowerCase();
    if (!cid) continue;
    const k = c.chiave_bozza || "c:" + cid;
    const sn = perChiave.get(c.chiave_bozza) || null;
    const cas = sn ? casellaDi(sn) : null;
    const tv = cas ? voceDi(cas) : null;
    if (tv && !registro) registro = tv.reg;
    chiaveDi[cid] = k;
    nodi[k] = { k, cid, cod: c.codice || "", rev: c.rev || "", desc: c.descrizione || "", tipo: c.tipo || "sciolto", stato: { c: "reg", t: "confermato" },
      nodo: sn ? { caselle: arr(sn.caselle), identita: sn.identita } : { caselle: [] }, voce: tv ? tv.v : null,
      fornitore: typeof c.emittente === "string" && c.emittente.indexOf("partner:") === 0, cad: false, rivedere: [], wk: true };
    if (c.radice) radice = k;
  }
  const archi = [];
  for (const a of arr(d.archi)) {
    const p = chiaveDi[String((a && a.padre_id) || "").toLowerCase()];
    const f = chiaveDi[String((a && a.figlio_id) || "").toLowerCase()];
    if (p && f) archi.push({ id: p + ">" + f + (a.posizione ? "@" + a.posizione : ""), p, f, qta: a.qta, origine: a.origine || "conferma", kind: "conf", arco: a });
  }
  return { radice, nodi, archi, nonInclusi: [], modo: "working", registro };
}
// documentiWorking: i documenti di un pezzo della BOM confermata, cioè le associazioni registrate del suo componente,
// lette dalle righe di S9 come nella bozza (associazioniRegistrate); niente preassociazioni né proposte.
function documentiWorking(v) {
  const out = { pb: [], pa: [], ar: [], nonRichieste: [] };
  for (const [aid, ar] of associazioniRegistrate()) {
    if (!v.cid || ar.componente_id !== v.cid) continue;
    const f = fileDi(aid);
    if (f) out.ar.push(ar.documento && !f.documento ? Object.assign({}, f, { documento: { motivo: "documento_confermato" } }) : f);
  }
  return out;
}
// revisioneWorking: la Revisione senza bozza con la conferma u (l'ultima di R7): la testata che dice che è la BOM
// confermata (la working) e i segni della conferma, la nota con il gesto per cambiarla, poi la tela o l'Elenco e il
// pannello del nodo. Se dopo la conferma la struttura è cambiata (u.valida falso), la testata non la chiama working.
function revisioneWorking(u) {
  const w = leggiWorking(u);
  const da = u.confermato_da_nome || u.confermato_da_sigla || "";
  const il = u.confermato_il ? new Date(u.confermato_il) : null;
  const data = il && !isNaN(il.getTime()) ? " del " + il.toLocaleDateString("it-IT") : "";
  const titolo = "BOM confermata" + (u.valida ? " (working)" : "") + " · v" + u.numero + data + (da ? " da " + da : "");
  const nota = u.valida ? "È la distinta ufficiale. Per cambiarla: «Modifica la BOM confermata» apre una bozza nuova da questa conferma."
    : "Dopo questa conferma la struttura del prodotto è cambiata: qui c'è la BOM della conferma n. " + u.numero + ", e serve una conferma nuova. «Modifica la BOM confermata» apre una bozza da questa conferma.";
  const g = w.stato === "letta" ? grafoWorking(w.d) : null;
  if (g && S.nodo && !g.nodi[S.nodo]) S.nodo = "";
  let corpo;
  if (anteprimaInRevisione()) corpo = [revisioneAnteprima()];
  else if (w.stato === "in_lettura") corpo = [h("p", { class: "du-muto", id: "du-working-lettura" }, "Lettura della BOM confermata…")];
  else if (!g) {
    corpo = [h("div", { class: "du-striscia du-striscia--ko", id: "du-working-non-letta" }, h("span", {}, "Lettura non riuscita: la struttura della conferma n. " + u.numero + " non si legge (" + w.errore + ")."),
      btn("↻ Riprova", () => {
        S.working = null;
        renderCorpo();
      }, { cls: "du-btn--sm", id: "du-working-riprova", perche: false }))];
  } else if (!Object.keys(g.nodi).length) corpo = [h("p", { class: "du-muto", id: "du-working-vuota" }, "La conferma n. " + u.numero + " non ha righe certificate da mostrare.")];
  else if (S.vista === "org") {
    const vp = organigramma(g);
    // le quantità della BOM confermata non si cambiano da qui: il motivo della tela (pensato per la Sorgente) si dice per lei
    vp.querySelectorAll(".du-oq").forEach((b) => {
      b.disabled = true;
      b.title = (b.getAttribute("aria-label") || "") + " (BOM confermata, in sola lettura)";
    });
    corpo = [strumentiOrg(g), vp];
  } else corpo = [elencoRighe(g)];
  const vista = h("section", { class: "du-zona du-zona--vista", id: "du-working", "data-du-conferma": u.conferma_id, "aria-label": "BOM confermata" },
    h("div", { class: "du-zona-testa", id: "du-working-testa" }, h("h2", {}, titolo), statoConferma()),
    h("div", { class: "du-zona-corpo" },
      h("div", { class: "du-striscia " + (u.valida ? "du-striscia--info" : "du-striscia--ver"), id: "du-working-nota" }, h("span", {}, nota),
        h("span", { class: "du-azioni" }, gestiPartenza(true), btn("Vai alla Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm du-btn--ghost", perche: false }))),
      // G39-02 [U]: lo STEP proposto dal motore, se il prodotto non ha una sorgente confermata
      boxStepProposto(S.prodotto, "rev"),
      corpo));
  const scelto = g && g.nodi[S.nodo];
  const nodo = h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, "Pannello del nodo")),
    h("div", { class: "du-zona-corpo" }, scelto ? pannelloNodo(g, scelto) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo: identità, provenienza e documenti della BOM confermata stanno qui, in sola lettura.")));
  return h("div", {}, zoneTabs(), h("div", { class: "du-ws", "data-zona": S.zona }, vista, nodo));
}
// apriSoloProdottoSenzaStep: «Parti dal solo prodotto…» senza STEP (direttiva 39 §1 [U]): R2 crea con la partenza vuota,
// la bozza con la sola radice del prodotto finito, senza allegato, SHA, gruppo né registro. Si modifica, si salva e si
// rivede (R4 completo, con il blocco della sorgente); «Conferma BOM» resta vietata finché uno STEP non è confermato come
// sorgente da S8, che poi riconcilia la stessa bozza.
function apriSoloProdottoSenzaStep() {
  const no = perchéNoPartenza("vuota");
  if (no) return avvisa("no", "La bozza non parte: " + no + ".");
  const cod = codiceProdotto();
  apriFinestra("Parti dal solo prodotto · " + cod, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Apre una bozza di " + cod + " con il solo prodotto finito: aggiungi i pezzi, le quantità e le preassociazioni dei file a mano. Niente è letto da uno STEP."),
    h("p", { class: "du-esito du-esito--ver", id: "f-vuota-r5" }, "«Conferma BOM» richiederà uno STEP confermato come sorgente: quando lo confermerai dalla Sorgente, il server riconcilierà questa stessa bozza (i pezzi uguali si abbinano, gli incerti restano da decidere)."),
    // G39-02 [U]: la bozza manuale con uno STEP proposto e non confermato è molto sconsigliata
    avvisoStepProposto(S.prodotto, "")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Apri la bozza dal solo prodotto", pri: true, id: "f-ok", spento: () => {
    const m = mancaPresaProposto($("#du-finestra"));
    return m ? "manca " + m : "";
  }, fai: async () => {
    const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea: { partenza: "vuota" } });
    if (r.esito !== "ok") return esitoFormaKo(r);
    await dopoFormaServer(r, "Bozza aperta dal solo prodotto: «Conferma BOM» richiederà uno STEP confermato come sorgente.");
    return true;
  } }]);
}
// perchéNoPartenza: perché «Modifica la BOM confermata» (partenza «conferma») o «Parti dalla BOM registrata» (partenza
// «registrato») non partono, o "". Le dice lo storico delle conferme (R7): senza storico letto nessuna delle due, perché
// non si sa se il prodotto ha una conferma né di che formato; «registrato» non si offre con una conferma formato 3
// (PE-23; EM §6.12): si parte da quella.
// «vuota» (direttiva 39 §1; R39-19 [T-39]): non con una conferma formato 3 (si parte da quella) né con uno STEP confermato da
// S8 e valido (la bozza nasce da lì); il server ricontrolla (422 partenza_non_ammessa).
function perchéNoPartenza(partenza) {
  if (S.modello && S.modello.congelata) return CONGELATA;
  if (storicoNonLetto()) return STORICO_NON_LETTO;
  if (partenza === "vuota") {
    if (haConfermaFormato3()) return PARTENZA_REGISTRATO_NO;
    return sorgenteValida(S.prodotto) ? "lo STEP sorgente del prodotto è confermato: la bozza nasce da lì («Riprendi: crea la bozza» nella Sorgente)" : "";
  }
  if (partenza === "conferma") return ultimaConferma() ? "" : "il prodotto non ha una conferma";
  return haConfermaFormato3() ? PARTENZA_REGISTRATO_NO : haBomRegistrata() ? "" : "il prodotto non ha una BOM registrata";
}
// haBomRegistrata: il prodotto ha una BOM registrata da cui partire: una conferma (di un formato di prima: con il formato 3
// «registrato» non si offre) o componenti registrati sotto di lui secondo il motore. «Parti dalla BOM registrata» accanto
// a «BOM mai confermata», per un prodotto senza niente sotto, era una partenza dal solo prodotto con un altro nome
// (collaudo UI46, I6/D41 [T]); il server ricontrolla comunque.
function haBomRegistrata() {
  if (arr(S.storico && S.storico.conferme).length) return true;
  const p = prodottoScelto();
  return !!(p && p.registrati > 0);
}
// statoConferma: i segni di U2 §5.5 (MAT §6.2, recepimento di U2), distinti e mai fusi in una pillola sola: la conferma
// registrata (R7), la verifica corrente del server (/dati conferma_corrente.verifica_adesso, «verificata in prima
// istanza», ricalcolata a ogni lettura e mai salvata), la proposta aggiornata. Nessun valore lo decide la pagina:
// verifica_calcolata di R7 non si legge più (U2 §5.4; DEC-B RQ-8), e per un prodotto senza conferma formato 3 il «calcolo
// di oggi» si legge da bom.verificata dell'adattatore, non da prodotto_verificato (U2 §5.4).
function statoConferma() {
  const ultima = ultimaConferma();
  const out = h("span", { class: "du-azioni", id: "du-stato-conferma" });
  const da = ultima ? ultima.confermato_da_nome || ultima.confermato_da_sigla || (ultima.confermato_da ? String(ultima.confermato_da).slice(0, 8) : "") : "";
  if (ultima) out.append(pill(ultima.valida ? "ok" : "ver", ultima.valida ? "v" + ultima.numero + " confermata il " + dataOra(ultima.confermato_il) + (da ? " da " + da : "") : "v" + ultima.numero + " confermata, poi la struttura è cambiata: serve una conferma nuova", "conferma n. " + ultima.numero));
  // lo storico che non si legge non è «nessuna conferma» (MAT §5.4: niente si mostra come attuale)
  else if (storicoNonLetto()) out.append(h("span", { id: "du-conferme-non-lette" }, pill("ver", "conferme: non lette (↻ Aggiorna)", S.erroreStorico || "")));
  else out.append(pill("neu", "BOM mai confermata"));
  // aggiungi salta i segni assenti (null): Element.append li scriverebbe come testo «null»
  aggiungi(out, [segnoVerifica(), segnoPropostaAggiornata()]);
  // gli altri segni di MAT §6.2: il finito mono_pezzo (della conferma, o proposto nella bozza) e la BOM congelata
  if (ultima && ultima.valida && ultima.mono_pezzo) out.append(pill("info", "finito mono_pezzo"));
  else if (S.lavoro && S.lavoro.decisioni.some((d) => d.tipo === "mono_pezzo")) out.append(pill("bozza", "mono_pezzo proposto in bozza"));
  // G39-03 [U]: la conferma è una revisione interna, correzione della sorgente (R7 correzione_sorgente, revisione «interna»)
  if (ultima && (ultima.correzione_sorgente || ultima.revisione === "interna")) out.append(h("span", { id: "du-conferma-correzione", "data-revisione": "interna" }, pill("ver", "revisione interna: correzione della sorgente")));
  if (S.modello && S.modello.congelata) out.append(pill("ver", "BOM congelata"));
  return out;
}
// segnoVerifica: la verifica corrente (U2 §5.5, riga «verifica corrente»). Con una conferma formato 3: «Verificata in prima
// istanza, adesso: sì» / «… adesso: no — ‹motivo›» / «non calcolata adesso», con «poggia sulla conferma n». Senza: il
// calcolo di oggi dell'adattatore (bom.verificata), o «struttura condivisa» per un prodotto senza conferma propria che
// condivide lo STEP con un prodotto confermato (U2-P34; il nome del campo, struttura_condivisa, è una proposta della
// corsia: EM non lo dice).
const MOTIVI_VERIFICA = { fonte_non_autorizzata: "lo STEP della fonte non è autorizzato", fonte_non_confermata: "la fonte non è confermata", conferma_non_valida: "la conferma non vale più",
  righe_da_decidere: "ci sono righe della struttura da decidere", gerarchia_non_ricostruibile: "la gerarchia non viene dallo STEP: serve una deroga", rimozione_aperta: "c'è una rimozione proposta aperta",
  conflitto_motore: "c'è un conflitto aperto del motore", secondario_aperto: "c'è una riga aperta di uno STEP secondario", conflitti_non_calcolati: "i conflitti del motore non sono calcolati",
  documento_sostituito: "il documento della fonte è sostituito", fatti_cambiati: "i fatti strutturali della fonte sono cambiati" };
function motivoVerifica(m) {
  const c = (m && (m.codice || m.motivo)) || m;
  return (m && m.testo) || MOTIVI_VERIFICA[c] || String(c || "");
}
function segnoVerifica(dove) {
  const cc = S.modello && S.modello.confermaCorrente;
  const id = "du-verifica-adesso" + (dove ? "-" + dove : "");
  if (cc && ogg(cc.verifica_adesso)) {
    const va = cc.verifica_adesso;
    const poggia = cc.numero ? " · poggia sulla conferma n. " + cc.numero : "";
    if (va.stato !== "calcolata") return h("span", { id, "data-du-verifica": "non_calcolata" }, pill("neu", "Verificata in prima istanza: non calcolata adesso" + poggia, arr(va.motivi).map(motivoVerifica).join("; ")));
    if (va.verificata === true) return h("span", { id, "data-du-verifica": "si" }, pill("ok", "Verificata in prima istanza, adesso: sì" + poggia, va.criterio ? "criterio " + va.criterio : ""));
    const mot = arr(va.motivi).map(motivoVerifica).filter(Boolean)[0] || "motivo non dato";
    return h("span", { id, "data-du-verifica": "no" }, pill("ver", "Verificata in prima istanza, adesso: no — " + mot + poggia, arr(va.motivi).map(motivoVerifica).join("; ")));
  }
  const sc = S.modello && S.modello.strutturaCondivisa;
  if (sc) return h("span", { id, "data-du-verifica": "condivisa" }, pill("neu", "Verificata: non calcolata (struttura condivisa con " + (sc.codice || sc.prodotto_id || "un altro prodotto") + ")"));
  const p = prodottoScelto();
  if (!p) return null;
  // messaggio 46 (D43): «Verificata: no» con il primo motivo detto in una frase, non «calcolo di oggi»; il resto nel
  // suggerimento. Un motivo che la pagina non sa dire in parole non va nella testata: il suo codice sta solo nel
  // suggerimento (revisione della corsia ALBERO, rilievo 3 [T])
  const motivi = motiviDelProdotto(p);
  const primo = motivi.find((m) => m.frase);
  return h("span", { id, "data-du-verifica": p.verificatoOggi ? "oggi_si" : "oggi_no" }, pill(p.verificatoOggi ? "ok" : "neu", "Verificata: " + (p.verificatoOggi ? "sì" : "no" + (primo ? " — " + primo.frase : "")),
    motivi.map((m) => m.frase || "motivo del motore: " + m.codice).join("; ") + (p.criterioOggi && p.criterioOggi !== "nessuno" ? " · criterio " + p.criterioOggi : "")));
}
// motiviDelProdotto: i motivi per cui il prodotto non è verificato (valutazione.MotivoProdotto, dieci valori), ciascuno
// con la frase che lo dice: {codice, frase}; frase vuota per un codice che la pagina non conosce. La fonte strutturale
// dipende da che cosa c'è: con lo STEP sorgente confermato da S8 manca solo «Conferma BOM» (direttiva 39: solo R5 fa dello
// STEP la fonte della BOM); senza, manca lo STEP. [T]
const MOTIVI_PRODOTTO = { target_non_confermato: "il prodotto della RFQ non è confermato", fonte_superata: "lo STEP della BOM confermata è superato da uno più nuovo",
  nomenclatura_non_verificata: "i codici della BOM non sono verificati", gerarchia_non_verificata: "l'albero della BOM non è verificato",
  smistamento_non_verificato: "lo smistamento dei file non è verificato", documentazione_incompleta: "mancano dei documenti dei pezzi",
  documentazione_non_calcolabile: "la completezza dei documenti non si può ancora calcolare", conflitto: "c'è un conflitto da risolvere",
  revisione_tecnica_aperta: "c'è una revisione tecnica aperta" };
function fraseMotivoProdotto(codice, prodotto) {
  if (codice === "fonte_strutturale_step_mancante_o_non_confermata") {
    return sorgenteValida(prodotto || S.prodotto) ? "la BOM non è ancora confermata: lo STEP sorgente diventa la sua fonte con «Conferma BOM»" : "manca lo STEP sorgente confermato";
  }
  return MOTIVI_PRODOTTO[codice] || "";
}
function motiviDelProdotto(p) {
  return arr(p && p.motivi).map((m) => (m && (m.codice || m.motivo)) || m).filter((c) => typeof c === "string" && c)
    .map((c) => ({ codice: c, frase: fraseMotivoProdotto(c, p && p.id) }));
}
// segnoPropostaAggiornata: il segno «Proposta aggiornata: N differenze da valutare» (U2 §5.3, §5.5; C-04 [U]), con i gesti:
// «Vedi le differenze», e senza bozza «Rifiuta la proposta aggiornata…» (R15, U2 §5.7) e «Modifica la BOM confermata»
// (BZ-20); con la bozza la strada è la striscia «proposta cambiata» e «Aggiorna…» (BZ-17). Un rifiuto della stessa
// impronta si dice al posto del segno. Il segno non tocca la verifica.
function differenzePerTipo(pa) {
  const t = (pa && pa.per_tipo) || {};
  return ["identita", "revisione", "metadati", "quantita", "relazioni", "gerarchia"].reduce((n, k) => n + (Number(t[k]) || 0), 0);
}
function segnoPropostaAggiornata() {
  const cc = S.modello && S.modello.confermaCorrente;
  const pa = cc && cc.proposta_aggiornata;
  if (!ogg(pa)) return null;
  if (ogg(pa.rifiutata)) return h("span", { id: "du-proposta-aggiornata", "data-du-proposta": "rifiutata" },
    pill("neu", "Proposta del " + dataOra(pa.rifiutata.quando) + " rifiutata da " + (chiE(pa.rifiutata.autore) || "?")));
  const n = differenzePerTipo(pa) || arr(pa.differenze && pa.differenze.nodi).length + arr(pa.differenze && pa.differenze.archi).length;
  return h("span", { id: "du-proposta-aggiornata", class: "du-azioni", "data-du-proposta": "aperta" },
    pill("prop", "Proposta aggiornata: " + n + " differenze da valutare"),
    btn("Vedi le differenze", () => apriDifferenzeConferma(pa), { cls: "du-btn--sm du-btn--ghost", id: "du-pa-differenze", perche: false }),
    bozzaAperta() ? null : btn("Rifiuta la proposta aggiornata…", () => apriRifiutaProposta(cc), { cls: "du-btn--sm", id: "du-pa-rifiuta", no: perchéNoRifiuta(cc) }));
}
function perchéNoRifiuta(cc) {
  if (bozzaAperta()) return "con la bozza aperta il rifiuto è «Mantieni» per tutte le differenze, in «Aggiorna proposta…»";
  if (!cc || !cc.conferma_id) return "il modello non dice la conferma corrente: ↻ Aggiorna";
  const pa = cc.proposta_aggiornata || {};
  if (!pa.gruppo || !pa.impronta_contenuto) return "il modello non dà gruppo e impronta della proposta aggiornata: ↻ Aggiorna";
  if (P.sospesi().some((s) => s.risorsa === "proposta:" + S.prodotto && !s.certo)) return "un rifiuto di prima non ha un esito certo: verificalo dalla striscia";
  return "";
}
// apriDifferenzeConferma: le differenze della proposta aggiornata rispetto alla conferma (U2 §5.3), per tipo; le evidenze
// nuove come informazione. Solo lettura: nessuna richiesta, le dice /dati.
// le differenze dette con il codice del pezzo e a parole, mai con la voce del registro (n:<allegato>:nodo:…) né con i nomi
// dei campi (collaudo UI46, B2)
const CAMPI_DIFFERENZA = { decisione_esistente: "componente già registrato", abbinamento_proposto: "abbinamento a un componente", tipo_proposto: "tipo",
  codice_proposto: "codice", identita_proposta: "codice letto", motivo_proposto: "codice", codice_composto: "codice del cartiglio", motivo_lettura: "lettura del codice",
  alternative: "letture alternative", riferimenti: "riferimenti ai documenti", grezzi: "dati letti dallo STEP", codice_manuale_legacy: "codice delle pagine di prima",
  rev_proposta: "revisione", motivo_rev: "revisione", attributi: "attributi", segnali: "segnalazioni", diagnostiche: "segnalazioni", foglia: "assieme o particolare",
  leggibile: "leggibilità", entita_id: "entità dello STEP", radice_del_file: "radice del file", relazioni: "relazioni", padri: "padri",
  "stato_motore.raggiunto": "presenza nella struttura", "stato_motore.tolto": "pezzo tolto dal motore", quantita: "quantità", occorrenze: "occorrenze" };
const TIPI_DIFFERENZA = { aggiunto: "nuovo nella proposta", tolto: "non più nella proposta", cambiato: "cambia" };
function nomeDellaVoce(voce, k) {
  const n = k && S.lavoro ? S.lavoro.nodi.find((y) => y.chiave_bozza === k) : null;
  if (n && n.identita && n.identita.codice) return n.identita.codice;
  const m = voce ? nodoDelModello(voce) : null;
  const de = m && ogg(m.decisione_esistente) && ogg(m.decisione_esistente.identita_registrata) ? m.decisione_esistente.identita_registrata.codice : "";
  const c = de || (m && (m.codice_proposto || m.codice || (m.identita_proposta && m.identita_proposta.letterale) || (m.grezzi && (m.grezzi.nome || m.grezzi.id))));
  if (c) return String(c);
  // il pezzo che la BOM confermata ha già per quella voce (la stessa voce nei registri letti dalla pagina)
  for (const r of Object.values(S.registri || {})) {
    const v = r && arr(r.proposta && r.proposta.nodi).find((x) => x.voce === voce);
    if (v) return nomeVoce(v);
  }
  const reg = registroBozza();
  const v = reg && voce ? arr(reg.proposta && reg.proposta.nodi).find((x) => x.voce === voce) : null;
  return v ? nomeVoce(v) : (n && n.descrizione) || "un pezzo dello STEP";
}
function testoDifferenza(x) {
  const campi = [...new Set(arr(x.campi).map((c) => CAMPI_DIFFERENZA[c] || String(c).replace(/[._]/g, " ")))];
  return nomeDellaVoce(x.voce, x.chiave_bozza) + ": " + (TIPI_DIFFERENZA[x.tipo] || x.tipo || "") + (campi.length ? " (" + campi.join(", ") + ")" : "");
}
function testoDifferenzaArco(x) {
  const voce = (rif) => (x.allegato_id && rif ? "n:" + x.allegato_id + ":" + rif : "");
  const campi = [...new Set(arr(x.campi).map((c) => CAMPI_DIFFERENZA[c] || String(c).replace(/[._]/g, " ")))];
  return "legame " + nomeDellaVoce(voce(x.padre)) + " → " + nomeDellaVoce(voce(x.figlio)) + ": " + (TIPI_DIFFERENZA[x.tipo] || x.tipo || "") + (campi.length ? " (" + campi.join(", ") + ")" : "");
}
function apriDifferenzeConferma(pa) {
  const d = pa.differenze || {};
  const t = pa.per_tipo || {};
  const nomi = { identita: "identità", revisione: "revisione", metadati: "dati del pezzo", quantita: "quantità", relazioni: "relazioni", gerarchia: "struttura" };
  apriFinestra("Proposta aggiornata: le differenze", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La proposta di adesso del Motore A è diversa da quella usata alla conferma. È una proposta non ufficializzata: la BOM confermata non cambia finché non la riesamini, la modifichi e la confermi con il wizard."),
    h("ul", { class: "du-elenco-p", id: "f-pa-tipi" }, Object.keys(nomi).filter((k) => Number(t[k])).map((k) => h("li", {}, nomi[k] + ": " + t[k]))),
    h("ul", { class: "du-elenco-p", id: "f-pa-differenze" }, arr(d.nodi).map((x) => h("li", {}, testoDifferenza(x))), arr(d.archi).map((x) => h("li", {}, testoDifferenzaArco(x)))),
    arr(d.evidenze).length ? h("p", { class: "du-nota", id: "f-pa-evidenze" }, "Evidenze nuove o cambiate (informazione, non differenze della proposta): " + arr(d.evidenze).length + ".") : null),
  [{ testo: "Chiudi", chiudi: true }]);
}
// apriRifiutaProposta: «Rifiuta la proposta aggiornata…» (U2 §5.7; MAT BZ-27; EM §9.5-bis), senza bozza: R15. «Rifiuta»
// non è «Scarta» (file) né «Abbandona» (bozza). Il rifiuto non riattiva una fonte revocata, non risolve un prerequisito,
// non cambia la verifica corrente; per le differenze strutturali il dialogo offre «Modifica la BOM confermata».
function apriRifiutaProposta(cc) {
  const no = perchéNoRifiuta(cc);
  if (no) return avvisa("no", "Il rifiuto non parte: " + no + ".");
  const pa = cc.proposta_aggiornata;
  const strutturali = (Number((pa.per_tipo || {}).quantita) || 0) + (Number((pa.per_tipo || {}).gerarchia) || 0) + (Number((pa.per_tipo || {}).relazioni) || 0);
  apriFinestra("Rifiuta la proposta aggiornata di " + codiceProdotto(), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La BOM confermata (conferma n. " + (cc.numero || "?") + ") resta com'è; il segno «Proposta aggiornata» sparisce per questa proposta. Una proposta diversa lo riaccende."),
    h("p", { class: "du-nota" }, "Rifiutare non riattiva una fonte revocata, non rinnova un'autorizzazione, non decide righe nuove della struttura e non cambia la verifica corrente."),
    strutturali ? h("p", { class: "du-nota", id: "f-pa-strutturali" }, "Ci sono differenze di struttura: la verifica resta caduta anche dopo il rifiuto. Per tenerle fuori con una conferma nuova usa «Modifica la BOM confermata».",
      btn("Modifica la BOM confermata", () => {
        chiudiFinestra();
        S.passo = "revisione";
        creaDa("conferma");
      }, { cls: "du-btn--sm", id: "f-pa-modifica", no: perchéNoPartenza("conferma"), perche: false })) : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Rifiuta la proposta aggiornata", pri: true, id: "f-ok", fai: async () => {
    const corpo = { prodotto_id: S.prodotto, conferma_id: cc.conferma_id, gruppo: pa.gruppo, impronta_contenuto: pa.impronta_contenuto };
    const r = await inSerie("proposta:" + S.prodotto, () => P.rifiutaProposta(corpo));
    return esitoRifiuto(r);
  } }]);
}
async function esitoRifiuto(r) {
  if (r.esito === "ok") {
    const riletta = await rileggiDopoUnGesto([], "Il rifiuto della proposta aggiornata è registrato.");
    avvisa("ok", ((r.dati && r.dati.testo) || "Proposta aggiornata rifiutata: la BOM confermata resta com'è.") + (r.dati && r.dati.riproposto ? " (riproposto: era già registrato)" : "") + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."));
    render();
    return true;
  }
  if (r.esito === "da_verificare") {
    avvisa("da_verificare", "Esito del rifiuto da verificare: verifico con la stessa chiave.");
    verificaConRiprova(r.chiave, async (j) => {
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        await rileggiDopoUnGesto([], "Il rifiuto è registrato.");
        avvisa("ok", "Verificato: il rifiuto della proposta aggiornata è registrato.");
        render();
        return true;
      }
      if (j.stato === "non_registrato") {
        avvisa("no", "Il rifiuto non è ancora registrato: «Riprova» dalla striscia lo rimanda con lo stesso corpo e la stessa chiave.");
        renderStrisce();
        return true;
      }
      return false;
    });
    renderStrisce();
    return true;
  }
  if (r.codice === "ANTEPRIMA_SUPERATA") {
    await caricaModello();
    render();
    avvisa("conflitto", "La proposta aggiornata è cambiata mentre guardavi: rivedila, poi decidi di nuovo (ANTEPRIMA_SUPERATA).");
    return true;
  }
  const err = $("#du-finestra-errore");
  if (err) err.textContent = (r.testo || "Il rifiuto non è riuscito.") + (r.codice ? " (" + r.codice + ")" : "");
  avvisa(r.esito === "no" ? "no" : "errore", r.testo || "Il rifiuto non è riuscito.");
  return false;
}
function strumentiOrg(g) {
  const parti = [];
  if (g.modo === "bozza") {
    const no = perchéNoModifica();
    parti.push(h("span", { class: "du-grp", role: "group", "aria-label": "Aggiungi" },
      btn("+ Assieme", () => apriAggiungi("sottoassieme"), { cls: "du-btn--sm", id: "du-nuovo-ass", no, perche: false }),
      btn("+ Particolare", () => apriAggiungi("sciolto"), { cls: "du-btn--sm", id: "du-nuovo-par", no, perche: false }),
      btn("+ Commerciale", () => apriAggiungi("commerciale"), { cls: "du-btn--sm", id: "du-nuovo-com", no, perche: false })));
    parti.push(btn("Confronto con la proposta", () => {
      S.confronto = !S.confronto;
      renderCorpo();
    }, { cls: "du-btn--sm", id: "du-confronto", pressed: S.confronto, perche: false, t: "Segna le schede che non sono come le ha importate la bozza (il registro della bozza, non la proposta di oggi)" }));
    parti.push(btn("Nodi segnalati (" + Object.values(g.nodi).filter((x) => x.cad && !x.noninc).length + ")", () => {
      S.segnalatiAperto = !S.segnalatiAperto;
      render();
    }, { cls: "du-btn--sm", id: "du-segnalati-apri", pressed: S.segnalatiAperto, perche: false }));
  }
  parti.push(h("span", { class: "du-zoom", role: "group", "aria-label": "Zoom" },
    h("button", { type: "button", id: "du-zoom-meno", "aria-label": "Rimpicciolisci", onclick: () => zoomCentro(0.8) }, "−"),
    h("span", { class: "du-z", id: "du-zoom-val", "aria-live": "polite" }, "—"),
    h("button", { type: "button", id: "du-zoom-piu", "aria-label": "Ingrandisci", onclick: () => zoomCentro(1.25) }, "+"),
    h("button", { type: "button", id: "du-zoom-adatta", title: "Tutto l'organigramma nel riquadro (tasto 0)", onclick: () => setCam(vistaAdatta()) }, "Adatta"),
    h("button", { type: "button", id: "du-zoom-centra", title: "Centra il pezzo scelto", onclick: centraSel }, "Centra")));
  return h("div", { class: "du-strumenti", role: "toolbar", "aria-label": "Strumenti dell'organigramma" }, parti);
}
// controIndizio: l'etichetta di un contro-indizio del segnale LS-02 (codici dell'ancoraggio, SegnaleNodo.ControIndizi):
// non cambiano l'esito del segnale, aiutano a decidere. Un codice sconosciuto si mostra com'è. Il testo di
// grafo_non_completo è la frase (b) di P-5 del motore (richiesta RQ-I2, ondata B): la spiegazione del segnale arriva dal
// motore così com'è.
const CONTRO_INDIZI = {
  lettura_identita: "il nodo ha anche una lettura d'identità: potrebbe essere un pezzo vero",
  grafo_non_completo: "il grafo del file non è completo: che il nodo non abbia figli potrebbe non essere un'informazione",
};
function controIndizio(c) {
  return CONTRO_INDIZI[c] ? CONTRO_INDIZI[c] + " (" + c + ")" : String(c);
}
function cassettoSegnalati(g) {
  if (!S.segnalatiAperto || g.modo !== "bozza") return null;
  const segnalati = Object.values(g.nodi).filter((v) => v.cad && !v.noninc);
  const box = h("div", { class: "du-segnalati", id: "du-segnalati" });
  box.append(h("div", { class: "du-segnalati-t" }, h("h3", {}, "Nodi segnalati (" + segnalati.length + ")"),
    btn("Seleziona tutti i segnalati", () => {
      segnalati.forEach((v) => {
        if (selezionabile(g, v).ok) S.selezione.add(v.k);
      });
      render();
    }, { cls: "du-btn--sm", id: "du-seleziona-segnalati", no: segnalati.length ? perchéNoModifica() : "nessun nodo segnalato", perche: false })));
  const c = h("div", { class: "du-segnalati-c" });
  c.append(h("p", { class: "du-nota" }, "Il segnale del motore non toglie niente e non esenta dal 2D: decidi tu, nodo per nodo. La selezione è un aiuto: niente è salvato finché non fai «Non includere nella BOM…»."));
  for (const v of segnalati) {
    const sel = selezionabile(g, v);
    const seg = segnaliDi(v.voce)[0] || {};
    const padre = padriA(g, v.k)[0];
    c.append(h("label", { class: "du-segnalato", "data-k": v.k },
      h("input", { type: "checkbox", id: "sel-seg-" + sid(v.k), checked: S.selezione.has(v.k), disabled: !sel.ok, title: sel.motivo || null, "aria-label": "Seleziona " + ((v.voce && v.voce.grezzi && v.voce.grezzi.nome) || v.k),
        onchange: (e) => cambiaSelezione(v.k, e.target.checked) }),
      h("span", { class: "du-segnalato-n" }, (v.voce && v.voce.grezzi && (v.voce.grezzi.nome || v.voce.grezzi.id)) || v.desc || v.k, !sel.ok && sel.motivo ? h("span", { class: "du-perche" }, " non selezionabile: " + sel.motivo) : null),
      h("span", { class: "du-segnalato-s" }, (padre ? "sotto " + (g.nodi[padre.p].cod || g.nodi[padre.p].desc) + " · " : "") + (seg.regola || "") + (seg.spiegazione ? " · " + seg.spiegazione : "") +
        (arr(seg.contro_indizi).length ? " · contro-indizi: " + arr(seg.contro_indizi).map(controIndizio).join("; ") : ""))));
  }
  box.append(c);
  return box;
}
// postoSelezione: il posto della barra «N nodi selezionati», con l'altezza riservata anche senza selezione: la barra che
// compare o sparisce non sposta la tela sotto il puntatore (UI46, controllo di stabilità del 21/10 [T])
function postoSelezione(g) {
  if (g.modo !== "bozza") return barraSelezione(g);
  return h("div", { class: "du-selezione-posto", id: "du-selezione-posto" }, barraSelezione(g));
}
function barraSelezione(g) {
  if (!S.selezione.size) return null;
  for (const k of [...S.selezione]) if (!g.nodi[k] || !selezionabile(g, g.nodi[k]).ok) S.selezione.delete(k);
  if (!S.selezione.size) return null;
  return h("div", { class: "du-selezione", id: "du-selezione", role: "status" }, h("b", {}, S.selezione.size + " nodi selezionati"),
    btn("Non includere nella BOM…", () => apriNonIncludere(g), { cls: "du-btn--sm du-btn--pri", id: "du-non-includere", no: perchéNoModifica(), perche: false }),
    btn("Svuota", () => {
      S.selezione.clear();
      render();
    }, { cls: "du-btn--sm", id: "du-svuota-selezione", perche: false }));
}

// L'Elenco: la stessa BOM in righe, orientate ai documenti (MAT §3.6).
function ordineAlbero(g) {
  const out = [];
  const visti = new Set();
  const giro = (k, d, q) => {
    if (visti.has(k) || !g.nodi[k]) return;
    visti.add(k);
    out.push({ k, d, q });
    figliA(g, k).forEach((a) => giro(a.f, d + 1, a.qta));
  };
  if (g.radice) giro(g.radice, 0, null);
  Object.keys(g.nodi).forEach((k) => {
    if (!visti.has(k)) giro(k, 0, null);
  });
  return out;
}
function documentiNodo(v) {
  const out = { pb: [], pa: [], ar: [], nonRichieste: [] };
  if (S.lavoro && v.noninc) {
    // le proposte d'associazione di un nodo non incluso: «non richiesta: nodo non incluso (bozza)» (MAT §4.3 riga 5)
    for (const a of arr(v.voce && v.voce.associazioni_proposte)) {
      const f = fileDi(a.allegato_id);
      if (f && (!f.stato || f.stato === "attivo")) out.nonRichieste.push({ allegato_id: a.allegato_id });
    }
  }
  if (!S.lavoro || !v.nodo) return out;
  // pb: gli allegato_id preassociati al nodo; pa: le proposte del registro per i file preassociabili non ancora
  // preassociati, anche senza proposta di documento (direttiva 35, C-2, Prova 2)
  for (const d of S.lavoro.documenti_preassociati) if (d.chiave_bozza === v.k) out.pb.push(d.allegato_id);
  const giaPb = new Set(S.lavoro.documenti_preassociati.map((d) => d.allegato_id));
  for (const a of arr(v.voce && v.voce.associazioni_proposte)) {
    const f = fileDi(a.allegato_id);
    if (f && !giaPb.has(f.allegato_id) && preassociabile(f)) out.pa.push({ allegato_id: a.allegato_id, motivo: a.motivo, livello: a.livello });
  }
  // ar: le associazioni registrate del componente, dalle righe gia_registrato e da_scollegare di S9 (la AR attiva di
  // file_rfq_associazione, o il documento confermato senza AR: associazioniRegistrate), mai dalla copia di S1. Un documento
  // confermato resta segnato (f.documento di S1, o il motivo documento_confermato della riga)
  if (v.nodo.componente_id) {
    const cid = String(v.nodo.componente_id).toLowerCase();
    for (const [aid, ar] of associazioniRegistrate()) {
      if (ar.componente_id !== cid || giaPb.has(aid)) continue;
      const f = fileDi(aid);
      if (f) out.ar.push(ar.documento && !f.documento ? Object.assign({}, f, { documento: { motivo: "documento_confermato" } }) : f);
    }
  }
  return out;
}
// svincoloDi: la decisione scollega_documento della bozza per l'allegato ({allegato_id, componente_id}; provvisorio,
// base [T] D-4 A)
function svincoloDi(allegato) {
  return S.lavoro ? S.lavoro.decisioni.find((d) => d.tipo === "scollega_documento" && d.allegato_id === allegato) : null;
}
// esitoPreassociazione: l'esito di R4 per la PB dell'allegato. La forma delle PB nel riepilogo è di C-5: la pagina la
// cerca per allegato_id (come lo snapshot)
function esitoPreassociazione(allegato) {
  const r = S.riepilogo;
  const e = r && arr(r.documenti_preassociati).find((x) => x.allegato_id === allegato);
  return e ? e.esito : "";
}
// I filtri dell'Elenco (MAT EL-01, mockup v3): solo nel browser, sul modello che la pagina ha già. L'esito documentale di un
// pezzo: «conflitto» (una preassociazione che R4 dice in conflitto o orfana), «associato» (preassociato o associato),
// «proposto» (solo proposte del motore), «manca» (nessun documento, e non è un commerciale).
const FILTRI_EL = [["tutti", "Tutti"], ["manca", "Mancanti"], ["prop", "Proposti da verificare"], ["assoc", "Associati"], ["smistare", "Da smistare"], ["conflitti", "Conflitti"]];
function esitoDocumentale(v, doc) {
  if (v.noninc) return "nessuno";
  if (doc.pb.some((pid) => ["in_conflitto", "orfana"].includes(esitoPreassociazione(pid)))) return "conflitto";
  if (doc.pb.length || doc.ar.length) return "assoc";
  if (doc.pa.length) return "prop";
  return v.tipo === "commerciale" ? "nessuno" : "manca";
}
function passaFiltro(es, filtro) {
  switch (filtro) {
    case "manca": return es === "manca";
    case "prop": return es === "prop";
    case "assoc": return es === "assoc";
    case "smistare": return es === "manca" || es === "conflitto";
    case "conflitti": return es === "conflitto";
    default: return true;
  }
}
function passaCerca(v, doc) {
  const q = String(S.cerca || "").trim().toLowerCase();
  if (!q) return true;
  const nomi = doc.pb.map((aid) => (fileDi(aid) || {}).nome_file).concat(doc.pa.map((x) => nomeFile(x.allegato_id)), doc.ar.map((f) => f.nome_file));
  return [v.cod, v.desc, v.rev ? "rev " + v.rev : ""].concat(nomi).some((t) => String(t || "").toLowerCase().indexOf(q) >= 0);
}
// «Accetta le proposte (N)» (MAT EL-03; il nome di PE-21, decisioni dell'ondata A: la pagina non sa quali proposte sono
// «valide», lo dice R4 dopo il salvataggio): le proposte del motore per i pezzi della bozza, in un solo salvataggio;
// nessuna preselezione, l'elenco si riduce togliendo le spunte. Un pezzo escluso non riceve preassociazioni (P-15).
function proposteDaAccettare(g) {
  const out = [];
  for (const v of Object.values(g.nodi)) {
    if (v.noninc || !v.nodo || v.escluso) continue;
    for (const pa of documentiNodo(v).pa) out.push({ k: v.k, cod: v.cod || v.desc || v.k, pa });
  }
  return out;
}
function apriAccettaProposte(g) {
  const elenco = proposteDaAccettare(g);
  if (!elenco.length) return avvisa("no", "Nessuna proposta del motore da accettare.");
  // un file proposto per due pezzi si preassocia a uno solo: il server non ammette lo stesso allegato due volte nella bozza
  // (provvisorio, base [T] D-ASS-04 A), e il pulsante resta spento finché le spunte lo ripetono
  const spunte = () => [...$("#du-finestra").querySelectorAll("#f-proposte input:checked")].map((x) => ({ aid: x.dataset.aid, k: x.dataset.k }));
  const ripetuto = () => {
    const l = spunte().map((x) => x.aid);
    const doppio = l.find((x, i) => l.indexOf(x) !== i);
    return doppio ? "«" + nomeFile(doppio) + "» è spuntato per due pezzi: un file si preassocia a un pezzo solo" : "";
  };
  apriFinestra("Accetta le proposte", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "Ogni spunta diventa una preassociazione della bozza; nessuna è preselezionata. Le proposte senza spunta restano proposte. Se il codice del file non è quello del pezzo, il riepilogo la dirà «in conflitto»."),
    h("div", { class: "du-voci-sel", id: "f-proposte" }, elenco.map((x) => h("label", { class: "du-spunta" }, h("input", { type: "checkbox", "data-aid": x.pa.allegato_id, "data-k": x.k }),
      h("span", {}, h("span", { class: "du-mono" }, nomeFile(x.pa.allegato_id)), " → ", h("span", { class: "du-mono" }, x.cod), x.pa.motivo ? " · " + x.pa.motivo : "", x.pa.livello ? " (" + x.pa.livello + ")" : ""))))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Accetta le proposte scelte", pri: true, id: "f-ok", spento: () => (spunte().length ? ripetuto() : "scegli almeno una proposta"), fai: () => {
    const scelte = spunte();
    fai("accettate " + scelte.length + " proposte", scelte.map((x) => ({ azione: "file_preassociato", chiave_bozza: x.k, prima: null, dopo: { allegato_id: x.aid, chiave_bozza: x.k }, origine: "utente" })));
  } }]);
}
function filtriElenco(g, righe) {
  const conta = {};
  for (const [k] of FILTRI_EL) conta[k] = righe.filter((r) => passaFiltro(r.es, k)).length;
  const n = proposteDaAccettare(g).length;
  const cerca = h("input", { type: "search", id: "du-el-cerca", value: S.cerca, placeholder: "Cerca codice, rev o nome file", "aria-label": "Cerca per codice, revisione o nome file", autocomplete: "off" });
  cerca.addEventListener("input", (e) => {
    S.cerca = e.target.value;
    renderCorpo();
    const c = $("#du-el-cerca");
    if (c) {
      c.focus();
      c.setSelectionRange(c.value.length, c.value.length);
    }
  });
  return h("div", { class: "du-strumenti", role: "toolbar", "aria-label": "Filtri dell'elenco", id: "du-el-filtri" }, cerca,
    FILTRI_EL.map(([k, t]) => h("button", { type: "button", class: "du-btn du-btn--sm", id: "du-filtro-" + k, "aria-pressed": S.filtro === k ? "true" : "false", onclick: () => {
      S.filtro = k;
      renderCorpo();
    } }, t + " " + conta[k])),
    btn("Accetta le proposte (" + n + ")", () => apriAccettaProposte(g), { cls: "du-btn--sm", id: "du-accetta-valide", no: !n ? "nessuna proposta da accettare" : perchéNoModifica(), perche: false }),
    // «Scarta» e «Metti in Altri file» stanno nella Sorgente: qui il rimando (MAT §3.6)
    btn("Scarta o metti in Altri file: Gestisci nella Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm du-btn--ghost", id: "du-gestisci-sorgente", perche: false }));
}
function elencoRighe(g) {
  let righe = ordineAlbero(g);
  const box = h("div", { class: "du-el", id: "du-elenco", role: "table", "aria-label": "Elenco dei pezzi" });
  let filtri = null;
  if (g.modo === "bozza") {
    righe = righe.map((r) => {
      const doc = documentiNodo(g.nodi[r.k]);
      return Object.assign({}, r, { doc, es: esitoDocumentale(g.nodi[r.k], doc) });
    });
    filtri = filtriElenco(g, righe);
    righe = righe.filter((r) => passaFiltro(r.es, S.filtro) && passaCerca(g.nodi[r.k], r.doc));
  }
  box.append(h("div", { class: "du-el-r du-el-r--testa", role: "row" }, h("span", { role: "columnheader" }, "Pezzo"), h("span", { role: "columnheader" }, "Tipo · qtà"),
    h("span", { role: "columnheader" }, "Documenti"), h("span", { role: "columnheader" }, "Azioni")));
  for (const r of righe) {
    const v = g.nodi[r.k];
    const doc = g.modo === "bozza" ? documentiNodo(v) : g.modo === "working" ? documentiWorking(v) : { pb: [], pa: [], ar: [], nonRichieste: [] };
    const celleDoc = [];
    for (const aid of doc.pb) {
      const es = esitoPreassociazione(aid);
      celleDoc.push(h("span", { class: "du-el-doc" }, pill("bozza", "◈ preassociato"), h("span", { class: "du-mono" }, nomeFile(aid)), es ? pill(es === "applicabile" ? "ok" : "ver", es) : null,
        btn("Togli la preassociazione", () => togliPreassociazione(aid, v.k), { cls: "du-btn--sm du-btn--ghost", no: perchéNoModifica(), perche: false })));
    }
    for (const pa of doc.pa) {
      celleDoc.push(h("span", { class: "du-el-doc" }, pill("prop", "◇ proposto"), h("span", { class: "du-mono" }, nomeFile(pa.allegato_id)),
        btn("Accetta la proposta", () => preassocia(pa.allegato_id, v.k), { cls: "du-btn--sm", id: "du-accetta-" + sid(pa.allegato_id), no: v.escluso ? ESCLUSO_SENZA_PB : perchéNoModifica(), perche: false })));
    }
    const ricod = v.nodo && v.nodo.componente_id && arr(S.lavoro && S.lavoro.decisioni).some((d) => d.tipo === "ricodifica" && d.componente_id === v.nodo.componente_id);
    for (const f of doc.ar) {
      const sv0 = svincoloDi(f.allegato_id);
      // un documento già confermato non si scollega da qui: serve una revisione, non in V1 (EM §3.6.2)
      celleDoc.push(h("span", { class: "du-el-doc" }, pill(f.documento ? "ok" : sv0 ? "ver" : "info", f.documento ? "✓ confermato" : sv0 ? "⊘ da scollegare" : "● associato"), h("span", { class: "du-mono" }, f.nome_file),
        ricod ? pill("ver", "da ricodificare") : null,
        sv0 || f.documento || g.modo !== "bozza" ? null : btn("Scollega dal componente…", () => apriScollega(f, v), { cls: "du-btn--sm du-btn--ghost", no: perchéNoModifica(), perche: false })));
    }
    for (const x of doc.nonRichieste) celleDoc.push(h("span", { class: "du-el-doc" }, pill("neu", "◇ non richiesta: nodo non incluso (bozza)"), h("span", { class: "du-mono" }, nomeFile(x.allegato_id))));
    if (!celleDoc.length) celleDoc.push(h("span", { class: "du-muto du-piccolo" }, v.tipo === "commerciale" ? "2D non richiesto" : "nessun documento"));
    // fra le righe della bozza: l'esito documentale per i filtri (data-du-esito)
    box.append(h("div", { class: "du-el-r" + (S.nodo === r.k ? " du-el-r--scelta" : "") + (v.escluso || v.noninc ? " du-el-r--escl" : ""), role: "row", id: "el-" + sid(r.k), "data-k": r.k, "data-du-esito": r.es || null },
      h("button", { type: "button", class: "du-el-nodo", style: "--du-d:" + r.d, onclick: () => scegli(r.k) }, h("span", { class: "du-el-cod" }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "")),
        h("span", { class: "du-el-desc" }, v.desc || "—"), g.modo === "bozza" ? h("span", { class: "du-el-desc" }, idClienteNodo(r.k)) : null),
      h("span", { class: "du-el-celle" }, h("span", { class: "du-tipo du-tipo--" + v.tipo }, TIPI[v.tipo] || v.tipo), r.q ? h("span", { class: "du-q" }, "×" + r.q) : null, pill(v.stato.c === "reg" ? "ok" : v.stato.c, v.stato.t)),
      h("span", { class: "du-el-celle" }, celleDoc),
      h("span", { class: "du-el-celle" }, btn("Conferma il documento", null, { cls: "du-btn--sm", no: NON_V1.documento, perche: false }),
        btn("Conferma 2D", null, { cls: "du-btn--sm", no: NON_V1.documento, perche: false }))));
  }
  if (!righe.length && filtri) box.append(h("div", { class: "du-el-r", role: "row" }, h("span", { role: "cell" }, "Nessuna riga con questo filtro o questa ricerca."),
    btn("Mostra tutto", () => {
      S.filtro = "tutti";
      S.cerca = "";
      renderCorpo();
    }, { cls: "du-btn--sm", id: "du-filtro-azzera", perche: false })));
  // i documenti dell'Elenco vengono da S1: se S1 non si è riletto, lo si dice (MAT §5.4)
  const avvisoFile = g.modo === "bozza" && S.erroreSorgente ? h("div", { class: "du-striscia du-striscia--ko", id: "du-el-file-non-letti" }, "Lettura non riuscita: i documenti dei pezzi non sono aggiornati. ↻ Aggiorna per riprovare.") : null;
  return filtri ? h("div", { style: "display:grid;gap:8px;min-width:0" }, avvisoFile, filtri, box) : box;
}
// il vassoio «Da ricollocare»: dal giornale con la regola scritta (EM §3.7), più le azioni non ancora salvate
export function vassoioDa(eventi) {
  const efficaci = [];
  const annullati = new Set();
  const ord = eventi.slice().sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  for (const e of ord) {
    const n = e.dopo && Number(e.dopo.n);
    if (e.azione === "annulla" && n) annullati.add(n);
    else if (e.azione === "rifai" && n) annullati.delete(n);
  }
  for (const e of ord) if (["file_svincolato", "file_preassociato", "file_lasciato"].includes(e.azione) && !annullati.has(Number(e.n))) efficaci.push(e);
  // la chiave è l'occorrenza (allegato_id; direttiva 35, C-2); il vassoio è ordinato per allegato_id, come quello di R1
  const ultima = new Map();
  for (const e of efficaci) {
    const aid = (e.dopo && e.dopo.allegato_id) || (e.prima && e.prima.allegato_id);
    if (aid) ultima.set(aid, e);
  }
  const out = [];
  for (const [aid, e] of ultima) {
    const motivo = (e.dopo && e.dopo.motivo) || e.motivo || "utente";
    if (e.azione === "file_svincolato" && motivo !== "utente") out.push({ allegato_id: aid, motivo, chiave_bozza: e.prima && e.prima.chiave_bozza });
  }
  return out.sort((a, b) => (a.allegato_id < b.allegato_id ? -1 : a.allegato_id > b.allegato_id ? 1 : 0));
}
function vassoioCorrente() {
  const eventi = arr(S.bozza && S.bozza.eventi).slice();
  let n = S.totaleEventi;
  for (const v of (S.invio ? S.invio.voci : []).concat(S.coda)) for (const a of v.inviare) eventi.push({ n: ++n, azione: a.azione, prima: a.prima, dopo: a.dopo });
  if (!Array.isArray(S.bozza && S.bozza.eventi)) return arr(S.bozza && S.bozza.vassoio);
  return vassoioDa(eventi);
}
// i motivi di file_svincolato in parole (EM §3.7, §9.2; nodo_escluso: decisioni dell'ondata A, PE-22): il codice resta
// accanto, perché è quello del giornale
const MOTIVI_SVINCOLO = { utente: "tolta a mano", nodo_tolto: "pezzo tolto", nodo_escluso: "pezzo escluso", non_incluso: "nodo non incluso",
  fonte_cambiata: "fonte cambiata", rif_scaduto: "riferimento scaduto" };
function motivoSvincolo(m) {
  return MOTIVI_SVINCOLO[m] ? MOTIVI_SVINCOLO[m] + " (" + m + ")" : String(m || "");
}
// un pezzo escluso non riceve preassociazioni: lo snapshot non lo ammette (formato 3, regole di forma di P-15)
const ESCLUSO_SENZA_PB = "il pezzo è escluso: un documento non si preassocia a un pezzo escluso (↺ Ripristina, poi preassocia)";
function vassoio() {
  const voci = vassoioCorrente();
  if (!voci.length) return null;
  const box = h("div", { class: "du-riq", id: "du-vassoio" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "Da ricollocare (" + voci.length + ")"),
    h("span", { class: "du-muto du-piccolo" }, "File preassociati tolti da un gesto: niente è cancellato.")));
  const scelto = S.nodo && S.lavoro ? S.lavoro.nodi.find((n) => n.chiave_bozza === S.nodo) : null;
  const noScelto = !S.nodo || S.nodo.indexOf("ni:") === 0 || !scelto ? "scegli prima un pezzo della bozza" : scelto.op === "esclude" ? ESCLUSO_SENZA_PB : "";
  for (const x of voci) {
    box.append(h("div", { class: "du-gesto", "data-du-motivo": x.motivo }, h("span", { class: "du-mono" }, nomeFile(x.allegato_id)), pill("neu", motivoSvincolo(x.motivo)),
      btn("Preassocia al nodo scelto", () => preassocia(x.allegato_id, S.nodo), { cls: "du-btn--sm", id: "du-ricolloca-" + sid(x.allegato_id), no: noScelto || perchéNoModifica(), perche: false }),
      btn("Lascia senza associazione", () => lascia(x.allegato_id), { cls: "du-btn--sm du-btn--ghost", id: "du-lascia-" + sid(x.allegato_id), no: perchéNoModifica(), perche: false })));
  }
  return box;
}
// abbinamentiProposti: gli abbinamenti non certi della riconciliazione fra la bozza senza STEP e lo STEP arrivato con S8
// (direttiva 39 §3; algoritmo cambio-fonte-2 della persistenza): l'ultimo evento sorgente_cambiata li porta in
// dopo.abbinamenti_proposti[] {chiave_bozza, voci[], motivo}, fuori dal vocabolario chiuso di «da rivedere». Un nodo
// toccato dopo (giornale o coda), che non c'è più o che ha già una casella non è più da decidere; quelli lasciati con
// «Tieni separati» restano fuori per questa pagina (S.abbinamentiLasciati: niente si scrive, la bozza resta com'è).
function abbinamentiProposti() {
  if (!bozzaLavorabile() || !S.lavoro) return [];
  const ev = arr(S.bozza && S.bozza.eventi).slice().sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  let ultimo = -1;
  ev.forEach((e, i) => {
    if (e.azione === "proposta_riallineata" || e.azione === "sorgente_cambiata") ultimo = i;
  });
  if (ultimo < 0) return [];
  const e = ev[ultimo];
  const toccati = new Set();
  for (const x of ev.slice(ultimo + 1)) if (x.chiave) toccati.add(x.chiave);
  for (const v of S.coda) for (const a of v.inviare) if (a.chiave_bozza) toccati.add(a.chiave_bozza);
  if (S.invio) for (const v of S.invio.voci || []) for (const a of v.inviare || []) if (a.chiave_bozza) toccati.add(a.chiave_bozza);
  const lasciati = S.abbinamentiLasciati && S.abbinamentiLasciati.bozza === S.bozza.bozza_id ? S.abbinamentiLasciati.chiavi : new Set();
  return arr(e.dopo && e.dopo.abbinamenti_proposti).filter((x) => {
    if (!ogg(x) || !x.chiave_bozza || toccati.has(x.chiave_bozza) || lasciati.has(x.chiave_bozza)) return false;
    const n = S.lavoro.nodi.find((y) => y.chiave_bozza === x.chiave_bozza);
    return !!n && !casellaDi(n);
  });
}
const MOTIVI_ABBINAMENTO = { ambiguo: "più candidati: la corrispondenza non è univoca", incompatibile: "codice simile ma revisione, emittente, maiuscole o registrazione diversi",
  evidenze_discordanti: "identità uguale ma evidenze che non concordano (o scritte a mano)" };
// riquadroAbbinamenti: la vista della riconciliazione (direttiva 39 §3): per ogni nodo della bozza senza STEP con un
// abbinamento incerto, le voci dello STEP candidate e i gesti espliciti. «Abbina a ‹voce›» = un salvataggio R2
// nodo_modificato {caselle: [casella della voce], identita} (prima {caselle: null, identita}): lo stesso nodo, la stessa
// chiave, acquisisce la casella (nessun doppione). «Tieni separati» non scrive niente; la voce resta fuori dalla bozza, e
// si porta come nodo nuovo con «Importa dalla proposta…». Le quantità e gli archi dell'ingegnere non cambiano.
function riquadroAbbinamenti() {
  const l = abbinamentiProposti();
  if (!l.length) return null;
  const reg = registroBozza();
  const no = perchéNoModifica();
  const box = h("div", { class: "du-riq", id: "du-abbinamenti" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "Abbinamenti da decidere (" + l.length + ")"),
    h("span", { class: "du-muto du-piccolo" }, "Lo STEP arrivato dopo: i pezzi certi sono già abbinati, questi no. Niente si unisce da solo.")));
  for (const x of l) {
    const n = S.lavoro.nodi.find((y) => y.chiave_bozza === x.chiave_bozza);
    const voci = arr(x.voci).map((vid) => ({ vid, voce: reg ? indiceVoci(reg) && indiceVoci(reg).get(vid) : null }));
    const riga = h("div", { class: "du-gesto du-abbinamento", "data-du-abbinamento": x.chiave_bozza, "data-motivo": x.motivo || "" },
      h("span", { class: "du-mono" }, voceEffetto(x.chiave_bozza)), pill("info", MOTIVI_ABBINAMENTO[x.motivo] || x.motivo || "da decidere"));
    for (const y of voci) {
      const v = y.voce;
      const nome = v ? nomeVoce(v) + (v.rev_proposta ? " rev " + v.rev_proposta : "") : y.vid;
      const perche = !reg ? "il registro della bozza non è letto: ↻ Aggiorna" : !v ? "la voce non è nel registro della bozza: ↻ Aggiorna" :
        !v.origine || !v.origine.rif ? "la voce non ha una casella" : voceNonImportabile(v) ? MOTIVO_VOCE_REGISTRATA :
          !v.codice_proposto && !["registrato", "decisione"].includes(n.identita && n.identita.modo) ? "la voce non ha un codice proposto: portala con «Importa dalla proposta…»" : no;
      riga.append(btn("Abbina a " + nome, () => accettaAbbinamento(n, v, reg), { cls: "du-btn--sm", id: "du-abbina-" + sid(x.chiave_bozza) + "-" + sid(y.vid), no: perche }));
    }
    riga.append(btn("Tieni separati", () => lasciaAbbinamento(x.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", id: "du-abbina-no-" + sid(x.chiave_bozza), perche: false }));
    box.append(riga);
  }
  return box;
}
function accettaAbbinamento(n, v, reg) {
  const casella = { allegato_id: v.origine.allegato_id, gruppo: reg.gruppo && reg.gruppo.chiave, rif: v.origine.rif };
  const tieneIdentita = ["registrato", "decisione"].includes(n.identita && n.identita.modo);
  const proposta = nodoDaVoce(v, n.chiave_bozza, casella).identita;
  const dopo = { caselle: [casella], identita: tieneIdentita ? clona(n.identita) : proposta };
  fai("abbinato " + (n.identita && n.identita.codice ? n.identita.codice : n.chiave_bozza) + " alla voce dello STEP",
    [{ azione: "nodo_modificato", chiave_bozza: n.chiave_bozza, prima: { caselle: null, identita: clona(n.identita) }, dopo, origine: "utente" }]);
}
function lasciaAbbinamento(k) {
  if (!S.abbinamentiLasciati || S.abbinamentiLasciati.bozza !== S.bozza.bozza_id) S.abbinamentiLasciati = { bozza: S.bozza.bozza_id, chiavi: new Set() };
  S.abbinamentiLasciati.chiavi.add(k);
  avvisa("ok", "Tenuti separati: il pezzo della bozza resta com'era; la voce dello STEP si porta, se serve, con «Importa dalla proposta…».");
  renderCorpo();
}
// ---- le differenze fra la bozza e lo STEP dopo la riconciliazione (direttiva 39 §3 [U + T-39]; EM §6.5 riga «arco con la
// quantità cambiata», §6.6 passi 5-6, App. I.7 «Quantità, padre e figlio, rimozioni, categorie e decisioni»)

// Dopo l'ultimo cambio della fonte o riallineamento (sorgente_cambiata, proposta_riallineata; con S8 anche la
// riconciliazione della bozza senza STEP), le differenze di archi e nodi fra la bozza e lo STEP della bozza: la quantità di
// un legame (lo STEP dice ×4, la bozza ×2), un legame dello STEP che la bozza non ha, un legame della bozza che lo STEP non
// ha, il tipo di un pezzo. «Mantieni» è il predefinito e non scrive niente; «Recepisci» è un gesto normale della bozza
// (qta_modificata, arco_aggiunto con origine step, arco_tolto con togli_arco per un legame registrato, il dialogo del
// tipo). Nessuna sovrascrittura silenziosa.
// La fonte: differenze {archi[], nodi[]} del server nel dopo dell'evento, se c'è (forma di propostabom.Differenze: archi
// {allegato_id, padre, figlio: rif dello STEP, tipo aggiunto|tolto|cambiato, campi[]}, nodi {chiave_bozza, voce, tipo,
// campi[]}). Oggi il server le mette nella risposta di R2 (importazione.differenze) ma non nell'evento né nella risposta di
// S8 (richiesta alla corsia persistenza): finché mancano, la pagina le ricava dal registro della bozza, che è del server
// (reg.proposta.archi con la quantità dei fatti; il tipo proposto delle voci), per i legami fra due pezzi con la casella
// dello STEP della bozza e, per il tipo, per i pezzi scritti a mano abbinati a una voce. Scelta [T-39] della corsia
// (rilievo del revisore del 20/10). Un legame toccato dopo l'evento non è più da decidere; «Mantieni» vale per la pagina
// aperta (S.differenzeTenute), come «Tieni separati».
function eventoFonteUltimo() {
  const ev = arr(S.bozza && S.bozza.eventi).slice().sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  let ultimo = -1;
  ev.forEach((e, i) => {
    if (e.azione === "proposta_riallineata" || e.azione === "sorgente_cambiata") ultimo = i;
  });
  return ultimo < 0 ? null : { e: ev[ultimo], dopo: ev.slice(ultimo + 1) };
}
const AZIONI_LEGAMI = ["arco_aggiunto", "arco_tolto", "arco_spostato", "qta_modificata"];
function differenzeRiconciliazione() {
  if (!bozzaLavorabile() || !S.lavoro) return [];
  const u = eventoFonteUltimo();
  if (!u) return [];
  const reg = registroBozza();
  // toccato dopo l'evento: un legame con un gesto sui legami del suo figlio (la chiave degli eventi dei legami è il figlio),
  // un pezzo con un nodo_modificato; «Abbina a…» (nodo_modificato delle caselle) non nasconde le differenze dei legami
  const toccatiArchi = new Set(), toccatiNodi = new Set();
  const tocca = (azione, k) => {
    if (!k) return;
    if (AZIONI_LEGAMI.includes(azione)) toccatiArchi.add(k);
    if (azione === "nodo_modificato") toccatiNodi.add(k);
  };
  for (const x of u.dopo) tocca(x.azione, x.chiave);
  for (const v of S.coda) for (const a of v.inviare) tocca(a.azione, a.chiave_bozza);
  if (S.invio) for (const v of S.invio.voci || []) for (const a of v.inviare || []) tocca(a.azione, a.chiave_bozza);
  const tenute = S.differenzeTenute && S.differenzeTenute.bozza === S.bozza.bozza_id ? S.differenzeTenute.chiavi : new Set();
  const perRif = new Map();
  const nodo = new Map();
  for (const n of S.lavoro.nodi) {
    nodo.set(n.chiave_bozza, n);
    const c = casellaDi(n);
    if (c && c.rif && n.op !== "esclude") perRif.set(c.rif, n.chiave_bozza);
  }
  const arco = (p, f) => S.lavoro.archi.find((a) => a.padre === p && a.figlio === f) || null;
  const qtaStep = new Map();
  for (const a of arr(reg && reg.proposta && reg.proposta.archi)) qtaStep.set(a.padre + ">" + a.figlio, a);
  const out = [];
  const metti = (d) => {
    if (!d.id || tenute.has(d.id) || out.some((x) => x.id === d.id)) return;
    if (d.genere === "arco" && toccatiArchi.has(d.figlio)) return;
    if (d.genere === "nodo" && toccatiNodi.has(d.k)) return;
    out.push(d);
  };
  const daArco = (tipo, rp, rf, origine) => {
    const kp = perRif.get(rp), kf = perRif.get(rf);
    if (!kp || !kf) return;
    const a = arco(kp, kf);
    const s = qtaStep.get(rp + ">" + rf) || null;
    if (tipo === "cambiato" && (!a || !s)) return;
    if (tipo === "aggiunto" && a) return;
    if (tipo === "tolto" && !a) return;
    metti({ genere: "arco", tipo, id: "a:" + tipo + ":" + kp + ">" + kf, padre: kp, figlio: kf, qtaBozza: a ? a.qta : null, qtaStep: s && typeof s.quantita === "number" ? s.quantita : null,
      step: s ? { allegato_id: s.allegato_id, gruppo: reg && reg.gruppo && reg.gruppo.chiave } : null, arco: a, origine });
  };
  const srv = ogg(u.e.dopo) && ogg(u.e.dopo.differenze) ? u.e.dopo.differenze : null;
  if (srv) {
    for (const x of arr(srv.archi)) if (ogg(x)) daArco(x.tipo, x.padre, x.figlio, "server");
    for (const x of arr(srv.nodi)) if (ogg(x) && x.chiave_bozza && arr(x.campi).includes("tipo")) metti({ genere: "nodo", tipo: "cambiato", id: "n:tipo:" + x.chiave_bozza, k: x.chiave_bozza, origine: "server" });
  } else if (reg) {
    const nellaBozza = new Set();
    for (const a of S.lavoro.archi) {
      const np = nodo.get(a.padre), nf = nodo.get(a.figlio);
      const cp = casellaDi(np), cf = casellaDi(nf);
      if (!cp || !cf) continue;
      nellaBozza.add(cp.rif + ">" + cf.rif);
      const s = qtaStep.get(cp.rif + ">" + cf.rif);
      if (!s) daArco("tolto", cp.rif, cf.rif, "registro");
      else if (typeof s.quantita === "number" && s.quantita !== a.qta) daArco("cambiato", cp.rif, cf.rif, "registro");
    }
    for (const s of arr(reg.proposta && reg.proposta.archi)) if (!nellaBozza.has(s.padre + ">" + s.figlio)) daArco("aggiunto", s.padre, s.figlio, "registro");
    for (const n of S.lavoro.nodi) {
      const c = casellaDi(n);
      if (!c || n.origine !== "utente" || n.componente_id || n.op === "esclude") continue;
      const v = voceDiCasella(reg, c);
      const tv = v && v.tipo_proposto && v.tipo_proposto.valore;
      if (tv && tv !== n.tipo && tv !== "finito") metti({ genere: "nodo", tipo: "cambiato", id: "n:tipo:" + n.chiave_bozza, k: n.chiave_bozza, tipoStep: tv, origine: "registro" });
    }
  }
  return out;
}
function nomeNodoBozza(k) {
  const n = S.lavoro && S.lavoro.nodi.find((y) => y.chiave_bozza === k);
  return (n && n.identita && n.identita.codice) || (n && n.descrizione) || k;
}
const TESTI_DIFFERENZA = { cambiato: "quantità diversa", aggiunto: "legame dello STEP che la bozza non ha", tolto: "legame della bozza che lo STEP non ha" };
// azioniRecepisci: il gesto normale che porta la bozza a quello che dice lo STEP; null se non si può (con il motivo).
function azioniRecepisci(d) {
  if (d.genere !== "arco") return null;
  if (d.tipo === "cambiato") {
    if (typeof d.qtaStep !== "number" || !d.arco) return { no: "lo STEP non dà la quantità di questo legame" };
    return { azioni: [{ azione: "qta_modificata", chiave_bozza: d.figlio, prima: { padre: d.padre, figlio: d.figlio, qta: d.arco.qta }, dopo: { padre: d.padre, figlio: d.figlio, qta: d.qtaStep }, origine: "utente" }] };
  }
  if (d.tipo === "aggiunto") {
    if (typeof d.qtaStep !== "number") return { no: "lo STEP non dà la quantità di questo legame: aggiungilo con «+ Collega anche sotto…»" };
    const vietato = vietatoSotto(d.figlio, d.padre);
    if (vietato) return { no: vietato };
    return { azioni: [{ azione: "arco_aggiunto", chiave_bozza: d.figlio, prima: null, dopo: { padre: d.padre, figlio: d.figlio, qta: d.qtaStep, origine: "step", step: d.step }, origine: "utente" }] };
  }
  if (d.tipo === "tolto") {
    if (!d.arco) return { no: "il legame non è più nella bozza" };
    const az = [{ azione: "arco_tolto", chiave_bozza: d.figlio, prima: clona(d.arco), dopo: null, origine: "utente" }];
    const np = S.lavoro.nodi.find((y) => y.chiave_bozza === d.padre), nf = S.lavoro.nodi.find((y) => y.chiave_bozza === d.figlio);
    if ((d.arco.origine === "registrato" || d.arco.origine === "conferma") && np && nf && np.componente_id && nf.componente_id) {
      az.push({ azione: "decisione_aggiunta", chiave_bozza: null, prima: null, dopo: { id: nuovoIdDecisione("ta"), tipo: "togli_arco", padre_id: np.componente_id, figlio_id: nf.componente_id }, origine: "utente" });
    }
    return { azioni: az };
  }
  return null;
}
// riquadroDifferenze: la vista delle differenze nella Revisione (Organigramma ed Elenco), accanto agli abbinamenti.
function riquadroDifferenze() {
  const l = differenzeRiconciliazione();
  if (!l.length) return null;
  const no = perchéNoModifica();
  const box = h("div", { class: "du-riq", id: "du-differenze", "data-n": String(l.length) }, h("div", { class: "du-riq-testa" }, h("h3", {}, "Differenze con lo STEP (" + l.length + ")"),
    h("span", { class: "du-muto du-piccolo" }, "Dopo l'ultimo cambio dello STEP: la bozza resta com'è finché non recepisci. Niente si sovrascrive da solo.")));
  for (const d of l) {
    const riga = h("div", { class: "du-gesto du-differenza", "data-du-differenza": d.id, "data-genere": d.genere, "data-tipo": d.tipo, "data-origine": d.origine });
    if (d.genere === "arco") {
      const q = d.tipo === "cambiato" ? "bozza ×" + d.qtaBozza + ", STEP ×" + (d.qtaStep === null ? "?" : d.qtaStep) : d.tipo === "aggiunto" ? "STEP ×" + (d.qtaStep === null ? "?" : d.qtaStep) : "bozza ×" + d.qtaBozza;
      riga.append(h("span", { class: "du-mono" }, nomeNodoBozza(d.padre) + " → " + nomeNodoBozza(d.figlio)), pill("info", TESTI_DIFFERENZA[d.tipo] || d.tipo), h("span", { class: "du-piccolo", "data-du-qta": "" }, " " + q));
      const r = azioniRecepisci(d) || { no: "non recepibile da qui" };
      riga.append(btn("Recepisci", () => fai("recepito dallo STEP: " + nomeNodoBozza(d.padre) + " → " + nomeNodoBozza(d.figlio) + " (" + (TESTI_DIFFERENZA[d.tipo] || d.tipo) + ")", r.azioni),
        { cls: "du-btn--sm", id: "du-recepisci-" + sid(d.id), no: r.no || no }));
    } else {
      const n = S.lavoro.nodi.find((y) => y.chiave_bozza === d.k);
      riga.append(h("span", { class: "du-mono" }, nomeNodoBozza(d.k)), pill("info", "tipo diverso"), h("span", { class: "du-piccolo" }, " bozza " + (TIPI[n && n.tipo] || (n && n.tipo) || "?") + (d.tipoStep ? ", STEP " + (TIPI[d.tipoStep] || d.tipoStep) : "")));
      riga.append(btn("Recepisci…", () => {
        const g = grafoBozza();
        if (g.nodi[d.k]) apriTipo(g.nodi[d.k]);
      }, { cls: "du-btn--sm", id: "du-recepisci-" + sid(d.id), no }));
    }
    riga.append(btn("Mantieni", () => tieniDifferenza(d.id), { cls: "du-btn--sm du-btn--ghost", id: "du-mantieni-" + sid(d.id), perche: false }));
    box.append(riga);
  }
  return box;
}
function tieniDifferenza(id) {
  if (!S.differenzeTenute || S.differenzeTenute.bozza !== S.bozza.bozza_id) S.differenzeTenute = { bozza: S.bozza.bozza_id, chiavi: new Set() };
  S.differenzeTenute.chiavi.add(id);
  avvisa("ok", "Mantenuto: la bozza resta com'era (nessun salvataggio).");
  renderCorpo();
}
function daRivedere() {
  const m = new Map();
  const ev = arr(S.bozza && S.bozza.eventi).slice().sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  let ultimo = -1;
  ev.forEach((e, i) => {
    if (e.azione === "proposta_riallineata" || e.azione === "sorgente_cambiata") ultimo = i;
  });
  if (ultimo < 0) return m;
  const e = ev[ultimo];
  for (const r of arr(e.dopo && e.dopo.da_rivedere)) {
    const k = r.chiave_bozza || r.decisione_id || r.decisione;
    if (!k) continue;
    if (!m.has(k)) m.set(k, []);
    m.get(k).push(r.motivo);
  }
  // un nodo toccato dopo non è più da rivedere (MAT §5.1)
  for (const x of ev.slice(ultimo + 1)) if (x.chiave) m.delete(x.chiave);
  for (const v of S.coda) for (const a of v.inviare) if (a.chiave_bozza) m.delete(a.chiave_bozza);
  return m;
}

// ---------------------------------------------------------------- il pannello del nodo (MAT §6.3)

function riga(dt, dd) {
  return [h("dt", {}, dt), h("dd", {}, dd === "" || dd === null || dd === undefined ? "—" : dd)];
}
// parola: un valore del vocabolario del server (snake_case) detto con le parole: «non_verificabile» → «non verificabile»
const parola = (s) => String(s === null || s === undefined ? "" : s).replace(/_/g, " ");
// parolaEmittente: chi ha emesso il codice, per nome (messaggio 46, D14): «cliente · acme», «fornitore beta»; mai
// l'identificativo interno del cliente
// testoQualificatore: il testo di un qualificatore della lettura (il marcatore, un affisso): l'originale letto, o il valore
function testoQualificatore(q) {
  if (typeof q === "string") return q;
  if (!ogg(q)) return "";
  return typeof q.originale === "string" && q.originale ? q.originale : typeof q.valore === "string" ? q.valore : "";
}
// l'emittente in parole: il namespace della grammatica del cliente è un identificativo, non si mostra (collaudo UI46)
function parolaEmittente(em, ns) {
  if (typeof em !== "string" || !em) return "";
  if (em.indexOf("partner:") === 0) return "fornitore " + (ns || em.slice(8));
  if (em.indexOf("cliente:") === 0) return "cliente";
  return parola(em);
}
function testoLettura(l) {
  if (!l) return "—";
  const em = parolaEmittente(l.emittente, l.namespace);
  return (l.letterale || "—") + (em ? " (" + em + ")" : "");
}
// sezione: una sezione richiudibile del pannello del nodo, come nella pagina di prima (messaggio 46, D15). Resta aperta o
// chiusa come l'ha lasciata chi guarda, anche quando la pagina si ridisegna.
function sezione(id, titolo, aperta, ...corpo) {
  const st = S.sezioni[id];
  const d = h("details", { class: "du-sez", id, open: st === undefined ? !!aperta : st }, h("summary", {}, h("h3", {}, titolo)), h("div", { class: "du-sez-c" }, corpo));
  d.addEventListener("toggle", () => {
    S.sezioni[id] = d.open;
  });
  return d;
}
const TIPI_RIFERIMENTO = { documentale: "riferimento documentale", lettura_ruolo: "lettura con ruolo", equivalente_k4: "P/N equivalente", manuale_legacy: "codice corretto a mano" };
function pannelloNodo(g, v) {
  const box = h("div", { class: "du-nodo", id: "du-pannello", "data-k": v.k });
  box.append(h("div", {}, h("div", { class: "du-nodo-cod" }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "")), v.desc ? h("div", { class: "du-muto du-piccolo" }, v.desc) : null,
    h("div", { class: "du-azioni" }, chipTipo(v), pill(v.stato.c === "reg" ? "ok" : v.stato.c, v.stato.t),
      v.rivedere && v.rivedere.length ? pill("info", "da rivedere: " + v.rivedere.join(", ")) : null)));
  // le linguette «Dettaglio nodo» e «Documenti del nodo» (MAT PN-02): solo nel browser
  box.append(h("div", { class: "du-linguette", role: "tablist", "aria-label": "Pannello del nodo" }, [["dettaglio", "Dettaglio nodo"], ["documenti", "Documenti del nodo"]].map(([k, t]) =>
    h("button", { type: "button", role: "tab", id: "du-ling-" + k, "aria-selected": S.linguettaNodo === k ? "true" : "false", onclick: () => {
      S.linguettaNodo = k;
      renderCorpo();
      // la linguetta cambia la vista del pannello: il fuoco resta sulla linguetta, senza scorrere (UI46, stabilità)
      const lg = document.getElementById("du-ling-" + k);
      if (lg) lg.focus({ preventScroll: true });
    } }, t))));
  if (S.linguettaNodo === "documenti") {
    box.append(documentiDelNodo(g, v));
    return box;
  }
  // il 2D del nodo, grande, come nella pagina di prima (messaggio 46, D15): un clic lo apre nel visore
  box.append(miniatura(v, true));
  // i gesti subito sotto il disegno: prima stavano in fondo, fuori vista (D15)
  if (g.modo === "bozza") box.append(gestiNodo(g, v));
  const voce = v.voce;
  // 1 · identità proposta
  const ip = voce && voce.identita_proposta;
  const s1 = sezione("du-sez-proposta", ["Identità proposta ", h("span", { class: "du-muto du-piccolo" }, "proposta del Motore A")], true);
  const c1 = s1.lastChild;
  if (!voce) c1.append(h("p", { class: "du-muto du-piccolo" }, v.nuovo ? "Pezzo aggiunto a mano: nessuna identità proposta." : "Nessuna voce del registro per questo pezzo."));
  else {
    c1.append(h("dl", { class: "du-dl" },
      riga("Letterale", ip ? h("span", { class: "du-mono" }, ip.letterale) : "nessuna lettura" + (voce.motivo_lettura ? " (" + parola(voce.motivo_lettura) + ")" : "")),
      riga("Emittente", ip ? parolaEmittente(ip.emittente, ip.namespace) : ""),
      riga("Base", ip && ip.base ? ip.base.normalizzata : ""),
      // il marcatore è un oggetto {originale, valore, intervallo}, come gli affissi (collaudo UI46, I2: «[object Object]»)
      riga("Qualificatori", ip && ip.qualificatori ? [testoQualificatore(ip.qualificatori.marcatore), ...arr(ip.qualificatori.affissi).map(testoQualificatore)].filter(Boolean).join(" · ") : ""),
      riga("Revisione letta", ip && ip.revisione ? ip.revisione.originale + " (" + parola(ip.revisione.stato) + ")" : ""),
      riga("Codice proposto", voce.codice_proposto ? h("span", { class: "du-mono" }, voce.codice_proposto + (voce.rev_proposta ? " rev " + voce.rev_proposta : "")) : "vuoto" + (voce.motivo_proposto ? " (" + parola(voce.motivo_proposto) + ")" : "")),
      voce.codice_composto && voce.codice_composto.testo ? riga("Codice del cartiglio", voce.codice_composto.testo) : null));
  }
  box.append(s1);
  // 4 · decisione dell'ingegnere, tipo e categoria (solo nella Revisione)
  // gestiCategoria è null per un nodo non incluso: aggiungi lo salta (Element.append scriverebbe «null»)
  if (g.modo === "bozza") aggiungi(box, [sezioneDecisione(g, v), gestiCategoria(g, v)]);
  // la posizione nell'albero, come nella pagina di prima (messaggio 46, D15): sotto quali padri e con quale quantità
  if (!v.noninc) box.append(sezionePosizione(g, v));
  // 2 · riferimenti e relazioni
  const rif = [];
  for (const a of arr(voce && voce.alternative)) rif.push(h("li", {}, "alternativa: ", h("span", { class: "du-mono" }, testoLettura(a.lettura || a)), a.motivo ? " · " + parola(a.motivo) : ""));
  const regAdotta = g.modo === "bozza" ? registroBozza() : g.registro;
  let azioniRif = false;
  for (const r of arr(voce && voce.riferimenti)) {
    const valore = r.lettura ? testoLettura(r.lettura) : r.equivalente ? (r.equivalente.base || "") + " (" + (parolaEmittente(r.equivalente.emittente, r.equivalente.namespace) || "emittente non concorde") + ")" : r.valore || "";
    const testo = (TIPI_RIFERIMENTO[r.tipo] || parola(r.tipo)) + (r.ruolo ? " «" + parola(r.ruolo) + "»" : "") + (valore ? ": " + valore : "");
    const k4 = r.tipo === "equivalente_k4" || !!r.equivalente;
    const ad = g.modo === "bozza" && v.nodo && (r.lettura || r.equivalente) ? letturaDaAdottare(r, regAdotta) : null;
    if (ad) azioniRif = true;
    rif.push(h("li", { "data-du-riferimento": r.tipo || "" }, testo, r.esito ? " · " + parola(r.esito) : "", ad ? h("span", { class: "du-azioni" },
      btn("Adotta come codice…", () => apriAdotta(v, r, ad), { cls: "du-btn--sm", no: ad.motivo || (v.escluso ? "il pezzo è escluso: ↺ Ripristina, poi decidi l'identità" : "") || perchéNoModifica() }),
      // il rifiuto esplicito del P/N K4: un'alternativa sulla lettura primaria (MAT PN-08; EM §3.6.1; senza motivo dal messaggio 46 [U])
      k4 ? btn("Conferma l'identità proposta…", () => apriConfermaIdentita(v, r), { cls: "du-btn--sm", id: "du-conferma-identita", no: perchéNoConfermaIdentita(v) }) : null) : null));
  }
  if (voce && voce.codice_manuale_legacy) {
    azioniRif = azioniRif || g.modo === "bozza";
    rif.push(h("li", {}, "codice corretto a mano nella pagina vecchia (evidenza): ", h("span", { class: "du-mono" }, voce.codice_manuale_legacy.codice),
      // il codice manuale si adotta solo su un nodo da creare (U2-P35, [U] Q16 con la regola [T]; MAT PN-08): il codice di un
      // componente registrato cambia solo con una ricodifica, e R2 rifiuterebbe l'identità decisa
      g.modo === "bozza" ? h("span", { class: "du-azioni" }, btn("Adotta come codice…", () => apriAdotta(v, { tipo: "manuale_legacy" }, { codice: voce.codice_manuale_legacy.codice, rev: voce.codice_manuale_legacy.rev || "" }),
        { cls: "du-btn--sm", id: "du-adotta-manuale", no: (v.escluso ? "il pezzo è escluso: ↺ Ripristina, poi decidi l'identità" : "") ||
          (v.nodo && v.nodo.op !== "crea" ? "per cambiare il codice di un componente registrato serve una ricodifica (✎ Correggi codice, rev e descrizione…)" : "") || perchéNoModifica() }),
        h("span", { class: "du-perche" }, "Corretto a mano nella pagina vecchia: autore e data non registrati.")) : null));
  }
  const reg = g.modo === "bozza" ? registroBozza() : g.registro;
  for (const r of arr(reg && reg.proposta && reg.proposta.relazioni).filter((x) => voce && x.sorgente && x.sorgente.voce === voce.voce)) {
    rif.push(h("li", {}, "relazione «" + parola(r.tipo) + "» · " + parola(r.stato || "proposta") + ", non ratificata", h("span", { class: "du-azioni" },
      btn("Accetta o rifiuta", null, { cls: "du-btn--sm", no: NON_V1.relazioni }))));
  }
  box.append(sezione("du-sez-riferimenti", "Riferimenti e relazioni", azioniRif, rif.length ? h("ul", { class: "du-elenco-p" }, rif) : h("p", { class: "du-muto du-piccolo" }, "Nessun riferimento né relazione.")));
  // 3 · provenienza: il file si legge; gli identificativi tecnici stanno in un dettaglio chiuso (messaggio 46, D14)
  const c = v.nodo ? casellaDi(v.nodo) : v.decisione ? v.decisione.casella : null;
  const s3 = sezione("du-sez-provenienza", "Provenienza", false);
  if (c) {
    const tecnici = [riga("Allegato", h("span", { class: "du-mono" }, String(c.allegato_id || ""))), riga("Nodo nello STEP", h("span", { class: "du-mono" }, String(c.rif || ""))),
      riga("Unità di lettura", ip ? ip.unita_id || "" : ""), riga("Struttura", h("span", { class: "du-mono" }, String(c.gruppo || "")))];
    const imp = g.modo === "bozza" && S.bozza && S.bozza.importazione;
    if (imp) tecnici.push(riga("Importato il", dataOra(imp.creata_il)), riga("Impronta", h("span", { class: "du-mono" }, String(imp.impronta || "").slice(0, 16))),
      riga("Regole", h("span", { class: "du-mono" }, breve(JSON.stringify(imp.regole || (reg && reg.regole) || {}), 160))));
    s3.lastChild.append(h("dl", { class: "du-dl" }, riga("File", nomeFile(c.allegato_id))),
      h("details", { class: "du-ripiega", id: "du-dati-tecnici" }, h("summary", {}, "Dati tecnici (per l'assistenza)"), h("dl", { class: "du-dl" }, tecnici)));
  } else s3.lastChild.append(h("p", { class: "du-muto du-piccolo" }, "Pezzo senza casella dello STEP."));
  box.append(s3);
  return box;
}
// perchéNoConfermaIdentita: «Conferma l'identità proposta…» (MAT PN-08) vuole la lettura primaria della voce, il codice
// proposto, il registro della bozza e un nodo che ha ancora l'identità proposta (modo «proposta»); o "".
function perchéNoConfermaIdentita(v) {
  const voce = v.voce;
  const id = (v.nodo && v.nodo.identita) || {};
  if (!voce || !voce.identita_proposta || !voce.identita_proposta.chiave_lettura || !voce.codice_proposto) return "la voce non ha una lettura primaria con un codice proposto";
  if (!S.bozza || !S.bozza.importazione) return "la bozza non ha un registro d'importazione";
  if (id.modo === "decisione") return "l'identità del nodo è già decisa";
  if (id.modo !== "proposta") return "il nodo non ha l'identità proposta";
  return perchéNoModifica();
}
// apriConfermaIdentita: il rifiuto esplicito del P/N equivalente K4 (MAT PN-08; EM §3.6.1, «Il rifiuto di un
// equivalente K4»): un'alternativa sulla lettura primaria (dal messaggio 46 [U] senza motivo). Il nodo tiene l'identità proposta, ma come
// identità decisa, e l'avviso EQUIVALENTE_NON_ADOTTATO non scatta più. La copia della lettura e la chiave stanno già nel
// registro della bozza: la proiezione non serve, perché il codice proposto è la proiezione della primaria (EM §6.1).
function apriConfermaIdentita(v, r) {
  const voce = v.voce;
  const ip = voce.identita_proposta;
  const id = v.nodo.identita || {};
  const eq = r.equivalente || {};
  apriFinestra("Conferma l'identità proposta di " + (v.cod || "questo pezzo"), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'identità resta quella proposta (", h("span", { class: "du-mono" }, (id.codice || "") + (id.rev ? " rev " + id.rev : "")), ", " + (ip.emittente || "") + "), ma diventa un'identità decisa: il P/N equivalente ",
      h("span", { class: "du-mono" }, eq.base || "?"), " (" + (eq.emittente || "emittente non concorde") + ") non si adotta.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Conferma l'identità proposta", pri: true, id: "f-ok", fai: () => {
    const identita = { emittente: id.emittente, codice: id.codice };
    if (id.namespace) identita.namespace = id.namespace;
    if (id.rev) identita.rev = id.rev;
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "alternativa", importazione: S.bozza.importazione.impronta, chiave_lettura: ip.chiave_lettura,
      lettura: copiaLettura(ip), identita };
    fai("confermata l'identità proposta di " + (v.cod || "un pezzo"), [{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(id) }, dopo: { identita: Object.assign({}, identita, { modo: "decisione", decisione: dec.id }) }, origine: "utente" }]);
  } }]);
}
// copiaLettura: la copia compatta di una lettura del registro (EM §3.6.1, «lettura»), byte per byte, nella forma in cui
// la rilegge il registro (importazione.LetturaCompatta; R-7 della persistenza): {emittente, hash_grammatica, namespace,
// letterale, base: {originale, normalizzata}, qualificatori, revisione}, con i membri vuoti (null, "", [], {}) tolti a
// ogni livello, anche dentro qualificatori e revisione (SenzaVuoti), e qualificatori e revisione assenti se vuoti. Il
// server confronta le due copie per byte canonici (V-1): i valori non si toccano, mai NFC.
function senzaVuoti(v) {
  if (Array.isArray(v)) return v.map(senzaVuoti);
  if (!ogg(v)) return v;
  const o = {};
  for (const k of Object.keys(v)) {
    const x = senzaVuoti(v[k]);
    if (x === null || x === undefined || x === "" || (Array.isArray(x) && !x.length) || (ogg(x) && !Object.keys(x).length)) continue;
    o[k] = x;
  }
  return o;
}
function copiaLettura(l) {
  const c = { emittente: l.emittente, letterale: l.letterale };
  if (typeof l.hash_grammatica === "string" && l.hash_grammatica) c.hash_grammatica = l.hash_grammatica;
  if (typeof l.namespace === "string" && l.namespace) c.namespace = l.namespace;
  const base = {};
  if (l.base && typeof l.base.originale === "string" && l.base.originale) base.originale = l.base.originale;
  if (l.base && typeof l.base.normalizzata === "string" && l.base.normalizzata) base.normalizzata = l.base.normalizzata;
  c.base = base;
  const q = senzaVuoti(l.qualificatori);
  if (ogg(q) && Object.keys(q).length) c.qualificatori = q;
  const r = senzaVuoti(l.revisione);
  if (ogg(r) && Object.keys(r).length) c.revisione = r;
  return c;
}
// letturaDaAdottare: la lettura di un riferimento che «Adotta come codice…» può citare, con il codice e la rev che la
// decisione porta (MAT PN-08): per una lettura con ruolo (riferimenti[].lettura) è lei; per un P/N equivalente K4 è la
// lettura del codice della parte nel PDF della prova (<allegato della prova>/<lettura_codice>, in file[].letture[] del
// registro). Il codice è la proiezione che il registro dà nella lettura stessa (codice_proposto, e rev_proposta se c'è:
// decisioni dell'ondata A, PE-08), mai il letterale né un campo del riferimento. Senza lettura o senza proiezione il gesto
// è spento con il motivo: per il P/N K4 la proiezione in file[].letture[] è RPC PD-07, e finché il registro non la dà il
// gesto resta spento.
function letturaDaAdottare(r, reg) {
  let l = r && r.lettura ? r.lettura : null;
  if (!l && r && r.equivalente) {
    for (const pr of arr(r.equivalente.prove)) {
      const k = (pr && pr.allegato_id) + "/" + (pr && pr.lettura_codice);
      for (const f of arr(reg && reg.proposta && reg.proposta.file)) {
        const x = arr(f.letture).find((y) => y && y.chiave_lettura === k);
        if (x) {
          l = x;
          break;
        }
      }
      if (l) break;
    }
    if (!l) return { motivo: "la lettura del codice della parte nel PDF della prova non è nel registro della bozza" };
  }
  if (!l || !l.chiave_lettura) return { motivo: "il riferimento non porta una lettura del registro" };
  const codice = typeof l.codice_proposto === "string" ? l.codice_proposto : "";
  if (!codice) return { motivo: r.equivalente ? "il registro non dà la proiezione del codice per la lettura del PDF della prova (codice_proposto in file[].letture[], RPC PD-07)" :
    "il registro non dà la proiezione del codice per questa lettura (codice_proposto, PE-08)" };
  return { lettura: l, codice, rev: typeof l.rev_proposta === "string" ? l.rev_proposta : "", motivo: "" };
}
// documentiDelNodo: la linguetta «Documenti del nodo» (MAT PN-02, §6.3): i file del nodo con il loro stato (proposto,
// preassociato con l'esito di R4, associato, da scollegare, confermato, da ricodificare), «Preassocia» e «Carica…»
// (EL-08), le azioni sui documenti che V1 non ha (MAT §6.1), i fabbisogni e le diagnostiche del registro.
function documentiDelNodo(g, v) {
  const s = h("div", { class: "du-sez", id: "du-sez-documenti" }, h("h3", {}, "Documenti del nodo"));
  const righe = [];
  const apri = (f) => (f && f.allegato_id && f.stato !== "scartato" ? bottoneApri(f, "du-doc-apri-" + sid(f.allegato_id)) : null);
  if (g.modo === "bozza") {
    const doc = documentiNodo(v);
    const ricod = v.nodo && v.nodo.componente_id && arr(S.lavoro && S.lavoro.decisioni).some((d) => d.tipo === "ricodifica" && d.componente_id === v.nodo.componente_id);
    for (const aid of doc.pb) {
      const es = esitoPreassociazione(aid);
      righe.push(h("li", { "data-du-doc": "preassociato" }, pill("bozza", "◈ preassociato"), " ", h("span", { class: "du-mono" }, nomeFile(aid)), es ? pill(es === "applicabile" ? "ok" : "ver", es) : null, apri(fileDi(aid))));
    }
    for (const pa of doc.pa) righe.push(h("li", { "data-du-doc": "proposto" }, pill("prop", "◇ proposto"), " ", h("span", { class: "du-mono" }, nomeFile(pa.allegato_id)), pa.motivo ? " · " + pa.motivo : "", apri(fileDi(pa.allegato_id))));
    for (const f of doc.ar) {
      const sv0 = svincoloDi(f.allegato_id);
      righe.push(h("li", { "data-du-doc": f.documento ? "confermato" : sv0 ? "da_scollegare" : "associato" }, pill(f.documento ? "ok" : sv0 ? "ver" : "info", f.documento ? "✓ confermato" : sv0 ? "⊘ da scollegare" : "● associato"), " ",
        h("span", { class: "du-mono" }, f.nome_file), ricod ? pill("ver", "da ricodificare") : null, apri(f)));
    }
    for (const x of doc.nonRichieste) righe.push(h("li", { "data-du-doc": "non_richiesta" }, pill("neu", "◇ non richiesta: nodo non incluso (bozza)"), " ", h("span", { class: "du-mono" }, nomeFile(x.allegato_id))));
  } else if (g.modo === "working") {
    // la BOM confermata (messaggio 47): le associazioni registrate del componente, in sola lettura
    for (const f of documentiWorking(v).ar) {
      righe.push(h("li", { "data-du-doc": f.documento ? "confermato" : "associato" }, pill(f.documento ? "ok" : "info", f.documento ? "✓ confermato" : "● associato"), " ",
        h("span", { class: "du-mono" }, f.nome_file), apri(f)));
    }
  } else {
    for (const a of arr(v.voce && v.voce.associazioni_proposte)) righe.push(h("li", { "data-du-doc": "proposto" }, pill("prop", "◇ proposto"), " ", h("span", { class: "du-mono" }, nomeFile(a.allegato_id)), a.motivo ? " · " + a.motivo : ""));
  }
  s.append(righe.length ? h("ul", { class: "du-elenco-p", id: "du-doc-nodo" }, righe) : h("p", { class: "du-muto du-piccolo", id: "du-doc-nodo" }, v.tipo === "commerciale" ? "2D non richiesto: nessun documento." : "Nessun documento."));
  if (g.modo === "bozza" && v.nodo && !v.noninc) s.append(sezioneDocumenti(g, v));
  // fabbisogni e diagnostiche: quelli del registro della bozza sono dell'importazione (EM §3.8), e la linguetta lo dice;
  // la completezza e le diagnostiche di adesso sono i campi per nodo di /dati (gruppi[].nodi[]: completezza, diagnostiche;
  // EM §9.9, PE-28; MAT PN-06, PN-07), con le diagnostiche del gruppo senza nodo accanto (gruppi[].diagnostiche, B-2)
  const voce = v.voce;
  const fab = arr(voce && voce.fabbisogni_previsti_all_importazione);
  const dia = arr(voce && voce.diagnostiche);
  const nm = nodoDelModello(voce && voce.voce);
  const comp = nm && ogg(nm.completezza) ? nm.completezza : null;
  const diaOggi = arr(nm && nm.diagnostiche);
  const gr = voce && S.modello ? S.modello.gruppi.find((x) => arr(x.nodi).some((n) => n && n.voce === voce.voce)) : null;
  const diaGruppo = arr(gr && gr.diagnostiche);
  // messaggio 46 (D14, rilievo 6 della revisione ALBERO [T]): le diagnostiche con il loro messaggio, il tipo di documento
  // e l'esito in parole; i codici del motore non si scrivono nella linguetta
  const testoDia = (l) => l.map((x) => x.messaggio || x.testo || parola(x.codice)).join("; ");
  const tipoDoc = (t) => TIPI_DOCUMENTO_NODO[t] || parola(t);
  // collaudo UI46, giro 3 (F3-I2) [T]: i messaggi delle diagnostiche citano selettori e regole del motore
  // («radice_step.nome», «rev-campo»): stanno in «Dettagli tecnici», chiusi; in vista resta solo quante sono
  const nDia = diaOggi.length + diaGruppo.length + dia.length;
  const vociDia = (t, l) => (l.length ? h("li", {}, h("b", {}, t + ": "), testoDia(l)) : null);
  s.append(h("dl", { class: "du-dl", id: "du-completezza" },
    riga("2D previsti all'importazione", fab.length ? fab.map((x) => (x.tipo_documento ? tipoDoc(x.tipo_documento) : "documento") + (x.bloccante ? " (bloccante)" : "")).join(", ") : voce ? "nessuno" : ""),
    riga("Completezza del 2D di adesso", comp ? (comp.requisito ? tipoDoc(comp.requisito) : "?") + " · " + (comp.esito ? parola(comp.esito) : "?") + (comp.deroga ? " · deroga" + (comp.deroga.stato ? " " + parola(comp.deroga.stato) : "") : "") :
      voce ? "il modello non dà la completezza di questo nodo" : ""),
    riga("Diagnostiche del Motore A", nDia ? nDia + (nDia === 1 ? " segnalazione del motore, nei «Dettagli tecnici» qui sotto" : " segnalazioni del motore, nei «Dettagli tecnici» qui sotto") : nm || voce ? "nessuna" : "")));
  if (nDia) {
    s.append(h("details", { class: "du-ripiega", id: "du-diagnostiche-tecniche" }, h("summary", {}, "Dettagli tecnici del Motore A (" + nDia + ")"),
      h("ul", { class: "du-elenco-p du-piccolo", style: "overflow-wrap:anywhere" }, vociDia("Adesso", diaOggi), vociDia("Della struttura (senza nodo)", diaGruppo), vociDia("All'importazione", dia))));
  }
  // le azioni sui documenti che V1 non ha (MAT §6.1, EL-09): visibili e spente, con il motivo accanto (D23:294)
  // messaggio 46 (D40): le azioni sui documenti che questa versione non ha stanno in un dettaglio chiuso, non in vista
  s.append(h("details", { class: "du-ripiega" }, h("summary", {}, "Azioni sui documenti non ancora disponibili"), h("div", { class: "du-azioni du-non-v1", id: "du-doc-non-v1" },
    btn("Conferma il documento", null, { cls: "du-btn--sm", no: NON_V1.documento }),
    btn("Conferma 2D", null, { cls: "du-btn--sm", no: NON_V1.documento }),
    btn("Correggi il codice di un file…", null, { cls: "du-btn--sm", no: NON_V1.r10 }),
    btn("Deroga", null, { cls: "du-btn--sm", no: NON_V1.deroga }),
    btn("Revoca la deroga", null, { cls: "du-btn--sm", no: NON_V1.deroga }),
    btn("Sostituisci il principale con questo", null, { cls: "du-btn--sm", no: NON_V1.sostituisci }))));
  return s;
}
const TIPI_DOCUMENTO_NODO = { disegno_2d: "2D", cad_3d: "3D", sviluppo_dxf: "sviluppo DXF", distinta_cliente: "distinta del cliente" };
// chiE: chi ha fatto un gesto, detto all'utente. Il server vero dà l'autore come identificativo dell'utente (R1
// EventoR1.Autore), il banco come sigla: l'identificativo dell'utente che guarda diventa la sua sigla, quello di un altro
// utente «un altro utente», mai l'identificativo a schermo (revisioni delle corsie MOTIVO e SORGENTE del messaggio 46 [T]).
function chiE(autore) {
  const a = String(autore || "").trim();
  if (!a) return "";
  if (S.utente && a.toLowerCase() === String(S.utente).toLowerCase()) return S.sigla || "tu";
  return RE_UUID.test(a) ? "un altro utente" : a;
}
function autoreDi(chiave) {
  const ev = arr(S.bozza && S.bozza.eventi).filter((e) => e.chiave === chiave || (e.dopo && e.dopo.id === chiave));
  const e = ev[ev.length - 1];
  return e ? (chiE(e.autore) || "?") + " alle " + ora(e.quando) : "";
}
// decisoDa: chi ha preso la decisione e quando, dagli eventi della bozza; è quello che pannello e wizard mostrano al posto del
// motivo (messaggio 46 [U]). Un evento appena salvato da questa scheda non ha ancora l'autore (lo dà la rilettura di R1):
// «deciso adesso». "" se la bozza non ha l'evento (per esempio dopo la conferma).
function decisoDa(chiave) {
  const ev = arr(S.bozza && S.bozza.eventi).filter((e) => e.chiave === chiave || (e.dopo && e.dopo.id === chiave));
  const e = ev[ev.length - 1];
  if (!e) return "";
  return e.autore ? "deciso da " + chiE(e.autore) + (dataOra(e.quando) ? " il " + dataOra(e.quando) : "") : "deciso adesso, in bozza";
}
// testoSegnale: un segnale del motore (LS-02) con le parole della pagina, non con il codice
const TESTI_SEGNALE = { "ancoraggio.possibile_elemento_cad": "possibile elemento CAD" };
function testoSegnale(codice) {
  return TESTI_SEGNALE[codice] || codice || "";
}
// L'identificativo del cliente (U2 §4.2 punto 4; C-01 [U]): una stringa calcolata dal server alla lettura con la regola
// della grammatica del cliente, mai salvata e mai composta dalla pagina. Per un nodo della bozza la dà il riepilogo
// (R4 identita[].decisa.identificativo_cliente, quello in sottofondo o quello del wizard); nella Sorgente la tabella di
// S9. Senza regola il server lo dice vuoto: «composizione non definita nella grammatica», e codice e revisione restano
// due campi. Mai una concatenazione di ripiego ([U] D31 §8).
const TITOLO_IDC = "identificativo verso il cliente, composto con la regola della sua grammatica; codice e revisione restano due campi";
// testoIdc: il testo dell'identificativo verso il cliente come lo dà il server, {testo, motivo}
// (propostabom.IdentificativoCliente: /dati, S9, R1, R4, R7), o una stringa (la forma del banco di prima); "" se non c'è.
// Mai l'oggetto scritto com'è («[object Object]», messaggio 46, difetto D12)
function testoIdc(v) {
  if (typeof v === "string") return v === "non determinato" ? "" : v;
  return ogg(v) && typeof v.testo === "string" ? v.testo : "";
}
function idClienteDa(valore, presente) {
  if (!presente) return null;
  valore = testoIdc(valore);
  if (valore) return h("span", { class: "du-idc", title: TITOLO_IDC, "data-du-idc": valore }, "id. cliente: ", h("span", { class: "du-mono" }, valore));
  // senza una regola di composizione nella grammatica non c'è niente da dire a chi guarda (collaudo UI46, minore)
  return h("span", { class: "du-idc du-muto", title: TITOLO_IDC, "data-du-idc": "", hidden: true }, "");
}
function idClienteNodo(k, rip) {
  const r = rip || S.riepilogo;
  const x = r && arr(r.identita).find((y) => y.chiave_bozza === k);
  const d = x && x.decisa;
  return idClienteDa(d && d.identificativo_cliente, !!(d && Object.prototype.hasOwnProperty.call(d, "identificativo_cliente")));
}
// sezionePosizione: i padri del pezzo con la quantità del legame e il suo stato, e quanti pezzi ha sotto
function sezionePosizione(g, v) {
  const nome = (k) => (g.nodi[k] && (g.nodi[k].cod || g.nodi[k].desc)) || "(senza codice)";
  const statoLegame = (a) => (g.modo === "proposta" ? "dal file" : a.kind === "conf" ? "confermato" : a.kind === "bozza" ? "deciso nella bozza" : "dal file, da confermare");
  const pa = padriA(g, v.k);
  const fi = figliA(g, v.k);
  return sezione("du-sez-posizione", "Posizione", true, h("dl", { class: "du-dl" },
    riga("Sotto", v.k === g.radice ? "è il prodotto della RFQ" : pa.length ? h("span", {}, pa.map((a, i) => [i ? "; " : "", h("span", { class: "du-mono" }, nome(a.p)), " ×" + a.qta + " (" + statoLegame(a) + ")"])) : "nessun padre: da ricollocare"),
    riga("Pezzi sotto", fi.length ? String(fi.length) : "nessuno")));
}
const MODI_IDENTITA = { proposta: "proposta dal file", decisione: "decisa nella bozza", registrato: "registrata", da_decidere: "da decidere" };
function sezioneDecisione(g, v) {
  if (v.noninc) {
    const d = v.decisione;
    // messaggio 46 [U]: al posto del motivo chi ha deciso e quando; il segnale del motore con le parole della pagina
    return sezione("du-sez-decisione", "Decisione dell'ingegnere", true, h("dl", { class: "du-dl" }, riga("Decisione", "non includere nella BOM (bozza)"),
      riga("Chi e quando", h("span", { id: "du-noninc-deciso" }, decisoDa(d.id) || "in bozza")), riga("Segnale visto", d.segnale_visto ? testoSegnale(d.segnale_visto.codice) : "nessuno")));
  }
  const id = v.nodo.identita || {};
  const dec = id.decisione ? S.lavoro.decisioni.find((x) => x.id === id.decisione) : null;
  // l'identificativo verso il cliente c'è solo quando il riepilogo lo dà: mai «da aggiornare» (messaggio 46, D16)
  const idc = idClienteNodo(v.k);
  return sezione("du-sez-decisione", "Decisione dell'ingegnere", true, h("dl", { class: "du-dl" }, riga("Identità decisa", h("span", { class: "du-mono" }, (id.codice || "(senza codice)") + (id.rev ? " rev " + id.rev : ""))),
    idc ? riga("Id. cliente", idc) : null,
    // il namespace della grammatica è un identificativo: non si mostra (collaudo UI46)
    riga("Emittente", emittenteParola(id)), riga("Stato", MODI_IDENTITA[id.modo] || parola(id.modo)),
    dec ? riga("Decisione", parola(dec.tipo) + (dec.modo ? " (" + parola(dec.modo) + ")" : "")) : null,
    dec ? riga("Chi e quando", autoreDi(dec.id) || "non ancora salvata") : null,
    v.escluso ? riga("Esclusione", "escluso alla conferma (archiviato, mai cancellato se ha storia)") : null,
    segnaliDi(v.voce).length ? riga("Segnale del motore", segnaliDi(v.voce).map((x) => x.spiegazione || parola(x.codice)).join(" ")) : null));
}
// vociGesti: i gesti del pezzo sulla bozza, per il pannello e per il menu del nodo (messaggio 46, D38). Solo quelli che
// valgono per il pezzo (D40: niente «Aggiungi sotto» su un particolare, niente «Ricolloca i figli» senza figli, niente
// «Cambia tipo» sul prodotto); spenti, con il motivo, quelli che oggi non si fanno (la bozza in conflitto, la conferma
// in sospeso). {id, testo, fai, no, cls}.
function vociGesti(g, v) {
  const no = perchéNoModifica();
  if (v.noninc) return [{ id: "du-includi", testo: "Includi di nuovo", fai: () => includiDiNuovo(v), no: no || (voceNonImportabile(v.voce) ? MOTIVO_VOCE_REGISTRATA : "") }];
  const radice = v.k === g.radice;
  const conCasella = !!casellaDi(v.nodo);
  const registrato = !!v.nodo.componente_id;
  const padri = padriA(g, v.k).length > 0;
  const out = [{ id: "du-correggi", testo: "✎ Correggi codice, rev e descrizione…", fai: () => apriCorreggi(v), no }];
  if (!radice) out.push({ id: "du-cambia-tipo", testo: "◆ Cambia tipo…", fai: () => apriTipo(v), no });
  if (contenitore(v)) out.push({ id: "du-aggiungi-sotto", testo: "+ Aggiungi un pezzo sotto…", fai: () => apriAggiungi("sciolto", v.k), no });
  if (!radice) out.push({ id: "du-sposta", testo: "↳ Sposta sotto…", fai: () => apriSposta(v.k, ""), no }, { id: "du-collega", testo: "+ Collega anche sotto…", fai: () => apriCollega(v.k), no });
  if (padri) out.push({ id: "du-scollega-padre", testo: "− Scollega da un padre…", fai: () => apriScollegaPadre(v.k), no }, { id: "du-quantita", testo: "× Quantità…", fai: () => apriQuantita(v.k, ""), no });
  if (figliA(g, v.k).length) out.push({ id: "du-ricolloca", testo: "⤴ Ricolloca i figli…", fai: () => apriRicollocaFigli(v.k), no });
  if (v.escluso) out.push({ id: "du-ripristina", testo: "↺ Ripristina", fai: () => ripristina(v), no });
  else if (registrato && !radice) out.push({ id: "du-escludi", testo: "Escludi (senza cascata)…", fai: () => apriEscludi(v), no, cls: "du-btn--danger" });
  if (!radice && conCasella && !registrato) out.push({ id: "du-non-includere-nodo", testo: "Non includere nella BOM…", fai: () => {
    S.selezione = new Set([v.k]);
    apriNonIncludere(g);
  }, no, cls: "du-btn--danger" });
  if (!radice && !conCasella && !registrato) out.push({ id: "du-togli", testo: "Non crearlo (togli dalla bozza)…", fai: () => togliNodo(v), no, cls: "du-btn--danger" });
  if (registrato && v.nodo.identita && v.nodo.identita.emittente_presunto) out.push({ id: "du-fissa-emittente", testo: "Fissa l'emittente…", fai: () => apriFissaEmittente(v), no });
  return out;
}
function gestiNodo(g, v) {
  const s = sezione("du-gesti", "Gesti sulla bozza", true);
  s.classList.add("du-gesti");
  const c = s.lastChild;
  for (const o of vociGesti(g, v)) c.append(btn(o.testo, o.fai, { cls: "du-btn--sm" + (o.cls ? " " + o.cls : ""), id: o.id, no: o.no }));
  if (!v.noninc) {
    const riuso = riusoNelPannello(g, v);
    if (riuso) c.append(riuso);
  }
  return s;
}

// «⤴ Ricolloca i figli…» (MAT BZ-11): tutti i legami dei figli verso un altro padre, in un solo salvataggio; la quantità
// si scrive, oppure si sceglie esplicitamente di usare quelle di prima.
function apriRicollocaFigli(k) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const figli = figliA(g, k);
  const verso = Object.values(g.nodi).filter((x) => !x.noninc && !x.escluso && contenitore(x) && x.k !== k && figli.every((a) => !vietatoSotto(a.f, x.k)));
  const sel = h("select", { id: "f-a" }, h("option", { value: "" }, "— scegli il padre nuovo —"), verso.map((x) => h("option", { value: x.k }, (x.cod || "(senza codice)") + " · " + (x.desc || ""))));
  const righe = figli.map((a) => campo((g.nodi[a.f].cod || g.nodi[a.f].desc || a.f) + " (ora ×" + a.qta + ")", h("input", { "data-f": a.f, type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric", class: "f-qta-figlio" })));
  apriFinestra("Ricolloca i figli di " + (v.cod || "il pezzo"), h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, figli.length + " legami vanno sotto il padre nuovo, in un solo salvataggio. Nessuna quantità implicita: scrivila per ogni figlio, oppure scegli di usare quelle di prima."),
    campo("Padre nuovo", sel), h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-stesse-qta" }), "Usa le quantità di prima"), h("div", { id: "f-qta-figli", style: "display:grid;gap:6px" }, righe)),
  [{ testo: "Annulla", chiudi: true }, { testo: "Ricolloca", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    if (!valore(f, "f-a")) return "scegli il padre nuovo";
    if (f.querySelector("#f-stesse-qta").checked) return "";
    return [...f.querySelectorAll(".f-qta-figlio")].every((x) => qtaValida(String(x.value || "").trim())) ? "" : "scrivi la quantità di ogni figlio, o usa quelle di prima";
  }, fai: () => {
    const f = $("#du-finestra");
    const a2 = valore(f, "f-a");
    const stesse = f.querySelector("#f-stesse-qta").checked;
    const qta = {};
    for (const x of f.querySelectorAll(".f-qta-figlio")) qta[x.dataset.f] = qtaValida(String(x.value || "").trim());
    const azioni = [];
    for (const a of figli) {
      const vecchio = S.lavoro.archi.find((y) => y.padre === k && y.figlio === a.f);
      if (!vecchio) continue;
      azioni.push({ azione: "arco_spostato", chiave_bozza: a.f, prima: clona(vecchio), dopo: { padre: a2, figlio: a.f, qta: stesse ? vecchio.qta : qta[a.f], origine: "utente" }, origine: "utente" });
    }
    if (!azioni.length) return false;
    fai("ricollocati " + azioni.length + " figli di " + (v.cod || "un pezzo"), azioni);
  } }]);
}
// La sezione «Documenti» del pannello (MAT EL-08): i file da smistare (attivi, non STEP, non preassociati né associati)
// da preassociare a questo pezzo, e «Carica…», che porta al caricamento della Sorgente: caricare non preassocia.
function sezioneDocumenti(g, v) {
  if (v.noninc || !v.nodo) return null;
  if (v.escluso) return h("p", { class: "du-nota", id: "du-doc-preassocia" }, "Il pezzo è escluso: non riceve preassociazioni. Con «↺ Ripristina» torna nella BOM, e i file andati in «Da ricollocare» tornano con lui.");
  const no = perchéNoModifica();
  // i file da smistare: preassociabili (natura e stato, anche senza proposta di documento: direttiva 35, C-2; gli STEP non
  // sorgente: D-2 (c)), non preassociati nella bozza e senza un'associazione registrata (le righe di S9)
  const giaPb = new Set(S.lavoro.documenti_preassociati.map((d) => d.allegato_id));
  const ar = associazioniRegistrate();
  const liberi = fileSorgente().filter((f) => preassociabile(f) && !giaPb.has(f.allegato_id) && !ar.has(String(f.allegato_id).toLowerCase()));
  const sel = h("select", { id: "du-pannello-file", "aria-label": "File da smistare" }, h("option", { value: "" }, "— scegli fra i file da smistare —"), liberi.map((f) => h("option", { value: f.allegato_id }, f.nome_file)));
  return h("div", { id: "du-doc-preassocia", style: "display:grid;gap:6px" }, h("b", { class: "du-piccolo" }, "Preassocia un file da smistare"),
    h("div", { class: "du-azioni" }, sel,
      btn("Preassocia", () => {
        const aid = $("#du-pannello-file") && $("#du-pannello-file").value;
        if (!aid) return avvisa("no", "Scegli prima un file da smistare.");
        preassocia(aid, v.k);
      }, { cls: "du-btn--sm", id: "du-pannello-preassocia", no: no || (liberi.length ? "" : "nessun file da smistare"), perche: false }),
      btn("Carica…", () => {
        S.passo = "sorgente";
        SG.cartella = "tutti";
        SG.caricaAperto = true;
        render();
        const c = $("#du-carica-file");
        if (c) c.focus();
      }, { cls: "du-btn--sm du-btn--ghost", id: "du-pannello-carica", perche: false })),
    h("p", { class: "du-muto du-piccolo" }, "Il caricamento non preassocia: preassociare è un gesto dopo."));
}

// ---------------------------------------------------------------- finestre

// UI46 (controllo di stabilità del 21/10 [T]): alla chiusura il fuoco torna a chi ha aperto il dialogo senza far
// scorrere la pagina (preventScroll), e lo si ritrova per id se un ridisegno l'ha sostituito. Un dialogo aperto
// trascinando una scheda o dal menu del nodo non ha un pulsante di partenza (il fuoco era rimasto altrove, anche lontano):
// torna sulla scheda del pezzo (#oc-‹k›), che conRitorno indica.
let fuocoPrima = null;
let fuocoPrimaId = "";
let ritornoFinestra = "";
function conRitorno(id, fai) {
  ritornoFinestra = id;
  let r;
  try {
    r = fai();
  } finally {
    if (r && typeof r.then === "function") r.then(() => (ritornoFinestra = ""), () => (ritornoFinestra = ""));
    else ritornoFinestra = "";
  }
  return r;
}
function apriFinestra(titolo, corpo, azioni, o) {
  const f = $("#du-finestra");
  const velo = $("#du-velo");
  const rit = ritornoFinestra && document.getElementById(ritornoFinestra);
  fuocoPrima = rit || document.activeElement;
  fuocoPrimaId = rit ? ritornoFinestra : fuocoPrima && fuocoPrima.id ? fuocoPrima.id : "";
  ritornoFinestra = "";
  const err = h("p", { class: "du-err", id: "du-finestra-errore", role: "alert" });
  const piede = h("div", { class: "du-azioni" });
  const bottoni = [];
  for (const a of azioni) {
    const b = h("button", { type: "button", class: "du-btn" + (a.pri ? " du-btn--pri" : "") + (a.danger ? " du-btn--danger" : ""), id: a.id || null }, a.testo);
    b.addEventListener("click", async () => {
      if (b.disabled) return;
      if (a.chiudi) return chiudiFinestra();
      err.textContent = "";
      b.disabled = true;
      try {
        const r = await a.fai(err);
        if (r !== false) chiudiFinestra();
      } finally {
        if (b.isConnected) b.disabled = a.spento ? !!a.spento() : false;
      }
    });
    bottoni.push({ b, a });
    piede.append(b);
  }
  const aggiornaBottoni = () => bottoni.forEach(({ b, a }) => {
    if (a.spento) {
      const motivo = a.spento();
      b.disabled = !!motivo;
      b.title = motivo || "";
    }
  });
  f.replaceChildren(h("div", { class: "du-fin-t" }, h("h2", { id: "du-finestra-titolo" }, titolo), h("button", { type: "button", class: "du-btn du-btn--sm du-btn--ghost", "aria-label": "Chiudi", onclick: chiudiFinestra }, "✕")),
    corpo, err, piede);
  f.hidden = false;
  velo.hidden = false;
  // il contenitore della finestra resta lo stesso fra un dialogo e l'altro: l'ascoltatore del dialogo di prima va tolto,
  // o i suoi controlli leggerebbero campi che non ci sono più
  staccaAggiorna(f);
  f.__aggiorna = aggiornaBottoni;
  f.addEventListener("input", aggiornaBottoni);
  f.addEventListener("change", aggiornaBottoni);
  aggiornaBottoni();
  const primo = f.querySelector("textarea, input, select") || bottoni.length && bottoni[bottoni.length - 1].b;
  // il fuoco entra nel dialogo senza far scorrere la pagina sotto (collaudo UI46, giro 2: all'apertura di «Sposta» la
  // pagina scorreva di circa 40 px)
  if (primo && primo.focus) primo.focus({ preventScroll: true });
  if (o && o.dopo) o.dopo(f);
  return { f, err, aggiorna: aggiornaBottoni };
}
function staccaAggiorna(f) {
  if (!f.__aggiorna) return;
  f.removeEventListener("input", f.__aggiorna);
  f.removeEventListener("change", f.__aggiorna);
  f.__aggiorna = null;
}
function chiudiFinestra() {
  const f = $("#du-finestra");
  if (!f || f.hidden) return;
  staccaAggiorna(f);
  f.hidden = true;
  f.replaceChildren();
  if (!S.wizard) $("#du-velo").hidden = true;
  const torna = fuocoPrima && document.body.contains(fuocoPrima) ? fuocoPrima : fuocoPrimaId ? document.getElementById(fuocoPrimaId) : null;
  if (torna && torna.focus) torna.focus({ preventScroll: true });
}
function campo(etichetta, input, aiuto) {
  return h("label", { class: "du-campo" }, h("span", {}, etichetta), input, aiuto ? h("small", { class: "du-muto" }, aiuto) : null);
}
function valore(f, id) {
  const el = f.querySelector("#" + id);
  return el ? String(el.value || "").trim() : "";
}
function qtaValida(v) {
  const q = Number(v);
  return v !== "" && Number.isInteger(q) && q >= 1 && q <= 100000 ? q : null;
}
// Le regole di forma di codici, rev e descrizioni che la pagina scrive nei nodi e nelle decisioni (EM §3.4, §3.4.2;
// classificazione.CodiceAmmissibile e RevAmmissibile; P-15 e P-18 della persistenza, accolte nelle decisioni
// dell'ondata A): la pagina le controlla prima del salvataggio, perché un gesto non finisca in un 422; il server le
// ricontrolla. Ognuna torna il motivo, o "".
const RE_SPAZIO_O_CONTROLLO = /[\s\p{Cc}\p{Cf}]/u;
function byteUtf8(s) {
  return new TextEncoder().encode(String(s)).length;
}
// un codice nuovo (crea, o il codice dopo una ricodifica): non vuoto, al più 40 byte, senza spazi né controlli
function perchéNoCodiceNuovo(c) {
  if (!c) return "il codice è vuoto";
  if (byteUtf8(c) > 40) return "il codice supera i 40 byte";
  if (RE_SPAZIO_O_CONTROLLO.test(c)) return "il codice ha spazi o caratteri di controllo";
  return "";
}
// una rev: al più 10 byte, senza spazi né controlli (vuota = assente, PE-11)
function perchéNoRev(r) {
  if (!r) return "";
  if (byteUtf8(r) > 10) return "la rev supera i 10 byte";
  if (RE_SPAZIO_O_CONTROLLO.test(r)) return "la rev ha spazi o caratteri di controllo";
  return "";
}
function perchéNoDescrizione(d) {
  if ([...String(d || "")].length > 200) return "la descrizione supera i 200 caratteri";
  if (/\p{Cc}/u.test(String(d || ""))) return "la descrizione ha caratteri di controllo";
  return "";
}
// minuscoleAscii: una rev nuova (che introduce l'ingegnere) non ha minuscole ASCII; una rev copiata dalla working per un
// riusa o un esclude passa byte per byte (P-18)
function minuscoleAscii(r) {
  return /[a-z]/.test(String(r || ""));
}
// revMinuscoleAmmesse: la rev del nodo come sarà DOPO il gesto può avere minuscole ASCII solo se resta copiata byte per
// byte da una fonte che le ammette (P-18 della persistenza, accolta nelle decisioni dell'ondata A; è la regola di
// revDallaFonte del server): la working per un nodo riusa o esclude, la bozza formato 2 per un nodo formato_2, il codice
// corretto a mano nella pagina vecchia (decisione identita con riferimento manuale_legacy). Una decisione di correzione,
// una ricodifica, un crea o una modifica fanno la rev nuova: senza minuscole. op e identita sono quelli del nodo dopo il
// gesto; un'identità decisa da una decisione nuova si passa con decisione "" (non è mai manuale_legacy).
function revMinuscoleAmmesse(op, identita) {
  if (op === "riusa" || op === "esclude") return true;
  const id = identita || {};
  if (id.modo === "formato_2") return true;
  if (id.modo !== "decisione" || !id.decisione) return false;
  const d = arr(S.lavoro && S.lavoro.decisioni).find((x) => x.id === id.decisione);
  return !!(d && d.tipo === "identita" && d.riferimento === "manuale_legacy");
}
// citaIdentita: l'identità del nodo è decisa da una decisione identita della bozza (modo decisione).
function citaIdentita(identita) {
  const id = identita || {};
  return id.modo === "decisione" && !!id.decisione && arr(S.lavoro && S.lavoro.decisioni).some((d) => d.id === id.decisione && d.tipo === "identita");
}
// togliIdentitaPrecedenti: le decisioni identita che il nodo k ha già nella bozza, da togliere nello stesso gesto che ne
// aggiunge una nuova (decisione_tolta prima della decisione_aggiunta). Una decisione identita è del nodo che la cita
// (EM §3.6.1), e il server rifiuta quella che il nodo non cita più (validaDecisioni): una sola per nodo, come la regola
// di PE-B06 per cambia_tipo (proposta PE-B16 della corsia).
function togliIdentitaPrecedenti(k) {
  return arr(S.lavoro && S.lavoro.decisioni).filter((d) => d.tipo === "identita" && d.chiave_bozza === k)
    .map((d) => ({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" }));
}
// «Riepilogo RFQ» (MAT TS-08): in lettura, dal modello della pagina e dai contatori della Sorgente.
function apriRiepilogoRfq() {
  const d = $("#du-menu-rfq");
  if (d) d.open = false;
  const m = S.modello;
  const t = (m && m.testa) || {};
  const c = (S.sorgente && S.sorgente.contatori) || {};
  const prodotti = m ? m.prodotti : [];
  apriFinestra("Riepilogo RFQ", h("div", { style: "display:grid;gap:8px" },
    h("dl", { class: "du-dl", id: "f-riepilogo-rfq" }, riga("Cliente", t.cliente || ""), riga("Oggetto", t.oggetto || ""), riga("Riferimento", t.riferimento_cliente || ""), riga("Stato", t.stato_thread || ""),
      riga("File", c.attivi !== undefined ? c.attivi + " attivi, " + (c.altri_file || 0) + " in Altri file, " + (c.scartati || 0) + " scartati, " + (c.in_analisi || 0) + " in analisi" : ""),
      riga("Rismistamento F8", (m && m.f8) || "")),
    h("ul", { class: "du-elenco-p", id: "f-riepilogo-prodotti" }, prodotti.map((x) => h("li", {}, h("span", { class: "du-mono" }, x.codice || x.rif), x.descrizione ? " · " + x.descrizione : "",
      " · Verificata: " + (x.verificatoOggi ? "sì" : "no") + (motiviDelProdotto(x).some((mm) => mm.frase) ? " (" + motiviDelProdotto(x).filter((mm) => mm.frase).map((mm) => mm.frase).join("; ") + ")" : ""))))),
  [{ testo: "Chiudi", chiudi: true }]);
}
function apriLimiti() {
  const voci = [
    "La working la scrive solo «Conferma BOM»: dalla pagina nuova nessun gesto strutturale è immediato.",
    "«Conferma questo STEP come sorgente» conferma la fonte della bozza, non la struttura né la BOM: lo STEP della working cambia solo con «Conferma BOM». «Revoca come sorgente» corregge uno STEP scelto per errore e blocca la conferma della BOM finché una sorgente non è confermata di nuovo.",
    "Una bozza senza STEP («Parti dal solo prodotto») si modifica, si salva e si rivede; «Conferma BOM» vuole uno STEP confermato come sorgente (STEP_SORGENTE_NON_CONFERMATA).",
    "Gli atti esistenti rispondono anche ai rifiuti con un esito testuale e fanno girare il rismistamento F8 anche dopo un rifiuto.",
    "Due emittenti con lo stesso letterale non convivono nella working: blocco COLLISIONE_EMITTENTI prima della conferma.",
    "Gli avvisi del motore (proposta cambiata, STEP secondario, non incluso condiviso) sono fuori dalla firma: possono cambiare fra il riepilogo e la conferma.",
    "Un nodo senza righe legacy, o con righe chiuse da un automatismo, non si può non includere (NON_INCLUSO_NON_APPLICABILE).",
    "Il fascicolo, la conferma dei documenti, le deroghe, la sostituzione dei documenti e la verifica dall'ultima conferma non sono in questa pagina.",
    "La pagina è solo per l'amministratore, dietro il flag [motore_a].",
  ];
  apriFinestra("Limiti di questa versione", h("ul", { class: "du-elenco-p" }, voci.map((t) => h("li", {}, t))), [{ testo: "Chiudi", chiudi: true }]);
}

// ---------------------------------------------------------------- gesti della bozza (MAT §3.4-§3.7)

function contenitoriPer(g, k) {
  return Object.values(g.nodi).filter((v) => !v.noninc && !v.escluso && contenitore(v) && (!k || !vietatoSotto(k, v.k)));
}
function opzioniPadri(g, k, scelto) {
  return contenitoriPer(g, k).map((v) => h("option", { value: v.k, selected: v.k === scelto }, (v.cod || "(senza codice)") + (v.desc ? " · " + v.desc : "")));
}
function apriAggiungi(tipo, padre) {
  const g = grafoBozza();
  // con un solo padre possibile è già scelto (collaudo UI46, minore)
  const possibili = contenitoriPer(g, "");
  const sel = h("select", { id: "f-padre" }, h("option", { value: "" }, "— scegli il padre —"), opzioniPadri(g, "", padre || (possibili.length === 1 ? possibili[0].k : "")));
  const tipoSel = h("select", { id: "f-tipo" }, ["sottoassieme", "sciolto", "commerciale"].map((t) => h("option", { value: t, selected: t === tipo }, TIPI[t])));
  apriFinestra("Aggiungi un pezzo", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "Il pezzo nasce nella bozza e diventa un componente solo con «Conferma BOM». Il codice si può lasciare vuoto: la conferma lo chiederà."),
    campo("Tipo", tipoSel), campo("Codice", h("input", { id: "f-codice", maxlength: 40, autocomplete: "off" })), campo("Rev", h("input", { id: "f-rev", maxlength: 10, autocomplete: "off" })),
    campo("Descrizione", h("input", { id: "f-desc", maxlength: 200, autocomplete: "off" })), campo("Padre", sel),
    campo("Quantità sul legame", h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric" }), "Nessuna quantità implicita: va scritta.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Aggiungi", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    // le regole di forma del nodo scritto a mano (P-15): codice nuovo ammissibile o vuoto, rev ammissibile, descrizione
    const codice = valore(f, "f-codice");
    const no = (codice ? perchéNoCodiceNuovo(codice) : "") || perchéNoRev(valore(f, "f-rev").toUpperCase()) || perchéNoDescrizione(valore(f, "f-desc"));
    if (no) return no;
    if (!valore(f, "f-padre")) return "scegli il padre";
    if (!qtaValida(valore(f, "f-qta"))) return "scrivi la quantità (un intero da 1 a 100000)";
    return "";
  }, fai: () => {
    const f = $("#du-finestra");
    const k = nuovaChiaveNodo();
    const codice = valore(f, "f-codice"), rev = valore(f, "f-rev").toUpperCase(), desc = valore(f, "f-desc");
    const identita = { emittente: emittenteCliente(), emittente_presunto: true, modo: "manuale" };
    if (codice) identita.codice = codice;
    if (rev) identita.rev = rev;
    const nodo = { chiave_bozza: k, op: "crea", tipo: valore(f, "f-tipo"), origine: "utente", identita };
    if (desc) nodo.descrizione = desc;
    const arco = { padre: valore(f, "f-padre"), figlio: k, qta: qtaValida(valore(f, "f-qta")), origine: "utente" };
    // il pezzo nuovo è il nodo scelto: il pannello lo mostra già nel disegno che fai() rifà
    const prima = S.nodo;
    S.nodo = k;
    if (!fai("aggiunto " + (codice || "un pezzo senza codice"), [
      { azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodo, origine: "utente" },
      { azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: arco, origine: "utente" }])) S.nodo = prima;
  } }]);
}
// decisioniDelNodo: le decisioni della bozza legate al nodo k (la sua chiave_bozza, o il soggetto di una
// conferma_categoria). «Non includere nella BOM…» e «Non crearlo» le tolgono nello stesso gesto, prima del nodo_tolto,
// con lo stesso azione_id quando il gesto ne ha uno; «Annulla» le riporta (DEC-B §7, le decisioni orfane; EM §7.6; prova
// OB-21): altrimenti resterebbero decisioni di un nodo che non c'è, e il server rifiuterebbe il salvataggio.
function decisioniDelNodo(k) {
  return arr(S.lavoro && S.lavoro.decisioni).filter((d) => d.chiave_bozza === k || (ogg(d.soggetto) && d.soggetto.chiave_bozza === k));
}
function toglieDecisioniDelNodo(k, azioneId) {
  return decisioniDelNodo(k).map((d) => ({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: azioneId ? { azione_id: azioneId } : null, origine: "utente" }));
}
function togliNodo(v) {
  const g = grafoBozza();
  const pb = S.lavoro.documenti_preassociati.filter((d) => d.chiave_bozza === v.k);
  apriFinestra("Non crearlo: togliere " + (v.cod || "il pezzo") + " dalla bozza?", h("div", {},
    h("p", {}, "Il pezzo esce dalla bozza. " + (pb.length ? pb.length + " file preassociati vanno in «Da ricollocare»: niente è cancellato." : "")),
    figliA(g, v.k).length ? h("p", { class: "du-nota" }, "I suoi figli restano nella bozza senza questo padre: vanno ricollocati.") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Togli dalla bozza", danger: true, id: "f-ok", fai: () => {
    const azioni = [];
    for (const a of S.lavoro.archi.filter((x) => x.padre === v.k || x.figlio === v.k)) azioni.push({ azione: "arco_tolto", chiave_bozza: a.figlio, prima: clona(a), dopo: null, origine: "utente" });
    for (const d of pb) azioni.push({ azione: "file_svincolato", chiave_bozza: v.k, prima: clona(d), dopo: { allegato_id: d.allegato_id, motivo: "nodo_tolto" }, origine: "utente" });
    azioni.push(...toglieDecisioniDelNodo(v.k, ""));
    azioni.push({ azione: "nodo_tolto", chiave_bozza: v.k, prima: clona(v.nodo), dopo: null, origine: "utente" });
    const prima = S.nodo;
    S.nodo = "";
    if (!fai("tolto " + (v.cod || "un pezzo"), azioni)) S.nodo = prima;
  } }]);
}
// «Escludi (senza cascata)…» di un registrato (MAT BZ-02; §4.3 riga 3). Le sue preassociazioni vanno nel vassoio con il
// motivo nodo_escluso (decisioni dell'ondata A, PE-22; non nodo_tolto, che è di un nodo tolto). Per le associazioni
// registrate (AR) la scelta è obbligatoria e senza preselezione: «Mantieni le associazioni» (il riepilogo avvisa
// ESCLUSO_CON_DOCUMENTI) oppure «Scollega alla conferma», uno svincolo (decisione scollega_documento) per AR. I documenti
// confermati non si toccano: si elencano. Gli svincoli nati dall'esclusione citano l'esclusione in da_decisione
// (decisioni dell'ondata A, PE-27, al posto della convenzione .svN): «↺ Ripristina» li ritrova anche dopo una rilettura
// (riga 4), e il server rifiuta uno svincolo che cita un'esclusione che lo snapshot non ha.
function apriEscludi(v) {
  const doc = documentiNodo(v);
  const ar = doc.ar.filter((f) => !f.documento && !svincoloDi(f.allegato_id));
  const confermati = doc.ar.filter((f) => f.documento);
  const pb = S.lavoro.documenti_preassociati.filter((d) => d.chiave_bozza === v.k);
  const scelta = (id, valore, testo) => h("label", { class: "du-spunta", for: id }, h("input", { type: "radio", name: "f-ar", id, value: valore }), h("span", {}, testo));
  apriFinestra("Escludere " + (v.cod || "il componente") + "?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il componente esce dalla BOM alla conferma: si archivia, mai si cancella se ha storia. Nessuna cascata: i figli restano e vanno ricollocati."),
    pb.length ? h("p", { class: "du-nota", id: "f-escludi-pb" }, pb.length + " file preassociati vanno in «Da ricollocare»: niente è cancellato.") : null,
    ar.length ? h("div", { style: "display:grid;gap:6px", id: "f-escludi-ar" },
      h("p", {}, "È un componente registrato con " + ar.length + " associazioni di file registrate: " + ar.map((f) => f.nome_file).join(", ") + ". Scegli come gestirle: niente viene cancellato, documenti, copie sul NAS e storia restano."),
      scelta("f-ar-mantieni", "mantieni", "Mantieni le associazioni: restano storiche, il componente esce dalla struttura alla conferma (il riepilogo lo avvisa)."),
      scelta("f-ar-scollega", "scollega", "Scollega alla conferma: alla conferma i file perdono l'associazione a questo componente e restano nella RFQ.")) : null,
    confermati.length ? h("p", { class: "du-nota", id: "f-escludi-confermati" }, "Documenti confermati (non si toccano da qui): " + confermati.map((f) => f.nome_file).join(", ") + ".") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Escludi nella bozza", danger: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    return ar.length && !fin.querySelector("input[name=f-ar]:checked") ? "scegli come gestire le associazioni registrate" : "";
  }, fai: () => {
    const fin = $("#du-finestra");
    const scollega = ar.length && fin.querySelector("input[name=f-ar]:checked").value === "scollega";
    const dec = { id: nuovoIdDecisione("es"), tipo: "escludi", componente_id: v.nodo.componente_id };
    const azioni = [];
    for (const d of pb) azioni.push({ azione: "file_svincolato", chiave_bozza: v.k, prima: clona(d), dopo: { allegato_id: d.allegato_id, motivo: "nodo_escluso" }, origine: "utente" });
    azioni.push({ azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: v.nodo.op }, dopo: { op: "esclude" }, origine: "utente" },
      { azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
    // uno svincolo per AR, con da_decisione = l'esclusione (dal messaggio 46 [U] senza motivo), nello stesso salvataggio (PE-27). Lo svincolo cita l'occorrenza, {allegato_id,
    // componente_id} (provvisorio, base [T] D-4 A); il server vuole una AR attiva della coppia (422
    // svincolo_senza_associazione): finché la pagina legge le AR dalla copia di S1 (giro successivo), una copia senza AR
    // si scopre al salvataggio
    if (scollega) ar.forEach((f) => azioni.push({ azione: "decisione_aggiunta", chiave_bozza: null, prima: null,
      dopo: { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", allegato_id: f.allegato_id, componente_id: v.nodo.componente_id, da_decisione: dec.id }, origine: "utente" }));
    fai("escluso " + v.cod + (scollega ? " (associazioni da scollegare alla conferma)" : ar.length ? " (associazioni mantenute)" : ""), azioni);
  } }]);
}
// opPrimaDiEscludi: l'op del nodo prima dell'esclusione, dal gesto che l'ha escluso (in coda, in volo o nel giornale); se
// non si ritrova, «riusa» (il registrato com'era).
function opPrimaDiEscludi(k) {
  const azioni = arr(S.bozza && S.bozza.eventi).map(eventoAzione).concat(...(S.invio ? S.invio.voci : []).concat(S.coda).map((x) => x.inviare));
  let op = "";
  for (const a of azioni) if (a.azione === "nodo_modificato" && a.chiave_bozza === k && ogg(a.dopo) && a.dopo.op === "esclude" && ogg(a.prima) && a.prima.op && a.prima.op !== "esclude") op = a.prima.op;
  return op || "riusa";
}
// «↺ Ripristina» (MAT BZ-04; §4.3 riga 4): il nodo torna con l'op di prima; l'esclusione e gli svincoli che la citano in
// da_decisione si tolgono nello stesso gesto (decisione_tolta; PE-27); le preassociazioni andate nel vassoio per quel
// nodo (nodo_escluso, PE-22) tornano, con eventi espliciti (file_preassociato). Gli svincoli con la convenzione .svN e le
// preassociazioni con nodo_tolto di una bozza scritta prima dell'ondata B si ritrovano lo stesso.
function ripristina(v) {
  const dec = S.lavoro.decisioni.find((d) => d.tipo === "escludi" && d.componente_id === v.nodo.componente_id);
  const azioni = [{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: "esclude" }, dopo: { op: opPrimaDiEscludi(v.k) }, origine: "utente" }];
  if (dec) {
    azioni.push({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(dec), prima: clona(dec), dopo: null, origine: "utente" });
    for (const sv0 of S.lavoro.decisioni.filter((d) => d.tipo === "scollega_documento" && (d.da_decisione === dec.id || String(d.id).indexOf(dec.id + ".sv") === 0))) {
      azioni.push({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(sv0), prima: clona(sv0), dopo: null, origine: "utente" });
    }
  }
  let tornano = 0, restano = 0;
  for (const x of vassoioCorrente().filter((y) => y.chiave_bozza === v.k && (y.motivo === "nodo_escluso" || y.motivo === "nodo_tolto"))) {
    const f = fileDi(x.allegato_id);
    if (f && !preassociabile(f)) {
      restano++;
      continue;
    }
    tornano++;
    azioni.push(...azioniPreassocia(x.allegato_id, v.k));
  }
  if (fai("ripristinato " + v.cod, azioni) && (tornano || restano)) {
    avvisa("ok", "Ripristinato " + v.cod + ": " + tornano + " file tornano preassociati" + (restano ? "; " + restano + " restano in «Da ricollocare» perché il file non è più attivo." : "."));
  }
}
// «✎ Correggi codice, rev e descrizione…» (MAT BZ-05), con le regole di forma di P-15 e la rev di P-18 (decisioni
// dell'ondata A): la rev che non si tocca resta byte per byte com'è (anche con minuscole, se viene dalla working per un
// riusa o un esclude); una rev scritta dall'ingegnere si scrive in maiuscolo; quando la correzione fa la rev nuova (una
// modifica del componente registrato, la decisione di correzione di un pezzo importato) una rev rimasta con minuscole
// ASCII spegne «Salva nella bozza» con il motivo (revMinuscoleAmmesse). Un pezzo importato o registrato ha sempre il codice.
function apriCorreggi(v) {
  const n = v.nodo;
  const id = n.identita || {};
  const radice = n.componente_id && n.componente_id === S.prodotto;
  const importato = !!casellaDi(n) && !n.componente_id;
  const registrato = !!n.componente_id;
  const revPrima = id.rev || "";
  const revDa = (f) => {
    const r = valore(f, "f-rev");
    return r === revPrima ? revPrima : r.toUpperCase();
  };
  apriFinestra("Correggi codice, rev e descrizione", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, importato ? "È un pezzo importato: la sua identità proposta resta com'è, e la correzione diventa una decisione (l'identità decisa)." :
      registrato ? "È un componente registrato: un codice nuovo è una ricodifica." : "È un pezzo aggiunto a mano."),
    campo("Codice", h("input", { id: "f-codice", value: id.codice || "", maxlength: 60, disabled: !!radice, title: radice ? "è il prodotto della RFQ: il codice non cambia (DB7-3)" : null })),
    campo("Rev", h("input", { id: "f-rev", value: id.rev || "", maxlength: 10 })),
    campo("Descrizione", h("input", { id: "f-desc", value: n.descrizione || "", maxlength: 200 }))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Salva nella bozza", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    const codice = valore(f, "f-codice"), rev = revDa(f), desc = valore(f, "f-desc");
    const cambiaCodice = codice !== (id.codice || ""), cambiaRev = rev !== revPrima;
    if (!cambiaCodice && !cambiaRev && desc === (n.descrizione || "")) return "niente da cambiare";
    if ((importato || registrato) && !codice) return "il codice è obbligatorio: un pezzo importato o registrato ha sempre il codice";
    const no = (cambiaCodice && codice ? perchéNoCodiceNuovo(codice) : "") || (cambiaRev ? perchéNoRev(rev) : "") || perchéNoDescrizione(desc);
    if (no) return no;
    // P-18: la rev lasciata com'era può avere minuscole solo se il nodo, dopo la correzione, la copia ancora dalla sua
    // fonte (revMinuscoleAmmesse). La decisione di correzione di un pezzo importato e la ricodifica di un registrato la
    // fanno nuova, anche quando veniva dal codice corretto a mano (manuale_legacy): il server la rifiuterebbe (422)
    const decisioneNuova = importato || (registrato && cambiaCodice) || citaIdentita(id);
    if ((cambiaCodice || cambiaRev) && minuscoleAscii(rev)) {
      const opDopo = registrato && (cambiaCodice || n.op === "riusa") ? "modifica" : n.op;
      const idDopo = decisioneNuova ? { modo: "decisione", decisione: "" } : id;
      if (!revMinuscoleAmmesse(opDopo, idDopo)) {
        return "la rev «" + rev + "» ha minuscole: " + (registrato ? "in una modifica del componente la rev è nuova" : importato ?
          "con la correzione la rev entra nell'identità decisa e non è più copiata dalla sua fonte" : "dopo la correzione la rev non è più copiata dalla sua fonte") + ", scrivila senza minuscole";
      }
    }
    return "";
  }, fai: () => {
    const f = $("#du-finestra");
    const codice = valore(f, "f-codice"), rev = revDa(f), desc = valore(f, "f-desc");
    const azioni = [];
    const prima = {}, dopo = {};
    if (desc !== (n.descrizione || "")) {
      prima.descrizione = n.descrizione === undefined ? null : n.descrizione;
      dopo.descrizione = desc || null;
    }
    const cambiaCodice = codice !== (id.codice || "");
    const cambiaRev = rev !== (id.rev || "");
    if (cambiaCodice || cambiaRev) {
      const nuova = { emittente: id.emittente || emittenteCliente() };
      if (id.namespace && !cambiaCodice) nuova.namespace = id.namespace;
      if (codice) nuova.codice = codice;
      if (rev) nuova.rev = rev;
      if (registrato && cambiaCodice) {
        // la ricodifica: il nodo cita lei, e una decisione identita che citava prima (per esempio «Fissa l'emittente») si
        // toglie nello stesso gesto (togliIdentitaPrecedenti)
        const dec = { id: nuovoIdDecisione("rc"), tipo: "ricodifica", componente_id: n.componente_id, codice_prima: id.codice || "", codice_dopo: codice, emittente_dopo: nuova.emittente };
        azioni.push(...togliIdentitaPrecedenti(v.k), { azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
        prima.identita = clona(id);
        prima.op = n.op;
        dopo.identita = Object.assign({}, nuova, { modo: "decisione", decisione: dec.id });
        dopo.op = "modifica";
      } else if (importato || citaIdentita(id)) {
        // la correzione di un pezzo importato, o di un'identità già decisa da una decisione identita (che deve portare la
        // stessa identità del nodo): una decisione di correzione nuova, che sostituisce quella di prima nello stesso gesto
        const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "correzione", identita: { emittente: nuova.emittente, codice: codice || undefined, rev: rev || undefined } };
        if (nuova.namespace) dec.identita.namespace = nuova.namespace;
        if (nuova.emittente.indexOf("partner:") === 0 && S.bozza.importazione) dec.importazione = S.bozza.importazione.impronta;
        azioni.push(...togliIdentitaPrecedenti(v.k), { azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" });
        prima.identita = clona(id);
        dopo.identita = Object.assign({}, nuova, { modo: "decisione", decisione: dec.id });
        if (registrato && n.op === "riusa") {
          prima.op = n.op;
          dopo.op = "modifica";
        }
      } else {
        prima.identita = clona(id);
        dopo.identita = Object.assign({}, id, { codice: codice || undefined, rev: rev || undefined });
        if (!codice) delete dopo.identita.codice;
        if (!rev) delete dopo.identita.rev;
        if (registrato && n.op === "riusa") {
          prima.op = n.op;
          dopo.op = "modifica";
        }
      }
    }
    azioni.push({ azione: "nodo_modificato", chiave_bozza: v.k, prima, dopo, origine: "utente" });
    // D-ASS-01 [U]: se il codice o la rev nuovi non corrispondono più ai file collegati al pezzo, prima il dialogo
    // «mantieni / scollega / annulla»
    const disc = cambiaCodice || cambiaRev ? fileDiscordanti(v, { codice: id.codice || "", rev: id.rev || "" }, { codice, rev }) : [];
    if (disc.length) {
      apriDiscordanza(v, { codice: id.codice || "", rev: id.rev || "" }, { codice, rev }, disc, "corretto " + (codice || v.cod || "un pezzo"), azioni);
      return false;
    }
    fai("corretto " + (codice || v.cod || "un pezzo"), azioni);
  } }]);
}
// fileDiscordanti: i file collegati al pezzo (le PB della bozza e le AR del componente non ancora scollegate, esclusi i
// documenti confermati) il cui codice letto, o la revisione quando il file e il pezzo ne hanno una, corrispondeva
// all'identità di prima e non corrisponde più a quella nuova (D-ASS-01 [U]: «nessun conflitto se la modifica lo elimina»;
// un file già discordante prima lo dice già il riepilogo). Il confronto è quello di R4 (codice_diverso): senza maiuscole
// né spazi ai lati; un file senza codice letto non si giudica (lo dice codice_non_letto) [T-39].
function fileDiscordanti(v, prima, dopo) {
  const norm = (x) => String(x || "").trim().toUpperCase();
  const corrisponde = (f, id) => {
    const c = f.proposta && f.proposta.codice;
    if (norm(c) !== norm(id.codice)) return false;
    const r = f.proposta && f.proposta.rev;
    return !(r && id.rev && norm(r) !== norm(id.rev));
  };
  const out = [];
  const doc = documentiNodo(v);
  const guarda = (f, tipo) => {
    if (!f || !(f.proposta && f.proposta.codice)) return;
    if (corrisponde(f, prima) && !corrisponde(f, dopo)) out.push({ f, tipo });
  };
  for (const aid of doc.pb) guarda(fileDi(aid), "pb");
  for (const f of doc.ar) if (!f.documento && !svincoloDi(f.allegato_id)) guarda(f, "ar");
  return out;
}
// apriDiscordanza: il dialogo di D-ASS-01 [U] dopo «Correggi codice, rev e descrizione…»: «Mantieni i collegamenti» (il
// collegamento resta sullo stesso nodo; il riepilogo lo dirà in conflitto, codice diverso, e la conferma resta bloccata
// finché non corrisponde: nessuna deroga ai blocchi di R4), «Scollega i file» (la PB esce dalla bozza, la AR si scollega
// alla conferma con lo svincolo {allegato_id, componente_id}), «Annulla» (niente cambia). Una ricodifica di un componente
// con documenti già confermati resta bloccata da RICODIFICA_FILE_NAS in ogni caso (motivo documento_confermato o
// scritto_sul_nas, blocco «R5 completo» della persistenza).
function apriDiscordanza(v, prima, dopo, disc, etichetta, azioni) {
  const id = (x) => (x.codice || "(senza codice)") + (x.rev ? " rev " + x.rev : "");
  const riga = (x) => h("li", { "data-du-discordante": x.f.allegato_id, "data-tipo": x.tipo }, h("span", { class: "du-mono" }, x.f.nome_file), " · ",
    x.tipo === "pb" ? "preassociato nella bozza" : "associazione registrata", " · codice letto ", h("span", { class: "du-mono" }, (x.f.proposta.codice || "") + (x.f.proposta.rev ? " rev " + x.f.proposta.rev : "")));
  const registrato = !!(v.nodo && v.nodo.componente_id);
  apriFinestra("I file collegati non corrispondono più", h("div", { style: "display:grid;gap:8px", id: "f-discordanza" },
    h("p", {}, "Il pezzo passa da ", h("span", { class: "du-mono" }, id(prima)), " a ", h("span", { class: "du-mono" }, id(dopo)), ": " + disc.length + (disc.length === 1 ? " file collegato non corrisponde più." : " file collegati non corrispondono più.")),
    h("ul", { class: "du-elenco-p", id: "f-discordanti" }, disc.map(riga)),
    h("p", { class: "du-nota" }, "«Mantieni i collegamenti»: restano su questo pezzo" +
      (disc.some((x) => x.tipo === "pb") ? "; il riepilogo dirà in conflitto le preassociazioni (codice diverso) e «Conferma BOM» resterà bloccata finché codice e file non corrispondono" : "") +
      (disc.some((x) => x.tipo === "ar") ? "; le associazioni registrate seguono il codice nuovo alla conferma («da ricodificare»)" : "") +
      ". «Scollega i file»: le preassociazioni escono dalla bozza, le associazioni registrate si scollegano alla conferma; i file restano nella RFQ."),
    registrato ? h("p", { class: "du-nota" }, "Per un componente con documenti già confermati (scritti sul NAS o no) la ricodifica resta bloccata (RICODIFICA_FILE_NAS) in ogni caso: il codice si corregge dalla scheda del componente.") : null),
  [{ testo: "Annulla", chiudi: true },
    { testo: "Scollega i file", id: "f-scollega-discordanti", fai: () => {
      const extra = [];
      for (const x of disc) {
        if (x.tipo === "pb") {
          const pb = pbDi(x.f.allegato_id);
          if (pb) extra.push({ azione: "file_svincolato", chiave_bozza: pb.chiave_bozza, prima: clona(pb), dopo: { allegato_id: x.f.allegato_id, motivo: "utente" }, origine: "utente" });
        } else {
          const dec = { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", allegato_id: x.f.allegato_id, componente_id: v.nodo.componente_id, motivo: "codice o rev del pezzo cambiati (D-ASS-01)" };
          extra.push({ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
        }
      }
      fai(etichetta + ", scollegati " + disc.length + " file", azioni.concat(extra));
    } },
    { testo: "Mantieni i collegamenti", pri: true, id: "f-mantieni-discordanti", fai: () => {
      fai(etichetta + ", collegamenti mantenuti", azioni);
    } }]);
}
// «◆ Cambia tipo…» (MAT BZ-06; §4.3 riga 8). Per un pezzo della bozza è un nodo_modificato del tipo. Per un componente
// registrato serve anche la decisione cambia_tipo{componente_id, tipo_nuovo, firma_effetto} (EM §3.6 riga 13; P-7 della
// persistenza: il tipo nuovo sta in tipo_nuovo), con la firma dell'effetto che dà la variante di lettura di R1
// (?effetto_cambio_tipo=, decisioni dell'ondata A, PE-09): il dialogo la legge per il tipo scelto, mostra l'effetto e
// accende «Cambia» solo con l'effetto letto e un cambio che si può fare. Un cambio di tipo già deciso nella bozza per lo
// stesso componente si sostituisce nello stesso gesto: una decisione sola per componente.
const AIUTI_TIPO = {
  sottoassieme: "Un assieme ha pezzi sotto. Il 2D resta richiesto.",
  sciolto: "Un particolare non ha pezzi sotto. Il 2D resta richiesto.",
  commerciale: "Un commerciale non ha pezzi sotto. Anche il commerciale chiede il 2D: non lo chiede solo la minuteria confermata da una persona («Conferma la categoria…»).",
};
function apriTipo(v) {
  const registrato = !!(v.nodo && v.nodo.componente_id);
  const sel = h("select", { id: "f-tipo" }, ["sottoassieme", "sciolto", "commerciale"].map((t) => h("option", { value: t, selected: t === v.tipo }, TIPI[t])));
  const box = h("div", { id: "f-effetto", "aria-live": "polite" });
  const st = { tipo: "", effetto: null, errore: "", calcolo: false, giro: 0 };
  const scelto = () => valore($("#du-finestra"), "f-tipo");
  let fin = null;
  const mostraEffetto = () => {
    if (!registrato) return;
    if (st.calcolo) return box.replaceChildren(h("p", { class: "du-muto du-piccolo" }, "Calcolo dell'effetto del cambio…"));
    if (st.errore) return box.replaceChildren(h("p", { class: "du-err", id: "f-effetto-errore" }, "L'effetto del cambio non si legge: " + st.errore));
    if (!st.effetto) return box.replaceChildren();
    box.replaceChildren(h("div", { class: "du-esito du-esito--ver", id: "f-effetto-righe" }, h("b", {}, "Effetto del cambio, calcolato dal server adesso:"),
      h("ul", { class: "du-elenco-p" }, righeEffetto(st.effetto.effetto, v).map((t) => h("li", {}, t)))));
  };
  const leggiEffetto = async () => {
    if (!registrato) return;
    const t = scelto();
    const giro = ++st.giro;
    st.tipo = t;
    st.effetto = null;
    st.errore = "";
    st.calcolo = t !== v.tipo;
    mostraEffetto();
    if (fin) fin.aggiorna();
    if (t === v.tipo) return;
    try {
      const j = await P.leggiBozza(S.prodotto, { effetto_cambio_tipo: v.nodo.componente_id, tipo_nuovo: t });
      if (giro !== st.giro) return;
      st.effetto = j;
    } catch (e) {
      if (giro !== st.giro) return;
      st.errore = testoErrore(e) + (e && e.codice === "PONTE_RISPOSTA" ? " (la lettura della bozza non dà ancora l'effetto del cambio di tipo, o non nella forma che la pagina legge)" : "");
    }
    st.calcolo = false;
    mostraEffetto();
    if (fin) fin.aggiorna();
  };
  // l'aiuto segue il tipo scelto (messaggio 46, D39), con le parole e senza i codici delle regole
  const nFigli = S.lavoro ? figliA(grafoBozza(), v.k).length : 0;
  const aiuto = h("p", { class: "du-nota", id: "f-tipo-aiuto", "aria-live": "polite" });
  const scriviAiuto = () => {
    const t = scelto() || v.tipo;
    aiuto.textContent = (AIUTI_TIPO[t] || "") + (nFigli && t !== "sottoassieme" ? " Ha " + nFigli + (nFigli === 1 ? " pezzo" : " pezzi") + " sotto: con questo tipo vanno spostati sotto un assieme." : "");
  };
  sel.addEventListener("change", scriviAiuto);
  sel.addEventListener("change", leggiEffetto);
  // un cambio di tipo già deciso nella bozza per questo componente: si toglie con il suo gesto, che riporta il nodo al
  // tipo di prima (dal giornale); l'effetto verso il tipo registrato il server non lo firma, perché non c'è un cambio
  const decise = registrato ? S.lavoro.decisioni.filter((x) => x.tipo === "cambia_tipo" && x.componente_id === v.nodo.componente_id) : [];
  const tipoPrima = decise.length ? tipoPrimaDelCambio(v.k, decise[decise.length - 1]) : "";
  const togli = decise.length ? h("div", { class: "du-azioni", id: "f-togli-cambio" }, btn("Togli il cambio di tipo", () => {
    chiudiFinestra();
    fai("tolto il cambio di tipo di " + (v.cod || "un pezzo"), decise.map((d) => ({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" }))
      .concat([{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { tipo: v.tipo }, dopo: { tipo: tipoPrima }, origine: "utente" }]));
  }, { cls: "du-btn--sm", id: "f-togli-tipo", no: tipoPrima ? "" : "il tipo di prima non si ritrova nel giornale della bozza: usa «Annulla»" }),
  h("span", { class: "du-muto du-piccolo" }, tipoPrima ? "La bozza ha già un cambio di tipo di questo componente: togliendolo, il pezzo torna " + (TIPI[tipoPrima] || tipoPrima) + "." : "")) : null;
  fin = apriFinestra("Cambia tipo di " + (v.cod || "il pezzo"), h("div", { style: "display:grid;gap:8px" }, campo("Tipo", sel),
    aiuto,
    registrato ? h("p", { class: "du-nota" }, "È un componente registrato: il cambio è una decisione della bozza, con l'effetto che il server calcola adesso. La conferma lo applica; se nel frattempo l'effetto cambia, il riepilogo lo dice.") : null,
    togli, box),
  [{ testo: "Annulla", chiudi: true }, { testo: "Cambia", pri: true, id: "f-ok", spento: () => {
    scriviAiuto();
    const t = scelto();
    if (t === v.tipo) return "è già di questo tipo";
    if (!registrato) return "";
    if (st.tipo !== t || st.calcolo) return "calcolo dell'effetto del cambio in corso";
    if (st.errore) return "l'effetto del cambio non si legge: il cambio non parte";
    if (!st.effetto) return "manca l'effetto del cambio";
    const e = st.effetto.effetto || {};
    if (typeof e.spento === "string" && e.spento) return "il cambio non si fa: " + e.spento;
    return "";
  }, fai: () => {
    const t = scelto();
    const azioni = [];
    if (registrato) {
      for (const d of S.lavoro.decisioni.filter((x) => x.tipo === "cambia_tipo" && x.componente_id === v.nodo.componente_id)) {
        azioni.push({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" });
      }
      azioni.push({ azione: "decisione_aggiunta", chiave_bozza: null, prima: null, dopo: { id: nuovoIdDecisione("ct"), tipo: "cambia_tipo", componente_id: v.nodo.componente_id,
        tipo_nuovo: t, firma_effetto: st.effetto.firma_effetto }, origine: "utente" });
    }
    azioni.push({ azione: "nodo_modificato", chiave_bozza: v.k, prima: { tipo: v.tipo }, dopo: { tipo: t }, origine: "utente" });
    fai("tipo di " + (v.cod || v.desc || "un pezzo") + " → " + TIPI[t], azioni);
  } }]);
}
// tipoPrimaDelCambio: il tipo del nodo k prima della decisione cambia_tipo d, dal giornale (R1 ?eventi=1, più i gesti in
// volo e in coda): il nodo_modificato del gesto che l'ha aggiunta. Se quel gesto sostituiva un cambio di tipo di prima
// (la sua decisione_tolta subito prima), si risale a quello: il tipo che conta è quello registrato. "" se non si ritrova.
function tipoPrimaDelCambio(k, d) {
  const ev = arr(S.bozza && S.bozza.eventi).filter((e) => ogg(e) && typeof e.azione === "string").sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0)).map(eventoAzione);
  const azioni = ev.concat(...(S.invio ? S.invio.voci : []).concat(S.coda).map((x) => x.inviare));
  let id = d.id;
  for (let giri = 0; giri < 20 && id; giri++) {
    const i = azioni.findIndex((a) => a.azione === "decisione_aggiunta" && ogg(a.dopo) && a.dopo.id === id);
    if (i < 0) return "";
    const prima = i > 0 ? azioni[i - 1] : null;
    if (prima && prima.azione === "decisione_tolta" && ogg(prima.prima) && prima.prima.tipo === "cambia_tipo" && prima.prima.componente_id === d.componente_id) {
      id = prima.prima.id;
      continue;
    }
    const dopo = azioni.slice(i + 1).find((a) => a.azione === "nodo_modificato" && a.chiave_bozza === k);
    return dopo && ogg(dopo.prima) && typeof dopo.prima.tipo === "string" ? dopo.prima.tipo : "";
  }
  return "";
}
// righeEffetto: l'effetto del cambio di tipo in parole. La forma di «effetto» non è ancora scritta in EM: la pagina legge
// i campi dell'anteprima del cambio di tipo che c'è già (EffettoTipo del fascicolo, in minuscolo: spento, sospende,
// con_lei, rimozioni, resta_sospesa, autorizzabile, step_strutturale, svuota_step, avvisi; proposta di emendamento della
// corsia PE-B05) e ignora gli altri. Un effetto in un'altra forma non arriva qui: il ponte lo rifiuta (effettoLeggibile,
// PONTE_RISPOSTA), e «Cambia» resta spento con «l'effetto del cambio non si legge».
function righeEffetto(e, v) {
  const o = ogg(e) ? e : {};
  const out = [];
  if (typeof o.spento === "string" && o.spento) out.push("Il cambio non si fa: " + o.spento + ".");
  if (arr(o.sospende).length) out.push("Si sospendono: " + arr(o.sospende).join("; ") + ".");
  if (arr(o.con_lei).length) out.push("Con loro si sospendono le deleghe di: " + arr(o.con_lei).join(", ") + ".");
  if (Number(o.rimozioni) > 0) out.push("Rimozioni aperte che si chiudono: " + Number(o.rimozioni) + ".");
  if (arr(o.resta_sospesa).length) out.push("Restano sospese finché una persona non le riattiva: " + arr(o.resta_sospesa).join("; ") + ".");
  if (o.autorizzabile === true) out.push("Il suo STEP si potrà autorizzare.");
  if (typeof o.step_strutturale === "string" && o.step_strutturale) out.push("Diventa il suo STEP strutturale: " + o.step_strutturale + ".");
  if (typeof o.svuota_step === "string" && o.svuota_step) out.push("Non è più il suo STEP strutturale: " + o.svuota_step + ".");
  for (const a of arr(o.avvisi)) out.push(String(a));
  if (!out.length) out.push((v.cod || "Il pezzo") + " cambia tipo: nessun altro effetto.");
  return out;
}
function apriSposta(k, verso) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const pa = padriA(g, k);
  const daSel = h("select", { id: "f-da" }, pa.map((a) => h("option", { value: a.p }, "da " + (g.nodi[a.p].cod || g.nodi[a.p].desc) + " (×" + a.qta + ")")));
  const aSel = h("select", { id: "f-a" }, h("option", { value: "" }, "— scegli il padre di arrivo —"), opzioniPadri(g, k, verso));
  // collaudo UI46, giro 2 (minori): la quantità del legame che si sposta è già scritta (si sposta il legame con la sua
  // quantità); si cambia se serve, e segue il legame scelto finché non la si tocca. Senza padri resta da scrivere [T]
  const qta = h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric", value: pa.length ? String(pa[0].qta) : "" });
  let toccata = false;
  qta.addEventListener("input", () => (toccata = true));
  daSel.addEventListener("change", () => {
    const x = pa.find((a) => a.p === daSel.value);
    if (x && !toccata) qta.value = String(x.qta);
  });
  apriFinestra("Sposta " + (v.cod || "il pezzo") + " sotto…", h("div", { style: "display:grid;gap:8px" },
    pa.length ? campo("Legame da spostare", daSel) : h("p", { class: "du-nota" }, "Il pezzo non ha padri: lo spostamento gli dà un padre."), campo("Nuovo padre", aSel),
    campo("Quantità sul legame nuovo", qta, pa.length ? "La quantità del legame di adesso: cambiala se sotto il nuovo padre è un'altra." : "Nessuna quantità implicita: va scritta.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Sposta", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    const a = valore(f, "f-a");
    if (!a) return "scegli il padre di arrivo";
    const no = vietatoSotto(k, a);
    if (no) return no;
    if (!qtaValida(valore(f, "f-qta"))) return "scrivi la quantità (un intero da 1 a 100000)";
    return "";
  }, fai: () => {
    const f = $("#du-finestra");
    const a = valore(f, "f-a");
    const q = qtaValida(valore(f, "f-qta"));
    const da = pa.length ? valore(f, "f-da") : "";
    const vecchio = da ? S.lavoro.archi.find((x) => x.padre === da && x.figlio === k) : null;
    const nuovo = { padre: a, figlio: k, qta: q, origine: "utente" };
    if (vecchio) fai("spostato " + (v.cod || "un pezzo"), [{ azione: "arco_spostato", chiave_bozza: k, prima: clona(vecchio), dopo: nuovo, origine: "utente" }]);
    else fai("collegato " + (v.cod || "un pezzo"), [{ azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: nuovo, origine: "utente" }]);
  } }]);
}
function apriCollega(k) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const giaPadri = new Set(padriA(g, k).map((a) => a.p));
  const aSel = h("select", { id: "f-a" }, h("option", { value: "" }, "— scegli il padre —"), opzioniPadri(g, k, "").filter((o) => !giaPadri.has(o.value)));
  apriFinestra("Collega " + (v.cod || "il pezzo") + " anche sotto…", h("div", { style: "display:grid;gap:8px" }, h("p", { class: "du-nota" }, "Un componente, più legami: non si duplica."), campo("Altro padre", aSel),
    campo("Quantità", h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric" }))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Collega", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    if (!valore(f, "f-a")) return "scegli il padre";
    return qtaValida(valore(f, "f-qta")) ? "" : "scrivi la quantità";
  }, fai: () => {
    const f = $("#du-finestra");
    fai("collegato anche " + (v.cod || "un pezzo"), [{ azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: { padre: valore(f, "f-a"), figlio: k, qta: qtaValida(valore(f, "f-qta")), origine: "utente" }, origine: "utente" }]);
  } }]);
}
function apriScollegaPadre(k) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const pa = padriA(g, k);
  apriFinestra("Scollega " + (v.cod || "il pezzo") + " da un padre", h("div", { style: "display:grid;gap:8px" },
    campo("Legame", h("select", { id: "f-da" }, pa.map((a) => h("option", { value: a.p }, (g.nodi[a.p].cod || g.nodi[a.p].desc) + " (×" + a.qta + ")")))),
    pa.length === 1 ? h("p", { class: "du-nota" }, "È l'unico padre: il pezzo resta «da ricollocare».") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scollega", danger: true, id: "f-ok", fai: () => {
    const p = valore($("#du-finestra"), "f-da");
    const a = S.lavoro.archi.find((x) => x.padre === p && x.figlio === k);
    if (!a) return false;
    const azioni = [{ azione: "arco_tolto", chiave_bozza: k, prima: clona(a), dopo: null, origine: "utente" }];
    const np = g.nodi[p].nodo, nf = v.nodo;
    if ((a.origine === "registrato" || a.origine === "conferma") && np.componente_id && nf.componente_id) {
      azioni.push({ azione: "decisione_aggiunta", chiave_bozza: null, prima: null, dopo: { id: nuovoIdDecisione("ta"), tipo: "togli_arco", padre_id: np.componente_id, figlio_id: nf.componente_id }, origine: "utente" });
    }
    fai("scollegato " + (v.cod || "un pezzo"), azioni);
  } }]);
}
function apriQuantita(k, padre) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const pa = padriA(g, k);
  if (!pa.length) return;
  apriFinestra("Quantità di " + (v.cod || "il pezzo"), h("div", { style: "display:grid;gap:8px" },
    campo("Legame", h("select", { id: "f-da" }, pa.map((a) => h("option", { value: a.p, selected: a.p === padre }, (g.nodi[a.p].cod || g.nodi[a.p].desc) + " (×" + a.qta + (a.qtaStep !== undefined ? ", STEP ×" + a.qtaStep : "") + ")")))),
    campo("Quantità", h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric" }))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Salva nella bozza", pri: true, id: "f-ok", spento: () => (qtaValida(valore($("#du-finestra"), "f-qta")) ? "" : "scrivi la quantità (un intero da 1 a 100000)"), fai: () => {
    const f = $("#du-finestra");
    const p = valore(f, "f-da");
    const a = S.lavoro.archi.find((x) => x.padre === p && x.figlio === k);
    const q = qtaValida(valore(f, "f-qta"));
    if (!a || a.qta === q) return false;
    fai("quantità di " + (v.cod || "un pezzo") + " → ×" + q, [{ azione: "qta_modificata", chiave_bozza: k, prima: { padre: p, figlio: k, qta: a.qta }, dopo: { padre: p, figlio: k, qta: q }, origine: "utente" }]);
  } }]);
}
// «Non includere nella BOM…» (MAT NI-05; EM §7.1): un solo salvataggio, una decisione per nodo con lo stesso azione_id
// su tutti gli eventi del gesto. L'elenco è visibile e si può ridurre. Dal messaggio 46 [U] l'utente non scrive il motivo
// e non sceglie il «motivo (codice)»: la decisione li vuole, perché lo snapshot confermato deve portarli uguali alle righe
// di conferma_strutturale_non_incluso (0023, controllo (iii)), e li scrive la pagina come traccia del gesto
// (tracciaDelGesto) con motivo_codice dal segnale visto (motivoCodiceDelGesto) [T].
function apriNonIncludere(g) {
  const scelti = [...S.selezione].filter((k) => g.nodi[k] && selezionabile(g, g.nodi[k]).ok);
  if (!scelti.length) return avvisa("no", "Nessun nodo selezionabile: la radice e i pezzi con un componente non si possono non includere.");
  const lista = h("div", { class: "du-voci-sel", id: "f-elenco" }, scelti.map((k) => {
    const v = g.nodi[k];
    const pb = S.lavoro.documenti_preassociati.filter((d) => d.chiave_bozza === k).length;
    return h("label", { class: "du-spunta" }, h("input", { type: "checkbox", value: k, checked: true }),
      h("span", {}, h("span", { class: "du-mono" }, (v.voce && v.voce.grezzi && (v.voce.grezzi.nome || v.voce.grezzi.id)) || v.cod || k), v.cad ? " · segnalato" : "", pb ? " · " + pb + " file preassociati vanno in «Da ricollocare»" : "",
        figliA(g, k).length ? " · ha figli: resteranno senza padre (nessuna cascata)" : ""));
  }));
  apriFinestra("Non includere nella BOM", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "I nodi escono dalla bozza con una decisione ciascuno, che la conferma applica. Fino alla conferma il 2D resta richiesto e bloccante. I fatti dello STEP restano."),
    lista),
  [{ testo: "Annulla", chiudi: true }, { testo: "Non includere", danger: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    return f.querySelectorAll("#f-elenco input:checked").length ? "" : "nessun nodo nell'elenco";
  }, fai: () => {
    const f = $("#du-finestra");
    const chiavi = [...f.querySelectorAll("#f-elenco input:checked")].map((x) => x.value);
    const azioneId = P.nuovaChiave();
    const reg = registroBozza();
    const imp = S.bozza.importazione ? S.bozza.importazione.impronta : undefined;
    const azioni = [];
    const decisioni = [];
    const tolti = new Set();
    // l'ordine: per ogni nodo preassociazioni, archi, nodo; in fondo le decisioni. L'annullamento ne fa l'inverso, cioè
    // l'ordine fisso di EM §7.6: decisioni, nodi, archi, preassociazioni
    for (const k of chiavi) {
      const n = S.lavoro.nodi.find((x) => x.chiave_bozza === k);
      if (!n) continue;
      for (const d of S.lavoro.documenti_preassociati.filter((x) => x.chiave_bozza === k)) {
        azioni.push({ azione: "file_svincolato", chiave_bozza: k, prima: clona(d), dopo: { allegato_id: d.allegato_id, motivo: "non_incluso", azione_id: azioneId }, origine: "utente" });
      }
      for (const a of S.lavoro.archi.filter((x) => (x.padre === k || x.figlio === k) && !tolti.has(x.padre + ">" + x.figlio))) {
        tolti.add(a.padre + ">" + a.figlio);
        azioni.push({ azione: "arco_tolto", chiave_bozza: a.figlio, prima: clona(a), dopo: { azione_id: azioneId }, origine: "utente" });
      }
      azioni.push(...toglieDecisioniDelNodo(k, azioneId));
      azioni.push({ azione: "nodo_tolto", chiave_bozza: k, prima: clona(n), dopo: { azione_id: azioneId }, origine: "utente" });
      const c = casellaDi(n);
      const voce = voceDiCasella(reg, c);
      const seg = segnaliDi(voce)[0];
      const nome = (voce && voce.grezzi && (voce.grezzi.nome || voce.grezzi.id)) || (g.nodi[k] && g.nodi[k].cod) || k;
      const dec = { id: nuovoIdDecisione("ni"), tipo: "non_includere", casella: clona(c), motivo_codice: motivoCodiceDelGesto(seg),
        motivo: tracciaDelGesto("Non includere nella BOM", "«" + nome + "» esce dalla BOM" + (seg && seg.codice ? " (segnale del motore: " + testoSegnale(seg.codice) + ")" : "")), azione_id: azioneId };
      if (imp) dec.importazione = imp;
      if (seg) dec.segnale_visto = { codice: seg.codice, regola: seg.regola, versione_servizio: versioneServizio(reg) };
      decisioni.push({ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
    }
    azioni.push(...decisioni);
    S.selezione.clear();
    fai(chiavi.length === 1 ? "non incluso un nodo" : "non inclusi " + chiavi.length + " nodi", azioni);
  } }]);
}
// versioneServizio: la versione del servizio dell'ancoraggio nel registro (regole.versioni.ancoraggio, EM §3.8): il
// segnale_visto la copia, non la calcola.
function versioneServizio(reg) {
  const r = reg && reg.regole;
  return (r && r.versioni && r.versioni.ancoraggio) || (r && r.ancoraggio) || "";
}
// nodoDaVoce: il nodo di una voce del registro con l'identità che darebbero le regole P-4 della partenza (EM §6.1):
// «proposta» con codice_proposto non vuoto (con rev_proposta), «da_decidere» senza; tipo proposto, descrizione grezza
// entro i 200 caratteri, la casella, origine «step». Una voce con decisione_esistente (P-3, «registrato») non passa da
// qui: l'identità registrata non è nel registro (proposta di emendamento della corsia).
// voceRegistrata: la voce ha un componente registrato (decisione_esistente) che non è il prodotto: per P-3 il nodo
// sarebbe «registrato», con l'identità della certificazione che il registro non porta (EM §6.1). Un raggruppamento
// dichiarato con «Autorizza» (decisione_esistente = il prodotto) segue invece P-4, con il tipo sottoassieme.
function voceRegistrata(voce) {
  const d = voce && voce.decisione_esistente;
  return !!(d && d.componente_id && String(d.componente_id).toLowerCase() !== S.prodotto);
}
// identitaRegistrata: l'identità registrata del componente di una voce con decisione_esistente, come la dà /dati
// (decisione_esistente.identita_registrata dei nodi del gruppo: EM §9.9, PE-10; B-3), nella forma che il nodo
// «registrato» copia (EM §3.4.2). Con quella «Importa dalla proposta…» e «Includi di nuovo» costruiscono il nodo di P-3
// (MAT BZ-15, NI-07: «la corsia interfaccia accende il gesto all'ondata C»); senza, il gesto resta spento con il motivo.
function identitaRegistrata(voce) {
  const n = nodoDelModello(voce && voce.voce);
  const de = n && n.decisione_esistente;
  const id = de && de.identita_registrata;
  if (!ogg(id) || typeof id.codice !== "string" || !id.codice || typeof id.emittente !== "string") return null;
  return { componente_id: de.componente_id || (voce.decisione_esistente && voce.decisione_esistente.componente_id), identita: id };
}
function voceNonImportabile(voce) {
  return voceRegistrata(voce) && !identitaRegistrata(voce);
}
// Messaggio 46 [U] (21/10): il motivo scritto dall'utente si toglie da ogni widget. TESTO_MOTIVO_VECCHIO: il testo di un
// MOTIVO_OBBLIGATORIO che un server di prima potrebbe ancora dare (la pagina non chiede mai il motivo).
const TESTO_MOTIVO_VECCHIO = "Il server chiede ancora un motivo scritto: è una versione di prima del server. Aggiorna il server; niente è stato scritto.";
// motivoCodiceDelGesto: il motivo_codice di «Non includere nella BOM» quando l'utente non sceglie (la scelta da un
// vocabolario chiuso che la pagina presentava come «motivo»): dal segnale che il nodo porta, «elemento_cad» per il
// possibile elemento CAD del motore, altrimenti «altro», il valore che non dice niente di più del gesto [T].
function motivoCodiceDelGesto(seg) {
  return seg && seg.codice === "ancoraggio.possibile_elemento_cad" ? "elemento_cad" : "altro";
}
// tracciaDelGesto: il motivo che la pagina scrive al posto dell'utente dove la decisione dello snapshot lo deve portare
// (non_includere: lo snapshot confermato e conferma_strutturale_non_incluso devono dire lo stesso, 0023 controllo (iii)).
// La forma è quella del server (core/rfq/traccia): «gesto di <sigla> da «<widget>»: <che cosa>», in NFC, al più 1000
// caratteri. Non finge che l'abbia scritto l'utente: dice il gesto e chi l'ha fatto.
function tracciaDelGesto(widget, cosa) {
  const pulito = (x) => String(x || "").replace(/[\r\n\t]+/g, " ").trim();
  const s = pulito(S.sigla);
  let t = "gesto" + (s ? " di " + s : "") + " da «" + pulito(widget) + "»" + (pulito(cosa) ? ": " + pulito(cosa) : "");
  t = typeof t.normalize === "function" ? t.normalize("NFC") : t;
  const r = [...t];
  return r.length > 1000 ? r.slice(0, 999).join("") + "…" : t;
}
const MOTIVO_VOCE_REGISTRATA = "la voce ha un componente registrato e il modello non dà ancora la sua identità registrata (decisione_esistente.identita_registrata di /dati): ↻ Aggiorna";
function nodoDaVoce(voce, k, casella) {
  if (voceRegistrata(voce)) {
    // P-3 (EM §6.1): un riuso del componente registrato, con l'identità registrata di /dati, byte per byte (V-1)
    const reg = identitaRegistrata(voce);
    const id = { emittente: reg.identita.emittente, codice: reg.identita.codice, modo: "registrato" };
    if (reg.identita.emittente_presunto) id.emittente_presunto = true;
    if (reg.identita.namespace) id.namespace = reg.identita.namespace;
    if (reg.identita.rev) id.rev = reg.identita.rev;
    const nodo = { chiave_bozza: k, componente_id: String(reg.componente_id).toLowerCase(), op: "riusa", tipo: (voce.tipo_proposto && voce.tipo_proposto.valore) || "sciolto", origine: "step", caselle: [clona(casella)], identita: id };
    const desc = voce.grezzi && voce.grezzi.descrizione;
    if (desc && String(desc).length <= 200) nodo.descrizione = desc;
    return nodo;
  }
  const ip = voce.identita_proposta || {};
  const identita = voce.codice_proposto ? { emittente: ip.emittente || emittenteCliente(), codice: voce.codice_proposto, modo: "proposta" } : { emittente: ip.emittente || emittenteCliente(), modo: "da_decidere" };
  if (voce.codice_proposto && ip.namespace) identita.namespace = ip.namespace;
  if (voce.codice_proposto && voce.rev_proposta) identita.rev = voce.rev_proposta;
  const raggruppamento = !!(voce.decisione_esistente && String(voce.decisione_esistente.componente_id || "").toLowerCase() === S.prodotto);
  const nodo = { chiave_bozza: k, op: "crea", tipo: raggruppamento ? "sottoassieme" : (voce.tipo_proposto && voce.tipo_proposto.valore) || "sciolto", origine: "step", caselle: [clona(casella)], identita };
  const desc = voce.grezzi && voce.grezzi.descrizione;
  if (desc && String(desc).length <= 200) nodo.descrizione = desc;
  return nodo;
}
function includiDiNuovo(v) {
  const d = v.decisione;
  const reg = registroBozza();
  const voce = v.voce;
  if (!voce) return avvisa("errore", "La voce del registro non c'è: il nodo non si può includere di nuovo da qui (rileggi la bozza).");
  if (voceNonImportabile(voce)) return avvisa("no", "«Includi di nuovo» non parte: " + MOTIVO_VOCE_REGISTRATA + ".");
  // la chiave di prima, se il giornale o la coda la dicono: lo stesso nodo, mai la stessa chiave per un nodo diverso
  let k = "";
  const tutti = arr(S.bozza.eventi).map(eventoAzione).concat(...S.coda.map((x) => x.inviare));
  for (const a of tutti) if (a.azione === "nodo_tolto" && a.prima && casellaDi(a.prima) && casellaDi(a.prima).rif === d.casella.rif) k = a.chiave_bozza || a.prima.chiave_bozza;
  if (!k || S.lavoro.nodi.some((n) => n.chiave_bozza === k)) k = nuovaChiaveNodo();
  const nodo = nodoDaVoce(voce, k, d.casella);
  const azioni = [{ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" }, { azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodo, origine: "utente" }];
  const perRif = new Map();
  for (const n of S.lavoro.nodi) {
    const c = casellaDi(n);
    if (c) perRif.set(c.rif, n.chiave_bozza);
  }
  for (const a of arr(reg && reg.proposta && reg.proposta.archi)) {
    const step = { allegato_id: d.casella.allegato_id, gruppo: d.casella.gruppo };
    if (a.figlio === d.casella.rif && perRif.has(a.padre)) azioni.push({ azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: { padre: perRif.get(a.padre), figlio: k, qta: a.quantita, origine: "step", step }, origine: "utente" });
    if (a.padre === d.casella.rif && perRif.has(a.figlio)) azioni.push({ azione: "arco_aggiunto", chiave_bozza: perRif.get(a.figlio), prima: null, dopo: { padre: k, figlio: perRif.get(a.figlio), qta: a.quantita, origine: "step", step }, origine: "utente" });
  }
  const prima = S.nodo;
  S.nodo = k;
  if (!fai("incluso di nuovo " + (voce.codice_proposto || "un nodo"), azioni)) S.nodo = prima;
}
// Il riquadro «Proposta dello STEP» della Revisione (mockup v3, «Proposta dello STEP e BOM registrata»; MAT BZ-15,
// BZ-16): solo per una bozza con un registro d'importazione.
function riquadroProposta() {
  if (!bozzaLavorabile() || !S.bozza.importazione) return null;
  const no = perchéNoModifica();
  const reg = registroBozza();
  const m = reg ? mancantiDellaProposta(reg) : null;
  const n = m ? m.voci.length + m.archi.length : 0;
  const stato = !reg ? "Il registro della bozza non è letto: ↻ Aggiorna." : n === 0 ? "La bozza contiene tutta la proposta dello STEP." :
    n === 1 ? "1 elemento della proposta dello STEP non è nella bozza." : n + " elementi della proposta dello STEP non sono nella bozza.";
  return h("div", { class: "du-riq du-riq--prop", id: "du-proposta-step" },
    h("div", { class: "du-riq-testa" }, h("h3", {}, "Proposta dello STEP"), h("span", { class: "du-muto du-piccolo", id: "du-proposta-stato" }, stato)),
    h("div", { class: "du-azioni" },
      btn("Importa dalla proposta…", apriImporta, { cls: "du-btn--sm", id: "du-importa", perche: false, no: no || (reg ? "" : "il registro della bozza non è letto: ↻ Aggiorna"),
        t: "Porta nella bozza solo gli elementi della proposta che scegli" }),
      btn("Riparti dalla proposta…", apriRiparti, { cls: "du-btn--sm du-btn--ghost", id: "du-riparti", perche: false, no: no || perchéNoRiparti(),
        t: "Sostituisce la bozza con la proposta dello STEP; la versione registrata non cambia" })));
}
// mancantiDellaProposta: le voci del registro che la bozza non ha (né come nodo, né come «non includere», che si
// ripristina con «Includi di nuovo») e i legami della proposta fra due nodi che la bozza ha senza quel legame.
function mancantiDellaProposta(reg) {
  const presenti = new Map();
  for (const n of S.lavoro.nodi) {
    const c = casellaDi(n);
    if (c) presenti.set(c.rif, n.chiave_bozza);
  }
  const decisi = new Set(S.lavoro.decisioni.filter((d) => d.tipo === "non_includere" && d.casella).map((d) => d.casella.rif));
  const radice = reg.gruppo && reg.gruppo.radice;
  const voci = arr(reg.proposta && reg.proposta.nodi).filter((v) => v && v.origine && v.origine.rif && v.origine.rif !== radice && !presenti.has(v.origine.rif) && !decisi.has(v.origine.rif));
  const archi = arr(reg.proposta && reg.proposta.archi).filter((a) => presenti.has(a.padre) && presenti.has(a.figlio) &&
    !S.lavoro.archi.some((x) => x.padre === presenti.get(a.padre) && x.figlio === presenti.get(a.figlio)));
  return { presenti, voci, archi };
}
function nomeVoce(v) {
  return (v && (v.codice_proposto || (v.grezzi && (v.grezzi.nome || v.grezzi.id)))) || (v && v.origine && v.origine.rif) || "?";
}
// «Importa dalla proposta…» (MAT BZ-15): selettivo, nessun elemento preselezionato. I nodi portano la casella e
// l'identità delle regole della partenza (nodoDaVoce); i legami la quantità dello STEP. Niente arriva alla working:
// è un salvataggio della bozza come gli altri.
function apriImporta() {
  const reg = registroBozza();
  if (!reg) return avvisa("errore", "Il registro della bozza non è letto: ↻ Aggiorna.");
  const { presenti, voci, archi } = mancantiDellaProposta(reg);
  const codice = (rif) => {
    const k = presenti.get(rif);
    const n = k && S.lavoro.nodi.find((x) => x.chiave_bozza === k);
    return (n && n.identita && n.identita.codice) || (n && n.descrizione) || rif;
  };
  const righe = [];
  for (const v of voci) {
    const registrato = voceNonImportabile(v);
    const stato = v.stato_motore && v.stato_motore.tolto ? "tolto nella proposta" : v.stato_motore && v.stato_motore.raggiunto === false ? "non raggiunto" : "manca nella bozza";
    righe.push(h("label", { class: "du-spunta", title: registrato ? MOTIVO_VOCE_REGISTRATA : null },
      h("input", { type: "checkbox", "data-rif": v.origine.rif, disabled: registrato }), h("span", {}, h("span", { class: "du-mono" }, nomeVoce(v)), " · " + stato + (registrato ? " · componente registrato: non importabile da qui" : ""))));
  }
  for (const a of archi) {
    righe.push(h("label", { class: "du-spunta" }, h("input", { type: "checkbox", "data-padre": a.padre, "data-figlio": a.figlio }),
      h("span", {}, "legame ", h("span", { class: "du-mono" }, codice(a.padre)), " → ", h("span", { class: "du-mono" }, codice(a.figlio)), " ×" + a.quantita + " (quantità dello STEP)")));
  }
  apriFinestra("Importa dalla proposta", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "Elementi della proposta che la bozza non ha. Scegli quali portare nella bozza: nessuno è preselezionato, e le decisioni della bozza non si toccano. I nodi non inclusi si ripristinano con «Includi di nuovo»."),
    righe.length ? h("div", { class: "du-voci-sel", id: "f-importa" }, righe) : h("p", { class: "du-muto du-piccolo" }, "La bozza contiene già tutto quello che la proposta dà.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Porta nella bozza", pri: true, id: "f-ok", spento: () => ($("#du-finestra").querySelectorAll("#f-importa input:checked").length ? "" : "scegli almeno un elemento"), fai: () => {
    const f = $("#du-finestra");
    const scelteVoci = new Set([...f.querySelectorAll("#f-importa input[data-rif]:checked")].map((x) => x.dataset.rif));
    const scelteArchi = new Set([...f.querySelectorAll("#f-importa input[data-padre]:checked")].map((x) => x.dataset.padre + ">" + x.dataset.figlio));
    const gruppo = reg.gruppo && reg.gruppo.chiave;
    const azioni = [];
    const nuovi = new Map();
    for (const v of voci) {
      if (!scelteVoci.has(v.origine.rif) || voceNonImportabile(v)) continue;
      const k = nuovaChiaveNodo();
      nuovi.set(v.origine.rif, k);
      azioni.push({ azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodoDaVoce(v, k, { allegato_id: v.origine.allegato_id, gruppo, rif: v.origine.rif }), origine: "utente" });
    }
    const chiave = (rif) => presenti.get(rif) || nuovi.get(rif) || "";
    for (const a of arr(reg.proposta && reg.proposta.archi)) {
      const nuovo = nuovi.has(a.padre) || nuovi.has(a.figlio);
      if (!nuovo && !scelteArchi.has(a.padre + ">" + a.figlio)) continue;
      const p = chiave(a.padre), fi = chiave(a.figlio);
      if (!p || !fi) continue;
      azioni.push({ azione: "arco_aggiunto", chiave_bozza: fi, prima: null, dopo: { padre: p, figlio: fi, qta: a.quantita, origine: "step", step: { allegato_id: a.allegato_id, gruppo } }, origine: "utente" });
    }
    if (!azioni.length) return false;
    fai("importati dalla proposta " + nuovi.size + " nodi e " + (azioni.length - nuovi.size) + " legami", azioni);
  } }]);
}
// perchéNoRiparti: «Riparti dalla proposta…» vuole il gruppo corrente della sorgente sulla stessa copia, con la sua
// impronta di oggi (EM §6.8, §6.4.1; /dati bozza_del_prodotto e gruppi).
function perchéNoRiparti() {
  const bp = S.modello && S.modello.bozzaDelProdotto;
  const sorg = S.salvato && S.salvato.sorgente;
  if (!ogg(sorg) || !sorg.allegato_id) return "la bozza non viene da uno STEP: si parte dalla proposta dalla Sorgente";
  if (!bp || bp.bozza_id !== S.bozza.bozza_id || !ogg(bp.gruppo_corrente)) return "il modello non dice il gruppo corrente della sorgente: ↻ Aggiorna";
  if (bp.gruppo_corrente.come !== "stessa_occorrenza") return "la struttura della bozza non è più proposta su questa copia: usa «Usa nuovo STEP…» dalla Sorgente";
  const gr = S.modello.gruppi.find((x) => x.chiave === bp.gruppo_corrente.chiave);
  if (!gr || !gr.impronta_contenuto) return "il modello non dà l'impronta della proposta di oggi";
  return "";
}
// «Riparti dalla proposta…» (MAT BZ-16; EM §6.8): la forma crea con «abbandona», in una transazione. Niente passa dalla
// bozza di adesso alla nuova: il dialogo lo dice.
async function apriRiparti() {
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "«Riparti dalla proposta» non parte: le modifiche della bozza non sono salvate.");
  }
  const no = perchéNoRiparti();
  if (no) return avvisa("no", "«Riparti dalla proposta» non parte: " + no + ".");
  const bp = S.modello.bozzaDelProdotto;
  const sorg = S.salvato.sorgente;
  // la proposta di oggi del gruppo corrente (S6): la sua impronta, e il confronto della radice con il prodotto (MAT
  // SG-05: è una creazione con partenza «step»; dal messaggio 46 [U] il motivo lo scrive il server)
  let a;
  try {
    a = await P.anteprimaStep(sorg.allegato_id, S.prodotto, { gruppo: bp.gruppo_corrente.chiave });
  } catch (e) {
    return avvisa("errore", "La proposta di oggi della sorgente non si legge: " + testoErrore(e));
  }
  const gr = arr(a.gruppi).find((x) => x.gruppo === bp.gruppo_corrente.chiave) || null;
  if (a.errore || a.stato !== "pronta" || !gr || !gr.impronta_contenuto) {
    return avvisa("no", "«Riparti dalla proposta» non parte: la proposta di oggi della sorgente non è disponibile" + (a.errore ? " (" + a.errore + ")" : "") + ".");
  }
  const diverso = a.confronto !== "coincide";
  const letta = (gr.radice && gr.radice.identita_proposta && gr.radice.identita_proposta.letterale) || "";
  apriFinestra("Riparti dalla proposta", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La bozza di adesso si chiude (resta nel registro delle bozze) e se ne apre una nuova, costruita dal server con la proposta di oggi della stessa sorgente. La versione registrata della BOM non cambia."),
    h("p", { class: "du-nota" }, "Niente della bozza di adesso passa alla nuova" + (S.confrontoNo ? "" : "; per tenere il lavoro usa «Aggiorna proposta…»") + ". Le preassociazioni non passano alla bozza nuova."),
    // collaudo UI46, giro 3 (minore): il codice della radice non letto si dice «non letto», non «(?)»
    diverso ? h("p", { class: "du-nota", id: "f-radice-diversa" }, letta ? ["Il codice letto dalla radice dello STEP (", h("span", { class: "du-mono" }, letta), ") non è quello del prodotto ("] :
      ["Il codice della radice dello STEP non è letto: nella bozza nuova vale il codice del prodotto ("], h("span", { class: "du-mono" }, a.codice_prodotto || codiceProdotto()),
    letta ? "): nella bozza nuova vale il codice del prodotto." : ").") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Riparti dalla proposta", danger: true, id: "f-ok", fai: async () => {
    const crea = { partenza: "step", sorgente: { allegato_id: sorg.allegato_id, sha256: sorg.sha256, gruppo: bp.gruppo_corrente.chiave }, impronta_contenuto: gr.impronta_contenuto,
      abbandona: { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione } };
    const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea });
    return dopoFormaServer(r, "Bozza ripartita dalla proposta dello STEP: quella di prima è abbandonata.");
  } }]);
}
// preassocia, togliPreassociazione, lascia: i gesti dei file sul pannello, sull'Elenco e sul vassoio, per allegato_id
// (direttiva 35, C-2; provvisorio, base [T] D-5: il vocabolario 2 emendato sul posto)
// imp: il registro d'importazione che propone il file su quel nodo, solo per un suggerimento del server accettato (D-PB03
// A [U]): va nello snapshot della PB nuova (le azioni restano {allegato_id, chiave_bozza}), e il server, con il registro
// dichiarato e corrispondente, dice la PB «motore»; la pagina non scrive mai la provenienza.
function preassocia(aid, k, imp) {
  if (!aid || !k) return;
  const azioni = azioniPreassocia(aid, k);
  if (!azioni.length) return avvisa("no", nomeFile(aid) + " è già preassociato a quel nodo.");
  const locali = typeof imp === "string" && imp ? azioni.map((a) => (a.azione === "file_preassociato" ? Object.assign({}, a, { dopo: Object.assign({}, a.dopo, { importazione: imp }) }) : a)) : null;
  fai("preassociato " + (fileDi(aid) ? fileDi(aid).nome_file : "un file"), azioni, locali ? { locali } : undefined);
}
function togliPreassociazione(aid, k) {
  fai("tolta la preassociazione", [{ azione: "file_svincolato", chiave_bozza: k, prima: { allegato_id: aid, chiave_bozza: k }, dopo: { allegato_id: aid, motivo: "utente" }, origine: "utente" }]);
}
function lascia(aid) {
  fai("lasciato senza associazione", [{ azione: "file_lasciato", chiave_bozza: null, prima: null, dopo: { allegato_id: aid }, origine: "utente" }]);
}
function apriScollega(f, v) {
  apriFinestra("Scollegare " + f.nome_file + " dal componente?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Lo scollegamento si decide nella bozza e si applica con «Conferma BOM». Il file resta nella RFQ: non è uno scarto e non va in Altri file.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scollega alla conferma", pri: true, id: "f-ok", fai: () => {
    const dec = { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", allegato_id: f.allegato_id, componente_id: v.nodo.componente_id };
    fai("da scollegare " + f.nome_file, [{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" }]);
  } }]);
}
// «Adotta come codice…» (MAT PN-08; EM §3.6.1): un'alternativa (dal messaggio 46 [U] senza motivo), che cita la lettura (chiave_lettura e la sua
// copia compatta) o il codice manuale (riferimento manuale_legacy). Il codice è la proiezione della lettura, la rev è
// rev_proposta (introdotta dal registro, in maiuscolo) o quella del codice manuale, copiata byte per byte (P-18).
function apriAdotta(v, r, ad) {
  const lettura = ad.lettura || null;
  const proiezione = ad.codice;
  apriFinestra("Adotta come codice", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'identità decisa di " + (v.cod || "questo pezzo") + " diventa ", h("span", { class: "du-mono" }, proiezione + (ad.rev ? " rev " + ad.rev : "")), " (" + (lettura ? lettura.emittente : r.tipo === "manuale_legacy" ? "cliente, presunto" : "?") + "). L'identità proposta resta com'era.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Adotta", pri: true, id: "f-ok", fai: () => {
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "alternativa", importazione: S.bozza.importazione ? S.bozza.importazione.impronta : undefined };
    if (r.tipo === "manuale_legacy") {
      dec.riferimento = "manuale_legacy";
      dec.identita = { emittente: emittenteCliente(), codice: proiezione };
    } else {
      dec.chiave_lettura = lettura.chiave_lettura;
      dec.lettura = copiaLettura(lettura);
      dec.identita = { emittente: lettura.emittente, codice: proiezione };
      if (lettura.namespace) dec.identita.namespace = lettura.namespace;
    }
    if (ad.rev) dec.identita.rev = ad.rev;
    const dopo = Object.assign({}, dec.identita, { modo: "decisione", decisione: dec.id });
    if (r.tipo === "manuale_legacy") dopo.emittente_presunto = true;
    // una decisione identita di prima dello stesso nodo si toglie nello stesso gesto (togliIdentitaPrecedenti)
    fai("adottato " + proiezione, togliIdentitaPrecedenti(v.k).concat([{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(v.nodo.identita) }, dopo: { identita: dopo }, origine: "utente" }]));
  } }]);
}
// «Fissa l'emittente…» di un registrato «cliente, presunto»: una decisione identita di correzione con il codice e la rev
// del nodo. La rev passa byte per byte solo se il nodo resta riusa o esclude (P-18, revMinuscoleAmmesse): con un altro op
// la decisione di correzione la fa nuova, e una rev con minuscole ASCII spegne «Fissa» con il motivo invece di finire in
// un 422 del server.
function apriFissaEmittente(v) {
  const reg = registroBozza();
  const partner = arr(reg && reg.regole && reg.regole.partner).map((p) => p.emittente).filter(Boolean);
  const sel = h("select", { id: "f-emittente" }, [emittenteCliente()].concat(partner).map((e) => h("option", { value: e }, e)));
  const rev = (v.nodo.identita && v.nodo.identita.rev) || "";
  const noRev = minuscoleAscii(rev) && !revMinuscoleAmmesse(v.nodo.op, { modo: "decisione", decisione: "" }) ?
    "la rev «" + rev + "» ha minuscole: con l'emittente fissato la rev entra in una decisione di correzione e va senza minuscole; correggila prima con «✎ Correggi codice, rev e descrizione…»" : "";
  apriFinestra("Fissa l'emittente di " + v.cod, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il componente è «cliente, presunto»: nessuno ha scelto l'emittente. Se in realtà è un codice del fornitore, fissalo: la conferma lo certificherà."),
    campo("Emittente", sel)),
  [{ testo: "Annulla", chiudi: true }, { testo: "Fissa", pri: true, id: "f-ok", spento: () => noRev, fai: () => {
    const f = $("#du-finestra");
    const em = valore(f, "f-emittente");
    const id = v.nodo.identita || {};
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "correzione", identita: { emittente: em, codice: id.codice } };
    if (id.rev) dec.identita.rev = id.rev;
    if (em.indexOf("partner:") === 0 && S.bozza.importazione) dec.importazione = S.bozza.importazione.impronta;
    fai("emittente fissato per " + v.cod, togliIdentitaPrecedenti(v.k).concat([{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(id) }, dopo: { identita: Object.assign({}, dec.identita, { modo: "decisione", decisione: dec.id }) }, origine: "utente" }]));
  } }]);
}
function apriRebase() {
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "Il rebase non parte: " + no + ".");
  const b = S.bozza;
  const vc = b.versione_corrente || {};
  apriFinestra("Rebase della bozza", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La base della bozza diventa quella di adesso" + (vc.numero ? " (conferma n. " + vc.numero + ")" : "") + ". Sorgente e registro non cambiano; le decisioni restano."),
    arr(b.rif_scaduti).length ? h("p", { class: "du-nota" }, "Riferimenti scaduti da sistemare: " + arr(b.rif_scaduti).map((r) => r.chiave_bozza || r.decisione).join(", ")) : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Fai il rebase", pri: true, id: "f-ok", fai: () => {
    const base = { conferma_id: vc.conferma_id === undefined ? null : vc.conferma_id, numero: vc.numero || 0, contatore: vc.contatore || 0, firma_struttura: vc.firma_struttura };
    const prima = { base: { conferma_id: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.conferma_id : null, numero: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.numero : 0,
      contatore: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.contatore : 0, firma_struttura: S.lavoro.firma_base } };
    fai("rebase sulla base corrente", [{ azione: "rebase", chiave_bozza: null, prima, dopo: { base, rif_scaduti: arr(b.rif_scaduti) }, origine: "utente" }], { rebase: { base } });
  } }]);
}

// ---------------------------------------------------------------- forme di R2 costruite dal server

function corpoForma(extra) {
  return Object.assign({ bozza_id: S.bozza ? S.bozza.bozza_id : null, versione_attesa: S.bozza ? S.bozza.versione : 0, prodotto_id: S.prodotto, vocabolario: 2 }, extra);
}
async function dopoFormaServer(r, testoOk) {
  if (r.esito === "ok") {
    const testo = testoOk + (r.dati && r.dati.riproposto ? " (riproposto: era già registrato)" : "");
    avvisa("ok", testo);
    S.ultimaForma = r.dati && r.dati.importazione ? r.dati.importazione : null;
    // dopo le forme costruite dal server si rileggono il modello e R1 (MAT §1.5); se non riesce, «da rileggere» (§1.3)
    const riletta = await rileggiDopoUnGesto(["bozza"], testoOk);
    // la proposta in vista nella Sorgente porta l'effetto sulla bozza di prima (S6 con «bozza»): si rilegge
    if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
    if (!riletta) avvisa("ok", testo + " La pagina non si è riletta: ↻ Rileggi.");
    render();
    return true;
  }
  return esitoFormaKo(r);
}
// esitoFormaKo: il rifiuto di una forma costruita dal server (MAT §5.4, riga R2). Torna true quando il dialogo va chiuso:
// con ANTEPRIMA_SUPERATA, gruppo_su_altra_occorrenza e stessa_sorgente il dialogo aperto porta un'impronta o un gruppo
// che non valgono più, e ripetere da lì fallirebbe allo stesso modo; la pagina rilegge la proposta e il gesto si rifà
// dal pulsante. Negli altri casi (motivo mancante, versione, motore occupato) il dialogo resta aperto con il motivo.
function esitoFormaKo(r) {
  const err = $("#du-finestra-errore");
  let testo = r.testo || "Il server non ha accettato il gesto.";
  let chiudi = false;
  if (r.codice === "ANTEPRIMA_SUPERATA") {
    // il 409 di R2 (C-3): anteprima (che cosa è cambiato), impronta_contenuto (quella di adesso), gruppo_corrente
    // ({chiave} della sorgente di oggi): si rivede l'anteprima sul gruppo di adesso
    const d = r.dati || {};
    const gc = d.gruppo_corrente;
    testo = "La proposta è cambiata mentre guardavi" + (d.anteprima && d.anteprima !== "proposta_step" ? " (" + d.anteprima + ")" : "") + ": rivedila, poi ripeti il gesto.";
    chiudi = true;
    if (S.anteprimaAllegato) guarda(S.anteprimaAllegato, gc && gc.chiave ? gc.chiave : "", true);
  } else if (r.codice === "FONTE_NON_CONFERMATA") {
    // C-3, U2 §2.8: lo STEP non è il documento cad_3d confermato del prodotto; la strada è la conferma della sorgente (S8)
    testo = "Lo STEP non è confermato come sorgente del prodotto: confermalo dalla Sorgente («Conferma questo STEP come sorgente»), poi riprova.";
  } else if (r.codice === "CORPO_NON_VALIDO" && r.motivo === "partenza_non_ammessa") {
    testo = haConfermaFormato3() ? "Il prodotto ha una BOM confermata: si parte da quella, con «Modifica la BOM confermata»." :
      sorgenteValida(S.prodotto) ? "Lo STEP sorgente del prodotto è confermato: la bozza nasce da lì («Riprendi: crea la bozza» nella Sorgente)." : "Questa partenza non è ammessa per il prodotto: ↻ Aggiorna.";
  } else if (r.motivo === "gruppo_su_altra_occorrenza" && r.dati && r.dati.allegato_id) {
    testo = "Questo gruppo sta su un'altra copia dello stesso file: guarda la proposta su quella copia.";
    chiudi = true;
    guarda(r.dati.allegato_id, "", true);
  } else if (r.motivo === "stessa_sorgente") {
    testo = S.confrontoNo ? "È la struttura della bozza." : "È la struttura della bozza: per prendere la proposta di oggi usa «Aggiorna proposta…».";
    chiudi = true;
  } else if (r.codice === "BOZZA_VERSIONE") {
    testo = r.motivo === "bozza_chiusa" ? "La bozza risulta già chiusa: la rileggo." : "La bozza è cambiata altrove: la rileggo; poi ripeti il gesto.";
    chiudi = true;
    rileggiTenendoDaParte();
  } else if (r.codice === "CALCOLO_OCCUPATO" || r.codice === "CALCOLO_SCADUTO" || r.codice === "NON_CONFIGURATO") {
    testo = "Il motore è occupato, riprova fra poco: niente è stato scritto.";
  } else if (r.codice === "PONTE_RETE" || r.codice === "PONTE_RISPOSTA" || /^PONTE_ERRORE_5/.test(r.codice || "")) {
    // una rete caduta o un 5xx su una forma di R2: l'esito è da verificare. Lo stesso gesto rimanda lo stesso corpo con la
    // stessa chiave_salvataggio: se il primo era arrivato, R2 risponde «riproposto» e niente si duplica (EM §9.2)
    testo = "Esito da verificare: ripeti lo stesso gesto (stessa chiave: se era arrivato, il server lo dice «riproposto») oppure ↻ Aggiorna.";
  }
  const conCodice = testo + (r.codice ? " (" + r.codice + (r.motivo ? ", " + r.motivo : "") + ")" : "");
  if (err && !chiudi) err.textContent = conCodice;
  avvisa(r.esito === "conflitto" ? "conflitto" : r.esito === "no" ? "no" : "errore", chiudi ? conCodice : testo);
  return chiudi;
}
function sorgenteDi(a, gr) {
  return { allegato_id: a.allegato_id, sha256: a.sha256, gruppo: gr.gruppo };
}
// rileggiTenendoDaParte: dopo un 409 BOZZA_VERSIONE di una forma di R2 o di R3 (MAT §5.4: «rilettura di R1»), R1 si
// rilegge; eventuali modifiche non salvate si mettono da parte, non si perdono (MAT §5.1).
async function rileggiTenendoDaParte() {
  clearTimeout(timerSalva);
  const voci = (S.invio ? S.invio.voci : []).concat(S.coda);
  const origine = origineDaParte();
  S.coda = [];
  S.invio = null;
  await caricaModello();
  await caricaBozza();
  if (voci.length) mettiDaParte(voci, "conflitto", "", origine);
  render();
}
// voceEffetto: un elemento di un elenco dell'effetto di S6 (o della risposta di cambia_sorgente): una chiave della bozza,
// oppure un oggetto con chiave, voce o decisione e il motivo. Il nome è il codice del nodo, se la bozza lo ha, o se lo ha
// la proposta in vista (gp, il grafo di grafoProposta: i nodi che entrano non sono ancora nella bozza). La chiave della
// bozza non si mostra mai: è un identificativo interno (revisione della corsia SORGENTE del messaggio 46, rilievo 1 [T]).
function voceEffetto(x, gp) {
  const k = typeof x === "string" ? x : ogg(x) ? x.chiave_bozza || x.decisione_id || x.voce || "" : String(x);
  const n = S.lavoro && S.lavoro.nodi.find((y) => y.chiave_bozza === k);
  const q = gp && gp.nodi && typeof k === "string" ? gp.nodi[k] : null;
  const nome = (n && n.identita && n.identita.codice) || (n && n.descrizione) || (q && (q.cod || q.desc)) || "un pezzo senza codice";
  // le preassociazioni nel vassoio citano l'allegato (direttiva 35, C-2): il nome del file davanti al nodo
  const file = ogg(x) && x.allegato_id ? nomeFile(x.allegato_id) + " → " : "";
  return file + nome + (ogg(x) && x.motivo ? " · " + x.motivo : "");
}
async function apriDifferenze() {
  avvisa("ok", "Calcolo delle differenze…");
  try {
    const j = await P.leggiBozza(S.prodotto, { confronto: true });
    const c = j.confronto || {};
    if (c.errore) return avvisa("errore", "Il calcolo delle differenze non è riuscito: " + c.errore);
    const d = c.differenze || {};
    apriFinestra("Differenze con la proposta di oggi", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "Esito: " + (c.esito || "?") + "."),
      h("ul", { class: "du-elenco-p", id: "f-differenze" }, arr(d.nodi).map((x) => h("li", {}, testoDifferenza(x))),
        arr(d.archi).map((x) => h("li", {}, "legame " + (x.padre || "") + " → " + (x.figlio || "") + ": " + (x.tipo || ""))))), [{ testo: "Chiudi", chiudi: true }]);
    avvisa("ok", "Differenze calcolate.");
  } catch (e) {
    avvisa("errore", "Le differenze non si leggono: " + testoErrore(e));
  }
}
async function apriRiallinea() {
  // come ogni modifica della bozza: ferma con una conferma in sospeso, con la BOM congelata, con un conflitto (MAT §5.2)
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "L'aggiornamento non parte: " + no + ".");
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "L'aggiornamento non parte: le modifiche della bozza non sono salvate.");
  }
  let c;
  try {
    c = (await P.leggiBozza(S.prodotto, { confronto: true })).confronto || {};
  } catch (e) {
    return avvisa("errore", "Il confronto con la proposta di oggi non si legge: " + testoErrore(e));
  }
  if (c.errore) return avvisa("errore", "Il calcolo della proposta non è riuscito: " + c.errore);
  if (c.esito === "altra_occorrenza") return avvisa("no", "La BOM di lavoro sta su un'altra copia dello stesso file: per seguirla usa «Usa nuovo STEP…».");
  if (c.esito === "gruppo_non_proposto") return avvisa("no", "Il Motore A non propone più questa struttura: la bozza resta valida.");
  const assenti = arr(c.differenze && c.differenze.nodi).filter((x) => x.tipo === "assente" && (x.chiave_bozza || x.decisione_id));
  const scelte = h("div", { class: "du-voci-sel", id: "f-scelte" }, assenti.length ? assenti.map((x) => h("div", { class: "du-gesto" }, h("span", { class: "du-mono" }, x.chiave_bozza || x.decisione_id),
    h("select", { "data-k": x.chiave_bozza || "", "data-d": x.decisione_id || "" }, h("option", { value: "" }, "— scegli —"), h("option", { value: "togli" }, "Togli"),
      x.decisione_id ? null : h("option", { value: "tieni_senza_casella" }, "Tieni come nodo a mano")))) : h("p", { class: "du-muto du-piccolo" }, "Nessuna casella sparita."));
  apriFinestra("Aggiorna la bozza alla proposta di oggi", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il server prende la proposta di oggi per la stessa sorgente: i nodi non toccati si aggiornano e si elencano, quelli decisi restano e vanno «da rivedere» se la loro proposta è cambiata. Niente cambia da solo senza una scelta."),
    scelte),
  [{ testo: "Annulla", chiudi: true }, { testo: "Aggiorna", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    return [...f.querySelectorAll("#f-scelte select")].some((s) => !s.value) ? "scegli per ogni casella sparita" : "";
  }, fai: async () => {
    const f = $("#du-finestra");
    const sc = [...f.querySelectorAll("#f-scelte select")].map((s) => (s.dataset.k ? { chiave_bozza: s.dataset.k, scelta: s.value } : { decisione_id: s.dataset.d, scelta: s.value }));
    const r = await P.riallineaProposta(corpoForma({ riallinea_proposta: { impronta_contenuto: c.impronta_contenuto_corrente, scelte: sc } }));
    return dopoFormaServer(r, "Bozza aggiornata alla proposta di oggi.");
  } }]);
}
async function creaDa(partenza) {
  const no = perchéNoPartenza(partenza);
  if (no) return avvisa("no", "La bozza non parte: " + no + ".");
  const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea: { partenza } });
  if (r.esito !== "ok") return esitoFormaKo(r);
  await dopoFormaServer(r, partenza === "conferma" ? "Bozza aperta dalla BOM confermata." : "Bozza aperta dalla BOM registrata.");
}
function apriConverti() {
  const no = perchéNoFormaBozza();
  if (no) return avvisa("errore", "La conversione non parte: " + no + ".");
  apriFinestra("Converti la bozza", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "La bozza di prima si chiude e se ne apre una nuova con la stessa struttura; i codici scritti restano come decisioni dell'ingegnere.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Converti", pri: true, id: "f-ok", spento: () => perchéNoFormaBozza(), fai: async () => {
    const r = await P.convertiBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, converti_da: { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione } });
    // la conversione traduce le preassociazioni da proposta_id ad allegato_id e segnala quelle su file non attivi o che
    // non si preassociano (direttiva 35, C-2): copiate nella bozza nuova, il riepilogo le dirà (R4)
    const d = (r.esito === "ok" && r.dati) || {};
    const seg = arr(d.preassociazioni_segnalate);
    const tr = Number(d.preassociazioni_tradotte) || 0;
    return dopoFormaServer(r, "Bozza convertita." + (tr ? " Preassociazioni riportate: " + tr + "." : "") +
      (seg.length ? " Da rivedere: " + seg.map((x) => nomeFile(x.allegato_id) + " (" + (x.motivo === "file_non_attivo" ? "file non attivo" : x.motivo === "natura_non_preassociabile" ? "non si preassocia" : x.motivo || "?") + ")").join(", ") + "." : ""));
  } }]);
}
// «Abbandona la bozza…» (MAT BZ-19, R3): spento con il motivo come le modifiche (perchéNoFormaBozza). Le modifiche non
// salvate (in coda o da parte) si perdono con la bozza: il dialogo le elenca. R3 passa dalla coda della risorsa della
// bozza: aspetta il salvataggio in volo (MAT §1.3).
function apriAbbandona() {
  const no = perchéNoFormaBozza();
  if (no) return avvisa("errore", "L'abbandono non parte: " + no + ".");
  const perse = S.coda.map((v) => v.etichetta).concat(S.daParte ? S.daParte.voci.map((v) => v.etichetta) : []);
  apriFinestra("Abbandona la bozza", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "La bozza viene chiusa e non si riapre: resta nel registro delle bozze. Le preassociazioni spariscono con lei; le associazioni registrate non cambiano."),
    perse.length ? h("p", { class: "du-nota", id: "f-perse" }, "Queste modifiche non salvate si perdono con la bozza: " + perse.join("; ") + ".") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Abbandona la bozza", danger: true, id: "f-ok", spento: () => perchéNoFormaBozza(), fai: async () => {
    clearTimeout(timerSalva);
    const prodotto = S.prodotto;
    const r = await inSerie("bozza:" + prodotto, async () => {
      await attendi(() => !S.invio, 30000);
      const corpo = { versione_attesa: S.bozza.versione };
      return P.abbandonaBozza(S.bozza.bozza_id, corpo, prodotto);
    });
    if (r.esito !== "ok") return esitoFormaKo(r);
    S.coda = [];
    S.invio = null;
    S.daParte = null;
    // un tentativo «certo» della conferma (COPIA_IN_CORSO) di questa bozza non ha più senso: niente era registrato
    const sosp = confermaInSospeso();
    if (sosp && sosp.certo) P.abbandonaSospeso(sosp.risorsa);
    avvisa("ok", "Bozza abbandonata.");
    await rileggiDopoUnGesto(["bozza"], "La bozza è abbandonata.");
    render();
    return true;
  } }]);
}

// ---------------------------------------------------------------- i gesti dei file (S2, S3, S4; EM §10.4-§10.6)

async function dettaglioFile(f) {
  try {
    const j = await P.leggiSorgente({ file: f.allegato_id, scartati: true });
    return j.dettaglio || {};
  } catch (e) {
    avvisa("errore", "Le dipendenze del file non si leggono: " + testoErrore(e));
    return null;
  }
}
function elencoDipendenze(det) {
  const dip = arr(det.dipendenze);
  if (!dip.length) return h("p", { class: "du-muto du-piccolo" }, "Nessuna dipendenza.");
  return h("ul", { class: "du-dip", id: "f-dipendenze" }, dip.map((d) => h("li", { "data-esito": d.esito || "mostra", "data-codice": d.codice || "" },
    h("b", {}, (d.codice || "") + (d.allegato_id && det.allegato_id && d.allegato_id !== det.allegato_id ? " · " + nomeFile(d.allegato_id) : "")), h("span", {}, d.testo || ""))));
}
function richiede(det, tipo) {
  return arr(det.dipendenze).filter((d) => d.esito === "risoluzione" && (d.risoluzione === tipo || d.tipo_risoluzione === tipo)).length;
}
function blocchiFile(det) {
  return arr(det.dipendenze).filter((d) => d.esito === "blocca");
}
// scollegaComposto: «Scollega e scarta» e «Scollega e metti in Altri file» nella RFQ formato 3 (D-ASS-03 / V1-Q42 [U] A;
// C-6b): il D-7 «blocca» di una AR senza lo svincolo in una bozza aperta si risolve con un gesto solo per chi guarda, in due
// effetti in ordine: prima lo svincolo {allegato_id, componente_id} salvato con R2 nella bozza aperta che ha il componente,
// poi S2 o S3 con «scollega» sull'elenco delle dipendenze riletto. La pagina lo offre solo se quella bozza è la bozza
// lavorabile del prodotto scelto e i D-7 sono gli unici blocchi; altrimenti D-7 resta un blocco con la strada scritta dal
// server. Restituisce gli svincoli da salvare, o null.
function scollegaComposto(det, f) {
  const bl = blocchiFile(det);
  const d7 = bl.filter((d) => d.codice === "D-7");
  if (!d7.length || d7.length !== bl.length || !bozzaLavorabile() || perchéNoModifica()) return null;
  const out = [];
  for (const d of d7) {
    const aid = d.allegato_id || f.allegato_id;
    const ar = arDi(aid);
    if (!ar || ar.documento) return null;
    const n = S.lavoro.nodi.find((x) => x.componente_id && String(x.componente_id).toLowerCase() === ar.componente_id);
    if (!n) return null;
    out.push({ allegato_id: aid, componente_id: n.componente_id, cod: (n.identita && n.identita.codice) || "" });
  }
  return out;
}
// salvaSvincoliComposti: il primo effetto del gesto composto, lo svincolo salvato nella bozza (R2); true se è salvato
async function salvaSvincoliComposti(sv, motivo) {
  const azioni = sv.filter((x) => !svincoloDi(x.allegato_id)).map((x) => {
    const dec = { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", allegato_id: x.allegato_id, componente_id: x.componente_id, motivo };
    return { azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" };
  });
  if (azioni.length && !fai("da scollegare " + sv.map((x) => nomeFile(x.allegato_id)).join(", "), azioni)) return false;
  return await salvaOra();
}
// gestoComposto: dopo lo svincolo salvato, il dettaglio riletto (le dipendenze sono cambiate: D-6 «scollega» al posto di
// D-7) e il corpo di S2 o S3 con «scollega» e l'impronta nuova. Se il secondo effetto non riesce, lo stato parziale si
// dice: lo svincolo resta nella bozza (D-ASS-03: stato parziale mostrato).
async function gestoComposto(f, sv, corpo, err) {
  if (!(await salvaSvincoliComposti(sv, "scollegato con il gesto sul file (D-ASS-03)"))) {
    if (err) err.textContent = "Lo svincolo non si è salvato nella bozza: niente è cambiato. Rileggi la bozza e riprova.";
    return null;
  }
  const det2 = await dettaglioFile(f);
  if (!det2) {
    if (err) err.textContent = "Lo svincolo è salvato nella bozza; le dipendenze del file non si rileggono: riprova il gesto.";
    return null;
  }
  if (blocchiFile(det2).length) {
    if (err) err.textContent = "Lo svincolo è salvato nella bozza, ma il file ha ancora dipendenze che bloccano: " + blocchiFile(det2).map((b) => b.codice).join(", ") + ".";
    return null;
  }
  return Object.assign({}, corpo, { impronta_dipendenze: det2.impronta_dipendenze || "", scollega: true });
}
function testoParziale(sv) {
  return "Lo svincolo di " + sv.map((x) => nomeFile(x.allegato_id) + (x.cod ? " da " + x.cod : "")).join(", ") + " è salvato nella bozza; il gesto sul file non è riuscito: ";
}
// «Riprova» di un gesto dei file con l'esito incerto (S2, S3, S4), dalla striscia: il ponte rimanda il corpo di
// allora con la stessa chiave, che tiene nella memoria della scheda anche dopo una ricarica (EM §10.9, «Reinvio e
// risposta persa»; MAT §5.4, righe S2-S4: «come R5»; §5.5 n. 4).
function fileDelSospeso(s) {
  const allegato = String(s.risorsa || "").slice("file:".length);
  return fileDi(allegato) || { allegato_id: allegato, nome_file: allegato.slice(0, 8) };
}
async function riprovaGestoFile(s) {
  const f = fileDelSospeso(s);
  avvisa("ok", "«" + (s.etichetta || s.nome) + "» su " + f.nome_file + ": lo rimando con lo stesso corpo e la stessa chiave…");
  const r = await inSerie(s.risorsa, () => P.riprovaSospeso(s.risorsa));
  await esitoGestoFile(r, f, "", { chiudi: true });
  renderStrisce();
}
// i testi dei 422 dei gesti dei file che hanno una frase della matrice (MAT §5.4)
const TESTI_FILE = {
  GIA_DOCUMENTO: "È già un documento confermato: per cambiarlo serve una revisione o una sostituzione, non da questa pagina.",
  NON_STEP: "Il file non è uno STEP: l'associazione al prodotto vale solo per lo STEP.",
  FILE_NON_ATTIVO: "Il file non è fra i file attivi della RFQ: riattivalo prima.",
  MOTIVO_OBBLIGATORIO: TESTO_MOTIVO_VECCHIO,
  RISOLUZIONE_RICHIESTA: "Manca una dichiarazione richiesta dalle dipendenze (l'unica copia o «Scollega»): rivedi le caselle del dialogo.",
  FILE_SENZA_BYTE: "Il file non ha ancora i byte (non scaricato): niente è scritto, si può riprovare dopo «Prepara i file».",
  FILE_SCARTATO: "Il file è stato scartato: per riaverlo, ricaricalo.",
  CONSERVAZIONE_NON_RIUSCITA: "La copia nella cartella ALTRO della RFQ non è riuscita: niente è cambiato.",
  CONSERVAZIONE_MANCANTE: "La copia in ALTRO non c'è più, o è stata cambiata: ricarica il file. Niente è cambiato.",
  VOCI_ATTIVE: "Lo ZIP ha file estratti ancora attivi: scartali o mettili in Altri file prima. Niente è cambiato.",
  RFQ_UNITA: "La RFQ è unita a un'altra: i gesti sui file si fanno sulla RFQ che la contiene.",
  NON_CONFIGURATO: "La scrittura sul NAS è spenta su questo server: la cartella ALTRO sta sul NAS. Niente è cambiato.",
};
// esitoGestoFile: l'esito di S2, S3 o S4 (MAT §5.4; EM §10.9). o.chiudi: il dialogo non c'è (un «Riprova» dalla
// striscia). Torna true quando il dialogo va chiuso.
async function esitoGestoFile(r, f, testoOk, o) {
  if (r.esito === "ok") {
    const dati = r.dati || {};
    // l'esito del gesto va nell'avviso; la riga del file mostra solo lo stato che S1 dice adesso (la rimozione o la
    // conservazione possono andare avanti dopo la risposta: una frase di allora la contraddirebbe)
    const rim = arr(dati.rimozioni).map((x) => x.testo).filter(Boolean).join(" ");
    const t = (rim || dati.testo || testoOk) + (dati.riproposto ? " (riproposto: era già registrato)" : "");
    // dopo un «ok» si rileggono S1 e il modello; se la rilettura non riesce, «da rileggere», mai «non riuscito» (MAT §1.3)
    const riletta = await rileggiDopoUnGesto(["sorgente"], "Il gesto su " + f.nome_file + " è registrato.");
    avvisa("ok", t + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."));
    render();
    return true;
  }
  if (r.esito === "da_verificare") {
    avvisa("da_verificare", "Esito da verificare: non so se il gesto è registrato. Verifico con la stessa chiave.");
    verificaConRiprova(r.chiave, async (j) => {
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        const riletta = await rileggiDopoUnGesto(["sorgente"], "Il gesto su " + f.nome_file + " è registrato.");
        avvisa("ok", "Verificato: il gesto su " + f.nome_file + " è registrato" + (j.stato === "registrato_senza_esito" ? " (l'esito di allora non è più conservato)" : "") + "." + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."));
        render();
        return true;
      }
      if (j.stato === "non_registrato") {
        const s = P.sospesi().find((x) => x.chiave === r.chiave);
        avvisa("no", "«" + (s ? s.etichetta || s.nome : "Il gesto") + "» su " + f.nome_file + " non è ancora registrato: «Riprova» lo rimanda con lo stesso corpo e la stessa chiave (se la richiesta di prima arriva, riceverai il suo esito); oppure abbandona il tentativo.");
        renderStrisce();
        return true;
      }
      return false;
    });
    renderStrisce();
    return true;
  }
  const err = o && o.chiudi ? null : $("#du-finestra-errore");
  let testo = TESTI_FILE[r.codice] || r.testo || "Il server non ha accettato il gesto.";
  let chiudi = false;
  if (r.codice === "VOCI_ATTIVE") {
    const vv = arr(r.dati && r.dati.voci);
    testo = "Lo ZIP ha " + (vv.length || "dei") + " file estratti ancora attivi" + (vv.length ? " (" + vv.map((v) => v.nome_file || nomeFile(v.allegato_id)).join(", ") + ")" : "") + ": scartali o mettili in Altri file prima. Niente è cambiato.";
  }
  if (r.codice === "FILE_VERSIONE") {
    // lo stato del file è cambiato: il dialogo si chiude e S1 si rilegge (MAT §5.4)
    const ora_ = r.dati && r.dati.stato ? r.dati.stato : "";
    testo = "Lo stato del file è cambiato" + (ora_ ? " (era " + (f.stato || "attivo") + ", ora " + ora_ + ")" : "") + ": rileggo la Sorgente.";
    chiudi = true;
    await caricaSorgente();
  } else if (r.codice === "COPIA_IN_CORSO") {
    // una copia di conservazione già in corso, o il tentativo chiuso da un'altra scheda: si rilegge S1 (MAT §5.4, S3).
    // Il tentativo resta «certo»: «Riprova» dalla striscia lo rimanda con la stessa chiave
    testo = r.motivo === "tentativo_chiuso" ? "Il tentativo è stato chiuso da un'altra scheda: rileggo." : "Una copia di conservazione è già in corso per questo file: riprova fra poco con la stessa chiave.";
    chiudi = true;
    await caricaSorgente();
  } else if (r.codice === "CHIAVE_RIUSATA") {
    testo = "Questo tentativo ha già un esito diverso: rileggo la Sorgente; il prossimo gesto userà una chiave nuova.";
    chiudi = true;
    await caricaSorgente();
  } else if (r.codice === "DIPENDENZE_ATTIVE") {
    // l'elenco, voce per voce, ciascuna con il suo rimedio (MAT §5.4): il dialogo resta aperto
    const dip = arr(r.dati && r.dati.dipendenze);
    testo = "Il file ha dipendenze attive: " + dip.map((d) => (d.allegato_id && d.allegato_id !== f.allegato_id ? nomeFile(d.allegato_id) + " · " : "") + (d.codice ? d.codice + ": " : "") + (d.testo || "")).join(" — ");
    if (err) {
      err.textContent = "";
      err.append(h("span", {}, "Il file ha dipendenze attive (DIPENDENZE_ATTIVE):"), h("ul", { class: "du-elenco-p", id: "f-dipendenze-attive" }, dip.map((d) =>
        h("li", { "data-codice": d.codice || "" }, (d.allegato_id && d.allegato_id !== f.allegato_id ? nomeFile(d.allegato_id) + " · " : "") + (d.codice ? d.codice + ": " : "") + (d.testo || "")))));
      avvisa("no", testo);
      return false;
    }
  }
  if (chiudi) {
    chiudiFinestra();
    render();
  } else if (err) err.textContent = testo + (r.codice ? " (" + r.codice + (r.motivo ? ", " + r.motivo : "") + ")" : "");
  avvisa(r.esito === "conflitto" ? "conflitto" : r.esito === "no" ? "no" : "errore", testo);
  return chiudi;
}
// mossaFile: dopo un gesto dei file riuscito (S2, S3, S4), la mossa: la riga fantasma nella cartella di partenza, il ±1
// sui conti, «Ultima mossa» con «Vai al file»; niente «Annulla la mossa» (si torna con «Riattiva…»; lo scarto è terminale).
// Il fuoco va sulla riga successiva, e il dettaglio la segue (T-11, 5.1).
function mossaFile(f, dove, prima, k0) {
  const esce = !contenuto(k0).some((y) => y.f.allegato_id === f.allegato_id);
  if (esce) prossimaRiga(k0, f.allegato_id);
  registraMossa(f.nome_file + " → " + dove, { allegato: f.allegato_id, contiPrima: prima, fantasma: esce ? { allegato: f.allegato_id, cartella: k0, dove } : null });
  if (S.passo === "sorgente") {
    renderCorpo();
    fuocoSuRiga(memo(k0).file);
  }
}
// riapriDopoAnteprima: il 409 ANTEPRIMA_SUPERATA di S2, S3 o S4 (dipendenze_file, voci_archivio): il dialogo si chiude,
// S1 e il dettaglio si rileggono e il dialogo si riapre con l'elenco nuovo; chi guarda rivede e ripete (MAT §5.4). Torna
// false: la finestra aperta adesso è quella nuova, e il pulsante del dialogo di prima non deve chiuderla.
async function riapriDopoAnteprima(f, apri) {
  chiudiFinestra();
  await caricaSorgente();
  render();
  const nuovo = fileDi(f.allegato_id);
  if (nuovo) await apri(nuovo);
  avvisa("conflitto", "Le dipendenze o il perimetro delle voci sono cambiati: rivedi il dialogo aggiornato e ripeti.");
  return false;
}
async function apriScarta(f) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const unica = richiede(det, "conferma_unica_copia");
  const sv = scollegaComposto(det, f);
  const scol = richiede(det, "scollega") + (sv ? sv.length : 0);
  const blocchi = sv ? [] : blocchiFile(det);
  const voci = arr(det.voci);
  apriFinestra("Scartare «" + f.nome_file + "»?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il file esce dai file attivi della RFQ e la sua copia gestita dal cockpit viene eliminata."),
    h("p", {}, "Resta la traccia: nome, SHA, origine, chi l'ha scartato e quando."),
    h("p", { id: "f-scarta-altro" }, "La mail originale, i documenti confermati e la storia non si toccano. Se il file è in Altri file, anche la sua copia nella cartella ALTRO della RFQ sul NAS viene eliminata. Non si annulla: per riaverlo dovrai ricaricarlo."),
    f.canale === "posta" ? h("p", { class: "du-nota" }, "La mail originale resta: per riaverlo, lo salvi dalla mail e lo ricarichi.") : null,
    f.stato === "altri_file" ? h("p", { class: "du-nota" }, "È in Altri file: si toglie solo la copia che il cockpit ha creato nella cartella ALTRO, e solo se nessuno l'ha cambiata.") : null,
    voci.length ? h("p", { class: "du-nota", id: "f-scarta-zip" }, "È un archivio: si scarta solo il contenitore. Se ha file estratti ancora attivi (" + voci.length + "), il server lo rifiuta: scartali o mettili in Altri file prima.") : null,
    elencoDipendenze(det),
    unica ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-unica" }), "Ho capito: è l'unica copia" + (unica > 1 ? " (" + unica + " file)" : "") + ". Scartandolo non ne resta nessuna: per riaverlo dovrò ricaricarlo.") : null,
    scol ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-scollega" }), "Scollega e scarta" + (scol > 1 ? " (" + scol + " file associati)" : "") + ": l'associazione al componente viene tolta.") : null,
    sv ? h("p", { class: "du-nota", id: "f-scollega-composto" }, "La RFQ è gestita con la Distinta nuova: prima lo svincolo si salva nella bozza di " + codiceProdotto() + " (" + sv.map((x) => nomeFile(x.allegato_id) + (x.cod ? " da " + x.cod : "")).join(", ") + "), poi lo scarto chiude l'associazione. Se lo scarto non riesce, lo svincolo resta nella bozza.") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scarta", danger: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    if (blocchi.length) return "il file ha dipendenze che bloccano: " + blocchi.map((b) => b.codice).join(", ");
    if (unica && !fin.querySelector("#f-unica").checked) return "serve la conferma dell'unica copia";
    if (scol && !fin.querySelector("#f-scollega").checked) return "serve «Scollega e scarta»";
    return "";
  }, fai: async () => {
    const fin = $("#du-finestra");
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0, impronta_dipendenze: det.impronta_dipendenze || "" };
    if (det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    if (scol) corpo.scollega = true;
    if (unica) corpo.conferma_unica_copia = true;
    let c2 = corpo;
    if (sv) {
      c2 = await gestoComposto(f, sv, corpo, $("#du-finestra-errore"));
      if (!c2) return false;
    }
    const prima = contiAlbero();
    const k0 = SG.cartella;
    const r = await inSerie("file:" + f.allegato_id, () => P.scartaFile(f.allegato_id, c2));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, apriScarta);
    const chiudi = await esitoGestoFile(r, f, "File scartato.");
    // D-ASS-03: lo stato parziale si dice per ultimo, così l'avviso non lo copre (lo svincolo resta nella bozza)
    if (sv && r.esito !== "ok" && r.esito !== "da_verificare") avvisa("no", testoParziale(sv) + (r.testo || r.codice || "rifiutato") + ".");
    if (r.esito === "ok") mossaFile(f, "Scartati", prima, k0);
    return chiudi;
  } }]);
}
// apriAltriFile: «Metti in Altri file…» (MAT FL-06; U2 §3): la copia va nella cartella ALTRO della RFQ sul NAS, e che cosa
// è confermato o soltanto conservato lo dice il database (C-02 B [U]). Per un archivio i gesti sono due (U2 §3.10, [U]
// Q21): perimetro «contenitore» («Metti lo ZIP originale in Altri file…», le voci restano attive) e
// «contenitore_e_voci» («Metti in Altri file anche i contenuti (N)…», con l'elenco delle voci e le loro impronte).
async function apriAltriFile(f, perimetro) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const sv = scollegaComposto(det, f);
  const scol = richiede(det, "scollega") + (sv ? sv.length : 0);
  const blocchi = sv ? [] : blocchiFile(det);
  const arch = eArchivio(f);
  const conVoci = arch && perimetro === "contenitore_e_voci";
  const voci = arr(det.voci);
  const fmt = formatoDi(f) || "ZIP";
  apriFinestra(arch ? (conVoci ? "Mettere in Altri file «" + f.nome_file + "» e i suoi " + voci.length + " file?" : "Mettere lo " + fmt + " originale «" + f.nome_file + "» in Altri file?") : "Mettere «" + f.nome_file + "» in Altri file?",
    h("div", { style: "display:grid;gap:8px" },
      h("p", { id: "f-altro" }, "Il file resta nella RFQ: la copia va nella cartella ALTRO della RFQ sul NAS. Stare lì non vuol dire che è un documento confermato: lo dice il database. Si apre e si scarica quando serve."),
      h("p", {}, "Non entra nella proposta della BOM e non riceve associazioni finché non lo riattivi."),
      arch && !conVoci ? h("p", { class: "du-nota", id: "f-altro-zip" }, "Le " + voci.length + " voci estratte restano attive e continuano a essere proposte. La copia dello " + fmt + " va nella cartella ALTRO della RFQ. Non sostituisce il backup del NAS.") : null,
      conVoci ? h("div", { id: "f-voci-altri" }, h("p", { class: "du-nota" }, "Vanno in Altri file anche le voci estratte (" + voci.length + "), con le loro dipendenze:"),
        h("ul", { class: "du-elenco-p" }, voci.map((v) => h("li", {}, h("span", { class: "du-mono" }, v.nome_file || nomeFile(v.allegato_id)), v.stato ? " · " + v.stato : "")))) : null,
      elencoDipendenze(det),
    scol ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-scollega" }), "Scollega e metti in Altri file" + (scol > 1 ? " (" + scol + " file associati)" : "") + ".") : null,
    sv ? h("p", { class: "du-nota", id: "f-scollega-composto" }, "La RFQ è gestita con la Distinta nuova: prima lo svincolo si salva nella bozza di " + codiceProdotto() + " (" + sv.map((x) => nomeFile(x.allegato_id) + (x.cod ? " da " + x.cod : "")).join(", ") + "), poi il gesto chiude l'associazione. Se non riesce, lo svincolo resta nella bozza.") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Metti in Altri file", pri: true, id: "f-ok", spento: () => {
    if (blocchi.length) return "il file ha dipendenze che bloccano: " + blocchi.map((b) => b.codice).join(", ");
    if (scol && !$("#du-finestra").querySelector("#f-scollega").checked) return "serve «Scollega e metti in Altri file»";
    return "";
  }, fai: async () => {
    const fin = $("#du-finestra");
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0, impronta_dipendenze: det.impronta_dipendenze || "" };
    // il perimetro (U2 §3.10): con «contenitore» n_voci e impronta_voci non ci sono; con «contenitore_e_voci» sì
    if (arch) corpo.perimetro = conVoci ? "contenitore_e_voci" : "contenitore";
    if ((!arch || conVoci) && det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    if (scol) corpo.scollega = true;
    let c2 = corpo;
    if (sv) {
      c2 = await gestoComposto(f, sv, corpo, $("#du-finestra-errore"));
      if (!c2) return false;
    }
    const prima = contiAlbero();
    const k0 = SG.cartella;
    const r = await inSerie("file:" + f.allegato_id, () => P.altriFile(f.allegato_id, c2));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, (x) => apriAltriFile(x, perimetro));
    const chiudi = await esitoGestoFile(r, f, "File messo in Altri file: la copia è nella cartella ALTRO della RFQ.");
    // D-ASS-03: lo stato parziale si dice per ultimo, così l'avviso non lo copre (lo svincolo resta nella bozza)
    if (sv && r.esito !== "ok" && r.esito !== "da_verificare") avvisa("no", testoParziale(sv) + (r.testo || r.codice || "rifiutato") + ".");
    if (r.esito === "ok") mossaFile(f, "Altri file", prima, k0);
    return chiudi;
  } }]);
}
// «Riattiva» (MAT FL-07, FL-10): per un archivio il dialogo dice quali voci tornano attive con lui, e S4 porta n_voci e
// impronta_voci dell'elenco che S1 ha mostrato (il dettaglio con file=; EM §10.9).
async function apriRiattiva(f, perimetro) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const voci = arr(det.voci);
  const arch = eArchivio(f);
  // un archivio: «Riattiva…» solo il contenitore, «Riattiva anche i contenuti…» con le voci (U2-P46, stessa forma di S3)
  const conVoci = !arch || perimetro === "contenitore_e_voci";
  apriFinestra("Riattivare «" + f.nome_file + "»?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Riportare «" + f.nome_file + "» fra i file attivi? Il motore lo rilegge e può proporlo di nuovo. Non conferma nessun documento e non autorizza nessuno STEP."),
    h("p", { class: "du-nota" }, "La copia nella cartella ALTRO resta dov'è: il file torna attivo dai suoi byte."),
    voci.length && conVoci ? h("div", { id: "f-voci-riattiva" }, h("p", { class: "du-nota" }, "È un archivio: tornano attive con lui le sue " + voci.length + " voci."),
      h("ul", { class: "du-elenco-p" }, voci.map((v) => h("li", {}, h("span", { class: "du-mono" }, v.nome_file || nomeFile(v.allegato_id)), v.stato ? " · " + v.stato : "")))) : null,
    voci.length && !conVoci ? h("p", { class: "du-nota", id: "f-riattiva-zip" }, "Torna attivo solo il contenitore: le sue " + voci.length + " voci in Altri file restano lì («Riattiva anche i contenuti…» è un gesto separato).") : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Riattiva", pri: true, id: "f-ok", fai: async () => {
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0 };
    if (arch) corpo.perimetro = conVoci ? "contenitore_e_voci" : "contenitore";
    if (conVoci && det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    const prima = contiAlbero();
    const k0 = SG.cartella;
    const r = await inSerie("file:" + f.allegato_id, () => P.riattivaFile(f.allegato_id, corpo));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, (x) => apriRiattiva(x, perimetro));
    const chiudi = await esitoGestoFile(r, f, "File riattivato.");
    if (r.esito === "ok") mossaFile(f, "In analisi", prima, k0);
    return chiudi;
  } }]);
}
// ---------------------------------------------------------------- gli atti esistenti (lista chiusa ridotta, EM §9.10)

// atto: uno dei tre atti della lista chiusa (EM §9.10; U2 §2.2; direttiva 39). Gli atti passano dalla coda della pagina (MAT §1.3): un atto
// chiesto mentre un altro è in volo parte quando il primo ha un esito, non si perde.
async function atto(nome, campi, u, etichetta) {
  const d = $("#du-menu-rfq");
  if (d) d.open = false;
  avvisa("ok", inVoloSu("atto") ? etichetta + ": in coda dopo l'atto in corso…" : etichetta + "…");
  return inSerie("atto", async () => {
    avvisa("ok", etichetta + "…");
    const r = await P.esegui(nome, campi, u);
    avvisa(r.esito === "ok" ? "ok" : r.esito === "no" ? "no" : "errore", (r.esito === "no" ? "Niente è cambiato: " : "") + r.testo);
    if (r.esito === "ok") {
      // dopo un atto della fonte si rileggono modello, file e bozza (la base può risultare superata); R1 non si rilegge con
      // modifiche non salvate (caricaBozza)
      await rileggiDopoUnGesto(["sorgente", "bozza"], "L'atto è registrato (" + etichetta + ").");
      render();
      if (S.daRileggere) avvisa("ok", r.testo + " La pagina non si è riletta: ↻ Rileggi.");
    }
    return r;
  });
}
// «Autorizza» e «Revoca l'autorizzazione» non sono più gesti di questa pagina (direttiva 39 §2; D-TAB-02 B rivista [T-39]):
// la fonte della working la promuove la conferma della BOM (R5); la revoca di uno STEP scelto per errore è
// apriRevocaSorgente.
async function riallineaDocumenti() {
  const r = await inSerie("f8", () => P.riallineaDocumenti());
  avvisa(r.esito === "ok" ? "ok" : "errore", r.esito === "ok" ? "Documenti riallineati (F8: " + ((r.dati && r.dati.f8_stato) || "?") + ")." : r.testo);
  await caricaModello();
  render();
}

// ---------------------------------------------------------------- i tentativi in sospeso (R6)

// verificaSospeso: «Verifica ora» (e la verifica all'apertura) di un tentativo in sospeso, con R6 e la stessa chiave. Su
// «registrato» si rilegge solo quello che il gesto tocca: per un gesto dei file S1 e il modello, per la conferma il
// modello, R1 e R7. R1 non si rilegge con modifiche non salvate (caricaBozza): un tentativo in sospeso su un file non
// ferma la bozza, e la verifica non deve buttare i gesti in coda (MAT §5.5 n. 7; §3.9, §5.1).
async function verificaSospeso(s) {
  try {
    const j = await P.esitoIntento(s.chiave);
    if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
      const testo = "Verificato: «" + (s.etichetta || s.nome) + "» è registrato" + (j.stato === "registrato_senza_esito" ? " (l'esito di allora non è più conservato)" : "") + ".";
      const riletta = s.nome === "confermaBom" ? await rileggiDopoUnGesto(["bozza", "storico"], "La conferma è registrata.") : await rileggiDopoUnGesto(["sorgente"], "Il gesto è registrato.");
      avvisa("ok", testo + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."));
      render();
      return;
    } else if (j.stato === "non_registrato") {
      avvisa("no", "«" + (s.etichetta || s.nome) + "» non è ancora registrato. Puoi rifarlo con lo stesso corpo e la stessa chiave: se la richiesta di prima arriva, riceverai il suo esito. Per un corpo diverso, abbandona prima il tentativo.");
    } else if (j.stato === "in_corso") {
      avvisa("ok", "Un gesto per questa RFQ è ancora in corso: verifico fra poco.");
      setTimeout(() => verificaSospeso(s), 3000);
    }
  } catch (e) {
    avvisa("errore", "Verifica non riuscita: " + testoErrore(e) + ". La chiave resta in sospeso: «Verifica ora» riprova.");
  }
  renderStrisce();
  renderBarra();
}
// riprovaConfermaSospesa: «Riprova la conferma» dalla striscia (dopo una ricarica il wizard di prima non c'è più): la
// Revisione nell'Organigramma e il wizard, che rilegge il riepilogo. Mai da sola: la conferma parte solo dal clic su
// «Conferma BOM» (MAT §5.2).
async function riprovaConfermaSospesa() {
  S.passo = "revisione";
  S.vista = "org";
  render();
  await apriWizard();
}
function abbandonaTentativo(s) {
  // un tentativo «certo» (COPIA_IN_CORSO): niente è registrato e niente è in volo, la richiesta di prima non arriverà
  const testo = s.certo ? "«" + (s.etichetta || s.nome) + "» non è registrato (una copia era in corso): togliendo il tentativo, la pagina permette un gesto nuovo con una chiave nuova."
    : "La richiesta di prima («" + (s.etichetta || s.nome) + "») potrebbe ancora arrivare: se arriva, la vedrai registrata. Abbandonando la chiave, la pagina permette un gesto nuovo con una chiave nuova.";
  apriFinestra(s.certo ? "Lasciare perdere il tentativo?" : "Abbandonare il tentativo?", h("p", {}, testo),
    [{ testo: "Annulla", chiudi: true }, { testo: s.certo ? "Lascia perdere" : "Abbandona il tentativo", danger: true, id: "f-ok", fai: () => {
      P.abbandonaSospeso(s.risorsa);
      avvisa("ok", s.certo ? "Tentativo tolto: niente era registrato." : "Tentativo abbandonato: la richiesta di prima, se arriva, sarà registrata.");
      render();
    } }]);
}
async function verificaSospesi() {
  // un tentativo «certo» non ha niente da verificare: il suo esito il server l'ha già detto (COPIA_IN_CORSO)
  for (const s of P.sospesi()) if (!s.certo) await verificaSospeso(s);
}
// verificaConRiprova: R6 con la stessa chiave dopo 1, 2, 4 e 8 secondi, poi su clic (MAT §5.5 n. 2); con «in_corso» si
// richiede dopo qualche secondo, per al più due minuti. fatto(j) torna true quando l'esito è chiuso. Un R6 che non
// risponde dice «Verifica non riuscita: riprovo» (MAT §5.4, R6) e, finiti i tentativi, «Verifica ora». Va avanti anche se
// chi guarda chiude il wizard.
function verificaConRiprova(chiave, fatto) {
  const attese = [1000, 2000, 4000, 8000];
  let i = 0;
  let inCorso = 0;
  S.verifiche.add(chiave);
  const fine = (testo) => {
    S.verifiche.delete(chiave);
    if (testo) avvisa("errore", testo);
    renderStrisce();
  };
  const giro = async () => {
    let j = null;
    let err = "";
    try {
      j = await P.esitoIntento(chiave);
    } catch (e) {
      err = testoErrore(e);
    }
    if (j && (await fatto(j))) return fine("");
    if (j && j.stato === "in_corso" && inCorso++ < 40) {
      setTimeout(giro, 3000);
      return;
    }
    if (j && j.stato === "non_registrato") return fine("");
    if (err && i < attese.length) avvisa("da_verificare", "Verifica non riuscita: riprovo (" + err + ").");
    if (i < attese.length) setTimeout(giro, attese[i++]);
    else fine("Verifica non riuscita: la chiave resta in sospeso. «Verifica ora» dalla striscia riprova.");
  };
  setTimeout(giro, attese[i++]);
}

// ---------------------------------------------------------------- la barra fissa

// renderBarra: la barra ricreata tiene il fuoco sul pulsante che l'aveva (Annulla, Ripeti), ritrovato per id, senza
// scorrere (UI46, controllo di stabilità del 21/10)
function renderBarra() {
  const box = $("#du-barra");
  const a = document.activeElement;
  const id = box && a && a !== box && box.contains(a) && a.id ? a.id : "";
  renderBarraDentro();
  const el = id && document.getElementById(id);
  if (el && box.contains(el) && el !== document.activeElement && !el.disabled) el.focus({ preventScroll: true });
}
function renderBarraDentro() {
  const box = $("#du-barra");
  if (!box) return;
  if (S.passo !== "revisione") {
    // nella Sorgente la struttura non si modifica, ma le preassociazioni dei file sì (MAT FL-12): con una bozza, la barra
    // dice anche qui lo stato del salvataggio (D23:286)
    // la barra di stato del tab Sorgente (mockup v5, §2 e §5): la scritta di garanzia, «Ultima mossa» (solo le mosse
    // dell'utente: resta, con «Vai al file» e «Annulla la mossa», finché non ne fa un'altra) e «Vai alla Revisione →»
    const m = SG.mossa;
    const garanzia = bozzaLavorabile() ? "Sorgente: qui si preassociano i file; la struttura si modifica nella Revisione." : "Sorgente: la BOM non si modifica qui.";
    // l'ultima mossa fatta su un altro prodotto lo dice (messaggio 46, D22): non si legge come un esito di quello in vista
    const altroP = m && m.prodotto && m.prodotto !== S.prodotto ? codiceDi(m.prodotto) : "";
    const mossa = m ? h("span", { class: "du-sg-mossa", id: "du-sg-mossa", "data-esito": m.esito, "data-prodotto": m.prodotto || null, role: m.esito === "ko" ? "alert" : "status" },
      h("span", { class: "du-lab" }, altroP ? "Ultima mossa · " + altroP : "Ultima mossa"), " ", h("span", { id: "du-sg-mossa-t" }, m.testo),
      m.allegato && fileDi(m.allegato) ? btn("Vai al file", () => vaiAlFile(m.allegato), { cls: "du-btn--sm du-btn--ghost", id: "du-sg-mossa-vai", perche: false }) : null,
      mossaAnnullabile() ? btn("Annulla la mossa", annullaMossa, { cls: "du-btn--sm", id: "du-sg-mossa-annulla", perche: false }) : null) : null;
    const nessuna = nessunaBozza();
    box.replaceChildren(h("div", { class: "du-salva du-salva--sorg" }, bozzaLavorabile() ? statoSalvataggio() : null,
      h("span", { class: "du-salva-conti", title: m ? garanzia : null }, m ? h("span", { class: "du-sg-garanzia-corta" }, h("span", { "aria-hidden": "true" }, "La BOM non si modifica qui ⓘ"), h("span", { class: "du-sr" }, garanzia)) : garanzia),
      mossa,
      btn("Vai alla Revisione →", () => vaiPasso("revisione"), { cls: "du-btn--sm" + (nessuna ? "" : " du-btn--pri"), id: "du-vai-revisione", no: S.prodotto ? "" : "nessun prodotto valutato", t: nessuna ? "nessuna bozza: conferma prima una sorgente" : null })));
    return;
  }
  if (!bozzaLavorabile()) {
    box.replaceChildren();
    S.barraAltezza = null;
    return;
  }
  const r = S.riepilogo;
  const conti = h("span", { class: "du-salva-conti", id: "du-conti" }, S.riepilogoStato === "aggiornato" && r ?
    [h("span", {}, "blocchi ", h("b", {}, String(arr(r.blocchi).length))), h("span", {}, "avvisi ", h("b", {}, String(arr(r.avvisi).length))),
      h("span", {}, "documenti irrisolti ", h("b", {}, String(arr(r.documenti_preassociati).filter((d) => d.esito === "in_conflitto" || d.esito === "in_attesa_codice" || d.esito === "orfana").length)))] :
    h("span", {}, S.riepilogoStato === "errore" ? "riepilogo non letto: " + (S.riepilogoErrore || "") : S.riepilogoStato === "da_aggiornare" ? "conti in calcolo…" : ""));
  const noCta = perchéNoConferma();
  const salva = h("div", { class: "du-salva", id: "du-salva" }, statoSalvataggio(), conti,
    h("span", { class: "du-azioni" },
      btn("Annulla", annulla, { cls: "du-btn--sm", id: "du-annulla", no: !S.pila.length ? "niente da annullare" : perchéNoModifica(), perche: false }),
      btn("Ripeti", ripeti, { cls: "du-btn--sm", id: "du-ripeti", no: !S.rifai.length ? "niente da ripetere" : perchéNoModifica(), perche: false }),
      btn("Abbandona la bozza…", apriAbbandona, { cls: "du-btn--sm du-btn--ghost", id: "du-abbandona", no: perchéNoFormaBozza(), perche: false })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma BOM", apriWizard, { cls: "du-btn--pri", id: "du-cta-bom", no: noCta })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma fascicolo", null, { cls: "du-btn--sm", id: "du-cta-fascicolo", no: NON_V1.fascicolo, perche: false })));
  // la barra non si accorcia mentre salva («conti in calcolo…» sta su una riga, i conti di R4 su due): l'altezza più
  // grande vista a questa larghezza resta come minimo, messa prima di inserirla, così una pagina scorsa fino in fondo
  // non salta di 32 px a ogni Annulla, Ripeti o OK (UI46, stabilità, seconda rimisura)
  const larga = window.innerWidth;
  if (!S.barraAltezza || S.barraAltezza.larga !== larga) {
    // a una larghezza nuova (la finestra ridimensionata, senza un render della barra) si parte dall'altezza naturale della
    // barra che c'è, misurata senza il minimo della larghezza di prima: da zero, il primo salvataggio dopo la accorcerebbe
    const prima = document.getElementById("du-salva");
    let alta = 0;
    if (prima && box.contains(prima)) {
      prima.style.minHeight = "";
      alta = prima.offsetHeight;
    }
    S.barraAltezza = { larga, alta };
  }
  if (S.barraAltezza.alta) salva.style.minHeight = S.barraAltezza.alta + "px";
  box.replaceChildren(salva);
  S.barraAltezza.alta = Math.max(S.barraAltezza.alta, salva.offsetHeight);
}
// statoSalvataggio: lo stato del salvataggio della bozza (MAT §5.1; D23:286), con «Riprova ora» dopo tre tentativi e
// «Rileggi la bozza salvata» dopo un conflitto o un rifiuto.
function statoSalvataggio() {
  const st = S.salvataggio;
  const nMod = S.coda.reduce((n, v) => n + (v.annullaDi ? 0 : 1), 0) + (S.invio ? S.invio.voci.length : 0);
  const testi = {
    pulita: "Nessuna modifica",
    modifiche: "✎ " + nMod + " modifiche non salvate",
    in_corso: "Salvataggio…",
    salvata: "Bozza salvata · versione " + st.versione + (st.ora ? " · " + st.ora : "") + (st.testo === "riproposto" ? " (riproposto)" : ""),
    non_arrivato: S.tentativi < 3 ? "Salvataggio non riuscito: riprovo" : "Salvataggio non riuscito",
    conflitto: "La bozza è cambiata altrove",
    rifiutata: "Salvataggio rifiutato" + (st.motivo ? ": " + st.motivo : st.codice ? ": " + st.codice : ""),
  };
  const statoEl = h("span", { class: "du-salva-stato", id: "du-salvataggio", "data-du-salvataggio": st.stato, role: "status" }, testi[st.stato] || st.stato);
  const extra = [];
  if (st.stato === "non_arrivato" && st.codice === "PONTE_SESSIONE") extra.push(h("span", { class: "du-perche", id: "du-salvataggio-testo" }, st.testo || ""));
  if (st.stato === "non_arrivato" && S.tentativi >= 3) extra.push(btn("Riprova ora", () => {
    S.tentativi = 0;
    invia();
  }, { cls: "du-btn--sm", id: "du-riprova-salvataggio", perche: false }));
  if (st.stato === "conflitto" || st.stato === "rifiutata") {
    extra.push(h("span", { class: "du-perche", id: "du-conflitto-testo" }, (st.testo || "") + (S.coda.length ? " Modifiche non salvate: " + S.coda.map((v) => v.etichetta).join("; ") +
      ". Non si perdono: rileggendo, restano da parte finché non decidi se riapplicarle o scartarle." : "")));
    extra.push(btn("Rileggi la bozza salvata", rileggiBozzaSalvata, { cls: "du-btn--sm du-btn--pri", id: "du-rileggi-bozza", perche: false }));
  }
  return [statoEl].concat(extra);
}
// rileggiBozzaSalvata: «Rileggi la bozza salvata» dopo un conflitto o un rifiuto (MAT §5.1). Le modifiche non salvate
// non si perdono finché la pagina è aperta: si mettono da parte, R1 si rilegge, e chi guarda decide con un gesto
// esplicito se riapplicarle alla bozza riletta (il server le riconcilia, o le rifiuta di nuovo) o scartarle.
async function rileggiBozzaSalvata() {
  clearTimeout(timerSalva);
  let voci = (S.invio ? S.invio.voci : []).concat(S.coda);
  const perche = S.salvataggio.stato;
  const testo = S.salvataggio.testo || "";
  const superato = S.salvataggio.motivo === "proposta_non_corrispondente";
  const origine = origineDaParte();
  S.coda = [];
  S.invio = null;
  // G39-01: il 422 proposta_non_corrispondente dice che un suggerimento accettato è superato. Tutte le voci restano da
  // parte, anche quelle con un registro dichiarato (una voce può avere altre azioni: «Ripristina», le mosse di gruppo, gli
  // spostamenti con il registro della versione): si tolgono solo i registri, e «Riapplica» li ricalcola con i
  // suggerimenti riletti (registroDaRifare). Riapplicate così, il server non le rifiuta per il registro superato
  // (rilievo importante della revisione indipendente 3 della corsia interfaccia)
  if (superato) {
    voci = voci.map((v) => (arr(v.locali).some((a) => a && a.azione === "file_preassociato" && ogg(a.dopo) && a.dopo.importazione)
      ? Object.assign({}, v, { locali: senzaRegistriDelMotore(v.locali), registroDaRifare: true }) : v));
  }
  await caricaBozza();
  if (superato) await rileggiSuggerimenti();
  if (voci.length) mettiDaParte(voci, perche, testo, origine);
  render();
}
// rileggiSuggerimenti: i suggerimenti del motore con il calcolo di adesso (R1 ?suggerimenti_motore=1, G39-01) nella bozza
// letta, poi S9 (che li calcola allo stesso modo). Un motore che non calcola non è un errore della lettura: restano quelli
// del registro della versione, e lo si dice.
async function rileggiSuggerimenti() {
  if (!S.prodotto || !S.bozza) return;
  try {
    const j = await P.leggiBozza(S.prodotto, { suggerimenti_motore: true });
    if (j && S.bozza && j.bozza_id === S.bozza.bozza_id) S.bozza.preassociazioni_proposte = arr(j.preassociazioni_proposte);
    if (j && ogg(j.suggerimenti_motore) && j.suggerimenti_motore.errore) avvisa("errore", "I suggerimenti del motore non sono ricalcolati adesso (" + j.suggerimenti_motore.errore + "): restano quelli della bozza. Riprova con ↻ Aggiorna.");
  } catch (e) {
    avvisa("errore", "I suggerimenti del motore non si rileggono: " + testoErrore(e));
  }
  await caricaSmistamento();
}
// origineDaParte: il prodotto e la bozza su cui sono nate le modifiche che stanno per andare da parte (prima di rileggere
// R1, che può portare un'altra bozza).
function origineDaParte() {
  return { prodotto: S.prodotto, codice: codiceProdotto(), bozza_id: S.bozza && S.bozza.bozza_id ? S.bozza.bozza_id : "", versione: S.bozza ? S.bozza.versione : 0 };
}
// mettiDaParte: le modifiche non salvate tenute da parte (MAT §5.1), con il prodotto e la bozza su cui sono nate
// (decisioni dell'ondata A, §5 n. 2). Se ce n'erano già da parte della stessa bozza, le nuove si aggiungono in coda.
function mettiDaParte(voci, perche, testo, origine) {
  const prima = S.daParte && S.daParte.prodotto === origine.prodotto && S.daParte.bozza_id === origine.bozza_id ? S.daParte.voci : [];
  S.daParte = Object.assign({ voci: prima.concat(voci), perche, testo }, origine);
}
// le voci da parte che si riapplicano: i gesti. Gli annullamenti e le ripetizioni di passi già salvati (annulla{n},
// rifai{n}) no: portano i numeri degli eventi di prima, e dopo la rilettura si rifanno con «Annulla» e «Ripeti».
function vociRiapplicabili(voci) {
  return voci.filter((v) => !v.annullaDi && !/^ripeti: /.test(v.etichetta) && !v.inviare.some((a) => a.azione === "annulla" || a.azione === "rifai"));
}
// perchéNoRiapplica: perché le modifiche da parte non si riapplicano alla bozza che si vede, o "". Valgono solo per il
// prodotto su cui sono nate: le loro azioni portano le chiavi dei suoi nodi (decisioni dell'ondata A, §5 n. 2).
function perchéNoRiapplica(dp) {
  if (dp.prodotto !== S.prodotto) return "sono di " + (dp.codice || "un altro prodotto") + ": si riapplicano solo alla sua bozza; torna a quel prodotto, o scartale";
  if (!bozzaLavorabile()) return "la bozza riletta non c'è o non si lavora: si possono solo scartare";
  return "";
}
// riapplicaDaParte: «Riapplica alla bozza riletta». Sulla stessa bozza (lo stesso bozza_id, una versione più avanti) le
// modifiche tornano in coda e il server le riconcilia al salvataggio. Su un'altra bozza dello stesso prodotto (quella di
// prima è stata chiusa o sostituita altrove) serve una conferma esplicita, con l'elenco: le modifiche erano pensate per
// la bozza di prima (decisioni dell'ondata A, §5 n. 2). Su un altro prodotto mai.
function riapplicaDaParte() {
  const dp = S.daParte;
  if (!dp) return;
  const noR = perchéNoRiapplica(dp);
  if (noR) return avvisa("errore", "Le modifiche non si riapplicano: " + noR + ".");
  const noM = perchéNoModifica({ daParte: true });
  if (noM) return avvisa("errore", "Le modifiche non si riapplicano: " + noM + ".");
  if (dp.bozza_id && dp.bozza_id !== S.bozza.bozza_id) {
    apriFinestra("Riapplicare a un'altra bozza?", h("div", { style: "display:grid;gap:8px" },
      h("p", {}, "Le modifiche erano della bozza di " + (dp.codice || "questo prodotto") + " aperta prima (versione " + dp.versione + "); la bozza di adesso è un'altra: quella di prima è stata chiusa o sostituita altrove."),
      h("ul", { class: "du-elenco-p", id: "f-da-parte-altra" }, dp.voci.map((v) => h("li", {}, v.etichetta))),
      h("p", { class: "du-nota" }, "Riapplicandole qui, il server le riconcilia con la bozza di adesso, o le rifiuta: i pezzi che non ci sono più non si ricreano da soli.")),
    [{ testo: "Annulla", chiudi: true }, { testo: "Riapplica a questa bozza", pri: true, id: "f-ok", fai: () => {
      riapplicaOra(dp);
    } }]);
    return;
  }
  riapplicaOra(dp);
}
function riapplicaOra(dp) {
  if (S.daParte !== dp) return;
  S.daParte = null;
  const no = perchéNoModifica();
  if (no) {
    S.daParte = dp;
    return avvisa("errore", "Le modifiche non si riapplicano: " + no + ".");
  }
  const voci = vociRiapplicabili(dp.voci);
  for (const v of voci) {
    // i registri tolti dopo proposta_non_corrispondente si ricalcolano con i suggerimenti di adesso (rileggiBozzaSalvata)
    const voce = { etichetta: v.etichetta, inviare: v.inviare, locali: v.registroDaRifare ? conRegistriDelMotore(v.locali) : v.locali, numeri: null, rebase: v.rebase || null };
    S.coda.push(voce);
    S.pila.push({ etichetta: v.etichetta, voci: [voce], salvata: false });
  }
  S.rifai = [];
  ricalcola();
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: voci.length ? "modifiche" : S.salvataggio.stato });
  if (voci.length) programmaSalvataggio(300);
  avvisa("ok", "Riapplicate alla bozza riletta " + voci.length + " modifiche: il server le riconcilia al salvataggio." + (voci.length < dp.voci.length ? " Gli annullamenti non salvati vanno rifatti con «Annulla»." : ""));
  render();
}
function scartaDaParte() {
  const dp = S.daParte;
  if (!dp) return;
  apriFinestra("Scartare le modifiche tenute da parte?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Queste modifiche di " + (dp.codice || "questo prodotto") + " non sono mai state salvate e si perdono:"),
    h("ul", { class: "du-elenco-p", id: "f-da-parte" }, dp.voci.map((v) => h("li", {}, v.etichetta)))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scarta le modifiche", danger: true, id: "f-ok", fai: () => {
    S.daParte = null;
    avvisa("ok", "Modifiche non salvate scartate: la bozza è quella salvata.");
    render();
  } }]);
}
// la riga delle modifiche tenute da parte (barra della Revisione e strisce): dice di quale prodotto sono
function rigaDaParte() {
  const dp = S.daParte;
  if (!dp) return null;
  const dopo = dp.perche === "conflitto" ? "dopo «La bozza è cambiata altrove»" : "dopo «Salvataggio rifiutato»";
  const di = dp.codice ? " di " + dp.codice : "";
  return h("div", { class: "du-striscia du-striscia--ver", id: "du-da-parte", role: "status", "data-du-prodotto": dp.prodotto || null },
    h("span", {}, dp.voci.length + " modifiche non salvate" + di + " tenute da parte " + dopo + ": " + dp.voci.map((v) => v.etichetta).join("; ") + ". Non si perdono finché la pagina è aperta."),
    h("span", { class: "du-azioni" },
      btn("Riapplica alla bozza riletta", riapplicaDaParte, { cls: "du-btn--sm du-btn--pri", id: "du-riapplica", no: perchéNoRiapplica(dp), perche: false }),
      btn("Scartale…", scartaDaParte, { cls: "du-btn--sm du-btn--ghost", id: "du-scarta-da-parte", perche: false })));
}
function perchéNoConferma() {
  if (S.vista !== "org") return "il riepilogo della BOM si apre dall'Organigramma";
  const no = perchéNoModifica({ riprova: true });
  if (no) return no;
  if (S.bozza.base && S.bozza.base.stato === "superata") return "la base è superata: fai prima il rebase";
  return nessunaModifica();
}

// ---------------------------------------------------------------- il wizard «Rivedi e conferma BOM» (MAT §3.8)

async function apriWizard() {
  const no = perchéNoConferma();
  if (no) return avvisa("errore", "Il riepilogo non si apre: " + no + ".");
  S.wizard = { fase: 1, stato: "lettura", gruppo: "tutti", riepilogo: null, errore: "", esito: null, dettagli: new Set(), codice: codiceProdotto(), prodotto: S.prodotto,
    // gli ingressi del riepilogo e le notifiche (U2 §6.3): le parti di R4, i documenti in analisi mostrati (la presa
    // d'atto ne porta l'elenco), N1-N5
    ingressi: null, inAnalisi: [], mostrati: [], presaAnalisi: false, analizzati: [], n1: "", n2: null, n2incerto: false, n3: null, n4: null, n5: "", nuovo: null, daRiepilogare: false, ultimoR4: 0 };
  mostraWizard();
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) {
      S.wizard.stato = "errore";
      S.wizard.errore = "Le modifiche della bozza non sono salvate: il riepilogo si calcola solo sulla bozza salvata.";
      return renderWizard();
    }
  }
  await rileggiRiepilogo();
}
function mostraWizard() {
  $("#du-velo").hidden = false;
  const w = $("#du-wizard");
  w.hidden = false;
  renderWizard();
}
function chiudiWizard() {
  if (S.wizard && S.wizard.stato === "conferma") return;
  fermaIngressi();
  S.wizard = null;
  $("#du-wizard").hidden = true;
  $("#du-wizard").replaceChildren();
  $("#du-velo").hidden = true;
  render();
  // il fuoco torna al pulsante che apre il wizard (ricreato dal render), senza far scorrere la pagina (UI46, 21/10)
  const cta = $("#du-cta-bom");
  if (cta && (!document.activeElement || document.activeElement === document.body)) cta.focus({ preventScroll: true });
}
async function rileggiRiepilogo() {
  const w = S.wizard;
  w.stato = "lettura";
  renderWizard();
  try {
    w.riepilogo = await P.riepilogoBom(corpoBozza(), { motore: true });
    w.versione = S.bozza.versione;
    w.stato = "pronto";
    // i blocchi di un 422 di R5 valevano per il riepilogo di prima: quello riletto dice i suoi
    if (w.esito && w.esito.codice === "BLOCCHI") w.esito = null;
    // gli ingressi su cui poggia il riepilogo e i documenti in analisi che mostra (U2 §6.3.3, §6.3.7): da qui le notifiche
    w.ingressi = partiIngressiR4(w.riepilogo.ingressi);
    w.inAnalisi = arr(w.riepilogo.file_in_analisi);
    w.mostrati = w.inAnalisi.slice();
    w.presaAnalisi = false;
    w.n1 = "";
    w.n2 = null;
    w.n2incerto = false;
    w.n3 = null;
    w.n4 = null;
    w.n5 = "";
    w.nuovo = null;
    w.daRiepilogare = false;
    // il limite di un R4 ogni 30 s vale per quelli che la pagina chiede da sola, non per il riepilogo aperto da chi guarda
    w.ultimoR4 = 0;
    programmaIngressi();
  } catch (e) {
    w.stato = "errore";
    w.errore = "Il riepilogo non si legge: " + testoErrore(e) + (e.codice === "CORPO_NON_VALIDO" ? " (la bozza salvata non si rilegge: difetto da segnalare)" : "");
  }
  renderWizard();
}
function mostraNelGrafo(k) {
  S.nodo = k;
  S.vista = "org";
  S.wizard = null;
  $("#du-wizard").hidden = true;
  $("#du-velo").hidden = true;
  render();
  centraSel();
}
function nonInclusiNellaBozza() {
  return S.salvato ? S.salvato.decisioni.filter((d) => d.tipo === "non_includere").length : 0;
}
// UI46 [U] (messaggio 48: «flickering nel wizard R5 che fanno tornare su la pagina mentre la scrollo»): il wizard si
// ridisegna intero (replaceChildren), e il corpo che scorre (#du-wz-corpo) nasceva sempre in cima. Ridisegnare non deve
// spostare chi legge [T]: lo scorrimento del corpo si conserva finché restano la stessa fase, lo stesso gruppo e lo
// stesso esito (un esito nuovo, una fase o un gruppo diversi ripartono dall'inizio, dove si leggono), e tornano il fuoco
// e i <details> aperti che hanno un id. Il «Calcolo del riepilogo…» di una rilettura non cancella la posizione: torna
// con il riepilogo riletto. La rilettura in sottofondo che non cambia niente non ridisegna affatto (leggiIngressi).
function vistaWizardPrima(box, w) {
  const c = box.querySelector("#du-wz-corpo");
  if (c && c.dataset.duPronto === "1") w.scorri = { chiave: c.dataset.duVista, esito: w.scorriEsito, top: c.scrollTop };
  const a = document.activeElement;
  return { fuoco: a && a !== box && box.contains(a) && a.id ? a.id : "", aperti: Array.from(box.querySelectorAll("details[open][id]"), (d) => d.id) };
}
function vistaWizardDopo(box, w, v, corpo) {
  for (const id of v.aperti) {
    const d = document.getElementById(id);
    if (d && box.contains(d)) d.open = true;
  }
  if (corpo.dataset.duPronto === "1" && w.scorri && w.scorri.chiave === corpo.dataset.duVista && w.scorri.esito === w.esito) corpo.scrollTop = w.scorri.top;
  w.scorriEsito = w.esito;
  if (v.fuoco) {
    const el = document.getElementById(v.fuoco);
    if (el && box.contains(el) && el !== document.activeElement) el.focus({ preventScroll: true });
  }
}
// firmaVistaWizard: quello che il wizard mostra e che la rilettura in sottofondo (leggiIngressi) può cambiare: le
// notifiche N1-N5, i documenti in analisi con la presa d'atto, il riepilogo nuovo, lo stato e la ragione del CTA spento
function firmaVistaWizard(w) {
  return JSON.stringify([w.stato, w.n1, w.n2, w.n2incerto, w.n3, w.n4, w.n5, arr(w.mostrati).map((x) => x.allegato_id), w.presaAnalisi, !!w.nuovo,
    w.esito && w.esito.tipo, perchéNoConfermaWizard(w)]);
}
function renderWizard() {
  const box = $("#du-wizard");
  const w = S.wizard;
  if (!box || !w) return;
  const vista = vistaWizardPrima(box, w);
  const versione = S.bozza && typeof S.bozza.versione === "number" ? "bozza versione " + S.bozza.versione : w.esito && w.esito.tipo === "ok" ? "bozza confermata" : "bozza non letta";
  const testa = h("div", { class: "du-wz-t" }, h("h2", { id: "du-wizard-titolo" }, "Rivedi e conferma BOM · " + (w.codice || codiceProdotto())),
    h("span", { class: "du-muto du-piccolo" }, versione), h("button", { type: "button", class: "du-btn du-btn--sm du-btn--ghost", id: "du-wz-chiudi", onclick: chiudiWizard,
      disabled: w.stato === "conferma" }, "Chiudi"));
  // collaudo UI46, giro 2 (C2-B2): dopo «Conferma BOM» il wizard dice solo l'esito. Il riepilogo di R4 era quello della
  // bozza, che non c'è più: i suoi avvisi («Con la conferma…»), «La BOM è confermabile» e i documenti senza la bozza
  // («un pezzo senza codice») dicevano lo stato di prima. Le fasi e «Conferma BOM» non servono più; la BOM confermata la
  // mostra la Revisione (la working, messaggio 47 [U]) [T]
  const confermata = !!(w.esito && w.esito.tipo === "ok");
  const passi = confermata ? null : h("div", { class: "du-wz-passi" },
    h("button", { type: "button", class: "du-wz-passo", "aria-current": w.fase === 1 ? "step" : null, id: "du-wz-fase1", onclick: () => {
      w.fase = 1;
      renderWizard();
    } }, "1 · Identità dei componenti"),
    h("button", { type: "button", class: "du-wz-passo", "aria-current": w.fase === 2 ? "step" : null, id: "du-wz-fase2", onclick: () => {
      w.fase = 2;
      renderWizard();
    } }, "2 · Gerarchie, quantità e tipi"));
  const corpo = h("div", { class: "du-wz-c", id: "du-wz-corpo", "data-du-vista": w.fase + "|" + (w.fase === 1 ? w.gruppo : ""),
    "data-du-pronto": w.riepilogo && w.stato !== "lettura" && w.stato !== "errore" ? "1" : "0" });
  if (w.esito) corpo.append(esitoWizard(w));
  if (w.stato === "lettura") corpo.append(h("p", {}, "Calcolo del riepilogo…"));
  else if (w.stato === "errore") corpo.append(h("div", { class: "du-striscia du-striscia--ko", role: "alert" }, w.errore));
  else if (confermata) corpo.append(h("p", { class: "du-nota", id: "du-wz-dopo-conferma" }, "La BOM confermata resta visibile nella Revisione, in sola lettura: è la BOM di lavoro (working). Per cambiarla, «Modifica la BOM confermata» apre una bozza nuova."));
  else if (w.riepilogo) {
    for (const n of notificheWizard(w)) corpo.append(n);
    corpo.append(w.fase === 1 ? faseIdentita(w) : faseGerarchie(w));
  }
  const piede = h("div", { class: "du-wz-p" });
  if (confermata) {
    piede.append(h("span", { class: "du-sp" }), btn("Chiudi e guarda la BOM confermata", chiudiWizard, { cls: "du-btn--pri", id: "du-wz-fine", perche: false }));
    box.replaceChildren(testa, corpo, piede);
    vistaWizardDopo(box, w, vista, corpo);
    return;
  }
  if (w.fase === 2) piede.append(btn("Torna alla fase 1", () => {
    w.fase = 1;
    renderWizard();
  }, { cls: "du-btn--sm", perche: false }));
  else piede.append(btn("Fase 2 →", () => {
    w.fase = 2;
    renderWizard();
  }, { cls: "du-btn--sm", id: "du-wz-avanti", perche: false }));
  piede.append(h("span", { class: "du-sp" }));
  const no = perchéNoConfermaWizard(w);
  piede.append(h("span", { class: "du-cta" }, btn("Conferma BOM", confermaWizard, { cls: "du-btn--pri", id: "du-wz-conferma", no })));
  box.replaceChildren(testa, passi, corpo, piede);
  vistaWizardDopo(box, w, vista, corpo);
}
function perchéNoConfermaWizard(w) {
  if (w.stato === "conferma") return "conferma in corso…";
  if (w.esito && w.esito.tipo === "ok") return "la BOM è confermata";
  if (w.esito && w.esito.tipo === "verifica") return "esito da verificare: aspetta la verifica";
  if (w.esito && w.esito.tipo === "controllo_finale") return "la conferma non è registrata per un errore interno: segnalalo, non riprovare";
  // R5 ha detto che la bozza è la conferma di base (422 NESSUNA_MODIFICA, PE-23): non c'è niente da confermare
  if (w.esito && w.esito.codice === "NESSUNA_MODIFICA") return "nessuna modifica rispetto alla conferma di base: niente da confermare";
  if (!w.riepilogo || w.stato !== "pronto") return "il riepilogo non è pronto";
  if (S.coda.length || S.invio) return "ci sono modifiche non salvate: il riepilogo va riletto";
  if (!S.bozza || w.versione !== S.bozza.versione) return "la bozza è cambiata: rileggi il riepilogo";
  // le notifiche che spengono la conferma (U2 §6.3.4) e la presa d'atto dei documenti in analisi (§6.3.7): prima degli
  // esiti del riepilogo, che non è più quello da confermare
  if (w.n4) return "la bozza è stata salvata da un'altra persona: rileggi la bozza";
  if (w.n3) return "la BOM registrata è cambiata: rileggi il riepilogo";
  if (w.n2) return "il riepilogo non è più aggiornato: apri il riepilogo nuovo";
  if (arr(w.mostrati).length && !w.presaAnalisi) return "ci sono documenti in analisi: spunta la presa d'atto";
  // il riepilogo del wizard decide con uguale_alla_base, se lo dà; altrimenti il giornale (PE-23)
  const nm = nessunaModifica(w.riepilogo);
  if (nm) return nm;
  // direttiva 39 §1 [U]: senza uno STEP confermato da S8 il riepilogo si consulta tutto, ma la conferma resta spenta
  if (arr(w.riepilogo.blocchi).some((b) => b.codice === "STEP_SORGENTE_NON_CONFERMATA")) return "serve uno STEP confermato come sorgente: conferma uno STEP dalla Sorgente («Conferma questo STEP come sorgente»)";
  // gli avvisi del motore stanno nella firma e il riepilogo senza di loro non è confermabile (blocco «R4 completo» della
  // persistenza, PA-C01): niente presa d'atto, si rilegge il riepilogo quando il motore risponde
  if (w.riepilogo.avvisi_motore_calcolati !== true) return "gli avvisi del motore non sono calcolati" + (w.riepilogo.avvisi_motore_errore && w.riepilogo.avvisi_motore_errore.codice ? " (" + w.riepilogo.avvisi_motore_errore.codice + ")" : "") + ": «Riprova» il riepilogo";
  if (!w.riepilogo.confermabile) return "la BOM non è confermabile: " + arr(w.riepilogo.blocchi).length + " blocchi";
  // R5 ha risposto 422 BLOCCHI su questo riepilogo (lo stato è cambiato dopo R4, per esempio una revoca della sorgente):
  // si rilegge il riepilogo prima di riprovare
  if (w.esito && w.esito.codice === "BLOCCHI") return "la conferma ha trovato dei blocchi: rileggi il riepilogo";
  return "";
}
function segnoEmittente(d) {
  if (!d || typeof d.emittente !== "string") return null;
  if (d.emittente.indexOf("partner:") === 0) return pill("info", "fornitore " + d.emittente.slice(8));
  return pill("neu", d.emittente_presunto ? "cliente, presunto" : "cliente");
}
function faseIdentita(w) {
  const r = w.riepilogo;
  const id = arr(r.identita);
  const collisioni = new Set();
  for (const b of arr(r.blocchi)) if (b.codice === "COLLISIONE_EMITTENTI" || b.codice === "IDENTITA_INCOMPATIBILE") for (const p of arr(b.parti)) if (p.chiave_bozza) collisioni.add(p.chiave_bozza);
  const gruppi = [["tutti", "Tutti", () => true], ["creati", "Creati", (x) => x.azione === "crea"], ["accettate", "Accettate dalla proposta", (x) => x.esito_identita === "accettata"],
    ["riusati", "Riusati", (x) => x.azione === "riusa"], ["modificati", "Modificati", (x) => x.azione === "modifica"], ["esclusi", "Esclusi", (x) => x.azione === "esclude" || x.azione === "archivia"],
    ["collisioni", "Collisioni", (x) => arr(x.collisioni).length > 0 || collisioni.has(x.chiave_bozza)]];
  const f = (gruppi.find((g) => g[0] === w.gruppo) || gruppi[0])[2];
  const box = h("div", { style: "display:grid;gap:10px" });
  box.append(h("div", { class: "du-gruppi", role: "group", "aria-label": "Gruppi" }, gruppi.map(([k, t, fn]) => h("button", { type: "button", "aria-pressed": w.gruppo === k ? "true" : "false", id: "du-wz-g-" + k,
    onclick: () => {
      w.gruppo = k;
      renderWizard();
    } }, t + " (" + id.filter(fn).length + ")")), h("button", { type: "button", "aria-pressed": w.gruppo === "noninc" ? "true" : "false", id: "du-wz-g-noninc",
    onclick: () => {
      w.gruppo = "noninc";
      renderWizard();
    } }, "Non inclusi (" + nonInclusiDi(r).length + ")"),
  // il gruppo «Categorie da confermare» (U2 §6.4.2; MAT WZ-02): un secondo punto d'ingresso, lo stesso modello dell'editor
  h("button", { type: "button", "aria-pressed": w.gruppo === "categorie" ? "true" : "false", id: "du-wz-g-categorie",
    onclick: () => {
      w.gruppo = "categorie";
      renderWizard();
    } }, "Categorie da confermare (" + categorieDaConfermare().filter((v) => !decisioneCategoria(v.k)).length + ")")));
  if (w.gruppo === "noninc") {
    box.append(tabellaNonInclusi(r));
    return box;
  }
  if (w.gruppo === "categorie") {
    box.append(tabellaCategorie(w));
    return box;
  }
  const righe = id.filter(f).map((x) => {
    const d = x.decisa || {};
    const codice = (x.codice && x.codice.dopo) || d.codice || (d.codice_dopo) || "";
    const aperto = w.dettagli.has(x.chiave_bozza);
    const tr = [h("tr", { class: collisioni.has(x.chiave_bozza) ? "du-r-blocco" : null, "data-k": x.chiave_bozza || "" },
      h("td", {}, h("span", { class: "du-mono" }, codice || "(senza codice)"), x.rev && x.rev.dopo ? " rev " + x.rev.dopo : ""),
      h("td", {}, x.tipo && x.tipo.dopo ? TIPI[x.tipo.dopo] || x.tipo.dopo : ""),
      h("td", {}, x.azione || "", x.esito_identita ? " · " + x.esito_identita : ""),
      h("td", {}, segnoEmittente(d)),
      h("td", {}, h("span", { class: "du-azioni" }, btn(aperto ? "Nascondi" : "Dettagli", () => {
        if (aperto) w.dettagli.delete(x.chiave_bozza);
        else w.dettagli.add(x.chiave_bozza);
        renderWizard();
      }, { cls: "du-btn--sm du-btn--ghost", perche: false }), x.chiave_bozza ? btn("Mostra nel grafo", () => mostraNelGrafo(x.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false }) : null)))];
    if (aperto) {
      const p = x.proposta || {};
      tr.push(h("tr", {}, h("td", { colspan: "5" }, h("dl", { class: "du-dl" }, riga("Proposta", p.letterale ? p.letterale + " · " + (p.emittente || "") : "nessuna"),
        riga("Riferimenti", String(arr(x.riferimenti).length)), riga("Modo", d.modo || ""), d.lettura ? riga("Lettura di allora", d.lettura.letterale || "") : null))));
    }
    return tr;
  });
  box.append(h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-identita" }, h("thead", {}, h("tr", {}, h("th", {}, "Codice"), h("th", {}, "Tipo"), h("th", {}, "Azione / esito"), h("th", {}, "Emittente"), h("th", {}, ""))),
    h("tbody", {}, righe.length ? righe : h("tr", {}, h("td", { colspan: "5" }, "Nessuna voce in questo gruppo."))))));
  return box;
}
// nonInclusiDi: i nodi non inclusi del riepilogo formato 3 (esclusi[] con origine non_includere, EM §7.4 con U2 §6.1)
function nonInclusiDi(r) {
  return arr(r && r.esclusi).filter((x) => ogg(x) && x.origine === "non_includere");
}
// nomeNonIncluso: il nome della voce dal registro della bozza (il nome grezzo dello STEP), o la casella. r: il riepilogo,
// il cui registro resta letto anche dopo la conferma, quando la bozza non c'è più
function nomeNonIncluso(x, r) {
  const reg = registroBozza() || (r && ogg(r.importazione) && S.registri[r.importazione.impronta]) || null;
  const v = x.casella ? voceDiCasella(reg, x.casella) : null;
  return (v && v.grezzi && v.grezzi.nome) || (v ? nomeVoce(v) : "") || (x.casella && x.casella.rif) || "voce senza nome";
}
function tabellaNonInclusi(r) {
  const nonInc = nonInclusiDi(r);
  return h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-noninclusi" },
    h("thead", {}, h("tr", {}, h("th", {}, "Nodo"), h("th", {}, "Decisione"), h("th", {}, "Effetti dopo la conferma"))),
    h("tbody", {}, nonInc.length ? nonInc.map((x) => h("tr", { "data-du-noninc": x.id || "" }, h("td", {}, h("span", { class: "du-mono" }, nomeNonIncluso(x, r))),
      // messaggio 46 [U]: nessun motivo (né il codice né il testo): chi ha deciso e quando, e il segnale del motore se c'era
      h("td", { "data-du-deciso": "" }, (decisoDa(x.id) || "non incluso nella bozza") + (x.segnale_visto && x.segnale_visto.codice ? " · segnale del motore: " + testoSegnale(x.segnale_visto.codice) : "")),
      h("td", { "data-du-effetti": "" }, testoEffettiEscluso(x.effetti)))) : h("tr", {}, h("td", { colspan: "3" }, "Nessun nodo non incluso.")))));
}
// testoEffettiEscluso: gli effetti di un'esclusione che R4 dichiara e R5 applica (EM §7.4; blocco «R4 completo» della
// persistenza): le righe e i legami dello STEP che la conferma chiude, nessuna cascata sui figli, i fabbisogni tolti, i
// file toccati e gli altri prodotti con lo stesso STEP. La pagina riporta i conti, non li calcola.
function testoEffettiEscluso(e) {
  const x = ogg(e) ? e : {};
  const righe = arr(x.righe_da_chiudere).length;
  const archi = arr(x.archi_da_chiudere).length;
  const fab = arr(x.fabbisogni_tolti_dopo_conferma).length;
  const stesso = arr(x.prodotti_con_lo_stesso_step);
  const parti = [];
  parti.push(righe === 1 ? "1 riga dello STEP si chiude alla conferma" : righe + " righe dello STEP si chiudono alla conferma");
  if (archi) parti.push(archi === 1 ? "1 legame dello STEP si chiude" : archi + " legami dello STEP si chiudono");
  parti.push(arr(x.figli).length ? arr(x.figli).length + " figli toccati" : "nessuna cascata sui figli");
  if (fab) parti.push(fab + " fabbisogni tolti dopo la conferma");
  parti.push(arr(x.occorrenze_toccate).length + " file toccati");
  if (stesso.length) parti.push("stesso STEP anche in: " + stesso.map((p) => codiceDi(String(p.prodotto_id).toLowerCase()) + " (" + p.come + ")").join(", "));
  return parti.join("; ");
}
// ALTRE_ESCLUSIONI: le voci di esclusi[] che non sono non_includere (EM §7.4 con U2 §6.1; origini legacy ed ereditata dal
// blocco «R4 completo» della persistenza): la conferma ne lascia la traccia; solo l'assenza chiude righe e legami
const ALTRE_ESCLUSIONI = { assenza: "voce dello STEP che la bozza non rappresenta", escludi: "componente escluso dalla BOM", legacy: "scartata nelle pagine di prima",
  ereditata: "non inclusa nella conferma di base, non ripetuta dalla bozza" };
function tabellaAltreEsclusioni(r) {
  const altre = arr(r && r.esclusi).filter((x) => ogg(x) && x.origine !== "non_includere");
  if (!altre.length) return null;
  return h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-altre-esclusioni" },
    h("thead", {}, h("tr", {}, h("th", {}, "Voce"), h("th", {}, "Origine"), h("th", {}, "Alla conferma"))),
    h("tbody", {}, altre.map((x) => h("tr", { "data-origine": x.origine || "" },
      h("td", {}, h("span", { class: "du-mono" }, x.codice || (x.casella ? nomeNonIncluso(x, r) : "voce senza nome"))),
      // messaggio 46 [U]: nessuna colonna del motivo; l'origine dice da dove viene l'esclusione
      h("td", {}, (ALTRE_ESCLUSIONI[x.origine] || x.origine || "") + (x.autore_id ? " (decisa da chi le guardava prima)" : "")),
      h("td", {}, x.origine === "assenza" ? testoEffettiEscluso(x.effetti) : x.origine === "escludi" ? "esce dalla BOM" : "solo la traccia, niente si chiude"))))));
}
function faseGerarchie(w) {
  const r = w.riepilogo;
  const box = h("div", { style: "display:grid;gap:12px" });
  const ctx = testoContestoNonApplicato();
  if (ctx) box.append(h("div", { class: "du-striscia du-striscia--ver", id: "du-wz-contesto" }, ctx));
  const blocchi = arr(r.blocchi);
  const senzaMotore = r.avvisi_motore_calcolati !== true;
  box.append(h("div", { class: "du-esito " + (r.confermabile ? "du-esito--ok" : "du-esito--ko"), id: "du-wz-confermabile" },
    h("b", {}, r.confermabile ? "La BOM è confermabile." : blocchi.length ? "La BOM non è confermabile: " + blocchi.length + " blocchi." + (senzaMotore ? " Anche gli avvisi del motore mancano." : "") :
      "La BOM non è confermabile finché gli avvisi del motore non sono calcolati: «Riprova» qui sotto."),
    riusoInBlocco(blocchi),
    blocchi.length ? h("ul", { class: "du-elenco-p", id: "du-wz-blocchi" }, blocchi.map((b) => h("li", { "data-codice": b.codice, "data-motivo": b.motivo || "" },
      h("b", {}, b.codice), b.motivo ? h("span", { class: "du-muto du-piccolo" }, " (" + b.motivo + ")") : null, " ", b.testo || "", arr(b.rimedi).length ? h("span", { class: "du-muto" }, " Rimedi: " + arr(b.rimedi).join("; ")) : null,
      b.chiave_bozza ? h("span", { class: "du-azioni" }, btn("Mostra nel grafo", () => mostraNelGrafo(b.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false }), gestiD1(b), gestoRiuso(b)) : null,
      // direttiva 39 §1: senza una sorgente confermata da S8 la conferma è bloccata; il rimedio è nella Sorgente
      b.codice === "STEP_SORGENTE_NON_CONFERMATA" ? h("span", { class: "du-azioni" }, btn("Vai alla Sorgente", () => {
        chiudiWizard();
        vaiPasso("sorgente");
      }, { cls: "du-btn--sm", id: "du-wz-vai-sorgente", perche: false })) : null))) : null));
  const avvisi = arr(r.avvisi);
  // gli avvisi del registro portano il codice o l'emittente che nominano (valore: il P/N K4, il codice del cartiglio,
  // l'emittente della lettura; blocco «R4 completo» della persistenza)
  // collaudo UI46 (I4, C2-I1): gli avvisi si dicono con il testo del server, senza il codice (che resta in data-codice);
  // quelli che dicono solo che cosa farà la conferma (AVVISI_INFORMATIVI) non hanno il colore dell'allarme [T]
  if (avvisi.length) {
    const soloInfo = avvisi.every((a) => AVVISI_INFORMATIVI.has(a.codice));
    box.append(h("div", { class: "du-esito " + (soloInfo ? "du-esito--info" : "du-esito--ver"), id: "du-wz-avvisi" }, h("b", {}, soloInfo ? "Da sapere" : "Avvisi"), h("ul", { class: "du-elenco-p" }, avvisi.map((a) => h("li", { "data-codice": a.codice },
      a.valore ? [h("span", { class: "du-mono", "data-du-valore": a.valore }, a.valore), " "] : null, testoAvviso(a),
      a.chiave_bozza ? h("span", { class: "du-azioni" }, btn("Mostra nel grafo", () => mostraNelGrafo(a.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false })) : null)))));
  }
  // gli avvisi e i conflitti del motore (EM §8.2, §9.4 con U2 §6.3.5): calcolati con il riepilogo e dentro la sua firma;
  // senza di loro la BOM non si conferma (niente presa d'atto: si riprova il riepilogo)
  const mot = h("div", { class: "du-esito du-esito--ver", id: "du-wz-motore" });
  // senza avvisi né conflitti il riquadro non è giallo: «nessuno» non è un avviso (collaudo UI46, I4) [T]
  if (r.avvisi_motore_calcolati === true && !arr(r.avvisi_motore).length && !arr(r.conflitti_motore).length) mot.className = "du-esito du-esito--info";
  if (r.avvisi_motore_calcolati !== true) {
    mot.append(h("b", {}, "Avvisi del motore non calcolati" + (r.avvisi_motore_errore ? " (" + (r.avvisi_motore_errore.codice || "") + ": " + (r.avvisi_motore_errore.testo || "") + ")" : "") + "."),
      h("span", {}, " La conferma resta spenta finché il riepilogo non li ha."),
      btn("Riprova", rileggiRiepilogo, { cls: "du-btn--sm", id: "du-wz-riprova-motore", perche: false }));
  } else {
    const am = arr(r.avvisi_motore);
    mot.append(h("b", {}, am.length ? "Avvisi del motore, calcolati con il riepilogo e firmati:" : "Avvisi del motore, calcolati con il riepilogo e firmati: il motore non ne ha."));
    if (am.length) mot.append(h("ul", { class: "du-elenco-p", id: "du-wz-avvisi-motore" }, am.map((a) => h("li", { "data-codice": a.codice || "", "data-motivo": a.motivo || "" },
      testoAvviso(a),
      ogg(a.differenze) ? h("span", { class: "du-muto du-piccolo" }, " · nodi diversi " + (a.differenze.nodi || 0) + ", legami diversi " + (a.differenze.archi || 0)) : null,
      arr(a.prodotti).length ? h("span", { class: "du-muto du-piccolo" }, " · anche " + arr(a.prodotti).map((x) => codiceDi(String(x).toLowerCase())).join(", ")) : null))));
    const cm = arr(r.conflitti_motore);
    mot.append(h("p", { class: "du-nota", id: "du-wz-conflitti-motore", "data-n": String(cm.length) }, cm.length ? "Conflitti aperti del motore sul prodotto: " + cm.join(", ") + " (la verifica prevista non sarà «sì» finché restano)." :
      "Nessun conflitto aperto del motore sul prodotto."));
  }
  box.append(mot);
  // mono_pezzo: casella mai spuntata (MAT BZ-14; dal messaggio 46 [U] senza motivo). Dopo la conferma la bozza non c'è più (R1 riletto:
  // nessuna bozza, S.salvato vuoto): il wizard mostra l'esito e il riepilogo di allora, senza gesti sulla bozza
  const salvate = S.salvato ? S.salvato.decisioni : [];
  if (avvisi.some((a) => a.codice === "PRODOTTO_SENZA_FIGLI") && bozzaLavorabile() && !salvate.some((d) => d.tipo === "mono_pezzo")) {
    box.append(h("div", { class: "du-esito du-esito--ver" }, h("b", {}, "Il prodotto non ha figli."), h("span", {}, "Se è un pezzo unico, dichiaralo: la conferma della BOM vuole lo STEP del prodotto confermato come sorgente."),
      btn("Dichiara mono_pezzo…", apriMonoPezzo, { cls: "du-btn--sm", perche: false })));
  }
  const archi = arr(r.archi);
  box.append(h("h3", {}, "Legami (" + archi.length + ")"), h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-archi" },
    h("thead", {}, h("tr", {}, h("th", {}, "Padre → figlio"), h("th", {}, "Qtà di prima"), h("th", {}, "Qtà voluta"), h("th", {}, "Azione"), h("th", {}, "Origine"))),
    h("tbody", {}, archi.map((a) => h("tr", { "data-azione": a.azione || "" }, h("td", {}, h("span", { class: "du-mono" }, ((a.padre && (a.padre.codice || nomeNodoWizard(a.padre.chiave_bozza, r))) || "?") + " → " + ((a.figlio && (a.figlio.codice || nomeNodoWizard(a.figlio.chiave_bozza, r))) || "?")),
      a.padre_prima ? h("span", { class: "du-muto du-piccolo" }, " (prima sotto " + (a.padre_prima.codice || "?") + ")") : null),
      h("td", {}, a.qta_precedente === null || a.qta_precedente === undefined ? "—" : String(a.qta_precedente)), h("td", {}, a.qta_voluta === null || a.qta_voluta === undefined ? "—" : String(a.qta_voluta)),
      h("td", {}, (a.azione || "") + (a.decisione ? " · " + a.decisione : "")), h("td", {}, a.origine || "")))))));
  box.append(h("h3", {}, "Non inclusi"), tabellaNonInclusi(r));
  const altre = tabellaAltreEsclusioni(r);
  if (altre) box.append(h("h3", {}, "Altre esclusioni"), altre);
  box.append(sezioneSorgenteWizard(r), sezioneDocumentiWizard(w, r));
  const cond = sezioneCondivisione(r);
  if (cond) box.append(cond);
  // il criterio previsto e la firma del riepilogo sono dati tecnici (collaudo UI46, I4): restano, ripiegati [T]
  const cp = r.criterio_previsto || {};
  box.append(h("details", { class: "du-ripiega du-piccolo", id: "du-wz-tecnici" }, h("summary", {}, "Dati tecnici del riepilogo"),
    h("p", { class: "du-nota", id: "du-wz-criterio" }, "Criterio che la conferma registrerà: " + (cp.criterio || "?") + (cp.motivo ? " — " + aParole(cp.motivo) : "")),
    h("p", { class: "du-nota" }, "Firma del riepilogo: ", h("span", { class: "du-mono", id: "du-wz-firma" }, String(r.firma_anteprima).slice(0, 12) + "…"))));
  return box;
}
// AVVISI_INFORMATIVI: gli avvisi di R4 che dicono che cosa farà la conferma, senza niente da decidere
const AVVISI_INFORMATIVI = new Set(["VERIFICA_DOPO_COMMIT", "SORGENTE_NON_AUTORIZZATA"]);
// testoAvviso: un avviso di R4 o del motore per chi legge: il testo del server in parole (aParole), mai il codice; senza
// testo, il codice in minuscolo e senza trattini bassi, e il motivo in parole (collaudo UI46, I4) [T]
function testoAvviso(a) {
  const t = aParole(a && a.testo);
  if (t) return t + ".";
  const c = String((a && a.codice) || "avviso").toLowerCase().replace(/_/g, " ");
  return c.charAt(0).toUpperCase() + c.slice(1) + (a && a.motivo ? " (" + aParole(String(a.motivo).replace(/_/g, " ")) + ")" : "") + ".";
}
// gestiD1: le decisioni D1 adatte a ciascun blocco (MAT BZ-12, WZ-07; EM §3.6, decisioni 1-3): per D1_ULTIMO_FIGLIO
// «Diventa particolare» (d1_a_particolare) o «Lascia assieme» (d1_resta_sottoassieme, un rinvio); per
// D1_PARTICOLARE_CON_FIGLI «Diventa assieme» (d1_a_sottoassieme). Ognuna è un salvataggio, poi il riepilogo si rilegge.
function gestiD1(b) {
  const id = (s) => "du-wz-d1-" + s + "-" + sid(b.chiave_bozza);
  const no = perchéNoModifica();
  if (b.codice === "D1_ULTIMO_FIGLIO") return [
    btn("Diventa particolare", () => decisioneWizard("D1: diventa particolare", { id: nuovoIdDecisione("d1"), tipo: "d1_a_particolare", chiave_bozza: b.chiave_bozza }), { cls: "du-btn--sm", id: id("particolare"), no, perche: false }),
    btn("Lascia assieme", () => decisioneWizard("D1: resta assieme", { id: nuovoIdDecisione("d1"), tipo: "d1_resta_sottoassieme", chiave_bozza: b.chiave_bozza }), { cls: "du-btn--sm", id: id("resta"), no, perche: false })];
  if (b.codice === "D1_PARTICOLARE_CON_FIGLI") return [
    btn("Diventa assieme", () => decisioneWizard("D1: diventa assieme", { id: nuovoIdDecisione("d1"), tipo: "d1_a_sottoassieme", chiave_bozza: b.chiave_bozza }), { cls: "du-btn--sm", id: id("assieme"), no, perche: false })];
  return null;
}
// ---- il riuso di un componente registrato (decisione riusa_esistente; EM §3.6 riga 8, §5.2 W3, §6.5, §6.6; C3:354, :514)

// Il rimedio di CREA_SU_ESISTENTE: un nodo da creare ha il codice di un componente già registrato (tipico dopo «Usa nuovo
// STEP» in una revisione, cambio-fonte-1 [T]: i nodi registrati dello STEP di prima escono e le voci dello STEP nuovo
// entrano come pezzi nuovi con gli stessi codici; EM §6.6 passi 4 e 6). «Riusa ‹C›» scrive con R2 la decisione
// riusa_esistente {chiave_bozza, componente_id}: il nodo resta lo stesso (stessa chiave, stessa casella dello STEP nuovo) e
// alla conferma va nel componente registrato, che così non è più un'assenza (ASSENZA_NON_DECISA) né un registrato che esce
// con i suoi file (nodo_uscente_con_ar). Niente si unisce da solo: il gesto è dell'ingegnere (EM §6.5, «il riuso lo decide
// l'ingegnere»). Il server verifica emittente e identità (W3: con emittenti diversi il blocco è COLLISIONE_EMITTENTI, non
// questo). Rilievo del revisore del 20/10, scelta [T-39] della corsia.

// decisioneRiuso: la decisione di riuso già nella bozza per il nodo k, o null.
function decisioneRiuso(k) {
  return arr(S.lavoro && S.lavoro.decisioni).find((d) => (d.tipo === "riusa_esistente" || d.tipo === "riusa_per_canonico") && d.chiave_bozza === k) || null;
}
// perchéNoRiuso: «Riusa ‹C›» vuole un nodo della bozza da creare (senza componente), un componente diverso dal prodotto e
// non già riusato; o "".
function perchéNoRiuso(k, cid) {
  const no = perchéNoModifica();
  if (no) return no;
  const n = S.lavoro && S.lavoro.nodi.find((x) => x.chiave_bozza === k);
  if (!n) return "il nodo non è più nella bozza: rileggi il riepilogo";
  if (n.componente_id || n.op !== "crea") return "il nodo è già un componente registrato";
  if (!cid) return "il server non dice quale componente registrato ha lo stesso codice";
  if (String(cid).toLowerCase() === S.prodotto) return "è il prodotto della RFQ: non si riusa come pezzo";
  const d = decisioneRiuso(k);
  if (d && String(d.componente_id).toLowerCase() === String(cid).toLowerCase()) return "il nodo riusa già questo componente";
  return "";
}
// azioniRiuso: le azioni di un riuso (una decisione_tolta del riuso di prima, se il nodo ne aveva uno verso un altro
// componente, poi la decisione_aggiunta).
function azioniRiuso(k, cid) {
  const out = [];
  const prima = decisioneRiuso(k);
  if (prima) out.push({ azione: "decisione_tolta", chiave_bozza: chiaveEvento(prima), prima: clona(prima), dopo: null, origine: "utente" });
  const dec = { id: nuovoIdDecisione("ri"), tipo: "riusa_esistente", chiave_bozza: k, componente_id: String(cid).toLowerCase() };
  out.push({ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
  return out;
}
// riusabili: i blocchi CREA_SU_ESISTENTE con un solo componente per nodo (un nodo che collide con due registrati non si
// riusa in blocco: si sceglie blocco per blocco), uno per nodo.
function riusabili(blocchi) {
  const per = new Map();
  for (const b of blocchi) {
    if (b.codice !== "CREA_SU_ESISTENTE" || !b.chiave_bozza || !b.componente_id) continue;
    if (!per.has(b.chiave_bozza)) per.set(b.chiave_bozza, new Set());
    per.get(b.chiave_bozza).add(String(b.componente_id).toLowerCase());
  }
  const visti = new Set();
  return blocchi.filter((b) => {
    if (b.codice !== "CREA_SU_ESISTENTE" || !b.chiave_bozza || !b.componente_id || per.get(b.chiave_bozza).size !== 1 || visti.has(b.chiave_bozza)) return false;
    visti.add(b.chiave_bozza);
    return !perchéNoRiuso(b.chiave_bozza, b.componente_id);
  });
}
// gestoRiuso: «Riusa ‹C›» nel blocco CREA_SU_ESISTENTE del riepilogo; un salvataggio, poi il riepilogo si rilegge.
function gestoRiuso(b) {
  if (b.codice !== "CREA_SU_ESISTENTE" || !b.chiave_bozza) return null;
  const cod = b.chiave_working || codiceComponenteR4(S.wizard && S.wizard.riepilogo, b.componente_id);
  return btn("Riusa " + cod, () => rimedioWizard("riusato il componente registrato " + cod, azioniRiuso(b.chiave_bozza, b.componente_id)),
    { cls: "du-btn--sm", id: "du-wz-riusa-" + sid(b.chiave_bozza), no: perchéNoRiuso(b.chiave_bozza, b.componente_id), perche: false });
}
// riusoInBlocco: con due o più nodi da riusare, «Riusa i componenti con lo stesso codice (n)», il testo del rimedio del
// server, in un solo salvataggio.
function riusoInBlocco(blocchi) {
  const l = riusabili(blocchi);
  if (l.length < 2) return null;
  return h("p", { class: "du-azioni", id: "du-wz-riusa-blocco" }, btn("Riusa i componenti con lo stesso codice (" + l.length + ")",
    () => rimedioWizard("riusati " + l.length + " componenti registrati con lo stesso codice", l.flatMap((b) => azioniRiuso(b.chiave_bozza, b.componente_id))),
    { cls: "du-btn--sm", id: "du-wz-riusa-tutti", perche: false }),
    h("span", { class: "du-muto du-piccolo" }, " Ogni pezzo nuovo va nel componente registrato con il suo codice: niente si unisce da solo, e «Annulla» toglie i riusi."));
}
// riusoNelPannello: «Riusa ‹C›» per un nodo da creare la cui voce porta un componente registrato (decisione_esistente: per
// esempio dopo la conferma di un altro prodotto con lo stesso STEP, o un nodo «da rivedere» con quel motivo; EM §6.5); con
// il riuso già deciso, la riga e «Togli il riuso».
function riusoNelPannello(g, v) {
  if (!v.nodo || v.noninc || v.nodo.componente_id || v.nodo.op !== "crea") return null;
  const d = decisioneRiuso(v.k);
  if (d) {
    return h("div", { class: "du-gesto", id: "du-riuso-nodo", "data-componente": d.componente_id || "" }, pill("info", "riusa il componente registrato " + (v.cod || "")),
      h("span", { class: "du-muto du-piccolo" }, " alla conferma il pezzo va nel componente registrato, non se ne crea uno nuovo"),
      btn("Togli il riuso", () => fai("tolto il riuso di " + (v.cod || v.k), [{ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" }]),
        { cls: "du-btn--sm du-btn--ghost", id: "du-togli-riuso", no: perchéNoModifica() }));
  }
  const de = v.voce && ogg(v.voce.decisione_esistente) ? v.voce.decisione_esistente : null;
  if (!de || !de.componente_id || String(de.componente_id).toLowerCase() === S.prodotto) return null;
  const cod = de.codice || (ogg(de.identita_registrata) && de.identita_registrata.codice) || v.cod || "il componente registrato";
  return btn("Riusa " + cod, () => fai("riusato il componente registrato " + cod, azioniRiuso(v.k, de.componente_id)),
    { cls: "du-btn--sm", id: "du-riusa-esistente", no: perchéNoRiuso(v.k, de.componente_id) });
}

// ---- la sorgente, i documenti e la condivisione nel riepilogo formato 3 (direttiva 39 §4, §5; EM §9.4, App. I.8)

// codiceComponenteR4: il codice di un componente registrato per i testi del riepilogo: dalle identità del riepilogo, dai nodi
// della bozza, dai prodotti della RFQ; altrimenti «un componente registrato»
function codiceComponenteR4(r, cid) {
  if (!cid) return "?";
  const c = String(cid).toLowerCase();
  const x = arr(r && r.identita).find((y) => y.componente_id && String(y.componente_id).toLowerCase() === c);
  if (x) return (x.codice && (x.codice.prima || x.codice.dopo)) || (x.decisa && x.decisa.codice) || c.slice(0, 8);
  const n = S.lavoro && S.lavoro.nodi.find((y) => y.componente_id && String(y.componente_id).toLowerCase() === c);
  if (n && n.identita && n.identita.codice) return n.identita.codice;
  for (const cc of arr(r && r.condivisione)) for (const e of arr(cc.elementi)) if (e.componente_id && String(e.componente_id).toLowerCase() === c && e.codice) return e.codice;
  if (S.modello && S.modello.prodotti.some((p) => p.id === c)) return codiceDi(c);
  // mai un pezzo dell'identificativo (UI46 [U], messaggio 48: niente UUID nel wizard)
  return "un componente registrato";
}
// sezioneSorgenteWizard: lo STEP sorgente della bozza (direttiva 39 §2, §5 [U]): la riga «legato come sorgente» in sola
// lettura e fuori dalle preassociazioni (lo STEP sorgente del finito non è mai un documento da associare né una AR), i
// legami storici di D-AR04 (iii) (non valgono come sorgente finché non si riconfermano con S8), l'effetto sulla fonte della
// working (la promuove solo «Conferma BOM»).
// testoServer: un testo del server per il wizard, in parole (aParole: niente «da S8», «commit», accenti con l'apostrofo;
// collaudo UI46, I4), con il punto finale
function testoServer(t) {
  const x = aParole(t);
  return x ? x + "." : "";
}
function sezioneSorgenteWizard(r) {
  const box = h("div", { class: "du-sez", id: "du-wz-sorgente" }, h("h3", {}, "STEP sorgente"));
  const l = ogg(r.sorgente_legata) ? r.sorgente_legata : null;
  if (l) {
    box.append(h("p", { id: "du-wz-sorgente-legata", "data-confermata": l.confermata ? "1" : "0" },
      pill(l.confermata ? "ok" : "ver", l.confermata ? "◆ legato come sorgente" : "◇ sorgente non confermata"), " ", h("span", { class: "du-mono" }, l.nome_file || nomeFile(l.allegato_id)), " — " + testoServer(l.testo),
      h("span", { class: "du-muto du-piccolo" }, " (sola lettura)")));
  } else box.append(h("p", { id: "du-wz-sorgente-legata", "data-confermata": "0" }, "La bozza non ha uno STEP: parte dal solo prodotto."));
  for (const x of arr(r.legami_storici)) box.append(h("p", { class: "du-nota", "data-du-legame-storico": x.documento_id || "" }, pill("ver", "legame storico"), " ", h("span", { class: "du-mono" }, x.nome_file || ""), " — " + testoServer(x.testo)));
  const e = ogg(r.effetto_sulla_fonte) ? r.effetto_sulla_fonte : null;
  if (e && e.testo) box.append(h("p", { class: "du-nota", id: "du-wz-effetto-fonte", "data-promozione": e.promozione ? "1" : "0" }, testoServer(e.testo)));
  // G39-03 [U]: la conferma che corregge uno STEP sorgente sbagliato dopo una BOM confermata è una revisione interna, non del
  // cliente (R4 effetto_sulla_fonte.correzione_sorgente; R5 la scrive nell'origine della conferma, R7 la rilegge)
  if (e && e.correzione_sorgente) box.append(h("p", { class: "du-esito du-esito--ver", id: "du-wz-correzione-sorgente", "data-revisione": "interna" }, pill("ver", "correzione della sorgente"),
    " La conferma sarà una revisione interna: corregge uno STEP sorgente sbagliato. Non è una revisione del cliente, anche se cambiano dei codici."));
  return box;
}
const GRUPPI_PB = [["motore_intatta", "Dal motore, intatte"], ["corretta", "Corrette dall'ingegnere"], ["manuale", "Manuali"], ["legacy_da_verificare", "Legacy da verificare"],
  ["legacy_verificate", "Legacy riconosciute"]];
const ESITI_PB = { applicabile: ["ok", "si registra"], gia_applicata: ["info", "già registrata"], sostituzione: ["ver", "sostituisce l'associazione di prima"], non_richiesta: ["neu", "non richiesta"],
  in_attesa_codice: ["ver", "in attesa del codice"], in_conflitto: ["ko", "in conflitto"] };
// rimedioWizard: un rimedio del riepilogo è un salvataggio della bozza come gli altri, poi il riepilogo si rilegge
async function rimedioWizard(etichetta, azioni) {
  if (!azioni.length || !fai(etichetta, azioni)) return;
  const ok = await salvaOra();
  if (!ok) return;
  await rileggiRiepilogo();
}
// rimediPB: i rimedi di una PB in conflitto, come gesti della bozza (D-PB-04 A [U]: la legacy da verificare ha il suo rimedio
// esplicito, mai una ratifica silenziosa). Sempre «Togli la preassociazione»; per gia_associata_altrove dello stesso
// prodotto anche lo svincolo della AR di prima nella stessa bozza (la sostituzione, mai un trasferimento silenzioso); per
// un altro prodotto il testo dice la strada (lo svincolo si fa nella bozza di quel prodotto: D-ASS-02, D-TAB-01).
function rimediPB(r, d) {
  const out = [];
  const no = perchéNoModifica();
  const pb = pbDi(d.allegato_id);
  const ids = (x) => "du-wz-rimedio-" + x + "-" + sid(d.allegato_id);
  // R39-15: la PB legacy da verificare si riconosce con un gesto esplicito (file_riconosciuto, R2): resta legacy e diventa
  // verificata; non si annulla (per tornare indietro la PB si toglie)
  if (pb && d.motivo === "legacy_da_verificare") out.push(btn("Riconosci…", () => apriRiconosci(pb, d), { cls: "du-btn--sm", id: ids("riconosci"), no, perche: false }));
  if (pb) out.push(btn("Togli la preassociazione", () => rimedioWizard("tolta la preassociazione di " + nomeFile(d.allegato_id), [{ azione: "file_svincolato", chiave_bozza: pb.chiave_bozza, prima: clona(pb), dopo: { allegato_id: d.allegato_id, motivo: "utente" }, origine: "utente" }]),
    { cls: "du-btn--sm", id: ids("togli"), no, perche: false }));
  if (d.motivo === "gia_associata_altrove" && d.variante === "stesso_prodotto" && d.ar_componente_id && !svincoloDi(d.allegato_id)) {
    const cod = codiceComponenteR4(r, d.ar_componente_id);
    out.push(btn("Scollega da " + cod + " e sostituisci", () => {
      const dec = { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", allegato_id: d.allegato_id, componente_id: d.ar_componente_id, motivo: "sostituita da una preassociazione della stessa bozza" };
      return rimedioWizard("da scollegare " + nomeFile(d.allegato_id) + " da " + cod, [{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" }]);
    }, { cls: "du-btn--sm", id: ids("sostituisci"), no, perche: false }));
  }
  if (d.chiave_bozza) out.push(btn("Mostra nel grafo", () => mostraNelGrafo(d.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false }));
  return out;
}
// apriRiconosci: il dialogo del riconoscimento di una PB legacy (R39-15; direttiva 39 §4, D-PB-04 A [U]: rimedio esplicito,
// mai una ratifica silenziosa). Dal messaggio 46 [U] senza motivo; il salvataggio porta file_riconosciuto {allegato_id, chiave_bozza}
// e lo snapshot invariato (la verifica la decide il server). Il passo non entra nella pila: non si annulla.
function apriRiconosci(pb, d) {
  const nome = d.nome_file || nomeFile(d.allegato_id);
  apriFinestra("Riconosci la preassociazione legacy", h("div", { style: "display:grid;gap:8px", id: "f-riconosci" },
    h("p", {}, "«" + nome + "» → " + voceEffetto(pb.chiave_bozza) + " viene da una pre-assegnazione delle pagine di prima. Riconoscendola dichiari di averla verificata: resta «legacy» e la conferma la registrerà così."),
    h("p", { class: "du-nota" }, "Il riconoscimento non si annulla: per tornare indietro togli la preassociazione. Se la sposti su un altro nodo diventa manuale.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Riconosci", pri: true, id: "f-ok", fai: async () => {
    const dopo = { allegato_id: pb.allegato_id, chiave_bozza: pb.chiave_bozza };
    const az = { azione: "file_riconosciuto", chiave_bozza: pb.chiave_bozza, prima: { allegato_id: pb.allegato_id, chiave_bozza: pb.chiave_bozza }, dopo, origine: "utente" };
    if (!fai("riconosciuta la preassociazione legacy di " + nome, [az])) return false;
    // fuori dalla pila d'annullamento (il server non lo annulla)
    const passo = S.pila[S.pila.length - 1];
    if (passo && passo.voci[0] && passo.voci[0].inviare[0] === az) S.pila.pop();
    renderBarra();
    chiudiFinestra();
    const ok = await salvaOra();
    if (ok && S.wizard) await rileggiRiepilogo();
    return false;
  } }]);
}
// testoConflittoPB: il motivo di una PB in conflitto, con il testo del blocco del server e le varianti di gia_associata_altrove
function testoConflittoPB(r, d) {
  const b = arr(r.blocchi).find((x) => x.codice === "PREASSOCIAZIONE_NON_APPLICABILE" && x.allegato_id && String(x.allegato_id).toLowerCase() === String(d.allegato_id).toLowerCase());
  let t = (b && b.testo) || d.motivo || "in conflitto";
  if (d.motivo === "gia_associata_altrove") {
    const cod = codiceComponenteR4(r, d.ar_componente_id || d.copia_componente_id);
    if (d.variante === "stesso_prodotto") t += " È registrato su " + cod + ", nello stesso prodotto: scollegalo nella stessa bozza (sostituzione) o togli la preassociazione.";
    else if (d.variante === "altro_prodotto") t += " È registrato su " + cod + (arr(d.ar_prodotti).length ? " di " + arr(d.ar_prodotti).map((x) => codiceDi(String(x).toLowerCase())).join(", ") : "") + ", un altro prodotto: si scollega nella bozza di quel prodotto (niente trasferimenti fra prodotti), oppure togli la preassociazione.";
    else if (d.variante === "copia_legacy") t += " È pre-assegnato a " + cod + " dalle pagine di prima: la pre-assegnazione si converte quando quel prodotto viene preso in carico; intanto togli la preassociazione.";
  }
  const rim = b ? arr(b.rimedi) : [];
  return t + (rim.length ? " Rimedi: " + rim.join("; ") + "." : "");
}
// nomeNodoWizard: il pezzo k per chi legge il wizard, il codice e, se c'è, la descrizione; mai la chiave interna del nodo
// (UI46 [U], messaggio 48: le preassociazioni mostravano «(k-c-…)»). Dalla bozza, poi dalle identità del riepilogo (dopo la
// conferma la bozza non c'è più), poi dal codice proposto o dal nome della voce dello STEP; altrimenti «pezzo senza codice».
function nomeNodoWizard(k, r) {
  const n = S.lavoro && k ? S.lavoro.nodi.find((y) => y.chiave_bozza === k) : null;
  const x = k ? arr(r && r.identita).find((y) => y.chiave_bozza === k) : null;
  const pd = (z) => (ogg(z) ? z.dopo || z.prima || "" : typeof z === "string" ? z : "");
  let cod = (n && n.identita && n.identita.codice) || (x && (pd(x.codice) || (x.decisa && x.decisa.codice))) || "";
  const des = (n && n.descrizione) || (x && pd(x.descrizione)) || "";
  if (!cod && n && casellaDi(n)) {
    const v = voceDiCasella(registroBozza(), casellaDi(n));
    cod = (v && (v.codice_proposto || (v.grezzi && v.grezzi.nome))) || "";
  }
  if (!cod) return des ? des + " (pezzo senza codice)" : "pezzo senza codice";
  return des && des !== cod ? cod + " · " + des : cod;
}
// sezioneDocumentiWizard: che cosa registra la conferma per i documenti (D-PB-04 A [U]): le preassociazioni raggruppate per
// provenienza («dal motore, intatte», «corrette», «manuali», «legacy da verificare»), le proposte del motore tolte, gli
// spostamenti delle associazioni registrate (sostituzioni e svincoli) e quelle che restano. «Conferma BOM» è
// l'approvazione cumulativa: nessuna spunta per singola preassociazione; una AR non è la conferma del documento.
function sezioneDocumentiWizard(w, r) {
  const ed = ogg(r.effetti_documentali) ? r.effetti_documentali : {};
  const perAllegato = new Map(arr(r.documenti_preassociati).map((d) => [String(d.allegato_id).toLowerCase(), d]));
  const box = h("div", { class: "du-sez", id: "du-wz-documenti" }, h("h3", {}, "Documenti: che cosa registra la conferma"),
    h("p", { class: "du-nota", id: "du-wz-cumulativa" }, "«Conferma BOM» approva insieme tutte le preassociazioni che si possono registrare: non ci sono spunte per singolo file. L'associazione registrata non è la conferma del documento, che resta del fascicolo."));
  for (const [g, t] of GRUPPI_PB) {
    const voci = arr(ed[g]);
    const sez = h("div", { class: "du-wz-gruppo", "data-gruppo": g, id: "du-wz-gruppo-" + g }, h("h4", { class: "du-lab" }, t + " (" + voci.length + ")"));
    if (g === "legacy_da_verificare" && voci.length) sez.append(h("p", { class: "du-nota" }, "Vengono da pre-assegnazioni delle pagine di prima: la conferma non le ratifica in silenzio. Riconoscile una per una («Riconosci…»: restano legacy, verificate), togli la preassociazione, o rifalla a mano (diventa manuale)."));
    if (g === "legacy_verificate" && voci.length) sez.append(h("p", { class: "du-nota" }, "Pre-assegnazioni delle pagine di prima riconosciute in questa bozza: la conferma le registra con la provenienza «legacy». Spostate su un altro nodo diventano manuali."));
    if (voci.length) {
      sez.append(h("ul", { class: "du-elenco-p" }, voci.map((v) => {
        const d = perAllegato.get(String(v.allegato_id).toLowerCase()) || v;
        const es = ESITI_PB[d.esito] || ["neu", d.esito || ""];
        const conflitto = d.esito === "in_conflitto";
        return h("li", { "data-du-pb": v.allegato_id, "data-esito": d.esito || "", "data-motivo": d.motivo || "" },
          h("span", { class: "du-mono" }, d.nome_file || nomeFile(v.allegato_id)), " → ", h("span", { class: "du-mono" }, nomeNodoWizard(v.chiave_bozza, r)), " ", pill(es[0], es[1]),
          // «si registra» lo dice già la pillola, e la nota sotto il gruppo dice che non è la conferma del documento: l'effetto
          // del server, uguale su ogni riga, si scrive solo negli altri esiti (collaudo UI46, giro 3, minore) [T]
          d.effetto && d.esito !== "applicabile" ? h("span", { class: "du-muto du-piccolo" }, " " + d.effetto) : null,
          conflitto ? h("div", { class: "du-err du-piccolo" }, testoConflittoPB(r, d)) : null,
          conflitto && bozzaLavorabile() ? h("span", { class: "du-azioni" }, rimediPB(r, d)) : null);
      })));
    }
    box.append(sez);
  }
  const rim = arr(ed.proposte_rimosse);
  if (rim.length) box.append(h("div", { class: "du-wz-gruppo", "data-gruppo": "proposte_rimosse", id: "du-wz-gruppo-proposte_rimosse" }, h("h4", { class: "du-lab" }, "Proposte del motore tolte (" + rim.length + ")"),
    h("ul", { class: "du-elenco-p" }, rim.map((x) => h("li", { "data-du-pb": x.allegato_id }, h("span", { class: "du-mono" }, nomeFile(x.allegato_id)), " proposto per ", h("span", { class: "du-mono" }, nomeNodoWizard(x.chiave_bozza, r)), ": tolto, non si registra")))));
  const sp = arr(ed.spostamenti_ar);
  box.append(h("div", { class: "du-wz-gruppo", "data-gruppo": "spostamenti_ar", id: "du-wz-gruppo-spostamenti_ar" }, h("h4", { class: "du-lab" }, "Associazioni registrate che cambiano (" + sp.length + ")"),
    sp.length ? h("ul", { class: "du-elenco-p" }, sp.map((x) => h("li", { "data-du-spostamento": x.allegato_id, "data-tipo": x.tipo || "", "data-esito": x.esito || "" },
      h("span", { class: "du-mono" }, nomeFile(x.allegato_id)), ": ",
      x.tipo === "sostituzione" ? "da " + codiceComponenteR4(r, x.da_componente_id) + " a " + (x.a_chiave_bozza ? nomeNodoWizard(x.a_chiave_bozza, r) : codiceComponenteR4(r, x.a_componente_id)) + " (l'associazione di prima si chiude, la storia resta)" :
        "scollegato da " + codiceComponenteR4(r, x.da_componente_id) + (x.esito === "svincolo_gia_chiuso" ? " (già chiusa da un gesto dei file)" : " alla conferma"),
      x.esito && x.esito !== "applicabile" && x.esito !== "sostituzione" && x.esito !== "svincolo_gia_chiuso" ? h("span", { class: "du-err du-piccolo" }, " " + (x.motivo || x.esito)) : null))) :
      h("p", { class: "du-muto du-piccolo" }, "Nessuna.")));
  const inv = arr(ed.ar_invariate);
  if (inv.length) box.append(h("div", { class: "du-wz-gruppo", "data-gruppo": "ar_invariate", id: "du-wz-gruppo-ar_invariate" }, h("h4", { class: "du-lab" }, "Associazioni registrate che restano (" + inv.length + ")"),
    h("ul", { class: "du-elenco-p" }, inv.map((x) => h("li", { "data-du-ar": x.allegato_id, "data-motivo": x.motivo || "" }, h("span", { class: "du-mono" }, nomeFile(x.allegato_id)), " su " + codiceComponenteR4(r, x.componente_id) + ": " + (x.testo || x.motivo || ""))))));
  return box;
}
// sezioneCondivisione: gli altri prodotti finiti che l'atto tocca (EM §7.7; D-TAB-03 A [T-39]): i componenti condivisi o lo
// stesso STEP, con l'effetto, la conseguenza attribuita a chi conferma e la validità della loro ultima conferma. Confermare
// questo prodotto non conferma l'altro.
function sezioneCondivisione(r) {
  const c = arr(r.condivisione);
  if (!c.length) return null;
  return h("div", { class: "du-sez", id: "du-wz-condivisione" }, h("h3", {}, "Altri prodotti toccati (" + c.length + ")"),
    h("ul", { class: "du-elenco-p" }, c.map((x) => {
      // gli elementi «documento» (704f6bf: un componente condiviso di cui l'atto cambia solo le associazioni) non cambiano la
      // working dell'altro prodotto: la sua ultima conferma resta valida, e il server non lo dice nella conseguenza
      // (riepilogo3_regole.go, solo con elementi strutturali). Il segno segue la stessa regola (revisione 2 della corsia)
      const strutturali = arr(x.elementi).some((e) => e.tipo !== "documento");
      return h("li", { "data-du-condivisione": x.prodotto_id, "data-strutturale": strutturali ? "1" : "0" },
        h("b", {}, x.codice || codiceDi(String(x.prodotto_id).toLowerCase())), ": ",
        arr(x.elementi).map((e) => (e.tipo === "step" ? "lo stesso STEP" : e.tipo === "documento" ? "i documenti di " + (e.codice || codiceComponenteR4(r, e.componente_id)) : e.codice || codiceComponenteR4(r, e.componente_id)) +
          (e.effetto ? " (" + e.effetto + ")" : "")).join("; "), ". ",
        h("span", { class: "du-muto" }, x.conseguenza || ""), x.conferma_valida_prima && strutturali ? pill("ver", "la sua ultima conferma può non valere più") : null);
    })));
}
async function decisioneWizard(etichetta, dec) {
  if (!fai(etichetta, [{ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" }])) return;
  const ok = await salvaOra();
  if (!ok) return;
  await rileggiRiepilogo();
}
// apriMonoPezzo: «Dichiara mono_pezzo…» (MAT BZ-14). Dal messaggio 46 [U] senza motivo: il dialogo dice che cosa
// dichiara; alla conferma della BOM il server registra la traccia del gesto (conferma_strutturale.mono_pezzo_motivo).
function apriMonoPezzo() {
  apriFinestra("Dichiara mono_pezzo", h("p", { id: "f-mono-pezzo" }, "Dichiari che " + (codiceProdotto() || "il prodotto") + " è un pezzo unico, senza figli. La conferma della BOM lo registra, con lo STEP del prodotto confermato come sorgente."),
    [{ testo: "Annulla", chiudi: true }, { testo: "Dichiara", pri: true, id: "f-ok", fai: async () => {
      await decisioneWizard("dichiarato mono_pezzo", { id: nuovoIdDecisione("mp"), tipo: "mono_pezzo" });
    } }]);
}
async function confermaWizard() {
  const w = S.wizard;
  if (perchéNoConfermaWizard(w)) return;
  w.stato = "conferma";
  w.corpo = corpoBozza();
  w.firma = w.riepilogo.firma_anteprima;
  w.esito = { tipo: "in_corso" };
  // la presa d'atto porta l'elenco dei documenti in analisi mostrati (U2 §6.3.7): il server lo confronta con il suo
  w.presa = arr(w.mostrati).map((x) => x.allegato_id);
  fermaIngressi();
  renderWizard();
  renderBarra();
  const r = await P.confermaBom(w.firma, w.corpo, { presa: w.presa });
  await esitoConferma(r);
}
async function esitoConferma(r) {
  const w = S.wizard;
  if (!w) return;
  w.stato = "pronto";
  if (r.esito === "ok") {
    w.esito = { tipo: "ok", dati: r.dati };
    renderWizard();
    await dopoConferma();
    return;
  }
  if (r.esito === "da_verificare") {
    w.esito = { tipo: "verifica", chiave: r.chiave, testo: "Non so se la conferma è registrata: verifico con la stessa chiave." };
    renderWizard();
    // la verifica va avanti anche se chi guarda chiude il wizard: su «registrato» la pagina si rilegge comunque (MAT §5.5)
    const codice = w.codice;
    verificaConRiprova(r.chiave, async (j) => {
      const wz = S.wizard;
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        const dati = j.stato === "registrato" ? j.esito || {} : j;
        if (wz) {
          wz.esito = { tipo: "ok", dati, verificato: true };
          renderWizard();
        } else avvisa("ok", "Verificato: la conferma di " + codice + " è registrata.");
        await dopoConferma(dati);
        return true;
      }
      if (j.stato === "non_registrato") {
        if (wz) {
          wz.esito = { tipo: "non_registrata", chiave: r.chiave };
          renderWizard();
        } else avvisa("no", "La conferma di " + codice + " non è ancora registrata: «Riprova la conferma…» dalla striscia, con lo stesso corpo e la stessa chiave.");
        render();
        return true;
      }
      if (j.stato === "in_corso" && wz) {
        wz.esito = { tipo: "verifica", chiave: r.chiave, testo: "Una conferma per questa RFQ è ancora in corso: verifico fra poco." };
        renderWizard();
      }
      return false;
    });
    return;
  }
  if (r.esito === "non_registrato") {
    w.esito = { tipo: "controllo_finale", testo: r.testo };
    renderWizard();
    return;
  }
  // 503 CALCOLO_OCCUPATO di R5 (esito non_registrato): niente è partito; il ponte tiene la chiave e «Riprova» la riusa. Lo
  // stesso per il motore nella connessione S scaduto (504 CALCOLO_SCADUTO) o non riuscito (500 ERRORE_CALCOLO; blocco
  // «R4 completo» della persistenza)
  if (r.esito === "errore" && ["CALCOLO_OCCUPATO", "CALCOLO_SCADUTO", "ERRORE_CALCOLO"].includes(r.codice)) {
    w.esito = { tipo: "occupato", codice: r.codice, testo: r.testo, chiave: r.chiave };
    programmaIngressi();
    renderWizard();
    return;
  }
  w.esito = { tipo: r.esito, codice: r.codice, motivo: r.motivo, testo: r.testo, dati: r.dati, chiave: r.chiave };
  if (r.codice === "ANTEPRIMA_SUPERATA" && r.dati && r.dati.riepilogo) {
    w.esito.diff = differenzeRiepiloghi(w.riepilogo, r.dati.riepilogo);
    w.riepilogo = r.dati.riepilogo;
    w.versione = S.bozza.versione;
    w.ingressi = partiIngressiR4(w.riepilogo.ingressi) || w.ingressi;
    w.inAnalisi = arr(w.riepilogo.file_in_analisi);
    w.mostrati = w.inAnalisi.slice();
    w.presaAnalisi = false;
    w.n1 = "";
    w.n2 = null;
  }
  programmaIngressi();
  renderWizard();
  if (r.codice === "BOZZA_VERSIONE" || r.codice === "BASE_SUPERATA") {
    // la bozza salvata non è quella confermata, o la base è superata: si rilegge R1, che dice la base e offre il rebase
    // (MAT §5.2)
    await caricaBozza();
    render();
    renderWizard();
  } else if (r.codice === "CHIAVE_RIUSATA") {
    // «Questo tentativo ha già un esito diverso»: R6 con la chiave, poi R1 e R7; il prossimo tentativo avrà una chiave
    // nuova (MAT §5.2)
    let j = null;
    try {
      j = r.chiave ? await P.esitoIntento(r.chiave) : null;
    } catch (e) {
      j = null;
    }
    await Promise.all([caricaBozza(), caricaStorico()]);
    if (S.wizard) S.wizard.esito.verifica = j ? j.stato : "non letta";
    render();
    renderWizard();
  }
}
function esitoWizard(w) {
  const e = w.esito;
  if (e.tipo === "in_corso") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "in_corso" }, "Conferma in corso…");
  if (e.tipo === "ok") {
    const d = e.dati || {};
    const n = d.numero_conferma || "?";
    // i nodi non inclusi applicati, con il nome dal riepilogo firmato: per ripristinarli serve una bozza nuova (MAT NI-10)
    // R5 formato 3 dà i conti come numeri (non_inclusi_applicati, associazioni_applicate, svincoli_applicati): i nomi dei
    // non inclusi vengono dal riepilogo firmato che la conferma ha applicato
    const conta = (x) => (typeof x === "number" ? x : arr(x).length);
    const nonInc = Array.isArray(d.non_inclusi_applicati) ? d.non_inclusi_applicati.map((x) => {
      const y = nonInclusiDi(w.riepilogo).find((z) => z.id === x);
      return y ? nomeNonIncluso(y, w.riepilogo) : "voce senza nome";
    }) : conta(d.non_inclusi_applicati) ? nonInclusiDi(w.riepilogo).map((y) => nomeNonIncluso(y, w.riepilogo)) : [];
    const fo = ogg(d.fonte) ? d.fonte : null;
    return h("div", { class: "du-esito du-esito--ok", id: "du-wz-esito", "data-esito": "ok" }, h("b", {}, "Conferma n. " + n + " di " + (w.codice || codiceProdotto()) + (e.verificato ? " (verificata con la stessa chiave)" : "") + "."),
      h("ul", { class: "du-elenco-p" }, h("li", {}, "Identità accettate: " + conta(d.identita_accettate) + ", corrette: " + conta(d.identita_corrette)),
        h("li", {}, "Non inclusi applicati: " + conta(d.non_inclusi_applicati)),
        h("li", { id: "du-wz-esito-associazioni" }, "Associazioni registrate: " + conta(d.associazioni_applicate) + (conta(d.associazioni_gia_applicate) ? " (già registrate: " + conta(d.associazioni_gia_applicate) + ")" : "") +
          ", svincoli: " + conta(d.svincoli_applicati) + (conta(d.svincoli_gia_chiusi) ? " (già chiusi: " + conta(d.svincoli_gia_chiusi) + ")" : "")),
        // R5 completo (persistenza): la ricodifica applicata (le associazioni restano sulla stessa riga, con una correzione
        // ciascuna), i ripristini, i cambi di tipo, le voci assenti chiuse
        conta(d.identita_ricodificate) || conta(d.correzioni_ricodifica) ? h("li", { id: "du-wz-esito-ricodifiche" }, "Componenti ricodificati: " + conta(d.identita_ricodificate) +
          " (associazioni registrate allineate al codice nuovo: " + conta(d.correzioni_ricodifica) + ")") : null,
        conta(d.identita_ripristinate) ? h("li", { id: "du-wz-esito-ripristinati" }, "Componenti archiviati ripristinati: " + conta(d.identita_ripristinate)) : null,
        conta(d.tipi_cambiati) ? h("li", { id: "du-wz-esito-tipi" }, "Tipi cambiati: " + conta(d.tipi_cambiati)) : null,
        conta(d.assenze_applicate) ? h("li", { id: "du-wz-esito-assenze" }, "Voci dello STEP non rappresentate, chiuse dalla conferma: " + conta(d.assenze_applicate)) : null,
        (fo && fo.correzione_sorgente) || d.correzione_sorgente ? h("li", { id: "du-wz-esito-correzione", "data-revisione": "interna" }, "Revisione interna: correzione della sorgente, non una revisione del cliente.") : null,
        fo && fo.promossa ? h("li", { id: "du-wz-esito-fonte" }, "Lo STEP sorgente è diventato lo STEP della BOM confermata" + (fo.sostituzione_di ? "; lo STEP di prima resta, sostituito, con la sua storia" : "") + ".") : null,
        conta(d.esclusi_condivisi) ? h("li", {}, "Esclusi condivisi con altri prodotti (restano nella BOM registrata): " + conta(d.esclusi_condivisi)) : null,
        d.f8_stato ? h("li", { id: "du-wz-esito-f8", "data-f8": d.f8_stato }, "Smistamento dei file dopo la conferma: " + (d.f8_stato === "eseguito" ? "fatto" : d.f8_stato === "errore" ? "non riuscito, si riprende con «Riallinea i documenti»" : "in attesa")) : null),
      nonInc.length ? h("div", { id: "du-wz-noninclusi-conferma" }, h("b", {}, "Nodi non inclusi nella conferma n. " + n + ":"), h("ul", { class: "du-elenco-p" }, nonInc.map((x) => h("li", {}, x))),
        h("p", { class: "du-nota" }, "Per ripristinarne uno serve una bozza nuova («Modifica la BOM confermata», poi «Includi di nuovo»).")) : null,
      // i segni distinti di U2 §5.5 (WZ-09): la conferma registrata e la verifica corrente del server, letta da /dati dopo
      // la conferma (mai salvata, mai decisa dalla pagina); finché /dati non è riletto, la verifica si dice non letta
      h("p", { id: "du-wz-segni" }, pill("ok", "v" + n + " confermata"), " ", S.modello && S.modello.confermaCorrente && d.conferma_id && S.modello.confermaCorrente.conferma_id === d.conferma_id ? segnoVerifica("wz") :
        pill("neu", "Verificata in prima istanza: non ancora letta (↻ Aggiorna)")),
      e.rilettura === "fallita" ? h("p", { class: "du-err", id: "du-wz-non-riletta" }, "La conferma n. " + n + " è registrata; la pagina non si è riletta: ",
        btn("↻ Rileggi", rileggiDopoConfermaOra, { cls: "du-btn--sm", id: "du-wz-rileggi", perche: false })) : null);
  }
  if (e.tipo === "verifica") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "da_verificare" }, h("b", {}, "Esito della conferma da verificare."), h("span", {}, e.testo),
    btn("Verifica ora", () => verificaConfermaOra(e.chiave), { cls: "du-btn--sm", id: "du-wz-verifica", perche: false }));
  if (e.tipo === "non_registrata") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "non_registrata" },
    h("b", {}, "La conferma non è ancora registrata."), h("span", {}, "Puoi riprovare con la stessa chiave e lo stesso corpo: se la richiesta di prima arriva, riceverai il suo esito."),
    btn("Riprova la conferma", riprovaConferma, { cls: "du-btn--sm du-btn--pri", id: "du-wz-riprova", perche: false }));
  if (e.tipo === "controllo_finale") return h("div", { class: "du-esito du-esito--ko", id: "du-wz-esito", "data-esito": "controllo_finale" },
    h("b", {}, "La conferma non è registrata per un errore interno del controllo finale: non riprovare, segnalalo."), h("span", {}, e.testo || ""));
  const occupato = arr(e.dati && e.dati.motivi).includes("codice_occupato");
  const testi = {
    BASE_SUPERATA: occupato ? "Un'altra pagina ha scritto lo stesso codice nel frattempo: rileggi il riepilogo." : "La distinta è cambiata dopo la tua lettura: la bozza resta, fai il rebase e riconferma.",
    // N6 (U2 §6.3.6): che cosa è cambiato, niente scritto, la bozza intatta, il riepilogo nuovo con le differenze
    ANTEPRIMA_SUPERATA: e.motivo === "documenti_in_analisi_cambiati" ? "La conferma non è registrata: nel frattempo un documento nuovo è entrato in analisi. Niente è stato scritto e la bozza è intatta. Ecco il riepilogo nuovo: rivedi la presa d'atto." :
      e.motivo === "riprova" ? "Un'altra operazione stava scrivendo gli stessi dati: niente è stato registrato, riprova." :
        e.motivo === "ingressi_in_movimento" ? "Mentre confermavi, i dati della proposta stavano cambiando (un'analisi o un gesto sui file): niente è registrato e la bozza è intatta. Rileggi il riepilogo e riprova." :
        "La conferma non è registrata: nel frattempo la proposta è cambiata. Niente è stato scritto e la bozza è intatta. Ecco il riepilogo nuovo, con le differenze.",
    BOZZA_VERSIONE: e.motivo === "bozza_chiusa" ? "La bozza non è più aperta (confermata o abbandonata altrove): la rileggo." : e.motivo === "snapshot_diverso" ?
      "La bozza che stai confermando non è quella salvata: la rileggo, niente è registrato." : "La bozza salvata non è quella che stai confermando: la rileggo.",
    CORPO_NON_VALIDO: e.motivo === "presa_d_atto_obbligatoria" ? "Ci sono documenti in analisi: serve la presa d'atto. Niente è registrato." : "Il server non ha accettato il corpo della conferma: niente è registrato.",
    FUORI_RFQ: "La bozza non è di questo prodotto o di questa RFQ: niente è registrato.",
    COPIA_IN_CORSO: "Una copia sul NAS è in corso: riprova fra poco con la stessa chiave. Niente è registrato.",
    CHIAVE_RIUSATA: "Questo tentativo ha già un esito diverso: ho chiesto l'esito con la chiave e riletto bozza e storico; la prossima conferma userà una chiave nuova.",
    BLOCCHI: "Niente è registrato, la bozza resta: risolvi i blocchi.",
    CALCOLO_OCCUPATO: "Il server è occupato: la conferma non è partita e niente è cambiato. Riprova fra poco con la stessa chiave.",
    CALCOLO_SCADUTO: "Il calcolo del motore non è finito in tempo: niente è cambiato. Riprova con la stessa chiave.",
    ERRORE_CALCOLO: "Il calcolo del motore non è riuscito: la conferma non è registrata e niente è cambiato. Riprova con la stessa chiave, o rileggi il riepilogo.",
    CONGELATA: "La BOM è congelata: niente è registrato.",
    // il testo di MAT §5.2 («nessuna modifica»), con il numero della conferma di base
    NESSUNA_MODIFICA: "La bozza è uguale alla conferma " + (S.salvato && S.salvato.versione_bom_base && S.salvato.versione_bom_base.numero ? "n. " + S.salvato.versione_bom_base.numero : "di base") +
      ": non c'è niente da confermare, e niente è registrato.",
    PONTE_TENTATIVO_IN_SOSPESO: "La conferma di prima non ha un esito certo e questo corpo è diverso: niente è partito. Verifica il tentativo, o abbandonalo dalla striscia in alto, poi conferma di nuovo.",
  };
  const blocchi = arr(e.dati && e.dati.blocchi);
  // i blocchi con i loro rimedi (MAT §5.2, «rifiutata per blocchi»)
  return h("div", { class: "du-esito du-esito--ko", id: "du-wz-esito", "data-esito": e.tipo, "data-codice": e.codice || "" }, h("b", {}, (testi[e.codice] || e.testo || "La conferma non è riuscita.") + (e.codice ? " (" + e.codice + ")" : "")),
    e.codice === "CHIAVE_RIUSATA" && e.verifica ? h("p", { class: "du-nota" }, "Esito di quella chiave: " + e.verifica + ".") : null,
    arr(e.diff).length ? h("details", { class: "du-ripiega", id: "du-wz-n6-differenze", open: true }, h("summary", {}, "Le differenze (" + e.diff.length + ")"), h("ul", { class: "du-elenco-p" }, e.diff.map((t) => h("li", {}, t)))) : null,
    blocchi.length ? h("ul", { class: "du-elenco-p", id: "du-wz-esito-blocchi" }, blocchi.map((b) => h("li", { "data-codice": b.codice, "data-motivo": b.motivo || "" },
      b.codice + (b.codice === "R5_NON_APPLICABILE" ? " (una parte che la conferma non sa ancora applicare" + (b.motivo ? ": " + b.motivo : "") + ")" : b.motivo ? " (" + b.motivo + ")" : "") + ": " + (b.testo || ""),
      arr(b.rimedi).length ? h("span", { class: "du-muto" }, " Rimedi: " + arr(b.rimedi).join("; ")) : null,
      // il gate della sorgente ricontrollato da R5 sotto il lucchetto (direttiva 39 §1): il rimedio è nella Sorgente
      b.codice === "STEP_SORGENTE_NON_CONFERMATA" ? h("span", { class: "du-azioni" }, btn("Vai alla Sorgente", () => {
        chiudiWizard();
        vaiPasso("sorgente");
      }, { cls: "du-btn--sm", id: "du-wz-esito-vai-sorgente", perche: false })) : null,
      b.chiave_bozza ? h("span", { class: "du-azioni" }, btn("Mostra nel grafo", () => mostraNelGrafo(b.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false })) : null))) : null,
    e.codice === "COPIA_IN_CORSO" ? btn("Riprova", riprovaConferma, { cls: "du-btn--sm du-btn--pri", id: "du-wz-riprova-copia", perche: false }) : null,
    e.tipo === "occupato" ? btn("Riprova", riprovaConferma, { cls: "du-btn--sm du-btn--pri", id: "du-wz-riprova-occupato", perche: false }) : null,
    e.tipo === "occupato" && e.codice === "ERRORE_CALCOLO" ? btn("Rileggi il riepilogo", rileggiRiepilogo, { cls: "du-btn--sm", id: "du-wz-rileggi-calcolo", perche: false }) : null,
    e.codice === "BLOCCHI" ? btn("Rileggi il riepilogo", rileggiRiepilogo, { cls: "du-btn--sm", id: "du-wz-rileggi-blocchi", perche: false }) : null,
    e.codice === "BASE_SUPERATA" && !occupato ? btn("Fai il rebase…", () => {
      chiudiWizard();
      apriRebase();
    }, { cls: "du-btn--sm du-btn--pri", id: "du-wz-rebase", no: S.bozza && S.bozza.base && S.bozza.base.stato === "superata" ? "" : "R1 non dice ancora la base superata: «Rileggi il riepilogo»", perche: false }) : null,
    e.codice === "ANTEPRIMA_SUPERATA" || e.codice === "BASE_SUPERATA" ? btn("Rileggi il riepilogo", rileggiRiepilogo, { cls: "du-btn--sm", perche: false }) : null);
}
async function verificaConfermaOra(chiave) {
  try {
    const j = await P.esitoIntento(chiave);
    const w = S.wizard;
    if (!w) return;
    if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
      w.esito = { tipo: "ok", dati: j.esito || j, verificato: true };
      renderWizard();
      await dopoConferma();
    } else if (j.stato === "non_registrato") {
      w.esito = { tipo: "non_registrata", chiave };
      renderWizard();
    } else {
      w.esito = { tipo: "verifica", chiave, testo: "Una conferma per questa RFQ è ancora in corso: verifico fra poco." };
      renderWizard();
    }
  } catch (e) {
    avvisa("errore", "Verifica non riuscita: " + testoErrore(e) + ". La chiave resta in sospeso.");
  }
}
async function riprovaConferma() {
  const w = S.wizard;
  if (!w || !w.corpo || !w.firma) return;
  w.stato = "conferma";
  w.esito = { tipo: "in_corso" };
  renderWizard();
  const r = await P.confermaBom(w.firma, w.corpo, { presa: arr(w.presa) });
  await esitoConferma(r);
}
// dopoConferma: dopo una conferma registrata (200, o R6 «registrato») si rileggono il modello, R1 e R7 (MAT WZ-09). Le
// letture prendono i loro errori da sé: qui si guarda se sono riuscite, e se no lo stato è «confermata, pagina non
// riletta» (MAT §5.2; §1.3 da_rileggere), con «↻ Rileggi», mai «non riuscita».
async function dopoConferma(dati) {
  const w = S.wizard;
  const n = (dati && dati.numero_conferma) || (w && w.esito && w.esito.dati && w.esito.dati.numero_conferma) || "";
  const codice = (w && w.codice) || codiceProdotto();
  const riletta = await rileggiDopoUnGesto(["bozza", "storico"], "La conferma n. " + n + " di " + codice + " è registrata.");
  if (w && w.esito) w.esito.rilettura = riletta ? "ok" : "fallita";
  // l'ultima mossa è la conferma, non più la sorgente confermata di prima; l'esito di S8 di prima non resta in vista
  // (collaudo UI46, B3)
  if (w && w.prodotto) delete SG.esitiS8[w.prodotto];
  delete SG.esitiS8[S.prodotto];
  registraMossa("BOM di " + codice + " confermata (conferma n. " + n + ").");
  avvisa("ok", riletta ? "BOM confermata (conferma n. " + n + " di " + codice + ")." : "La conferma n. " + n + " di " + codice + " è registrata; la pagina non si è riletta: ↻ Rileggi.");
  if (S.wizard) renderWizard();
  render();
}
async function rileggiDopoConfermaOra() {
  await aggiorna();
  const w = S.wizard;
  if (w && w.esito && w.esito.tipo === "ok" && !letturaNonRiuscita()) w.esito.rilettura = "ok";
  if (S.wizard) renderWizard();
}

// ---------------------------------------------------------------- il wizard aperto e gli ingressi che cambiano (U2 §6.3)

// RV3-25 [U] (D31 §2): se durante la revisione cambiano la BOM o i dati che ne determinano la proposta, chi ha il wizard
// aperto riceve una notifica che dice che cosa è cambiato, senza confondere la proposta con la BOM registrata; il
// controllo finale del server resta (R5, strategia A). Il meccanismo è [T] (U2 §6.3.2): la lettura leggera di R1
// ?ingressi=1 ogni 10 s (5 s con documenti in analisi), ferma con la scheda nascosta; poi, solo quando la proposta cambia,
// un R4 nuovo con il motore, al più ogni 30 s e rinviato finché ci sono documenti in analisi (al più ogni 2 minuti).
// I testi N1-N6 sono quelli di U2 §6.3.6.
let timerIngressi = null;
function programmaIngressi(ms) {
  clearTimeout(timerIngressi);
  const w = S.wizard;
  if (!w) return;
  const pausa = ms !== undefined ? ms : arr(w.mostrati).length ? 5000 : 10000;
  timerIngressi = setTimeout(leggiIngressi, pausa);
}
document.addEventListener("visibilitychange", () => {
  if (!document.hidden && S.wizard) programmaIngressi(0);
});
function fermaIngressi() {
  clearTimeout(timerIngressi);
  timerIngressi = null;
}
// partiIngressiR4: le parti degli ingressi di R4 (bom, bozza, proposta, analisi). Il server le dà nella forma di R1
// ?ingressi=1, {prodotto_id, algoritmo, impronta_ingressi, parti} (blocco «R4 completo» della persistenza); le notifiche
// confrontano le parti
function partiIngressiR4(x) {
  if (!ogg(x)) return null;
  return ogg(x.parti) ? x.parti : x;
}
async function leggiIngressi() {
  const w = S.wizard;
  if (!w) return;
  if (document.hidden) return; // riprende con visibilitychange
  if (!w.ingressi || w.stato === "conferma" || w.stato === "lettura" || (w.esito && ["ok", "verifica"].includes(w.esito.tipo))) return programmaIngressi();
  let j;
  try {
    j = await P.leggiBozza(w.prodotto, { ingressi: true });
  } catch (e) {
    return programmaIngressi();
  }
  if (S.wizard !== w) return;
  // UI46 [U] (messaggio 48): la rilettura ogni 5-10 s ridisegnava il wizard anche senza niente di nuovo, e lo riportava
  // in cima mentre lo si scorreva; ora ridisegna solo se cambia quello che il wizard mostra (firmaVistaWizard)
  const vistaPrima = firmaVistaWizard(w);
  const pv = w.ingressi;
  const pn = j.parti;
  const inAn = arr(pn.analisi && pn.analisi.in_analisi);
  // N1: un documento nuovo in analisi; la presa d'atto si rifà con l'elenco nuovo (mai spuntata da sola)
  const nuovi = inAn.filter((x) => !arr(w.mostrati).some((y) => y.allegato_id === x.allegato_id));
  if (nuovi.length) {
    w.mostrati = arr(w.mostrati).concat(nuovi);
    w.presaAnalisi = false;
    w.n1 = nuovi.map((x) => x.nome_file || nomeFile(x.allegato_id)).join(", ");
  }
  const finiti = arr(w.mostrati).filter((x) => !inAn.some((y) => y.allegato_id === x.allegato_id));
  if (finiti.length) {
    w.analizzati = arr(w.analizzati).concat(finiti);
    w.mostrati = arr(w.mostrati).filter((x) => inAn.some((y) => y.allegato_id === x.allegato_id));
    if (!w.mostrati.length) w.n1 = "";
  }
  // N4: la bozza salvata da un'altra sessione (non dai salvataggi di questa pagina)
  if (pn.bozza && pv.bozza && pn.bozza.impronta !== pv.bozza.impronta && typeof pn.bozza.versione === "number" && S.bozza && pn.bozza.versione !== S.bozza.versione) {
    w.n4 = { utente: chiHaSalvato(pn.bozza.aggiornata_da), ora: ora(pn.bozza.aggiornata_il), versione: pn.bozza.versione };
  }
  // la BOM registrata (parte per RFQ): N3 solo con la base del prodotto superata; altrimenti come la proposta
  if (pn.bom && pv.bom && pn.bom.impronta !== pv.bom.impronta && !w.n3) {
    let base = null;
    try {
      const r1 = await P.leggiBozza(w.prodotto, {});
      base = r1 && r1.base;
    } catch (e) {
      base = null;
    }
    if (S.wizard !== w) return;
    if (base && base.stato === "superata") w.n3 = { che: arr(base.motivi).join(", ") || "una conferma nuova", quando: ora() };
    else w.daRiepilogare = true;
    w.ingressi = Object.assign({}, w.ingressi, { bom: pn.bom });
  }
  if (pn.proposta && pv.proposta && pn.proposta.impronta !== pv.proposta.impronta) w.daRiepilogare = true;
  if (w.daRiepilogare) await riepilogoNuovo(w, inAn.length > 0);
  if (S.wizard === w && firmaVistaWizard(w) !== vistaPrima) renderWizard();
  programmaIngressi();
}
// chiHaSalvato: il nome di chi ha salvato la bozza, per N4. R1 ?ingressi=1 dà aggiornata_da come utente_id (EM §9.1,
// U2 §6.3.3), senza il nome: la pagina lo cerca fra le persone che già conosce (le conferme della sorgente e delle BOM
// lette da S9 e da R7); chi guarda è «te, da un'altra scheda»; altrimenti «un altro utente», mai l'identificativo
// [T-39, difetto trovato dalla prova d1: N4 mostrava l'UUID].
function chiHaSalvato(da) {
  if (ogg(da)) return da.nome || chiHaSalvato(da.utente_id);
  const id = String(da || "").toLowerCase();
  if (!id) return "un altro utente";
  if (S.utente && id === String(S.utente).toLowerCase()) return "te, da un'altra scheda o sessione";
  const noti = [];
  for (const p of arr(S.smist && S.smist.prodotti)) {
    const fo = ogg(p.fonte) ? p.fonte : {};
    noti.push(fo.confermata_da, fo.autorizzata_da);
  }
  for (const c of arr(S.storico && S.storico.conferme)) noti.push({ utente_id: c.confermato_da, nome: c.confermato_da_nome });
  const x = noti.find((n) => ogg(n) && n.nome && String(n.utente_id || "").toLowerCase() === id);
  return x ? x.nome : "un altro utente";
}
// riepilogoNuovo: un R4 nuovo con il motore (U2 §6.3.4 punto 3): stessa firma e stessa verifica prevista = nessun
// conflitto, al più la riga quieta N5; diverse = N2, e «Conferma BOM» spento finché non si apre il riepilogo nuovo; il motore
// che non risponde = «non so ancora se la proposta è cambiata», «Conferma BOM» resta acceso (decide il controllo finale).
async function riepilogoNuovo(w, inAnalisi) {
  const adesso = Date.now();
  const ultimo = w.ultimoR4 || 0;
  if (adesso - ultimo < 30000) return;
  if (inAnalisi && adesso - ultimo < 120000) return;
  if (S.coda.length || S.invio) return;
  w.ultimoR4 = adesso;
  let r;
  try {
    r = await P.riepilogoBom(corpoBozza(), { motore: true });
  } catch (e) {
    if (S.wizard !== w) return;
    if (e && (e.codice === "CALCOLO_OCCUPATO" || e.codice === "CALCOLO_SCADUTO")) {
      w.n2incerto = true;
      w.daRiepilogare = false;
    }
    return;
  }
  if (S.wizard !== w) return;
  w.daRiepilogare = false;
  w.n2incerto = false;
  const vp = (x) => JSON.stringify((x && x.verifica_prevista) || null);
  if (r.firma_anteprima === w.riepilogo.firma_anteprima && vp(r) === vp(w.riepilogo)) {
    w.ingressi = partiIngressiR4(r.ingressi) || w.ingressi;
    if (arr(w.analizzati).length) w.n5 = arr(w.analizzati).map((x) => x.nome_file || nomeFile(x.allegato_id)).join(", ");
    w.analizzati = [];
    return;
  }
  w.nuovo = r;
  w.n2 = { causa: arr(w.analizzati).length ? "" : "un gesto sui file o una nuova analisi", diff: differenzeRiepiloghi(w.riepilogo, r) };
  w.analizzati = [];
}
// differenzeRiepiloghi: le differenze fra due riepiloghi, per chiave stabile (chiave_bozza; padre e figlio per i legami;
// codice per i blocchi): solo presentazione, la decisione la prende il server (U2 §6.3.4 punto 3). Il testo nomina i pezzi
// con codice e descrizione, mai con la chiave del nodo (UI46 [U], messaggio 48)
function differenzeRiepiloghi(a, b) {
  const out = [];
  const nome = nomeNodoWizard;
  const nomeArco = (z) => ((z.padre && (z.padre.codice || nome(z.padre.chiave_bozza, b))) || "?") + " → " + ((z.figlio && (z.figlio.codice || nome(z.figlio.chiave_bozza, b))) || "?");
  const idA = new Map(arr(a && a.identita).map((x) => [x.chiave_bozza, x]));
  for (const x of arr(b && b.identita)) {
    const y = idA.get(x.chiave_bozza);
    const c = (z) => JSON.stringify([z && z.codice, z && z.rev, z && z.tipo, z && z.esito_identita, z && z.decisa && z.decisa.codice]);
    if (!y) out.push(nome(x.chiave_bozza, b) + ": nuovo nel riepilogo");
    else if (c(x) !== c(y)) out.push(nome(x.chiave_bozza, a) + ": identità, rev o tipo cambiati");
  }
  for (const k of idA.keys()) if (!arr(b && b.identita).some((x) => x.chiave_bozza === k)) out.push(nome(k, a) + ": non più nel riepilogo");
  const ka = (z) => (z.padre && (z.padre.chiave_bozza || z.padre.codice)) + "→" + (z.figlio && (z.figlio.chiave_bozza || z.figlio.codice));
  const archiA = new Map(arr(a && a.archi).map((x) => [ka(x), x]));
  for (const x of arr(b && b.archi)) {
    const y = archiA.get(ka(x));
    if (!y) out.push("legame " + nomeArco(x) + ": nuovo");
    else if (String(y.qta_voluta) !== String(x.qta_voluta) || String(y.qta_step) !== String(x.qta_step)) out.push("legame " + nomeArco(x) + ": quantità cambiata");
  }
  const bA = new Set(arr(a && a.blocchi).map((x) => x.codice + (x.chiave_bozza || "")));
  for (const x of arr(b && b.blocchi)) if (!bA.has(x.codice + (x.chiave_bozza || ""))) out.push("blocco nuovo: " + x.codice);
  const avA = new Set(arr(a && a.avvisi).map((x) => x.codice + (x.chiave_bozza || "")));
  for (const x of arr(b && b.avvisi)) if (!avA.has(x.codice + (x.chiave_bozza || ""))) out.push("avviso nuovo: " + x.codice);
  if (JSON.stringify((a && a.verifica_prevista) || null) !== JSON.stringify((b && b.verifica_prevista) || null)) out.push("verifica prevista cambiata");
  return out;
}
// notificheWizard: N1-N5 (U2 §6.3.6) e la presa d'atto dei documenti in analisi (§6.3.7). N2 non dice mai che la BOM
// confermata è cambiata; N3 non parla della proposta; N1 non presenta mai come considerato un documento in analisi.
function notificheWizard(w) {
  const out = [];
  const prod = w.codice || codiceProdotto();
  if (w.n4) out.push(h("div", { class: "du-striscia du-striscia--ver", id: "du-wz-n4", role: "status" },
    h("span", {}, "La bozza di " + prod + " è stata salvata da " + w.n4.utente + " alle " + w.n4.ora + " (versione " + w.n4.versione + "): il riepilogo che stai guardando non è più quello della bozza salvata. Rileggi la bozza."),
    btn("Rileggi la bozza", async () => {
      w.n4 = null;
      await rileggiTenendoDaParte();
      if (S.wizard === w) await rileggiRiepilogo();
    }, { cls: "du-btn--sm du-btn--pri", id: "du-wz-n4-rileggi", perche: false })));
  if (w.n3) out.push(h("div", { class: "du-striscia du-striscia--ko", id: "du-wz-n3", role: "status" },
    h("span", {}, "Attenzione: la BOM registrata di " + prod + " è cambiata (" + w.n3.che + ", " + w.n3.quando + "). Il riepilogo non è più confermabile. La tua bozza è conservata: rileggi il riepilogo; se la base è superata, serve il rebase."),
    btn("Rileggi il riepilogo", async () => {
      w.n3 = null;
      await caricaBozza();
      await rileggiRiepilogo();
    }, { cls: "du-btn--sm", id: "du-wz-n3-rileggi", perche: false })));
  if (w.n2) out.push(h("div", { class: "du-striscia du-striscia--ver", id: "du-wz-n2", role: "status" },
    h("span", {}, w.n2.causa ? "Attenzione: il riepilogo non è più aggiornato. La proposta del motore è cambiata (" + w.n2.causa + "). Le tue modifiche sono conservate: rivedi le differenze prima di confermare." :
      "Attenzione: il riepilogo non è più aggiornato. Un nuovo documento ha modificato la proposta del motore. Le tue modifiche sono conservate: rivedi le differenze prima di confermare."),
    w.n2.diff.length ? h("details", { class: "du-ripiega", id: "du-wz-n2-differenze" }, h("summary", {}, "Le differenze (" + w.n2.diff.length + ")"), h("ul", { class: "du-elenco-p" }, w.n2.diff.map((t) => h("li", {}, t)))) : null,
    btn("Apri il riepilogo nuovo", () => {
      w.riepilogo = w.nuovo;
      w.ingressi = partiIngressiR4(w.nuovo.ingressi) || w.ingressi;
      w.inAnalisi = arr(w.nuovo.file_in_analisi);
      w.mostrati = w.inAnalisi.slice();
      w.presaAnalisi = false;
      w.versione = S.bozza.versione;
      w.nuovo = null;
      w.n2 = null;
      renderWizard();
    }, { cls: "du-btn--sm du-btn--pri", id: "du-wz-n2-apri", perche: false })));
  if (w.n2incerto && !w.n2) out.push(h("div", { class: "du-striscia du-striscia--info", id: "du-wz-n2-incerto", role: "status" },
    "Un documento è stato analizzato: non so ancora se la proposta è cambiata. Rileggi il riepilogo prima di confermare.",
    btn("Rileggi il riepilogo", rileggiRiepilogo, { cls: "du-btn--sm", perche: false })));
  if (w.n1) out.push(h("div", { class: "du-striscia du-striscia--info", id: "du-wz-n1", role: "status" },
    "È arrivato un documento nuovo (" + w.n1 + "): è in analisi e il riepilogo non lo considera ancora. Puoi aspettare l'analisi, oppure confermare dichiarando che non lo consideri."));
  if (w.n5 && !w.n2) out.push(h("p", { class: "du-nota", id: "du-wz-n5" }, "Il documento " + w.n5 + " è stato analizzato: la proposta non cambia."));
  const m = arr(w.mostrati);
  if (m.length) {
    const nomi = m.map((x) => x.nome_file || nomeFile(x.allegato_id)).join(", ");
    out.push(h("div", { class: "du-esito du-esito--ver", id: "du-wz-in-analisi" }, h("b", {}, m.length + (m.length === 1 ? " documento è ancora in analisi" : " documenti sono ancora in analisi") + ": questo riepilogo non li considera."),
      h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "du-wz-presa-analisi", checked: !!w.presaAnalisi, onchange: (e) => {
        w.presaAnalisi = e.target.checked;
        renderWizard();
      } }), "Confermo senza i documenti ancora in analisi: " + nomi)));
  }
  return out;
}

// ---------------------------------------------------------------- tipo e categoria: conferma_categoria (U2 §6.4)

// RV3-34 A [U] estesa all'editor del nodo (D31 §3): la conferma del tipo e della categoria del nodo, minuteria compresa,
// dall'editor del singolo nodo e dal wizard, in modo cumulativo; un solo modello di decisione, due punti d'ingresso
// ([UR]). Tipo e categoria restano separati; la classe della grammatica è una proposta e da sola non esenta dal 2D; l'unica
// esenzione è la minuteria confermata da una persona, mai sul finito; anche il commerciale chiede il 2D (R103 C [U]). La
// forma è quella del diciassettesimo tipo del formato 3 (U2 §6.4.2): soggetto, categoria del vocabolario chiuso, esito,
// origine_categoria, classe_proposta e importazione con la classe della grammatica, ingresso (editor | wizard), motivo.
const CATEGORIE = [["fabbricato", "fabbricato"], ["commerciale", "commerciale"], ["minuteria", "minuteria"]];
// classeProposta: la classe proposta dal registro per la voce del nodo (tipo_proposto.classe), o null
function classeProposta(v) {
  const voce = v && v.voce;
  const c = voce && voce.tipo_proposto && voce.tipo_proposto.classe;
  return ogg(c) && typeof c.categoria === "string" ? c : null;
}
function decisioneCategoria(k) {
  return arr(S.lavoro && S.lavoro.decisioni).find((d) => d.tipo === "conferma_categoria" && ogg(d.soggetto) && d.soggetto.chiave_bozza === k) || null;
}
function testoClasse(c) {
  return c.categoria + " proposta" + (c.fonte ? ", " + (c.fonte === "sdf-norma" ? "normativa" : c.fonte) : "") + (c.valore ? " " + c.valore : "");
}
// decisioneCat: la decisione conferma_categoria, nella forma chiusa del formato 3
function decisioneCat(v, o) {
  const c = classeProposta(v);
  const dec = { id: nuovoIdDecisione("cc"), tipo: "conferma_categoria", soggetto: { chiave_bozza: v.k }, categoria: o.categoria, esito: o.esito, origine_categoria: o.origine, ingresso: o.ingresso };
  if (o.origine === "classe_grammatica" && c) {
    dec.classe_proposta = clona(c);
    if (S.bozza && S.bozza.importazione && S.bozza.importazione.impronta) dec.importazione = S.bozza.importazione.impronta;
  }
  if (o.motivo) dec.motivo = o.motivo;
  return dec;
}
// gestiCategoria: nell'editor del nodo, sotto il tipo: la classe proposta, «Conferma la categoria…», «Non è minuteria…»,
// «Conferma il tipo proposto» (U2 §6.4.2, «Un solo modello, due punti d'ingresso»)
function gestiCategoria(g, v) {
  if (v.noninc || !v.nodo) return null;
  const s = sezione("du-sez-categoria", "Tipo e categoria", true);
  const radice = v.k === g.radice || v.tipo === "finito";
  const no = perchéNoModifica();
  const c = classeProposta(v);
  const d = decisioneCategoria(v.k);
  const tp = v.voce && v.voce.tipo_proposto && v.voce.tipo_proposto.valore;
  s.lastChild.append(h("dl", { class: "du-dl" }, riga("Tipo", (TIPI[v.tipo] || v.tipo) + (tipoProposto(v) ? " (proposto dal file)" : "") + (v.nodo.tipo_confermato ? " · tipo confermato" + (autoreGestoNodo(v.k) ? " da " + autoreGestoNodo(v.k) : "") : "")),
    riga("Classe proposta", c ? testoClasse(c) + " (una proposta: da sola non esenta dal 2D)" : "nessuna"),
    riga("Categoria", d ? (d.esito === "rifiuto" ? "non è " + d.categoria : d.categoria + " confermata") + (autoreDi(d.id) ? " da " + autoreDi(d.id) : " (in bozza)") : "non confermata"),
    d && d.esito === "conferma" && d.categoria === "minuteria" ? riga("Effetto", h("span", { id: "du-cat-effetto" }, "in bozza: il 2D non sarà richiesto dopo la conferma")) : null));
  if (radice) {
    s.lastChild.append(h("p", { class: "du-perche" }, "Il prodotto della RFQ non ha una categoria: chiede sempre il 2D."));
    return s;
  }
  const az = h("div", { class: "du-azioni" });
  // aggiungi salta il pulsante assente (null): Element.append lo scriveva come testo («Conferma il tipo proposto null», D13)
  aggiungi(az, [btn("Conferma la categoria…", () => apriCategoria(v, "conferma", "editor"), { cls: "du-btn--sm", id: "du-conferma-categoria", no }),
    btn("Non è minuteria…", () => apriCategoria(v, "rifiuto", "editor"), { cls: "du-btn--sm", id: "du-non-minuteria", no: no || (d && d.esito === "rifiuto" ? "è già detto «non è minuteria»" : "") }),
    btn("Conferma il tipo proposto", () => confermaTipoProposto(v), { cls: "du-btn--sm", id: "du-conferma-tipo", no: no || (!tp ? "il registro non propone un tipo" : tp !== v.tipo ? "il tipo di adesso non è quello proposto (" + (TIPI[tp] || tp) + "): «◆ Cambia tipo…»" : v.nodo.tipo_confermato ? "il tipo proposto è già confermato" : "") }),
    d ? btn("Togli la decisione sulla categoria", () => fai("tolta la decisione sulla categoria di " + (v.cod || v.k), [{ azione: "decisione_tolta", chiave_bozza: chiaveEvento(d), prima: clona(d), dopo: null, origine: "utente" }]),
      { cls: "du-btn--sm du-btn--ghost", id: "du-togli-categoria", no }) : null]);
  s.lastChild.append(az);
  return s;
}
// autoreGestoNodo: chi ha salvato l'ultimo nodo_modificato del nodo (dal giornale della bozza), o ""
function autoreGestoNodo(k) {
  const e = arr(S.bozza && S.bozza.eventi).filter((x) => x.azione === "nodo_modificato" && x.chiave === k).pop();
  return e ? chiE(e.autore) + (e.quando ? " il " + dataOra(e.quando) : "") : "";
}
function apriCategoria(v, esito, ingresso) {
  const c = classeProposta(v);
  const sel = h("select", { id: "f-categoria" }, CATEGORIE.map(([k, t]) => h("option", { value: k, selected: (esito === "rifiuto" ? k === "minuteria" : c ? k === c.categoria : k === "minuteria") ? true : null }, t)));
  if (esito === "rifiuto") sel.disabled = true;
  const origini = h("select", { id: "f-origine" }, c ? h("option", { value: "classe_grammatica" }, "accetto la classe proposta (" + testoClasse(c) + ")") : null,
    h("option", { value: "normativa" }, "cito la normativa"), h("option", { value: "manuale" }, "scelta mia, senza proposta"));
  const titolo = esito === "rifiuto" ? "Non è minuteria: " + (v.cod || v.k) : "Conferma la categoria di " + (v.cod || v.k);
  apriFinestra(titolo, h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, esito === "rifiuto" ? "Dice che il pezzo non è minuteria: il 2D resta richiesto." :
      "Tipo e categoria sono separati: la categoria non cambia il tipo. Solo la minuteria confermata da una persona non chiede il 2D, e solo dopo la conferma della BOM; fabbricato e commerciale si registrano, ma il 2D resta richiesto."),
    campo("Categoria", sel), esito === "rifiuto" ? null : campo("Da dove viene", origini)),
  [{ testo: "Annulla", chiudi: true }, { testo: esito === "rifiuto" ? "Non è minuteria" : "Conferma la categoria", pri: true, id: "f-ok", fai: () => {
    const f = $("#du-finestra");
    const dec = decisioneCat(v, { categoria: esito === "rifiuto" ? "minuteria" : valore(f, "f-categoria"), esito, origine: esito === "rifiuto" ? (c ? "classe_grammatica" : "manuale") : valore(f, "f-origine"), ingresso });
    const prima = decisioneCategoria(v.k);
    const azioni = prima ? [{ azione: "decisione_tolta", chiave_bozza: chiaveEvento(prima), prima: clona(prima), dopo: null, origine: "utente" }] : [];
    azioni.push({ azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" });
    fai((esito === "rifiuto" ? "non è minuteria: " : "categoria confermata: ") + (v.cod || v.k), azioni);
  } }]);
}
// confermaTipoProposto: «Conferma il tipo proposto»: un nodo_modificato con il tipo uguale al proposto e il segno
// tipo_confermato (U2 §6.4.2, «La conferma del tipo»). Nessun effetto sul 2D.
function confermaTipoProposto(v) {
  fai("tipo proposto confermato: " + (v.cod || v.k), [{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { tipo: v.tipo, tipo_confermato: v.nodo.tipo_confermato ? true : null }, dopo: { tipo: v.tipo, tipo_confermato: true }, origine: "utente" }]);
}
// tabellaCategorie: il gruppo «Categorie da confermare» del wizard (U2 §6.4.2; MAT WZ-02): i nodi con una classe proposta e
// nessuna decisione, con caselle non spuntate e «Seleziona tutte le proposte» come aiuto esplicito; quelli già decisi
// (anche dall'editor) con chi e quando, senza una seconda accettazione. «Conferma le categorie (N)» = un salvataggio R2
// con N decisioni (ingresso «wizard»), poi il riepilogo si rilegge.
function categorieDaConfermare() {
  // dopo la conferma (la bozza non c'è più) il wizard mostra l'esito: niente categorie da confermare
  if (!S.lavoro || !bozzaLavorabile()) return [];
  const g = grafoBozza();
  return Object.values(g.nodi).filter((v) => v.nodo && !v.noninc && v.k !== g.radice && v.tipo !== "finito" && classeProposta(v));
}
function tabellaCategorie(w) {
  const vv = categorieDaConfermare();
  const r = w.riepilogo || {};
  const effetti = new Map(arr(r.effetti_categoria).map((x) => [x.chiave_bozza, x]));
  if (!w.catScelte) w.catScelte = new Set();
  const libere = vv.filter((v) => !decisioneCategoria(v.k));
  const no = perchéNoModifica();
  const righe = vv.map((v) => {
    const c = classeProposta(v);
    const d = decisioneCategoria(v.k);
    const e = effetti.get(v.k);
    return h("tr", { "data-k": v.k }, h("td", {}, d ? null : h("input", { type: "checkbox", class: "du-wz-cat", "data-k": v.k, "aria-label": "Conferma la categoria di " + (v.cod || v.k), checked: w.catScelte.has(v.k), onchange: (ev) => {
      if (ev.target.checked) w.catScelte.add(v.k);
      else w.catScelte.delete(v.k);
      renderWizard();
    } })),
    h("td", {}, h("span", { class: "du-mono" }, v.cod || "(senza codice)"), " ", v.desc || ""), h("td", {}, testoClasse(c)),
    h("td", {}, d ? (d.esito === "rifiuto" ? "non è " + d.categoria : d.categoria + " confermata") + (autoreDi(d.id) ? " da " + autoreDi(d.id) : " (in bozza)") + (d.ingresso ? " · dall'" + (d.ingresso === "editor" ? "editor" : "wizard") : "") : "da confermare"),
    h("td", {}, e ? (e.prima || "?") + " → " + (e.dopo || "?") + " dopo la conferma" : d && d.esito === "conferma" && d.categoria === "minuteria" ? "in bozza: il 2D non sarà richiesto dopo la conferma" : "2D richiesto"));
  });
  return h("div", { style: "display:grid;gap:8px", id: "du-wz-categorie" },
    h("p", { class: "du-nota" }, "La classe della grammatica è una proposta: da sola non esenta dal 2D. Solo la minuteria confermata da una persona non lo chiede, e solo dopo la conferma della BOM."),
    h("div", { class: "du-azioni" }, btn("Seleziona tutte le proposte", () => {
      for (const v of libere) w.catScelte.add(v.k);
      renderWizard();
    }, { cls: "du-btn--sm", id: "du-wz-cat-tutte", no: libere.length ? "" : "nessuna categoria da confermare", perche: false }),
    btn("Conferma le categorie (" + [...w.catScelte].filter((k) => libere.some((v) => v.k === k)).length + ")", () => confermaCategorieWizard(w, libere), { cls: "du-btn--sm du-btn--pri", id: "du-wz-cat-conferma",
      no: no || ([...w.catScelte].some((k) => libere.some((v) => v.k === k)) ? "" : "spunta almeno una proposta"), perche: false })),
    h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab" }, h("thead", {}, h("tr", {}, h("th", {}, ""), h("th", {}, "Nodo"), h("th", {}, "Classe proposta"), h("th", {}, "Decisione"), h("th", {}, "Effetto"))),
      h("tbody", {}, righe.length ? righe : h("tr", {}, h("td", { colspan: "5" }, "Nessun nodo con una classe proposta."))))));
}
async function confermaCategorieWizard(w, libere) {
  const scelte = libere.filter((v) => w.catScelte.has(v.k));
  if (!scelte.length) return;
  const azioni = scelte.map((v) => {
    const c = classeProposta(v);
    const dec = decisioneCat(v, { categoria: c.categoria, esito: "conferma", origine: "classe_grammatica", ingresso: "wizard" });
    return { azione: "decisione_aggiunta", chiave_bozza: chiaveEvento(dec), prima: null, dopo: dec, origine: "utente" };
  });
  if (!fai("categorie confermate: " + scelte.length, azioni)) return;
  w.catScelte = new Set();
  const ok = await salvaOra();
  if (!ok) return;
  await rileggiRiepilogo();
}

// ---------------------------------------------------------------- avvio

function installaAscoltatori() {
  document.addEventListener("pointermove", vpMove);
  document.addEventListener("pointerup", (e) => vpUp(e, false));
  document.addEventListener("pointercancel", (e) => vpUp(e, true));
  document.addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    if (!$("#du-finestra").hidden) chiudiFinestra();
    else if (S.wizard) chiudiWizard();
  });
  // le scorciatoie del tab Sorgente (mockup v5, §6): «/» la ricerca, Ctrl+Z «Annulla la mossa», F6 / Maiusc+F6 fra
  // ricerca, cartelle, elenco, dettaglio e barra di stato. Mai dentro un campo, mai con un dialogo aperto
  document.addEventListener("keydown", (e) => {
    if (S.passo !== "sorgente" || !$("#du-sg") || !$("#du-finestra").hidden || S.wizard) return;
    const campo = /^(INPUT|TEXTAREA|SELECT)$/.test((document.activeElement && document.activeElement.tagName) || "");
    if (e.key === "/" && !campo) {
      e.preventDefault();
      const c = $("#du-sg-cerca");
      if (c) c.focus();
    } else if ((e.key === "z" || e.key === "Z") && (e.ctrlKey || e.metaKey) && !e.shiftKey && !campo && mossaAnnullabile()) {
      e.preventDefault();
      annullaMossa();
    } else if (e.key === "F6") {
      const zone = ["#du-sg-cerca", "#du-sg-albero [role=treeitem][tabindex='0']", "#du-sg-elenco [role=row][tabindex='0']", "#du-sg-dettaglio h3", "#du-barra button"];
      const at = zone.findIndex((z) => {
        const el = $(z);
        return el && (el === document.activeElement || (el.closest && document.activeElement && el.parentElement && el.parentElement.contains(document.activeElement)));
      });
      const prossima = zone[(at + (e.shiftKey ? zone.length - 1 : 1) + zone.length) % zone.length];
      const el = $(prossima);
      if (el) {
        e.preventDefault();
        if (el.tagName === "H3") el.setAttribute("tabindex", "-1");
        el.focus();
      }
    }
  });
  // uscire dalla pagina con modifiche non salvate (in coda, in volo o tenute da parte, MAT §5.1) chiede conferma: le
  // modifiche stanno solo in memoria (decisioni dell'ondata A, §5 n. 2)
  window.addEventListener("beforeunload", (e) => {
    if (S.coda.length || S.invio || S.daParte) {
      e.preventDefault();
      e.returnValue = "";
    }
  });
  let timer = null;
  window.addEventListener("resize", () => {
    clearTimeout(timer);
    timer = setTimeout(() => {
      if (S.vista === "org") renderCorpo();
    }, 200);
  });
}
async function avvio() {
  const radice = $("#du");
  if (!radice) return;
  S.thread = radice.dataset.duThread || "";
  S.utente = radice.dataset.duUtente || "";
  S.sigla = radice.dataset.duSigla || "";
  try {
    P.configura({ thread: S.thread, utente: S.utente });
  } catch (e) {
    avvisa("errore", "La pagina non si configura: " + testoErrore(e));
    return;
  }
  leggiIndirizzo();
  installaAscoltatori();
  renderTestaAzioni();
  avviaGuida();
  await leggiTutto(true);
  await verificaSospesi();
}
avvio();
