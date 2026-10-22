# -*- coding: utf-8 -*-
"""Browser, messaggio 46 dell'utente (21/10), corsia ALBERO: l'organigramma della Distinta unica come nella pagina di prima.

Lo lancia TestDistintaUnicaAlberoNelBrowser (distinta_unica_albero_browser_test.go, tag `integrazione browser`) sul banco
finto di distinta_unica_banco_test.go, con `python -I`. Riusa gli aiuti di distinta_unica.py (la classe Banco).

Che cosa prova, con il numero del difetto dell'elenco del video (D05...D43):
  A01 D05, D06, D07  le anteprime dei 2D nei nodi (la pagina 1 vera del PDF, «nessun 2D associato» senza), la disposizione
                     dell'organigramma di prima (i figli di un padre in una riga sola, il padre centrato sopra), la
                     quantità del legame «x1 dal file · da confermare», il tipo proposto con «?»
  A02 D13-D16, D40   il pannello del nodo: niente «null», UUID, prefissi «cliente:»/«partner:» o nomi tecnici; sezioni
                     richiudibili, il 2D grande, i gesti prima della provenienza, niente codici delle regole; «conti da
                     aggiornare» e «da aggiornare (lo calcola il riepilogo)» non ci sono più
  A03 D25, D39       «Cambia tipo»: l'aiuto segue la scelta e non cita le regole; dopo il gesto la pagina dice che cosa è
                     cambiato
  A04 D36            trascinare un pezzo vicino al bordo della scheda del nuovo padre apre «Sposta», con il bersaglio
                     segnato durante il trascinamento
  A05 D37            trascinare lo sfondo non porta l'albero fuori dal riquadro
  A06 D38            il clic destro su un nodo apre il menu del nodo, non quello del browser
  A07 D42, D43       la testata della bozza e la proposta dello STEP senza meccanica interna
  A08 D05 (rilievo 2 della revisione)  un 2D che c'è ma i cui byte su questo server non si aprono (S1 contenuto non
                     disponibile, come sul server vero): la scheda dice il file e «anteprima non disponibile» con il motivo,
                     mai «nessun 2D associato»
  A09 D43 (rilievo 3) la testata dice il motivo della verifica in una frase («la BOM non è ancora confermata… «Conferma
                     BOM»» con lo STEP sorgente confermato), mai il codice del motore riscritto in minuscolo
  A10 D14 (rilievo 6) la linguetta «Documenti del nodo» senza codici del motore («disegno_2d», «struttura.…»)

Dati di prova: quelli del banco (ACME, ACM-xxxx), più i 2D del motore per due nodi (prodotti_valutati[].
strutture_candidate[].nodi[].disegni) aggiunti alla risposta di /motore-a/dati da questo script (page.route), dichiarati
tali. Le schermate vanno nella cartella --foto.
"""
import argparse
import hashlib
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from playwright.sync_api import expect, sync_playwright  # noqa: E402

import distinta_unica as du  # noqa: E402

T_BREVE = du.T_BREVE
T_LUNGO = du.T_LUNGO
verifica = du.verifica
sid = du.sid
UUID_RE = re.compile(r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}", re.I)
SNAKE_RE = re.compile(r"\b[a-z]{2,}_[a-z0-9_]+\b")
S1_RE = re.compile(r"/thread/[^/]+/sorgente(\?[^/]*)?$")

PROVE = []


def prova(numero, nome):
    def deco(fn):
        PROVE.append((numero, nome, fn))
        return fn
    return deco


def inietta_disegni(b):
    """Un gestore di route per GET /motore-a/dati: i 2D del motore (disegni.primario con l'anteprima) per i nodi #40 e #41
    dello STEP del finito, come li dà l'esito vero (valutazione.NodoBOM). Dati di prova dichiarati tali."""
    a1 = b.al["ACM-1000_assieme.stp"]
    pdf40 = b.al["ACM-1400_ASSIEME.pdf"]
    pdf41 = b.al["ACM-1400_A.pdf"]

    def nodo(sha, chiave, aid, nome):
        sha_pdf = hashlib.sha256(aid.encode("utf-8")).hexdigest()
        return {"nodo": {"rif": "nodo:%s:%s" % (sha, chiave), "allegato_id": a1}, "tipo": "sottoassieme", "origine_tipo": "",
                "classificazione": {}, "disegni": {"primario": {"allegato_id": aid, "sha256": sha_pdf, "nome_file": nome, "estensione": "pdf",
                                                                  "anteprima": {"sha256": sha_pdf, "allegato_id": aid, "disponibile": True}}}}

    def gestore(route):
        if "sorgente=1" in route.request.url:
            route.continue_()
            return
        r = route.fetch()
        try:
            j = r.json()
        except Exception:
            route.fulfill(response=r)
            return
        sha = ""
        for g in j.get("gruppi") or []:
            if g.get("allegato_id") == a1:
                sha = str(g.get("radice", "")).split(":")[1] if ":" in str(g.get("radice", "")) else ""
        es = j.get("esito") or {}
        for pv in es.get("prodotti_valutati") or []:
            if pv.get("componente_id") == b.p1 and sha:
                pv["strutture_candidate"] = [{"allegato_id": a1, "sha256": sha, "radice": "nodo:%s:#1" % sha, "nodi": [
                    nodo(sha, "#40", pdf40, "ACM-1400_ASSIEME.pdf"), nodo(sha, "#41", pdf41, "ACM-1400_A.pdf")]}]
                # A09: i motivi del prodotto come li dà il motore vero (valutazione.MotivoProdotto), più un codice ignoto
                if getattr(b, "motivi_prodotto", None) is not None:
                    pv["motivi"] = list(b.motivi_prodotto)
        route.fulfill(response=r, json=j)
    return gestore


def rett(p, sel):
    return p.evaluate("s => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect(); "
                      "return {x: r.left, y: r.top, w: r.width, h: r.height, cx: r.left + r.width / 2, cy: r.top + r.height / 2}; }", sel)


def scheda(b, k):
    """Sceglie la scheda k senza cambiare la vista (niente «Adatta»: la disposizione resta quella dell'utente)."""
    p = b.page
    c = p.locator("#nc-" + sid(k) + " .du-nc-cod")
    c.scroll_into_view_if_needed()
    c.click()
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", k, timeout=T_BREVE)


def testo_visibile(p, sel):
    return p.locator(sel).inner_text()


@prova("A01", "D05 D06 D07: le anteprime dei 2D nei nodi, la disposizione di prima, la quantità «dal file · da confermare» e il tipo «?»")
def a01(b):
    p = b.page
    p.route(re.compile(r"/motore-a/dati(\?|$)"), inietta_disegni(b))
    b.apri()
    a1 = b.al["ACM-1000_assieme.stp"]
    b.guarda(a1)
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    b.ok_finestra()
    expect(p.locator("#du-passo-revisione")).to_contain_text("versione 1", timeout=T_LUNGO)
    b.passo("revisione")
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10, timeout=T_LUNGO)
    b.attendi(lambda: p.locator("#du-vp .du-nc-mini img").count() >= 4, "le miniature non arrivano")
    b.foto("A01_apertura_dopo.png")
    p.click("#du-zoom-adatta")
    # D05: la miniatura del 2D nella scheda (la pagina 1 del PDF, disegnata da pdf.js), dal motore (k-40, k-41) e dalla
    # proposta del registro (k-7, k-11); «nessun 2D associato» dove non c'è
    for k in ("k-7", "k-11", "k-40", "k-41"):
        img = p.locator("#nc-%s .du-nc-mini img" % k)
        expect(img).to_have_count(1, timeout=T_LUNGO)
        b.attendi(lambda: img.evaluate("i => i.complete && i.naturalWidth > 0"), "la miniatura di %s non si disegna" % k)
    for k in ("k-9", "k-31"):
        expect(p.locator("#nc-%s .du-nc-mini" % k)).to_contain_text("nessun 2D associato")
    # D06: la disposizione dell'organigramma di prima: i figli di un padre in una riga sola, il padre centrato sopra
    tops = {}
    for k in ("k-root", "k-7", "k-40", "k-9", "k-11", "k-12", "k-31", "k-32", "k-33", "k-41"):
        r = rett(p, "#nc-" + sid(k))
        verifica(r is not None, "manca la scheda %s" % k)
        tops[k] = r
    verifica(abs(tops["k-7"]["y"] - tops["k-40"]["y"]) < 1.5, "i figli del prodotto non stanno in una riga: %s" % {k: round(tops[k]["y"]) for k in ("k-7", "k-40")})
    riga = [tops[k]["y"] for k in ("k-9", "k-11", "k-12", "k-31", "k-32", "k-33")]
    verifica(max(riga) - min(riga) < 1.5, "i figli del telaio non stanno in una riga: %s" % [round(y) for y in riga])
    verifica(tops["k-9"]["y"] > tops["k-7"]["y"] + tops["k-7"]["h"], "i figli del telaio non stanno sotto il telaio")
    xs = sorted(tops[k]["x"] for k in ("k-9", "k-11", "k-12", "k-31", "k-32", "k-33"))
    verifica(all(xs[i + 1] - xs[i] >= tops["k-9"]["w"] for i in range(5)), "i figli del telaio non stanno affiancati: %s" % [round(x) for x in xs])
    cxs = [tops[k]["cx"] for k in ("k-9", "k-11", "k-12", "k-31", "k-32", "k-33")]
    mezzo = (min(cxs) + max(cxs)) / 2
    verifica(abs(tops["k-7"]["cx"] - mezzo) < 2, "il telaio non è centrato sopra i suoi figli: %.1f contro %.1f" % (tops["k-7"]["cx"], mezzo))
    verifica(abs(tops["k-40"]["cx"] - tops["k-41"]["cx"]) < 2, "il supporto non è centrato sopra il piede")
    mezzo = (tops["k-7"]["cx"] + tops["k-40"]["cx"]) / 2
    verifica(abs(tops["k-root"]["cx"] - mezzo) < 2, "il prodotto non è centrato sopra i suoi figli")
    # D07: la quantità del legame del file e «da confermare», il tipo proposto con «?», niente «—» al posto della descrizione
    q = p.locator("#oq-" + sid("k-7>k-11"))
    expect(q).to_contain_text("×4 dal file")
    expect(q).to_contain_text("da confermare")
    expect(p.locator("#nc-k-11 .du-tipo")).to_contain_text("?")
    verifica(p.locator("#du-vp .du-nc-desc", has_text=re.compile(r"^—$")).count() == 0, "una scheda dice «—» al posto della descrizione")
    expect(p.locator("#du-legenda")).to_contain_text("legame del file")
    b.foto("A01_organigramma_dopo.png")


@prova("A02", "D13 D14 D15 D16 D40: il pannello del nodo senza testi grezzi, a sezioni, con il 2D e i gesti in vista")
def a02(b):
    p = b.page
    scheda(b, "k-9")
    pan = p.locator("#du-pannello")
    t = pan.inner_text()
    for grezzo in ("null", "undefined", "[object Object]", "partner:", "cliente:", "nodo:", "cand:", "u:step", "non_verificabile", "lettura_ruolo",
                   "documentato_da", "da aggiornare", "E37", "R103", "DB7-3"):
        verifica(grezzo not in t, "il pannello del nodo dice «%s»:\n%s" % (grezzo, t[:1500]))
    u = UUID_RE.search(t)
    verifica(not u, "il pannello del nodo mostra un UUID: %s" % (u.group(0) if u else ""))
    expect(pan).to_contain_text("fornitore beta")
    # D15: sezioni richiudibili come nella pagina di prima, il 2D grande del nodo, i gesti prima della provenienza
    verifica(pan.locator("details.du-sez").count() >= 4, "il pannello non ha le sezioni richiudibili")
    expect(pan.locator("#du-nodo-2d")).to_be_visible()
    ordine = p.evaluate("() => [...document.querySelectorAll('#du-pannello .du-sez')].map(e => e.id)")
    verifica("du-gesti" in ordine and "du-sez-provenienza" in ordine and ordine.index("du-gesti") < ordine.index("du-sez-provenienza"),
             "i gesti non vengono prima della provenienza: %s" % ordine)
    # la provenienza tecnica sta in un dettaglio chiuso: il nome del file si vede, gli identificativi no
    expect(p.locator("#du-sez-provenienza")).to_contain_text("ACM-1000_assieme.stp")
    # D13: niente «null» accanto a «Conferma il tipo proposto»
    verifica("null" not in p.locator("#du-sez-categoria").inner_text(), "«Conferma il tipo proposto null»")
    # D40: su un particolare i gesti che non valgono non ci sono, senza codici delle regole
    scheda(b, "k-11")
    g = p.locator("#du-gesti").inner_text()
    verifica("E37" not in g and "Aggiungi un pezzo sotto" not in g and "Ricolloca i figli" not in g, "gesti spenti con i codici delle regole:\n%s" % g)
    # D16: la barra non dice «conti da aggiornare»
    verifica("da aggiornare" not in p.locator("#du-barra").inner_text(), "la barra dice «conti da aggiornare»")
    b.foto("A02_pannello_dopo.png")


@prova("A03", "D25 D39: «Cambia tipo», l'aiuto che segue la scelta e l'esito del gesto")
def a03(b):
    p = b.page
    scheda(b, "k-12")
    p.click("#du-cambia-tipo")
    f = b.finestra()
    expect(f).to_be_visible()
    p.select_option("#f-tipo", "sottoassieme")
    aiuto_ass = f.locator("#f-tipo-aiuto").inner_text()
    p.select_option("#f-tipo", "commerciale")
    aiuto_com = f.locator("#f-tipo-aiuto").inner_text()
    verifica(aiuto_ass != aiuto_com, "l'aiuto di «Cambia tipo» non segue la scelta: %r" % aiuto_ass)
    tutto = f.inner_text()
    verifica("E37" not in tutto and "R103" not in tutto and "TIPO_EFFETTO" not in tutto, "«Cambia tipo» cita le regole:\n%s" % tutto)
    p.select_option("#f-tipo", "sottoassieme")
    b.foto("A03_cambia_tipo_dopo.png")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Assieme", timeout=T_BREVE)
    expect(b.avviso()).to_contain_text("Piastrina")
    expect(p.locator("#nc-k-12 .du-tipo")).to_contain_text("Assieme")


@prova("A04", "D36: trascinare un pezzo accanto al bordo della scheda del nuovo padre apre «Sposta», con il bersaglio segnato")
def a04(b):
    p = b.page
    b.salvata(2)
    da = rett(p, "#nc-k-11 .du-nc-cod")
    a = rett(p, "#nc-k-12")
    verifica(da and a, "schede mancanti")
    p.mouse.move(da["cx"], da["cy"])
    p.mouse.down()
    p.mouse.move(da["cx"] + 10, da["cy"] + 10, steps=3)
    # il rilascio poco sotto il bordo inferiore della scheda (dentro il margine di tolleranza)
    x, y = a["cx"], a["y"] + a["h"] + 6
    p.mouse.move(x, y, steps=12)
    expect(p.locator("#nc-k-12")).to_have_class(re.compile(r"du-nc--drop"), timeout=T_BREVE)
    b.foto("A04_trascina_dopo.png")
    p.mouse.up()
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    expect(b.finestra()).to_contain_text("Sposta")
    b.chiudi_finestra()


@prova("A05", "D37: trascinare lo sfondo non porta l'albero fuori dal riquadro")
def a05(b):
    p = b.page
    p.click("#du-zoom-adatta")
    vp = rett(p, "#du-vp")
    # un punto dello sfondo, lontano dalle schede: l'angolo in basso a sinistra della parte visibile del riquadro
    alto = p.evaluate("() => window.innerHeight")
    x0, y0 = vp["x"] + 8, min(vp["y"] + vp["h"], alto) - 12
    p.mouse.move(x0, y0)
    p.mouse.down()
    p.mouse.move(x0 + 3000, y0 + 3000, steps=20)
    p.mouse.up()
    dentro = p.evaluate("""() => { const v = document.querySelector('#du-vp').getBoundingClientRect();
        return [...document.querySelectorAll('#du-vp .du-nc[data-k]')].filter(e => { const r = e.getBoundingClientRect();
          return r.right > v.left + 20 && r.left < v.right - 20 && r.bottom > v.top + 20 && r.top < v.bottom - 20; }).length; }""")
    b.foto("A05_pan_dopo.png")
    verifica(dentro > 0, "dopo il trascinamento dello sfondo nessuna scheda resta nel riquadro")
    sel = p.evaluate("() => String(window.getSelection())")
    verifica(not sel.strip(), "trascinare lo sfondo seleziona il testo della pagina: %r" % sel[:120])
    p.click("#du-zoom-adatta")


@prova("A06", "D38: il clic destro su un nodo apre il menu del nodo")
def a06(b):
    p = b.page
    p.locator("#nc-k-11 .du-nc-cod").click(button="right")
    m = p.locator("#du-menu-nodo")
    expect(m).to_be_visible(timeout=T_BREVE)
    expect(m).to_contain_text("Cambia tipo")
    expect(m).to_contain_text("Sposta sotto")
    b.foto("A06_menu_nodo_dopo.png")
    m.get_by_role("menuitem", name=re.compile("Cambia tipo")).click()
    expect(b.finestra()).to_be_visible(timeout=T_BREVE)
    expect(m).to_be_hidden()
    b.chiudi_finestra()


@prova("A07", "D42 D43: la testata della bozza e la proposta dello STEP senza meccanica interna")
def a07(b):
    p = b.page
    st = p.locator("#du-stato-conferma").inner_text()
    verifica("calcolo di oggi" not in st and "nessuna conferma" not in st, "la testata della bozza dice: %r" % st)
    pr = p.locator("#du-proposta-step").inner_text()
    verifica("L'import porta nella bozza solo gli elementi" not in pr, "la proposta dello STEP ripete la spiegazione dell'import: %r" % pr)
    expect(p.locator("#du-proposta-step")).to_contain_text("non è nella bozza")
    b.foto("A07_testata_dopo.png")


def chiudi_contenuto(b, allegato, motivo):
    """Un gestore di route per GET /thread/{id}/sorgente (S1): il file dato con azioni.contenuto non disponibile e il
    motivo, come S1 lo dice sul server vero per un allegato che non è nello staging. Dati di prova dichiarati tali."""
    def gestore(route):
        r = route.fetch()
        try:
            j = r.json()
        except Exception:
            route.fulfill(response=r)
            return
        for f in j.get("file") or []:
            if f.get("allegato_id") == allegato:
                f.setdefault("azioni", {})["contenuto"] = {"disponibile": False, "motivo": motivo, "richiede": ["scarica"]}
        route.fulfill(response=r, json=j)
    return gestore


@prova("A08", "D05, rilievo 2: un 2D che c'è ma non si apre su questo server non è «nessun 2D associato»")
def a08(b):
    p = b.page
    pdf40 = b.al["ACM-1400_ASSIEME.pdf"]
    motivo = "il file non è nello staging: scaricalo dalla mail con «Scarica»"
    p.route(S1_RE, chiudi_contenuto(b, pdf40, motivo))
    b.apri()
    b.passo("revisione")
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10, timeout=T_LUNGO)
    m = p.locator("#nc-k-40 .du-nc-mini")
    expect(m).to_have_count(1, timeout=T_LUNGO)
    t = m.inner_text()
    print("      scheda k-40: %r · suggerimento %r" % (t, m.get_attribute("title")))
    verifica("nessun 2D" not in t, "la scheda con il 2D del motore dice «nessun 2D associato»: %r" % t)
    expect(m).to_contain_text("anteprima non disponibile")
    expect(m).to_contain_text("ACM-1400_ASSIEME.pdf")
    expect(m).to_have_attribute("title", re.compile("non è nello staging"))
    # il pannello dice il motivo per intero, e non c'è «Apri il 2D» per un file che non si apre
    scheda(b, "k-40")
    expect(p.locator("#du-nodo-2d")).to_contain_text("non è nello staging")
    # il nodo senza 2D resta «nessun 2D associato»; quello con il 2D che si apre ha la miniatura
    expect(p.locator("#nc-k-9 .du-nc-mini")).to_contain_text("nessun 2D associato")
    img = p.locator("#nc-k-41 .du-nc-mini img")
    expect(img).to_have_count(1, timeout=T_LUNGO)
    p.locator("#nc-k-40 .du-nc-cod").click(button="right")
    expect(p.locator("#du-menu-nodo")).to_be_visible(timeout=T_BREVE)
    verifica(p.locator("#du-menu-nodo").get_by_role("menuitem", name="Apri il 2D").count() == 0, "il menu offre «Apri il 2D» per un file che non si apre")
    p.keyboard.press("Escape")
    b.foto("A08_2d_non_apribile_dopo.png")
    p.unroute(S1_RE)


@prova("A09", "D43, rilievo 3: il motivo della verifica in una frase, mai il codice del motore")
def a09(b):
    p = b.page
    b.motivi_prodotto = ["fonte_strutturale_step_mancante_o_non_confermata", "nomenclatura_non_verificata", "motivo_ignoto_di_prova"]
    try:
        b.apri()
        b.passo("revisione")
        va = p.locator("#du-verifica-adesso")
        expect(va).to_be_visible(timeout=T_LUNGO)
        t = va.inner_text()
        tit = va.locator("[title]").first.get_attribute("title") if va.locator("[title]").count() else ""
        print("      testata: %r · suggerimento %r" % (t, tit))
        for grezzo in ("fonte strutturale", "step mancante", "_", "motivo ignoto"):
            verifica(grezzo not in t, "la testata dice «%s»: %r" % (grezzo, t))
        # lo STEP sorgente del prodotto è confermato da S8 (A01): manca solo «Conferma BOM»
        expect(va).to_contain_text("Verificata: no — la BOM non è ancora confermata")
        expect(va).to_contain_text("Conferma BOM")
        verifica("i codici della BOM non sono verificati" in (tit or ""), "il suggerimento non dice gli altri motivi: %r" % tit)
        b.foto("A09_verifica_dopo.png")
    finally:
        b.motivi_prodotto = None


@prova("A10", "D14, rilievo 6: la linguetta «Documenti del nodo» senza codici del motore")
def a10(b):
    p = b.page
    b.apri()
    b.passo("revisione")
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10, timeout=T_LUNGO)
    visti = 0
    for k in ("k-9", "k-11", "k-12", "k-31", "k-41"):
        scheda(b, k)
        if p.locator("#du-ling-documenti").count():
            b.linguetta("documenti")
        d = p.locator("#du-sez-documenti")
        if not d.count():
            continue
        visti += 1
        # l'elenco dei documenti e le righe della completezza (i nomi nella tendina dei file da smistare sono nomi di file)
        t = "\n".join(p.locator(s).first.inner_text() for s in ("#du-doc-nodo", "#du-completezza") if p.locator(s).count())
        for grezzo in ("disegno_2d", "struttura.", "occorrenze_omesse"):
            verifica(grezzo not in t, "«Documenti del nodo» di %s dice «%s»: %s" % (k, grezzo, t[:800]))
        cod = SNAKE_RE.search(t)
        verifica(not cod, "«Documenti del nodo» di %s scrive un codice: %s" % (k, cod.group(0) if cod else ""))
        if p.locator("#du-ling-dettaglio").count():
            b.linguetta("dettaglio")
    verifica(visti > 0, "nessuna linguetta «Documenti del nodo» trovata")
    b.foto("A10_documenti_dopo.png")



def stato_file(b, allegato, stato, motivo):
    """Un gestore di route per S1: il file dato con lo stato (scartato, altri_file) e il contenuto non disponibile, come S1
    lo dice sul server vero per un file scartato o messo in Altri file. Dati di prova dichiarati tali."""
    def gestore(route):
        r = route.fetch()
        try:
            j = r.json()
        except Exception:
            route.fulfill(response=r)
            return
        for f in j.get("file") or []:
            if f.get("allegato_id") == allegato:
                f["stato"] = stato
                f.setdefault("azioni", {})["contenuto"] = {"disponibile": False, "motivo": motivo, "richiede": []}
        route.fulfill(response=r, json=j)
    return gestore


@prova("A11", "seconda revisione ALBERO, rilievo A: il 2D primario del motore scartato o in Altri file non è più il 2D del pezzo")
def a11(b):
    p = b.page
    pdf40 = b.al["ACM-1400_ASSIEME.pdf"]
    for stato, motivo in (("scartato", "il file è scartato in questa RFQ: per riaverlo, ricaricalo"),
                          ("altri_file", "la copia di conservazione non si apre su questo server")):
        p.route(S1_RE, stato_file(b, pdf40, stato, motivo))
        try:
            b.apri()
            b.passo("revisione")
            expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10, timeout=T_LUNGO)
            m = p.locator("#nc-k-40 .du-nc-mini")
            expect(m).to_have_count(1, timeout=T_LUNGO)
            p.wait_for_timeout(800)
            t = m.inner_text() + " " + (m.get_attribute("title") or "")
            print("      %s: scheda k-40 %r" % (stato, t))
            verifica("ACM-1400_ASSIEME.pdf" not in t, "il 2D primario %s resta il 2D della scheda k-40: %r" % (stato, t))
            verifica("scartato" not in t and "conservazione" not in t, "la scheda k-40 parla del file %s: %r" % (stato, t))
            b.foto("A11_primario_%s_dopo.png" % stato)
        finally:
            p.unroute(S1_RE)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="")
    ap.add_argument("--foto", default="")
    ap.add_argument("--suffisso", default="")
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
        foto = b.foto

        def foto_con_suffisso(nome):
            if a.suffisso:
                nome = nome.replace("_dopo", "_" + a.suffisso)
            foto(nome)
        b.foto = foto_con_suffisso
        for numero, nome, fn in PROVE:
            if scelte and numero not in scelte:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  DU-%s  %-100s ESEGUITA PASSATA  (%.1fs)" % (numero, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(numero)
                testo = str(e)
                print("  DU-%s  %-100s FALLITA  %s" % (numero, nome, testo.splitlines()[0] if testo else repr(e)))
                if "\n" in testo:
                    print("         " + "\n         ".join(testo.splitlines()[1:12]))
                b.foto("errore_%s_dopo.png" % numero)
                try:
                    for _ in range(2):
                        page.keyboard.press("Escape")
                        page.wait_for_timeout(150)
                    page.mouse.up()
                except Exception:
                    pass
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
