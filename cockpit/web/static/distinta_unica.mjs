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

// I motivi delle azioni che V1 non ha (MAT §6.1): restano visibili e spente, con il motivo scritto accanto (D23:294).
const NON_V1 = {
  documento: "I documenti si confermano solo con «Rivedi e conferma fascicolo» (V3)",
  r10: "La correzione di un file arriva con R10 (V4); in V1 vale solo per lo STEP del prodotto",
  deroga: "Le deroghe non sono in questa pagina",
  sostituisci: "La sostituzione dei documenti non è in V1",
  revisione: "Revisione e congelamento non sono in questa pagina",
  relazioni: "Le relazioni restano proposte: la loro ratifica arriva in una tappa dopo",
  candidata: "Il rifiuto motivato di una candidata arriva in V5: intanto si sceglie un altro STEP",
  fascicolo: "Arriva in V3: i documenti si confermano con il lotto, dopo una BOM confermata e verificata",
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
  filtro: "tutti", // il filtro dell'Elenco (MAT EL-01)
  cerca: "", // la ricerca dell'Elenco (MAT EL-01)
  confronto: false, // «Confronto con la proposta» sulle schede (MAT BZ-25)
  daParte: null, // le modifiche non salvate tenute da parte dopo un conflitto o un rifiuto: {voci, perche, testo} (MAT §5.1)
  daRileggere: "", // un gesto registrato dopo il quale la pagina non si è riletta (MAT §1.3, §5.2: «↻ Rileggi»)
  linguettaNodo: "dettaglio", // la linguetta del pannello del nodo: "dettaglio" | "documenti" (MAT PN-02)
  verifiche: new Set(), // le chiavi che la pagina sta verificando da sola con R6 (MAT §5.5 n. 2)
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
  await Promise.all([caricaSorgente(), caricaStorico(), caricaBozza()]);
  if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
  const non = letturaNonRiuscita();
  if (!non) S.daRileggere = "";
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
// l'abbandono esplicito della chiave (MAT §5.2, «verificato: non registrata»; §5.5 n. 7). Un tentativo «certo» (409
// COPIA_IN_CORSO: niente registrato, niente in volo) non ferma le modifiche: il ponte ammette un corpo diverso con una
// chiave nuova, e «Riprova» con lo stesso corpo riusa la chiave (MAT §5.2, «copia in corso»).
function confermaInSospeso() {
  return P.sospesi().find((s) => s.nome === "confermaBom" && s.risorsa === "bozza:" + S.prodotto) || null;
}
const D18 = "il modello della pagina non si legge: niente tela, bozza né wizard (D-18)";
const BOZZA_NON_LETTA = "la bozza non si legge: ↻ Aggiorna";
const CONGELATA = "la BOM è congelata: revisione e congelamento non sono in questa pagina";
// perchéNoModifica: perché un gesto sulla bozza non parte, o "". Con o.riprova una conferma in sospeso già verificata
// «non registrata» non ferma il wizard: è la strada di «Riprova la conferma», lo stesso corpo con la stessa chiave, anche
// dopo una ricarica della pagina (MAT §5.2, «verificato: non registrata»; §5.5 n. 4). Le modifiche restano ferme.
function perchéNoModifica(o) {
  if (!S.modello || S.erroreModello) return D18;
  if (S.erroreBozza) return BOZZA_NON_LETTA;
  if (!bozzaAperta()) return "nessuna bozza: la BOM di lavoro nasce dalla scelta dello STEP nella Sorgente";
  if (bozzaDaConvertire()) return "la bozza è nel formato di prima: per lavorarla va convertita";
  if (S.modello.congelata) return CONGELATA;
  if (S.salvataggio.stato === "conflitto") return "la bozza è cambiata altrove: rileggi la bozza salvata";
  if (S.salvataggio.stato === "rifiutata") return "il server ha rifiutato l'ultimo salvataggio: rileggi la bozza salvata";
  if (S.daParte) return "prima decidi le modifiche tenute da parte: riapplicale o scartale";
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
// nessunaModifica: la bozza è la conferma di base così come il server l'ha aperta («Modifica la BOM confermata»,
// da-conferma-1, EM §6.12) e dopo non è cambiato niente: né eventi dell'ingegnere o del server nel giornale (R1
// ?eventi=1) né gesti in coda. Allora «Rivedi e conferma BOM» e «Conferma BOM» sono spenti con il motivo, perché una
// conferma n+1 uguale alla n sarebbe un atto in più che non si toglie (MAT WZ-01). La prima conferma resta ammessa anche
// senza modifiche: la regola vale solo per una bozza aperta dalla conferma. Il segno vero lo darà il server (proposta di
// emendamento della corsia); quando il giornale non basta a dirlo, la pagina non spegne niente.
function nessunaModifica() {
  if (!bozzaLavorabile() || S.coda.length || S.invio || !Array.isArray(S.bozza.eventi)) return "";
  const ev = S.bozza.eventi.filter((e) => ogg(e) && typeof e.azione === "string").sort((a, b) => (Number(a.n) || 0) - (Number(b.n) || 0));
  if (!ev.length) return "";
  const ultimo = ev[ev.length - 1];
  if (ultimo.azione !== "bozza_aperta" || !ogg(ultimo.dopo) || ultimo.dopo.partenza !== "conferma") return "";
  const u = ultimaConferma();
  const base = S.salvato.versione_bom_base;
  if (!u || !u.valida || !base || Number(base.numero) !== Number(u.numero)) return "";
  return "nessuna modifica: la v" + u.numero + " è già confermata";
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
    // il 422 annullamento_non_ammesso ha il suo testo (MAT §5.4, riga R2; BZ-18)
    const testo = r.motivo === "annullamento_non_ammesso" ? "Questo passo è prima di un aggiornamento fatto dal server: per tornare indietro usa «Includi di nuovo» o «Ripristina»." : r.testo;
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
      nuovo = true;
    }
  } catch (e) {
    S.riepilogoStato = "errore";
    S.riepilogoErrore = testoErrore(e);
  }
  renderBarra();
  // l'Elenco e la linguetta «Documenti del nodo» mostrano gli stati delle preassociazioni che dà R4: si ridisegnano
  if (nuovo && S.passo === "revisione" && (S.vista === "elenco" || (S.nodo && S.linguettaNodo === "documenti"))) renderCorpo();
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
  } else if (g.modo === "bozza" && !v.noninc && sel.motivo) {
    // un nodo che non si può selezionare: la casella c'è, spenta, con il motivo (MAT NI-06); sulla scheda (236 × 84) il
    // motivo sta nel suggerimento e nell'etichetta accessibile, nel cassetto «Nodi segnalati» è scritto accanto
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
  if (S.erroreBozza) return pill("ko", "bozza: lettura non riuscita");
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
  if (S.erroreBozza && S.passo === "revisione") add("ko", "Lettura non riuscita: la bozza non si legge (" + S.erroreBozza + "). ↻ Aggiorna per riprovare.", null, "du-errore-bozza");
  if (S.daRileggere) add("ver", S.daRileggere, btn("↻ Rileggi", rileggiTutto, { cls: "du-btn--sm du-btn--pri", id: "du-rileggi", perche: false }), "du-da-rileggere");
  // la BOM congelata (MAT TS-11, §6.1): modifiche spente; «Chiedi una revisione autorizzata…» visibile e spento, con il motivo
  if (S.modello && !S.erroreModello && S.modello.congelata) add("info", "BOM congelata: le modifiche della bozza sono spente.",
    btn("Chiedi una revisione autorizzata…", null, { cls: "du-btn--sm", id: "du-chiedi-revisione", no: "Revisione e congelamento non sono in questa pagina" }), "du-congelata");
  const sosp = P.sospesi();
  for (const s of sosp) {
    const file = String(s.risorsa).indexOf("file:") === 0;
    const etichetta = "«" + (s.etichetta || s.nome) + "»" + (file ? " su " + fileDelSospeso(s).nome_file : "");
    if (s.certo) {
      // un esito certo da ritentare con la stessa chiave (409 COPIA_IN_CORSO): niente è registrato e niente è in volo, non
      // c'è niente da verificare (MAT §5.2, «copia in corso»; §5.4, S3)
      const riprovaC = s.nome === "confermaBom" && s.risorsa === "bozza:" + S.prodotto && bozzaLavorabile();
      add("ver", etichetta + ": " + (s.nome === "confermaBom" ? "una copia sul NAS è in corso" : "una copia di conservazione è già in corso per questo file") + ", niente è registrato. Riprova fra poco con la stessa chiave.",
        h("span", { class: "du-azioni" },
          riprovaC ? btn("Riprova la conferma…", riprovaConfermaSospesa, { cls: "du-btn--sm du-btn--pri", id: "du-riprova-sospesa", perche: false }) : null,
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
        btn("Aggiorna alla proposta di oggi…", apriRiallinea, { cls: "du-btn--sm", id: "du-riallinea", no: perchéNoModifica() })) };
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
      btn("Rifiuta la candidata…", null, { cls: "du-btn--sm", no: "Il rifiuto motivato di una candidata arriva in V5: intanto si sceglie un altro STEP" })),
    secondario(f));
}
// secondario: l'avviso «STEP secondario disponibile» (MAT SG-12): una riga, senza gesti; l'accettazione degli archi
// arriva in V5. La forma di step_secondario la dice il modello (nome del file, oppure l'allegato).
function secondario(f) {
  const s2 = f && f.step_secondario;
  if (!s2) return null;
  const nome = typeof s2 === "string" ? s2 : s2.nome_file || nomeFile(s2.allegato_id) || "uno STEP";
  return h("p", { class: "du-nota", id: "du-step-secondario" }, pill("info", "STEP secondario disponibile"), " ", h("span", { class: "du-mono" }, nome),
    typeof s2 === "object" && s2.motivo ? " · " + s2.motivo : "", ". L'accettazione dei suoi archi arriva in V5.");
}
function stepDellaRfq() {
  const st = arr(S.sorgente && S.sorgente.step);
  if (st.length) return st;
  return fileSorgente().filter((f) => /^(stp|step)$/i.test(f.estensione || "") && f.stato !== "scartato");
}
// gruppiDelloStep: le strutture che il modello propone per il prodotto da uno STEP (/dati, gruppi[]; PE-01 esteso):
// radice letta, compatibilità, stato della struttura (LS-01) e nodi segnalati (LS-02), per la fascia «Quale struttura
// guardare» (MAT SG-01). Solo presentazione: le dice il modello.
function gruppiDelloStep(allegato) {
  return arr(S.modello && S.modello.gruppi).filter((x) => x.allegato_id === allegato);
}
function sintesiStep(allegato) {
  const gr = gruppiDelloStep(allegato);
  if (!S.modello || S.erroreModello) return null;
  if (!gr.length) return h("div", { class: "du-fr-meta du-piccolo", "data-du-candidate": "0" }, pill("neu", "nessuna candidata per " + (codiceProdotto() || "il prodotto")));
  return h("div", { class: "du-fr-meta du-piccolo", "data-du-candidate": String(gr.length) }, gr.map((x) => {
    const st = ogg(x.struttura) ? (x.struttura.completa === false ? "struttura non completa" + (x.struttura.motivo ? ": " + x.struttura.motivo : "") : "struttura completa") : "";
    return h("span", { class: "du-st du-st--wrap du-st--" + (x.compatibilita === "compatibile" ? "ok" : "ver"), title: x.chiave || "" },
      "candidata " + (x.radice_letterale || breve(x.radice || x.chiave || "", 24)) + (x.compatibilita ? " · " + x.compatibilita : "") + (st ? " · " + st : "") +
      (typeof x.segnalati === "number" ? " · segnalati: " + x.segnalati : ""));
  }));
}
function riquadroStep() {
  const elenco = stepDellaRfq();
  const box = h("div", { class: "du-riq", id: "du-step" }, h("div", { class: "du-riq-testa" }, h("h3", {}, "STEP della RFQ"),
    h("span", { class: "du-muto du-piccolo" }, "Per ogni STEP: che cosa propone il motore per " + (codiceProdotto() || "il prodotto") + ".")));
  if (S.erroreSorgente) {
    // niente elenco vecchio mostrato come attuale (MAT §5.4, «letture: 5xx»)
    box.append(h("div", { class: "du-striscia du-striscia--ko", id: "du-step-non-letti" }, "Lettura non riuscita: gli STEP della RFQ non si leggono. ↻ Aggiorna per riprovare."));
    return box;
  }
  if (!elenco.length) {
    box.append(h("p", { class: "du-muto du-piccolo" }, "Nessuno STEP fra i file attivi: carica lo STEP del prodotto con «+ Aggiungi file»."));
    return box;
  }
  // «nessuna candidata» (MAT SG-13): ci sono STEP, ma il modello non dà strutture per il prodotto
  if (S.modello && !S.erroreModello && prodottoScelto() && !S.modello.gruppi.length) {
    box.append(h("p", { class: "du-nota", id: "du-nessuna-candidata" }, "Nessuna candidata per " + codiceProdotto() + " fra gli STEP della RFQ: «Guarda» dice il motivo per ciascuno; se manca lo STEP del prodotto, caricalo con «+ Aggiungi file»."));
  }
  const ul = h("ul", { class: "du-fl" });
  for (const s of elenco) {
    const f = fileDi(s.allegato_id) || s;
    const attivo = !f.stato || f.stato === "attivo";
    const inVista = S.anteprimaAllegato === s.allegato_id;
    ul.append(h("li", { class: "du-fr" + (inVista ? " du-fr--sel" : ""), id: "du-step-" + sid(s.allegato_id) },
      h("div", {}, h("div", { class: "du-fr-nome" }, s.nome_file || f.nome_file), h("div", { class: "du-fr-meta" }, pill("neu", testoAnalisi(f)), inVista ? pill("prop", "in vista, non confermata") : null),
        sintesiStep(s.allegato_id)),
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
  const scelto = gruppi.find((x) => x.gruppo === S.anteprimaGruppo) || null;
  // la scelta della struttura: con più candidate, e anche con una sola quando non è in vista (un'incerta non si mette in
  // vista da sola, ma si guarda e si usa con un gesto esplicito; SG-02, [U] RAT05-FONTE-SOSTITUIBILE)
  if (gruppi.length > 1 || (gruppi.length === 1 && !scelto)) {
    box.append(h("div", { class: "du-cartelle", role: "group", "aria-label": "Strutture dello STEP", id: "du-strutture" }, gruppi.map((x) => h("button", { type: "button", "aria-pressed": x.gruppo === S.anteprimaGruppo ? "true" : "false",
      id: "du-struttura-" + sid(x.gruppo), title: "compatibilità: " + (x.compatibilita || "?"), onclick: () => guarda(a.allegato_id, x.gruppo) },
    (x.radice && x.radice.identita_proposta && x.radice.identita_proposta.letterale) || x.gruppo.slice(0, 18), " · ", x.compatibilita || ""))));
    if (!scelto) box.append(h("p", { class: "du-nota", id: "du-nessuna-in-vista" }, "Nessuna struttura in vista: una candidata incerta non si mette in vista da sola. Sceglila qui sopra per guardarla."));
  }
  if (!gruppi.length) {
    box.append(h("div", { class: "du-striscia du-striscia--ver", id: "du-anteprima-nessuna" }, "Nessuna candidata per " + codiceProdotto() + " in questo STEP" +
      (a.testo ? ": " + a.testo : a.struttura && a.struttura.motivo ? ": " + a.struttura.motivo : "") + ". Se lo STEP del prodotto è un altro file, caricalo con «+ Aggiungi file»."));
    return box;
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
  if (S.erroreBozza) {
    // la bozza del prodotto non si legge: né «Usa per la BOM» né «Cambia lo STEP» (MAT §5.4)
    az.append(btn("Usa per la BOM di " + prodotto + "…", null, { cls: "du-btn--sm", id: "du-usa-bom", no: "la bozza del prodotto non si legge: ↻ Aggiorna" }));
  } else if (!bozzaAperta()) {
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
  const r = await inSerie("carica", () => P.carica(files, origine));
  const ul = moduloCarica().__esiti;
  ul.replaceChildren();
  for (const f of arr(r.file)) {
    const testo = f.esito === "caricato" ? (f.ricaricato_dopo_scarto ? "ricaricato: è un file nuovo della RFQ; lo scarto di prima resta nella storia" : "caricato · " + (ANALISI[f.analisi] || f.analisi || "in analisi"))
      : f.esito === "gia_presente" ? "già presente" + (f.stato_file === "altri_file" ? " (in Altri file: Riattiva)" : "")
      : "rifiutato: " + ((f.motivo && f.motivo.testo) || "motivo non dato");
    ul.append(h("li", { "data-esito": f.esito }, h("span", { class: "du-mono" }, f.nome_file), pill(f.esito === "rifiutato" ? "ko" : f.esito === "gia_presente" ? "neu" : "ok", testo)));
  }
  // i rifiuti di E1′ (MAT §5.4): 413 «troppo grande: dividi in più invii»; 503 «caricamento non configurato su questo
  // server»; 422 NESSUN_FILE_CARICATO con i motivi file per file (le righe qui sopra)
  const testi = { TROPPO_GRANDE: "Troppo grande: dividi in più invii.", NON_CONFIGURATO: "Caricamento non configurato su questo server.",
    NESSUN_FILE_CARICATO: "Nessun file è stato caricato: i motivi sono file per file, qui sotto." };
  if (r.esito === "caricati") avvisa("ok", r.testo || "File caricati.");
  else if (r.esito === "parziale") avvisa("no", "Caricamento parziale: " + (r.testo || "alcuni file non sono stati caricati") + ". Non caricati: " + arr(r.file).filter((f) => f.esito === "rifiutato").map((f) => f.nome_file).join(", ") + ".");
  else avvisa(r.esito === "no" ? "no" : "errore", testi[r.codice] || r.testo || "Il caricamento non è riuscito.");
  await caricaSorgente();
  await caricaModello();
  render();
}
// Le linguette dei file della Sorgente (MAT §3.3): «Tutti» (i file attivi), «Associati / in proposta», «Senza
// associazione», «Da ricollocare» (il vassoio della bozza), «Altri file», «Scartati». Solo presentazione, sui dati di S1,
// della bozza e del suo registro.
const LINGUETTE_FILE = [["attivi", "Tutti"], ["associati", "Associati / in proposta"], ["senza", "Senza associazione"], ["ricollocare", "Da ricollocare"],
  ["altri", "Altri file"], ["scartati", "Scartati"]];
function cartelleFile() {
  const tutti = fileSorgente();
  const attivi = tutti.filter((f) => !f.stato || f.stato === "attivo");
  const vassoio = new Set(bozzaLavorabile() ? vassoioCorrente().map((x) => x.proposta_id) : []);
  const associato = (f) => {
    const d = destinazioneFile(f);
    return !!(d.pb || d.pa.length || d.noninc.length || (f.proposta && f.proposta.componente_id));
  };
  return { attivi, associati: attivi.filter(associato), senza: attivi.filter((f) => !associato(f)), ricollocare: attivi.filter((f) => vassoio.has(f.proposta_id)),
    altri: tutti.filter((f) => f.stato === "altri_file"), scartati: tutti.filter((f) => f.stato === "scartato") };
}
function riquadroFile() {
  const box = h("div", { class: "du-riq", id: "du-file" });
  if (S.erroreSorgente) {
    // niente elenco vecchio mostrato come attuale (MAT §5.4, «letture: 5xx»): la lista non c'è finché S1 non si rilegge
    box.append(h("div", { class: "du-riq-testa" }, h("h3", {}, "File della RFQ")),
      h("div", { class: "du-striscia du-striscia--ko", id: "du-file-non-letti" }, "Lettura non riuscita: l'elenco dei file non è aggiornato (" + S.erroreSorgente + "). ↻ Aggiorna per riprovare."),
      moduloCarica());
    return box;
  }
  const cart = cartelleFile();
  if (!cart[S.cartella]) S.cartella = "attivi";
  box.append(h("div", { class: "du-riq-testa" }, h("h3", {}, "File della RFQ"),
    h("div", { class: "du-cartelle", role: "group", "aria-label": "Cartelle dei file" },
      LINGUETTE_FILE.map(([k, t]) => h("button", { type: "button", id: "du-cartella-" + k, "aria-pressed": S.cartella === k ? "true" : "false",
        onclick: () => {
          S.cartella = k;
          render();
        } }, t + " (" + cart[k].length + ")")))));
  if (S.cartella === "attivi") box.append(moduloCarica());
  if (S.cartella === "altri") box.append(h("p", { class: "du-nota" }, "Altri file: restano nella RFQ, si aprono e si scaricano quando serve, anche dopo un riavvio e dopo la pulizia dello staging. Non entrano nella proposta della BOM e non ricevono associazioni finché non li riattivi. Non è la cartella ALTRO del NAS."));
  if (S.cartella === "ricollocare") box.append(h("p", { class: "du-nota" }, "File preassociati che un gesto della bozza ha tolto dal loro pezzo: ricollocali dalla Revisione, oppure lasciali senza associazione. Niente è cancellato."));
  const elenco = cart[S.cartella];
  const vuoti = { attivi: "Nessun file attivo.", associati: "Nessun file associato o in proposta.", senza: "Nessun file senza associazione.", ricollocare: "Nessun file da ricollocare.",
    altri: "Nessun file in Altri file.", scartati: "Nessun file scartato." };
  if (!elenco.length) box.append(h("p", { class: "du-muto du-piccolo" }, vuoti[S.cartella]));
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
  // dove va il file nella BOM di lavoro (MAT SG-06): preassociato nella bozza, oppure proposto dal motore per un pezzo
  const dove = destinazioneFile(f);
  if (f.proposta && f.proposta.componente_id) meta.push(pill("info", f.documento ? "✓ confermato" : "● associato"));
  if (dove.ricodifica) meta.push(pill("ver", "da ricodificare", "il componente del file ha una ricodifica nella bozza: alla conferma il file segue il codice nuovo (MAT §4.3 riga 7)"));
  if (dove.pb) meta.push(pill("bozza", "◈ preassociato a " + dove.pb));
  for (const x of dove.pa) meta.push(pill("prop", "◇ proposto per " + x));
  for (const x of dove.noninc) meta.push(pill("neu", "◇ proposto per " + x + ": non richiesta, nodo non incluso (bozza)"));
  const az = h("div", { class: "du-azioni du-fr-az" });
  const contenuto = f.azioni && f.azioni.contenuto;
  if (f.stato !== "scartato") {
    if (contenuto && contenuto.disponibile === false) az.append(btn("Apri", null, { cls: "du-btn--sm", no: "contenuto non disponibile: " + (contenuto.motivo || "") }));
    else az.append(bottoneApri(f, "du-apri-" + sid(f.allegato_id)),
      h("a", { class: "du-btn du-btn--sm du-btn--ghost", href: P.urlContenuto(f.allegato_id, "scarica"), download: f.nome_file || "", id: "du-scarica-" + sid(f.allegato_id) }, "Scarica"));
  }
  // associare il file a un pezzo dalla Sorgente (MAT FL-12, come EL-02…EL-04): una preassociazione della bozza
  if ((!f.stato || f.stato === "attivo") && f.proposta_id && !/^(stp|step)$/i.test(f.estensione || "")) {
    if (dove.pbChiave) az.append(btn("Togli la preassociazione", () => togliPreassociazione(f.proposta_id, dove.pbChiave), { cls: "du-btn--sm du-btn--ghost", id: "du-togli-pb-" + sid(f.allegato_id), no: perchéNoModifica() }));
    else az.append(btn("Assegna a un pezzo…", () => apriAssegna(f), { cls: "du-btn--sm", id: "du-assegna-" + sid(f.allegato_id),
      no: S.erroreBozza ? BOZZA_NON_LETTA : !bozzaAperta() ? "prima crea la BOM dalla proposta (Usa per la BOM)" : perchéNoModifica() }));
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
  if (f.stato === "altri_file") {
    // chi l'ha messo in Altri file, quando e perché (MAT FL-08; la forma proposta nel resoconto della corsia, PE-06)
    const af = ogg(f.altri_file) ? f.altri_file : arr(f.eventi).find((e) => e.azione === "altri_file") || null;
    if (af) traccia.push("In Altri file da " + (af.autore || af.autore_id || "?") + " il " + dataOra(af.quando) + (af.motivo ? ": " + af.motivo : "") + ". ");
  }
  if (f.stato === "altri_file" && f.conservazione) {
    const c = f.conservazione.stato;
    const motivo = f.conservazione.motivo || f.conservazione.causa || "";
    traccia.push(c === "copiata" ? "Conservato (copia verificata)." : c === "in_copia" ? "Conservazione in corso." : c === "mancante" ? "Copia nel deposito mancante: il file resta nello staging." :
      c === "errore" ? "Conservazione non riuscita: " + (motivo || "motivo non dato") + "." : "Conservazione: " + c + ".");
  }
  return h("li", { class: cls, id: "du-file-" + sid(f.allegato_id), "data-stato": f.stato || "attivo" },
    h("div", {}, h("div", { class: "du-fr-nome" }, f.nome_file), h("div", { class: "du-fr-meta" }, meta)),
    h("div", {}),
    traccia.join("").trim() ? h("div", { class: "du-fr-traccia", "data-du-traccia": "1" }, traccia.join("").trim()) : null,
    az);
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
    const st = e && (e.status || (e.details && e.details.status));
    tela.replaceChildren(h("p", { class: "du-err", id: "du-visore-errore" }, "Contenuto non disponibile" +
      (st === 410 ? ": il file è scartato o i suoi byte non ci sono più (per riaverlo, ricaricalo)" : st ? " (la rotta del contenuto risponde " + st + ")" : ": " + testoErrore(e)) + "."));
  }
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
  if (!S.lavoro || !f.proposta_id) return out;
  const nodoDi = (k) => S.lavoro.nodi.find((n) => n.chiave_bozza === k);
  const cod = (n) => (n && n.identita && n.identita.codice) || (n && n.descrizione) || (n && n.chiave_bozza) || "?";
  // un'associazione registrata di un componente con una ricodifica nella bozza: alla conferma il file segue il codice
  // nuovo, e la pagina lo segna «da ricodificare» (MAT §4.3 riga 7)
  const cid = f.proposta && f.proposta.componente_id;
  if (cid) out.ricodifica = S.lavoro.decisioni.some((d) => d.tipo === "ricodifica" && d.componente_id === cid);
  const pb = S.lavoro.documenti_preassociati.find((d) => d.proposta_id === f.proposta_id);
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
// «Assegna a un pezzo…» (MAT FL-12, EL-04): la preassociazione del file a un pezzo della bozza, con un salvataggio. La
// proposta del motore, se c'è, si mostra accanto: non si sceglie da sola.
function apriAssegna(f) {
  const g = grafoBozza();
  const proposti = destinazioneFile(f).pa;
  const pezzi = Object.values(g.nodi).filter((v) => !v.noninc && v.nodo).sort((a, b) => String(a.cod || a.desc).localeCompare(String(b.cod || b.desc)));
  const sel = h("select", { id: "f-pezzo" }, h("option", { value: "" }, "— scegli il pezzo —"), pezzi.map((v) => h("option", { value: v.k }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "") + " · " + (v.desc || ""))));
  apriFinestra("Assegna «" + f.nome_file + "» a un pezzo", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "È una preassociazione della bozza: la conferma della BOM la applica. Il codice e la rev del file restano quelli letti: se non sono quelli del pezzo, il riepilogo la dirà «in conflitto»."),
    proposti.length ? h("p", { class: "du-nota", id: "f-proposti" }, "Il motore lo propone per: ", h("span", { class: "du-mono" }, proposti.join(", ")), ".") : null,
    campo("Pezzo", sel)),
  [{ testo: "Annulla", chiudi: true }, { testo: "Assegna", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-pezzo") ? "" : "scegli il pezzo"), fai: () => {
    preassocia(f.proposta_id, valore($("#du-finestra"), "f-pezzo"));
  } }]);
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
    h("div", { class: "du-zona-corpo" }, S.vista === "org" ? [riquadroProposta(), strumentiOrg(g), cassettoSegnalati(g), barraSelezione(g), organigramma(g)] : [riquadroProposta(), vassoio(), elencoRighe(g)]));
  const nodo = h("section", { class: "du-zona du-zona--nodo", "aria-label": "Pannello del nodo" },
    h("div", { class: "du-zona-testa" }, h("h2", {}, "Pannello del nodo")),
    h("div", { class: "du-zona-corpo" }, g.nodi[S.nodo] ? pannelloNodo(g, g.nodi[S.nodo]) : h("p", { class: "du-muto du-piccolo" }, "Scegli un pezzo: identità proposta, riferimenti, provenienza e decisione dell'ingegnere stanno qui, non sulle schede.")));
  return h("div", {}, zoneTabs(), h("div", { class: "du-ws", "data-zona": S.zona }, vista, nodo));
}
function revisioneSenzaBozza() {
  if (S.erroreBozza) {
    // una lettura non riuscita non è «nessuna bozza» (MAT §5.1, §5.4): nessun gesto di creazione, solo «↻ Aggiorna»
    return h("div", { class: "du-vuoto", id: "du-bozza-non-letta" },
      h("p", {}, "Lettura non riuscita: la bozza di " + (codiceProdotto() || "questo prodotto") + " non si legge (" + S.erroreBozza + "). Niente è mostrato come attuale finché la lettura non riesce."),
      h("div", { class: "du-azioni" }, btn("↻ Aggiorna", aggiorna, { cls: "du-btn--sm du-btn--pri", id: "du-aggiorna-bozza", perche: false })));
  }
  const ultima = ultimaConferma();
  return h("div", { class: "du-vuoto", id: "du-nessuna-bozza" },
    h("p", {}, "Nessuna bozza per " + codiceProdotto() + ". La BOM di lavoro nasce dalla scelta dello STEP nella Sorgente («Usa per la BOM»)."),
    h("div", { class: "du-azioni" },
      btn("Vai alla Sorgente", () => vaiPasso("sorgente"), { cls: "du-btn--sm", perche: false }),
      btn("Modifica la BOM confermata", () => creaDa("conferma"), { cls: "du-btn--sm", id: "du-da-conferma", no: S.modello && S.modello.congelata ? CONGELATA : ultima ? "" : "il prodotto non ha una conferma" }),
      btn("Parti dalla BOM registrata", () => creaDa("registrato"), { cls: "du-btn--sm", id: "du-da-registrato", no: S.modello && S.modello.congelata ? CONGELATA : "", perche: false })),
    S.erroreStorico ? h("p", { class: "du-muto du-piccolo" }, "Lo storico delle conferme non si legge: " + S.erroreStorico) : null);
}
// statoConferma: «vN confermata» e «Verificata» sono due segni distinti, mai fusi (MAT §6.2).
// verificaNonCalcolata: l'ultima conferma è una conferma di R5 di V1, la cui verifica in V1 non è calcolata (mai
// positiva, EM §9.5 passo 12). Il segno preferito è un campo del server (verifica_calcolata: false, proposta PE-07);
// finché R7 non lo dà, la pagina legge «formato», l'unico campo di R7 che lo dice (EM §9.7). È una deroga dichiarata
// alla regola di MAT §1.3 («nessun numero di formato nella pagina»), che riguarda il formato della bozza (lo dice R1).
function verificaNonCalcolata(u) {
  if (!u) return false;
  if (typeof u.verifica_calcolata === "boolean") return !u.verifica_calcolata;
  return u.formato === 3 || !!(u.origine && u.origine.rotta === "motore-a/bom/conferma");
}
function statoConferma() {
  const ultima = ultimaConferma();
  const out = h("span", { class: "du-azioni", id: "du-stato-conferma" });
  const da = ultima ? ultima.confermato_da_nome || ultima.confermato_da_sigla || (ultima.confermato_da ? String(ultima.confermato_da).slice(0, 8) : "") : "";
  if (ultima) out.append(pill(ultima.valida ? "ok" : "ver", ultima.valida ? "v" + ultima.numero + " confermata il " + dataOra(ultima.confermato_il) + (da ? " da " + da : "") : "v" + ultima.numero + " confermata, poi la struttura è cambiata: serve una conferma nuova", "conferma n. " + ultima.numero));
  else out.append(pill("neu", "nessuna conferma"));
  const p = prodottoScelto();
  if (verificaNonCalcolata(ultima)) out.append(pill("neu", "Verificata: non ancora calcolata (arriva in V2)"));
  else if (p) out.append(pill(p.verificato ? "ok" : "neu", "Verificata (calcolo di oggi): " + (p.verificato ? "sì" : "no"), arr(p.motivi).map((m) => (m && (m.codice || m.motivo)) || m).join(", ")));
  // gli altri segni di MAT §6.2: il finito mono_pezzo (della conferma, o proposto nella bozza) e la BOM congelata
  if (ultima && ultima.valida && ultima.mono_pezzo) out.append(pill("info", "finito mono_pezzo"));
  else if (S.lavoro && S.lavoro.decisioni.some((d) => d.tipo === "mono_pezzo")) out.append(pill("bozza", "mono_pezzo proposto in bozza"));
  if (S.modello && S.modello.congelata) out.append(pill("ver", "BOM congelata"));
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
// non cambiano l'esito del segnale, aiutano a decidere. Un codice sconosciuto si mostra com'è.
const CONTRO_INDIZI = {
  lettura_identita: "il nodo ha anche una lettura d'identità: potrebbe essere un pezzo vero",
  grafo_non_completo: "il grafo del file non è completo: la foglia potrebbe avere figli non letti",
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
      h("input", { type: "checkbox", checked: S.selezione.has(v.k), disabled: !sel.ok, title: sel.motivo || null, "aria-label": "Seleziona " + ((v.voce && v.voce.grezzi && v.voce.grezzi.nome) || v.k),
        onchange: (e) => cambiaSelezione(v.k, e.target.checked) }),
      h("span", { class: "du-segnalato-n" }, (v.voce && v.voce.grezzi && (v.voce.grezzi.nome || v.voce.grezzi.id)) || v.desc || v.k, !sel.ok && sel.motivo ? h("span", { class: "du-perche" }, " non selezionabile: " + sel.motivo) : null),
      h("span", { class: "du-segnalato-s" }, (padre ? "sotto " + (g.nodi[padre.p].cod || g.nodi[padre.p].desc) + " · " : "") + (seg.regola || "") + (seg.spiegazione ? " · " + seg.spiegazione : "") +
        (arr(seg.contro_indizi).length ? " · contro-indizi: " + arr(seg.contro_indizi).map(controIndizio).join("; ") : ""))));
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
  const out = { pb: [], pa: [], ar: [], nonRichieste: [] };
  if (S.lavoro && v.noninc) {
    // le proposte d'associazione di un nodo non incluso: «non richiesta: nodo non incluso (bozza)» (MAT §4.3 riga 5)
    for (const a of arr(v.voce && v.voce.associazioni_proposte)) {
      const f = fileDi(a.allegato_id);
      if (f && (!f.stato || f.stato === "attivo")) out.nonRichieste.push({ proposta_id: f.proposta_id, allegato_id: a.allegato_id });
    }
  }
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
  const nomi = doc.pb.map((pid) => (fileDiProposta(pid) || {}).nome_file).concat(doc.pa.map((x) => nomeFile(x.allegato_id)), doc.ar.map((f) => f.nome_file));
  return [v.cod, v.desc, v.rev ? "rev " + v.rev : ""].concat(nomi).some((t) => String(t || "").toLowerCase().indexOf(q) >= 0);
}
// «Accetta le proposte valide (N)» (MAT EL-03): le proposte del motore per i pezzi della bozza, in un solo salvataggio;
// nessuna preselezione, l'elenco si riduce togliendo le spunte.
function proposteDaAccettare(g) {
  const out = [];
  for (const v of Object.values(g.nodi)) {
    if (v.noninc || !v.nodo) continue;
    for (const pa of documentiNodo(v).pa) out.push({ k: v.k, cod: v.cod || v.desc || v.k, pa });
  }
  return out;
}
function apriAccettaProposte(g) {
  const elenco = proposteDaAccettare(g);
  if (!elenco.length) return avvisa("no", "Nessuna proposta del motore da accettare.");
  apriFinestra("Accetta le proposte valide", h("div", { style: "display:grid;gap:8px" },
    h("p", { class: "du-nota" }, "Ogni spunta diventa una preassociazione della bozza; nessuna è preselezionata. Le proposte senza spunta restano proposte."),
    h("div", { class: "du-voci-sel", id: "f-proposte" }, elenco.map((x) => h("label", { class: "du-spunta" }, h("input", { type: "checkbox", "data-pid": x.pa.proposta_id, "data-k": x.k }),
      h("span", {}, h("span", { class: "du-mono" }, nomeFile(x.pa.allegato_id)), " → ", h("span", { class: "du-mono" }, x.cod), x.pa.motivo ? " · " + x.pa.motivo : "", x.pa.livello ? " (" + x.pa.livello + ")" : ""))))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Accetta le proposte scelte", pri: true, id: "f-ok", spento: () => ($("#du-finestra").querySelectorAll("#f-proposte input:checked").length ? "" : "scegli almeno una proposta"), fai: () => {
    const scelte = [...$("#du-finestra").querySelectorAll("#f-proposte input:checked")].map((x) => ({ pid: x.dataset.pid, k: x.dataset.k }));
    fai("accettate " + scelte.length + " proposte", scelte.map((x) => ({ azione: "file_preassociato", chiave_bozza: x.k, prima: null, dopo: { proposta_id: x.pid, chiave_bozza: x.k }, origine: "utente" })));
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
    btn("Accetta le proposte valide (" + n + ")", () => apriAccettaProposte(g), { cls: "du-btn--sm", id: "du-accetta-valide", no: !n ? "nessuna proposta da accettare" : perchéNoModifica(), perche: false }),
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
    const doc = g.modo === "bozza" ? documentiNodo(v) : { pb: [], pa: [], ar: [], nonRichieste: [] };
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
    const ricod = v.nodo && v.nodo.componente_id && S.lavoro.decisioni.some((d) => d.tipo === "ricodifica" && d.componente_id === v.nodo.componente_id);
    for (const f of doc.ar) {
      const sv0 = svincoloDi(f.proposta_id);
      // un documento già confermato non si scollega da qui: serve una revisione, non in V1 (EM §3.6.2)
      celleDoc.push(h("span", { class: "du-el-doc" }, pill(f.documento ? "ok" : sv0 ? "ver" : "info", f.documento ? "✓ confermato" : sv0 ? "⊘ da scollegare" : "● associato"), h("span", { class: "du-mono" }, f.nome_file),
        ricod ? pill("ver", "da ricodificare") : null,
        sv0 || f.documento ? null : btn("Scollega dal componente…", () => apriScollega(f, v), { cls: "du-btn--sm du-btn--ghost", no: perchéNoModifica(), perche: false })));
    }
    for (const x of doc.nonRichieste) celleDoc.push(h("span", { class: "du-el-doc" }, pill("neu", "◇ non richiesta: nodo non incluso (bozza)"), h("span", { class: "du-mono" }, nomeFile(x.allegato_id))));
    if (!celleDoc.length) celleDoc.push(h("span", { class: "du-muto du-piccolo" }, v.tipo === "commerciale" ? "2D non richiesto" : "nessun documento"));
    // fra le righe della bozza: l'esito documentale per i filtri (data-du-esito)
    box.append(h("div", { class: "du-el-r" + (S.nodo === r.k ? " du-el-r--scelta" : "") + (v.escluso || v.noninc ? " du-el-r--escl" : ""), role: "row", id: "el-" + sid(r.k), "data-k": r.k, "data-du-esito": r.es || null },
      h("button", { type: "button", class: "du-el-nodo", style: "--du-d:" + r.d, onclick: () => scegli(r.k) }, h("span", { class: "du-el-cod" }, (v.cod || "(senza codice)") + (v.rev ? " rev " + v.rev : "")),
        h("span", { class: "du-el-desc" }, v.desc || "—")),
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
  // le linguette «Dettaglio nodo» e «Documenti del nodo» (MAT PN-02): solo nel browser
  box.append(h("div", { class: "du-linguette", role: "tablist", "aria-label": "Pannello del nodo" }, [["dettaglio", "Dettaglio nodo"], ["documenti", "Documenti del nodo"]].map(([k, t]) =>
    h("button", { type: "button", role: "tab", id: "du-ling-" + k, "aria-selected": S.linguettaNodo === k ? "true" : "false", onclick: () => {
      S.linguettaNodo = k;
      renderCorpo();
    } }, t))));
  if (S.linguettaNodo === "documenti") {
    box.append(documentiDelNodo(g, v));
    return box;
  }
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
    const k4 = r.tipo === "equivalente_k4" || !!r.equivalente;
    rif.push(h("li", { "data-du-riferimento": r.tipo || "" }, testo, r.esito ? " · " + r.esito : "", g.modo === "bozza" && v.nodo && (r.lettura || r.equivalente) ? h("span", { class: "du-azioni" },
      btn("Adotta come codice…", () => apriAdotta(v, r, proiezione), { cls: "du-btn--sm", no: !proiezione ? "il registro non dà la proiezione del codice per questa lettura: la calcola il server (proposta di emendamento della corsia)" : perchéNoModifica() }),
      // il rifiuto esplicito del P/N K4: un'alternativa sulla lettura primaria, con il motivo (MAT PN-08; EM §3.6.1)
      k4 ? btn("Conferma l'identità proposta…", () => apriConfermaIdentita(v, r), { cls: "du-btn--sm", id: "du-conferma-identita", no: perchéNoConfermaIdentita(v) }) : null) : null));
  }
  if (voce && voce.codice_manuale_legacy) rif.push(h("li", {}, "codice corretto a mano nella pagina vecchia (evidenza): ", h("span", { class: "du-mono" }, voce.codice_manuale_legacy.codice),
    g.modo === "bozza" ? h("span", { class: "du-azioni" }, btn("Adotta come codice…", () => apriAdotta(v, { tipo: "manuale_legacy" }, voce.codice_manuale_legacy.codice), { cls: "du-btn--sm", no: perchéNoModifica() })) : null));
  const reg = g.modo === "bozza" ? registroBozza() : g.registro;
  for (const r of arr(reg && reg.proposta && reg.proposta.relazioni).filter((x) => voce && x.sorgente && x.sorgente.voce === voce.voce)) {
    rif.push(h("li", {}, "relazione " + r.tipo + " · " + (r.stato || "proposta") + ", non ratificata", h("span", { class: "du-azioni" },
      btn("Accetta o rifiuta", null, { cls: "du-btn--sm", no: NON_V1.relazioni }))));
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
// equivalente K4»): un'alternativa sulla lettura primaria, con il motivo. Il nodo tiene l'identità proposta, ma come
// identità decisa, e l'avviso EQUIVALENTE_NON_ADOTTATO non scatta più. La copia della lettura e la chiave stanno già nel
// registro della bozza: la proiezione non serve, perché il codice proposto è la proiezione della primaria (EM §6.1).
function apriConfermaIdentita(v, r) {
  const voce = v.voce;
  const ip = voce.identita_proposta;
  const id = v.nodo.identita || {};
  const eq = r.equivalente || {};
  apriFinestra("Conferma l'identità proposta di " + (v.cod || "questo pezzo"), h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'identità resta quella proposta (", h("span", { class: "du-mono" }, (id.codice || "") + (id.rev ? " rev " + id.rev : "")), ", " + (ip.emittente || "") + "), ma diventa un'identità decisa: il P/N equivalente ",
      h("span", { class: "du-mono" }, eq.base || "?"), " (" + (eq.emittente || "emittente non concorde") + ") non si adotta."),
    campo("Motivo", area("f-motivo", "perché non adotti il P/N equivalente"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Conferma l'identità proposta", pri: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const identita = { emittente: id.emittente, codice: id.codice };
    if (id.namespace) identita.namespace = id.namespace;
    if (id.rev) identita.rev = id.rev;
    const dec = { id: nuovoIdDecisione("di"), tipo: "identita", chiave_bozza: v.k, modo: "alternativa", importazione: S.bozza.importazione.impronta, chiave_lettura: ip.chiave_lettura,
      lettura: copiaLettura(ip), identita, motivo };
    fai("confermata l'identità proposta di " + (v.cod || "un pezzo"), [{ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: JSON.parse(JSON.stringify(dec)), origine: "utente" },
      { azione: "nodo_modificato", chiave_bozza: v.k, prima: { identita: clona(id) }, dopo: { identita: Object.assign({}, identita, { modo: "decisione", decisione: dec.id }) }, origine: "utente" }]);
  } }]);
}
// copiaLettura: la copia compatta di una lettura del registro (EM §3.6.1, «lettura»), byte per byte.
function copiaLettura(l) {
  return { emittente: l.emittente, hash_grammatica: l.hash_grammatica, namespace: l.namespace, letterale: l.letterale,
    base: l.base ? { originale: l.base.originale, normalizzata: l.base.normalizzata } : undefined, qualificatori: l.qualificatori, revisione: l.revisione === undefined ? null : l.revisione };
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
    const ricod = v.nodo && v.nodo.componente_id && S.lavoro.decisioni.some((d) => d.tipo === "ricodifica" && d.componente_id === v.nodo.componente_id);
    for (const pid of doc.pb) {
      const f = fileDiProposta(pid);
      const es = esitoPreassociazione(pid);
      righe.push(h("li", { "data-du-doc": "preassociato" }, pill("bozza", "◈ preassociato"), " ", h("span", { class: "du-mono" }, f ? f.nome_file : pid.slice(0, 8)), es ? pill(es === "applicabile" ? "ok" : "ver", es) : null, apri(f)));
    }
    for (const pa of doc.pa) righe.push(h("li", { "data-du-doc": "proposto" }, pill("prop", "◇ proposto"), " ", h("span", { class: "du-mono" }, nomeFile(pa.allegato_id)), pa.motivo ? " · " + pa.motivo : "", apri(fileDi(pa.allegato_id))));
    for (const f of doc.ar) {
      const sv0 = svincoloDi(f.proposta_id);
      righe.push(h("li", { "data-du-doc": f.documento ? "confermato" : sv0 ? "da_scollegare" : "associato" }, pill(f.documento ? "ok" : sv0 ? "ver" : "info", f.documento ? "✓ confermato" : sv0 ? "⊘ da scollegare" : "● associato"), " ",
        h("span", { class: "du-mono" }, f.nome_file), ricod ? pill("ver", "da ricodificare") : null, apri(f)));
    }
    for (const x of doc.nonRichieste) righe.push(h("li", { "data-du-doc": "non_richiesta" }, pill("neu", "◇ non richiesta: nodo non incluso (bozza)"), " ", h("span", { class: "du-mono" }, nomeFile(x.allegato_id))));
  } else {
    for (const a of arr(v.voce && v.voce.associazioni_proposte)) righe.push(h("li", { "data-du-doc": "proposto" }, pill("prop", "◇ proposto"), " ", h("span", { class: "du-mono" }, nomeFile(a.allegato_id)), a.motivo ? " · " + a.motivo : ""));
  }
  s.append(righe.length ? h("ul", { class: "du-elenco-p", id: "du-doc-nodo" }, righe) : h("p", { class: "du-muto du-piccolo", id: "du-doc-nodo" }, v.tipo === "commerciale" ? "2D non richiesto: nessun documento." : "Nessun documento."));
  if (g.modo === "bozza" && v.nodo && !v.noninc) s.append(sezioneDocumenti(g, v));
  // fabbisogni e diagnostiche del registro della bozza, cioè dell'importazione (EM §3.8): la completezza di adesso con i
  // nove stati del 2D e le diagnostiche correnti vogliono campi per nodo di /dati che il modello non dà ancora (MAT PN-06,
  // PN-07: richiesta alla corsia motore nel resoconto)
  const voce = v.voce;
  const fab = arr(voce && voce.fabbisogni_previsti_all_importazione);
  const dia = arr(voce && voce.diagnostiche);
  s.append(h("dl", { class: "du-dl", id: "du-completezza" },
    riga("2D previsti all'importazione", fab.length ? fab.map((x) => (x.tipo_documento || "documento") + (x.bloccante ? " (bloccante)" : "")).join(", ") : voce ? "nessuno" : ""),
    riga("Completezza del 2D di adesso", "non disponibile in questa pagina: il modello non dà ancora la completezza per nodo"),
    riga("Diagnostiche del Motore A", dia.length ? dia.map((x) => (x.codice || "") + (x.messaggio ? ": " + x.messaggio : "")).join("; ") : voce ? "nessuna all'importazione" : "")));
  // le azioni sui documenti che V1 non ha (MAT §6.1, EL-09): visibili e spente, con il motivo accanto (D23:294)
  s.append(h("div", { class: "du-azioni du-non-v1", id: "du-doc-non-v1" },
    btn("Conferma il documento", null, { cls: "du-btn--sm", no: NON_V1.documento }),
    btn("Conferma 2D", null, { cls: "du-btn--sm", no: NON_V1.documento }),
    btn("Correggi il codice di un file…", null, { cls: "du-btn--sm", no: NON_V1.r10 }),
    btn("Deroga", null, { cls: "du-btn--sm", no: NON_V1.deroga }),
    btn("Revoca la deroga", null, { cls: "du-btn--sm", no: NON_V1.deroga }),
    btn("Sostituisci il principale con questo", null, { cls: "du-btn--sm", no: NON_V1.sostituisci })));
  return s;
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
    // una voce con un componente registrato (decisione_esistente) tornerebbe «registrato» con l'identità della
    // certificazione, che il registro non porta: il browser non la inventa (PE-10, come «Importa dalla proposta»)
    s.append(btn("Includi di nuovo", () => includiDiNuovo(v), { cls: "du-btn--sm", id: "du-includi", no: no || (voceRegistrata(v.voce) ? MOTIVO_VOCE_REGISTRATA : "") }));
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
  s.append(btn("⤴ Ricolloca i figli…", () => apriRicollocaFigli(v.k), { cls: "du-btn--sm", id: "du-ricolloca", no: no || (figliA(g, v.k).length ? "" : "il pezzo non ha figli") }));
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
  const no = perchéNoModifica();
  const giaPb = new Set(S.lavoro.documenti_preassociati.map((d) => d.proposta_id));
  const liberi = fileSorgente().filter((f) => (!f.stato || f.stato === "attivo") && f.proposta_id && !giaPb.has(f.proposta_id) && !(f.proposta && f.proposta.componente_id) && !/^(stp|step)$/i.test(f.estensione || ""));
  const sel = h("select", { id: "du-pannello-file", "aria-label": "File da smistare" }, h("option", { value: "" }, "— scegli fra i file da smistare —"), liberi.map((f) => h("option", { value: f.proposta_id }, f.nome_file)));
  return h("div", { id: "du-doc-preassocia", style: "display:grid;gap:6px" }, h("b", { class: "du-piccolo" }, "Preassocia un file da smistare"),
    h("div", { class: "du-azioni" }, sel,
      btn("Preassocia", () => {
        const pid = $("#du-pannello-file") && $("#du-pannello-file").value;
        if (!pid) return avvisa("no", "Scegli prima un file da smistare.");
        preassocia(pid, v.k);
      }, { cls: "du-btn--sm", id: "du-pannello-preassocia", no: no || (liberi.length ? "" : "nessun file da smistare"), perche: false }),
      btn("Carica…", () => {
        S.passo = "sorgente";
        S.cartella = "attivi";
        render();
        const c = $("#du-carica-file");
        if (c) c.focus();
      }, { cls: "du-btn--sm du-btn--ghost", id: "du-pannello-carica", perche: false })),
    h("p", { class: "du-muto du-piccolo" }, "Il caricamento non preassocia: preassociare è un gesto dopo."));
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
      " · Verificata (calcolo di oggi): " + (x.verificato ? "sì" : "no") + (arr(x.motivi).length ? " (" + arr(x.motivi).map((mm) => (mm && (mm.codice || mm.motivo)) || mm).join(", ") + ")" : ""))))),
  [{ testo: "Chiudi", chiudi: true }]);
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
    // il pezzo nuovo è il nodo scelto: il pannello lo mostra già nel disegno che fai() rifà
    const prima = S.nodo;
    S.nodo = k;
    if (!fai("aggiunto " + (codice || "un pezzo senza codice"), [
      { azione: "nodo_aggiunto", chiave_bozza: k, prima: null, dopo: nodo, origine: "utente" },
      { azione: "arco_aggiunto", chiave_bozza: k, prima: null, dopo: arco, origine: "utente" }])) S.nodo = prima;
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
    const prima = S.nodo;
    S.nodo = "";
    if (!fai("tolto " + (v.cod || "un pezzo"), azioni)) S.nodo = prima;
  } }]);
}
// «Escludi (senza cascata)…» di un registrato (MAT BZ-02; §4.3 riga 3). Le sue preassociazioni vanno nel vassoio come per
// un nodo tolto (riga 2: file_svincolato con il motivo nodo_tolto, il solo dei motivi di EM §3.7 per un nodo che esce dalla
// struttura: proposta di emendamento della corsia per un motivo «escluso»). Per le associazioni registrate (AR) la scelta
// è obbligatoria e senza preselezione: «Mantieni le associazioni» (il riepilogo avvisa ESCLUSO_CON_DOCUMENTI) oppure
// «Scollega alla conferma», uno svincolo (decisione scollega_documento) per AR. I documenti confermati non si toccano: si
// elencano. Gli svincoli nati dall'esclusione hanno l'id dell'esclusione come prefisso, così «↺ Ripristina» li ritrova
// anche dopo una rilettura (riga 4).
function apriEscludi(v) {
  const doc = documentiNodo(v);
  const ar = doc.ar.filter((f) => !f.documento && !svincoloDi(f.proposta_id));
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
    confermati.length ? h("p", { class: "du-nota", id: "f-escludi-confermati" }, "Documenti confermati (non si toccano da qui): " + confermati.map((f) => f.nome_file).join(", ") + ".") : null,
    campo("Motivo", area("f-motivo", "perché esce dalla BOM"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Escludi nella bozza", danger: true, id: "f-ok", spento: () => {
    const fin = $("#du-finestra");
    if (ar.length && !fin.querySelector("input[name=f-ar]:checked")) return "scegli come gestire le associazioni registrate";
    return valore(fin, "f-motivo") ? "" : "scrivi il motivo";
  }, fai: () => {
    const fin = $("#du-finestra");
    const motivo = valore(fin, "f-motivo");
    const scollega = ar.length && fin.querySelector("input[name=f-ar]:checked").value === "scollega";
    const dec = { id: nuovoIdDecisione("es"), tipo: "escludi", componente_id: v.nodo.componente_id, motivo };
    const azioni = [];
    for (const d of pb) azioni.push({ azione: "file_svincolato", chiave_bozza: v.k, prima: clona(d), dopo: { proposta_id: d.proposta_id, motivo: "nodo_tolto" }, origine: "utente" });
    azioni.push({ azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: v.nodo.op }, dopo: { op: "esclude" }, origine: "utente" },
      { azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null, dopo: dec, origine: "utente" });
    if (scollega) ar.forEach((f, i) => azioni.push({ azione: "decisione_aggiunta", chiave_bozza: v.k, prima: null,
      dopo: { id: dec.id + ".sv" + (i + 1), tipo: "scollega_documento", proposta_id: f.proposta_id, componente_id: v.nodo.componente_id, motivo: "escluso dalla BOM: " + motivo }, origine: "utente" }));
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
// «↺ Ripristina» (MAT BZ-04; §4.3 riga 4): il nodo torna con l'op di prima, l'esclusione e i suoi svincoli si tolgono
// (decisione_tolta), e le preassociazioni andate nel vassoio per quel nodo tornano, con eventi espliciti (file_preassociato).
function ripristina(v) {
  const dec = S.lavoro.decisioni.find((d) => d.tipo === "escludi" && d.componente_id === v.nodo.componente_id);
  const azioni = [{ azione: "nodo_modificato", chiave_bozza: v.k, prima: { op: "esclude" }, dopo: { op: opPrimaDiEscludi(v.k) }, origine: "utente" }];
  if (dec) {
    azioni.push({ azione: "decisione_tolta", chiave_bozza: v.k, prima: clona(dec), dopo: null, origine: "utente" });
    for (const sv0 of S.lavoro.decisioni.filter((d) => d.tipo === "scollega_documento" && String(d.id).indexOf(dec.id + ".sv") === 0)) {
      azioni.push({ azione: "decisione_tolta", chiave_bozza: v.k, prima: clona(sv0), dopo: null, origine: "utente" });
    }
  }
  let tornano = 0, restano = 0;
  for (const x of vassoioCorrente().filter((y) => y.chiave_bozza === v.k && y.motivo === "nodo_tolto")) {
    const f = fileDiProposta(x.proposta_id);
    if (f && f.stato && f.stato !== "attivo") {
      restano++;
      continue;
    }
    tornano++;
    azioni.push({ azione: "file_preassociato", chiave_bozza: v.k, prima: null, dopo: { proposta_id: x.proposta_id, chiave_bozza: v.k }, origine: "utente" });
  }
  if (fai("ripristinato " + v.cod, azioni) && (tornano || restano)) {
    avvisa("ok", "Ripristinato " + v.cod + ": " + tornano + " file tornano preassociati" + (restano ? "; " + restano + " restano in «Da ricollocare» perché il file non è più attivo." : "."));
  }
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
const MOTIVO_VOCE_REGISTRATA = "la voce ha un componente registrato: la sua identità registrata non è nel registro, la porta il server (proposta di emendamento della corsia)";
function nodoDaVoce(voce, k, casella) {
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
  if (voceRegistrata(voce)) return avvisa("no", "«Includi di nuovo» non parte: " + MOTIVO_VOCE_REGISTRATA + ".");
  // la chiave di prima, se il giornale o la coda la dicono: lo stesso nodo, mai la stessa chiave per un nodo diverso
  let k = "";
  const tutti = arr(S.bozza.eventi).map(eventoAzione).concat(...S.coda.map((x) => x.inviare));
  for (const a of tutti) if (a.azione === "nodo_tolto" && a.prima && casellaDi(a.prima) && casellaDi(a.prima).rif === d.casella.rif) k = a.chiave_bozza || a.prima.chiave_bozza;
  if (!k || S.lavoro.nodi.some((n) => n.chiave_bozza === k)) k = nuovaChiaveNodo();
  const nodo = nodoDaVoce(voce, k, d.casella);
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
  const prima = S.nodo;
  S.nodo = k;
  if (!fai("incluso di nuovo " + (voce.codice_proposto || "un nodo"), azioni)) S.nodo = prima;
}
// Il riquadro «Proposta dello STEP» della Revisione (mockup v3, «Proposta dello STEP e BOM registrata»; MAT BZ-15,
// BZ-16): solo per una bozza con un registro d'importazione.
function riquadroProposta() {
  if (!bozzaLavorabile() || !S.bozza.importazione) return null;
  const no = perchéNoModifica();
  return h("div", { class: "du-riq du-riq--prop", id: "du-proposta-step" },
    h("div", { class: "du-riq-testa" }, h("h3", {}, "Proposta dello STEP"),
      h("span", { class: "du-muto du-piccolo" }, "L'import porta nella bozza solo gli elementi che scegli; ripartire dalla proposta sostituisce la bozza, non la versione registrata.")),
    h("div", { class: "du-azioni" },
      btn("Importa dalla proposta…", apriImporta, { cls: "du-btn--sm", id: "du-importa", no: no || (registroBozza() ? "" : "il registro della bozza non è letto: ↻ Aggiorna") }),
      btn("Riparti dalla proposta…", apriRiparti, { cls: "du-btn--sm du-btn--ghost", id: "du-riparti", no: no || perchéNoRiparti() })));
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
    const registrato = voceRegistrata(v);
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
      if (!scelteVoci.has(v.origine.rif) || voceRegistrata(v)) continue;
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
  if (bp.gruppo_corrente.come !== "stessa_occorrenza") return "la struttura della bozza non è più proposta su questa copia: usa «Cambia lo STEP della BOM…» dalla Sorgente";
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
  // la proposta di oggi del gruppo corrente (S6): la sua impronta, e il confronto della radice con il prodotto, che
  // chiede il motivo anche qui (MAT SG-05: è una creazione con partenza «step»)
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
  const letta = (gr.radice && gr.radice.identita_proposta && gr.radice.identita_proposta.letterale) || "?";
  apriFinestra("Riparti dalla proposta", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "La bozza di adesso si chiude (resta nel registro delle bozze) e se ne apre una nuova, costruita dal server con la proposta di oggi della stessa sorgente. La versione registrata della BOM non cambia."),
    h("p", { class: "du-nota" }, "Niente della bozza di adesso passa alla nuova; per tenere il lavoro usa «Aggiorna alla proposta di oggi». Le preassociazioni non passano alla bozza nuova."),
    diverso ? h("p", { class: "du-nota", id: "f-radice-diversa" }, "Il codice letto dalla radice dello STEP (", h("span", { class: "du-mono" }, letta), ") non è quello del prodotto (",
      h("span", { class: "du-mono" }, a.codice_prodotto || codiceProdotto()), "): il motivo vale anche per questo.") : null,
    campo("Motivo", area("f-motivo", "perché riparti dalla proposta"))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Riparti dalla proposta", danger: true, id: "f-ok", spento: () => (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const motivo = valore($("#du-finestra"), "f-motivo");
    const crea = { partenza: "step", sorgente: { allegato_id: sorg.allegato_id, sha256: sorg.sha256, gruppo: bp.gruppo_corrente.chiave }, impronta_contenuto: gr.impronta_contenuto,
      abbandona: { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione, motivo } };
    if (diverso) crea.motivo = motivo;
    const r = await P.creaBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, crea });
    return dopoFormaServer(r, "Bozza ripartita dalla proposta dello STEP: quella di prima è abbandonata.");
  } }]);
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
      dec.lettura = copiaLettura(lettura);
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
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "Il rebase non parte: " + no + ".");
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
// rileggiTenendoDaParte: dopo un 409 BOZZA_VERSIONE di una forma di R2 o di R3 (MAT §5.4: «rilettura di R1»), R1 si
// rilegge; eventuali modifiche non salvate si mettono da parte, non si perdono (MAT §5.1).
async function rileggiTenendoDaParte() {
  clearTimeout(timerSalva);
  const voci = (S.invio ? S.invio.voci : []).concat(S.coda);
  S.coda = [];
  S.invio = null;
  await caricaModello();
  await caricaBozza();
  if (voci.length) S.daParte = { voci, perche: "conflitto", testo: "" };
  render();
}
// voceEffetto: un elemento di un elenco dell'effetto di S6 (o della risposta di cambia_sorgente): una chiave della bozza,
// oppure un oggetto con chiave, voce o decisione e il motivo. Il nome è il codice del nodo, se la bozza lo ha.
function voceEffetto(x) {
  const k = typeof x === "string" ? x : ogg(x) ? x.chiave_bozza || x.decisione_id || x.voce || "" : String(x);
  const n = S.lavoro && S.lavoro.nodi.find((y) => y.chiave_bozza === k);
  const nome = (n && n.identita && n.identita.codice) || (n && n.descrizione) || k;
  return nome + (nome !== k && k ? " (" + k + ")" : "") + (ogg(x) && x.motivo ? " · " + x.motivo : "");
}
// «Cambia lo STEP della BOM…» (MAT SG-04; §1.5): prima si salvano le modifiche, poi si rilegge S6 con bozza= sulla
// versione di adesso, perché l'effetto che il dialogo mostra (e a cui chi guarda acconsente) sia quello della bozza che
// cambierà, con gli elenchi dei nodi.
async function apriCambiaSorgente(a, gr) {
  const no = perchéNoModifica();
  if (no) return avvisa("errore", "Il cambio dello STEP non parte: " + no + ".");
  if (S.coda.length || S.invio) {
    const ok = await salvaOra();
    if (!ok) return avvisa("errore", "Il cambio dello STEP non parte: le modifiche della bozza non sono salvate.");
  }
  let a2;
  try {
    a2 = await P.anteprimaStep(a.allegato_id, S.prodotto, { gruppo: gr.gruppo, bozza: S.bozza.bozza_id });
  } catch (err) {
    return avvisa("errore", "L'effetto del cambio sulla bozza non si legge: " + testoErrore(err) + ". Il cambio non parte.");
  }
  if (a2.errore) return avvisa("errore", "L'effetto del cambio non si calcola: " + a2.errore + ". Il cambio non parte.");
  const gr2 = arr(a2.gruppi).find((x) => x.gruppo === gr.gruppo) || gr;
  const e = a2.effetto_sulla_bozza || {};
  if (e.stessa_sorgente) return avvisa("no", "È la struttura della bozza: per prendere la proposta di oggi usa «Aggiorna alla proposta di oggi».");
  const versione = S.bozza.versione;
  const elenco = (t, x) => (arr(x).length ? h("li", {}, t + ": " + arr(x).length + " — " + arr(x).map(voceEffetto).join(", ")) : null);
  apriFinestra("Cambia lo STEP della BOM", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "L'effetto sulla bozza (versione " + versione + "), calcolato dal server adesso, prima del gesto:"),
    h("ul", { class: "du-elenco-p", id: "f-effetto" }, elenco("nodi che restano", e.nodi_che_restano), elenco("nodi che escono", e.fuori_per_cambio_fonte), elenco("nodi che entrano", e.nodi_che_entrano),
      elenco("caselle riscritte su un'altra copia", e.caselle_riscritte), elenco("archi riscritti", e.archi_riscritti), elenco("identità aggiornate", e.identita_aggiornate),
      elenco("da rivedere", e.da_rivedere), elenco("decisioni che cadono", e.decisioni_cadute), elenco("preassociazioni nel vassoio", e.preassociazioni_nel_vassoio), elenco("voci nuove proposte (non entrano da sole)", e.nuovi_nodi_proposti)),
    campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Cambia lo STEP", pri: true, id: "f-ok", spento: () => (S.bozza && S.bozza.versione !== versione ? "la bozza è cambiata: riapri il dialogo" : valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const r = await P.cambiaSorgente(corpoForma({ cambia_sorgente: { sorgente: sorgenteDi(a2, gr2), impronta_contenuto: gr2.impronta_contenuto, motivo: valore($("#du-finestra"), "f-motivo") } }));
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
  const no = perchéNoFormaBozza();
  if (no) return avvisa("errore", "La conversione non parte: " + no + ".");
  apriFinestra("Converti la bozza", h("div", { style: "display:grid;gap:8px" }, h("p", {}, "La bozza di prima si chiude e se ne apre una nuova con la stessa struttura; i codici scritti restano come decisioni dell'ingegnere."),
    campo("Motivo", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Converti", pri: true, id: "f-ok", spento: () => perchéNoFormaBozza() || (valore($("#du-finestra"), "f-motivo") ? "" : "scrivi il motivo"), fai: async () => {
    const r = await P.convertiBozza({ bozza_id: null, versione_attesa: 0, prodotto_id: S.prodotto, vocabolario: 2, converti_da: { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione, motivo: valore($("#du-finestra"), "f-motivo") } });
    return dopoFormaServer(r, "Bozza convertita.");
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
    perse.length ? h("p", { class: "du-nota", id: "f-perse" }, "Queste modifiche non salvate si perdono con la bozza: " + perse.join("; ") + ".") : null,
    campo("Motivo (facoltativo)", area("f-motivo", ""))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Abbandona la bozza", danger: true, id: "f-ok", spento: () => perchéNoFormaBozza(), fai: async () => {
    clearTimeout(timerSalva);
    const motivo = valore($("#du-finestra"), "f-motivo");
    const prodotto = S.prodotto;
    const r = await inSerie("bozza:" + prodotto, async () => {
      await attendi(() => !S.invio, 30000);
      const corpo = { versione_attesa: S.bozza.versione };
      if (motivo) corpo.motivo = motivo;
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
// «Riprova» di un gesto dei file con l'esito incerto (S2, S3, S4, S7), dalla striscia: il ponte rimanda il corpo di
// allora con la stessa chiave, che tiene nella memoria della scheda anche dopo una ricarica (EM §10.9, «Reinvio e
// risposta persa»; MAT §5.4, righe S2-S4 e S7: «come R5»; §5.5 n. 4).
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
  GIA_DOCUMENTO: "È già un documento confermato: per cambiarlo serve una revisione o una sostituzione (non in V1).",
  NON_STEP: "Il file non è uno STEP: l'associazione al prodotto vale solo per lo STEP.",
  FILE_NON_ATTIVO: "Il file non è fra i file attivi della RFQ: riattivalo prima.",
  MOTIVO_OBBLIGATORIO: "Serve il motivo.",
  RISOLUZIONE_RICHIESTA: "Manca una dichiarazione richiesta dalle dipendenze (l'unica copia o «Scollega»): rivedi le caselle del dialogo.",
  FILE_SENZA_BYTE: "Il file non ha ancora i byte (non scaricato): niente è scritto, si può riprovare dopo «Prepara i file».",
  FILE_SCARTATO: "Il file è stato scartato: per riaverlo, ricaricalo.",
  CONSERVAZIONE_NON_RIUSCITA: "La copia di conservazione non è riuscita: niente è cambiato.",
  CONSERVAZIONE_MANCANTE: "La copia di conservazione non c'è o non torna con lo SHA: niente è cambiato (anomalia da segnalare).",
  RFQ_UNITA: "La RFQ è unita a un'altra: i gesti sui file si fanno sulla RFQ che la contiene.",
  NON_CONFIGURATO: "La conservazione degli Altri file non è configurata su questo server: niente è cambiato.",
};
// esitoGestoFile: l'esito di S2, S3, S4 o S7 (MAT §5.4; EM §10.9). o.chiudi: il dialogo non c'è (un «Riprova» dalla
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
    const r = await inSerie("file:" + f.allegato_id, () => P.scartaFile(f.allegato_id, corpo));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, apriScarta);
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
    const r = await inSerie("file:" + f.allegato_id, () => P.altriFile(f.allegato_id, corpo));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, apriAltriFile);
    return esitoGestoFile(r, f, "File messo in Altri file: il contenuto è conservato.");
  } }]);
}
// «Riattiva» (MAT FL-07, FL-10): per un archivio il dialogo dice quali voci tornano attive con lui, e S4 porta n_voci e
// impronta_voci dell'elenco che S1 ha mostrato (il dettaglio con file=; EM §10.9).
async function apriRiattiva(f) {
  const det = await dettaglioFile(f);
  if (!det) return;
  const voci = arr(det.voci);
  apriFinestra("Riattivare «" + f.nome_file + "»?", h("div", { style: "display:grid;gap:8px" },
    h("p", {}, "Riportare «" + f.nome_file + "» fra i file attivi? Il motore lo rilegge e può proporlo di nuovo. Non conferma nessun documento e non autorizza nessuno STEP."),
    voci.length ? h("div", { id: "f-voci-riattiva" }, h("p", { class: "du-nota" }, "È un archivio: tornano attive con lui le sue " + voci.length + " voci."),
      h("ul", { class: "du-elenco-p" }, voci.map((v) => h("li", {}, h("span", { class: "du-mono" }, v.nome_file || nomeFile(v.allegato_id)), v.stato ? " · " + v.stato : "")))) : null),
  [{ testo: "Annulla", chiudi: true }, { testo: "Riattiva", pri: true, id: "f-ok", fai: async () => {
    const corpo = { versione_attesa: typeof f.versione === "number" ? f.versione : 0 };
    if (det.n_voci !== undefined) {
      corpo.n_voci = det.n_voci;
      corpo.impronta_voci = det.impronta_voci;
    }
    const r = await inSerie("file:" + f.allegato_id, () => P.riattivaFile(f.allegato_id, corpo));
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") return riapriDopoAnteprima(f, apriRiattiva);
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
    const r = await inSerie("file:" + a.allegato_id, () => P.associaStep(a.allegato_id, corpo));
    if (r.esito === "ok") {
      const riletta = await rileggiDopoUnGesto(["sorgente"], "Lo STEP è associato al prodotto.");
      if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
      avvisa("ok", ((r.dati && r.dati.testo) || "STEP associato al prodotto.") + (riletta ? "" : " La pagina non si è riletta: ↻ Rileggi."));
      render();
      return true;
    }
    if (r.esito === "conflitto" && r.codice === "ANTEPRIMA_SUPERATA") {
      // la proposta dello STEP è cambiata (codice o componente): il dialogo, con codice_atteso e componente_atteso di
      // prima, si chiude; si rileggono modello, S1 e S6, e il gesto si rifà dal pulsante (MAT §5.4, S7)
      chiudiFinestra();
      await caricaModello();
      await caricaSorgente();
      if (S.anteprimaAllegato) await guarda(S.anteprimaAllegato, S.anteprimaGruppo, true);
      render();
      avvisa("conflitto", "La proposta dello STEP è cambiata: rivedila e ripeti «Associa lo STEP» (ANTEPRIMA_SUPERATA).");
      return true;
    }
    return esitoGestoFile(r, f, "");
  } }]);
}

// ---------------------------------------------------------------- gli atti esistenti (lista chiusa ridotta, EM §9.10)

// atto: uno dei sei atti della lista chiusa (EM §9.10). Gli atti passano dalla coda della pagina (MAT §1.3): un atto
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

function renderBarra() {
  const box = $("#du-barra");
  if (!box) return;
  if (S.passo !== "revisione") {
    // nella Sorgente la struttura non si modifica, ma le preassociazioni dei file sì (MAT FL-12): con una bozza, la barra
    // dice anche qui lo stato del salvataggio (D23:286)
    box.replaceChildren(h("div", { class: "du-salva du-salva--sorg" }, bozzaLavorabile() ? statoSalvataggio() : null,
      h("span", { class: "du-salva-conti" }, bozzaLavorabile() ? "Sorgente: qui si preassociano i file; la struttura si modifica nella Revisione." : "Sorgente: la BOM non si modifica qui."),
      btn("Vai alla Revisione", () => vaiPasso("revisione"), { cls: "du-btn--sm", id: "du-vai-revisione", no: S.prodotto ? "" : "nessun prodotto valutato" })));
    return;
  }
  if (!bozzaLavorabile()) {
    box.replaceChildren();
    return;
  }
  const r = S.riepilogo;
  const conti = h("span", { class: "du-salva-conti", id: "du-conti" }, S.riepilogoStato === "aggiornato" && r ?
    [h("span", {}, "blocchi ", h("b", {}, String(arr(r.blocchi).length))), h("span", {}, "avvisi ", h("b", {}, String(arr(r.avvisi).length))),
      h("span", {}, "documenti irrisolti ", h("b", {}, String(arr(r.documenti_preassociati).filter((d) => d.esito && d.esito !== "applicabile").length)))] :
    h("span", {}, S.riepilogoStato === "errore" ? "riepilogo non letto: " + (S.riepilogoErrore || "") : "conti da aggiornare"));
  const noCta = perchéNoConferma();
  box.replaceChildren(h("div", { class: "du-salva", id: "du-salva" }, statoSalvataggio(), conti,
    h("span", { class: "du-azioni" },
      btn("Annulla", annulla, { cls: "du-btn--sm", id: "du-annulla", no: !S.pila.length ? "niente da annullare" : perchéNoModifica(), perche: false }),
      btn("Ripeti", ripeti, { cls: "du-btn--sm", id: "du-ripeti", no: !S.rifai.length ? "niente da ripetere" : perchéNoModifica(), perche: false }),
      btn("Abbandona la bozza…", apriAbbandona, { cls: "du-btn--sm du-btn--ghost", id: "du-abbandona", no: perchéNoFormaBozza(), perche: false })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma BOM", apriWizard, { cls: "du-btn--pri", id: "du-cta-bom", no: noCta })),
    h("span", { class: "du-cta" }, btn("Rivedi e conferma fascicolo", null, { cls: "du-btn--sm", id: "du-cta-fascicolo", no: NON_V1.fascicolo }))));
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
  const voci = (S.invio ? S.invio.voci : []).concat(S.coda);
  const perche = S.salvataggio.stato;
  const testo = S.salvataggio.testo || "";
  S.coda = [];
  S.invio = null;
  await caricaBozza();
  if (voci.length) S.daParte = { voci, perche, testo };
  render();
}
// le voci da parte che si riapplicano: i gesti. Gli annullamenti e le ripetizioni di passi già salvati (annulla{n},
// rifai{n}) no: portano i numeri degli eventi di prima, e dopo la rilettura si rifanno con «Annulla» e «Ripeti».
function vociRiapplicabili(voci) {
  return voci.filter((v) => !v.annullaDi && !/^ripeti: /.test(v.etichetta) && !v.inviare.some((a) => a.azione === "annulla" || a.azione === "rifai"));
}
function riapplicaDaParte() {
  const dp = S.daParte;
  if (!dp) return;
  if (!bozzaLavorabile()) return avvisa("errore", "Le modifiche non si riapplicano: la bozza riletta non c'è o non si lavora (" + (perchéNoModifica() || "nessuna bozza") + ").");
  S.daParte = null;
  const no = perchéNoModifica();
  if (no) {
    S.daParte = dp;
    return avvisa("errore", "Le modifiche non si riapplicano: " + no + ".");
  }
  const voci = vociRiapplicabili(dp.voci);
  for (const v of voci) {
    const voce = { etichetta: v.etichetta, inviare: v.inviare, locali: v.locali, numeri: null, rebase: v.rebase || null };
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
    h("p", {}, "Queste modifiche non sono mai state salvate e si perdono:"),
    h("ul", { class: "du-elenco-p", id: "f-da-parte" }, dp.voci.map((v) => h("li", {}, v.etichetta)))),
  [{ testo: "Annulla", chiudi: true }, { testo: "Scarta le modifiche", danger: true, id: "f-ok", fai: () => {
    S.daParte = null;
    avvisa("ok", "Modifiche non salvate scartate: la bozza è quella salvata.");
    render();
  } }]);
}
// la riga delle modifiche tenute da parte (barra della Revisione e strisce)
function rigaDaParte() {
  const dp = S.daParte;
  if (!dp) return null;
  const dopo = dp.perche === "conflitto" ? "dopo «La bozza è cambiata altrove»" : "dopo «Salvataggio rifiutato»";
  return h("div", { class: "du-striscia du-striscia--ver", id: "du-da-parte", role: "status" },
    h("span", {}, dp.voci.length + " modifiche non salvate tenute da parte " + dopo + ": " + dp.voci.map((v) => v.etichetta).join("; ") + ". Non si perdono finché la pagina è aperta."),
    h("span", { class: "du-azioni" },
      btn("Riapplica alla bozza riletta", riapplicaDaParte, { cls: "du-btn--sm du-btn--pri", id: "du-riapplica", no: bozzaLavorabile() ? "" : "la bozza riletta non c'è: si possono solo scartare", perche: false }),
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
  S.wizard = { fase: 1, stato: "lettura", gruppo: "tutti", riepilogo: null, errore: "", presaMotore: false, esito: null, dettagli: new Set(), codice: codiceProdotto(), prodotto: S.prodotto };
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
  if (!S.bozza || w.versione !== S.bozza.versione) return "la bozza è cambiata: rileggi il riepilogo";
  const nm = nessunaModifica();
  if (nm) return nm;
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
      b.chiave_bozza ? h("span", { class: "du-azioni" }, btn("Mostra nel grafo", () => mostraNelGrafo(b.chiave_bozza), { cls: "du-btn--sm du-btn--ghost", perche: false }), gestiD1(b)) : null))) : null));
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
  w.esito = { tipo: r.esito, codice: r.codice, motivo: r.motivo, testo: r.testo, dati: r.dati, chiave: r.chiave };
  if (r.codice === "ANTEPRIMA_SUPERATA" && r.dati && r.dati.riepilogo) {
    w.riepilogo = r.dati.riepilogo;
    w.versione = S.bozza.versione;
  }
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
    const nomi = new Map(arr(w.riepilogo && w.riepilogo.non_inclusi).map((x) => [x.id, x.nome_grezzo || (x.casella && x.casella.rif) || x.id]));
    const nonInc = arr(d.non_inclusi_applicati).map((x) => (typeof x === "string" ? nomi.get(x) || x : (x && (x.nome_grezzo || x.id)) || ""));
    return h("div", { class: "du-esito du-esito--ok", id: "du-wz-esito", "data-esito": "ok" }, h("b", {}, "Conferma n. " + n + " di " + (w.codice || codiceProdotto()) + (e.verificato ? " (verificata con la stessa chiave)" : "") + "."),
      h("ul", { class: "du-elenco-p" }, h("li", {}, "Identità accettate: " + arr(d.identita_accettate).length + ", corrette: " + arr(d.identita_corrette).length),
        h("li", {}, "Non inclusi applicati: " + arr(d.non_inclusi_applicati).length), h("li", {}, "Associazioni applicate: " + arr(d.associazioni_applicate).length + ", svincoli: " + arr(d.svincoli_applicati).length),
        h("li", {}, "Relazioni registrate come non ratificate: " + arr(d.relazioni_non_ratificate).length)),
      nonInc.length ? h("div", { id: "du-wz-noninclusi-conferma" }, h("b", {}, "Nodi non inclusi nella conferma n. " + n + ":"), h("ul", { class: "du-elenco-p" }, nonInc.map((x) => h("li", {}, x))),
        h("p", { class: "du-nota" }, "Per ripristinarne uno serve una bozza nuova («Modifica la BOM confermata», poi «Includi di nuovo»).")) : null,
      h("p", {}, pill("ok", "v" + n + " confermata"), " ", pill("neu", "Verificata: non ancora calcolata (arriva in V2)")),
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
    ANTEPRIMA_SUPERATA: "Il riepilogo è cambiato: rileggilo e conferma di nuovo (firma nuova, chiave nuova).",
    BOZZA_VERSIONE: "La bozza salvata non è quella che stai confermando: la rileggo.",
    COPIA_IN_CORSO: "Una copia sul NAS è in corso: riprova fra poco con la stessa chiave. Niente è registrato.",
    CHIAVE_RIUSATA: "Questo tentativo ha già un esito diverso: ho chiesto l'esito con la chiave e riletto bozza e storico; la prossima conferma userà una chiave nuova.",
    BLOCCHI: "Niente è registrato, la bozza resta: risolvi i blocchi.",
    CONGELATA: "La BOM è congelata: niente è registrato.",
    PONTE_TENTATIVO_IN_SOSPESO: "La conferma di prima non ha un esito certo e questo corpo è diverso: niente è partito. Verifica il tentativo, o abbandonalo dalla striscia in alto, poi conferma di nuovo.",
  };
  const blocchi = arr(e.dati && e.dati.blocchi);
  // i blocchi con i loro rimedi (MAT §5.2, «rifiutata per blocchi»)
  return h("div", { class: "du-esito du-esito--ko", id: "du-wz-esito", "data-esito": e.tipo, "data-codice": e.codice || "" }, h("b", {}, (testi[e.codice] || e.testo || "La conferma non è riuscita.") + (e.codice ? " (" + e.codice + ")" : "")),
    e.codice === "CHIAVE_RIUSATA" && e.verifica ? h("p", { class: "du-nota" }, "Esito di quella chiave: " + e.verifica + ".") : null,
    blocchi.length ? h("ul", { class: "du-elenco-p" }, blocchi.map((b) => h("li", { "data-codice": b.codice }, b.codice + ": " + (b.testo || ""), arr(b.rimedi).length ? h("span", { class: "du-muto" }, " Rimedi: " + arr(b.rimedi).join("; ")) : null))) : null,
    e.codice === "COPIA_IN_CORSO" ? btn("Riprova", riprovaConferma, { cls: "du-btn--sm du-btn--pri", id: "du-wz-riprova-copia", perche: false }) : null,
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
  const r = await P.confermaBom(w.firma, w.corpo);
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
