/* ================= IL CONTRATTO CON IL BACKEND (ramo smistamento-giro5) =================
   Il mockup usa il vocabolario del backend e dichiara, per ogni gesto, la rotta che chiamerebbe.
   - Enum copiati dallo schema (migrazioni 0001–0021): fase, stato_thread, tipo_componente, tipo_documento,
     stato_proposta, stato_richiesta_fornitore, esito_fattibilita.
   - FASI: il catalogo seminato in fase_catalogo, con ufficio e SLA; le due transizioni automatiche di
     «transizione» (RICEVUTA/ATTESA_DISEGNI → FATTIBILITA quando v_fascicolo non ha righe bloccanti aperte).
   - GESTI: gesto del mockup → rotta HTTP vera, oppure la richiesta R-nn che manca (RICHIESTE_a_Pietro.md).
   - DTO: i nomi dei tipi di valutazione (ProdottoValutato, StatoFascicolo, CompletezzaDocumentale, NodoBOM…),
     costruiti qui dai dati finti; nel frontend vero arrivano già calcolati dal server. */
const ENUM={
 fase:["RICEVUTA","ATTESA_DISEGNI","FATTIBILITA","SCHEDA_COSTO","OFFERTE_FORN","OFFERTA_INVIATA","ACCETTATA","DISTINTA_ERP","ORDINE","PRODUZIONE","PERSA","RESPINTA","SCADUTA"],
 stato_thread:["APERTA","CHIUSA"],
 tipo_componente:["finito","sottoassieme","sciolto","commerciale"],
 tipo_documento:["cad_3d","disegno_2d","sviluppo_dxf","capitolato","distinta_cliente","commerciale","offerta_fornitore","offerta_promatec","ordine_cliente","corrispondenza","rumore","altro","da_determinare"],
 stato_proposta:["aperta","confermata","scartata","duplicato"],
 stato_richiesta_fornitore:["bozza","inviata","offerta_ricevuta","declinata","scaduta","annullata"],
 esito_fattibilita:["OK","KO","DUBBIO"]
};
/* il mockup tiene chiavi corte; verso il backend si traducono così */
const TIPO_BE={finito:"finito",sottoass:"sottoassieme",sciolto:"sciolto",comm:"commerciale"};
const DOC_BE={step:"cad_3d",pdf:"disegno_2d",d3:"cad_3d",d2:"disegno_2d",dxf:"sviluppo_dxf"};
const STATO_BE={prop:"aperta",verifica:"aperta",ok:"confermata"};
/* fase_catalogo, come lo semina la migrazione */
const FASI={RICEVUTA:[10,"Ricevuta","Sistema"],ATTESA_DISEGNI:[15,"Attesa disegni","Commerciale"],FATTIBILITA:[20,"Fattibilità","Tecnico"],
 SCHEDA_COSTO:[30,"Scheda costo","Commerciale"],OFFERTE_FORN:[31,"Offerte fornitori","Acquisti"],OFFERTA_INVIATA:[40,"Offerta inviata","Commerciale"],
 ACCETTATA:[50,"Accettata","Commerciale"],DISTINTA_ERP:[60,"Distinta nel gestionale","Uff. tecnico"],ORDINE:[70,"Ordine","Sistema"],
 PRODUZIONE:[80,"Produzione","Produzione"],PERSA:[90,"Persa","—"],RESPINTA:[91,"Respinta (non fattibile)","—"],SCADUTA:[92,"Scaduta","—"]};
const TERMINALI=new Set(["PERSA","RESPINTA","SCADUTA"]);
/* la fase con le due transizioni automatiche (fatto richiesto: v_fascicolo, righe bloccanti con esito <> ok) */
function faseCorrente(q){
  const f=q.faseBase||"RICEVUTA";
  if(f!=="RICEVUTA"&&f!=="ATTESA_DISEGNI") return f;
  const st=stat(datiRFQ(q).prodotti), bloccanteMancante=st.mancanti.some(x=>x[2]==="manca"||x[2]==="sul portale");
  if(bloccanteMancante) return "ATTESA_DISEGNI";
  return st.pronto?"FATTIBILITA":f;
}
/* i gruppi della gestione sono solo presentazione, calcolati dalla fase e dallo stato del thread */
function gruppoDi(q){
  const f=faseCorrente(q);
  if(TERMINALI.has(f)||q.statoThread==="CHIUSA") return "terminata";
  if(f==="PRODUZIONE") return "produzione";
  if(f==="ACCETTATA"||f==="DISTINTA_ERP"||f==="ORDINE") return "accettata";
  return "avviata";
}
function preparaRFQ(q){
  if(Object.getOwnPropertyDescriptor(q,"stato")) delete q.stato;
  Object.defineProperty(q,"stato",{get(){return gruppoDi(this);},enumerable:false,configurable:true});
  Object.defineProperty(q,"fase",{get(){return faseCorrente(this);},enumerable:false,configurable:true});
  q.statoThread=q.statoThread||"APERTA";
  return q;
}
/* ---- gesti → rotte del backend ---- */
const R_=(m,p,stato,nota)=>({m,p,stato:stato||"esiste",nota:nota||""});
const GESTI={
 apririfq:R_("GET","/thread/{id}/fascicolo","esiste","sola lettura"),
 step0:R_("GET","/thread/{id}/fascicolo","esiste","sola lettura"),
 step1:R_("GET","/thread/{id}/distinta","esiste","sola lettura: niente POST prepara all’apertura"),
 step2:R_("GET","/thread/{id}","esiste","offerta: solo lettura"),
 crearfq:R_("JOB","crea_cartella_thread","R-02","significato di «Crea l’RFQ» da confermare"),
 creareq:R_("POST","/messaggio/{id}/rfq"),
 aggancia:R_("POST","/messaggio/{id}/aggancia"),
 nonric:R_("POST","/messaggio/{id}/ignora"),
 conf:R_("POST","/thread/{id}/fascicolo/proposta/{pid}/decidi","esiste","conferma"),
 confstep:R_("POST","/thread/{id}/fascicolo/componente/{cid}/step-strutturale","esiste","autorizza la fonte"),
 confall:R_("POST","/thread/{id}/fascicolo/conferma"),
 rim:R_("POST","/thread/{id}/fascicolo/proposta/{pid}/decidi","esiste","scarta"),
 usa:R_("POST","/thread/{id}/fascicolo/documento/{did}/sostituisci"),
 cerca:R_("POST","/thread/{id}/fascicolo/rianalizza"),
 carica:R_("POST","/thread/{id}/fascicolo/carica","R-06","oggi a livello di thread: manca il caricamento dalla riga del componente"),
 assoc:R_("POST","/thread/{id}/fascicolo/assegna"),
 libvia:R_("POST","/thread/{id}/fascicolo/proposta/{pid}/decidi","esiste","messo da parte = scarta"),
 nas:R_("POST","/thread/{id}/fascicolo/conferma","esiste","conferma e copia (job copia_nas)"),
 mincf:R_("POST","(spazio di verifica) conferma della categoria","R-08","gesto nuovo"),
 cfconferma:R_("POST","(spazio di verifica) conferma fascicolo cumulativa","SV","E2 §3.3: riepilogo, impronta, esito unico"),
 nodoaccetta:R_("POST","/thread/{id}/fascicolo/nodo/{pid}/accetta"),
 nodoscarta:R_("POST","/thread/{id}/fascicolo/nodo/{pid}/scarta"),
 nodocodice:R_("POST","/thread/{id}/fascicolo/nodo/{pid}/codice"),
 accettatutto:R_("POST","/thread/{id}/distinta/albero/conferma"),
 salvadistinta:R_("POST","/thread/{id}/fascicolo/bom/applica","esiste","tutto in una volta"),
 sposta:R_("POST","/thread/{id}/fascicolo/componente/{cid}/sposta","R-07","con la regola dei contenitori"),
 tipo:R_("POST","/thread/{id}/fascicolo/componente/{cid}/tipo"),
 rimuovi:R_("POST","/thread/{id}/fascicolo/componente/{cid}/rimuovi"),
 rifai:R_("POST","/thread/{id}/fascicolo/rianalizza"),
 nota:R_("POST","/thread/{id}/fascicolo/nota","esiste","con la fase: R-14"),
 notaelimina:R_("POST","/thread/{id}/fascicolo/nota/{nid}/elimina"),
 ciclo:R_("POST","(nuovo) ciclo di produzione","R-10","ciclo, fasi, componenti per fase"),
 preparazione:R_("POST","(nuovo) preparazioni del ciclo","R-11"),
 richiestaterzista:R_("POST","/thread/{id}/richiesta","esiste","istruzioni e allegati: R-13"),
 buyeraggiungi:R_("POST","/admin/anagrafica/{id}/buyer"),
 buyerelimina:R_("POST","/admin/anagrafica/{id}/buyer/elimina"),
 buyersospendi:R_("POST","/admin/anagrafica/{id}/buyer","R-04","oggi c’è solo «confermato»"),
 clienteattivo:R_("POST","/admin/anagrafica/{id}"),
 clientenuovo:R_("POST","/admin/anagrafica")
};
const CHIAMATE=[];
function traccia(gesto,param){
  const g=GESTI[gesto]; if(!g) return;
  const p=g.p.replace("{id}",(param&&param.id)||"…").replace(/\{(pid|cid|did|nid)\}/,(param&&param.ref)||"…");
  CHIAMATE.unshift({gesto,m:g.m,p,stato:g.stato,nota:g.nota,ora:new Date().toTimeString().slice(0,8)});
  if(CHIAMATE.length>60) CHIAMATE.pop();
}
/* i data-* del mockup → gesto (ascolto in cattura: registra prima che l'azione cambi lo stato) */
const DATA_GESTO={apririfq:"apririfq",crearfq:"crearfq",creareq:"creareq",aggancia:"aggancia",nonric:"nonric",confall:"confall",rim:"rim",usa:"usa",
 cerca:"cerca",carica:"carica",assoc:"assoc",libvia:"libvia",nas:"nas",mincf:"mincf",addmitt:"buyeraggiungi",togli:"buyerelimina",mitt:"buyersospendi",
 attivo:"clienteattivo",salvacli:"clientenuovo",wfp:"preparazione"};
const DX_GESTO={accetta:"accettatutto",salva:"salvadistinta",conferma:"nodoaccetta",scartaok:"nodoscarta",eliminaok:"rimuovi",aggiungi:"salvadistinta",rifai:"rifai"};
document.addEventListener("click",e=>{
  const t=e.target.closest&&e.target.closest("[data-step],[data-conf],[data-dx],[data-dvx],[data-wf],[data-cf]"+Object.keys(DATA_GESTO).map(k=>",[data-"+k+"]").join("")); if(!t) return;
  const D=t.dataset, id=S.rfq||"…";
  if(D.step!==undefined) return traccia("step"+D.step,{id});
  if(D.conf) return traccia(D.conf.endsWith("|step")?"confstep":"conf",{id,ref:D.conf.split("|")[1]});
  if(D.dx&&DX_GESTO[D.dx]) return traccia(DX_GESTO[D.dx],{id,ref:typeof DIST!=="undefined"?DIST.sel:""});
  if(D.dvx==="salvanota") return traccia("nota",{id});
  if(D.dvx&&D.dvx.startsWith("togli:")) return traccia("notaelimina",{id,ref:D.dvx.slice(6)});
  if(D.wf&&/^(aggiungi|conferma|su|giu|via|proponi|usa)/.test(D.wf)) return traccia("ciclo",{id});
  if(D.wf&&D.wf.startsWith("invia")) return traccia("richiestaterzista",{id});
  if(D.cf==="conferma") return traccia("cfconferma",{id});
  for(const k in DATA_GESTO) if(D[k]!==undefined) return traccia(DATA_GESTO[k],{id:k==="aggancia"||k==="nonric"?D[k]:id,ref:D[k]});
},true);
document.addEventListener("change",e=>{
  const id=e.target.id, rfq=S.rfq||"…";
  if(id==="dd-padre") traccia("sposta",{id:rfq}); else if(id==="dd-tipo") traccia("tipo",{id:rfq}); else if(id==="dd-cod") traccia("nodocodice",{id:rfq});
},true);

/* ---- i DTO, con i nomi dei tipi del backend (dai dati finti del mockup) ---- */
function dtoProdottoValutato(q,pid){
  const A=assiProdotto(q,pid), v=Object.fromEntries(A.map(([k,,s])=>[k,s]));
  return {Codice:PROD[pid].cod,Target:{Stato:v.target},Fonte:{Stato:v.fonte},
    BOM:{Nomenclatura:v.nom,Gerarchia:v.ger},Smistamento:{Stato:v.smi},Documenti:{Stato:v.doc},
    Stato:v.stato,Verificato:v.stato==="pronto_fattibilita",Motivi:A.filter(([k,,s])=>k!=="stato"&&!["confermata","verificata","verificato","completa"].includes(s)).map(([k,,s,why])=>({asse:k,valore:s,motivo:why}))};
}
function dtoStatoFascicolo(q){
  const d=datiRFQ(q), pronti=d.prodotti.filter(p=>assiProdotto(q,p)[6][2]==="pronto_fattibilita");
  return {NumeroTarget:d.prodotti.length,NumeroVerificati:pronti.length,Pronti:pronti.map(p=>PROD[p].cod),
    Bloccati:d.prodotti.filter(p=>!pronti.includes(p)).map(p=>PROD[p].cod),Congelabile:pronti.length===d.prodotti.length&&d.prodotti.length>0,
    Congelato:false,MotivoNonCongelato:"gesto_non_registrato",FaseThread:faseCorrente(q),StatoThread:q.statoThread};
}
function dtoCompletezza(pid){
  const voci=chiavi(pid).filter(k=>obblig(pid,k)).map(k=>{const s=getSlot(pid,k), p=k.startsWith("p:")?DOCS[pid].parti[+k.split(":")[1]]:null;
    return {Componente:p?p.cod:PROD[pid].cod,TipoComponente:TIPO_BE[p?p.tipo:"finito"],Tipo:DOC_BE[k.split(":").pop()],
      Esito:!s||s.portale?"manca":s.stato==="ok"?"presente":"da_verificare",Motivo:!s?"nessun_file":s.portale?"sul_portale":s.stato==="ok"?"":"associazione_non_confermata",
      FileCandidato:s&&!s.portale?s.f:null,Categoria:p?(p.cat||(p.tipo==="comm"?"commerciale":"fabbricato")):"fabbricato",Invariante:!p};});
  const st=assiProdotto(rf(S.rfq)||RFQ[0],pid)[5][2];
  return {Stato:st,PerimetroChiuso:st!=="non_calcolabile",Voci:voci};
}
function dtoNodiBOM(pid){
  const B=bomDi(pid);
  return B.nodes.map(n=>({Rif:n.pk||n.id,Codice:n.codice,Tipo:TIPO_BE[n.tipo],Padre:n.padre?(nodo(B,n.padre)||{}).codice:null,Quantita:n.padre?n.qta:null,
    Stato:n.proposto?"aperta":"confermata",Classificazione:{Ruolo:n.padre?"componente":"prodotto",Categoria:n.cat||(n.tipo==="comm"?"commerciale":"fabbricato"),Confermata:!!(n.catConf)},
    Evidenze:(n.fonti||[]).map(([,a,b])=>a+": "+b)}));
}
/* ---- il pannello «Dati e chiamate» della pagina RFQ ---- */
const DEV={aperto:false,tab:"dto"};
function pannelloContratto(){
  if(!DEV.aperto) return "";
  const q=rf(S.rfq), pid=S.rp; if(!q) return "";
  const dto={ProdottoValutato:dtoProdottoValutato(q,pid),StatoFascicolo:dtoStatoFascicolo(q),CompletezzaDocumentale:dtoCompletezza(pid),NodoBOM:dtoNodiBOM(pid)};
  return `<div class="backdrop" data-dev="chiudi"></div><aside class="drawer largo" role="dialog" aria-label="Dati e chiamate">
    <header><div style="min-width:0"><span class="lab">Contratto con il backend · ${esc(PROD[pid].cod)}</span><h3>Dati e chiamate</h3></div><span class="sp"></span>
      <button class="btn sm ghost" data-dev="chiudi" aria-label="Chiudi">${I("x",15)}</button></header>
    <div class="dbody"><div class="seg2" role="tablist"><button role="tab" data-dev="dto" aria-selected="${DEV.tab==="dto"}">DTO della pagina</button>
      <button role="tab" data-dev="chiamate" aria-selected="${DEV.tab==="chiamate"}">Chiamate · ${CHIAMATE.length}</button></div>
      ${DEV.tab==="dto"?`<p class="hint" style="margin:0">Quello che la pagina legge, con i nomi dei tipi di <span class="mono">valutazione</span>. Qui è costruito dai dati del mockup; nel frontend vero arriva dal server, già calcolato.</p>
        <pre class="dto">${esc(JSON.stringify(dto,null,2))}</pre>`
      :`<p class="hint" style="margin:0">Ogni gesto del mockup e la rotta che chiamerebbe. <b>esiste</b> = c’è nel ramo; <b>R-nn</b> = manca, vedi le richieste; <b>SV</b> = spazio di verifica.</p>
        ${CHIAMATE.length?`<table class="tb chiamate"><thead><tr><th>Ora</th><th>Metodo</th><th>Rotta</th><th>Stato</th></tr></thead><tbody>${CHIAMATE.map(c=>`<tr><td class="mono">${c.ora}</td><td class="mono">${c.m}</td>
          <td class="mono" style="overflow-wrap:anywhere">${esc(c.p)}${c.nota?`<div class="k3" style="font-family:var(--fb)">${esc(c.nota)}</div>`:""}</td><td>${c.stato==="esiste"?C("ok","esiste"):C(c.stato==="SV"?"acc":"warn",c.stato)}</td></tr>`).join("")}</tbody></table>`
          :`<p class="k3" style="margin:0">Nessuna chiamata ancora: fai un gesto nella pagina.</p>`}`}</div>
    <footer><button class="btn" data-dev="chiudi">Chiudi</button></footer></aside>`;
}
document.addEventListener("click",e=>{const t=e.target.closest&&e.target.closest("[data-dev]"); if(!t) return; const a=t.dataset.dev;
  if(a==="apri"){DEV.aperto=true;DEV.tab="dto";} else if(a==="chiudi") DEV.aperto=false; else DEV.tab=a; render();});
