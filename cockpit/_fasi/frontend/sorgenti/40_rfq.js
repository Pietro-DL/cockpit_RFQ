
/* ================= GESTIONE RFQ ================= */
function filtroRFQ(q){
  const d=datiRFQ(q), c=cl(d.cli);
  if(S.fcli&&d.cli!==S.fcli) return false;
  if(!S.q) return true;
  const hay=[q.num,c.nome,d.titolo,d.buyer,...d.prodotti.map(p=>PROD[p].cod+" "+PROD[p].nome)].join(" ").toLowerCase();
  return hay.includes(S.q.toLowerCase());
}
function gestione(){
  const clientiRFQ=[...new Set(RFQ.map(q=>datiRFQ(q).cli))];
  return `<div class="gest">
    <div class="ghead"><div><h2>Richieste RFQ</h2><p class="k" style="margin:4px 0 0">Tutte le richieste d’offerta, dall’avvio alla consegna</p></div>
      <span class="sp"></span>
      <div class="search"><span class="ic">${I("search",15)}</span><input id="gq" value="${esc(S.q)}" placeholder="Cerca numero, cliente, codice, buyer" aria-label="Cerca RFQ"></div>
      <select id="gcli" aria-label="Filtra per cliente"><option value="">Tutti i clienti</option>
        ${clientiRFQ.map(id=>`<option value="${id}" ${S.fcli===id?"selected":""}>${esc(cl(id).nome)}</option>`).join("")}</select></div>
    <div class="stati" role="tablist" id="gstati">${gstati()}</div>
    <div class="gbody" id="gbody">${gbody()}</div></div>`;
}
function gstati(){
  return STATI.map(([k,l])=>`<button class="stato" role="tab" data-stato="${k}" aria-selected="${S.stato===k}">${l}
    <span class="cnt">${RFQ.filter(q=>q.stato===k&&filtroRFQ(q)).length}</span></button>`).join("");
}
function gbody(){
  const rows=RFQ.filter(q=>q.stato===S.stato&&filtroRFQ(q));
  const senza=RICHIESTE.filter(r=>!r.rfq&&cl(r.cli).attivo&&(!S.fcli||r.cli===S.fcli));
  const blocco = S.stato==="avviata"&&senza.length?`<div class="senza">
    <div style="display:flex;align-items:center;gap:8px">${I("tray",16)}<b>${plur(senza.length,"richiesta","richieste")} nell’Inbox senza RFQ</b>
      <span class="k3" style="font-size:var(--t-sm)">crea l’RFQ quando sei pronto a lavorarla</span></div>
    ${senza.map(r=>{const c=cl(r.cli), st=stat(r.prodotti); return `<div class="riga">
      <span class="av lg" style="background:${c.col}">${c.sigla}</span>
      <span style="min-width:0"><b style="font-weight:500">${esc(r.titolo)}</b>
        <div class="k3" style="font-size:var(--t-sm)">${esc(c.nome)} · ${r.prodotti.map(p=>PROD[p].cod).join(", ")} · ricevuta ${esc(r.aperta)} · ${st.pres} documenti riconosciuti</div></span>
      <span style="display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end">
        <button class="btn sm" data-req="${r.id}">${I("inbox",13)} Conversazione</button>
        <button class="btn sm pri" data-crearfq="${r.id}">${I("plus",13)} Crea l’RFQ</button></span></div>`;}).join("")}</div>`:"";
  if(!rows.length) return gwrap(blocco,vuoto("file","Nessuna RFQ in questo stato",S.q||S.fcli?"Nessun risultato con questi filtri.":"Quando un’RFQ passa a questo stato compare qui."));
  const riga=q=>{const d=datiRFQ(q), c=cl(d.cli), st=stat(d.prodotti);
    return {q,d,c,st,passo:chipStato(q)};};
  const R=rows.map(riga);
  return gwrap(blocco,`<div class="wrap-tb desk" style="overflow-x:auto"><table class="tb"><thead><tr>
      <th>RFQ</th><th>Cliente</th><th>Richiesta</th><th>Prodotti</th><th>Fase</th><th>STEP · PDF</th><th>Scadenza</th><th>Resp.</th><th>Aggiornata</th></tr></thead>
    <tbody>${R.map(({q,d,c,st,passo})=>`<tr class="click" data-apririfq="${q.id}">
      <td class="mono" style="font-weight:600;color:var(--acc);white-space:nowrap">${esc(q.num)}</td>
      <td><span style="display:flex;align-items:center;gap:8px"><span class="av" style="background:${c.col}">${c.sigla}</span><span>${esc(c.nome)}
        <div class="k3" style="font-size:var(--t-xs)"><span class="sett" style="background:${SETT[c.sett][1]}"></span> ${SETT[c.sett][0]}</div></span></span></td>
      <td>${esc(d.titolo)}${q.nota?`<div class="k3" style="font-size:var(--t-xs)">${esc(q.nota)}</div>`:""}</td>
      <td>${d.prodotti.map(p=>`<span class="mono" style="font-size:var(--t-sm)">${PROD[p].cod}</span>`).join("<br>")}</td>
      <td>${passo}</td>
      <td>${st.pronto?C("ok","completi","check"):`${C(st.fonteOk===st.fonteTot?"ok":"prop","STEP "+st.fonteOk+"/"+st.fonteTot)} ${C(st.pdfConf===st.pdfO?"ok":st.manca?"bad":"prop","PDF "+st.pdfConf+"/"+st.pdfO)}`}</td>
      <td class="mono" style="font-size:var(--t-sm)">${esc(d.scad)}</td><td>${esc(q.resp)}</td>
      <td class="mono k3" style="font-size:var(--t-sm);white-space:nowrap">${esc(q.agg)}</td></tr>`).join("")}</tbody></table></div>
    <div class="cards">${R.map(({q,d,c,st,passo})=>`<button class="rcard" data-apririfq="${q.id}">
      <span style="display:flex;align-items:center;gap:8px"><span class="av" style="background:${c.col}">${c.sigla}</span>
        <span class="mono" style="font-weight:600;color:var(--acc)">${esc(q.num)}</span><span style="flex:1"></span>${passo}</span>
      <span><b style="font-weight:500">${esc(d.titolo)}</b></span>
      <span class="k3" style="font-size:var(--t-sm)">${esc(c.nome)} · ${d.prodotti.map(p=>PROD[p].cod).join(", ")} · PDF ${st.pdfConf}/${st.pdfO} · ${esc(q.resp)}</span></button>`).join("")}</div>`);
}

/* sugli schermi larghi le richieste senza RFQ stanno accanto all’elenco, non sopra */
const gwrap=(blocco,elenco)=>`<div class="gwrap${blocco?" con-senza":""}"><div class="gmain">${elenco}</div>${blocco}</div>`;

/* ================= creazione e apertura dell'RFQ ================= */
function creaRFQ(rid){
  const r=rq(rid);
  if(r.rfq){ apriRFQ(r.rfq); return; }                       /* esiste già: si riapre, niente doppioni */
  const c=cl(r.cli), [g,mm,aa]=r.aperta.split("/"), cogn=r.buyer.split(" ").pop();
  const num=r.numCli||("P-2026-0"+(++contatoreRFQ));
  const q={id:"q-"+rid,num,numProvvisorio:!r.numCli,req:rid,faseBase:"RICEVUTA",passo:0,resp:"Franco",agg:"adesso",
    nas:`${c.nas}\\WIP\\${aa} ${mm} ${g} ${cogn} ${r.titolo.replace(/[\\/:*?"<>|]/g," ").replace(/\s+/g," ").trim()}`};
  RFQ.push(preparaRFQ(q)); r.rfq=q.id;
  apriRFQ(q.id); S.creato=q.id;
}
function apriRFQ(id){ S.tab="rfq"; S.rfq=id; S.step=0; S.dview="struttura"; S.rp=datiRFQ(rf(id)).prodotti[0]; S.insp=null; S.esito=null; S.creato=null; }

/* ================= PAGINA RFQ ================= */
const PROVL={mail:"dalla mail",port:"dal portale",car:"caricato a mano"};
function miniDis(kind){
  if(kind==="step") return `<svg viewBox="0 0 120 84" aria-hidden="true"><rect width="120" height="84" fill="#fff"/>
    <g fill="none" stroke="#3a3a36" stroke-width="1.1" stroke-linejoin="round">
    <path d="M30 34 60 20 92 34 62 48z"/><path d="M30 34v22l32 14V48M92 34v22L62 70"/>
    <path d="M44 41v12M78 41v12" stroke-dasharray="2 2" stroke="#8d8d86"/><circle cx="61" cy="34" r="4"/></g>
    <text x="6" y="80" font-family="IBM Plex Mono,monospace" font-size="7" fill="#8d8d86">STEP · 3D</text></svg>`;
  return `<svg viewBox="0 0 120 84" aria-hidden="true"><rect width="120" height="84" fill="#fff"/>
    <g fill="none" stroke="#3a3a36" stroke-width="1"><rect x="4" y="4" width="112" height="76"/>
    <rect x="76" y="62" width="40" height="18"/><path d="M76 68h40M96 62v18" stroke-width=".6"/>
    <path d="M16 16h44l6 6v26l-6 6H16l-4-4V20z" stroke-width="1.1"/>
    <circle cx="24" cy="24" r="2.4"/><circle cx="56" cy="24" r="2.4"/><circle cx="24" cy="46" r="2.4"/><circle cx="56" cy="46" r="2.4"/>
    <path d="M12 60h54M12 58v4M66 58v4" stroke-width=".6"/><path d="M80 18v34M80 18h8v34h-8" stroke-width="1"/></g>
    <text x="6" y="78" font-family="IBM Plex Mono,monospace" font-size="6" fill="#8d8d86">PDF · 2D</text></svg>`;
}
const kindOf = k=>(k==="step"||k.endsWith(":d3"))?"step":"pdf";
function impronta(s){let h=2166136261;for(const ch of s)h=Math.imul(h^ch.charCodeAt(0),16777619)>>>0;const x=h.toString(16).padStart(8,"0");return x.slice(0,4)+"…"+x.slice(4);}

function paginaRFQ(){
  const q=rf(S.rfq); if(!q) return vuoto("file","RFQ non trovata","Torna a «Richieste RFQ».");
  const d=datiRFQ(q), c=cl(d.cli);
  if(!S.rp||!d.prodotti.includes(S.rp)) S.rp=d.prodotti[0];
  const st=stat(d.prodotti);
  const stati=[[st.pronto?"ok":"warn",`STEP ${st.fonteOk}/${st.fonteTot} · PDF ${st.pdfConf}/${st.pdfO}`],statoDistinta(d.prodotti),["",q.stato==="avviata"?(q.nota||"dopo la distinta"):(q.nota||q.esito||"chiusa")]];
  return `<div class="rfqhead">
    <div class="bc">${I("file",14)}<button class="btn sm ghost" data-tab="gest">Richieste RFQ</button>${I("chev",13)}<b>${esc(c.nome)}</b>${I("chev",13)}<b>RFQ ${esc(q.num)}</b>
      <span style="flex:1"></span><button class="btn sm" data-dev="apri" title="I DTO che la pagina legge e le rotte che chiama">{ } Dati e chiamate</button>${d.req?`<button class="btn sm" data-req="${d.req.id}">${I("inbox",13)} Apri la conversazione</button>`:""}</div>
    ${S.creato===q.id?`<div class="strip okk" style="margin:0 0 12px">${I("check",15)}<b>RFQ ${esc(q.num)} creata.</b>
      La cartella è sul NAS; i documenti riconosciuti nella richiesta sono già al loro posto, da controllare e confermare.</div>`:""}
    ${cart(`<div class="cell"><span class="lab">Richiesta d’offerta</span><h1>${esc(q.num)}</h1>${q.numProvvisorio?C("warn","numero provvisorio · R-03"):C("ghost","riferimento del cliente")}<span class="nome">${esc(d.titolo)}</span></div>
      <div class="cell"><span class="lab">Cliente</span><div class="v">${esc(c.nome)}</div><span class="k3" style="font-size:var(--t-xs)">${SETT[c.sett][0]}</span></div>
      <div class="cell"><span class="lab">Buyer</span><div class="v">${esc(d.buyer)}</div></div>
      <div class="cell"><span class="lab">Ricevuta</span><div class="v">${esc(d.aperta.split(" ")[0])}</div><span class="k3" style="font-size:var(--t-xs)">${esc(d.aperta.split(" ")[1]||"")}</span></div>
      <div class="cell"><span class="lab">Scadenza</span><div class="v">${esc(d.scad)}</div></div>
      <div class="cell r2 span4"><span class="lab">Cartella sul NAS</span><div class="v mono">${esc(q.nas)}</div></div>
      <div class="cell r2"><span class="lab">Stato</span><div class="v">${chipStato(q)}</div></div>`)}</div>
  <div class="steps" role="tablist">${PASSI.map((t,i)=>`<button class="step" role="tab" data-step="${i}" aria-selected="${S.step===i}"><span class="n">${i+1}</span>
    <span class="t">${t}</span><span class="s ${stati[i][0]}">${esc(stati[i][1])}</span></button>`).join("")}</div>
  <div class="rfqbody">${[passoDocumenti,passoDistinta,passoOfferta][S.step](q,d,c)}</div>
  <div class="navsteps"><button class="btn" data-step="${Math.max(0,S.step-1)}" ${S.step===0?"disabled":""}>← ${S.step>0?PASSI[S.step-1]:""}</button>
    <span class="dove">passo ${S.step+1} di 3 · ${PASSI[S.step]}</span>
    <button class="btn pri" data-step="${Math.min(2,S.step+1)}" ${S.step===2?"disabled":""}>${S.step<2?PASSI[S.step+1]:""} →</button></div>
  ${drawer()}${riepilogoConferma()}${pannelloContratto()}`;
}

/* ---- i sette assi del prodotto e lo stato del fascicolo (contratto A1c §1.0, R79, R88, R89) ----
   Il frontend MOSTRA gli assi che il backend calcola (ProdottoValutato, StatoFascicolo): qui il mockup li simula
   con i suoi dati. Il composto non sostituisce mai gli assi, e «pronto» non vuol dire «partito» (FaseThread). */
const ASSE_ETI={confermata:["ok","confermata"],in_attesa_di_conferma:["prop","da confermare"],assente:["bad","assente"],
  verificata:["ok","verificata"],da_verificare:["prop","da verificare"],non_verificabile:["neu","non verificabile"],conflitto:["bad","conflitto"],
  verificato:["ok","verificato"],completa:["ok","completa"],incompleta:["bad","incompleta"],non_calcolabile:["neu","non calcolabile"],
  pronto_fattibilita:["ok","pronto per la fattibilità"],non_pronto:["warn","non pronto"],da_riesaminare:["warn","da riesaminare"]};
function assiProdotto(q,pid){
  const D=DOCS[pid], st=D.step, B=bomDi(pid), chiusa=!!q.bomOk;
  const fonte=st&&!st.portale?(st.stato==="ok"?"confermata":"in_attesa_di_conferma"):"assente";
  const senzaFigli=B.nodes.length===1, ver=bomVerificata(pid);
  const nom=ver?"verificata":senzaFigli&&!chiusa?"non_verificabile":"da_verificare";
  const ger=nom==="non_verificabile"?"non_verificabile":ver&&fonte==="confermata"?"verificata":"da_verificare";
  const s1=stat(pid), libere=(D.liberi||[]).length;
  const smist=s1.daconf||libere?"da_verificare":"verificato";
  const mancaCerto=s1.mancanti.some(x=>x[2]==="manca"||x[2]==="sul portale"), perimetro=fonte==="confermata"&&ver;
  const doc=mancaCerto?"incompleta":perimetro&&s1.pronto?"completa":"non_calcolabile";
  const tutti=fonte==="confermata"&&nom==="verificata"&&ger==="verificata"&&smist==="verificato"&&doc==="completa";
  return [["target","Prodotto della RFQ","confermata","codice confermato alla creazione dell’RFQ"],
    ["fonte","Fonte strutturale (STEP)",fonte,fonte==="assente"?(st&&st.portale?"lo STEP è sul portale":"nessuno STEP"):fonte==="in_attesa_di_conferma"?"STEP proposto: va autorizzato":"STEP autorizzato come fonte"],
    ["nom","Nomenclatura della BOM",nom,nom==="non_verificabile"?"nessun figlio, né proposto né confermato":ver?"codici confermati":"codici da confermare nella Distinta"],
    ["ger","Gerarchia della BOM",ger,ger==="da_verificare"&&fonte!=="confermata"?"serve la fonte confermata":ver?"archi e quantità confermati":"archi e quantità da confermare"],
    ["smi","Smistamento dei file",smist,smist==="verificato"?"ogni file è confermato o escluso":plur(s1.daconf+libere,"file da decidere","file da decidere")],
    ["doc","Completezza documentale",doc,doc==="incompleta"?"manca un documento obbligatorio":doc==="non_calcolabile"?"il perimetro non è chiuso: PDF previsti":"ogni 2D obbligatorio confermato"],
    ["stato","Stato del prodotto",tutti?"pronto_fattibilita":"non_pronto",tutti?"può passare alla fattibilità":"un motivo per ogni asse aperto"]];
}
function treStati(q){
  const d=datiRFQ(q), pid=S.rp&&d.prodotti.includes(S.rp)?S.rp:d.prodotti[0], A=assiProdotto(q,pid);
  const pronti=d.prodotti.filter(p=>assiProdotto(q,p)[6][2]==="pronto_fattibilita").length, congelabile=pronti===d.prodotti.length;
  return `<div class="assi-box"><div class="assi-testa"><span class="lab">Assi del prodotto <span class="mono">${esc(PROD[pid].cod)}</span></span>
      <span class="sp"></span>${q.req||q.stato==="avviata"?bottoneConferma(pid):""}<span class="k3" style="font-size:var(--t-xs)">Fascicolo: ${pronti} su ${plur(d.prodotti.length,"prodotto","prodotti")} pronti · ${congelabile?C("ok","congelabile","check"):C("neu","non congelabile")}</span></div>
    <div class="assi">${A.map(([k,t,v,why],i)=>{const [c,l]=ASSE_ETI[v]; return `<div class="asse ${c}${i===6?" tot":""}" data-asse="${k}"><span class="lab">${i<6?(i+1)+" · ":""}${t}</span><b>${C(c,l)}</b><span class="k3">${esc(why)}</span></div>`;}).join("")}</div></div>`;
}
/* ---- passo 1: Documenti e NAS ---- */
function slotCard(pid,k,label,c){
  const s=getSlot(pid,k), ob=obblig(pid,k), lab=`<span class="lab">${label}</span>`;
  if(!s||s.portale) return `<div class="slot vuoto"><div class="thumb v">${I(s?"globe":"file",26)}</div><div style="min-width:0">
    <div class="tt">${lab}${s?C("acc","indicato sul portale","down"):ob?C("bad","manca","x"):C("ghost","facoltativo")}</div>
    <div class="meta">${s?`Il cliente lo indica sul portale fornitori${c.portale!=="—"?` (<span class="mono">${esc(c.portale)}</span>)`:""}: va scaricato e caricato qui.`:"Nessun file trovato nella mail né sul portale."}</div>
    <div class="acts"><button class="btn sm" data-cerca="1">${I("refresh",13)} Cerca di nuovo</button>
      <button class="btn sm pri" data-carica="${pid}|${k}">${I("upload",13)} Carica dal PC</button></div></div></div>`;
  const stc=s.stato==="ok"?C("ok","confermato","check"):s.stato==="verifica"?C("warn","da verificare","alert"):C("prop","proposto");
  return `<div class="slot ${s.stato==="verifica"?"verifica":""}"><div class="thumb">${miniDis(kindOf(k))}</div><div style="min-width:0">
    <div class="tt">${lab}${stc}</div>
    <div class="fname">${esc(s.f)}</div>
    <div class="meta">${esc(s.s)} · ${PROVL[s.prov]} · ${esc(s.da)}</div>
    <div class="why k">Perché: ${esc(s.perche)}</div>
    ${s.nota?`<div class="why" style="color:var(--warn)">${I("alert",12)} ${esc(s.nota)}</div>`:""}
    <div class="acts"><button class="btn sm" data-insp="${pid}|${k}">${I("eye",13)} Ispeziona</button>
      ${s.stato!=="ok"?`<button class="btn sm pri" data-conf="${pid}|${k}">${I("check",13)} Conferma</button>`:""}
      <button class="btn sm" data-insp="${pid}|${k}">${I("swap",13)} Cambia</button>
      <button class="btn sm ghost danger" data-rim="${pid}|${k}">${I("trash",13)} Rimuovi</button></div></div></div>`;
}
function cellSlot(pid,i,col){
  const p=DOCS[pid].parti[i]; if(!previsto(p.tipo,col)) return `<span class="k3">—</span>`;
  const k=`p:${i}:${col}`, s=getSlot(pid,k), ob=obblig(pid,k);
  if(!s||s.portale) return `<span class="cellslot">${pill(COL[col],s,ob)}
    <span style="display:flex;gap:4px"><button class="btn sm" data-cerca="1" title="Cerca di nuovo">${I("refresh",12)}</button>
    <button class="btn sm" data-carica="${pid}|${k}" title="Carica dal PC">${I("upload",12)}</button></span></span>`;
  return `<span class="cellslot">${pill(COL[col],s,ob)}<span class="fn" title="${esc(s.f)}">${esc(s.f)}</span>
    <span style="display:flex;gap:4px"><button class="btn sm" data-insp="${pid}|${k}">${I("eye",12)} Ispeziona</button>
    ${s.stato!=="ok"?`<button class="btn sm pri" data-conf="${pid}|${k}" title="Conferma">${I("check",12)}</button>`:""}</span></span>`;
}
function passoDocumenti(q,d,c){
  const pid=S.rp, P=PROD[pid], D=DOCS[pid], st=stat(d.prodotti);
  const esito=S.esito?`<div class="esito ${S.esito.ok?"ok":"no"}">${I(S.esito.ok?"check":"alert",16)}<div>${S.esito.righe.map(x=>`<div>${x}</div>`).join("")}</div></div>`:"";
  return `<div class="rfq largo">
    ${treStati(q,st)}
    <div class="toolbar"><span class="sum"><span><b>${st.pres}</b> riconosciuti</span>
        ${st.daconf?`<span><b>${st.daconf}</b> da confermare</span>`:""}${st.manca?`<span><b>${st.manca}</b> mancanti</span>`:""}${st.port?`<span><b>${st.port}</b> sul portale</span>`:""}</span>
      <span class="sp"></span>
      <button class="btn" data-cerca="1">${I("refresh",14)} Cerca di nuovo i documenti mancanti</button>
      <button class="btn" data-carica="auto">${I("upload",14)} Carica a mano</button>
      <button class="btn pri" data-confall="1" ${st.daconf-st.ver>0?"":"disabled"}>${I("check",14)} Conferma tutti i riconosciuti</button></div>
    ${esito}
    ${d.prodotti.length>1?`<div class="ptabs" role="tablist">${d.prodotti.map(p=>{const s2=stat(p);
      return `<button class="ptab" role="tab" data-rp="${p}" aria-selected="${p===pid}"><span><span class="mono">${PROD[p].cod}</span> <span class="k">${esc(PROD[p].nome)}</span>
        <div class="k3" style="font-size:var(--t-xs)">${s2.conf}/${s2.o} confermati${s2.mancanti.length?" · "+s2.mancanti.length+" da sistemare":""}</div></span>
        ${s2.pronto?C("ok","pronto","check"):C(s2.manca?"bad":"prop",s2.mancanti.length+"")}</button>`;}).join("")}</div>`:""}
    <div class="rfqgrid"><div style="display:grid;gap:var(--sp5);min-width:0">
      <div class="fs"><header>${I("cube",17)}<h3>Documenti principali · <span class="mono">${P.cod}</span></h3><span class="k">${esc(P.nome)} · ${esc(P.qta)}</span></header>
        <p class="hint">Compilati con i file che il sistema ha riconosciuto per questo prodotto. Controlla, correggi se serve, conferma.</p>
        <div class="slots">${slotCard(pid,"step","STEP strutturale · fonte della distinta",c)}${slotCard(pid,"pdf","PDF del prodotto",c)}</div></div>
      <div class="fs"><header>${I("list",17)}<h3>Sotto-parti richieste</h3><span class="sp"></span>
        <span class="k3" style="font-size:var(--t-sm)">2D obbligatorio per assiemi e particolari · commerciali senza obbligo · 3D e DXF informativi</span></header>
        ${D.parti.length?`<div class="wrap-tb" style="overflow-x:auto"><table class="tb"><thead><tr><th>Pezzo</th><th>Tipo</th><th>Qtà</th><th>3D</th><th>2D</th><th>DXF</th></tr></thead>
          <tbody>${D.parti.map((p,i)=>`<tr><td><span class="mono" style="font-weight:600">${p.cod}</span><div class="k3" style="font-size:var(--t-xs)">${esc(p.nome)}${p.sotto?` · sotto <span class="mono">${esc(p.sotto)}</span>`:""}</div>
              ${!p.conf&&!q.bomOk?`<div style="margin-top:3px">${C("prop","da confermare nella Distinta")}</div>`:""}
              ${p.cat==="minuteria"?`<div style="margin-top:3px;display:flex;gap:6px;flex-wrap:wrap;align-items:center">${p.catConf?C("ok","minuteria confermata: 2D non richiesto","check"):`${C("prop","minuteria proposta")}<button class="btn sm" data-mincf="${pid}|${i}" title="${esc(p.catPerche||"")}">${I("check",12)} Conferma minuteria</button>`}</div>`:""}</td>
            <td><span class="tipo ${p.tipo}">${nomeTipo(p.tipo)}</span></td><td class="mono">×${p.qta}</td>
            <td>${cellSlot(pid,i,"d3")}</td><td>${cellSlot(pid,i,"d2")}</td><td>${cellSlot(pid,i,"dxf")}</td></tr>`).join("")}</tbody></table></div>`
          :`<p class="hint">Nessuna sotto-parte rilevata: lo STEP non ha figli${D.step?"":" (o non c’è ancora)"}. Se il prodotto ha componenti, la struttura si conferma nella Distinta.</p>`}</div>
      <div class="fs"><header>${I("folder",17)}<h3>File senza destinazione</h3><span class="k3" style="font-size:var(--t-sm)">distinti da quelli con una proposta da confermare (qui sopra) e da quelli in analisi</span><span class="sp"></span><span class="k3" style="font-size:var(--t-sm)">${D.liberi.length}</span></header>
        ${D.liberi.length?D.liberi.map(([f,cosa,tag,an],i)=>`<div class="pick" style="grid-template-columns:auto minmax(0,1fr) auto">
          <span class="ext ${f.split(".").pop().toLowerCase()}">${f.split(".").pop().toUpperCase()}</span>
          <span style="min-width:0"><span class="fn">${esc(f)}</span> ${an==="in_corso"?C("neu","analisi in corso"):an==="errore"?C("bad","analisi fallita","x"):C("warn","nessuna destinazione")} ${tag?C("ghost",tag):""}<div class="k3" style="font-size:var(--t-sm)">${esc(cosa)}</div></span>
          <span style="display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end"><button class="btn sm" data-libass="${i}">${I("link",13)} Associa a…</button>
            <button class="btn sm ghost" data-libvia="${i}">Metti da parte</button></span></div>`).join(""):`<p class="hint">Nessuno: ogni file della richiesta è associato a un pezzo.</p>`}</div>
    </div>
    <div class="side">
      <div class="fs nasbox"><header>${I("folder",17)}<h3>Copia sul NAS</h3></header>
        <div class="go" style="margin:0"><span class="mono" style="font-size:var(--t-xs);overflow-wrap:anywhere">${esc(q.nas)}</span></div>
        ${q.nasOk?`<div class="esito ok">${I("check",16)}<div><b>Copiati sul NAS.</b> Hash verificati, file in ELENCO DISEGNI.</div></div>`
          :st.pronto?`<p class="hint">Tutti i documenti obbligatori sono confermati.</p>`
          :`<p class="hint">Si copia quando lo STEP strutturale e tutti i PDF obbligatori sono confermati. Manca:</p>
            <ul class="miss">${st.mancanti.slice(0,8).map(([p,k,why])=>`<li>${C(why==="manca"?"bad":why==="sul portale"?"acc":"prop",why)}<span><span class="mono">${PROD[p].cod}</span> · ${esc(etichetta(p,k))}</span></li>`).join("")}
            ${st.mancanti.length>8?`<li class="k3">e altri ${st.mancanti.length-8}</li>`:""}</ul>`}
        <button class="btn pri xl" data-nas="1" ${st.pronto&&!q.nasOk?"":"disabled"}>${I("check",18)} Conferma e copia sul NAS</button></div>
      <div class="fs"><header>${I("mail",17)}<h3>La richiesta</h3></header>
        <dl class="dati"><dt>Cliente</dt><dd>${esc(c.nome)}</dd><dt>Buyer</dt><dd>${esc(d.buyer)}</dd><dt>Ricevuta</dt><dd>${esc(d.aperta)}</dd>
          <dt>Prodotti</dt><dd class="mono" style="font-size:var(--t-sm)">${d.prodotti.map(p=>PROD[p].cod).join(", ")}</dd></dl>
        ${d.req?`${mail(msgDi(d.req.id)).map(m=>`<div class="itm"><span class="av" style="background:${m.dir==="out"?"#4f5a63":c.col}">${ini(m.chi)}</span>
            <span class="g"><b style="font-weight:500">${esc(m.chi)}</b> <span class="k3" style="font-size:var(--t-xs)">${esc(m.d)} ${m.t}${m.files?" · "+plur(m.files.length,"allegato","allegati"):""}</span>
            <div class="k" style="font-size:var(--t-sm);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${esc(testo(m.txt))}</div></span></div>`).join("")}
          <button class="btn" data-req="${d.req.id}">${I("inbox",14)} Apri la conversazione</button>`
          :`<p class="hint">RFQ chiusa: la conversazione è in archivio.</p>`}</div>
    </div></div></div>`;
}

/* ---- pannello Ispeziona ---- */
function drawer(){
  if(!S.insp) return "";
  const q=rf(S.rfq), d=datiRFQ(q);
  if(S.insp.mode==="associa"){
    const L=DOCS[S.rp].liberi[S.insp.lib]; if(!L) return "";
    const target=d.prodotti.flatMap(p=>chiavi(p).map(k=>[p,k]));
    return `<div class="backdrop" data-close="1"></div><aside class="drawer" role="dialog" aria-label="Associa il file">
      <header><div style="min-width:0"><span class="lab">Associa a un pezzo</span><h3 class="mono">${esc(L[0])}</h3></div><span class="sp"></span>
        <button class="btn sm ghost" data-close="1" aria-label="Chiudi">${I("x",15)}</button></header>
      <div class="dbody"><p class="hint" style="margin:0">Scegli il ruolo del file. Se il posto è occupato, il file che c’era torna fra quelli non associati.</p>
        ${target.map(([p,k])=>{const s=getSlot(p,k);return `<div class="alt"><span style="min-width:0"><span class="mono">${PROD[p].cod}</span> · ${esc(etichetta(p,k))}
          <div class="k3" style="font-size:var(--t-xs)">${s&&!s.portale?"ora: "+esc(s.f):obblig(p,k)?"vuoto · obbligatorio":"vuoto · facoltativo"}</div></span>
          <button class="btn sm" data-assoc="${p}|${k}">Associa qui</button></div>`;}).join("")}</div>
      <footer><button class="btn" data-close="1">Chiudi</button></footer></aside>`;
  }
  const {pid,k}=S.insp, s=getSlot(pid,k); if(!s||s.portale) return "";
  const altri=[...(s.alt||[]), ...DOCS[pid].liberi.filter(l=>/\.(pdf|stp|step|igs|dxf)$/i.test(l[0])).map(l=>[l[0],"",l[1]])];
  return `<div class="backdrop" data-close="1"></div><aside class="drawer" role="dialog" aria-label="Ispeziona il documento">
    <header><div style="min-width:0"><span class="lab">${esc(etichetta(pid,k))} · ${PROD[pid].cod}</span><h3 class="mono">${esc(s.f)}</h3></div><span class="sp"></span>
      <button class="btn sm ghost" data-close="1" aria-label="Chiudi">${I("x",15)}</button></header>
    <div class="dbody">
      <div class="bigthumb">${miniDis(kindOf(k))}</div>
      <dl class="dati">
        <dt>Stato</dt><dd>${s.stato==="ok"?C("ok","confermato","check"):s.stato==="verifica"?C("warn","da verificare","alert"):C("prop","proposto dal sistema")}</dd>
        <dt>Provenienza</dt><dd>${PROVL[s.prov]} · ${esc(s.da)}</dd>
        <dt>Perché qui</dt><dd>${esc(s.perche)}</dd>
        ${s.rev?`<dt>Revisione</dt><dd class="mono">${esc(s.rev)}</dd>`:""}
        <dt>Dimensione</dt><dd>${esc(s.s)}</dd>
        <dt>Impronta</dt><dd class="mono" style="font-size:var(--t-sm)">sha256 ${impronta(s.f)}</dd></dl>
      ${s.nota?`<div class="esito no">${I("alert",16)}<div>${esc(s.nota)}</div></div>`:""}
      <div><span class="lab">Altri file candidati</span>
        <div style="display:grid;gap:6px;margin-top:8px">${altri.length?altri.map(([f,sz,desc],i)=>`<div class="alt"><span style="min-width:0"><span class="fn">${esc(f)}</span>
          <div class="k3" style="font-size:var(--t-xs)">${esc(desc)}${sz?" · "+esc(sz):""}</div></span>
          <button class="btn sm" data-usa="${i}">${I("swap",13)} Usa questo</button></div>`).join("")
          :`<p class="hint" style="margin:0">Nessun altro candidato nella richiesta. Puoi caricare un file dal PC.</p>`}</div></div></div>
    <footer>${s.stato!=="ok"?`<button class="btn pri" data-conf="${pid}|${k}">${I("check",14)} Conferma</button>`:""}
      <button class="btn" data-carica="${pid}|${k}">${I("upload",14)} Carica un altro file</button>
      <button class="btn danger" data-rim="${pid}|${k}">${I("trash",14)} Rimuovi</button>
      <button class="btn ghost" data-close="1">Chiudi</button></footer></aside>`;
}

/* ---- passo 2: la Distinta è in 45_distinta.js · passo 3: anteprima ---- */
function passoOfferta(q,d,c){
  return `<div class="rfq"><div class="fs" style="background:var(--surf2)"><header>${I("file",17)}<h3>Offerta</h3><span class="sp"></span>${chipStato(q)}</header>
    ${q.stato!=="avviata"||q.nota?`<p style="margin:0">${esc(q.nota||"")}</p>`:""}
    <p class="hint">Il preventivo si genera dalla Distinta confermata: costi per componente, attrezzature, margine. Arriva dopo la Distinta.</p></div></div>`;
}
