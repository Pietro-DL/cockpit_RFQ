"""La pagina della Distinta unica contro il server VERO: i rilievi del collaudo visivo UI46, giro 1 (messaggio 46 [U]).

Lo lancia TestDistintaUnicaUI46ServerVeroNelBrowser (distinta_unica_ui46_server_browser_test.go, tag «integrazione
browser») con il banco web vero. Lo script fa il giro della riunione con i soli gesti della pagina e controlla i rilievi
uno per uno: ogni controllo stampa una riga «CONTROLLO <nome> PASSATO|FALLITO», e un controllo fallito non ferma il giro
(si vede quali rilievi restano). Un gesto che non riesce ferma il giro: i controlli che restano sono NON ESEGUITI, e
l'uscita è 1. I dati sono inventati (ACME, 91xxxxx).
"""

import argparse
import json
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from playwright.sync_api import expect, sync_playwright  # noqa: E402

import distinta_unica as du  # noqa: E402
import distinta_unica_server as dus  # noqa: E402

T_LUNGO = 30000
sid = du.sid

CONTROLLI = ["B1 anteprima", "B1 anteprima 2D", "B1 dialogo S8", "minore esito S8", "I2 qualificatori", "I7 aggiungi un pezzo", "I1 pillola e barra", "I4 riepilogo R4",
             "B3 wizard dopo R5", "B3 ultima mossa", "B2 proposta aggiornata", "B3 sorgente dopo R5", "I5 striscia di un altro prodotto",
             "I3 card del prodotto", "I3 STEP della RFQ", "I5 striscia dell'anteprima", "I6 parti dalla BOM registrata", "I7 versioni del progetto"]


# OSSERVAZIONI: rilievi che il giro dice senza farlo fallire. Erano I4, B3 del wizard e I6, lasciati alle corsie wizard e
# working: nel ramo finale (unite le corsie, collaudo 2) sono controlli come gli altri, e falliscono se restano.
OSSERVAZIONI = set()


class Esiti:
    def __init__(self, b):
        self.b = b
        self.fatti = {}

    def controlla(self, nome, ok, dettaglio="", foto=None):
        assert nome in CONTROLLI, nome
        if nome in OSSERVAZIONI:
            self.fatti[nome] = True
            print("  OSSERVAZIONE %-33s %s%s" % (nome, "chiusa" if ok else "ANCORA VISIBILE (corsia da unire)", "" if ok else "  " + str(dettaglio)[:300]), flush=True)
            if foto:
                self.b.foto(foto)
            return
        self.fatti[nome] = bool(ok)
        print("  CONTROLLO %-36s %s%s" % (nome, "PASSATO" if ok else "FALLITO", "" if ok else "  " + str(dettaglio)[:600]), flush=True)
        if foto:
            self.b.foto(foto)

    def fine(self):
        for n in CONTROLLI:
            if n not in self.fatti:
                print("  CONTROLLO %-36s NON ESEGUITO" % n)
        return all(self.fatti.get(n) for n in CONTROLLI)


def testo(loc):
    try:
        return loc.inner_text(timeout=2000) if loc.count() else ""
    except Exception:
        return ""


def scheda_di(p, codice):
    return p.locator("#du-vp .du-nc[data-k]").filter(has=p.locator(".du-nc-c", has_text=re.compile("^" + re.escape(codice) + "$")))


def giro(b, e):
    p = b.page
    step1 = b.al["9120001.stp"]
    step2 = b.al["9130001.stp"]
    p3 = b.p3

    # ---- B1: l'anteprima dello STEP univoco mostra l'albero proposto
    b.guarda(step1)
    p.wait_for_timeout(1500)
    ant = p.locator("#du-anteprima")
    nodi = ant.locator("[data-k]").count()
    t = testo(ant)
    e.controlla("B1 anteprima", nodi >= 4 and "Scegli una struttura dello STEP per vederla" not in t,
                "nodi nell'anteprima %d; testo: %r" % (nodi, t[:300]), "ui46_b1_anteprima.png")

    # la scheda del sottoassieme con la sua tavola: la miniatura del 2D, come nella pagina di prima (messaggio 46 [U])
    def mini():
        return p.locator("#du-anteprima .du-nc-mini").evaluate_all(
            "(ls) => ls.filter(l => { const n = l.closest('.du-nc').querySelector('.du-nc-c'); return n && n.innerText.trim() === '9120010A'; })"
            ".map(l => [l.dataset.duStato || '', l.innerText.replace(/\\s+/g, ' ').trim()])")
    fine = time.time() + 15
    m = mini()
    while time.time() < fine and not (m and all(x[0] in ("pronta", "chiuso") for x in m)):
        p.wait_for_timeout(300)
        m = mini()
    e.controlla("B1 anteprima 2D", m and all(x[0] in ("pronta", "chiuso") and "nessun 2D" not in x[1] for x in m), "la scheda di 9120010A nell'anteprima: %s" % m, "ui46_b1_anteprima_2d.png")

    # ---- B1: il dialogo di S8 mostra che cosa si conferma
    bt = p.locator("#du-conferma-sorgente").first
    expect(bt).to_be_enabled(timeout=T_LUNGO)
    bt.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(800)
    st = p.locator("#du-finestra #f-struttura")
    n_dialogo = int(st.get_attribute("data-nodi") or "0") if st.count() else 0
    e.controlla("B1 dialogo S8", n_dialogo >= 4 and p.locator("#du-finestra #f-struttura-non-letta").count() == 0,
                "pezzi nel dialogo %d; testo: %r" % (n_dialogo, testo(b.finestra())[:400]), "ui46_b1_dialogo.png")
    # su una pagina di prima il dialogo può chiedere il motivo: lo si scrive, per andare avanti
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova del collaudo")
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    t = testo(esito)
    e.controlla("minore esito S8", "Riuscito: associazione" not in t, "esito di S8: %r" % t[:300])

    # ---- I2: il pannello del nodo
    b.passo("revisione")
    expect(scheda_di(p, "9120010A").first).to_be_visible(timeout=T_LUNGO)
    p.click("#du-zoom-adatta")
    scheda_di(p, "9120010A").first.click()
    expect(p.locator("#du-pannello")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(500)
    corpo = p.locator("body").inner_text()
    e.controlla("I2 qualificatori", "[object Object]" not in corpo, "«[object Object]» nella pagina: %r" % testo(p.locator("#du-pannello"))[:400], "ui46_i2_pannello.png")

    # ---- I7: «Aggiungi un pezzo» senza «UUID» né codici del server
    if p.locator("#du-nuovo-par").count() and p.locator("#du-nuovo-par").is_enabled():
        p.click("#du-nuovo-par")
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        t = testo(b.finestra())
        e.controlla("I7 aggiungi un pezzo", "UUID" not in t and "CODICE_DA_ASSEGNARE" not in t, "dialogo: %r" % t[:300])
        b.chiudi_finestra()

    # ---- I1: dopo «Cambia tipo» la pillola della Revisione e la barra dicono la stessa versione
    scheda_di(p, "9120040A").first.click()
    expect(p.locator("#du-pannello #du-cambia-tipo")).to_be_visible(timeout=T_LUNGO)
    p.click("#du-pannello #du-cambia-tipo")
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    p.select_option("#du-finestra #f-tipo", "commerciale")
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova del collaudo")
    b.ok_finestra()
    sv = p.locator("#du-salvataggio")
    expect(sv).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.wait_for_timeout(700)
    barra, pillola = testo(sv), testo(p.locator("#du-passo-revisione"))
    vb = re.search(r"versione (\d+)", barra)
    vp = re.search(r"versione (\d+)", pillola)
    e.controlla("I1 pillola e barra", vb and vp and vb.group(1) == vp.group(1) and "da salvare" not in pillola,
                "barra %r, pillola %r" % (barra, pillola), "ui46_i1_versione.png")

    # ---- I7: niente versioni del progetto nei testi della pagina (anche nei titoli dei pulsanti spenti dell'Elenco)
    b.vista("elenco")
    p.wait_for_timeout(800)
    html = p.evaluate("() => document.body.innerHTML")
    trovati = sorted(set(re.findall(r"\(V\d\)|in V\d|R10 \(V4\)", html)))
    e.controlla("I7 versioni del progetto", not trovati, "nella pagina: %s" % trovati, "ui46_i7_elenco.png")
    b.vista("org")

    # ---- I4: il riepilogo di R4
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_be_visible(timeout=T_LUNGO)
    t = testo(p.locator("#du-wz-corpo"))
    vietati = [x for x in ("VERIFICA_DOPO_COMMIT", "SORGENTE_NON_AUTORIZZATA", "Firma del riepilogo", "non e'", "da S8", "Criterio che la conferma") if x in t]
    gialli_vuoti = p.locator("#du-wz-corpo .du-esito--ver").evaluate_all("(ls) => ls.filter(l => /nessuno\\s*$/m.test(l.innerText) || /:\\s*nessuno\\b/.test(l.innerText) && !/conflitto/.test(l.innerText)).length")
    e.controlla("I4 riepilogo R4", not vietati and not gialli_vuoti, "nel riepilogo: %s; riquadri gialli vuoti: %s" % (vietati, gialli_vuoti), "ui46_i4_r4.png")

    # ---- R5 e B3: dopo «Conferma BOM»
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-wz-conferma")
    ew = b.esito_wizard()
    expect(ew).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    p.wait_for_timeout(1500)
    t = testo(p.locator("#du-wz-corpo"))
    # collaudo 2, C2-B2: dopo l'esito niente riepilogo di R4 di prima (nemmeno i documenti con «un pezzo senza codice»)
    vietati = [x for x in ("La BOM è confermabile", "Con la conferma", "pezzo senza codice", "Documenti: che cosa registra la conferma") if x in t]
    e.controlla("B3 wizard dopo R5", not vietati and p.locator("#du-wz-fase2").count() == 0, "il wizard dopo l'esito: %s · %r" % (vietati, t[:500]), "ui46_b3_wizard.png")
    chiudi = p.locator("#du-wz-fine, #du-wz-chiudi").first
    chiudi.click()
    expect(p.locator("#du-wizard")).to_be_hidden(timeout=T_LUNGO)

    # ---- B2 (prima della rilettura): nessuna «Proposta aggiornata» subito dopo la conferma
    p.wait_for_timeout(1000)
    pa1 = p.locator("#du-proposta-aggiornata[data-du-proposta=aperta]").count()
    dett = testo(p.locator("#du-proposta-aggiornata"))

    # ---- I5: la striscia della conferma di 9120001A non resta su 9130001A (subito, senza rileggere la pagina)
    p.click(".du-ptab[data-prodotto='%s']" % b.p2)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()
    p.wait_for_timeout(800)
    av = b.avviso()
    t = testo(av) if av.is_visible() else ""
    e.controlla("I5 striscia di un altro prodotto", "9120001A" not in t, "striscia su 9130001A: %r" % t, "ui46_i5_tab.png")
    p.click(".du-ptab[data-prodotto='%s']" % b.p1)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % b.p1)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()

    # ---- B2 (dopo la rilettura)
    b.passo("revisione")
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    p.wait_for_timeout(800)
    pa2 = p.locator("#du-proposta-aggiornata[data-du-proposta=aperta]").count()
    dett = dett or testo(p.locator("#du-proposta-aggiornata"))
    if pa1 or pa2:
        try:
            p.click("#du-pa-differenze")
            dett += " | " + testo(b.finestra())
            b.foto("ui46_b2_differenze.png")
            b.chiudi_finestra()
        except Exception:
            pass
    e.controlla("B2 proposta aggiornata", not pa1 and not pa2, "segno dopo la conferma: %r" % dett[:500], "ui46_b2_revisione.png")

    # ---- B3: la Sorgente dopo la conferma
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    p.wait_for_timeout(800)
    t = testo(p.locator("#du-corpo"))
    vietati = [x for x in ("bozza non creata", "Riprendi: crea la bozza", "Diventa lo STEP della BOM con") if x in t]
    e.controlla("B3 sorgente dopo R5", not vietati, "nella Sorgente: %s" % vietati, "ui46_b3_sorgente.png")
    m = testo(p.locator("#du-sg-mossa-t"))
    e.controlla("B3 ultima mossa", "La BOM non è confermata" not in m, "ultima mossa: %r" % m)

    # ---- passa a 9130001A
    p.click(".du-ptab[data-prodotto='%s']" % b.p2)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()

    # ---- I3: lo STEP proposto senza gruppo
    b.cartella("p:" + b.p2)
    p.wait_for_timeout(800)
    card = testo(p.locator("#du-sg-prodotto"))
    riga = testo(p.locator("#du-sg-c-" + sid("p:" + b.p2)))
    e.controlla("I3 card del prodotto", "non propone nessuno STEP" not in card and "0 STEP" not in riga and "Nessuna candidata per" not in testo(p.locator("#du-corpo")),
                "card %r · riga %r" % (card[:400], riga), "ui46_i3_card.png")
    b.cartella("step")
    p.wait_for_timeout(500)
    e.controlla("I3 STEP della RFQ", p.locator("#du-step-" + sid(step2)).count() == 1,
                "gli STEP della RFQ: %r" % testo(p.locator("#du-sg-elenco"))[:400], "ui46_i3_step.png")

    # ---- I5: la striscia dell'anteprima senza proposta non dice «Proposta dello STEP in vista»
    b.cartella("p:" + b.p2)
    g = p.locator("#du-corpo button", has_text="Guarda l'anteprima per 9130001A").first
    if g.count():
        g.click()
        expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
        p.wait_for_timeout(800)
        ha_albero = p.locator("#du-anteprima [data-k]").count() > 0
        t = testo(b.avviso()) if b.avviso().is_visible() else ""
        e.controlla("I5 striscia dell'anteprima", ha_albero or "Proposta dello STEP in vista" not in t, "striscia %r con l'anteprima senza albero" % t, "ui46_i5_anteprima.png")

    # ---- I6: 9140001A, senza STEP né BOM registrata
    p.click(".du-ptab[data-prodotto='%s']" % p3)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % p3)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    reg = p.locator("#du-da-registrato")
    e.controlla("I6 parti dalla BOM registrata", reg.count() == 0 or reg.is_disabled(), "«Parti dalla BOM registrata» acceso senza BOM registrata", "ui46_i6.png")



def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="AD")
    ap.add_argument("--password", default="prova-ad")
    ap.add_argument("--dati", required=True)
    ap.add_argument("--fase", default="ui46", choices=["ui46"])
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    esito = 0
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1600, "height": 1000})
        page = ctx.new_page()
        errori = []
        page.on("pageerror", lambda x: errori.append("pageerror: " + str(x)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        page.on("dialog", lambda d: d.accept())
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=15000)
        b = dus.BancoVero(page, a)
        b.p3 = json.loads(a.dati).get("prodotto3") or ""
        b.apri()
        e = Esiti(b)
        inizio = time.time()
        try:
            giro(b, e)
        except Exception as x:
            esito = 1
            print("  GIRO interrotto: %s" % str(x).replace("\n", " ")[:600])
            b.foto("ui46_errore.png")
        if not e.fine():
            esito = 1
        if errori:
            print("  errori nella pagina: %s" % errori[:5])
            esito = 1
        print("  giro UI46 %s (%.1fs)" % ("PASSATO" if not esito else "FALLITO", time.time() - inizio))
        browser.close()
    return esito


if __name__ == "__main__":
    sys.exit(main())
