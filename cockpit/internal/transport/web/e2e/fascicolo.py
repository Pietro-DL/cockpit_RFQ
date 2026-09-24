# -*- coding: utf-8 -*-
"""L7 - la schermata del Fascicolo in un BROWSER VERO (B8.7).

Non lo si lancia a mano: lo avvia `fascicolo_browser_test.go` (tag `browser`), che prepara il banco su
PostgreSQL (una RFQ con una BOM, uno STEP con un nodo nuovo, PDF veri in staging) e mette in piedi il
server vero.

Le prove (lettere, scelte dal test Go con --prove):
  A  la pagina si apre con i tre pannelli e la barra, senza scorrimento orizzontale
  B  un clic sul nodo filtra i documenti, senza ricaricare la pagina
  C  un clic sul file apre il PDF nel pannello Anteprima
  D  accettare una proposta cambia albero e completezza senza F5 e senza chiudere il PDF
  E  tre file assegnati al nodo con un gesto
  F  correggere la struttura (il tipo) cambia la completezza
  G  cento file: la pagina in meno di un secondo, il filtro, niente scorrimento orizzontale
  H  griglia, cassetti Codici e Avvisi, riepilogo di uno STEP (e le fotografie)

«Senza F5» si verifica con due segni messi da JavaScript: uno sulla finestra, uno sull'iframe del PDF.
Se la pagina si ricaricasse, o l'iframe venisse sostituito, i segni sparirebbero.
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
        self.page, self.a = page, a
        self.base = a.url + "/thread/" + a.thread + "/fascicolo"

    def apri(self, stato=""):
        self.page.goto(self.base + stato)
        self.page.wait_for_load_state("domcontentloaded")
        self.page.evaluate("window.__marca = 'viva'")

    def viva(self):
        return self.page.evaluate("window.__marca") == "viva"

    def clic_e_aspetta(self, locator, pezzo_url):
        with self.page.expect_response(lambda r: pezzo_url in r.url, timeout=15000) as risp:
            locator.click()
        verifica(risp.value.status == 200, "risposta %d da %s" % (risp.value.status, risp.value.url))
        self.page.wait_for_timeout(250)  # htmx: swap e settle
        return risp.value

    def avviso(self):
        el = self.page.locator("#fasc-avviso .avviso-f")
        return el.inner_text().strip() if el.count() else ""

    def niente_orizzontale(self):
        largo = self.page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
        verifica(largo <= 1, "la pagina scorre in orizzontale di %d px" % largo)
        for pan in ["#struttura", "#documenti"]:
            d = self.page.evaluate("(() => { const e = document.querySelector('%s').parentElement; return e.scrollWidth - e.clientWidth })()" % pan)
            verifica(d <= 1, "il pannello %s scorre in orizzontale di %d px" % (pan, d))

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)


@prova("A", "la pagina si apre con i pannelli e la barra, niente scorrimento orizzontale")
def prova_a(b):
    b.apri()
    for sel in ["#fasc-testata", "#struttura", "#documenti", "#anteprima", "#completezza"]:
        verifica(b.page.locator(sel).is_visible(), "%s non si vede" % sel)
    nodo = b.page.locator('[id="proposta-%s-#3"]' % b.a.stp)
    verifica(nodo.count() == 1, "la proposta tratteggiata del nodo nuovo 53011111 non c'e' sotto l'assieme")
    verifica("proposal" in (nodo.get_attribute("class") or ""), "la proposta non e' tratteggiata")
    b.niente_orizzontale()
    b.foto("01_apertura.png")


@prova("B", "un clic sul nodo filtra i documenti, senza ricaricare")
def prova_b(b):
    b.apri()
    link = b.page.locator('[id^="nodo-%s"] a.node-link' % b.a.particolare).first
    b.clic_e_aspetta(link, "/fascicolo/parti")
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica("nodo=" + b.a.particolare in b.page.url, "l'indirizzo non dice il nodo: %s" % b.page.url)
    attiva = b.page.locator(".filtri-doc a.on")
    verifica(attiva.get_attribute("data-filtro") == "nodo", "linguetta accesa: %s" % attiva.get_attribute("data-filtro"))
    verifica(b.page.locator("#scheda-nodo").is_visible(), "la scheda del nodo scelto non si vede")
    cand = b.page.locator('.filtri-doc a[data-filtro="candidati"]')
    b.clic_e_aspetta(cand, "/fascicolo/parti")
    righe = b.page.locator("#documenti tbody tr[id^='file-']")
    verifica(righe.count() >= 4, "candidati per il nodo: %d righe" % righe.count())
    verifica(b.page.locator("#file-%s" % b.a.pdf).count() == 1, "il PDF 53017189.pdf non e' fra i candidati")
    verifica(b.viva(), "la pagina si e' ricaricata")


@prova("C", "un clic sul file apre il PDF nell'anteprima")
def prova_c(b):
    b.apri()
    link = b.page.locator("#file-%s td.nome a" % b.a.pdf)
    b.clic_e_aspetta(link, "/fascicolo/anteprima")
    ifr = b.page.locator("#anteprima-pdf")
    verifica(ifr.count() == 1, "nessun iframe nell'anteprima")
    src = ifr.get_attribute("src")
    verifica(src == "/allegato/%s/anteprima" % b.a.pdf, "iframe su %r" % src)
    verifica("file=" + b.a.pdf in b.page.url, "l'indirizzo non dice il file: %s" % b.page.url)
    verifica(b.viva(), "la pagina si e' ricaricata")
    r = b.page.request.get(b.a.url + src)
    verifica(r.status == 200 and r.headers.get("content-type") == "application/pdf", "il PDF non arriva: %d %s" % (r.status, r.headers.get("content-type")))
    b.foto("02_pdf_aperto.png")


@prova("D", "accettare una proposta cambia albero e completezza, il PDF resta aperto")
def prova_d(b):
    b.apri("?file=" + b.a.pdf)
    b.page.evaluate("document.getElementById('anteprima-pdf').dataset.marca = 'stesso'")
    riga = b.page.locator('[id="proposta-%s-#3"]' % b.a.stp)
    verifica(riga.count() == 1, "la proposta del nodo 53011111 non c'e'")
    bottone = riga.get_by_role("button", name="Accetta il nodo")
    b.clic_e_aspetta(bottone, "/accetta")
    verifica(b.viva(), "la pagina si e' ricaricata")
    marca = b.page.evaluate("(document.getElementById('anteprima-pdf') || {dataset: {}}).dataset.marca")
    verifica(marca == "stesso", "l'iframe del PDF e' stato sostituito o chiuso (segno: %r)" % marca)
    avviso = b.avviso()
    verifica(avviso and "Niente" not in avviso, "avviso: %r" % avviso)
    pieno = b.page.locator("#struttura .node:not(.proposal)", has_text="53011111")
    verifica(pieno.count() >= 1, "il nodo accettato non e' un nodo pieno dell'albero")
    verifica(b.page.locator("#completezza", has_text="53011111").count() == 1, "la completezza non ha il componente nuovo")
    # adesso l'arco si accetta dalla stessa riga, sempre senza ricaricare
    arco = b.page.locator('[id="proposta-%s-#3"]' % b.a.stp).get_by_role("button", name="Accetta l'arco ×2")
    verifica(arco.count() == 1, "dopo il nodo, la riga offre l'arco")
    b.clic_e_aspetta(arco, "/relazione/accetta")
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica(b.page.locator('[id^="nodo-"][id$="-%s"]' % b.a.assieme, has_text="53011111").count() >= 1,
             "dopo l'arco il nodo sta sotto l'assieme")
    marca = b.page.evaluate("(document.getElementById('anteprima-pdf') || {dataset: {}}).dataset.marca")
    verifica(marca == "stesso", "dopo l'arco l'iframe del PDF e' cambiato (segno: %r)" % marca)
    b.foto("03_dopo_accetta.png")


@prova("E", "tre file assegnati al nodo con un gesto")
def prova_e(b):
    b.apri("?nodo=%s&filtro=candidati" % b.a.particolare)
    for p in b.a.proposte.split(","):
        casella = b.page.locator('input[name="proposta"][value="%s"]' % p)
        verifica(casella.count() == 1, "la casella del file %s non c'e'" % p)
        casella.check()
    b.foto("08_assegna_prima.png")
    b.clic_e_aspetta(b.page.locator("#assegna-al-nodo"), "/fascicolo/assegna")
    verifica(b.viva(), "la pagina si e' ricaricata")
    avviso = b.avviso()
    verifica(avviso == "3 file assegnati al componente 53017189.", "avviso: %r" % avviso)
    n = b.page.locator('.filtri-doc a[data-filtro="nodo"] small').inner_text().strip()
    verifica(n == "3", "file del nodo dopo l'assegnazione: %s" % n)
    b.foto("08_assegna_dopo.png")


@prova("F", "correggere il tipo di un nodo cambia la completezza")
def prova_f(b):
    b.apri("?nodo=" + b.a.assieme)
    riga = b.page.locator("#completezza .comp-riga", has_text="52920517")
    verifica("assieme" in riga.inner_text(), "prima: %r" % riga.inner_text())
    scheda = b.page.locator("#scheda-nodo")
    scheda.locator("summary", has_text="Modifica").click()
    scheda.locator('select[name="tipo"]').first.select_option("sciolto")
    b.clic_e_aspetta(scheda.get_by_role("button", name="Salva"), "/modifica")
    verifica(b.viva(), "la pagina si e' ricaricata")
    riga = b.page.locator("#completezza .comp-riga", has_text="52920517")
    verifica("particolare" in riga.inner_text(), "dopo: %r" % riga.inner_text())
    verifica("52920517: tipo assieme → particolare" in b.avviso(), "avviso: %r" % b.avviso())


@prova("G", "cento file: meno di un secondo, il filtro, niente scorrimento orizzontale")
def prova_g(b):
    b.page.goto(b.base + "?filtro=tutti")
    b.page.wait_for_load_state("load")
    t = b.page.evaluate("(() => { const n = performance.getEntriesByType('navigation')[0]; return {dcl: n.domContentLoadedEventEnd, server: n.responseEnd - n.requestStart} })()")
    verifica(t["dcl"] < 1000, "la pagina e' pronta dopo %.0f ms" % t["dcl"])
    righe = b.page.locator("#documenti tbody tr[id^='file-']").count()
    verifica(righe >= 100, "righe: %d" % righe)
    b.page.evaluate("window.__marca = 'viva'")
    b.niente_orizzontale()
    b.foto("09_cento_file.png")
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="rumore"]'), "/fascicolo/parti")
    verifica(b.page.locator("#documenti tbody tr[id^='file-']").count() == 0, "il filtro rumore non filtra")
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="non_assegnati"]'), "/fascicolo/parti")
    verifica(b.page.locator("#documenti tbody tr[id^='file-']").count() >= 100, "il filtro non assegnati ha perso righe")
    b.niente_orizzontale()
    verifica(b.viva(), "la pagina si e' ricaricata")
    print("      pronta in %.0f ms (server %.0f ms), %d righe" % (t["dcl"], t["server"], righe))


@prova("H", "griglia, cassetti e riepilogo di uno STEP")
def prova_h(b):
    b.apri()
    b.clic_e_aspetta(b.page.locator(".ft-comandi a", has_text="Griglia"), "/fascicolo/parti")
    verifica(b.page.locator("#struttura table.griglia").is_visible(), "la griglia non si vede")
    b.foto("04_griglia.png")
    b.clic_e_aspetta(b.page.locator(".ft-comandi a", has_text="Codici"), "/fascicolo/parti")
    verifica(b.page.locator("#cassetto .cassetto-dentro").is_visible(), "il cassetto dei codici non si vede")
    verifica(b.page.locator("#cassetto", has_text="Codici della richiesta").count() == 1, "il cassetto non dice i codici")
    b.foto("05_cassetto_codici.png")
    # dal cassetto aperto si passa agli avvisi con le sue linguette: la testata e' sotto il cassetto
    b.clic_e_aspetta(b.page.locator("#cassetto .cassetto-linguette a", has_text="Avvisi"), "/fascicolo/parti")
    verifica(b.page.locator("#cassetto .avviso-riga").count() >= 1, "nessun avviso nel cassetto")
    b.foto("06_cassetto_avvisi.png")
    b.clic_e_aspetta(b.page.locator("#cassetto a.chiudi-cassetto"), "/fascicolo/parti")
    verifica(b.page.locator("#cassetto .cassetto-dentro").count() == 0, "il cassetto non si chiude")
    # lo STEP del banco non ha una proposta di documento: sta fra «Tutti», non fra i non assegnati
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="tutti"]'), "/fascicolo/parti")
    b.clic_e_aspetta(b.page.locator("#file-%s td.nome a" % b.a.stp), "/fascicolo/anteprima")
    corpo = b.page.locator("#anteprima-corpo")
    verifica("letta per intero" in corpo.inner_text(), "riepilogo dello STEP: %r" % corpo.inner_text()[:200])
    verifica(b.page.locator("#anteprima-corpo iframe").count() == 0, "per uno STEP niente iframe")
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.foto("07_step_riepilogo.png")


def main():
    ap = argparse.ArgumentParser()
    for k in ["url", "thread", "prodotto", "assieme", "particolare", "pdf", "stp", "proposte"]:
        ap.add_argument("--" + k, required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="ABCDEFGH")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()

    falliti = []
    fatte = 0
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        # una risorsa che risponde con un errore si nomina, non si conta e basta; la favicon non c'e' di proposito
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
                print("  %s  %-70s PASSATO   (%.1fs)" % (lettera, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(lettera)
                print("  %s  %-70s FALLITO   %s" % (lettera, nome, e))
                if a.foto:
                    b.foto("errore_%s.png" % lettera)
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
