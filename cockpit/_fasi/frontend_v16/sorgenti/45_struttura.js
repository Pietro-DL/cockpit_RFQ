/* ================= 45 · stazione ① Struttura e documenti (zona A, pagina A1d) =================
   La traduzione nel mockup di web/templates/motore_a.html: stesse ancore (ma-*), stesse regioni R3…R10, stessi testi
   fissi (contratto §2.4), stessi gesti (data-ma-gesto, lista chiusa del ponte). Qui si sceglie solo COME mostrare:
   ogni stato, asse, motivo, quantità e fabbisogno viene dal finto server A attraverso l'Indice (RV-02, RV-03).
   Tre conferme in tre posti, con tre testi: la fonte in R3, l'albero in R4, i documenti in R8 e R9 (RV-13).
   v16 rev. (P-01 di Pietro): Distinta e Documenti sono UN SOLO workspace, come ma_workspace di motore_a.html. L'ordine del
   markup è quello che la richiesta 2 chiede a Pietro (un cambiamento del TEMPLATE, non del CSS): R9 Documenti e R8 Da
   smistare in alto, con la riga d'aiuto; poi R3, R4 sopra R5, R7; R6 a destra. La vista sceglie solo l'ANCORA su cui
   scorrere (motore_a.mjs, evidenziaDocumenti): «documenti» → #ma-documenti, «distinta» → #ma-fonte. Viste: + rfq (R10). */

const U07 = "La strada principale è \"Conferma l'albero\", dopo il riepilogo. Le decisioni nodo per nodo non rendono la BOM verificata e, prese prima del riepilogo, possono lasciarla da verificare.";
const FRASE_CAND = "Proposta dal file, non è la BOM. Nessuna struttura è scelta finché non si autorizza uno STEP: questa scheda è aperta solo per guardarla. I gesti dei nodi possono essere rifiutati finché lo STEP non è autorizzato: lo dice il server.";
const FRASE_QTA = "Scrivi la quantità: non c'è un valore già messo. Su un legame che c'è già, Collega cambia la quantità.";
const FRASE_ANTEPRIMA = "Scegli una casella: qui compaiono il disegno, i codici, la posizione e i gesti.";
const DB72 = "La copia sul NAS parte subito; la struttura non è ancora verificata.";
/* v16 (SPEC_v16 §2.1, M1 di Lino): i Documenti vengono per primi; la riga d'aiuto dice DOVE si fa ogni cosa (testo fisso) */
const AIUTO_DOC = "Qui confermi i disegni con il codice sicuro: dopo la copia sul NAS codice e documento non si correggono più. Lo STEP si autorizza più sotto, in «Analisi del prodotto»; l'albero in «Conferma dell'albero».";
/* gesto spento come in motore_a.html (template ma_na): il motivo nel title e sotto, alla lettera di Pietro */
function naP(testo, motivo){ return `<span class="ma-na"><button type="button" class="ma-btn ma-btn--sm ma-btn--na" disabled title="${esc(motivo)}">${esc(testo)}</button><small>${esc(motivo)}</small></span>`; }

const ic = (n,s=16)=>I(n,s);
/* un gesto che qui non c'è o è spento: il pulsante spento e il motivo, scritto accanto (zona C) */
let naSeq = 0;
function na(testo, motivo){ const id="na-"+(++naSeq); return `<span class="ma-na"><button type="button" class="ma-btn ma-btn--sm ma-btn--na" disabled aria-disabled="true" aria-describedby="${id}" title="${esc(motivo)}">${esc(testo)}</button><small id="${id}">non ancora disponibile · ${esc(motivo)}</small></span>`; }
const motivo = (dom, v) => v ? `<span class="ma-motivo" title="${esc(dom)} = ${esc(v)}">${esc(etichetta(dom,v))}</span>` : "";

/* ---------------- la catena dei codici: ogni valore con il suo segno, mai fusi ---------------- */
function catenaTesto(n){ const c=n.nodo.codice; if(c.confermato) return c.confermato.codice; if(n.nodo.decisione) return n.nodo.decisione.codice; if(c.manuale) return c.manuale.codice+" (a mano)"; if(c.proposto) return c.proposto+" (proposto)"; return "«"+((c.grezzo||{}).testo||"")+"»"; }
function catenaBreve(n){
  const c=n.nodo.codice, d=n.nodo.decisione; let k=0; if(c.confermato||d) k++; if(c.manuale) k++; if(c.proposto) k++;
  let h;
  if(c.confermato) h=`<b class="ma-segno ma-segno--conf" title="codice confermato">✓</b>${esc(c.confermato.codice)}${c.confermato.rev?`<span class="ma-rev">rev ${esc(c.confermato.rev)}</span>`:""}`;
  else if(d) h=`<b class="ma-segno ma-segno--conf" title="codice del componente confermato">✓</b>${esc(d.codice)}${d.rev?`<span class="ma-rev">rev ${esc(d.rev)}</span>`:""}`;
  else if(c.manuale) h=`<b class="ma-segno ma-segno--man" title="codice corretto a mano">✎</b>${esc(c.manuale.codice)}`;
  else if(c.proposto) h=`<b class="ma-segno ma-segno--prop" title="codice proposto dal file">◇</b>${esc(c.proposto)}`;
  else h=c.grezzo&&c.grezzo.testo?`<span class="ma-grezzo" title="testo letto nel file: non è ancora un codice">«${esc(c.grezzo.testo)}»</span>`:`<span class="ma-st ma-st--na">senza codice</span>`;
  return `<span class="ma-nodo-cod" title="${esc(catenaTesto(n))}">${h}${k>1?`<span class="ma-piu" title="la catena intera è nell'anteprima">+${k-1}</span>`:""}</span>`;
}
function catena(n){
  const c=n.nodo.codice, d=n.nodo.decisione, out=[];
  if(c.confermato) out.push(`<span class="ma-cod ma-cod--conf" title="codice confermato">✓ ${esc(c.confermato.codice)}${c.confermato.rev?` <span class="ma-rev">rev ${esc(c.confermato.rev)}</span>`:""}</span>`);
  else if(d) out.push(`<span class="ma-cod ma-cod--conf" title="codice del componente confermato">✓ ${esc(d.codice)}</span>`);
  if(c.manuale) out.push(`<span class="ma-cod ma-cod--man" title="codice corretto a mano">✎ ${esc(c.manuale.codice)}${c.manuale.rev?" rev "+esc(c.manuale.rev):""}</span>`);
  if(c.proposto) out.push(`<span class="ma-cod ma-cod--prop" title="codice proposto dal file">◇ ${esc(c.proposto)}</span>`);
  if(!out.length) out.push(c.grezzo&&c.grezzo.testo?`<span class="ma-cod ma-cod--grezzo">«${esc(c.grezzo.testo)}»</span>`:`<span class="ma-st ma-st--na">senza codice</span>`);
  return `<span class="ma-catena">${out.join("")}</span>`;
}
/* il tipo lo dà il server (Tipo, OrigineTipo): sulla casella l'etichetta corta di una parola (M6), per esteso nel pannello */
function tipoTag(n, corto){
  if(!n.tipo) return `<span class="ma-st ma-st--na" title="tipo vuoto">tipo non determinato</span>`;
  const prop = n.origine_tipo!=="confermato";
  return `<span class="ma-tipo ma-tipo--${esc(n.tipo)}${prop?" ma-tipo--proposto":""}" title="tipo_componente = ${esc(n.tipo)} · origine_tipo = ${esc(n.origine_tipo)} (${esc(etichetta("origine_tipo",n.origine_tipo))})">${esc(etichetta(corto?"tipo_componente_breve":"tipo_componente",n.tipo))}${prop?`<span aria-hidden="true">&nbsp;?</span><span class="vh"> (tipo proposto, non confermato)</span>`:""}</span>`;
}
function nodoTag(n, cid){
  if(n.nodo.riga_legacy) return `<span class="ma-proposto-tag" title="riga aperta di componente_proposta">da decidere</span>`;
  if(n.nodo.riga_decisa) return st("stato_riga", n.nodo.riga_decisa.stato);
  if(!cid) return `<span class="ma-proposto-tag ma-proposto-tag--neu">nessuna proposta</span>`;
  return "";
}
/* il componente della casella: Decisione; altrimenti SOLO nella scheda bom e SOLO per la radice, il componente del
   prodotto (contratto E3; template ma_li). Non è un calcolo di stato: è quale ID usare per i gesti. */
function cidCasella(IX, p, g, n){
  if(n.nodo.decisione) return n.nodo.decisione.componente_id;
  if(g.tipo==="bom" && n.nodo.rif===g.radice){ const k=IX.Componente(p.componente_id); return k?k.id:null; }
  return null;
}
function qtaArco(a){
  let h = a.quantita!=null ? `<span class="ma-qta" title="quantità letta nel file">×${a.quantita} dal file</span>` : "";
  if(a.quantita_confermata!=null) h += `<span class="ma-qta ma-qta--conf" title="quantità della relazione confermata">✓×${a.quantita_confermata} confermata</span>`;
  else h += a.decisa ? `<span class="ma-qta ma-qta--manca">confermato · qtà —</span>` : `<span class="ma-qta ma-qta--manca" title="legame non ancora confermato">da confermare</span>`;
  return h;
}
function celle(IX, p, cid){
  if(!cid) return `<span class="ma-celle"><span class="ma-cella ma-cella--neu" title="non ancora un componente: i documenti si chiedono quando lo diventa">non ancora un componente</span></span>`;
  const v = IX.Voce(p.rif, cid, "disegno_2d");
  let h = "";
  if(v){
    const k = v.esito==="presente"?"ok":v.esito==="da_verificare"?"sugg":v.esito==="manca"?"manca":"neu";
    h += `<span class="ma-cella ma-cella--${k}" title="2D: ${esc(etichetta("esito_fabbisogno",v.esito))}${v.stato_nas?" · "+esc(etichetta("stato_nas",v.stato_nas)):""}">2D ${v.esito==="presente"?"✓":v.esito==="da_verificare"?"✓?":"✗"}${v.stato_nas==="scritto"?" · NAS ✓":v.stato_nas==="in_coda"?" · NAS ◷":v.stato_nas==="errore"?" · NAS ✗":""}</span>`;
  }
  return `<span class="ma-celle">${h}</span>`;
}

/* ---------------- schizzi di esempio per famiglia (A11): il PDF vero lo mostra pdf.js; qui un disegno segnato «di esempio» ---------------- */
function schizzo(nomeFile, misura){
  const fam = SCHIZZI_FILE[nomeFile] || "documento";
  const S1 = 'stroke="var(--sk-tratto)" fill="none" stroke-width="1.6" stroke-linejoin="round"';
  const Q = 'stroke="var(--sk-quota)" fill="none" stroke-width=".9"';
  const forme = {
    telaio:`<rect x="30" y="50" width="240" height="110" ${S1}/><rect x="42" y="62" width="216" height="86" ${S1}/><path d="M30 105h240M150 50v110" ${S1}/><circle cx="54" cy="74" r="9" ${S1}/><circle cx="246" cy="74" r="9" ${S1}/><circle cx="54" cy="136" r="9" ${S1}/><circle cx="246" cy="136" r="9" ${S1}/><path d="M30 178h240M30 174v8M270 174v8" ${Q}/>`,
    profilo:`<path d="M70 40h70v18h-52v94h52v18H70z" ${S1}/><path d="M180 105h100M180 92v26M280 92v26" ${S1}/><path d="M60 40v130M56 40h8M56 170h8" ${Q}/><text x="190" y="85" font-size="11" fill="var(--sk-quota)" font-family="IBM Plex Mono,monospace">L 12000</text>`,
    tubo:`<circle cx="95" cy="105" r="52" ${S1}/><circle cx="95" cy="105" r="40" ${S1}/><path d="M170 75h110v60H170z" ${S1}/><path d="M170 85h110M170 125h110" stroke="var(--sk-quota)" stroke-dasharray="4 3" fill="none"/><path d="M43 172h104M43 168v8M147 168v8" ${Q}/>`,
    lamiera:`<path d="M40 150h70l30-60h90l30 60h20" ${S1}/><path d="M40 160h70l30-60h90l30 60h20" ${S1}/><path d="M40 178h240M40 174v8M280 174v8" ${Q}/>`,
    piastra:`<rect x="70" y="40" width="160" height="140" ${S1}/>${[[90,60],[210,60],[90,160],[210,160],[150,60],[150,160],[90,110],[210,110]].map(([x,y])=>`<circle cx="${x}" cy="${y}" r="7" ${S1}/>`).join("")}<path d="M70 192h160M70 188v8M230 188v8" ${Q}/>`,
    piastra2:`<rect x="80" y="45" width="140" height="130" ${S1}/>${[[100,65],[200,65],[100,155],[200,155]].map(([x,y])=>`<circle cx="${x}" cy="${y}" r="8" ${S1}/>`).join("")}<path d="M80 188h140M80 184v8M220 184v8" ${Q}/>`,
    perno:`<rect x="60" y="85" width="180" height="40" rx="4" ${S1}/><path d="M60 92h-14v26h14M240 95h20v20h-20" ${S1}/><path d="M60 140h180" ${Q}/>`,
    documento:`<rect x="80" y="22" width="140" height="166" ${S1}/>${[48,62,76,90,104,118,132,146,160].map((y,i)=>`<path d="M96 ${y}h${i%3===2?70:108}" ${Q}/>`).join("")}`,
    staffa:`<path d="M60 160V60h110l40 40v60z" ${S1}/><circle cx="90" cy="90" r="8" ${S1}/><circle cx="90" cy="135" r="8" ${S1}/><path d="M60 175h150M60 171v8M210 171v8" ${Q}/>`,
  };
  const big = misura==="grande";
  return `<svg class="schizzo" viewBox="0 0 300 210" role="img" aria-label="schizzo di esempio di ${esc(nomeFile)}" ${big?'preserveAspectRatio="xMidYMid meet"':""}>
    <rect x="2" y="2" width="296" height="206" fill="var(--sk-fondo)" stroke="var(--sk-quota)" stroke-width="1"/>${forme[fam]||forme.staffa}
    <rect x="196" y="4" width="100" height="14" fill="var(--sk-fondo)" stroke="var(--sk-quota)" stroke-width=".6"/><text x="200" y="14" font-size="7" textLength="92" lengthAdjust="spacingAndGlyphs" fill="var(--sk-quota)" font-family="IBM Plex Mono,monospace">schizzo di esempio</text></svg>`;
}
function mini(IX, a, nome){
  const an = a ? IX.Anteprima(a) : null;
  if(an && an.disponibile && an.sha256) return `<span class="ma-mini" aria-hidden="true" title="${esc(nome)}">${schizzo(nome)}</span>`;
  return `<span class="ma-mini ma-mini--na" aria-hidden="true" title="${esc(nome||"")}">anteprima non disponibile</span>`;
}

/* ================================================================ la stazione */
function stazioneStruttura(T, p){
  naSeq = 0;
  const IX = indiceDi(T);
  const av = PONTE.avviso;
  const vista = S.vista||"distinta";
  /* v16: Documenti · Distinta · Riepilogo RFQ. Documenti e Distinta sono la stessa pagina: il collegamento scorre all'ancora */
  const viste = [["documenti","folder","Documenti"],["distinta","list","Distinta"],["rfq","grid","Riepilogo RFQ"]];
  let corpo;
  if(!p) corpo = `<p class="ma-vuoto">Nessun prodotto valutato in questa RFQ: la pagina non ha un prodotto da mostrare.</p>`;
  else if(vista==="rfq") corpo = riepilogoRFQ(T, IX, p);
  else corpo = `<div class="ma-ws${S.latoChiuso?" ma-ws--lato-chiuso":""}" data-ma-ws><div class="ma-ws-griglia">
      <div class="ma-ws-doc"><p class="ma-aiuto-doc" id="ma-aiuto-doc">${ic("alert",14)}<span>${esc(AIUTO_DOC)}</span><a class="ma-poi" href="#ma-fonte" data-ma-scorri="#ma-fonte">↓ Analisi e albero</a></p>${documenti(T,IX,p)}</div>
      <div class="ma-ws-destra">${daSmistare(T,IX,p)}</div>
      <div class="ma-ws-centro">${analisi(T,IX,p)}${barraAlbero(T,IX,p)}${schedeAlbero(T,IX,p)}${conflitti(T,IX,p,"struttura")}</div>
      <aside class="ma-ws-lato${S.casella?" ma-ws-lato--foglio":""}" id="ma-anteprima" aria-label="Anteprima della casella scelta">${anteprimaCasella(T,IX,p)}</aside>
    </div></div>`;
  return `<div class="ma" id="ma" data-ma-thread="${T.id}" data-ma-prodotto="${esc(p?p.rif:"")}" data-ma-vista="${vista}">
    <div class="ma-navi">
      <nav class="ma-viste" id="ma-viste" aria-label="Viste della RFQ">${viste.map(([k,ico,l])=>`<a class="ma-vista" href="#${k}" data-vista="${k}" ${vista===k?'aria-current="page"':""}>${ic(ico,15)}${l}</a>`).join("")}</nav>
      <span class="sp"></span>
      <span class="ma-thread-gesti">
        <button type="button" class="ma-btn ma-btn--sm" data-ma-azione="aggiorna" title="Rilegge la pagina dal server. Non scrive niente.">${ic("refresh",14)} Aggiorna</button>
        <details class="ma-menu" data-ma-menu><summary class="ma-btn ma-btn--sm">Azioni del thread</summary>
          <div class="ma-menu-corpo" role="group" aria-label="Azioni del thread">
            <button type="button" class="ma-menu-voce" data-ma-gesto="/thread/${T.id}/fascicolo/prepara">Prepara i file<small>scarica, estrae e analizza i file del thread</small></button>
            <button type="button" class="ma-menu-voce" data-ma-gesto="/thread/${T.id}/smistamento/aggiorna">Rifai lo smistamento<small>ricalcola le destinazioni dei file</small></button>
          </div></details>
      </span>
    </div>
    <div id="ma-avviso" role="status" aria-live="polite" ${av?`data-esito="${av.esito}"`:"hidden"}>${av?`<span>${esc(av.testo)}</span><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="chiudi-avviso" aria-label="Chiudi l'avviso">${ic("x",14)}</button>`:""}</div>
    ${corpo}
  </div>`;
}
/* la vista sceglie solo l'ancora (motore_a.mjs): dopo una navigazione, non dopo la rilettura di un gesto */
const ANCORA_VISTA = {documenti:"#ma-documenti", distinta:"#ma-fonte"};
function scorriAncora(sel){
  const el = document.querySelector(sel); if(!el) return;
  if(el.scrollIntoView) try{ el.scrollIntoView({block:"start"}); }catch(e){}
  const t = el.querySelector("h2"); if(t){ if(!t.hasAttribute("tabindex")) t.setAttribute("tabindex","-1"); try{ t.focus({preventScroll:true}); }catch(e){} }
}

/* ---------------- R3 · Analisi del prodotto · conferma della fonte ---------------- */
function analisi(T, IX, p){
  const fo = p.fonte, gruppi = IX.Gruppi(p.rif), cand = gruppi.filter(g=>g.tipo==="cand");
  const conf = fo.stato==="confermata";
  const aperta = S.fonteAperta===null ? !conf : S.fonteAperta;
  const stato = fo.calcolata ? st("stato_fonte", fo.stato) : `<span class="ma-st ma-st--na" title="fonte.calcolata = false">non calcolata</span>`;
  const testa = `<header class="ma-fs-testa">${conf?ic("check"):ic("cube")}<h2 id="ma-fonte-t">Analisi del prodotto</h2><span class="lab" id="ma-fonte-lab">Conferma della fonte</span>${stato}
      ${!aperta&&fo.riferimento?`<span class="ma-fonte-riga mono">${esc(IX.NomeFile(fo.riferimento.allegato_id))}</span><span class="k3 piccolo">il ${esc((fo.riferimento.confermato_il||"").slice(0,10))}</span>`:""}
      <span class="sp"></span><button type="button" class="ma-btn ma-btn--sm" data-ma-gesto="/thread/${T.id}/fascicolo/rianalizza" title="Rilegge gli STEP e accoda le analisi.">${ic("refresh",14)} Rifai l'analisi</button>
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="fonte" aria-expanded="${aperta}" aria-controls="ma-fonte-corpo">${aperta?"Chiudi ▴":"Apri ▾"}</button></header>`;
  if(!aperta) return `<section class="ma-fs ma-analisi-box ma-analisi-box--chiusa${conf?" ma-analisi-box--ok":""}" id="ma-fonte" aria-labelledby="ma-fonte-t ma-fonte-lab">${testa}</section>`;
  const nessunaScelta = !conf && cand.length;
  const unica = cand.length===1;
  return `<section class="ma-fs ma-analisi-box${conf?" ma-analisi-box--ok":""}" id="ma-fonte" aria-labelledby="ma-fonte-t ma-fonte-lab">${testa}
    <div class="ma-analisi" id="ma-fonte-corpo">
      <div><span class="lab">Come ha deciso</span>
        <ul class="ma-decisioni">
          <li>Fonte: ${fo.motivo?motivo("motivo_fonte",fo.motivo):`<span class="ma-muto">${fo.calcolata?"nessun motivo":"i file non sono ancora preparati: «Prepara i file» è un pulsante, non parte da solo"}</span>`}</li>
          ${fo.estrazione?`<li>Estrazione: ${motivo("estrazione",fo.estrazione)}</li>`:""}
          ${fo.riferimento?`<li class="ma-riferimento"><span class="lab">STEP confermato</span> <b class="mono">${esc(IX.NomeFile(fo.riferimento.allegato_id))}</b> <span class="ma-muto">il ${esc((fo.riferimento.confermato_il||"").slice(0,10))}</span>${fo.riferimento.superato?`<span class="ma-st ma-st--ver">superato da un file più recente</span>`:""}
            <form class="ma-gesto ma-gesto--riga" data-ma-gesto="/thread/${T.id}/fascicolo/componente/${p.componente_id}/step-strutturale/revoca" data-ma-domanda="Revocare lo STEP autorizzato di ${esc(p.codice_richiesto)}? I figli diretti tornano guida e la BOM di lavoro si svuota. Le decisioni già prese restano.">
              <input type="hidden" name="sha" value="${esc(fo.riferimento.sha256)}"><label class="ma-campo"><span>Motivo</span><input name="motivo" maxlength="200" placeholder="facoltativo"></label>
              <button class="ma-btn ma-btn--sm ma-btn--danger">Revoca l'autorizzazione</button></form></li>`:""}
          ${!p.componente_id?`<li class="ma-nota--ver">Il prodotto non ha ancora un componente, quindi lo STEP strutturale non si può autorizzare.</li>`:""}
          ${T.esito.file.length?`<li class="ma-letti-file">${riassuntoFile(T,IX)}</li>`:""}
        </ul></div>
      <div><span class="lab">Che cosa ha letto</span>
        <ul class="ma-letti">
          ${gruppi.filter(g=>g.tipo==="bom").map(g=>`<li class="ma-letto ma-letto--bom"><span class="ma-letto-r1"><span class="ma-ext">BOM</span><b>BOM di lavoro</b><span class="mono">${esc(g.nome_file)}</span>
            <span class="ma-muto">dallo STEP autorizzato · ${g.nodi.length} nodi</span><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="vai-scheda" data-ma-gruppo="${esc(g.chiave)}">vedi lo schema ↓</button></span></li>`).join("")}
          ${cand.map((g,i)=>candRiga(T,IX,p,g,i,unica)).join("")}
          ${p.strutture_candidate.length?`<li class="ma-muto ma-piccolo">Strutture candidate: ${p.strutture_candidate.length} · ${nessunaScelta&&!unica?"scegli quale autorizzare: ":""}nessuna è scelta finché non si autorizza uno STEP.</li>`
            : !p.nodi.length?`<li class="ma-muto">Nessuna struttura candidata nell'esito per questo prodotto: nessuna è scelta.</li>`:""}
        </ul></div>
    </div></section>`;
}
/* che cosa c'è fra i file del thread, per disponibilità: conteggi di dati del server, letti (File[].Disponibilita) */
function riassuntoFile(T, IX){
  const per = {};
  for(const f of T.esito.file){ per[f.disponibilita]=(per[f.disponibilita]||0)+1; }
  return `<span class="lab">File del thread</span> ${Object.entries(per).map(([d,n])=>`<span class="ma-file-n">${n} ${st("disponibilita",d)}</span>`).join(" ")}`;
}
function candRiga(T, IX, p, g, i, unica){
  const sc = g.struttura;
  const aut = IX.AutorizzazioneDiStruttura(p.rif, sc.allegato_id, sc.radice);
  const prim = unica && p.fonte.stato!=="confermata";
  return `<li class="ma-letto ma-letto--cand" aria-label="Struttura candidata ${esc(g.nome_file)}">
    <span class="ma-letto-dati"><span class="ma-letto-r1"><span class="ma-ext ma-ext--step">STEP</span><b class="mono" title="allegato ${sc.allegato_id} · sha256 ${sc.sha256}">${esc(g.nome_file)}</b>${sc.radice_del_file?`<span class="ma-motivo">radice del file</span>`:`<span class="ma-motivo">nodo interno del file</span>`}</span>
    <span class="ma-letto-r2">codice ${st("compatibilita",sc.compatibilita)} · grafo ${sc.grafo_completo?`<span class="ma-st ma-st--ok">completo</span>`:`<span class="ma-st ma-st--ver">incompleto</span> ${motivo("motivo_grafo",sc.motivo_grafo)}`} · ${sc.nodi.length} nodi
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="vai-scheda" data-ma-gruppo="${esc(g.chiave)}">vedi lo schema ↓</button></span></span>
    <span class="ma-letto-gesti">${!p.componente_id ? na("Autorizza lo STEP strutturale…","il prodotto non ha ancora un componente, e la rotta del gesto lo vuole")
      : aut ? `<button type="button" class="ma-btn ma-btn--sm${prim?" ma-btn--pri":" ma-btn--acc"}" data-ma-azione="step" data-ma-componente="${p.componente_id}" data-ma-nodo="${esc(aut.nodo)}" data-ma-sorgente="${esc(aut.sorgente)}" data-ma-file="${esc(g.nome_file)}" data-ma-codice="${esc(p.codice_richiesto)}">${ic("check",14)} Autorizza lo STEP strutturale…</button>
        <small class="ma-muto">Prima si legge l'anteprima del server; la casella «autorizzo» non è spuntata.</small>`
      : na("Autorizza lo STEP strutturale…","nei dati della pagina non c'è un documento confermato del componente con questo file, né un nodo da delegare")}</span>
  </li>`;
}

/* ---------------- R4 · Conferma dell'albero (sopra le schede) ---------------- */
function barraAlbero(T, IX, p){
  const b = p.bom, nt = T.esito.fascicolo.numero_target;
  const fc = p.fonte.stato==="confermata";
  const pri = fc && !b.verificata;
  return `<section class="ma-barra-albero${b.verificata?" ma-barra-albero--ok":""}" id="ma-albero" aria-labelledby="ma-albero-t">
    <div class="ma-barra-riga">
      ${b.verificata?ic("check"):""}<h2 class="lab ma-barra-t" id="ma-albero-t">Conferma dell'albero</h2>
      ${b.verificata?`<span class="ma-st ma-st--ok" title="bom.verificata = true">BOM verificata</span>`:`<span class="ma-st ma-st--ver" title="bom.verificata = false">BOM non verificata</span>`}
      <span class="ma-barra-assi">nomenclatura ${st("stato_verifica_asse",b.nomenclatura.stato)}${b.nomenclatura.da_decidere?` <span class="ma-motivo">${b.nomenclatura.da_decidere} da decidere</span>`:""}
        · gerarchia ${st("stato_verifica_asse",b.gerarchia.stato)}${b.gerarchia.da_decidere?` <span class="ma-motivo">${b.gerarchia.da_decidere} da decidere</span>`:""}
        ${b.rimozioni_aperte?` · <span class="ma-motivo">${b.rimozioni_aperte} rimozioni aperte</span>`:""}</span>
      <span class="ma-barra-rfq">vale per tutta la RFQ (${nt} ${nt===1?"prodotto":"prodotti"})</span>
      <span class="sp"></span>
      <button type="button" class="ma-btn${pri?" ma-btn--pri":""}" data-ma-azione="albero">${ic("check",14)} ${b.verificata?"Rileggi il riepilogo dell'albero…":"Leggi il riepilogo e conferma l'albero…"}</button>
    </div>
    <p class="ma-nota ma-u07 ma-barra-nota">${esc(U07)} <span class="ma-k3">Conferma la struttura: non conferma la fonte e non conferma i documenti.${!p.nodi.length?" La BOM di lavoro non c'è ancora: se c'è qualcosa da confermare lo dice il riepilogo del server.":""}</span></p>
  </section>`;
}

/* ---------------- R5 · l'albero a schede ---------------- */
function schedeAlbero(T, IX, p){
  const gruppi = IX.Gruppi(p.rif);
  const vis = gruppi.find(g=>g.chiave===S.scheda) || gruppi[0];
  let k = 0;
  const schede = gruppi.map((g,i)=>{ if(g.tipo==="cand") k++; const sel = g===vis;
    return `<button type="button" role="tab" id="ma-scheda-${i}" class="ma-scheda ma-scheda--${g.tipo}" data-ma-azione="scheda" data-ma-gruppo="${esc(g.chiave)}"
      aria-controls="ma-albero-${i}" aria-selected="${sel}" tabindex="${sel?0:-1}" ${g.tipo==="cand"?'title="proposta dal file, non è la BOM; nessuna struttura è scelta finché non si autorizza uno STEP"':""}>
      <b>${g.tipo==="bom"?"BOM di lavoro":g.tipo==="cand"?"Candidata "+k:"Confermati senza STEP"}</b><small>${g.nome_file?esc(g.nome_file):g.nodi.length+" nodi"}</small></button>`; }).join("");
  k = 0;
  const pannelli = gruppi.map((g,i)=>{ if(g.tipo==="cand") k++; if(g!==vis) return "";
    const testa = g.tipo==="bom" ? `<span class="ma-k3 ma-piccolo">BOM di lavoro dallo STEP autorizzato · ${g.nodi.length} nodi · i componenti confermati senza STEP compaiono sotto il loro padre</span>`
      : g.tipo==="cand" ? `<p class="ma-nota ma-nota--cand"><b>Struttura candidata ${k} · ${g.nodi.length} nodi.</b> ${esc(FRASE_CAND)}</p>`
      : `<p class="ma-nota"><b>Componenti confermati senza STEP · ${g.nodi.length} nodi.</b> Il codice viene dal componente confermato, non da un file. Confermare componenti non conferma la fonte.</p>`;
    return `<div role="tabpanel" id="ma-albero-${i}" class="ma-pannello-albero ma-pannello-albero--${g.tipo}" data-ma-gruppo="${esc(g.chiave)}" aria-labelledby="ma-scheda-${i}">
      <div class="ma-tela-testa">${testa}</div>${tela(T,IX,p,g)}</div>`; }).join("");
  return `<section class="ma-fs ma-schede-albero" id="ma-schede-albero" aria-labelledby="ma-schede-albero-t">
    <header class="ma-fs-testa">${ic("list")}<h2 id="ma-schede-albero-t">Albero del prodotto <span class="mono">${esc(p.codice_richiesto)}</span></h2>
      ${!p.nodi.length?`<span class="ma-st ma-st--na" title="nessun nodo nella BOM di lavoro">BOM di lavoro: non ancora (si forma con lo STEP autorizzato)</span>`:""}
      <span class="sp"></span>
      ${gruppi.length?`<span class="ma-tela-ctl" role="group" aria-label="Vista della tela">
        <details class="ma-menu ma-menu--zoom" data-ma-menu><summary class="ma-btn ma-btn--sm ma-btn--ghost" title="Zoom della tela (anche + e − con Ctrl e la rotella)">⤢ <span class="ma-zoom mono">${Math.round((S.zoom||1)*100)} %</span> ▾</summary>
          <div class="ma-menu-corpo" role="group" aria-label="Zoom della tela">
            <button type="button" class="ma-menu-voce" data-ma-azione="zoom" data-z="adatta">⤢ Adatta alla larghezza</button>
            <button type="button" class="ma-menu-voce" data-ma-azione="zoom" data-z="-" aria-label="Rimpicciolisci">− Rimpicciolisci</button>
            <button type="button" class="ma-menu-voce" data-ma-azione="zoom" data-z="+" aria-label="Ingrandisci">+ Ingrandisci</button>
          </div></details>
        <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="rami" data-ma-aperti="1">Apri tutto</button>
        <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="rami" data-ma-aperti="0">Chiudi tutto</button></span>`:""}
    </header>
    <p class="vh" id="ma-tasti-albero">Albero come un organigramma: freccia giù porta al primo pezzo sotto, freccia su al padre, sinistra e destra ai pezzi accanto; più e meno aprono e chiudono un ramo; Invio apre l'anteprima del pezzo.</p>
    <div class="ma-schede-riga">${gruppi.length?`<div class="ma-schede" id="ma-schede" role="tablist" aria-label="Schede dell'albero">${schede}</div>`:""}
    <div class="ma-strumenti" role="group" aria-label="Strumenti della distinta" aria-describedby="ma-strumenti-perche">
      <button type="button" class="ma-btn ma-btn--sm" disabled aria-disabled="true">${ic("plus",13)} ${esc(etichetta("tipo_componente","sottoassieme"))}</button>
      <button type="button" class="ma-btn ma-btn--sm" disabled aria-disabled="true">${ic("plus",13)} ${esc(etichetta("tipo_componente","sciolto"))}</button>
      <button type="button" class="ma-btn ma-btn--sm" disabled aria-disabled="true">${ic("plus",13)} ${esc(etichetta("tipo_componente","commerciale"))}</button>
      <button type="button" class="ma-btn ma-btn--sm" disabled aria-disabled="true">${ic("folder",13)} Dal catalogo componenti</button>
      <small class="ma-strumenti-perche" id="ma-strumenti-perche">⊘ Non ancora disponibile: in questa pagina la bozza dell'albero resta vuota e nessun codice d'assieme parte dal browser (DB7-5, DB7-7).</small>
    </div></div>
    ${striscia38(T, IX, p)}
    ${gruppi.length?pannelli
      : `<p class="ma-vuoto">${T.nonPreparato?"I file non sono ancora preparati: nessuna struttura da mostrare. «Prepara i file» è il pulsante in alto.":"Nessuna struttura nell'esito per questo prodotto: né una BOM di lavoro, né strutture candidate, né componenti confermati."}</p>`}
    <div class="ma-legenda" role="group" aria-label="Legenda">
      <span><i class="ma-leg-linea ma-leg-linea--file"></i>legame del file (da confermare)</span>
      <span><i class="ma-leg-linea ma-leg-linea--conf"></i>legame confermato</span>
      <span><span class="ma-badge ma-badge--conf">confermato sotto Y ✓×q</span> relazione confermata fuori dal file</span>
      <span><span class="ma-badge">⧉ 2 padri nel file</span> sotto più padri</span>
      <span><b class="ma-segno ma-segno--conf">✓</b> confermato · <b class="ma-segno ma-segno--man">✎</b> a mano · <b class="ma-segno ma-segno--prop">◇</b> proposto · «» letto nel file</span>
      <span class="ma-muto">si trascina solo nella BOM di lavoro e fra i confermati senza STEP: apre il modulo «Sposta»</span>
    </div>
    <div class="ma-regola">${ic("alert",14)}<span>Che cosa può stare sotto che cosa lo decide il server: se uno spostamento non è ammesso, il rifiuto lo dice e niente cambia.</span></div>
  </section>`;
}
/* RL-01 · R-38 precisata (Proposta · dopo A2): senza STEP i componenti si aggiungono a mano sotto un padre, una riga per
   componente, con il PDF del prodotto aperto accanto per leggere la tabella distinta. Il codice lo scrive Lino: niente
   è precompilato dal nome di un file o dal cartiglio (DB7-7, R1). Il gesto è del finto server B (rotta immaginata
   /distinta/{thread}/componente) e scrive i componenti e le relazioni come farebbe il backend con R-38. */
function striscia38(T, IX, p){
  if(!p || !p.componente_id || p.nodi.length || T.nonPreparato) return "";
  const padri = (p.confermati_senza_step||[]).filter(n=>["finito","sottoassieme"].includes(n.tipo)).map(n=>n.nodo.decisione);
  const v = p.componente_id ? IX.Voce(p.rif, p.componente_id, "disegno_2d") : null, pr = v && v.disegni && v.disegni.primario;
  const pdf = pr && pr.anteprima && pr.anteprima.disponibile ? `<button type="button" class="ma-btn ma-btn--sm" data-ma-azione="pdf" data-ma-visore data-ma-allegato="${esc(pr.anteprima.allegato_id)}" data-ma-sha="${esc(pr.anteprima.sha256)}" data-ma-file="${esc(pr.nome_file)}" title="Apre ${esc(pr.nome_file)} nel visore: la tabella distinta si legge da lì">${ic("eye",13)} Apri ${esc(pr.nome_file)} accanto</button>` : "";
  const sotto = (p.confermati_senza_step||[]).length - 1;
  return `<section class="r38" aria-labelledby="r38-t" data-r38="1">
    <div class="r38-testa"><span class="banda-b-t" title="Proposta · dopo A2 · R-38 (precisata)">◇ Proposta · dopo A2 · R-38</span><b id="r38-t">Senza STEP: aggiungi i componenti a mano</b>
      <span class="k3 piccolo">${sotto>0?`${sotto} ${sotto===1?"componente aggiunto":"componenti aggiunti"} · escono fra i «Confermati senza STEP»`:"una riga per componente, leggendo la tabella distinta del disegno"}</span><span class="sp"></span>${pdf}</div>
    <form class="r38-riga" data-pa-gesto="/distinta/${T.id}/componente" aria-label="Aggiungi un componente sotto un padre">
      <label><span>Sotto</span><select name="padre" aria-label="Padre"><option value="">${esc((padri[0]||{}).codice||"padre")} (radice)</option>${padri.slice(1).map(d=>`<option value="${esc(d.componente_id)}">${esc(d.codice)}</option>`).join("")}</select></label>
      <label><span>Codice</span><input name="codice" id="r38-cod" autocomplete="off" placeholder="scrivilo dal disegno" aria-label="Codice del componente" required></label>
      <label class="r38-c"><span>Rev</span><input name="rev" autocomplete="off" aria-label="Revisione"></label>
      <label class="r38-l"><span>Denominazione</span><input name="descrizione" autocomplete="off" placeholder="dal cartiglio del pezzo" aria-label="Denominazione"></label>
      <label class="r38-c"><span>Q.tà</span><input name="qta" inputmode="numeric" autocomplete="off" aria-label="Quantità sotto il padre"></label>
      <label><span>Tipo</span><select name="tipo" aria-label="Tipo"><option value="">${esc(etichetta("tipo_componente","sciolto"))}</option><option value="sottoassieme">${esc(etichetta("tipo_componente","sottoassieme"))}</option><option value="commerciale">${esc(etichetta("tipo_componente","commerciale"))}</option></select></label>
      <button class="ma-btn ma-btn--sm ma-btn--acc">${ic("plus",13)} Aggiungi</button></form>
    <p class="k3 piccolo r38-nota">Il codice lo scrivi tu, mai dal nome del file né dal cartiglio (DB7-7). Dopo: «Rifai lo smistamento» propone i PDF con lo stesso nome, poi «Conferma l'albero». Senza STEP la gerarchia resta «da verificare» (K-02): la zona B va avanti con la presa d'atto (default, da confermare). La rev del prodotto si scrive con «Rev e descrizione» nella sua casella (A1d); il codice secondario del cliente non ha un campo (D).
      ${na("Proponi dalla tabella distinta del 2D (R-43)","spento: è una gerarchia dal PDF e R68 A la esclude; solo se Stefano riapre R68")}</p>
  </section>`;
}
function tela(T, IX, p, g){
  const rad = IX.Radici(p.rif, g.chiave);
  if(!rad.length) return `<p class="ma-vuoto">Nessun nodo in questa scheda.</p>`;
  let alb = rad.map(n=>casella(T,IX,p,g,n,null,0,g.chiave+">"+n.nodo.rif)).join("");
  if(!/aria-selected="true" tabindex="0"/.test(alb)) alb = alb.replace('aria-selected="false" tabindex="-1"', 'aria-selected="false" tabindex="0"');
  return `<div class="ma-tela" data-ma-gruppo="${esc(g.chiave)}" style="--z:${S.zoom||1}">
    <ul class="ma-dtree" role="tree" aria-describedby="ma-tasti-albero" aria-label="${g.tipo==="bom"?"BOM di lavoro":g.tipo==="cand"?"Struttura candidata "+esc(g.nome_file):"Componenti confermati senza STEP"}">
      ${alb}</ul></div>`;
}
function casella(T, IX, p, g, n, arco, liv, cam){
  const figli = IX.Figli(p.rif, g.chiave, n.nodo.rif), leg = IX.Legami(p.rif, g.chiave, n.nodo.rif);
  const scart = !!(n.nodo.riga_decisa && n.nodo.riga_decisa.stato==="scartata");
  const cid = cidCasella(IX, p, g, n), cod = cid ? (IX.Componente(cid)||{}).codice : "";
  const chiave = g.chiave+"|"+n.nodo.rif;
  const chiuso = (S.rami||{})[cam]===false;
  let da="", daCod="la radice", sposta=false;
  if(cid && arco && g.tipo!=="cand" && leg.confermati.length<=1){ sposta=true; leg.confermati.forEach(x=>{da=x.padre_id;daCod=x.codice;}); }
  const sel = S.casella===chiave;
  const aria = `${catenaTesto(n)}${n.tipo?", "+etichetta("tipo_componente",n.tipo):""}${!cid?", da decidere":""}${scart?", scartata":""}${arco?(arco.quantita!=null?`, ×${arco.quantita} dal file`:"")+(arco.quantita_confermata!=null?`, ×${arco.quantita_confermata} confermata`:arco.decisa?"":", legame da confermare"):""}${leg.del_file.length>1?`, ${leg.del_file.length} padri nel file`:""}`;
  const badge = [];
  if(leg.del_file.length>1) badge.push(`<span class="ma-badge" title="Nel file questo pezzo sta sotto ${leg.del_file.length} padri: compare sotto ciascuno.">⧉ ${leg.del_file.length} padri nel file</span>`);
  if(!n.nodo.senza_file) leg.confermati.filter(x=>!x.nel_file).forEach(x=>badge.push(`<span class="ma-badge ma-badge--conf" title="relazione confermata corrente, non nel file">confermato sotto ${esc(x.codice)} ✓×${x.qta}</span>`));
  return `<li role="treeitem" aria-level="${liv+1}" aria-selected="${sel}" tabindex="${sel?0:-1}" ${figli.length?`aria-expanded="${!chiuso}"`:""}
    class="${arco?(arco.decisa?"ma-li--conf":"ma-li--file"):""}" data-ma-chiave="${esc(chiave)}" data-ma-rif="${esc(n.nodo.rif)}" data-ma-cammino="${esc(cam)}" aria-label="${esc(aria)}">
    <div class="ma-nodo-box">
      ${arco?`<span class="ma-qta-arco">${qtaArco(arco)}</span>`:""}
      <div class="ma-nodo ma-nodo--${esc(n.tipo||"nd")}${!cid?" ma-nodo--proposto":""}${scart?" ma-nodo--scartato":""}" data-ma-azione="casella"
        ${sposta?`draggable="true" data-ma-sposta="/thread/${T.id}/fascicolo/componente/${cid}/sposta" data-ma-da="${esc(da)}" data-ma-da-cod="${esc(daCod)}"`:""}
        ${cid?`${g.tipo!=="cand"?`data-ma-drop="${cid}"`:""} data-ma-cod="${esc(cod)}"`:""}>
        <span class="ma-nodo-testa">${tipoTag(n,true)}${nodoTag(n,cid)}</span>
        ${catenaBreve(n)}
        ${n.classificazione&&n.classificazione.proposta==="minuteria"?`<span class="ma-cat" title="classificazione.proposta = minuteria (non confermata): non è un'esenzione dal 2D">minuteria proposta</span>`:""}
        ${cid&&!n.nodo.decisione?`<span class="ma-cat ma-cat--ok" title="contratto E3: la radice della BOM di lavoro prende il componente del prodotto">componente ✓</span>`:""}
        ${n.descrizione?`<span class="ma-nodo-nome" title="${esc(n.descrizione)}">${esc(n.descrizione)}</span>`:""}
        ${badge.length?`<span class="ma-nodo-badge">${badge.join("")}</span>`:""}
        ${celle(IX,p,cid)}
        ${figli.length?`<button type="button" class="ma-ramo" data-ma-azione="ramo" data-ma-cammino="${esc(cam)}" tabindex="-1" aria-label="Apri o chiudi i ${figli.length} pezzi sotto">${chiuso?"▸":"▾"} ${figli.length} sotto</button>`:""}
      </div>
    </div>
    ${figli.length&&!chiuso&&liv<24?`<ul role="group"${rastrello(IX,p,g,figli)}>${figli.map(f=>casella(T,IX,p,g,f.nodo,f,liv+1,cam+">"+f.nodo.nodo.rif)).join("")}</ul>`:""}
  </li>`;
}

/* con cinque o più figli che non hanno figli, la riga dell'organigramma diventa un rastrello su più righe: si legge
   senza scorrere di lato e ogni casella tiene il suo segno (tratteggio o pieno) e le parole delle quantità */
function rastrello(IX, p, g, figli){
  if(figli.length<5 || figli.some(f=>IX.Figli(p.rif,g.chiave,f.nodo.nodo.rif).length)) return "";
  return ` class="ma-rastrello" style="--col:${figli.length<=6?3:4}"`;
}

/* ---------------- la testata del pezzo: UN componente solo, uguale qui e nel Ciclo (M11) ---------------- */
function testataPezzo(T, IX, p, g, n, zonaB){
  const cid = cidCasella(IX, p, g, n), k = cid ? IX.Componente(cid) : null;
  const leg = IX.Legami(p.rif, g.chiave, n.nodo.rif);
  const v = cid ? IX.Voce(p.rif, cid, "disegno_2d") : null;
  const entra = leg.del_file.map(x=>{ const pd = IX.NodoIn(p.rif, g.chiave, x.padre) || IX.Nodo(p.rif, x.padre);
      return `<span class="tp-entra-v">sotto <span class="mono">${esc(pd?catenaTesto(pd):x.padre)}</span> ${qtaArco(x)}</span>`; }).join("")
    + leg.confermati.filter(x=>!x.nel_file).map(x=>`<span class="tp-entra-v">confermato sotto <span class="mono">${esc(x.codice)}</span> <span class="ma-qta ma-qta--conf">✓×${x.qta}</span></span>`).join("");
  return `<div class="tp" data-tp="${esc(n.nodo.rif)}">
    <div class="tp-r1">${tipoTag(n,false)}${k?`<b class="tp-cod mono">${esc(k.codice)}</b>`:`<span class="tp-cod">${catenaBreve(n)}</span>`}${k&&k.rev?`<span class="ma-rev">rev ${esc(k.rev)}</span>`:""}<span class="tp-nome">${esc(n.descrizione||"")}</span></div>
    <div class="tp-r2"><span class="lab">Entra</span>${entra||`<span class="k3">radice: nessun padre</span>`}</div>
    <div class="tp-r3">
      <span class="tp-cella"><span class="lab">Componente</span>${cid?`<span class="ma-st ma-st--ok" title="componente ${esc(k.codice)}">confermato</span>`:`<span class="ma-st ma-st--prop">non ancora</span>`}</span>
      <span class="tp-cella"><span class="lab">2D · conferma</span>${v?st("esito_fabbisogno",v.esito):`<span class="ma-st ma-st--na">—</span>`}</span>
      <span class="tp-cella"><span class="lab">Copia sul NAS</span>${v&&v.stato_nas?st("stato_nas",v.stato_nas):`<span class="ma-muto" title="stato_nas vuoto">—</span>`}</span>
    </div>
    ${zonaB||""}
  </div>`;
}

/* ---------------- R6 · l'anteprima della casella scelta ---------------- */
function casellaScelta(IX, p){
  if(!S.casella) return null;
  const i = S.casella.indexOf("|"); const chiave = S.casella.slice(0,i), rif = S.casella.slice(i+1);
  const g = IX.Gruppi(p.rif).find(x=>x.chiave===chiave); if(!g) return null;
  const n = g.nodi.find(x=>x.nodo.rif===rif); return n ? {g,n} : null;
}
function anteprimaCasella(T, IX, p){
  const x = casellaScelta(IX, p);
  if(!x) return `<div class="ma-fs ma-anteprima-vuota"><p class="ma-hint">${esc(FRASE_ANTEPRIMA)}</p>
    <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost ma-lato-btn" data-ma-azione="lato" title="Richiude la colonna: la tela si allarga">⇥ richiudi</button></div>`;
  const {g, n} = x, cid = cidCasella(IX, p, g, n), scart = !!(n.nodo.riga_decisa&&n.nodo.riga_decisa.stato==="scartata");
  const pr = n.disegni && n.disegni.primario;
  const stato = scart?"scartata":n.nodo.riga_legacy?"da decidere":n.nodo.decisione?"componente":cid?"componente del prodotto (radice della BOM di lavoro)":"non ancora un componente";
  const palco = pr ? (pr.anteprima && pr.anteprima.disponibile && pr.anteprima.sha256
      ? `<div class="ma-palco-barra"><button type="button" class="ma-btn ma-btn--sm" data-ma-azione="pdf" data-ma-visore data-ma-allegato="${esc(pr.anteprima.allegato_id||"")}" data-ma-sha="${esc(pr.anteprima.sha256)}" data-ma-file="${esc(pr.nome_file)}">${ic("eye",14)} Apri il visore</button><span class="mono ma-palco-file" title="${esc(pr.nome_file)}">${esc(pr.nome_file)}</span>${st("validita_2d",pr.validita)}</div>
         <div class="ma-palco">${schizzo(pr.nome_file)}</div>`
      : `<div class="ma-palco-barra"><span class="mono ma-palco-file">${esc(pr.nome_file)}</span></div><div class="ma-palco ma-palco--vuoto"><p>anteprima non disponibile</p></div>`)
    : `<div class="ma-palco ma-palco--vuoto"><p>nessun 2D associato al nodo</p></div>`;
  return `<section class="ma-fs ma-anteprima${!cid?" ma-anteprima--proposto":""}" data-ma-pannello="${esc(S.casella)}" aria-label="Anteprima di ${esc(catenaTesto(n))}">
    <header class="ma-anteprima-testa"><span class="lab">${g.tipo==="bom"?"BOM di lavoro":g.tipo==="cand"?"Struttura candidata · "+esc(g.nome_file):"Confermati senza STEP"} · ${stato}</span>
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost ma-chiudi" data-ma-azione="chiudi-anteprima" aria-label="Chiudi l'anteprima">${ic("x",15)}</button></header>
    ${testataPezzo(T, IX, p, g, n)}
    ${palco}
    ${(n.disegni.alternativi||[]).length?`<p class="ma-k3 ma-piccolo">Disegni alternativi: ${n.disegni.alternativi.map(a=>`<span class="mono">${esc(a.nome_file)}</span>`).join(", ")} · il principale cambia solo con «Sostituisci», che è un gesto.</p>`:""}
    <details class="ma-sez"><summary><h3 class="ma-sez-t">Codice</h3></summary>
      <dl class="ma-dl"><dt>Catena</dt><dd>${catena(n)}</dd>
        <dt>Letto nel file</dt><dd>${n.nodo.codice.grezzo&&n.nodo.codice.grezzo.testo?`<span class="mono">«${esc(n.nodo.codice.grezzo.testo)}»</span> <span class="ma-muto">dal campo ${esc(n.nodo.codice.grezzo.campo||"")}</span>`:`<span class="ma-muto">—</span>`}</dd>
        <dt>Proposto</dt><dd>${n.nodo.codice.proposto?`<span class="ma-cod ma-cod--prop">◇ ${esc(n.nodo.codice.proposto)}</span>`:`<span class="ma-muto">—</span>`} ${motivo("motivo_proposto",n.nodo.codice.motivo_proposto)}</dd>
        <dt>A mano</dt><dd>${n.nodo.codice.manuale?`<span class="ma-cod ma-cod--man">✎ ${esc(n.nodo.codice.manuale.codice)}</span> <span class="ma-motivo">da: ${esc(etichetta("origine_dato",n.nodo.codice.manuale.origine))}</span>`:`<span class="ma-muto">—</span>`}</dd>
        <dt>Componente</dt><dd>${n.nodo.decisione?`<span class="ma-cod ma-cod--conf">✓ ${esc(n.nodo.decisione.codice)}</span> <span class="ma-motivo">autorità: ${esc(etichetta("autorita",n.nodo.decisione.autorita))}</span>`:cid?`<span class="ma-cod ma-cod--conf">✓ ${esc(IX.Componente(cid).codice)}</span> <span class="ma-motivo">il componente del prodotto</span>`:`<span class="ma-muto">non ancora un componente</span>`}</dd>
        <dt>Riferimento</dt><dd><span class="mono ma-rif-tecnico" title="riferimento calcolato: non è un ID">${esc(n.nodo.rif)}</span></dd></dl></details>
    <details class="ma-sez"><summary><h3 class="ma-sez-t">Tipo e classificazione</h3></summary>
      <dl class="ma-dl"><dt>Tipo</dt><dd>${tipoTag(n,false)} ${motivo("origine_tipo",n.origine_tipo)}</dd>
        <dt>Ruolo</dt><dd>${n.classificazione&&n.classificazione.ruolo?esc(etichetta("ruolo",n.classificazione.ruolo)):"—"}</dd>
        <dt>Categoria</dt><dd>${st("categoria",n.classificazione&&n.classificazione.categoria)} <span class="ma-motivo">${n.classificazione&&n.classificazione.confermata?"categoria confermata":"categoria non confermata"}</span> ${motivo("origine_categoria",n.classificazione&&n.classificazione.origine)}</dd>
        ${n.classificazione&&n.classificazione.proposta==="minuteria"?`<dt>Proposta</dt><dd><span class="ma-cat">minuteria proposta (non confermata): non è un'esenzione dal 2D</span></dd>`:""}</dl>
      ${n.classificazione&&n.classificazione.categoria==="commerciale"?`<p class="ma-nota">Commerciale: non è un'esenzione dal 2D (R103 C). Se il 2D serve lo dicono le voci dei Documenti.</p>`:""}
      ${na("Conferma la minuteria","la conferma della categoria arriva con lo spazio di verifica (R-08): finché non c'è, la minuteria proposta chiede il 2D")}</details>
    <details class="ma-sez" open><summary><h3 class="ma-sez-t">Posizione</h3></summary>${posizione(T,IX,p,g,n)}</details>
    <details class="ma-sez"><summary><h3 class="ma-sez-t">Motivi${n.motivi&&n.motivi.length?` (${n.motivi.length})`:""}</h3></summary>
      ${n.motivi&&n.motivi.length?`<p>${n.motivi.map(m=>motivo("motivo_smistamento",m)).join(" ")}</p>`:`<p class="ma-muto">Nessun motivo dello smistamento sul nodo.</p>`}</details>
    <details class="ma-sez ma-sez-gesti" open><summary><h3 class="ma-sez-t">Gesti</h3></summary>${gestiNodo(T,IX,p,g,n)}</details>
    <div class="striscia-b" role="note"><span class="striscia-b-t">Proposta · dopo A2 · R-22 R-10</span>
      <span>Come si fa: <i>da decidere in ②</i> · Ciclo: <i>${cid?"vuoto":"si scrive su un componente confermato"}</i></span>
      ${cid?`<button type="button" class="ma-btn ma-btn--sm pa-btn" data-staz="3" data-ciclo-cid="${cid}">Apri nel Ciclo →</button>`:""}</div>
  </section>`;
}
function posizione(T, IX, p, g, n){
  const leg = IX.Legami(p.rif, g.chiave, n.nodo.rif);
  let file;
  if(n.nodo.senza_file) file = `<p class="ma-muto">Nessun legame del file: è un componente confermato senza STEP.</p>`;
  else if(!leg.del_file.length) file = `<p class="ma-muto">Radice nel file: nessun padre.</p>`;
  else {
    const aperta = leg.del_file.some(x=>!x.decisa && IX.RelazioneProposta(p.rif, x.padre, n.nodo.rif));
    file = `${aperta?`<p class="ma-nota ma-u07">${esc(U07)}</p>`:""}<ul class="ma-elenco">${leg.del_file.map(x=>{ const pd = IX.NodoIn(p.rif,g.chiave,x.padre);
      const rp = !x.decisa ? IX.RelazioneProposta(p.rif, x.padre, n.nodo.rif) : null;
      return `<li>sotto ${pd?catena(pd):`<span class="mono">${esc(x.padre)}</span>`} ${x.quantita!=null?`<span class="ma-qta">×${x.quantita} dal file</span>`:`<span class="ma-qta ma-qta--manca">qtà non letta</span>`}
        ${x.quantita_confermata!=null?`<span class="ma-qta ma-qta--conf">✓×${x.quantita_confermata} confermata</span>`:""}
        ${x.decisa?`<span class="ma-st ma-st--ok">legame confermato</span>`:`<span class="ma-st ma-st--prop">da confermare</span>`}
        ${rp?`<span class="ma-gesti-linea">
          <form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/relazione/accetta"><input type="hidden" name="allegato" value="${rp.allegato}"><input type="hidden" name="padre" value="${esc(rp.padre)}"><input type="hidden" name="figlio" value="${esc(rp.figlio)}"><button class="ma-btn ma-btn--sm">Accetta il legame</button></form>
          <form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/relazione/scarta" data-ma-domanda="Scartare il legame proposto? Il legame non entra nella BOM."><input type="hidden" name="allegato" value="${rp.allegato}"><input type="hidden" name="padre" value="${esc(rp.padre)}"><input type="hidden" name="figlio" value="${esc(rp.figlio)}"><button class="ma-btn ma-btn--sm ma-btn--danger">Scarta il legame</button></form></span>`
          : !x.decisa ? `<span class="ma-muto ma-piccolo">nessuna proposta aperta per questo legame</span>` : ""}</li>`; }).join("")}</ul>`;
  }
  return `<div class="ma-posizione"><div class="ma-pos-col"><span class="lab">Nel file</span>${file}</div>
    <div class="ma-pos-col"><span class="lab">Relazioni confermate correnti</span>${leg.confermati.length?`<ul class="ma-elenco">${leg.confermati.map(x=>`<li>sotto <span class="ma-cod ma-cod--conf">✓ ${esc(x.codice)}</span> <span class="ma-qta ma-qta--conf">✓×${x.qta}</span> <span class="ma-motivo">${x.nel_file?"anche nel file":"solo confermata"}</span></li>`).join("")}</ul>`:`<p class="ma-muto">nessuna</p>`}</div></div>`;
}
function opzioniComponenti(T, IX, p, escludi){
  const out = []; const visti = new Set();
  const k = IX.Componente(p.componente_id);
  if(k && k.id!==escludi){ out.push(`<option value="${k.id}">${esc(k.codice)} · il prodotto</option>`); visti.add(k.id); }
  for(const n of [...p.nodi, ...p.confermati_senza_step]) if(n.nodo.decisione){ const c=n.nodo.decisione.componente_id; if(c!==escludi && !visti.has(c)){ visti.add(c); out.push(`<option value="${c}">${esc(n.nodo.decisione.codice)}</option>`); } }
  return out.join("");
}
/* «Associa a…» di un file da smistare: i componenti dei prodotti DEL FILE (DaSmistare.Prodotti), raggruppati per prodotto;
   quelli dei prodotti del messaggio a parte. Senza prodotti nel file, i componenti del prodotto scelto. */
function opzioniPerFile(T, IX, p, allegato){
  const f = (T.esito.da_smistare||[]).find(x=>x.allegato_id===allegato);
  const gruppo = (rif, etich) => { const pp = IX.Prodotto(rif); return pp ? `<optgroup label="${esc(etich+" "+pp.codice_richiesto)}">${opzioniComponenti(T,IX,pp,"")}</optgroup>` : ""; };
  if(!f || !(f.prodotti||[]).length && !(f.prodotti_contesto||[]).length) return opzioniComponenti(T,IX,p,"");
  return [...(f.prodotti||[]).map(r=>gruppo(r,"del file ·")), ...(f.prodotti_contesto||[]).filter(r=>!(f.prodotti||[]).includes(r)).map(r=>gruppo(r,"dal messaggio ·"))].join("") || opzioniComponenti(T,IX,p,"");
}
function gestiNodo(T, IX, p, g, n){
  const t = T.id, cid = cidCasella(IX, p, g, n), out = [];
  if(n.nodo.riga_legacy || cid) out.push(`<p class="ma-nota ma-u07">${esc(U07)}</p>`);
  const rl = n.nodo.riga_legacy;
  if(rl){
    out.push(`<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/nodo/${rl.id}/accetta">
      <label class="ma-campo"><span>Tipo</span><select name="tipo"><option value="">come proposto dalla riga</option><option value="finito">${esc(etichetta("tipo_componente","finito"))}</option><option value="sottoassieme">${esc(etichetta("tipo_componente","sottoassieme"))}</option><option value="sciolto">${esc(etichetta("tipo_componente","sciolto"))}</option></select></label>
      <button class="ma-btn">${ic("check",14)} Accetta il nodo</button>
      <small class="ma-muto">Crea o ritrova il componente. Non conferma l'albero e non mette il legame con il padre.</small></form>`);
    out.push(`<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/nodo/${rl.id}/codice">
      <label class="ma-campo"><span>Codice</span><input name="codice" required autocomplete="off" spellcheck="false" value="${esc(rl.codice_manuale||"")}"></label>
      <label class="ma-campo ma-campo--corto"><span>Rev</span><input name="rev" autocomplete="off" value="${esc(rl.rev_manuale||"")}"></label>
      <button class="ma-btn">Correggi il codice</button><small class="ma-muto">La rev si manda sempre: un campo vuoto cancella quella che c'è.</small></form>`);
    out.push(`<div class="ma-gesti-pericolo"><button type="button" class="ma-btn ma-btn--danger" data-ma-gesto="/thread/${t}/fascicolo/nodo/${rl.id}/scarta"
      data-ma-domanda="Scartare il nodo ${esc(catenaTesto(n))}? Lo scarto è definitivo: la proposta esce dalla BOM di lavoro; i figli non vengono scartati.">Scarta il nodo (definitivo)</button></div>`);
  }
  const rd = n.nodo.riga_decisa;
  if(rd && rd.stato==="scartata") out.push(`<button type="button" class="ma-btn" data-ma-gesto="/thread/${t}/fascicolo/nodo/${rd.id}/riapri">Riapri il nodo</button>`);
  if(rd && rd.componente_id){ const k = IX.Componente(rd.componente_id); if(k && k.archiviato) out.push(`<div class="ma-gesto"><button type="button" class="ma-btn" data-ma-gesto="/thread/${t}/fascicolo/componente/${k.id}/ripristina">${ic("refresh",14)} Ripristina il componente</button><small class="ma-muto">${esc(k.codice)} è archiviato: «Ripristina» lo rimette nella BOM di lavoro con la sua storia.</small></div>`); }
  if(cid){
    const k = IX.Componente(cid), leg = IX.Legami(p.rif, g.chiave, n.nodo.rif), cod = k.codice;
    out.push(cid===p.componente_id ? na("Correggi il codice del componente","è il prodotto della RFQ: la rinomina del prodotto non si fa da questa pagina (DB7-3)")
      : `<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/codice" data-ma-domanda="Cambiare il codice del componente ${esc(cod)}? Cambia anche sui documenti non ancora copiati.">
        <label class="ma-campo"><span>Nuovo codice</span><input name="codice" required autocomplete="off" spellcheck="false"></label><button class="ma-btn">Correggi il codice del componente</button></form>`);
    out.push(k.descrizione ? `<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/modifica">
        <label class="ma-campo ma-campo--corto"><span>Rev</span><input name="rev" maxlength="10" autocomplete="off" value="${esc(k.rev||"")}"></label>
        <label class="ma-campo"><span>Descrizione</span><input name="descrizione" maxlength="200" autocomplete="off" value="${esc(k.descrizione||"")}"></label>
        <button class="ma-btn">Salva rev e descrizione</button><small class="ma-muto">Si mandano sempre tutti e due: un campo svuotato cancella quello registrato.</small></form>`
      : na("Salva rev e descrizione","la descrizione registrata del componente non c'è; quella mostrata può venire dallo STEP e il gesto la riscriverebbe"));
    out.push(`<form class="ma-gesto" data-ma-azione="tipo" data-ma-componente="${cid}" data-ma-codice="${esc(cod)}">
      <label class="ma-campo"><span>Tipo nuovo</span><select name="tipo" required><option value="" disabled selected>scegli</option>${["finito","sottoassieme","sciolto","commerciale"].map(x=>`<option value="${x}">${esc(etichetta("tipo_componente",x))}</option>`).join("")}</select></label>
      <button class="ma-btn">Vedi l'effetto del cambio…</button></form>`);
    out.push(`<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/collega">
      <label class="ma-campo"><span>Sotto il padre</span><select name="padre" required><option value="" disabled selected>scegli</option>${opzioniComponenti(T,IX,p,cid)}</select></label>
      <label class="ma-campo ma-campo--corto"><span>Qtà</span><input name="qta" type="number" required min="1" step="1" inputmode="numeric"></label>
      <button class="ma-btn">${ic("link",14)} Collega</button><small class="ma-muto">${esc(FRASE_QTA)}</small></form>`);
    out.push(`<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/sposta">
      ${leg.confermati.length?`<label class="ma-campo"><span>Da</span><select name="da" required><option value="" disabled selected>scegli</option>${leg.confermati.map(x=>`<option value="${x.padre_id}">${esc(x.codice)} ✓×${x.qta}${x.nel_file?" · anche nel file":""}</option>`).join("")}</select></label>`
        :`<input type="hidden" name="da" value=""><span class="ma-campo ma-campo--testo">Da: la radice (nessuna relazione confermata)</span>`}
      <label class="ma-campo"><span>A</span><select name="a" required><option value="" disabled selected>scegli</option><option value="">— diventa radice</option>${opzioniComponenti(T,IX,p,cid)}</select></label>
      <label class="ma-campo ma-campo--corto"><span>Qtà</span><input name="qta" type="number" required min="1" step="1" inputmode="numeric"></label>
      <button class="ma-btn">Sposta</button><small class="ma-muto">Scrivi la quantità: non c'è un valore già messo.</small></form>`);
    for(const x of leg.confermati) out.push(`<form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/scollega" data-ma-domanda="Togliere ${esc(cod)} da sotto ${esc(x.codice)}? Il componente resta; se non ha altri padri diventa una radice.">
      <input type="hidden" name="padre" value="${x.padre_id}"><button class="ma-btn ma-btn--sm">Scollega da ${esc(x.codice)}</button></form>`);
    if(cid!==p.componente_id) out.push(`<div class="ma-gesti-pericolo">
      <form class="ma-gesto" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/archivia" data-ma-domanda="Archiviare ${esc(cod)}? Esce dalla BOM di lavoro con i suoi legami; documenti, proposte e storia restano.">
        <label class="ma-campo"><span>Motivo</span><input name="motivo" required maxlength="200"></label><button class="ma-btn ma-btn--danger">Archivia</button></form>
      <button type="button" class="ma-btn ma-btn--danger" data-ma-gesto="/thread/${t}/fascicolo/componente/${cid}/rimuovi" data-ma-domanda="Rimuovere del tutto ${esc(cod)}? Il componente e i suoi legami spariscono e non si recuperano. Riesce solo se non ha storia; altrimenti usa «Archivia».">Rimuovi</button></div>`);
  }
  if(!rl && !cid && !rd) out.push(`<p class="ma-muto">Il nodo non è ancora un componente e non ha una proposta aperta: nessun gesto possibile da qui.</p>`);
  return `<div class="ma-gesti">${out.join("")}</div>`;
}

/* ---------------- R7 · conflitti ---------------- */
function conflitti(T, IX, p, quali){
  const el = IX.Conflitti(p.rif, quali);
  return `<details class="ma-fs ma-conflitti" id="ma-conflitti-${quali}" ${el.length?"open":""}>
    <summary><h2>Conflitti ${quali==="smistamento"?"dello smistamento":"della struttura"} (${el.length})</h2></summary>
    ${el.length?`<div class="wrap-tb"><table class="tb ma-tabella"><thead><tr><th>Tipo</th><th>Asse</th><th>Su</th><th>Decisione</th><th>Proposta</th><th>Motivo</th></tr></thead><tbody>
      ${el.map(c=>{ const nd = IX.Nodo(p.rif, c.rif); return `<tr><td>${st("tipo_conflitto",c.tipo)}</td><td>${esc(etichetta("asse_conflitto",c.asse))}</td>
        <td>${nd?catena(nd):`<span class="ma-muto" title="${esc(c.rif)}">riferimento tecnico</span>`}</td><td><span class="mono">${esc(c.decisione)}</span> ${c.origine_decisione?motivo("origine_dato",c.origine_decisione):""}</td>
        <td><span class="mono">${esc(c.proposta)}</span>${c.evidenza_proposta&&c.evidenza_proposta.allegato_id?` <span class="ma-muto">da ${esc(IX.NomeFile(c.evidenza_proposta.allegato_id))}</span>`:""}</td><td>${motivo("motivo_conflitto",c.motivo)}</td></tr>`; }).join("")}</tbody></table></div>
      <p class="ma-k3 ma-piccolo">Un conflitto di codice si risolve con «Correggi il codice» sul nodo, nella colonna a destra.</p>`
      :`<p class="ma-vuoto">Nessun conflitto ${quali==="smistamento"?"dello smistamento":"della struttura"} nell'esito per questo prodotto.</p>`}
  </details>`;
}

/* ---------------- R8 · Da smistare (tutta la RFQ) ---------------- */
function daSmistare(T, IX, p){
  const ds = T.esito.da_smistare;
  return `<section class="ma-fs ma-ws-smistare" id="ma-da-smistare" aria-labelledby="ma-ds-t">
    <header class="ma-fs-testa">${ic("folder")}<h2 id="ma-ds-t">Da smistare</h2><span class="ma-k3 ma-piccolo">tutta la RFQ</span><span class="sp"></span><span class="ma-k3">${ds.length}</span></header>
    ${ds.length?ds.map((f,i)=>{ const a=f.allegato_id, nome=f.nome_file, fi=IX.File(a), pid=IX.PropostaAperta(a);
      return `<article class="ma-pick" aria-label="File ${esc(nome)}" data-ma-allegato="${a}">
        ${mini(IX,a,nome)}
        <div class="ma-pick-corpo"><span class="ma-fn" title="${esc(nome)}">${esc(nome)}</span>
          <span class="ma-pick-stati"><span class="ma-pick-k">motivo</span>${st("motivo_smistamento",f.motivo)}
            <span class="ma-pick-k">file</span>${fi?st("disponibilita",fi.disponibilita)+(fi.motivo?" "+motivo("motivo_file",fi.motivo):""):`<span class="ma-st ma-st--na">—</span>`}</span>
          ${contestoFile(T,IX,f)}</div>
        <div class="ma-pick-gesti">${pid?`
          <button type="button" class="ma-btn ma-btn--sm" data-ma-gesto="/thread/${T.id}/fascicolo/conferma" data-ma-campi='{"voce":"${pid}"}' aria-label="Conferma il documento ${esc(nome)}"
            data-ma-domanda="Confermare ${esc(nome)} con il tipo e il componente che propone? ${p.bom.verificata?"La copia sul NAS parte subito.":esc(DB72)}">${ic("check",14)} Conferma il documento</button>
          <button type="button" class="ma-btn ma-btn--sm" data-ma-azione="associa" data-ma-allegato="${a}">${ic("link",14)} Associa a…</button>
          <button type="button" class="ma-btn ma-btn--sm ma-btn--danger" data-ma-gesto="/proposta/${pid}/scarta" title="Lo scarto del file è definitivo (DB7-8)." aria-label="Scarta il file ${esc(nome)}"
            data-ma-domanda="Scartare il file ${esc(nome)}? Lo scarto è definitivo: nessun gesto lo riporta indietro.">Scarta il file</button>`
          :`<span class="ma-muto ma-piccolo">Nessuna proposta aperta per questo file: da qui non c'è un gesto.</span>`}</div></article>`; }).join("")
      :`<p class="ma-hint">Nessun file da smistare nell'esito.</p>`}
  </section>`;
}
function contestoFile(T, IX, f){
  const as = IX.Associazione(f.allegato_id);
  const cod = r => { const x=IX.Prodotto(r); return x?x.codice_richiesto:r; };
  return `<div class="ma-contesto"><span><span class="ma-pick-k">del file</span>${f.prodotti.length?f.prodotti.map(r=>`<span class="ma-badge">${esc(cod(r))}</span>`).join(" "):`<span class="ma-muto">—</span>`}</span>
    <span><span class="ma-pick-k">dal messaggio</span>${f.prodotti_contesto.length?f.prodotti_contesto.map(r=>`<span class="ma-badge ma-badge--prop">${esc(cod(r))}</span>`).join(" "):`<span class="ma-muto">—</span>`}</span>
    ${as?`<span><span class="ma-pick-k">associazione</span>${st("associazione",as.associazione)} ${as.collocazione?motivo("collocazione",as.collocazione):""}${as.manuale?` <span class="ma-motivo">a mano: ${esc((IX.Componente(as.manuale)||{}).codice||"")}</span>`:""}</span>`:""}</div>`;
}

/* ---------------- R9 · Documenti e NAS: la conferma documentale, voce per voce ---------------- */
function documenti(T, IX, p){
  const cd = p.documenti;
  const riga = (v,i) => {
    const nome = v.disegni&&v.disegni.primario ? v.disegni.primario.nome_file : v.file_candidato ? IX.NomeFile(v.file_candidato) : "";
    return `<tr class="ma-voce${v.del_prodotto?" ma-voce--prodotto":""}" data-ma-voce="${v.componente_id}|${v.tipo_documento}">
      <td data-ma-lab="Componente"><span class="mono ma-voce-cod">${esc(v.codice)}</span> ${v.tipo_componente?`<span class="ma-tipo ma-tipo--${esc(v.tipo_componente)}">${esc(etichetta("tipo_componente_breve",v.tipo_componente))}</span>`:""}${v.categoria==="commerciale"?` <span class="ma-cat" title="categoria = commerciale (proposta, come nel riepilogo dell'albero): non è un'esenzione dal 2D">commerciale</span>`:""}
        <span class="ma-voce-regola">${v.bloccante?`<span class="ma-badge">bloccante</span>`:""}${v.invariante?` <span class="ma-badge" title="il 2D che le regole del cliente non tolgono">2D invariante</span>`:""}</span></td>
      <td data-ma-lab="Documento">${esc(etichetta("tipo_documento",v.tipo_documento))}</td>
      <td data-ma-lab="File e associazione" class="ma-c-file">${nome?`<span class="ma-file">${mini(IX, v.disegni&&v.disegni.primario?v.disegni.primario.allegato_id:v.file_candidato, nome)}<span class="ma-file-t"><span class="mono ma-nome-file" title="${esc(nome)}"><bdi>${esc(nome)}</bdi></span>${v.disegni&&v.disegni.primario?st("origine_dato",v.disegni.primario.provenienza):""}</span></span>`:`<span class="ma-muto">${v.documento_id?"documento del componente":"nessun file"}</span>`}</td>
      <td data-ma-lab="Conferma documentale">${st("esito_fabbisogno",v.esito)} ${motivo("motivo_fabbisogno",v.motivo)}${v.deroga_id?` <span class="ma-badge">derogato</span>`:""}</td>
      <td data-ma-lab="Copia sul NAS">${v.stato_nas?st("stato_nas",v.stato_nas):`<span class="ma-muto" title="stato_nas vuoto">—</span>`}</td>
      <td data-ma-lab="Gesti" class="ma-c-gesti">${v.proposta_aperta?`<button type="button" class="ma-btn ma-btn--sm" data-ma-gesto="/thread/${T.id}/fascicolo/conferma" data-ma-campi='{"voce":"${v.proposta_aperta}"}' aria-label="Conferma il documento ${esc(nome||"senza nome")} per ${esc(v.codice)}"
          data-ma-domanda="Confermare ${esc(nome||"il file")} per ${esc(v.codice)}? ${p.bom.verificata?"La copia sul NAS parte subito.":esc(DB72)} Dopo la copia codice e documento non si correggono più.">${ic("check",14)} Conferma il documento</button>`:""}
        ${v.esito==="manca"?`<button type="button" class="ma-btn ma-btn--sm ma-btn--ghost ma-link-b" data-cassetto-apri="richieste" title="Proposta · dopo A2 · R-41: la bozza nasce nel cassetto Richieste">✉ Chiedi al cliente <span class="bollino-b" aria-label="proposta dopo A2">◇</span></button>`:""}
        <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="ispeziona" data-ma-voce="${i}" aria-label="Ispeziona ${esc(etichetta("tipo_documento",v.tipo_documento))} di ${esc(v.codice)}">${ic("eye",14)} Ispeziona</button></td></tr>`;
  };
  const aperte = IX.CopieAperte(p.rif);
  const as = T.esito.associazioni;
  return `<section class="ma-fs ma-documenti${S.vista==="documenti"?" ma-evidenza":""}" id="ma-documenti" aria-labelledby="ma-documenti-t">
    <header class="ma-fs-testa ma-doc-testa">${ic("folder")}<h2 id="ma-documenti-t">Documenti e NAS · <span class="mono">${esc(p.codice_richiesto)}</span></h2>
      <span class="ma-sum"><span class="lab">Conferma documentale</span>${st("stato_documenti",cd.stato)} ${motivo("motivo_documenti",cd.motivo)}
        ${cd.perimetro_chiuso?`<span class="ma-st ma-st--ok">perimetro chiuso</span>`:`<span class="ma-st ma-st--ver" title="motivo_perimetro = ${esc(cd.motivo_perimetro||"")}">perimetro aperto</span> ${motivo("motivo_perimetro",cd.motivo_perimetro)}`}</span></header>
    <div class="ma-toolbar" role="group" aria-label="Gesti non ancora disponibili">
      ${naP("Cerca di nuovo sul portale","Non ancora disponibile: nessun codice scarica dal portale.")}
      ${naP("Carica a mano","Non ancora disponibile qui: si carica dalla pagina del Fascicolo (R-06).")}
      ${naP("Conferma tutti i riconosciuti","Non ancora disponibile: arriva con lo spazio di verifica, non si anticipa né si simula.")}
      ${naP("Conferma la minuteria","Non ancora disponibile: la conferma della categoria arriva con lo spazio di verifica (R-08).")}
    </div>
    ${avviso3D(T, p)}
    <p class="ma-hint">Lo STEP strutturale non è un documento da confermare qui: è la fonte (conferma della fonte, più sotto in «Analisi del prodotto»). Confermato non vuol dire copiato: solo «copiato sul NAS» dice che la copia è fatta.</p>
    ${!p.bom.verificata&&cd.voci.length?`<p class="ma-esito ma-esito--no">${ic("alert",15)}<span>La struttura non è ancora verificata: confermare un documento avvia subito la copia sul NAS.</span></p>`:""}
    ${cd.voci.length||cd.previsti.length?`<div class="wrap-tb ma-wrap-voci"><table class="tb ma-tabella ma-voci"><thead><tr><th>Componente</th><th>Doc.</th><th>File e associazione</th><th>Conferma documentale</th><th>Copia sul NAS</th><th>Gesti</th></tr></thead><tbody>
      ${cd.voci.map((v,i)=>v.del_prodotto?riga(v,i):"").join("")}${cd.voci.map((v,i)=>!v.del_prodotto?riga(v,i):"").join("")}
      ${cd.previsti.length?`<tr class="ma-gruppo-riga"><th colspan="6" scope="rowgroup">Previsti, non ancora nel perimetro <span class="ma-muto">· un previsto non conta come mancante</span></th></tr>
        ${cd.previsti.map(x=>{ const nd=IX.Nodo(p.rif,x.nodo); const pr=x.disegni&&x.disegni.primario; return `<tr class="ma-previsto"><td data-ma-lab="Componente">${nd?catena(nd):`<span class="mono">${esc(x.codice_proposto)}</span>`}</td>
          <td data-ma-lab="Documento">${esc(etichetta("tipo_documento",x.tipo_documento))}</td>
          <td data-ma-lab="File e associazione">${pr?`<span class="mono ma-nome-file" title="${esc(pr.nome_file)}">${esc(pr.nome_file)}</span>`:"—"} <span class="ma-motivo">${esc(etichetta("motivo_previsto",x.motivo))}</span></td>
          <td data-ma-lab="Conferma documentale"><span class="ma-st ma-st--neu ma-st--piatto" title="entra fra le voci quando il nodo entra nel perimetro">previsto</span></td><td data-ma-lab="Copia sul NAS">—</td><td data-ma-lab="Gesti"><span class="ma-muto">—</span></td></tr>`; }).join("")}`:""}
    </tbody></table></div>`
    : cd.stato==="non_calcolabile" ? `<p class="ma-vuoto">Nessuna voce: la completezza non si calcola ancora (vedi il motivo qui sopra).</p>` : `<p class="ma-vuoto">Nessuna voce di fabbisogno per questo prodotto nell'esito.</p>`}
    <div class="ma-doc-sotto">
      <section class="ma-nasbox" id="ma-nas" aria-labelledby="ma-nas-t"><header class="ma-fs-testa">${ic("folder",15)}<h3 id="ma-nas-t">Copia sul NAS</h3></header>
        ${aperte.length?`<ul class="ma-miss">${aperte.map(v=>`<li>${st("stato_nas",v.stato_nas)} <span class="mono">${esc(v.codice)}</span> · ${esc(etichetta("tipo_documento",v.tipo_documento))}</li>`).join("")}</ul>`:`<p class="ma-hint">Nessuna copia in coda o in errore per questo prodotto.</p>`}
        <button type="button" class="ma-btn${aperte.some(v=>v.stato_nas==="errore")?" ma-btn--pri":""}" data-ma-gesto="/thread/${T.id}/riprova-copie" title="Rimette in coda le copie in attesa o in errore.">${ic("refresh",14)} Riprova le copie sul NAS</button>
        <p class="ma-hint">Confermare un documento mette la copia in coda: «copiato sul NAS» arriva solo quando la copia è scritta. «Aggiorna» rilegge lo stato.</p></section>
      <div class="ma-chiusi">
        <details class="ma-fs ma-fs--chiusa"><summary><h3>Non bloccanti (${cd.non_bloccanti.length})</h3></summary>${cd.non_bloccanti.length?"":`<p class="ma-vuoto">Nessuno.</p>`}</details>
        <details class="ma-fs ma-fs--chiusa"><summary><h3>Fuori perimetro</h3></summary><ul class="ma-elenco">${as.filter(x=>x.perimetro!=="dentro").map(x=>`<li><span class="mono">${esc(IX.NomeFile(x.allegato_id))}</span> ${st("perimetro",x.perimetro)} ${st("disponibilita",(IX.File(x.allegato_id)||{}).disponibilita)}</li>`).join("")||`<li class="ma-muto">nessuno</li>`}</ul></details>
        <details class="ma-fs ma-fs--chiusa"><summary><h3>Scartati</h3></summary><ul class="ma-elenco">${as.filter(x=>x.esclusa).map(x=>`<li><span class="mono ma-muto">${esc(IX.NomeFile(x.allegato_id))}</span> <span class="ma-st ma-st--neu">scartato</span></li>`).join("")||`<li class="ma-muto">nessuno</li>`}</ul></details>
        <details class="ma-fs ma-fs--chiusa"><summary><h3>Orfani e avvisi (${T.esito.fascicolo.orfani} · ${T.esito.fascicolo.avvisi.length})</h3></summary><p class="ma-vuoto">${T.esito.fascicolo.orfani?"":"Nessun orfano e nessun avviso."}</p></details>
      </div>
    </div>
  </section>`;
}

/* RL-17 · avviso, non blocco: nessun 3D (STEP) fra i file del thread. La deroga del fabbisogno cad_3d è già un gesto di
   A1d; se il 3D sia un fabbisogno per questo cliente lo decide Lino (default: no, da confermare). */
function avviso3D(T, p){
  if(T.nonPreparato || !p) return "";
  const step = Object.values(T.allegati).some(a=>/^(step|stp|igs)$/i.test(a.tipo||"") || /\.(step|stp|igs)$/i.test(a.nome));
  if(step) return "";
  return `<p class="banda-b banda-b--3d" role="note" data-avviso-3d="1" title="Proposta · dopo A2 · RL-17"><span class="banda-b-t">◇ Proposta · dopo A2</span><span class="banda-b-x">3D assente: il thread ha solo PDF. È un avviso, non un blocco; se il 3D serve al cliente si chiede (default: non è un fabbisogno, da confermare).</span></p>`;
}

/* ---------------- R10 · Riepilogo della RFQ (vista rfq): gli assi letti, nessun gesto ---------------- */
function riepilogoRFQ(T, IX, p){
  const e = T.esito, f = e.fascicolo;
  const asse = (n, lab, t, corpo) => `<div class="ma-asse ma-asse--${t}"><span class="lab">${n} · ${lab}</span>${corpo}</div>`;
  const bomAsse = a => `<b>${st("stato_verifica_asse",a.stato)}</b>${a.motivo?`<span class="ma-k3">${esc(etichetta("motivo_asse",a.motivo))}</span>`:""}${a.da_decidere?`<span class="ma-k3">${a.da_decidere} da decidere</span>`:""}`;
  return `<div class="ma-riepilogo" id="ma-riepilogo">
    <section class="ma-fs ma-assi-box" aria-labelledby="ma-assi-t"><header class="ma-fs-testa"><h2 id="ma-assi-t">Assi del prodotto <span class="mono">${esc(p.codice_richiesto)}</span></h2></header>
      <div class="ma-assi">
        ${asse(1,"Prodotto della RFQ",tono(p.identita),`<b>${st("stato_identita_target",p.identita)}</b><span class="ma-k3">autorità: ${esc(etichetta("autorita",p.autorita))}</span>`)}
        ${asse(2,"Fonte strutturale",p.fonte.calcolata?tonoD("stato_fonte",p.fonte.stato):"neu",`<b>${p.fonte.calcolata?st("stato_fonte",p.fonte.stato):`<span class="ma-st ma-st--na">non calcolata</span>`}</b>${p.fonte.motivo?`<span class="ma-k3">${esc(etichetta("motivo_fonte",p.fonte.motivo))}</span>`:""}`)}
        ${asse(3,"Nomenclatura",tono(p.bom.nomenclatura.stato),bomAsse(p.bom.nomenclatura))}
        ${asse(4,"Gerarchia",tono(p.bom.gerarchia.stato),bomAsse(p.bom.gerarchia))}
        ${asse(5,"Smistamento",tono(p.smistamento.stato),`<b>${st("stato_smistamento",p.smistamento.stato)}</b>${p.smistamento.motivi.map(m=>`<span class="ma-k3">${esc(etichetta("motivo_smistamento",m))}</span>`).join("")}`)}
        ${asse(6,"Documenti",tono(p.documenti.stato),`<b>${st("stato_documenti",p.documenti.stato)}</b>${p.documenti.motivo?`<span class="ma-k3">${esc(etichetta("motivo_documenti",p.documenti.motivo))}</span>`:""}`)}
        ${asse("","Stato del prodotto",tono(p.stato),`<b>${st("stato_prodotto",p.stato)} ${p.prodotto_verificato?`<span class="ma-st ma-st--ok">verificato</span>`:`<span class="ma-st ma-st--ver">non verificato</span>`}</b>${p.motivi.map(m=>`<span class="ma-k3">${esc(etichetta("motivo_prodotto",m))}</span>`).join("")}`)}
      </div>
      <p class="ma-nota">«Pronto» non vuol dire «partito»: la fase la cambia solo un gesto registrato.</p></section>
    <section class="ma-fs"><header class="ma-fs-testa"><h2>Prodotti della RFQ</h2></header>
      <div class="wrap-tb"><table class="tb ma-tabella"><thead><tr><th>Prodotto</th><th>Identità</th><th>Fonte</th><th>Struttura</th><th>Smistamento</th><th>Documenti</th><th>Stato</th></tr></thead><tbody>
      ${e.prodotti_valutati.map(x=>`<tr${x.rif===p.rif?' class="ma-riga-scelta"':""}><th scope="row"><button type="button" class="ma-link mono" data-prodotto="${esc(x.rif)}" ${x.rif===p.rif?'aria-current="true"':""}>${esc(x.codice_richiesto)}</button></th>
        <td>${st("stato_identita_target",x.identita)}</td><td>${x.fonte.calcolata?st("stato_fonte",x.fonte.stato):`<span class="ma-st ma-st--na">non calcolata</span>`}</td>
        <td>${st("stato_struttura",x.struttura)}${x.bom.verificata?` <span class="ma-motivo">BOM verificata</span>`:""}</td><td>${st("stato_smistamento",x.smistamento.stato)}</td>
        <td>${st("stato_documenti",x.documenti.stato)}</td><td>${st("stato_prodotto",x.stato)}</td></tr>`).join("")}</tbody></table></div></section>
    <section class="ma-fs"><header class="ma-fs-testa"><h2>Fascicolo della RFQ</h2>${e.valutato?`<span class="ma-st ma-st--ok">thread valutato</span>`:`<span class="ma-st ma-st--ver">thread non valutato</span>`}<span class="sp"></span>${na("Congela la V1","il congelamento non si registra da questa pagina")}</header>
      <dl class="ma-dl"><dt>Pronti</dt><dd>${f.pronti.length?f.pronti.map(r=>`<span class="ma-badge">${esc((IX.Prodotto(r)||{}).codice_richiesto||r)}</span>`).join(" "):`<span class="ma-muto">nessuno</span>`}</dd>
        <dt>Bloccati</dt><dd>${f.bloccati.length?f.bloccati.map(b=>`<div><span class="mono">${esc((IX.Prodotto(b.rif)||{}).codice_richiesto||b.rif)}</span> ${b.motivi.map(m=>motivo("motivo_prodotto",m)).join(" ")}</div>`).join(""):`<span class="ma-muto">nessuno</span>`}</dd>
        <dt>Orfani</dt><dd>${f.orfani}</dd></dl></section>
    ${conflitti(T,IX,p,"smistamento")}
    <details class="ma-fs ma-fs--chiusa" open><summary><h2>Non ancora disponibile</h2></summary>
      <ul class="ma-spenti">
        <li>${na("Passaggi di fase","nessun codice li esegue; la fase cambia solo con un gesto registrato")}</li>
        <li>${na("Nuova Inbox e prodotti del messaggio","arrivano con AM (R-15)")}</li>
        <li>${na("Numero interno, mittente sospeso, «consegnata», richiesta senza RFQ","decisioni ancora aperte (R-02, R-03, R-04, R-05)")}</li>
        <li>${na("Note sui PDF, Conferma Fascicolo legacy","restano nella pagina del Fascicolo")}</li>
      </ul></details>
  </div>`;
}

/* ================================================================ le finestre (in pagina: niente window.confirm né showModal) */
function finestraStruttura(T, p){
  const d = S.dialogo, IX = indiceDi(T);
  let titolo="", sopra="", corpo="", piede="";
  if(d.tipo==="step"){
    const a = d.a; sopra = "Autorizza lo STEP strutturale · conferma della fonte"; titolo = d.file;
    corpo = a.errLettura ? `<p class="ma-errore">L'anteprima non si legge: ${esc(a.errLettura)}</p>` : `
      <p class="ma-nota">Conferma della fonte di ${esc(d.codice)}: non conferma l'albero e non conferma i documenti.</p>
      ${a.errore?`<p class="ma-errore">${esc(a.errore)}</p>`:""}${a.spento?`<p class="ma-spento">Non si può autorizzare: ${esc(a.spento)}</p>`:""}${a.gia?`<p class="ma-nota--ver">${esc(a.gia)}</p>`:""}
      ${(a.frasi||[]).length?`<ul class="ma-frasi">${a.frasi.map(x=>`<li>${esc(x)}</li>`).join("")}</ul>`:""}
      ${(a.figli||[]).length?`<div class="ma-step-figli"><span class="lab">Figli diretti</span><ul class="ma-frasi ma-frasi--figli">${a.figli.map(f=>`<li><span class="mono">${esc(f.codice)}</span> · ${esc(f.nome)}${f.qta!=null?` · ×${f.qta}`:""} <span class="ma-motivo">${esc(f.stato)}</span></li>`).join("")}</ul></div>`:""}
      ${a.presa_d_atto?`<label class="ma-scelta"><input type="checkbox" name="presa_d_atto" value="1" data-step-check> <span>Prendo atto: ${esc(a.presa_d_atto)}</span></label>`:""}
      ${a.inviabile&&a.firma?`<label class="ma-scelta ma-firma"><input type="checkbox" name="autorizzo" value="1" data-step-check> <span>${esc(a.bottone||"Autorizzo lo STEP strutturale")}</span></label>`:(!a.spento&&!a.errore?`<p class="ma-spento">Il server dice che da qui non si può inviare: niente da firmare.</p>`:"")}`;
    piede = `${d.errore?`<p class="ma-errore" role="alert">${esc(d.errore)}</p>`:""}<button type="button" class="ma-btn" data-ma-azione="chiudi-dialogo">Chiudi senza autorizzare</button>
      <button type="button" class="ma-btn ma-btn--pri" data-ma-azione="step-invia" disabled>Autorizza</button>`;
  } else if(d.tipo==="albero"){
    const r = d.r; sopra = "Conferma dell'albero"; titolo = "Riepilogo dell'albero";
    const TIT = [["nuovi","Componenti nuovi"],["ritrovati","Componenti ritrovati"],["tipi","Tipi"],["revisioni","Revisioni"],["legami_nuovi","Legami nuovi"],["legami_tolti","Legami tolti"],["quantita","Quantità"],["cascata","Tolti a cascata"],["fuori","Fuori dall'albero"],["scartate","Proposte scartate"],["rimozioni","Rimozioni"],["commerciali","Commerciali"],["vicini","Codici quasi uguali"],["quantita_discordi","Quantità discordi"],["senza_codice","Senza codice"]];
    const descrivi = x => typeof x!=="object"||x===null ? String(x) : Object.entries(x).filter(([,v])=>v!=null&&v!=="").sort(([a],[b])=>(a==="codice"?-1:b==="codice"?1:0)).map(([k,v])=>`${k}: ${typeof v==="object"?JSON.stringify(v):v}`).join(" · ");
    const vuote = TIT.filter(([k])=>!(r[k]||[]).length).map(([,t])=>t);
    corpo = r.errLettura ? `<p class="ma-errore">Il riepilogo non si legge: ${esc(r.errLettura)}</p>` : `
      <p class="ma-nota">Si conferma l'albero com'è adesso, dopo i gesti fatti nodo per nodo. Vale per tutta la RFQ, non solo per il prodotto scelto. È la conferma della struttura: non conferma la fonte e non conferma i documenti.</p>
      <p class="ma-nota">Accettare i nodi uno per uno non rende la BOM verificata: lo fa solo questa conferma.</p>
      ${r.blocchi.length?`<div class="ma-errore"><b>Prima di confermare va risolto:</b><ul class="ma-blocchi">${r.blocchi.map(b=>`<li>${esc(descrivi(b))}</li>`).join("")}</ul></div>`:""}
      ${r.confermabile===true?"":`<p class="ma-spento">Il server dice che l'albero non si può ancora confermare.</p>`}
      ${TIT.filter(([k])=>(r[k]||[]).length).map(([k,t])=>`<details class="ma-riep"><summary><span>${t}</span><b>${r[k].length}</b></summary><ul>${r[k].map(x=>`<li>${esc(descrivi(x))}</li>`).join("")}</ul></details>`).join("")}
      ${vuote.length?`<p class="ma-muto ma-piccolo">Nessuna voce in: ${vuote.join(" · ")}.</p>`:""}`;
    piede = `${d.errore?`<p class="ma-errore" role="alert">${esc(d.errore)}</p>`:""}<button type="button" class="ma-btn" data-ma-azione="chiudi-dialogo">Chiudi senza confermare</button>
      <button type="button" class="ma-btn ma-btn--pri" data-ma-azione="albero-invia" ${r.confermabile===true&&r.firma?"":"disabled"}>Conferma l'albero</button>`;
  } else if(d.tipo==="tipo"){
    const a = d.a; sopra = "Cambio del tipo · prima l'effetto"; titolo = `${d.codice}: tipo ${etichetta("tipo_componente",d.tipoNuovo)}`;
    corpo = `${a.errore?`<p class="ma-errore">${esc(a.errore)}</p>`:""}${a.spento?`<p class="ma-spento">Non si può cambiare: ${esc(a.spento)}</p>`:""}
      ${(a.frasi||[]).length?`<ul class="ma-frasi">${a.frasi.map(x=>`<li>${esc(x)}</li>`).join("")}</ul>`:`<p class="ma-muto">Il server non segnala altri effetti.</p>`}`;
    piede = `${d.errore?`<p class="ma-errore" role="alert">${esc(d.errore)}</p>`:""}<button type="button" class="ma-btn" data-ma-azione="chiudi-dialogo">Chiudi senza cambiare</button>
      <button type="button" class="ma-btn ma-btn--pri" data-ma-azione="tipo-invia" ${a.firma&&!a.spento&&!a.errore?"":"disabled"}>${esc(a.bottone||"Cambia il tipo")}</button>`;
  } else if(d.tipo==="sposta"){
    sopra = "Sposta · conferma"; titolo = "Sposta il componente";
    corpo = `<form class="ma-gesto" data-ma-gesto="${esc(d.url)}"><p>Spostare <b class="mono">${esc(d.cod)}</b> da sotto <b class="mono">${esc(d.daCod)}</b> a sotto <b class="mono">${esc(d.versoCod)}</b>?</p>
      <p class="ma-muto ma-piccolo">Che cosa può stare sotto che cosa lo decide il server: se non è ammesso, il rifiuto lo dice e niente cambia.</p>
      <input type="hidden" name="da" value="${esc(d.da)}"><input type="hidden" name="a" value="${esc(d.a)}">
      <label class="ma-campo ma-campo--corto"><span>Qtà</span><input name="qta" type="number" required min="1" step="1" inputmode="numeric"></label>
      <button class="ma-btn ma-btn--pri">Sposta</button><small class="ma-muto">Scrivi la quantità: non c'è un valore già messo.</small></form>`;
    piede = `<button type="button" class="ma-btn" data-ma-azione="chiudi-dialogo">Annulla</button>`;
  } else if(d.tipo==="ispeziona"){
    const v = p.documenti.voci[d.i]; if(!v){ S.dialogo=null; return ""; }
    const nome = v.disegni&&v.disegni.primario ? v.disegni.primario.nome_file : v.file_candidato ? IX.NomeFile(v.file_candidato) : "";
    sopra = `${etichetta("tipo_documento",v.tipo_documento)} · ${v.codice}`; titolo = nome||"nessun file";
    corpo = `<dl class="ma-dl"><dt>Conferma documentale</dt><dd>${st("esito_fabbisogno",v.esito)} ${motivo("motivo_fabbisogno",v.motivo)}</dd>
        <dt>Copia sul NAS</dt><dd>${v.stato_nas?st("stato_nas",v.stato_nas):"—"}</dd>
        <dt>Associazione</dt><dd>${v.disegni&&v.disegni.primario?st("origine_dato",v.disegni.primario.provenienza):"—"}</dd>
        <dt>Regola</dt><dd>${v.bloccante?"bloccante":"—"}${v.invariante?" · 2D invariante":""}</dd></dl>
      ${nome&&v.disegni&&v.disegni.primario?`<div class="ma-palco ma-palco--piccolo">${schizzo(nome)}</div>`:""}`;
    const pa = v.proposta_aperta, did = v.documento_id;
    piede = `<div class="ma-dialogo-gesti">
      ${pa?`<form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/assegna"><input type="hidden" name="proposta" value="${pa}">
        <label class="ma-campo ma-campo--largo"><span>Componente</span><select name="componente" required><option value="" disabled selected>scegli il componente</option>${opzioniComponenti(T,IX,p,"")}</select></label>
        <button class="ma-btn ma-btn--sm">${ic("link",13)} Associa a questo componente</button><small class="ma-muto">Associazione a mano, non confermata.</small></form>`:""}
      ${pa||did?`<form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/assegna" data-ma-domanda="Togliere ${esc(nome||"il file")} da ${esc(v.codice)}? Il file resta nel thread e non viene scartato: perde solo il componente.">
        <input type="hidden" name="componente" value="">${pa?`<input type="hidden" name="proposta" value="${pa}">`:`<input type="hidden" name="documento" value="${did}">`}<button class="ma-btn ma-btn--sm">Togli dal componente</button></form>`:""}
      ${(()=>{ const corr = did && IX.Correnti ? IX.Correnti(v.componente_id, v.tipo_documento).filter(x=>x.documento_id!==did) : [];
        return corr.length ? `<form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/documento/${did}/sostituisci" data-ma-domanda="Rendere ${esc(nome||"questo documento")} il disegno corrente di ${esc(v.codice)}? Quello sostituito resta nella storia.">
          <label class="ma-campo"><span>Sostituisce</span><select name="vecchio" required><option value="" disabled selected>scegli il documento corrente</option>${corr.map(x=>`<option value="${x.documento_id}">${esc(x.nome_file)}${x.rev?" · rev "+esc(x.rev):""}</option>`).join("")}</select></label>
          <label class="ma-campo"><span>Motivo</span><input name="motivo" required maxlength="200"></label><button class="ma-btn ma-btn--sm">Sostituisci</button></form>` : ""; })()}
      ${v.deroga_id?`<button type="button" class="ma-btn ma-btn--sm" data-ma-gesto="/thread/${T.id}/fascicolo/deroga/${v.deroga_id}/revoca" data-ma-domanda="Revocare la deroga? Il requisito torna a contare.">Revoca la deroga</button>`
        : v.esito!=="presente"?`<form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/componente/${v.componente_id}/deroga"><input type="hidden" name="tipo" value="${v.tipo_documento}">
          <label class="ma-campo"><span>Motivo della deroga</span><input name="motivo" required maxlength="200"></label><button class="ma-btn ma-btn--sm">Deroga</button><small class="ma-muto">Una deroga non sostituisce il 2D.</small></form>`:""}
      ${pa?`<div class="ma-gesti-pericolo"><button type="button" class="ma-btn ma-btn--sm ma-btn--danger" data-ma-gesto="/proposta/${pa}/scarta" title="Lo scarto del file è definitivo (DB7-8)." data-ma-domanda="Scartare il file ${esc(nome||"proposto")}? Lo scarto è definitivo: nessun gesto lo riporta indietro.">Scarta il file</button></div>`:""}
      <button type="button" class="ma-btn ma-btn--ghost" data-ma-azione="chiudi-dialogo">Chiudi</button></div>`;
  } else if(d.tipo==="associa"){
    const pid = IX.PropostaAperta(d.allegato), nome = IX.NomeFile(d.allegato);
    sopra = "Associa a un componente"; titolo = nome;
    corpo = `<p class="ma-hint">Scegli il componente. L'associazione è a mano e non è una conferma: il file resta da confermare.</p>
      ${pid?`<form class="ma-gesto" data-ma-gesto="/thread/${T.id}/fascicolo/assegna"><input type="hidden" name="proposta" value="${pid}">
        <label class="ma-campo ma-campo--largo"><span>Componente</span><select name="componente" required><option value="" disabled selected>scegli il componente</option>${opzioniPerFile(T,IX,p,d.allegato)}</select></label>
        <button class="ma-btn ma-btn--pri ma-btn--sm">${ic("link",13)} Associa a questo componente</button></form>`:`<p class="ma-muto">Nessuna proposta aperta per questo file.</p>`}`;
    piede = `<button type="button" class="ma-btn ma-btn--ghost" data-ma-azione="chiudi-dialogo">Chiudi</button>`;
  } else if(d.tipo==="visore"){
    sopra = "Visore · sola lettura"; titolo = d.file;
    corpo = `<div class="ma-palco ma-palco--grande">${schizzo(d.file,"grande")}</div><p class="ma-hint">Nel Cockpit vero qui c'è la pagina del PDF (pdf.js). Le note numerate legate alle fasi arrivano con il Ciclo (R-14).</p>`;
    piede = `<button type="button" class="ma-btn ma-btn--ghost" data-ma-azione="chiudi-dialogo">Chiudi</button>`;
  }
  return `<div class="velo" data-ma-azione="chiudi-velo"></div><div class="ma-finestra" role="dialog" aria-modal="true" aria-label="${esc(titolo)}">
    <header class="ma-dialogo-testa"><div><span class="lab">${esc(sopra)}</span><h2${["step","visore","ispeziona","associa"].includes(d.tipo)?' class="mono"':""}>${esc(titolo)}</h2></div>
      <button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-ma-azione="chiudi-dialogo" aria-label="Chiudi">${ic("x",15)}</button></header>
    <div class="ma-dialogo-corpo">${corpo}</div><div class="ma-dialogo-piede">${piede}</div></div>`;
}
function finestra(T, p){ return finestraStruttura(T, p); }
function dopoGestoOk(){ if(S.dialogo && ["ispeziona","associa","sposta"].includes(S.dialogo.tipo)) S.dialogo = null; }

/* ================================================================ le azioni della pagina (data-ma-azione): solo letture e presentazione */
function apriStep(btn){
  const T = threadAperto(), ds = btn.dataset;
  const d = { tipo:"step", file:ds.maFile, codice:ds.maCodice, cid:ds.maComponente, nodo:ds.maNodo, sorgente:ds.maSorgente, a:null, errore:null };
  try { d.a = leggiSubito(`/thread/${T.id}/motore-a/anteprima/step-strutturale?componente=${encodeURIComponent(d.cid)}&nodo=${encodeURIComponent(d.nodo)}&sorgente=${encodeURIComponent(d.sorgente)}`); }
  catch(e){ d.a = {errLettura:e.message}; }
  S.dialogo = d; render();
}
function inviaStep(){
  const T = threadAperto(), d = S.dialogo, a = d.a;
  const campi = { nodo:d.nodo, sorgente:d.sorgente, firma:a.firma, autorizza:"1" };
  if(document.querySelector('.ma-finestra input[name=presa_d_atto]:checked')) campi.presa_d_atto = "1";
  const r = eseguiSubito(`/thread/${T.id}/fascicolo/componente/${d.cid}/step-strutturale`, campi);
  if(r.esito==="ok"){ S.dialogo = null; S.scheda = null; S.casella = null; S.fonteAperta = null; render(); return; }
  apriStep({dataset:{maFile:d.file, maCodice:d.codice, maComponente:d.cid, maNodo:d.nodo, maSorgente:d.sorgente}});
  S.dialogo.errore = r.testo || "L'autorizzazione non è riuscita: niente è cambiato."; render();
}
function apriAlbero(errore){
  const T = threadAperto(); let r;
  try { r = leggiSubito(`/thread/${T.id}/distinta/albero/riepilogo`); } catch(e){ r = {errLettura:e.message, blocchi:[]}; }
  S.dialogo = { tipo:"albero", r, errore:errore||null }; render();
}
function inviaAlbero(){
  const r = S.dialogo.r;
  const x = confermaAlberoSubito(r.firma, { formato:1, base:r.albero_firma });
  /* P-03: dopo la conferma il ponte rilegge la pagina con lo stesso URL: nessuna vista scelta dal JS guardando i dati */
  if(x.esito==="ok"){ S.dialogo = null; render(); return; }
  apriAlbero(x.testo || "La conferma non è riuscita: niente è cambiato.");     /* rifiutata (422): si rilegge il riepilogo */
}
function apriTipo(form){
  const T = threadAperto(), sel = form.elements.tipo, tipo = sel && sel.value;
  if(!tipo){ if(sel && sel.focus) sel.focus(); return; }
  let a; try { a = leggiSubito(`/thread/${T.id}/distinta/tipo?componente=${encodeURIComponent(form.dataset.maComponente)}&tipo=${encodeURIComponent(tipo)}`); } catch(e){ a = {errore:e.message}; }
  S.dialogo = { tipo:"tipo", cid:form.dataset.maComponente, codice:form.dataset.maCodice, tipoNuovo:tipo, a }; render();
}
function inviaTipo(){
  const T = threadAperto(), d = S.dialogo;
  const r = eseguiSubito(`/thread/${T.id}/fascicolo/componente/${d.cid}/tipo`, { tipo:d.tipoNuovo, firma:d.a.firma });
  if(r.esito==="ok"){ S.dialogo = null; render(); return; }
  apriTipo({ elements:{tipo:{value:d.tipoNuovo}}, dataset:{maComponente:d.cid, maCodice:d.codice} }); S.dialogo.errore = r.testo; render();
}

/* tasti dell'albero: come un organigramma; nessuna lettera scrive */
function tastiStruttura(e){
  if(S.vista==="rfq") return;
  if(e.key==="[" || e.key==="]"){
    const T=threadAperto(), p=prodottoScelto(T), g=indiceDi(T).Gruppi(p.rif); if(!g.length) return;
    const i = Math.max(0, g.findIndex(x=>x.chiave===(S.scheda||g[0].chiave)));
    S.scheda = g[(i + (e.key==="]"?1:g.length-1)) % g.length].chiave; render(); e.preventDefault(); return;
  }
  const li = document.activeElement && document.activeElement.closest && document.activeElement.closest('li[role="treeitem"]');
  if(!li) return;
  const padre = li.parentElement.closest('li[role="treeitem"]');
  const figli = [...li.querySelectorAll(':scope > ul > li[role="treeitem"]')];
  const fratelli = [...li.parentElement.children].filter(x=>x.matches('li[role="treeitem"]'));
  const i = fratelli.indexOf(li);
  let dest = null;
  const tutte = [...li.closest(".ma-dtree").querySelectorAll('li[role="treeitem"]')].filter(x=>x.offsetParent!==null || !x.closest('li[aria-expanded="false"] > ul'));
  if(e.key==="ArrowDown") dest = figli[0]; else if(e.key==="ArrowUp") dest = padre; else if(e.key==="ArrowRight") dest = fratelli[i+1]; else if(e.key==="ArrowLeft") dest = fratelli[i-1];
  else if(e.key==="Home") dest = tutte[0]; else if(e.key==="End") dest = tutte[tutte.length-1];
  else if(e.key==="Enter"){ S.casella = li.dataset.maChiave; render(); const x=document.querySelector(`li[data-ma-chiave="${(window.CSS&&window.CSS.escape)?window.CSS.escape(S.casella):S.casella}"]`); if(x) x.focus(); e.preventDefault(); return; }
  else if(e.key==="+"||e.key==="-"){ S.rami=S.rami||{}; S.rami[li.dataset.maCammino] = e.key==="+"; render(); e.preventDefault(); return; }
  if(dest){ li.tabIndex=-1; dest.tabIndex=0; dest.focus(); e.preventDefault(); }
}

document.addEventListener("click", e=>{
  const el = e.target.closest ? e.target.closest("[data-ma-azione]") : null;
  if(!el || el.tagName==="FORM") return;
  const a = el.dataset.maAzione, T = threadAperto();
  if(!T) return;
  switch(a){
    case "casella": { const li = el.closest('li[role="treeitem"]'); S.casella = li.dataset.maChiave; S.latoChiuso = false; render();
      if(window.innerWidth<1024){ const a=document.getElementById("ma-anteprima"); if(a&&a.scrollIntoView) a.scrollIntoView({block:"start"}); } return; }
    case "chiudi-anteprima": S.casella = null; render(); return;
    case "lato": S.latoChiuso = !S.latoChiuso; render(); return;
    case "scheda": S.scheda = el.dataset.maGruppo; S.casella = null; render(); return;
    case "vai-scheda": S.scheda = el.dataset.maGruppo; render(); { const t=document.getElementById("ma-schede-albero"); if(t&&t.scrollIntoView) t.scrollIntoView({block:"start"}); } return;
    case "ramo": S.rami = S.rami||{}; S.rami[el.dataset.maCammino] = (S.rami[el.dataset.maCammino]===false); render(); return;
    case "rami": { S.rami = {}; if(el.dataset.maAperti==="0"){ document.querySelectorAll('.ma-tela li[aria-expanded]').forEach(li=>{ S.rami[li.dataset.maCammino]=false; }); } render(); return; }
    case "zoom": { const z = el.dataset.z; let v = S.zoom||1;
      if(z==="+") v = Math.min(1.4, +(v+0.1).toFixed(2)); else if(z==="-") v = Math.max(0.5, +(v-0.1).toFixed(2));
      else { const tl=document.querySelector(".ma-tela"), tr=tl&&tl.querySelector(".ma-dtree"); if(tl&&tr&&tr.scrollWidth){ v = Math.max(0.5, Math.min(1, +((tl.clientWidth-24)/(tr.scrollWidth/(S.zoom||1))).toFixed(2))); } }
      S.zoom = v; render(); return; }
    case "fonte": { const conf = prodottoScelto(T).fonte.stato==="confermata"; const ora = S.fonteAperta===null ? !conf : S.fonteAperta; S.fonteAperta = !ora; render(); return; }
    case "step": apriStep(el); return;
    case "step-invia": inviaStep(); return;
    case "albero": apriAlbero(); return;
    case "albero-invia": inviaAlbero(); return;
    case "tipo-invia": inviaTipo(); return;
    case "ispeziona": S.dialogo = {tipo:"ispeziona", i:+el.dataset.maVoce}; render(); return;
    case "associa": S.dialogo = {tipo:"associa", allegato:el.dataset.maAllegato}; render(); return;
    case "pdf": case "visore": S.dialogo = {tipo:"visore", file:el.dataset.maFile, allegato:el.dataset.maAllegato||"", sha:el.dataset.maSha||""}; render(); return;
    case "chiudi-dialogo": case "chiudi-velo": S.dialogo = null; render(); return;
    case "aggiorna": aggiornaSubito(); PONTE.avviso = {esito:"ok", testo:"Pagina riletta dal server: niente è stato scritto."}; render(); return;
    case "chiudi-avviso": PONTE.avviso = null; render(); return;
  }
});
/* la riga d'aiuto scorre all'ancora nella stessa pagina (nessuna navigazione) */
document.addEventListener("click", e=>{ const a = e.target.closest && e.target.closest("[data-ma-scorri]"); if(!a) return; e.preventDefault(); scorriAncora(a.dataset.maScorri); });
document.addEventListener("change", e=>{
  if(e.target.matches && e.target.matches("[data-step-check]")){
    const a = S.dialogo && S.dialogo.a, ok = document.querySelector(".ma-finestra input[name=autorizzo]"), pa = document.querySelector(".ma-finestra input[name=presa_d_atto]");
    const b = document.querySelector('[data-ma-azione="step-invia"]');
    if(b) b.disabled = !(a && a.inviabile===true && a.firma && !a.spento && !a.errore && ok && ok.checked && (!pa || pa.checked));
  }
});
document.addEventListener("submit", e=>{
  const f = e.target;
  if(f && f.tagName==="FORM" && f.dataset && f.dataset.maAzione==="tipo"){ e.preventDefault(); apriTipo(f); }
  else if(f && f.tagName==="FORM" && f.closest && f.closest(".ma-finestra") && f.hasAttribute("data-ma-gesto")){ /* la finestra si chiude dopo l'esito (dopoGestoOk) */ }
});
/* trascinare = il gesto Sposta: apre il modulo, non sposta da solo (v1.1 §2.11), solo nelle schede bom e senza */
let trascinato = null;
document.addEventListener("dragstart", e=>{ const n = e.target.closest && e.target.closest(".ma-nodo[data-ma-sposta]"); if(!n) return; trascinato = n; if(e.dataTransfer){ e.dataTransfer.effectAllowed="move"; try{ e.dataTransfer.setData("text/plain", n.dataset.maCod||""); }catch(x){} } });
/* lasciarlo sul padre che ha già non è uno spostamento: niente finestra, solo l'indizio «è già sotto questo padre» */
const stessoPadre = t => !!(trascinato && t && trascinato.dataset.maDa && t.dataset.maDrop===trascinato.dataset.maDa);
document.addEventListener("dragover", e=>{ const t = e.target.closest && e.target.closest(".ma-nodo[data-ma-drop]"); if(trascinato && t && t!==trascinato){ if(stessoPadre(t)){ t.classList.add("ma-drop-no"); t.title = "è già sotto questo padre"; return; } e.preventDefault(); t.classList.add("ma-drop"); } });
document.addEventListener("dragleave", e=>{ const t = e.target.closest && e.target.closest(".ma-nodo[data-ma-drop]"); if(t){ t.classList.remove("ma-drop"); t.classList.remove("ma-drop-no"); } });
document.addEventListener("drop", e=>{
  const t = e.target.closest && e.target.closest(".ma-nodo[data-ma-drop]");
  if(!trascinato || !t || t===trascinato){ trascinato=null; return; }
  if(stessoPadre(t)){ t.classList.remove("ma-drop-no"); trascinato=null; return; }
  e.preventDefault();
  S.dialogo = { tipo:"sposta", url:trascinato.dataset.maSposta, cod:trascinato.dataset.maCod||"il componente", da:trascinato.dataset.maDa||"", daCod:trascinato.dataset.maDaCod||"la radice", a:t.dataset.maDrop, versoCod:t.dataset.maCod||"il componente scelto" };
  trascinato = null; render();
});
