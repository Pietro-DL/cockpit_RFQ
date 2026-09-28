# -*- coding: utf-8 -*-
"""L7 - l'anteprima PDF in un BROWSER VERO (blocco 8, B8.1).

Non lo si lancia a mano: lo avvia `anteprima_browser_test.go` (tag `browser`), che prima prepara il
banco su PostgreSQL, scrive il file sul NAS finto e mette in piedi il server vero.

    python anteprima_pdf.py --url http://127.0.0.1:PORTA --thread UUID --allegato UUID --byte 2097152

Perche' serve un browser vero. Le prove L4 chiedono al server e guardano la risposta: passano anche
se il pulsante non c'e', se apre l'indirizzo sbagliato, o se il viewer del browser riceve
intestazioni che gli impediscono di chiedere i pezzi. Qui si guarda cio' che vede chi lavora: il
pulsante nella riga dell'allegato, la scheda che si apre, e - con la rete del browser, non con quella
di Python - un Range servito a pezzi e un secondo giro che non porta byte.

Il viewer PDF incorporato NON si interroga: in una finestra senza schermo Edge a volte lo disegna e a
volte scarica il file, e una prova che dipende da quale dei due fa oggi non e' una prova. Si
verifica cio' che dipende da noi: che il browser apra l'indirizzo giusto e che il server risponda
come un file server deve rispondere.
"""
import argparse
import sys
import time

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


def link_anteprima(page, base, a):
    """Il pulsante come lo vede l'occhio: dentro la riga dell'allegato, nella pagina della RFQ."""
    page.goto(base + "/thread/" + a.thread)
    page.wait_for_load_state("domcontentloaded")
    return page.locator("a[href='/allegato/%s/anteprima']" % a.allegato)


@prova("A  il pulsante «Anteprima» e' nella riga dell'allegato")
def prova_a(page, base, a):
    link = link_anteprima(page, base, a)
    verifica(link.count() == 1, "link «Anteprima» trovati: %d" % link.count())
    verifica(link.first.inner_text().strip() == "Anteprima",
             "il link si chiama %r" % link.first.inner_text().strip())
    verifica(link.first.get_attribute("target") == "_blank",
             "l'anteprima non apre una scheda nuova: chi la chiude perderebbe la RFQ aperta")
    rel = (link.first.get_attribute("rel") or "")
    verifica("noopener" in rel, "manca rel=noopener sul link che apre la scheda nuova (rel=%r)" % rel)


@prova("B  il clic apre una scheda nuova sull'indirizzo dell'anteprima")
def prova_b(page, base, a):
    link = link_anteprima(page, base, a)
    atteso = base + "/allegato/%s/anteprima" % a.allegato
    with page.context.expect_page(timeout=15000) as nuova:
        link.first.click(modifiers=[])
    scheda = nuova.value
    # Il viewer puo' metterci un istante; l'indirizzo e' quello dal primo momento.
    verifica(scheda.url.startswith(atteso) or scheda.url == "about:blank",
             "la scheda nuova e' andata su %r invece che su %r" % (scheda.url, atteso))
    if scheda.url == "about:blank":
        try:
            scheda.wait_for_url(atteso, timeout=5000)
        except Exception:
            pass
    scheda.close()


@prova("C  il browser riceve un PDF intero, con le intestazioni che contano")
def prova_c(page, base, a):
    r = page.request.get(base + "/allegato/%s/anteprima" % a.allegato)
    verifica(r.status == 200, "stato %d" % r.status)
    verifica(r.headers.get("content-type") == "application/pdf",
             "content-type %r" % r.headers.get("content-type"))
    disp = r.headers.get("content-disposition", "")
    verifica(disp.startswith("inline"), "content-disposition %r: l'anteprima si guarda, non si scarica" % disp)
    verifica(r.headers.get("x-content-type-options") == "nosniff", "manca nosniff")
    verifica("sandbox" in (r.headers.get("content-security-policy") or ""),
             "manca la CSP sandbox: un PDF puo' contenere JavaScript")
    corpo = r.body()
    verifica(corpo[:5] == b"%PDF-", "i primi byte non sono quelli di un PDF: %r" % corpo[:16])
    verifica(len(corpo) == a.byte, "il file servito e' %d byte, ne erano attesi %d" % (len(corpo), a.byte))


@prova("D  un Range chiede un pezzo e riceve esattamente quel pezzo")
def prova_d(page, base, a):
    pezzo = 512 * 1024
    r = page.request.get(base + "/allegato/%s/anteprima" % a.allegato,
                         headers={"Range": "bytes=0-%d" % (pezzo - 1)})
    verifica(r.status == 206, "stato %d, atteso 206: senza Range un PDF grosso si apre solo dopo averlo scaricato tutto" % r.status)
    verifica(len(r.body()) == pezzo, "pezzo servito: %d byte, chiesti %d" % (len(r.body()), pezzo))
    cr = r.headers.get("content-range", "")
    verifica(cr == "bytes 0-%d/%d" % (pezzo - 1, a.byte), "content-range %r" % cr)
    # e l'ultimo pezzo, che e' quello da cui il viewer comincia davvero (la tabella delle pagine)
    coda = page.request.get(base + "/allegato/%s/anteprima" % a.allegato,
                            headers={"Range": "bytes=-1024"})
    verifica(coda.status == 206, "coda: stato %d" % coda.status)
    verifica(len(coda.body()) == 1024, "coda: %d byte" % len(coda.body()))


@prova("E  con l'ETag il secondo giro e' 304 e non porta byte")
def prova_e(page, base, a):
    r = page.request.get(base + "/allegato/%s/anteprima" % a.allegato)
    etag = r.headers.get("etag", "")
    verifica(etag != "", "nessun ETag")
    r2 = page.request.get(base + "/allegato/%s/anteprima" % a.allegato,
                          headers={"If-None-Match": etag})
    verifica(r2.status == 304, "stato %d, atteso 304" % r2.status)
    verifica(len(r2.body()) == 0, "un 304 con %d byte di corpo" % len(r2.body()))


@prova("F  HEAD dice la dimensione e che i Range si possono chiedere")
def prova_f(page, base, a):
    r = page.request.head(base + "/allegato/%s/anteprima" % a.allegato)
    verifica(r.status == 200, "stato %d" % r.status)
    verifica(r.headers.get("accept-ranges") == "bytes",
             "accept-ranges %r: il viewer non saprebbe di poter chiedere pezzi" % r.headers.get("accept-ranges"))
    verifica(r.headers.get("content-length") == str(a.byte),
             "content-length %r, attesi %d" % (r.headers.get("content-length"), a.byte))


@prova("G  senza sessione il file non esce")
def prova_g(page, base, a):
    anonimo = page.context.browser.new_context(extra_http_headers={"X-Prova-IP": a.ip})
    try:
        r = anonimo.request.get(base + "/allegato/%s/anteprima" % a.allegato)
        # La rete del browser segue i redirect: senza sessione si finisce sulla pagina di accesso, e
        # cio' che conta e' che quello che torna non sia il PDF.
        verifica(b"%PDF-" not in r.body(), "il file e' uscito a un browser senza sessione")
        verifica(r.status != 200 or "/login" in r.url,
                 "senza sessione la risposta e' %d da %r e non e' l'accesso" % (r.status, r.url))
    finally:
        anonimo.close()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--thread", required=True)
    ap.add_argument("--allegato", required=True)
    ap.add_argument("--byte", type=int, required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge", help="browser installato: msedge | chrome")
    ap.add_argument("--vedi", action="store_true", help="finestra visibile, per guardare")
    ap.add_argument("--solo", default="", help="lettere delle prove da eseguire, es. ACD")
    a = ap.parse_args()

    falliti = []
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip},
                                  viewport={"width": 1400, "height": 900},
                                  accept_downloads=True)
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
                fn(page, a.url, a)
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
