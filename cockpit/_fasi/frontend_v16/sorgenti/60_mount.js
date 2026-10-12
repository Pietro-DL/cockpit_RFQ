/* ================= 60 · montaggio: render e navigazione =================
   Le scritture della zona A NON passano di qui: le fa solo il finto ponte (15_ponte_a.js) da [data-ma-gesto].
   Qui ci sono la navigazione, l'Inbox, le impostazioni e i gesti dell'Inbox (aggancio, nuova richiesta, RFQ). */
let vistaPrima = "";
/* A-1, M-4 (revisione frontend): ogni gesto ridisegna, ma chi lavora non deve perdere il posto. Si ricordano lo scorrimento
   (verticale e orizzontale) di tutte le colonne che scorrono, e l'elemento con il fuoco; dopo il disegno si rimettono. */
const SCORRONO = [".ws-main", ".ma-ws", ".ma-ws-centro", "#ma-anteprima", ".ma-tela", "#cl-scroll", ".cl-elenco", ".seq16-tela", ".seq16-pannello", ".b-corpo", "#cassetto .cas-corpo"];
function firmaFuoco(el){
  if(!el || el===document.body || !el.tagName) return null;
  if(el.id) return {sel:"#"+(window.CSS&&CSS.escape?CSS.escape(el.id):el.id)};
  /* i data-* stabili (non i campi del gesto, che cambiano con lo stato), più il testo per distinguere i fratelli */
  const at = [...el.attributes].filter(a=>/^data-/.test(a.name) && a.name!=="data-pa-campi");
  if(!at.length) return null;
  const sel = el.tagName.toLowerCase()+at.map(a=>`[${a.name}="${String(a.value).replace(/\\/g,"\\\\").replace(/"/g,'\\"')}"]`).join("");
  const testo = x => (x.textContent||"").replace(/[✓◇]/g,"").replace(/\s+/g," ").trim();
  return {sel, testo:testo(el), testoDi:testo};
}
function rimettiFuoco(f){
  if(!f) return;
  try{ const l = [...document.querySelectorAll(f.sel)]; const el = f.testoDi ? (l.find(x=>f.testoDi(x)===f.testo) || (l.length===1?l[0]:null)) : l[0];
    if(el && el.focus) el.focus({preventScroll:true}); }catch(e){}
}
function render(){
  const vista = [S.tab, S.rfq, S.staz, S.vista, S.prodotto].join("|"), stessa = vista===vistaPrima;
  const pos = stessa ? SCORRONO.map(sel=>{ const el = document.querySelector(sel); return el ? [sel, el.scrollTop, el.scrollLeft] : null; }).filter(Boolean) : [];
  const fuoco = stessa ? firmaFuoco(document.activeElement) : null;
  vistaPrima = vista;
  renderTabs();
  const b = $("body"), sh = $("shell");
  sh.classList.toggle("ws-on", S.tab==="rfq");
  if(S.tab==="inbox"){
    b.innerHTML = `<div class="panes${S.inboxRail?" panes--rail":""}${S.inboxDati?" panes--dati":""}"><aside class="pane p-rail">${rail()}</aside><section class="pane p-list">${pannelliInbox()}${lista()}</section>
      <section class="pane p-conv">${conversazione()}</section><aside class="pane p-ctx">${dati()}</aside></div>`;
  } else if(S.tab==="nuovareq"){
    b.innerHTML = `<div class="rfqbody">${nuovaRichiesta()}</div>`;
  } else if(S.tab==="gest"){
    b.innerHTML = gestione();
  } else if(S.tab==="rfq"){
    b.innerHTML = paginaDistinta();
    for(const [sel, top, left] of pos){ const el = document.querySelector(sel); if(el){ if(top) el.scrollTop = top; if(left) el.scrollLeft = left; } }
    segnaBordiTela();
    rimettiFuoco(fuoco);
    if(typeof dopoRender==="function") dopoRender();
    if(typeof adattaAlberoProcesso==="function" && !stessa) setTimeout(adattaAlberoProcesso, 0);
  } else {
    const due = S.setSez!=="clienti";
    b.innerHTML = `<div class="set${due?" due":""}"><aside class="pane p-rail">${setRail()}</aside>${due?"":`<section class="pane p-list">${setLista()}</section>`}
      <section class="pane p-conv">${due?setNascosti():formCliente()}</section></div>`;
  }
}
/* la sfumatura sul bordo della tela quando c'è altro da scorrere (C3, A7): presentazione */
function segnaBordiTela(){
  const tl = document.querySelector(".ma-tela"); if(!tl) return;
  const f = () => { tl.classList.toggle("ma-tela--sx", tl.scrollLeft>2); tl.classList.toggle("ma-tela--dx", tl.scrollLeft+tl.clientWidth < tl.scrollWidth-2); };
  f(); tl.addEventListener("scroll", f, {passive:true});
}
window.addEventListener("resize", ()=>{ if(S.tab==="rfq") segnaBordiTela(); });

/* ---- ricerca e filtro della gestione: si ridisegna solo l'elenco ---- */
document.addEventListener("input", e=>{
  if(e.target.id==="gq"){ S.q=e.target.value; $("gstati").innerHTML=gstati(); $("gbody").innerHTML=gbody(); }
});
document.addEventListener("change", e=>{
  if(e.target.id==="gcli"){ S.fcli=e.target.value; $("gstati").innerHTML=gstati(); $("gbody").innerHTML=gbody(); }
});

/* i menu «⋯» e «Azioni del thread» (details): un clic fuori li chiude, come in motore_a.mjs */
const MENU_DETAILS = "details.piu[open],details[data-ma-menu][open]";
document.addEventListener("click", e=>{ for(const m of document.querySelectorAll(MENU_DETAILS)) if(!m.contains(e.target)) m.open = false; }, true);
document.addEventListener("click", e=>{
  /* fuori dalle tendine: si chiudono */
  if(S.tab==="rfq" && S.menu && !e.target.closest(".tendina,[data-menu]")){ S.menu=null; render(); }
  const t = e.target.closest("[data-tab],[data-sez],[data-cli],[data-req],[data-pf],[data-orf],[data-altra],[data-tz],[data-crearfq],[data-apririfq],[data-nuovareq],[data-creareq],[data-aggancia],[data-nonric],[data-gfase],[data-setsez],[data-setcli],[data-nuovo],[data-attivo],[data-mitt],[data-togli],[data-addmitt],[data-salvacli],[data-autorizza],[data-censisci],[data-ignora],[data-staz],[data-cassetto],[data-cassetto-apri],[data-castab],[data-menu],[data-prodotto],[data-ruolo],[data-vista]");
  if(!t) return;
  const D = t.dataset;
  if(t.tagName==="A") e.preventDefault();

  /* il guscio della Distinta */
  if(D.staz!==undefined && !D.tab){ if(D.cicloCid){ S.cicloCid=D.cicloCid; S.cicloRif=null; } vaiStazione(+D.staz); return; }
  if(D.cassettoApri){ S.cassetto = D.cassettoApri; render(); return; }
  if(D.cassetto!==undefined){ const stesso = S.cassetto===D.cassetto && (!D.castab || S.cassettoTab===D.castab); S.cassetto = D.cassetto && !stesso ? D.cassetto : null; if(D.castab) S.cassettoTab = D.castab; S.menu = null; render(); return; }
  if(D.castab){ S.cassettoTab=D.castab; render(); return; }
  if(D.menu!==undefined){ S.menu = D.menu && S.menu!==D.menu ? D.menu : null; render(); return; }
  if(D.prodotto){ S.prodotto=D.prodotto; S.casella=null; S.scheda=null; S.menu=null; S.fonteAperta=null; render(); return; }
  if(D.ruolo){ S.ruolo=D.ruolo; S.menu=null; if(D.ruolo==="Franco") S.esitoVista="foglio"; else if(S.esitoVista && S.esitoVista!=="esito" && D.ruolo==="Lino") S.esitoVista="esito"; render(); return; }
  if(D.vista){ e.preventDefault(); S.vista=D.vista; S.dialogo=null; render(); if(typeof ANCORA_VISTA!=="undefined" && ANCORA_VISTA[D.vista]) scorriAncora(ANCORA_VISTA[D.vista]); return; }

  /* navigazione */
  if(D.tab){ S.tab=D.tab; S.menu=null; if(D.sez){S.sez=D.sez;} if(D.setsez){S.setSez=D.setsez;S.nuovo=false;} if(D.setcli){S.setCli=D.setcli;S.setSez="clienti";S.nuovo=false;}
    S.setOk=null; if(D.tab!=="inbox")S.ok=null; return render(); }
  if(D.setsez!==undefined&&!D.tab){S.tab="set";S.setSez=D.setsez;S.nuovo=false;S.setOk=null;return render();}
  if(D.setcli&&!D.tab){S.tab="set";S.setSez="clienti";S.setCli=D.setcli;S.nuovo=false;S.setOk=null;return render();}
  if(D.sez){S.tab="inbox";S.sez=D.sez;S.ok=null;return render();}
  if(D.cli){S.tab="inbox";S.sez="clienti";S.cli=D.cli;S.req=(reqDi(D.cli)[0]||{}).id;S.pf=null;S.ok=null;return render();}
  if(D.req){const r=rq(D.req);S.tab="inbox";S.sez="clienti";S.cli=r.cli;S.req=r.id;S.pf=D.pfset||null;S.menu=null;if(S.ok&&S.ok.req!==r.id)S.ok=null;return render();}
  if(D.pf!==undefined){S.pf=D.pf||null;return render();}
  if(D.orf){S.orf=D.orf;S.ok=null;return render();}
  if(D.altra){S.altra=D.altra;return render();}
  if(D.tz){const[f,i]=D.tz.split(":");S.tab="inbox";S.sez="terzisti";S.tz=f;S.tzc=+i||0;return render();}
  if(D.gfase){S.gfase=D.gfase;$("gstati").innerHTML=gstati();$("gbody").innerHTML=gbody();return;}

  /* RFQ: crea se non esiste, riapre se esiste; aprire non scrive niente */
  if(D.crearfq){creaRFQ(D.crearfq);return render();}
  if(D.apririfq){apriRFQ(D.apririfq);return render();}

  /* smistamento dell'Inbox */
  if(D.nuovareq){S.tab="nuovareq";S.nuovaFrom=D.nuovareq;return render();}
  if(D.creareq){creaRichiesta();return render();}
  if(D.aggancia){aggancia(D.aggancia);return render();}
  if(D.nonric){const i=SMISTARE.findIndex(x=>x.id===D.nonric),m=SMISTARE[i];
    ALTRA.unshift({id:"a-"+m.id,chi:m.chi,em:m.em,az:cl(m.cli)?.nome||"",cli:m.cli,cat:"Commerciale (non RFQ)",t:m.t,sub:m.sub,snip:"spostato dall’operatore",txt:m.txt,files:m.files,azioni:["Archivia nel cliente"]});
    SMISTARE.splice(i,1);S.orf=(SMISTARE[0]||{}).id;S.ok={txt:`«${m.sub}» spostato in «Senza prodotto».`};return render();}

  /* clienti e mittenti */
  if(D.nuovo!==undefined){S.tab="set";S.setSez="clienti";S.nuovo=D.nuovo==="1";S.censDa=null;S.setOk=null;return render();}
  /* RL-06: «Censisci come cliente» da un messaggio nascosto senza cliente (il gesto c'è già nel backend: censisci.go) */
  if(D.censisci){S.tab="set";S.setSez="clienti";S.nuovo=true;S.censDa=D.censisci;S.setOk=null;return render();}
  if(D.attivo){const c=cl(S.setCli);c.attivo=!c.attivo;S.setOk=c.attivo?`${esc(c.nome)} è di nuovo attivo: le sue mail entrano nell’Inbox.`:`${esc(c.nome)} è sospeso: le sue mail vanno fra i messaggi nascosti.`;return render();}
  if(D.mitt!==undefined){const c=cl(S.setCli),x=c.mittenti[+D.mitt];x.on=!x.on;S.setOk=`${esc(x.e)} ${x.on?"autorizzato":"sospeso"}.`;return render();}
  if(D.togli!==undefined){const c=cl(S.setCli),x=c.mittenti.splice(+D.togli,1)[0];S.setOk=`${esc(x.e)} rimosso dai mittenti autorizzati.`;return render();}
  if(D.addmitt){const n=$("nm-n").value.trim(),r=$("nm-r").value.trim()||"buyer",em=$("nm-e").value.trim().toLowerCase();
    if(!n||!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(em)){S.setOk=null;$("nm-e").focus();$("nm-e").style.borderColor="var(--ko)";return;}
    if(S.nuovo){S.setOk="Salva prima il cliente, poi aggiungi i mittenti.";return render();}
    cl(S.setCli).mittenti.push({n,r,e:em,on:true,ultimo:"—"});S.setOk=`${esc(em)} aggiunto ai mittenti autorizzati.`;return render();}
  if(D.salvacli){
    if(S.nuovo){const nome=$("fc-nome").value.trim();if(!nome){$("fc-nome").focus();$("fc-nome").style.borderColor="var(--ko)";return;}
      const id="c"+(CLIENTI.length+1), dom=$("fc-dom").value.trim().toLowerCase();
      CLIENTI.push({id,nome,sigla:ini(nome)||"NC",col:"var(--av6)",sett:$("fc-sett").value||"altri",dominio:dom,sede:$("fc-sede").value.trim(),stab:0,
        nas:($("fc-nas").value.trim()||nome).toUpperCase(),portale:"—",attivo:true,mittenti:[]});
      /* RL-06: al salvataggio i messaggi nascosti non decisi dello stesso dominio prendono il cliente e mostrano «Autorizza» */
      const stessi=dom?NASCOSTI.filter(x=>!x.cli&&String(x.em).toLowerCase().endsWith("@"+dom)):[];
      stessi.forEach(x=>{x.cli=id;x.motivo="dominio del cliente, mittente non ancora autorizzato";});
      const daCens=S.censDa; S.nuovo=false;S.censDa=null;S.setCli=id;
      if(daCens&&stessi.length){S.setSez="nascosti";S.setOk=`${esc(nome)} creato. ${plur(stessi.length,"messaggio nascosto","messaggi nascosti")} di ${esc(dom)} ora ${stessi.length===1?"è":"sono"} del cliente: «Autorizza» porta la mail in «Da smistare».`;}
      else S.setOk=`${esc(nome)} creato. Ora aggiungi i mittenti autorizzati.`;
      return render();}
    S.setOk="Modifiche salvate.";return render();}
  if(D.autorizza){const i=NASCOSTI.findIndex(x=>x.id===D.autorizza),m=NASCOSTI[i],c=cl(m.cli);
    const giaC=c.mittenti.find(x=>x.e===m.em); if(giaC)giaC.on=true; else c.mittenti.push({n:m.chi,r:"buyer",e:m.em,on:true,ultimo:"oggi"});
    NASCOSTI.splice(i,1);
    if(m.porta){SMISTARE.unshift(Object.assign({id:"s-"+m.id,chi:m.chi,em:m.em,cli:m.cli,t:m.t,sub:m.sub,snip:"mittente appena autorizzato",
      stato:C("prop","nuova richiesta?"),cands:[{t:"Nuova richiesta · "+c.nome,st:C("prop","proposto"),why:"Mittente appena autorizzato. Il codice non compare in nessuna richiesta aperta.",src:[C("neu","mittente autorizzato"),C("acc","1 candidato")]}]},m.porta));
      S.setOk=`${esc(m.chi)} autorizzata per ${esc(c.nome)}. La sua richiesta «${esc(m.sub)}» è ora in «Da smistare».`;}
    else S.setOk=`${esc(m.chi)} autorizzato per ${esc(c.nome)}.`;
    return render();}
  if(D.ignora){const i=NASCOSTI.findIndex(x=>x.id===D.ignora);const m=NASCOSTI.splice(i,1)[0];S.setOk=`«${esc(m.sub)}» ignorato.`;return render();}
});

/* ---- smistamento: aggancio e nuova richiesta ---- */
function aggancia(id){
  const i=SMISTARE.findIndex(x=>x.id===id), m=SMISTARE[i], r=rq(m.dest);
  MESSAGGI.push({mid:"x"+m.id,req:r.id,prod:r.prodotti.slice(),d:"oggi",t:m.t.replace(/^\D+/,""),dir:"in",chi:m.chi,r:cl(m.cli).mittenti.find(x=>x.e===m.em)?.r||"",
    sub:m.sub,txt:m.txt,files:m.files,meta:"agganciato dall’operatore da «Da smistare»"});
  SMISTARE.splice(i,1); S.orf=(SMISTARE[0]||{}).id;
  S.ok={txt:`Messaggio agganciato a «${r.titolo}». I suoi allegati entrano nel thread: nella Distinta li trovi in «Da smistare» dopo «Rifai lo smistamento».`,req:r.id};
}
function creaRichiesta(){
  const i=SMISTARE.findIndex(x=>x.id===S.nuovaFrom), m=SMISTARE[i], c=cl(m.cli), rid="r-n"+(RICHIESTE.length+1), pids=[];
  (m.codici||[]).forEach(([cod,nome,qta],j)=>{
    const cb=document.querySelector(`[data-cod="${j}"]`); if(cb&&!cb.checked) return;
    const nm=(document.querySelector(`[data-nome="${j}"]`)||{}).value||nome, qt=(document.querySelector(`[data-qta="${j}"]`)||{}).value||qta;
    const pid=rid+"p"+j; PROD[pid]={cod,nome:nm,qta:qt}; pids.push(pid);
  });
  if(!pids.length){S.ok={txt:"Spunta almeno un codice: una richiesta senza prodotti non nasce."};S.tab="inbox";S.sez="smistare";return;}
  const sc=($("nr-scad")||{}).value, scad=sc?sc.split("-").reverse().slice(0,2).join("/"):"da chiedere";
  const ora=m.t.replace(/^\D+/,"");
  RICHIESTE.push({id:rid,cli:c.id,titolo:(($("nr-tit")||{}).value||m.sub).trim(),aperta:"11/10/2026",ora,buyer:m.chi,prodotti:pids,rfq:null,scad,sla:sc?"ok":"nessuna"});   /* RL-28: senza scadenza non è «vicina»: è da chiedere */
  MESSAGGI.push({mid:"m"+rid,req:rid,prod:pids,d:"oggi",t:ora,dir:"in",chi:m.chi,r:"buyer",sub:m.sub,txt:m.txt,files:m.files,meta:"messaggio da cui è nata la richiesta"});
  SMISTARE.splice(i,1); S.orf=(SMISTARE[0]||{}).id;
  S.tab="inbox";S.sez="clienti";S.cli=c.id;S.req=rid;S.pf=null;
  S.ok={txt:"Richiesta creata. Quando sei pronto a lavorarla, crea l’RFQ dal riquadro qui sotto.",req:rid};
}

render();
</script>
