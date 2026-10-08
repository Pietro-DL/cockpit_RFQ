
/* ================= CLIENTI E MITTENTI ================= */
function setRail(){
  const nb=(id,ic,l,n)=>`<button class="navb" data-setsez="${id}" aria-current="${S.setSez===id}">${I(ic,17)}<span class="lbl">${l}</span><span class="cnt">${n}</span></button>`;
  return `<div class="railg"><span class="lab">Anagrafica</span></div>
  <div class="nav">${nb("clienti","users","Clienti e mittenti",CLIENTI.length)}${nb("nascosti","eyeoff","Messaggi nascosti",NASCOSTI.length)}</div>
  <div class="railg"><p class="k3" style="font-size:var(--t-xs);margin:8px 0 0;max-width:30ch">Nell’Inbox entrano solo le mail dei mittenti autorizzati dei clienti attivi. Il resto finisce fra i messaggi nascosti.</p></div>`;
}
function setLista(){
  return `<div class="lhead"><h2>Clienti</h2><div class="meta">${CLIENTI.filter(c=>c.attivo).length} attivi · ${CLIENTI.filter(c=>!c.attivo).length} sospesi</div></div>
  <div class="sub"><span class="lab">Ragione sociale</span><span style="flex:1"></span><button class="btn sm pri" data-nuovo="1">${I("plus",13)} Nuovo cliente</button></div>
  ${CLIENTI.map(c=>`<button class="row" data-setcli="${c.id}" aria-current="${S.setCli===c.id&&!S.nuovo}">
    <span class="av lg" style="background:${c.attivo?c.col:"#b9b9b2"}">${c.sigla}</span>
    <span><span class="r1"><span class="nm">${esc(c.nome)}</span><span class="tm">${reqDi(c.id).length} rich.</span></span>
    <span class="snip"><span class="sett" style="background:${SETT[c.sett][1]}"></span> ${SETT[c.sett][0]} · ${esc(c.sede)}</span>
    <span class="rchips">${C(c.attivo?"ok":"neu",c.attivo?"attivo":"sospeso",c.attivo?"check":"x")}${C("neu",plur(c.mittenti.filter(x=>x.on).length,"mittente","mittenti"),"users")}</span></span></button>`).join("")}`;
}
function formCliente(){
  const c=S.nuovo?null:cl(S.setCli);
  const d=c||{nome:"",sigla:"",col:"#4f5a63",sett:"agri",dominio:"",sede:"",stab:0,nas:"",portale:"",attivo:true,mittenti:[]};
  const att=d.mittenti.filter(x=>x.on).length;
  return `<div class="form">
    ${S.setOk?`<div class="esito ok">${I("check",16)}<div>${S.setOk}</div></div>`:""}
    <div class="fs"><header><span class="av lg" style="background:${d.col}">${d.sigla||"—"}</span>
      <div style="min-width:0"><h3>${c?esc(d.nome):"Nuovo cliente"}</h3><div class="k3" style="font-size:var(--t-sm)">${c?"Cartella sul NAS «"+esc(d.nas)+"»":"Entra nell’Inbox dopo il salvataggio"}</div></div>
      <span class="sp"></span>
      ${c?`<button class="sw" style="width:auto;border:0;background:none;padding:0" data-attivo="1" aria-pressed="${d.attivo}"><span class="tr"></span><span class="t" style="font-size:var(--t-ms)">${d.attivo?"Attivo":"Sospeso"}</span></button>`:""}</header>
    <div class="g2">
      <label class="fld"><span>Ragione sociale</span><input id="fc-nome" value="${esc(d.nome)}" placeholder="es. ACME Trattori"></label>
      <label class="fld"><span>Sede</span><input id="fc-sede" value="${esc(d.sede)}" placeholder="es. Borgo Nord (XX)"></label>
      <label class="fld"><span>Settore</span><select id="fc-sett">${Object.entries(SETT).map(([k,v])=>`<option value="${k}" ${k===d.sett?"selected":""}>${v[0]}</option>`).join("")}</select></label>
      <label class="fld"><span>Stabilimento di riferimento</span><select>${STAB.map((s,i)=>`<option ${i===d.stab?"selected":""}>${esc(s)}</option>`).join("")}</select></label>
      <label class="fld mono"><span>Cartella sul NAS</span><input id="fc-nas" value="${esc(d.nas)}" placeholder="ACME TRATTORI"><small>sotto «PREVENTIVI DA FARE»</small></label>
      <label class="fld mono"><span>Dominio di posta</span><input id="fc-dom" value="${esc(d.dominio)}" placeholder="acme-trattori.example"><small>riconosce il cliente, non autorizza da solo</small></label>
      <label class="fld mono"><span>Portale fornitori</span><input value="${esc(d.portale==="—"?"":d.portale)}" placeholder="supplier.cliente.com"></label></div></div>

    <div class="fs"><header>${I("shield",17)}<h3>Mittenti autorizzati</h3><span class="sp"></span>${C(att?"ok":"bad",att+" attivi su "+d.mittenti.length)}</header>
      <p class="hint">Nell’Inbox entrano <b>solo</b> le mail che arrivano da questi indirizzi. Un cliente può avere più referenti commerciali e una casella generica: aggiungine quanti servono.</p>
      <div style="display:grid;gap:8px">${d.mittenti.map((x,i)=>`<div class="send ${x.on?"":"off"}">
        <span class="av" style="background:${d.col}">${ini(x.n)}</span>
        <span style="min-width:0"><span class="nmr">${esc(x.n)}</span> ${C("ghost",x.r)}<div class="em k">${esc(x.e)}</div></span>
        <span class="k3" style="font-size:var(--t-xs)">ultimo<br>${esc(x.ultimo)}</span>
        <span style="display:flex;gap:4px;align-items:center">
          <button class="sw" style="width:auto;border:0;background:none;padding:0" data-mitt="${i}" aria-pressed="${x.on}" title="${x.on?"togli la conferma (buyer.confermato · R-04)":"conferma il mittente (buyer.confermato)"}"><span class="tr"></span></button>
          <button class="btn sm ghost danger" data-togli="${i}" title="Rimuovi">${I("x",14)}</button></span></div>`).join("")||`<p class="hint">Nessun mittente: aggiungine almeno uno.</p>`}</div>
      <div class="addsend">
        <label class="fld"><span>Nome e cognome</span><input id="nm-n" placeholder="es. Marta Sali"></label>
        <label class="fld"><span>Ruolo</span><input id="nm-r" placeholder="buyer, ufficio tecnico, casella"></label>
        <label class="fld mono"><span>Indirizzo e-mail</span><input id="nm-e" placeholder="nome@${esc(d.dominio||"cliente.com")}"></label>
        <button class="btn pri" data-addmitt="1">${I("plus",14)} Aggiungi</button></div>
      ${c&&NASCOSTI.some(x=>x.cli===c.id)?`<div class="go">${I("eyeoff",15)}<span><b>${plur(NASCOSTI.filter(x=>x.cli===c.id).length,"messaggio","messaggi")}</b> di questo cliente nascosti: mittenti non in elenco</span>
        <button class="btn sm" data-setsez="nascosti">Vedi</button></div>`:""}</div>

    <div class="acts" style="margin:0"><button class="btn pri xl" data-salvacli="1">${I("check",18)} ${c?"Salva le modifiche":"Crea il cliente"}</button>
      ${c?"":`<button class="btn" data-nuovo="0">Annulla</button>`}</div></div>`;
}
function setNascosti(){
  return `<div class="form">
    ${S.setOk?`<div class="esito ok">${I("check",16)}<div>${S.setOk}</div></div>`:""}
    <div class="fs"><header>${I("eyeoff",17)}<h3>Messaggi nascosti</h3><span class="sp"></span>${C("neu",NASCOSTI.length+" negli ultimi 7 giorni")}</header>
      <p class="hint">Non entrano nell’Inbox perché il mittente non è autorizzato. Restano qui per non perdere una richiesta arrivata da un indirizzo nuovo: autorizzando il mittente, la sua mail passa in «Da smistare».</p>
      ${NASCOSTI.length?`<div class="wrap-tb" style="overflow-x:auto"><table class="tb">
        <thead><tr><th>Mittente</th><th>Oggetto</th><th>Perché è nascosto</th><th>Arrivato</th><th></th></tr></thead>
        <tbody>${NASCOSTI.map(m=>`<tr>
          <td><b style="font-weight:500">${esc(m.chi)}</b><div class="mono k3" style="font-size:var(--t-xs)">${esc(m.em)}</div></td>
          <td>${esc(m.sub)}</td><td>${m.cli?C("warn",m.motivo,"alert"):C("neu",m.motivo)}</td>
          <td class="mono" style="font-size:var(--t-sm);white-space:nowrap">${esc(m.t)}</td>
          <td><div style="display:flex;gap:6px;justify-content:flex-end">
            ${m.cli?`<button class="btn sm pri" data-autorizza="${m.id}">${I("shield",13)} Autorizza</button>`:`<button class="btn sm" data-nuovo="1">${I("plus",13)} Nuovo cliente</button>`}
            <button class="btn sm ghost" data-ignora="${m.id}">Ignora</button></div></td></tr>`).join("")}</tbody></table></div>`
        :vuoto("check","Nessun messaggio nascosto","Tutti i mittenti che scrivono sono autorizzati.")}</div></div>`;
}
