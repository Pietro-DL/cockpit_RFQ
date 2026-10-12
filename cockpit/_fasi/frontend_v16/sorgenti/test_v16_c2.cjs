/* test_v16_c2.cjs · v16, costruttore 2 (SPEC_v16 §9.3): Ciclo (PDF del prodotto finito, pre-unioni, preparazioni con
   «Richiedi»), Sequenza scritta a mano, albero del processo, finto server A2. jsdom, una sola esecuzione alla volta.
   Invarianti Z1 Z2 Z4 Z5 e B1 dopo ogni sezione. */
const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup-v16.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));vc.on("error",e=>errori.push(String(e)));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document;let ok=0,ko=0;
const t=(nome,cond)=>{if(cond){ok++;}else{ko++;console.log("✗ "+nome);}};
const q=s=>d.querySelector(s), qa=s=>[...d.querySelectorAll(s)];
const ev=s=>w.eval(s);
const click=(s,nome)=>{const el=typeof s==="string"?q(s):s;if(!el){ko++;console.log("✗ manca da cliccare: "+(nome||s));return false;}el.click();return true;};
const byText=(sel,txt)=>qa(sel).find(e=>e.textContent.includes(txt));
const tasto=(k,o={},el)=>(el||d.body).dispatchEvent(new w.KeyboardEvent("keydown",Object.assign({key:k,bubbles:true,cancelable:true},o)));
const cambia=(el,v,nome)=>{ if(!el){ko++;console.log("✗ manca il campo: "+(nome||""));return;} if(el.type==="checkbox") el.checked=v; else el.value=v; el.dispatchEvent(new w.Event("change",{bubbles:true})); };
const submit=(form,nome)=>{if(!form){ko++;console.log("✗ manca il modulo: "+nome);return;}const b=form.querySelector("button:not([type=button])");b.click();};
const apriRFQ=num=>{click('[data-tab="gest"]');click('[data-gfase="tutte"]');click(byText("#gbody tr.click",num),"RFQ "+num);};
const paAv=()=>{const a=q("#pa-avviso")||q("#pa-avviso-cas");return a?{esito:a.getAttribute("data-esito"),testo:a.textContent}:{esito:null,testo:""};};
const ultimo=()=>ev("PA.avviso")||{esito:null,testo:""};
const S2=e=>ev(`(x=>${e})(a2Di(threadAperto()))`);
const cidDi=s=>ev(`(righeB(threadAperto(),prodottoScelto(threadAperto())).find(r=>r.cod.endsWith(${JSON.stringify(s)}))||{}).cid`);
const ciclo=s=>JSON.parse(ev(`JSON.stringify(cicloB(threadAperto(),${JSON.stringify(cidDi(s))})||null)`));
const fotoA=()=>ev("JSON.stringify((t=>[t.prodotti.map(p=>[p.fonte,p.nodi.map(n=>[n.nodo.rif,n.tipo,n.classificazione,n.nodo.decisione,n.parentela]),p.bom]),t.componenti,t.relazioni,t.documenti,t.testa])(threadAperto()))");
const nReg=()=>ev("REGISTRO.length");
const ruolo=r=>ev(`S.ruolo=${JSON.stringify(r)}`);
const albero=()=>JSON.parse(ev("JSON.stringify(alberoProcesso(threadAperto(),prodottoScelto(threadAperto())))"));
const nodi=(n,out=[])=>{ if(!n) return out; out.push(n); (n.figli||[]).forEach(x=>nodi(x,out)); return out; };
const gesto=(url,campi)=>JSON.parse(ev(`JSON.stringify(eseguiB(${JSON.stringify(url)},${JSON.stringify(campi||{})}))`));
const ROTTE=ev("ROTTE.map(compilaRotta).map(r=>r.source)").map(s=>new RegExp(s));
const FASI0=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).filter(([k,T])=>T.testa.fase).map(([k,T])=>[k,[T.testa.fase.nome,T.testa.stato_thread]])))"));
function Z(sez){
  const ora=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).filter(([k,T])=>T.testa.fase).map(([k,T])=>[k,[T.testa.fase.nome,T.testa.stato_thread]])))"));
  t(`Z1 [${sez}] fase e thread invariati`,Object.keys(FASI0).every(k=>JSON.stringify(FASI0[k])===JSON.stringify(ora[k])));
  t(`Z4 [${sez}] ogni data-ma-gesto nella lista chiusa`,qa("[data-ma-gesto]").every(e=>ROTTE.some(r=>r.test(e.getAttribute("data-ma-gesto")))));
  t(`Z4 [${sez}] nessun data-pa-gesto nella lista`,qa("[data-pa-gesto]").every(e=>!ROTTE.some(r=>r.test(e.getAttribute("data-pa-gesto")))));
  t(`Z4 [${sez}] nessun valore con nodo: o componente:`,qa("input,option,[data-ma-campi],[data-pa-campi],[data-pp-nodo]").every(e=>!/^(nodo|componente):/.test(e.value||"")&&!/(nodo|componente):/.test(e.getAttribute("data-pa-campi")||"")&&!/(nodo|componente):/.test(e.getAttribute("data-pp-nodo")||"")));
  t(`Z5 [${sez}] errori JS = 0`,errori.length===0||(console.log("   JS: "+errori.slice(0,3).join(" | ")),false));
}
function Z2(sez){
  t(`Z2 [${sez}] nessun input spuntato`,!qa("input[type=checkbox],input[type=radio]").some(x=>x.checked));
  t(`Z2 [${sez}] nessuna tendina con un valore scelto`,qa("select").every(s=>s.value===""));
  t(`Z2 [${sez}] nessuna qta scritta`,qa('input[name=qta]').every(x=>x.value===""));
}
const apriFase=fid=>{ if(ev("S.faseAperta")!==fid) click(`[data-fase-apri="${fid}"]`,"fase "+fid); };
const visibili=root=>[...(root||d).querySelectorAll("button,summary")].filter(b=>!b.closest("details:not([open]) > :not(summary)")&&!b.closest("[hidden]"));

/* ================= preparazione: VFR con l'albero confermato e Come smistato ================= */
ruolo("Lino");
apriRFQ("482-00");
click('[data-vista="distinta"]');
click('[data-ma-azione="step"]');const cb=q('.ma-finestra input[name=autorizzo]');cb.checked=true;cb.dispatchEvent(new w.Event("change",{bubbles:true}));click('[data-ma-azione="step-invia"]');
click('[data-ma-azione="albero"]');click('[data-ma-azione="albero-invia"]');
t("preparazione: BOM verificata",ev("threadAperto().prodotti[0].bom.verificata")===true);
const A0=fotoA();
click('.staz[data-staz="2"]');tasto("Enter");tasto("3");for(let i=0;i<6;i++) tasto("Enter");
t("preparazione: Come smistato 8/8, cicli in bozza",ciclo(".000")&&ciclo(".000").modello==="T2"&&ciclo(".008"));

/* ================= C2-01 · il PDF del prodotto finito come primo disegno ================= */
click('.staz[data-staz="3"]');
Z2("Ciclo");
t("C2-01 entrando nel Ciclo è scelta «Prodotto finito»",!!q('[data-dis-ciclo="prodotto"][aria-pressed="true"]'));
t("C2-01 mostra lo schizzo del 2D del prodotto (F+800.514263.000.pdf)",q(".dis-file").textContent.includes("F+800.514263.000.pdf")&&!!q('.cl-disegno .ma-palco svg[aria-label*="F+800.514263.000.pdf"]'));
t("C2-01 l'anteprima porta la rotta /allegato/{id}/anteprima?v=<sha256> (pdf.js, mai iframe)",/^\/allegato\/[0-9a-f-]{36}\/anteprima\?v=[0-9a-f]{64}$/.test(q(".cl-disegno .ma-palco").getAttribute("data-anteprima")||"")&&!q(".cl-disegno iframe"));
t("C2-01 il 2D del prodotto solo proposto porta il segno «proposto»",q(".dis-file").textContent.includes("proposto"));
t("C2-01 «schizzo di esempio» resta scritto (mai un disegno inventato, RV-23)",q(".cl-disegno svg").getAttribute("aria-label").includes("schizzo di esempio"));
t("C2-01 B1: sul prodotto finito la scheda «Componente» è spenta, con il motivo",q(".cl-riga--su").textContent.includes(".000")&&!!byText(".dis-schede button[disabled]","Componente")&&byText(".dis-schede button[disabled]","Componente").title.includes("prodotto finito"));
click(byText(".cl-riga",".008"));click('.staz[data-staz="2"]');click('.staz[data-staz="3"]');tasto("v");
t("C2-01 V → Componente",ev("S.disCiclo")==="componente"&&!!q('[data-dis-ciclo="componente"][aria-pressed="true"]'));
tasto("v");t("C2-01 V → torna al Prodotto finito",ev("S.disCiclo")==="prodotto");
t("C2-01 senza anteprima: «anteprima non disponibile»",ev(`disegnoProdotto(threadAperto(),Object.assign({},prodottoScelto(threadAperto()),{componente_id:null,rif:"x"})).testo`).startsWith("anteprima non disponibile"));
t("C2-01 uno STEP: «anteprima non disponibile: è uno STEP»",ev(`(()=>{const T=threadAperto(),p=prodottoScelto(T),v=indiceDi(T).Voce(p.rif,p.componente_id,"disegno_2d"),f=v.disegni.primario.formato;v.disegni.primario.formato="STEP";const r=disegnoProdotto(T,p).testo;v.disegni.primario.formato=f;return r;})()`)==="anteprima non disponibile: è uno STEP");
click(byText(".cl-riga",".008"));t("C2-01 clic su un componente → «Componente»",ev("S.disCiclo")==="componente");
click('.staz[data-staz="2"]');click('.staz[data-staz="3"]');t("C2-01 rientrando nella stazione → di nuovo «Prodotto finito»",ev("S.disCiclo")==="prodotto");
tasto("g");tasto("2");tasto("g");tasto("3");t("C2-01 anche con G 3",ev("S.disCiclo")==="prodotto"&&ev("S.staz")===3);

/* ================= C2-02 · da «Apri nel Ciclo →» della Struttura si apre il componente ================= */
click('.staz[data-staz="1"]');click('[data-vista="distinta"]');
click(byText(".ma-nodo","514263.008"));
click(q('#ma-anteprima [data-staz="3"][data-ciclo-cid]'),"Apri nel Ciclo");
t("C2-02 dalla Struttura: il Ciclo apre il componente, non il prodotto",ev("S.staz")===3&&ev("S.disCiclo")==="componente"&&q(".cl-riga--su").textContent.includes(".008"));

/* ================= C2-19 · Ciclo semplificato: «⋯» per fase, Alt+↑↓, pulsanti ================= */
const b008=visibili(q(".ws-main")).length;
t(`C2-19 .008 all'apertura: ≤ 42 pulsanti visibili nella stazione (${b008})`,b008<=42);
t("C2-19 niente ↑ ↓ ✕ sulle righe: un «⋯» sulla fase aperta",qa(".fase-t .fase-az button").every(b=>b.textContent.trim()==="⋯")&&qa(".fase-piu").length===1&&!!q(".fase--aperta .fase-piu"));
const f8=ciclo(".008").fasi;apriFase(f8[0].id);click(`[data-fase-menu="${f8[0].id}"]`);
t("C2-19 il «⋯» contiene Sposta su, Sposta giù, Togli",["Sposta su","Sposta giù","Togli"].every(x=>!!byText(".fase-az .seq-menu button",x)));
click(byText(".fase-az .seq-menu button","Sposta giù"));
t("C2-19 Sposta giù dal «⋯»: una scrittura, l'ordine cambia",ciclo(".008").fasi[1].id===f8[0].id);
apriFase(f8[0].id);tasto("ArrowUp",{altKey:true});
t("C2-19 Alt+↑ sulla fase aperta la riporta su",ciclo(".008").fasi[0].id===f8[0].id);
t("C2-19 un solo «+ nota» (sotto il disegno), legato alla fase aperta",qa(".ws-main button").filter(b=>/\+ nota/.test(b.textContent)).length===1);
t("C2-19 «Requisiti» chiuso è un chip solo",qa(".req-riga .req-chip").length===0&&q(".req-tit").textContent.includes("senza fase"));
click('.req-tit');t("C2-19 aperto mostra l'elenco con la decisione 1",qa(".req-riga .req-chip").length>0&&q(".req-riga").textContent.includes("dec. 1"));click('.req-tit');
t("C2-19 niente marchi «dopo A2» dentro la stazione: una fascia",qa(".ciclo .banda-b").length===1&&!qa(".ciclo .marchio,.ciclo .chip-b").some(x=>/dopo A2|R-\d/.test(x.textContent)));

/* ================= C2-03 · pre-unione dal Ciclo (gambe: .002 + .008 prima del telaio) ================= */
click(byText(".cl-riga",".000"));
const fS=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id;
apriFase(fS);
t("C2-03 «Entrano» è una scelta dei figli del padre (8 dalla BOM confermata)",qa(`.fase--aperta [data-pa-gesto="/fase/${fS}/componente"]`).length===8);
t("C2-03 la proposta ◇ è un seme: .002 e .008 nella saldatura principale",ciclo(".000").fasi.find(f=>f.id===fS).entrano.some(e=>e.cid===cidDi(".002")&&e.fonte==="proposta"));
const r0=nReg();
click(`[data-seq-menu="prima|${fS}|ciclo"]`);click(byText(".fase--aperta .seq-menu button","Saldatura MIG/MAG"));
const fP=ciclo(".000").fasi[0];
t("C2-03 «+ fase di unione prima» → una fase VUOTA subito prima, una scrittura",paAv().esito==="ok"&&fP.p==="sald_mag"&&fP.id!==fS&&(fP.entrano||[]).length===0&&nReg()===r0+1&&ciclo(".000").fasi[1].id===fS);
t("C2-03 la fase nuova è aperta, nessun figlio dentro",ev("S.faseAperta")===fP.id);
const chip=(fid,suf)=>qa(`[data-pa-gesto="/fase/${fid}/componente"]`).find(b=>b.textContent.includes(suf));
click(chip(fP.id,".002"),".002 entra");click(chip(fP.id,".008"),".008 entra");
const c0=ciclo(".000"), eP=c0.fasi[0].entrano.map(e=>e.cid).sort(), eS=c0.fasi.find(f=>f.id===fS).entrano.map(e=>e.cid);
t("C2-03 .002 e .008 entrano nella pre-unione e lasciano la saldatura principale",JSON.stringify(eP)===JSON.stringify([cidDi(".002"),cidDi(".008")].sort())&&!eS.includes(cidDi(".002"))&&!eS.includes(cidDi(".008")));
t("C2-03 il titolo della fase dice «pre-unione»",q(`[data-fase="${fP.id}"] .fase-t`).textContent.includes("pre-unione"));
t("C2-03 fonte della sequenza: scritta da Lino",c0.fonteSeq==="L");
click('.staz[data-staz="4"]');
let A=albero(), uP=nodi(A).find(n=>n.tipo==="unione"&&n.fid===fP.id);
t("C2-03 nella Sequenza lo stesso rombo, con gli stessi figli",!!uP&&uP.preUnione&&uP.figli.filter(x=>x.tipo==="componente").map(x=>x.cid).sort().join()===eP.join()&&!!q(`.pp-unione[data-pp-fid="${fP.id}"]`));
t("C2-11 una pre-unione è un rombo, mai una casella",!q(`.pp-box[data-pp-fid="${fP.id}"]`)&&!!q(`.pp-unione--pre[data-pp-fid="${fP.id}"] .pp-rombo`));

/* ================= C2-10 · l'albero del processo è un componente separato ================= */
apriRFQ("482-00");click('.staz[data-staz="4"]');
Z2("Sequenza");
t("C2-10 fuori da #ma, nessun data-ma-* né classe ma-* nell'albero",!q("#ma .pp-albero")&&!qa(".pp-albero *").some(e=>[...e.attributes].some(a=>a.name.startsWith("data-ma-"))||[...e.classList].some(c=>c.startsWith("ma-"))));
t("C2-10 role=tree, uno solo, nessuna tablist nella stazione",qa('[role="tree"]').length===1&&!q('.seq16 [role="tablist"]'));
t("C2-10 chiavi dei nodi solo cid/fid (nessun «nodo:»)",qa("[data-pp-nodo]").every(e=>!/nodo:|componente:/.test(e.getAttribute("data-pp-nodo")))&&qa("[data-pp-cid]").every(e=>/^[0-9a-f-]{36}$|^$/.test(e.getAttribute("data-pp-cid"))));
t("C2-10 nessun nodo scelto all'apertura",!qa('[aria-selected="true"]').length&&q(".seq16-pannello").textContent.includes("Scegli un nodo"));
t("C2-10 treeitem con aria-level e aria-label a parole",qa('[role="treeitem"]').every(e=>e.getAttribute("aria-level")&&e.getAttribute("aria-label")));
/* C2-11 ogni figlio sotto un'unione del SUO padre o nel riquadro «senza fase» */
A=albero();
t("C2-11 ogni figlio di …000 compare sotto un'unione di …000 o in «senza fase»",(()=>{const pad=ev(`JSON.stringify(righeB(threadAperto(),prodottoScelto(threadAperto())).find(r=>r.radice).figli.map(x=>x.cid))`);const figli=JSON.parse(pad);const sotto=nodi(A).filter(n=>(n.tipo==="unione"||n.tipo==="senza_fase")&&n.cid===A.cid).flatMap(n=>n.figli.filter(x=>x.tipo==="componente").map(x=>x.cid));return figli.every(c=>sotto.includes(c));})());
t("C2-11 l'unione principale ha rif proposto il più pesante (.001, CALC)",(()=>{const u=nodi(A).find(n=>n.tipo==="unione"&&n.principale);return u&&u.rif&&u.rif.cod.endsWith(".001")&&u.rif.fonte==="CALC";})());
t("C2-20 stadi letti: la saldatura è U, il controllo finale C (non salvati)",(()=>{const u=nodi(A).find(n=>n.tipo==="unione"&&n.principale);return u.stadio==="U"&&(A.ultime||[]).some(f=>f.st==="C");})());
const bSeq=visibili(q(".ws-main")).length;
t(`C2-20 Sequenza VFR all'apertura: ≤ 30 pulsanti visibili (${bSeq})`,bSeq<=30);
t("C2-20 un solo primario, nella barra in basso: «Conferma la sequenza ⌃↵»",qa(".ws-main .ma-btn--pri").length===1&&!!q(".seq-barra [data-conferma-seq].ma-btn--pri"));

/* ================= C2-04 · pre-unione dalla Sequenza (dopo il ripristino) ================= */
t("C2-08 fonte «scritta da te» (dal Ciclo) e il ripristino si offre",q(".seq-barra").textContent.includes("scritta da te")&&!!q('[data-seq-ripristina="chiedi"]'));
click('[data-seq-ripristina="chiedi"]');
t("C2-08 A1: «Ripristina la proposta» chiede prima e dice che cosa si toglie",q(".seq-chiedi").textContent.includes("Torna la proposta del sistema?")&&q(".seq-chiedi").textContent.includes("Si tolgono:")&&q(".seq-chiedi").textContent.includes("f.10"));
t("C2-08 B-9: il fuoco va su «No»",d.activeElement&&d.activeElement.id==="seq-chiedi-no");
click('[data-seq-ripristina="no"]');t("C2-08 No: niente cambia",ciclo(".000").fonteSeq==="L"&&ciclo(".000").fasi[0].id===fP.id);
click('[data-seq-ripristina="chiedi"]');click('[data-seq-ripristina="si"]');
t("C2-08 Sì: torna la proposta, la pre-unione aggiunta a mano è tolta",ciclo(".000").fonteSeq==="proposta"&&!ciclo(".000").fasi.some(f=>f.id===fP.id)&&ciclo(".000").fasi.find(f=>f.id===fS).entrano.some(e=>e.cid===cidDi(".002")));
click(q(`.pp-unione[data-pp-fid="${fS}"]`),"rombo della saldatura");
t("C2-04 clic sul rombo: il pannello dello stadio con «Entrano»",ev("S.seqScelto.fid")===fS&&q(".seq16-pannello").textContent.includes("Entrano")&&qa(`.seq16-pannello [data-pa-gesto="/fase/${fS}/componente"]`).length===8);
click(`.seq16-pannello [data-seq-menu="prima|${fS}|seq"]`);click(byText(".seq16-pannello .seq-menu button","Saldatura MIG/MAG"));
const fP2=ciclo(".000").fasi[0].id;
t("C2-04 la pre-unione nasce vuota e diventa il nodo scelto",ev("S.seqScelto.fid")===fP2&&(ciclo(".000").fasi[0].entrano||[]).length===0);
const r1=nReg();
click(chip(fP2,".002"),".002");
t("C2-05 spostare un figlio fra stadi: una scrittura sola, non resta in due fasi",nReg()===r1+1&&ciclo(".000").fasi.filter(f=>(f.entrano||[]).some(e=>e.cid===cidDi(".002"))).length===1);
click(chip(fP2,".008"),".008");
t("C2-04 nel Ciclo lo stesso dato (la fase 10 ha .002 e .008)",(()=>{click('.staz[data-staz="3"]');click(byText(".cl-riga",".000"));apriFase(fP2);const ok=q(`[data-fase="${fP2}"]`).textContent.includes("pre-unione")&&qa(`[data-fase="${fP2}"] [data-pa-gesto="/fase/${fP2}/componente"][aria-pressed="true"]`).length===2;click('.staz[data-staz="4"]');return ok;})());
/* C2-05 dividi */
ev(`S.seqScelto={tipo:"unione",cid:${JSON.stringify(cidDi(".000"))},fid:${JSON.stringify(fS)},padre:null}`);ev("render()");
click(`.seq16-pannello [data-seq-dividi="${fS}"]`);
const fd=q(".seq16-pannello form.ent-dividi");
t("C2-05 «dividi…»: campo obbligatorio e VUOTO, figlio non scelto",!!fd&&fd.querySelector("input[name=qta]").value===""&&fd.querySelector("input[name=qta]").required&&fd.querySelector("select[name=cid]").value==="");
const firma0=ev("firmaSequenza(threadAperto(),prodottoScelto(threadAperto()))");
t("C2-05 dividi con la qta vuota → no, niente cambia",gesto(`/fase/${fS}/componente`,{cid:cidDi(".009"),entra:"1",dividi:"1",qta:""}).esito==="no"&&ev("firmaSequenza(threadAperto(),prodottoScelto(threadAperto()))")===firma0);
{const fd2=q(".seq16-pannello form.ent-dividi");fd2.querySelector("select[name=cid]").value=cidDi(".009");fd2.querySelector("input[name=qta]").value="4";submit(fd2,"dividi 4 su 8");}
t("C2-05 4 su 8 di .009 anche in questa fase → SEQ-08 (le quantità non tornano)",ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==="SEQ-08")`));
gesto(`/fase/${fP2}/componente`,{cid:cidDi(".009"),entra:"1",dividi:"1",qta:"4"});
t("C2-05 l'altra metà entra nella pre-unione → SEQ-08 sparisce",(()=>{const c=ciclo(".000");return !ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==="SEQ-08")`)&&c.fasi.find(f=>f.id===fS).entrano.find(e=>e.cid===cidDi(".009")).qta===4;})());
gesto(`/fase/${fS}/componente`,{cid:cidDi(".009"),entra:"1"});
t("C2-05 un clic senza «dividi» rimette tutta la quantità in una fase",ciclo(".000").fasi.filter(f=>(f.entrano||[]).some(e=>e.cid===cidDi(".009"))).length===1);
/* C2-06 */
const firma1=ev("firmaSequenza(threadAperto(),prodottoScelto(threadAperto()))");
const nf=gesto(`/fase/${fS}/componente`,{cid:cidDi(".000"),entra:"1"});
t("C2-06 un cid che non è figlio del padre → no (SEQ-01, R-38) e niente cambia",nf.esito==="no"&&nf.testo.includes("SEQ-01")&&nf.testo.includes("R-38")&&ev("firmaSequenza(threadAperto(),prodottoScelto(threadAperto()))")===firma1);
t("C2-06 nessun componente entra in una fase che non è di unione",gesto(`/fase/${ciclo(".000").fasi.find(f=>f.p==="controllo").id}/componente`,{cid:cidDi(".009"),entra:"1"}).esito==="no");

/* ================= C2-07 · Rinomina, sposta, togli ================= */
ev(`S.seqScelto={tipo:"unione",cid:${JSON.stringify(cidDi(".000"))},fid:${JSON.stringify(fP2)},padre:null}`);ev("render()");
click(`.seq16-pannello [data-seq-menu="fase|${fP2}"]`);click('.seq16-pannello [data-seq-rinomina]');
const rin=q("#seq-rin-in");rin.value="Saldatura gambe";submit(q("form.seq-rin"),"rinomina");
t("C2-07 Rinomina: va nel nome della fase (nota R-10), la lavorazione resta",ciclo(".000").fasi[0].par.nome==="Saldatura gambe"&&ciclo(".000").fasi[0].p==="sald_mag"&&!!byText(".pp-unione","Saldatura gambe"));
click(`.seq16-pannello [data-seq-menu="stadio|${fP2}"]`);click(byText(".seq16-pannello .seq-menu button","Foratura e filettatura"));
const fF=ciclo(".000").fasi[1];
t("C2-07 «+ stadio ▾» → Foratura dopo la pre-unione",fF.p==="foratura"&&ev("S.seqScelto.fid")===fF.id);
t("C2-07 la foratura lavora «sulla pre-unione»: si legge sul ramo e nel pannello",q(".seq16-pannello").textContent.includes("pre-unione della f.10")&&!!byText(".pp-ramo-f","Foratura"));
ev(`S.seqScelto={tipo:"stadio",cid:${JSON.stringify(cidDi(".000"))},fid:${JSON.stringify(fF.id)},padre:null}`);ev("render()");
tasto("ArrowUp",{altKey:true});
t("C2-07 Alt+↑ sposta lo stadio scelto",ciclo(".000").fasi[0].id===fF.id);
tasto("ArrowDown",{altKey:true});
t("C2-07 Alt+↓ lo rimette dopo la pre-unione",ciclo(".000").fasi[1].id===fF.id);
ev(`S.seqScelto={tipo:"unione",cid:${JSON.stringify(cidDi(".000"))},fid:${JSON.stringify(fP2)},padre:null}`);ev("render()");
click(`.seq16-pannello [data-seq-menu="fase|${fP2}"]`);click(byText(".seq16-pannello .seq-menu button","Togli"));
A=albero();
t("C2-07 togliere uno stadio con figli li manda «in nessuna fase» (SEQ-01)",!ciclo(".000").fasi.some(f=>f.id===fP2)&&nodi(A).some(n=>n.tipo==="senza_fase"&&n.cods.some(c=>c.endsWith(".002")))&&!!q(".pp-box--senza"));
click(q(".pp-box--senza"),"senza fase");
t("C2-13 clic sul ⚠ «senza fase» → il pannello con il punto da correggere",q(".seq16-pannello").textContent.includes("Non entrano in nessuna fase")&&qa(".seq16-pannello [data-seq-entra]").length>=1);
cambia(q('.seq16-pannello [data-seq-entra]'),fS,"entra in");
cambia(q('.seq16-pannello [data-seq-entra]'),fS,"entra in");
t("C2-07 da «senza fase» con «entra in ▾»: tornano nella saldatura",!nodi(albero()).some(n=>n.tipo==="senza_fase"));

/* ================= C2-08 · la proposta non torna da sola; C2-09 conferma con la firma ================= */
apriRFQ("482-00");click('.staz[data-staz="4"]');
t("C2-08 riaprendo la stazione la proposta NON torna (scritta da te)",ciclo(".000").fonteSeq==="L"&&q(".seq-barra").textContent.includes("scritta da te"));
const fv=ev("firmaSequenza(threadAperto(),prodottoScelto(threadAperto()))");
gesto(`/fase/${fS}`,{rif:cidDi(".006"),fonte:"L"});
const vecchia=ev(`SERVER_B.posta(threadAperto(),prodottoScelto(threadAperto()),"/prodotto/${cidDi(".000")}/sequenza/conferma",new Map([["firma",${JSON.stringify(fv)}]]))`);
t("C2-09 una scrittura fra lettura e conferma → no, «rileggi»",vecchia.esito==="no"&&vecchia.testo.includes("rileggi")&&ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")!=="confermata");
tasto("Enter",{ctrlKey:true});
t("C2-09 ⌃↵: confermata con la firma, NON con la bozza intera",ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="confermata"&&ev("REGISTRO[0].url").endsWith("/sequenza/conferma")&&!!ev("REGISTRO[0].campi.firma")&&ev("REGISTRO[0].campi.bozza")===undefined);
t("C2-09 nessuna bozza della sequenza nella pagina (via S.seqBozza)",ev("S.seqBozza")===undefined&&!html.includes("seqBozza"));
t("C2-09 la barra dice «confermata»",q(".seq-barra").textContent.includes("confermata"));
gesto(`/fase/${fS}`,{rif:"",fonte:"L"});
t("C2-09 una modifica dopo la conferma la riapre",ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="bozza");
t("B1 Sequenza: albero, tipi, documenti della zona A invariati",fotoA()===A0);
Z("VFR · Sequenza");

/* ================= C2-20 · i tasti della Sequenza ================= */
click(q(`.pp-box[data-pp-cid="${cidDi(".006")}"]`),".006");
t("C2-20 clic su un figlio: pannello con «entra in ▾», da proteggere, riferimento",!!q(`.seq16-pannello [data-seq-entra="${cidDi(".006")}"]`)&&q(".seq16-pannello").textContent.includes("riferimento"));
const uu=ev(`JSON.stringify(unioniDi(cicloB(threadAperto(),${JSON.stringify(cidDi(".000"))})).map(f=>f.id))`);const U=JSON.parse(uu);
tasto("2");
t("C2-20 tasto 2: il figlio entra nella 2ª fase di unione (dadi saldati)",ciclo(".000").fasi.find(f=>f.id===U[1]).entrano.some(e=>e.cid===cidDi(".006")));
tasto("1");
t("C2-20 tasto 1: torna nella 1ª",ciclo(".000").fasi.find(f=>f.id===U[0]).entrano.some(e=>e.cid===cidDi(".006")));
tasto("c");
t("C2-20 C: riferimento della fase di unione",ciclo(".000").fasi.find(f=>f.id===U[0]).par.rif===cidDi(".006"));
tasto("c");
t("C2-20 M senza trattamento nel padre → no, con il motivo",(tasto("m"),ultimo().esito==="no"&&ultimo().testo.includes("trattamento")));
const prima=ev("S.seqScelto.cid");tasto("Enter");
t("C2-20 Invio tiene la proposta e scende al figlio dopo",ev("S.seqScelto.cid")!==prima&&ev("S.seqScelto.tipo")==="componente");
click(q(`.pp-box[data-pp-cid="${cidDi(".001")}"]`),".001");tasto("ArrowRight");
t("C2-20 M-1: → va al nodo accanto (come nella Struttura)",ev("S.seqScelto.tipo")==="componente"&&ev("S.seqScelto.cid")!==cidDi(".001"));
tasto("ArrowLeft");t("C2-20 M-1: ← torna al nodo accanto",ev("S.seqScelto.cid")===cidDi(".001"));
tasto("ArrowUp");t("C2-20 M-1: ↑ va al nodo sopra (il rombo della sua unione)",ev("S.seqScelto.tipo")==="unione");
tasto("ArrowDown");t("C2-20 M-1: ↓ scende al primo figlio",ev("S.seqScelto.tipo")==="componente"||ev("S.seqScelto.tipo")==="unione");
tasto("Home");t("C2-20 M-1: Home va al primo nodo (il prodotto)",ev("S.seqScelto.tipo")==="prodotto");
tasto("Escape");t("C2-20 Esc richiude il pannello",ev("S.seqScelto")===null);
t("C2-20 «da proteggere» al posto di «maschera M» (tasti nel «?»)",ev("AIUTO_STAZ[4].tasti").some(x=>x[0]==="M"&&/da proteggere/.test(x[1]))&&!html.includes('data-seq-masch'));

/* ================= C2-14 · preparazioni: in maschera su una pre-unione, due maschere ================= */
click('.staz[data-staz="3"]');click(byText(".cl-riga",".000"));
const fP3=gesto(`/componente/${cidDi(".000")}/ciclo/fase`,{p:"sald_mag",prima:fS}).fid;
gesto(`/fase/${fP3}/componente`,{cid:cidDi(".002"),entra:"1"});gesto(`/fase/${fP3}/componente`,{cid:cidDi(".008"),entra:"1"});
apriFase(fS);
click(q(`.fase--aperta [data-pa-spunta="${fS}|maschera|sì"]`),"in maschera (assieme), con un clic vero");
t("C2-14 un clic vero sulla spunta scrive (il foglio non si ridisegna prima del «change»)",ciclo(".000").fasi.find(f=>f.id===fS).par.maschera==="sì");
apriFase(fP3);
cambia(q(`.fase--aperta [data-pa-spunta="${fP3}|maschera|sì"]`),true,"in maschera (pre-unione)");
let preps=JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()))")).filter(x=>x.k==="maschera");
t("C2-14 due unioni in maschera = due maschere, ognuna con la SUA fase",preps.length===2&&preps.some(x=>x.fid===fP3&&/pre-unione/.test(x.fase))&&preps.some(x=>x.fid===fS));
t("C2-14 sotto «in maschera»: Maschera · da fare · Richiedi: Interno · Fornitore",q(`[data-fase="${fP3}"] .prep-mini`).textContent.includes("da fare")&&!!byText(`[data-fase="${fP3}"] .prep-mini button`,"Interno")&&!!byText(`[data-fase="${fP3}"] .prep-mini button`,"Fornitore"));
t("C2-14 il riepilogo in testa: «Preparazioni · 2 · maschera da quotare …»",q(".prep>summary").textContent.includes("2")&&q(".prep>summary").textContent.includes("da quotare"));
click('.staz[data-staz="4"]');
t("C2-13 nell'albero: la chiave inglese con la parola «da quotare» sulla pre-unione",!!q(`.pp-unione[data-pp-fid="${fP3}"] .pp-prep--ver`)&&q(`.pp-unione[data-pp-fid="${fP3}"] .pp-prep`).textContent.includes("da quotare"));
click(q(`.pp-unione[data-pp-fid="${fP3}"] .pp-prep`));
t("C2-13 clic sulla chiave inglese → la riga della preparazione nel Ciclo",ev("S.staz")===3&&!!ev("S.prepEvid")&&!!q(".prep[open] .prep-riga--evid"));
click('.staz[data-staz="4"]');
const sg=q(`.pp-box[data-pp-cid="${cidDi(".008")}"] .pp-sg`);const sgFid=(sg.getAttribute("data-vai")||"").split("|")[2];
click(sg);t("C2-13 clic su una sigla di fase → Ciclo con quella fase aperta",ev("S.staz")===3&&ev("S.faseAperta")===sgFid&&ev("S.cicloCid")===cidDi(".008"));

/* ================= C2-15 · «Richiedi: Fornitore» fino alla registrazione e all'Offerta ================= */
click(byText(".cl-riga",".000"));apriFase(fS);
click(byText(`[data-fase="${fS}"] .prep-mini button`,"Fornitore"),"Richiedi: Fornitore");
const cA=()=>q('#cassetto .rq[data-rq="attrezzatura"]'), rfA=n=>cA()?[...cA().querySelectorAll(".rq-forn")].find(x=>x.textContent.includes(n)):null;
t("C2-15 P-02: «Fornitore» apre la carta «Attrezzature» nel cassetto; il clic non crea richieste",ev("S.cassetto")==="richieste"&&!!cA()&&!S2("x.richieste.some(q=>q.tipo==='attrezzatura')"));
t("C2-15 la carta porta le due maschere, ognuna con la sua fase (pre-unione e assieme)",/pre-unione/.test(cA().textContent)&&(cA().querySelector("summary").textContent.match(/maschera/gi)||[]).length>=2);
t("C2-15 M7: Robosald, Minitool e Dalmi con la descrizione; nessuna richiesta scelta al posto tuo",["Robosald","Minitool","Dalmi"].every(n=>!!rfA(n))&&rfA("Minitool").textContent.includes("set di curvatura e utensili")&&!S2("x.richieste.length"));
t("C2-15 la nota è precompilata con la frase della maschera",q('#cassetto [data-rq-nota="attrezzatura"]').value.includes("maschera di saldatura per il codice F+800.514263.000"));
click([...rfA("Robosald").querySelectorAll("button")].find(b=>b.textContent.includes("Prepara la bozza")),"Prepara la bozza Robosald");
const idM=S2(`(x.richieste.find(q=>q.forn&&q.forn.id==="robosald")||{}).id`);
t("C2-15 UNA riga per Robosald (lavorazione «attrezzature») con le due maschere: il legame si legge dalle note (P-10)",!!idM&&S2(`x.richieste.find(q=>q.id==="${idM}").lavorazione`)==="attrezzature"&&S2(`x.richieste.find(q=>q.id==="${idM}").preps.length`)===2&&S2(`x.richieste.find(q=>q.id==="${idM}").note`).includes("rif. Cockpit: attrezzatura"));
t("C2-15 allegati: PDF del pezzo, 3D, PDF del prodotto finito",S2(`x.richieste.find(q=>q.id==="${idM}").allegati.join()`).includes("F+800.514263.000.pdf")&&S2(`x.richieste.find(q=>q.id==="${idM}").allegati.join()`).includes("(3D)"));
gesto(`/mockup/richiesta/${idM}/sync`,{});
t("C2-15 «inviata» la mette il sync (stati di Pietro)",S2(`x.richieste.find(q=>q.id==="${idM}").stato`)==="inviata");
gesto(`/mockup/richiesta/${idM}/risposta`,{});
t("C2-15 Simula → offerta di Robosald con costo e settimane",S2(`x.richieste.find(q=>q.id==="${idM}").stato`)==="offerta_ricevuta"&&S2(`x.richieste.find(q=>q.id==="${idM}").risp.settimane`)===4);
gesto(`/rfq/${ev("threadAperto().id")}/risposta`,{richiesta:idM});
preps=JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()))")).filter(x=>x.k==="maschera");
t("C2-15 Registra → costo, settimane, chi e fonte RISP in tutte e due le preparazioni",preps.every(x=>x.costo&&x.settimane&&x.fonte==="RISP"&&x.chi==="Robosald-Gasitalia")&&preps.find(x=>x.fid===fS).costo==="1300"&&preps.find(x=>x.fid===fP3).costo==="720");
t("C2-15 la richiesta non diventa «registrata»: resta «offerta ricevuta» (P-02)",S2(`x.richieste.find(q=>q.id==="${idM}").stato`)==="offerta_ricevuta");
t("C2-15 il costo arriva da solo nel riepilogo per l'Offerta (attrezzaturaPronta)",ev("attrezzaturaPronta(threadAperto(),prodottoScelto(threadAperto())).ok")===true&&ev("attrezzaturaPronta(threadAperto(),prodottoScelto(threadAperto())).settimaneMax")===4);
ev("S.cassetto=null;render()");apriFase(fS);
t("C2-15 nel Ciclo: «Robosald-Gasitalia 1.300 € · 4 sett.» sotto in maschera",q(`[data-fase="${fS}"] .prep-mini`).textContent.includes("1.300")&&q(`[data-fase="${fS}"] .prep-mini`).textContent.includes("4 sett."));
t("C2-15 M2: il riepilogo dice la fase di ogni maschera",/maschera f\.\d+ pre-unione/.test(q(".prep>summary").textContent));
t("C2-15 nell'albero la chiave inglese diventa «quotata»",(click('.staz[data-staz="4"]'),!!q(`.pp-unione[data-pp-fid="${fS}"] .pp-prep--ok`)));

/* ================= C2-16 · interna all'officina; scaduta → PREP-02 ================= */
click('.staz[data-staz="3"]');click(byText(".cl-riga",".000"));
gesto(`/componente/${cidDi(".000")}/preparazioni`,{k:"robot",on:"1",fase_id:fS});
const rr=gesto(`/rfq/${ev("threadAperto().id")}/domanda`,{tipo:"officina",cid:cidDi(".000"),fid:fS,prep:JSON.parse(ev(`JSON.stringify(Object.keys(cicloB(threadAperto(),${JSON.stringify(cidDi(".000"))}).prep).find(k=>k.startsWith("robot")))`))});
t("C2-16 «Richiedi» del robot → bozza interna all'officina (proposta R-28)",rr.esito==="ok"&&S2(`x.richieste.find(q=>q.id==="${rr.rid}").tipo`)==="officina"&&!S2(`x.richieste.find(q=>q.id==="${rr.rid}").forn`));
gesto(`/componente/${cidDi(".000")}/preparazioni`,{k:"piega",on:"1",fase_id:fS});
const rp=gesto(`/thread/${ev("threadAperto().id")}/richiesta`,{fornitore_id:"minitool",lavorazione:"attrezzature",codice:"F+800.514263.000",note:"Attrezzatura di piega\n— rif. Cockpit: attrezzatura",bozza:"1"});
gesto(`/mockup/richiesta/${rp.rid}/sync`,{});gesto(`/mockup/richiesta/${rp.rid}/scadenza`,{});
t("C2-16 richiesta scaduta senza risposta (il server la calcola dalla scadenza, R-27) → PREP-02 DB «si quota con riserva»",S2(`x.richieste.find(q=>q.id==="${rp.rid}").stato`)==="scaduta"&&ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==="PREP-02"&&x.liv==="DB"&&/con riserva/.test(x.testo))`));
t("C2-16 in Esito il componente è DUBBIO",ev(`esitoProposto(threadAperto(),prodottoScelto(threadAperto()),${JSON.stringify(cidDi(".000"))}).v`)!=="OK");

/* ================= C2-18 · «+ preparazione ▾» con i 9 tipi ================= */
click('.prep>summary');if(!q(".prep[open]")) ev("S.prepAperta=true;render()");
click('[data-prep-menu]');
t("C2-18 il menu ha i 9 tipi (con «altra»)",qa(".prep-agg .seq-menu button[role=menuitem]").length===8&&!!q(".prep-agg form.prep-altra"));
t("C2-18 «altra» vuole il nome: senza → no",gesto(`/componente/${cidDi(".000")}/preparazioni`,{k:"altra",on:"1"}).esito==="no");
const fa=q(".prep-agg form.prep-altra");fa.querySelector("input[name=nome]").value="ganci del verniciatore";submit(fa,"altra");
t("C2-18 con il nome: «ganci del verniciatore» da realizzare",JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()))")).some(x=>x.k==="altra"&&x.nome==="ganci del verniciatore"));
t("B1 preparazioni: la zona A non cambia",fotoA()===A0);
Z("VFR · preparazioni");

/* ================= C2-17 · «esiste già» su Potaflex 92176C ================= */
apriRFQ("P-2026-0131");click('.staz[data-staz="2"]');tasto("Enter");click('.staz[data-staz="3"]');
ev("S.prepAperta=true;render()");
click('[data-prep-menu]');click(byText(".prep-agg .seq-menu button","Maschera di saldatura"));
t("C2-17 lo storico propone «◇ esiste già: 92176C (comodato del cliente)»",q(".prep").textContent.includes("esiste già: 92176C (comodato del cliente)"));
click(byText(".prep-riga .seg button","esiste già"));
const p12=JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()))")).find(x=>x.k==="maschera");
t("C2-17 esiste già: costo 0, riferimento «comodato del cliente», fonte STORICO",p12&&p12.costo==="0"&&/comodato del cliente/.test(p12.riferimento)&&p12.fonte==="STORICO"&&p12.stato==="esiste già");
t("C2-17 una preparazione che esiste già non si richiede",!byText(".prep-riga button","Fornitore")&&gesto(`/rfq/${ev("threadAperto().id")}/domanda`,{tipo:"officina",cid:p12.cid,prep:p12.key}).esito==="no"&&!ev(`bisogniRichieste(threadAperto(),prodottoScelto(threadAperto())).some(b=>b.key==="attrezzatura")`));
Z("Potaflex");

/* ================= C2-12 · FitLab: tre livelli, sottoassieme vero, perno in due posti ================= */
apriRFQ("RDO 2026/318");
const A2tg=fotoA();
click('.staz[data-staz="2"]');for(let i=0;i<4;i++) tasto("Enter");
click('.staz[data-staz="4"]');
Z2("Sequenza FitLab");
A=albero();
const sub=nodi(A).find(n=>n.tipo==="componente"&&n.cod==="9N008518AA");
t("C2-12 tre livelli: prodotto → unione → 9N008518AA → le sue unioni → foglie",!!sub&&sub.sottoassieme&&sub.figli.some(x=>x.tipo==="unione"&&x.figli.some(y=>y.cod==="9N008520AA")));
const perni=nodi(A).filter(n=>n.tipo==="componente"&&n.cod==="9N008519AB");
t("C2-12 9N008519AB in due posti, con «⧉ anche sotto»",perni.length===2&&perni.every(x=>x.anche.length===1)&&qa(".pp-anche").length===2);
const uRoot=nodi(A).find(n=>n.tipo==="unione"&&n.cid===A.cid);
t("C2-12 il perno è proposto nel montaggio dopo il trattamento (D, SEQ-06)",uRoot&&uRoot.stadio==="D"&&uRoot.figli.some(x=>x.cod==="9N008519AB"&&x.fonte==="proposta"&&/SEQ-06/.test(x.perche)));
t("C2-12 il sottoassieme ha le sue fasi dopo l'unione (trattamenti ⇄ sul ramo, sabbiatura e polvere)",(sub.dopo||[]).some(f=>f.est&&/Sabbiatura/.test(f.nome))&&!!byText(".pp-ramo-f--est","Sabbiatura"));
/* nel sottoassieme il perno passa dalla saldatura a uno stadio nuovo «Piantaggio» */
const fSub=nodi(A).find(n=>n.tipo==="unione"&&n.cid===sub.cid).fid;
click(q(`.pp-unione[data-pp-fid="${fSub}"]`));
click(`.seq16-pannello [data-seq-menu="stadio|${fSub}"]`);click(byText(".seq16-pannello .seq-menu button","Piantaggio a pressa"));
const fPi=ev("S.seqScelto.fid");
click(chip(fPi,"9N008519AB")||byText(`.seq16-pannello [data-pa-gesto="/fase/${fPi}/componente"]`,"19AB"),"perno nel piantaggio");
A=albero();
t("C2-12 nel sottoassieme il perno è nello stadio «Piantaggio» (una scrittura, 4 gesti)",nodi(A).some(n=>n.tipo==="unione"&&n.fid===fPi&&n.figli.some(x=>x.cod==="9N008519AB"))&&!nodi(A).some(n=>n.tipo==="unione"&&n.fid===fSub&&n.figli.some(x=>x.cod==="9N008519AB")));
click('[data-seq-filtro="sistemare"]');
t("C2-12 filtro «solo da sistemare»: restano i ⚠ e i loro antenati",ev("S.seqFiltro")==="sistemare"&&qa('.pp-albero [data-pp-nodo]:not([data-pp-tipo="stadio"])').length<nodi(A).length&&qa(".pp-albero [data-pp-nodo]").length>=1);
click('[data-seq-filtro="tutto"]');
const vern=nodi(A).flatMap(n=>(n.dopo||[]).concat(n.tra||[],n.ultime||[])).find(f=>f.p==="verniciatura_polvere");
const rv=q(`[data-pp-stadio$="|${vern.fid}"]`);
click(rv,"viaggio verniciatura");
t("C2-12 clic sul viaggio ⇄: pannello dello stadio esterno con i terzisti (nessuno preselezionato) e «Richiesta ai verniciatori»",q(".seq16-pannello").textContent.includes("Richiesta ai verniciatori")&&qa(".seq16-pannello .cb-terzi button").length>=2&&!qa('.seq16-pannello .cb-terzi button[aria-pressed="true"]').length);
click(q(".seq16-pannello .cb-terzi button"),"un terzista");
t("C2-12 un terzista scelto: una scrittura della fase",!!ev(`valF(faseDelThread(threadAperto(),${JSON.stringify(vern.fid)}).f,"terzi")`));
click(byText(".seq16-pannello .seq-prot button","19AB"),"da proteggere il perno");
t("C2-12 «da proteggere» (M) per il perno nella fase di trattamento",String(ev(`valF(faseDelThread(threadAperto(),${JSON.stringify(vern.fid)}).f,"proteggi")`)||"").includes(cidDi("9N008519AB")));
tasto("Enter",{ctrlKey:true});
t("C2-12 ⌃↵: sequenza FitLab confermata (scritta da te)",ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="confermata"&&ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).fonte")==="L");
t("B1 FitLab: la zona A non cambia",fotoA()===A2tg);
Z("FitLab");

/* ================= C2-21 · inventario §6.3 (parte C2): ogni voce raggiungibile ================= */
apriRFQ("482-00");click('.staz[data-staz="3"]');
const inv={
  "Ciclo: filtri da fare · con dubbi · tutti (una tendina, M-8)":!!q("[data-ciclo-filtro-sel]")&&q("[data-ciclo-filtro-sel]").options.length===3,
  "Ciclo: ← → componente":qa("[data-ciclo-nav]").length===2,
  "Ciclo: schede Prodotto finito · Componente":qa(".dis-schede button").length===2,
  "Ciclo: zoom, adatta, nascondi nel ⋯ del disegno":(click('[data-dis-menu]'),qa(".dis-menu button").length===4),
  "Ciclo: visore (P-11: zona B, con allegato e sha256)":!!q('.cl-disegno [data-pa-visore][data-pa-allegato][data-pa-sha]')&&!q('.cl-disegno [data-ma-azione]'),
  "Ciclo: + nota (N)":!!q('form[data-pa-gesto$="/nota"]'),
  "Ciclo: Copia da… nel ▾ e con C":(tasto("c"),!!q(".cl-foglio").textContent.includes("Copia da")),
  "Ciclo: preparazioni con «+ preparazione ▾»":!!q("[data-prep-menu]"),
  "Ciclo: aggiungi una fase (/)":!!q("#agg-fase"),
  "Ciclo: barra con primario e ▾":!!q(".cl-foglio .barra-conf .ma-btn--pri")&&!!q("[data-bc-menu]"),
  "Ciclo: controlli del componente (riquadro che si apre)":!!q('.ctrl[aria-label="Controlli del componente"]'),
  "Ciclo: Vedi nell'albero → (nel ⋯ della fase)":(()=>{const f=ciclo(".000").fasi.find(x=>x.p==="sald_mag");apriFase(f.id);click(`[data-fase-menu="${f.id}"]`);return !!q("[data-vedi-albero]");})(),
};
tasto("Escape");
for(const [k,v] of Object.entries(inv)) t("C2-21 "+k,v);
click(q("[data-vedi-albero]"));
t("C2-21 «Vedi nell'albero →» porta alla Sequenza con il nodo scelto",ev("S.staz")===4&&!!q('.pp-albero [aria-selected="true"]'));
const inv4={
  "Sequenza: filtro tutto · solo da sistemare":qa("[data-seq-filtro]").length>=2,
  "Sequenza: zoom ⤢ e Apri/Chiudi tutto":(click('[data-seq-menu="zoom"]'),!!q("[data-pp-tutti=apri]")&&!!q("[data-pp-tutti=chiudi]")&&!!q('[data-pp-zoom="0"]')),
  "Sequenza: + stadio, + fase di unione prima, ⋯ (Rinomina, Sposta, Togli)":(click('[data-seq-menu="zoom"]'),!!q(".seq16-pannello [data-seq-menu^=stadio]")&&!!q(".seq16-pannello [data-seq-menu^=prima]")&&!!q(".seq16-pannello [data-seq-menu^=fase]")),
  "Sequenza: Entrano e dividi":!!q(".seq16-pannello .ent [data-seq-dividi]"),
  "Sequenza: riferimento":!!q(".seq16-pannello [data-seq-rif]"),
  "Sequenza: in maschera con Richiedi":!!q('.seq16-pannello [data-pa-spunta$="|maschera|sì"]'),
  "Sequenza: conferma con la firma":!!q('.seq-barra [data-conferma-seq]')&&JSON.parse(q('.seq-barra [data-conferma-seq]').getAttribute("data-pa-campi")).firma,
  "Sequenza: Proposte d'albero (R-31 codice vuoto, R-38 dal catalogo)":(ev(`S.seqScelto={tipo:"prodotto",cid:${JSON.stringify(cidDi(".000"))},fid:null,padre:null};S.seqPropAperte=true;render()`),q("#r31-codice").value===""&&!/\d/.test(q("#r31-codice").placeholder||"")&&q("#r31-btn").disabled&&!!q('[data-cat-apri="componenti|r38"]')),
  "Sequenza: stadi letti P U F T D C e viaggi":q(".seq16-pannello").textContent.includes("Stadi letti")&&q(".seq16-pannello").textContent.includes("Viaggi"),
};
for(const [k,v] of Object.entries(inv4)) t("C2-21 "+k,v);
const cod=q("#r31-codice");cod.value="F+800.514263.008-A";cod.dispatchEvent(new w.Event("input",{bubbles:true}));q("#r31-cid").value=cidDi(".008");click("#r31-btn");
t("C2-21 R-31: «proposta registrata: l'albero non cambia»",paAv().testo.includes("l'albero non cambia")&&fotoA()===A0);
const UNIONE_T=["sald_mag","sald_robot","sald_tig","prigionieri","montaggio","piantaggio","inserti"];
/* ================= REV · le revisioni del 12/10 (Lino, Pietro, frontend) ================= */
apriRFQ("482-00");click('.staz[data-staz="4"]');
/* A2 / P-08: la sequenza confermata è del prodotto; una scrittura riapre solo i prodotti che contengono l'assieme */
ev(`a2Di(threadAperto()).seq["altro-prodotto"]={stato:"confermata",firma:"x"}`);
tasto("Enter",{ctrlKey:true});
const rifSVI=ev("prodottoScelto(threadAperto()).rif");
t("REV A2: la conferma scrive lo stato del prodotto, non della RFQ",S2(`x.seq[${JSON.stringify(rifSVI)}].stato`)==="confermata"&&S2(`x.seq["altro-prodotto"].stato`)==="confermata");
gesto(`/fase/${ciclo(".000").fasi.find(f=>f.p==="sald_mag").id}`,{nome:"Saldatura telaio",fonte:"L"});
t("REV A2: una modifica riapre solo il prodotto che contiene l'assieme",S2(`x.seq[${JSON.stringify(rifSVI)}].stato`)==="bozza"&&S2(`x.seq["altro-prodotto"].stato`)==="confermata");
/* M-2, M-3: una fermata Tab nell'albero all'apertura (come motore_a.mjs) */
ev("S.seqScelto=null;render()");
t("REV M-2: albero del processo, un nodo nel giro del Tab senza nodi scelti",qa('.pp-albero [role="treeitem"][tabindex="0"]').length===1);
t("REV M-2: le fasi sul ramo sono nodi dell'albero",qa('.pp-albero .pp-ramo [role="treeitem"][data-pp-tipo="stadio"]').length>=1);
t("REV M-2: niente controlli nel giro del Tab dentro i nodi (sigle e chiavi inglesi aria-hidden)",qa('.pp-albero [role="treeitem"]').every(n=>![...n.querySelectorAll("button,a[href],input,select,[tabindex='0']")].length&&[...n.querySelectorAll("[data-vai],[data-pp-apri]")].every(x=>x.getAttribute("aria-hidden")==="true")));
t("REV M-6: linee tratteggiate finché la sequenza non è confermata",!q(".pp-albero--conf"));
/* A-2 / M4: il ramo si legge dal basso */
t("REV A-2: sul ramo la fase più vicina al rombo è la prima dopo l'unione, con la freccia ↑",(()=>{const r=q(".pp-li--u > .pp-ramo");if(!r)return false;const f=[...r.querySelectorAll(".pp-ramo-f")].map(x=>+x.textContent.match(/f\.(\d+)/)[1]);return !!r.querySelector(".pp-ramo-su")&&f.every((v,i)=>!i||v<f[i-1]);})());
/* M5: parole sui nodi, codice nel title; legenda nel «?» */
t("REV M5: sui nodi la parola, il codice della regola nel title",qa(".pp-mot").every(x=>!/^(SEQ|PREP|CIC|COME)-\d/.test(x.textContent.trim())));
t("REV M5: la legenda di sigle e stadi nel «?»",ev("AIUTO_STAZ[4].testi").some(x=>x.startsWith("Legenda delle sigle"))&&ev("AIUTO_STAZ[4].testi").some(x=>x.startsWith("Legenda degli stadi")));
/* P-12: «da proteggere» è un gesto del server, non una lista riscritta dal JS */
t("REV P-12: la rotta /fase/{fid}/proteggi c'è e la lista non si scrive più dal JS",ev("ROTTE_B.some(x=>x[0]==='faseProteggi')")&&!/proteggi:\s*\(on \?/.test(html));
/* A1 di Lino: «Ripristina la proposta» con una maschera già chiesta all'attrezzista */
click('.staz[data-staz="3"]');click(byText(".cl-riga",".000"));
const fSa=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id;
const fPu=gesto(`/componente/${cidDi(".000")}/ciclo/fase`,{p:"sald_mag",prima:fSa}).fid;
gesto(`/fase/${fPu}/componente`,{cid:cidDi(".002"),entra:"1"});gesto(`/fase/${fPu}/componente`,{cid:cidDi(".008"),entra:"1"});
gesto(`/fase/${fPu}`,{maschera:"sì",fonte:"L"});
const fFo=gesto(`/componente/${cidDi(".000")}/ciclo/fase`,{p:"foratura",dopo:fPu,seq:"1"}).fid;
const keyPu=JSON.parse(ev(`JSON.stringify(Object.entries(cicloB(threadAperto(),${JSON.stringify(cidDi(".000"))}).prep).find(([k,v])=>v.k==="maschera"&&v.nata===${JSON.stringify(fPu)})[0])`));
const rqPu=gesto(`/rfq/${ev("threadAperto().id")}/domanda`,{tipo:"officina",cid:cidDi(".000"),fid:fPu,prep:keyPu});
gesto(`/mockup/richiesta/${rqPu.rid}/sync`,{});
click('.staz[data-staz="4"]');click('[data-seq-ripristina="chiedi"]');
t("REV A1: la domanda elenca che cosa si toglie (la pre-unione e la foratura che ci lavorava) e che cosa resta aperto",q(".seq-chiedi").textContent.includes("Saldatura MIG/MAG")&&q(".seq-chiedi").textContent.includes("lavorava sulla pre-unione")&&q(".seq-chiedi").textContent.includes(rqPu.rid)&&q(".seq-chiedi").textContent.includes("Resta aperta"));
click('[data-seq-ripristina="si"]');
t("REV A1: tolte la pre-unione e la foratura che ci lavorava (mai una foratura prima della saldatura)",!ciclo(".000").fasi.some(f=>f.id===fPu||f.id===fFo));
t("REV A1: la maschera con la richiesta aperta resta, PREP-04 (avviso) e non conta «senza costo»",ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==="PREP-04"&&x.liv==="AV")`)&&ev(`preparazioniRFQ(threadAperto()).some(x=>x.key===${JSON.stringify(keyPu)}&&x.orfana)`)&&!ev(`attrezzaturaPronta(threadAperto(),prodottoScelto(threadAperto())).elenco.filter(x=>!x.orfana&&x.stato!=="esiste già"&&!x.costo).some(x=>x.key===${JSON.stringify(keyPu)})`));
t("REV P-05: il ripristino è una rotta sola per prodotto, con la firma",ev("REGISTRO[0].url").endsWith("/sequenza/ripristina")&&/^\/prodotto\//.test(ev("REGISTRO[0].url"))&&!!ev("REGISTRO[0].campi.firma"));
/* P-06: «Conferma» del ciclo con la firma di quello che si è letto */
click('.staz[data-staz="3"]');click(byText(".cl-riga",".008"));
const bc=q("[data-conferma-ciclo]");
t("REV P-06: il primario del Ciclo porta la firma",!!bc&&!!JSON.parse(bc.getAttribute("data-pa-campi")).firma);
const vecchiaC=ev(`SERVER_B.posta(threadAperto(),prodottoScelto(threadAperto()),"/componente/${cidDi(".008")}/ciclo/conferma",campiDa({firma:"seq-vecchia",accettaTutto:"1"}))`);
t("REV P-06: con una firma vecchia il server dice «rileggi» e niente cambia",vecchiaC.esito==="no"&&vecchiaC.testo.includes("rileggi")&&ciclo(".008").stato!=="confermato");
/* M6: «Annulla» nell'avviso dopo un clic su «Entrano» */
click('.staz[data-staz="4"]');
const fS6=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id;
ev(`S.seqScelto={tipo:"unione",cid:${JSON.stringify(cidDi(".000"))},fid:${JSON.stringify(fS6)},padre:null};render()`);
click(chip(fS6,".001"),".001 esce");
t("REV M6: l'avviso ha «Annulla» (la scrittura inversa sulla stessa rotta)",!!q('#pa-avviso [data-pa-annulla]')&&q('#pa-avviso [data-pa-annulla]').getAttribute("data-pa-gesto")===`/fase/${fS6}/componente`);
click(q('#pa-avviso [data-pa-annulla]'));
t("REV M6: Annulla rimette .001 dov'era",ciclo(".000").fasi.find(f=>f.id===fS6).entrano.some(e=>e.cid===cidDi(".001")));
/* M-4: il fuoco resta sull'elemento dopo il ridisegno */
const ch1=chip(fS6,".001");ch1.focus();click(ch1);
t("REV M-4: dopo il clic il fuoco resta sul chip (non torna al BODY)",d.activeElement&&d.activeElement!==d.body&&d.activeElement.getAttribute("data-pa-gesto")===`/fase/${fS6}/componente`);
Z("revisioni");

/* RL-02 (prova Cervi): un prodotto senza STEP. La zona B legge i confermati senza STEP (contratto §1.4, E4) e la quantità
   dalla parentela decisa; un componente aggiunto con R-38 (proposta) diventa una riga con il suo componente */
{ const r=JSON.parse(ev(`(()=>{const T=nuovoThread("t-rl02",{cliente:"Prova",oggetto:"RL-02",file:{p:["X-100.pdf","pdf"]},prodotti:[{k:"x1",cod:"X-100",descr:"Assieme di prova",pdf:"p"}]});const p=T.prodotti[0];
    const prima=righeB(T,p).map(x=>[x.cod,!!x.cid]);
    const g=SERVER_B.posta(T,p,"/distinta/"+T.id+"/componente",new URLSearchParams({codice:"X-101",qta:"2",descrizione:"Piastra"}));
    const senza=p.confermati_senza_step.map(n=>[n.nodo.decisione.codice,n.parentela.map(x=>[x.decisa,x.quantita])]);
    return JSON.stringify({prima,esito:g.esito,senza,righe:righeB(T,p).map(x=>[x.cod,!!x.cid,x.qtaProd,x.radice]),rie:riepilogoAlbero(T).blocchi.length});})()`));
  t("RL-02: senza STEP la radice è una riga con il suo componente (E4)",JSON.stringify(r.prima)==='[["X-100",true]]');
  t("RL-02: R-38 aggiunge X-101 ×2 nei confermati senza STEP, parentela decisa",r.esito==="ok"&&JSON.stringify(r.senza)==='[["X-100",[]],["X-101",[[true,2]]]]');
  t("RL-02: righeB legge i confermati senza STEP, con la quantità decisa",JSON.stringify(r.righe)==='[["X-100",true,1,true],["X-101",true,2,false]]');
  t("RL-03: il riepilogo non blocca un prodotto senza STEP",r.rie===0);
  ev(`delete THREADS["t-rl02"]`); }

Z("fine");

console.log(`\n${ok} superati · ${ko} falliti · errori JS: ${errori.length}`);errori.slice(0,5).forEach(e=>console.log("  JS: "+e));
process.exit(ko||errori.length?1:0);
