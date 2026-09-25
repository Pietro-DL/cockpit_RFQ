# -*- coding: utf-8 -*-
"""L7 - la pagina Richieste in un BROWSER VERO.

Non lo si lancia a mano: lo avvia `richieste_browser_test.go` (tag `browser`), che prepara le RFQ su
PostgreSQL, mette PDF veri nello staging e accorcia il poll a due secondi.

Perche' serve un browser. Le prove L4 leggono l'elenco che il server manda. Che un filtro cambi solo
l'elenco e non la pagina, che l'indirizzo nella barra del browser segua i filtri e che «indietro» li
riporti, che il poll non ricarichi le anteprime aperte, che la scheda porti al Fascicolo: questo lo
vede solo un browser.
"""
import argparse
import os
import sys
import time

from playwright.sync_api import sync_playwright

PROVE = []


def prova(lettera, nome):
    def deco(fn):
        PROVE.append((lettera, nome, fn))
        return fn
    return deco


class Rotto(AssertionError):
    pass


def verifica(condizione, messaggio):
    if not condizione:
        raise Rotto(messaggio)


class Banco:
    def __init__(self, page, a):
        self.page, self.a, self.base = page, a, a.url
        self.risposte = []
        page.on("response", lambda r: self.risposte.append((r.status, r.url)))

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)

    def apri(self, percorso="/richieste"):
        self.page.goto(self.base + percorso)
        self.page.wait_for_selector("#elenco")
        # un segno sulla finestra: se la pagina si ricarica, sparisce
        self.page.evaluate("window.__segno = 'stessa pagina'")

    def card(self):
        return self.page.locator("#elenco article.rq-card")

    def aspetta_card(self, n, entro=8000):
        fine = time.time() + entro / 1000
        while time.time() < fine:
            if self.card().count() == n:
                return
            time.sleep(0.1)
        raise Rotto("card %d invece di %d" % (self.card().count(), n))

    def stessa_pagina(self):
        verifica(self.page.evaluate("window.__segno") == "stessa pagina", "la pagina si e' ricaricata")


@prova("A", "la pagina: card delle RFQ aperte, schede con anteprime pigre, niente tabella")
def prova_a(b):
    b.apri()
    b.aspetta_card(3)
    verifica(b.page.locator("table").count() == 0, "c'e' ancora una tabella")
    prima = b.card().first
    verifica("Staffe carrello" in (prima.text_content() or ""), "la prima card non e' la RFQ di ACME (peso 10)")
    for i in range(b.card().count()):
        n = b.card().nth(i).locator("iframe").count()
        verifica(n <= 6, "una card con %d iframe" % n)
    pigri = b.page.locator("iframe[loading=lazy]").count()
    verifica(pigri == b.page.locator("iframe").count() and pigri > 0, "iframe non pigri o assenti: %d" % pigri)
    larga = b.page.evaluate("document.documentElement.scrollWidth > document.documentElement.clientWidth + 1")
    verifica(not larga, "la pagina scorre in orizzontale")
    b.page.wait_for_timeout(1500)
    b.foto("01_richieste.png")


@prova("B", "la ricerca cambia l'elenco senza ricaricare e mette il filtro nell'indirizzo")
def prova_b(b):
    b.apri()
    b.page.fill("#rq-q", "carrello")
    b.aspetta_card(1)
    b.stessa_pagina()
    b.page.wait_for_function("location.search.indexOf('q=carrello') >= 0", timeout=5000)
    verifica("stato=" not in b.page.url and "sort=" not in b.page.url, "indirizzo sporco: %s" % b.page.url)
    b.foto("02_ricerca.png")
    b.page.go_back()
    b.aspetta_card(3)
    verifica("q=" not in b.page.url, "«indietro» non riporta l'indirizzo senza filtro: %s" % b.page.url)
    verifica(b.page.input_value("#rq-q") in ("", "carrello"), "barra in uno stato strano")


@prova("C", "stato, cliente e SLA critico dalla barra")
def prova_c(b):
    b.apri()
    b.page.click(".rq-seg label:has-text('Tutte')")
    b.aspetta_card(4)
    verifica(b.page.locator(".rq-badge.chiusa").count() == 1, "la RFQ chiusa non si vede con «Tutte»")
    b.page.select_option("#rq-cliente", label="GAMMA")
    b.aspetta_card(2)
    b.page.click(".rq-rapido:has-text('SLA critico')")
    b.aspetta_card(1)
    verifica("Carter motore" in (b.card().first.text_content() or ""), "SLA critico non e' la RFQ in FATTIBILITA da cinque giorni")
    b.stessa_pagina()
    url = b.page.url
    for pezzo in ("stato=tutte", "cliente=", "sla=1"):
        verifica(pezzo in url, "manca %s nell'indirizzo %s" % (pezzo, url))
    b.foto("03_filtri.png")
    # l'indirizzo con i filtri, aperto da capo, ridisegna la stessa barra
    b.page.goto(url)
    b.aspetta_card(1)
    verifica(b.page.is_checked(".rq-rapido input[name=sla]"), "la barra ricaricata non ha SLA critico")


@prova("D", "«+ 2 altri» apre tutte le schede, «Mostra meno» le richiude")
def prova_d(b):
    b.apri("/richieste?q=carter+motore")
    b.aspetta_card(1)
    schede = b.card().first.locator(".rq-prod-link")
    verifica(schede.count() == 6, "schede %d" % schede.count())
    b.page.click("button.rq-piu:has-text('altri')")
    b.page.wait_for_function("document.querySelectorAll('.rq-prod-link').length == 8", timeout=5000)
    b.page.wait_for_timeout(1500)  # le anteprime appena arrivate si disegnano: la foto le aspetta
    b.foto("04_tutte_le_schede.png")
    b.page.click("button.rq-piu:has-text('Mostra meno')")
    b.page.wait_for_function("document.querySelectorAll('.rq-prod-link').length == 6", timeout=5000)
    b.stessa_pagina()


@prova("E", "il poll non ricarica le anteprime quando niente e' cambiato")
def prova_e(b):
    b.apri()
    b.aspetta_card(3)
    b.page.wait_for_timeout(1500)
    b.page.evaluate("document.querySelector('#elenco iframe').__segno = 'stesso iframe'")
    anteprime_prima = len([u for s, u in b.risposte if "/anteprima" in u])
    b.risposte.clear()
    b.page.wait_for_timeout(5500)  # piu' di due giri di poll a 2 s
    poll = [(s, u) for s, u in b.risposte if "/richieste?" in u and "firma=" in u]
    verifica(len(poll) >= 2, "giri di poll visti: %d" % len(poll))
    verifica(all(s == 204 for s, _ in poll), "il poll senza cambiamenti non risponde 204: %s" % poll)
    verifica(b.page.evaluate("document.querySelector('#elenco iframe').__segno") == "stesso iframe",
             "l'iframe e' stato rifatto: l'anteprima si e' ricaricata")
    verifica(not [u for s, u in b.risposte if "/anteprima" in u], "il poll ha richiesto di nuovo le anteprime")
    verifica(anteprime_prima >= 1, "nessuna anteprima caricata all'apertura")


@prova("F", "la scheda porta al Fascicolo, sul componente")
def prova_f(b):
    b.apri("/richieste?q=carrello")
    b.aspetta_card(1)
    link = b.page.locator(".rq-prod-link").first
    href = link.get_attribute("href")
    verifica("/fascicolo?nodo=" in href, "la prima scheda non apre il componente: %s" % href)
    b.page.locator(".rq-prod").first.click(position={"x": 60, "y": 50})  # sull'anteprima: il clic passa alla scheda
    b.page.wait_for_url("**/fascicolo?nodo=*", timeout=10000)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="ABCDEF")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    falliti = []
    fatte = 0
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        page.on("response", lambda r: errori.append("risposta %d: %s" % (r.status, r.url)) if r.status >= 400 and not r.url.endswith("/favicon.ico") else None)
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=10000)
        b = Banco(page, a)
        for lettera, nome, fn in PROVE:
            if lettera not in a.prove:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  %s  %-72s PASSATO   (%.1fs)" % (lettera, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(lettera)
                print("  %s  %-72s FALLITO   %s" % (lettera, nome, e))
                b.foto("errore_%s.png" % lettera)
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
