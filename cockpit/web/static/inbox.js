// L'Inbox nuova nel browser: niente di tutto questo decide qualcosa. Scorre la lista, apre l'anteprima di un
// allegato sopra la pagina, evidenzia nel testo i codici letti dal Cockpit, fa la selezione multipla con i gesti
// di sempre (una richiesta per mail), e dopo una decisione passa da solo alla mail dopo. Le scorciatoie da
// tastiera valgono solo se accese sotto l'ingranaggio.
(function () {
  "use strict";
  const $ = s => document.querySelector(s);
  const UI = () => window.CockpitUI;
  const ui = () => $("#ui");

  /* ---------------------------------------------------------------- la lista */
  const righe = () => [...document.querySelectorAll("#lista .riga")];
  const aperta = () => { const m = $("#pannello .msg, #pannello .foglio"); return m ? m.dataset.id : ""; };
  function segnaAperta() {
    const id = aperta(), rr = righe();
    rr.forEach(r => r.classList.toggle("sel", r.dataset.id === id));
    const i = rr.findIndex(r => r.dataset.id === id), pos = $("#pos-lista");
    if (pos) pos.textContent = i >= 0 ? `${i + 1} di ${rr.length}` : "fuori dalla lista";
  }
  function vai(d) {
    const rr = righe(); if (!rr.length) return;
    const i = rr.findIndex(r => r.dataset.id === aperta());
    const j = i < 0 ? 0 : Math.max(0, Math.min(rr.length - 1, i + d));
    if (rr[j] && (i !== j || i < 0)) { rr[j].click(); rr[j].scrollIntoView({ block: "nearest" }); }
  }
  // dopo una decisione: la mail che era sotto quella decisa (o sopra, se era l'ultima)
  function vicina(id) {
    const rr = righe(), i = rr.findIndex(r => r.dataset.id === id);
    if (i < 0) return "";
    return (rr[i + 1] || rr[i - 1] || {}).dataset?.id || "";
  }

  /* ---------------------------------------------------------------- selezione multipla */
  const spunte = new Set();
  function bulk() {
    const on = $("#sel-multipla")?.checked, l = $("#lista");
    if (l) l.classList.toggle("bulk", !!on);
    document.querySelectorAll("[data-spunta]").forEach(c => { c.checked = spunte.has(c.dataset.spunta); });
    const b = $("#barra-bulk"); if (!b) return;
    b.hidden = !(on && spunte.size);
    if (!b.hidden) b.innerHTML = `<b>${spunte.size} selezionat${spunte.size === 1 ? "a" : "e"}</b><span class="sp"></span>
      <button type="button" class="btn piccolo" data-bulk="ignora">Ignora</button>
      <button type="button" class="btn piccolo" data-bulk="letti">Segna lette</button>
      <button type="button" class="btn piccolo" data-bulk="annulla">Deseleziona</button>`;
  }
  // un POST per mail, con i gesti di sempre: ognuno ha la sua fotografia e la sua riga nel registro
  async function perOgni(ids, url, corpo) {
    let ok = 0;
    for (const id of ids) {
      try {
        const r = await fetch(url.replace("{id}", id), { method: "POST", headers: { "HX-Request": "true", "Content-Type": "application/x-www-form-urlencoded" }, body: corpo || "" });
        if (r.ok) ok++;
      } catch (e) { }
    }
    return ok;
  }
  async function gestoBulk(k) {
    const ids = [...spunte];
    if (k === "annulla") { spunte.clear(); return bulk(); }
    if (k === "ignora") {
      const n = await perOgni(ids, "/messaggio/{id}/ignora");
      spunte.clear(); bulk(); aggiorna();
      UI().toast(`${n} mail ignorate`, async () => { await perOgni(ids, "/messaggio/{id}/ripristina"); aggiorna(); UI().toast("Rimesse fra i da decidere"); });
    }
    if (k === "letti") {
      const n = await perOgni(ids, "/messaggio/{id}/letto", "letto=1");
      spunte.clear(); bulk(); aggiorna();
      UI().toast(n === ids.length ? `${n} mail segnate lette in Outlook` : `${n} di ${ids.length} segnate lette: per le altre Outlook non è raggiungibile da questo PC`);
    }
  }
  const aggiorna = () => document.body.dispatchEvent(new Event("inbox-aggiorna"));

  /* ---------------------------------------------------------------- i codici nel testo */
  function evidenzia(radice) {
    (radice || document).querySelectorAll(".corpo[data-codici]").forEach(c => {
      const codici = (c.dataset.codici || "").split("|").map(s => s.trim()).filter(s => s.length >= 4).slice(0, 40);
      if (!codici.length || c.dataset.fatto) return;
      c.dataset.fatto = "1";
      const re = new RegExp("(" + codici.map(s => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|") + ")", "gi");
      const w = document.createTreeWalker(c, NodeFilter.SHOW_TEXT, { acceptNode: n => n.parentElement.closest("mark, summary, script, style") ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT });
      const nodi = []; while (w.nextNode()) nodi.push(w.currentNode);
      for (const n of nodi) {
        if (!re.test(n.nodeValue)) continue; re.lastIndex = 0;
        const f = document.createDocumentFragment(); let da = 0, m;
        while ((m = re.exec(n.nodeValue))) {
          f.append(n.nodeValue.slice(da, m.index));
          const k = document.createElement("mark"); k.className = "cod"; k.textContent = m[0]; k.title = "codice letto dal Cockpit"; f.append(k);
          da = m.index + m[0].length;
        }
        f.append(n.nodeValue.slice(da)); n.replaceWith(f);
      }
    });
  }

  /* ---------------------------------------------------------------- l'anteprima di un allegato */
  function anteprima(a) {
    $("#strato").innerHTML = `<div class="velo" data-chiudi="1"><div class="visore-box" role="dialog" aria-label="Anteprima">
      <div class="visore-barra"><b>${UI().esc(a.dataset.anteprima)}</b><span class="sp"></span>
        <a class="btn piccolo" href="${UI().esc(a.href)}" target="_blank" rel="noopener">Apri in una scheda nuova</a>
        <button type="button" class="btn quieto piccolo" data-chiudi="1">Chiudi</button></div>
      <iframe src="${UI().esc(a.href)}" title="${UI().esc(a.dataset.anteprima)}"></iframe></div></div>`;
  }

  /* ---------------------------------------------------------------- il modulo «Nuova RFQ» */
  function riepilogo() {
    const f = $("#modulo-triage"), r = $("#riep-triage"); if (!f || !r) return;
    const cod = f.querySelectorAll("[name=codice]:checked").length + (f.identificativi?.value || "").split(",").filter(s => s.trim()).length;
    const ins = f.querySelectorAll("[name=insieme]:checked").length;
    const cl = f.cliente_id?.selectedOptions?.[0]?.textContent.split("—")[0].trim() || "";
    const sc = f.scadenza?.value ? f.scadenza.value.split("-").reverse().slice(0, 2).join("/") : "";
    r.innerHTML = `Nasce la RFQ${cl && !cl.startsWith("+") ? ` di <b>${UI().esc(cl)}</b>` : ""} con <b>${cod} prodott${cod === 1 ? "o" : "i"}</b>${sc ? ` · scadenza <b>${sc}</b>` : ""}${ins ? ` · con <b>${ins} mail collegat${ins === 1 ? "a" : "e"}</b>` : ""}`;
  }

  /* ---------------------------------------------------------------- dopo una decisione */
  let dopo = null; // { id, prossima, ignora }
  document.body.addEventListener("htmx:beforeRequest", e => {
    const elt = e.detail.elt, sub = e.detail.requestConfig?.triggeringEvent?.submitter;
    const d = (sub && sub.closest("[data-decisione]")) || (elt.closest && elt.closest("[data-decisione]"));
    if (!d) return;
    const id = aperta();
    dopo = { id, prossima: vicina(id), ignora: d.dataset.decisione === "ignora" || (sub && sub.dataset.decisione === "ignora") };
  });
  document.body.addEventListener("htmx:afterSwap", e => {
    const t = e.detail.target;
    if (t && t.id === "pannello") {
      segnaAperta(); evidenzia(t); riepilogo();
      if (dopo && e.detail.xhr && e.detail.xhr.status === 200 && !t.querySelector(".errore-box")) {
        const d = dopo; dopo = null;
        const avviso = t.querySelector(".avviso-pannello")?.textContent.trim() || "Fatto.";
        UI().toast(avviso, d.ignora ? () => {
          htmx.ajax("POST", `/messaggio/${d.id}/ripristina`, { target: "#pannello" }).then(() => { aggiorna(); UI().toast("Rimessa fra i da decidere"); });
        } : null);
        if (d.prossima) {
          let fatto = false;
          const apri = () => { if (fatto) return; fatto = true; const r = document.querySelector(`#lista .riga[data-id="${d.prossima}"]`); if (r) r.click(); };
          document.body.addEventListener("inbox-lista-pronta", apri, { once: true });
          setTimeout(apri, 2500);
        }
      } else if (dopo && t.querySelector(".errore-box")) dopo = null;
    }
    if (t && t.id === "inbox-stato" || e.detail.elt?.id === "inbox-stato") listaNuova = true;
  });
  // La lista nuova si usa dopo l'assestamento: prima htmx non ha ancora preparato le righe, e un clic su una riga
  // seguirebbe il link invece di aprire la mail nel pannello.
  let listaNuova = false;
  document.body.addEventListener("htmx:afterSettle", () => {
    if (!document.getElementById("inbox-stato")) return;
    bulk(); segnaAperta();
    if (listaNuova) { listaNuova = false; document.body.dispatchEvent(new Event("inbox-lista-pronta")); }
  });

  /* ---------------------------------------------------------------- clic */
  document.addEventListener("click", e => {
    const t = e.target;
    if (t.closest(".velo") && (t.closest("[data-chiudi]") && (t.closest("button[data-chiudi]") || t.classList.contains("velo")))) { $("#strato").innerHTML = ""; return; }
    const a = t.closest("a[data-anteprima]"); if (a) { e.preventDefault(); return anteprima(a); }
    const b = t.closest("[data-bulk]"); if (b) return gestoBulk(b.dataset.bulk);
    if (t.matches("[data-spunta]")) { t.checked ? spunte.add(t.dataset.spunta) : spunte.delete(t.dataset.spunta); return bulk(); }
    const r = t.closest("#lista .riga"); if (r) { ui()?.classList.add("aperto"); return; }
    const az = t.closest("[data-azione]"); if (!az) return;
    switch (az.dataset.azione) {
      case "prev": return vai(-1);
      case "next": return vai(1);
      case "indietro": return ui()?.classList.remove("aperto");
      case "rispondi": { const s = $("#rispondi"); if (!s) return; s.hidden = !s.hidden; document.querySelectorAll('[data-azione="rispondi"]').forEach(x => x.setAttribute("aria-pressed", String(!s.hidden))); if (!s.hidden) $("#risp-testo")?.focus(); return; }
      case "usa-bozza": { const s = $("#rispondi"), p = $(".bozza-assistente"); if (s && p) { s.hidden = false; $("#risp-testo").value = p.textContent; $("#risp-testo").focus(); } return; }
      case "scambia": { const f = az.closest("form, .campi, fieldset"), c = f?.querySelector("[name=buyer_cognome]"), n = f?.querySelector("[name=buyer_nome]"); if (c && n) [c.value, n.value] = [n.value, c.value]; return; }
    }
  });
  document.addEventListener("change", e => {
    if (e.target.id === "sel-multipla") { spunte.clear(); return bulk(); }
    if (e.target.closest("#modulo-triage")) riepilogo();
  });
  document.addEventListener("input", e => { if (e.target.closest("#modulo-triage")) riepilogo(); });

  /* ---------------------------------------------------------------- tastiera: Esc sempre, il resto se acceso */
  document.addEventListener("keydown", e => {
    const t = e.target, scrive = t.matches("input, textarea, select");
    if (e.key === "Escape") {
      if ($("#strato")?.innerHTML) { $("#strato").innerHTML = ""; return; }
      if (scrive) return t.blur();
      const chiudi = $("#pannello .foglio .foglio-testa [hx-get]"); if (chiudi) chiudi.click();
      return;
    }
    if (!UI()?.scorciatoie() || scrive || t.closest(".maniglia") || e.ctrlKey || e.metaKey || e.altKey || $("#strato")?.innerHTML) return;
    const clic = s => { const x = $(s); if (x) { e.preventDefault(); x.click(); } };
    switch (e.key.toLowerCase()) {
      case "j": case "arrowdown": e.preventDefault(); return vai(1);
      case "k": case "arrowup": e.preventDefault(); return vai(-1);
      case "/": e.preventDefault(); return $("#cerca-inbox")?.focus();
      case "n": return clic('#pannello .fare [data-azione="nuova"]');
      case "a": return clic('#pannello .fare [data-azione="aggancia"]');
      case "i": return clic('#pannello .fare [data-decisione="ignora"]');
      case "r": return clic('#pannello [data-azione="rispondi"]');
      case "o": return clic('#pannello [data-azione="outlook"]');
      case "x": { const id = aperta(), c = document.querySelector(`[data-spunta="${id}"]`); if ($("#sel-multipla")?.checked && c) { e.preventDefault(); c.checked = !c.checked; c.checked ? spunte.add(id) : spunte.delete(id); bulk(); } return; }
    }
  });

  function avvia() { bulk(); segnaAperta(); evidenzia(document); }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia); else avvia();
})();
