/* ================= 18 · il finto server A2 (zona B · proposta «dopo A2») =================
   SPEC_v15 §2.3–§2.9, §5.3. Il backend di oggi (A1d) NON ha come si fa, ciclo, fasi, grezzo, magazzino, lavorabilità,
   saldatura, preparazioni, Sequenza, richieste, esiti né consegna: sono PROPOSTE legate alle richieste a Pietro
   (R-10…R-41, D-01…D-06) e qui le fa un secondo finto server, separato da quello della zona A.
   Regole di questo file:
   - è l'UNICO posto dove si calcolano gli stati della zona B: statoGrezzo, fabbisogno, capacitaMateriale, coperturaWPS,
     convertiCordone, stadiDi, modoComponente e i controlli (REGOLE). Le viste li leggono e basta (RV-28, UI-18);
   - un'anteprima (lettura) non scrive mai niente;
   - le scritture arrivano solo come gesti [data-pa-gesto] con rotte IMMAGINATE, mai una rotta della lista A (RV-25);
   - non scrive MAI nella zona A: albero, tipi, categorie, documenti, fonte, fase registrata restano come sono (RV-26);
   - il ciclo si lega a componente.id: senza un componente confermato risponde «no» (RV-27);
   - un controllo che nasce da dati inventati non è mai KO (UI-20).
   Risponde {esito:"ok"|"no", testo}. */

/* l'orologio del mockup: il giorno e l'ora dei gesti. RL-09, RL-28: ogni consegna, firma e revisione ha la sua ora; il
   gesto «(mockup) passa una settimana» sposta il giorno, così una revisione emessa giorni dopo ha la data propria */
let B_OGGI = "11/10/2026", B_MIN = 10*60+10;
const due = n => String(n).padStart(2,"0");
function oraB(){ B_MIN += 2; return `${due(Math.floor(B_MIN/60)%24)}:${due(B_MIN%60)}`; }
function avanzaGiorniB(n){ const [g,m,a] = B_OGGI.split("/").map(Number), d = new Date(Date.UTC(a, m-1, g+n)); B_OGGI = `${due(d.getUTCDate())}/${due(d.getUTCMonth()+1)}/${d.getUTCFullYear()}`; B_MIN = 8*60+28; return B_OGGI; }
const A2 = {};
function a2Di(T){
  if(!A2[T.key]) A2[T.key] = { come:{}, cicli:{}, richieste:[], posta:[], seq:{}, proposteAlbero:[],
    esiti:{}, firme:{}, consegne:[], note:{}, visti:{}, datiDoc:{}, prezziMP:{}, nF:0, nR:0, avvisato:false };
  return A2[T.key];
}
/* v16 rev. (A2 di Lino, P-08 di Pietro): la Sequenza confermata è PER PRODOTTO, come la rotta /prodotto/{pid}/sequenza/…
   (R-32 precisata: prodotto, stato, firma, chi, quando). Una conferma non tocca gli altri prodotti della RFQ. */
const SEQ_VUOTA = Object.freeze({stato:"vuota", quando:null, firma:null});
function seqDi(T, p){ return (p && a2Di(T).seq[p.rif]) || SEQ_VUOTA; }
/* il prodotto della RFQ il cui componente radice è cid (le rotte della Sequenza portano il componente del prodotto) */
function prodottoDaCid(T, cid){ return (T.esito.prodotti_valutati||[]).find(p=>{ const r = righeB(T,p).find(x=>x.radice); return r && r.cid===cid; }) || null; }
const SI_B = t => ({esito:"ok", testo:t});
const NO_B = t => ({esito:"no", testo:t});

/* ---------------- dati della RFQ che il backend non ha ancora: lotti (R-18), requisiti dell'ordine (R-20), «ultima volta» ---------------- */
function lottiDi(T){
  const q = T.rfq ? rf(T.rfq) : null, r = T.req ? rq(T.req) : null;
  const pids = q ? datiRFQ(q).prodotti : r ? r.prodotti : [];
  const s = pids.length ? String(PROD[pids[0]].qta||"") : "";
  const m = s.match(/(\d[\d.]*)/), v = m ? parseInt(m[1].replace(/\./g,""),10) : null;
  const extra = (A2[T.key] && A2[T.key].lottiExtra) || [];
  const conExtra = o => { const l = [...new Set([...o.lotti, ...extra])].sort((a,b)=>a-b); return Object.assign(o, {lotti:l, testo: o.testo + (extra.length ? ` · aggiunti: ${extra.join(", ")}` : "")}); };
  if(!v) return conExtra({lotti:[100,500], daChiedere:true, testo:"quantità non indicata: lotti proposti 100 e 500 (LOT-01)"});
  if(/anno/.test(s)) return conExtra({lotti:[Math.max(1,Math.round(v/4)), v], daChiedere:false, testo:`${s}: lotti ${Math.round(v/4)} e ${v}`});
  return conExtra({lotti:[v], daChiedere:false, testo:s});
}
/* la revisione precedente di un codice già quotato o prodotto (ECO, §2.9): dati di prova */
const STORICO_REV = { "92176C": {rev:"B", data:"05/2026", rif:"offerta SO 5402 (dati di prova)", delta:"rev. C: spessore 3 → 4, un foro Ø9 in più"} };
const REQUISITI = {
  "q-sv1":[
    {id:"rq1", testo:"EN 15085-2 CL1 · specifica SP-07 rev.00", fonte:"ORD", tipo:"saldatura"},
    {id:"rq2", testo:"Certificati 3.1 EN 10204 citati sul DDT", fonte:"ORD", tipo:"certificati"},
    {id:"rq3", testo:"Tolleranze ISO 13920 BE", fonte:"CART", tipo:"controllo", valore:"ISO 13920 BE"},
    {id:"rq4", testo:"Penale 120 €/h di fermo linea (valore di esempio)", fonte:"ORD", tipo:"condizione"},
    {id:"rq5", testo:"Accettazione tacita dell'ordine in 3 giorni", fonte:"ORD", tipo:"condizione", avvisa:true},
    {id:"rq6", testo:"Verifiche del cliente nella nostra sede senza preavviso", fonte:"ORD", tipo:"condizione"}],
  "r-tg1":[
    {id:"rq1", testo:"Sabbiatura CAP-SAB-017 + polvere CAP-VER-024", fonte:"CAPIT", tipo:"trattamento"},
    {id:"rq2", testo:"Tolleranze EN 22768 cL", fonte:"CART", tipo:"controllo", valore:"dimensionale a calibro"}],
  "r-ar1":[
    {id:"rq1", testo:"Zincatura Fe/Zn 12 III Cr3 · SPEC-ZINC-013", fonte:"CAPIT", tipo:"trattamento"},
    {id:"rq2", testo:"Saldatura ad arco continua SPEC-SALD-023", fonte:"CAPIT", tipo:"saldatura"},
    {id:"rq3", testo:"Cost breakdown e peso al pezzo con l'offerta", fonte:"MAIL", tipo:"condizione"}],
};
const ULTIMA = { "F+800.514263.000":{prev:"18 h", cons:"26 h", rif:"commessa VFR 2025"} };

/* ---------------- i semi: le proposte del modello e dello storico per i codici che conosciamo ----------------
   Ogni valore proposto porta la sua fonte: CART cartiglio · STEP · DXF · NOTA · ORD ordine · STR struttura · STO storico ·
   MAG magazzino · CAP capacità · RISP risposta. Sono PROPOSTE (◇): diventano dati solo quando Lino le accetta. */
const SEMI = {
 "F+800.514263.000":{modo:"noi", modello:"T2", fonte:"ORD", perche:"telaio con 8 figli saldati (ordine 482-00)", netto:[1816.58,"STEP"],
   prep:{maschera:{costo:"", stato:"da fare"}},
   fasi:[{p:"sald_mag", prop:{norma:["EN 15085-2 CL1 · SP-07 rev.00","ORD"], wps:["135-020","CAP"], maschera:["sì","STR"]}},
         {p:"prigionieri", prop:{n:["6","STR"], mis:["M14","STR"]}},
         {p:"controllo", prop:{tipo:["scanner 3D Scan3X","STO"], freq:["100 %","STO"]}},
         {p:"certificati", prop:{tipo:["3.1 EN 10204","ORD"], per:["travi UPN, tubi e piastre","ORD"]}},
         {p:"imballo", prop:{tipo:["a vista","STO"]}}]},
 "F+800.514263.001":{modo:"fuori", modello:"T4b", fonte:"STO", perche:"travi lavorate da RTC in conto lavoro; le barre le compriamo noi", netto:[1096,"STEP"],
   grezzo:{q:["S355J2","CART"], forma:["profilo","STEP"], prof:["UPN 160","STEP"], barre:["5 × 12 m","STO"]},
   fasi:[{p:"travi_est", prop:{barre:["5 × 12 m","STO"], terzi:["rtc","STO"], lav:["taglio a misura, forature e lavorazioni del disegno 001 (fogli 1 e 2)","NOTA"], conto:["conto lavoro","STO"]}}]},
 "F+800.514263.002":{modo:"noi", modello:"T3", fonte:"STO", perche:"tubo Ø127 × 10: sega, foratura e smussatura (scheda costo)", netto:[19.1,"STEP"],
   grezzo:{q:["S355J2","CART"], forma:["tubo_tondo","STEP"], D:["127","STEP"], sp:["10","STEP"]},
   fasi:[{p:"sega", prop:{L:["690","STEP"], tagli:["1","STEP"], tempo:["30 pz/h","STO"]}},
         {p:"foratura", prop:{n:["2","NOTA"], mis:["Ø18","NOTA"], tempo:["20 pz/h","STO"]}},
         {p:"smussatura", prop:{n:["2","NOTA"], mis:["2 × 45°","NOTA"], tempo:["20 pz/h","STO"]}}]},
 "F+800.514263.006":{modo:"lamiera", modello:"T5", fonte:"STO", perche:"lamiera sagomata: tagliata e sagomata fuori, in conto vendita", netto:[298.3,"STO"],
   fasi:[{p:"laser_est", prop:{terzi:["lamitaglio","STO"], lotto:["2","STR"], conto:["conto vendita","STO"], cert:["3.1 EN 10204","ORD"]}},
         {p:"trasporto", prop:{da:["Lamitaglio","STO"], a:["Stab. 4 · Borgo Piano","STO"], mezzo:["motrice","STO"]}}]},
 "F+800.514263.008":{modo:"noi", modello:"T1b", fonte:"STO", perche:"piastra sp. 15 al laser con 8 fori", netto:[15.02,"STEP"],
   grezzo:{q:["S355J2","CART"], forma:["lamiera","STEP"], sp:["15","STEP"], larg:["360","STEP"], lung:["360","STEP"]},
   fasi:[{p:"laser", prop:{rett:["360 × 360","STEP"], m:["1,5","STO"], fori:["8","STO"]}},
         {p:"filettatura", prop:{n:["8","NOTA"]}}]},
 "F+800.514263.009":{modo:"noi", modello:"T1b", fonte:"STO", copia:"F+800.514263.008", perche:"piastra sp. 15 come …008: copia il ciclo e scrivi i numeri", netto:[9.4,"STEP"],
   grezzo:{q:["S355J2","CART"], forma:["lamiera","STEP"], sp:["15","STEP"], larg:["280","STEP"], lung:["290","STEP"]}},
 "POS.2":{modo:"cliente", modello:"T7", fonte:"ORD", perche:"forniti dal cliente in conto lavoro con l'ordine 482-00"},
 "POS.3":{modo:"cliente", modello:"T7", fonte:"ORD", perche:"forniti dal cliente in conto lavoro con l'ordine 482-00"},
 /* FitLab */
 "9N007614AE":{modo:"noi", modello:"T2", fonte:"STO", perche:"perni piantati nel supporto verniciato", fasi:[
   {p:"piantaggio", prop:{n:["2","STR"], tempo:["60 pz/h","STO"]}}, {p:"controllo", prop:{tipo:["dimensionale a calibro","CART"], freq:["a campione","STO"]}}]},
 "9N008518AA":{modo:"noi", modello:"T2", fonte:"STO", perche:"supporto saldato, sabbiato e verniciato (CAP-SAB-017, CAP-VER-024)", fasi:[
   {p:"sald_mag", prop:{tempo:["12 pz/h","STO"], maschera:["no","STO"]}}, {p:"raddrizzatura", prop:{}},
   {p:"sabbiatura", prop:{terzi:["f5","STO"], spec:["CAP-SAB-017","CAPIT"]}}, {p:"verniciatura_polvere", prop:{terzi:["f1","STO"], col:["a capitolato","CAPIT"], spec:["CAP-VER-024","CAPIT"]}},
   {p:"controllo", prop:{tipo:["dimensionale a calibro","CART"], freq:["a campione","STO"]}}]},
 "9N008520AA":{modo:"noi", modello:"T1b", fonte:"CART", perche:"piastra base sp. 8", netto:[0.98,"STEP"],
   grezzo:{q:["S235JR","CART"], forma:["lamiera","STEP"], sp:["8","STEP"], larg:["160","STEP"], lung:["110","STEP"]},
   fasi:[{p:"laser", prop:{rett:["160 × 110","STEP"], m:["0,62","STO"], fori:["4","STO"]}}, {p:"filettatura", prop:{n:["4","NOTA"], mis:["M8","NOTA"]}}]},
 "9N008519AB":{modo:"fuori", modello:"T4", fonte:"STO", perche:"perno tornito: fuori, materiale del terzista", fasi:[
   {p:"tornitura", prop:{terzi:["f2","STO"], lotti:["300","STO"], conto:["conto vendita","STO"]}}]},
 /* ACME e Potaflex */
 "9990708A1":{modo:"noi", modello:"T2", fonte:"NOTA", perche:"supporto con 3 dadi saldati, poi zincato", fasi:[
   {p:"sald_robot", prop:{tempo:["", ""], maschera:["sì","STR"], norma:["SPEC-SALD-023 · ad arco continua","CAPIT"]}},
   {p:"controllo", prop:{tipo:["scanner 3D Scan3X","STO"], freq:["primo pezzo","STO"]}},
   {p:"zincatura", prop:{terzi:["f3","STO"], spec:["Fe/Zn 12 III Cr3 · SPEC-ZINC-013","CAPIT"]}}]},
 "9990707A1":{modo:"noi", modello:"T1", fonte:"CART", perche:"lamiera S235JR sp. 3 piegata (vista A)", netto:[0.52,"STEP"],
   grezzo:{q:["S235JR","CART"], forma:["lamiera","STEP"], sp:["3","STEP"], larg:["182","STEP"], lung:["140","STEP"]},
   fasi:[{p:"laser", prop:{rett:["182 × 140","STEP"], m:["0,72","DXF"], fori:["6","DXF"]}}, {p:"piega", prop:{pieghe:["1","STEP"], lmax:["170","STEP"], op:["1","STO"], tempo:["40 pz/h","STO"]}}, {p:"sbavatura", prop:{}}]},
 "92176C":{modo:"noi", modello:"T1", fonte:"STO", perche:"come nell'offerta SO 5478", netto:[0.31,"STEP"],
   grezzo:{q:["S235JR","CART"], forma:["lamiera","STEP"], sp:["4","STEP"], larg:["120","STEP"], lung:["95","STEP"]},
   fasi:[{p:"laser", prop:{rett:["120 × 95","STEP"], m:["0,48","STO"], fori:["2","STO"]}}, {p:"piega", prop:{pieghe:["2","STO"], lmax:["95","STO"], op:["1","STO"], tempo:["45 pz/h","STO"]}}]},
};
/* le risposte simulate delle richieste: chi risponde che cosa (il «ritardo dichiarato» è scritto sulla risposta) */
const RISPOSTE_SIM = {
 "terzista|rtc":       {prezzo:600, unita:"€ a telaio", lotto:"1 telaio", gg:15, materiale:"barre vostre, conto lavoro", valida:"30 gg"},
 "lamiera|lamitaglio":   {prezzo:410, unita:"€/pz", lotto:"2", gg:20, materiale:"S355J2 · 3.1", valida:"30 gg", trasporto:250},
 "lamiera|mp4":        {declina:"non sagomiamo questo spessore"},
 "mp_mai|mp1":         {prezzo:1.24, unita:"€/kg", lotto:"1 barra da 6 m", gg:10, materiale:"S355J2 · EN 10210 · 3.1", valida:"15 gg"},
 "mp_mai|mp3":         {prezzo:1.18, unita:"€/kg", lotto:"1 barra da 6 m", gg:12, materiale:"S235JRH · EN 10219 · 3.1", valida:"15 gg"},
 "mp_prezzo|mp2":      {prezzo:0.83, unita:"€/kg", lotto:"5 barre da 12 m", gg:10, materiale:"S355J2 · 3.1", valida:"15 gg"},
 "mp_prezzo|mp1":      {declina:"profili non a listino"},
 "mp_prezzo|mp4":      {declina:"lamiera sp. 8 non disponibile prima di 6 settimane"},
 "terzista|f2":        {prezzo:4.20, unita:"€/pz", lotto:"300", gg:15, materiale:"C40 a carico nostro", valida:"60 gg"},
 "terzista|mrt":       {prezzo:4.40, unita:"€/pz", lotto:"300", gg:20, materiale:"C40", valida:"30 gg"},
 "terzista|tfr":       {prezzo:3.70, unita:"€/pz", lotto:"300", gg:15, materiale:"11SMnPb37 (automatico)", valida:"30 gg"},
 "verniciatori|f5":    {prezzo:1.20, unita:"€/pz", lotto:"300", gg:7, materiale:"—", valida:"60 gg"},
 "verniciatori|f1":    {prezzo:3.90, unita:"€/pz", lotto:"300", gg:10, materiale:"—", valida:"60 gg"},
 "verniciatori|polvernova":{prezzo:3.95, unita:"€/pz", lotto:"300", gg:10, materiale:"—", valida:"60 gg"},
 "officina|officina":  {testo:"Sp. 8 S235JR: lo tagliamo sulla Lasertek 3020 con i parametri del 6; serve 1 ora di prova di taglio prima del lotto.", lav:"○"},
 "attrezzatura|attrezzature":{prezzo:1200, unita:"€", testo:"set di curvatura in 3 settimane"},
 "qualita|qualita":    {testo:"Confermato: la WPS 135-020 copre il giunto; per il dado zincato serve l'aspirazione localizzata."},
 "cliente|cliente":    {testo:"Il cliente risponde: il dado M14 è ISO 4032 cl. 8 zincato, nessun disegno. La conferma della minuteria resta da fare (R-08)."},
 "domanda|cliente":    {testo:"Il cliente risponde: la quota è da intendersi dopo la saldatura."},
 "commerciale|commerciale":{testo:"Visto: rispondo al cliente entro la scadenza dell'accettazione tacita."},
};

/* v16 · le risposte simulate delle preparazioni: una riga per preparazione della richiesta (costo e SETTIMANE, R-13 + settimane) */
const PREP_SIM = { maschera:[1300,4], robot:[640,1], piega:[1000,4], curva:[1200,3], stampo:[8000,10], dima:[350,1], calibro:[280,2], prova:[180,1], altra:[500,2] };
function rispostaPrepSim(T, q, id){
  if(id==="dalmi") return {declina:"il set lo quota Minitool per noi"};
  const interno = !TIPI_RICHIESTA[q.tipo].esterni, righe = (q.preps||[]).map(x=>{ const cy = cicloB(T,x.cid), v = cy && cy.prep[x.key], k = v ? v.k : x.k;
    const iu = (()=>{ if(!cy || !v || !v.nata) return null; const r = (T.esito.prodotti_valutati||[]).map(p=>rigaB(T,p,x.cid)).find(Boolean); const f = cy.fasi.find(y=>y.id===v.nata); return r && f ? infoUnione(T, null, r, cy, f) : null; })();
    const [eur, sett] = PREP_SIM[k] || PREP_SIM.altra;
    const pre = iu && iu.preUnione ? 0.55 : 1;   /* la maschera della pre-unione è più piccola */
    return {cid:x.cid, key:x.key, k, prezzo: Math.round(eur*pre*(interno?0.7:1)/10)*10, settimane: interno ? Math.max(1, sett-2) : sett}; });
  const tot = righe.reduce((a,x)=>a+x.prezzo,0), sett = righe.reduce((a,x)=>Math.max(a,x.settimane),0);
  return {prezzo:tot, unita:"€", settimane:sett, righe, lotto:"1", gg: sett*5, valida:"60 gg",
    testo: interno ? `La facciamo noi: ${righe.map(x=>`${nomePrep(x.k).toLowerCase()} ${fmt(x.prezzo,0)} € in ${x.settimane} sett.`).join(" · ")}` : `${righe.map(x=>`${nomePrep(x.k).toLowerCase()} ${fmt(x.prezzo,0)} €`).join(" + ")} · consegna ${sett} settimane dall'ordine`};
}

/* ---------------- le righe di Come: una per codice distinto, lette dalla zona A (il server legge, la vista no) ---------------- */
function nodiDiLavoro(p){
  /* RL-02: senza BOM di lavoro si leggono i confermati senza STEP (contratto §1.4, E4: radice compresa) quando sotto la
     radice c'è qualcosa (componenti aggiunti a mano, R-38) o quando non c'è nessuna candidata; altrimenti, come prima,
     l'anteprima grigia si fa solo se la candidata è UNA: con due o più nessuna è scelta dal codice (R3, v1 §3.4 A) */
  const cand = p.strutture_candidate || [], senza = p.confermati_senza_step || [];
  if(p.nodi && p.nodi.length) return {bom:true, nodi: p.nodi.filter(n=>!(n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata"))};
  if(senza.length && (senza.length>1 || !cand.length)) return {bom:true, nodi: senza, senzaStep:true};
  const nodi = cand.length===1 ? cand[0].nodi : [];
  return {bom:false, nodi: nodi.filter(n=>!(n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata"))};
}
function qtaArcoB(T, p, pad, n, x, bom){
  /* la quantità confermata: la relazione (BOM di lavoro) o la parentela decisa dei confermati senza STEP (E4) */
  if(bom){ const cp = cidDelNodo(p,pad), cf = cidDelNodo(p,n); const r = cp&&cf ? T.relazioni.find(y=>y.padre===cp && y.figlio===cf) : null; if(r) return r.qta; if(x.decisa && x.quantita!=null) return x.quantita; }
  return x.quantita!=null ? x.quantita : 1;
}
function righeB(T, p){
  if(!p) return [];
  const {bom, nodi} = nodiDiLavoro(p);
  const qtaNel = (n, prof) => { if(!n.parentela.length || prof>8) return 1; let t=0;
    for(const x of n.parentela){ const pad = nodi.find(m=>m.nodo.rif===x.padre); if(!pad) continue; t += qtaArcoB(T,p,pad,n,x,bom) * qtaNel(pad, prof+1); } return t; };
  const out = [];
  for(const n of nodi){
    const cid = bom ? cidDelNodo(p, n) : null;
    const cod = codiceDelNodo(n);
    const figli = nodi.filter(m=>m.parentela.some(x=>x.padre===n.nodo.rif)).map(m=>{ const x=m.parentela.find(y=>y.padre===n.nodo.rif); return {rif:m.nodo.rif, cid: bom?cidDelNodo(p,m):null, cod:codiceDelNodo(m), descr:m.descrizione||"", qta:qtaArcoB(T,p,n,m,x,bom)}; });
    const esistente = out.find(r=>r.cod===cod);
    if(esistente){ esistente.assiemi += Math.max(1,n.parentela.length); esistente.qtaProd += qtaNel(n,0); continue; }
    const cat = (n.classificazione||{}).categoria;
    out.push({ rif:n.nodo.rif, cid, cod, descr: n.descrizione || ((T.componenti[cid]||{}).descrizione) || "", tipo:n.tipo, cat,
      comm: n.tipo==="commerciale" || cat==="commerciale", radice: !n.parentela.length, figli, assiemi: Math.max(1,n.parentela.length), qtaProd: qtaNel(n,0),
      pdf: n.disegni && n.disegni.primario ? n.disegni.primario.nome_file : null, bom });
  }
  return out;
}
const rigaB = (T, p, cid) => righeB(T,p).find(r=>r.cid===cid) || null;

/* ---------------- PRO-06: come si fa e il modello proposto (◇) ---------------- */
function modoComponente(T, p, r){
  if(r.comm) return {modo:"comm", modello:"T6", fonte:"A", perche:"commerciale nell'albero: lo compra Fabio, entra in una fase del padre"};
  const s = SEMI[r.cod];
  if(s && s.modo) return {modo:s.modo, modello:s.modello, fonte:s.fonte||"STO", perche:s.perche||"", copia:s.copia||null};
  const d = (r.descr||"").toLowerCase();
  if(r.figli.length) return {modo:"noi", modello:"T2", fonte:"STR", perche:`${r.figli.length} figli nell'albero: saldato`};
  /* RL-12: tondo → tornitura fuori; profilato, quadro, piatto, forcella → fresatura fuori; tubo → solo la sega */
  if(/perno|boccola|tornit|tondo/.test(d)) return {modo:"fuori", modello:"T4", fonte:"PRO-06", perche:"tondo lavorato: la tornitura va fuori il 99 % delle volte"};
  if(/upn|heb|ipe|trave/.test(d)) return {modo:"fuori", modello:"T4b", fonte:"PRO-06", perche:"profilo: travi da terzi"};
  if(/profilat|quadro|piatto|forcella|fresat/.test(d)) return {modo:"fuori", modello:"T4c", fonte:"PRO-06", perche:"profilato o forcella: fresatura e foratura da terzi"};
  if(/tubo/.test(d)) return {modo:"noi", modello:"T3", fonte:"PRO-06", perche:"tubo: taglio a sega · la foratura si aggiunge solo se il disegno la chiede"};
  if(/piastra/.test(d)) return {modo:"noi", modello:"T1b", fonte:"PRO-06", perche:"piastra: laser e fori"};
  return {modo:"noi", modello:"T1", fonte:"PRO-06", perche:"particolare in lamiera: laser e piega"};
}
/* la decisione registrata (dopo «Invio» o un tasto) oppure null */
const comeDi = (T, cid) => (a2Di(T).come[cid]||null);
const cicloB = (T, cid) => (a2Di(T).cicli[cid]||null);
function modoEffettivo(T, p, r){ const c = r.cid ? comeDi(T, r.cid) : null; return c ? c.modo : (r.comm ? "comm" : null); }

/* ---------------- il grezzo: ricerca nella banca dati, peso, stato in magazzino, fabbisogno, lavorabilità ---------------- */
function famigliaDi(q){ const m = matDi(q); return m ? m.f.k : (String(q||"").match(/^S\d{3}/)||[""])[0]; }
function gruppoISO(q){ const f = famigliaDi(q); if(/^S(235|275)/.test(f)) return "1.1"; if(/^S(355|420|460)/.test(f)) return "1.2"; if(/^S(500|700)/.test(f)) return "2"; if(/AISI/.test(f)) return "8.1"; if(/^(5|6)\d{3}/.test(f)) return "22"; return "1.1"; }
/* «s355j2 15» → S355J2 lamiera sp. 15 · «upn160» → UPN 160 · «355 tubo 127x10» · «gsx9486 ii» → equivalenza proposta */
function cercaGrezzo(testo, cliente){
  const s = String(testo||"").trim(); if(!s) return null;
  const low = s.toLowerCase().replace(/\s+/g," ");
  const eq = EQUIVALENZE.find(e=>low.replace(/[^a-z0-9]/g,"").includes(e.nome.toLowerCase().replace(/[^a-z0-9]/g,"")) || (/gsx9486/.test(low) && e.nome.startsWith("GSX9486") && ((/\bii\b/.test(low) && /II/.test(e.nome)) || (/\bia\b/.test(low) && /IA/.test(e.nome)))));
  let q = null, equiv = null, vistoPer = null;
  if(eq){ q = eq.q; equiv = eq; }
  else {
    const tutte = Object.keys(MAT_IDX).sort((a,b)=>b.length-a.length);
    const compatto = low.replace(/[\s+]/g,"");
    q = tutte.find(k=>compatto.includes(k.toLowerCase().replace(/[\s+]/g,""))) || null;
    /* RL-13: dal solo numero si propone prima la qualità già «vista» per quel cliente (S355J2+N per Telesol), poi quella di sempre */
    if(!q){ const m = low.match(/\b(235|275|355|420|460|500|700)\b/);
      if(m){ const cl = String(cliente||"").toLowerCase(), vista = cl ? Object.values(MAT_IDX).find(x=>x.f.k==="S"+m[1] && (x.q.visto||[]).some(v=>cl.includes(String(v).toLowerCase()))) : null;
        if(vista){ q = vista.q.k; vistoPer = (vista.q.visto||[]).find(v=>cl.includes(String(v).toLowerCase())); }
        else q = "S"+m[1]+(m[1]==="235"||m[1]==="355"?(m[1]==="355"?"J2":"JR"):"MC"); } }
  }
  const out = {q, equiv, forma:null, vistoPer};
  const prof = low.match(/\b(upn|heb|ipe)\s*(\d{2,3})/); const ang = low.match(/\bl\s*(\d{2})\s*[x×]\s*(\d)/);
  if(prof){ out.forma="profilo"; out.prof = prof[1].toUpperCase()+" "+prof[2]; if(!q){ out.q="S355J2"; out.qProposta=true; } }
  else if(ang){ out.forma="profilo"; out.prof=`L ${ang[1]}×${ang[2]}`; }
  else if(/tubo|ø|Ø/.test(low)){ const m = low.match(/(\d+[.,]?\d*)\s*[x×]\s*(\d+[.,]?\d*)(?:\s*[x×]\s*(\d+[.,]?\d*))?/);
    if(m && m[3]){ out.forma="tubo_rett"; out.a=m[1]; out.b=m[2]; out.sp=m[3]; } else if(m){ out.forma="tubo_tondo"; out.D=m[1]; out.sp=m[2]; } else out.forma="tubo_tondo"; }
  else if(/tondo/.test(low)){ out.forma="tondo"; const m=low.match(/(\d+)/g); if(m) out.D=m[m.length-1]; }
  else { const resto = eq ? low.replace(/gsx\s*9486|cat\.?|\bii\b|\bia\b/g," ") : low.replace(q?q.toLowerCase():"",""); const m = resto.match(/(\d+[.,]?\d*)/); out.forma="lamiera"; if(m) out.sp=m[1]; }
  return out;
}
function testoGrezzo(g){
  if(!g || !g.q) return "";
  const v = k => g[k]||"?";
  const f = {lamiera:`lamiera sp. ${v("sp")}`, tubo_tondo:`tubo Ø${v("D")} × ${v("sp")}`, tubo_rett:`tubo ${v("a")} × ${v("b")} × ${v("sp")}`, profilo:`${v("prof")}`, tondo:`tondo Ø${v("D")}`, piatto:`piatto ${v("b")} × ${v("sp")}`}[g.forma]||"";
  return `${g.q} · ${f}`;
}
/* il grezzo del ciclo: i dati accettati, altrimenti le proposte (per le anteprime) */
function grezzoVal(c){ if(!c||!c.grezzo) return null; const g = {}; for(const k of Object.keys(Object.assign({}, c.grezzo.prop, c.grezzo.par))) g[k] = c.grezzo.par[k]!=null ? c.grezzo.par[k] : (c.grezzo.prop[k]||[])[0]; return g.q ? g : null; }
function grezzoAccettato(c){ return !!(c && c.grezzo && c.grezzo.par && c.grezzo.par.q); }
function grezzoDi(g){
  if(!g) return null;
  return GREZZI.find(x=>x.q===g.q && x.forma===g.forma && (
    (g.forma==="lamiera" && num(g.sp)===x.sp) || (g.forma==="tubo_tondo" && num(g.D)===x.D && num(g.sp)===x.sp) ||
    (g.forma==="tubo_rett" && num(g.a)===x.a && num(g.b)===x.b && num(g.sp)===x.sp) || (g.forma==="profilo" && g.prof===x.prof) || (g.forma==="tondo" && num(g.D)===x.D))) || null;
}
/* peso lordo del pezzo, kg (anteprima: entra nel fabbisogno) */
function pesoGrezzo(T, cid){ return pesoGrezzoDi(cicloB(T,cid)); }
function pesoGrezzoDi(c){
  const g = grezzoVal(c); if(!g) return null;
  const fasi = c.fasi||[], val = (p,k) => { const f = fasi.find(x=>x.p===p); return f ? (f.par[k]!=null?f.par[k]:(f.prop[k]||[])[0]) : null; };
  const rt = String(val("laser","rett")||"").match(/(\d+[.,]?\d*)\s*[×x]\s*(\d+[.,]?\d*)/);
  const m = {fam: famigliaDi(g.q), forma: g.forma, sp:g.sp, larg: rt?rt[1]:g.larg, lung: rt?rt[2]:g.lung, D:g.D, a:g.a, b:g.b, prof:g.prof, L: g.L || val("sega","L") || val("lasertubo","L")};
  if(g.forma==="lamiera"){ g.larg = m.larg; g.lung = m.lung; }
  if(g.forma==="profilo"){ const b = String(g.barre||val("travi_est","barre")||"").match(/(\d+)\s*[×x]\s*(\d+[.,]?\d*)\s*m/); const pr = PROFILI.find(x=>x[0]===g.prof);
    if(b && pr) return {kg: +b[1]*num(b[2])*pr[1], formula:`${b[1]} barre × ${b[2]} m × ${fmt(pr[1])} kg/m`}; return null; }
  const kg = pesoLordo(m); if(!kg) return null;
  const formula = g.forma==="lamiera" ? `${g.sp} × ${g.larg} × ${g.lung} mm × ρ ${fmt((famDi(m.fam)||{rho:7.85}).rho)}` : g.forma==="tubo_tondo" ? `tubo Ø${g.D} × ${g.sp} × L ${m.L} × ρ` : "volume × ρ";
  return {kg, formula};
}
function certRichiesto(T){ return (REQUISITI[T.key]||[]).some(r=>r.tipo==="certificati" && /3\.1/.test(r.testo)) ? "3.1" : null; }
/* il fabbisogno del lotto più grande: pezzi × kg lordi (R-37) */
function fabbisogno(T, p, cid){
  const r = rigaB(T,p,cid), w = pesoGrezzo(T,cid), L = lottiDi(T), lotto = Math.max(...L.lotti);
  if(!r || !w) return null;
  return {pezzi: r.qtaProd*lotto, kg: w.kg*r.qtaProd*lotto, lotto, testo:`${r.qtaProd} pz × lotto ${lotto} × ${fmt(w.kg,2)} kg = ${fmt(w.kg*r.qtaProd*lotto,1)} kg`};
}
/* MAT-01: lo stato del grezzo, come anteprima (decisione 2: tre stati, con la variante gialla «non basta») */
function statoGrezzo(T, p, cid){
  const c = cicloB(T,cid), g = grezzoVal(c); if(!g) return null;
  const S2 = a2Di(T), gr = grezzoDi(g), fb = fabbisogno(T,p,cid), cert = certRichiesto(T);
  const risp = gr ? S2.prezziMP[gr.id] : null;
  const base = {gid: gr?gr.id:null, al:GIACENZA_AL, risp, fb, cert};
  if(!gr) return Object.assign(base, {stato:"mai", testo:"Mai acquistato: il grezzo non è nella banca dati", prezzoProvv:null});
  const kgUnita = gr.kgm2 ? gr.kgm2*4.5 : gr.kgm ? gr.kgm*(gr.barra||6000)/1000 : 0;
  const giac = GIACENZE.filter(x=>x.g===gr.id);
  if(giac.length){
    const buone = giac.filter(x=>!cert || x.cert===cert), libere = buone.reduce((a,x)=>a+x.qta-x.impegnato,0), kgLib = libere*kgUnita;
    const migliore = buone[0] || giac[0];
    if(buone.length && fb && kgLib >= fb.kg) return Object.assign(base, {stato:"magazzino", testo:`In magazzino · ${libere} ${migliore.um} · ${migliore.stab} · colata ${migliore.colata} · ${migliore.cert} · copre il lotto`, giac:migliore, kgLib});
    const motivo = !buone.length ? `a magazzino solo con ${giac[0].cert}: l'ordine chiede ${cert}` : `libero ${fmt(kgLib,0)} kg su ${fb?fmt(fb.kg,0):"?"} kg${giac.some(x=>x.impegnatoPer)?" · impegnato per "+giac.find(x=>x.impegnatoPer).impegnatoPer:""}`;
    return Object.assign(base, {stato:"non_basta", testo:`In magazzino, non basta · ${motivo}`, giac:migliore, kgLib});
  }
  const acq = ACQUISTI.find(x=>x.g===gr.id && !x.soloQuotato);
  if(acq) return Object.assign(base, {stato:"acquistato", testo:`Già acquistato · ${fornMP(acq.forn).nome} ${acq.data} · ${fmt(acq.eurkg,2)} €/kg · lotto min. ${acq.lottoMin} · ${acq.gg} gg${gr.gest==="commessa"?" · a commessa":" · a scorta"}`, acq});
  const quot = ACQUISTI.find(x=>x.g===gr.id && x.soloQuotato);
  /* RL-13: prima il prezzo della qualità esatta (S355J2+N tubo del tirante Telesol), poi uno simile della famiglia */
  const esatto = PREZZI_MAT.find(x=>x.q && x.q===g.q && x.gr===GRUPPO_FORMA[g.forma]);
  const simile = PREZZI_MAT_B.find(x=>x.fam===famigliaDi(g.q) && x.forma===g.forma);
  return Object.assign(base, {stato:"mai", testo:`Mai acquistato${quot?" · quotato per "+quot.commessa:""}`, prezzoProvv: esatto ? `${fmt(esatto.eur,2)} €/kg provvisorio da «${esatto.rif}» (stessa qualità, da confermare)` : simile ? `${fmt(simile.eur,2)} €/kg provvisorio da «${simile.nome}» (da confermare)` : "prezzo da chiedere"});
}
const PREZZI_MAT_B = [ {fam:"S355", forma:"tubo_tondo", eur:1.24, nome:"tubo S355 · scheda di esempio"}, {fam:"S355", forma:"tubo_rett", eur:1.20, nome:"tubo S355J2H"}, {fam:"S235", forma:"lamiera", eur:0.85, nome:"lamiera S235"} ];
/* CAP-01: lavorabilità del grezzo per le lavorazioni del ciclo (tabelle 46c + CONSUMI); tutto «inventato» */
const PARAM_LASER = [ {q:"S355J2", sp:15, macc:"lasertek3050"}, {q:"S235JR", sp:3, macc:"lasertek3020"}, {q:"S235JR", sp:4, macc:"lasertek3020"} ];
function capacitaMateriale(T, cid){
  const c = cicloB(T,cid), g = grezzoVal(c); if(!g) return [];
  const gr = grezzoDi(g), cons = gr ? CONSUMI.filter(x=>x.g===gr.id) : [], fatto = lav => cons.some(x=>x.lav.includes(lav));
  const sp = num(g.sp), out = [];
  const usa = new Set((c.fasi||[]).map(f=>f.p));
  if(usa.has("laser")){
    const pl = PARAM_LASER.find(x=>x.q===g.q && x.sp===sp), mac = laserPerSpessore(sp);
    if(!mac) out.push({lav:"laser", s:"✕", testo:`nessun laser taglia sp. ${g.sp}`, ko:!/AISI|EN AW/.test(g.q)});
    else if(pl) out.push({lav:"laser", s:"✓", testo:`parametri validati · ${nomeCentro("laser",pl.macc).split(" · ")[0]}`});
    else if(fatto("laser")) out.push({lav:"laser", s:"○", testo:"già tagliato, da ricontrollare"});
    else out.push({lav:"laser", s:"✕", testo:`parametri mai validati per ${g.q} sp. ${g.sp}`, chiedi:"officina", prep:"prova"});
  }
  if(usa.has("piega")){ const u = UTENSILI_PIEGA.find(x=>sp>=x.sp[0] && sp<=x.sp[1]);
    out.push(u ? {lav:"piega", s:"✓", testo:u.nota} : fatto("piega") ? {lav:"piega", s:"○", testo:"piegato in passato, da ricontrollare"} : {lav:"piega", s:"✕", testo:"nessun utensile per questo spessore", chiedi:"attrezzatura", prep:"piega"}); }
  if(usa.has("sega")){ const d = num(g.D)||num(g.a)||160; out.push(d<=300 ? {lav:"sega", s: fatto("sega")?"✓":"○", testo:`sezione nei limiti della sega (${fatto("sega")?"già tagliata":"mai tagliata: da ricontrollare"})`} : {lav:"sega", s:"✕", testo:"sezione oltre la sega", chiedi:"officina"}); }
  if(usa.has("curva")){ const sc = SET_CURVA.find(x=>x.d===num(g.D) && x.sp===sp); out.push(sc ? {lav:"curva", s:"✕", testo:sc.nota, chiedi:"attrezzatura", prep:"curva"} : {lav:"curva", s:"✕", testo:"nessun set di curvatura", chiedi:"attrezzatura", prep:"curva"}); }
  const norma = ((REQUISITI[T.key]||[]).find(q=>q.tipo==="saldatura")||{}).testo||"";
  const w = WPS.filter(x=>x.gruppo.includes(gruppoISO(g.q)) && (!sp || (sp>=x.sp[0] && sp<=x.sp[1]))).sort((a,b)=>(norma.includes(b.norma.split(" ")[1]||"#")?1:0)-(norma.includes(a.norma.split(" ")[1]||"#")?1:0))[0];
  out.push(w ? {lav:"saldatura", s:"✓", testo:`WPS ${w.id}`} : {lav:"saldatura", s:"○", testo:"nessuna WPS per questo spessore: da verificare", chiedi:"qualita"});
  return out;
}

/* ---------------- la saldatura: giunti, WPS (copertura, non uguaglianza) ---------------- */
function convertiCordone(riga){
  const s = String(riga||"").trim().toLowerCase().replace(",","."); if(!s) return null;
  let m;
  if((m = s.match(/^circ\s*ø?\s*(\d+[.]?\d*)\s*(?:[x×]\s*(\d+))?/))){ const d=+m[1], n=+(m[2]||1), mm=Math.PI*d*n; return {ok:true, a:null, n, mm, testo:`circonferenza Ø${fmt(d)} → ${fmt(Math.PI*d,0)} mm${n>1?" × "+n:""} = ${fmt(mm,0)} mm`}; }
  const ga = s.match(/^([az])\s*(\d+[.]?\d*)/); if(!ga) return {ok:false, testo:"non capito: scrivi per esempio «z5 51-152 L1016», «a4 4×120», «z3 continuo 240», «circ Ø127 ×4»"};
  const a = ga[1]==="z" ? 0.707*(+ga[2]) : +ga[2], resto = s.slice(ga[0].length).trim(), pre = `${ga[1]}${ga[2]}${ga[1]==="z"?` → a ${fmt(a,1)}`:""}`;
  if((m = resto.match(/^(\d+)\s*-\s*(\d+)\s*l\s*(\d+)/))){ const t=+m[1], passo=+m[2], L=+m[3], n=Math.floor((L-t)/passo)+1; return {ok:true, a, n, mm:n*t, testo:`${pre} · ${n} tratti da ${t} mm · ${fmt(n*t,0)} mm`}; }
  if((m = resto.match(/^(\d+)\s*-\s*(\d+)\s*[x×]\s*(\d+)/))){ return {ok:true, a, n:+m[3], mm:null, testo:`${pre} · intermittente ${m[1]}-${m[2]} × ${m[3]} · lunghezza da scrivere (L…)`}; }
  if((m = resto.match(/^continuo\s*(\d+)/))){ return {ok:true, a, n:1, mm:+m[1], testo:`${pre} · continuo ${m[1]} mm`}; }
  if((m = resto.match(/^(\d+)\s*[x×]\s*(\d+)/))){ const n=+m[1], L=+m[2]; return {ok:true, a, n, mm:n*L, testo:`${pre} · ${n} cordoni × ${L} mm = ${fmt(n*L,0)} mm`}; }
  return {ok:true, a, n:1, mm:null, testo:`${pre} · lunghezza da scrivere`};
}
function coperturaWPS(T, p, cid, f){
  const r = rigaB(T,p,cid); if(!r) return null;
  const gruppi = new Set(), sp = [];
  let zincato = false, ignoti = 0;
  for(const x of r.figli){
    const cx = x.cid ? cicloB(T,x.cid) : null, g = grezzoVal(cx);
    if(g){ gruppi.add(gruppoISO(g.q)); if(num(g.sp)) sp.push(num(g.sp)); else ignoti++; }
    else { const rr = righeB(T,p).find(y=>y.cod===x.cod); if(rr && rr.comm){ const it = COMPONENTI.find(k=>(k.usato||[]).includes(r.cod)); if(it&&it.zincato) zincato=true; } else ignoti++; }
  }
  const proc = f.p==="sald_tig"?"141":"135";
  const norma = ((REQUISITI[T.key]||[]).find(q=>q.tipo==="saldatura")||{}).testo||"";
  const copre = x => x.proc===proc && [...gruppi].every(g=>x.gruppo.includes(g)) && sp.every(s=>s>=x.sp[0] && s<=x.sp[1]);
  const w = WPS.filter(copre).sort((a,b)=>(norma.includes(b.norma.split(" ")[1]||"#")?1:0)-(norma.includes(a.norma.split(" ")[1]||"#")?1:0))[0];
  const q = w ? QUALIFICHE.find(x=>x.wps===w.id) : null;
  return {wps:w, qual:q, gruppi:[...gruppi], sp, zincato, ignoti, testo: w ? `WPS ${w.id} copre: ${proc} · gruppo ${[...gruppi].join("+")||"?"} · ${sp.length?Math.min(...sp)+"–"+Math.max(...sp)+" mm":"spessori ?"} · ${w.giunti.join("/")} · ${q?q.n+" saldatori qualificati, prima scadenza "+q.scad:"nessuna qualifica"}` : `nessuna WPS copre ${proc} · gruppo ${[...gruppi].join("+")} · ${sp.join(", ")} mm`};
}

/* ---------------- la Sequenza v16 (SPEC_v16 §4): le fasi del padre e CHI ENTRA in quale fase ----------------
   Il dato è R-10 così com'è: per ogni assieme (prodotto e sottoassiemi) le sue fasi in ordine, e per ogni fase di
   unione l'elenco «entrano» = fase_componente {cid, qta: null = tutta la quantità della BOM confermata}.
   Più fasi di unione nello stesso ciclo = pre-unioni in fila. Nessun nodo nuovo, nessun codice (DB7-7, RV-26).
   Gli stadi P · U · F · T · D · C NON si salvano: si leggono dalla lavorazione (R-12, R-32).
   La proposta è un SEME che il server scrive una volta (fonte «proposta», R-35 SEQ-06); appena Lino cambia qualcosa
   il ciclo ha fonteSeq «L» e il server non lo ripropone più. «Ripristina la proposta» rimette il seme. */
/* RL-27: D è il montaggio dopo il trattamento (R-12); l'imballo e la spedizione stanno in C */
const STADI = [["P","Preparazione dei componenti"],["U","Unione"],["F","Finitura del saldato"],["T","Trattamento esterno"],["D","Montaggio dopo il trattamento"],["C","Controllo finale, imballo e spedizione"]];
function stadioFase(f, dopoT){
  if(SALD.has(f.p) || f.p==="prigionieri") return "U";
  if(["montaggio","piantaggio","inserti"].includes(f.p)) return dopoT ? "D" : "U";
  if(["raddrizzatura","fresa"].includes(f.p)) return "F";
  if(TRATT.has(f.p) || f.p==="trattamento_termico") return "T";
  if(f.p==="controllo") return dopoT ? "C" : "F";
  if(PROC[f.p] && ["taglio","forma","mecc"].includes(PROC[f.p][2])) return "P";
  return "C";
}
const assiemiDi = (T, p) => righeB(T,p).filter(r=>r.cid && r.figli.length && !r.comm);
const unioniDi = c => c ? c.fasi.filter(f=>UNIONE.has(f.p)) : [];
/* chi entra davvero: solo i figli di QUESTO padre nella BOM confermata (un figlio tolto dall'albero sparisce da sé) */
function entranoDi(r, f){ const ok = new Set(r.figli.map(x=>x.cid).filter(Boolean)); return (f.entrano||[]).filter(e=>ok.has(e.cid)); }
/* l'unione principale: quella con più figli (a pari merito la prima saldatura). Le unioni PRIMA di lei sono pre-unioni */
function principaleDi(r, c){
  const u = unioniDi(c); if(!u.length) return null;
  let best = null, n = -1;
  for(const f of u){ const k = entranoDi(r,f).length; if(k>n || (k===n && SALD.has(f.p) && !SALD.has(best.p))){ best = f; n = k; } }
  return best;
}
function stadiCiclo(T, p, r, c){
  let vistoT = false;
  const fasi = c ? c.fasi : [], iUltT = fasi.map(f=>TRATT.has(f.p)||f.p==="trattamento_termico").lastIndexOf(true);
  const dentroT = f => entranoDi(r,f).some(e=>{ const cx = cicloB(T,e.cid); return cx && cx.fasi.some(y=>TRATT.has(y.p)); });
  /* il controllo è «finitura» solo se dopo c'è ancora un trattamento; altrimenti è il controllo finale (C) */
  return fasi.map((f,i)=>{ let st = stadioFase(f, vistoT || (UNIONE.has(f.p) && dentroT(f))); if(f.p==="controllo") st = i<iUltT ? "F" : "C"; if(st==="T") vistoT = true; return {f, i, st}; });
}
/* il riferimento proposto della fase di unione: il figlio più pesante che ci entra (◇ CALC) */
function rifProposto(T, r, ent){
  let best = null, w = -1;
  for(const e of ent){ const pg = pesoGrezzo(T,e.cid), kg = pg ? pg.kg*(e.qta||e.qtaBom||1) : 0; if(kg>w){ w = kg; best = e; } }
  return best ? {cid:best.cid, cod:best.cod, fonte:"CALC"} : null;
}
/* lettura di una fase di unione: chi entra, pre-unione, riferimento, maschera. La vista la mostra e basta (RV-28) */
function infoUnione(T, p, r, c, f){
  if(!c || !f || !UNIONE.has(f.p)) return null;
  const pr = principaleDi(r,c), i = c.fasi.indexOf(f), iP = c.fasi.indexOf(pr);
  const ent = entranoDi(r,f).map(e=>{ const x = r.figli.find(y=>y.cid===e.cid); return {cid:e.cid, cod:x.cod, descr:x.descr, qta:e.qta, qtaBom:x.qta, fonte:e.fonte, perche:e.perche||""}; });
  const rc = valF(f,"rif"), rx = rc ? r.figli.find(y=>y.cid===rc) : null;
  const rif = rx ? {cid:rc, cod:rx.cod, fonte:"L"} : rifProposto(T, r, ent);
  const st = (stadiCiclo(T,p,r,c)[i]||{}).st || "U";
  return {principale: f===pr, preUnione: f!==pr && i<iP, entrano:ent, rif, maschera: valF(f,"maschera")==="sì", N:(i+1)*10, i, stadio:st,
    nome: valF(f,"nome") || (PROC[f.p]?PROC[f.p][0]:f.p), vuota: !ent.length};
}
/* M3 · «che cosa si salda» non si scrive più: si LEGGE da «Entrano» (nessun figlio = il pezzo su se stesso) */
function oggettoSald(T, p, r, c, f){
  if(!r || !r.figli.length) return "il pezzo su se stesso";
  const iu = infoUnione(T,p,r,c,f); if(!iu || iu.vuota) return "nessun figlio entra ancora: scegli chi si salda";
  return iu.entrano.length===r.figli.filter(x=>x.cid).length ? "l'assieme: entrano tutti i figli" : `componenti sul pezzo: ${iu.entrano.length} di ${r.figli.filter(x=>x.cid).length} figli`;
}
/* «puntato prima» ha senso solo quando nella fase entra un commerciale (un dado, una vite a saldare) */
function entraCommerciale(T, p, r, c, f){ const iu = r && r.figli.length ? infoUnione(T,p,r,c,f) : null; if(!iu) return false; const righe = righeB(T,p); return iu.entrano.some(e=>(righe.find(y=>y.cid===e.cid)||{}).comm); }
/* una fase non di unione dopo una pre-unione lavora il pezzo così com'è a quel punto: «sulla pre-unione f.10» */
function suPreUnione(T, p, r, c, f){
  if(!c || !r || !r.figli.length || UNIONE.has(f.p)) return null;
  const i = c.fasi.indexOf(f);
  for(let j=i-1;j>=0;j--){ const g = c.fasi[j]; if(UNIONE.has(g.p)){ const iu = infoUnione(T,p,r,c,g); return iu && iu.preUnione ? {fid:g.id, N:iu.N} : null; } }
  return null;
}
/* il SEME della proposta (SEQ-06, R-35): chi entra in quale unione. Si scrive quando il ciclo dell'assieme nasce
   (ciclo.fonte_sequenza vuota) e si RICALCOLA uguale quando Lino chiede «Ripristina la proposta» (P-09: niente
   fotografia del seme; basta la colonna ciclo.fonte_sequenza = proposta | scritta, in R-10). */
function seminaEntrano(T, p, r, c){
  const u = unioniDi(c), iT = c.fasi.findIndex(f=>TRATT.has(f.p)), righe = righeB(T,p);
  const main = u.find(f=>SALD.has(f.p)) || u[0];
  for(const f of u) f.entrano = [];
  for(const x of r.figli){
    if(!x.cid) continue;
    const rr = righe.find(y=>y.cod===x.cod) || {};
    let dove = main, perche = "entra nell'unione principale";
    if(rr.comm){ const pg = u.find(f=>f.p==="prigionieri"); if(pg){ dove = pg; perche = "dado saldato sul pezzo unito (SEQ-06)"; } else perche = "commerciale nell'unione (SEQ-06)"; }
    if(/perno/i.test(x.descr||"")){ const d = u.find(f=>iT>=0 && c.fasi.indexOf(f)>iT) || u.find(f=>["piantaggio","montaggio"].includes(f.p)); if(d){ dove = d; perche = "perno da piantare dopo il trattamento (SEQ-06)"; } }
    if(dove && !dove.entrano.some(e=>e.cid===x.cid)) dove.entrano.push({cid:x.cid, qta:null, fonte:"proposta", perche});
  }
}
function semeSequenza(T, p, r, c){
  if(!r || !c || c.fonteSeq || !r.figli.length) return;
  seminaEntrano(T, p, r, c);
  c.fonteSeq = "proposta";
}
/* le unioni che la proposta prevede per questo assieme: quelle del seme o del modello, nell'ordine (R-35) */
function unioniProposte(r, c){
  const s = SEMI[r.cod];
  const fasi = s && s.fasi && !s.copia ? s.fasi.map(x=>x.p) : ((MODELLI[c.modello]||{}).fasi||[]);
  const u = fasi.filter(x=>UNIONE.has(x));
  return u.length ? u : (unioniDi(c)[0] ? [unioniDi(c)[0].p] : ["sald_mag"]);
}
/* il PIANO del ripristino (lettura, non scrive): che cosa si toglie, che cosa resta aperto. La domanda lo mostra prima del
   gesto (A1 di Lino): le fasi di unione in più si tolgono; una fase che lavorava su una pre-unione tolta si toglie con lei
   (mai una foratura prima della saldatura); una preparazione con una richiesta aperta resta, segnata PREP-04. */
function pianoRipristino(T, p){
  const out = {assiemi:[], togli:[], aperte:[]};
  for(const r of assiemiDi(T,p)){
    const c = cicloB(T,r.cid); if(!c || c.fonteSeq!=="L") continue;
    const voglio = unioniProposte(r, c).slice(), tieni = new Set(), via = new Set();
    /* ogni unione proposta resta sull'unione dello stesso tipo nata per prima (l'id più vecchio): le aggiunte a mano si tolgono */
    const nId = f => +String(f.id).replace(/\D/g,"");
    for(const f of c.fasi.filter(x=>UNIONE.has(x.p)).sort((a,b)=>nId(a)-nId(b))){ const j = voglio.indexOf(f.p); if(j>=0){ voglio.splice(j,1); tieni.add(f.id); } else via.add(f.id); }
    /* le fasi non di unione che lavoravano su una pre-unione tolta */
    for(const f of c.fasi){ if(UNIONE.has(f.p)) continue; const su = suPreUnione(T,p,r,c,f); if(su && via.has(su.fid)) via.add(f.id); }
    const nome = f => `f.${(c.fasi.indexOf(f)+1)*10} ${valF(f,"nome")||PROC[f.p][0]}`;
    c.fasi.filter(f=>via.has(f.id)).forEach(f=>out.togli.push({cid:r.cid, cod:r.cod, fid:f.id, testo: nome(f) + (UNIONE.has(f.p) ? "" : " (lavorava sulla pre-unione)")}));
    for(const [key,v] of prepVoci(c)) if(v.on && via.has(v.nata)){ const rq = richiesteDellaPrep(T, r.cid, key).filter(q=>q.stato!=="annullata");
      if(rq.length) out.aperte.push({cid:r.cid, cod:r.cod, key, testo:`${rq.map(q=>q.id).join(", ")} ${nomePrep(v.k).toLowerCase()} (${rq.map(q=>q.forn?q.forn.nome:q.dest.nome).join(", ")})`}); }
    out.assiemi.push({r, c, via, manca: voglio});
  }
  return out;
}
function applicaRipristino(T, p, piano){
  for(const {r, c, via, manca} of piano.assiemi){
    for(const [key,v] of prepVoci(c)) if(via.has(v.nata) && !richiesteDellaPrep(T, r.cid, key).some(q=>q.stato!=="annullata")) delete c.prep[key];
    c.fasi = c.fasi.filter(f=>!via.has(f.id));
    /* un'unione della proposta che Lino aveva tolto torna, prima della prima fase che non è di preparazione */
    for(const pp of manca){ const S2 = a2Di(T), f = nuovaFase(S2, pp, null); proposteGeneriche(T,p,r,c,f);
      const sc = stadiCiclo(T,p,r,c), i = sc.findIndex(x=>x.st!=="P"); c.fasi.splice(i<0 ? c.fasi.length : i, 0, f); }
    for(const f of c.fasi) if(UNIONE.has(f.p)) for(const k of ["nome","rif"]) delete f.par[k];
    for(const f of c.fasi) delete f.par.proteggi;
    seminaEntrano(T, p, r, c);
    c.fonteSeq = "proposta"; if(c.stato==="confermato"){ c.stato = "bozza"; c.riaperto = "sequenza ripristinata: da riconfermare"; }
  }
  const S2 = a2Di(T); if(S2.seq[p.rif] && S2.seq[p.rif].stato==="confermata") S2.seq[p.rif] = Object.assign({}, S2.seq[p.rif], {stato:"bozza", riaperta:"proposta ripristinata: da riconfermare"});
}
/* un gesto della Sequenza: il ciclo dell'assieme è «scritto da Lino» (fonte_sequenza = scritta); le sequenze
   confermate dei prodotti che contengono questo assieme tornano da confermare (solo quelle: A2) */
function seqScritta(T, c){
  c.fonteSeq = "L"; const S2 = a2Di(T);
  for(const p of (T.esito.prodotti_valutati||[])){ const sq = S2.seq[p.rif]; if(!sq || sq.stato!=="confermata") continue;
    if(assiemiDi(T,p).some(r=>cicloB(T,r.cid)===c)) S2.seq[p.rif] = Object.assign({}, sq, {stato:"bozza", riaperta:"modificata dopo la conferma: da riconfermare"}); }
}
function fonteSequenza(T, p){
  const aa = assiemiDi(T,p).map(r=>cicloB(T,r.cid)).filter(Boolean);
  if(!aa.length) return "vuota";
  return aa.some(c=>c.fonteSeq==="L") ? "L" : "proposta";
}
/* P-06 · la firma del ciclo letto: fasi, campi accettati, proposte ◇ ancora aperte e grezzo. «Conferma» la manda; se nel
   frattempo qualcosa è cambiato (un altro utente, una regola nuova) il server rifiuta: niente si accetta senza vederlo */
function firmaCiclo(T, cid){
  const c = cicloB(T,cid); if(!c) return "";
  return impronta(JSON.stringify([c.stato, c.fasi.map(f=>[f.id, f.p, f.par, Object.keys(f.prop).filter(k=>propBlocco(f,k)).map(k=>[k, f.prop[k][0]])]), c.grezzo ? [c.grezzo.par, c.grezzo.prop] : null]));
}
/* le proposte ◇ che «Accetta N ◇ · conferma» accetterebbe: l'elenco si mostra PRIMA del gesto */
function proposteCiclo(T, cid){
  const c = cicloB(T,cid); if(!c) return [];
  const out = c.fasi.flatMap((f,i)=>Object.keys(f.prop).filter(k=>propBlocco(f,k)).map(k=>`f.${(i+1)*10} ${(campiDi(f).find(d=>d[0]===k)||[k,k])[1].toLowerCase()} ${f.prop[k][0]}`));
  if(c.grezzo && Object.keys(c.grezzo.prop).length && !grezzoAccettato(c) && c.fasi.some(f=>CON_GREZZO.has(f.p))) out.unshift(`grezzo ${testoGrezzo(grezzoVal({grezzo:{par:{}, prop:c.grezzo.prop}}))}`);
  return out;
}
/* la firma di quello che si è letto: ordine, nomi, chi entra, riferimento, da proteggere di tutti i cicli degli assiemi */
function impronta(s){ let h = 5381; for(let i=0;i<s.length;i++) h = ((h<<5)+h+s.charCodeAt(i))>>>0; return "seq-"+h.toString(16); }
function firmaSequenza(T, p){
  return impronta(JSON.stringify(assiemiDi(T,p).map(r=>{ const c = cicloB(T,r.cid);
    return [r.cid, c ? c.fasi.map(f=>[f.id, f.p, f.par.nome||"", f.par.rif||"", f.par.proteggi||"", entranoDi(r,f).map(e=>e.cid+":"+(e.qta==null?"":e.qta)).sort().join(",")]) : null]; })));
}
/* gli stadi letti e i viaggi del prodotto (pannello del prodotto, Esito). Lettura. */
function stadiDi(T, p){
  const r = righeB(T,p).find(x=>x.radice); if(!r) return null;
  const c = r.cid ? cicloB(T,r.cid) : null;
  const viaggi = [];
  for(const x of [r, ...righeB(T,p).filter(y=>!y.radice)]){
    const cx = x.cid ? cicloB(T,x.cid) : null; if(!cx) continue;
    cx.fasi.forEach(f=>{ if(PROC[f.p] && PROC[f.p][1]==="est" && f.p!=="trasporto"){ const t = valF(f,"terzi");
      viaggi.push({cod:x.cod, cid:x.cid, f, chi: t ? t.split(",").map(k=>terz(k.trim()).nome).join(", ") : "terzista da scegliere",
        trasporto: cx.fasi.some(y=>y.p==="trasporto") ? "trasporto nel ciclo" : (valF(f,"conto")==="conto lavoro" || TRATT.has(f.p) ? "andata e ritorno proposti ◇" : "il fornitore consegna")}); } });
  }
  return {root:r, ciclo:c, fasi: stadiCiclo(T,p,r,c), viaggi, rif: (()=>{ const pr = c ? principaleDi(r,c) : null; const iu = pr ? infoUnione(T,p,r,c,pr) : null; return iu && iu.rif ? iu.rif.cod : null; })(),
    clienti: righeB(T,p).filter(x=>x.cid && (comeDi(T,x.cid)||{}).modo==="cliente").length};
}

/* ---------------- l'albero del processo (SPEC_v16 §4.3, §4.6): una LETTURA delle tabelle di R-10 (R-37) ----------------
   Nodi: prodotto · unione (rombo, mai una casella: non è un nodo della BOM) · componente (foglia o sottoassieme) · senza_fase.
   Chiavi: SOLO componente.id e fase.id, mai il Rif né <gruppo>|<rif> (FB §7.3). Gli stati li dà il server, con la parola. */
function faseLettura(T, c, f){
  const est = PROC[f.p] && PROC[f.p][1]==="est", t = valF(f,"terzi");
  return {fid:f.id, N:(c.fasi.indexOf(f)+1)*10, sigla:siglaDi(f.p), nome: valF(f,"nome") || (PROC[f.p]?PROC[f.p][0]:f.p), p:f.p, est,
    terzista: est ? (t ? nomiTerzi(t) : "") : "", trasporto: est && f.p!=="trasporto" ? (c.fasi.some(y=>y.p==="trasporto") ? "trasporto nel ciclo" : "trasporto proposto ◇") : "",
    proteggi: valF(f,"proteggi") || ""};
}
function prepDellaFase(c, fid){ return prepVoci(c).filter(([,v])=>v.on && v.nata===fid).map(([key,v])=>({key, k:v.k, nome: v.k==="altra" ? (v.nome||"altra") : nomePrep(v.k), stato: statoPrepParola(v), costo:v.costo||"", settimane:v.settimane||""})); }
function statoPrepParola(v){ return v.stato==="esiste già" ? "esiste già" : (v.costo!==""&&v.costo!=null) ? "quotata" : "da quotare"; }
function alberoProcesso(T, p){
  const righe = righeB(T,p), root = righe.find(r=>r.radice); if(!root || !root.cid) return null;
  const ctrl = controlliB(T,p), sq = seqDi(T,p), conf = sq.stato==="confermata" && sq.firma===firmaSequenza(T,p);
  const padriDi = cod => righe.filter(r=>r.figli.some(x=>x.cod===cod));
  const motiviDi = (pred) => ctrl.filter(pred).filter(x=>x.liv!=="AV" || /^(SEQ|PREP)/.test(x.cod)).map(x=>({cod:x.cod, liv:x.liv, testo:x.testo, azione:x.azione}));
  const statoComp = (rr, c, mot) => mot.some(x=>x.liv!=="AV") ? "manca" : rr.comm || ["cliente","escluso"].includes((comeDi(T,rr.cid)||{}).modo) ? "ok" : !c ? "manca" : c.stato==="confermato" ? "ok" : "bozza";
  function nodoAssieme(r, prof){
    const c = cicloB(T,r.cid), sc = stadiCiclo(T,p,r,c), u = unioniDi(c);
    const usati = new Set(u.flatMap(f=>entranoDi(r,f).map(e=>e.cid)));
    const leaf = (x, e, f) => nodoComponente(x, r, e, prof+1);
    /* la catena delle unioni: l'ultima sta in alto, ognuna porta sotto di sé quella di prima (pre-unioni in fila) */
    let sotto = null;
    u.forEach((f, k)=>{
      const iu = infoUnione(T,p,r,c,f), i = c.fasi.indexOf(f);
      const prossima = u[k+1], iN = prossima ? c.fasi.indexOf(prossima) : c.fasi.length;
      const tra = c.fasi.slice(i+1, iN).filter(()=>!!prossima).map(g=>Object.assign(faseLettura(T,c,g), {st:(sc[c.fasi.indexOf(g)]||{}).st}));
      const prima = k===0 ? c.fasi.slice(0, i).map(g=>Object.assign(faseLettura(T,c,g), {st:(sc[c.fasi.indexOf(g)]||{}).st})) : [];
      const mot = motiviDi(x=>x.fid===f.id && /^(SEQ|PREP)/.test(x.cod));
      /* SEQ-02: sul ramo che sale da questa unione c'è un trattamento (o una fase esterna) senza terzista */
      const seguenti = c.fasi.slice(i+1, iN).filter(g=>PROC[g.p] && PROC[g.p][1]==="est" && g.p!=="trasporto" && !valF(g,"terzi"));
      if(seguenti.length) mot.push({cod:"SEQ-02", liv:"DB", testo:`${seguenti.map(g=>`f.${(c.fasi.indexOf(g)+1)*10} ${PROC[g.p][0].toLowerCase()}`).join(", ")}: terzista da scegliere`, azione:{pannello:{tipo:"stadio", cid:r.cid, fid:seguenti[0].id}}});
      const figli = [...(sotto?[sotto]:[]), ...iu.entrano.map(e=>leaf(r.figli.find(y=>y.cid===e.cid), e, f)).filter(Boolean)];
      /* B6: un'unione appena nata, vuota, non è «da sistemare»: dice «scegli chi entra» */
      const stato = mot.some(x=>x.liv!=="AV") ? "manca" : iu.vuota ? "vuota" : conf ? "ok" : (c.fonteSeq==="proposta" && iu.entrano.every(e=>e.fonte==="proposta")) ? "proposta" : "bozza";
      sotto = {tipo:"unione", cid:r.cid, fid:f.id, N:iu.N, nome:iu.nome, p:f.p, stadio:iu.stadio, preUnione:iu.preUnione, principale:iu.principale, maschera:iu.maschera,
        rif: iu.rif, tra, prima, prep: prepDellaFase(c, f.id), stato, motivi: mot.concat(iu.vuota?[{cod:"SEQ-09", liv:"AV", testo:"scegli chi entra", azione:{pannello:{tipo:"unione", cid:r.cid, fid:f.id}}}]:[]), figli};
    });
    const iUlt = u.length ? c.fasi.indexOf(u[u.length-1]) : -1;
    const dopo = c ? c.fasi.slice(iUlt+1).map(g=>Object.assign(faseLettura(T,c,g), {st:(sc[c.fasi.indexOf(g)]||{}).st})) : [];
    const senza = r.figli.filter(x=>x.cid && !usati.has(x.cid));
    const nodoSenza = senza.length ? {tipo:"senza_fase", cid:r.cid, cods: senza.map(x=>x.cod), stato:"manca",
      motivi:[{cod:"SEQ-01", liv:"DB", testo:`non ${senza.length>1?"entrano":"entra"} in nessuna fase: ${senza.map(x=>x.cod).join(", ")}`, azione:{pannello:{tipo:"assieme", cid:r.cid}}}],
      figli: senza.map(x=>nodoComponente(x, r, null, prof+1))} : null;
    return {sotto, dopo, nodoSenza, c};
  }
  function nodoComponente(x, padre, e, prof){
    const rr = righe.find(y=>y.cod===x.cod); if(!rr) return null;
    const c = rr.cid ? cicloB(T,rr.cid) : null, modo = rr.cid ? (comeDi(T,rr.cid)||{}).modo : null;
    const altri = padriDi(rr.cod).filter(y=>y.cod!==padre.cod).map(y=>y.cod);
    const mot = motiviDi(y=>y.cid===rr.cid && !y.fid && /^(CIC-01|COME-01|SEQ|PREP)/.test(y.cod));
    const n = {tipo:"componente", cid:rr.cid, cod:rr.cod, descr:rr.descr, qta: e && e.qta!=null ? e.qta : x.qta, qtaBom:x.qta, diviso: !!(e && e.qta!=null), fonte: e ? e.fonte : null, perche: e ? e.perche : "",
      comm: !!rr.comm, cliente: modo==="cliente", modo, padre: padre.cid, anche: altri, fasi: c ? c.fasi.map(f=>faseLettura(T,c,f)) : [],
      prep: c ? prepVoci(c).filter(([,v])=>v.on).map(([key,v])=>({key, k:v.k, nome:v.k==="altra"?(v.nome||"altra"):nomePrep(v.k), stato:statoPrepParola(v), fid:v.nata||null})) : [],
      motivi: mot, stato: statoComp(rr, c, mot), figli: []};
    if(rr.figli.length && prof<6){ const a = nodoAssieme(rr, prof); n.sottoassieme = true; n.dopo = a.dopo; n.figli = [a.sotto, a.nodoSenza].filter(Boolean);
      n.fasi = a.dopo; if(n.figli.some(y=>y.stato==="manca")) n.statoSotto = "manca"; }
    return n;
  }
  const a = nodoAssieme(root, 0);
  const mot = motiviDi(x=>x.cid===root.cid && !x.fid && /^(SEQ|CIC-01)/.test(x.cod));
  return {tipo:"prodotto", cid:root.cid, cod:root.cod, descr:root.descr, ultime:a.dopo, fonte: fonteSequenza(T,p), confermata: conf,
    stato: mot.some(x=>x.liv!=="AV") || (!a.sotto && !a.nodoSenza) ? "manca" : conf ? "ok" : fonteSequenza(T,p)==="proposta" ? "proposta" : "bozza",
    motivi: mot.concat(!a.c ? [{cod:"CIC-01", liv:"DB", testo:`${root.cod} non ha ancora un ciclo: prima ② Come e ③ Ciclo`, azione:{staz:2}}] : !unioniDi(a.c).length ? [{cod:"SEQ-01", liv:"KO", testo:`il ciclo di ${root.cod} non ha una fase di unione`, azione:{staz:3, cid:root.cid}}] : []),
    figli: [a.sotto, a.nodoSenza].filter(Boolean)};
}
/* quanti nodi da sistemare (⚠) nell'albero: la barra della stazione lo legge */
function daSistemare(n){ if(!n) return 0; return (n.stato==="manca"?1:0) + (n.figli||[]).reduce((a,x)=>a+daSistemare(x),0); }

/* ---------------- le preparazioni (R-11 precisata, PREPARAZIONI §5.1): una riga per preparazione, legata alla fase ----------------
   La chiave è il tipo, oppure «tipo@fase» quando lo stesso tipo nasce da due fasi (due maschere: pre-unione e assieme). */
function prepVoci(c){ return Object.entries((c&&c.prep)||{}).map(([key,v])=>{ if(!v.k) v.k = key.split("@")[0]; return [key,v]; }); }
function chiavePrep(T, c, k, fid){
  const voci = prepVoci(c).filter(([,v])=>v.k===k), cid = Object.keys(a2Di(T).cicli).find(x=>a2Di(T).cicli[x]===c);
  const stessa = voci.find(([,v])=>fid && v.nata===fid); if(stessa) return stessa[0];
  const libera = voci.find(([key,v])=>!v.nata || (!v.on && !richiesteDellaPrep(T, cid, key).length)); if(libera) return libera[0];
  return voci.length ? `${k}@${fid||voci.length+1}` : k;
}
function accendiPrep(T, r, c, k, fid, fonte){
  const key = chiavePrep(T, c, k, fid);
  const v = c.prep[key] = Object.assign(c.prep[key] || {costo:"", settimane:"", stato:"da fare"}, {k, on:true, nata: fid || (c.prep[key]||{}).nata || null, proposta:false});
  if(!v.fonte) v.fonte = fonte || "PREP-03";
  const st = r ? storicoPrep(r.cod, k) : null;
  if(st && !v.storico) v.storico = {rif:st.rif, rfq:st.rfq};   /* «esiste già»: la proposta dallo storico, non ancora scelta */
  return key;
}
/* tutte le preparazioni accese della RFQ (C1 le legge in Esito e Offerta) */
function preparazioniRFQ(T){
  const out = [], visti = new Set();
  for(const p of (T.esito ? T.esito.prodotti_valutati : [])) for(const r of righeB(T,p)){
    if(!r.cid || visti.has(r.cid)) continue; visti.add(r.cid);
    const c = cicloB(T,r.cid); if(!c) continue;
    for(const [key,v] of prepVoci(c)){ if(!v.on) continue;
      const i = c.fasi.findIndex(f=>f.id===v.nata), f = i>=0 ? c.fasi[i] : null, iu = f ? infoUnione(T,p,r,c,f) : null;
      const q = richiesteDellaPrep(T, r.cid, key).filter(x=>x.stato!=="annullata")[0] || null;
      out.push({cid:r.cid, cod:r.cod, key, k:v.k, nome: v.k==="altra" ? (v.nome||"altra") : nomePrep(v.k), fid:v.nata||null, fase: i>=0 ? `f.${(i+1)*10}${iu&&iu.preUnione?" pre-unione":""}` : "",
        orfana: !!v.nata && i<0,   /* PREP-04: la sua fase non c'è più (ripristino della proposta) */
        stato: v.stato||"da fare", costo: v.costo||"", settimane: v.settimane||"", fonte: v.fonte||"", chi: v.chi||"", riferimento: v.riferimento||"",
        richiesta: q ? {id:q.id, stato:q.stato} : null});
    }
  }
  return out;
}
function attrezzaturaPronta(T, p){
  const el = preparazioniRFQ(T).filter(x=>!p || righeB(T,p).some(r=>r.cid===x.cid));
  /* una preparazione orfana (PREP-04) non conta come «senza costo»: è un avviso che apre la sua richiesta */
  const senza = el.filter(x=>!x.orfana && x.stato!=="esiste già" && (x.costo===""||x.costo==null));
  return {ok: !senza.length, senzaCosto: senza.length, elenco: el, settimaneMax: el.reduce((a,x)=>Math.max(a, num(x.settimane)||0), 0)};
}

/* ---------------- i campi della fase: dato accettato, proposta, completezza ---------------- */
function valF(f, k){ return f.par[k]!=null && f.par[k]!=="" ? f.par[k] : null; }
function propF(f, k){ return f.prop[k] && f.prop[k][0]!=="" && f.par[k]==null ? f.prop[k] : null; }
/* i terzisti non si accettano mai in blocco (né «Accetta ◇ (n)», né Ctrl+Invio, né «Accetta e conferma»):
   il suggerimento resta un pulsante non premuto, la scelta è un gesto esplicito di Lino (nessuno preselezionato) */
const SCELTA_ESPLICITA = new Set(["terzi"]);
function propBlocco(f, k){ return SCELTA_ESPLICITA.has(k) ? null : propF(f, k); }
function campiDi(f){ return CAMPI_FAM[f.p] || [[T_,"Tempo","",1,"t"]]; }
/* il tempo: «20 h», «40 min», «25 pz/h», «1,5 pz/h» → pezzi l'ora e minuti al pezzo [FE: conversione d'unità] */
function leggiTempo(s){
  const t = String(s||"").trim().toLowerCase().replace(",","."); if(!t) return null;
  let m;
  if((m=t.match(/^(\d+(?:\.\d+)?)\s*pz\s*\/\s*h$/))) return {pzh:+m[1], min:60/+m[1], testo:`${fmt(60/+m[1],2)} min/pz`};
  if((m=t.match(/^(\d+(?:\.\d+)?)\s*h$/))) return {pzh:1/+m[1], min:+m[1]*60, testo:`${fmt(+m[1]*60,0)} min/pz`};
  if((m=t.match(/^(\d+(?:\.\d+)?)\s*min$/))) return {pzh:60/+m[1], min:+m[1], testo:`${fmt(60/+m[1],2)} pz/h`};
  return {errore:true, testo:"scrivi «20 h», «40 min» o «25 pz/h»"};
}
function statoFase(T, p, cid, f){
  const campi = campiDi(f), stelle = campi.filter(x=>x[3]);
  const ha = k => valF(f,k)!=null;
  const tempo = ha(T_) && !leggiTempo(valF(f,T_)).errore;
  const driver = stelle.filter(x=>x[0]!==T_);
  let fatte = 0;
  for(const x of stelle){ if(ha(x[0]) || (x[0]!==T_ && tempo)) fatte++; }
  const proposte = Object.keys(f.prop).filter(k=>propBlocco(f,k)).length;
  const manca = stelle.filter(x=>!ha(x[0]) && !(x[0]!==T_ && tempo)).map(x=>x[1].toLowerCase());
  const soloTempo = tempo && driver.length && driver.some(x=>!ha(x[0]));
  return {stelle:stelle.length, fatte, proposte, manca, soloTempo, completa: !manca.length};
}

/* ---------------- REGOLE: i controlli automatici (KO solo per l'impossibile; mai KO da dati inventati) ---------------- */
function controlliB(T, p){
  if(!p) return [];
  const S2 = a2Di(T), righe = righeB(T,p), out = [], IX = indiceDi(T);
  const add = (cod, liv, r, testo, azione, extra) => out.push(Object.assign({cod, liv, cid:r?r.cid:null, rcod:r?r.cod:"", testo, azione:azione||null}, extra||{}));
  const radice = righe.find(r=>r.radice);
  for(const r of righe){
    if(!r.cid) continue;
    const modo = modoEffettivo(T,p,r), c = cicloB(T,r.cid);
    const v = IX.Voce(p.rif, r.cid, "disegno_2d");
    /* P-07: la conferma della minuteria è della zona C (R-08, spenta anche qui): DOC-01 pesa sempre */
    if(v && v.esito!=="presente") add("DOC-01","DB", r, `${r.cod}: nessun 2D confermato${r.comm?" (commerciale: il 2D si chiede comunque; la conferma della minuteria arriva con R-08)":""}`, r.pdf?{l:"Conferma il documento",staz:1,vista:"documenti"}:{l:"Chiedi al cliente",richiesta:"cliente"}, r.comm ? {minuteria:true} : null);
    const pr = r.pdf && Object.values(T.allegati).find(a=>a.nome===r.pdf);
    if(pr && pr.disponibilita==="senza_testo") add("DOC-02","AV",r,`${r.cod}: PDF senza testo, il cartiglio va letto a mano`,null);
    if(!modo){ add("COME-01","DB",r,`${r.cod}: come si fa non è deciso`,{l:"Decidi in Come",staz:2}); continue; }
    if(modo==="noquote") add("NQ-01","KO",r,`${r.cod}: no quote · ${(comeDi(T,r.cid)||{}).testo||"motivo da scrivere"}`,{l:"Rivedi in Come",staz:2});
    if(!MODO_CON_CICLO.has(modo)) continue;
    if(!c || !c.fasi.length){ add("CIC-01","DB",r,`${r.cod}: ciclo vuoto`,{l:"Apri nel Ciclo",staz:3,cid:r.cid}); continue; }
    if(c.stato!=="confermato") add("CIC-02","AV",r,`${r.cod}: ciclo in bozza, non ancora confermato`,{l:"Apri nel Ciclo",staz:3,cid:r.cid});
    /* grezzo */
    const g = grezzoVal(c), primaG = c.fasi.find(f=>CON_GREZZO.has(f.p));
    if(primaG && !g) add("MAT-00","DB",r,`${r.cod}: grezzo da scrivere nella fase ${(c.fasi.indexOf(primaG)+1)*10}`,{l:"Scrivi il grezzo",staz:3,cid:r.cid,fid:primaG.id});
    if(g){
      const sg = statoGrezzo(T,p,r.cid), rr = richiesteDi(T,r.cid).filter(q=>/^mp_/.test(q.tipo) && q.stato!=="annullata"), rich = rr[0] || null;
      if(sg.stato==="mai" && !rich) add("MAT-02","DB",r,`${r.cod}: ${testoGrezzo(g)} mai acquistato`,{l:"Chiedi ai fornitori di materia prima",richiesta:"mp_mai",cid:r.cid,fid:primaG&&primaG.id},{prova:true});
      if(sg.stato==="non_basta" && !rich) add("GRZ-02","DB",r,`${r.cod}: ${sg.testo}`,{l:"Chiedi prezzo e tempi",richiesta:"mp_prezzo",cid:r.cid,fid:primaG&&primaG.id},{prova:true});
      const offerta = rr.some(q=>q.stato==="offerta_ricevuta");
      if(!offerta && rr.some(q=>q.stato==="inviata")) add("MAT-03","DB",r,`${r.cod}: richiesta di materia prima inviata il ${rr.find(q=>q.stato==="inviata").inviata}, senza risposta (scade il ${rr[0].scad})`,{l:"Apri la richiesta",cassetto:"richieste"});
      if(rr.length && rr.every(q=>q.stato==="declinata")) add("MAT-04","DB",r,`${r.cod}: tutti i fornitori hanno declinato · KO oppure «si ottiene diversamente»`,{l:"Apri la richiesta",cassetto:"richieste"});
      if(!offerta && rr.some(q=>q.stato==="scaduta") && !rr.some(q=>q.stato==="inviata")) add("MAT-07","DB",r,`${r.cod}: richiesta scaduta senza risposta · si quota con riserva`,{l:"Apri la richiesta",cassetto:"richieste"});
      const pa = pesoAnteprima(T,r);
      if(pa && pa.pes01) add("PES-01","AV",r,`${r.cod}: grezzo ${fmt(pa.grezzo,2)} kg < finito ${fmt(pa.netto,2)} kg: controlla le misure`,{l:"Correggi il grezzo",staz:3,cid:r.cid,fid:primaG&&primaG.id});
      for(const k of capacitaMateriale(T,r.cid)) if(k.s==="✕") add(k.lav==="laser"?"LAS-03":k.lav==="piega"?"PIE-04":k.lav==="curva"?"CUR-01":"CAP-01", k.ko?"KO":"DB", r, `${r.cod}: ${k.lav} ✕ ${k.testo}`, k.chiedi?{l:k.prep?`Richiedi: ${nomePrep(k.prep).toLowerCase()}`:k.chiedi==="qualita"?"Chiedi alla qualità":"Chiedi all'officina",richiesta:k.chiedi,cid:r.cid,fid:primaG&&primaG.id}:null, {inventato:true});
    }
    /* fasi */
    c.fasi.forEach((f,i)=>{
      const sf = statoFase(T,p,r.cid,f), N = (i+1)*10, nome = PROC[f.p]?PROC[f.p][0]:f.p, az = {l:"Apri la fase",staz:3,cid:r.cid,fid:f.id};
      if(sf.manca.length) add("FAS-01","DB",r,`${r.cod} f.${N} ${nome}: manca ${sf.manca.join(", ")}`,az,{fid:f.id});
      if(sf.soloTempo) add(SALD.has(f.p)?"SAL-15":"FAS-04","AV",r,`${r.cod} f.${N} ${nome}: c'è il tempo, manca il driver per Franco`,az,{fid:f.id});
      if(PROC[f.p] && PROC[f.p][1]==="est" && f.p!=="trasporto"){ const t = valF(f,"terzi"); const n = t ? t.split(",").filter(Boolean).length : 0;
        if(n<2) add(TRATT.has(f.p)?"TRT-08":"FAS-03", n?"AV":"DB", r, `${r.cod} f.${N} ${nome}: ${n?"un terzista solo":"nessun terzista scelto"}: scegli 2–3 terzisti${propF(f,"terzi")?" (◇ suggerito: "+nomiTerzi(propF(f,"terzi")[0])+")":""}`, {l:"Richiesta al terzista",richiesta:TRATT.has(f.p)?"verniciatori":"terzista",cid:r.cid,fid:f.id},{fid:f.id}); }
      if(f.p==="laser"){ const sp = num((g||{}).sp); if(sp && !laserPerSpessore(sp)) add("LAS-01","KO",r,`${r.cod} f.${N}: sp. ${sp} oltre ogni laser`,az,{fid:f.id}); }
      /* RL-08: MAT-05 sulle quotazioni registrate della fase: DUBBIO se restano solo quotazioni con il materiale discorde */
      if(PROC[f.p] && PROC[f.p][1]==="est" && f.p!=="trasporto"){ const mc = materialeComp(T, r.cid), qq = QUOTAZIONI_B.filter(x=>x.tk===T.key && x.cod===r.cod && x.p===f.p);
        const dis = mc ? qq.filter(x=>matDiscorde(x.materiale, mc.q)) : [];
        if(dis.length) add("MAT-05", dis.length===qq.length?"DB":"AV", r, `${r.cod} f.${N}: ${[...new Set(dis.map(x=>terz(x.terz).nome))].join(", ")} dichiara «${dis[0].materiale}» ≠ disegno ${mc.q}${dis.length===qq.length?": resta solo una quotazione non confrontabile":": esclusa dal confronto, si usano le altre"}`,{l:"Apri la fase",staz:3,cid:r.cid,fid:f.id},{fid:f.id}); }
      if(SALD.has(f.p)){
        if(valF(f,"maschera")==="sì" && !prepVoci(c).some(([,v])=>v.k==="maschera" && v.on && v.nata===f.id)) add("PREP-01","AV",r,`${r.cod} f.${N}: in maschera senza la preparazione`,az,{fid:f.id});
        const cw = coperturaWPS(T,p,r.cid,f); if(cw && !cw.wps) add("SAL-01","DB",r,`${r.cod} f.${N}: ${cw.testo}`,{l:"Chiedi alla qualità",richiesta:"qualita",cid:r.cid,fid:f.id},{fid:f.id,inventato:true});
        if(cw && cw.zincato) add("SAL-12","AV",r,`${r.cod} f.${N}: componente zincato saldato (dado M14): WPS dedicata, aspirazione`,{l:"Chiedi alla qualità",richiesta:"qualita",cid:r.cid,fid:f.id},{fid:f.id});
        const ul = ULTIMA[r.cod]; if(ul && valF(f,T_)) add("STO-01","AV",r,`${r.cod} f.${N}: ultima volta preventivo ${ul.prev}, consuntivo ${ul.cons} (+50 %)`,az,{fid:f.id});
        if(r.figli.some(x=>{ const rr = righe.find(y=>y.cod===x.cod); return rr && rr.comm; }) && c.fasi.slice(i+1).some(y=>TRATT.has(y.p))) add("SAL-08","AV",r,`${r.cod} f.${N}: dadi saldati e trattamento dopo: filetti da mascherare`,{l:"Apri la Sequenza",staz:4},{fid:f.id});
      }
    });
    /* PREP-01: ogni preparazione accesa ha un costo o «esiste già»; PREP-02: la sua richiesta è scaduta senza risposta;
       PREP-04 (avviso): la sua fase non c'è più (ripristino della proposta) ma una richiesta è aperta: si apre la richiesta */
    for(const [key,v] of prepVoci(c)){ if(!v.on) continue; const i = c.fasi.findIndex(f=>f.id===v.nata), dove = i>=0 ? ` (f.${(i+1)*10})` : "";
      const qq = richiesteDellaPrep(T, r.cid, key).filter(x=>x.stato!=="annullata"), q = qq[0] || null;
      if(v.nata && i<0){ add("PREP-04","AV",r,`${r.cod}: ${nomePrep(v.k).toLowerCase()}: la sua fase non c'è più${q?`, ma la richiesta ${q.id} è aperta`:""}`,{l:"Apri la richiesta",cassetto:"richieste"},{prep:key}); continue; }
      if(qq.length && qq.every(x=>x.stato==="scaduta") && !v.costo && v.stato!=="esiste già") add("PREP-02","DB",r,`${r.cod}: ${nomePrep(v.k).toLowerCase()}${dove}: richiesta scaduta senza risposta · si quota con riserva`,{l:"Apri la richiesta",cassetto:"richieste"},{fid:v.nata||null, prep:key});
      else if(!v.costo && v.stato!=="esiste già") add("PREP-01","AV",r,`${r.cod}: ${v.k==="altra"?(v.nome||"preparazione"):nomePrep(v.k).toLowerCase()}${dove} senza costo né «esiste già»`,{l:"Apri le preparazioni",staz:3,cid:r.cid,fid:v.nata||""},{fid:v.nata||null, prep:key}); }
    /* la Sequenza (R-35): SEQ-01 un figlio in nessuna fase, SEQ-08 quantità divise che non tornano, SEQ-09 unione vuota, SEQ-02 trattamento senza terzista */
    if(r.figli.length){
      const u = unioniDi(c);
      if(!u.length) add("SEQ-01","KO",r,`${r.cod}: ${r.figli.length} componenti non entrano in nessuna fase (manca un'unione)`,{l:"Aggiungi la saldatura",staz:3,cid:r.cid});
      else {
        const senza = r.figli.filter(x=>x.cid && !u.some(f=>entranoDi(r,f).some(e=>e.cid===x.cid)));
        if(senza.length) add("SEQ-01","DB",r,`${r.cod}: ${senza.map(x=>x.cod).join(", ")} non ${senza.length>1?"entrano":"entra"} in nessuna fase del padre`,{l:"Apri la Sequenza",staz:4},{figli:senza.map(x=>x.cid)});
        for(const x of r.figli){ if(!x.cid) continue; const ee = u.flatMap(f=>entranoDi(r,f).filter(e=>e.cid===x.cid));
          if(ee.length && (ee.length>1 || ee[0].qta!=null)){ const somma = ee.reduce((a,e)=>a+(e.qta==null?x.qta:e.qta),0); if(somma!==x.qta) add("SEQ-08","DB",r,`${r.cod}: ${x.cod} diviso fra le fasi: ${somma} su ${x.qta} della BOM confermata`,{l:"Apri la Sequenza",staz:4},{figlio:x.cid}); } }
        u.forEach(f=>{ if(!entranoDi(r,f).length) add("SEQ-09","AV",r,`${r.cod} f.${(c.fasi.indexOf(f)+1)*10}: fase di unione senza nessun figlio`,{l:"Apri la Sequenza",staz:4},{fid:f.id}); });
      }
    }
  }
  /* requisiti dell'ordine (REQ-01, ORD-01) e lotti (LOT-01): sul prodotto */
  for(const q of (REQUISITI[T.key]||[])){
    const cop = requisitoCoperto(T,p,q);
    if(q.tipo==="condizione"){ if(!S2.visti[q.id]) add("ORD-01","DB",radice,`condizione dell'ordine da vedere: ${q.testo}`,{l:q.avvisa?"Avvisa il commerciale":"Presa visione in Esito",staz:5,richiesta:q.avvisa?"commerciale":null}); }
    else if(!cop.ok) add("REQ-01","DB",radice,`requisito ${q.fonte==="CART"?"del cartiglio":"dell'ordine"} senza fase: ${q.testo}`,{l:"Aggiungi o correggi la fase",staz:3,cid:radice&&radice.cid});
  }
  if(lottiDi(T).daChiedere) add("LOT-01","AV",radice,"RFQ senza lotti: si usano 100 e 500 e lo si dice (R-18)",null);
  return out;
}
function requisitoCoperto(T, p, q){
  const tutte = righeB(T,p).filter(r=>r.cid).flatMap(r=>{ const c = cicloB(T,r.cid); return c ? c.fasi.map((f,i)=>({r,f,N:(i+1)*10})) : []; });
  let x = null;
  if(q.tipo==="saldatura") x = tutte.find(y=>SALD.has(y.f.p));
  else if(q.tipo==="certificati") x = tutte.find(y=>y.f.p==="certificati");
  else if(q.tipo==="trattamento") x = tutte.find(y=>TRATT.has(y.f.p));
  else if(q.tipo==="controllo") x = tutte.find(y=>y.f.p==="controllo" && valF(y.f,"tipo")===q.valore);
  else return {ok:!!a2Di(T).visti[q.id], dove:"Esito"};
  return x ? {ok:true, dove:`${x.r.cod.slice(-4)} f.${x.N}`, cid:x.r.cid, fid:x.f.id} : {ok:false, dove:"nessuna fase"};
}
/* l'esito proposto per componente (R-35): il peggiore fra le regole; un AV non fa DUBBIO */
function esitoProposto(T, p, cid){
  const cc = controlliB(T,p).filter(x=>x.cid===cid);
  const v = cc.some(x=>x.liv==="KO" && !x.inventato) ? "KO" : cc.some(x=>x.liv==="DB"||(x.liv==="KO"&&x.inventato)) ? "DUBBIO" : "OK";
  return {v, perche: cc.filter(x=>(x.liv!=="AV" || v==="OK") && x.cod!=="DOC-02").slice(0,4)};
}

/* ---------------- le richieste (v16 rev., P-02): il Blocco 7B di Pietro così com'è, più le domande interne ----------------
   AI FORNITORI = richiesta_fornitore di Pietro (migrazioni 0015-0016): UNA riga per (RFQ, fornitore, lavorazione), con i
   codici e le note. Rotte di Pietro, già nel suo backend: POST /thread/{id}/richiesta (fornitore_id, lavorazione, codice,
   note, bozza=1 → la bozza in Outlook con il marcatore) e POST /thread/{id}/richiesta/{rid}/annulla. Stati di Pietro:
   bozza · inviata · offerta_ricevuta · declinata · scaduta · annullata. «inviata» la mette il sync quando la mail con il
   marcatore compare nella Posta inviata; «offerta ricevuta» e «declinata» le mette l'aggancio della risposta nell'Inbox
   (l'atto che conferma una persona); «scaduta» la calcola il server dalla scadenza (R-27). Il Cockpit non manda mail.
   Il BISOGNO (la carta del cassetto) non è una tabella: si legge da preparazioni, fasi esterne e grezzo (R-10 R-11). Il
   legame richiesta → bisogno si legge da lavorazione, codici e dal riferimento scritto nelle note (P-10: nessuna FK).
   INTERNE (officina, qualità, commerciale, cliente): non sono richiesta_fornitore (fornitore_id NOT NULL). Proposta R-28 ·
   R-29 · R-41: la stessa forma (bozza in Outlook con il marcatore), stati bozza · inviata · risposta · annullata.
   «Registra» NON è uno stato della richiesta: scrive costo, settimane o prezzo nel punto d'origine con la fonte RISP (R-13).
   I gesti «Simula …» del cassetto fanno quello che nel vero fanno il sync e l'Inbox: sono segnati «(mockup)». */
const NOMI_STATO_RQ = {bozza:"bozza in Outlook", inviata:"inviata · in attesa", offerta_ricevuta:"offerta ricevuta", declinata:"declinata", scaduta:"scaduta", annullata:"annullata", risposta:"risposta ricevuta"};
const statoRichiesta = q => q.stato;
function candidatiDi(T, tipo, p, f){
  const cli = (CLIENTI.find(c=>c.nome===T.testa.cliente)||{}).id;
  if(tipo==="mp_mai"||tipo==="mp_prezzo"){ return FORNITORI_MP.filter(x=>!f || x.forme.includes(f)).map(x=>x.id); }
  if(tipo==="lamiera") return ["lamitaglio","mp4"];
  /* i fornitori di attrezzature (fornitore_lavorazione = 'attrezzature', seme 0014 di Pietro) */
  if(tipo==="attrezzatura") return TERZISTI.filter(t=>(t.lavs||[]).includes("attrezzature")).map(t=>t.id);
  const lav = p;
  let l = TERZISTI.filter(t=>(t.lavs||[]).includes(lav));
  if(TRATT.has(lav) && cli) l.sort((a,b)=>((b.qual||{})[cli]||[]).includes(lav)-((a.qual||{})[cli]||[]).includes(lav));
  return l.slice(0,3).map(t=>t.id);
}
const TIPI_RICHIESTA = {
  terzista:{nome:"Richiesta al terzista", r:"7B · R-13", esterni:true},
  lamiera:{nome:"Lamiera tagliata comprata", r:"7B · R-13", esterni:true},
  mp_mai:{nome:"Materia prima mai comprata", r:"7B · R-27", esterni:true, via:"di solito la prepara Fabio (acquisti)"},
  mp_prezzo:{nome:"Materia prima: prezzo e tempi", r:"7B · R-27 · dec. 5", esterni:true, via:"di solito la prepara Fabio (acquisti)"},
  verniciatori:{nome:"Richiesta ai verniciatori", r:"7B · R-13 · R-33", esterni:true},
  attrezzatura:{nome:"Attrezzature (preparazioni)", r:"7B · R-11 · R-13", esterni:true},
  cliente:{nome:"Chiedi al cliente", r:"R-41", dest:"cliente", nomeDest:"il cliente (la manda il commerciale)"},
  domanda:{nome:"Domanda tecnica al cliente", r:"R-14 · R-41", dest:"cliente", nomeDest:"il cliente (la manda il commerciale)"},
  officina:{nome:"Chiedi all'officina", r:"R-28", dest:"officina", nomeDest:"capo officina e attrezzeria"},
  qualita:{nome:"Chiedi alla qualità", r:"R-29 · R-23", dest:"qualita", nomeDest:"responsabile qualità"},
  commerciale:{nome:"Avvisa il commerciale", r:"R-41", dest:"commerciale", nomeDest:"Franco (commerciale)"},
};
function giorniLavorativi(n){ return "14/10/2026"; }
/* le righe che toccano un codice, e quelle che portano una preparazione (il legame si LEGGE dalle note, P-10) */
function richiesteDi(T, cid){ return a2Di(T).richieste.filter(q=>q.codici.some(x=>x.cid===cid)); }
function richiesteDellaPrep(T, cid, key){ if(!cid) return []; return a2Di(T).richieste.filter(q=>(q.preps||[]).some(x=>x.cid===cid && x.key===key)); }
const RIF_NOTE = /\n— rif\. Cockpit: (\S+)$/;
const descrFornitore = id => { const t = terz(id); return t.lav || (t.forme ? `materia prima: ${t.forme.map(x=>x.replace("_"," ")).join(", ")}` : ""); };

/* ---------------- i BISOGNI: le carte del cassetto, lette dai dati (non una tabella) ---------------- */
function bisogniRichieste(T, p){
  const S2 = a2Di(T), out = [];
  const carta = (key, tipo, lav, titolo) => { let b = out.find(x=>x.key===key); if(!b){ b = {key, tipo, lav, titolo, voci:[]}; out.push(b); } return b; };
  for(const r of righeB(T,p)){
    if(!r.cid) continue; const cy = cicloB(T,r.cid); if(!cy) continue;
    cy.fasi.forEach((f,i)=>{ if(!(PROC[f.p] && PROC[f.p][1]==="est") || f.p==="trasporto") return;
      const tipo = TRATT.has(f.p) ? "verniciatori" : f.p==="laser_est" ? "lamiera" : "terzista";
      carta(tipo==="lamiera" ? "lamiera" : `${tipo}|${f.p}`, tipo, f.p, `${TIPI_RICHIESTA[tipo].nome} · ${PROC[f.p][0].toLowerCase()}`)
        .voci.push({cid:r.cid, cod:r.cod, descr:r.descr, qta:r.qtaProd, fid:f.id, dove:`f.${(i+1)*10}`, fatto: !!f.risp, terzi: valF(f,"terzi")||""}); });
    const g = grezzoVal(cy);
    if(g){ const sg = statoGrezzo(T,p,r.cid), f = cy.fasi.find(x=>CON_GREZZO.has(x.p)), tipo = sg.stato==="mai" ? "mp_mai" : (sg.stato==="acquistato"||sg.stato==="non_basta") ? "mp_prezzo" : null;
      if(tipo) carta(tipo, tipo, null, TIPI_RICHIESTA[tipo].nome).voci.push({cid:r.cid, cod:r.cod, descr:r.descr, qta:r.qtaProd, fid:f?f.id:null, dove:"grezzo", grezzo:testoGrezzo(g), forma:g.forma, fb:fabbisogno(T,p,r.cid), fatto: !!S2.prezziMP[sg.gid]}); }
    for(const [key,v] of prepVoci(cy)){
      if(!v.on || v.stato==="esiste già" || !(PREP_INFO[v.k]||PREP_INFO.altra).rich.includes("attrezzatura")) continue;
      /* chiesta all'interno (attrezzeria): non è un bisogno verso i fornitori */
      if(richiesteDellaPrep(T, r.cid, key).some(q=>!q.forn && q.stato!=="annullata")) continue;
      const i = cy.fasi.findIndex(f=>f.id===v.nata), iu = i>=0 ? infoUnione(T,p,r,cy,cy.fasi[i]) : null;
      carta("attrezzatura", "attrezzatura", "attrezzature", TIPI_RICHIESTA.attrezzatura.nome).voci.push({cid:r.cid, cod:r.cod, descr:r.descr, qta:r.qtaProd, fid:v.nata||null, prep:key, k:v.k,
        dove: `${v.k==="altra"?(v.nome||"altra"):nomePrep(v.k).toLowerCase()} ${i>=0?`f.${(i+1)*10}${iu&&iu.preUnione?" pre-unione":""}`:"fuori dalle fasi"}`, fatto: !!v.costo});
    }
  }
  for(const b of out){
    const forme = [...new Set(b.voci.map(v=>v.forma).filter(Boolean))];
    const ids = b.tipo==="attrezzatura" ? candidatiDi(T,"attrezzatura") : /^mp_/.test(b.tipo) ? FORNITORI_MP.filter(x=>!forme.length || forme.some(f=>x.forme.includes(f))).map(x=>x.id)
      : [...new Set([...b.voci.flatMap(v=>String(v.terzi).split(",").filter(Boolean)), ...candidatiDi(T, b.tipo, b.lav)])];
    /* ogni fornitore con la sua descrizione accanto (M7): per le attrezzature solo le preparazioni che fa (prep) */
    b.cands = ids.map(id=>({id, nome:terz(id).nome, descr:descrFornitore(id), voci: b.voci.filter(v=>!v.fatto && (!v.k || !terz(id).prep || terz(id).prep.includes(v.k)))}));
    b.righe = S2.richieste.filter(q=>q.bisogno===b.key);
  }
  return out;
}
/* la frase precompilata della nota (PREPARAZIONI §3.2 per le attrezzature) e il riferimento letto (P-10) */
function notaBisogno(T, p, b, voci){
  const L = lottiDi(T), cert = certRichiesto(T);
  let t;
  if(b.tipo==="attrezzatura") t = voci.map(v=>{ const cy = cicloB(T,v.cid), pv = cy && cy.prep[v.prep], r = rigaB(T,p,v.cid); return pv && r ? frasePrep(T,p,r,cy,pv) : ""; }).filter(Boolean).join("\n");
  else if(/^mp_/.test(b.tipo)) t = `Prezzo e tempi per ${voci.map(v=>`${v.grezzo}${v.fb?` (${fmt(v.fb.kg,0)} kg)`:""}`).join(", ")}${cert?`, certificato ${cert}`:""}.`;
  else {
    /* RL-11: per ogni codice i lotti della SUA fase (campo «Lotti»), altrimenti quelli della testata; il materiale del
       componente e il conto vendita, se la fase lo dice */
    const parti = voci.map(v=>{ const x = v.fid ? faseDelThread(T, v.fid) : null, f = x ? x.f : null, mc = materialeComp(T, v.cid);
      const lot = f ? (valF(f,"lotti")||valF(f,"lotto")) : null, conto = f ? valF(f,"conto") : null;
      return `${v.cod}${v.descr?` (${String(v.descr).toLowerCase()})`:""}: lotti ${lot ? String(lot).split(/[\/;,]/).map(y=>y.trim()).filter(Boolean).join(" / ") : L.lotti.join(" / ")} pezzi${mc?`, materiale ${mc.q}`:""}${conto?`, ${conto}${conto==="conto vendita"?" (il materiale lo mettete voi)":""}`:""}`; });
    t = `Quotazione ${PROC[b.lav]?PROC[b.lav][0].toLowerCase():""} per ${parti.join("; ")}${cert?`, certificato ${cert}`:""}. In allegato i disegni. Indicate i tempi di consegna.`;
  }
  return t;
}
/* per Come: i bisogni ancora senza nessuna richiesta (nessuna parte da sola) */
function richiesteProposte(T, p){
  return bisogniRichieste(T,p).filter(b=>b.voci.some(v=>!v.fatto) && !b.righe.some(q=>q.stato!=="annullata"))
    .flatMap(b=>b.voci.filter(v=>!v.fatto).map(v=>({tipo:b.tipo, key:b.key, cid:v.cid, cod:v.cod, fid:v.fid, lav:b.lav})));
}
/* la frase precompilata della richiesta di una preparazione (PREPARAZIONI §3.2) */
function frasePrep(T, p, r, c, v){
  const info = PREP_INFO[v.k] || PREP_INFO.altra, L = lottiDi(T);
  const i = c.fasi.findIndex(f=>f.id===v.nata), f = i>=0 ? c.fasi[i] : null, iu = f ? infoUnione(T,p,r,c,f) : null;
  const rif = iu && iu.entrano.length ? iu.entrano.map(e=>`${e.cod.replace(/^F\+800\.514263/,"…")} ×${e.qta||e.qtaBom}`).join(", ") : "dal disegno";
  const g = grezzoVal(c), dato = v.k==="curva" && g ? `tubo Ø${g.D||"?"} × ${g.sp||"?"}` : v.k==="piega" && g ? `lamiera ${g.q} sp. ${g.sp||"?"}` : v.k==="prova" && g ? `${g.q} sp. ${g.sp||"?"}` : v.k==="calibro" ? "controllo delle quote critiche" : "dati sul disegno";
  return info.frase.replace("{cod}", r.cod).replace("{rev}", "").replace("{rif}", rif).replace("{pezzi} pezzi", Math.max(...L.lotti)===1 ? "1 pezzo" : "{pezzi} pezzi").replace("{pezzi}", String(Math.max(...L.lotti))).replace("{dato}", dato)
    .replace("{cordoni}", `${(f&&f.giunti||[]).length} giunti scritti`).replace("{nome}", v.nome||"Preparazione").replace("In allegato PDF e 3D.", ha3D(T) ? "In allegato PDF e 3D." : "In allegato il PDF (il 3D non c'è).") + (iu && iu.preUnione ? ` (maschera della pre-unione f.${iu.N})` : "");
}
/* RIPROVA: il 3D si promette al fornitore solo se nel thread c'è un file 3D (RL-17) */
function ha3D(T){ return Object.values(T.allegati).some(a=>/^(step|stp|igs)$/i.test(a.tipo||"") || /\.(step|stp|igs)$/i.test(a.nome)); }
/* dove è stata registrata la risposta: si LEGGE dal punto d'origine (fonte RISP con il riferimento), non dalla richiesta */
function registrazioneDi(T, q){
  const S2 = a2Di(T), dove = [], giro = (q.giri||[]).length+1;
  /* R-13 precisata: una risposta per giro; un giro nuovo si registra da capo, i precedenti restano */
  if(q.regGiro===giro && q.regTesto) return q.regTesto;
  if(q.regGiro && q.regGiro!==giro) return null;
  for(const x of (q.preps||[])){ const cy = cicloB(T,x.cid), v = cy && cy.prep[x.key]; if(v && v.rispDa===q.id) dove.push(`${nomePrep(v.k).toLowerCase()} ${fmt(num(v.costo),0)} €`); }
  for(const c of q.codici){ const cy = c.cid ? cicloB(T,c.cid) : null; if(!cy) continue;
    cy.fasi.forEach((f,i)=>{ if(f.risp && f.risp.rid===q.id) dove.push(`f.${(i+1)*10} di ${c.cod.slice(-4)}`); }); }
  for(const v of Object.values(S2.prezziMP||{})) if(v.rid===q.id) dove.push(`grezzo ${fmt(v.eur,2)} €/kg`);
  for(const v of (S2.noteRisp||[])) if(v.rid===q.id) dove.push(v.tipo==="officina" ? "lavorabilità" : "nota");
  return dove.length ? [...new Set(dove)].join(" · ") : null;
}
/* RL-08 · il materiale del componente: il grezzo del ciclo se c'è, altrimenti il «materiale del disegno» scritto nella fase
   esterna (conto vendita: il grezzo è del terzista). Il confronto con il materiale dichiarato normalizza la grafia
   («S 355 J2 N» = S355J2+N) e passa dalle equivalenze; mai un'alternativa al ribasso. */
function materialeComp(T, cid){
  const cy = cid ? cicloB(T,cid) : null, g = grezzoVal(cy);
  if(g && g.q) return {q:g.q, fonte:"grezzo"};
  const f = cy && cy.fasi.find(x=>valF(x,"mat"));
  return f ? {q:valF(f,"mat"), fonte:"fase"} : null;
}
function normMat(s){ const t = String(s||"").split(/[·,;(]/)[0].trim(), eq = EQUIVALENZE.find(e=>e.stato==="confermata" && e.nome.replace(/\s/g,"").toUpperCase()===t.replace(/\s/g,"").toUpperCase());
  return String(eq ? eq.q : t).toUpperCase().replace(/[\s+.\-]/g,""); }
function matDiscorde(dich, comp){
  if(!dich || !comp || !/^\s*(S\s*\d|C\s*\d|\d{2}[A-Z])/i.test(String(dich))) return false;
  const a = normMat(dich), b = normMat(comp); return !(a.startsWith(b) || b.startsWith(a));
}
/* i controlli sulla risposta: materiale dichiarato ≠ disegno (MAT-05), lotto diverso (LOT-02) */
function controlliRisposta(T, p, q){
  const out = [], x = q.risp; if(!x) return out;
  for(const c of q.codici){
    const mc = materialeComp(T, c.cid);
    if(x.materiale && mc && matDiscorde(x.materiale, mc.q) && !out.some(k=>k.cod==="MAT-05")) out.push({cod:"MAT-05", liv:"DB", testo:`materiale dichiarato «${x.materiale}» ≠ disegno ${mc.q}: non confrontabile, mai un'alternativa al ribasso`});
    if(x.materiale && c.cod==="9N008519AB" && !/C40/.test(x.materiale)) out.push({cod:"MAT-05", liv:"DB", testo:`materiale dichiarato «${x.materiale}» ≠ disegno C40`});
  }
  /* il confronto si fa in PEZZI: lotto prodotto N → N × q.tà per assieme di ogni codice chiesto (1 telaio = 2 pz di .006) */
  if(x.lotto && !(x.righe && x.righe[0] && x.righe[0].lotti) && q.lotti && !/telaio|barra|barre/.test(x.lotto)){
    const pz = pezziChiesti(q), n = parseInt(String(x.lotto),10);
    if(isFinite(n) && pz.length && !pz.includes(n)) out.push({cod:"LOT-02", liv:"AV", testo:`la risposta quota ${n} pz: chiesti ${pz.join(" / ")} pz (${testoPezzi(q)})`});
  }
  return out;
}

/* ---------------- i gesti della zona B: rotte immaginate, mai della lista A ---------------- */
const ROTTE_B = [
 ["comeFornitura", /^\/componente\/([0-9a-f-]{36})\/fornitura$/],
 ["comeModello", /^\/componente\/([0-9a-f-]{36})\/ciclo\/modello$/],
 ["comeBlocco", /^\/rfq\/([0-9a-f-]{36})\/come\/blocco$/],
 ["fattibilita", /^\/componente\/([0-9a-f-]{36})\/fattibilita$/],
 ["faseNuova", /^\/componente\/([0-9a-f-]{36})\/ciclo\/fase$/],
 ["fase", /^\/fase\/(f-\d+)$/],
 ["faseAccetta", /^\/fase\/(f-\d+)\/accetta$/],
 ["faseTogli", /^\/fase\/(f-\d+)\/togli$/],
 ["faseSposta", /^\/fase\/(f-\d+)\/sposta$/],
 ["faseMateriale", /^\/fase\/(f-\d+)\/materiale$/],
 ["faseGiunto", /^\/fase\/(f-\d+)\/giunto$/],
 ["faseDaRisposta", /^\/fase\/(f-\d+)\/da-risposta$/],
 ["preparazioni", /^\/componente\/([0-9a-f-]{36})\/preparazioni$/],
 ["cicloConferma", /^\/componente\/([0-9a-f-]{36})\/ciclo\/conferma$/],
 ["cicloRiapri", /^\/componente\/([0-9a-f-]{36})\/ciclo\/riapri$/],
 ["cicloCopia", /^\/componente\/([0-9a-f-]{36})\/ciclo\/copia$/],
 ["nota", /^\/componente\/([0-9a-f-]{36})\/nota$/],
 /* le richieste ai fornitori: le rotte di PIETRO (Blocco 7B), così come sono nel suo backend */
 ["richiestaFornitore", /^\/thread\/([0-9a-f-]{36})\/richiesta$/],
 ["richiestaAnnulla", /^\/thread\/([0-9a-f-]{36})\/richiesta\/(rq-\d+)\/annulla$/],
 /* le domande interne (proposta R-28 · R-29 · R-41) e la registrazione della risposta nel punto d'origine (R-13) */
 ["domanda", /^\/rfq\/([0-9a-f-]{36})\/domanda$/],
 ["domandaAnnulla", /^\/domanda\/(rq-\d+)\/annulla$/],
 ["risposta", /^\/rfq\/([0-9a-f-]{36})\/risposta$/],
 ["richiestaGiro", /^\/richiesta\/(rq-\d+)\/giro$/],
 /* simulazioni del mockup: nel vero le fanno il sync (Posta inviata), l'Inbox (aggancio della risposta) e il server (scadenza) */
 ["simSync", /^\/mockup\/richiesta\/(rq-\d+)\/sync$/],
 ["simRisposta", /^\/mockup\/richiesta\/(rq-\d+)\/risposta$/],
 ["simScadenza", /^\/mockup\/richiesta\/(rq-\d+)\/scadenza$/],
 ["faseComponente", /^\/fase\/(f-\d+)\/componente$/],
 ["faseProteggi", /^\/fase\/(f-\d+)\/proteggi$/],
 ["seqRipristina", /^\/prodotto\/([0-9a-f-]{36})\/sequenza\/ripristina$/],
 ["seqConferma", /^\/prodotto\/([0-9a-f-]{36})\/sequenza\/conferma$/],
 ["assiemeInterno", /^\/componente\/([0-9a-f-]{36})\/assieme-interno$/],
 ["nodoProposto", /^\/distinta\/([0-9a-f-]{36})\/nodo$/],
 /* R-38 precisata: «Aggiungi un componente sotto un padre» anche senza STEP (InsertComponente origine manuale + Collega) */
 ["componenteManuale", /^\/distinta\/([0-9a-f-]{36})\/componente$/],
 /* R-13 precisata: la risposta del fornitore con i prezzi per codice e per scaglione, il materiale dichiarato e la data del giro */
 ["rispostaRighe", /^\/rfq\/([0-9a-f-]{36})\/risposta\/righe$/],
 /* RL-18 (R-10): il trattamento del prodotto finito come scelta esplicita: «nessuno (consegna grezza)» */
 ["cicloTrattamento", /^\/componente\/([0-9a-f-]{36})\/ciclo\/trattamento$/],
 /* il tempo del mockup: nel vero passano i giorni (RL-28) */
 ["simTempo", /^\/mockup\/tempo$/],
 ["esiti", /^\/prodotto\/([0-9a-f-]{36})\/esiti$/],
 ["firma", /^\/prodotto\/([0-9a-f-]{36})\/firma$/],
 ["consegna", /^\/prodotto\/([0-9a-f-]{36})\/consegna$/],
 ["requisito", /^\/prodotto\/([0-9a-f-]{36})\/requisito$/],
 ["datiDoc", /^\/prodotto\/([0-9a-f-]{36})\/dati-documenti$/],
 ["lotto", /^\/prodotto\/([0-9a-f-]{36})\/lotto$/],
 ["offerta", /^\/rfq\/([0-9a-f-]{36})\/offerta\/(\w+)$/],
 ["costiCampo", /^\/rfq\/([0-9a-f-]{36})\/costi\/campo$/],
];
function faseDelThread(T, fid){ const S2 = a2Di(T); for(const [cid,c] of Object.entries(S2.cicli)){ const i = c.fasi.findIndex(f=>f.id===fid); if(i>=0) return {cid, c, f:c.fasi[i], i}; } return null; }
function componenteConfermato(T, p, cid){ const r = rigaB(T,p,cid); return !!(r && r.cid); }
function nuovaFase(S2, p, prop){ return {id:"f-"+(++S2.nF), p, par:{}, prop: prop ? copia(prop) : {}}; }
function proposteGeneriche(T, p, r, c, f){
  const g = grezzoVal(c);
  if(f.p==="laser" && g && !f.prop.macc){ const m = laserPerSpessore(num(g.sp)); if(m) f.prop.macc = [m,"CAP"]; }
  if(f.p==="piega" && !f.prop.macc) f.prop.macc = ["px80","CAP"];
  delete f.prop.oggetto;   /* v16 rev. (M3): «che cosa si salda» si legge da «Entrano», non si scrive */
  /* il programma del robot nasce dalla fase sald_robot come proposta (PREP-03): la accende Lino con «da fare» */
  if(f.p==="sald_robot" && c && !prepVoci(c).some(([,v])=>v.k==="robot" && v.nata===f.id)) c.prep[chiavePrep(T,c,"robot",f.id)] = {k:"robot", on:false, proposta:true, nata:f.id, costo:"", settimane:"", stato:"da fare", fonte:"PREP-03"};
  if(SALD.has(f.p) && !f.prop.norma){ const q = (REQUISITI[T.key]||[]).find(x=>x.tipo==="saldatura"); if(q) f.prop.norma = [q.testo, q.fonte]; }
  if(f.p==="certificati" && !f.prop.tipo){ const c3 = certRichiesto(T); if(c3) f.prop.tipo = ["3.1 EN 10204","ORD"]; }
}
function creaCiclo(T, p, r, modello, seme){
  const S2 = a2Di(T);
  const c = {stato:"bozza", modello, fasi:[], grezzo:{par:{}, prop: seme && seme.grezzo ? copia(seme.grezzo) : {}}, prep:{}, fonte: seme ? (seme.fonte||"STO") : "PRO-06", copiaDa:null, conferme:0};
  const fasi = seme && seme.fasi && !seme.copia ? seme.fasi : (MODELLI[modello]||{fasi:[]}).fasi.map(x=>({p:x}));
  if(!(seme && seme.copia)) c.fasi = fasi.map(x=>nuovaFase(S2, x.p, x.prop));
  if(seme && seme.prep) for(const [k,v] of Object.entries(seme.prep)) c.prep[k] = Object.assign({on:false, proposta:true}, v);
  c.fasi.forEach(f=>proposteGeneriche(T,p,r,c,f));
  /* il tempo di saldatura non si propone mai: è la stima più debole, Lino lo scrive guardando preventivo e consuntivo */
  c.fasi.forEach(f=>{ if(SALD.has(f.p)) delete f.prop[T_]; });
  S2.cicli[r.cid] = c;
  if(r.figli && r.figli.length) semeSequenza(T,p,r,c);   /* v16: il seme della sequenza, una volta sola */
  return c;
}
/* scrivere un campo nel ciclo: la zona B non tocca mai la zona A */
function scriviCampo(T, p, cid, c, f, k, v, fonte){
  if(v===""||v==null){ delete f.par[k]; if(["nome","rif","proteggi"].includes(k)){ const r0 = rigaB(T,p,cid); if(r0 && r0.figli.length) seqScritta(T, c); } return; }
  f.par[k] = String(v); f.fonti = f.fonti || {}; f.fonti[k] = fonte || "L";
  /* v16 (PREP-03): la preparazione nasce dalla SUA fase. Due unioni in maschera = due maschere (pre-unione e assieme) */
  const r = rigaB(T,p,cid);
  const spegni = kk => { const x = prepVoci(c).find(([,y])=>y.k===kk && y.nata===f.id); if(x) c.prep[x[0]].on = false; };
  if(k==="maschera" && (SALD.has(f.p) || f.p==="prigionieri")){ if(v==="sì") accendiPrep(T, r, c, "maschera", f.id); else spegni("maschera"); }
  if(k==="dima" && f.p==="foratura"){ if(v==="sì") accendiPrep(T, r, c, "dima", f.id); else spegni("dima"); }
  if(k==="tipo" && f.p==="controllo" && v==="dimensionale a calibro" && !prepVoci(c).some(([,y])=>y.k==="calibro")) c.prep[chiavePrep(T,c,"calibro",f.id)] = {k:"calibro", on:false, proposta:true, nata:f.id, costo:"", settimane:"", stato:"da fare", fonte:"PREP-03"};
  if(["nome","rif","proteggi"].includes(k) && r && r.figli.length) seqScritta(T, c);
  if(c.stato==="confermato"){ c.stato="bozza"; c.riaperto = "modificato dopo la conferma: da riconfermare"; }
}
const SERVER_B = {
  posta(T, p, url, campi){
    let nome = null, m = null;
    for(const [k, re] of ROTTE_B){ const x = re.exec(url); if(x){ nome = k; m = x; break; } }
    if(!nome) return {esito:"errore", testo:"Rotta della zona B non prevista: niente è partito."};
    if(ROTTE.map(compilaRotta).some(re=>re.test(url))) return {esito:"errore", testo:"Una rotta della zona A non passa dal server B."};
    const S2 = a2Di(T), g = k => campi.get(k);
    switch(nome){
      case "comeFornitura": {
        const cid = m[1], r = rigaB(T,p,cid);
        if(!r || !r.cid) return NO_B("Niente è cambiato: si smista solo un componente confermato. Prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38) (RV-27).");
        if(r.comm) return NO_B("Niente è cambiato: è un commerciale nell'albero, lo compra Fabio. Il tipo si cambia solo in Struttura (gesto Tipo), e l'albero torna da confermare.");
        const modo = g("modo"); if(!MODI.some(x=>x[0]===modo)) return NO_B("Niente è cambiato: modo non previsto.");
        if((modo==="escluso"||modo==="noquote") && !String(g("testo")||"").trim()) return NO_B(modo==="escluso" ? "Niente è cambiato: scrivi il testo per l'offerta («escluso dalla fornitura: …»)." : "Niente è cambiato: scrivi il motivo del no quote.");
        const prop = modoComponente(T,p,r), cc = cicloB(T,cid);
        if(cc && cc.stato==="confermato" && !MODO_CON_CICLO.has(modo)) return NO_B(`Niente è cambiato: il ciclo di ${r.cod} è confermato. Per farlo diventare «${nomeModo(modo)}» riaprilo prima nel Ciclo.`);
        S2.come[cid] = {modo, testo: g("testo")||"", da: g("da")||"Lino", modello: MODO_CON_CICLO.has(modo) ? (prop.modo===modo ? prop.modello : {noi:"T1",lamiera:"T5",fuori:"T4"}[modo]) : {cliente:"T7",escluso:"T8",noquote:null}[modo]};
        if(modo==="noquote") S2.esiti[cid] = {v:"KO", nota:g("testo")||"", da:"Lino (no quote in Come)"};
        else if(S2.esiti[cid] && /no quote/.test(S2.esiti[cid].da||"")) delete S2.esiti[cid];
        const c0 = cicloB(T,cid);
        if(c0 && c0.stato!=="confermato" && MODO_CON_CICLO.has(modo) && g("conModello")==="1" && c0.modello!==S2.come[cid].modello && !c0.copiaDa) delete S2.cicli[cid];
        if(MODO_CON_CICLO.has(modo) && g("conModello")==="1" && !cicloB(T,cid)){
          const seme = SEMI[r.cod];
          if(seme && seme.copia){ const da = righeB(T,p).find(x=>x.cod===seme.copia); const cs = da && cicloB(T,da.cid);
            if(cs){ copiaCiclo(T,p,r,cs,da.cod); return SI_B(`${r.cod}: ${nomeModo(modo)} · ⧉ ciclo copiato da ${da.cod} in bozza, con i driver da scrivere (metri, fori, filetti).`); } }
          creaCiclo(T,p,r,S2.come[cid].modello, seme && seme.modo===modo ? seme : null);
          return SI_B(`${r.cod}: ${nomeModo(modo)} · modello ${S2.come[cid].modello} in bozza nel Ciclo (proposte ◇ da accettare).`);
        }
        if(!MODO_CON_CICLO.has(modo) && S2.cicli[cid]) delete S2.cicli[cid];
        return SI_B(`${r.cod}: ${nomeModo(modo)}${S2.come[cid].testo?" · "+S2.come[cid].testo:""}.`);
      }
      case "comeModello": {
        const cid = m[1], r = rigaB(T,p,cid); if(!r||!r.cid) return NO_B("Niente è cambiato: il ciclo si lega a un componente confermato (RV-27).");
        const mod = g("modello"); if(!MODELLI[mod]) return NO_B("Niente è cambiato: modello non previsto.");
        const c0 = cicloB(T,cid);
        if(c0 && c0.stato==="confermato" && g("riapri")!=="1") return NO_B(`Niente è cambiato: il ciclo di ${r.cod} è confermato. Per applicare un modello serve la spunta «riapri anche i confermati».`);
        S2.come[cid] = Object.assign(S2.come[cid]||{modo:"noi", da:"Lino"}, {modello:mod});
        const seme = SEMI[r.cod];
        creaCiclo(T,p,r,mod, seme && seme.modello===mod ? seme : null);
        return SI_B(`${r.cod}: modello ${mod} «${MODELLI[mod].nome}» in bozza. Le fasi hanno i campi vuoti o proposti ◇.`);
      }
      case "comeBlocco": {
        const cids = String(g("cids")||"").split(",").filter(Boolean), modo = g("modo");
        if(!cids.length) return NO_B("Niente è cambiato: nessuna riga selezionata.");
        if(g("modello")){ const esiti = cids.map(cid=>{ const r0 = SERVER_B.posta(T,p,`/componente/${cid}/ciclo/modello`, new URLSearchParams({modello:g("modello"), riapri:g("riapri")||""})); const rr = rigaB(T,p,cid); return `${rr?rr.cod:cid}: ${r0.esito==="ok"?"ok":"no · "+r0.testo}`; });
          return SI_B(`Blocco, modello ${g("modello")}: ${esiti.join(" · ")}`); }
        const esiti = cids.map(cid=>{ const q = new URLSearchParams({modo, conModello:"1", testo:g("testo")||""}); const r0 = SERVER_B.posta(T,p,`/componente/${cid}/fornitura`, q); const rr = rigaB(T,p,cid); return `${rr?rr.cod:cid}: ${r0.esito==="ok"?"ok":"no · "+r0.testo}`; });
        return SI_B(`Blocco: ${esiti.length} righe · ${esiti.join(" · ")}`);
      }
      case "fattibilita": {
        const cid = m[1], v = g("esito"); if(v && !["OK","DUBBIO","KO"].includes(v)) return NO_B("Niente è cambiato: esito non previsto.");
        if(!v && g("nota")==null) return NO_B("Niente è cambiato.");
        const prima = S2.esiti[cid]||{};
        S2.esiti[cid] = {v: v || prima.v || null, nota: g("nota")!=null ? g("nota") : (prima.nota||""), da: v ? "Lino" : (prima.da||"")};
        return SI_B(v ? `Esito ${v} registrato per ${(rigaB(T,p,cid)||{}).cod||""}.` : `Nota salvata per ${(rigaB(T,p,cid)||{}).cod||""} (l'esito resta ${prima.v||"da decidere"}).`);
      }
      case "faseNuova": {
        const cid = m[1], r = rigaB(T,p,cid); if(!r||!r.cid) return NO_B("Niente è cambiato: il ciclo si lega a un componente confermato (RV-27).");
        const pp = g("p"); if(!PROC[pp]) return NO_B("Niente è cambiato: lavorazione non nel catalogo.");
        let c = cicloB(T,cid); if(!c){ S2.come[cid] = S2.come[cid] || {modo:"noi", da:"Lino", modello:null}; c = creaCiclo(T,p,r,null,null); }
        const f = nuovaFase(S2, pp, null); proposteGeneriche(T,p,r,c,f);
        if(UNIONE.has(pp)) f.entrano = [];   /* una fase di unione nuova nasce VUOTA: i figli li fa entrare Lino */
        const dopo = g("dopo"), prima = g("prima");
        const iP = prima ? c.fasi.findIndex(x=>x.id===prima) : -1, i = dopo ? c.fasi.findIndex(x=>x.id===dopo) : -1;
        if(prima && iP<0) return NO_B("Niente è cambiato: la fase di riferimento non c'è più. Rileggi.");
        if(iP>=0) c.fasi.splice(iP,0,f); else if(i>=0) c.fasi.splice(i+1,0,f); else c.fasi.push(f);
        if(c.stato==="confermato") c.stato="bozza";
        if(r.figli.length && (UNIONE.has(pp) || prima || g("seq")==="1")) seqScritta(T, c);
        const N = (c.fasi.indexOf(f)+1)*10;
        return Object.assign(SI_B(`Fase ${N} «${PROC[pp][0]}» aggiunta a ${r.cod}${UNIONE.has(pp)&&r.figli.length?": nasce vuota, fai entrare i figli":""}.`), {fid:f.id});
      }
      case "fase": {
        const x = faseDelThread(T, m[1]); if(!x) return NO_B("Niente è cambiato: fase non trovata.");
        const n = []; for(const [k,v] of campi.entries()){ if(k==="fonte") continue; scriviCampo(T,p,x.cid,x.c,x.f,k,v, g("fonte")); n.push(k); }
        const t = x.f.par[T_]; if(t && leggiTempo(t).errore) return NO_B(`Il tempo «${t}» non si legge: ${leggiTempo(t).testo}. Il resto della fase è salvato.`);
        return SI_B(`Fase ${(x.i+1)*10} salvata${n.length?" ("+n.join(", ")+")":""}.`);
      }
      case "faseAccetta": {
        const x = faseDelThread(T, m[1]); if(!x) return NO_B("Niente è cambiato: fase non trovata.");
        const k = g("campo"); const chiavi = k ? [k] : Object.keys(x.f.prop).filter(y=>propBlocco(x.f,y));
        /* v16: senza un campo, la fase che consuma il grezzo accetta anche il grezzo proposto */
        const gz = x.c.grezzo, conGrezzo = !k && x.c.fasi.find(y=>CON_GREZZO.has(y.p))===x.f && gz && Object.keys(gz.prop).length && !grezzoAccettato(x.c);
        if(conGrezzo){ Object.keys(gz.prop).forEach(y=>{ if(gz.par[y]==null) gz.par[y] = gz.prop[y][0]; }); gz.fonte = "proposta accettata"; if(x.c.stato==="confermato") x.c.stato = "bozza"; }
        if(!chiavi.length && conGrezzo) return SI_B(`Fase ${(x.i+1)*10}: grezzo accettato (${testoGrezzo(grezzoVal(x.c))}).`);
        if(!chiavi.length) return NO_B("Niente è cambiato: nessuna proposta ◇ da accettare in questa fase.");
        chiavi.forEach(y=>{ const pr = x.f.prop[y]; if(pr && pr[0]!=="") scriviCampo(T,p,x.cid,x.c,x.f,y,pr[0],pr[1]); });
        return SI_B(`Fase ${(x.i+1)*10}: ${chiavi.length===1?"proposta accettata":chiavi.length+" proposte accettate"} (la fonte resta visibile).`);
      }
      case "faseTogli": { const x = faseDelThread(T,m[1]); if(!x) return NO_B("Niente è cambiato."); const rr = rigaB(T,p,x.cid), n = rr ? entranoDi(rr,x.f).length : 0;
        /* A1: le fasi che lavoravano su questa pre-unione restano dove sono, e l'avviso lo dice (spostale o toglile) */
        const sopra = rr && UNIONE.has(x.f.p) && (infoUnione(T,p,rr,x.c,x.f)||{}).preUnione ? x.c.fasi.filter(g=>{ const su = suPreUnione(T,p,rr,x.c,g); return su && su.fid===x.f.id; }) : [];
        x.c.fasi.splice(x.i,1); if(x.c.stato==="confermato") x.c.stato="bozza";
        for(const [key,v] of prepVoci(x.c)) if(v.nata===x.f.id && !richiesteDellaPrep(T, x.cid, key).some(q=>q.stato!=="annullata")) delete x.c.prep[key];
        if(rr && rr.figli.length && (UNIONE.has(x.f.p) || x.c.fonteSeq)) seqScritta(T, x.c);
        return SI_B(`Fase ${valF(x.f,"nome")||PROC[x.f.p][0]} tolta${n?`: ${n} ${n===1?"figlio torna":"figli tornano"} «in nessuna fase» (SEQ-01)`:""}.${sopra.length?` Lavoravano sulla pre-unione: ${sopra.map(g=>`f.${(x.c.fasi.indexOf(g)+1)*10} ${valF(g,"nome")||PROC[g.p][0]}`).join(", ")}: spostale o toglile.`:""}`); }
      case "faseSposta": { const x = faseDelThread(T,m[1]); if(!x) return NO_B("Niente è cambiato."); const d = g("dir")==="su"?-1:1, j = x.i+d;
        if(j<0||j>=x.c.fasi.length) return NO_B("Niente è cambiato: la fase è già in fondo o in cima."); x.c.fasi.splice(x.i,1); x.c.fasi.splice(j,0,x.f); if(x.c.stato==="confermato") x.c.stato="bozza";
        const rr = rigaB(T,p,x.cid); if(rr && rr.figli.length) seqScritta(T, x.c); return SI_B(`Fase spostata ${d<0?"su":"giù"}: ora è la f.${(j+1)*10}.`); }
      case "faseMateriale": {
        const x = faseDelThread(T, m[1]); if(!x) return NO_B("Niente è cambiato: fase non trovata.");
        const c = x.c, gz = c.grezzo;
        if(g("accetta")==="1"){ const k = Object.keys(gz.prop); if(!k.length) return NO_B("Niente è cambiato: nessuna proposta per il grezzo."); k.forEach(y=>{ if(gz.par[y]==null) gz.par[y] = gz.prop[y][0]; }); gz.fonte = "proposta accettata";
          if(c.stato==="confermato") c.stato="bozza"; return SI_B(`Grezzo accettato: ${testoGrezzo(grezzoVal(c))}.`); }
        if(g("cerca")!=null){ const r = cercaGrezzo(g("cerca"), T.testa.cliente); if(!r || !r.q) return NO_B(`«${g("cerca")}» non si trova nella banca dati dei materiali: scrivi per esempio «s355j2 15», «upn160», «355 tubo 127x10».`);
          if(r.equiv && r.equiv.stato!=="confermata") gz.equiv = {nome:r.equiv.nome, q:r.q, stato:"proposta"}; else delete gz.equiv;
          const formaPrima = gz.par.forma || (gz.prop.forma||[])[0];
          for(const k of ["q","forma","sp","D","a","b","prof"]) if(r[k]!=null) gz.par[k] = String(r[k]); else if(k!=="q" && k!=="forma" && r.forma!==formaPrima) delete gz.par[k];
          if(gz.par.forma===formaPrima) for(const k of ["sp","D","a","b","prof"]) if(gz.par[k]==null && gz.prop[k]) gz.par[k] = gz.prop[k][0];
          gz.fonte = "L"; if(c.stato==="confermato") c.stato="bozza";
          return SI_B(`Grezzo: ${testoGrezzo(grezzoVal(c))}${gz.equiv?` · equivalenza «${gz.equiv.nome}» da confermare dalla qualità`:""}${r.vistoPer?` · ◇ qualità già vista per ${r.vistoPer}: controllala sul disegno`:""}.`); }
        for(const [k,v] of campi.entries()) if(k!=="fonte"){ if(v==="") delete gz.par[k]; else gz.par[k] = v; }
        if(c.stato==="confermato") c.stato="bozza";
        return SI_B("Grezzo salvato con la fase.");
      }
      case "faseGiunto": {
        const x = faseDelThread(T, m[1]); if(!x || !SALD.has(x.f.p)) return NO_B("Niente è cambiato: i giunti stanno in una fase di saldatura.");
        if(g("togli")!=null){ (x.f.giunti||[]).splice(+g("togli"),1); return SI_B("Giunto tolto."); }
        const cc = convertiCordone(g("riga")); if(!cc || !cc.ok) return NO_B(`Niente è cambiato: ${cc?cc.testo:"riga vuota"}.`);
        x.f.giunti = x.f.giunti || []; x.f.giunti.push({riga:g("riga"), unisce:g("unisce")||"", conv:cc});
        if(x.c.stato==="confermato") x.c.stato="bozza";
        return SI_B(`Giunto aggiunto: ${cc.testo}.`);
      }
      case "preparazioni": {
        /* v16: una riga per preparazione (chiave = tipo, o tipo@fase). Campi: on, stato (da fare | esiste già), costo, settimane,
           riferimento, nome (solo «altra», obbligatorio), fase_id. «Esiste già» cerca nello storico (MASCHERE) e nelle capacità R-28. */
        const cid = m[1], c = cicloB(T,cid), r = rigaB(T,p,cid); if(!c) return NO_B("Niente è cambiato: prima il ciclo.");
        const k = g("k"); if(!PREP_TIPI.some(x=>x[0]===k)) return NO_B("Niente è cambiato: preparazione non prevista.");
        if(k==="altra" && g("key")==null && !String(g("nome")||"").trim()) return NO_B("Niente è cambiato: «altra» vuole il nome (per esempio «attrezzo di piantaggio», «ganci del verniciatore»).");
        const fid = g("fase_id") || null;
        if(fid && !c.fasi.some(f=>f.id===fid)) return NO_B("Niente è cambiato: la fase non è di questo componente.");
        const key = g("key") && c.prep[g("key")] ? g("key") : (g("key") ? g("key") : chiavePrep(T, c, k, fid));
        const v = c.prep[key] = Object.assign(c.prep[key] || {k, on:false, costo:"", settimane:"", stato:"da fare", nata:fid}, {k});
        if(g("costo")!=null && g("costo")!=="" && !(num(g("costo"))>=0)) return NO_B(`Niente è cambiato: il costo «${g("costo")}» non è un numero.`);
        for(const kk of ["on","stato","costo","settimane","riferimento","nome","nota"]) if(g(kk)!=null) v[kk] = kk==="on" ? g(kk)==="1" : g(kk);
        if(fid) v.nata = fid;
        if(v.on && !v.storico && r){ const st = storicoPrep(r.cod, k); if(st) v.storico = {rif:st.rif, rfq:st.rfq}; }
        if(g("costo")!=null || g("settimane")!=null){ v.fonte = "L"; v.fonteIl = B_OGGI; v.chi = v.chi || S.ruolo; }
        if(g("stato")==="esiste già"){ const st = storicoPrep(r?r.cod:"", k); v.costo = "0"; v.fonte = st ? "STORICO" : "L"; v.fonteIl = B_OGGI; v.riferimento = g("riferimento") || (st ? st.rif : v.riferimento || "esistente (da scrivere dove)"); }
        if(g("stato")==="da fare" && v.costo==="0" && v.fonte==="STORICO"){ v.costo = ""; v.riferimento = ""; v.fonte = "L"; }
        { const aperte = richiesteDellaPrep(T, cid, key).filter(q=>q.stato!=="annullata" && !v.rispDa);
          if(g("on")==="0" && aperte.length){ v.on = true; return NO_B(`Niente è cambiato: ${nomePrep(k).toLowerCase()} ha una richiesta aperta (${aperte.map(q=>q.id).join(", ")}): annullala prima di togliere la preparazione.`); } }
        v.proposta = false;
        const nome = k==="altra" ? (v.nome||"altra") : nomePrep(k);
        return SI_B(`${nome}: ${v.on?"da realizzare":"tolta"}${v.stato==="esiste già"?" · esiste già · "+(v.riferimento||""):v.costo?" · "+v.costo+" €":" · da quotare"}${v.settimane?" · "+v.settimane+" sett.":""}.`);
      }
      case "cicloConferma": {
        const cid = m[1], r = rigaB(T,p,cid), c = cicloB(T,cid);
        if(!r||!r.cid) return NO_B("Niente è cambiato: il ciclo si lega a un componente confermato (RV-27).");
        if(!c || !c.fasi.length) return NO_B("Niente è cambiato: il ciclo è vuoto.");
        /* P-06: si conferma QUELLO CHE SI È LETTO: la firma copre fasi, campi e proposte mostrate (come la Sequenza) */
        if(String(g("firma")||"")!==firmaCiclo(T,cid)) return NO_B("Niente è cambiato: il ciclo è cambiato dopo la lettura: rileggi e conferma di nuovo.");
        const prima = JSON.stringify(c), rifiuta = t => { S2.cicli[cid] = JSON.parse(prima); return NO_B(t); };
        if(g("accettaTutto")==="1"){ c.fasi.forEach(f=>Object.keys(f.prop).forEach(k=>{ const pr = propBlocco(f,k); if(pr) scriviCampo(T,p,cid,c,f,k,pr[0],pr[1]); }));
          if(c.grezzo && Object.keys(c.grezzo.prop).length && !grezzoAccettato(c)) Object.keys(c.grezzo.prop).forEach(k=>{ if(c.grezzo.par[k]==null) c.grezzo.par[k]=c.grezzo.prop[k][0]; }); }
        const ko = controlliB(T,p).filter(x=>x.cid===cid && x.liv==="KO" && !x.inventato);
        if(ko.length) return rifiuta(`Niente è cambiato: ${ko.map(x=>x.cod+" "+x.testo).join(" · ")}. Il rosso blocca la conferma di questo componente, non della RFQ.`);
        const pend = c.fasi.reduce((a,f)=>a+Object.keys(f.prop).filter(k=>propBlocco(f,k)).length,0);
        if(pend) return rifiuta(`Niente è cambiato: ${pend} proposte ◇ ancora da accettare (Maiusc+Invio sulla fase, oppure «Accetta le proposte e conferma»).`);
        const manca = c.fasi.map((f,i)=>({f,i,s:statoFase(T,p,cid,f)})).filter(x=>x.s.manca.length);
        if(manca.length) return rifiuta(`Niente è cambiato: mancano dati ★ per Franco · ${manca.map(x=>`f.${(x.i+1)*10} ${x.s.manca.join(", ")}`).join(" · ")}.`);
        if(c.fasi.some(f=>CON_GREZZO.has(f.p)) && !grezzoAccettato(c) && (SEMI[r.cod]||{}).modo!=="fuori") return rifiuta("Niente è cambiato: manca il grezzo nella prima fase.");
        c.stato = "confermato"; c.conferme++; c.quando = B_OGGI; delete c.riaperto;
        return SI_B(`Ciclo di ${r.cod} confermato.`);
      }
      case "cicloRiapri": { const c = cicloB(T,m[1]); if(!c) return NO_B("Niente è cambiato."); c.stato="bozza"; c.riaperto="riaperto da Lino"; return SI_B("Ciclo riaperto in bozza."); }
      case "cicloCopia": {
        const cid = m[1], r = rigaB(T,p,cid); if(!r||!r.cid) return NO_B("Niente è cambiato: il ciclo si lega a un componente confermato (RV-27).");
        if(String(g("da")||"").startsWith("rev:")){ const sr = STORICO_REV[r.cod], seme = SEMI[r.cod]; if(!sr || !seme) return NO_B("Niente è cambiato: nessuna revisione precedente nello storico.");
          S2.come[cid] = S2.come[cid] || {modo:seme.modo, da:"Lino", modello:seme.modello};
          const c = creaCiclo(T,p,r,seme.modello,seme); c.fasi.forEach(f=>Object.keys(f.prop).forEach(k=>{ f.prop[k] = [f.prop[k][0], "⧉"]; })); c.fonte = `⧉ rev. ${sr.rev}`; c.copiaDa = `rev. ${sr.rev}`; c.delta = sr.delta;
          return SI_B(`⧉ Ciclo copiato dalla rev. ${sr.rev} (${sr.rif}) in bozza. Delta: ${sr.delta}. Tocca solo ciò che cambia.`); }
        const da = righeB(T,p).find(x=>x.cod===g("da") || x.cid===g("da")); const cs = da && da.cid ? cicloB(T,da.cid) : null;
        if(!cs) return NO_B("Niente è cambiato: il componente da copiare non ha un ciclo.");
        S2.come[cid] = S2.come[cid] || {modo:(comeDi(T,da.cid)||{}).modo||"noi", da:"Lino", modello:cs.modello};
        copiaCiclo(T,p,r,cs,da.cod);
        return SI_B(`⧉ Ciclo copiato da ${da.cod} in bozza: macchine e qualità tenute, driver svuotati (metri, fori, filetti, pieghe). Niente diventa confermato copiando.`);
      }
      case "nota": { const cid = m[1]; S2.note[cid] = S2.note[cid]||[]; const t = String(g("testo")||"").trim(); if(!t) return NO_B("Niente è cambiato: la nota è vuota.");
        S2.note[cid].push({n:S2.note[cid].length+1, testo:t, fase:g("fase")||""}); return SI_B(`Nota ${S2.note[cid].length} sul disegno${g("fase")?", legata alla fase aperta":""} (R-14).`); }
      case "richiestaFornitore": {
        /* la rotta di Pietro (richieste.go, nuovaRichiestaFornitore): UNA riga per (RFQ, fornitore, lavorazione). Con bozza=1
           prepara la bozza in Outlook con il marcatore. Le frasi di rifiuto sono le sue. */
        const fid = String(g("fornitore_id")||""), forn = TERZISTI.find(t=>t.id===fid) || FORNITORI_MP.find(t=>t.id===fid);
        if(!fid) return NO_B("Scegli un fornitore.");
        if(!forn) return NO_B("Fornitore non trovato.");
        const lav = String(g("lavorazione")||"").trim() || null, note = String(g("note")||"");
        if(S2.richieste.some(q=>q.forn && q.forn.id===fid && (q.lavorazione||"")===(lav||""))) return NO_B(`Richiesta non creata: esiste già una richiesta a ${forn.nome} per questa RFQ e questa lavorazione.`);
        /* RL-11: i codici si spuntano per fornitore (cod_*); senza spunte valgono quelli del gesto */
        const spunte = [...campi.keys()].filter(k=>/^cod_/.test(k)).map(k=>campi.get(k)).filter(Boolean);
        const via = [...campi.keys()].filter(k=>/^via_/.test(k)).map(k=>campi.get(k)).filter(Boolean);
        const codici = (g("scelta")==="1" ? spunte : String(g("codice")||"").split(/\s+/).filter(Boolean)).filter(c=>!via.includes(c));
        if(!codici.length) return NO_B("Niente è cambiato: a questo fornitore non resta nessun codice da chiedere.");
        const key = (note.match(RIF_NOTE)||[])[1] || null, b = key ? bisogniRichieste(T,p).find(x=>x.key===key) : null;
        const voci = b ? b.voci.filter(v=>codici.includes(v.cod) && !v.fatto && (!v.k || !forn.prep || forn.prep.includes(v.k))) : [];
        const L = lottiDi(T), tipo = b ? b.tipo : "terzista";
        const cods = [...new Map(voci.map(v=>[v.cid, {cid:v.cid, cod:v.cod, descr:v.descr, qta:v.qta, fid:v.fid, lav, grezzo:v.grezzo||null, fb:v.fb||null}])).values()];
        const preps = voci.filter(v=>v.prep).map(v=>({cid:v.cid, key:v.prep, k:v.k, fid:v.fid}));
        const q = {id:"rq-"+(++S2.nR), canale:"fornitore", tipo, bisogno:key, forn:{id:fid, nome:forn.nome}, lavorazione:lav, codici: cods.length ? cods : codici.map(c=>({cid:null, cod:c, qta:1})),
          preps, note, stato:"bozza", creata:B_OGGI, scad:giorniLavorativi(3), lotti:L.lotti.join(" / "), cert:certRichiesto(T)||"", giri:[],
          nataDa: b ? voci.map(v=>`${v.dove} · ${v.cod}`).join(" · ") : "",
          allegati: preps.length ? [...new Set(preps.flatMap(x=>{ const r = rigaB(T,p,x.cid); return [r && r.pdf ? r.pdf : `${r?r.cod:""} (PDF del pezzo)`, ...(ha3D(T) ? [`${r?r.cod:""} (3D)`] : []), ...(x.k==="maschera" && r && r.figli.length ? [(righeB(T,p).find(y=>y.radice)||{}).pdf || "PDF del prodotto finito"] : [])]; }))]
            /* RL-11: alle richieste dei pezzi vanno i PDF confermati dei codici della carta (letti dalla voce 2D della zona A) */
            : [...new Set(cods.map(c=>{ const v = c.cid ? indiceDi(T).Voce(p.rif, c.cid, "disegno_2d") : null, d = v && v.esito==="presente" && v.documento_id ? Object.values(T.documenti).find(x=>x.id===v.documento_id) : null;
                return d ? d.nome : `${c.cod} (2D da confermare)`; }))],
          lottiCod: Object.fromEntries(cods.map(c=>{ const x = c.fid ? faseDelThread(T,c.fid) : null; return [c.cid, x ? (valF(x.f,"lotti")||valF(x.f,"lotto")||"") : ""]; }))};
        S2.richieste.push(q);
        /* RL-19: il fornitore a cui si chiede entra nei «Terzisti» della fase come proposta ◇ (la scelta resta di Lino) */
        for(const c of cods){ const x = c.fid ? faseDelThread(T, c.fid) : null; if(!x || !(PROC[x.f.p] && PROC[x.f.p][1]==="est")) continue;
          const att = String(valF(x.f,"terzi")||"").split(",").filter(Boolean); if(att.includes(fid)) continue;
          const pr = String((x.f.prop.terzi||[""])[0]||"").split(",").filter(Boolean); if(!pr.includes(fid)) pr.push(fid); x.f.prop.terzi = [pr.join(","), "RICH"]; }
        S2.posta.push({rid:q.id, a:forn.nome, oggetto:`${TIPI_RICHIESTA[tipo].nome} · ${q.codici.map(x=>x.cod).join(", ")}`, data:B_OGGI, da:S.ruolo, bozza:true});
        return Object.assign(SI_B(`Richiesta a ${forn.nome} creata.${g("bozza")==="1"?" La bozza è in Outlook con il marcatore: parte quando la mandi da lì, e il sync la lega da solo.":" Quando la mail parte da Outlook, il sync la lega da solo se ha il marcatore; altrimenti confermala dall'Inbox."}`), {rid:q.id});
      }
      case "richiestaAnnulla": {
        const q = S2.richieste.find(x=>x.id===m[2] && x.forn); if(!q) return NO_B("Richiesta non trovata in questa RFQ.");
        q.stato = "annullata"; q.annullata = B_OGGI;
        return SI_B("Richiesta annullata. La mail, se è partita, resta nel suo quadrante.");
      }
      case "domanda": {
        /* proposta R-28 · R-29 · R-41: un messaggio interno (officina, qualità, commerciale) o al cliente (lo manda il commerciale) */
        const tipo = g("tipo"), TR = TIPI_RICHIESTA[tipo]; if(!TR || TR.esterni) return NO_B("Niente è cambiato: tipo di domanda non previsto.");
        let prep = g("prep") || null; const cid = g("cid") || null, r = cid ? rigaB(T,p,cid) : null, cy = r ? cicloB(T,r.cid) : null;
        if(prep || g("accendi")==="1"){
          if(!r || !cy) return NO_B("Niente è cambiato: la preparazione sta nel ciclo di un componente confermato.");
          if(g("accendi")==="1"){ const k = g("k"); if(!PREP_TIPI.some(x=>x[0]===k)) return NO_B("Niente è cambiato: preparazione non prevista."); prep = accendiPrep(T, r, cy, k, g("fid")||null); }
          const v = cy.prep[prep]; if(!v || !v.on) return NO_B("Niente è cambiato: la preparazione non è accesa. Prima «da fare».");
          if(v.stato==="esiste già") return NO_B(`Niente è cambiato: ${nomePrep(v.k).toLowerCase()} esiste già (${v.riferimento||"storico"}): non serve chiederla.`);
          if(!(PREP_INFO[v.k]||PREP_INFO.altra).rich.includes(tipo)) return NO_B(`Niente è cambiato: ${nomePrep(v.k).toLowerCase()} si chiede a ${(PREP_INFO[v.k]||{}).chi||"chi la fa"}.`);
        }
        const cids = String(g("cids")||cid||"").split(",").filter(Boolean);
        const cods = cids.map(x=>rigaB(T,p,x)).filter(Boolean).map(x=>({cid:x.cid, cod:x.cod, descr:x.descr, qta:x.qtaProd, fid:g("fid")||null}));
        const pv = prep && cy ? cy.prep[prep] : null;
        const pr = pv ? {cid:r.cid, key:prep, k:pv.k, fid:pv.nata||null} : null;
        const testo = g("testo") || (pv ? frasePrep(T,p,r,cy,pv) : "");
        const gia = S2.richieste.find(q=>!q.forn && q.stato==="bozza" && q.tipo===tipo);
        if(gia){ for(const c of cods) if(!gia.codici.some(x=>x.cid===c.cid)) gia.codici.push(c); if(pr && !gia.preps.some(x=>x.cid===pr.cid && x.key===pr.key)) gia.preps.push(pr);
          if(testo && !gia.testo.includes(testo)) gia.testo = [gia.testo, testo].filter(Boolean).join("\n");
          return Object.assign(SI_B(`Aggiunto alla bozza già preparata per ${TR.nomeDest}: una mail per destinatario.`), {rid:gia.id}); }
        const q = {id:"rq-"+(++S2.nR), canale:"interno", tipo, dest:{id:TR.dest, nome:TR.nomeDest}, codici:cods, preps: pr ? [pr] : [], testo, stato:"bozza", creata:B_OGGI, lotti:lottiDi(T).lotti.join(" / "), giri:[],
          nataDa: pr ? `${nomePrep(pr.k).toLowerCase()} · ${r.cod}` : ""};
        S2.richieste.push(q);
        S2.posta.push({rid:q.id, a:TR.nomeDest, oggetto:`${TR.nome} · ${cods.map(x=>x.cod).join(", ")||"RFQ"}`, data:B_OGGI, da:S.ruolo, bozza:true});
        return Object.assign(SI_B(`Bozza preparata per ${TR.nomeDest}${cods.length?" · "+cods.map(x=>x.cod).join(", "):""}. Parte quando la mandi da Outlook: niente parte da solo.`), {rid:q.id});
      }
      case "domandaAnnulla": { const q = S2.richieste.find(x=>x.id===m[1] && !x.forn); if(!q) return NO_B("Richiesta non trovata in questa RFQ."); q.stato = "annullata"; q.annullata = B_OGGI; return SI_B("Richiesta annullata. La mail, se è partita, resta nel suo quadrante."); }
      case "simSync": {
        const q = S2.richieste.find(x=>x.id===m[1]); if(!q || q.stato!=="bozza") return NO_B("Niente è cambiato: solo una bozza può partire.");
        q.stato = "inviata"; q.inviata = B_OGGI; q.da = S.ruolo;
        S2.posta.push({rid:q.id, a:q.forn?q.forn.nome:q.dest.nome, oggetto:`${TIPI_RICHIESTA[q.tipo].nome} · ${q.codici.map(x=>x.cod).join(", ")||"RFQ"}`, data:B_OGGI, da:S.ruolo});
        if(q.dest && (q.dest.id==="cliente" || q.dest.id==="commerciale")){
          const rr = T.req ? rq(T.req) : null;
          if(rr) MESSAGGI.push({mid:"pa-"+q.id, req:rr.id, prod:rr.prodotti.slice(), d:"oggi", t:"adesso", dir:"out", chi: q.dest.id==="cliente"?"Franco":"Lino", r: q.dest.id==="cliente"?"commerciale · noi":"ufficio tecnico · noi",
            sub:`${TIPI_RICHIESTA[q.tipo].nome} — ${q.codici.map(x=>x.cod).join(", ")}`, txt:`<p>${esc(q.testo||TIPI_RICHIESTA[q.tipo].nome)}</p><p>Codici: ${q.codici.map(x=>`<mark class="hl">${esc(x.cod)}</mark>`).join(", ")||"—"}</p>`,
            meta:"mail partita da Outlook con il marcatore (proposta R-41): il sync l'ha legata alla richiesta"});
          q.nelThread = !!rr;
        }
        return SI_B(`(mockup) La mail è partita da Outlook: il sync l'ha trovata nella Posta inviata e la richiesta è «inviata»${q.nelThread?" · la mail è nella conversazione del thread":""}. Scadenza ${q.scad||"—"}.`);
      }
      case "simRisposta": {
        const q = S2.richieste.find(x=>x.id===m[1]); if(!q || q.stato!=="inviata") return NO_B("Niente è cambiato: la richiesta non è in attesa di risposta.");
        const id = q.forn ? q.forn.id : q.dest.id;
        const sim = (q.preps && q.preps.length) ? rispostaPrepSim(T, q, id) : (RISPOSTE_SIM[q.tipo+"|"+id] || {testo:"risposta ricevuta"});
        const ris = copia(sim);
        if(q.tipo==="terzista" && id==="mrt" && q.giri.length) ris.prezzo = +(ris.prezzo - 0.25*q.giri.length).toFixed(2);
        if(sim.declina){ q.stato = "declinata"; q.risp = {declina:sim.declina, data:B_OGGI}; }
        else { q.stato = q.forn ? "offerta_ricevuta" : "risposta"; q.risp = Object.assign(ris, {data:B_OGGI, ritardo:"simulata: nel vero arriva in 2–5 giorni"}); q.controlli = controlliRisposta(T,p,q); }
        return SI_B(`(mockup) ${q.forn?q.forn.nome:q.dest.nome} ${sim.declina?"declina":"risponde"}: nel vero la mail si aggancia alla richiesta dall'Inbox, con l'atto che conferma una persona.${sim.declina?"":" Registrala nel punto d'origine."}`);
      }
      case "simScadenza": { const q = S2.richieste.find(x=>x.id===m[1]); if(!q || !q.forn || q.stato!=="inviata") return NO_B("Niente è cambiato: solo una richiesta a un fornitore inviata può scadere.");
        q.stato = "scaduta"; return SI_B(`(mockup) Passata la scadenza del ${q.scad}: il server segna la richiesta «scaduta» (R-27). In Esito diventa DUBBIO, «si quota con riserva»${q.preps&&q.preps.length?" (PREP-02)":""}.`); }
      case "richiestaGiro": {
        /* proposta R-17: un nuovo giro sulla stessa riga (l'indice UNIQUE di 0015 non permette una riga nuova) */
        const q = S2.richieste.find(x=>x.id===m[1]); if(!q || !["offerta_ricevuta","declinata","scaduta","risposta"].includes(q.stato)) return NO_B("Niente è cambiato: un nuovo giro si apre dopo una risposta o una scadenza.");
        q.giri.push({n:q.giri.length+1, data:q.inviata||B_OGGI, rispData: q.risp ? q.risp.data : null, stato:q.stato, risp:q.risp?copia(q.risp):null, reg:q.regTesto||null, nota:g("nota")||"un altro lotto"});
        q.stato = "bozza"; q.risp = null; q.controlli = []; q.inviata = null;
        return SI_B(`Nuovo giro (proposta R-17): una bozza nuova in Outlook; i ${q.giri.length} giri precedenti restano.`);
      }
      case "risposta": {
        /* R-13: la risposta si registra nel PUNTO D'ORIGINE (preparazione, fase, grezzo) con la fonte RISP e il riferimento.
           La richiesta non cambia stato: «registrata» è un fatto della preparazione o della fase, non della richiesta */
        const q = S2.richieste.find(x=>x.id===g("richiesta")); if(!q) return NO_B("Niente è cambiato: richiesta non trovata.");
        if(!["offerta_ricevuta","risposta"].includes(q.stato) || !q.risp || q.risp.declina) return NO_B("Niente è cambiato: non c'è un'offerta da registrare.");
        if(registrazioneDi(T,q)) return NO_B(`Niente è cambiato: è già registrata (${registrazioneDi(T,q)}).`);
        const d = {nome: q.forn ? q.forn.nome : q.dest.nome, id: q.forn ? q.forn.id : q.dest.id};
        q.regGiro = q.giri.length+1; q.regTesto = null;
        if((q.controlli||[]).some(x=>x.cod==="MAT-05") && g("forza")!=="1") return NO_B(`Niente è cambiato: ${q.controlli.find(x=>x.cod==="MAT-05").testo}. Registra un'altra risposta o chiedi alla qualità.`);
        const regs = [];
        if(q.preps && q.preps.length){
          for(const x of q.preps){ const cy = cicloB(T,x.cid), v = cy && cy.prep[x.key]; if(!v) continue;
            const riga = (q.risp.righe||[]).find(y=>y.key===x.key && y.cid===x.cid) || {prezzo:q.risp.prezzo, settimane:q.risp.settimane};
            if(riga.prezzo==null) continue;
            v.costo = String(riga.prezzo); v.settimane = riga.settimane!=null ? String(riga.settimane) : (v.settimane||""); v.chi = d.nome; v.fonte = "RISP"; v.fonteIl = B_OGGI; v.rispDa = q.id; v.stato = "da fare";
            const rr = righeB(T,p).find(y=>y.cid===x.cid); regs.push(`${rr?rr.cod:x.cid} ${nomePrep(v.k).toLowerCase()} ${fmt(riga.prezzo,0)} €${riga.settimane?" · "+riga.settimane+" sett.":""}`); }
          if(q.tipo==="officina" && q.risp.testo) for(const c of q.codici) S2.capOfficina = Object.assign(S2.capOfficina||{}, {[c.cid]:q.risp.testo});
          if(regs.length) return SI_B(`Registrata nella preparazione con la fonte RISP del ${B_OGGI}: ${regs.join(" · ")}. Il costo arriva da solo nell'Offerta.`);
        }
        for(const c of q.codici){
          const cy = c.cid ? cicloB(T,c.cid) : null; if(!cy) continue;
          if(/^mp_/.test(q.tipo)){ const gr = grezzoDi(grezzoVal(cy)); if(gr){ S2.prezziMP[gr.id] = {eur:q.risp.prezzo, forn:d.nome, data:B_OGGI, materiale:q.risp.materiale, cid:c.cid, rid:q.id}; regs.push(`${c.cod} grezzo ${fmt(q.risp.prezzo,2)} €/kg`); } }
          else if(!q.forn || q.risp.prezzo==null){
            if(q.tipo==="officina") S2.capOfficina = Object.assign(S2.capOfficina||{}, {[c.cid]:q.risp.testo});
            S2.noteRisp = S2.noteRisp||[]; S2.noteRisp.push({cid:c.cid, tipo:q.tipo, testo:q.risp.testo||`${fmt(q.risp.prezzo,2)} ${q.risp.unita||""}`, data:B_OGGI, rid:q.id});
            regs.push(`${c.cod}: ${q.tipo==="officina"?"lavorabilità aggiornata":"nota registrata"}`); }
          else { const f = cy.fasi.find(x=>x.id===c.fid) || cy.fasi.find(x=>PROC[x.p] && PROC[x.p][1]==="est");
            if(f){ f.risp = {da:d.nome, terz:d.id, prezzo:q.risp.prezzo, unita:q.risp.unita, lotto:q.risp.lotto, data:B_OGGI, rid:q.id};
              if(PROC[f.p] && PROC[f.p][1]==="est"){ const att = (valF(f,"terzi")||"").split(",").filter(Boolean); if(!att.includes(d.id)) scriviCampo(T,p,c.cid,cy,f,"terzi",[...att,d.id].join(","),"RISP"); }
              QUOTAZIONI_B.push({id:"qr"+QUOTAZIONI_B.length, tk:T.key, terz:d.id, cod:c.cod, p:f.p, lotti:[[parseInt(q.risp.lotto,10)||1, q.risp.prezzo]], data:B_OGGI, f:"risp", rif:`risposta alla ${q.id}`, materiale:q.risp.materiale||"", rid:q.id, giro:q.giri.length+1});
              if(q.risp.trasporto){ const tr = cy.fasi.find(x=>x.p==="trasporto"); if(tr) tr.risp = {da:d.nome, terz:d.id, prezzo:q.risp.trasporto, unita:"€ a viaggio", data:B_OGGI, rid:q.id}; }
              regs.push(`${c.cod} f.${(cy.fasi.indexOf(f)+1)*10} ${fmt(q.risp.prezzo,2)} ${q.risp.unita||""}`); }
          }
        }
        if(q.tipo==="commerciale"){ (REQUISITI[T.key]||[]).filter(x=>x.avvisa).forEach(x=>S2.visti[x.id]={da:"commerciale", data:B_OGGI, rid:q.id}); S2.noteRisp = S2.noteRisp||[]; S2.noteRisp.push({cid:null, tipo:"commerciale", testo:q.risp.testo||"", data:B_OGGI, rid:q.id}); regs.push("condizione vista dal commerciale"); }
        if(!regs.length){ S2.noteRisp = S2.noteRisp||[]; S2.noteRisp.push({cid:null, tipo:q.tipo, testo:q.risp.testo||"", data:B_OGGI, rid:q.id}); regs.push("risposta registrata come nota"); }
        return SI_B(`Registrata nel punto d'origine con la fonte RISP del ${B_OGGI}: ${regs.join(" · ")}.`);
      }
      case "faseComponente": {
        /* un figlio entra in QUESTA fase di unione (e lascia le altre dello stesso ciclo), oppure esce; «dividi» = solo una parte (qta) */
        const x = faseDelThread(T, m[1]); if(!x) return NO_B("Niente è cambiato: fase non trovata. Rileggi.");
        const r = rigaB(T,p,x.cid); if(!r) return NO_B("Niente è cambiato: l'assieme non è nella BOM confermata.");
        if(!UNIONE.has(x.f.p)) return NO_B(`Niente è cambiato: in ${PROC[x.f.p][0].toLowerCase()} non entra nessun figlio. I figli entrano in una fase di unione o di montaggio.`);
        const cid = g("cid"), fg = r.figli.find(y=>y.cid===cid);
        if(!fg) return NO_B("Niente è cambiato: non è un figlio di questo assieme nella BOM confermata (SEQ-01). Se manca nell'albero, proponilo in Struttura (R-38).");
        const u = unioniDi(x.c), entra = g("entra")!=="0";
        if(entra && g("dividi")==="1"){ const qn = num(g("qta")); if(g("qta")==null || String(g("qta")).trim()==="" || !(qn>0) || qn!==Math.round(qn)) return NO_B("Niente è cambiato: scrivi quanti pezzi entrano in questa fase (un numero intero, più di zero)."); if(qn>fg.qta) return NO_B(`Niente è cambiato: ${qn} è più di ${fg.qta} della BOM confermata.`);
          x.f.entrano = (x.f.entrano||[]).filter(e=>e.cid!==cid); x.f.entrano.push({cid, qta:qn, fonte:"L"});
          seqScritta(T, x.c);
          const somma = u.reduce((a,f)=>a+(f.entrano||[]).filter(e=>e.cid===cid).reduce((b,e)=>b+(e.qta==null?fg.qta:e.qta),0),0);
          return SI_B(`${fg.cod}: ${qn} su ${fg.qta} nella f.${(x.i+1)*10}${somma===fg.qta?" · le quantità tornano":` · in tutte le fasi ${somma} su ${fg.qta}: dividi anche l'altra parte (SEQ-08)`}.`); }
        const daF = u.filter(f=>f!==x.f && (f.entrano||[]).some(e=>e.cid===cid)), prima = daF.map(f=>`f.${(x.c.fasi.indexOf(f)+1)*10}`);
        if(entra){ for(const f of u) f.entrano = (f.entrano||[]).filter(e=>e.cid!==cid); x.f.entrano = x.f.entrano||[]; x.f.entrano.push({cid, qta:null, fonte:"L"}); }
        else { if(!(x.f.entrano||[]).some(e=>e.cid===cid)) return NO_B(`Niente è cambiato: ${fg.cod} non entra in questa fase.`); x.f.entrano = x.f.entrano.filter(e=>e.cid!==cid); }
        seqScritta(T, x.c);
        const nome = valF(x.f,"nome") || PROC[x.f.p][0];
        /* M6: l'avviso porta «Annulla», che è la scrittura inversa sulla stessa rotta (nessuna rotta nuova) */
        const annulla = entra ? (daF.length ? {url:`/fase/${daF[0].id}/componente`, campi:{cid, entra:"1"}} : {url:`/fase/${x.f.id}/componente`, campi:{cid, entra:"0"}}) : {url:`/fase/${x.f.id}/componente`, campi:{cid, entra:"1"}};
        return Object.assign(SI_B(entra ? `${fg.cod} ×${fg.qta} entra nella f.${(x.i+1)*10} ${nome}${prima.length?` (spostato dalla ${prima.join(", ")}: una scrittura sola)`:""}.` : `${fg.cod} esce dalla f.${(x.i+1)*10}: ora non entra in nessuna fase (SEQ-01).`), {annulla});
      }
      case "faseProteggi": {
        /* P-12: «da proteggere» è un gesto: il server sceglie le fasi di trattamento del padre e aggiunge o toglie il figlio.
           Il dato va in R-33 (o in una colonna di fase_componente): niente lista scritta dal JS */
        const x = faseDelThread(T, m[1]); if(!x) return NO_B("Niente è cambiato: fase non trovata. Rileggi.");
        const r = rigaB(T,p,x.cid), cid = g("cid"), fg = r ? r.figli.find(y=>y.cid===cid) : null;
        if(!fg) return NO_B("Niente è cambiato: non è un figlio di questo assieme nella BOM confermata.");
        const tratt = x.c.fasi.filter(f=>TRATT.has(f.p));
        if(!tratt.length) return NO_B("Niente è cambiato: il padre non ha un trattamento, non c'è niente da proteggere.");
        const on = g("on")==="1";
        for(const f of tratt){ const l = String(valF(f,"proteggi")||"").split(",").filter(Boolean).filter(y=>y!==cid); if(on) l.push(cid); if(l.length) f.par.proteggi = l.join(","); else delete f.par.proteggi; }
        seqScritta(T, x.c);
        return SI_B(`${fg.cod}: ${on?"da proteggere":"non più da proteggere"} nel trattamento (${tratt.map(f=>PROC[f.p][0].toLowerCase()).join(", ")}).`);
      }
      case "seqRipristina": {
        /* P-05, P-09: UNA rotta per prodotto, con la firma di quello che si è letto. Il server decide quali assiemi tornano
           alla proposta (quelli con fonte_sequenza = scritta) e ricalcola il seme (R-35): niente fotografia salvata */
        const pr = prodottoDaCid(T, m[1]); if(!pr) return NO_B("Niente è cambiato: prodotto non trovato. Rileggi.");
        if(String(g("firma")||"")!==firmaSequenza(T,pr)) return NO_B("Niente è cambiato: la sequenza è cambiata dopo la lettura: rileggi e conferma di nuovo.");
        const piano = pianoRipristino(T, pr);
        if(!piano.assiemi.length) return NO_B("Niente è cambiato: la sequenza è già la proposta del sistema.");
        applicaRipristino(T, pr, piano);
        return SI_B(`Torna la proposta del sistema${piano.togli.length?` · tolte: ${piano.togli.map(x=>x.testo).join(", ")}`:""}${piano.aperte.length?` · restano aperte: ${piano.aperte.map(x=>x.testo).join(", ")} (PREP-04: la loro fase non c'è più)`:""}.`);
      }
      case "seqConferma": {
        /* come «Conferma l'albero»: si conferma QUELLO CHE SI È LETTO, con la firma. Lo stato è del prodotto (P-08) */
        const pr = prodottoDaCid(T, m[1]) || p;
        const sd = stadiDi(T,pr); if(!sd) return NO_B("Niente è cambiato: il prodotto non ha componenti.");
        if(!sd.root.cid || !sd.ciclo || !sd.ciclo.fasi.length) return NO_B(`Niente è cambiato: il ciclo di ${sd.root.cod} è vuoto. La sequenza si conferma sulle fasi del prodotto (SEQ-01).`);
        if(String(g("firma")||"")!==firmaSequenza(T,pr)) return NO_B("Niente è cambiato: la sequenza è cambiata dopo la lettura: rileggi e conferma di nuovo.");
        const bloc = controlliB(T,pr).filter(x=>(x.cod==="SEQ-01" || x.cod==="SEQ-08") && x.liv!=="AV");
        if(bloc.length) return NO_B(`Niente è cambiato: ${bloc.map(x=>x.cod+" "+x.testo).join(" · ")}. Sistemali nell'albero del processo (filtro «solo da sistemare»).`);
        S2.seq[pr.rif] = {stato:"confermata", quando:B_OGGI, firma:firmaSequenza(T,pr), da:S.ruolo||"Lino", fonte:fonteSequenza(T,pr), prodotto:pr.rif};
        return SI_B(`Sequenza di ${pr.codice_richiesto} confermata (${S2.seq[pr.rif].fonte==="L"?"scritta da te":"proposta del sistema"}) con la firma di quello che hai letto. L'albero della BOM non cambia.`);
      }
      case "assiemeInterno": {
        const cod = String(g("codice")||"").trim();
        if(!cod) return NO_B("Niente è cambiato: serve il codice dell'assieme, e lo scrive una persona (DB7-7).");
        S2.proposteAlbero.push({tipo:"assieme", cid:m[1], codice:cod, data:B_OGGI, r:"R-31"});
        return SI_B("Proposta registrata: l'albero non cambia. Quando il backend avrà R-38 la vedrai in Struttura, e l'albero tornerà da confermare.");
      }
      case "nodoProposto": {
        const d = String(g("descr")||"").trim(); if(!d) return NO_B("Niente è cambiato: scrivi che cosa manca.");
        S2.proposteAlbero.push({tipo:"manca", descr:d, padre:g("padre")||"", qta:g("qta")||"", data:B_OGGI, r:"R-38"});
        return SI_B("Proposta registrata: l'albero non cambia. Quando il backend avrà R-38 la vedrai in Struttura, e l'albero tornerà da confermare.");
      }
      case "componenteManuale": {
        /* R-38 precisata (proposta · dopo A2): senza STEP Lino aggiunge un componente sotto un padre, scrivendo lui il codice
           (DB7-7, R1: mai dal nome del file né dal cartiglio). Nel backend sono pezzi che ci sono già: InsertComponente con
           origine «manuale», ControllaCodiceNuovo, Collega (fascicolo/modifiche.go). Il finto server B li simula sui dati del
           finto server A: il componente esce in confermati_senza_step e l'albero torna da confermare («Conferma l'albero»).
           È l'unico gesto B che scrive la zona A, ed è marcato R-38. Niente bom/applica (DB7-5 A). */
        if(!p || !p.componente_id) return NO_B("Niente è cambiato: il prodotto non ha ancora il suo componente.");
        if(p.nodi.length) return NO_B("Niente è cambiato: questo prodotto ha la BOM di lavoro dallo STEP. I pezzi si decidono sui nodi (Accetta il nodo, Collega).");
        const cod = String(g("codice")||"").trim(), rev = String(g("rev")||"").trim(), descr = String(g("descrizione")||"").trim(), tipo = g("tipo")||"sciolto", qn = num(g("qta"));
        if(!cod) return NO_B("Niente è cambiato: scrivi il codice. Lo scrive una persona, leggendo il disegno (DB7-7).");
        if(!(qn>=1) || qn!==Math.round(qn)) return NO_B("Niente è cambiato: scrivi la quantità sotto il padre (un numero intero, almeno 1).");
        if(!["sciolto","sottoassieme","commerciale"].includes(tipo)) return NO_B("Niente è cambiato: tipo non previsto (particolare, assieme o commerciale).");
        const pad = g("padre") || p.componente_id, cp = T.componenti[pad];
        if(!cp) return NO_B("Niente è cambiato: il padre scelto non è un componente di questa RFQ.");
        if(cp.tipo==="sciolto" || cp.tipo==="commerciale") return NO_B(`Niente è cambiato: un ${etichetta("tipo_componente",cp.tipo)} non può contenere altri pezzi.`);
        if(Object.values(T.componenti).some(c=>!c.archiviato && String(c.codice).toLowerCase()===cod.toLowerCase())) return NO_B(`Niente è cambiato: ${cod} è già un componente di questa RFQ. Per metterlo anche qui usa «Collega» nella sua casella.`);
        const cid = uuidDa(T.key+":comp:"+cod+":r38");
        T.componenti[cid] = {id:cid, codice:cod, rev:rev||null, descrizione:descr||null, tipo, archiviato:false, step_strutturale:null, disegni:{primario:null, alternativi:[]}, storia:false, origine:"manuale"};
        T.relazioni.push({padre:pad, figlio:cid, qta:qn});
        bomTornaDaVerificare(T); ricalcola(T);
        const mancano = T.daSmistare.filter(f=>String(f.nome_file).replace(/\.[^.]+$/,"").trim().toLowerCase()===cod.toLowerCase()).length;
        return SI_B(`(R-38) ${cod}${rev?" rev "+rev:""} aggiunto sotto ${cp.codice} ×${qn}: è un componente confermato senza STEP. L'albero torna da confermare.${mancano?" Il suo PDF è in «Da smistare»: «Rifai lo smistamento» lo propone.":""}`);
      }
      case "rispostaRighe": {
        /* R-13 precisata: Fabio registra la risposta VERA del fornitore: righe codice × lotto × €/pz, il materiale dichiarato,
           i tempi, la data del giro e l'allegato. Scrive una quotazione con N scaglioni per codice (Listini, Costi) e la fonte
           RISP nella fase. Un materiale discorde (MAT-05) si registra lo stesso, con riserva: esce dalla scelta automatica
           (default, da confermare con Franco e Fabio). «Simula la risposta» resta solo come aiuto della prova. */
        const q = S2.richieste.find(x=>x.id===g("richiesta")); if(!q || !q.forn) return NO_B("Niente è cambiato: richiesta a un fornitore non trovata.");
        if(!["inviata","offerta_ricevuta"].includes(q.stato)) return NO_B("Niente è cambiato: la richiesta non è partita, o è chiusa. Prima la mail da Outlook.");
        if(registrazioneDi(T,q)) return NO_B(`Niente è cambiato: questo giro è già registrato (${registrazioneDi(T,q)}). Per un altro lotto apri un nuovo giro.`);
        const leggi = t => [...String(t||"").matchAll(/(\d+)\s*(?:pz)?\s*[:=]\s*(\d+(?:[.,]\d+)?)/g)].map(m=>[parseInt(m[1],10), num(m[2])]).filter(x=>x[0]>0 && x[1]!=null);
        const righe = q.codici.map(c=>({cid:c.cid, cod:c.cod, fid:c.fid, lotti: leggi(g("l_"+(c.cid||c.cod)))})).filter(x=>x.lotti.length);
        if(!righe.length) return NO_B("Niente è cambiato: scrivi almeno un prezzo, come «10: 22» o «5: 30 / 20: 8» (lotto: €/pz).");
        const data = String(g("data")||"").trim() || B_OGGI, mat = String(g("materiale")||"").trim(), gg = String(g("tempi")||"").trim(), all = String(g("allegato")||"").trim(), giro = q.giri.length+1;
        if(q.stato==="inviata"){ q.stato = "offerta_ricevuta"; }   /* nel vero: l'aggancio della mail dall'Inbox, con l'atto di una persona */
        q.risp = {righe, prezzo: righe[0].lotti[0][1], unita:"€/pz", lotto: String(righe[0].lotti[0][0]), materiale: mat, gg: gg ? num(gg) : null, tempi: gg, data, allegato: all, giro};
        q.controlli = controlliRisposta(T,p,q);
        const m5 = q.controlli.some(x=>x.cod==="MAT-05"), regs = [];
        for(const x of righe){
          const cy = x.cid ? cicloB(T,x.cid) : null; const f = cy ? (cy.fasi.find(y=>y.id===x.fid) || cy.fasi.find(y=>PROC[y.p] && PROC[y.p][1]==="est" && y.p!=="trasporto")) : null;
          const qt = {id:"qr"+QUOTAZIONI_B.length, tk:T.key, terz:q.forn.id, cod:x.cod, p: f ? f.p : (q.lavorazione||""), lotti:x.lotti.slice().sort((a,b)=>a[0]-b[0]), data, f:"risp",
            rif:`risposta alla ${q.id} · giro ${giro}${all?" · "+all:""}`, materiale:mat, tempi:gg, rid:q.id, giro, riserva: m5};
          QUOTAZIONI_B.push(qt);
          if(f){ f.risp = {da:q.forn.nome, terz:q.forn.id, prezzo:qt.lotti[0][1], unita:"€/pz", lotto:String(qt.lotti[0][0]), data, rid:q.id, giro, scaglioni:qt.lotti, materiale:mat};
            const att = String(valF(f,"terzi")||"").split(",").filter(Boolean); if(!att.includes(q.forn.id)) scriviCampo(T,p,x.cid,cy,f,"terzi",[...att,q.forn.id].join(","),"RISP"); }
          regs.push(`${x.cod}: ${qt.lotti.map(([l,e])=>`${l} pz ${fmt(e,2)} €`).join(" · ")}`);
        }
        q.regGiro = giro; q.regTesto = `giro ${giro} del ${data}: ${regs.join(" · ")}${m5?" · con riserva (MAT-05)":""}`;
        return SI_B(`(R-13) Risposta di ${q.forn.nome} registrata con la fonte RISP, giro ${giro} del ${data}: ${regs.join(" · ")}.${m5?` ${q.controlli.find(x=>x.cod==="MAT-05").testo}: resta con riserva e non entra nella scelta automatica dei costi.`:""}`);
      }
      case "cicloTrattamento": {
        const cid = m[1], r = rigaB(T,p,cid), c = cicloB(T,cid); if(!r || !c) return NO_B("Niente è cambiato: prima il ciclo del prodotto.");
        const v = g("v")||"";
        if(v && v!=="nessuno") return NO_B("Niente è cambiato: la scelta è «nessuno», oppure si aggiunge la fase di trattamento.");
        if(v && c.fasi.some(f=>TRATT.has(f.p))) return NO_B("Niente è cambiato: il ciclo ha già una fase di trattamento. Toglila prima di dire «nessuno».");
        if(v) c.trattamento = {v, fonte:"L", da:S.ruolo, il:B_OGGI}; else delete c.trattamento;
        if(c.stato==="confermato"){ c.stato = "bozza"; c.riaperto = "modificato dopo la conferma: da riconfermare"; }
        return SI_B(v ? `${r.cod}: trattamento «nessuno», consegna grezza (scelta esplicita, fonte L).` : `${r.cod}: la scelta del trattamento è tolta.`);
      }
      case "simTempo": { const n = Math.max(1, parseInt(g("giorni"),10)||7); avanzaGiorniB(n); return SI_B(`(mockup) Sono passati ${n} giorni: oggi è il ${B_OGGI}.`); }
      case "esiti": {
        const cids = String(g("cids")||"").split(",").filter(Boolean); if(!cids.length) return NO_B("Niente è cambiato: nessuna proposta OK da accettare.");
        cids.forEach(cid=>{ S2.esiti[cid] = {v:"OK", nota:(S2.esiti[cid]||{}).nota||"", da:"proposta accettata da Lino"}; });
        return SI_B(`${cids.length} esiti OK registrati, uno per riga (gesto unico esplicito, R-35). I DUBBI e i KO restano da decidere uno per uno.`);
      }
      case "firma": {
        const area = g("area"), v = g("v"), chi = {tec:"Lino", prod:"Marta", qual:"Qualità", acq:"Fabio"}[area];
        if(!chi) return NO_B("Niente è cambiato: area non prevista.");
        if(chi!=="Qualità" && S.ruolo!==chi) return NO_B(`Niente è cambiato: l'area ${({tec:"tecnica",prod:"produzione",qual:"qualità",acq:"acquisti"})[area]} la firma ${chi} (ruolo simulato: ${S.ruolo}).`);
        if(chi==="Qualità") return NO_B("Niente è cambiato: la qualità firma dal suo ruolo, che il mockup non simula (D-02).");
        const ora = oraB(); S2.firme[area] = {v, chi, quando:B_OGGI+" "+ora};
        return SI_B(`Firmato da ${chi} · ${B_OGGI} ${ora} · ${v==="positiva"?"positiva":"negativa"}.`);
      }
      case "consegna": {
        const lotti = String(g("lotti")||"").split(",").filter(Boolean).map(Number);
        if(!lotti.length) return NO_B("Niente è cambiato: spunta almeno un lotto.");
        if(!S2.firme.tec) return NO_B("Niente è cambiato: prima la firma dell'area tecnica.");
        const ko = righeB(T,p).filter(r=>r.cid && ((S2.esiti[r.cid]||{}).v || esitoProposto(T,p,r.cid).v)==="KO");
        const noq = g("noquote")==="1";
        if(ko.length && !noq) return NO_B(`Niente è cambiato: ${ko.map(r=>r.cod).join(", ")} KO. Consegna come no quote, con il motivo.`);
        /* lo stato della zona A si legge, non si calcola: prima struttura e codici (DB7-2); pronto ≠ consegnato (EB7-3) */
        const sa = statoZonaA(p);
        if(!sa.pronto && g("presaAtto")!=="1") return NO_B(`Niente è cambiato: il prodotto è «${sa.stato}» nella zona A (${sa.motivi}). Prima struttura e documenti (DB7-2), oppure spunta la presa d'atto: finirà nella fotografia.`);
        const foto = { n:S2.consegne.length, quando:B_OGGI+" "+oraB(), lotti, noquote:noq, motivo: noq ? ko.map(r=>`${r.cod}: ${(S2.esiti[r.cid]||{}).nota||"KO"}`).join(" · ") : "",
          righe: righeB(T,p), come: copia(S2.come), cicli: copia(S2.cicli),
          esiti: righeB(T,p).filter(r=>r.cid).map(r=>{ const d = (S2.esiti[r.cid]||{}).v; return {cid:r.cid, cod:r.cod, v: d||esitoProposto(T,p,r.cid).v, deciso: !!d, da: d ? ((S2.esiti[r.cid]||{}).da||"Lino") : "proposta del sistema (R-35)", nota:(S2.esiti[r.cid]||{}).nota||""}; }),
          zonaA: Object.assign({}, sa, {presaAtto: !sa.pronto}),
          prezziMP: copia(S2.prezziMP), firma: copia(S2.firme.tec), prodotto: p.rif, codice: p.codice_richiesto, firmaDistinta: firmaDistinta(T) };
        S2.consegne.push(foto);
        return SI_B(`Consegnato a Franco: fotografia della Distinta per ${lotti.length===1?"il lotto "+lotti[0]:"i lotti "+lotti.join(", ")}${noq?" · come no quote":""}. La fase registrata non cambia (RV-29).`);
      }
      case "requisito": { /* una o più condizioni in un gesto (ids): una scrittura per condizione, sul server */
        const ids = String(g("ids")||g("id")||"").split(",").filter(Boolean); if(!ids.length || !ids.every(id=>(REQUISITI[T.key]||[]).some(x=>x.id===id))) return NO_B("Niente è cambiato.");
        ids.forEach(id=>{ S2.visti[id] = {da:S.ruolo, data:B_OGGI}; }); return SI_B(ids.length===1 ? "Presa visione registrata." : `Presa visione registrata per ${ids.length} condizioni.`); }
      case "lotto": { const vv = String(g("lotto")||"").split(/[\/;,\s]+/).map(x=>parseInt(x.replace(/\./g,""),10)).filter(v=>v>0);
        if(!vv.length) return NO_B("Niente è cambiato: scrivi un lotto in pezzi (anche più scaglioni: 5/20/50/100).");
        S2.lottiExtra = S2.lottiExtra || []; vv.forEach(v=>{ if(!S2.lottiExtra.includes(v)) S2.lottiExtra.push(v); });
        return SI_B(`${vv.length===1?"Lotto "+vv[0]+" aggiunto":"Scaglioni "+vv.join(" / ")+" aggiunti"}, non spuntati: le fotografie già consegnate restano come sono.`); }
      case "offerta": return azioneOffertaB(T, m[2], g);
      case "costiCampo": return campoCostiB(T, g);
      case "datiDoc": { /* una voce (voce, v) o tutte le proposte lette in un gesto solo (voci = k=v,k=v): niente sequenze dal JS */
        if(g("voci")){ const l = String(g("voci")).split(",").map(x=>x.split("=")).filter(x=>x[0]&&x[1]); l.forEach(([k,v])=>{ S2.datiDoc[k] = v; }); return SI_B(`Dati e documenti: ${l.length} proposte accettate, una per voce.`); }
        S2.datiDoc[g("voce")] = g("v"); return SI_B(`Dati e documenti: ${g("voce")} = ${g("v")}.`); }
    }
    return NO_B("Niente è cambiato.");
  },
};
function copiaCiclo(T, p, r, cs, daCod){
  const S2 = a2Di(T), DRIVER = new Set(["m","fori","n","pieghe","lmax","tagli","L","rett","barre","colpi","curve","viti"]);
  const c = {stato:"bozza", modello:cs.modello, fasi:[], grezzo:{par:{}, prop:{}}, prep:copia(cs.prep||{}), fonte:`⧉ copiato da ${daCod}`, copiaDa:daCod, conferme:0};
  const seme = SEMI[r.cod];
  if(seme && seme.grezzo) c.grezzo.prop = copia(seme.grezzo);
  else { const gv = grezzoVal(cs); if(gv) for(const k of ["q","forma","sp"]) if(gv[k]) c.grezzo.prop[k] = [gv[k], "⧉"]; }
  for(const f of cs.fasi){ const nf = nuovaFase(S2, f.p, null);
    for(const [k,v] of Object.entries(f.par)) if(!DRIVER.has(k)) nf.prop[k] = [v, "⧉"];
    for(const [k,v] of Object.entries(f.prop)) if(!DRIVER.has(k) && nf.prop[k]==null && f.par[k]==null) nf.prop[k] = v;
    if(f.p==="laser" && seme && seme.grezzo && seme.grezzo.larg) nf.prop.rett = [`${seme.grezzo.larg[0]} × ${seme.grezzo.lung[0]}`, "STEP"];
    delete nf.prop.oggetto;
    c.fasi.push(nf); }
  S2.cicli[r.cid] = c;
  if(r.figli && r.figli.length){ c.fasi.forEach(f=>{ if(UNIONE.has(f.p)) f.entrano = []; }); semeSequenza(T,p,r,c); }
  return c;
}

/* M8 di Lino: la firma di quello che la consegna fotografa (come si fa e cicli). Se dopo la consegna cambia, il Foglio per
   Franco lo dice in sola lettura (R-40): nessun dato nuovo, si confronta con la fotografia */
function firmaDistinta(T){ const S2 = a2Di(T); return impronta(JSON.stringify([S2.come, S2.cicli])); }
function distintaCambiata(T, foto){ return !!(foto && foto.firmaDistinta && foto.firmaDistinta!==firmaDistinta(T)); }
/* aiuti di calcolo del server (anteprime, non scrivono): pezzi per barra e barre per lotto (Q-03), forza di piega */
function barraPreview(T, p, cid){
  const c = cicloB(T,cid), g = grezzoVal(c); if(!g || !/tubo|tondo|profilo/.test(g.forma||"")) return null;
  const f = c.fasi.find(x=>x.p==="sega"||x.p==="lasertubo"); if(!f) return null;
  const L = num(valF(f,"L")||(propF(f,"L")||[])[0]); if(!L) return null;
  const gr = grezzoDi(g), barra = gr && gr.barra ? gr.barra : 6000, lama = f.p==="sega"?3:1, spezzone = 50;
  const perBarra = Math.floor((barra-spezzone)/(L+lama)), r = rigaB(T,p,cid), lotto = Math.max(...lottiDi(T).lotti), pezzi = (r?r.qtaProd:1)*lotto;
  return {perBarra, barre: Math.ceil(pezzi/Math.max(1,perBarra)), testo:`⌊(${barra} − ${spezzone}) ÷ (${L} + ${lama})⌋ = ${perBarra} pz per barra · ${pezzi} pz → ${Math.ceil(pezzi/Math.max(1,perBarra))} barre`};
}
function forzaPiega(T, cid, f){
  const c = cicloB(T,cid), g = grezzoVal(c); const sp = num((g||{}).sp), L = num(valF(f,"lmax")||(propF(f,"lmax")||[])[0]); if(!sp||!L) return null;
  const Rm = /355/.test((g||{}).q||"") ? 510 : 400, V = sp<=3?24:sp<=6?50:80, t = 1.33*sp*sp*Rm/V*L/1000/9.81;
  return {t, testo:`1,33 × ${sp}² × Rm ${Rm} ÷ V${V} × ${L} mm ≈ ${fmt(t,1)} t`};
}
/* «Copia da»: stesso codice altrove, poi codice simile della stessa RFQ (stessa forma e qualità, spessore vicino) */
function copiaDaProposte(T, p, cid){
  const r = rigaB(T,p,cid), c = cicloB(T,cid), seme = SEMI[r?r.cod:""];
  const gv = grezzoVal(c) || (seme && seme.grezzo ? {q:seme.grezzo.q[0], forma:seme.grezzo.forma[0], sp:(seme.grezzo.sp||[])[0]} : null);
  return righeB(T,p).filter(x=>x.cid && x.cid!==cid && cicloB(T,x.cid) && cicloB(T,x.cid).fasi.length).map(x=>{
    const g2 = grezzoVal(cicloB(T,x.cid)); let punti = 0, perche = [];
    if(seme && seme.copia===x.cod){ punti += 10; perche.push("proposta dal modello"); }
    if(gv && g2 && gv.forma===g2.forma){ punti += 3; perche.push("stessa forma"); }
    if(gv && g2 && gv.q===g2.q){ punti += 2; perche.push("stessa qualità"); }
    if(gv && g2 && num(gv.sp) && Math.abs(num(gv.sp)-num(g2.sp))<=2){ punti += 2; perche.push("spessore vicino"); }
    return {cod:x.cod, cid:x.cid, descr:x.descr, punti, perche: perche.join(" · ")||"altro componente della RFQ"};
  }).sort((a,b)=>b.punti-a.punti).slice(0,4);
}
/* le quotazioni registrate dalle risposte (le legge Franco nei listini) */
const QUOTAZIONI_B = [];

/* ---------------- il ponte della zona B: [data-pa-gesto], esito in #pa-avviso, registro B ---------------- */
const PA = { avviso:null };
function eseguiB(url, campi){
  const T = threadAperto(); const p = typeof prodottoScelto==="function" ? prodottoScelto(T) : null;
  const pp = campiDa(campi);
  let r;
  try { r = SERVER_B.posta(T, p, String(url), pp); } catch(err){ r = {esito:"errore", testo:"Il gesto non è partito: "+(err&&err.message||err)}; }
  const obj = {}; for(const [k,v] of pp.entries()) obj[k] = v;
  registra("B","POST",String(url),obj,r);
  /* i gesti «silenziosi» (campi dei costi, offerta che ha già il suo messaggio) non aprono l'avviso galleggiante */
  PA.avviso = r.quiet ? null : {esito:r.esito, testo:r.testo, annulla:r.annulla||null};
  if(typeof dopoGestoB==="function") dopoGestoB(url, r, obj);
  if(typeof render==="function") render();
  return r;
}
document.addEventListener("click", e=>{
  const el = e.target.closest ? e.target.closest("[data-pa-gesto]") : null;
  if(!el || el.tagName==="FORM" || el.tagName==="INPUT" || el.tagName==="SELECT" || el.tagName==="TEXTAREA") return;
  e.preventDefault();
  if(el.disabled || el.getAttribute("aria-disabled")==="true") return;
  let campi = {};
  try { campi = JSON.parse(el.getAttribute("data-pa-campi")||"{}"); } catch(err){}
  if(el.getAttribute("data-pa-form-id")){ const x = document.getElementById(el.getAttribute("data-pa-form-id")); if(x) campi.lotto = x.value; }
  const form = el.getAttribute("data-pa-form") ? document.getElementById(el.getAttribute("data-pa-form")) : null;
  if(form){ for(const x of form.querySelectorAll("input,select,textarea")){ if(!x.name) continue; if(x.type==="checkbox"){ if(x.checked) campi[x.name] = x.value||"1"; } else campi[x.name] = x.value; } }
  eseguiB(el.getAttribute("data-pa-gesto"), campi);
});
document.addEventListener("submit", e=>{
  const f = e.target; if(!f || !f.hasAttribute || !f.hasAttribute("data-pa-gesto")) return;
  e.preventDefault();
  const campi = {}; for(const x of f.querySelectorAll("input,select,textarea")){ if(!x.name) continue; if(x.type==="checkbox"){ if(x.checked) campi[x.name] = x.value||"1"; } else campi[x.name] = x.value; }
  eseguiB(f.getAttribute("data-pa-gesto"), campi);
});

/* Anteprima del peso (proposta R-37, finto server B): grezzo, netto, sfrido e PES-01 li dà il server, non la vista.
   I numeri dei semi sono puri (1096 = 1.096 kg); si formattano solo quando si mostrano. */
function pesoAnteprima(T, r){
  const w = pesoGrezzo(T, r.cid), n = SEMI[r.cod] && SEMI[r.cod].netto;
  if(!w || !n) return w ? {grezzo:w.kg, formula:w.formula, netto:null, fonteNetto:null, sfrido:null, pes01:false} : null;
  const netto = typeof n[0]==="number" ? n[0] : num(n[0]);
  return {grezzo:w.kg, formula:w.formula, netto, fonteNetto:n[1], sfrido: Math.max(0, w.kg - netto), pes01: netto > w.kg + 0.01};
}

/* lotti del prodotto → pezzi del componente (q.tà per assieme × lotto) */
function pezziChiesti(q){
  const L = String(q.lotti||"").split(" / ").map(v=>parseInt(v,10)).filter(isFinite), out = new Set();
  for(const l of L){ if(!q.codici.length) out.add(l); for(const c of q.codici) out.add(l * (c.qta||1)); }
  return [...out].sort((a,b)=>a-b);
}
function testoPezzi(q){
  const L = String(q.lotti||"").split(" / ").map(v=>parseInt(v,10)).filter(isFinite);
  if(!q.codici.length) return `lotti ${L.join(" / ")}`;
  return L.map(l => `lotto prodotto ${l} → ` + q.codici.map(c=>`${String(c.cod).replace(/^F\+800\.514263/,"")} ${l*(c.qta||1)} pz`).join(", ")).join(" · ");
}

/* i 2D mancanti da chiedere al cliente (R-41): letti dalla voce 2D della zona A (sola lettura), senza richiesta «cliente» già aperta */
function mancantiAlCliente(T, p){
  if(!p) return [];
  const S2 = a2Di(T), IX = indiceDi(T);
  return righeB(T,p).filter(r=>r.cid).filter(r=>{ const v = IX.Voce(p.rif, r.cid, "disegno_2d"); return v && v.esito==="manca"; })
    .filter(r=>!S2.richieste.some(q=>q.tipo==="cliente" && q.codici.some(x=>x.cid===r.cid)));
}

/* lo stato del prodotto nella zona A, letto così com'è (ProdottoValutato.Stato, BOM.Verificata, Documenti.Stato) */
function statoZonaA(p){
  return { pronto: p.stato==="pronto_fattibilita", stato: etichetta("stato_prodotto", p.stato),
    motivi: (p.motivi||[]).map(m=>etichetta("motivo_prodotto",m)).join(" · ") || "nessun motivo",
    bom: p.bom && p.bom.verificata ? "BOM verificata" : "BOM da verificare",
    documenti: "conferma documentale: " + etichetta("stato_documenti", p.documenti ? p.documenti.stato : "") };
}
