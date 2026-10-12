/* ================= 47 · stazione ② Come (zona B) e gli aiuti comuni delle viste della zona B =================
   SPEC_v15 §2.3. Sostituisce la lista «piccole lavorazioni» del passo 5 reale: una riga per codice distinto, un tasto per riga
   (1 noi · 2 lamiera comprata · 3 fuori · 4 dal cliente · 5 escluso · 6 no quote), Invio accetta la proposta ◇ e scende.
   Regole: si smistano solo i nodi con un componente confermato (RV-27); tipo e categoria si LEGGONO dalla zona A con il
   chip «A»; un commerciale non ha tasti (lo compra Fabio); lo smistamento non scrive mai nell'albero (RV-26).
   Le richieste nascono solo con «Prepara le richieste»: niente parte da solo (DB7-6, V4). */

/* ---------------- aiuti comuni della zona B ---------------- */
const NOME_FONTE = {CART:"dal cartiglio del disegno", STEP:"dallo STEP", DXF:"dal DXF", NOTA:"da una nota del disegno", ORD:"dall'ordine del cliente",
  STR:"dalla struttura confermata (zona A)", STO:"dallo storico: scheda costo, offerte passate", MAG:"dal magazzino", CAP:"dalle capacità dell'officina",
  CAPIT:"dal capitolato del cliente", RISP:"da una risposta registrata", CALC:"calcolato", "PRO-06":"dal modello di ciclo per famiglia", MAIL:"da una mail del thread",
  "⧉":"copiato da un altro componente", A:"letto dalla zona A (A1d)", L:"scritto da Lino", WPS:"dalle WPS"};
const chipF = f => f ? `<span class="fonte${f==="RISP"?" f-risp":f==="L"?" f-l":""}" title="${esc(NOME_FONTE[f]||f)}">${esc(f==="L"?"Lino":f)}</span>` : "";
/* v16 (SE regola 5): dentro una stazione B basta la fascia; il bollino «◇» resta solo dove la zona B entra in una
   superficie non B (testata, cartiglio, Documenti, cassetti condivisi). Le R-xx stanno nel title e nel «?». */
const marchio = r => `<span class="bollino-b" title="Proposta · dopo A2 · ${esc(r)}: il backend di oggi non ha questo dato" aria-label="proposta dopo A2">◇</span>`;
const datoProva = (t) => `<span class="dato inventato" title="Dato inventato per il mockup: le tabelle vere arrivano dall'officina e da GestX">${esc(t||"inventato")}</span>`;
let paTimer = null;
/* i livelli dei controlli per esteso: «dubbio» e «avviso», non DB e AV */
const NOME_LIV = {KO:"KO", DB:"dubbio", AV:"avviso"};
function paAvvisoHtml(){
  const a = PA.avviso; if(!a) return "";
  if(S.cassetto==="richieste") return "";   /* il cassetto Richieste aperto mostra lui l'avviso: uno solo */
  /* «fatto» sparisce da solo dopo 6 s; «no» ed «errore» restano finché Lino li chiude o cambia stazione */
  if(a.esito==="ok" && typeof setTimeout==="function"){ clearTimeout(paTimer); paTimer = setTimeout(()=>{ if(PA.avviso===a){ PA.avviso=null; const el=document.getElementById("pa-avviso"); if(el) el.remove(); } }, 6000); }
  return `<div class="pa-avviso" id="pa-avviso" role="status" aria-live="polite" data-esito="${esc(a.esito)}"><b class="pa-e" title="esito di un gesto della proposta per dopo A2 (zona B)">${a.esito==="ok"?"fatto":a.esito==="no"?"non fatto":"errore"}</b><span title="${esc((String(a.testo).match(/\((?:RV|DB7|R)-[^)]*\)/g)||[]).join(" "))}">${esc(String(a.testo).replace(/\s*\((?:RV|DB7)-[^)]*\)/g,"").replace(/\s*\(R103 C\)|,? R103 C/g,""))}</span>
    ${a.annulla?btnB(a.annulla.url, a.annulla.campi, "Annulla", "ma-btn--ghost", 'data-pa-annulla="1"'):""}<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-pa-chiudi="1" aria-label="Chiudi l'avviso">${I("x",14)}</button></div>`;
}
/* P-07: la conferma della minuteria è della zona C (R-08): qui il solo motivo, come nella zona A */
const minuteriaSpenta = () => `<span class="minu k3 piccolo" title="La conferma della categoria arriva con lo spazio di verifica (R-08): non si anticipa né si simula, come in ① Struttura.">⊘ minuteria a norma: arriva con R-08</span>`;
/* P-02: una richiesta ai fornitori si prepara nel cassetto, una per fornitore: da qui si apre la sua carta (navigazione) */
const apriRichiesta = (key, label, cls) => `<button type="button" class="ma-btn ma-btn--sm ${cls||"pa-btn"}" data-rq-apri="${esc(key)}" title="Apre la carta nel cassetto ✉: prepari la bozza per ogni fornitore (rotta di Pietro, Blocco 7B)">${label}</button>`;
/* un pulsante che scrive nella zona B: data-pa-gesto (mai data-ma-gesto, RV-07, RV-25) */
function btnB(url, campi, label, cls, extra){ return `<button type="button" class="ma-btn ma-btn--sm ${cls||""}" data-pa-gesto="${esc(url)}" data-pa-campi="${esc(JSON.stringify(campi||{}))}" ${extra||""}>${label}</button>`; }
const suffisso = (cod, pre) => pre && cod.startsWith(pre) ? cod.slice(pre.length-1) : cod;
const tipoBreve = r => r.comm ? "comm." : ({finito:"prodotto", sottoassieme:"assieme", sciolto:"partic.", commerciale:"comm."})[r.tipo] || (r.tipo||"—");
function righeCome(T, p){ return righeB(T,p); }

/* ---------------- la stazione ---------------- */
function stazioneCome(T, p){
  const righe = righeCome(T,p), S2 = a2Di(T), pre = prefissoComune(righe.map(r=>r.cod));
  const banda = bandaB("R-22 · R-39 · R-13 · R-27","modo di fornitura e modelli di ciclo restano nel mockup");
  if(!righe.length) return `<div class="staz-b">${banda}<div class="empty"><span class="ib">${I("cube",22)}</span><h3>Nessun componente</h3><p>Il prodotto non ha struttura: prima la stazione ①.</p></div></div>`;
  let filtro = S.comeFiltro || "da";
  if(filtro==="da" && !S.comeFiltro && righe.every(r=>!r.cid || r.comm || comeDi(T,r.cid))) filtro = "tutti";   /* tutto smistato: si vede il riepilogo */
  const decisa = r => r.comm || !!(r.cid && comeDi(T,r.cid));
  const attive = righe.filter(r=>r.cid && !r.comm);
  const vis = righe.filter(r => filtro==="tutti" ? true : filtro==="da" ? (!r.cid || !decisa(r)) : (r.cid && decisa(r) && !r.comm));
  if(!vis.some(r=>r.cid===S.comeRiga)) S.comeRiga = (vis.find(r=>r.cid && !r.comm)||{}).cid || null;
  const sm = attive.filter(decisa).length, nRich = S2.richieste.length, proposte = richiesteProposte(T,p);
  const sel = S.comeSel || (S.comeSel = new Set());
  const testa = `<div class="b-testa"><h2>② Come</h2><span class="b-conta" title="${righe.length>attive.length?`+${righe.length-attive.length} commerciali o non ancora componenti · `:""}richieste: ${proposte.length} proposte, ${nRich} preparate">${sm}/${attive.length} smistati</span>
      <span class="sp"></span>
      <span class="seg" role="group" aria-label="Filtro">${[["da","da smistare"],["fatti","smistati"],["tutti","tutti"]].map(([k,l])=>`<button type="button" data-come-filtro="${k}" aria-pressed="${filtro===k}">${l}</button>`).join("")}</span>
      <label class="cerca-b">${I("search",14)}<input id="come-cerca" placeholder="cerca (/)" aria-label="Cerca un codice" value="${esc(S.comeQ||"")}"></label></div>`;
  const q = (S.comeQ||"").toLowerCase();
  const filtrate = q ? vis.filter(r=>(r.cod+" "+r.descr).toLowerCase().includes(q)) : vis;
  S._comeVis = filtrate.filter(r=>r.cid && !r.comm).map(r=>r.cid);
  const blocco = S.blocco && S.blocco.staz===2 ? bloccoAnteprima(T,p,righe) : "";
  const corpo = filtrate.length ? `<div class="come-wrap"><table class="come-tab" aria-label="Smistamento: una riga per codice distinto">
    <thead><tr><th class="c-sel"><span class="vh">Seleziona</span></th><th>Codice${pre?` <span class="k3 mono" title="prefisso comune">${esc(pre.slice(0,-1))}…</span>`:""}</th><th>Denominazione</th><th class="come-q">Q.tà</th><th class="c-tipo" title="letto dalla zona A (struttura confermata)">Tipo · dalla zona A</th><th>Come</th><th class="c-mod">Modello di ciclo</th><th class="c-rq">Richiesta</th></tr></thead>
    <tbody>${filtrate.map(r=>rigaCome(T,p,r,pre,sel)).join("")}</tbody></table>
    ${S.menuMod?tendinaModello(T,p):""}</div>`
    : `<div class="empty"><span class="ib">✓</span><h3>${filtro==="da"?"Tutti i codici sono smistati":"Nessun codice"}</h3><p>${filtro==="da"?"Guarda «smistati» o «tutti», poi prepara le richieste e vai al Ciclo.":"Cambia il filtro."}</p></div>`;
  /* P-02: le richieste si preparano nel cassetto, una per fornitore (rotta di Pietro): da qui si apre il cassetto */
  const primario = proposte.length ? `<button type="button" class="ma-btn ma-btn--pri" data-rq-apri="${esc(proposte[0].key)}">Apri le richieste da preparare ✉ (${proposte.length})</button>`
    : `<button type="button" class="ma-btn ma-btn--pri" data-staz="3">Vai al Ciclo →</button>`;
  return `<div class="staz-b" data-come="1">${banda}${paAvvisoHtml()}
    <div class="staz-b-main">${testa}
      <div class="b-corpo">${blocco}${corpo}</div>
      <div class="b-piede barra-staz"><span class="b-piede-t">${proposte.length?`${proposte.length} richieste da preparare (${proposte.map(x=>TIPI_RICHIESTA[x.tipo].nome.toLowerCase()+" · "+x.cod.slice(-4)).join(", ")}): nessuna parte da sola.`:S2.richieste.length?`${S2.richieste.filter(x=>x.stato==="bozza").length} bozze in Outlook · ${S2.richieste.filter(x=>x.stato==="inviata").length} in attesa.`:"Nessuna richiesta finché non decidi come si fa."}</span>
        ${primario}</div></div></div>`;
}
function rigaCome(T, p, r, pre, sel){
  if(!r.cid) return `<tr class="come-riga come-riga--grigia"><td class="c-sel"></td><td class="c-cod"><span class="come-cod" title="${esc(r.cod)}">${esc(suffisso(r.cod,pre))}</span></td>
    <td class="c-den come-den" title="${esc(r.descr)}">${esc(r.descr)}</td><td class="c-q come-q">${r.qtaProd}</td><td class="c-tipo">${esc(tipoBreve(r))}</td>
    <td class="c-come" colspan="3"><span class="ma-st ma-st--prop">non ancora un componente</span> <span class="k3 piccolo">prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38)</span> <button type="button" class="ma-btn ma-btn--sm" data-staz="1">① Struttura</button></td></tr>`;
  const c = comeDi(T, r.cid), pr = modoComponente(T,p,r), cy = cicloB(T,r.cid), cur = S.comeRiga===r.cid;
  const cls = `come-riga${cur?" come-riga--cur":""}${sel.has(r.cid)?" come-riga--sel":""}`;
  const cod = `<span class="come-cod" title="${esc(r.cod)}">${esc(suffisso(r.cod,pre))}${r.assiemi>1?`<small>in ${r.assiemi} assiemi</small>`:""}</span>`;
  const testaR = `<td class="c-sel"><input type="checkbox" data-come-sel="${r.cid}" ${sel.has(r.cid)?"checked":""} aria-label="Seleziona ${esc(r.cod)}" ${r.comm?"disabled":""}></td>
    <td class="c-cod">${cod}</td><td class="c-den come-den" title="${esc(r.descr)}">${esc(r.descr)}</td><td class="c-q come-q">${r.qtaProd}</td>
    <td class="c-tipo">${esc(tipoBreve(r))}</td>`;
  if(r.comm) return `<tr class="${cls}" data-come-cid="${r.cid}">${testaR}<td class="c-come" colspan="3"><span class="ma-st ma-st--ver">commerciale</span> lo compra Fabio · entra in una fase del padre ${minuteriaSpenta()}
    <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-staz="1" title="Il tipo si cambia solo in Struttura (gesto Tipo): l'albero torna da confermare">commerciale · cambia in ① →</button></td></tr>`;
  const seg = `<span class="seg" role="group" aria-label="Come si fa ${esc(r.cod)}">${MODI.map(([k,l],i)=>{
      const on = c && c.modo===k, prop = !c && pr.modo===k;
      const campi = MODO_CON_CICLO.has(k) ? {modo:k, conModello:"1"} : {modo:k};
      return (k==="escluso"||k==="noquote") ? `<button type="button" data-come-form="${k}|${r.cid}" aria-pressed="${on}" class="${prop?"seg--prop":""}" title="${esc(MODI[i][2])}"><kbd>${i+1}</kbd>${esc(l)}</button>`
        : `<button type="button" data-pa-gesto="/componente/${r.cid}/fornitura" data-pa-campi='${JSON.stringify(campi)}' aria-pressed="${on}" class="${prop?"seg--prop":""}" title="${esc(MODI[i][2])}${prop?" · proposta ◇ "+esc(pr.perche):""}"><kbd>${i+1}</kbd>${esc(l)}</button>`; }).join("")}</span>`;
  const modello = c ? (MODO_CON_CICLO.has(c.modo) ? `<button type="button" data-menu-mod="${r.cid}" title="Scegli il modello (M)">${esc(c.modello||"—")} ${c.modello&&MODELLI[c.modello]?`<span class="k3">${esc(MODELLI[c.modello].nome)}</span>`:""} ▾</button>${cy&&cy.copiaDa?` <span class="k3" title="ciclo copiato">⧉ ${esc(cy.copiaDa.slice(-4))}</span>`:""}`
      : `<span class="k3">${esc(c.modello||"—")} ${c.modello&&MODELLI[c.modello]?esc(MODELLI[c.modello].nome):""}</span>`)
    : `<span class="pr" title="${esc(pr.perche)}"><span title="modello ${esc(pr.modello)}">${esc((MODELLI[pr.modello]||{}).nome||pr.modello)}</span>${pr.copia?` · ⧉ come ${esc(pr.copia.slice(-4))}`:""}</span> ${chipF(pr.fonte)}`;
  const rq = richiesteDi(T, r.cid), prR = richiesteProposte(T,p).filter(x=>x.cid===r.cid);
  const rich = rq.length ? rq.map(x=>`<span class="ma-st ma-st--${{bozza:"neu",inviata:"prop",offerta_ricevuta:"ok",risposta:"ok",declinata:"ver",scaduta:"ver",annullata:"neu"}[x.stato]||"neu"}" title="${esc(TIPI_RICHIESTA[x.tipo].nome)}${x.forn?" · "+esc(x.forn.nome):""}">${esc(x.id)} ${esc(NOMI_STATO_RQ[x.stato]||x.stato)}</span>`).join(" ")
    : prR.length ? prR.map(x=>`<span class="pr" title="da preparare nel cassetto ✉, una per fornitore">${esc({terzista:"terzista",lamiera:"lamiera",mp_mai:"materiale mai comprato: chiedi prezzo",mp_prezzo:"prezzo e tempi"}[x.tipo]||x.tipo)}${x.tipo==="terzista"||x.tipo==="lamiera"?": "+esc(candidatiDi(T,x.tipo,x.lav).map(k=>nomeBreve(terz(k).nome)).join(", ")):""}</span>`).join(" ") : `<span class="k3">—</span>`;
  /* v16 (SE regola 7): niente colonna Stato: la scelta della riga e il filtro lo dicono già; lo stato del ciclo sta nel title */
  const statoT = !c ? "da smistare" : c.modo==="noquote" ? "no quote" : !MODO_CON_CICLO.has(c.modo) ? nomeModo(c.modo) : !cy ? "ciclo vuoto" : cy.stato==="confermato" ? "ciclo confermato" : "ciclo in bozza";
  const form = S.comeForm && S.comeForm.cid===r.cid ? `<tr class="come-riga-form"><td colspan="8"><form data-pa-gesto="/componente/${r.cid}/fornitura" id="cf-${r.cid}">
      <input type="hidden" name="modo" value="${S.comeForm.modo}"><span class="lab">${S.comeForm.modo==="escluso"?"Escluso: testo per l'offerta":"No quote: il motivo"}</span>
      <input name="testo" id="come-form-testo" required placeholder="${S.comeForm.modo==="escluso"?"es. escluso dalla fornitura: lo fornisce il cliente":"es. problema di stampaggio"}">
      <button class="ma-btn ma-btn--sm">Conferma</button><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-come-form="">Annulla</button></form></td></tr>` : "";
  return `<tr class="${cls}" data-come-cid="${r.cid}" tabindex="-1" title="${esc(r.cod)} · ${esc(statoT)}">${testaR}<td class="c-come">${seg}</td><td class="c-mod come-mod">${modello}</td><td class="c-rq come-rq">${rich}</td></tr>${form}`;
}
function tendinaModello(T, p){
  const cid = S.menuMod, r = rigaB(T,p,cid); if(!r) return "";
  const cy = cicloB(T,cid);
  return `<div class="tendina-mod" role="menu" aria-label="Modelli di ciclo per ${esc(r.cod)}" style="right:120px;top:${40+Math.max(0,righeB(T,p).findIndex(x=>x.cid===cid))*34}px">
    ${Object.entries(MODELLI).filter(([k])=>!["T6","T7","T8"].includes(k)).map(([k,m])=>`<button type="button" role="menuitem" data-pa-gesto="/componente/${cid}/ciclo/modello" data-pa-campi='${JSON.stringify({modello:k, riapri: cy&&cy.stato==="confermato"?"1":""})}'><b class="mono">${k}</b> ${esc(m.nome)} <span class="k3">${m.fasi.map(x=>PROC[x]?PROC[x][0].toLowerCase():x).join(" → ")}</span></button>`).join("")}
    ${cy&&cy.stato==="confermato"?`<p class="tendina-nota">Il ciclo è confermato: il modello lo riapre in bozza.</p>`:""}
    <button type="button" data-menu-mod="">Chiudi</button></div>`;
}
/* l'anteprima del blocco: «Cambierà N componenti: …», Invio conferma, Esc annulla (FLUSSO §6) */
function bloccoAnteprima(T, p, righe){
  const b = S.blocco, rr = righe.filter(r=>b.cids.includes(r.cid));
  return `<section class="blocco-b" aria-label="Anteprima del blocco"><b>Cambierà ${rr.length} ${rr.length===1?"componente":"componenti"}:</b>
    <ul>${rr.map(r=>`<li><span class="mono">${esc(r.cod)}</span> ${esc(r.descr)}${comeDi(T,r.cid)?` <span class="k3">(ora: ${esc(nomeModo(comeDi(T,r.cid).modo))})</span>`:""}</li>`).join("")}</ul>
    <div class="acts"><span class="lab">diventa</span><span class="seg" role="group" aria-label="Come per il blocco">${MODI.slice(0,4).map(([k,l],i)=>`<button type="button" data-blocco-modo="${k}" aria-pressed="${b.modo===k}"><kbd>${i+1}</kbd>${esc(l)}</button>`).join("")}</span>
      ${btnB(`/rfq/${T.id}/come/blocco`, {cids:b.cids.join(","), modo:b.modo||""}, "Applica (Invio)", "", b.modo?"":"disabled")}
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-blocco-annulla="1">Annulla (Esc)</button>
      <span class="k3 piccolo">Una scrittura per componente sotto un gesto solo, con l'esito per riga (R-36). Il blocco non tocca l'albero.</span></div></section>`;
}

/* ---------------- i clic della stazione ---------------- */
document.addEventListener("click", e=>{
  const t = e.target.closest && e.target.closest("[data-come-filtro],[data-come-sel],[data-come-form],[data-menu-mod],[data-blocco-modo],[data-blocco-annulla],[data-come-cid],[data-pa-chiudi]");
  if(!t) return;
  const D = t.dataset;
  if(D.paChiudi){ PA.avviso = null; render(); return; }
  if(D.comeFiltro){ S.comeFiltro = D.comeFiltro; render(); return; }
  if(D.comeSel){ const s = S.comeSel || (S.comeSel = new Set()); if(t.checked) s.add(D.comeSel); else s.delete(D.comeSel); render(); return; }
  if(D.comeForm!==undefined){ if(!D.comeForm){ S.comeForm=null; render(); return; } const [modo,cid] = D.comeForm.split("|"); S.comeForm = {modo,cid}; S.comeRiga = cid; render(); const i = document.getElementById("come-form-testo"); if(i) i.focus(); return; }
  if(D.menuMod!==undefined){ S.menuMod = D.menuMod || null; render(); return; }
  if(D.bloccoModo){ S.blocco.modo = D.bloccoModo; render(); return; }
  if(D.bloccoAnnulla){ S.blocco = null; render(); return; }
  if(D.comeCid && !e.target.closest("button,input,a")){ S.comeRiga = D.comeCid; render(); return; }
});
document.addEventListener("input", e=>{ if(e.target.id==="come-cerca"){ S.comeQ = e.target.value; const pos = e.target.selectionStart; render(); const i = document.getElementById("come-cerca"); if(i){ i.focus(); try{ i.setSelectionRange(pos,pos); }catch(err){} } } });
/* dopo un gesto B: la navigazione che il gesto chiede (presentazione) */
function dopoGestoB(url, r, campi){
  if(r.esito!=="ok") return;
  /* una domanda interna è una bozza in Outlook: l'avviso lo dice, il cassetto resta com'è (nessun gesto in più per chiuderlo) */
  if(/\/rfq\/[^/]+\/domanda$/.test(url) && r.rid){ S.rqSu = r.rid; S.rqAperta = r.rid; }
  if(/\/preparazioni$/.test(url) && campi && campi.apri){ S.cassetto = "richieste"; S.rqSu = campi.apri; S.rqAperta = campi.apri; }
  if(/\/fornitura$/.test(url) || /\/come\/blocco$/.test(url)){ S.comeForm = null; S.blocco = null; if(S.comeSel) S.comeSel.clear(); if(S.cicloSel) S.cicloSel.clear(); }
  if(/\/ciclo\/modello$/.test(url)) S.menuMod = null;
  if(/^\/fase\//.test(url) && campi && campi.catalogo && S.cassetto==="cataloghi"){ S.catDest = null; if(window.innerWidth<1280) S.cassetto = null; }
  if(/\/ciclo\/fase$/.test(url) && r.fid){ S.faseAperta = r.fid; S.aggFase = ""; }
  if(typeof dopoSequenza==="function") dopoSequenza(url, r);
  if(typeof dopoCiclo==="function") dopoCiclo(url, r, campi);
  if(typeof dopoEsito==="function") dopoEsito(url, r, campi);
}
