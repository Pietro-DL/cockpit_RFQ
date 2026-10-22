# -*- coding: utf-8 -*-
"""Browser, richiesta del 22/10 (frontend): il browser ad albero della Revisione, come quello degli assiemi di SolidWorks o
di Fusion 360, prima dell'organigramma (in distinta_unica.mjs, blocco «browser ad albero»).

Lo lancia TestDistintaUnicaBrowserAlberoNelBrowser (distinta_unica_browser_albero_browser_test.go, tag `integrazione
browser`) sul banco finto di distinta_unica_banco_test.go, con `python -I`. Riusa gli aiuti di distinta_unica.py (la
classe Banco).

Che cosa prova:
  BR0  senza una scelta nella pagina il browser parte richiuso (una linguetta nell'angolo della tela) nelle finestre alte meno di
       1200 px, aperto da lì in su (seconda richiesta del 22/10: il browser sta sopra l'organigramma)
  BR1  le righe annidate: il prodotto, i suoi figli, sotto i pezzi i loro documenti come foglie (PDF, STEP…), con
       l'icona del tipo, il codice, la quantità del legame e lo stato; ruoli ARIA tree/treeitem con aria-level e
       aria-expanded; nessuna chiave interna (k-…), UUID o nome tecnico a schermo
  BR2  apri/chiudi con il triangolo e con le frecce, «Espandi tutto» e «Comprimi tutto», senza ridisegnare la pagina
       (il nodo della tela resta lo stesso elemento)
  BR3  il filtro tiene le righe trovate e i loro antenati, «Nessun pezzo trovato» senza risultati, Esc lo svuota
  BR4  il clic su una riga sceglie il pezzo come la tela: il pannello del nodo lo mostra, la riga è evidenziata, la
       pagina non si sposta (scrollY uguale ±1 px), la tela non si muove, il riquadro del browser non torna in cima e il
       fuoco resta sulla riga; il doppio clic centra il pezzo sulla tela
  BR5  il pannello si richiude e si riapre; la scelta resta passando a Sorgente e tornando (la pagina si ridisegna),
       e dopo un ricaricamento si riparte dalla scelta di partenza (la pagina non usa la memoria del browser)
  BR6  le schermate a 1600 × 1000 e 1366 × 768: il browser e la tela stanno nella finestra, nessuno scorrimento
       orizzontale della pagina
  BR7  le schermate dell'insieme a 1600 × 1000 e 1366 × 768 (Sorgente, anteprima, Revisione con il browser, aiuto «(?)»,
       guida al passo del browser, elenco, wizard), senza scorrimento orizzontale
  BR8  (seconda richiesta del 22/10 [U]) il browser sta SOPRA la tela e largo quanto lei; a 1600 × 1000 l'organigramma si
       apre intero; aprirlo e chiuderlo non fa saltare la pagina (±1 px) e la vista della tela si riadatta
  BR9  «Mostra tutto» (il pulsante che era «Adatta», e il tasto 0): l'albero intero e al centro della tela
  BR10 lo schermo intero: tutta la finestra, «Cerca» che evidenzia e centra (Invio al seguente), il clic su una scheda
       che la sceglie ed evidenzia padri, figli e documenti, Esc che rimette la pagina dov'era; nessuna richiesta

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
    # sotto i 1200 px d'altezza, senza una scelta nella pagina, il browser parte richiuso (la linguetta): lo si apre
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


@prova("BR0", "senza una scelta nella pagina: richiuso (una linguetta) sotto i 1200 px d'altezza, aperto da lì in su")
def br0(b):
    p = b.page
    p.evaluate("() => { try { sessionStorage.removeItem('du-br-chiuso'); } catch (e) {} }")
    # il browser sta sopra la tela (seconda richiesta del 22/10): conta l'altezza della finestra, non la larghezza
    for w, hh, atteso in ((1600, 1000, "false"), (1440, 900, "false"), (1600, 1200, "true")):
        p.set_viewport_size({"width": w, "height": hh})
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


@prova("BR5", "il pannello si richiude e si riapre; la scelta resta fra Sorgente e Revisione; ricaricando si riparte da capo")
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
    b.passo("sorgente")
    b.passo("revisione")
    expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "false", timeout=T_LUNGO)
    expect(p.locator("#du-br-corpo")).to_be_hidden(timeout=T_BREVE)
    # aperto, resta aperto fra le schede; ricaricando, si riparte dalla scelta di partenza (richiuso sotto i 1200 px d'altezza)
    p.click("#du-br-chiudi")
    b.passo("sorgente")
    b.passo("revisione")
    expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "true", timeout=T_LUNGO)
    p.reload()
    b.pronta()
    b.passo("revisione")
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
            return { sw: document.documentElement.scrollWidth, iw: innerWidth, a: [a.left, a.right, a.width, a.height, a.bottom], v: [v.left, v.right, v.width, v.top] }; }""")
        print("      %dx%d: %r" % (w, hh, m))
        verifica(m["sw"] <= m["iw"] + 1, "scorrimento orizzontale a %dx%d: %r" % (w, hh, m))
        verifica(m["a"][2] >= 240 and m["v"][2] >= 360, "browser o tela troppo stretti a %dx%d: %r" % (w, hh, m))
        verifica(m["v"][1] <= m["iw"] + 1, "la tela esce dalla finestra a %dx%d" % (w, hh))
        # il browser sta sopra la tela, largo quanto lei (seconda richiesta del 22/10)
        verifica(abs(m["a"][0] - m["v"][0]) <= 1 and abs(m["a"][2] - m["v"][2]) <= 1, "il browser non è largo quanto la tela a %dx%d: %r" % (w, hh, m))
        b.foto("BR6_%dx%d.png" % (w, hh))
    p.set_viewport_size({"width": 1440, "height": 900})



def senza_scorrimento_orizzontale(p, cosa):
    m = p.evaluate("() => [document.documentElement.scrollWidth, innerWidth]")
    verifica(m[0] <= m[1] + 1, "scorrimento orizzontale in %s: %r" % (cosa, m))


@prova("BR7", "l'insieme a 1600 × 1000 e 1366 × 768: Sorgente, anteprima, Revisione con il browser, elenco, aiuto «(?)», guida, wizard")
def br7(b):
    """Le schermate dell'insieme (integrazione frontend del 22/10) per chi rivede il lavoro: con --foto le salva; in ogni
    vista controlla che la pagina non scorra di lato."""
    p = b.page
    for w, hh in ((1600, 1000), (1366, 768)):
        p.set_viewport_size({"width": w, "height": hh})
        n = "%dx%d" % (w, hh)
        revisione(b)
        b.passo("sorgente")
        p.wait_for_timeout(400)
        senza_scorrimento_orizzontale(p, "Sorgente " + n)
        b.foto("BR7_%s_1_sorgente.png" % n)
        b.guarda(b.al["ACM-1000_assieme.stp"])
        p.wait_for_timeout(800)
        senza_scorrimento_orizzontale(p, "anteprima " + n)
        b.foto("BR7_%s_2_anteprima.png" % n)
        revisione(b)
        p.click("#du-br-espandi")
        r = p.locator("#du-br-albero .du-br-riga--pezzo[aria-level='2']").first
        r.click()
        p.wait_for_timeout(500)
        senza_scorrimento_orizzontale(p, "Revisione " + n)
        b.foto("BR7_%s_3_revisione_browser.png" % n)
        ic = p.locator('.du-aiuto[data-aiuto="br-filtro"]')
        if ic.count() and ic.is_visible():
            ic.hover()
            expect(p.locator("#du-aiuto-fumetto")).to_be_visible(timeout=T_BREVE)
            b.foto("BR7_%s_4_aiuto.png" % n)
            p.mouse.move(5, hh - 5)
        p.click("#du-guida-apri")
        expect(p.locator(".du-guida")).to_be_visible(timeout=T_BREVE)
        for _ in range(8):
            if "browser" in p.locator(".du-guida-t").inner_text().lower():
                break
            p.keyboard.press("ArrowRight")
            p.wait_for_timeout(250)
        b.foto("BR7_%s_5_guida_browser.png" % n)
        p.keyboard.press("Escape")
        expect(p.locator(".du-guida")).to_have_count(0, timeout=T_BREVE)
        b.vista("elenco")
        p.wait_for_timeout(400)
        senza_scorrimento_orizzontale(p, "elenco " + n)
        b.foto("BR7_%s_6_elenco.png" % n)
        b.vista("org")
        b.apri_wizard()
        p.wait_for_timeout(400)
        b.foto("BR7_%s_7_wizard.png" % n)
        p.keyboard.press("Escape")
        p.wait_for_timeout(300)
    p.set_viewport_size({"width": 1440, "height": 900})
    b.apri()


# ---- seconda richiesta del 22/10 [U] (Stefano): il browser SOPRA l'organigramma, «Mostra tutto» e lo schermo intero

TELA = r"""() => { const v = document.getElementById('du-vp'); if (!v) return null; const R = v.getBoundingClientRect();
  const alto = Math.max(R.top, 0), basso = Math.min(R.bottom, innerHeight);
  let fuori = [], fuoriVisibile = [], l = 1e9, r = -1e9;
  for (const c of v.querySelectorAll('.du-nc[data-k]')) { const q = c.getBoundingClientRect();
    l = Math.min(l, q.left); r = Math.max(r, q.right);
    if (q.left < R.left - 1 || q.right > R.right + 1 || q.top < R.top - 1 || q.bottom > R.bottom + 1) fuori.push(c.dataset.k);
    else if (q.top < alto - 1 || q.bottom > basso + 1) fuoriVisibile.push(c.dataset.k); }
  const b = document.getElementById('du-br'), s = document.getElementById('du-br-chiudi');
  const B = b ? b.getBoundingClientRect() : null, Sx = s ? s.getBoundingClientRect() : null;
  const m = document.getElementById('du-vp-mondo');
  return { vp: [R.left, R.top, R.width, R.height, R.right, R.bottom], n: v.querySelectorAll('.du-nc[data-k]').length, fuori, fuoriVisibile,
    margini: [l - R.left, R.right - r], scala: v.dataset.scala, sy: window.scrollY, pos: getComputedStyle(v).position,
    br: B ? [B.left, B.top, B.width, B.height, B.right, B.bottom] : null, striscia: Sx ? Sx.top : null, mondo: m ? m.style.transform : '' }; }"""


def tela(p):
    return p.evaluate(TELA)


def revisione_da_capo(b, w, hh):
    """La Revisione a w × hh come la apre chi arriva: la pagina ricaricata (nessuna scelta sul browser), con la bozza."""
    p = b.page
    p.set_viewport_size({"width": w, "height": hh})
    revisione(b)
    b.apri()
    b.passo("revisione")
    expect(p.locator("#du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    p.evaluate("window.scrollTo(0, 0)")
    p.wait_for_timeout(500)


@prova("BR8", "il browser sta SOPRA l'organigramma, largo quanto la tela; a 1600 × 1000 l'albero si apre intero; aprire e chiudere non fa saltare la pagina e la vista si riadatta")
def br8(b):
    p = b.page
    try:
        revisione_da_capo(b, 1600, 1000)
        expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "false")
        m0 = tela(p)
        print("      1600x1000 all'apertura: %r" % {k: m0[k] for k in ("vp", "scala", "fuori", "fuoriVisibile", "br")})
        verifica(m0["n"] >= 6 and not m0["fuori"] and not m0["fuoriVisibile"], "a 1600 × 1000 l'organigramma non si apre intero: %r" % m0)
        # richiuso, il browser è una linguetta nell'angolo in alto a sinistra della tela: non le toglie altezza
        verifica(abs(m0["br"][0] - m0["vp"][0]) <= 1 and abs(m0["br"][1] - m0["vp"][1]) <= 1 and m0["br"][3] <= 40 and m0["br"][2] < m0["vp"][2] / 2,
                 "la linguetta del browser non sta nell'angolo della tela: %r" % m0)
        # aperto: sopra la tela, largo quanto lei; la pagina e la striscia non si spostano, la tela non cambia larghezza
        p.click("#du-br-chiudi")
        expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "true", timeout=T_BREVE)
        expect(p.locator("#du-br-albero .du-br-riga").first).to_be_visible(timeout=T_BREVE)
        p.wait_for_timeout(300)
        m1 = tela(p)
        print("      aperto: %r" % {k: m1[k] for k in ("vp", "scala", "br", "sy")})
        verifica(m1["br"][3] >= 200, "il browser aperto è troppo basso: %r" % m1["br"])
        verifica(m1["br"][5] <= m1["vp"][1] + 1, "il browser aperto non sta sopra la tela: %r / %r" % (m1["br"], m1["vp"]))
        verifica(abs(m1["br"][0] - m1["vp"][0]) <= 1 and abs(m1["br"][2] - m1["vp"][2]) <= 1, "il browser non è largo quanto la tela: %r / %r" % (m1["br"], m1["vp"]))
        verifica(abs(m1["vp"][2] - m0["vp"][2]) <= 1, "aprendo il browser la tela ha cambiato larghezza: %r → %r" % (m0["vp"][2], m1["vp"][2]))
        verifica(abs(m1["sy"] - m0["sy"]) <= 1 and abs(m1["striscia"] - m0["striscia"]) <= 1, "aprendo il browser la pagina è saltata: %r → %r" % ((m0["sy"], m0["striscia"]), (m1["sy"], m1["striscia"])))
        verifica(abs(m1["br"][1] - m0["vp"][1]) <= 1, "il browser aperto non comincia dove cominciava la tela: %r / %r" % (m1["br"], m0["vp"]))
        verifica(not m1["fuori"], "aperto il browser, schede fuori dalla tela: %r" % m1["fuori"])
        senza_scorrimento_orizzontale(p, "browser sopra 1600")
        b.foto("BR8_1600x1000_aperto.png")
        # richiuso: di nuovo la vista di partenza (riadattata), niente salti
        p.click("#du-br-chiudi")
        expect(p.locator("#du-br-chiudi")).to_have_attribute("aria-expanded", "false", timeout=T_BREVE)
        p.wait_for_timeout(300)
        m2 = tela(p)
        verifica(abs(m2["sy"] - m0["sy"]) <= 1 and abs(m2["striscia"] - m0["striscia"]) <= 1 and abs(m2["vp"][1] - m0["vp"][1]) <= 1,
                 "richiudendo il browser la pagina è saltata: %r → %r" % (m0, m2))
        verifica(not m2["fuori"] and not m2["fuoriVisibile"] and abs(int(m2["scala"]) - int(m0["scala"])) <= 1,
                 "richiuso il browser la vista non è tornata intera: %r (all'apertura %r)" % (m2, m0))
        # a 1366 × 768: aperto, sopra e largo quanto la tela, niente scorrimento di lato
        revisione_da_capo(b, 1366, 768)
        p.click("#du-br-chiudi")
        expect(p.locator("#du-br-corpo")).to_be_visible(timeout=T_BREVE)
        p.wait_for_timeout(300)
        m3 = tela(p)
        verifica(m3["br"][5] <= m3["vp"][1] + 1 and abs(m3["br"][2] - m3["vp"][2]) <= 1, "a 1366 × 768 il browser non sta sopra la tela: %r" % m3)
        verifica(not m3["fuori"], "a 1366 × 768, aperto il browser, schede fuori dalla tela: %r" % m3["fuori"])
        senza_scorrimento_orizzontale(p, "browser sopra 1366")
        b.foto("BR8_1366x768_aperto.png")
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("BR9", "«Mostra tutto»: dopo uno spostamento e uno zoom, l'albero torna intero e al centro della tela (anche col tasto 0), senza scorrere la pagina")
def br9(b):
    p = b.page
    try:
        revisione_da_capo(b, 1600, 1000)
        expect(p.locator("#du-zoom-adatta")).to_have_text("Mostra tutto")
        v = p.locator("#du-vp").bounding_box()
        p.mouse.move(v["x"] + 40, v["y"] + 40)
        p.mouse.down()
        p.mouse.move(v["x"] + 340, v["y"] + 240, steps=6)
        p.mouse.up()
        p.click("#du-zoom-piu")
        p.click("#du-zoom-piu")
        p.wait_for_timeout(300)
        m0 = tela(p)
        verifica(m0["fuori"], "lo spostamento non ha portato fuori nessuna scheda: la prova non prova niente")
        p.click("#du-zoom-adatta")
        p.wait_for_timeout(300)
        m1 = tela(p)
        print("      Mostra tutto: %r" % {k: m1[k] for k in ("scala", "fuori", "fuoriVisibile", "margini")})
        verifica(not m1["fuori"] and not m1["fuoriVisibile"], "«Mostra tutto» lascia schede fuori: %r" % m1)
        verifica(abs(m1["margini"][0] - m1["margini"][1]) <= 4, "«Mostra tutto» non centra l'albero: margini %r" % m1["margini"])
        verifica(abs(m1["sy"] - m0["sy"]) <= 1, "«Mostra tutto» ha fatto scorrere la pagina")
        # il tasto 0 sulla tela fa lo stesso
        p.click("#du-zoom-piu")
        # il tasto 0 con la tela a fuoco (senza far scorrere la pagina, come in C3)
        p.locator("#du-vp").evaluate("(v) => { v.focus({ preventScroll: true }); v.dispatchEvent(new KeyboardEvent('keydown', { key: '0', bubbles: true })); }")
        p.wait_for_timeout(200)
        m2 = tela(p)
        verifica(m2["mondo"] == m1["mondo"], "il tasto 0 non dà la stessa vista di «Mostra tutto»: %r / %r" % (m2["mondo"], m1["mondo"]))
        # e col browser aperto sopra: tutte le schede dentro la tela
        p.click("#du-br-chiudi")
        p.wait_for_timeout(300)
        p.click("#du-zoom-adatta")
        p.wait_for_timeout(300)
        m3 = tela(p)
        verifica(not m3["fuori"], "col browser aperto «Mostra tutto» lascia schede fuori: %r" % m3["fuori"])
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("BR10", "schermo intero: si apre sopra la pagina, cerca e centra, evidenzia padri/figli/documenti, il clic sceglie il pezzo, Esc torna esattamente dov'era; nessuna richiesta")
def br10(b):
    p = b.page
    try:
        revisione_da_capo(b, 1600, 1000)
        # una scelta di partenza e la pagina scorsa un poco
        p.locator("#du-vp .du-nc[data-k]").nth(2).click()
        p.wait_for_timeout(300)
        k0 = p.evaluate("() => (document.querySelector('#du-vp .du-nc--scelto') || {}).dataset.k")
        verifica(k0, "nessuna scheda scelta per la partenza")
        p.evaluate("window.scrollTo(0, 140)")
        p.wait_for_timeout(300)
        m0 = tela(p)
        i = b.segna()
        # Esc senza fare niente: tutto come prima
        p.click("#du-zoom-schermo")
        expect(p.locator("#du-fs-barra")).to_be_visible(timeout=T_BREVE)
        p.wait_for_timeout(300)
        p.keyboard.press("Escape")
        expect(p.locator("#du-fs-barra")).to_have_count(0, timeout=T_BREVE)
        p.wait_for_timeout(200)
        m1 = tela(p)
        verifica(abs(m1["sy"] - m0["sy"]) <= 1 and abs(m1["vp"][1] - m0["vp"][1]) <= 1 and m1["mondo"] == m0["mondo"] and m1["pos"] != "fixed",
                 "Esc non ha rimesso la pagina dov'era: %r → %r" % (m0, m1))
        verifica(p.evaluate("() => (document.querySelector('#du-vp .du-nc--scelto') || {}).dataset.k") == k0, "Esc ha cambiato la scelta")
        verifica(p.evaluate("() => document.activeElement && document.activeElement.id") == "du-zoom-schermo", "il fuoco non torna su «Schermo intero»")
        # di nuovo, e questa volta si cerca e si sceglie
        p.click("#du-zoom-schermo")
        expect(p.locator("#du-fs-barra")).to_be_visible(timeout=T_BREVE)
        p.wait_for_timeout(300)
        f = tela(p)
        vw, vh = p.viewport_size["width"], p.viewport_size["height"]
        print("      schermo intero: %r" % {k: f[k] for k in ("vp", "scala", "fuori", "pos")})
        verifica(f["pos"] == "fixed" and f["vp"][0] <= 1 and abs(f["vp"][2] - vw) <= 2 and abs(f["vp"][5] - vh) <= 2 and f["vp"][1] <= 80,
                 "la tela non è a tutta finestra: %r" % f["vp"])
        verifica(not f["fuori"], "all'apertura dello schermo intero l'albero non è intero: %r" % f["fuori"])
        verifica(p.evaluate("() => document.activeElement && document.activeElement.id") == "du-fs-cerca", "il fuoco non è sulla ricerca")
        # la ricerca: evidenzia e centra; Invio va al seguente
        p.fill("#du-fs-cerca", "ACM-11")
        expect(p.locator("#du-fs-conto")).to_have_text(re.compile(r"^1 di [2-9]$"), timeout=T_BREVE)
        tro = p.evaluate("""() => { const c = document.querySelector('#du-vp .du-nc--trovato-att'), v = document.getElementById('du-vp').getBoundingClientRect();
            if (!c) return null; const q = c.getBoundingClientRect(); return { k: c.dataset.k, dx: q.left + q.width / 2 - (v.left + v.width / 2), dy: q.top + q.height / 2 - (v.top + v.height / 2),
            n: document.querySelectorAll('#du-vp .du-nc--trovato').length }; }""")
        verifica(tro and abs(tro["dx"]) <= 30 and abs(tro["dy"]) <= 30 and tro["n"] >= 2, "il pezzo trovato non è evidenziato e al centro: %r" % tro)
        b.foto("BR10_cerca.png")
        p.keyboard.press("Enter")
        expect(p.locator("#du-fs-conto")).to_have_text(re.compile(r"^2 di "), timeout=T_BREVE)
        k2 = p.evaluate("() => document.querySelector('#du-vp .du-nc--trovato-att').dataset.k")
        verifica(k2 != tro["k"], "Invio non passa al risultato seguente")
        p.fill("#du-fs-cerca", "zzzz")
        expect(p.locator("#du-fs-conto")).to_have_text("Nessun pezzo trovato", timeout=T_BREVE)
        p.fill("#du-fs-cerca", "")
        expect(p.locator("#du-fs-conto")).to_have_text("", timeout=T_BREVE)
        # «Mostra tutto» dello schermo intero: di nuovo l'albero intero
        p.click("#du-fs-tutto")
        p.wait_for_timeout(200)
        verifica(not tela(p)["fuori"], "«Mostra tutto» nello schermo intero lascia schede fuori")
        # il clic su una scheda con padre, figli e documenti: la sceglie ed evidenzia i collegamenti
        kc = p.evaluate("""() => { for (const c of document.querySelectorAll('#du-vp .du-nc[data-k]')) if ((c.innerText || '').indexOf('ACM-1400') >= 0) return c.dataset.k; return null; }""")
        verifica(kc, "la scheda di ACM-1400 non c'è")
        p.locator("#nc-" + sid(kc)).click(position={"x": 30, "y": 60})
        expect(p.locator("#du-fs-coll")).to_be_visible(timeout=T_BREVE)
        # l'attenuazione ha una transizione d'opacità (sobria, 120-180 ms): si misura a transizione finita
        p.wait_for_timeout(450)
        testo = p.locator("#du-fs-coll").inner_text()
        print("      collegamenti: %s" % testo.replace("\n", " | "))
        verifica("ACM-1400" in testo and re.search(r"Padri \(1\)", testo, re.I) and re.search(r"Figli \([1-9]", testo, re.I) and re.search(r"Documenti \([1-9]", testo, re.I) and "ACM-1400_A.pdf" in testo,
                 "il riquadro dei collegamenti non dice padri, figli e documenti: %r" % testo)
        ev = p.evaluate("""(k) => { const v = document.getElementById('du-vp'); const st = (e) => +getComputedStyle(e).opacity;
            const coll = Array.from(v.querySelectorAll('.du-nc--coll'), c => c.dataset.k), altri = Array.from(v.querySelectorAll('.du-nc[data-k]:not(.du-nc--coll)'));
            return { fuoco: v.classList.contains('du-vp--fuoco'), coll, scelta: (v.querySelector('.du-nc--fuoco') || {}).dataset.k,
              attenuati: altri.filter(c => st(c) < 0.5).length, altri: altri.length, tratti: v.querySelectorAll('.du-ar--coll').length }; }""", kc)
        verifica(ev["fuoco"] and ev["scelta"] == kc and len(ev["coll"]) >= 3 and ev["tratti"] >= 2 and ev["altri"] >= 1 and ev["attenuati"] == ev["altri"],
                 "l'evidenza dei collegamenti non va: %r" % ev)
        verifica(p.evaluate("() => (document.querySelector('#du-vp .du-nc--scelto') || {}).dataset.k") == kc, "il clic nello schermo intero non sceglie il pezzo come la tela")
        b.foto("BR10_collegamenti.png")
        # un clic su un figlio nel riquadro: lo sceglie e lo centra
        p.locator("#du-fs-figli .du-fs-voce").first.click()
        p.wait_for_timeout(300)
        verifica(p.evaluate("() => (document.querySelector('#du-vp .du-nc--fuoco') || {}).dataset.k") != kc, "il clic su un figlio nel riquadro non lo sceglie")
        # Esc: la pagina torna dov'era (scorrimento, posizione della tela, vista), la scelta resta quella fatta
        kf = p.evaluate("() => (document.querySelector('#du-vp .du-nc--fuoco') || {}).dataset.k")
        p.keyboard.press("Escape")
        expect(p.locator("#du-fs-barra")).to_have_count(0, timeout=T_BREVE)
        p.wait_for_timeout(300)
        m2 = tela(p)
        verifica(abs(m2["sy"] - m0["sy"]) <= 1 and abs(m2["vp"][1] - m0["vp"][1]) <= 1 and abs(m2["striscia"] - m0["striscia"]) <= 1 and m2["pos"] != "fixed",
                 "uscendo dallo schermo intero la pagina non è tornata dov'era: %r → %r" % (m0, m2))
        verifica(p.locator("#du-vp.du-vp--fuoco, #du-vp .du-nc--coll, .du-vp-posto").count() == 0, "dopo l'uscita restano evidenze o segnaposti")
        verifica(p.evaluate("() => (document.querySelector('#du-vp .du-nc--scelto') || {}).dataset.k") == kf, "uscendo si è persa la scelta fatta nello schermo intero")
        richieste = b.richieste_da(i)
        verifica(not richieste, "lo schermo intero ha fatto richieste: %s" % ["%s %s" % (x["metodo"], x["path"]) for x in richieste])
    finally:
        if p.locator("#du-fs-barra").count():
            p.keyboard.press("Escape")
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
