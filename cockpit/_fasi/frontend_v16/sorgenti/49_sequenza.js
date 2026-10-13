/* ================= 49 · stazione ④ Sequenza (zona B · R-10 R-32 R-33 R-35 R-36 R-37 · R-31 R-38) =================
   SPEC_v16 §4. La Sequenza È l'albero del processo (49b), con un pannello a destra per correggere e la barra in basso.
   - Il dato: le fasi del padre (prodotto e sottoassiemi) e chi entra in quale fase (R-10 fase_componente). Gli stadi
     P · U · F · T · D · C si LEGGONO dalla lavorazione (R-12, R-32), non si salvano.
   - La proposta è un seme del server (SEQ-06): per i prodotti facili si conferma e basta. Per quelli complessi Lino la
     scrive tutta: aggiunge, toglie, rinomina, riordina stadi, sposta figli fra fasi, crea pre-unioni. OGNI gesto è una
     scrittura (R-36): nessuna bozza nella pagina. Appena scrive, il server non ripropone più («scritta da te»).
   - «Conferma la sequenza ⌃↵» manda la FIRMA di quello che si è letto, come «Conferma l'albero» (FB §6.5).
   - Un pezzo intermedio con un codice di magazzino non è una pre-unione: è un sottoassieme (R-31, «Proposte d'albero»).
     Il codice è VUOTO, senza segnaposto con un codice (DB7-7): lo scrive una persona. «L'albero non cambia».
   bloccoEntrano() e rigaMaschera() sono lo STESSO pezzo di interfaccia nel Ciclo (48) e qui (§4.7). */

const SEQ_STADI_AGG = ["foratura","raddrizzatura","sabbiatura","verniciatura_polvere","zincatura","controllo","montaggio","piantaggio","sald_mag","imballo"];
const SEQ_UNIONI_AGG = ["sald_mag","sald_robot","sald_tig","prigionieri","montaggio","piantaggio"];
const sufSeq = (T, p, cod) => { const pre = prefissoComune(righeB(T,p).map(r=>r.cod)); return suffisso(cod, pre); };
const nomeFaseSeq = (c, f) => `f.${(c.fasi.indexOf(f)+1)*10} ${valF(f,"nome") || (PROC[f.p]?PROC[f.p][0]:f.p)}`;

/* ---------------- i pezzi condivisi con il Ciclo ---------------- */
/* «Richiedi» di una preparazione: maschera e «altra» hanno due destinatari (Interno · Fornitore), le altre uno solo */
function richiediPrep(T, r, key, v){
  if(v.stato==="esiste già") return "";
  /* il legame preparazione → richiesta si LEGGE (P-10): le righe che la portano nelle note */
  const qq = richiesteDellaPrep(T, r.cid, key).filter(x=>x.stato!=="annullata");
  if(qq.length){ const q = qq[0]; return `<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-rq-apri="${esc(q.forn?(q.bisogno||"attrezzatura"):q.id)}" title="Apri nel cassetto ✉">${esc(q.id)}${qq.length>1?" +"+(qq.length-1):""} · ${esc(NOMI_STATO_RQ[q.stato]||q.stato)} ✉</button>`; }
  const rich = (PREP_INFO[v.k]||PREP_INFO.altra).rich, base = {cid:r.cid, fid:v.nata||"", prep:key};
  const interno = t => btnB(`/rfq/${T.id}/domanda`, Object.assign({tipo:t}, base), rich.length>1?"Interno":"Richiedi", "", `title="Bozza in Outlook a ${esc(TIPI_RICHIESTA[t].nomeDest)} (proposta R-28): costo e tempi. Niente parte da solo."`);
  const forn = apriRichiesta("attrezzatura", rich.length>1?"Fornitore":"Richiedi", "");
  if(rich.length>1) return `<span class="prep-rich"><span class="k3">Richiedi:</span>${interno(rich[0])}${forn}</span>`;
  return rich[0]==="attrezzatura" ? forn : interno(rich[0]);
}
/* sotto «in maschera»: una riga sola con lo stato della SUA maschera e «Richiedi» */
function rigaMaschera(T, p, r, c, f){
  if(valF(f,"maschera")!=="sì") return "";
  const x = prepVoci(c).find(([,v])=>v.k==="maschera" && v.nata===f.id && v.on);
  if(!x) return `<span class="prep-mini"><b>Maschera</b> · non ancora accesa</span>`;
  const [key, v] = x;
  const stato = v.stato==="esiste già" ? `esiste già · ${esc(v.riferimento||"")}` : v.costo ? `<b>${esc(v.chi||"")} ${fmt(num(v.costo),0)} €</b>${v.settimane?` · ${esc(v.settimane)} sett.`:""} ${v.fonte==="RISP"?chipF("RISP"):""}` : "da fare · da quotare";
  return `<span class="prep-mini" data-prep-riga="${esc(key)}">${I("wrench",13)} <b>Maschera</b> · ${stato} ${v.costo||v.stato==="esiste già"?"":richiediPrep(T, r, key, v)}${v.storico&&v.stato!=="esiste già"?` <span class="pr" title="dallo storico: ${esc(v.storico.rfq)}">◇ esiste già: ${esc(v.storico.rif)}</span>`:""}</span>`;
}
/* «Entrano»: la lista di scelta dei figli del padre (BOM confermata). Un clic fa entrare o uscire; un figlio in un'altra
   unione mostra «in f.10» e il clic lo SPOSTA (una scrittura sola). «dividi…» = solo una parte, campo obbligatorio e vuoto */
function bloccoEntrano(T, p, r, c, f, dove){
  const iu = infoUnione(T,p,r,c,f); if(!iu) return "";
  const u = unioniDi(c), righe = righeB(T,p);
  const chip = x => { const e = iu.entrano.find(y=>y.cid===x.cid), rr = righe.find(y=>y.cod===x.cod)||{}, co = rr.cid ? comeDi(T,rr.cid) : null;
    const altrove = u.filter(g=>g!==f && entranoDi(r,g).some(y=>y.cid===x.cid)).map(g=>`f.${(c.fasi.indexOf(g)+1)*10}`);
    const qta = e && e.qta!=null ? `${e.qta} di ${x.qta}` : x.qta;
    const lab = `${e?"✓ ":""}<span class="mono">${esc(sufSeq(T,p,x.cod))}</span> ×${qta}${rr.comm?" comm.":co&&co.modo==="cliente"?" dal cliente":""}${altrove.length?` <i>· in ${altrove.join(", ")}</i>`:""}${e&&e.fonte==="proposta"?" ◇":""}`;
    return btnB(`/fase/${f.id}/componente`, {cid:x.cid, entra: e?"0":"1"}, lab, e?"ent--su":"ent--no", `aria-pressed="${!!e}" title="${esc(x.cod)} · ${esc(x.descr||"")}${e?" · clic: esce da questa fase":altrove.length?" · clic: si sposta qui da "+altrove.join(", "):" · clic: entra in questa fase"}${e&&e.perche?" · "+esc(e.perche):""}"`); };
  const dividi = S.seqDividi===f.id ? `<form class="ent-dividi" data-pa-gesto="/fase/${f.id}/componente"><input type="hidden" name="entra" value="1"><input type="hidden" name="dividi" value="1">
      <select name="cid" aria-label="Figlio da dividere"><option value="">figlio</option>${r.figli.filter(x=>x.cid).map(x=>`<option value="${x.cid}">${esc(sufSeq(T,p,x.cod))} ×${x.qta}</option>`).join("")}</select>
      <input name="qta" value="" required inputmode="numeric" placeholder="quanti in questa fase" aria-label="Quanti pezzi in questa fase"><button class="ma-btn ma-btn--sm">Dividi</button>
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-dividi="">Annulla</button></form>` : "";
  const menuPrima = S.seqMenu===`prima|${f.id}|${dove}` ? `<span class="seq-menu" role="menu">${SEQ_UNIONI_AGG.map(k=>btnB(`/componente/${r.cid}/ciclo/fase`, {p:k, prima:f.id, seq:"1"}, esc(PROC[k][0]), "", 'role="menuitem"')).join("")}</span>` : "";
  const tutti = r.figli.filter(x=>x.cid), dentro = tutti.filter(x=>iu.entrano.some(y=>y.cid===x.cid));
  /* M-8: nel Ciclo una pre-unione con i suoi figli già dentro mostra solo quelli, più «+ altri figli ▾» (nella Sequenza tutti) */
  const corta = dove==="ciclo" && iu.preUnione && dentro.length>=2 && !(S.entAltri||{})[f.id], vis = corta ? dentro : tutti;
  return `<div class="ent" aria-label="Entrano nella fase ${iu.N}">
    <span class="lab" title="Dalla BOM confermata (STR). Un componente che manca non si aggiunge qui: si propone in Struttura (R-38).">Entrano${iu.preUnione?` · <b class="pp-tag pp-tag--pre">pre-unione</b>`:iu.principale&&u.length>1?" · unione principale":""}</span>
    <span class="ent-chips">${vis.map(chip).join("")}${corta?`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ent-altri="${f.id}" aria-expanded="false" title="Mostra gli altri figli del padre">+ altri figli (${tutti.length-dentro.length}) ▾</button>`:`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-dividi="${f.id}" title="Solo una parte dei pezzi in questa fase: il campo è obbligatorio e vuoto">dividi…</button>`}</span>
    ${dividi}
    <span class="ent-az"><button type="button" class="ma-btn ma-btn--sm" data-seq-menu="prima|${f.id}|${dove}" aria-expanded="${!!menuPrima}" title="Una fase di unione VUOTA subito prima di questa: i pezzi che vanno uniti prima di ogni altra lavorazione">+ fase di unione prima ▾</button>${menuPrima}
      ${dove==="ciclo"?`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vedi-albero="unione|${r.cid}|${f.id}" title="La stessa fase nell'albero del processo (④)">Vedi nell'albero →</button>`:""}</span>
  </div>`;
}

/* ---------------- la stazione ---------------- */
function stazioneSequenza(T, p){
  const banda = bandaB("R-10 · R-32 · R-33 · R-35 · R-36 · R-37","Sequenza e albero del processo: proposte, il backend di oggi non ha cicli né fasi. I motivi ⚠ vengono dalle regole R-35: senza R-35 l'albero mostra struttura e stati, senza motivi.");
  const sd = p ? stadiDi(T,p) : null;
  if(!sd || !sd.root.cid || !sd.ciclo) return `<div class="staz-vuota">${banda}${paAvvisoHtml()}<div class="empty"><span class="ib">${I("cube",22)}</span><h3>La sequenza si scrive sul prodotto confermato</h3><p>${!sd||!sd.root.cid?"Prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38).":`${esc(sd.root.cod)} non ha ancora un ciclo: prima ② Come e ③ Ciclo.`}</p>
    <div class="acts" style="justify-content:center"><button type="button" class="ma-btn" data-staz="${!sd||!sd.root.cid?1:2}">${!sd||!sd.root.cid?"← Struttura e documenti":"← ② Come"}</button></div></div>
    <div class="barra-conf seq-barra"><span class="bc-stato">sequenza non ancora scrivibile</span><span class="sp"></span><button type="button" class="ma-btn ma-btn--pri" disabled data-conferma-seq="1" title="La sequenza si scrive sul prodotto confermato: prima conferma l'albero: autorizza lo STEP, oppure aggiungi i componenti a mano (R-38).">Conferma la sequenza ⌃↵</button></div></div>`;
  const A = alberoProcesso(T,p), sq = seqDi(T,p), n = daSistemare(A), fonte = fonteSequenza(T,p);
  if(S.seqScelto && !trovaNodo(A, S.seqScelto)) S.seqScelto = null;
  const filtro = S.seqFiltro || "tutto", z = S.ppZoom || 1;
  const testa = `<div class="b-testa seq-testa"><h2>④ Sequenza</h2><span class="b-conta">${esc(sd.root.cod)} · ${esc(sd.root.descr||"")}</span><span class="sp"></span>
    <span class="seg" role="group" aria-label="Che cosa vedere">${[["tutto","tutto"],["sistemare",`solo da sistemare${n?" ("+n+")":""}`]].map(([k,l])=>`<button type="button" data-seq-filtro="${k}" aria-pressed="${filtro===k}">${l}</button>`).join("")}</span>
    <span class="seq-zoom"><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-menu="zoom" aria-expanded="${S.seqMenu==="zoom"}" title="Zoom (come nella Struttura)">⤢ ${Math.round(z*100)} % ▾</button>
      ${S.seqMenu==="zoom"?`<span class="seq-menu" role="menu"><button type="button" role="menuitem" data-pp-zoom="0">⤢ Adatta alla larghezza</button><button type="button" role="menuitem" data-pp-zoom="-1">− Rimpicciolisci</button><button type="button" role="menuitem" data-pp-zoom="1">+ Ingrandisci</button></span>`:""}</span>
    ${(()=>{ const ch = S.ppChiusi || new Set(), k = []; const giro = n => { if((n.tipo==="unione" || (n.tipo==="componente" && n.sottoassieme)) && (n.figli||[]).length) k.push(ppChiave(n)); (n.figli||[]).forEach(giro); }; (A.figli||[]).forEach(giro);
      return `<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-pp-tutti="apri" ${k.some(x=>ch.has(x))?"":"disabled"}>Apri tutto</button><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-pp-tutti="chiudi" ${k.length&&!k.every(x=>ch.has(x))?"":"disabled"}>Chiudi tutto</button>`; })()}</div>`;
  const conf = A.confermata;
  const statoTxt = conf ? `<span class="ma-st ma-st--ok ma-st--piatto">✓ confermata il ${esc(sq.quando)}</span>` : fonte==="L" ? `<span class="ma-st ma-st--prop ma-st--piatto">◐ scritta da te</span>` : `<span class="ma-st ma-st--prop ma-st--piatto">◇ proposta del sistema</span>`;
  const firma = firmaSequenza(T,p);
  /* A1 di Lino: la domanda dice che cosa si toglie e che cosa resta aperto, letto dal piano del server (nessuna scrittura) */
  const piano = S.seqChiediRip ? pianoRipristino(T,p) : null;
  const chiedi = piano ? `<span class="seq-chiedi" role="alertdialog" aria-modal="false" aria-labelledby="seq-chiedi-t" aria-describedby="seq-chiedi-d"><b id="seq-chiedi-t">Torna la proposta del sistema?</b>
      <span id="seq-chiedi-d">${piano.togli.length?`Si tolgono: ${esc(piano.togli.map(x=>`${sufSeq(T,p,x.cod)} ${x.testo}`).join(", "))}.`:"Le unioni restano; chi entra torna come proposto."}${piano.aperte.length?` Resta aperta: ${esc(piano.aperte.map(x=>x.testo).join(", "))} (PREP-04).`:""}</span>
      ${btnB(`/prodotto/${sd.root.cid}/sequenza/ripristina`, {firma}, "Sì, ripristina", "", 'data-seq-ripristina="si"')}<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-ripristina="no" id="seq-chiedi-no">No</button></span>` : "";
  const barra = `<div class="barra-conf seq-barra" role="group" aria-label="Conferma della sequenza">
    <span class="bc-stato">${statoTxt}${sq.riaperta&&!conf?` <span class="k3">${esc(sq.riaperta)}</span>`:""}</span>
    ${n?`<button type="button" class="ma-btn ma-btn--sm seq-manca" data-seq-filtro="sistemare" title="Mostra solo i nodi da sistemare">⚠ ${n} da sistemare</button>`:""}
    <span class="sp"></span>${chiedi}
    ${fonte==="L"&&!chiedi?`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-ripristina="chiedi">Ripristina la proposta</button>`:""}
    ${btnB(`/prodotto/${sd.root.cid}/sequenza/conferma`, {firma}, conf?"✓ Confermata":"Conferma la sequenza ⌃↵", "ma-btn--pri", `data-conferma-seq="1" title="Conferma quello che hai letto (con la firma): se nel frattempo è cambiato, il server dice di rileggere"`)}</div>`;
  return `<div class="staz-b seq16">${banda}${paAvvisoHtml()}<div class="seq16-main">${testa}
    <div class="seq16-corpo${S.seqScelto||S.seqPropAperte||S.r38descr?"":" seq16-corpo--vuoto"}">
      <section class="seq16-tela" aria-label="Albero del processo">${alberoProcessoHtml(T,p,A)}</section>
      <aside class="seq16-pannello" aria-label="Pannello del nodo scelto">${pannelloSeq(T,p,A)}</aside>
    </div>${barra}</div></div>`;
}
function trovaNodo(n, x){
  if(!n || !x) return null;
  const ok = x.tipo==="stadio" ? (n.cid===x.cid && ((n.tra||[]).concat(n.trasu||[], n.ultime||[], n.prima||[], n.dopo||[]).some(f=>f.fid===x.fid))) :
    n.tipo===x.tipo && n.cid===x.cid && (x.tipo!=="unione" || n.fid===x.fid) && (x.tipo!=="componente" || !x.padre || n.padre===x.padre);
  if(ok) return n;
  for(const y of (n.figli||[])){ const t = trovaNodo(y, x); if(t) return t; }
  return null;
}

/* ---------------- il pannello a destra: lo stadio, chi entra, i gesti ---------------- */
function pannelloSeq(T, p, A){
  const x = S.seqScelto;
  if(!x) return (S.seqPropAperte||S.r38descr) ? pannelloProdotto(T,p,A) : `<p class="seq-vuoto">Scegli un nodo: qui compaiono lo stadio, chi entra e i gesti.</p>`;
  const chiudi = `<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost seq-chiudi" data-seq-chiudi="1" title="Richiudi il pannello (Esc)">⇥ richiudi</button>`;
  if(x.tipo==="prodotto") return chiudi + pannelloProdotto(T,p,A);
  const r = rigaB(T,p,x.tipo==="componente"?x.padre:x.cid), c = r ? cicloB(T,r.cid) : null;
  if(x.tipo==="unione" || x.tipo==="stadio"){ const f = c ? c.fasi.find(y=>y.id===x.fid) : null; if(!f) return chiudi+`<p class="seq-vuoto">La fase non c'è più: scegli un altro nodo.</p>`; return chiudi + pannelloStadio(T,p,r,c,f); }
  if(x.tipo==="senza_fase") return chiudi + pannelloSenza(T,p,rigaB(T,p,x.cid));
  if(x.tipo==="componente") return chiudi + pannelloFiglio(T,p,A,x);
  return chiudi;
}
function menuFase(T, r, c, f){
  const aperto = S.seqMenu===`fase|${f.id}`;
  return `<span class="seq-piu"><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-menu="fase|${f.id}" aria-expanded="${aperto}" aria-label="Altre azioni sulla fase">⋯</button>
    ${aperto?`<span class="seq-menu seq-menu--dx" role="menu"><button type="button" role="menuitem" data-seq-rinomina="${f.id}">Rinomina</button>${btnB(`/fase/${f.id}/sposta`, {dir:"su"}, "Sposta su (Alt+↑)", "", 'role="menuitem"')}${btnB(`/fase/${f.id}/sposta`, {dir:"giu"}, "Sposta giù (Alt+↓)", "", 'role="menuitem"')}${btnB(`/fase/${f.id}/togli`, {}, "Togli", "", 'role="menuitem"')}</span>`:""}</span>`;
}
function rinominaForm(f){
  return S.seqRinomina===f.id ? `<form class="seq-rin" data-pa-gesto="/fase/${f.id}"><input name="nome" id="seq-rin-in" value="${esc(valF(f,"nome")||"")}" placeholder="${esc(PROC[f.p][0])}" aria-label="Nuovo nome dello stadio" autocomplete="off"><input type="hidden" name="fonte" value="L"><button class="ma-btn ma-btn--sm">Rinomina</button><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-seq-rinomina="">Annulla</button></form>` : "";
}
function menuStadio(T, r, c, f){
  const aperto = S.seqMenu===`stadio|${f.id}`;
  return `<span class="seq-agg"><button type="button" class="ma-btn ma-btn--sm" data-seq-menu="stadio|${f.id}" aria-expanded="${aperto}" title="Uno stadio nuovo del padre, subito dopo questo">+ stadio ▾</button>
    ${aperto?`<span class="seq-menu" role="menu">${SEQ_STADI_AGG.map(k=>btnB(`/componente/${r.cid}/ciclo/fase`, {p:k, dopo:f.id, seq:"1"}, esc(PROC[k][0]), "", 'role="menuitem"')).join("")}</span>`:""}</span>`;
}
function pannelloStadio(T, p, r, c, f){
  const sc = stadiCiclo(T,p,r,c), st = (sc[c.fasi.indexOf(f)]||{}).st || "", est = PROC[f.p] && PROC[f.p][1]==="est";
  const testa = `<header class="seq-p-testa"><span class="pp-lett">${esc(st)}</span><h3>${UNIONE.has(f.p)?"◆ ":est?"⇄ ":""}${esc(nomeFaseSeq(c,f))}</h3>${menuFase(T,r,c,f)}</header>
    <p class="seq-p-sotto">${UNIONE.has(f.p)?"unione":est?"stadio esterno":"stadio"} · fase del padre <span class="mono">${esc(sufSeq(T,p,r.cod))}</span>${valF(f,"nome")?` · <span class="k3" title="nome della lavorazione">${esc(PROC[f.p][0])}</span>`:""}</p>${rinominaForm(f)}`;
  const vai = `<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vai="3|${r.cid}|${f.id}">Apri nel Ciclo →</button>`;
  if(UNIONE.has(f.p)){
    const iu = infoUnione(T,p,r,c,f);
    const rifSel = `<label class="seq-rif"><span>Riferimento</span><select data-seq-rif="${f.id}" aria-label="Riferimento della fase di unione"><option value="">${iu.rif&&iu.rif.fonte==="CALC"?`◇ ${esc(sufSeq(T,p,iu.rif.cod))} (il più pesante · CALC)`:"scegli"}</option>${iu.entrano.map(e=>`<option value="${e.cid}" ${valF(f,"rif")===e.cid?"selected":""}>${esc(sufSeq(T,p,e.cod))}</option>`).join("")}</select></label>`;
    const masch = SALD.has(f.p)||f.p==="prigionieri" ? `<div class="seq-masch"><label class="spunta"><input type="checkbox" data-pa-spunta="${f.id}|maschera|sì" ${valF(f,"maschera")==="sì"?"checked":""}> in maschera</label>${propF(f,"maschera")&&valF(f,"maschera")==null?` <span class="pr" title="proposta del sistema · fonte ${esc(NOME_FONTE[propF(f,"maschera")[1]]||propF(f,"maschera")[1])}">◇ proposta: ${esc(propF(f,"maschera")[0])}</span>`:""} ${rigaMaschera(T,p,r,c,f)}</div>` : "";
    return `${testa}${bloccoEntrano(T,p,r,c,f,"seq")}${rifSel}${masch}
      <div class="seq-p-az">${menuStadio(T,r,c,f)}${vai}</div>
      <details class="seq-det"><summary>Pezzo intermedio con un codice di magazzino?</summary><p class="piccolo">Non è una pre-unione: è un sottoassieme vero. Si propone in <b>Proposte d'albero</b> (R-31): il codice lo scrive una persona e l'albero torna da confermare in Struttura.</p><button type="button" class="ma-btn ma-btn--sm" data-seq-scegli="prodotto|${A_root(T,p)}|prop">Proposte d'albero →</button></details>`;
  }
  const pre = suPreUnione(T,p,r,c,f);
  if(est){
    const figli = r.figli.filter(x=>x.cid), prot = String(valF(f,"proteggi")||"").split(",").filter(Boolean);
    const terzi = campoB(T,p,r,c,f,(CAMPI_FAM[f.p]||[]).find(d=>d[4]==="terzi")||["terzi","Terzisti (2–3)","",1,"terzi"]);
    const rich = f.p!=="trasporto" ? apriRichiesta(TRATT.has(f.p)?`verniciatori|${f.p}`:f.p==="laser_est"?"lamiera":`terzista|${f.p}`, TRATT.has(f.p)?"✉ Richiesta ai verniciatori":"✉ Richiesta al terzista") : "";
    const passi = TRATT.has(f.p) ? c.fasi.filter(y=>TRATT.has(y.p)).map(y=>nomeFaseSeq(c,y)).join(" → ") : "";
    return `${testa}${pre?`<p class="seq-p-sotto">lavora la pre-unione della f.${pre.N}</p>`:""}${terzi}<div class="seq-p-az">${rich}</div>
      ${passi?`<p class="seq-p-riga"><span class="lab">Passi</span> ${esc(passi)}</p>`:""}
      ${TRATT.has(f.p)?`<div class="seq-prot"><span class="lab" title="Le zone da non trattare (filetti dei dadi, sedi dei perni). Non è la maschera di saldatura.">Da proteggere <kbd>M</kbd></span>${figli.map(x=>{ const on = prot.includes(x.cid);
        return btnB(`/fase/${f.id}/proteggi`, {cid:x.cid, on: on?"0":"1"}, `${on?"✓ ":""}${esc(sufSeq(T,p,x.cod))}`, on?"ent--su":"ent--no", `aria-pressed="${on}"`); }).join("")}</div>`:""}
      <p class="seq-p-riga"><span class="lab">Trasporto</span> <span class="pr">${c.fasi.some(y=>y.p==="trasporto")?"nel ciclo":"andata e ritorno proposti"}</span> ${chipF("STO")}</p>
      <p class="seq-p-riga"><span class="lab">Semilavorato</span> esce ${esc(sufSeq(T,p,r.cod))} ${pre?"(pre-unione)":"saldato"}, rientra ${esc(PROC[f.p][0].toLowerCase())} · codice ERP: lo scrive Marta (R-33)</p>
      <div class="seq-p-az">${menuStadio(T,r,c,f)}${vai}</div>`;
  }
  return `${testa}${pre?`<p class="seq-p-sotto">lavora la pre-unione della f.${pre.N}, così com'è a quel punto</p>`:""}<div class="seq-p-az">${menuStadio(T,r,c,f)}${vai}</div>`;
}
function A_root(T,p){ const r = righeB(T,p).find(x=>x.radice); return r ? r.cid : ""; }
function pannelloFiglio(T, p, A, x){
  const n = trovaNodo(A, x); if(!n) return "";
  const r = rigaB(T,p,x.padre), c = r ? cicloB(T,r.cid) : null; if(!r || !c) return "";
  const u = unioniDi(c), dove = u.find(f=>entranoDi(r,f).some(e=>e.cid===x.cid));
  const sel = `<label class="seq-rif"><span>Entra in</span><select data-seq-entra="${x.cid}" aria-label="Fase in cui entra">${!dove?`<option value="">⚠ nessuna fase</option>`:""}${u.map((f,i)=>`<option value="${f.id}" ${dove===f?"selected":""}>${i+1} · ${esc(nomeFaseSeq(c,f))}${(infoUnione(T,p,r,c,f)||{}).preUnione?" (pre-unione)":""}</option>`).join("")}</select></label>`;
  const tratt = c.fasi.filter(f=>TRATT.has(f.p)), prot = tratt.some(f=>String(valF(f,"proteggi")||"").split(",").includes(x.cid));
  const rif = dove && valF(dove,"rif")===x.cid;
  const e = dove ? entranoDi(r,dove).find(y=>y.cid===x.cid) : null;
  return `<header class="seq-p-testa"><h3><span class="mono">${esc(sufSeq(T,p,n.cod))}</span> ×${n.qtaBom}</h3></header><p class="seq-p-sotto">${esc(n.descr||"")} · figlio di <span class="mono">${esc(sufSeq(T,p,r.cod))}</span>${e&&e.fonte==="proposta"?` · <span class="pr">◇ ${esc(e.perche||"proposta")}</span>`:""}</p>
    ${sel}
    <div class="seq-p-az">${tratt.length?`<button type="button" class="ma-btn ma-btn--sm${prot?"":" ma-btn--ghost"}" data-seq-proteggi="${x.cid}" aria-pressed="${prot}" title="da proteggere nel trattamento (M): non è la maschera di saldatura">${prot?"✓ ":""}da proteggere <kbd>M</kbd></button>`:""}
      ${dove?`<button type="button" class="ma-btn ma-btn--sm${rif?"":" ma-btn--ghost"}" data-seq-rifx="${x.cid}" aria-pressed="${rif}" title="riferimento della fase di unione (C)">${rif?"✓ ":""}riferimento <kbd>C</kbd></button>`:""}</div>
    ${n.comm?`<p class="seq-p-riga">commerciale: nessun ciclo, lo compra Fabio</p>`:n.cliente?`<p class="seq-p-riga">dal cliente: controllo in accettazione · arrivo con l'ordine</p>`:`<p class="seq-p-riga"><span class="lab">Ciclo</span> ${(n.fasi||[]).length?ppCatena(n.cid,n.fasi):"vuoto"} ${ppStato(n.stato)}</p>`}
    ${(n.motivi||[]).map(m=>`<p class="seq-p-riga seq-p-mot"><code>${esc(m.cod)}</code> ${esc(m.testo)} ${azioneCtrl(T,m.azione)}</p>`).join("")}
    <div class="seq-p-az">${n.comm?"":`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vai="3|${x.cid}">Apri nel Ciclo →</button>`}</div>`;
}
function pannelloSenza(T, p, r){
  if(!r) return "";
  const c = cicloB(T,r.cid), u = unioniDi(c), senza = r.figli.filter(x=>x.cid && !u.some(f=>entranoDi(r,f).some(e=>e.cid===x.cid)));
  return `<header class="seq-p-testa"><h3>⚠ Non entrano in nessuna fase</h3></header><p class="seq-p-sotto">SEQ-01 · figli di <span class="mono">${esc(sufSeq(T,p,r.cod))}</span> nella BOM confermata</p>
    ${senza.map(x=>`<label class="seq-rif"><span><span class="mono">${esc(sufSeq(T,p,x.cod))}</span> ×${x.qta}</span><select data-seq-entra="${x.cid}" data-seq-padre="${r.cid}" aria-label="Fase in cui entra ${esc(x.cod)}"><option value="">entra in…</option>${u.map(f=>`<option value="${f.id}">${esc(nomeFaseSeq(c,f))}</option>`).join("")}</select></label>`).join("")||`<p class="seq-vuoto">Nessuno: tutti entrano in una fase.</p>`}
    ${!u.length?`<p class="seq-p-riga">Il ciclo non ha una fase di unione: aggiungila.</p>${btnB(`/componente/${r.cid}/ciclo/fase`, {p:"sald_mag", seq:"1"}, "+ Saldatura MIG/MAG")}`:""}`;
}
function pannelloProdotto(T, p, A){
  const sd = stadiDi(T,p), conta = l => sd.fasi.filter(y=>y.st===l).length;
  const aa = assiemiDi(T,p);
  const particolari = righeB(T,p).filter(r=>r.cid && !r.comm && !r.figli.length && !r.radice);
  const aperte = !!(S.seqPropAperte || S.r38descr);
  return `<header class="seq-p-testa"><h3>Prodotto <span class="mono">${esc(A.cod)}</span></h3></header><p class="seq-p-sotto">${esc(A.descr||"")}</p>
    <p class="seq-p-riga"><span class="lab">Stadi letti</span> ${STADI.map(([l,nome])=>`<span class="seq-stadio-chip${conta(l)?"":" seq-stadio-chip--0"}" title="${esc(nome)}: ${conta(l)} fasi del prodotto (si leggono dalla lavorazione, non si scrivono)">${l} ${conta(l)}</span>`).join("")}</p>
    <p class="seq-p-riga"><span class="lab">Viaggi</span> ${sd.viaggi.length} esterni · ${sd.viaggi.map(v=>`<span class="k3">${esc(v.cod.slice(-4))} ${esc(PROC[v.f.p][0].toLowerCase())} · ${esc(v.chi)} · ${esc(v.trasporto)}</span>`).join(" · ")||"nessuno"}</p>
    <p class="seq-p-riga"><span class="lab">Dal cliente</span> ${sd.clienti} pezzi · controllo in accettazione</p>
    <p class="seq-p-riga"><span class="lab">Chi l'ha scritta</span> ${aa.map(r=>{ const c = cicloB(T,r.cid); return `<span><span class="mono">${esc(sufSeq(T,p,r.cod))}</span> ${!c?"senza ciclo":c.fonteSeq==="L"?"<b>scritta da te</b>":"◇ proposta"}</span>`; }).join(" · ")}</p>
    <div class="seq-p-az">${btnB(`/componente/${A.cid}/ciclo/fase`, {p:"controllo", seq:"1"}, "+ stadio in fondo", "", `title="Uno stadio nuovo in fondo al ciclo del prodotto (poi «Rinomina» e «Sposta» dal ⋯)"`)}<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-vai="3|${A.cid}">Apri nel Ciclo →</button></div>
    <details class="seq-det" id="seq-proposte" ${aperte?"open":""} data-seq-prop-det="1"><summary>Proposte d'albero</summary>
      <div class="seq-prop" id="prop-r31"><b title="R-31">Diventa assieme saldato…</b><span class="piccolo">Un particolare con componenti saldati sopra (un dado su una piastra) diventa un assieme con il suo codice di magazzino.</span>
        <label class="fld"><span>Particolare</span><select name="cid" id="r31-cid" aria-label="Particolare"><option value="">scegli</option>${particolari.map(r=>`<option value="${r.cid}">${esc(r.cod)} · ${esc(r.descr)}</option>`).join("")}</select></label>
        <label class="fld"><span>Codice dell'assieme</span><input name="codice" id="r31-codice" value="" autocomplete="off" aria-describedby="r31-nota"><small id="r31-nota">lo scrive una persona (convenzione di Marta)</small></label>
        <span><button type="button" class="ma-btn ma-btn--sm" id="r31-btn" data-r31="1" disabled>Proponi l'assieme</button></span></div>
      <div class="seq-prop" id="prop-r38"><b title="R-38">Manca nell'albero: proponi…</b>
        <label class="fld"><span>Che cosa manca</span><input name="descr" id="r38-descr" value="${esc(S.r38descr||"")}" placeholder="dado a saldare M10, dal catalogo (L)" autocomplete="off"></label>
        <label class="fld"><span>Sotto</span><select name="padre" id="r38-padre"><option value="">scegli</option>${righeB(T,p).filter(r=>r.cid && !r.comm).map(r=>`<option value="${esc(r.cod)}">${esc(r.cod)}</option>`).join("")}</select></label>
        <label class="fld"><span>Quantità</span><input name="qta" id="r38-qta" value="" inputmode="numeric"></label>
        <span><button type="button" class="ma-btn ma-btn--sm" data-pa-gesto="/distinta/${T.id}/nodo" data-pa-form="prop-r38">Proponi</button> <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-cat-apri="componenti|r38">▤ dal catalogo</button></span></div>
      ${a2Di(T).proposteAlbero.length?`<div class="seq-prop"><b>Proposte registrate</b>${a2Di(T).proposteAlbero.map(x=>`<span class="piccolo">${x.tipo==="assieme"?`assieme interno <span class="mono">${esc(x.codice)}</span>`:`manca: ${esc(x.descr)}${x.padre?" sotto "+esc(x.padre):""}`} · ${esc(x.r)} · ${esc(x.data)} · <b>l'albero non cambia</b></span>`).join("")}</div>`:""}
    </details>`;
}

/* ---------------- i clic della stazione (presentazione; le scritture sono [data-pa-gesto] o eseguiB) ---------------- */
document.addEventListener("click", e=>{
  const t = e.target.closest && e.target.closest("[data-seq-filtro],[data-seq-menu],[data-pp-zoom],[data-pp-tutti],[data-seq-chiudi],[data-seq-dividi],[data-seq-rinomina],[data-seq-ripristina],[data-seq-proteggi],[data-seq-rifx],[data-seq-scegli],[data-pp-stadio],[data-r31],[data-ent-altri]");
  if(!t) return;
  const D = t.dataset;
  if(D.seqRipristina==="si" && t.hasAttribute("data-pa-gesto")){ S.seqChiediRip = false; return; }   /* il gesto lo manda il ponte B */
  if(D.entAltri){ S.entAltri = Object.assign({}, S.entAltri, {[D.entAltri]:true}); render(); return; }
  if(D.seqFiltro){ S.seqFiltro = D.seqFiltro==="tutto" ? null : D.seqFiltro; render(); return; }
  if(D.seqMenu!==undefined){ S.seqMenu = S.seqMenu===D.seqMenu ? null : D.seqMenu; render(); return; }
  if(D.ppZoom!==undefined){ const z = +D.ppZoom; S.ppZoom = z===0 ? zoomAdatta() : Math.min(1.5, Math.max(.5, +((S.ppZoom||1) + z*.1).toFixed(2))); render(); return; }
  if(D.ppTutti){ const T = threadAperto(), A = alberoProcesso(T, prodottoScelto(T)); const s = S.ppChiusi = new Set();
    if(D.ppTutti==="chiudi"){ const giro = n => { if(n.tipo==="unione" || (n.tipo==="componente" && n.sottoassieme)) s.add(ppChiave(n)); (n.figli||[]).forEach(giro); }; (A&&A.figli||[]).forEach(giro); }
    S.seqMenu = null; render(); return; }
  if(D.seqChiudi){ S.seqScelto = null; S.seqPropAperte = false; render(); return; }
  if(D.seqDividi!==undefined){ S.seqDividi = D.seqDividi || null; render(); return; }
  if(D.seqRinomina!==undefined){ S.seqRinomina = D.seqRinomina || null; S.seqMenu = null; S.fuoco = D.seqRinomina ? "#seq-rin-in" : null; render(); if(typeof dopoRender==="function") dopoRender(); return; }
  /* P-05: «Sì, ripristina» è UN gesto (una rotta per prodotto, con la firma); qui solo la domanda, con il fuoco su «No» (B-9) */
  if(D.seqRipristina){ if(D.seqRipristina==="chiedi"){ S.seqChiediRip = true; S.fuoco = "#seq-chiedi-no"; render(); if(typeof dopoRender==="function") dopoRender(); return; }
    S.seqChiediRip = false; if(D.seqRipristina==="no"){ render(); const b = document.querySelector('[data-seq-ripristina="chiedi"]'); if(b) b.focus(); } return; }
  if(D.seqProteggi){ proteggiFiglio(D.seqProteggi); return; }
  if(D.seqRifx){ rifFiglio(D.seqRifx); return; }
  if(D.seqScegli){ const [tipo, cid, prop] = D.seqScegli.split("|"); S.seqScelto = {tipo, cid, fid:null, padre:null}; if(prop) S.seqPropAperte = true; render(); const d = document.getElementById("seq-proposte"); if(d && d.scrollIntoView) try{ d.scrollIntoView({block:"nearest"}); }catch(err){} return; }
  if(D.ppStadio){ const [cid, fid] = D.ppStadio.split("|"); S.seqScelto = {tipo:"stadio", cid, fid, padre:null}; render(); return; }
  if(D.r31){ const c = document.getElementById("r31-cid"), k = document.getElementById("r31-codice"); if(!c||!k) return;
    if(!c.value){ PA.avviso = {esito:"no", testo:"Scegli il particolare che diventa assieme."}; render(); return; }
    eseguiB(`/componente/${c.value}/assieme-interno`, {codice:k.value}); return; }
});
/* «entra in ▾» e il riferimento: un cambio = una scrittura */
document.addEventListener("change", e=>{
  const el = e.target; if(!el || !el.dataset) return;
  if(el.dataset.seqEntra){ if(!el.value) return; eseguiB(`/fase/${el.value}/componente`, {cid:el.dataset.seqEntra, entra:"1"}); return; }
  if(el.dataset.seqRif){ eseguiB(`/fase/${el.dataset.seqRif}`, {rif:el.value, fonte:"L"}); return; }
});
/* il gesto R-31 si accende solo con un codice scritto da una persona */
document.addEventListener("input", e=>{ if(e.target.id==="r31-codice"){ const b = document.getElementById("r31-btn"); if(b) b.disabled = !e.target.value.trim(); } if(e.target.id==="r38-descr") S.r38descr = e.target.value; });
document.addEventListener("toggle", e=>{ const t = e.target; if(t && t.dataset && t.dataset.seqPropDet) S.seqPropAperte = t.open; }, true);

/* «da proteggere» (M): UN gesto (P-12): il server sceglie le fasi di trattamento del padre e aggiunge o toglie il figlio */
function proteggiFiglio(cid){
  const T = threadAperto(), p = prodottoScelto(T), x = S.seqScelto, r = rigaB(T,p,(x&&x.padre)||""), c = r ? cicloB(T,r.cid) : null;
  const tratt = c ? c.fasi.filter(f=>TRATT.has(f.p)) : [];
  if(!tratt.length){ PA.avviso = {esito:"no", testo:"Niente è cambiato: il padre non ha un trattamento, non c'è niente da proteggere."}; render(); return; }
  const on = tratt.some(f=>String(valF(f,"proteggi")||"").split(",").includes(cid));
  eseguiB(`/fase/${tratt[0].id}/proteggi`, {cid, on: on?"0":"1"});
}
/* riferimento (C): il figlio diventa il riferimento della fase di unione in cui entra */
function rifFiglio(cid){
  const T = threadAperto(), p = prodottoScelto(T), x = S.seqScelto, r = rigaB(T,p,(x&&x.padre)||""), c = r ? cicloB(T,r.cid) : null;
  const f = c ? unioniDi(c).find(g=>entranoDi(r,g).some(e=>e.cid===cid)) : null;
  if(!f){ PA.avviso = {esito:"no", testo:"Niente è cambiato: il figlio non entra in nessuna fase di unione (SEQ-01)."}; render(); return; }
  eseguiB(`/fase/${f.id}`, {rif: valF(f,"rif")===cid ? "" : cid, fonte:"L"});
}

/* ---------------- i tasti della stazione (chiamati da tastiStazione di 58) ---------------- */
function tastiSequenza(e){
  const k = e.key;
  if(k==="Enter" && !e.shiftKey && S.seqScelto && S.seqScelto.tipo==="componente"){
    /* Invio accetta la proposta ◇ così com'è (niente da scrivere) e scende al figlio dopo */
    const vis = ppNodiVisibili().filter(x=>x.dataset.ppTipo==="componente"), i = vis.findIndex(x=>x.dataset.ppNodo===ppSceltoChiave());
    if(vis[i+1]) ppScegliDa(vis[i+1]); e.preventDefault(); return; }
  if(tastiAlbero(e)){ e.preventDefault(); return; }
  const x = S.seqScelto; if(!x) return;
  const T = threadAperto(), p = prodottoScelto(T);
  if(/^[1-9]$/.test(k) && x.tipo==="componente"){ const r = rigaB(T,p,x.padre), c = r ? cicloB(T,r.cid) : null, f = unioniDi(c)[+k-1];
    if(f){ eseguiB(`/fase/${f.id}/componente`, {cid:x.cid, entra:"1"}); } else { PA.avviso = {esito:"no", testo:`Niente è cambiato: il padre ha ${unioniDi(c).length} fasi di unione.`}; render(); } e.preventDefault(); return; }
  if((k==="m"||k==="M") && x.tipo==="componente"){ proteggiFiglio(x.cid); e.preventDefault(); return; }
  if((k==="c"||k==="C") && x.tipo==="componente"){ rifFiglio(x.cid); e.preventDefault(); return; }
}
/* Alt+↑↓ riordina lo stadio scelto (il gestore globale lascia passare Alt solo nel Ciclo); Esc richiude il pannello */
document.addEventListener("keydown", e=>{
  if(S.tab!=="rfq" || S.staz!==4) return;
  const t = e.target, campo = t && (t.tagName==="INPUT"||t.tagName==="TEXTAREA"||t.tagName==="SELECT");
  if(e.altKey && (e.key==="ArrowUp"||e.key==="ArrowDown") && !campo && S.seqScelto && S.seqScelto.fid && ["unione","stadio"].includes(S.seqScelto.tipo)){
    eseguiB(`/fase/${S.seqScelto.fid}/sposta`, {dir: e.key==="ArrowUp"?"su":"giu"}); e.preventDefault(); e.stopPropagation(); return; }
  if(e.key==="Escape" && !campo && !S.cassetto && !S.menu && !S.dialogo && !document.getElementById("ma-domanda")){
    if(S.seqMenu||S.seqDividi||S.seqRinomina||S.seqChiediRip){ S.seqMenu = S.seqDividi = S.seqRinomina = null; S.seqChiediRip = false; render(); e.preventDefault(); e.stopPropagation(); return; }
    if(S.seqScelto){ S.seqScelto = null; render(); e.preventDefault(); e.stopPropagation(); } }
}, true);

/* dopo un gesto B della Sequenza: i menu si chiudono, la fase nuova diventa il nodo scelto */
function dopoSequenza(url, r){
  if(/\/sequenza\/ripristina$/.test(url)) S.seqChiediRip = false;
  if(r.esito!=="ok") return;
  if(/\/fase\/[^/]+\/componente$/.test(url)) S.seqDividi = null;
  if(/\/ciclo\/fase$/.test(url) && r.fid && S.staz===4){ const T = threadAperto(), x = faseDelThread(T, r.fid); if(x) S.seqScelto = {tipo: UNIONE.has(x.f.p)?"unione":"stadio", cid:x.cid, fid:r.fid, padre:null}; }
  if(/\/fase\/[^/]+$/.test(url)) S.seqRinomina = null;
  if(/\/togli$/.test(url) && S.staz===4) S.seqScelto = null;
  S.seqMenu = null;
}

if(typeof AIUTO_STAZ!=="undefined") AIUTO_STAZ[4] = {
  tasti:[["↓ ↑","il nodo sotto · il nodo sopra (come nella Struttura)"],["← →","i nodi accanto"],["Home Fine","il primo e l'ultimo nodo"],["+ −","apre e chiude un ramo"],
    ["Invio","apre il pannello del nodo; su un figlio tiene la proposta ◇ e scende"],["Maiusc + Invio","apre il ciclo del nodo"],
    ["1…9","il figlio scelto entra nella 1ª…9ª fase di unione del padre (lo sposta)"],["M","da proteggere nel trattamento (non è la maschera di saldatura)"],["C","riferimento della fase di unione"],
    ["Alt + ↑ ↓","sposta lo stadio scelto"],["Esc","richiude il pannello"],["⌃↵","conferma la sequenza del prodotto (con la firma di quello che hai letto)"]],
  testi:["La Sequenza è l'albero del processo: il prodotto in alto, sotto le fasi di unione (◆) con i figli che ci entrano. Il ramo sopra ogni rombo si legge dal basso (↑): la fase più vicina al rombo è quella che viene subito dopo l'unione.",
    "Legenda delle sigle sulle caselle: L laser · LT laser tubo · S sega · Tr travi da terzi · P piega · Cu curvatura · F foratura · Fi filettatura · Sm smussatura · Sb sbavatura · Sa saldatura · Pr dadi saldati · M montaggio · Pi piantaggio · Rd raddrizzatura · Sab sabbiatura · Vp verniciatura a polvere · Zn zincatura · Co controllo · Ce certificati · Im imballo · Tp trasporto. Il nome intero è nel title.",
    "Legenda degli stadi: P preparazione dei componenti · U unione · F finitura del saldato · T trattamento esterno · D montaggio dopo il trattamento · C controllo finale e imballo. Si leggono dalla lavorazione, non si scrivono.",
    "Stati: ✓ completo · ◐ ciclo in bozza · ◇ proposta del sistema · ○ scegli chi entra · ⚠ da sistemare. Le linee sono tratteggiate finché la sequenza del prodotto non è confermata; poi diventano piene.",
    "La proposta del sistema è un punto di partenza: per i prodotti facili la confermi e basta. Per quelli complessi la scrivi tutta tu: + stadio, + fase di unione prima, Rinomina, Sposta, Togli, chi entra dove. Ogni gesto è una scrittura; il sistema non ti rifà il lavoro.",
    "Una pre-unione è una fase di unione del padre che viene prima dell'unione principale, con una parte dei figli: non è un pezzo nuovo e non ha un codice. Se il pezzo intermedio va a magazzino con un codice, è un sottoassieme: Proposte d'albero (R-31).",
    "«Ripristina la proposta» chiede prima che cosa si toglie (le unioni aggiunte a mano e le fasi che lavoravano su di loro) e che cosa resta aperto (una richiesta già preparata: PREP-04). La sequenza confermata è di ogni prodotto, non della RFQ."],
  richieste:"R-10 · R-32 · R-33 · R-35 · R-36 · R-37 · R-31 · R-38"};
