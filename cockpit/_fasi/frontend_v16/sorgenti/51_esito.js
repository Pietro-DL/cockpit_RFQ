/* ================= 51 · stazione ⑤ Esito → Franco (zona B · D-01 D-02 R-35 R-40) =================
   SPEC_v15 §2.7. Una riga per componente con UNA colonna «esito» (B3): la proposta del server è il segmento con ◇, la
   decisione di Lino lo rende pieno. «Accetta le N proposte OK…» elenca le righe prima di scrivere (gesto B esplicito,
   non tocca BOM, fonte o documenti). Le firme sono segmenti (M3). «Consegna a Franco» registra la firma dell'area
   tecnica, fotografa la Distinta PER OGNI LOTTO SPUNTATO (partono non spuntati) e avvisa Franco: NON tocca la fase,
   l'albero, la fonte o i documenti (RV-29). Con un KO diventa «Consegna come no quote»; un DUBBIO non blocca.
   La stessa stazione apre, per il ruolo Franco, il Foglio per Franco, i Costi, i Listini e l'Offerta (52). */

/* P-07: «minuteria a norma» non si conferma qui: è della zona C (R-08), spenta con lo stesso motivo (minuteriaSpenta, 47) */
const VISTE_5 = [["esito","Esito (Lino)"],["foglio","Foglio per Franco"],["costi","Costi"],["listini","Listini"],["offerta","Offerta"]];
/* v16 (SE §4.10): il sottomenu dipende dal ruolo. Lino vede «Esito» e il collegamento al Foglio; Costi, Listini e Offerta
   stanno nel «⋯». Con il ruolo Franco si vedono le sue quattro viste, ed «Esito» resta raggiungibile. */
function navEsito(){
  const b = ([k,l]) => `<button type="button" data-esito-vista="${k}" aria-current="${S.esitoVista===k?"page":"false"}">${esc(l)}</button>`;
  const piuV = l => `<details class="piu"><summary aria-label="Altre viste della stazione ⑤" title="Altre viste">⋯</summary><div class="piu-corpo" role="group" aria-label="Altre viste">${l.map(([k,x])=>`<button type="button" class="ma-btn ma-btn--sm" data-esito-vista="${k}">${esc(x)}</button>`).join("")}</div></details>`;
  if(S.ruolo==="Franco") return `<nav class="off-nav" aria-label="Viste della stazione ⑤">${VISTE_5.slice(1).map(b).join("")}${b(VISTE_5[0])}</nav>`;
  const cur = VISTE_5.find(x=>x[0]===S.esitoVista) || VISTE_5[0];
  const vis = [VISTE_5[0], VISTE_5[1]]; if(!vis.includes(cur)) vis.push(cur);
  return `<nav class="off-nav" aria-label="Viste della stazione ⑤">${vis.map(b).join("")}${piuV(VISTE_5.filter(x=>!vis.includes(x)))}</nav>`;
}
/* le preparazioni della RFQ, lette dal finto server A2 (C2: preparazioniRFQ, attrezzaturaPronta). Prima che ci siano, nulla. */
function attrezzaturaDi(T, p){
  if(typeof attrezzaturaPronta!=="function") return null;
  try { return attrezzaturaPronta(T,p); } catch(err){ return null; }
}
/* RL-17: «Matematica (3D)» è SI solo con un documento 3D (STEP) nel thread o con lo STEP autorizzato; altrimenti «NO · solo 2D» */
function tre3D(T, p){
  if(p.fonte.stato==="confermata") return "SI";
  const step = Object.values(T.allegati).some(a=>/^(step|stp|igs)$/i.test(a.tipo||"") || /\.(step|stp|igs)$/i.test(a.nome));
  return step ? "da verificare" : "NO";
}
function stazioneEsito(T, p){
  const banda = bandaB("D-01 · D-02 · R-35 · R-40 · R-11","esiti, firme, attrezzature e consegna non sono nel backend di oggi; la fase cambia solo con un gesto registrato");
  if(!S.esitoVista) S.esitoVista = S.ruolo==="Franco" ? "foglio" : "esito";
  const nav = navEsito();
  const bandaF = S.esitoVista==="foglio" ? bandaB("R-40 · D-01","il Foglio per Franco è la fotografia della consegna")
    : bandaB("R-10…R-36 · costi","costi, listini e offerta: tariffe di esempio nella forma del Mod 99 o marcate «inventato»");
  if(S.esitoVista!=="esito") return `<div class="staz-b">${bandaF}${paAvvisoHtml()}<div class="b-testa">${nav}</div><div class="b-corpo">${vistaFrancesco(T,p)}</div></div>`;
  const righe = p ? righeB(T,p).filter(r=>r.cid) : [];
  if(!righe.length) return `<div class="staz-vuota">${banda}<div class="b-testa">${nav}</div><div class="empty"><span class="ib">${I("cube",22)}</span><h3>Nessun componente confermato</h3><p>L'esito si dà sui componenti: prima la stazione ①.</p></div></div>`;
  const S2 = a2Di(T), root = righe.find(r=>r.radice) || righe[0];
  const prop = Object.fromEntries(righe.map(r=>[r.cid, esitoProposto(T,p,r.cid)]));
  const deciso = r => (S2.esiti[r.cid]||{}).v || null;
  const val = r => deciso(r) || prop[r.cid].v;
  const peggio = ["OK","DUBBIO","KO"][Math.max(...righe.map(r=>["OK","DUBBIO","KO"].indexOf(val(r))))];
  const nDB = controlliB(T,p).filter(x=>x.liv==="DB").length;
  const {fare} = elencoCiclo(T,p), nCicli = fare.filter(r=>{ const c=cicloB(T,r.cid); return c&&c.stato==="confermato"; }).length;
  const rq = REQUISITI[T.key]||[], rqOk = rq.filter(q=>requisitoCoperto(T,p,q).ok).length;
  /* RL-23: le risposte arrivate e non ancora registrate sono richieste ancora aperte */
  const daReg = S2.richieste.filter(q=>["offerta_ricevuta","risposta"].includes(q.stato) && !registrazioneDi(T,q)).length;
  const aperte = S2.richieste.filter(q=>["inviata","bozza"].includes(q.stato)).length + daReg, sq = seqDi(T,p);
  if(!S.esitoRiga || !righe.some(r=>r.cid===S.esitoRiga)) S.esitoRiga = righe[0].cid;
  const okProposti = righe.filter(r=>!deciso(r) && prop[r.cid].v==="OK");
  const pre = prefissoComune(righe.map(r=>r.cod));
  const LIV_ORD = {KO:0, DB:1, AV:2};
  /* la riga corrente ha OK · DUBBIO · KO interi; le altre il solo valore (S-06): un clic lo apre, i tasti O B X restano */
  const riga = r => { const pv = prop[r.cid], dv = deciso(r), cur = S.esitoRiga===r.cid, nota = (S2.esiti[r.cid]||{}).nota||"";
    const regole = pv.perche.filter(x=>x.cod!=="DOC-02").slice().sort((a,b)=>(LIV_ORD[a.liv]??3)-(LIV_ORD[b.liv]??3));
    const regola = x => `<span><span class="liv liv--${x.liv}">${x.cod}</span> ${esc(x.testo.replace(r.cod+": ","").replace(r.cod+" ",""))}</span>`;
    /* M-8: le regole in più si leggono sulla riga scelta (un clic sulla riga); sulle altre resta «+n» come testo */
    const perche = regole.length ? (cur ? regole.map(regola).join("") : `${regola(regole[0])}${regole.length>1?` <span class="k3 perche-n" title="${esc(regole.slice(1).map(x=>x.cod+" "+x.testo).join(" · "))}">+${regole.length-1}</span>`:""}`) : `<span class="k3">—</span>`;
    const seg = cur ? `<span class="seg seg--esito" role="group" aria-label="Esito di ${esc(r.cod)}">${["OK","DUBBIO","KO"].map(v=>`<button type="button" data-v="${v}" data-pa-gesto="/componente/${r.cid}/fattibilita" data-pa-campi='${JSON.stringify({esito:v})}' aria-pressed="${dv===v}" class="${!dv&&pv.v===v?"seg--prop":""}" title="${!dv&&pv.v===v?"proposta del sistema ◇":""}"><kbd>${{OK:"O",DUBBIO:"B",KO:"X"}[v]}</kbd>${!dv&&pv.v===v?"◇ ":""}${v}</button>`).join("")}</span>`
      : `<span class="seg seg--esito seg--compatta" role="group" aria-label="Esito di ${esc(r.cod)}"><button type="button" data-v="${dv||pv.v}" data-esito-apri="${r.cid}" aria-pressed="${!!dv}" class="${dv?"":"seg--prop"}" title="${dv?"deciso da te":"proposta del sistema ◇"}: clic per cambiarlo (O B X)">${dv?"":"◇ "}${dv||pv.v}</button></span>`;
    const campoNota = cur || dv==="DUBBIO" || dv==="KO" ? `<input class="nota" data-esito-nota="${r.cid}" value="${esc(nota)}" placeholder="nota per il cliente o l'ufficio" aria-label="Nota per ${esc(r.cod)}">`
      : nota ? `<span class="es-nota" title="clic sulla riga per cambiarla">${esc(nota)}</span>` : `<span class="es-nota es-nota--agg" data-esito-apri="${r.cid}" data-esito-fuoco="1" title="clic: scrivi una nota su questa riga">+ nota</span>`;
    return `<tr class="${cur?"es-cur":""}" data-esito-riga="${r.cid}"><td><b class="mono" title="${esc(r.cod)}">${esc(suffisso(r.cod,pre))}</b> <span class="k3 piccolo">${esc(r.descr)}</span></td>
      <td>${seg}</td><td><span class="esito-perche">${perche}${r.comm&&cur?minuteriaSpenta():""}</span></td><td>${campoNota}</td></tr>`; };
  const anteprima = S.esitoAnteprima ? `<div class="blocco-b"><b>Accetterai ${okProposti.length} proposte OK, una per riga:</b><ul>${okProposti.map(r=>`<li><span class="mono">${esc(r.cod)}</span> ${esc(r.descr)}</li>`).join("")}</ul>
      <div class="acts">${btnB(`/prodotto/${root.cid}/esiti`, {cids: okProposti.map(r=>r.cid).join(",")}, "Accetta le OK (Invio)")}<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-esito-anteprima="">Annulla (Esc)</button>
      <span class="k3 piccolo">Non tocca BOM, fonte o documenti. I DUBBI e i KO si decidono uno per uno.</span></div></div>` : "";
  const reqs = rq.map(q=>{ const c = requisitoCoperto(T,p,q), visto = S2.visti[q.id];
    if(q.tipo==="condizione") return `<div><span title="fonte: ${esc(NOME_FONTE[q.fonte]||q.fonte||"")}">${esc(q.testo)}</span>${visto?`<span class="ma-st ma-st--ok">vista da ${esc(visto.da)} ${esc(visto.data)}</span>`:`${q.avvisa?(richiesteDi(T,root.cid).some(x=>x.tipo==="commerciale"&&x.stato!=="annullata")?`<span class="ma-st ma-st--prop">avvisato: attende il commerciale</span>`:btnB(`/rfq/${T.id}/domanda`, {tipo:"commerciale", cid:root.cid, testo:`Condizione dell'ordine: ${q.testo}. Rispondere entro la scadenza.`}, "✉ Avvisa il commerciale", "pa-btn")):""} <span class="liv liv--DB" title="ORD-01">da vedere</span>`}</div>`;
    return `<div><span title="fonte: ${esc(NOME_FONTE[q.fonte]||q.fonte||"")}">${esc(q.testo)}</span>${c.ok?`<span class="ma-st ma-st--ok">→ ${esc(c.dove)} ✓</span>`:`<span class="ma-st ma-st--ver">nessuna fase</span> <span class="liv liv--DB">REQ-01</span> <button type="button" class="ma-btn ma-btn--sm" data-vai="3|${root.cid}">→ aggiungi o correggi la fase</button>`}</div>`; }).join("");
  /* Dati e documenti (Mod 99): sei interruttori; la proposta ◇ si accetta con un clic o tutte insieme */
  const AP = attrezzaturaDi(T,p);
  const DD = [["disegno","Disegno"],["mp","Materia prima"],["attr","Attrezzatura"],["npart","N° particolari"],["mat3d","Matematica (3D)"],["spec","Caratteristiche speciali"]];
  const attrVecchia = righe.some(r=>{ const c=cicloB(T,r.cid); return c&&Object.values(c.prep||{}).some(v=>v.on); })?"SI":"N.A.";
  const ddProp = {disegno: controlliB(T,p).some(x=>x.cod==="DOC-01"&&x.liv!=="AV")?"da verificare":"SI", mp:"SI",
    attr: AP ? (!(AP.elenco||[]).length ? "N.A." : AP.ok ? "SI" : "da verificare") : attrVecchia, npart:"SI", mat3d: tre3D(T,p), spec: rq.some(q=>q.tipo==="saldatura")?"SI":"N.A."};
  const accettabili = DD.filter(([k])=>!S2.datiDoc[k] && ["SI","N.A.","NO"].includes(ddProp[k]));
  const dd = DD.map(([k,l])=>{ const v = S2.datiDoc[k], pv = ddProp[k], prossimo = v ? (v==="SI"?"N.A.":"SI") : (["SI","N.A.","NO"].includes(pv) ? pv : "SI");
    return `<div><span>${esc(l)}</span><button type="button" class="dd-int${v?" dd-int--deciso":" dd-int--prop"}" role="switch" aria-checked="${v==="SI"}" data-pa-gesto="/prodotto/${root.cid}/dati-documenti" data-pa-campi='${JSON.stringify({voce:k, v:prossimo})}' title="${v?`deciso: ${v} · clic: ${prossimo}`:`proposta del sistema ◇ ${pv} · clic: ${prossimo}`}">${v?esc(v==="NO"&&k==="mat3d"?"NO · solo 2D":v):"◇ "+esc(pv==="NO"&&k==="mat3d"?"NO · solo 2D":pv)}</button></div>`; }).join("");
  const attrRiga = AP && (AP.elenco||[]).length ? `<div class="dd-attr"><span class="lab">Attrezzatura · preparazioni</span>
      <span class="dd-attr-l">${AP.elenco.map(x=>`<button type="button" class="ma-st ma-st--${x.orfana?"ver":x.stato==="esiste già"||x.stato==="esiste_gia"?"neu":x.costo!=null&&x.costo!==""?"ok":"ver"} dd-prep" ${x.orfana&&x.richiesta?`data-rq-apri="attrezzatura" title="PREP-04: la sua fase non c'è più · apri la richiesta ${esc(x.richiesta.id)}"`:`data-vai="3|${esc(x.cid)}|${esc(x.fid||"")}|prep:${esc(x.key||x.k)}" title="apri la preparazione nel Ciclo"`}>${esc(x.nome||x.k)}${x.fase?" "+esc(x.fase):""} · ${esc(suffisso(x.cod||"",pre))} · ${x.orfana?"⚠ fase tolta · richiesta aperta":x.stato==="esiste già"||x.stato==="esiste_gia"?"esiste già":x.costo!=null&&x.costo!==""?fmt(+x.costo,0)+" €"+(x.settimane?" · "+x.settimane+" sett.":""):"senza costo"}</button>`).join("")}</span>
      ${AP.senzaCosto?`<button type="button" class="ma-btn ma-btn--sm pa-btn" data-rq-apri="${AP.elenco.some(x=>!x.orfana&&!x.costo&&x.stato!=="esiste già"&&(PREP_INFO[x.k]||PREP_INFO.altra).rich.includes("attrezzatura"))?"attrezzatura":""}" data-richiedi-prep="1" title="Apre il cassetto ✉: prepari la bozza per ogni fornitore o all'interno. Non parte niente da solo.">✉ ${AP.senzaCosto===1?"Richiedi la preparazione senza costo":`Richiedi le ${AP.senzaCosto} preparazioni senza costo`}</button>`:`<span class="ma-st ma-st--ok">tutte con un costo o «esiste già»</span>`}</div>` : "";
  const AREE = [["tec","Area tecnica","Lino"],["prod","Produzione","Marta"],["qual","Qualità","Qualità"],["acq","Acquisti","Fabio"]];
  /* firme: attiva solo l'area del ruolo simulato; le altre si leggono */
  const firme = AREE.map(([k,l,chi])=>{ const f = S2.firme[k], mia = S.ruolo===chi;
    return `<div class="firma-b${mia&&!f?" firma-b--mia":""}"><span class="lab">${esc(l)}</span>${f?`<span class="ma-st ma-st--${f.v==="positiva"?"ok":"ko"}">${esc(f.v)}</span><span class="piccolo">Firmato da ${esc(f.chi)} · ${esc(f.quando)}</span>`
      : mia ? `<span class="seg" role="group" aria-label="Firma ${esc(l)}">${["positiva","negativa"].map(v=>`<button type="button" data-pa-gesto="/prodotto/${root.cid}/firma" data-pa-campi='${JSON.stringify({area:k, v})}'>${v==="positiva"?"Positiva":"Negativa"}</button>`).join("")}</span>`
      : `<span class="k3 piccolo">attende ${esc(chi)}</span>`}</div>`; }).join("");
  const L = lottiDi(T), sel = S.lottiSel || (S.lottiSel = new Set());
  const ko = righe.some(r=>val(r)==="KO");
  const sa = statoZonaA(p);
  const pronta = sel.size && (sa.pronto || S.presaAtto);
  /* B-3: spento con aria-disabled: il clic (o ⌃↵) porta al riquadro Consegna, con il fuoco su quello che manca */
  const consegna = btnB(`/prodotto/${root.cid}/consegna`, {lotti:[...sel].join(","), noquote: ko?"1":"", presaAtto: S.presaAtto?"1":""}, ko ? "Consegna come no quote →" : "Consegna a Franco →", "ma-btn--pri",
    pronta ? 'data-consegna="1"' : `aria-disabled="true" data-consegna="1" data-consegna-guida="${!sel.size?"lotto":"presa"}" aria-describedby="consegna-manca" title="${!sel.size?"Spunta almeno un lotto nel riquadro Consegna":"Prendi atto che la zona A non è pronta, nel riquadro Consegna"}"`)
    + (pronta ? "" : `<span class="vh" id="consegna-manca">${!sel.size?"Spunta almeno un lotto nel riquadro Consegna":"Prendi atto che la zona A non è pronta"}</span>`);
  const testaA = `<div class="zona-a-ro${sa.pronto?" zona-a-ro--ok":""}" role="note"><span class="lab">Zona A · sola lettura</span><span>prodotto <b>${esc(sa.stato)}</b>${sa.pronto?"":` · ${esc(sa.motivi)}`}</span><span>${esc(sa.bom)}</span><span>${esc(sa.documenti)}</span>${sa.pronto?"":`<span class="k3">si sistema in ① Struttura</span>`}</div>`;
  const presa = sa.pronto ? "" : `<label class="spunta presa-atto"><input type="checkbox" data-presa-atto="1" ${S.presaAtto?"checked":""}> Consegno sapendo che il prodotto è «${esc(sa.stato)}» nella zona A: lo scrivo nella fotografia</label>`;
  const statoBarra = `<span class="b-piede-t" title="Cicli ${nCicli}/${fare.length} · Sequenza di ${esc(p.codice_richiesto)} ${sq.stato==="confermata"?"confermata":"non confermata"} · Richieste ${aperte} aperte · Requisiti ${rqOk}/${rq.length}">${esc(root.cod)} · proposta <b class="pr">${peggio}</b>${nDB?` (${nDB} ${nDB===1?"dubbio":"dubbi"})`:""} · cicli ${nCicli}/${fare.length} · seq. ${sq.stato==="confermata"?"✓":"da confermare"} · richieste ${aperte} · requisiti ${rqOk}/${rq.length}${sel.size?` · lotti ${[...sel].join(", ")}`:" · nessun lotto spuntato"}</span>`;
  return `<div class="staz-b">${banda}${paAvvisoHtml()}<div class="staz-b-main">
    <div class="b-testa">${nav}</div>
    <div class="b-testa"><h2>⑤ Esito</h2>${okProposti.length?`<button type="button" class="ma-btn ma-btn--sm" data-esito-anteprima="1">Accetta le ${okProposti.length} proposte OK…</button>`:""}</div>
    <div class="b-corpo">
      ${testaA}
      ${anteprima}
      ${(()=>{ const d2 = controlliB(T,p).filter(x=>x.cod==="DOC-02"); return d2.length ? `<p class="piccolo esito-comune"><span class="liv liv--AV">avviso</span> <code class="k3">DOC-02</code> ${d2.length} PDF senza testo: il cartiglio va letto a mano (${d2.map(x=>esc(suffisso(x.rcod,pre))).join(", ")})</p>` : ""; })()}
      <div class="wrap-tb"><table class="esito-tab" aria-label="Esito per componente"><thead><tr><th>Componente</th><th>Esito (proposto ◇ finché non decidi)</th><th>Perché (regola peggiore)</th><th>Nota</th></tr></thead><tbody>${righe.map(riga).join("")}</tbody></table></div>
      <div class="esito-blocchi">
        <section class="esito-blocco"><header><h3>Requisiti e condizioni dell'ordine</h3>${(()=>{ const da = rq.filter(q=>q.tipo==="condizione" && !q.avvisa && !S2.visti[q.id]); return da.length ? btnB(`/prodotto/${root.cid}/requisito`, {ids:da.map(q=>q.id).join(",")}, `Presa visione${da.length>1?` delle ${da.length}`:""}`, "", `data-presa-visione="${da.length}" title="Presa visione di: ${esc(da.map(q=>q.testo).join(" · "))}"`) : ""; })()}</header><div class="req-l">${reqs||`<span class="k3">nessun requisito letto</span>`}</div></section>
        <section class="esito-blocco"><header><h3>Dati e documenti (Mod 99)</h3>${accettabili.length?`<button type="button" class="ma-btn ma-btn--sm" data-dd-tutte="${accettabili.map(([k])=>k+"="+ddProp[k]).join(",")}">Accetta le ${accettabili.length} proposte</button>`:""}</header><div class="dd">${dd}</div>${attrRiga}</section>
        <section class="esito-blocco"><header><h3>Firme (Mod 99)</h3><span class="k3 piccolo">firmi la tua area: ${esc(S.ruolo)}</span></header><div class="firme-b">${firme}</div></section>
        <section class="esito-blocco" id="esito-consegna"><header><h3>Consegna a Franco</h3><span class="k3 piccolo">${esc(L.testo)}</span></header>
          <div class="lotti-b"><span class="lab">Lotti da fotografare</span>${L.lotti.map(l=>`<label class="spunta"><input type="checkbox" data-lotto="${l}" ${sel.has(l)?"checked":""}> ${fmt(l,0)} pz</label>`).join("")}
            <span class="cerca-b" style="min-height:28px"><input id="lotto-nuovo" inputmode="numeric" placeholder="altro lotto" aria-label="Aggiungi un lotto" style="width:9ch"></span><button type="button" class="ma-btn ma-btn--sm" data-pa-gesto="/prodotto/${root.cid}/lotto" data-pa-form-id="lotto-nuovo">+ lotto</button>
            ${na("Passa alla scheda costo","la fase cambia solo con un gesto registrato")}</div>${presa}
          ${S2.consegne.length?`<div class="consegne">${S2.consegne.map((c,i)=>`<span>✓ Consegnato il ${esc(c.quando)} · lotti ${c.lotti.join(", ")}${c.noquote?" · no quote":""} · firmato da ${esc(c.firma?c.firma.chi:"—")} <button type="button" class="ma-btn ma-btn--sm" data-esito-vista="foglio" data-foto="${i}">Apri il Foglio per Franco →</button></span>`).join("")}</div>`:""}
          <p class="k3 piccolo" title="Un DUBBIO va nella fotografia con la nota.">La consegna non tocca la fase (resta «${esc(T.testa.fase?T.testa.fase.etichetta||T.testa.fase.nome:"nessuna")}»), l'albero, la fonte o i documenti.</p></section>
      </div></div>
    <div class="b-piede barra-staz">${statoBarra}${consegna}</div></div></div>`;
}
document.addEventListener("click", e=>{
  const t = e.target.closest && e.target.closest("[data-esito-vista],[data-esito-anteprima],[data-esito-apri],[data-dd-tutte],[data-consegna-guida],[data-esito-riga]");
  if(!t) return;
  const D = t.dataset;
  if(D.esitoApri){ S.esitoRiga = D.esitoApri; if(D.esitoFuoco) S.fuoco = `[data-esito-nota="${D.esitoApri}"]`; render(); if(typeof dopoRender==="function") dopoRender(); return; }
  /* «Accetta le N proposte» dei Dati e documenti: una scrittura per voce, sotto un gesto solo (R-36) */
  if(D.ddTutte){ const T = threadAperto(), p = prodottoScelto(T), root = righeB(T,p).find(r=>r.radice) || righeB(T,p)[0];
    eseguiB(`/prodotto/${root.cid}/dati-documenti`, {voci:D.ddTutte}); return; }
  if(D.esitoVista){ S.esitoVista = D.esitoVista; if(D.foto!==undefined) S.foto = +D.foto; render(); const m = document.querySelector(".b-corpo"); if(m) m.scrollTop = 0; return; }
  if(D.esitoAnteprima!==undefined){ S.esitoAnteprima = D.esitoAnteprima==="1"; render(); return; }
  if(D.esitoRiga && !e.target.closest("button,input")){ S.esitoRiga = D.esitoRiga; render(); return; }
  if(D.consegnaGuida){ const sez = document.getElementById("esito-consegna"); if(sez && sez.scrollIntoView) try{ sez.scrollIntoView({block:"nearest"}); }catch(err){}
    const el = document.querySelector(D.consegnaGuida==="lotto" ? "[data-lotto]" : "[data-presa-atto]"); if(el) el.focus(); return; }
});
document.addEventListener("change", e=>{
  const el = e.target; if(!el || !el.dataset) return;
  if(el.dataset.presaAtto){ S.presaAtto = el.checked; render(); return; }
  if(el.dataset.lotto){ const s = S.lottiSel || (S.lottiSel = new Set()); const v = +el.dataset.lotto; if(el.checked) s.add(v); else s.delete(v); render(); return; }
  if(el.dataset.esitoNota){ eseguiB(`/componente/${el.dataset.esitoNota}/fattibilita`, {nota:el.value}); return; }
});
function dopoEsito(url, r, campi){
  if(r.esito!=="ok") return;
  if(/\/esiti$/.test(url)) S.esitoAnteprima = false;
  if(/\/consegna$/.test(url)){ S.lottiSel = new Set(); S.presaAtto = false; S.foto = a2Di(threadAperto()).consegne.length-1; }
}
