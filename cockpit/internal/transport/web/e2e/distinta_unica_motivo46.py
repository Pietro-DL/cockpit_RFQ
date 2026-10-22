# -*- coding: utf-8 -*-
"""Messaggio 46 dell'utente (21/10) [U], corsia MOTIVO: «la conferma con motivo non mi piace, anzi dobbiamo rimuoverlo da
ogni widget in cui appare». La prova DU-M46 apre, sul banco finto di distinta_unica_banco_test.go (dati inventati: ACME,
ACM-xxxx), i dialoghi della Distinta unica che chiedevano un motivo all'utente e controlla, dialogo per dialogo:
  - nessun campo del motivo (nessuna textarea, nessuna scelta «Motivo (codice)», nessuna etichetta «Motivo»);
  - il pulsante del gesto acceso senza scrivere niente (prima era spento con «scrivi il motivo»);
  - il corpo mandato al server senza il motivo (dove il gesto parte);
  - nessun testo che chiede un motivo («serve un motivo», «Scrivi il motivo», «rifiuto motivato», «decisione motivata»).
I difetti: D01 (la conferma della sorgente), D02 (il codice della radice diverso «serve un motivo»), D03 («Correggi codice,
rev e descrizione»), D04 (gli altri dialoghi e le etichette: revoca, non includere, escludi, fissa l'emittente, abbandona,
scarta, metti in Altri file, il rifiuto «motivato» della candidata).
Revisione della corsia: R2 il motivo registrato non si MOSTRA (pannello del nodo non incluso, wizard «Rivedi e conferma BOM»
nel gruppo «Non inclusi», dettaglio del file scartato: chi e quando, mai la traccia «gesto di …»); R3 il dialogo della
conferma della sorgente dice che il codice letto dal file (il banco lo mette diverso per ACM-1000_rev2.stp) non è quello
del prodotto e che il documento prende quello del prodotto. Ogni dialogo si fotografa (--foto) PRIMA dei controlli:
sul codice di prima (342010f) le foto sono il «prima», qui il «dopo». I controlli non si fermano al primo che non passa:
li raccoglie tutti e la prova fallisce alla fine con l'elenco.

La lancia TestDistintaUnicaMotivo46NelBrowser (distinta_unica_motivo46_browser_test.go, tag «integrazione browser»).
"""
import argparse
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from playwright.sync_api import expect, sync_playwright  # noqa: E402

import distinta_unica as du  # noqa: E402

T_LUNGO = du.T_LUNGO
sid = du.sid
UUID_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
TESTI_VIETATI = ("serve un motivo", "Scrivi il motivo", "scrivi il motivo", "rifiuto motivato", "decisione motivata", "con il motivo (fino a 500")


class Esito:
    def __init__(self):
        self.difetti = []

    def controlla(self, condizione, messaggio):
        if not condizione:
            self.difetti.append(messaggio)
            print("      NO: " + messaggio)
        return condizione


def senza_motivo(b, e, nome, foto, ok_acceso=True):
    """I controlli comuni di un dialogo aperto: la foto, poi nessun campo né testo del motivo, il pulsante acceso."""
    p = b.page
    fin = b.finestra()
    expect(fin).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(300)
    b.foto(foto)
    e.controlla(fin.locator("textarea").count() == 0, "%s: c'è ancora un campo di testo (il motivo)" % nome)
    e.controlla(fin.locator("#f-motivo, #f-motivo-codice, #f-motivo-riconosci").count() == 0, "%s: c'è ancora il campo del motivo" % nome)
    etichette = [t.strip() for t in fin.locator(".du-campo > span, label > span, .du-campo-t").all_inner_texts()]
    e.controlla(not [t for t in etichette if t.lower().startswith("motivo")], "%s: un'etichetta «Motivo»: %s" % (nome, etichette))
    testo = fin.inner_text()
    for v in TESTI_VIETATI:
        e.controlla(v not in testo, "%s: il testo chiede ancora un motivo («%s»)" % (nome, v))
    if ok_acceso:
        ok = p.locator("#du-finestra #f-ok")
        acceso = True
        try:
            expect(ok).to_be_enabled(timeout=2500)
        except Exception:
            acceso = False
        e.controlla(acceso, "%s: il pulsante del gesto è spento senza un motivo (%r)" % (nome, ok.get_attribute("title")))
        return acceso
    return True


def chiudi(b):
    p = b.page
    if b.finestra().is_visible():
        p.keyboard.press("Escape")
        try:
            expect(b.finestra()).to_be_hidden(timeout=3000)
        except Exception:
            pass


def motivo_mostrato(b, e):
    """R2 della revisione: dopo «Non includere» il pannello del nodo e il wizard (gruppo «Non inclusi») non mostrano il
    motivo (né «Motivo», né «altro ·», né la traccia «gesto di …»), ma chi ha deciso."""
    p = b.page
    # integrazione UI46: la pagina si riapre, così pannello e wizard leggono gli eventi da R1, con l'autore come lo dà il
    # server (l'identificativo dell'utente), e non quelli appena salvati da questa scheda (ancora senza autore)
    b.apri()
    b.passo("revisione")
    expect(p.locator("#du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    nc = p.locator('#du-vp .du-nc[data-du-stato="non incluso (bozza)"]')
    if not e.controlla(nc.count() >= 1, "nessun nodo non incluso dopo «Non includere»: il pannello e il wizard non si controllano"):
        return
    p.click("#du-zoom-adatta")
    nc.first.click()
    sez = p.locator("#du-sez-decisione")
    expect(sez).to_be_visible(timeout=T_LUNGO)
    b.foto("M46_11a_pannello_non_incluso.png")
    t = sez.inner_text()
    e.controlla("Motivo" not in t and "gesto di" not in t and "altro ·" not in t, "il pannello del nodo non incluso mostra il motivo: %r" % t)
    e.controlla(p.locator("#du-noninc-deciso").count() == 1 and "deciso" in p.locator("#du-noninc-deciso").inner_text(), "il pannello del nodo non incluso non dice chi ha deciso: %r" % t)
    # integrazione UI46 (revisione della corsia MOTIVO, rilievo 1): R1 dà l'autore come identificativo dell'utente (il banco
    # ora come il server vero): a schermo la sigla di chi guarda, mai l'identificativo
    dd = p.locator("#du-noninc-deciso").inner_text() if p.locator("#du-noninc-deciso").count() else ""
    e.controlla(not UUID_RE.search(t) and "deciso da AD" in dd, "il pannello del nodo non incluso mostra l'identificativo dell'utente, non la sigla: %r" % t)
    b.apri_wizard()
    p.click("#du-wz-g-noninc")
    tab = p.locator("#du-wz-noninclusi")
    expect(tab).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(300)
    b.foto("M46_11b_wizard_non_inclusi.png")
    intest = [x.strip().lower() for x in tab.locator("thead th").all_inner_texts()]
    t = tab.inner_text()
    e.controlla("motivo" not in intest, "il wizard ha ancora la colonna «Motivo»: %s" % intest)
    e.controlla("gesto di" not in t and "altro ·" not in t and "ancoraggio." not in t, "il wizard mostra il motivo o la traccia: %r" % t)
    e.controlla("deciso" in t, "il wizard non dice chi ha deciso il non incluso: %r" % t)
    e.controlla(not UUID_RE.search(t) and "deciso da AD" in t, "il wizard mostra l'identificativo dell'utente, non la sigla: %r" % t)
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden(timeout=T_LUNGO)


def m46(b, e):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    a10 = b.al["ACM-1000_rev2.stp"]
    # D01 + D02: la conferma della sorgente con il codice della radice diverso (lo STEP rev 2: radice ACM-1000-R2)
    b.guarda(a10)
    expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    senza_motivo(b, e, "conferma della sorgente con la radice diversa", "M46_01_conferma_sorgente_radice_diversa.png")
    rd = p.locator("#f-radice-diversa")
    if e.controlla(rd.count() == 1, "conferma della sorgente: la nota del codice della radice diverso non c'è"):
        e.controlla("ACM-1000-R2" in rd.inner_text() and "serve un motivo" not in rd.inner_text(), "la nota della radice diversa: %r" % rd.inner_text())
    # R3: il codice letto dal file (ACM-1000-R2) non è quello del prodotto: il dialogo dice che il documento prende ACM-1000
    cl = p.locator("#du-finestra #f-codice-letto")
    if e.controlla(cl.count() == 1, "conferma della sorgente: il dialogo non dice che il codice letto dal file non è quello del prodotto"):
        t = cl.inner_text()
        e.controlla("ACM-1000-R2" in t and "il documento prende il codice del prodotto (ACM-1000)" in t, "la nota del codice letto dal file: %r" % t)
    chiudi(b)
    e.controlla(not b.richieste_da(i, "POST"), "chiudere il dialogo della conferma ha scritto")
    # D01: la conferma della sorgente che parte, senza motivo nel corpo
    b.guarda(a1)
    expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    # R3: il codice letto dal file è quello del prodotto: nessuna nota
    e.controlla(p.locator("#du-finestra #f-codice-letto").count() == 0, "conferma della sorgente: la nota del codice letto diverso con il codice uguale")
    if senza_motivo(b, e, "conferma della sorgente", "M46_02_conferma_sorgente.png"):
        b.ok_finestra()
        expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
        c = b.corpi(i, "POST", r"/conferma-sorgente$")
        e.controlla(len(c) == 1 and "motivo" not in c[0], "corpo di S8: %s" % c)
    else:
        chiudi(b)
    # D03: «Correggi codice, rev e descrizione» di un pezzo importato
    if p.locator("#du-vai-revisione").count():
        p.click("#du-vai-revisione")
    else:
        b.passo("revisione")
    expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
    b.vista("org")
    b.scheda("k-12")
    i = b.segna()
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1130")
    if senza_motivo(b, e, "Correggi codice, rev e descrizione", "M46_03_correggi_codice.png"):
        b.ok_finestra()
        b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "«Correggi» non ha salvato")
        dec = [a["dopo"] for a in b.corpi(i, "POST", r"/motore-a/bozza$")[-1]["azioni"] if a["azione"] == "decisione_aggiunta"]
        e.controlla(len(dec) == 1 and dec[0]["modo"] == "correzione" and "motivo" not in dec[0], "la correzione dell'identità: %s" % dec)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    else:
        chiudi(b)
    # D04: «Non includere nella BOM…» dal pannello del nodo: nessuna scelta del «motivo (codice)», nessun motivo
    b.scheda("k-33")
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-non-includere-nodo")
    if senza_motivo(b, e, "Non includere nella BOM", "M46_04_non_includere.png"):
        b.ok_finestra()
        b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "«Non includere» non ha salvato")
        dec = [a["dopo"] for a in b.corpi(i, "POST", r"/motore-a/bozza$")[-1]["azioni"] if a["azione"] == "decisione_aggiunta"]
        atteso = "elemento_cad" if dec and (dec[0].get("segnale_visto") or {}).get("codice") == "ancoraggio.possibile_elemento_cad" else "altro"
        e.controlla(len(dec) == 1 and dec[0]["motivo_codice"] == atteso and dec[0]["motivo"].startswith("gesto ")
                    and "da «Non includere nella BOM»" in dec[0]["motivo"], "la decisione non_includere (motivo_codice dal segnale visto e la traccia del gesto): %s" % dec)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    else:
        chiudi(b)
    motivo_mostrato(b, e)
    # D04: «Escludi (senza cascata)…» del registrato e «Fissa l'emittente…» della radice: solo il dialogo (niente scritto)
    b.scheda("k-40")
    b.linguetta("dettaglio")
    if p.locator("#du-escludi").count() and p.locator("#du-escludi").is_enabled():
        p.click("#du-escludi")
        ar = p.locator("#f-escludi-ar").count() > 0
        senza_motivo(b, e, "Escludi", "M46_05_escludi.png", ok_acceso=not ar)
        chiudi(b)
    b.scheda("k-root")
    b.linguetta("dettaglio")
    if p.locator("#du-fissa-emittente").count() and p.locator("#du-fissa-emittente").is_enabled():
        p.click("#du-fissa-emittente")
        senza_motivo(b, e, "Fissa l'emittente", "M46_06_fissa_emittente.png")
        chiudi(b)
    # D04: «Abbandona la bozza…»: il motivo facoltativo non c'è più
    if p.locator("#du-abbandona").count() and p.locator("#du-abbandona").is_enabled():
        p.click("#du-abbandona")
        senza_motivo(b, e, "Abbandona la bozza", "M46_07_abbandona.png")
        chiudi(b)
    # D04: la Sorgente: «Revoca come sorgente…» (si revoca davvero: il corpo senza motivo), «Scarta…», «Metti in Altri file…»
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-revoca-sorgente").count() == 0:
        p.click("#du-sg-prodotto-apri")
    i = b.segna()
    p.click("#du-revoca-sorgente")
    if senza_motivo(b, e, "Revoca come sorgente", "M46_08_revoca.png"):
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("Sorgente revocata per ACM-1000", timeout=T_LUNGO)
        rv = b.corpi(i, "POST", r"/revoca-sorgente$")
        e.controlla(len(rv) == 1 and "motivo" not in rv[0] and rv[0].get("sorgente_errata") is True, "corpo della revoca: %s" % rv)
    else:
        chiudi(b)
    scarta = b.al["SCHEMA_GENERALE.pdf"]
    i = b.segna()
    b.azione("scarta", scarta)
    if senza_motivo(b, e, "Scarta", "M46_09_scarta.png"):
        b.ok_finestra()
        b.attendi(lambda: b.corpi(i, "POST", r"/scarta$"), "«Scarta» non è partito")
        s2 = b.corpi(i, "POST", r"/scarta$")
        e.controlla(len(s2) == 1 and "motivo" not in s2[0], "corpo di «Scarta»: %s" % s2)
        expect(b.avviso()).to_contain_text("Scart", timeout=T_LUNGO)
        # R2: la riga e il dettaglio del file scartato dicono chi e quando, non la traccia del gesto (il motivo della riga)
        b.cartella("scartati")
        riga = p.locator("#du-file-" + sid(scarta))
        expect(riga).to_have_attribute("data-stato", "scartato", timeout=T_LUNGO)
        p.locator("#du-file-%s .du-fr-nome" % sid(scarta)).first.click()
        p.wait_for_timeout(300)
        b.foto("M46_12_file_scartato.png")
        e.controlla("Scartato da AD il" in riga.inner_text(), "la riga dello scartato non dice chi e quando: %r" % riga.inner_text())
        testo = p.locator("#du-sg-elenco").inner_text() + " " + p.locator("#du-sg-dettaglio").inner_text()
        e.controlla("gesto di" not in testo and "«Scarta»:" not in testo, "il file scartato mostra la traccia del gesto: %r" % testo[:400])
        b.cartella("tutti")
    else:
        chiudi(b)
    b.azione("altri", b.al["PIASTRA_400123.dxf"])
    senza_motivo(b, e, "Metti in Altri file", "M46_10_altri_file.png")
    chiudi(b)
    # D04: l'etichetta del rifiuto della candidata non promette un rifiuto «motivato»
    e.controlla("rifiuto motivato" not in p.content(), "la pagina parla ancora di un rifiuto «motivato»")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    expect.set_options(timeout=du.T_BREVE)
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(viewport={"width": 1440, "height": 900}, locale="it-IT")
        page = ctx.new_page()
        page.set_default_timeout(T_LUNGO)
        errori = []
        page.on("pageerror", lambda x: errori.append("pageerror: " + str(x)))
        page.on("dialog", lambda d: (errori.append("dialogo del browser inatteso: " + d.message), d.dismiss()))
        b = du.Banco(page, a)
        b.apri()
        e = Esito()
        inizio = time.time()
        nome = "M46 nessun motivo nei dialoghi né nei widget della Distinta unica (D01-D04; revisione R2, R3)"
        try:
            m46(b, e)
        except Exception as x:
            e.difetti.append("interrotta: " + (str(x).splitlines()[0] if str(x) else repr(x)))
            b.foto("M46_errore.png")
        if errori:
            e.difetti.append("errori nella pagina: %s" % errori)
        browser.close()
    if e.difetti:
        print("  DU-M46  %-92s FALLITA  %d difetti" % (nome, len(e.difetti)))
        for d in e.difetti:
            print("         - " + d)
        return 1
    print("  DU-M46  %-92s ESEGUITA PASSATA  (%.1fs)" % (nome, time.time() - inizio))
    return 0


if __name__ == "__main__":
    sys.exit(main())
