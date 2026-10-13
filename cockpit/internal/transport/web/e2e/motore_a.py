# -*- coding: utf-8 -*-
"""Browser - giro 5, A1d-P4, passo B5.1 dopo la direttiva 12 del 13/10: la Distinta nuova del motore A
(/thread/<id>/motore-a) in un BROWSER VERO, Scenario H (direttiva 12, par. 10), rilievi R-01 e R-02 (par. 7), D-18 = A
(par. 9).

Non lo si lancia a mano: lo avvia `motore_a_browser_test.go` (tag `integrazione browser`), con la scena di quel file
(la B87 delle L4 di A1d con un secondo prodotto finito, il flag acceso con le regole ACME) e la «sonda»: GET = lo stato
delle proposte della RFQ e il numero dei job; POST /regole-non-pronte e /regole-pronte cambiano le regole del server.

Perche' serve un browser. Le L1 provano che il server, data una richiesta, tiene prodotto e vista e scrive gli
attributi giusti. Che il modulo della pagina riscriva davvero l'indirizzo dopo la rilettura (e quindi che htmx lo
rimandi in HX-Current-URL), che dopo un gesto e dopo «Aggiorna» si resti sul secondo prodotto nella vista Documenti, che
il frammento d'errore non sembri un successo: questo lo vede solo un browser.

La pagina e' dell'amministratore: lo script entra con la sigla e la password che gli da' il test. Legge la pagina con i
selettori stabili: #ma-avviso[data-esito], #ma, la radice .ma (data-ma-pagina, data-ma-prodotto, data-ma-vista,
data-ma-calcolo, data-ma-errore, data-ma-sorgente, data-ma-incompleto), #ma-viste, #ma-prodotti, #ma-da-smistare,
[data-ma-azione=aggiorna], #ma-diagnostiche, #ma-incompleto, #ma-errore, #ma-scelta-predefinita.

Le prove (cifre, scelte dal test Go con --prove; si eseguono nell'ordine dato, sulla stessa RFQ):
  1  l'apertura senza query: nessuna richiesta che non sia GET, la sonda invariata, l'indirizzo riscritto con il primo
     prodotto e la vista distinta
  2  il secondo prodotto e la vista Documenti dai collegamenti: radice, schede correnti e indirizzo concordi
  3  «Scarta il file» sul secondo prodotto nella vista Documenti: «ok», #ma riletto senza ricaricare, prodotto e vista
     uguali, la sorgente «indirizzo», la proposta scartata nella sonda. Il file «Da smistare» vale per tutta la RFQ:
     il gesto e' fatto guardando il secondo prodotto, ma non modifica un dato del secondo prodotto; i gesti per
     prodotto sono spenti (R1-R14) e andranno qui quando B5.3/B5.4 li accendono (revisione D-E del 13/10, rilievo 11)
  4  «Aggiorna»: lo stesso
  5  le diagnostiche del thread visibili; con l'esito incompleto nessun badge di verifica positivo
  6  il calcolo che non riesce e «Aggiorna»: errore visibile, avviso «errore», indirizzo intatto; poi di nuovo pronto,
     e l'avviso non dice piu' che la pagina e' senza esito (revisione D-E del 13/10, rilievo 4)
  7  l'impaginazione con l'anteprima aperta (collaudo dell'utente del 13/10: il pannello del pezzo scelto scendeva sopra
     il riepilogo documentale, testi uno sopra l'altro), a 1900x950, 1440x900, 1280x800 (due colonne) e 1024x768 e nel
     tema scuro del sistema:
     una casella scelta, la vista Documenti, tre posizioni di scorrimento; i blocchi del workspace (centro, anteprima,
     «Da smistare», riepilogo documentale) non si sovrappongono, il pannello ha uno sfondo pieno, la pagina non scorre
     di lato, i gesti spenti delle barre non si accavallano; «Ispeziona» apre la finestra modale sopra tutto e la
     richiude. Nessuna scrittura (la sonda resta uguale)

I clienti, i codici e i nomi dei file sono inventati (ACME, 712xxxx, acme.example).
"""
import argparse
import json
import os
import sys
import time
import urllib.request

from playwright.sync_api import sync_playwright

PROVE = []

# Le frasi del modulo (web/static/motore_a.mjs): se cambiano la', cambiano qui.
PAGINA_SENZA_ESITO = "La pagina si è riletta, ma il server non ha calcolato l'esito"
PAGINA_DI_NUOVO_CON_ESITO = "La pagina si è riletta con l'esito calcolato dal server."


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


JS_RADICE = r"""() => {
  const ma = document.getElementById('ma');
  const r = document.querySelector('#ma > .ma');
  if (!ma || !r || ma.children.length !== 1) return null;
  const d = r.dataset;
  return {pagina: d.maPagina || '', prodotto: d.maProdotto || '', vista: d.maVista || '', calcolo: d.maCalcolo || '',
          errore: d.maErrore || '', sorgente: d.maSorgente || '', incompleto: d.maIncompleto || ''};
}"""

JS_INDIRIZZO = "() => Object.fromEntries(new URLSearchParams(location.search))"

# segna la radice di adesso e la finestra: dopo la rilettura la radice e' un'altra, la finestra la stessa
JS_SEGNA = r"""() => {
  const r = document.querySelector('#ma > .ma');
  if (r) r.__vecchia = 1;
  window.__segno = 'stessa pagina';
}"""

JS_RILETTA = r"""() => {
  const ma = document.getElementById('ma');
  const r = document.querySelector('#ma > .ma');
  return !!(ma && r && !r.__vecchia && !ma.hasAttribute('aria-busy'));
}"""

# i badge che dicono vero un booleano di verifica del server senza essere sospesi, visibili
JS_VERIFICHE_POSITIVE = r"""() => [...document.querySelectorAll('#ma [title^="prodotto_verificato = true"], #ma [title^="bom.verificata = true"]')]
  .filter((e) => !e.hasAttribute('data-ma-verifica-sospesa') && e.offsetParent !== null).map((e) => e.textContent.trim())"""

# le schede correnti: prodotto (title = Rif) e vista (testo)
JS_CORRENTI = r"""() => ({
  prodotti: [...document.querySelectorAll('#ma-prodotti a[aria-current]')].map((a) => a.title),
  viste: [...document.querySelectorAll('#ma-viste a[aria-current]')].map((a) => a.textContent.trim()),
})"""


class Banco:
    def __init__(self, page, a):
        self.page, self.a = page, a
        self.pagina = a.url + "/thread/" + a.thread + "/motore-a"
        self.richieste = []
        self.dialoghi = []
        page.on("request", lambda r: self.richieste.append((r.method, r.url)))

    def foto(self, nome):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.a.foto, nome), full_page=False)

    def sonda(self, percorso="", post=False):
        req = urllib.request.Request(self.a.sonda + percorso, method="POST" if post else "GET", data=b"" if post else None)
        with urllib.request.urlopen(req, timeout=10) as r:
            corpo = r.read()
            return json.loads(corpo) if corpo else None

    def radice(self):
        r = self.page.evaluate(JS_RADICE)
        verifica(r is not None, "#ma non ha un solo figlio, la radice .ma")
        verifica(r["pagina"].lower() == self.a.thread.lower(), "la radice e' di un'altra RFQ: %s" % r["pagina"])
        return r

    def aspetta(self, js, messaggio, entro=15000, arg=None):
        try:
            self.page.wait_for_function(js, arg=arg, timeout=entro)
        except Exception:
            raise Rotto(messaggio)

    def aspetta_scelta(self, prodotto, vista, messaggio):
        self.aspetta("([p, v]) => { const r = document.querySelector('#ma > .ma'); const q = new URLSearchParams(location.search);"
                     " return r && r.dataset.maProdotto === p && r.dataset.maVista === v && q.get('prodotto') === p && q.get('vista') === v; }",
                     messaggio, arg=[prodotto, vista])

    def stessa_pagina(self):
        verifica(self.page.evaluate("window.__segno") == "stessa pagina", "la pagina si e' ricaricata: la rilettura doveva cambiare solo #ma")

    def correnti(self, prodotto, vista, contesto):
        c = self.page.evaluate(JS_CORRENTI)
        verifica(c["prodotti"] == [prodotto], "%s: scheda corrente del prodotto %s, attesa %s" % (contesto, c["prodotti"], prodotto))
        verifica(c["viste"] == [vista], "%s: vista corrente %s, attesa %s" % (contesto, c["viste"], vista))

    def senza_strisce_di_scelta(self, contesto):
        for s in ("#ma-scelta-predefinita", "#ma-prodotto-non-trovato", "#ma-vista-non-valida"):
            verifica(self.page.locator(s).count() == 0, "%s: c'e' %s" % (contesto, s))

    def aggiorna(self):
        self.page.evaluate(JS_SEGNA)
        self.page.locator("#ma [data-ma-azione=aggiorna]").click()
        self.aspetta(JS_RILETTA, "«Aggiorna» non ha riletto #ma")


@prova("1", "l'apertura senza query: nessuna scrittura, l'indirizzo riscritto con la scelta resa")
def prova_1(b):
    prima = b.sonda()
    b.richieste.clear()
    b.page.goto(b.pagina)
    b.page.wait_for_selector("#ma > .ma")
    r = b.radice()
    verifica(r["calcolo"] in ("completo", "incompleto"), "all'apertura data-ma-calcolo = %r" % r["calcolo"])
    verifica(r["prodotto"] == b.a.primo and r["vista"] == "distinta", "aperta senza query: %s %s" % (r["prodotto"], r["vista"]))
    verifica(r["sorgente"] == "nessuna", "aperta senza query: la sorgente e' %r" % r["sorgente"])
    b.aspetta_scelta(b.a.primo, "distinta", "il modulo non ha riscritto l'indirizzo con il primo prodotto e la vista distinta: %s" % b.page.url)
    b.page.wait_for_timeout(1000)
    verifica(not [m for m, u in b.richieste if m != "GET"], "richieste che non sono GET all'apertura: %s" % [x for x in b.richieste if x[0] != "GET"])
    verifica(b.sonda() == prima, "l'apertura ha cambiato la sonda")
    b.foto("h1_apertura.png")


@prova("2", "il secondo prodotto e la vista Documenti dai collegamenti")
def prova_2(b):
    i = b.page.evaluate("(rif) => [...document.querySelectorAll('#ma-prodotti a.ma-ptab')].findIndex((a) => a.title === rif)", b.a.secondo)
    verifica(i >= 0, "nessuna scheda per il secondo prodotto %s" % b.a.secondo)
    b.page.locator("#ma-prodotti a.ma-ptab").nth(i).click()
    b.aspetta_scelta(b.a.secondo, "distinta", "la scheda del secondo prodotto non porta a quel prodotto: %s" % b.page.url)
    b.page.locator("#ma-viste a:has-text('Documenti')").click()
    b.aspetta_scelta(b.a.secondo, "documenti", "«Documenti» non porta alla vista documenti del secondo prodotto: %s" % b.page.url)
    r = b.radice()
    verifica(r["sorgente"] == "query", "dal collegamento la sorgente e' %r" % r["sorgente"])
    b.correnti(b.a.secondo, "Documenti", "dopo i collegamenti")
    b.foto("h2_secondo_documenti.png")


@prova("3", "un gesto sul secondo prodotto nella vista Documenti: la rilettura tiene prodotto e vista")
def prova_3(b):
    r = b.radice()
    verifica(r["prodotto"] == b.a.secondo and r["vista"] == "documenti", "la prova parte da %s %s (va dopo la 2)" % (r["prodotto"], r["vista"]))
    prima = b.sonda()
    gesti = b.page.locator("#ma-da-smistare button[data-ma-gesto^='/proposta/'][data-ma-gesto$='/scarta']:not([disabled])")
    verifica(gesti.count() >= 1, "nessun «Scarta il file» acceso nella vista Documenti del secondo prodotto")
    b.page.evaluate(JS_SEGNA)
    b.dialoghi.clear()
    gesti.first.click()
    b.aspetta("() => { const a = document.getElementById('ma-avviso'); return a && a.dataset.esito === 'ok'; }",
              "l'avviso non dice «ok» dopo «Scarta il file»: %r" % b.page.text_content("#ma-avviso"))
    b.aspetta(JS_RILETTA, "dopo il gesto #ma non si e' riletto")
    verifica(b.dialoghi and "Scartare il file" in b.dialoghi[-1], "nessuna domanda prima dello scarto: %s" % b.dialoghi)
    b.stessa_pagina()
    b.aspetta_scelta(b.a.secondo, "documenti", "dopo il gesto la pagina non e' piu' sul secondo prodotto nella vista Documenti: %s" % b.page.url)
    r = b.radice()
    verifica(r["sorgente"] == "indirizzo", "la rilettura non ha preso la scelta dall'indirizzo: sorgente %r" % r["sorgente"])
    b.correnti(b.a.secondo, "Documenti", "dopo il gesto")
    b.senza_strisce_di_scelta("dopo il gesto")
    dopo = b.sonda()
    cambiate = [k for k in set(prima["proposte"]) | set(dopo["proposte"]) if prima["proposte"].get(k) != dopo["proposte"].get(k)]
    verifica(len(cambiate) == 1 and dopo["proposte"][cambiate[0]] == "scartata", "la sonda dopo lo scarto: %s -> %s" % (prima, dopo))
    b.foto("h3_dopo_il_gesto.png")


@prova("4", "«Aggiorna» tiene prodotto e vista")
def prova_4(b):
    b.aggiorna()
    b.stessa_pagina()
    b.aspetta_scelta(b.a.secondo, "documenti", "dopo «Aggiorna» la pagina non e' piu' sul secondo prodotto nella vista Documenti: %s" % b.page.url)
    r = b.radice()
    verifica(r["sorgente"] == "indirizzo", "«Aggiorna»: sorgente %r" % r["sorgente"])
    b.correnti(b.a.secondo, "Documenti", "dopo «Aggiorna»")
    b.senza_strisce_di_scelta("dopo «Aggiorna»")


@prova("5", "diagnostiche visibili; con l'esito incompleto nessun «verificato»")
def prova_5(b):
    d = b.page.locator("#ma-diagnostiche")
    verifica(d.count() == 1 and d.is_visible(), "il riquadro delle diagnostiche non si vede")
    righe = b.page.locator("#ma-diagnostiche [data-ma-diagnostica]")
    verifica(righe.count() == b.a.diagnostiche, "%d diagnostiche a vista, %d nell'esito" % (righe.count(), b.a.diagnostiche))
    for i in range(righe.count()):
        verifica(righe.nth(i).is_visible(), "la diagnostica %d non si vede" % i)
    r = b.radice()
    positivi = b.page.evaluate(JS_VERIFICHE_POSITIVE)
    if r["calcolo"] == "incompleto":
        verifica(b.page.locator("#ma-incompleto").is_visible(), "esito incompleto senza la striscia #ma-incompleto")
        verifica(not positivi, "esito incompleto con badge di verifica positivi: %s" % positivi)
    else:
        verifica(r["calcolo"] == "completo", "data-ma-calcolo = %r" % r["calcolo"])
        verifica(b.page.locator("#ma-incompleto").count() == 0, "esito completo con la striscia #ma-incompleto")
    b.foto("h5_diagnostiche.png")


@prova("6", "il calcolo che non riesce: errore visibile, mai un successo; poi di nuovo pronto")
def prova_6(b):
    b.sonda("/regole-non-pronte", post=True)
    try:
        b.aggiorna()
        b.stessa_pagina()
        r = b.radice()
        verifica(r["calcolo"] == "errore" and r["errore"] == "regole_non_pronte", "frammento d'errore: calcolo %r, errore %r" % (r["calcolo"], r["errore"]))
        verifica(b.page.locator("#ma-errore").is_visible(), "la striscia dell'errore non si vede")
        av = b.page.locator("#ma-avviso")
        verifica(av.get_attribute("data-esito") == "errore", "l'avviso dopo il frammento d'errore ha data-esito = %r" % av.get_attribute("data-esito"))
        verifica(PAGINA_SENZA_ESITO in (av.text_content() or ""), "l'avviso non dice che la pagina e' senza esito: %r" % av.text_content())
        verifica(av.get_attribute("data-ma-errore") == "regole_non_pronte", "l'avviso non ha la tipologia: %r" % av.get_attribute("data-ma-errore"))
        verifica(not b.page.evaluate(JS_VERIFICHE_POSITIVE), "badge di verifica positivi senza esito")
        # la testa senza esito non inventa dati della RFQ (integrazione E del 13/10, prima corsa nel browser)
        verifica(b.page.locator("#ma-testa-non-letta").is_visible(), "la testa non dice che i dati della RFQ non sono letti")
        testo_ma = b.page.text_content("#ma") or ""
        for x in ("cliente non indicato", "senza oggetto", "nessuna fase aperta"):
            verifica(x not in testo_ma, "senza esito la pagina scrive %r come un dato della RFQ" % x)
        q = b.page.evaluate(JS_INDIRIZZO)
        verifica(q.get("prodotto") == b.a.secondo and q.get("vista") == "documenti", "il frammento d'errore ha cambiato l'indirizzo: %s" % b.page.url)
        b.foto("h6_errore.png")
    finally:
        b.sonda("/regole-pronte", post=True)
    b.aggiorna()
    b.stessa_pagina()
    b.aspetta_scelta(b.a.secondo, "documenti", "dopo l'errore «Aggiorna» non torna al secondo prodotto nella vista Documenti: %s" % b.page.url)
    r = b.radice()
    verifica(r["calcolo"] in ("completo", "incompleto"), "dopo l'errore data-ma-calcolo = %r" % r["calcolo"])
    # revisione D-E del 13/10, rilievo 4: con l'esito di nuovo calcolato l'avviso non dice piu' che la pagina e' senza esito
    av = b.page.locator("#ma-avviso")
    testo = av.text_content() or ""
    verifica(av.get_attribute("data-esito") != "errore", "dopo il ritorno l'avviso ha ancora data-esito = 'errore': %r" % testo)
    verifica(PAGINA_SENZA_ESITO not in testo, "dopo il ritorno l'avviso dice ancora che la pagina e' senza esito: %r" % testo)
    verifica(PAGINA_DI_NUOVO_CON_ESITO in testo, "dopo il ritorno l'avviso non dice che la pagina ha di nuovo l'esito: %r" % testo)
    verifica(av.get_attribute("data-ma-errore") in (None, ""), "dopo il ritorno l'avviso ha ancora la tipologia: %r" % av.get_attribute("data-ma-errore"))


# --------------------------------------------------------------------------- 7: impaginazione con l'anteprima aperta

# le tre del collaudo e 1280x800, dove il workspace ha due colonne (da 860 a 1180 px di contenitore: la colonna destra
# sticky con anteprima e «Da smistare»)
MISURE = [(1900, 950), (1440, 900), (1280, 800), (1024, 768)]

# i blocchi del workspace a vista (rettangoli nella finestra), il pannello scelto e i suoi sfondi, le sovrapposizioni;
# un blocco senza scatola (display: contents, hidden) non conta; un blocco dentro un riquadro che scorre (overflow non
# visible, come la colonna destra sticky da 860 a 1180 px) conta solo per la parte che il riquadro lascia vedere
JS_IMPAGINAZIONE = r"""() => {
  const r = document.querySelector('#ma > .ma');
  if (!r) return {errore: 'nessuna radice'};
  const visibile = (e) => !!e && e.getClientRects().length > 0 && getComputedStyle(e).visibility !== 'hidden';
  const scatola = (nome, e) => {
    if (!visibile(e)) return null;
    const b = e.getBoundingClientRect();
    let x = b.left, y = b.top, dx = b.right, giu = b.bottom;
    for (let p = e.parentElement; p && p !== r; p = p.parentElement) {
      const cs = getComputedStyle(p);
      if (cs.display === 'contents' || (cs.overflowX === 'visible' && cs.overflowY === 'visible')) continue;
      const c = p.getBoundingClientRect();
      x = Math.max(x, c.left); y = Math.max(y, c.top); dx = Math.min(dx, c.right); giu = Math.min(giu, c.bottom);
    }
    return dx - x > 0 && giu - y > 0 ? {nome, x, y, r: dx, b: giu} : null;
  };
  const pannello = r.querySelector('#ma-anteprima .ma-anteprima:not([hidden])');
  const blocchi = [
    scatola('centro', r.querySelector('.ma-ws-centro')),
    scatola('anteprima', r.querySelector('#ma-anteprima')),
    scatola('da smistare', r.querySelector('#ma-da-smistare')),
    scatola('riepilogo documentale', r.querySelector('#ma-documenti')),
  ].filter(Boolean);
  const sopra = [];
  for (let i = 0; i < blocchi.length; i++) for (let j = i + 1; j < blocchi.length; j++) {
    const a = blocchi[i], b = blocchi[j];
    const w = Math.min(a.r, b.r) - Math.max(a.x, b.x), h = Math.min(a.b, b.b) - Math.max(a.y, b.y);
    if (w > 1 && h > 1) sopra.push(a.nome + ' su ' + b.nome + ' (' + Math.round(w) + 'x' + Math.round(h) + ' px)');
  }
  // i gesti spenti di ogni barra del riepilogo: nessuno sopra un altro
  for (const barra of r.querySelectorAll('#ma-documenti .ma-toolbar')) {
    const na = [...barra.querySelectorAll(':scope > .ma-na')].map((e, k) => scatola('gesto spento ' + (k + 1), e)).filter(Boolean);
    for (let i = 0; i < na.length; i++) for (let j = i + 1; j < na.length; j++) {
      const a = na[i], b = na[j];
      if (Math.min(a.r, b.r) - Math.max(a.x, b.x) > 1 && Math.min(a.b, b.b) - Math.max(a.y, b.y) > 1) sopra.push(a.nome + ' su ' + b.nome + ' della barra');
    }
  }
  const alfa = (c) => { const m = /rgba?\(([^)]+)\)/.exec(c || ''); if (!m) return 0; const p = m[1].split(/[ ,\/]+/).filter(Boolean); return p.length > 3 ? parseFloat(p[3]) : 1; };
  return {
    sopra,
    pannello: pannello ? pannello.dataset.maPannello : '',
    sfondo_pannello: pannello ? getComputedStyle(pannello).backgroundColor : '',
    alfa_pannello: pannello ? alfa(getComputedStyle(pannello).backgroundColor) : 0,
    alfa_smistare: alfa(getComputedStyle(r.querySelector('#ma-da-smistare') || r).backgroundColor),
    alfa_documenti: alfa(getComputedStyle(r.querySelector('#ma-documenti') || r).backgroundColor),
    di_lato: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    blocchi,
  };
}"""

# la finestra di «Ispeziona» aperta: modale (top layer), dentro la finestra del browser, sfondo pieno, sopra tutto
JS_FINESTRA = r"""() => {
  const d = [...document.querySelectorAll('dialog.ma-dialogo--cassetto')].find((x) => x.open);
  if (!d) return null;
  const b = d.getBoundingClientRect();
  const cx = Math.round(b.left + b.width / 2), cy = Math.round(b.top + Math.min(b.height / 2, 60));
  const e = document.elementFromPoint(cx, cy);
  const sfondo = getComputedStyle(d).backgroundColor;
  const m = /rgba?\(([^)]+)\)/.exec(sfondo || '');
  const p = m ? m[1].split(/[ ,\/]+/).filter(Boolean) : [];
  return {id: d.id, modale: d.matches(':modal'), dentro: b.left >= 0 && b.top >= 0 && b.right <= innerWidth + 0.5 && b.bottom <= innerHeight + 0.5,
          sopra: !!(e && d.contains(e)), alfa: m ? (p.length > 3 ? parseFloat(p[3]) : 1) : 0, sfondo};
}"""

CASELLE = "#ma-schede-albero [role=tabpanel]:not([hidden]) .ma-dtree .ma-nodo"
PANNELLO = "#ma-anteprima .ma-anteprima:not([hidden])"


def misura_impaginazione(b, contesto, rotture):
    m = b.page.evaluate(JS_IMPAGINAZIONE)
    if m.get("errore"):
        rotture.append("%s: %s" % (contesto, m["errore"]))
        return m
    for s in m["sopra"]:
        rotture.append("%s: %s" % (contesto, s))
    if m["pannello"] and m["alfa_pannello"] < 1:
        rotture.append("%s: il pannello del pezzo scelto ha uno sfondo trasparente (%s)" % (contesto, m["sfondo_pannello"]))
    if m["alfa_smistare"] < 1 or m["alfa_documenti"] < 1:
        rotture.append("%s: «Da smistare» o il riepilogo documentale senza sfondo pieno" % contesto)
    if m["di_lato"] > 1:
        rotture.append("%s: la pagina scorre di lato di %d px" % (contesto, m["di_lato"]))
    return m


def scegli_una_casella(b):
    """Una casella dell'albero scelta (il pannello dell'anteprima a vista); False se l'albero non ha caselle."""
    caselle = b.page.locator(CASELLE)
    if caselle.count() == 0:
        return False
    caselle.first.click()
    b.aspetta("() => !!document.querySelector(%s)" % json.dumps(PANNELLO), "scelta la casella, il pannello dell'anteprima non compare")
    return True


def impaginazione_a(b, larghezza, altezza, prodotto, rotture, tema=""):
    nome = "%dx%d%s" % (larghezza, altezza, ("_" + tema) if tema else "")
    b.page.set_viewport_size({"width": larghezza, "height": altezza})
    b.page.goto(b.pagina + "?prodotto=" + prodotto + "&vista=distinta")
    b.page.wait_for_selector("#ma > .ma")
    verifica(scegli_una_casella(b), "%s: il prodotto %s non ha caselle nell'albero" % (nome, prodotto))
    misura_impaginazione(b, nome + " distinta, casella scelta", rotture)
    b.foto("i7_%s_distinta.png" % nome)
    b.page.locator("#ma-viste a:has-text('Documenti')").click()
    b.aspetta_scelta(prodotto, "documenti", "%s: «Documenti» non porta alla vista documenti: %s" % (nome, b.page.url))
    b.page.wait_for_timeout(300)
    # la casella scelta resta (sessionStorage); se non c'e' piu', la si sceglie di nuovo
    if not b.page.locator(PANNELLO).count():
        scegli_una_casella(b)
    b.page.evaluate("() => document.getElementById('ma-documenti').scrollIntoView({block: 'start'})")
    b.page.wait_for_timeout(200)
    m = misura_impaginazione(b, nome + " documenti, in cima al riepilogo", rotture)
    verifica(m.get("pannello"), "%s: nella vista Documenti nessun pannello dell'anteprima scelto" % nome)
    b.foto("i7_%s_documenti.png" % nome)
    b.page.evaluate("() => { const t = document.querySelector('#ma-documenti .ma-voci') || document.querySelector('#ma-documenti .ma-toolbar--p4'); if (t) t.scrollIntoView({block: 'center'}); }")
    b.page.wait_for_timeout(200)
    misura_impaginazione(b, nome + " documenti, tabella al centro", rotture)
    b.foto("i7_%s_tabella.png" % nome)
    b.page.evaluate("() => window.scrollTo(0, document.documentElement.scrollHeight)")
    b.page.wait_for_timeout(200)
    misura_impaginazione(b, nome + " documenti, in fondo alla pagina", rotture)
    b.foto("i7_%s_fondo.png" % nome)
    # «Ispeziona» della prima voce: la finestra modale sopra tutto, poi Esc
    isp = b.page.locator("#ma-documenti button[data-ma-azione=ispeziona]")
    verifica(isp.count() >= 1, "%s: nessun «Ispeziona» nel riepilogo documentale" % nome)
    isp.first.scroll_into_view_if_needed()
    try:
        isp.first.click(timeout=5000)
    except Exception as e:
        # un blocco che copre il pulsante: Playwright dice chi intercetta il clic
        chi = [r.strip() for r in str(e).splitlines() if "intercepts pointer events" in r]
        rotture.append("%s: «Ispeziona» non si clicca, %s" % (nome, chi[0] if chi else str(e).splitlines()[0]))
        b.foto("i7_%s_ispeziona.png" % nome)
        return
    b.aspetta("() => [...document.querySelectorAll('dialog.ma-dialogo--cassetto')].some((x) => x.open)", "%s: «Ispeziona» non apre la finestra" % nome)
    f = b.page.evaluate(JS_FINESTRA)
    b.foto("i7_%s_ispeziona.png" % nome)
    if not (f and f["modale"] and f["dentro"] and f["sopra"] and f["alfa"] >= 1):
        rotture.append("%s: la finestra di «Ispeziona» non sta sopra tutto, dentro la finestra, con sfondo pieno: %s" % (nome, f))
    b.page.keyboard.press("Escape")
    b.aspetta("() => ![...document.querySelectorAll('dialog.ma-dialogo--cassetto')].some((x) => x.open)", "%s: Esc non chiude la finestra di «Ispeziona»" % nome)


@prova("7", "impaginazione con l'anteprima aperta, quattro misure e tema scuro: nessun blocco sopra un altro")
def prova_7(b):
    prima = b.sonda()
    # il prodotto con l'albero: il primo dei due che ne ha uno
    prodotto = ""
    for rif in (b.a.primo, b.a.secondo):
        b.page.goto(b.pagina + "?prodotto=" + rif + "&vista=distinta")
        b.page.wait_for_selector("#ma > .ma")
        if b.page.locator(CASELLE).count():
            prodotto = rif
            break
    verifica(prodotto, "nessuno dei due prodotti ha caselle nell'albero")
    rotture = []
    try:
        for w, h in MISURE:
            impaginazione_a(b, w, h, prodotto, rotture)
        b.page.emulate_media(color_scheme="dark")
        impaginazione_a(b, 1440, 900, prodotto, rotture, tema="scuro")
    finally:
        b.page.emulate_media(color_scheme="light")
        b.page.set_viewport_size({"width": 1440, "height": 900})
    verifica(b.sonda() == prima, "la prova dell'impaginazione ha cambiato la sonda")
    verifica(not rotture, "%d difetti di impaginazione:\n      %s" % (len(rotture), "\n      ".join(rotture)))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--sonda", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="AD")
    ap.add_argument("--password", default="prova-ad")
    ap.add_argument("--thread", required=True)
    ap.add_argument("--primo", required=True)
    ap.add_argument("--secondo", required=True)
    ap.add_argument("--diagnostiche", type=int, required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="123456")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    falliti = []
    fatte = 0
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
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
        # le domande dei gesti (confirm del ponte): si annotano e si accettano
        page.on("dialog", lambda d: (b.dialoghi.append(d.message), d.accept()))
        for lettera, nome, fn in PROVE:
            if lettera not in a.prove:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  %s  %-72s PASSATO   (%.1fs)" % (lettera, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(lettera)
                print("  %s  %-72s FALLITO   %s" % (lettera, nome, e))
                b.foto("errore_%s.png" % lettera)
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
