/* ================= 21 · le fixture della zona A =================
   Un thread per richiesta (o per RFQ storica), nella forma di paginaMotoreA. Dati di prova di v13, nessun nome nuovo
   (RV-31). Gli stati di partenza:
   - VFR 482-00 · telaio skid: fonte NON confermata, una struttura candidata visibile, BOM non verificata. La demo
     percorre davvero le tre conferme: fonte → albero → documenti (SPEC §2.2);
   - ACME 990020338: due strutture candidate (STEP e IGES), un conflitto di codice che blocca «Conferma l'albero»,
     il 2D del prodotto confermato con la copia sul NAS in ERRORE («Riprova le copie»);
   - FitLab: STEP autorizzato e albero confermato su tre livelli, un pezzo sotto due padri, un 2D in coda sul NAS;
   - VignaBot, ACME 990020412, TDL, Potaflex e le RFQ storiche: casi più semplici.
   La «storia» di un thread (autorizzazioni, conferme già fatte) si applica qui, con i gesti veri del finto server:
   è il passato del thread, non qualcosa che succede all'apertura (DB7-6). */

const SCHIZZI_FILE = {};   // nome del PDF → famiglia dello schizzo di esempio (il «contenuto» finto del PDF)

function storia(T, passi){
  for(const [che, a, b] of passi){
    if(che==="autorizza"){
      const p = prodottoPerChiave(T, a), s = p.strutture_candidate[b||0];
      const IX = indiceDi(T), aut = IX.AutorizzazioneDiStruttura(p.rif, s.allegato_id, s.radice);
      const ant = anteprimaStep(T, {componente:p.componente_id, nodo:aut.nodo});
      const q = new URLSearchParams({nodo:aut.nodo, sorgente:aut.sorgente, firma:ant.firma, autorizza:"1"}); if(ant.presa_d_atto) q.set("presa_d_atto","1");
      const r = SERVER_A.posta(T, `/thread/${T.id}/fascicolo/componente/${p.componente_id}/step-strutturale`, q);
      if(r.esito!=="ok") throw new Error("storia "+T.key+": "+r.testo);
    } else if(che==="albero"){
      const r0 = riepilogoAlbero(T); const r = SERVER_A.confermaAlbero(T, {firma:r0.firma, bozza:{formato:1, base:r0.albero_firma}});
      if(r.esito!=="ok") throw new Error("storia "+T.key+": "+r.testo);
    } else if(che==="conferma"){
      const al = T.fileKey[a], pr = T.proposte[al];
      const r = SERVER_A.posta(T, `/thread/${T.id}/fascicolo/conferma`, new URLSearchParams({voce:pr.id}));
      if(r.esito!=="ok") throw new Error("storia "+T.key+": "+r.testo);
      const d = Object.values(T.documenti).find(x=>x.allegato===al); d.stato_nas = b||"scritto";
    }
  }
  ricalcola(T);
}
const faseDi = (rfqId, gg, sem) => { const q = RFQ.find(x=>x.id===rfqId); return q ? {nome:q.fase, gg, semaforo:sem} : null; };

/* ---------------- VFR · ordine 482-00 · telaio skid (il caso completo) ---------------- */
(()=>{
  const T = nuovoThread("q-sv1", { rfq:"q-sv1", cliente:"VFR", oggetto:"Ordine 482-00 · telaio skid F+800.514263.000", rif:"482-00",
    fase:{nome:"FATTIBILITA", gg:3, semaforo:"verde"},
    file:{
      step000:["F+800.514263.000.STEP","step"],
      pdf000:["F+800.514263.000.pdf","pdf","senza_testo"],
      pdf001:["F+800.514263.001 01 1-2.PDF","pdf","senza_testo"],
      pdf002:["F+800.514263.002.pdf","pdf","senza_testo"],
      pdf006:["F+800.514263.006.pdf","pdf","senza_testo"],
      pdf008:["F+800.514263.008.pdf","pdf","senza_testo"],
      pdf009:["F+800.514263.009.pdf","pdf","senza_testo"],
      dwg000:["F+800.514263.000  02.DWG","dwg","illeggibile","documento_non_leggibile"],
      dwg001:["F+800.514263.001  01  1_2.DWG","dwg","illeggibile","documento_non_leggibile"],
      ord:["ORD.482-00.pdf","pdf"],
    },
    prodotti:[{ k:"sv1", cod:"F+800.514263.000", descr:"Telaio skid KX90I", pdf:"pdf000",
      candidate:[{ step:"step000", radice:{ cod:"F+800.514263.000", descr:"Telaio skid KX90I", pdf:"pdf000", figli:[
        { cod:"F+800.514263.001", descr:"Travi UPN 160", qta:1, pdf:"pdf001" },
        { cod:"F+800.514263.002", descr:"Tubo Ø127 × 10", qta:4, pdf:"pdf002" },
        { cod:"F+800.514263.006", descr:"Lamiera sagomata", qta:2, pdf:"pdf006" },
        { cod:"F+800.514263.008", descr:"Piastra 360 × 360 sp. 15", qta:4, pdf:"pdf008" },
        { cod:"F+800.514263.009", descr:"Piastra 280 × 290", qta:8, pdf:"pdf009" },
        { cod:"POS.2", descr:"Blocco d'angolo sinistro", qta:2 },
        { cod:"POS.3", descr:"Blocco d'angolo destro", qta:2 },
        { cod:"ISO 4032 M14", descr:"Dado M14 cl. 8", qta:6, cat:"commerciale", proposta:"minuteria" },
      ]}}]}],
    smistare:[["dwg000","file_da_smistare","da_determinare",["sv1"]],["dwg001","file_da_smistare","da_determinare",["sv1"]],["ord","nessun_candidato","ordine_cliente"]],
  });
  Object.assign(SCHIZZI_FILE, {"F+800.514263.000.pdf":"telaio","F+800.514263.001 01 1-2.PDF":"profilo","F+800.514263.002.pdf":"tubo",
    "F+800.514263.006.pdf":"lamiera","F+800.514263.008.pdf":"piastra","F+800.514263.009.pdf":"piastra2"});
})();

/* ---------------- ACME Trattori · 990020338 ---------------- */
(()=>{
  const albero = { cod:"9990708A1", descr:"Supporto batteria", pdf:"pdf708", figli:[
    { cod:"9990707A1", descr:"Supporto", qta:1, pdf:"pdf707" },
    { cod:"990679X1", descr:"Dado M8 cl. 8 PL", qta:2, cat:"commerciale", proposta:"minuteria" },
    { cod:"939552X1", descr:"Dado SDPR M6", qta:3, cat:"commerciale", proposta:"minuteria" },
    { cod:"9443580X", grezzo:"9443580X", descr:"Dado SDPR M5", qta:1, cat:"commerciale", proposta:"minuteria" },
  ]};
  const T = nuovoThread("r-ar1", { req:"r-ar1", rfq:"q-ar1", cliente:"ACME Trattori", oggetto:"RICHIESTA D'OFFERTA 990020338", rif:"990020338",
    fase:{nome:"RICEVUTA", gg:9, semaforo:"giallo"},
    file:{
      stp:["9990708A_1.STP","step"], igs:["9990708A_1.IGS","igs"],
      pdf708:["9990708A_1.pdf","pdf"], pdf707:["9990707A_1.pdf","pdf"],
      msg:["RDO 990020338_100_260902.msg","msg"],
      zip:["9990707A_1.zip","zip","pendente","contenitore","contenitore_estratto"],
      img:["image001.jpg","jpg","disponibile","","inline"],
    },
    prodotti:[{ k:"at1", cod:"9990708A1", descr:"Supporto batteria", rev:"1", pdf:"pdf708",
      candidate:[ { step:"stp", radice:albero }, { step:"igs", radice:albero, compat:"equivalente", grafo:false, motivoGrafo:"completezza del grafo non dichiarata dal documento" } ] }],
    smistare:[["msg","nessun_candidato","corrispondenza"]],
  });
  // il 2D del prodotto è già confermato (DB7-2: la conferma documentale può venire prima della BOM verificata); la copia sul NAS è finita in errore
  const p = prodottoPerChiave(T,"at1"); const al = T.fileKey.pdf708, pr = T.proposte[al];
  const did = uuidDa(T.key+":doc:"+al);
  T.documenti[did] = { id:did, allegato:al, componente:p.componente_id, tipo_documento:"disegno_2d", nome:T.allegati[al].nome, stato_nas:"errore", nasDa:0, nasErrore:false, corrente:true, rev:"1" };
  delete T.proposte[al];
  // il conflitto di codice: nello STEP «9443580X», nel disegno d'assieme «9443580X1»
  const n = p.strutture_candidate[0].nodi.find(x=>x.nodo.codice.proposto==="9443580X");
  T.conflitti.push({ prodotto:p.rif, tipo:"codice", asse:"nomenclatura", rif:n.nodo.rif, decisione:"9443580X1", origine_decisione:"grezzo",
    evidenza_decisione:{il:null}, proposta:"9443580X", evidenza_proposta:{allegato_id:T.fileKey.stp, evidenza:{valore:"9443580X"}}, motivo:"codice" });
  Object.assign(SCHIZZI_FILE, {"9990708A_1.pdf":"staffa","9990707A_1.pdf":"lamiera"});
  ricalcola(T);
})();

/* ---------------- FitLab · RDO 2026/318 · tre livelli, un pezzo sotto due padri ---------------- */
(()=>{
  const T = nuovoThread("r-tg1", { req:"r-tg1", rfq:"q-tg1", cliente:"FitLab", oggetto:"Richiesta di quotazione 9N007614AE", rif:"RDO 2026/318",
    fase:{nome:"FATTIBILITA", gg:5, semaforo:"giallo"},
    file:{ stp:["9N007614AE.stp","step"], pdf:["9N007614AE.pdf","pdf"], p518:["9N008518AA.pdf","pdf"], p519:["9N008519AB.pdf","pdf"], p520:["9N008520AA.pdf","pdf"],
      scan:["9N008521AA_scan.pdf","pdf","senza_testo"], cap:["capitolato_TG_2026.pdf","pdf"], xls:["target_price_Q4.xlsx","xlsx"] },
    prodotti:[{ k:"tg1", cod:"9N007614AE", descr:"Telaio Reformer", rev:"2", pdf:"pdf",
      candidate:[{ step:"stp", radice:{ cod:"9N007614AE", descr:"Telaio Reformer", pdf:"pdf", figli:[
        { cod:"9N008518AA", descr:"Supporto saldato", qta:1, pdf:"p518", figli:[
          { cod:"9N008520AA", descr:"Piastra base", qta:1, pdf:"p520" },
          { cod:"9N008519AB", descr:"Perno guida", qta:1, pdf:"p519" } ]},
        { cod:"9N008519AB", descr:"Perno guida", qta:2, pdf:"p519" } ]}}]}],
    smistare:[["scan","nessun_candidato","disegno_2d",["tg1"]],["cap","nessun_candidato","capitolato"],["xls","nessun_candidato","commerciale"]],
  });
  storia(T, [["autorizza","tg1"],["albero"],["conferma","pdf","scritto"],["conferma","p518","scritto"],["conferma","p519","scritto"],["conferma","p520","in_coda"]]);
  const d = Object.values(T.documenti).find(x=>x.allegato===T.fileKey.p520); d.nasDa = T.lettura+1;
  Object.assign(SCHIZZI_FILE, {"9N007614AE.pdf":"telaio","9N008518AA.pdf":"staffa","9N008519AB.pdf":"perno","9N008520AA.pdf":"piastra"});
})();

/* ---------------- VignaBot · due prodotti, revisione b3 che sostituisce la b2 ---------------- */
(()=>{
  nuovoThread("r-vb1", { req:"r-vb1", cliente:"VignaBot", oggetto:"Demande de devis — 902235_B / 902256_B", rif:"",
    file:{ s35:["902235-telaio_b2.step","step"], b3:["902235-telaio_b3.pdf","pdf"], b2:["902235-telaio_b2.pdf","pdf"], s56:["902256-symtelaio_b2.step","step"] },
    prodotti:[
      { k:"vb1", cod:"902235_B", descr:"Telaio V", pdf:"b3", alt:["b2"],
        candidate:[{ step:"s35", compat:"compatibile_parziale", radice:{ cod:"902235-telaio", descr:"Telaio V", pdf:"b3", figli:[
          { cod:"902235-01", descr:"Piastra", qta:2 }, { cod:"902235-04", descr:"Rinforzo", qta:1 } ]}}]},
      { k:"vb2", cod:"902256_B", descr:"SymTelaio", candidate:[{ step:"s56", compat:"compatibile_parziale", radice:{ cod:"902256-symtelaio", descr:"SymTelaio" }}]} ],
  });
  Object.assign(SCHIZZI_FILE, {"902235-telaio_b3.pdf":"staffa","902235-telaio_b2.pdf":"staffa"});
})();

/* ---------------- ACME · 990020412 · due prodotti, lo STEP del secondo arrivato dopo ---------------- */
(()=>{
  nuovoThread("r-ar2", { req:"r-ar2", cliente:"ACME Trattori", oggetto:"RICHIESTA D'OFFERTA 990020412", rif:"990020412",
    file:{ s12:["9990712B1.stp","step"], p12:["9990712B1.pdf","pdf"], p13:["9990713C1.pdf","pdf"], s13:["9990713C1.stp","step"] },
    prodotti:[
      { k:"at2", cod:"9990712B1", descr:"Staffa parafango", pdf:"p12", candidate:[{ step:"s12", radice:{ cod:"9990712B1", descr:"Staffa parafango", pdf:"p12" }}]},
      { k:"at3", cod:"9990713C1", descr:"Supporto faro", pdf:"p13", candidate:[{ step:"s13", radice:{ cod:"9990713C1", descr:"Supporto faro", pdf:"p13" }}]} ],
  });
  Object.assign(SCHIZZI_FILE, {"9990712B1.pdf":"staffa","9990713C1.pdf":"staffa"});
})();

/* ---------------- TDL · i disegni sono sul portale: nessun file nel thread ---------------- */
(()=>{
  const T = nuovoThread("r-sd1", { req:"r-sd1", cliente:"Trattori Delta", oggetto:"Nuova RdO — S9.1248 / S9.1249", rif:"",
    file:{ p49:["S9.1249_rev0.pdf","pdf"] },
    prodotti:[ { k:"sd1", cod:"S9.1248", descr:"Staffa serbatoio", portale:true }, { k:"sd2", cod:"S9.1249", descr:"Staffa pompa", portale:true, pdf:"p49" } ],
  });
  storia(T, [["conferma","p49","scritto"]]);
  Object.assign(SCHIZZI_FILE, {"S9.1249_rev0.pdf":"staffa"});
})();

/* ---------------- Potaflex · 92176C · offerta inviata: tutto verificato ---------------- */
(()=>{
  const T = nuovoThread("r-pl1", { req:"r-pl1", rfq:"q-pl1", cliente:"Potaflex", oggetto:"Demande de prix — support batterie 92176C", rif:"",
    fase:{nome:"OFFERTA_INVIATA", gg:16, semaforo:"bianco"},
    file:{ stp:["92176C.stp","step"], pdf:["92176C.pdf","pdf"], so:["SO 5478.pdf","pdf","disponibile","","messaggio_in_uscita"] },
    prodotti:[{ k:"pl1", cod:"92176C", descr:"Supporto batteria", pdf:"pdf", candidate:[{ step:"stp", radice:{ cod:"92176C", descr:"Supporto batteria", pdf:"pdf" }}]}],
  });
  storia(T, [["autorizza","pl1"],["albero"],["conferma","pdf","scritto"]]);
  Object.assign(SCHIZZI_FILE, {"92176C.pdf":"staffa"});
})();

/* ---------------- RFQ storiche: struttura e documenti verificati ---------------- */
for(const [id, cli, cod, nome, gg] of [["q-fa1","Gru Alfa","GA-55.210","Staffa braccio gru",20],["q-sd0","Trattori Delta","S9.0987","Supporto cabina",11],
  ["q-kr1","Falci Beta","920-097","Staffa trincia",40],["q-cl1","Mieti Gamma","9911.45.22","Telaio sedile",85],["q-pl0","Potaflex","93840B","Carter forbice",120]]){
  const q = RFQ.find(x=>x.id===id);
  const T = nuovoThread(id, { rfq:id, cliente:cli, oggetto:q.titolo, rif:q.num, stato_thread: q.statoThread||"APERTA",
    fase:{nome:q.fase, gg, semaforo:"bianco"},
    file:{ stp:[cod+".stp","step"], pdf:[cod+".pdf","pdf"] },
    prodotti:[{ k:id, cod, descr:nome, pdf:"pdf", candidate:[{ step:"stp", radice:{ cod, descr:nome, pdf:"pdf" }}]}] });
  storia(T, [["autorizza",id],["albero"],["conferma","pdf","scritto"]]);
  SCHIZZI_FILE[cod+".pdf"] = "staffa";
}

/* un thread nuovo da una richiesta nata dall'Inbox: i file non sono ancora preparati (nessun prepara all'apertura,
   DB7-6). «Prepara i file» è un pulsante; dopo, l'esito mostra la fonte e i documenti. */
function threadDaRichiesta(r){
  if(THREADS[r.id]) return THREADS[r.id];
  const msg = MESSAGGI.find(m=>m.req===r.id) || {files:[]};
  const file = {}, prodotti = [];
  (msg.files||[]).forEach((f,i)=>{ file["f"+i] = [f.n, f.e==="pdf"?"pdf":f.e==="stp"||f.e==="step"?"step":f.e]; });
  for(const pid of r.prodotti){
    const P = PROD[pid];
    const pdf = Object.keys(file).find(k=>file[k][1]==="pdf" && file[k][0].startsWith(P.cod));
    const stp = Object.keys(file).find(k=>file[k][1]==="step" && file[k][0].startsWith(P.cod));
    prodotti.push({ k:pid, cod:P.cod, descr:P.nome, pdf, candidate: stp ? [{ step:stp, radice:{cod:P.cod, descr:P.nome, pdf}}] : [] });
    if(pdf) SCHIZZI_FILE[file[pdf][0]] = "staffa";
  }
  /* RL-05: ogni file che non è il disegno o lo STEP di un prodotto va in «Da smistare» (nessun_candidato), come nelle
     altre fixture: niente si perde. Un PDF è proposto come 2D, il resto come «da determinare». */
  const usati = new Set(prodotti.flatMap(x=>[x.pdf, ...(x.candidate||[]).map(c=>c.step)]).filter(Boolean));
  const smistare = Object.keys(file).filter(k=>!usati.has(k)).map(k=>[k, "nessun_candidato", file[k][1]==="pdf" ? "disegno_2d" : "da_determinare"]);
  for(const [k] of smistare) if(file[k][1]==="pdf") SCHIZZI_FILE[file[k][0]] = SCHIZZI_FILE[file[k][0]] || "perno";
  const q = r.rfq ? RFQ.find(x=>x.id===r.rfq) : null;
  return nuovoThread(r.id, { req:r.id, rfq:r.rfq||null, cliente:cl(r.cli).nome, oggetto:r.titolo, rif:r.numCli||"", nonPreparato:true,
    fase: q ? {nome:q.fase, gg:0, semaforo:"verde"} : null, file, prodotti, smistare });
}
