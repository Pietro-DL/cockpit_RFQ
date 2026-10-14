const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));vc.on("error",e=>errori.push(String(e)));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document;let ok=0,ko=0;
const t=(nome,cond)=>{if(cond){ok++;}else{ko++;console.log("✗ "+nome);}};
const q=s=>d.querySelector(s), qa=s=>[...d.querySelectorAll(s)];
const click=(s,nome)=>{const el=typeof s==="string"?q(s):s;if(!el){ko++;console.log("✗ manca da cliccare: "+(nome||s));return false;}el.click();return true;};
const byText=(sel,txt)=>qa(sel).find(e=>e.textContent.includes(txt));
const nMsg=()=>qa(".p-conv .msg").length;

// 1. elementi tolti
t("niente Sistema grafico",!d.body.textContent.includes("Sistema grafico"));
t("niente Inbox e filtri nel sorgente",!html.includes("Inbox e filtri"));
t("niente commutatore modalità",!html.includes("data-mode")&&!html.includes("Conversazione unica"));
t("tre schede",qa("#tabs button").length===3);

// 2. VignaBot: una richiesta, 4 messaggi, filtro 4/3
click('[data-cli="vignabot"]');
t("VignaBot una richiesta",qa(".p-list [data-req]").length===1);
t("VignaBot 4 messaggi",nMsg()===4);
t("b3 di Théo nella richiesta con riga di provenienza",!!q(".p-conv .catena")&&d.querySelector(".p-conv").textContent.includes("902235-telaio_b3"));
click('.pfilter [data-pf="vb1"]');t("filtro 902235_B = 4",nMsg()===4);
click('.pfilter [data-pf="vb2"]');t("filtro 902256_B = 3",nMsg()===3);
click('.pfilter [data-pf=""]');t("tutti = 4",nMsg()===4);

// 3. crea RFQ VignaBot, poi riapri senza doppioni
const nRFQ0=w.eval("RFQ.length");
t("blocco RFQ grande presente",!!q(".rfqblock.nuova .btn.xl[data-crearfq]"));
click('.rfqblock [data-crearfq="r-vb1"]');
t("pagina RFQ aperta",!!q(".rfqhead")&&q(".rfqhead").textContent.includes("creata"));
t("primo passo = Documenti e NAS",q('.step[aria-selected="true"]').textContent.includes("Documenti e NAS"));
const idVB=w.eval("S.rfq");
click('.rfqhead [data-req="r-vb1"]');
t("blocco ora dice Apri l’RFQ",!!q('.rfqblock [data-apririfq]'));
click('.rfqblock [data-apririfq]');t("riapre la stessa RFQ",w.eval("S.rfq")===idVB);
click('[data-tab="inbox"]');click('[data-cli="vignabot"]');click('.p-ctx [data-apririfq]');
t("nessun doppione",w.eval("RFQ.length")===nRFQ0+1);
t("VignaBot due prodotti nelle linguette",qa(".ptab").length===2);
t("slot PDF 902235 da verificare",!!q(".slot.verifica"));
click('[data-cerca]');t("cerca di nuovo VignaBot: niente di nuovo",q(".esito.no")&&q(".esito.no").textContent.includes("Nessun nuovo"));

// 4. TDL crea, ACME/FitLab/Potaflex aprono
click('[data-tab="inbox"]');click('[data-cli="tdl"]');click('.rfqblock [data-crearfq]');
t("TDL RFQ creata",w.eval("rq('r-sd1').rfq")!==null&&!!q(".rfqhead"));
t("TDL slot sul portale",qa(".slot.vuoto").length>=1&&d.body.textContent.includes("indicato sul portale"));
click('[data-cerca]');t("TDL: trovato PDF sul portale",q(".esito.ok")&&q(".esito.ok").textContent.includes("S9.1248"));
for(const [cli,num] of [["acme","990020338"],["fitlab","RDO 2026/318"],["potaflex","P-2026-0131"]]){
  click('[data-tab="inbox"]');click(`[data-cli="${cli}"]`);click('.p-list [data-req]');click('.rfqblock [data-apririfq]',cli);
  t(`apre RFQ ${num}`,q(".cart h1")&&q(".cart h1").textContent===num);
}
// seconda richiesta ACME: crea
click('[data-tab="inbox"]');click('[data-cli="acme"]');
t("ACME due richieste",qa(".p-list [data-req]").length===2);
click('[data-req="r-ar2"]');click('.rfqblock [data-crearfq]');
t("ACME 990020412 creata col numero del cliente",q(".cart h1").textContent==="990020412");
click('[data-cerca]');t("ACME 2: trovato lo STEP di 9990713C1",d.body.textContent.includes("STEP di 9990713C1"));

// 5. Da smistare → Carrelli Nord → crea richiesta → crea RFQ
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s2"]');
click('[data-nuovareq="s2"]');t("modulo nuova richiesta",!!q('[data-creareq]'));
click('[data-creareq]');
t("Carrelli Nord: conversazione con blocco RFQ",w.eval("S.cli")==="carrellinord"&&!!q('.rfqblock [data-crearfq]'));
click('.rfqblock [data-crearfq]');t("Carrelli Nord RFQ creata",q(".cart h1").textContent.startsWith("P-2026-0"));
t("Carrelli Nord: PDF riconosciuto, STEP manca",d.body.textContent.includes("CN-4471_rev_a.pdf")&&!!q(".slot.vuoto"));

// 6. aggancio
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s1"]');
const nAr=w.eval("mail(msgDi('r-ar1')).length");click('[data-aggancia="s1"]');
t("aggancio: messaggio nella richiesta",w.eval("mail(msgDi('r-ar1')).length")===nAr+1&&w.eval("SMISTARE.length")===1);

// 7. gestione RFQ
click('[data-tab="gest"]');
const cnt=k=>+q(`[data-stato="${k}"] .cnt`).textContent;
t("avviate 7",cnt("avviata")===7);t("accettate 2",cnt("accettata")===2);t("produzione 1",cnt("produzione")===1);t("terminate 2",cnt("terminata")===2);
const g=q("#gq");g.value="falci";g.dispatchEvent(new w.Event("input",{bubbles:true}));
t("ricerca Falci Beta: 0 avviate, 1 produzione",cnt("avviata")===0&&cnt("produzione")===1);
g.value="";g.dispatchEvent(new w.Event("input",{bubbles:true}));
click('[data-stato="produzione"]');click('#gbody tr.click');t("riga storica si apre",q(".cart h1").textContent==="920-097");
t("storica completa: NAS pronto",!q('[data-nas]').disabled);

// 8. ACME 990020338: documenti
click('[data-tab="gest"]');click('[data-stato="avviata"]');click(byText("#gbody tr.click","990020338"));
t("NAS disabilitato all’inizio",q('[data-nas]').disabled);
click('.slot [data-insp="at1|step"]');t("Ispeziona apre il pannello",!!q(".drawer"));
click('.drawer [data-usa="0"]');t("Cambia: ora IGS",d.body.textContent.includes("9990708A_1.IGS"));
click('[data-cerca]');t("trovato DXF annidato",d.body.textContent.includes("archivio annidato"));
click('[data-confall]');
t("R103 C: la minuteria solo proposta non esenta, NAS bloccato",q('[data-nas]').disabled&&q('.nasbox').textContent.includes("990679X1"));
t("sette assi del prodotto",qa(".asse").length===7&&q('.asse[data-asse="stato"]').textContent.includes("non pronto"));
t("nomenclatura da verificare finché la Distinta non è confermata",q('.asse[data-asse="nom"]').textContent.includes("da verificare"));
t("file senza destinazione distinti da quelli in analisi",d.body.textContent.includes("analisi in corso")&&d.body.textContent.includes("nessuna destinazione"));
qa("[data-mincf]").forEach(()=>click("[data-mincf]"));
t("minuteria confermata: 2D non più richiesto",w.eval("DOCS.at1.parti.filter(p=>p.cat==='minuteria').every(p=>p.catConf)")&&!q('.nasbox').textContent.includes("990679X1"));
t("NAS abilitato dopo la conferma della minuteria",!q('[data-nas]').disabled);
t("documenti: i pezzi proposti sono segnati",d.body.textContent.includes("da confermare nella Distinta"));
click('[data-nas]');t("copiato sul NAS",d.body.textContent.includes("Copiati sul NAS"));
click('.slot [data-rim="at1|pdf"]');t("rimosso: slot vuoto e NAS bloccato",!!q(".slot.vuoto")&&q('[data-nas]').disabled);
const lib=qa("[data-libass]").length;click(byText(".pick","9990708A_1.pdf").querySelector("[data-libass]"),"associa il PDF tolto");t("associa: pannello",!!q(".drawer")&&!!q('[data-assoc="at1|pdf"]'));
click('[data-assoc="at1|pdf"]');t("associato di nuovo",!q(".slot.vuoto")&&qa("[data-libass]").length===lib-1);
// 8a. Conferma fascicolo: il gesto cumulativo con il riepilogo (R67, R108), su VignaBot
click('[data-tab="gest"]');click('[data-stato="avviata"]');click(byText("#gbody tr.click","902235"));
t("conferma fascicolo: pulsante con le decisioni",!!q('[data-cf="apri"]')&&/decisioni/.test(q('[data-cf="apri"]').textContent));
click('[data-cf="apri"]');
const grp=qa(".drawer .cf-g .lab").map(x=>x.textContent).join("|");
t("riepilogo per gruppi di decisione",grp.includes("Fonte strutturale")&&grp.includes("Nomenclatura")&&grp.includes("Relazioni e quantità")&&grp.includes("Associazioni dei file"));
t("l’alternativa irrisolta (PDF b3 da verificare) resta fuori, non scelta in silenzio",!!q(".drawer .cf-g.fuori")&&q(".drawer .cf-g.fuori").textContent.includes("902235-telaio_b3.pdf"));
const primaCF=w.eval("bomDi('vb1').nodes.filter(n=>n.proposto).length");
const unaFile=q('.drawer input[data-cfsel^="file:"]');unaFile.checked=false;unaFile.dispatchEvent(new w.Event("change",{bubbles:true}));
click('.drawer [data-cf="conferma"]');
t("un solo gesto, esito unico",d.body.textContent.includes("Fascicolo confermato")&&d.body.textContent.includes("Non è congelato"));
t("nodi confermati e distinta salvata",primaCF===2&&w.eval("bomDi('vb1').nodes.filter(n=>n.proposto).length")===0&&w.eval("bomVerificata('vb1')"));
t("la fonte STEP è autorizzata",w.eval("DOCS.vb1.step.stato")==="ok");
t("la decisione tolta dal riepilogo resta da confermare",w.eval("chiavi('vb1').some(k=>{const s=getSlot('vb1',k);return s&&!s.portale&&s.stato==='prop'})"));
click('[data-tab="gest"]');click('[data-stato="avviata"]');click(byText("#gbody tr.click","990020338"));

// 8b. Distinta di ACME 990020338: tutte le funzioni della pagina del giro 4
click('[data-step="1"]');
const B=()=>w.eval("bomDi('at1')");
t("albero: 5 caselle",qa(".tela .nodo").length===5);
t("4 caselle proposte, tratteggiate",qa(".tela .nodo.proposto").length===4);
t("analisi: come ha deciso e che cosa ha letto",!!q(".analisi-box")&&q(".analisi-box").textContent.includes("Come ha deciso")&&q(".analisi-box").textContent.includes("Che cosa ha letto"));
t("elenco particolari letto dal disegno",!!byText(".analisi-box summary","elenco particolari"));
t("passo Distinta: 4 proposte",q('.step[data-step="1"]').textContent.includes("4 proposte"));
t("miniatura del disegno nella casella del prodotto",!!q('.nodo [data-dvis="9990708A_1.pdf"] svg'));
t("dadi: nessun disegno, non serve",qa(".miniatura.vuota").length===3&&d.body.textContent.includes("per un particolare commerciale non serve"));
t("quantità sugli archi",qa(".qta-arco").map(x=>x.textContent).sort().join()==="×1,×1,×2,×3");
t("celle documenti: il 2D del dado non compare",!byText('.nodo[data-dsel="at1-1"] .cella',"2D"));
click('[data-dsel="at1-0"]');
t("scheda del pezzo: da dove viene",q(".dett").textContent.includes("Da dove viene")&&q(".dett").textContent.includes("9990707A_1.pdf"));
t("scheda: quantità totale",q(".dett").textContent.includes("quantità totale"));
// nuovo assieme con codice interno automatico
click('[data-dnuovo="sottoass"]');t("codice interno proposto",q("#dn-cod").value==="9990708A1-A01");
q("#dn-nome").value="Gruppo piastra";click('[data-dx="aggiungi"]');
t("assieme aggiunto",B().nodes.length===6&&qa(".tela .nodo").length===6);
t("barra: modifiche da salvare",!!q(".salva-barra")&&q(".salva-barra").textContent.includes("non ancora salvat"));
const idA=w.eval("DIST.sel");
// sposta il supporto sotto il nuovo assieme (tendina «Sotto»)
click('[data-dsel="at1-0"]');const sp=q("#dd-padre");sp.value=idA;sp.dispatchEvent(new w.Event("change",{bubbles:true}));
t("supporto ora sotto l’assieme",w.eval(`nodo(bomDi('at1'),'at1-0').padre`)===idA);
t("quantità totale per prodotto",w.eval(`qtaTot(bomDi('at1'),nodo(bomDi('at1'),'at1-0'))`)===1);
// trascinare un dado sopra un particolare: rifiutato
const dado=q('.nodo[data-dsel="at1-1"]'),sup=q('.nodo[data-dsel="at1-0"]');
dado.dispatchEvent(new w.Event("dragstart",{bubbles:true}));sup.dispatchEvent(new w.Event("drop",{bubbles:true,cancelable:true}));
t("sotto un particolare non si mette niente",q(".esito.no")&&q(".esito.no").textContent.includes("sotto non ci va niente"));
// trascinare un dado sopra l'assieme: accettato
q('.nodo[data-dsel="at1-1"]').dispatchEvent(new w.Event("dragstart",{bubbles:true}));q(`.nodo[data-dsel="${idA}"]`).dispatchEvent(new w.Event("drop",{bubbles:true,cancelable:true}));
t("trascinato sotto l’assieme",w.eval(`nodo(bomDi('at1'),'at1-1').padre`)===idA);
// tipo: un assieme con pezzi sotto non diventa particolare
click(`[data-dsel="${idA}"]`);const tp=q("#dd-tipo");tp.value="sciolto";tp.dispatchEvent(new w.Event("change",{bubbles:true}));
t("assieme con figli non diventa particolare",w.eval(`nodo(bomDi('at1'),'${idA}').tipo`)==="sottoass"&&q(".esito.no").textContent.includes("prima spostali"));
// codice doppio rifiutato
const cd=q("#dd-cod");cd.value="9990707A1";cd.dispatchEvent(new w.Event("change",{bubbles:true}));
t("codice doppio rifiutato",q(".esito.no").textContent.includes("c’è già"));
// elimina con conferma: va via anche ciò che ha sotto
click('[data-dx="eliminasel"]');t("chiede conferma, con i pezzi sotto",!!q(".avviso-conf")&&q(".avviso-conf").textContent.includes("pezzi che ha sotto"));
click('[data-dx="eliminaok"]');t("eliminato con i suoi 2 pezzi",B().nodes.length===3);
click('[data-dx="annullatutto"]');t("annulla le modifiche: torna com’era",B().nodes.length===5&&qa(".tela .nodo.proposto").length===4);
// il prodotto non si elimina
click('[data-dsel="r"]');click('[data-dx="eliminasel"]');t("il prodotto non si elimina",q(".esito.no").textContent.includes("il prodotto non si elimina"));
// tastiera: Invio sceglie la casella
q('.nodo[data-dsel="at1-2"]').dispatchEvent(new w.KeyboardEvent("keydown",{key:"Enter",bubbles:true}));
t("Invio sceglie la casella",w.eval("DIST.sel")==="at1-2");
click('[data-dx="conferma"]');t("conferma una casella",qa(".tela .nodo.proposto").length===3);
// scarta la proposta, poi annulla
click('[data-dx="scarta"]');t("scarta chiede conferma",!!q(".analisi-box .avviso-conf"));
click('[data-dx="scartaok"]');t("scartate le 3 proposte",B().nodes.length===2);
click('[data-dx="annullatutto"]');
// accetta e salva: BOM verificata
click('[data-dx="accetta"]');t("accetta: nessuna proposta",qa(".tela .nodo.proposto").length===0);
click('[data-dx="salva"]');
t("salvata e verificata",d.body.textContent.includes("Distinta salvata e verificata")&&w.eval("bomVerificata('at1')"));
t("passo Distinta: verificata",q('.step[data-step="1"]').textContent.includes("verificata"));
t("assi nomenclatura e gerarchia verificati",q('.asse[data-asse="nom"]').classList.contains("ok")&&!!q('.asse[data-asse="ger"]'));
t("documenti seguono la distinta: pezzi confermati",w.eval("DOCS.at1.parti.every(p=>p.conf)"));
// visore con note
click('.nodo [data-dvis="9990708A_1.pdf"]');t("visore aperto",!!q(".visore")&&q(".lato-note").textContent.includes("Nota di esempio"));
click('[data-dvx="zpiu"]');t("zoom 125%",q(".gruppo .z").textContent==="125%");
click('[data-dvx="arma"]');click('[data-dvx="carta"]');t("nota: si apre il riquadro",!!q("#nota-testo"));
q("#nota-testo").value="Verificare la piega a 90°";click('[data-dvx="salvanota"]');
t("nota salvata",qa(".lato-note .nota").length===2&&w.eval("NOTE['9990708A_1.pdf'].length")===2);
click('[data-dvx="togli:3"]');t("nota tolta",qa(".lato-note .nota").length===1);
const sel=q("#vis-file");sel.value="9990707A_1.pdf";sel.dispatchEvent(new w.Event("change",{bubbles:true}));
t("cambia disegno dalla tendina",q(".visore-barra .titolo b").textContent==="9990707A_1.pdf");
d.dispatchEvent(new w.KeyboardEvent("keydown",{key:"Escape",bubbles:true}));t("Esc chiude il visore",!q(".visore"));
// verso la fattibilità
const fat=()=>q(".sezione-titolo")&&q(".sezione-titolo").parentElement.textContent;
t("fattibilità: da comprare i dadi",fat().includes("Da comprare")&&fat().includes("990679X1"));
t("fattibilità: lavorazioni dal disegno",fat().includes("SPEC-ZINC-013")&&fat().includes("UNI EN ISO 13920-BE"));
t("fattibilità: chat del terzista",!!q('[data-tz="f3:0"]'));
t("congela la V1 disabilitato",byText(".btn","Congela la V1").disabled);
t("note di progetto",!!q(".nota-prog"));
// eliminare un pezzo con file: i file restano, senza pezzo
click('[data-dsel="at1-1"]');click('[data-dx="elimina"]');click('[data-dx="eliminaok"]');click('[data-dx="salva"]');
t("file del pezzo tolto fra i non associati",w.eval("DOCS.at1.liberi.some(l=>l[0]==='990679X_1.STP')"));
// 8c. ciclo di produzione di ACME 990020338
t("struttura: chip del ciclo in ogni casella",qa(".tela .cella.ciclo").length===qa(".tela .nodo").length);
click('.tela .cella.ciclo[data-wfgo="at1-0"]');t("dal chip si apre il ciclo del pezzo",w.eval("S.dview")==="ciclo"&&w.eval("WF.cur")==="at1-0");
click('[data-wfgo="r"]');
t("ciclo: si parte dal prodotto",q(".wfciclo").textContent.includes("9990708A1")&&!!q(".wfnav"));
t("striscia dei componenti in ordine",qa(".mini .mchip").length===4&&q(".mini .mchip").textContent.includes("9990708A1"));
t("disegno del prodotto nel ciclo",!!q(".wfpdf .carta svg"));
t("note condivise con l’albero",q(".wfnote").textContent.includes("Nota di esempio"));
t("5 fasi proposte",qa(".fasi .fase").length===5);
t("ciclo a tutto schermo",!!q(".rfq.largo"));
const gruppi=[...q("#wf-nuova").querySelectorAll("optgroup")].map(g=>g.label);
t("menu per categorie",["Taglio","Piegatura e formatura","Lavorazioni meccaniche","Saldatura","Assemblaggio","Trattamenti superficiali","Trattamenti termici","Finitura e controllo"].every(x=>gruppi.includes(x)));
t("saldature sotto Saldatura",qa('#wf-nuova optgroup[label="Saldatura"] option').map(o=>o.value).join()==="puntatura,sald_robot,sald_mag,sald_tig");
t("niente acquisto nel menu",!q('#wf-nuova option[value="acquisto"]'));
t("la maschera non è una lavorazione",!qa("#wf-nuova option").some(o=>/maschera/i.test(o.textContent)));
t("categoria scritta sulla fase",q(".fase .fase-cat").textContent==="Saldatura");
t("zincatura: dallo storico, in uso",byText(".sug","9990708A1")&&byText(".sug","9990708A1").textContent.includes("in uso"));
t("terzista qualificato per ACME",[...q('.fase.est select[data-wff$="|terz"]').options].some(o=>o.textContent.includes("qualificato per ACME")));
click('[data-wf="conferma"]');t("ciclo del prodotto confermato",w.eval("statoCiclo('at1',nodo(bomDi('at1'),'r'))")==="confermato"&&d.body.textContent.includes("Prossimo"));
click('[data-wf="next"]');t("successivo: 9990707A1",w.eval("WF.cur")==="at1-0");
t("materiale S235JR",q('[data-wfm="m"]').value==="S235JR");
click('[data-wf="conferma"]');
d.dispatchEvent(new w.KeyboardEvent("keydown",{key:"ArrowRight",bubbles:true}));t("freccia destra: commerciale",w.eval("WF.cur")==="at1-2");
t("commerciale: nessun ciclo, lo compra Fabio",!!q(".wfacq")&&q(".wfacq").textContent.includes("Fabio")&&!q(".fasi"));
t("commerciale: entra nella fase 10 del padre",q(".wfinfo").textContent.includes("fase 10"));
t("commerciali non contano fra i cicli",w.eval("contaCicli('at1')[1]")===2);
// nota scritta nel ciclo, legata a una fase, visibile nell'albero
click('[data-wfgo="r"]');click('[data-wv="arma"]');click('[data-wv="carta"]');t("nota nel ciclo: riquadro",!!q("#wf-nota"));
q("#wf-nota").value="Robot: attenzione all’accessibilità del dado M5";const fsel=q("#wf-nota-fase");fsel.value=fsel.options[2].value;click('[data-wv="salvanota"]');
t("nota salvata con la fase",q(".wfnote").textContent.includes("fase 20 Saldatura"));
click('[data-dview="struttura"]');click('.nodo [data-dvis="9990708A_1.pdf"]');
t("la stessa nota nel visore dell’albero",q(".lato-note").textContent.includes("accessibilità del dado M5")&&q(".lato-note").textContent.includes("fase 20"));
d.dispatchEvent(new w.KeyboardEvent("keydown",{key:"Escape",bubbles:true}));
// controlli: un figlio fuori da ogni fase blocca la conferma
click('.nodo[data-dsel="r"] .cella.ciclo');
const cb=q('[data-wff$="|figlio:at1-0"]');cb.checked=false;cb.dispatchEvent(new w.Event("change",{bubbles:true}));
t("figlio in nessuna fase: punto rosso",q(".wfciclo .ctrl li.bad")&&q(".wfciclo .ctrl").textContent.includes("9990707A1 non entra"));
t("modificato: da riconfermare, conferma bloccata",w.eval("statoCiclo('at1',nodo(bomDi('at1'),'r'))")==="proposto"&&q('[data-wf="conferma"]').disabled);
const cb2=q('[data-wff$="|figlio:at1-0"]');cb2.checked=true;cb2.dispatchEvent(new w.Event("change",{bubbles:true}));
t("rimesso: conferma di nuovo possibile",!q('[data-wf="conferma"]').disabled);
// riordino e fasi nuove
const prima=w.eval("CICLI['at1|r'].fasi.map(f=>f.p).join()");click(qa('.fase [data-wf^="su:"]')[2]);
t("fase spostata prima",w.eval("CICLI['at1|r'].fasi.map(f=>f.p).join()")!==prima&&w.eval("CICLI['at1|r'].fasi[1].p")==="controllo");
click(qa('.fase [data-wf^="giu:"]')[1]);
const nv=q("#wf-nuova");nv.value="verniciatura_polvere";click('[data-wf="aggiungi"]');t("fase esterna aggiunta",qa(".fasi .fase").length===6);
const ts=qa('.fase.est select[data-wff$="|terz"]').pop();ts.value="f1";ts.dispatchEvent(new w.Event("change",{bubbles:true}));
t("terzista non qualificato per ACME: punto rosso",q(".wfciclo .ctrl").textContent.includes("non è qualificato per ACME"));
click(qa('.fase [data-wf^="via:"]').pop());t("fase tolta",qa(".fasi .fase").length===5);
// richiesta al terzista dalla fase di zincatura
const nChat=w.eval("terz('f3').chat.length");
click(q('.fase.est [data-wf^="prepara:"]'));t("pacchetto per il terzista con il PDF del pezzo",!!q(".invio")&&qa(".invio .allegato").length===1&&q(".invio .allegato").textContent.includes("9990708A_1.pdf"));
click('[data-wf^="catdis:"]');t("archivio dei disegni per cliente",!!q(".drawer")&&q(".drawer").textContent.includes("Stai allegando")&&qa(".drawer details.arch").length>=4&&q(".drawer details.arch summary").textContent.includes("ACME"));
const aq=q("#wf-catq");aq.value="9993449";aq.dispatchEvent(new w.Event("input",{bubbles:true}));
t("ricerca nell’archivio",qa("#wfcatlist .arch-f").length===1);
click('[data-wf="allega:9993449A_1.pdf"]');t("allegato dall’archivio",qa(".invio .allegato").length===2);
click('.drawer footer [data-wf="catchiudi"]');
click('[data-wf^="carica:"]');const up=q("#wf-upl");Object.defineProperty(up,"files",{value:[new w.File(["%PDF-1.4"],"zincatura_istruzioni.pdf",{type:"application/pdf"})]});
up.dispatchEvent(new w.Event("change",{bubbles:true}));
t("PDF caricato e allegato",qa(".invio .allegato").length===3&&d.body.textContent.includes("archivio di ACME"));
t("il PDF caricato entra nell’archivio",w.eval("archivioPdf().some(o=>o.f==='zincatura_istruzioni.pdf'&&o.cli==='acme')"));
click(qa('.invio [data-wf^="stacca:"]')[1]);t("allegato tolto",qa(".invio .allegato").length===2);
click('[data-wf^="invia:"]');
t("bozza nella chat del terzista",w.eval("terz('f3').chat.length")===nChat+1&&d.body.textContent.includes("Bozza pronta"));
t("la bozza porta i due PDF",w.eval("terz('f3').chat.at(-1).msg.at(-1).files.length")===2);
click(byText(".fase.est .btn","Apri la chat"));
t("la chat del terzista si apre nell’Inbox",w.eval("S.tab")==="inbox"&&w.eval("S.sez")==="terzisti"&&d.body.textContent.includes("bozza preparata dalla Distinta"));
// catalogo
click('[data-tab="gest"]');click(byText("#gbody tr.click","990020338"));click('[data-step="1"]');click('[data-dview="ciclo"]');
click('.wfadd [data-wf="catalogo"]');t("catalogo aperto sull’archivio dei disegni",!!q(".drawer")&&q(".drawer").textContent.includes("Archivio dei disegni")&&!q('.drawer [data-wf^="allega:"]'));
click('[data-wf="cattab:storico"]');
const cq=q("#wf-catq");cq.value="tornitura";cq.dispatchEvent(new w.Event("input",{bubbles:true}));t("ricerca nello storico: 2 torniture",qa("#wfcatlist .alt").length===2);
click('[data-wf="cattab:terzisti"]');t("terzisti nel catalogo",qa(".drawer .alt").length===w.eval("TERZISTI.length"));
click('.drawer footer [data-wf="catchiudi"]');t("catalogo chiuso",!q(".drawer"));
// FitLab: tre livelli, una proposta
click('[data-tab="gest"]');click('[data-stato="avviata"]');click(byText("#gbody tr.click","RDO 2026/318"));click('[data-step="1"]');
t("FitLab: piastra sotto l’assieme saldato",w.eval("nodo(bomDi('tg1'),'tg1-2').padre")==="tg1-0");
t("FitLab: una proposta",qa(".tela .nodo.proposto").length===1);
t("FitLab: perno tornito fuori, materiale del terzista",w.eval("CICLI['tg1|tg1-1'].fasi[0].matNostro")===false&&w.eval("CICLI['tg1|tg1-1'].mat.forn")==="terzista");
click('.tela .cella.ciclo[data-wfgo="tg1-0"]');
const mk=qa('.wfciclo input[data-wff$="|maschera"]');
t("saldature: spunta «in maschera» su ogni fase di saldatura",mk.length===2&&!q('[data-wff$="|mascheraNuova"]'));
const pb=q(".wfciclo .prep"), fz=q(".wfciclo .fasi");
t("preparazioni in testa al ciclo, fuori dalle fasi",!!pb&&!!(pb.compareDocumentPosition(fz)&w.Node.DOCUMENT_POSITION_FOLLOWING)&&!pb.closest(".fase"));
t("sette preparazioni come la maschera",qa(".wfciclo .prep-i").length===7&&q(".prep").textContent.includes("Attrezzatura di piega")&&q(".prep").textContent.includes("Stampo"));
mk[0].checked=true;mk[0].dispatchEvent(new w.Event("change",{bubbles:true}));
t("puntatura in maschera: la maschera è suggerita in alto",byText(".prep-i","Maschera di saldatura").textContent.includes("suggerita"));
t("e i controlli lo ricordano",q(".wfciclo .ctrl").textContent.includes("spuntala fra le preparazioni"));
const pm=q('.prep input[data-wfp="maschera"]');pm.checked=true;pm.dispatchEvent(new w.Event("change",{bubbles:true}));
t("maschera da realizzare spuntata in alto",w.eval("CICLI['tg1|tg1-0'].prep.maschera.on")===true&&!!q('.prep input[data-wfpn="maschera"]'));
const pn=q('.prep input[data-wfpn="maschera"]');pn.value="una maschera per 2 piastre";pn.dispatchEvent(new w.Event("change",{bubbles:true}));
t("nota sulla preparazione",w.eval("CICLI['tg1|tg1-0'].prep.maschera.nota")==="una maschera per 2 piastre");
t("riepilogo delle preparazioni nei controlli del prodotto",q(".wfside .prep-sum").textContent.includes("Maschera di saldatura"));
t("assieme saldato: sabbiatura con terzista qualificato",!(q(".wfciclo .ctrl")||{textContent:""}).textContent.includes("non è qualificato"));
click('[data-dview="struttura"]');
t("FitLab: nota di Lino sul disegno (non togliibile)",q('.nodo [data-dvis="9N008518AA.pdf"] .note-mini')!==null);
// ACME 990020412: due prodotti, 9990713C1 senza struttura
click('[data-tab="gest"]');click(byText("#gbody tr.click","990020412"));click('[data-step="1"]');
t("due prodotti nelle linguette",qa(".ptab").length===2);
t("9990712B1: pezzo unico, si conferma la distinta",!!q('.salva-barra.acc'));
click('.salva-barra.acc [data-dx="salva"]');t("9990712B1 verificata",w.eval("bomVerificata('at2')"));
click('[data-rp="at3"]');t("9990713C1: rifai l’analisi con lo STEP trovato",!!q('[data-dx="rifai"]'));
click('[data-dx="rifai"]');t("analisi rifatta",d.body.textContent.includes("Analisi rifatta"));
// TDL: struttura a mano
click('[data-tab="gest"]');click(byText("#gbody tr.click","Nuova RdO"));click('[data-step="1"]');
t("TDL: struttura non proposta",d.body.textContent.includes("Struttura non proposta"));
click('[data-dnuovo="sciolto"]');q("#dn-cod").value="S9.1248-01";q("#dn-nome").value="Lamiera";click('[data-dx="aggiungi"]');click('[data-dx="salva"]');
t("TDL: pezzo a mano salvato nei documenti",w.eval("DOCS.sd1.parti.length")===1);
click('[data-step="1"]');click('.tela .cella.ciclo[data-wfgo]:not([data-wfgo="r"])');
t("TDL: pezzo a mano senza ciclo",w.eval("S.dview")==="ciclo"&&!!q('[data-wf="proponi"]'));
click('[data-wf="proponi"]');t("ciclo tipico proposto: laser, piega, sbavatura",qa(".fasi .fase").length===3);
click('[data-dview="struttura"]');
click('[data-step="0"]');t("passo 1: il pezzo a mano chiede il 2D",d.body.textContent.includes("S9.1248-01"));
click('[data-step="2"]');t("Offerta segnaposto",d.body.textContent.includes("Arriva dopo la Distinta"));

// 8d. riadattamento allo schermo
t("classe dello schermo su <html>",["telefono","tablet","portatile","desktop","ampio","ultra"].includes(d.documentElement.dataset.schermo));
for(const [lw,k] of [[390,"telefono"],[1280,"portatile"],[1920,"ampio"],[2600,"ultra"]]){Object.defineProperty(w,"innerWidth",{value:lw,configurable:true});w.eval("applicaSchermo()");
  t(`${lw}px → ${k}`,d.documentElement.dataset.schermo===k);}
click('[data-tab="gest"]');click('[data-stato="avviata"]');t("elenco RFQ dentro la griglia adattiva",!!q(".gwrap .gmain")&&(!q(".senza")||!!q(".gwrap.con-senza .senza")));
click(byText("#gbody tr.click","990020338"));click('[data-step="1"]');click('[data-dview="struttura"]');
t("struttura: albero e scheda in due colonne",!!q(".strgrid .strmain .tela")&&!!q(".strgrid .strside .dett"));
click('[data-dview="ciclo"]');t("ciclo: disegno, fasi e controlli",!!q(".rfq.ciclo .wfgrid .wfside"));

// 8e. il contratto con il backend: fasi, DTO, rotte
t("fasi del backend: ACME in ATTESA_DISEGNI per la transizione automatica",w.eval("rf('q-ar1').fase")==="ATTESA_DISEGNI"||w.eval("rf('q-ar1').fase")==="FATTIBILITA");
t("Mieti Gamma: thread CHIUSO, niente «consegnata»",w.eval("rf('q-cl1').statoThread")==="CHIUSA"&&w.eval("rf('q-cl1').stato")==="terminata"&&!d.documentElement.innerHTML.includes("consegnata"));
t("Potaflex persa: fase terminale",w.eval("rf('q-pl0').fase")==="PERSA");
t("ogni fase è del catalogo del backend",w.eval("RFQ.every(q=>ENUM.fase.includes(q.fase))"));
click('[data-tab="gest"]');click('[data-stato="avviata"]');
t("la gestione mostra la colonna Fase",qa("#gbody th").some(x=>x.textContent==="Fase"));
click(byText("#gbody tr.click","RDO 2026/318"));click('[data-step="1"]');
click('[data-dev="apri"]');const dto=q("pre.dto")&&q("pre.dto").textContent;
t("pannello DTO con i nomi del backend",!!dto&&["ProdottoValutato","StatoFascicolo","CompletezzaDocumentale","NodoBOM","Nomenclatura","Gerarchia","FaseThread"].every(k=>dto.includes(k)));
t("tipi del backend nei DTO",dto.includes('"sottoassieme"')&&dto.includes('"disegno_2d"')&&!dto.includes('"sottoass"'));
click('[data-dev="chiamate"]');
t("la Distinta si apre con una GET, senza prepara",byText("table.chiamate td","/thread/q-tg1/distinta")&&!q("table.chiamate").textContent.includes("/prepara"));
click('[data-dev="chiudi"]');click('[data-step="0"]');click('[data-cerca]');click('[data-dev="apri"]');click('[data-dev="chiamate"]');
t("«Cerca di nuovo» chiama rianalizza",q("table.chiamate").textContent.includes("/fascicolo/rianalizza"));
t("le rotte mancanti dicono la richiesta",w.eval("Object.values(GESTI).some(g=>g.stato==='R-06')&&Object.values(GESTI).every(g=>/^(esiste|SV|R-[0-9][0-9])$/.test(g.stato))"));
click('[data-dev="chiudi"]');

// 9. impostazioni
click('[data-tab="set"]');t("impostazioni: solo 2 sezioni",qa(".p-rail [data-setsez]").length===2);
t("niente radio modalità",!q("[data-radio]"));
click('[data-setsez="nascosti"]');click('[data-autorizza="n2"]');
t("Giorgia autorizzata → Da smistare",w.eval("SMISTARE.some(s=>s.id==='s-n2')")&&w.eval("cl('acme').mittenti.some(x=>x.e==='m.sali@acme-trattori.example')"));
click('[data-setsez="clienti"]');click('[data-setcli="fitlab"]');q("#nm-n").value="Marco Bassini";q("#nm-e").value="m.bassini@fitlab.example";click('[data-addmitt]');
t("mittente aggiunto",w.eval("cl('fitlab').mittenti.length")===4);
click('[data-mitt="2"]');t("mittente riattivato",w.eval("cl('fitlab').mittenti[2].on")===true);
click('[data-nuovo="1"]');q("#fc-nome").value="Solleva Epsilon Italia";click('[data-salvacli]');t("nuovo cliente creato",w.eval("CLIENTI.some(c=>c.nome==='Solleva Epsilon Italia')"));
// la richiesta di Giorgia si crea
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s-n2"]');click('[data-nuovareq="s-n2"]');click('[data-creareq]');
t("richiesta di Giorgia creata in ACME",w.eval("reqDi('acme').length")===3);

console.log(`\n${ok} superati · ${ko} falliti · errori JS: ${errori.length}`);errori.slice(0,5).forEach(e=>console.log("  JS: "+e));
process.exit(ko||errori.length?1:0);
