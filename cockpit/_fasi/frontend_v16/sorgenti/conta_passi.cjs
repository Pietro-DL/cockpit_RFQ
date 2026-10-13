/* conta_passi.cjs · v16 rev. (revisioni del 12/10): Documenti e Distinta sono una pagina sola (nessun clic di vista); le
   richieste ai fornitori sono le righe di Pietro: «Prepara la bozza» una per fornitore, niente «Manda» (la mail parte da
   Outlook, il sync la segna «inviata»: simulato, non di Lino); le risposte si registrano dall'elenco in alto nel cassetto;
   le domande interne sono bozze (niente «Manda»); la presa visione delle condizioni è un gesto solo.
   Prima: v16 (SPEC_v16 §9.5): la ① si apre sui Documenti; tratti nuovi «Preparazioni (v16)» (la maschera
   di …000: spunta, Richiedi, Robosald, Manda, Registra), «Sequenza a mano (VFR)» (pre-unione in maschera) e «FitLab»
   (tre livelli con un sottoassieme vero). Fallisce se il percorso semplice supera 64, con la maschera 69, il caso 1 13,
   il caso 2 10, o un tratto supera il suo bersaglio.
   Prima: SPEC_v15 §8.2 e §9.3. Percorre il caso VFR COME LINO, dalla RFQ aperta alla consegna a Franco,
   e conta i GESTI: clic e tasti che decidono o spostano. Tab e digitazione non contano; i numeri che Lino calcola a mano
   (qui: metri, fori e filetti della .009) sono dati, non gesti. I gesti di Fabio (le richieste di materia prima) e le
   risposte simulate non sono di Lino e non contano. Fallisce se il totale supera 71 o un tratto supera il suo bersaglio.
   Si esegue come test_clic.cjs: NODE_PATH=…/node_modules node --max-old-space-size=768 conta_passi.cjs */
const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup-v16.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document,q=s=>d.querySelector(s),qa=s=>[...d.querySelectorAll(s)],ev=s=>w.eval(s);
const byText=(sel,txt)=>qa(sel).find(e=>e.textContent.includes(txt));
let tratto="", gesti={}; const conta=()=>{gesti[tratto]=(gesti[tratto]||0)+1;};
const G=(el,nome)=>{ if(typeof el==="string") el=q(el); if(!el){ console.log("✗ manca: "+nome); process.exitCode=1; return; } conta(); el.click(); };
const T=(k,o={},el)=>{ conta(); (el||d.body).dispatchEvent(new w.KeyboardEvent("keydown",Object.assign({key:k,bubbles:true,cancelable:true},o))); };
const scrivi=(el,v)=>{ el.value=v; el.dispatchEvent(new w.Event("change",{bubbles:true})); };   /* digitazione: non conta */
const fuori=(fn)=>{ const t=tratto; tratto="(non di Lino)"; fn(); tratto=t; };
const ruolo=r=>fuori(()=>{ const a=q('[data-menu="app"]'); if(a) a.click(); q('[data-menu="ruolo"]').click(); q(`[data-ruolo="${r}"]`).click(); });   /* v16: «Vedi come» nel menu ≡ (RF-01) */

/* la RFQ è aperta (dall'elenco): da qui si conta */
ev(`apriRFQ("q-sv1"); render();`);
tratto="Struttura e documenti";
/* v16 rev. (P-01): un solo workspace: lo STEP e l'albero sono nella stessa pagina dei Documenti, senza un clic di vista */
G('[data-ma-azione="step"]',"Autorizza lo STEP…"); const cb=q('.ma-finestra input[name=autorizzo]'); conta(); cb.checked=true; cb.dispatchEvent(new w.Event("change",{bubbles:true}));
G('[data-ma-azione="step-invia"]',"Autorizza");
G('[data-ma-azione="albero"]',"riepilogo"); G('[data-ma-azione="albero-invia"]',"Conferma l'albero");
let n=0; while(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]') && n++<12){ G(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]'),"Conferma il documento"); G("[data-ma-si]","Sì"); }
tratto="Smistamento";
T("g"); gesti[tratto]--; T("2");                                   /* G 2 = un gesto */
for(let i=0;i<8;i++) T("Enter");
tratto="Richieste esterne";
G(".b-piede .ma-btn--pri","Apri le richieste da preparare");
const carta=k=>q(`#cassetto .rq[data-rq="${k}"]`), rigaF=(k,n)=>carta(k)?[...carta(k).querySelectorAll(".rq-forn")].find(x=>x.textContent.includes(n)):null;
const prepara=(k,n,mio)=>{ const vai=()=>{ if(!carta(k).querySelector("details[open]")) (mio?G:(x=>x.click()))(carta(k).querySelector("summary"),"apri la carta"); (mio?G:(x=>x.click()))([...rigaF(k,n).querySelectorAll("button")].find(b=>b.textContent.includes("Prepara la bozza")),"Prepara la bozza "+n); }; if(mio) vai(); else fuori(vai); };
const rqF=(id,lav)=>ev(`(a2Di(threadAperto()).richieste.find(q=>q.forn&&q.forn.id===${JSON.stringify(id)}${lav?`&&q.lavorazione===${JSON.stringify(lav)}`:""})||{}).id`);
const sim=(id,url)=>fuori(()=>ev(`eseguiB("/mockup/richiesta/${id}/${url}",{})`));
prepara("terzista|travi_est","RTC",true); prepara("lamiera","Lamitaglio",true); prepara("lamiera","Fornitore MP 4",true);
prepara("mp_prezzo","Fornitore MP 2",false); prepara("mp_mai","Fornitore MP 1",false);   /* le richieste di materia prima le prepara Fabio */
for(const id of [rqF("rtc"),rqF("lamitaglio"),rqF("mp4","laser_est"),rqF("mp2"),rqF("mp1")]){ sim(id,"sync"); sim(id,"risposta"); }   /* il sync e l'Inbox, non Lino */
tratto="Registrare le risposte";
for(let i=0;i<4;i++){ const b=q('#cassetto .rq-gruppo--reg [data-registra-veloce]'); if(b) G(b,"Registra"); }
tratto="Ciclo";
G('[data-cassetto=""]',"chiudi il cassetto");
T("g"); gesti[tratto]--; T("3");
const cid=s=>ev(`righeB(threadAperto(),prodottoScelto(threadAperto())).find(r=>r.cod.endsWith("${s}")).cid`), ciclo=s=>JSON.parse(ev(`JSON.stringify(cicloB(threadAperto(),"${cid(s)}"))`));
G(`[data-fase-apri="${ciclo(".000").fasi[2].id}"]`,"apri il controllo"); G(byText(".fase--aperta .seg button","ISO 13920 BE"),"ISO 13920 BE (REQ-01)");
/* v15: il tempo di saldatura non è più proposto: Lino lo scrive (un gesto per entrare nel campo, la digitazione non conta) */
const fSald=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id;   /* v16: prima la saldatura del telaio, poi i dadi saldati */
G(`[data-fase-apri="${fSald}"]`,"apri la saldatura"); conta(); scrivi(q(`[data-pa-campo="${fSald}|tempo"]`),"20 h");
/* ★ v16 · la maschera di …000: la spunta, «Richiedi: Fornitore», Robosald, Manda; la risposta (simulata) non è di Lino; Registra */
tratto="Preparazioni (v16)";
G(q(`.fase--aperta [data-pa-spunta="${fSald}|maschera|sì"]`),"in maschera");   /* un clic vero sulla spunta */
G(byText(`[data-fase="${fSald}"] .prep-mini button`,"Fornitore"),"Richiedi: Fornitore");
prepara("attrezzatura","Robosald",true);
const rm=rqF("robosald"); sim(rm,"sync"); sim(rm,"risposta");
G(q('#cassetto .rq-gruppo--reg [data-registra-veloce]'),"Registra la maschera");
const maschera=JSON.parse(ev("JSON.stringify(preparazioniRFQ(threadAperto()))")).find(x=>x.k==="maschera");
tratto="Ciclo";
for(const s of [".000",".001",".002",".006",".008"]) T("Enter",{ctrlKey:true});
const f9=ciclo(".009").fasi; scrivi(q(`[data-pa-campo="${f9[0].id}|m"]`),"1,1"); scrivi(q(`[data-pa-campo="${f9[0].id}|fori"]`),"3");
G(`[data-fase-apri="${f9[1].id}"]`,"apri la filettatura"); scrivi(q(`[data-pa-campo="${f9[1].id}|n"]`),"3");
T("Enter",{ctrlKey:true});
const confermati=["000","001","002","006","008","009"].every(s=>ciclo("."+s).stato==="confermato");
tratto="Sequenza";
T("g"); gesti[tratto]--; T("4"); T("Enter",{ctrlKey:true});
tratto="Esito e consegna";
T("g"); gesti[tratto]--; T("5");
G("[data-esito-anteprima]","Accetta le proposte OK…"); G(".blocco-b [data-pa-gesto]","Accetta le OK");
G("[data-presa-visione]","presa visione della penale e delle verifiche");
G(byText(".req-l div","Accettazione tacita").querySelector('[data-pa-gesto$="/domanda"]'),"Avvisa il commerciale");
G(byText(".firma-b","Area tecnica").querySelector("[data-pa-gesto]"),"firma");
const l=q("[data-lotto]"); conta(); l.checked=true; l.dispatchEvent(new w.Event("change",{bubbles:true}));
const pa=q("[data-presa-atto]"); if(pa){ conta(); pa.checked=true; pa.dispatchEvent(new w.Event("change",{bubbles:true})); }   /* v15: zona A non pronta → presa d'atto esplicita */
G("[data-consegna]","Consegna a Franco");
const consegnato=ev("a2Di(threadAperto()).consegne.length")===1;

/* ★ caso complesso 1 · VFR scritto a mano: le gambe (.002 + .008) saldate prima, in maschera, forate, poi il telaio */
tratto="Sequenza a mano (VFR)";
T("g"); gesti[tratto]--; T("4");
const fP=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id;
G(`.pp-unione[data-pp-fid="${fP}"]`,"il rombo della saldatura");
G(`.seq16-pannello [data-seq-menu^="prima|${fP}"]`,"+ fase di unione prima"); G(byText(".seq16-pannello .seq-menu button","Saldatura MIG/MAG"),"Saldatura MAG");
const fPre=ev("S.seqScelto.fid"), ch=(fid,suf)=>[...d.querySelectorAll(`.seq16-pannello [data-pa-gesto="/fase/${fid}/componente"]`)].find(b=>b.textContent.includes(suf));
G(ch(fPre,".002"),".002 entra"); G(ch(fPre,".008"),".008 entra");
{ const s=q(`.seq16-pannello [data-seq-rif="${fPre}"]`); conta(); s.value=cid(".008"); s.dispatchEvent(new w.Event("change",{bubbles:true})); }
G(q(`.seq16-pannello [data-pa-spunta="${fPre}|maschera|sì"]`),"in maschera (pre-unione)");
G(byText(".seq16-pannello .prep-mini button","Interno"),"Richiedi: Interno");
G(`.seq16-pannello [data-seq-menu^="stadio|${fPre}"]`,"+ stadio"); G(byText(".seq16-pannello .seq-menu button","Foratura e filettatura"),"Foratura");
T("Enter",{ctrlKey:true});
const caso1 = ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="confermata" && ciclo(".000").fasi[0].id===fPre && ciclo(".000").fasi[1].p==="foratura";

/* ★ caso complesso 2 · FitLab: tre livelli, il sottoassieme 9N008518AA, il perno in due posti */
fuori(()=>{ ev(`apriRFQ(RFQ.find(x=>x.num==="RDO 2026/318").id); render();`); q('.staz[data-staz="2"]').click(); for(let i=0;i<4;i++) d.body.dispatchEvent(new w.KeyboardEvent("keydown",{key:"Enter",bubbles:true,cancelable:true})); });
tratto="FitLab";
T("g"); gesti[tratto]--; T("4");
G('[data-seq-filtro="sistemare"]',"solo da sistemare");
const sub=cid("9N008518AA"), fSub=ciclo("9N008518AA").fasi.find(f=>f.p==="sald_mag").id;
G(`.pp-unione[data-pp-fid="${fSub}"]`,"il rombo del sottoassieme");
G(`.seq16-pannello [data-seq-menu^="stadio|${fSub}"]`,"+ stadio"); G(byText(".seq16-pannello .seq-menu button","Piantaggio a pressa"),"Piantaggio");
const fPi=ev("S.seqScelto.fid"); G(ch(fPi,"9N008519AB"),"il perno entra nel piantaggio");
const fV=ciclo("9N008518AA").fasi.find(f=>f.p==="verniciatura_polvere").id;
G(`[data-pp-stadio="${sub}|${fV}"]`,"il viaggio della verniciatura"); G(q(".seq16-pannello .cb-terzi button"),"un verniciatore");
G(byText(".seq16-pannello .seq-prot button","9N008519AB"),"da proteggere: il perno");
T("Enter",{ctrlKey:true});
const caso2 = ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="confermata";

const BERS={"Struttura e documenti":18,"Smistamento":9,"Richieste esterne":6,"Registrare le risposte":4,"Ciclo":13,"Sequenza":2,"Esito e consegna":12};
const V15={"Struttura e documenti":"18","Smistamento":"9","Richieste esterne":"6","Registrare le risposte":"4","Ciclo":"13","Sequenza":"2","Esito e consegna":"12"};
let tot=0, ok=true; console.log("\nTratto                       v15            v16 (gesti)   bersaglio");
for(const [k,b] of Object.entries(BERS)){ const v=gesti[k]||0; tot+=v; if(v>b) ok=false; console.log(`${k.padEnd(28)} ${V15[k].padEnd(14)} ${String(v).padEnd(13)} ≤ ${b}${v>b?"  ✗":""}`); }
console.log(`${"Totale senza preparazioni".padEnd(28)} ${"64".padEnd(14)} ${String(tot).padEnd(13)} ≤ 64${tot>64?"  ✗":""}`);
const prep=gesti["Preparazioni (v16)"]||0, c1=gesti["Sequenza a mano (VFR)"]||0, c2=gesti["FitLab"]||0;
console.log(`${"★ Preparazioni (v16)".padEnd(28)} ${"—".padEnd(14)} ${String(prep).padEnd(13)} ≤ 5${prep>5?"  ✗":""}`);
console.log(`${"Totale v16".padEnd(28)} ${"—".padEnd(14)} ${String(tot+prep).padEnd(13)} ≤ 69${tot+prep>69?"  ✗":""}`);
console.log(`${"★ caso 1 · Sequenza a mano".padEnd(28)} ${"—".padEnd(14)} ${String(c1).padEnd(13)} ≤ 13${c1>13?"  ✗":""}`);
console.log(`${"★ caso 2 · FitLab".padEnd(28)} ${"—".padEnd(14)} ${String(c2).padEnd(13)} ≤ 10${c2>10?"  ✗":""}`);
console.log(`\ncicli confermati: ${confermati} · maschera registrata: ${!!(maschera&&maschera.costo&&maschera.fonte==="RISP")} · consegnato: ${consegnato} · caso 1: ${caso1} · caso 2: ${caso2} · errori JS: ${errori.length}`);
if(!ok || tot>64 || prep>5 || tot+prep>69 || c1>13 || c2>10 || !confermati || !consegnato || !caso1 || !caso2 || !(maschera&&maschera.fonte==="RISP") || errori.length){ console.log("conta_passi: FALLITO"); process.exit(1); }
console.log("conta_passi: OK");
