/* ================= 53 · cassetto ✉ Richieste (Blocco 7B di Pietro · zona B R-11 R-13 R-17 R-27 R-28 R-41) =================
   v16 rev. (P-02 di Pietro): ai fornitori il cassetto usa le richieste di Pietro così come sono nel suo backend
   (richiesta_fornitore, migrazioni 0015-0016; richieste.go). Una riga per fornitore; «Prepara la bozza» =
   POST /thread/{id}/richiesta con bozza=1 (la bozza in Outlook con il marcatore); «Annulla» = POST …/{rid}/annulla.
   Il Cockpit NON manda mail: «inviata» la mette il sync quando la mail compare nella Posta inviata; «offerta ricevuta» e
   «declinata» le mette l'aggancio della risposta nell'Inbox; «scaduta» la calcola il server (R-27). Nel mockup questi
   tre eventi si simulano dal «⋯», segnati «(mockup)».
   La carta è il BISOGNO (una preparazione, una fase esterna, un grezzo): si legge dai dati, non è una tabella. Sotto, una
   riga per fornitore, con la sua descrizione accanto al nome (M7). «Registra» scrive nel punto d'origine (R-13): la
   richiesta non cambia stato. Le domande interne (officina, qualità, commerciale, cliente) sono la proposta R-28 · R-41. */

/* quale carta è aperta: quella appena chiesta (S.rqSu), quella toccata per ultima, altrimenti la prima da registrare */
function carteRichieste(T, p){
  const S2 = a2Di(T), bis = bisogniRichieste(T,p).filter(b=>b.voci.some(v=>!v.fatto) || b.righe.length);
  /* le preparazioni da chiedere all'interno (dima, robot, calibro, prova): una riga per preparazione */
  const interne = [];
  for(const r of righeB(T,p)){ if(!r.cid) continue; const cy = cicloB(T,r.cid); if(!cy) continue;
    for(const [key,v] of prepVoci(cy)){ const info = PREP_INFO[v.k]||PREP_INFO.altra;
      if(!v.on || v.costo || v.stato==="esiste già" || info.rich.includes("attrezzatura") || richiesteDellaPrep(T,r.cid,key).some(q=>q.stato!=="annullata")) continue;
      const i = cy.fasi.findIndex(f=>f.id===v.nata);
      interne.push({cid:r.cid, cod:r.cod, key, k:v.k, tipo:info.rich[0], chi:info.chi, dove:`${nomePrep(v.k).toLowerCase()} ${i>=0?`f.${(i+1)*10}`:""}`}); } }
  const dom = S2.richieste.filter(q=>!q.forn);
  return {bis, interne, dom};
}
function statoCarta(T, b){
  const vive = b.righe.filter(q=>q.stato!=="annullata");
  if(vive.some(q=>q.stato==="offerta_ricevuta" && !registrazioneDi(T,q))) return ["ok","offerta da registrare"];
  if(b.voci.length && b.voci.every(v=>v.fatto)) return ["ok","registrata ✓"];
  if(!vive.length) return ["neu","da preparare"];
  if(vive.some(q=>q.stato==="bozza"||q.stato==="inviata")) return ["prop","in attesa"];
  return ["ver", vive.every(q=>q.stato==="declinata") ? "declinata" : "scaduta"];
}
function cassettoRichieste(T, p){
  const S2 = a2Di(T), pre = prefissoComune(righeB(T,p).map(r=>r.cod)), {bis, interne, dom} = carteRichieste(T,p);
  const mc = mancantiAlCliente(T,p);
  const ordine = c => ({"offerta da registrare":0, "da preparare":1, "in attesa":2})[c] ?? 3;
  const carte = bis.map(b=>({k:b.key, o:ordine(statoCarta(T,b)[1]), html:()=>cartaBisogno(T,p,b,pre)}))
    .concat(dom.map(q=>({k:q.id, o: q.stato==="risposta" && !registrazioneDi(T,q) ? 0 : q.stato==="bozza"||q.stato==="inviata" ? 2 : 3, html:()=>cartaDomanda(T,p,q,pre)})))
    .sort((a,b)=>a.o-b.o);
  if(!S.rqAperta || (S.rqAperta!=="-" && !carte.some(c=>c.k===S.rqAperta))) S.rqAperta = S.rqSu && carte.some(c=>c.k===S.rqSu) ? S.rqSu : (carte.find(c=>c.o===0)||carte[0]||{}).k || null;
  return `${bandaB("7B · R-11 · R-13 · R-17 · R-27 · R-28 · R-41","ai fornitori le richieste di Pietro: una per fornitore, la bozza in Outlook, gli stati dal sync e dall'Inbox; le domande interne sono la proposta R-28 · R-41")}
    ${PA.avviso?`<div class="pa-avviso" id="pa-avviso-cas" role="status" data-esito="${esc(PA.avviso.esito)}"><b class="pa-e">${PA.avviso.esito==="ok"?"fatto":"non fatto"}</b><span>${esc(PA.avviso.testo)}</span></div>`:""}
    <p class="rq-testa piccolo k3">Il Cockpit prepara la bozza in Outlook: la mail la mandi tu, da lì. Gli stati li dà il server.</p>
    ${(()=>{ /* le risposte arrivate e non ancora registrate: tutte qui in alto, un clic ciascuna (R-13) */
      const da = S2.richieste.filter(q=>["offerta_ricevuta","risposta"].includes(q.stato) && q.risp && !q.risp.declina && !registrazioneDi(T,q));
      return da.length ? `<section class="rq-gruppo rq-gruppo--reg"><h3>Risposte da registrare <span class="mono">${da.length}</span></h3>
        ${da.map(q=>`<div class="rq-da" data-rq-reg="${esc(q.id)}"><b>${esc(q.forn?q.forn.nome:q.dest.nome)}</b> <span class="k3">${esc(TIPI_RICHIESTA[q.tipo].nome.toLowerCase())}${q.risp.prezzo!=null?` · ${fmt(q.risp.prezzo,2)} ${esc(q.risp.unita||"")}`:""}${(q.controlli||[]).some(k=>k.cod==="MAT-05")?" · MAT-05":""}</span><span class="sp"></span>${(q.controlli||[]).some(k=>k.cod==="MAT-05")?`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-rq-apri="${esc(q.bisogno||q.id)}">Apri</button>`:btnB(`/rfq/${T.id}/risposta`, {richiesta:q.id}, "Registra", "pa-btn", `data-registra-veloce="1"`)}</div>`).join("")}</section>` : ""; })()}
    ${mc.length ? `<section class="rq-gruppo rq-gruppo--da"><h3 title="Dalla voce 2D dei Documenti (zona A, sola lettura). La bozza va al commerciale, che la manda al cliente.">2D da chiedere al cliente <span class="mono">${mc.length}</span>
        <span class="sp"></span>${btnB(`/rfq/${T.id}/domanda`, {tipo:"cliente", cids:mc.map(r=>r.cid).join(","), staz:"1", testo:testo2D(mc)}, `✉ Prepara la bozza (${mc.length} ${mc.length===1?"codice":"codici"})`, "pa-btn", `data-rq-clienti="1"`)}</h3>
      ${mc.map(r=>`<div class="rq-da"><span class="mono">${esc(suffisso(r.cod,pre))}</span> <span class="k3">${esc(r.descr||"")}</span><span class="sp"></span>
        <details class="piu"><summary aria-label="Altro per ${esc(r.cod)}">⋯</summary><div class="piu-corpo">${btnB(`/rfq/${T.id}/domanda`, {tipo:"cliente", cid:r.cid, staz:"1", testo:testo2D([r])}, "✉ Prepara solo questa bozza", "pa-btn", `data-rq-cliente="${r.cid}"`)}</div></details></div>`).join("")}</section>` : ""}
    ${interne.length ? `<section class="rq-gruppo rq-gruppo--da"><h3 title="Preparazioni che si chiedono all'interno (R-28): attrezzeria, programmatore, qualità, officina laser">Preparazioni da chiedere all'interno <span class="mono">${interne.length}</span></h3>
      ${interne.map(x=>`<div class="rq-da"><span class="mono">${esc(suffisso(x.cod,pre))}</span> <span class="k3">${esc(x.dove)} · a ${esc(x.chi)}</span><span class="sp"></span>${btnB(`/rfq/${T.id}/domanda`, {tipo:x.tipo, cid:x.cid, prep:x.key}, "✉ Prepara la bozza", "pa-btn")}</div>`).join("")}</section>` : ""}
    ${carte.length ? carte.map(c=>c.html()).join("") : (mc.length||interne.length ? "" : `<div class="empty"><span class="ib">✉</span><h3>Nessuna richiesta</h3><p>Le richieste nascono dove serve: preparazioni, fasi esterne, grezzo, Documenti ed Esito. Qui prepari la bozza per ogni fornitore.</p></div>`)}
    ${S2.posta.length?`<details class="cas-blocco"><summary>Bozze e mail (${S2.posta.length})</summary><ul class="cas-elenco">${S2.posta.map(x=>`<li>${esc(x.data)} · ${x.bozza?"bozza in Outlook":"partita"} · a ${esc(x.a)} · ${esc(x.oggetto)}</li>`).join("")}</ul></details>`:""}`;
}
const testo2D = rr => `Manca il disegno 2D di ${rr.map(r=>r.cod).join(", ")}: ce lo mandate? Se è minuteria a norma, confermatecelo per iscritto.`;
/* la riga di un fornitore dentro la carta: «Prepara la bozza» se non c'è ancora, altrimenti lo stato di Pietro e i suoi gesti */
function rigaFornitore(T, p, b, c, pre){
  const q = b.righe.find(x=>x.forn.id===c.id) || a2Di(T).richieste.find(x=>x.forn && x.forn.id===c.id && (x.lavorazione||"")===(b.lav||""));
  const nome = `<span class="rq-f-nome"><b>${esc(c.nome)}</b><small class="k3">${esc(c.descr||"")}</small></span>`;
  if(!q){
    const ok = c.voci.length, nota = (S.rqNote||{})[b.key] || notaBisogno(T,p,b,c.voci), cods = [...new Set(c.voci.map(v=>v.cod))];
    /* RL-11: con più codici nella carta si toglie quello che a questo fornitore non va chiesto (nessuna spunta già messa) */
    const idSel = `rcs-${String(b.key).replace(/[^\w-]/g,"_")}-${c.id}`;
    const scelta = cods.length>1 ? `<span class="rq-cod-sel" id="${idSel}">${cods.map((x,i)=>`<label><input type="checkbox" name="via_${i}" value="${esc(x)}"> non a ${esc(nomeBreve(c.nome))}: <span class="mono">${esc(x)}</span></label>`).join("")}</span>` : "";
    return `<div class="rq-forn">${nome}<span class="sp"></span>${ok ? btnB(`/thread/${T.id}/richiesta`, {fornitore_id:c.id, lavorazione:b.lav||"", codice:cods.join(" "), note:`${nota}\n— rif. Cockpit: ${b.key}`, bozza:"1"}, "Prepara la bozza", "pa-btn", `title="Richiesta a ${esc(c.nome)}: la bozza in Outlook con il marcatore. Non parte da sola."${scelta?` data-pa-form="${idSel}"`:""}`)
      : b.voci.length && b.voci.every(v=>v.fatto) ? `<span class="k3 piccolo" title="Tutti i codici di questa carta hanno già una risposta registrata">niente da chiedere</span>` : `<span class="k3 piccolo" title="Il fornitore non fa questo tipo di preparazione">non la fa</span>`}${scelta}</div>`;
  }
  return rigaRichiesta(T, p, q, nome, pre);
}
function rigaRichiesta(T, p, q, nome, pre){
  const st = q.stato, reg = registrazioneDi(T,q), annulla = q.forn ? `/thread/${T.id}/richiesta/${q.id}/annulla` : `/domanda/${q.id}/annulla`;
  const mock = l => l.length ? `<details class="piu"><summary aria-label="Altro per ${esc(q.id)}" title="Altro">⋯</summary><div class="piu-corpo" role="group" aria-label="Altro">${l.join("")}</div></details>` : "";
  const chip = `<span class="ma-st ma-st--${st==="bozza"?"neu":st==="offerta_ricevuta"||st==="risposta"?"ok":st==="inviata"?"prop":"ver"}" title="${esc(q.id)} · stato del server">${esc(NOMI_STATO_RQ[st]||st)}</span>`;
  const quando = st==="bozza" ? `<span class="k3 piccolo">⧗ parte quando la mandi da Outlook</span>` : st==="inviata" ? `<span class="k3 piccolo">inviata il ${esc(q.inviata||"")}${q.scad?` · scade il ${esc(q.scad)}`:""}</span>` : ""
  /* RIPROVA (RL-11): gli allegati della richiesta si vedono, non solo nei dati */
  const allegati = q.allegati && q.allegati.length ? `<span class="k3 piccolo rq-allegati" data-rq-allegati="${esc(q.id)}">allegati: ${esc(q.allegati.join(", "))}</span>` : "";
  const giro = btnB(`/richiesta/${q.id}/giro`, {}, "Nuovo giro (proposta R-17)");
  const az = st==="bozza" ? [btnB(annulla, {}, "Annulla", "ma-btn--ghost"), mock([btnB(`/mockup/richiesta/${q.id}/sync`, {}, "Simula: la mail parte da Outlook (mockup)")])]
    : st==="inviata" ? [btnB(annulla, {}, "Annulla", "ma-btn--ghost"), mock([btnB(`/mockup/richiesta/${q.id}/risposta`, {}, "Simula la risposta (mockup)"), ...(q.forn?[btnB(`/mockup/richiesta/${q.id}/scadenza`, {}, "Simula: passa la scadenza (mockup · R-27)")]:[])])]
    : ["offerta_ricevuta","risposta","declinata","scaduta"].includes(st) ? [mock([giro, ...(st==="scaduta"?[btnB(annulla, {}, "Annulla")]:[])])] : [];
  /* R-13 precisata (RL-04): la risposta VERA si registra con righe codice × lotto × €/pz, materiale, tempi, data del giro, allegato */
  const regForm = q.forn && ["inviata","offerta_ricevuta"].includes(st) && !reg ? `<details class="rq-reg" data-rq-reg-det="${esc(q.id)}" ${(S.rqRegAperta||{})[q.id]?"open":""}><summary title="Proposta · dopo A2 · R-13 precisata">Registra la risposta del fornitore… <span class="k3">giro ${q.giri.length+1}</span></summary>
      <form data-pa-gesto="/rfq/${T.id}/risposta/righe" aria-label="Registra la risposta di ${esc(q.forn.nome)}"><input type="hidden" name="richiesta" value="${esc(q.id)}">
        <div class="rq-reg-g">${q.codici.map(c=>`<label><span><b class="mono">${esc(c.cod)}</b> · lotto: €/pz${(q.lottiCod||{})[c.cid]?` (chiesti ${esc(q.lottiCod[c.cid])})`:""}</span><input name="l_${esc(c.cid||c.cod)}" placeholder="10: 22 / 5: 30" autocomplete="off" style="width:22ch"></label>`).join("")}</div>
        <div class="rq-reg-g"><label><span>Materiale dichiarato</span><input name="materiale" placeholder="solo se lo scrive" style="width:14ch"></label><label><span>Tempi</span><input name="tempi" placeholder="15 gg lav." style="width:10ch"></label>
          <label><span>Data del giro</span><input name="data" value="${esc(B_OGGI)}" style="width:11ch"></label><label><span>Allegato</span><input name="allegato" placeholder="file della risposta" style="width:16ch"></label>
          <button class="ma-btn ma-btn--sm pa-btn">Registra</button></div></form></details>` : "";
  return `<div class="rq-forn rq-forn--${esc(st)}" data-rq-riga="${esc(q.id)}">${nome}${chip}${quando}${allegati}<span class="sp"></span>${az.join("")}
    ${q.risp?rispostaHtml(T,p,q,reg):""}${regForm}
    ${q.giri.length?`<div class="rq-giri">${q.giri.map(g=>`giro ${g.n} · ${esc(g.rispData||g.data)} · ${esc(NOMI_STATO_RQ[g.stato]||g.stato)}${g.reg?" · "+esc(g.reg):g.risp&&g.risp.prezzo!=null?` · ${fmt(g.risp.prezzo,2)} ${esc(g.risp.unita||"")}`:""}${g.risp&&g.risp.materiale?` · materiale «${esc(g.risp.materiale)}»`:""}`).join("<br>")}</div>`:""}</div>`;
}
function cartaBisogno(T, p, b, pre){
  const [cl, parola] = statoCarta(T,b), aperta = S.rqAperta===b.key;
  const vociAperte = b.voci.filter(v=>!v.fatto), daPreparare = b.cands.some(c=>c.voci.length && !b.righe.some(q=>q.forn.id===c.id));
  const codici = [...new Map(b.voci.map(v=>[v.cid+"|"+(v.prep||v.fid||""), v])).values()].map(v=>`<span class="ma-st ma-st--neu ma-st--piatto" title="${esc(v.descr||"")}"><span class="mono">${esc(suffisso(v.cod,pre))}</span>&nbsp;${esc(v.dove||"")}${v.grezzo?" · "+esc(v.grezzo):""}${v.fb?" · "+esc(fmt(v.fb.kg,0))+" kg":""}${v.fatto?" ✓":""}</span>`).join("");
  const TR = TIPI_RICHIESTA[b.tipo]||{};
  return `<article class="rq${S.rqSu===b.key?" rq--su":""}${b.tipo==="attrezzatura"?" rq--prep":""}" data-rq="${esc(b.key)}">
    <details class="rq-carta" name="rq" data-rq-carta="${esc(b.key)}" ${aperta?"open":""}>
      <summary class="rq-t"><b>${esc(b.titolo)}</b><span class="ma-st ma-st--${cl}">${esc(parola)}</span>
        <span class="rq-riga2"><span class="k3 piccolo">${esc(vociAperte.length?vociAperte.map(v=>`${suffisso(v.cod,pre)} ${v.dove||""}`).join(" · "):"tutte registrate")}</span><span class="k3 piccolo">${b.righe.filter(q=>q.stato!=="annullata").length} di ${b.cands.length} fornitori</span></span></summary>
      <div class="rq-corpo">
        <div class="rq-cod">${codici}</div>
        <div class="piccolo k3">${esc(testoPezzi({lotti:lottiDi(T).lotti.join(" / "), codici:b.voci.map(v=>({cod:v.cod, qta:v.qta}))}))}${certRichiesto(T)?" · certificato "+esc(certRichiesto(T)):""}${TR.via?" · "+esc(TR.via):""}</div>
        ${daPreparare?`<label class="rq-nota"><span class="lab">Nota per i fornitori</span><textarea data-rq-nota="${esc(b.key)}" aria-label="Nota della richiesta" placeholder="istruzioni, conto lavoro o vendita, consegna">${esc((S.rqNote||{})[b.key] || notaBisogno(T,p,b,vociAperte))}</textarea></label>`:""}
        <div class="rq-forni" role="list" aria-label="Fornitori">${b.cands.map(c=>rigaFornitore(T,p,b,c,pre)).join("")}</div>
      </div></details></article>`;
}
function cartaDomanda(T, p, q, pre){
  const TR = TIPI_RICHIESTA[q.tipo]||{nome:q.tipo}, aperta = S.rqAperta===q.id;
  const cod = q.codici.map(c=>`<span class="ma-st ma-st--neu ma-st--piatto"><span class="mono">${esc(suffisso(c.cod,pre))}</span></span>`).join("");
  return `<article class="rq${S.rqSu===q.id?" rq--su":""}" data-rq="${esc(q.id)}">
    <details class="rq-carta" name="rq" data-rq-carta="${esc(q.id)}" ${aperta?"open":""}>
      <summary class="rq-t"><b>${esc(TR.nome)}</b><span class="ma-st ma-st--${q.stato==="bozza"?"neu":q.stato==="risposta"?"ok":q.stato==="inviata"?"prop":"ver"}">${esc(NOMI_STATO_RQ[q.stato]||q.stato)}</span>
        <span class="rq-riga2"><span class="k3 piccolo">${esc(q.nataDa||q.codici.map(c=>suffisso(c.cod,pre)).join(", "))}</span><span class="k3 piccolo">a ${esc(q.dest.nome)}</span></span></summary>
      <div class="rq-corpo">${cod?`<div class="rq-cod">${cod}</div>`:""}${q.testo?`<p class="piccolo">${esc(q.testo)}</p>`:""}
        <div class="rq-forni">${rigaRichiesta(T, p, q, `<span class="rq-f-nome"><b>${esc(q.dest.nome)}</b><small class="k3">proposta ${esc(TR.r||"")}</small></span>`, pre)}</div></div></details></article>`;
}
function rispostaHtml(T, p, q, reg){
  const x = q.risp;
  if(x.declina) return `<div class="rq-risp rq-risp--no"><span><b>${esc(q.forn?q.forn.nome:q.dest.nome)}</b> declina: «${esc(x.declina)}» · ${esc(x.data)}</span></div>`;
  const c0 = q.codici[0], dove = q.preps && q.preps.length ? "nella preparazione" : /^mp_/.test(q.tipo) ? "sul grezzo" : c0 && c0.fid ? (()=>{ const f = faseDelThread(T,c0.fid); return f ? `nella f.${(f.i+1)*10} di ${c0.cod.slice(-4)}` : ""; })() : c0 ? `in ${c0.cod.slice(-4)}` : "";
  const m5 = (q.controlli||[]).some(k=>k.cod==="MAT-05");
  if(x.righe && x.righe[0] && x.righe[0].lotti) return `<div class="rq-risp"><span>${x.righe.map(r=>`<b class="mono">${esc(r.cod)}</b> ${r.lotti.map(([l,e])=>`${l} pz ${fmt(e,2)} €`).join(" · ")}`).join(" · ")}${x.tempi?" · "+esc(x.tempi):""}${x.materiale?" · materiale «"+esc(x.materiale)+"»":""}${x.allegato?" · "+esc(x.allegato):""} ${chipF("RISP")} <span class="k3">giro ${x.giro} · ${esc(x.data)}</span></span>
    ${(q.controlli||[]).map(k=>`<span><span class="liv liv--${k.liv}">${k.cod}</span> ${esc(k.testo)}${k.cod==="MAT-05"?" · registrata con riserva: esce dalla scelta automatica dei costi (default, da confermare)":""}</span>`).join("")}
    ${reg?`<span><span class="ma-st ma-st--ok" title="R-13: scritta nel punto d'origine con la fonte RISP">registrata: ${esc(reg)}</span></span>`:""}</div>`;
  return `<div class="rq-risp"><span>${x.prezzo!=null?`<b>${fmt(x.prezzo,2)} ${esc(x.unita||"")}</b>`:""}${x.lotto?" · lotto "+esc(x.lotto):""}${x.gg?" · "+x.gg+" gg":""}${x.settimane?" · "+x.settimane+" settimane":""}${x.materiale?" · materiale «"+esc(x.materiale)+"»":""}${x.valida?" · valida "+esc(x.valida):""}${x.testo?" · "+esc(x.testo):""} ${chipF("RISP")} <span class="k3">${esc(x.data)}</span></span>
    ${(q.controlli||[]).map(k=>`<span><span class="liv liv--${k.liv}">${k.cod}</span> ${esc(k.testo)}</span>`).join("")}
    <span>${reg ? `<span class="ma-st ma-st--ok" title="R-13: scritta nel punto d'origine con la fonte RISP">registrata: ${esc(reg)}</span>`
      : `${btnB(`/rfq/${T.id}/risposta`, {richiesta:q.id}, `Registra${dove?" "+esc(dove):""}`, m5?"ma-btn--ghost":"pa-btn")}${m5?" "+btnB(`/rfq/${T.id}/domanda`, {tipo:"qualita", cid:c0?c0.cid:"", testo:`${q.forn?q.forn.nome:""} dichiara «${x.materiale}»: è confrontabile con il disegno?`}, "✉ Chiedi alla qualità", "pa-btn"):""}`}</span></div>`;
}
/* la nota della carta: un campo di chi scrive (presentazione); entra nella richiesta solo con «Prepara la bozza» */
document.addEventListener("change", e=>{
  const el = e.target, ds = el.dataset||{};
  if(ds.rqNota){ S.rqNote = S.rqNote || {}; S.rqNote[ds.rqNota] = el.value; render(); return; }
});
/* aprire il cassetto su una carta (da Ciclo, Sequenza, Come, Esito): navigazione, nessuna scrittura */
document.addEventListener("click", e=>{
  const c = e.target.closest && e.target.closest("[data-rq]"); if(c) S.rqAperta = c.dataset.rq;   /* la carta toccata resta aperta dopo il ridisegno */
  const t = e.target.closest && e.target.closest("[data-rq-apri]"); if(!t) return;
  e.preventDefault(); S.cassetto = "richieste"; S.rqSu = t.dataset.rqApri || null; S.rqAperta = S.rqSu; S.menu = null; render();
  const el = document.querySelector(`#cassetto [data-rq="${S.rqSu}"]`); if(el && el.scrollIntoView) try{ el.scrollIntoView({block:"nearest"}); }catch(err){}
});
document.addEventListener("toggle", e=>{ const d = e.target; if(d && d.dataset && d.dataset.rqRegDet){ S.rqRegAperta = S.rqRegAperta || {}; S.rqRegAperta[d.dataset.rqRegDet] = d.open; } }, true);
/* una carta aperta alla volta anche dopo il ridisegno: si ricorda quale ha aperto chi guarda */
document.addEventListener("toggle", e=>{ const d = e.target; if(!d || !d.dataset || !d.dataset.rqCarta) return;
  if(d.open) S.rqAperta = d.dataset.rqCarta; else if(S.rqAperta===d.dataset.rqCarta && !document.querySelector("details[data-rq-carta][open]")) S.rqAperta = "-"; }, true);
