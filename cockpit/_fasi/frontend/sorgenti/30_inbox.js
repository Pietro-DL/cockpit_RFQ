
/* ================= stato e utilità ================= */
const S = {tab:"inbox", sez:"clienti", cli:"acme", req:"r-ar1", pf:null, orf:"s1", altra:"a1", tz:"f3", tzc:0,
  rfq:null, step:0, rp:null, insp:null, esito:null, creato:null, upl:null,
  stato:"avviata", q:"", fcli:"", setSez:"clienti", setCli:"acme", nuovo:false, nuovaFrom:null, ok:null, setOk:null};
const $ = id=>document.getElementById(id);
const esc = s=>String(s??"").replace(/[<>&"]/g,c=>({"<":"&lt;",">":"&gt;","&":"&amp;",'"':"&quot;"}[c]));
const testo = h=>String(h||"").replace(/<[^>]+>/g," ").replace(/&gt;/g,">").replace(/&lt;/g,"<").replace(/&amp;/g,"&").replace(/\s+/g," ").trim();
const cl = id=>CLIENTI.find(c=>c.id===id);
const rq = id=>RICHIESTE.find(r=>r.id===id);
const rf = id=>RFQ.find(q=>q.id===id);
const ini = s=>String(s).split(" ").filter(Boolean).map(x=>x[0]).join("").slice(0,2).toUpperCase();
const attivi = ()=>CLIENTI.filter(c=>c.attivo);
const reqDi = cid=>RICHIESTE.filter(r=>r.cli===cid);
const msgDi = (rid,pid)=>MESSAGGI.filter(m=>m.req===rid&&(!pid||(m.prod||[]).includes(pid)));
const mail = ms=>ms.filter(m=>m.kind!=="xref");
const rfqDi = r=>r&&r.rfq?rf(r.rfq):null;
const plur = (n,s,p)=>n+" "+(n===1?s:p);
function datiRFQ(q){
  if(q.req){const r=rq(q.req);return {cli:r.cli,titolo:r.titolo,buyer:r.buyer,aperta:r.aperta+(r.ora?" "+r.ora:""),scad:r.scad,prodotti:r.prodotti,req:r};}
  return {cli:q.cli,titolo:q.titolo,buyer:q.buyer,aperta:q.aperta,scad:q.scad,prodotti:q.prodotti,req:null};
}
function chipStato(q){
  /* la fase del backend (fase_catalogo); il gruppo della gestione è solo presentazione */
  const f=q.fase, et=FASI[f][1];
  if(q.statoThread==="CHIUSA") return C("ok",et+" · chiusa","check");
  if(f==="PERSA"||f==="RESPINTA"||f==="SCADUTA") return C("bad",et,"x");
  if(q.stato==="accettata") return C("ok",et,"check");
  if(q.stato==="produzione") return C("neu",et,"factory");
  return C(f==="ATTESA_DISEGNI"?"warn":"acc",et);
}

/* ================= documenti: regole e conteggi (contratto A1c: §1.0, §1.6; E1R R103 C, K-01 A; T-E1-18) =================
   I sette assi per prodotto stanno in 40_rfq.js (assiProdotto). Qui il fabbisogno dei documenti:
   - il finito chiede lo STEP strutturale (asse della fonte) e il 2D;
   - ogni componente del perimetro chiede il 2D, commerciali compresi (R103 C: «la sola classificazione
     commerciale non costituisce un'esenzione»);
   - l'unica esenzione è la MINUTERIA CONFERMATA (T-E1-18): una minuteria solo proposta non toglie niente.
   La categoria (fabbricato · commerciale · minuteria) è una dimensione distinta dal tipo (E1 §6, Classificazione).
   3D e DXF dei pezzi sono informativi: si mostrano, non bloccano. */
const OBBL = {sottoass:{d3:false,d2:true,dxf:false}, sciolto:{d3:false,d2:true,dxf:false}, comm:{d3:false,d2:true,dxf:false}};
const minuteriaConfermata = p=>!!p&&p.cat==="minuteria"&&!!p.catConf;
const richiesto = (tipo,col,p)=>!!(OBBL[tipo]||{})[col]&&!(col==="d2"&&minuteriaConfermata(p));
const COL = {d3:"3D",d2:"2D",dxf:"DXF"};
const previsto = (tipo,c)=>!(tipo==="comm"&&c==="dxf");
const nomeTipo = t=>({finito:"prodotto",sottoass:"assieme",sciolto:"particolare",comm:"commerciale"}[t]||t);
function getSlot(pid,k){const D=DOCS[pid]; if(k==="step"||k==="pdf")return D[k]; const[,i,c]=k.split(":"); return D.parti[+i][c];}
function setSlot(pid,k,v){const D=DOCS[pid]; if(k==="step"||k==="pdf"){D[k]=v;return;} const[,i,c]=k.split(":"); D.parti[+i][c]=v;}
function obblig(pid,k){if(k==="step"||k==="pdf")return true; const[,i,c]=k.split(":"); const p=DOCS[pid].parti[+i]; return richiesto(p.tipo,c,p);}
function etichetta(pid,k){
  if(k==="step") return "STEP strutturale"; if(k==="pdf") return "PDF del prodotto";
  const[,i,c]=k.split(":"); const p=DOCS[pid].parti[+i]; return `${COL[c]} di ${p.cod}`;
}
function chiavi(pid){const D=DOCS[pid]; const ks=["step","pdf"];
  D.parti.forEach((p,i)=>["d3","d2","dxf"].forEach(c=>{if(previsto(p.tipo,c))ks.push(`p:${i}:${c}`);})); return ks;}
function stat(pids){
  const r={o:0,pres:0,conf:0,daconf:0,ver:0,manca:0,port:0,mancanti:[],pdfO:0,pdfConf:0,fonteOk:0,fonteTot:0};
  for(const pid of [].concat(pids)){ const st=DOCS[pid].step; r.fonteTot++; if(st&&!st.portale&&st.stato==="ok") r.fonteOk++; }
  for(const pid of [].concat(pids)) for(const k of chiavi(pid)){
    if((k==="pdf"||k.endsWith(":d2"))&&obblig(pid,k)){ r.pdfO++; const s0=getSlot(pid,k); if(s0&&!s0.portale&&s0.stato==="ok") r.pdfConf++; }
    const s=getSlot(pid,k), ob=obblig(pid,k);
    if(ob) r.o++;
    if(!s){ if(ob){r.manca++; r.mancanti.push([pid,k,"manca"]);} continue; }
    if(s.portale){ r.port++; if(ob) r.mancanti.push([pid,k,"sul portale"]); continue; }
    r.pres++;
    if(s.stato==="ok"){ if(ob) r.conf++; }
    else { r.daconf++; if(s.stato==="verifica") r.ver++; if(ob) r.mancanti.push([pid,k,s.stato==="verifica"?"da verificare":"da confermare"]); }
  }
  r.pronto = r.mancanti.length===0;
  return r;
}
const SK = {mail:["has","Mail"],"mail?":["prop","Mail ✓?"],"mail!":["warn","Mail ⚠"],port:["has","Portale"],"port?":["prop","Portale ✓?"],
  "port!":["warn","Portale ⚠"],car:["has","Caricato"],"car?":["prop","Caricato ✓?"],"car!":["warn","Caricato ⚠"],
  portale:["port","Portale ⇩"],no:["no","manca"],fac:["nr","○ facoltativo"],na:["nr","—"]};
function skey(s,ob){if(!s)return ob?"no":"fac"; if(s.portale)return "portale"; return s.prov+(s.stato==="ok"?"":s.stato==="verifica"?"!":"?");}
const pill = (t,s,ob)=>{const[c,l]=SK[skey(s,ob)]; return `<span class="doc ${c}"><b>${t}</b>${l}</span>`;};

/* ================= testata ================= */
function renderTabs(){
  const t=[["inbox","inbox","Inbox"],["gest","file","Richieste RFQ"],["set","users","Clienti e mittenti"]];
  const cur = S.tab==="rfq"?"gest":S.tab==="nuovareq"?"inbox":S.tab;
  $("tabs").innerHTML=t.map(([id,ic,l])=>`<button data-tab="${id}" aria-current="${cur===id}">${I(ic,17)}${l}</button>`).join("");
}

/* ================= INBOX ================= */
const vuoto=(ic,t,p)=>`<div class="empty"><span class="ib">${I(ic,22)}</span><h3>${esc(t)}</h3><p>${esc(p)}</p></div>`;
function cart(cells){return `<section class="cart"><div class="grid">${cells}</div></section>`;}
function rail(){
  const nb=(id,ic,l,n,cur)=>`<button class="navb" data-sez="${id}" aria-current="${cur}">${I(ic,17)}<span class="lbl">${l}</span><span class="cnt">${n}</span></button>`;
  return `<div class="search"><span class="ic">${I("search",15)}</span><input placeholder="Cerca codice, cliente, persona" aria-label="Cerca"></div>
  <div class="railg"><span class="lab">Caselle</span></div>
  <div class="nav">${nb("smistare","tray","Da smistare",SMISTARE.length,S.sez==="smistare")}
    ${nb("altra","mail","Senza prodotto",ALTRA.length,S.sez==="altra")}
    ${nb("terzisti","factory","Terzisti",TERZISTI.length,S.sez==="terzisti")}</div>
  <hr class="hr" style="margin:12px">
  <div class="railg"><span class="lab">Clienti autorizzati</span></div>
  <div class="nav">${attivi().map(c=>`<button class="navb" data-cli="${c.id}" aria-current="${S.sez==="clienti"&&S.cli===c.id}">
    <span class="av" style="background:${c.col}">${c.sigla}</span><span class="lbl">${esc(c.nome)}</span>
    <span class="cnt" title="richieste aperte">${reqDi(c.id).length}</span></button>`).join("")}</div>
  <div class="railg" style="display:grid;gap:4px">
    <button class="btn sm ghost" data-tab="set" data-setsez="nascosti">${I("eyeoff",14)} ${plur(NASCOSTI.length,"messaggio nascosto","messaggi nascosti")}</button>
    <button class="btn sm ghost" data-tab="set" data-setsez="clienti">${I("users",14)} Gestisci clienti e mittenti</button></div>
  <div style="height:16px"></div>`;
}
function rowReq(r){
  const q=rfqDi(r), ms=mail(msgDi(r.id)), last=ms[ms.length-1];
  const chips=r.prodotti.map(pid=>C("acc",PROD[pid].cod,"cube")).join("")+(q?C("ok","RFQ "+q.num,"file"):C("ghost","senza RFQ"));
  return `<button class="row" data-req="${r.id}" aria-current="${S.sez==="clienti"&&S.req===r.id}">
    <span class="av lg" style="background:${cl(r.cli).col}">${cl(r.cli).sigla}</span>
    <span><span class="r1"><span class="nm">${esc(r.titolo)}</span><span class="tm">${last?esc(last.d)+" "+last.t:""}</span></span>
    <span class="snip">${last?`<b>${esc(last.chi)}:</b> ${esc(testo(last.txt))}`:""}</span>
    <span class="rchips">${chips}</span><span class="snip" style="-webkit-line-clamp:1">${plur(ms.length,"messaggio","messaggi")} · ${plur(r.prodotti.length,"prodotto","prodotti")}</span></span></button>`;
}
function rowS(attr,val,cur,av,col,nm,tm,snip,chips){
  return `<button class="row" ${attr}="${val}" aria-current="${cur}">
    <span class="av lg" style="background:${col}">${av}</span>
    <span><span class="r1"><span class="nm">${nm}</span><span class="tm">${tm}</span></span>
    <span class="snip">${snip}</span>${chips?`<span class="rchips">${chips}</span>`:""}</span></button>`;
}
function okStrip(){
  if(!S.ok) return "";
  return `<div class="strip okk" style="margin:12px 16px">${I("check",15)}<span>${esc(S.ok.txt)}</span>
    ${S.ok.req&&S.sez!=="clienti"?`<button class="btn sm" data-req="${S.ok.req}">Apri la richiesta ${I("chev",13)}</button>`:""}</div>`;
}
function lista(){
  if(S.sez==="clienti"){
    const c=cl(S.cli), rs=reqDi(c.id);
    return `<div class="lhead"><h2>${esc(c.nome)}</h2><div class="meta">
      <span class="sett" style="background:${SETT[c.sett][1]}"></span>${SETT[c.sett][0]} · ${esc(c.sede)} · ${plur(c.mittenti.filter(x=>x.on).length,"mittente autorizzato","mittenti autorizzati")}</div></div>
      <div class="sub"><span class="lab">${plur(rs.length,"richiesta","richieste")}</span><span style="flex:1"></span><span class="n">una conversazione per richiesta</span></div>
      ${rs.length?rs.map(rowReq).join(""):vuoto("tray","Nessuna richiesta aperta","Quando un mittente autorizzato manda una richiesta, la trovi in «Da smistare» e da lì nasce la conversazione.")}`;
  }
  if(S.sez==="smistare") return `<div class="lhead"><h2>Da smistare</h2><div class="meta">Messaggi di mittenti autorizzati che non appartengono ancora a una richiesta · nessun aggancio automatico</div></div>
    ${okStrip()}
    ${SMISTARE.length?SMISTARE.map(m=>rowS("data-orf",m.id,S.orf===m.id,ini(m.chi),cl(m.cli)?.col||"#8d8d86",esc(m.chi),m.t,
      `<b>${esc(m.sub)}</b> — ${esc(m.snip)}`,C("neu",cl(m.cli)?.nome||"")+m.stato)).join(""):vuoto("check","Niente da smistare","Tutti i messaggi sono in una richiesta.")}`;
  if(S.sez==="altra"){
    const g=[...new Set(ALTRA.map(m=>m.cat))];
    return `<div class="lhead"><h2>Senza prodotto</h2><div class="meta">Posta che non è una richiesta d’offerta</div></div>
    ${g.map(cat=>`<div class="sub"><span class="lab">${esc(cat)}</span></div>`+ALTRA.filter(m=>m.cat===cat).map(m=>
      rowS("data-altra",m.id,S.altra===m.id,ini(m.chi),m.cli?cl(m.cli).col:"#8d8d86",esc(m.chi),m.t,`<b>${esc(m.sub)}</b> — ${esc(m.snip)}`,C("neu",m.az))).join("")).join("")}`;
  }
  return `<div class="lhead"><h2>Terzisti</h2><div class="meta">Una chat per lavorazione affidata, collegata alla richiesta</div></div>
    ${TERZISTI.map(f=>`<div class="sub"><span class="lab">${esc(f.nome)}</span><span class="n">${esc(f.lav)}</span></div>`+
      (f.chat.length?f.chat.map((ch,i)=>{const p=PROD[ch.pid]; return rowS("data-tz",`${f.id}:${i}`,S.tz===f.id&&S.tzc===i,f.sigla,"#4f5a63",
        `<span class="mono">${ch.cod||p.cod}</span> · ${esc(ch.nome||p.nome)}`,ch.ultimo,`${esc(cl(rq(ch.req).cli).nome)} — ${esc(f.lav)}`,ch.stato);}).join("")
      :`<div style="padding:10px 16px 14px" class="why k3">Nessuna lavorazione in corso.</div>`)).join("")}`;
}

/* ---- messaggi ---- */
function bolla(m){
  const files=m.files?`<div class="files">${m.files.map(f=>`<div class="file"><span class="ext ${f.e}">${f.e.toUpperCase()}</span>
    <span><span class="fn">${esc(f.n)}</span><br><span class="fs">${f.s}</span></span><span class="k3">${I("clip",14)}</span></div>`).join("")}</div>`:"";
  return `<div class="msg ${m.dir}">
    ${m.catena?`<span class="catena">${I("link",12)}${esc(m.catena)}</span>`:""}
    <div class="who"><b>${esc(m.chi)}</b><span class="rl">${esc(m.r)}</span>·<span class="mono rl">${m.t}</span></div>
    <div class="bub">${m.sub&&m.catena?`<p><b>${esc(m.sub)}</b></p>`:""}${m.txt}${files}</div>
    ${m.meta?`<div class="mline">${I("arrow",14)}<span>${esc(m.meta)}</span></div>`:""}</div>`;
}
function xref(m){
  return `<div class="xref"><span class="ib">${I("factory",16)}</span>
    <span><b>Quotazione terzista · ${esc(m.tzn)}</b><span class="m">${esc(m.lav)} · ${esc(m.quando)}</span></span>
    <span style="display:flex;gap:8px;align-items:center">${m.esito}<button class="btn sm" data-tz="${m.tz}:0">Apri${I("chev",13)}</button></span></div>`;
}
function flusso(ms){
  let last="",out="";
  for(const m of ms){ if(m.d!==last){out+=`<div class="day">${esc(m.d)}</div>`;last=m.d;} out+=m.kind==="xref"?xref(m):bolla(m); }
  return out;
}

/* ---- il blocco RFQ: sempre sotto il cartiglio, crea o riapre ---- */
function rfqBlock(r){
  const q=rfqDi(r), st=stat(r.prodotti);
  if(!q) return `<div class="rfqblock nuova"><span class="ib">${I("file",22)}</span>
    <div><h3>Questa richiesta non ha ancora un’RFQ</h3>
      <p>${plur(r.prodotti.length,"prodotto","prodotti")} · ${plur(st.pres,"documento riconosciuto","documenti riconosciuti")}${st.manca?` · ${st.manca} mancanti`:""}${st.port?` · ${st.port} sul portale`:""} · la cartella sul NAS nasce con l’RFQ</p></div>
    <div class="acts"><button class="btn pri xl" data-crearfq="${r.id}">${I("plus",18)} Crea l’RFQ per questa richiesta</button></div></div>`;
  return `<div class="rfqblock"><span class="ib">${I("file",22)}</span>
    <div><h3><span class="num">RFQ ${esc(q.num)}</span>${chipStato(q)}</h3>
      <p>Fase ${FASI[q.fase][1]} (${FASI[q.fase][2]}) · STEP confermati ${st.fonteOk}/${st.fonteTot} · PDF confermati ${st.pdfConf}/${st.pdfO} · scadenza ${esc(r.scad)} · ${esc(q.resp)}</p></div>
    <div class="acts"><button class="btn pri xl" data-apririfq="${q.id}">${I("arrow",18)} Apri l’RFQ</button></div></div>`;
}
function pfilter(r){
  if(r.prodotti.length<2) return "";
  return `<div class="pfilter"><span class="lab">${I("split",13)} Guarda un prodotto alla volta</span>
    <button class="pf" data-pf="" aria-pressed="${!S.pf}">Tutti i prodotti <span class="n">${mail(msgDi(r.id)).length}</span></button>
    ${r.prodotti.map(pid=>`<button class="pf" data-pf="${pid}" aria-pressed="${S.pf===pid}"><span class="mono">${PROD[pid].cod}</span> <span class="n">${mail(msgDi(r.id,pid)).length}</span></button>`).join("")}</div>`;
}
function conversazione(){
  if(S.sez==="smistare") return convSmistare();
  if(S.sez==="altra") return convAltra();
  if(S.sez==="terzisti") return convTerzista();
  const r=rq(S.req); if(!r||r.cli!==S.cli) return vuoto("tray","Nessuna richiesta selezionata","Scegli una richiesta dall’elenco.");
  const c=cl(r.cli);
  return `<div class="chead"><div class="bc"><span class="av" style="background:${c.col}">${c.sigla}</span><b>${esc(c.nome)}</b>${I("chev",13)}richiesta</div>
    ${S.ok&&S.ok.req===r.id?`<div class="strip okk" style="margin:0 0 12px">${I("check",15)}<span>${esc(S.ok.txt)}</span></div>`:""}
    ${cart(`<div class="cell"><span class="lab">Richiesta</span><h1 style="font-family:var(--fd);color:var(--ink);font-size:1.1875rem">${esc(r.titolo)}</h1>
        <span class="nome">${esc(r.buyer)} · ${esc(c.nome)}</span></div>
      <div class="cell"><span class="lab">Ricevuta</span><div class="v">${esc(r.aperta)}</div><span class="k3" style="font-size:var(--t-xs)">${esc(r.ora||"")}</span></div>
      <div class="cell"><span class="lab">Prodotti</span><div class="v">${r.prodotti.length}</div><span class="k3 mono" style="font-size:var(--t-xs)">${r.prodotti.map(p=>PROD[p].cod).join(" · ")}</span></div>
      <div class="cell"><span class="lab">Messaggi</span><div class="v">${mail(msgDi(r.id)).length}</div></div>
      <div class="cell"><span class="lab">Scadenza</span><div class="v">${esc(r.scad)}</div>${r.sla!=="ok"?C(r.sla==="risk"?"bad":"warn","vicina","clock"):""}</div>`)}
    ${rfqBlock(r)}${pfilter(r)}</div>
    <div class="stream">${flusso(msgDi(r.id,S.pf))}</div>`;
}
function convSmistare(){
  const m=SMISTARE.find(x=>x.id===S.orf)||SMISTARE[0];
  if(!m) return vuoto("check","Niente da smistare","Tutti i messaggi sono in una richiesta.");
  const c=cl(m.cli);
  return `<div class="chead"><div class="bc">${I("tray",14)}<b>Da smistare</b>${I("chev",13)}${esc(c?.nome||"")}</div>
    ${cart(`<div class="cell"><span class="lab">Messaggio</span><h1 style="font-family:var(--fd);color:var(--ink);font-size:1.1875rem">${esc(m.sub)}</h1>
      <span class="nome">${esc(m.chi)} · <span class="mono">${esc(m.em)}</span></span></div>
      <div class="cell"><span class="lab">Cliente</span><div class="v">${esc(c?.nome||"—")}</div>${C("ok","autorizzato","shield")}</div>
      <div class="cell"><span class="lab">Arrivato</span><div class="v">${m.t}</div></div>
      <div class="cell span2"><span class="lab">Proposta del sistema</span><div class="v">${m.stato}</div></div>`)}</div>
    <div class="stream">${bolla({dir:"in",chi:m.chi,r:c?.nome||"",t:m.t,txt:m.txt,files:m.files})}
    <div class="card"><span class="lab">${I("link",13)} Dove va questo messaggio</span>
      ${m.cands.map(k=>`<div class="cand"><div><div>${k.t}</div><div class="src">${k.src.join("")}</div><div class="why">${esc(k.why)}</div></div><div>${k.st}</div></div>`).join("")}
      <div class="acts">${m.nuovo
        ?`<button class="btn pri xl" data-nuovareq="${m.id}">${I("plus",18)} Crea la richiesta</button>`
        :`<button class="btn pri xl" data-aggancia="${m.id}">${I("check",18)} Conferma l’aggancio</button>`}
        <button class="btn ghost" data-nonric="${m.id}">Non è una richiesta</button></div></div></div>`;
}
function convAltra(){
  const m=ALTRA.find(x=>x.id===S.altra)||ALTRA[0];
  return `<div class="chead"><div class="bc">${I("mail",14)}<b>Senza prodotto</b>${I("chev",13)}${esc(m.cat)}</div>
    ${cart(`<div class="cell"><span class="lab">Messaggio</span><h1 style="font-family:var(--fd);color:var(--ink);font-size:1.1875rem">${esc(m.sub)}</h1>
      <span class="nome">${esc(m.chi)} · <span class="mono">${esc(m.em)}</span></span></div>
      <div class="cell"><span class="lab">Provenienza</span><div class="v">${esc(m.az)}</div></div>
      <div class="cell"><span class="lab">Arrivato</span><div class="v">${m.t}</div></div>
      <div class="cell span2"><span class="lab">Categoria</span><div class="v">${esc(m.cat)}</div></div>`)}</div>
    <div class="stream">${bolla({dir:"in",chi:m.chi,r:m.cat,t:m.t,txt:m.txt,files:m.files})}
    <div class="card"><span class="lab">${I("eyeoff",13)} Fuori dalle richieste</span>
      <div class="why">Non genera proposte di prodotto e non entra nelle conversazioni delle richieste.</div>
      <div class="acts">${m.azioni.map((a,i)=>`<button class="btn${i===0?" pri":""}">${esc(a)}</button>`).join("")}</div></div></div>`;
}
function convTerzista(){
  const f=TERZISTI.find(t=>t.id===S.tz), ch=f.chat[S.tzc];
  if(!ch) return vuoto("factory",f.nome,"Nessuna lavorazione affidata in questo momento.");
  const r=rq(ch.req), c=cl(r.cli), p=PROD[ch.pid];
  return `<div class="chead"><div class="bc">${I("factory",14)}<b>Terzisti</b>${I("chev",13)}${esc(f.nome)}</div>
    ${cart(`<div class="cell"><span class="lab">Lavorazione per</span><h1>${esc(ch.cod||p.cod)}</h1><span class="nome">${esc(ch.nome||p.nome)}</span></div>
      <div class="cell"><span class="lab">Terzista</span><div class="v">${esc(f.nome)}</div><span class="k3" style="font-size:var(--t-xs)">${esc(f.lav)}</span></div>
      <div class="cell"><span class="lab">Cliente</span><div class="v">${esc(c.nome)}</div></div>
      <div class="cell span2"><span class="lab">Esito</span><div class="v">${ch.stato}</div></div>`)}
    <div class="strip terzisti">${I("link",15)}<b>Per la richiesta</b> ${esc(r.titolo)}
      <button class="btn sm" data-req="${r.id}" data-pfset="${ch.pid}">Apri la richiesta ${I("chev",13)}</button></div></div>
    <div class="stream">${flusso(ch.msg)}</div>`;
}

/* ---- riquadro destro ---- */
function dati(){
  if(S.sez==="smistare"||S.sez==="altra"){
    const m = S.sez==="smistare"?(SMISTARE.find(x=>x.id===S.orf)||SMISTARE[0]):(ALTRA.find(x=>x.id===S.altra)||ALTRA[0]);
    if(!m) return "";
    const c = m.cli?cl(m.cli):null;
    return `<div class="sec"><h3 class="lab">${I("shield",13)} Mittente</h3><dl class="dati">
      <dt>Nome</dt><dd>${esc(m.chi)}</dd><dt>Indirizzo</dt><dd class="mono" style="font-size:var(--t-sm)">${esc(m.em)}</dd>
      <dt>Stato</dt><dd>${c?C("ok","autorizzato","check"):C("neu","fuori anagrafica")}</dd></dl></div>
      ${c?`<div class="sec"><h3 class="lab">${I("building",13)} Cliente</h3><dl class="dati">
        <dt>Ragione sociale</dt><dd>${esc(c.nome)}</dd><dt>Settore</dt><dd>${SETT[c.sett][0]}</dd>
        <dt>Richieste aperte</dt><dd>${reqDi(c.id).length}</dd></dl>
        <div class="acts"><button class="btn sm" data-tab="set" data-setcli="${c.id}">${I("users",13)} Scheda del cliente</button></div></div>`:""}`;
  }
  if(S.sez==="terzisti"){
    const f=TERZISTI.find(t=>t.id===S.tz);
    return `<div class="sec"><h3 class="lab">${I("factory",13)} Terzista</h3><dl class="dati">
      <dt>Ragione sociale</dt><dd>${esc(f.nome)}</dd><dt>Lavorazione</dt><dd>${esc(f.lav)}</dd><dt>Chat aperte</dt><dd>${f.chat.length}</dd></dl></div>`;
  }
  const r=rq(S.req); if(!r||r.cli!==S.cli) return "";
  const c=cl(r.cli), q=rfqDi(r);
  const pers=[...new Set(mail(msgDi(r.id)).filter(m=>m.dir==="in").map(m=>m.chi))];
  const tzs=msgDi(r.id).filter(m=>m.kind==="xref");
  return `<div class="sec"><h3 class="lab">${I("file",13)} RFQ della richiesta</h3>
    <div class="rfqmini">${q?`<div><span class="mono" style="font-weight:600;color:var(--acc)">${esc(q.num)}</span> ${chipStato(q)}
        <div class="k3" style="font-size:var(--t-sm)">fase ${FASI[q.fase][1]} · ${FASI[q.fase][2]}</div></div>
      <button class="btn pri" data-apririfq="${q.id}">${I("arrow",14)} Apri l’RFQ</button>`
      :`<div class="k" style="font-size:var(--t-sm)">Non ancora creata.</div>
      <button class="btn pri" data-crearfq="${r.id}">${I("plus",14)} Crea l’RFQ</button>`}</div></div>
  <div class="sec"><h3 class="lab">${I("cube",13)} Prodotti della richiesta</h3>
    ${r.prodotti.map(pid=>{const p=PROD[pid], D=DOCS[pid], st=stat(pid);
      return `<button class="prow" data-pf="${r.prodotti.length>1?pid:""}" aria-pressed="${S.pf===pid}">
        <span style="min-width:0"><span class="mono" style="font-weight:600">${p.cod}</span> <span class="k">${esc(p.nome)}</span>
          <div class="k3" style="font-size:var(--t-xs)">${esc(p.qta)} · ${plur(D.parti.length,"sotto-parte","sotto-parti")} · PDF ${st.pdfConf}/${st.pdfO} confermati</div>
          <div class="docs">${pill("STEP",D.step,true)}${pill("PDF",D.pdf,true)}</div></span>
        ${r.prodotti.length>1?I("chev",14):""}</button>`;}).join("")}
    <div class="leg"><span>verde: c’è</span><span>viola: da confermare</span><span>ocra: da verificare</span><span>azzurro: sul portale</span><span>rosso: manca</span></div></div>
  ${tzs.length?`<div class="sec"><h3 class="lab">${I("factory",13)} Terzisti</h3>${tzs.map(t=>`<div class="itm"><span class="g"><b style="font-weight:500">${esc(t.tzn)}</b>
      <div class="k3" style="font-size:var(--t-sm)">${esc(t.lav)}</div><div style="margin-top:4px">${t.esito}</div></span>
      <button class="btn sm" data-tz="${t.tz}:0">${I("chev",13)}</button></div>`).join("")}</div>`:""}
  <div class="sec"><h3 class="lab">${I("users",13)} Persone</h3>
    ${pers.map(n=>{const x=c.mittenti.find(y=>y.n===n);return `<div class="pers"><span class="av" style="background:${c.col}">${ini(n)}</span>
      <span style="min-width:0"><b style="font-weight:500">${esc(n)}</b><div class="k3" style="font-size:var(--t-xs)">${esc(x?x.r+" · "+x.e:"")}</div></span></div>`;}).join("")}</div>`;
}

/* ---- nuova richiesta da «Da smistare» ---- */
function nuovaRichiesta(){
  const m=SMISTARE.find(x=>x.id===S.nuovaFrom); if(!m) return vuoto("tray","Messaggio non trovato","Torna a «Da smistare».");
  const c=cl(m.cli);
  return `<div class="rfqhead"><div class="bc">${I("tray",14)}<button class="btn sm ghost" data-tab="inbox" data-sez="smistare">Da smistare</button>
    ${I("chev",13)}<b>Nuova richiesta</b></div>
    ${cart(`<div class="cell"><span class="lab">Dal messaggio</span><h1 style="font-family:var(--fd);color:var(--ink);font-size:1.1875rem">${esc(m.sub)}</h1>
      <span class="nome">${esc(m.chi)} · <span class="mono">${esc(m.em)}</span></span></div>
      <div class="cell"><span class="lab">Cliente</span><div class="v">${esc(c.nome)}</div>${C("ok","mittente autorizzato","shield")}</div>
      <div class="cell"><span class="lab">Arrivato</span><div class="v">${m.t}</div></div>
      <div class="cell span2"><span class="lab">Proposta del sistema</span><div class="v">${m.stato}</div></div>`)}</div>
  <div class="rfqbody"><div class="rfq">
    <div class="fs"><header>${I("cube",17)}<h3>Prodotti richiesti</h3><span class="sp"></span><span class="k3" style="font-size:var(--t-sm)">spunta i codici veri: niente nasce senza spunta</span></header>
      ${(m.codici||[]).map(([cod,nome,qta],i)=>`<div class="pick" style="grid-template-columns:auto minmax(0,1fr) minmax(0,1fr) minmax(0,.7fr)">
        <input type="checkbox" checked data-cod="${i}" aria-label="usa ${esc(cod)}">
        <span class="mono" style="font-weight:600">${esc(cod)}</span>
        <input class="fldi" data-nome="${i}" value="${esc(nome)}" aria-label="denominazione" style="padding:6px 8px;border:1px solid var(--line2);border-radius:6px">
        <input class="fldi" data-qta="${i}" value="${esc(qta)}" aria-label="quantità" style="padding:6px 8px;border:1px solid var(--line2);border-radius:6px"></div>`).join("")}
      <div class="g2"><label class="fld"><span>Oggetto della richiesta</span><input id="nr-tit" value="${esc(m.sub)}"></label>
        <label class="fld"><span>Scadenza dell’offerta</span><input id="nr-scad" type="date"><small>se non è indicata, va chiesta al buyer</small></label></div></div>
    <div class="fs"><header>${I("clip",17)}<h3>Allegati</h3></header>
      <p class="hint">Gli allegati entrano nella richiesta; il sistema li riconosce e li propone nella pagina RFQ, dove li confermi.</p>
      ${(m.files||[]).map(f=>`<div class="pick" style="grid-template-columns:auto minmax(0,1fr) auto"><span class="ext ${f.e}">${f.e.toUpperCase()}</span>
        <span style="min-width:0"><span class="fn">${esc(f.n)}</span> <span class="fs">${f.s}</span></span>${C("prop","riconosciuto dal nome")}</div>`).join("")||`<p class="hint">Nessun allegato.</p>`}</div>
    <div class="acts" style="margin:0"><button class="btn pri xl" data-creareq="1">${I("check",18)} Crea la richiesta</button>
      <button class="btn" data-tab="inbox" data-sez="smistare">Annulla</button></div>
  </div></div>`;
}
