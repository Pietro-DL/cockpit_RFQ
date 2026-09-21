# -*- coding: utf-8 -*-
"""L7 - l'Inbox a quadranti in un BROWSER VERO (checkpoint 7B.5, prove A..G).

Non lo si lancia a mano: lo avvia `inbox_browser_test.go` (tag `browser`), che prima prepara il
banco su PostgreSQL, semina i messaggi nei tre quadranti e mette in piedi il server vero. Qui si
guarda solo cio' che un operatore vedrebbe: quale linguetta e' accesa, quali righe ci sono sotto, e
che cosa dice la barra degli indirizzi.

    python inbox_quadranti.py --url http://127.0.0.1:PORTA --ip 10.0.0.5:51000

Perche' serve un browser vero: le prove L4 chiedono al server un frammento e guardano l'HTML che
torna. Il difetto del 18/09/2026 non stava li' - il server rispondeva giusto - stava nel fatto che
la pagina ne sostituiva soltanto un pezzo, e i comandi restavano quelli di prima. Un difetto che
vive fra due risposte giuste lo vede solo chi guarda la pagina intera.
"""
import argparse
import sys
import time
from urllib.parse import urlparse, parse_qs

from playwright.sync_api import sync_playwright

ESITI = []


def prova(nome):
    def deco(fn):
        ESITI.append((nome, fn))
        return fn
    return deco


class Rotto(AssertionError):
    pass


def verifica(condizione, messaggio):
    if not condizione:
        raise Rotto(messaggio)


# ---------------------------------------------------------------- lettura della pagina

def quadrante_acceso(page):
    """La linguetta accesa, come la vede l'occhio: la classe `attivo` sul link del quadrante."""
    accesi = page.locator(".quadranti a.quadrante.attivo")
    n = accesi.count()
    verifica(n == 1, "linguette accese: %d (ne deve essere accesa una sola)" % n)
    classi = accesi.first.get_attribute("class").split()
    for k in ("clienti", "fornitori", "interni", "altro", "validare", "tutti"):
        if k in classi:
            return k
    raise Rotto("linguetta accesa senza nome: %s" % classi)


def filtro_acceso(page):
    accesi = page.locator(".filtri > a.attivo")
    verifica(accesi.count() == 1, "filtri accesi: %d" % accesi.count())
    return accesi.first.inner_text().split()[0].strip()


def direzione_accesa(page):
    accesi = page.locator(".filtri .direzione a.attivo")
    verifica(accesi.count() == 1, "direzioni accese: %d" % accesi.count())
    t = accesi.first.inner_text().strip()
    return {"↓ entrata": "entrata", "↑ uscita": "uscita", "↕ tutte": ""}.get(t, t)


def oggetti(page):
    return [t.strip() for t in page.locator("#lista a.riga .oggetto").all_inner_texts()]


def controparti(page):
    return [t.strip() for t in page.locator("#lista a.riga .cliente").all_inner_texts()]


def parametri(page):
    q = parse_qs(urlparse(page.url).query)
    return {k: (v[0] if v else "") for k, v in q.items()}


def stabile(page):
    """Aspetta che HTMX abbia finito di sostituire: nessuna richiesta in volo."""
    page.wait_for_function("() => document.querySelectorAll('.htmx-request').length === 0", timeout=5000)
    page.wait_for_timeout(120)


def vai(page, base, percorso):
    page.goto(base + percorso, wait_until="load")
    stabile(page)


def clic(page, selettore):
    page.click(selettore)
    stabile(page)


def coerente(page, atteso_quadrante, dentro, fuori, dove):
    """Il cuore di tutte le prove: cio' che si VEDE acceso e cio' che si LEGGE devono dire lo
    stesso, e la barra degli indirizzi deve dire quello."""
    acceso = quadrante_acceso(page)
    verifica(acceso == atteso_quadrante,
             "%s: la linguetta accesa e' «%s», ma lo stato chiesto e' «%s»" % (dove, acceso, atteso_quadrante))
    p = parametri(page)
    verifica(p.get("q", "") == atteso_quadrante,
             "%s: l'indirizzo dice q=%r, la linguetta dice %r" % (dove, p.get("q", ""), atteso_quadrante))
    testi = oggetti(page)
    for d in dentro:
        verifica(any(d in t for t in testi), "%s: manca la riga %r (ci sono: %s)" % (dove, d, testi))
    for f in fuori:
        verifica(not any(f in t for t in testi), "%s: c'e' la riga %r, che e' di un altro quadrante (ci sono: %s)" % (dove, f, testi))


# ---------------------------------------------------------------- le prove del piano

@prova("A  ?q=validare: Da validare accesa, righe del solo quadrante")
def prova_a(page, base):
    vai(page, base, "/inbox?q=validare&filtro=tutti")
    coerente(page, "validare", ["SCONOSCIUTO UNO", "SCONOSCIUTO DUE"],
             ["CLIENTE ENTRATA", "FORNITORE ENTRATA"], "A")
    for c in controparti(page):
        verifica("sconosciut" in c.lower() or "ambiguo" in c.lower(),
                 "A: in Da validare c'e' una riga con controparte %r" % c)


@prova("B  clic su Fornitori: URL, linguetta e righe cambiano insieme")
def prova_b(page, base):
    vai(page, base, "/inbox?q=validare&filtro=tutti")
    clic(page, ".quadranti a.quadrante.fornitori")
    coerente(page, "fornitori", ["FORNITORE ENTRATA"], ["SCONOSCIUTO UNO", "CLIENTE ENTRATA"], "B")
    classi = page.locator(".quadranti a.quadrante.validare").get_attribute("class")
    verifica("attivo" not in classi.split(), "B: «Da validare» e' rimasta accesa insieme a «Fornitori»")


@prova("C  clic su Buyer: stessa coerenza")
def prova_c(page, base):
    vai(page, base, "/inbox?q=fornitori&filtro=tutti")
    clic(page, ".quadranti a.quadrante.clienti")
    coerente(page, "clienti", ["CLIENTE ENTRATA UNO", "CLIENTE USCITA"], ["FORNITORE ENTRATA", "SCONOSCIUTO UNO"], "C")


@prova("D  direzione entrata/uscita: pillola e righe")
def prova_d(page, base):
    vai(page, base, "/inbox?q=clienti&filtro=tutti")
    clic(page, ".filtri .direzione a:has-text('entrata')")
    verifica(direzione_accesa(page) == "entrata", "D: la pillola accesa non e' «entrata» ma %r" % direzione_accesa(page))
    verifica(parametri(page).get("dir") == "entrata", "D: l'indirizzo non dice dir=entrata: %s" % parametri(page))
    coerente(page, "clienti", ["CLIENTE ENTRATA UNO"], ["CLIENTE USCITA"], "D entrata")
    clic(page, ".filtri .direzione a:has-text('uscita')")
    verifica(direzione_accesa(page) == "uscita", "D: la pillola accesa non e' «uscita» ma %r" % direzione_accesa(page))
    coerente(page, "clienti", ["CLIENTE USCITA"], ["CLIENTE ENTRATA UNO"], "D uscita")


@prova("E  orfani / agganciati / ignorati: pillola e dati")
def prova_e(page, base):
    vai(page, base, "/inbox?q=clienti&filtro=tutti")
    clic(page, ".filtri > a:has-text('orfani')")
    verifica(filtro_acceso(page) == "orfani", "E: filtro acceso %r" % filtro_acceso(page))
    coerente(page, "clienti", ["CLIENTE ENTRATA UNO"], ["CLIENTE AGGANCIATO", "CLIENTE IGNORATO"], "E orfani")
    clic(page, ".filtri > a:has-text('agganciati')")
    verifica(filtro_acceso(page) == "agganciati", "E: filtro acceso %r" % filtro_acceso(page))
    coerente(page, "clienti", ["CLIENTE AGGANCIATO"], ["CLIENTE ENTRATA UNO"], "E agganciati")
    clic(page, ".filtri > a:has-text('ignorati')")
    verifica(filtro_acceso(page) == "ignorati", "E: filtro acceso %r" % filtro_acceso(page))
    coerente(page, "clienti", ["CLIENTE IGNORATO"], ["CLIENTE ENTRATA UNO"], "E ignorati")


@prova("F  dopo un poll: linguetta, filtri, selezione e URL restano")
def prova_f(page, base):
    vai(page, base, "/inbox?q=fornitori&filtro=tutti")
    page.click("#lista a.riga:has-text('FORNITORE ENTRATA')")
    stabile(page)
    prima_url = page.url
    prima_sel = parametri(page).get("sel", "")
    verifica(prima_sel != "", "F: il clic su una riga non ha messo `sel` nell'indirizzo: %s" % prima_url)
    verifica(page.locator("#lista a.riga.sel").count() == 1, "F: la riga scelta non e' evidenziata")
    # il poll e' ogni 15 s: si aspetta che passi davvero, non si simula
    page.wait_for_timeout(17000)
    stabile(page)
    coerente(page, "fornitori", ["FORNITORE ENTRATA"], ["SCONOSCIUTO UNO"], "F dopo il poll")
    verifica(page.url == prima_url, "F: dopo il poll l'indirizzo e' cambiato da solo: %s → %s" % (prima_url, page.url))
    verifica(page.locator("#lista a.riga.sel").count() == 1,
             "F: dopo il poll la riga scelta non e' piu' evidenziata")
    verifica(page.locator("#pannello a.riga, #pannello .messaggio, #pannello h2").count() > 0
             or "FORNITORE ENTRATA" in page.locator("#pannello").inner_text(),
             "F: dopo il poll il pannello del messaggio scelto e' sparito")


@prova("G  indietro / avanti del browser: visuale e lista seguono l'indirizzo")
def prova_g(page, base):
    vai(page, base, "/inbox?q=clienti&filtro=tutti")
    clic(page, ".quadranti a.quadrante.fornitori")
    clic(page, ".quadranti a.quadrante.validare")
    page.go_back()
    stabile(page)
    coerente(page, "fornitori", ["FORNITORE ENTRATA"], ["SCONOSCIUTO UNO"], "G indietro")
    page.go_back()
    stabile(page)
    coerente(page, "clienti", ["CLIENTE ENTRATA UNO"], ["FORNITORE ENTRATA"], "G indietro due")
    page.go_forward()
    stabile(page)
    coerente(page, "fornitori", ["FORNITORE ENTRATA"], ["CLIENTE ENTRATA UNO"], "G avanti")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge", help="browser installato: msedge | chrome")
    ap.add_argument("--vedi", action="store_true", help="finestra visibile, per guardare")
    ap.add_argument("--solo", default="", help="lettere delle prove da eseguire, es. ABF")
    a = ap.parse_args()

    falliti = []
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1400, "height": 900})
        page = ctx.new_page()
        errori_js = []
        page.on("pageerror", lambda e: errori_js.append(str(e)))
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=10000)

        for nome, fn in ESITI:
            if a.solo and nome[0] not in a.solo:
                print("  %-58s SALTATA" % nome)
                continue
            inizio = time.time()
            try:
                fn(page, a.url)
                print("  %-58s PASSATO   (%.1fs)" % (nome, time.time() - inizio))
            except Exception as e:
                falliti.append(nome)
                print("  %-58s FALLITO   %s" % (nome, e))
        if errori_js:
            print("  errori JavaScript nella pagina: %s" % errori_js)
            falliti.append("javascript")
        browser.close()

    print("\n%d prove nel browser: %d passate, %d fallite" % (len(ESITI), len(ESITI) - len(falliti), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
