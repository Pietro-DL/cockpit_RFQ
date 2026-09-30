// Le miniature dei disegni fuori dalla Distinta (Richieste): la prima pagina di un PDF, disegnata con pdf.js quando
// arriva sullo schermo, una volta sola per allegato e un file per volta. E' lo stesso modo della Distinta
// (distinta.mjs): <span class="mini" data-a="ALLEGATO"> riceve un'immagine, o «anteprima non disponibile».
const PDFJS = "/static/pdfjs-6.3.289/";
let pdfjsPromessa = null;
function pdfjs() {
  if (!pdfjsPromessa) {
    pdfjsPromessa = import(PDFJS + "pdf.min.mjs")
      .then((lib) => { lib.GlobalWorkerOptions.workerSrc = PDFJS + "pdf.worker.min.mjs"; return lib; })
      .catch(() => null);
  }
  return pdfjsPromessa;
}
const opzioni = (url) => ({
  url, isEvalSupported: false, enableXfa: false, verbosity: 0,
  wasmUrl: PDFJS + "wasm/", iccUrl: PDFJS + "iccs/", cMapUrl: PDFJS + "cmaps/", cMapPacked: true,
  standardFontDataUrl: PDFJS + "standard_fonts/",
});

const cache = new Map(); // allegato → dataURL, oppure "" = non disponibile
const coda = [];
let lavoro = false, osservatore = null;

function metti(m, url) {
  if (m.querySelector("img")) return;
  if (!url) { if (!m.classList.contains("ant")) m.classList.add("vuoto"); else m.textContent = "anteprima non disponibile"; return; }
  const i = document.createElement("img");
  i.src = url; i.alt = ""; m.textContent = ""; m.append(i);
}
async function disegna(a) {
  const lib = await pdfjs();
  if (!lib) return "";
  const task = lib.getDocument(opzioni("/allegato/" + a + "/anteprima"));
  try {
    const doc = await task.promise, pag = await doc.getPage(1);
    const v1 = pag.getViewport({ scale: 1 }), vp = pag.getViewport({ scale: 360 / Math.max(v1.width, v1.height) });
    const c = document.createElement("canvas");
    c.width = Math.ceil(vp.width); c.height = Math.ceil(vp.height);
    const ctx = c.getContext("2d");
    ctx.fillStyle = "#ffffff"; ctx.fillRect(0, 0, c.width, c.height);
    await pag.render({ canvasContext: ctx, viewport: vp, background: "#ffffff" }).promise;
    return c.toDataURL("image/png");
  } catch (e) {
    return "";
  } finally {
    task.destroy().catch(() => {});
  }
}
async function lavora() {
  if (lavoro) return;
  lavoro = true;
  while (coda.length) {
    const m = coda.shift();
    if (!m.isConnected) continue;
    const a = m.dataset.a;
    if (!cache.has(a)) cache.set(a, await disegna(a));
    document.querySelectorAll(`.mini[data-a="${a}"]`).forEach((x) => metti(x, cache.get(a)));
  }
  lavoro = false;
}
export function miniature() {
  if (osservatore) osservatore.disconnect();
  const da = [];
  document.querySelectorAll(".mini[data-a]").forEach((m) => { if (cache.has(m.dataset.a)) metti(m, cache.get(m.dataset.a)); else da.push(m); });
  if (!da.length) return;
  if (!("IntersectionObserver" in window)) { coda.push(...da); lavora(); return; }
  osservatore = new IntersectionObserver((voci) => {
    for (const v of voci) if (v.isIntersecting) { osservatore.unobserve(v.target); coda.push(v.target); }
    lavora();
  }, { rootMargin: "200px" });
  da.forEach((m) => osservatore.observe(m));
}
