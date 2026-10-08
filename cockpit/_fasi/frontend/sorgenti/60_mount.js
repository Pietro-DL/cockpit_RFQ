
/* ================= montaggio ================= */
/* ---- riadattamento automatico allo schermo ----
   La larghezza della finestra dà una classe (telefono … ultra) su <html>: ogni scheda si ridispone da sola
   e usa tutto lo spazio del monitor. Si ricalcola a ogni ridimensionamento, senza ridisegnare la pagina. */
const SCHERMI=[[640,"telefono"],[1040,"tablet"],[1440,"portatile"],[1920,"desktop"],[2560,"ampio"],[Infinity,"ultra"]];
function applicaSchermo(){
  const w=window.innerWidth||document.documentElement.clientWidth||1440, k=SCHERMI.find(([m])=>w<m)[1], h=document.documentElement;
  if(h.dataset.schermo===k) return false; h.dataset.schermo=k; return true;
}
applicaSchermo();
let tSchermo; window.addEventListener("resize",()=>{clearTimeout(tSchermo); tSchermo=setTimeout(applicaSchermo,100);});
let vistaPrima="";
function render(){
  /* cambiando pagina, passo o prodotto si riparte in cima e la Distinta torna allo stato iniziale;
     dentro la stessa vista si conserva lo scorrimento, così un gesto non fa perdere il punto */
  const vista=[S.tab,S.rfq,S.step,S.rp].join("|"), stessa=vista===vistaPrima;
  const sc=document.querySelector(".rfqbody"), top=stessa&&sc?sc.scrollTop:0;
  const tl=document.querySelector(".tela"), left=stessa&&tl?tl.scrollLeft:0;
  if(!stessa){DIST.sel="r";DIST.nuovo=null;DIST.elimina=false;DIST.scarta=false;DIST.msg=null;VIS.file=null;
    WF.cur=null;WF.vis={armato:false,nuovo:null,sel:null,zoom:0};WF.invio=null;WF.msg=null;WF.cat=null;CF.aperto=false;DEV.aperto=false;}
  vistaPrima=vista;
  renderTabs();
  const b=$("body");
  if(S.tab==="inbox"){
    b.style.gridTemplateRows="minmax(0,1fr)";
    b.innerHTML=`<div class="panes"><aside class="pane p-rail">${rail()}</aside><section class="pane p-list">${lista()}</section>
      <section class="pane p-conv">${conversazione()}</section><aside class="pane p-ctx">${dati()}</aside></div>`;
  } else if(S.tab==="nuovareq"){
    b.style.gridTemplateRows="auto minmax(0,1fr)"; b.innerHTML=nuovaRichiesta();
  } else if(S.tab==="gest"){
    b.style.gridTemplateRows="minmax(0,1fr)"; b.innerHTML=gestione();
  } else if(S.tab==="rfq"){
    b.style.gridTemplateRows="auto auto minmax(0,1fr) auto"; b.innerHTML=paginaRFQ();
    const sc2=document.querySelector(".rfqbody"); if(sc2&&top) sc2.scrollTop=top;
    const tl2=document.querySelector(".tela"); if(tl2&&left) tl2.scrollLeft=left;
  } else {
    const due=S.setSez!=="clienti";
    b.style.gridTemplateRows="minmax(0,1fr)";
    b.innerHTML=`<div class="set" style="${due?"grid-template-columns:238px minmax(0,1fr)":""}">
      <aside class="pane p-rail">${setRail()}</aside>${due?"":`<section class="pane p-list">${setLista()}</section>`}
      <section class="pane p-conv" style="background:var(--paper)">${due?setNascosti():formCliente()}</section></div>`;
  }
}

/* ---- azioni sui documenti dell'RFQ ---- */
function prodottiRFQ(){return datiRFQ(rf(S.rfq)).prodotti;}
function cercaDiNuovo(){
  const trovati=[];
  for(const p of prodottiRFQ()){
    const D=DOCS[p];
    D.trova=(D.trova||[]).filter(t=>{const s=getSlot(p,t.k);
      if(s&&!s.portale) return true;
      setSlot(p,t.k,t.slot); trovati.push(t.msg);
      if(t.togli) D.liberi=D.liberi.filter(l=>l[0]!==t.togli);
      return false;});
  }
  const st=stat(prodottiRFQ());
  S.esito = trovati.length
    ? {ok:true,righe:[`<b>${plur(trovati.length,"documento trovato","documenti trovati")}.</b>`,...trovati.map(esc),"Sono proposti: controllali e confermali."]}
    : {ok:false,righe:["<b>Nessun nuovo documento</b> nella mail né sul portale.",
        st.mancanti.length?"Ancora da sistemare: "+st.mancanti.slice(0,4).map(([p,k,w])=>`${esc(PROD[p].cod)} ${esc(etichetta(p,k))} (${w})`).join(" · ")+(st.mancanti.length>4?" …":"")
        :"Tutti gli obbligatori ci sono."]};
}
function caricaFile(target){
  let pid,k;
  if(target==="auto"){
    const m=stat(S.rp).mancanti.find(x=>x[2]==="manca"||x[2]==="sul portale")||stat(prodottiRFQ()).mancanti.find(x=>x[2]==="manca"||x[2]==="sul portale");
    if(!m){S.esito={ok:true,righe:["<b>Niente da caricare:</b> tutti i documenti obbligatori ci sono."]};render();return;}
    [pid,k]=m;
  } else [pid,k]=target.split("|");
  S.upl={pid,k};
  const inp=$("upl"); inp.value=""; inp.accept=kindOf(k)==="step"?".stp,.step,.igs,.iges":".pdf,.dxf,.dwg,.tif";
  inp.click();
}
$("upl").addEventListener("change",e=>{
  const f=e.target.files&&e.target.files[0]; if(!f||!S.upl) return;
  const kb=f.size>1048576?(f.size/1048576).toFixed(1).replace(".",",")+" MB":Math.max(1,Math.round(f.size/1024))+" KB";
  setSlot(S.upl.pid,S.upl.k,SL(f.name,kb,"car","caricato a mano da Franco · adesso","scelto dall’operatore","ok"));
  S.esito={ok:true,righe:[`<b>Caricato ${esc(f.name)}</b> come ${esc(etichetta(S.upl.pid,S.upl.k))} di ${esc(PROD[S.upl.pid].cod)}.`]};
  S.upl=null; S.insp=null; render();
});

/* ---- ricerca e filtro della gestione: si ridisegna solo l'elenco, il campo resta attivo ---- */
document.addEventListener("input",e=>{
  if(e.target.id==="gq"){S.q=e.target.value;$("gstati").innerHTML=gstati();$("gbody").innerHTML=gbody();}
});
document.addEventListener("change",e=>{
  if(e.target.id==="gcli"){S.fcli=e.target.value;$("gstati").innerHTML=gstati();$("gbody").innerHTML=gbody();}
});

document.addEventListener("click",e=>{
  const t=e.target.closest("[data-tab],[data-sez],[data-cli],[data-req],[data-pf],[data-orf],[data-altra],[data-tz],[data-crearfq],[data-apririfq],[data-nuovareq],[data-creareq],[data-aggancia],[data-nonric],[data-stato],[data-step],[data-rp],[data-insp],[data-conf],[data-confall],[data-rim],[data-usa],[data-cerca],[data-carica],[data-libass],[data-libvia],[data-assoc],[data-nas],[data-close],[data-setsez],[data-setcli],[data-nuovo],[data-attivo],[data-mitt],[data-togli],[data-addmitt],[data-salvacli],[data-autorizza],[data-ignora],[data-mincf]");
  if(!t) return;
  const D=t.dataset;

  /* navigazione */
  if(D.tab){ S.tab=D.tab; if(D.sez){S.sez=D.sez;} if(D.setsez){S.setSez=D.setsez;S.nuovo=false;} if(D.setcli){S.setCli=D.setcli;S.setSez="clienti";S.nuovo=false;}
    S.setOk=null; if(D.tab!=="inbox")S.ok=null; return render(); }
  if(D.setsez!==undefined&&!D.tab){S.tab="set";S.setSez=D.setsez;S.nuovo=false;S.setOk=null;return render();}
  if(D.setcli&&!D.tab){S.tab="set";S.setSez="clienti";S.setCli=D.setcli;S.nuovo=false;S.setOk=null;return render();}
  if(D.sez){S.tab="inbox";S.sez=D.sez;S.ok=null;return render();}
  if(D.cli){S.tab="inbox";S.sez="clienti";S.cli=D.cli;S.req=(reqDi(D.cli)[0]||{}).id;S.pf=null;S.ok=null;return render();}
  if(D.req){const r=rq(D.req);S.tab="inbox";S.sez="clienti";S.cli=r.cli;S.req=r.id;S.pf=D.pfset||null;if(S.ok&&S.ok.req!==r.id)S.ok=null;return render();}
  if(D.pf!==undefined){S.pf=D.pf||null;return render();}
  if(D.orf){S.orf=D.orf;S.ok=null;return render();}
  if(D.altra){S.altra=D.altra;return render();}
  if(D.tz){const[f,i]=D.tz.split(":");S.tab="inbox";S.sez="terzisti";S.tz=f;S.tzc=+i||0;return render();}

  /* RFQ: crea se non esiste, riapre se esiste */
  if(D.crearfq){creaRFQ(D.crearfq);return render();}
  if(D.apririfq){apriRFQ(D.apririfq);return render();}
  if(D.stato){S.stato=D.stato;return render();}
  if(D.step!==undefined){S.step=+D.step;S.insp=null;return render();}
  if(D.rp){S.rp=D.rp;S.esito=null;return render();}

  /* smistamento */
  if(D.nuovareq){S.tab="nuovareq";S.nuovaFrom=D.nuovareq;return render();}
  if(D.creareq){creaRichiesta();return render();}
  if(D.aggancia){aggancia(D.aggancia);return render();}
  if(D.nonric){const i=SMISTARE.findIndex(x=>x.id===D.nonric),m=SMISTARE[i];
    ALTRA.unshift({id:"a-"+m.id,chi:m.chi,em:m.em,az:cl(m.cli)?.nome||"",cli:m.cli,cat:"Commerciale (non RFQ)",t:m.t,sub:m.sub,snip:"spostato dall’operatore",txt:m.txt,files:m.files,azioni:["Archivia nel cliente"]});
    SMISTARE.splice(i,1);S.orf=(SMISTARE[0]||{}).id;S.ok={txt:`«${m.sub}» spostato in «Senza prodotto».`};return render();}

  /* documenti */
  if(D.insp){const[p,k]=D.insp.split("|");S.insp={pid:p,k,mode:"file"};return render();}
  if(D.close){S.insp=null;return render();}
  if(D.conf){const[p,k]=D.conf.split("|");const s=getSlot(p,k);s.stato="ok";delete s.nota;S.esito={ok:true,righe:[`<b>Confermato</b> ${esc(s.f)} come ${esc(etichetta(p,k))} di ${esc(PROD[p].cod)}.`]};S.insp=null;return render();}
  if(D.confall){let n=0,v=0;for(const p of prodottiRFQ())for(const k of chiavi(p)){const s=getSlot(p,k);if(s&&!s.portale&&s.stato==="prop"){s.stato="ok";n++;}else if(s&&s.stato==="verifica")v++;}
    S.esito={ok:true,righe:[`<b>${plur(n,"documento confermato","documenti confermati")}.</b>`,v?`${plur(v,"documento resta","documenti restano")} da verificare: aprili con «Ispeziona».`:""].filter(Boolean)};return render();}
  if(D.rim){const[p,k]=D.rim.split("|");const s=getSlot(p,k);DOCS[p].liberi.push([s.f,"tolto da "+etichetta(p,k)+" dall’operatore",""]);setSlot(p,k,null);
    S.insp=null;S.esito={ok:false,righe:[`<b>Rimosso</b> ${esc(s.f)}: ora è fra i file non associati.`]};return render();}
  if(D.usa!==undefined){const{pid,k}=S.insp,s=getSlot(pid,k);
    const altri=[...(s.alt||[]),...DOCS[pid].liberi.filter(l=>/\.(pdf|stp|step|igs|dxf)$/i.test(l[0])).map(l=>[l[0],"",l[1]])];
    const [f,sz,desc]=altri[+D.usa];
    DOCS[pid].liberi=DOCS[pid].liberi.filter(l=>l[0]!==f);
    const nuovo=SL(f,sz||s.s,s.prov,s.da,"scelto dall’operatore: "+desc,"ok",{alt:[[s.f,s.s,"file usato prima"]]});
    setSlot(pid,k,nuovo);S.insp=null;S.esito={ok:true,righe:[`<b>Cambiato:</b> ${esc(etichetta(pid,k))} ora è ${esc(f)}.`]};return render();}
  if(D.cerca){cercaDiNuovo();S.insp=null;return render();}
  if(D.carica){caricaFile(D.carica);return;}
  if(D.libass!==undefined){S.insp={mode:"associa",lib:+D.libass};return render();}
  if(D.libvia!==undefined){const L=DOCS[S.rp].liberi.splice(+D.libvia,1)[0];S.esito={ok:true,righe:[`<b>Messo da parte</b> ${esc(L[0])}.`]};return render();}
  if(D.assoc){const[p,k]=D.assoc.split("|");const L=DOCS[S.rp].liberi.splice(S.insp.lib,1)[0];const prima=getSlot(p,k);
    if(prima&&!prima.portale)DOCS[p].liberi.push([prima.f,"sostituito in "+etichetta(p,k),""]);
    setSlot(p,k,SL(L[0],"—","mail","file della richiesta","associato dall’operatore","ok"));
    S.insp=null;S.esito={ok:true,righe:[`<b>Associato</b> ${esc(L[0])} a ${esc(PROD[p].cod)} · ${esc(etichetta(p,k))}.`]};return render();}
  if(D.mincf){const[p,i]=D.mincf.split("|"),x=DOCS[p].parti[+i]; x.catConf=true; const n=bomDi(p).nodes.find(m=>m.pk===x.pk); if(n){n.catConf=true; const sv=bomDi(p).salvati.find(m=>m.id===n.id); if(sv) sv.catConf=true;}
    S.esito={ok:true,righe:[`<b>${esc(x.cod)} confermato come minuteria:</b> il 2D non è più richiesto (R103 C). I documenti ricevuti restano associati.`]};return render();}
  if(D.nas){const q=rf(S.rfq);q.nasOk=true;q.agg="adesso";S.esito={ok:true,righe:["<b>Copiati sul NAS</b> tutti i documenti confermati, con verifica dell’impronta."]};return render();}

  /* clienti e mittenti */
  if(D.nuovo!==undefined){S.tab="set";S.setSez="clienti";S.nuovo=D.nuovo==="1";S.setOk=null;return render();}
  if(D.attivo){const c=cl(S.setCli);c.attivo=!c.attivo;S.setOk=c.attivo?`${esc(c.nome)} è di nuovo attivo: le sue mail entrano nell’Inbox.`:`${esc(c.nome)} è sospeso: le sue mail vanno fra i messaggi nascosti.`;return render();}
  if(D.mitt!==undefined){const c=cl(S.setCli),x=c.mittenti[+D.mitt];x.on=!x.on;S.setOk=`${esc(x.e)} ${x.on?"autorizzato":"sospeso"}.`;return render();}
  if(D.togli!==undefined){const c=cl(S.setCli),x=c.mittenti.splice(+D.togli,1)[0];S.setOk=`${esc(x.e)} rimosso dai mittenti autorizzati.`;return render();}
  if(D.addmitt){const n=$("nm-n").value.trim(),r=$("nm-r").value.trim()||"buyer",em=$("nm-e").value.trim().toLowerCase();
    if(!n||!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(em)){S.setOk=null;$("nm-e").focus();$("nm-e").style.borderColor="var(--bad)";return;}
    if(S.nuovo){S.setOk="Salva prima il cliente, poi aggiungi i mittenti.";return render();}
    cl(S.setCli).mittenti.push({n,r,e:em,on:true,ultimo:"—"});S.setOk=`${esc(em)} aggiunto ai mittenti autorizzati.`;return render();}
  if(D.salvacli){
    if(S.nuovo){const nome=$("fc-nome").value.trim();if(!nome){$("fc-nome").focus();$("fc-nome").style.borderColor="var(--bad)";return;}
      const id="c"+(CLIENTI.length+1);
      CLIENTI.push({id,nome,sigla:ini(nome)||"NC",col:"#4f5a63",sett:$("fc-sett").value,dominio:$("fc-dom").value.trim(),sede:$("fc-sede").value.trim(),stab:0,
        nas:($("fc-nas").value.trim()||nome).toUpperCase(),portale:"—",attivo:true,mittenti:[]});
      S.nuovo=false;S.setCli=id;S.setOk=`${esc(nome)} creato. Ora aggiungi i mittenti autorizzati.`;return render();}
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
  if(m.files) for(const f of m.files) DOCS[r.prodotti[0]].liberi.push([f.n,"arrivato dopo la richiesta, mail di "+m.chi,"della richiesta"]);
  SMISTARE.splice(i,1); S.orf=(SMISTARE[0]||{}).id;
  S.ok={txt:`Messaggio agganciato a «${r.titolo}».`,req:r.id};
}
function creaRichiesta(){
  const i=SMISTARE.findIndex(x=>x.id===S.nuovaFrom), m=SMISTARE[i], c=cl(m.cli), rid="r-n"+(RICHIESTE.length+1), pids=[];
  (m.codici||[]).forEach(([cod,nome,qta],j)=>{
    const cb=document.querySelector(`[data-cod="${j}"]`); if(cb&&!cb.checked) return;
    const nm=(document.querySelector(`[data-nome="${j}"]`)||{}).value||nome, qt=(document.querySelector(`[data-qta="${j}"]`)||{}).value||qta;
    const pid=rid+"p"+j; PROD[pid]={cod,nome:nm,qta:qt}; pids.push(pid);
    const pdf=(m.files||[]).find(f=>f.e==="pdf"&&f.n.startsWith(cod)), stp=(m.files||[]).find(f=>/^(stp|step)$/.test(f.e)&&f.n.startsWith(cod));
    DOCS[pid]={step:stp?SL(stp.n,stp.s,"mail",`mail di oggi · ${m.chi}`,"nome = codice richiesto","prop"):null,
      pdf:pdf?SL(pdf.n,pdf.s,"mail",`mail di oggi · ${m.chi}`,"nome = codice richiesto · cartiglio da leggere","prop"):null,parti:[],liberi:[],trova:[]};
  });
  if(!pids.length){S.ok={txt:"Spunta almeno un codice: una richiesta senza prodotti non nasce."};S.tab="inbox";S.sez="smistare";return;}
  const sc=($("nr-scad")||{}).value, scad=sc?sc.split("-").reverse().slice(0,2).join("/"):"da chiedere";
  const ora=m.t.replace(/^\D+/,"");
  RICHIESTE.push({id:rid,cli:c.id,titolo:(($("nr-tit")||{}).value||m.sub).trim(),aperta:"06/10/2026",ora,buyer:m.chi,prodotti:pids,rfq:null,scad,sla:sc?"ok":"warn"});
  MESSAGGI.push({mid:"m"+rid,req:rid,prod:pids,d:"oggi",t:ora,dir:"in",chi:m.chi,r:"buyer",sub:m.sub,txt:m.txt,files:m.files,meta:"messaggio da cui è nata la richiesta"});
  SMISTARE.splice(i,1); S.orf=(SMISTARE[0]||{}).id;
  S.tab="inbox";S.sez="clienti";S.cli=c.id;S.req=rid;S.pf=null;
  S.ok={txt:"Richiesta creata. Quando sei pronto a lavorarla, crea l’RFQ dal riquadro qui sotto.",req:rid};
}

render();
</script>
