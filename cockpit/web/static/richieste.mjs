// Le Richieste nel browser: lo stato della pagina e' l'indirizzo (come prima), la RFQ scelta si apre accanto, le
// miniature dei disegni si disegnano con pdf.js (miniature.mjs) quando arrivano sullo schermo, e un disegno si
// guarda sopra la pagina. Le scorciatoie valgono solo se accese sotto l'ingranaggio.
import { miniature } from "./miniature.mjs";

const $ = (s) => document.querySelector(s);
const UI = () => window.CockpitUI;

// i parametri dell'indirizzo, tranne quelli che il controllo manda da se' (la ricerca, l'ordinamento)
window.cockpitStatoRichieste = (escludi) => {
  const p = new URLSearchParams(window.location.search), v = {};
  for (const [k, x] of p) if (!(escludi || []).includes(k)) v[k] = x;
  return v;
};

function segna() {
  const sel = $("#rq-det .det")?.dataset.rq || new URLSearchParams(location.search).get("sel") || "";
  document.querySelectorAll("[data-rq]").forEach((r) => { if (r.closest("#rq-det")) return; r.classList.toggle("sel", r.dataset.rq === sel); });
}
// la RFQ aperta finisce nell'indirizzo: il poll e «indietro» la ritrovano
function ricordaSel(id) {
  const u = new URL(location.href);
  if (id) u.searchParams.set("sel", id); else u.searchParams.delete("sel");
  history.replaceState(history.state, "", u);
}
function vai(d) {
  const rr = [...document.querySelectorAll("#rq-elenco a[data-rq]")];
  if (!rr.length) return;
  const i = rr.findIndex((r) => r.classList.contains("sel"));
  const j = i < 0 ? 0 : Math.max(0, Math.min(rr.length - 1, i + d));
  rr[j].click(); rr[j].scrollIntoView({ block: "nearest" });
}
function disegno(a) {
  $("#strato").innerHTML = `<div class="velo" data-chiudi="1"><div class="visore-box" role="dialog" aria-label="Disegno">
    <div class="visore-barra"><b>${UI().esc(a.dataset.titolo)}</b><span class="sp"></span>
      <a class="btn piccolo" href="${UI().esc(a.getAttribute("href"))}">Apri nella Distinta</a>
      <a class="btn piccolo" href="${UI().esc(a.dataset.anteprima)}" target="_blank" rel="noopener">Apri in una scheda nuova</a>
      <button type="button" class="btn quieto piccolo" data-chiudi="1">Chiudi</button></div>
    <iframe src="${UI().esc(a.dataset.anteprima)}" title="${UI().esc(a.dataset.titolo)}"></iframe></div></div>`;
}

document.addEventListener("click", (e) => {
  const t = e.target;
  if (t.closest(".velo") && (t.closest("button[data-chiudi]") || t.classList.contains("velo"))) { $("#strato").innerHTML = ""; return; }
  const p = t.closest(".prod[data-anteprima]"); if (p) { e.preventDefault(); return disegno(p); }
  const r = t.closest("#rq-elenco a[data-rq]"); if (r) { ricordaSel(r.dataset.rq); $("#ui")?.classList.add("aperto"); setTimeout(segna, 0); return; }
  if (t.closest('[data-azione="indietro"]')) { $("#ui")?.classList.remove("aperto"); ricordaSel(""); }
});
document.body.addEventListener("htmx:afterSettle", () => { segna(); miniature(); });
document.addEventListener("keydown", (e) => {
  const t = e.target, scrive = t.matches("input, textarea, select");
  if (e.key === "Escape") { if ($("#strato")?.innerHTML) { $("#strato").innerHTML = ""; return; } if (scrive) t.blur(); return; }
  if (!UI()?.scorciatoie() || scrive || t.closest(".maniglia") || e.ctrlKey || e.metaKey || e.altKey || $("#strato")?.innerHTML) return;
  const k = e.key.toLowerCase();
  if (k === "j" || k === "arrowdown") { e.preventDefault(); vai(1); }
  if (k === "k" || k === "arrowup") { e.preventDefault(); vai(-1); }
  if (k === "/") { e.preventDefault(); $("#rq-q")?.focus(); }
  if (k === "d") { const a = $('#rq-det .det-azioni a.primario'); if (a) { e.preventDefault(); a.click(); } }
  if (k === "f") { const a = $('.vista-seg a:not([aria-current])'); if (a) { e.preventDefault(); a.click(); } }
});
segna(); miniature();
