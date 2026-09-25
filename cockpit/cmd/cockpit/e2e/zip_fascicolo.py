# -*- coding: utf-8 -*-
"""L7 - la richiesta con lo ZIP, dall'arrivo della mail al Fascicolo confermato, in un BROWSER VERO (B8.7b).

Non lo si lancia a mano: lo avvia `zip_browser_test.go` (tag `integrazione browser`), che compila e avvia
cockpit.exe con un cockpit.toml temporaneo e il worker Outlook vero con la posta finta. Il worker di analisi
parte solo quando questo script lo chiede (crea il file --segnale): cosi' il Fascicolo e' gia' aperto quando
l'analisi lavora, e l'aggiornamento senza F5 si vede davvero.

I passi seguono la richiesta; ciascuno ha bisogno del precedente, e al primo che fallisce gli altri non si
eseguono:
  1  la mail con lo ZIP arriva nell'Inbox
  2  il cliente si censisce e la RFQ nasce con il codice della richiesta; lo ZIP «si prepara da solo»
  3  il Fascicolo si apre con il prodotto finito gia' card radice, senza «+ Prodotto», e la preparazione in corso
  4  senza F5: le proposte dello STEP arrivano da sole nella gerarchia, poi il poll si ferma
  5  il pannello di destra: il prodotto (con i file in arrivo) e una proposta (con chi la aspetta)
  6  «Da verificare» chiede solo il codice del foglio, gia' suggerito dal nome
  7  «Rivedi» e «Conferma Fascicolo»: un gesto porta nel fascicolo struttura, documenti e STEP strutturale
  8  «Aggiungi file › Importa dal NAS»: la ricerca per codice sotto la radice, poi la strada di tutti

Alla fine stampa una riga «ESITO {json}» con il thread e la cronologia: il test Go la legge e controlla il
database passo per passo.
"""
import argparse
import json
import os
import re
import sys
import time

from playwright.sync_api import sync_playwright

PASSI = []


def passo(nome):
    def deco(fn):
        PASSI.append((nome, fn))
        return fn
    return deco


class Rotto(AssertionError):
    pass


def verifica(condizione, messaggio):
    if not condizione:
        raise Rotto(messaggio)


# gli archi dello STEP (padre, figlio, quantita' sull'arco)
ARCHI = {("52922757", "52920517", "×2"), ("52920517", "53011111", "×2"), ("52920517", "53017189", "×1"), ("52922757", "53017189", "×4")}

# la BOM visuale letta dal DOM: una riga per card, con il padre (la card del li che la contiene)
JS_ALBERO = """() => Array.from(document.querySelectorAll('#tela li.bom-li')).map(li => {
  const carta = li.querySelector(':scope > div.carta');
  const arco = li.querySelector(':scope > span.arco');
  const su = li.parentElement.closest('li.bom-li');
  const padre = su ? su.querySelector(':scope > div.carta .codice') : null;
  const cod = carta ? carta.querySelector('.codice') : null;
  return {codice: cod ? cod.textContent.trim() : '', padre: padre ? padre.textContent.trim() : '',
          qta: arco ? arco.textContent.trim() : '', classe: carta ? carta.className : ''};
})"""


class Banco:
    def __init__(self, page, a):
        self.page, self.a = page, a
        self.base = ""
        self.dati = {}
        self.poll = []  # gli istanti delle richieste del poll dell'avanzamento
        self.t0 = time.time()

    def apri(self, stato=""):
        self.page.goto(self.base + stato)
        self.page.wait_for_load_state("domcontentloaded")
        self.page.evaluate("window.__marca = 'viva'")

    def viva(self):
        return self.page.evaluate("window.__marca") == "viva"

    def clic_e_aspetta(self, locator, pezzo_url):
        with self.page.expect_response(lambda r: pezzo_url in r.url, timeout=20000) as risp:
            locator.click()
        verifica(risp.value.status == 200, "risposta %d da %s" % (risp.value.status, risp.value.url))
        self.page.wait_for_timeout(300)  # htmx: swap e settle
        return risp.value

    def avviso(self):
        el = self.page.locator("#fasc-avviso .avviso-f")
        return el.inner_text().strip() if el.count() else ""

    def menu(self, voce):
        m = self.page.locator("#fasc-testata details.menu:not(.aggiungi)")
        m.locator("summary").click()
        self.clic_e_aspetta(m.locator(".menu-dentro a", has_text=voce), "/fascicolo/parti")

    def chiudi_cassetto(self):
        self.clic_e_aspetta(self.page.locator("#cassetto a.chiudi-cassetto"), "/fascicolo/parti")

    def albero(self):
        return self.page.evaluate(JS_ALBERO)

    def pronti(self):
        return int(self.page.locator("#piano .piano-conta.ok b").inner_text().strip())

    def stato(self):
        av = self.page.locator("#fasc-avanzamento")
        attivo = av.count() and "attivo" in (av.get_attribute("class") or "")
        conf = self.page.locator("#conferma-fascicolo")
        return {
            "s": round(time.time() - self.t0, 1),
            "lavoro": av.inner_text().strip().replace("\n", " ") if attivo else "",
            "carte": self.page.locator("#tela div.carta").count(),
            "proposte": self.page.locator("#tela div.carta.proposta").count(),
            "piano": self.page.locator("#piano").inner_text().strip().replace("\n", " "),
            "conferma": conf.is_enabled() if conf.count() else False,
        }

    def aspetta(self, pronto, secondi, cosa):
        """Aspetta SENZA ricaricare che la pagina arrivi a uno stato; registra ogni cambiamento."""
        storia = [self.stato()]
        fine = time.time() + secondi
        while time.time() < fine:
            self.page.wait_for_timeout(500)
            s = self.stato()
            if {k: v for k, v in s.items() if k != "s"} != {k: v for k, v in storia[-1].items() if k != "s"}:
                storia.append(s)
            if pronto(s):
                return storia
        raise Rotto("%s: dopo %d s la pagina e' ancora %r" % (cosa, secondi, storia[-1]))

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)


@passo("la mail con lo ZIP arriva nell'Inbox (worker Outlook vero, posta finta)")
def passo_1(b):
    riga = b.page.locator("#lista a.riga", has_text="52922757")
    fine = time.time() + 90
    while True:
        b.page.goto(b.a.url + "/inbox")
        b.page.locator(".quadranti a.quadrante.tutti").click()
        b.page.wait_for_timeout(800)
        if riga.count() or time.time() > fine:
            break
        time.sleep(2)
    verifica(riga.count() >= 1, "la mail di ACME non e' arrivata in 90 s")
    b.dati["mail_dopo_s"] = round(time.time() - b.t0, 1)


@passo("il cliente si censisce, la RFQ nasce con il codice, lo ZIP si prepara da solo")
def passo_2(b):
    b.page.locator("#lista a.riga", has_text="52922757").first.click()
    b.page.wait_for_selector("#pannello button:has-text('Censisci come cliente')", timeout=15000)
    b.page.click("#pannello button:has-text('Censisci come cliente')")
    form = "form.form-censisci"
    b.page.wait_for_selector(form, timeout=15000)
    b.page.fill(form + " input[name=ragione_sociale]", "ACME Industrie S.p.A.")
    b.page.fill(form + " input[name=cartella_nas]", "ACME")
    for campo in ["usa_dominio", "usa_contatto"]:
        box = b.page.locator(form + " input[name=%s]" % campo)
        if box.count() and not box.is_checked():
            box.check()
    b.page.click(form + " button[type=submit]")
    b.page.wait_for_selector("#pannello button:has-text('Nuova RFQ')", timeout=15000)
    b.page.click("#pannello button:has-text('Nuova RFQ')")
    b.page.wait_for_selector("form.form-triage", timeout=15000)
    allegati = b.page.locator("form.form-triage fieldset:has(legend:text('Allegati'))").inner_text()
    verifica("RFQ ACME 52922757.zip" in allegati and "si prepara da solo" in allegati, "lo ZIP nel triage: %r" % allegati)
    b.page.locator("form.form-triage input[name=codice][value='52922757']").check()
    b.foto("00_triage.png")
    b.page.click("form.form-triage button[type=submit]")
    b.page.wait_for_selector("#pannello :text('RFQ creata')", timeout=15000)
    avviso = b.page.locator("#pannello .avviso").first.inner_text()
    b.dati["avviso_creazione"] = avviso
    verifica("52922757 nella BOM come prodotto finito" in avviso, "il prodotto finito non nasce con la RFQ: %r" % avviso)
    verifica("1 file utile in preparazione" in avviso, "lo ZIP non parte da solo: %r" % avviso)


@passo("il Fascicolo si apre con il prodotto gia' card radice, senza «+ Prodotto»")
def passo_3(b):
    b.page.goto(b.a.url + "/richieste")
    href = b.page.locator("a[href^='/thread/']").first.get_attribute("href")
    b.dati["thread"] = re.search(r"/thread/([0-9a-f-]{36})", href).group(1)
    b.base = b.a.url + "/thread/" + b.dati["thread"] + "/fascicolo"
    b.apri()
    prodotto = b.page.locator("#tela div.carta.prodotto")
    verifica(prodotto.count() == 1 and "52922757" in prodotto.inner_text(), "la card del prodotto: %r" % prodotto.all_inner_texts())
    struttura = prodotto.locator(".carta-struttura").text_content().strip() if prodotto.locator(".carta-struttura").count() else ""
    b.dati["struttura_all_apertura"] = struttura
    verifica(any(x in struttura for x in ["struttura da definire", "STEP in analisi", "STEP arrivato"]),
             "la card del prodotto non dice come sta la sua struttura: %r" % struttura)
    av = b.page.locator("#fasc-avanzamento.attivo")
    verifica(av.count() == 1 and av.get_attribute("hx-trigger"), "la preparazione in corso non si vede (o non chiede aggiornamenti)")
    b.dati["avanzamento_all_apertura"] = av.inner_text().strip()
    # il codice della richiesta e' gia' nella BOM: il cassetto dei codici non offre «+ Prodotto»
    b.menu("Codici")
    riga = b.page.locator("#cassetto #codice-52922757")
    verifica(riga.count() == 1 and "nel Fascicolo" in riga.inner_text(), "il codice della richiesta nel cassetto: %r" % riga.all_inner_texts())
    verifica(riga.get_by_role("button", name=re.compile(r"^\+")).count() == 0, "il cassetto offre ancora di aggiungere 52922757")
    b.chiudi_cassetto()
    b.page.evaluate("window.__marca = 'viva'")
    b.foto("01_apertura.png")
    # adesso parte il worker di analisi: il Fascicolo resta aperto e non si ricarica piu'
    open(b.a.segnale, "w").close()
    b.t_segnale = time.time()


@passo("senza F5: le proposte dello STEP arrivano nella gerarchia, poi il poll si ferma")
def passo_4(b):
    storia = b.aspetta(lambda s: not s["lavoro"] and s["proposte"] >= 4 and s["conferma"], b.a.attesa, "le proposte")
    b.dati["senza_f5"] = storia
    verifica(b.viva(), "la pagina si e' ricaricata")
    # la pagina era gia' aperta quando le proposte non c'erano: sono arrivate con il poll, non con un F5
    verifica(storia[0]["proposte"] == 0 and storia[0]["lavoro"], "le proposte c'erano gia' all'inizio dell'attesa: %r" % storia[0])
    albero = b.albero()
    b.dati["albero"] = albero
    trovati = {(x["padre"], x["codice"], x["qta"]) for x in albero if x["padre"]}
    verifica(ARCHI <= trovati, "gli archi dello STEP nella BOM: %r" % sorted(trovati))
    verifica(all("proposta" in x["classe"].split() for x in albero if x["padre"]), "sotto il prodotto ci sono card non tratteggiate: %r" % albero)
    verifica(b.page.locator("#tela div.carta.prodotto").count() == 1, "il prodotto non e' piu' una card sola")
    condiviso = b.page.locator("#tela div.carta.proposta", has_text="53017189")
    verifica(condiviso.count() == 2 and condiviso.filter(has_text="un altro padre").count() == 1,
             "53017189, sotto due padri, si disegna una volta e l'altra e' un rimando: %r" % condiviso.all_inner_texts())
    # finito il lavoro il poll si ferma: niente richieste per piu' del suo intervallo massimo
    fine_lavoro = time.time()
    b.page.wait_for_timeout(12000)
    dopo = [t for t in b.poll if t > fine_lavoro + 1.0]
    verifica(not dopo, "senza lavoro il poll continua: %d richieste in 12 s" % len(dopo))
    verifica(b.page.locator("#fasc-avanzamento[hx-trigger]").count() == 0, "l'elemento dell'avanzamento chiede ancora aggiornamenti")
    b.dati["richieste_poll"] = len(b.poll)
    verifica(len(b.poll) >= 1, "il Fascicolo si e' aggiornato senza nessuna richiesta di poll")
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.foto("02_bom_con_proposte.png")


@passo("il pannello di destra: il prodotto con i file in arrivo, una proposta con chi la aspetta")
def passo_5(b):
    b.clic_e_aspetta(b.page.locator("#tela div.carta.prodotto a.carta-link").first, "/fascicolo/anteprima")
    verifica("52922757" in b.page.locator("#scheda-nodo").inner_text(), "il dettaglio del prodotto non si vede")
    arrivo = b.page.locator("#anteprima .in-arrivo").inner_text()
    verifica("52922757.pdf" in arrivo and "52922757.stp" in arrivo, "i file in arrivo per il prodotto: %r" % arrivo)
    b.page.wait_for_timeout(1000)  # il viewer del PDF si disegna dopo lo swap: la fotografia lo aspetta
    b.foto("03_dettaglio_prodotto.png")
    prop = b.page.locator("#tela div.carta.proposta", has_text="52920517").first
    b.clic_e_aspetta(prop.locator("a.carta-link"), "/fascicolo/anteprima")
    verifica("52920517" in b.page.locator("#scheda-proposta").inner_text(), "il dettaglio della proposta non si vede")
    verifica(b.page.locator("#anteprima .in-arrivo", has_text="52920517.pdf").count() == 1, "la proposta non dice che 52920517.pdf la aspetta")
    b.foto("04_dettaglio_proposta.png")
    verifica(b.viva(), "la pagina si e' ricaricata")


@passo("«Da verificare» chiede solo il codice del foglio, gia' suggerito")
def passo_6(b):
    b.clic_e_aspetta(b.page.locator("#fasc-testata a.verifica"), "/fascicolo/parti")
    voci = b.page.locator("#cassetto .verifica-voce")
    b.dati["da_verificare"] = voci.all_inner_texts()
    verifica(voci.count() == 1 and "53017189 foglio 2.pdf" in voci.first.inner_text(), "le voci da verificare: %r" % voci.all_inner_texts())
    codice = voci.first.locator("input[name=codice]")
    verifica(codice.input_value() == "53017189", "il codice suggerito per il foglio: %r" % codice.input_value())
    b.foto("05_da_verificare.png")
    b.clic_e_aspetta(voci.first.get_by_role("button", name="Salva", exact=True), "/decidi")
    b.dati["decisione"] = b.avviso()
    verifica(b.avviso().startswith("53017189 foglio 2.pdf: 2D, codice 53017189"), "avviso: %r" % b.avviso())
    verifica(b.page.locator("#cassetto .verifica-voce").count() == 0, "dopo la decisione restano voci: %r" % b.page.locator("#cassetto .verifica-voce").all_inner_texts())
    verifica(b.viva(), "la pagina si e' ricaricata")


@passo("«Rivedi» e «Conferma Fascicolo»: un gesto per struttura, documenti e STEP strutturale")
def passo_7(b):
    b.chiudi_cassetto()
    verifica(b.pronti() == 7, "voci pronte: %d (%s)" % (b.pronti(), b.page.locator("#piano").inner_text()))
    b.clic_e_aspetta(b.page.locator("#piano").get_by_role("link", name="Rivedi"), "/fascicolo/parti")
    form = b.page.locator("#cassetto form.rivedi")
    testo = form.inner_text()
    b.dati["rivedi"] = testo
    for x in ["52922757.stp", "52922757.pdf", "52920517.pdf", "53017189 foglio 2.pdf", "Capitolato fornitura.pdf"]:
        verifica(x in testo, "%s non e' nel riepilogo" % x)
    caselle = form.locator("input[type=checkbox]")
    verifica(caselle.count() == 7 and all(caselle.nth(i).is_checked() for i in range(caselle.count())), "le caselle del riepilogo: %d" % caselle.count())
    b.foto("06_rivedi.png")
    b.chiudi_cassetto()
    b.clic_e_aspetta(b.page.locator("#conferma-fascicolo"), "/fascicolo/conferma")
    b.dati["conferma"] = b.avviso()
    verifica(b.avviso().startswith("Fascicolo confermato"), "avviso: %r" % b.avviso())
    verifica(b.page.locator("#tela div.carta.proposta").count() == 0, "dopo la conferma restano card proposte")
    albero = b.albero()
    trovati = {(x["padre"], x["codice"], x["qta"]) for x in albero if x["padre"]}
    verifica(ARCHI <= trovati, "gli archi confermati: %r" % sorted(trovati))
    verifica(b.page.locator("#conferma-fascicolo").is_disabled(), "dopo la conferma resta qualcosa di pronto: %r" % b.page.locator("#piano").inner_text())
    # il pannello di destra segue la conferma: la proposta scelta (52920517) e' diventata un componente, e il
    # corpo mostra il suo 2D, non piu' il riepilogo dello STEP con i nodi «aperta»
    corpo = b.page.locator("#anteprima-corpo")
    verifica("aperta" not in corpo.inner_text(), "il corpo del pannello e' quello di prima della conferma: %r" % corpo.inner_text()[:300])
    verifica(b.page.locator("#anteprima-corpo iframe[title='Anteprima di 52920517.pdf']").count() == 1,
             "il corpo del pannello non mostra il 2D del componente scelto: %r" % corpo.inner_text()[:300])
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.page.wait_for_timeout(1000)  # il viewer del PDF si disegna dopo lo swap: la fotografia lo aspetta
    b.foto("07_dopo_conferma.png")


@passo("«Aggiungi file › Importa dal NAS»: cerca per codice, importa, poi la strada di tutti")
def passo_8(b):
    agg = b.page.locator("#fasc-testata details.aggiungi")
    agg.locator("summary").first.click()
    verifica(agg.locator("summary", has_text="Carica dal PC").is_visible(), "«Carica dal PC» non c'e'")
    verifica(agg.locator("a", has_text="Importa dal NAS").is_visible(), "«Importa dal NAS» non c'e'")
    verifica(not b.page.locator("input[type=file]").is_visible(), "il file picker si vede prima di scegliere «Carica dal PC»")
    b.foto("08_aggiungi_file.png")
    b.clic_e_aspetta(agg.locator("a", has_text="Importa dal NAS"), "/fascicolo/parti")
    cerca = b.page.locator("#cassetto form.nas-cerca")
    verifica(cerca.is_visible(), "il cassetto del NAS non si vede")
    b.dati["nas_partenza"] = b.page.locator("#cassetto .nas-briciole").inner_text().strip()
    b.foto("09_nas.png")
    cerca.locator("input[name=nas_cerca]").fill("52922757")
    b.clic_e_aspetta(cerca.get_by_role("button", name="Cerca"), "/fascicolo/nas")
    voce = b.page.locator("#cassetto li.file", has_text="52922757.dxf")
    verifica(voce.count() == 1, "la ricerca sul NAS: %r" % b.page.locator("#cassetto").inner_text()[:400])
    b.foto("10_nas_ricerca.png")
    b.clic_e_aspetta(voce.get_by_role("button", name="Importa"), "/nas/importa")
    b.dati["importa"] = b.avviso()
    verifica("52922757.dxf importato dal NAS" in b.avviso(), "avviso: %r" % b.avviso())
    b.chiudi_cassetto()
    b.dati["dopo_nas"] = b.aspetta(lambda s: not s["lavoro"] and s["conferma"], 60, "il DXF importato")
    verifica(b.pronti() == 1, "dopo l'importazione le voci pronte sono %d" % b.pronti())
    b.clic_e_aspetta(b.page.locator("#conferma-fascicolo"), "/fascicolo/conferma")
    b.dati["conferma_dxf"] = b.avviso()
    verifica(b.avviso().startswith("Fascicolo confermato"), "avviso: %r" % b.avviso())
    b.clic_e_aspetta(b.page.locator("#tela div.carta.prodotto a.carta-link").first, "/fascicolo/anteprima")
    b.clic_e_aspetta(b.page.locator("#anteprima nav.schede a", has_text="DXF"), "/fascicolo/anteprima")
    verifica(b.page.locator("#anteprima ul.det-doc", has_text="52922757.dxf").count() == 1, "il DXF non e' fra i documenti del prodotto")
    verifica(b.viva(), "la pagina si e' ricaricata")
    b.page.wait_for_timeout(1000)  # il viewer del PDF si disegna dopo lo swap: la fotografia lo aspetta
    b.foto("11_finale.png")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--segnale", required=True, help="file da creare quando il Fascicolo e' aperto: il test avvia l'analisi")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--attesa", type=int, default=150)
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    falliti, fatti = [], 0
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        page = browser.new_context(viewport={"width": 1600, "height": 960}).new_page()
        b = Banco(page, a)
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        # una risorsa che risponde con un errore si nomina; la favicon non c'e' di proposito
        page.on("response", lambda r: errori.append("risposta %d: %s" % (r.status, r.url)) if r.status >= 400 and not r.url.endswith("/favicon.ico") else None)
        page.on("request", lambda r: b.poll.append(time.time()) if "/fascicolo/avanzamento" in r.url else None)
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=10000)
        for i, (nome, fn) in enumerate(PASSI, 1):
            if falliti:
                print("  %d  %-78s NON ESEGUITO" % (i, nome))
                continue
            fatti += 1
            inizio = time.time()
            try:
                fn(b)
                print("  %d  %-78s PASSATO   (%.1fs)" % (i, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(i)
                print("  %d  %-78s FALLITO   %s" % (i, nome, e))
                b.foto("errore_passo_%d.png" % i)
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("ESITO " + json.dumps(b.dati, ensure_ascii=False))
    print("\n%d passi nel browser: %d passati, %d falliti" % (fatti, fatti - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
