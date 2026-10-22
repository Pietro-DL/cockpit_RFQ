"""La pagina della Distinta unica contro il server VERO: i rilievi del collaudo visivo UI46, giro 2 (messaggio 46 [U]).

Lo lancia TestDistintaUnicaC2ServerVeroNelBrowser (distinta_unica_c2_server_browser_test.go, tag «integrazione browser»)
con il banco web vero e la scena del collaudo, giro 2: quattro finiti, fra cui 9150001A con lo STEP 9150001A_1.stp dalla
radice muta (la candidatura «incerta»). Ogni controllo stampa una riga «CONTROLLO <nome> PASSATO|FALLITO»; un controllo
fallito non ferma il giro. Un gesto che non riesce ferma il giro: i controlli che restano sono NON ESEGUITI, e l'uscita
è 1. I dati sono inventati (ACME, 91xxxxx).

  a  dalla Revisione senza bozza «Guarda l'anteprima» mostra l'albero, con lo stesso riquadro della Sorgente (C2-B1);
  c  la tela dell'anteprima della Sorgente sta nello spazio visibile e «Adatta» la adatta a quello spazio (C2-I4);
  d  la card «STEP proposto come sorgente» in un posto solo, con un solo «Guarda l'anteprima» (C2-I5);
  e  il tab del prodotto porta con sé il corpo della Sorgente (C2-I6);
  f  la candidata «incerta» dice perché, con il codice senza la revisione (C2-I7);
  g  «Sposta» con la quantità del legame già scritta, e il dialogo che si apre senza far scorrere la pagina;
  s8 confermare come sorgente lo STEP della incerta, con il codice della radice non letto, non chiede mai un motivo e
     non si blocca (il caso di S8 dell'utente, messaggio 46 [U]).
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

CONTROLLI = ["a anteprima dalla Revisione", "a anteprima dalla Revisione, elenco", "f incerta, il perché", "f incerta, il codice senza la rev",
             "c tela nello spazio visibile", "c Adatta sul visibile", "d card in un posto solo", "e tab e corpo del prodotto",
             "g Sposta, quantità scritta", "g Sposta, la pagina non scorre", "s8 incerta senza motivo"]


class Esiti:
    def __init__(self, b):
        self.b = b
        self.fatti = {}

    def controlla(self, nome, ok, dettaglio="", foto=None):
        assert nome in CONTROLLI, nome
        self.fatti[nome] = bool(ok)
        print("  CONTROLLO %-38s %s%s" % (nome, "PASSATO" if ok else "FALLITO", "" if ok else "  " + str(dettaglio)[:600]), flush=True)
        if foto:
            self.b.foto(foto)

    def fine(self):
        for n in CONTROLLI:
            if n not in self.fatti:
                print("  CONTROLLO %-38s NON ESEGUITO" % n)
        return all(self.fatti.get(n) for n in CONTROLLI)


def testo(loc):
    try:
        return loc.inner_text(timeout=2000) if loc.count() else ""
    except Exception:
        return ""


def visibili(p, sel):
    return p.locator(sel).evaluate_all("(ls) => ls.filter(l => l.offsetParent !== null && getComputedStyle(l).visibility !== 'hidden').length")


def rett(loc):
    return loc.evaluate("(l) => { const r = l.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }")


def tab(b, pid):
    p = b.page
    p.click(".du-ptab[data-prodotto='%s']" % pid)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % pid)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()
    p.wait_for_timeout(600)


def giro(b, e):
    p = b.page
    step1 = b.al["9120001.stp"]
    step2 = b.al["9130001.stp"]
    step4 = b.al["9150001A_1.stp"]

    # ---- a: dalla Revisione senza bozza (9120001A non ha né bozza né conferma) l'anteprima dello STEP proposto
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    g = p.locator("#du-nessuna-bozza button", has_text="Guarda l'anteprima").first
    expect(g).to_be_visible(timeout=T_LUNGO)
    g.click()
    p.wait_for_timeout(2500)
    ant = p.locator("#du-corpo #du-anteprima")
    nodi = ant.locator("#du-vp [data-k]").count() if ant.count() else 0
    e.controlla("a anteprima dalla Revisione", ant.count() == 1 and nodi >= 4 and "du-riq--cand" in (ant.get_attribute("class") or ""),
                "anteprima %d, nodi %d; la Revisione: %r" % (ant.count(), nodi, testo(p.locator("#du-corpo"))[:400]), "c2_a_revisione_org.png")
    b.vista("elenco")
    p.wait_for_timeout(800)
    righe = p.locator("#du-corpo #du-anteprima [data-k]").count()
    e.controlla("a anteprima dalla Revisione, elenco", righe >= 4, "righe dell'anteprima nell'Elenco: %d" % righe, "c2_a_revisione_elenco.png")
    b.vista("org")
    if p.locator("#du-anteprima-chiudi").count():
        p.click("#du-anteprima-chiudi")
        expect(p.locator("#du-anteprima")).to_have_count(0, timeout=T_LUNGO)

    # ---- f: la candidata «incerta» di 9150001A (radice muta, il nome dice 9150001A_1)
    b.passo("sorgente")
    b.cartella("step")
    p.locator("#du-step-%s .du-fr-nome" % sid(step4)).first.click()
    expect(p.locator("#du-sg-file")).to_have_attribute("data-allegato", step4, timeout=T_LUNGO)
    p.wait_for_timeout(600)
    riga = testo(p.locator("#du-step-" + sid(step4)))
    det = testo(p.locator("#du-sg-dettaglio"))
    tutto = riga + "\n" + det
    print("      riga della incerta: %r" % riga[:300])
    print("      dettaglio della incerta: %r" % det[:600])
    perche = p.locator("[data-du-incerta]")
    pt = " | ".join(perche.all_inner_texts()) if perche.count() else ""
    e.controlla("f incerta, il perché", "incerta" in tutto and perche.count() >= 1 and "nome del file" in pt,
                "il perché della incerta: %r" % (pt or tutto[:400]), "c2_f_incerta.png")
    e.controlla("f incerta, il codice senza la rev", "codice letto 9150001A_1" not in tutto and "9150001A_1 (" not in tutto,
                "il codice letto con la rev: %r" % tutto[:500])

    # ---- c: la tela dell'anteprima nella Sorgente sta nel riquadro che si vede, «Adatta» la adatta a quello
    b.guarda(step1)
    p.wait_for_timeout(1500)
    det = p.locator("#du-sg-dettaglio")
    det.evaluate("(d) => { d.scrollTop = 0; }")
    p.wait_for_timeout(300)
    vp = p.locator("#du-anteprima #du-vp")
    expect(vp).to_be_visible(timeout=T_LUNGO)
    rd, rv = rett(det), rett(vp)
    e.controlla("c tela nello spazio visibile", rv[3] <= rd[3] + 1 and rv[1] >= rd[1] - 1,
                "tela %s, riquadro che scorre %s" % ([round(x) for x in rv], [round(x) for x in rd]), "c2_c_tela.png")
    p.click("#du-zoom-adatta")
    p.wait_for_timeout(500)
    rv = rett(vp)
    vis = [max(rv[0], rd[0]), max(rv[1], rd[1]), min(rv[2], rd[2]), min(rv[3], rd[3])]
    fuori = p.locator("#du-anteprima #du-vp .du-nc[data-k]").evaluate_all(
        "(ls, v) => ls.map(l => { const r = l.getBoundingClientRect(); return [l.dataset.k, r.left, r.top, r.right, r.bottom]; })"
        ".filter(x => x[1] < v[0] - 2 || x[2] < v[1] - 2 || x[3] > v[2] + 2 || x[4] > v[3] + 2).map(x => x[0])", vis)
    n = p.locator("#du-anteprima #du-vp .du-nc[data-k]").count()
    e.controlla("c Adatta sul visibile", n >= 4 and not fuori, "schede %d, fuori dal visibile dopo «Adatta»: %s" % (n, fuori), "c2_c_adatta.png")
    p.click("#du-anteprima-chiudi")
    expect(p.locator("#du-anteprima")).to_have_count(0, timeout=T_LUNGO)

    # ---- d: nella cartella del prodotto, con lo STEP proposto scelto, la card è una sola e «Guarda l'anteprima» uno solo
    b.cartella("p:" + b.p1)
    riga1 = p.locator("#du-sg-elenco [id$='%s'] .du-fr-nome" % sid(step1)).first
    if riga1.count():
        riga1.click()
        expect(p.locator("#du-sg-file")).to_have_attribute("data-allegato", step1, timeout=T_LUNGO)
    p.wait_for_timeout(600)
    card = visibili(p, "#du-corpo .du-step-proposto[data-prodotto='%s'], #du-corpo .du-stato-sorgente[data-prodotto='%s']" % (b.p1, b.p1))
    guarda = p.locator("#du-corpo button:visible", has_text="Guarda l'anteprima per 9120001A").count()
    e.controlla("d card in un posto solo", card == 1 and guarda == 1, "card %d, «Guarda l'anteprima per 9120001A» %d" % (card, guarda), "c2_d_card.png")

    # ---- e: il tab di un altro prodotto porta con sé il corpo: niente dettaglio di un file dell'altro prodotto
    b.cartella("tutti")
    p.locator("#du-file-%s .du-fr-nome, #du-step-%s .du-fr-nome" % (sid(step1), sid(step1))).first.click()
    expect(p.locator("#du-sg-file")).to_have_attribute("data-allegato", step1, timeout=T_LUNGO)
    tab(b, b.p2)
    scelta = p.locator("#du-sg-albero [aria-selected=true]").first
    k = scelta.get_attribute("id") or ""
    f = p.locator("#du-sg-file")
    fa = f.get_attribute("data-allegato") if f.count() else ""
    dt = testo(p.locator("#du-sg-dettaglio"))
    e.controlla("e tab e corpo del prodotto", k == "du-sg-c-" + sid("p:" + b.p2) and fa in ("", None, step2) and "9120001A" not in dt,
                "cartella %r, file nel dettaglio %r (%r)" % (k, fa, dt[:300]), "c2_e_tab.png")
    tab(b, b.p1)

    # ---- g: S8 su 9120001A, poi «Sposta sotto…» di 9120020A (×4 sotto 9120010A)
    b.guarda(step1)
    bt = p.locator("#du-conferma-sorgente").first
    expect(bt).to_be_enabled(timeout=T_LUNGO)
    bt.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    b.passo("revisione")
    sch = p.locator("#du-vp .du-nc[data-k]").filter(has=p.locator(".du-nc-c", has_text=re.compile(r"^9120020A$"))).first
    expect(sch).to_be_visible(timeout=T_LUNGO)
    p.click("#du-zoom-adatta")
    sch.click()
    expect(p.locator("#du-pannello #du-sposta")).to_be_visible(timeout=T_LUNGO)
    p.evaluate("() => window.scrollTo(0, 120)")
    p.wait_for_timeout(300)
    y0 = p.evaluate("() => window.scrollY")
    p.click("#du-pannello #du-sposta")
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(500)
    y1 = p.evaluate("() => window.scrollY")
    q = p.locator("#du-finestra #f-qta").input_value()
    e.controlla("g Sposta, quantità scritta", q == "4", "quantità nel dialogo: %r" % q, "c2_g_sposta.png")
    e.controlla("g Sposta, la pagina non scorre", abs(y1 - y0) <= 1, "scrollY da %s a %s" % (y0, y1))
    b.chiudi_finestra()

    # ---- s8: lo STEP della incerta (la radice non porta il codice: «codice letto dalla radice: non letto») confermato
    # come sorgente di 9150001A: nessun motivo chiesto, la conferma parte e riesce
    b.passo("sorgente")
    b.cartella("step")
    p.locator("#du-step-%s .du-fr-nome" % sid(step4)).first.click()
    expect(p.locator("#du-sg-file")).to_have_attribute("data-allegato", step4, timeout=T_LUNGO)
    bc = p.locator("#du-sg-dettaglio .du-step-proposto[data-prodotto='%s'] button" % b.p4, has_text="Conferma questo STEP").first
    expect(bc).to_be_enabled(timeout=T_LUNGO)
    bc.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(800)
    motivo = p.locator("#du-finestra textarea, #du-finestra #f-motivo").count()
    ok = p.locator("#du-finestra #f-ok")
    acceso = ok.count() == 1 and ok.is_enabled()
    td = testo(b.finestra())
    esito = ""
    if acceso and not motivo:
        ok.click()
        try:
            expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
            esito = "completata"
        except Exception as x:
            esito = "non completata: " + " ".join(str(x).split())[:200]
    e.controlla("s8 incerta senza motivo", not motivo and acceso and esito == "completata",
                "motivo chiesto: %d, conferma accesa: %s (%r), esito: %r; dialogo: %r" % (motivo, acceso, ok.get_attribute("title") if ok.count() else "", esito, td[:300]),
                "c2_s8_incerta.png")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="AD")
    ap.add_argument("--password", default="prova-ad")
    ap.add_argument("--dati", required=True)
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
        page.on("pageerror", lambda x: errori.append("pageerror: " + str(x) + " | " + " ".join(str(getattr(x, "stack", "") or "").split()[:40])))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        page.on("dialog", lambda d: d.accept())
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=15000)
        b = dus.BancoVero(page, a)
        b.p4 = json.loads(a.dati).get("prodotto4") or ""
        b.apri()
        e = Esiti(b)
        inizio = time.time()
        try:
            giro(b, e)
        except Exception as x:
            esito = 1
            print("  GIRO interrotto: %s" % str(x).replace("\n", " ")[:600])
            b.foto("c2_errore.png")
        if not e.fine():
            esito = 1
        if errori:
            print("  errori nella pagina: %s" % errori[:5])
            esito = 1
        print("  giro C2 %s (%.1fs)" % ("PASSATO" if not esito else "FALLITO", time.time() - inizio))
        browser.close()
    return esito


if __name__ == "__main__":
    sys.exit(main())
