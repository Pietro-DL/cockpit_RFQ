// L1 — giro 5, A1d: il DOPPIO DI PROVA di pdf.js, usato solo da prove.cjs (QFE-03). NON è pdf.js.
//
// motore_a.mjs carica pdf.js con una sola import() (il caricatore pdfjs(), piano del frontend §7.4); prove.cjs la
// cambia in window.__importa(…), che restituisce `lib` di questo modulo. Ha solo le parti che la pagina usa:
//   GlobalWorkerOptions.workerSrc, new PDFWorker({name}), AnnotationMode.DISABLE, getDocument(params) → task
//   {promise, destroy()}, doc {numPages, getPage(n), destroy()}, page {getViewport({scale}), render(params) → {promise}}.
// Niente disegno vero: render() registra la chiamata e basta (in jsdom il canvas non disegna).
//
// Che cosa misura, per le prove: ogni PDFWorker creato; ogni getDocument con i suoi parametri (data o url); i
// documenti aperti insieme (aperti, massimoAperti: la coda delle miniature ne tiene al massimo 2) e quelli distrutti
// (task.destroy()); ogni getPage e ogni render.
//
// Due modi:
//   - "auto": il documento si apre al giro dopo (una promessa già risolta);
//   - "manuale": il documento resta in attesa finché la prova non chiama rilascia(n) o fallisci(n): così si vede quanti
//     documenti la pagina tiene aperti insieme («promesse a comando»).
// I byte arrivano in params.data (la GET del PDF la fa motore_a.mjs, D-PF-08) oppure, con params.url (il visore),
// da opzioni.perIndirizzo(url). Un contenuto che non comincia con «%PDF-» si rifiuta come farebbe pdf.js
// (InvalidPDFException). Il numero di pagine e la misura si leggono dai byte (/Type /Page, /MediaBox): i PDF ACME del
// kit (pdf/) hanno una e due pagine.
"use strict";

const TESTA = "%PDF-";

// byteDi: i byte di params.data (Uint8Array, ArrayBuffer o Buffer, anche di un altro «mondo» JS come la finestra jsdom)
function byteDi(x) {
  if (x == null) return null;
  if (typeof x === "string") return Buffer.from(x, "latin1");
  if (ArrayBuffer.isView(x)) return Buffer.from(x.buffer, x.byteOffset, x.byteLength);
  if (Object.prototype.toString.call(x) === "[object ArrayBuffer]") return Buffer.from(new Uint8Array(x));
  return null;
}

function contaPagine(b) {
  const s = Buffer.from(b).toString("latin1");
  const n = (s.match(/\/Type\s*\/Page(?![A-Za-z])/g) || []).length;
  return n || 1;
}

function misura(b) {
  const s = Buffer.from(b).toString("latin1");
  const m = s.match(/\/MediaBox\s*\[\s*(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s*\]/);
  if (!m) return { w: 842, h: 595 };
  return { w: Number(m[3]) - Number(m[1]), h: Number(m[4]) - Number(m[2]) };
}

function creaPdfjsFinto(opzioni = {}) {
  const stato = {
    modo: opzioni.modo === "manuale" ? "manuale" : "auto",
    lavoratori: [], // ogni new PDFWorker(params): {name, distrutto}
    documenti: [], // ogni getDocument: {n, params, url, dati, esito: attesa|aperto|rifiutato|distrutto, distrutto}
    attesa: [], // le aperture ferme in modo manuale, in ordine
    aperti: 0,
    massimoAperti: 0,
    distrutti: 0,
    pagine: [], // ogni getPage(n)
    disegni: [], // ogni render: {pagina, annotationMode, canvas, contesto, larghezza}
  };

  class PDFWorker {
    constructor(params) {
      this.name = params && params.name;
      this.destroyed = false;
      stato.lavoratori.push(this);
    }
    destroy() {
      this.destroyed = true;
    }
  }
  const AnnotationMode = Object.freeze({ DISABLE: 0, ENABLE: 1, ENABLE_FORMS: 2, ENABLE_STORAGE: 3 });
  const GlobalWorkerOptions = { workerSrc: "", workerPort: null };

  function pagina(i, m) {
    return {
      pageNumber: i,
      rotate: 0,
      view: [0, 0, m.w, m.h],
      getViewport({ scale = 1, rotation = 0 } = {}) {
        return { width: m.w * scale, height: m.h * scale, scale, rotation, transform: [scale, 0, 0, -scale, 0, m.h * scale] };
      },
      render(params) {
        const p = params || {};
        stato.disegni.push({
          pagina: i,
          annotationMode: p.annotationMode,
          canvas: !!p.canvas,
          contesto: !!p.canvasContext,
          larghezza: p.viewport ? p.viewport.width : undefined,
        });
        return { promise: Promise.resolve(), cancel() {} };
      },
      getTextContent() {
        return Promise.resolve({ items: [], styles: {} });
      },
      cleanup() {},
    };
  }

  function documento(task, b) {
    const n = contaPagine(b);
    const m = misura(b);
    return {
      numPages: n,
      fingerprints: ["acme-" + task.docId],
      loadingTask: task,
      getPage(i) {
        stato.pagine.push(i);
        if (!(i >= 1 && i <= n)) return Promise.reject(new Error("Invalid page request."));
        return Promise.resolve(pagina(i, m));
      },
      cleanup() {
        return Promise.resolve();
      },
      destroy() {
        return task.destroy();
      },
    };
  }

  function getDocument(params) {
    const p = typeof params === "string" ? { url: params } : params || {};
    const voce = {
      n: stato.documenti.length + 1,
      params: p,
      url: p.url ? String(p.url) : "",
      dati: byteDi(p.data),
      esito: "attesa",
      distrutto: false,
    };
    let risolvi;
    let rifiuta;
    const promise = new Promise((ok, ko) => {
      risolvi = ok;
      rifiuta = ko;
    });
    // come pdf.js: una promessa rifiutata senza nessuno che la ascolti non deve fermare Node
    promise.catch(() => {});
    const task = {
      docId: "d" + voce.n,
      promise,
      destroyed: false,
      onProgress: null,
      onPassword: null,
      destroy() {
        if (!task.destroyed) {
          task.destroyed = true;
          voce.distrutto = true;
          stato.aperti--;
          stato.distrutti++;
          if (voce.esito === "attesa") {
            voce.esito = "distrutto";
            rifiuta(new Error("Worker was destroyed"));
          }
        }
        return Promise.resolve();
      },
    };
    voce.apri = () => {
      if (voce.esito !== "attesa") return;
      let b = voce.dati;
      if (!b && voce.url && typeof opzioni.perIndirizzo === "function") b = byteDi(opzioni.perIndirizzo(voce.url));
      if (!b || b.subarray(0, TESTA.length).toString("latin1") !== TESTA) {
        voce.esito = "rifiutato";
        const e = new Error("Invalid PDF structure.");
        e.name = "InvalidPDFException";
        rifiuta(e);
        return;
      }
      voce.esito = "aperto";
      risolvi(documento(task, b));
    };
    voce.fallisci = (err) => {
      if (voce.esito !== "attesa") return;
      voce.esito = "rifiutato";
      rifiuta(err || new Error("errore di prova"));
    };
    stato.aperti++;
    stato.massimoAperti = Math.max(stato.massimoAperti, stato.aperti);
    stato.documenti.push(voce);
    if (stato.modo === "manuale") stato.attesa.push(voce);
    else Promise.resolve().then(voce.apri);
    return task;
  }

  function ferme() {
    stato.attesa = stato.attesa.filter((v) => v.esito === "attesa");
    return stato.attesa;
  }

  return {
    lib: { GlobalWorkerOptions, PDFWorker, AnnotationMode, getDocument, version: "finto" },
    stato,
    // rilascia: apre i primi n documenti fermi (modo manuale); ritorna quanti ne ha aperti
    rilascia(n = 1) {
      let k = 0;
      for (const v of ferme().slice(0, n)) {
        v.apri();
        k++;
      }
      return k;
    },
    // fallisci: rifiuta i primi n documenti fermi con l'errore dato
    fallisci(n = 1, err) {
      let k = 0;
      for (const v of ferme().slice(0, n)) {
        v.fallisci(err);
        k++;
      }
      return k;
    },
    fermi() {
      return ferme().length;
    },
    cambiaModo(m) {
      stato.modo = m === "manuale" ? "manuale" : "auto";
    },
  };
}

module.exports = { creaPdfjsFinto, contaPagine, byteDi };
