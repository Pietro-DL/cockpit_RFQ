// L'Anagrafica nel browser: l'elenco si filtra e si ordina qui, la barra «modifiche non salvate» compare quando si
// cambia qualcosa, le famiglie di codice dicono subito se riconoscono il loro esempio, e «Nuova famiglia dagli
// esempi» propone la regola spiegata in parole. Niente si scrive da qui: si salva con i bottoni dei moduli, che
// passano dallo stesso controllo di sempre sul server.
(function () {
  "use strict";
  const $ = s => document.querySelector(s), $$ = s => [...document.querySelectorAll(s)];
  const UI = () => window.CockpitUI;

  /* ---------------------------------------------------------------- l'elenco */
  let filtro = "tutti";
  function elenco() {
    const q = ($("#cerca-ana")?.value || "").trim().toLowerCase(), ord = $("#ordina-ana")?.value || "";
    const voci = $$("#ae-voci .ae-voce");
    voci.forEach(v => {
      const ok = (!q || (v.dataset.nome || "").toLowerCase().includes(q)) &&
        (filtro === "tutti" || (filtro === "incompleti" ? v.dataset.incompleto === "true" : v.dataset.spento === "true"));
      v.hidden = !ok;
    });
    if (ord) {
      const box = $("#ae-voci"), num = (v, k) => +(v.dataset[k] || 0);
      voci.sort((a, b) => ord === "nome" ? a.dataset.nome.localeCompare(b.dataset.nome) : ord === "rfq" ? num(b, "rfq") - num(a, "rfq") || num(b, "peso") - num(a, "peso") : num(b, "peso") - num(a, "peso") || a.dataset.nome.localeCompare(b.dataset.nome))
        .forEach(v => box.append(v));
    }
  }

  /* ---------------------------------------------------------------- le regole: Go (RE2) letto da JavaScript */
  // Le regole si scrivono per il motore in Go: (?P<nome>…) e' il gruppo con il nome. JavaScript lo scrive (?<nome>…).
  function reJs(re) { try { return new RegExp(re.replace(/\(\?P</g, "(?<")); } catch (e) { return null; } }
  function famiglie() {
    $$("#famiglie .fam:not([hidden])").forEach(f => {
      const re = f.querySelector("[name=fam_regex]")?.value.trim() || "", es = f.querySelector("[name=fam_esempio]")?.value.trim() || "";
      const chip = f.querySelector(".esito-fam");
      if (!re && f.classList.contains("nuova")) return;
      const r = reJs(re), ok = !!r && !!es && r.test(es);
      f.classList.toggle("rotta", !ok);
      if (chip) { chip.className = "chip esito-fam " + (ok ? "ok" : "bad"); chip.textContent = ok ? "riconosce il suo esempio" : !r ? "regola scritta male" : !es ? "manca l'esempio" : "l'esempio non passa"; }
    });
  }

  // dagli esempi a una regola: cifre, lettere e separatori. Con la stessa forma, le lettere uguali restano scritte,
  // il numero di cifre si allarga al minimo e al massimo visti, e un separatore visto anche una sola volta diventa
  // facoltativo; con forme diverse la regola le elenca tutte. La regola e' RE2: la legge il motore in Go.
  function pezzi(s) {
    const out = []; let sep = "";
    for (const m of s.trim().matchAll(/(\d+)|([A-Za-z]+)|([ _\-./])|(.)/g)) {
      if (m[3]) { sep += m[3]; continue; }
      out.push({ t: m[1] ? "d" : m[2] ? "L" : "x", v: m[0], sep }); sep = "";
    }
    return out;
  }
  const scappa = s => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const nomeSep = c => ({ " ": "spazio", "_": "trattino basso", "-": "trattino", ".": "punto", "/": "barra" }[c] || `«${c}»`);
  function genera(esempi) {
    const ee = esempi.map(s => s.trim()).filter(Boolean);
    if (!ee.length) return null;
    const pp = ee.map(pezzi), forma = p => p.map(x => x.t).join("");
    const parti = (p, lista) => p.map((x, i) => {
      const tutti = lista.map(q => q[i]), seps = [...new Set(tutti.map(q => q.sep).join(""))].join(""), sempre = tutti.every(q => q.sep);
      const sp = seps ? (seps.length === 1 ? scappa(seps) : `[${seps.replace(/[\]\\-]/g, "\\$&")}]`) + (sempre ? "" : "?") : "";
      const vv = [...new Set(tutti.map(q => q.v.toUpperCase()))], n = tutti.map(q => q.v.length), a = Math.min(...n), b = Math.max(...n);
      let re, parole;
      if (x.t === "d") { re = a === b ? `\\d{${a}}` : `\\d{${a},${b}}`; parole = a === b ? `${a} cifr${a === 1 ? "a" : "e"}` : `da ${a} a ${b} cifre`; }
      else if (x.t === "L") { if (vv.length === 1 && vv[0].length > 1) { re = scappa(vv[0]); parole = `«${vv[0]}»`; } else { re = a === b ? (a === 1 ? "[A-Z]" : `[A-Z]{${a}}`) : `[A-Z]{${a},${b}}`; parole = a === 1 && b === 1 ? "una lettera" : `${a === b ? a : a + "-" + b} lettere`; } }
      else { re = scappa(x.v); parole = `«${x.v}»`; }
      const sepP = seps ? `${sempre ? "" : "con o senza "}${[...seps].map(nomeSep).join(" o ")} e ` : "";
      return { re: sp + re, parole: (i ? sepP : "") + parole };
    });
    const forme = [...new Set(pp.map(forma))];
    let re, parole;
    if (forme.length === 1) { const pr = parti(pp[0], pp); re = pr.map(x => x.re).join(""); parole = pr.map(x => x.parole).join(", poi "); }
    else {
      const alt = forme.map(f => { const g = pp.filter(p => forma(p) === f), pr = parti(g[0], g); return { re: pr.map(x => x.re).join(""), p: pr.map(x => x.parole).join(", poi ") }; });
      re = `(?:${alt.map(a => a.re).join("|")})`; parole = "una di queste forme: " + alt.map(a => a.p).join(" · oppure ");
    }
    return { re: `\\b${re}\\b`, parole, esempi: ee };
  }
  let proposta = null;
  function costruttore() {
    const box = $("#cost-esito"), usa = $("#cost-usa"); if (!box) return;
    const es = ($("#cost-es")?.value || "").split(","), no = ($("#cost-no")?.value || "").split(",").map(s => s.trim()).filter(Boolean);
    proposta = genera(es);
    box.hidden = usa.hidden = !proposta;
    if (!proposta) return;
    const r = reJs(proposta.re), esc = UI().esc;
    box.innerHTML = `<span><b>La regola che ne esce:</b> ${esc(proposta.parole)}.</span><span class="mono k" style="overflow-wrap:anywhere">${esc(proposta.re)}</span>
      <span class="lista-chip">${proposta.esempi.map(x => { const ok = r.test(x); return `<span class="es ${ok ? "si" : "no"}">${ok ? "✓" : "✗"} ${esc(x)}</span>`; }).join("")}
      ${no.map(x => { const bad = r.test(x); return `<span class="es ${bad ? "no" : "si"}" title="${bad ? "lo riconosce, e non dovrebbe" : "giustamente non lo riconosce"}">${bad ? "✗ riconosce" : "✓ ignora"} ${esc(x)}</span>`; }).join("")}</span>`;
  }
  function usaProposta() {
    const f = $("#fam-nuova"); if (!f || !proposta) return;
    f.hidden = false;
    f.querySelector("[name=fam_descrizione]").value = "codici: " + proposta.parole;
    f.querySelector("[name=fam_esempio]").value = proposta.esempi[0];
    f.querySelector("[name=fam_regex]").value = proposta.re;
    sporco(f.closest("form")); famiglie(); prova();
    f.scrollIntoView({ block: "center" });
    UI().toast("Famiglia aggiunta al modulo: controlla la descrizione e salva le regole");
  }

  /* ---------------------------------------------------------------- «modifiche non salvate» e il banco di prova */
  function sporco(form) { const b = form?.querySelector(".salva-barra"); if (b) b.hidden = false; }
  function prova() { const f = $("#form-prova"); if (f && window.htmx) htmx.trigger(f, "prova"); }
  // Il mittente di sistema (giro 4, 4.13b) si prova solo con la prova a pagina intera (POST …/prova senza htmx), che
  // usa le regole salvate: con il campo pieno il banco non risponde mentre si scrive, e «Prova» manda il modulo.
  document.addEventListener("htmx:confirm", e => {
    const f = e.target; if (f.id !== "form-prova") return;
    const m = f.querySelector("[name=mittente]"); if (!m || !m.value.trim()) return;
    e.preventDefault();
    if (e.detail.triggeringEvent?.type === "submit") f.submit();
  });
  function notaMittente() {
    const m = $("#form-prova [name=mittente]"), n = $("#nota-mittente"); if (m && n) n.hidden = !m.value.trim();
  }

  /* ---------------------------------------------------------------- qualifiche: solo chi fa la lavorazione */
  function qualifiche() {
    const l = $("#q-lav"), s = $("#q-forn"); if (!l || !s) return;
    let primo = null;
    [...s.options].forEach(o => { const fa = (o.dataset.lav || "").split(" ").includes(l.value); o.hidden = o.disabled = !fa; if (fa && !primo) primo = o; });
    if (s.selectedOptions[0]?.disabled) s.value = primo ? primo.value : "";
    $("#q-btn").disabled = !primo;
    $("#q-nota").textContent = primo ? "Nella tendina ci sono solo i fornitori che fanno quella lavorazione: una qualifica impossibile non si può nemmeno scegliere."
      : "Nessun fornitore fa questa lavorazione: prima si aggiunge fra le sue capacità, nella scheda del fornitore.";
  }

  document.addEventListener("input", e => {
    const t = e.target;
    if (t.id === "cerca-ana") return elenco();
    if (t.id === "cost-es" || t.id === "cost-no") return costruttore();
    if (t.id === "g-peso") { const o = t.nextElementSibling; if (o) o.value = t.value; }
    const f = t.closest(".form-scheda"); if (f) sporco(f);
    if (t.closest("#form-regole")) { famiglie(); prova(); }
    if (t.name === "mittente" && t.closest("#form-prova")) notaMittente();
  });
  document.addEventListener("change", e => {
    if (e.target.id === "ordina-ana") return elenco();
    if (e.target.id === "q-lav") return qualifiche();
    const f = e.target.closest(".form-scheda"); if (f) sporco(f);
  });
  document.addEventListener("reset", e => { const b = e.target.querySelector(".salva-barra"); if (b) setTimeout(() => { b.hidden = true; famiglie(); prova(); }, 0); });
  document.addEventListener("click", e => {
    const f = e.target.closest("[data-filtro]");
    if (f) { filtro = f.dataset.filtro; $$("[data-filtro]").forEach(b => b.setAttribute("aria-pressed", String(b === f))); return elenco(); }
    if (e.target.closest("#cost-usa")) return usaProposta();
    // una sola casella del fabbisogno aperta per volta
    const d = e.target.closest(".cella-det"); $$(".cella-det[open]").forEach(x => { if (x !== d) x.open = false; });
  });
  document.addEventListener("keydown", e => {
    const t = e.target, scrive = t.matches("input, textarea, select");
    if (e.key === "Escape") { $$(".cella-det[open]").forEach(x => x.open = false); if (scrive) t.blur(); return; }
    if (!UI()?.scorciatoie() || scrive || t.closest(".maniglia") || e.ctrlKey || e.metaKey || e.altKey) return;
    const k = e.key.toLowerCase();
    if (k === "/") { e.preventDefault(); $("#cerca-ana")?.focus(); }
    if (k === "j" || k === "k") {
      const voci = $$("#ae-voci .ae-voce:not([hidden])"), i = voci.findIndex(v => v.hasAttribute("aria-current"));
      const n = voci[Math.max(0, Math.min(voci.length - 1, i + (k === "j" ? 1 : -1)))];
      if (n) { e.preventDefault(); n.click(); }
    }
  });
  function avvia() { elenco(); famiglie(); qualifiche(); }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", avvia); else avvia();
})();
