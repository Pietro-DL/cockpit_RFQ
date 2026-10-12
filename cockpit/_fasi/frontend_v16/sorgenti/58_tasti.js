/* ================= 58 · i tasti per stazione (SPEC §2.0, §2.3, §2.5, §2.6, §2.7; FLUSSO §10) =================
   Mai Ctrl+S, F o P. Le lettere funzionano solo FUORI dai campi; dentro i campi valgono Invio, Maiusc+Invio, Esc e ⌃↵.
   Nessun tasto esegue un gesto della zona A: le scritture A partono solo con un clic e la loro domanda.
   I tasti della zona B chiamano gli stessi gesti B dei pulsanti (data-pa-gesto), mai una rotta A. */

const TASTI_STAZ = {
  1:[["← → ↑ ↓","nell'albero: fra le caselle"],["Invio","apre la casella"],["[ ]","scheda dell'albero prima o dopo"]],
  2:[["1…6","come si fa: noi · lamiera comprata · fuori · dal cliente · escluso · no quote"],["Invio","accetta la proposta ◇ e scende"],["↑ ↓","riga"],["Spazio","seleziona la riga"],["Maiusc + ↑ ↓","estende la selezione"],["A","applica ai selezionati (anteprima del blocco)"],["M","modello di ciclo"],["/","cerca"]],
  5:[["↑ ↓","riga"],["O B X","OK · DUBBIO · KO, poi la nota"],["⌃↵","consegna a Franco"]],
};
/* v16 (SE regola 4, regola 8): le legende dei tasti e le spiegazioni lunghe della zona B escono dalla pagina e stanno qui,
   nella scheda «Tasti» del «?» unico (cassetto «Aiuto · non ancora nel backend»). C2 registra [3] e [4] in AIUTO_STAZ. */
AIUTO_STAZ[1] = {tasti: TASTI_STAZ[1], testi: [
  "Documenti e Distinta sono una pagina sola: in alto i Documenti e «Da smistare», sotto l'Analisi del prodotto (lo STEP), la Conferma dell'albero e l'albero; a destra l'anteprima della casella. I collegamenti «Documenti» e «Distinta» scorrono alla loro parte.",
  "Le tre conferme restano in tre posti: la fonte in «Analisi del prodotto», l'albero in «Conferma dell'albero», i documenti voce per voce. Con la BOM verificata, nella barra delle stazioni compare «Poi: ② Come →».",
  "Le aggiunte di pezzi non entrano in questa pagina (DB7-5, DB7-7). Nella zona B c'è la proposta: «Manca nell'albero» (R-38) e «Diventa assieme saldato» (R-31), nella Sequenza.",
  "«✉ Chiedi al cliente» è un collegamento della zona B (R-41): la bozza nasce nel cassetto Richieste e la manda il commerciale."], richieste: "R-41 · R-38 · R-31"};
AIUTO_STAZ[2] = {tasti: TASTI_STAZ[2], testi: [
  "Una riga per codice distinto. I numeri dentro i pulsanti sono i tasti: 1 noi · 2 lamiera comprata · 3 fuori · 4 dal cliente · 5 escluso · 6 no quote.",
  "Il backend di oggi non ha il modo di fornitura né i modelli di ciclo: quello che scrivi qui resta nel mockup.",
  "Tipo e categoria si leggono dalla zona A (struttura confermata) e qui non si cambiano: «cambia in ① →».",
  "Le richieste si preparano nel cassetto ✉, una per fornitore (le richieste di Pietro, Blocco 7B): «Apri le richieste da preparare» ci porta. Niente parte da solo (DB7-6): la mail la mandi da Outlook."], richieste: "7B · R-22 · R-39 · R-13 · R-27"};
AIUTO_STAZ[5] = {tasti: TASTI_STAZ[5], testi: [
  "La proposta ◇ di ogni riga viene dalle regole (R-35): diventa piena solo quando decidi tu. «Accetta le N proposte OK…» elenca le righe prima di scrivere.",
  "Le firme del Mod 99: ognuno firma la sua area con il suo ruolo (Lino l'area tecnica, Marta la produzione, Fabio gli acquisti).",
  "La consegna fotografa la Distinta per ogni lotto spuntato e avvisa Franco: non tocca la fase registrata, l'albero, la fonte o i documenti.",
  "Attrezzatura: SI quando ogni preparazione ha un costo o «esiste già». «Richiedi le N preparazioni senza costo» apre il cassetto ✉, dove prepari la bozza per ogni fornitore; non parte niente da solo.",
  "Minuteria a norma: la conferma della categoria arriva con lo spazio di verifica (R-08, zona C). Fino ad allora DOC-01 pesa: si chiede il 2D al cliente."], richieste: "D-01 · D-02 · R-35 · R-40 · R-11 · R-13"};
/* la scheda «Tasti» del «?»: i tasti globali, quelli della stazione e le spiegazioni lunghe */
function tastiHtml(){
  const A = AIUTO_STAZ[S.staz] || {}, l = A.tasti || TASTI_STAZ[S.staz] || [];
  const k = x => x.split(" ").map(y=>y.length<=3||/[↑↓←→]/.test(y)?`<kbd>${esc(y)}</kbd>`:esc(y)).join(" ");
  return `<div class="tasti" aria-label="Tasti">
    <dl class="tasti-dl"><dt><kbd>G</kbd> poi <kbd>1</kbd>…<kbd>5</kbd></dt><dd>stazione</dd><dt><kbd>R</kbd> <kbd>K</kbd> <kbd>L</kbd> <kbd>I</kbd></dt><dd>cassetti Richieste, Controlli, Cataloghi, Non ancora nel backend</dd>
      <dt><kbd>?</kbd></dt><dd>questo aiuto</dd><dt><kbd>Esc</kbd></dt><dd>chiude: domanda → tendina → anteprima → cassetto → fase aperta</dd>
      ${l.map(([t,d])=>`<dt>${k(t)}</dt><dd>${esc(d)}</dd>`).join("")}</dl>
    ${(A.testi||[]).length?`<ul class="cas-elenco aiuto-testi">${A.testi.map(t=>`<li>${esc(t)}</li>`).join("")}</ul>`:""}
    ${A.richieste?`<p class="tendina-nota">Richieste a Pietro per questa stazione: ${esc(A.richieste)}</p>`:""}
    <p class="tendina-nota">Nessun tasto esegue un gesto della zona A: le scritture partono solo con un clic sul pulsante e la sua domanda.</p></div>`;
}

/* ---------------- dopo ogni disegno: il fuoco dove lo chiede il tasto (presentazione) ---------------- */
function dopoRender(){
  if(S.fuoco){ const el = document.querySelector(S.fuoco); S.fuoco = null; if(el && el.focus){ el.focus(); if(el.select && el.tagName==="INPUT") try{ el.select(); }catch(e){} } }
}
function campoDopo(el){
  const tutti = [...document.querySelectorAll("[data-pa-campo],[data-pa-grezzo]")].filter(x=>x.tagName==="INPUT");
  const i = tutti.indexOf(el); const nx = tutti[i+1];
  if(!nx) return null;
  return nx.dataset.paCampo ? `[data-pa-campo="${nx.dataset.paCampo}"]` : `[data-pa-grezzo="${nx.dataset.paGrezzo}"]`;
}
function cliccaSe(sel){ const b = document.querySelector(sel); if(b && !b.disabled){ b.click(); return true; } return false; }

/* ---------------- ⌃↵ e Invio dentro i campi (prima del gestore globale) ---------------- */
document.addEventListener("keydown", e=>{
  if(S.tab!=="rfq") return;
  const t = e.target;
  if(e.key==="Enter" && (e.ctrlKey || e.metaKey)){
    const sel = {3:"[data-conferma-ciclo]", 4:"[data-conferma-seq]", 5:"[data-consegna]"}[S.staz];
    if(sel){ e.preventDefault(); if(t && t.dataset && t.dataset.paCampo) scriviCampoVista(t); cliccaSe(sel); }
    return;
  }
  if(e.key==="Enter" && e.shiftKey && S.staz===3){ e.preventDefault(); const a = document.querySelector(".fase--aperta [data-accetta-fase]"); if(a) a.click(); return; }
  if(e.key==="Enter" && t && t.dataset && t.dataset.paCampo && t.tagName==="INPUT"){ e.preventDefault(); const nx = campoDopo(t); S.fuoco = nx; scriviCampoVista(t); if(!PA.avviso || PA.avviso.esito==="ok") dopoRender(); return; }
  if(e.key==="Enter" && t && t.dataset && t.dataset.paGrezzo){ e.preventDefault(); const nx = campoDopo(t); S.fuoco = nx; const v = t.value.trim(), fid = t.dataset.paGrezzo;
    if(t.dataset.prop!==undefined && v===t.dataset.prop) eseguiB(`/fase/${fid}/materiale`, {accetta:"1"}); else eseguiB(`/fase/${fid}/materiale`, {cerca:v}); dopoRender(); return; }
  if(e.key==="Enter" && t && t.dataset && t.dataset.faseApri){ e.preventDefault(); t.click(); return; }
  if(e.key==="Escape" && t && t.id==="come-form-testo"){ S.comeForm = null; render(); e.preventDefault(); e.stopPropagation(); return; }
}, true);

/* ---------------- le lettere fuori dai campi, per stazione (chiamate dal gestore globale di 40_guscio) ---------------- */
function tastiStazione(e){
  if(S.staz===2) return tastiCome(e);
  if(S.staz===3 && typeof tastiCiclo==="function") return tastiCiclo(e);
  if(S.staz===4 && typeof tastiSequenza==="function") return tastiSequenza(e);
  if(S.staz===5) return tastiEsito(e);
}
function tastiCome(e){
  const T = threadAperto(), p = prodottoScelto(T), vis = S._comeVis || [], k = e.key;
  const i = vis.indexOf(S.comeRiga);
  if(S.blocco && S.blocco.staz===2){
    if(/^[1-4]$/.test(k)){ S.blocco.modo = MODI[+k-1][0]; render(); e.preventDefault(); return; }
    if(k==="Enter"){ if(S.blocco.modo) eseguiB(`/rfq/${T.id}/come/blocco`, {cids:S.blocco.cids.join(","), modo:S.blocco.modo}); e.preventDefault(); return; }
    return;
  }
  if(k==="ArrowDown"||k==="ArrowUp"){ const j = Math.max(0, Math.min(vis.length-1, i + (k==="ArrowDown"?1:-1))); if(e.shiftKey && S.comeRiga){ (S.comeSel||(S.comeSel=new Set())).add(S.comeRiga); if(vis[j]) S.comeSel.add(vis[j]); }
    S.comeRiga = vis[j] || S.comeRiga; render(); e.preventDefault(); return; }
  if(!S.comeRiga) return;
  const r = rigaB(T,p,S.comeRiga); if(!r) return;
  if(/^[1-6]$/.test(k)){ const modo = MODI[+k-1][0]; e.preventDefault();
    if(modo==="escluso"||modo==="noquote"){ S.comeForm = {modo, cid:r.cid}; S.fuoco = "#come-form-testo"; render(); return; }
    eseguiB(`/componente/${r.cid}/fornitura`, MODO_CON_CICLO.has(modo) ? {modo, conModello:"1"} : {modo}); return; }
  if(k==="Enter"){ e.preventDefault(); const pr = modoComponente(T,p,r); if(pr.modo==="comm") return; const nx = vis[i+1] || vis[i-1] || null;
    const res = eseguiB(`/componente/${r.cid}/fornitura`, MODO_CON_CICLO.has(pr.modo) ? {modo:pr.modo, conModello:"1"} : {modo:pr.modo, testo:pr.perche});
    if(res.esito==="ok" && nx){ S.comeRiga = nx; render(); } return; }
  if(k===" "){ const s = S.comeSel || (S.comeSel = new Set()); if(s.has(r.cid)) s.delete(r.cid); else s.add(r.cid); render(); e.preventDefault(); return; }
  if(k==="a"||k==="A"){ const s = S.comeSel||new Set(); if(s.size){ S.blocco = {staz:2, cids:[...s], modo:null}; render(); } else { PA.avviso = {esito:"no", testo:"Seleziona prima le righe (Spazio), poi A."}; render(); } e.preventDefault(); return; }
  if(k==="m"||k==="M"){ const c = comeDi(T,r.cid); if(c && MODO_CON_CICLO.has(c.modo)){ S.menuMod = r.cid; render(); } e.preventDefault(); return; }
  if(k==="/"){ e.preventDefault(); const c = document.getElementById("come-cerca"); if(c) c.focus(); return; }
}
/* tastiCiclo e tastiSequenza li definisce C2 in 48_ciclo.js e 49_sequenza.js (SPEC_v16 §8.3) */
function tastiEsito(e){
  if(S.esitoVista && S.esitoVista!=="esito") return;
  const T = threadAperto(), p = prodottoScelto(T), righe = righeB(T,p).filter(r=>r.cid), vis = righe.map(r=>r.cid), i = vis.indexOf(S.esitoRiga), k = e.key;
  if(S.esitoAnteprima && k==="Enter"){ cliccaSe(".blocco-b [data-pa-gesto]"); e.preventDefault(); return; }
  if(k==="ArrowDown"||k==="ArrowUp"){ S.esitoRiga = vis[Math.max(0, Math.min(vis.length-1, i + (k==="ArrowDown"?1:-1)))]; render(); e.preventDefault(); return; }
  const v = {o:"OK", b:"DUBBIO", x:"KO"}[k.toLowerCase()];
  if(v && S.esitoRiga){ S.fuoco = `[data-esito-nota="${S.esitoRiga}"]`; eseguiB(`/componente/${S.esitoRiga}/fattibilita`, {esito:v}); dopoRender(); e.preventDefault(); return; }
}
