# -*- coding: utf-8 -*-
"""L7 - il Fascicolo v3 in un BROWSER VERO: la vista Documenti, le note sul disegno, l'editor della struttura.

Non lo si lancia a mano: lo avvia `fascicolo_v3_browser_test.go` (tag `browser`), con il banco di
`fascicolo_browser_test.go` (una RFQ con una BOM, uno STEP con un nodo nuovo, PDF veri in staging).

Le prove (lettere, scelte dal test Go con --prove):
  J  la vista Documenti si apre da sola: il disegno disegnato da pdf.js, il filmstrip, la struttura; un clic
     su un'altra miniatura o una freccia cambia componente senza ricaricare; i collegamenti portano il
     componente scelto adesso; lo stage resta lo stesso dopo un gesto
  K  una nota sul disegno: si arma, si clicca il punto, si scrive, si salva (lo stage resta lo stesso elemento
     dopo il gesto); il pin resta dopo un F5, si apre, si toglie; un «no» del server lascia il testo nel riquadro
  L  l'editor della struttura: si apre dalla Struttura BOM, il nodo proposto dallo STEP si sposta sotto il
     prodotto, la conferma chiude l'editor e la BOM cambia senza ricaricare
  M  l'associazione dal pannello: il PDF in arrivo di un componente si conferma con un gesto
  N  l'editor senza STEP: un componente del vassoio trascinato sotto l'assieme, poi trascinato sotto il prodotto
     (trascinare sposta: dall'assieme sparisce), poi «Condividi» sotto l'assieme (un secondo padre, esplicito)

Il JavaScript della pagina si sorveglia: un errore, un messaggio d'errore in console o una risposta >= 400
fanno fallire la corsa.
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

    def avviso(self):
        el = self.page.locator("#fasc-avviso .avviso-f")
        return el.inner_text().strip() if el.count() else ""

    def disegno_pronto(self, timeout=15000):
        """Aspetta che lo stage abbia disegnato una pagina (il foglio visibile, un canvas non vuoto)."""
        self.page.wait_for_function("""() => {
            const f = document.querySelector('#doc-stage .ds-pagina');
            const c = f && f.querySelector('canvas');
            return f && !f.hidden && c && c.width > 50 && c.height > 50;
        }""", timeout=timeout)

    def scelto(self):
        return self.page.evaluate("(document.querySelector('.docv-tile.sel') || {dataset: {}}).dataset.k || ''")

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)

    def trascina(self, da, a):
        """Un trascinamento vero con il mouse: giu' sulla riga, un primo passo (Chromium comincia il drag solo
        dopo una soglia), poi fino al bersaglio a piccoli passi, su."""
        s, t = da.bounding_box(), a.bounding_box()
        x0, y0 = s["x"] + 12, s["y"] + s["height"] / 2
        self.page.mouse.move(x0, y0)
        self.page.mouse.down()
        self.page.mouse.move(x0 + 8, y0 + 4, steps=4)
        self.page.mouse.move(t["x"] + t["width"] / 3, t["y"] + t["height"] / 2, steps=12)
        self.page.mouse.up()
        self.page.wait_for_timeout(150)

    def niente_orizzontale(self):
        largo = self.page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
        verifica(largo <= 1, "la pagina scorre in orizzontale di %d px" % largo)


@prova("J", "Documenti: pdf.js, filmstrip, scelta senza ricaricare, collegamenti e stage che restano")
def prova_j(b):
    b.apri("?nodo=%s" % b.a.particolare)
    verifica(b.page.locator("#doc-vista").count() == 1, "la vista predefinita non e' Documenti")
    verifica(b.page.locator("#anteprima").is_hidden(), "il vecchio pannello di destra si vede ancora")
    verifica(b.page.locator("iframe").count() == 0, "nella vista Documenti c'e' un iframe")
    b.disegno_pronto()
    verifica(b.scelto() == "c:" + b.a.particolare, "il componente dell'indirizzo non e' quello scelto: %s" % b.scelto())
    # tre componenti nel filmstrip, nell'ordine dell'albero (il prodotto, l'assieme, il particolare)
    tiles = b.page.locator("#doc-film .docv-tile")
    verifica(tiles.count() >= 3, "miniature nel filmstrip: %d" % tiles.count())
    b.page.wait_for_function("document.querySelectorAll('#doc-film .docv-mini img').length >= 2", timeout=15000)
    b.foto("v3_01_documenti.png")
    b.niente_orizzontale()
    # la stage si marca: se venisse sostituita il segno sparirebbe
    b.page.evaluate("document.getElementById('doc-stage').dataset.segno = 'resta'")
    # un clic su un'altra miniatura: cambia componente, l'indirizzo lo dice, la pagina non si ricarica
    with b.page.expect_response(lambda r: "/fascicolo/sezione" in r.url, timeout=15000):
        b.page.locator('#doc-film .docv-tile[data-k="c:%s"]' % b.a.prodotto).click()
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica("nodo=%s" % b.a.prodotto in b.page.url, "l'indirizzo non porta il componente scelto: %s" % b.page.url)
    b.page.wait_for_function("(k) => (document.querySelector('.docv-sez') || {dataset: {}}).dataset.k === k", arg="c:" + b.a.prodotto, timeout=15000)
    b.disegno_pronto()
    # con la freccia si passa al componente dopo
    b.page.locator("#doc-stage .ds-area").focus()
    b.page.keyboard.press("ArrowRight")
    b.page.wait_for_function("(k) => (document.querySelector('.docv-tile.sel') || {dataset: {}}).dataset.k === k", arg="c:" + b.a.assieme, timeout=10000)
    b.page.wait_for_timeout(400)
    verifica("nodo=%s" % b.a.assieme in b.page.url, "dopo la freccia l'indirizzo non porta l'assieme: %s" % b.page.url)
    # il cassetto «Da verificare» si apre con il componente di adesso, non con quello con cui la pagina e' nata
    with b.page.expect_response(lambda r: "/fascicolo/parti" in r.url, timeout=15000) as risp:
        b.page.locator("#fasc-testata a.verifica").click()
    verifica("nodo=%s" % b.a.assieme in risp.value.url, "il cassetto e' stato chiesto con il nodo vecchio: %s" % risp.value.url)
    b.page.wait_for_timeout(300)
    verifica("nodo=%s" % b.a.assieme in b.page.url, "dopo il cassetto l'indirizzo ha perso il componente: %s" % b.page.url)
    verifica(b.page.evaluate("document.getElementById('doc-stage').dataset.segno") == "resta", "lo stage e' stato sostituito")
    b.page.locator("#cassetto a.chiudi-cassetto").click()
    b.page.wait_for_timeout(400)
    verifica(b.page.evaluate("document.getElementById('doc-stage').dataset.segno") == "resta", "lo stage e' stato sostituito chiudendo il cassetto")
    # la linguetta del gruppo gia' aperto, dopo una scelta fatta qui: tutto torna al primo elemento del gruppo
    # (il disegno, l'albero, il pannello e l'indirizzo dicono la stessa cosa)
    with b.page.expect_response(lambda r: "/fascicolo/parti" in r.url, timeout=15000):
        b.page.locator(".docv-gruppi a.on").click()
    b.page.wait_for_function("(k) => (document.querySelector('.docv-tile.sel') || {dataset: {}}).dataset.k === k && (document.querySelector('.docv-sez') || {dataset: {}}).dataset.k === k",
                             arg="c:" + b.a.prodotto, timeout=10000)
    # htmx scrive l'indirizzo della risposta (?gruppo=) prima dello scambio, e il markup del server dice gia' il
    # primo elemento: il viewer ci aggiunge il nodo all'assestamento, qualche millisecondo dopo
    fino = time.time() + 5
    while "nodo=%s" % b.a.prodotto not in b.page.url and time.time() < fino:
        b.page.wait_for_timeout(50)
    verifica("nodo=%s" % b.a.prodotto in b.page.url, "dopo la linguetta del gruppo l'indirizzo non dice il primo elemento: %s" % b.page.url)
    b.disegno_pronto()
    # le linguette: Struttura BOM e Completezza, e di nuovo Documenti con il disegno
    b.page.locator('#fasc-testata a[data-vista="completezza"]').click()
    b.page.wait_for_selector("table#completezza", timeout=10000)
    b.page.locator('#fasc-testata a[data-vista="documenti"]').click()
    b.page.wait_for_selector("#doc-vista", timeout=10000)
    b.disegno_pronto()
    verifica(b.viva(), "cambiando vista la pagina si e' ricaricata")


@prova("K", "una nota sul disegno: si mette, resta, si apre, si toglie")
def prova_k(b):
    b.apri("?nodo=%s&file=%s" % (b.a.particolare, b.a.pdf))
    b.disegno_pronto()
    b.page.locator("#doc-stage .ds-arma").click()
    verifica(b.page.locator("#doc-stage .ds-area.armato").count() == 1, "il cursore non si e' armato")
    foglio = b.page.locator("#doc-stage .ds-pagina")
    box = foglio.bounding_box()
    b.page.mouse.click(box["x"] + box["width"] * 0.30, box["y"] + box["height"] * 0.40)
    pop = b.page.locator("#doc-stage .ds-pop textarea")
    verifica(pop.count() == 1, "il riquadro della nota non si e' aperto")
    pop.fill("Tolleranza da verificare sul foro")
    # un «no» del server: il riquadro resta aperto, con il testo, e dice perche'
    no = '<div class="avviso-f no" role="status">Niente è cambiato: prova di rifiuto</div>'
    b.page.route("**/fascicolo/nota", lambda route: route.fulfill(status=200, content_type="text/html; charset=utf-8", body=no))
    b.page.locator("#doc-stage .ds-pop .btn.primary").click()
    b.page.wait_for_selector("#doc-stage .ds-pop .ds-pop-errore:not([hidden])", timeout=10000)
    verifica("prova di rifiuto" in b.page.locator("#doc-stage .ds-pop .ds-pop-errore").inner_text(), "il riquadro non dice il rifiuto")
    verifica(pop.input_value() == "Tolleranza da verificare sul foro", "dopo il rifiuto il testo non c'e' piu'")
    b.page.unroute("**/fascicolo/nota")
    # lo stage e' lo stesso elemento dopo un gesto che rifa' l'area principale
    b.page.evaluate("document.getElementById('doc-stage').dataset.segno = 'resta'")
    with b.page.expect_response(lambda r: r.url.endswith("/fascicolo/nota") and r.request.method == "POST", timeout=15000) as risp:
        b.page.locator("#doc-stage .ds-pop .btn.primary").click()
    verifica(risp.value.status == 200, "salvataggio della nota: %d" % risp.value.status)
    b.page.wait_for_selector("#doc-stage .ds-pin:not(.tmp)", timeout=10000)
    verifica(b.page.evaluate("document.getElementById('doc-stage').dataset.segno") == "resta", "il gesto ha sostituito lo stage")
    pin = b.page.locator("#doc-stage .ds-pin:not(.tmp)")
    verifica(pin.count() == 1 and pin.first.inner_text().strip() == "1", "il pin numero 1 non c'e'")
    verifica(b.page.locator(".docv-nota").count() == 1, "la nota non e' nell'elenco del pannello")
    # dove sta il pin: sul punto cliccato (in percentuale del foglio)
    x = b.page.evaluate("parseFloat(document.querySelector('#doc-stage .ds-pin:not(.tmp)').style.left)")
    y = b.page.evaluate("parseFloat(document.querySelector('#doc-stage .ds-pin:not(.tmp)').style.top)")
    verifica(abs(x - 30) < 1.5 and abs(y - 40) < 1.5, "il pin e' in %.1f%%, %.1f%% invece di 30%%, 40%%" % (x, y))
    b.foto("v3_02_nota.png")
    # dopo un F5 il pin c'e' ancora, e ingrandendo resta sul punto
    b.apri("?nodo=%s&file=%s" % (b.a.particolare, b.a.pdf))
    b.disegno_pronto()
    b.page.wait_for_selector("#doc-stage .ds-pin", timeout=10000)
    b.page.locator("#doc-stage .ds-piu").click()
    b.page.wait_for_timeout(700)
    b.disegno_pronto()
    x2 = b.page.evaluate("parseFloat(document.querySelector('#doc-stage .ds-pin').style.left)")
    verifica(abs(x2 - 30) < 1.5, "ingrandendo il pin si e' spostato: %.1f%%" % x2)
    # un clic sul pin lo apre; «Togli» lo toglie
    b.page.locator("#doc-stage .ds-pin").first.click()
    verifica("Tolleranza da verificare sul foro" in b.page.locator("#doc-stage .ds-pop").inner_text(), "il testo della nota non si vede")
    with b.page.expect_response(lambda r: "/elimina" in r.url, timeout=15000):
        b.page.locator("#doc-stage .ds-pop button", has_text="Togli").click()
    b.page.wait_for_function("document.querySelectorAll('#doc-stage .ds-pin').length === 0", timeout=10000)
    verifica(b.page.locator(".docv-nota").count() == 0, "la nota tolta e' ancora nell'elenco")
    # una nota vuota non si manda; il riquadro resta
    b.page.locator("#doc-stage .ds-arma").click()
    box = foglio.bounding_box()
    b.page.mouse.click(box["x"] + box["width"] * 0.5, box["y"] + box["height"] * 0.5)
    b.page.locator("#doc-stage .ds-pop .btn.primary").click()
    verifica(b.page.locator("#doc-stage .ds-pop").count() == 1, "il riquadro si e' chiuso senza testo")
    b.page.keyboard.press("Escape")
    verifica(b.page.locator("#doc-stage .ds-pop").count() == 0, "Esc non chiude il riquadro")
    b.page.keyboard.press("Escape")
    verifica(b.page.locator("#doc-stage .ds-area.armato").count() == 0, "Esc non disarma il cursore")


@prova("L", "l'editor della struttura: sposta il nodo proposto, conferma, la BOM cambia")
def prova_l(b):
    b.apri("?vista=bom")
    b.page.locator(".editor-avvio [data-editor]").first.click()
    ed = b.page.locator(".bomed")
    ed.wait_for(timeout=10000)
    riga = ed.locator('.bomed-albero .bomed-riga', has_text="77811111")
    verifica(riga.count() == 1, "il nodo proposto 77811111 non e' nell'albero dell'editor")
    verifica("proposto" in (riga.get_attribute("class") or ""), "il nodo proposto non e' segnato come proposto")
    b.foto("v3_03_editor.png")
    # «Sposta sotto…» dal menu, sotto il prodotto
    riga.locator(".bomed-apri-menu").click()
    ed.locator('.bomed-menu-dentro button[data-voce="sposta"]').click()
    scegli = ed.locator(".bomed-scegli select")
    scegli.select_option(label="77722757")
    ed.locator(".bomed-scegli button.primario").click()
    radice = ed.locator(".bomed-albero .bomed-riga.radice")
    verifica(radice.count() == 1, "la radice non c'e'")
    verifica(ed.locator('.bomed-albero .bomed-riga[aria-level="2"]', has_text="77811111").count() == 1,
             "77811111 non e' finito sotto il prodotto")
    verifica("modific" in ed.locator(".bomed-conferma").inner_text(), "il bottone di conferma non conta le modifiche")
    with b.page.expect_response(lambda r: "/bom/applica" in r.url, timeout=15000) as risp:
        ed.locator(".bomed-conferma").click()
    verifica(risp.value.status == 200, "conferma della struttura: %d" % risp.value.status)
    b.page.wait_for_selector(".bomed", state="detached", timeout=10000)
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica(b.avviso().startswith("Struttura di 77722757 confermata"), "avviso: %r" % b.avviso())
    # nella BOM visuale 77811111 adesso e' un componente sotto il prodotto
    verifica(b.page.locator('#tela div.carta[id^="nodo-"][id$="-%s"]' % b.a.prodotto, has_text="77811111").count() == 1,
             "77811111 non e' sotto il prodotto nella BOM")
    b.foto("v3_04_dopo_editor.png")


@prova("M", "l'associazione dal pannello: il PDF in arrivo si conferma con un gesto")
def prova_m(b):
    b.apri("?nodo=%s" % b.a.assieme)
    b.page.wait_for_selector(".docv-sez", timeout=10000)
    f = b.page.locator(".docv-file", has_text="77720517.pdf")
    verifica(f.count() == 1, "il PDF in arrivo dell'assieme non e' nel pannello")
    testo = f.text_content()  # inner_text applica il text-transform delle etichette
    for atteso in ["Tipo rilevato", "Codice letto", "Associato a", "Confidenza"]:
        verifica(atteso in testo, "manca %r nel pannello del file" % atteso)
    with b.page.expect_response(lambda r: r.url.endswith("/fascicolo/conferma") and r.request.method == "POST", timeout=15000):
        f.locator(".docv-conferma").click()
    b.page.wait_for_timeout(400)
    verifica(b.viva(), "la pagina si e' ricaricata")
    av = b.avviso()
    verifica("1 documento" in av or "Confermat" in av, "avviso: %r" % av)
    b.page.wait_for_function("() => [...document.querySelectorAll('.docv-file')].some(x => x.innerText.includes('77720517.pdf') && x.innerText.includes('Confermato'))", timeout=10000)
    b.foto("v3_05_confermato.png")


@prova("N", "l'editor senza STEP: dal vassoio, trascinare sposta, Condividi aggiunge un padre")
def prova_n(b):
    b.apri("?vista=bom")
    b.page.locator(".editor-avvio [data-editor]").first.click()
    ed = b.page.locator(".bomed")
    ed.wait_for(timeout=10000)
    riga = lambda dove, cod: ed.locator(dove + " .bomed-riga", has_text=cod)
    vass = riga(".bomed-vassoio", "54000000")
    verifica(vass.count() == 1, "54000000 non e' fra i non posizionati")
    # dal vassoio sotto l'assieme
    b.trascina(vass, riga(".bomed-albero", "77720517").first)
    b.page.wait_for_function("() => [...document.querySelectorAll('.bomed-albero .bomed-riga')].some(r => r.textContent.includes('54000000') && r.dataset.arco.includes('|'))", timeout=5000)
    sotto = riga(".bomed-albero", "54000000")
    verifica(sotto.count() == 1 and sotto.get_attribute("aria-level") == "3", "54000000 non e' sotto l'assieme (livello %s)" % sotto.first.get_attribute("aria-level"))
    verifica(riga(".bomed-vassoio", "54000000").count() == 0, "54000000 e' rimasto nel vassoio")
    # trascinato sotto il prodotto: si sposta, dall'assieme sparisce
    b.trascina(sotto, ed.locator(".bomed-albero .bomed-riga.radice"))
    b.page.wait_for_timeout(200)
    righe = riga(".bomed-albero", "54000000")
    verifica(righe.count() == 1 and righe.get_attribute("aria-level") == "2", "trascinare non ha spostato 54000000 sotto il prodotto")
    b.foto("v3_06_editor_trascina.png")
    # «Condividi»: un secondo padre, esplicito
    righe.locator(".bomed-apri-menu").click()
    ed.locator('.bomed-menu-dentro button[data-voce="condividi"]').click()
    ed.locator(".bomed-scegli select").select_option(label="77720517")
    ed.locator(".bomed-scegli button.primario").click()
    righe = riga(".bomed-albero", "54000000")
    verifica(righe.count() == 2, "dopo Condividi 54000000 compare %d volte" % righe.count())
    verifica(ed.locator(".bomed-albero .bomed-riga", has_text="condiviso · 2 padri").filter(has_text="54000000").count() >= 1, "il secondo padre non si vede come condiviso")
    with b.page.expect_response(lambda r: "/bom/applica" in r.url, timeout=15000) as risp:
        ed.locator(".bomed-conferma").click()
    verifica(risp.value.status == 200, "conferma: %d" % risp.value.status)
    b.page.wait_for_selector(".bomed", state="detached", timeout=10000)
    verifica(b.avviso().startswith("Struttura di 77722757 confermata"), "avviso: %r" % b.avviso())
    b.foto("v3_07_dopo_condividi.png")


def main():
    ap = argparse.ArgumentParser()
    for k in ["url", "thread", "prodotto", "assieme", "particolare", "pdf"]:
        ap.add_argument("--" + k, required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="JKLMN")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    falliti = []
    fatte = 0
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        page.on("dialog", lambda d: d.accept())
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
