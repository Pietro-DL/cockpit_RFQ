// distinta_unica_tif.js: il worker delle anteprime TIFF della Distinta unica (richiesta del frontend del 22/10: alcuni 2D
// TIFF non si vedono in anteprima). Edge e Chrome non sanno disegnare un TIFF; i 2D di alcuni clienti sono TIFF CCITT G4 a 1 bit
// fino a 14043×9933 px (circa 140 Mpx). Il worker riceve i byte che la pagina ha preso con il ponte (byteContenuto, la
// rotta S5 che c'era già), decodifica la prima pagina con UTIF.js (vendor-utif-3.1.0, caricato qui con importScripts,
// solo quando serve) e la riduce per aree direttamente dai bit a una tela piccola (la miniatura, il visore: al più circa
// 2000 px di lato), contando i pixel neri per cella perché le linee sottili restino visibili. Mai una conversione RGBA a
// piena risoluzione (circa 560 MB per 140 Mpx). Non fa richieste oltre allo script di UTIF; non parla con il server.
//
// Messaggio in: {id, chiave, utif, buf, maxW, maxH, spessa}. Risposta: {id, ok: true, w, h, W, H, rgba (ArrayBuffer, W×H×4),
// compressione, msDecodifica, msRiduzione} oppure {id, ok: false, errore}. L'ultima immagine decodificata resta in
// memoria per «chiave» (il visore dopo la miniatura dello stesso file non la decodifica di nuovo).
"use strict";

// i nomi delle compressioni TIFF (tag 259) che l'errore dice; AMMESSE quelle che UTIF decodifica senza librerie in più
const NOMI = {
  1: "non compresso", 2: "CCITT RLE", 3: "CCITT G3", 4: "CCITT G4", 5: "LZW", 6: "JPEG vecchio", 7: "JPEG", 8: "Deflate",
  32773: "PackBits", 32946: "Deflate", 34661: "JBIG", 34676: "SGILog", 34712: "JPEG 2000", 34887: "LERC", 34925: "LZMA",
  50000: "ZSTD", 50001: "WebP",
};
const AMMESSE = new Set([1, 3, 4, 5, 32773]);
const MAX_PIXEL = 400e6; // oltre, nemmeno il buffer a 1 bit si tiene
const MAX_BYTE = 320e6; // il buffer decodificato (1 bit: 17 MB per 140 Mpx; 8 bit RGB: tre byte per pixel)

let caricato = "";
let ultima = null; // {chiave, img, compressione}

function nonSupportato(motivo) {
  const e = new Error("formato TIFF non supportato (" + motivo + ")");
  e.tif = true;
  return e;
}
function nomeCompressione(c) {
  return NOMI[c] || "compressione " + c;
}
function tag(img, n, d) {
  const v = img["t" + n];
  return v && v.length ? v[0] : d;
}

function decodifica(buf) {
  const u8 = new Uint8Array(buf);
  if (u8.length < 8) throw nonSupportato("file troppo corto");
  const ii = u8[0] === 0x49 && u8[1] === 0x49;
  const mm = u8[0] === 0x4d && u8[1] === 0x4d;
  if (!ii && !mm) throw nonSupportato("non è un file TIFF");
  const magia = ii ? u8[2] | (u8[3] << 8) : (u8[2] << 8) | u8[3];
  if (magia === 43) throw nonSupportato("BigTIFF");
  if (magia !== 42) throw nonSupportato("intestazione illeggibile");
  let ifds;
  try {
    ifds = self.UTIF.decode(buf);
  } catch (e) {
    throw nonSupportato("intestazione illeggibile");
  }
  const img = (ifds || []).find((x) => x && x.t256 && x.t257);
  if (!img) throw nonSupportato("nessuna pagina");
  const w = tag(img, 256, 0);
  const h = tag(img, 257, 0);
  const cmpr = tag(img, 259, 1);
  const nome = nomeCompressione(cmpr);
  if (!(w > 0 && h > 0)) throw nonSupportato(nome + ", dimensioni non valide");
  if (!AMMESSE.has(cmpr)) throw nonSupportato(nome);
  if (w * h > MAX_PIXEL) throw nonSupportato(nome + ", " + w + "×" + h + " px");
  const bps = tag(img, 258, 1);
  const spp = tag(img, 277, img.t258 ? img.t258.length : 1);
  if (Math.ceil((w * bps * spp) / 8) * h > MAX_BYTE) throw nonSupportato(nome + ", " + w + "×" + h + " px a " + bps * spp + " bit");
  if (!img.t273 && !img.t324) throw nonSupportato(nome + ", senza dati");
  try {
    self.UTIF.decodeImage(buf, img, ifds);
  } catch (e) {
    throw nonSupportato(nome + ", dati illeggibili");
  }
  if (!img.data || !img.data.length) throw nonSupportato(nome + ", dati illeggibili");
  // FillOrder 2 (il bit meno significativo prima): UTIF lo applica solo a G3 e G4; per gli altri i bit si girano qui
  if (tag(img, 266, 1) === 2 && cmpr !== 3 && cmpr !== 4 && bps === 1) {
    const g = new Uint8Array(256);
    for (let i = 0; i < 256; i++) {
      let r = 0;
      for (let k = 0; k < 8; k++) if (i & (1 << k)) r |= 128 >> k;
      g[i] = r;
    }
    const d = img.data;
    for (let i = 0; i < d.length; i++) d[i] = g[d[i]];
  }
  return { img, compressione: nome };
}

// riduci: la pagina in una tela di al più maxW×maxH (mai ingrandita), per aree. A 1 bit si contano i neri per cella
// (PhotometricInterpretation 0: 1 è nero; 1: 0 è nero) e la cella si scurisce più della media, perché una linea di un
// pixel in una cella di venti resti visibile; a 8 bit (grigi, RGB, palette) la media della cella.
function riduci(img, maxW, maxH, spessa) {
  const w = img.width;
  const h = img.height;
  const s = Math.min(1, maxW / w, maxH / h);
  const W = Math.max(1, Math.round(w * s));
  const H = Math.max(1, Math.round(h * s));
  const bps = tag(img, 258, 1);
  const spp = tag(img, 277, img.t258 ? img.t258.length : 1);
  const intp = tag(img, 262, bps === 1 ? 0 : 1);
  const data = img.data;
  const colx = new Int32Array(w);
  const nCol = new Uint32Array(W);
  for (let x = 0; x < w; x++) {
    const t = Math.min(W - 1, Math.floor((x * W) / w));
    colx[x] = t;
    nCol[t]++;
  }
  const righe = new Int32Array(h);
  const nRig = new Uint32Array(H);
  for (let y = 0; y < h; y++) {
    const t = Math.min(H - 1, Math.floor((y * H) / h));
    righe[y] = t;
    nRig[t]++;
  }
  const out = new Uint8ClampedArray(W * H * 4);
  if (bps === 1 && spp === 1 && (intp === 0 || intp === 1)) {
    const bpl = Math.ceil(w / 8);
    const nero = new Uint32Array(W * H);
    const inv = intp === 1 ? 255 : 0;
    for (let y = 0; y < h; y++) {
      const off = y * bpl;
      if (off + bpl > data.length) break;
      const base = righe[y] * W;
      for (let i = 0; i < bpl; i++) {
        const b = data[off + i] ^ inv;
        if (b === 0) continue;
        const x0 = i << 3;
        for (let k = 0; k < 8; k++) {
          if (b & (128 >> k)) {
            const x = x0 + k;
            if (x < w) nero[base + colx[x]]++;
          }
        }
      }
    }
    for (let ty = 0; ty < H; ty++) {
      for (let tx = 0; tx < W; tx++) {
        const i = ty * W + tx;
        const area = nCol[tx] * nRig[ty] || 1;
        const d = Math.pow(Math.min(1, (nero[i] / area) * 6), 0.6);
        const g = 255 - Math.round(d * 255);
        const q = i << 2;
        out[q] = out[q + 1] = out[q + 2] = g;
        out[q + 3] = 255;
      }
    }
    // «spessa» (le miniature, che il browser riduce ancora nella scheda e nel pannello): ogni pixel prende il più scuro
    // fra sé e i tre vicini a destra e sotto, così una linea di un pixel ne diventa due e non si spezza in puntini
    if (spessa) {
      for (let ty = 0; ty < H; ty++) {
        for (let tx = 0; tx < W; tx++) {
          const q = (ty * W + tx) << 2;
          let g = out[q];
          if (tx + 1 < W) g = Math.min(g, out[q + 4]);
          if (ty + 1 < H) g = Math.min(g, out[q + W * 4]);
          if (tx + 1 < W && ty + 1 < H) g = Math.min(g, out[q + W * 4 + 4]);
          out[q] = g;
        }
      }
      for (let i = 0; i < out.length; i += 4) out[i + 1] = out[i + 2] = out[i];
    }
    return { W, H, rgba: out };
  }
  let px;
  if (bps === 8 && (intp === 0 || intp === 1)) {
    px = (y, x, c) => {
      const v = data[(y * w + x) * spp];
      return intp === 0 ? 255 - v : v;
    };
  } else if (bps === 8 && intp === 2 && spp >= 3) {
    px = (y, x, c) => data[(y * w + x) * spp + c];
  } else if (bps === 8 && intp === 3 && img.t320) {
    const mappa = img.t320;
    px = (y, x, c) => mappa[c * 256 + data[y * w + x]] >> 8;
  } else {
    throw nonSupportato(nomeCompressione(tag(img, 259, 1)) + ", " + bps + " bit per campione, interpretazione " + intp);
  }
  const somma = new Float64Array(W * H * 3);
  const colore = !(bps === 8 && (intp === 0 || intp === 1));
  for (let y = 0; y < h; y++) {
    const base = righe[y] * W;
    for (let x = 0; x < w; x++) {
      const i = (base + colx[x]) * 3;
      if (colore) {
        somma[i] += px(y, x, 0);
        somma[i + 1] += px(y, x, 1);
        somma[i + 2] += px(y, x, 2);
      } else {
        const v = px(y, x, 0);
        somma[i] += v;
        somma[i + 1] += v;
        somma[i + 2] += v;
      }
    }
  }
  for (let ty = 0; ty < H; ty++) {
    for (let tx = 0; tx < W; tx++) {
      const i = ty * W + tx;
      const area = nCol[tx] * nRig[ty] || 1;
      const q = i << 2;
      out[q] = somma[i * 3] / area;
      out[q + 1] = somma[i * 3 + 1] / area;
      out[q + 2] = somma[i * 3 + 2] / area;
      out[q + 3] = 255;
    }
  }
  return { W, H, rgba: out };
}

self.onmessage = (ev) => {
  const m = ev.data || {};
  try {
    if (caricato !== m.utif) {
      self.importScripts(m.utif);
      if (!self.UTIF) throw new Error("il lettore dei TIFF non si carica");
      caricato = m.utif;
    }
    const t0 = performance.now();
    if (!ultima || ultima.chiave !== m.chiave || !m.chiave) {
      ultima = null; // la memoria della pagina di prima si libera prima di decodificare la nuova
      const d = decodifica(m.buf);
      ultima = { chiave: m.chiave, img: d.img, compressione: d.compressione };
    }
    const t1 = performance.now();
    const r = riduci(ultima.img, Math.max(1, m.maxW | 0), Math.max(1, m.maxH | 0), !!m.spessa);
    const t2 = performance.now();
    self.postMessage({ id: m.id, ok: true, w: ultima.img.width, h: ultima.img.height, W: r.W, H: r.H, rgba: r.rgba.buffer, compressione: ultima.compressione,
      msDecodifica: Math.round(t1 - t0), msRiduzione: Math.round(t2 - t1) }, [r.rgba.buffer]);
  } catch (e) {
    if (!(e && e.tif)) ultima = null;
    self.postMessage({ id: m.id, ok: false, errore: (e && e.tif && e.message) || "formato TIFF non supportato (" + ((e && e.message) || String(e)) + ")" });
  }
};
