
/* ================= CATALOGHI DELLA DISTINTA =================
   Dati di prova del mockup: i materiali, le macchine e i componenti visti nelle RFQ reali (schede costo Mod 99,
   disegni Cabine Omega, Telesol, VFR) più le qualità più comuni a magazzino. Nel programma vero sono tabelle del backend
   che si aggiornano dall'anagrafica: qui servono per far provare la Distinta a Lino e Franco. */

/* ---- banca dati dei materiali: gruppo › famiglia › qualità ----
   «visto» = dove è comparso nelle RFQ reali; la densità serve al peso del grezzo (kg/dm³). */
const MAT_DB=[
 {g:"Acciai al carbonio da costruzione",fam:[
  {k:"S235",rho:7.85,q:[
   {k:"S235JR",s:"JR",norma:"EN 10025-2",nota:"il più usato su lamiere, piatti e profili",visto:["ACME","Telesol","Orsini"]},
   {k:"S235J0",s:"J0",norma:"EN 10025-2"},
   {k:"S235J2",s:"J2",norma:"EN 10025-2"},
   {k:"S235J2+N",s:"J2+N",norma:"EN 10025-2",nota:"a disegno Telesol è scritto «S 235 J2 N»",visto:["Telesol"]},
   {k:"S235JRH",s:"JRH",norma:"EN 10219",nota:"tubi saldati formati a freddo"}]},
  {k:"S275",rho:7.85,q:[
   {k:"S275JR",s:"JR",norma:"EN 10025-2"},{k:"S275J0",s:"J0",norma:"EN 10025-2"},{k:"S275J2",s:"J2",norma:"EN 10025-2"}]},
  {k:"S355",rho:7.85,q:[
   {k:"S355JR",s:"JR",norma:"EN 10025-2"},
   {k:"S355J0",s:"J0",norma:"EN 10025-2"},
   {k:"S355J2",s:"J2",norma:"EN 10025-2",nota:"telai e strutture saldate",visto:["VFR","Cabine Omega"]},
   {k:"S355J2+N",s:"J2+N",norma:"EN 10025-2",nota:"a disegno Telesol è scritto «S 355 J2 N»",visto:["Telesol"]},
   {k:"S355K2",s:"K2",norma:"EN 10025-2"},
   {k:"S355MC",s:"MC",norma:"EN 10149-2",nota:"alto limite per formatura a freddo"},
   {k:"S355J2H",s:"J2H",norma:"EN 10219",nota:"tubi saldati formati a freddo"}]},
  {k:"S420",rho:7.85,q:[{k:"S420MC",s:"MC",norma:"EN 10149-2"}]},
  {k:"S460",rho:7.85,q:[{k:"S460MC",s:"MC",norma:"EN 10149-2"}]},
  {k:"S500",rho:7.85,q:[{k:"S500MC",s:"MC",norma:"EN 10149-2"}]},
  {k:"S700",rho:7.85,q:[{k:"S700MC",s:"MC",norma:"EN 10149-2",nota:"Cabine Omega scrive «s700» nel modulo d’offerta",visto:["Cabine Omega"]}]}]},
 {g:"Lamiere per piega e stampaggio",fam:[
  {k:"DC",rho:7.85,q:[{k:"DC01",s:"DC01",norma:"EN 10130",nota:"laminato a freddo"},{k:"DC04",s:"DC04",norma:"EN 10130"}]},
  {k:"DD",rho:7.85,q:[{k:"DD11",s:"DD11",norma:"EN 10111",nota:"laminato a caldo decapato"},{k:"DD13",s:"DD13",norma:"EN 10111"}]},
  {k:"Zincate",rho:7.85,q:[{k:"DX51D+Z",s:"DX51D+Z",norma:"EN 10346"},{k:"S350GD+Z",s:"S350GD+Z",norma:"EN 10346"}]}]},
 {g:"Acciai da bonifica e automatici",fam:[
  {k:"C40",rho:7.85,q:[{k:"C40",s:"C40",norma:"EN 10083-2",nota:"perni e boccole torniti",visto:["FitLab"]}]},
  {k:"C45",rho:7.85,q:[{k:"C45",s:"C45",norma:"EN 10083-2"}]},
  {k:"42CrMo4",rho:7.85,q:[{k:"42CrMo4",s:"42CrMo4",norma:"EN 10083-3"}]},
  {k:"11SMnPb37",rho:7.85,q:[{k:"11SMnPb37",s:"11SMnPb37",norma:"EN 10087",nota:"automatico da tornitura"}]}]},
 {g:"Acciai inossidabili",fam:[
  {k:"AISI 304",rho:7.93,q:[{k:"AISI 304",s:"304 · 1.4301",norma:"EN 10088-2"},{k:"AISI 304L",s:"304L · 1.4307",norma:"EN 10088-2"}]},
  {k:"AISI 316",rho:7.98,q:[{k:"AISI 316L",s:"316L · 1.4404",norma:"EN 10088-2"}]},
  {k:"AISI 430",rho:7.70,q:[{k:"AISI 430",s:"430 · 1.4016",norma:"EN 10088-2"}]}]},
 {g:"Alluminio",fam:[
  {k:"5754",rho:2.66,q:[{k:"EN AW-5754 H111",s:"H111",norma:"EN 485-2",nota:"lamiere piegate, coperchi",visto:["Cabine Omega"]},{k:"EN AW-5754 H22",s:"H22",norma:"EN 485-2"}]},
  {k:"5083",rho:2.66,q:[{k:"EN AW-5083 H111",s:"H111",norma:"EN 485-2"}]},
  {k:"6082",rho:2.70,q:[{k:"EN AW-6082 T6",s:"T6",norma:"EN 755-2",nota:"profili e lavorati; dopo saldatura T4 → T6",visto:["Cabine Omega","FitLab"]}]},
  {k:"6060",rho:2.70,q:[{k:"EN AW-6060 T6",s:"T6",norma:"EN 755-2"}]}]}
];
const MAT_IDX={}; MAT_DB.forEach(g=>g.fam.forEach(f=>f.q.forEach(q=>{MAT_IDX[q.k]={g:g.g,f,q};})));
const famDi=k=>{for(const g of MAT_DB){const f=g.fam.find(x=>x.k===k); if(f) return f;} return null;};
const matDi=k=>MAT_IDX[k]||null;

/* ---- forme del grezzo e le misure che Lino scrive per ognuna ---- */
const FORME=[["lamiera","Lamiera"],["tubo_tondo","Tubo tondo"],["tubo_rett","Tubo quadro o rettangolare"],["profilo","Profilo UPN, HEB, IPE, L"],["tondo","Tondo pieno"],["piatto","Piatto"]];
const FORMA_CAMPI={
 lamiera:[["sp","Spessore","mm"],["larg","Grezzo: larghezza","mm"],["lung","Grezzo: lunghezza (passo)","mm"],["fig","Pezzi per grezzo","pz"]],
 tubo_tondo:[["D","Ø esterno","mm"],["sp","Spessore","mm"],["L","Lunghezza del pezzo","mm"]],
 tubo_rett:[["a","Lato a","mm"],["b","Lato b","mm"],["sp","Spessore","mm"],["L","Lunghezza del pezzo","mm"]],
 profilo:[["prof","Profilo",""],["L","Lunghezza","mm"]],
 tondo:[["D","Ø","mm"],["L","Lunghezza del pezzo","mm"]],
 piatto:[["b","Larghezza","mm"],["sp","Spessore","mm"],["L","Lunghezza del pezzo","mm"]]
};
/* i campi senza i quali il peso del grezzo non si calcola */
const FORMA_OBBL={lamiera:["sp","larg","lung"],tubo_tondo:["D","sp","L"],tubo_rett:["a","b","sp","L"],profilo:["prof","L"],tondo:["D","L"],piatto:["b","sp","L"]};
/* profili commerciali in acciaio: kg al metro dai prontuari */
const PROFILI=[["UPN 80",8.64],["UPN 100",10.6],["UPN 120",13.4],["UPN 140",16.0],["UPN 160",18.8],["UPN 200",25.3],
 ["HEB 100",20.4],["HEB 140",33.7],["HEB 180",51.2],["HEB 200",61.3],["IPE 120",10.4],["IPE 160",15.8],["IPE 200",22.4],
 ["L 30×3",1.36],["L 40×4",2.42],["L 50×5",3.77],["L 60×6",5.42]];
/* i numeri si scrivono all'italiana (0,72 · 1.134,5) o col punto (0.72): tutti e due valgono */
const num=v=>{if(v===null||v===undefined) return null; let s=String(v).trim(); if(!s) return null;
  if(s.includes(",")) s=s.replace(/\./g,"").replace(",","."); const x=parseFloat(s); return isFinite(x)?x:null;};
const fmt=(x,d=2)=>x===null||x===undefined||!isFinite(x)?"—":x.toLocaleString("it-IT",{minimumFractionDigits:0,maximumFractionDigits:d});
/* peso del grezzo, kg: volume (mm³) × densità (kg/dm³) / 10⁶ */
function pesoLordo(m){
  if(!m||!m.forma) return null;
  const rho=(famDi(m.fam)||{rho:7.85}).rho, g=k=>num(m[k]), P=Math.PI;
  switch(m.forma){
    case "lamiera": return g("sp")&&g("larg")&&g("lung")?g("sp")*g("larg")*g("lung")*rho/1e6/(g("fig")||1):null;
    case "tubo_tondo": {const D=g("D"),s=g("sp"),L=g("L"); return D&&s&&L?P/4*(D*D-(D-2*s)*(D-2*s))*L*rho/1e6:null;}
    case "tubo_rett": {const a=g("a"),b=g("b"),s=g("sp"),L=g("L"); return a&&b&&s&&L?(a*b-(a-2*s)*(b-2*s))*L*rho/1e6:null;}
    case "profilo": {const p=PROFILI.find(x=>x[0]===m.prof), L=g("L"); return p&&L?p[1]*L/1000:null;}
    case "tondo": {const D=g("D"),L=g("L"); return D&&L?P/4*D*D*L*rho/1e6:null;}
    case "piatto": {const b=g("b"),s=g("sp"),L=g("L"); return b&&s&&L?b*s*L*rho/1e6:null;}
  }
  return null;
}
function misureTesto(m){
  if(!m||!m.forma) return "";
  const v=k=>m[k]||"?";
  switch(m.forma){
    case "lamiera": return `lamiera sp. ${v("sp")} · grezzo ${v("larg")} × ${v("lung")}${num(m.fig)>1?" · "+m.fig+" pz per grezzo":""}`;
    case "tubo_tondo": return `tubo Ø${v("D")} × ${v("sp")} · L ${v("L")}`;
    case "tubo_rett": return `tubo ${v("a")} × ${v("b")} × ${v("sp")} · L ${v("L")}`;
    case "profilo": return `${v("prof")} · L ${v("L")}`;
    case "tondo": return `tondo Ø${v("D")} · L ${v("L")}`;
    case "piatto": return `piatto ${v("b")} × ${v("sp")} · L ${v("L")}`;
  }
  return "";
}

/* ---- centri di lavoro: le macchine del sito e della scheda costo, per lavorazione ---- */
const CENTRI={
 laser:[["lasertek3020","Lasertek 3020 · CO2 3 kW · fino a 8 mm",8],["lasl4","Laser L4 3015 · CO2 4 kW · fino a 10 mm",10],["fibra","Pressix Fibra 3015 · fibra 3 kW · fino a 12 mm",12],
  ["lasertek3050","Lasertek 3050 · CO2 5 kW · fino a 20 mm",20],["cutrix20","Cutrix Alfa · fibra 20 kW · fino a 50 mm",50]],
 lasertubo:[["cutrixtubo","Cutrix Alfa tubo · fibra 6 kW"]],
 sega:[["sega","Sega a nastro"]],
 piega:[["px50","Pressix P 50 · 50 t"],["px80","Pressix P 80 · 80 t"],["px100","Pressix P 100 · 100 t"],["px170","Pressix P 170 · 170 t"],["px220","Pressix P 220 · 220 t"],["idraulica","Pressa idraulica (scheda costo)"]],
 curva:[["curvix516","Curvix CV 516"],["tubix58","Tubix TB 58"],["tubix86","Tubix TB 86"],["curvix980","Curvix CV 980"]],
 stampa:[["p120","Pressa idraulica 120 t"],["p130","Pressa idraulica 130 t"],["p315","Pressa 315 t (scheda costo · da verificare)"],["p400","Pressa 400 t (scheda costo · da verificare)"]],
 foratura:[["trapano","Trapano a colonna"]],
 filettatura:[["trapano","Trapano e maschiatrice"]],
 smussatura:[["mola","Mola"],["trapano","Trapano con svasatore"]],
 molatura:[["mola","Mola"]],
 buratto:[["buratto","Buratto"]],
 fresa:[["fresix1050","Fresix FX 1050"],["fresix2010","Fresix FX 2010"]],
 puntatura:[["puntatrice","Puntatrice"]],
 sald_robot:[["robix","Cella robot Robix"],["robon","Cella robot Robon"],["robyu","Cella robot Robyu"]],
 sald_mag:[["mig","Saldatrice MIG/MAG"]],
 sald_tig:[["tig","Saldatrice TIG"]],
 prigionieri:[["stud","Saldatrice per prigionieri"]]
};
const nomeCentro=(p,k)=>((CENTRI[p]||[]).find(x=>x[0]===k)||[,""])[1];
/* il laser più piccolo che taglia quello spessore: una proposta, la sceglie Lino */
function laserPerSpessore(sp){ if(!sp) return null; const m=CENTRI.laser.find(x=>sp<=x[2]); return m?m[0]:null; }

/* ---- catalogo dei componenti acquistati: viteria, dadi, prigionieri, molle, cerniere… ----
   cod = codice di catalogo; cli = codice del cliente quando a disegno c'è il suo; file = scheda o STEP del fornitore */
const FAM_COMP=[["dadi","Dadi"],["dadisald","Dadi a saldare e rivettabili"],["viti","Viti"],["rondelle","Rondelle e clip"],["prigionieri","Prigionieri a saldare"],
 ["molle","Molle a gas"],["cerniere","Cerniere e chiusure"],["boccole","Boccole e spine"],["guarn","Guarnizioni e passacavi"],["altro","Altro"]];
const MINUTERIA=new Set(["dadi","dadisald","viti","rondelle","prigionieri"]);
const COMPONENTI=[
 {id:"d1",fam:"dadi",cod:"ISO 4032 M8-8 ZN",desc:"Dado esagonale M8 cl. 8 zincato",norma:"ISO 4032",fin:"cl. 8 · zincato",cli:[["acme","990679X1"]],forn:"",file:["990679X_1.STP"],usato:["9990708A1"]},
 {id:"d2",fam:"dadi",cod:"SDPR M6",desc:"Dado SDPR M6",norma:"capitolato ACME",fin:"",cli:[["acme","939552X1"]],forn:"",file:["939552X_1.STP"],usato:["9990708A1"]},
 {id:"d3",fam:"dadi",cod:"SDPR M5",desc:"Dado SDPR M5",norma:"capitolato ACME",fin:"",cli:[["acme","9443580X1"]],forn:"",file:["9443580X_1.STP"],usato:["9990708A1"]},
 {id:"d4",fam:"dadi",cod:"ISO 4032 M14-8 ZN",desc:"Dado esagonale M14 cl. 8 zincato",norma:"ISO 4032",fin:"cl. 8 · zincato",cli:[],forn:"",file:[],usato:["F+800.514263.000"]},
 {id:"d5",fam:"dadi",cod:"ISO 7040 M8 ZN",desc:"Dado autobloccante M8 con anello in nylon",norma:"ISO 7040",fin:"cl. 8 · zincato",cli:[],forn:"",file:[],usato:[]},
 {id:"d6",fam:"dadi",cod:"NYLOCK FLG 5/16-18",desc:"Dado flangiato autobloccante 5/16-18",norma:"UNC",fin:"zincato",cli:[["cabineomega","5709381"]],forn:"",file:[],usato:[]},
 {id:"s1",fam:"dadisald",cod:"DIN 929 M8",desc:"Dado esagonale a saldare M8",norma:"DIN 929",fin:"grezzo",cli:[],forn:"",file:["DIN929_M8.pdf"],usato:[]},
 {id:"s2",fam:"dadisald",cod:"DIN 929 M10",desc:"Dado esagonale a saldare M10",norma:"DIN 929",fin:"grezzo",cli:[],forn:"",file:["DIN929_M10.pdf"],usato:[]},
 {id:"s3",fam:"dadisald",cod:"RIV M6 ZN",desc:"Dado rivettabile M6, testa piana",norma:"—",fin:"acciaio zincato",cli:[],forn:"",file:[],usato:[]},
 {id:"s4",fam:"dadisald",cod:"RIV M8 ZN",desc:"Dado rivettabile M8, testa piana",norma:"—",fin:"acciaio zincato",cli:[],forn:"",file:[],usato:[]},
 {id:"v1",fam:"viti",cod:"ISO 4017 M8×20-8.8 ZN",desc:"Vite testa esagonale M8 × 20",norma:"ISO 4017",fin:"8.8 · zincata",cli:[],forn:"",file:[],usato:[]},
 {id:"v2",fam:"viti",cod:"ISO 4762 M6×16-8.8",desc:"Vite a testa cilindrica con esagono incassato M6 × 16",norma:"ISO 4762",fin:"8.8 · brunita",cli:[],forn:"",file:[],usato:[]},
 {id:"v3",fam:"viti",cod:"HHF 1/4-20 × .5",desc:"Vite testa esagonale flangiata 1/4-20 × 0,5\"",norma:"UNC",fin:"gr. 8 · zincata",cli:[["cabineomega","362472"]],forn:"",file:[],usato:[]},
 {id:"r1",fam:"rondelle",cod:"ISO 7089 M8 ZN",desc:"Rondella piana M8",norma:"ISO 7089",fin:"zincata",cli:[],forn:"",file:[],usato:[]},
 {id:"r2",fam:"rondelle",cod:"USS 1/4",desc:"Rondella piana USS 1/4",norma:"USS",fin:"zincata",cli:[["cabineomega","362508"]],forn:"",file:[],usato:[]},
 {id:"r3",fam:"rondelle",cod:"SPRING CLIP 1/4-20",desc:"Clip a molla 1/4-20",norma:"—",fin:"zincata",cli:[["cabineomega","27385"]],forn:"",file:[],usato:[]},
 {id:"p1",fam:"prigionieri",cod:"ISO 13918 M8×20",desc:"Prigioniero a saldare M8 × 20",norma:"ISO 13918",fin:"acciaio ramato",cli:[],forn:"",file:[],usato:[]},
 {id:"m1",fam:"molle",cod:"GS 300N C100",desc:"Molla a gas 300 N, corsa 100 mm",norma:"—",fin:"",cli:[],forn:"",file:["molla_gas_300N.pdf"],usato:[]},
 {id:"c1",fam:"cerniere",cod:"48170",desc:"Cerniera (codice Cabine Omega)",norma:"—",fin:"",cli:[["cabineomega","48170"]],forn:"",file:[],usato:[],nota:"in un’offerta Cabine Omega era «escluse cerniere»: le fornisce il cliente"},
 {id:"c2",fam:"cerniere",cod:"MOSCH 1/4 × 2.375",desc:"Moschettone 1/4 × 2,375\", 170 lb",norma:"—",fin:"",cli:[["cabineomega","5762915"]],forn:"",file:[],usato:[]},
 {id:"b1",fam:"boccole",cod:"ISO 8752 6×30",desc:"Spina elastica Ø6 × 30",norma:"ISO 8752",fin:"",cli:[],forn:"",file:[],usato:[]},
 {id:"g1",fam:"guarn",cod:"GUARN-EPDM",desc:"Guarnizione in EPDM a profilo",norma:"—",fin:"",cli:[],forn:"",file:[],usato:[],nota:"in un’offerta Cabine Omega era «esclusa guarnizione»"}
];
const compDi=id=>COMPONENTI.find(c=>c.id===id);
const codiceComp=(it,cli)=>{const x=(it.cli||[]).find(([c])=>c===cli); return x?x[1]:it.cod;};
