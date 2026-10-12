/* ================= 40 · il guscio: elenco delle RFQ, testata e barra delle stazioni della Distinta =================
   SPEC §2.0. Testata di una riga (48 px) e una sola barra delle stazioni (40 px): il guscio fisso è 88 px (C1).
   Fase registrata, stato del thread e stato del prodotto stanno in TRE celle diverse (EB7-3); lo stato del prodotto
   compare SOLO nel menu dei prodotti (nav#ma-prodotti). Nessun contatore si calcola nella vista: la stazione ① legge
   BOM.Verificata e Documenti.Stato dal finto server A; le altre stazioni leggeranno il finto server A2 (giri 3-5). */

/* ---------------- l'elenco delle RFQ: per fase registrata e stato del thread, letti (si toglie gruppoDi, n. 19) ---------------- */
function filtroRFQ(q){
  const d=datiRFQ(q), c=cl(d.cli);
  if(S.fcli&&d.cli!==S.fcli) return false;
  if(!S.q) return true;
  const hay=[q.num,c.nome,d.titolo,d.buyer,...d.prodotti.map(p=>PROD[p].cod+" "+PROD[p].nome)].join(" ").toLowerCase();
  return hay.includes(S.q.toLowerCase());
}
const faseRegistrata = q => { const t=THREADS[chiaveThread(q)]; return t&&t.testa.fase ? t.testa.fase.nome : null; };
const threadChiuso = q => { const t=THREADS[chiaveThread(q)]; return !!(t&&t.testa.stato_thread==="CHIUSA"); };
function gruppoLetto(q){ return threadChiuso(q) ? "chiuse" : (faseRegistrata(q)||"nessuna"); }
function gestione(){
  const clientiRFQ=[...new Set(RFQ.map(q=>datiRFQ(q).cli))];
  return `<div class="gest">
    <div class="ghead"><div title="Raggruppate per fase registrata e stato del thread, come li dà il server"><h2>Richieste RFQ</h2></div>
      <span class="sp"></span>
      <div class="search"><span class="ic">${I("search",15)}</span><input id="gq" value="${esc(S.q)}" placeholder="Cerca numero, cliente, codice, buyer" aria-label="Cerca RFQ"></div>
      <select id="gcli" aria-label="Filtra per cliente"><option value="">Tutti i clienti</option>
        ${clientiRFQ.map(id=>`<option value="${id}" ${S.fcli===id?"selected":""}>${esc(cl(id).nome)}</option>`).join("")}</select></div>
    <div class="gfasi" id="gstati" role="group" aria-label="Fase registrata">${gstati()}</div>
    <div class="gbody" id="gbody">${gbody()}</div></div>`;
}
function gstati(){
  const vis = RFQ.filter(filtroRFQ);
  const gruppi = [...new Set(RFQ.map(gruppoLetto))];
  const nome = g => g==="chiuse" ? "Thread chiusi" : g==="nessuna" ? "Nessuna fase aperta" : (FASE_CATALOGO[g]||etichetta("fase",g));
  return `<button class="gfase" data-gfase="tutte" aria-pressed="${S.gfase==="tutte"}">Tutte <span class="cnt">${vis.length}</span></button>`+
    gruppi.map(g=>`<button class="gfase" data-gfase="${g}" aria-pressed="${S.gfase===g}">${esc(nome(g))} <span class="cnt">${vis.filter(q=>gruppoLetto(q)===g).length}</span></button>`).join("");
}
function gbody(){
  const rows=RFQ.filter(q=>(S.gfase==="tutte"||gruppoLetto(q)===S.gfase)&&filtroRFQ(q));
  const senza=RICHIESTE.filter(r=>!r.rfq&&cl(r.cli).attivo&&(!S.fcli||r.cli===S.fcli));
  const blocco = senza.length&&(S.gfase==="tutte")?`<div class="senza">
    <div style="display:flex;align-items:center;gap:8px">${I("tray",16)}<b>${plur(senza.length,"richiesta","richieste")} nell’Inbox senza RFQ</b></div>
    <span class="k3 piccolo">crea l’RFQ quando sei pronto a lavorarla</span>
    ${senza.map(r=>{const c=cl(r.cli); return `<div class="riga">
      <span class="av lg" style="background:${c.col}">${c.sigla}</span>
      <span style="min-width:0"><b style="font-weight:500">${esc(r.titolo)}</b>
        <span class="k3 piccolo" style="display:block">${esc(c.nome)} · ${r.prodotti.map(p=>PROD[p].cod).join(", ")} · ricevuta ${esc(r.aperta)}</span></span>
      <span class="acts"><button class="btn sm" data-req="${r.id}">${I("inbox",13)} Conversazione</button>
        <button class="btn sm" data-crearfq="${r.id}">${I("plus",13)} Crea l’RFQ</button></span></div>`;}).join("")}</div>`:"";
  const R=rows.map(q=>({q,d:datiRFQ(q),c:cl(datiRFQ(q).cli),T:THREADS[chiaveThread(q)]}));
  const elenco = rows.length ? `<div class="wrap-tb desk"><table class="tb"><thead><tr>
      <th>RFQ</th><th>Cliente</th><th>Richiesta</th><th>Prodotti e stato</th><th>Fase registrata</th><th>Thread</th><th>Scadenza</th><th>Resp.</th></tr></thead>
    <tbody>${R.map(({q,d,c,T})=>`<tr class="click" data-apririfq="${q.id}">
      <td class="mono" style="font-weight:600;color:var(--acc);white-space:nowrap">${esc(q.num)}</td>
      <td><span style="display:flex;align-items:center;gap:8px"><span class="av" style="background:${c.col}">${c.sigla}</span><span style="white-space:nowrap">${esc(c.nome)}</span></span></td>
      <td>${esc(d.titolo)}${q.nota?`<span class="k3" style="display:block;font-size:var(--t-xs)">${esc(q.nota)}</span>`:""}</td>
      <td>${statiProdotti(T)}</td>
      <td>${chipFase(q)}</td><td>${chipThread(q)}</td>
      <td class="mono" style="font-size:var(--t-sm);white-space:nowrap">${esc(d.scad)}</td><td>${esc(q.resp)}</td></tr>`).join("")}</tbody></table></div>
    <div class="cards">${R.map(({q,d,c,T})=>`<button class="rcard" data-apririfq="${q.id}">
      <span style="display:flex;align-items:center;gap:8px;flex-wrap:wrap"><span class="av" style="background:${c.col}">${c.sigla}</span>
        <span class="mono" style="font-weight:600;color:var(--acc)">${esc(q.num)}</span><span class="sp"></span>${chipFase(q)}</span>
      <span><b style="font-weight:500">${esc(d.titolo)}</b></span>
      <span class="k3" style="font-size:var(--t-sm)">${esc(c.nome)} · ${esc(q.resp)}</span><span>${statiProdotti(T)}</span></button>`).join("")}</div>`
    : vuoto("file","Nessuna RFQ con questi filtri","Cambia la fase o la ricerca.");
  return `<div class="gwrap${blocco?" con-senza":""}"><div class="gmain">${elenco}</div>${blocco}</div>`;
}

/* ---------------- creare e aprire l'RFQ ---------------- */
function creaRFQ(rid){
  const r=rq(rid);
  if(r.rfq){ apriRFQ(r.rfq); return; }                       /* esiste già: si riapre, niente doppioni */
  const c=cl(r.cli), [g,mm,aa]=r.aperta.split("/"), cogn=r.buyer.split(" ").pop();
  const num=r.numCli||("P-2026-0"+(++contatoreRFQ));
  const q={id:"q-"+rid,num,numProvvisorio:!r.numCli,req:rid,fase:"RICEVUTA",resp:"Franco",agg:"adesso",
    nas:`${c.nas}\\WIP\\${aa} ${mm} ${g} ${cogn} ${r.titolo.replace(/[\\/:*?"<>|]/g," ").replace(/\s+/g," ").trim()}`};
  RFQ.push(q); r.rfq=q.id;
  /* la creazione dell'RFQ è un gesto registrato dell'Inbox (AM): la fase RICEVUTA la scrive lui, non la Distinta */
  const T = threadDiRichiesta(r); T.rfq = q.id; if(!T.testa.fase) T.testa.fase = {nome:"RICEVUTA", etichetta:FASE_CATALOGO.RICEVUTA, inizio:OGGI_ISO+"T09:00:00+02:00", gg_in_fase:0, semaforo:"verde", terminale:false};
  apriRFQ(q.id); S.creato=q.id;
}
/* aprire la Distinta non scrive niente (DB7-6): nessun prepara, analisi o smistamento */
function apriRFQ(id){
  const q = rf(id);
  S.tab="rfq"; S.rfq=id; S.staz=1; S.vista="documenti";   /* v16 §2.1: si entra sui Documenti (valore di vista che esiste già) */
  S.casella=null; S.scheda=null; S.cassetto=null; S.dialogo=null; S.menu=null; S.creato=null; S.zoom=1;
  threadCorrenteKey = chiaveThread(q);
  if(!THREADS[threadCorrenteKey] && q.req) threadDiRichiesta(rq(q.req));
  const T = threadAperto();
  S.prodotto = T && T.prodotti[0] ? T.prodotti[0].rif : null;
  S.fonteAperta = null; PONTE.avviso = null;
  /* la zona B riparte pulita: nessuna riga, fase o lotto scelto al posto di chi guarda (EB7-1) */
  Object.assign(S, {comeFiltro:null, cicloFiltro:null, cicloCid:null, cicloRif:null, faseAperta:undefined, comeRiga:null, comeSel:new Set(), cicloSel:new Set(), comeForm:null, menuMod:null, blocco:null, bcMenu:false,
    copiaDa:false, disCiclo:null, seqScelto:null, seqFiltro:null, seqRiga:null, esitoVista:null, esitoRiga:null, esitoAnteprima:false, foto:null, lottiSel:new Set(), rqSel:new Set(), rqSu:null, catDest:null, aggFase:"", giuntoTesto:""});
  if(typeof PA!=="undefined") PA.avviso = null; if(typeof OF!=="undefined") OF.msg = null;
}

/* ---------------- il «?» unico per stazione (SPEC_v16 §8.3) ----------------
   AIUTO_STAZ[n] = {tasti:[[tasto, che cosa],…], testi:[stringa,…], richieste:"R-xx …"}. C2 scrive [3] e [4] dentro i suoi file. */
const AIUTO_STAZ = {};

/* ---------------- la pagina della Distinta ---------------- */
const STAZIONI = [
  [1,"Struttura e documenti","zona A · contratto A1d"],
  [2,"Come","proposta · dopo A2 · R-22 R-39"],
  [3,"Ciclo","proposta · dopo A2 · R-10 R-25"],
  [4,"Sequenza","proposta · dopo A2 · R-32"],
  [5,"Esito → Franco","proposta · dopo A2 · D-01 R-40"],
];
function prodottoScelto(T){ return T ? (T.esito.prodotti_valutati.find(p=>p.rif===S.prodotto) || T.esito.prodotti_valutati[0] || null) : null; }

function paginaDistinta(){
  const q = rf(S.rfq), T = threadAperto();
  if(!q || !T) return vuoto("file","RFQ non trovata","Torna a «Richieste RFQ».");
  const p = prodottoScelto(T);
  if(p && S.prodotto!==p.rif) S.prodotto = p.rif;
  const corpo = S.staz===1 ? stazioneStruttura(T, p) : S.staz===2 ? stazioneCome(T, p) : S.staz===3 ? stazioneCiclo(T, p) : S.staz===4 ? stazioneSequenza(T, p) : stazioneEsito(T, p);
  return `<div class="ws${S.cassetto?" ws--cassetto":""}" data-stazione="${S.staz}">
    ${testataDistinta(q, T, p)}
    ${barraStazioni(T, p)}
    <div class="ws-corpo">
      <main class="ws-main" id="ws-main" aria-label="${esc(STAZIONI[S.staz-1][1])}">${corpo}</main>
      ${S.cassetto?cassetto(T,p):""}
    </div>
    ${S.dialogo?finestra(T,p):""}
  </div>`;
}

/* la testata: una riga, celle separate da filetti */
function testataDistinta(q, T, p){
  const d = datiRFQ(q), c = cl(d.cli), t = T.testa, f = t.fase, e = T.esito;
  const fase = f ? `<span class="tt-fase" title="fase = ${esc(f.nome)} dal ${esc((f.inizio||"").slice(0,10))} · semaforo: ${esc(etichetta("semaforo",f.semaforo))}. La fase cambia solo con un gesto registrato.">
      <i class="semaforo semaforo--${esc(f.semaforo)}" aria-hidden="true"></i><b>${esc(f.etichetta||etichetta("fase",f.nome))}</b><span class="tt-gg">${f.gg_in_fase} gg</span><span class="vh">semaforo ${esc(etichetta("semaforo",f.semaforo))}</span></span>`
    : `<span class="ma-st ma-st--neu">nessuna fase aperta</span>`;
  const prodotti = e.prodotti_valutati;
  return `<header class="tt" id="ma-testa">
    <h1 class="vh">Distinta della RFQ ${esc(t.riferimento_cliente||q.num)} · ${esc(t.cliente)}</h1>
    <button type="button" class="tt-menu" data-menu="app" aria-expanded="${S.menu==="app"}" aria-label="Menu del Cockpit">${I("list",18)}</button>
    <button type="button" class="tt-rfq" data-menu="cartiglio" aria-expanded="${S.menu==="cartiglio"}" title="${esc(d.titolo)}">
      <span class="tt-num mono">${esc(q.num)}</span><span class="tt-cli">${esc(c.nome)}</span><span class="tt-ogg">${esc(PROD[d.prodotti[0]]?PROD[d.prodotti[0]].nome:d.titolo)}</span><span aria-hidden="true" class="tt-ch">▾</span></button>
    <span class="tt-b" title="Proposta · dopo A2: tipo di pratica (R-19) e lotti (R-18) non sono ancora nel backend">
      <span class="pa-segno" aria-hidden="true">◇</span><span>${q.id==="q-sv1"?"ordine diretto":"richiesta d'offerta"}</span><span class="tt-sep">·</span><span>scad. ${esc(d.scad)}</span></span>
    <span class="tt-cella" aria-label="Fase registrata"><span class="lab">Fase</span>${fase}</span>
    <span class="tt-cella" aria-label="Stato del thread"><span class="lab">Thread</span><span class="ma-st ma-st--neu ma-st--piatto tt-thread" title="stato_thread = ${esc(t.stato_thread)}">${esc(etichetta("stato_thread",t.stato_thread))}</span></span>
    <nav class="tt-cella tt-prodotti" id="ma-prodotti" aria-label="Prodotti della RFQ">
      <button type="button" class="tt-prod" data-menu="prodotti" aria-expanded="${S.menu==="prodotti"}" title="Lo stato del prodotto, come lo dà il server">
        <span class="lab">Prodotto${prodotti.length>1?` 1 di ${prodotti.length}`:""}</span>
        <span class="tt-prod-v"><span class="mono">${esc(p?p.codice_richiesto:"—")}</span>${p?st("stato_prodotto",p.stato):""}</span><span aria-hidden="true" class="tt-ch">▾</span></button>
      ${S.menu==="prodotti"?`<div class="tendina tendina--prodotti" role="menu">${prodotti.map(x=>`<button type="button" role="menuitemradio" aria-checked="${x.rif===S.prodotto}" class="tendina-voce" data-prodotto="${esc(x.rif)}">
        <span class="mono">${esc(x.codice_richiesto)}</span>${st("stato_prodotto",x.stato)}${x.prodotto_verificato?`<span class="ma-st ma-st--ok">verificato</span>`:`<span class="ma-st ma-st--ver">non verificato</span>`}
        <small>${x.motivi.length?x.motivi.map(m=>esc(etichetta("motivo_prodotto",m))).join(" · "):"nessun motivo"}</small></button>`).join("")}</div>`:""}
    </nav>
    <span class="sp"></span>
    <span class="tt-ruolo" title="Ruolo simulato del mockup: si cambia dal menu ≡ → «Vedi come»">${esc(S.ruolo)} <span class="chip pa">simulato</span></span>
    ${S.menu==="ruolo"?`<div class="tendina tendina--ruolo" role="menu" aria-label="Vedi come">${["Lino","Franco","Fabio","Marta"].map(r=>`<button type="button" role="menuitemradio" aria-checked="${S.ruolo===r}" class="tendina-voce" data-ruolo="${r}">${r} <small>${{Lino:"ufficio tecnico",Franco:"commerciale",Fabio:"acquisti",Marta:"produzione e gestionale"}[r]}</small></button>`).join("")}
      <p class="tendina-nota">Ruolo simulato: A1d chiede solo il ruolo operatore. I ruoli servono alle proposte della zona B (R-27, R-33).</p></div>`:""}
    ${S.menu==="app"?`<div class="tendina tendina--app" role="menu">
      <button type="button" class="tendina-voce" data-tab="inbox">${I("inbox",15)} Inbox</button>
      <button type="button" class="tendina-voce" data-tab="gest">${I("file",15)} Richieste RFQ</button>
      <button type="button" class="tendina-voce" data-tab="set">${I("users",15)} Clienti e mittenti</button>
      ${d.req?`<button type="button" class="tendina-voce" data-req="${d.req.id}">${I("mail",15)} La conversazione di questa richiesta</button>`:""}
      <button type="button" class="tendina-voce tendina-voce--sep" data-menu="ruolo" aria-haspopup="menu">${I("users",15)} Vedi come: <b>${esc(S.ruolo)}</b> ▾<small>ruolo simulato del mockup (Lino, Franco, Fabio, Marta)</small></button></div>`:""}
    ${S.menu==="cartiglio"?cartiglioTendina(q,T):""}
  </header>
  ${!e.valutato?`<div class="tt-riga tt-riga--ver" role="status">${I("alert",15)}<b>Thread non valutato.</b> <span>${esc(etichetta("motivo_thread",e.motivo))}</span> <span class="k3">Quello che segue non dice che va tutto bene.</span></div>`:""}`;
}
function cartiglioTendina(q, T){
  const d = datiRFQ(q), f = T.esito.fascicolo, t = T.testa;
  return `<section class="tendina tendina--cartiglio" aria-label="Cartiglio della RFQ">
    <div class="cartiglio">
      <div class="cg-cella cg-cella--rfq"><span class="lab">Richiesta d’offerta</span><span class="cg-num"><span class="mono">${esc(q.num)}</span>
        <small class="cg-rif">${q.numProvvisorio?"numero provvisorio · R-03":t.riferimento_cliente?"riferimento del cliente":""}</small></span>
        <span class="cg-ogg">${esc(t.oggetto)}</span></div>
      <div class="cg-cella"><span class="lab">Cliente</span><b>${esc(t.cliente)}</b></div>
      <div class="cg-cella"><span class="lab">Buyer</span><b>${esc(d.buyer)}</b></div>
      <div class="cg-cella"><span class="lab">Ricevuta</span><b class="nowrap">${esc(d.aperta)}</b></div>
      <div class="cg-cella"><span class="lab">Scadenza</span><b class="nowrap">${esc(d.scad)}</b></div>
      ${(()=>{ const L = lottiDi(T), pz = prodottoScelto(T); return `<div class="cg-cella cg-larga cg-b"><span class="lab">Quantità e scaglioni ${marchio("R-18")}</span>
        <span><b>${L.lotti.map(l=>fmt(l,0)).join(" / ")} pz</b> <span class="k3 piccolo">${esc(L.testo)}</span></span>
        ${pz?`<span class="cg-lotti"><input id="lotto-testa" inputmode="numeric" placeholder="es. 5/20/50/100" aria-label="Scaglioni della pratica"><button type="button" class="ma-btn ma-btn--sm pa-btn" data-pa-gesto="/prodotto/${esc(pz.componente_id)}/lotto" data-pa-form-id="lotto-testa">+ scaglioni</button></span>`:""}
        <small class="k3">ogni fase li eredita in pezzi: lotto × q.tà per assieme · i lotti del terzista si scrivono nella fase (Ciclo › Lotti), non qui <span class="chip-b" title="RL-21: i lotti dei terzisti stanno solo nella fase. Default, da confermare.">default, da confermare</span></small></div>`; })()}
      <div class="cg-cella cg-larga"><span class="lab">Cartella sul NAS</span><span class="mono cg-nas">${esc(q.nas||"—")}</span></div>
      <div class="cg-cella cg-larga"><span class="lab">Fascicolo della RFQ</span><span class="cg-fasc">${f.calcolato?`<span class="ma-st ma-st--neu ma-st--piatto">${f.numero_verificati} su ${f.numero_target} prodotti verificati</span>
        ${f.congelabile?`<span class="ma-st ma-st--ok">congelabile</span>`:`<span class="ma-motivo">non congelabile: ${esc(etichetta("motivo_non_congelabile",f.motivo_non_congelabile))}</span>`}
        <span class="ma-motivo">non congelato: ${esc(etichetta("motivo_non_congelato",f.motivo_non_congelato))}</span>`:`<span class="ma-st ma-st--na">fascicolo non calcolato</span>`}
        <span class="ma-na"><button type="button" class="ma-btn ma-btn--sm ma-btn--na" disabled aria-describedby="na-congela">Congela la V1</button><small id="na-congela">non ancora disponibile: il congelamento non si registra da questa pagina</small></span></span></div>
    </div>
    <p class="tendina-nota">Codice della RFQ, cliente e prodotto sono testo: la rinomina del prodotto non si fa da qui (DB7-3). Esc chiude.</p>
  </section>`;
}

/* la barra delle stazioni: una sola, numerata, con i quattro cassetti a destra */
/* nome breve sempre visibile + coda che si nasconde sotto i 1440 px («Struttura» e documenti, «Esito» → Franco) */
function staz2(nome){ const m = nome.match(/^(\S+)(\s.+)$/); return m ? `${esc(m[1])}<span class="staz-t2">${esc(m[2])}</span>` : esc(nome); }
function contatoreStazione(n, T, p){
  if(!p) return "—";
  /* RL-05: «completa» vale per le voci dentro il perimetro; se il perimetro è aperto lo dice (lettura del campo, non un calcolo) */
  if(n===1) return `BOM ${p.bom.verificata?"✓":"da verif."} · doc. ${etichetta("stato_documenti",p.documenti.stato)}${p.documenti.perimetro_chiuso===false&&p.documenti.stato!=="non_calcolabile"?" · perimetro aperto":""}`;
  /* dalla stazione ② in poi i contatori li dà il finto server A2 (zona B) */
  const righe = righeB(T,p), att = righe.filter(r=>r.cid && !r.comm), S2 = a2Di(T);
  if(n===2) return att.length ? `${att.filter(r=>comeDi(T,r.cid)).length}/${att.length}` : "—";
  if(n===3){ const {fare} = elencoCiclo(T,p); return fare.length ? `${fare.filter(r=>{ const c=cicloB(T,r.cid); return c&&c.stato==="confermato"; }).length}/${fare.length}` : "—"; }
  /* M-11: le stesse parole della barra della Sequenza; la sequenza è del prodotto scelto (A2) */
  if(n===4){ const root = righe.find(r=>r.radice); if(!root||!root.cid||!cicloB(T,root.cid)) return "—"; const sq = seqDi(T,p);
    return sq.stato==="confermata" && sq.firma===firmaSequenza(T,p) ? "✓ confermata" : fonteSequenza(T,p)==="L" ? "scritta da te" : "◇ proposta"; }
  if(n===5) return S2.consegne.length ? `consegnato ${S2.consegne.length}` : "da consegnare";
  return "—";
}
function barraStazioni(T, p){
  const S2 = T ? a2Di(T) : null, cc = p ? controlliB(T,p) : [];
  /* il numero sul cassetto: le offerte e le risposte da registrare (le bozze aspettano Outlook, non il Cockpit) */
  const nRq = S2 ? S2.richieste.filter(q=>["offerta_ricevuta","risposta"].includes(q.stato) && !registrazioneDi(T,q)).length : 0;
  const nKO = cc.filter(x=>x.liv==="KO"&&!x.inventato).length, nDB = cc.filter(x=>x.liv==="DB").length;
  const badge = {richieste: nRq?` ${nRq}`:"", controlli: nKO||nDB?` ${nKO?nKO+" KO":""}${nKO&&nDB?" · ":""}${nDB?nDB+(nDB===1?" dubbio":" dubbi"):""}`:"", cataloghi:"", backend:""};
  const cas = (k,ic,l,tasto) => `<button type="button" class="cas-btn" data-cassetto="${k}" aria-pressed="${S.cassetto===k}" title="${l} (tasto ${tasto})"><span aria-hidden="true">${ic}</span><span class="cas-l">${l}</span>${badge[k]?`<span class="cas-n mono">${badge[k]}</span>`:""}</button>`;
  /* v16 (SE §4.1): Richieste · Controlli · ⋯ ; nel ⋯ Cataloghi, il «?» e Non ancora nel backend (tasti L, ?, I) */
  const voce = (k,tab,ic,l,tasto) => `<button type="button" class="tendina-voce" role="menuitem" data-cassetto="${k}"${tab?` data-castab="${tab}"`:""}><span aria-hidden="true">${ic}</span> ${l} <kbd>${tasto}</kbd></button>`;
  const altro = `<span class="cas-altro"><button type="button" class="cas-btn" data-menu="altro" aria-expanded="${S.menu==="altro"}" aria-pressed="${["cataloghi","backend"].includes(S.cassetto)}" title="Cataloghi, aiuto e tasti, non ancora nel backend" aria-label="Altri cassetti"><span aria-hidden="true">⋯</span></button>
    ${S.menu==="altro"?`<div class="tendina tendina--altro" role="menu" aria-label="Altri cassetti">${voce("cataloghi","","▤","Cataloghi","L")}${voce("backend","tasti","?","Aiuto e tasti","?")}${voce("backend","spiegazione","ⓘ","Non ancora nel backend","I")}</div>`:""}</span>`;
  return `<nav class="stazioni" aria-label="Stazioni della Distinta">
    <div class="staz-elenco">${STAZIONI.map(([n,nome,zona])=>`<button type="button" class="staz${n>1?" staz--b":""}" data-staz="${n}" aria-current="${S.staz===n?"step":"false"}" title="${esc(nome)} · ${esc(contatoreStazione(n,T,p))} · ${esc(zona)} (G poi ${n})">
      <span class="staz-n">${n}</span><span class="staz-t">${staz2(nome)}</span><span class="staz-c">${esc(contatoreStazione(n,T,p))}</span></button>`).join("")}</div>
    ${S.staz===1 && p && p.bom.verificata ? `<button type="button" class="staz-poi" data-staz="2" title="La BOM è verificata: il passo dopo è ② Come (G poi 2)">Poi: ② Come →</button>` : ""}
    <div class="cassetti-btn">${cas("richieste","✉","Richieste","R")}${cas("controlli","⚠","Controlli","K")}${altro}</div>
  </nav>`;
}

/* le stazioni dei giri successivi: si aprono e dicono che cosa arriva (una stazione bloccata non è disabilitata) */
/* v16 (SE regola 5): UNA fascia di una riga per superficie B; le R-xx stanno nel title (e nel «?»), il testo lungo nel title */
function bandaB(r, testo){ const t = testo ? String(testo).replace(/<[^>]+>/g,"") : ""; return `<div class="banda-b" role="note" data-r="${esc(r)}" title="Proposta · dopo A2 · ${esc(r)}${t?" — "+esc(t):""}"><span class="banda-b-t">◇ Proposta · dopo A2</span>${t?`<span class="banda-b-x">${esc(t)}</span>`:""}</div>`; }

/* ---------------- i cassetti: spingono il foglio, non lo coprono ---------------- */

/* ---------------- tasti globali (v16: un solo tema, chiaro; niente pulsante del tema) ---------------- */
let attesaG = false;
/* un solo modo di cambiare stazione (clic, G + numero, collegamenti): l'avviso e i blocchi della stazione di prima non seguono Lino */
function vaiStazione(n){ if(S.staz!==n){ if(typeof PA!=="undefined") PA.avviso=null; S.blocco=null; S.menuMod=null; } S.staz=n; S.menu=null; render(); const m=document.querySelector(".ws-main"); if(m) m.scrollTop=0; }
document.addEventListener("keydown", e=>{
  if(S.tab!=="rfq") return;
  const t = e.target, campo = t && (t.tagName==="INPUT"||t.tagName==="TEXTAREA"||t.tagName==="SELECT"||t.isContentEditable);
  if(e.key==="Escape"){
    if(document.getElementById("ma-domanda")){ togliDomanda(); e.preventDefault(); return; }
    { const m = [...document.querySelectorAll("details.piu[open],details[data-ma-menu][open]")].pop(); if(m){ m.open = false; const su = m.querySelector("summary"); if(su) su.focus(); e.preventDefault(); return; } }
    if(S.dialogo){ S.dialogo=null; render(); e.preventDefault(); return; }
    if(S.menu){ S.menu=null; render(); e.preventDefault(); return; }
    if(S.menuMod||S.bcMenu){ S.menuMod=null; S.bcMenu=false; render(); e.preventDefault(); return; }
    if(S.blocco||S.esitoAnteprima||S.copiaDa||S.comeForm){ S.blocco=null; S.esitoAnteprima=false; S.copiaDa=false; S.comeForm=null; render(); e.preventDefault(); return; }
    if(S.cassetto){ S.cassetto=null; S.catDest=null; render(); e.preventDefault(); return; }
    if(S.staz===3 && S.faseAperta){ S.faseAperta=null; render(); e.preventDefault(); return; }
    if(S.casella){ S.casella=null; render(); e.preventDefault(); return; }
    return;
  }
  if(campo || e.ctrlKey || e.metaKey) return;
  if(e.altKey && !(S.staz===3 && (e.key==="ArrowUp"||e.key==="ArrowDown"))) return;
  if(attesaG){ attesaG=false; if(/^[1-5]$/.test(e.key)){ vaiStazione(+e.key); e.preventDefault(); } return; }
  const k = e.key.toLowerCase();
  if(k==="g"){ attesaG=true; setTimeout(()=>{attesaG=false;},1200); return; }
  const cas = {r:"richieste",k:"controlli",l:"cataloghi",i:"backend"}[k];
  if(cas){ const tab = k==="i" ? "spiegazione" : null; S.cassetto = S.cassetto===cas && (!tab || S.cassettoTab===tab) ? null : cas; if(tab) S.cassettoTab = tab; S.menu = null; render(); e.preventDefault(); return; }
  /* il «?» unico: lo stesso cassetto, sulla scheda Tasti */
  if(e.key==="?"){ const su = S.cassetto==="backend" && S.cassettoTab==="tasti"; S.cassetto = su ? null : "backend"; S.cassettoTab = "tasti"; S.menu = null; render(); e.preventDefault(); return; }
  if(S.staz===1 && typeof tastiStruttura==="function") tastiStruttura(e);
  else if(typeof tastiStazione==="function") tastiStazione(e);
});
