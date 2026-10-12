/* ================= 54 · i cassetti: Richieste, Controlli, Cataloghi, Non ancora nel backend =================
   SPEC_v15 §2.0, §3.5, §5.3. Un cassetto di 440 px che SPINGE il foglio e non lo copre; la testata dice la
   destinazione una volta sola (A6). Cataloghi si apre sulla scheda utile al campo da cui lo chiami (M7): materiale →
   Materiali, saldatura o Sequenza → Componenti, Ciclo → Lavorazioni. Una riga di catalogo ha due livelli e il clic su
   tutta la riga è «Usa» (nessun pulsante ripetuto con il codice intero). */

function cassetto(T, p){
  const titolo = {richieste:"Richieste", controlli:"Controlli", cataloghi:"Cataloghi", backend:"Aiuto · non ancora nel backend"}[S.cassetto];
  const corpo = S.cassetto==="richieste" ? cassettoRichieste(T,p) : S.cassetto==="controlli" ? cassettoControlli(T,p) : S.cassetto==="cataloghi" ? cassettoCataloghi(T,p) : pannelloBackend(T,p);
  const dest = S.cassetto==="cataloghi" && S.catDest ? `<p class="cat-dest">${esc(destinazioneCat(T,p))}</p>` : "";
  return `<aside class="cassetto" id="cassetto" aria-label="${esc(titolo)}">
    <header class="cas-testa"><h2>${esc(titolo)}</h2><span class="sp"></span><button type="button" class="ma-btn ma-btn--sm ma-btn--ghost" data-cassetto="" aria-label="Chiudi il cassetto">${I("x",15)}</button></header>
    <div class="cas-corpo">${dest}${corpo}</div></aside>`;
}

/* ---------------- ⚠ Controlli: i motivi della zona A (letti) e i controlli della zona B (REGOLE del server A2) ---------------- */
function cassettoControlli(T, p){
  const cc = p ? controlliB(T,p) : [];
  const per = {}; cc.forEach(x=>{ (per[x.rcod||"RFQ"] = per[x.rcod||"RFQ"] || []).push(x); });
  const conta = l => ["KO","DB","AV"].map(k=>{ const n = cc.filter(x=>x.liv===k).length; return n?`<span class="liv liv--${k}">${n} ${NOME_LIV[k]==="dubbio"&&n>1?"dubbi":NOME_LIV[k]==="avviso"&&n>1?"avvisi":NOME_LIV[k]}</span>`:""; }).join(" ");
  return `<p class="cas-sotto">Zona A · perché il prodotto non è pronto (ProdottoValutato.Motivi, dal server):</p>
    ${T.esito.prodotti_valutati.map(x=>`<div class="cas-blocco"><b class="mono">${esc(x.codice_richiesto)}</b> ${st("stato_prodotto",x.stato)}
      <ul class="cas-elenco">${x.motivi.length?x.motivi.map(m=>`<li><span class="ma-st ma-st--ver">${esc(etichetta("motivo_prodotto",m))}</span></li>`).join(""):`<li class="k3">nessun motivo</li>`}</ul></div>`).join("")}
    ${bandaB("R-35","controlli della lavorazione: KO solo per l'impossibile, mai da un dato inventato")}
    <div class="cas-blocco"><span>${conta(cc)||"✓ nessun controllo aperto"}</span>
      ${(()=>{ /* v16 (SE §4.12): raggruppati per componente, chiusi, con il contatore; si apre quello su cui stai lavorando */
        const rc = p ? righeB(T,p) : [], curCid = S.staz===3 ? S.cicloCid : S.staz===5 ? S.esitoRiga : S.staz===2 ? S.comeRiga : null;
        const curCod = (rc.find(r=>r.cid===curCid)||{}).cod || (Object.entries(per).find(([,l])=>l.some(x=>x.liv==="KO"))||[])[0];
        const peg = l => ["KO","DB","AV"].find(k=>l.some(x=>x.liv===k));
        return Object.entries(per).map(([cod,l])=>`<details class="ctrl-cas"${cod===curCod?" open":""}><summary><b class="mono">${esc(cod)}</b> <span class="liv liv--${peg(l)||"AV"}">${l.length}</span></summary>${l.map(x=>`<div class="ctrl-riga"><span class="liv liv--${x.liv}">${NOME_LIV[x.liv]||x.liv}</span><span><code class="k3">${esc(x.cod)}</code> ${esc(x.testo.replace(cod+": ","").replace(cod+" ",""))}${x.inventato?" "+datoProva("dati di prova"):""}</span>${azioneCtrl(T,x.azione)}</div>`).join("")}</details>`).join(""); })()}</div>`;
}

/* ---------------- ▤ Cataloghi: Materiali (banca dati + magazzino + lavorabilità), Componenti, Lavorazioni, Terzisti ---------------- */
function destinazioneCat(T, p){
  const d = S.catDest; if(!d) return "";
  if(d==="r38") return "Scegli il componente da proporre in Sequenza (R-38)";
  if(d.startsWith("fid:")){ const x = faseDelThread(T, d.slice(4)); return x ? `Scegli il componente di catalogo per la fase ${(x.i+1)*10} (${PROC[x.f.p][0].toLowerCase()})` : ""; }
  const x = faseDelThread(T, d); if(!x) return "";
  const r = righeB(T,p).find(y=>y.cid===x.cid);
  return `Scegli il materiale per ${r?suffisso(r.cod,prefissoComune(righeB(T,p).map(y=>y.cod))):""} · fase ${(x.i+1)*10}`;
}
function cassettoCataloghi(T, p){
  const tab = S.catTab || "materiali", q = (S.catQ||"").toLowerCase();
  const tb = (k,l) => `<button type="button" class="cas-tab" data-cat-tab="${k}" aria-pressed="${tab===k}">${l}</button>`;
  let c = "";
  if(tab==="materiali"){
    const stato = g => { const giac = GIACENZE.filter(x=>x.g===g.id), acq = ACQUISTI.find(x=>x.g===g.id && !x.soloQuotato);
      return giac.length ? `● ${giac.reduce((a,x)=>a+x.qta-x.impegnato,0)} ${giac[0].um} · ${giac.map(x=>x.cert).join("/")}` : acq ? `○ già acquistato ${acq.data}` : "✕ mai"; };
    const lavora = g => { const cons = CONSUMI.filter(x=>x.g===g.id).flatMap(x=>x.lav); const L = k => cons.includes(k) ? "✓" : "○";
      return g.forma==="lamiera" ? `laser ${PARAM_LASER.some(x=>x.q===g.q&&x.sp===g.sp)?"✓":L("laser")} · piega ${UTENSILI_PIEGA.some(u=>g.sp>=u.sp[0]&&g.sp<=u.sp[1])?"✓":L("piega")}` : `sega ${L("sega")} · curva ✕`; };
    const gz = GREZZI.filter(g=>!q || (g.q+" "+testoGrezzo(Object.assign({},g,{D:g.D,sp:g.sp}))).toLowerCase().includes(q));
    c = `<p class="cas-sotto" title="Giacenza al ${esc(GIACENZA_AL)}: dati di prova; lavorabilità inventata">Grezzi a magazzino e già acquistati ${datoProva("dati di prova")}</p>
      ${gz.map(g=>{ const testoG = testoGrezzo({q:g.q, forma:g.forma, sp:g.sp, D:g.D, a:g.a, b:g.b, prof:g.prof}); const usa = S.catDest && /^f-\d+$/.test(S.catDest);
        return `<button type="button" class="cat-r" ${usa?`data-pa-gesto="/fase/${esc(S.catDest)}/materiale" data-pa-campi='${JSON.stringify({cerca:`${g.q} ${g.forma==="lamiera"?g.sp:g.forma==="profilo"?g.prof:g.forma==="tubo_tondo"?"tubo "+g.D+"x"+g.sp:g.forma==="tubo_rett"?"tubo "+g.a+"x"+g.b+"x"+g.sp:g.D}`})}'`:`data-cat-info="1"`} title="${usa?"Usa questo grezzo":"Apri un campo grezzo nel Ciclo per usarlo"}">
          <b>${esc(testoG)}</b><span class="mono k3">${esc(g.formato||(g.barra?"barra "+g.barra/1000+" m":""))}</span>
          <span class="cat-r2" title="${esc(g.norma)} · ${g.gest==="scorta"?"a scorta":"a commessa"}"><span>${esc(stato(g))}</span><span>${esc(lavora(g))}</span>${usa?"<b>Usa</b>":""}</span></button>`; }).join("")}
      <div class="cas-blocco"><span class="lab">Banca dati delle qualità · base → sottoqualità</span>
        ${MAT_DB.map(g=>`<details><summary class="piccolo">${esc(g.g)}</summary>${g.fam.map(f=>`<div class="cat-riga"><b class="mono">${esc(f.k)}</b><span class="cat-sub">${f.q.map(x=>`<span class="cat-q" title="${esc(x.k)} · ${esc(x.norma)}${x.nota?" · "+esc(x.nota):""}">${esc(x.s)}</span>`).join("")}</span><span class="dato prova">ρ ${fmt(f.rho)}</span></div>`).join("")}</details>`).join("")}</div>
      <div class="cas-blocco"><span class="lab" title="Mai un'alternativa al ribasso (MAT-05).">Equivalenze dei clienti</span>${EQUIVALENZE.map(e=>`<span class="piccolo">«${esc(e.nome)}» (${esc(e.cli)}) → <b>${esc(e.q)}</b> <span class="ma-st ma-st--${e.stato==="confermata"?"ok":"ver"}">${esc(e.stato)}</span></span>`).join("")}</div>`;
  } else if(tab==="componenti"){
    const fam = S.catFam || "";
    const l = COMPONENTI.filter(it=>(!fam||it.fam===fam) && (!q || (it.cod+" "+it.desc).toLowerCase().includes(q)));
    c = `<p class="cas-sotto" title="Entrano in una fase del padre; nell'albero solo come proposta R-38. MINUTERIA non esenta dal 2D (R103 C).">Componenti acquistati: li compra Fabio.</p>
      <div class="cat-tabs">${[["","tutte"],...FAM_COMP].map(([k,n])=>`<button type="button" class="cas-tab" data-cat-fam="${k}" aria-pressed="${fam===k}">${esc(n)}</button>`).join("")}</div>
      ${l.map(it=>`<button type="button" class="cat-r" ${S.catDest==="r38"?`data-cat-usa="${it.id}"`:S.catDest&&S.catDest.startsWith("fid:")?`data-pa-gesto="/fase/${esc(S.catDest.slice(4))}" data-pa-campi='${JSON.stringify({catalogo:`${it.cod} · ${it.desc}`.replace(/'/g,"’"), fonte:"L"})}'`:`data-cat-info="1"`}><b>${esc(it.desc)}</b><span class="mono k3">${esc(it.cod)}</span>
        <span class="cat-r2">${it.norma?`<span>${esc(it.norma)}</span>`:""}${it.fin?`<span>${esc(it.fin)}</span>`:""}${(it.cli||[]).map(([cl,cod])=>`<span>codice ${esc(cl)} ${esc(cod)}</span>`).join("")}${it.gruppo?`<span>saldabile · gruppo ${esc(it.gruppo)}${it.zonaSaldata?" · zona "+it.zonaSaldata+" mm":""} ${datoProva("inventato")}</span>`:""}${(it.usato||[]).length?`<span>visto in ${esc(it.usato.join(", "))}</span>`:""}${S.catDest==="r38"||(S.catDest||"").startsWith("fid:")?"<b>Usa</b>":""}</span></button>`).join("")}`;
  } else if(tab==="lavorazioni"){
    c = `<p class="cas-sotto" title="Nel Ciclo: «/» e il nome. ★ = serve a Franco.">Le lavorazioni del ciclo e i campi della loro famiglia.</p>
      ${LAVORAZIONI.map(([g,gn,l])=>`<div class="cas-blocco"><span class="lab">${esc(gn)}</span>${l.filter(([k,n])=>!q||n.toLowerCase().includes(q)).map(([k,n,t])=>`<span class="piccolo"><b>${esc(n)}</b>${t==="est"?" · terzisti":""} · ${(CAMPI_FAM[k]||[]).map(x=>x[1].toLowerCase()+(x[3]?" ★":"")).join(", ")}</span>`).join("")}</div>`).join("")}
      <div class="cas-blocco"><span class="lab">Capacità dell'officina ${datoProva("inventato")}</span>
        ${LIMITI_TAGLIO.map(x=>`<span class="piccolo">${esc(x.lav)} · ${esc(x.gruppo)} · ${x.spMax?"fino a "+x.spMax+" mm":x.dim?esc(x.dim):""} · ${esc(x.nota)}</span>`).join("")}
        ${UTENSILI_PIEGA.map(x=>`<span class="piccolo">piega · ${esc(x.nota)}</span>`).join("")}${SET_CURVA.map(x=>`<span class="piccolo">curvatura · ${esc(x.nota)}</span>`).join("")}
        ${WPS.map(w=>`<span class="piccolo">WPS ${esc(w.id)} · ${esc(w.proc)} · gruppo ${w.gruppo.join("/")} · ${w.sp[0]}–${w.sp[1]} mm · ${w.giunti.join("/")} · ${esc(w.norma)} · ${(QUALIFICHE.find(x=>x.wps===w.id)||{n:0}).n} saldatori qualificati</span>`).join("")}</div>`;
  } else {
    const cli = (CLIENTI.find(x=>x.nome===T.testa.cliente)||{}).id;
    c = `<p class="cas-sotto" title="nel backend: cliente_fornitore_lavorazione">Terzisti e fornitori di attrezzature, con lavorazioni e qualifiche per cliente.</p>
      ${TERZISTI.filter(t=>!q||t.nome.toLowerCase().includes(q)).map(t=>`<div class="cat-r" style="cursor:default"><b>${esc(t.nome)}</b><span class="k3 piccolo">${esc(t.lav||"")}</span>
        <span class="cat-r2">${(t.lavs||[]).map(k=>`<span>${esc((PROC[k]||[k])[0])}${cli&&((t.qual||{})[cli]||[]).includes(k)?" ✓ qualificato per "+esc(T.testa.cliente):""}</span>`).join("")}${TERZISTI_CAP.filter(x=>x.terz===t.id).map(x=>`<span>${esc(x.cosa)} · ${esc(x.pesoMax)} ${datoProva("inventato")}</span>`).join("")}</span></div>`).join("")}
      <div class="cas-blocco"><span class="lab">Fornitori di materia prima</span>${FORNITORI_MP.map(f=>`<span class="piccolo">${esc(f.nome)} · ${f.forme.map(x=>(FORME.find(y=>y[0]===x)||[,x])[1].toLowerCase()).join(", ")} · ${f.cert.join("/")} · ${f.gg} gg ${datoProva("dati di prova")}</span>`).join("")}</div>`;
  }
  return `${bandaB("R-23 · R-24 · R-26 · R-28","dati di prova: nel vero si leggono da GestX e dall'officina")}
    <div class="cat-tabs" role="group" aria-label="Cataloghi">${tb("materiali","Materiali")}${tb("componenti","Componenti acquistati")}${tb("lavorazioni","Lavorazioni")}${tb("terzisti","Terzisti")}</div>
    <label class="cerca-b">${I("search",14)}<input id="cat-q" value="${esc(S.catQ||"")}" placeholder="cerca nel catalogo" aria-label="Cerca nel catalogo"></label>${c}`;
}

/* ---------------- ⓘ Non ancora nel backend: che cosa è vero, JSON nella forma di /motore-a/dati, gesti A e B ---------------- */
const PROPOSTE_STAZ = {
  1:[["Tipo di pratica e lotti (testata ◇)","R-19 · R-18"],["Chiedi al cliente (2D mancante, DWG)","R-41"]],
  2:[["Come si fa (fornitura)","R-22"],["Modelli di ciclo T1…T9","R-39 · PRO-06"],["No quote dallo smistamento","D-01"],["Richieste preparate qui","R-13 · R-27"]],
  3:[["Prodotto finito come primo disegno (dato A letto: Voce 2D o nodo radice)","A1d"],["Pre-unioni: più fasi di unione con «Entrano» a scelta","R-10 · R-36"],["Richiedi la maschera e le altre preparazioni","R-11 · R-13"],["Ciclo, fasi a righe, una scrittura per fase","R-10 · R-36"],["Grezzo nella prima fase, banca dati dei materiali","R-23 · R-25"],["Magazzino e fabbisogno (anteprime)","R-24 · R-37"],["Lavorabilità","R-26 · R-28"],["Saldatura, giunti, WPS","R-29 · R-30"],["Preparazioni","R-11"],["Note sul disegno legate alla fase","R-14"],["Requisiti nelle fasi","R-20 · R-34"]],
  4:[["Albero del processo (lettura delle fasi e di chi entra)","R-37 · R-32"],["Sequenza scritta a mano, non riproposta; «Ripristina la proposta»","R-10 · R-35"],["Conferma con la firma di quello che si è letto","R-32 · R-36"],["Sequenza, stadi e viaggi","R-32 · R-33"],["Diventa assieme saldato","R-31"],["Manca nell'albero","R-38"]],
  5:[["Attrezzatura letta dalle preparazioni; «Richiedi le N preparazioni senza costo»","R-11 · R-13"],["Esito per componente","D-01 · R-35"],["Firme dell'area tecnica e delle altre","D-02"],["Consegna per lotto (fotografia)","R-40"],["Foglio per Franco, costi, offerta","dopo A2"]],
};
function pannelloBackend(T, p){
  const tab = S.cassettoTab||"spiegazione";
  const tb = (k,l) => `<button type="button" class="cas-tab" data-castab="${k}" aria-pressed="${tab===k}">${l}</button>`;
  let c = "";
  if(tab==="tasti") c = tastiHtml();
  else if(tab==="spiegazione") c = `
    <div class="cas-blocco"><span class="lab">In questa stazione (${esc(STAZIONI[S.staz-1][1])})</span><ul class="cas-elenco">${(PROPOSTE_STAZ[S.staz]||[]).map(([t,r])=>`<li><b>${esc(t)}</b> · ${esc(r)} · proposta, il backend di oggi non lo ha</li>`).join("")}</ul></div>
    <div class="cas-blocco"><span class="lab">Zona A · contratto A1d</span><p>Struttura e documenti: dati e gesti veri, attraverso il finto ponte con la lista chiusa di Pietro (29 rotte, «Conferma l'albero», 4 letture). Il finto server dice anche «no».</p></div>
    <div class="cas-blocco"><span class="lab">Zona B · proposta dopo A2</span><ul class="cas-elenco">
      <li><b>Come</b> · R-22 R-39 R-13 R-27</li><li><b>Ciclo, grezzo, banca dati dei materiali, magazzino, lavorabilità</b> · R-10 R-23 R-24 R-25 R-26 R-28</li>
      <li><b>Saldatura, pre-unioni e preparazioni (una tabella: attrezzatura_componente)</b> · R-10 R-11 R-29 R-30</li><li><b>Sequenza</b> · R-31 R-32 R-33 R-38</li><li><b>Esito, firme, consegna</b> · D-01 D-02 R-35 R-40</li>
      <li><b>Richieste e risposte</b> · R-13 R-17 R-27 R-41</li><li><b>Tipo di pratica e lotti</b> (in testata, ◇) · R-19 R-18</li></ul>
      <p class="k3 piccolo">Un secondo finto server (A2) calcola gli stati della zona B; i gesti usano data-pa-gesto e rotte immaginate, mai una rotta della lista A.</p></div>
    <div class="cas-blocco"><span class="lab">Zona C · fuori, spenta con il motivo</span><ul class="cas-elenco">
      <li>Conferma cumulativa del fascicolo · spazio di verifica</li><li>Conferma della minuteria · R-08</li><li>Caricamento dalla riga · R-06</li>
      <li>«Cerca di nuovo sul portale»</li><li>Rinomina del prodotto · DB7-3</li><li>Aggiunte di pezzi · DB7-5, DB7-7</li><li>Congela la V1</li><li>Passaggi di fase (anche dall'Offerta)</li></ul></div>`;
  else if(tab==="json"){ let j; try { j = leggiSubito(`/thread/${T.id}/motore-a/dati`); } catch(err){ j = {errore:String(err.message)}; }
    c = `<p class="cas-sotto">GET /thread/{id}/motore-a/dati, nella forma vera (chiavi del contratto A1d §2.4). Contiene dati del cliente: solo per l'admin. La zona B non è qui: il backend non la ha.</p><pre class="cas-json">${esc(JSON.stringify(j,null,1))}</pre>`; }
  else c = `<p class="cas-sotto">Le chiamate fatte in questa sessione. Zona A = rotte della lista chiusa; zona B = rotte immaginate (data-pa-gesto).</p>
    ${REGISTRO.length?`<ul class="cas-reg">${REGISTRO.map(x=>`<li><span class="chip ${x.zona==="A"?"acc":"pa"}">${x.zona}</span><span class="mono">${esc(x.metodo)} ${esc(x.url)}</span><span class="k3">${esc(x.quando)}</span>${x.esito&&x.esito.esito?`<span class="ma-st ma-st--${x.esito.esito==="ok"?"ok":x.esito.esito==="no"?"ver":x.esito.esito==="lettura"?"neu":"ko"}">${esc(x.esito.esito)}</span>`:""}</li>`).join("")}</ul>`:`<p class="k3">Nessuna chiamata: aprire la Distinta non scrive niente (DB7-6).</p>`}`;
  return `<div class="cas-tabs" role="group" aria-label="Viste del pannello">${tb("tasti","Tasti")}${tb("spiegazione","Che cosa è vero")}${tb("json","JSON")}${tb("gesti","Gesti")}</div>${c}`;
}
document.addEventListener("click", e=>{
  const ti = e.target.closest && e.target.closest("[data-cat-info]");
  if(ti){ PA.avviso = {esito:"no", testo:"Per usare una riga apri il catalogo da un campo del Ciclo (grezzo, saldatura, montaggio) o da «Manca nell'albero»: la destinazione compare in testa al cassetto (M7)."}; render(); return; }
  const t = e.target.closest && e.target.closest("[data-cat-tab],[data-cat-fam],[data-cat-usa],[data-cat-dest],[data-cat-apri]");
  if(!t) return;
  const D = t.dataset;
  if(D.catApri){ S.cassetto = "cataloghi"; S.catTab = D.catApri.split("|")[0]; S.catDest = D.catApri.split("|")[1]||null; render(); return; }
  if(D.catTab){ S.catTab = D.catTab; if(D.catDest) S.catDest = D.catDest; if(t.hasAttribute("data-cassetto")) S.cassetto = "cataloghi"; render(); return; }
  if(D.catFam!==undefined){ S.catFam = D.catFam; render(); return; }
  if(D.catUsa){ const it = compDi(D.catUsa); S.r38descr = `${it.cod} · ${it.desc}`; S.catDest = null; S.cassetto = window.innerWidth<1280 ? null : S.cassetto; S.staz = 4; PA.avviso = {esito:"ok", testo:`«${it.cod}» nel modulo «Manca nell'albero» (R-38): scegli sotto chi e la quantità, poi «Proponi». Niente è ancora scritto.`}; render(); return; }
});
document.addEventListener("input", e=>{ if(e.target.id==="cat-q"){ S.catQ = e.target.value; const pos = e.target.selectionStart; render(); const i = document.getElementById("cat-q"); if(i){ i.focus(); try{ i.setSelectionRange(pos,pos); }catch(err){} } } });
