/* ================= 48 · stazione ③ Ciclo: il foglio a righe di fase (zona B · R-10 R-11 R-23…R-30 R-36 R-37) =================
   SPEC_v15 §2.5. Tre colonne, ognuna scorre da sola: elenco · disegno (sempre aperto, D lo nasconde) · foglio.
   Il foglio, dall'alto: testataPezzo() (lo STESSO componente di R6, M11) · preparazioni in testa, FUORI dalla sequenza
   delle fasi (decisione 07/10) · fasi a righe, ognuna con i soli campi della sua famiglia; il grezzo nella prima fase
   che lo consuma, dalla banca dati dei materiali · aggiungi una fase (/) · controlli · barra di conferma come ULTIMA
   riga della colonna (C2, M10, B5).
   v16 (SPEC_v16 §3): il PRIMO disegno è il PDF del prodotto finito (dato A letto: Indice.Voce del prodotto, poi il nodo
   radice; mai un disegno inventato), con le schede Prodotto finito · Componente e il tasto V. Le fasi di unione hanno
   «Entrano» a scelta e «+ fase di unione prima» (pre-unioni in fila, lo stesso pezzo della Sequenza: 49). Le preparazioni
   hanno una riga ciascuna con «Richiedi». Meno pulsanti: «⋯» per fase, un solo «+ nota», fonti solo sulle proposte.
   Regole: il ciclo si lega al componente confermato; senza componente il foglio è in sola lettura (RV-27). Le proposte
   ◇ sono in corsivo prugna con la fonte e diventano dati solo con Invio o «Accetta». Tutti gli stati (grezzo, lavorabilità,
   WPS, completezza, controlli) li dà il finto server A2: qui si sceglie solo come mostrarli. */

/* il prefisso comune è aritmetica sulle stringhe dei codici, non uno stato (SPEC §2.5) */
function prefissoComune(cods){
  if(cods.length<2) return "";
  let meglio = "";
  for(const c of cods){
    for(let k=4;k<c.length;k++){ if(!/[.\-_ ]/.test(c[k-1])) continue;
      const pre = c.slice(0,k), n = cods.filter(x=>x.startsWith(pre) && x.length>pre.length).length;
      if(n>=2 && n>=cods.length/2 && pre.length>meglio.length) meglio = pre; }
  }
  return meglio;
}
function gruppoCiclo(IX, p){ return IX.Gruppi(p.rif).find(x=>x.tipo==="bom") || IX.Gruppi(p.rif).find(x=>x.tipo==="senza") || IX.Gruppi(p.rif).find(x=>x.tipo==="cand") || null; }
/* l'ordine dell'elenco: da fare (con ciclo) · senza ciclo (dal cliente, esclusi, commerciali) · senza componente */
function elencoCiclo(T, p){
  const righe = righeB(T,p);
  const conCiclo = r => r.cid && !r.comm && (()=>{ const c = comeDi(T,r.cid); return !c || MODO_CON_CICLO.has(c.modo); })();
  return { righe, fare: righe.filter(conCiclo), senza: righe.filter(r=>r.cid && !conCiclo(r)), nocomp: righe.filter(r=>!r.cid) };
}
function prossimoDaFare(T, p, cid){
  const {fare} = elencoCiclo(T,p); const i = fare.findIndex(r=>r.cid===cid);
  const giro = [...fare.slice(i+1), ...fare.slice(0, Math.max(0,i))];
  return giro.find(r=>{ const c = cicloB(T,r.cid); return !c || c.stato!=="confermato"; }) || null;
}
function peggiore(cc){ return cc.some(x=>x.liv==="KO"&&!x.inventato)?"KO":cc.some(x=>x.liv==="DB"||x.liv==="KO")?"DB":cc.some(x=>x.liv==="AV")?"AV":null; }

function stazioneCiclo(T, p){
  const IX = indiceDi(T);
  if(!p) return `<p class="ma-vuoto">Nessun prodotto.</p>`;
  const banda = bandaB("R-10 · R-25 · R-36","Il backend di oggi non ha il ciclo, le fasi né il grezzo: quello che scrivi qui resta nel mockup.");
  const g = gruppoCiclo(IX, p);
  const {righe, fare, senza, nocomp} = elencoCiclo(T,p);
  if(!g || !righe.length) return `<div class="staz-vuota">${banda}<div class="empty"><span class="ib">${I("cube",22)}</span><h3>Il foglio del ciclo è in sola lettura</h3>
      <p>Il ciclo si scrive su un componente confermato: prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38).</p>
      <div class="acts" style="justify-content:center"><button type="button" class="ma-btn" data-staz="1">← Vai a Struttura e documenti</button></div></div></div>`;
  const pre = prefissoComune(righe.map(r=>r.cod));
  const scelta = righe.find(r=>r.cid && r.cid===S.cicloCid) || righe.find(r=>r.rif===S.cicloRif) || fare.find(r=>{ const c=cicloB(T,r.cid); return !c||c.stato!=="confermato"; }) || fare[0] || senza[0] || righe[0];
  S.cicloCid = scelta.cid || null; S.cicloRif = scelta.rif;
  const ctrl = controlliB(T,p);
  const filtro = S.cicloFiltro || "da";
  const passaFiltro = r => filtro==="tutti" || (filtro==="da" ? (()=>{ const c=cicloB(T,r.cid); return !c||c.stato!=="confermato"; })() : !!peggiore(ctrl.filter(x=>x.cid===r.cid)));
  const sel = S.cicloSel || (S.cicloSel = new Set());
  const riga = (r, conPal) => { const c = r.cid ? cicloB(T,r.cid) : null, pg = peggiore(ctrl.filter(x=>x.cid===r.cid)), co = r.cid ? comeDi(T,r.cid) : null;
    const st = !c ? "○" : c.stato==="confermato" ? "✓" : "◐";
    const tag = !r.cid ? "non ancora un componente" : r.comm ? "commerciale · nella fase del padre" : co && !MODO_CON_CICLO.has(co.modo) ? nomeModo(co.modo) : !co ? "come: da decidere" : (c && c.copiaDa ? "⧉ "+c.copiaDa.slice(-4) : "");
    return `<button type="button" class="cl-riga${r===scelta?" cl-riga--su":""}${sel.has(r.cid)?" cl-riga--sel":""}" data-ciclo-rif="${esc(r.rif)}" ${r.cid?`data-ciclo-cid="${r.cid}"`:""} aria-current="${r===scelta}" title="${esc(r.cod)} · ${esc(r.descr)}">
      <span class="cl-pal${st==="◐"?" cl-pal--b":st==="✓"?" cl-pal--c":""}" aria-label="ciclo ${st==="○"?"vuoto":st==="✓"?"confermato":"in bozza"}">${conPal?st:"·"}</span><b class="cl-suf mono">${esc(suffisso(r.cod,pre))}</b>
      <span class="cl-nome">${esc(r.descr||"")}</span><span class="cl-pals">${pg?`<i class="pal pal--${pg}" title="controllo peggiore: ${pg}"></i>`:""}</span>
      ${tag?`<span class="cl-tag">${esc(tag)}</span>`:""}</button>`; };
  const nOk = fare.filter(r=>{ const c=cicloB(T,r.cid); return c&&c.stato==="confermato"; }).length;
  const fareVis = fare.filter(passaFiltro);
  const elenco = `<nav class="cl-elenco" aria-label="Componenti del prodotto">
      <div class="cl-testa"><span class="lab">Elenco ${nOk}/${fare.length}${fare.length&&nOk===fare.length?" ✓":""}</span>${pre?`<span class="cl-pre mono" title="prefisso comune">${esc(pre.slice(0,-1))}</span>`:""}</div>
      <div class="cl-filtro"><label class="cl-filtro-l"><span class="vh">Filtro dell'elenco</span><select data-ciclo-filtro-sel="1" aria-label="Filtro dell'elenco">${[["","da fare"],["dubbi","con dubbi"],["tutti","tutti"]].map(([k,l])=>`<option value="${k}" ${(filtro==="da"?"":filtro)===k?"selected":""}>${l}</option>`).join("")}</select></label></div>
      ${sel.size?`<div class="cl-filtro"><span class="piccolo">${sel.size} selezionati</span> <button type="button" class="ma-btn ma-btn--sm" data-ciclo-blocco="1">Applica un modello (A)</button></div>`:""}
      ${fareVis.map(r=>riga(r,true)).join("")||(!fare.length&&nocomp.length?`<div class="cl-vuoto-el"><p class="piccolo"><b>Nessun ciclo si può ancora scrivere</b>: i pezzi non sono ancora componenti. In ①: prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38).</p><button type="button" class="ma-btn ma-btn--sm" data-staz="1">→ ① Struttura</button></div>`
        :!fare.length?`<p class="k3 piccolo" style="padding:8px 12px">Nessun componente con un ciclo: decidi prima come si fa in ② Come.</p>`
        :`<p class="k3 piccolo" style="padding:8px 12px">Niente ${filtro==="da"?"da fare: tutti i cicli sono confermati":"con dubbi"}.</p>`)}
      ${senza.length?`<div class="cl-sep"><span class="lab">senza ciclo</span></div>${senza.map(r=>riga(r,false)).join("")}`:""}
      ${nocomp.length?`<div class="cl-sep"><span class="lab">senza componente · sola lettura</span></div>${nocomp.map(r=>riga(r,false)).join("")}`:""}
    </nav>`;
  const n = g.nodi.find(x=>x.nodo.rif===scelta.rif) || g.nodi[0];
  if(!S.disCiclo) S.disCiclo = "prodotto";   /* entrando nella stazione il primo disegno è il prodotto finito (S-04) */
  const v420 = S.vista420 || "disegno";   /* B-7: a 420 px si apre sul disegno del prodotto finito (S-04) */
  return `<div class="ciclo">${paAvvisoHtml()}
    <div class="ciclo-sopra">${banda}${requisitiLetti(T,p)}${S.blocco&&S.blocco.staz===3?bloccoModelli(T,p):""}
    <div class="cl-segmento"><span class="seg" role="group" aria-label="Che cosa vedere">${[["fasi","Fasi"],["disegno","Disegno"],["elenco","Elenco"]].map(([k,l])=>`<button type="button" data-vista420="${k}" aria-pressed="${v420===k}">${l}</button>`).join("")}</span></div></div>
    <div class="ciclo-griglia${S.disegnoNascosto?" ciclo-griglia--nodis":""}" data-vista420="${v420}">
      ${elenco}
      ${disegnoCiclo(T, p, scelta, n)}
      ${foglioCiclo(T, IX, p, g, n, scelta, pre, fare, ctrl)}
    </div></div>`;
}

/* la riga «requisiti letti» (decisione 1, default): ogni requisito va nella fase che lo copre; «nessuna fase» è REQ-01 */
function requisitiLetti(T, p){
  const rr = REQUISITI[T.key]||[]; if(!rr.length) return "";
  const chip = q => { const c = requisitoCoperto(T,p,q);
    if(q.tipo==="condizione") return `<button type="button" class="req-chip${c.ok?" req-chip--ok":""}" data-vai="5" title="${esc(q.testo)} · condizione dell'ordine: si vede in Esito">${chipF(q.fonte)} ${esc(q.testo)} → Esito ${c.ok?"✓":"⚑"}</button>`;
    return c.ok ? `<button type="button" class="req-chip req-chip--ok" data-vai="3|${c.cid}|${c.fid}" title="${esc(q.testo)} · coperto dalla fase">${chipF(q.fonte)} ${esc(q.testo)} → ${esc(c.dove)} ✓</button>`
      : `<button type="button" class="req-chip req-chip--no" data-vai="3|${(righeB(T,p).find(r=>r.radice)||{}).cid||""}|||${({controllo:"contr",saldatura:"sald",trattamento:"sabb",certificati:"cert"})[q.tipo]||""}" title="${esc(q.testo)} · REQ-01: requisito senza fase · porta ad «Aggiungi una fase»">${chipF(q.fonte)} ${esc(q.testo)} → nessuna fase ⚠</button>`; };
  const no = rr.filter(q=>q.tipo!=="condizione" && !requisitoCoperto(T,p,q).ok).length;
  const ordinati = [...rr.filter(q=>q.tipo!=="condizione" && !requisitoCoperto(T,p,q).ok), ...rr.filter(q=>!(q.tipo!=="condizione" && !requisitoCoperto(T,p,q).ok))];
  /* v16 (SE §4.8): chiuso è UN chip «Requisiti: 1 senza fase ▸»; aperto mostra l'elenco (l'elenco intero sta anche in Esito e Controlli) */
  return `<div class="req-riga${S.reqAperti?" req-riga--aperta":" req-riga--chiusa"}" role="group" aria-label="Requisiti letti"><button type="button" class="req-tit" data-req-toggle="1" aria-expanded="${!!S.reqAperti}" title="Decisione 1 (default, da confermare): i requisiti escono dal foglio e vanno nelle fasi">Requisiti: ${no?`<b>${no} senza fase</b>`:`${rr.length} letti ✓`} ${S.reqAperti?"▴":"▸"}</button>
    ${S.reqAperti?ordinati.map(chip).join("")+`<span class="chip-b" title="Decisione 1: i requisiti escono dal foglio e vanno nelle fasi. Default, da confermare.">dec. 1</span>`:""}</div>`;
}

/* ---------------- il disegno: sempre aperto, note numerate legate alla fase aperta (N, R-14) ----------------
   v16: due schede sopra il disegno. «Prodotto finito» (aperta entrando nella stazione) è il PDF del prodotto, il
   riferimento per capire dove va il pezzo; «Componente» è il 2D del pezzo aperto. V passa dall'uno all'altro, D nasconde.
   Il PDF è un dato A LETTO, nell'ordine (FB §5.1): 1) Indice.Voce(prodotto, componente_id, "disegno_2d") → disegni.primario;
   2) il nodo radice della prima scheda di Indice.Gruppi; 3) niente: «anteprima non disponibile». Si mostra solo con
   «disponibile» e sha256; nel Cockpit vero è GET /allegato/{AllegatoID}/anteprima?v=<Sha256> con pdf.js (mai iframe). */
function disegnoProdotto(T, p){
  const IX = indiceDi(T), v = p.componente_id ? IX.Voce(p.rif, p.componente_id, "disegno_2d") : null;
  let pr = v && v.disegni && v.disegni.primario ? v.disegni.primario : null, da = "voce 2D del prodotto";
  if(!pr){ const gr = IX.Gruppi(p.rif)[0], rad = gr ? gr.nodi.find(x=>!x.parentela.length) : null; if(rad && rad.disegni && rad.disegni.primario){ pr = rad.disegni.primario; da = "nodo radice della scheda"; } }
  const an = pr ? pr.anteprima : null;
  if(!pr) return {ok:false, testo:"anteprima non disponibile: il prodotto non ha un 2D"};
  if(/step|stp/i.test(pr.formato||"") || /\.(step|stp)$/i.test(pr.nome_file||"")) return {ok:false, testo:"anteprima non disponibile: è uno STEP", pr};
  if(!an || !an.disponibile || !an.sha256) return {ok:false, testo:"anteprima non disponibile (il file non è leggibile o non ha l'impronta)", pr};
  return {ok:true, pr, an, da, proposto: pr.provenienza==="proposto", url:`/allegato/${an.allegato_id}/anteprima?v=${an.sha256}`};
}
function disegnoComponente(n){
  const pr = n && n.disegni && n.disegni.primario; if(!pr) return {ok:false, testo:"nessun 2D associato: lo dicono i Documenti (DOC-01)"};
  const an = pr.anteprima; if(!an || !an.disponibile || !an.sha256) return {ok:false, testo:"anteprima non disponibile (il file non è leggibile o non ha l'impronta)", pr};
  return {ok:true, pr, an};
}
function disegnoCiclo(T, p, r, n){
  const S2 = a2Di(T), note = r.cid ? (S2.note[r.cid]||[]) : [], dc = disegnoComponente(n), prC = dc.ok ? dc.pr : null;
  const eRadice = !!r.radice, z = S.disZoom || 1, vista = S.disCiclo==="componente" && !eRadice ? "componente" : "prodotto";
  const dp = disegnoProdotto(T, p), radice = righeB(T,p).find(x=>x.radice);
  const faseNome = id => { const c = r.cid ? cicloB(T,r.cid) : null; const i = c ? c.fasi.findIndex(f=>f.id===id) : -1; return i>=0 ? `f.${(i+1)*10}` : ""; };
  const pr = vista==="prodotto" ? (dp.ok ? dp.pr : null) : prC;
  const schede = `<span class="seg dis-schede" role="group" aria-label="Quale disegno"><button type="button" data-dis-ciclo="prodotto" aria-pressed="${vista==="prodotto"}" title="Il PDF del prodotto finito: il riferimento (V)">Prodotto finito · <span class="mono">${esc(suffisso((radice||{}).cod||p.codice_richiesto, prefissoComune(righeB(T,p).map(x=>x.cod))))}</span></button>${eRadice?`<button type="button" disabled aria-disabled="true" title="È il prodotto finito: il suo disegno è la scheda «Prodotto finito»">Componente</button>`:`<button type="button" data-dis-ciclo="componente" aria-pressed="${vista==="componente"}" title="Il 2D del componente aperto (V)">Componente · <span class="mono">${esc(suffisso(r.cod, prefissoComune(righeB(T,p).map(x=>x.cod))))}</span></button>`}</span>`;
  const menu = S.disMenu ? `<span class="dis-menu" role="menu"><button type="button" role="menuitem" data-dis-zoom="-1">⊖ Riduci</button><button type="button" role="menuitem" data-dis-zoom="1">⊕ Ingrandisci</button><button type="button" role="menuitem" data-dis-zoom="0">⤢ Adatta</button><button type="button" role="menuitem" data-dis-nascondi="1">Nascondi il disegno (D)</button></span>` : "";
  const palco = vista==="prodotto"
    ? (dp.ok ? `<div class="ma-palco" data-anteprima="${esc(dp.url)}" title="${esc(dp.pr.nome_file)} · ${esc(dp.da)}"><div style="width:${Math.round(100*z)}%;margin:auto">${schizzo(dp.pr.nome_file)}</div></div>` : `<div class="ma-palco ma-palco--vuoto"><p>${esc(dp.testo)}</p></div>`)
    : (prC ? `<div class="ma-palco"><div style="width:${Math.round(100*z)}%;margin:auto">${schizzo(prC.nome_file)}</div></div>` : `<div class="ma-palco ma-palco--vuoto"><p>${esc(dc.testo)}</p></div>`);
  const anV = vista==="prodotto" ? (dp.ok ? dp.an : null) : (dc.ok ? dc.an : null);
  return `<section class="cl-disegno" aria-label="Disegno">
    <div class="ma-palco-barra dis-barra">${schede}<span class="sp"></span>
      ${pr&&anV?`<button type="button" class="ma-btn ma-btn--sm" data-pa-visore="1" data-pa-allegato="${esc(anV.allegato_id)}" data-pa-sha="${esc(anV.sha256)}" data-pa-file="${esc(pr.nome_file)}" title="Apre ${esc(pr.nome_file)} nel visore (GET /allegato/{id}/anteprima?v=sha256)">⤢ Visore</button>`:""}
      <span class="dis-piu"><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-dis-menu="1" aria-expanded="${!!S.disMenu}" aria-label="Zoom e nascondi">⋯</button>${menu}</span></div>
    <div class="dis-file">${pr?`<span class="mono ma-palco-file" title="${esc(pr.nome_file)}">${esc(pr.nome_file)}</span>${vista==="prodotto"&&dp.proposto?` <span class="ma-st ma-st--prop" title="Il 2D del prodotto è solo proposto: lo conferma la vista Documenti (zona A)">proposto</span>`:""}`:`<span class="k3">${vista==="prodotto"?esc(dp.testo):"nessun 2D"}</span>`}</div>
    ${palco}
    <div class="cl-note"><span class="lab" title="R-14: note numerate sul disegno, legate alla fase aperta">Note sul disegno</span>
      ${note.length?note.map(x=>`<span><i class="cl-nota-n">${x.n}</i> ${esc(x.testo)}${x.fase?` → ${esc(faseNome(x.fase))}`:""}</span>`).join(""):""}
      ${r.cid&&note.length?btnB(`/rfq/${T.id}/domanda`, {tipo:"domanda", cid:r.cid, testo:`Domanda sulla nota ${note[note.length-1].n} del disegno: «${note[note.length-1].testo}»`}, "? Domanda tecnica al cliente", "pa-btn ma-btn--ghost"):""}
      ${r.cid?`<form data-pa-gesto="/componente/${r.cid}/nota"><input name="testo" id="nota-in" placeholder="nota legata alla fase aperta (N)" aria-label="Nota sul disegno"><input type="hidden" name="fase" value="${esc(S.faseAperta||"")}"><button class="ma-btn ma-btn--sm">+ nota</button></form>`:""}</div>
  </section>`;
}

/* ---------------- il foglio ---------------- */
function foglioCiclo(T, IX, p, g, n, r, pre, fare, ctrl){
  const c = r.cid ? cicloB(T,r.cid) : null, co = r.cid ? comeDi(T,r.cid) : null, pr = r.cid && !r.comm ? modoComponente(T,p,r) : null;
  const seme = SEMI[r.cod]||{}, ul = ULTIMA[r.cod];
  const zonaB = `<div class="tp-b" role="note" title="Proposta · dopo A2 · R-39 R-36">
      <span class="tp-b-v">modello ${c&&c.modello?`<b title="modello ${esc(c.modello)} · fonte ${esc(c.fonte||"")}">${esc((MODELLI[c.modello]||{}).nome||c.modello)}</b>${c.fonte&&c.fonte.startsWith("⧉")?" ⧉":""}`:pr?`<span class="pr" title="modello ${esc(pr.modello)}">${esc((MODELLI[pr.modello]||{}).nome||pr.modello)}</span> ${chipF(pr.fonte)}`:"—"}</span>
      <span class="tp-b-v">ultima volta ${ul?`<b title="fonte STO · ${esc(ul.rif)}">preventivo ${esc(ul.prev)} · consuntivo ${esc(ul.cons)}</b>`:"<i>—</i>"}</span>
      ${c&&c.delta?`<span class="tp-b-v">delta ECO: <b>${esc(c.delta)}</b></span>`:""}
      <span class="tp-b-v">peso netto ${seme.netto?`<span class="pr">${fmt(seme.netto[0],2)} kg</span> ${chipF(seme.netto[1])}`:"<i>—</i>"}</span></div>`;
  const testa = `<div class="cl-foglio-testa">${testataPezzo(T, IX, p, gruppoCiclo(IX,p), n, zonaB)}${S.copiaDa?pannelloCopia(T,p,r):""}</div>`;
  let corpo, barra = "";
  if(!r.cid){
    corpo = `<p class="ma-esito ma-esito--no">${I("alert",15)}<span>Sola lettura: il ciclo si scrive su un componente confermato. Prima la struttura e i codici: accetta il nodo o conferma l'albero in ①.</span></p>`;
  } else if(r.comm){
    corpo = `<div class="cl-vuoto"><b>Commerciale: nessun ciclo (decisione 07/10).</b><span>Lo compra Fabio ed entra in una fase del padre (saldatura o montaggio). Il 2D si chiede comunque; la conferma della minuteria è spenta (R-08).</span>
      ${na("Conferma la minuteria","la conferma della categoria arriva con lo spazio di verifica (R-08)")}</div>`;
  } else if(!co){
    corpo = `<div class="cl-vuoto"><b>Come si fa: non ancora deciso.</b><span>Proposta ◇ <span class="pr" title="modello ${esc(pr.modello)}">${esc(nomeModo(pr.modo))} · ${esc((MODELLI[pr.modello]||{}).nome||pr.modello)}</span> ${chipF(pr.fonte)} — ${esc(pr.perche)}</span>
      <span class="acts" style="display:flex;gap:8px;flex-wrap:wrap">${btnB(`/componente/${r.cid}/fornitura`, MODO_CON_CICLO.has(pr.modo)?{modo:pr.modo, conModello:"1"}:{modo:pr.modo}, "Accetta la proposta (Invio)", "", 'data-accetta-come="1"')}<button type="button" class="ma-btn ma-btn--sm" data-staz="2">Decidi in ② Come</button></span></div>`;
  } else if(!MODO_CON_CICLO.has(co.modo)){
    corpo = `<div class="cl-vuoto"><b>${esc(nomeModo(co.modo))}: nessun ciclo.</b><span>${co.modo==="cliente"?"Arriva dal cliente in conto lavoro: controllo in accettazione e data d'arrivo dall'ordine, nella Sequenza (T7).":co.modo==="escluso"?"Escluso dalla fornitura: «"+esc(co.testo)+"» va nell'offerta (T8).":"No quote: «"+esc(co.testo)+"» (KO in Esito)."}</span>
      <button type="button" class="ma-btn ma-btn--sm" data-staz="2">Cambia in ② Come</button></div>`;
  } else if(!c){
    corpo = `<div class="cl-vuoto"><b>Ciclo vuoto.</b><span>Modello ◇ ${esc((MODELLI[co.modello||pr.modello]||{}).nome||co.modello||pr.modello)} · scegli il modello o aggiungi le fasi.</span>${btnB(`/componente/${r.cid}/ciclo/modello`, {modello:co.modello||pr.modello}, `Applica «${esc((MODELLI[co.modello||pr.modello]||{}).nome||co.modello||pr.modello)}»`)}</div>${aggiungiFase(T,p,r,null)}`;
  } else {
    const ctrlC = ctrl.filter(x=>x.cid===r.cid);
    if(S.faseAperta===undefined || (S.faseAperta && !c.fasi.some(f=>f.id===S.faseAperta))){ const pr0 = c.fasi.find(f=>{ const s0 = statoFase(T,p,r.cid,f); return s0.proposte || s0.manca.length; }); S.faseAperta = pr0 ? pr0.id : null; }
    corpo = `${preparazioni(T,p,r,c)}${r.radice?rigaTrattamento(T,r,c):""}
      <div class="fasi" role="list" aria-label="Fasi del ciclo">${c.fasi.map((f,i)=>faseHtml(T,p,r,c,f,i,ctrlC)).join("")||`<p class="k3 piccolo">Nessuna fase.</p>`}</div>
      ${aggiungiFase(T,p,r,c)}
      ${controlliComponente(T,ctrlC)}`;
    barra = barraConferma(T,p,r,c,fare,pre,ctrlC);
  }
  if(!barra && r.cid && !r.comm){ const nx = prossimoDaFare(T,p,r.cid); barra = `<div class="barra-conf"><span class="bc-stato">${co&&!MODO_CON_CICLO.has(co.modo)?"senza ciclo":"ciclo non ancora iniziato"}</span><span class="bc-nav">${navComp(fare, r)}</span>${nx?`<button type="button" class="ma-btn ma-btn--sm" data-ciclo-cid="${nx.cid}" data-ciclo-rif="${esc(nx.rif)}">Vai a ${esc(suffisso(nx.cod,pre))} →</button>`:""}</div>`; }
  return `<section class="cl-foglio" aria-label="Foglio del componente">${testa}<div class="cl-scroll" id="cl-scroll">${corpo}</div>${barra}</section>`;
}
/* RL-18 (R-10): sul prodotto finito il trattamento è una scelta esplicita, non un campo vuoto: «nessuno (consegna grezza)»
   oppure una fase di trattamento. Il disegno del tirante non lo dice: lo decide Lino (fonte L). */
function rigaTrattamento(T, r, c){
  if(c.fasi.some(f=>TRATT.has(f.p))) return "";
  const v = c.trattamento && c.trattamento.v;
  return `<div class="tratt-r" role="group" aria-label="Trattamento del prodotto finito" title="Proposta · dopo A2 · R-10"><span class="lab">Trattamento</span>
    ${v?`<b>nessuno · consegna grezza</b> <span class="k3">scelta di ${esc(c.trattamento.da||"Lino")} · ${esc(c.trattamento.il||"")} · fonte L</span> ${btnB(`/componente/${r.cid}/ciclo/trattamento`, {v:""}, "Cambia", "ma-btn--ghost")}`
      :`<span class="k3">il disegno non lo dice:</span> ${btnB(`/componente/${r.cid}/ciclo/trattamento`, {v:"nessuno"}, "nessuno (consegna grezza)", "", 'data-tratt-nessuno="1"')} <span class="k3">oppure</span> <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vai="3|${r.cid}|||vern">+ fase di trattamento</button>`}</div>`;
}
function navComp(fare, r){ const i = fare.indexOf(r); if(i<0) return ""; return `<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ciclo-nav="-1" aria-label="Componente prima">←</button><span title="componente ${i+1} di ${fare.length}">${i+1}/${fare.length}</span><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ciclo-nav="1" aria-label="Componente dopo">→</button>`; }
function pannelloCopia(T, p, r){
  const l = copiaDaProposte(T,p,r.cid), sr = STORICO_REV[r.cod];
  return `<div class="cl-vuoto" style="margin-top:8px"><b>⧉ Copia da… (C)</b><span class="piccolo">Una copia è sempre una bozza: macchine e qualità restano, i driver (metri, fori, filetti, pieghe) si svuotano.</span>
    ${sr?`<span style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">${btnB(`/componente/${r.cid}/ciclo/copia`, {da:"rev:"+r.cod}, `⧉ rev. precedente ${esc(sr.rev)}`)} <span class="k3 piccolo">${esc(sr.rif)} · delta: ${esc(sr.delta)} · ${datoProva("dati di prova")}</span></span>`:""}
    ${l.length?l.map(x=>`<span style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">${btnB(`/componente/${r.cid}/ciclo/copia`, {da:x.cod}, `⧉ ${esc(x.cod)}`)} <span class="k3 piccolo">${esc(x.descr)} · ${esc(x.perche)}</span></span>`).join(""):`<span class="k3">Nessun altro componente con un ciclo.</span>`}
    <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-copia-da="">Chiudi</button></div>`;
}

/* M2 di Lino: nel riepilogo ogni preparazione dice la SUA fase («maschera f.10 pre-unione · da quotare»); due dello stesso
   tipo e nello stesso stato si leggono insieme («2 maschere da quotare (f.10, f.30)») */
function riepilogoPrep(T, p, r, c, attive){
  const breve = v => (v.k==="altra" ? (v.nome||"altra") : nomePrep(v.k)).replace(/ di saldatura| del robot| di piega| di curvatura| di foratura| di controllo| di taglio/,"").toLowerCase();
  const fase = v => { const i = c.fasi.findIndex(f=>f.id===v.nata); if(i<0) return v.nata ? "fase tolta" : ""; const iu = infoUnione(T,p,r,c,c.fasi[i]); return `f.${(i+1)*10}${iu&&iu.preUnione?" pre-unione":""}`; };
  const parola = v => v.stato==="esiste già" ? "esiste già" : v.costo ? `${fmt(num(v.costo),0)} €` : "da quotare";
  const gruppi = [];
  for(const [,v] of attive){ const k = breve(v)+"|"+(v.costo?"€":parola(v)); const g = gruppi.find(x=>x.k===k); if(g) g.v.push(v); else gruppi.push({k, v:[v]}); }
  return gruppi.map(g=>g.v.length>1 && !g.v[0].costo ? `${esc(breve(g.v[0]).replace(/a$/,"e"))} ${esc(parola(g.v[0]))} (${g.v.map(x=>esc(fase(x))).join(", ")})`
    : g.v.map(v=>`${esc(breve(v))} ${esc(fase(v))} · ${esc(parola(v))}`).join(" · ")).join(" · ");
}
/* ---------------- le preparazioni (R-11 precisata): in testa, FUORI dalla sequenza delle fasi ----------------
   v16 (PREPARAZIONI §4): riepilogo chiuso in una riga; aperto, una riga per preparazione con TRE controlli soli:
   da fare | esiste già · costo € (e settimane) · Richiedi. In lettura: tipo, «nata dalla f.10 (pre-unione)», stato
   della richiesta, fonte. «+ preparazione ▾» è un menu con i 9 tipi. «Come si paga» lo decide Franco nell'Offerta. */
function preparazioni(T, p, r, c){
  const voci = prepVoci(c).filter(([,v])=>v.on || v.proposta);
  const attive = voci.filter(([,v])=>v.on);
  const nata = v => { const i = c.fasi.findIndex(f=>f.id===v.nata); if(i<0) return v.proposta ? "proposta dal modello" : "aggiunta a mano"; const iu = infoUnione(T,p,r,c,c.fasi[i]); return `nata dalla f.${(i+1)*10}${iu&&iu.preUnione?" (pre-unione)":""}`; };
  const nome = v => v.k==="altra" ? (v.nome||"Altra") : nomePrep(v.k);
  const parola = v => v.stato==="esiste già" ? "esiste già" : v.costo ? `${fmt(num(v.costo),0)} €` : "da quotare";
  const aperto = S.prepAperta || !!S.prepEvid;
  /* l'Esito può portare qui con «prep:<tipo>» o «prep:<chiave>»: si evidenzia la riga giusta */
  if(S.prepEvid && !c.prep[S.prepEvid]){ const x = voci.find(([,v])=>v.k===S.prepEvid && (!S.faseAperta || v.nata===S.faseAperta)) || voci.find(([,v])=>v.k===S.prepEvid); if(x) S.prepEvid = x[0]; }
  const riga = ([key, v]) => { const q = richiesteDellaPrep(T, r.cid, key).filter(x=>x.stato!=="annullata")[0] || null;
    return `<div class="prep-riga${S.prepEvid===key?" prep-riga--evid":""}" data-prep-riga="${esc(key)}">
      <span class="prep-nome"><b>${esc(nome(v))}</b>${v.proposta&&!v.on?` <span class="pr">◇ proposta</span>`:""}<span class="k3"> · ${esc(nata(v))}${v.settimane?` · ${esc(v.settimane)} sett.`:""}${v.chi?` · ${esc(v.chi)}`:""}${v.fonte&&(v.costo||v.stato==="esiste già")?` · ${esc(v.fonte)}${v.fonteIl?" "+esc(v.fonteIl):""}`:""}${v.riferimento?` · ${esc(v.riferimento)}`:""}${q?` · ${esc(q.id)} ${esc(NOMI_STATO_RQ[q.stato]||q.stato)}`:""}</span>
        ${v.storico&&v.stato!=="esiste già"?`<span class="pr" title="dallo storico: ${esc(v.storico.rfq)}">◇ esiste già: ${esc(v.storico.rif)}</span>`:""}</span>
      <span class="seg" role="group" aria-label="Da fare o esiste già">${["da fare","esiste già"].map(st=>`<button type="button" data-pa-gesto="/componente/${r.cid}/preparazioni" data-pa-campi='${esc(JSON.stringify({k:v.k, key, stato:st, on:"1"}))}' aria-pressed="${!!v.on&&v.stato===st}">${st}</button>`).join("")}</span>
      <span class="cb-in prep-costo"><input type="text" data-pa-prep="${r.cid}|${esc(key)}|costo" value="${esc(v.stato==="esiste già"?"":v.costo||"")}" placeholder="${v.stato==="esiste già"?"0":"costo"}" aria-label="Costo di ${esc(nome(v))}" ${v.stato==="esiste già"?"disabled":""}><b>€</b><input type="text" class="prep-sett" data-pa-prep="${r.cid}|${esc(key)}|settimane" value="${esc(v.settimane||"")}" placeholder="sett." aria-label="Settimane di ${esc(nome(v))}" ${v.stato==="esiste già"?"disabled":""}></span>
      <span class="prep-az">${v.on?richiediPrep(T, r, key, v):""}${v.on&&!q?`<button type="button" class="prep-togli" data-pa-gesto="/componente/${r.cid}/preparazioni" data-pa-campi='${esc(JSON.stringify({k:v.k, key, on:"0"}))}' aria-label="Togli ${esc(nome(v))}" title="Togli la preparazione">✕</button>`:""}</span></div>`; };
  const menu = S.prepMenu ? `<span class="seq-menu" role="menu">${PREP_TIPI.map(([k,l])=>k==="altra"
      ? `<form class="prep-altra" data-pa-gesto="/componente/${r.cid}/preparazioni"><input type="hidden" name="k" value="altra"><input type="hidden" name="on" value="1"><input name="nome" placeholder="altra: nome (piantaggio, ganci…)" aria-label="Nome della preparazione" required><button class="ma-btn ma-btn--sm">+</button></form>`
      : btnB(`/componente/${r.cid}/preparazioni`, {k, on:"1", fase_id:S.faseAperta||""}, esc(l), "", 'role="menuitem"')).join("")}</span>` : "";
  return `<details class="prep" ${aperto?"open":""} data-prep-det="1"><summary title="Proposta · dopo A2 · R-11 · R-13"><span class="lab">Preparazioni</span>
      ${attive.length?`<span><b>${attive.length}</b> · ${riepilogoPrep(T,p,r,c,attive)}</span>`:`<span class="k3">nessuna${voci.length?" · "+(voci.length===1?"1 proposta":voci.length+" proposte"):""}</span>`} <span class="k3">${aperto?"▴":"▸"}</span></summary>
    <div class="prep-corpo">${voci.map(riga).join("")||`<p class="k3 piccolo">Nascono dalla fase: «in maschera», saldatura robot, utensile o set mancante (✕), dima, calibro.</p>`}
      <span class="prep-agg"><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-prep-menu="1" aria-expanded="${!!S.prepMenu}">+ preparazione ▾</button>${menu}</span></div></details>`;
}

/* ---------------- la riga di fase ---------------- */
const vF = (f,k) => f.par[k]!=null && f.par[k]!=="" ? f.par[k] : (f.prop[k] && f.prop[k][0]!=="" ? "◇"+f.prop[k][0] : "");
function nomiTerzi(s){ return String(s||"").replace(/^◇/,"").split(",").filter(Boolean).map(k=>nomeBreve(terz(k.trim()).nome)).join(", "); }
function riassuntoFase(T, r, c, f){
  const g = grezzoVal(c), gT = g ? `${g.q} ${g.forma==="lamiera"?"sp "+g.sp:g.forma==="profilo"?g.prof:g.forma==="tubo_tondo"?"Ø"+g.D+"×"+g.sp:""}` : "";
  const t = vF(f,T_), x = k => vF(f,k), j = a => a.filter(Boolean).join(" · ");
  switch(f.p){
    case "laser": return j([gT, x("rett"), x("m")&&x("m")+" m", x("fori")&&x("fori")+" sf."]);
    case "laser_est": return j([nomiTerzi(x("terzi")), x("conto"), x("lotto")&&"lotto "+x("lotto"), f.risp&&`${fmt(f.risp.prezzo,2)} ${f.risp.unita} RISP`]);
    case "sega": case "lasertubo": return j([gT, x("L")&&"L "+x("L"), x("tagli")&&x("tagli")+" tagli", t]);
    case "travi_est": return j([gT||"UPN", x("barre"), nomiTerzi(x("terzi")), x("conto"), f.risp&&`${fmt(f.risp.prezzo,0)} ${f.risp.unita} RISP`]);
    case "piega": return j([x("pieghe")&&x("pieghe")+" pieghe", x("lmax")&&"L max "+x("lmax"), x("op")&&x("op")+" op.", t]);
    case "foratura": case "filettatura": case "smussatura": case "prigionieri": case "piantaggio": case "inserti": return j([x("n")?(x("n")+" × "+(x("mis")||({foratura:"fori",filettatura:"filetti",smussatura:"smussi",prigionieri:"dadi",piantaggio:"pezzi",inserti:"inserti"})[f.p])):x("mis"), t]);
    case "sald_mag": case "sald_robot": case "sald_tig": { const iu = r.figli.length ? infoUnione(T, null, r, c, f) : null;
      return j([iu ? (iu.entrano.length===r.figli.filter(y=>y.cid).length ? `tutti i ${iu.entrano.length} figli` : iu.entrano.length ? iu.entrano.map(e=>e.cod.replace(/^.*(\.\d{3})$/,"$1")).join(" + ") : "nessun figlio") : "", t, x("maschera")==="sì"||x("maschera")==="◇sì"?(x("maschera").startsWith("◇")?"◇":"")+"in maschera":"", x("norma")]); }
    case "tornitura": case "fresatura_est": case "curvatura_tubi": return j([nomiTerzi(x("terzi")), x("lotti")&&"lotti "+x("lotti"), x("conto"), f.risp&&`${fmt(f.risp.prezzo,2)} ${f.risp.unita} RISP`]);
    case "controllo": return j([x("tipo"), x("freq")]);
    case "certificati": return j([x("tipo"), x("per")]);
    case "imballo": return j([x("tipo")]);
    case "trasporto": return j([x("da")&&x("da")+" → "+x("a"), x("mezzo"), f.risp&&`${fmt(f.risp.prezzo,0)} € RISP`]);
    default: return j([x("spec"), x("col"), nomiTerzi(x("terzi")), t]);
  }
}
function faseHtml(T, p, r, c, f, i, ctrlC){
  const sf = statoFase(T,p,r.cid,f), N = (i+1)*10, aperta = S.faseAperta===f.id, nome = valF(f,"nome") || (PROC[f.p] ? PROC[f.p][0] : f.p);
  const iu = r.figli.length ? infoUnione(T,p,r,c,f) : null, su = r.figli.length ? suPreUnione(T,p,r,c,f) : null;
  const segno = iu && iu.preUnione ? `<span class="pp-tag pp-tag--pre" title="unione che viene prima dell'unione principale, con una parte dei figli">pre-unione</span>` : su ? `<span class="pp-tag" title="lavora il pezzo così com'è dopo la pre-unione">sulla pre-unione f.${su.N}</span>` : "";
  const cc = ctrlC.filter(x=>x.fid===f.id), pg = peggiore(cc);
  const chi = PROC[f.p] && PROC[f.p][1]==="est" ? `esterna · ${nomiTerzi(vF(f,"terzi"))||"terzista da scegliere"}` : (()=>{ const m = vF(f,"macc"); return m ? (m.startsWith("◇")?"◇":"")+nomeCentro(f.p==="piega"?"piega":"laser", m.replace("◇","")).split(" · ")[0] : "interna"; })();
  /* M-8: nel «⋯» della fase anche «+ giunto», «dal catalogo» e «Chiedi alla qualità» (prima erano pulsanti sempre visibili) */
  const conCat = ["montaggio","inserti","prigionieri","piantaggio"].includes(f.p) || SALD.has(f.p);
  const voci = [btnB(`/fase/${f.id}/sposta`, {dir:"su"}, "Sposta su (Alt+↑)", "", 'role="menuitem"'), btnB(`/fase/${f.id}/sposta`, {dir:"giu"}, "Sposta giù (Alt+↓)", "", 'role="menuitem"'), btnB(`/fase/${f.id}/togli`, {}, "Togli la fase", "", 'role="menuitem"'),
    ...(SALD.has(f.p) ? [`<button type="button" role="menuitem" data-giunto-apri="${f.id}">+ giunto (cordoni dal disegno)</button>`] : []),
    ...(conCat ? [`<button type="button" role="menuitem" data-cat-apri="componenti|fid:${f.id}" title="Componenti di catalogo usati nella fase (viti, dadi a saldare, inserti): non entrano nell'albero">▤ dal catalogo</button>`] : []),
    ...(SALD.has(f.p) ? [btnB(`/rfq/${T.id}/domanda`, {tipo:"qualita", cid:r.cid, fid:f.id}, "✉ Chiedi alla qualità", "", 'role="menuitem"')] : [])];
  return `<div class="fase${aperta?" fase--aperta":""}${S.evidenzia===f.id?" fase--evidenza":""}" role="listitem" data-fase="${f.id}">
    <div class="fase-testa">
    <div class="fase-t" data-fase-apri="${f.id}" role="button" tabindex="0" aria-expanded="${aperta}" title="${aperta?"Chiudi (Esc)":"Apri la fase (Invio)"}">
      <span class="fase-n">${N}</span>
      <span class="fase-nome"><b>${esc(nome)} ${segno}<span class="k3" style="font-weight:500;font-family:var(--fb);font-size:var(--t-xs)">${esc(chi)}</span></b><span class="fase-riass">${esc(riassuntoFase(T,r,c,f))||"<i>campi da scrivere</i>"}</span></span>
      ${pg?`<i class="pal pal--${pg}" title="controllo peggiore della fase: ${pg}"></i>`:`<span></span>`}
      <span class="fase-stelle" title="dati ★ per Franco">★ ${sf.fatte}/${sf.stelle}${sf.proposte?` · ◇${sf.proposte}`:""}</span>
    </div>
    <span class="fase-az">${aperta||S.faseMenu===f.id?`<button type="button" class="fase-piu" data-fase-menu="${f.id}" aria-expanded="${S.faseMenu===f.id}" aria-label="Altre azioni sulla fase ${N}">⋯</button>${S.faseMenu===f.id?`<span class="seq-menu seq-menu--dx" role="menu">${voci.join("")}</span>`:""}`:""}</span>
    </div>
    ${aperta?corpoFase(T,p,r,c,f,i,sf,cc):""}</div>`;
}
function corpoFase(T, p, r, c, f, i, sf, cc){
  const primaG = c.fasi.find(x=>CON_GREZZO.has(x.p));
  const grezzo = primaG===f ? bloccoGrezzo(T,p,r,c,f) : (CON_GREZZO.has(f.p) || ["piega","foratura","filettatura"].includes(f.p)) && grezzoVal(c) ? `<span class="cb-aiuto">⇐ ${esc(testoGrezzo(grezzoVal(c)))} (dalla f.${(c.fasi.indexOf(primaG)+1)*10})</span>` : "";
  const su = r.figli.length ? suPreUnione(T,p,r,c,f) : null;
  /* v16: il grezzo proposto si accetta con la fase che lo consuma (un pulsante in meno; Invio nel campo resta) */
  const pgz = primaG===f && c.grezzo && Object.keys(c.grezzo.prop).length && !grezzoAccettato(c) ? 1 : 0;
  const sald = SALD.has(f.p) ? bloccoSaldatura(T,p,r,c,f) : UNIONE.has(f.p) && r.figli.length ? `<div class="grz">${bloccoEntrano(T,p,r,c,f,"ciclo")}${f.p==="prigionieri"?`<div class="sald-q"><label class="spunta"><input type="checkbox" data-pa-spunta="${f.id}|maschera|sì" ${valF(f,"maschera")==="sì"?"checked":""}> in maschera</label> ${rigaMaschera(T,p,r,c,f)}</div>`:""}</div>` : "";
  const dima = f.p==="foratura" ? `<div class="sald-q"><label class="spunta"><input type="checkbox" data-pa-spunta="${f.id}|dima|sì" ${valF(f,"dima")==="sì"?"checked":""}> con la dima di foratura</label>${prepVoci(c).some(([,v])=>v.k==="dima"&&v.on&&v.nata===f.id)?` <span class="k3 piccolo">→ preparazione «dima»</span>`:""}</div>` : "";
  const campi = campiDi(f).filter(d=>!(SALD.has(f.p) && ["norma","wps"].includes(d[0])));
  const rich = PROC[f.p] && PROC[f.p][1]==="est" && f.p!=="trasporto" ? apriRichiesta(TRATT.has(f.p)?`verniciatori|${f.p}`:f.p==="laser_est"?"lamiera":`terzista|${f.p}`, TRATT.has(f.p)?"✉ Richiesta ai verniciatori":f.p==="laser_est"?"✉ Richiesta di lamiera":"✉ Richiesta al terzista") : "";
  const cat = valF(f,"catalogo") ? `<span class="cb-aiuto">dal catalogo: <b title="nella fase, non nell'albero (R-10)">${esc(valF(f,"catalogo"))}</b></span>` : "";
  const conf = PROC[f.p] && PROC[f.p][1]==="est" && f.p!=="trasporto" ? confrontoTerzisti(T, r, c, f) : "";
  const risp = f.risp ? `<span class="cb-aiuto">risposta registrata: <b>${esc(f.risp.da)}</b> ${fmt(f.risp.prezzo,2)} ${esc(f.risp.unita||"")} ${chipF("RISP")} ${esc(f.risp.data)}</span>` : "";
  return `<div class="fase-corpo">
    ${grezzo}${sald}${dima}
    <div class="fase-campi">${campi.map(d=>campoB(T,p,r,c,f,d)).join("")}</div>
    ${cat}${risp}${conf}
    ${cc.length?`<div class="ctrl">${cc.map(x=>rigaCtrl(T,x,true)).join("")}</div>`:""}
    ${sf.proposte+pgz||rich?`<div class="fase-acc">${sf.proposte+pgz?btnB(`/fase/${f.id}/accetta`, {}, `Accetta ◇ (${sf.proposte+pgz}) ⇧↵`, "pa-btn", `data-accetta-fase="1" title="Accetta le proposte della fase${pgz?" e il grezzo":""} (la fonte resta visibile)"`):""}${rich}</div>`:""}
  </div>`;
}
/* un campo: dato accettato (testo normale) · proposta ◇ (corsivo prugna, Invio la accetta) · vuoto ★ «da scrivere» */
function campoB(T, p, r, c, f, d){
  const [k, l, u, star, tipo, opz] = d, val = valF(f,k), pr = propF(f,k), fonte = (f.fonti||{})[k];
  const span = tipo==="terzi"||k==="lav"||k==="norma"||k==="per"||k==="cosa"||k==="ops"||k==="masch"||k==="spec" ? "cb--3" : k===T_ ? "" : "";
  const cls = `cb ${span}${star && !val && !pr ? " cb--manca" : ""}${pr ? " cb--prop" : ""}`;
  const lab = `<span class="cb-l" ${val?`title="fonte ${esc(fonte||"L")}"`:""}>${esc(l)}${star?` <span class="stella" title="serve a Franco">★</span>`:""} ${!val&&pr?chipF(pr[1]):""}</span>`;
  if(tipo==="terzi"){
    const att = (val||"").split(",").filter(Boolean), prop = pr ? pr[0].split(",") : [];
    const lav = f.p, cands = [...new Set([...att, ...prop, ...candidatiDi(T, TRATT.has(lav)?"verniciatori":f.p==="laser_est"?"lamiera":"terzista", lav)])];
    return `<div class="${cls}">${lab}<span class="cb-terzi">${cands.map(t=>{ const on = att.includes(t), isP = !val && prop.includes(t); const nuovo = on ? att.filter(x=>x!==t) : [...att, t];
      const q = TRATT.has(lav) && (terz(t).qual||{})[(CLIENTI.find(x=>x.nome===T.testa.cliente)||{}).id]; const ql = q && q.includes(lav);
      return `<button type="button" data-pa-gesto="/fase/${f.id}" data-pa-campi='${JSON.stringify({terzi:nuovo.join(","), fonte:"L"})}' aria-pressed="${on}" class="${isP?"seg--prop":""}" title="${esc(terz(t).lav||"")}${ql?" · qualificato per il cliente":""}">${isP?"◇ ":""}${esc(terz(t).nome)}${ql?" ✓q":""}</button>`; }).join("")}</span>
      <span class="cb-aiuto">${att.length<2?`<span class="warn">2–3 terzisti: ${att.length?"uno solo":"nessuno scelto"}</span>`:`${att.length} terzisti`} · nessuno è scelto al posto tuo</span></div>`;
  }
  if(tipo==="s" && Array.isArray(opz)){
    return `<div class="${cls}">${lab}<span class="seg" role="group" aria-label="${esc(l)}">${opz.map(o=>`<button type="button" data-pa-gesto="/fase/${f.id}" data-pa-campi='${JSON.stringify({[k]:o, fonte:"L"})}' aria-pressed="${val===o}" class="${pr&&pr[0]===o?"seg--prop":""}">${esc(o)}</button>`).join("")}</span></div>`;
  }
  if(tipo==="s"){
    const lista = CENTRI[opz]||[];
    return `<label class="${cls}">${lab}<span class="cb-in"><select data-pa-campo="${f.id}|${k}" aria-label="${esc(l)}"><option value="">${pr?"◇ "+esc(nomeCentro(opz,pr[0]).split(" · ")[0]):"scegli"}</option>${lista.map(([kk,n])=>`<option value="${kk}" ${val===kk?"selected":""}>${esc(n)}</option>`).join("")}</select></span>
      ${pr?`<span class="cb-aiuto"><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-pa-gesto="/fase/${f.id}/accetta" data-pa-campi='${JSON.stringify({campo:k})}' title="${f.p==="laser"?"il più piccolo che taglia questo spessore (PRO-05)":"proposta del sistema"}">usa ◇ ${esc(nomeCentro(opz,pr[0]).split(" · ")[0])}</button></span>`:""}</label>`;
  }
  const v = val!=null ? val : pr ? pr[0] : "";
  const formula = (f.par["f_"+k]||"");
  let aiuto = "";
  if(k===T_){ const lt = leggiTempo(val||(pr&&pr[0])); const ul = ULTIMA[r.cod];
    aiuto = `${lt?(lt.errore?`<span class="warn">${esc(lt.testo)}</span>`:`<small class="k3">(= ${esc(lt.testo)})</small>`):""}${ul&&SALD.has(f.p)?` · <b>riferimento</b>: ultima volta preventivo ${esc(ul.prev)} · consuntivo ${esc(ul.cons)} ⚠ <span class="k3">(non proposto: scrivilo tu)</span>`:""}`; }
  if(k==="lotti"||k==="lotto"){ const L = lottiDi(T); aiuto = `⇐ dalla testata: ${L.lotti.map(l=>fmt(l*(r.qtaProd||1),0)).join(" / ")} pz (lotto × ${r.qtaProd||1} per assieme) · scrivi più scaglioni con «/»`; }
  if(k==="L" && (f.p==="sega"||f.p==="lasertubo")){ const b = barraPreview(T,p,r.cid); if(b) aiuto = `anteprima: ${esc(b.testo)}`; }
  if(k==="lmax" && f.p==="piega"){ const fp = forzaPiega(T,r.cid,f); if(fp) aiuto = `forza ≈ ${esc(fp.testo)} · anteprima`; }
  if(k==="barre" && f.p==="travi_est"){ const w = pesoGrezzo(T,r.cid); if(w) aiuto = `anteprima: ${esc(w.formula)} = ${fmt(w.kg,0)} kg`; }
  if(formula) aiuto = `${esc(formula)} = ${esc(val||"")} ${aiuto}`;
  return `<label class="${cls}${k===T_?" cb--3":""}" title="${esc(formula)}">${lab}<span class="cb-in"><input data-pa-campo="${f.id}|${k}" value="${esc(v)}" ${pr&&val==null?`class="campo--prop" data-prop="${esc(pr[0])}"`:""} placeholder="${k===T_?"20 h · 40 min · 25 pz/h":star?"da scrivere":""}" inputmode="${tipo==="n"?"decimal":"text"}" aria-label="${esc(l)}${k===T_?" (20 h, 40 min o 25 pz/h)":""}">${u?`<b>${esc(u)}</b>`:""}</span>${aiuto?`<span class="cb-aiuto">${aiuto}</span>`:""}</label>`;
}

/* ---------------- il grezzo nella prima fase (2.5.1): banca dati, magazzino, lavorabilità, peso ---------------- */
function bloccoGrezzo(T, p, r, c, f){
  const gz = c.grezzo, gv = grezzoVal(c), acc = grezzoAccettato(c), propT = Object.keys(gz.prop).length && !acc ? testoGrezzo(grezzoVal({grezzo:{par:{}, prop:gz.prop}})) : "";
  const fuori = f.p==="travi_est";
  if(!gv && fuori===false && !propT) {/* niente */}
  const testoIn = acc ? testoGrezzo(gv) : propT;
  const sg = gv ? statoGrezzo(T,p,r.cid) : null, w = gv ? pesoGrezzo(T,r.cid) : null, netto = (SEMI[r.cod]||{}).netto, cap = gv ? capacitaMateriale(T,r.cid) : [];
  const md = gv ? matDi(gv.q) : null;
  const stato = !sg ? "" : (()=>{ const ic = {magazzino:"●", non_basta:"◐", acquistato:"○", mai:"✕"}[sg.stato], et = {magazzino:"In magazzino", non_basta:"In magazzino, non basta", acquistato:"Già acquistato", mai:"Mai acquistato"}[sg.stato];
    const az = sg.stato==="mai" ? apriRichiesta("mp_mai", "✉ Chiedi ai fornitori di materia prima")
      : (sg.stato==="acquistato"||sg.stato==="non_basta") ? apriRichiesta("mp_prezzo", "✉ Chiedi prezzo e tempi") + ` <span class="chip-b" title="Decisione 5: alla RFQ «Chiedi prezzo e tempi», il riordino solo dopo l'ordine. Default, da confermare.">dec. 5</span>` : "";
    const rq = richiesteDi(T,r.cid).find(q=>/^mp_/.test(q.tipo) && q.stato!=="annullata");
    return `<div class="sg sg--${sg.stato}"><b>${ic} ${et}</b><span>${esc(sg.testo.replace(et,"").replace(/^\s*·\s*/,"").replace(/^, /,"")||"")}${sg.prezzoProvv?(sg.testo.replace(et,"").trim()?" · ":"")+esc(sg.prezzoProvv):""}</span>
      ${sg.fb?`<span class="k3">fabbisogno ${esc(sg.fb.testo)}</span>`:""}${sg.risp?`<span>prezzo ${fmt(sg.risp.eur,2)} €/kg · ${esc(sg.risp.forn)} ${chipF("RISP")} ${esc(sg.risp.data)}</span>`:""}
      <span class="dato prova">dati di prova · giacenza al ${esc(GIACENZA_AL.split(" ")[0].slice(0,5))}</span>${sg.stato==="non_basta"?` <span class="chip-b" title="Decisione 2: tre stati, con la variante gialla «non basta» dentro «In magazzino». Default, da confermare.">dec. 2</span>`:""}
      ${rq?`<button type="button" class="ma-st ma-st--prop" data-rq-apri="${esc(rq.bisogno||rq.tipo)}" title="${esc(rq.id)} · ${esc(rq.forn?rq.forn.nome:"")}">richiesta ${esc(NOMI_STATO_RQ[rq.stato]||rq.stato)}</button>`:az}</div>`; })();
  /* v16: sulla lavorabilità ✕ (PIE-04, CUR-01, LAS-03) il pulsante accende la preparazione e prepara la bozza: «→ Set di curvatura · Richiedi» */
  const lavAz = k => { if(!(k.chiedi && k.s==="✕")) return "";
    if(k.prep){ const x = prepVoci(c).find(([,v])=>v.k===k.prep && v.on), fId = (c.fasi.find(y=>y.p===k.lav)||f).id;
      /* ai fornitori: accende la preparazione e apre la sua carta nel cassetto; all'interno: prepara la bozza (R-28) */
      return ` → <b>${esc(nomePrep(k.prep))}</b> ${x ? richiediPrep(T, r, x[0], x[1]) : k.chiedi==="attrezzatura" ? btnB(`/componente/${r.cid}/preparazioni`, {k:k.prep, on:"1", fase_id:fId, apri:"attrezzatura"}, "Richiedi", "pa-btn", `title="Accende la preparazione «${esc(nomePrep(k.prep))}» e apre la carta nel cassetto ✉: prepari la bozza per ogni fornitore"`)
        : btnB(`/rfq/${T.id}/domanda`, {tipo:k.chiedi, cid:r.cid, fid:fId, accendi:"1", k:k.prep}, "Richiedi all'officina", "pa-btn", `title="Accende la preparazione «${esc(nomePrep(k.prep))}» e prepara la bozza: niente parte da solo"`)}`; }
    return " "+btnB(`/rfq/${T.id}/domanda`, {tipo:k.chiedi, cid:r.cid, fid:f.id}, k.chiedi==="qualita"?"✉ Chiedi alla qualità":"✉ Chiedi all'officina", "pa-btn"); };
  const lav = cap.length ? `<div class="lav"><span class="lab">Lavorabilità ⚑</span>${cap.map(k=>`<span class="${k.s==="✕"?"lav-ko":""}">${esc(k.lav)} <b>${k.s}</b> ${esc(k.testo)}${lavAz(k)}</span>`).join("")}${datoProva("inventato")}${(a2Di(T).capOfficina||{})[r.cid]?`<span>officina: ${esc(a2Di(T).capOfficina[r.cid])} ${chipF("RISP")}</span>`:""}</div>` : "";
  const pa = gv ? pesoAnteprima(T, r) : null;
  const sfrido = pa && pa.sfrido!=null ? `sfrido ${fmt(pa.sfrido,2)} kg` : "sfrido: da scrivere";
  const peso = gv ? `<span class="cb-aiuto">Peso: grezzo ${w?`<b>${fmt(w.kg,2)} kg</b> (${esc(w.formula)})`:"da calcolare: mancano le misure"} · netto ${netto?`<span class="pr">${fmt(netto[0],2)} kg</span> ${chipF(netto[1])}`:"—"} · ${esc(sfrido)} <span class="dato prova" title="Calcolo del finto server B (proposta R-37): non è un dato del backend">anteprima R-37</span>${pa&&pa.pes01?` <span class="warn">PES-01 grezzo &lt; finito: controlla le misure</span>`:""}</span>` : "";
  const fornit = fuori ? `<span class="cb-aiuto">Materiale: <b>nostro</b> (le barre le compriamo noi: conto lavoro) · la regola: lo mette il terzista solo se prima non c'è una nostra fase</span>` : "";
  return `<div class="grz" aria-label="Grezzo">
    <div class="grz-r1"><label class="cb${!testoIn?" cb--manca":""}${propT?" cb--prop":""}"><span class="cb-l" title="R-23 R-25 · fonte ${esc(acc?(gz.fonte==="L"?"L":"CART"):"")}">Grezzo <span class="stella">★</span> ${!acc&&propT?chipF((Object.values(gz.prop)[0]||[])[1]):""}</span>
      <span class="cb-in"><input data-pa-grezzo="${f.id}" value="${esc(testoIn)}" ${propT?`class="campo--prop" data-prop="${esc(propT)}"`:""} placeholder="s355j2 15 · upn160 · 355 tubo 127x10 · gsx9486 ii" aria-label="Grezzo: qualità e misura" autocomplete="off"></span></label>
      <span style="display:flex;gap:4px"><button type="button" class="ma-btn ma-btn--sm" data-cat-apri="materiali|${f.id}" title="Banca dati dei materiali: giacenza e lavorabilità (L)">▤ Materiali</button></span></div>
    ${gv?`<div class="grz-esito">→ <b>${esc(testoGrezzo(gv))}</b>${(grezzoDi(gv)||md)?` · ${grezzoDi(gv)?esc(grezzoDi(gv).norma):/^tubo/.test(gv.forma||"")?`<span class="warn" title="RL-13: la norma del tubo (EN 10210 o EN 10219) si legge sul disegno o sul certificato">norma del tubo da verificare</span>`:esc(md.q.norma)}${md?" · "+esc(md.g.split(" ")[0]):""}`:""}${gz.equiv?` <span class="ma-st ma-st--ver" title="equivalenza del cliente proposta: da confermare dalla qualità">equivalenza «${esc(gz.equiv.nome)}» da confermare</span> ${btnB(`/rfq/${T.id}/domanda`, {tipo:"qualita", cid:r.cid, fid:f.id, testo:`Equivalenza «${gz.equiv.nome}» → ${gz.equiv.q}: confermate?`}, "✉ Chiedi alla qualità", "pa-btn")}`:""}${certRichiesto(T)?` · cert. richiesto <b>${esc(certRichiesto(T))}</b> ${chipF("ORD")}`:""}</div>`:""}
    ${stato}${lav}${peso}${fornit}
    <label class="spunta"><input type="checkbox" data-diverso="${r.cid}" ${S.diverso&&S.diverso[r.cid]?"checked":""}> Si ottiene diversamente dal disegno</label>${S.diverso&&S.diverso[r.cid]?` <label class="spunta"><input type="checkbox"> da dire al cliente</label>`:""}
  </div>`;
}

/* ---------------- la saldatura (fase del padre; decisione 4, default) ----------------
   v16 rev. (M3 di Lino): un solo modo di dire «prima» o «insieme»: «Entrano» e «+ fase di unione prima». «Che cosa si
   salda» si legge da «Entrano»; «puntato prima» compare solo se nella fase entra un commerciale; «+ giunto», il catalogo
   e «Chiedi alla qualità» stanno nel «⋯» della fase (M-8). In una pre-unione si vedono solo i figli che entrano. */
function bloccoSaldatura(T, p, r, c, f){
  const spunta = (k, l, si, extra) => { const v = valF(f,k), pr = propF(f,k);
    return `<label class="spunta"><input type="checkbox" data-pa-spunta="${f.id}|${k}|${si}" ${v===si?"checked":""}> ${l}</label>${pr&&v==null?` <span class="pr" title="proposta del sistema · fonte ${esc(NOME_FONTE[pr[1]]||pr[1])}">◇ proposta: ${esc(pr[0])}</span>`:""}${extra||""}`; };
  const giunti = f.giunti||[], tot = giunti.reduce((a,x)=>a+(x.conv.mm||0),0), kg = giunti.reduce((a,x)=>a+(x.conv.a&&x.conv.mm?x.conv.a*x.conv.a*x.conv.mm*7.85e-6:0),0);
  const ant = S.giuntoTesto ? convertiCordone(S.giuntoTesto) : null;
  const cw = coperturaWPS(T,p,r.cid,f), formGiunto = S.giuntoForm===f.id;
  return `<div class="grz" aria-label="Saldatura">
    <div class="sald-q"><span class="lab">Si salda</span><span class="sald-ogg">${esc(oggettoSald(T,p,r,c,f))}</span></div>
    ${r.figli.length?`<div class="sald-entra">${bloccoEntrano(T,p,r,c,f,"ciclo")}</div>`:""}
    <div class="sald-q">${spunta("maschera","in maschera","sì", " "+rigaMaschera(T,p,r,c,f))}</div>
    ${entraCommerciale(T,p,r,c,f)?`<div class="sald-q">${spunta("puntato","puntato prima · compreso nel tempo","compreso", "")}</div>`:""}
    <div class="fase-campi">${campoB(T,p,r,c,f,["norma","Norma e livello","",0,"t"])}${campoB(T,p,r,c,f,["wps","WPS ⚑","",0,"t"])}</div>
    <span class="cb-aiuto">${cw?esc(cw.testo):""} · controllo VT 100 % ◇ · ${datoProva("inventato")}</span>
    ${giunti.length||formGiunto?`<div class="giunti"><span class="lab" title="R-30">Giunti (facoltativi) · come sul disegno</span>
      ${giunti.length?`<ol>${giunti.map((x,j)=>`<li><span class="mono">${esc(x.riga)}</span> → ${esc(x.conv.testo)} ${btnB(`/fase/${f.id}/giunto`, {togli:String(j)}, "✕", "ma-btn--ghost", 'aria-label="Togli il giunto"')}</li>`).join("")}</ol><span class="cb-aiuto">totale ${fmt(tot,0)} mm di cordone · ≈ ${fmt(kg,3)} kg di apporto · ${giunti.reduce((a,x)=>a+(x.conv.n||0),0)} inneschi</span>`:""}
      <form data-pa-gesto="/fase/${f.id}/giunto"><input name="riga" id="giunto-in" value="${esc(S.giuntoTesto||"")}" placeholder="z5 51-152 L1016 · a4 4×120 · circ Ø127 ×4" aria-label="Giunto"><button class="ma-btn ma-btn--sm">+ giunto</button>
        ${ant?`<span class="giunto-ant">${ant.ok?"→ ":""}${esc(ant.testo)} · anteprima</span>`:""}</form></div>`:""}
  </div>`;
}

/* ---------------- aggiungi una fase (/) e i controlli ---------------- */
function aggiungiFase(T, p, r, c){
  const ultima = c && c.fasi.length ? c.fasi[c.fasi.length-1] : null;
  const dopo = ultima ? (SEGUE[ultima.p]||[]).slice(0,3) : ["laser","sega","sald_mag"];
  const q = (S.aggFase||"").toLowerCase().replace(/^\//,"");
  const trov = q ? Object.entries(PROC).filter(([k,v])=>v[0].toLowerCase().includes(q)||k.includes(q)).slice(0,6) : [];
  return `<div class="agg-fase"><span class="lab">Aggiungi una fase</span><input id="agg-fase" value="${esc(S.aggFase||"")}" placeholder="/ cerca: /pie, /zinc, /sald" aria-label="Cerca e aggiungi una fase" autocomplete="off">
    <span class="agg-fase-l">${trov.map(([k,v])=>btnB(`/componente/${r.cid}/ciclo/fase`, {p:k, dopo: ultima?ultima.id:""}, "+ "+esc(v[0]))).join("")}
    ${!q?`<span class="k3 piccolo">di solito dopo:</span>${dopo.map(k=>btnB(`/componente/${r.cid}/ciclo/fase`, {p:k, dopo: ultima?ultima.id:""}, "+ "+esc(PROC[k][0]), "ma-btn--ghost")).join("")}`:""}</span></div>`;
}
function azioneCtrl(T, a){
  if(!a) return "";
  if(a.richiesta){ const TR = TIPI_RICHIESTA[a.richiesta];
    if(TR && TR.esterni){ const x = a.fid ? faseDelThread(threadAperto(), a.fid) : null, lav = x ? x.f.p : "";
      return apriRichiesta(a.richiesta==="lamiera" ? "lamiera" : /^mp_/.test(a.richiesta) ? a.richiesta : a.richiesta==="attrezzatura" ? "attrezzatura" : `${a.richiesta}|${lav}`, "✉ "+esc(a.l)); }
    return btnB(`/rfq/${T.id}/domanda`, {tipo:a.richiesta, cid:a.cid||"", fid:a.fid||""}, "✉ "+esc(a.l), "pa-btn"); }
  if(a.cassetto) return `<button type="button" class="ma-btn ma-btn--sm" data-cassetto="${a.cassetto}">→ ${esc(a.l)}</button>`;
  if(a.staz) return `<button type="button" class="ma-btn ma-btn--sm" data-vai="${a.staz}|${a.cid||""}|${a.fid||""}|${a.vista||""}">→ ${esc(a.l)}</button>`;
  return "";
}
function rigaCtrl(T, x, locale){
  const testo = locale && x.rcod ? x.testo.replace(x.rcod+": ","").replace(new RegExp("^"+x.rcod.replace(/[.*+?^${}()|[\]\\]/g,"\\$&")+" "),"") : x.testo;
  const az = locale && x.azione && x.azione.staz===3 && x.azione.cid===S.cicloCid && (!x.azione.fid || x.azione.fid===S.faseAperta) ? null : x.azione;
  return `<div class="ctrl-riga"><span class="liv liv--${x.liv}">${NOME_LIV[x.liv]||x.liv}</span><code>${esc(x.cod)}</code><span>${esc(testo)}${x.inventato?" "+datoProva("dati di prova"):""}</span>${azioneCtrl(T,az)}</div>`; }
/* v16: i controlli del componente stanno in un riquadro chiuso (l'elenco intero è anche nel cassetto Controlli, K):
   quelli di una fase si vedono già dentro la fase aperta */
function controlliComponente(T, cc){
  if(!cc.length) return `<div class="ctrl" aria-label="Controlli del componente"><span class="lab" title="R-35">Controlli</span><span class="piccolo">✓ nessun controllo aperto</span></div>`;
  const pg = peggiore(cc);
  return `<details class="ctrl ctrl-det" aria-label="Controlli del componente" ${S.ctrlAperti?"open":""} data-ctrl-det="1"><summary title="R-35"><span class="lab">Controlli</span> <b>${cc.length}</b>${pg?` <i class="pal pal--${pg}"></i> <span class="k3">il peggiore: ${esc(NOME_LIV[pg]||pg)}</span>`:""}</summary>${cc.map(x=>rigaCtrl(T,x,true)).join("")}</details>`;
}

/* ---------------- la barra di conferma: ultima riga della colonna, un solo primario (C2, M10, B5) ---------------- */
function barraConferma(T, p, r, c, fare, pre, ctrlC){
  const stati = c.fasi.map(f=>statoFase(T,p,r.cid,f)), stelle = stati.reduce((a,s)=>a+s.stelle,0), fatte = stati.reduce((a,s)=>a+s.fatte,0);
  const pend = stati.reduce((a,s)=>a+s.proposte,0) + (Object.keys(c.grezzo.prop).length && !grezzoAccettato(c) && c.fasi.some(f=>CON_GREZZO.has(f.p)) ? 1 : 0);
  const nx = prossimoDaFare(T,p,r.cid), ko = ctrlC.filter(x=>x.liv==="KO"&&!x.inventato).length;
  const firma = firmaCiclo(T, r.cid), elenco = proposteCiclo(T, r.cid);
  const verso = nx ? ` e vai a ${esc(suffisso(nx.cod,pre))}` : "";
  const prim = c.stato==="confermato"
    ? (nx ? `<button type="button" class="ma-btn ma-btn--pri" data-ciclo-cid="${nx.cid}" data-ciclo-rif="${esc(nx.rif)}">Vai a ${esc(suffisso(nx.cod,pre))} →</button>` : `<button type="button" class="ma-btn ma-btn--pri" data-staz="4">Tutti i cicli confermati · Sequenza →</button>`)
    : btnB(`/componente/${r.cid}/ciclo/conferma`, {accettaTutto: pend?"1":"", vai:"1", firma}, pend ? `Accetta ${pend} ◇ · conferma${nx&&suffisso(nx.cod,pre).length<=8?" → "+esc(suffisso(nx.cod,pre)):nx?" →":""} ⌃↵` : `Conferma${nx&&suffisso(nx.cod,pre).length<=8?" → "+esc(suffisso(nx.cod,pre)):nx?" →":""} ⌃↵`, "ma-btn--pri", `data-conferma-ciclo="1" ${pend?'aria-describedby="bc-proposte"':""} title="${pend?`Accetta le ${pend} proposte ◇ elencate (la fonte resta visibile) e conferma il ciclo, con la firma di quello che hai letto`:"Conferma il ciclo, con la firma di quello che hai letto"}${verso}"`);
  return `<div class="barra-conf" role="group" aria-label="Conferma del componente">
    <span class="bc-stato">${c.stato==="confermato"?`<span class="ma-st ma-st--ok ma-st--piatto">✓ confermato ${esc(c.quando||"")}</span>`:`<span class="ma-st ma-st--prop ma-st--piatto">◐ bozza</span>`} <span class="mono">★ ${fatte}/${stelle}</span>${pend?` <span class="pr bc-proposte" id="bc-proposte" title="${esc(elenco.join(" · "))}">${pend} da accettare: ${esc(elenco.slice(0,3).join(" · "))}${elenco.length>3?` · +${elenco.length-3}`:""}</span>`:""}${ko?` <span class="liv liv--KO">${ko} KO</span>`:""}${c.riaperto?` <span class="k3">${esc(c.riaperto)}</span>`:""}</span>
    <span class="bc-nav">${navComp(fare, r)}</span>
    ${prim}
    <button type="button" class="ma-btn ma-btn--sm" data-bc-menu="1" aria-expanded="${!!S.bcMenu}" aria-label="Altre azioni">▾</button>
    ${S.bcMenu?`<div class="bc-menu" role="menu">${c.stato!=="confermato"?btnB(`/componente/${r.cid}/ciclo/conferma`, {accettaTutto: pend?"1":"", firma}, "Conferma senza spostarti", "", 'role="menuitem"'):""}${c.stato==="confermato"?btnB(`/componente/${r.cid}/ciclo/riapri`, {}, "Riapri il ciclo", "", 'role="menuitem"'):""}<button type="button" data-copia-da="1" role="menuitem">⧉ Copia da…</button></div>`:""}
  </div>`;
}

/* ---------------- i gesti di presentazione del Ciclo (aprire, filtrare, zoom) ---------------- */
document.addEventListener("click", e=>{
  const t = e.target.closest && e.target.closest("[data-ciclo-rif],[data-ciclo-filtro],[data-fase-apri],[data-dis-zoom],[data-dis-nascondi],[data-copia-da],[data-bc-menu],[data-ciclo-nav],button[data-vista420],[data-nota-fase],[data-ciclo-blocco],[data-vai],[data-dis-ciclo],[data-dis-menu],[data-fase-menu],[data-prep-menu],[data-vedi-albero],[data-pa-visore],[data-giunto-apri]");
  if(!t) return;
  if(e.target.closest("[data-pa-gesto]") && !t.hasAttribute("data-pa-gesto")) return;   // i gesti B dentro la riga vanno al ponte B
  const D = t.dataset;
  if(D.cicloRif!==undefined && !D.paGesto){
    if(e.shiftKey && D.cicloCid){ const s = S.cicloSel || (S.cicloSel = new Set()); if(s.has(D.cicloCid)) s.delete(D.cicloCid); else s.add(D.cicloCid); render(); return; }
    S.cicloRif = D.cicloRif; S.cicloCid = D.cicloCid || null; S.faseAperta = undefined; S.copiaDa = false; S.bcMenu = false; S.staz = 3; S.disCiclo = "componente"; S.prepEvid = null; render(); const sc = document.getElementById("cl-scroll"); if(sc) sc.scrollTop = 0; return; }
  if(D.cicloFiltro){ S.cicloFiltro = D.cicloFiltro; render(); return; }
  if(D.faseMenu){ S.faseMenu = S.faseMenu===D.faseMenu ? null : D.faseMenu; render(); return; }
  if(D.paVisore){ S.dialogo = {tipo:"visore", file:D.paFile, allegato:D.paAllegato, sha:D.paSha}; render(); return; }
  if(D.giuntoApri){ S.giuntoForm = D.giuntoApri; S.faseMenu = null; S.fuoco = "#giunto-in"; render(); if(typeof dopoRender==="function") dopoRender(); return; }
  if(D.faseApri){ if(e.target.closest("[data-fase-menu],.seq-menu")) return; S.faseAperta = S.faseAperta===D.faseApri ? null : D.faseApri; S.giuntoTesto = ""; S.faseMenu = null; render(); return; }
  if(D.disCiclo){ S.disCiclo = D.disCiclo; render(); return; }
  if(D.disMenu){ S.disMenu = !S.disMenu; render(); return; }
  if(D.prepMenu){ S.prepMenu = !S.prepMenu; render(); return; }
  if(D.vediAlbero){ const [tipo, cid, fid] = D.vediAlbero.split("|"); PA.avviso = null; S.seqScelto = {tipo, cid, fid, padre:null}; S.staz = 4; render(); return; }
  if(D.disZoom!==undefined){ const z = +D.disZoom; S.disZoom = z===0 ? 1 : Math.min(2, Math.max(.5, (S.disZoom||1) + z*.25)); render(); return; }
  if(D.disNascondi){ S.disegnoNascosto = !S.disegnoNascosto; S.disMenu = false; render(); return; }
  if(D.copiaDa!==undefined){ S.copiaDa = D.copiaDa==="1"; S.bcMenu = false; render(); return; }
  if(D.bcMenu){ S.bcMenu = !S.bcMenu; render(); return; }
  if(D.cicloNav){ const T = threadAperto(), p = prodottoScelto(T), {fare} = elencoCiclo(T,p); const i = fare.findIndex(r=>r.cid===S.cicloCid); const nx = fare[(i + +D.cicloNav + fare.length) % fare.length];
    if(nx){ S.cicloCid = nx.cid; S.cicloRif = nx.rif; S.faseAperta = undefined; S.disCiclo = "componente"; render(); } return; }
  /* solo i tre pulsanti del segmento: la griglia porta anche lei data-vista420 (v16: prima ogni clic nel foglio ridisegnava,
     e una spunta perdeva il suo «change» perché il campo spariva prima dell'evento) */
  if(D.vista420 && t.tagName==="BUTTON"){ S.vista420 = D.vista420; render(); return; }
  if(D.notaFase){ const i = document.getElementById("nota-in"); if(i) i.focus(); return; }
  if(D.cicloBlocco){ S.blocco = {staz:3, cids:[...(S.cicloSel||[])], modello:null}; render(); return; }
  if(D.vai){ const [st, cid, fid, vista, cerca] = D.vai.split("|"); if(S.staz!==+st){ PA.avviso=null; S.blocco=null; S.menuMod=null; } S.staz = +st; if(cid){ S.cicloCid = cid; S.cicloRif = null; } if(fid){ S.faseAperta = fid; S.evidenzia = fid; }
    /* v16: il quarto campo può essere «prep:<chiave>» = apri la riga della preparazione (SPEC §8.3) */
    if(vista && vista.startsWith("prep:")){ S.prepEvid = vista.slice(5); S.prepAperta = true; } else if(vista) S.vista = vista; else S.prepEvid = null;
    if(+st===3) S.disCiclo = cid ? "componente" : "prodotto";
    if(cerca){ S.aggFase = "/"+cerca; S.fuoco = "#agg-fase"; } S.cassetto = S.cassetto && window.innerWidth<1280 ? null : S.cassetto; render();
    const el = vista && vista.startsWith("prep:") ? document.querySelector(`.prep [data-prep-riga="${vista.slice(5)}"]`) : fid ? document.querySelector(`[data-fase="${fid}"]`) : cerca ? document.getElementById("agg-fase") : null; if(el && el.scrollIntoView) try{ el.scrollIntoView({block:"nearest"}); }catch(err){}
    if(cerca && !el){ PA.avviso = {esito:"no", testo:"Il componente non ha un ciclo dove aggiungere la fase: decidi prima come si fa in ② Come."}; render(); } return; }
});
document.addEventListener("toggle", e=>{ const t = e.target; if(t && t.dataset){ if(t.dataset.prepDet) S.prepAperta = t.open; if(t.dataset.ctrlDet) S.ctrlAperti = t.open; } }, true);
document.addEventListener("click", e=>{ const t = e.target.closest && e.target.closest("[data-req-toggle]"); if(t){ S.reqAperti = !S.reqAperti; render(); } });
/* un campo cambiato: una scrittura della fase (R-36); «=0,8*2,36» si calcola qui e la formula resta nel title [FE] */
function valoreEspressione(s){
  const e = String(s).slice(1).replace(/,/g,".");
  if(!/^[\d.\s+\-*/()]+$/.test(e)) return null;
  try { const v = Function(`"use strict";return (${e})`)(); return isFinite(v) ? Math.round(v*100)/100 : null; } catch(err){ return null; }
}
function scriviCampoVista(el){
  const [fid, k] = el.dataset.paCampo.split("|"), v = el.value.trim();
  if(el.dataset.prop!==undefined && v===el.dataset.prop){ eseguiB(`/fase/${fid}/accetta`, {campo:k}); return; }
  if(v.startsWith("=")){ const r = valoreEspressione(v); if(r==null){ PA.avviso = {esito:"no", testo:`L'espressione «${v}» non si calcola: usa numeri e + − × ÷.`}; render(); return; }
    eseguiB(`/fase/${fid}`, {[k]: String(r).replace(".",","), ["f_"+k]: v, fonte:"CALC"}); return; }
  eseguiB(`/fase/${fid}`, {[k]: v, fonte:"L"});
}
document.addEventListener("change", e=>{
  const el = e.target; if(!el || !el.dataset) return;
  if(el.dataset.paCampo){ scriviCampoVista(el); return; }
  if(el.dataset.paSpunta){ const [fid,k,si] = el.dataset.paSpunta.split("|"); eseguiB(`/fase/${fid}`, {[k]: el.checked ? si : "no", fonte:"L"}); return; }
  if(el.dataset.paPrep){ const [cid,key,campo] = el.dataset.paPrep.split("|"); const k = key.split("@")[0]; eseguiB(`/componente/${cid}/preparazioni`, campo==="on" ? {k, key, on: el.checked?"1":"0"} : {k, key, [campo]: el.value.trim(), on:"1"}); return; }
  if(el.dataset.paGrezzo){ const fid = el.dataset.paGrezzo, v = el.value.trim(); if(el.dataset.prop!==undefined && v===el.dataset.prop) eseguiB(`/fase/${fid}/materiale`, {accetta:"1"}); else eseguiB(`/fase/${fid}/materiale`, {cerca:v}); return; }
  if(el.dataset.diverso){ S.diverso = S.diverso || {}; S.diverso[el.dataset.diverso] = el.checked; render(); return; }
  if(el.dataset.cicloFiltroSel){ S.cicloFiltro = el.value || null; render(); return; }
});
document.addEventListener("input", e=>{
  const el = e.target;
  if(el.id==="agg-fase"){ S.aggFase = el.value; const pos = el.selectionStart; render(); const i = document.getElementById("agg-fase"); if(i){ i.focus(); try{ i.setSelectionRange(pos,pos); }catch(err){} } }
  if(el.id==="giunto-in"){ S.giuntoTesto = el.value; const pos = el.selectionStart; render(); const i = document.getElementById("giunto-in"); if(i){ i.focus(); try{ i.setSelectionRange(pos,pos); }catch(err){} } }
});

/* dopo un gesto B del Ciclo: «Conferma e vai» porta al prossimo da fare, con la prima fase da completare aperta */
function dopoCiclo(url, r, campi){
  if(r.esito!=="ok") return;
  if(/\/ciclo\/conferma$/.test(url)){ S.bcMenu = false; if(campi && campi.vai==="1"){ const T = threadAperto(), p = prodottoScelto(T), nx = prossimoDaFare(T,p,S.cicloCid); if(nx){ S.cicloCid = nx.cid; S.cicloRif = nx.rif; S.faseAperta = undefined; S.disCiclo = "componente"; } } }
  if(/\/(sposta|togli)$/.test(url)) S.faseMenu = null;
  if(/\/preparazioni$/.test(url) && campi && campi.k && !campi.key) S.prepMenu = false;
  if(/\/ciclo\/(copia|modello)$/.test(url)){ S.copiaDa = false; S.faseAperta = undefined; }
  if(/\/fornitura$/.test(url) && S.staz===3) S.faseAperta = undefined;
  if(/\/giunto$/.test(url)){ S.giuntoTesto = ""; S.giuntoForm = null; }
  if(/\/nota$/.test(url)) { const i = document.getElementById("nota-in"); if(i) i.value = ""; }
}

/* il blocco nel Ciclo: un modello per i componenti selezionati (Maiusc+clic nell'elenco), con l'anteprima */
function bloccoModelli(T, p){
  const b = S.blocco, rr = righeB(T,p).filter(r=>b.cids.includes(r.cid)), conf = rr.filter(r=>{ const c = cicloB(T,r.cid); return c && c.stato==="confermato"; });
  return `<section class="blocco-b" aria-label="Anteprima del blocco"><b>Cambierà ${rr.length} ${rr.length===1?"componente":"componenti"}:</b>
    <ul>${rr.map(r=>`<li><span class="mono">${esc(r.cod)}</span> ${esc(r.descr)}${cicloB(T,r.cid)?` <span class="k3">(ora ${esc(cicloB(T,r.cid).modello||"—")} · ${esc(cicloB(T,r.cid).stato)})</span>`:""}</li>`).join("")}</ul>
    <div class="acts"><span class="lab">modello</span><span class="seg" role="group" aria-label="Modello per il blocco">${["T1","T1b","T3","T4","T4b","T5"].map(k=>`<button type="button" data-blocco-mod="${k}" aria-pressed="${b.modello===k}" title="modello ${k}">${esc(MODELLI[k].nome)}</button>`).join("")}</span>
      ${conf.length?`<label class="spunta"><input type="checkbox" data-blocco-riapri="1" ${b.riapri?"checked":""}> riapri anche i confermati (${conf.length})</label>`:""}
      ${btnB(`/rfq/${T.id}/come/blocco`, {cids:b.cids.join(","), modello:b.modello||"", riapri:b.riapri?"1":""}, "Applica (Invio)", "", b.modello?"":"disabled")}
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-blocco-annulla="1">Annulla (Esc)</button>
      <span class="k3 piccolo">Una scrittura per fase sotto un gesto solo, con l'esito per riga (R-36). Il blocco non tocca l'albero.</span></div></section>`;
}
document.addEventListener("click", e=>{ const t = e.target.closest && e.target.closest("[data-blocco-mod]"); if(t){ S.blocco.modello = t.dataset.bloccoMod; render(); } });
document.addEventListener("change", e=>{ if(e.target.dataset && e.target.dataset.bloccoRiapri){ S.blocco.riapri = e.target.checked; render(); } });

/* il confronto dei terzisti (T10): una riga per terzista con le sue risposte; il prezzo è in sola lettura (è di Franco),
   la scelta e il motivo li scrive Lino. Per un trattamento in più passi lo dice e rimanda all'altra fase. */
function confrontoTerzisti(T, r, c, f){
  const qq = [...(typeof QUOTAZIONI!=="undefined"?QUOTAZIONI:[]), ...QUOTAZIONI_B].filter(x=>x.cod===r.cod && x.p===f.p);
  const scelti = String(valF(f,"terzi")||"").split(",").filter(Boolean);
  const ids = [...new Set([...scelti, ...qq.map(x=>x.terz)])];
  const passi = TRATT.has(f.p) ? c.fasi.filter(x=>TRATT.has(x.p)) : [];
  const passo = passi.length>1 ? `<span class="cb-aiuto">trattamento in ${passi.length} passi: ${passi.map(x=>`${x===f?"<b>":""}f.${(c.fasi.indexOf(x)+1)*10} ${esc(PROC[x.p][0])}${x===f?"</b>":""}`).join(" → ")} · una sola richiesta ai verniciatori per tutti i passi</span>` : "";
  if(!ids.length) return passo;
  const sc = valF(f,"scelto");
  /* RIPROVA: una quotazione per riga (giri e storico), gli scaglioni non si spezzano a metà.
     RL-08: il materiale dichiarato accanto al prezzo, giallo se non è quello del disegno; RL-14: lo storico si dice */
  const mc = materialeComp(T, r.cid);
  const righe = ids.map(id=>{ const qs = qq.filter(x=>x.terz===id && (!x.tk || x.tk===T.key));
    const mat = qs.map(x=>x.materiale ? (mc && matDiscorde(x.materiale, mc.q) ? `<span class="cf-mat--no" title="MAT-05: non confrontabile con il disegno ${esc(mc.q)}">⚠ ${esc(x.materiale)} ≠ ${esc(mc.q)}</span>` : esc(x.materiale)) : `<span class="k3">—</span>`).join("<br>");
    return `<tr${sc===id?' class="cf-scelto"':""}><td>${esc(terz(id).nome)}</td><td class="mono">${qs.length?qs.map(x=>`<span class="cf-q">${x.lotti.map(([l,e])=>`<span class="cf-l">${fmt(l,0)} pz: ${fmt(e,2)} €</span>`).join(" · ")}${x.storico?` <span class="k3">(storico ${esc(x.storico)})</span>`:x.giro?` <span class="k3">(giro ${x.giro})</span>`:""}</span>`).join("<br>"):`<span class="k3">nessuna risposta</span>`}</td>
      <td>${qs.length?mat:"—"}</td><td class="k3">${qs.map(x=>esc(x.data)).join("<br>")||"—"}</td>
      <td>${btnB(`/fase/${f.id}`, {scelto: sc===id?"":id, fonte:"L"}, sc===id?"✓ scelto":"scegli", sc===id?"":"ma-btn--ghost", `aria-pressed="${sc===id}"`)}</td></tr>`; }).join("");
  return `${passo}<div class="cf"><span class="lab" title="R-13 R-17">Confronto dei terzisti</span>
    <div class="wrap-tb"><table class="cf-tab"><thead><tr><th>Terzista</th><th>Prezzo (sola lettura · Franco)</th><th>Materiale dichiarato</th><th>Data</th><th>Scelta</th></tr></thead><tbody>${righe}</tbody></table></div>
    <label class="cb cb--3"><span class="cb-l">Motivo della scelta</span><span class="cb-in"><input data-pa-campo="${f.id}|motivo" value="${esc(valF(f,"motivo")||"")}" placeholder="es. qualificato dal cliente, tempi più corti" aria-label="Motivo della scelta"></span></label></div>`;
}

/* ---------------- v16 · entrare nella stazione ③: il primo disegno è il prodotto finito (S-04) ----------------
   Dalla barra delle stazioni, da «Vai al Ciclo →» o con G 3 si apre «Prodotto finito»; da «Apri nel Ciclo →» della
   Struttura (che porta il componente) si apre subito il componente. È solo presentazione: nessun gesto parte. */
document.addEventListener("click", e=>{
  const b = e.target.closest && e.target.closest("[data-staz]"); if(!b || b.dataset.staz!=="3" || S.tab!=="rfq") return;
  S.disCiclo = b.dataset.cicloCid ? "componente" : "prodotto"; S.prepEvid = null;
}, true);
let c2AttesaG = false;
document.addEventListener("keydown", e=>{
  if(S.tab!=="rfq") return; const t = e.target; if(t && (t.tagName==="INPUT"||t.tagName==="TEXTAREA"||t.tagName==="SELECT")) return;
  if(c2AttesaG){ c2AttesaG = false; if(e.key==="3" && S.staz!==3){ S.disCiclo = "prodotto"; S.prepEvid = null; } return; }
  if(e.key==="g"||e.key==="G") c2AttesaG = true;
}, true);

/* ---------------- i tasti del Ciclo (chiamati da tastiStazione di 58) ---------------- */
function tastiCiclo(e){
  const k = e.key;
  if(S.blocco && S.blocco.staz===3){ if(k==="Enter"){ cliccaSe(".blocco-b [data-pa-gesto]"); e.preventDefault(); } return; }
  if(k==="ArrowRight"||k==="ArrowLeft"){ cliccaSe(`[data-ciclo-nav="${k==="ArrowRight"?1:-1}"]`); e.preventDefault(); return; }
  if(e.altKey && (k==="ArrowUp"||k==="ArrowDown") && S.faseAperta){ eseguiB(`/fase/${S.faseAperta}/sposta`, {dir: k==="ArrowUp"?"su":"giu"}); e.preventDefault(); return; }
  if(k==="d"||k==="D"){ S.disegnoNascosto = !S.disegnoNascosto; render(); e.preventDefault(); return; }
  if(k==="v"||k==="V"){ S.disCiclo = S.disCiclo==="componente" ? "prodotto" : "componente"; if(S.disegnoNascosto) S.disegnoNascosto = false; render(); e.preventDefault(); return; }
  if(k==="c"||k==="C"){ S.copiaDa = !S.copiaDa; render(); e.preventDefault(); return; }
  if(k==="n"||k==="N"){ const i = document.getElementById("nota-in"); if(i){ i.focus(); e.preventDefault(); } return; }
  if(k==="/"){ const i = document.getElementById("agg-fase"); if(i){ i.focus(); e.preventDefault(); } return; }
  if(k==="a"||k==="A"){ if(S.cicloSel && S.cicloSel.size){ S.blocco = {staz:3, cids:[...S.cicloSel], modello:null}; render(); e.preventDefault(); } return; }
}
if(typeof AIUTO_STAZ!=="undefined") AIUTO_STAZ[3] = {
  tasti:[["V","disegno: prodotto finito ⇄ componente"],["← →","componente prima o dopo"],["Invio","in un campo: accetta ◇ o salva, poi il campo dopo"],["Maiusc + Invio","accetta le proposte della fase aperta"],
    ["⌃↵","conferma e vai al prossimo da fare"],["/","aggiungi una fase"],["C","Copia da…"],["D","nasconde o mostra il disegno"],["N","nota sul disegno legata alla fase aperta"],
["Alt + ↑ ↓","sposta la fase aperta"],["A","con una selezione (Maiusc+clic): applica un modello"]],
  testi:["Entrando nel Ciclo il disegno è il PDF del prodotto finito: il riferimento per capire dove va il pezzo. Un clic su un componente (o V) passa al suo 2D.",
    "Nelle fasi di unione (saldatura, dadi saldati, montaggio) «Entrano» è una scelta: un clic fa entrare un figlio, oppure lo sposta da un'altra unione (l'avviso ha «Annulla»). «+ fase di unione prima» crea una pre-unione vuota: i pezzi che vanno uniti prima di ogni altra lavorazione. «Si salda» si legge da «Entrano». Lo stesso dato si vede nella Sequenza.",
    "Le preparazioni (maschera, robot, piega, curvatura, stampo, dima, calibro, prova di taglio, altra) nascono dalla fase. «Interno» prepara la bozza all'attrezzeria; «Fornitore» apre la carta nel cassetto ✉, dove prepari la bozza per ogni fornitore (le richieste di Pietro). La mail la mandi da Outlook. La risposta registrata scrive costo e settimane, e arriva da sola nell'Offerta.",
    "Il costo delle preparazioni lo usa Franco: ammortizzato, a parte o del cliente si decide nell'Offerta."],
  richieste:"R-10 · R-11 · R-13 · R-14 · R-23…R-30 · R-36 · R-37"};
