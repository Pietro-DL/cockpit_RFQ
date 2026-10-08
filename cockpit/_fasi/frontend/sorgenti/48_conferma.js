/* ================= CONFERMA FASCICOLO: il gesto cumulativo (R67 A, R108; E2 §3.3, spazio di verifica) =================
   Un riepilogo delle singole decisioni del prodotto, raggruppate come le chiede R108: fonte STEP, nomenclatura,
   relazioni e quantità, classificazioni, associazioni dei file. Si spuntano quelle verificate e si confermano
   con un solo gesto e un esito unico. Le alternative irrisolte non si scelgono mai in silenzio: restano fuori,
   con il motivo. Il gesto non congela il fascicolo e non avvia la fattibilità.
   Nel backend sarà un servizio dello spazio di verifica con controllo della versione esaminata (l'impronta). */
const CF={aperto:false,sel:{},impronta:""};
function decisioniPendenti(pid){
  const D=DOCS[pid], B=bomDi(pid), out=[], fuori=[];
  if(D.step&&!D.step.portale&&D.step.stato!=="ok") (D.step.stato==="verifica"?fuori:out).push({id:"fonte",g:"Fonte strutturale (STEP)",t:`Autorizza ${D.step.f} come fonte della distinta`,why:D.step.perche});
  if(modifiche(B)) fuori.push({g:"Struttura",t:`${plur(modifiche(B),"modifica non salvata","modifiche non salvate")} nella Distinta`,why:"salvale o annullale prima: il riepilogo vale sulla versione salvata"});
  else B.nodes.filter(n=>n.proposto).forEach(n=>{
    out.push({id:"nodo:"+n.id,g:"Nomenclatura",t:`Codice ${n.codice} · ${TIPI[n.tipo]}`,why:(n.fonti||[])[0]?.[2]||"proposto dall’analisi"});
    const p=nodo(B,n.padre); if(p) out.push({id:"arco:"+n.id,g:"Relazioni e quantità",t:`${n.codice} ×${n.qta} sotto ${p.codice}`,why:"dallo STEP strutturale",dipende:"nodo:"+n.id});
  });
  D.parti.forEach((p,i)=>{ if(p.cat==="minuteria"&&!p.catConf) out.push({id:"cat:"+i,g:"Classificazioni",t:`${p.cod} è minuteria`,why:p.catPerche||"categoria proposta dalla grammatica"}); });
  chiavi(pid).forEach(k=>{const s=getSlot(pid,k); if(!s||s.portale||s.stato==="ok"||k==="step") return;
    if(s.stato==="verifica") fuori.push({g:"Associazioni dei file",t:`${s.f} come ${etichetta(pid,k)}`,why:s.nota||"fonti discordanti: va risolto prima"});
    else out.push({id:"file:"+k,g:"Associazioni dei file",t:`${s.f} come ${etichetta(pid,k)}`,why:s.perche});});
  return {out,fuori};
}
const GRUPPI_CF=["Fonte strutturale (STEP)","Nomenclatura","Relazioni e quantità","Classificazioni","Associazioni dei file"];
function bottoneConferma(pid){
  const n=decisioniPendenti(pid).out.length;
  return `<button class="btn sm ${n?"pri":""}" data-cf="apri" ${n?"":"disabled"}>${I("check",13)} Conferma fascicolo${n?` · ${n} decisioni`:""}</button>`;
}
function riepilogoConferma(){
  if(!CF.aperto) return "";
  const pid=S.rp, {out,fuori}=decisioniPendenti(pid), scelte=out.filter(x=>CF.sel[x.id]!==false);
  return `<div class="backdrop" data-cf="chiudi"></div><aside class="drawer largo" role="dialog" aria-label="Conferma fascicolo">
    <header><div style="min-width:0"><span class="lab">Conferma fascicolo · ${esc(PROD[pid].cod)}</span><h3>Riepilogo delle decisioni</h3></div><span class="sp"></span>
      <button class="btn sm ghost" data-cf="chiudi" aria-label="Chiudi">${I("x",15)}</button></header>
    <div class="dbody">
      <p class="hint" style="margin:0">Spunta quello che hai verificato. Si conferma tutto insieme, con un esito unico; ogni decisione resta registrata da sola. Il gesto <b>non congela</b> il fascicolo e <b>non avvia</b> la fattibilità.</p>
      ${GRUPPI_CF.map(g=>{const v=out.filter(x=>x.g===g); return v.length?`<div class="cf-g"><span class="lab">${g} · ${v.length}</span>${v.map(x=>`<label class="cf-i"><input type="checkbox" data-cfsel="${esc(x.id)}" ${CF.sel[x.id]!==false?"checked":""}>
          <span style="min-width:0"><b>${esc(x.t)}</b><span class="k3">${esc(x.why||"")}</span></span></label>`).join("")}</div>`:"";}).join("")||`<p class="ctrlok">${I("check",14)} Nessuna decisione da confermare per questo prodotto.</p>`}
      ${fuori.length?`<div class="cf-g fuori"><span class="lab">${I("alert",13)} Non comprese · da risolvere prima</span>${fuori.map(x=>`<div class="cf-i"><span></span><span><b>${esc(x.t)}</b><span class="k3">${esc(x.why)}</span></span></div>`).join("")}</div>`:""}
      <p class="k3" style="margin:0;font-size:var(--t-xs)">Versione esaminata: <span class="mono">${esc(CF.impronta)}</span>. Se il fascicolo cambia prima della conferma, il gesto si ferma e il riepilogo va riaperto.</p></div>
    <footer><button class="btn pri" data-cf="conferma" ${scelte.length?"":"disabled"}>${I("check",14)} Conferma ${plur(scelte.length,"decisione","decisioni")}</button>
      <button class="btn ghost" data-cf="chiudi">Annulla</button></footer></aside>`;
}
function improntaFascicolo(pid){return impronta(JSON.stringify([DOCS[pid].step&&DOCS[pid].step.stato,bomDi(pid).nodes.map(n=>[n.id,n.proposto,n.padre,n.qta]),DOCS[pid].parti.map(p=>[p.cat,p.catConf,p.d2&&p.d2.stato])]));}
function confermaFascicolo(){
  const pid=S.rp;
  if(improntaFascicolo(pid)!==CF.impronta){S.esito={ok:false,righe:["<b>Il fascicolo è cambiato</b> dal riepilogo: riaprilo e ricontrolla."]};CF.aperto=false;return;}
  const {out}=decisioniPendenti(pid), sc=out.filter(x=>CF.sel[x.id]!==false&&(!x.dipende||CF.sel[x.dipende]!==false)), B=bomDi(pid), conta={};
  sc.forEach(x=>{conta[x.g]=(conta[x.g]||0)+1; const [tipo,...r]=x.id.split(":"), ref=r.join(":");
    if(tipo==="fonte") DOCS[pid].step.stato="ok";
    if(tipo==="nodo"){const n=nodo(B,ref); if(n) n.proposto=false;}
    if(tipo==="cat"){const p=DOCS[pid].parti[+ref]; p.catConf=true; const n=B.nodes.find(m=>m.pk===p.pk); if(n) n.catConf=true;}
    if(tipo==="file"){const s=getSlot(pid,ref); if(s) s.stato="ok";}});
  if(sc.some(x=>x.id.startsWith("nodo:"))){syncParti(pid,B); B.salvati=clona(B.nodes); B.verificata=!B.nodes.some(n=>n.proposto);}
  CF.aperto=false; CF.sel={};
  S.esito={ok:true,righe:[`<b>Fascicolo confermato: ${plur(sc.length,"decisione","decisioni")}</b> in un solo gesto.`,
    GRUPPI_CF.filter(g=>conta[g]).map(g=>`${g}: ${conta[g]}`).join(" · "),"Non è congelato e la fattibilità non è partita: si decidono a parte."]};
}
document.addEventListener("click",e=>{
  const t=e.target.closest("[data-cf]"); if(!t) return;
  const a=t.dataset.cf;
  if(a==="apri"){CF.aperto=true; CF.sel={}; CF.impronta=improntaFascicolo(S.rp);}
  if(a==="chiudi") CF.aperto=false;
  if(a==="conferma") confermaFascicolo();
  render();
});
document.addEventListener("change",e=>{ if(e.target.dataset&&e.target.dataset.cfsel){CF.sel[e.target.dataset.cfsel]=e.target.checked; render();} });
