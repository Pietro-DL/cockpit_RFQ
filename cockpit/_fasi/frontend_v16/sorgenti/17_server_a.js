/* ================= 17 · il finto server della zona A =================
   Fa le veci del backend di Pietro per la pagina della Distinta (A1d). È l'UNICO posto del mockup dove si calcolano
   esiti, assi, BOM verificata, fabbisogno dei documenti, stato del prodotto e fascicolo: la vista (45_struttura.js)
   li legge e basta, attraverso l'Indice con le 25 ricerche vere (RV-02, RV-03, RV-04).
   Regole vere che applica, comprese quelle che dicono «no»:
   - «Accetta il nodo» crea il componente ma lascia la BOM da verificare (DB7-4); non accetta «commerciale»;
   - un nodo sotto un particolare o un commerciale si rifiuta (regola dei contenitori, nuovo.go);
   - solo «Conferma l'albero», con il riepilogo confermabile e la firma giusta, rende BOM.Verificata vero;
   - la rinomina del target è spenta (DB7-3); «Rev e descrizione» vuole la descrizione registrata;
   - la conferma documentale è per una voce; «confermato» non è «copiato»: la copia sul NAS va in coda e diventa
     «scritto» (o «errore») a una lettura successiva; «Scarta il file» è definitivo (DB7-8);
   - nessun gesto cambia la fase registrata o lo stato del thread (EB7-3, RV-05).
   La forma dei dati è quella del JSON di GET /thread/{id}/motore-a/dati (contratto A1d §1, §2.4). */

const THREADS = {};                       // chiave del mockup → thread
let threadCorrenteKey = null;
function threadAperto(){ return threadCorrenteKey ? THREADS[threadCorrenteKey] || null : null; }

/* identificativi deterministici (uuid e sha) dai nomi della fixture: stabili fra un caricamento e l'altro */
function hexDa(s, n){ let out="", i=0; while(out.length<n){ let h=0x811c9dc5^i; for(const ch of String(s)+"·"+i){ h=Math.imul(h^ch.charCodeAt(0),16777619)>>>0; } out+=h.toString(16).padStart(8,"0"); i++; } return out.slice(0,n); }
function uuidDa(s){ const h=hexDa(s,32); return `${h.slice(0,8)}-${h.slice(8,12)}-4${h.slice(13,16)}-a${h.slice(17,20)}-${h.slice(20,32)}`; }
const OGGI_ISO = "2026-10-11";
const UUID_NULLO = "00000000-0000-0000-0000-000000000000";
const copia = x => JSON.parse(JSON.stringify(x));

/* ------------------------------------------------------------------ costruzione di un thread dalla fixture */
function nuovoThread(key, def){
  const T = { key, id: uuidDa("thread:"+key), rfq: def.rfq||null, req: def.req||null,
    testa: { cliente: def.cliente||"", oggetto: def.oggetto||"", riferimento_cliente: def.rif||"", stato_thread: def.stato_thread||"APERTA",
      fase: def.fase ? { nome: def.fase.nome, etichetta: (FASE_CATALOGO[def.fase.nome]||""), inizio: def.fase.inizio||"2026-10-08T08:40:00+02:00",
        gg_in_fase: def.fase.gg||0, semaforo: def.fase.semaforo||"bianco", terminale: !!(FASE_TERMINALI.includes(def.fase.nome)) } : null },
    allegati:{}, fileKey:{}, componenti:{}, relazioni:[], proposte:{}, documenti:{}, deroghe:{}, esclusi:[],
    prodotti:[], candOrig:{}, daSmistare:[], conflitti:[], lettura:0, nonPreparato: !!def.nonPreparato, nodoSeq:0, schizzi:{},
    esito:null, calcolato:"" };
  for(const [k, f] of Object.entries(def.file||{})){
    const [nome, tipo, disp, motivo, perimetro] = f;
    const id = uuidDa(key+":file:"+k);
    T.allegati[id] = { id, nome, tipo, disponibilita: disp||"disponibile", motivo: motivo||"", perimetro: perimetro||"dentro", sha: hexDa(key+nome,64) };
    T.fileKey[k] = id;
  }
  for(const pd of (def.prodotti||[])) creaProdotto(T, pd);
  for(const [fk, motivo, tipoDoc, pks] of (def.smistare||[])){
    const a = T.fileKey[fk];
    T.proposte[a] = { id: uuidDa(key+":prop:"+fk), allegato:a, tipo_documento: tipoDoc||"da_determinare", nodo:null, componente:null, manuale:false };
    T.daSmistare.push({ allegato_id:a, nome_file:T.allegati[a].nome, motivo, prodotti:(pks||[]).map(k=>prodottoPerChiave(T,k).rif), orfano:false, prodotti_contesto:[] });
  }
  for(const s of (def.schizzi?Object.entries(def.schizzi):[])) T.schizzi[T.allegati[T.fileKey[s[0]]].nome]=s[1];
  THREADS[key] = T;
  ricalcola(T);
  return T;
}
const prodottoPerChiave = (T,k) => T.prodotti.find(p=>p._k===k);

function disegnoDa(T, fk, prov){
  const a = T.fileKey[fk]; if(!a) return null;
  const f = T.allegati[a];
  return { allegato_id:a, documento_id:null, nome_file:f.nome, formato:"PDF", provenienza:prov||"proposto", corrente:false,
    validita: f.disponibilita==="illeggibile"?"da_verificare":"valido", cartiglio: f.disponibilita==="senza_testo"?"non_leggibile":"letto",
    codice: null, revisione_proposta:{valore:null,fonte:"assente"}, registrata:null, confermata:null,
    anteprima:{ allegato_id:a, sha256:f.sha, disponibile: ["disponibile","senza_testo","parziale"].includes(f.disponibilita) } };
}
function creaProdotto(T, pd){
  const cid = uuidDa(T.key+":comp:"+pd.cod);
  T.componenti[cid] = { id:cid, codice:pd.cod, rev:pd.rev||null, descrizione: pd.descr||null, tipo:"finito", archiviato:false, step_strutturale:null,
    disegni: pd.pdf ? { primario: disegnoDa(T, pd.pdf), alternativi:(pd.alt||[]).map(fk=>disegnoDa(T,fk)), regola_primario: pd.alt?"recente":null, relazioni_da_confermare:[] } : { primario:null, alternativi:[] },
    storia:false };
  if(pd.pdf) T.proposte[T.fileKey[pd.pdf]] = { id: uuidDa(T.key+":prop:"+pd.pdf), allegato:T.fileKey[pd.pdf], tipo_documento:"disegno_2d", nodo:"target:"+cid, componente:null, manuale:false };
  for(const fk of (pd.alt||[])) T.proposte[T.fileKey[fk]] = { id: uuidDa(T.key+":prop:"+fk), allegato:T.fileKey[fk], tipo_documento:"disegno_2d", nodo:"target:"+cid, componente:null, manuale:false, alternativo:true };
  const p = { rif: "componente:"+cid, codice_richiesto: pd.cod, componente_id: cid,
    identita:"confermata", autorita:"confermata",
    fonte:{ stato:"assente", motivo:"nessuna_fonte", calcolata:true, estrazione:"", riferimento:null, candidati:[] },
    struttura:"nessuna", bom:{ nomenclatura:{stato:"non_verificabile",motivo:"fonte_non_confermata",da_decidere:0,legacy:false,conflitti:[]},
      gerarchia:{stato:"non_verificabile",motivo:"fonte_non_confermata",da_decidere:0,legacy:false,conflitti:[]}, verificata:false, senza_figli:false, rimozioni_aperte:0 },
    nodi:[], strutture_candidate:[], confermati_senza_step:[],
    smistamento:{ stato:"verificato", motivi:[], calcolata:true, file_non_terminali:[] },
    documenti:{}, prodotto_verificato:false, stato:"non_pronto", motivi:[] };
  /* chiave interna del mockup: non enumerabile, così non esce nel JSON di /motore-a/dati (RV-01) */
  Object.defineProperty(p, "_k", {value: pd.k, enumerable:false, writable:true});
  if(pd.portale) p.fonte = { stato:"assente", motivo:"indicata_sul_portale", calcolata:true, estrazione:"", riferimento:null, candidati:[] };
  T.prodotti.push(p);
  (pd.candidate||[]).forEach((c,i)=>{
    const a = T.fileKey[c.step], f = T.allegati[a];
    const nodi = []; nodiDaSpec(T, a, f.sha, c.radice, null, null, nodi, c.prefisso||"");
    p.strutture_candidate.push({ allegato_id:a, sha256:f.sha, radice:nodi[0].nodo.rif, radice_del_file:c.radiceDelFile!==false,
      compatibilita:c.compat||"uguale", grafo_completo:c.grafo!==false, motivo_grafo:c.motivoGrafo||"", nodi });
  });
  if(p.strutture_candidate.length){
    p.fonte = { stato:"in_attesa_di_conferma", motivo: p.strutture_candidate.length>1?"da_scegliere":"documento_candidato", calcolata:true, estrazione:"riuscita", riferimento:null,
      candidati: p.strutture_candidate.map(s=>({allegato_id:s.allegato_id, origine:"documento_del_prodotto"})) };
    p.struttura = "struttura_candidata";
  }
  T.candOrig[p.rif] = copia(p.strutture_candidate);
}
/* i nodi di uno STEP, dalla specifica annidata della fixture: rif nodo:<sha>:#n, riga aperta, parentela con la qtà */
function nodiDaSpec(T, allegato, sha, s, padreRif, qta, out, pref){
  const n = ++T.nodoSeq, rif = `nodo:${sha}:#${n*10}`;
  const pid = uuidDa(T.key+":riga:"+rif);
  const esistente = out.find(x=>x.nodo.codice.proposto && x.nodo.codice.proposto===s.cod && s.cod);
  if(esistente && padreRif){            // lo stesso pezzo sotto un secondo padre: un nodo solo, due archi
    esistente.nodo.padri.push(padreRif); esistente.parentela.push({padre:padreRif, quantita:qta, decisa:false}); return esistente;
  }
  const node = {
    nodo:{ rif, allegato_id:allegato, entita_id:"#"+(n*10), senza_file:false,
      codice:{ grezzo:{testo: s.grezzo||s.cod||"", campo:"PRODUCT"}, proposto: s.cod||null, manuale:null, confermato:null, documentale:[], identita:null, motivo_lettura:null, motivo_proposto: s.cod?null:"nessuna_lettura" },
      padri: padreRif?[padreRif]:[], decisione:null, riga_legacy:{ id:pid, tipo_proposto: s.tipo||null, codice_manuale:null, rev_manuale:null }, riga_decisa:null, deciso_da_persona:false },
    descrizione: s.descr||null,
    parentela: padreRif?[{padre:padreRif, quantita:qta, decisa:false}]:[],
    disegni: s.pdf ? { primario: disegnoDa(T, s.pdf), alternativi:[], regola_primario:null, relazioni_da_confermare:[] } : { primario:null, alternativi:[] },
    associazione: s.pdf ? "proposto" : null,
    motivi: [],
    classificazione:{ ruolo: padreRif?"componente":"prodotto", categoria: s.cat||"non_determinata", origine:"proposto", confermata:false, motivo:null, proposta: s.proposta||null },
    anche_in: [],
    tipo: s.tipo || ((s.figli&&s.figli.length)?"sottoassieme":"sciolto"),
    origine_tipo: s.tipo ? "proposto_dalla_riga" : "proposto_dalla_regola",
  };
  out.push(node);
  if(s.pdf && !T.proposte[T.fileKey[s.pdf]]) T.proposte[T.fileKey[s.pdf]] = { id: uuidDa(T.key+":prop:"+s.pdf), allegato:T.fileKey[s.pdf], tipo_documento:"disegno_2d", nodo:"cod:"+(s.cod||rif), componente:null, manuale:false };
  for(const f of (s.figli||[])) nodiDaSpec(T, allegato, sha, f, rif, f.qta||1, out, pref);
  return node;
}

/* ------------------------------------------------------------------ il ricalcolo (come una GET: si rifà a ogni lettura) */
const tuttiINodi = p => [...p.nodi, ...p.strutture_candidate.flatMap(s=>s.nodi), ...p.confermati_senza_step];
const nodoPerRiga = (T, pid) => { for(const p of T.prodotti) for(const n of tuttiINodi(p)) if((n.nodo.riga_legacy&&n.nodo.riga_legacy.id===pid)||(n.nodo.riga_decisa&&n.nodo.riga_decisa.id===pid)) return {p,n}; return null; };
const cidDelNodo = (p, n) => n.nodo.decisione ? n.nodo.decisione.componente_id : (p.nodi.length && n===p.nodi[0] ? p.componente_id : null);
function nodoDelComponente(T, cid){ for(const p of T.prodotti){ for(const n of [...p.nodi,...p.confermati_senza_step]) if(cidDelNodo(p,n)===cid) return {p,n}; } return null; }
const figliDelComp = (T,cid) => T.relazioni.filter(r=>r.padre===cid);
function codiceDelNodo(n){ const c=n.nodo.codice; if(c.confermato) return c.confermato.codice; if(n.nodo.decisione) return n.nodo.decisione.codice; if(c.manuale) return c.manuale.codice; if(c.proposto) return c.proposto; return c.grezzo&&c.grezzo.testo?"«"+c.grezzo.testo+"»":"senza codice"; }

function propostaPerComponente(T, cid, p){
  // la proposta aperta del 2D di un componente: associata a mano a lui, oppure proposta dal nome per il suo nodo
  const target = p && cid===p.componente_id;
  const nc = nodoDelComponente(T, cid);
  const cod = nc ? (nc.n.nodo.codice.proposto||nc.n.nodo.rif) : null;
  return Object.values(T.proposte).find(x=>x.tipo_documento==="disegno_2d" && !x.alternativo && (
    x.componente===cid || (!x.componente && ((target && x.nodo==="target:"+cid) || (cod && x.nodo==="cod:"+cod)))));
}
function ricalcola(T){
  const np = T.nonPreparato;
  /* il disegno del nodo segue lo stato del suo file (proposto, a mano, confermato) */
  for(const p of T.prodotti) for(const n of tuttiINodi(p)) if(n.disegni&&n.disegni.primario) aggiornaDisegno(T, n.disegni.primario);
  for(const c of Object.values(T.componenti)) if(c.disegni){ if(c.disegni.primario) aggiornaDisegno(T,c.disegni.primario); (c.disegni.alternativi||[]).forEach(d=>aggiornaDisegno(T,d)); }
  for(const p of T.prodotti){
    if(np){ continue; }
    /* RL-03 (contratto §1.4, E4): i componenti del perimetro confermato (il target e i raggiungibili per relazioni
       confermate) che non sono un nodo di Nodi. Senza BOM di lavoro ci sono tutti, radice compresa. Solo una lettura. */
    p.confermati_senza_step = confermatiSenzaStep(T, p);
    const fc = p.fonte.stato==="confermata";
    const aperti = p.nodi.filter(n=>n.nodo.riga_legacy).length;
    const archiAperti = p.nodi.reduce((s,n)=>s+n.parentela.filter(x=>!x.decisa && !scartato(p.nodi,x.padre) && !(n.nodo.riga_decisa&&n.nodo.riga_decisa.stato==="scartata")).length,0);
    const confl = T.conflitti.filter(c=>c.prodotto===p.rif);
    const ver = p.bom.verificata;
    if(!p.nodi.length){
      /* senza BOM di lavoro: «Conferma l'albero» copre la nomenclatura dei confermati senza STEP; la gerarchia resta
         «da verificare» senza la fonte STEP confermata (R68 A, K-02 A, bom.go:72). Quindi verificata = false. */
      const m = fc?"bom_di_lavoro_assente":"fonte_non_confermata", g = !!p._albero;
      p.bom.nomenclatura = {stato: g?"verificata":"non_verificabile", motivo: g?null:m, da_decidere:0, legacy:false, conflitti:[]};
      p.bom.gerarchia = {stato:"da_verificare", motivo: m, da_decidere:0, legacy:false, conflitti:[]};
      p.bom.verificata = false;
    } else {
      const cn = confl.filter(c=>c.asse==="nomenclatura").map(c=>c.rif), cg = confl.filter(c=>c.asse==="gerarchia").map(c=>c.rif);
      p.bom.nomenclatura = {stato: cn.length?"conflitto":ver?"verificata":"da_verificare", motivo: cn.length?"conflitto":aperti?"da_decidere":null, da_decidere:aperti, legacy:false, conflitti:cn};
      p.bom.gerarchia = {stato: cg.length?"conflitto":ver?"verificata":"da_verificare", motivo: cg.length?"conflitto":archiAperti?"da_decidere":null, da_decidere:archiAperti, legacy:false, conflitti:cg};
    }
    p.bom.rimozioni_aperte = confl.filter(c=>c.tipo==="arco"&&c.motivo==="rimozione").length;
    p.struttura = p.nodi.length ? "bom_di_lavoro_proposta" : p.strutture_candidate.length ? "struttura_candidata" : "nessuna";
    p.documenti = completezza(T, p);
    const ds = T.daSmistare.filter(f=>f.prodotti.includes(p.rif));
    p.smistamento = { stato: ds.length?"da_verificare":"verificato", motivi: ds.length?["file_da_smistare"]:[], calcolata:true, file_non_terminali:[] };
    const mot = [];
    if(!fc) mot.push("fonte_strutturale_step_mancante_o_non_confermata");
    if(p.bom.nomenclatura.stato!=="verificata") mot.push("nomenclatura_non_verificata");
    if(p.bom.gerarchia.stato!=="verificata") mot.push("gerarchia_non_verificata");
    if(p.smistamento.stato!=="verificato") mot.push("smistamento_non_verificato");
    if(p.documenti.stato!=="completa") mot.push(p.documenti.stato==="non_calcolabile"?"documentazione_non_calcolabile":"documentazione_incompleta");
    if(confl.length) mot.push("conflitto");
    p.motivi = mot; p.prodotto_verificato = !mot.length; p.stato = mot.length ? "non_pronto" : "pronto_fattibilita";
  }
  if(np) for(const p of T.prodotti){
    T.fontiVere = T.fontiVere || {}; if(!T.fontiVere[p.rif]) T.fontiVere[p.rif] = p.fonte;
    p.fonte = { stato:"", motivo:"", calcolata:false, estrazione:"non_analizzata", riferimento:null, candidati:[] };
    p.documenti = { stato:"non_calcolabile", voci:[], previsti:[], non_bloccanti:[], motivo:"non_determinabile", perimetro_chiuso:false, motivo_perimetro:"non_determinabile" };
    p.smistamento = { stato:"da_verificare", motivi:["file_da_smistare"], calcolata:false, file_non_terminali:[] };
    p.motivi = ["fonte_strutturale_step_mancante_o_non_confermata","documentazione_non_calcolabile"]; p.stato="non_pronto"; p.prodotto_verificato=false;
  }
  const pv = T.prodotti;
  const conflittiThread = T.conflitti.filter(c=>!c.prodotto);
  T.esito = {
    thread_id: T.id, valutato: true, motivo: "",
    file: Object.values(T.allegati).map(a=>({ allegato_id:a.id, disponibilita: np?"mancante":a.disponibilita, motivo: np?"":a.motivo })),
    ancoraggi: { strutture: [] },
    prodotti_valutati: pv,
    associazioni: Object.values(T.allegati).map(a=>associazioneDi(T,a)),
    da_smistare: np ? [] : T.daSmistare,
    conflitti: [...T.conflitti.map(c=>{const x={...c}; delete x.prodotto; return x;}), ...[]],
    fascicolo: {
      calcolato: !np, numero_target: pv.length, numero_verificati: pv.filter(p=>p.prodotto_verificato).length,
      pronti: pv.filter(p=>p.stato==="pronto_fattibilita").map(p=>p.rif),
      bloccati: pv.filter(p=>p.stato!=="pronto_fattibilita").map(p=>({rif:p.rif, motivi:p.motivi})),
      congelabile: pv.length>0 && pv.every(p=>p.stato==="pronto_fattibilita"),
      motivo_non_congelabile: pv.length===0?"nessun_target":pv.every(p=>p.stato==="pronto_fattibilita")?"":"prodotti_non_pronti",
      congelato:false, motivo_non_congelato:"gesto_non_registrato",
      orfani: T.daSmistare.filter(f=>f.orfano).length, avvisi:[], fase_thread: T.testa.stato_thread },
  };
  T.conflittiThread = conflittiThread;
  T.calcolato = `${OGGI_ISO}T${String(9+Math.floor(T.lettura/60)%10).padStart(2,"0")}:${String(12+T.lettura%48).padStart(2,"0")}:00+02:00`;
}
/* RL-03 · i confermati senza STEP nella forma di E4: Rif componente:<uuid>, nessun file, il codice nella Decisione,
   le parentele dalle relazioni confermate (decise, con la quantità), prima il target e poi per ComponenteID */
function confermatiSenzaStep(T, p){
  if(!p.componente_id || !T.componenti[p.componente_id]) return [];
  const inNodi = new Set(p.nodi.map(n=>cidDelNodo(p,n)).filter(Boolean));
  const per = [p.componente_id], visti = new Set(per);
  for(let i=0;i<per.length;i++) for(const r of T.relazioni){ const c = T.componenti[r.figlio];
    if(r.padre===per[i] && !visti.has(r.figlio) && c && !c.archiviato){ visti.add(r.figlio); per.push(r.figlio); } }
  const fuori = per.filter(c=>!inNodi.has(c));
  const ord = [...fuori.filter(c=>c===p.componente_id), ...fuori.filter(c=>c!==p.componente_id).sort()];
  const rifDi = cid => { const n = p.nodi.find(x=>cidDelNodo(p,x)===cid); return n ? n.nodo.rif : "componente:"+cid; };
  return ord.map(cid=>{ const c = T.componenti[cid], target = cid===p.componente_id;
    const par = T.relazioni.filter(r=>r.figlio===cid && visti.has(r.padre)).map(r=>({padre:rifDi(r.padre), quantita:r.qta, decisa:true}));
    return { nodo:{ rif:"componente:"+cid, allegato_id:UUID_NULLO, entita_id:"", senza_file:true,
        codice:{ grezzo:null, proposto:null, manuale:null, confermato:null, documentale:[], identita:null, motivo_lettura:null, motivo_proposto:null },
        padri: par.map(x=>x.padre), decisione:{ componente_id:cid, codice:c.codice, rev:c.rev||null, autorita:"confermata", origine:"confermato" },
        riga_legacy:null, riga_decisa:null, deciso_da_persona:true },
      descrizione: c.descrizione||null, parentela: par, disegni: disegniDelComponente(T, cid), associazione:null, motivi:[],
      classificazione:{ ruolo: target?"prodotto":"componente", categoria: c.tipo==="commerciale"?"commerciale":"fabbricato", origine:"confermato", confermata:true, motivo:null, proposta:null },
      anche_in:[], tipo:c.tipo, origine_tipo:"confermato" }; });
}
/* il 2D di un componente senza nodo: quello del componente, altrimenti il documento confermato, altrimenti la proposta aperta */
function disegniDelComponente(T, cid){
  const c = T.componenti[cid];
  if(c && c.disegni && c.disegni.primario) return c.disegni;
  const doc = Object.values(T.documenti).find(d=>d.componente===cid && d.tipo_documento==="disegno_2d" && d.corrente);
  const pr = doc ? null : Object.values(T.proposte).find(x=>x.componente===cid && x.tipo_documento==="disegno_2d" && !x.alternativo);
  const a = doc ? doc.allegato : pr ? pr.allegato : null, f = a ? T.allegati[a] : null;
  if(!f) return { primario:null, alternativi:[] };
  return { primario:{ allegato_id:a, documento_id: doc?doc.id:null, nome_file:f.nome, formato:"PDF", provenienza: doc?"confermato":pr.manuale?"manuale":"proposto", corrente:!!doc,
      validita: f.disponibilita==="illeggibile"?"da_verificare":"valido", cartiglio: f.disponibilita==="senza_testo"?"non_leggibile":"letto", codice:null,
      revisione_proposta:{valore:null,fonte:"assente"}, registrata:null, confermata:null,
      anteprima:{ allegato_id:a, sha256:f.sha, disponibile: ["disponibile","senza_testo","parziale"].includes(f.disponibilita) } }, alternativi:[], regola_primario:null, relazioni_da_confermare:[] };
}
const scartato = (nodi, rif) => { const n = nodi.find(x=>x.nodo.rif===rif); return !!(n && n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata"); };
function aggiornaDisegno(T, d){
  const a = d.allegato_id; const doc = Object.values(T.documenti).find(x=>x.allegato===a && x.corrente);
  const pr = T.proposte[a];
  d.documento_id = doc ? doc.id : null; d.corrente = !!doc;
  d.provenienza = doc ? "confermato" : (pr && pr.manuale) ? "manuale" : "proposto";
  if(T.esclusi.includes(a)) d.provenienza = "proposto";
}
function associazioneDi(T, a){
  const id = a.id;
  const prp = T.proposte[id], dc = Object.values(T.documenti).find(x=>x.allegato===id);
  const esclusa = T.esclusi.includes(id);
  const pert = T.prodotti.filter(p=>{
    const cids = [p.componente_id, ...p.nodi.map(n=>cidDelNodo(p,n)).filter(Boolean)];
    return (prp && (prp.nodo==="target:"+p.componente_id || cids.includes(prp.componente) || tuttiINodi(p).some(n=>prp.nodo==="cod:"+(n.nodo.codice.proposto||n.nodo.rif))))
      || (dc && cids.includes(dc.componente)) || p.strutture_candidate.some(s=>s.allegato_id===id) || (p.fonte.riferimento&&p.fonte.riferimento.allegato_id===id);
  }).map(p=>p.rif);
  /* una proposta senza destinazione (né nodo né componente) non è un candidato: è «nessun candidato» (RL-05) */
  const conDest = dc || (prp && (prp.nodo || prp.componente));
  return { allegato_id:id, associazione: conDest ? (pert.length>1?"ambiguo":"candidato_unico") : (pert.length?"candidato_unico":"nessun_candidato"),
    collocazione: pert.length ? "figlio" : "non_determinabile", proposta: [],
    manuale: prp&&prp.manuale ? prp.componente : null, confermata: dc ? dc.componente : null,
    origine: dc ? "confermato" : prp&&prp.manuale ? "manuale" : prp ? "proposto" : null,
    perimetro: a.perimetro||"dentro", esclusa, pertinente: pert, pertinenza_contesto: [], contesto:{ prodotti: [] } };
}
function completezza(T, p){
  const fc = p.fonte.stato==="confermata";
  const comp = [p.componente_id];
  for(const n of [...p.nodi, ...p.confermati_senza_step]){ const c = cidDelNodo(p,n); if(c && !comp.includes(c) && !(T.componenti[c]||{}).archiviato) comp.push(c); }
  const voci = comp.map(cid=>voce2D(T, p, cid));
  for(const d of Object.values(T.documenti)) if(d.tipo_documento!=="disegno_2d" && comp.includes(d.componente) && d.corrente)
    voci.push({ componente_id:d.componente, codice:T.componenti[d.componente].codice, tipo_componente:T.componenti[d.componente].tipo, del_prodotto:d.componente===p.componente_id,
      tipo_documento:d.tipo_documento, bloccante:false, invariante:false, regola_cliente:false, esito:"presente", motivo:null, disegni:null,
      documento_id:d.id, stato_nas:d.stato_nas, proposta_aperta:null, file_candidato:null, deroga_id:null, nota_regola:null, categoria:null });
  const previsti = p.nodi.filter(n=>!cidDelNodo(p,n) && !(n.nodo.riga_decisa&&n.nodo.riga_decisa.stato==="scartata"))
    .map(n=>({ nodo:n.nodo.rif, codice_proposto: codiceDelNodo(n), tipo_documento:"disegno_2d", motivo:"nodo_proposto", disegni:n.disegni }));
  const tutte = voci.filter(v=>v.bloccante).every(v=>v.esito==="presente");
  const aperti = p.nodi.some(n=>n.nodo.riga_legacy);
  return { stato: tutte?"completa":"incompleta", voci, previsti, non_bloccanti:[],
    motivo: tutte?null:voci.some(v=>v.esito==="manca")?"voce_mancante":"voce_non_presente",
    perimetro_chiuso: fc && p.bom.verificata && !aperti,
    motivo_perimetro: !fc ? "fonte_non_confermata" : !p.nodi.length ? "bom_di_lavoro_assente" : aperti ? "nodi_da_decidere" : !p.bom.verificata ? "archi_da_decidere" : null };
}
function voce2D(T, p, cid){
  const c = T.componenti[cid];
  const nc = nodoDelComponente(T, cid);
  const disegni = (nc && nc.n.disegni && nc.n.disegni.primario) ? nc.n.disegni : c.disegni;
  const doc = Object.values(T.documenti).find(d=>d.componente===cid && d.tipo_documento==="disegno_2d" && d.corrente);
  const der = Object.values(T.deroghe).find(d=>d.componente===cid && d.tipo==="disegno_2d");
  const pr = doc ? null : propostaPerComponente(T, cid, p);
  const v = { componente_id:cid, codice:c.codice, tipo_componente:c.tipo, del_prodotto: cid===p.componente_id, tipo_documento:"disegno_2d",
    bloccante:true, invariante: cid===p.componente_id, regola_cliente:false, esito:"manca", motivo:"nessun_documento", disegni,
    documento_id:null, stato_nas:null, proposta_aperta:null, file_candidato:null, deroga_id: der?der.id:null, nota_regola:null,
    categoria: nc && nc.n.classificazione && nc.n.classificazione.categoria!=="non_determinata" ? nc.n.classificazione.categoria : null };
  if(doc){ v.esito="presente"; v.motivo=null; v.documento_id=doc.id; v.stato_nas=doc.stato_nas; }
  else if(pr){ v.esito="da_verificare"; v.motivo="associazione_non_confermata"; v.proposta_aperta=pr.id; v.file_candidato=pr.allegato; }
  else if(der){ v.motivo="derogato_non_sostituisce_2d"; }
  return v;
}

/* ------------------------------------------------------------------ l'Indice: le 25 ricerche, nessuna regola (contratto §2.3; motore_a_indice.go) */
function indiceDi(T){
  const e = T.esito;
  const pr = rif => e.prodotti_valutati.find(p=>p.rif===rif) || null;
  const gruppiDi = rif => {
    const p = pr(rif); if(!p) return [];
    const g = [];
    if(p.nodi.length){ const a = p.fonte.riferimento ? p.fonte.riferimento.allegato_id : null;
      g.push({ chiave:"bom", tipo:"bom", allegato_id:a, radice:p.nodi[0].nodo.rif, nome_file: a?T.allegati[a].nome:"", struttura:null, nodi:p.nodi }); }
    for(const s of p.strutture_candidate) g.push({ chiave:`cand:${s.allegato_id}:${s.radice}`, tipo:"cand", allegato_id:s.allegato_id, radice:s.radice, nome_file:T.allegati[s.allegato_id].nome, struttura:s, nodi:s.nodi });
    if(p.confermati_senza_step.length) g.push({ chiave:"senza", tipo:"senza", allegato_id:null, radice:"", nome_file:"", struttura:null, nodi:p.confermati_senza_step });
    return g;
  };
  const gruppo = (rif, chiave) => gruppiDi(rif).find(g=>g.chiave===chiave) || null;
  const relConf = (padreCid, figlioCid) => T.relazioni.find(r=>r.padre===padreCid && r.figlio===figlioCid);
  const IX = {
    File: a => e.file.find(f=>f.allegato_id===a) || null,
    NomeFile: a => (T.allegati[a]||{}).nome || "",
    Associazione: a => e.associazioni.find(x=>x.allegato_id===a) || null,
    Badge: a => null,
    Prodotto: pr,
    Nodo: (rifP, rifN) => { const p=pr(rifP); return p ? tuttiINodi(p).find(n=>n.nodo.rif===rifN)||null : null; },
    QuantitaConfermata: (rifP, padre, figlio) => { const p=pr(rifP); if(!p) return null; const np=p.nodi.find(n=>n.nodo.rif===padre), nf=p.nodi.find(n=>n.nodo.rif===figlio);
      const cp = np?cidDelNodo(p,np):null, cf = nf?cidDelNodo(p,nf):null; const r = cp&&cf ? relConf(cp,cf) : null; return r ? r.qta : null; },
    PropostaAperta: a => (T.proposte[a]||{}).id || null,
    AutorizzazioneDi: (rifP, a) => { const p=pr(rifP); const s=p&&p.strutture_candidate.find(x=>x.allegato_id===a); return s ? {documento:null, nodo:`${a}:${s.nodi[0].nodo.entita_id}`, sorgente:s.nodi[0].nodo.entita_id} : null; },
    RelazioneProposta: (rifP, padreRif, figlioRif) => { const p=pr(rifP); if(!p || !p.nodi.length) return null; const n=p.nodi.find(x=>x.nodo.rif===figlioRif); const pa=n&&n.parentela.find(x=>x.padre===padreRif);
      return pa && !pa.decisa ? { allegato:n.nodo.allegato_id, padre:padreRif, figlio:figlioRif } : null; },
    Rimozione: c => c && c.tipo==="arco" && c.motivo==="rimozione" && c.chiave ? c.chiave : null,
    Gruppi: gruppiDi,
    NodoIn: (rifP, chiave, rifN) => { const g=gruppo(rifP,chiave); return g ? g.nodi.find(n=>n.nodo.rif===rifN)||null : null; },
    Radici: (rifP, chiave) => { const g=gruppo(rifP,chiave); if(!g) return []; const dentro = new Set(g.nodi.map(n=>n.nodo.rif)); return g.nodi.filter(n=>!n.parentela.some(x=>dentro.has(x.padre))); },
    Figli: (rifP, chiave, rifN) => { const g=gruppo(rifP,chiave); const p=pr(rifP); if(!g) return [];
      return g.nodi.filter(n=>n.parentela.some(x=>x.padre===rifN)).map(n=>{ const x=n.parentela.find(y=>y.padre===rifN);
        return { nodo:n, quantita: n.nodo.senza_file ? null : x.quantita, quantita_confermata: IX.QuantitaConfermata(rifP, rifN, n.nodo.rif) ?? (n.nodo.senza_file?x.quantita:null), decisa:x.decisa }; }); },
    Legami: (rifP, chiave, rifN) => { const p=pr(rifP); const n=p&&tuttiINodi(p).find(x=>x.nodo.rif===rifN); if(!n) return {del_file:[],confermati:[]};
      const cid = cidDelNodo(p,n);
      const del_file = n.nodo.senza_file ? [] : n.parentela.map(x=>({ padre:x.padre, quantita:x.quantita, quantita_confermata: IX.QuantitaConfermata(rifP,x.padre,rifN), decisa:x.decisa }));
      const confermati = cid ? T.relazioni.filter(r=>r.figlio===cid).map(r=>{ const pc=T.componenti[r.padre]; const npad = nodoDelComponente(T, r.padre);
        return { padre_id:r.padre, padre_rif: npad?npad.n.nodo.rif:"componente:"+r.padre, codice: pc?pc.codice:"", qta:r.qta, nel_file: !n.nodo.senza_file && !!(npad && n.parentela.some(x=>x.padre===npad.n.nodo.rif)) }; }) : []; /* RIPROVA: un confermato senza STEP non ha file: la sua parentela è la relazione stessa */
      return { del_file, confermati }; },
    Componente: cid => { const c=T.componenti[cid]; return c ? { id:c.id, codice:c.codice, rev:c.rev, descrizione:c.descrizione, tipo:c.tipo, archiviato:c.archiviato, step_strutturale:c.step_strutturale } : null; },
    Correnti: (cid, tipo) => Object.values(T.documenti).filter(d=>d.componente===cid && d.tipo_documento===tipo && d.corrente).map(d=>({documento_id:d.id, nome_file:d.nome, rev:d.rev||null})),
    Anteprima: a => { const f=T.allegati[a]; return f ? { allegato_id:a, sha256:f.sha, disponibile:["disponibile","senza_testo","parziale"].includes(f.disponibilita) && !T.nonPreparato } : null; },
    Conflitti: (rifP, quali) => T.conflitti.filter(c=>c.prodotto===rifP && (quali==="smistamento" ? c.asse==="smistamento" : c.asse!=="smistamento")),
    ConflittiDelThread: () => T.conflittiThread || [],
    Voce: (rifP, cid, tipo) => { const p=pr(rifP); return p&&p.documenti.voci ? p.documenti.voci.find(v=>v.componente_id===cid && v.tipo_documento===tipo)||null : null; },
    CopieAperte: rifP => { const p=pr(rifP); return p&&p.documenti.voci ? p.documenti.voci.filter(v=>v.stato_nas==="in_coda"||v.stato_nas==="errore") : []; },
    CandidatiFuori: rifP => [],
    AutorizzazioneDiStruttura: (rifP, a, radice) => { const p=pr(rifP); const s=p&&p.strutture_candidate.find(x=>x.allegato_id===a && x.radice===radice); return s ? {documento:null, nodo:`${a}:${s.nodi[0].nodo.entita_id}`, sorgente:s.nodi[0].nodo.entita_id} : null; },
  };
  return IX;
}

/* la pagina, nella forma vera di GET /thread/{id}/motore-a/dati */
function paginaJSON(T, vista, prodotto){
  return { thread_id:T.id, vista: vista||"distinta", prodotto: prodotto || (T.prodotti[0]||{}).rif || "", testa: T.testa, esito: T.esito, confronto: null, errore: "", calcolato: T.calcolato };
}

/* ------------------------------------------------------------------ le firme e le anteprime (GET di sola lettura) */
function statoFirma(T){ return hexDa(JSON.stringify([T.prodotti.map(p=>[p.fonte.stato, p.nodi.map(n=>[n.nodo.rif,n.nodo.riga_legacy&&n.nodo.riga_legacy.codice_manuale,!!n.nodo.decisione,n.nodo.riga_decisa&&n.nodo.riga_decisa.stato,n.parentela,n.tipo]), p.bom.verificata]), T.relazioni, T.conflitti.length]), 16); }
function anteprimaStep(T, q){
  const p = T.prodotti.find(x=>x.componente_id===q.componente);
  if(!p) return { spento:"Il componente non è un prodotto di questa RFQ.", autorizzabile:false, inviabile:false, frasi:[], figli:[], sorgenti:[], raggruppamenti:[], deleghe:[] };
  const a = q.documento || (q.nodo||"").split(":")[0];
  const s = p.strutture_candidate.find(x=>x.allegato_id===a);
  if(p.fonte.stato==="confermata" && p.fonte.riferimento && p.fonte.riferimento.allegato_id===a)
    return { gia: `${T.allegati[a].nome} è già lo STEP autorizzato di ${p.codice_richiesto}.`, autorizzabile:false, inviabile:false, frasi:[], figli:[], sorgenti:[], raggruppamenti:[], deleghe:[] };
  if(!s) return { spento:"Questo file non è una struttura candidata del prodotto.", autorizzabile:false, inviabile:false, frasi:[], figli:[], sorgenti:[], raggruppamenti:[], deleghe:[] };
  const rad = s.nodi[0];
  const figli = s.nodi.filter(n=>n.parentela.some(x=>x.padre===rad.nodo.rif)).map(n=>({ chiave:n.nodo.entita_id, codice:codiceDelNodo(n), nome:n.descrizione||"", qta:(n.parentela.find(x=>x.padre===rad.nodo.rif)||{}).quantita, stato:"proposto" }));
  const altri = p.strutture_candidate.filter(x=>x!==s).map(x=>T.allegati[x.allegato_id].nome);
  const frasi = [
    `${T.allegati[a].nome} diventa lo STEP strutturale di ${p.codice_richiesto}: è la conferma della fonte.`,
    `La BOM di lavoro si forma dai ${figli.length} figli diretti della radice. Restano proposte: si decidono nodo per nodo o tutte insieme con «Conferma l'albero».`,
    "Le decisioni già prese su questo prodotto restano.",
  ];
  if(altri.length) frasi.push(`L'altra struttura candidata (${altri.join(", ")}) non diventa la fonte: resta da guardare e si può scegliere più tardi revocando questa.`);
  if(!s.grafo_completo) frasi.push("Il grafo del file non è completo: dopo l'autorizzazione controlla i pezzi sotto la radice.");
  return { autorizzabile:true, inviabile:true, frasi, figli, bottone:`Autorizzo ${T.allegati[a].nome} come STEP strutturale di ${p.codice_richiesto}`,
    firma: "s-"+hexDa(statoFirma(T)+a, 16), presa_d_atto: altri.length ? "un altro STEP candidato resta fuori dalla fonte" : "",
    sorgenti:[], raggruppamenti:[], deleghe:[] };
}
function anteprimaTipo(T, q){
  const c = T.componenti[q.componente];
  if(!c) return { spento:"Componente non trovato in questa RFQ.", frasi:[] };
  const tipo = q.tipo;
  if(!["finito","sottoassieme","sciolto","commerciale"].includes(tipo)) return { errore:"Tipo non valido.", frasi:[] };
  const figli = figliDelComp(T, c.id).length + (nodoDelComponente(T,c.id) ? T.prodotti.reduce((s,p)=>s+p.nodi.filter(n=>n.parentela.some(x=>x.padre===nodoDelComponente(T,c.id).n.nodo.rif) && !(n.nodo.riga_decisa&&n.nodo.riga_decisa.stato==="scartata")).length,0) : 0);
  if(figli && (tipo==="sciolto"||tipo==="commerciale")) return { spento:`${c.codice} ha ${figli} pezzi sotto: un ${etichetta("tipo_componente",tipo)} non può contenere altri pezzi.`, frasi:[] };
  const frasi = [`${c.codice} passa da ${etichetta("tipo_componente",c.tipo)} a ${etichetta("tipo_componente",tipo)}.`];
  if(tipo==="commerciale") frasi.push("Un commerciale chiede comunque il suo 2D (R103 C): il tipo non è un'esenzione.");
  frasi.push("Se la BOM era verificata, torna da verificare: serve un nuovo «Conferma l'albero».");
  return { frasi, firma:"t-"+hexDa(statoFirma(T)+c.id+tipo,16), bottone:`Cambia il tipo in ${etichetta("tipo_componente",tipo)}` };
}
function riepilogoAlbero(T){
  const r = { formato:1, nuovi:[], ritrovati:[], tipi:[], revisioni:[], legami_nuovi:[], legami_tolti:[], quantita:[], cascata:[], fuori:[], scartate:[], rimozioni:[], blocchi:[], commerciali:[], vicini:[], quantita_discordi:[], senza_codice:[] };
  for(const p of T.prodotti){
    /* RL-03: il riepilogo vero non ha un blocco «prima autorizza lo STEP»: senza BOM di lavoro l'albero è il prodotto con i
       suoi confermati senza STEP (solo_prodotto). Che cosa la conferma copre senza STEP è la domanda 1 di R-38. */
    if(!p.nodi.length) continue;
    for(const n of p.nodi){
      const sc = n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata";
      if(sc){ r.scartate.push({ codice: codiceDelNodo(n) }); continue; }
      if(n.nodo.riga_legacy){ r.nuovi.push({ codice: codiceDelNodo(n), tipo: n.tipo }); r.tipi.push({ codice: codiceDelNodo(n), tipo: n.tipo, origine: n.origine_tipo }); }
      else if(n.nodo.decisione) r.ritrovati.push({ codice: n.nodo.decisione.codice });
      if(n.classificazione && n.classificazione.categoria==="commerciale") r.commerciali.push({ codice: codiceDelNodo(n), categoria: "commerciale", nota:"chiede il 2D" });
      if(!n.nodo.codice.proposto && !n.nodo.codice.manuale && !n.nodo.decisione && n.parentela.length) r.senza_codice.push(codiceDelNodo(n));
      for(const x of n.parentela) if(!x.decisa && !scartato(p.nodi,x.padre)){ const pd=p.nodi.find(y=>y.nodo.rif===x.padre); r.legami_nuovi.push({ padre: pd?codiceDelNodo(pd):"?", figlio: codiceDelNodo(n), qta: x.quantita }); }
    }
    for(const c of T.conflitti.filter(c=>c.prodotto===p.rif && c.asse!=="smistamento")){
      const n = p.nodi.find(x=>x.nodo.rif===c.rif);
      r.blocchi.push(`${n?codiceDelNodo(n):c.proposta} · conflitto aperto: ${etichetta("motivo_conflitto", c.motivo)} (${c.proposta} contro ${c.decisione})`);
      if(c.motivo==="codice") r.vicini.push({ codice: c.proposta, vicino: c.decisione });
    }
  }
  const firma = "a-"+hexDa(statoFirma(T),16);
  r.firma = firma; r.albero_firma = "b-"+hexDa(statoFirma(T)+"base",16);
  r.confermabile = r.blocchi.length===0;
  return r;
}

/* ------------------------------------------------------------------ i gesti (POST della lista chiusa) */
const NO = t => ({ esito:"no", testo:t });
const OK = t => ({ esito:"ok", testo:t });
function bomTornaDaVerificare(T){ for(const p of T.prodotti){ p.bom.verificata = false; if(p._albero) p._albero = false; } }
function contenitoreAmmesso(T, padreCid){ const c=T.componenti[padreCid]; return !(c && (c.tipo==="sciolto"||c.tipo==="commerciale")); }
function nuovoComponente(T, n, tipo){
  const cod = n.nodo.codice.manuale ? n.nodo.codice.manuale.codice : (n.nodo.codice.proposto || (n.nodo.codice.grezzo||{}).testo || "senza codice");
  const cid = uuidDa(T.key+":comp:"+cod+":"+n.nodo.rif);
  T.componenti[cid] = { id:cid, codice:cod, rev: n.nodo.codice.manuale ? n.nodo.codice.manuale.rev : null, descrizione:null, tipo: tipo||n.tipo||"sciolto", archiviato:false, step_strutturale:null, disegni:n.disegni, storia:false };
  n.nodo.decisione = { componente_id:cid, codice:cod, rev:T.componenti[cid].rev, autorita:"confermata", origine:"confermato" };
  n.nodo.riga_decisa = { id:n.nodo.riga_legacy.id, stato:"confermata", componente_id:cid };
  n.nodo.riga_legacy = null; n.nodo.deciso_da_persona = true;
  n.tipo = T.componenti[cid].tipo; n.origine_tipo = "confermato";
  n.classificazione.origine = n.classificazione.origine==="proposto" ? "derivato_dal_tipo" : n.classificazione.origine;
  return cid;
}
const prodottoDelComp = (T, cid) => T.prodotti.find(p=>p.componente_id===cid) || (nodoDelComponente(T,cid)||{}).p || null;

const SERVER_A = {
  rileggi(T){
    T.lettura++;
    for(const d of Object.values(T.documenti)) if(d.stato_nas==="in_coda" && T.lettura>=d.nasDa) d.stato_nas = d.nasErrore ? "errore" : "scritto";
    ricalcola(T);
  },
  leggi(T, path, q){
    if(/\/motore-a\/dati$/.test(path)) return paginaJSON(T, S.vista, S.prodotto);
    if(/\/motore-a\/anteprima\/step-strutturale$/.test(path)) return anteprimaStep(T, q);
    if(/\/distinta\/tipo$/.test(path)) return anteprimaTipo(T, q);
    if(/\/distinta\/albero\/riepilogo$/.test(path)) return riepilogoAlbero(T);
    throw new Error("lettura sconosciuta");
  },
  confermaAlbero(T, corpo){
    const r = riepilogoAlbero(T);
    const b = corpo.bozza || {};
    if(Object.keys(b).some(k=>!["formato","base"].includes(k))) return { esito:"errore", testo:"La bozza dell'albero deve restare vuota (DB7-5): niente è cambiato." };
    if(corpo.firma!==r.firma || b.base!==r.albero_firma) return NO("L'albero è cambiato dopo il riepilogo: rileggi il riepilogo e conferma di nuovo. Niente è cambiato.");
    if(!r.confermabile) return NO("L'albero non si può ancora confermare: "+r.blocchi.join("; ")+".");
    let nuovi=0, legami=0, senza=[];
    for(const p of T.prodotti){
      /* senza BOM di lavoro: il gesto copre i confermati senza STEP (default della domanda 1 di R-38, da confermare) */
      if(!p.nodi.length){ if(p.componente_id){ Object.defineProperty(p, "_albero", {value:true, enumerable:false, writable:true, configurable:true}); senza.push(p.codice_richiesto); } continue; }
      const rad = p.nodi[0];
      for(const n of p.nodi){ if(n.nodo.riga_legacy){ nuovoComponente(T, n); nuovi++; } }
      for(const n of p.nodi){
        if(n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata") continue;
        const cf = cidDelNodo(p,n);
        for(const x of n.parentela){ if(scartato(p.nodi,x.padre)) continue; const np = p.nodi.find(y=>y.nodo.rif===x.padre); const cp = np?cidDelNodo(p,np):null;
          if(!x.decisa){ x.decisa = true; legami++; }
          if(cp && cf && !T.relazioni.find(z=>z.padre===cp&&z.figlio===cf)) T.relazioni.push({ padre:cp, figlio:cf, qta:x.quantita||1 }); }
        if(n.nodo.decisione && !n.nodo.codice.confermato) n.nodo.codice.confermato = { codice:n.nodo.decisione.codice, rev:n.nodo.decisione.rev, origine:"confermato", rev_provenienza:"decisione_tracciata" };
      }
      if(!rad.nodo.codice.confermato) rad.nodo.codice.confermato = { codice:p.codice_richiesto, rev:null, origine:"confermato" };
      p.bom.verificata = true;
    }
    ricalcola(T);
    const tutti = T.prodotti.every(p=>p.bom.verificata);
    return OK(`Albero confermato per tutta la RFQ: ${nuovi} componenti nuovi, ${legami} legami confermati.${tutti?" La BOM è verificata."
      : ` ${senza.length?senza.join(", ")+": nomenclatura verificata; la gerarchia resta da verificare senza lo STEP confermato (K-02).":"La BOM resta da verificare."}`}`);
  },
  posta(T, path, p){
    const v = k => (p.get(k)||"").trim();
    let m;
    const rotta = path.replace(/^\/thread\/[^/]+/, "");
    const ok = (x) => { ricalcola(T); return x; };
    /* --- A. la fonte --- */
    if((m = rotta.match(/^\/fascicolo\/componente\/([^/]+)\/step-strutturale$/))){
      const pr = T.prodotti.find(x=>x.componente_id===m[1]);
      if(!pr) return NO("Il componente non è un prodotto di questa RFQ.");
      if(v("autorizza")!=="1") return NO("Manca «autorizzo»: niente è cambiato.");
      const a = v("documento") || v("nodo").split(":")[0];
      const ant = anteprimaStep(T, {componente:m[1], documento:v("documento"), nodo:v("nodo")});
      if(ant.gia) return NO(ant.gia);
      if(ant.spento) return NO(ant.spento);
      if(v("firma")!==ant.firma) return NO("La firma non corrisponde più all'anteprima: il server ha riletto l'anteprima, controllala e autorizza di nuovo.");
      if(ant.presa_d_atto && v("presa_d_atto")!=="1") return NO("Serve la presa d'atto: "+ant.presa_d_atto+".");
      const s = pr.strutture_candidate.find(x=>x.allegato_id===a);
      pr.nodi = s.nodi; pr.strutture_candidate = [];
      const rad = pr.nodi[0];
      rad.nodo.riga_legacy = null; rad.nodo.codice.confermato = { codice:pr.codice_richiesto, rev:T.componenti[pr.componente_id].rev, origine:"confermato" };
      rad.tipo = "finito"; rad.origine_tipo = "confermato"; rad.classificazione = { ruolo:"prodotto", categoria:"fabbricato", origine:"derivato_dal_tipo", confermata:false, motivo:null, proposta:null };
      pr.fonte = { stato:"confermata", motivo:"estrazione_riuscita", calcolata:true, estrazione:"riuscita",
        riferimento:{ allegato_id:a, documento_id:uuidDa(T.key+":docstep:"+a), sha256:T.allegati[a].sha, confermato_il:OGGI_ISO, superato:false }, candidati:[] };
      T.componenti[pr.componente_id].step_strutturale = a;
      pr.bom.verificata = false;
      return ok(OK(`STEP strutturale autorizzato: ${T.allegati[a].nome}. La BOM di lavoro ha ${pr.nodi.length} nodi, ancora da verificare: la conferma è «Conferma l'albero».`));
    }
    if((m = rotta.match(/^\/fascicolo\/componente\/([^/]+)\/step-strutturale\/revoca$/))){
      const pr = T.prodotti.find(x=>x.componente_id===m[1]);
      if(!pr || pr.fonte.stato!=="confermata") return NO("Non c'è uno STEP autorizzato da revocare.");
      if(v("sha")!==pr.fonte.riferimento.sha256) return NO("Lo STEP indicato non è quello autorizzato: rileggi la pagina.");
      pr.nodi = []; pr.strutture_candidate = copia(T.candOrig[pr.rif]); pr.bom.verificata = false;
      pr.fonte = { stato:"in_attesa_di_conferma", motivo: pr.strutture_candidate.length>1?"da_scegliere":"documento_candidato", calcolata:true, estrazione:"riuscita", riferimento:null, candidati:[] };
      T.componenti[pr.componente_id].step_strutturale = null;
      return ok(OK("Autorizzazione revocata: la BOM di lavoro si svuota, le decisioni già prese restano."));
    }
    /* --- B. la struttura --- */
    if((m = rotta.match(/^\/fascicolo\/nodo\/([^/]+)\/(accetta|scarta|codice|riapri)$/))){
      const x = nodoPerRiga(T, m[1]); if(!x) return NO("Nessuna riga con questo identificativo: rileggi la pagina.");
      const {p:pr, n} = x;
      const inBom = pr.nodi.includes(n);
      if(m[2]==="riapri"){
        if(!(n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata" && n.nodo.riga_decisa.id===m[1])) return NO("Il nodo non è scartato: non c'è niente da riaprire.");
        n.nodo.riga_legacy = { id:m[1], tipo_proposto:null, codice_manuale:null, rev_manuale:null }; n.nodo.riga_decisa = null;
        return ok(OK("Nodo riaperto: è di nuovo una proposta da decidere."));
      }
      if(!n.nodo.riga_legacy || n.nodo.riga_legacy.id!==m[1]) return NO("Il nodo non ha una proposta aperta: è già deciso.");
      if(!inBom) return NO("Lo STEP di questo nodo non è autorizzato: i gesti dei nodi partono dalla BOM di lavoro. Prima autorizza lo STEP strutturale (conferma della fonte).");
      if(m[2]==="accetta"){
        const tipo = v("tipo");
        if(tipo==="commerciale") return NO("«Accetta il nodo» non accetta il tipo commerciale: accetta il nodo e poi cambia il tipo, con l'anteprima.");
        if(tipo && !["finito","sottoassieme","sciolto"].includes(tipo)) return NO("Tipo non ammesso.");
        for(const pa of n.parentela){ const np = pr.nodi.find(y=>y.nodo.rif===pa.padre); if(!np) continue; const cp = cidDelNodo(pr,np);
          const tp = cp ? T.componenti[cp].tipo : np.tipo;
          if(tp==="sciolto"||tp==="commerciale") return NO(`Un ${etichetta("tipo_componente",tp)} non può contenere altri pezzi: ${codiceDelNodo(n)} non si accetta sotto ${codiceDelNodo(np)}. Niente è cambiato.`); }
        const cid = nuovoComponente(T, n, tipo||null);
        return ok(OK(`Nodo accettato: ${T.componenti[cid].codice} ora è un componente. La BOM resta da verificare: la conferma è «Conferma l'albero».`));
      }
      if(m[2]==="scarta"){
        n.nodo.riga_decisa = { id:m[1], stato:"scartata", componente_id:null }; n.nodo.riga_legacy = null;
        return ok(OK(`Nodo ${codiceDelNodo(n)} scartato in modo definitivo: esce dalla BOM di lavoro. I pezzi sotto non sono scartati.`));
      }
      if(m[2]==="codice"){
        const cod = v("codice"); if(!cod) return NO("Il codice è obbligatorio.");
        if(!p.has("rev")) return NO("Manca il campo rev: il modulo lo manda sempre.");
        const rev = v("rev")||null;
        n.nodo.riga_legacy.codice_manuale = cod; n.nodo.riga_legacy.rev_manuale = rev;
        n.nodo.codice.manuale = { codice:cod, rev, origine:"manuale" };
        const prima = T.conflitti.length;
        T.conflitti = T.conflitti.filter(c=>!(c.rif===n.nodo.rif && c.decisione===cod));
        return ok(OK(`Codice corretto a mano: ${cod}${rev?" rev "+rev:""}. Il codice proposto dal file resta visibile.`+(T.conflitti.length<prima?" Il conflitto di codice è risolto.":"")));
      }
    }
    if((m = rotta.match(/^\/fascicolo\/componente\/([^/]+)\/(codice|modifica|tipo|collega|sposta|scollega|archivia|rimuovi|ripristina|deroga)$/))){
      const cid = m[1], c = T.componenti[cid];
      if(!c) return NO("Componente non trovato in questa RFQ.");
      const isTarget = T.prodotti.some(x=>x.componente_id===cid);
      switch(m[2]){
        case "codice":
          if(isTarget) return NO("È il prodotto della RFQ: la rinomina del prodotto non si fa da questa pagina (DB7-3).");
          if(!v("codice")) return NO("Il codice è obbligatorio.");
          if(Object.values(T.documenti).some(d=>d.componente===cid && d.stato_nas==="scritto")) return NO("Un documento di questo componente è già copiato sul NAS: codice e documento non si correggono più.");
          c.codice = v("codice"); { const nc=nodoDelComponente(T,cid); if(nc&&nc.n.nodo.decisione) nc.n.nodo.decisione.codice=c.codice; }
          return ok(OK(`Codice del componente cambiato in ${c.codice}.`));
        case "modifica":
          if(c.descrizione===null) return NO("La descrizione registrata del componente non c'è: il gesto non parte.");
          if(!p.has("rev")||!p.has("descrizione")) return NO("Rev e descrizione si mandano sempre insieme.");
          c.rev = v("rev")||null; c.descrizione = v("descrizione")||null;
          return ok(OK("Rev e descrizione salvate."));
        case "tipo": {
          const tipo = v("tipo"); const a = anteprimaTipo(T, {componente:cid, tipo});
          if(a.spento) return NO(a.spento); if(a.errore) return NO(a.errore);
          if(v("firma")!==a.firma) return NO("La firma non corrisponde più: rileggi l'effetto del cambio.");
          c.tipo = tipo; const nc=nodoDelComponente(T,cid); if(nc){ nc.n.tipo=tipo; nc.n.origine_tipo="confermato"; }
          bomTornaDaVerificare(T);
          return ok(OK(`Tipo cambiato: ${c.codice} è ${etichetta("tipo_componente",tipo)}. L'albero torna da confermare.`));
        }
        case "collega": {
          const pad = v("padre"), q = parseInt(v("qta"),10);
          if(!pad) return NO("Scegli il padre: non c'è un valore già messo.");
          if(!(q>=1)) return NO("Scrivi la quantità: è obbligatoria e almeno 1.");
          if(pad===cid) return NO("Un componente non sta sotto se stesso.");
          if(!T.componenti[pad]) return NO("Il padre scelto non è un componente di questa RFQ.");
          if(!contenitoreAmmesso(T,pad)) return NO(`Un ${etichetta("tipo_componente",T.componenti[pad].tipo)} non può contenere altri pezzi: il legame non si crea. Niente è cambiato.`);
          const r = T.relazioni.find(z=>z.padre===pad&&z.figlio===cid);
          if(r) r.qta = q; else T.relazioni.push({padre:pad, figlio:cid, qta:q});
          bomTornaDaVerificare(T);
          return ok(OK(`${c.codice} collegato sotto ${T.componenti[pad].codice} ×${q}. L'albero torna da confermare.`));
        }
        case "sposta": {
          if(!p.has("da")||!p.has("a")) return NO("Da e A sono obbligatori.");
          const da = v("da"), a = v("a"), q = parseInt(v("qta"),10);
          if(!(q>=1)) return NO("Scrivi la quantità: è obbligatoria e almeno 1.");
          if(a && !T.componenti[a]) return NO("La destinazione non è un componente di questa RFQ.");
          if(a===cid) return NO("Un componente non sta sotto se stesso.");
          if(a && !contenitoreAmmesso(T,a)) return NO(`Un ${etichetta("tipo_componente",T.componenti[a].tipo)} non può contenere altri pezzi: lo spostamento non è ammesso. Niente è cambiato.`);
          if(da) T.relazioni = T.relazioni.filter(z=>!(z.padre===da&&z.figlio===cid));
          if(a){ const r=T.relazioni.find(z=>z.padre===a&&z.figlio===cid); if(r) r.qta=q; else T.relazioni.push({padre:a, figlio:cid, qta:q}); }
          bomTornaDaVerificare(T);
          return ok(OK(`${c.codice} spostato ${a?"sotto "+T.componenti[a].codice:"come radice"} ×${q}. L'albero torna da confermare.`));
        }
        case "scollega": {
          const pad = v("padre"); const r = T.relazioni.find(z=>z.padre===pad&&z.figlio===cid);
          if(!r) return NO("Il legame non c'è più: rileggi la pagina.");
          T.relazioni = T.relazioni.filter(z=>z!==r); bomTornaDaVerificare(T);
          return ok(OK(`${c.codice} tolto da sotto ${(T.componenti[pad]||{}).codice||"il padre"}: il componente resta.`));
        }
        case "archivia": {
          if(isTarget) return NO("Il prodotto della RFQ non si archivia da questa pagina.");
          if(!v("motivo")) return NO("Il motivo è obbligatorio.");
          c.archiviato = true; T.relazioni = T.relazioni.filter(z=>z.figlio!==cid && z.padre!==cid);
          const nc = nodoDelComponente(T,cid); if(nc){ nc.n.nodo.decisione = null; }
          bomTornaDaVerificare(T);
          return ok(OK(`${c.codice} archiviato: esce dalla BOM di lavoro con i suoi legami; documenti, proposte e storia restano.`));
        }
        case "rimuovi": {
          if(isTarget) return NO("Il prodotto della RFQ non si rimuove da questa pagina.");
          if(Object.values(T.documenti).some(d=>d.componente===cid) || T.relazioni.some(z=>z.figlio===cid||z.padre===cid)) return NO(`${c.codice} ha una storia (documenti o legami confermati): non si rimuove. Usa «Archivia».`);
          const nc = nodoDelComponente(T,cid);
          if(nc){ nc.n.nodo.riga_legacy = { id: nc.n.nodo.riga_decisa?nc.n.nodo.riga_decisa.id:uuidDa(cid), tipo_proposto:null, codice_manuale:null, rev_manuale:null }; nc.n.nodo.riga_decisa=null; nc.n.nodo.decisione=null; nc.n.origine_tipo="proposto_dalla_regola"; }
          delete T.componenti[cid]; bomTornaDaVerificare(T);
          return ok(OK("Componente rimosso del tutto: il nodo torna una proposta da decidere."));
        }
        case "ripristina": {
          if(!c.archiviato) return NO("Il componente non è archiviato.");
          c.archiviato = false;
          for(const pr of T.prodotti) for(const n of pr.nodi) if(n.nodo.riga_decisa && n.nodo.riga_decisa.componente_id===cid) n.nodo.decisione = { componente_id:cid, codice:c.codice, rev:c.rev, autorita:"confermata", origine:"confermato" };
          bomTornaDaVerificare(T);
          return ok(OK(`${c.codice} ripristinato nella BOM di lavoro con la sua storia. L'albero torna da confermare.`));
        }
        case "deroga": {
          if(!v("motivo")) return NO("Il motivo della deroga è obbligatorio.");
          const id = uuidDa(T.key+":deroga:"+cid+v("tipo")+T.lettura);
          T.deroghe[id] = { id, componente:cid, tipo:v("tipo"), motivo:v("motivo") };
          return ok(OK("Deroga registrata. Una deroga non sostituisce il 2D: la voce resta da sistemare."));
        }
      }
    }
    if((m = rotta.match(/^\/fascicolo\/relazione\/(accetta|scarta)$/))){
      const pr = T.prodotti.find(x=>x.nodi.some(n=>n.nodo.rif===v("figlio")));
      const n = pr && pr.nodi.find(x=>x.nodo.rif===v("figlio")); const pa = n && n.parentela.find(x=>x.padre===v("padre"));
      if(!pa || pa.decisa) return NO("Nessun legame proposto aperto con questi dati: rileggi la pagina.");
      if(m[1]==="scarta"){ n.parentela = n.parentela.filter(x=>x!==pa); n.nodo.padri = n.nodo.padri.filter(x=>x!==v("padre")); return ok(OK("Legame scartato: non entra nella BOM.")); }
      const np = pr.nodi.find(x=>x.nodo.rif===v("padre")); const cp = np&&cidDelNodo(pr,np), cf = cidDelNodo(pr,n);
      if(!cp||!cf) return NO("Il legame si accetta fra due componenti: prima accetta i due nodi (o conferma l'albero).");
      if(!contenitoreAmmesso(T,cp)) return NO("Il padre è un particolare: non può contenere altri pezzi.");
      pa.decisa = true; if(!T.relazioni.find(z=>z.padre===cp&&z.figlio===cf)) T.relazioni.push({padre:cp, figlio:cf, qta:pa.quantita||1});
      return ok(OK(`Legame accettato: ${codiceDelNodo(n)} sotto ${codiceDelNodo(np)} ×${pa.quantita||1}. La BOM resta da verificare.`));
    }
    if((m = rotta.match(/^\/fascicolo\/rimozione\/(accetta|scarta)$/))){
      const c = T.conflitti.find(x=>x.chiave && x.chiave.step===v("step") && x.chiave.padre===v("padre") && x.chiave.figlio===v("figlio"));
      if(!c) return NO("Nessuna rimozione aperta con questi dati.");
      T.conflitti = T.conflitti.filter(x=>x!==c);
      if(m[1]==="accetta"){ T.relazioni = T.relazioni.filter(z=>!(z.padre===v("padre")&&z.figlio===v("figlio"))); bomTornaDaVerificare(T); return ok(OK("Rimozione accettata: il legame esce dalla BOM di lavoro.")); }
      return ok(OK("Rimozione scartata: il legame resta."));
    }
    /* --- C. i documenti --- */
    if(rotta==="/fascicolo/conferma"){
      const id = v("voce"); if(p.getAll("voce").length!==1) return NO("La conferma è per una voce sola.");
      const pr = Object.values(T.proposte).find(x=>x.id===id);
      if(!pr) return NO("Nessuna proposta aperta con questo identificativo: rileggi la pagina.");
      if(pr.tipo_documento==="da_determinare") return NO("Il tipo del documento è da decidere: in questa pagina la conferma si rifiuta (U-08). Associalo a un componente o scartalo.");
      let cid = pr.componente;
      if(!cid && pr.nodo && pr.nodo.startsWith("target:")) cid = pr.nodo.slice(7);
      if(!cid && pr.nodo && pr.nodo.startsWith("cod:")){ for(const prd of T.prodotti) for(const n of prd.nodi) if((n.nodo.codice.proposto||n.nodo.rif)===pr.nodo.slice(4) && cidDelNodo(prd,n)) cid = cidDelNodo(prd,n);
        if(!cid) return NO("Il componente di questo file non c'è ancora: prima la struttura e i codici (DB7-2). Accetta il nodo o conferma l'albero, poi conferma il documento."); }
      if(pr.tipo_documento==="disegno_2d" && !cid) return NO("Il file non ha un componente: associalo prima.");
      if(cid && pr.tipo_documento==="disegno_2d" && Object.values(T.documenti).some(d=>d.componente===cid && d.tipo_documento==="disegno_2d" && d.corrente) && !pr.alternativo)
        return NO("Il componente ha già un 2D corrente: la scelta fra aggiungere e sostituire si fa nel Fascicolo (U-08).");
      const did = uuidDa(T.key+":doc:"+pr.allegato);
      const nome = T.allegati[pr.allegato].nome;
      T.documenti[did] = { id:did, allegato:pr.allegato, componente:cid, tipo_documento:pr.tipo_documento, nome, stato_nas:"in_coda", nasDa:T.lettura+2, nasErrore:!!pr.nasErrore, corrente: !pr.alternativo, rev:null };
      delete T.proposte[pr.allegato];
      T.daSmistare = T.daSmistare.filter(f=>f.allegato_id!==pr.allegato);
      return ok(OK(`Documento confermato: ${nome}${cid?" per "+T.componenti[cid].codice:""}. La copia sul NAS è in coda: «copiato» arriva solo quando la copia è scritta.`));
    }
    if((m = path.match(/^\/proposta\/([^/]+)\/scarta$/))){
      const pr = Object.values(T.proposte).find(x=>x.id===m[1]);
      if(!pr) return NO("Nessuna proposta aperta con questo identificativo.");
      if(v("ritorna_thread")!==T.id) return { esito:"errore", testo:"Manca ritorna_thread: la rotta risponderebbe con un'altra pagina." };
      T.esclusi.push(pr.allegato); delete T.proposte[pr.allegato];
      T.daSmistare = T.daSmistare.filter(f=>f.allegato_id!==pr.allegato);
      return ok(OK(`File scartato in modo definitivo: ${T.allegati[pr.allegato].nome}. Nessun gesto lo riporta indietro (DB7-8).`));
    }
    if(rotta==="/fascicolo/assegna"){
      const comp = v("componente"), prop = v("proposta"), doc = v("documento");
      if(!p.has("componente")) return NO("Manca il campo componente.");
      if(!!prop === !!doc) return NO("Serve una proposta oppure un documento, non tutti e due.");
      if(prop){
        const pr = Object.values(T.proposte).find(x=>x.id===prop); if(!pr) return NO("Proposta non trovata: rileggi la pagina.");
        if(!comp){ pr.componente = null; pr.nodo = null; pr.manuale = false;
          if(!T.daSmistare.find(f=>f.allegato_id===pr.allegato)) T.daSmistare.push({ allegato_id:pr.allegato, nome_file:T.allegati[pr.allegato].nome, motivo:"nessun_candidato", prodotti:[], orfano:false, prodotti_contesto:[] });
          return ok(OK(`Tolto dal componente: ${T.allegati[pr.allegato].nome} resta nel thread, senza componente. Non è scartato e non è confermato.`)); }
        if(!T.componenti[comp]) return NO("Il componente scelto non è di questa RFQ.");
        pr.componente = comp; pr.manuale = true; if(pr.tipo_documento==="da_determinare") pr.tipo_documento = "disegno_2d";
        T.daSmistare = T.daSmistare.filter(f=>f.allegato_id!==pr.allegato);
        return ok(OK(`Associato a mano a ${T.componenti[comp].codice}: non è una conferma, il file resta da confermare.`));
      }
      const d = T.documenti[doc]; if(!d) return NO("Documento non trovato.");
      if(!comp){ d.componente = null; d.corrente = false; return ok(OK(`Tolto dal componente: ${d.nome} resta nel thread, senza componente.`)); }
      const corr = Object.values(T.documenti).filter(x=>x.componente===comp && x.tipo_documento===d.tipo_documento && x.corrente && x!==d);
      if(corr.length && !v("scelta")) return NO("Il componente ha già documenti correnti di questo tipo: scegli se aggiungere o sostituire. Correnti: "+corr.map(x=>x.nome).join(", ")+".");
      if(v("scelta").startsWith("sostituisci:")){ const o = T.documenti[v("scelta").slice(12)]; if(o) o.corrente = false; }
      d.componente = comp; d.corrente = true;
      return ok(OK(`${d.nome} associato a ${T.componenti[comp].codice}.`));
    }
    if((m = rotta.match(/^\/fascicolo\/documento\/([^/]+)\/sostituisci$/))){
      const nu = T.documenti[m[1]], ve = T.documenti[v("vecchio")];
      if(!nu||!ve) return NO("Si sostituisce solo fra due documenti confermati.");
      ve.corrente = false; nu.corrente = true; nu.componente = ve.componente;
      return ok(OK(`${nu.nome} è il disegno corrente; ${ve.nome} resta nella storia.`));
    }
    if((m = rotta.match(/^\/fascicolo\/deroga\/([^/]+)\/revoca$/))){
      if(!T.deroghe[m[1]]) return NO("Deroga non trovata.");
      delete T.deroghe[m[1]]; return ok(OK("Deroga revocata: il requisito torna a contare."));
    }
    /* --- D. il thread --- */
    if(rotta==="/riprova-copie"){
      const er = Object.values(T.documenti).filter(d=>d.stato_nas==="errore"||d.stato_nas==="in_coda");
      if(!er.length) return NO("Nessuna copia in coda o in errore.");
      er.forEach(d=>{ d.stato_nas="in_coda"; d.nasDa=T.lettura+2; d.nasErrore=false; });
      return ok(OK(`Copie rimesse in coda: ${er.length}. «Copiato sul NAS» arriva quando la copia è scritta.`));
    }
    if(rotta==="/fascicolo/prepara"){
      if(!T.nonPreparato) return OK("Niente da preparare: i file del thread sono già scaricati e letti.");
      T.nonPreparato = false;
      for(const p of T.prodotti) if(T.fontiVere && T.fontiVere[p.rif]) p.fonte = T.fontiVere[p.rif];
      return ok(OK("File preparati: scaricati, estratti e analizzati. Qui sotto c'è l'esito nuovo, da guardare e confermare."));
    }
    if(rotta==="/fascicolo/rianalizza") return OK("Analisi accodata: gli STEP si rileggono. L'esito nuovo arriva con la prossima lettura; le decisioni prese restano.");
    if(rotta==="/smistamento/aggiorna"){
      /* lo smistamento si rifà: un file senza destinazione il cui nome è il codice di un componente confermato diventa una
         PROPOSTA di associazione (come il nome del file per un nodo). Non conferma niente: la conferma resta per voce. */
      const nuovi = [];
      for(const f of T.daSmistare.slice()){ const pr = T.proposte[f.allegato_id]; if(!pr || pr.componente || f.motivo!=="nessun_candidato") continue;
        const stem = String(f.nome_file).replace(/\.[^.]+$/,"").trim().toLowerCase();
        const c = Object.values(T.componenti).find(x=>!x.archiviato && String(x.codice).trim().toLowerCase()===stem); if(!c) continue;
        pr.componente = c.id; pr.manuale = false; if(pr.tipo_documento==="da_determinare") pr.tipo_documento = "disegno_2d";
        T.daSmistare = T.daSmistare.filter(x=>x.allegato_id!==f.allegato_id); nuovi.push(`${f.nome_file} → ${c.codice}`); }
      /* gli avvisi alla lettera di smistamento.go (aggiornaSmistamento): «già aggiornate» o «N destinazioni riscritte su M file» */
      const nf = Object.keys(T.allegati).length, conta = (n,s,p)=>`${n} ${n===1?s:p}`;
      if(!nuovi.length) return OK(`Le proposte erano già aggiornate (${conta(nf,"file","file")}).`);
      return ok(OK(`Proposte aggiornate: ${conta(nuovi.length,"destinazione riscritta","destinazioni riscritte")} su ${conta(nf,"file","file")}.`));
    }
    return { esito:"errore", testo:"Rotta della lista senza gestore nel finto server: "+rotta };
  },
};
