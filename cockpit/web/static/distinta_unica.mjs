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
  cartella: "attivi",
  esitiCarica: null,
  anteprima: null, // S6
  anteprimaAllegato: "",
  anteprimaGruppo: "",
  erroreAnteprima: "",
  bozza: null, // R1
  erroreBozza: "",
  registri: {}, // impronta → contenuto del registro d'importazione
  storico: null, // R7
  erroreStorico: "",
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
  selezione: new Set(), // chiavi_bozza scelte per «Non includere»
  segnalatiAperto: false,
  compatte: false,
  camere: {},
  compressi: {},
  wizard: null,
  filtro: "tutti",
};
let L = null; // la tela in vista: {lay, chiave, vp, mondo, g}

// ---------------------------------------------------------------- avvisi, strisce

function avvisa(esito, testo) {
  const el = $("#du-avviso");
  if (!el) return;
  el.textContent = "";
  el.textContent = testo || "";
  el.setAttribute("data-esito", esito);
  el.hidden = !testo;
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

// modelloDa: i campi di /motore-a/dati che la pagina legge. Quelli che c'erano (contratto A1d §2.4): errore, testa, i
// prodotti valutati del thread (rif, componente_id, codice_richiesto, prodotto_verificato, motivi), il cliente, il
// congelamento. Le aggiunte di V1 (EM §9.9) con i nomi di EM e MAT dove li danno (bozza_del_prodotto con il gruppo
// corrente, impronta_contenuto per gruppo) e, dove EM non dà la forma, i nomi proposti nel resoconto della corsia:
// gruppi[], fonte, f8_stato, associazioni[]. È l'unica funzione da adeguare se la corsia del motore sceglie altri nomi.
function modelloDa(j) {
  const th = arr(j && j.esito && j.esito.thread)[0] || {};
  const prodotti = arr(th.prodotti_valutati)
    .filter((p) => p && typeof p.componente_id === "string" && RE_UUID.test(p.componente_id))
    .map((p) => ({ rif: p.rif || "", id: p.componente_id.toLowerCase(), codice: p.codice_richiesto || "", descrizione: p.descrizione || "",
      verificato: p.prodotto_verificato === true, motivi: arr(p.motivi) }));
  return {
    errore: typeof j.errore === "string" ? j.errore : "",
    testa: ogg(j.testa) ? j.testa : {},
    prodotti,
    rif: typeof j.prodotto === "string" ? j.prodotto : "",
    clienteId: typeof th.cliente_id === "string" ? th.cliente_id.toLowerCase() : "",
    congelata: !!(th.fascicolo && th.fascicolo.congelato),
    bozzaDelProdotto: ogg(j.bozza_del_prodotto) ? j.bozza_del_prodotto : null,
    gruppi: arr(j.gruppi),
    fonte: ogg(j.fonte) ? j.fonte : null,
    f8: typeof j.f8_stato === "string" ? j.f8_stato : "",
    associazioni: arr(j.associazioni),
    calcolato: j.calcolato || "",
  };
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
    S.sorgente = await P.leggiSorgente({ scartati: true });
    S.erroreSorgente = "";
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
    await caricaSorgente();
    if (S.passo === "sorgente") renderCorpo();
  }, 5000);
}
async function caricaBozza() {
  if (!S.prodotto) {
    impostaBozza(null);
    return;
  }
  try {
    impostaBozza(await P.leggiBozza(S.prodotto, { eventi: true, importazione: true }));
    S.erroreBozza = "";
  } catch (e) {
    impostaBozza(null);
    S.erroreBozza = testoErrore(e);
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
  await Promise.all([caricaSorgente(), caricaBozza(), caricaStorico()]);
  if (prima && !passoChiesto) S.passo = bozzaAperta() ? "revisione" : "sorgente";
  if (!S.passo) S.passo = "sorgente";
  render();
}
// aggiorna (MAT TS-03): rilegge il modello e, sul passo aperto, R1 o S1. Il guscio resta, la bozza non si perde: con
// modifiche non salvate R1 non si rilegge.
async function aggiorna() {
  avvisa("ok", "Rilettura in corso…");
  await caricaModello();
  const lavori = [caricaSorgente(), caricaStorico()];
  if (!S.coda.length && !S.invio) lavori.push(caricaBozza());
  await Promise.all(lavori);
  if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
  avvisa("ok", "Pagina riletta alle " + ora() + ".");
  render();
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
  S.coda = [];
  S.invio = null;
  S.rifai = [];
  S.riepilogo = null;
  S.riepilogoStato = "";
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
    if (e.azione === "annulla" || e.azione === "rifai" || annullati.has(Number(e.n))) continue;
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
    file_preassociato: "file preassociato", file_svincolato: "file svincolato", file_lasciato: "file lasciato senza associazione", rebase: "rebase" };
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
    case "file_preassociato":
      s.documenti_preassociati = s.documenti_preassociati.filter((x) => x.proposta_id !== d.proposta_id).concat([{ proposta_id: d.proposta_id, chiave_bozza: d.chiave_bozza }]);
      break;
    case "file_svincolato":
      s.documenti_preassociati = s.documenti_preassociati.filter((x) => x.proposta_id !== p.proposta_id);
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

// Una conferma in sospeso per il prodotto (R5 con l'esito incerto): finché c'è, la bozza non si modifica, salvo
// l'abbandono esplicito della chiave (MAT §5.2, «verificato: non registrata»; §5.5 n. 7).
function confermaInSospeso() {
  return P.sospesi().find((s) => s.nome === "confermaBom" && s.risorsa === "bozza:" + S.prodotto) || null;
}
// perchéNoModifica: perché un gesto sulla bozza non parte, o "". Con o.riprova una conferma in sospeso già verificata
// «non registrata» non ferma il wizard: è la strada di «Riprova la conferma», lo stesso corpo con la stessa chiave, anche
// dopo una ricarica della pagina (MAT §5.2, «verificato: non registrata»; §5.5 n. 4). Le modifiche restano ferme.
function perchéNoModifica(o) {
  if (!S.modello || S.erroreModello) return "il modello della pagina non si legge: niente tela, bozza né wizard (D-18)";
  if (!bozzaAperta()) return "nessuna bozza: la BOM di lavoro nasce dalla scelta dello STEP nella Sorgente";
  if (bozzaDaConvertire()) return "la bozza è nel formato di prima: per lavorarla va convertita";
  if (S.modello.congelata) return "la BOM è congelata: revisione e congelamento non sono in questa pagina";
  if (S.salvataggio.stato === "conflitto") return "la bozza è cambiata altrove: rileggi la bozza salvata";
  if (S.salvataggio.stato === "rifiutata") return "il server ha rifiutato l'ultimo salvataggio: rileggi la bozza salvata";
  const sosp = confermaInSospeso();
  if (sosp && !(o && o.riprova && sosp.verificato)) return sosp.verificato ? "una conferma è in sospeso (non ancora registrata): riprovala con lo stesso corpo, o abbandona il tentativo" : "una conferma è in sospeso: verificala, o abbandona il tentativo";
  if (S.wizard && S.wizard.stato === "conferma") return "conferma in corso";
  return "";
}
function puoModificare() {
  return !perchéNoModifica();
}

// fai: un gesto della bozza. Le azioni vanno in coda (debounce 1,5 s, MAT §3.4), si applicano subito allo snapshot che
// si vede e diventano un passo della pila. Niente arriva alla working: la scrive solo R5 (D23:254).
function fai(etichetta, azioni, o) {
  const no = perchéNoModifica();
  if (no) {
    avvisa("errore", "Il gesto non parte: " + no + ".");
    return false;
  }
  const voce = { etichetta, inviare: azioni, locali: azioni, numeri: null, rebase: o && o.rebase ? o.rebase : null };
  S.coda.push(voce);
  S.pila.push({ etichetta, voci: [voce], salvata: false });
  S.rifai = [];
  ricalcola();
  S.salvataggio = Object.assign({}, S.salvataggio, { stato: "modifiche" });
  programmaSalvataggio(voce.rebase ? 0 : 1500);
  render();
  return true;
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
    const inviare = numeri.map((n) => ({ azione: "annulla", chiave_bozza: null, prima: null, dopo: { n }, origine: "utente" }));
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
    const inviare = voce.numeri.slice().sort((a, b) => a - b).map((n) => ({ azione: "rifai", chiave_bozza: null, prima: null, dopo: { n }, origine: "utente" }));
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
  const r = await P.salvaBozza(S.invio.corpo);
  esitoSalvataggio(r);
}
function esitoSalvataggio(r) {
  const inv = S.invio;
  if (!inv) return;
  if (r.esito === "ok") {
    S.salvato = inv.snapshot;
    S.bozza.versione = r.dati && typeof r.dati.versione === "number" ? r.dati.versione : S.bozza.versione + 1;
    S.bozza.impronta = r.dati && r.dati.impronta;
    let n = S.totaleEventi;
    inv.voci.forEach((v) => {
      v.numeri = v.inviare.map(() => ++n);
    });
    S.totaleEventi = n;
    S.invio = null;
    S.tentativi = 0;
    S.salvataggio = { stato: S.coda.length ? "modifiche" : "salvata", testo: r.dati && r.dati.riproposto ? "riproposto" : "", versione: S.bozza.versione, ora: ora() };
    ricalcola();
    if (S.coda.length) programmaSalvataggio(300);
    programmaRiepilogo();
    renderBarra();
    return;
  }
  if (r.codice === "PONTE_SESSIONE") return;
  if (r.esito === "conflitto" || r.esito === "no") {
    S.coda = inv.voci.concat(S.coda);
    S.invio = null;
    S.salvataggio = { stato: r.esito === "conflitto" ? "conflitto" : "rifiutata", codice: r.codice, motivo: r.motivo, testo: r.testo, dati: r.dati, versione: S.bozza.versione, ora: S.salvataggio.ora };
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
      nuovo = true;
    }
  } catch (e) {
    S.riepilogoStato = "errore";
    S.riepilogoErrore = testoErrore(e);
  }
  renderBarra();
  // l'Elenco mostra gli stati delle preassociazioni che dà R4: si ridisegna con il riepilogo nuovo
  if (nuovo && S.passo === "revisione" && S.vista === "elenco") renderCorpo();
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
function fileDiProposta(pid) {
  return fileSorgente().find((f) => f.proposta_id === pid) || null;
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

const CW = 236, CH = 84, MARGINE = 24, RG = 16, IND = 58, SPX = 20, GXC = 26, BUS1 = 22, BUS2 = 22, SH = 30, KMIN = 0.25, KMAX = 2.2;
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
function grafoProposta() {
  const a = S.anteprima;
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
function impagina(g, ap, modo, perRiga) {
  const cw = CW;
  const SW = cw - 18;
  const comp = compressiCorr();
  const pos = {}, stubs = [], tratti = [], pills = [], etichette = [];
  const ext = { X: 0, Y: 0 };
  const cresci = (x2, y2) => {
    ext.X = Math.max(ext.X, x2);
    ext.Y = Math.max(ext.Y, y2);
  };
  const metti = (k, x, y) => {
    pos[k] = { x, y };
    cresci(x + cw, y + CH);
  };
  function colonna(k, x, y) {
    metti(k, x, y);
    let yy = y + CH;
    const ch = comp[k] ? [] : ap.kids[k] || [];
    const st = comp[k] ? [] : ap.rim[k] || [];
    if (!ch.length && !st.length) return yy;
    const sx = x + SPX;
    let ult = null;
    const seq = ch.map((a) => ({ a })).concat(st.map((r) => ({ r })));
    seq.forEach((it) => {
      if (it.a) {
        const cy = yy + RG;
        const fine = colonna(it.a.f, x + IND, cy);
        const mid = cy + CH / 2;
        tratti.push({ d: "M" + sx + " " + mid + " H" + (x + IND), a: it.a });
        pills.push({ a: it.a, x: sx + (IND - SPX) / 2, y: mid });
        ult = mid;
        yy = fine;
      } else {
        const cy = yy + RG;
        const mid = cy + SH / 2;
        stubs.push({ a: it.r, x: x + IND, y: cy, w: SW });
        cresci(x + IND + SW, cy + SH);
        tratti.push({ d: "M" + sx + " " + mid + " H" + (x + IND), a: it.r, secondo: true });
        pills.push({ a: it.r, x: sx + (IND - SPX) / 2, y: mid, secondo: true });
        ult = mid;
        yy = cy + SH;
      }
    });
    tratti.push({ d: "M" + sx + " " + (y + CH) + " V" + ult, neutro: true });
    return yy;
  }
  let yFondo = MARGINE;
  if (g.radice) {
    if (modo === "lista") {
      yFondo = colonna(g.radice, MARGINE, MARGINE);
    } else {
      const r = g.radice;
      const items = (comp[r] ? [] : ap.kids[r] || []).map((a) => ({ a })).concat((comp[r] ? [] : ap.rim[r] || []).map((a) => ({ a, stub: true })));
      const righe = [];
      for (let i = 0; i < items.length; i += perRiga) righe.push(items.slice(i, i + perRiga));
      const piu = righe.length > 1;
      const trunkX = MARGINE + 4;
      const x0 = piu ? MARGINE + 18 : MARGINE;
      let yRow = MARGINE + CH + BUS1 + BUS2;
      const info = [];
      righe.forEach((riga) => {
        let x = x0;
        let yMax = yRow;
        const drops = [];
        const yb = yRow - BUS2;
        riga.forEach((it) => {
          if (!it.stub) {
            const X1 = ext.X;
            ext.X = 0;
            const fine = colonna(it.a.f, x, yRow);
            const dx = x + cw / 2;
            drops.push(dx);
            tratti.push({ d: "M" + dx + " " + yb + " V" + yRow, a: it.a });
            pills.push({ a: it.a, x: dx, y: yb + BUS2 / 2 });
            const destra = Math.max(ext.X, x + cw);
            ext.X = Math.max(X1, destra);
            x = destra + GXC;
            yMax = Math.max(yMax, fine);
          } else {
            stubs.push({ a: it.a, x, y: yRow, w: SW });
            cresci(x + SW, yRow + SH);
            const dx = x + SW / 2;
            drops.push(dx);
            tratti.push({ d: "M" + dx + " " + yb + " V" + yRow, a: it.a, secondo: true });
            pills.push({ a: it.a, x: dx, y: yb + BUS2 / 2, secondo: true });
            x += SW + GXC;
            yMax = Math.max(yMax, yRow + SH);
          }
        });
        info.push({ yb, drops });
        yRow = yMax + 40 + BUS2;
      });
      const r1 = info[0];
      const cx = r1 && r1.drops.length ? (r1.drops[0] + r1.drops[r1.drops.length - 1]) / 2 : MARGINE + cw / 2;
      metti(r, Math.max(MARGINE, cx - cw / 2), MARGINE);
      const rcx = pos[r].x + cw / 2;
      const base = [];
      if (r1 && r1.drops.length) base.push({ d: "M" + rcx + " " + (MARGINE + CH) + " V" + r1.yb, neutro: true });
      info.forEach((ri, i) => {
        if (!ri.drops.length) return;
        let xa = Math.min.apply(null, ri.drops), xb = Math.max.apply(null, ri.drops);
        if (i === 0) {
          xa = Math.min(xa, rcx);
          xb = Math.max(xb, rcx);
        }
        if (piu) xa = Math.min(xa, trunkX);
        base.push({ d: "M" + xa + " " + ri.yb + " H" + xb, neutro: true });
      });
      if (piu) base.push({ d: "M" + trunkX + " " + info[0].yb + " V" + info[info.length - 1].yb, neutro: true });
      tratti.unshift.apply(tratti, base);
      yFondo = info.length ? yRow - 40 - BUS2 : MARGINE + CH;
    }
  }
  const fuori = [["Senza padre · da ricollocare", ap.altre], ["Non inclusi nella BOM (bozza)", g.nonInclusi]];
  let yE = Math.max(yFondo, ext.Y);
  for (const [titolo, elenco] of fuori) {
    if (!elenco.length) continue;
    yE += 40;
    etichette.push({ x: MARGINE, y: yE, t: titolo });
    let x = MARGINE;
    const yC = yE + 28;
    let yMax = yC;
    elenco.forEach((k) => {
      const X1 = ext.X;
      ext.X = 0;
      const fine = colonna(k, x, yC);
      const destra = Math.max(ext.X, x + cw);
      ext.X = Math.max(X1, destra);
      x = destra + GXC;
      yMax = Math.max(yMax, fine);
    });
    yE = Math.max(yE, yMax);
  }
  return { W: ext.X + MARGINE, H: Math.max(ext.Y, yE) + MARGINE, cw, pos, stubs, tratti, pills, etichette, kids: ap.kids, padrePrim: ap.padrePrim, modo };
}
function layout(g, larghezza) {
  const ap = alberoPrimario(g);
  const n1 = g.radice ? (ap.kids[g.radice] || []).length + (ap.rim[g.radice] || []).length : 0;
  if (larghezza < 560 || n1 < 2) return impagina(g, ap, "lista", 1);
  let best = null, bk = -1;
  const alt = Math.max(320, window.innerHeight - 300);
  for (let per = n1; per >= 2; per--) {
    const l = impagina(g, ap, "righe", per);
    const k = Math.min((larghezza - 24) / l.W, (alt - 24) / l.H);
    if (k > bk + 0.005) {
      best = l;
      bk = k;
    }
  }
  return best;
}

// ---------------------------------------------------------------- le schede e la tela

function segniScheda(g, v) {
  const out = [];
  if (v.cad) out.push(h("span", { class: "du-segno du-segno--cad", title: "Segnale del motore (LS-02): possibile elemento CAD, non un articolo. È solo un segnale: il nodo resta e il suo 2D resta richiesto finché non decidi." }, "possibile elemento CAD"));
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
function scheda(g, v, ps) {
  const cls = ["du-nc", "du-nc--" + v.tipo];
  if (v.nuovo) cls.push("du-nc--nuovo");
  else if (v.proposto || g.modo === "proposta") cls.push("du-nc--proposto");
  if (v.escluso || v.noninc) cls.push("du-nc--escluso");
  if (S.nodo === v.k) cls.push("du-nc--scelto");
  if (S.selezione.has(v.k)) cls.push("du-nc--selezionato");
  const lungo = String(v.cod || "").length + (v.rev ? 4 + String(v.rev).length : 0);
  const card = h("div", { class: cls.join(" "), id: "nc-" + sid(v.k), "data-k": v.k, "data-du-stato": v.stato.t, style: "left:" + ps.x + "px;top:" + ps.y + "px;width:" + CW + "px;height:" + CH + "px",
    title: (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "") + " · " + (v.desc || "") });
  card.append(h("button", { type: "button", class: "du-nc-cod" + (lungo > 17 ? " du-nc-cod--l" : ""), id: "oc-" + sid(v.k),
    "aria-label": (TIPI[v.tipo] || v.tipo) + " " + (v.cod || "senza codice") + (v.rev ? " rev " + v.rev : "") + ", " + v.stato.t + ". Invio: apri nel pannello", "aria-current": S.nodo === v.k ? "true" : null },
  h("span", { class: "du-nc-c" }, v.cod || "(senza codice)"), v.rev ? h("span", { class: "du-nc-rev" }, "rev " + v.rev) : null));
  card.append(h("span", { class: "du-nc-desc" }, v.desc || "—"));
  card.append(h("div", { class: "du-nc-chips" }, h("span", { class: "du-tipo du-tipo--" + v.tipo }, TIPI[v.tipo] || v.tipo), h("span", { class: "du-nc-st du-nc-st--" + v.stato.c }, v.stato.t)));
  card.append(h("div", { class: "du-nc-piede" }, segniScheda(g, v)));
  const sel = selezionabile(g, v);
  if (g.modo === "bozza" && sel.ok) {
    card.append(h("input", { type: "checkbox", class: "du-nc-sel", id: "sel-" + sid(v.k), checked: S.selezione.has(v.k), "aria-label": "Seleziona " + (v.cod || v.desc || "il pezzo") + " per «Non includere»",
      onclick: (e) => {
        e.stopPropagation();
        cambiaSelezione(v.k, e.target.checked);
      } }));
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
  return card;
}
function cambiaSelezione(k, si) {
  if (si) S.selezione.add(k);
  else S.selezione.delete(k);
  render();
}
function scegli(k) {
  S.nodo = k;
  S.zona = "nodo";
  scriviIndirizzo();
  render();
}
function qPill(a, g) {
  if (g.modo === "proposta") return h("span", { class: "du-q du-q--prop" }, "×" + a.qta);
  if (a.kind === "bozza") return h("span", { class: "du-q du-q--bozza" }, "✎×" + a.qta);
  if (a.kind === "conf") return h("span", { class: "du-q du-q--conf" }, "✓×" + a.qta);
  return h("span", { class: "du-q du-q--prop" }, "×" + a.qta);
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
  return S.prodotto + ":" + S.passo + ":" + (S.passo === "sorgente" ? S.anteprimaAllegato + "|" + S.anteprimaGruppo : "bozza");
}
function organigramma(g) {
  const larghezza = Math.max(260, ($("#du-corpo") ? $("#du-corpo").getBoundingClientRect().width : 900) - (window.innerWidth >= 900 ? 340 : 30));
  const lay = layout(g, larghezza);
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
    mondo.append(h("button", { type: "button", class: "du-nc du-nc--" + v.tipo, style: "left:" + s.x + "px;top:" + s.y + "px;width:" + s.w + "px;height:" + SH + "px;display:flex;align-items:center;gap:6px",
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
  S.camere[L.chiave] = c;
  applicaCam();
}
function vistaAdatta() {
  const d = vpDim();
  const l = L.lay;
  const k = clamp(Math.min((d.w - 24) / l.W, (d.h - 24) / l.H), KMIN, 1);
  return { k, tx: (d.w - l.W * k) / 2, ty: Math.max(12, (d.h - l.H * k) / 2) };
}
function vistaIniziale() {
  const a = vistaAdatta();
  if (a.k >= 0.7) return { k: a.k, tx: a.tx, ty: 12 };
  const d = vpDim();
  const l = L.lay;
  const k = clamp(Math.min(1, Math.max(0.75, (d.w - 24) / l.W)), KMIN, 1);
  const r = l.pos[L.g.radice] || { x: 0 };
  const tx = l.W * k <= d.w - 24 ? (d.w - l.W * k) / 2 : Math.min(12, d.w - 12 - (r.x + l.cw) * k);
  return { k, tx, ty: 12 };
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
    if (puoTrascinare(card.dataset.k)) V.drag = { k: card.dataset.k, x0: e.clientX, y0: e.clientY, attivo: false, card };
    return;
  }
  if (!card && e.target.closest("button, input, select, a")) return;
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
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const t = el && el.closest ? el.closest(".du-vp .du-nc[data-k]") : null;
    if (V.target && V.target !== t) V.target.classList.remove("du-nc--drop", "du-nc--nodrop");
    V.target = t && t.dataset.k !== d.k ? t : null;
    if (V.target) {
      const no = vietatoSotto(d.k, V.target.dataset.k);
      V.target.classList.toggle("du-nc--drop", !no);
      V.target.classList.toggle("du-nc--nodrop", !!no);
    }
  }
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
    if (!d.attivo || annullato) return;
    V.ultimoMov = Date.now();
    if (!t) return avvisa("no", "Rilasciato fuori da una scheda: nessuna modifica alla bozza.");
    const no = vietatoSotto(d.k, t.dataset.k);
    if (no) return avvisa("no", "Lo spostamento non si fa: " + no);
    apriSposta(d.k, t.dataset.k);
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
  if (!contenitore(v)) return (v.cod || "il pezzo") + " è " + (v.tipo === "commerciale" ? "un commerciale" : "un particolare") + ": non ha figli (E37). Il tipo non cambia da solo.";
  if (discendenti(g, k).has(verso)) return (v.cod || "il pezzo") + " sta già sotto il pezzo che sposti (ciclo).";
  return "";
}

// ---------------------------------------------------------------- render

function render() {
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
      onclick: () => scegliProdotto(p.id) }, h("span", { class: "du-mono" }, p.codice || p.rif), p.descrizione ? h("span", { class: "du-muto du-piccolo" }, p.descrizione) : null));
  }
}
async function scegliProdotto(id) {
  if (id === S.prodotto) return;
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "Il prodotto non cambia: le modifiche di questo prodotto non sono salvate.");
  }
  S.prodotto = id;
  S.nodo = "";
  S.anteprima = null;
  S.anteprimaAllegato = "";
  S.anteprimaGruppo = "";
  await caricaModello();
  await Promise.all([caricaBozza(), caricaStorico()]);
  render();
}
function chipSorgente() {
  const f = S.modello && S.modello.fonte;
  if (!f) return pill("neu", "fonte: non letta");
  return h("span", { class: "du-ptab-r" }, pill(f.confermata ? "ok" : "ver", f.confermata ? "fonte confermata" : "fonte da confermare"),
    pill(f.autorizzata ? "ok" : "neu", f.autorizzata ? "STEP autorizzato" : "STEP non autorizzato"));
}
function chipRevisione() {
  if (!bozzaAperta()) return pill("neu", "nessuna bozza");
  if (bozzaDaConvertire()) return pill("ver", "bozza da convertire");
  const base = S.salvato && S.salvato.versione_bom_base;
  const n = S.coda.length + (S.invio ? S.invio.voci.length : 0);
  return pill("bozza", "bozza " + (base && base.numero ? "su v" + base.numero : "senza conferma") + " · versione " + S.bozza.versione + (n ? " · " + n + " da salvare" : ""));
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
  if (S.erroreBozza && S.passo === "revisione") add("ko", "La bozza non si legge: " + S.erroreBozza, null, "du-errore-bozza");
  const sosp = P.sospesi();
  for (const s of sosp) {
    // la conferma di questo prodotto verificata «non registrata»: «Riprova la conferma» riapre il wizard, che rilegge il
    // riepilogo e manda lo stesso corpo; il ponte riusa la chiave se il corpo è lo stesso (MAT §5.2; §5.5 n. 4)
    const riprova = s.nome === "confermaBom" && s.verificato && s.risorsa === "bozza:" + S.prodotto && bozzaLavorabile();
    add("ver", "Un tentativo («" + (s.etichetta || s.nome) + "») non ha un esito certo" + (s.verificato ? ": il server dice che non è ancora registrato." : ": lo verifico con la stessa chiave."),
      h("span", { class: "du-azioni" }, btn("Verifica ora", () => verificaSospeso(s), { cls: "du-btn--sm", perche: false }),
        riprova ? btn("Riprova la conferma…", riprovaConfermaSospesa, { cls: "du-btn--sm du-btn--pri", id: "du-riprova-sospesa", perche: false }) : null,
        btn("Abbandona il tentativo…", () => abbandonaTentativo(s), { cls: "du-btn--sm du-btn--ghost", perche: false })), "du-sospeso-" + sid(s.risorsa));
  }
  if (!bozzaAperta()) return;
  if (bozzaDaConvertire()) {
    add("ver", "Bozza nel formato di prima: per lavorarla va convertita (la bozza vecchia si chiude, la nuova parte dalla stessa struttura).",
      h("span", { class: "du-azioni" }, btn("Converti…", apriConverti, { cls: "du-btn--sm du-btn--pri", id: "du-converti", perche: false }),
        btn("Abbandona la bozza…", apriAbbandona, { cls: "du-btn--sm", perche: false })), "du-formato-vecchio");
    return;
  }
  const b = S.bozza;
  if (b.base && b.base.stato === "superata") {
    add("ver", "La BOM o lo STEP sono cambiati dopo la tua base (" + arr(b.base.motivi).join(", ") + "): serve il rebase. La bozza si salva comunque; la conferma no.",
      btn("Fai il rebase…", apriRebase, { cls: "du-btn--sm", id: "du-rebase", perche: false }), "du-base-superata");
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
      extra: h("span", { class: "du-azioni" }, btn("Vedi le differenze", apriDifferenze, { cls: "du-btn--sm", id: "du-differenze", perche: false }),
        btn("Aggiorna alla proposta di oggi…", apriRiallinea, { cls: "du-btn--sm", id: "du-riallinea", perche: false })) };
  }
  if (g.come === "altra_occorrenza") {
    return { tono: "info", testo: "La BOM di lavoro di " + prodotto + " sta su un'altra copia dello stesso file (" + nomeFile(g.allegato_id) + "): la bozza resta valida sulla sua copia; per seguirla, «Cambia lo STEP della BOM…».",
      extra: btn("Guarda l'altra copia", () => {
        S.passo = "sorgente";
        guarda(g.allegato_id, "bom");
      }, { cls: "du-btn--sm", perche: false }) };
  }
  if (g.come === "assente") {
    if (g.motivo === "occorrenza_non_attiva") return { tono: "info", testo: "La BOM di lavoro di " + prodotto + " sta su una copia dello stesso file che non è attiva (" + nomeFile(g.allegato_id) + ", in Altri file o scartata): la bozza resta valida; per seguirla, prima riattiva il file." };
    return { tono: "info", testo: "Il Motore A non propone più questa struttura per " + prodotto + ": la bozza resta valida." };
  }
  return null;
}

function renderCorpo() {
  const box = $("#du-corpo");
  if (!box) return;
  box.removeAttribute("aria-busy");
  L = null;
  box.replaceChildren(S.passo === "revisione" ? corpoRevisione() : corpoSorgente());
  if (L) {
    const c = cam();
    if (!c) setCam(vistaIniziale());
    else applicaCam();
  }
}

// ---------------------------------------------------------------- Sorgente

function corpoSorgente() {
  const g = S.anteprima ? grafoProposta() : null;
  const vista = h("section", { class: "du-zona du-zona--vista", "aria-label": "Sorgente" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, "Sorgente"), h("span", { class: "du-zona-sub" }, "Gli STEP e i file della RFQ. Guardare una proposta non conferma niente.")),
    h("div", { class: "du-zona-corpo" }, riquadroFonte(), riquadroStep(), S.anteprima ? riquadroAnteprima(g) : null, riquadroFile()));
  const nodo = h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, S.nodo && g && g.nodi[S.nodo] ? "Nodo della proposta" : "Pannello del nodo")),
    h("div", { class: "du-zona-corpo" }, g && g.nodi[S.nodo] ? pannelloNodo(g, g.nodi[S.nodo]) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo della proposta per vedere identità proposta, riferimenti e provenienza.")));
  return h("div", {}, zoneTabs(), h("div", { class: "du-ws", "data-zona": S.zona }, vista, nodo));
}
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
function riquadroFonte() {
  const f = S.modello && S.modello.fonte;
  const prodotto = codiceProdotto();
  if (!prodottoScelto()) return null;
  const noConf = !f ? "il modello non dice la fonte del prodotto" : f.confermata ? "la fonte è già confermata" : !f.conferma_ammessa ? f.motivo || "lo STEP non è associato al prodotto" :
    !f.voce ? "il modello non dice la voce del documento STEP da confermare" : "";
  const noAut = !f ? "il modello non dice la fonte del prodotto" : f.autorizzata ? "lo STEP è già autorizzato" : !f.confermata ? "prima conferma la fonte" : "";
  return h("div", { class: "du-riq", id: "du-fonte" },
    h("div", { class: "du-riq-testa" }, h("h3", {}, "Fonte di " + prodotto), chipSorgente()),
    h("p", { class: "du-nota" }, "Eccezione dichiarata: la fonte STEP si conferma prima della BOM. Confermare la fonte e autorizzare lo STEP non confermano la struttura: la BOM diventa ufficiale solo con «Conferma BOM»."),
    h("div", { class: "du-azioni" },
      btn("Conferma la fonte…", apriConfermaFonte, { cls: "du-btn--sm", id: "du-conferma-fonte", no: noConf }),
      btn("Autorizza lo STEP strutturale…", apriAutorizza, { cls: "du-btn--sm", id: "du-autorizza", no: noAut }),
      f && f.autorizzata ? btn("Revoca l'autorizzazione…", apriRevoca, { cls: "du-btn--sm du-btn--danger", id: "du-revoca", perche: false }) : null,
      btn("Rifiuta la candidata…", null, { cls: "du-btn--sm", no: "Il rifiuto motivato di una candidata arriva in V5: intanto si sceglie un altro STEP" })));
}
function stepDellaRfq() {
  const st = arr(S.sorgente && S.sorgente.step);
  if (st.length) return st;
  return fileSorgente().filter((f) => /^(stp|step)$/i.test(f.estensione || "") && f.stato !== "scartato");
}
function riquadroStep() {
  const elenco = stepDellaRfq();
  const box = h("div", { class: "du-riq", id: "du-step" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "STEP della RFQ"),
    h("span", { class: "du-muto du-piccolo" }, "Per ogni STEP: che cosa propone il motore per " + (codiceProdotto() || "il prodotto") + ".")));
  if (!elenco.length) {
    box.append(h("p", { class: "du-muto du-piccolo" }, "Nessuno STEP fra i file attivi: carica lo STEP del prodotto con «+ Aggiungi file»."));
    return box;
  }
  const ul = h("ul", { class: "du-fl" });
  for (const s of elenco) {
    const f = fileDi(s.allegato_id) || s;
    const attivo = !f.stato || f.stato === "attivo";
    const inVista = S.anteprimaAllegato === s.allegato_id;
    ul.append(h("li", { class: "du-fr" + (inVista ? " du-fr--sel" : ""), id: "du-step-" + sid(s.allegato_id) },
      h("div", {}, h("div", { class: "du-fr-nome" }, s.nome_file || f.nome_file), h("div", { class: "du-fr-meta" }, pill("neu", testoAnalisi(f)), inVista ? pill("prop", "in vista, non confermata") : null)),
      h("div", { class: "du-azioni" }, btn("Guarda", () => guarda(s.allegato_id, ""), { cls: "du-btn--sm", id: "du-guarda-" + sid(s.allegato_id), no: !S.prodotto ? "nessun prodotto valutato" : !attivo ? "il file non è attivo" : "" }))));
  }
  box.append(ul);
  return box;
}
async function guarda(allegato, gruppo, silenzioso) {
  if (!S.prodotto) return;
  S.anteprimaAllegato = allegato;
  S.anteprimaGruppo = gruppo || "";
  S.erroreAnteprima = "";
  if (!silenzioso) avvisa("ok", "Lettura della proposta dello STEP…");
  try {
    const opz = { gruppo: gruppo || undefined };
    if (bozzaLavorabile()) opz.bozza = S.bozza.bozza_id;
    const a = await P.anteprimaStep(allegato, S.prodotto, opz);
    S.anteprima = a;
    if (!gruppo) {
      const inVista = arr(a.gruppi).find((x) => x.in_vista);
      S.anteprimaGruppo = inVista ? inVista.gruppo : "";
    }
    if (!silenzioso) avvisa("ok", a.errore ? "Il calcolo della proposta non è riuscito: " + a.errore : "Proposta dello STEP in vista, non confermata.");
  } catch (e) {
    S.anteprima = null;
    S.erroreAnteprima = testoErrore(e);
    avvisa("errore", "La proposta dello STEP non si legge: " + S.erroreAnteprima);
  }
  S.nodo = "";
  render();
}
function riquadroAnteprima(g) {
  const a = S.anteprima;
  const box = h("div", { class: "du-riq du-riq--cand", id: "du-anteprima" });
  box.append(h("div", { class: "du-riq-testa" }, h("h3", {}, "Proposta di " + (a.nome_file || nomeFile(a.allegato_id))), pill("prop", "in vista, non confermata"),
    btn("Chiudi", () => {
      S.anteprima = null;
      S.anteprimaAllegato = "";
      render();
    }, { cls: "du-btn--sm du-btn--ghost", perche: false })));
  if (a.errore) {
    box.append(h("div", { class: "du-striscia du-striscia--ko" }, "Il calcolo della proposta non è riuscito: " + a.errore));
    return box;
  }
  if (a.stato !== "pronta") {
    const testi = { in_analisi: "Lo STEP è ancora in analisi.", non_scaricato: "Lo STEP non è scaricato.", errore: "L'analisi dello STEP è in errore.",
      struttura_non_determinabile: "La struttura dello STEP non è determinabile" + (a.struttura && a.struttura.motivo ? ": " + a.struttura.motivo : "") + ". Nessuna struttura inventata." };
    box.append(h("div", { class: "du-striscia du-striscia--ver" }, testi[a.stato] || "Proposta non disponibile (" + a.stato + ")."));
    return box;
  }
  const gruppi = arr(a.gruppi);
  if (gruppi.length > 1) {
    box.append(h("div", { class: "du-cartelle", role: "group", "aria-label": "Strutture dello STEP" }, gruppi.map((x) => h("button", { type: "button", "aria-pressed": x.gruppo === S.anteprimaGruppo ? "true" : "false",
      title: "compatibilità: " + (x.compatibilita || "?"), onclick: () => guarda(a.allegato_id, x.gruppo) }, (x.radice && x.radice.identita_proposta && x.radice.identita_proposta.letterale) || x.gruppo.slice(0, 18), " · ", x.compatibilita || ""))));
  }
  const gr = gruppi.find((x) => x.gruppo === S.anteprimaGruppo) || null;
  box.append(h("p", { class: "du-nota" }, "Codice del prodotto: ", h("span", { class: "du-mono" }, a.codice_prodotto || codiceProdotto()), ". Radice letta: ",
    a.confronto === "coincide" ? "coincide con il prodotto." : a.confronto === "diverso" ? "diversa dal prodotto: per usarla serve un motivo." : "non determinabile."));
  box.append(azioniAnteprima(a, gr));
  if (S.vista === "org" && g && g.radice) box.append(strumentiOrg(g), organigramma(g));
  else if (g && g.radice) box.append(elencoRighe(g));
  else box.append(h("p", { class: "du-muto du-piccolo" }, "Scegli una struttura dello STEP per vederla."));
  return box;
}
function azioniAnteprima(a, gr) {
  const prodotto = codiceProdotto();
  const az = h("div", { class: "du-azioni" });
  const f = fileDi(a.allegato_id);
  const associato = f && f.proposta && f.proposta.componente_id === S.prodotto;
  az.append(associato ? pill("ok", "STEP associato a " + prodotto) : btn("Associa lo STEP a " + prodotto + "…", () => apriAssocia(a), { cls: "du-btn--sm", id: "du-associa-step",
    no: !f ? "lo STEP non è fra i file della Sorgente" : !f.proposta ? "lo STEP non ha una proposta di documento" : "" }));
  if (!gr) return az;
  const noGr = !gr.impronta_contenuto ? "il server non dà l'impronta della proposta" : "";
  if (!bozzaAperta()) {
    az.append(btn("Usa per la BOM di " + prodotto + "…", () => apriUsaPerBom(a, gr), { cls: "du-btn--sm du-btn--pri", id: "du-usa-bom", no: noGr || (S.modello && S.modello.congelata ? "la BOM è congelata" : "") }));
  } else if (bozzaDaConvertire()) {
    az.append(btn("Cambia lo STEP della BOM…", null, { cls: "du-btn--sm", no: "prima converti la bozza" }));
  } else if (a.effetto_sulla_bozza && a.effetto_sulla_bozza.stessa_sorgente) {
    az.append(btn("Aggiorna alla proposta di oggi…", apriRiallinea, { cls: "du-btn--sm", id: "du-riallinea-da-step", no: perchéNoModifica() }));
  } else {
    az.append(btn("Cambia lo STEP della BOM…", () => apriCambiaSorgente(a, gr), { cls: "du-btn--sm", id: "du-cambia-sorgente", no: noGr || perchéNoModifica() }));
  }
  return az;
}

// I file della Sorgente: cartelle «File attivi», «Altri file», «Scartati» (D23:290; MAT §3.3).
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
async function carica(files, origine) {
  avvisa("ok", "Caricamento in corso…");
  const r = await P.carica(files, origine);
  const ul = moduloCarica().__esiti;
  ul.replaceChildren();
  for (const f of arr(r.file)) {
    const testo = f.esito === "caricato" ? (f.ricaricato_dopo_scarto ? "ricaricato: è un file nuovo della RFQ; lo scarto di prima resta nella storia" : "caricato · " + (ANALISI[f.analisi] || f.analisi || "in analisi"))
      : f.esito === "gia_presente" ? "già presente" + (f.stato_file === "altri_file" ? " (in Altri file: Riattiva)" : "")
      : "rifiutato: " + ((f.motivo && f.motivo.testo) || "motivo non dato");
    ul.append(h("li", { "data-esito": f.esito }, h("span", { class: "du-mono" }, f.nome_file), pill(f.esito === "rifiutato" ? "ko" : f.esito === "gia_presente" ? "neu" : "ok", testo)));
  }
  if (r.esito === "caricati") avvisa("ok", r.testo || "File caricati.");
  else if (r.esito === "parziale") avvisa("no", "Caricamento parziale: " + (r.testo || "alcuni file non sono stati caricati") + ". Non caricati: " + arr(r.file).filter((f) => f.esito === "rifiutato").map((f) => f.nome_file).join(", ") + ".");
  else avvisa(r.esito === "no" ? "no" : "errore", r.testo || "Il caricamento non è riuscito.");
  await caricaSorgente();
  await caricaModello();
  render();
}
function riquadroFile() {
  const tutti = fileSorgente();
  const cart = { attivi: tutti.filter((f) => !f.stato || f.stato === "attivo"), altri: tutti.filter((f) => f.stato === "altri_file"), scartati: tutti.filter((f) => f.stato === "scartato") };
  const box = h("div", { class: "du-riq", id: "du-file" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "File della RFQ"),
    h("div", { class: "du-cartelle", role: "group", "aria-label": "Cartelle dei file" },
      [["attivi", "File attivi"], ["altri", "Altri file"], ["scartati", "Scartati"]].map(([k, t]) => h("button", { type: "button", id: "du-cartella-" + k, "aria-pressed": S.cartella === k ? "true" : "false",
        onclick: () => {
          S.cartella = k;
          render();
        } }, t + " (" + cart[k].length + ")")))));
  if (S.erroreSorgente) box.append(h("div", { class: "du-striscia du-striscia--ko" }, "I file della Sorgente non si leggono: " + S.erroreSorgente));
  if (S.cartella === "attivi") box.append(moduloCarica());
  if (S.cartella === "altri") box.append(h("p", { class: "du-nota" }, "Altri file: restano nella RFQ, si aprono e si scaricano quando serve, anche dopo un riavvio e dopo la pulizia dello staging. Non entrano nella proposta della BOM e non ricevono associazioni finché non li riattivi. Non è la cartella ALTRO del NAS."));
  const elenco = cart[S.cartella];
  if (!elenco.length) box.append(h("p", { class: "du-muto du-piccolo" }, S.cartella === "attivi" ? "Nessun file attivo." : S.cartella === "altri" ? "Nessun file in Altri file." : "Nessun file scartato."));
  const ul = h("ul", { class: "du-fl" });
  for (const f of elenco) ul.append(rigaFile(f));
  box.append(ul);
  return box;
}
function rigaFile(f) {
  const cls = "du-fr" + (f.stato === "altri_file" ? " du-fr--altri" : f.stato === "scartato" ? " du-fr--scartato" : "");
  const meta = [pill("neu", f.origine === "interna" ? "interna" : f.canale === "posta" ? "posta" : "del cliente"), pill(f.analisi && f.analisi.stato === "errore" ? "ko" : "neu", testoAnalisi(f))];
  if (f.proposta && f.proposta.codice) meta.push(h("span", { class: "du-mono" }, f.proposta.codice + (f.proposta.rev ? " rev " + f.proposta.rev : "")));
  if (f.scartato_legacy) meta.push(pill("neu", "Scartato (gesto precedente)"));
  const az = h("div", { class: "du-azioni du-fr-az" });
  const url = P.urlContenuto(f.allegato_id, "anteprima");
  const contenuto = f.azioni && f.azioni.contenuto;
  if (f.stato !== "scartato") {
    az.append(contenuto && contenuto.disponibile === false ? btn("Apri", null, { cls: "du-btn--sm", no: "contenuto non disponibile: " + (contenuto.motivo || "") }) :
      h("a", { class: "du-btn du-btn--sm", href: url, target: "_blank", rel: "noopener", id: "du-apri-" + sid(f.allegato_id) }, "Apri"));
  }
  const sc = azioneFile(f, "scarta"), af = azioneFile(f, "altri_file"), ri = azioneFile(f, "riattiva");
  if (f.stato !== "scartato" || f.scartato_legacy) az.append(btn("Scarta…", () => apriScarta(f), { cls: "du-btn--sm du-btn--danger", id: "du-scarta-" + sid(f.allegato_id), no: sc.ok ? "" : sc.motivo }));
  if (!f.stato || f.stato === "attivo") az.append(btn("Metti in Altri file…", () => apriAltriFile(f), { cls: "du-btn--sm", id: "du-altri-" + sid(f.allegato_id), no: af.ok ? "" : af.motivo }));
  if (f.stato === "altri_file") az.append(btn("Riattiva", () => apriRiattiva(f), { cls: "du-btn--sm", id: "du-riattiva-" + sid(f.allegato_id), no: ri.ok ? "" : ri.motivo }));
  const traccia = [];
  if (f.stato === "scartato") {
    const ev = arr(f.eventi).find((e) => e.azione === "scarta") || f.scarto || null;
    traccia.push(ev ? "Scartato da " + (ev.autore || ev.autore_id || "?") + " il " + dataOra(ev.quando) + ": " + (ev.motivo || "") + ". " : "");
    traccia.push(testoRimozione(f));
    traccia.push(f.canale === "posta" ? " Per riaverlo: salvalo dalla mail e ricaricalo." : " Per riaverlo: ricaricalo.");
  }
  if (f.stato === "altri_file" && f.conservazione) {
    const c = f.conservazione.stato;
    traccia.push(c === "copiata" ? "Conservato (copia verificata)." : c === "in_copia" ? "Conservazione in corso." : c === "mancante" ? "Copia nel deposito mancante: il file resta nello staging." : "Conservazione: " + c + ".");
  }
  return h("li", { class: cls, id: "du-file-" + sid(f.allegato_id), "data-stato": f.stato || "attivo" },
    h("div", {}, h("div", { class: "du-fr-nome" }, f.nome_file), h("div", { class: "du-fr-meta" }, meta)),
    h("div", {}),
    traccia.join("").trim() ? h("div", { class: "du-fr-traccia", "data-du-traccia": "1" }, traccia.join("").trim()) : null,
    az);
}

// ---------------------------------------------------------------- Revisione

function corpoRevisione() {
  if (!S.prodotto) return h("div", { class: "du-vuoto" }, "Nessun prodotto valutato: la Revisione non ha una BOM da mostrare.");
  if (S.erroreModello) return h("div", { class: "du-vuoto" }, "Il calcolo del motore non è riuscito: niente tela, bozza né wizard finché non riesce (D-18).");
  if (!bozzaAperta()) return revisioneSenzaBozza();
  if (bozzaDaConvertire()) return h("div", { class: "du-vuoto" }, "La bozza aperta è nel formato di prima: convertila per lavorarla (striscia qui sopra).");
  const g = grafoBozza();
  if (S.nodo && !g.nodi[S.nodo]) S.nodo = "";
  const vista = h("section", { class: "du-zona du-zona--vista", "aria-label": "BOM di lavoro" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, S.vista === "org" ? "Organigramma della bozza" : "Elenco della bozza"), statoConferma()),
    h("div", { class: "du-zona-corpo" }, S.vista === "org" ? [strumentiOrg(g), cassettoSegnalati(g), barraSelezione(g), organigramma(g)] : [vassoio(), elencoRighe(g)]));
  const nodo = h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, "Pannello del nodo")),
    h("div", { class: "du-zona-corpo" }, g.nodi[S.nodo] ? pannelloNodo(g, g.nodi[S.nodo]) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo: identità proposta, riferimenti, provenienza e decisione dell'ingegnere stanno qui, non sulle schede.")));
  return h("div", {}, zoneTabs(), h("div", { class: "du-ws", "data-zona": S.zona }, vista, nodo));
}
function revisioneSenzaBozza() {
  const ultima = arr(S.storico && S.storico.conferme).find((c) => c.ultima) || null;
  return h("div", { class: "du-vuoto", id: "du-nessuna-bozza" },
    h("p", {}, "Nessuna bozza per " + codiceProdotto() + ". La BOM di lavoro nasce dalla scelta dello STEP nella Sorgente («Usa per la BOM»)."),
    h("div", { class: "du-azioni" },
      btn("Vai alla Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm", perche: false }),
      btn("Modifica la BOM confermata", () => creaDa("conferma"), { cls: "du-btn--sm", id: "du-da-conferma", no: ultima ? "" : "il prodotto non ha una conferma" }),
      btn("Parti dalla BOM registrata", () => creaDa("registrato"), { cls: "du-btn--sm", id: "du-da-registrato", perche: false })),
    S.erroreStorico ? h("p", { class: "du-muto du-piccolo" }, "Lo storico delle conferme non si legge: " + S.erroreStorico) : null);
}
// statoConferma: «vN confermata» e «Verificata» sono due segni distinti, mai fusi (MAT §6.2).
function statoConferma() {
  const conf = arr(S.storico && S.storico.conferme);
  const ultima = conf.find((c) => c.ultima) || null;
  const out = h("span", { class: "du-azioni", id: "du-stato-conferma" });
  if (ultima) out.append(pill(ultima.valida ? "ok" : "ver", ultima.valida ? "v" + ultima.numero + " confermata il " + dataOra(ultima.confermato_il) : "v" + ultima.numero + " confermata, poi la struttura è cambiata: serve una conferma nuova", "conferma n. " + ultima.numero));
  else out.append(pill("neu", "nessuna conferma"));
  const daR5 = conf.some((c) => c.origine && c.origine.rotta === "motore-a/bom/conferma");
  const p = prodottoScelto();
  if (daR5) out.append(pill("neu", "Verificata: non ancora calcolata (arriva in V2)"));
  else if (p) out.append(pill(p.verificato ? "ok" : "neu", "Verificata (calcolo di oggi): " + (p.verificato ? "sì" : "no"), arr(p.motivi).map((m) => (m && (m.codice || m.motivo)) || m).join(", ")));
  return out;
}
function strumentiOrg(g) {
  const parti = [];
  if (g.modo === "bozza") {
    const no = perchéNoModifica();
    parti.push(h("span", { class: "du-grp", role: "group", "aria-label": "Aggiungi" },
      btn("+ Assieme", () => apriAggiungi("sottoassieme"), { cls: "du-btn--sm", id: "du-nuovo-ass", no, perche: false }),
      btn("+ Particolare", () => apriAggiungi("sciolto"), { cls: "du-btn--sm", id: "du-nuovo-par", no, perche: false }),
      btn("+ Commerciale", () => apriAggiungi("commerciale"), { cls: "du-btn--sm", id: "du-nuovo-com", no, perche: false })));
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
      h("input", { type: "checkbox", checked: S.selezione.has(v.k), disabled: !sel.ok, title: sel.motivo || null, "aria-label": "Seleziona " + ((v.voce && v.voce.grezzi && v.voce.grezzi.nome) || v.k),
        onchange: (e) => cambiaSelezione(v.k, e.target.checked) }),
      h("span", { class: "du-segnalato-n" }, (v.voce && v.voce.grezzi && (v.voce.grezzi.nome || v.voce.grezzi.id)) || v.desc || v.k),
      h("span", { class: "du-segnalato-s" }, (padre ? "sotto " + (g.nodi[padre.p].cod || g.nodi[padre.p].desc) + " · " : "") + (seg.regola || "") + (seg.spiegazione ? " · " + seg.spiegazione : "") +
        (arr(seg.contro_indizi).length ? " · contro-indizi: " + arr(seg.contro_indizi).join(", ") : ""))));
  }
  box.append(c);
  return box;
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
  const out = { pb: [], pa: [], ar: [] };
  if (!S.lavoro || !v.nodo) return out;
  for (const d of S.lavoro.documenti_preassociati) if (d.chiave_bozza === v.k) out.pb.push(d.proposta_id);
  const giaPb = new Set(S.lavoro.documenti_preassociati.map((d) => d.proposta_id));
  for (const a of arr(v.voce && v.voce.associazioni_proposte)) {
    const f = fileDi(a.allegato_id);
    if (f && f.proposta_id && !giaPb.has(f.proposta_id) && (!f.stato || f.stato === "attivo")) out.pa.push({ proposta_id: f.proposta_id, allegato_id: a.allegato_id, motivo: a.motivo, livello: a.livello });
  }
  if (v.nodo.componente_id) for (const f of fileSorgente()) if (f.proposta && f.proposta.componente_id === v.nodo.componente_id && f.proposta_id && !giaPb.has(f.proposta_id)) out.ar.push(f);
  return out;
}
function svincoloDi(pid) {
  return S.lavoro ? S.lavoro.decisioni.find((d) => d.tipo === "scollega_documento" && d.proposta_id === pid) : null;
}
function esitoPreassociazione(pid) {
  const r = S.riepilogo;
  const e = r && arr(r.documenti_preassociati).find((x) => x.proposta_id === pid);
  return e ? e.esito : "";
}
function elencoRighe(g) {
  const righe = ordineAlbero(g);
  const box = h("div", { class: "du-el", id: "du-elenco", role: "table", "aria-label": "Elenco dei pezzi" });
  box.append(h("div", { class: "du-el-r du-el-r--testa", role: "row" }, h("span", { role: "columnheader" }, "Pezzo"), h("span", { role: "columnheader" }, "Tipo · qtà"),
    h("span", { role: "columnheader" }, "Documenti"), h("span", { role: "columnheader" }, "Azioni")));
  for (const r of righe) {
    const v = g.nodi[r.k];
    const doc = g.modo === "bozza" ? documentiNodo(v) : { pb: [], pa: [], ar: [] };
    const celleDoc = [];
    for (const pid of doc.pb) {
      const f = fileDiProposta(pid);
      const es = esitoPreassociazione(pid);
      celleDoc.push(h("span", { class: "du-el-doc" }, pill("bozza", "◈ preassociato"), h("span", { class: "du-mono" }, f ? f.nome_file : pid.slice(0, 8)), es ? pill(es === "applicabile" ? "ok" : "ver", es) : null,
        btn("Togli la preassociazione", () => togliPreassociazione(pid, v.k), { cls: "du-btn--sm du-btn--ghost", no: perchéNoModifica(), perche: false })));
    }
    for (const pa of doc.pa) {
      celleDoc.push(h("span", { class: "du-el-doc" }, pill("prop", "◇ proposto"), h("span", { class: "du-mono" }, nomeFile(pa.allegato_id)),
        btn("Accetta la proposta", () => preassocia(pa.proposta_id, v.k), { cls: "du-btn--sm", id: "du-accetta-" + sid(pa.proposta_id), no: perchéNoModifica(), perche: false })));
    }
    for (const f of doc.ar) {
      const sv0 = svincoloDi(f.proposta_id);
      celleDoc.push(h("span", { class: "du-el-doc" }, pill(sv0 ? "ver" : "info", sv0 ? "⊘ da scollegare" : "● associato"), h("span", { class: "du-mono" }, f.nome_file),
        sv0 ? null : btn("Scollega dal componente…", () => apriScollega(f, v), { cls: "du-btn--sm du-btn--ghost", no: perchéNoModifica(), perche: false })));
    }
    if (!celleDoc.length) celleDoc.push(h("span", { class: "du-muto du-piccolo" }, v.tipo === "commerciale" ? "2D non richiesto" : "nessun documento"));
    box.append(h("div", { class: "du-el-r" + (S.nodo === r.k ? " du-el-r--scelta" : "") + (v.escluso || v.noninc ? " du-el-r--escl" : ""), role: "row", id: "el-" + sid(r.k), "data-k": r.k },
      h("button", { type: "button", class: "du-el-nodo", style: "--du-d:" + r.d, onclick: () => scegli(r.k) }, h("span", { class: "du-el-cod" }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "")),
        h("span", { class: "du-el-desc" }, v.desc || "—")),
      h("span", { class: "du-el-celle" }, h("span", { class: "du-tipo du-tipo--" + v.tipo }, TIPI[v.tipo] || v.tipo), r.q ? h("span", { class: "du-q" }, "×" + r.q) : null, pill(v.stato.c === "reg" ? "ok" : v.stato.c, v.stato.t)),
      h("span", { class: "du-el-celle" }, celleDoc),
      h("span", { class: "du-el-celle" }, btn("Conferma il documento", null, { cls: "du-btn--sm", no: "I documenti si confermano solo con «Rivedi e conferma fascicolo» (V3)", perche: false }))));
  }
  return box;
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
  const ultima = new Map();
  for (const e of efficaci) {
    const pid = (e.dopo && e.dopo.proposta_id) || (e.prima && e.prima.proposta_id);
    if (pid) ultima.set(pid, e);
  }
  const out = [];
  for (const [pid, e] of ultima) {
    const motivo = (e.dopo && e.dopo.motivo) || e.motivo || "utente";
    if (e.azione === "file_svincolato" && motivo !== "utente") out.push({ proposta_id: pid, motivo, chiave_bozza: e.prima && e.prima.chiave_bozza });
  }
  return out;
}
function vassoioCorrente() {
  const eventi = arr(S.bozza && S.bozza.eventi).slice();
  let n = S.totaleEventi;
  for (const v of (S.invio ? S.invio.voci : []).concat(S.coda)) for (const a of v.inviare) eventi.push({ n: ++n, azione: a.azione, prima: a.prima, dopo: a.dopo });
  if (!Array.isArray(S.bozza && S.bozza.eventi)) return arr(S.bozza && S.bozza.vassoio);
  return vassoioDa(eventi);
}
function vassoio() {
  const voci = vassoioCorrente();
  if (!voci.length) return null;
  const box = h("div", { class: "du-riq", id: "du-vassoio" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "Da ricollocare (" + voci.length + ")"),
    h("span", { class: "du-muto du-piccolo" }, "File preassociati tolti da un gesto: niente è cancellato.")));
  for (const x of voci) {
    const f = fileDiProposta(x.proposta_id);
    box.append(h("div", { class: "du-gesto" }, h("span", { class: "du-mono" }, f ? f.nome_file : x.proposta_id.slice(0, 8)), pill("neu", x.motivo),
      btn("Preassocia al nodo scelto", () => preassocia(x.proposta_id, S.nodo), { cls: "du-btn--sm", no: !S.nodo || S.nodo.indexOf("ni:") === 0 ? "scegli prima un pezzo della bozza" : perchéNoModifica(), perche: false }),
      btn("Lascia senza associazione", () => lascia(x.proposta_id), { cls: "du-btn--sm du-btn--ghost", id: "du-lascia-" + sid(x.proposta_id), no: perchéNoModifica(), perche: false })));
  }
  return box;
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
function testoLettura(l) {
  if (!l) return "—";
  return (l.letterale || "—") + (l.emittente ? " · " + l.emittente : "") + (l.namespace ? " (" + l.namespace + ")" : "");
}
function pannelloNodo(g, v) {
  const box = h("div", { class: "du-nodo", id: "du-pannello", "data-k": v.k });
  box.append(h("div", {}, h("div", { class: "du-nodo-cod" }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "")), h("div", { class: "du-muto du-piccolo" }, v.desc || "—"),
    h("div", { class: "du-azioni" }, h("span", { class: "du-tipo du-tipo--" + v.tipo }, TIPI[v.tipo] || v.tipo), pill(v.stato.c === "reg" ? "ok" : v.stato.c, v.stato.t),
      v.rivedere && v.rivedere.length ? pill("info", "da rivedere: " + v.rivedere.join(", ")) : null)));
  const voce = v.voce;
  // 1 · identità proposta
  const ip = voce && voce.identita_proposta;
  const s1 = h("div", { class: "du-sez", id: "du-sez-proposta" }, h("h3", {}, "Identità proposta ", h("span", { class: "du-muto du-piccolo" }, "proposta del Motore A")));
  if (!voce) s1.append(h("p", { class: "du-muto du-piccolo" }, v.nuovo ? "Pezzo aggiunto a mano: nessuna identità proposta." : "Nessuna voce del registro per questo pezzo."));
  else {
    s1.append(h("dl", { class: "du-dl" },
      riga("Letterale", ip ? h("span", { class: "du-mono" }, ip.letterale) : "nessuna lettura" + (voce.motivo_lettura ? " (" + voce.motivo_lettura + ")" : "")),
      riga("Emittente", ip ? ip.emittente + (ip.namespace ? " · namespace " + ip.namespace : "") : ""),
      riga("Base", ip && ip.base ? ip.base.normalizzata : ""),
      riga("Qualificatori", ip && ip.qualificatori ? [ip.qualificatori.marcatore, ...arr(ip.qualificatori.affissi).map((a) => a.originale)].filter(Boolean).join(" · ") : ""),
      riga("Revisione letta", ip && ip.revisione ? ip.revisione.originale + " (" + ip.revisione.stato + ")" : ""),
      riga("Codice proposto", voce.codice_proposto ? h("span", { class: "du-mono" }, voce.codice_proposto + (voce.rev_proposta ? " rev " + voce.rev_proposta : "")) : "vuoto" + (voce.motivo_proposto ? " (" + voce.motivo_proposto + ")" : "")),
      riga("Codice del cartiglio", voce.codice_composto && voce.codice_composto.testo ? voce.codice_composto.testo : "")));
  }
  box.append(s1);
  // 2 · riferimenti e relazioni
  const s2 = h("div", { class: "du-sez", id: "du-sez-riferimenti" }, h("h3", {}, "Riferimenti e relazioni"));
  const rif = [];
  for (const a of arr(voce && voce.alternative)) rif.push(h("li", {}, "alternativa: ", h("span", { class: "du-mono" }, testoLettura(a.lettura || a)), a.motivo ? " · " + a.motivo : ""));
  for (const r of arr(voce && voce.riferimenti)) {
    const testo = r.tipo + (r.ruolo ? " (" + r.ruolo + ")" : "") + ": " + (r.lettura ? testoLettura(r.lettura) : r.equivalente ? (r.equivalente.base || "") + " · " + (r.equivalente.emittente || "emittente non concorde") : r.valore || "");
    const proiezione = r.codice_proposto || (r.lettura && r.lettura.codice_proposto) || (r.equivalente && r.equivalente.codice_proposto) || "";
    rif.push(h("li", {}, testo, r.esito ? " · " + r.esito : "", g.modo === "bozza" && v.nodo && (r.lettura || r.equivalente) ? h("span", { class: "du-azioni" },
      btn("Adotta come codice…", () => apriAdotta(v, r, proiezione), { cls: "du-btn--sm", no: !proiezione ? "il registro non dà la proiezione del codice per questa lettura: la calcola il server (proposta di emendamento della corsia)" : perchéNoModifica() })) : null));
  }
  if (voce && voce.codice_manuale_legacy) rif.push(h("li", {}, "codice corretto a mano nella pagina vecchia (evidenza): ", h("span", { class: "du-mono" }, voce.codice_manuale_legacy.codice),
    g.modo === "bozza" ? h("span", { class: "du-azioni" }, btn("Adotta come codice…", () => apriAdotta(v, { tipo: "manuale_legacy" }, voce.codice_manuale_legacy.codice), { cls: "du-btn--sm", no: perchéNoModifica() })) : null));
  const reg = g.modo === "bozza" ? registroBozza() : g.registro;
  for (const r of arr(reg && reg.proposta && reg.proposta.relazioni).filter((x) => voce && x.sorgente && x.sorgente.voce === voce.voce)) {
    rif.push(h("li", {}, "relazione " + r.tipo + " · " + (r.stato || "proposta") + ", non ratificata", h("span", { class: "du-azioni" },
      btn("Accetta o rifiuta", null, { cls: "du-btn--sm", no: "Le relazioni restano proposte: la loro ratifica arriva in una tappa dopo" }))));
  }
  s2.append(rif.length ? h("ul", { class: "du-elenco-p" }, rif) : h("p", { class: "du-muto du-piccolo" }, "Nessun riferimento né relazione."));
  box.append(s2);
  // 3 · provenienza
  const s3 = h("div", { class: "du-sez", id: "du-sez-provenienza" }, h("h3", {}, "Provenienza"));
  const c = v.nodo ? casellaDi(v.nodo) : v.decisione ? v.decisione.casella : null;
  if (c) {
    s3.append(h("dl", { class: "du-dl" }, riga("File", nomeFile(c.allegato_id)), riga("Occorrenza", h("span", { class: "du-mono" }, String(c.allegato_id).slice(0, 8))),
      riga("SHA", h("span", { class: "du-mono" }, String(c.rif).split(":")[1] ? String(c.rif).split(":")[1].slice(0, 12) : "")), riga("Unità di lettura", ip ? ip.unita_id || "" : ""),
      riga("Gruppo", h("span", { class: "du-mono" }, breve(c.gruppo, 40))), riga("Rif", h("span", { class: "du-mono" }, breve(c.rif, 50)))));
    if (g.modo === "bozza" && S.bozza && S.bozza.importazione) {
      const imp = S.bozza.importazione;
      s3.append(h("details", { class: "du-ripiega" }, h("summary", {}, "Regole e versioni dell'importazione"), h("dl", { class: "du-dl" },
        riga("Importato il", dataOra(imp.creata_il)), riga("Impronta", h("span", { class: "du-mono" }, String(imp.impronta || "").slice(0, 16))),
        riga("Regole", h("span", { class: "du-mono" }, breve(JSON.stringify(imp.regole || (reg && reg.regole) || {}), 160))))));
    }
  } else s3.append(h("p", { class: "du-muto du-piccolo" }, "Pezzo senza casella dello STEP."));
  box.append(s3);
  // 4 · decisione dell'ingegnere (solo nella Revisione)
  if (g.modo === "bozza") box.append(sezioneDecisione(g, v), gestiNodo(g, v));
  return box;
}
function autoreDi(chiave) {
  const ev = arr(S.bozza && S.bozza.eventi).filter((e) => e.chiave === chiave || (e.dopo && e.dopo.id === chiave));
  const e = ev[ev.length - 1];
  return e ? (e.autore || "?") + " alle " + ora(e.quando) : "";
}
function sezioneDecisione(g, v) {
  const s = h("div", { class: "du-sez", id: "du-sez-decisione" }, h("h3", {}, "Decisione dell'ingegnere"));
  if (v.noninc) {
    const d = v.decisione;
    s.append(h("dl", { class: "du-dl" }, riga("Decisione", "non includere nella BOM (bozza)"), riga("Motivo", (d.motivo_codice || "") + " · " + (d.motivo || "")),
      riga("Segnale visto", d.segnale_visto ? d.segnale_visto.codice : "nessuno"), riga("Gesto", h("span", { class: "du-mono" }, String(d.azione_id || "").slice(0, 8)))));
    return s;
  }
  const id = v.nodo.identita || {};
  const dec = id.decisione ? S.lavoro.decisioni.find((x) => x.id === id.decisione) : null;
  s.append(h("dl", { class: "du-dl" }, riga("Identità decisa", h("span", { class: "du-mono" }, (id.codice || "(senza codice)") + (id.rev ? " rev " + id.rev : ""))),
    riga("Emittente", emittenteParola(id) + (id.namespace ? " · " + id.namespace : "")), riga("Modo", id.modo || ""),
    dec ? riga("Decisione", dec.tipo + (dec.modo ? " (" + dec.modo + ")" : "") + (dec.motivo ? ": " + dec.motivo : "")) : null,
    dec ? riga("Chi e quando", autoreDi(dec.id) || "non ancora salvata") : null,
    v.escluso ? riga("Esclusione", "escluso alla conferma (archiviato, mai cancellato se ha storia)") : null,
    segnaliDi(v.voce).length ? riga("Segnale LS-02", segnaliDi(v.voce).map((x) => x.spiegazione || x.codice).join(" ")) : null));
  return s;
}
function gestiNodo(g, v) {
  const no = perchéNoModifica();
  const s = h("div", { class: "du-sez du-gesti", id: "du-gesti" }, h("h3", {}, "Gesti sulla bozza"));
  if (v.noninc) {
    s.append(btn("Includi di nuovo", () => includiDiNuovo(v), { cls: "du-btn--sm", id: "du-includi", no }));
    return s;
  }
  const radice = v.k === g.radice;
  const conCasella = !!casellaDi(v.nodo);
  const registrato = !!v.nodo.componente_id;
  s.append(btn("✎ Correggi codice, rev e descrizione…", () => apriCorreggi(v), { cls: "du-btn--sm", id: "du-correggi", no }));
  s.append(btn("◆ Cambia tipo…", () => apriTipo(v), { cls: "du-btn--sm", id: "du-cambia-tipo", no: no || (radice ? "è il prodotto della RFQ: il tipo non cambia (DB7-3)" : registrato ? "il cambio di tipo di un componente registrato vuole la firma dell'effetto, che nessuna rotta di V1 dà (proposta di emendamento della corsia)" : "") }));
  s.append(btn("+ Aggiungi un pezzo sotto…", () => apriAggiungi("sciolto", v.k), { cls: "du-btn--sm", id: "du-aggiungi-sotto", no: no || (contenitore(v) ? "" : "un " + (TIPI[v.tipo] || v.tipo).toLowerCase() + " non ha figli (E37)") }));
  s.append(btn("↳ Sposta sotto…", () => apriSposta(v.k, ""), { cls: "du-btn--sm", id: "du-sposta", no: no || (radice ? "il prodotto della RFQ non va sotto un altro pezzo" : "") }));
  s.append(btn("+ Collega anche sotto…", () => apriCollega(v.k), { cls: "du-btn--sm", id: "du-collega", no: no || (radice ? "il prodotto della RFQ non va sotto un altro pezzo" : "") }));
  s.append(btn("− Scollega da un padre…", () => apriScollegaPadre(v.k), { cls: "du-btn--sm", id: "du-scollega-padre", no: no || (padriA(g, v.k).length ? "" : "il pezzo non ha padri") }));
  s.append(btn("× Quantità…", () => apriQuantita(v.k, ""), { cls: "du-btn--sm", id: "du-quantita", no: no || (padriA(g, v.k).length ? "" : "il pezzo non ha padri") }));
  if (v.escluso) s.append(btn("↺ Ripristina", () => ripristina(v), { cls: "du-btn--sm", id: "du-ripristina", no }));
  else if (registrato && !radice) s.append(btn("Escludi (senza cascata)…", () => apriEscludi(v), { cls: "du-btn--sm du-btn--danger", id: "du-escludi", no }));
  if (!radice && conCasella && !registrato) s.append(btn("Non includere nella BOM…", () => {
    S.selezione = new Set([v.k]);
    apriNonIncludere(g);
  }, { cls: "du-btn--sm du-btn--danger", id: "du-non-includere-nodo", no }));
  if (!radice && !conCasella && !registrato) s.append(btn("Non crearlo (togli dalla bozza)…", () => togliNodo(v), { cls: "du-btn--sm du-btn--danger", id: "du-togli", no }));
  if (registrato && v.nodo.identita && v.nodo.identita.emittente_presunto) s.append(btn("Fissa l'emittente…", () => apriFissaEmittente(v), { cls: "du-btn--sm", id: "du-fissa-emittente", no }));
  return s;
}

// ---------------------------------------------------------------- finestre

let fuocoPrima = null;
function apriFinestra(titolo, corpo, azioni, o) {
  const f = $("#du-finestra");
  const velo = $("#du-velo");
  fuocoPrima = document.activeElement;
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
  if (primo && primo.focus) primo.focus();
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
  if (fuocoPrima && document.body.contains(fuocoPrima)) fuocoPrima.focus();
}
function campo(etichetta, input, aiuto) {
  return h("label", { class: "du-campo" }, h("span", {}, etichetta), input, aiuto ? h("small", { class: "du-muto" }, aiuto) : null);
}
function area(id, segnaposto, max) {
  return h("textarea", { id, placeholder: segnaposto || null, maxlength: max || 1000 });
}
function valore(f, id) {
  const el = f.querySelector("#" + id);
  return el ? String(el.value || "").trim() : "";
}
function qtaValida(v) {
  const q = Number(v);
  return v !== "" && Number.isInteger(q) && q >= 1 && q <= 100000 ? q : null;
}
function apriLimiti() {
  const voci = [
    "La working la scrive solo «Conferma BOM»: dalla pagina nuova nessun gesto strutturale è immediato.",
    "«Conferma la fonte», «Autorizza» e «Revoca» sono atti della fonte: non confermano la struttura. «Autorizza» rilegge lo STEP e riscrive le righe di proposta: le bozze aperte vedono la base superata e propongono il rebase.",
    "Gli atti esistenti rispondono anche ai rifiuti con un esito testuale e fanno girare il rismistamento F8 anche dopo un rifiuto.",
    "Due emittenti con lo stesso letterale non convivono nella working: blocco COLLISIONE_EMITTENTI prima della conferma.",
    "Gli avvisi del motore (proposta cambiata, STEP secondario, non incluso condiviso) sono fuori dalla firma: possono cambiare fra il riepilogo e la conferma.",
    "Un nodo senza righe legacy, o con righe chiuse da un automatismo, non si può non includere (NON_INCLUSO_NON_APPLICABILE).",
    "Il fascicolo, la conferma dei documenti, le deroghe, la sostituzione dei documenti e la verifica dall'ultima conferma non sono in V1.",
    "La pagina è solo per l'amministratore, dietro il flag [motore_a].",
  ];
  apriFinestra("Limiti di questa versione", h("ul", { class: "du-elenco-p" }, voci.map((t) => h("li", {}, t))), [{ testo: "Chiudi", chiudi: true }]);
}

// ---------------------------------------------------------------- gesti della bozza (MAT §3.4-§3.7)

function contenitoriPer(g, k) {
  return Object.values(g.nodi).filter((v) => !v.noninc && !v.escluso && contenitore(v) && (!k || !vietatoSotto(k, v.k)));
}
function opzioniPadri(g, k, scelto) {
  return contenitoriPer(g, k).map((v) => h("option", { value: v.k, selected: v.k === scelto }, (v.cod || "(senza codice)") + " · " + (v.desc || "")));
}
function apriAggiungi(tipo, padre) {
  const g = grafoBozza();
  const sel = h("select", { id: "f-padre" }, h("option", { value: "" }, "— scegli il padre —"), opzioniPadri(g, "", padre || ""));
  const tipoSel = h("select", { id: "f-tipo" }, ["sottoassieme", "sciolto", "commerciale"].map((t) => h("option", { value: t, selected: t === tipo }, TIPI[t])));
  apriFinestra("Aggiungi un pezzo", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "Il pezzo nasce nella bozza, senza UUID finché non confermi. Il codice si può lasciare vuoto: la conferma lo chiederà (CODICE_DA_ASSEGNARE)."),
    campo("Tipo", tipoSel), campo("Codice", h("input", { id: "f-codice", maxlength: 40, autocomplete: "off" })), campo("Rev", h("input", { id: "f-rev", maxlength: 10, autocomplete: "off" })),
    campo("Descrizione", h("input", { id: "f-desc", maxlength: 200, autocomplete: "off" })), campo("Padre", sel),
    campo("Quantità sul legame", h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric" }), "Nessuna quantità implicita: va scritta.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Aggiungi", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
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
    fai("aggiunto " + (codice || "un pezzo senza codice"), [
      { azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodo, origine: "utente" },
      { azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: arco, origine: "utente" }]);
    S.nodo = k;
  } }]);
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
    for (const d of pb) azioni.push({ azione: "file_svincolato", chiave_bozza: v.k, prima: clona(d), dopo: { proposta_id: d.proposta_id, motivo: "nodo_tolto" }, origine: "utente" });
    azioni.push({ azione: "nodo_tolto", chiave_bozza: v.k, prima: clona(v.nodo), dopo: null, origine: "utente" });
    fai("tolto " + (v.cod || "un pezzo"), azioni);
    S.nodo = "";
  } }]);
}
function apriEscludi(v) {
  apriFinestra("Escludere " + (v.cod || "il componente") + "?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il componente esce dalla BOM alla conferma: si archivia, mai si cancella se ha storia. Nessuna cascata: i figli restano e vanno ricollocati."),
    campo("Motivo", area("f-motivo", "perché esce dalla BOM"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Escludi", danger: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const dec = { id: nuovoIdDecisione("es"), tipo: "escludi", componente_id: v.nodo.componente_id, motivo };
    fai("escluso " + v.cod, [
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: v.nodo.op }, dopo: { op: "esclude" }, origine: "utente" },
      { azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: dec, origine: "utente" }]);
  } }]);
}
function ripristina(v) {
  const dec = S.lavoro.decisioni.find((d) => d.tipo === "escludi" && d.componente_id === v.nodo.componente_id);
  const prima = arr(S.salvato && S.salvato.nodi).find((n) => n.chiave_bozza === v.k);
  const op = prima && prima.op !== "esclude" ? prima.op : "riusa";
  const azioni = [{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: "esclude" }, dopo: { op }, origine: "utente" }];
  if (dec) azioni.push({ azione: "decisione_tolta", chiave_bozza: v.k, prima: clona(dec), dopo: null, origine: "utente" });
  fai("ripristinato " + v.cod, azioni);
}
function apriCorreggi(v) {
  const n = v.nodo;
  const id = n.identita || {};
  const radice = n.componente_id && n.componente_id === S.prodotto;
  const importato = !!casellaDi(n) && !n.componente_id;
  const registrato = !!n.componente_id;
  apriFinestra("Correggi codice, rev e descrizione", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, importato ? "È un pezzo importato: la sua identità proposta resta com'è, e la correzione diventa una decisione motivata (l'identità decisa)." :
      registrato ? "È un componente registrato: un codice nuovo è una ricodifica, con il motivo (fino a 500 caratteri)." : "È un pezzo aggiunto a mano."),
    campo("Codice", h("input", { id: "f-codice", value: id.codice || "", maxlength: 60, disabled: !!radice, title: radice ? "è il prodotto della RFQ: il codice non cambia (DB7-3)" : null })),
    campo("Rev", h("input", { id: "f-rev", value: id.rev || "", maxlength: 10 })),
    campo("Descrizione", h("input", { id: "f-desc", value: n.descrizione || "", maxlength: 200 })),
    campo("Motivo", area("f-motivo", "obbligatorio per correggere un'identità importata o ricodificare", registrato ? 500 : 1000))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Salva nella bozza", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    const cambiaId = valore(f, "f-codice") !== (id.codice || "") || valore(f, "f-rev").toUpperCase() !== (id.rev || "");
    if (!cambiaId && valore(f, "f-desc") === (n.descrizione || "")) return "niente da cambiare";
    if (cambiaId && (importato || (registrato && valore(f, "f-codice") !== (id.codice || ""))) && !valore(f, "f-motivo")) return "scrivi il motivo";
    return "";
  }, fai: () => {
    const f = $("#du-finestra");
    const codice = valore(f, "f-codice"), rev = valore(f, "f-rev").toUpperCase(), desc = valore(f, "f-desc"), motivo = valore(f, "f-motivo");
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
      if (importato) {
        const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "correzione", identita: { emittente: nuova.emittente, codice: codice || undefined, rev: rev || undefined }, motivo };
        if (nuova.namespace) dec.identita.namespace = nuova.namespace;
        if (nuova.emittente.indexOf("partner:") === 0 && S.bozza.importazione) dec.importazione = S.bozza.importazione.impronta;
        azioni.push({ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" });
        prima.identita = clona(id);
        dopo.identita = Object.assign({}, nuova, { modo: "decisione", decisione: dec.id });
      } else if (registrato && cambiaCodice) {
        const dec = { id: nuovoIdDecisione("rc"), tipo: "ricodifica", componente_id: n.componente_id, codice_prima: id.codice || "", codice_dopo: codice, motivo, emittente_dopo: nuova.emittente };
        azioni.push({ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: dec, origine: "utente" });
        prima.identita = clona(id);
        prima.op = n.op;
        dopo.identita = Object.assign({}, nuova, { modo: "decisione", decisione: dec.id });
        dopo.op = "modifica";
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
    fai("corretto " + (codice || v.cod || "un pezzo"), azioni);
  } }]);
}
function apriTipo(v) {
  const sel = h("select", { id: "f-tipo" }, ["sottoassieme", "sciolto", "commerciale"].map((t) => h("option", { value: t, selected: t === v.tipo }, TIPI[t])));
  apriFinestra("Cambia tipo di " + (v.cod || "il pezzo"), h("div", { style: "display:grid;gap:8px" }, campo("Tipo", sel),
    h("p", { class: "du-nota" }, "Un commerciale non ha figli (E37) e il suo 2D non è più richiesto, ma solo dopo la conferma.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Cambia", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-tipo") === v.tipo ? "è già di questo tipo" : ""), fai: () => {
    const t = valore($("#du-finestra"), "f-tipo");
    fai("tipo di " + (v.cod || "un pezzo") + " → " + TIPI[t], [{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { tipo: v.tipo }, dopo: { tipo: t }, origine: "utente" }]);
  } }]);
}
function apriSposta(k, verso) {
  const g = grafoBozza();
  const v = g.nodi[k];
  const pa = padriA(g, k);
  const daSel = h("select", { id: "f-da" }, pa.map((a) => h("option", { value: a.p }, "da " + (g.nodi[a.p].cod || g.nodi[a.p].desc) + " (×" + a.qta + ")")));
  const aSel = h("select", { id: "f-a" }, h("option", { value: "" }, "— scegli il padre di arrivo —"), opzioniPadri(g, k, verso));
  apriFinestra("Sposta " + (v.cod || "il pezzo") + " sotto…", h("div", { style: "display:grid;gap:8px" },
    pa.length ? campo("Legame da spostare", daSel) : h("p", { class: "du-nota" }, "Il pezzo non ha padri: lo spostamento gli dà un padre."), campo("Nuovo padre", aSel),
    campo("Quantità sul legame nuovo", h("input", { id: "f-qta", type: "number", min: "1", max: "100000", step: "1", inputmode: "numeric" }), "Nessuna quantità implicita: va scritta.")),
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
      azioni.push({ azione: "decisione_aggiunta", chiave_bozza: k, prima: null, dopo: { id: nuovoIdDecisione("ta"), tipo: "togli_arco", padre_id: np.componente_id, figlio_id: nf.componente_id }, origine: "utente" });
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
// su tutti gli eventi del gesto. L'elenco è visibile e si può ridurre; il motivo è obbligatorio.
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
  const codice = h("select", { id: "f-motivo-codice" }, [["elemento_cad", "elemento CAD (cordone, annotazione)"], ["duplicato_cad", "duplicato CAD"], ["non_articolo", "non è un articolo"], ["altro", "altro"]]
    .map(([v, t]) => h("option", { value: v }, t)));
  apriFinestra("Non includere nella BOM", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "I nodi escono dalla bozza con una decisione ciascuno, che la conferma applica. Fino alla conferma il 2D resta richiesto e bloccante. I fatti dello STEP restano."),
    lista, campo("Motivo (codice)", codice), campo("Motivo", area("f-motivo", "per esempio: cordoni di saldatura, annotazioni CAD"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Non includere", danger: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    if (!f.querySelectorAll("#f-elenco input:checked").length) return "nessun nodo nell'elenco";
    return valore(f, "f-motivo") ? "" : "scrivi il motivo";
  }, fai: () => {
    const f = $("#du-finestra");
    const chiavi = [...f.querySelectorAll("#f-elenco input:checked")].map((x) => x.value);
    const motivo = valore(f, "f-motivo"), motivoCodice = valore(f, "f-motivo-codice");
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
        azioni.push({ azione: "file_svincolato", chiave_bozza: k, prima: clona(d), dopo: { proposta_id: d.proposta_id, motivo: "non_incluso", azione_id: azioneId }, origine: "utente" });
      }
      for (const a of S.lavoro.archi.filter((x) => (x.padre === k || x.figlio === k) && !tolti.has(x.padre + ">" + x.figlio))) {
        tolti.add(a.padre + ">" + a.figlio);
        azioni.push({ azione: "arco_tolto", chiave_bozza: a.figlio, prima: clona(a), dopo: { azione_id: azioneId }, origine: "utente" });
      }
      azioni.push({ azione: "nodo_tolto", chiave_bozza: k, prima: clona(n), dopo: { azione_id: azioneId }, origine: "utente" });
      const c = casellaDi(n);
      const voce = voceDiCasella(reg, c);
      const seg = segnaliDi(voce)[0];
      const dec = { id: nuovoIdDecisione("ni"), tipo: "non_includere", casella: clona(c), motivo_codice: motivoCodice, motivo, azione_id: azioneId };
      if (imp) dec.importazione = imp;
      if (seg) dec.segnale_visto = { codice: seg.codice, regola: seg.regola, versione_servizio: versioneServizio(reg) };
      decisioni.push({ azione: "decisione_aggiunta", chiave_bozza: k, prima: null, dopo: dec, origine: "utente" });
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
function includiDiNuovo(v) {
  const d = v.decisione;
  const reg = registroBozza();
  const voce = v.voce;
  if (!voce) return avvisa("errore", "La voce del registro non c'è: il nodo non si può includere di nuovo da qui (rileggi la bozza).");
  // la chiave di prima, se il giornale o la coda la dicono: lo stesso nodo, mai la stessa chiave per un nodo diverso
  let k = "";
  const tutti = arr(S.bozza.eventi).map(eventoAzione).concat(...S.coda.map((x) => x.inviare));
  for (const a of tutti) if (a.azione === "nodo_tolto" && a.prima && casellaDi(a.prima) && casellaDi(a.prima).rif === d.casella.rif) k = a.chiave_bozza || a.prima.chiave_bozza;
  if (!k || S.lavoro.nodi.some((n) => n.chiave_bozza === k)) k = nuovaChiaveNodo();
  const ip = voce.identita_proposta || {};
  const identita = voce.codice_proposto ? { emittente: ip.emittente || emittenteCliente(), codice: voce.codice_proposto, modo: "proposta" } : { emittente: ip.emittente || emittenteCliente(), modo: "da_decidere" };
  if (voce.codice_proposto && ip.namespace) identita.namespace = ip.namespace;
  if (voce.codice_proposto && voce.rev_proposta) identita.rev = voce.rev_proposta;
  const nodo = { chiave_bozza: k, op: "crea", tipo: (voce.tipo_proposto && voce.tipo_proposto.valore) || "sciolto", origine: "step", caselle: [clona(d.casella)], identita };
  const desc = voce.grezzi && voce.grezzi.descrizione;
  if (desc && String(desc).length <= 200) nodo.descrizione = desc;
  const azioni = [{ azione: "decisione_tolta", chiave_bozza: k, prima: clona(d), dopo: null, origine: "utente" }, { azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodo, origine: "utente" }];
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
  fai("incluso di nuovo " + (voce.codice_proposto || "un nodo"), azioni);
  S.nodo = k;
}
function preassocia(pid, k) {
  if (!pid || !k) return;
  fai("preassociato " + (fileDiProposta(pid) ? fileDiProposta(pid).nome_file : "un file"), [{ azione: "file_preassociato", chiave_bozza: k, prima: null, dopo: { proposta_id: pid, chiave_bozza: k }, origine: "utente" }]);
}
function togliPreassociazione(pid, k) {
  fai("tolta la preassociazione", [{ azione: "file_svincolato", chiave_bozza: k, prima: { proposta_id: pid, chiave_bozza: k }, dopo: { proposta_id: pid, motivo: "utente" }, origine: "utente" }]);
}
function lascia(pid) {
  fai("lasciato senza associazione", [{ azione: "file_lasciato", chiave_bozza: null, prima: null, dopo: { proposta_id: pid }, origine: "utente" }]);
}
function apriScollega(f, v) {
  apriFinestra("Scollegare " + f.nome_file + " dal componente?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Lo scollegamento si decide nella bozza e si applica con «Conferma BOM». Il file resta nella RFQ: non è uno scarto e non va in Altri file."),
    campo("Motivo (facoltativo)", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scollega alla conferma", pri: true, id: "f-ok", fai: () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const dec = { id: nuovoIdDecisione("sv"), tipo: "scollega_documento", proposta_id: f.proposta_id, componente_id: v.nodo.componente_id };
    if (motivo) dec.motivo = motivo;
    fai("da scollegare " + f.nome_file, [{ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: dec, origine: "utente" }]);
  } }]);
}
function apriAdotta(v, r, proiezione) {
  const voce = v.voce;
  const lettura = r.lettura || null;
  apriFinestra("Adotta come codice", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'identità decisa di " + (v.cod || "questo pezzo") + " diventa ", h("span", { class: "du-mono" }, proiezione), " (" + (lettura ? lettura.emittente : r.tipo === "manuale_legacy" ? "cliente, presunto" : "?") + "). L'identità proposta resta com'era."),
    campo("Motivo", area("f-motivo", "perché adotti questa lettura"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Adotta", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "alternativa", importazione: S.bozza.importazione ? S.bozza.importazione.impronta : undefined, motivo };
    if (r.tipo === "manuale_legacy") {
      dec.riferimento = "manuale_legacy";
      dec.identita = { emittente: emittenteCliente(), codice: proiezione };
      if (voce.codice_manuale_legacy.rev) dec.identita.rev = voce.codice_manuale_legacy.rev;
    } else {
      dec.chiave_lettura = lettura.chiave_lettura;
      dec.lettura = { emittente: lettura.emittente, hash_grammatica: lettura.hash_grammatica, namespace: lettura.namespace, letterale: lettura.letterale, base: lettura.base ? { originale: lettura.base.originale, normalizzata: lettura.base.normalizzata } : undefined,
        qualificatori: lettura.qualificatori, revisione: lettura.revisione === undefined ? null : lettura.revisione };
      dec.identita = { emittente: lettura.emittente, codice: proiezione };
      if (lettura.namespace) dec.identita.namespace = lettura.namespace;
    }
    const dopo = Object.assign({}, dec.identita, { modo: "decisione", decisione: dec.id });
    if (r.tipo === "manuale_legacy") dopo.emittente_presunto = true;
    fai("adottato " + proiezione, [{ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(v.nodo.identita) }, dopo: { identita: dopo }, origine: "utente" }]);
  } }]);
}
function apriFissaEmittente(v) {
  const reg = registroBozza();
  const partner = arr(reg && reg.regole && reg.regole.partner).map((p) => p.emittente).filter(Boolean);
  const sel = h("select", { id: "f-emittente" }, [emittenteCliente()].concat(partner).map((e) => h("option", { value: e }, e)));
  apriFinestra("Fissa l'emittente di " + v.cod, h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il componente è «cliente, presunto»: nessuno ha scelto l'emittente. Se in realtà è un codice del fornitore, fissalo con un motivo: la conferma lo certificherà."),
    campo("Emittente", sel), campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Fissa", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: () => {
    const f = $("#du-finestra");
    const em = valore(f, "f-emittente");
    const id = v.nodo.identita || {};
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "correzione", identita: { emittente: em, codice: id.codice }, motivo: valore(f, "f-motivo") };
    if (id.rev) dec.identita.rev = id.rev;
    if (em.indexOf("partner:") === 0 && S.bozza.importazione) dec.importazione = S.bozza.importazione.impronta;
    fai("emittente fissato per " + v.cod, [{ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: dec, origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(id) }, dopo: { identita: Object.assign({}, dec.identita, { modo: "decisione", decisione: dec.id }) }, origine: "utente" }]);
  } }]);
}
function apriRebase() {
  const b = S.bozza;
  const vc = b.versione_corrente || {};
  apriFinestra("Rebase della bozza", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La base della bozza diventa quella di adesso" + (vc.numero ? " (conferma n. " + vc.numero + ")" : "") + ". Sorgente e registro non cambiano; le decisioni restano."),
    arr(b.rif_scaduti).length ? h("p", { class: "du-nota" }, "Riferimenti scaduti da sistemare: " + arr(b.rif_scaduti).map((r) => r.chiave_bozza || r.decisione).join(", ")) : null,
    campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Fai il rebase", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const base = { conferma_id: vc.conferma_id === undefined ? null : vc.conferma_id, numero: vc.numero || 0, contatore: vc.contatore || 0, firma_struttura: vc.firma_struttura };
    const prima = { base: { conferma_id: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.conferma_id : null, numero: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.numero : 0,
      contatore: S.lavoro.versione_bom_base ? S.lavoro.versione_bom_base.contatore : 0, firma_struttura: S.lavoro.firma_base } };
    fai("rebase sulla base corrente", [{ azione: "rebase", chiave_bozza: null, prima, dopo: { base, rif_scaduti: arr(b.rif_scaduti) }, origine: "utente", motivo }], { rebase: { base, motivo } });
  } }]);
}

// ---------------------------------------------------------------- forme di R2 costruite dal server

function corpoForma(extra) {
  return Object.assign({ bozza_id: S.bozza ? S.bozza.bozza_id : null, versione_attesa: S.bozza ? S.bozza.versione : 0, prodotto_id: S.prodotto, vocabolario: 2 }, extra);
}
async function dopoFormaServer(r, testoOk) {
  if (r.esito === "ok") {
    avvisa("ok", testoOk + (r.dati && r.dati.riproposto ? " (riproposto: era già registrato)" : ""));
    S.ultimaForma = r.dati && r.dati.importazione ? r.dati.importazione : null;
    await caricaModello();
    await caricaBozza();
    // la proposta in vista nella Sorgente porta l'effetto sulla bozza di prima (S6 con «bozza»): si rilegge
    if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
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
    testo = "La proposta è cambiata mentre guardavi: rivedila, poi ripeti il gesto.";
    chiudi = true;
    const gc = r.dati && r.dati.gruppo_corrente;
    if (S.anteprimaAllegato) guarda(S.anteprimaAllegato, gc && gc.chiave ? gc.chiave : "", true);
  } else if (r.motivo === "gruppo_su_altra_occorrenza" && r.dati && r.dati.allegato_id) {
    testo = "Questo gruppo sta su un'altra copia dello stesso file: guarda la proposta su quella copia.";
    chiudi = true;
    guarda(r.dati.allegato_id, "", true);
  } else if (r.motivo === "stessa_sorgente") {
    testo = "È la struttura della bozza: per prendere la proposta di oggi usa «Aggiorna alla proposta di oggi».";
    chiudi = true;
  } else if (r.codice === "BOZZA_VERSIONE") {
    testo = "La bozza è cambiata altrove: rileggila e ripeti il gesto.";
  } else if (r.codice === "CALCOLO_OCCUPATO" || r.codice === "CALCOLO_SCADUTO" || r.codice === "NON_CONFIGURATO") {
    testo = "Il motore è occupato, riprova fra poco: niente è stato scritto.";
  }
  const conCodice = testo + (r.codice ? " (" + r.codice + (r.motivo ? ", " + r.motivo : "") + ")" : "");
  if (err && !chiudi) err.textContent = conCodice;
  avvisa(r.esito === "conflitto" ? "conflitto" : r.esito === "no" ? "no" : "errore", chiudi ? conCodice : testo);
  return chiudi;
}
function sorgenteDi(a, gr) {
  return { allegato_id: a.allegato_id, sha256: a.sha256, gruppo: gr.gruppo };
}
function apriUsaPerBom(a, gr) {
  const diverso = a.confronto !== "coincide";
  apriFinestra("Usa per la BOM di " + codiceProdotto(), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il server costruisce la versione 1 della bozza dalla proposta di questo STEP. La Sorgente mostra subito come i file della RFQ andrebbero associati: sono proposte, non decisioni."),
    h("p", { class: "du-nota" }, "Struttura: ", h("span", { class: "du-mono" }, breve(gr.gruppo, 60)), " · " + (gr.nodi || "?") + " nodi, " + (gr.segnalati || 0) + " segnalati."),
    diverso ? campo("Motivo (il codice letto dalla radice non è quello del prodotto)", area("f-motivo", "")) : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Crea la bozza", pri: true, id: "f-ok", spento: () => (diverso && !valore($("#du-finestra"), "f-motivo") ? "scrivi il motivo" : ""), fai: async () => {
    const motivo = diverso ? valore($("#du-finestra"), "f-motivo") : "";
    const crea = { partenza: "step", sorgente: sorgenteDi(a, gr), impronta_contenuto: gr.impronta_contenuto };
    if (motivo) crea.motivo = motivo;
    const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea });
    const chiudi = await dopoFormaServer(r, "Bozza creata dalla proposta dello STEP.");
    if (r.esito === "ok") {
      S.passo = "revisione";
      render();
    }
    return chiudi;
  } }]);
}
async function apriCambiaSorgente(a, gr) {
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "Il cambio dello STEP non parte: le modifiche della bozza non sono salvate.");
  }
  const e = a.effetto_sulla_bozza || {};
  const elenco = (t, x) => (arr(x).length ? h("li", {}, t + ": " + arr(x).length) : null);
  apriFinestra("Cambia lo STEP della BOM", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'effetto sulla bozza, calcolato dal server prima del gesto:"),
    h("ul", { class: "du-elenco-p", id: "f-effetto" }, elenco("nodi che restano", e.nodi_che_restano), elenco("nodi che escono", e.fuori_per_cambio_fonte), elenco("nodi che entrano", e.nodi_che_entrano),
      elenco("caselle riscritte su un'altra copia", e.caselle_riscritte), elenco("archi riscritti", e.archi_riscritti), elenco("identità aggiornate", e.identita_aggiornate),
      elenco("da rivedere", e.da_rivedere), elenco("decisioni che cadono", e.decisioni_cadute), elenco("preassociazioni nel vassoio", e.preassociazioni_nel_vassoio), elenco("voci nuove proposte (non entrano da sole)", e.nuovi_nodi_proposti)),
    campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Cambia lo STEP", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const r = await P.cambiaSorgente(corpoForma({ cambia_sorgente: { sorgente: sorgenteDi(a, gr), impronta_contenuto: gr.impronta_contenuto, motivo: valore($("#du-finestra"), "f-motivo") } }));
    return dopoFormaServer(r, "Fonte della bozza cambiata.");
  } }]);
}
async function apriDifferenze() {
  avvisa("ok", "Calcolo delle differenze…");
  try {
    const j = await P.leggiBozza(S.prodotto, { confronto: true });
    const c = j.confronto || {};
    if (c.errore) return avvisa("errore", "Il calcolo delle differenze non è riuscito: " + c.errore);
    const d = c.differenze || {};
    apriFinestra("Differenze con la proposta di oggi", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "Esito: " + (c.esito || "?") + "."),
      h("ul", { class: "du-elenco-p", id: "f-differenze" }, arr(d.nodi).map((x) => h("li", {}, (x.chiave_bozza || x.voce || "?") + ": " + (x.tipo || "") + (arr(x.campi).length ? " (" + arr(x.campi).join(", ") + ")" : ""))),
        arr(d.archi).map((x) => h("li", {}, "legame " + (x.padre || "") + " → " + (x.figlio || "") + ": " + (x.tipo || ""))))), [{ testo: "Chiudi", chiudi: true }]);
    avvisa("ok", "Differenze calcolate.");
  } catch (e) {
    avvisa("errore", "Le differenze non si leggono: " + testoErrore(e));
  }
}
async function apriRiallinea() {
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
  if (c.esito === "altra_occorrenza") return avvisa("no", "La BOM di lavoro sta su un'altra copia dello stesso file: per seguirla usa «Cambia lo STEP della BOM…».");
  if (c.esito === "gruppo_non_proposto") return avvisa("no", "Il Motore A non propone più questa struttura: la bozza resta valida.");
  const assenti = arr(c.differenze && c.differenze.nodi).filter((x) => x.tipo === "assente" && (x.chiave_bozza || x.decisione_id));
  const scelte = h("div", { class: "du-voci-sel", id: "f-scelte" }, assenti.length ? assenti.map((x) => h("div", { class: "du-gesto" }, h("span", { class: "du-mono" }, x.chiave_bozza || x.decisione_id),
    h("select", { "data-k": x.chiave_bozza || "", "data-d": x.decisione_id || "" }, h("option", { value: "" }, "— scegli —"), h("option", { value: "togli" }, "Togli"),
      x.decisione_id ? null : h("option", { value: "tieni_senza_casella" }, "Tieni come nodo a mano")))) : h("p", { class: "du-muto du-piccolo" }, "Nessuna casella sparita."));
  apriFinestra("Aggiorna la bozza alla proposta di oggi", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il server prende la proposta di oggi per la stessa sorgente: i nodi non toccati si aggiornano e si elencano, quelli decisi restano e vanno «da rivedere» se la loro proposta è cambiata. Niente cambia da solo senza una scelta."),
    scelte, campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Aggiorna", pri: true, id: "f-ok", spento: () => {
    const f = $("#du-finestra");
    if ([...f.querySelectorAll("#f-scelte select")].some((s) => !s.value)) return "scegli per ogni casella sparita";
    return valore(f, "f-motivo") ? "" : "scrivi il motivo";
  }, fai: async () => {
    const f = $("#du-finestra");
    const sc = [...f.querySelectorAll("#f-scelte select")].map((s) => (s.dataset.k ? { chiave_bozza: s.dataset.k, scelta: s.value } : { decisione_id: s.dataset.d, scelta: s.value }));
    const r = await P.riallineaProposta(corpoForma({ riallinea_proposta: { impronta_contenuto: c.impronta_contenuto_corrente, motivo: valore(f, "f-motivo"), scelte: sc } }));
    return dopoFormaServer(r, "Bozza aggiornata alla proposta di oggi.");
  } }]);
}
async function creaDa(partenza) {
  const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea: { partenza } });
  if (r.esito !== "ok") return esitoFormaKo(r);
  await dopoFormaServer(r, partenza === "conferma" ? "Bozza aperta dalla BOM confermata." : "Bozza aperta dalla BOM registrata.");
}
function apriConverti() {
  apriFinestra("Converti la bozza", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "La bozza di prima si chiude e se ne apre una nuova con la stessa struttura; i codici scritti restano come decisioni dell'ingegnere."),
    campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Converti", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const r = await P.convertiBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, converti_da: { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione, motivo: valore($("#du-finestra"), "f-motivo") } });
    return dopoFormaServer(r, "Bozza convertita.");
  } }]);
}
function apriAbbandona() {
  apriFinestra("Abbandona la bozza", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "La bozza viene chiusa e non si riapre: resta nel registro delle bozze. Le preassociazioni spariscono con lei; le associazioni registrate non cambiano."),
    campo("Motivo (facoltativo)", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Abbandona la bozza", danger: true, id: "f-ok", fai: async () => {
    clearTimeout(timerSalva);
    const motivo = valore($("#du-finestra"), "f-motivo");
    const corpo = { versione_attesa: S.bozza.versione };
    if (motivo) corpo.motivo = motivo;
    const r = await P.abbandonaBozza(S.bozza.bozza_id, corpo, S.prodotto);
    if (r.esito !== "ok") return esitoFormaKo(r);
    avvisa("ok", "Bozza abbandonata.");
    await caricaModello();
    await caricaBozza();
    render();
    return true;
  } }]);
}

// ---------------------------------------------------------------- i gesti dei file (S2, S3, S4, S7; EM §10.4-§10.6)

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
async function esitoGestoFile(r, f, testoOk) {
  if (r.esito === "ok") {
    const dati = r.dati || {};
    // l'esito del gesto va nell'avviso; la riga del file mostra solo lo stato che S1 dice adesso (la rimozione o la
    // conservazione possono andare avanti dopo la risposta: una frase di allora la contraddirebbe)
    const rim = arr(dati.rimozioni).map((x) => x.testo).filter(Boolean).join(" ");
    const t = rim || dati.testo || testoOk;
    avvisa("ok", t + (dati.riproposto ? " (riproposto: era già registrato)" : ""));
    await caricaSorgente();
    await caricaModello();
    render();
    return true;
  }
  if (r.esito === "da_verificare") {
    avvisa("da_verificare", "Esito da verificare: non so se il gesto è registrato. Verifico con la stessa chiave.");
    verificaConRiprova(r.chiave, async (j) => {
      if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
        avvisa("ok", "Verificato: il gesto su " + f.nome_file + " è registrato.");
        await caricaSorgente();
        render();
        return true;
      }
      return false;
    });
    renderStrisce();
    return true;
  }
  const err = $("#du-finestra-errore");
  let testo = r.testo || "Il server non ha accettato il gesto.";
  if (r.codice === "FILE_VERSIONE") {
    testo = "Lo stato del file è cambiato" + (r.dati && r.dati.stato ? " (ora " + r.dati.stato + ")" : "") + ": rileggo la Sorgente.";
    await caricaSorgente();
    render();
  } else if (r.codice === "ANTEPRIMA_SUPERATA") {
    testo = "Le dipendenze o il perimetro delle voci sono cambiati: il dialogo si aggiorna, rivedi e ripeti.";
  } else if (r.codice === "DIPENDENZE_ATTIVE") {
    const dip = arr(r.dati && r.dati.dipendenze);
    testo = "Il file ha dipendenze attive: " + dip.map((d) => (d.codice ? d.codice + ": " : "") + (d.testo || "")).join(" ");
  } else if (r.codice === "NON_CONFIGURATO") {
    testo = "La conservazione degli Altri file non è configurata su questo server: niente è cambiato.";
  }
  if (err) err.textContent = testo + (r.codice ? " (" + r.codice + ")" : "");
  avvisa(r.esito === "conflitto" ? "conflitto" : r.esito === "no" ? "no" : "errore", testo);
  return false;
}
async function apriScarta(f) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const unica = richiede(det, "conferma_unica_copia");
  const scol = richiede(det, "scollega");
  const blocchi = blocchiFile(det);
  const voci = arr(det.voci);
  apriFinestra("Scartare «" + f.nome_file + "»?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il file esce dai file attivi della RFQ e la sua copia nello staging viene eliminata."),
    h("p", {}, "Resta la traccia: nome, SHA, origine, chi l'ha scartato, quando e perché."),
    h("p", {}, "La mail originale, i documenti confermati, le copie sul NAS e la storia non si toccano."),
    f.canale === "posta" ? h("p", { class: "du-nota" }, "La mail originale resta: per riaverlo, lo salvi dalla mail e lo ricarichi.") : null,
    f.stato === "altri_file" ? h("p", { class: "du-nota" }, "È in Altri file: anche la copia di conservazione viene eliminata.") : null,
    voci.length ? h("p", { class: "du-nota" }, "È un archivio: lo seguono tutte le sue " + voci.length + " voci attive, ognuna con le sue dipendenze.") : null,
    elencoDipendenze(det),
    unica ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-unica" }), "Ho capito: è l'unica copia" + (unica > 1 ? " (" + unica + " file)" : "") + ". Scartandolo non ne resta nessuna: per riaverlo dovrò ricaricarlo.") : null,
    scol ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-scollega" }), "Scollega e scarta" + (scol > 1 ? " (" + scol + " file associati)" : "") + ": l'associazione al componente viene tolta.") : null,
    campo("Motivo", area("f-motivo", "obbligatorio"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scarta", danger: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    if (blocchi.length) return "il file ha dipendenze che bloccano: " + blocchi.map((b) => b.codice).join(", ");
    if (!valore(fin, "f-motivo")) return "scrivi il motivo";
    if (unica && !fin.querySelector("#f-unica").checked) return "serve la conferma dell'unica copia";
    if (scol && !fin.querySelector("#f-scollega").checked) return "serve «Scollega e scarta»";
    return "";
  }, fai: async () => {
    const fin = $("#du-finestra");
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0, motivo: valore(fin, "f-motivo"), impronta_dipendenze: det.impronta_dipendenze || "" };
    if (det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    if (scol) corpo.scollega = true;
    if (unica) corpo.conferma_unica_copia = true;
    const r = await P.scartaFile(f.allegato_id, corpo);
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") {
      chiudiFinestra();
      await caricaSorgente();
      const nuovo = fileDi(f.allegato_id);
      if (nuovo) apriScarta(nuovo);
      avvisa("conflitto", "Le dipendenze o il perimetro delle voci sono cambiati: rivedi il dialogo aggiornato e ripeti.");
      return true;
    }
    return esitoGestoFile(r, f, "File scartato.");
  } }]);
}
async function apriAltriFile(f) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const scol = richiede(det, "scollega");
  const blocchi = blocchiFile(det);
  apriFinestra("Mettere «" + f.nome_file + "» in Altri file?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Il file resta nella RFQ, nella cartella Altri file: si apre e si scarica quando serve, anche dopo un riavvio e dopo la pulizia dello staging."),
    h("p", {}, "Non entra nella proposta della BOM e non riceve associazioni finché non lo riattivi."),
    h("p", {}, "Non è la cartella ALTRO del NAS: sul NAS non si scrive niente."),
    elencoDipendenze(det),
    scol ? h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-scollega" }), "Scollega e metti in Altri file" + (scol > 1 ? " (" + scol + " file associati)" : "") + ".") : null,
    campo("Motivo (facoltativo)", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Metti in Altri file", pri: true, id: "f-ok", spento: () => {
    if (blocchi.length) return "il file ha dipendenze che bloccano: " + blocchi.map((b) => b.codice).join(", ");
    if (scol && !$("#du-finestra").querySelector("#f-scollega").checked) return "serve «Scollega e metti in Altri file»";
    return "";
  }, fai: async () => {
    const fin = $("#du-finestra");
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0, impronta_dipendenze: det.impronta_dipendenze || "" };
    const motivo = valore(fin, "f-motivo");
    if (motivo) corpo.motivo = motivo;
    if (det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    if (scol) corpo.scollega = true;
    const r = await P.altriFile(f.allegato_id, corpo);
    return esitoGestoFile(r, f, "File messo in Altri file: il contenuto è conservato.");
  } }]);
}
function apriRiattiva(f) {
  apriFinestra("Riattivare «" + f.nome_file + "»?", h("p", {}, "Riportare «" + f.nome_file + "» fra i file attivi? Il motore lo rilegge e può proporlo di nuovo. Non conferma nessun documento e non autorizza nessuno STEP."),
    [{ testo: "Annulla", chiudi: true }, { testo: "Riattiva", pri: true, id: "f-ok", fai: async () => {
      const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0 };
      if (f.n_voci !== undefined) {
        corpo.n_voci = f.n_voci;
        corpo.impronta_voci = f.impronta_voci;
      }
      const r = await P.riattivaFile(f.allegato_id, corpo);
      return esitoGestoFile(r, f, "File riattivato.");
    } }]);
}
function apriAssocia(a) {
  const f = fileDi(a.allegato_id);
  const prodotto = prodottoScelto();
  const diverso = f && f.proposta && f.proposta.codice && prodotto && f.proposta.codice !== prodotto.codice;
  apriFinestra("Associa lo STEP a " + codiceProdotto(), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "È un atto della fonte, motivato e tracciato: associa lo STEP al prodotto e non conferma niente. Dopo si accende «Conferma la fonte…»."),
    diverso ? h("p", { class: "du-nota" }, "Il codice della proposta (", h("span", { class: "du-mono" }, f.proposta.codice), ") non è quello del prodotto (", h("span", { class: "du-mono" }, prodotto.codice), "): serve un motivo.") : null,
    campo("Motivo" + (diverso ? "" : " (facoltativo)"), area("f-motivo", "", 500), "Fino a 500 caratteri.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Associa", pri: true, id: "f-ok", spento: () => {
    const m = valore($("#du-finestra"), "f-motivo");
    if (diverso && !m) return "scrivi il motivo";
    return m.length > 500 ? "il motivo sta in 500 caratteri" : "";
  }, fai: async () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const corpo = { prodotto_id: S.prodotto, codice_atteso: (f.proposta && f.proposta.codice) || "", componente_atteso: (f.proposta && f.proposta.componente_id) || null };
    if (motivo) corpo.motivo = motivo;
    const r = await P.associaStep(a.allegato_id, corpo);
    if (r.esito === "ok") {
      avvisa("ok", (r.dati && r.dati.testo) || "STEP associato al prodotto.");
      await caricaSorgente();
      await caricaModello();
      if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
      render();
      return true;
    }
    return esitoGestoFile(r, f, "");
  } }]);
}

// ---------------------------------------------------------------- gli atti esistenti (lista chiusa ridotta, EM §9.10)

async function atto(nome, campi, u, etichetta) {
  const d = $("#du-menu-rfq");
  if (d) d.open = false;
  avvisa("ok", etichetta + "…");
  const r = await P.esegui(nome, campi, u);
  avvisa(r.esito === "ok" ? "ok" : r.esito === "no" ? "no" : "errore", (r.esito === "no" ? "Niente è cambiato: " : "") + r.testo);
  if (r.esito === "ok") {
    await caricaModello();
    await Promise.all([caricaSorgente(), S.coda.length || S.invio ? null : caricaBozza()]);
    render();
  }
  return r;
}
function apriConfermaFonte() {
  const f = S.modello.fonte;
  apriFinestra("Conferma la fonte di " + codiceProdotto(), h("div", {},
    h("p", {}, "Conferma il documento STEP sorgente del prodotto (una voce). È l'eccezione dichiarata: lo STEP sorgente si conferma prima della BOM, perché serve all'autorizzazione. Non autorizza e non conferma la BOM.")),
  [{ testo: "Annulla", chiudi: true }, { testo: "Conferma la fonte", pri: true, id: "f-ok", fai: async () => {
    const r = await atto("confermaFonte", { voce: f.voce }, "", "Conferma della fonte");
    return r.esito === "ok";
  } }]);
}
async function apriAutorizza() {
  const f = S.modello.fonte;
  let a;
  try {
    a = await P.anteprimaAutorizzazione({ componente: S.prodotto, documento: f.documento_id || "", sorgente: f.sorgente || "" });
  } catch (e) {
    return avvisa("errore", "L'anteprima dell'autorizzazione non si legge: " + testoErrore(e));
  }
  if (a.errore) return avvisa("no", "Niente è cambiato: " + a.errore);
  const sorgenti = arr(a.sorgenti);
  apriFinestra("Autorizza lo STEP strutturale di " + codiceProdotto(), h("div", { style: "display:grid;gap:8px" },
    h("ul", { class: "du-elenco-p" }, arr(a.frasi).map((t) => h("li", {}, t))),
    arr(a.figli).length ? h("p", { class: "du-nota" }, "Figli diretti della sorgente: " + arr(a.figli).map((x) => x.codice || x.nome).join(", ")) : null,
    arr(a.avvisi).length ? h("p", { class: "du-nota" }, arr(a.avvisi).join(" ")) : null,
    arr(a.revoche).length ? h("p", { class: "du-nota" }, "Si revocano: " + arr(a.revoche).join(", ")) : null,
    sorgenti.length ? campo("Radice da autorizzare", h("select", { id: "f-sorgente" }, h("option", { value: "" }, "— scegli —"), sorgenti.map((x) => h("option", { value: x.chiave }, x.etichetta)))) : null,
    a.spento ? h("p", { class: "du-err" }, a.spento) : null,
    h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "f-presa" }), a.presa_d_atto || "Ho letto l'anteprima: l'autorizzazione rilegge lo STEP e non conferma la BOM.")),
  [{ testo: "Annulla", chiudi: true }, { testo: a.bottone || "Autorizza lo STEP strutturale", pri: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    if (!a.autorizzabile || !a.inviabile) return a.spento || a.gia || "il server dice che l'autorizzazione non si può inviare";
    if (sorgenti.length && !valore(fin, "f-sorgente")) return "scegli la radice";
    return fin.querySelector("#f-presa").checked ? "" : "serve la presa d'atto";
  }, fai: async () => {
    const fin = $("#du-finestra");
    const campi = { autorizza: "1", presa_d_atto: "1", firma: a.firma || "" };
    if (a.documento_id) campi.documento = a.documento_id;
    const sorg = sorgenti.length ? valore(fin, "f-sorgente") : a.sorgente || "";
    if (sorg) campi.sorgente = sorg;
    const r = await atto("autorizza", campi, S.prodotto, "Autorizzazione dello STEP");
    return r.esito === "ok";
  } }]);
}
function apriRevoca() {
  const f = S.modello.fonte;
  apriFinestra("Revoca l'autorizzazione dello STEP di " + codiceProdotto(), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Serve a cambiare lo STEP dopo l'autorizzazione: la bozza vedrà la base superata."), campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Revoca", danger: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const r = await atto("revoca", { sha: f.sha256 || "", motivo: valore($("#du-finestra"), "f-motivo") }, S.prodotto, "Revoca dell'autorizzazione");
    return r.esito === "ok";
  } }]);
}
async function riallineaDocumenti() {
  const r = await P.riallineaDocumenti();
  avvisa(r.esito === "ok" ? "ok" : "errore", r.esito === "ok" ? "Documenti riallineati (F8: " + ((r.dati && r.dati.f8_stato) || "?") + ")." : r.testo);
  await caricaModello();
  render();
}

// ---------------------------------------------------------------- i tentativi in sospeso (R6)

async function verificaSospeso(s) {
  try {
    const j = await P.esitoIntento(s.chiave);
    if (j.stato === "registrato" || j.stato === "registrato_senza_esito") {
      avvisa("ok", "Verificato: «" + (s.etichetta || s.nome) + "» è registrato" + (j.stato === "registrato_senza_esito" ? " (l'esito di allora non è più conservato)" : "") + ".");
      await leggiTutto(false);
    } else if (j.stato === "non_registrato") {
      avvisa("no", "«" + (s.etichetta || s.nome) + "» non è ancora registrato. Puoi rifarlo con lo stesso corpo e la stessa chiave: se la richiesta di prima arriva, riceverai il suo esito. Per un corpo diverso, abbandona prima il tentativo.");
    } else if (j.stato === "in_corso") {
      avvisa("ok", "Un gesto per questa RFQ è ancora in corso: verifico fra poco.");
      setTimeout(() => verificaSospeso(s), 3000);
    }
  } catch (e) {
    avvisa("errore", "Verifica non riuscita: " + testoErrore(e) + ". La chiave resta in sospeso.");
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
  apriFinestra("Abbandonare il tentativo?", h("p", {}, "La richiesta di prima («" + (s.etichetta || s.nome) + "») potrebbe ancora arrivare: se arriva, la vedrai registrata. Abbandonando la chiave, la pagina permette un gesto nuovo con una chiave nuova."),
    [{ testo: "Annulla", chiudi: true }, { testo: "Abbandona il tentativo", danger: true, id: "f-ok", fai: () => {
      P.abbandonaSospeso(s.risorsa);
      avvisa("ok", "Tentativo abbandonato: la richiesta di prima, se arriva, sarà registrata.");
      render();
    } }]);
}
async function verificaSospesi() {
  for (const s of P.sospesi()) await verificaSospeso(s);
}
// verificaConRiprova: R6 dopo 1, 2, 4 e 8 secondi, poi su clic (MAT §5.5 n. 2); fatto(j) torna true quando l'esito è
// chiuso.
function verificaConRiprova(chiave, fatto) {
  const attese = [1000, 2000, 4000, 8000];
  let i = 0;
  const giro = async () => {
    let j = null;
    try {
      j = await P.esitoIntento(chiave);
    } catch (e) {
      j = null;
    }
    if (j && (await fatto(j))) return;
    if (j && j.stato === "in_corso") {
      setTimeout(giro, 3000);
      return;
    }
    if (j && j.stato === "non_registrato") return;
    if (i < attese.length) setTimeout(giro, attese[i++]);
  };
  setTimeout(giro, attese[i++]);
}

// ---------------------------------------------------------------- la barra fissa

function renderBarra() {
  const box = $("#du-barra");
  if (!box) return;
  if (S.passo !== "revisione") {
    box.replaceChildren(h("div", { class: "du-salva du-salva--sorg" }, h("span", { class: "du-salva-conti" }, "Sorgente: la BOM non si modifica qui."),
      btn("Vai alla Revisione", () => vaiPasso("revisione"), { cls: "du-btn--sm", id: "du-vai-revisione", no: S.prodotto ? "" : "nessun prodotto valutato" })));
    return;
  }
  if (!bozzaLavorabile()) {
    box.replaceChildren();
    return;
  }
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
  if (st.stato === "non_arrivato" && S.tentativi >= 3) extra.push(btn("Riprova ora", () => {
    S.tentativi = 0;
    invia();
  }, { cls: "du-btn--sm", id: "du-riprova-salvataggio", perche: false }));
  if (st.stato === "conflitto" || st.stato === "rifiutata") {
    extra.push(h("span", { class: "du-perche", id: "du-conflitto-testo" }, (st.testo || "") + (S.coda.length ? " Modifiche non salvate: " + S.coda.map((v) => v.etichetta).join("; ") + "." : "")));
    extra.push(btn("Rileggi la bozza salvata", async () => {
      await caricaBozza();
      render();
    }, { cls: "du-btn--sm du-btn--pri", id: "du-rileggi-bozza", perche: false }));
  }
  const r = S.riepilogo;
  const conti = h("span", { class: "du-salva-conti", id: "du-conti" }, S.riepilogoStato === "aggiornato" && r ?
    [h("span", {}, "blocchi ", h("b", {}, String(arr(r.blocchi).length))), h("span", {}, "avvisi ", h("b", {}, String(arr(r.avvisi).length))),
      h("span", {}, "documenti irrisolti ", h("b", {}, String(arr(r.documenti_preassociati).filter((d) => d.esito && d.esito !== "applicabile").length)))] :
    h("span", {}, S.riepilogoStato === "errore" ? "riepilogo non letto: " + (S.riepilogoErrore || "") : "conti da aggiornare"));
  const noCta = perchéNoConferma();
  box.replaceChildren(h("div", { class: "du-salva", id: "du-salva" }, statoEl, extra, conti,
    h("span", { class: "du-azioni" },
      btn("Annulla", annulla, { cls: "du-btn--sm", id: "du-annulla", no: !S.pila.length ? "niente da annullare" : perchéNoModifica(), perche: false }),
      btn("Ripeti", ripeti, { cls: "du-btn--sm", id: "du-ripeti", no: !S.rifai.length ? "niente da ripetere" : perchéNoModifica(), perche: false }),
      btn("Abbandona la bozza…", apriAbbandona, { cls: "du-btn--sm du-btn--ghost", id: "du-abbandona", perche: false })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma BOM", apriWizard, { cls: "du-btn--pri", id: "du-cta-bom", no: noCta })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma fascicolo", null, { cls: "du-btn--sm", id: "du-cta-fascicolo", no: "Arriva in V3: i documenti si confermano con il lotto, dopo una BOM confermata e verificata" }))));
}
function perchéNoConferma() {
  if (S.vista !== "org") return "il riepilogo della BOM si apre dall'Organigramma";
  const no = perchéNoModifica({ riprova: true });
  if (no) return no;
  if (S.bozza.base && S.bozza.base.stato === "superata") return "la base è superata: fai prima il rebase";
  return "";
}

// ---------------------------------------------------------------- il wizard «Rivedi e conferma BOM» (MAT §3.8)

async function apriWizard() {
  const no = perchéNoConferma();
  if (no) return avvisa("errore", "Il riepilogo non si apre: " + no + ".");
  S.wizard = { fase: 1, stato: "lettura", gruppo: "tutti", riepilogo: null, errore: "", presaMotore: false, esito: null, dettagli: new Set() };
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
  S.wizard = null;
  $("#du-wizard").hidden = true;
  $("#du-wizard").replaceChildren();
  $("#du-velo").hidden = true;
  render();
}
async function rileggiRiepilogo() {
  const w = S.wizard;
  w.stato = "lettura";
  renderWizard();
  try {
    w.riepilogo = await P.riepilogoBom(corpoBozza(), { motore: true });
    w.versione = S.bozza.versione;
    w.stato = "pronto";
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
function renderWizard() {
  const box = $("#du-wizard");
  const w = S.wizard;
  if (!box || !w) return;
  const testa = h("div", { class: "du-wz-t" }, h("h2", { id: "du-wizard-titolo" }, "Rivedi e conferma BOM · " + codiceProdotto()),
    h("span", { class: "du-muto du-piccolo" }, "bozza versione " + (S.bozza ? S.bozza.versione : "?")), h("button", { type: "button", class: "du-btn du-btn--sm du-btn--ghost", id: "du-wz-chiudi", onclick: chiudiWizard,
      disabled: w.stato === "conferma" }, "Chiudi"));
  const passi = h("div", { class: "du-wz-passi" },
    h("button", { type: "button", class: "du-wz-passo", "aria-current": w.fase === 1 ? "step" : null, id: "du-wz-fase1", onclick: () => {
      w.fase = 1;
      renderWizard();
    } }, "1 · Identità dei componenti"),
    h("button", { type: "button", class: "du-wz-passo", "aria-current": w.fase === 2 ? "step" : null, id: "du-wz-fase2", onclick: () => {
      w.fase = 2;
      renderWizard();
    } }, "2 · Gerarchie, quantità e tipi"));
  const corpo = h("div", { class: "du-wz-c", id: "du-wz-corpo" });
  if (w.esito) corpo.append(esitoWizard(w));
  if (w.stato === "lettura") corpo.append(h("p", {}, "Calcolo del riepilogo…"));
  else if (w.stato === "errore") corpo.append(h("div", { class: "du-striscia du-striscia--ko", role: "alert" }, w.errore));
  else if (w.riepilogo) corpo.append(w.fase === 1 ? faseIdentita(w) : faseGerarchie(w));
  const piede = h("div", { class: "du-wz-p" });
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
}
function perchéNoConfermaWizard(w) {
  if (w.stato === "conferma") return "conferma in corso…";
  if (w.esito && w.esito.tipo === "ok") return "la BOM è confermata";
  if (w.esito && w.esito.tipo === "verifica") return "esito da verificare: aspetta la verifica";
  if (w.esito && w.esito.tipo === "controllo_finale") return "la conferma non è registrata per un errore interno: segnalalo, non riprovare";
  if (!w.riepilogo || w.stato !== "pronto") return "il riepilogo non è pronto";
  if (S.coda.length || S.invio) return "ci sono modifiche non salvate: il riepilogo va riletto";
  if (w.versione !== S.bozza.versione) return "la bozza è cambiata: rileggi il riepilogo";
  if (!w.riepilogo.confermabile) return "la BOM non è confermabile: " + arr(w.riepilogo.blocchi).length + " blocchi";
  if (w.riepilogo.avvisi_motore_calcolati === false && nonInclusiNellaBozza() && !w.presaMotore) return "gli avvisi del motore non sono calcolati: spunta la presa d'atto";
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
    } }, "Non inclusi (" + arr(r.non_inclusi).length + ")")));
  if (w.gruppo === "noninc") {
    box.append(tabellaNonInclusi(r));
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
        riga("Riferimenti", String(arr(x.riferimenti).length)), riga("Motivo", d.motivo || ""), riga("Modo", d.modo || ""), d.lettura ? riga("Lettura di allora", d.lettura.letterale || "") : null))));
    }
    return tr;
  });
  box.append(h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-identita" }, h("thead", {}, h("tr", {}, h("th", {}, "Codice"), h("th", {}, "Tipo"), h("th", {}, "Azione / esito"), h("th", {}, "Emittente"), h("th", {}, ""))),
    h("tbody", {}, righe.length ? righe : h("tr", {}, h("td", { colspan: "5" }, "Nessuna voce in questo gruppo."))))));
  return box;
}
function tabellaNonInclusi(r) {
  const nonInc = arr(r.non_inclusi);
  return h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-noninclusi" },
    h("thead", {}, h("tr", {}, h("th", {}, "Nodo"), h("th", {}, "Motivo"), h("th", {}, "Effetti dopo la conferma"))),
    h("tbody", {}, nonInc.length ? nonInc.map((x) => {
      const e = x.effetti || {};
      return h("tr", {}, h("td", {}, h("span", { class: "du-mono" }, x.nome_grezzo || (x.casella && x.casella.rif) || x.id)),
        h("td", {}, (x.motivo_codice || "") + (x.motivo ? " · " + x.motivo : "")),
        h("td", {}, arr(e.righe_da_chiudere).length + " righe e " + arr(e.archi_da_chiudere).length + " legami da chiudere; 2D previsti: " + arr(e.fabbisogni_tolti_dopo_conferma).length + " → 0 dopo la conferma" +
          (arr(e.prodotti_con_lo_stesso_step).length ? "; stesso STEP anche in: " + arr(e.prodotti_con_lo_stesso_step).map((p) => p.prodotto_id.slice(0, 8) + " (" + p.come + ")").join(", ") : "")));
    }) : h("tr", {}, h("td", { colspan: "3" }, "Nessun nodo non incluso.")))));
}
function faseGerarchie(w) {
  const r = w.riepilogo;
  const box = h("div", { style: "display:grid;gap:12px" });
  const blocchi = arr(r.blocchi);
  box.append(h("div", { class: "du-esito " + (r.confermabile ? "du-esito--ok" : "du-esito--ko"), id: "du-wz-confermabile" },
    h("b", {}, r.confermabile ? "La BOM è confermabile." : "La BOM non è confermabile: " + blocchi.length + " blocchi."),
    blocchi.length ? h("ul", { class: "du-elenco-p", id: "du-wz-blocchi" }, blocchi.map((b) => h("li", { "data-codice": b.codice },
      h("b", {}, b.codice), " ", b.testo || "", arr(b.rimedi).length ? h("span", { class: "du-muto" }, " Rimedi: " + arr(b.rimedi).join("; ")) : null,
      b.chiave_bozza ? h("span", { class: "du-azioni" }, btn("Mostra nel grafo", () => mostraNelGrafo(b.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false }),
        b.codice === "D1_ULTIMO_FIGLIO" ? btn("Diventa particolare", () => decisioneWizard("diventa particolare", { id: nuovoIdDecisione("d1"), tipo: "d1_a_particolare", chiave_bozza: b.chiave_bozza }), { cls: "du-btn--sm", perche: false }) : null) : null))) : null));
  const avvisi = arr(r.avvisi);
  if (avvisi.length) box.append(h("div", { class: "du-esito du-esito--ver", id: "du-wz-avvisi" }, h("b", {}, "Avvisi"), h("ul", { class: "du-elenco-p" }, avvisi.map((a) => h("li", { "data-codice": a.codice }, h("b", {}, a.codice), " ", a.testo || "")))));
  // gli avvisi del motore, calcolati adesso e non firmati (EM §8.2; MAT WZ-06)
  const mot = h("div", { class: "du-esito du-esito--ver", id: "du-wz-motore" });
  if (r.avvisi_motore_calcolati === false) {
    mot.append(h("b", {}, "Avvisi del motore non calcolati" + (r.avvisi_motore_errore ? " (" + (r.avvisi_motore_errore.codice || "") + ": " + (r.avvisi_motore_errore.testo || "") + ")" : "") + "."),
      btn("Riprova", rileggiRiepilogo, { cls: "du-btn--sm", id: "du-wz-riprova-motore", perche: false }));
    if (nonInclusiNellaBozza()) mot.append(h("label", { class: "du-spunta" }, h("input", { type: "checkbox", id: "du-wz-presa", checked: w.presaMotore, onchange: (e) => {
      w.presaMotore = e.target.checked;
      renderWizard();
    } }), "Confermo senza gli avvisi del motore (la bozza ha nodi non inclusi: lo STEP potrebbe essere usato da un altro prodotto)."));
  } else mot.append(h("b", {}, "Avvisi del motore, calcolati adesso e non firmati: "), arr(r.avvisi_motore).length ? arr(r.avvisi_motore).map((a) => a.codice + (a.testo ? " — " + a.testo : "")).join("; ") : "nessuno");
  box.append(mot);
  // mono_pezzo: casella mai spuntata, motivo obbligatorio (MAT BZ-14)
  if (avvisi.some((a) => a.codice === "PRODOTTO_SENZA_FIGLI") && !S.salvato.decisioni.some((d) => d.tipo === "mono_pezzo")) {
    box.append(h("div", { class: "du-esito du-esito--ver" }, h("b", {}, "Il prodotto non ha figli."), h("span", {}, "Se è un pezzo unico, dichiaralo con il motivo: vale solo con lo STEP autorizzato del prodotto."),
      btn("Dichiara mono_pezzo…", apriMonoPezzo, { cls: "du-btn--sm", perche: false })));
  }
  const archi = arr(r.archi);
  box.append(h("h3", {}, "Legami (" + archi.length + ")"), h("div", { class: "du-tab-guscio" }, h("table", { class: "du-tab", id: "du-wz-archi" },
    h("thead", {}, h("tr", {}, h("th", {}, "Padre → figlio"), h("th", {}, "Qtà STEP"), h("th", {}, "Qtà voluta"), h("th", {}, "Azione"), h("th", {}, "Origine"))),
    h("tbody", {}, archi.map((a) => h("tr", {}, h("td", {}, h("span", { class: "du-mono" }, ((a.padre && a.padre.codice) || "?") + " → " + ((a.figlio && a.figlio.codice) || "?"))),
      h("td", {}, a.qta_step === null || a.qta_step === undefined ? "—" : String(a.qta_step)), h("td", {}, String(a.qta_voluta === undefined ? "" : a.qta_voluta)), h("td", {}, a.azione || ""), h("td", {}, a.origine || "")))))));
  box.append(h("h3", {}, "Non inclusi"), tabellaNonInclusi(r));
  const pre = arr(r.documenti_preassociati);
  if (pre.length) box.append(h("h3", {}, "Preassociazioni"), h("ul", { class: "du-elenco-p" }, pre.map((d) => h("li", {}, (d.nome_file || d.proposta_id) + " → " + (d.chiave_bozza || "") + ": " + (d.esito || "")))));
  const svi = arr(r.svincoli);
  if (svi.length) box.append(h("h3", {}, "Svincoli"), h("ul", { class: "du-elenco-p" }, svi.map((d) => h("li", {}, (d.proposta_id || "") + ": " + (d.esito || "")))));
  const rel = arr(r.relazioni);
  if (rel.length) box.append(h("h3", {}, "Relazioni registrate come non ratificate"), h("ul", { class: "du-elenco-p" }, rel.map((x) => h("li", {}, x.tipo + ": " + (x.lettura_sorgente || "?") + " ↔ " + arr(x.letture_bersaglio).join(", ") + " (" + (x.ratifica || "non_ratificata") + ")"))));
  const cp = r.criterio_previsto || {};
  box.append(h("p", { class: "du-nota", id: "du-wz-criterio" }, "Criterio che la conferma registrerà: " + (cp.criterio || "?") + (cp.motivo ? " — " + cp.motivo : "")),
    h("p", { class: "du-nota" }, "Firma del riepilogo: ", h("span", { class: "du-mono", id: "du-wz-firma" }, String(r.firma_anteprima).slice(0, 12) + "…")));
  return box;
}
async function decisioneWizard(etichetta, dec) {
  if (!fai(etichetta, [{ azione: "decisione_aggiunta", chiave_bozza: dec.chiave_bozza || null, prima: null, dopo: dec, origine: "utente" }])) return;
  const ok = await salvaOra();
  if (!ok) return;
  await rileggiRiepilogo();
}
function apriMonoPezzo() {
  apriFinestra("Dichiara mono_pezzo", campo("Motivo", area("f-motivo", "")), [{ testo: "Annulla", chiudi: true }, { testo: "Dichiara", pri: true, id: "f-ok",
    spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
      await decisioneWizard("dichiarato mono_pezzo", { id: nuovoIdDecisione("mp"), tipo: "mono_pezzo", motivo: valore($("#du-finestra"), "f-motivo") });
    } }]);
}
async function confermaWizard() {
  const w = S.wizard;
  if (perchéNoConfermaWizard(w)) return;
  w.stato = "conferma";
  w.corpo = corpoBozza();
  w.firma = w.riepilogo.firma_anteprima;
  w.esito = { tipo: "in_corso" };
  renderWizard();
  renderBarra();
  const r = await P.confermaBom(w.firma, w.corpo);
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
    verificaConRiprova(r.chiave, async (j) => {
      if (!S.wizard) return true;
      if (j.stato === "registrato") {
        S.wizard.esito = { tipo: "ok", dati: j.esito || {}, verificato: true };
        renderWizard();
        await dopoConferma();
        return true;
      }
      if (j.stato === "registrato_senza_esito") {
        S.wizard.esito = { tipo: "ok", dati: j, verificato: true };
        renderWizard();
        await dopoConferma();
        return true;
      }
      if (j.stato === "non_registrato") {
        S.wizard.esito = { tipo: "non_registrata", chiave: r.chiave };
        renderWizard();
        return true;
      }
      if (j.stato === "in_corso") {
        S.wizard.esito = { tipo: "verifica", chiave: r.chiave, testo: "Una conferma per questa RFQ è ancora in corso: verifico fra poco." };
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
  w.esito = { tipo: r.esito, codice: r.codice, motivo: r.motivo, testo: r.testo, dati: r.dati };
  if (r.codice === "ANTEPRIMA_SUPERATA" && r.dati && r.dati.riepilogo) {
    w.riepilogo = r.dati.riepilogo;
    w.versione = S.bozza.versione;
  }
  renderWizard();
  if (r.codice === "BOZZA_VERSIONE") {
    await caricaBozza();
    render();
  }
}
function esitoWizard(w) {
  const e = w.esito;
  if (e.tipo === "in_corso") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "in_corso" }, "Conferma in corso…");
  if (e.tipo === "ok") {
    const d = e.dati || {};
    return h("div", { class: "du-esito du-esito--ok", id: "du-wz-esito", "data-esito": "ok" }, h("b", {}, "Conferma n. " + (d.numero_conferma || "?") + " di " + codiceProdotto() + (e.verificato ? " (verificata con la stessa chiave)" : "") + "."),
      h("ul", { class: "du-elenco-p" }, h("li", {}, "Identità accettate: " + arr(d.identita_accettate).length + ", corrette: " + arr(d.identita_corrette).length),
        h("li", {}, "Non inclusi applicati: " + arr(d.non_inclusi_applicati).length), h("li", {}, "Associazioni applicate: " + arr(d.associazioni_applicate).length + ", svincoli: " + arr(d.svincoli_applicati).length),
        h("li", {}, "Relazioni registrate come non ratificate: " + arr(d.relazioni_non_ratificate).length)),
      h("p", {}, pill("ok", "v" + (d.numero_conferma || "?") + " confermata"), " ", pill("neu", "Verificata: non ancora calcolata (arriva in V2)")),
      e.rilettura === "fallita" ? h("p", { class: "du-err" }, "La conferma è registrata; la pagina non si è riletta: ↻ Aggiorna.") : null);
  }
  if (e.tipo === "verifica") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "da_verificare" }, h("b", {}, "Esito della conferma da verificare."), h("span", {}, e.testo),
    btn("Verifica ora", () => verificaConfermaOra(e.chiave), { cls: "du-btn--sm", id: "du-wz-verifica", perche: false }));
  if (e.tipo === "non_registrata") return h("div", { class: "du-esito du-esito--ver", id: "du-wz-esito", "data-esito": "non_registrata" },
    h("b", {}, "La conferma non è ancora registrata."), h("span", {}, "Puoi riprovare con la stessa chiave e lo stesso corpo: se la richiesta di prima arriva, riceverai il suo esito."),
    btn("Riprova la conferma", riprovaConferma, { cls: "du-btn--sm du-btn--pri", id: "du-wz-riprova", perche: false }));
  if (e.tipo === "controllo_finale") return h("div", { class: "du-esito du-esito--ko", id: "du-wz-esito", "data-esito": "controllo_finale" },
    h("b", {}, "La conferma non è registrata per un errore interno del controllo finale: non riprovare, segnalalo."), h("span", {}, e.testo || ""));
  const testi = {
    BASE_SUPERATA: arr(e.dati && e.dati.motivi).includes("codice_occupato") ? "Un'altra pagina ha scritto lo stesso codice nel frattempo: rileggi il riepilogo." : "La distinta è cambiata dopo la tua lettura: la bozza resta, fai il rebase e riconferma.",
    ANTEPRIMA_SUPERATA: "Il riepilogo è cambiato: rileggilo e conferma di nuovo (firma nuova, chiave nuova).",
    BOZZA_VERSIONE: "La bozza salvata non è quella che stai confermando: la rileggo.",
    COPIA_IN_CORSO: "Una copia sul NAS è in corso: riprova fra poco (stessa chiave).",
    CHIAVE_RIUSATA: "Questo tentativo ha già un esito diverso: verifica lo storico e rileggi.",
    BLOCCHI: "Niente è registrato, la bozza resta: risolvi i blocchi.",
    CONGELATA: "La BOM è congelata: niente è registrato.",
    PONTE_TENTATIVO_IN_SOSPESO: "La conferma di prima non ha un esito certo e questo corpo è diverso: niente è partito. Verifica il tentativo, o abbandonalo dalla striscia in alto, poi conferma di nuovo.",
  };
  const blocchi = arr(e.dati && e.dati.blocchi);
  return h("div", { class: "du-esito du-esito--ko", id: "du-wz-esito", "data-esito": e.tipo, "data-codice": e.codice || "" }, h("b", {}, (testi[e.codice] || e.testo || "La conferma non è riuscita.") + (e.codice ? " (" + e.codice + ")" : "")),
    blocchi.length ? h("ul", { class: "du-elenco-p" }, blocchi.map((b) => h("li", {}, b.codice + ": " + (b.testo || "")))) : null,
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
  const r = await P.confermaBom(w.firma, w.corpo);
  await esitoConferma(r);
}
async function dopoConferma() {
  const w = S.wizard;
  try {
    await caricaModello();
    await Promise.all([caricaBozza(), caricaStorico()]);
  } catch (e) {
    if (w && w.esito) w.esito.rilettura = "fallita";
  }
  avvisa("ok", "BOM confermata.");
  renderWizard();
  renderNav();
  renderStrisce();
  renderBarra();
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
  window.addEventListener("beforeunload", (e) => {
    if (S.coda.length || S.invio) {
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
  try {
    P.configura({ thread: S.thread, utente: S.utente });
  } catch (e) {
    avvisa("errore", "La pagina non si configura: " + testoErrore(e));
    return;
  }
  leggiIndirizzo();
  installaAscoltatori();
  renderTestaAzioni();
  await leggiTutto(true);
  await verificaSospesi();
}
avvio();
