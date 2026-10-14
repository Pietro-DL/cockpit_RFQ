/* ================= DISTINTA: la struttura del prodotto =================
   Riporta nel passo 2 tutte le funzioni della pagina «Distinta RFQ 990020338» del giro 4:
   l'analisi del worker, l'albero a caselle con miniature, quantità sugli archi e trascinamento,
   gli strumenti per aggiungere ed eliminare, la scheda del pezzo, il salvataggio tutto in una volta,
   il visore dei disegni con le note, l'anteprima della fattibilità e il congelamento della V1.
   Regole: la gerarchia viene dallo STEP strutturale (R68 A), l'elenco particolari del PDF è un controllo incrociato;
   sotto un particolare e sotto un commerciale non si mette niente; salvare = un'unica scrittura (rotta bom/applica). */
const TIPI={finito:"Prodotto",sottoass:"Assieme",sciolto:"Particolare",comm:"Particolare commerciale"};
const CONTENITORI=new Set(["finito","sottoass"]);
const SIMB={ok:"✓",sugg:"✓?",manca:"✗",fac:"○",nr:"—"};
const DIST={sel:"r",nuovo:null,elimina:false,scarta:false,drag:null,msg:null};
const VIS={file:null,zoom:0,armato:false,sel:null,nuovo:null};
const ZOOM=[1,1.25,1.5,2,3];
let contaNuovi=0, prossimaNota=3;

/* che cosa ha letto e deciso il worker di analisi, per prodotto (HTML statico del mockup) */
const ANALISI={
 at1:{decisioni:[
   "Lo STEP d’assieme <b class=\"mono\">9990708A_1.STP</b> ha la radice 9990708A e 4 legami: è la fonte della struttura.",
   "<b class=\"mono\">9990707A1</b> è nello STEP con quantità 1, e il suo disegno <b class=\"mono\">9990707A_1.pdf</b> non ha un elenco particolari: è un particolare, niente sotto.",
   "<b class=\"mono\">990679X1</b>, <b class=\"mono\">939552X1</b>, <b class=\"mono\">9443580X1</b> sono dadi con lo STEP del fornitore e senza disegno 2D: particolari commerciali, da confermare.",
   "Le quantità vengono dallo STEP; l’elenco particolari di <b class=\"mono\">9990708A_1.pdf</b> le conferma."],
  letti:[["ok","2D","2 disegni, di cui 1 con l’elenco particolari"],["ok","3D","5 STEP e 5 IGES, 1 STEP d’assieme con 4 legami"],
   ["neu","nomi","lo stesso pezzo sotto più nomi: 990679X_1 = 990679X1 = 990679X1_PRT"]],
  elenco:[[1,"9443580X1",1,"DADO SDPR M5"],[2,"939552X1",3,"DADO SDPR M6"],[3,"990679X1",2,"DADO M8 CL 8 PL"],[4,"9990707A1",1,"SUPPORTO"]],
  lav:[["Saldatura","ad arco continua","SPEC-SALD-023"],["Zincatura","Fe/Zn 12 III Cr3","SPEC-ZINC-013"]],toll:"UNI EN ISO 13920-BE"},
 at2:{decisioni:["Lo STEP <b class=\"mono\">9990712B1.stp</b> ha una sola radice e nessun figlio: il prodotto è un pezzo unico.",
   "Il cartiglio del PDF dice 9990712B1: coincide con il codice richiesto."],
  letti:[["ok","2D","1 disegno, senza elenco particolari"],["ok","3D","1 STEP, nessun legame"]]},
 at3:{motivo:"Lo STEP di 9990713C1 non è arrivato con la richiesta: senza STEP la struttura non si propone (R68). «Cerca di nuovo» nel passo 1 lo trova nella mail di oggi; poi «Rifai l’analisi»."},
 vb1:{decisioni:["Lo STEP <b class=\"mono\">902235-telaio_b2.step</b> ha 2 figli: 902235-01 ×2 e 902235-04 ×1.",
   "Il PDF del prodotto è alla revisione b3, lo STEP alla b2: la struttura viene dallo STEP b2 e va controllata sul disegno nuovo."],
  letti:[["ok","3D","1 STEP d’assieme con 2 legami"],["warn","2D","1 PDF in revisione b3, più recente dello STEP"]],
  elenco:[[1,"902235-01",2,"PLATINE"],[2,"902235-04",1,"RENFORT"]],
  lav:[["Saldatura","MAG; il cliente chiede le deroghe di processo su modulo firmato","—"]]},
 vb2:{decisioni:["Lo STEP <b class=\"mono\">902256-symtelaio_b2.step</b> ha una sola radice: nessun figlio rilevato."],
  letti:[["ok","3D","1 STEP"],["bad","2D","nessun PDF del prodotto"]]},
 tg1:{decisioni:["Lo STEP d’assieme <b class=\"mono\">9N007614AE.stp</b> ha 2 figli diretti: 9N008518AA ×1 e 9N008519AB ×2.",
   "<b class=\"mono\">9N008518AA</b> ha a sua volta un figlio nello STEP e un disegno con l’elenco particolari: è un assieme saldato.",
   "<b class=\"mono\">9N008520AA</b> è la piastra base sotto 9N008518AA: proposta, da confermare.",
   "<b class=\"mono\">9N008519AB</b> è un perno: particolare."],
  letti:[["ok","2D","4 disegni, 2 con l’elenco particolari"],["ok","3D","3 STEP, 1 d’assieme con 3 legami"],["ok","DXF","2 sviluppi in piano"]],
  elenco:[[1,"9N008518AA",1,"SUPPORTO SALDATO"],[2,"9N008519AB",2,"PERNO GUIDA"]],
  lav:[["Sabbiatura","capitolato a cartiglio","CAP-SAB-017"],["Verniciatura a polvere","capitolato a cartiglio","CAP-VER-024"],["Qualità del pezzo finito","capitolato","CAP-QUA-001"]],toll:"EN 22768 cL"},
 sd1:{motivo:"Lo STEP strutturale è indicato sul portale fornitori TDL e non è ancora stato scaricato: senza STEP la struttura non si propone (R68)."},
 sd2:{motivo:"Lo STEP strutturale è indicato sul portale fornitori TDL e non è ancora stato scaricato: senza STEP la struttura non si propone (R68)."},
 pl1:{verificata:true,decisioni:["Lo STEP <b class=\"mono\">92176C.stp</b> ha un figlio: 92176C-02 ×2, squadretta piegata."],
  letti:[["ok","2D","1 disegno con l’elenco particolari"],["ok","3D","1 STEP d’assieme con 1 legame"]],
  elenco:[[1,"92176C-02",2,"EQUERRE"]],
  lav:[["Verniciatura","a cartiglio, senza capitolato: quotato grezzo","—"],["Saldatura","MAG","—"]]}
};

/* note sui disegni (tabella annotazione_pdf): una nota sta su una revisione del disegno */
const NOTE={
 "9990708A_1.pdf":[{id:1,x:.296,y:.305,testo:"Nota di esempio: controllare sul 3D il passo dei tre fori per i dadi M6.",autore:"SV",quando:"oggi 09:40",mia:true}],
 "9N008518AA.pdf":[{id:2,x:.42,y:.52,testo:"Cordone d’angolo a 4 su tutto il perimetro: controllare che la torcia ci arrivi.",autore:"Lino",quando:"ieri 16:20",mia:false}]
};

/* ---- il modello: una distinta per prodotto, costruita dalle proposte e dai documenti ---- */
const BOM={};
const clona=x=>JSON.parse(JSON.stringify(x));
function rfqDelProdotto(pid){return RFQ.find(q=>datiRFQ(q).prodotti.includes(pid));}
function fontiDa(p){
  return [["neu","STEP",p.d3&&!p.d3.portale?p.d3.perche:"figlio della radice nello STEP d’assieme"],
    p.d2&&!p.d2.portale?["ok","Disegno",p.d2.perche]:null].filter(Boolean);
}
function bomDi(pid){
  if(BOM[pid]) return BOM[pid];
  const D=DOCS[pid], P=PROD[pid], q=rfqDelProdotto(pid), A=ANALISI[pid]||{}, chiusa=!!(q&&q.bomOk);
  const nodes=[{id:"r",codice:P.cod,nome:P.nome,tipo:"finito",padre:null,qta:1,proposto:false,
    fonti:[["acc","Richiesta","codice richiesto"+(q?" nella RFQ "+q.num:"")],
      D.pdf&&!D.pdf.portale?["ok","Disegno","cartiglio di "+D.pdf.f]:null,
      D.step&&!D.step.portale?["neu","STEP","radice di "+D.step.f]:null].filter(Boolean)}];
  D.parti.forEach((p,i)=>{p.pk=p.pk||pid+"-"+i;});
  D.parti.forEach(p=>{
    const su=p.sotto?D.parti.find(x=>x.cod===p.sotto):null;
    nodes.push({id:p.pk,pk:p.pk,codice:p.cod,nome:p.nome,tipo:p.tipo,padre:su?su.pk:"r",qta:p.qta,proposto:!(p.conf||chiusa),fonti:p.fonti||fontiDa(p),cat:p.cat||(p.tipo==="comm"?"commerciale":"fabbricato"),catConf:!!p.catConf});
  });
  const conStep=D.step&&!D.step.portale;
  return BOM[pid]={nodes,salvati:clona(nodes),verificata:chiusa||!!A.verificata,interni:0,analisi:A.motivo&&!D.parti.length?"senza":conStep||D.parti.length?"ok":"senza"};
}
const nodo=(B,id)=>B.nodes.find(n=>n.id===id);
const figliDi=(B,id)=>B.nodes.filter(n=>n.padre===id);
const discende=(B,a,b)=>{let x=nodo(B,b);while(x&&x.padre){if(x.padre===a)return true;x=nodo(B,x.padre);}return false;}; /* b sta sotto a */
const inOrdine=B=>{const out=[];const giu=(id,l)=>{const n=nodo(B,id);if(!n)return;out.push([n,l]);figliDi(B,id).forEach(f=>giu(f.id,l+1));};giu("r",0);return out;};
const qtaTot=(B,n)=>{let q=1,x=n;while(x&&x.padre){q*=x.qta||1;x=nodo(B,x.padre);}return q;};
const puoContenere=(B,verso,id)=>{const v=nodo(B,verso);return !!v&&CONTENITORI.has(v.tipo)&&verso!==id&&!discende(B,id,verso);};
const CAMPI=["codice","nome","tipo","padre","qta","proposto"];
function modifiche(B){
  let n=0; const m=new Map(B.salvati.map(c=>[c.id,c]));
  for(const c of B.nodes){const s=m.get(c.id); if(!s||CAMPI.some(k=>s[k]!==c[k])) n++;}
  for(const s of B.salvati) if(!nodo(B,s.id)) n++;
  return n;
}
const parteDi=(pid,n)=>n.pk?DOCS[pid].parti.find(p=>p.pk===n.pk):null;
function slotNodo(pid,n,col){
  const D=DOCS[pid];
  if(n.id==="r") return col==="d3"?D.step:col==="d2"?D.pdf:null;
  const p=parteDi(pid,n); return p?p[col]:null;
}
/* le celle 3D · 2D · DXF della casella: regole del passo 1 (R82: il commerciale non chiede il 2D) */
function cellaNodo(pid,n,col){
  if(n.id==="r"&&col==="dxf") return "nr";
  const s=slotNodo(pid,n,col);
  if(s&&!s.portale) return s.stato==="ok"?"ok":"sugg";
  if(n.id==="r") return "manca";
  const p=parteDi(pid,n)||n;
  if(richiesto(n.tipo,col,p)) return "manca";
  return n.tipo==="comm"||minuteriaConfermata(p)?"nr":"fac";
}
function bomVerificata(pid){const B=bomDi(pid); return B.verificata&&!B.nodes.some(n=>n.proposto)&&!modifiche(B);}
function bomOkRFQ(q){return !!q.bomOk||datiRFQ(q).prodotti.every(bomVerificata);}
function statoDistinta(pids){
  const Bs=pids.map(bomDi), p=Bs.reduce((n,b)=>n+b.nodes.filter(x=>x.proposto).length,0), m=Bs.reduce((n,b)=>n+modifiche(b),0);
  if(p) return ["warn",plur(p,"proposta","proposte")+" da confermare"];
  if(m) return ["warn","da salvare"];
  if(pids.every(bomVerificata)) return ["ok","verificata · "+plur(Bs.reduce((n,b)=>n+b.nodes.length,0),"pezzo","pezzi")];
  return ["warn","da confermare"];
}
/* salvare: la distinta diventa le righe dei documenti del passo 1; i file dei pezzi tolti restano, senza pezzo */
function syncParti(pid,B){
  const D=DOCS[pid], vecchie=D.parti, nuove=[];
  inOrdine(B).forEach(([n])=>{
    if(n.id==="r") return;
    let p=n.pk?vecchie.find(x=>x.pk===n.pk):null;
    if(!p){p={pk:n.id,d3:null,d2:null,dxf:null}; n.pk=n.id;}
    const su=nodo(B,n.padre);
    Object.assign(p,{cod:n.codice,nome:n.nome,tipo:n.tipo,qta:n.qta,sotto:su&&su.id!=="r"?su.codice:undefined,conf:!n.proposto,fonti:n.fonti,cat:n.cat,catConf:n.catConf});
    if(!previsto(p.tipo,"dxf")&&p.dxf){ if(!p.dxf.portale) D.liberi.push([p.dxf.f,p.cod+" è diventato commerciale: lo sviluppo non serve più",""]); p.dxf=null; }
    nuove.push(p);
  });
  vecchie.filter(p=>!nuove.includes(p)).forEach(p=>["d3","d2","dxf"].forEach(c=>{const s=p[c];
    if(s&&!s.portale) D.liberi.push([s.f,"il suo pezzo "+p.cod+" è stato tolto dalla distinta",""]);}));
  D.parti=nuove;
}
function sposta(B,id,verso){
  const c=nodo(B,id), v=nodo(B,verso);
  if(!c||!c.padre||!v||id===verso) return null;
  if(!CONTENITORI.has(v.tipo)) return {ok:false,t:`<b>${esc(v.codice)}</b> è un ${TIPI[v.tipo].toLowerCase()}: sotto non ci va niente. Mettilo sotto il prodotto o sotto un assieme.`};
  if(discende(B,id,verso)) return {ok:false,t:`Non si può: <b>${esc(v.codice)}</b> sta già sotto <b>${esc(c.codice)}</b>.`};
  if(c.padre===verso) return null;
  c.padre=verso; return {ok:true,t:`<b>${esc(c.codice)}</b> ora è sotto <b>${esc(v.codice)}</b>.`};
}

/* ---- i disegni: schizzi originali con l'impaginazione di un disegno vero (i PDF dei clienti sono riservati) ---- */
const esagono=(cx,cy,r)=>Array.from({length:6},(_,i)=>{const a=Math.PI/6+i*Math.PI/3;return (cx+r*Math.cos(a)).toFixed(1)+","+(cy+r*Math.sin(a)).toFixed(1);}).join(" ");
const pallina=(x,y,n,tx,ty)=>`<line class="f" x1="${x}" y1="${y}" x2="${tx}" y2="${ty}"/><circle class="f" cx="${tx}" cy="${ty}" r="7" fill="#fff"/><text x="${tx}" y="${ty+3}" font-size="8" text-anchor="middle">${n}</text><circle class="fill" cx="${x}" cy="${y}" r="1.4"/>`;
const cornice=(titolo,codice,scala)=>`<rect class="l" x="6" y="6" width="408" height="285"/>
  <rect class="l" x="262" y="228" width="146" height="57"/>
  <line class="f" x1="262" y1="248" x2="408" y2="248"/><line class="f" x1="262" y1="268" x2="408" y2="268"/><line class="f" x1="350" y1="268" x2="350" y2="285"/>
  <text class="tb" x="268" y="242" font-size="11">${esc(String(titolo).toUpperCase().slice(0,24))}</text>
  <text x="268" y="262" font-size="10" font-weight="600">${esc(codice)}</text><text x="370" y="262" font-size="7">REV 1</text>
  <text x="268" y="280" font-size="7">SCALA ${scala}</text><text x="356" y="280" font-size="7">TAV. 1/1</text>
  <text x="12" y="286" font-size="5.5" fill="#6a6a64">schizzo di esempio · non è il disegno del cliente</text>`;
const piastra=dadi=>`<path class="l" d="M40 44 H196 L204 52 V156 L196 164 H40 L32 156 V52 Z"/>
  <line class="h" x1="32" y1="146" x2="204" y2="146"/>
  <line class="c" x1="118" y1="36" x2="118" y2="172"/><line class="c" x1="24" y1="104" x2="212" y2="104"/>
  ${[[52,58],[184,58],[52,132],[184,132]].map(([x,y])=>`<circle class="l" cx="${x}" cy="${y}" r="5"/><line class="c" x1="${x-9}" y1="${y}" x2="${x+9}" y2="${y}"/><line class="c" x1="${x}" y1="${y-9}" x2="${x}" y2="${y+9}"/>`).join("")}
  <rect class="l" x="92" y="112" width="52" height="22" rx="3"/>${dadi}
  <path class="f" d="M40 180 V188 M204 180 V188 M40 185 H204"/><path class="fill" d="M40 185 l6 -2 v4 z M204 185 l-6 -2 v4 z"/><text x="122" y="182" font-size="8" text-anchor="middle">170</text>
  <path class="f" d="M22 44 H14 M22 164 H14 M18 44 V164"/><path class="fill" d="M18 44 l-2 6 h4 z M18 164 l-2 -6 h4 z"/><text x="12" y="108" font-size="8" transform="rotate(-90 12 108)" text-anchor="middle">120</text>
  <rect class="l" x="228" y="60" width="7" height="104"/><rect class="l" x="228" y="157" width="28" height="7"/>
  <text x="231" y="54" font-size="6.5" text-anchor="middle">VISTA A</text>`;
const svgApri=(lab)=>`<svg class="dw" viewBox="0 0 420 297" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="${esc(lab)}"><rect width="420" height="297" fill="#fff"/>`;
function disegnoProdotto(){
  return svgApri("Disegno d’assieme 9990708A1, schizzo di esempio")+cornice("Supporto batteria","9990708A1","1:2")
    +piastra(`<polygon class="l" points="${esagono(118,70,7)}"/><circle class="f" cx="118" cy="70" r="3"/>
      ${[80,118,156].map(x=>`<polygon class="l" points="${esagono(x,96,8)}"/><circle class="f" cx="${x}" cy="96" r="3.5"/>`).join("")}
      ${[64,172].map(x=>`<polygon class="l" points="${esagono(x,150,9)}"/><circle class="f" cx="${x}" cy="150" r="4.5"/>`).join("")}`)
    +pallina(125,66,1,150,30)+pallina(160,92,2,186,30)+pallina(172,150,3,218,186)+pallina(40,120,4,22,206)
    +`<rect class="l" x="262" y="12" width="146" height="66"/><text class="tb" x="268" y="23" font-size="7.5">ELENCO PARTICOLARI</text>
    <line class="f" x1="262" y1="27" x2="408" y2="27"/>
    <text x="266" y="35" font-size="5.5">POS</text><text x="282" y="35" font-size="5.5">CODICE</text><text x="322" y="35" font-size="5.5">QTÀ</text><text x="338" y="35" font-size="5.5">DENOMINAZIONE</text>
    <line class="f" x1="262" y1="38" x2="408" y2="38"/>
    ${[["1","9443580X1","1","DADO SDPR M5"],["2","939552X1","3","DADO SDPR M6"],["3","990679X1","2","DADO M8 CL 8"],["4","9990707A1","1","SUPPORTO"]].map((r,i)=>`<text x="268" y="${47+i*9}" font-size="6">${r[0]}</text><text x="282" y="${47+i*9}" font-size="6">${r[1]}</text><text x="325" y="${47+i*9}" font-size="6">${r[2]}</text><text x="338" y="${47+i*9}" font-size="6">${r[3]}</text>`).join("")}
    <text x="30" y="218" font-size="6.5">1 - SALDATURA AD ARCO CONTINUA</text><text x="30" y="228" font-size="6.5">2 - ZINCATURA FE/ZN 12</text></svg>`;
}
function disegnoSupporto(){
  return svgApri("Disegno del particolare 9990707A1, schizzo di esempio")+cornice("Supporto","9990707A1","1:2")
    +piastra(`<circle class="l" cx="118" cy="70" r="3"/>${[80,118,156].map(x=>`<circle class="l" cx="${x}" cy="96" r="3.5"/>`).join("")}${[64,172].map(x=>`<circle class="l" cx="${x}" cy="150" r="4.5"/>`).join("")}`)
    +`<text x="126" y="64" font-size="6.5">Ø6</text><text x="164" y="90" font-size="6.5">3× Ø7</text><text x="180" y="146" font-size="6.5">2× Ø9</text>
    <text x="244" y="112" font-size="6.5">SP. 3</text><text x="244" y="152" font-size="6.5">R 3</text>
    <text x="30" y="218" font-size="6.5">LAMIERA S235JR · SPESSORE 3</text><text x="30" y="228" font-size="6.5">SPIGOLI VIVI SMUSSATI</text></svg>`;
}
/* per gli altri disegni: tre sagome generiche (piastra forata, staffa piegata, pezzo tornito), scelte dal nome del pezzo */
function disegnoGenerico(nome,cod,f){
  const nm=String(nome||"");
  const v=/perno|boccola|albero|tornit|bussola|pin\b/i.test(nm)?2:/staffa|squadr|piegat|equerre|supporto faro|parafango|carter/i.test(nm)?1:0;
  const corpo=v===0
    ?`<rect class="l" x="44" y="50" width="160" height="110" rx="6"/><line class="c" x1="124" y1="40" x2="124" y2="170"/><line class="c" x1="34" y1="105" x2="214" y2="105"/>
      ${[[64,70],[184,70],[64,140],[184,140]].map(([x,y])=>`<circle class="l" cx="${x}" cy="${y}" r="6"/>`).join("")}<circle class="l" cx="124" cy="105" r="16"/>
      <path class="f" d="M44 182 V190 M204 182 V190 M44 186 H204"/><text x="124" y="183" font-size="8" text-anchor="middle">160</text>
      <rect class="l" x="232" y="50" width="6" height="110"/><text x="235" y="44" font-size="6.5" text-anchor="middle">SEZ. A-A</text>`
    :v===1
    ?`<path class="l" d="M40 60 H180 V72 H52 V170 H40 Z"/><path class="h" d="M52 72 L60 64"/><line class="c" x1="34" y1="66" x2="190" y2="66"/>
      <circle class="l" cx="120" cy="66" r="0"/>${[80,140].map(x=>`<circle class="l" cx="${x}" cy="66" r="3"/>`).join("")}
      <text x="190" y="64" font-size="7">90°</text><text x="58" y="130" font-size="7">R 4</text>
      <rect class="l" x="228" y="60" width="120" height="70"/><text x="288" y="54" font-size="6.5" text-anchor="middle">SVILUPPO</text>
      ${[260,316].map(x=>`<circle class="l" cx="${x}" cy="95" r="3"/>`).join("")}<line class="h" x1="228" y1="80" x2="348" y2="80"/>`
    :`<rect class="l" x="50" y="80" width="150" height="40"/><rect class="l" x="200" y="88" width="40" height="24"/><line class="c" x1="40" y1="100" x2="252" y2="100"/>
      <path class="f" d="M50 72 V64 M240 72 V64 M50 68 H240"/><text x="145" y="64" font-size="8" text-anchor="middle">190</text>
      <text x="60" y="140" font-size="7">Ø40 h9</text><text x="206" y="130" font-size="7">Ø24</text><path class="h" d="M70 80 L60 90 M190 80 L200 90"/>`;
  return svgApri("Disegno di "+cod+", schizzo di esempio")+cornice(nome||cod,cod,"1:2")+corpo
    +`<text x="30" y="218" font-size="6.5">MATERIALE E TRATTAMENTO: VEDI CARTIGLIO</text><text x="30" y="228" font-size="6.5">TOLLERANZE GENERALI A CARTIGLIO</text></svg>`;
}
const DIS_SPEC={"9990708A_1.pdf":disegnoProdotto,"9990707A_1.pdf":disegnoSupporto};
const svgDisegno=(f,cod,nome)=>(DIS_SPEC[f]?DIS_SPEC[f]():disegnoGenerico(nome,cod,f));
const ICONA_DADO=`<svg viewBox="0 0 40 40" aria-hidden="true"><polygon points="${esagono(20,20,15)}" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="20" cy="20" r="6.5" fill="none" stroke="currentColor" stroke-width="2"/></svg>`;
/* i disegni 2D della RFQ, per la tendina del visore */
function pdfDellaRFQ(){
  const q=rf(S.rfq); if(!q) return [];
  const out=[], visti=new Set();
  for(const pid of datiRFQ(q).prodotti){
    const D=DOCS[pid], B=bomDi(pid);
    for(const [n] of inOrdine(B)){const s=slotNodo(pid,n,"d2");
      if(s&&!s.portale&&/\.pdf$/i.test(s.f)&&!visti.has(s.f)){visti.add(s.f);out.push({f:s.f,cod:n.codice,nome:n.nome,tipo:n.tipo});}}
    if(D.pdf&&!D.pdf.portale&&!visti.has(D.pdf.f)){visti.add(D.pdf.f);out.push({f:D.pdf.f,cod:PROD[pid].cod,nome:PROD[pid].nome,tipo:"finito"});}
  }
  return out;
}

/* ---- il passo 2 ---- */
function passoDistinta(q,d,c){
  const pid=S.rp, B=bomDi(pid), D=DOCS[pid];
  if(!nodo(B,DIST.sel)) DIST.sel="r";
  const nProp=B.nodes.filter(n=>n.proposto).length, m=modifiche(B);
  return `<div class="rfq largo${S.dview==="ciclo"?" ciclo":""}">
    ${treStati(q,stat(d.prodotti))}
    ${d.prodotti.length>1?`<div class="ptabs" role="tablist">${d.prodotti.map(p=>{const b=bomDi(p),np=b.nodes.filter(x=>x.proposto).length,mm=modifiche(b);
      return `<button class="ptab" role="tab" data-rp="${p}" aria-selected="${p===pid}"><span><span class="mono">${PROD[p].cod}</span> <span class="k">${esc(PROD[p].nome)}</span>
        <div class="k3" style="font-size:var(--t-xs)">${plur(b.nodes.length,"pezzo","pezzi")}${np?" · "+plur(np,"proposta","proposte"):""}${mm?" · da salvare":""}</div></span>
        ${bomVerificata(p)?C("ok","verificata","check"):C(np||mm?"prop":"neu",np?np+"":"da confermare")}</button>`;}).join("")}</div>`:""}
    ${vistaDistinta(pid)}
    ${S.dview==="ciclo"?passoCiclo(q,d,pid,B)+`</div>${visore()}`:corpoStruttura(q,d,pid,B,D,nProp,m)}`;
}
function vistaDistinta(pid){
  const [ok,tot]=contaCicli(pid), ciclo=S.dview==="ciclo";
  return `<div class="dviste" role="tablist" aria-label="Viste della distinta">
    <button role="tab" data-dview="struttura" aria-selected="${!ciclo}">${I("list",15)}<span><b>Struttura</b><span class="k3">${esc(statoDistinta([pid])[1])}</span></span></button>
    <button role="tab" data-dview="ciclo" aria-selected="${ciclo}">${I("wrench",15)}<span><b>Ciclo di produzione</b><span class="k3">${ok}/${tot} cicli confermati</span></span></button></div>`;
}
function corpoStruttura(q,d,pid,B,D,nProp,m){
  return `${DIST.msg?`<div class="esito ${DIST.msg.ok?"ok":"no"}" role="status">${I(DIST.msg.ok?"check":"alert",16)}<div>${DIST.msg.t}</div></div>`:""}
    ${boxAnalisi(pid,B,D)}
    <div class="strumenti" role="toolbar" aria-label="Strumenti della distinta">
      <button class="btn t-ass" data-dnuovo="sottoass">${I("plus",14)} Assieme</button>
      <button class="btn t-par" data-dnuovo="sciolto">${I("plus",14)} Particolare</button>
      <button class="btn t-com" data-dnuovo="comm">${I("plus",14)} Particolare commerciale</button>
      <span class="sep"></span>
      <button class="btn danger" data-dx="eliminasel">${I("trash",14)} Elimina il selezionato</button>
      <span class="sp"></span>
      <span class="k" style="font-size:var(--t-sm)">${nProp?plur(nProp,"casella proposta","caselle proposte")+", da confermare":"Tutte le caselle sono confermate"}</span></div>
    ${DIST.nuovo?formNuovo(pid,B):""}
    <div class="strgrid"><div class="strmain"><div><div class="tela" aria-label="Schema della distinta"><div class="dtree"><ul>${liNodo(pid,B,nodo(B,"r"))}</ul></div></div>
      <div class="legenda"><span><i></i>confermato</span><span><i class="tratt"></i>proposto dall’analisi</span>
        <span class="tipo finito">Prodotto</span><span class="tipo sottoass">Assieme</span><span class="tipo sciolto">Particolare</span><span class="tipo comm">Particolare commerciale</span>
        <span>· clic sulla miniatura: si apre il disegno · trascina una casella sopra il prodotto o un assieme per metterla sotto</span></div></div>
    <div class="regola">${I("alert",15)}<span>Sotto un particolare e sotto un particolare commerciale non si mette niente: i pezzi stanno solo sotto il prodotto e sotto gli assiemi.</span></div>
    </div><div class="strside">${dettaglio(pid,B)}
    ${barraSalva(B,m,nProp)}</div></div>
    ${fattibilita(q,d)}
    ${noteProgetto()}
  </div>${visore()}`;
}
function boxAnalisi(pid,B,D){
  const A=ANALISI[pid]||{};
  if(B.analisi==="senza") return `<div class="fs" style="background:var(--surf2)"><header>${I("cube",17)}<h3>Struttura non proposta</h3>${C("neu","nessuna analisi")}<span class="sp"></span>
      <button class="btn sm" data-dx="rifai">${I("refresh",13)} Rifai l’analisi</button></header>
      <p class="hint" style="margin:0">${esc(A.motivo||"L’analisi non ha trovato uno STEP strutturale: senza STEP la struttura non si propone, e nessuna struttura viene inventata (R68).")}</p>
      <p class="hint" style="margin:0">Puoi costruirla a mano con i pulsanti qui sotto, partendo dal prodotto, oppure chiedere al cliente il 3D.</p></div>`;
  const dec=A.decisioni||[`Lo STEP <b class="mono">${esc(D.step?D.step.f:"")}</b> ha una sola radice e nessun figlio: il prodotto è un pezzo unico.`];
  const letti=A.letti||[["ok","3D","1 STEP, nessun legame"]];
  const n={sottoass:0,sciolto:0,comm:0}; B.nodes.forEach(x=>{if(x.tipo in n)n[x.tipo]++;});
  const prop=B.nodes.some(x=>x.proposto);
  return `<div class="fs analisi-box"><header>${I("check",17)}<h3>Analisi del prodotto</h3>${C("ok","completata dal worker di analisi","check")}<span class="sp"></span>
      <button class="btn sm" data-dx="rifai">${I("refresh",13)} Rifai l’analisi</button>
      ${prop?`<button class="btn sm ghost" data-dx="scarta">Scarta la proposta</button><button class="btn sm pri" data-dx="accetta">${I("check",13)} Accetta la struttura proposta</button>`:""}</header>
    ${DIST.scarta&&prop?`<div class="avviso-conf"><span>Scartare le caselle proposte dall’analisi? Quelle confermate restano; i pezzi confermati sotto una casella scartata salgono di un livello.</span>
      <span style="display:flex;gap:8px"><button class="btn danger" data-dx="scartaok">Sì, scarta</button><button class="btn" data-dx="scartano">Annulla</button></span></div>`:""}
    <div class="analisi"><div><span class="lab">Come ha deciso</span><ul class="decisioni">${dec.map(x=>`<li><span>${x}</span></li>`).join("")}</ul></div>
      <div><span class="lab">Che cosa ha letto</span><ul class="letti">${letti.map(([k,t,x])=>`<li>${C(k,t)}<span>${esc(x)}</span></li>`).join("")}</ul>
        <div class="conta-tipi"><span class="tipo finito">1 prodotto</span><span class="tipo sottoass">${plur(n.sottoass,"assieme","assiemi")}</span>
          <span class="tipo sciolto">${plur(n.sciolto,"particolare","particolari")}</span><span class="tipo comm">${plur(n.comm,"commerciale","commerciali")}</span></div>
        ${A.elenco&&A.elenco.length?`<details style="margin-top:10px"><summary class="k" style="cursor:pointer;font-size:var(--t-sm)">L’elenco particolari letto dal disegno · controllo incrociato</summary>
          <div class="wrap-tb" style="margin-top:8px;overflow-x:auto"><table class="tb"><thead><tr><th>Pos.</th><th>Codice</th><th>Qtà</th><th>Denominazione</th></tr></thead>
          <tbody>${A.elenco.map(([p,cod,qt,den])=>`<tr><td class="mono">${p}</td><td class="mono">${esc(cod)}</td><td class="mono">${qt}</td><td>${esc(den)}</td></tr>`).join("")}</tbody></table></div></details>`:""}</div></div></div>`;
}
function formNuovo(pid,B){
  const t=DIST.nuovo, s=nodo(B,DIST.sel), pref=s&&CONTENITORI.has(s.tipo)?s.id:"r";
  const cod=t==="sottoass"?PROD[pid].cod+"-A"+String(B.interni+1).padStart(2,"0"):"";
  return `<div class="fs" style="background:var(--surf2)"><header><span class="lab">Nuovo ${TIPI[t].toLowerCase()}</span></header>
    <div class="form-nuovo">
      <label class="fld mono"><span>Codice</span><input id="dn-cod" maxlength="60" value="${esc(cod)}"><small>${t==="sottoass"?"Codice interno proposto: lo puoi cambiare.":"Il codice del cliente. Se non c’è, scrivi un codice interno."}</small></label>
      <label class="fld"><span>Denominazione</span><input id="dn-nome" maxlength="120" placeholder="per esempio: Staffa laterale"></label>
      <label class="fld"><span>Sotto</span><select id="dn-padre">${B.nodes.filter(n=>CONTENITORI.has(n.tipo)).map(n=>`<option value="${n.id}" ${n.id===pref?"selected":""}>${TIPI[n.tipo]} ${esc(n.codice)}</option>`).join("")}</select><small>Solo il prodotto o un assieme</small></label>
      <label class="fld"><span>Quantità</span><input id="dn-qta" type="number" min="1" value="1"></label>
      <div style="display:flex;gap:8px"><button class="btn pri" data-dx="aggiungi">Aggiungi</button><button class="btn" data-dx="annullanuovo">Annulla</button></div></div></div>`;
}
function liNodo(pid,B,n){const f=figliDi(B,n.id); return `<li>${nodoBox(pid,B,n)}${f.length?`<ul>${f.map(x=>liNodo(pid,B,x)).join("")}</ul>`:""}</li>`;}
function nodoBox(pid,B,n){
  const pdf=slotNodo(pid,n,"d2"), haPdf=!!(pdf&&!pdf.portale&&/\.pdf$/i.test(pdf.f)), nN=haPdf?(NOTE[pdf.f]||[]).length:0;
  const s3=slotNodo(pid,n,"d3"), solo3d=!!(s3&&!s3.portale);
  const mini=haPdf
    ?`<button class="miniatura" data-dvis="${esc(pdf.f)}" aria-label="Apri il disegno ${esc(pdf.f)}" title="Apri il disegno ${esc(pdf.f)}">${svgDisegno(pdf.f,n.codice,n.nome)}<span class="apri">Apri il disegno</span>${nN?`<span class="note-mini">✎ ${nN}</span>`:""}</button>`
    :`<div class="miniatura vuota">${n.tipo==="comm"?ICONA_DADO:""}<span>${solo3d?"Nessun disegno 2D · solo 3D":"Nessun disegno"}${n.tipo==="comm"?"<br>per un particolare commerciale non serve":""}</span></div>`;
  const TIT={ok:"confermato",sugg:"riconosciuto, da confermare",manca:"manca (obbligatorio)",fac:"manca (facoltativo)"};
  const docs=["d3","d2","dxf"].map(col=>{const s=cellaNodo(pid,n,col); return s==="nr"?"":`<span class="cella ${s}" title="${COL[col]}: ${TIT[s]}">${COL[col]} ${SIMB[s]}</span>`;}).join("");
  return `<div class="nodo-box">${n.padre?`<span class="qta-arco" title="quantità sotto ${esc(nodo(B,n.padre).codice)}">×${n.qta}</span>`:""}
    <div class="nodo ${n.tipo}${n.proposto?" proposto":""}${n.id===DIST.sel?" sel":""}" tabindex="0" role="button" data-dsel="${n.id}" draggable="${n.padre?"true":"false"}"
      aria-label="${TIPI[n.tipo]} ${esc(n.codice)} ${esc(n.nome||"")}${n.proposto?", proposto":""}">
      <span class="nodo-testa"><span class="tipo ${n.tipo}">${TIPI[n.tipo]}</span>${n.proposto?`<span class="proposto-tag">da confermare</span>`:""}</span>
      ${n.cat==="minuteria"?`<span class="cat-tag${(parteDi(pid,n)||n).catConf?" ok":""}">minuteria ${(parteDi(pid,n)||n).catConf?"confermata":"proposta"}</span>`:""}
      <span class="cod">${esc(n.codice)}</span>${n.nome?`<span class="nome">${esc(n.nome)}</span>`:""}
      ${mini}<span class="docs">${docs}${(()=>{const st=statoCiclo(pid,n);return `<button class="cella ciclo ${st}" data-wfgo="${n.id}" title="Ciclo di produzione: ${CHIP_CICLO[st][1]}">${st==="acquisto"?"da acquistare":"ciclo "+(st==="confermato"?"✓":st==="proposto"?"✓?":"○")}</button>`;})()}</span></div></div>`;
}
function dettaglio(pid,B){
  const n=nodo(B,DIST.sel);
  if(!n) return `<div class="fs"><p class="hint" style="margin:0">Scegli una casella nello schema per vederne i dati.</p></div>`;
  const pdf=slotNodo(pid,n,"d2"), haPdf=!!(pdf&&!pdf.portale&&/\.pdf$/i.test(pdf.f)), nN=haPdf?(NOTE[pdf.f]||[]).length:0;
  const cand=B.nodes.filter(x=>puoContenere(B,x.id,n.id)), sotto=B.nodes.filter(x=>discende(B,n.id,x.id)).length;
  return `<div class="fs dett${n.proposto?" proposto":""}" aria-live="polite"><header><span class="lab">${n.proposto?"Casella proposta · da confermare":"Casella scelta"}</span>
      <span class="tipo ${n.tipo}">${TIPI[n.tipo]}</span><b class="mono">${esc(n.codice)}</b><span class="sp"></span>
      ${n.padre?`<span class="k3" style="font-size:var(--t-sm)">quantità totale per prodotto: <b class="mono">${qtaTot(B,n)}</b></span>`:""}</header>
    <div class="dettaglio-griglia"><div style="display:grid;gap:10px">
      <label class="fld mono"><span>Codice</span><input id="dd-cod" maxlength="60" value="${esc(n.codice)}"></label>
      <label class="fld"><span>Denominazione</span><input id="dd-nome" maxlength="120" value="${esc(n.nome||"")}"></label>
      <label class="fld"><span>Tipo</span><select id="dd-tipo" ${n.tipo==="finito"?"disabled":""}>${Object.entries(TIPI).filter(([k])=>n.tipo==="finito"||k!=="finito").map(([k,v])=>`<option value="${k}" ${k===n.tipo?"selected":""}>${v}</option>`).join("")}</select>
        <small>${n.tipo==="finito"?"È il prodotto della richiesta.":"Solo il prodotto e gli assiemi possono avere pezzi sotto."}</small></label>
      ${n.padre?`<div class="due"><label class="fld"><span>Categoria</span><select id="dd-cat">${[["fabbricato","fabbricato"],["commerciale","commerciale"],["minuteria","minuteria"]].map(([k,v])=>`<option value="${k}" ${(n.cat||"fabbricato")===k?"selected":""}>${v}</option>`).join("")}</select>
          <small>distinta dal tipo: dice che cosa è il pezzo</small></label>
        <label class="fld"><span>Confermata</span><input type="checkbox" id="dd-catconf" ${(parteDi(pid,n)||n).catConf?"checked":""} style="width:auto;margin-top:8px"></label></div>
        ${n.cat==="minuteria"?`<p class="k3" style="margin:0;font-size:var(--t-xs)">${(parteDi(pid,n)||n).catConf?"Minuteria confermata: il 2D non è richiesto (l’unica esenzione, R103 C).":"Minuteria solo proposta: il 2D resta richiesto finché qualcuno non la conferma."}</p>`:""}
      <div class="due"><label class="fld"><span>Sotto</span><select id="dd-padre">${cand.map(x=>`<option value="${x.id}" ${x.id===n.padre?"selected":""}>${TIPI[x.tipo]} ${esc(x.codice)}</option>`).join("")}</select></label>
        <label class="fld"><span>Quantità</span><input id="dd-qta" type="number" min="1" value="${n.qta}"></label></div>`:""}
    </div><div style="display:grid;gap:12px;align-content:start">
      <div><span class="lab">Da dove viene</span><ul class="fonti dfonti">${(n.fonti||[]).map(([k,a,b])=>`<li>${C(k,a)}<span>${esc(b)}</span></li>`).join("")}</ul></div>
      <div class="acts" style="margin:0">${haPdf?`<button class="btn" data-dvis="${esc(pdf.f)}">${I("eye",14)} Apri il disegno${nN?" (✎ "+nN+")":""}</button>`:""}
        <button class="btn" data-wfgo="${n.id}">${I("wrench",14)} Ciclo di produzione</button>
        ${n.proposto?`<button class="btn pri" data-dx="conferma">${I("check",14)} Conferma</button>`:""}
        ${n.padre?`<button class="btn danger" data-dx="elimina">${I("trash",14)} Elimina</button>`:""}</div>
      ${DIST.elimina&&n.padre?`<div class="avviso-conf"><span>Eliminare <b class="mono">${esc(n.codice)}</b>${sotto?` e ${plur(sotto,"il pezzo che ha sotto","i "+sotto+" pezzi che ha sotto")}?`:"?"} I suoi file restano, senza pezzo.</span>
        <span style="display:flex;gap:8px"><button class="btn danger" data-dx="eliminaok">Sì, elimina</button><button class="btn" data-dx="eliminano">Annulla</button></span></div>`:""}
    </div></div></div>`;
}
function barraSalva(B,m,nProp){
  if(m) return `<div class="salva-barra"><span><b>${m}</b> ${m===1?"modifica non ancora salvata":"modifiche non ancora salvate"}</span>
    <span style="display:flex;gap:8px;flex-wrap:wrap"><button class="btn" data-dx="annullatutto">Annulla le modifiche</button><button class="btn pri" data-dx="salva">${I("check",14)} Salva la distinta</button></span></div>`;
  if(B.verificata&&!nProp) return `<div class="salva-barra ok">${I("check",16)}<span><b>Distinta verificata.</b> Salvata e senza proposte aperte: la BOM di questo prodotto è chiusa.</span></div>`;
  if(!nProp) return `<div class="salva-barra acc"><span>Tutte le caselle sono confermate. Conferma la distinta per chiudere la verifica della BOM.</span>
    <button class="btn pri" data-dx="salva">${I("check",14)} Conferma la distinta</button></div>`;
  return "";
}
function fattibilita(q,d){
  const multi=d.prodotti.length>1, prod=[], comp=[];
  d.prodotti.forEach(p=>{const B=bomDi(p); inOrdine(B).forEach(([n])=>(n.tipo==="comm"?comp:prod).push([p,B,n]));});
  const riga=([p,B,n],extra)=>`<div class="dl"><span><span class="mono">${esc(n.codice)}</span> <span class="tipo ${n.tipo}">${TIPI[n.tipo]}</span></span>
    <span class="k3" style="font-size:var(--t-sm)">${esc(n.nome||"")} · ×${qtaTot(B,n)}${extra}${multi?" · "+esc(PROD[p].cod):""}</span></div>`;
  const lav=d.prodotti.flatMap(p=>((ANALISI[p]||{}).lav||[]).map(x=>[p,...x]));
  const toll=d.prodotti.map(p=>(ANALISI[p]||{}).toll&&[p,ANALISI[p].toll]).filter(Boolean);
  const tz=TERZISTI.flatMap(t=>t.chat.map((ch,i)=>[t,ch,i]).filter(([,ch])=>d.prodotti.includes(ch.pid)));
  return `<div class="sezione-titolo"><h2>Verso la fattibilità</h2><span class="k" style="font-size:var(--t-ms)">Che cosa si produce, che cosa si compra, quali lavorazioni chiede il disegno. Qui nascono gli assiemi interni, con un codice automatico che si può cambiare.</span></div>
  <div class="g3">
    <div class="fs"><header><span class="lab">Da produrre</span><span class="sp"></span><span class="k3">${prod.length}</span></header>${prod.map(x=>riga(x,"")).join("")||`<p class="hint" style="margin:0">Niente da produrre.</p>`}</div>
    <div class="fs"><header><span class="lab">Da comprare, per un prodotto</span><span class="sp"></span><span class="k3">${comp.length}</span></header>${comp.map(x=>riga(x," per prodotto")).join("")||`<p class="hint" style="margin:0">Nessun particolare commerciale nella distinta.</p>`}</div>
    <div class="fs"><header><span class="lab">Lavorazioni dal disegno</span></header>
      ${lav.map(([p,a,b,sp])=>`<div class="dl"><span><b style="font-weight:600">${esc(a)}</b> ${esc(b)}</span>${sp&&sp!=="—"?`<span class="mono k3" style="font-size:var(--t-xs)">specifica ${esc(sp)} · su ${esc(PROD[p].cod)}</span>`:`<span class="k3" style="font-size:var(--t-xs)">su ${esc(PROD[p].cod)}</span>`}</div>`).join("")||`<p class="hint" style="margin:0">Non indicate nei disegni letti.</p>`}
      ${toll.map(([p,t])=>`<div class="dl"><span><b style="font-weight:600">Tolleranze</b> <span class="mono">${esc(t)}</span></span></div>`).join("")}
      ${tz.map(([t,ch,i])=>`<div class="dl" style="margin-top:4px"><span>${C("acc",t.nome,"factory")} <span class="k3" style="font-size:var(--t-xs)">${esc(t.lav)}</span></span>
        <span style="display:flex;gap:6px;align-items:center;flex-wrap:wrap">${ch.stato}<button class="btn sm ghost" data-tz="${t.id}:${i}">Apri la chat ${I("chev",12)}</button></span></div>`).join("")}</div></div>
  <div class="g2">
    <div class="fs" style="background:var(--surf2)"><header>${I("wrench",17)}<h3>Ciclo di produzione</h3><span class="sp"></span>${(()=>{const t=d.prodotti.map(contaCicli).reduce((a,b)=>[a[0]+b[0],a[1]+b[1]],[0,0]);return C(t[0]===t[1]?"ok":"prop",t[0]+"/"+t[1]+" cicli confermati");})()}</header>
      <p class="hint" style="margin:0">Per ogni componente, dal prodotto ai figli: materiale, fasi in ordine, dove entrano figli e commerciali, maschere, robot o manuale, lavorazioni esterne dal catalogo dei terzisti.</p>
      <div><button class="btn pri" data-dview="ciclo">${I("wrench",14)} Apri il ciclo di produzione</button></div></div>
    <div class="fs"><header>${I("list",17)}<h3>Congela la distinta</h3></header>
      <p class="hint" style="margin:0">La prima versione (V1) si congela all’inizio della fattibilità. Da lì in poi la distinta cambia solo aprendo una revisione.</p>
      <div><button class="btn" disabled title="Si congela in fattibilità">Congela la V1</button> <span class="k3" style="font-size:var(--t-sm)">· si fa quando parte la fattibilità</span></div></div></div>`;
}
function noteProgetto(){
  return `<details class="nota-prog"><summary>Note di progetto (per noi, non per l’operatore)</summary><ul>
    <li>La struttura la propone il worker di analisi dallo STEP strutturale (R68 A). L’elenco particolari del PDF si mostra solo come controllo incrociato.</li>
    <li>Le caselle tratteggiate sono le proposte (<span class="mono">componente_proposta</span>, <span class="mono">relazione_proposta</span>, 0018). «Accetta la struttura proposta» e «Salva la distinta» corrispondono a «Conferma l’albero» (<span class="mono">albero_conferma.go</span>) e alla rotta <span class="mono">bom/applica</span>, tutto in una volta.</li>
    <li>Da cambiare nel backend: oggi l’editor trasforma un particolare in assieme quando gli si mette qualcosa sotto; la regola nuova lo rifiuta.</li>
    <li>Da aggiungere: il riconoscimento dello stesso pezzo sotto nomi diversi (<span class="mono">X_1</span>, <span class="mono">X1</span>, <span class="mono">X1_PRT</span>, la radice dello STEP senza la cifra della versione).</li>
    <li>Completezza (R82, S1): il commerciale non chiede il 2D; la minuteria confermata nemmeno (T-E1-18). La categoria fabbricato, commerciale o minuteria è un calcolo (<span class="mono">Classificazione</span>), non un tipo nuovo del DB.</li>
    <li>Un prodotto senza figli oggi risulta «non verificabile» (R71, aperta): qui la verifica si chiude con «Conferma la distinta».</li>
    <li>Congelamento: <span class="mono">bom_versione</span> per RFQ (0020); il gesto del modello nuovo non c’è ancora (LD-23).</li>
    <li>Miniature e visore usano il visore PDF e le note di oggi (<span class="mono">annotazione_pdf</span>). I disegni qui sono schizzi di esempio: i PDF veri dei clienti sono riservati.</li></ul></details>`;
}
function visore(){
  if(!VIS.file) return "";
  const files=pdfDellaRFQ(), arch=typeof archivioPdf==="function"?archivioPdf().find(x=>x.f===VIS.file):null;
  const cur=files.find(x=>x.f===VIS.file)||(arch?{f:arch.f,cod:arch.cod,nome:arch.nome,tipo:"sciolto"}:{f:VIS.file,cod:"",nome:"",tipo:"sciolto"});
  if(!files.some(x=>x.f===VIS.file)) files.unshift(cur);
  const lista=NOTE[VIS.file]||[], z=ZOOM[VIS.zoom];
  return `<div class="visore" data-dvx="sfondo"><div class="visore-box" role="dialog" aria-modal="true" aria-label="Disegno ${esc(VIS.file)}">
    <div class="visore-barra"><div class="titolo"><b>${esc(VIS.file)}</b><span class="k" style="font-size:var(--t-sm)">${TIPI[cur.tipo]} ${esc(cur.cod)} · ${esc(cur.nome)} · disegno 2D · pagina 1 di 1</span></div>
      <select id="vis-file" aria-label="Disegno da vedere">${files.map(x=>`<option value="${esc(x.f)}" ${x.f===VIS.file?"selected":""}>${esc(x.f)} · ${esc(x.nome)}</option>`).join("")}</select>
      <span class="sp"></span>
      <span class="gruppo" role="group" aria-label="Zoom"><button data-dvx="zmeno" aria-label="Rimpicciolisci">−</button><span class="z">${Math.round(z*100)}%</span><button data-dvx="zpiu" aria-label="Ingrandisci">+</button><button data-dvx="zadatta">Adatta</button></span>
      <button class="btn" data-dvx="arma" aria-pressed="${VIS.armato}">${VIS.armato?"Fai clic sul disegno… (Esc annulla)":"+ Aggiungi nota"}</button>
      <button class="btn" data-dvx="chiudi">Chiudi</button></div>
    <div class="visore-corpo"><div class="scena"><div class="carta${VIS.armato?" armata":""}" data-dvx="carta" style="width:${z*100}%;max-width:${z===1?"calc((100vh - 230px) * 420 / 297)":"none"}">${svgDisegno(VIS.file,cur.cod,cur.nome)}
      ${lista.map((nt,i)=>`<button class="pin${VIS.sel===nt.id?" sel":""}" style="left:${nt.x*100}%;top:${nt.y*100}%" data-dvx="pin:${nt.id}" aria-label="Nota ${i+1}: ${esc(nt.testo)}">${i+1}</button>`).join("")}
      ${VIS.nuovo?`<span class="pin nuovo" style="left:${VIS.nuovo.x*100}%;top:${VIS.nuovo.y*100}%">${lista.length+1}</span>
        <div class="pop-nota" data-dvx="pop" style="left:min(calc(${VIS.nuovo.x*100}% + 18px), calc(100% - 270px));top:min(calc(${VIS.nuovo.y*100}% - 10px), calc(100% - 160px))">
          <label class="fld"><span>Nota ${lista.length+1}</span><textarea id="nota-testo" maxlength="2000" placeholder="Che cosa c’è da sapere su questo punto?"></textarea></label>
          <span style="display:flex;gap:8px;justify-content:flex-end"><button class="btn sm" data-dvx="annullanota">Annulla</button><button class="btn sm pri" data-dvx="salvanota">Salva la nota</button></span></div>`:""}
    </div></div>
    <aside class="lato-note" aria-label="Note sul disegno"><div class="testa"><span class="lab">Note sul disegno · ${lista.length}</span>
      <span class="k" style="font-size:var(--t-sm)">Premi «+ Aggiungi nota», poi fai clic sul punto del disegno.</span></div>
      <div class="lista-note">${lista.length?lista.map((nt,i)=>`<div class="nota${VIS.sel===nt.id?" sel":""}" role="button" tabindex="0" data-dvx="pin:${nt.id}">
        <span class="n">${i+1}</span><span>${esc(nt.testo)}</span><span class="meta">${esc(nt.autore)} · ${esc(nt.quando)} · pag. 1${tagFase(nt)}</span>
        ${nt.mia?`<span class="azioni"><button class="btn sm danger" data-dvx="togli:${nt.id}">Togli</button></span>`:""}</div>`).join(""):`<p class="k" style="margin:4px 2px">Nessuna nota su questo disegno.</p>`}</div></aside></div>
    <div class="visore-piede"><span>Esc per chiudere</span><span>Una nota sta su questa revisione del disegno; la cambia o la toglie solo chi l’ha scritta.</span><span>Schizzo di esempio: nel Cockpit si vede il PDF vero del cliente.</span></div></div></div>`;
}

/* ---- i gesti ---- */
function azioneDistinta(a){
  const pid=S.rp, B=bomDi(pid), n=nodo(B,DIST.sel);
  if(a==="rifai"){
    const D=DOCS[pid];
    if(B.analisi==="senza"&&D.step&&!D.step.portale){B.analisi="ok";
      DIST.msg={ok:true,t:`<b>Analisi rifatta.</b> Lo STEP <span class="mono">${esc(D.step.f)}</span> ha una sola radice e nessun figlio: il prodotto è un pezzo unico.`};}
    else DIST.msg={ok:true,t:B.analisi==="senza"?"<b>Analisi rifatta:</b> lo STEP strutturale non c’è ancora, la struttura resta da costruire.":"<b>Analisi rifatta.</b> Il worker rilegge disegni e STEP e rifà la proposta. Quello che hai già confermato resta."};
  }
  if(a==="accetta"){const k=B.nodes.filter(x=>x.proposto).length; B.nodes.forEach(x=>x.proposto=false);
    DIST.msg={ok:true,t:`<b>${plur(k,"casella confermata","caselle confermate")}.</b> Ricordati di salvare la distinta.`};}
  if(a==="scarta") DIST.scarta=true;
  if(a==="scartano") DIST.scarta=false;
  if(a==="scartaok"){
    const via=new Set(B.nodes.filter(x=>x.proposto&&x.padre).map(x=>x.id));
    const su=id=>{let p=nodo(B,id).padre; while(via.has(p)) p=nodo(B,p).padre; return p;};
    B.nodes.filter(x=>!via.has(x.id)&&via.has(x.padre)).forEach(x=>x.padre=su(x.id));
    B.nodes=B.nodes.filter(x=>!via.has(x.id)); DIST.scarta=false; DIST.sel="r";
    DIST.msg={ok:true,t:`<b>${plur(via.size,"casella scartata","caselle scartate")}.</b> I loro file restano, senza pezzo, quando salvi.`};
  }
  if(a==="annullanuovo") DIST.nuovo=null;
  if(a==="aggiungi"){
    const codice=($("dn-cod").value||"").trim(), padre=$("dn-padre").value, qta=Math.max(1,parseInt($("dn-qta").value,10)||1), nome=($("dn-nome").value||"").trim();
    if(!codice){$("dn-cod").focus();$("dn-cod").style.borderColor="var(--bad)";return;}
    if(B.nodes.some(x=>x.codice.toUpperCase()===codice.toUpperCase())){DIST.msg={ok:false,t:`<b>${esc(codice)}</b> c’è già nella distinta.`};return render();}
    if(DIST.nuovo==="sottoass"&&codice.startsWith(PROD[pid].cod+"-A")) B.interni++;
    const id="n"+(++contaNuovi);
    B.nodes.push({id,pk:null,codice,nome,tipo:DIST.nuovo,padre,qta,proposto:false,fonti:[["acc","A mano","aggiunto da SV · adesso"]]});
    DIST.msg={ok:true,t:`<b>${esc(codice)}</b> aggiunto sotto <b>${esc(nodo(B,padre).codice)}</b>.${DIST.nuovo==="sottoass"?" Trascina qui sopra i pezzi che contiene.":""}`};
    DIST.sel=id; DIST.nuovo=null;
  }
  if(a==="eliminasel"){ if(!n||!n.padre) DIST.msg={ok:false,t:"Scegli prima una casella: il prodotto non si elimina."}; else DIST.elimina=true; }
  if(a==="elimina") DIST.elimina=true;
  if(a==="eliminano") DIST.elimina=false;
  if(a==="eliminaok"&&n&&n.padre){
    const via=new Set([n.id,...B.nodes.filter(x=>discende(B,n.id,x.id)).map(x=>x.id)]);
    B.nodes=B.nodes.filter(x=>!via.has(x.id)); DIST.sel=n.padre; DIST.elimina=false;
    DIST.msg={ok:true,t:`<b>${esc(n.codice)}</b> eliminato dalla distinta. I suoi file restano: quando salvi, vanno fra i file non associati del passo 1.`};
  }
  if(a==="conferma"&&n){n.proposto=false; DIST.msg={ok:true,t:`<b>${esc(n.codice)}</b> confermato.`};}
  if(a==="annullatutto"){B.nodes=clona(B.salvati); DIST.sel="r"; DIST.msg={ok:true,t:"Modifiche annullate."};}
  if(a==="salva"){
    syncParti(pid,B); B.salvati=clona(B.nodes); const k=B.nodes.filter(x=>x.proposto).length; B.verificata=!k;
    const q=rf(S.rfq); if(q) q.agg="adesso";
    DIST.msg=k?{ok:false,t:`<b>Distinta salvata</b>, tutta in una volta. Restano ${plur(k,"proposta","proposte")} da confermare: la BOM non è ancora verificata.`}
      :{ok:true,t:`<b>Distinta salvata e verificata</b> per <span class="mono">${esc(PROD[pid].cod)}</span>. I documenti del passo 1 seguono la distinta nuova.`};
  }
  render();
  if(a==="aggiungi"||a==="eliminaok"){const x=document.querySelector(`.nodo[data-dsel="${DIST.sel}"]`); if(x) x.focus();}
}
function azioneVisore(a,e,t){
  if(a==="sfondo"){ if(e.target===t){VIS.file=null;render();} return; }
  if(a==="pop") return;
  if(a==="chiudi"){VIS.file=null;return render();}
  if(a==="zpiu"){VIS.zoom=Math.min(ZOOM.length-1,VIS.zoom+1);return render();}
  if(a==="zmeno"){VIS.zoom=Math.max(0,VIS.zoom-1);return render();}
  if(a==="zadatta"){VIS.zoom=0;return render();}
  if(a==="arma"){VIS.armato=!VIS.armato;VIS.nuovo=null;return render();}
  if(a==="carta"){
    if(!VIS.armato){VIS.sel=null;return render();}
    const r=t.getBoundingClientRect();
    const x=r.width?Math.min(1,Math.max(0,(e.clientX-r.left)/r.width)):.5, y=r.height?Math.min(1,Math.max(0,(e.clientY-r.top)/r.height)):.5;
    VIS.nuovo={x,y}; VIS.armato=false; render(); const ta=$("nota-testo"); if(ta) ta.focus(); return;
  }
  if(a.startsWith("pin:")){VIS.sel=+a.slice(4);return render();}
  if(a.startsWith("togli:")){const id=+a.slice(6); NOTE[VIS.file]=(NOTE[VIS.file]||[]).filter(x=>x.id!==id); VIS.sel=null; return render();}
  if(a==="annullanota"){VIS.nuovo=null;return render();}
  if(a==="salvanota"){
    const ta=$("nota-testo"), tx=ta?ta.value.trim():"";
    if(!tx){if(ta)ta.focus();return;}
    (NOTE[VIS.file]=NOTE[VIS.file]||[]).push({id:prossimaNota++,x:VIS.nuovo.x,y:VIS.nuovo.y,testo:tx,autore:"SV",quando:"adesso",mia:true});
    VIS.sel=prossimaNota-1; VIS.nuovo=null; return render();
  }
}
document.addEventListener("click",e=>{
  const t=e.target.closest("[data-dsel],[data-dnuovo],[data-dx],[data-dvis],[data-dvx]"); if(!t) return;
  const D=t.dataset;
  if(D.dvx!==undefined) return azioneVisore(D.dvx,e,t);
  if(D.dvis){Object.assign(VIS,{file:D.dvis,zoom:0,armato:false,sel:null,nuovo:null});render();const b=document.querySelector('[data-dvx="chiudi"]');if(b)b.focus();return;}
  if(D.dsel){if(DIST.sel!==D.dsel){DIST.sel=D.dsel;DIST.elimina=false;DIST.msg=null;}return render();}
  if(D.dnuovo){DIST.nuovo=D.dnuovo;render();const x=$(D.dnuovo==="sottoass"?"dn-nome":"dn-cod");if(x)x.focus();return;}
  if(D.dx) return azioneDistinta(D.dx);
});
document.addEventListener("change",e=>{
  const id=e.target.id;
  if(id==="vis-file"){Object.assign(VIS,{file:e.target.value,sel:null,nuovo:null,armato:false});return render();}
  if(!/^dd-/.test(id)) return;
  const B=bomDi(S.rp), n=nodo(B,DIST.sel); if(!n) return;
  const v=e.target.value;
  if(id==="dd-cod"){const c=v.trim();
    if(c&&B.nodes.some(x=>x!==n&&x.codice.toUpperCase()===c.toUpperCase())) DIST.msg={ok:false,t:`<b>${esc(c)}</b> c’è già nella distinta.`};
    else if(c) n.codice=c;}
  if(id==="dd-nome") n.nome=v;
  if(id==="dd-cat"||id==="dd-catconf"){const p=parteDi(S.rp,n);
    if(id==="dd-cat"){n.cat=v; n.catConf=false;} else n.catConf=e.target.checked;
    if(p){p.cat=n.cat; p.catConf=n.catConf;} /* la classificazione è una decisione a sé: vale subito anche per i documenti */
    const sv=B.salvati.find(x=>x.id===n.id); if(sv){sv.cat=n.cat; sv.catConf=n.catConf;}}
  if(id==="dd-tipo"){
    if(!CONTENITORI.has(v)&&figliDi(B,n.id).length) DIST.msg={ok:false,t:`<b>${esc(n.codice)}</b> ha dei pezzi sotto: prima spostali, poi può diventare ${TIPI[v].toLowerCase()}.`};
    else n.tipo=v;}
  if(id==="dd-padre") DIST.msg=sposta(B,n.id,v);
  if(id==="dd-qta"){const q=parseInt(v,10); if(q>0) n.qta=q;}
  render();
});
/* trascinare una casella sopra il prodotto o un assieme */
const nodoDa=e=>e.target&&e.target.closest?e.target.closest(".nodo[data-dsel]"):null;
document.addEventListener("dragstart",e=>{const n=nodoDa(e); if(!n||n.getAttribute("draggable")!=="true") return;
  DIST.drag=n.dataset.dsel; try{e.dataTransfer.setData("text/plain",DIST.drag);e.dataTransfer.effectAllowed="move";}catch(_){} });
document.addEventListener("dragend",()=>{DIST.drag=null;document.querySelectorAll(".nodo.drop,.nodo.no-drop").forEach(x=>x.classList.remove("drop","no-drop"));});
document.addEventListener("dragover",e=>{const n=nodoDa(e); if(!n||!DIST.drag||n.dataset.dsel===DIST.drag) return;
  if(puoContenere(bomDi(S.rp),n.dataset.dsel,DIST.drag)){e.preventDefault();n.classList.add("drop");} else n.classList.add("no-drop");});
document.addEventListener("dragleave",e=>{const n=nodoDa(e); if(n) n.classList.remove("drop","no-drop");});
document.addEventListener("drop",e=>{const n=nodoDa(e); if(!n||!DIST.drag) return; e.preventDefault();
  const id=DIST.drag; DIST.drag=null; DIST.msg=sposta(bomDi(S.rp),id,n.dataset.dsel); DIST.sel=id; render();});
document.addEventListener("keydown",e=>{
  if(VIS.file&&e.key==="Escape"){ if(VIS.nuovo) VIS.nuovo=null; else if(VIS.armato) VIS.armato=false; else VIS.file=null; return render(); }
  const n=nodoDa(e);
  if(n&&e.target===n&&(e.key==="Enter"||e.key===" ")){e.preventDefault();DIST.sel=n.dataset.dsel;DIST.elimina=false;DIST.msg=null;render();
    const x=document.querySelector(`.nodo[data-dsel="${DIST.sel}"]`); if(x) x.focus(); return;}
  const nt=e.target.closest&&e.target.closest(".nota[data-dvx]");
  if(nt&&e.target===nt&&e.key==="Enter"){VIS.sel=+nt.dataset.dvx.slice(4);render();}
});
