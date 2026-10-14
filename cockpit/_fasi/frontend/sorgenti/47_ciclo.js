/* ================= DISTINTA · CICLO DI PRODUZIONE =================
   Per ogni componente, dal prodotto ai figli: il disegno (con le stesse note dell'albero), il materiale,
   le fasi in ordine cronologico (interne, esterne dai terzisti, acquisto), dove entrano i figli e i commerciali,
   la maschera di saldatura, robot o manuale. Le fasi esterne pescano dal catalogo dei terzisti (qualificati per
   cliente, come cliente_fornitore_lavorazione) e dallo storico delle lavorazioni esterne già fatte, e preparano la
   richiesta al terzista con disegni e istruzioni. Serve a Lino: costi e pianificazione vengono dopo, con altri strumenti. */
/* il catalogo dei processi, per categoria: ogni voce è interna o esterna (terzisti). Niente «acquisto»:
   i commerciali non hanno un ciclo, entrano in una fase del padre (saldatura o assemblaggio) e li compra Fabio */
const CAT=[
 ["taglio","Taglio",[["laser","Taglio laser lamiera","int"],["lasertubo","Taglio laser tubo","int"],["sega","Taglio a sega","int"]]],
 ["forma","Piegatura e formatura",[["piega","Piegatura lamiera","int"],["curva","Curvatura tubi","int"],["stampa","Stampaggio","int"],["curvatura_tubi","Curvatura tubi da terzi","est"]]],
 ["mecc","Lavorazioni meccaniche",[["fresa","Fresatura CNC","int"],["foratura","Foratura","int"],["filettatura","Filettatura","int"],["sbavatura","Sbavatura","int"],["tornitura","Tornitura","est"],["fresatura_est","Fresatura da terzi","est"]]],
 ["sald","Saldatura",[["puntatura","Puntatura","int"],["sald_robot","Saldatura robotizzata","int"],["sald_mag","Saldatura manuale MIG/MAG","int"],["sald_tig","Saldatura manuale TIG","int"]]],
 ["ass","Assemblaggio",[["assemblaggio","Assemblaggio dei componenti","int"],["montaggio","Montaggio con viteria","int"],["inserti","Insertaggio","int"],["rivettatura","Rivettatura","int"],["piantaggio","Piantaggio a pressa","int"]]],
 ["sup","Trattamenti superficiali",[["sabbiatura","Sabbiatura","est"],["cataforesi","Cataforesi","est"],["verniciatura_polvere","Verniciatura a polvere","est"],["verniciatura_liquido","Verniciatura a liquido","est"],["zincatura","Zincatura","est"],["lavaggio_zinco","Lavaggio e zinco","est"]]],
 ["term","Trattamenti termici",[["trattamento_termico","Trattamento termico","est"]]],
 ["fin","Finitura e controllo",[["raddrizzatura","Raddrizzatura","int"],["marcatura","Marcatura ed etichettatura","int"],["controllo","Controllo dimensionale","int"]]]
];
const PROC={}; CAT.forEach(([g,gn,l])=>l.forEach(([k,n,t])=>{PROC[k]=[n,t,g,gn];}));
/* le preparazioni: attrezzature e lavori una tantum fatti apposta per il pezzo, come la maschera di saldatura.
   Si spuntano in testa al ciclo, fuori dalla sequenza delle fasi; «suggerita» quando una fase del ciclo la fa pensare */
const PREP=[["maschera","Maschera di saldatura",f=>CON_MASCHERA.has(f.p)&&f.maschera],
 ["robot","Programma del robot di saldatura",f=>f.p==="sald_robot"],
 ["piega","Attrezzatura di piega",f=>f.p==="piega"],
 ["curva","Attrezzatura di curvatura",f=>f.p==="curva"||f.p==="curvatura_tubi"],
 ["stampo","Stampo",f=>f.p==="stampa"],
 ["dima","Dima di foratura",f=>f.p==="foratura"],
 ["calibro","Calibro di controllo",f=>f.p==="controllo"]];
const SALD=new Set(["sald_robot","sald_mag","sald_tig"]);
const CON_FIGLI=new Set(["puntatura",...SALD,"assemblaggio","montaggio","inserti","rivettatura","piantaggio"]); /* qui entrano figli e commerciali */
const CON_MASCHERA=new Set(["puntatura",...SALD]);
const TRATT=new Set(["sabbiatura","cataforesi","verniciatura_polvere","verniciatura_liquido","zincatura","lavaggio_zinco"]); /* chiedono un terzista qualificato per il cliente */
const codEst=k=>k==="fresatura_est"?"fresatura":k;   /* codice della lavorazione come in cliente_fornitore_lavorazione */
const opzProcessi=sel=>CAT.map(([g,gn,l])=>`<optgroup label="${gn}">${l.map(([k,n,t])=>`<option value="${k}" ${k===sel?"selected":""}>${n}${t==="est"?" · terzisti":""}</option>`).join("")}</optgroup>`).join("");
const MATERIALI=["S235JR","S355JR","S355J2","DD11","C40","11SMnPb37","AISI 304","Al 6082 T6"];
const SEMI=["lamiera","tubo tondo","tubo quadro o rettangolare","barra","profilo"];
const TIPO_FASE={int:["neu","interna"],est:["acc","esterna · terzista"]};
let contaFasi=0;
const fase=o=>Object.assign({id:"f"+(++contaFasi)},o);
const WF={cur:null,vis:{armato:false,nuovo:null,sel:null,zoom:0},invio:null,msg:null,cat:null,catq:"",catFase:null,uplFase:null};

/* il catalogo dei terzisti: lavorazioni e qualifiche per cliente (nel backend: cliente_fornitore_lavorazione, 0014) */
(()=>{const add={f3:{lavs:["zincatura","lavaggio_zinco"],qual:{acme:["zincatura"],tdl:["zincatura"]}},
  f2:{lavs:["tornitura","fresatura"],qual:{}},
  f1:{lavs:["verniciatura_polvere","verniciatura_liquido"],qual:{fitlab:["verniciatura_polvere"],tdl:["verniciatura_polvere"],potaflex:["verniciatura_polvere","verniciatura_liquido"]}}};
 TERZISTI.forEach(t=>Object.assign(t,add[t.id]||{lavs:[],qual:{}}));
 TERZISTI.push(
  {id:"f5",nome:"Sabbiatura Tiberina",sigla:"ST",lav:"sabbiatura, cataforesi e polveri",chat:[],lavs:["sabbiatura","cataforesi","verniciatura_polvere"],
   qual:{acme:["sabbiatura","cataforesi","verniciatura_polvere"],fitlab:["sabbiatura"],mietigamma:["sabbiatura","cataforesi"]}},
  {id:"f4",nome:"Termotrattamenti Umbri",sigla:"TU",lav:"trattamenti termici",chat:[],lavs:["trattamento_termico"],qual:{}},
  {id:"f6",nome:"Curvatubi Adriatica",sigla:"CA",lav:"curvatura tubi oltre Ø80",chat:[],lavs:["curvatura_tubi"],qual:{}});})();
const terz=id=>TERZISTI.find(t=>t.id===id);
function qualificato(t,cli,k){ if(!TRATT.has(k)) return null; return ((t.qual||{})[cli]||[]).includes(codEst(k)); }

/* lo storico delle lavorazioni esterne già fatte: disegno, istruzioni e terzista si riusano */
const STORICO_EST=[
 {id:"e1",cod:"9N008519AB",nome:"Perno guida",cli:"fitlab",p:"tornitura",terz:"f2",quando:"02/10/2026",rif:"RFQ RDO 2026/318",prezzo:"4,20 €/pz · lotti da 300",file:"9N008519AB.pdf",matNostro:false,spec:"",
  istr:"Tornitura da barra C40 Ø42 a vostro carico. Ø40 h9, smussi 1×45°, filetto M10 in testa. Lotti da 300."},
 {id:"e2",cod:"9990708A1",nome:"Supporto batteria",cli:"acme",p:"zincatura",terz:"f3",quando:"03/10/2026",rif:"RFQ 990020338",prezzo:"0,42 €/pz · lotti da 500",file:"9990708A_1.pdf",matNostro:true,spec:"Fe/Zn 12 III Cr3 · SPEC-ZINC-013",
  istr:"Zincatura Fe/Zn 12 III Cr3 secondo SPEC-ZINC-013. Il pezzo arriva saldato e sgrassato. Proteggere i fori filettati dei dadi; deidrogenazione se richiesta dal capitolato."},
 {id:"e3",cod:"9993449A1",nome:"Staffa cofano",cli:"acme",p:"zincatura",terz:"f3",quando:"12/05/2026",rif:"RFQ 990019877",prezzo:"0,38 €/pz · lotti da 400",file:"9993449A_1.pdf",matNostro:true,spec:"Fe/Zn 12 III Cr3 · SPEC-ZINC-013",
  istr:"Zincatura Fe/Zn 12 III Cr3 secondo SPEC-ZINC-013 su staffa piegata, appesa dal foro Ø9."},
 {id:"e4",cod:"9995936A1",nome:"Telaio ROPS",cli:"acme",p:"cataforesi",terz:"f5",quando:"09/09/2026",rif:"RFQ 990020214",prezzo:"6,80 €/pz con sabbiatura e polveri",file:"9995936A_1.pdf",matNostro:true,spec:"ciclo sabbiatura + cataforesi + polveri RAL 9011",
  istr:"Ciclo completo: sabbiatura, cataforesi, polveri RAL 9011. Mascherare le boccole e le superfici di appoggio indicate sul disegno."},
 {id:"e5",cod:"9N004719AA",nome:"Supporto pedana",cli:"fitlab",p:"sabbiatura",terz:"f5",quando:"31/08/2026",rif:"campionatura CAP-SAB-017",prezzo:"1,10 €/pz",file:"9N004719AA.pdf",matNostro:true,spec:"CAP-SAB-017",
  istr:"Sabbiatura secondo CAP-SAB-017 prima della verniciatura CAP-VER-024. Rugosità come da capitolato."},
 {id:"e6",cod:"9N008100AB",nome:"Telaio pedana",cli:"fitlab",p:"verniciatura_polvere",terz:"f1",quando:"15/07/2026",rif:"RFQ RDO 2026/244",prezzo:"3,90 €/pz",file:"9N008100AB.pdf",matNostro:true,spec:"CAP-VER-024",
  istr:"Verniciatura a polvere secondo CAP-VER-024, colore a capitolato. Pezzo già sabbiato."},
 {id:"e7",cod:"9D010306AA",nome:"Tower (alluminio)",cli:"fitlab",p:"trattamento_termico",terz:"f4",quando:"02/07/2026",rif:"RFQ Tower",prezzo:"a forno, 180 €",file:"9D010306AA.pdf",matNostro:true,spec:"T4 → T6, alluminio 6082",
  istr:"Invecchiamento T4 → T6 dopo la saldatura. Il saldato arriva su telaio di appoggio."},
 {id:"e8",cod:"9N003311AB",nome:"Boccola",cli:"fitlab",p:"tornitura",terz:"f2",quando:"20/06/2026",rif:"RFQ RDO 2026/198",prezzo:"0,95 €/pz · lotti da 1.000",file:"9N003311AB.pdf",matNostro:false,spec:"",
  istr:"Boccola da barra, materiale del terzista: nessuna lavorazione nostra prima."},
 {id:"e9",cod:"S9.0987",nome:"Supporto cabina",cli:"tdl",p:"verniciatura_polvere",terz:"f1",quando:"20/06/2026",rif:"S9-RDO-0987",prezzo:"2,60 €/pz",file:"S9.0987.pdf",matNostro:true,spec:"RAL 7016 TDL",
  istr:"Polvere RAL 7016, spessore minimo 80 µm."}
];

/* cicli proposti e confermati (per prodotto e casella). Le RFQ senza ciclo partono vuote: si propone un ciclo tipico */
const CICLI={
 "at1|r":{stato:"proposto",prep:{maschera:{on:true,nota:"una maschera per puntatura e saldatura, con i riferimenti dei 6 dadi"},robot:{on:true,nota:""}},fonte:"dalle note del disegno 9990708A_1.pdf (saldatura e zincatura) e dallo storico esterno",fasi:[
   fase({p:"puntatura",figli:{"at1-0":1,"at1-1":2,"at1-2":3,"at1-3":1},maschera:true,nota:"i dadi si puntano sul supporto prima della saldatura"}),
   fase({p:"sald_robot",maschera:true,norma:"SPEC-SALD-023 · ad arco continua"}),
   fase({p:"controllo",nota:"scansione 3D del primo pezzo"}),
   fase({p:"zincatura",terz:"f3",spec:"Fe/Zn 12 III Cr3 · SPEC-ZINC-013",matNostro:true,daCat:"e2",istr:STORICO_EST[1].istr}),
   fase({p:"controllo",nota:"spessore dello zinco a campione"})]},
 "at1|at1-0":{stato:"proposto",fonte:"dal disegno 9990707A_1.pdf: lamiera S235JR sp. 3, vista A",mat:{m:"S235JR",s:"lamiera",d:"sp. 3",forn:"noi"},fasi:[
   fase({p:"laser"}),fase({p:"piega",nota:"1 piega a 90° (vista A)"}),fase({p:"sbavatura"})]},
 "tg1|r":{stato:"confermato",fonte:"confermato da Lino il 06/10",fasi:[
   fase({p:"piantaggio",figli:{"tg1-0":1,"tg1-1":2},nota:"perni piantati a pressa nell’assieme già verniciato"}),fase({p:"controllo"})]},
 "tg1|tg1-0":{stato:"proposto",fonte:"dal disegno 9N008518AA.pdf e dai capitolati CAP-SAB-017 e CAP-VER-024",fasi:[
   fase({p:"puntatura",figli:{"tg1-2":1},maschera:false}),
   fase({p:"sald_mag",maschera:false,norma:""}),
   fase({p:"raddrizzatura"}),
   fase({p:"sabbiatura",terz:"f5",spec:"CAP-SAB-017",matNostro:true,istr:""}),
   fase({p:"verniciatura_polvere",terz:"f1",spec:"CAP-VER-024",matNostro:true,istr:""}),
   fase({p:"controllo"})]},
 "tg1|tg1-1":{stato:"confermato",fonte:"dallo storico: tornito da Torneria Bolognese il 02/10",mat:{m:"C40",s:"barra",d:"Ø42",forn:"terzista"},fasi:[
   fase({p:"tornitura",terz:"f2",matNostro:false,daCat:"e1",spec:"",istr:STORICO_EST[0].istr,inviata:true})]},
 "tg1|tg1-2":{stato:"proposto",fonte:"dal disegno 9N008520AA.pdf",mat:{m:"S235JR",s:"lamiera",d:"sp. 8",forn:"noi"},fasi:[
   fase({p:"laser"}),fase({p:"filettatura",filetti:"4 × M8 passanti"})]},
 "pl1|r":{stato:"confermato",prep:{maschera:{on:true,nota:"quotata a parte: 1.300 €"}},fonte:"confermato da Lino il 12/09 · offerta SO 5478",fasi:[
   fase({p:"puntatura",figli:{"pl1-0":2},maschera:true}),
   fase({p:"sald_mag",maschera:true,norma:"",nota:"stessa maschera della puntatura"}),
   fase({p:"controllo",nota:"quotato grezzo: la verniciatura a cartiglio resta da integrare quando arriva il capitolato"})]},
 "pl1|pl1-0":{stato:"confermato",fonte:"confermato da Lino il 12/09",mat:{m:"S235JR",s:"lamiera",d:"sp. 4",forn:"noi"},fasi:[
   fase({p:"laser"}),fase({p:"piega",pieghe:"2 pieghe a 90°"})]}
};
const cicloDi=(pid,n)=>CICLI[pid+"|"+n.id]||null;
function cicloPer(pid,n){const k=pid+"|"+n.id; return CICLI[k]||(CICLI[k]={stato:"proposto",fonte:"scritto a mano",mat:{m:"",s:"",d:"",forn:"noi"},fasi:[]});}
function statoCiclo(pid,n){ if(n.tipo==="comm") return "acquisto"; const c=cicloDi(pid,n); return !c||!c.fasi.length?"vuoto":c.stato;}
const CHIP_CICLO={vuoto:["ghost","senza ciclo"],proposto:["prop","da confermare"],confermato:["ok","ciclo confermato"],acquisto:["warn","da acquistare"]};
function contaCicli(pid){const B=bomDi(pid), da=B.nodes.filter(n=>n.tipo!=="comm"); return [da.filter(n=>statoCiclo(pid,n)==="confermato").length,da.length];}
function faseDi(id){for(const k in CICLI){const c=CICLI[k], i=c.fasi.findIndex(f=>f.id===id); if(i>=0) return [c,i,c.fasi[i],k];} return [null,-1,null,null];}
function tagFase(nt){ if(!nt||!nt.fase) return ""; const [,i,f]=faseDi(nt.fase); return f?` · fase ${(i+1)*10} ${PROC[f.p][0]}`:""; }
function tocca(c){ if(c.stato==="confermato"){c.stato="proposto"; c.fonte="modificato dopo la conferma: da riconfermare";} }
/* in quale fase del padre entra un componente */
function faseNelPadre(pid,B,n){
  if(!n.padre) return null; const cp=cicloDi(pid,nodo(B,n.padre)); if(!cp) return null;
  const i=cp.fasi.findIndex(f=>f.figli&&f.figli[n.id]!=null); return i<0?null:[i,cp.fasi[i]];
}
function proponiTipico(pid,B,n){
  const c=cicloPer(pid,n), figli=figliDi(B,n.id), tutti=Object.fromEntries(figli.map(f=>[f.id,f.qta]));
  if(n.tipo==="sciolto") c.fasi=[fase({p:"laser"}),fase({p:"piega"}),fase({p:"sbavatura"})];
  else { c.fasi=[fase({p:"puntatura",figli:tutti,maschera:true}),fase({p:"sald_robot",maschera:true}),fase({p:"controllo"})]; c.prep={maschera:{on:true,nota:""},robot:{on:true,nota:""}}; }
  c.stato="proposto"; c.fonte="ciclo tipico per "+TIPI[n.tipo].toLowerCase()+": da controllare sul disegno";
}
/* i controlli: che cosa manca perché il ciclo si possa confermare */
function controlliNodo(pid,B,n,cli){
  const out=[], c=cicloDi(pid,n);
  if(n.tipo==="comm") return out;
  if(!c||!c.fasi.length){ out.push(["warn","Nessuna fase: il ciclo è vuoto."]); return out; }
  if(n.tipo==="sciolto"&&!(c.mat&&c.mat.m)) out.push(["warn","Manca il materiale."]);
  figliDi(B,n.id).forEach(f=>{ if(!c.fasi.some(x=>x.figli&&x.figli[f.id]!=null)) out.push(["bad",`${f.codice} non entra in nessuna fase: indica dove si monta, si punta o si salda.`]); });
  let saldato=false;
  c.fasi.forEach((f,i)=>{const num=(i+1)*10, P=PROC[f.p];
    if(SALD.has(f.p)) saldato=true;
    if(f.p==="sald_robot"&&!f.maschera) out.push(["warn",`Fase ${num}: saldatura robotizzata senza maschera. È voluto?`]);
    if(CON_MASCHERA.has(f.p)&&f.maschera&&!((c.prep||{}).maschera||{}).on&&!out.some(x=>x[1].startsWith("Saldatura in maschera"))) out.push(["info","Saldatura in maschera: se la maschera non c’è ancora, spuntala fra le preparazioni in alto."]);
    if(CON_FIGLI.has(f.p)&&!SALD.has(f.p)&&!Object.keys(f.figli||{}).length) out.push(["warn",`Fase ${num}: quali componenti entrano qui?`]);
    if(P[1]==="est"){
      if(!f.terz) out.push(["warn",`Fase ${num}: ${P[0].toLowerCase()} senza terzista.`]);
      else { const t=terz(f.terz), q=qualificato(t,cli,f.p); if(q===false) out.push(["bad",`Fase ${num}: ${t.nome} non è qualificato per ${cl(cli).nome} per ${P[0].toLowerCase()}.`]); }
      if(TRATT.has(f.p)&&!saldato&&c.fasi.slice(i+1).some(x=>SALD.has(x.p))) out.push(["warn",`Fase ${num}: trattamento prima della saldatura. È voluto?`]);
    }
  });
  /* regola di Lino: il materiale lo mettiamo noi, tranne quando il pezzo non passa prima da una nostra lavorazione (es. boccole) */
  const primaEst=c.fasi.length&&PROC[c.fasi[0].p][1]==="est";
  if(primaEst&&c.fasi[0].matNostro!==false) out.push(["info","Nessuna nostra lavorazione prima della fase esterna: di solito il materiale lo mette il terzista."]);
  if(n.tipo==="sciolto"&&c.mat&&primaEst&&c.mat.forn==="noi"&&c.fasi[0].matNostro===false) out.push(["warn","Il materiale risulta nostro, ma la fase esterna dice che lo mette il terzista."]);
  return out;
}

/* ---- la vista ---- */
function passoCiclo(q,d,pid,B){
  const ord=inOrdine(B); if(!WF.cur||!nodo(B,WF.cur)) WF.cur="r";
  const i=ord.findIndex(([n])=>n.id===WF.cur), n=ord[i][0], prev=ord[i-1], next=ord[i+1];
  const path=[]; for(let x=n;x;x=x.padre?nodo(B,x.padre):null) path.unshift(x);
  return `${WF.msg?`<div class="esito ${WF.msg.ok?"ok":"no"}" role="status">${I(WF.msg.ok?"check":"alert",16)}<div>${WF.msg.t}</div></div>`:""}
    <div class="wfnav">
      <button class="btn" data-wf="prev" ${prev?"":"disabled"}>← ${prev?`<span class="mono">${esc(prev[0].codice)}</span>`:"Inizio"}</button>
      <div class="wfdove"><span class="k3" style="font-size:var(--t-xs)">componente ${i+1} di ${ord.length} · dal prodotto ai figli</span>
        <div class="wfpath">${path.map(p=>`<button class="btn sm ghost${p.id===n.id?" on":""}" data-wfgo="${p.id}">${esc(p.codice)}</button>`).join(I("chev",12))}</div></div>
      <button class="btn pri" data-wf="next" ${next?"":"disabled"}>${next?`<span class="mono">${esc(next[0].codice)}</span>`:"Fine"} →</button></div>
    <div class="mini" role="list" aria-label="Componenti in ordine, dal prodotto ai figli">${ord.map(([m,l])=>{const st=statoCiclo(pid,m);
      return `<button class="mchip${m.id===n.id?" on":""}" role="listitem" data-wfgo="${m.id}" title="${esc(TIPI[m.tipo])} ${esc(m.codice)} · ${CHIP_CICLO[st][1]}">${"<span class=\"lv\"></span>".repeat(l)}<span class="mdot ${m.tipo}"></span><span class="mono">${esc(m.codice)}</span><span class="st ${st}">${st==="confermato"?"✓":st==="proposto"?"✓?":st==="acquisto"?"acq.":"○"}</span></button>`;}).join("")}</div>
    <div class="wfgrid"><div class="fs wfpdf">${pdfInline(pid,n)}</div><div class="fs wfciclo">${cicloEditor(q,d,pid,B,n)}</div>
      <div class="wfside">${controlliProdotto(pid,B,d.cli)}</div></div>
    ${WF.cat?catalogo(d.cli):""}`;
}
function pdfInline(pid,n){
  const s=slotNodo(pid,n,"d2"), ok=!!(s&&!s.portale&&/\.pdf$/i.test(s.f));
  if(!ok) return `<header>${I("file",17)}<h3>Disegno</h3></header><div class="wfvuoto">${n.tipo==="comm"?ICONA_DADO:I("file",28)}
    <p>${n.tipo==="comm"?"Particolare commerciale: non serve un disegno 2D. Si compra a catalogo e si monta nella fase del padre.":"Nessun PDF per questo componente: si associa nel passo Documenti e NAS."}</p>
    ${n.tipo!=="comm"?`<button class="btn sm" data-step="0">${I("file",13)} Vai a Documenti e NAS</button>`:""}</div>`;
  const V=WF.vis, z=ZOOM[V.zoom], lista=NOTE[s.f]||[], c=cicloDi(pid,n);
  return `<header>${I("file",17)}<h3 class="mono" style="font-size:var(--t-base)">${esc(s.f)}</h3><span class="sp"></span>
      <span class="gruppo" role="group" aria-label="Zoom"><button data-wv="zmeno" aria-label="Rimpicciolisci">−</button><span class="z">${Math.round(z*100)}%</span><button data-wv="zpiu" aria-label="Ingrandisci">+</button></span>
      <button class="btn sm" data-wv="arma" aria-pressed="${V.armato}">${V.armato?"Fai clic sul disegno…":"+ Nota"}</button>
      <button class="btn sm" data-dvis="${esc(s.f)}" title="Schermo intero">${I("eye",13)}</button></header>
    <div class="wfscena"><div class="carta${V.armato?" armata":""}" data-wv="carta" style="width:${z*100}%">${svgDisegno(s.f,n.codice,n.nome)}
      ${lista.map((nt,i)=>`<button class="pin${V.sel===nt.id?" sel":""}" style="left:${nt.x*100}%;top:${nt.y*100}%" data-wv="pin:${nt.id}" aria-label="Nota ${i+1}: ${esc(nt.testo)}">${i+1}</button>`).join("")}
      ${V.nuovo?`<span class="pin nuovo" style="left:${V.nuovo.x*100}%;top:${V.nuovo.y*100}%">${lista.length+1}</span>
        <div class="pop-nota" data-wv="pop" style="left:min(calc(${V.nuovo.x*100}% + 18px), calc(100% - 270px));top:min(calc(${V.nuovo.y*100}% - 10px), calc(100% - 210px))">
          <label class="fld"><span>Nota ${lista.length+1}</span><textarea id="wf-nota" maxlength="2000" placeholder="Che cosa c’è da sapere su questo punto?"></textarea></label>
          <label class="fld"><span>Riguarda la fase</span><select id="wf-nota-fase"><option value="">tutto il pezzo</option>${(c?c.fasi:[]).map((f,i)=>`<option value="${f.id}">${(i+1)*10} · ${esc(PROC[f.p][0])}</option>`).join("")}</select></label>
          <span style="display:flex;gap:8px;justify-content:flex-end"><button class="btn sm" data-wv="annullanota">Annulla</button><button class="btn sm pri" data-wv="salvanota">Salva la nota</button></span></div>`:""}
    </div></div>
    <div class="wfnote"><span class="lab">Note sul disegno · ${lista.length}</span>
      ${lista.length?lista.map((nt,i)=>`<div class="nota${V.sel===nt.id?" sel":""}" role="button" tabindex="0" data-wv="pin:${nt.id}"><span class="n">${i+1}</span><span>${esc(nt.testo)}</span>
        <span class="meta">${esc(nt.autore)} · ${esc(nt.quando)}${tagFase(nt)}</span>${nt.mia?`<span class="azioni"><button class="btn sm danger" data-wv="togli:${nt.id}">Togli</button></span>`:""}</div>`).join("")
        :`<p class="k3" style="margin:0;font-size:var(--t-sm)">Nessuna nota. «+ Nota», poi un clic sul punto del disegno.</p>`}
      <p class="k3" style="margin:0;font-size:var(--t-xs)">Le note sono le stesse dell’albero: quelle scritte qui si vedono aprendo il disegno dalla Struttura, e viceversa.</p></div>`;
}
function cicloEditor(q,d,pid,B,n){
  const cli=d.cli, c=cicloDi(pid,n), st=statoCiclo(pid,n), ctr=controlliNodo(pid,B,n,cli), bad=ctr.some(x=>x[0]==="bad");
  const padre=n.padre?nodo(B,n.padre):null, nelPadre=faseNelPadre(pid,B,n);
  const testa=`<header><span class="tipo ${n.tipo}">${TIPI[n.tipo]}</span><b class="mono">${esc(n.codice)}</b><span class="k" style="font-size:var(--t-sm)">${esc(n.nome||"")}</span><span class="sp"></span>${C(...CHIP_CICLO[st])}</header>
    <div class="wfinfo">
      <span>${n.padre?`×${qtaTot(B,n)} per prodotto`:"prodotto richiesto"} · ${esc(PROD[pid].qta)}</span>
      ${padre?`<span>entra in <button class="btn sm ghost" data-wfgo="${padre.id}"><span class="mono">${esc(padre.codice)}</span></button>
        ${nelPadre?C("ok",`fase ${(nelPadre[0]+1)*10} · ${PROC[nelPadre[1].p][0]}`):C("bad","in nessuna fase del padre")}</span>`:""}</div>`;
  if(n.tipo==="comm") return testa+`<div class="wfacq">${I("tray",20)}<div><b>Componente da acquistare</b>
      <p>Non ha un ciclo suo: lo compra l’ufficio acquisti (Fabio), che controlla la lista dei componenti e decide fornitore e codice.
      Qui conta solo <b>dove entra</b> nel ciclo del padre: in una fase di saldatura (per esempio un dado a saldare) o di assemblaggio.</p>
      ${padre?`<button class="btn sm" data-wfgo="${padre.id}">${I("wrench",13)} Apri il ciclo di <span class="mono">${esc(padre.codice)}</span></button>`:""}</div></div>`;
  return testa+`
    ${c&&c.fonte&&c.fasi.length?`<p class="hint" style="margin:0">${st==="confermato"?"":"Proposto "}${esc(c.fonte)}.</p>`:""}
    ${n.tipo==="sciolto"?materiale(cicloPer(pid,n)):`<p class="hint" style="margin:0">${TIPI[n.tipo]}: il materiale è nei suoi componenti. Qui si decide in che ordine si assemblano, si saldano e si trattano.</p>`}
    ${prepBox(cicloPer(pid,n))}
    <div class="fasi">${c&&c.fasi.length?c.fasi.map((f,i)=>faseCard(q,pid,B,n,c,f,i,cli)).join("")
      :`<div class="wfvuoto" style="padding:14px"><p>Nessuna fase ancora.</p><button class="btn" data-wf="proponi">${I("wrench",14)} Proponi un ciclo tipico</button></div>`}</div>
    <div class="wfadd"><select id="wf-nuova" aria-label="Fase da aggiungere">${opzProcessi(n.tipo==="sciolto"?"laser":"puntatura")}</select>
      <button class="btn" data-wf="aggiungi">${I("plus",14)} Aggiungi la fase</button>
      <button class="btn ghost" data-wf="catalogo">${I("factory",14)} Catalogo</button></div>
    ${ctr.length?`<ul class="ctrl">${ctr.map(([k,t])=>`<li class="${k}">${I(k==="bad"?"x":k==="warn"?"alert":"dot",13)}<span>${esc(t)}</span></li>`).join("")}</ul>`:`<p class="ctrlok">${I("check",14)} Nessun problema nel ciclo di questo componente.</p>`}
    <div class="acts" style="margin:0">${st==="confermato"?`<button class="btn" data-wf="riapri">Riapri il ciclo</button>`
      :`<button class="btn pri" data-wf="conferma" ${bad||st==="vuoto"?"disabled":""} title="${bad?"Prima risolvi i punti in rosso":""}">${I("check",14)} Conferma il ciclo</button>`}</div>`;
}
function prepBox(c){
  const pr=c.prep||(c.prep={}), fasi=c.fasi||[];
  return `<div class="prep"><div class="prep-t"><span class="lab">${I("wrench",13)} Preparazioni da realizzare</span>
      <span class="k3" style="font-size:var(--t-xs)">attrezzature fatte apposta per questo pezzo, una volta sola: spunta quelle da costruire</span></div>
    <div class="prep-g">${PREP.map(([k,l,sugg])=>{const v=pr[k]||{}, sg=fasi.some(sugg);
      return `<div class="prep-i${v.on?" on":""}"><label class="chk-m"><input type="checkbox" data-wfp="${k}" ${v.on?"checked":""}> <b>${l}</b>${sg&&!v.on?` <span class="sug-chip">suggerita</span>`:""}</label>
        ${v.on?`<input class="prep-n" data-wfpn="${k}" value="${esc(v.nota||"")}" placeholder="nota: per quanti pezzi, chi la fa, costo noto…" aria-label="Nota su ${esc(l)}">`:""}</div>`;}).join("")}</div></div>`;
}
function materiale(c){
  const m=c.mat||(c.mat={m:"",s:"",d:"",forn:"noi"}), opt=(arr,v)=>`<option value="">—</option>${arr.map(x=>`<option ${x===v?"selected":""}>${x}</option>`).join("")}${v&&!arr.includes(v)?`<option selected>${esc(v)}</option>`:""}`;
  return `<div class="wfmat"><span class="lab">Materiale</span><div class="wfmat-g">
    <label class="fld"><span>Materiale</span><select data-wfm="m">${opt(MATERIALI,m.m)}</select></label>
    <label class="fld"><span>Semilavorato</span><select data-wfm="s">${opt(SEMI,m.s)}</select></label>
    <label class="fld"><span>Dimensione</span><input data-wfm="d" value="${esc(m.d)}" placeholder="sp. 3 · Ø42 · 40×40×3"></label>
    <label class="fld"><span>Lo fornisce</span><select data-wfm="forn"><option value="noi" ${m.forn!=="terzista"?"selected":""}>Promatec</option><option value="terzista" ${m.forn==="terzista"?"selected":""}>il terzista</option></select></label></div></div>`;
}
function faseCard(q,pid,B,n,c,f,i,cli){
  const P=PROC[f.p], num=(i+1)*10, [tc,tl]=TIPO_FASE[P[1]], id=f.id;
  const inp=(campo,lab,ph)=>`<label class="fld"><span>${lab}</span><input data-wff="${id}|${campo}" value="${esc(f[campo]||"")}" placeholder="${esc(ph||"")}"></label>`;
  let corpo="";
  if(CON_FIGLI.has(f.p)){
    const figli=figliDi(B,n.id), sald=SALD.has(f.p), quanti=Object.keys(f.figli||{}).length;
    corpo+=`${sald?`<details class="figli-d" ${quanti?"open":""}><summary>${quanti?plur(quanti,"componente saldato","componenti saldati")+" in questa fase":"Si salda qui un componente in più? (per esempio un dado a saldare)"}</summary>`:""}<div class="figli-l">${sald?"":`<span class="lab">Componenti che entrano in questa fase</span>`}${figli.length?figli.map(m=>{
      const dentro=f.figli&&f.figli[m.id]!=null, altrove=c.fasi.findIndex(x=>x!==f&&x.figli&&x.figli[m.id]!=null);
      return `<label class="figlio"><input type="checkbox" data-wff="${id}|figlio:${m.id}" ${dentro?"checked":""}>
        <span class="tipo ${m.tipo}">${TIPI[m.tipo]}</span><span class="mono">${esc(m.codice)}</span><span class="k3">${esc(m.nome||"")}</span>
        <span class="sp"></span>${altrove>=0?`<span class="k3" style="font-size:var(--t-xs)">anche in fase ${(altrove+1)*10}</span>`:""}
        ${dentro?`<input class="q" type="number" min="1" data-wff="${id}|fq:${m.id}" value="${f.figli[m.id]}" aria-label="quantità di ${esc(m.codice)}">`:""}</label>`;}).join("")
      :`<p class="k3" style="margin:0;font-size:var(--t-sm)">Questo componente non ha figli nella distinta.</p>`}</div>${sald?"</details>":""}`;
  }
  if(CON_MASCHERA.has(f.p)) corpo+=`<div class="maschera"><label class="chk-m"><input type="checkbox" data-wff="${id}|maschera" ${f.maschera?"checked":""}> <b>In maschera</b> <span class="k3">la ${f.p==="puntatura"?"puntatura":"saldatura"} si fa con la maschera di saldatura</span></label>
      </div>
    ${SALD.has(f.p)?`<div class="wf2">${inp("norma","Norma o specifica","es. SPEC-SALD-023, ISO 5817 C")}</div>`:""}`;
  if(f.p==="filettatura") corpo+=`<div class="wf2">${inp("filetti","Filetti","es. 4 × M8 passanti")}</div>`;
  if(f.p==="piega") corpo+=`<div class="wf2">${inp("pieghe","Pieghe","es. 2 pieghe a 90°")}</div>`;
  if(P[1]==="est"){
    const cand=TERZISTI.filter(t=>(t.lavs||[]).includes(codEst(f.p)));
    corpo+=`<div class="wf2"><label class="fld"><span>Terzista</span><select data-wff="${id}|terz"><option value="">${cand.length?"scegli…":"nessuno nel catalogo"}</option>
        ${cand.map(t=>{const qq=qualificato(t,cli,f.p);return `<option value="${t.id}" ${f.terz===t.id?"selected":""}>${esc(t.nome)}${qq===true?" · qualificato per "+esc(cl(cli).nome):qq===false?" · NON qualificato per "+esc(cl(cli).nome):""}</option>`;}).join("")}</select></label>
      ${inp("spec","Specifica o capitolato","es. CAP-VER-024, Fe/Zn 12")}</div>
      <label class="chk-mat"><input type="checkbox" data-wff="${id}|matNostro" ${f.matNostro!==false?"checked":""}> Il materiale (o il pezzo) lo mandiamo noi</label>
      <label class="fld"><span>Istruzioni per il terzista</span><textarea data-wff="${id}|istr" rows="2" placeholder="Che cosa deve sapere il terzista: superfici da proteggere, lotti, consegna…">${esc(f.istr||"")}</textarea></label>
      ${suggerimenti(pid,n,f,cli)}
      <div class="acts" style="margin:0">${f.inviata?`${C("acc","richiesta al terzista in bozza","mail")}${f.terz?`<button class="btn sm" data-tz="${f.terz}:${Math.max(0,terz(f.terz).chat.findIndex(ch=>ch.cod===n.codice))}">Apri la chat ${I("chev",12)}</button>`:""}`
        :`<button class="btn sm pri" data-wf="prepara:${id}" ${f.terz?"":"disabled title=\"Scegli prima il terzista\""}>${I("mail",13)} Prepara la richiesta al terzista</button>`}</div>
      ${WF.invio===id?invio(q,pid,B,n,f):""}`;
  }
  corpo+=`<div class="wf2">${inp("nota","Nota per la fase","")}</div>`;
  return `<div class="fase ${P[1]}"><div class="fase-n">${num}</div><div class="fase-c">
    <div class="fase-t"><span class="fase-cat">${esc(P[3])}</span><select data-wff="${id}|p" aria-label="Lavorazione della fase ${num}">${opzProcessi(f.p)}</select>${C(tc,tl)}<span class="sp"></span>
      <button class="btn sm ghost" data-wf="su:${id}" ${i===0?"disabled":""} aria-label="Sposta prima">↑</button>
      <button class="btn sm ghost" data-wf="giu:${id}" ${i===c.fasi.length-1?"disabled":""} aria-label="Sposta dopo">↓</button>
      <button class="btn sm ghost danger" data-wf="via:${id}" aria-label="Togli la fase">${I("x",13)}</button></div>
    ${corpo}</div></div>`;
}
function suggerimenti(pid,n,f,cli){
  const ss=STORICO_EST.filter(e=>e.p===f.p&&(e.cod===n.codice||e.cli===cli)).sort((a,b)=>(b.cod===n.codice)-(a.cod===n.codice)).slice(0,3);
  if(!ss.length) return "";
  return `<div class="sugg"><span class="lab">Dallo storico delle lavorazioni esterne</span>${ss.map(e=>`<div class="sug${f.daCat===e.id?" on":""}">
    <span style="min-width:0"><b>${e.cod===n.codice?"Già fatta per questo codice":"Già fatta per "+esc(cl(e.cli).nome)}</b> · <span class="mono">${esc(e.cod)}</span> ${esc(e.nome)}
      <div class="k3" style="font-size:var(--t-xs)">${esc(terz(e.terz).nome)} · ${esc(e.quando)} · ${esc(e.rif)} · ${esc(e.prezzo)}${e.spec?" · "+esc(e.spec):""} · disegno <span class="mono">${esc(e.file)}</span></div></span>
    ${f.daCat===e.id?C("ok","in uso","check"):`<button class="btn sm" data-wf="usa:${f.id}:${e.id}">Usa terzista, istruzioni e disegno</button>`}</div>`).join("")}</div>`;
}
/* il pacchetto per il terzista: i PDF allegati (del pezzo, dall'archivio per cliente, o caricati adesso) e le istruzioni */
function invio(q,pid,B,n,f){
  const t=terz(f.terz), c=cicloDi(pid,n), m=c&&c.mat, al=f.allegati||[];
  return `<div class="invio"><span class="lab">Richiesta a ${esc(t.nome)} · si apre come bozza nella sua chat dell’Inbox</span>
    <dl class="dati"><dt>Lavorazione</dt><dd>${esc(PROC[f.p][0])}${f.spec?" · "+esc(f.spec):""}</dd>
      <dt>Pezzo</dt><dd><span class="mono">${esc(n.codice)}</span> ${esc(n.nome||"")}</dd>
      <dt>Quantità</dt><dd>${esc(PROD[pid].qta)} × ${qtaTot(B,n)} per prodotto</dd>
      <dt>Materiale</dt><dd>${f.matNostro!==false?"lo mandiamo noi":"del terzista"}${m&&m.m?" · "+esc(m.m)+" "+esc(m.d||""):""}</dd>
      <dt>Istruzioni</dt><dd>${esc(f.istr||"—")}</dd></dl>
    <div class="allegati"><span class="lab">Disegni allegati · ${al.length}</span>
      ${al.length?al.map((a,j)=>`<div class="allegato"><span class="ext pdf">PDF</span><span style="min-width:0"><span class="fn">${esc(a.f)}</span>
          <div class="k3" style="font-size:var(--t-xs)">${esc(a.da)}${a.s?" · "+esc(a.s):""}</div></span>
          ${DIS_PDF(a.f)?`<button class="btn sm ghost" data-dvis="${esc(a.f)}" aria-label="Guarda ${esc(a.f)}">${I("eye",13)}</button>`:""}
          <button class="btn sm ghost danger" data-wf="stacca:${f.id}:${j}" aria-label="Togli ${esc(a.f)}">${I("x",13)}</button></div>`).join("")
        :`<p class="k3" style="margin:0;font-size:var(--t-sm)">Nessun disegno: il terzista ha bisogno del PDF del pezzo con la lavorazione richiesta.</p>`}
      <label class="chk-mat"><input type="checkbox" data-wff="${f.id}|conNote" ${f.conNote!==false?"checked":""}> Stampa sul PDF del pezzo le note del disegno (${plur((NOTE[(slotNodo(pid,n,"d2")||{}).f]||[]).length,"nota","note")})</label>
      <div class="acts" style="margin:0"><button class="btn sm" data-wf="catdis:${f.id}">${I("folder",13)} Aggiungi dall’archivio disegni</button>
        <button class="btn sm" data-wf="carica:${f.id}">${I("upload",13)} Carica un PDF</button>
        <input type="file" id="wf-upl" accept=".pdf,application/pdf" hidden></div></div>
    <div class="acts" style="margin:0"><button class="btn pri" data-wf="invia:${f.id}" ${al.length&&q.req?"":"disabled"}>${I("check",14)} Crea la bozza nella chat del terzista</button>
      <button class="btn" data-wf="annullainvio">Annulla</button>${q.req?"":`<span class="k3" style="font-size:var(--t-sm)">RFQ storica: la chat si apre solo da una richiesta in corso.</span>`}</div></div>`;
}
const DIS_PDF=f=>/\.pdf$/i.test(f||"");
function controlliProdotto(pid,B,cli){
  const righe=inOrdine(B).flatMap(([n])=>controlliNodo(pid,B,n,cli).filter(x=>x[0]!=="info").map(x=>[n,...x]));
  const [ok,tot]=contaCicli(pid), comm=B.nodes.filter(n=>n.tipo==="comm");
  return `<div class="fs"><header>${I("list",17)}<h3>Controlli del ciclo · <span class="mono">${esc(PROD[pid].cod)}</span></h3><span class="sp"></span>
      ${C(ok===tot?"ok":"prop",`${ok}/${tot} cicli confermati`)}<button class="btn sm" data-wf="catalogo">${I("factory",13)} Catalogo</button></header>
    ${righe.length?`<ul class="ctrl">${righe.map(([n,k,t])=>`<li class="${k}">${I(k==="bad"?"x":"alert",13)}<span><button class="btn sm ghost" data-wfgo="${n.id}"><span class="mono">${esc(n.codice)}</span></button> ${esc(t)}</span></li>`).join("")}</ul>`
      :`<p class="ctrlok">${I("check",14)} Ogni componente ha il suo ciclo, senza punti aperti.</p>`}
    ${(()=>{const pp=inOrdine(B).flatMap(([n])=>PREP.filter(([k])=>(((cicloDi(pid,n)||{}).prep||{})[k]||{}).on).map(([k,l])=>[n,l]));
      return pp.length?`<div class="prep-sum"><span class="lab">${I("wrench",13)} Preparazioni da realizzare · ${pp.length}</span>${pp.map(([n,l])=>`<span><button class="btn sm ghost" data-wfgo="${n.id}"><span class="mono">${esc(n.codice)}</span></button> ${esc(l)}</span>`).join("")}
        <span class="k3" style="font-size:var(--t-xs)">Sono costi una tantum, a parte dal prezzo del pezzo.</span></div>`:"";})()}
    ${comm.length?`<p class="hint" style="margin:0">${plur(comm.length,"componente da acquistare","componenti da acquistare")} (${comm.map(n=>esc(n.codice)).join(", ")}): li gestisce Fabio dalla lista dei componenti.</p>`:""}
    <p class="hint" style="margin:0">Quando tutti i cicli sono confermati, il prodotto ha il ciclo completo per la valutazione dei costi e, se il cliente accetta, per la pianificazione.</p></div>`;
}
/* l'archivio dei disegni, per cliente e per prodotto: i PDF di tutte le richieste, delle lavorazioni esterne e quelli caricati */
const CARICATI=[];
function clienteDi(pid){const r=RICHIESTE.find(x=>x.prodotti.includes(pid)); if(r) return r.cli; const q=rfqDelProdotto(pid); return q?datiRFQ(q).cli:null;}
function archivioPdf(){
  const out=[], visti=new Set(), add=o=>{if(!o.f||!DIS_PDF(o.f)||visti.has(o.f))return; visti.add(o.f); out.push(o);};
  CARICATI.forEach(add);
  for(const pid in PROD){const D=DOCS[pid], cli=clienteDi(pid); if(!D||!cli) continue;
    const q=rfqDelProdotto(pid), r=RICHIESTE.find(x=>x.prodotti.includes(pid)), rif=q?"RFQ "+q.num:r?r.titolo:"";
    if(D.pdf&&!D.pdf.portale) add({f:D.pdf.f,s:D.pdf.s,cli,cod:PROD[pid].cod,nome:PROD[pid].nome,prod:PROD[pid].cod,rif,rev:D.pdf.rev});
    D.parti.forEach(p=>{const s=p.d2; if(s&&!s.portale) add({f:s.f,s:s.s,cli,cod:p.cod,nome:p.nome,prod:PROD[pid].cod,rif,rev:s.rev});});}
  STORICO_EST.forEach(e=>add({f:e.file,s:"",cli:e.cli,cod:e.cod,nome:e.nome,prod:e.cod,rif:e.rif+" · lavorazione esterna"}));
  return out;
}
function listaDisegni(cli){
  const qq=WF.catq.trim().toLowerCase(), fase=WF.catFase?faseDi(WF.catFase)[2]:null, gia=new Set(((fase||{}).allegati||[]).map(a=>a.f));
  const tutti=archivioPdf().filter(o=>!qq||[o.f,o.cod,o.nome,o.prod,o.rif,(cl(o.cli)||{}).nome,SETT[(cl(o.cli)||{}).sett||"altro"]?.[0]].join(" ").toLowerCase().includes(qq));
  const clienti=[...new Set(tutti.map(o=>o.cli))].sort((a,b)=>(b===cli)-(a===cli)||cl(a).nome.localeCompare(cl(b).nome));
  if(!clienti.length) return `<p class="hint" style="margin:0">Nessun disegno con questa ricerca.</p>`;
  return clienti.map(k=>{const c2=cl(k), suoi=tutti.filter(o=>o.cli===k), prodotti=[...new Set(suoi.map(o=>o.prod))];
    return `<details class="arch" ${k===cli||qq?"open":""}><summary><span class="av" style="background:${c2.col}">${c2.sigla}</span><b>${esc(c2.nome)}</b>
        <span class="k3" style="font-size:var(--t-xs)"><span class="sett" style="background:${SETT[c2.sett][1]}"></span> ${SETT[c2.sett][0]}</span><span class="sp"></span><span class="k3">${suoi.length}</span></summary>
      ${prodotti.map(pr=>`<div class="arch-p"><span class="lab">prodotto <span class="mono">${esc(pr)}</span></span>
        ${suoi.filter(o=>o.prod===pr).map(o=>`<div class="arch-f"><span class="ext pdf">PDF</span><span style="min-width:0"><span class="fn">${esc(o.f)}</span>
            <div class="k3" style="font-size:var(--t-xs)"><span class="mono">${esc(o.cod)}</span> ${esc(o.nome||"")}${o.rev?" · rev "+esc(o.rev):""} · ${esc(o.rif||"")}</div></span>
          <button class="btn sm ghost" data-dvis="${esc(o.f)}" aria-label="Guarda ${esc(o.f)}">${I("eye",13)}</button>
          ${fase?gia.has(o.f)?C("ok","allegato","check"):`<button class="btn sm pri" data-wf="allega:${esc(o.f)}">Allega</button>`:""}</div>`).join("")}</div>`).join("")}</details>`;}).join("");
}
function catalogo(cli){
  const tab=WF.cat, fase=WF.catFase?faseDi(WF.catFase):null;
  const tabs=[["disegni","Disegni"],["storico","Lavorazioni fatte"],["terzisti","Terzisti"]];
  return `<div class="backdrop" data-wf="catchiudi"></div><aside class="drawer largo" role="dialog" aria-label="Catalogo">
    <header><div style="min-width:0"><span class="lab">Catalogo</span><h3>${tab==="disegni"?"Archivio dei disegni":tab==="storico"?"Lavorazioni esterne già fatte":"Terzisti e lavorazioni"}</h3></div><span class="sp"></span>
      <button class="btn sm ghost" data-wf="catchiudi" aria-label="Chiudi">${I("x",15)}</button></header>
    <div class="dbody"><div class="seg3" role="tablist">${tabs.map(([k,l])=>`<button role="tab" data-wf="cattab:${k}" aria-selected="${tab===k}">${l}</button>`).join("")}</div>
      ${fase&&tab==="disegni"?`<div class="esito ok">${I("link",15)}<div>Stai allegando i disegni alla fase ${(fase[1]+1)*10} · ${esc(PROC[fase[2].p][0])}. I PDF sono ordinati per cliente e per prodotto.</div></div>`:""}
      ${tab!=="terzisti"?`<input id="wf-catq" class="catq" value="${esc(WF.catq)}" placeholder="${tab==="disegni"?"Cerca file, codice, prodotto, cliente, settore":"Cerca codice, cliente, lavorazione, terzista"}" aria-label="Cerca nel catalogo">`:""}
      <div id="wfcatlist" style="display:grid;gap:8px">${tab==="disegni"?listaDisegni(cli):tab==="storico"?listaStorico():listaTerzisti(cli)}</div>
      <p class="hint" style="margin:0">Il catalogo vero arriva quando il software esce dal mockup; nel backend le qualifiche dei terzisti stanno in <span class="mono">cliente_fornitore_lavorazione</span>.</p></div>
    <footer><button class="btn" data-wf="catchiudi">Chiudi</button></footer></aside>`;
}
function listaTerzisti(cli){
  return TERZISTI.map(t=>`<div class="alt" style="align-items:flex-start"><span style="min-width:0"><b>${esc(t.nome)}</b> <span class="k3" style="font-size:var(--t-xs)">${esc(t.lav)}</span>
    <div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:4px">${(t.lavs||[]).map(k=>C("neu",PROC[k]?PROC[k][0]:k==="fresatura"?"Fresatura":k)).join("")}</div>
    <div class="k3" style="font-size:var(--t-xs);margin-top:4px">${Object.keys(t.qual||{}).length?"Qualificato per: "+Object.entries(t.qual).map(([c2,l])=>esc(cl(c2)?cl(c2).nome:c2)+" ("+l.map(k=>PROC[k]?PROC[k][0].toLowerCase():k).join(", ")+")").join(" · "):"Lavora su disegno: nessuna qualifica per cliente richiesta."}</div>
    <div class="k3" style="font-size:var(--t-xs)">${plur(STORICO_EST.filter(e=>e.terz===t.id).length,"lavorazione","lavorazioni")} nello storico${(t.qual||{})[cli]?` · ${C("ok","qualificato per "+esc(cl(cli).nome))}`:""}</div></span>
    ${t.chat.length?`<button class="btn sm" data-tz="${t.id}:0">Chat ${I("chev",12)}</button>`:""}</div>`).join("");
}
function listaStorico(){
  const qq=WF.catq.trim().toLowerCase();
  const rr=STORICO_EST.filter(e=>!qq||[e.cod,e.nome,cl(e.cli).nome,PROC[e.p][0],terz(e.terz).nome,e.spec,e.rif].join(" ").toLowerCase().includes(qq));
  return rr.length?rr.map(e=>`<div class="alt" style="align-items:flex-start"><span style="min-width:0"><span class="mono" style="font-weight:600">${esc(e.cod)}</span> ${esc(e.nome)} · ${esc(cl(e.cli).nome)}
      <div>${C("acc",PROC[e.p][0])} <span class="k3" style="font-size:var(--t-xs)">${esc(terz(e.terz).nome)} · ${esc(e.quando)} · ${esc(e.prezzo)}</span></div>
      <div class="k3" style="font-size:var(--t-xs);margin-top:3px">${e.spec?esc(e.spec)+" · ":""}${e.matNostro?"materiale nostro":"materiale del terzista"} · disegno <span class="mono">${esc(e.file)}</span></div>
      <div class="k" style="font-size:var(--t-sm);margin-top:3px">${esc(e.istr)}</div></span></div>`).join("")
    :`<p class="hint" style="margin:0">Nessuna lavorazione con questa ricerca.</p>`;
}

function allegatiIniziali(pid,n){const s=slotNodo(pid,n,"d2"); return s&&!s.portale&&DIS_PDF(s.f)?[{f:s.f,s:s.s,da:"disegno del pezzo"}]:[];}
/* ---- i gesti ---- */
function azioneCiclo(a){
  const q=rf(S.rfq), d=datiRFQ(q), pid=S.rp, B=bomDi(pid), ord=inOrdine(B), i=ord.findIndex(([x])=>x.id===WF.cur), n=nodo(B,WF.cur)||nodo(B,"r");
  const vai=id=>{WF.cur=id;WF.vis={armato:false,nuovo:null,sel:null,zoom:WF.vis.zoom};WF.invio=null;WF.msg=null;};
  const [cmd,a1,a2]=a.split(":");
  if(cmd==="prev"&&i>0) vai(ord[i-1][0].id);
  if(cmd==="next"&&i<ord.length-1) vai(ord[i+1][0].id);
  if(cmd==="proponi"){proponiTipico(pid,B,n); WF.msg={ok:true,t:`<b>Ciclo tipico proposto</b> per <span class="mono">${esc(n.codice)}</span>: controllalo sul disegno.`};}
  if(cmd==="aggiungi"){const k=$("wf-nuova").value, c=cicloPer(pid,n), o={p:k};
    if(CON_FIGLI.has(k)) o.figli=Object.fromEntries(figliDi(B,n.id).filter(m=>!c.fasi.some(x=>x.figli&&x.figli[m.id]!=null)).map(m=>[m.id,m.qta]));
    if(PROC[k][1]==="est") Object.assign(o,{terz:"",spec:"",istr:"",matNostro:true});
    if(CON_MASCHERA.has(k)) o.maschera=k!=="sald_mag"&&k!=="sald_tig";
    c.fasi.push(fase(o)); tocca(c); WF.msg=null;}
  const trova=()=>{const [c,j,f]=faseDi(a1); return {c,j,f};};
  if(cmd==="su"||cmd==="giu"){const {c,j}=trova(), k=cmd==="su"?j-1:j+1; if(c&&k>=0&&k<c.fasi.length){[c.fasi[j],c.fasi[k]]=[c.fasi[k],c.fasi[j]]; tocca(c);}}
  if(cmd==="via"){const {c,j}=trova(); if(c){c.fasi.splice(j,1); tocca(c);}}
  if(cmd==="conferma"){const c=cicloDi(pid,n); c.stato="confermato"; c.fonte="confermato da Lino adesso";
    const nx=ord[i+1]; WF.msg={ok:true,t:`<b>Ciclo di <span class="mono">${esc(n.codice)}</span> confermato.</b>${nx?` Prossimo: <span class="mono">${esc(nx[0].codice)}</span>.`:" Era l’ultimo componente."}`};}
  if(cmd==="riapri"){const c=cicloDi(pid,n); c.stato="proposto"; c.fonte="riaperto da Lino"; WF.msg=null;}
  if(cmd==="usa"){const {c,f}=trova(), e=STORICO_EST.find(x=>x.id===a2);
    Object.assign(f,{terz:e.terz,spec:e.spec,istr:e.istr,matNostro:e.matNostro,daCat:e.id}); tocca(c);
    f.allegati=f.allegati||allegatiIniziali(pid,n);
    if(e.cod!==n.codice&&!f.allegati.some(x=>x.f===e.file)) f.allegati.push({f:e.file,s:"",da:"dallo storico: "+e.cod+" · "+e.quando});
    WF.msg={ok:true,t:`<b>Ripreso dallo storico:</b> ${esc(terz(e.terz).nome)}, istruzioni del ${esc(e.quando)} per <span class="mono">${esc(e.cod)}</span>. Controllale prima di inviare.`};}
  if(cmd==="prepara"){const {f}=trova(); f.allegati=f.allegati||allegatiIniziali(pid,n); WF.invio=a1;}
  if(cmd==="stacca"){const {f}=trova(); f.allegati.splice(+a2,1);}
  if(cmd==="catdis"){WF.cat="disegni"; WF.catFase=a1; WF.catq="";}
  if(cmd==="allega"){const file=a.slice(7), [,,f]=faseDi(WF.catFase), o=archivioPdf().find(x=>x.f===file);
    if(f&&o&&!(f.allegati||[]).some(x=>x.f===file)){ f.allegati=f.allegati||[]; f.allegati.push({f:o.f,s:o.s||"",da:"dall’archivio · "+cl(o.cli).nome+" · "+o.prod}); }}
  if(cmd==="carica"){WF.uplFase=a1; const inp=$("wf-upl"); if(inp){inp.value=""; inp.click();} return;}
  if(cmd==="annullainvio") WF.invio=null;
  if(cmd==="invia"){const {f}=trova(), t=terz(f.terz), s=slotNodo(pid,n,"d2");
    let ch=t.chat.find(x=>x.req===q.req&&x.cod===n.codice);
    if(!ch){ch={req:q.req,pid,cod:n.codice,nome:n.nome,stato:C("prop","bozza da inviare"),ultimo:"adesso",msg:[]}; t.chat.push(ch);}
    ch.msg.push({d:"oggi",t:"adesso",dir:"out",chi:"Lino",r:"ufficio tecnico · noi",
      txt:`<p>Buongiorno, richiesta di quotazione per <mark class="hl">${esc(PROC[f.p][0].toLowerCase())}</mark>${f.spec?" ("+esc(f.spec)+")":""} di <mark class="hl">${esc(n.codice)}</mark> ${esc(n.nome||"")}, ${esc(PROD[pid].qta)}.</p><p>${esc(f.istr||"")}</p><p>${f.matNostro!==false?"Il materiale lo forniamo noi.":"Il materiale è a vostro carico."} In allegato ${(f.allegati||[]).length===1?"il disegno":"i disegni"}${f.conNote!==false&&(NOTE[(s||{}).f]||[]).length?", con le nostre note":""}.</p>`,
      files:(f.allegati||[]).map(x=>F(x.f,"pdf",x.s||"")),meta:"bozza preparata dalla Distinta con "+plur((f.allegati||[]).length,"disegno","disegni")+(f.conNote!==false?" (note stampate sul PDF del pezzo)":"")+" e istruzioni: controllala e inviala"});
    f.inviata=true; WF.invio=null;
    if(!STORICO_EST.some(e=>e.cod===n.codice&&e.p===f.p)) STORICO_EST.unshift({id:"e"+(STORICO_EST.length+1)+"n",cod:n.codice,nome:n.nome,cli:d.cli,p:f.p,terz:f.terz,quando:"oggi",rif:"RFQ "+q.num,prezzo:"in attesa di offerta",file:s?s.f:"—",matNostro:f.matNostro!==false,spec:f.spec||"",istr:f.istr||""});
    WF.msg={ok:true,t:`<b>Bozza pronta nella chat di ${esc(t.nome)}</b> (Inbox › Terzisti), con il disegno e le istruzioni. La lavorazione entra nello storico: la prossima volta si riprende da lì.`};}
  if(cmd==="catalogo"){WF.cat="disegni"; WF.catq=""; WF.catFase=null;}
  if(cmd==="cattab"){WF.cat=a1; WF.catq="";}
  if(cmd==="catchiudi"){WF.cat=null; WF.catFase=null;}
  render();
  if(cmd==="aggiungi"){const el=document.querySelectorAll(".fase select[data-wff]"); if(el.length) el[el.length-1].focus();}
}
function azioneVistaPdf(a,e,t){
  const V=WF.vis, pid=S.rp, n=nodo(bomDi(pid),WF.cur||"r"), s=slotNodo(pid,n,"d2"), file=s&&s.f;
  if(a==="pop") return;
  if(a==="zpiu"){V.zoom=Math.min(ZOOM.length-1,V.zoom+1);return render();}
  if(a==="zmeno"){V.zoom=Math.max(0,V.zoom-1);return render();}
  if(a==="arma"){V.armato=!V.armato;V.nuovo=null;return render();}
  if(a==="carta"){ if(!V.armato){V.sel=null;return render();}
    const r=t.getBoundingClientRect(), x=r.width?Math.min(1,Math.max(0,(e.clientX-r.left)/r.width)):.5, y=r.height?Math.min(1,Math.max(0,(e.clientY-r.top)/r.height)):.5;
    V.nuovo={x,y}; V.armato=false; render(); const ta=$("wf-nota"); if(ta) ta.focus(); return; }
  if(a.startsWith("pin:")){V.sel=+a.slice(4);return render();}
  if(a.startsWith("togli:")){const id=+a.slice(6); NOTE[file]=(NOTE[file]||[]).filter(x=>x.id!==id); V.sel=null; return render();}
  if(a==="annullanota"){V.nuovo=null;return render();}
  if(a==="salvanota"){const ta=$("wf-nota"), tx=ta?ta.value.trim():""; if(!tx){if(ta)ta.focus();return;}
    (NOTE[file]=NOTE[file]||[]).push({id:prossimaNota++,x:V.nuovo.x,y:V.nuovo.y,testo:tx,autore:"Lino",quando:"adesso",mia:true,fase:($("wf-nota-fase")||{}).value||null});
    V.sel=prossimaNota-1; V.nuovo=null; return render();}
}
document.addEventListener("click",e=>{
  const t=e.target.closest("[data-wf],[data-wv],[data-wfgo],[data-dview]"); if(!t) return;
  const D=t.dataset;
  if(D.wv!==undefined) return azioneVistaPdf(D.wv,e,t);
  if(D.dview){S.dview=D.dview; WF.msg=null; return render();}
  if(D.wfgo){S.dview="ciclo"; WF.cur=D.wfgo; WF.vis={armato:false,nuovo:null,sel:null,zoom:WF.vis.zoom}; WF.invio=null; WF.msg=null; return render();}
  if(D.wf) return azioneCiclo(D.wf);
});
document.addEventListener("change",e=>{
  const el=e.target, B=S.rp&&DOCS[S.rp]?bomDi(S.rp):null; if(!B) return;
  if(el.dataset.wfp||el.dataset.wfpn){const n=nodo(B,WF.cur||"r"), c=cicloPer(S.rp,n), k=el.dataset.wfp||el.dataset.wfpn; c.prep=c.prep||{}; c.prep[k]=c.prep[k]||{on:false,nota:""};
    if(el.dataset.wfp) c.prep[k].on=el.checked; else c.prep[k].nota=el.value; tocca(c); return render();}
  if(el.dataset.wfm){const n=nodo(B,WF.cur||"r"), c=cicloPer(S.rp,n); c.mat=c.mat||{}; c.mat[el.dataset.wfm]=el.value; tocca(c); return render();}
  if(!el.dataset.wff) return;
  const [fid,campo]=el.dataset.wff.split("|"), [c,,f]=faseDi(fid); if(!f) return;
  if(campo.startsWith("figlio:")){const id=campo.slice(7); f.figli=f.figli||{};
    if(el.checked) f.figli[id]=(nodo(B,id)||{qta:1}).qta; else delete f.figli[id];}
  else if(campo.startsWith("fq:")){const v=parseInt(el.value,10); if(v>0) f.figli[campo.slice(3)]=v;}
  else if(campo==="matNostro"||campo==="conNote"||campo==="maschera") f[campo]=el.checked;
  else if(campo==="p"){ const k=el.value; f.p=k;
    if(CON_FIGLI.has(k)&&!f.figli) f.figli={};
    if(PROC[k][1]==="est"){ if(f.matNostro===undefined) f.matNostro=true; f.inviata=false; f.daCat=null; }
    if(CON_MASCHERA.has(k)&&f.maschera===undefined) f.maschera=false; }
  else { f[campo]=el.value; if(campo==="terz"){f.inviata=false; f.daCat=null;} }
  tocca(c); render();
});
document.addEventListener("input",e=>{ if(e.target.id==="wf-catq"){WF.catq=e.target.value; const l=$("wfcatlist"); if(l) l.innerHTML=WF.cat==="disegni"?listaDisegni(datiRFQ(rf(S.rfq)).cli):listaStorico();} });
/* un PDF caricato adesso: si allega alla fase ed entra nell'archivio del cliente, sotto il prodotto */
document.addEventListener("change",e=>{
  if(e.target.id!=="wf-upl"||!WF.uplFase) return;
  const file=e.target.files&&e.target.files[0], [,,f]=faseDi(WF.uplFase); if(!file||!f) return;
  const kb=file.size>1048576?(file.size/1048576).toFixed(1).replace(".",",")+" MB":Math.max(1,Math.round(file.size/1024))+" KB";
  const n=nodo(bomDi(S.rp),WF.cur||"r"), cli=datiRFQ(rf(S.rfq)).cli;
  f.allegati=f.allegati||[]; f.allegati.push({f:file.name,s:kb,da:"caricato adesso"});
  CARICATI.unshift({f:file.name,s:kb,cli,cod:n.codice,nome:n.nome,prod:PROD[S.rp].cod,rif:"caricato per la fase "+PROC[f.p][0].toLowerCase()});
  WF.uplFase=null; WF.msg={ok:true,t:`<b>Caricato ${esc(file.name)}</b>: allegato alla richiesta e salvato nell’archivio di ${esc(cl(cli).nome)}.`}; render();
});
document.addEventListener("keydown",e=>{
  if(S.tab!=="rfq"||S.step!==1||S.dview!=="ciclo"||VIS.file||WF.cat) return;
  if(/^(INPUT|SELECT|TEXTAREA)$/.test(e.target.tagName)) return;
  if(e.key==="ArrowRight"||e.key==="ArrowLeft"){e.preventDefault(); azioneCiclo(e.key==="ArrowRight"?"next":"prev");}
  if(e.key==="Escape"&&WF.vis.nuovo){WF.vis.nuovo=null;render();}
});
