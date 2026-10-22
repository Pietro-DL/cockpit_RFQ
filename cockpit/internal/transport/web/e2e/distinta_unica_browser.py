# -*- coding: utf-8 -*-
"""Browser, richiesta del 22/10 (frontend): il browser ad albero della Revisione, come quello degli assiemi di SolidWorks o
di Fusion 360, prima dell'organigramma (web/static/du_browser.mjs).

Lo lancia TestDistintaUnicaBrowserAlberoNelBrowser (distinta_unica_browser_albero_browser_test.go, tag `integrazione
browser`) sul banco finto di distinta_unica_banco_test.go, con `python -I`. Riusa gli aiuti di distinta_unica.py (la
classe Banco).

Che cosa prova:
  BR0  senza una scelta nella sessione il browser parte richiuso sotto i 1600 px di larghezza, aperto da lì in su
  BR1  le righe annidate: il prodotto, i suoi figli, sotto i pezzi i loro documenti come foglie (PDF, STEP…), con
       l'icona del tipo, il codice, la quantità del legame e lo stato; ruoli ARIA tree/treeitem con aria-level e
       aria-expanded; nessuna chiave interna (k-…), UUID o nome tecnico a schermo
  BR2  apri/chiudi con il triangolo e con le frecce, «Espandi tutto» e «Comprimi tutto», senza ridisegnare la pagina
       (il nodo della tela resta lo stesso elemento)
  BR3  il filtro tiene le righe trovate e i loro antenati, «Nessun pezzo trovato» senza risultati, Esc lo svuota
  BR4  il clic su una riga sceglie il pezzo come la tela: il pannello del nodo lo mostra, la riga è evidenziata, la
       pagina non si sposta (scrollY uguale ±1 px), la tela non si muove, il riquadro del browser non torna in cima e il
       fuoco resta sulla riga; il doppio clic centra il pezzo sulla tela
  BR5  il pannello si richiude e si riapre, e la scelta resta dopo un ricaricamento della pagina (nella sessione)
  BR6  le schermate a 1600 × 1000 e 1366 × 768: il browser e la tela stanno nella finestra, nessuno scorrimento
       orizzontale della pagina

Codici e nomi dei file inventati (ACME, ACM-xxxx), quelli del banco. Le schermate vanno nella cartella --foto.
"""
import argparse
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
UUID_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
SNAKE_RE = re.compile(r"\b[a-z]{2,}_[a-z0-9_]+\b")
CHIAVE_RE = re.compile(r"\bk-\d+\b|\bni:")

PROVE = []


def prova(numero, nome):
    def deco(fn):
        PROVE.append((numero, nome, fn))
        return fn
    return deco


def revisione(b):
    """La Revisione con la bozza dallo STEP del finito: la crea la prima volta (conferma della sorgente), poi la riapre."""
    p = b.page
    b.apri()
    b.passo("revisione")
    p.wait_for_timeout(300)
    if p.locator("#du-vp .du-nc[data-k]").count() == 0:
        a1 = b.al["ACM-1000_assieme.stp"]
        b.guarda(a1)
        p.click("#du-conferma-sorgente")
        expect(b.finestra()).to_be_visible(timeout=T_BREVE)
        b.ok_finestra()
        expect(p.locator("#du-passo-revisione")).to_contain_text("versione 1", timeout=T_LUNGO)
        b.passo("revisione")
        b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    # sotto i 1600 px, senza una scelta nella sessione, il browser parte richiuso (la linguetta): lo si apre
    expect(p.locator("#du-br-chiudi")).to_be_visible(timeout=T_LUNGO)
    if p.locator("#du-br-chiudi").get_attribute("aria-expanded") == "false":
        p.click("#du-br-chiudi")
    expect(p.locator("#du-br-albero .du-br-riga").first).to_be_visible(timeout=T_LUNGO)


def righe(p):
    return p.evaluate("""() => Array.from(document.querySelectorAll('#du-br-albero .du-br-riga'), r => ({
        percorso: r.dataset.percorso, k: r.dataset.k || '', livello: +r.getAttribute('aria-level'), esp: r.getAttribute('aria-expanded'),
        tipo: r.className.replace(/.*du-br-riga--(pezzo|doc|gruppo).*/, '$1'), testo: r.innerText.replace(/\\s+/g, ' ').trim(), sel: r.getAttribute('aria-selected') }))""")


def riga_di(p, percorso):
    return p.locator('#du-br-albero .du-br-riga[data-percorso="%s"]' % percorso)


def testo_browser(p):
    return p.locator("#du-br").inner_text()


@prova("BR0", "senza una scelta nella sessione: richiuso sotto i 1600 px (la tela larga come prima), aperto da 1600 px in su")
def br0(b):
    p = b.page
    p.evaluate("() => { try { sessionStorage.removeItem('du-br-chiuso'); } catch (e) {} }")
    for w, atteso in ((1440, "false"), (1600, "true")):
        p.set_viewport_size({"width": w, "height": 900})
        b.apri()
        b.passo("revisione")
        p.wait_for_timeout(300)
        if p.locator("#du-vp .du-nc[data-k]").count() == 0:
            revisione(b)
            p.evaluate("() => { try { sessionStorage.removeItem('du-br-chiuso'); } catch (e) {} }")
            b.apri()
            b.passo("revisione")
        expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", atteso, timeout=T_LUNGO)
    p.evaluate("() => { try { sessionStorage.removeItem('du-br-chiuso'); } catch (e) {} }")
    p.set_viewport_size({"width": 1440, "height": 900})


@prova("BR1", "le righe annidate: prodotto → figli → documenti, con icona, codice, quantità e stato; ARIA tree; niente chiavi grezze")
def br1(b):
    p = b.page
    revisione(b)
    tutto = righe(p)
    verifica(tutto, "il browser non ha righe")
    radice = tutto[0]
    verifica(radice["livello"] == 1 and radice["esp"] == "true" and radice["tipo"] == "pezzo", "la prima riga non è il prodotto aperto: %r" % radice)
    verifica(p.locator("#du-br-albero[role=tree]").count() == 1, "manca il ruolo tree")
    verifica(p.locator("#du-br-albero [role=treeitem]").count() == len(tutto), "le righe non sono tutte treeitem")
    figli = [r for r in tutto if r["livello"] == 2 and r["tipo"] == "pezzo"]
    verifica(figli, "il prodotto non ha figli nel browser: %r" % tutto[:4])
    verifica(any("×" in r["testo"] for r in figli), "nessuna quantità ×q sui figli: %r" % figli[:3])
    # «Espandi tutto»: compaiono i livelli sotto e i documenti come foglie
    p.click("#du-br-espandi")
    tutto = righe(p)
    livelli = {r["livello"] for r in tutto}
    verifica(max(livelli) >= 3, "nessun sottoassieme aperto sotto il primo livello: livelli %r" % sorted(livelli))
    docs = [r for r in tutto if r["tipo"] == "doc"]
    verifica(docs, "nessun documento come foglia nel browser")
    d = docs[0]
    padre = [r for r in tutto if r["tipo"] == "pezzo" and d["percorso"].startswith(r["percorso"] + "#d")]
    verifica(padre and padre[0]["livello"] == d["livello"] - 1, "il documento %r non sta sotto il suo pezzo" % d)
    verifica(re.search(r"\b(PDF|STEP|DXF)\b", d["testo"]), "il documento non dice il tipo del file: %r" % d)
    print("      %d righe, livelli %s, %d documenti; per esempio %r sotto %r" % (len(tutto), sorted(livelli), len(docs), d["testo"], padre[0]["testo"]))
    # le icone del tipo e lo stato con le parole della pagina
    verifica(p.locator("#du-br .du-br-tipo svg").count() >= len([r for r in tutto if r["tipo"] == "pezzo"]), "manca l'icona del tipo su qualche pezzo")
    verifica(p.locator("#du-br .du-br-st").count() > 0, "nessuno stato sulle righe")
    t = testo_browser(p)
    verifica(not CHIAVE_RE.search(t), "chiavi interne a schermo nel browser: %r" % CHIAVE_RE.findall(t))
    verifica(not UUID_RE.search(t), "UUID a schermo nel browser")
    verifica(not SNAKE_RE.search(t), "nomi tecnici a schermo nel browser: %r" % SNAKE_RE.findall(t))
    # il browser sta prima dell'organigramma (a sinistra o sopra)
    pos = p.evaluate("() => { const a = document.querySelector('#du-br').getBoundingClientRect(), v = document.querySelector('#du-vp').getBoundingClientRect(); return {a: [a.left, a.top, a.right], v: [v.left, v.top]}; }")
    verifica(pos["a"][2] <= pos["v"][0] + 1 or pos["a"][1] < pos["v"][1], "il browser non sta prima dell'organigramma: %r" % pos)
    b.foto("BR1_righe.png")


@prova("BR2", "apri e chiudi: triangolo, frecce destra/sinistra, «Comprimi tutto» ed «Espandi tutto», senza ridisegnare la pagina")
def br2(b):
    p = b.page
    revisione(b)
    p.evaluate("() => { window.__vp = document.querySelector('#du-vp'); }")
    p.click("#du-br-espandi")
    tutto = righe(p)
    ass = [r for r in tutto if r["esp"] == "true" and r["livello"] == 2]
    verifica(ass, "nessun sottoassieme aperto al secondo livello")
    x = ass[0]
    n_prima = len(tutto)
    riga_di(p, x["percorso"]).locator("[data-tri]").click()
    expect(riga_di(p, x["percorso"])).to_have_attribute("aria-expanded", "false", timeout=T_BREVE)
    verifica(len(righe(p)) < n_prima, "chiudere il ramo non toglie righe")
    # la freccia a destra riapre, la sinistra richiude
    riga_di(p, x["percorso"]).focus()
    p.keyboard.press("ArrowRight")
    expect(riga_di(p, x["percorso"])).to_have_attribute("aria-expanded", "true", timeout=T_BREVE)
    p.keyboard.press("ArrowLeft")
    expect(riga_di(p, x["percorso"])).to_have_attribute("aria-expanded", "false", timeout=T_BREVE)
    verifica(p.evaluate("() => document.activeElement && document.activeElement.dataset.percorso") == x["percorso"], "il fuoco non resta sulla riga dopo le frecce")
    # la freccia giù va alla riga dopo
    p.keyboard.press("ArrowDown")
    dopo = p.evaluate("() => document.activeElement && document.activeElement.dataset.percorso")
    verifica(dopo and dopo != x["percorso"], "la freccia giù non sposta il fuoco")
    p.click("#du-br-comprimi")
    verifica(len(righe(p)) == 1 or all(r["livello"] == 1 for r in righe(p)), "«Comprimi tutto» lascia righe sotto il primo livello: %r" % righe(p)[:4])
    p.click("#du-br-espandi")
    verifica(len(righe(p)) >= n_prima, "«Espandi tutto» non riapre tutto")
    verifica(p.evaluate("() => window.__vp === document.querySelector('#du-vp')"), "aprire e chiudere nel browser ha ridisegnato la pagina (la tela è un elemento nuovo)")


@prova("BR3", "il filtro: righe trovate con i loro antenati, «Nessun pezzo trovato», Esc lo svuota")
def br3(b):
    p = b.page
    revisione(b)
    p.click("#du-br-espandi")
    tutto = righe(p)
    profondi = [r for r in tutto if r["tipo"] == "pezzo" and r["livello"] >= 3]
    verifica(profondi, "nessun pezzo al terzo livello per provare il filtro")
    bersaglio = profondi[-1]
    cod = bersaglio["testo"].split(" ")[0]
    f = p.locator("#du-br-filtro")
    f.fill(cod)
    b.attendi(lambda: any(r["percorso"] == bersaglio["percorso"] for r in righe(p)) and len(righe(p)) < len(tutto), "il filtro non riduce le righe")
    trovate = righe(p)
    percorsi = {r["percorso"] for r in trovate}
    # gli antenati del pezzo trovato restano, aperti
    pezzi = bersaglio["percorso"].split(">")
    for i in range(1, len(pezzi)):
        verifica(">".join(pezzi[:i]) in percorsi, "l'antenato %r del pezzo trovato non c'è" % ">".join(pezzi[:i]))
    for r in trovate:
        if r["tipo"] == "pezzo" and r["percorso"] != bersaglio["percorso"] and not bersaglio["percorso"].startswith(r["percorso"] + ">"):
            verifica(cod.lower() in r["testo"].lower() or any(x["percorso"].startswith(r["percorso"] + ">") for x in trovate), "riga che il filtro non dovrebbe tenere: %r" % r)
    expect(p.locator("#du-br-conto")).to_contain_text("trovat", timeout=T_BREVE)
    b.foto("BR3_filtro.png")
    f.fill("zz-nessun-pezzo-zz")
    expect(p.locator("#du-br-vuoto")).to_contain_text("Nessun pezzo trovato", timeout=T_BREVE)
    f.focus()
    p.keyboard.press("Escape")
    expect(f).to_have_value("", timeout=T_BREVE)
    b.attendi(lambda: len(righe(p)) == len(tutto), "Esc non riporta tutte le righe")


@prova("BR4", "il clic su una riga sceglie il pezzo: pannello del nodo, riga evidenziata, pagina e tela ferme, fuoco sulla riga; doppio clic centra")
def br4(b):
    p = b.page
    revisione(b)
    p.click("#du-br-espandi")
    tutto = righe(p)
    pezzi = [r for r in tutto if r["tipo"] == "pezzo" and r["livello"] >= 3]
    verifica(pezzi, "nessun pezzo profondo da scegliere")
    x = pezzi[-1]
    # il browser scorso un po' (se scorre) e la pagina scorsa un po': dopo il clic devono restare dove sono
    p.evaluate("() => { const a = document.querySelector('#du-br-albero'); a.scrollTop = Math.min(60, a.scrollHeight - a.clientHeight); }")
    p.evaluate("() => window.scrollTo(0, Math.min(80, document.documentElement.scrollHeight - innerHeight))")
    p.wait_for_timeout(200)
    riga_di(p, x["percorso"]).scroll_into_view_if_needed()
    prima = p.evaluate("""() => ({ y: scrollY, br: document.querySelector('#du-br-albero').scrollTop, cam: document.querySelector('#du-vp-mondo').style.transform,
        brTop: document.querySelector('#du-br').getBoundingClientRect().top })""")
    riga_di(p, x["percorso"]).locator(".du-br-nome").click()
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", x["k"], timeout=T_BREVE)
    expect(riga_di(p, x["percorso"])).to_have_attribute("aria-selected", "true", timeout=T_BREVE)
    p.wait_for_timeout(400)
    dopo = p.evaluate("""() => ({ y: scrollY, br: document.querySelector('#du-br-albero').scrollTop, cam: document.querySelector('#du-vp-mondo').style.transform,
        brTop: document.querySelector('#du-br').getBoundingClientRect().top, fuoco: document.activeElement && document.activeElement.dataset.percorso })""")
    print("      prima %r, dopo %r" % (prima, dopo))
    verifica(abs(dopo["y"] - prima["y"]) <= 1, "la pagina si è spostata dopo il clic su una riga: %r → %r" % (prima["y"], dopo["y"]))
    verifica(abs(dopo["brTop"] - prima["brTop"]) <= 1, "il browser si è spostato sullo schermo: %r → %r" % (prima["brTop"], dopo["brTop"]))
    verifica(abs(dopo["br"] - prima["br"]) <= 1, "il riquadro del browser è tornato a %r da %r" % (dopo["br"], prima["br"]))
    verifica(dopo["cam"] == prima["cam"], "la tela si è mossa dopo il clic su una riga: %r → %r" % (prima["cam"], dopo["cam"]))
    verifica(dopo["fuoco"] == x["percorso"], "il fuoco non è sulla riga cliccata: %r" % dopo["fuoco"])
    # il pannello mostra quel pezzo: il codice della riga è nel pannello, la scheda della tela è la scelta
    cod = x["testo"].split(" ")[0]
    if cod != "(senza":
        expect(p.locator("#du-pannello")).to_contain_text(cod, timeout=T_BREVE)
    t = p.locator(".du-ws").inner_text()
    verifica(not CHIAVE_RE.search(t), "chiavi interne a schermo dopo il clic: %r" % CHIAVE_RE.findall(t))
    b.foto("BR4_scelta.png")
    # tastiera: freccia su e Invio sceglie e centra la riga sopra
    p.keyboard.press("ArrowUp")
    su = p.evaluate("() => document.activeElement && document.activeElement.dataset.k")
    if su:
        p.keyboard.press("Enter")
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", su, timeout=T_BREVE)
    # il doppio clic centra il pezzo sulla tela (la tela si muove, la pagina no)
    y0 = p.evaluate("() => scrollY")
    cam0 = p.evaluate("() => document.querySelector('#du-vp-mondo').style.transform")
    riga_di(p, x["percorso"]).locator(".du-br-nome").dblclick()
    p.wait_for_timeout(300)
    verifica(abs(p.evaluate("() => scrollY") - y0) <= 1, "il doppio clic ha spostato la pagina")
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", x["k"], timeout=T_BREVE)
    print("      doppio clic: tela %r → %r" % (cam0, p.evaluate("() => document.querySelector('#du-vp-mondo').style.transform")))


@prova("BR5", "il pannello si richiude e si riapre; la scelta resta dopo un ricaricamento (sessione)")
def br5(b):
    p = b.page
    revisione(b)
    w0 = p.evaluate("() => document.querySelector('#du-vp').getBoundingClientRect().width")
    p.click("#du-br-chiudi")
    expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "false", timeout=T_BREVE)
    expect(p.locator("#du-br-corpo")).to_be_hidden(timeout=T_BREVE)
    w1 = p.evaluate("() => document.querySelector('#du-vp').getBoundingClientRect().width")
    verifica(w1 >= w0 - 1, "richiuso il browser, la tela non si allarga: %r → %r" % (w0, w1))
    b.foto("BR5_chiuso.png")
    p.reload()
    b.pronta()
    expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "false", timeout=T_LUNGO)
    p.click("#du-br-chiudi")
    expect(p.locator("#du-br-corpo")).to_be_visible(timeout=T_BREVE)
    expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "true", timeout=T_BREVE)


@prova("BR6", "1600 × 1000 e 1366 × 768: browser e tela nella finestra, niente scorrimento orizzontale")
def br6(b):
    p = b.page
    for w, hh in ((1600, 1000), (1366, 768)):
        p.set_viewport_size({"width": w, "height": hh})
        revisione(b)
        p.click("#du-br-espandi")
        p.wait_for_timeout(300)
        m = p.evaluate("""() => { const a = document.querySelector('#du-br').getBoundingClientRect(), v = document.querySelector('#du-vp').getBoundingClientRect();
            return { sw: document.documentElement.scrollWidth, iw: innerWidth, a: [a.left, a.right, a.width, a.height], v: [v.left, v.right, v.width] }; }""")
        print("      %dx%d: %r" % (w, hh, m))
        verifica(m["sw"] <= m["iw"] + 1, "scorrimento orizzontale a %dx%d: %r" % (w, hh, m))
        verifica(m["a"][2] >= 240 and m["v"][2] >= 360, "browser o tela troppo stretti a %dx%d: %r" % (w, hh, m))
        verifica(m["v"][1] <= m["iw"] + 1, "la tela esce dalla finestra a %dx%d" % (w, hh))
        b.foto("BR6_%dx%d.png" % (w, hh))
    p.set_viewport_size({"width": 1440, "height": 900})


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
                b.foto("errore_%s.png" % numero)
                try:
                    page.keyboard.press("Escape")
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
