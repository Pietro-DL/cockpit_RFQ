# -*- coding: utf-8 -*-
"""L7 - la Distinta in un BROWSER VERO, quando si confermano le cose (29/09), riscritta per il modello nuovo della
fase 4.4a.3 del giro 4 (domande 27, 28, 29, 30, 9a e 10).

Non lo si lancia a mano: lo avvia `distinta_browser_test.go` (tag `browser`), con la scena di quel file (un
prodotto, quattro assiemi con i figli in comune, trenta file) e la «sonda»: un indirizzo che restituisce la
fotografia delle righe della RFQ (componenti, archi, documenti, proposte, copie sul NAS, versioni, job).

Dopo OGNI gesto lo script controlla tre cose:
  - la pagina: niente errori in console, un solo corpo (#distinta) con un solo pannello, niente id doppi,
    ogni file in una riga sola, i contatori (chip, linguette) uguali alle righe, niente testi del programma a vista
    («ZgotmplZ», «undefined», «NaN», «[object Object]», «%!»);
  - il database: la differenza fra la fotografia di prima e quella di dopo e' quella del gesto, niente di piu'
    (nel passo 2 niente, finche' non si conferma l'albero);
  - la ricarica: ricaricando la pagina si vede lo stesso stato (nel passo 3; nel passo 2 la bozza non si applica da
    sola al ricaricamento, e lo prova S).

Le prove (lettere, scelte dal test Go con --prove). Maiuscole: comportamenti che devono esserci. Minuscole: i difetti
trovati il 29/09 (docs/specs/BUG_DISTINTA_29-09.md); con --noti si eseguono e si scrive l'esito, senza far fallire la
corsa.
  Passo 2, l'albero proposto (scena con la sola distinta del prodotto):
  A  l'albero compare gia' proposto: gli assiemi e i loro pezzi, il figlio in comune una volta, il quinto STEP fuori
  B  rinomina un pezzo proposto             C  togli con la cascata (e «Annulla l'ultima»; il figlio in comune resta)
  D  aggiungi un assieme con il codice interno, e il rifiuto sotto un particolare
  E  il menu del tasto destro, e lo stesso da tastiera (Maiusc+F10, tasto menu), con il fuoco che torna
  Q  il ✓ e il ✗ della minuteria proposta; senza risposta la conferma resta spenta
  R  «Rivedi e conferma»: il riepilogo, «Conferma l'albero», i pezzi nel database, la bozza cancellata
  S  la bozza ripresa dopo il ricaricamento; con l'albero cambiato e' vecchia e non si applica; «Riapri» un pezzo
  T  due schede: la conferma della scheda rimasta indietro si ferma
  Passo 3, Documenti e NAS (distinta fatta):
  F  «✓ Conferma» su un file, con la domanda che dice il percorso sul NAS
  G  doppio clic su «✓ Conferma»      H  conferma di un figlio in comune       I  «Sposta» e poi conferma
  J  «Documento della richiesta», con il tipo scelto                           K  «Metti da parte»
  P  risposta a «che cos'e'?» e conferma                                       L  niente gesto cumulativo sul NAS
  N  «Congela», con la domanda          O  Indietro/Avanti fra i passi dopo un gesto
  M  due schede: i gesti della scheda vecchia si rifiutano, niente doppioni
  U  la bozza dell'albero non confermata: si esce senza perderla, e il passo 3 lo dice
  Difetti (BUG_DISTINTA_29-09.md):
  a  i pezzi dello STEP finivano tutti sotto il prodotto («Crea questi pezzi»)               (Bug 1)
  c  «N proposte da decidere» senza nessun gesto per deciderle                               (Bug 2)
  l  il PDF d'assieme letto con il codice di un figlio e' pronto come 2D del figlio          (Bug 3, aperto: piano)
  s  dopo «✓ Conferma» la pagina saltava di migliaia di pixel                                (Bug 4)
  o  il tasto Indietro del browser mostrava la pagina di prima del gesto                     (Bug 5)
  k  un file messo da parte non si riprende                                                  (Bug 6, aperto: F11)
  n  il rifiuto di «Metti da parte» era mostrato come riuscito                               (Bug 7)
  p  «che cos'e' questo file?» partiva da «3D» per un PDF                                    (Bug 8)
  j  «Documento della richiesta» senza conferma e senza ritorno                              (Bug 9)
  d  un pezzo tolto dalla distinta era «il prodotto» nel passo 3                             (Bug 10)
  h  il blocco di un pezzo in comune nominava un padre solo                                  (Bug 11)
  g  la guida elencava due volte lo stesso legame                                            (Bug 12)
  r  la linguetta «da verificare» contava un file in piu' delle righe                       (Bug 13)
  q  il segno «✓°» senza legenda vicino                                                      (Bug 14)
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


# le tabelle dell'albero: nel passo 2 nessun gesto le cambia finche' non si conferma (la preparazione dei file, che
# la pagina chiede da sola quando si apre, puo' accodare lavoro: i job non contano)
TABELLE_ALBERO = ["componenti", "relazioni", "nodi", "archi", "documenti", "proposte", "provenienze", "copie", "versioni"]


def albero_intatto(contesto):
    def controlla(d):
        cambiate = [k for k in TABELLE_ALBERO if k in d]
        verifica(not cambiate, "%s: la bozza ha scritto nel database: %s" % (contesto, json.dumps({k: d[k] for k in cambiate}, ensure_ascii=False)))
    return controlla


# ------------------------------------------------------------------ la pagina

# Una fotografia della pagina come la vede l'operatore, senza l'avviso del gesto: serve a dire che dopo una
# ricarica lo stato e' lo stesso. Nell'albero: livello, codice, tipo, classi (proposto, nuovo, scartato, rimando) e
# padre di ogni casella.
JS_STATO = r"""() => {
  const t = (e) => (e ? e.textContent.replace(/\s+/g, ' ').trim() : '');
  const out = {passo: '', linguette: [], chips: [], blocchi: [], righe: [], sezioni: [], albero: [], analisi: '', nas: '', ripresa: '', vassoio: ''};
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
  // il codice del padre di una casella: quello della casella del <li> sopra
  const padre = (n) => { const li = n.closest('li'); const su = li && li.parentElement ? li.parentElement.closest('li') : null;
    return su ? t(su.querySelector(':scope > .dst-nodo-box .cod')) : ''; };
  for (const n of document.querySelectorAll('#dst-tree .dst-nodo')) {
    let liv = 0, x = n.closest('li');
    while (x && x.parentElement && x.parentElement.closest('li')) { liv++; x = x.parentElement.closest('li'); }
    out.albero.push(liv + ':' + t(n.querySelector('.cod')) + ':' + t(n.querySelector('.dst-tipo')) + ':' +
      [...n.classList].filter((c) => ['proposto', 'nuovo', 'rimando', 'scartato'].includes(c)).join('+') + ':' + padre(n));
  }
  out.analisi = t(document.getElementById('dst-analisi-chip'));
  out.nas = t(document.getElementById('dst-box-nas'));
  const rp = document.getElementById('dst-ripresa');
  out.ripresa = rp && !rp.hidden ? t(rp) : '';
  const v = document.getElementById('dst-vassoio');
  out.vassoio = v && !v.hidden ? t(v) : '';
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
  if (conta('#dst-dialogo') !== 1) p.push('finestre dei comandi: ' + conta('#dst-dialogo'));
  if (conta('#dst-menu') !== 1) p.push('menu: ' + conta('#dst-menu'));
  if (document.querySelector('#distinta').closest('.dst-pannello')) p.push('#distinta dentro un pannello');
  // un frammento della pagina intera dentro il corpo: la testata o il layout
  if (conta('#distinta header, #distinta nav.topbar, #distinta html, #distinta body')) p.push('pezzi del layout dentro #distinta');
  const ids = {};
  for (const e of document.querySelectorAll('[id]')) ids[e.id] = (ids[e.id] || 0) + 1;
  const doppi = Object.entries(ids).filter(([, n]) => n > 1).map(([k, n]) => k + '×' + n);
  if (doppi.length) p.push('id doppi: ' + doppi.join(', '));
  // niente bottoni dentro una casella che e' gia' un bottone
  if (conta('[role=button] button, button button')) p.push('bottoni dentro un bottone: ' + conta('[role=button] button, button button'));
  const testo = document.body.innerText;
  for (const s of ['ZgotmplZ', 'undefined', 'NaN', '[object Object]', '%!', '<no value>', '{{', 'null ·', '· null']) {
    if (testo.includes(s)) p.push('testo strano a vista: «' + s + '»');
  }
  // un null del programma attaccato a una parola («confermarenull»), non solo fra i separatori
  if (/null/.test(testo)) p.push('testo strano a vista: «null»');
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
  // niente gesto cumulativo (domanda 9a = A)
  if (document.body.innerText.includes('file pronti e copia sul NAS')) p.push('c\'e\' ancora il gesto cumulativo');
  const sistemare = document.querySelectorAll('.dst-box.tono-warn tr.dst-file').length;
  if (numero(/^(\d+) da sistemare/) !== sistemare) p.push('chip «da sistemare» ' + numero(/^(\d+) da sistemare/) + ', righe ' + sistemare);
  const decidere = document.querySelectorAll('.dst-blocco tr.dst-file.decidere').length;
  if (numero(/^(\d+) da decidere accanto ai pezzi/) !== decidere) p.push('chip «da decidere accanto ai pezzi» ' + numero(/^(\d+) da decidere accanto ai pezzi/) + ', righe ' + decidere);
  const parte = document.querySelectorAll('details.dst-box').length ? [...document.querySelectorAll('details.dst-box')].filter((d) => t(d.querySelector('summary')).startsWith('Messi da parte')).map((d) => d.querySelectorAll('li').length)[0] || 0 : 0;
  if (numero(/^(\d+) messi da parte/) !== parte) p.push('chip «messi da parte» ' + numero(/^(\d+) messi da parte/) + ', righe ' + parte);
  // la linguetta del passo dice la stessa cosa delle righe
  const ling = t(document.querySelector('.dst-passo[aria-current] .s'));
  const mp = ling.match(/^(\d+) pront/);
  if (mp && parseInt(mp[1], 10) !== pronti) p.push('la linguetta dice «' + ling + '», righe pronte ' + pronti);
  const ms = ling.match(/^(\d+) file da sistemare/);
  if (ms && parseInt(ms[1], 10) !== sistemare + decidere) p.push('la linguetta dice «' + ling + '», righe da sistemare ' + sistemare + ' e da decidere ' + decidere);
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

JS_BOZZA = r"""() => {
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k && k.indexOf('cockpit.distinta.bozza.') === 0) return JSON.parse(localStorage.getItem(k));
  }
  return null;
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

    def ultima_domanda(self):
        """Il testo dell'ultima finestra di conferma del browser (hx-confirm)."""
        conferme = [m for (tipo, m) in self.dialoghi if tipo == "confirm"]
        return conferme[-1] if conferme else ""

    # ---- la pagina
    def apri(self, passo="distinta", pagina=None):
        p = pagina or self.page
        p.goto(self.base + "?passo=" + passo)
        p.wait_for_load_state("domcontentloaded")
        self.pronta(p)
        p.evaluate("window.__marca = 'viva'")

    def pronta(self, pagina=None):
        """Aspetta che la pagina abbia finito: nessuna richiesta htmx in corso e, nel passo 2, l'albero disegnato."""
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

    def bozza(self, pagina=None):
        return (pagina or self.page).evaluate(JS_BOZZA)

    def fuoco(self, pagina=None):
        """Dove sta il fuoco: la chiave del pezzo (il codice dell'albero), o il ruolo e il testo dell'elemento."""
        return (pagina or self.page).evaluate("""() => { const a = document.activeElement; if (!a) return ''; if (a.dataset && a.dataset.chiave) return 'pezzo:' + a.dataset.chiave;
          return (a.getAttribute('role') || a.tagName.toLowerCase()) + ':' + (a.textContent || '').replace(/\\s+/g, ' ').trim().slice(0, 60); }""")

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


# ------------------------------------------------------------------ l'albero (passo 2)

def albero(b, pagina=None):
    return b.stato(pagina)["albero"]


def casella(b, codice, pagina=None, piena=True):
    p = pagina or b.page
    sel = "#dst-tree .dst-nodo:not(.rimando)" if piena else "#dst-tree .dst-nodo"
    return p.locator(sel).filter(has=p.locator(".cod", has_text=re.compile("^" + re.escape(codice) + "$"))).first


def scegli(b, codice, pagina=None):
    casella(b, codice, pagina).locator(".cod").click()
    (pagina or b.page).wait_for_timeout(120)


def dialogo(b, pagina=None):
    return (pagina or b.page).locator("#dst-dialogo[open]")


def aspetta_dialogo(b, testo=None, pagina=None):
    p = pagina or b.page
    d = dialogo(b, p)
    d.wait_for(timeout=10000)
    if testo:
        try:
            p.wait_for_function("(t) => { const d = document.querySelector('#dst-dialogo[open]'); return d && d.innerText.includes(t); }", arg=testo, timeout=15000)
        except Exception:
            raise Rotto("la finestra non dice «%s»: %s" % (testo, " ".join((d.inner_text() if d.count() else "(chiusa)").split())[:900]))
    return d


def dialogo_chiuso(b, pagina=None):
    (pagina or b.page).wait_for_function("() => !document.querySelector('#dst-dialogo[open]')", timeout=10000)


def codici(a, livello=None):
    return [x.split(":")[1] for x in a if livello is None or x.startswith("%d:" % livello)]


def rinomina(b, codice, nuovo, pagina=None):
    p = pagina or b.page
    scegli(b, codice, p)
    p.locator("#dst-dettaglio button", has_text="Rinomina").click()
    d = aspetta_dialogo(b, pagina=p)
    inp = d.locator("input").first
    verifica(inp.input_value() == codice, "la finestra della rinomina parte da %r, non dal codice proposto %s" % (inp.input_value(), codice))
    inp.fill(nuovo)
    d.locator(".dst-dialogo-azioni button", has_text="Rinomina").click()
    dialogo_chiuso(b, p)


def rivedi(b, pagina=None):
    """«Rivedi e conferma»: il riepilogo nella finestra (una POST che legge soltanto)."""
    p = pagina or b.page
    with p.expect_response(lambda r: "/albero/riepilogo" in r.url, timeout=20000):
        p.locator("[data-azione=rivedi]").click()
    d = aspetta_dialogo(b, "Conferma l'albero", p)
    return d


def bottone_conferma(b, pagina=None):
    return dialogo(b, pagina).locator(".dst-dialogo-azioni button", has_text="Conferma l'albero")


def rispondi_ai_vicini(b, pagina=None, giri=6):
    """Il riepilogo con dei codici quasi uguali senza risposta (P4): «Vai al pezzo», la casella «è un pezzo diverso»
    (mai spuntata da sola), e di nuovo il riepilogo. Restituisce la finestra con la conferma accesa."""
    p = pagina or b.page
    for _ in range(giri):
        d = rivedi(b, p)
        if bottone_conferma(b, p).is_enabled():
            return d
        diverso = d.locator("section", has_text="Codici quasi uguali").locator("button", has_text="È un pezzo diverso")
        verifica(diverso.count() > 0, "il riepilogo non e' confermabile, e non per dei codici quasi uguali: %s" % " ".join(d.inner_text().split())[:900])
        with p.expect_response(lambda r: "/albero/riepilogo" in r.url, timeout=20000):
            diverso.first.click()
        aspetta_dialogo(b, "Conferma l'albero", p)
        if bottone_conferma(b, p).is_enabled():
            return dialogo(b, p)
        d.locator(".dst-dialogo-azioni button", has_text="Torna all'albero").click()
        dialogo_chiuso(b, p)
    raise Rotto("dopo %d giri il riepilogo non e' ancora confermabile" % giri)


def aggiungi_con_i_vicini(b, d, pagina=None):
    """«Aggiungi» nella finestra: con dei codici quasi uguali la finestra li mostra, con la casella «è un pezzo diverso»
    vuota, e al secondo «Aggiungi» il pezzo entra con la domanda aperta (P4: «diverso» non si segna da solo)."""
    p = pagina or b.page
    for giro in range(2):
        with p.expect_response(lambda r: "/bom/codice" in r.url, timeout=15000):
            d.locator(".dst-dialogo-azioni button", has_text="Aggiungi").click()
        p.wait_for_timeout(250)
        if not dialogo(b, p).count():
            break
        spunta = d.locator("input[type=checkbox]")
        verifica(giro == 0 and spunta.count() == 1 and not spunta.is_checked(), "la finestra resta aperta: %s" % " ".join(d.inner_text().split())[:400])
    dialogo_chiuso(b, p)


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


# ------------------------------------------------------------------ il passo 2: l'albero proposto

# la distinta vera della scena: (padre, figlio) sotto gli assiemi
PEZZI = {("7120010", "7121001"), ("7120010", "7121002"), ("7120010", "7121003"), ("7120011", "7121003"), ("7120011", "7121004"),
         ("7120011", "7121005"), ("7120012", "7121003"), ("7120012", "7121006"), ("7120013", "7121001"), ("7120013", "7121007"),
         ("7120013", "7121008")}


@prova("A", "l'albero compare gia' proposto: gli assiemi e i loro pezzi a tutti i livelli, il figlio in comune una volta, il quinto STEP fuori, la vite con la domanda; aprire non scrive")
def prova_a(b):
    prima = b.db()
    b.apri("distinta")
    b.sana("apertura")
    a = albero(b)
    verifica(sorted(codici(a, 1)) == ["7120010", "7120011", "7120012", "7120013"], "sotto il prodotto: %s" % codici(a, 1))
    verifica(all(":proposto" in x for x in a if not x.startswith("0:")), "i pezzi non sono tutti «proposto»: %s" % [x for x in a if ":proposto" not in x])
    coppie = {(x.split(":")[4], x.split(":")[1]) for x in a if x.startswith("2:")}
    verifica(coppie == PEZZI, "i pezzi sotto gli assiemi: in piu' %s, mancano %s" % (sorted(coppie - PEZZI), sorted(PEZZI - coppie)))
    tre = [x for x in a if ":7121003:" in x]
    verifica(len(tre) == 3 and sum(1 for x in tre if "rimando" not in x) == 1, "7121003 sotto tre assiemi: una casella piena e due rimandi: %s" % tre)
    testo = b.page.locator("#dst-analisi").inner_text()
    verifica("ACME-7120014 00 IN_WORK.stp" in testo and "non si raggiunge da nessun prodotto" in testo, "il quinto STEP non e' fra i file senza posto: %s" % testo)
    vite = casella(b, "7121007")
    verifica("minuteria?" in vite.inner_text() and "ISO 4762" in vite.inner_text(), "la vite senza la domanda sulla minuteria: %s" % vite.inner_text())
    corpo = b.page.locator("#distinta").inner_text()
    for vietato in ["Crea questi pezzi", "Accetta la struttura proposta", "Salva la distinta"]:
        verifica(vietato not in corpo, "il passo 2 ha ancora «%s»" % vietato)
    ling = " ".join(b.page.locator(".dst-passo", has_text="Distinta").inner_text().split())
    verifica("12 pezzi, 15 legami da confermare" in ling, "la linguetta: %s" % ling)
    verifica(b.page.locator("[data-azione=rivedi]").is_visible(), "«Rivedi e conferma» non si vede")
    verifica(b.bozza() is None, "aprendo la pagina c'e' gia' una bozza: %s" % b.bozza())
    b.page.wait_for_timeout(500)
    albero_intatto("apertura del passo 2")(differenza(prima, b.db()))


@prova("B", "rinomina di un pezzo proposto: la casella dice il codice nuovo e quello di prima, la bozza lo tiene nel browser, il database no")
def prova_b(b):
    b.apri("distinta")
    b.gesto("rinomina 7121008 in 7121018", lambda: rinomina(b, "7121008", "7121018"), albero_intatto("rinomina"), ricarica=False)
    a = albero(b)
    verifica("7121018" in codici(a) and "7121008" not in codici(a), "l'albero dopo la rinomina: %s" % a)
    verifica("era 7121008" in casella(b, "7121018").inner_text(), "la casella non dice il codice di prima: %s" % casella(b, "7121018").inner_text())
    bz = b.bozza()
    verifica(bz and bz["bozza"]["rinomine"] == [{"nodo": "cod:7121008", "codice": "7121018", "rev": ""}], "la bozza nel browser: %s" % bz)
    verifica(b.fuoco() == "pezzo:cod:7121008", "dopo la rinomina il fuoco e' su %s, non sul pezzo" % b.fuoco())


@prova("C", "togli con la cascata: la finestra dice che cosa va via e che il figlio in comune resta; «Annulla l'ultima»; un pezzo sotto due padri si toglie da tutti o da uno")
def prova_c(b):
    scegli(b, "7120011")
    b.page.locator("[data-azione=elimina]").click()
    d = aspetta_dialogo(b)
    t = " ".join(d.inner_text().split())
    # le etichette sono in maiuscolo (text-transform): si confronta in minuscolo
    verifica("vanno via (3)" in t.lower() and all(c in t for c in ["7120011", "7121004", "7121005"]), "la cascata: %s" % t)
    verifica("restano, sotto altri padri (1)" in t.lower() and "7121003 · sotto 7120010, 7120012" in t, "il figlio in comune che resta: %s" % t)
    verifica(b.fuoco().startswith("button:Annulla"), "nella finestra il fuoco parte da %s, non da «Annulla»" % b.fuoco())
    b.gesto("togli 7120011", lambda: (d.locator("button", has_text="Sì, togli").click(), dialogo_chiuso(b)), albero_intatto("togli"), ricarica=False)
    a = albero(b)
    verifica(not {"7120011", "7121004", "7121005"} & set(codici(a)), "dopo «togli» restano %s" % [x for x in a if x.split(":")[1] in ("7120011", "7121004", "7121005")])
    verifica(sum(1 for x in a if ":7121003:" in x and "rimando" not in x) == 1 and len([x for x in a if ":7121003:" in x]) == 2, "7121003 resta sotto 7120010 e 7120012: %s" % [x for x in a if ":7121003:" in x])
    vs = b.stato()["vassoio"]
    verifica("tolti nella bozza (1)" in vs.lower() and "7120011" in vs and "7121004" in vs and "7121005" in vs, "il vassoio dei tolti: %s" % vs)
    verifica(b.bozza()["bozza"]["tolti"] == [{"nodo": "cod:7120011"}], "la bozza: %s" % b.bozza())
    # «Annulla l'ultima» rimette 7120011, poi lo si toglie di nuovo
    b.page.locator("[data-azione=indietro]").click()
    b.page.wait_for_timeout(150)
    verifica("7120011" in codici(albero(b), 1) and b.bozza()["bozza"]["tolti"] == [], "dopo «Annulla l'ultima»: %s, %s" % (codici(albero(b), 1), b.bozza()))
    scegli(b, "7120011")
    b.page.locator("[data-azione=elimina]").click()
    aspetta_dialogo(b).locator("button", has_text="Sì, togli").click()
    dialogo_chiuso(b)
    # 7121001 sta sotto 7120010 e 7120013: dalla casella sotto 7120010 si sceglie da dove toglierlo, e nessuna risposta e' gia' data
    scegli(b, "7121001")
    b.page.locator("[data-azione=elimina]").click()
    d = aspetta_dialogo(b)
    t = " ".join(d.inner_text().split())
    verifica("lo togli da tutti, o solo da sotto 7120010?" in t, "la domanda sul pezzo in comune: %s" % t)
    verifica(b.fuoco().startswith("button:Annulla"), "la domanda parte con il fuoco su %s" % b.fuoco())
    b.gesto("togli 7121001 solo da sotto 7120010", lambda: (d.locator("button", has_text="Togli solo da sotto 7120010").click(), dialogo_chiuso(b)),
            albero_intatto("togli da qui"), ricarica=False)
    a = albero(b)
    uno = [x for x in a if ":7121001:" in x]
    verifica(len(uno) == 1 and uno[0].endswith(":7120013"), "7121001 resta solo sotto 7120013: %s" % uno)
    verifica({"nodo": "cod:7121001", "padre": "cod:7120010"} in b.bozza()["bozza"]["tolti"], "la bozza: %s" % b.bozza())


@prova("D", "aggiungi un assieme: il codice parte vuoto, «usa il codice interno 7120001-A01»; sotto un particolare non si mette niente")
def prova_d(b):
    def fai():
        b.page.locator("[data-nuovo=sottoassieme]").click()
        d = aspetta_dialogo(b, "Aggiungi un pezzo")
        cod = d.locator("input.mono").first
        verifica(cod.input_value() == "", "il codice di un pezzo nuovo parte scritto: %r (P5)" % cod.input_value())
        d.locator("button", has_text="usa il codice interno 7120001-A01").click()
        verifica(cod.input_value() == "7120001-A01", "il codice interno: %r" % cod.input_value())
        padre = d.locator("select").nth(1)
        verifica(padre.evaluate("(s) => s.options[s.selectedIndex].text") == "Prodotto 7120001", "il padre: %s" % padre.evaluate("(s) => s.options[s.selectedIndex].text"))
        aggiungi_con_i_vicini(b, d)
    b.gesto("aggiungi 7120001-A01", fai, albero_intatto("aggiungi"), ricarica=False)
    a = albero(b)
    verifica("1:7120001-A01:Assieme:nuovo:7120001" in a, "l'assieme nuovo nell'albero: %s" % [x for x in a if "A01" in x])
    bz = b.bozza()["bozza"]
    verifica(bz["aggiunti"] == [{"id": "nuovo:1", "codice": "7120001-A01", "rev": "", "tipo": "sottoassieme", "padre": "cod:7120001", "qta": 1}] and bz["diversi"] == [],
             "la bozza: %s («diverso» non si segna da solo, P4)" % bz)
    # un particolare non ha figli (6b): il rilascio si rifiuta, e l'albero resta com'era
    prima = albero(b)
    trascina(b, casella(b, "7121006"), casella(b, "7121002"))
    verifica("sotto non ci va niente" in b.toast(), "il rilascio su un particolare: %r" % b.toast())
    verifica(albero(b) == prima, "l'albero e' cambiato dopo un rilascio rifiutato")


@prova("E", "il menu del tasto destro sui pezzi, e lo stesso da tastiera (Maiusc+F10, tasto menu): le frecce, Esc, il fuoco che torna; un comando dal menu")
def prova_e(b):
    p = b.page
    menu = p.locator("#dst-menu")
    casella(b, "7121002").locator(".cod").click(button="right")
    menu.wait_for(state="visible", timeout=5000)
    voci = [" ".join(x.split()) for x in menu.locator(".voce").all_inner_texts()]
    for attesa in ["Rinomina il codice…", "È un particolare commerciale", "Togli dall'albero…", "Anche sotto un altro assieme…"]:
        verifica(any(v.startswith(attesa) for v in voci), "il menu di 7121002 non ha «%s»: %s" % (attesa, voci))
    verifica(not any(v.startswith("Aggiungi un pezzo sotto") for v in voci), "un particolare offre «Aggiungi un pezzo sotto»: %s" % voci)
    verifica(b.fuoco().startswith("menuitem:"), "aperto il menu, il fuoco e' su %s" % b.fuoco())
    primo = b.fuoco()
    p.keyboard.press("ArrowDown")
    verifica(b.fuoco() != primo and b.fuoco().startswith("menuitem:"), "la freccia non muove il fuoco nel menu: %s" % b.fuoco())
    p.keyboard.press("Escape")
    verifica(menu.is_hidden(), "Esc non chiude il menu")
    verifica(b.fuoco() == "pezzo:cod:7121002", "chiuso il menu il fuoco e' su %s, non sul pezzo" % b.fuoco())
    # da tastiera: Maiusc+F10 sul pezzo, poi «Rinomina…», poi Esc nella finestra: il fuoco torna al pezzo
    casella(b, "7121006").locator(".cod").focus()
    p.keyboard.press("Shift+F10")
    menu.wait_for(state="visible", timeout=5000)
    verifica(b.fuoco().startswith("menuitem:"), "Maiusc+F10: il fuoco e' su %s" % b.fuoco())
    for _ in range(12):
        if b.fuoco().startswith("menuitem:Rinomina"):
            break
        p.keyboard.press("ArrowDown")
    verifica(b.fuoco().startswith("menuitem:Rinomina"), "nel menu da tastiera non si arriva a «Rinomina»: %s" % b.fuoco())
    p.keyboard.press("Enter")
    d = aspetta_dialogo(b)
    verifica(b.fuoco().startswith("input:"), "la finestra della rinomina si apre con il fuoco su %s" % b.fuoco())
    p.keyboard.press("Escape")
    dialogo_chiuso(b)
    verifica(b.fuoco() == "pezzo:cod:7121006", "chiusa la finestra il fuoco e' su %s, non sul pezzo" % b.fuoco())
    # il tasto menu
    p.keyboard.press("ContextMenu")
    menu.wait_for(state="visible", timeout=5000)
    p.keyboard.press("Escape")
    verifica(menu.is_hidden() and b.fuoco() == "pezzo:cod:7121006", "il tasto menu: %s" % b.fuoco())
    # un comando dal menu: un particolare sotto l'assieme nuovo
    def fai():
        casella(b, "7120001-A01").locator(".cod").click(button="right")
        menu.wait_for(state="visible", timeout=5000)
        menu.locator(".voce", has_text="Aggiungi un pezzo sotto").click()
        d = aspetta_dialogo(b, "Aggiungi un pezzo")
        verifica(d.locator("select").nth(1).evaluate("(s) => s.options[s.selectedIndex].text") == "Assieme 7120001-A01", "il padre scelto dal menu")
        d.locator("input.mono").first.fill("7129876")
        aggiungi_con_i_vicini(b, d)
    b.gesto("aggiungi 7129876 dal menu", fai, albero_intatto("aggiungi dal menu"), ricarica=False)
    verifica("2:7129876:Particolare:nuovo:7120001-A01" in albero(b), "il pezzo aggiunto dal menu: %s" % [x for x in albero(b) if "7129876" in x])


@prova("Q", "la minuteria proposta: senza risposta il riepilogo la dice aperta e la conferma e' spenta; ✓ la fa particolare commerciale, ✗ no")
def prova_q(b):
    d = None
    def leggi():
        nonlocal d
        d = rivedi(b)
    b.gesto("il riepilogo con la domanda aperta", leggi, albero_intatto("riepilogo"), ricarica=False)
    t = " ".join(d.inner_text().split())
    verifica("proposta commerciale senza ✓ o ✗" in t and "senza risposta" in t, "il riepilogo non dice la domanda aperta: %s" % t[:900])
    verifica(bottone_conferma(b).is_disabled(), "con una domanda aperta «Conferma l'albero» e' acceso")
    d.locator(".dst-dialogo-azioni button", has_text="Torna all'albero").click()
    dialogo_chiuso(b)
    vite = casella(b, "7121007")
    vite.locator(".dst-minuteria button", has_text="✓").click()
    b.page.wait_for_timeout(150)
    vite = casella(b, "7121007")
    verifica("minuteria: particolare commerciale" in vite.inner_text() and vite.locator(".dst-tipo").first.inner_text().strip().lower() == "particolare commerciale",
             "dopo il ✓: %s" % " ".join(vite.inner_text().split()))
    verifica(b.fuoco().startswith("button:✓"), "dopo il ✓ il fuoco e' su %s" % b.fuoco())
    verifica({"nodo": "cod:7121007", "risposta": "si"} in b.bozza()["bozza"]["commerciali"], "la bozza: %s" % b.bozza())
    casella(b, "7121007").locator(".dst-minuteria button", has_text="✗").click()
    b.page.wait_for_timeout(150)
    verifica("non è minuteria" in casella(b, "7121007").inner_text() and {"nodo": "cod:7121007", "risposta": "no"} in b.bozza()["bozza"]["commerciali"], "dopo il ✗: %s" % b.bozza())
    casella(b, "7121007").locator(".dst-minuteria button", has_text="✓").click()
    b.page.wait_for_timeout(150)
    verifica({"nodo": "cod:7121007", "risposta": "si"} in b.bozza()["bozza"]["commerciali"], "di nuovo il ✓: %s" % b.bozza())


@prova("R", "«Rivedi e conferma»: il riepilogo con le sezioni, «Conferma l'albero» scrive una volta sola quello che dice, la bozza si cancella, la pagina si rilegge")
def prova_r(b):
    d = rispondi_ai_vicini(b)
    t = " ".join(d.inner_text().split())
    for sezione in ["Pezzi che nascono", "La cascata dei pezzi tolti", "Proposte di minuteria", "Legami nuovi", "✓ particolare commerciale"]:
        verifica(sezione.lower() in t.lower(), "il riepilogo non ha «%s»: %s" % (sezione, t[:1200]))

    def attesa(dd):
        solo(dd, ["componenti", "relazioni", "nodi", "archi", "job"], "conferma dell'albero")
        nuovi = sorted(dd["componenti"]["+"])
        attesi = sorted(["7120010|sottoassieme", "7120012|sottoassieme", "7120013|sottoassieme", "7120001-A01|sottoassieme", "7121001|sciolto", "7121002|sciolto",
                         "7121003|sciolto", "7121006|sciolto", "7121007|commerciale", "7121018|sciolto", "7129876|sciolto"])
        verifica(nuovi == attesi and not dd["componenti"]["-"], "componenti nuovi %s, attesi %s" % (nuovi, attesi))
        rel = sorted(dd["relazioni"]["+"])
        attese = sorted(["7120001>7120010x1", "7120001>7120012x1", "7120001>7120013x1", "7120001>7120001-A01x1", "7120001-A01>7129876x1", "7120010>7121002x1",
                         "7120010>7121003x2", "7120012>7121003x1", "7120012>7121006x1", "7120013>7121001x1", "7120013>7121007x4", "7120013>7121018x1"])
        verifica(rel == attese and not dd["relazioni"]["-"], "legami nuovi %s, attesi %s" % (rel, attese))
    conferma = bottone_conferma(b)
    b.gesto("conferma l'albero", lambda: (b.htmx(lambda: conferma.dblclick(), "/albero/conferma"), dialogo_chiuso(b), b.page.wait_for_timeout(800)), attesa, ricarica=False)
    verifica(b.bozza() is None, "dopo la conferma la bozza e' ancora nel browser: %s" % b.bozza())
    ling = " ".join(b.page.locator(".dst-passo", has_text="Distinta").inner_text().split())
    verifica("da confermare" not in ling, "dopo la conferma la linguetta dice ancora: %s" % ling)
    a = albero(b)
    verifica("7121018" in codici(a) and not [x for x in a if ":7120010:" in x and "proposto" in x], "dopo la conferma l'albero: %s" % a)
    b.ricarica_uguale("dopo la conferma")


@prova("S", "la bozza nel browser: al ricaricamento la pagina chiede se riprenderla; con l'albero cambiato e' vecchia e non si applica; «Riapri» un pezzo scartato")
def prova_s(b):
    b.apri("distinta")
    b.gesto("rinomina 7121006 in 7121016", lambda: rinomina(b, "7121006", "7121016"), albero_intatto("rinomina"), ricarica=False)
    b.page.reload()
    b.page.wait_for_load_state("domcontentloaded")
    b.pronta()
    r = b.page.locator("#dst-ripresa")
    verifica(r.is_visible() and r.get_attribute("data-stato") == "uguale", "al ricaricamento la bozza non si propone: %s" % b.stato()["ripresa"])
    t = " ".join(r.inner_text().split())
    verifica(re.search(r"C'è una bozza non confermata delle \d\d:\d\d \(1 modifica\)", t), "il testo della bozza trovata: %s" % t)
    verifica("7121006" in codici(albero(b)) and "7121016" not in codici(albero(b)), "la bozza si e' applicata da sola: %s" % albero(b))
    verifica(not b.page.locator("#dst-salva").is_visible(), "con la bozza trovata e non scelta «Rivedi e conferma» si vede")
    b.page.locator("[data-nuovo=sciolto]").click()
    verifica("Prima scegli che cosa fare della bozza" in b.toast() and not dialogo(b).count(), "con la bozza trovata si lavora sull'albero: %r" % b.toast())
    b.gesto("riprendi la bozza", lambda: r.locator("button", has_text="Riprendila").click(), albero_intatto("riprendi"), ricarica=False)
    verifica("7121016" in codici(albero(b)) and r.is_hidden(), "dopo «Riprendila»: %s" % albero(b))
    # l'albero cambia fuori da questa pagina (un collega scarta 7121008): la bozza e' vecchia
    fascicolo = b.base.replace("/distinta", "/fascicolo")
    stati = b.page.evaluate("""async ([base, f]) => { const r = await fetch(base + '/albero'); const a = await r.json(); const out = [];
      for (const riga of a.nodi.find((n) => n.chiave === 'cod:7121008').righe) {
        const x = await fetch(f + '/nodo/' + riga.proposta + '/scarta', {method: 'POST', headers: {'HX-Request': 'true', 'HX-Current-URL': f}});
        out.push(x.status);
      }
      return out; }""", [b.base, fascicolo])
    verifica(stati and all(s == 200 for s in stati), "lo scarto di 7121008: %s" % stati)
    b.page.reload()
    b.page.wait_for_load_state("domcontentloaded")
    b.pronta()
    verifica(r.is_visible() and r.get_attribute("data-stato") == "vecchia" and "è vecchia, e non si applica da sola" in r.inner_text(),
             "con l'albero cambiato: %s" % b.stato()["ripresa"])
    a = albero(b)
    verifica("7121016" not in codici(a) and any(":7121008:" in x and "scartato" in x for x in a), "la bozza vecchia si e' applicata, o lo scartato non si vede: %s" % a)
    b.gesto("scarta la bozza vecchia", lambda: r.locator("button", has_text="Scartala").click(), albero_intatto("scarta la bozza"), ricarica=False)
    verifica(r.is_hidden() and b.bozza() is None, "dopo «Scartala»: %s" % b.bozza())
    # «Riapri» il pezzo scartato, dal menu: torna proposto
    def riapri():
        casella(b, "7121008").locator(".cod").click(button="right")
        b.page.locator("#dst-menu .voce", has_text="Riapri il pezzo scartato").click()
        b.page.wait_for_function("() => [...document.querySelectorAll('#dst-tree .dst-nodo')].some((n) => n.textContent.includes('7121008') && n.classList.contains('proposto') && !n.classList.contains('scartato'))", timeout=15000)

    def attesa(d):
        solo(d, ["nodi", "archi", "job"], "riapri")
    b.gesto("riapri 7121008", riapri, attesa, ricarica=False)


@prova("T", "due schede sull'albero: la scheda rimasta indietro ha il riepilogo di prima, e la sua conferma si ferma senza scrivere")
def prova_t(b):
    b.apri("distinta")
    p2 = seconda_scheda(b, "distinta")
    # la scheda 2 prepara la sua conferma e resta li'
    casella(b, "7121007", p2).locator(".dst-minuteria button", has_text="✓").click()
    p2.wait_for_timeout(150)
    rispondi_ai_vicini(b, p2)
    verifica(bottone_conferma(b, p2).is_enabled(), "scheda 2: la conferma e' spenta")
    # la scheda 1 conferma
    b.page.reload()
    b.pronta()
    if b.page.locator("#dst-ripresa").is_visible():
        b.page.locator("#dst-ripresa button", has_text="Riprendila").click()
    rispondi_ai_vicini(b)

    def uno(d):
        solo(d, ["componenti", "relazioni", "nodi", "archi", "job"], "scheda 1")
        verifica(d.get("componenti", {}).get("+"), "scheda 1: nessun componente nuovo: %s" % d)
    b.gesto("scheda 1: conferma", lambda: (b.htmx(lambda: bottone_conferma(b).click(), "/albero/conferma"), dialogo_chiuso(b), b.page.wait_for_timeout(800)), uno, ricarica=False)
    # la scheda 2 conferma con il riepilogo di prima: si ferma
    b.gesto("scheda 2: conferma con il riepilogo vecchio", lambda: (b.htmx(lambda: bottone_conferma(b, p2).click(), "/albero/conferma", p2), p2.wait_for_timeout(300)),
            niente("scheda 2"), pagina=p2, ricarica=False)
    t = " ".join(dialogo(b, p2).inner_text().split())
    verifica("Niente è cambiato" in t, "scheda 2: la conferma rifiutata dice %s" % t[:600])
    p2.close()


# ------------------------------------------------------------------ il passo 3: Documenti e NAS (distinta fatta)

@prova("F", "«✓ Conferma» su un file: chiede, con il percorso sul NAS; un documento, una copia in coda, la riga confermata, la ricarica uguale")
def prova_f(b):
    b.apri("documenti")
    b.sana("apertura")
    verifica(b.stato_riga("ACME-7121002.pdf") == "pronto da confermare", "prima: %s" % b.stato_riga("ACME-7121002.pdf"))
    b.gesto("conferma di ACME-7121002.pdf", lambda: b.conferma_riga("ACME-7121002.pdf"),
            entrati(["ACME-7121002.pdf"], "disegno_2d", "7121002", "conferma di ACME-7121002.pdf"), ricarica=False)
    dom = b.ultima_domanda()
    verifica(dom.startswith("Confermare «ACME-7121002.pdf» come 2D di 7121002?") and "Va sul NAS in " in dom and "7121002" in dom.split("Va sul NAS in ")[1],
             "la domanda di «✓ Conferma»: %r" % dom)
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
    albero_ = b.stato()["albero"]
    pieni = [x for x in albero_ if ":7121003:" in x and "rimando" not in x]
    verifica(len(pieni) == 1, "caselle piene di 7121003 nell'albero: %s" % pieni)


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


@prova("J", "«Documento della richiesta» con il tipo scelto e la domanda: un documento senza pezzo, la sua copia, la riga fra i documenti della richiesta")
def prova_j(b):
    b.apri("documenti")
    nome = "Capitolato fornitura ACME.pdf"
    form = b.riga(nome).locator("form[hx-post$='/generale']")
    form.locator("select[name=tipo]").select_option("capitolato")
    b.gesto("documento della richiesta", lambda: b.htmx(lambda: form.locator("button", has_text="Documento della richiesta").click(), "/generale"),
            entrati([nome], "capitolato", "-", "documento della richiesta"))
    verifica("entra come documento della richiesta" in b.ultima_domanda(), "la domanda: %r" % b.ultima_domanda())
    verifica(b.blocco_di(nome).startswith("Documenti della richiesta"), "la riga e' in %s" % b.blocco_di(nome))
    # un file tecnico non diventa un documento della richiesta
    verifica(b.riga("ACME-7121005.dxf").locator("form[hx-post$='/generale']").count() == 0, "il DXF ha «Documento della richiesta»")


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


@prova("p", "Bug 8: «che cos'e' questo file?» per un PDF partiva con «3D» gia' scelto e il nome del file come codice")
def prova_p_minuscola(b):
    b.apri("documenti")
    form = b.riga("ACME-7120012.pdf").locator("form[hx-post$='/decidi']")
    scelto = form.locator("select[name=tipo]").evaluate("(s) => s.options[s.selectedIndex].value")
    codice = form.locator("input[name=codice]").input_value()
    verifica(scelto != "cad_3d" and scelto == "", "per ACME-7120012.pdf (un PDF) la tendina «che cos'è» parte da %r: un «Salva» distratto lo registra come CAD 3D" % scelto)
    verifica(codice == "", "il codice parte dal nome del file: %r" % codice)
    verifica(form.locator("select[name=tipo]").get_attribute("required") is not None, "la tendina non e' obbligatoria")


@prova("L", "niente gesto cumulativo sul NAS (9a = A): ogni file si conferma da solo, e la sua domanda dice il pezzo e il percorso sul NAS")
def prova_l(b):
    b.apri("documenti")
    verifica("file pronti e copia sul NAS" not in b.page.locator("#distinta").inner_text(), "c'e' ancora «Conferma i N file pronti e copia sul NAS»")
    verifica(b.page.locator("#dst-box-nas button[type=submit], #dst-box-nas form").count() == 0, "il riquadro della copia sul NAS ha un modulo")
    nome = "ACME-7121005.pdf"
    verifica(b.stato_riga(nome) == "pronto da confermare", "%s: %s" % (nome, b.stato_riga(nome)))
    domanda = b.riga(nome).locator("form[hx-post$='/conferma']").get_attribute("hx-confirm") or ""
    verifica("come 2D di 7121005" in domanda and "Va sul NAS in" in domanda, "la domanda di «✓ Conferma» per %s non dice a quale pezzo e dove va: %r" % (nome, domanda))


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
    # Riscritta per lo Smistamento (Distinta, fase 4.4a.3): la seconda prova usava la conferma cumulativa con la firma
    # vecchia, che non c'e' piu' (9a = A); al suo posto il «✓ Conferma» della scheda vecchia su un file che la prima ha
    # messo da parte. Casi: prima 4, dopo 4.
    b.apri("documenti")
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121006.pdf"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121006", "scheda 1"))
    # la scheda 2 non sa niente: il suo «✓ Conferma» sullo stesso file
    b.gesto("scheda 2: conferma dello stesso file", lambda: b.conferma_riga(nome, p2), niente("scheda 2: conferma dello stesso file"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: conferma dello stesso file")
    verifica(b.stato_riga(nome, p2).startswith("✓ confermato"), "scheda 2 dopo il rifiuto: %s" % b.stato_riga(nome, p2))
    p2.close()
    # la scheda 1 mette da parte un file, la scheda 2 lo conferma
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121006 00 IN_WORK.stp"

    def parte(d):
        solo(d, ["proposte"], "scheda 1: metti da parte")
    b.gesto("scheda 1: metti da parte " + nome, lambda: b.htmx(lambda: b.riga(nome).locator("button", has_text="Metti da parte").click(), "/scarta"), parte)
    b.gesto("scheda 2: conferma di un file messo da parte", lambda: b.conferma_riga(nome, p2), niente("scheda 2: conferma di un file messo da parte"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: conferma di un file messo da parte")
    p2.close()
    # «Documento della richiesta» e «Sposta» su un file che la scheda 1 ha appena confermato
    p2 = seconda_scheda(b, "documenti")
    nome = "Capitolato fornitura ACME.pdf"
    fatto = "ACME-7121008.pdf"
    b.gesto("scheda 1: conferma di " + fatto, lambda: b.conferma_riga(fatto), entrati([fatto], "disegno_2d", "7121008", "scheda 1"))
    form = b.riga(nome, p2).locator("form[hx-post$='/generale']")
    form.locator("select[name=tipo]").select_option("altro")
    b.riga(nome).locator("form[hx-post$='/generale'] select[name=tipo]").select_option("altro")
    b.gesto("scheda 1: documento della richiesta", lambda: b.htmx(lambda: b.riga(nome).locator("button", has_text="Documento della richiesta").click(), "/generale"),
            entrati([nome], "altro", "-", "scheda 1: documento della richiesta"))
    b.gesto("scheda 2: documento della richiesta su un file gia' deciso",
            lambda: b.htmx(lambda: form.locator("button", has_text="Documento della richiesta").click(), "/generale", p2),
            niente("scheda 2: documento della richiesta"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: documento della richiesta")
    p2.close()
    p2 = seconda_scheda(b, "documenti")
    nome = "ACME-7121008 00 IN_WORK.stp"
    b.gesto("scheda 1: conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "cad_3d", "7121008", "scheda 1"))
    b.gesto("scheda 2: sposta un file confermato", lambda: b.sposta_riga(nome, "7121001", p2), niente("scheda 2: sposta"), pagina=p2, ricarica=False)
    rifiutato(b, p2, "scheda 2: sposta")
    p2.close()


@prova("n", "Bug 7: «Metti da parte» su un file gia' deciso in un'altra scheda: il rifiuto era mostrato come riuscito")
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


@prova("N", "i file pronti confermati uno per uno, poi «Congela» con la domanda: una versione congelata, la pagina lo dice, i gesti sull'albero spariscono")
def prova_n(b):
    b.apri("documenti")
    # senza il gesto cumulativo (9a = A) i file pronti si confermano uno per uno: ognuno chiede, e ognuno entra da solo
    confermati = []
    for _ in range(60):
        pronti = b.page.evaluate("() => [...document.querySelectorAll('tr.dst-file.pronto td.nome > span.mono')].map((e) => e.textContent.trim())")
        if not pronti:
            break
        nome = pronti[0]
        prima = b.db()
        b.conferma_riga(nome)
        b.pronta()
        b.page.wait_for_timeout(250)
        d = differenza(prima, b.db())
        verifica(len(d.get("documenti", {}).get("+", [])) <= 1 and b.stato_riga(nome).startswith("✓ confermato"), "la conferma di %s: %s, database %s" % (nome, b.stato_riga(nome), d.get("documenti")))
        verifica(b.ultima_domanda().startswith("Confermare «" + nome + "»"), "«✓ Conferma» di %s non ha chiesto: %r" % (nome, b.ultima_domanda()))
        confermati.append(nome)
    verifica(len(confermati) > 5, "file pronti confermati uno per uno: %s" % confermati)
    box = b.page.locator(".dst-box", has=b.page.locator(".dst-label", has_text="Congela la distinta"))
    bottone = box.locator("button[type=submit]")
    if bottone.is_disabled():
        raise Rotto("non si congela: %s" % " ".join(box.inner_text().split()))

    def attesa(d):
        solo(d, ["versioni", "job"], "congela")
        verifica(d["versioni"]["+"] == ["1|congelata"] and not d["versioni"]["-"], "versioni: %s" % d["versioni"])
    box.locator("input[name=motivo]").fill("prima baseline di prova")
    b.gesto("congela", lambda: b.htmx(lambda: bottone.click(), "/fascicolo/congela"), attesa)
    verifica(b.ultima_domanda().startswith("Congelare la V1"), "«Congela la V1» non ha chiesto: %r" % b.ultima_domanda())
    verifica("congelata nella V1" in b.page.locator(".dst-passo", has_text="Distinta").inner_text(), "la linguetta della Distinta non dice congelata")
    b.apri("distinta")
    verifica(b.page.locator("[data-nuovo]").count() == 0, "con la distinta congelata ci sono ancora i bottoni «+ Assieme»")
    verifica(b.page.locator("[data-azione=rivedi]").count() == 0 or not b.page.locator("#dst-salva").is_visible(), "con la distinta congelata si conferma ancora l'albero")


@prova("O", "«Indietro/Avanti» fra i passi dopo un gesto: lo stato e' quello di adesso")
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


@prova("U", "la bozza dell'albero non confermata: si esce dal passo 2 senza domande e senza perderla, e il passo 3 dice che c'e'")
def prova_u(b):
    b.apri("distinta")

    def fai():
        scegli(b, "7121008")
        b.page.locator("#dst-dettaglio select[data-fuoco='det|tipo']").select_option("commerciale")
        b.page.wait_for_timeout(200)
    b.gesto("7121008 commerciale nella bozza", fai, albero_intatto("tipo nella bozza"), ricarica=False)
    verifica(b.bozza()["bozza"]["tipi"] == [{"nodo": "cod:7121008", "tipo": "commerciale"}], "la bozza: %s" % b.bozza())
    del b.dialoghi[:]
    b.page.locator(".dst-navbar a", has_text="Avanti").click()
    b.page.wait_for_url("**passo=documenti*")
    b.pronta()
    verifica(not b.dialoghi, "uscendo il browser ha chiesto: %s (la bozza e' salvata, non si perde)" % b.dialoghi)
    nota = b.page.locator("#dst-bozza-nota")
    verifica(nota.is_visible() and "bozza dell'albero non confermata (1 modifica" in nota.inner_text(), "il passo 3 non dice la bozza: %s" % (nota.inner_text() if nota.count() else "(assente)"))
    b.page.locator(".dst-navbar a", has_text="Distinta").click()
    b.page.wait_for_url("**passo=distinta*")
    b.pronta()
    verifica(b.page.locator("#dst-ripresa").is_visible(), "tornando al passo 2 la bozza non si propone")


@prova("o", "Bug 5: il tasto Indietro del browser dopo un gesto mostrava la pagina di prima del gesto")
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


@prova("h", "Bug 11: il blocco di un pezzo in comune diceva un padre solo (e la quantita' di quell'arco)")
def prova_h_minuscola(b):
    b.apri("documenti")
    testa = " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121003$"))).locator(".dst-blocco-testa").inner_text().split())
    mancano = [p for p in ["7120010", "7120011", "7120012"] if p not in testa]
    verifica(not mancano and "5 in tutto" in testa, "il blocco di 7121003 (sotto tre assiemi, 5 pezzi in tutto) dice «%s»: non nomina %s" % (testa, mancano))


@prova("q", "Bug 14: il segno «✓°» nella casella di un pezzo, senza legenda vicino")
def prova_q_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121002.pdf"
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121002", "conferma"), ricarica=False)
    slot = " ".join(b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121002$"))).locator(".dst-slot", has_text="2D").inner_text().split())
    verifica("✓°" not in slot, "la casella del 2D di 7121002 dice «%s»: il segno «✓°» si spiega solo nella legenda della matrice in fondo" % slot)
    verifica("la copia sul NAS è in coda" in b.page.locator(".dst-legenda-caselle").inner_text(), "la legenda delle caselle non e' vicino ai blocchi")


@prova("l", "Bug 3: il PDF d'assieme letto con il codice di un figlio e' pronto come 2D del figlio (piano, fasi 4.2 e 4.6)")
def prova_l_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7120011.pdf"
    stato = b.stato_riga(nome)
    blocco = b.blocco_di(nome)
    verifica(not (stato == "pronto da confermare" and blocco == "7121003"),
             "%s (il nome dice 7120011, un assieme della distinta) e' «%s» sotto %s: «✓ Conferma» lo scrive come 2D del figlio" % (nome, stato, blocco))


@prova("j", "Bug 9: «Documento della richiesta» senza conferma e senza ritorno")
def prova_j_minuscola(b):
    b.apri("documenti")
    nome = "Capitolato fornitura ACME.pdf"
    form = b.riga(nome).locator("form[hx-post$='/generale']")
    chiede = form.get_attribute("hx-confirm")
    form.locator("select[name=tipo]").select_option("altro")
    b.gesto("documento della richiesta", lambda: b.htmx(lambda: form.locator("button", has_text="Documento della richiesta").click(), "/generale"),
            entrati([nome], "altro", "-", "documento della richiesta"))
    gesti = b.riga(nome).locator("form[hx-post$='/assegna'] input[name=documento]").count()
    verifica(chiede and gesti, "«Documento della richiesta» parte senza conferma (hx-confirm %r) o la riga dopo non si porta a un pezzo (%d)" % (chiede, gesti))


@prova("s", "Bug 4: dopo «✓ Conferma» la pagina saltava di migliaia di pixel (lo scroll anchoring del browser e lo swap di htmx)")
def prova_s_minuscola(b):
    b.apri("documenti")
    nome = "ACME-7121006.pdf"
    r = b.riga(nome)
    r.scroll_into_view_if_needed()
    prima = r.bounding_box()["y"]
    misura = "() => [Math.round(window.scrollY), document.documentElement.scrollHeight]"
    m0 = b.page.evaluate(misura)
    b.gesto("conferma di " + nome, lambda: b.conferma_riga(nome), entrati([nome], "disegno_2d", "7121006", "conferma"), ricarica=False)
    dopo = b.riga(nome).bounding_box()["y"]
    fuoco = b.page.evaluate("() => { const a = document.activeElement; return a ? a.tagName + (a.closest('[data-riga]') ? ' nella riga ' + a.closest('[data-riga]').querySelector('.mono').textContent : '') : ''; }")
    m1 = b.page.evaluate(misura)
    verifica(abs(dopo - prima) < 100, "la riga di %s era a %d px dall'alto della finestra, dopo la conferma e' a %d px (scrollY e altezza della pagina: prima %s, dopo %s; fuoco su %s)" % (nome, prima, dopo, m0, m1, fuoco))
    verifica(nome in fuoco, "dopo la conferma il fuoco e' su %s, non sulla riga del file" % fuoco)


# ------------------------------------------------------------------ i difetti del passo 2

@prova("a", "Bug 1: i pezzi degli STEP finivano tutti sotto il prodotto; nell'albero proposto stanno sotto i loro assiemi")
def prova_a_minuscola(b):
    b.apri("distinta")
    a = albero(b)
    sotto_prodotto = [x for x in a if x.startswith("1:7121")]
    verifica(not sotto_prodotto, "sotto il prodotto ci sono dei pezzi: %s; gli assiemi hanno %s" % (sotto_prodotto, [x for x in a if x.startswith("2:")]))
    verifica("7121009" not in codici(a), "7121009, del quinto STEP che non sta sotto il prodotto, e' nell'albero")


@prova("g", "Bug 12: la guida elencava due volte lo stesso legame; nell'albero ogni legame c'e' una volta")
def prova_g_minuscola(b):
    b.apri("distinta")
    coppie = [(x.split(":")[4], x.split(":")[1]) for x in albero(b) if not x.startswith("0:")]
    doppi = sorted({c for c in coppie if coppie.count(c) > 1})
    verifica(not doppi, "l'albero ha %d caselle per %d legami: doppi %s" % (len(coppie), len(set(coppie)), doppi))
    verifica(b.page.locator("#dst-analisi-corpo ul.dst-proposta-lista li").count() == 0, "c'e' ancora l'elenco della guida")


@prova("r", "Bug 13: la linguetta «Documenti e NAS» contava un file da verificare in piu' delle righe")
def prova_r_minuscola(b):
    b.apri("documenti")
    ling = " ".join(b.page.locator(".dst-passo[aria-current] .s").inner_text().split())
    righe = b.page.locator(".dst-box.tono-warn tr.dst-file").count() + b.page.locator(".dst-blocco tr.dst-file.decidere").count()
    m = re.match(r"^(\d+) (da verificare|file da sistemare)", ling)
    verifica(m and int(m.group(1)) == righe, "la linguetta dice «%s», le righe da sistemare e da decidere sono %d" % (ling, righe))


@prova("c", "Bug 2: la linguetta diceva «proposte da decidere» senza un gesto; ora «Rivedi e conferma» le decide e il Congela non le conta piu'")
def prova_c_minuscola(b):
    b.apri("documenti")
    gate = " ".join(b.page.locator(".dst-box", has=b.page.locator(".dst-label", has_text="Congela la distinta")).inner_text().split())
    b.apri("distinta")
    ling = " ".join(b.page.locator(".dst-passo", has_text="Distinta").inner_text().split())
    verifica("da confermare" in ling and b.page.locator("[data-azione=rivedi]").is_visible(),
             "la linguetta dice «%s» e «Rivedi e conferma» %s" % (ling, "si vede" if b.page.locator("[data-azione=rivedi]").is_visible() else "non si vede"))
    rispondi_ai_vicini(b)

    def attesa(d):
        solo(d, ["nodi", "archi", "job"], "conferma dell'albero cosi' com'e'")
    b.gesto("conferma l'albero cosi' com'e'", lambda: (b.htmx(lambda: bottone_conferma(b).click(), "/albero/conferma"), dialogo_chiuso(b), b.page.wait_for_timeout(800)),
            attesa, ricarica=False)
    b.apri("documenti")
    dopo = " ".join(b.page.locator(".dst-box", has=b.page.locator(".dst-label", has_text="Congela la distinta")).inner_text().split())
    verifica("decisioni strutturali aperte" not in dopo, "dopo la conferma dell'albero il Congela dice ancora «%s» (prima: «%s»)" % (dopo, gate))


@prova("d", "Bug 10: un pezzo tolto dalla distinta, nel passo 3, era «il prodotto»")
def prova_d_minuscola(b):
    b.apri("distinta")
    scegli(b, "7121002")
    b.page.locator("[data-azione=elimina]").click()
    aspetta_dialogo(b).locator("button", has_text="Sì, togli").click()
    dialogo_chiuso(b)
    rispondi_ai_vicini(b)

    def attesa(d):
        solo(d, ["componenti", "relazioni", "nodi", "archi", "job"], "togli e conferma")
        verifica(d["relazioni"] == {"+": [], "-": ["7120010>7121002x1"]}, "archi: %s" % d["relazioni"])
    b.gesto("togli 7121002 e conferma", lambda: (b.htmx(lambda: bottone_conferma(b).click(), "/albero/conferma"), dialogo_chiuso(b), b.page.wait_for_timeout(800)),
            attesa, ricarica=False)
    b.apri("documenti")
    blocco = b.page.locator(".dst-blocco", has=b.page.locator(".cod", has_text=re.compile("^7121002$")))
    testa = " ".join(blocco.locator(".dst-blocco-testa").inner_text().split()) if blocco.count() else ""
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
            pg.on("dialog", lambda d: (dialoghi.append((d.type, d.message)), d.accept()))
            pg.on("pageerror", lambda e: errori.append("%s pageerror: %s" % (nome, e)))
            pg.on("console", lambda m: errori.append("%s console: %s" % (nome, m.text)) if m.type == "error" and "Failed to load resource" not in m.text else None)
            # un 422 della conferma rifiutata (due schede) e' una risposta attesa, non un errore della pagina
            pg.on("response", lambda r: errori.append("%s risposta %d: %s %s" % (nome, r.status, r.request.method, r.url))
                  if r.status >= 400 and not r.url.endswith("/favicon.ico") and not (r.status == 422 and "/albero/" in r.url) else None)

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
                # una finestra o un menu rimasti aperti dalla prova fallita non fermano quella dopo
                try:
                    page.evaluate("() => { const d = document.querySelector('#dst-dialogo[open]'); if (d) d.close(); const m = document.getElementById('dst-menu'); if (m) m.hidden = true; }")
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
