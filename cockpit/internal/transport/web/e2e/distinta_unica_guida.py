# -*- coding: utf-8 -*-
"""Browser - frontend del 22/10 [U] (Stefano): la guida interattiva e gli aiuti «(?)» della Distinta unica
(web/static/du_guida.mjs), sul banco finto di distinta_unica_banco_test.go, come le altre prove della pagina.

Non lo si lancia a mano: lo avvia distinta_unica_guida_browser_test.go (tag `integrazione browser`), con `python -I`.
Usa il Banco e gli aiuti di e2e/distinta_unica.py (stessa cartella). Le prove, in sequenza sullo stesso banco:
  G1 Sorgente: le icone «(?)» sugli elementi principali, accanto all'angolo dell'elemento; il fumetto al passaggio del
     mouse e al fuoco da tastiera con il testo del catalogo; Esc e l'uscita del mouse lo chiudono
  G2 Sorgente: accendere e spegnere gli aiuti, mostrare un fumetto con il mouse e con il fuoco non spostano niente
     (scorrimento della pagina e dei riquadri, posizione degli elementi, ±1 px) e non toccano il DOM della pagina
  G3 la guida della Sorgente: si apre dalla testata, evidenzia l'elemento vero, avanza con «Avanti» e con le frecce,
     torna indietro, dice che cosa serve quando l'elemento non c'è, un clic sulla pagina velata non fa niente, Esc la
     chiude e il fuoco torna su «Guida»; nessuna richiesta al server mentre è aperta
  G4 «Mostra aiuti (?)» spento resta spento dopo una ricarica (memoria della scheda), e riacceso torna
  G5 Revisione con la bozza: le icone sulla tela, sul browser ad albero, sul pannello del nodo, sulla barra; il fumetto giusto; la pagina
     scorsa non si sposta con gli aiuti e con i fumetti
  G6 la guida della Revisione: tutti i passi fino a «Fine», l'evidenziazione degli elementi che ci sono, la pagina che
     torna dov'era alla chiusura; nessuna richiesta al server
  G7 (seconda richiesta del 22/10 [U]) gli aiuti in sovrimpressione: il fumetto dei (?) è piccolo (al più 320 px), sta
     accanto all'icona, con la freccia, dentro la finestra e senza coprire l'elemento spiegato; la guida non vela la
     pagina (solo il contorno luminoso), il suo fumetto compatto sta accanto all'elemento evidenziato e non sopra, con
     Esci, Indietro e Avanti su una riga; a 1600 × 1000 e a 1366 × 768
I codici e i nomi dei file sono inventati (ACM-xxxx), come nel banco.
"""
import argparse
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import distinta_unica as du  # noqa: E402
from distinta_unica import verifica, T_BREVE, T_LUNGO  # noqa: E402
from playwright.sync_api import expect, sync_playwright  # noqa: E402

PROVE = []


def prova(numero, nome):
    def deco(fn):
        PROVE.append((numero, nome, fn))
        return fn
    return deco


# ---- misure

ICONA = r"""(k) => { const b = document.querySelector('.du-aiuto[data-aiuto="' + k + '"]'); if (!b || b.hidden) return null;
  const r = b.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height }; }"""

RETT = r"""(s) => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect();
  return { l: r.left, t: r.top, r: r.right, b: r.bottom, w: r.width, h: r.height }; }"""

# la fotografia della pagina: scorrimento della finestra e dei riquadri che scorrono, posizione degli elementi con un id
# dentro #du, numero dei nodi del DOM di #du (le icone non ci entrano)
# il riquadro che fa scorrere la pagina: la finestra o un antenato di #du (il <main> del layout)
SCORRITORE = r"""const scorritore = () => { for (let e = document.getElementById('du'); e; e = e.parentElement) {
  if (e === document.body || e === document.documentElement) break;
  const o = getComputedStyle(e).overflowY; if ((o === 'auto' || o === 'scroll') && e.scrollHeight > e.clientHeight + 2) return e; } return null; };"""

FOTO = r"""() => { """ + SCORRITORE + r"""
  const sc = scorritore();
  const out = { sx: window.scrollX, sy: window.scrollY + (sc ? sc.scrollTop : 0), nodi: document.querySelectorAll('#du *').length, scroll: {}, pos: {} };
  for (const e of document.querySelectorAll('#du *')) {
    if (e.scrollHeight > e.clientHeight + 2 && e.id) out.scroll[e.id] = e.scrollTop;
  }
  for (const e of document.querySelectorAll('#du [id]')) {
    const r = e.getBoundingClientRect(); if (!r.width && !r.height) continue;
    out.pos[e.id] = [Math.round(r.left * 10) / 10, Math.round(r.top * 10) / 10, Math.round(r.width * 10) / 10, Math.round(r.height * 10) / 10];
  }
  return out; }"""


def scorri(p, dove):
    """La pagina (la finestra o il riquadro che la fa scorrere) in cima o a metà."""
    p.evaluate("(dove) => { " + SCORRITORE + " const e = scorritore() || document.scrollingElement;"
               " e.scrollTop = dove === 'cima' ? 0 : Math.round((e.scrollHeight - e.clientHeight) / 2); }", dove)


def alto(p):
    return p.evaluate("() => { " + SCORRITORE + " const e = scorritore(); return window.scrollY + (e ? e.scrollTop : 0); }")


def foto_pagina(p):
    return p.evaluate(FOTO)


def uguali(a, b, cosa):
    verifica(abs(a["sx"] - b["sx"]) <= 1 and abs(a["sy"] - b["sy"]) <= 1, "%s: la pagina è scorsa da %s,%s a %s,%s" % (cosa, a["sx"], a["sy"], b["sx"], b["sy"]))
    verifica(a["nodi"] == b["nodi"], "%s: il DOM della pagina è cambiato (%d nodi, poi %d)" % (cosa, a["nodi"], b["nodi"]))
    for k, v in a["scroll"].items():
        w = b["scroll"].get(k)
        verifica(w is not None and abs(w - v) <= 1, "%s: il riquadro %s è scorso da %s a %s" % (cosa, k, v, w))
    diversi = []
    for k, v in a["pos"].items():
        w = b["pos"].get(k)
        if w is None or any(abs(x - y) > 1 for x, y in zip(v, w)):
            diversi.append("%s %s→%s" % (k, v, w))
    verifica(not diversi, "%s: elementi spostati (%d): %s" % (cosa, len(diversi), "; ".join(diversi[:6])))


COPRE = r"""() => { const sel = 'button, input, select, textarea, a[href], summary, label, [role=button], [role=row], [role=treeitem], [role=tab]';
  const out = [];
  for (const b of document.querySelectorAll('.du-aiuto')) { if (b.hidden) continue; const r = b.getBoundingClientRect();
    for (const c of document.querySelectorAll('#du ' + sel.split(', ').join(', #du '))) { const q = c.getBoundingClientRect(); if (!q.width) continue;
      const cx = q.left + q.width / 2, cy = q.top + q.height / 2;
      if (cx >= r.left && cx <= r.right && cy >= r.top && cy <= r.bottom) out.push(b.dataset.aiuto + ' copre ' + (c.id || c.tagName)); } }
  return out; }"""


def nessuna_copre(p):
    """Nessuna icona «(?)» copre il centro di un elemento cliccabile della pagina: i clic restano dove erano."""
    c = p.evaluate(COPRE)
    verifica(not c, "icone sopra il centro di elementi cliccabili: %s" % c)


def icona(p, k):
    return p.evaluate(ICONA, k)


def attendi_icona(b, k, ms=T_BREVE):
    b.attendi(lambda: icona(b.page, k) is not None, "nessuna icona «(?)» visibile per «%s»" % k, ms=ms)
    return icona(b.page, k)


def icona_accanto(b, k, sel):
    """L'icona di k è visibile, di 16-18 px, sull'angolo in alto a destra dell'elemento sel (entro 12 px)."""
    c = attendi_icona(b, k)
    r = b.page.evaluate(RETT, sel)
    verifica(r is not None, "l'elemento %s non c'è" % sel)
    verifica(14 <= c["w"] <= 18 and 14 <= c["h"] <= 18, "l'icona di «%s» non è di 16-18 px: %sx%s" % (k, c["w"], c["h"]))
    verifica(abs(c["x"] - r["r"]) <= 12 and abs(c["y"] - r["t"]) <= 12,
             "l'icona di «%s» non sta sull'angolo di %s: icona %s,%s, angolo %s,%s" % (k, sel, round(c["x"]), round(c["y"]), round(r["r"]), round(r["t"])))


def fumetto(p):
    return p.locator("#du-aiuto-fumetto")


def fumetto_al_mouse(b, k, titolo, parola):
    p = b.page
    c = attendi_icona(b, k)
    p.mouse.move(c["x"], c["y"])
    f = fumetto(p)
    expect(f).to_be_visible(timeout=T_BREVE)
    t = f.inner_text()
    verifica(titolo in t and parola in t, "il fumetto di «%s» non dice «%s» / «%s»: %r" % (k, titolo, parola, t))
    # il fumetto sta dentro la finestra
    r = p.evaluate(RETT, "#du-aiuto-fumetto")
    vw = p.evaluate("() => document.documentElement.clientWidth")
    vh = p.viewport_size["height"]
    verifica(r["l"] >= 0 and r["t"] >= 0 and r["r"] <= vw and r["b"] <= vh, "il fumetto esce dalla finestra: %s" % r)
    p.mouse.move(5, vh - 5)
    expect(f).to_be_hidden(timeout=T_BREVE)


def fumetto_al_fuoco(b, k, titolo):
    p = b.page
    attendi_icona(b, k)
    p.evaluate("(k) => document.querySelector('.du-aiuto[data-aiuto=\"' + k + '\"]').focus({ preventScroll: true })", k)
    f = fumetto(p)
    expect(f).to_be_visible(timeout=T_BREVE)
    verifica(titolo in f.inner_text(), "il fumetto al fuoco di «%s» non dice «%s»: %r" % (k, titolo, f.inner_text()))
    ic = p.locator('.du-aiuto[data-aiuto="%s"]' % k)
    verifica(ic.get_attribute("aria-describedby") == "du-aiuto-fumetto" and ic.get_attribute("tabindex") == "0", "l'icona di «%s» non è focusabile o non descrive il fumetto" % k)
    p.keyboard.press("Escape")
    expect(f).to_be_hidden(timeout=T_BREVE)
    p.evaluate("() => document.activeElement && document.activeElement.blur()")


def aiuti(p, acceso):
    c = p.locator("#du-aiuti-mostra")
    if c.is_checked() != acceso:
        # il clic dal DOM: il clic di Playwright porterebbe in vista la testata e sporcherebbe la misura dello scorrimento
        c.evaluate("(e) => e.click()")
    expect(c).to_be_checked(checked=acceso)
    p.wait_for_timeout(250)


# ---- le prove

@prova("G1", "Sorgente: icone «(?)» sugli elementi principali, accanto all'angolo; fumetto al mouse e al fuoco con il testo del catalogo; Esc lo chiude")
def g1(b):
    p = b.page
    expect(p.locator("#du-guida-apri")).to_be_visible()
    expect(p.locator("#du-aiuti-mostra")).to_be_checked()
    p.wait_for_timeout(300)
    icona_accanto(b, "passo-sorgente", "#du-passo-sorgente")
    icona_accanto(b, "passo-revisione", "#du-passo-revisione")
    icona_accanto(b, "sg-cerca", "#du-sg-cerca")
    icona_accanto(b, "sg-cartelle", "#du-sg-albero")
    icona_accanto(b, "sg-c-step", "#du-sg-c-step")
    icona_accanto(b, "sg-aggiungi", "#du-sg-aggiungi")
    icona_accanto(b, "vai-revisione", "#du-vai-revisione")
    for k in ("prodotti", "sg-riga", "st-candidata", "guarda", "conferma-sorgente", "altri", "scarta", "sg-dettaglio", "aggiorna", "menu-rfq"):
        attendi_icona(b, k)
    # «Tutti» è l'ultima cartella: con le voci più ariose del restyling, a 1440 × 900 sta sotto il bordo del riquadro delle
    # cartelle, che scorre per conto suo, e un'icona compare solo sugli elementi in vista. Si scorre il riquadro (non la
    # pagina), si cerca l'icona e si torna in cima
    p.evaluate("() => { const a = document.querySelector('#du-sg-albero'); a.scrollTop = a.scrollHeight; }")
    p.wait_for_timeout(300)
    attendi_icona(b, "sg-c-tutti")
    p.evaluate("() => { document.querySelector('#du-sg-albero').scrollTop = 0; }")
    p.wait_for_timeout(300)
    # le icone non hanno testo e stanno fuori da #du: i testi e i nomi della pagina restano quelli
    verifica(p.locator("#du .du-aiuto").count() == 0, "icone «(?)» dentro la pagina")
    verifica(p.evaluate("() => [...document.querySelectorAll('.du-aiuto')].every((x) => x.textContent === '')"), "un'icona «(?)» ha del testo")
    nessuna_copre(p)
    fumetto_al_mouse(b, "guarda", "Guarda l'anteprima", "prima di sceglierlo")
    fumetto_al_mouse(b, "conferma-sorgente", "Conferma questo STEP come sorgente", "Non conferma la BOM")
    fumetto_al_mouse(b, "scarta", "Scarta", "traccia")
    fumetto_al_mouse(b, "altri", "Metti in Altri file", "ALTRO")
    fumetto_al_mouse(b, "sg-aggiungi", "+ Aggiungi file", "Carica")
    fumetto_al_fuoco(b, "passo-revisione", "Scheda Revisione")
    fumetto_al_fuoco(b, "sg-cartelle", "Cartelle dei file")
    # il clic sull'icona non arriva alla pagina: niente dialogo, niente cartella cambiata
    sel_prima = p.locator("#du-sg-albero [aria-selected=true]").first.get_attribute("id")
    c = attendi_icona(b, "sg-aggiungi")
    p.mouse.click(c["x"], c["y"])
    p.wait_for_timeout(300)
    verifica(p.locator("#du-finestra").is_hidden(), "il clic sull'icona ha aperto un dialogo")
    verifica(p.locator("#du-sg-albero [aria-selected=true]").first.get_attribute("id") == sel_prima, "il clic sull'icona ha cambiato cartella")
    p.keyboard.press("Escape")
    p.mouse.move(5, 895)
    b.foto("G1_sorgente_aiuti.png")


@prova("G2", "Sorgente: aiuti accesi/spenti e fumetti al mouse e al fuoco non spostano la pagina né i riquadri (±1 px) e non toccano il DOM di #du")
def g2(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 720})
    try:
        p.wait_for_timeout(400)
        # l'elenco della Sorgente scorso un poco, se scorre
        p.evaluate("() => { const e = document.querySelector('#du-sg-elenco'); if (e) e.scrollTop = Math.round((e.scrollHeight - e.clientHeight) / 2); }")
        aiuti(p, False)
        verifica(p.locator(".du-aiuto:visible").count() == 0, "con gli aiuti spenti si vedono icone")
        a = foto_pagina(p)
        aiuti(p, True)
        attendi_icona(b, "sg-cerca")
        b2 = foto_pagina(p)
        uguali(a, b2, "accendere gli aiuti")
        c = attendi_icona(b, "sg-cartelle")
        p.mouse.move(c["x"], c["y"])
        expect(fumetto(p)).to_be_visible()
        uguali(a, foto_pagina(p), "il fumetto al passaggio del mouse")
        p.mouse.move(5, 715)
        expect(fumetto(p)).to_be_hidden()
        p.evaluate("() => document.querySelector('.du-aiuto[data-aiuto=\"sg-cerca\"]').focus({ preventScroll: true })")
        expect(fumetto(p)).to_be_visible()
        uguali(a, foto_pagina(p), "il fumetto al fuoco")
        p.keyboard.press("Escape")
        expect(fumetto(p)).to_be_hidden()
        uguali(a, foto_pagina(p), "Esc sul fumetto")
        p.evaluate("() => document.activeElement && document.activeElement.blur()")
        # lo scorrimento della pagina non sposta niente quando le icone si ricalcolano
        p.mouse.move(5, 715)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})
        p.wait_for_timeout(300)


def apri_guida(b):
    p = b.page
    p.click("#du-guida-apri")
    expect(p.locator("#du-guida")).to_be_visible(timeout=T_BREVE)
    expect(p.locator("#du-guida-fumetto")).to_be_visible()
    p.wait_for_timeout(300)


def passo_guida(p):
    t = p.locator("#du-guida-conta").text_content()
    m = re.search(r"Passo (\d+) di (\d+)", t)
    verifica(m, "il contatore dei passi non si legge: %r" % t)
    return int(m.group(1)), int(m.group(2))


def evidenzia(p, sel):
    """Il riquadro luminoso della guida sta intorno all'elemento sel (entro 10 px, nella finestra)."""
    p.wait_for_timeout(300)
    verifica(p.locator("#du-guida").get_attribute("data-evidenzia") == "1", "la guida non evidenzia niente per %s" % sel)
    a = p.evaluate(RETT, "#du-guida-anello")
    r = p.evaluate(RETT, sel)
    vw = p.evaluate("() => document.documentElement.clientWidth")
    vh = p.viewport_size["height"]
    att = {"l": max(4, r["l"] - 6), "t": max(4, r["t"] - 6), "r": min(vw - 4, r["r"] + 6), "b": min(vh - 4, r["b"] + 6)}
    verifica(all(abs(a[k] - att[k]) <= 10 for k in att), "il riquadro della guida non sta intorno a %s: %s, atteso %s" % (sel, a, att))


@prova("G3", "guida della Sorgente: si apre, evidenzia elementi veri, avanza e torna (pulsanti e frecce), spiega gli elementi assenti, la pagina velata non risponde, Esc chiude; nessuna richiesta")
def g3(b):
    p = b.page
    if p.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
        b.passo("sorgente")
    p.wait_for_timeout(300)
    sy = alto(p)
    i = b.segna()
    apri_guida(b)
    n, tot = passo_guida(p)
    verifica(n == 1 and 6 <= tot <= 12, "la guida della Sorgente parte da %d di %d (attesi 1 di 6-12)" % (n, tot))
    verifica("Sorgente" in p.locator("#du-guida-conta").text_content(), "il contatore non dice la scheda")
    evidenzia(p, "#du-prodotti")
    verifica(p.evaluate("() => document.activeElement && document.activeElement.id") == "du-guida-avanti", "il fuoco non è su «Avanti»")
    expect(p.locator("#du-guida-indietro")).to_be_disabled()
    verifica(p.locator(".du-aiuto:visible").count() == 0, "con la guida aperta si vedono le icone «(?)»")
    p.click("#du-guida-avanti")
    verifica(passo_guida(p)[0] == 2, "«Avanti» non porta al passo 2")
    evidenzia(p, "#du-nav")
    p.keyboard.press("ArrowRight")
    verifica(passo_guida(p)[0] == 3, "la freccia a destra non porta al passo 3")
    evidenzia(p, "#du-sg-albero")
    p.keyboard.press("ArrowLeft")
    verifica(passo_guida(p)[0] == 2, "la freccia a sinistra non torna al passo 2")
    p.click("#du-guida-indietro")
    verifica(passo_guida(p)[0] == 1, "«Indietro» non torna al passo 1")
    # un clic sulla pagina velata (sopra «+ Aggiungi file») non fa niente
    r = p.evaluate(RETT, "#du-sg-aggiungi")
    p.mouse.click(r["l"] + r["w"] / 2, r["t"] + r["h"] / 2)
    p.wait_for_timeout(300)
    verifica(p.locator("#du-finestra").is_hidden() and p.locator("#du-guida").is_visible(), "il clic sulla pagina velata ha fatto un gesto o chiuso la guida")
    # tutti i passi: o l'elemento evidenziato, o la spiegazione di che cosa serve
    titoli = []
    for k in range(tot):
        n, _ = passo_guida(p)
        verifica(n == k + 1, "passo %d atteso, letto %d" % (k + 1, n))
        titoli.append(p.locator("#du-guida-titolo").inner_text())
        ev = p.locator("#du-guida").get_attribute("data-evidenzia")
        verifica(ev == "1" or p.locator("#du-guida-manca").count() == 1, "il passo %d non evidenzia niente e non dice che cosa serve" % n)
        if ev == "1":
            expect(p.locator("#du-guida-anello")).to_be_visible()
        f = p.evaluate(RETT, "#du-guida-fumetto")
        vw = p.evaluate("() => document.documentElement.clientWidth")
        verifica(f["l"] >= 0 and f["t"] >= 0 and f["r"] <= vw and f["b"] <= p.viewport_size["height"], "il fumetto del passo %d esce dalla finestra: %s" % (n, f))
        if k == 5:
            b.foto("G3_guida_sorgente_passo6.png")
        if k < tot - 1:
            p.click("#du-guida-avanti")
            p.wait_for_timeout(150)
    expect(p.locator("#du-guida-avanti")).to_have_text("Fine")
    p.keyboard.press("Escape")
    expect(p.locator("#du-guida")).to_have_count(0)
    verifica(p.evaluate("() => document.activeElement && document.activeElement.id") == "du-guida-apri", "chiusa la guida il fuoco non torna su «Guida»")
    verifica(abs(alto(p) - sy) <= 1, "chiusa la guida la pagina non è tornata dov'era")
    richieste = b.richieste_da(i)
    verifica(not richieste, "la guida ha fatto richieste: %s" % ["%s %s" % (x["metodo"], x["path"]) for x in richieste])
    attendi_icona(b, "sg-cerca")
    # «Fine» dell'ultimo passo chiude anche lei
    apri_guida(b)
    for _ in range(tot - 1):
        p.keyboard.press("ArrowRight")
    p.click("#du-guida-avanti")
    expect(p.locator("#du-guida")).to_have_count(0)
    print("         passi della Sorgente: %s" % " | ".join(titoli))


@prova("G4", "«Mostra aiuti (?)» spento resta spento dopo una ricarica (memoria della scheda), riacceso torna")
def g4(b):
    p = b.page
    aiuti(p, False)
    verifica(p.evaluate("() => localStorage.getItem('cockpit.guida.aiuti')") == "0", "la scelta non è nella memoria della scheda")
    b.apri()
    p.wait_for_timeout(500)
    expect(p.locator("#du-aiuti-mostra")).not_to_be_checked()
    verifica(p.locator(".du-aiuto:visible").count() == 0, "dopo la ricarica con gli aiuti spenti si vedono icone")
    aiuti(p, True)
    attendi_icona(b, "sg-cerca")
    b.apri()
    expect(p.locator("#du-aiuti-mostra")).to_be_checked()
    attendi_icona(b, "sg-cerca")


@prova("G5", "Revisione con la bozza: icone su viste, browser ad albero, tela, scheda, pannello del nodo e barra; fumetti giusti; con la pagina scorsa aiuti e fumetti non spostano niente")
def g5(b):
    p = b.page
    du.x_bozza(b)
    b.scheda("k-12")
    expect(p.locator("#du-pannello")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(400)
    for k in ("vista-org", "vista-elenco", "tela", "scheda", "zoom", "pannello", "ling-dettaglio", "ling-documenti"):
        attendi_icona(b, k)
    icona_accanto(b, "vista-org", "#du-vista-org")
    nessuna_copre(p)
    fumetto_al_mouse(b, "vista-org", "Organigramma", "albero")
    fumetto_al_mouse(b, "tela", "Organigramma della bozza", "pannello del nodo")
    # il browser ad albero (integrazione frontend): l'aiuto sul pulsante «Browser»; aperto, gli aiuti su gesti, filtro e
    # righe; poi si richiude com'era. Sta sotto la tela (terza richiesta del 22/10): lo si porta in vista
    p.locator("#du-br").scroll_into_view_if_needed()
    p.wait_for_timeout(300)
    attendi_icona(b, "br")
    fumetto_al_mouse(b, "br", "Browser della distinta", "browser degli assiemi")
    era_chiuso = p.locator("#du-br-chiudi").get_attribute("aria-expanded") == "false"
    if era_chiuso:
        p.locator("#du-br-chiudi").evaluate("(e) => e.click()")
    expect(p.locator("#du-br-albero")).to_be_visible(timeout=T_BREVE)
    p.wait_for_timeout(300)
    for k in ("br-espandi", "br-filtro", "br-albero"):
        attendi_icona(b, k)
    fumetto_al_mouse(b, "br-filtro", "Filtra il browser", "Esc")
    fumetto_al_fuoco(b, "br-albero", "Righe del browser")
    if era_chiuso:
        p.locator("#du-br-chiudi").evaluate("(e) => e.click()")
        p.wait_for_timeout(300)
    # la barra e i gesti del nodo: si portano in vista scorrendo la pagina
    p.locator("#du-cta-bom").scroll_into_view_if_needed()
    p.wait_for_timeout(300)
    for k in ("salvataggio", "annulla", "ripeti", "cta-bom"):
        attendi_icona(b, k)
    fumetto_al_mouse(b, "cta-bom", "Rivedi e conferma BOM", "due fasi")
    fumetto_al_fuoco(b, "annulla", "Annulla")
    fumetto_al_fuoco(b, "salvataggio", "Stato del salvataggio")
    p.locator("#du-correggi").scroll_into_view_if_needed()
    p.wait_for_timeout(300)
    for k in ("gesti", "correggi", "cambia-tipo", "sposta", "non-includere"):
        attendi_icona(b, k)
    nessuna_copre(p)
    fumetto_al_mouse(b, "correggi", "Correggi codice, rev e descrizione", "proposta")
    fumetto_al_mouse(b, "non-includere", "Non includere nella BOM", "includere di nuovo")
    b.foto("G5_revisione_aiuti.png")
    # stabilità con la pagina scorsa
    scorri(p, "meta")
    p.wait_for_timeout(300)
    aiuti(p, False)
    a = foto_pagina(p)
    verifica(a["sy"] > 0, "la pagina della Revisione non scorre: la prova non prova niente")
    aiuti(p, True)
    p.wait_for_timeout(300)
    uguali(a, foto_pagina(p), "accendere gli aiuti nella Revisione")
    vis = p.evaluate("() => [...document.querySelectorAll('.du-aiuto')].filter((x) => !x.hidden).map((x) => x.dataset.aiuto)")
    verifica(vis, "nessuna icona in vista nella Revisione scorsa")
    c = icona(p, vis[0])
    p.mouse.move(c["x"], c["y"])
    expect(fumetto(p)).to_be_visible()
    uguali(a, foto_pagina(p), "il fumetto nella Revisione")
    p.mouse.move(5, 895)
    expect(fumetto(p)).to_be_hidden()
    p.evaluate("(k) => document.querySelector('.du-aiuto[data-aiuto=\"' + k + '\"]').focus({ preventScroll: true })", vis[-1])
    expect(fumetto(p)).to_be_visible()
    uguali(a, foto_pagina(p), "il fumetto al fuoco nella Revisione")
    p.keyboard.press("Escape")
    expect(fumetto(p)).to_be_hidden()
    p.evaluate("() => document.activeElement && document.activeElement.blur()")
    scorri(p, "cima")


@prova("G6", "guida della Revisione: tutti i passi fino a «Fine», gli elementi presenti evidenziati, la pagina che torna dov'era; nessuna richiesta")
def g6(b):
    p = b.page
    if p.locator("#du-passo-revisione[aria-current=step]").count() == 0:
        b.passo("revisione")
    scorri(p, "cima")
    p.wait_for_timeout(300)
    sy = alto(p)
    i = b.segna()
    apri_guida(b)
    n, tot = passo_guida(p)
    verifica(n == 1 and 6 <= tot <= 12, "la guida della Revisione parte da %d di %d (attesi 1 di 6-12)" % (n, tot))
    verifica("Revisione" in p.locator("#du-guida-conta").text_content(), "il contatore non dice la Revisione")
    evidenzia(p, "#du-passo-revisione")
    titoli = []
    evidenziati = 0
    for k in range(tot):
        titoli.append(p.locator("#du-guida-titolo").inner_text())
        t = titoli[-1]
        ev = p.locator("#du-guida").get_attribute("data-evidenzia") == "1"
        evidenziati += 1 if ev else 0
        verifica(ev or p.locator("#du-guida-manca").count() == 1, "il passo «%s» non evidenzia niente e non dice che cosa serve" % t)
        if t == "Rivedi e conferma BOM":
            evidenzia(p, "#du-cta-bom")
            b.foto("G6_guida_revisione_cta.png")
        if t == "Il pannello del nodo":
            evidenzia(p, "#du-pannello")
        if k < tot - 1:
            p.keyboard.press("ArrowRight")
            p.wait_for_timeout(200)
    verifica(evidenziati >= tot - 1, "con la bozza aperta la guida della Revisione evidenzia solo %d passi su %d" % (evidenziati, tot))
    p.click("#du-guida-avanti")
    expect(p.locator("#du-guida")).to_have_count(0)
    p.wait_for_timeout(200)
    verifica(abs(alto(p) - sy) <= 1, "chiusa la guida la pagina non è tornata dov'era (%s, prima %s)" % (alto(p), sy))
    richieste = b.richieste_da(i)
    verifica(not richieste, "la guida ha fatto richieste: %s" % ["%s %s" % (x["metodo"], x["path"]) for x in richieste])
    print("         passi della Revisione: %s" % " | ".join(titoli))



# ---- seconda richiesta del 22/10 [U] (Stefano): aiuti in sovrimpressione, piccoli e accanto a quello che spiegano

FUMETTO = r"""(arg) => { const f = document.getElementById(arg.f); const t = arg.t ? document.querySelector(arg.t) : null; const ic = arg.k ? document.querySelector('.du-aiuto[data-aiuto="' + arg.k + '"]') : null;
  if (!f) return null; const r = f.getBoundingClientRect(); const q = t ? t.getBoundingClientRect() : null; const i = ic ? ic.getBoundingClientRect() : null;
  const inter = (a, b) => Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left)) * Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
  const dist = (a, b) => Math.hypot(Math.max(a.left - b.right, 0, b.left - a.right), Math.max(a.top - b.bottom, 0, b.top - a.bottom));
  return { w: r.width, h: r.height, l: r.left, t: r.top, r: r.right, b: r.bottom, vw: document.documentElement.clientWidth, vh: innerHeight,
    copre: q ? inter(r, q) : 0, area: r.width * r.height, distIcona: i ? dist(r, i) : null, distElem: q ? dist(r, q) : null,
    freccia: !!f.querySelector('.du-freccia'), lato: f.dataset.lato || '' }; }"""

AIUTI_G7 = (("aggiorna", "#du-aggiorna", "Aggiorna"), ("vista-org", "#du-vista-org", "Organigramma"), ("zoom", "#du-zoom-adatta", "Mostra tutto"),
            ("schermo", "#du-zoom-schermo", "Schermo intero"), ("br-filtro", "#du-br-filtro", "Filtra il browser"))


@prova("G7", "aiuti in sovrimpressione: il fumetto dei (?) è piccolo, accanto all'icona e non copre l'elemento; la guida non vela la pagina e il suo fumetto compatto sta accanto all'elemento, con i pulsanti su una riga")
def g7(b):
    p = b.page
    try:
        for w, hh in ((1600, 1000), (1366, 768)):
            p.set_viewport_size({"width": w, "height": hh})
            b.apri()
            du.x_bozza(b)
            if p.locator("#du-br-chiudi").get_attribute("aria-expanded") == "false":
                p.click("#du-br-chiudi")
            scorri(p, "cima")
            p.wait_for_timeout(400)
            for k, sel, titolo in AIUTI_G7:
                if icona(p, k) is None:
                    # l'elemento può stare sotto la piega a 768 px: lo si porta in vista senza aiuti (la prova misura il fumetto, non lo scorrimento)
                    p.locator(sel).scroll_into_view_if_needed()
                    p.wait_for_timeout(300)
                attendi_icona(b, k)
                p.locator('.du-aiuto[data-aiuto="%s"]' % k).hover()
                expect(fumetto(p)).to_be_visible(timeout=T_BREVE)
                verifica(titolo in fumetto(p).inner_text(), "il fumetto di «%s» non dice «%s»" % (k, titolo))
                m = p.evaluate(FUMETTO, {"f": "du-aiuto-fumetto", "t": sel, "k": k})
                print("      %dx%d (?) %-10s %r" % (w, hh, k, {x: round(m[x]) if isinstance(m[x], float) else m[x] for x in ("w", "h", "copre", "distIcona", "lato")}))
                verifica(m["w"] <= 320 and m["h"] <= 200, "il fumetto di «%s» è troppo grande: %r" % (k, m))
                verifica(m["l"] >= 0 and m["t"] >= 0 and m["r"] <= m["vw"] and m["b"] <= m["vh"], "il fumetto di «%s» esce dalla finestra: %r" % (k, m))
                verifica(m["copre"] <= 0.05 * m["area"], "il fumetto di «%s» copre l'elemento che spiega: %r" % (k, m))
                # accanto: a filo dell'elemento spiegato (o della sua icona) e vicino all'icona sotto il mouse
                verifica(m["distIcona"] is not None and m["distIcona"] <= 60 and min(m["distIcona"], m["distElem"]) <= 24, "il fumetto di «%s» non sta accanto all'icona: %r" % (k, m))
                verifica(m["freccia"] and m["lato"] in ("sotto", "sopra", "destra", "sinistra"), "il fumetto di «%s» non ha la freccia verso l'elemento: %r" % (k, m))
                if k == "zoom":
                    b.foto("G7_%dx%d_aiuto.png" % (w, hh))
                p.mouse.move(2, hh - 2)
                expect(fumetto(p)).to_be_hidden(timeout=T_BREVE)
            # la guida: niente velo, contorno luminoso, fumetto compatto accanto e non sopra, pulsanti su una riga
            scorri(p, "cima")
            p.wait_for_timeout(300)
            apri_guida(b)
            n, tot = passo_guida(p)
            velo = p.evaluate("""() => { const b = document.querySelector('.du-guida-blocco'), a = document.getElementById('du-guida-anello');
                return { fondo: getComputedStyle(b).backgroundColor, ombra: a ? getComputedStyle(a).boxShadow : '' }; }""")
            verifica(velo["fondo"] in ("rgba(0, 0, 0, 0)", "transparent") and "9999" not in velo["ombra"], "la guida vela ancora la pagina: %r" % velo)
            for _ in range(tot):
                ev = p.locator("#du-guida").get_attribute("data-evidenzia") == "1"
                m = p.evaluate(FUMETTO, {"f": "du-guida-fumetto", "t": "#du-guida-anello" if ev else "", "k": ""})
                riga = p.evaluate("() => Array.from(document.querySelectorAll('#du-guida-fumetto .du-guida-piede button'), x => Math.round(x.getBoundingClientRect().top))")
                anello = p.evaluate(RETT, "#du-guida-anello") if ev else None
                grande = anello and (anello["w"] > 0.55 * m["vw"] and anello["h"] > 0.5 * m["vh"])
                verifica(m["w"] <= 320, "passo %d: il fumetto della guida è largo %s px" % (n, m["w"]))
                verifica(m["l"] >= 0 and m["t"] >= 0 and m["r"] <= m["vw"] and m["b"] <= m["vh"], "passo %d: il fumetto esce dalla finestra: %r" % (n, m))
                verifica(len(riga) == 3 and max(riga) - min(riga) <= 2, "passo %d: Esci, Indietro e Avanti non stanno su una riga: %r" % (n, riga))
                if ev and not grande:
                    verifica(m["copre"] <= 0.05 * m["area"], "passo %d: il fumetto copre l'elemento evidenziato: %r / %r" % (n, m, anello))
                    verifica(m["distElem"] is not None and m["distElem"] <= 24, "passo %d: il fumetto non sta accanto all'elemento: %r" % (n, m))
                if n == 3:
                    b.foto("G7_%dx%d_guida_passo3.png" % (w, hh))
                if n == tot:
                    break
                p.click("#du-guida-avanti")
                p.wait_for_timeout(250)
                n, _ = passo_guida(p)
            p.keyboard.press("Escape")
            expect(p.locator("#du-guida")).to_have_count(0, timeout=T_BREVE)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    scelte = {x.strip().upper() for x in a.prove.split(",") if x.strip()}
    expect.set_options(timeout=T_BREVE)
    falliti = []
    fatte = 0
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(viewport={"width": 1440, "height": 900}, locale="it-IT")
        page = ctx.new_page()
        page.set_default_timeout(T_LUNGO)
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        page.on("dialog", lambda d: (errori.append("dialogo inatteso: " + d.message), d.dismiss()))
        b = du.Banco(page, a)
        b.apri()
        for numero, nome, fn in PROVE:
            if scelte and numero not in scelte:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  DU-%s  %-92s ESEGUITA PASSATA  (%.1fs)" % (numero, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(numero)
                print("  DU-%s  %-92s FALLITA  %s" % (numero, nome, str(e).splitlines()[0] if str(e) else repr(e)))
                testo = str(e)
                if "\n" in testo:
                    print("         " + "\n         ".join(testo.splitlines()[1:12]))
                b.foto("errore_%s.png" % numero)
                try:
                    for _ in range(2):
                        page.keyboard.press("Escape")
                        page.wait_for_timeout(150)
                except Exception:
                    pass
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove della guida nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
