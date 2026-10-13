/* test_clic.cjs · v15 passate 1 e 2 (SPEC_v15 §8.1). jsdom, una sola esecuzione alla volta.
   Sezioni: 1-7 Inbox, smistamento, gestione (tenute da v13, adattate); T-Z invarianti dopo ogni sezione;
   T-A zona A (VFR, ACME, FitLab, RFQ nuova); T-C zona C; T-V forme statiche; T-K tastiera; 9 impostazioni.
   Passata 2: T-B (Come, Ciclo, Sequenza, Esito), T-R (le 11 richieste fino in fondo), T-O (Foglio per Franco, Costi,
   Listini, Offerta), T-K della zona B, T-G (giro completo) e §9.2 (ogni pulsante fa qualcosa). */
const {JSDOM,VirtualConsole}=require("jsdom");const fs=require("fs");
const html=fs.readFileSync(require("path").join(__dirname,"..","cockpit-frontend-mockup-v16.html"),"utf8");
const errori=[];const vc=new VirtualConsole();vc.on("jsdomError",e=>errori.push(e.message));vc.on("error",e=>errori.push(String(e)));
const dom=new JSDOM(`<!doctype html><html><head><meta charset="utf-8"></head><body>${html}</body></html>`,{runScripts:"dangerously",virtualConsole:vc,pretendToBeVisual:true});
const w=dom.window,d=w.document;let ok=0,ko=0;
const t=(nome,cond)=>{if(cond){ok++;}else{ko++;console.log("✗ "+nome);}};
const q=s=>d.querySelector(s), qa=s=>[...d.querySelectorAll(s)];
const click=(s,nome)=>{const el=typeof s==="string"?q(s):s;if(!el){ko++;console.log("✗ manca da cliccare: "+(nome||s));return false;}el.click();return true;};
const byText=(sel,txt)=>qa(sel).find(e=>e.textContent.includes(txt));
const nMsg=()=>qa(".p-conv .msg").length;
const avviso=()=>{const a=q("#ma-avviso");return a&&!a.hidden?{esito:a.getAttribute("data-esito"),testo:a.textContent.trim()}:null;};
const si=()=>click("[data-ma-si]","Sì della domanda");
const ev=s=>w.eval(s);
const submit=(form,nome)=>{if(!form){ko++;console.log("✗ manca il modulo: "+nome);return;}const b=form.querySelector("button:not([type=button])");if(form.requestSubmit)form.requestSubmit(b||undefined);else b.click();};
/* v16: la stazione ① si apre sui Documenti; le prove della Distinta passano alla vista Distinta (un clic, come Lino) */
const apriRFQ=num=>{click('[data-tab="gest"]');click('[data-gfase="tutte"]');click(byText("#gbody tr.click",num),"RFQ "+num);if(q('[data-vista="distinta"]'))click('[data-vista="distinta"]');};

/* ---- invarianti T-Z, eseguite dopo ogni sezione ---- */
const FASI0r=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).map(([k,T])=>[k,[T.testa.fase&&T.testa.fase.nome,T.testa.stato_thread]])))"));
const FASI0=Object.fromEntries(Object.entries(FASI0r).filter(([,v])=>v[0]));  /* le RFQ create dall'Inbox ricevono RICEVUTA dal gesto di creazione (AM) */
const ROTTE=ev("ROTTE.map(compilaRotta).map(r=>r.source)").map(s=>new RegExp(s));
function Z(sez){
  const ora=JSON.parse(ev("JSON.stringify(Object.fromEntries(Object.entries(THREADS).map(([k,T])=>[k,[T.testa.fase&&T.testa.fase.nome,T.testa.stato_thread]])))"));
  t(`Z1 [${sez}] fase e thread invariati`,Object.keys(FASI0).every(k=>JSON.stringify(FASI0[k])===JSON.stringify(ora[k])));
  const g=qa("[data-ma-gesto]").map(e=>e.getAttribute("data-ma-gesto"));
  t(`Z4 [${sez}] ogni data-ma-gesto nella lista chiusa`,g.every(u=>ROTTE.some(r=>r.test(u))));
  t(`Z4 [${sez}] nessun data-pa-gesto nella lista`,qa("[data-pa-gesto]").every(e=>!ROTTE.some(r=>r.test(e.getAttribute("data-pa-gesto")))));
  t(`Z4 [${sez}] nessun valore con nodo: o componente:`,qa("input,option,[data-ma-campi]").every(e=>!/^(nodo|componente):/.test(e.value||"")&&!/"(nodo|componente):/.test(e.getAttribute("data-ma-campi")||"")));
  t(`Z5 [${sez}] errori JS = 0`,errori.length===0);
}
/* Z2: all'apertura di una stazione nessuna scelta fatta al posto di chi guarda */
function Z2(sez){
  t(`Z2 [${sez}] nessun input spuntato`,!qa("input[type=checkbox],input[type=radio]").some(x=>x.checked));
  t(`Z2 [${sez}] nessuna tendina con un valore scelto`,qa("select").every(s=>s.value===""));
  t(`Z2 [${sez}] nessuna qta scritta`,qa('input[name=qta]').every(x=>x.value===""));
}
/* Z3: nel file assemblato non ci sono le funzioni di RV-03 né le parole vietate */
for(const f of ["faseCorrente","gruppoDi","assiProdotto","treStati","minuteriaConfermata","cellaNodo","bomVerificata","statoDistinta","qtaTot","puoContenere","formNuovo","decisioniPendenti","improntaFascicolo","confermaFascicolo","cercaDiNuovo","preparaRFQ"])
  t("Z3 niente "+f,!new RegExp("function "+f+"\\b|const "+f+"\\b").test(html));
for(const p of ["Metti da parte","bom/applica\"","codice automatico","Codice interno proposto","Salva la distinta","Accetta la struttura proposta","Conferma e copia sul NAS"])
  t("Z3 niente «"+p+"»",!html.includes(p));
t("Z3 un solo tema chiaro: niente token scuri né codice del tema",!/prefers-color-scheme|data-theme|data-tema|cambiaTema|applicaTema/.test(html.replace(/test_clic[\s\S]*$/,"")));
(()=>{const css=html.split("</style>").map(x=>x.split("<style>")[1]||"").join("\n");const fuori=css.replace(/:root\{[\s\S]*?fine dei token/,"");
  t("M12 nessun colore scritto a mano fuori dai token",!/#[0-9a-fA-F]{3,6}\b|rgba?\(/.test(fuori));})();

// 1. elementi tolti e guscio
t("tre schede",qa("#tabs button").length===3);
t("A2 .brand-sub",!!q(".brand-sub")&&!q(".brand .sub"));
Z("apertura");

// 2. VignaBot: una richiesta, 4 messaggi, filtro 4/3
click('[data-cli="vignabot"]');
t("VignaBot una richiesta",qa(".p-list [data-req]").length===1);
t("VignaBot 4 messaggi",nMsg()===4);
t("A1 dimensione dei file in .fsize",qa(".file .fsize").length>0&&!q(".file .fs"));
t("B6 title sui nomi dei clienti",qa(".p-rail [data-cli] .lbl").every(x=>x.title));
click('.pfilter [data-pf="vb1"]');t("filtro 902235_B = 4",nMsg()===4);
click('.pfilter [data-pf="vb2"]');t("filtro 902256_B = 3",nMsg()===3);
click('.pfilter [data-pf=""]');
t("contesto: stati letti dal server",q(".p-ctx").textContent.includes("in attesa di conferma"));

// 3. crea RFQ VignaBot: la Distinta si apre senza scrivere niente (DB7-6)
const nRFQ0=ev("RFQ.length");
click('.rfqblock [data-crearfq="r-vb1"]');
t("Distinta aperta sulla stazione ①",!!q(".ws")&&q('.staz[aria-current="step"]').textContent.includes("Struttura e documenti"));
t("aprire non scrive niente",ev("REGISTRO.length")===0);
t("VignaBot: due prodotti nel menu",(click('[data-menu="prodotti"]'),qa(".tendina--prodotti [data-prodotto]").length===2));
click(".tendina--prodotti [data-prodotto]:nth-child(2)");
t("cambio prodotto: 902256_B",q("#ma-prodotti").textContent.includes("902256_B"));
Z2("VignaBot");
click('[data-menu="app"]');click('.tendina--app [data-tab="inbox"]');click('[data-cli="vignabot"]');click('.rfqblock [data-apririfq]');
t("riapre la stessa RFQ, nessun doppione",ev("RFQ.length")===nRFQ0+1);
Z("VignaBot");

// 5. Da smistare → Carrelli Nord → nuova richiesta → RFQ: thread non preparato, «Prepara i file» è solo un pulsante
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s2"]');
click('[data-nuovareq="s2"]');t("nuova richiesta: nessun codice spuntato in partenza",qa('[data-cod]').length>0&&qa('[data-cod]').every(c=>!c.checked));
click('[data-creareq]');t("senza spunta la richiesta non nasce",ev("S.ok&&S.ok.txt").includes("Spunta almeno"));
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s2"]');click('[data-nuovareq="s2"]');qa('[data-cod]').forEach(c=>c.checked=true);click('[data-creareq]');
t("Carrelli Nord: conversazione con blocco RFQ",ev("S.cli")==="carrellinord"&&!!q('.rfqblock [data-crearfq]'));
click('.rfqblock [data-crearfq]');
t("v16: la Distinta si apre sui Documenti",q('#ma-viste [aria-current="page"]').dataset.vista==="documenti"&&!!q("#ma-documenti"));
click('[data-vista="distinta"]');
t("Carrelli Nord: fonte non calcolata (file non preparati)",q("#ma-fonte").textContent.includes("non calcolata"));
t("Carrelli Nord: niente scritto all'apertura",ev("REGISTRO.filter(x=>x.zona==='A'&&x.metodo==='POST').length")===0);
click('[data-ma-gesto$="/fascicolo/prepara"]');
t("Prepara i file: ok e l'esito nuovo",avviso().esito==="ok"&&!q("#ma-fonte").textContent.includes("non calcolata"));
t("prepara mai con auto",ev(`eseguiSubito("/thread/"+threadAperto().id+"/fascicolo/prepara",{auto:"1"}).esito`)==="errore");
Z("Carrelli Nord");

// 6. aggancio
click('[data-menu="app"]');click('.tendina--app [data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s1"]');
const nAr=ev("mail(msgDi('r-ar1')).length");click('[data-aggancia="s1"]');
t("aggancio: messaggio nella richiesta",ev("mail(msgDi('r-ar1')).length")===nAr+1);

// 7. gestione: gruppi per fase registrata e stato del thread, letti
click('[data-tab="gest"]');
t("gruppo Fattibilità: 2",+q('[data-gfase="FATTIBILITA"] .cnt').textContent===2);
t("gruppo thread chiusi: 1",+q('[data-gfase="chiuse"] .cnt').textContent===1);
const g=q("#gq");g.value="falci";g.dispatchEvent(new w.Event("input",{bubbles:true}));
t("ricerca Falci Beta: una riga",qa("#gbody tr.click").length===1);
g.value="";g.dispatchEvent(new w.Event("input",{bubbles:true}));
Z("gestione");

/* ================= T-A · zona A: VFR, il percorso con le tre conferme in tre posti ================= */
apriRFQ("482-00");
const tid=ev("threadAperto().id");
t("VFR: fonte non confermata all'inizio",q("#ma-fonte").textContent.includes("in attesa di conferma"));
t("VFR: BOM non verificata",q("#ma-albero").textContent.includes("BOM non verificata"));
t("VFR: una scheda candidata con la frase fissa",qa(".ma-scheda--cand").length===1&&q(".ma-nota--cand").textContent.includes("Proposta dal file, non è la BOM. Nessuna struttura è scelta finché non si autorizza uno STEP"));
t("U-07 alla lettera in R4",q("#ma-albero .ma-u07").textContent.includes("La strada principale è \"Conferma l'albero\", dopo il riepilogo. Le decisioni nodo per nodo non rendono la BOM verificata e, prese prima del riepilogo, possono lasciarla da verificare."));
t("anteprima vuota: la frase fissa",q("#ma-anteprima").textContent.includes("Scegli una casella: qui compaiono il disegno, i codici, la posizione e i gesti."));
t("tre posti: ma-fonte, ma-albero, documenti",!!q("section#ma-fonte")&&!!q("section#ma-albero")&&!!q("#ma-viste"));
t("primario: Autorizza lo STEP (unica candidata)",qa(".ws-main .ma-btn--pri, .ws-main .btn.pri").length===1&&q(".ws-main .ma-btn--pri").textContent.includes("Autorizza lo STEP"));
t("tela: 9 caselle, nessuna scelta",qa(".ma-dtree li").length===9&&!q('.ma-dtree li[aria-selected="true"]'));
t("tratteggio con le parole: ×n dal file e da confermare",qa(".ma-dtree li.ma-li--file").length===8&&qa(".ma-qta-arco").every(x=>/dal file/.test(x.textContent)&&/da confermare/.test(x.textContent)));
t("A11: schizzi diversi per famiglia",(()=>{const sk=n=>ev(`schizzo("${n}")`);return sk("F+800.514263.001 01 1-2.PDF")!==sk("F+800.514263.002.pdf")&&sk("F+800.514263.002.pdf")!==sk("F+800.514263.008.pdf");})());
// un nodo di una candidata: il server dice no prima dell'autorizzazione
click(byText(".ma-nodo","514263.002"));
t("R6: testata del pezzo e gesti",!!q("#ma-anteprima .tp")&&!!q('#ma-anteprima form[data-ma-gesto*="/nodo/"][data-ma-gesto$="/accetta"]'));
submit(q('#ma-anteprima form[data-ma-gesto*="/nodo/"][data-ma-gesto$="/accetta"]'),"accetta (candidata)");
t("candidata: accetta → no (lo STEP non è autorizzato)",avviso().esito==="no"&&ev("threadAperto().prodotti[0].nodi.length")===0);
// 3 · conferma della fonte: l'anteprima, «autorizzo» non spuntato, Autorizza acceso solo dopo
click('[data-ma-azione="step"]');
t("anteprima JSON letta (GET della lista)",ev("REGISTRO[0].metodo")==="GET"&&ev("REGISTRO[0].url").includes("/motore-a/anteprima/step-strutturale"));
t("autorizzo non spuntato, Autorizza spento",!q('.ma-finestra input[name=autorizzo]').checked&&q('[data-ma-azione="step-invia"]').disabled);
t("figli diretti nell'anteprima",qa(".ma-frasi--figli li").length===8);
const cb=q('.ma-finestra input[name=autorizzo]');cb.checked=true;cb.dispatchEvent(new w.Event("change",{bubbles:true}));
t("Autorizza acceso dopo la spunta",!q('[data-ma-azione="step-invia"]').disabled);
click('[data-ma-azione="step-invia"]');
t("fonte confermata, BOM di lavoro con 9 nodi",avviso().esito==="ok"&&q("#ma-fonte").textContent.includes("confermata")&&qa(".ma-scheda--bom").length===1);
t("R3 chiusa in una riga dopo la conferma",q("#ma-fonte").classList.contains("ma-analisi-box--chiusa"));
t("la fonte non verifica la BOM",q("#ma-albero").textContent.includes("BOM non verificata"));
t("primario ora: Leggi il riepilogo e conferma l'albero",q(".ws-main .ma-btn--pri").textContent.includes("conferma l'albero"));
// 4 · un nodo: accetta → BOM resta da verificare; commerciale → no
click(byText(".ma-nodo","514263.002"));
t("U-07 sopra Accetta il nodo",q("#ma-anteprima .ma-gesti .ma-u07").textContent.startsWith("La strada principale"));
submit(q('#ma-anteprima form[data-ma-gesto*="/nodo/"][data-ma-gesto$="/accetta"]'),"accetta");
t("Accetta il nodo: ok, BOM resta da verificare (DB7-4)",avviso().esito==="ok"&&ev("threadAperto().prodotti[0].bom.verificata")===false&&q("#ma-albero").textContent.includes("BOM non verificata"));
const pid=ev("threadAperto().prodotti[0].nodi.find(n=>n.nodo.riga_legacy).nodo.riga_legacy.id");
const statoPrima=ev("JSON.stringify(threadAperto().prodotti[0].nodi.map(n=>!!n.nodo.decisione))");
t("accetta con commerciale → no, niente cambia",ev(`eseguiSubito("/thread/${tid}/fascicolo/nodo/${pid}/accetta",{tipo:"commerciale"}).esito`)==="no"&&ev("JSON.stringify(threadAperto().prodotti[0].nodi.map(n=>!!n.nodo.decisione))")===statoPrima&&q("#ma-avviso").getAttribute("data-esito")==="no");
t("rotta fuori lista → rifiutata senza richiesta",ev(`eseguiSubito("/thread/${tid}/distinta/bom/applica",{}).esito`)==="errore"&&q("#ma-avviso").textContent.includes("non prevista dal contratto"));
// Collega con la qtà vuota: il modulo non parte
const collega=q('#ma-anteprima form[data-ma-gesto$="/collega"]');
t("Collega: qta vuota e required, padre su «scegli»",collega.querySelector("input[name=qta]").required&&collega.querySelector("input[name=qta]").value===""&&collega.querySelector("select[name=padre]").value==="");
const nReg=ev("REGISTRO.length");submit(collega,"collega vuoto");
t("Collega senza qtà non parte",ev("REGISTRO.length")===nReg);
t("Rev e descrizione spento senza descrizione registrata",byText("#ma-anteprima .ma-na","Salva rev e descrizione")&&byText("#ma-anteprima .ma-na","Salva rev e descrizione").querySelector("button").disabled);
// la radice: il codice del target è spento (DB7-3)
click(byText(".ma-nodo","514263.000"));
t("codice del prodotto spento (DB7-3)",!!byText("#ma-anteprima .ma-na","Correggi il codice del componente")&&!q('#ma-anteprima form[data-ma-gesto$="/codice"]'));
t("Rev e descrizione acceso per il prodotto (descrizione registrata)",!!q('#ma-anteprima form[data-ma-gesto$="/modifica"]'));
t("M11: testata del pezzo in R6",!!q("#ma-anteprima .tp"));
// 5 · conferma dell'albero: riepilogo, poi Conferma
click('[data-ma-azione="albero"]');
t("riepilogo letto e confermabile",ev("REGISTRO[0].url").includes("/distinta/albero/riepilogo")&&!q('[data-ma-azione="albero-invia"]').disabled);
t("riepilogo: commerciali e componenti nuovi",byText(".ma-riep summary","Commerciali")&&byText(".ma-riep summary","Componenti nuovi"));
const firmaVecchia=ev("S.dialogo.r.firma"), baseVecchia=ev("S.dialogo.r.albero_firma");
click('[data-ma-azione="albero-invia"]');
t("v16 rev. (P-03): dopo «Conferma l'albero» nessuna vista scelta dal JS; i Documenti sono nella stessa pagina",q('#ma-viste [aria-current="page"]').dataset.vista==="distinta"&&avviso().esito==="ok"&&!!q("#ma-documenti")&&!!q("#ma-albero"));
t("Conferma l'albero: BOM verificata",avviso().esito==="ok"&&ev("threadAperto().prodotti[0].bom.verificata")===true&&q("#ma-albero").textContent.includes("BOM verificata"));
t("bozza vuota mandata (formato 1, base)",ev("JSON.stringify(REGISTRO[0].campi.bozza)")===JSON.stringify({formato:1,base:baseVecchia}));
t("legami pieni con ✓×m confermata",qa(".ma-dtree li.ma-li--conf").length===8&&qa(".ma-qta--conf").length>=8);
t("P-13: «Poi: ② Come →» nella barra delle stazioni, fuori da #ma",!!q(".stazioni .staz-poi")&&q(".stazioni .staz-poi").textContent.includes("Come")&&!q("#ma .ma-btn--pri[data-staz]"));
t("la fase non cambia con la BOM verificata",ev("threadAperto().testa.fase.nome")==="FATTIBILITA");
// 6 · conferma documentale, voce per voce, con la domanda; NAS in coda → scritto
click('[data-vista="documenti"]');
t("documenti: 9 voci, 6 da confermare",qa("tr.ma-voce").length===9&&qa('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]').length===6);
t("il commerciale (dado M14) chiede il 2D",byText("tr.ma-voce","ISO 4032 M14")&&byText("tr.ma-voce","ISO 4032 M14").textContent.includes("manca"));
const b1=q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]');
t("una sola voce per conferma",Object.keys(JSON.parse(b1.getAttribute("data-ma-campi"))).join()==="voce");
click(b1);
t("la domanda in pagina con la frase del contratto",!!q("#ma-domanda")&&q("#ma-domanda").textContent.includes("La copia sul NAS parte subito"));
si();
t("confermato · copia in coda (confermato ≠ copiato)",avviso().esito==="ok"&&qa("tr.ma-voce")[0].textContent.includes("copia sul NAS in coda")&&!qa("tr.ma-voce")[0].textContent.includes("copiato sul NAS"));
click('[data-ma-azione="aggiorna"]');
t("alla lettura dopo: copiato sul NAS",qa("tr.ma-voce")[0].textContent.includes("copiato sul NAS"));
// No alla domanda: niente parte
const nR2=ev("REGISTRO.length");click(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]'));click("[data-ma-no]");
t("No alla domanda: niente parte",ev("REGISTRO.length")===nR2&&!q("#ma-domanda"));
// Da smistare: il DWG «da decidere» si rifiuta (U-08); lo scarto è definitivo
const dwg=byText("#ma-da-smistare .ma-pick","02.DWG");
click(dwg.querySelector('[data-ma-gesto$="/fascicolo/conferma"]'));si();
t("DWG da decidere: conferma rifiutata (U-08)",avviso().esito==="no");
const sc=byText("#ma-da-smistare .ma-pick","02.DWG").querySelector('[data-ma-gesto^="/proposta/"]');
t("Scarta il file: titolo DB7-8",sc.title==="Lo scarto del file è definitivo (DB7-8).");
click(sc);si();
t("file scartato, fra gli scartati",avviso().esito==="ok"&&!byText("#ma-da-smistare .ma-pick","02.DWG")&&byText(".ma-fs--chiusa","Scartati").textContent.includes("02.DWG"));
t("ritorna_thread aggiunto dal ponte",ev("REGISTRO[0].campi.ritorna_thread")===tid);
// Ispeziona: Togli dal componente = assegna con componente vuoto
click(qa('tr.ma-voce [data-ma-azione="ispeziona"]')[1]);
const togli=byText(".ma-finestra form","Togli dal componente");
submit(togli,"togli");si();
t("Togli dal componente: assegna vuoto, né scarto né conferma",ev("REGISTRO[0].url").endsWith("/fascicolo/assegna")&&ev("REGISTRO[0].campi.componente")===""&&avviso().testo.includes("Non è scartato"));
Z("VFR documenti");
// R10: assi letti
click('[data-vista="rfq"]');
t("R10: sette assi",qa(".ma-asse").length===7&&!!q("#ma-riepilogo"));
// stazione ③: elenco con il prefisso comune, testata uguale (M11), C4
click('.staz[data-staz="3"]');
t("C4: prefisso comune in testa",q(".cl-pre").textContent==="F+800.514263");
const suf=qa(".cl-suf").map(x=>x.textContent);
t("C4: suffissi tutti diversi",new Set(suf).size===suf.length&&suf.includes(".008"));
t("M11: la stessa testataPezzo nel Ciclo",!!q(".cl-foglio .tp")&&!!q(".cl-foglio .tp .tp-r3"));
t("Ciclo: marchio della zona B (v16: R-10 nel title della fascia)",!!q(".banda-b")&&q(".banda-b").getAttribute("title").includes("R-10"));
Z("VFR");

/* ================= ACME: due candidate, un conflitto che blocca, NAS in errore ================= */
apriRFQ("990020338");
t("ACME: due candidate, nessuna primaria (nessuna scelta)",qa(".ma-scheda--cand").length===2&&qa('[data-ma-azione="step"]').length===2&&!qa('[data-ma-azione="step"]').some(b=>b.classList.contains("ma-btn--pri")));
t("ACME: minuteria proposta, non esente",qa(".ma-cat").some(x=>x.textContent.includes("minuteria proposta")));
click('[data-ma-azione="step"]');const cb2=q('.ma-finestra input[name=autorizzo]');cb2.checked=true;cb2.dispatchEvent(new w.Event("change",{bubbles:true}));
t("ACME: con un'altra candidata serve la presa d'atto",!!q('.ma-finestra input[name=presa_d_atto]')&&q('[data-ma-azione="step-invia"]').disabled);
const pa=q('.ma-finestra input[name=presa_d_atto]');pa.checked=true;pa.dispatchEvent(new w.Event("change",{bubbles:true}));
click('[data-ma-azione="step-invia"]');
t("ACME: STEP autorizzato",avviso().esito==="ok");
t("ACME: conflitto di codice visibile",q("#ma-conflitti-struttura").textContent.includes("9443580X1"));
click('[data-ma-azione="albero"]');
t("ACME: il riepilogo ha un blocco, Conferma spenta",q(".ma-finestra .ma-blocchi")&&q('[data-ma-azione="albero-invia"]').disabled);
const r0=JSON.parse(ev("JSON.stringify(S.dialogo.r)"));click('[data-ma-azione="chiudi-dialogo"]');
// correggo il codice del nodo: il conflitto si risolve
click(byText(".ma-nodo","9443580X"));
const fc=q('#ma-anteprima form[data-ma-gesto*="/nodo/"][data-ma-gesto$="/codice"]');fc.querySelector("input[name=codice]").value="9443580X1";
t("il modulo del codice manda sempre rev",!!fc.querySelector("input[name=rev]"));
submit(fc,"codice");
t("codice corretto a mano: conflitto risolto",avviso().esito==="ok"&&avviso().testo.includes("conflitto"));
t("firma vecchia → no e niente cambia",ev(`confermaAlberoSubito(${JSON.stringify(r0.firma)},{formato:1,base:${JSON.stringify(r0.albero_firma)}}).esito`)==="no"&&ev("threadAperto().prodotti[0].bom.verificata")===false);
click('[data-ma-azione="albero"]');click('[data-ma-azione="albero-invia"]');
t("ACME: albero confermato dopo la correzione",ev("threadAperto().prodotti[0].bom.verificata")===true);
click('[data-vista="documenti"]');
t("ACME: la copia del 2D in errore si vede come errore",qa("tr.ma-voce")[0].textContent.includes("copia sul NAS in errore"));
t("Riprova le copie: primario solo con un errore",q('[data-ma-gesto$="/riprova-copie"]').classList.contains("ma-btn--pri"));
click('[data-ma-gesto$="/riprova-copie"]');click('[data-ma-azione="aggiorna"]');
t("Riprova → in coda → scritto",qa("tr.ma-voce")[0].textContent.includes("copiato sul NAS"));
Z("ACME");

/* ================= FitLab: un pezzo sotto due padri; Sposta e Scollega ================= */
apriRFQ("RDO 2026/318");
t("FitLab: BOM verificata",q("#ma-albero").textContent.includes("BOM verificata"));
t("un figlio con due padri compare sotto tutti e due",qa(".ma-dtree li").filter(li=>li.querySelector(":scope>.ma-nodo-box .ma-nodo-cod").textContent.includes("9N008519AB")).length===2);
t("⧉ 2 padri nel file",qa(".ma-badge").some(x=>x.textContent.includes("2 padri nel file")));
click(qa(".ma-nodo").find(x=>x.textContent.includes("9N008520AA")));
const sp=q('#ma-anteprima form[data-ma-gesto$="/sposta"]');
t("Sposta: da, a su «scegli», qta vuota; «— diventa radice» esplicito",sp.querySelector("select[name=a]").value===""&&[...sp.querySelectorAll("select[name=a] option")].some(o=>o.textContent.includes("diventa radice"))&&sp.querySelector("input[name=qta]").value==="");
// sotto un particolare: il server rifiuta
const perno=ev("Object.values(threadAperto().componenti).find(c=>c.codice==='9N008519AB').id");
const sel=sp.querySelector("select[name=a]");sel.value=perno;const sda=sp.querySelector("select[name=da]");if(sda)sda.selectedIndex=1;sp.querySelector("input[name=qta]").value="1";
submit(sp,"sposta sotto un particolare");
t("Sposta sotto un particolare → no, niente cambia",avviso().esito==="no"&&ev("threadAperto().prodotti[0].bom.verificata")===true);
click(qa(".ma-nodo").find(x=>x.textContent.includes("9N008520AA")));
submit(q('#ma-anteprima form[data-ma-gesto$="/scollega"]'),"scollega");
t("Scollega con la sua domanda",q("#ma-domanda")&&q("#ma-domanda").textContent.includes("Il componente resta"));si();
t("Scollega: l'albero torna da confermare",avviso().esito==="ok"&&ev("threadAperto().prodotti[0].bom.verificata")===false);
t("trascinare sul padre di adesso non apre niente (non è uno spostamento)",(()=>{const n=q(".ma-nodo[data-ma-sposta]"),pad=qa(".ma-nodo[data-ma-drop]").find(x=>x.dataset.maDrop===n.dataset.maDa);if(!n||!pad)return true;
  const mk=tp=>new w.Event(tp,{bubbles:true,cancelable:true});n.dispatchEvent(mk("dragstart"));pad.dispatchEvent(mk("dragover"));pad.dispatchEvent(mk("drop"));return !q('.ma-finestra form[data-ma-gesto$="/sposta"]');})());
t("trascinare apre il modulo Sposta, non sposta da solo",(()=>{const n=q(".ma-nodo[data-ma-sposta]"),dst=qa(".ma-nodo[data-ma-drop]").find(x=>x!==n&&x.dataset.maDrop!==n.dataset.maDa);if(!n||!dst)return false;
  const mk=tp=>{const e=new w.Event(tp,{bubbles:true,cancelable:true});return e;};n.dispatchEvent(mk("dragstart"));dst.dispatchEvent(mk("dragover"));dst.dispatchEvent(mk("drop"));
  const f=q('.ma-finestra form[data-ma-gesto$="/sposta"]');return !!f&&f.querySelector("input[name=qta]").value===""&&f.querySelector("input[name=qta]").required;})());
click('[data-ma-azione="chiudi-dialogo"]');
Z("FitLab");

/* ================= T-C · zona C: spento, con il motivo visibile ================= */
apriRFQ("482-00");
t("strumenti dell'albero spenti con il motivo",qa(".ma-strumenti button").every(b=>b.disabled)&&q("#ma-strumenti-perche").textContent.includes("DB7-5"));
click('[data-vista="documenti"]');
/* v16: barra visibile, etichette e motivi alla lettera di motore_a.html:1108 */
for(const x of ["Cerca di nuovo sul portale","Carica a mano","Conferma tutti i riconosciuti","Conferma la minuteria"]){
  const n=byText("#ma-documenti .ma-toolbar .ma-na",x);t("zona C: "+x,!!n&&!n.closest("details")&&n.querySelector("button").disabled&&/^Non ancora disponibile/.test(n.querySelector("small").textContent));}
click('[data-menu="cartiglio"]');
t("Congela la V1 spento nel cartiglio",byText(".tendina--cartiglio .ma-na","Congela la V1").querySelector("button").disabled);
click('[data-menu="cartiglio"]');
const fase0=ev("threadAperto().testa.fase.nome");qa(".ma-na button").forEach(b=>b.click());
t("i pulsanti spenti non cambiano niente",ev("threadAperto().testa.fase.nome")===fase0);
Z("zona C");

/* ================= T-V · forme statiche e T-K · tastiera, cassetti, tema ================= */
click('[data-vista="distinta"]');
t("un solo primario nella stazione",qa(".ws-main .ma-btn--pri, .ws-main .btn.pri").length<=1);
t("testata: tre celle (fase, thread, prodotto)",!!q('.tt-cella[aria-label="Fase registrata"]')&&!!q('.tt-cella[aria-label="Stato del thread"]')&&!!q("nav#ma-prodotti"));
t("lo stato del prodotto sta nel menu dei prodotti",q("nav#ma-prodotti").textContent.includes("non pronto"));
t("barra delle stazioni: cinque stazioni; cassetti Richieste · Controlli · ⋯ (v16)",qa(".staz").length===5&&qa(".cas-btn").length===3&&!!q('.cas-btn[data-menu="altro"]'));
const key=(k)=>d.body.dispatchEvent(new w.KeyboardEvent("keydown",{key:k,bubbles:true}));
key("g");key("3");t("G 3 → Ciclo",ev("S.staz")===3);
key("g");key("1");t("G 1 → Struttura",ev("S.staz")===1);
key("i");t("I apre «Non ancora nel backend»",!!q("#cassetto")&&q("#cassetto").textContent.includes("Zona A"));
click('[data-castab="json"]');t("JSON nella forma di /motore-a/dati",/"thread_id"[\s\S]*"testa"[\s\S]*"esito"[\s\S]*"prodotti_valutati"/.test(q(".cas-json").textContent));
click('[data-castab="gesti"]');t("registro dei gesti A",qa(".cas-reg li").length>0);
key("Escape");t("Esc chiude il cassetto",!q("#cassetto"));
key("?");t("? mostra i tasti",!!q(".tasti"));key("Escape");
key("r");t("R: cassetto Richieste con il marchio B",q("#cassetto")&&!!q("#cassetto .banda-b"));key("Escape");
click('.staz[data-staz="2"]');t("stazione ② si apre con il marchio della zona B",!!q(".staz-b .banda-b"));
click('.staz[data-staz="1"]');
t("tema: non c'è un pulsante del tema (v16, solo chiaro)",!q("[data-tema]")&&!d.documentElement.hasAttribute("data-theme"));
click('[data-menu="app"]');click('[data-menu="ruolo"]');click('[data-ruolo="Fabio"]');t("ruolo simulato (v16: dal menu ≡ «Vedi come»)",q(".tt-ruolo").textContent.includes("Fabio")&&q(".tt-ruolo").textContent.includes("simulato"));
Z("tastiera");

/* ================= passata 2 · T-B zona B: il viaggio di Lino su VFR, da Come alla consegna (SPEC §8.1) ================= */
const tasto=(k,o={},el)=>(el||d.body).dispatchEvent(new w.KeyboardEvent("keydown",Object.assign({key:k,bubbles:true,cancelable:true},o)));
const cambia=(el,v,nome)=>{ if(!el){ko++;console.log("✗ manca il campo: "+(nome||""));return;} if(el.type==="checkbox") el.checked=v; else el.value=v; el.dispatchEvent(new w.Event("change",{bubbles:true})); };
const scrivi=(el,v,nome)=>{ if(!el){ko++;console.log("✗ manca il campo: "+(nome||""));return;} el.value=v; el.dispatchEvent(new w.Event("input",{bubbles:true})); };
const SALD_T=["sald_mag","sald_robot","sald_tig"];
const paAv=()=>{const a=q("#pa-avviso")||q("#pa-avviso-cas");return a?{esito:a.getAttribute("data-esito"),testo:a.textContent}:{esito:null,testo:""};};
const fotoA=()=>ev("JSON.stringify((t=>[t.prodotti.map(p=>[p.fonte,p.nodi.map(n=>[n.nodo.rif,n.tipo,n.classificazione,n.nodo.decisione,n.parentela]),p.bom]),t.componenti,t.relazioni,t.documenti,t.testa])(threadAperto()))");
const cidDi=s=>ev(`(righeB(threadAperto(),prodottoScelto(threadAperto())).find(r=>r.cod.endsWith(${JSON.stringify(s)}))||{}).cid`);
const ciclo=s=>JSON.parse(ev(`JSON.stringify(cicloB(threadAperto(),${JSON.stringify(cidDi(s))})||null)`));
const S2=e=>ev(`(x=>${e})(a2Di(threadAperto()))`);
const ruolo=r=>{click('[data-menu="app"]');click('[data-menu="ruolo"]');click(`[data-ruolo="${r}"]`);};
const ROTTE_A=ROTTE;
function B(sez){ /* invarianti della zona B */
  t(`B3 [${sez}] le rotte B non sono nella lista A`,ev("REGISTRO.filter(x=>x.zona==='B').map(x=>x.url)").every(u=>!ROTTE_A.some(r=>r.test(u))));
  t(`B3 [${sez}] ogni gesto B usa data-pa-gesto (mai data-ma-gesto)`,qa("[data-pa-gesto]").every(e=>!e.hasAttribute("data-ma-gesto")));
  t(`B7 [${sez}] nessun KO da un dato inventato`,ev("(()=>{const T=threadAperto(),p=prodottoScelto(T);return !p||controlliB(T,p).every(x=>!(x.inventato&&x.liv==='KO'&&esitoProposto(T,p,x.cid).v==='KO'));})()"));
}

ev('S.ruolo="Lino"');
apriRFQ("482-00");
const A0=fotoA(), nB0=ev("REGISTRO.filter(x=>x.zona==='B').length");
click('.staz[data-staz="2"]');
t("T-B Come: la fascia «Proposta per dopo A2» con R-22 (v16: nel title)",!!q(".staz-b .banda-b")&&q(".staz-b .banda-b").getAttribute("title").includes("R-22"));
Z2("Come");
t("Come: 8 righe da smistare, nessuna selezionata",qa(".come-tab tbody tr[data-come-cid]").length===8&&!qa(".come-riga--sel").length);
t("Come: proposte ◇ visibili, nessuna decisa al posto di Lino",qa(".come-tab .seg--prop").length===8&&!qa('.come-tab .seg button[aria-pressed="true"]').length);
t("Come: il tipo si legge dalla zona A (v16: detto una volta nell'intestazione)",q(".come-tab th.c-tipo").textContent.includes("dalla zona A")&&qa(".come-tab td.c-tipo .fonte").length===0);
t("Come: aprire non scrive niente",ev("REGISTRO.filter(x=>x.zona==='B').length")===nB0);
tasto("Enter");
t("Invio: .000 diventa «noi» con il modello T2 in bozza",paAv().esito==="ok"&&ciclo(".000").modello==="T2"&&ciclo(".000").stato==="bozza");
t("Invio: la riga lascia il filtro e il cursore scende a .001",qa(".come-tab tbody tr[data-come-cid]").length===7&&q(".come-riga--cur").textContent.includes(".001"));
tasto("3");
t("tasto 3: .001 fuori (travi da terzi T4b)",S2(`x.come[${JSON.stringify(cidDi(".001"))}].modo`)==="fuori"&&ciclo(".001").modello==="T4b");
for(let i=0;i<6;i++) tasto("Enter");
t("smistati 8/8, contatore nella barra letto dal server A2",q('.staz[data-staz="2"] .staz-c').textContent==="8/8");
t("⧉ .009 copiato da .008 con i driver svuotati",ciclo(".009").copiaDa==="F+800.514263.008"&&!ciclo(".009").fasi[0].par.m&&!ciclo(".009").fasi[0].prop.m);
t("dal cliente: POS.2 e POS.3 senza ciclo (T7)",S2(`x.come[${JSON.stringify(cidDi("POS.2"))}].modo`)==="cliente"&&!ciclo("POS.2"));
click('[data-come-filtro="tutti"]');
t("il commerciale non ha tasti: lo compra Fabio",byText(".come-tab tr","ISO 4032 M14")&&!byText(".come-tab tr","ISO 4032 M14").querySelector(".seg")&&byText(".come-tab tr","ISO 4032 M14").textContent.includes("lo compra Fabio"));
t("B1 Come: l'albero, i tipi e i documenti non cambiano",fotoA()===A0);
t("primario: Apri le richieste da preparare ✉ (4)",q(".b-piede .ma-btn--pri").textContent.includes("Apri le richieste da preparare ✉ (4)"));
const nRq0=S2("x.richieste.length");
click(".b-piede .ma-btn--pri");
t("P-02: il cassetto si apre sulle carte dei bisogni (4); il clic non crea richieste",ev("S.cassetto")==="richieste"&&qa("#cassetto .rq").length===4&&S2("x.richieste.length")===nRq0);
Z2("cassetto Richieste");
t("nessuna richiesta parte da sola, nessun «Manda» nel cassetto",S2("x.richieste.length")===0&&!qa("#cassetto button").some(b=>/^Manda/.test(b.textContent.trim())));
B("Come");

/* ================= T-R · le richieste (v16 rev., P-02): le righe di Pietro, una per fornitore ================= */
const carta=k=>q(`#cassetto .rq[data-rq="${k}"]`);
const rigaF=(k,nome)=>{const c=carta(k);return c?[...c.querySelectorAll(".rq-forn")].find(x=>x.textContent.includes(nome)):null;};
const btnIn=(el,txt)=>el?[...el.querySelectorAll("button")].find(b=>b.textContent.includes(txt)):null;
const rqDi=fid=>S2(`(x.richieste.find(q=>q.forn&&q.forn.id===${JSON.stringify(fid)})||{}).id`);
const rqTipo=tipo=>S2(`(x.richieste.find(q=>q.tipo===${JSON.stringify(tipo)})||{}).id`);
const statoRq=id=>S2(`(x.richieste.find(q=>q.id===${JSON.stringify(id)})||{}).stato`);
const rigaR=id=>q(`#cassetto [data-rq-riga="${id}"]`);
const apriCarta=k=>ev(`S.cassetto="richieste";S.rqAperta=${JSON.stringify(k)};render()`);
const prepara=(k,nome)=>{apriCarta(k);click(btnIn(rigaF(k,nome),"Prepara la bozza"),"Prepara la bozza "+nome);};
const simula=(k,id,txt)=>{apriCarta(k);click(btnIn(rigaR(id),txt),txt+" "+id);};
const registraR=(k,id)=>{apriCarta(k);click(rigaR(id)&&rigaR(id).querySelector('[data-pa-gesto$="/risposta"]'),"Registra "+id);};
// richiesta al terzista (RTC, travi)
let k="terzista|travi_est";apriCarta(k);
t("T-R terzista: RTC con la sua descrizione accanto al nome (M7) e «Prepara la bozza»",!!rigaF(k,"RTC")&&rigaF(k,"RTC").textContent.includes("taglio e lavorazione di travi")&&!!btnIn(rigaF(k,"RTC"),"Prepara la bozza"));
prepara(k,"RTC");
let id=rqDi("rtc");
t("T-R terzista: una riga per fornitore sulla rotta di Pietro, stato «bozza»",!!id&&statoRq(id)==="bozza"&&ev(`REGISTRO.some(x=>x.zona==="B"&&x.url.endsWith("/thread/"+threadAperto().id+"/richiesta")&&x.campi.fornitore_id==="rtc"&&x.campi.bozza==="1")`));
t("T-R terzista: lo stesso fornitore e la stessa lavorazione → la frase di Pietro",ev(`SERVER_B.posta(threadAperto(),prodottoScelto(threadAperto()),"/thread/"+threadAperto().id+"/richiesta",campiDa({fornitore_id:"rtc",lavorazione:"travi_est",codice:"X",note:""})).testo`).includes("esiste già una richiesta a RTC Travi per questa RFQ e questa lavorazione"));
simula(k,id,"Simula: la mail parte");
t("T-R terzista: «inviata» la mette il sync (simulato)",statoRq(id)==="inviata");
simula(k,id,"Simula la risposta");
t("T-R terzista: offerta ricevuta (stato di Pietro, 0016)",statoRq(id)==="offerta_ricevuta"&&carta(k).textContent.includes("600"));
registraR(k,id);
t("T-R terzista: registrata nella fase 10 di .001 (RISP); la richiesta resta «offerta ricevuta»",statoRq(id)==="offerta_ricevuta"&&ciclo(".001").fasi[0].risp.prezzo===600&&ev("QUOTAZIONI_B.length")===1&&carta(k).textContent.includes("registrata"));
// chiedi prezzo e tempi (UPN, Fornitore MP 2)
k="mp_prezzo";prepara(k,"Fornitore MP 2");id=rqDi("mp2");
simula(k,id,"Simula: la mail parte");simula(k,id,"Simula la risposta");registraR(k,id);
t("T-R prezzo e tempi: 0,83 €/kg registrato sul grezzo (RISP)",S2("x.prezziMP['g-upn160'].eur")===0.83);
// chiedi ai fornitori di materia prima (tubo mai acquistato): MP1 e MP3, due righe
k="mp_mai";prepara(k,"Fornitore MP 1");prepara(k,"Fornitore MP 3");
const id1=rqDi("mp1"),id3=rqDi("mp3");
t("T-R materia prima: due richieste, una per fornitore",!!id1&&!!id3&&id1!==id3);
simula(k,id1,"Simula: la mail parte");simula(k,id3,"Simula: la mail parte");
t("T-R MAT-03 mentre aspetta: DUBBIO in Esito",ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==='MAT-03')`));
simula(k,id1,"Simula la risposta");simula(k,id3,"Simula la risposta");
t("T-R MAT-05: materiale dichiarato ≠ disegno (S235JRH su S355J2)",rigaR(id3).textContent.includes("MAT-05"));
registraR(k,id3);
t("T-R MAT-05: la risposta al ribasso non si registra",paAv().esito==="no"&&!S2("x.prezziMP['g-s355-127']"));
registraR(k,id1);
t("T-R materia prima: 1,24 €/kg di Fornitore MP 1 registrato",S2("x.prezziMP['g-s355-127'].eur")===1.24&&statoRq(id1)==="offerta_ricevuta");
// lamiera comprata (Lamitaglio; Fornitore MP 4 declina)
k="lamiera";prepara(k,"Lamitaglio");prepara(k,"Fornitore MP 4");
const idF=rqDi("lamitaglio"),id4=S2(`(x.richieste.find(q=>q.forn&&q.forn.id==="mp4"&&q.lavorazione==="laser_est")||{}).id`);
for(const x of [idF,id4]){simula(k,x,"Simula: la mail parte");simula(k,x,"Simula la risposta");}
t("T-R lamiera: una declinata (stato di Pietro), una offerta",statoRq(id4)==="declinata"&&carta(k).textContent.includes("declina")&&carta(k).textContent.includes("410"));
registraR(k,idF);
t("T-R lamiera: registrata su .006 f.10 (e il trasporto)",ciclo(".006").fasi[0].risp.prezzo===410&&ciclo(".006").fasi[1].risp.prezzo===250);
// Annulla: la rotta di Pietro; lo stato «annullata» è suo
apriCarta(k);click(btnIn(rigaR(id4),"Nuovo giro"),"Nuovo giro");
t("T-R nuovo giro (proposta R-17): la riga torna in bozza, il giro resta",statoRq(id4)==="bozza"&&S2(`x.richieste.find(q=>q.id==="${id4}").giri.length`)===1);
apriCarta(k);click(btnIn(rigaR(id4),"Annulla"),"Annulla");
t("T-R Annulla: «annullata» (stato di Pietro) con la sua frase",statoRq(id4)==="annullata"&&paAv().testo.includes("Richiesta annullata"));
t("T-R stati: solo quelli di Pietro per i fornitori",S2(`x.richieste.filter(q=>q.forn).every(q=>["bozza","inviata","offerta_ricevuta","declinata","scaduta","annullata"].includes(q.stato))`));
t("B1 richieste: la zona A non cambia",fotoA()===A0);
B("Richieste");

/* ================= T-B · Ciclo VFR ================= */
click('[data-cassetto=""]');
click('.staz[data-staz="3"]');
Z2("Ciclo");
t("Ciclo: marchio B con R-10 (v16: nel title)",q(".ciclo .banda-b").getAttribute("title").includes("R-10"));
t("Ciclo: elenco 6 da fare, si apre la .000",qa(".cl-elenco .cl-riga[data-ciclo-cid]").length>=6&&q(".cl-riga--su").textContent.includes(".000"));
t("Ciclo: una sola fase aperta, la prima da completare",qa(".fase--aperta").length===1);
t("C2: la barra di conferma è l'ultima riga della colonna, un solo primario",q(".cl-foglio").lastElementChild.classList.contains("barra-conf")&&qa(".cl-foglio .barra-conf .ma-btn--pri").length===1);
t("T-V: un solo primario nella stazione",qa(".ws-main .ma-btn--pri").length===1);
t("M11: la stessa testataPezzo nel Ciclo",!!q(".cl-foglio .tp .tp-r3"));
t("riga «requisiti letti» con REQ-01 sull'ISO 13920 (v16: un chip «1 senza fase», dec. 1 nel title)",q(".req-riga").textContent.includes("1 senza fase")&&q(".req-tit").title.includes("Decisione 1"));
// la saldatura della .000 (v16: prima la saldatura del telaio, poi i dadi saldati sul telaio)
const fS=ciclo(".000").fasi.find(f=>f.p==="sald_mag").id, fSi=()=>ciclo(".000").fasi.find(f=>f.id===fS);
if(ev("S.faseAperta")!==fS) click(`[data-fase-apri="${fS}"]`);
t("M3: «Si salda» si legge da «Entrano» (niente domanda da scrivere) e gli 8 figli del padre sono nella lista",q(".fase--aperta .sald-q").textContent.includes("Si salda")&&!q(".fase--aperta").textContent.includes("Che cosa si salda")&&q(".fase--aperta .sald-entra").textContent.includes("M14"));
t("M3: «puntato prima» non c'è se nella fase non entra un commerciale",!q(`.fase--aperta [data-pa-spunta="${fS}|puntato|compreso"]`));
cambia(q(`.fase--aperta [data-pa-spunta="${fS}|maschera|sì"]`),true,"in maschera");
t("in maschera → la preparazione «maschera di saldatura» si accende",ciclo(".000").prep.maschera.on===true&&ciclo(".000").prep.maschera.nata===fS);
click(`[data-fase-menu="${fS}"]`,"⋯ della fase");click(`[data-giunto-apri="${fS}"]`,"+ giunto nel ⋯");
scrivi(q("#giunto-in"),"z5 51-152 L1016","giunto");
t("giunto: anteprima convertiCordone «7 tratti · 357 mm»",q(".giunto-ant").textContent.includes("7 tratti")&&q(".giunto-ant").textContent.includes("357"));
const nG=fSi().giunti; submit(q(`form[data-pa-gesto="/fase/${fS}/giunto"]`),"giunto");
t("giunto aggiunto, una scrittura della fase",fSi().giunti.length===1&&!nG);
t("circ Ø127 ×4 → 1.596 mm",ev(`convertiCordone("circ Ø127 ×4").mm`)>1595&&ev(`convertiCordone("circ Ø127 ×4").mm`)<1596);
const tIn=q(`[data-pa-campo="${fS}|tempo"]`);
t("tempo di saldatura NON proposto: campo vuoto, con preventivo e consuntivo come riferimento",tIn&&!tIn.classList.contains("campo--prop")&&tIn.value===""&&q(".fase--aperta").textContent.includes("consuntivo 26 h"));
tIn.value="20 h";tasto("Enter",{},tIn);
t("Lino scrive 20 h (un tempo a corpo): il tempo diventa dato (fonte L)",fSi().par.tempo==="20 h"&&fSi().fonti.tempo==="L");
t("tempo: 20 h = 1.200 min/pz; 25 pz/h = 2,4 min/pz [FE]",ev(`leggiTempo("20 h").testo`).includes("1.200")&&ev(`leggiTempo("25 pz/h").testo`).includes("2,4"));
t("WPS ⚑: copertura e solo il conteggio dei saldatori",q(".fase--aperta").textContent.includes("WPS 135-020 copre")&&q(".fase--aperta").textContent.includes("saldatori qualificati"));
// REQ-01: il controllo secondo ISO 13920
const fC=ciclo(".000").fasi[2].id; click(`[data-fase-apri="${fC}"]`);
click(byText(".fase--aperta .seg button","ISO 13920 BE"),"ISO 13920");
t("REQ-01 risolto: ISO 13920 → .000 f.30",!q(".req-riga").textContent.includes("nessuna fase"));
tasto("Enter",{ctrlKey:true});
t("⌃↵: accetta le proposte e conferma la .000, poi va alla .001",ciclo(".000").stato==="confermato"&&q(".cl-riga--su").textContent.includes(".001"));
tasto("Enter",{ctrlKey:true});
t("⌃↵: .001 confermata (travi da terzi, RISP RTC)",ciclo(".001").stato==="confermato"&&q(".cl-riga--su").textContent.includes(".002"));
t(".002: grezzo «mai acquistato» con il prezzo RISP",q(".sg--mai")&&q(".sg--mai").textContent.includes("1,24"));
t(".002: anteprima pezzi per barra (sega)",q(".cl-foglio").textContent.includes("pz per barra"));
// la ricerca del grezzo nella banca dati
const gIn=q(`[data-pa-grezzo]`);
cambia(gIn,"355 tubo 127x10","grezzo");
t("ricerca «355 tubo 127x10» → S355J2 tubo Ø127 × 10",ev(`testoGrezzo(grezzoVal(cicloB(threadAperto(),${JSON.stringify(cidDi(".002"))})))`)==="S355J2 · tubo Ø127 × 10");
tasto("Enter",{ctrlKey:true});
t(".002 confermata",ciclo(".002").stato==="confermato");
tasto("Enter",{ctrlKey:true});
t(".006 confermata (lamiera comprata, Lamitaglio RISP)",ciclo(".006").stato==="confermato"&&q(".cl-riga--su").textContent.includes(".008"));
/* v15 · correzioni dopo la prova di Lino */
t("sfrido .001: il netto 1.096 kg è 1096 (non 1,096), sfrido ~32 kg, PES-01 non scatta",ev(`(()=>{const T=threadAperto(),p=prodottoScelto(T),r=righeB(T,p).find(x=>x.cod.endsWith(".001"));const a=pesoAnteprima(T,r);return !!a&&a.sfrido>20&&a.sfrido<60&&!a.pes01;})()`));
t("LOT-02: nessun falso allarme sul lotto (1 telaio = 2 pz di .006, confronto in pezzi)",ev("a2Di(threadAperto()).richieste.filter(q=>q.risp).flatMap(q=>controlliRisposta(threadAperto(),prodottoScelto(threadAperto()),q)).filter(x=>x.cod==='LOT-02').length")===0);
t("richiesta: il lotto prodotto si legge in pezzi del componente",ev("a2Di(threadAperto()).richieste.filter(q=>q.codici.some(c=>c.cod.endsWith('.006'))).some(q=>testoPezzi(q).includes('2 pz'))"));
t("terzisti: mai accettati in blocco (nessun preselezionato)",ev(`propBlocco({prop:{terzi:["rtc","STO"]},par:{}},"terzi")`)===null&&ev(`statoFase(threadAperto(),prodottoScelto(threadAperto()),null,{p:"tornitura",prop:{terzi:["f2","STO"]},par:{}}).proposte`)===0);
ev(`PA.avviso={esito:"ok",testo:"prova"}`);tasto("g");tasto("4");
t("G + numero: cambia stazione e l'avviso di prima non segue Lino",ev("S.staz")===4&&ev("PA.avviso")===null&&!q("#pa-avviso"));
tasto("g");tasto("3");
// .008: Invio sul grezzo, Maiusc+Invio sulla fase
const g8=q(`[data-pa-grezzo]`);
t(".008: grezzo proposto ◇ dal cartiglio",g8.classList.contains("campo--prop"));
tasto("Enter",{},g8);
t(".008: Invio accetta il grezzo · In magazzino con il 3.1",ev(`grezzoAccettato(cicloB(threadAperto(),${JSON.stringify(cidDi(".008"))}))`)&&q(".sg--magazzino")&&q(".sg--magazzino").textContent.includes("3.1"));
t(".008: lavorabilità laser ✓ (dati inventati, segnati)",q(".lav").textContent.includes("laser ✓")&&q(".lav").textContent.includes("inventato"));
tasto("Enter",{shiftKey:true});
t(".008: Maiusc+Invio accetta le proposte della fase 10",Object.keys(ciclo(".008").fasi[0].par).includes("m")&&ciclo(".008").fasi[0].par.fori==="8");
t("A10: netto ≤ lordo e lo sfrido è un numero",!q(".cl-foglio").textContent.includes("Sfrido —")&&q(".cl-foglio").textContent.includes("sfrido 0,24 kg"));
tasto("Enter",{ctrlKey:true});
t(".008 confermata, si apre la .009",ciclo(".008").stato==="confermato"&&q(".cl-riga--su").textContent.includes(".009"));
tasto("Enter",{ctrlKey:true});
t(".009 copiata: senza i driver la conferma dice no",paAv().esito==="no"&&paAv().testo.includes("mancano dati ★")&&ciclo(".009").stato==="bozza");
const f9=ciclo(".009").fasi;
cambia(q(`[data-pa-campo="${f9[0].id}|m"]`),"=0,55*2","metri");
t("espressione «=0,55*2» → 1,1 e la formula resta nel title",ciclo(".009").fasi[0].par.m==="1,1"&&ciclo(".009").fasi[0].par.f_m==="=0,55*2"&&q(`[data-pa-campo="${f9[0].id}|m"]`).closest("label").title.includes("=0,55*2"));
cambia(q(`[data-pa-campo="${f9[0].id}|fori"]`),"3","fori");
click(`[data-fase-apri="${f9[1].id}"]`);cambia(q(`[data-pa-campo="${f9[1].id}|n"]`),"3","filetti");
tasto("Enter",{ctrlKey:true});
t(".009 confermata: cicli 6/6",ciclo(".009").stato==="confermato"&&q('.staz[data-staz="3"] .staz-c').textContent==="6/6");
t("B1 Ciclo: la zona A non cambia",fotoA()===A0);
B("Ciclo");
// un nodo senza componente: foglio in sola lettura (RV-27)
apriRFQ("990020338");
click('[data-menu="app"]');click('.tendina--app [data-tab="inbox"]');click('[data-cli="vignabot"]');click('.rfqblock [data-apririfq]');
click('.staz[data-staz="3"]');
t("RV-27: senza componente il foglio è in sola lettura, con il motivo",q(".cl-foglio").textContent.includes("Sola lettura")&&!q(".cl-foglio [data-pa-gesto]"));
click('.staz[data-staz="2"]');
t("RV-27: in Come i nodi senza componente sono grigi, con il motivo",qa(".come-riga--grigia").length>0&&q(".come-riga--grigia").textContent.includes("autorizza lo STEP")&&q(".come-riga--grigia").textContent.includes("R-38"));
const nRB=ev("REGISTRO.filter(x=>x.zona==='B').length");
t("RV-27: il server B rifiuta il ciclo su un nodo non confermato",ev(`eseguiB("/componente/00000000-0000-4000-a000-000000000000/fornitura",{modo:"noi"}).esito`)==="no");

/* ================= T-B · Sequenza ================= */
apriRFQ("482-00");
const A1=fotoA();
click('.staz[data-staz="4"]');
/* v16 (SPEC §4): la Sequenza è l'albero del processo; ogni gesto è una scrittura (niente bozza nella pagina) */
t("Sequenza: l'albero del processo, due unioni (saldatura, dadi saldati), nessun nodo scelto",!!q('.pp-albero[role="tree"]')&&qa(".pp-unione").length===2&&!q('.pp-albero [aria-selected="true"]'));
t("Sequenza: il riferimento è il più pesante (UPN .001, ◇ CALC)",byText(".pp-unione","Saldatura").textContent.includes("rif .001 ◇"));
const nR3=ev("REGISTRO.length");
click(q(`.pp-box[data-pp-cid="${cidDi("M14")}"]`));
t("Sequenza: il dado è proposto nei dadi saldati (SEQ-06), aprire non scrive",q(".seq16-pannello").textContent.includes("SEQ-06")&&ev("REGISTRO.length")===nR3);
tasto("1");
t("tasto 1: il dado entra nella saldatura del telaio (una scrittura)",ev("REGISTRO.length")===nR3+1&&ciclo(".000").fasi.find(f=>f.p==="sald_mag").entrano.some(e=>e.cid===cidDi("M14"))&&S2("x.seq.stato")!=="confermata");
tasto("2");
t("tasto 2: torna nei dadi saldati",ciclo(".000").fasi.find(f=>f.p==="prigionieri").entrano.some(e=>e.cid===cidDi("M14")));
tasto("m");t("M senza trattamento nel padre: no, niente cambia",ev("PA.avviso.esito")==="no");
click(q(".pp-box--prodotto"));
const cod=q("#r31-codice");
t("C5: codice dell'assieme vuoto, nessun segnaposto con cifre, gesto spento",cod.value===""&&!/\d/.test(cod.placeholder||"")&&q("#r31-btn").disabled);
scrivi(cod,"F+800.514263.008-A","codice");
t("R-31: con il codice scritto da una persona il gesto si accende",!q("#r31-btn").disabled);
q("#r31-cid").value=cidDi(".008");click("#r31-btn");
t("R-31: «proposta registrata: l'albero non cambia»",paAv().esito==="ok"&&paAv().testo.includes("l'albero non cambia")&&fotoA()===A1);
click('[data-cat-apri="componenti|r38"]');
t("M7: Cataloghi si apre su Componenti con la destinazione",q('#cassetto [data-cat-tab="componenti"][aria-pressed="true"]')&&q("#cassetto .cat-dest").textContent.includes("R-38"));
t("A6: nessun pulsante della riga di catalogo contiene un codice intero ripetuto",qa("#cassetto .cat-r").every(b=>!b.querySelector("button")));
click(q("#cassetto [data-cat-usa]"));
t("R-38: il componente scelto nel modulo, niente scritto",q("#r38-descr").value.length>3);
q("#r38-padre").value="F+800.514263.000";click(q('#prop-r38 [data-pa-gesto]'));
t("R-38: proposta registrata, l'albero non cambia",paAv().testo.includes("l'albero non cambia")&&fotoA()===A1&&S2("x.proposteAlbero.length")===2);
tasto("Enter",{ctrlKey:true});
t("⌃↵: sequenza confermata con una scrittura (v16: con la firma), per il prodotto (A2)",ev("seqDi(threadAperto(),prodottoScelto(threadAperto())).stato")==="confermata"&&ev("REGISTRO[0].url").endsWith("/sequenza/conferma")&&!!ev("REGISTRO[0].campi.firma"));
B("Sequenza");

/* ================= T-B · Esito e consegna ================= */
// Lino conferma i documenti (zona A, voce per voce, con la domanda): poi l'Esito propone gli OK
click('.staz[data-staz="1"]');click('[data-vista="documenti"]');
let giri=0; while(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]') && giri++<12){ click(q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]')); si(); }
click('[data-ma-azione="aggiorna"]');
t("documenti confermati voce per voce (zona A)",!q('tr.ma-voce [data-ma-gesto$="/fascicolo/conferma"]'));
const lCli=(byText("tr.ma-voce","ISO 4032 M14")||{querySelector:()=>null}).querySelector('[data-cassetto-apri="richieste"]');
t("Documenti (zona A): «Chiedi al cliente» è solo un collegamento, nessun gesto B nella regione R9",!!lCli&&!q('tr.ma-voce [data-pa-gesto]'));
click(lCli);
const bCli=q(`#cassetto [data-rq-cliente="${cidDi("M14")}"]`);
t("cassetto Richieste: «2D da chiedere al cliente» con il dado (R-41)",!!bCli&&q("#cassetto").textContent.includes("ISO 4032 M14"));
click(bCli);
const idCli=rqTipo("cliente");
t("T-R cliente (proposta R-41): bozza in Outlook, nessun «Manda»",statoRq(idCli)==="bozza"&&!qa("#cassetto button").some(b=>/^Manda/.test(b.textContent.trim())));
simula(idCli,idCli,"Simula: la mail parte");
t("T-R cliente: inviata quando il sync la vede (VFR: ordine diretto, nessuna conversazione nell'Inbox)",statoRq(idCli)==="inviata");
apriCarta(idCli);click(btnIn(rigaR(idCli),"Annulla"),"Annulla");
t("T-R cliente: annullata, con la frase di Pietro",statoRq(idCli)==="annullata"&&paAv().testo.includes("Richiesta annullata"));
click('[data-cassetto=""]');
const A3=fotoA();
click('.staz[data-staz="5"]');
Z2("Esito");
t("Esito: una riga per componente con una sola colonna «esito»",qa(".esito-tab tbody tr").length===9&&qa(".esito-tab thead th").filter(x=>/esito/i.test(x.textContent)).length===1);
t("Esito: le proposte ◇ dal server, nessuna decisa",qa(".esito-tab .seg--prop").length===9&&!qa('.esito-tab .seg button[aria-pressed="true"]').length);
const nOK=qa('.esito-tab .seg--prop[data-v="OK"]').length;
t("Esito: ci sono proposte OK dopo la conferma dei documenti",nOK>=3);
click("[data-esito-anteprima]");
t("«Accetta le OK»: prima l'elenco delle righe",q(".blocco-b").textContent.includes(`Accetterai ${nOK} proposte OK`));
click(q(".blocco-b [data-pa-gesto]"));
t("OK registrati uno per riga (R-35)",qa('.esito-tab .seg button[aria-pressed="true"][data-v="OK"]').length===nOK);
click(q('[data-esito-riga="'+cidDi("M14")+'"]'));tasto("b");
t("tasto B: DUBBIO sulla riga del dado",S2(`x.esiti[${JSON.stringify(cidDi("M14"))}].v`)==="DUBBIO");
cambia(q(`[data-esito-nota="${cidDi("M14")}"]`),"dado ISO 4032 a norma: 2D chiesto al cliente","nota");
t("la nota non cambia l'esito",S2(`x.esiti[${JSON.stringify(cidDi("M14"))}].v`)==="DUBBIO"&&S2(`x.esiti[${JSON.stringify(cidDi("M14"))}].nota`).includes("2D chiesto"));
const pv=q("[data-presa-visione]");
t("M-8: «Presa visione delle 2» in testa al riquadro (un gesto, una scrittura per condizione sul server)",!!pv&&pv.textContent.includes("delle 2")&&!byText(".req-l div","Penale").querySelector("[data-pa-gesto]"));
click(pv);
t("ORD-01: presa visione della penale e delle verifiche",S2("!!x.visti.rq4")&&S2("!!x.visti.rq6"));
click(byText(".req-l div","Accettazione tacita").querySelector('[data-pa-gesto$="/domanda"]'));
const idCom=rqTipo("commerciale");
t("«Avvisa il commerciale» (proposta R-41): bozza in Outlook, la riga dice che attende",statoRq(idCom)==="bozza"&&byText(".req-l div","Accettazione tacita").textContent.includes("attende il commerciale"));
simula(idCom,idCom,"Simula: la mail parte");simula(idCom,idCom,"Simula la risposta");registraR(idCom,idCom);
t("T-R «Avvisa il commerciale»: la condizione è vista; la richiesta non diventa «registrata»",S2("!!x.visti.rq5")&&statoRq(idCom)==="risposta");
t("P-07: minuteria spenta anche in Esito, con il motivo R-08 (nessun gesto B)",!q("[data-minuteria]")&&!ev("ROTTE_B.some(x=>x[0]==='minuteria')")&&q(".esito-tab").textContent.includes("R-08"));
click('[data-cassetto=""]');
let eseguiDiretto;const cons=()=>q("[data-consegna]"),spento=b=>b.disabled||b.getAttribute("aria-disabled")==="true";
t("consegna spenta finché nessun lotto è spuntato",spento(cons()));
cambia(q("[data-lotto]"),true,"lotto 1");
t("Esito: lo stato della zona A si legge in testa (sola lettura)",!!q(".zona-a-ro"));
if(q("[data-presa-atto]")){
  t("zona A non pronta: consegna spenta finché non si prende atto (DB7-2, EB7-3)",spento(cons()));
  const prima=S2("x.consegne.length");eseguiDiretto=ev(`(SERVER_B.posta(threadAperto(),prodottoScelto(threadAperto()),"/prodotto/"+${JSON.stringify(cidDi("000"))}+"/consegna",new Map([["lotti","1"]]))||{}).esito`);
  t("finto server B: consegna senza presa d'atto su prodotto non pronto → no",eseguiDiretto==="no"&&S2("x.consegne.length")===prima);
  cambia(q("[data-presa-atto]"),true,"presa d'atto");
}
t("lotto spuntato: consegna accesa",!spento(cons()));
click(cons());
t("consegna senza firma → no",paAv().esito==="no"&&paAv().testo.includes("firma"));
ruolo("Fabio");
t("v16: con il ruolo Fabio l'area tecnica si legge soltanto («attende Lino»)",!byText(".firma-b","Area tecnica").querySelector("[data-pa-gesto]")&&byText(".firma-b","Area tecnica").textContent.includes("attende Lino")&&!!byText(".firma-b","Acquisti").querySelector("[data-pa-gesto]"));
t("la firma dell'area tecnica la mette Lino (il finto server dice no)",ev(`SERVER_B.posta(threadAperto(),prodottoScelto(threadAperto()),"/prodotto/"+${JSON.stringify(cidDi("000"))}+"/firma",new Map([["area","tec"],["v","positiva"]])).esito`)==="no");
ruolo("Lino");click(byText(".firma-b","Area tecnica").querySelector("[data-pa-gesto]"));
t("M3: firmato da Lino, segmento e non un campo",q(".firme-b").textContent.includes("Firmato da Lino"));
const faseSVI=ev("threadAperto().testa.fase.nome");
cambia(q("[data-lotto]"),true,"lotto 1");click(cons());
t("Consegna a Franco: fotografia per il lotto 1",paAv().esito==="ok"&&S2("x.consegne.length")===1&&S2("x.consegne[0].lotti[0]")===1);
t("la fotografia distingue gli esiti decisi da quelli proposti (R-35)",S2("x.consegne[0].esiti.some(e=>e.deciso===false)")&&S2("x.consegne[0].esiti.some(e=>e.deciso===true)"));
t("la fotografia scrive lo stato della zona A",S2("!!x.consegne[0].zonaA"));
t("RV-29: la consegna non cambia la fase registrata",ev("threadAperto().testa.fase.nome")===faseSVI);
t("B1 Esito: albero, fonte e documenti invariati",fotoA()===A3);
B("Esito");

/* ================= T-O · Foglio per Franco e Offerta ================= */
ruolo("Franco");
t("ruolo Franco: si apre il Foglio per Franco",q('[data-esito-vista="foglio"][aria-current="page"]')&&q(".foglio-v").textContent.includes("Foglio per Franco"));
t("Foglio: blocchi del Mod 99, sola lettura, nessun € nei dati di Lino",qa(".fr-blocco").length>=4&&!q(".fr-tab").textContent.includes("€/kg"));
click('[data-esito-vista="costi"]');
t("Costi: totale VFR 3.710,04 € e il confronto coincide",q("[data-totale]").textContent.trim()==="3.710,04 €"&&q("[data-confronto]").textContent.includes("coincide"));
t("Costi: ogni euro ha la formula e la fonte",qa("table.costi td.drv").length>15&&qa("table.costi .chip").length>10);
click('[data-esito-vista="listini"]');
q("#nq-terz").value="polvernova";q("#nq-cod").value="F+800.514263.000";q("#nq-p").value="verniciatura_polvere";q("#nq-lotto").value="1";q("#nq-eur").value="52";click('[data-of="qadd"]');
t("Listini: quotazione aggiunta",ev("QUOTAZIONI.some(x=>x.terz==='polvernova'&&x.cod==='F+800.514263.000')"));
t("Listini: la quotazione passa dal finto server B (registro B)",ev("REGISTRO.some(x=>x.zona==='B'&&/\\/offerta\\/qadd$/.test(x.url)&&x.esito&&x.esito.esito==='ok')"));
click('[data-esito-vista="offerta"]');
t("Offerta: anteprima su carta e «DA» leggibile (B4)",q(".so-carta").textContent.includes("Ufficio commerciale"));
click('[data-of="accettaord"]');
t("Offerta VFR (ordine diretto): accettato nel registro, la fase non cambia (n. 18)",ev("offertaDi(threadAperto()).stato")==="accettata"&&ev("threadAperto().testa.fase.nome")===faseSVI&&q(".off-v").textContent.includes("la fase cambia solo con un gesto registrato"));
ruolo("Lino");
/* §2.9 · che cosa torna indietro: un lotto nuovo (non spuntato, la fotografia vecchia resta) e la revisione precedente (ECO) */
click('[data-esito-vista="esito"]');
q("#lotto-nuovo").value="5";click('[data-pa-gesto$="/lotto"]');
t("§2.9 lotto nuovo aggiunto e non spuntato",!!q('[data-lotto="5"]')&&!q('[data-lotto="5"]').checked&&S2("x.consegne[0].lotti.join()")==="1");
apriRFQ("P-2026-0131");click('.staz[data-staz="2"]');tasto("Enter");click('.staz[data-staz="3"]');
tasto("c");
t("§2.9 ECO: «Copia da…» propone la revisione precedente",!!byText(".cl-foglio button","rev. precedente B"));
click(byText(".cl-foglio button","rev. precedente B"));
t("§2.9 ECO: ciclo copiato dalla rev. B in bozza, con il delta in testata",ciclo("92176C").copiaDa==="rev. B"&&ciclo("92176C").stato==="bozza"&&q(".cl-foglio-testa").textContent.includes("delta ECO"));
apriRFQ("482-00");
Z("passata 2 · VFR");

/* ================= FitLab: escluso, no quote, blocco, verniciatori, giri, officina, attrezzatura, qualità, domanda ================= */
apriRFQ("RDO 2026/318");
const A2tg=fotoA();
click('.staz[data-staz="2"]');
const rTG=qa(".come-tab tbody tr[data-come-cid]");
t("FitLab: una riga per codice distinto (il perno in 2 assiemi)",q(".come-tab").textContent.includes("in 2 assiemi"));
click(byText(".come-tab tr[data-come-cid]","9N008520AA"));tasto("6");
t("tasto 6: il motivo del no quote, con il fuoco nel campo",!!q("#come-form-testo"));
q("#come-form-testo").value="problema di stampaggio";submit(q(".come-riga-form form"),"no quote");
t("no quote registrato: KO in Esito",S2(`x.esiti[${JSON.stringify(cidDi("9N008520AA"))}].v`)==="KO");
click('[data-come-filtro="tutti"]');click(byText(".come-tab tr[data-come-cid]","9N008520AA"));tasto("1");
t("tasto 1: torna «noi»",S2(`x.come[${JSON.stringify(cidDi("9N008520AA"))}].modo`)==="noi");
click('[data-come-filtro="da"]');
const vis=qa(".come-tab tbody tr[data-come-cid]").map(r=>r.dataset.comeCid);
click(q(`tr[data-come-cid="${vis[0]}"]`));tasto(" ");tasto("ArrowDown",{shiftKey:true});
tasto("a");
t("A: anteprima del blocco «Cambierà 2 componenti»",q(".blocco-b").textContent.includes("Cambierà 2 componenti"));
tasto("1");tasto("Enter");
t("blocco applicato con l'esito per riga",paAv().esito==="ok"&&paAv().testo.includes("Blocco"));
tasto("Enter");
t("FitLab: tutti smistati",!qa(".come-tab tbody tr[data-come-cid]").length);
click('.staz[data-staz="3"]');
// verniciatori dalla fase di trattamento del supporto saldato
click(byText(".cl-riga","9N008518AA")||q(".cl-riga"));
const fSab=ciclo("9N008518AA").fasi.find(f=>f.p==="sabbiatura").id; click(`[data-fase-apri="${fSab}"]`);
click(byText(".fase--aperta button","Richiesta ai verniciatori"));
const kV="verniciatori|sabbiatura";
t("T-R verniciatori: la carta si apre nel cassetto con 2–3 fornitori, nessuna richiesta creata dal clic",ev("S.cassetto")==="richieste"&&!!carta(kV)&&carta(kV).querySelectorAll(".rq-forn").length>=2&&!S2("x.richieste.length"));
prepara(kV,"Sabbiatura Valle");const idV=rqDi("f5");simula(kV,idV,"Simula: la mail parte");simula(kV,idV,"Simula la risposta");registraR(kV,idV);
t("T-R verniciatori: registrata nella fase di sabbiatura (RISP)",statoRq(idV)==="offerta_ricevuta"&&!!ciclo("9N008518AA").fasi.find(f=>f.p==="sabbiatura").risp);
// M7: il catalogo si apre sulla scheda utile al campo, con la destinazione; «Usa» è il clic sulla riga (v16 rev.: dal «⋯» della fase)
click('[data-cassetto=""]');
const fSa=ciclo("9N008518AA").fasi.find(f=>SALD_T.includes(f.p)).id; click(`[data-fase-apri="${fSa}"]`);
click(`[data-fase-menu="${fSa}"]`,"⋯ della saldatura");
click(q(`[data-cat-apri="componenti|fid:${fSa}"]`));
t("M7: dalla saldatura il catalogo si apre su Componenti, con la destinazione",!!q('#cassetto [data-cat-tab="componenti"][aria-pressed="true"]')&&q("#cassetto .cat-dest").textContent.includes("fase"));
click(byText("#cassetto .cat-r","Dado esagonale a saldare M8"));
t("componente di catalogo nella fase, non nell'albero (R-10)",String(ciclo("9N008518AA").fasi.find(f=>f.id===fSa).par.catalogo||"").includes("DIN 929 M8")&&fotoA()===A2tg);
if(q("#cassetto")) click('[data-cassetto=""]');click(byText(".cl-riga","9N008520AA"));
click(q('[data-cat-apri^="materiali|"]'));
t("M7: dal grezzo il catalogo si apre su Materiali: «Scegli il materiale per …»",q("#cassetto .cat-dest").textContent.includes("Scegli il materiale per"));
click(byText("#cassetto .cat-r","S235JR · lamiera sp. 8"));
t("«Usa» sul grezzo: dalla banca dati nella prima fase",ev(`testoGrezzo(grezzoVal(cicloB(threadAperto(),${JSON.stringify(cidDi("9N008520AA"))})))`)==="S235JR · lamiera sp. 8");
if(q("#cassetto")) click('[data-cassetto=""]');
// il perno: tre terzisti, tre righe; MRT in tre giri conservati (R-17), TFR dichiara un altro materiale
click(byText(".cl-riga","9N008519AB"));
click(byText(".fase--aperta button","Richiesta al terzista"));
const kT="terzista|tornitura";
for(const n of ["Torneria Centrale","MRT","T.F.R."]) prepara(kT,n);
const idT=rqDi("mrt"),idTMB=rqDi("tfr");
for(const x of [rqDi("f2"),idT,idTMB]){simula(kT,x,"Simula: la mail parte");simula(kT,x,"Simula la risposta");}
t("T-R tornitura: tre righe, una per terzista; TFR dichiara un altro materiale (MAT-05)",S2("x.richieste.filter(q=>q.lavorazione==='tornitura').length")===3&&rigaR(idTMB).textContent.includes("MAT-05"));
for(let g=0;g<2;g++){apriCarta(kT);click(btnIn(rigaR(idT),"Nuovo giro"),"Nuovo giro");simula(kT,idT,"Simula: la mail parte");simula(kT,idT,"Simula la risposta");}
t("T-R tre giri conservati (il caso MRT)",S2(`x.richieste.find(q=>q.id===${JSON.stringify(idT)}).giri.length`)===2&&rigaR(idT).querySelector(".rq-giri").textContent.includes("giro 2"));
registraR(kT,idT);
t("T-R tornitura registrata (nella fase, RISP)",!!ciclo("9N008519AB").fasi[0].risp&&ciclo("9N008519AB").fasi[0].risp.rid===idT);
// la piastra: grezzo «non basta», lavorabilità laser ✕ → officina
click('[data-cassetto=""]');click(byText(".cl-riga","9N008520AA"));
const gTG=q("[data-pa-grezzo]");tasto("Enter",{},gTG);
t("stato del grezzo «In magazzino, non basta» (variante gialla, dec. 2)",q(".sg--non_basta")&&q(".sg--non_basta").textContent.includes("dec. 2"));
t("lavorabilità laser ✕ → «Prova di taglio · Richiedi all'officina» (v16: accende la preparazione)",q(".lav").textContent.includes("laser ✕")&&q(".lav").textContent.includes("Prova di taglio")&&!!byText(".lav button","Richiedi all'officina"));
click(byText(".lav button","Richiedi all'officina"));
const idO=rqTipo("officina");simula(idO,idO,"Simula: la mail parte");simula(idO,idO,"Simula la risposta");registraR(idO,idO);
click('[data-cassetto=""]');
t("T-R officina (proposta R-28): risposta registrata nella lavorabilità",statoRq(idO)==="risposta"&&q(".lav").textContent.includes("officina:"));
click(byText(".sg button","Chiedi prezzo e tempi"));
prepara("mp_prezzo","Fornitore MP 4");const idP=rqDi("mp4");simula("mp_prezzo",idP,"Simula: la mail parte");simula("mp_prezzo",idP,"Simula la risposta");
t("T-R tutti declinano → MAT-04",statoRq(idP)==="declinata"&&ev(`controlliB(threadAperto(),prodottoScelto(threadAperto())).some(x=>x.cod==='MAT-04')`));
click('[data-cassetto=""]');
// la ricerca con l'equivalenza del cliente
cambia(q("[data-pa-grezzo]"),"gsx9486 ii","equivalenza");
t("«gsx9486 ii» → S355J2 con l'equivalenza da confermare dalla qualità",q(".grz-esito").textContent.includes("S355J2")&&q(".grz-esito").textContent.includes("equivalenza «GSX9486 cat. II» da confermare"));
click(byText(".grz-esito button","Chiedi alla qualità"));
const idQ=rqTipo("qualita");simula(idQ,idQ,"Simula: la mail parte");simula(idQ,idQ,"Simula la risposta");registraR(idQ,idQ);
t("T-R qualità (proposta R-29): fino in fondo",statoRq(idQ)==="risposta"&&!!ev(`registrazioneDi(threadAperto(),a2Di(threadAperto()).richieste.find(q=>q.id===${JSON.stringify(idQ)}))`));
click('[data-cassetto=""]');
cambia(q("[data-pa-grezzo]"),"s235jr 8","grezzo");
// attrezzatura (Q-06): un set di curvatura acceso dalla fase, poi la carta «Attrezzature» del cassetto
ev(`eseguiB("/componente/"+${JSON.stringify(cidDi("9N008520AA"))}+"/preparazioni",{k:"curva",on:"1"})`);
prepara("attrezzatura","Minitool");const idAt=rqDi("minitool");simula("attrezzatura",idAt,"Simula: la mail parte");simula("attrezzatura",idAt,"Simula la risposta");registraR("attrezzatura",idAt);
t("T-R attrezzatura: fino in fondo, il costo nella preparazione (RISP)",statoRq(idAt)==="offerta_ricevuta"&&Object.values(ciclo("9N008520AA").prep).some(v=>v.k==="curva"&&v.fonte==="RISP"&&v.rispDa===idAt));
// domanda tecnica dalla nota sul disegno: la mail entra nella conversazione del thread quando parte
click('[data-cassetto=""]');
const msg0=ev("MESSAGGI.filter(m=>m.req==='r-tg1').length");
submit(q('form[data-pa-gesto$="/nota"]').querySelector("input[name=testo]").value="raggio di piega non quotato?"&&q('form[data-pa-gesto$="/nota"]'),"nota");
t("N: nota sul disegno legata alla fase aperta (R-14)",S2(`x.note[${JSON.stringify(cidDi("9N008520AA"))}].length`)===1);
click(byText(".cl-note button","Domanda tecnica al cliente"));
const idD=rqTipo("domanda");simula(idD,idD,"Simula: la mail parte");
t("T-R domanda tecnica: partita, la mail è nella conversazione della richiesta",ev("MESSAGGI.filter(m=>m.req==='r-tg1').length")===msg0+1);
simula(idD,idD,"Simula la risposta");registraR(idD,idD);
t("T-R domanda tecnica: registrata come nota",!!ev(`registrazioneDi(threadAperto(),a2Di(threadAperto()).richieste.find(q=>q.id===${JSON.stringify(idD)}))`));
t("T-R il contatore del cassetto è coerente",(()=>{const n=S2("x.richieste.filter(q=>['offerta_ricevuta','risposta'].includes(q.stato)&&!registrazioneDi(threadAperto(),q)).length");const b=q('.cas-btn[data-cassetto="richieste"] .cas-n');return n?(b&&+b.textContent.trim()===n):!b;})());
t("tutti gli 11 tipi di richiesta del §2.4 percorsi",(()=>{const tipi=new Set([...JSON.parse(ev("JSON.stringify(Object.values(A2).flatMap(x=>x.richieste.map(q=>q.tipo)))"))]);return ["terzista","lamiera","mp_mai","mp_prezzo","verniciatori","cliente","domanda","officina","attrezzatura","qualita","commerciale"].every(k=>tipi.has(k));})());
t("B1 FitLab: la zona A non cambia",fotoA()===A2tg);
B("FitLab");
Z("passata 2 · FitLab");

/* ================= T-K · tastiera della zona B ================= */
apriRFQ("482-00");
tasto("g");tasto("2");t("G 2 → Come",ev("S.staz")===2);
tasto("g");tasto("5");t("G 5 → Esito",ev("S.staz")===5);
tasto("l");t("L: Cataloghi",q("#cassetto")&&q("#cassetto").textContent.includes("Materiali"));tasto("Escape");
tasto("k");t("K: Controlli con le regole della zona B (v16: R-35 nel title della fascia)",q("#cassetto")&&q("#cassetto .banda-b").getAttribute("title").includes("R-35"));tasto("Escape");
tasto("?");t("? tasti della stazione",q(".tasti")&&q(".tasti").textContent.includes("OK · DUBBIO · KO"));tasto("Escape");
tasto("g");tasto("3");tasto("/");t("/ nel Ciclo: aggiungi una fase",d.activeElement&&d.activeElement.id==="agg-fase");
scrivi(q("#agg-fase"),"/pie","cerca fase");t("«/pie» trova la piegatura",!!byText(".agg-fase button","Piegatura"));
tasto("Escape");
t("nessun tasto esegue un gesto della zona A",ev("REGISTRO.filter(x=>x.zona==='A'&&x.metodo==='POST').every(x=>!/tastiera/.test(x.url))"));

/* ================= T-G · giro completo: ogni RFQ, ogni stazione, ogni vista ================= */
for(const num of ev("RFQ.map(q=>q.num)")){
  apriRFQ(num);
  for(const s of [1,2,3,4,5]){ click(`.staz[data-staz="${s}"]`); if(s===5) for(const v of ["foglio","costi","listini","offerta","esito"]) click(`[data-esito-vista="${v}"]`); }
  for(const c of ["richieste","controlli","cataloghi","backend"]){ if(!q(`[data-cassetto="${c}"]`)) click('[data-menu="altro"]'); click(`[data-cassetto="${c}"]`); }
  click('[data-cassetto=""]');
}
t("T-G: ogni RFQ, stazione e vista si apre senza errori",errori.length===0);

/* ================= §9.2 · ogni pulsante fa qualcosa di visibile (zona B, VFR) ================= */
apriRFQ("482-00");
for(const s of [2,3,4,5]){
  click(`.staz[data-staz="${s}"]`);
  const n=qa(".ws-main button:not([disabled])").length; let muti=[];
  for(let i=0;i<Math.min(n,45);i++){
    click(`.staz[data-staz="${s}"]`); if(ev("S.cassetto")) click('[data-cassetto=""]');
    const b=qa(".ws-main button:not([disabled])")[i]; if(!b) break;
    if(["true","page","step"].includes(b.getAttribute("aria-pressed")||b.getAttribute("aria-current")||"")) continue;   /* già nello stato che il pulsante chiede */
    const html0=d.body.innerHTML, reg0=ev("REGISTRO.length"), etich=b.textContent.trim().slice(0,40);
    b.click();
    const cambiato = d.body.innerHTML!==html0 || ev("REGISTRO.length")!==reg0 || !!ev("PA.avviso");
    tasto("Escape");
    if(!cambiato) muti.push(etich);
    ev("PA.avviso=null");
  }
  t(`§9.2 stazione ${s}: ogni pulsante fa qualcosa (${n} provati)`,!muti.length||(console.log("   muti: "+muti.join(" | ")),false));
}
Z("passata 2 · fine");

// 9. impostazioni
click('[data-menu="app"]');click('.tendina--app [data-tab="set"]');t("impostazioni: solo 2 sezioni",qa(".p-rail [data-setsez]").length===2);
click('[data-setsez="nascosti"]');click('[data-autorizza="n2"]');
t("Marta autorizzata → Da smistare",ev("SMISTARE.some(s=>s.id==='s-n2')"));
click('[data-setsez="clienti"]');click('[data-setcli="fitlab"]');q("#nm-n").value="Marco Bassini";q("#nm-e").value="m.bassini@fitlab.example";click('[data-addmitt]');
t("mittente aggiunto",ev("cl('fitlab').mittenti.length")===4);
click('[data-nuovo="1"]');q("#fc-nome").value="Solleva Epsilon Italia";click('[data-salvacli]');t("nuovo cliente creato",ev("CLIENTI.some(c=>c.nome==='Solleva Epsilon Italia')"));
click('[data-tab="inbox"]');click('[data-sez="smistare"]');click('[data-orf="s-n2"]');click('[data-nuovareq="s-n2"]');qa('[data-cod]').forEach(c=>c.checked=true);click('[data-creareq]');
t("richiesta di Marta creata in ACME",ev("reqDi('acme').length")===3);
t("zona B: con due strutture candidate e nessuna BOM non si sceglie la prima (R3)",ev(`(()=>{const L=Object.values(THREADS).flatMap(T=>T.prodotti.filter(p=>(p.strutture_candidate||[]).length>1&&!p.nodi.length).map(p=>[T,p]));return L.every(([T,p])=>righeB(T,p).length===0);})()`));
t("JSON di /motore-a/dati: nessuna chiave interna _k sul ProdottoValutato",ev(`Object.values(THREADS).every(T=>T.prodotti.every(p=>!JSON.stringify(p).includes('"_k"')))`));
t("riepilogo dell'albero nella forma del Go: blocchi e senza_codice sono stringhe, formato 1",ev(`Object.values(THREADS).every(T=>{const r=riepilogoAlbero(T);return r.formato===1&&r.blocchi.every(x=>typeof x==="string")&&r.senza_codice.every(x=>typeof x==="string");})`));
Z("fine");

console.log(`\n${ok} superati · ${ko} falliti · errori JS: ${errori.length}`);errori.slice(0,5).forEach(e=>console.log("  JS: "+e));
process.exit(ko||errori.length?1:0);
