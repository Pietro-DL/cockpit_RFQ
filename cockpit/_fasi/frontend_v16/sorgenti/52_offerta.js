/* ================= 52 · Foglio per Franco, Costi, Listini e Offerta (ruolo Franco; zona B · R-40) =================
   SPEC_v15 §2.8. Si apre dalla consegna di Lino (o con il ruolo «Franco (simulato)»): il Foglio per Franco è la
   vista di sola lettura nei BLOCCHI del Mod 99, PER LOTTO FOTOGRAFATO, con i campi vuoti in evidenza. Poi, come in v13:
   Costi (la scheda Mod 99 rifatta dai numeri della Distinta: nessuno li ricopia) · Listini · Offerta al cliente.
   Si tengono la formula scritta di ogni euro, i chip della fonte (mod99, reale, chat, inventato) e il confronto con la
   scheda di esempio del telaio: 3.710,04 € (valore di esempio). Si corregge il n. 18: inviare, accettare, perdere o rifiutare NON cambiano la fase
   registrata (Testa.Fase): la mail arriva nella conversazione, il registro dell'offerta si aggiorna (A8, B4). */
const FONTE = {mod99:["ok","scheda costo · valore di esempio"],reale:["acc","documento · valore di esempio"],chat:["acc","chat del terzista"],inv:["warn","inventato"],risp:["ok","risposta registrata"]};
const fonteChip = f => C(...(FONTE[f]||FONTE.inv));

/* ---- tariffe orarie dei centri di lavoro (valori di esempio, nella forma della scheda costo) ---- */
const TARIFFE = [
 {k:"laser",n:"Taglio laser lamiera",eh:84,f:"mod99",nota:"1,40 € al minuto (valore di esempio)"},{k:"lasertubo",n:"Taglio laser tubo",eh:95,f:"inv"},
 {k:"piegatrice",n:"Piegatrice",eh:50,f:"mod99",nota:"40 €/h in un modello più vecchio (valore di esempio)"},{k:"operatore",n:"Secondo operatore",eh:30,f:"mod99",nota:"«2 persone»: piegatrice 50 + 30 (valore di esempio)"},
 {k:"presse",n:"Pressa idraulica · altre presse",eh:38,f:"mod99"},{k:"piegatubi",n:"Piegatubi",eh:56,f:"mod99",nota:"45 €/h nel modello vecchio (valore di esempio)"},
 {k:"sega",n:"Sega",eh:32,f:"mod99"},{k:"trapano",n:"Trapano",eh:32,f:"mod99"},{k:"mola",n:"Mola",eh:32,f:"mod99"},
 {k:"puntatrice",n:"Puntatrice",eh:32,f:"mod99"},{k:"saldatrice",n:"Saldatrice manuale",eh:32,f:"mod99"},{k:"robot",n:"Saldatrice robot",eh:54,f:"mod99"},
 {k:"fresatrice",n:"Fresatrice CNC",eh:45,f:"inv"},{k:"stud",n:"Saldatrice per prigionieri",eh:35,f:"inv"},
 {k:"banco",n:"Banco: montaggio, sbavatura, marcatura",eh:32,f:"inv"},{k:"controllo",n:"Controllo e collaudo",eh:40,f:"inv"},{k:"imballo",n:"Imballo",eh:30,f:"inv"}];
const tarDi = k => TARIFFE.find(t=>t.k===k);
const CENTRO_TAR = {lasertek3020:"laser",lasl4:"laser",fibra:"laser",lasertek3050:"laser",cutrix20:"laser",cutrixtubo:"lasertubo",px50:"piegatrice",px80:"piegatrice",px100:"piegatrice",px170:"piegatrice",px220:"piegatrice",idraulica:"presse",
 curvix516:"piegatubi",tubix58:"piegatubi",tubix86:"piegatubi",curvix980:"piegatubi",p120:"presse",p130:"presse",p315:"presse",p400:"presse",sega:"sega",trapano:"trapano",mola:"mola",
 fresix1050:"fresatrice",fresix2010:"fresatrice",puntatrice:"puntatrice",robix:"robot",robon:"robot",robyu:"robot",mig:"saldatrice",tig:"saldatrice",stud:"stud"};
const PROC_TAR = {laser:"laser",lasertubo:"lasertubo",piega:"piegatrice",curva:"piegatubi",stampa:"presse",sega:"sega",foratura:"trapano",filettatura:"trapano",smussatura:"mola",sbavatura:"banco",
 fresa:"fresatrice",rilavorazione:"trapano",sald_robot:"robot",sald_mag:"saldatrice",sald_tig:"saldatrice",prigionieri:"stud",montaggio:"banco",inserti:"banco",piantaggio:"presse",
 raddrizzatura:"banco",controllo:"controllo",campionatura:"controllo",imballo:"imballo"};
const LASER = {eurMin:1.4, vel:[[1.5,3.5,"mod99"],[2,3.5,"mod99"],[2.5,3.5,"mod99"],[3,3.5,"mod99"],[4,2.8,"mod99"],[5,2.5,"mod99"],[6,1.8,"mod99"],[8,1.5,"mod99"],
 [10,0.7,"mod99"],[12,0.6,"mod99"],[15,0.5,"mod99"],[20,0.3,"mod99"],[25,0.2,"inv"],[30,0.15,"inv"],[40,0.1,"inv"],[50,0.08,"inv"]]};
const velLaser = sp => LASER.vel.find(r=>r[0]>=sp) || LASER.vel[LASER.vel.length-1];
const PREZZI_MAT = [
 {fam:"S235",gr:"lamiera",eur:0.85,f:"mod99",rif:"valore di esempio"},{fam:"S235",gr:"tubo",eur:1.06,f:"inv"},
 /* RL-13: il prezzo del tirante Telesol è del tubo S355J2+N (a disegno «S 355 J2 N»), non di un S235 */
 {fam:"S355",q:"S355J2+N",gr:"tubo",eur:1.12,f:"mod99",rif:"tirante, caso di prova (valore di esempio)"},
 {fam:"S235",gr:"profilo",eur:0.95,f:"inv"},{fam:"S235",gr:"barra",eur:1.05,f:"inv"},{fam:"S275",gr:"*",eur:0.90,f:"inv"},
 {fam:"S355",gr:"lamiera",eur:0.74,f:"mod99",rif:"telaio skid, caso di prova (valore di esempio)"},{fam:"S355",gr:"tubo",eur:1.24,f:"mod99",rif:"telaio skid: tubo Ø127 × 10 (valore di esempio)"},
 {fam:"S355",gr:"profilo",eur:0.83,f:"mod99",rif:"telaio skid: UPN 160 (valore di esempio)"},{fam:"S355",gr:"barra",eur:1.10,f:"inv"},
 {fam:"S420",gr:"*",eur:1.15,f:"inv"},{fam:"S460",gr:"*",eur:1.20,f:"inv"},{fam:"S500",gr:"*",eur:1.30,f:"inv"},{fam:"S700",gr:"*",eur:1.52,f:"reale",rif:"valore di esempio"},
 {fam:"C40",gr:"*",eur:1.35,f:"inv"},{fam:"C45",gr:"*",eur:1.40,f:"inv"},{fam:"AISI 304",gr:"*",eur:3.90,f:"inv"},{fam:"6082",gr:"*",eur:4.40,f:"reale",rif:"valore di esempio"}];
const GRUPPO_FORMA = {lamiera:"lamiera",tubo_tondo:"tubo",tubo_rett:"tubo",profilo:"profilo",piatto:"profilo",tondo:"barra"};
/* RL-13: prima la qualità esatta (S355J2+N tubo), poi la famiglia e la forma, poi la famiglia */
function prezzoMat(q, forma){ const g = GRUPPO_FORMA[forma], fam = famigliaDi(q) || q;
  return PREZZI_MAT.find(x=>x.q && x.q===q && x.gr===g) || PREZZI_MAT.find(x=>!x.q && x.fam===fam && x.gr===g) || PREZZI_MAT.find(x=>!x.q && x.fam===fam && x.gr==="*") || null; }
const PREZZI_COMP = {d1:[0.06,"inv"],d2:[0.09,"inv"],d3:[0.08,"inv"],d4:[0.12,"mod99"],d5:[0.07,"inv"],d6:[0.12,"inv"],s1:[0.11,"inv"],s2:[0.15,"inv"],s3:[0.09,"inv"],s4:[0.12,"inv"],
 v1:[0.09,"inv"],v2:[0.08,"inv"],v3:[0.10,"inv"],r1:[0.02,"inv"],r2:[0.03,"inv"],r3:[0.15,"inv"],p1:[0.07,"inv"],m1:[9.80,"inv"],c1:[4.50,"inv"],c2:[1.20,"inv"],b1:[0.05,"inv"],g1:[1.80,"inv"]};
const COSTO_OP = {filettatura:{eur:0.25,u:"€ a filetto",par:"n",f:"mod99",rif:"telaio skid: «filetta» 0,25 × n (valore di esempio)"},
 prigionieri:{eur:0.35,u:"€ a dado o prigioniero",par:"n",f:"mod99",rif:"telaio skid: «dado m14 puntatura» 0,35 × 6 (valore di esempio)"}};
const RIC = {mat:{v:4,n:"Ricarico sulla materia prima",f:"mod99"},acq:{v:8,n:"Ricarico su semilavorati, acquisti e trasporti",f:"mod99"},
 tratt:{v:8,n:"Ricarico sui trattamenti",f:"mod99"},trasp:{v:6,n:"Trasporti, in % del subtotale (se non c’è una quotazione)",f:"mod99"},
 mdc:{v:30,n:"Margine di contribuzione obiettivo",f:"mod99",nota:"valore di esempio"}};
let ATT_STD = false;
const ATT_MIN = {laser:10,piega:20,curva:20,stampa:30,sald_robot:30,fresa:30,lasertubo:15};
const QUOTAZIONI = [
 {id:"qt1",terz:"f3",cod:"9990708A1",p:"zincatura",lotti:[[500,0.42]],data:"ieri",f:"chat",rif:"offerta_zinc_9990708A1.pdf"},
 {id:"qt2",terz:"f2",cod:"9N008519AB",p:"tornitura",lotti:[[300,4.20]],data:"oggi",f:"chat",rif:"chat di Torneria Centrale, consegna 15 giorni"},
 {id:"qt3",terz:"rtc",cod:"F+800.514263.001",p:"travi_est",lotti:[[1,600]],data:"25/03/2025",f:"reale",rif:"scheda costo del telaio: conto lavoro UPN 160 (valore di esempio)"},
 {id:"qt4",terz:"lamitaglio",cod:"F+800.514263.006",p:"laser_est",lotti:[[2,410]],data:"26/03/2025",f:"reale",rif:"offerta del terzista, conto vendita (valore di esempio)"},
 {id:"qt5",terz:"lamitaglio",cod:"F+800.514263.006",p:"trasporto",lotti:[[1,250]],corpo:true,data:"26/03/2025",f:"reale",rif:"motrice fino allo Stab. 4, a viaggio (valore di esempio)"},
 {id:"qt6",terz:"f5",cod:"9N008518AA",p:"sabbiatura",lotti:[[100,1.45],[300,1.20]],data:"03/10",f:"inv",rif:""},
 {id:"qt7",terz:"f1",cod:"9N008518AA",p:"verniciatura_polvere",lotti:[[100,4.40],[300,3.90]],data:"03/10",f:"inv",rif:""},
 /* RL-14: lo storico del tirante Telesol 73416 (SO 6218, 03/2025), un giro per riga con la sua data. È storico: si legge nel
    Ciclo e nei Listini, si sceglie a mano nei Costi, non entra nella scelta automatica di una RFQ nuova */
 {id:"qt8",terz:"mrt",cod:"640341",p:"fresatura_est",lotti:[[10,20]],data:"17/03/2025",giro:2,tempi:"15 gg lav.",f:"reale",storico:"SO 6218 · 03/2025",rif:"MRT giro 2: forcella per tirante, conto vendita, 15 gg lav."},
 {id:"qt8b",terz:"mrt",cod:"640341",p:"fresatura_est",lotti:[[5,27]],data:"24/03/2025",giro:3,tempi:"15 gg lav.",f:"reale",storico:"SO 6218 · 03/2025",rif:"MRT giro 3: forcella, lotto 5, conto vendita"},
 {id:"qt9",terz:"mrt",cod:"640339",p:"tornitura",lotti:[[20,7.5],[50,6],[100,5.5]],data:"03/03/2025",giro:1,tempi:"15 gg lav.",f:"reale",storico:"SO 6218 · 03/2025",rif:"MRT giro 1: boccola a saldare, conto vendita"},
 {id:"qt9b",terz:"mrt",cod:"640339",p:"tornitura",lotti:[[5,15],[20,7.5],[50,6],[100,5.5]],data:"24/03/2025",giro:3,tempi:"15 gg lav.",f:"reale",storico:"SO 6218 · 03/2025",rif:"MRT giro 3: boccola, ripete 20/50/100 e aggiunge il lotto 5"},
 {id:"qt10",terz:"tfr",cod:"640341",p:"fresatura_est",lotti:[[10,14]],data:"18/03/2025",giro:2,tempi:"20 gg D.O.",materiale:"S235JR",f:"reale",storico:"SO 6218 · 03/2025",rif:"TFR a penna sul disegno: S235JR invece di S 355 J2 N"},
 {id:"qt11",terz:"polvernova",cod:"5306142",p:"verniciatura_polvere",lotti:[[1,1.10]],data:"20/07/2023",f:"reale",rif:"sabbiatura + polvere blu; cataforesi 0,55 (valore di esempio)"},
 {id:"qt12",terz:"polvernova",cod:"5306143",p:"verniciatura_polvere",lotti:[[1,4.60]],data:"20/07/2023",f:"reale",rif:"sabbiatura + polvere blu; cataforesi 3,20 (valore di esempio)"}];
const tutteQuot = () => [...QUOTAZIONI, ...QUOTAZIONI_B];
/* RL-07, RL-08: il materiale del componente nella fotografia (grezzo o «materiale del disegno» della fase esterna) */
function matCiclo(c){ const g = grezzoVal(c); if(g && g.q) return g.q; const f = c && c.fasi.find(x=>valF(x,"mat")); return f ? valF(f,"mat") : null; }
/* RL-07: la quotazione usata per un lotto. La scelta automatica toglie le quotazioni sotto il lotto minimo, con il
   materiale discorde (MAT-05) o storiche: se non resta niente, «manca · scegli tu», mai un prezzo basso. Franco può
   scegliere a mano per lotto (C0.qsel[fase@lotto]) anche una quotazione esclusa: la riga lo dice. */
function sceltaQuot(T, foto, r, f, L, C0){
  const Lc = L*(r.qtaProd||1), mc = matCiclo(foto.cicli[r.cid]);
  const all = tutteQuot().filter(x=>x.p===f.p && x.cod===r.cod && (!x.tk || x.tk===T.key)).map(x=>{ const pl = prezzoLotto(x, Lc), dis = mc && matDiscorde(x.materiale, mc);
    return {x, pl, dis, escl: pl.sotto ? `sotto il lotto minimo (${x.lotti[0][0]} pz)` : dis ? `materiale «${x.materiale}» ≠ ${mc}` : x.storico ? `storico ${x.storico}` : ""}; });
  const sel = C0.qsel[f.id+"@"+L]!=null && C0.qsel[f.id+"@"+L]!=="" ? C0.qsel[f.id+"@"+L] : C0.qsel[f.id];
  if(sel==="man") return {man:true, all, Lc};
  if(sel){ const it = all.find(y=>y.x.id===sel); if(it) return {it, all, Lc, scelta:true}; }
  const ok = all.filter(y=>!y.escl).sort((a,b)=>a.pl.eur-b.pl.eur);
  return {it: ok[0]||null, all, Lc, auto:true};
}
const quotDi = id => tutteQuot().find(x=>x.id===id);
function prezzoLotto(qt, Lc){ const l = [...qt.lotti].sort((a,b)=>a[0]-b[0]); let r = l[0]; l.forEach(x=>{ if(x[0]<=Lc) r = x; }); return {eur:r[1], lotto:r[0], sotto:Lc<l[0][0]}; }

/* ---- i costi: per prodotto consegnato (fotografia di Lino), scenari = lotti fotografati ---- */
const COSTI = {}, OFFERTE = {};
const OF = {msg:null};
let prossimaSO = 5479;
function prodDiFoto(T, foto){ const q = T.rfq ? rf(T.rfq) : null, r = T.req ? rq(T.req) : null; const pids = q ? datiRFQ(q).prodotti : r ? r.prodotti : []; const pid = pids.find(x=>PROD[x].cod===foto.codice) || pids[0]; return {pid, P: PROD[pid]||{cod:foto.codice, nome:"", qta:""}}; }
/* RL-09: la chiave dei costi è la CONSEGNA (indice n), non l'ora. Una consegna nuova dello stesso prodotto parte dalla
   scheda di Franco della consegna prima (margine, €/kg, pesi, scelte, prezzi per lotto) e somma i lotti */
const chiaveCosto = (T, foto) => T.key+"|"+foto.prodotto+"|"+(foto.n!=null ? "c"+foto.n : foto.quando);
function costoDi(T, foto){
  const k = chiaveCosto(T, foto); if(COSTI[k]) return COSTI[k];
  const {P} = prodDiFoto(T,foto);
  const prima = a2Di(T).consegne.filter(c=>c!==foto && c.prodotto===foto.prodotto && (c.n!=null && foto.n!=null ? c.n<foto.n : true)).map(c=>COSTI[chiaveCosto(T,c)]).filter(Boolean).pop();
  if(prima){ const C0 = JSON.parse(JSON.stringify(prima)), scen = [...new Set([...prima.scen, ...foto.lotti])].sort((a,b)=>a-b);
    C0.prezzo = {}; scen.forEach((L,i)=>{ const j = prima.scen.indexOf(L); if(j>=0 && prima.prezzo[j]!=null && prima.prezzo[j]!=="") C0.prezzo[i] = prima.prezzo[j]; });
    C0.scen = scen; C0.daConsegna = prima.n!=null ? prima.n : null; C0.n = foto.n; return COSTI[k] = C0; }
  const C0 = {scen:foto.lotti.slice(), vol: /anno/.test(P.qta||"") ? String(parseInt(String(P.qta).replace(/\./g,""),10)) : "", target: P.target?String(P.target):"", mdc:"", amm:"", kg:{}, eurkg:{}, pzh:{}, incl:{}, qsel:{}, man:{}, manC:{}, prezzo:{}, n:foto.n};
  if(P.target) C0.prezzo[0] = String(P.target);
  /* telaio VFR: i pesi usati da Franco e le fasi comprese nella saldatura, come nella scheda reale */
  if(P.cod==="F+800.514263.000"){ const by = cod => foto.righe.find(r=>r.cod===cod);
    if(by("F+800.514263.001")) C0.kg[by("F+800.514263.001").cid] = "1134"; if(by("F+800.514263.002")) C0.kg[by("F+800.514263.002").cid] = "20";
    const root = foto.righe.find(r=>r.radice), c = root && foto.cicli[root.cid]; if(c) c.fasi.forEach(f=>{ if(f.p==="controllo") C0.incl[f.id] = true; }); }
  return COSTI[k] = C0;
}
const mdcDi = C0 => { const v = num(C0.mdc); return (v!=null?v:RIC.mdc.v)/100; };
function arrot(x){ const p = x<10?0.05:x<100?0.5:x<1000?1:5; return Math.round(x/p)*p; }
const fmtE = (x,d=2) => x==null||!isFinite(x) ? "—" : x.toLocaleString("it-IT",{minimumFractionDigits:d,maximumFractionDigits:d})+" €";
/* tempi standard quando nessuno ha scritto il tempo: inventati per il mockup */
function stimaMin(f){
  const N = k => num(valF(f,k))||0, giunti = f.giunti||[];
  switch(f.p){
    case "piega": return N("pieghe") ? [0.35*N("pieghe")+0.3, `${N("pieghe")} pieghe × 0,35 min + 0,3 min di carico`] : null;
    case "sald_mag": case "sald_tig": case "sald_robot": { const L = giunti.reduce((a,x)=>a+(x.conv.mm||0),0), cd = giunti.length; if(!L) return null;
      const v = f.p==="sald_robot"?0.8:f.p==="sald_tig"?0.2:0.4, sec = L/10/v + cd*(f.p==="sald_robot"?10:4); return [sec/60, `${fmt(L)} mm a ${fmt(v)} cm/s + ${cd} cordoni`]; }
    case "sega": return N("tagli") ? [N("tagli")*1.5, `${N("tagli")} tagli × 1,5 min`] : null;
    case "foratura": return N("n") ? [N("n")*0.4, `${N("n")} fori × 0,4 min`] : null;
    case "filettatura": return N("n") ? [N("n")*0.5, `${N("n")} filetti × 0,5 min`] : null;
    case "smussatura": return N("n") ? [N("n")*0.3, `${N("n")} smussi × 0,3 min`] : null;
    case "sbavatura": return [0.5, "0,5 min standard"];
    case "curva": return N("curve") ? [N("curve")*0.5+0.3, `${N("curve")} curve × 0,5 min + 0,3`] : null;
    case "stampa": return N("colpi") ? [N("colpi")*0.15, `${N("colpi")} colpi × 0,15 min`] : null;
    case "prigionieri": return N("n") ? [N("n")*0.15, `${N("n")} × 0,15 min`] : null;
    case "inserti": return N("n") ? [N("n")*0.2, `${N("n")} inserti × 0,2 min`] : null;
    case "piantaggio": return N("n") ? [N("n")*0.3, `${N("n")} × 0,3 min`] : null;
    case "montaggio": return [0.5+N("viti")*0.25, `0,5 min + ${N("viti")} viti × 0,25 min`];
    case "raddrizzatura": return [2, "2 min standard"];
    case "rilavorazione": return [3, "3 min standard"];
    case "controllo": { const q = valF(f,"freq"); return q==="100 %" ? [3,"100 %: 3 min"] : q==="a campione" ? [0.5,"a campione: 0,5 min"] : [0,"primo pezzo: solo attrezzaggio"]; }
    case "campionatura": return [0, "a parte"];
    case "imballo": { const t = valF(f,"tipo"); return [{"a vista":0,bancale:1,gabbia:2,cassa:4}[t]||0, `imballo ${t||"—"}`]; }
  }
  return null;
}
function materialeNostro(modo, c){ return modo==="noi" || (modo==="fuori" && c.fasi.some(f=>valF(f,"conto")==="conto lavoro")); }
function catalogoDi(cod){ return COMPONENTI.find(it=>it.cod.startsWith(cod) || (it.cli||[]).some(x=>x[1]===cod)) || null; }
/* le righe di costo di un prodotto consegnato per un lotto L: ogni riga porta il suo driver */
function righeCosto(T, foto, L, C0){
  const righe = [], esclusi = [], aParte = [], nq = [];
  const R = o => righe.push(Object.assign({eur:0, manca:false, fonte:"", edit:null}, o));
  const ric = k => RIC[k].v/100;
  for(const r of foto.righe.filter(x=>x.cid)){
    const co = foto.come[r.cid], modo = r.comm ? "comm" : co ? co.modo : null, c = foto.cicli[r.cid], Q = r.qtaProd;
    if(!modo){ R({b:"int", r, key:"x"+r.cid, voce:"come si fa: non deciso", driver:"Lino non ha deciso come si fa", q:Q, manca:true}); continue; }
    if(modo==="escluso"){ esclusi.push({r, testo:co.testo}); continue; }
    if(modo==="noquote"){ nq.push({r, testo:co.testo}); continue; }
    if(modo==="cliente"){ R({b:"acq", r, key:"c"+r.cid, voce:"fornito dal cliente", driver:"conto lavoro: costo zero", q:Q}); continue; }
    if(modo==="comm"){ const it = catalogoDi(r.cod), pr = it && PREZZI_COMP[it.id], man = num(C0.manC[r.cid]), eur = man!=null ? man : pr ? pr[0] : null;
      R({b:"acq", r, key:"c"+r.cid, voce: it ? it.desc : "componente da acquistare", driver: eur!=null ? `${fmtE(eur,3)} × ${Q} + ${RIC.acq.v} %` : "prezzo da indicare", q:Q, eur: eur!=null ? eur*(1+ric("acq"))*Q : 0, manca: eur==null, fonte: man!=null ? "manuale" : pr ? pr[1] : "", edit:{t:"manC", id:r.cid, ph: pr ? String(pr[0]).replace(".",",") : "€/pz"}}); continue; }
    if(!c || !c.fasi.length){ R({b:"int", r, key:"v"+r.cid, voce:"ciclo vuoto", driver:"Lino non ha scritto le fasi", q:Q, manca:true}); continue; }
    const g = grezzoVal(c);
    if(materialeNostro(modo, c) && g){
      const w = pesoGrezzoDi(c), kgO = num(C0.kg[r.cid]), kg = kgO!=null ? kgO : (w ? w.kg : null), gr = grezzoDi(g);
      const risp = gr && foto.prezziMP[gr.id], giac = gr && GIACENZE.find(x=>x.g===gr.id && (!certRichiesto(T) || x.cert===certRichiesto(T))), pm = prezzoMat(g.q, g.forma);
      const eO = num(C0.eurkg[r.cid]), ek = eO!=null ? eO : risp ? risp.eur : giac ? giac.costoMedio : pm ? pm.eur : null;
      const fonte = eO!=null ? "manuale" : risp ? "risp" : giac ? giac.fonte==="mod99"?"mod99":"inv" : pm ? pm.f : "";
      R({b:"mat", r, key:"m"+r.cid, voce:`${testoGrezzo(g)}`, driver: kg && ek ? `${fmt(kg,2)} kg × ${fmtE(ek)}/kg + ${RIC.mat.v} %${Q>1?" × "+Q:""}${risp?` (${esc(risp.forn)})`:giac?" (costo medio di magazzino)":""}` : "mancano peso o prezzo",
        q:Q, eur: kg && ek ? kg*ek*(1+ric("mat"))*Q : 0, manca: !(kg && ek), fonte, edit:{t:"mat", id:r.cid, kg:kgO, kgPh: w?w.kg:null, ek:eO, ekPh: risp?risp.eur:giac?giac.costoMedio:pm?pm.eur:null}});
    }
    c.fasi.forEach((f,i)=>{
      const P = PROC[f.p]||[f.p,"int"], N10 = (i+1)*10, base = {r, f, key:f.id, voce:`${P[0]} · fase ${N10}`, q:Q};
      if(C0.incl[f.id]){ R(Object.assign(base, {b: f.p==="laser"?"laser":"int", driver:"compresa in un’altra voce", eur:0, edit:{t:"int", id:f.id, incl:true}})); return; }
      if(f.p==="certificati"){ R(Object.assign(base, {b:"int", driver:"li manda il fornitore del materiale: nessun costo", eur:0})); return; }
      if(f.p==="laser"){ const sp = num((g||{}).sp), mm = num(valF(f,"m")), fori = num(valF(f,"fori"));
        if(!sp || mm==null || fori==null){ R(Object.assign(base, {b:"laser", driver:"mancano spessore, metri o fori", manca:true, edit:{t:"int", id:f.id, laser:true}})); return; }
        const [,v,vf] = velLaser(sp), t = 2*(fori+1)/60 + mm/v, att = ATT_STD ? ATT_MIN.laser : 0;
        R(Object.assign(base, {b:"laser", driver:`(2 × (${fori}+1) ÷ 60 + ${fmt(mm)} m ÷ ${fmt(v)} m/min) = ${fmt(t,2)} min × ${fmtE(LASER.eurMin)}/min${att?` + attrezzaggio ${att} min ÷ ${L}`:""}`, eur:(t*Q+att/L)*LASER.eurMin, fonte:vf, edit:{t:"int", id:f.id, laser:true}})); return; }
      if(f.p==="trasporto" || P[1]==="est"){
        const sq = sceltaQuot(T, foto, r, f, L, C0), cands = sq.all.map(y=>y.x), man = num(C0.man[f.id]);
        const rr = TRATT.has(f.p) ? ric("tratt") : ric("acq"), Lc = sq.Lc, b = f.p==="trasporto" ? "trasp" : TRATT.has(f.p) ? "tratt" : "est";
        const fuori = sq.auto ? [...new Set(sq.all.filter(y=>y.escl && !y.x.storico).map(y=>`${esc(nomeBreve(terz(y.x.terz).nome))} ${esc(y.escl)}`))] : [], nSto = sq.auto ? sq.all.filter(y=>y.x.storico && !sq.all.some(z=>z===y && !y.escl)).length : 0;
        const perche = (fuori.length ? ` · esclusa: ${fuori.join(" · ")}` : "") + (nSto ? ` · ${nSto} ${nSto===1?"quotazione storica":"quotazioni storiche"} non usate (si scelgono a mano)` : "");
        if(sq.man || (!sq.it && man!=null)){ R(Object.assign(base, {b, driver: man!=null ? `a mano ${fmtE(man)}/pz + ${Math.round(rr*100)} %` : "prezzo da scrivere", eur: man!=null ? man*(1+rr)*Q : 0, manca: man==null, fonte:"manuale", edit:{t:"est", id:f.id, cands, man}})); return; }
        if(!sq.it){ R(Object.assign(base, {b, driver: cands.length ? `nessuna quotazione utilizzabile per ${Lc} pz${perche} · manca · scegli tu` : "nessuna quotazione: chiedila al terzista o scrivi il prezzo", manca: f.p!=="trasporto" || cands.length>0, edit:{t:"est", id:f.id, cands, man}})); return; }
        const qt = sq.it.x, pl = sq.it.pl, eur = qt.corpo ? pl.eur*(1+rr)/L : pl.eur*(1+rr)*Q;
        R(Object.assign(base, {b, driver:`${esc(terz(qt.terz).nome)} · ${qt.corpo ? fmtE(pl.eur)+" a viaggio ÷ "+L : fmtE(pl.eur)+"/pz (lotto da "+pl.lotto+")"+(!pl.sotto && pl.lotto<Lc && !qt.lotti.some(x=>x[0]===Lc)?` · lotto ${Lc} non quotato: si usa il lotto ${pl.lotto}`:"")+(sq.it.escl?` · ⚠ ${esc(sq.it.escl)}, scelta da Franco`:"")} + ${Math.round(rr*100)} %${perche}`, eur, fonte:qt.f, matDis: !!sq.it.dis, edit:{t:"est", id:f.id, cands, man}})); return; }
      const macc = valF(f,"macc"), tk = CENTRO_TAR[macc] || PROC_TAR[f.p] || "banco", Tt = tarDi(tk), eh = Tt.eh + (valF(f,"op")==="2" ? tarDi("operatore").eh : 0);
      const pzF = num(C0.pzh[f.id]), lt = leggiTempo(valF(f,T_)), pzL = lt && !lt.errore ? lt.pzh : null, op = COSTO_OP[f.p], att = ATT_STD ? (ATT_MIN[f.p]||0) : 0;
      let min = null, drv = "", fonte = Tt.f;
      if(pzF){ min = 60/pzF; drv = `${fmtE(eh,0)}/h ÷ ${fmt(pzF)} pz/h (Franco)`; fonte = "manuale"; }
      else if(pzL){ min = 60/pzL; drv = `${fmtE(eh,0)}/h × ${esc(valF(f,T_))} (Lino)`; }
      else if(op && num(valF(f,op.par))){ const k = num(valF(f,op.par)); R(Object.assign(base, {b:"int", driver:`${k} × ${fmtE(op.eur)} (${op.u})${Q>1?" × "+Q:""}`, eur:k*op.eur*Q, fonte:op.f, edit:{t:"int", id:f.id, ph:"pz/h"}})); return; }
      else { const s = stimaMin(f);
        /* RL-25: il controllo senza un tempo scritto non si stima: «manca il tempo» (la stima standard resta nel testo) */
        if(s && f.p==="controllo" && s[0]>0){ R(Object.assign(base, {b:"int", driver:`manca il tempo del controllo (la stima standard sarebbe ${esc(s[1])}: non si usa)`, manca:true, edit:{t:"int", id:f.id, ph:"pz/h"}})); return; }
        if(s){ min = s[0]; drv = `${s[1]} = ${fmt(s[0],2)} min × ${fmtE(eh,0)}/h · tempo standard`; fonte = "inv"; } }
      if(min==null){ R(Object.assign(base, {b:"int", driver:"mancano i dati della fase o il tempo", manca:true, edit:{t:"int", id:f.id, ph:"pz/h"}})); return; }
      R(Object.assign(base, {b:"int", driver: drv + (att?` + attrezzaggio ${att} min ÷ ${L}`:"") + (valF(f,"op")==="2"?" · 2 operatori":""), eur:(min*Q+att/L)/60*eh, fonte, edit:{t:"int", id:f.id, ph: pzL ? fmt(pzL,2) : "pz/h"}}));
    });
  }
  /* v16: le preparazioni (R-11) si leggono vive dal finto server A2: una risposta registrata dopo la consegna arriva qui.
     Solo quelle «ammortizzate» entrano nel costo al pezzo; «a parte» diventano una riga dell'offerta; «del cliente» non costano. */
  const O = offertaDi(T);
  for(const x of prepDi(T, foto)){ if(x.esiste) continue; const modo = O.pagaAttr[x.key] || "ammortizzata"; if(modo!=="ammortizzata") continue;
    const r = foto.righe.find(y=>y.cid===x.cid) || {cod:x.cod, cid:x.cid}, costo = num(x.costo), amm = num(C0.amm)||num(C0.vol)||null;
    R({b:"attr", r, key:"p"+x.key, voce:x.nome+(x.settimane?` · ${x.settimane} sett.`:""), driver: costo ? `${fmtE(costo,0)} ÷ ${amm?fmt(amm,0)+" pezzi":"volume da indicare"}${x.fonte?" · "+x.fonte:""}` : "costo da quotare", q:1, eur: costo&&amm ? costo/amm : 0, manca: !costo||!amm}); }
  return {righe, esclusi, aParte, nq};
}
function totaliCosto(T, foto, C0, i){
  const L = C0.scen[i]||1, {righe, esclusi, aParte, nq} = righeCosto(T, foto, L, C0);
  const att = righe.filter(r=>r.b==="attr").reduce((a,r)=>a+r.eur,0), sub = righe.filter(r=>r.b!=="attr"&&r.b!=="trasp").reduce((a,r)=>a+r.eur,0);
  /* RL-15: senza una quotazione il trasporto è il 5 % del subtotale più il ricarico acquisti (Mod 99, Q111: × 1,10) */
  const tr = righe.filter(r=>r.b==="trasp"), trasp = tr.length ? tr.reduce((a,r)=>a+r.eur,0) : sub*RIC.trasp.v/100*(1+RIC.acq.v/100);
  const tot = sub+trasp, conAtt = tot+att, mdc = mdcDi(C0), calc = conAtt/(1-mdc), off = num(C0.prezzo[i]), prezzo = off!=null ? off : arrot(calc);
  const peso = foto.righe.filter(r=>!r.radice && SEMI[r.cod] && SEMI[r.cod].netto).reduce((a,r)=>a+num(SEMI[r.cod].netto[0])*r.qtaProd,0);
  return {L, righe, esclusi, aParte, nq, sub, trasp, trConQuot:tr.length>0, tot, att, conAtt, calc, prezzo, mdcEff: prezzo ? (prezzo-conAtt)/prezzo : null, peso, manca: righe.filter(r=>r.manca).length};
}

/* le preparazioni dei prodotti consegnati: dal finto server A2 (preparazioniRFQ, C2) o, prima, dai cicli fotografati */
function prepDi(T, foto){
  const cids = new Set(foto.righe.map(r=>r.cid));
  const norm = x => Object.assign({}, x, {key: x.cid+"|"+(x.key||x.k+(x.fid?"@"+x.fid:"")), nome: x.nome || nomePrep(x.k), esiste: x.stato==="esiste già" || x.stato==="esiste_gia"});
  if(typeof preparazioniRFQ==="function"){ try { return preparazioniRFQ(T).filter(x=>cids.has(x.cid) && x.stato!=="proposta").map(norm); } catch(err){} }
  const out = [];
  for(const r of foto.righe) Object.entries(((foto.cicli[r.cid]||{}).prep)||{}).forEach(([k,v])=>{ if(v.on) out.push(norm({cid:r.cid, cod:r.cod, k, fid:v.nata||null, stato:v.stato, costo:v.costo, settimane:v.settimane||null, fonte:v.fonte||""})); });
  return out;
}
/* le settimane dell'attrezzatura pesano sul primo ordine (Cabine Omega: 20 settimane): proposta ◇, decide Franco */
const settAttrezzature = (T, foto) => Math.max(0, ...prepDi(T,foto).filter(x=>!x.esiste).map(x=>num(x.settimane)||0));

/* ================= le viste del ruolo Franco ================= */
const BLOCCHI = [["mat","Materia prima"],["laser","Taglio laser"],["int","Lavorazioni interne"],["est","Semilavorati e lavorazioni esterne"],["tratt","Trattamenti"],["acq","Componenti acquistati e del cliente"],["trasp","Trasporti"],["attr","Attrezzature ammortizzate"]];
function fotoCorrente(T){ const S2 = a2Di(T); if(!S2.consegne.length) return null; const i = S.foto!=null && S2.consegne[S.foto] ? S.foto : S2.consegne.length-1; S.foto = i; return S2.consegne[i]; }
function vistaFrancesco(T, p){
  const foto = fotoCorrente(T);
  if(S.esitoVista==="listini") return vistaListini();
  if(!foto) return `<div class="empty"><span class="ib">${I("file",22)}</span><h3>Lino non ha ancora consegnato</h3><p>Il Foglio per Franco, i Costi e l'Offerta nascono dalla consegna dell'Esito: una fotografia della Distinta per ogni lotto spuntato (R-40).</p>
    <div class="acts" style="justify-content:center"><button type="button" class="ma-btn" data-esito-vista="esito">← Esito</button><button type="button" class="ma-btn" data-esito-vista="listini">Listini e tariffe</button></div></div>`;
  return `${OF.msg?`<div class="esito-b ${OF.msg.ok?"ok":"no"}" role="status">${I(OF.msg.ok?"check":"alert",16)}<div>${OF.msg.t}</div></div>`:""}
    ${S.esitoVista==="foglio" ? vistaFoglio(T,foto) : S.esitoVista==="costi" ? vistaCosti(T,foto) : vistaOfferta(T,foto)}`;
}
/* il Foglio per Franco: i numeri di Lino nei blocchi Mod 99, sola lettura, i campi vuoti in evidenza */
function vistaFoglio(T, foto){
  const C0 = costoDi(T,foto), S2 = a2Di(T);
  const tt = totaliCosto(T, foto, C0, 0);
  const dati = r => { const c = foto.cicli[r.r.cid];
    if(r.b==="mat" && c){ const w = pesoGrezzoDi(c), n = SEMI[r.r.cod] && SEMI[r.r.cod].netto; const b = c.fasi.map(f=>valF(f,"barre")).find(Boolean);
      return esc([r.voce, b, w?`grezzo a misura ${fmt(w.kg,2)} kg, senza sfrido di barra (${w.formula})`:"peso del grezzo da calcolare", n?`netto ${fmt(n[0],2)} kg ${n[1]}`:"", certRichiesto(T)?`cert. ${certRichiesto(T)}`:""].filter(Boolean).join(" · ")); }
    if(!r.f) return esc(r.voce); return esc(riassuntoFase(T, r.r, c, r.f)) || "—"; };
  return `<div class="foglio-v">
    <div class="dati-h"><div><span class="lab">Foglio per Franco · sola lettura</span><h2><span class="mono">${esc(foto.codice)}</span></h2>
      <p class="k3 piccolo">Consegnato da ${esc(foto.firma?foto.firma.chi:"—")} il ${esc(foto.quando)} · fotografia della Distinta: un cambio dopo la consegna non cambia questo foglio.${foto.noquote?" · <b>no quote</b>: "+esc(foto.motivo):""}</p></div><span class="sp"></span>
      <span class="fr-lotti">${S2.consegne.map((c,i)=>`<button type="button" class="ma-btn ma-btn--sm${i===S.foto?" ma-btn--ghost":""}" data-esito-vista="foglio" data-foto="${i}" aria-pressed="${i===S.foto}">consegna ${i+1} · lotti ${c.lotti.join(", ")}</button>`).join("")}</span></div>
    ${distintaCambiata(T,foto)&&foto===a2Di(T).consegne[a2Di(T).consegne.length-1]?`<div class="zona-a-ro zona-a-ro--cambio" role="note"><span class="lab">Distinta cambiata</span><span>${esc(foto.firma?foto.firma.chi:"Lino")} ha cambiato la Distinta dopo la consegna del ${esc(foto.quando)}: arriva una nuova consegna. Questo foglio resta la fotografia di allora.</span></div>`:""}
    ${foto.zonaA&&foto.zonaA.presaAtto?`<div class="zona-a-ro" role="note"><span class="lab">Consegnata con la zona A non pronta</span><span>prodotto <b>${esc(foto.zonaA.stato)}</b> · ${esc(foto.zonaA.motivi)}</span><span>${esc(foto.zonaA.bom)}</span><span>${esc(foto.zonaA.documenti)}</span></div>`:""}
    <div class="esito-blocco"><header><h3>Esiti per componente</h3><span class="k3 piccolo">pieno = deciso da Lino · ◇ = proposto dal sistema, non deciso</span></header><div class="dd">${foto.esiti.map(x=>`<div><span class="mono">${esc(x.cod)}</span>${x.deciso===false?`<span class="liv liv--${x.v==="DUBBIO"?"DB":x.v} liv--prop" title="proposta del sistema (R-35): Lino non l'ha decisa">◇ ${esc(x.v)} · proposto, non deciso</span>`:`<span class="liv liv--${x.v==="DUBBIO"?"DB":x.v}">${esc(x.v)}</span>`}${x.nota?`<span class="k3 piccolo">${esc(x.nota)}</span>`:""}</div>`).join("")}</div></div>
    ${BLOCCHI.map(([b,tit])=>{ const rr = tt.righe.filter(r=>r.b===b); if(!rr.length) return "";
      return `<section class="dblock fr-blocco"><header><h3>${tit}</h3></header><div class="wrap-tb"><table class="tb fr-tab"><thead><tr><th>Componente</th><th>Voce</th><th>Dati di Lino</th><th>Q.tà</th></tr></thead><tbody>
        ${rr.map(r=>`<tr><td class="mono">${esc(r.r.cod)}</td><td>${esc(r.voce)}</td><td class="${r.manca?"vuoto":""}">${r.manca?"manca: "+esc(r.driver):dati(r)}</td><td class="mono">${r.q}</td></tr>`).join("")}</tbody></table></div></section>`; }).join("")}
    ${tt.esclusi.length?`<p class="piccolo">Esclusi dalla fornitura: ${tt.esclusi.map(x=>`<span class="mono">${esc(x.r.cod)}</span> «${esc(x.testo)}»`).join(" · ")}</p>`:""}
    <div class="acts"><button type="button" class="ma-btn ma-btn--pri" data-esito-vista="costi">Vai ai costi →</button></div></div>`;
}
function campoCosto(T, foto, r){
  const e = r.edit; if(!e) return "";
  const C0 = costoDi(T,foto);
  if(e.t==="mat") return `<span class="cedit"><label>kg <input data-cs="kg|${e.id}" value="${e.kg!=null?String(e.kg).replace(".",","):""}" placeholder="${e.kgPh?fmt(e.kgPh,2):"kg"}" inputmode="decimal"></label>
    <label>€/kg <input data-cs="eurkg|${e.id}" value="${e.ek!=null?String(e.ek).replace(".",","):""}" placeholder="${e.ekPh!=null?fmt(e.ekPh,2):"€/kg"}" inputmode="decimal"></label></span>`;
  if(e.t==="int" && (e.laser||e.incl)) return `<span class="cedit"><label><input type="checkbox" data-cs="incl|${e.id}" ${e.incl?"checked":""}> compresa</label></span>`;
  if(e.t==="int") return `<span class="cedit"><label>pz/h <input data-cs="pzh|${e.id}" value="${esc(C0.pzh[e.id]||"")}" placeholder="${esc(e.ph||"pz/h")}" inputmode="decimal"></label><label><input type="checkbox" data-cs="incl|${e.id}"> compresa</label></span>`;
  if(e.t==="est"){
    /* RL-04: la quotazione si sceglie PER LOTTO (C0.qsel[fase@lotto]); vuoto = la più conveniente fra quelle utilizzabili */
    const mc = r.r && foto.cicli[r.r.cid] ? matCiclo(foto.cicli[r.r.cid]) : null;
    const opz = sel => `<option value="" ${!sel?"selected":""}>la più conveniente</option>${e.cands.map(x=>`<option value="${x.id}" ${sel===x.id?"selected":""}>${esc(nomeBreve(terz(x.terz).nome))} · ${x.lotti.map(([l,p])=>l+" pz "+fmtE(p)).join(" / ")}${x.f==="risp"?" · RISP"+(x.giro?" giro "+x.giro:""):""}${x.storico?" · storico "+esc(x.storico):""}${mc&&matDiscorde(x.materiale,mc)?" · ⚠ "+esc(x.materiale):""}</option>`).join("")}<option value="man" ${sel==="man"?"selected":""}>prezzo a mano</option>`;
    const selDi = L => C0.qsel[e.id+"@"+L]!=null && C0.qsel[e.id+"@"+L]!=="" ? C0.qsel[e.id+"@"+L] : (C0.qsel[e.id]||"");
    const piu = C0.scen.length>1, anyMan = C0.scen.some(L=>selDi(L)==="man");
    return `<span class="cedit cedit--lotti">${C0.scen.map(L=>`<label class="qsel-l">${piu?`<span class="k3">lotto ${fmt(L,0)}</span>`:""}<select data-cs="qsel|${e.id}@${L}" aria-label="Quotazione usata${piu?" per il lotto "+L:""}">${opz(selDi(L))}</select></label>`).join("")}
    ${anyMan||!e.cands.length?`<label>€/pz <input data-cs="man|${e.id}" value="${e.man!=null?String(e.man).replace(".",","):""}" inputmode="decimal" placeholder="€/pz"></label>`:""}</span>`; }
  if(e.t==="manC") return `<span class="cedit"><label>€/pz <input data-cs="manC|${e.id}" value="${esc(C0.manC[e.id]||"")}" placeholder="${esc(e.ph)}" inputmode="decimal"></label></span>`;
  return "";
}
function vistaCosti(T, foto){
  const C0 = costoDi(T,foto), Tt = C0.scen.map((_,i)=>totaliCosto(T,foto,C0,i)), t0 = Tt[0], {P} = prodDiFoto(T,foto);
  const head = `<tr><th>Componente</th><th>Voce</th><th>Da dove viene il costo</th><th>Valore usato</th>${Tt.map(t=>`<th class="num">€ / pz<br><span class="k3">lotto ${fmt(t.L,0)}</span></th>`).join("")}</tr>`;
  const corpo = BLOCCHI.map(([b,tit])=>{ const rr = t0.righe.filter(r=>r.b===b); if(!rr.length) return "";
    const somme = Tt.map(t=>t.righe.filter(r=>r.b===b).reduce((a,r)=>a+r.eur,0));
    const mancaB = rr.some(r=>r.manca);
    return `<tr class="blocco"><th colspan="4">${tit}${mancaB?` <span class="manca-c">incompleto</span>`:""}</th>${somme.map((s,i)=>`<th class="num" data-l="lotto ${Tt[i].L}">${fmtE(s)}${mancaB?"<sup>*</sup>":""}</th>`).join("")}</tr>`+
      rr.map(r=>{ const idx = t0.righe.indexOf(r); return `<tr class="${r.manca?"manca":""}"><td><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vai="3|${r.r.cid}|${r.f?r.f.id:""}" title="Apri nel Ciclo"><span class="mono">${esc(r.r.cod)}</span></button></td><td>${esc(r.voce)}${r.q>1&&b!=="attr"?` <span class="k3">× ${r.q}</span>`:""}</td>
        <td class="drv">${r.driver}${r.fonte&&r.fonte!=="manuale"?" "+fonteChip(r.fonte):r.fonte==="manuale"?" "+C("neu","scritto da Franco"):""}${r.manca?` <span class="manca-c">manca</span>`:""}</td>
        <td>${campoCosto(T,foto,r)}</td>${Tt.map(t=>`<td class="num mono" data-l="€/pz lotto ${t.L}">${r.manca?`<span class="k3" title="voce senza dati: non è zero">—</span>`:fmtE(t.righe[idx]?t.righe[idx].eur:0)}</td>`).join("")}</tr>`; }).join(""); }).join("");
  const riga = (lab, fn, cls) => `<tr class="${cls||""}"><th colspan="4">${lab}</th>${Tt.map((t,i)=>`<td class="num mono" data-l="lotto ${t.L}">${fn(t,i)}</td>`).join("")}</tr>`;
  const tgt = num(C0.target);
  const q = (k, lab, val, ph, u) => `<label class="qv"><span class="l">${lab}</span><span class="in"><input data-cs="${k}|" value="${esc(val||"")}" placeholder="${esc(ph||"")}" inputmode="decimal">${u?`<b>${u}</b>`:""}</span></label>`;
  return `<div class="costi-v">
    <div class="dati-h"><div><span class="lab">Scheda costo · analisi di fattibilità (Mod 99)</span><h2 title="Ogni importo nasce da un dato della fotografia di Lino e da un valore dei listini. Franco corregge dove serve (peso, €/kg, pezzi l’ora, quotazione) e la scheda si ricalcola.">Costi · <span class="mono">${esc(foto.codice)}</span> ${esc(P.nome||"")}</h2></div></div>
    <div class="cost-param">${q("vol","Volume annuo",C0.vol,"da chiedere","pz/anno")}${q("target","Prezzo obiettivo del cliente",C0.target,"se c’è","€")}${q("mdc","Margine obiettivo",C0.mdc,String(RIC.mdc.v),"%")}${q("amm","Attrezzature ammortizzate su",C0.amm,C0.vol||"volume","pz")}
      <div class="scen"><span class="l" title="un lotto nuovo si chiede a Lino: torna in Esito, lotto nuovo non spuntato (§2.9)">Lotti fotografati da Lino</span><span class="scen-l">${C0.scen.map(l=>`<span class="scen-c mono">${fmt(l,0)} pz</span>`).join("")}</span></div></div>
    ${t0.righe.length?`<div class="wrap-tb scroll"><table class="tb costi"><thead>${head}</thead><tbody>${corpo}
      ${riga("Subtotale materiali e lavorazioni",t=>fmtE(t.sub),"tot")}
      ${t0.trConQuot?"":riga(`Trasporti · ${RIC.trasp.v} % del subtotale + ${RIC.acq.v} %`,t=>fmtE(t.trasp))}
      ${riga(`Costo totale del prodotto${t0.manca?" "+'<span class="manca-c">incompleto · '+t0.manca+(t0.manca===1?' mancante':' mancanti')+'</span>':""}`,t=>`<b data-totale="1">${fmtE(t.tot)}</b>${t0.manca?"<sup>*</sup>":""}`,"tot forte")}
      ${t0.att?riga("Più attrezzature ammortizzate",t=>fmtE(t.conAtt)):""}
      ${riga(`Prezzo calcolato · margine ${Math.round(mdcDi(C0)*100)} %`,t=>fmtE(t.calc))}
      ${riga("Prezzo offerto",(t,i)=>`<input class="prezzo" data-cs="prezzo|${i}" value="${C0.prezzo[i]!=null&&C0.prezzo[i]!==""?esc(C0.prezzo[i]):""}" placeholder="${fmt(t.prezzo,2)}" inputmode="decimal" aria-label="Prezzo offerto per il lotto ${t.L}">`,"prezzo-r")}
      ${riga(`Margine effettivo${t0.manca?" · su costi incompleti":""}`,t=>t.mdcEff==null?"—":`<b class="${t.mdcEff<mdcDi(C0)-0.0005?"neg":""}" ${t.mdcEff<mdcDi(C0)-0.0005?`data-sotto-obiettivo="1" title="sotto l'obiettivo del ${Math.round(mdcDi(C0)*100)} %"`:""}>${fmt(t.mdcEff*100,1)} %</b>${t.mdcEff<mdcDi(C0)-0.0005?` <span class="manca-c">sotto il ${Math.round(mdcDi(C0)*100)} %</span>`:""}${t0.manca?"<sup>*</sup>":""}`)}
      ${t0.peso?riga("Prezzo al kg",t=>fmtE(t.prezzo/t.peso)):""}
      ${tgt?riga("Scarto dal prezzo obiettivo",t=>`${fmtE(t.prezzo-tgt)}`):""}</tbody></table></div>`:`<div class="empty"><p>Nessuna riga: la fotografia non ha cicli.</p></div>`}
    ${(()=>{ const sotto = Tt.filter(t=>t.mdcEff!=null && t.mdcEff<mdcDi(C0)-0.0005); return sotto.length ? `<div class="esito-b no" role="status" data-avviso-margine="1">${I("alert",16)}<div><b>Margine sotto l'obiettivo del ${Math.round(mdcDi(C0)*100)} %</b> per ${sotto.map(t=>`il lotto ${fmt(t.L,0)} (${fmt(t.mdcEff*100,1)} % a ${fmtE(t.prezzo)})`).join(", ")}: il prezzo offerto è sotto il prezzo calcolato.</div></div>` : ""; })()}
    ${P.rifSC?`<div class="confronto ${Math.abs(t0.tot-P.rifSC.costo)<0.5?"ok":""}" data-confronto="1">${I(Math.abs(t0.tot-P.rifSC.costo)<0.5?"check":"alert",16)}<div><b>Confronto con la scheda costo di riferimento</b> (${esc(P.rifSC.rif)}): costo ${fmtE(P.rifSC.costo)}, margine ${fmt(P.rifSC.mdc,1)} %.
      Qui: ${fmtE(t0.tot)}${Math.abs(t0.tot-P.rifSC.costo)<0.5?" · coincide.":" · differenza "+fmtE(t0.tot-P.rifSC.costo)+": viene dai dati scritti nella Distinta."}</div></div>`:""}
    ${t0.esclusi.length?`<p class="k3 piccolo">Esclusi dalla fornitura: ${t0.esclusi.map(x=>`<span class="mono">${esc(x.r.cod)}</span>`).join(", ")}.</p>`:""}
    ${t0.manca?`<p class="k3 piccolo"><span class="manca-c">manca</span> ${plur(t0.manca,"voce senza dati","voci senza dati")}: si completano nel Ciclo (clic sul codice) o qui, a mano.</p>`:""}
    <div class="acts"><button type="button" class="ma-btn ma-btn--pri" data-esito-vista="offerta">${I("file",14)} Vai all’offerta al cliente →</button></div></div>`;
}
function opzProcessi(sel){ return LAVORAZIONI.map(([g,gn,l])=>`<optgroup label="${gn}">${l.map(([k,n,t])=>`<option value="${k}" ${k===sel?"selected":""}>${n}${t==="est"?" · terzisti":""}</option>`).join("")}</optgroup>`).join(""); }
function vistaListini(){
  const inp = (attr,v,ph) => `<input class="lin" ${attr} value="${v==null?"":String(v).replace(".",",")}" placeholder="${ph||""}" inputmode="decimal">`;
  const sez = (tit,nota,th,rows) => `<section class="dblock"><header><h3>${tit}</h3><span class="k3 piccolo">${nota}</span></header><div class="wrap-tb scroll"><table class="tb"><thead><tr>${th.map(x=>`<th>${x}</th>`).join("")}</tr></thead><tbody>${rows.join("")}</tbody></table></div></section>`;
  return `<div class="listini-v">
    <div class="dati-h"><div><span class="lab">Banca dati dei costi · ruolo Franco</span><h2>Listini e tariffe</h2>
      <p class="k3 piccolo" style="max-width:86ch">I valori che la scheda costo usa. Ogni riga dice da dove viene: ${fonteChip("mod99")} dalle schede costo reali, ${fonteChip("reale")} da un documento reale, ${fonteChip("chat")} da una chat dei terzisti, ${fonteChip("risp")} da una risposta registrata nella Distinta, ${fonteChip("inv")} inventato per il mockup. €/kg e tariffe non stanno nel foglio di Lino.</p></div></div>
    ${sez("Tariffe orarie dei centri di lavoro","valori di esempio",["Centro di lavoro","€/h","Fonte","Nota"],TARIFFE.map(t=>`<tr><td>${esc(t.n)}</td><td>${inp(`data-ls="tar|${t.k}"`,t.eh)}</td><td>${fonteChip(t.f)}</td><td class="k3">${esc(t.nota||"")}</td></tr>`))}
    ${sez("Taglio laser","tempo = 2 × (fori + 1) ÷ 60 + metri ÷ velocità; costo = tempo × €/min",["Spessore fino a (mm)","Velocità (m/min)","Fonte"],[`<tr><td><b>€ al minuto</b></td><td>${inp('data-ls="lasermin|"',LASER.eurMin)}</td><td>${fonteChip("mod99")}</td></tr>`,...LASER.vel.map((r,i)=>`<tr><td class="mono">${fmt(r[0])}</td><td>${inp(`data-ls="vel|${i}"`,r[1])}</td><td>${fonteChip(r[2])}</td></tr>`)])}
    ${sez("Materie prime","€/kg per famiglia e forma (le risposte registrate dei fornitori hanno la precedenza)",["Famiglia","Forma","€/kg","Fonte","Riferimento"],PREZZI_MAT.map((x,i)=>`<tr><td class="mono">${esc(x.fam)}</td><td>${x.gr==="*"?"tutte":esc(x.gr)}</td><td>${inp(`data-ls="mat|${i}"`,x.eur)}</td><td>${fonteChip(x.f)}</td><td class="k3">${esc(x.rif||"")}</td></tr>`))}
    ${sez("Componenti a catalogo","€ al pezzo",["Codice","Descrizione","€/pz","Fonte"],COMPONENTI.map(it=>{ const pp = PREZZI_COMP[it.id]||[null,"inv"]; return `<tr><td class="mono">${esc(it.cod)}</td><td>${esc(it.desc)}</td><td>${inp(`data-ls="comp|${it.id}"`,pp[0],"€/pz")}</td><td>${fonteChip(pp[1])}</td></tr>`; }))}
    ${sez("Piccole operazioni a pezzo","come le scrive Franco nella scheda",["Operazione","€","Unità","Fonte","Riferimento"],Object.entries(COSTO_OP).map(([k,o])=>`<tr><td>${esc(PROC[k][0])}</td><td>${inp(`data-ls="op|${k}"`,o.eur)}</td><td class="k3">${esc(o.u)}</td><td>${fonteChip(o.f)}</td><td class="k3">${esc(o.rif)}</td></tr>`))}
    ${sez("Ricarichi e margine","percentuali",["Voce","%","Fonte","Nota"],Object.entries(RIC).map(([k,r])=>`<tr><td>${esc(r.n)}</td><td>${inp(`data-ls="ric|${k}"`,r.v)}</td><td>${fonteChip(r.f)}</td><td class="k3">${esc(r.nota||"")}</td></tr>`))}
    <section class="dblock"><header><h3>Quotazioni dei terzisti</h3><span class="k3 piccolo">prezzo per lotto; la scheda sceglie la più conveniente per il lotto, Franco può cambiarla</span></header>
      <div class="wrap-tb scroll"><table class="tb"><thead><tr><th>Terzista</th><th>Codice</th><th>Lavorazione</th><th>Lotti e prezzi</th><th>Materiale dichiarato</th><th>Data · giro</th><th>Fonte</th><th>Riferimento</th></tr></thead><tbody>
      ${tutteQuot().map(x=>`<tr><td>${esc(terz(x.terz).nome)}</td><td class="mono">${esc(x.cod)}</td><td>${esc((PROC[x.p]||[x.p])[0])}</td><td class="mono">${x.lotti.map(([l,pp])=>`${l} pz · ${fmtE(pp)}`).join("<br>")}${x.corpo?' <span class="k3">a viaggio</span>':""}</td><td>${x.materiale?esc(x.materiale)+(x.riserva?' <span class="manca-c" title="MAT-05: non confrontabile con il disegno">MAT-05</span>':""):`<span class="k3">—</span>`}</td><td>${esc(x.data)}${x.giro?` · giro ${x.giro}`:""}${x.storico?`<br><span class="k3">storico ${esc(x.storico)}</span>`:""}</td><td>${fonteChip(x.f)}</td><td class="k3">${esc(x.rif)}</td></tr>`).join("")}</tbody></table></div>
      <div class="nuovaq"><span class="lab">Nuova quotazione</span><div class="qgrid">
        <label class="qv"><span class="l">Terzista</span><span class="in"><select id="nq-terz"><option value="">scegli</option>${TERZISTI.map(t=>`<option value="${t.id}">${esc(t.nome)}</option>`).join("")}</select></span></label>
        <label class="qv"><span class="l">Codice</span><span class="in"><input id="nq-cod" placeholder="codice del pezzo"></span></label>
        <label class="qv"><span class="l">Lavorazione</span><span class="in"><select id="nq-p"><option value="">scegli</option>${opzProcessi("")}</select></span></label>
        <label class="qv"><span class="l">Lotti (scaglioni)</span><span class="in"><input id="nq-lotto" inputmode="numeric" placeholder="5/10/20"><b>pz</b></span></label>
        <label class="qv"><span class="l">Prezzi, uno per lotto</span><span class="in"><input id="nq-eur" inputmode="decimal" placeholder="30/22/18"><b>€/pz</b></span></label>
        <label class="qv"><span class="l">Materiale dichiarato</span><span class="in"><input id="nq-mat" placeholder="se il fornitore lo scrive"></span></label></div>
        <span><button type="button" class="ma-btn ma-btn--sm" data-of="qadd">${I("plus",13)} Aggiungi la quotazione</button></span></div></section></div>`;
}
/* ---- l'offerta al cliente: il documento SO ---- */
const NOTE_SO = ["Tutti i prezzi si intendono salvo aumenti di materia prima e operazioni aggiuntive;","Ci riserviamo di confermare il prezzo dopo l’avvio produzione;","Nulla ricevendo entro 3 gg dalla presente, tutte le condizioni si intendono accettate."];
function offertaDi(T){
  if(OFFERTE[T.key]) return OFFERTE[T.key];
  const O = {num:null, anno:2026, rev:0, data:null, emesse:[], stato:"bozza", lt1:"5", lt2:"3", pag:"RI.BA 60GG FM", resa:"franco nostro stabilimento", mat:"VENDITA", valid:"30", note:NOTE_SO.join("\n"), attn:"", escl:{}, storia:[], pagaAttr:{}, ltAttr:false};
  if(T.key==="r-pl1") Object.assign(O, {num:5478, data:"12/09/2026", stato:"inviata", storia:[["12/09/2026","emessa e inviata · SO 5478"]], emesse:[{rev:0, num:5478, anno:2026, data:"12/09/2026", ora:"", righe:[], cond:null}]});
  return OFFERTE[T.key] = O;
}
function righeOfferta(T){
  const out = [], S2 = a2Di(T);
  const ultime = {}; S2.consegne.forEach(f=>{ ultime[f.prodotto] = f; });
  for(const foto of Object.values(ultime)){
    const C0 = costoDi(T,foto), {P} = prodDiFoto(T,foto);
    if(foto.noquote){ out.push({k:foto.prodotto, nq:true, cod:foto.codice, desc:"non quotato: "+foto.motivo}); continue; }
    C0.scen.forEach((L,i)=>{ const t = totaliCosto(T,foto,C0,i); out.push({k:foto.prodotto, i:"L"+L, cod:foto.codice, desc:"OFFERTA · "+(P.nome||""), um:"NR", qta:L, prezzo:t.prezzo, costo:t.conAtt, mdc:t.mdcEff, obiettivo:mdcDi(C0), sotto: t.mdcEff!=null && t.mdcEff<mdcDi(C0)-0.0005}); });
    totaliCosto(T,foto,C0,0).esclusi.forEach(x=>out.push({k:foto.prodotto, escl:true, cod:x.r.cod, desc:"escluso dalla fornitura: "+(x.testo||"")}));
    const O = offertaDi(T);
    prepDi(T,foto).filter(x=>!x.esiste && O.pagaAttr[x.key]==="a parte").forEach(x=>out.push({k:foto.prodotto, i:"a"+x.key, cod:x.cod, desc:`ATTREZZATURA · ${x.nome} (una tantum)`, um:"NR", qta:1, prezzo:num(x.costo), costo:num(x.costo), mdc:0, attr:true}));
  }
  return out;
}
const CHIP_FASE_FERMA = `<span class="ma-st ma-st--neu ma-st--piatto" title="RV-05, EB7-3">la fase cambia solo con un gesto registrato: non ancora disponibile</span>`;
/* RL-29: i prezzi della SO con due decimali, come nel documento vero */
const prezzoSO = x => Number(x).toLocaleString("it-IT",{minimumFractionDigits:2,maximumFractionDigits:2});
/* RL-10: la nota di sostituzione, da rev 1 in poi, dice quale offerta annulla (numero, revisione e data) */
function notaSostituisce(O){ const e = [...O.emesse].reverse().find(x=>x.rev<O.rev && x.data); return O.rev && e ? `Annulla e sostituisce la SO ${e.num}/${e.anno}${e.rev?" rev "+e.rev:""} del ${e.data}.` : ""; }
/* il documento SO: quello in bozza (vivo dai costi) o una revisione emessa, fotografata e in sola lettura */
function cartaSO(cli, d, O, x){
  const so = x || {num:O.num, anno:O.anno, rev:O.rev, data:O.data, righe:null, cond:O};
  const cond = so.cond || O, note = (x ? (x.sostituisce?[x.sostituisce]:[]) : (notaSostituisce(O)?[notaSostituisce(O)]:[])).concat(String(cond.note||"").split("\n"));
  return `<div class="so-carta${x?" so-carta--rev":""}" aria-label="${x?`Revisione ${x.rev} emessa, sola lettura`:"Anteprima dell’offerta"}">
      <div class="so-h"><div><b class="so-ditta">PROMATEC S.r.l.</b><div class="k3">Sede amministrativa e stabilimento 1 · Via dell’Esempio 1 · 06000 Umbertide (PG)</div><div class="k3">Azienda certificata ISO 9001:2015</div></div>
        <div class="so-n"><span class="lab">Offerta${x?" · sola lettura":""}</span><b class="mono">N. ${so.num?so.num+" / "+so.anno:"— (bozza)"}${so.rev?" rev "+so.rev:""}</b><span class="k3" data-so-data="1">${so.data?"del "+esc(so.data)+(x&&x.ora?" "+esc(x.ora):""):"non ancora emessa"}</span></div></div>
      <div class="so-a"><div><span class="lab">Spett.le</span><b>${esc(cli.nome)}</b><div class="k3">${esc(cli.sede||"")}</div></div><div><span class="lab">Alla c/att.ne</span><div>${esc(cond.attn||(d?d.buyer:"")||"")}${!(cond.attn||(d&&d.buyer))?'<span class="manca-c">vuoto</span>':""}</div><span class="lab">Da</span><div title="RL-30: chi firma la SO (nel modello reale «ACQUISTI») · default, da confermare">Ufficio commerciale · Franco</div></div></div>
      <p class="so-p">Con la presente, sottoponiamo alla Vs. cortese attenzione la ns. migliore offerta per gli articoli sotto descritti.</p>
      <table class="tb so-t"><thead><tr><th>Cod. articolo</th><th>Descrizione</th><th>UM</th><th class="num">Quantità</th><th class="num">Prezzo unitario €</th></tr></thead><tbody>
      ${(so.righe||[]).map(r=>`<tr><td class="mono">${esc(r.cod)}</td><td>${esc(r.desc)}</td><td>${r.um}</td><td class="num mono">${fmt(r.qta,0)}</td><td class="num mono">${r.prezzo!=null?prezzoSO(r.prezzo):"da quotare"}</td></tr>`).join("")}`;
}
function vistaOfferta(T, foto){
  const O = offertaDi(T), rr = righeOfferta(T), q = T.rfq ? rf(T.rfq) : null, d = q ? datiRFQ(q) : null, cli = CLIENTI.find(c=>c.nome===T.testa.cliente) || {nome:T.testa.cliente, sede:""};
  const req = T.req ? rq(T.req) : null, bloccata = O.stato!=="bozza", {P} = prodDiFoto(T,foto), diretta = !req && P.target;
  const fld = (k,lab,ph,lungo) => `<label class="qv${lungo?" largo":""}"><span class="l">${lab}</span><span class="in"><input data-oc="${k}" value="${esc(O[k]||"")}" placeholder="${esc(ph||"")}" ${bloccata?"disabled":""}></span></label>`;
  /* RIPROVA (RL-10): emessa, l'offerta corrente si legge dalla sua fotografia: un prezzo cambiato dopo nei Costi non la tocca */
  const fotoCorr = bloccata ? [...O.emesse].reverse().find(x=>x.rev===O.rev && x.righe && x.cond) || null : null;
  const vRev = S.offRev!=null && O.emesse[S.offRev] ? O.emesse[S.offRev] : fotoCorr;
  const cambiata = fotoCorr && JSON.stringify(fotoCorr.righe.map(r=>[r.cod,r.qta,r.prezzo])) !== JSON.stringify(rr.filter(r=>!r.nq&&!r.escl&&!O.escl[r.k+"|"+r.i]).map(r=>[r.cod,r.qta,r.prezzo]));
  const doc = vRev ? `${cartaSO(cli, d, O, vRev)}</tbody><tfoot><tr><td></td><td colspan="4" class="k3">LT 1° ordine ${esc(vRev.cond.lt1)} settimane, ordini successivi ${esc(vRev.cond.lt2)} settimane</td></tr></tfoot></table>
      <dl class="so-c"><dt>Pagamento</dt><dd>${esc(vRev.cond.pag)}</dd><dt>Resa</dt><dd>${esc(vRev.cond.resa)}</dd><dt>Materiale in</dt><dd>${esc(vRev.cond.mat)}</dd><dt>Validità</dt><dd>${esc(vRev.cond.valid)} giorni</dd></dl>
      <div class="so-note">${(vRev.sostituisce?[vRev.sostituisce]:[]).concat(String(vRev.cond.note||"").split("\n")).map(x=>`<div>${esc(x)}</div>`).join("")}</div>
      <div class="so-f"><span class="k3">Firma cliente per accettazione</span><span class="so-linea"></span></div></div>`
    : `${cartaSO(cli, d, O, null).replace('<tbody>\n      ', '<tbody>')}
      ${rr.filter(r=>!r.nq&&!r.escl&&!O.escl[r.k+"|"+r.i]).map(r=>`<tr><td class="mono">${esc(r.cod)}</td><td>${esc(r.desc)}</td><td>${r.um}</td><td class="num mono">${fmt(r.qta,0)}</td><td class="num mono">${r.prezzo!=null?prezzoSO(r.prezzo):"da quotare"}</td></tr>`).join("")}
      <tr><td></td><td colspan="4" class="k3">LT 1° ordine ${esc(O.lt1)} settimane${!O.ltAttr&&settAttrezzature(T,foto)?` (+ attrezzature fino a ${settAttrezzature(T,foto)} settimane)`:""}, ordini successivi ${esc(O.lt2)} settimane${rr.some(r=>r.escl)?" · "+rr.filter(r=>r.escl).map(r=>esc(r.desc)).join(" · "):""}${rr.filter(r=>r.qta>1).length>1?" · ogni lotto ha il suo prezzo: i lotti più grandi ammortizzano attrezzaggi e attrezzature":""}</td></tr></tbody></table>
      <dl class="so-c"><dt>Pagamento</dt><dd>${esc(O.pag)}</dd><dt>Resa</dt><dd>${esc(O.resa)}</dd><dt>Materiale in</dt><dd>${esc(O.mat)}</dd><dt>Validità</dt><dd>${esc(O.valid)} giorni</dd></dl>
      <div class="so-note">${notaSostituisce(O)?`<div data-sostituisce="1"><b>${esc(notaSostituisce(O))}</b></div>`:""}${esc(O.note).split("\n").map(x=>`<div>${x}</div>`).join("")}</div>
      <div class="so-f"><span class="k3">Firma cliente per accettazione</span><span class="so-linea"></span></div></div>`;
  const tDir = diretta ? totaliCosto(T,foto,costoDi(T,foto),0) : null;
  /* v16 (SE §4.10): un primario per stato; il resto nel «⋯». «Crea una revisione» compare una volta sola. */
  const piu = (voci, su) => voci.length ? `<details class="piu${su?" piu--su":""}"><summary aria-label="Altre azioni dell'offerta" title="Altre azioni">⋯</summary><div class="piu-corpo" role="group" aria-label="Altre azioni">${voci.join("")}</div></details>` : "";
  const bRev = `<button type="button" class="ma-btn ma-btn--sm" data-of="rev">Crea una revisione</button>`;
  const az = diretta ? `<div class="esito-b ${tDir.manca?"no":(tDir.mdcEff||0)>=0.15?"ok":"no"}">${I("alert",16)}<div><b>Ordine con il prezzo del cliente.</b> L’ordine fissa ${fmtE(P.target)}: con i costi calcolati il margine è ${fmt((tDir.mdcEff||0)*100,1)} %${tDir.manca?` <b>ma i costi sono incompleti (${tDir.manca} ${tDir.manca===1?"voce mancante":"voci mancanti"}): il margine vero è più basso</b>`:""}.
       ${O.stato==="accettata"?" Ordine accettato e registrato nell'offerta.":`<div class="acts" style="margin-top:8px;display:flex;gap:8px;flex-wrap:wrap;align-items:center"><button type="button" class="ma-btn ma-btn--pri" data-of="accettaord">${I("check",14)} Accetta l’ordine</button>${piu([`<button type="button" class="ma-btn ma-btn--sm" data-of="rifiutaord">Rifiuta l’ordine</button>`])}</div>`} ${CHIP_FASE_FERMA}</div></div>`
    : `<div class="acts off-azioni">${{
      bozza:`<button type="button" class="ma-btn ma-btn--pri" data-of="emetti">${I("check",14)} ${O.num?`Emetti la revisione ${O.rev} di SO ${O.num}`:`Emetti l’offerta SO ${prossimaSO}/2026`}</button>`,
      emessa: req ? `<button type="button" class="ma-btn ma-btn--pri" data-of="mail">${I("mail",14)} Prepara la mail al cliente</button>${piu([`<button type="button" class="ma-btn ma-btn--sm" data-of="inviata">Segna come inviata</button>`, bRev])}`
        : `<button type="button" class="ma-btn ma-btn--pri" data-of="inviata">Segna come inviata</button>${piu([bRev])}`,
      /* RL-20: la mail preparata è una BOZZA in Outlook; «inviata» arriva dal sync (simulato), come per i terzisti */
      bozza_mail:`<span class="ma-st ma-st--neu">bozza in Outlook · ⧗ parte quando la mandi da lì</span> <button type="button" class="ma-btn ma-btn--pri" data-of="mailsync">Simula: la mail parte da Outlook (mockup)</button>${piu([`<button type="button" class="ma-btn ma-btn--sm" data-of="inviata">Segna come inviata</button>`, bRev])}`,
      inviata:`<details class="piu piu--esito"><summary class="ma-btn ma-btn--pri" aria-label="Esito del cliente">Esito del cliente ▾</summary><div class="piu-corpo" role="group" aria-label="Esito del cliente">
          <button type="button" class="ma-btn ma-btn--sm" data-of="accettata">${I("check",14)} Il cliente ha accettato</button>
          <span class="perso"><select id="of-perso" aria-label="Motivo"><option value="">motivo</option><option>prezzo fuori target</option><option>tempi di consegna</option><option>il cliente ha rinunciato</option><option>altro</option></select><button type="button" class="ma-btn ma-btn--sm" data-of="persa">Offerta persa</button></span>
          ${bRev}</div></details>`,
      accettata:`${C("ok","offerta accettata","check")}`, persa:`${C("bad","offerta persa")}`}[O.stato]} ${CHIP_FASE_FERMA}</div>`;
  /* le attrezzature (R-11): costo, settimane, come si paga. Le settimane propongono il LT del primo ordine ◇ */
  const pp = prepDi(T,foto), sett = settAttrezzature(T,foto);
  const attrBlocco = pp.length ? `<section class="dblock"><header><h3>Attrezzature</h3><span class="k3 piccolo">come si paga lo decidi tu: ammortizzata nel prezzo, a parte, oppure del cliente</span></header>
      <div class="wrap-tb scroll"><table class="tb"><thead><tr><th>Codice</th><th>Preparazione</th><th class="num">Costo</th><th class="num">Settimane</th><th>Come si paga</th></tr></thead><tbody>
      ${pp.map(x=>`<tr><td class="mono">${esc(x.cod)}</td><td>${esc(x.nome)}${x.esiste?` <span class="k3">esiste già${x.riferimento?" · "+esc(x.riferimento):""}</span>`:""}</td><td class="num mono">${x.esiste?"0 €":num(x.costo)!=null?fmtE(num(x.costo),0):`<span class="manca-c">da quotare</span>`}</td><td class="num mono">${x.esiste?"—":x.settimane?esc(x.settimane):"—"}</td>
        <td>${x.esiste?`<span class="k3">—</span>`:`<select data-opa="${esc(x.key)}" ${bloccata?"disabled":""} aria-label="Come si paga ${esc(x.nome)}">${["ammortizzata","a parte","del cliente"].map(m=>`<option ${(O.pagaAttr[x.key]||"ammortizzata")===m?"selected":""}>${m}</option>`).join("")}</select>`}</td></tr>`).join("")}</tbody></table></div>
      ${sett?`<p class="piccolo off-lt-attr">${O.ltAttr?`✓ LT del primo ordine con le attrezzature: ${esc(O.lt1)} settimane`:`<span class="pr">◇ LT 1° ordine: + attrezzature, fino a ${sett} settimane</span> <button type="button" class="ma-btn ma-btn--sm" data-of="ltattr" data-sett="${sett}" ${bloccata?"disabled":""}>Usa: ${(+O.lt1||0)+sett} settimane</button>`}</p>`:""}</section>` : "";
  return `<div class="off-v"><div class="off-g">
    <div class="off-l"><div class="dati-h"><div><span class="lab">Offerta al cliente</span><h2>${O.num?"SO "+O.num+" / "+O.anno+(O.rev?" rev "+O.rev:""):"Offerta in bozza"}</h2>
        <p class="k3 piccolo">Le righe vengono dai costi: un prezzo cambiato là cambia qui. ${bloccata?"Emessa: per cambiarla si crea una revisione, con la sua data.":""}</p>
        ${cambiata?`<p class="piccolo" data-off-cambiata="1"><span class="manca-c">costi cambiati dopo l'emissione</span> Il documento emesso resta com'era: per mandare i prezzi nuovi crea una revisione.</p>`:""}</div></div>
      ${az}
      <section class="dblock"><header><h3>Righe</h3></header><div class="wrap-tb scroll"><table class="tb"><thead><tr><th></th><th>Codice</th><th>Descrizione</th><th class="num">Q.tà</th><th class="num">Prezzo</th><th class="num">Costo</th><th class="num">Margine</th></tr></thead><tbody>
        ${rr.map(r=>{ const k = r.k+"|"+r.i; return r.nq||r.escl ? `<tr class="k3"><td></td><td class="mono">${esc(r.cod)}</td><td colspan="5">${esc(r.desc)}</td></tr>`
          : `<tr><td><input type="checkbox" data-oe="${k}" ${O.escl[k]?"":"checked"} ${bloccata?"disabled":""} aria-label="Includi la riga"></td><td class="mono">${esc(r.cod)}</td><td>${esc(r.desc)}</td><td class="num mono">${fmt(r.qta,0)}</td>
            <td class="num mono">${r.prezzo!=null?fmtE(r.prezzo):"—"}</td><td class="num mono">${r.costo!=null?fmtE(r.costo):"—"}</td><td class="num mono">${r.mdc!=null?`<span class="${r.sotto?"neg":""}" ${r.sotto?`data-sotto-obiettivo="1" title="sotto l'obiettivo del ${Math.round(r.obiettivo*100)} %"`:""}>${fmt(r.mdc*100,1)} %</span>${r.sotto?` <span class="manca-c">sotto il ${Math.round(r.obiettivo*100)} %</span>`:""}`:"—"}</td></tr>`; }).join("")}</tbody></table></div></section>
      ${attrBlocco}
      <section class="dblock"><header><h3>Condizioni</h3></header><div class="qgrid">
        ${fld("lt1","LT primo ordine (settimane)","5")}${fld("lt2","LT ordini successivi (settimane)","3")}${fld("pag","Pagamento","RI.BA 60GG FM",1)}
        ${fld("resa","Resa","franco nostro stabilimento",1)}${fld("mat","Materiale in","VENDITA")}${fld("valid","Validità (giorni)","30")}${fld("attn","Alla c/att.ne",d?d.buyer:"",1)}</div>
        <label class="fld"><span>Note</span><textarea data-oc="note" rows="3" ${bloccata?"disabled":""}>${esc(O.note)}</textarea></label></section>
      ${O.emesse.filter(x=>x.righe&&x.cond).length?`<section class="dblock" data-revisioni="1"><header><h3>Revisioni emesse</h3><span class="k3 piccolo">ognuna con la sua data · si aprono in sola lettura</span></header><ul class="cas-elenco">${O.emesse.map((x,i)=>x.righe&&x.cond?`<li>SO ${x.num}/${x.anno}${x.rev?" rev "+x.rev:" rev 0"} · del ${esc(x.data)} ${esc(x.ora||"")} · ${x.righe.length} ${x.righe.length===1?"riga":"righe"} (${x.righe.map(r=>fmt(r.qta,0)+" pz "+fmtE(r.prezzo)).join(", ")})${x.sostituisce?" · "+esc(x.sostituisce):""} <button type="button" class="ma-btn ma-btn--sm${S.offRev===i?" ma-btn--ghost":""}" data-of-rev="${i}" aria-pressed="${S.offRev===i}">${S.offRev===i?"← torna alla corrente":"Apri"}</button></li>`:"").join("")}</ul></section>`:""}
      ${O.storia.length?`<section class="dblock"><header><h3>Registro dell'offerta</h3><span class="sp"></span><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-pa-gesto="/mockup/tempo" data-pa-campi='{"giorni":"7"}' title="Nel vero passano i giorni: qui si sposta l'orologio del mockup">⧗ Passa una settimana (mockup)</button></header><ul class="cas-elenco">${O.storia.map(([dt,t])=>`<li>${esc(dt)} · ${esc(t)}</li>`).join("")}</ul></section>`:""}</div>
    <div class="off-r">${doc}</div></div></div>`;
}
/* i gesti dell'Offerta passano dal finto server B (rotta /rfq/{id}/offerta/{gesto}): prima il server, poi la vista */
function azioneOffertaB(T, a, g){
  const O = offertaDi(T), req = T.req ? rq(T.req) : null;
  OF.msg = null;
  if(a==="qadd"){ const cod = (g("cod")||"").trim(), Ls = String(g("lotto")||"").split(/[\/;\s]+/).filter(Boolean).map(x=>parseInt(x,10)), Es = String(g("eur")||"").split(/[\/;\s]+/).filter(Boolean).map(num), tz = g("terz"), pp = g("p");
    /* RL-04: più scaglioni in una quotazione: «5/10/20» e «30/22/18», un prezzo per lotto */
    if(!cod||!Ls.length||Ls.some(x=>!(x>0))||Es.length!==Ls.length||Es.some(x=>x==null)||!tz||!pp) OF.msg = {ok:false, t:"Per la quotazione servono terzista, codice, lavorazione, i lotti e un prezzo per ogni lotto (es. 5/10/20 e 30/22/18)."};
    else { QUOTAZIONI.unshift({id:"qn"+QUOTAZIONI.length, tk:T.key, terz:tz, cod, p:pp, lotti:Ls.map((l,i)=>[l,Es[i]]).sort((x,y)=>x[0]-y[0]), materiale:(g("mat")||"").trim(), data:B_OGGI, f:"reale", rif:"scritta da Franco"}); OF.msg = {ok:true, t:`Quotazione aggiunta per <span class="mono">${esc(cod)}</span>: ${Ls.length} ${Ls.length===1?"lotto":"scaglioni"}.`}; } }
  if(a==="ltattr"){ const n = parseInt(g("sett"),10)||0; O.lt1 = String((parseInt(O.lt1,10)||0)+n); O.ltAttr = true; O.storia.push([B_OGGI, `LT del primo ordine con le attrezzature: ${O.lt1} settimane`]); OF.msg = {ok:true, t:`LT del primo ordine: <b>${O.lt1} settimane</b>, attrezzature comprese.`}; }
  if(a==="emetti"){ if(!O.num) O.num = prossimaSO++; O.data = B_OGGI; O.stato = "emessa";
    /* RL-10: a «Emetti» la revisione si fotografa (numero, data e ora, righe, condizioni): le passate restano in sola lettura */
    const sost = notaSostituisce(O), righe = righeOfferta(T).filter(r=>!r.nq&&!r.escl&&!O.escl[r.k+"|"+r.i]).map(r=>({cod:r.cod, desc:r.desc, um:r.um, qta:r.qta, prezzo:r.prezzo}));
    O.emesse.push({rev:O.rev, num:O.num, anno:O.anno, data:O.data, ora:oraB(), righe, sostituisce:sost, cond:{lt1:O.lt1, lt2:O.lt2, pag:O.pag, resa:O.resa, mat:O.mat, valid:O.valid, attn:O.attn, note:O.note}});
    O.storia.push([O.data, "emessa · SO "+O.num+(O.rev?" rev "+O.rev:"")]);
    OF.msg = {ok:true, t:`<b>Offerta SO ${O.num}/${O.anno}${O.rev?" rev "+O.rev:""} emessa il ${O.data}.</b> ${req?"Ora la mail al cliente.":"Segnala quando è inviata."}`}; }
  if(a==="rev"){ O.rev++; O.stato = "bozza"; O.data = null; S.offRev = null; O.storia.push([B_OGGI, "aperta la revisione "+O.rev+" di SO "+O.num]); OF.msg = {ok:true, t:`Revisione ${O.rev} aperta: cambia righe e condizioni, poi emettila. Avrà la sua data.`}; }
  if(a==="mail" && req){ const nome = `SO ${O.num} ${O.anno}${O.rev?" rev "+O.rev:""}.pdf`;
    MESSAGGI.push({mid:"so"+O.num+"r"+O.rev, req:req.id, prod:req.prodotti.slice(), d:"oggi", t:"adesso", dir:"out", chi:"Franco", r:"commerciale · noi", sub:`Offerta SO ${O.num}/${O.anno} — ${req.titolo}`,
      txt:`<p>Buongiorno ${esc(O.attn||req.buyer||"")},</p><p>in allegato la nostra offerta <mark class="hl">SO ${O.num}/${O.anno}${O.rev?" rev "+O.rev:""}</mark> per ${req.prodotti.map(p=>`<mark class="hl">${esc(PROD[p].cod)}</mark>`).join(", ")}.</p><p>Tempi: primo ordine ${esc(O.lt1)} settimane, ordini successivi ${esc(O.lt2)} settimane. Restiamo a disposizione.</p>`,
      files:[F(nome,"pdf","92 KB")], meta:"bozza preparata dall’Offerta: controllala e inviala da Outlook"});
    O.stato = "bozza_mail"; O.storia.push([B_OGGI, "bozza della mail in Outlook"]);
    OF.msg = {ok:true, t:`<b>Bozza della mail pronta</b> nella conversazione di ${esc(T.testa.cliente)}, con l’offerta allegata: parte quando la mandi da Outlook. La fase registrata non cambia.`}; }
  if(a==="mailsync"){ if(O.stato!=="bozza_mail") OF.msg = {ok:false, t:"Niente è cambiato: non c'è una bozza della mail da mandare."};
    else { O.stato = "inviata"; O.storia.push([B_OGGI, "inviata al cliente (la mail è partita da Outlook: il sync l'ha trovata)"]); OF.msg = {ok:true, t:"(mockup) La mail è partita da Outlook: il sync l'ha trovata e l'offerta è «inviata». La fase registrata non cambia."}; } }
  if(a==="inviata"){ O.stato = "inviata"; O.storia.push([B_OGGI, "segnata come inviata"]); OF.msg = {ok:true, t:"Segnata come inviata nel registro dell'offerta. La fase registrata non cambia."}; }
  if(a==="accettata"){ O.stato = "accettata"; O.storia.push([B_OGGI, "accettata dal cliente"]); OF.msg = {ok:true, t:"<b>Offerta accettata</b> nel registro dell'offerta. La fase registrata non cambia: il passaggio è un gesto registrato, non ancora disponibile."}; }
  if(a==="persa"){ const mot = g("motivo") || "altro"; O.stato = "persa"; O.storia.push([B_OGGI, "persa: "+mot]); OF.msg = {ok:false, t:`<b>Offerta persa</b> (${esc(mot)}). Il motivo resta nel registro; la fase registrata non cambia.`}; }
  if(a==="accettaord"){ O.stato = "accettata"; O.storia.push([B_OGGI, "ordine del cliente accettato"]); OF.msg = {ok:true, t:"<b>Ordine accettato</b> nel registro dell'offerta. Il riesame dell'ordine e la distinta nel gestionale vengono dopo; la fase registrata non cambia."}; }
  if(a==="rifiutaord"){ O.stato = "persa"; O.storia.push([B_OGGI, "ordine del cliente rifiutato"]); OF.msg = {ok:false, t:"<b>Ordine rifiutato</b> nel registro dell'offerta. La fase registrata non cambia."}; }
  if(!OF.msg) return {esito:"no", testo:"Niente è cambiato: gesto dell'offerta non previsto."};
  const testo = String(OF.msg.t).replace(/<[^>]+>/g,"");
  return OF.msg.ok ? {esito:"ok", testo, quiet:true} : (a==="persa"||a==="rifiutaord" ? {esito:"ok", testo, quiet:true} : {esito:"no", testo, quiet:true});
}
/* i campi di Costi, Listini e Offerta: anche loro passano dal finto server B (rotta /rfq/{id}/costi/campo) */
function campoCostiB(T, g){
  const k = g("k"), id = g("id")||"", v = g("v"), chk = g("chk")==="1";
  if(k==="cs"){ const foto = fotoCorrente(T); if(!foto) return {esito:"no", testo:"Niente è cambiato: nessuna consegna da Lino."}; const [kk,ii] = id.split("|"), C0 = costoDi(T,foto);
    if(["vol","target","mdc","amm"].includes(kk)) C0[kk] = v; else if(kk==="incl") C0.incl[ii] = chk; else if(kk==="prezzo") C0.prezzo[ii] = v; else C0[kk][ii] = v; }
  else if(k==="ls"){ const [kk,ii] = id.split("|"), x = num(v);
    if(x==null) return {esito:"no", testo:"Niente è cambiato: scrivi un numero.", quiet:false};
    if(kk==="tar") tarDi(ii).eh = x; if(kk==="lasermin") LASER.eurMin = x; if(kk==="vel") LASER.vel[+ii][1] = x; if(kk==="mat") PREZZI_MAT[+ii].eur = x;
    if(kk==="comp") PREZZI_COMP[ii] = [x,"reale"]; if(kk==="op") COSTO_OP[ii].eur = x; if(kk==="ric") RIC[ii].v = x; }
  else if(k==="oc"){ offertaDi(T)[id] = v; }
  else if(k==="oe"){ offertaDi(T).escl[id] = !chk; }
  else if(k==="opa"){ if(!["ammortizzata","a parte","del cliente"].includes(v)) return {esito:"no", testo:"Niente è cambiato: come si paga non previsto."}; offertaDi(T).pagaAttr[id] = v; }
  else return {esito:"no", testo:"Niente è cambiato."};
  return {esito:"ok", testo:"Campo salvato.", quiet:true};
}
/* RL-10: aprire una revisione emessa è solo una lettura (nessun gesto) */
document.addEventListener("click", e=>{ const t = e.target.closest && e.target.closest("[data-of-rev]"); if(!t) return; const i = +t.dataset.ofRev; S.offRev = S.offRev===i ? null : i; render(); });
document.addEventListener("click", e=>{
  const t = e.target.closest && e.target.closest("[data-of]"); if(!t || t.disabled) return;
  const T = threadAperto(); if(!T) return;
  const a = t.dataset.of, campi = {};
  if(a==="qadd"){ campi.mat = ($("nq-mat")||{}).value||""; campi.cod = ($("nq-cod")||{}).value||""; campi.lotto = ($("nq-lotto")||{}).value||""; campi.eur = ($("nq-eur")||{}).value||""; campi.terz = ($("nq-terz")||{}).value||""; campi.p = ($("nq-p")||{}).value||""; }
  if(a==="persa") campi.motivo = ($("of-perso")||{}).value||"";
  if(a==="ltattr") campi.sett = t.dataset.sett||"";
  eseguiB(`/rfq/${T.id}/offerta/${a}`, campi);
});
document.addEventListener("change", e=>{
  const el = e.target, ds = el.dataset||{};
  if(!ds.cs && !ds.ls && !ds.oc && !ds.oe && !ds.opa) return;
  const T = threadAperto(); if(!T) return;
  const k = ds.cs!=null?"cs":ds.ls!=null?"ls":ds.oc!=null?"oc":ds.opa!=null?"opa":"oe";
  eseguiB(`/rfq/${T.id}/costi/campo`, {k, id: ds[k], v: el.value, chk: el.checked?"1":""});
});
