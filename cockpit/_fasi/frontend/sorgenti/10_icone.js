/* ============ icone: tratto 1.7, griglia 24, una sola famiglia ============ */
const ICN = {
 inbox:'<path d="M3 13h5l1.5 2.5h5L16 13h5"/><path d="M4.5 5.5h15l1.5 7.5v5.5a1.5 1.5 0 0 1-1.5 1.5H4.5A1.5 1.5 0 0 1 3 18.5V13z"/>',
 mail:'<rect x="3" y="5.5" width="18" height="13" rx="1.6"/><path d="m3.6 6.4 8.4 6.1 8.4-6.1"/>',
 tray:'<path d="M3 15h4l1.4 2h7.2l1.4-2h4M5 15V6.5A1.5 1.5 0 0 1 6.5 5h11A1.5 1.5 0 0 1 19 6.5V15"/><path d="M3 15v3.5A1.5 1.5 0 0 0 4.5 20h15a1.5 1.5 0 0 0 1.5-1.5V15"/>',
 factory:'<path d="M3 20h18M4 20v-9l5 3V11l5 3V9l5 3v8"/><path d="M8 20v-3.5h3V20"/>',
 sliders:'<path d="M5 6h14M5 12h14M5 18h14"/><circle cx="9" cy="6" r="2.1"/><circle cx="15" cy="12" r="2.1"/><circle cx="8" cy="18" r="2.1"/>',
 users:'<path d="M3.5 20v-1.5A4.5 4.5 0 0 1 8 14h2a4.5 4.5 0 0 1 4.5 4.5V20"/><circle cx="9" cy="8.5" r="3.4"/><path d="M16.5 11.8A3.2 3.2 0 0 0 16.5 5.4M17.5 20v-1.5a4.5 4.5 0 0 0-1.6-3.4"/>',
 building:'<path d="M4 20V5.6a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1V20M15 10h4a1 1 0 0 1 1 1v9M4 20h16"/><path d="M7.5 8h1.5M7.5 12h1.5M7.5 16h1.5M11.5 8H13M11.5 12H13M11.5 16H13"/>',
 search:'<circle cx="11" cy="11" r="6.2"/><path d="m15.6 15.6 4.4 4.4"/>',
 clip:'<path d="M20 11.5 12.6 19a4.3 4.3 0 0 1-6-6l7.6-7.6a2.9 2.9 0 0 1 4.1 4.1l-7.5 7.6a1.5 1.5 0 0 1-2.1-2.1l6.8-6.9"/>',
 down:'<path d="M12 4v11m0 0 4-4m-4 4-4-4"/><path d="M4.5 18.5h15"/>',
 link:'<path d="M9.5 14.5 14.5 9.5"/><path d="M11.8 7.2 13.5 5.5a3.6 3.6 0 0 1 5 5l-1.7 1.7M12.2 16.8 10.5 18.5a3.6 3.6 0 0 1-5-5l1.7-1.7"/>',
 check:'<path d="m5 12.5 4.5 4.5L19 7.5"/>',
 alert:'<path d="M12 5.2 3.6 19h16.8L12 5.2z"/><path d="M12 10v4.2M12 16.6v.4"/>',
 clock:'<circle cx="12" cy="12" r="7.6"/><path d="M12 7.8V12l3 2"/>',
 chev:'<path d="m9.5 6 6 6-6 6"/>',
 plus:'<path d="M12 5.5v13M5.5 12h13"/>',
 x:'<path d="m6.5 6.5 11 11m0-11-11 11"/>',
 shield:'<path d="M12 3.6 5 6v6c0 4.2 3 7 7 8.4 4-1.4 7-4.2 7-8.4V6l-7-2.4z"/><path d="m9.2 12 2 2 3.6-3.6"/>',
 eyeoff:'<path d="M3.5 3.5l17 17"/><path d="M9.6 5.3A8.3 8.3 0 0 1 12 5c4.5 0 7.6 3 9 7-.5 1.4-1.3 2.7-2.3 3.7M6.6 7.1C5 8.3 3.8 10 3 12c1.4 4 4.5 7 9 7 1.3 0 2.5-.2 3.6-.7"/><path d="M10.2 10.4a2.4 2.4 0 0 0 3.3 3.4"/>',
 grid:'<rect x="4" y="4" width="7" height="7" rx="1.2"/><rect x="13" y="4" width="7" height="7" rx="1.2"/><rect x="4" y="13" width="7" height="7" rx="1.2"/><rect x="13" y="13" width="7" height="7" rx="1.2"/>',
 cube:'<path d="M12 3.6 20 7.8v8.4L12 20.4 4 16.2V7.8L12 3.6z"/><path d="m4 7.8 8 4.2 8-4.2M12 12v8.4"/>',
 arrow:'<path d="M4.5 12h14m0 0-5-5m5 5-5 5"/>',
 pencil:'<path d="M15.6 4.9 19 8.3 8.6 18.7l-4.1.8.8-4.1L15.6 4.9z"/>',
 globe:'<circle cx="12" cy="12" r="7.8"/><path d="M4.4 10h15.2M4.4 14h15.2"/><path d="M12 4.2c2 2.2 3 5 3 7.8s-1 5.6-3 7.8c-2-2.2-3-5-3-7.8s1-5.6 3-7.8z"/>',
 wrench:'<path d="M14.5 6.2a3.8 3.8 0 0 1 5.1 5.1l-2.4-.6-2.1-2.1-.6-2.4z"/><path d="M14.7 9.6 6.3 18a2.3 2.3 0 0 0 3.2 3.2l8.4-8.4"/>',
 file:'<path d="M13.5 3.8H7.2a1.4 1.4 0 0 0-1.4 1.4v13.6a1.4 1.4 0 0 0 1.4 1.4h9.6a1.4 1.4 0 0 0 1.4-1.4V8.3l-4.7-4.5z"/><path d="M13.4 3.9V8h4.6"/>',
 split:'<path d="M5 5h3.5l4 7 4-7H20"/><path d="M5 19h3.5l4-7"/><path d="m17 16.5 3 2.5-3 2.5"/>',
 merge:'<path d="M5 5h3.2l7.6 14H20M5 19h3.2l2.4-4.4"/><path d="m17 2.5 3 2.5-3 2.5"/>',
 dot:'<circle cx="12" cy="12" r="3.4" fill="currentColor" stroke="none"/>'
};
const I = (n,s=16)=>`<svg width="${s}" height="${s}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICN[n]||""}</svg>`;
const C = (cls,txt,ic)=>`<span class="chip ${cls}">${ic?I(ic,12):""}${txt}</span>`;
const F = (n,e,s)=>({n,e,s});

/* ============ settori serviti da Promatec ============ */
const SETT = {
 agri:["Agricoltura","var(--s-agri)"], off:["Off-highway","var(--s-off)"], lift:["Lift machinery","var(--s-lift)"],
 rail:["Railway","var(--s-rail)"], auto:["Automotive & moto","var(--s-auto)"], constr:["Construction","var(--s-constr)"],
 altri:["Altri settori","#6b6b64"]
};
const STAB = ["Stab. 1 · Via dei Falegnami 18","Stab. 2 · Via dei Carrai 12","Stab. 3 · Via Portella della Ginestra 4","Stab. 4 · Bevagna"];
/* provenienza di un documento */
const DOC = {
 mail:["has","Mail","allegato di una mail di questa conversazione"],
 "mail?":["prop","Mail ✓?","arrivato per mail, associazione ancora proposta: si conferma nella Distinta"],
 port:["has","Portale","scaricato dal portale del cliente e caricato dalla Distinta"],
 "port!":["port","Portale ⇩","il cliente lo indica sul portale: non ancora acquisito"],
 car:["has","Caricato","caricato a mano dalla Distinta"],
 ocr:["prop","Mail · OCR","PDF senza testo, in attesa di OCR: non è un file mancante"],
 no:["no","manca","nessuna copia in conversazione né sul portale"],
 na:["nr","—","non previsto per questo tipo di pezzo"]
};
const TIPIDOC=["3D","2D","DXF","CAP"], PRES=["mail","mail?","port","car","ocr"];

