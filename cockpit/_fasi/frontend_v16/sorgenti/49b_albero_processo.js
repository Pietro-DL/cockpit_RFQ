/* ================= 49b · l'albero del processo (zona B · R-10 R-32 R-37) =================
   SPEC_v16 §4.3. Lo STESSO linguaggio visivo dell'albero della Struttura (organigramma a caselle, linee, frecce,
   zoom «⤢ 100 % ▾», Apri tutto / Chiudi tutto, rastrello oltre 5 foglie), ma un COMPONENTE SEPARATO:
   - classi pp-*, mai ma-* né data-ma-*; sta fuori da #ma; è un role="tree", non una tablist (D-PF-14);
   - la struttura viene dalle FASI (chi entra in quale unione), non dalle relazioni della BOM;
   - le chiavi sono solo componente.id e fase.id (data-pp-cid, data-pp-fid): mai il Rif, mai «nodo:»;
   - un'unione è un ROMBO ◆, mai una casella: non è un nodo della BOM (FB §7.4); nessun trascinamento cambia la BOM.
   v16 rev.: il ramo che sale si legge DAL BASSO (la fase più vicina al rombo è quella che viene subito dopo l'unione,
   A-2/M4); le fasi sul ramo sono nodi dell'albero (tastiera, M-2); le frecce come nella Struttura e in motore_a.mjs (M-1);
   sui nodi la parola, il codice della regola nel title (M5); linee tratteggiate finché la sequenza non è confermata (M-6).
   Legge alberoProcesso(T,p) dal finto server A2 e disegna e basta (RV-28): stati e motivi sono del server. */

const PP_STATO = { ok:["✓","completo"], bozza:["◐","bozza"], proposta:["◇","proposta"], manca:["⚠","da sistemare"], vuota:["○","scegli chi entra"] };
/* M5: sul nodo la parola; il codice della regola resta nel title. Le regole sono di R-35 (P-15: senza R-35, niente ⚠) */
const PP_PAROLA = {"SEQ-01":"fuori da ogni fase", "SEQ-02":"terzista da scegliere", "SEQ-08":"quantità che non tornano", "SEQ-09":"scegli chi entra",
  "PREP-01":"preparazione senza costo", "PREP-02":"richiesta scaduta", "PREP-04":"fase tolta, richiesta aperta", "CIC-01":"ciclo vuoto", "COME-01":"come: da decidere"};
const STADIO_NOME = Object.fromEntries(STADI);
const ppChiave = n => n.tipo==="unione" ? `u|${n.cid}|${n.fid}` : n.tipo==="componente" ? `c|${n.cid}|${n.padre||""}` : n.tipo==="senza_fase" ? `s|${n.cid}` : `p|${n.cid}`;
const ppSceltoChiave = () => { const x = S.seqScelto; return !x ? "" : x.tipo==="unione" ? `u|${x.cid}|${x.fid}` : x.tipo==="stadio" ? `st|${x.cid}|${x.fid}` : x.tipo==="componente" ? `c|${x.cid}|${x.padre||""}` : x.tipo==="senza_fase" ? `s|${x.cid}` : `p|${x.cid}`; };
/* M-11: sulle caselle dei componenti lo stato del CICLO è il solo simbolo (◐ con il title); le parole restano per ✓ e ⚠ */
function ppStato(st, compatto){ const [s, w] = PP_STATO[st] || PP_STATO.bozza;
  if(compatto && st==="bozza") return `<span class="pp-st pp-st--bozza" title="ciclo in bozza"><span aria-hidden="true">◐</span><span class="vh">ciclo in bozza</span></span>`;
  return `<span class="pp-st pp-st--${st}" title="${esc(w)}">${s} ${esc(w)}</span>`; }
const ppLett = st => st ? `<b class="pp-lett" title="${esc(STADIO_NOME[st]||st)}">${esc(st)}</b>` : "";
/* «solo da sistemare»: restano i nodi ⚠ e i loro antenati */
function ppManca(n){ return n.stato==="manca" || (n.figli||[]).some(ppManca); }
/* una sigla di fase: porta al Ciclo con quella fase aperta. È una scorciatoia del mouse: il pannello dà le stesse azioni */
function ppSigla(cid, f){
  const cls = f.est ? " pp-sg--est" : "";
  return `<span class="pp-sg${cls}" aria-hidden="true" data-vai="3|${cid}|${f.fid}" title="f.${f.N} ${esc(f.nome)}${f.est?` · esterna · ${esc(f.terzista||"terzista da scegliere")}`:""}${f.proteggi?" · da proteggere":""}">${f.est?"⇄ ":""}${esc(f.sigla)}${f.est&&f.terzista?` · ${esc(f.terzista)}`:""}</span>`;
}
function ppCatena(cid, fasi){ return fasi.length ? `<span class="pp-catena" title="${esc(fasi.map(f=>`f.${f.N} ${f.nome}`).join(" → "))}">${fasi.map(f=>ppSigla(cid,f)).join(`<i aria-hidden="true">▸</i>`)}</span>` : ""; }
/* la chiave inglese con la PAROLA dello stato: esiste già · da quotare · quotata. Porta alla riga nel Ciclo */
function ppPrep(cid, fid, x){
  const cl = x.stato==="esiste già" ? "off" : x.stato==="quotata" ? "ok" : "ver";
  return `<span class="pp-prep pp-prep--${cl}" aria-hidden="true" data-vai="3|${cid}|${fid||""}|prep:${esc(x.key)}" title="${esc(x.nome)}${x.costo?` · ${esc(x.costo)} €`:""}${x.settimane?` · ${esc(x.settimane)} sett.`:""}">${I("wrench",12)} ${esc(x.nome.replace(/ di saldatura$/,""))} · ${esc(x.stato)}</span>`;
}
/* il ramo che sale dal rombo verso il padre: le fasi del padre fra questa unione e la prossima. Si legge DAL BASSO (A-2):
   la fase più vicina al rombo è la prima dopo l'unione; in alto la freccia ↑ dice il verso. Ogni fase è un nodo (M-2). */
function ppRamo(cid, fasi, liv){
  if(!fasi || !fasi.length) return "";
  const sel = ppSceltoChiave();
  return `<div class="pp-ramo" role="group" aria-label="Fasi dopo l'unione, dal basso verso l'alto"><span class="pp-ramo-su" aria-hidden="true" title="il flusso sale: dal rombo verso il padre">↑</span>${fasi.slice().reverse().map(f=>{ const k = `st|${cid}|${f.fid}`;
    return `<span class="pp-ramo-f${f.est?" pp-ramo-f--est":""}${f.est&&!f.terzista?" pp-ramo-f--manca":""}${sel===k?" pp-ramo-f--su":""}" role="treeitem" aria-level="${liv}" aria-selected="${sel===k}" tabindex="${sel===k?0:-1}"
      data-pp-nodo="${esc(k)}" data-pp-tipo="stadio" data-pp-cid="${esc(cid)}" data-pp-fid="${esc(f.fid)}" data-pp-stadio="${cid}|${f.fid}" title="${esc(f.nome)}${f.trasporto?" · "+esc(f.trasporto):""} · Invio o clic: il pannello dello stadio"
      aria-label="fase ${f.N} ${esc(f.nome)}${f.est?` · esterna · ${esc(f.terzista||"terzista da scegliere")}`:""}">
    ${ppLett(f.st)} ${f.est?"⇄ ":""}f.${f.N} ${esc(f.nome)}${f.est?` · ${esc(f.terzista||"⚠ terzista da scegliere")}${f.trasporto?` · ${I("truck",12)} ${esc(f.trasporto.replace(" ◇",""))} ◇`:""}`:""}${f.proteggi?` · da proteggere: ${esc(f.proteggi.split(",").length)}`:""}</span>`; }).join("")}</div>`;
}
function ppMot(n){
  if(!(n.motivi||[]).length) return "";
  const m = n.motivi[0], w = PP_PAROLA[m.cod] || m.testo;
  return `<span class="pp-mot" title="${esc(n.motivi.map(x=>x.cod+" "+x.testo).join(" · "))}">${esc(w)}${n.motivi.length>1?` +${n.motivi.length-1}`:""}</span>`;
}
function ppBox(n, liv){
  const sel = ppSceltoChiave()===ppChiave(n), chiave = ppChiave(n);
  const attr = `role="treeitem" aria-level="${liv}" aria-selected="${sel}" tabindex="${sel?0:-1}" data-pp-nodo="${esc(chiave)}" data-pp-tipo="${n.tipo}" data-pp-cid="${esc(n.cid||"")}"${n.fid?` data-pp-fid="${esc(n.fid)}"`:""}${n.padre?` data-pp-padre="${esc(n.padre)}"`:""}`;
  const mot = ppMot(n);
  if(n.tipo==="prodotto"){
    return `<div class="pp-box pp-box--prodotto${sel?" pp-box--su":""}" ${attr} aria-label="Prodotto ${esc(n.cod)} · ${esc((PP_STATO[n.stato]||[])[1]||"")}">
      <span class="pp-riga1"><span class="pp-tipo">prodotto</span><b class="mono">${esc(n.cod)}</b>${ppStato(n.stato)}</span><span class="pp-descr">${esc(n.descr||"")}</span>${mot}</div>`;
  }
  if(n.tipo==="unione"){
    return `<div class="pp-unione${sel?" pp-unione--su":""}${n.preUnione?" pp-unione--pre":""} pp-u--${n.stato}" ${attr} aria-label="Fase ${n.N} ${esc(n.nome)}${n.preUnione?" · pre-unione":""} · ${n.figli.length} entrano · ${esc((PP_STATO[n.stato]||[])[1]||"")}">
      <span class="pp-rombo" aria-hidden="true"></span>
      <span class="pp-u-t">${ppLett(n.stadio)} <b>f.${n.N} ${esc(n.nome)}</b>${n.preUnione?` <span class="pp-tag pp-tag--pre">pre-unione</span>`:""}${n.maschera?` <span class="pp-tag">in maschera</span>`:""}</span>
      <span class="pp-u-d">${n.stato==="vuota"?"":n.rif?`rif <span class="mono">${esc(n.rif.cod.replace(/^F\+800\.514263/,""))}</span>${n.rif.fonte==="CALC"?" ◇":""}`:"rif —"}${ppStato(n.stato)}${n.stato==="vuota"?"":mot}</span>
      ${(n.prep||[]).length?`<span class="pp-u-p">${n.prep.map(x=>ppPrep(n.cid, n.fid, x)).join("")}</span>`:""}
      ${(n.prima||[]).length?`<span class="pp-u-d">prima: ${n.prima.map(f=>ppSigla(n.cid,f)).join(" ")}</span>`:""}</div>`;
  }
  if(n.tipo==="senza_fase"){
    return `<div class="pp-box pp-box--senza${sel?" pp-box--su":""}" ${attr} aria-label="Non entrano in nessuna fase: ${esc(n.cods.join(", "))}">
      <span class="pp-riga1">⚠ <b>non entra in nessuna fase</b></span><span class="pp-descr" title="SEQ-01">${n.cods.length} ${n.cods.length===1?"figlio":"figli"}: scegli il padre e falli entrare</span></div>`;
  }
  /* componente o sottoassieme */
  const tipoT = n.comm ? "commerciale" : n.sottoassieme ? "sottoassieme" : "sciolto";
  const chiuso = (S.ppChiusi||new Set()).has(chiave);
  const sotto = n.sottoassieme ? `<span class="pp-apri" aria-hidden="true" data-pp-apri="${esc(chiave)}" title="${chiuso?"Apri":"Chiudi"} il sottoassieme (+ e −)">${chiuso?"▸":"▾"} ${n.figli.reduce((a,x)=>a+(x.figli||[]).length,0)} sotto</span>` : "";
  const qta = n.diviso ? `×${n.qta} di ${n.qtaBom}` : `×${n.qta}`;
  return `<div class="pp-box pp-box--comp pp-t--${tipoT}${sel?" pp-box--su":""}${n.stato==="manca"?" pp-box--manca":""}" ${attr} aria-label="${esc(n.cod)} ${esc(qta.replace("×","quantità "))} · ${n.stato==="bozza"?"ciclo in bozza":esc((PP_STATO[n.stato]||[])[1]||"")}${(n.fasi||[]).length?" · ciclo "+esc(n.fasi.map(f=>f.nome).join(", ")):""}">
    <span class="pp-riga1"><b class="mono pp-cod" title="${esc(n.cod)} · ${esc(n.descr||"")}">${esc(n.cod.replace(/^F\+800\.514263/,"…"))}</b><span class="pp-qta">${esc(qta)}</span>${n.fonte==="proposta"?`<span class="pp-pr" title="${esc(n.perche||"proposta del sistema")}">◇</span>`:""}${ppStato(n.stato, true)}</span>
    ${n.comm?`<span class="pp-descr">comm. · lo compra Fabio</span>`:n.cliente?`<span class="pp-descr">dal cliente · controllo in accettazione</span>`:ppCatena(n.cid, n.fasi||[])}
    ${(n.prep||[]).length && !n.sottoassieme?`<span class="pp-u-p">${n.prep.map(x=>ppPrep(n.cid, x.fid, x)).join("")}</span>`:""}
    ${(n.anche||[]).length?`<span class="pp-anche" title="lo stesso codice entra anche sotto ${esc(n.anche.join(", "))}">⧉ anche sotto ${esc(n.anche.map(c=>c.slice(-6)).join(", "))}</span>`:""}${mot}${sotto}</div>`;
}
/* un livello dell'organigramma; le foglie oltre 5 vanno nel rastrello (una colonna con il pettine) */
function ppLi(n, liv){
  const chiuso = (S.ppChiusi||new Set()).has(ppChiave(n));
  const figli = (S.seqFiltro==="sistemare" ? (n.figli||[]).filter(ppManca) : (n.figli||[]));
  const gid = "ppg-"+ppChiave(n).replace(/[^a-zA-Z0-9-]/g,"_");
  const ramo = n.tipo==="unione" ? ppRamo(n.cid, n.trasu, liv) : "";
  let sotto = "";
  if(figli.length && !chiuso){
    const foglie = figli.filter(x=>x.tipo==="componente" && !x.sottoassieme), altri = figli.filter(x=>!(x.tipo==="componente" && !x.sottoassieme));
    const rastrello = foglie.length>5;
    sotto = `<ul class="pp-ul" role="group" id="${gid}">${altri.map(x=>ppLi(x, liv+1)).join("")}${rastrello
      ? `<li class="pp-li pp-li--rastrello" role="none"><ul class="pp-rastrello" role="none" style="--col:${Math.min(4, Math.ceil(Math.sqrt(foglie.length)))}">${foglie.map(x=>`<li class="pp-li" role="none">${ppBox(x, liv+1)}</li>`).join("")}</ul></li>`
      : foglie.map(x=>ppLi(x, liv+1)).join("")}</ul>`;
  }
  return `<li class="pp-li${n.tipo==="unione"?" pp-li--u":""}" role="none">${ramo}${ppBox(n, liv).replace('role="treeitem"', `role="treeitem"${figli.length?` aria-expanded="${!chiuso}"${!chiuso?` aria-owns="${gid}"`:""}`:""}`)}${sotto}</li>`;
}
/* la catena delle unioni porta su ogni rombo le fasi che lo SEGUONO fino alla prossima unione: il ramo che sale.
   Le ultime fasi del prodotto (anche il controllo finale e l'imballo) e quelle di un sottoassieme stanno sul ramo della loro
   ultima unione: così sono tutte nodi dell'albero, raggiungibili da tastiera (M-2) */
function ppPrepara(n, sopra){
  if(!n) return n;
  if(n.tipo==="prodotto"){ const top = (n.figli||[]).find(x=>x.tipo==="unione"); if(top) top.trasu = (n.ultime||[]); }
  if(n.tipo==="componente" && n.sottoassieme){ const top = (n.figli||[]).find(x=>x.tipo==="unione"); if(top){ top.trasu = (n.dopo||[]); n.fasi = []; } }
  if(n.tipo==="unione"){ const giu = (n.figli||[]).find(x=>x.tipo==="unione"); if(giu) giu.trasu = giu.tra; }
  (n.figli||[]).forEach(x=>ppPrepara(x, n));
  return n;
}
function alberoProcessoHtml(T, p, A){
  if(!A) return "";
  ppPrepara(A, null);
  const z = S.ppZoom || 1;
  let html = `<div class="pp-tela" id="pp-tela"><div class="pp-zoom" style="--z:${z}">
    <ul class="pp-albero${A.confermata?" pp-albero--conf":""}" role="tree" aria-label="Albero del processo di ${esc(A.cod)}" aria-describedby="pp-tasti">${ppLi(A, 1)}</ul></div></div>
    <p class="vh" id="pp-tasti">Come la Struttura: freccia giù va al nodo sotto, freccia su al nodo sopra, sinistra e destra ai nodi accanto; Home e Fine al primo e all'ultimo; più e meno aprono e chiudono; Invio apre il pannello; Maiusc più Invio apre il ciclo.</p>`;
  /* M-2, M-3: senza un nodo scelto il primo nodo entra nel giro del Tab, come motore_a.mjs (preparaTabindex) */
  if(!/role="treeitem"[^>]*tabindex="0"/.test(html) && !/tabindex="0"[^>]*data-pp-nodo/.test(html)) html = html.replace('aria-selected="false" tabindex="-1"', 'aria-selected="false" tabindex="0"');
  return html;
}
/* lista piatta dei nodi visibili (per la tastiera): l'ordine del DOM */
function ppNodiVisibili(){ return [...document.querySelectorAll(".pp-albero [data-pp-nodo]")]; }
/* la catena verticale di un li: le fasi del ramo (dall'alto) e poi la casella o il rombo */
const ppCatenaLi = li => li ? [...li.querySelectorAll(":scope > .pp-ramo [data-pp-nodo]"), ...li.querySelectorAll(":scope > [data-pp-nodo]")] : [];
const ppFigliLi = li => li ? [...li.querySelectorAll(":scope > ul > li.pp-li, :scope > ul > li.pp-li--rastrello > ul > li.pp-li")].filter(x=>!x.classList.contains("pp-li--rastrello")) : [];
const ppFratelliLi = li => { if(!li) return []; const su = li.parentElement && li.parentElement.closest("li.pp-li:not(.pp-li--rastrello)"); return su ? ppFigliLi(su) : [li]; };
function ppScegliDa(el){
  if(!el) return; const D = el.dataset;
  S.seqScelto = {tipo:D.ppTipo, cid:D.ppCid||null, fid:D.ppFid||null, padre:D.ppPadre||null};
  render(); const x = ppNodiVisibili().find(y=>y.dataset.ppNodo===D.ppNodo); if(x) ppMostra(x);
}
/* A-1, M9: il nodo scelto prende il fuoco senza far saltare la tela; se è fuori vista, la tela lo centra */
function ppMostra(x){
  try{ x.focus({preventScroll:true}); }catch(e){ try{ x.focus(); }catch(e2){} }
  const tela = x.closest(".seq16-tela"); if(!tela || !x.getBoundingClientRect) return;
  const a = x.getBoundingClientRect(), b = tela.getBoundingClientRect();
  const fuori = a.left < b.left || a.right > b.right || a.top < b.top || a.bottom > b.bottom;
  if(fuori && x.scrollIntoView) try{ x.scrollIntoView({block:"center", inline:"center"}); }catch(e){}
}
/* M-1: i tasti come la Struttura e motore_a.mjs (tastiAlbero): ↓ il nodo sotto (apre se chiuso), ↑ quello sopra,
   ← → i nodi accanto, Home e Fine, + e −. Invio sceglie; Maiusc+Invio apre il ciclo del nodo */
function tastiAlbero(e){
  const vis = ppNodiVisibili(); if(!vis.length) return false;
  const k = e.key, el = vis.find(x=>x.dataset.ppNodo===ppSceltoChiave()) || (document.activeElement && document.activeElement.dataset && document.activeElement.dataset.ppNodo ? document.activeElement : null);
  if(!el){ if(["ArrowDown","ArrowUp","ArrowLeft","ArrowRight","Home","End"].includes(k)){ ppScegliDa(k==="End"?vis[vis.length-1]:vis[0]); return true; } return false; }
  const li = el.closest("li.pp-li"), cat = ppCatenaLi(li), i = cat.indexOf(el);
  const box = l => l ? l.querySelector(":scope > [data-pp-nodo]") : null;
  if(k==="ArrowUp"){ if(i>0) ppScegliDa(cat[i-1]); else { const su = li.parentElement && li.parentElement.closest("li.pp-li:not(.pp-li--rastrello)"); if(su) ppScegliDa(box(su)); } return true; }
  if(k==="ArrowDown"){ if(i<cat.length-1){ ppScegliDa(cat[i+1]); return true; }
    if(el.getAttribute("aria-expanded")==="false"){ const s = S.ppChiusi || (S.ppChiusi = new Set()); s.delete(el.dataset.ppNodo); render(); }
    const l2 = document.querySelector(`.pp-albero [data-pp-nodo="${(window.CSS&&window.CSS.escape)?window.CSS.escape(el.dataset.ppNodo):el.dataset.ppNodo}"]`), li2 = l2 && l2.closest("li.pp-li"), f = ppFigliLi(li2)[0];
    if(f) ppScegliDa(ppCatenaLi(f)[0]); return true; }
  if(k==="ArrowLeft"||k==="ArrowRight"){ const fr = ppFratelliLi(li), j = fr.indexOf(li), d = fr[j + (k==="ArrowRight"?1:-1)]; if(d) ppScegliDa(box(d)); return true; }
  if(k==="Home"){ ppScegliDa(vis[0]); return true; }
  if(k==="End"){ ppScegliDa(vis[vis.length-1]); return true; }
  if((k==="+"||k==="-") && el.hasAttribute("aria-expanded")){ const s = S.ppChiusi || (S.ppChiusi = new Set()); if(k==="-") s.add(el.dataset.ppNodo); else s.delete(el.dataset.ppNodo); render(); const x = ppNodiVisibili().find(y=>y.dataset.ppNodo===el.dataset.ppNodo); if(x) ppMostra(x); return true; }
  if(k==="Enter" && !e.shiftKey){ ppScegliDa(el); return true; }
  if(k==="Enter" && e.shiftKey && el.dataset.ppCid){ S.staz = 3; S.cicloCid = el.dataset.ppCid; S.cicloRif = null; S.disCiclo = "componente"; S.faseAperta = el.dataset.ppFid || undefined; PA.avviso = null; render(); return true; }
  return false;
}
document.addEventListener("click", e=>{
  const ap = e.target.closest && e.target.closest("[data-pp-apri]");
  if(ap){ const s = S.ppChiusi || (S.ppChiusi = new Set()); const k = ap.dataset.ppApri; if(s.has(k)) s.delete(k); else s.add(k); render(); e.stopPropagation(); return; }
  if(e.target.closest && e.target.closest("[data-vai],[data-pp-stadio]")) return;   /* le sigle e le chiavi inglesi portano al Ciclo (48); gli stadi li sceglie 49 */
  const n = e.target.closest && e.target.closest(".pp-albero [data-pp-nodo]");
  if(n){ const gia = n.dataset.ppNodo===ppSceltoChiave(); if(gia && e.detail>1) return; ppScegliDa(n); }
});
/* M9: entrando nella stazione, se l'albero è più largo della tela, «Adatta» da solo (una volta per prodotto) */
function adattaAlberoProcesso(){
  if(S.staz!==4) return;
  const T = threadAperto(), p = T ? prodottoScelto(T) : null, chiave = (T?T.key:"")+"|"+(p?p.rif:"");
  if(S.ppAdattato===chiave) return;
  const tela = document.querySelector(".seq16-tela"); if(!tela || !tela.clientWidth) return;
  S.ppAdattato = chiave;
  const z = zoomAdatta(); if(z < (S.ppZoom||1)){ S.ppZoom = z; render(); }
}
/* «Adatta alla larghezza», come nella Struttura (zoom CSS, non transform: la tela scorre davvero sul contenuto) */
function zoomAdatta(){
  const tela = document.querySelector(".seq16-tela"); if(!tela) return 1;
  const largo = tela.scrollWidth / (S.ppZoom||1); if(!largo || !tela.clientWidth) return 1;
  return Math.max(.5, Math.min(1, Math.floor((tela.clientWidth-28)/largo*20)/20));
}
