// Il comune di Inbox, Richieste e Anagrafica: i bordi trascinabili fra i pannelli, l'ingranaggio delle
// impostazioni in testata (disposizioni pronte e scorciatoie, spente di base) e l'avviso in fondo alla pagina.
//
// Le larghezze e le scorciatoie sono preferenze di chi guarda, come la rail chiusa: stanno nel localStorage
// del browser e non arrivano mai al server. La pagina dichiara che cosa ha con window.CockpitPagina:
//   { chiave, maniglie: { id: { v, def, min, max, dir, chiudi, nome } }, preset: [...], tasti: [[tasto, cosa]] }
(function () {
  "use strict";
  const leggi = (k, d) => { try { const v = localStorage.getItem(k); return v === null ? d : JSON.parse(v); } catch (e) { return d; } };
  const scrivi = (k, v) => { try { localStorage.setItem(k, JSON.stringify(v)); } catch (e) { } };
  const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const P = () => window.CockpitPagina || { chiave: "", maniglie: {}, preset: [], tasti: [] };
  const chiavePannelli = () => "cockpit.pannelli." + P().chiave;

  // l'altezza della testata: la pagina occupa il resto dello schermo e scorre dentro le colonne
  function misuraTestata() {
    const b = document.querySelector(".barra");
    if (b) document.documentElement.style.setProperty("--alto-barra", b.offsetHeight + "px");
  }
  window.addEventListener("resize", () => { misuraTestata(); applica(); });
  // la navigazione a sinistra si apre e si chiude (layout.html): lo spazio utile cambia
  document.addEventListener("click", e => { if (e.target.closest && e.target.closest(".rail-toggle")) setTimeout(applica, 0); });
  document.addEventListener("htmx:afterSettle", misuraTestata);

  /* ---------------------------------------------------------------- i bordi */
  const stato = () => leggi(chiavePannelli(), {});
  // La larghezza di partenza di un bordo: quella della pagina, o quella «stretta» quando lo spazio utile (lo
  // schermo meno la navigazione aperta) e' poco. Quella che l'utente ha trascinato vince sempre.
  function def(m) {
    const aperta = !document.body.classList.contains("rail-chiusa") && innerWidth > 900;
    const libero = innerWidth - (aperta ? 184 : 0);
    return m.stretta && libero < m.stretta.sotto ? m.stretta.w : m.def;
  }
  function applica() {
    const st = stato(), root = document.documentElement;
    for (const [id, m] of Object.entries(P().maniglie || {})) {
      const s = st[id] || {}, chiuso = !!(m.chiudi && s.chiuso);
      root.style.setProperty(m.v, chiuso ? "0px" : (s.w || def(m)) + "px");
      if (m.chiudi) root.classList.toggle("chiuso-" + m.chiudi, chiuso);
      document.querySelectorAll(`[data-maniglia="${id}"]`).forEach(el => {
        el.classList.toggle("chiuso", chiuso);
        el.setAttribute("aria-valuenow", chiuso ? 0 : (s.w || def(m)));
        el.setAttribute("aria-valuemin", m.min); el.setAttribute("aria-valuemax", m.max);
      });
    }
  }
  function salva(id, w, chiuso) { const st = stato(); st[id] = { w, chiuso }; scrivi(chiavePannelli(), st); applica(); }
  // I bordi nel markup del server sono <div class="maniglia" data-maniglia="m1"></div>: qui prendono ruolo,
  // etichetta e la linguetta per riaprire. Anche quelli che arrivano con un frammento HTMX.
  function prepara(radice) {
    (radice || document).querySelectorAll(".maniglia[data-maniglia]:not([role])").forEach(el => {
      const m = (P().maniglie || {})[el.dataset.maniglia]; if (!m) return;
      el.setAttribute("role", "separator"); el.setAttribute("aria-orientation", "vertical"); el.tabIndex = 0;
      el.setAttribute("aria-label", "Allarga o stringi " + m.nome);
      el.title = "Trascina per allargare o stringere " + m.nome + " · doppio clic: com'era";
      if (m.chiudi) el.insertAdjacentHTML("beforeend", `<button type="button" class="riapri" data-riapri="${esc(el.dataset.maniglia)}" title="Riapri ${esc(m.nome)}" aria-label="Riapri ${esc(m.nome)}">${m.dir > 0 ? "›" : "‹"}</button>`);
    });
    applica();
  }
  let tiro = null;
  document.addEventListener("pointerdown", e => {
    const h = e.target.closest && e.target.closest(".maniglia[data-maniglia]"); if (!h || e.target.closest(".riapri")) return;
    const id = h.dataset.maniglia, m = (P().maniglie || {})[id]; if (!m) return;
    const s = stato()[id] || {};
    tiro = { id, h, x: e.clientX, w: s.chiuso ? 0 : (s.w || def(m)) };
    try { h.setPointerCapture(e.pointerId); } catch (err) { }
    h.classList.add("tira"); document.body.classList.add("ui-tirando"); e.preventDefault();
  });
  document.addEventListener("pointermove", e => {
    if (!tiro) return;
    const m = P().maniglie[tiro.id], grezza = tiro.w + (e.clientX - tiro.x) * m.dir;
    const chiude = !!m.chiudi && grezza < m.min - 70, w = Math.max(m.min, Math.min(m.max, grezza));
    document.documentElement.style.setProperty(m.v, chiude ? "0px" : w + "px");
    if (m.chiudi) document.documentElement.classList.toggle("chiuso-" + m.chiudi, chiude);
    tiro.ultimo = { w: chiude ? ((stato()[tiro.id] || {}).w || def(m)) : w, chiuso: chiude };
  });
  document.addEventListener("pointerup", () => {
    if (!tiro) return;
    tiro.h.classList.remove("tira"); document.body.classList.remove("ui-tirando");
    if (tiro.ultimo) salva(tiro.id, tiro.ultimo.w, tiro.ultimo.chiuso);
    tiro = null;
  });
  document.addEventListener("dblclick", e => {
    const h = e.target.closest && e.target.closest(".maniglia[data-maniglia]"); if (!h) return;
    const m = P().maniglie[h.dataset.maniglia]; if (m) { const st = stato(); delete st[h.dataset.maniglia]; scrivi(chiavePannelli(), st); applica(); }
  });
  document.addEventListener("keydown", e => {
    const h = e.target.closest && e.target.closest(".maniglia[data-maniglia]"); if (!h) return;
    const id = h.dataset.maniglia, m = P().maniglie[id]; if (!m) return;
    const s = stato()[id] || {}, w = s.chiuso ? m.min : (s.w || def(m)), passo = e.shiftKey ? 64 : 16;
    const d = { ArrowLeft: -passo * m.dir, ArrowRight: passo * m.dir }[e.key];
    if (d !== undefined) { e.preventDefault(); salva(id, Math.max(m.min, Math.min(m.max, w + d)), false); h.focus(); }
    if (e.key === "Enter" && m.chiudi) { e.preventDefault(); salva(id, w, !s.chiuso); }
  });
  document.addEventListener("click", e => {
    const r = e.target.closest && e.target.closest("[data-riapri]"); if (!r) return;
    const id = r.dataset.riapri, m = P().maniglie[id]; salva(id, (stato()[id] || {}).w || def(m), false);
  });

  /* ---------------------------------------------------------------- l'ingranaggio */
  const scorciatoie = () => !!leggi("cockpit.scorciatoie", false);
  function pannelloImpostazioni() {
    const p = P(), on = scorciatoie();
    return `<div class="sez-imp"><b>Pannelli</b><small>Trascina i bordi fra i pannelli per allargarli o stringerli. Doppio clic su un bordo lo rimette com'era.</small>
        ${(p.preset || []).length ? `<div class="preset">${p.preset.map((x, i) => `<button type="button" data-preset="${i}"><span class="schema">${x.schema.map(([w, f]) => `<i style="flex:${w}" class="${f ? "forte" : ""}"></i>`).join("")}</span>${esc(x.nome)}<small>${esc(x.sotto)}</small></button>`).join("")}</div>` : ""}
        <button type="button" class="quieto" data-preset="reset">Ripristina le larghezze</button></div>
      <div class="sez-imp"><label class="interruttore"><input type="checkbox" id="imp-tasti" ${on ? "checked" : ""}> Scorciatoie da tastiera</label>
        <small>Spente di base: si decidono a fine lavori. Accese qui valgono solo su questo PC.</small>
        ${on && (p.tasti || []).length ? `<div class="tasti-mini">${p.tasti.map(([k, t]) => `<div><kbd>${esc(k)}</kbd>${esc(t)}</div>`).join("")}</div>` : ""}</div>`;
  }
  function montaIngranaggio() {
    const barra = document.querySelector(".barra");
    if (!barra || barra.querySelector(".ui-ingranaggio")) return;
    const g = document.createElement("span");
    g.className = "ui-ingranaggio";
    g.innerHTML = `<button type="button" aria-expanded="false" aria-label="Impostazioni" title="Impostazioni"><svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg></button>
      <div class="ui-pop" hidden></div>`;
    const u = barra.querySelector(".utente");
    barra.insertBefore(g, u || null);
    const b = g.querySelector("button"), pop = g.querySelector(".ui-pop");
    b.addEventListener("click", () => { pop.hidden = !pop.hidden; b.setAttribute("aria-expanded", String(!pop.hidden)); if (!pop.hidden) pop.innerHTML = pannelloImpostazioni(); });
    document.addEventListener("click", e => { if (!g.contains(e.target)) { pop.hidden = true; b.setAttribute("aria-expanded", "false"); } });
    pop.addEventListener("click", e => {
      const pr = e.target.closest("[data-preset]"); if (!pr) return;
      if (pr.dataset.preset === "reset") scrivi(chiavePannelli(), {});
      else { const x = P().preset[+pr.dataset.preset], st = {}; for (const [id, w, chiuso] of x.valori) st[id] = { w: w || P().maniglie[id].def, chiuso: !!chiuso }; scrivi(chiavePannelli(), st); }
      applica();
    });
    pop.addEventListener("change", e => {
      if (e.target.id !== "imp-tasti") return;
      scrivi("cockpit.scorciatoie", e.target.checked); pop.innerHTML = pannelloImpostazioni();
      toast(e.target.checked ? "Scorciatoie accese su questo PC" : "Scorciatoie spente");
    });
  }

  /* ---------------------------------------------------------------- l'avviso in fondo */
  let timer = null;
  function toast(testo, annulla, errore) {
    const root = document.getElementById("ui-toast"); if (!root) return;
    root.innerHTML = `<div class="toast ${errore ? "errore" : ""}" role="status"><span></span>${annulla ? `<button type="button" class="btn piccolo">Annulla</button>` : ""}</div>`;
    root.querySelector("span").textContent = testo; // il testo e' testo: puo' venire da un oggetto di mail
    if (annulla) root.querySelector("button").onclick = () => { root.innerHTML = ""; annulla(); };
    clearTimeout(timer); timer = setTimeout(() => { root.innerHTML = ""; }, annulla ? 8000 : 6000);
  }

  window.CockpitUI = { toast, applica, prepara, scorciatoie, esc, leggi, scrivi };
  function avvia() { misuraTestata(); montaIngranaggio(); prepara(document); }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia); else avvia();
  document.addEventListener("htmx:afterSettle", e => prepara(e.target));
})();
