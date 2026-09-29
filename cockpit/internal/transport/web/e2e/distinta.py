# -*- coding: utf-8 -*-
"""L7 - la Distinta in un BROWSER VERO, quando si confermano le cose (29/09).

Non lo si lancia a mano: lo avvia `distinta_browser_test.go` (tag `browser`), con la scena di quel file (un
prodotto, quattro assiemi con i figli in comune, trenta file) e la «sonda»: un indirizzo che restituisce la
fotografia delle righe della RFQ (componenti, archi, documenti, proposte, copie sul NAS, versioni, job).

Dopo OGNI gesto lo script controlla tre cose:
  - la pagina: niente errori in console, un solo corpo (#distinta) con un solo pannello, niente id doppi,
    ogni file in una riga sola, i contatori (chip, linguette, «Conferma i N file pronti») uguali alle righe,
    niente testi del programma a vista («ZgotmplZ», «undefined», «NaN», «[object Object]», «%!»);
  - il database: la differenza fra la fotografia di prima e quella di dopo e' quella del gesto, niente di piu';
  - la ricarica: ricaricando la pagina si vede lo stesso stato.

Le prove (lettere, scelte dal test Go con --prove). Maiuscole: comportamenti che devono esserci e ci sono.
Minuscole: difetti trovati il 29/09 (docs/specs/BUG_DISTINTA_29-09.md), che falliscono finche' ci sono; con
--noti si eseguono e si scrive l'esito, senza far fallire la corsa.
  Passo 2, Distinta:
  A  «Accetta la struttura proposta» non scrive; «Salva la distinta» porta i quattro assiemi e chiude le proposte
  B  trascinare sotto un altro assieme e salvare (doppio clic: un salvataggio); il rilascio su un particolare si rifiuta
  D  «Avanti» con modifiche non salvate chiede; dopo il salvataggio Indietro/Avanti mostrano lo stato salvato
  E  due schede: il salvataggio della scheda vecchia si rifiuta e non scrive
  Passo 3, Documenti e NAS:
  F  «✓ Conferma» su un file          G  doppio clic su «✓ Conferma»      H  conferma di un figlio in comune
  I  «Sposta» e poi conferma          J  «Documento della richiesta»      K  «Metti da parte»
  P  risposta a «che cos'e'?» e conferma                                 L  «Conferma e copia sul NAS»
  N  «Congela»                        O  Indietro/Avanti fra i passi dopo un gesto
  M  due schede: i gesti della scheda vecchia si rifiutano, niente doppioni
  Difetti (BUG_DISTINTA_29-09.md):
  a  «Crea questi pezzi nella distinta» mette tutti i pezzi sotto il prodotto            (Bug 1)
  c  «N proposte da decidere» senza nessun gesto per deciderle                           (Bug 2)
  l  il PDF d'assieme letto con il codice di un figlio e' pronto come 2D del figlio      (Bug 3)
  s  dopo «✓ Conferma» la pagina salta di migliaia di pixel                             (Bug 4)
  o  il tasto Indietro del browser mostra la pagina di prima del gesto                  (Bug 5)
  k  un file messo da parte non si riprende                                             (Bug 6)
  n  il rifiuto di «Metti da parte» e' mostrato come riuscito                           (Bug 7)
  p  «che cos'e' questo file?» parte da «3D» per un PDF                                 (Bug 8)
  j  «Documento della richiesta» senza conferma e senza ritorno                          (Bug 9)
  d  un pezzo tolto dalla distinta e' «il prodotto» nel passo 3                         (Bug 10)
  h  il blocco di un pezzo in comune nomina un padre solo                               (Bug 11)
  g  la guida elenca due volte lo stesso legame                                         (Bug 12)
  r  la linguetta «da verificare» conta un file in piu' delle righe                     (Bug 13)
  q  il segno «✓°» senza legenda vicino                                                  (Bug 14)
  Z  esplorazione: fotografa i passi (non verifica niente)
"""
import argparse
import json
import os
import re
import sys
import time
import urllib.request

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


# ------------------------------------------------------------------ il database, dalla sonda

def differenza(prima, dopo):
    """La differenza fra due fotografie: per gli elenchi le righe in piu' (+) e in meno (-), per le mappe le chiavi
    cambiate (valore di prima -> valore di dopo)."""
    out = {}
    for k in sorted(set(prima) | set(dopo)):
        a, b = prima.get(k), dopo.get(k)
        if isinstance(a, list) or isinstance(b, list):
            a, b = a or [], b or []
            piu = [x for x in b if x not in a]
            meno = [x for x in a if x not in b]
            if piu or meno:
                out[k] = {"+": piu, "-": meno}
        else:
            a, b = a or {}, b or {}
            cambi = {x: (a.get(x), b.get(x)) for x in sorted(set(a) | set(b)) if a.get(x) != b.get(x)}
            if cambi:
                out[k] = cambi
    return out


# ------------------------------------------------------------------ attese sul database

def solo(d, chiavi, contesto):
    extra = sorted(set(d) - set(chiavi))
    verifica(not extra, "%s: il gesto ha cambiato anche %s: %s" % (contesto, extra, json.dumps({k: d[k] for k in extra}, ensure_ascii=False)))


def entrati(nomi, tipo, comp, contesto, stato_nas="in_coda"):
    """L'attesa di una conferma: per ogni file un documento nuovo (tipo, componente, copia in coda), la sua proposta
    confermata, una copia sul NAS in coda e una provenienza. Niente altro."""
    def controlla(d):
        solo(d, ["documenti", "proposte", "copie", "provenienze", "job"], contesto)
        job = d.get("job", {})
        verifica(list(job) == ["copia_nas"] and int(job["copia_nas"][1] or 0) - int(job["copia_nas"][0] or 0) == len(nomi),
                 "%s: job cambiati %s, attese %d copie in piu'" % (contesto, job, len(nomi)))
        doc = d.get("documenti", {"+": [], "-": []})
        attesi = sorted("%s|%s|%s|%s" % (n, tipo if isinstance(tipo, str) else tipo[n], comp if isinstance(comp, str) else comp[n], stato_nas) for n in nomi)
        verifica(sorted(doc["+"]) == attesi and not doc["-"], "%s: documenti nuovi %s, attesi %s (tolti %s)" % (contesto, sorted(doc["+"]), attesi, doc["-"]))
        pr = d.get("proposte", {})
        verifica(sorted(pr) == sorted(nomi), "%s: proposte cambiate %s, attese %s" % (contesto, sorted(pr), sorted(nomi)))
        for n, (a, b) in pr.items():
            verifica(a and a.startswith("aperta|") and b and b.startswith("confermata|"), "%s: la proposta di %s: %s -> %s" % (contesto, n, a, b))
        verifica(d.get("copie", {}) == {n: (None, "1") for n in nomi}, "%s: copie sul NAS accodate %s, attesa una per file" % (contesto, d.get("copie")))
        verifica(sorted(d.get("provenienze", {})) == sorted(nomi), "%s: provenienze %s" % (contesto, d.get("provenienze")))
    return controlla


def niente(contesto):
    def controlla(d):
        verifica(not d, "%s: il database e' cambiato, e non doveva: %s" % (contesto, json.dumps(d, ensure_ascii=False)))
    return controlla


# ------------------------------------------------------------------ la pagina

# Una fotografia della pagina come la vede l'operatore, senza l'avviso del gesto: serve a dire che dopo una
# ricarica lo stato e' lo stesso.
JS_STATO = r"""() => {
  const t = (e) => (e ? e.textContent.replace(/\s+/g, ' ').trim() : '');
  const out = {passo: '', linguette: [], chips: [], blocchi: [], righe: [], sezioni: [], albero: [], analisi: '', nas: '', congela: ''};
  const pan = document.querySelector('.dst-pannello');
  out.passo = pan ? pan.dataset.passo : '';
  for (const l of document.querySelectorAll('.dst-passo')) out.linguette.push(t(l));
  for (const c of document.querySelectorAll('.dst-pannello > .dst-chips .dst-chip')) out.chips.push(t(c));
  for (const b of document.querySelectorAll('.dst-blocco')) {
    const testa = t(b.querySelector('.dst-blocco-testa'));
    const slot = [...b.querySelectorAll('.dst-slot')].map(t);
    const file = [...b.querySelectorAll('tr.dst-file')].map((r) => t(r.querySelector('td.nome > span.mono')) + ' :: ' + t(r.querySelector('td.stato')));
    out.blocchi.push({testa, slot, file});
  }
  for (const r of document.querySelectorAll('tr.dst-file')) {
    const dove = r.closest('.dst-blocco') ? 'blocco:' + t(r.closest('.dst-blocco').querySelector('.cod')) : (r.closest('.dst-box') ? t(r.closest('.dst-box').querySelector('.dst-label')) : '?');
    out.righe.push({nome: t(r.querySelector('td.nome > span.mono')), stato: t(r.querySelector('td.stato')), classe: r.className, dove});
  }
  for (const s of document.querySelectorAll('details.dst-box > summary.dst-label, .dst-box.tono-warn > .dst-label')) out.sezioni.push(t(s));
  for (const n of document.querySelectorAll('#dst-tree .dst-nodo')) {
    let liv = 0, x = n.closest('li');
    while (x && x.parentElement && x.parentElement.closest('li')) { liv++; x = x.parentElement.closest('li'); }
    out.albero.push(liv + ':' + t(n.querySelector('.cod')) + ':' + t(n.querySelector('.dst-tipo')) + ':' + [...n.classList].filter((c) => ['proposto', 'nuovo', 'rimando'].includes(c)).join('+'));
  }
  out.analisi = t(document.getElementById('dst-analisi-chip'));
  out.nas = t(document.getElementById('dst-box-nas'));
  return out;
}"""

# I controlli della pagina che valgono sempre, dopo ogni gesto.
JS_SANA = r"""() => {
  const p = [];
  const conta = (s) => document.querySelectorAll(s).length;
  if (conta('#distinta') !== 1) p.push('#distinta: ' + conta('#distinta'));
  if (conta('#distinta #distinta')) p.push('un #distinta dentro #distinta');
  if (conta('.dst-pannello') !== 1) p.push('pannelli: ' + conta('.dst-pannello'));
  if (conta('#dst-dati') !== 1) p.push('#dst-dati: ' + conta('#dst-dati'));
  if (conta('.dst-navbar') !== 1) p.push('barre in fondo: ' + conta('.dst-navbar'));
  if (conta('.dst-passi-box') !== 1) p.push('linguette: ' + conta('.dst-passi-box'));
  if (conta('.dst-avviso') > 1) p.push('avvisi: ' + conta('.dst-avviso'));
  if (conta('.dst-cartiglio') !== 1) p.push('cartigli: ' + conta('.dst-cartiglio'));
  if (conta('#dst-visore') !== 1) p.push('visori: ' + conta('#dst-visore'));
  if (conta('body > main #distinta, #distinta') && document.querySelector('#distinta').closest('.dst-pannello')) p.push('#distinta dentro un pannello');
  // un frammento della pagina intera dentro il corpo: la testata o il layout
  if (conta('#distinta header, #distinta nav.topbar, #distinta html, #distinta body')) p.push('pezzi del layout dentro #distinta');
  const ids = {};
  for (const e of document.querySelectorAll('[id]')) ids[e.id] = (ids[e.id] || 0) + 1;
  const doppi = Object.entries(ids).filter(([, n]) => n > 1).map(([k, n]) => k + '×' + n);
  if (doppi.length) p.push('id doppi: ' + doppi.join(', '));
  const testo = document.body.innerText;
  for (const s of ['ZgotmplZ', 'undefined', 'NaN', '[object Object]', '%!', '<no value>', '{{', 'null ·', '· null']) {
    if (testo.includes(s)) p.push('testo strano a vista: «' + s + '»');
  }
  return p;
}"""

# I controlli del passo «Documenti e NAS»: ogni file in una riga sola, i numeri uguali alle righe.
JS_DOCUMENTI = r"""() => {
  const p = [];
  const t = (e) => (e ? e.textContent.replace(/\s+/g, ' ').trim() : '');
  const righe = [...document.querySelectorAll('tr.dst-file')];
  const nomi = {};
  for (const r of righe) { const n = t(r.querySelector('td.nome > span.mono')); nomi[n] = (nomi[n] || 0) + 1; }
  for (const li of document.querySelectorAll('details.dst-box li .mono')) { const n = t(li); nomi[n] = (nomi[n] || 0) + 1; }
  const doppi = Object.entries(nomi).filter(([, n]) => n > 1).map(([k, n]) => k + '×' + n);
  if (doppi.length) p.push('file in piu\' righe: ' + doppi.join(', '));
  const pronti = righe.filter((r) => r.classList.contains('pronto')).length;
  const chip = [...document.querySelectorAll('.dst-pannello > .dst-chips .dst-chip')].map(t);
  const numero = (re) => { for (const c of chip) { const m = c.match(re); if (m) return parseInt(m[1], 10); } return 0; };
  if (numero(/^(\d+) pronti da confermare/) !== pronti) p.push('chip «pronti» ' + numero(/^(\d+) pronti da confermare/) + ', righe pronte ' + pronti);
  const bottone = document.querySelector('#dst-box-nas button[type=submit]');
  const nb = bottone ? (t(bottone).match(/i (\d+) file pronti/) || [0, '0'])[1] : '0';
  if (parseInt(nb, 10) !== pronti) p.push('«Conferma i N file pronti» dice ' + nb + ', righe pronte ' + pronti);
  if (bottone && (pronti > 0) === bottone.disabled) p.push('il bottone della conferma e\' ' + (bottone.disabled ? 'spento' : 'acceso') + ' con ' + pronti + ' righe pronte');
  const sistemare = document.querySelectorAll('.dst-box.tono-warn tr.dst-file').length;
  if (numero(/^(\d+) da sistemare/) !== sistemare) p.push('chip «da sistemare» ' + numero(/^(\d+) da sistemare/) + ', righe ' + sistemare);
  const parte = document.querySelectorAll('details.dst-box').length ? [...document.querySelectorAll('details.dst-box')].filter((d) => t(d.querySelector('summary')).startsWith('Messi da parte')).map((d) => d.querySelectorAll('li').length)[0] || 0 : 0;
  if (numero(/^(\d+) messi da parte/) !== parte) p.push('chip «messi da parte» ' + numero(/^(\d+) messi da parte/) + ', righe ' + parte);
  // la linguetta del passo dice la stessa cosa delle righe
  const ling = t(document.querySelector('.dst-passo.on .s'));
  const mp = ling.match(/^(\d+) pront/);
  if (mp && parseInt(mp[1], 10) !== pronti) p.push('la linguetta dice «' + ling + '», righe pronte ' + pronti);
  // lo stato del NAS nelle caselle di un blocco e nelle righe confermate dello stesso blocco
  for (const b of document.querySelectorAll('.dst-blocco')) {
    const cod = t(b.querySelector('.cod'));
    const confermati = [...b.querySelectorAll('tr.dst-file.confermato')].length;
    const slot = b.querySelectorAll('.dst-slot').length;
    const slotOk = [...b.querySelectorAll('.dst-slot.ok, .dst-slot.coda')].length;
    if (confermati && slot && !slotOk) p.push(cod + ': ' + confermati + ' file confermati ma nessuna casella presente');
  }
  return p;
}"""


class Banco:
    def __init__(self, page, a):
        self.page, self.a = page, a
        self.base = a.url + "/thread/" + a.thread + "/distinta"
        self.pezzi = dict(x.split("=", 1) for x in a.pezzi.split(",") if x)
        self.errori = []
        self.noti = []

    # ---- il database
    def db(self):
        with urllib.request.urlopen(self.a.sonda) as r:
            return json.loads(r.read().decode("utf-8"))

    # ---- le righe dei file (passo 3)
    def riga(self, nome, pagina=None):
        p = pagina or self.page
        return p.locator("tr.dst-file").filter(has=p.locator("td.nome > span.mono", has_text=re.compile("^" + re.escape(nome) + "$")))

    def stato_riga(self, nome, pagina=None):
        r = self.riga(nome, pagina)
        verifica(r.count() == 1, "%s: righe %d" % (nome, r.count()))
        return " ".join(r.locator("td.stato").inner_text().split())

    def blocco_di(self, nome, pagina=None):
        r = self.riga(nome, pagina)
        verifica(r.count() == 1, "%s: righe %d" % (nome, r.count()))
        return r.evaluate("(e) => { const b = e.closest('.dst-blocco'); return b ? b.querySelector('.cod').textContent.trim() : (e.closest('.dst-box') ? e.closest('.dst-box').querySelector('.dst-label').textContent.trim() : '?'); }")

    def htmx(self, clic, parte, pagina=None):
        """Un clic che manda una richiesta htmx (o fetch) a una rotta che contiene parte: aspetta la risposta."""
        p = pagina or self.page
        with p.expect_response(lambda r: parte in r.url and r.request.method == "POST", timeout=20000) as risp:
            clic()
        return risp.value

    def conferma_riga(self, nome, pagina=None):
        p = pagina or self.page
        self.htmx(lambda: self.riga(nome, p).locator("button", has_text="✓ Conferma").click(), "/fascicolo/conferma", p)

    def sposta_riga(self, nome, verso, pagina=None):
        p = pagina or self.page
        r = self.riga(nome, p)
        sel = r.locator("select[name=componente]")
        valore = sel.evaluate("(s, c) => { const o = [...s.options].find((o) => o.textContent.trim().split(' ')[0] === c); return o ? o.value : ''; }", verso)
        verifica(valore, "%s: %s non e' fra i pezzi della tendina" % (nome, verso))
        sel.select_option(valore)
        self.htmx(lambda: r.locator("form[hx-post$='/assegna'] button[type=submit]").click(), "/fascicolo/assegna", p)

    # ---- la pagina
    def apri(self, passo="distinta", pagina=None):
        p = pagina or self.page
        p.goto(self.base + "?passo=" + passo)
        p.wait_for_load_state("domcontentloaded")
        self.pronta(p)
        p.evaluate("window.__marca = 'viva'")

    def pronta(self, pagina=None):
        """Aspetta che la pagina abbia finito: nessuna richiesta htmx in corso e, nel passo 2, lo schema disegnato."""
        p = pagina or self.page
        p.wait_for_function("() => !document.querySelector('.htmx-request')", timeout=15000)
        if p.evaluate("(document.querySelector('.dst-pannello') || {dataset: {}}).dataset.passo") == "distinta":
            p.wait_for_function("() => document.querySelector('#dst-tree .dst-nodo') || document.querySelector('#dst-tree .bad-t') || document.querySelector('#dst-tree .dst-box')", timeout=15000)
        p.wait_for_timeout(150)

    def viva(self, pagina=None):
        return (pagina or self.page).evaluate("window.__marca") == "viva"

    def avviso(self, pagina=None):
        el = (pagina or self.page).locator("#distinta .dst-avviso")
        return el.inner_text().strip() if el.count() else ""

    def toast(self, pagina=None):
        el = (pagina or self.page).locator("#dst-toast")
        return el.inner_text().strip() if el.count() and el.is_visible() else ""

    def stato(self, pagina=None):
        return (pagina or self.page).evaluate(JS_STATO)

    def foto(self, nome, pagina=None):
        if self.a.foto:
            os.makedirs(self.a.foto, exist_ok=True)
            (pagina or self.page).screenshot(path=os.path.join(self.a.foto, nome), full_page=True)

    def sana(self, contesto, pagina=None):
        p = pagina or self.page
        problemi = p.evaluate(JS_SANA)
        if p.evaluate("(document.querySelector('.dst-pannello') || {dataset: {}}).dataset.passo") == "documenti":
            problemi += p.evaluate(JS_DOCUMENTI)
        verifica(not problemi, "%s: la pagina non e' sana: %s" % (contesto, "; ".join(problemi)))

    def ricarica_uguale(self, contesto, pagina=None):
        p = pagina or self.page
        prima = self.stato(p)
        p.reload()
        p.wait_for_load_state("domcontentloaded")
        self.pronta(p)
        dopo = self.stato(p)
        if prima != dopo:
            diff = []
            for k in prima:
                if prima[k] != dopo[k]:
                    diff.append("%s: prima %s / dopo %s" % (k, json.dumps(prima[k], ensure_ascii=False)[:600], json.dumps(dopo[k], ensure_ascii=False)[:600]))
            raise Rotto("%s: ricaricando la pagina lo stato e' diverso: %s" % (contesto, " | ".join(diff)))
        self.sana(contesto + " (dopo la ricarica)", p)
        p.evaluate("window.__marca = 'viva'")

    def gesto(self, contesto, fai, atteso, pagina=None, ricarica=True):
        """Un gesto dell'operatore: fai() nel browser, poi la pagina sana, la differenza del database uguale ad
        atteso (un dict chiave -> differenza, o una funzione che la verifica), la ricarica uguale."""
        p = pagina or self.page
        prima = self.db()
        fai()
        self.pronta(p)
        p.wait_for_timeout(250)  # il flusso gira dopo la risposta (dopoIlGesto): si aspetta che finisca
        dopo = self.db()
        d = differenza(prima, dopo)
        self.sana(contesto, p)
        if callable(atteso):
            atteso(d)
        else:
            verifica(d == atteso, "%s: il database e' cambiato cosi':\n      %s\n    atteso:\n      %s" % (
                contesto, json.dumps(d, ensure_ascii=False, sort_keys=True), json.dumps(atteso, ensure_ascii=False, sort_keys=True)))
        if ricarica:
            self.ricarica_uguale(contesto, p)
        return d


# ------------------------------------------------------------------ Z: esplorazione

@prova("Z", "esplorazione: i passi fotografati")
def prova_z(b):
    cartella = b.a.foto or "."
    os.makedirs(cartella, exist_ok=True)
    with open(os.path.join(cartella, "sonda.json"), "w", encoding="utf-8") as f:
        json.dump(b.db(), f, ensure_ascii=False, indent=1)
    for passo in ["richiesta", "distinta", "documenti", "fattibilita"]:
        b.apri(passo)
        b.page.wait_for_timeout(1200)
        b.foto("z_%s.png" % passo)
        with open(os.path.join(cartella, "z_%s.html" % passo), "w", encoding="utf-8") as f:
            f.write(b.page.content())
        with open(os.path.join(cartella, "z_%s.json" % passo), "w", encoding="utf-8") as f:
            json.dump(b.stato(), f, ensure_ascii=False, indent=1)
        problemi = b.page.evaluate(JS_SANA)
        if passo == "documenti":
            problemi += b.page.evaluate(JS_DOCUMENTI)
        print("    %s: %s" % (passo, problemi))
    if b.page.locator("text=Crea questi pezzi nella distinta").count():
        pass


# ------------------------------------------------------------------ il passo 3: Documenti e NAS (distinta fatta)

@prova("F", "«✓ Conferma» su un file: un documento, una copia in coda, la riga confermata, la ricarica uguale")
def prova_f(b):
    b.apri("documenti")
    b.sana("apertura")
    verifica(b.stato_riga("ACME-7121002.pdf") == "pronto da confermare", "prima: %s" % b.stato_riga("ACME-7121002.pdf"))
    b.gesto("conferma di ACME-7121002.pdf", lambda: b.conferma_riga("ACME-7121002.pdf"),
            entrati(["ACME-7121002.pdf"], "disegno_2d", "7121002", "conferma di ACME-7121002.pdf"), ricarica=False)
    verifica(b.viva(), "la pagina si e' ricaricata")
    verifica(b.avviso().startswith("Fascicolo confermato"), "avviso: %r" % b.avviso())
    verifica(b.stato_riga("ACME-7121002.pdf").startswith("✓ confermato"), "dopo: %s" % b.stato_riga("ACME-7121002.pdf"))
    verifica(b.blocco_di("ACME-7121002.pdf") == "7121002", "la riga e' finita in %s" % b.blocco_di("ACME-7121002.pdf"))
    b.ricarica_uguale("conferma di ACME-7121002.pdf")
    b.gesto("conferma di ACME-7121002 00 IN_WORK.stp", lambda: b.conferma_riga("ACME-7121002 00 IN_WORK.stp"),
            entrati(["ACME-7121002 00 IN_WORK.stp"], "cad_3d", "7121002", "conferma dello STEP"))


@prova("G", "doppio clic su «✓ Conferma»: un documento solo, una copia sola")
def prova_g(b):
    b.apri("documenti")
    nome = "ACME-7121004.pdf"

    def doppio():
        with b.page.expect_response(lambda r: "/fascicolo/conferma" in r.url, timeout=20000):
            b.riga(nome).locator("button", has_text="✓ Conferma").dblclick()
        b.page.wait_for_timeout(1500)
    b.gesto("doppio clic su " + nome, doppio, entrati([nome], "disegno_2d", "7121004", "doppio clic"))


@prova("H", "conferma dello STEP di un figlio in comune a tre assiemi: un documento, un blocco solo")
def prova_h(b):
    b.apri("documenti")
    nome = "ACME-7121003 00 IN_WORK.stp"
    verifica(b.page.locator(".dst-blocco .cod", has_text=re.compile("^7121003$")).count() == 1, "7121003 ha piu' di un blocco")
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "cad_3d", "7121003", "conferma del figlio in comune"))
    verifica(b.page.locator(".dst-blocco .cod", has_text=re.compile("^7121003$")).count() == 1, "dopo: 7121003 ha piu' di un blocco")
    b.apri("distinta")
    albero = b.stato()["albero"]
    pieni = [x for x in albero if ":7121003:" in x and "rimando" not in x]
    verifica(len(pieni) == 1, "caselle piene di 7121003 nello schema: %s" % pieni)


@prova("I", "«Sposta» su un altro pezzo: il PDF dell'assieme letto con il codice del figlio torna all'assieme, poi si conferma")
def prova_i(b):
    b.apri("documenti")
    nome = "ACME-7120011.pdf"
    verifica(b.blocco_di(nome) == "7121003", "il file parte da %s" % b.blocco_di(nome))

    def attesa(d):
        solo(d, ["proposte"], "sposta")
        verifica(list(d["proposte"]) == [nome], "proposte cambiate: %s" % d["proposte"])
        verifica(d["proposte"][nome][1].startswith("aperta|7120011|7120011|"), "la proposta dopo lo spostamento: %s" % (d["proposte"][nome],))
    b.gesto("sposta " + nome + " su 7120011", lambda: b.sposta_riga(nome, "7120011"), attesa)
    verifica(b.blocco_di(nome) == "7120011", "dopo lo spostamento il file e' in %s" % b.blocco_di(nome))
    verifica(b.stato_riga(nome) == "pronto da confermare", "dopo lo spostamento: %s" % b.stato_riga(nome))
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7120011", "conferma dopo lo spostamento"))


@prova("J", "«Documento della richiesta»: un documento senza pezzo, la sua copia, la riga fra i documenti della richiesta")
def prova_j(b):
    b.apri("documenti")
    nome = "Capitolato fornitura ACME.pdf"
    b.gesto("documento della richiesta", lambda: b.htmx(lambda: b.riga(nome).locator("button", has_text="Documento della richiesta").click(), "/generale"),
            entrati([nome], "altro", "-", "documento della richiesta"))
    verifica(b.blocco_di(nome).startswith("Documenti della richiesta"), "la riga e' in %s" % b.blocco_di(nome))


@prova("K", "«Metti da parte»: la proposta scartata e basta, il file fra i messi da parte")
def prova_k(b):
    b.apri("documenti")
    nome = "ACME-7120014 00 IN_WORK.stp"  # il quinto STEP: un assieme che nella distinta non c'e'

    def attesa(d):
        solo(d, ["proposte"], "metti da parte")
        verifica(list(d["proposte"]) == [nome] and d["proposte"][nome][1].startswith("scartata|"), "proposte: %s" % d["proposte"])
    b.gesto("metti da parte " + nome, lambda: b.htmx(lambda: b.riga(nome).locator("button", has_text="Metti da parte").click(), "/scarta"), attesa)
    verifica(b.riga(nome).count() == 0, "il file messo da parte e' ancora una riga di file")
    verifica(b.page.locator("details.dst-box li .mono", has_text=nome).count() == 1, "il file non e' fra i messi da parte")


@prova("P", "«Salva» la risposta a «che cos'e' questo file?»: il PDF letto dal nome diventa il 2D del suo assieme, poi si conferma")
def prova_p(b):
    b.apri("documenti")
    nome = "ACME-7120012.pdf"
    r = b.riga(nome)
    verifica(b.blocco_di(nome).startswith("Da sistemare"), "il file parte da %s" % b.blocco_di(nome))
    form = r.locator("form[hx-post$='/decidi']")
    form.locator("select[name=tipo]").select_option("disegno_2d")
    form.locator("input[name=codice]").fill("7120012")

    def attesa(d):
        solo(d, ["proposte"], "decidi")
        verifica(d["proposte"][nome][1] == "aperta|-|7120012|disegno_2d", "la proposta dopo la risposta: %s" % (d["proposte"][nome],))
    b.gesto("risposta per " + nome, lambda: b.htmx(lambda: form.locator("button[type=submit]").click(), "/decidi"), attesa)
    verifica(b.blocco_di(nome) == "7120012", "dopo la risposta il file e' in %s" % b.blocco_di(nome))
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7120012", "conferma dopo la risposta"))


@prova("p", "Bug 8: «che cos'e' questo file?» per un PDF parte con «3D» gia' scelto")
def prova_p_minuscola(b):
    b.apri("documenti")
    scelto = b.riga("ACME-7120012.pdf").locator("form[hx-post$='/decidi'] select[name=tipo]").evaluate("(s) => s.options[s.selectedIndex].value")
    verifica(scelto != "cad_3d", "per ACME-7120012.pdf (un PDF) la tendina «che cos'è» parte da %s: un «Salva» distratto lo registra come CAD 3D con il codice ACME-7120012" % scelto)


@prova("L", "«Conferma e copia sul NAS»: entrano tutti e soli i file pronti, una copia ciascuno")
def prova_l(b):
    b.apri("documenti")
    pronti = b.page.evaluate("() => [...document.querySelectorAll('tr.dst-file.pronto td.nome > span.mono')].map((e) => e.textContent.trim())")
    verifica(len(pronti) > 5, "file pronti: %s" % pronti)

    def attesa(d):
        solo(d, ["documenti", "proposte", "copie", "provenienze", "job"], "conferma cumulativa")
        verifica(int(d["job"]["copia_nas"][1]) - int(d["job"]["copia_nas"][0] or 0) == len(pronti), "job: %s, pronti a vista %d: %s; documenti nuovi %d: %s" % (d["job"], len(pronti), sorted(pronti), len(d["documenti"]["+"]), sorted(d["documenti"]["+"])))
        nuovi = sorted(x.split("|")[0] for x in d["documenti"]["+"])
        verifica(nuovi == sorted(pronti) and not d["documenti"]["-"], "entrati %s, pronti a vista %s" % (nuovi, sorted(pronti)))
        verifica(sorted(d["copie"]) == sorted(pronti), "copie %s" % sorted(d["copie"]))
        for x in d["documenti"]["+"]:
            verifica(x.endswith("|in_coda"), "documento %s" % x)
    b.gesto("conferma cumulativa", lambda: b.htmx(lambda: b.page.locator("#dst-box-nas button[type=submit]").click(), "/fascicolo/conferma"), attesa)
    verifica(b.page.locator("tr.dst-file.pronto").count() == 0, "dopo la conferma restano righe pronte")
    verifica(b.page.locator("#dst-box-nas button[type=submit]").is_disabled(), "il bottone della conferma e' ancora acceso")


def seconda_scheda(b, passo):
    p2 = b.ctx.new_page()
    b.sorveglia(p2, "scheda 2")
    b.apri(passo, p2)
    return p2


def rifiutato(b, pagina, contesto):
    """Un gesto rifiutato: l'avviso dice che non e' cambiato niente, ed e' segnato come esito negativo."""
    av = (pagina or b.page).locator("#distinta .dst-avviso")
    verifica(av.count() == 1, "%s: nessun avviso" % contesto)
    testo = " ".join(av.inner_text().split())
    verifica(av.get_attribute("data-esito") == "no", "%s: l'avviso di un gesto rifiutato e' mostrato come riuscito: «%s»" % (contesto, testo))
    return testo


@prova("M", "due schede sulla stessa RFQ: i gesti della scheda vecchia si rifiutano, niente doppioni")
def prova_m(b):
    b.apri("documenti")
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121006.pdf"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121006", "scheda 1"))
    # la scheda 2 non sa niente: il suo «✓ Conferma» sullo stesso file
    b.gesto("scheda 2: conferma dello stesso file", lambda: b.conferma_riga(nome, p2), niente("scheda 2: conferma dello stesso file"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: conferma dello stesso file")
    verifica(b.stato_riga(nome, p2).startswith("✓ confermato"), "scheda 2 dopo il rifiuto: %s" % b.stato_riga(nome, p2))
    p2.close()
    # la conferma cumulativa con la firma vecchia
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121006 00 IN_WORK.stp"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "cad_3d", "7121006", "scheda 1"))
    b.gesto("scheda 2: conferma cumulativa con il piano vecchio",
            lambda: b.htmx(lambda: p2.locator("#dst-box-nas button[type=submit]").click(), "/fascicolo/conferma", p2),
            niente("scheda 2: conferma cumulativa"), pagina=p2, ricarica=False)
    t = rifiutato(b, p2, "scheda 2: conferma cumulativa")
    verifica("piano è cambiato" in t, "scheda 2: %s" % t)
    p2.close()
    # «Documento della richiesta» e «Sposta» su un file che la scheda 1 ha appena confermato
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121008.pdf"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121008", "scheda 1"))
    b.gesto("scheda 2: documento della richiesta su un file confermato",
            lambda: b.htmx(lambda: b.riga(nome, p2).locator("button", has_text="Documento della richiesta").click(), "/generale", p2),
            niente("scheda 2: documento della richiesta"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: documento della richiesta")
    p2.close()
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121008 00 IN_WORK.stp"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "cad_3d", "7121008", "scheda 1"))
    b.gesto("scheda 2: sposta un file confermato", lambda: b.sposta_riga(nome, "7121001", p2), niente("scheda 2: sposta"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: sposta")
    p2.close()


@prova("n", "Bug 7: «Metti da parte» su un file gia' deciso in un'altra scheda: il rifiuto e' mostrato come riuscito")
def prova_n_minuscola(b):
    b.apri("documenti")
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121001.pdf"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121001", "scheda 1"))
    b.gesto("scheda 2: metti da parte un file confermato",
            lambda: b.htmx(lambda: b.riga(nome, p2).locator("button", has_text="Metti da parte").click(), "/scarta", p2),
            niente("scheda 2: metti da parte"), pagina=p2, ricarica=False)
    try:
        rifiutato(b, p2, "scheda 2: metti da parte")
    finally:
        p2.close()


@prova("N", "«Congela» dopo la conferma di tutto: una versione congelata, la pagina lo dice, i gesti sulla struttura spariscono")
def prova_n(b):
    b.apri("documenti")
    box = b.page.locator(".dst-box", has=b.page.locator(".dst-label", has_text="Congela la distinta"))
    bottone = box.locator("button[type=submit]")
    if bottone.is_disabled():
        raise Rotto("non si congela: %s" % " ".join(box.inner_text().split()))

    def attesa(d):
        solo(d, ["versioni", "job"], "congela")
        verifica(d["versioni"]["+"] == ["1|congelata"] and not d["versioni"]["-"], "versioni: %s" % d["versioni"])
    box.locator("input[name=motivo]").fill("prima baseline di prova")
    b.gesto("congela", lambda: b.htmx(lambda: bottone.click(), "/fascicolo/congela"), attesa)
    verifica("congelata nella V1" in b.page.locator(".dst-passo", has_text="Distinta").inner_text(), "la linguetta della Distinta non dice congelata")
    b.apri("distinta")
    verifica(b.page.locator("[data-nuovo]").count() == 0, "con la distinta congelata ci sono ancora i bottoni «+ Assieme»")
    verifica(b.page.locator("[data-azione=salva]").count() == 0 or not b.page.locator("#dst-salva").is_visible(), "con la distinta congelata si salva ancora")


@prova("O", "«Indietro/Avanti» e il tasto Indietro del browser dopo un gesto: lo stato e' quello di adesso")
def prova_o(b):
    b.apri("documenti")
    nome = "ACME-7121005.dxf"
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "sviluppo_dxf", "7121005", "conferma del DXF"), ricarica=False)
    dopo = b.stato()
    # i bottoni in fondo: indietro alla Distinta e di nuovo avanti
    b.page.locator(".dst-navbar a", has_text="Distinta").click()
    b.page.wait_for_url("**passo=distinta*")
    b.pronta()
    b.sana("indietro alla Distinta")
    b.page.locator(".dst-navbar a", has_text="Avanti: Documenti").click()
    b.page.wait_for_url("**passo=documenti*")
    b.pronta()
    b.sana("avanti ai Documenti")
    verifica(b.stato_riga(nome).startswith("✓ confermato"), "con «Avanti» il DXF e': %s" % b.stato_riga(nome))
    verifica(b.avviso() == "", "l'avviso del gesto e' rimasto dopo la navigazione: %s" % b.avviso())
    uguale = b.stato()
    verifica(uguale["righe"] == dopo["righe"], "dopo Indietro/Avanti le righe sono diverse")


@prova("o", "Bug 5: il tasto Indietro del browser dopo un gesto mostra la pagina di prima del gesto")
def prova_o_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121001 00 IN_WORK.stp"
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "cad_3d", "7121001", "conferma"), ricarica=False)
    b.page.locator(".dst-navbar a", has_text="Avanti: Fattibilit").click()
    b.page.wait_for_url("**passo=fattibilita*")
    b.pronta()
    b.page.go_back()
    b.page.wait_for_load_state("domcontentloaded")
    b.pronta()
    stato = b.stato_riga(nome)
    verifica(stato.startswith("✓ confermato"), "tornando indietro con il browser %s risulta «%s»: la pagina e' quella di prima del gesto" % (nome, stato))


@prova("k", "Bug 6: un file messo da parte non si riprende (nessun gesto per tornare indietro)")
def prova_k_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121008.pdf"

    def attesa(d):
        solo(d, ["proposte"], "metti da parte")
    b.gesto("metti da parte " + nome, lambda: b.htmx(lambda: b.riga(nome).locator("button", has_text="Metti da parte").click(), "/scarta"), attesa)
    li = b.page.locator("details.dst-box li", has=b.page.locator(".mono", has_text=nome))
    verifica(li.count() == 1, "il file non e' fra i messi da parte")
    verifica(li.locator("button, a").count() > 0, "fra i «Messi da parte» %s non ha nessun gesto per riprenderlo: il file e' perso per questa RFQ" % nome)


@prova("h", "Bug 11: il blocco di un pezzo in comune dice un padre solo (e la quantita' di quell'arco)")
def prova_h_minuscola(b):
    b.apri("documenti")
    testa = " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121003$"))).locator(".dst-blocco-testa").inner_text().split())
    mancano = [p for p in ["7120010", "7120011", "7120012"] if p not in testa]
    verifica(not mancano, "il blocco di 7121003 (sotto tre assiemi, 5 pezzi in tutto) dice «%s»: non nomina %s" % (testa, mancano))


@prova("q", "Bug 14: il segno «✓°» nella casella di un pezzo, senza legenda vicino")
def prova_q_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121002.pdf"
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121002", "conferma"), ricarica=False)
    slot = " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121002$"))).locator(".dst-slot", has_text="2D").inner_text().split())
    verifica("✓°" not in slot, "la casella del 2D di 7121002 dice «%s»: il segno «✓°» si spiega solo nella legenda della matrice in fondo" % slot)


@prova("l", "Bug 3: il PDF d'assieme letto con il codice di un figlio entra come foglio del figlio con la conferma cumulativa")
def prova_l_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7120011.pdf"
    stato = b.stato_riga(nome)
    blocco = b.blocco_di(nome)
    verifica(not (stato == "pronto da confermare" and blocco == "7121003"),
             "%s (il nome dice 7120011, un assieme della distinta) e' «%s» sotto %s: «Conferma e copia sul NAS» lo scrive come 2D del figlio, accanto al suo disegno" % (nome, stato, blocco))



# ------------------------------------------------------------------ il passo 2: Distinta

def salva(b, pagina=None):
    p = pagina or b.page
    b.htmx(lambda: p.locator("[data-azione=salva]").click(), "/bom/applica", p)


def albero(b, pagina=None):
    return b.stato(pagina)["albero"]


@prova("A", "«Accetta la struttura proposta»: niente si scrive finche' non si salva; «Salva la distinta» porta i quattro assiemi")
def prova_a(b):
    b.apri("distinta")
    verifica(b.page.locator("[data-azione=accetta-tutto]").is_visible(), "il bottone «Accetta la struttura proposta» non si vede")
    b.gesto("accetta la struttura proposta", lambda: b.page.locator("[data-azione=accetta-tutto]").click(), niente("accetta"), ricarica=False)
    verifica(b.viva(), "la pagina si e' ricaricata")
    a = albero(b)
    verifica(sum(1 for x in a if x.startswith("1:71200")) == 4, "dopo «Accetta» lo schema: %s" % a)
    verifica(b.page.locator("#dst-salva").is_visible(), "la barra «Salva la distinta» non si vede")

    def attesa(d):
        solo(d, ["componenti", "relazioni", "nodi", "archi"], "salva")
        verifica(sorted(x.split("|")[0] for x in d["componenti"]["+"]) == ["7120010", "7120011", "7120012", "7120013"] and not d["componenti"]["-"],
                 "componenti nuovi: %s" % d["componenti"])
        verifica(sorted(d["relazioni"]["+"]) == ["7120001>7120010x1", "7120001>7120011x2", "7120001>7120012x1", "7120001>7120013x1"] and not d["relazioni"]["-"],
                 "archi nuovi: %s" % d["relazioni"])
    b.gesto("salva la distinta", lambda: salva(b), attesa)
    verifica(not b.page.locator("#dst-salva").is_visible(), "dopo il salvataggio la barra «Salva» si vede ancora")
    verifica(not b.page.locator("[data-azione=accetta-tutto]").is_visible(), "dopo il salvataggio «Accetta la struttura proposta» si vede ancora")
    ling = " ".join(b.page.locator(".dst-passo", has_text="Distinta").inner_text().split())
    verifica("proposte da decidere" not in ling, "dopo il salvataggio la linguetta dice ancora: %s" % ling)


@prova("a", "Bug 1: «Crea questi pezzi nella distinta» dalla guida mette tutti i pezzi sotto il prodotto, non sotto i loro assiemi")
def prova_a_minuscola(b):
    b.apri("distinta")
    if b.page.locator("[data-azione=accetta-tutto]").is_visible():
        b.page.locator("[data-azione=accetta-tutto]").click()
        salva(b)
        b.pronta()
    b.page.locator("button", has_text="Crea questi pezzi nella distinta").click()
    b.page.wait_for_selector("text=Sì, mettili nella distinta", timeout=15000)
    bozza = " ".join(b.page.locator("#dst-analisi .dst-conferma-box").inner_text().split())
    b.page.locator("button", has_text="Sì, mettili nella distinta").click()
    b.page.wait_for_timeout(300)
    a = albero(b)
    sotto_prodotto = [x for x in a if x.startswith("1:7121")]
    b.foto("a_guida.png")
    verifica(not sotto_prodotto, "dopo «Crea questi pezzi» (%s) lo schema ha sotto il prodotto %s; gli assiemi hanno %s" % (
        bozza[:300], sotto_prodotto, [x for x in a if x.startswith("2:")]))


def casella(b, codice, pagina=None, piena=True):
    p = pagina or b.page
    sel = "#dst-tree .dst-nodo:not(.rimando)" if piena else "#dst-tree .dst-nodo"
    return p.locator(sel).filter(has=p.locator(".cod", has_text=re.compile("^" + re.escape(codice) + "$"))).first


def trascina(b, da, a, pagina=None):
    """Un trascinamento con gli eventi HTML5 veri (dragstart, dragover, drop, dragend) mandati alle due caselle:
    lo schema e' piu' largo della finestra, e le due caselle non stanno sempre insieme sullo schermo."""
    p = pagina or b.page
    da.scroll_into_view_if_needed()
    h = da.element_handle()
    k = a.element_handle()
    p.evaluate("""([src, dst]) => {
      const dt = new DataTransfer();
      const ev = (tipo, el) => el.dispatchEvent(new DragEvent(tipo, {bubbles: true, cancelable: true, dataTransfer: dt}));
      ev('dragstart', src); ev('dragenter', dst); ev('dragover', dst); ev('drop', dst); ev('dragend', src);
    }""", [h, k])
    p.wait_for_timeout(250)


@prova("B", "trascinare un pezzo sotto un altro assieme e «Salva la distinta»: un arco tolto, uno messo; sotto un particolare si rifiuta")
def prova_b(b):
    b.apri("distinta")
    # sotto un particolare non si mette niente: il rilascio si rifiuta e lo schema resta com'e'
    prima = albero(b)
    trascina(b, casella(b, "7121004"), casella(b, "7121005"))
    verifica("sotto non ci va niente" in b.toast(), "il rilascio su un particolare: toast %r" % b.toast())
    verifica(albero(b) == prima, "lo schema e' cambiato dopo un rilascio rifiutato")
    verifica(not b.page.locator("#dst-salva").is_visible(), "dopo un rilascio rifiutato c'e' da salvare")
    # 7121002 da 7120010 a 7120012
    trascina(b, casella(b, "7121002"), casella(b, "7120012"))
    verifica("ora è sotto 7120012" in b.toast(), "il trascinamento: toast %r" % b.toast())
    verifica(b.page.locator("#dst-salva").is_visible(), "dopo il trascinamento la barra «Salva» non si vede")

    def attesa(d):
        solo(d, ["relazioni"], "salva dopo il trascinamento")
        verifica(d["relazioni"] == {"+": ["7120012>7121002x1"], "-": ["7120010>7121002x1"]}, "archi: %s" % d["relazioni"])
    # doppio clic su «Salva la distinta»: un salvataggio solo
    def doppio():
        with b.page.expect_response(lambda r: "/bom/applica" in r.url, timeout=20000):
            b.page.locator("[data-azione=salva]").dblclick()
        b.page.wait_for_timeout(1200)
    b.gesto("salva dopo il trascinamento (doppio clic)", doppio, attesa)
    verifica(not b.page.locator("#dst-salva").is_visible(), "dopo il salvataggio la barra «Salva» si vede ancora")


@prova("D", "«Avanti» con la distinta non salvata chiede prima di uscire; dopo il salvataggio Indietro/Avanti mostrano lo stato salvato")
def prova_d(b):
    b.apri("distinta")
    casella(b, "7121008").locator(".cod").click()  # un clic vero: il browser chiede prima di uscire solo dopo un gesto
    trascina(b, casella(b, "7121008"), casella(b, "7120010"))
    verifica(b.page.locator("#dst-salva").is_visible(), "dopo il trascinamento la barra «Salva» non si vede")
    del b.dialoghi[:]
    b.page.locator(".dst-navbar a", has_text="Avanti").click()
    b.page.wait_for_url("**passo=documenti*")
    b.pronta()
    verifica("beforeunload" in b.dialoghi, "uscendo con una modifica non salvata il browser non ha chiesto niente (dialoghi: %s)" % b.dialoghi)
    b.sana("documenti dopo l'uscita")
    # la modifica non salvata non c'e': la distinta e' quella del database
    b.page.locator(".dst-navbar a", has_text="Distinta").click()
    b.page.wait_for_url("**passo=distinta*")
    b.pronta()
    a = albero(b)
    verifica("2:7121008:Particolare:" in a and not any(x.startswith("2:7121008") and "rimando" in x for x in a), "lo schema dopo l'uscita: %s" % a)
    # adesso si salva, poi Avanti e Indietro
    trascina(b, casella(b, "7121008"), casella(b, "7120010"))

    def attesa(d):
        solo(d, ["relazioni"], "salva")
        verifica(d["relazioni"] == {"+": ["7120010>7121008x1"], "-": ["7120013>7121008x1"]}, "archi: %s" % d["relazioni"])
    b.gesto("salva 7121008 sotto 7120010", lambda: salva(b), attesa, ricarica=False)
    salvato = albero(b)
    b.page.locator(".dst-navbar a", has_text="Avanti").click()
    b.page.wait_for_url("**passo=documenti*")
    b.pronta()
    b.sana("avanti dopo il salvataggio")
    verifica("sotto 7120010" in " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121008$"))).locator(".dst-blocco-testa").inner_text().split()),
             "nel passo 3 7121008 non e' sotto 7120010")
    b.page.locator(".dst-navbar a", has_text="Distinta").click()
    b.page.wait_for_url("**passo=distinta*")
    b.pronta()
    verifica(albero(b) == salvato, "tornando alla Distinta lo schema e' diverso da quello salvato")


@prova("g", "Bug 12: la guida elenca due volte lo stesso legame (dallo STEP del prodotto e da quello dell'assieme)")
def prova_g_minuscola(b):
    b.apri("distinta")
    righe = b.page.evaluate("""() => [...document.querySelectorAll('#dst-analisi-corpo ul.dst-proposta-lista li')]
        .map((li) => [...li.querySelectorAll('b.mono')].map((x) => x.textContent.trim()).join('>'))""")
    visti, doppi = set(), []
    for r in righe:
        if r in visti:
            doppi.append(r)
        visti.add(r)
    verifica(not doppi, "la guida ha %d righe per %d legami: doppi %s" % (len(righe), len(visti), doppi))


@prova("r", "Bug 13: la linguetta «Documenti e NAS» conta un file da verificare in piu' delle righe da sistemare")
def prova_r_minuscola(b):
    b.apri("documenti")
    ling = " ".join(b.page.locator(".dst-passo.on .s").inner_text().split())
    righe = b.page.locator(".dst-box.tono-warn tr.dst-file").count()
    m = re.match(r"^(\d+) da verificare", ling)
    verifica(not m or int(m.group(1)) == righe, "la linguetta dice «%s», le righe «Da sistemare» sono %d" % (ling, righe))


@prova("j", "Bug 9: «Documento della richiesta» non chiede conferma e non si disfa dalla Distinta")
def prova_j_minuscola(b):
    b.apri("documenti")
    nome = "Capitolato fornitura ACME.pdf"
    bottone = b.riga(nome).locator("button", has_text="Documento della richiesta")
    chiede = bottone.get_attribute("hx-confirm")
    b.gesto("documento della richiesta", lambda: b.htmx(lambda: bottone.click(), "/generale"), entrati([nome], "altro", "-", "documento della richiesta"))
    gesti = b.riga(nome).locator("form, button:not([data-apri-file])").count()
    verifica(chiede or gesti, "«Documento della richiesta» parte senza conferma (hx-confirm %r) e la riga dopo non ha gesti: il file e' sul NAS come documento generale e dalla Distinta non si porta a un pezzo" % chiede)


@prova("s", "Bug 4: dopo «✓ Conferma» la pagina salta di migliaia di pixel (lo scroll anchoring del browser e lo swap di htmx)")
def prova_s_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121006.pdf"
    r = b.riga(nome)
    r.scroll_into_view_if_needed()
    b.page.evaluate("(y) => window.scrollBy(0, y)", 0)
    prima = r.bounding_box()["y"]
    misura = "() => [Math.round(window.scrollY), document.documentElement.scrollHeight]"
    m0 = b.page.evaluate(misura)
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121006", "conferma"), ricarica=False)
    dopo = b.riga(nome).bounding_box()["y"]
    fuoco = b.page.evaluate("document.activeElement ? document.activeElement.tagName : ''")
    m1 = b.page.evaluate(misura)
    # l'avviso che il gesto mette in testa sposta la riga di una sessantina di pixel: si tollera
    verifica(abs(dopo - prima) < 100, "la riga di %s era a %d px dall'alto della finestra, dopo la conferma e' a %d px (scrollY e altezza della pagina: prima %s, dopo %s; fuoco su %s)" % (nome, prima, dopo, m0, m1, fuoco))


@prova("E", "due schede sulla distinta: il salvataggio della scheda vecchia si rifiuta e non scrive niente")
def prova_e(b):
    b.apri("distinta")
    p2 = seconda_scheda(b, "distinta")
    trascina(b, casella(b, "7121006"), casella(b, "7120013"))

    def uno(d):
        solo(d, ["relazioni"], "scheda 1")
        verifica(d["relazioni"] == {"+": ["7120013>7121006x1"], "-": ["7120012>7121006x1"]}, "archi: %s" % d["relazioni"])
    b.gesto("scheda 1: salva", lambda: salva(b), uno)
    # la scheda 2 ha lo schema di prima: sposta lo stesso pezzo altrove
    trascina(b, casella(b, "7121006", p2), casella(b, "7120010", p2), p2)
    b.gesto("scheda 2: salva con lo schema vecchio", lambda: salva(b, p2), niente("scheda 2"), pagina=p2, ricarica=False)
    t = b.toast(p2)
    verifica(t.startswith("Niente è cambiato"), "scheda 2: toast %r" % t)
    verifica(p2.locator("#dst-salva").is_visible(), "scheda 2: le modifiche non salvate sono sparite")
    p2.close()


@prova("c", "Bug 2: la linguetta dice «proposte da decidere» ma la Distinta non offre nessun gesto per deciderle")
def prova_c_minuscola(b):
    b.apri("distinta")
    ling = " ".join(b.page.locator(".dst-passo", has_text="Distinta").inner_text().split())
    if "proposte da decidere" not in ling:
        return
    gesti = b.page.locator("[data-azione=accetta-tutto]:visible, #dst-tree .dst-nodo-azioni button:visible, #dst-salva:visible").count()
    chip = b.stato()["analisi"]
    verifica(gesti > 0, "la linguetta dice «%s», il riquadro dice «%s», e nella pagina non c'e' un gesto per decidere (il Congela resta spento)" % (ling, chip))


@prova("d", "Bug 10: un pezzo tolto dalla distinta, nel passo 3, e' «il prodotto»")
def prova_d_minuscola(b):
    b.apri("distinta")
    casella(b, "7121002").locator(".cod").click()
    b.page.locator("[data-azione=elimina]").click()
    b.page.locator("#dst-dettaglio button", has_text="Sì, togli").click()

    def attesa(d):
        solo(d, ["relazioni", "nodi", "archi"], "togli")  # il salvataggio chiude come duplicati le proposte dello STEP del prodotto (vedi c)
        verifica(d["relazioni"] == {"+": [], "-": ["7120010>7121002x1"]}, "archi: %s" % d["relazioni"])
    b.gesto("togli 7121002 e salva", lambda: salva(b), attesa)
    b.apri("documenti")
    testa = " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121002$"))).locator(".dst-blocco-testa").inner_text().split())
    verifica("il prodotto" not in testa, "il blocco di 7121002, fuori dalla distinta, dice «%s»" % testa)


def main():
    ap = argparse.ArgumentParser()
    for k in ["url", "sonda", "thread", "pezzi"]:
        ap.add_argument("--" + k, required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="FP")
    ap.add_argument("--password", default="prova-fp")
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--noti", action="store_true")
    ap.add_argument("--prove", default="")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

    falliti = []
    fatte = 0
    note = {l for l, _, _ in PROVE}
    ignote = [l for l in a.prove if l not in note]
    if ignote:
        print("prove sconosciute: %s" % ignote)
        return 2
    with sync_playwright() as p:
        browser = p.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1440, "height": 900})
        page = ctx.new_page()
        errori = []
        dialoghi = []

        def sorveglia(pg, nome):
            pg.on("dialog", lambda d: (dialoghi.append(d.type), d.accept()))
            pg.on("pageerror", lambda e: errori.append("%s pageerror: %s" % (nome, e)))
            pg.on("console", lambda m: errori.append("%s console: %s" % (nome, m.text)) if m.type == "error" and "Failed to load resource" not in m.text else None)
            pg.on("response", lambda r: errori.append("%s risposta %d: %s %s" % (nome, r.status, r.request.method, r.url)) if r.status >= 400 and not r.url.endswith("/favicon.ico") else None)

        sorveglia(page, "scheda 1")
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=10000)
        b = Banco(page, a)
        b.ctx, b.sorveglia, b.errori, b.dialoghi = ctx, sorveglia, errori, dialoghi
        per_lettera = {l: (l, n, f) for l, n, f in PROVE}
        for lettera, nome, fn in [per_lettera[l] for l in a.prove]:  # nell'ordine chiesto: sono una sequenza
            noto = lettera.islower()
            fatte += 1
            inizio = time.time()
            prima = len(errori)
            try:
                fn(b)
                if len(errori) > prima:
                    raise Rotto("errori nella pagina: %s" % errori[prima:])
                print("  %s  %-78s PASSATO   (%.1fs)" % (lettera, nome, time.time() - inizio))
            except Exception as e:
                del errori[prima:]
                if noto and a.noti:
                    print("  %s  %-78s DIFETTO NOTO: %s" % (lettera, nome, e))
                else:
                    falliti.append(lettera)
                    print("  %s  %-78s FALLITO   %s" % (lettera, nome, e))
                if a.foto:
                    try:
                        b.foto("errore_%s.png" % lettera)
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
