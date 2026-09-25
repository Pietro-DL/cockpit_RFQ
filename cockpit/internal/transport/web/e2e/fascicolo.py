# -*- coding: utf-8 -*-
"""L7 - la schermata del Fascicolo in un BROWSER VERO (B8.7, rifatta in B8.7b).

Non lo si lancia a mano: lo avvia `fascicolo_browser_test.go` (tag `browser`), che prepara il banco su
PostgreSQL (una RFQ con una BOM, uno STEP con un nodo nuovo, PDF veri in staging) e mette in piedi il
server vero.

Le prove (lettere, scelte dal test Go con --prove):
  A  la pagina si apre con la BOM visuale, il dettaglio e la barra del piano, senza scorrimento orizzontale
  B  un clic sulla card apre il dettaglio del componente, senza ricaricare; i candidati nella vista Documenti
  C  un clic sul file apre il PDF nel pannello di destra
  D  accettare il nodo (vista Albero) e poi la struttura (BOM) cambia la BOM senza F5 e senza chiudere il PDF
  E  tre file assegnati al nodo con un gesto
  F  correggere la struttura (il tipo) cambia la completezza sulla card
  G  cento file: la pagina in meno di un secondo, il filtro, niente scorrimento orizzontale
  H  viste tecniche (griglia, codici, avvisi), «Da verificare», riepilogo di uno STEP (e le fotografie)
  I  «Rivedi» e «Conferma Fascicolo»: il piano entra con un gesto

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

    def menu(self, voce):
        """Apre il menu «•••» della testata e sceglie una voce (le viste tecniche, i cassetti tecnici)."""
        m = self.page.locator("#fasc-testata details.menu:not(.aggiungi)")
        m.locator("summary").click()
        self.clic_e_aspetta(m.locator(".menu-dentro a", has_text=voce), "/fascicolo/parti")

    def carta_confermata(self, codice, padre):
        """La card di un componente della working sotto quel padre (l'id finisce con il padre)."""
        return self.page.locator('#tela div.carta[id^="nodo-"][id$="-%s"]' % padre, has_text=codice)

    def pdf_marcato(self):
        return self.page.evaluate("(document.getElementById('anteprima-pdf') || {dataset: {}}).dataset.marca")

    def niente_orizzontale(self):
        largo = self.page.evaluate("document.documentElement.scrollWidth - document.documentElement.clientWidth")
        verifica(largo <= 1, "la pagina scorre in orizzontale di %d px" % largo)
        # i due pannelli: l'area principale (la tela della BOM scorre dentro di se', non il pannello) e il dettaglio
        for pan in [".fasc-principale", "#anteprima"]:
            d = self.page.evaluate("(() => { const e = document.querySelector('%s'); return e.scrollWidth - e.clientWidth })()" % pan)
            verifica(d <= 1, "il pannello %s scorre in orizzontale di %d px" % (pan, d))

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)


@prova("A", "la pagina si apre con la BOM visuale, il dettaglio e il piano, niente scorrimento orizzontale")
def prova_a(b):
    b.apri()
    for sel in ["#fasc-testata", "#tela", "#anteprima", "#piano", "#conferma-fascicolo"]:
        verifica(b.page.locator(sel).is_visible(), "%s non si vede" % sel)
    carta = b.page.locator('[id="proposta-%s-#3"]' % b.a.stp)
    verifica(carta.count() == 1, "la card del nodo nuovo 53011111, proposta dallo STEP, non c'e'")
    verifica("proposta" in (carta.get_attribute("class") or "").split(), "la card proposta non ha lo stile delle proposte")
    # nella gerarchia: sotto la card dell'assieme, con la quantita' sull'arco
    sotto = b.page.locator('li:has(> div[id^="nodo-%s-"]) [id="proposta-%s-#3"]' % (b.a.assieme, b.a.stp))
    verifica(sotto.count() == 1, "la card proposta non sta sotto l'assieme")
    arco = carta.locator("xpath=../span[contains(concat(' ', @class, ' '), ' arco ')]")
    verifica(arco.count() == 1 and arco.inner_text().strip() == "×2", "la quantita' sull'arco proposto: %r" % (arco.all_inner_texts(),))
    condiviso = b.page.locator('#tela div.carta[id^="nodo-%s-"]' % b.a.particolare, has_text="condiviso")
    verifica(condiviso.count() >= 1, "il particolare sotto due padri non si riconosce come condiviso")
    verifica(b.page.locator('#tela div.carta.prodotto[id="nodo-%s"]' % b.a.prodotto).count() == 1, "il prodotto finito non e' una card radice")
    # il file picker non sta davanti: e' dentro «+ Aggiungi file › Carica dal PC»
    verifica(b.page.locator("input[type=file]").count() == 1, "i file picker nella pagina: %d" % b.page.locator("input[type=file]").count())
    verifica(not b.page.locator("input[type=file]").is_visible(), "il file picker si vede senza aprire «Aggiungi file»")
    b.niente_orizzontale()
    b.foto("01_apertura.png")


@prova("B", "un clic sulla card apre il dettaglio del componente, senza ricaricare; i candidati")
def prova_b(b):
    b.apri()
    link = b.page.locator('#tela div.carta[id^="nodo-%s-"] a.carta-link' % b.a.particolare).first
    b.clic_e_aspetta(link, "/fascicolo/anteprima")
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica("nodo=" + b.a.particolare in b.page.url, "l'indirizzo non dice il nodo: %s" % b.page.url)
    scheda = b.page.locator("#scheda-nodo")
    verifica(scheda.is_visible() and "53017189" in scheda.inner_text(), "il dettaglio del componente scelto non si vede")
    nomi = [x.strip() for x in b.page.locator("#anteprima nav.schede a").all_inner_texts()]
    for s in ["3D", "2D", "DXF", "Altri", "Storico"]:
        verifica(any(n.startswith(s) for n in nomi), "la scheda %s non c'e' nel dettaglio: %r" % (s, nomi))
    verifica(b.page.locator('#tela div.carta.sel[id^="nodo-%s-"]' % b.a.particolare).count() >= 1, "la card scelta non e' evidenziata nella BOM")
    verifica(b.page.locator("#anteprima .in-arrivo", has_text="53017189.pdf").count() == 1, "il PDF del particolare non e' fra i file in arrivo")
    b.clic_e_aspetta(b.page.locator("#anteprima nav.schede a", has_text="3D"), "/fascicolo/anteprima")
    verifica("scheda=3d" in b.page.url, "l'indirizzo non dice la scheda: %s" % b.page.url)
    verifica(b.page.locator("#anteprima nav.schede a.on").inner_text().strip().startswith("3D"), "la scheda 3D non e' accesa")
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.foto("02_dettaglio_componente.png")
    # la vista Documenti tiene il nodo scelto: i file candidati per lui
    b.clic_e_aspetta(b.page.locator("#fasc-testata .linguette a", has_text="Documenti"), "/fascicolo/parti")
    verifica("vista=documenti" in b.page.url and "nodo=" + b.a.particolare in b.page.url, "la vista Documenti ha perso il nodo: %s" % b.page.url)
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="candidati"]'), "/fascicolo/parti")
    righe = b.page.locator("#vista tbody tr[id^='file-']")
    verifica(righe.count() >= 4, "candidati per il nodo: %d righe" % righe.count())
    verifica(b.page.locator("#file-%s" % b.a.pdf).count() == 1, "il PDF 53017189.pdf non e' fra i candidati")
    verifica(b.viva(), "la pagina si e' ricaricata")


@prova("C", "un clic sul file apre il PDF nel pannello di destra")
def prova_c(b):
    b.apri("?vista=documenti")
    link = b.page.locator("#file-%s td.nome a" % b.a.pdf)
    b.clic_e_aspetta(link, "/fascicolo/anteprima")
    ifr = b.page.locator("#anteprima-pdf")
    verifica(ifr.count() == 1, "nessun iframe nel pannello di destra")
    src = ifr.get_attribute("src")
    verifica(src == "/allegato/%s/anteprima" % b.a.pdf, "iframe su %r" % src)
    verifica("file=" + b.a.pdf in b.page.url, "l'indirizzo non dice il file: %s" % b.page.url)
    verifica(b.viva(), "la pagina si e' ricaricata")
    r = b.page.request.get(b.a.url + src)
    verifica(r.status == 200 and r.headers.get("content-type") == "application/pdf", "il PDF non arriva: %d %s" % (r.status, r.headers.get("content-type")))
    b.foto("03_pdf_aperto.png")


@prova("D", "accettare nodo e struttura cambia la BOM, il PDF resta aperto")
def prova_d(b):
    # il nodo dalla vista tecnica Albero, riga per riga come in B8.7
    b.apri("?vista=albero&file=" + b.a.pdf)
    b.page.evaluate("document.getElementById('anteprima-pdf').dataset.marca = 'stesso'")
    riga = b.page.locator('[id="albero-proposta-%s-#3"]' % b.a.stp)
    verifica(riga.count() == 1, "la proposta del nodo 53011111 non c'e' nella vista Albero")
    b.clic_e_aspetta(riga.get_by_role("button", name="Accetta il nodo"), "/accetta")
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica(b.pdf_marcato() == "stesso", "l'iframe del PDF e' stato sostituito o chiuso (segno: %r)" % b.pdf_marcato())
    avviso = b.avviso()
    verifica(avviso and "Niente" not in avviso, "avviso: %r" % avviso)
    verifica(b.page.locator("#vista .node:not(.proposal)", has_text="53011111").count() >= 1, "il nodo accettato non e' un nodo pieno dell'albero")
    # la BOM visuale: resta l'arco proposto, e lo STEP lo offre con «Applica struttura proposta»
    b.clic_e_aspetta(b.page.locator("#fasc-testata .linguette a", has_text="BOM"), "/fascicolo/parti")
    verifica(b.pdf_marcato() == "stesso", "cambiando vista l'iframe del PDF e' cambiato (segno: %r)" % b.pdf_marcato())
    rimando = b.page.locator('#tela li.li-arco-proposto > div.carta[id^="arco-%s-"]' % b.a.stp, has_text="53011111")
    verifica(rimando.count() == 1, "l'arco proposto 52920517 → 53011111 non e' disegnato sotto l'assieme")
    banner = b.page.locator('[id="step-%s"]' % b.a.stp)
    verifica(banner.count() == 1 and "1 arco" in banner.inner_text(), "il banner dello STEP: %r" % (banner.all_inner_texts(),))
    b.foto("04_arco_proposto.png")
    b.clic_e_aspetta(banner.get_by_role("button", name="Applica struttura proposta"), "/accetta")
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica(b.pdf_marcato() == "stesso", "dopo la struttura l'iframe del PDF e' cambiato (segno: %r)" % b.pdf_marcato())
    pieno = b.carta_confermata("53011111", b.a.assieme)
    verifica(pieno.count() == 1, "dopo la struttura 53011111 non e' una card confermata sotto l'assieme")
    verifica("proposta" not in (pieno.get_attribute("class") or "").split(), "la card di 53011111 e' ancora una proposta")
    verifica(pieno.locator(".docs .doc").count() >= 1, "la card nuova non mostra la sua completezza")
    verifica(b.page.locator('[id="step-%s"]' % b.a.stp).count() == 0, "il banner dello STEP e' rimasto dopo la struttura applicata")
    b.foto("05_dopo_struttura.png")


@prova("E", "tre file assegnati al nodo con un gesto")
def prova_e(b):
    b.apri("?vista=documenti&nodo=%s&filtro=candidati" % b.a.particolare)
    for p in b.a.proposte.split(","):
        casella = b.page.locator('input[name="proposta"][value="%s"]' % p)
        verifica(casella.count() == 1, "la casella del file %s non c'e'" % p)
        casella.check()
    b.foto("12_assegna_prima.png")
    b.clic_e_aspetta(b.page.locator("#assegna-al-nodo"), "/fascicolo/assegna")
    verifica(b.viva(), "la pagina si e' ricaricata")
    avviso = b.avviso()
    verifica(avviso == "3 file assegnati al componente 53017189.", "avviso: %r" % avviso)
    n = b.page.locator('.filtri-doc a[data-filtro="nodo"] small').inner_text().strip()
    verifica(n == "3", "file del nodo dopo l'assegnazione: %s" % n)
    b.foto("12_assegna_dopo.png")


@prova("F", "correggere il tipo di un nodo cambia la card e la completezza")
def prova_f(b):
    b.apri("?nodo=" + b.a.assieme)
    carta = b.page.locator('#tela div.carta[id="nodo-%s-%s"]' % (b.a.assieme, b.a.prodotto))
    tipo = carta.locator(".carta-tipo").text_content().strip()  # il testo, non come lo mostra il CSS (maiuscolo)
    verifica(tipo == "assieme", "prima: %r" % tipo)
    pannello = b.page.locator("#anteprima")
    pannello.locator("details.azioni-nodo > summary").click()
    pannello.locator("details.azioni-nodo details > summary", has_text="Modifica").click()
    pannello.locator('select[name="tipo"]').first.select_option("sciolto")
    b.clic_e_aspetta(pannello.get_by_role("button", name="Salva", exact=True), "/modifica")
    verifica(b.viva(), "la pagina si e' ricaricata")
    carta = b.page.locator('#tela div.carta[id="nodo-%s-%s"]' % (b.a.assieme, b.a.prodotto))
    tipo = carta.locator(".carta-tipo").text_content().strip()  # il testo, non come lo mostra il CSS (maiuscolo)
    verifica(tipo == "particolare", "dopo: %r" % tipo)
    verifica("52920517: tipo assieme → particolare" in b.avviso(), "avviso: %r" % b.avviso())
    # la vista tecnica Completezza, con la riga del componente (il controllo di B8.7)
    b.menu("Completezza")
    riga = b.page.locator("#completezza .comp-riga", has_text="52920517")
    verifica("particolare" in riga.text_content(), "la completezza dopo: %r" % riga.text_content())


@prova("G", "cento file: meno di un secondo, il filtro, niente scorrimento orizzontale")
def prova_g(b):
    b.page.goto(b.base + "?vista=documenti&filtro=tutti")
    b.page.wait_for_load_state("load")
    t = b.page.evaluate("(() => { const n = performance.getEntriesByType('navigation')[0]; return {dcl: n.domContentLoadedEventEnd, server: n.responseEnd - n.requestStart} })()")
    verifica(t["dcl"] < 1000, "la pagina e' pronta dopo %.0f ms" % t["dcl"])
    righe = b.page.locator("#vista tbody tr[id^='file-']").count()
    verifica(righe >= 100, "righe: %d" % righe)
    b.page.evaluate("window.__marca = 'viva'")
    b.niente_orizzontale()
    b.foto("13_cento_file.png")
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="rumore"]'), "/fascicolo/parti")
    verifica(b.page.locator("#vista tbody tr[id^='file-']").count() == 0, "il filtro rumore non filtra")
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="non_assegnati"]'), "/fascicolo/parti")
    verifica(b.page.locator("#vista tbody tr[id^='file-']").count() >= 100, "il filtro non assegnati ha perso righe")
    b.niente_orizzontale()
    verifica(b.viva(), "la pagina si e' ricaricata")
    # la BOM visuale con gli stessi cento file (nel piano)
    b.page.goto(b.base)
    b.page.wait_for_load_state("load")
    t2 = b.page.evaluate("(() => { const n = performance.getEntriesByType('navigation')[0]; return {dcl: n.domContentLoadedEventEnd, server: n.responseEnd - n.requestStart} })()")
    verifica(t2["dcl"] < 1000, "la BOM visuale e' pronta dopo %.0f ms" % t2["dcl"])
    b.niente_orizzontale()
    print("      documenti pronti in %.0f ms (server %.0f ms), %d righe; BOM in %.0f ms (server %.0f ms)"
          % (t["dcl"], t["server"], righe, t2["dcl"], t2["server"]))


@prova("H", "viste tecniche, «Da verificare» e riepilogo di uno STEP")
def prova_h(b):
    b.apri()
    b.menu("Griglia")
    verifica(b.page.locator("#vista table.griglia").is_visible(), "la griglia non si vede")
    b.foto("06_griglia.png")
    b.menu("Codici")
    verifica(b.page.locator("#cassetto .cassetto-dentro").is_visible(), "il cassetto dei codici non si vede")
    verifica(b.page.locator("#cassetto", has_text="Codici della richiesta").count() == 1, "il cassetto non dice i codici")
    b.foto("07_cassetto_codici.png")
    # il clic vero sul gesto di B8.6: il bottone manda codice (hx-include) e tipo (hx-vals)
    riga = b.page.locator("#cassetto #codice-53099999")
    verifica(riga.count() == 1, "il codice nuovo 53099999 non e' nel cassetto")
    b.clic_e_aspetta(riga.get_by_role("button", name="+ Particolare"), "/fascicolo/codice/aggiungi")
    verifica(b.avviso() == "53099999 entra nella BOM come particolare.", "avviso: %r" % b.avviso())
    verifica("nel Fascicolo" in b.page.locator("#cassetto #codice-53099999").inner_text(), "dopo il gesto la riga non dice «nel Fascicolo»")
    verifica(b.page.locator("#cassetto .cassetto-dentro").is_visible(), "il gesto ha chiuso il cassetto")
    # l'area principale e' ancora la Griglia: il componente nuovo ha la sua riga
    verifica(b.page.locator("#vista table.griglia tr", has_text="53099999").count() == 1, "il particolare aggiunto non e' nella griglia")
    verifica(b.viva(), "la pagina si e' ricaricata")
    # dal cassetto aperto si passa agli avvisi con le sue linguette: la testata e' sotto il cassetto
    b.clic_e_aspetta(b.page.locator("#cassetto .cassetto-linguette a", has_text="Avvisi"), "/fascicolo/parti")
    verifica(b.page.locator("#cassetto .avviso-riga").count() >= 1, "nessun avviso nel cassetto")
    b.foto("08_cassetto_avvisi.png")
    b.clic_e_aspetta(b.page.locator("#cassetto a.chiudi-cassetto"), "/fascicolo/parti")
    verifica(b.page.locator("#cassetto .cassetto-dentro").count() == 0, "il cassetto non si chiude")
    # «Da verificare»: il concetto operativo, dalla testata
    b.clic_e_aspetta(b.page.locator("#fasc-testata a.verifica"), "/fascicolo/parti")
    verifica("cassetto=verifica" in b.page.url, "l'indirizzo non dice il cassetto: %s" % b.page.url)
    verifica(b.page.locator("#cassetto .cassetto-dentro[aria-label='Da verificare']").is_visible(), "il cassetto «Da verificare» non si vede")
    b.foto("09_da_verificare.png")
    b.clic_e_aspetta(b.page.locator("#cassetto a.chiudi-cassetto"), "/fascicolo/parti")
    # lo STEP del banco non ha una proposta di documento: sta fra «Tutti», non fra i non assegnati
    b.clic_e_aspetta(b.page.locator("#fasc-testata .linguette a", has_text="Documenti"), "/fascicolo/parti")
    b.clic_e_aspetta(b.page.locator('.filtri-doc a[data-filtro="tutti"]'), "/fascicolo/parti")
    b.clic_e_aspetta(b.page.locator("#file-%s td.nome a" % b.a.stp), "/fascicolo/anteprima")
    corpo = b.page.locator("#anteprima-corpo")
    verifica("letta per intero" in corpo.inner_text(), "riepilogo dello STEP: %r" % corpo.inner_text()[:200])
    verifica(b.page.locator("#anteprima-corpo iframe").count() == 0, "per uno STEP niente iframe")
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.foto("10_step_riepilogo.png")


@prova("I", "«Rivedi» e «Conferma Fascicolo»: il piano entra con un gesto")
def prova_i(b):
    b.apri()
    piano = b.page.locator("#piano")
    pronti = int(piano.locator(".piano-conta.ok b").inner_text().strip())
    verifica(pronti >= 3, "voci pronte nel piano: %d" % pronti)
    verifica(b.page.locator("#conferma-fascicolo").is_enabled(), "«Conferma Fascicolo» e' spento con %d voci pronte" % pronti)
    b.clic_e_aspetta(piano.get_by_role("link", name="Rivedi"), "/fascicolo/parti")
    caselle = b.page.locator("#cassetto form.rivedi input[type=checkbox]")
    verifica(caselle.count() == pronti, "nel riepilogo %d caselle per %d voci pronte" % (caselle.count(), pronti))
    verifica(all(caselle.nth(i).is_checked() for i in range(caselle.count())), "le voci pronte non sono tutte spuntate")
    verifica(b.page.locator("#cassetto form.rivedi", has_text="assieme.stp").count() == 1, "la struttura dello STEP non e' nel riepilogo")
    b.foto("11_rivedi.png")
    b.clic_e_aspetta(b.page.locator("#cassetto a.chiudi-cassetto"), "/fascicolo/parti")
    b.clic_e_aspetta(b.page.locator("#conferma-fascicolo"), "/fascicolo/conferma")
    verifica(b.viva(), "la pagina si e' ricaricata")
    avviso = b.avviso()
    verifica(avviso.startswith("Fascicolo confermato"), "avviso: %r" % avviso)
    verifica(b.page.locator('[id="proposta-%s-#3"]' % b.a.stp).count() == 0, "la card proposta di 53011111 e' rimasta")
    verifica(b.carta_confermata("53011111", b.a.assieme).count() == 1, "53011111 non e' nato sotto l'assieme")
    verifica(b.page.locator("#conferma-fascicolo").is_disabled(), "dopo la conferma resta qualcosa di pronto: %r" % piano.inner_text())
    b.foto("14_dopo_conferma.png")


def main():
    ap = argparse.ArgumentParser()
    for k in ["url", "thread", "prodotto", "assieme", "particolare", "pdf", "stp", "proposte"]:
        ap.add_argument("--" + k, required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="ABCDEFGHI")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")  # i messaggi hanno ○ ✓ ×: non solo cp1252

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
