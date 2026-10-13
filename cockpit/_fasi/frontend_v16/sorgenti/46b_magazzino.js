/* ================= 46b · magazzino e storico della materia prima (dati di prova) =================
   SPEC_v15 §2.5.1 e piano v14 §3.2, §3.7. Sono le anagrafiche che nel programma vero stanno in GestX (articoli,
   giacenze, acquisti, consumi) e che il Cockpit LEGGE soltanto: qui sono dati di prova, con la data della fotografia.
   Nessun nome nuovo di fornitori: quelli di materia prima che v13 non conteneva sono «Fornitore MP 1…4» (RV-31).
   I prezzi con il chip mod99 sono valori di esempio: 0,74 €/kg lamiera, 0,83 €/kg UPN, 1,24 €/kg tubo.
   Lo STATO del grezzo (in magazzino, non basta, già acquistato, mai) NON si calcola qui: lo dice statoGrezzo() del
   finto server A2 (18_server_a2.js), come anteprima di sola lettura (R-37). */

const GIACENZA_AL = "08/10/2026 07:00";

/* grezzi commerciali: qualità + forma + misure + finitura + norma di prodotto; kg/m o kg/m² dai prontuari */
const GREZZI = [
 {id:"g-s235-3",   q:"S235JR",  forma:"lamiera",   sp:3,   formato:"3000 × 1500", fin:"decapata", norma:"EN 10025-2", kgm2:23.55, gest:"scorta"},
 {id:"g-s235-4",   q:"S235JR",  forma:"lamiera",   sp:4,   formato:"3000 × 1500", fin:"decapata", norma:"EN 10025-2", kgm2:31.40, gest:"scorta"},
 {id:"g-s235-8",   q:"S235JR",  forma:"lamiera",   sp:8,   formato:"3000 × 1500", fin:"nera",     norma:"EN 10025-2", kgm2:62.80, gest:"scorta"},
 {id:"g-s235-3.5", q:"S235JR",  forma:"lamiera",   sp:3.5, formato:"3000 × 1500", fin:"decapata", norma:"EN 10025-2", kgm2:27.48, gest:"commessa"},
 {id:"g-s355-15",  q:"S355J2",  forma:"lamiera",   sp:15,  formato:"3000 × 1500", fin:"nera",     norma:"EN 10025-2", kgm2:117.75, gest:"scorta"},
 {id:"g-s355-127", q:"S355J2",  forma:"tubo_tondo",D:127,  sp:10, barra:6000, norma:"EN 10210", kgm:28.85, gest:"commessa"},
 {id:"g-upn160",   q:"S355J2",  forma:"profilo",   prof:"UPN 160", barra:12000, norma:"EN 10025-2", kgm:18.8, gest:"commessa"},
 {id:"g-q35",      q:"S355J2H", forma:"tubo_rett", a:35, b:35, sp:2, barra:6000, norma:"EN 10219", kgm:2.03, gest:"scorta"},
 {id:"g-r70",      q:"S355J2H", forma:"tubo_rett", a:70, b:40, sp:4, barra:6000, norma:"EN 10219", kgm:6.24, gest:"commessa", nota:"raggiato"},
 {id:"g-t13",      q:"S235JRH", forma:"tubo_tondo",D:13.5, sp:2.3, barra:6000, norma:"EN 10219", kgm:0.64, gest:"commessa"},
 {id:"g-c40-42",   q:"C40",     forma:"tondo",     D:42, barra:3000, norma:"EN 10083-2", kgm:10.88, gest:"commessa"},
];

/* giacenze (fotografia di GestX): grezzo × stabilimento × colata; quantità inventate, costo medio dal Mod 99 dove c'è */
const GIACENZE = [
 {g:"g-s235-3",  qta:14, um:"lamiere", stab:"Stab. 1", cert:"2.2", colata:"24B302", impegnato:0, costoMedio:0.86, fonte:"inventato"},
 {g:"g-s235-4",  qta:6,  um:"lamiere", stab:"Stab. 2", cert:"2.2", colata:"24C117", impegnato:0, costoMedio:0.86, fonte:"inventato"},
 {g:"g-s235-8",  qta:2,  um:"lamiere", stab:"Stab. 1", cert:"2.2", colata:"25A044", impegnato:1, impegnatoPer:"RFQ 2026/301", costoMedio:0.90, fonte:"inventato"},
 {g:"g-s355-15", qta:2,  um:"lamiere", stab:"Stab. 1", cert:"3.1", colata:"23A118", impegnato:0, costoMedio:0.74, fonte:"mod99"},
 {g:"g-s355-15", qta:3,  um:"lamiere", stab:"Stab. 2", cert:"2.2", colata:"22K051", impegnato:0, costoMedio:0.78, fonte:"inventato"},
];
/* acquisti (GestX): l'ultima volta che l'abbiamo comprato */
const ACQUISTI = [
 {g:"g-upn160",  data:"03/2025", forn:"mp2", eurkg:0.83, lottoMin:"1 barra da 12 m", gg:10, commessa:"telaio skid VFR (2025)", fonte:"mod99"},
 {g:"g-q35",     data:"03/2026", forn:"mp1", eurkg:1.05, lottoMin:"10 barre", gg:7, commessa:"Telesol Orsini", fonte:"inventato"},
 {g:"g-s235-3.5",data:"11/2023", forn:"mp4", eurkg:0.86, lottoMin:"1 pacco", gg:12, commessa:"quotato per Cabine Omega, mai ordinato", fonte:"inventato", soloQuotato:true},
];
/* consumi (GestX, produzione): la prova che un grezzo è passato davvero da una lavorazione */
const CONSUMI = [
 {g:"g-s235-3",  data:"2026", kg:420, lav:["laser","piega","sald"], commessa:"ACME 9990707A1"},
 {g:"g-s235-4",  data:"2025", kg:180, lav:["laser","piega"], commessa:"Potaflex 92176C"},
 {g:"g-s355-15", data:"2025", kg:137, lav:["laser","sald"], commessa:"telaio skid VFR"},
 {g:"g-upn160",  data:"2025", kg:1134, lav:["sald"], commessa:"telaio skid VFR"},
 {g:"g-q35",     data:"2026", kg:96, lav:["sega","sald"], commessa:"Telesol Orsini"},
];
/* equivalenze: la grafia del cliente o del disegno → la qualità nostra; MAI un'alternativa al ribasso (MAT-05) */
const EQUIVALENZE = [
 {nome:"GSX9486 cat. II", cli:"Cabine Omega", q:"S355J2",   stato:"proposta",   da:"Lino (scheda 5718402)"},
 {nome:"GSX9486 cat. IA", cli:"Cabine Omega", q:"S235JR",   stato:"proposta",   da:"Lino (scheda 5718402)"},
 {nome:"S 355 J2 N",      cli:"Telesol", q:"S355J2+N", stato:"confermata", da:"qualità"},
 {nome:"S 235 J2 N",      cli:"Telesol", q:"S235J2+N", stato:"confermata", da:"qualità"},
];
/* fornitori di materia prima (anonimi: non sono nel mockup v13) e i fornitori di lamiera tagliata che c'erano */
const FORNITORI_MP = [
 {id:"mp1", nome:"Fornitore MP 1", forme:["tubo_tondo","tubo_rett","tondo"], cert:["3.1","2.2"], gg:7},
 {id:"mp2", nome:"Fornitore MP 2", forme:["profilo","piatto"], cert:["3.1","2.2"], gg:10},
 {id:"mp3", nome:"Fornitore MP 3", forme:["tubo_tondo","tubo_rett"], cert:["3.1"], gg:12},
 {id:"mp4", nome:"Fornitore MP 4", forme:["lamiera"], cert:["3.1","2.2"], gg:8},
];
const fornMP = id => FORNITORI_MP.find(f=>f.id===id) || {id, nome:id};

/* i terzisti: quelli di v13 (anagrafica dell'Inbox) con lavorazioni e qualifiche per cliente
   (nel backend: cliente_fornitore_lavorazione, 0014). Nessun nome nuovo rispetto a v13. */
(()=>{
  const add = {f3:{lavs:["zincatura","lavaggio_zinco"],qual:{acme:["zincatura"],tdl:["zincatura"]}},
    f2:{lavs:["tornitura","fresatura_est"],qual:{}},
    f1:{lavs:["verniciatura_polvere","verniciatura_liquido"],qual:{fitlab:["verniciatura_polvere"],tdl:["verniciatura_polvere"],potaflex:["verniciatura_polvere","verniciatura_liquido"]}}};
  TERZISTI.forEach(t=>Object.assign(t, add[t.id]||{lavs:[],qual:{}}));
  TERZISTI.push(
    {id:"f5",nome:"Sabbiatura Valle",sigla:"SB",lav:"sabbiatura, cataforesi e polveri",chat:[],lavs:["sabbiatura","cataforesi","verniciatura_polvere"],qual:{acme:["sabbiatura","cataforesi","verniciatura_polvere"],fitlab:["sabbiatura"],mietigamma:["sabbiatura","cataforesi"]}},
    {id:"f4",nome:"Termotrattamenti Centro",sigla:"TT",lav:"trattamenti termici",chat:[],lavs:["trattamento_termico"],qual:{}},
    {id:"f6",nome:"Curvatubi Marina",sigla:"CM",lav:"curvatura tubi oltre Ø80",chat:[],lavs:["curvatura_tubi"],qual:{}},
    {id:"lamitaglio",nome:"Lamitaglio",sigla:"LT",lav:"lamiera tagliata e sagomata, conto vendita",chat:[],lavs:["laser_est","trasporto"],qual:{}},
    {id:"rtc",nome:"RTC Travi",sigla:"RT",lav:"taglio e lavorazione di travi e profili",chat:[],lavs:["travi_est"],qual:{}},
    {id:"mrt",nome:"MRT Lavorazioni Meccaniche",sigla:"MR",lav:"tornitura e fresatura di precisione",chat:[],lavs:["tornitura","fresatura_est"],qual:{}},
    {id:"polvernova",nome:"Polvernova",sigla:"PN",lav:"sabbiatura, cataforesi e polveri",chat:[],lavs:["sabbiatura","cataforesi","verniciatura_polvere"],qual:{fitlab:["cataforesi","verniciatura_polvere"],vfr:["verniciatura_polvere"]}},
    {id:"tfr",nome:"T.F.R. Meccanica",sigla:"TF",lav:"tornitura e lavorazioni meccaniche",chat:[],lavs:["tornitura","fresatura_est"],qual:{}},
    {id:"colorama",nome:"Colorama",sigla:"CR",lav:"verniciatura a polvere",chat:[],lavs:["verniciatura_polvere"],qual:{vfr:["verniciatura_polvere"]}},
    /* v16 · fornitori di attrezzature (lavorazione 'attrezzature', già nel seme 0014 di Pietro): maschere, set di curvatura, utensili */
    {id:"robosald",nome:"Robosald-Gasitalia",sigla:"RG",lav:"maschere di saldatura e attrezzature",chat:[],lavs:["attrezzature"],prep:["maschera","piega","stampo","dima","altra"],qual:{}},
    {id:"minitool",nome:"Minitool",sigla:"MT",lav:"set di curvatura e utensili",chat:[],lavs:["attrezzature"],prep:["curva","piega","stampo","altra"],qual:{}},
    {id:"dalmi",nome:"Dalmi",sigla:"DL",lav:"set di curvatura (quota per Minitool)",chat:[],lavs:["attrezzature"],prep:["curva"],qual:{}});
})();
const terz = id => TERZISTI.find(t=>t.id===id) || FORNITORI_MP.find(f=>f.id===id) || {id, nome:id, lavs:[], qual:{}};
const nomeBreve = n => /^Fornitore MP/.test(n) ? n.replace("Fornitore ","") : n.split(" ")[0];

/* MINUTERIA non esenta più dal 2D (R103 C, SPEC §6.1): resta solo come famiglia del catalogo */
const FAM_MINUTERIA = MINUTERIA;

/* i componenti di catalogo saldabili: gruppo ISO/TR 15608 e spessore della zona saldata (inventati e segnati) */
Object.assign(compDi("d4")||{}, {gruppo:"1.1", zonaSaldata:12, zincato:true, datoInventato:true});
Object.assign(compDi("s1")||{}, {gruppo:"1.1", zonaSaldata:8, datoInventato:true});
Object.assign(compDi("s2")||{}, {gruppo:"1.1", zonaSaldata:10, datoInventato:true});
Object.assign(compDi("c1")||{}, {gruppo:"—", zonaSaldata:null, datoInventato:true});
