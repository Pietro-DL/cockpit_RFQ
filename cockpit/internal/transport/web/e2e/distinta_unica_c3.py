"""La pagina della Distinta unica contro il server VERO: i rilievi del collaudo visivo UI46, giro 3 (messaggio 46 [U]).

Lo lancia TestDistintaUnicaC3ServerVeroNelBrowser (distinta_unica_c3_server_browser_test.go, tag «integrazione browser»)
con il banco web vero e la scena del giro 2 più 9170001A, il cui STEP 9170001A_1.stp ha la radice 9170099A (diversa dal
prodotto). Ogni controllo stampa una riga «CONTROLLO <nome> PASSATO|FALLITO»; un controllo fallito non ferma il giro. Un
gesto che non riesce ferma il giro: i controlli che restano sono NON ESEGUITI, e l'uscita è 1. I dati sono inventati
(ACME, 91xxxxx).

  b1  il pulsante principale della card di uno STEP proposto senza struttura importabile (9170001A, radice diversa;
      9130001A, radice senza marcatore) apre il dialogo «solo prodotto», senza motivo, e la conferma riesce (F3-B1);
  i1  «Aggiorna proposta…» spento con il perché finché il server non dà il confronto; «Riparti dalla proposta» non lo
      suggerisce (F3-I1);
  i2  le diagnostiche del motore nel pannello del nodo stanno in «Dettagli tecnici», chiuse (F3-I2);
  i3  la BOM confermata (working) senza il «?» e il tratteggio del tipo proposto (F3-I3);
  i4  la card e l'anteprima dello STEP con la radice diversa dicono la radice letta (F3-I4);
  i5  l'anteprima della Sorgente e l'organigramma della Revisione si aprono già adattati al riquadro (F3-I5);
  f4  dalla Revisione senza bozza «Guarda l'anteprima» porta l'anteprima in cima alla finestra (il bordo alto fra 0 e
      200 px) con l'albero in vista, o adattato alla parte che si vede, a 1600x1000 e a 1366x768 (collaudo finale 4, F4-I1).
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

CONTROLLI = ["i4 card con la radice letta", "i4 anteprima con la radice letta", "b1 card radice diversa, dialogo",
             "b1 card radice diversa, conferma", "b1 card senza struttura, dialogo", "i5 anteprima adattata all'apertura",
             "i1 Aggiorna proposta spento", "i5 organigramma adattato all'apertura", "i2 diagnostiche nella bozza",
             "i1 Riparti senza rimando", "i3 working senza tipo proposto", "i2 diagnostiche nella working",
             "m una sola conferma nella Revisione", "m R4 senza effetto ripetuto",
             "f4 anteprima in vista 1600x1000", "f4 anteprima in vista 1366x768"]

# le chiavi interne del motore che non vanno a schermo: selettori (radice_step.nome) e id delle regole fra virgolette
RE_CHIAVI = re.compile(r"\b[a-z]+_[a-z_]+\.[a-z_]+\b|\"[a-z]+-[a-z-]+\"")


class Esiti:
    def __init__(self, b):
        self.b = b
        self.fatti = {}

    def controlla(self, nome, ok, dettaglio="", foto=None):
        assert nome in CONTROLLI, nome
        self.fatti[nome] = bool(ok)
        print("  CONTROLLO %-40s %s%s" % (nome, "PASSATO" if ok else "FALLITO", "" if ok else "  " + str(dettaglio)[:700]), flush=True)
        if foto:
            self.b.foto(foto)

    def fine(self):
        for n in CONTROLLI:
            if n not in self.fatti:
                print("  CONTROLLO %-40s NON ESEGUITO" % n)
        return all(self.fatti.get(n) for n in CONTROLLI)


def testo(loc):
    try:
        return loc.inner_text(timeout=2000) if loc.count() else ""
    except Exception:
        return ""


def rett(loc):
    return loc.evaluate("(l) => { const r = l.getBoundingClientRect(); return [r.left, r.top, r.right, r.bottom]; }")


def fuori_dal_riquadro(p, vp_sel, riq=None):
    """Le schede della tela vp_sel che escono (oltre 2 px) dal riquadro che si vede: la tela stessa, tagliata dal
    riquadro riq che scorre (se c'è) e dalla finestra."""
    return p.evaluate("""([vs, rs]) => {
        const vp = document.querySelector(vs);
        if (!vp) return ['tela assente'];
        let r = vp.getBoundingClientRect();
        let v = [r.left, r.top, r.right, r.bottom];
        if (rs) { const q = document.querySelector(rs); if (q) { const x = q.getBoundingClientRect(); v = [Math.max(v[0], x.left), Math.max(v[1], x.top), Math.min(v[2], x.right), Math.min(v[3], x.bottom)]; } }
        v = [v[0], v[1], Math.min(v[2], innerWidth), Math.min(v[3], innerHeight)];
        return [...vp.querySelectorAll('.du-nc[data-k]')].map(l => { const b = l.getBoundingClientRect(); return [l.dataset.k, b.left, b.top, b.right, b.bottom]; })
          .filter(x => x[1] < v[0] - 2 || x[2] < v[1] - 2 || x[3] > v[2] + 2 || x[4] > v[3] + 2).map(x => x[0] + '@' + x.slice(1).map(Math.round).join(','));
    }""", [vp_sel, riq])


def anteprima_in_vista(b, e, larg, alt):
    """F4-I1 (collaudo finale 4): dalla Revisione senza bozza, a larg x alt, il clic su «Guarda l'anteprima» porta
    #du-rev-anteprima in cima alla finestra (il bordo alto fra 0 e 200 px); la tela mostra la radice e i figli, oppure lo
    zoom è quello che «Adatta» (tasto 0) darebbe adesso."""
    p = b.page
    nome = "f4 anteprima in vista %dx%d" % (larg, alt)
    p.set_viewport_size({"width": larg, "height": alt})
    p.wait_for_timeout(600)
    if p.locator("#du-anteprima-chiudi").count():
        p.click("#du-anteprima-chiudi")
        expect(p.locator("#du-anteprima")).to_have_count(0, timeout=T_LUNGO)
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    p.evaluate("window.scrollTo(0, 0)")
    p.wait_for_timeout(400)
    p.locator("#du-nessuna-bozza button", has_text="Guarda l'anteprima").first.click()
    expect(p.locator("#du-rev-anteprima #du-vp .du-nc[data-k]").first).to_be_attached(timeout=T_LUNGO)
    p.wait_for_timeout(1500)
    alto = p.locator("#du-rev-anteprima").evaluate("(l) => Math.round(l.getBoundingClientRect().top)")
    vp = p.locator("#du-rev-anteprima #du-vp")
    n = vp.locator(".du-nc[data-k]").count()
    fuori = fuori_dal_riquadro(p, "#du-rev-anteprima #du-vp")
    scala = vp.get_attribute("data-scala") or ""
    adatta = ""
    if fuori:
        # la tela non mostra tutto: lo zoom deve essere quello adattato alla parte che si vede (senza scorrere: la misura
        # resta quella della finestra di adesso)
        adatta = vp.evaluate("""(v) => { v.focus({ preventScroll: true });
            v.dispatchEvent(new KeyboardEvent('keydown', { key: '0', bubbles: true })); return v.dataset.scala || ''; }""")
    zoom_ok = bool(scala) and bool(adatta) and abs(int(scala) - int(adatta)) <= 1
    ok = 0 <= alto <= 200 and n >= 2 and (not fuori or zoom_ok)
    e.controlla(nome, ok, "bordo alto %s px (atteso 0-200), schede %d, fuori dalla vista: %s, zoom %s %%, adattato %s %%" % (alto, n, fuori, scala, adatta or "-"),
                "c3_f4_anteprima_%dx%d.png" % (larg, alt))


def tab(b, pid):
    p = b.page
    p.click(".du-ptab[data-prodotto='%s']" % pid)
    expect(p.locator(".du-ptab[data-prodotto='%s']" % pid)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.pronta()
    p.wait_for_timeout(600)


def dialogo_solo_prodotto(b):
    """Il dialogo aperto: (motivo chiesto, casella della presa d'atto, testo)."""
    p = b.page
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(800)
    motivo = p.locator("#du-finestra textarea, #du-finestra #f-motivo").count()
    presa = p.locator("#du-finestra #f-presa").count()
    return motivo, presa, testo(b.finestra())


def diagnostiche(p):
    """Il testo della linguetta «Documenti del nodo» che si vede (senza i dettagli chiusi) e lo stato dei dettagli tecnici."""
    sez = p.locator("#du-sez-documenti")
    vis = sez.evaluate("""(s) => {
        const c = s.cloneNode(true);
        c.querySelectorAll('details:not([open]) > :not(summary)').forEach(x => x.remove());
        document.body.append(c); const t = c.innerText; c.remove(); return t; }""") if sez.count() else ""
    det = p.locator("#du-sez-documenti #du-diagnostiche-tecniche")
    return vis, det.count(), (det.get_attribute("open") is not None) if det.count() else False, testo(det)


def giro(b, e):
    p = b.page
    step1 = b.al["9120001.stp"]

    # ---- i4: lo STEP con la radice diversa (9170099A, il nome dice 9170001A): la card e l'anteprima la dicono
    tab(b, b.p7)
    b.cartella("p:" + b.p7)
    p.wait_for_timeout(800)
    card = p.locator("#du-corpo .du-step-proposto[data-prodotto='%s']" % b.p7).first
    tc = testo(card)
    e.controlla("i4 card con la radice letta", "9170099A" in tc, "la card: %r" % tc[:600], "c3_i4_card.png")
    g = card.locator("button", has_text="Guarda l'anteprima").first
    g.click()
    expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(1500)
    ta = testo(p.locator("#du-anteprima"))
    e.controlla("i4 anteprima con la radice letta", "9170099A" in ta and "non è determinabile" not in ta, "l'anteprima: %r" % ta[:600], "c3_i4_anteprima.png")
    if p.locator("#du-anteprima-chiudi").count():
        p.click("#du-anteprima-chiudi")
        expect(p.locator("#du-anteprima")).to_have_count(0, timeout=T_LUNGO)

    # ---- b1: il pulsante principale della card di 9170001A (senza struttura per il prodotto) apre il «solo prodotto»
    card = p.locator("#du-corpo .du-step-proposto[data-prodotto='%s']" % b.p7).first
    bc = card.locator("button", has_text="Conferma questo STEP").first
    expect(bc).to_be_enabled(timeout=T_LUNGO)
    bc.click()
    p.wait_for_timeout(1500)
    aperta = b.finestra().is_visible()
    av = testo(b.avviso()) if b.avviso().is_visible() else ""
    if aperta:
        motivo, presa, td = dialogo_solo_prodotto(b)
    else:
        motivo, presa, td = 0, 0, ""
    e.controlla("b1 card radice diversa, dialogo", aperta and not motivo and presa == 1 and "gruppo e impronta" not in av,
                "dialogo %s, motivo %d, presa d'atto %d; avviso %r; dialogo %r" % (aperta, motivo, presa, av[:300], td[:300]), "c3_b1_dialogo_9170.png")
    esito = ""
    if aperta and presa:
        p.check("#du-finestra #f-presa")
        b.ok_finestra()
        try:
            expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
            esito = "completata"
        except Exception as x:
            esito = "non completata: " + " ".join(str(x).split())[:200]
    e.controlla("b1 card radice diversa, conferma", esito == "completata", "esito: %r; avviso %r" % (esito, testo(b.avviso())[:300]), "c3_b1_esito_9170.png")

    # ---- b1: la card di 9130001A (radice senza marcatore, nessun gruppo importabile): lo stesso dialogo
    tab(b, b.p2)
    b.cartella("p:" + b.p2)
    p.wait_for_timeout(800)
    card = p.locator("#du-corpo .du-step-proposto[data-prodotto='%s']" % b.p2).first
    bc = card.locator("button", has_text="Conferma questo STEP").first
    expect(bc).to_be_enabled(timeout=T_LUNGO)
    bc.click()
    p.wait_for_timeout(1500)
    aperta = b.finestra().is_visible()
    av = testo(b.avviso()) if b.avviso().is_visible() else ""
    if aperta:
        motivo, presa, td = dialogo_solo_prodotto(b)
    else:
        motivo, presa, td = 0, 0, ""
    e.controlla("b1 card senza struttura, dialogo", aperta and not motivo and presa == 1 and "gruppo e impronta" not in av,
                "dialogo %s, motivo %d, presa d'atto %d; avviso %r; dialogo %r" % (aperta, motivo, presa, av[:300], td[:300]), "c3_b1_dialogo_9130.png")
    if aperta:
        b.chiudi_finestra()

    # ---- minore: nella Revisione senza bozza, con l'anteprima dello STEP proposto aperta, una sola «Conferma questo STEP»
    tab(b, b.p1)
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    p.locator("#du-nessuna-bozza button", has_text="Guarda l'anteprima").first.click()
    expect(p.locator("#du-corpo #du-anteprima")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(1000)
    nc = p.locator("#du-corpo button:visible", has_text="Conferma questo STEP come sorgente").count()
    e.controlla("m una sola conferma nella Revisione", nc == 1, "«Conferma questo STEP come sorgente» visibili: %d" % nc, "c3_m_revisione_anteprima.png")

    # ---- f4 (F4-I1): l'anteprima chiesta dalla Revisione senza bozza si apre in vista, a 1600x1000 e a 1366x768
    anteprima_in_vista(b, e, 1600, 1000)
    anteprima_in_vista(b, e, 1366, 768)
    p.set_viewport_size({"width": 1600, "height": 1000})
    p.wait_for_timeout(600)
    if p.locator("#du-anteprima-chiudi").count():
        p.click("#du-anteprima-chiudi")
        expect(p.locator("#du-anteprima")).to_have_count(0, timeout=T_LUNGO)

    # ---- i5: l'anteprima dello STEP a tre livelli si apre già dentro il riquadro, senza «Adatta»
    b.guarda(step1)
    p.wait_for_timeout(1500)
    det = p.locator("#du-sg-dettaglio")
    if det.count():
        det.evaluate("(d) => { d.scrollTop = 0; }")
        p.wait_for_timeout(300)
    fuori = fuori_dal_riquadro(p, "#du-anteprima #du-vp", "#du-sg-dettaglio" if det.count() else None)
    n = p.locator("#du-anteprima #du-vp .du-nc[data-k]").count()
    e.controlla("i5 anteprima adattata all'apertura", n >= 4 and not fuori, "schede %d, fuori dal riquadro all'apertura: %s" % (n, fuori), "c3_i5_anteprima.png")

    # ---- S8 su 9120001A dall'anteprima, poi i1: «Aggiorna proposta…» nella card è spento con il perché
    bt = p.locator("#du-conferma-sorgente").first
    expect(bt).to_be_enabled(timeout=T_LUNGO)
    bt.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    b.cartella("step")
    p.wait_for_timeout(800)
    ag = p.locator("#du-corpo button", has_text="Aggiorna proposta")
    stati = ag.evaluate_all("(ls) => ls.map(l => [l.id, l.disabled, l.title || ''])")
    e.controlla("i1 Aggiorna proposta spento", stati and all(x[1] and "non è ancora disponibile" in x[2] for x in stati),
                "«Aggiorna proposta…» nella pagina: %s" % stati, "c3_i1_aggiorna.png")

    # ---- i5: l'organigramma della bozza nella Revisione si apre dentro la tela, senza «Adatta»
    b.passo("revisione")
    expect(p.locator("#du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(800)
    fuori = fuori_dal_riquadro(p, "#du-vp")
    n = p.locator("#du-vp .du-nc[data-k]").count()
    e.controlla("i5 organigramma adattato all'apertura", n >= 4 and not fuori, "schede %d, fuori dalla tela all'apertura: %s" % (n, fuori), "c3_i5_organigramma.png")

    # ---- i2: le diagnostiche del pannello del nodo (radice) nella bozza
    p.locator("#du-vp .du-nc[data-k]").filter(has=p.locator(".du-nc-c", has_text=re.compile(r"^9120001A$"))).first.click()
    expect(p.locator("#du-pannello")).to_be_visible(timeout=T_LUNGO)
    b.linguetta("documenti")
    p.wait_for_timeout(500)
    vis, nd, aperto, td = diagnostiche(p)
    chiavi = sorted(set(RE_CHIAVI.findall(vis)))
    print("      dettagli tecnici della bozza: %r" % td[:400])
    e.controlla("i2 diagnostiche nella bozza", not chiavi and nd == 1 and not aperto,
                "chiavi a schermo %s; dettagli tecnici %d (aperti: %s); testo %r" % (chiavi, nd, aperto, vis[:500]), "c3_i2_bozza.png")

    # ---- i1: il dialogo «Riparti dalla proposta» non suggerisce «Aggiorna proposta…»
    rp = p.locator("#du-riparti")
    if rp.count() and rp.is_enabled():
        rp.click()
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        t = testo(b.finestra())
        e.controlla("i1 Riparti senza rimando", "Aggiorna proposta" not in t, "il dialogo: %r" % t[:500], "c3_i1_riparti.png")
        b.chiudi_finestra()
    else:
        e.controlla("i1 Riparti senza rimando", False, "«Riparti dalla proposta…» assente o spento: %r" % (rp.get_attribute("title") if rp.count() else ""))

    # ---- R4 e R5, poi i3: la BOM confermata (working) senza «?» né tratteggio del tipo proposto
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_be_visible(timeout=T_LUNGO)
    t = testo(p.locator("#du-wz-corpo"))
    n_pb = p.locator("#du-wz-corpo [data-du-pb]").count()
    e.controlla("m R4 senza effetto ripetuto", n_pb >= 1 and "si registra associazione registrata" not in t and t.count("non è la conferma del documento") <= 1,
                "righe dei documenti %d; il riepilogo: %r" % (n_pb, t[:600]), "c3_m_r4.png")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-wz-conferma")
    expect(b.esito_wizard()).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    p.locator("#du-wz-fine, #du-wz-chiudi").first.click()
    expect(p.locator("#du-wizard")).to_be_hidden(timeout=T_LUNGO)
    b.passo("revisione")
    expect(p.locator("#du-working #du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(800)
    chips = p.locator("#du-working .du-tipo").evaluate_all("(ls) => ls.map(l => [l.textContent.trim(), l.classList.contains('du-tipo--proposto')])")
    male = [c for c in chips if c[1] or c[0].endswith("?")]
    e.controlla("i3 working senza tipo proposto", chips and not male, "tipi nella working: %s" % chips, "c3_i3_working.png")

    # ---- i2: le diagnostiche del pannello del nodo (radice) nella working
    p.locator("#du-working #du-vp .du-nc[data-k]").filter(has=p.locator(".du-nc-c", has_text=re.compile(r"^9120001A$"))).first.click()
    expect(p.locator("#du-pannello")).to_be_visible(timeout=T_LUNGO)
    b.linguetta("documenti")
    p.wait_for_timeout(500)
    vis, nd, aperto, td = diagnostiche(p)
    chiavi = sorted(set(RE_CHIAVI.findall(vis)))
    e.controlla("i2 diagnostiche nella working", not chiavi and nd == 1 and not aperto,
                "chiavi a schermo %s; dettagli tecnici %d (aperti: %s); testo %r" % (chiavi, nd, aperto, vis[:500]), "c3_i2_working.png")


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
        d = json.loads(a.dati)
        b.p7 = d.get("prodotto7") or ""
        b.apri()
        e = Esiti(b)
        inizio = time.time()
        try:
            giro(b, e)
        except Exception as x:
            esito = 1
            print("  GIRO interrotto: %s" % str(x).replace("\n", " ")[:600])
            b.foto("c3_errore.png")
        if not e.fine():
            esito = 1
        if errori:
            print("  errori nella pagina: %s" % errori[:5])
            esito = 1
        print("  giro C3 %s (%.1fs)" % ("PASSATO" if not esito else "FALLITO", time.time() - inizio))
        browser.close()
    return esito


if __name__ == "__main__":
    sys.exit(main())
