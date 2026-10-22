# -*- coding: utf-8 -*-
"""Browser, richiesta del frontend del 22/10 (alcuni 2D TIFF non si vedono in anteprima), corsia TIF:
le anteprime dei 2D TIFF della Distinta unica. Edge e Chrome non disegnano un TIFF: la pagina prende i byte con il ponte
(byteContenuto, la rotta S5 di sempre) e li decodifica nel worker degli statici (distinta_unica_tif.js con UTIF.js).

Lo lancia TestDistintaUnicaTifNelBrowser (distinta_unica_tif_browser_test.go, tag `integrazione browser`) sul banco finto
di distinta_unica_banco_test.go con le quattro TIF sintetiche di testdata/distinta_unica_tif (aggiungiTif), `python -I`.

  T01  le miniature: la TIF G4 (nodo #12) e la LZW (#41) si disegnano nella scheda (un'immagine data: non bianca); la
       TIF rotta (#9) e la Deflate (#31) dicono «formato TIFF non supportato (…)»; il nodo #40, che ha anche i PDF
       associati al componente ACM-1400, mostra il PDF (con tutti e due vale il PDF); nessuno spostamento della pagina
       né delle schede (±1 px) fra i segnaposto e le miniature arrivate
  T02  il visore dal pannello del nodo: la pagina della G4 (1600×1131) su una tela non bianca, «Dettaglio» e «Adatta»
  T03  «Apri» della LZW nella Sorgente apre il visore della pagina (non una scheda nuova), con la pagina 800×566
  T04  il visore della TIF rotta dice «anteprima non disponibile: formato TIFF non supportato (…)»; la pagina resta in piedi
  T05  le richieste: i byte delle TIF passano dal ponte (Accept JSON, la rotta S5), nessuna richiesta fuori dagli statici e
       dalle rotte della pagina
Nessun pageerror e nessun errore in console in tutta la sessione (lo controlla main).

Dati di prova: quelli del banco (ACME, ACM-xxxx), più i 2D del motore per quattro nodi dello STEP del finito
(strutture_candidate[].nodi[].disegni.primario con anteprima.formato «tiff», come li dà valutazione.NodoBOM per un TIFF),
aggiunti alla risposta di /motore-a/dati da questo script (page.route), dichiarati tali.
"""
import argparse
import hashlib
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from playwright.sync_api import expect, sync_playwright  # noqa: E402

import distinta_unica as du  # noqa: E402

T_BREVE = du.T_BREVE
T_LUNGO = du.T_LUNGO
verifica = du.verifica
sid = du.sid

G4, LZW, DEFLATE, ROTTA = "ACM-1400_ASSIEME_G4.tif", "ACM-1400_A_LZW.tif", "ACM-1400_DEFLATE.tif", "ACM-1400_ROTTA.tif"
NODI = {"#12": G4, "#41": LZW, "#9": ROTTA, "#31": DEFLATE, "#40": G4}

PROVE = []


def prova(numero, nome):
    def deco(fn):
        PROVE.append((numero, nome, fn))
        return fn
    return deco


def inietta_tif(b):
    """GET /motore-a/dati: il 2D primario TIFF del motore per i nodi di NODI (anteprima.disponibile false, formato tiff,
    come per un TIFF vero), nelle strutture candidate del primo prodotto. Dati di prova dichiarati tali."""
    a1 = b.al["ACM-1000_assieme.stp"]

    def nodo(sha, chiave, nome):
        aid = b.al[nome]
        sha_f = hashlib.sha256(aid.encode("utf-8")).hexdigest()
        return {"nodo": {"rif": "nodo:%s:%s" % (sha, chiave), "allegato_id": a1}, "tipo": "parte", "origine_tipo": "",
                "classificazione": {}, "disegni": {"primario": {"allegato_id": aid, "sha256": sha_f, "nome_file": nome, "estensione": "tif",
                                                                  "anteprima": {"sha256": sha_f, "formato": "tiff", "allegato_id": aid, "disponibile": False}}}}

    def gestore(route):
        if "sorgente=1" in route.request.url:
            route.continue_()
            return
        r = route.fetch()
        try:
            j = r.json()
        except Exception:
            route.fulfill(response=r)
            return
        sha = ""
        for g in j.get("gruppi") or []:
            if g.get("allegato_id") == a1 and ":" in str(g.get("radice", "")):
                sha = str(g.get("radice")).split(":")[1]
        for pv in (j.get("esito") or {}).get("prodotti_valutati") or []:
            if pv.get("componente_id") == b.p1 and sha:
                pv["strutture_candidate"] = [{"allegato_id": a1, "sha256": sha, "radice": "nodo:%s:#1" % sha,
                                              "nodi": [nodo(sha, k, n) for k, n in NODI.items()]}]
        route.fulfill(response=r, json=j)
    return gestore


def scuri(p, sel, tela=False):
    """I pixel scuri (luminanza < 160) dell'immagine o della tela sel, ridisegnata su una tela di prova; -1 se non c'è."""
    return p.evaluate("""([s, tela]) => {
      const e = document.querySelector(s);
      if (!e) return -1;
      const w = tela ? e.width : e.naturalWidth, h = tela ? e.height : e.naturalHeight;
      if (!w || !h) return -1;
      const c = document.createElement('canvas'); c.width = w; c.height = h;
      const x = c.getContext('2d'); x.drawImage(e, 0, 0);
      const d = x.getImageData(0, 0, w, h).data; let n = 0;
      for (let i = 0; i < d.length; i += 4) if (0.3 * d[i] + 0.59 * d[i + 1] + 0.11 * d[i + 2] < 160) n++;
      return n;
    }""", [sel, tela])


def posizioni(p):
    return p.evaluate("""() => ({ y: window.scrollY, x: window.scrollX,
      schede: Object.fromEntries([...document.querySelectorAll('#du-vp .du-nc[data-k]')].map(e => {
        const r = e.getBoundingClientRect(); return [e.dataset.k, [r.left, r.top, r.width, r.height]]; })),
      mini: Object.fromEntries([...document.querySelectorAll('#du-vp .du-nc[data-k] .du-nc-mini')].map(e => {
        const r = e.getBoundingClientRect(); return [e.closest('.du-nc').dataset.k, [r.left, r.top, r.width, r.height]]; })) })""")


def apri_bozza(b):
    p = b.page
    p.route(re.compile(r"/motore-a/dati(\?|$)"), inietta_tif(b))
    b.apri()
    b.guarda(b.al["ACM-1000_assieme.stp"])
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    b.ok_finestra()
    expect(p.locator("#du-passo-revisione")).to_contain_text("versione 1", timeout=T_LUNGO)
    b.passo("revisione")
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10, timeout=T_LUNGO)


def scegli_scheda(b, k):
    p = b.page
    c = p.locator("#nc-" + sid(k) + " .du-nc-cod")
    c.scroll_into_view_if_needed()
    c.click()
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", k, timeout=T_BREVE)


@prova("T01", "le miniature delle TIF: G4 e LZW disegnate, la rotta e la Deflate con il motivo, la pagina ferma")
def t01(b):
    p = b.page
    apri_bozza(b)
    prima = posizioni(p)
    for k in ("k-12", "k-41"):
        img = p.locator("#nc-%s .du-nc-mini img" % k)
        expect(img).to_have_count(1, timeout=T_LUNGO)
        b.attendi(lambda: img.evaluate("i => i.complete && i.naturalWidth > 0"), "la miniatura di %s non si disegna" % k)
        verifica(img.get_attribute("src").startswith("data:image/png"), "la miniatura di %s non è un data: URL" % k)
        n = scuri(p, "#nc-%s .du-nc-mini img" % k)
        verifica(n > 500, "la miniatura di %s è bianca (%d pixel scuri)" % (k, n))
    for k, attesa in (("k-9", "formato TIFF non supportato ("), ("k-31", "formato TIFF non supportato (Deflate)")):
        m = p.locator("#nc-%s .du-nc-mini" % k)
        expect(m).to_have_attribute("data-du-stato", "errore", timeout=T_LUNGO)
        expect(m).to_contain_text("anteprima non disponibile")
        verifica(attesa in (m.get_attribute("title") or ""), "il suggerimento della miniatura di %s non dice %r: %r" % (k, attesa, m.get_attribute("title")))
    # i nodi con un PDF restano come prima; con il PDF e la TIF vale il PDF
    expect(p.locator("#nc-k-11 .du-nc-mini img, #nc-k-7 .du-nc-mini img")).to_have_count(2, timeout=T_LUNGO)
    m40 = p.locator("#nc-k-40 .du-nc-mini")
    verifica(m40.get_attribute("data-du-mini") != b.al[G4], "il nodo #40 mostra la TIF invece del PDF del componente")
    expect(p.locator("#nc-k-33 .du-nc-mini")).to_contain_text("nessun 2D associato")
    dopo = posizioni(p)
    verifica(abs(prima["y"] - dopo["y"]) <= 1 and abs(prima["x"] - dopo["x"]) <= 1, "la pagina è scorsa: %s → %s" % ((prima["x"], prima["y"]), (dopo["x"], dopo["y"])))
    for gruppo in ("schede", "mini"):
        for k, r in prima[gruppo].items():
            r2 = dopo[gruppo].get(k)
            verifica(r2 is not None, "%s %s sparita" % (gruppo, k))
            verifica(all(abs(a - c) <= 1 for a, c in zip(r, r2)), "%s %s si è spostata: %s → %s" % (gruppo, k, [round(v) for v in r], [round(v) for v in r2]))
    b.foto("T01_miniature_tif_dopo.png")


@prova("T02", "il visore della TIF G4 dal pannello del nodo: la pagina intera, «Dettaglio» e «Adatta»")
def t02(b):
    p = b.page
    scegli_scheda(b, "k-12")
    grande = p.locator("#du-nodo-2d")
    expect(grande.locator("img")).to_have_count(1, timeout=T_LUNGO)
    y0 = p.evaluate("() => window.scrollY")
    grande.click()
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    expect(b.finestra()).to_contain_text("Visore: " + G4)
    tela = p.locator("#du-visore-tela canvas")
    expect(tela).to_have_count(1, timeout=T_LUNGO)
    expect(tela).to_have_attribute("data-du-tif", "1600x1131")
    expect(p.locator("#du-visore-pagina")).to_contain_text("1600×1131")
    n = scuri(p, "#du-visore-tela canvas", tela=True)
    verifica(n > 2000, "la pagina del visore è bianca (%d pixel scuri)" % n)
    # «Adatta»: la riduzione per aree alla larghezza della finestra (pixel veri), non una riduzione del browser
    w, cw = tela.evaluate("c => c.width"), p.locator("#du-visore-tela").evaluate("e => e.clientWidth")
    verifica(w <= cw * p.evaluate("() => window.devicePixelRatio || 1") and w >= cw * 0.8, "la tela di «Adatta» è larga %s per una finestra di %s" % (w, cw))
    p.click("#du-visore-dettaglio")
    expect(p.locator("#du-visore-dettaglio")).to_have_attribute("aria-pressed", "true", timeout=T_LUNGO)
    tela = p.locator("#du-visore-tela canvas")
    w = tela.evaluate("c => c.width")
    verifica(w == 1600, "la tela di «Dettaglio» è larga %s, attesa 1600 (al più 2000 px, mai ingrandita)" % w)
    verifica(tela.evaluate("c => c.getBoundingClientRect().width") > p.locator("#du-visore-tela").evaluate("e => e.clientWidth") - 1,
             "«Dettaglio» non mostra la tela a grandezza")
    b.foto("T02_visore_dettaglio_dopo.png")
    p.click("#du-visore-adatta")
    expect(p.locator("#du-visore-adatta")).to_have_attribute("aria-pressed", "true", timeout=T_LUNGO)
    tela = p.locator("#du-visore-tela canvas")
    verifica(tela.evaluate("c => c.getBoundingClientRect().width") <= p.locator("#du-visore-tela").evaluate("e => e.clientWidth") + 1,
             "«Adatta» non fa stare la tela nella finestra")
    b.foto("T02_visore_adatta_dopo.png")
    b.chiudi_finestra()
    verifica(abs(p.evaluate("() => window.scrollY") - y0) <= 1, "chiuso il visore la pagina è scorsa")


@prova("T03", "«Apri» della TIF LZW nella Sorgente: il visore della pagina, non una scheda nuova")
def t03(b):
    p = b.page
    aid = b.al[LZW]
    b.file(aid)
    pagine = len(p.context.pages)
    p.click("#du-sg-dettaglio #du-apri-" + sid(aid))
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    tela = p.locator("#du-visore-tela canvas")
    expect(tela).to_have_count(1, timeout=T_LUNGO)
    expect(tela).to_have_attribute("data-du-tif", "800x566")
    n = scuri(p, "#du-visore-tela canvas", tela=True)
    verifica(n > 500, "la pagina LZW del visore è bianca (%d pixel scuri)" % n)
    verifica(len(p.context.pages) == pagine, "«Apri» ha aperto una scheda nuova")
    b.foto("T03_visore_lzw_dopo.png")
    b.chiudi_finestra()


@prova("T04", "la TIF rotta nel visore: «anteprima non disponibile: formato TIFF non supportato (…)», la pagina resta in piedi")
def t04(b):
    p = b.page
    b.passo("revisione")
    scegli_scheda(b, "k-9")
    grande = p.locator("#du-nodo-2d")
    expect(grande).to_have_attribute("data-du-stato", "errore", timeout=T_LUNGO)
    expect(grande).to_contain_text("anteprima non disponibile: formato TIFF non supportato (")
    grande.click()
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    err = p.locator("#du-visore-errore")
    expect(err).to_contain_text("anteprima non disponibile: formato TIFF non supportato (", timeout=T_LUNGO)
    b.foto("T04_visore_rotta_dopo.png")
    b.chiudi_finestra()
    # la Deflate dalla Sorgente: il nome della compressione
    aid = b.al[DEFLATE]
    b.file(aid)
    p.click("#du-sg-dettaglio #du-apri-" + sid(aid))
    expect(p.locator("#du-visore-errore")).to_contain_text("formato TIFF non supportato (Deflate)", timeout=T_LUNGO)
    b.chiudi_finestra()
    expect(p.locator("#du-passo-sorgente")).to_be_visible()


@prova("T05", "le richieste: i byte delle TIF dal ponte (S5 con Accept JSON), niente fuori dagli statici e dalle rotte della pagina")
def t05(b):
    s5 = re.compile(r"^/thread/" + re.escape(b.thread) + r"/sorgente/file/(" + du.UUID + r")/contenuto$")
    tif = {b.al[n] for n in NODI.values()}
    viste = set()
    for r in b.richieste:
        m = s5.match(r["path"])
        if m and m.group(1) in tif:
            viste.add(m.group(1))
            verifica(r["metodo"] == "GET" and "modo=anteprima" in r["query"], "i byte della TIF non vengono da S5 in anteprima: %s" % r)
            verifica(r["headers"].get("accept") == "application/json", "la richiesta dei byte della TIF non passa dal ponte (Accept %r)" % r["headers"].get("accept"))
    verifica(viste == tif, "S5 non è stata chiamata per tutte le TIF: mancano %s" % sorted(tif - viste))
    ammesse = re.compile(r"^(/static/[A-Za-z0-9_.\-/]+|/thread/" + re.escape(b.thread) + r"/.*|/favicon\.ico|/stato/worker)$")
    fuori = sorted({(r["metodo"], r["path"]) for r in b.richieste if not ammesse.match(r["path"])})
    verifica(not fuori, "richieste fuori dalla pagina: %s" % fuori)
    statici = {r["path"] for r in b.richieste if r["path"].startswith("/static/")}
    verifica("/static/distinta_unica_tif.js" in statici, "il worker delle TIF non è stato chiesto agli statici: %s" % sorted(statici))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    scelte = {x.strip().upper() for x in a.prove.split(",") if x.strip()}
    expect.set_options(timeout=T_BREVE)
    falliti = []
    fatte = 0
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(viewport={"width": 1440, "height": 900}, locale="it-IT")
        page = ctx.new_page()
        page.set_default_timeout(T_LUNGO)
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        page.on("dialog", lambda d: (errori.append("dialogo inatteso: " + d.message), d.dismiss()))
        b = du.Banco(page, a)
        for numero, nome, fn in PROVE:
            if scelte and numero not in scelte:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  DU-%s  %-100s ESEGUITA PASSATA  (%.1fs)" % (numero, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(numero)
                testo = str(e)
                print("  DU-%s  %-100s FALLITA  %s" % (numero, nome, testo.splitlines()[0] if testo else repr(e)))
                if "\n" in testo:
                    print("         " + "\n         ".join(testo.splitlines()[1:12]))
                b.foto("errore_%s_dopo.png" % numero)
                try:
                    for _ in range(2):
                        page.keyboard.press("Escape")
                        page.wait_for_timeout(150)
                except Exception:
                    pass
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
