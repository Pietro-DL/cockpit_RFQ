// «Aggiungi file alla RFQ» (14/10): il trascinamento sulla zona, l'elenco dei file scelti, i limiti controllati
// prima dell'invio e la barra di avanzamento. L'invio lo fa htmx (hx-post multipart sul form #rfq-carica, risposta
// in #thread); qui solo la comodità. Gli ascoltatori stanno sul documento, perché il form si rifà a ogni gesto.

const MB = 1024 * 1024;
const formDi = (x) => (x instanceof Element ? x.closest("#rfq-carica") : null);
const zonaDi = (e) => (e.target instanceof Element ? e.target.closest("[data-rfq-carica-zona]") : null);

function errore(form, testo) {
  const p = form && form.querySelector("[data-rfq-carica-errore]");
  if (!p) return;
  p.textContent = testo || "";
  p.hidden = !testo;
}

// controlla: i limiti che il server applica (per file e in tutto), detti prima di mandare centinaia di MB
function controlla(form) {
  const input = form.querySelector("input[type=file]");
  const files = Array.from((input && input.files) || []);
  const perFile = Number(form.dataset.maxFile || 0) * MB;
  const totale = Number(form.dataset.maxTotale || 0) * MB;
  const grandi = perFile ? files.filter((f) => f.size > perFile).map((f) => f.name) : [];
  if (grandi.length) return `oltre ${form.dataset.maxFile} MB per file: ${grandi.join(", ")}`;
  const somma = files.reduce((s, f) => s + f.size, 0);
  if (totale && somma > totale) return `in tutto ${Math.ceil(somma / MB)} MB, il limite è ${form.dataset.maxTotale} MB: dividi in più invii`;
  if (files.length > 50) return `${files.length} file: al massimo 50 per volta`;
  return "";
}

function mostraScelti(form) {
  const input = form.querySelector("input[type=file]");
  const out = form.querySelector("[data-rfq-carica-scelti]");
  if (!input || !out) return;
  const nomi = Array.from(input.files || []).map((f) => f.name);
  out.textContent = nomi.length ? `${nomi.length} scelt${nomi.length === 1 ? "o" : "i"}: ${nomi.join(", ")}` : "";
  errore(form, controlla(form));
}

document.addEventListener("dragover", (e) => {
  const z = zonaDi(e);
  if (!z) return;
  e.preventDefault();
  z.classList.add("sopra");
});
document.addEventListener("dragleave", (e) => {
  const z = zonaDi(e);
  if (z && !z.contains(e.relatedTarget)) z.classList.remove("sopra");
});
document.addEventListener("drop", (e) => {
  const z = zonaDi(e);
  if (!z) return;
  e.preventDefault();
  z.classList.remove("sopra");
  const input = z.querySelector("input[type=file]");
  const dt = e.dataTransfer;
  if (!input || !dt || !dt.files.length) return;
  // una cartella trascinata arriva come «file» senza contenuto e farebbe fallire tutta la richiesta: si toglie
  const tieni = new DataTransfer();
  let cartelle = 0;
  Array.from(dt.items || []).forEach((it, i) => {
    const voce = typeof it.webkitGetAsEntry === "function" ? it.webkitGetAsEntry() : null;
    if (voce && voce.isDirectory) cartelle++;
    else if (dt.files[i]) tieni.items.add(dt.files[i]);
  });
  if (!dt.items || !dt.items.length) Array.from(dt.files).forEach((f) => tieni.items.add(f));
  input.files = tieni.files;
  const form = z.closest("form");
  mostraScelti(form);
  if (cartelle) errore(form, `${cartelle === 1 ? "una cartella è stata lasciata fuori" : `${cartelle} cartelle sono state lasciate fuori`}: le cartelle non si caricano, comprimile in uno zip`);
});
document.addEventListener("change", (e) => {
  const form = formDi(e.target);
  if (form && e.target instanceof HTMLInputElement && e.target.type === "file") mostraScelti(form);
});

// prima dell'invio: oltre i limiti la richiesta non parte
document.body.addEventListener("htmx:confirm", (e) => {
  const form = formDi(e.target);
  if (!form || e.target !== form) return;
  const motivo = controlla(form);
  if (motivo) {
    e.preventDefault();
    errore(form, motivo);
  } else {
    errore(form, "");
  }
});

// avanzamento: htmx manda htmx:xhr:progress dall'elemento che ha fatto la richiesta
document.body.addEventListener("htmx:xhr:progress", (e) => {
  const form = formDi(e.target);
  const bar = form && form.querySelector("[data-rfq-carica-avanzamento]");
  if (!bar || !e.detail || !e.detail.lengthComputable) return;
  bar.hidden = false;
  bar.value = Math.round((e.detail.loaded / e.detail.total) * 100);
});

// la connessione cade o il server risponde con un errore: senza questo la pagina resterebbe muta
for (const evento of ["htmx:sendError", "htmx:responseError"]) {
  document.body.addEventListener(evento, (e) => {
    const form = formDi(e.target);
    if (!form) return;
    const bar = form.querySelector("[data-rfq-carica-avanzamento]");
    if (bar) bar.hidden = true;
    errore(form, "Il caricamento non è arrivato in fondo (connessione interrotta o errore del server). " +
      "Ricarica la pagina per vedere che cosa è entrato: i file già arrivati non si duplicano se li rimandi.");
  });
}
