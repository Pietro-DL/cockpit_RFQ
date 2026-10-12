/* ================= 46c · capacità dell'officina e catalogo delle lavorazioni (dati inventati e segnati) =================
   SPEC_v15 §2.5, piano v14 §3.6. Le soglie che in v13 erano scritte nel codice (laser fino a 20 mm, piega sp > 20 = KO,
   formato 3000 × 1500, «Ø > 80») diventano RIGHE di tabella con il chip «inventato»: servono a far vedere la forma a
   Lino, non a decidere. Una riga senza il dato vero dell'officina dà «da chiedere all'officina» (AV), MAI un KO
   inventato (regola di onestà, UI-20). Chi risponde alle domande (ci sta? sappiamo farlo?) è capacitaMateriale() nel
   finto server A2. */

/* le macchine, dai CENTRI di v13; lo stabilimento è da chiedere: «Stab. ?» */
const MACCHINE = Object.entries(CENTRI).flatMap(([lav, l]) => l.map(([k, n, sp]) => ({k, lav, nome:n, spMax:sp||null, stab:"Stab. ?"})));
const macchinaDi = k => MACCHINE.find(m=>m.k===k) || null;
/* limiti di taglio per gruppo di materiale: per l'acciaio i limiti del sito; inox e alluminio «da chiedere» */
const LIMITI_TAGLIO = [
 {lav:"laser", gruppo:"acciaio", spMax:50, nota:"Cutrix Alfa 20 kW fino a 50 mm (dal sito)"},
 {lav:"laser", gruppo:"inox", spMax:null, nota:"da chiedere all'officina"},
 {lav:"laser", gruppo:"alluminio", spMax:null, nota:"da chiedere all'officina"},
 {lav:"laser", gruppo:"formato", dim:"3000 × 1500", nota:"formato della lamiera su tutte le macchine: da confermare"},
 {lav:"lasertubo", gruppo:"sezione", dMin:20, dMax:220, barra:"3–6,5 m", nota:"Cutrix Alfa tubo"},
 {lav:"sega", gruppo:"sezione", dMax:300, a45:200, nota:"sezione massima a 90° e a 45°"},
];
/* utensili di piega e set di curvatura */
const UTENSILI_PIEGA = [
 {pressa:"px220", v:"V24", punzone:"R4", sp:[2,4], lunghezza:3340, nota:"P 220 · V24 · R4 per sp. 3"},
 {pressa:"px50",  v:"V12", punzone:"R1", sp:[1,3], lunghezza:1250, nota:"P 50 · V12"},
];
const SET_CURVA = [
 {d:33.7, sp:3.2, r:100, macchina:"curvix516", stato:"da fare", nota:"set per Ø33,7 × 3,2 · R100: da quotare al fornitore di attrezzature"},
];
/* WPS e qualifiche: copertura, non uguaglianza (v14 §4.4). Nessun nome di saldatore: solo i conteggi. */
const WPS = [
 {id:"135-012", proc:"135", gruppo:["1.1","1.2"], sp:[3,24], giunti:["FW"], pos:["PA","PB"], norma:"ISO 3834-2"},
 {id:"135-020", proc:"135", gruppo:["1.1","1.2"], sp:[8,30], giunti:["FW","BW"], pos:["PA","PB","PF"], norma:"EN 15085-2 CL1"},
];
const QUALIFICHE = [
 {wps:"135-012", n:3, scad:"03/2027", norma:"ISO 9606-1"},
 {wps:"135-020", n:2, scad:"11/2026", norma:"ISO 9606-1 · EN 15085-2"},
];
/* capacità dei terzisti: vasche e bilancelle */
const TERZISTI_CAP = [
 {terz:"f3", cosa:"vasca dello zinco 7 × 1,5 × 2,5 m", pesoMax:"2.000 kg"},
 {terz:"polvernova", cosa:"bilancella 2,5 × 1,8 m", pesoMax:"400 kg"},
];
/* v16: lo STORICO delle preparazioni (lettura 1 di PREPARAZIONI §5.4): «c'era nella RFQ del …» per codice e revisione.
   Nel backend è una query sulle preparazioni delle Distinte confermate, non una tabella a parte. Dati di prova. */
const MASCHERE = [ {cod:"92176C", rev:"", k:"maschera", rif:"92176C (comodato del cliente)", nota:"maschera 92176C (Potaflex), in comodato del cliente", rfq:"P-2026-0071 · 05/2026"} ];
const storicoPrep = (cod, k) => MASCHERE.find(x=>x.cod===cod && (x.k||"maschera")===k) || null;

/* ---------------- il catalogo delle lavorazioni (da v13) e i campi di ogni famiglia (SPEC §2.5.2) ----------------
   p → [nome, int|est, gruppo]. I campi: [chiave, etichetta, unità, ★ serve a Franco, tipo n|t|s, opzioni]. */
const LAVORAZIONI = [
 ["taglio","Taglio",[["laser","Taglio laser lamiera","int"],["laser_est","Lamiera tagliata comprata","est"],["lasertubo","Taglio laser tubo","int"],["sega","Taglio a sega","int"],["travi_est","Travi da terzi","est"]]],
 ["forma","Piegatura e formatura",[["piega","Piegatura","int"],["curva","Curvatura tubi","int"],["stampa","Stampaggio","int"],["curvatura_tubi","Curvatura tubi da terzi","est"]]],
 ["mecc","Lavorazioni meccaniche",[["foratura","Foratura e filettatura","int"],["filettatura","Filettatura","int"],["smussatura","Smussatura","int"],["sbavatura","Sbavatura","int"],["fresa","Fresatura CNC","int"],["fresatura_est","Fresatura da terzi","est"],["tornitura","Tornitura da terzi","est"],["rilavorazione","Rilavorazione","int"]]],
 ["sald","Saldatura",[["sald_mag","Saldatura MIG/MAG","int"],["sald_robot","Saldatura robot","int"],["sald_tig","Saldatura TIG","int"],["prigionieri","Dadi e prigionieri saldati","int"]]],
 ["ass","Assemblaggio",[["montaggio","Montaggio con viteria","int"],["piantaggio","Piantaggio a pressa","int"],["inserti","Insertaggio","int"]]],
 ["sup","Trattamenti",[["sabbiatura","Sabbiatura","est"],["cataforesi","Cataforesi","est"],["verniciatura_polvere","Verniciatura a polvere","est"],["zincatura","Zincatura","est"],["trattamento_termico","Trattamento termico","est"]]],
 ["fin","Controllo e documenti",[["raddrizzatura","Raddrizzatura","int"],["controllo","Controllo dimensionale","int"],["certificati","Certificati","int"],["campionatura","Campionatura (PPAP)","int"]]],
 ["log","Imballo e trasporto",[["imballo","Imballo","int"],["trasporto","Trasporto","int"]]],
];
const PROC = {}; LAVORAZIONI.forEach(([g,gn,l])=>l.forEach(([k,n,t])=>{ PROC[k]=[n,t,g,gn]; }));
const SALD = new Set(["sald_mag","sald_robot","sald_tig"]);
const TRATT = new Set(["sabbiatura","cataforesi","verniciatura_polvere","zincatura"]);
const CON_GREZZO = new Set(["laser","lasertubo","sega","travi_est","fresa"]);           /* la prima fase che consuma il grezzo nostro */
const T_ = "tempo";                                                                         /* il campo tempo, unico */
const CAMPI_FAM = {
 laser:      [["rett","Rettangolo","mm",1,"t"],["m","Metri di taglio","m",1,"n"],["fori","Fori (sfondamenti)","n°",1,"n"],["macc","Macchina","",0,"s","laser"],[T_,"Tempo","",0,"t"]],
 laser_est:  [["terzi","Fornitori (2–3)","",1,"terzi"],["lotto","Lotti","pz",1,"t"],["conto","Conto","",0,"s",["conto vendita","conto lavoro"]],["cert","Certificato","",0,"t"]],
 lasertubo:  [["L","Lunghezza del pezzo","mm",1,"n"],["tagli","Tagli","n°",1,"n"],["fori","Fori e asole","n°",0,"n"],[T_,"Tempo","",0,"t"]],
 sega:       [["L","Lunghezza del pezzo","mm",1,"n"],["tagli","Tagli","n°",1,"n"],[T_,"Tempo","",0,"t"]],
 travi_est:  [["barre","Barre n × L","",1,"t"],["terzi","Terzisti (2–3)","",1,"terzi"],["lav","Lavorazioni","",0,"t"],["conto","Conto","",0,"s",["conto lavoro","conto vendita"]]],
 piega:      [["pieghe","Pieghe","n°",1,"n"],["lmax","Piega più lunga","mm",1,"n"],["op","Operatori","",0,"s",["1","2"]],["macc","Pressa","",0,"s","piega"],[T_,"Tempo","",1,"t"]],
 curva:      [["curve","Curve","n°",1,"n"],["r","Raggio","mm",1,"n"],[T_,"Tempo","",0,"t"]],
 stampa:     [["colpi","Colpi","n°",1,"n"],[T_,"Tempo","",0,"t"]],
 curvatura_tubi:[["terzi","Terzisti (2–3)","",1,"terzi"],["curve","Curve","n°",0,"n"]],
 foratura:   [["n","Fori","n°",1,"n"],["mis","Misura","",0,"t"],[T_,"Tempo","",0,"t"]],
 filettatura:[["n","Filetti","n°",1,"n"],["mis","Misura","",0,"t"],[T_,"Tempo","",0,"t"]],
 smussatura: [["n","Smussi","n°",1,"n"],["mis","Misura","",0,"t"],[T_,"Tempo","",0,"t"]],
 sbavatura:  [[T_,"Tempo","",0,"t"]],
 fresa:      [["ops","Operazioni","",1,"t"],[T_,"Tempo","",0,"t"]],
 fresatura_est:[["terzi","Terzisti (2–3)","",1,"terzi"],["lotti","Lotti","pz",1,"t"],["conto","Conto","",0,"s",["conto vendita","conto lavoro"]],["mat","Materiale del disegno","",0,"t"]],
 tornitura:  [["terzi","Terzisti (2–3)","",1,"terzi"],["lotti","Lotti","pz",1,"t"],["conto","Conto","",0,"s",["conto vendita","conto lavoro"]],["mat","Materiale del disegno","",0,"t"]],
 rilavorazione:[["cosa","Che cosa","",1,"t"],["pz","Pezzi","pz",1,"n"]],
 sald_mag:   [[T_,"Tempo","",1,"t"],["norma","Norma","",0,"t"],["wps","WPS","",0,"t"]],
 sald_robot: [[T_,"Tempo","",1,"t"],["norma","Norma","",0,"t"],["wps","WPS","",0,"t"]],
 sald_tig:   [[T_,"Tempo","",1,"t"],["norma","Norma","",0,"t"],["wps","WPS","",0,"t"]],
 prigionieri:[["n","Dadi o prigionieri","n°",1,"n"],["mis","Misura","",1,"t"],[T_,"Tempo","",0,"t"]],
 montaggio:  [["viti","Viti","n°",0,"n"],[T_,"Tempo","",1,"t"]],
 piantaggio: [["n","Pezzi piantati","n°",1,"n"],[T_,"Tempo","",0,"t"]],
 inserti:    [["n","Inserti","n°",1,"n"],[T_,"Tempo","",0,"t"]],
 sabbiatura: [["terzi","Terzisti (2–3)","",1,"terzi"],["spec","Specifica","",0,"t"],["masch","Mascherature","",0,"t"]],
 cataforesi: [["terzi","Terzisti (2–3)","",1,"terzi"],["spec","Specifica","",0,"t"],["masch","Mascherature","",0,"t"]],
 verniciatura_polvere:[["terzi","Verniciatori (2–3)","",1,"terzi"],["col","Colore","",1,"t"],["spec","Specifica","",0,"t"],["masch","Mascherature","",0,"t"]],
 zincatura:  [["terzi","Terzisti (2–3)","",1,"terzi"],["spec","Specifica","",1,"t"],["masch","Mascherature","",0,"t"]],
 trattamento_termico:[["terzi","Terzisti (2–3)","",1,"terzi"],["ciclo","Ciclo","",1,"t"]],
 raddrizzatura:[[T_,"Tempo","",0,"t"]],
 controllo:  [["tipo","Tipo","",1,"s",["ISO 13920 BE","dimensionale a calibro","scanner 3D Scan3X"]],["freq","Frequenza","",1,"s",["primo pezzo","a campione","100 %"]]],
 certificati:[["tipo","Certificato","",1,"s",["3.1 EN 10204","2.2 EN 10204","3.2 EN 10204"]],["per","Per quali materiali","",0,"t"]],
 campionatura:[["liv","PPAP","",1,"s",["livello 1","livello 2","livello 3"]],["pz","Pezzi campione","pz",0,"n"]],
 imballo:    [["tipo","Imballo","",1,"s",["a vista","bancale","gabbia","cassa"]],["pz","Pezzi per imballo","pz",0,"n"]],
 trasporto:  [["da","Da","",1,"t"],["a","A","",1,"t"],["mezzo","Mezzo","",0,"t"]],
};
/* dopo una fase, quelle che di solito vengono: scorciatoie, non regole */
const SEGUE = {laser:["piega","foratura","sbavatura"],laser_est:["trasporto","foratura","piega"],lasertubo:["curva","foratura","sbavatura"],sega:["foratura","smussatura","curva"],
 piega:["sbavatura","filettatura","prigionieri"],foratura:["filettatura","smussatura","sbavatura"],travi_est:["sabbiatura","controllo","trasporto"],
 prigionieri:["sald_mag","controllo","sabbiatura"],sald_mag:["raddrizzatura","controllo","sabbiatura"],sald_robot:["controllo","sabbiatura","zincatura"],
 sabbiatura:["verniciatura_polvere","cataforesi","controllo"],verniciatura_polvere:["controllo","montaggio","imballo"],zincatura:["controllo","montaggio","imballo"],
 controllo:["certificati","imballo","trasporto"],certificati:["imballo","trasporto"],tornitura:["zincatura","controllo"],fresatura_est:["zincatura","controllo"],filettatura:["sbavatura","controllo"],smussatura:["sbavatura","controllo"]};
/* le preparazioni da realizzare: una tantum, in testa al ciclo, FUORI dalla sequenza delle fasi (decisione 07/10) */
/* v16 (PREPARAZIONI.md §2-§3): 8 tipi + «altra». Per ogni tipo: a chi si chiede (tipo di richiesta del mockup) e la frase
   precompilata. «Interno» = messaggio interno (officina, qualità); «Fornitore» = richiesta_fornitore con lavorazione
   'attrezzature' (c'è già nel backend, migrazioni 0014-0015). Una preparazione NON è la «preparazione» del Fascicolo. */
const PREP_TIPI = [["maschera","Maschera di saldatura"],["robot","Programma del robot"],["piega","Attrezzatura di piega"],["curva","Set di curvatura"],
 ["stampo","Stampo"],["dima","Dima di foratura"],["calibro","Calibro di controllo"],["prova","Prova di taglio"],["altra","Altra (nome libero)"]];
const PREP_INFO = {
 maschera:{rich:["officina","attrezzatura"], chi:"attrezzeria interna o Robosald-Gasitalia", frase:"Ci serve una maschera di saldatura per il codice {cod}{rev}: riferimenti {rif}, {pezzi} pezzi l'anno. Ci mandate prezzo e tempi? In allegato PDF e 3D."},
 robot:{rich:["officina"], chi:"capo officina (programmatore della cella)", frase:"Programma del robot per {cod}: {cordoni}. Ore di programmazione e quando sarebbe pronto?"},
 piega:{rich:["attrezzatura"], chi:"fornitori di attrezzature", frase:"Attrezzatura di piega per {cod}: {dato}. Prezzo e tempi di consegna?"},
 curva:{rich:["attrezzatura"], chi:"Minitool / Dalmi, con i 3D", frase:"Set di curvatura per {cod}: {dato}. Prezzo e tempi di consegna? In allegato i 3D dei tubi."},
 stampo:{rich:["attrezzatura"], chi:"fornitori di attrezzature", frase:"Stampo per {cod}{rev}: {dato}. Prezzo e tempi?"},
 dima:{rich:["officina"], chi:"attrezzeria interna", frase:"Dima di foratura per {cod}: la facciamo noi? Ore e costo stimati, e quando sarebbe pronta?"},
 calibro:{rich:["qualita"], chi:"qualità", frase:"Calibro di controllo per {cod}: {dato}. Costo e tempi?"},
 prova:{rich:["officina"], chi:"officina laser", frase:"Prova di taglio per {cod}: {dato}. Quante ore macchina, e quando?"},
 altra:{rich:["officina","attrezzatura"], chi:"scelto da chi chiede", frase:"{nome} per {cod}: costo e tempi?"},
};
const nomePrep = k => (PREP_TIPI.find(x=>x[0]===String(k||"").split("@")[0])||[,k])[1];
/* le fasi in cui entrano i figli: unioni (saldatura, dadi saldati) e montaggi. Più unioni nello stesso ciclo = pre-unioni in fila (R-10) */
const UNIONE = new Set(["sald_mag","sald_robot","sald_tig","prigionieri","montaggio","piantaggio","inserti"]);
/* le sigle delle fasi nell'albero del processo (SPEC_v16 §4.3): il nome intero va nel title */
const SIGLE = {laser:"L",laser_est:"L",lasertubo:"LT",sega:"S",travi_est:"Tr",piega:"P",curva:"Cu",stampa:"St",curvatura_tubi:"Cu",foratura:"F",filettatura:"Fi",smussatura:"Sm",sbavatura:"Sb",
 fresa:"Fr",fresatura_est:"Fr",tornitura:"To",rilavorazione:"R",sald_mag:"Sa",sald_robot:"SaR",sald_tig:"TIG",prigionieri:"Pr",montaggio:"M",piantaggio:"Pi",inserti:"In",
 sabbiatura:"Sab",cataforesi:"KTL",verniciatura_polvere:"Vp",zincatura:"Zn",trattamento_termico:"TT",raddrizzatura:"Rd",controllo:"Co",certificati:"Ce",campionatura:"PPAP",imballo:"Im",trasporto:"Tp"};
const siglaDi = p => SIGLE[p] || String(p||"").slice(0,2);

/* i modelli di ciclo per famiglia (PRO-06, da validare con Lino): fasi proposte, MAI numeri inventati */
const MODELLI = {
 T1:  {nome:"Lamiera laser + piega", fasi:["laser","piega","sbavatura"]},
 T1b: {nome:"Lamiera laser + fori", fasi:["laser","filettatura"]},
 T2:  {nome:"Saldato", fasi:["sald_mag","controllo","imballo"]},
 T3:  {nome:"Tubo a sega", fasi:["sega"]},                 /* RL-12: solo la sega; la foratura si aggiunge se il disegno la chiede («di solito dopo») */
 T4:  {nome:"Tornito fuori", fasi:["tornitura"]},
 T4c: {nome:"Fresato fuori", fasi:["fresatura_est"]},
 T4b: {nome:"Travi da terzi", fasi:["travi_est"]},
 T5:  {nome:"Lamiera tagliata comprata", fasi:["laser_est"]},
 T6:  {nome:"Commerciale", fasi:[]},
 T7:  {nome:"Dal cliente", fasi:[]},
 T8:  {nome:"Escluso", fasi:[]},
 T9:  {nome:"Rilavorazione", fasi:["rilavorazione"]},
};
/* come si ottiene: i sei tasti di «Come» */
const MODI = [
 ["noi","noi","lo facciamo noi"],["lamiera","lamiera comprata","lamiera tagliata comprata, poi le nostre fasi"],
 ["fuori","fuori","pezzo fatto da un terzista su nostro disegno"],["cliente","dal cliente","conto lavoro: arriva dal cliente"],
 ["escluso","escluso","è nel disegno, non lo forniamo noi"],["noquote","no quote","non si quota, con il motivo"]];
const nomeModo = k => (MODI.find(x=>x[0]===k)||[,k])[1];
const MODO_CON_CICLO = new Set(["noi","lamiera","fuori"]);
