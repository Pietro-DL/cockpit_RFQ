/* test_v16_c1.cjs · prove del COSTRUTTORE 1 della v16 (SPEC_v16 §9.2, C1-01 … C1-14), sullo schema di test_clic.cjs.
   Area: tema chiaro unico, guscio, ① Struttura e documenti (Documenti prima), ② Come, ⑤ Esito, Offerta, cassetti, Inbox.
   Si esegue: NODE_PATH=…/node_modules node --max-old-space-size=768 test_v16_c1.cjs */
const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup-v16.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));vc.on("error",e=>errori.push(String(e)));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document;let ok=0,ko=0;
const t=(nome,cond)=>{if(cond){ok++;}else{ko++;console.log("✗ "+nome);}};
const q=s=>d.querySelector(s), qa=s=>[...d.querySelectorAll(s)];
const click=(s,nome)=>{const el=typeof s==="string"?q(s):s;if(!el){ko++;console.log("✗ manca da cliccare: "+(nome||s));return false;}el.click();return true;};
const byText=(sel,txt)=>qa(sel).find(e=>e.textContent.includes(txt));
const ev=s=>w.eval(s);
const tasto=(k,o={},el)=>(el||d.body).dispatchEvent(new w.KeyboardEvent("keydown",Object.assign({key:k,bubbles:true,cancelable:true},o)));
const cambia=(el,v,nome)=>{ if(!el){ko++;console.log("✗ manca il campo: "+(nome||""));return;} if(el.type==="checkbox") el.checked=v; else el.value=v; el.dispatchEvent(new w.Event("change",{bubbles:true})); };
const avviso=()=>{const a=q("#ma-avviso");return a&&!a.hidden?{esito:a.getAttribute("data-esito"),testo:a.textContent.trim()}:null;};
const paAv=()=>{const a=q("#pa-avviso")||q("#pa-avviso-cas");return a?{esito:a.getAttribute("data-esito"),testo:a.textContent}:{esito:null,testo:""};};
const S2=e=>ev(`(x=>${e})(a2Di(threadAperto()))`);
const cidDi=s=>ev(`(righeB(threadAperto(),prodottoScelto(threadAperto())).find(r=>r.cod.endsWith(${JSON.stringify(s)}))||{}).cid`);
const ruolo=r=>{click('[data-menu="app"]');click('[data-menu="ruolo"]',"Vedi come");click(`[data-ruolo="${r}"]`,"ruolo "+r);};
const ROTTE=ev("ROTTE.map(compilaRotta).map(r=>r.source)").map(s=>new RegExp(s));
const FASI0=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).map(([k,T])=>[k,[T.testa.fase&&T.testa.fase.nome,T.testa.stato_thread]])))"));
function Z(sez){
  const ora=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).map(([k,T])=>[k,[T.testa.fase&&T.testa.fase.nome,T.testa.stato_thread]])))"));
  t(`Z1 [${sez}] fase e thread invariati`,Object.keys(FASI0).filter(k=>FASI0[k][0]).every(k=>JSON.stringify(FASI0[k])===JSON.stringify(ora[k])));
  t(`Z4 [${sez}] ogni data-ma-gesto nella lista chiusa`,qa("[data-ma-gesto]").every(e=>ROTTE.some(r=>r.test(e.getAttribute("data-ma-gesto")))));
  t(`Z4 [${sez}] nessun data-pa-gesto nella lista`,qa("[data-pa-gesto]").every(e=>!ROTTE.some(r=>r.test(e.getAttribute("data-pa-gesto")))));
  t(`Z5 [${sez}] errori JS = 0`,errori.length===0);
}
function Z2(sez){
  t(`Z2 [${sez}] nessun input spuntato`,!qa("input[type=checkbox],input[type=radio]").some(x=>x.checked));
  t(`Z2 [${sez}] nessuna tendina con un valore scelto`,qa("select").every(s=>s.value===""));
}
/* lo stato della zona A (albero, tipi, documenti, fonte): non deve cambiare dopo un gesto B (B1) */
const fotoA=()=>ev("JSON.stringify(threadAperto().prodotti.map(p=>[p.fonte&&p.fonte.stato,p.bom,p.nodi.map(n=>[n.nodo.codice,n.nodo.decisione,n.nodo.tipo]),p.documenti]))");

/* ================= C1-01 · tema: solo chiaro ================= */
t("C1-01 nessun pulsante del tema nell'Inbox",!q("[data-tema]"));
t("C1-01 <meta name=color-scheme content=light>",!!q('meta[name="color-scheme"][content="light"]'));
t("C1-01 :root ha color-scheme: light",/:root\{\s*color-scheme:light;/.test(html));
t("C1-01 niente token scuri né codice del tema",!/prefers-color-scheme|data-theme|data-tema|cambiaTema|applicaTema|distinta\.tema/.test(html));
t("C1-01 body con lo sfondo esplicito",/body\{background:var\(--paper\);color:var\(--ink\)/.test(html));

/* ================= C1-02 · contrasti in un solo tema ================= */
(()=>{
  const root=html.match(/:root\{([\s\S]*?)\n\}/)[1], tok={};
  for(const m of root.matchAll(/--([\w-]+):(#[0-9A-Fa-f]{6})/g)) tok[m[1]]=m[2];
  const lum=h=>{const c=[1,3,5].map(i=>parseInt(h.substr(i,2),16)/255).map(v=>v<=0.03928?v/12.92:Math.pow((v+0.055)/1.055,2.4));return .2126*c[0]+.7152*c[1]+.0722*c[2];};
  const cr=(a,b)=>{const x=lum(tok[a]),y=lum(tok[b]);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05);};
  for(const [a,b] of [["ink","paper"],["ink2","paper"],["ink3","paper"],["ink3","sunken"],["acc-ink","acc"],["ok","ok-soft"],["ver","ver-soft"],["ko","ko-soft"],["info","info-soft"],["prop","prop-soft"],["pa","pa-soft"],["off","off-soft"]])
    t(`C1-02 contrasto --${a} su --${b} ≥ 4,5 (${cr(a,b).toFixed(1)})`,cr(a,b)>=4.5);
  t(`C1-02 --line-campo su --surf ≥ 3 (${cr("line-campo","surf").toFixed(1)})`,cr("line-campo","surf")>=3);
  t("C1-02 --off = #5C6570",tok.off.toUpperCase()==="#5C6570");
  t("C1-02 i campi usano --line-campo",/\.fld input,\.fld select,\.fld textarea\{[^}]*var\(--line-campo\)/.test(html));
})();

/* ================= Inbox e Richieste RFQ (§6.2) ================= */
click('[data-cli="acme"]');
t("Inbox: «Apri la Distinta» una volta sola (nella carta della RFQ)",qa('[data-apririfq]').length===1&&!!q(".rfqblock [data-apririfq]"));
t("Inbox: «Gestisci clienti e mittenti» è un link",!!q('a.link-piccolo[data-setsez="clienti"]')&&!byText(".p-rail button","Gestisci clienti"));
t("Inbox: «Stati letti dal server» nel title, non a video",!q(".nota-lettura")&&!!q('.p-ctx [title*="ProdottoValutato"]'));
click('a.link-piccolo[data-setsez="clienti"]');t("il link porta a Clienti e mittenti",ev("S.tab")==="set");
click('[data-tab="gest"]');
t("Richieste RFQ: solo «thread chiusa» come chip",!byText("#gbody td","thread aperta")&&!!byText("#gbody td","thread chiusa"));
t("Richieste RFQ: il sottotitolo sta nel title",!byText(".ghead p","Raggruppate")&&!!q('.ghead [title*="Raggruppate"]'));

/* ================= C1-03 · ① si apre sui Documenti ================= */
click('[data-gfase="tutte"]');click(byText("#gbody tr.click","482-00"),"RFQ VFR");
const tid=ev("threadAperto().id");
t("C1-03 aprire non scrive niente",ev("REGISTRO.length")===0);
t("C1-03 all'ingresso la vista è Documenti; P-01: un solo workspace, i Documenti in alto e l'albero nella stessa pagina",ev("S.vista")==="documenti"&&!!q("#ma-documenti")&&!!q("#ma-albero")&&!!(q("#ma-documenti").compareDocumentPosition(q("#ma-fonte"))&w.Node.DOCUMENT_POSITION_FOLLOWING)&&!!(q("#ma-da-smistare").compareDocumentPosition(q("#ma-fonte"))&w.Node.DOCUMENT_POSITION_FOLLOWING));
t("P-01: l'ordine del markup R9 · R8 · R3 · R4 · R5 · R7, R6 a destra",(()=>{const o=["#ma-documenti","#ma-da-smistare","#ma-fonte","#ma-albero","#ma-schede-albero","#ma-conflitti-struttura","#ma-anteprima"].map(x=>q(x));return o.every(Boolean)&&o.every((x,i)=>!i||(o[i-1].compareDocumentPosition(x)&w.Node.DOCUMENT_POSITION_FOLLOWING));})());
t("P-14: nessun numero calcolato dal JS nella nav delle viste",!q("#ma-viste .ma-vista-n"));
const viste=qa("#ma-viste a");
t("C1-03 tre link nell'ordine Documenti · Distinta · Riepilogo RFQ",viste.map(a=>a.dataset.vista).join()==="documenti,distinta,rfq"&&/Documenti/.test(viste[0].textContent)&&/Riepilogo RFQ/.test(viste[2].textContent));
t("C1-03 aria-current sul primo link; nessuna tablist",viste[0].getAttribute("aria-current")==="page"&&!q('#ma-viste[role="tablist"]'));
t("C1-03 M1: la riga d'aiuto dice dove si fa ogni cosa",q("#ma-aiuto-doc").textContent.includes("Qui confermi i disegni con il codice sicuro")&&q("#ma-aiuto-doc").textContent.includes("Lo STEP si autorizza più sotto"));
t("C1-03 «↓ Analisi e albero» scorre nella stessa pagina (nessuna navigazione)",!!q('#ma-aiuto-doc a[data-ma-scorri="#ma-fonte"]'));
t("C1-03 il contatore della stazione ① viene dal finto server A",q('.staz[data-staz="1"]').textContent.includes("BOM")&&q('.staz[data-staz="1"]').textContent.includes("doc."));
Z2("① Documenti");

/* ================= C1-06 · i 4 spenti dei Documenti, visibili come in Pietro ================= */
const MOTIVI={"Cerca di nuovo sul portale":"Non ancora disponibile: nessun codice scarica dal portale.","Carica a mano":"Non ancora disponibile qui: si carica dalla pagina del Fascicolo (R-06).","Conferma tutti i riconosciuti":"Non ancora disponibile: arriva con lo spazio di verifica, non si anticipa né si simula.","Conferma la minuteria":"Non ancora disponibile: la conferma della categoria arriva con lo spazio di verifica (R-08)."};
for(const [x,m] of Object.entries(MOTIVI)){ const n=byText("#ma-documenti .ma-toolbar .ma-na",x);
  t("C1-06 spento e visibile: "+x,!!n&&!n.closest("details")&&n.querySelector("button").disabled&&n.querySelector("button").title===m&&n.querySelector("small").textContent===m); }
t("C1-06 «Chiedi al cliente» col solo bollino ◇ e il title R-41",qa('#ma-documenti [data-cassetto-apri="richieste"]').every(b=>b.querySelector(".bollino-b")&&/Proposta · dopo A2 · R-41/.test(b.title))&&!q("#ma-documenti .chip-b"));

/* ================= C1-04 · conferma di un documento con la BOM non verificata ================= */
const bConf=q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]');
t("C1-04 la domanda contiene la frase DB7-2 alla lettera",!!bConf&&bConf.dataset.maDomanda.includes("La copia sul NAS parte subito; la struttura non è ancora verificata."));
click(bConf);t("C1-04 prima la domanda, niente parte",!!q("#ma-domanda")&&q("#ma-domanda").textContent.includes("La copia sul NAS parte subito; la struttura non è ancora verificata.")&&ev("REGISTRO.filter(x=>x.metodo==='POST').length")===0);
click("[data-ma-si]");
t("C1-04 una sola voce per gesto",ev("REGISTRO.filter(x=>x.metodo==='POST'&&/fascicolo\\/conferma$/.test(x.url)).length")===1&&ev("Object.keys(REGISTRO.find(x=>/fascicolo\\/conferma$/.test(x.url)).campi).join()")==="voce");
const riga0=q("tr.ma-voce");
t("C1-04 «confermato» e «copiato» in due celle diverse; copiato solo con la copia scritta",!!riga0&&riga0.querySelector('[data-ma-lab="Conferma documentale"]')!==riga0.querySelector('[data-ma-lab="Copia sul NAS"]')&&!riga0.querySelector('[data-ma-lab="Copia sul NAS"]').textContent.includes("copiato sul NAS"));

/* ================= C1-05 · la vista Distinta ================= */
click('#ma-viste a[data-vista="distinta"]');
t("C1-05 il link «Distinta» sceglie solo l'ancora (stessa pagina)",ev("S.vista")==="distinta"&&!!q("#ma-albero")&&!!q("#ma-documenti"));
t("C1-05 #ma-albero prima di #ma-schede-albero",!!(q("#ma-albero").compareDocumentPosition(q("#ma-schede-albero"))&w.Node.DOCUMENT_POSITION_FOLLOWING));
t("C1-05 nota U-07 alla lettera",q("#ma-albero .ma-u07").textContent.includes("La strada principale è \"Conferma l'albero\", dopo il riepilogo. Le decisioni nodo per nodo non rendono la BOM verificata e, prese prima del riepilogo, possono lasciarla da verificare."));
const menuT=q(".ma-navi details[data-ma-menu]");
t("C1-05 «Aggiorna» più il menu «Azioni del thread» (Prepara i file, Rifai lo smistamento)",!!q('.ma-navi [data-ma-azione="aggiorna"]')&&!!menuT&&menuT.querySelector("summary").textContent.includes("Azioni del thread")&&!!menuT.querySelector('[data-ma-gesto$="/fascicolo/prepara"]')&&!!menuT.querySelector('[data-ma-gesto$="/smistamento/aggiorna"]')&&!menuT.querySelector('[data-ma-gesto$="/rianalizza"]'));
t("C1-05 «Rifai l'analisi» nell'intestazione di R3",!!q('#ma-fonte .ma-fs-testa [data-ma-gesto$="/fascicolo/rianalizza"]'));
t("C1-05 nessun gesto parte all'apertura (registro: solo la conferma del documento)",ev("REGISTRO.filter(x=>x.metodo==='POST').length")===1);
t("C1-05 zoom in un solo comando «⤢ 100 % ▾» (dentro adatta, −, +)",!!q(".ma-menu--zoom summary")&&q(".ma-menu--zoom summary").textContent.includes("100 %")&&qa('.ma-menu--zoom [data-ma-azione="zoom"]').length===3);
t("C1-05 «Apri tutto» e «Chiudi tutto» restano",qa('[data-ma-azione="rami"]').length===2);
t("C1-05 sotto le aggiunte spente il testo di Pietro",q("#ma-strumenti-perche").textContent==="⊘ Non ancora disponibile: in questa pagina la bozza dell'albero resta vuota e nessun codice d'assieme parte dal browser (DB7-5, DB7-7).");
click('.ma-menu--zoom [data-z="-"]');t("C1-05 lo zoom funziona dal menu",ev("S.zoom")===0.9);click('.ma-menu--zoom [data-z="+"]');
/* STEP, albero: le tre conferme restano in tre posti */
click('[data-ma-azione="step"]');const cb=q('.ma-finestra input[name=autorizzo]');t("STEP: «autorizzo» mai spuntata",!!cb&&!cb.checked);
cambia(cb,true,"autorizzo");click('[data-ma-azione="step-invia"]');
click('[data-ma-azione="albero"]');click('[data-ma-azione="albero-invia"]');
t("C1-05 P-03/P-04: dopo «Conferma l'albero» la vista non cambia da sola; l'avviso resta finché non lo si legge",ev("prodottoScelto(threadAperto()).bom.verificata")===true&&ev("S.vista")==="distinta"&&avviso()&&avviso().esito==="ok"&&!/setTimeout\(\(\)=>\{ if\(PONTE\.avviso/.test(html));
let n=0;while(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]')&&n++<12){click(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]'));click("[data-ma-si]");}
t("documenti confermati uno per uno",n>=4);
t("C1-03 «Conferma tutti i riconosciuti» resta spento",byText("#ma-documenti .ma-na","Conferma tutti i riconosciuti").querySelector("button").disabled);
Z("① Struttura e documenti");
const A0=fotoA();

/* ================= guscio (§6.2) ================= */
t("guscio: ruolo «Lino · simulato» sempre visibile e non è un pulsante",q(".tt-ruolo").tagName!=="BUTTON"&&q(".tt-ruolo").textContent.includes("Lino")&&q(".tt-ruolo").textContent.includes("simulato"));
t("guscio: «lotti» esce dalla testata",!q(".tt-b").textContent.includes("lotti")&&q(".tt-b").textContent.includes("scad."));
click('[data-menu="cartiglio"]');t("guscio: quantità e scaglioni restano nel cartiglio",q(".tendina--cartiglio").textContent.includes("Quantità e scaglioni"));click('[data-menu="cartiglio"]');
click('[data-menu="app"]');
t("guscio: nel menu ≡ Inbox, Richieste RFQ, Clienti e mittenti e «Vedi come»",["inbox","gest","set"].every(k=>q(`.tendina--app [data-tab="${k}"]`))&&!!q('.tendina--app [data-menu="ruolo"]'));
click('.tendina--app [data-menu="ruolo"]');t("guscio: «Vedi come» con i 4 ruoli",qa(".tendina--ruolo [data-ruolo]").length===4);tasto("Escape");
t("guscio: cassetti Richieste · Controlli · ⋯",qa(".cas-btn").length===3&&!!q('.cas-btn[data-cassetto="richieste"]')&&!!q('.cas-btn[data-cassetto="controlli"]')&&!!q('.cas-btn[data-menu="altro"]'));
click('[data-menu="altro"]');
t("guscio: nel ⋯ Cataloghi (L), Aiuto e tasti (?), Non ancora nel backend (I)",!!q('.tendina--altro [data-cassetto="cataloghi"]')&&!!q('.tendina--altro [data-cassetto="backend"][data-castab="tasti"]')&&!!q('.tendina--altro [data-cassetto="backend"][data-castab="spiegazione"]'));
click('.tendina--altro [data-cassetto="cataloghi"]');t("⋯ → Cataloghi",ev("S.cassetto")==="cataloghi"&&!q(".tendina--altro"));
tasto("Escape");tasto("l");t("tasto L: Cataloghi",ev("S.cassetto")==="cataloghi");tasto("Escape");
tasto("i");t("tasto I: «Non ancora nel backend» sulla scheda «Che cosa è vero»",ev("S.cassetto")==="backend"&&q("#cassetto").textContent.includes("Zona A · contratto A1d"));tasto("Escape");
tasto("k");t("tasto K: Controlli, gruppi per componente chiusi (uno aperto al massimo)",ev("S.cassetto")==="controlli"&&qa("#cassetto details.ctrl-cas[open]").length<=1);tasto("Escape");
t("Esc a cascata: il cassetto si chiude",!q("#cassetto"));

/* ================= C1-08 · un solo «?» per stazione ================= */
const CHIAVE={1:"scheda dell'albero",2:"lamiera comprata",5:"OK · DUBBIO · KO"};
for(const s of [1,2,3,4,5]){
  tasto("g");tasto(String(s));tasto("?");
  const c=q("#cassetto");
  t(`C1-08 ? sulla stazione ${s}: cassetto con la scheda Tasti`,!!c&&!!c.querySelector(".tasti")&&q('[data-castab="tasti"][aria-pressed="true"]'));
  t(`C1-08 ? sulla stazione ${s}: le schede Che cosa è vero, JSON, Gesti`,["spiegazione","json","gesti"].every(k=>c&&c.querySelector(`[data-castab="${k}"]`)));
  if(CHIAVE[s]) t(`C1-08 ? sulla stazione ${s}: i tasti della stazione`,c.textContent.includes(CHIAVE[s]));
  else t(`C1-08 ? sulla stazione ${s}: i tasti da AIUTO_STAZ[${s}] (C2)`,ev(`!!(AIUTO_STAZ[${s}]&&AIUTO_STAZ[${s}].tasti&&AIUTO_STAZ[${s}].tasti.length)`)&&c.querySelectorAll(".tasti-dl dt").length>4);
  tasto("?");t(`C1-08 ? di nuovo chiude (stazione ${s})`,!q("#cassetto"));
}
click('.staz[data-staz="1"]');

/* ================= C1-10, C1-09, C1-07 · ② Come ================= */
click('.staz[data-staz="2"]');
Z2("② Come");
t("C1-09 Come: una sola fascia B, di una riga, con R-xx nel title",qa(".ws-main .banda-b").length===1&&q(".ws-main .banda-b").title.includes("R-22")&&!q(".ws-main .banda-b").textContent.includes("R-22"));
t("C1-09 Come: nessun «◇ dopo A2» per elemento",!q(".ws-main .chip-b")&&!byText(".ws-main span","dopo A2 ")||!q(".ws-main .chip-b"));
t("C1-10 Come: i 6 modi interi su ogni riga",qa(".come-tab .c-come .seg").length>0&&qa(".come-tab .c-come .seg").every(s=>s.querySelectorAll("button").length===6));
t("C1-10 Come: la legenda dei tasti non è nella pagina",!q(".b-legenda")&&!q('.ws-main [data-menu="tasti"]'));
t("C1-10 Come: niente colonna Stato e niente chip «A» per riga",!q(".come-tab th.c-st")&&!q(".come-tab td.c-tipo .fonte"));
t("C1-07 Come: una sola primaria, nella barra in basso",qa(".ws-main .ma-btn--pri").length===1&&!!q(".ws-main .barra-staz .ma-btn--pri"));
t("C1-10 Come: il contatore è corto, il resto nel title",/^\d+\/\d+ smistati$/.test(q(".b-conta").textContent.trim()));
const vis0=ev("S._comeVis.length");
for(let i=0;i<vis0;i++) tasto("Enter");
t("Come: Invio accetta e scende (tutte le righe smistate)",ev("(()=>{const T=threadAperto(),p=prodottoScelto(T);return righeB(T,p).filter(r=>r.cid&&!r.comm).every(r=>comeDi(T,r.cid));})()"));
t("Come: B1 la zona A non cambia",fotoA()===A0);

/* ================= C1-11 · ⑤ Esito ================= */
click('.staz[data-staz="5"]');
const righeE=qa(".esito-tab tbody tr");
t("C1-09 Esito: una sola fascia B e nessun «◇ dopo A2» per elemento",qa(".ws-main .banda-b").length===1&&!q(".ws-main .chip-b"));
t("C1-07 Esito: una sola primaria «Consegna a Franco →», nella barra in basso",qa(".ws-main .ma-btn--pri").length===1&&!!q(".ws-main .barra-staz [data-consegna]"));
t("C1-11 Esito: scelta intera solo sulla riga corrente, compatta sulle altre",qa(".esito-tab .seg--compatta").length===righeE.length-1&&qa(".esito-tab tr.es-cur .seg button").length===3);
t("C1-11 Esito: proposte ◇ su tutte le righe, nessuna decisa",qa(".esito-tab .seg--prop").length===righeE.length&&!qa('.esito-tab .seg button[aria-pressed="true"]').length);
t("C1-11 Esito (M-8): sulle righe non scelte la regola peggiore più «+n» come testo, nessun pulsante",qa(".esito-tab tr:not(.es-cur) .esito-perche").every(x=>x.querySelectorAll(":scope > span > .liv").length<=1&&!x.querySelector("button,summary")));
t("C1-11 Esito: Lino vede Esito e il Foglio, il resto nel ⋯",qa('.off-nav > [data-esito-vista]').length===2&&qa('.off-nav details.piu [data-esito-vista]').length===3);
const cids=righeE.map(r=>r.dataset.esitoRiga), VAL=["OK","DUBBIO","KO"];
cids.forEach((cid,i)=>{ const r=q(`[data-esito-apri="${cid}"]`); if(r) click(r); tasto("obx"[i%3]); });
t("C1-11 Esito: O B X funzionano su ogni riga",cids.every((cid,i)=>S2(`(x.esiti[${JSON.stringify(cid)}]||{}).v`)===VAL[i%3]));
t("C1-11 Esito: nota sulle righe DUBBIO e KO",cids.every((cid,i)=>i%3===0&&cid!==ev("S.esitoRiga") ? !q(`[data-esito-nota="${cid}"]`) : !!q(`[data-esito-nota="${cid}"]`)));
cids.forEach(cid=>{ click(q(`[data-esito-apri="${cid}"]`)||q(`tr[data-esito-riga="${cid}"]`)); tasto("o"); });
t("Esito: tutte OK di nuovo",cids.every(cid=>S2(`x.esiti[${JSON.stringify(cid)}].v`)==="OK"));
/* Dati e documenti: sei interruttori e «Accetta le N proposte» */
t("Esito: Dati e documenti con sei interruttori",qa(".dd .dd-int").length===6);
const bTutte=q("[data-dd-tutte]");t("Esito: «Accetta le N proposte» dei Dati e documenti",!!bTutte);
if(bTutte){ const nA=+bTutte.textContent.match(/\d+/)[0]; click(bTutte); t("Esito: «Accetta le N proposte» scrive una voce per proposta",S2("Object.keys(x.datiDoc).length")===nA); }
/* preparazioni: la maschera dell'assieme, senza costo → «Richiedi le N preparazioni senza costo» */
const root=cidDi(".000");
const haPrep=ev("typeof attrezzaturaPronta==='function'&&typeof preparazioniRFQ==='function'");
t("C1-11 (C2) attrezzaturaPronta e preparazioniRFQ disponibili",haPrep);
if(haPrep){
  ev(`eseguiB("/componente/${root}/preparazioni",{k:"maschera",on:"1",fase_id:cicloB(threadAperto(),"${root}").fasi.find(f=>f.p==="sald_mag").id})`);
  click('.staz[data-staz="5"]');
  const bRich=q("[data-richiedi-prep]");
  t("C1-11 Esito: attrezzatura letta dalle preparazioni, con «Richiedi la preparazione senza costo» (B2: singolare)",!!q(".dd-attr")&&!!bRich&&/Richiedi la preparazione senza costo|Richiedi le \d+ preparazioni senza costo/.test(bRich.textContent));
  t("C1-11 M2: la preparazione dice la sua fase anche in Esito",/maschera.*f\.\d+/i.test(q(".dd-attr").textContent));
  const nRq0=S2("x.richieste.length");
  click(bRich);
  t("C1-11 «Richiedi…» (P-02): apre la carta «Attrezzature» del cassetto; il clic non crea richieste né sceglie fornitori",S2("x.richieste.length")===nRq0&&ev("S.cassetto")==="richieste"&&!!q('#cassetto .rq[data-rq="attrezzatura"]'));
  /* ================= C1-12 · cassetto Richieste ================= */
  t("C1-12 cassetto: carte chiuse, una aperta al massimo",qa("#cassetto details.rq-carta").length>=1&&qa("#cassetto details.rq-carta[open]").length<=1);
  const cP=()=>q('#cassetto .rq[data-rq="attrezzatura"]'), rf=n=>cP()?[...cP().querySelectorAll(".rq-forn")].find(x=>x.textContent.includes(n)):null, bi=(el,txt)=>el?[...el.querySelectorAll("button")].find(b=>b.textContent.includes(txt)):null;
  t("C1-12 la carta delle attrezzature dice la preparazione e la sua fase",!!cP()&&/maschera/i.test(cP().querySelector("summary").textContent)&&/f\.\d+/.test(cP().querySelector("summary").textContent));
  t("C1-12 M7: i fornitori di attrezzature con la loro descrizione accanto al nome",["Robosald","Minitool","Dalmi"].every(n=>!!rf(n))&&rf("Dalmi").textContent.includes("set di curvatura"));
  t("C1-12 chi non fa maschere non ha «Prepara la bozza»",!bi(rf("Dalmi"),"Prepara la bozza")&&!!bi(rf("Robosald"),"Prepara la bozza"));
  click(bi(rf("Robosald"),"Prepara la bozza"),"Prepara la bozza Robosald");
  const idP=S2(`(x.richieste.find(q=>q.forn&&q.forn.id==="robosald")||{}).id`), riga=()=>q(`#cassetto [data-rq-riga="${idP}"]`);
  t("C1-12 una riga di Pietro per Robosald: «bozza in Outlook», nessun «Manda»",!!riga()&&riga().textContent.includes("bozza in Outlook")&&!qa("#cassetto button").some(b=>/^Manda/.test(b.textContent.trim())));
  t("C1-12 la carta resta aperta dopo il gesto",!!cP()&&cP().querySelector("details.rq-carta").open);
  click(bi(riga(),"Simula: la mail parte"),"sync");
  t("C1-12 «inviata» quando il sync la vede; «Simula la risposta» e «passa la scadenza» nel ⋯, marcati «(mockup)»",S2(`x.richieste.find(q=>q.id==="${idP}").stato`)==="inviata"&&!!bi(riga(),"Simula la risposta (mockup)")&&!!bi(riga(),"passa la scadenza (mockup"));
  click(bi(riga(),"Simula la risposta (mockup)"),"Simula");
  click(riga().querySelector('[data-pa-gesto$="/risposta"]'),"Registra");
  const pr=JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()).find(x=>x.k==='maschera'))"));
  t("C1-12 Registra: costo, settimane e fonte RISP nella preparazione; la richiesta resta «offerta ricevuta»",!!pr&&pr.costo!==""&&pr.fonte==="RISP"&&S2(`x.richieste.find(q=>q.id==="${idP}").stato`)==="offerta_ricevuta");
  click('[data-cassetto=""]');
}
t("C1-11 B1 la zona A non cambia",fotoA()===A0);

/* ================= consegna e C1-13 · Offerta ================= */
click('.staz[data-staz="5"]');
const firmaB=byText(".firma-b","Area tecnica").querySelector("[data-pa-gesto]");
t("Esito: firme, attiva solo l'area del ruolo (Lino: area tecnica)",!!firmaB&&qa(".firma-b [data-pa-gesto]").length===2&&byText(".firma-b","Acquisti").textContent.includes("attende Fabio"));
click(firmaB);
cambia(q("[data-lotto]"),true,"lotto");
if(q("[data-presa-atto]")) cambia(q("[data-presa-atto]"),true,"presa d'atto");
click("[data-consegna]");
t("Esito: consegnato a Franco",S2("x.consegne.length")===1);
ruolo("Franco");
t("C1-13 ruolo Franco: le sue quattro viste e l'Esito, senza ⋯",qa('.off-nav [data-esito-vista]').length===5&&!q(".off-nav details.piu"));
click('[data-esito-vista="offerta"]');
const prim=()=>qa(".off-v .ma-btn--pri").filter(b=>!b.closest("details:not([open]) .piu-corpo"));
t("C1-13 Offerta VFR (ordine diretto): un primario «Accetta l'ordine», «Rifiuta» nel ⋯",prim().length===1&&prim()[0].dataset.of==="accettaord"&&!!q('.off-v details.piu [data-of="rifiutaord"]'));
const statiO={bozza:"emetti",emessa:"inviata",inviata:null};
for(const [st,pri] of Object.entries(statiO)){
  ev(`(()=>{const T=threadAperto(),O=offertaDi(T);O.stato=${JSON.stringify(st)};O.num=O.num||5479;})()`);
  ev(`(()=>{const T=threadAperto(),f=fotoCorrente(T),P=prodDiFoto(T,f).P;window.__tgt=P.target;P.target=null;})()`); render0();
  const pp=qa(".off-v .off-azioni > .ma-btn--pri, .off-v .off-azioni > details.piu--esito > summary.ma-btn--pri");
  t(`C1-13 Offerta «${st}»: un solo primario${pri?" ("+pri+")":" (Esito del cliente ▾)"}`,pp.length===1&&(pri?pp[0].dataset.of===pri:/Esito del cliente/.test(pp[0].textContent)));
  t(`C1-13 Offerta «${st}»: «Crea una revisione» al massimo una volta`,qa('.off-v [data-of="rev"]').length<=1);
  ev("(()=>{const T=threadAperto(),f=fotoCorrente(T),P=prodDiFoto(T,f).P;P.target=window.__tgt;})()");
}
function render0(){ ev("render()"); }
ev(`(()=>{const O=offertaDi(threadAperto());O.stato="bozza";})()`);render0();
if(haPrep){
  const sett=ev("settAttrezzature(threadAperto(),fotoCorrente(threadAperto()))");
  t("C1-13 Offerta: blocco Attrezzature con costo e settimane",!!byText(".off-v .dblock h3","Attrezzature")&&sett>0);
  t("C1-13 Offerta: «LT 1° ordine: + attrezzature, fino a N settimane ◇» e nel documento",!!q(".off-lt-attr")&&q(".off-lt-attr").textContent.includes(`fino a ${sett} settimane`)&&q(".so-carta").textContent.includes(`+ attrezzature fino a ${sett} settimane`));
  const lt0=+ev("offertaDi(threadAperto()).lt1");click('[data-of="ltattr"]');
  t("C1-13 Offerta: «Usa» porta il LT del primo ordine a LT + settimane",+ev("offertaDi(threadAperto()).lt1")===lt0+sett&&ev("REGISTRO.some(x=>/offerta\\/ltattr$/.test(x.url))"));
  const sel=q(".off-v select[data-opa]");
  t("C1-13 Offerta: come si paga (ammortizzata · a parte · del cliente), qui e non nel Ciclo",!!sel&&[...sel.options].map(o=>o.value).join()==="ammortizzata,a parte,del cliente");
  cambia(sel,"a parte","come si paga");
  t("C1-13 Offerta: «a parte» diventa una riga dell'offerta",!!byText(".so-carta td","ATTREZZATURA"));
}
ruolo("Lino");
Z("⑤ Esito e Offerta");

/* ================= C1-14 · inventario §6.3 (parte C1): ogni voce raggiungibile ================= */
click('.staz[data-staz="1"]');click('[data-vista="distinta"]');
const INV=[
  ["≡ menu",'[data-menu="app"]'],["cartiglio",'[data-menu="cartiglio"]'],["menu prodotti",'[data-menu="prodotti"]'],["5 stazioni",'.staz[data-staz="5"]'],
  ["Richieste",'[data-cassetto="richieste"]'],["Controlli",'[data-cassetto="controlli"]'],["⋯ cassetti",'[data-menu="altro"]'],
  ["viste ①",'#ma-viste [data-vista="documenti"]'],["Aggiorna",'[data-ma-azione="aggiorna"]'],["Prepara i file",'[data-ma-gesto$="/fascicolo/prepara"]'],["Rifai l'analisi",'[data-ma-gesto$="/rianalizza"]'],["Rifai lo smistamento",'[data-ma-gesto$="/smistamento/aggiorna"]'],
  ["Conferma dell'albero",'[data-ma-azione="albero"]'],["Poi: ② Come (fuori da #ma)",'.stazioni .staz-poi[data-staz="2"]'],["schede dell'albero",'.ma-scheda'],["zoom",'.ma-menu--zoom'],["Apri tutto",'[data-ma-aperti="1"]'],["Chiudi tutto",'[data-ma-aperti="0"]'],
  ["aggiunte spente",".ma-strumenti button[disabled]"],
];
for(const [nome,sel] of INV) t("C1-14 raggiungibile: "+nome,!!q(sel));
click('[data-vista="documenti"]');
for(const [nome,sel] of [["Ispeziona",'[data-ma-azione="ispeziona"]'],["Riprova le copie",'[data-ma-gesto$="/riprova-copie"]'],["Da smistare","#ma-da-smistare"],["spenti C visibili","#ma-documenti .ma-toolbar .ma-na button[disabled]"]]) t("C1-14 raggiungibile: "+nome,!!q(sel));
click('[data-vista="rfq"]');t("C1-14 raggiungibile: Riepilogo RFQ (sette assi)",qa(".ma-asse").length===7);
click('.staz[data-staz="2"]');
for(const [nome,sel] of [["filtri di Come","[data-come-filtro]"],["ricerca di Come","#come-cerca"],["modello di ciclo","[data-menu-mod]"],["primario di Come",".barra-staz .ma-btn--pri"]]) t("C1-14 raggiungibile: "+nome,!!q(sel));
click('.staz[data-staz="5"]');
for(const [nome,sel] of [["Esito: vista Foglio",'[data-esito-vista="foglio"]'],["Esito: Costi, Listini, Offerta nel ⋯",'details.piu [data-esito-vista="offerta"]'],["lotti","[data-lotto]"],["+ lotto",'[data-pa-gesto$="/lotto"]'],["Passa alla scheda costo spento",".ma-na button[disabled]"],["requisiti",".req-l"],["firme",".firme-b"],["Apri il Foglio",'[data-foto="0"]']]) t("C1-14 raggiungibile: "+nome,!!q(sel));
tasto("r");t("C1-14 raggiungibile: cassetto Richieste (R)",ev("S.cassetto")==="richieste");tasto("Escape");
Z("fine");

console.log(`\ntest_v16_c1: ${ok} superati · ${ko} falliti · errori JS: ${errori.length}`);
[...new Set(errori)].slice(0,5).forEach(e=>console.log("  JS: "+e.slice(0,160)));
process.exitCode = ko ? 1 : 0;
