/* ================= 15 · il finto ponte della zona A =================
   Copia del ponte vero di Pietro (web/static/motore_a_gesti.mjs): stessa API, stessa LISTA CHIUSA, stessi esiti.
     esegui(url, campi)           → {esito:"ok"|"no"|"errore", testo}; dopo "ok" rilegge la pagina
     leggi(url)                   → la GET JSON di sola lettura (solo le LETTURE) → l'oggetto; Error con il testo
     confermaAlbero(firma, bozza) → «Conferma l'albero» (fuori da ROTTE, come nel vero) → {esito, testo}
     aggiorna()                   → rilegge la pagina (qui: ridisegna da capo dalla fixture, come htmx su #ma)
   Differenze dichiarate (solo perché l'artifact non ha un server):
   - la RFQ della pagina non si legge dal percorso /thread/{id}/motore-a: è il thread aperto nel mockup (PONTE.thread);
   - al posto di fetch c'è il finto server A (17_server_a.js), che applica le regole vere, comprese quelle che dicono «no»;
   - window.confirm può essere bloccato nella sandbox: la domanda di data-ma-domanda si fa con la STESSA FRASE in una
     barra in pagina (#ma-domanda), sotto il pulsante (SPEC K8);
   - il ponte lavora in modo sincrono (eseguiSubito) e le funzioni dell'API restituiscono comunque una Promise.
   Le scritture della zona A partono SOLO da qui: un clic su [data-ma-gesto] o il submit di un form [data-ma-gesto]. */

const FATTO_A = "Fatto: il server non ha aggiunto altro.";
const NIENTE_A = "Niente è cambiato.";

// ROTTE:inizio — copiate alla lettera da motore_a_gesti.mjs
const ROTTE = [
  "/thread/{id}/fascicolo/componente/{u}/step-strutturale",
  "/thread/{id}/fascicolo/componente/{u}/step-strutturale/revoca",
  "/thread/{id}/fascicolo/nodo/{u}/accetta",
  "/thread/{id}/fascicolo/nodo/{u}/scarta",
  "/thread/{id}/fascicolo/nodo/{u}/codice",
  "/thread/{id}/fascicolo/nodo/{u}/riapri",
  "/thread/{id}/fascicolo/componente/{u}/codice",
  "/thread/{id}/fascicolo/componente/{u}/modifica",
  "/thread/{id}/fascicolo/componente/{u}/tipo",
  "/thread/{id}/fascicolo/componente/{u}/collega",
  "/thread/{id}/fascicolo/componente/{u}/sposta",
  "/thread/{id}/fascicolo/componente/{u}/scollega",
  "/thread/{id}/fascicolo/componente/{u}/archivia",
  "/thread/{id}/fascicolo/componente/{u}/rimuovi",
  "/thread/{id}/fascicolo/componente/{u}/ripristina",
  "/thread/{id}/fascicolo/relazione/accetta",
  "/thread/{id}/fascicolo/relazione/scarta",
  "/thread/{id}/fascicolo/rimozione/accetta",
  "/thread/{id}/fascicolo/rimozione/scarta",
  "/thread/{id}/fascicolo/conferma",
  "/proposta/{u}/scarta",
  "/thread/{id}/fascicolo/assegna",
  "/thread/{id}/fascicolo/documento/{u}/sostituisci",
  "/thread/{id}/fascicolo/componente/{u}/deroga",
  "/thread/{id}/fascicolo/deroga/{u}/revoca",
  "/thread/{id}/riprova-copie",
  "/thread/{id}/fascicolo/prepara",
  "/thread/{id}/fascicolo/rianalizza",
  "/thread/{id}/smistamento/aggiorna",
];
// ROTTE:fine
// LETTURE:inizio
const LETTURE = [
  "/thread/{id}/motore-a/dati",
  "/thread/{id}/motore-a/anteprima/step-strutturale",
  "/thread/{id}/distinta/tipo",
  "/thread/{id}/distinta/albero/riepilogo",
];
// LETTURE:fine
const ALBERO = "/thread/{id}/distinta/albero/conferma";

const UUID_RE = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
function compilaRotta(rotta){
  let re="^";
  for(const pezzo of rotta.split(/(\{id\}|\{u\})/)){
    if(pezzo==="{id}") re+="("+UUID_RE+")";
    else if(pezzo==="{u}") re+=UUID_RE;
    else re+=pezzo.replace(/[.*+?^${}()|[\]\\]/g,"\\$&");
  }
  return new RegExp(re+"$");
}
const RE_ROTTE = ROTTE.map(compilaRotta), RE_LETTURE = LETTURE.map(compilaRotta);

/* il registro delle chiamate, zona A e zona B separate (per il cassetto ⓘ, scheda «Gesti») */
const REGISTRO = [];
function registra(zona, metodo, url, campi, esito){
  REGISTRO.unshift({zona, metodo, url, campi: campi||{}, esito: esito||null, quando: new Date().toLocaleTimeString("it-IT")});
  if(REGISTRO.length>80) REGISTRO.pop();
}

const PONTE = {
  thread: ()=> (typeof threadAperto==="function" ? threadAperto() : null),   // il thread della pagina (17_server_a)
  avviso: null,             // {esito, testo} → #ma-avviso
  inAttesa: null,           // la domanda aperta: {el, campi, url, testo}
};

function ammessaA(url, elenco){
  let u;
  try { u = new URL(String(url), "https://cockpit.invalid/"); } catch(e){ return {motivo:"indirizzo non valido"}; }
  if(u.origin!=="https://cockpit.invalid") return {motivo:"rotta di un'altra origine"};
  const T = PONTE.thread();
  if(!T) return {motivo:"il ponte vale solo nella pagina della Distinta"};
  for(const re of elenco){
    const m = re.exec(u.pathname);
    if(!m) continue;
    if(m[1]!==undefined && m[1].toLowerCase()!==T.id.toLowerCase()) return {motivo:"rotta di un'altra RFQ"};
    return {url:u};
  }
  return {motivo:"rotta non prevista dal contratto"};
}
function campiDa(campi){
  const p = new URLSearchParams();
  if(!campi) return p;
  if(campi instanceof URLSearchParams){ for(const [k,v] of campi.entries()) p.append(k,v); return p; }
  if(typeof FormData!=="undefined" && campi instanceof FormData){ for(const [k,v] of campi.entries()) if(typeof v==="string") p.append(k,v); return p; }
  for(const [k,v] of Object.entries(campi)){
    if(v===undefined||v===null) continue;
    if(Array.isArray(v)) v.forEach(x=>p.append(k,String(x))); else p.append(k,String(v));
  }
  return p;
}
function mostraAvviso(esito, testo){ PONTE.avviso = {esito, testo}; }

/* il cuore, sincrono: un gesto della lista chiusa → il finto server A → esito; con "ok" rilegge */
function eseguiSubito(url, campi){
  const a = ammessaA(url, RE_ROTTE);
  if(!a.url){ const r={esito:"errore",testo:"Il gesto non è partito: "+a.motivo+"."}; mostraAvviso(r.esito,r.testo); registra("A","POST",String(url),{},r); aggiornaVista(); return r; }
  if(a.url.search!==""){ const r={esito:"errore",testo:"Il gesto non è partito: le rotte dei gesti non hanno parametri nell'indirizzo."}; mostraAvviso(r.esito,r.testo); aggiornaVista(); return r; }
  const p = campiDa(campi);
  if(/\/fascicolo\/prepara$/.test(a.url.pathname) && p.has("auto")){
    const r={esito:"errore",testo:"Il gesto non è partito: “Prepara i file” è solo il pulsante (DB7-6)."}; mostraAvviso(r.esito,r.testo); aggiornaVista(); return r;
  }
  if(a.url.pathname.startsWith("/proposta/")) p.set("ritorna_thread", PONTE.thread().id);
  let r;
  try { r = SERVER_A.posta(PONTE.thread(), a.url.pathname, p); }
  catch(err){ r = {esito:"errore", testo:"Il gesto non è partito: "+(err&&err.message||err)}; }
  if(!r||!["ok","no","errore"].includes(r.esito)) r={esito:"errore",testo:"La risposta non dice l'esito: non si sa se il gesto è riuscito."};
  if(!r.testo) r.testo = r.esito==="ok"?FATTO_A:NIENTE_A;
  const obj={}; for(const [k,v] of p.entries()) obj[k]=k in obj?[].concat(obj[k],v):v;
  registra("A","POST",a.url.pathname,obj,r);
  mostraAvviso(r.esito, r.testo);
  if(r.esito==="ok"){ if(typeof dopoGestoOk==="function") dopoGestoOk(); aggiornaSubito(); } else aggiornaVista();
  return r;
}
function leggiSubito(url){
  const a = ammessaA(url, RE_LETTURE);
  if(!a.url) throw new Error("Lettura non partita: "+a.motivo+".");
  const q = Object.fromEntries(a.url.searchParams.entries());
  const r = SERVER_A.leggi(PONTE.thread(), a.url.pathname, q);
  registra("A","GET",a.url.pathname+a.url.search,{},{esito:"lettura"});
  return r;
}
function confermaAlberoSubito(firma, bozza){
  const T = PONTE.thread();
  if(!T){ const r={esito:"errore",testo:"La conferma non è partita: il ponte vale solo nella pagina della Distinta."}; mostraAvviso(r.esito,r.testo); return r; }
  let r;
  try { r = SERVER_A.confermaAlbero(T, {firma, bozza}); } catch(err){ r={esito:"errore",testo:"La conferma non è partita: "+(err&&err.message||err)}; }
  registra("A","POST",ALBERO.replace("{id}",T.id),{firma, bozza},r);
  mostraAvviso(r.esito, r.testo);
  if(r.esito==="ok") aggiornaSubito(); else aggiornaVista();
  return r;
}
/* rilettura: il finto server ricalcola (come una GET della pagina) e la vista si ridisegna tutta */
function aggiornaSubito(){ const T=PONTE.thread(); if(T) SERVER_A.rileggi(T); aggiornaVista(); return true; }
function aggiornaVista(){ if(typeof render==="function") render(); }

/* l'API del ponte, come nel vero (Promise) */
const Gesti = {
  esegui: (url,campi)=>Promise.resolve(eseguiSubito(url,campi)),
  leggi: (url)=>{ try { return Promise.resolve(leggiSubito(url)); } catch(e){ return Promise.reject(e); } },
  confermaAlbero: (firma,bozza)=>Promise.resolve(confermaAlberoSubito(firma,bozza)),
  aggiorna: ()=>Promise.resolve(aggiornaSubito()),
};

/* ---------------- l'intercettazione: clic e submit di [data-ma-gesto], con la domanda in pagina ---------------- */
function lanciaGesto(el, fd, url){
  if(!url || el.getAttribute("data-ma-in-corso")==="1") return;
  const domanda = el.getAttribute("data-ma-domanda");
  const p = new URLSearchParams();
  if(fd) for(const [k,v] of campiDa(fd).entries()) p.append(k,v);
  const fissi = el.getAttribute("data-ma-campi");
  if(fissi){
    try { for(const [k,v] of campiDa(JSON.parse(fissi)).entries()) p.append(k,v); }
    catch(e){ mostraAvviso("errore","I campi del gesto non si leggono (data-ma-campi): niente è partito."); aggiornaVista(); return; }
  }
  if(domanda){ chiediInPagina(el, domanda, ()=>eseguiSubito(url,p)); return; }
  eseguiSubito(url, p);
}
/* la barra della domanda: la stessa frase del contratto, [Sì] [No], subito sotto il pulsante (mai sopra il disegno) */
function chiediInPagina(el, testo, si){
  togliDomanda();
  PONTE.inAttesa = {si, testo};
  const bar = document.createElement("div");
  bar.id = "ma-domanda"; bar.className = "ma-domanda"; bar.setAttribute("role","alertdialog"); bar.setAttribute("aria-label","Domanda prima del gesto");
  bar.innerHTML = `<p class="ma-domanda-t"></p><span class="ma-domanda-b"><button type="button" class="ma-btn ma-btn--sm ma-btn--conferma" data-ma-si="1">Sì, procedi</button><button type="button" class="ma-btn ma-btn--sm" data-ma-no="1">No</button></span>`;
  bar.querySelector(".ma-domanda-t").textContent = testo;
  const dopo = el.closest(".ma-gesto, form") || el;
  dopo.insertAdjacentElement("afterend", bar);
  const b = bar.querySelector("[data-ma-si]"); if(b && b.focus) b.focus();
}
function togliDomanda(){ const v=document.getElementById("ma-domanda"); if(v) v.remove(); PONTE.inAttesa=null; }

document.addEventListener("click", e=>{
  const si = e.target.closest && e.target.closest("[data-ma-si]");
  if(si){ const x=PONTE.inAttesa; togliDomanda(); if(x) x.si(); return; }
  const no = e.target.closest && e.target.closest("[data-ma-no]");
  if(no){ togliDomanda(); return; }
  const el = e.target.closest ? e.target.closest("[data-ma-gesto]") : null;
  if(!el || el.tagName==="FORM") return;
  e.preventDefault();
  if(el.disabled || el.getAttribute("aria-disabled")==="true") return;
  lanciaGesto(el, null, el.getAttribute("data-ma-gesto"));
});
document.addEventListener("submit", e=>{
  const f = e.target;
  if(!f || f.tagName!=="FORM") return;
  if(!f.hasAttribute("data-ma-gesto")) return;
  e.preventDefault();
  if(typeof f.checkValidity==="function" && !f.checkValidity()){ if(f.reportValidity) f.reportValidity(); return; }
  let fd; try { fd = new FormData(f, e.submitter||undefined); } catch(err){ fd = new FormData(f); }
  lanciaGesto(f, fd, f.getAttribute("data-ma-gesto"));
});
