
/* icone aggiunte in v3, stesso tratto */
Object.assign(ICN,{
 upload:'<path d="M12 15V4m0 0-4 4m4-4 4 4"/><path d="M4.5 15.5v3a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5v-3"/>',
 refresh:'<path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3"/><path d="M19.5 4.5v3.8h-3.8"/>',
 eye:'<path d="M3 12c1.6-4 5-7 9-7s7.4 3 9 7c-1.6 4-5 7-9 7s-7.4-3-9-7z"/><circle cx="12" cy="12" r="2.8"/>',
 folder:'<path d="M3.5 7.2a1.7 1.7 0 0 1 1.7-1.7h4.2l2 2.2h7.4a1.7 1.7 0 0 1 1.7 1.7v8.9a1.7 1.7 0 0 1-1.7 1.7H5.2a1.7 1.7 0 0 1-1.7-1.7V7.2z"/>',
 swap:'<path d="M5 8.5h13m0 0-3.5-3.5M18 8.5 14.5 12M19 15.5H6m0 0 3.5-3.5M6 15.5 9.5 19"/>',
 list:'<path d="M9 6.5h11M9 12h11M9 17.5h11"/><circle cx="4.8" cy="6.5" r="1" fill="currentColor"/><circle cx="4.8" cy="12" r="1" fill="currentColor"/><circle cx="4.8" cy="17.5" r="1" fill="currentColor"/>',
 trash:'<path d="M4.5 7h15M9.5 7V5h5v2M6.5 7l1 12.5h9L17.5 7"/>'
});

/* ================= ANAGRAFICA: clienti e mittenti autorizzati ================= */
const CLIENTI = [
 {id:"acme",nome:"ACME Trattori",sigla:"AT",col:"#a93127",sett:"agri",dominio:"acme-trattori.example",sede:"Borgo Nord (XX)",stab:0,nas:"ACME TRATTORI",portale:"—",attivo:true,
  mittenti:[{n:"Rossi",r:"buyer",e:"rossi@acme-trattori.example",on:true,ultimo:"29/09"},
            {n:"Luca Neri",r:"ufficio tecnico",e:"l.neri@acme-trattori.example",on:true,ultimo:"oggi"},
            {n:"Acquisti (casella)",r:"casella generica",e:"acquisti@acme-trattori.example",on:true,ultimo:"—"}]},
 {id:"vignabot",nome:"VignaBot",sigla:"VB",col:"#4a6b2a",sett:"agri",dominio:"vignabot.example",sede:"Valmont (FR)",stab:1,nas:"VIGNABOT",portale:"—",attivo:true,
  mittenti:[{n:"Claire Dubois",r:"buyer",e:"c.dubois@vignabot.example",on:true,ultimo:"29/09"},
            {n:"Louis Petit",r:"ufficio tecnico",e:"l.petit@vignabot.example",on:true,ultimo:"ieri"}]},
 {id:"fitlab",nome:"FitLab",sigla:"TG",col:"#2b5d9e",sett:"altri",dominio:"fitlab.example",sede:"Riva Est (XX)",stab:0,nas:"FITLAB",portale:"portal.fitlab.example",attivo:true,
  mittenti:[{n:"Sara Fiorini",r:"buyer",e:"s.fiorini@fitlab.example",on:true,ultimo:"02/10"},
            {n:"Ufficio acquisti",r:"casella generica",e:"acquisti@fitlab.example",on:true,ultimo:"oggi"},
            {n:"Marketing",r:"sospeso dal 12/09",e:"news@fitlab.example",on:false,ultimo:"12/09"}]},
 {id:"tdl",nome:"Trattori Delta",sigla:"SD",col:"#8a5300",sett:"agri",dominio:"trattoridelta.example",sede:"Pianura (XX)",stab:2,nas:"TDL",portale:"supplier.trattoridelta.example",attivo:true,
  mittenti:[{n:"Paolo Verdini",r:"buyer",e:"p.verdini@trattoridelta.example",on:true,ultimo:"03/10"},
            {n:"Portale fornitori",r:"notifiche automatiche",e:"no-reply@trattoridelta.example",on:true,ultimo:"ieri"}]},
 {id:"potaflex",nome:"Potaflex",sigla:"PL",col:"#574099",sett:"agri",dominio:"potaflex.example",sede:"Bellerive (FR)",stab:1,nas:"POTAFLEX",portale:"—",attivo:true,
  mittenti:[{n:"Jean Moreau",r:"buyer",e:"j.moreau@potaflex.example",on:true,ultimo:"25/09"}]},
 {id:"carrellinord",nome:"Carrelli Nord",sigla:"TM",col:"#4f5a63",sett:"lift",dominio:"carrellinord.example",sede:"Bologna",stab:3,nas:"CARRELLI NORD",portale:"—",attivo:true,
  mittenti:[{n:"Irene Coste",r:"buyer",e:"i.coste@carrellinord.example",on:true,ultimo:"oggi"}]},
 {id:"grualfa",nome:"Gru Alfa",sigla:"FG",col:"#8a4b2a",sett:"off",dominio:"grualfa.example",sede:"Valle Alta (XX)",stab:0,nas:"GRU ALFA",portale:"—",attivo:true,
  mittenti:[{n:"Gino Ferretti",r:"buyer",e:"g.ferretti@grualfa.example",on:true,ultimo:"22/09"}]},
 {id:"falcibeta",nome:"Falci Beta",sigla:"KR",col:"#3f7a5c",sett:"agri",dominio:"falcibeta.example",sede:"Feldheim (DE)",stab:1,nas:"FALCI BETA",portale:"—",attivo:true,
  mittenti:[{n:"Hans Weber",r:"buyer",e:"h.weber@falcibeta.example",on:true,ultimo:"15/09"}]},
 {id:"mietigamma",nome:"Mieti Gamma",sigla:"CL",col:"#6b7d2a",sett:"agri",dominio:"mietigamma.com",sede:"Nordstadt (DE)",stab:1,nas:"MIETI GAMMA",portale:"—",attivo:false,
  mittenti:[{n:"Eva Schmidt",r:"buyer",e:"e.schmidt@mietigamma.com",on:true,ultimo:"18/07"}]}
];

/* ================= RICHIESTE: l'unità di lavoro dell'Inbox (più prodotti in una sola richiesta) ================= */
const RICHIESTE = [
 {id:"r-ar1",cli:"acme",titolo:"RICHIESTA D'OFFERTA 990020338",numCli:"990020338",aperta:"02/09/2026",ora:"15:07",buyer:"Rossi",prodotti:["at1"],rfq:"q-ar1",scad:"29/10",sla:"warn"},
 {id:"r-ar2",cli:"acme",titolo:"RICHIESTA D'OFFERTA 990020412",numCli:"990020412",aperta:"03/10/2026",ora:"10:44",buyer:"Luca Neri",prodotti:["at2","at3"],rfq:null,scad:"24/10",sla:"ok"},
 {id:"r-vb1",cli:"vignabot",titolo:"Demande de devis — 902235_B / 902256_B",aperta:"24/09/2026",ora:"09:12",buyer:"Claire Dubois",prodotti:["vb1","vb2"],rfq:null,scad:"09/10",sla:"risk"},
 {id:"r-tg1",cli:"fitlab",titolo:"Richiesta di quotazione 9N007614AE",numCli:"RDO 2026/318",aperta:"30/09/2026",ora:"10:21",buyer:"Sara Fiorini",prodotti:["tg1"],rfq:"q-tg1",scad:"15/10",sla:"ok"},
 {id:"r-sd1",cli:"tdl",titolo:"Nuova RdO — S9.1248 / S9.1249",aperta:"03/10/2026",ora:"15:22",buyer:"Paolo Verdini",prodotti:["sd1","sd2"],rfq:null,scad:"08/10",sla:"warn"},
 {id:"r-pl1",cli:"potaflex",titolo:"Demande de prix — support batterie 92176C",aperta:"10/09/2026",ora:"11:02",buyer:"Jean Moreau",prodotti:["pl1"],rfq:"q-pl1",scad:"—",sla:"ok"}
];

/* prodotti: appartengono a una richiesta (o a una RFQ storica) */
const PROD = {
 at1:{cod:"9990708A1",nome:"Supporto batteria",qta:"non indicata"},
 at2:{cod:"9990712B1",nome:"Staffa parafango",qta:"600 pz/anno"},
 at3:{cod:"9990713C1",nome:"Supporto faro",qta:"600 pz/anno"},
 vb1:{cod:"902235_B",nome:"Telaio V",qta:"40 pz"},
 vb2:{cod:"902256_B",nome:"SymTelaio",qta:"60 pz"},
 tg1:{cod:"9N007614AE",nome:"Telaio Reformer",qta:"1.200 pz/anno"},
 sd1:{cod:"S9.1248",nome:"Staffa serbatoio",qta:"250 pz"},
 sd2:{cod:"S9.1249",nome:"Staffa pompa",qta:"250 pz"},
 pl1:{cod:"92176C",nome:"Supporto batteria",qta:"500 pz"},
 fa1:{cod:"GA-55.210",nome:"Staffa braccio gru",qta:"350 pz/anno"},
 sd0:{cod:"S9.0987",nome:"Supporto cabina",qta:"400 pz/anno"},
 kr1:{cod:"920-097",nome:"Staffa trincia",qta:"900 pz/anno"},
 cl1:{cod:"9911.45.22",nome:"Telaio sedile",qta:"1.500 pz/anno"},
 pl0:{cod:"93840B",nome:"Carter forbice",qta:"800 pz"}
};

/* ================= MESSAGGI: ognuno appartiene a una richiesta e cita uno o più prodotti ================= */
const MESSAGGI = [
 {mid:"ar1",req:"r-ar1",prod:["at1"],d:"mar 2 settembre",t:"15:07",dir:"in",chi:"Rossi",r:"buyer",sub:"RICHIESTA D'OFFERTA 990020338",
  txt:`<p>Buongiorno,</p><p>in allegato la richiesta d’offerta <mark class="hl">990020338</mark> per il codice <mark class="hl">9990708A1</mark> — supporto batteria.</p><p>Vi chiediamo risposta entro 5 giorni, con il cost breakdown.</p>`,
  files:[F("990020338.zip","zip","8,1 MB"),F("RDO 990020338_100_260902.msg","msg","240 KB")],
  meta:"la richiesta è il documento Baan allegato: codice del prodotto e 4 posizioni nell’elenco particolari"},
 {mid:"ar2",req:"r-ar1",prod:["at1"],d:"mer 3 settembre",t:"09:20",dir:"out",chi:"Franco",r:"commerciale · noi",
  txt:`<p>Buongiorno, ricevuta la 990020338. Lino guarda la fattibilità, vi aggiorniamo entro venerdì.</p>`},
 {mid:"ar3",req:"r-ar1",prod:["at1"],d:"lun 29 settembre",t:"11:15",dir:"in",chi:"Rossi",r:"buyer",
  txt:`<p>Ricordo che insieme all’offerta ci serve il <mark class="hl">CBD</mark> e il peso al pezzo in kg.</p>`,meta:"richiesta commerciale, nessun codice nuovo"},
 {mid:"ar4",req:"r-ar1",prod:["at1"],d:"ieri",t:"14:03",kind:"xref",tz:"f3",tzn:"Zincatura Reggiana",
  lav:"zincatura Fe/Zn 12 III Cr3 come da specifica SPEC-ZINC-013",quando:"4 messaggi · ultimo ieri 14:03",esito:C("prop","offerta 0,42 €/pz da valutare")},

 {mid:"ar5",req:"r-ar2",prod:["at2","at3"],d:"ven 3 ottobre",t:"10:44",dir:"in",chi:"Luca Neri",r:"ufficio tecnico",sub:"RICHIESTA D'OFFERTA 990020412",
  txt:`<p>Buongiorno,</p><p>richiesta d’offerta <mark class="hl">990020412</mark> per <mark class="hl">9990712B1</mark> staffa parafango e <mark class="hl">9990713C1</mark> supporto faro, 600 pz/anno ciascuno. Disegni e STEP in allegato.</p>`,
  files:[F("9990712B1.pdf","pdf","402 KB"),F("9990712B1.stp","stp","1,1 MB"),F("9990713C1.pdf","pdf","388 KB")],
  meta:"due codici nel testo nuovo e nei nomi dei file"},
 {mid:"ar6",req:"r-ar2",prod:["at3"],d:"ven 3 ottobre",t:"16:30",dir:"out",chi:"Franco",r:"commerciale · noi",
  txt:`<p>Buongiorno Andrea, del 9990713C1 manca lo STEP: ce lo mandate?</p>`},
 {mid:"ar7",req:"r-ar2",prod:["at3"],d:"oggi",t:"08:02",dir:"in",chi:"Luca Neri",r:"ufficio tecnico",
  txt:`<p>Ecco il 3D del 9990713C1.</p>`,files:[F("9990713C1.stp","stp","860 KB")],meta:"risposta nella stessa catena di mail"},

 {mid:"vb-m1",req:"r-vb1",prod:["vb1","vb2"],d:"gio 24 settembre",t:"09:12",dir:"in",chi:"Claire Dubois",r:"buyer",sub:"Demande de devis — 902235_B / 902256_B",
  txt:`<p>Bonjour,</p><p>demande de devis pour <mark class="hl">902235_B</mark> — 40 unités — et <mark class="hl">902256_B</mark> — 60 unités. Livraison souhaitée semaine 45.</p><p>Merci, Camille</p>`,
  files:[F("902235-telaio_b2.step","step","1,8 MB"),F("902235-telaio_b2.pdf","pdf","512 KB"),F("902256-symtelaio_b2.step","step","2,1 MB")],
  meta:"codici nel testo nuovo e nei nomi dei file · le quantità sono scritte in testo libero"},
 {mid:"vb-m2",req:"r-vb1",prod:["vb1","vb2"],d:"gio 24 settembre",t:"11:40",dir:"out",chi:"Franco",r:"commerciale · noi",
  txt:`<p>Bonjour Camille, nous confirmons la réception. Nous revenons avec la faisabilité d’ici vendredi.</p>`},
 {mid:"vb-m3",req:"r-vb1",prod:["vb1","vb2"],d:"lun 29 settembre",t:"08:05",dir:"in",chi:"Claire Dubois",r:"buyer",
  txt:`<p>Perfetto, confermo quanto sotto.</p><div class="quote">&gt; 902235_B — 40 unités, livraison semaine 45<br>&gt; 902256_B — 60 unités</div>`,
  meta:"nessun codice nel testo nuovo: il riferimento è la mail del 24/09 09:12"},
 {mid:"vb-m4",req:"r-vb1",prod:["vb1"],d:"ieri",t:"14:20",dir:"in",chi:"Louis Petit",r:"ufficio tecnico",sub:"Mise à jour plan 902235",
  catena:"nuova catena di mail · agganciata a questa richiesta perché cita 902235-telaio_b3",
  txt:`<p>Mise à jour du plan: <mark class="hl">902235-telaio_b3</mark>. Merci d’ignorer la version b2.</p>`,
  files:[F("902235-telaio_b3.pdf","pdf","498 KB")],
  meta:"stesso codice, revisione b3: sostituisce il disegno b2 del 24/09"},

 {mid:"tg-m1",req:"r-tg1",prod:["tg1"],d:"mar 30 settembre",t:"10:21",dir:"in",chi:"Sara Fiorini",r:"buyer",sub:"Richiesta di quotazione 9N007614AE",
  txt:`<p>Buongiorno,</p><p>richiesta di quotazione per <mark class="hl">9N007614AE</mark> (telaio reformer), volume annuo 1.200 pz. In allegato lo zip con disegni e 3D.</p><p>Offerta entro il 15/10.</p>`,
  files:[F("9N007614AE_rev2.zip","zip","6,4 MB")],meta:"6 file estratti dallo zip · il suffisso AE fa parte del codice, non è una revisione"},
 {mid:"tg-m2",req:"r-tg1",prod:["tg1"],d:"mar 30 settembre",t:"16:02",dir:"out",chi:"Franco",r:"commerciale · noi",
  txt:`<p>Buongiorno Giulia, ricevuto. Lino analizza la fattibilità, ti aggiorniamo entro mercoledì.</p>`},
 {mid:"tg-m3",req:"r-tg1",prod:["tg1"],d:"gio 2 ottobre",t:"09:47",dir:"in",chi:"Sara Fiorini",r:"buyer",
  txt:`<p>Aggiungo il target price per il confronto. Il capitolato è quello che trovate sul nostro portale fornitori.</p>`,files:[F("target_price_Q4.xlsx","xlsx","84 KB")]},
 {mid:"tg-m4",req:"r-tg1",prod:["tg1"],d:"oggi",t:"09:10",kind:"xref",tz:"f2",tzn:"Torneria Bolognese",
  lav:"tornitura del componente 9N008519AB",quando:"2 messaggi · ultimo oggi 09:10",esito:C("ok","quotata 4,20 €/pz")},

 {mid:"sd-m1",req:"r-sd1",prod:["sd1","sd2"],d:"ven 3 ottobre",t:"15:22",dir:"in",chi:"Paolo Verdini",r:"buyer",sub:"Nuova RdO — S9.1248 / S9.1249",
  txt:`<p>Buongiorno, abbiamo caricato i disegni sul <mark class="hl">portale fornitori</mark>, codici <mark class="hl">S9.1248</mark> e <mark class="hl">S9.1249</mark>. 250 pz ciascuno.</p>`,
  meta:"nessun allegato: i file sono indicati sul portale del cliente"},

 {mid:"pl-m0",req:"r-pl1",prod:["pl1"],d:"mer 10 settembre",t:"11:02",dir:"in",chi:"Jean Moreau",r:"buyer",sub:"Demande de prix — support batterie 92176C",
  txt:`<p>Bonjour, merci de nous faire une offre pour le <mark class="hl">92176C</mark>, 500 pièces.</p>`,files:[F("92176C.stp","stp","940 KB"),F("92176C.pdf","pdf","420 KB")]},
 {mid:"pl-m1",req:"r-pl1",prod:["pl1"],d:"gio 25 settembre",t:"17:30",dir:"out",chi:"Franco",r:"commerciale · noi",
  txt:`<p>Bonjour Hugo, en pièce jointe notre offre SO 5478 pour le 92176C.</p>`,files:[F("SO 5478.pdf","pdf","320 KB")],meta:"nostra offerta in uscita"}
];

/* ================= DOCUMENTI riconosciuti, per prodotto =================
   slot = file associato a un ruolo (STEP o PDF del prodotto; 3D/2D/DXF di una sotto-parte).
   prov: mail | port | car   ·   stato: prop (proposto) | ok (confermato) | verifica (fonti discordanti)
   {portale:true} = il cliente lo indica sul portale, non ancora acquisito.  trova = che cosa trova «Cerca di nuovo». */
const SL=(f,s,prov,da,perche,stato,x)=>Object.assign({f,s,prov,da,perche,stato},x||{});
const PORT={portale:true};
const DOCS = {
 at1:{step:SL("9990708A_1.STP","2,4 MB","mail","zip della mail del 02/09 · Rossi","nome 9990708A_1 = 9990708A1 · radice dello STEP d’assieme","prop",{alt:[["9990708A_1.IGS","2,9 MB","stesso pezzo in formato IGES"]]}),
  pdf:SL("9990708A_1.pdf","512 KB","mail","zip della mail del 02/09 · Rossi","cartiglio PART N° 9990708A1 · disegno d’assieme con l’elenco particolari","prop",{rev:"1"}),
  parti:[{cod:"9990707A1",nome:"Supporto",tipo:"sciolto",qta:1,fonti:[["neu","STEP","9990707A ×1 nello STEP d’assieme 9990708A_1.STP"],["ok","Suo disegno","9990707A_1.pdf, senza elenco particolari: è un particolare"],["ok","Disegno","pos. 4 dell’elenco di 9990708A_1.pdf, qtà 1 (controllo incrociato)"]],
          d3:SL("9990707A_1.STP","310 KB","mail","zip della mail del 02/09","nome 9990707A_1 = 9990707A1 · pos. 4 dell’elenco","prop"),
          d2:SL("9990707A_1.pdf","280 KB","mail","zip della mail del 02/09","cartiglio 9990707A1 · pos. 4 del disegno d’assieme","prop"),dxf:null},
         {cod:"990679X1",cat:"minuteria",catConf:false,catPerche:"dado a norma: categoria minuteria letta dalla grammatica, da confermare",nome:"Dado M8 cl. 8 PL",tipo:"comm",qta:2,fonti:[["neu","STEP","990679X1 ×2; nel suo STEP si chiama 990679X1_PRT"],["neu","Tipo","dado, senza disegno 2D: particolare commerciale"],["ok","Disegno","pos. 3 dell’elenco, qtà 2 (controllo incrociato)"]],d3:SL("990679X_1.STP","48 KB","mail","zip della mail del 02/09","nome · pos. 3 · STEP del fornitore","prop"),d2:null,dxf:null},
         {cod:"939552X1",cat:"minuteria",catConf:false,catPerche:"dado a norma: categoria minuteria letta dalla grammatica, da confermare",nome:"Dado SDPR M6",tipo:"comm",qta:3,fonti:[["neu","STEP","939552X1 ×3; nel suo STEP si chiama 939552X1_PRT"],["neu","Tipo","dado, senza disegno 2D: particolare commerciale"],["ok","Disegno","pos. 2 dell’elenco, qtà 3 (controllo incrociato)"]],d3:SL("939552X_1.STP","51 KB","mail","zip della mail del 02/09","nome · pos. 2 · STEP del fornitore","prop"),d2:null,dxf:null},
         {cod:"9443580X1",cat:"minuteria",catConf:false,catPerche:"dado a norma: categoria minuteria letta dalla grammatica, da confermare",nome:"Dado SDPR M5",tipo:"comm",qta:1,fonti:[["neu","STEP","9443580X ×1: lo STEP lo scrive senza la cifra della versione"],["neu","Tipo","dado, senza disegno 2D: particolare commerciale"],["ok","Disegno","pos. 1 dell’elenco, qtà 1 (controllo incrociato)"]],d3:SL("9443580X_1.STP","44 KB","mail","zip della mail del 02/09","nome 9443580X_1 · pos. 1 · lo STEP omette la cifra della versione","prop"),d2:null,dxf:null}],
  liberi:[["RDO 990020338_100_260902.msg","documento Baan della richiesta","della richiesta"],["9990707A_1.zip","archivio dentro l’archivio: può contenere il DXF di 9990707A1","","in_corso"],["image001.jpg","immagine nella firma della mail",""]],
  trova:[{k:"p:0:dxf",slot:SL("9990707A_1.dxf","38 KB","mail","archivio annidato 9990707A_1.zip","nome 9990707A_1 = 9990707A1 · sviluppo in piano","prop"),
          msg:"Aperto l’archivio annidato 9990707A_1.zip: c’era lo sviluppo DXF di 9990707A1.",togli:"9990707A_1.zip"}]},
 at2:{step:SL("9990712B1.stp","1,1 MB","mail","mail del 03/10 · Luca Neri","nome = codice richiesto · radice dello STEP","prop"),
  pdf:SL("9990712B1.pdf","402 KB","mail","mail del 03/10 · Luca Neri","cartiglio 9990712B1","prop"),parti:[],liberi:[],trova:[]},
 at3:{step:null,pdf:SL("9990713C1.pdf","388 KB","mail","mail del 03/10 · Luca Neri","cartiglio 9990713C1","prop"),parti:[],liberi:[],
  trova:[{k:"step",slot:SL("9990713C1.stp","860 KB","mail","mail di oggi 08:02 · Luca Neri","nome = codice richiesto · arrivato dopo la richiesta","prop"),
          msg:"Nella mail di Luca Neri di oggi 08:02 c’è lo STEP di 9990713C1: associato."}]},
 vb1:{step:SL("902235-telaio_b2.step","1,8 MB","mail","mail del 24/09 · Claire Dubois","nome 902235-telaio = 902235_B","prop"),
  pdf:SL("902235-telaio_b3.pdf","498 KB","mail","mail di ieri · Louis Petit","cartiglio 902235-Telaio V · revisione b3, sostituisce la b2","verifica",
         {rev:"b3",nota:"Il disegno b2 conteneva una nota che cita 902256-SymTelaio: controlla che il b3 riguardi solo 902235_B.",alt:[["902235-telaio_b2.pdf","512 KB","revisione precedente, mail del 24/09"]]}),
  parti:[{cod:"902235-01",nome:"Piastra",tipo:"sciolto",qta:2,d3:null,d2:SL("902235-telaio_b3.pdf","498 KB","mail","mail di ieri · Louis Petit","distinta nel PDF del prodotto, pos. 1","prop"),dxf:null},
         {cod:"902235-04",nome:"Rinforzo",tipo:"sciolto",qta:1,d3:null,d2:null,dxf:null}],
  liberi:[["902235-telaio_b2.pdf","revisione b2, sostituita dalla b3",""]],trova:[]},
 vb2:{step:SL("902256-symtelaio_b2.step","2,1 MB","mail","mail del 24/09 · Claire Dubois","nome 902256-symtelaio = 902256_B","prop"),pdf:null,parti:[],liberi:[],trova:[]},
 tg1:{step:SL("9N007614AE.stp","3,2 MB","mail","zip della mail del 30/09 · Sara Fiorini","radice dello STEP d’assieme","ok"),
  pdf:SL("9N007614AE.pdf","640 KB","mail","zip della mail del 30/09","cartiglio 9N007614AE","ok",{rev:"2"}),
  parti:[{cod:"9N008518AA",nome:"Supporto saldato",tipo:"sottoass",qta:1,conf:true,d3:SL("9N008518AA.stp","1,2 MB","mail","zip della mail del 30/09","figlio della radice nello STEP","ok"),
          d2:SL("9N008518AA.pdf","310 KB","mail","zip della mail del 30/09","cartiglio 9N008518AA","ok"),dxf:null},
         {cod:"9N008519AB",nome:"Perno guida",tipo:"sciolto",qta:2,conf:true,d3:null,d2:SL("9N008519AB.pdf","190 KB","mail","zip della mail del 30/09","cartiglio 9N008519AB","ok"),
          dxf:SL("9N008519AB_flat.dxf","24 KB","mail","zip della mail del 30/09","nome del file: sviluppo del perno","prop")},
         {cod:"9N008520AA",nome:"Piastra base",tipo:"sciolto",qta:1,sotto:"9N008518AA",
          d3:SL("9N008520AA.stp","220 KB","mail","zip della mail del 30/09","figlio di 9N008518AA nello STEP d’assieme","prop"),
          d2:SL("9N008520AA.pdf","205 KB","mail","zip della mail del 30/09","cartiglio 9N008520AA · pos. 1 del disegno di 9N008518AA","prop"),
          dxf:SL("9N008520AA.dxf","31 KB","mail","zip della mail del 30/09","nome 9N008520AA · sviluppo in piano","prop")}],
  liberi:[["9N008521AA_scan.pdf","PDF senza testo (scansione): l’analisi non ha potuto leggere il cartiglio","","errore"],["capitolato_TG_2026.pdf","capitolato del cliente, scaricato dal portale fornitori","della richiesta"],["target_price_Q4.xlsx","foglio commerciale del buyer, non è un documento tecnico","della richiesta"]],trova:[]},
 sd1:{step:PORT,pdf:PORT,parti:[],liberi:[],
  trova:[{k:"pdf",slot:SL("S9.1248_rev0.pdf","290 KB","port","portale fornitori TDL","cartiglio S9.1248 · scaricato dal portale","prop"),
          msg:"Sul portale fornitori TDL c’è il PDF di S9.1248: scaricato e associato. Lo STEP non c’è ancora."}]},
 sd2:{step:PORT,pdf:SL("S9.1249_rev0.pdf","310 KB","port","portale fornitori TDL · caricato da Franco il 04/10","cartiglio S9.1249","ok"),parti:[],liberi:[],trova:[]},
 pl1:{step:SL("92176C.stp","940 KB","mail","mail del 10/09 · Jean Moreau","radice dello STEP","ok"),pdf:SL("92176C.pdf","420 KB","mail","mail del 10/09","cartiglio 92176C","ok"),
  parti:[{cod:"92176C-02",nome:"Squadretta",tipo:"sciolto",qta:2,conf:true,d3:null,d2:SL("92176C.pdf","420 KB","mail","mail del 10/09","distinta del PDF, pos. 2","ok"),
          dxf:SL("92176C-02.dxf","22 KB","car","caricato a mano da Lino il 12/09","sviluppo rifatto in ufficio tecnico","ok")}],
  liberi:[["SO 5478.pdf","nostra offerta in uscita","della richiesta"]],trova:[]}
};
/* RFQ storiche: documenti completi e confermati */
function docsCompleti(cod,da){return {step:SL(cod+".stp","1,0 MB","mail",da,"radice dello STEP","ok"),pdf:SL(cod+".pdf","350 KB","mail",da,"cartiglio "+cod,"ok"),parti:[],liberi:[],trova:[]};}
Object.assign(DOCS,{fa1:docsCompleti("GA-55.210","mail del 01/08 · Gino Ferretti"),sd0:docsCompleti("S9.0987","portale TDL · 12/06"),
  kr1:docsCompleti("920-097","mail del 03/06 · Hans Weber"),cl1:docsCompleti("9911.45.22","mail del 14/03 · Eva Schmidt"),pl0:docsCompleti("93840B","mail del 02/05 · Jean Moreau")});

/* ================= RFQ ================= */
const STATI = [["avviata","Avviate"],["accettata","Accettate"],["produzione","In produzione"],["terminata","Terminate"]];
const PASSI = ["Documenti e NAS","Distinta","Offerta"];
const RFQ = [
 {id:"q-ar1",num:"990020338",req:"r-ar1",faseBase:"RICEVUTA",passo:0,resp:"Franco",agg:"ieri 14:03",
  nas:"ACME TRATTORI\\WIP\\2026 09 02 Rossi RICHIESTA D'OFFERTA 990020338"},
 {id:"q-tg1",num:"RDO 2026/318",req:"r-tg1",faseBase:"FATTIBILITA",passo:1,resp:"Lino",agg:"oggi 09:10",
  nas:"FITLAB\\WIP\\2026 09 30 Fiorini Richiesta di quotazione 9N007614AE"},
 {id:"q-pl1",num:"P-2026-0131",req:"r-pl1",faseBase:"OFFERTA_INVIATA",passo:2,resp:"Franco",agg:"25/09",nota:"offerta SO 5478 inviata, nessun riscontro",
  nas:"POTAFLEX\\WIP\\2026 09 10 Martin Demande de prix 92176C"},
 {id:"q-fa1",bomOk:true,num:"P-2026-0118",req:null,cli:"grualfa",titolo:"Richiesta offerta staffa braccio gru",buyer:"Gino Ferretti",aperta:"01/08/2026",scad:"—",prodotti:["fa1"],
  faseBase:"ACCETTATA",passo:2,resp:"Franco",agg:"22/09",nota:"ordine atteso entro ottobre",nas:"GRU ALFA\\WIP\\2026 08 01 Ferretti Staffa braccio gru"},
 {id:"q-sd0",bomOk:true,num:"S9-RDO-0987",req:null,cli:"tdl",titolo:"RdO supporto cabina S9.0987",buyer:"Paolo Verdini",aperta:"12/06/2026",scad:"—",prodotti:["sd0"],
  faseBase:"DISTINTA_ERP",passo:2,resp:"Franco",agg:"30/09",nota:"accettata il 30/09",nas:"TDL\\WIP\\2026 06 12 Verdi Supporto cabina"},
 {id:"q-kr1",bomOk:true,num:"920-097",req:null,cli:"falcibeta",titolo:"Anfrage Zeichnung 920-097",buyer:"Hans Weber",aperta:"03/06/2026",scad:"—",prodotti:["kr1"],
  faseBase:"PRODUZIONE",passo:2,resp:"Marta",agg:"oggi",nota:"Stab. 2 · lotto 1 di 4",nas:"FALCI BETA\\DONE\\2026 06 03 Weber Staffa trincia (done SO 5402)"},
 {id:"q-cl1",bomOk:true,num:"9911.45.22",req:null,cli:"mietigamma",titolo:"Anfrage Sitzrahmen 9911.45.22",buyer:"Eva Schmidt",aperta:"14/03/2026",scad:"—",prodotti:["cl1"],
  faseBase:"PRODUZIONE",statoThread:"CHIUSA",nota:"ordine evaso: thread chiuso",passo:2,resp:"Franco",agg:"18/07",nas:"MIETI GAMMA\\DONE\\2026 03 14 Schmidt Telaio sedile (done SO 5311)"},
 {id:"q-pl0",bomOk:true,num:"P-2026-0071",req:null,cli:"potaflex",titolo:"Demande de prix — carter sécateur 93840B",buyer:"Jean Moreau",aperta:"02/05/2026",scad:"—",prodotti:["pl0"],
  faseBase:"PERSA",passo:2,resp:"Franco",agg:"10/06",nota:"prezzo fuori target del 12%",nas:"POTAFLEX\\DONE\\2026 05 02 Martin Carter forbice"}
];
RFQ.forEach(preparaRFQ);
let contatoreRFQ = 141;

/* ================= code e sezioni dell'Inbox ================= */
const TERZISTI = [
 {id:"f3",nome:"Zincatura Reggiana",sigla:"ZR",lav:"zincatura a caldo e Fe/Zn",
  chat:[{req:"r-ar1",pid:"at1",stato:C("prop","offerta da valutare"),ultimo:"ieri 14:03",
   msg:[{d:"lun 29 settembre",t:"10:12",dir:"out",chi:"Fabio",r:"acquisti · noi",
         txt:`<p>Buongiorno, richiesta di quotazione per zincatura <mark class="hl">Fe/Zn 12 III Cr3</mark> del supporto <mark class="hl">9990708A1</mark>, specifica ACME SPEC-ZINC-013. In allegato il disegno.</p>`,
         files:[F("9990708A_1.pdf","pdf","512 KB")]},
        {d:"mer 1 ottobre",t:"09:05",dir:"in",chi:"Rita Mellini",r:"Zincatura Reggiana",txt:`<p>Buongiorno, lo spessore 12 micron lo garantiamo. Serve sapere se il pezzo arriva già saldato.</p>`},
        {d:"mer 1 ottobre",t:"15:40",dir:"out",chi:"Fabio",r:"acquisti · noi",txt:`<p>Sì, saldato e sgrassato: arriva finito dal nostro Stab. 2.</p>`},
        {d:"ieri",t:"14:03",dir:"in",chi:"Rita Mellini",r:"Zincatura Reggiana",txt:`<p>In allegato l’offerta: 0,42 €/pz per lotti da 500.</p>`,
         files:[F("offerta_zinc_9990708A1.pdf","pdf","96 KB")],meta:"il prezzo entra nel preventivo dalla pagina RFQ, dopo la conferma di una persona"}]}]},
 {id:"f2",nome:"Torneria Bolognese",sigla:"TB",lav:"tornitura e lavorazioni meccaniche",
  chat:[{req:"r-tg1",pid:"tg1",cod:"9N008519AB",nome:"Perno guida",stato:C("ok","quotata 4,20 €/pz"),ultimo:"oggi 09:10",
   msg:[{d:"gio 2 ottobre",t:"11:30",dir:"out",chi:"Fabio",r:"acquisti · noi",txt:`<p>Richiesta quotazione tornitura <mark class="hl">9N008519AB</mark>, 1.200 pz/anno.</p>`,files:[F("9N008519AB.pdf","pdf","180 KB")]},
        {d:"oggi",t:"09:10",dir:"in",chi:"Ugo Montini",r:"Torneria Bolognese",txt:`<p>4,20 €/pz per lotti da 300, consegna 15 giorni.</p>`}]}]},
 {id:"f1",nome:"Verniciatura Emiliana",sigla:"VE",lav:"verniciatura a polvere",chat:[]}
];

const SMISTARE = [
 {id:"s1",chi:"Luca Neri",em:"l.neri@acme-trattori.example",cli:"acme",t:"oggi 07:51",sub:"RE: 990020338 — certificato materiale",
  snip:"Risposta su una richiesta aperta",stato:C("acc","aggancio proposto 95%","link"),dest:"r-ar1",
  txt:`<p>Buongiorno, allego il certificato del materiale del supporto 9990707A1.</p>`,files:[F("certificato_S235JR_9990707A1.pdf","pdf","140 KB")],
  cands:[{t:"ACME Trattori › RICHIESTA D'OFFERTA 990020338",st:C("acc","95%"),why:"In-Reply-To corrisponde a un messaggio di questa richiesta.",src:[C("neu","In-Reply-To"),C("neu","conversazione Outlook"),C("acc","1 candidato")]}]},
 {id:"s2",chi:"Irene Coste",em:"i.coste@carrellinord.example",cli:"carrellinord",t:"oggi 08:14",sub:"Richiesta offerta staffa montante",
  snip:"Mittente autorizzato · nessuna richiesta aperta per questo codice",stato:C("prop","nuova richiesta?"),nuovo:true,
  codici:[["CN-4471","Staffa montante","300 pz"]],
  txt:`<p>Buongiorno, chiediamo quotazione per la staffa <mark class="hl">CN-4471</mark>, 300 pz.</p>`,files:[F("CN-4471_rev_a.pdf","pdf","410 KB")],
  cands:[{t:"Nuova richiesta · Carrelli Nord",st:C("prop","proposto"),why:"Mittente autorizzato del cliente. Il codice non compare in nessuna richiesta aperta.",src:[C("neu","mittente autorizzato"),C("neu","oggetto"),C("acc","1 candidato")]}]},
 {id:"s3",chi:"Portale fornitori",em:"no-reply@trattoridelta.example",cli:"tdl",t:"ieri 09:30",sub:"Nuovo documento disponibile sul portale",
  snip:"Riguarda una richiesta aperta dello stesso cliente",stato:C("acc","aggancio proposto 80%","link"),dest:"r-sd1",
  txt:`<p>Un nuovo documento è disponibile per il codice S9.1248.</p>`,
  cands:[{t:"Trattori Delta › Nuova RdO — S9.1248 / S9.1249",st:C("acc","80%"),why:"Il codice compare fra i prodotti di una richiesta aperta dello stesso cliente.",src:[C("neu","codice nel testo"),C("neu","prodotto della richiesta")]}]}
];

const ALTRA = [
 {id:"a1",chi:"Ufficio acquisti",em:"acquisti@fitlab.example",az:"FitLab",cli:"fitlab",cat:"Commerciale (non RFQ)",t:"oggi 11:20",
  sub:"Aggiornamento condizioni di pagamento 2027",snip:"Nessun codice prodotto: riguarda il rapporto commerciale",
  txt:`<p>Buongiorno, dal 1° gennaio 2027 le condizioni passano a 60 giorni data fattura fine mese.</p>`,files:[F("condizioni_2027.pdf","pdf","140 KB")],azioni:["Archivia nel cliente","Inoltra all’amministrazione"]},
 {id:"a2",chi:"Rossi",em:"rossi@acme-trattori.example",az:"ACME Trattori",cli:"acme",cat:"Commerciale (non RFQ)",t:"ieri 16:10",
  sub:"Visita in stabilimento — proposta date",snip:"Organizzazione, nessuna richiesta d’offerta",
  txt:`<p>Possiamo venire il 20 o il 21 ottobre per vedere la linea di saldatura robotizzata.</p>`,azioni:["Apri in calendario","Archivia nel cliente"]},
 {id:"a3",chi:"Marcegaglia",em:"ordini@marcegaglia.com",az:"fornitore materiale",cli:null,cat:"Acquisti e materiali",t:"oggi 08:20",
  sub:"Conferma d’ordine 2026/4471 — lamiera S355 sp. 3",snip:"Consegna prevista 14/10 · 4,2 t",
  txt:`<p>Confermiamo l’ordine 2026/4471: lamiera S355JR sp. 3,0 mm, 4,2 t, consegna prevista 14/10 allo Stab. 2.</p>`,files:[F("CO_2026_4471.pdf","pdf","180 KB")],azioni:["Archivia in acquisti"]},
 {id:"a4",chi:"Mario Bianchi",em:"p.bianchi@proma-tec.it",az:"interno · noi",cli:null,cat:"Riunioni e calendario",t:"oggi 09:00",
  sub:"Riepilogo commerciale — lunedì 9:30",snip:"Invito con 4 partecipanti · sala riunioni Stab. 1",
  txt:`<p>Confermate la presenza per il riepilogo delle offerte aperte. Lunedì 9:30, sala riunioni.</p>`,azioni:["Apri in calendario","Archivia"]},
 {id:"a5",chi:"Microsoft Teams",em:"noreply@teams.microsoft.com",az:"automatica",cli:null,cat:"Notifiche automatiche",t:"oggi 07:02",
  sub:"Hai 3 messaggi non letti",snip:"Notifica automatica",txt:`<p>Riepilogo attività del canale Commerciale.</p>`,azioni:["Archivia"]}
];

const NASCOSTI = [
 {id:"n1",chi:"FitLab Marketing",em:"news@fitlab.example",cli:"fitlab",t:"oggi 06:40",sub:"FitLab Newsletter — ottobre",motivo:"mittente sospeso per questo cliente"},
 {id:"n2",chi:"Marta Sali",em:"m.sali@acme-trattori.example",cli:"acme",t:"oggi 10:05",sub:"Richiesta quotazione staffa 9990720F",
  motivo:"dominio del cliente, mittente non ancora autorizzato",
  porta:{nuovo:true,codici:[["9990720F","Staffa serbatoio idraulico","400 pz/anno"]],txt:`<p>Buongiorno, sono la nuova buyer per la linea 5. Vi chiedo quotazione per la staffa <mark class="hl">9990720F</mark>, 400 pz/anno.</p>`,
         files:[F("9990720F_1.pdf","pdf","350 KB")]}},
 {id:"n3",chi:"info@lamiere-rapide.it",em:"info@lamiere-rapide.it",cli:null,t:"ieri 16:40",sub:"Richiesta preventivo",motivo:"dominio non associato a nessun cliente"},
 {id:"n4",chi:"Fiere & Eventi",em:"news@fieremeccanica.it",cli:null,t:"ieri 11:12",sub:"MECSPE 2027 — early booking",motivo:"dominio non associato a nessun cliente"}
];
