/* test_tirante.cjs · il caso permanente Cervi 73416 «tirante» (Telesol, 03/2025), da zero fino all'offerta e alla revisione.
   Il copione è quello di v16/prova_cervi/DOSSIER.md §6, con le correzioni di RILIEVI.md. jsdom sul file ricomposto.
   Si clicca come Lino, Fabio e Franco; nessuna iniezione di stato (la mail è un dato permanente del mockup).
   Valori attesi (valori di esempio): lotto 5 → costo 58,16 €, prezzo al 35 % 89,47 → 89,50 €; lotto 20 (MRT) → costo 41,48 €. */
const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup-v16.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));vc.on("error",e=>errori.push(String(e)));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document;let ok=0,ko=0,gesti=0;
const t=(nome,cond)=>{if(cond){ok++;}else{ko++;console.log("✗ "+nome);}};
const q=s=>d.querySelector(s), qa=s=>[...d.querySelectorAll(s)];
const ev=s=>w.eval(s);
const T_=el=>(el?el.textContent:"").replace(/\s+/g," ").trim();
const click=(s,nome)=>{const el=typeof s==="string"?q(s):s;if(!el){ko++;console.log("✗ manca da cliccare: "+(nome||s));return false;}gesti++;el.click();return true;};
const byText=(sel,txt)=>qa(sel).find(e=>T_(e).includes(txt));
const scrivi=(el,v,nome)=>{ if(typeof el==="string") el=q(el); if(!el){ko++;console.log("✗ manca il campo: "+(nome||""));return;} gesti++; if(el.type==="checkbox") el.checked=v; else el.value=v; el.dispatchEvent(new w.Event("input",{bubbles:true})); el.dispatchEvent(new w.Event("change",{bubbles:true})); };
const invia=(form,nome)=>{ if(!form){ko++;console.log("✗ manca il modulo: "+nome);return;} gesti++; form.dispatchEvent(new w.Event("submit",{bubbles:true,cancelable:true})); };
const avB=()=>ev("PA.avviso")||{esito:null,testo:""};
const avA=()=>ev("PONTE.avviso")||{esito:null,testo:""};
const ruolo=r=>ev(`S.ruolo=${JSON.stringify(r)};render()`);
const TH="threadAperto()", P="prodottoScelto(threadAperto())";
const righe=()=>JSON.parse(ev(`JSON.stringify(righeB(${TH},${P}).map(r=>({cod:r.cod,cid:r.cid,descr:r.descr,radice:r.radice,figli:r.figli.length,qta:r.qtaProd})))`));
const cidDi=cod=>(righe().find(r=>r.cod===cod)||{}).cid;
const ciclo=cod=>JSON.parse(ev(`JSON.stringify(cicloB(${TH},${JSON.stringify(cidDi(cod))})||null)`));
const S2=e=>ev(`(x=>${e})(a2Di(${TH}))`);
const vicino=(a,b,tol=0.011)=>Math.abs(a-b)<=tol;
/* fotografie statiche per lo screenshot di controllo: solo con FOTO=1 (Firefox snap legge solo sotto ~/snap/firefox/common) */
const foto=nome=>{ if(!process.env.FOTO) return; const dir=require("path").join(__dirname,"foto"); fs.mkdirSync(dir,{recursive:true});
  const c=d.documentElement.cloneNode(true); c.querySelectorAll("script").forEach(x=>x.remove()); fs.writeFileSync(`${dir}/${nome}.html`,"<!doctype html>"+c.outerHTML); };
const euro=s=>{const m=String(s).replace(/\./g,"").replace(",",".").match(/-?\d+(\.\d+)?/);return m?+m[0]:NaN;};

/* ================= Stazione 0 · Inbox: la mail è fra i nascosti (cliente non censito) ================= */
t("S0 la mail Cervi è un dato permanente fra i nascosti, senza cliente",ev("NASCOSTI.some(m=>m.id==='n5'&&!m.cli&&m.porta&&m.porta.files.length===4)"));
click('[data-tab="set"]',"Clienti e mittenti");click('[data-setsez="nascosti"]',"Messaggi nascosti");
const rigaN5=()=>qa("tbody tr").find(r=>T_(r).includes("73416"));
t("RL-06 la riga senza cliente ha «Censisci come cliente», non «Nuovo cliente»",!!rigaN5()&&!!rigaN5().querySelector("[data-censisci]")&&!rigaN5().querySelector("[data-nuovo]"));
click(rigaN5().querySelector("[data-censisci]"),"Censisci come cliente");
t("RL-06 il modulo arriva con il dominio della mail già scritto",q("#fc-dom")&&q("#fc-dom").value==="telesol.example"&&T_(q(".form")).includes("acquisti@telesol.example"));
t("RL-29 il settore non ha un valore già messo",q("#fc-sett")&&q("#fc-sett").value==="");
scrivi("#fc-nome","Telesol Sollevatori","ragione sociale");
click('[data-salvacli="1"]',"Crea il cliente");
const CID=ev("CLIENTI[CLIENTI.length-1].id");
foto("00_nascosti_censisci");
t("RL-06 dopo il salvataggio la mail dello stesso dominio ha il cliente e mostra «Autorizza»",ev(`NASCOSTI.find(m=>m.id==='n5').cli`)===CID&&!!rigaN5()&&!!rigaN5().querySelector('[data-autorizza="n5"]'));
click('[data-autorizza="n5"]',"Autorizza");
t("S0 la mail è in «Da smistare» con i 4 PDF",ev("SMISTARE.some(s=>s.id==='s-n5'&&s.files.length===4)"));
click('[data-tab="inbox"]',"Inbox");click('[data-sez="smistare"]',"Da smistare");click('[data-orf="s-n5"]',"la mail di Telesol");
click('[data-nuovareq="s-n5"]',"Crea la richiesta");
scrivi('[data-cod="0"]',true,"spunta 73416");scrivi('[data-nome="0"]',"Tirante RIF.40071526","denominazione");scrivi('[data-qta="0"]',"20 pz/anno","quantità");
click('[data-creareq="1"]',"Crea la richiesta (conferma)");
const RID=ev("RICHIESTE[RICHIESTE.length-1].id");
t("S0 richiesta di Telesol con un prodotto 73416",ev(`rq("${RID}").cli`)===CID&&ev(`PROD[rq("${RID}").prodotti[0]].cod`)==="73416");
t("RL-28 senza scadenza: «da chiedere», non «vicina»",ev(`rq("${RID}").sla`)==="nessuna");

/* ================= Stazione 1-2 · RFQ, file, struttura senza STEP (R-38) ================= */
click(`[data-crearfq="${RID}"]`,"Crea l'RFQ");
t("S1 lotti proposti 5 e 20 dai «20 pz/anno»",ev(`JSON.stringify(lottiDi(${TH}).lotti)`)==="[5,20]");
click(q('[data-ma-gesto$="/fascicolo/prepara"]'),"Azioni del thread › Prepara i file");
t("RL-05 i 3 PDF dei figli sono in «Da smistare» (nessun_candidato), niente si perde",ev(`${TH}.esito.da_smistare.filter(f=>f.motivo==='nessun_candidato').map(f=>f.nome_file).sort().join()`)==="640339.pdf,640340.pdf,640341.pdf");
t("RL-03 senza BOM di lavoro i confermati senza STEP hanno la radice (E4)",ev(`${TH}.prodotti[0].confermati_senza_step.length`)===1&&ev(`${TH}.prodotti[0].confermati_senza_step[0].nodo.rif`).startsWith("componente:")&&ev(`${TH}.prodotti[0].confermati_senza_step[0].nodo.senza_file`)===true);
t("RL-17 avviso «3D assente» in Documenti (zona B, non un blocco)",!!q("[data-avviso-3d]"));
// il 2D del prodotto
const bDoc=qa('#ma-documenti [data-ma-gesto$="/fascicolo/conferma"]').find(b=>/73416\.pdf/.test(b.getAttribute("aria-label")||""));
click(bDoc,"Conferma il documento 73416.pdf");click("[data-ma-si]","Sì, procedi");
t("S2 73416.pdf confermato",avA().esito==="ok"&&/73416\.pdf/.test(avA().testo));
// R-38: tre componenti sotto 73416, scritti da Lino
t("RL-01 la striscia R-38 (zona B) c'è, sotto gli strumenti spenti",!!q("[data-r38] form[data-pa-gesto]")&&qa(".ma-strumenti button").every(b=>b.disabled));
t("RL-01 nessun codice precompilato nella riga R-38",q('[data-r38] input[name="codice"]').value===""&&q('[data-r38] input[name="qta"]').value==="");
const FIGLI=[["640340","0","Tubolare per tirante"],["640339","0","Boccola a saldare per tirante"],["640341","0","Forcella per tirante"]];
for(const [cod,rev,descr] of FIGLI){
  const f=q("[data-r38] form");
  scrivi(f.querySelector('[name="codice"]'),cod,"codice "+cod);scrivi(f.querySelector('[name="rev"]'),rev,"rev");scrivi(f.querySelector('[name="descrizione"]'),descr,"denominazione");scrivi(f.querySelector('[name="qta"]'),"1","q.tà");
  invia(f,"Aggiungi "+cod);
  t(`R-38 ${cod} aggiunto sotto 73416 ×1`,avB().esito==="ok"&&avB().testo.includes(cod));
}
t("RL-01 il codice si scrive: un doppione si rifiuta",(()=>{ const f=q("[data-r38] form"); f.querySelector('[name="codice"]').value="640340"; f.querySelector('[name="qta"]').value="1"; invia(f,"doppione"); const r=avB(); f.querySelector('[name="codice"]').value=""; return r.esito==="no"&&r.testo.includes("già un componente"); })());
t("RL-03 i 3 componenti escono in confermati_senza_step con la parentela decisa ×1",ev(`(p=>p.confermati_senza_step.length===4&&p.confermati_senza_step.slice(1).every(n=>n.parentela.length===1&&n.parentela[0].decisa&&n.parentela[0].quantita===1&&n.parentela[0].padre===p.confermati_senza_step[0].nodo.rif))(${TH}.prodotti[0])`));
foto("01_r38_struttura");
t("RL-02 la zona B legge i confermati senza STEP: 4 righe, 3 figli della radice, tutte con il componente",(()=>{const r=righe();return r.length===4&&r.every(x=>x.cid)&&r.find(x=>x.radice).figli===3;})());
// i PDF dei figli: lo smistamento li propone, Lino li conferma uno per uno
click(q('[data-ma-gesto$="/smistamento/aggiorna"]'),"Rifai lo smistamento");
t("S2 «Rifai lo smistamento» propone i 3 PDF per i componenti con lo stesso codice",avA().esito==="ok"&&/3 destinazioni riscritte/.test(avA().testo)&&ev(`${TH}.esito.da_smistare.length`)===0);
for(const cod of ["640339","640340","640341"]){
  const b=qa('#ma-documenti [data-ma-gesto$="/fascicolo/conferma"]').find(x=>(x.getAttribute("aria-label")||"").includes(cod+".pdf"));
  click(b,"Conferma il documento "+cod);click("[data-ma-si]","Sì");
  t(`S2 ${cod}.pdf confermato per ${cod}`,avA().esito==="ok"&&avA().testo.includes("per "+cod));
}
t("S2 4 documenti confermati; la copia sul NAS va in coda e diventa «scritto» a una lettura (confermato ≠ copiato)",ev(`Object.values(${TH}.documenti).length`)===4&&ev(`Object.values(${TH}.documenti).every(d=>["in_coda","scritto"].includes(d.stato_nas))`)&&ev(`Object.values(${TH}.documenti).some(d=>d.stato_nas==="in_coda")`));
// Conferma l'albero
click('[data-ma-azione="albero"]',"Leggi il riepilogo e conferma l'albero");
t("RL-03 il riepilogo non ha il blocco «prima autorizza lo STEP»: è confermabile",!q(".ma-finestra .ma-blocchi")&&!q('[data-ma-azione="albero-invia"]').disabled);
click('[data-ma-azione="albero-invia"]',"Conferma l'albero");
t("R68 A, K-02 A: nomenclatura verificata, gerarchia «da verificare · fonte_non_confermata», BOM non verificata",ev(`(b=>b.nomenclatura.stato==="verificata"&&b.gerarchia.stato==="da_verificare"&&b.gerarchia.motivo==="fonte_non_confermata"&&b.verificata===false)(${TH}.prodotti[0].bom)`));
t("RL-05 la testata dice «perimetro aperto»",T_(q('.staz[data-staz="1"]')).includes("perimetro aperto"));
t("DB7-5 A: nessun bom/applica nel registro",ev("REGISTRO.every(x=>!/bom\\/applica/.test(x.url))"));

/* ================= Stazione 3 · Come ================= */
click('.staz[data-staz="2"]',"② Come");
const prop=JSON.parse(ev(`JSON.stringify(righeB(${TH},${P}).map(r=>[r.cod,modoComponente(${TH},${P},r)]))`));
const pm=c=>(prop.find(x=>x[0]===c)||[])[1]||{};
t("RL-12 «Come» propone: 640340 noi tubo a sega · 640339 fuori tornitura · 640341 fuori fresatura · 73416 noi saldato",pm("640340").modo==="noi"&&pm("640340").modello==="T3"&&pm("640339").modo==="fuori"&&pm("640339").modello==="T4"&&pm("640341").modo==="fuori"&&pm("640341").modello==="T4c"&&pm("73416").modello==="T2");
for(const cod of ["73416","640340","640339","640341"]){
  const b=qa(`tr[data-come-cid="${cidDi(cod)}"] button[data-pa-gesto$="/fornitura"]`).find(x=>x.classList.contains("seg--prop"));
  click(b,`${cod} → proposta`);
  t(`S3 ${cod}: come si fa deciso`,avB().esito==="ok");
}
t("RL-12 il tubo ha solo la sega (la foratura non è proposta)",JSON.stringify(ciclo("640340").fasi.map(f=>f.p))==='["sega"]');
t("RL-12 la forcella è «Fresatura da terzi»",ciclo("640341").fasi[0].p==="fresatura_est");

/* ================= Stazione 4 · Ciclo ================= */
click('.staz[data-staz="3"]',"③ Ciclo");
const apriRiga=cod=>{ click(qa(".cl-riga").find(x=>x.dataset.cicloCid===cidDi(cod)),"riga "+cod); };
const apriFase=fid=>{ if(ev("S.faseAperta")!==fid) click(`[data-fase-apri="${fid}"]`,"apri la fase"); };
const campo=(fid,k,v)=>scrivi(q(`[data-pa-campo="${fid}|${k}"]`),v,k);
const btnF=(fid,pred,nome)=>click(qa(`[data-pa-gesto="/fase/${fid}"]`).find(x=>pred(JSON.parse(x.getAttribute("data-pa-campi")||"{}"))),nome);
const conferma=cod=>{ click("[data-pa-gesto$='/ciclo/conferma']","Conferma il ciclo "+cod); t(`S4 ciclo ${cod} confermato`,ciclo(cod).stato==="confermato"||(console.log("   ",avB().testo),false)); };
// 640340 · tubo
apriRiga("640340");let F=ciclo("640340").fasi;apriFase(F[0].id);
scrivi(q(`input[data-pa-grezzo="${F[0].id}"]`),"355 tubo 42,4x4","grezzo");
t("RL-13 dal «355» si propone la qualità già vista per Telesol: S355J2+N",ciclo("640340").grezzo.par.q==="S355J2+N"&&avB().testo.includes("già vista per Telesol"));
t("RL-13 la norma del tubo resta «da verificare», non EN 10025-2",T_(q(`[data-fase="${F[0].id}"] .grz-esito`)).includes("norma del tubo da verificare")&&!T_(q(`[data-fase="${F[0].id}"] .grz-esito`)).includes("EN 10025-2"));
campo(F[0].id,"L","1371");campo(F[0].id,"tagli","1");campo(F[0].id,"tempo","20 pz/h");
conferma("640340");
// 640339 · boccola, fuori
apriRiga("640339");F=ciclo("640339").fasi;apriFase(F[0].id);
btnF(F[0].id,c=>c.terzi==="mrt","terzista MRT");campo(F[0].id,"lotti","5/20/50/100");btnF(F[0].id,c=>c.conto==="conto vendita","conto vendita");campo(F[0].id,"mat","S355J2+N");
conferma("640339");
// 640341 · forcella, fuori
apriRiga("640341");F=ciclo("640341").fasi;apriFase(F[0].id);
btnF(F[0].id,c=>c.terzi==="mrt","terzista MRT");btnF(F[0].id,c=>c.terzi==="mrt,tfr","terzista TFR");campo(F[0].id,"lotti","5/10/20");btnF(F[0].id,c=>c.conto==="conto vendita","conto vendita");campo(F[0].id,"mat","S355J2+N");
conferma("640341");
// 73416 · assieme saldato
apriRiga("73416");F=ciclo("73416").fasi;
t("S4 73416: saldatura, controllo, imballo; nella saldatura entrano i 3 figli",JSON.stringify(F.map(f=>f.p))==='["sald_mag","controllo","imballo"]'&&(F[0].entrano||[]).length===3);
apriFase(F[0].id);campo(F[0].id,"tempo","20 pz/h");
const fc=F.find(f=>f.p==="controllo");apriFase(fc.id);btnF(fc.id,c=>c.tipo==="dimensionale a calibro","controllo dimensionale");btnF(fc.id,c=>c.freq==="primo pezzo","primo pezzo");
const fi=F.find(f=>f.p==="imballo");apriFase(fi.id);btnF(fi.id,c=>c.tipo==="a vista","imballo a vista");
foto("02_ciclo_73416");
t("RL-18 il prodotto finito offre «nessun trattamento» come scelta esplicita",!!q("[data-tratt-nessuno]"));
click("[data-tratt-nessuno]","Trattamento: nessuno (consegna grezza)");
t("RL-18 trattamento «nessuno», fonte L",ciclo("73416").trattamento&&ciclo("73416").trattamento.v==="nessuno"&&ciclo("73416").trattamento.fonte==="L");
conferma("73416");

/* ================= Richieste ai terzisti (Fabio): la nota, gli allegati, i giri ================= */
ruolo("Fabio");
click('[data-cassetto="richieste"]',"cassetto ✉ Richieste");
const carta=k=>q(`#cassetto [data-rq="${k}"]`);
t("S3 due carte: tornitura (640339) e fresatura da terzi (640341)",!!carta("terzista|tornitura")&&!!carta("terzista|fresatura_est"));
const nota=k=>{ const ta=q(`#cassetto textarea[data-rq-nota="${k}"]`); return ta?ta.value:""; };
const apriCarta=k=>{ const dd=q(`#cassetto details[data-rq-carta="${k}"]`); if(dd && !dd.open){ gesti++; dd.querySelector("summary").click(); } };
apriCarta("terzista|tornitura");
t("RL-11 la nota dice lotti della fase, materiale S355J2+N e conto vendita",/640339.*lotti 5 \/ 20 \/ 50 \/ 100 pezzi, materiale S355J2\+N, conto vendita/.test(nota("terzista|tornitura")));
apriCarta("terzista|fresatura_est");
t("RL-11 forcella: lotti 5 / 10 / 20 della sua fase, non quelli della testata",/640341.*lotti 5 \/ 10 \/ 20 pezzi, materiale S355J2\+N, conto vendita/.test(nota("terzista|fresatura_est")));
const prepara=(k,nome)=>{ apriCarta(k); const r=qa(`#cassetto [data-rq="${k}"] .rq-forn`).find(x=>T_(x).includes(nome)); const b=r&&r.querySelector('[data-pa-gesto$="/richiesta"]'); click(b,`Prepara la bozza · ${nome} (${k})`); return avB(); };
t("S3 bozza MRT tornitura",prepara("terzista|tornitura","MRT").esito==="ok");
t("S3 bozza MRT fresatura",prepara("terzista|fresatura_est","MRT").esito==="ok");
t("S3 bozza TFR fresatura",prepara("terzista|fresatura_est","T.F.R.").esito==="ok");
const RQ=JSON.parse(S2("JSON.stringify(x.richieste.map(q=>({id:q.id,forn:q.forn&&q.forn.id,lav:q.lavorazione,allegati:q.allegati,note:q.note})))"));
const rqDi=(forn,lav)=>RQ.find(x=>x.forn===forn&&x.lav===lav)||{};
t("RL-11 gli allegati sono i PDF confermati dei codici",JSON.stringify(rqDi("mrt","tornitura").allegati)==='["640339.pdf"]'&&JSON.stringify(rqDi("tfr","fresatura_est").allegati)==='["640341.pdf"]');
t("RL-19 i fornitori delle richieste entrano nei «Terzisti» della fase (TFR già scelto da Lino, nessun doppione)",(()=>{const f=ciclo("640341").fasi[0];return String(f.par.terzi).split(",").sort().join()==="mrt,tfr";})());
const rigaRq=id=>q(`#cassetto [data-rq-riga="${id}"]`);
const sync=id=>{ const r=rigaRq(id); const s=r&&r.querySelector("details.piu summary"); if(s) click(s,"⋯"); const b=r&&[...r.querySelectorAll("[data-pa-gesto]")].find(x=>/\/sync$/.test(x.getAttribute("data-pa-gesto"))); click(b,"Simula: la mail parte da Outlook"); };
const registra=(id,righeL,extra)=>{ const r=rigaRq(id); const det=r&&r.querySelector("details.rq-reg"); if(det&&!det.open){gesti++;det.open=true;} const f=r&&r.querySelector("form[data-pa-gesto$='/risposta/righe']");
  if(!f){ko++;console.log("✗ manca il modulo «Registra la risposta» di "+id);return {};}
  for(const [k,v] of Object.entries(righeL)) scrivi(f.querySelector(`[name="l_${cidDi(k)}"]`),v,"prezzi "+k);
  for(const [k,v] of Object.entries(extra||{})) scrivi(f.querySelector(`[name="${k}"]`),v,k);
  invia(f,"Registra"); return avB(); };
const idMT=rqDi("mrt","tornitura").id, idMF=rqDi("mrt","fresatura_est").id, idTF=rqDi("tfr","fresatura_est").id;
for(const id of [idMT,idMF,idTF]) sync(id);
t("S3 le tre richieste sono «inviate»",S2(`x.richieste.filter(q=>q.stato==="inviata").length`)===3);
// giro 1 (boccola) e giro 2 (forcella): le risposte vere, scritte a righe
const DATA1=ev("B_OGGI");
let r=registra(idMT,{"640339":"20: 7,5 / 50: 6 / 100: 5,5"},{tempi:"15 gg lav.",allegato:"Offerta Rif. RDO 640339.doc"});
t("RL-04 MRT boccola: una quotazione con 3 scaglioni, fonte RISP",r.esito==="ok"&&ev(`QUOTAZIONI_B.filter(x=>x.tk===${TH}.key&&x.cod==="640339").length`)===1&&ev(`JSON.stringify(QUOTAZIONI_B.find(x=>x.tk===${TH}.key&&x.cod==="640339").lotti)`)==="[[20,7.5],[50,6],[100,5.5]]");
r=registra(idMF,{"640341":"10: 20"},{tempi:"15 gg lav."});
t("RL-04 MRT forcella lotto 10 a 20 €",r.esito==="ok");
r=registra(idTF,{"640341":"10: 14"},{materiale:"S235JR",tempi:"20 gg D.O.",allegato:"SCAN-0000000001.pdf (scansione)"});
t("RL-08 TFR dichiara S235JR: registrata con riserva, MAT-05 segnalato",r.esito==="ok"&&r.testo.includes("MAT-05")===false&&/S235JR/.test(r.testo)&&S2(`x.richieste.find(q=>q.id==="${idTF}").controlli.some(k=>k.cod==="MAT-05")`));
foto("03_cassetto_registra");
t("RL-08 la riga di TFR nel cassetto dice MAT-05",T_(rigaRq(idTF)).includes("MAT-05"));
t("RL-14 le quotazioni storiche 2025 hanno un giro per riga e il materiale di TFR",ev("QUOTAZIONI.filter(x=>x.storico&&/^64034[01]|^640339/.test(x.cod)).length")===5&&ev("QUOTAZIONI.find(x=>x.id==='qt10').materiale")==="S235JR");
ruolo("Lino");
click('[data-cassetto="richieste"]',"chiude il cassetto");
// il Ciclo legge le risposte: confronto con il materiale
click('.staz[data-staz="3"]',"③ Ciclo");scrivi(q("[data-ciclo-filtro-sel]"),"tutti","filtro «tutti»");apriRiga("640341");apriFase(ciclo("640341").fasi[0].id);
foto("04_ciclo_confronto");
t("RL-08 nel «Confronto dei terzisti» la colonna Materiale avvisa: S235JR ≠ S355J2+N",!!q(".cf-tab .cf-mat--no")&&T_(q(".cf-tab .cf-mat--no")).includes("S235JR"));
t("RL-14 nel Ciclo lo storico 2025 si dice «storico», non è un'offerta della RFQ",T_(q(".cf-tab")).includes("storico SO 6218"));

/* ================= Stazione 5 · Sequenza ================= */
click('.staz[data-staz="4"]',"④ Sequenza");
click("[data-pa-gesto$='/sequenza/conferma']","Conferma la sequenza");
t("S5 sequenza confermata, l'albero della struttura non cambia",ev(`seqDi(${TH},${P}).stato`)==="confermata");

/* ================= Stazione 6 · Esito e consegna (lotto 20) ================= */
click('.staz[data-staz="5"]',"⑤ Esito");
const esP=cod=>ev(`esitoProposto(${TH},${P},${JSON.stringify(cidDi(cod))}).v`);
t("RL-08 640341 OK con MRT (TFR escluso: solo un avviso MAT-05)",esP("640341")==="OK"&&ev(`controlliB(${TH},${P}).some(x=>x.cod==="MAT-05"&&x.liv==="AV"&&x.rcod==="640341")`));
t("RL-24 640340 DUBBIO per «mai acquistato» (default, da confermare)",esP("640340")==="DUBBIO");
t("RL-17 «Matematica (3D)» proposta «NO · solo 2D»",T_(q(".dd")).includes("NO · solo 2D"));
t("RL-23 il riepilogo conta le richieste aperte",/richieste \d/.test(T_(q(".b-piede-t"))));
const esito=(cod,v)=>{ if(!q(`[data-pa-gesto="/componente/${cidDi(cod)}/fattibilita"][data-v="${v}"]`)) click(`[data-esito-apri="${cidDi(cod)}"]`,"riga "+cod); click(q(`[data-pa-gesto="/componente/${cidDi(cod)}/fattibilita"][data-v="${v}"]`),`${cod} → ${v}`); };
for(const c of ["73416","640340","640339","640341"]) esito(c,"OK");
click(qa('[data-pa-gesto$="/firma"]').find(x=>x.getAttribute("data-pa-campi").includes('"positiva"')),"Firma area tecnica: positiva");
scrivi('input[data-lotto="20"]',true,"lotto 20");
scrivi('input[data-presa-atto]',true,"presa d'atto: zona A non pronta (K-02)");
click('[data-pa-gesto$="/consegna"]',"Consegna a Franco");
t("S6 consegna 1: lotto 20, con la presa d'atto della zona A",S2("x.consegne.length")===1&&S2("x.consegne[0].lotti.join()")==="20"&&S2("x.consegne[0].zonaA.presaAtto")===true);

/* ================= Stazione 7-8 · Costi e offerta rev 0 (Franco) ================= */
ruolo("Franco");
click('[data-esito-vista="costi"]',"Costi");
const cs=(k,v)=>scrivi(q(`[data-cs="${k}"]`),v,k);
cs("mdc|","35");cs(`kg|${cidDi("640340")}`,"5,2");
cs(`incl|${ciclo("73416").fasi.find(f=>f.p==="controllo").id}`,true);
const tot=i=>JSON.parse(ev(`JSON.stringify((f=>{const C0=costoDi(${TH},f),t=totaliCosto(${TH},f,C0,${i});return {L:t.L,tot:t.tot,prezzo:t.prezzo,mdc:t.mdcEff,calc:t.calc,manca:t.manca,trasp:t.trasp,righe:t.righe.map(r=>[r.r.cod,r.b,r.eur,r.driver])};})(fotoCorrente(${TH})))`));
let T20=tot(0);
t("S7 lotto 20: costo 41,48 € (tubo 6,06 · sega e saldatura 3,20 · forcella MRT 21,60 · boccola 8,10 · trasporti 2,52)",vicino(T20.tot,41.48)&&!T20.manca||(console.log("   ",JSON.stringify(T20)),false));
const drvF=T20.righe.find(x=>x[0]==="640341")[3];
t("RL-07 la forcella non prende TFR a 14 € (materiale discorde): MRT 20 € × 1,08",vicino(T20.righe.find(x=>x[0]==="640341")[2],21.60)&&/MRT/.test(drvF)&&/esclusa:.*T\.F\.R\.[^·]*S235JR/.test(drvF)&&(drvF.match(/T\.F\.R\./g)||[]).length===1);
t("RL-07 «lotto 20 non quotato: si usa il lotto 10»",drvF.includes("lotto 20 non quotato: si usa il lotto 10"));
t("RL-15 trasporti: 6 % del subtotale × 1,08",vicino(T20.trasp,38.95696*0.06*1.08)&&T_(q("table.costi")).includes("6 % del subtotale + 8 %"));
cs("prezzo|0","60");T20=tot(0);
t("S8 lotto 20 a 60,00 €: margine 30,9 %",vicino(T20.mdc*100,30.9,0.06));
foto("05_costi_lotto20");
t("RL-16 avviso: margine sotto l'obiettivo del 35 %",!!q("[data-avviso-margine]")&&!!q("table.costi [data-sotto-obiettivo]"));
click('[data-esito-vista="offerta"]',"Offerta");
t("RL-16 anche nelle righe dell'Offerta",!!q(".off-v [data-sotto-obiettivo]"));
click('[data-of="emetti"]',"Emetti l'offerta");
const O=()=>JSON.parse(ev(`JSON.stringify(offertaDi(${TH}))`));
t("S8 rev 0 emessa: una riga, 73416 · 20 pz · 60,00 €, con la sua data",O().emesse.length===1&&O().emesse[0].rev===0&&O().emesse[0].righe.length===1&&O().emesse[0].righe[0].qta===20&&O().emesse[0].righe[0].prezzo===60&&O().emesse[0].data===DATA1);
t("RL-29 prezzi SO con due decimali",T_(q(".so-carta .so-t")).includes("60,00")&&!T_(q(".so-carta .so-t")).includes("60,0000"));
click('[data-of="mail"]',"Prepara la mail al cliente");
t("RL-20 la mail è una bozza in Outlook, non «inviata»",O().stato==="bozza_mail");
click('[data-of="mailsync"]',"Simula: la mail parte da Outlook");
t("RL-20 «inviata» arriva dal sync",O().stato==="inviata");

/* ================= Giorni dopo: il lotto 5 (giro 3 MRT), consegna 2, revisione 1 ================= */
click('[data-pa-gesto="/mockup/tempo"]',"⧗ Passa una settimana (mockup)");
const DATA2=ev("B_OGGI");
t("RL-28 l'orologio del mockup va avanti",DATA2!==DATA1);
ruolo("Fabio");click('[data-cassetto="richieste"]',"cassetto ✉");
const giro=id=>{ const r=rigaRq(id); const s=r&&r.querySelector("details.piu summary"); if(s) click(s,"⋯"); click(r&&[...r.querySelectorAll("[data-pa-gesto]")].find(x=>/\/giro$/.test(x.getAttribute("data-pa-gesto"))),"Nuovo giro"); };
for(const id of [idMT,idMF]){ apriCarta(id===idMT?"terzista|tornitura":"terzista|fresatura_est"); giro(id); sync(id); }
r=registra(idMT,{"640339":"5: 15 / 20: 7,5 / 50: 6 / 100: 5,5"},{tempi:"15 gg lav."});
t("R-13 giro 3 MRT boccola registrato con la sua data; il giro 1 resta",r.esito==="ok"&&r.testo.includes("giro 2 del "+DATA2)&&S2(`x.richieste.find(q=>q.id==="${idMT}").giri.length`)===1&&ev(`QUOTAZIONI_B.filter(x=>x.tk===${TH}.key&&x.cod==="640339").map(x=>x.data).join()`)===DATA1+","+DATA2);
r=registra(idMF,{"640341":"5: 27"},{tempi:"15 gg lav."});
t("R-13 giro 3 MRT forcella lotto 5 a 27 €",r.esito==="ok");
ruolo("Lino");click('[data-cassetto="richieste"]',"chiude il cassetto");
click('[data-esito-vista="esito"]',"Esito");
scrivi('input[data-lotto="5"]',true,"lotto 5");scrivi('input[data-presa-atto]',true,"presa d'atto");
click('[data-pa-gesto$="/consegna"]',"Consegna a Franco (lotto 5)");
t("RL-09 consegna 2 con un'ora sua, diversa dalla prima",S2("x.consegne.length")===2&&S2("x.consegne[1].quando")!==S2("x.consegne[0].quando"));
ruolo("Franco");click('[data-esito-vista="costi"]',"Costi");
t("RL-09 i costi della consegna 2 sommano i lotti (5 e 20) e tengono la scheda di Franco",ev(`JSON.stringify(costoDi(${TH},fotoCorrente(${TH})).scen)`)==="[5,20]"&&ev(`costoDi(${TH},fotoCorrente(${TH})).mdc`)==="35");
const T5=tot(0);T20=tot(1);
foto("06_costi_lotti_5_20");
t("S7 lotto 5: costo 58,16 € (forcella 29,16 · boccola 16,20 · trasporti 3,54)",vicino(T5.tot,58.16)&&!T5.manca||(console.log("   ",JSON.stringify(T5)),false));
t("S7 lotto 5: prezzo al 35 % 89,47 → 89,50 €",vicino(T5.calc,89.47)&&T5.prezzo===89.5);
t("S7 lotto 5: TFR non quota il lotto 5 (sotto il lotto minimo) e il materiale è discorde",/esclusa:.*T\.F\.R\.[^·]*sotto il lotto minimo/.test(T5.righe.find(x=>x[0]==="640341")[3]));
t("RL-09 il lotto 20 resta a 60,00 € accanto al lotto 5",T20.L===20&&T20.prezzo===60);
click('[data-esito-vista="offerta"]',"Offerta");
click(q('[data-of="rev"]')||(()=>{ const s=q(".off-v details.piu--esito summary"); if(s) click(s,"Esito del cliente ▾"); return q('[data-of="rev"]'); })(),"Crea una revisione");
click('[data-of="emetti"]',"Emetti la revisione 1");
const o=O();
t("RL-10 rev 1 emessa con la SUA data; la rev 0 resta con la sua",o.emesse.length===2&&o.emesse[1].rev===1&&o.emesse[1].data===DATA2&&o.emesse[0].data===DATA1&&o.emesse[0].righe.length===1);
t("RL-10 rev 1: due righe dello stesso codice, 20 pz a 60,00 € e 5 pz a 89,50 €",o.emesse[1].righe.length===2&&o.emesse[1].righe.some(x=>x.qta===20&&x.prezzo===60)&&o.emesse[1].righe.some(x=>x.qta===5&&x.prezzo===89.5));
t("RL-10 «Annulla e sostituisce» la rev 0, da sola, con numero e data",(o.emesse[1].sostituisce||"").includes("Annulla e sostituisce la SO "+o.num+"/2026 del "+DATA1)&&T_(q(".so-carta")).includes("Annulla e sostituisce"));
t("RL-10 il documento porta «rev 1» e la data della revisione",T_(q(".so-carta .so-n")).includes("rev 1")&&T_(q(".so-carta [data-so-data]")).includes(DATA2));
click('[data-of-rev="0"]',"Apri la rev 0");
t("RL-10 la rev 0 si apre in sola lettura con la sua data e una riga",!!q(".so-carta--rev")&&T_(q(".so-carta--rev [data-so-data]")).includes(DATA1)&&qa(".so-carta--rev .so-t tbody tr").length===1);
foto("08_offerta_rev0_lettura");
click('[data-of-rev="0"]',"torna alla corrente");foto("07_offerta_rev1");

/* ================= invarianti ================= */
t("Z la fase registrata del thread non è cambiata (RV-05)",ev(`${TH}.testa.fase.nome`)==="RICEVUTA");
t("Z nessun data-pa-gesto nella lista A",(()=>{const R=ev("ROTTE.map(compilaRotta).map(r=>r.source)").map(x=>new RegExp(x));return qa("[data-pa-gesto]").every(e=>!R.some(x=>x.test(e.getAttribute("data-pa-gesto"))));})());
t("Z errori JS = 0",errori.length===0||(console.log("   JS: "+errori.slice(0,3).join(" | ")),false));
console.log(`\ntest_tirante: ${ok} superati · ${ko} falliti · gesti ${gesti} · errori JS: ${errori.length}`);
process.exit(ko?1:0);
