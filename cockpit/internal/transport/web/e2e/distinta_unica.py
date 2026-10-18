# -*- coding: utf-8 -*-
"""Browser - giro 5, A1d, V1 (direttiva 23), passo 8 di PM23 par. 14, prima parte: la pagina della Distinta unica di V1
(web/templates/distinta_unica.html, web/static/distinta_unica.mjs e il ponte web/static/distinta_unica_ponte.mjs) in un
BROWSER VERO, sul banco finto di distinta_unica_banco_test.go.

Non lo si lancia a mano: lo avvia distinta_unica_browser_test.go (tag `integrazione browser`), con `python -I`. Il banco
e' un httptest.Server senza database: il guscio VERO della pagina e risposte FINTE delle rotte di V1, conformi agli
esempi dell'emendamento di V1 (EM par. 9 e par. 10.9). I dati sono di prova e dichiarati tali (D23 par. 7): le prove
dicono che la pagina e il ponte rispettano il contratto (matrice dei gesti MAT, stati visibili, errori), non che il
server lo fa. La sonda del banco (/__banco/...) dice gli identificativi, lo stato del banco e provoca gli esiti (409,
422, 5xx, il motore che non risponde); le risposte perse le provoca questo script con route.fetch() + route.abort().

Le prove (DU-Bnn), in sequenza sulla stessa RFQ (ognuna parte dallo stato lasciato dalla precedente):
  01 apertura: solo letture, passo Sorgente senza bozza, indirizzo riscritto, nessun R4 e nessuna scrittura; il testo
     corretto di «Prepara i file»
  02 Sorgente: cartelle, «Guarda» (S6), organigramma della proposta, pannello del nodo (proposta, riferimenti,
     provenienza; le schede senza identita'), zoom e pan senza richieste
  03 S7 «Associa lo STEP», poi «Conferma la fonte» e «Autorizza» (atti della lista chiusa, HX-Current-URL, presa d'atto
     mai spuntata da sola)
  04 «Usa per la BOM» (R2 crea): 409 ANTEPRIMA_SUPERATA chiude il dialogo e rilegge S6; poi la versione 1 dal server, il
     passo Revisione, nessun R4 all'apertura della bozza
  05 Elenco: «Accetta la proposta» (preassociazione) = R2 con vocabolario 2 e chiave_salvataggio; R4 in sottofondo; i
     gesti di V3 spenti con il motivo
  06 selezione multipla: «Seleziona tutti i segnalati», Ctrl+clic, «Non includere nella BOM...»: un salvataggio, un
     azione_id, le decisioni in fondo con segnale_visto
  07 Annulla e Ripeti: annulla{n} e rifai{n} al server; mai oltre la versione costruita dal server
  08 pannello del nodo e «Correggi» su un pezzo importato: decisione motivata, identita' decisa accanto alla proposta;
     «Salvataggio...» mentre R2 e' in volo
  09 R2 409 BOZZA_VERSIONE: «La bozza e' cambiata altrove», gesti spenti con il motivo; «Rileggi la bozza salvata» tiene
     da parte le modifiche non salvate, «Scartale...» le elenca prima di perderle
  10 R2 422: «Salvataggio rifiutato» con il motivo; «Rileggi», poi «Riapplica alla bozza riletta»
  11 R2 con la risposta persa: «riprovo», reinvio con la stessa chiave_salvataggio, «riproposto», nessun doppione
  12 wizard «Rivedi e conferma BOM»: R4 con gli avvisi del motore, fasi, non inclusi con gli effetti; avvisi del motore
     non calcolati = «Riprova» e presa d'atto
  13 R5 rifiutata: 409 BASE_SUPERATA (R1 riletto, «Fai il rebase»), 422 BLOCCHI con i rimedi e CONGELATA, 409 CHIAVE_RIUSATA
     (R6, R1, R7), 409 COPIA_IN_CORSO («Riprova» con la stessa chiave, modifiche libere), 500 CONTROLLO_FINALE
  14 R5 «esito da verificare» (500): R6 automatico, «non ancora registrata», «Riprova la conferma» con la stessa chiave;
     le letture di dopo non rispondono: «la pagina non si e' riletta: ↻ Rileggi»; i nodi non inclusi della conferma
  15 WZ-01: dalla BOM confermata senza modifiche il CTA e' spento; R5 con la risposta persa: R6 «in corso» poi «registrato»
  16 R5 mai arrivata e pagina ricaricata: la chiave resta nella memoria della scheda, R6 all'apertura, «Riprova la
     conferma...» manda la stessa chiave anche con le decisioni rilette nell'ordine del server (EM par. 3.10); EL-06 e
     «Fissa l'emittente»
  17 «Scollega da un padre» (togli_arco); con la conferma in sospeso R3 e' spento; abbandono esplicito del tentativo, R3
  18 proposta cambiata: striscia, «Vedi le differenze» (R1 ?confronto=1), «Aggiorna alla proposta di oggi...»
  19 «Cambia lo STEP della BOM...»: l'effetto da S6 prima del gesto, il motivo, cambia_sorgente
  20 «Importa dalla proposta...» (selettivo, nessun elemento preselezionato, identita' delle regole della partenza)
     e «Riparti dalla proposta...» (crea con abbandona): niente della bozza di prima passa alla nuova
  21 file: «Scarta...» con le dipendenze (D-6 blocca, D-7 scollega, D-14 unica copia), l'archivio con le voci, «Altri
     file» e «Riattiva», 503 NON_CONFIGURATO
  22 S2 con la risposta persa: «Esito da verificare», R6 con la stessa chiave, «registrato»
  23 S3 mai arrivata: R6 «non registrato», la pagina ricaricata ritrova il tentativo, «Riprova» dalla striscia con lo
     stesso corpo e la stessa chiave
  24 «+ Aggiungi file» (E1'): esito per file, «parziale», analisi in corso e poi letto, reinvio «gia' presente»
  25 gli atti del menu («Prepara i file» senza «auto», «Rifai l'analisi», «Riprova le copie») e «Revoca» con il motivo;
     R11 «Riallinea i documenti» con F8 in errore; «Apri» e' un link a S5, mai una fetch; un PDF nel visore pdf.js
  26 secondo prodotto: lo STEP che non propone una struttura per il prodotto non inventa niente
  27 D-18: il modello che non si calcola (200 con «errore»): striscia, niente tela; «Aggiorna» lo rifa'
  28 impaginazione a 1440, 1024 e 390 px senza scorrimento orizzontale (Sorgente, Organigramma, Elenco, wizard)
  29 la Sorgente dice dove va ogni file (SG-06: preassociato, proposto per un pezzo); «Assegna a un pezzo...» (FL-12) e
     «Togli la preassociazione»; «Scarica» (FL-03); senza bozza «Assegna» e' spento con il motivo
  30 l'Elenco: filtri e ricerca (EL-01), «Accetta le proposte valide (N)» senza preselezione (EL-03); «Confronto con la
     proposta» sulle schede (BZ-25)
  31 il pannello del nodo: la sezione «Documenti» (EL-08) e «⤴ Ricolloca i figli...» in un salvataggio (BZ-11)
  32 «Riepilogo RFQ» (TS-08), «STEP secondario disponibile» (SG-12) e «nessuna candidata» (SG-13)
  33 i casi di contratto condivisi (e2e/distinta_unica_casi.json): il vassoio «Da ricollocare» (PM23 T-16, la meta'
     della pagina) e le azioni del vocabolario 2 applicate allo snapshot, con le funzioni esportate dalla pagina,
     caricata in una scheda senza il guscio (l'avvio della pagina non parte senza #du)

  35 §4.3 righe 1 e 2: un pezzo aggiunto, un file preassociato «in attesa del codice»; «Non crearlo» lo manda nel vassoio;
     «Lascia senza associazione» (EL-07)
  36 §4.3 righe 3 e 4: «Escludi» un registrato, con la scelta senza preselezione per le associazioni registrate, i documenti
     confermati elencati e il file preassociato nel vassoio; «Ripristina» toglie esclusione e svincoli e riporta il file
  37 D1 nel wizard («Lascia assieme», «Diventa assieme»), «Cambia tipo» (§4.3 riga 8), Sposta, Collega e Scollega dai dialoghi
  38 PN-08: «Conferma l'identita' proposta» (rifiuto del P/N K4) e «Adotta come codice» del codice corretto a mano
  39 «Includi di nuovo»; una voce con un componente registrato non si include da qui (PE-10); proposte «non richieste»
  40 letture e forme di R2 che non riescono: R1 e S1 in 5xx, annullamento_non_ammesso, i 422 di cambia_sorgente, la rete
     caduta (stessa chiave_salvataggio), R3 409
  41 i 409 e i 422 dei gesti dei file (S3 e S4 ANTEPRIMA_SUPERATA, FILE_VERSIONE, COPIA_IN_CORSO, DIPENDENZE_ATTIVE,
     RISOLUZIONE_RICHIESTA, registrato_senza_esito, S7 ANTEPRIMA_SUPERATA e GIA_DOCUMENTO, NESSUN_FILE_CARICATO, 413),
     «Riattiva» di un archivio con le voci, gli atti in coda
  42 rebase con la base superata; BOM congelata: fascia e «Chiedi una revisione autorizzata...» spento
  43 pannello: linguette, documenti del nodo, azioni non in V1 con il motivo; «da ricodificare»; NI-06; linguette dei file;
     una sola candidata incerta (SG-02)
  44 401 durante un salvataggio: si resta, «Riprova ora» con la stessa chiave_salvataggio
  45 «Verifica ora» di un gesto dei file in sospeso non butta le modifiche della bozza in volo
  46 con una conferma in sospeso «Aggiorna alla proposta di oggi...», «Abbandona la bozza...» e i gesti sono spenti
  47 «Converti...», mono_pezzo, R5 da verificare con il wizard chiuso; WZ-01 con la partenza «registrato» (decisioni
     dell'ondata A, §5 n. 1 e PE-23): con conferme formato 3 «Parti dalla BOM registrata» non si offre, e una bozza
     «registrato» aperta altrove senza modifiche ha il CTA spento
  dall'ondata B (le decisioni dell'ondata A nella pagina, con le prove dei rilievi aperti):
  48 le modifiche tenute da parte ricordano prodotto e bozza: il prodotto non cambia senza una scelta, «Riapplica» su
     un'altra bozza chiede conferma, l'uscita dalla pagina chiede conferma (§5 n. 2)
  52 PE-23: una base formato 2 (la prima conferma formato 3 è ammessa senza modifiche), la conferma dalla fase 2 con la
     pagina che non si rilegge e senza errori JavaScript, «registrato» non offerto dopo, uguale_alla_base di R4 dopo un
     annullamento netto, NESSUNA_MODIFICA di R5
  54 P-6: il nodo del prodotto segnalato non si «Non include» (casella spenta, «Seleziona tutti i segnalati» lo salta)
  55 P-7 e PE-09: «Cambia tipo» di un registrato con la variante di R1 e cambia_tipo{componente_id, tipo_nuovo, firma}
  56 P-15 e P-18 nei dialoghi (codice, rev e descrizione; il codice che non si svuota; la rev della working che non si
     tocca); «Adotta come codice» con la proiezione e la rev proposta (PE-08)
  50 E1' lento che avanza: oltre i 120 s delle chiamate JSON, con l'avanzamento, il file entra (§5 n. 3)
  51 E1' fermo: nessun byte per il tempo d'inattività, il caricamento si ferma e lo dice (§5 n. 3)
  34 le richieste della pagina: solo guscio, statici, rotte del ponte e sei atti; nessun gesto legacy
  49 (per ultima, dopo la 34: esce dalla sessione) all'uscita («esci» del layout) le chiavi cockpit.du. della memoria
     della scheda si tolgono, le altre restano (richiesta R-3 della corsia)

Gli esiti che il banco non sa provocare li danno risposte finte di questo script (rispondi_json, page.route), dichiarate
tali come il banco. I clienti, i codici e i nomi dei file sono inventati (ACME, BETA, ACM-xxxx).
"""
import argparse
import json
import os
import re
import sys
import time
import urllib.parse

from playwright.sync_api import expect, sync_playwright

PROVE = []
T_BREVE = 6000
T_LUNGO = 20000
UUID = r"[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"


def prova(numero, nome):
    def deco(fn):
        PROVE.append((numero, nome, fn))
        return fn
    return deco


class Rotto(AssertionError):
    pass


def verifica(condizione, messaggio):
    if not condizione:
        raise Rotto(messaggio)


def sid(k):
    return re.sub(r"[^A-Za-z0-9_-]", "_", str(k))


class Banco:
    def __init__(self, page, a):
        self.page = page
        self.url = a.url.rstrip("/")
        self.foto_dir = a.foto
        self.dati = page.request.get(self.url + "/__banco/dati").json()
        self.thread = self.dati["thread"]
        self.p1 = self.dati["prodotto"]
        self.p2 = self.dati["prodotto2"]
        self.al = self.dati["allegati"]
        self.pr = self.dati["proposte"]
        self.richieste = []
        # il dialogo del browser «vuoi uscire?» (beforeunload) è atteso solo nella prova del 401 (DU-B44): lì si resta
        self.consenti_beforeunload = False
        self.beforeunload = 0
        page.on("request", self._richiesta)

    def _richiesta(self, req):
        u = urllib.parse.urlsplit(req.url)
        try:
            corpo = req.post_data
        except Exception:
            corpo = None
        self.richieste.append({"metodo": req.method, "path": u.path, "query": u.query, "headers": dict(req.headers), "corpo": corpo})

    # ---- la sonda del banco
    def stato(self):
        return self.page.request.get(self.url + "/__banco/stato").json()

    def modo(self, rotta, esito):
        r = self.page.request.post(self.url + "/__banco/modo", data=json.dumps({"rotta": rotta, "esito": esito}), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda non ha preso il modo %s=%s: %d" % (rotta, esito, r.status))

    def proposta_cambia(self):
        r = self.page.request.post(self.url + "/__banco/proposta")
        verifica(r.status == 204, "la sonda non ha cambiato la proposta: %d" % r.status)

    def ritardo(self, rotta, ms):
        r = self.page.request.post(self.url + "/__banco/ritardo", data=json.dumps({"rotta": rotta, "ms": ms}), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda non ha preso il ritardo: %d" % r.status)

    def f8_errore(self):
        r = self.page.request.post(self.url + "/__banco/f8-errore")
        verifica(r.status == 204, "la sonda non ha messo F8 in errore: %d" % r.status)

    def interruttore(self, nome, attivo):
        r = self.page.request.post(self.url + "/__banco/interruttore", data=json.dumps({"nome": nome, "attivo": attivo}), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda non ha preso l'interruttore %s=%s: %d" % (nome, attivo, r.status))

    def bozza_formato2(self):
        r = self.page.request.post(self.url + "/__banco/bozza-formato2")
        verifica(r.status == 204, "la sonda non ha creato la bozza del formato di prima: %d" % r.status)

    def sonda_post(self, percorso):
        """Un gesto della sonda dell'ondata B (conferma-formato2, bozza-registrato, sostituisci-bozza): 204 atteso."""
        r = self.page.request.post(self.url + "/__banco/" + percorso)
        verifica(r.status == 204, "la sonda %s ha risposto %d" % (percorso, r.status))

    def proposta_di(self, nome_file):
        """La proposta di un file, anche caricato durante le prove (la sonda la rilegge)."""
        return self.page.request.get(self.url + "/__banco/dati").json()["proposte"][nome_file]

    def allegato_di(self, nome_file):
        return self.page.request.get(self.url + "/__banco/dati").json()["allegati"][nome_file]

    def scritture(self):
        c = self.stato()["conti"]
        return {k: v for k, v in c.items() if k in ("R2", "R3", "R4", "R5", "R11", "E1", "S2", "S3", "S4", "S7", "atti") and v}

    # ---- le richieste viste dal browser
    def segna(self):
        return len(self.richieste)

    def richieste_da(self, i, metodo=None, path_re=None):
        out = []
        for r in self.richieste[i:]:
            if metodo and r["metodo"] != metodo:
                continue
            if path_re and not re.search(path_re, r["path"]):
                continue
            out.append(r)
        return out

    def corpi(self, i, metodo, path_re):
        return [json.loads(r["corpo"]) for r in self.richieste_da(i, metodo, path_re)]

    def attendi(self, condizione, messaggio, ms=T_LUNGO):
        fine = time.time() + ms / 1000.0
        while time.time() < fine:
            if condizione():
                return
            self.page.wait_for_timeout(100)
        raise Rotto(messaggio)

    # ---- la pagina
    def pagina(self):
        return self.url + "/thread/" + self.thread + "/distinta-unica"

    def apri(self):
        self.page.goto(self.pagina())
        self.pronta()

    def pronta(self):
        expect(self.page.locator("#du-passo-sorgente")).to_be_visible(timeout=T_LUNGO)
        expect(self.page.locator("#du-corpo")).not_to_have_attribute("aria-busy", "true", timeout=T_LUNGO)

    def indirizzo(self):
        return self.page.evaluate("() => Object.fromEntries(new URLSearchParams(location.search))")

    def avviso(self):
        return self.page.locator("#du-avviso")

    def finestra(self):
        return self.page.locator("#du-finestra")

    def ok_finestra(self):
        b = self.page.locator("#du-finestra #f-ok")
        expect(b).to_be_enabled(timeout=T_BREVE)
        b.click()

    def chiudi_finestra(self):
        self.page.keyboard.press("Escape")
        expect(self.finestra()).to_be_hidden(timeout=T_BREVE)

    def salvata(self, versione):
        s = self.page.locator("#du-salvataggio")
        expect(s).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        expect(s).to_contain_text(re.compile(r"versione %d(\D|$)" % versione), timeout=T_LUNGO)

    def passo(self, nome):
        self.page.click("#du-passo-" + nome)
        expect(self.page.locator("#du-passo-" + nome)).to_have_attribute("aria-current", "step", timeout=T_BREVE)

    def vista(self, nome):
        self.page.click("#du-vista-" + nome)
        expect(self.page.locator("#du-vista-" + nome)).to_have_attribute("aria-pressed", "true", timeout=T_BREVE)

    def scheda(self, k, **kw):
        self.page.click("#du-zoom-adatta")
        self.page.locator("#nc-" + sid(k)).click(**kw)

    def linguetta(self, nome):
        """La linguetta del pannello del nodo: «dettaglio» o «documenti» (MAT PN-02)."""
        self.page.click("#du-ling-" + nome)
        expect(self.page.locator("#du-ling-" + nome)).to_have_attribute("aria-selected", "true", timeout=T_BREVE)

    def ultime_azioni(self, i):
        """Le azioni dell'ultimo salvataggio R2 dopo il segno i (aspetta che arrivi)."""
        self.attendi(lambda: self.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio della bozza")
        return self.corpi(i, "POST", r"/motore-a/bozza$")[-1]["azioni"]

    def sospesi(self):
        return self.page.evaluate("() => Object.keys(sessionStorage).filter(k => k.indexOf('cockpit.du.sospeso.') === 0)"
                                  ".map(k => [k, JSON.parse(sessionStorage.getItem(k))])")

    def striscia_sospesa(self):
        return self.page.locator("#du-sospeso-bozza_" + sid(self.p1))

    def apri_wizard(self):
        self.page.click("#du-cta-bom")
        expect(self.page.locator("#du-wizard")).to_be_visible(timeout=T_BREVE)
        expect(self.page.locator("#du-wz-identita")).to_be_visible(timeout=T_LUNGO)

    def esito_wizard(self):
        return self.page.locator("#du-wz-esito")

    def foto(self, nome):
        if not self.foto_dir:
            return
        try:
            os.makedirs(self.foto_dir, exist_ok=True)
            self.page.screenshot(path=os.path.join(self.foto_dir, nome))
        except Exception:
            pass


def perdi_la_risposta(n_volte=1):
    """Un gestore di route: la richiesta POST arriva al banco (route.fetch), la risposta si perde (abort)."""
    stato = {"n": 0}

    def gestore(route):
        if route.request.method == "POST" and stato["n"] < n_volte:
            stato["n"] += 1
            route.fetch()
            route.abort("connectionreset")
        else:
            route.continue_()
    return gestore


def blocca_la_richiesta(catturate):
    """Un gestore di route: la richiesta POST non arriva al banco (rete caduta prima)."""
    def gestore(route):
        if route.request.method == "POST":
            catturate.append(route.request.post_data_json)
            route.abort("internetdisconnected")
        else:
            route.continue_()
    return gestore


def rispondi_json(stato, corpo, metodo="POST", volte=1, cattura=None, intestazioni=None):
    """Un gestore di route: le prime `volte` richieste con quel metodo hanno questa risposta (un esito del contratto che il
    banco non sa provocare: 409, 422, 401, 413, 5xx); le altre vanno al banco. Le risposte sono finte e dichiarate tali."""
    stato_g = {"n": 0}

    def gestore(route):
        if route.request.method == metodo and stato_g["n"] < volte:
            stato_g["n"] += 1
            if cattura is not None:
                try:
                    cattura.append(route.request.post_data_json)
                except Exception:
                    cattura.append(None)
            h = {"Content-Type": "application/json"}
            h.update(intestazioni or {})
            route.fulfill(status=stato, headers=h, body=json.dumps(corpo))
        else:
            route.continue_()
    return gestore


def interrompi(metodo="GET"):
    """Un gestore di route: le richieste con quel metodo non arrivano (rete caduta); le altre passano."""
    def gestore(route):
        if route.request.method == metodo:
            route.abort("internetdisconnected")
        else:
            route.continue_()
    return gestore


# ---------------------------------------------------------------- le prove

@prova("01", "apertura: solo letture, passo Sorgente, indirizzo riscritto, nessun R4")
def b01(b):
    p = b.page
    i = b.segna()
    b.apri()
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    b.attendi(lambda: b.indirizzo().get("prodotto") == b.p1, "l'indirizzo non porta il prodotto: %s" % b.indirizzo())
    q = b.indirizzo()
    verifica(q.get("passo") == "sorgente" and q.get("vista") == "org", "indirizzo: %s" % q)
    non_get = [(r["metodo"], r["path"]) for r in b.richieste[i:] if r["metodo"] != "GET"]
    verifica(not non_get, "richieste non GET all'apertura: %s" % non_get)
    lette = {r["path"] for r in b.richieste[i:]}
    for atteso in ("/motore-a/dati", "/sorgente", "/motore-a/bozza", "/motore-a/conferme"):
        verifica(("/thread/%s%s" % (b.thread, atteso)) in lette, "manca la lettura %s all'apertura" % atteso)
    verifica(b.scritture() == {}, "scritture (o R4) all'apertura: %s" % b.scritture())
    expect(p.locator("#du-passo-revisione")).to_contain_text("nessuna bozza")
    expect(p.locator("#du-barra")).to_contain_text("Sorgente: la BOM non si modifica qui.")
    verifica(p.locator("#du-cta-bom").count() == 0, "il CTA della conferma c'è nella Sorgente")
    p.click("#du-menu-rfq summary")
    expect(p.locator("#du-menu-rfq")).to_contain_text("Non rilegge gli STEP già analizzati")
    p.click("#du-menu-rfq summary")


@prova("02", "Sorgente: cartelle, «Guarda» (S6), proposta, pannello del nodo, zoom e pan")
def b02(b):
    p = b.page
    expect(p.locator("#du-cartella-attivi")).to_have_text("Tutti (15)")
    expect(p.locator("#du-cartella-altri")).to_have_text("Altri file (0)")
    expect(p.locator("#du-cartella-scartati")).to_have_text("Scartati (0)")
    a1 = b.al["ACM-1000_assieme.stp"]
    # SG-01: per ogni STEP le radici candidate, la compatibilità, lo stato della struttura (LS-01) e i nodi segnalati
    expect(p.locator("#du-step-" + sid(a1))).to_contain_text("candidata ACM-1000 · compatibile · struttura completa · segnalati: 3")
    expect(p.locator("#du-step-" + sid(b.al["ACM-1000_rev2.stp"]))).to_contain_text("candidata ACM-1000-R2")
    p.click("#du-guarda-" + sid(a1))
    expect(p.locator("#du-anteprima")).to_contain_text("in vista, non confermata", timeout=T_LUNGO)
    expect(p.locator("#du-anteprima .du-vp .du-nc[data-k]")).to_have_count(10)
    testo_schede = p.locator("#du-anteprima .du-vp").inner_text()
    verifica("Identità proposta" not in testo_schede and "partner:beta" not in testo_schede and "Provenienza" not in testo_schede,
             "le schede portano i dettagli del pannello")
    expect(p.locator("#nc-k-9")).to_contain_text("fornitore")
    expect(p.locator("#nc-k-31")).to_contain_text("possibile elemento CAD")
    b.scheda("k-9")
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-9")
    expect(p.locator("#du-sez-proposta")).to_contain_text("400123")
    expect(p.locator("#du-sez-proposta")).to_contain_text("partner:beta")
    expect(p.locator("#du-sez-riferimenti")).to_contain_text("documentale")
    expect(p.locator("#du-sez-riferimenti")).to_contain_text("documentato_da")
    expect(p.locator("#du-sez-provenienza")).to_contain_text("ACM-1000_assieme.stp")
    verifica(p.locator("#du-sez-decisione").count() == 0, "nella Sorgente il pannello ha la decisione dell'ingegnere")
    verifica(b.indirizzo().get("nodo") == "k-9", "il nodo scelto non è nell'indirizzo: %s" % b.indirizzo())
    b.foto("B02_sorgente_proposta.png")
    # zoom e pan: solo presentazione, nessuna richiesta
    i = b.segna()
    vp = p.locator("#du-anteprima #du-vp")
    s0 = int(vp.get_attribute("data-scala"))
    p.click("#du-zoom-piu")
    s1 = int(vp.get_attribute("data-scala"))
    verifica(s1 > s0, "lo zoom non cresce: %d -> %d" % (s0, s1))
    mondo = p.locator("#du-anteprima #du-vp-mondo")
    t0 = mondo.evaluate("e => e.style.transform")
    vp.focus()
    p.keyboard.press("ArrowLeft")
    t1 = mondo.evaluate("e => e.style.transform")
    verifica(t1 != t0, "la freccia non sposta la vista")
    box = vp.bounding_box()
    p.mouse.move(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
    p.mouse.wheel(0, 120)
    b.attendi(lambda: mondo.evaluate("e => e.style.transform") != t1, "la rotella non sposta la vista", 3000)
    vp.focus()
    p.keyboard.press("0")
    verifica(not b.richieste_da(i), "zoom e pan hanno chiamato il server: %s" % [(r["metodo"], r["path"]) for r in b.richieste_da(i)])
    verifica(b.scritture() == {}, "scritture nella Sorgente: %s" % b.scritture())


@prova("03", "S7 «Associa lo STEP», «Conferma la fonte», «Autorizza» (atti con HX-Current-URL)")
def b03(b):
    p = b.page
    expect(p.locator("#du-conferma-fonte")).to_be_disabled()
    expect(p.locator("#du-fonte")).to_contain_text("lo STEP non è associato al prodotto")
    i = b.segna()
    p.click("#du-associa-step")
    expect(b.finestra()).to_contain_text("Associa lo STEP a ACM-1000")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("STEP associato a ACM-1000", timeout=T_LUNGO)
    s7 = b.corpi(i, "POST", r"/sorgente/step/[^/]+/associa$")
    verifica(len(s7) == 1, "S7: %d richieste" % len(s7))
    c = s7[0]
    verifica(re.fullmatch(UUID, c.get("chiave_idempotenza", "")) and c.get("prodotto_id") == b.p1 and c.get("codice_atteso") == "ACM-1000", "corpo di S7: %s" % c)
    verifica(not [k for k in c if re.search("percorso|path|staging", k, re.I)], "S7 manda un campo percorso: %s" % c)
    expect(p.locator("#du-anteprima")).to_contain_text("STEP associato a ACM-1000", timeout=T_LUNGO)
    verifica(not b.sospesi(), "S7 riuscito ha lasciato una chiave in sospeso")
    # Conferma la fonte: una voce, l'atto con le intestazioni della pagina di motore-a
    expect(p.locator("#du-conferma-fonte")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-fonte")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Fonte confermata", timeout=T_LUNGO)
    atti = b.richieste_da(i, "POST", r"/fascicolo/conferma$")
    verifica(len(atti) == 1, "«Conferma la fonte»: %d richieste" % len(atti))
    h = atti[0]["headers"]
    verifica(h.get("hx-request") == "true" and h.get("hx-current-url", "").endswith("/thread/%s/motore-a" % b.thread), "intestazioni dell'atto: %s" % h)
    voce = b.stato()["fonte"]["voce"]
    verifica(urllib.parse.parse_qs(atti[0]["corpo"] or "") == {"voce": [voce]}, "campi di «Conferma la fonte»: %s" % atti[0]["corpo"])
    expect(p.locator("#du-passo-sorgente")).to_contain_text("fonte confermata")
    # Autorizza: l'anteprima, la presa d'atto spuntata da chi guarda, la firma letta
    p.click("#du-autorizza")
    expect(b.finestra()).to_contain_text("diventa lo STEP strutturale di ACM-1000", timeout=T_LUNGO)
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    verifica(not p.locator("#f-presa").is_checked(), "la presa d'atto è già spuntata")
    p.locator("#f-presa").check()
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("STEP strutturale autorizzato", timeout=T_LUNGO)
    aut = b.richieste_da(i, "POST", r"/fascicolo/componente/[^/]+/step-strutturale$")
    verifica(len(aut) == 1 and ("/componente/%s/" % b.p1) in aut[0]["path"], "«Autorizza»: %s" % [r["path"] for r in aut])
    f = urllib.parse.parse_qs(aut[0]["corpo"] or "")
    verifica(f.get("presa_d_atto") == ["1"] and f.get("firma") == ["f" * 64], "campi di «Autorizza»: %s" % f)
    expect(p.locator("#du-passo-sorgente")).to_contain_text("STEP autorizzato")
    expect(p.locator("#du-revoca")).to_be_visible()


@prova("04", "«Usa per la BOM»: 409 ANTEPRIMA_SUPERATA chiude il dialogo, poi la bozza v1 dal server")
def b04(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    b.proposta_cambia()
    i = b.segna()
    p.click("#du-usa-bom")
    expect(b.finestra()).to_contain_text("Usa per la BOM di ACM-1000")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("La proposta è cambiata mentre guardavi", timeout=T_LUNGO)
    expect(b.avviso()).to_have_attribute("data-esito", "conflitto")
    expect(b.finestra()).to_be_hidden()
    b.attendi(lambda: b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$"), "S6 non riletto dopo ANTEPRIMA_SUPERATA")
    expect(p.locator("#du-usa-bom")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-usa-bom")
    b.ok_finestra()
    expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10)
    crea = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(crea) == 2, "R2 crea: %d richieste" % len(crea))
    verifica(crea[0]["crea"]["impronta_contenuto"] != crea[1]["crea"]["impronta_contenuto"], "il secondo invio porta l'impronta vecchia")
    verifica(crea[0]["chiave_salvataggio"] != crea[1]["chiave_salvataggio"], "dopo un 409 la chiave_salvataggio non cambia")
    c = crea[1]
    verifica(c["bozza_id"] is None and c["versione_attesa"] == 0 and c["vocabolario"] == 2 and c["crea"]["partenza"] == "step"
             and c["crea"]["sorgente"]["allegato_id"] == a1 and c["crea"]["sorgente"]["gruppo"].startswith("cand:"), "corpo di R2 crea: %s" % c)
    verifica("snapshot" not in c and "azioni" not in c, "R2 crea manda lo snapshot o le azioni: la versione 1 la costruisce il server")
    verifica(b.stato()["conti"].get("R4", 0) == 0, "R4 all'apertura della bozza")
    verifica(b.indirizzo().get("passo") == "revisione", "indirizzo: %s" % b.indirizzo())


@prova("05", "Elenco: «Accetta la proposta» = R2 vocabolario 2; R4 in sottofondo; gesti V3 spenti")
def b05(b):
    p = b.page
    b.vista("elenco")
    expect(p.locator("#du-elenco [role=row][data-k]")).to_have_count(10)
    riga = p.locator("#el-k-7")
    expect(riga).to_contain_text("◇ proposto")
    expect(riga).to_contain_text("ACM-1100_A.pdf")
    expect(riga.locator("button", has_text="Conferma il documento")).to_be_disabled()
    verifica("V3" in (riga.locator("button", has_text="Conferma il documento").get_attribute("title") or ""), "«Conferma il documento» senza il motivo")
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica("Organigramma" in (p.locator("#du-cta-bom").get_attribute("title") or ""), "il CTA nell'Elenco senza il motivo")
    expect(p.locator("#du-cta-fascicolo")).to_be_disabled()
    i = b.segna()
    p.click("#du-accetta-" + sid(b.pr["ACM-1100_A.pdf"]))
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "modifiche")
    expect(p.locator("#du-salvataggio")).to_contain_text("1 modifiche non salvate")
    b.salvata(2)
    r2 = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(r2) == 1, "R2: %d richieste (il debounce le raccoglie)" % len(r2))
    c = r2[0]
    verifica(c["vocabolario"] == 2 and c["versione_attesa"] == 1 and re.fullmatch(UUID, c["chiave_salvataggio"]), "testa di R2: %s" % {k: c[k] for k in c if k not in ("snapshot", "azioni")})
    verifica([a["azione"] for a in c["azioni"]] == ["file_preassociato"], "azioni: %s" % c["azioni"])
    verifica(c["snapshot"]["documenti_preassociati"] == [{"proposta_id": b.pr["ACM-1100_A.pdf"], "chiave_bozza": "k-7"}], "snapshot: %s" % c["snapshot"]["documenti_preassociati"])
    verifica(not [k for k in ("bozza_id", "bozza_versione", "chiave_idempotenza") if k in c["snapshot"]], "lo snapshot porta campi della richiesta")
    expect(p.locator("#du-conti")).to_contain_text("blocchi", timeout=T_LUNGO)
    r4 = b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$")
    verifica(len(r4) == 1 and "avvisi_motore" not in r4[0]["query"], "R4 in sottofondo: %s" % [r["query"] for r in r4])
    expect(p.locator("#el-k-7")).to_contain_text("◈ preassociato")
    expect(p.locator("#el-k-7")).to_contain_text("applicabile")
    b.foto("B05_revisione_elenco.png")


@prova("06", "selezione multipla e «Non includere nella BOM…»: un salvataggio, un azione_id")
def b06(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    b.vista("org")
    p.click("#du-segnalati-apri")
    expect(p.locator("#du-segnalati")).to_contain_text("Nodi segnalati (3)")
    # il cassetto dice la regola, la spiegazione e i contro-indizi del segnale, con l'etichetta (richiesta R-1 della corsia motore)
    expect(p.locator('#du-segnalati .du-segnalato[data-k="k-33"]')).to_contain_text("elemento_cad_nome_foglia_v1")
    expect(p.locator('#du-segnalati .du-segnalato[data-k="k-33"]')).to_contain_text("il grafo del file non è completo: la foglia potrebbe avere figli non letti (grafo_non_completo)")
    verifica("contro-indizi" not in p.locator('#du-segnalati .du-segnalato[data-k="k-31"]').inner_text(), "contro-indizi su un segnale che non ne ha")
    p.click("#du-seleziona-segnalati")
    expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
    b.scheda("k-12", modifiers=["Control"])
    expect(p.locator("#du-selezione")).to_contain_text("4 nodi selezionati")
    expect(p.locator("#nc-k-12")).to_have_class(re.compile(r"du-nc--selezionato"))
    p.locator("#sel-k-12").click()
    expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
    verifica(b.scritture().get("R2") == 2, "la selezione ha scritto qualcosa: %s" % b.scritture())
    i = b.segna()
    p.click("#du-non-includere")
    expect(p.locator("#f-elenco input:checked")).to_have_count(3)
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "cordoni di saldatura del CAD")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="non incluso (bozza)"]')).to_have_count(3)
    b.salvata(3)
    r2 = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(r2) == 1, "«Non includere»: %d salvataggi" % len(r2))
    az = r2[0]["azioni"]
    tipi = [a["azione"] for a in az]
    verifica(len(az) == 9 and tipi[-3:] == ["decisione_aggiunta"] * 3 and tipi[:6].count("arco_tolto") == 3 and tipi[:6].count("nodo_tolto") == 3,
             "ordine delle azioni: %s" % tipi)
    ids = {(a.get("dopo") or {}).get("azione_id") for a in az}
    verifica(len(ids) == 1 and re.fullmatch(UUID, list(ids)[0] or ""), "azione_id del gesto: %s" % ids)
    for a in az[-3:]:
        d = a["dopo"]
        verifica(d["tipo"] == "non_includere" and d["motivo_codice"] == "elemento_cad" and d["motivo"] and d["casella"]["allegato_id"] == a1
                 and d["casella"]["gruppo"].startswith("cand:") and d["casella"]["rif"].startswith("nodo:"), "decisione: %s" % d)
        verifica(d["segnale_visto"] == {"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"},
                 "segnale_visto: %s" % d.get("segnale_visto"))
    b.foto("B06_revisione_organigramma.png")


@prova("07", "Annulla e Ripeti: annulla{n} e rifai{n}; mai oltre la versione del server")
def b07(b):
    p = b.page
    noninc = p.locator('#du-vp .du-nc[data-du-stato="non incluso (bozza)"]')
    i = b.segna()
    p.click("#du-annulla")
    expect(b.avviso()).to_contain_text("Annullato: non inclusi 3 nodi")
    expect(noninc).to_have_count(0)
    b.salvata(4)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([a["azione"] for a in az] == ["annulla"] * 9 and [a["dopo"]["n"] for a in az] == list(range(12, 3, -1)), "annulla: %s" % az)
    # l'annullamento del gesto cumulativo porta il suo azione_id nel «prima» (EM §7.1)
    prime = [a["prima"] for a in az]
    verifica(all(isinstance(x, dict) and set(x) == {"azione_id"} for x in prime) and len({x["azione_id"] for x in prime}) == 1
             and re.fullmatch(UUID, prime[0]["azione_id"]), "«prima» dell'annullamento: %s" % prime)
    i = b.segna()
    p.click("#du-ripeti")
    expect(noninc).to_have_count(3)
    b.salvata(5)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([a["azione"] for a in az] == ["rifai"] * 9 and [a["dopo"]["n"] for a in az] == list(range(4, 13)), "rifai: %s" % az)
    # due annulli: la pila si ferma alla versione 1, costruita dal server
    p.click("#du-annulla")
    b.salvata(6)
    i = b.segna()
    p.click("#du-annulla")
    b.salvata(7)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["prima"], a["dopo"]) for a in az] == [("annulla", None, {"n": 3})], "annulla della preassociazione: %s" % az)
    expect(p.locator("#du-annulla")).to_be_disabled()
    verifica("niente da annullare" in (p.locator("#du-annulla").get_attribute("title") or ""), "Annulla spento senza il motivo")
    p.click("#du-ripeti")
    b.salvata(8)
    p.click("#du-ripeti")
    b.salvata(9)
    expect(noninc).to_have_count(3)
    expect(p.locator("#du-ripeti")).to_be_disabled()


@prova("08", "pannello del nodo e «Correggi» su un pezzo importato: decisione motivata")
def b08(b):
    p = b.page
    b.scheda("k-12")
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-12")
    expect(p.locator("#du-sez-proposta")).to_contain_text("nessuna lettura")
    expect(p.locator("#du-sez-decisione")).to_contain_text("da_decidere")
    i = b.segna()
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1130")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "codice dal disegno del cliente")
    # il server risponde dopo un secondo e mezzo: la barra dice «Salvataggio…» mentre R2 è in volo
    b.ritardo("POST /thread/{id}/motore-a/bozza", 1500)
    b.ok_finestra()
    expect(p.locator("#nc-k-12")).to_contain_text("ACM-1130")
    expect(p.locator("#nc-k-12")).to_have_attribute("data-du-stato", "decisa in bozza")
    s = p.locator("#du-salvataggio")
    expect(s).to_have_attribute("data-du-salvataggio", "in_corso", timeout=T_BREVE)
    expect(s).to_have_text("Salvataggio…")
    b.salvata(10)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    dec = [a for a in az if a["azione"] == "decisione_aggiunta"][0]["dopo"]
    verifica(dec["tipo"] == "identita" and dec["modo"] == "correzione" and dec["identita"]["codice"] == "ACM-1130" and dec["motivo"], "decisione: %s" % dec)
    mod = [a for a in az if a["azione"] == "nodo_modificato"][0]
    verifica(mod["dopo"]["identita"]["modo"] == "decisione" and mod["dopo"]["identita"]["decisione"] == dec["id"], "nodo_modificato: %s" % mod)
    expect(p.locator("#du-sez-decisione")).to_contain_text("ACM-1130")
    expect(p.locator("#du-sez-proposta")).to_contain_text("nessuna lettura")


@prova("09", "R2 409 BOZZA_VERSIONE: conflitto, gesti spenti; «Rileggi» tiene da parte le modifiche, «Scartale…» le elenca")
def b09(b):
    p = b.page
    n_r2 = b.stato()["conti"]["R2"]
    b.modo("R2", "409")
    b.scheda("k-41")
    p.click("#du-quantita")
    p.fill("#f-qta", "3")
    b.ok_finestra()
    s = p.locator("#du-salvataggio")
    expect(s).to_have_attribute("data-du-salvataggio", "conflitto", timeout=T_LUNGO)
    expect(s).to_have_text("La bozza è cambiata altrove")
    expect(p.locator("#du-conflitto-testo")).to_contain_text("quantità di ACM-1410")
    expect(p.locator("#du-conflitto-testo")).to_contain_text("Non si perdono")
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    verifica("cambiata altrove" in (p.locator("#du-nuovo-ass").get_attribute("title") or ""), "gesto spento senza il motivo")
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica(b.stato()["conti"]["R2"] == n_r2, "un 409 ha scritto la bozza")
    # «Rileggi la bozza salvata»: le modifiche non salvate restano da parte (MAT §5.1), niente si perde senza un gesto
    p.click("#du-rileggi-bozza")
    da_parte = p.locator("#du-da-parte")
    expect(da_parte).to_contain_text("quantità di ACM-1410", timeout=T_LUNGO)
    expect(p.locator("#oq-k-40_k-41")).to_contain_text("×2")
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    verifica("tenute da parte" in (p.locator("#du-nuovo-ass").get_attribute("title") or ""), "gesto spento senza il motivo delle modifiche da parte")
    p.click("#du-scarta-da-parte")
    expect(p.locator("#f-da-parte")).to_contain_text("quantità di ACM-1410")
    b.ok_finestra()
    expect(da_parte).to_have_count(0)
    b.salvata(10)
    expect(p.locator("#oq-k-40_k-41")).to_contain_text("×2")
    expect(p.locator("#du-nuovo-ass")).to_be_enabled()
    verifica(b.stato()["conti"]["R2"] == n_r2, "lo scarto delle modifiche da parte ha scritto la bozza")


@prova("10", "R2 422: «Salvataggio rifiutato» con il motivo; «Rileggi», poi «Riapplica alla bozza riletta»")
def b10(b):
    p = b.page
    b.modo("R2", "422")
    b.scheda("k-41")
    p.click("#du-quantita")
    p.fill("#f-qta", "3")
    b.ok_finestra()
    s = p.locator("#du-salvataggio")
    expect(s).to_have_attribute("data-du-salvataggio", "rifiutata", timeout=T_LUNGO)
    expect(s).to_contain_text("azioni_non_riconciliate")
    expect(p.locator("#du-rileggi-bozza")).to_be_visible()
    p.click("#du-rileggi-bozza")
    expect(p.locator("#du-da-parte")).to_contain_text("quantità di ACM-1410", timeout=T_LUNGO)
    expect(p.locator("#oq-k-40_k-41")).to_contain_text("×2")
    i = b.segna()
    p.click("#du-riapplica")
    b.salvata(11)
    expect(p.locator("#oq-k-40_k-41")).to_contain_text("×3")
    expect(p.locator("#du-da-parte")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(c) == 1 and c[0]["versione_attesa"] == 10 and [a["azione"] for a in c[0]["azioni"]] == ["qta_modificata"],
             "la riapplicazione delle modifiche da parte: %s" % [(x["versione_attesa"], [a["azione"] for a in x["azioni"]]) for x in c])


@prova("11", "R2 con la risposta persa: stessa chiave_salvataggio, «riproposto», nessun doppione")
def b11(b):
    p = b.page
    n_r2 = b.stato()["conti"]["R2"]
    g = perdi_la_risposta()
    p.route("**/motore-a/bozza", g)
    try:
        i = b.segna()
        b.scheda("k-41")
        p.click("#du-quantita")
        p.fill("#f-qta", "5")
        b.ok_finestra()
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "non_arrivato", timeout=T_LUNGO)
        expect(p.locator("#du-salvataggio")).to_contain_text("riprovo")
        b.salvata(12)
        expect(p.locator("#du-salvataggio")).to_contain_text("(riproposto)")
        corpi = b.corpi(i, "POST", r"/motore-a/bozza$")
        verifica(len(corpi) == 2 and corpi[0] == corpi[1], "il reinvio non è lo stesso corpo con la stessa chiave: %d invii" % len(corpi))
        verifica(b.stato()["conti"]["R2"] == n_r2 + 1, "la bozza è stata scritta due volte")
        expect(p.locator("#oq-k-40_k-41")).to_contain_text("×5")
    finally:
        p.unroute("**/motore-a/bozza", g)


@prova("12", "wizard: R4 con gli avvisi del motore, fasi, non inclusi; avvisi non calcolati → presa d'atto")
def b12(b):
    p = b.page
    i = b.segna()
    b.apri_wizard()
    r4 = b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$")
    verifica(r4 and "avvisi_motore=1" in r4[-1]["query"], "R4 del wizard senza gli avvisi del motore: %s" % [r["query"] for r in r4])
    p.click("#du-wz-g-noninc")
    expect(p.locator("#du-wz-noninclusi tbody tr")).to_have_count(3)
    expect(p.locator("#du-wz-noninclusi")).to_contain_text("Saldatura")
    expect(p.locator("#du-wz-noninclusi")).to_contain_text("2D previsti: 1 → 0 dopo la conferma")
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_contain_text("La BOM è confermabile.")
    expect(p.locator("#du-wz-avvisi")).to_contain_text("VERIFICA_DOPO_COMMIT")
    expect(p.locator("#du-wz-motore")).to_contain_text("calcolati adesso e non firmati")
    expect(p.locator("#du-wz-criterio")).to_contain_text("nessuno")
    expect(p.locator("#du-wz-conferma")).to_be_enabled()
    b.foto("B12_wizard_fase2.png")
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()
    b.modo("motore", "ko")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-motore")).to_contain_text("Avvisi del motore non calcolati (CALCOLO_OCCUPATO")
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    verifica("presa d'atto" in (p.locator("#du-wz-conferma").get_attribute("title") or ""), "conferma spenta senza il motivo")
    p.locator("#du-wz-presa").click()
    expect(p.locator("#du-wz-conferma")).to_be_enabled()
    p.click("#du-wz-riprova-motore")
    expect(p.locator("#du-wz-motore")).to_contain_text("calcolati adesso e non firmati", timeout=T_LUNGO)


@prova("13", "R5 rifiutata: 409 BASE_SUPERATA, 422 BLOCCHI e CONGELATA, 409 CHIAVE_RIUSATA e COPIA_IN_CORSO, 500 CONTROLLO_FINALE")
def b13(b):
    p = b.page
    chiavi0 = len(b.stato()["chiavi_r5"])
    e = b.esito_wizard()
    b.modo("R5", "409_base")
    i = b.segna()
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-codice", "BASE_SUPERATA", timeout=T_LUNGO)
    expect(e).to_contain_text("fai il rebase")
    # la base superata si rilegge da R1, che offre il rebase (MAT §5.2)
    b.attendi(lambda: b.richieste_da(i, "GET", r"/motore-a/bozza$"), "R1 non riletto dopo BASE_SUPERATA")
    expect(p.locator("#du-wz-rebase")).to_be_visible()
    verifica(not b.sospesi(), "BASE_SUPERATA ha lasciato la chiave in sospeso")
    b.modo("R5", "422")
    expect(p.locator("#du-wz-conferma")).to_be_enabled()
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-codice", "BLOCCHI", timeout=T_LUNGO)
    expect(e).to_contain_text("D1_ULTIMO_FIGLIO")
    expect(e).to_contain_text("Rimedi: Diventa particolare; Lascia assieme")
    expect(e).to_contain_text("Niente è registrato, la bozza resta")
    verifica(not b.sospesi(), "BLOCCHI ha lasciato la chiave in sospeso")
    # 422 CONGELATA (risposta finta): il motivo, niente registrato, niente in sospeso
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "CONGELATA", "testo": "La BOM è congelata."})
    p.route("**/motore-a/bom/conferma", g)
    try:
        p.click("#du-wz-conferma")
        expect(e).to_have_attribute("data-codice", "CONGELATA", timeout=T_LUNGO)
        expect(e).to_contain_text("La BOM è congelata: niente è registrato.")
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    verifica(not b.sospesi(), "CONGELATA ha lasciato la chiave in sospeso")
    # 409 CHIAVE_RIUSATA (risposta finta): R6 con la chiave, poi R1 e R7 (MAT §5.2)
    g = rispondi_json(409, {"esito": "conflitto", "codice": "CHIAVE_RIUSATA", "testo": "Questo tentativo ha già un esito diverso."})
    p.route("**/motore-a/bom/conferma", g)
    try:
        i = b.segna()
        p.click("#du-wz-conferma")
        expect(e).to_have_attribute("data-codice", "CHIAVE_RIUSATA", timeout=T_LUNGO)
        b.attendi(lambda: b.richieste_da(i, "GET", r"/motore-a/intento/") and b.richieste_da(i, "GET", r"/motore-a/bozza$") and b.richieste_da(i, "GET", r"/motore-a/conferme$"),
                  "dopo CHIAVE_RIUSATA mancano R6, R1 o R7")
        expect(e).to_contain_text("Esito di quella chiave: non_registrato")
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    verifica(not b.sospesi(), "CHIAVE_RIUSATA ha lasciato la chiave in sospeso")
    # 409 COPIA_IN_CORSO (risposta finta): esito certo, «Riprova» con la stessa chiave; la striscia dice «copia in corso»,
    # non «non ancora registrato», e le modifiche restano libere (scelta dichiarata nel resoconto)
    catturate = []
    g = rispondi_json(409, {"esito": "conflitto", "codice": "COPIA_IN_CORSO", "testo": "Una copia sul NAS è in corso."}, volte=2, cattura=catturate)
    p.route("**/motore-a/bom/conferma", g)
    try:
        p.click("#du-wz-conferma")
        expect(e).to_have_attribute("data-codice", "COPIA_IN_CORSO", timeout=T_LUNGO)
        expect(e).to_contain_text("riprova fra poco con la stessa chiave")
        sosp = b.sospesi()
        verifica(len(sosp) == 1 and sosp[0][1].get("certo") is True, "COPIA_IN_CORSO: memoria della scheda %s" % sosp)
        p.click("#du-wz-riprova-copia")
        b.attendi(lambda: len(catturate) == 2, "«Riprova» dopo COPIA_IN_CORSO non è partito")
        expect(e).to_have_attribute("data-codice", "COPIA_IN_CORSO", timeout=T_LUNGO)
        verifica(catturate[0]["bozza"]["chiave_idempotenza"] == catturate[1]["bozza"]["chiave_idempotenza"], "«Riprova» dopo COPIA_IN_CORSO non riusa la chiave")
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    p.click("#du-wz-chiudi")
    striscia = b.striscia_sospesa()
    expect(striscia).to_contain_text("una copia sul NAS è in corso", timeout=T_LUNGO)
    verifica("non è ancora registrato" not in striscia.inner_text(), "la striscia di COPIA_IN_CORSO dice un esito incerto")
    expect(p.locator("#du-nuovo-ass")).to_be_enabled()
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    b.modo("R5", "controllo_finale")
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-esito", "controllo_finale", timeout=T_LUNGO)
    expect(e).to_contain_text("non riprovare, segnalalo")
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    verifica(p.locator("#du-wz-riprova").count() == 0, "«Riprova» dopo CONTROLLO_FINALE")
    verifica(not b.sospesi(), "CONTROLLO_FINALE ha lasciato la chiave in sospeso")
    st = b.stato()
    chiavi = st["chiavi_r5"][chiavi0:]
    verifica(len(chiavi) == 3 and len(set(chiavi)) == 3, "chiavi di R5: %s (una nuova per ogni esito certo)" % chiavi)
    verifica(st["conti"].get("R5", 0) == 0 and not st["conferme"], "una conferma registrata da un rifiuto")
    p.click("#du-wz-chiudi")


@prova("14", "R5 da verificare (500): R6, «non ancora registrata», «Riprova la conferma» con la stessa chiave")
def b14(b):
    p = b.page
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    b.modo("R5", "500_prima")
    i = b.segna()
    p.click("#du-wz-conferma")
    e = b.esito_wizard()
    expect(e).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
    expect(e).to_contain_text("Non so se la conferma è registrata")
    sosp = b.sospesi()
    verifica(len(sosp) == 1 and sosp[0][1]["nome"] == "confermaBom", "memoria della scheda: %s" % sosp)
    verifica(sosp[0][0] == "cockpit.du.sospeso.%s.%s.bozza:%s" % (b.dati["utente"], b.thread, b.p1), "chiave della memoria: %s" % sosp[0][0])
    k = sosp[0][1]["chiave"]
    expect(e).to_have_attribute("data-esito", "non_registrata", timeout=T_LUNGO)
    expect(e).to_contain_text("non è ancora registrata")
    r6 = b.richieste_da(i, "GET", r"/motore-a/intento/")
    verifica(r6 and all(r["path"].endswith("/" + k) for r in r6), "R6 senza la stessa chiave: %s" % [r["path"] for r in r6])
    verifica(b.sospesi() and b.sospesi()[0][1].get("verificato") is True, "dopo R6 non_registrato la chiave non è segnata verificata")
    # la conferma passa, ma le letture di dopo (modello, R1, R7) non rispondono: «confermata, pagina non riletta» con
    # «↻ Rileggi», mai «non riuscita» (MAT §5.2; §1.3 da_rileggere)
    letture = re.compile(r"/motore-a/(dati|bozza|conferme)(\?|$)")
    g = rispondi_json(500, {"esito": "errore", "testo": "errore interno"}, metodo="GET", volte=100)
    p.route(letture, g)
    try:
        p.click("#du-wz-riprova")
        expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
        expect(e).to_contain_text("Conferma n. 1 di ACM-1000")
        expect(p.locator("#du-wz-non-riletta")).to_contain_text("La conferma n. 1 è registrata; la pagina non si è riletta", timeout=T_LUNGO)
    finally:
        p.unroute(letture, g)
    p.click("#du-wz-rileggi")
    expect(p.locator("#du-wz-non-riletta")).to_have_count(0, timeout=T_LUNGO)
    expect(e).to_contain_text("Conferma n. 1 di ACM-1000")
    expect(e).to_contain_text("Verificata: non ancora calcolata (arriva in V2)")
    # i nodi non inclusi della conferma, con il nome (MAT NI-10)
    expect(p.locator("#du-wz-noninclusi-conferma")).to_contain_text("Nodi non inclusi nella conferma n. 1")
    expect(p.locator("#du-wz-noninclusi-conferma")).to_contain_text("Saldatura")
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 2 and r5[0]["bozza"]["chiave_idempotenza"] == k == r5[1]["bozza"]["chiave_idempotenza"], "«Riprova» non riusa la chiave")
    verifica(r5[0] == r5[1], "«Riprova» non manda lo stesso corpo")
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo la conferma")
    verifica(len(b.stato()["conferme"][b.p1]) == 1, "conferme registrate: %s" % b.stato()["conferme"])


@prova("15", "WZ-01: senza modifiche sulla BOM confermata il CTA è spento; R5 con la risposta persa: R6 «in corso» poi «registrato»")
def b15(b):
    p = b.page
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    p.click("#du-da-conferma")
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(2)
    # «v1 confermata il … da …» e «Verificata» restano due segni distinti; la verifica di una conferma di V1 non è calcolata
    expect(p.locator("#du-stato-conferma")).to_contain_text("v1 confermata")
    expect(p.locator("#du-stato-conferma")).to_contain_text(" da AD")
    expect(p.locator("#du-stato-conferma")).to_contain_text("Verificata: non ancora calcolata (arriva in V2)")
    # WZ-01: nessuna modifica su una versione confermata, il CTA è spento con il motivo (una conferma n+1 uguale alla n no)
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica("nessuna modifica: la v1 è già confermata" in (p.locator("#du-cta-bom").get_attribute("title") or ""), "il CTA senza modifiche non è spento con il motivo")
    # una modifica: la quantità di ACM-1400 sotto il prodotto
    b.scheda("k-40")
    p.click("#du-quantita")
    p.fill("#f-qta", "2")
    b.ok_finestra()
    b.salvata(2)
    expect(p.locator("#du-cta-bom")).to_be_enabled()
    b.apri_wizard()
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    b.modo("R6", "in_corso")
    g = perdi_la_risposta()
    p.route("**/motore-a/bom/conferma", g)
    try:
        i = b.segna()
        p.click("#du-wz-conferma")
        e = b.esito_wizard()
        expect(e).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
        expect(e).to_contain_text("ancora in corso", timeout=T_LUNGO)
        expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
        expect(e).to_contain_text("Conferma n. 2 di ACM-1000 (verificata con la stessa chiave)")
        r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
        verifica(len(r5) == 1, "la pagina ha rimandato la conferma: %d invii" % len(r5))
        k = r5[0]["bozza"]["chiave_idempotenza"]
        r6 = b.richieste_da(i, "GET", r"/motore-a/intento/")
        verifica(len(r6) >= 2 and all(r["path"].endswith("/" + k) for r in r6), "R6: %s" % [r["path"] for r in r6])
        st = b.stato()
        verifica(st["chiavi_r5"][-1] == k and len(st["conferme"][b.p1]) == 2, "conferme: %s" % st["conferme"])
        verifica(not b.sospesi(), "la chiave resta in sospeso dopo «registrato»")
    finally:
        p.unroute("**/motore-a/bom/conferma", g)


@prova("16", "R5 mai arrivata, ricarica: la chiave resta, R6 all'apertura; «Riprova la conferma…» riusa la chiave con gli elenchi nell'ordine del server")
def b16(b):
    p = b.page
    p.click("#du-wz-chiudi")
    p.click("#du-da-conferma")
    b.salvata(1)
    # EL-06: «Scollega dal componente…» di un'associazione registrata, una decisione della bozza (sv-…); il documento
    # confermato non si scollega da qui
    b.vista("elenco")
    riga = p.locator("#el-k-40")
    expect(riga).to_contain_text("● associato")
    expect(riga).to_contain_text("✓ confermato")
    verifica(riga.locator("button", has_text="Scollega dal componente").count() == 1, "«Scollega» anche sul documento confermato")
    i = b.segna()
    riga.locator("button", has_text="Scollega dal componente").click()
    expect(b.finestra()).to_contain_text("Scollegare ACM-1400_A.pdf dal componente?")
    b.ok_finestra()
    b.salvata(2)
    expect(riga).to_contain_text("⊘ da scollegare")
    dec = [a["dopo"] for a in b.ultime_azioni(i) if a["azione"] == "decisione_aggiunta"][0]
    verifica(dec["tipo"] == "scollega_documento" and dec["proposta_id"] == b.pr["ACM-1400_A.pdf"] and re.fullmatch(UUID, dec["componente_id"]), "svincolo: %s" % dec)
    # PN-08: «Fissa l'emittente…» sul prodotto registrato presunto (decisione di-…): un'altra decisione, dopo la prima
    b.vista("org")
    b.scheda("k-root")
    i = b.segna()
    p.click("#du-fissa-emittente")
    p.fill("#f-motivo", "il codice del prodotto è del cliente")
    b.ok_finestra()
    b.salvata(3)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and az[0]["dopo"]["modo"] == "correzione" and az[1]["dopo"]["identita"]["modo"] == "decisione",
             "«Fissa l'emittente»: %s" % az)
    b.apri_wizard()
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/motore-a/bom/conferma", g)
    try:
        p.click("#du-wz-conferma")
        expect(b.esito_wizard()).to_have_attribute("data-esito", "non_registrata", timeout=T_LUNGO)
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    verifica(len(catturate) == 1, "richieste bloccate: %d" % len(catturate))
    k = catturate[0]["bozza"]["chiave_idempotenza"]
    ids_prima = [d["id"] for d in catturate[0]["bozza"]["decisioni"]]
    verifica(len(ids_prima) == 2 and ids_prima != sorted(ids_prima), "le decisioni della pagina sono già nell'ordine del server: la prova non prova niente (%s)" % ids_prima)
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    i = b.segna()
    p.reload()
    b.pronta()
    striscia = b.striscia_sospesa()
    expect(striscia).to_contain_text("il server dice che non è ancora registrato", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "GET", r"/motore-a/intento/"), "nessun R6 all'apertura")
    r6 = b.richieste_da(i, "GET", r"/motore-a/intento/")
    verifica(r6[0]["path"].endswith("/" + k), "R6 all'apertura con un'altra chiave: %s" % r6[0]["path"])
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    verifica("in sospeso" in (p.locator("#du-nuovo-ass").get_attribute("title") or ""), "gesto spento senza il motivo della conferma in sospeso")
    p.click("#du-riprova-sospesa")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-wz-conferma")
    e = b.esito_wizard()
    expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    expect(e).to_contain_text("Conferma n. 3 di ACM-1000")
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 1 and r5[0]["bozza"]["chiave_idempotenza"] == k, "dopo la ricarica la conferma non riusa la chiave in sospeso")
    # il corpo di dopo la ricarica viene dai byte di R1, con gli elenchi nell'ordine del server (EM §3.10): lo stesso
    # contenuto, la stessa chiave
    ids_dopo = [d["id"] for d in r5[0]["bozza"]["decisioni"]]
    verifica(ids_dopo == sorted(ids_dopo) and sorted(ids_dopo) == sorted(ids_prima), "il corpo dopo la ricarica non è quello riletto dal server: %s" % ids_dopo)
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo la conferma")
    expect(striscia).to_have_count(0)


@prova("17", "«Scollega da un padre» (togli_arco); con la conferma in sospeso R3 è spento; abbandono del tentativo, poi R3")
def b17(b):
    p = b.page
    p.click("#du-wz-chiudi")
    p.click("#du-da-conferma")
    b.salvata(1)
    # BZ-09: un arco della conferma fra due componenti si toglie con la decisione togli_arco
    b.scheda("k-40")
    i = b.segna()
    p.click("#du-scollega-padre")
    expect(b.finestra()).to_contain_text("È l'unico padre")
    b.ok_finestra()
    b.salvata(2)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_tolto", "decisione_aggiunta"] and az[1]["dopo"]["tipo"] == "togli_arco" and re.fullmatch(UUID, az[1]["dopo"]["padre_id"])
             and re.fullmatch(UUID, az[1]["dopo"]["figlio_id"]), "«Scollega da un padre»: %s" % az)
    b.apri_wizard()
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/motore-a/bom/conferma", g)
    try:
        p.click("#du-wz-conferma")
        expect(b.esito_wizard()).to_have_attribute("data-esito", "non_registrata", timeout=T_LUNGO)
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    p.click("#du-wz-chiudi")
    striscia = b.striscia_sospesa()
    expect(striscia).to_be_visible()
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    # con la chiave in sospeso anche «Abbandona la bozza…» è spento con il motivo (MAT §5.2, §5.5 n. 7)
    expect(p.locator("#du-abbandona")).to_be_disabled()
    verifica("in sospeso" in (p.locator("#du-abbandona").get_attribute("title") or ""), "«Abbandona la bozza» spento senza il motivo")
    striscia.locator("button", has_text="Abbandona il tentativo").click()
    expect(b.finestra()).to_contain_text("potrebbe ancora arrivare")
    b.ok_finestra()
    expect(striscia).to_have_count(0)
    verifica(not b.sospesi(), "la chiave abbandonata resta nella memoria della scheda")
    expect(p.locator("#du-cta-bom")).to_be_enabled()
    expect(p.locator("#du-nuovo-ass")).to_be_enabled()
    i = b.segna()
    p.click("#du-abbandona")
    expect(b.finestra()).to_contain_text("non si riapre")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    expect(p.locator("#du-nessuna-bozza")).to_be_visible()
    r3 = b.richieste_da(i, "POST", r"/motore-a/bozza/[^/]+/abbandona$")
    verifica(len(r3) == 1 and json.loads(r3[0]["corpo"]) == {"versione_attesa": 2}, "R3: %s" % [r["corpo"] for r in r3])


@prova("18", "proposta cambiata: striscia, «Vedi le differenze», «Aggiorna alla proposta di oggi…»")
def b18(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    b.passo("sorgente")
    p.click("#du-guarda-" + sid(a1))
    expect(p.locator("#du-usa-bom")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-usa-bom")
    b.ok_finestra()
    expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
    b.salvata(1)
    verifica(p.locator("#du-proposta-cambiata").count() == 0, "striscia della proposta cambiata su una bozza appena importata")
    b.proposta_cambia()
    p.click("#du-aggiorna")
    expect(p.locator("#du-proposta-cambiata")).to_contain_text("La proposta del Motore A è cambiata dopo l'importazione.", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-differenze")
    expect(p.locator("#f-differenze")).to_contain_text("k-11: cambiato", timeout=T_LUNGO)
    verifica([r for r in b.richieste_da(i, "GET", r"/motore-a/bozza$") if "confronto=1" in r["query"]], "«Vedi le differenze» senza R1 ?confronto=1")
    b.chiudi_finestra()
    p.click("#du-riallinea")
    expect(b.finestra()).to_contain_text("Aggiorna la bozza alla proposta di oggi", timeout=T_LUNGO)
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "la proposta di oggi corregge la traversa")
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza aggiornata alla proposta di oggi.", timeout=T_LUNGO)
    expect(p.locator("#du-ultima-forma")).to_contain_text("identità aggiornate: 1")
    expect(p.locator("#du-proposta-cambiata")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica("riallinea_proposta" in c and "snapshot" not in c and "azioni" not in c and c["riallinea_proposta"]["motivo"] and len(c["riallinea_proposta"]["impronta_contenuto"]) == 64,
             "corpo di riallinea_proposta: %s" % c)
    b.salvata(2)


@prova("19", "«Cambia lo STEP della BOM…»: effetto da S6, motivo, cambia_sorgente")
def b19(b):
    p = b.page
    a10 = b.al["ACM-1000_rev2.stp"]
    b.passo("sorgente")
    i = b.segna()
    p.click("#du-guarda-" + sid(a10))
    expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    s6 = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6 and "bozza=" in s6[-1]["query"], "S6 senza la bozza: %s" % [r["query"] for r in s6])
    expect(p.locator("#du-anteprima")).to_contain_text("diversa dal prodotto")
    i = b.segna()
    p.click("#du-cambia-sorgente")
    eff = p.locator("#f-effetto")
    expect(eff).to_contain_text("nodi che restano: 1")
    expect(eff).to_contain_text("nodi che escono: 1")
    expect(eff).to_contain_text("nodi che entrano: 3")
    # l'effetto è quello di S6 riletto adesso con la bozza (MAT §1.5, SG-04), con i nodi
    s6b = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6b and "bozza=" in s6b[-1]["query"], "«Cambia lo STEP…» non rilegge S6 con la bozza prima del dialogo: %s" % [r["query"] for r in s6b])
    expect(eff).to_contain_text("(k-12)")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "lo STEP rev D sostituisce la rev C")
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Fonte della bozza cambiata.", timeout=T_LUNGO)
    expect(p.locator("#du-ultima-forma")).to_contain_text("identità aggiornate: 1")
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["cambia_sorgente"]["sorgente"]["allegato_id"] == a10 and c["cambia_sorgente"]["motivo"] and c["versione_attesa"] == 2, "cambia_sorgente: %s" % c)
    expect(p.locator("#du-riallinea-da-step")).to_be_visible(timeout=T_LUNGO)


@prova("20", "«Importa dalla proposta…» (BZ-15) e «Riparti dalla proposta…» (BZ-16)")
def b20(b):
    p = b.page
    a10 = b.al["ACM-1000_rev2.stp"]
    b.passo("revisione")
    b.vista("org")
    expect(p.locator("#du-proposta-step")).to_be_visible(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-importa")
    elenco = p.locator("#f-importa")
    expect(elenco).to_contain_text("ACM-1500")
    expect(elenco).to_contain_text("tolto nella proposta")
    verifica(p.locator("#f-importa input:checked").count() == 0, "elementi preselezionati nell'import")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.locator("#f-importa label", has_text="ACM-1500").locator("input").check()
    b.ok_finestra()
    expect(p.locator("#du-vp .du-nc", has_text="ACM-1500")).to_have_count(1)
    b.salvata(4)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    nodi = [a["dopo"] for a in az if a["azione"] == "nodo_aggiunto"]
    archi = [a["dopo"] for a in az if a["azione"] == "arco_aggiunto"]
    verifica(len(nodi) == 1 and len(archi) == 1 and len(az) == 2, "azioni dell'import: %s" % [a["azione"] for a in az])
    nodo, arco = nodi[0], archi[0]
    verifica(nodo["origine"] == "step" and nodo["op"] == "crea" and nodo["identita"]["modo"] == "proposta" and nodo["identita"]["codice"] == "ACM-1500"
             and nodo["caselle"][0]["rif"].endswith(":#50") and nodo["caselle"][0]["gruppo"].startswith("cand:") and nodo["caselle"][0]["allegato_id"] == a10,
             "nodo importato: %s" % nodo)
    verifica(arco["padre"] == "k-root" and arco["figlio"] == nodo["chiave_bozza"] and arco["qta"] == 1 and arco["origine"] == "step"
             and arco["step"] == {"allegato_id": a10, "gruppo": nodo["caselle"][0]["gruppo"]}, "legame importato: %s" % arco)
    # «Riparti dalla proposta…»: la bozza di adesso si abbandona, la nuova nasce dalla proposta di oggi
    i = b.segna()
    p.click("#du-riparti")
    expect(b.finestra()).to_contain_text("Niente della bozza di adesso passa alla nuova")
    expect(b.finestra()).to_contain_text("Le preassociazioni non passano alla bozza nuova.")
    # la radice dello STEP rev D non ha il codice del prodotto: il motivo vale anche per la creazione (MAT SG-05)
    expect(p.locator("#f-radice-diversa")).to_contain_text("ACM-1000-R2")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "si riparte dalla proposta della rev D")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza ripartita dalla proposta dello STEP", timeout=T_LUNGO)
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc", has_text="ACM-1500")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["bozza_id"] is None and c["versione_attesa"] == 0 and c["crea"]["partenza"] == "step" and c["crea"]["sorgente"]["allegato_id"] == a10
             and c["crea"]["abbandona"]["versione_attesa"] == 4 and c["crea"]["abbandona"]["motivo"] and len(c["crea"]["impronta_contenuto"]) == 64
             and c["crea"].get("motivo") == c["crea"]["abbandona"]["motivo"], "corpo della ripartenza: %s" % c)
    verifica(b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$"), "«Riparti» senza la proposta di oggi della sorgente (S6)")
    verifica(b.stato()["conti"].get("ripartenze") == 1, "il banco non ha visto la ripartenza")


@prova("21", "file: Scarta (D-6, D-7, D-14, archivio), Altri file, Riattiva, 503 NON_CONFIGURATO")
def b21(b):
    p = b.page
    a = b.al
    ok = p.locator("#du-finestra #f-ok")
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    # D-6: la fonte della bozza blocca
    p.click("#du-scarta-" + sid(a["ACM-1000_rev2.stp"]))
    expect(p.locator('#f-dipendenze li[data-codice="D-6"][data-esito="blocca"]')).to_be_visible(timeout=T_LUNGO)
    expect(ok).to_be_disabled()
    verifica("D-6" in (ok.get_attribute("title") or ""), "Scarta spento senza il motivo D-6")
    b.chiudi_finestra()
    # D-7: lo STEP associato al prodotto vuole «Scollega e scarta»
    p.click("#du-scarta-" + sid(a["ACM-1000_assieme.stp"]))
    expect(p.locator('#f-dipendenze li[data-codice="D-7"]')).to_be_visible(timeout=T_LUNGO)
    p.fill("#f-motivo", "prova")
    expect(ok).to_be_disabled()
    verifica("Scollega e scarta" in (ok.get_attribute("title") or ""), "Scarta spento senza il motivo D-7")
    b.chiudi_finestra()
    # D-14: l'unica copia
    i = b.segna()
    p.click("#du-scarta-" + sid(a["collaudo_interno.pdf"]))
    expect(p.locator("#f-unica")).to_be_visible(timeout=T_LUNGO)
    expect(b.finestra()).to_contain_text("La mail originale, i documenti confermati, le copie sul NAS e la storia non si toccano.")
    p.fill("#f-motivo", "collaudo superato dalla versione nuova")
    expect(ok).to_be_disabled()
    p.locator("#f-unica").check()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Scarto registrato; rimozione dallo staging ancora da completare.", timeout=T_LUNGO)
    s2 = b.corpi(i, "POST", r"/scarta$")
    verifica(len(s2) == 1 and s2[0].get("conferma_unica_copia") is True and s2[0]["motivo"] and re.fullmatch(UUID, s2[0]["chiave_idempotenza"])
             and len(s2[0]["impronta_dipendenze"]) == 64 and s2[0]["versione_attesa"] == 0, "corpo di S2: %s" % s2)
    expect(p.locator("#du-cartella-scartati")).to_have_text("Scartati (1)", timeout=T_LUNGO)
    p.click("#du-cartella-scartati")
    riga = p.locator("#du-file-" + sid(a["collaudo_interno.pdf"]))
    expect(riga).to_have_attribute("data-stato", "scartato")
    expect(riga).to_contain_text("Scartato da AD")
    expect(riga).to_contain_text("File scartato ed eliminato dallo staging.", timeout=25000)
    # la riga dice lo stato di adesso (S1), non la frase della risposta di S2
    expect(riga).not_to_contain_text("ancora da completare")
    b.foto("B20_file_scartati.png")
    # l'archivio porta le sue voci
    p.click("#du-cartella-attivi")
    i = b.segna()
    p.click("#du-scarta-" + sid(a["pacchetto.zip"]))
    expect(b.finestra()).to_contain_text("lo seguono tutte le sue 2 voci attive", timeout=T_LUNGO)
    p.fill("#f-motivo", "archivio duplicato")
    b.ok_finestra()
    expect(p.locator("#du-cartella-scartati")).to_have_text("Scartati (4)", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/scarta$")[0]
    verifica(c["n_voci"] == 2 and len(c["impronta_voci"]) == 64, "S2 dell'archivio senza il perimetro delle voci: %s" % c)
    # Altri file e Riattiva
    p.click("#du-altri-" + sid(a["PIASTRA_400123.dxf"]))
    expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file: il contenuto è conservato.", timeout=T_LUNGO)
    expect(p.locator("#du-cartella-altri")).to_have_text("Altri file (1)", timeout=T_LUNGO)
    p.click("#du-cartella-altri")
    riga = p.locator("#du-file-" + sid(a["PIASTRA_400123.dxf"]))
    expect(riga).to_have_attribute("data-stato", "altri_file")
    expect(riga).to_contain_text("Conservato (copia verificata).")
    p.click("#du-riattiva-" + sid(a["PIASTRA_400123.dxf"]))
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File riattivato", timeout=T_LUNGO)
    expect(p.locator("#du-cartella-altri")).to_have_text("Altri file (0)", timeout=T_LUNGO)
    # 503 NON_CONFIGURATO: il dialogo resta aperto, niente cambia
    p.click("#du-cartella-attivi")
    b.modo("S3", "503")
    p.click("#du-altri-" + sid(a["SCHEMA_GENERALE.pdf"]))
    b.ok_finestra()
    expect(p.locator("#du-finestra-errore")).to_contain_text("non è configurata su questo server: niente è cambiato. (NON_CONFIGURATO", timeout=T_LUNGO)
    expect(b.finestra()).to_be_visible()
    b.chiudi_finestra()
    verifica(b.stato()["file"]["SCHEMA_GENERALE.pdf"]["stato"] == "attivo", "il 503 ha cambiato il file")
    verifica(not b.sospesi(), "un 503 ha lasciato la chiave in sospeso")


@prova("22", "S2 con la risposta persa: «Esito da verificare», R6 con la stessa chiave, «registrato»")
def b22(b):
    p = b.page
    a4 = b.al["SCHEMA_GENERALE.pdf"]
    g = perdi_la_risposta()
    p.route("**/sorgente/file/*/scarta", g)
    try:
        p.click("#du-scarta-" + sid(a4))
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        p.fill("#f-motivo", "schema generale superato")
        b.ok_finestra()
        expect(b.avviso()).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
        expect(b.avviso()).to_contain_text("Verificato: il gesto su SCHEMA_GENERALE.pdf è registrato.", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/scarta", g)
    st = b.stato()
    verifica(st["file"]["SCHEMA_GENERALE.pdf"]["stato"] == "scartato", "il file non è scartato nel banco")
    corpi = [c for c in st["corpi_file"] if c["gesto"] == "scarta_file" and c["allegato"] == a4]
    verifica(len(corpi) == 1, "S2 arrivato %d volte" % len(corpi))
    k = corpi[0]["corpo"]["chiave_idempotenza"]
    verifica(k in st["chiavi_r6"], "R6 senza la chiave dello scarto")
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo «registrato»")
    expect(p.locator("#du-cartella-scartati")).to_have_text("Scartati (5)", timeout=T_LUNGO)


@prova("23", "S3 mai arrivata: R6 «non registrato», ricarica, «Riprova» dalla striscia con la stessa chiave")
def b23(b):
    p = b.page
    a3 = b.al["ACM-1120_B.pdf"]
    risorsa = "file:" + a3
    p.click("#du-cartella-attivi")
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        p.click("#du-altri-" + sid(a3))
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("non è ancora registrato", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    verifica(len(catturate) == 1, "richieste bloccate: %d" % len(catturate))
    k = catturate[0]["chiave_idempotenza"]
    striscia = p.locator("#du-sospeso-" + sid(risorsa))
    expect(striscia).to_contain_text("il server dice che non è ancora registrato", timeout=T_LUNGO)
    expect(striscia).to_contain_text("ACM-1120_B.pdf")
    sosp = [x for x in b.sospesi() if x[1]["risorsa"] == risorsa]
    verifica(len(sosp) == 1 and sosp[0][1]["chiave"] == k and sosp[0][1].get("verificato") is True, "memoria della scheda: %s" % sosp)
    verifica(b.stato()["file"]["ACM-1120_B.pdf"]["stato"] == "attivo", "una richiesta mai arrivata ha cambiato il file")
    # la ricarica non perde il tentativo: la chiave e il corpo stanno nella memoria della scheda (EM §10.9)
    i0 = b.segna()
    p.reload()
    b.pronta()
    b.passo("sorgente")
    expect(striscia).to_contain_text("il server dice che non è ancora registrato", timeout=T_LUNGO)
    b.attendi(lambda: [r for r in b.richieste_da(i0, "GET", r"/motore-a/intento/") if r["path"].endswith("/" + k)],
              "nessun R6 all'apertura per il tentativo in sospeso")
    i = b.segna()
    p.click("#du-riprova-" + sid(risorsa))
    expect(b.avviso()).to_contain_text("File messo in Altri file: il contenuto è conservato.", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1 and s3[0] == catturate[0], "«Riprova» non rimanda lo stesso corpo con la stessa chiave: %s" % s3)
    expect(striscia).to_have_count(0)
    verifica(not [x for x in b.sospesi() if x[1]["risorsa"] == risorsa], "la chiave resta in sospeso dopo «Riprova»")
    verifica(b.stato()["file"]["ACM-1120_B.pdf"]["stato"] == "altri_file", "il file non è in Altri file nel banco")


@prova("24", "«+ Aggiungi file» (E1′): parziale per file, in analisi poi letto, «già presente»")
def b24(b):
    p = b.page
    p.click("#du-cartella-attivi")
    i = b.segna()
    p.set_input_files("#du-carica-file", files=[
        {"name": "NUOVO_DISEGNO.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% sintetico\n"},
        {"name": "programma.exe", "mimeType": "application/octet-stream", "buffer": b"MZ sintetico"},
    ])
    p.select_option("#du-carica-origine", "interna")
    p.click("#du-carica-invia")
    esiti = p.locator("#du-carica-esiti")
    expect(esiti.locator('li[data-esito="caricato"]')).to_contain_text("caricato · in analisi", timeout=T_LUNGO)
    expect(esiti.locator('li[data-esito="rifiutato"]')).to_contain_text("rifiutato: tipo di file non ammesso")
    expect(b.avviso()).to_have_attribute("data-esito", "no")
    expect(b.avviso()).to_contain_text("Non caricati: programma.exe.")
    e1 = b.richieste_da(i, "POST", r"/carica$")
    verifica(len(e1) == 1 and e1[0]["headers"].get("accept") == "application/json" and "multipart/form-data" in e1[0]["headers"].get("content-type", ""),
             "E1′: %s" % [r["headers"] for r in e1])
    riga = p.locator("#du-file li.du-fr", has_text="NUOVO_DISEGNO.pdf")
    expect(riga).to_contain_text("in analisi", timeout=T_LUNGO)
    expect(riga).to_contain_text("interna")
    expect(riga).to_contain_text("letto", timeout=25000)
    p.set_input_files("#du-carica-file", files=[{"name": "NUOVO_DISEGNO.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% sintetico\n"}])
    p.click("#du-carica-invia")
    expect(esiti.locator('li[data-esito="gia_presente"]')).to_contain_text("già presente", timeout=T_LUNGO)


@prova("25", "atti del menu e «Revoca»; R11 con F8 in errore; «Apri»: un link a S5, un PDF nel visore")
def b25(b):
    p = b.page
    i = b.segna()
    p.click("#du-menu-rfq summary")
    p.click("#du-prepara")
    expect(b.avviso()).to_contain_text("Preparazione accodata", timeout=T_LUNGO)
    r = b.richieste_da(i, "POST", r"/fascicolo/prepara$")
    verifica(len(r) == 1 and (r[0]["corpo"] or "") == "" and r[0]["headers"].get("hx-request") == "true", "«Prepara i file»: %s" % r)
    # gli altri due atti del menu
    i = b.segna()
    p.click("#du-menu-rfq summary")
    p.click("#du-rianalizza")
    expect(b.avviso()).to_contain_text("Rianalisi accodata", timeout=T_LUNGO)
    p.click("#du-menu-rfq summary")
    p.click("#du-riprova-copie")
    expect(b.avviso()).to_contain_text("Copie riaccodate", timeout=T_LUNGO)
    verifica(len(b.richieste_da(i, "POST", r"/fascicolo/rianalizza$")) == 1 and len(b.richieste_da(i, "POST", r"/riprova-copie$")) == 1,
             "«Rifai l'analisi» e «Riprova le copie»: %s" % [(r["metodo"], r["path"]) for r in b.richieste_da(i, "POST")])
    # «Revoca l'autorizzazione…»: il motivo, lo SHA della fonte
    expect(p.locator("#du-revoca")).to_be_visible()
    p.click("#du-revoca")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "lo STEP strutturale va cambiato")
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Autorizzazione revocata.", timeout=T_LUNGO)
    rv = b.richieste_da(i, "POST", r"/fascicolo/componente/[^/]+/step-strutturale/revoca$")
    verifica(len(rv) == 1, "«Revoca»: %d richieste" % len(rv))
    f = urllib.parse.parse_qs(rv[0]["corpo"] or "")
    verifica(f.get("motivo") == ["lo STEP strutturale va cambiato"] and f.get("sha") == [b.stato()["fonte"]["sha256"]], "campi di «Revoca»: %s" % f)
    expect(p.locator("#du-passo-sorgente")).to_contain_text("STEP non autorizzato")
    # R11: F8 in errore = «Riallinea i documenti» nel menu
    verifica(p.locator("#du-riallinea-documenti").count() == 0, "«Riallinea i documenti» con F8 eseguito")
    b.f8_errore()
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    p.click("#du-menu-rfq summary")
    i = b.segna()
    p.click("#du-riallinea-documenti")
    expect(b.avviso()).to_contain_text("Documenti riallineati (F8: eseguito).", timeout=T_LUNGO)
    verifica(len(b.richieste_da(i, "POST", r"/motore-a/riallinea$")) == 1, "R11: %s" % [(r["metodo"], r["path"]) for r in b.richieste_da(i, "POST")])
    verifica(p.locator("#du-riallinea-documenti").count() == 0, "«Riallinea i documenti» resta dopo R11")
    # «Apri» di un file che non è un PDF: un link a S5, che lo dà come allegato (FL-03)
    dxf = b.al["PIASTRA_400123.dxf"]
    href = p.locator("#du-apri-" + sid(dxf)).get_attribute("href")
    verifica(href == "/thread/%s/sorgente/file/%s/contenuto?modo=anteprima" % (b.thread, dxf), "«Apri»: %s" % href)
    resp = p.request.get(b.url + href)
    verifica(resp.status == 200 and "attachment" in resp.headers.get("content-disposition", "") and resp.headers.get("x-content-type-options") == "nosniff",
             "S5: %d %s" % (resp.status, resp.headers))
    verifica(not b.richieste_da(i, "GET", r"/contenuto$"), "la pagina ha chiesto S5 con una fetch")
    # «Apri» di un PDF: il visore pdf.js della pagina, che legge l'indirizzo di S5 (modo=anteprima), una pagina alla volta
    a2 = b.al["ACM-1100_A.pdf"]
    i = b.segna()
    p.click("#du-apri-" + sid(a2))
    expect(p.locator("#du-visore-tela canvas")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-visore-pagina")).to_have_text("pagina 1 di 1")
    expect(p.locator("#du-visore-dopo")).to_be_disabled()
    s5 = b.richieste_da(i, "GET", r"/contenuto$")
    verifica(s5 and all("modo=anteprima" in r["query"] for r in s5) and all(r["headers"].get("accept") != "application/json" for r in s5),
             "il visore non legge S5 per indirizzo: %s" % [(r["path"], r["query"]) for r in s5])
    b.chiudi_finestra()


@prova("26", "secondo prodotto: indirizzo, nessuna struttura inventata")
def b26(b):
    p = b.page
    a10 = b.al["ACM-1000_rev2.stp"]
    tab2 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)
    tab2.click()
    expect(tab2).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.attendi(lambda: b.indirizzo().get("prodotto") == b.p2, "l'indirizzo non porta il secondo prodotto")
    b.passo("sorgente")
    expect(p.locator("#du-fonte")).to_contain_text("il prodotto non ha uno STEP associato")
    p.click("#du-guarda-" + sid(a10))
    expect(p.locator("#du-anteprima")).to_contain_text("La struttura dello STEP non è determinabile", timeout=T_LUNGO)
    expect(p.locator("#du-anteprima")).to_contain_text("Nessuna struttura inventata.")
    verifica(p.locator("#du-anteprima .du-vp").count() == 0, "un organigramma per una struttura non determinabile")
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_contain_text("Nessuna bozza per ACM-2000.")
    tab1 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)
    tab1.click()
    expect(tab1).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)


@prova("27", "D-18: il modello che non si calcola: striscia, niente tela; «Aggiorna» lo rifà")
def b27(b):
    p = b.page
    b.passo("revisione")
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)
    b.modo("dati", "errore")
    p.click("#du-aggiorna")
    expect(p.locator("#du-errore-modello")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-errore-modello")).to_contain_text("Il calcolo del motore A non è riuscito")
    expect(p.locator("#du-corpo")).to_contain_text("niente tela, bozza né wizard")
    verifica(p.locator("#du-vp").count() == 0, "la tela con il modello in errore")
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    p.click("#du-aggiorna")
    expect(p.locator("#du-errore-modello")).to_have_count(0, timeout=T_LUNGO)
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)


@prova("28", "impaginazione a 1440, 1024 e 390 px senza scorrimento orizzontale")
def b28(b):
    p = b.page
    misura = "() => document.documentElement.scrollWidth - document.documentElement.clientWidth"
    try:
        for larg, alt in ((1440, 900), (1024, 768), (390, 844)):
            p.set_viewport_size({"width": larg, "height": alt})
            for passo, vista in (("sorgente", None), ("revisione", "org"), ("revisione", "elenco")):
                b.passo(passo)
                if vista:
                    b.vista(vista)
                p.wait_for_timeout(350)
                d = p.evaluate(misura)
                verifica(d <= 1, "%d px, %s %s: scorrimento orizzontale di %d px" % (larg, passo, vista or "", d))
            b.vista("org")
            b.apri_wizard()
            d = p.evaluate(misura)
            verifica(d <= 1, "%d px, wizard: scorrimento orizzontale di %d px" % (larg, d))
            b.foto("larghezza_%d.png" % larg)
            p.click("#du-wz-chiudi")
            expect(p.locator("#du-wizard")).to_be_hidden()
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("29", "Sorgente: dove va il file (SG-06), «Assegna a un pezzo…» (FL-12), «Scarica» (FL-03)")
def b29(b):
    p = b.page
    a2 = b.al["ACM-1100_A.pdf"]
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    riga = p.locator("#du-file-" + sid(a2))
    expect(riga).to_contain_text("◇ proposto per ACM-1100", timeout=T_LUNGO)
    sc = p.locator("#du-scarica-" + sid(a2))
    verifica(sc.get_attribute("href") == "/thread/%s/sorgente/file/%s/contenuto?modo=scarica" % (b.thread, a2) and sc.get_attribute("download") == "ACM-1100_A.pdf",
             "«Scarica»: %s %s" % (sc.get_attribute("href"), sc.get_attribute("download")))
    i = b.segna()
    p.click("#du-assegna-" + sid(a2))
    expect(p.locator("#f-proposti")).to_contain_text("ACM-1100")
    verifica(p.locator("#f-pezzo").input_value() == "", "«Assegna» ha un pezzo preselezionato")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.select_option("#f-pezzo", "k-7")
    b.ok_finestra()
    expect(riga).to_contain_text("◈ preassociato a ACM-1100", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Assegna»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"proposta_id": b.pr["ACM-1100_A.pdf"], "chiave_bozza": "k-7"})], "«Assegna»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-togli-pb-" + sid(a2))
    expect(riga).to_contain_text("◇ proposto per ACM-1100", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Togli la preassociazione»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([a["azione"] for a in az] == ["file_svincolato"] and az[0]["dopo"]["motivo"] == "utente", "«Togli la preassociazione»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # senza bozza (il secondo prodotto) «Assegna» è spento con il motivo
    p.locator('.du-ptab[data-prodotto="%s"]' % b.p2).click()
    expect(p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    bt = p.locator("#du-assegna-" + sid(a2))
    expect(bt).to_be_disabled()
    verifica("Usa per la BOM" in (bt.get_attribute("title") or ""), "«Assegna» spento senza il motivo")
    p.locator('.du-ptab[data-prodotto="%s"]' % b.p1).click()
    expect(p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)


@prova("30", "Elenco: filtri e ricerca (EL-01), «Accetta le proposte (N)» (EL-03, PE-21), «Confronto con la proposta» (BZ-25)")
def b30(b):
    p = b.page
    b.passo("revisione")
    b.vista("elenco")
    righe = p.locator("#du-elenco [role=row][data-k]")
    expect(righe).to_have_count(10)
    p.click("#du-filtro-prop")
    expect(p.locator("#du-filtro-prop")).to_have_attribute("aria-pressed", "true")
    n = righe.count()
    verifica(n >= 1 and all(righe.nth(j).get_attribute("data-du-esito") == "prop" for j in range(n)), "filtro «Proposti da verificare»: %d righe" % n)
    p.click("#du-filtro-tutti")
    p.fill("#du-el-cerca", "ACM-14")
    expect(righe).to_have_count(2)
    verifica(sorted(righe.nth(j).get_attribute("data-k") for j in range(2)) == ["k-40", "k-41"], "ricerca «ACM-14»")
    p.fill("#du-el-cerca", "zzz-nessuno")
    expect(righe).to_have_count(0)
    p.click("#du-filtro-azzera")
    expect(righe).to_have_count(10)
    i = b.segna()
    # PE-21 (decisioni dell'ondata A): il pulsante si chiama «Accetta le proposte (N)», senza «valide» (la validità la dice
    # R4 dopo il salvataggio) e senza preselezione
    bt = p.locator("#du-accetta-valide")
    verifica(re.fullmatch(r"Accetta le proposte \(\d+\)", bt.inner_text().strip()) is not None, "nome del pulsante dell'accettazione cumulativa: %r" % bt.inner_text())
    bt.click()
    expect(b.finestra()).to_contain_text("Accetta le proposte")
    verifica("valide" not in b.finestra().inner_text(), "il dialogo dice ancora «proposte valide»")
    elenco = p.locator("#f-proposte input")
    verifica(elenco.count() >= 1 and p.locator("#f-proposte input:checked").count() == 0, "«Accetta le proposte»: %d proposte, preselezioni presenti" % elenco.count())
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.locator("#f-proposte label", has_text="ACM-1100_A.pdf").locator("input").check()
    b.ok_finestra()
    expect(p.locator("#el-k-7")).to_contain_text("◈ preassociato", timeout=T_LUNGO)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["dopo"]["chiave_bozza"]) for a in az] == [("file_preassociato", "k-7")], "accettazione cumulativa: %s" % az)
    # «Confronto con la proposta»: il codice corretto a mano si vede sulla scheda, le altre schede no
    b.vista("org")
    b.scheda("k-12")
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1130")
    p.fill("#f-motivo", "codice dal disegno del cliente")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    verifica("≠ proposta" not in p.locator("#du-vp").inner_text(), "«≠ proposta» senza il confronto acceso")
    p.click("#du-confronto")
    expect(p.locator("#du-confronto")).to_have_attribute("aria-pressed", "true")
    expect(p.locator("#nc-k-12")).to_contain_text("≠ proposta")
    expect(p.locator("#du-vp .du-nc", has_text="≠ proposta")).to_have_count(1)
    p.click("#du-confronto")


@prova("31", "pannello: «Documenti» (EL-08), «⤴ Ricolloca i figli…» (BZ-11)")
def b31(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-40")
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-40")
    i = b.segna()
    p.click("#du-ricolloca")
    expect(b.finestra()).to_contain_text("Ricolloca i figli di ACM-1400")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.select_option("#f-a", "k-7")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.locator("#f-stesse-qta").check()
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica(len(az) == 1 and az[0]["azione"] == "arco_spostato" and az[0]["prima"]["padre"] == "k-40" and az[0]["dopo"] == {"padre": "k-7", "figlio": "k-41", "qta": 2, "origine": "utente"},
             "«Ricolloca i figli»: %s" % az)
    # la linguetta «Documenti del nodo» del pannello: un file da smistare preassociato a questo pezzo
    b.scheda("k-41")
    b.linguetta("documenti")
    expect(p.locator("#du-sez-documenti")).to_be_visible()
    opz = p.locator("#du-pannello-file option").all_inner_texts()
    verifica("PIASTRA_400123.dxf" in opz and "ACM-1100_A.pdf" not in opz and not [x for x in opz if x.endswith(".stp")], "file da smistare nel pannello: %s" % opz)
    i = b.segna()
    p.select_option("#du-pannello-file", label="PIASTRA_400123.dxf")
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dalla sezione «Documenti»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"proposta_id": b.pr["PIASTRA_400123.dxf"], "chiave_bozza": "k-41"})], "preassociazione dal pannello: %s" % az)
    p.click("#du-pannello-carica")
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    b.attendi(lambda: p.evaluate("() => document.activeElement && document.activeElement.id") == "du-carica-file", "«Carica…» non porta al caricamento")


@prova("32", "«Riepilogo RFQ» (TS-08), «STEP secondario disponibile» (SG-12), «nessuna candidata» (SG-13)")
def b32(b):
    p = b.page
    p.click("#du-menu-rfq summary")
    p.click("#du-riepilogo-rfq")
    expect(p.locator("#f-riepilogo-rfq")).to_contain_text("ACME Macchine")
    expect(p.locator("#f-riepilogo-prodotti")).to_contain_text("ACM-1000")
    expect(p.locator("#f-riepilogo-prodotti")).to_contain_text("ACM-2000")
    b.chiudi_finestra()
    b.passo("sorgente")
    expect(p.locator("#du-step-secondario")).to_contain_text("STEP secondario disponibile")
    expect(p.locator("#du-step-secondario")).to_contain_text("ACM-1000_rev2.stp")
    expect(p.locator("#du-step-secondario")).to_contain_text("V5")
    verifica(p.locator("#du-nessuna-candidata").count() == 0, "«nessuna candidata» per un prodotto con candidate")
    p.locator('.du-ptab[data-prodotto="%s"]' % b.p2).click()
    expect(p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    expect(p.locator("#du-nessuna-candidata")).to_contain_text("Nessuna candidata per ACM-2000")
    expect(p.locator("#du-nessuna-candidata")).to_contain_text("+ Aggiungi file")
    verifica(p.locator("#du-step-secondario").count() == 0, "STEP secondario per un prodotto senza")
    p.locator('.du-ptab[data-prodotto="%s"]' % b.p1).click()
    expect(p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)


JS_CASI = r"""async (casi) => {
  const M = await import("/static/distinta_unica.mjs");
  const can = (v) => JSON.stringify(v, (k, x) => (x && typeof x === "object" && !Array.isArray(x)) ?
    Object.keys(x).sort().reduce((o, kk) => { o[kk] = x[kk]; return o; }, {}) : x);
  const out = [];
  for (const c of casi.vassoio) {
    const avuto = M.vassoioDa(c.eventi);
    out.push({ nome: c.nome, ok: can(avuto) === can(c.atteso), avuto });
  }
  for (const c of casi.applica) {
    const s = JSON.parse(JSON.stringify(c.snapshot));
    for (const a of c.azioni) M.applica(s, a);
    out.push({ nome: c.nome, ok: can(s) === can(c.atteso), avuto: s });
  }
  // la forma canonica di R5 nel ponte (EM §3.10): lo stesso contenuto in un altro ordine, con i facoltativi vuoti e un
  // motivo in un'altra forma Unicode, ha la stessa impronta; un contenuto diverso no
  const P = await import("/static/distinta_unica_ponte.mjs");
  const a = { firma_anteprima: "f", bozza: { descrizione: "", documenti_preassociati: [], archi: [{ padre: "k-b", figlio: "k-c" }, { padre: "k-a", figlio: "k-c" }],
    nodi: [{ chiave_bozza: "k-b" }, { chiave_bozza: "k-a", caselle: [{ gruppo: "g", rif: "r2", allegato_id: "x" }, { gruppo: "g", rif: "r1", allegato_id: "x" }] }],
    decisioni: [{ id: "sv-1", motivo: "é" }, { id: "di-1", identita: { codice: "é" } }] } };
  const b = { firma_anteprima: "f", bozza: { decisioni: [{ id: "di-1", identita: { codice: "é" } }, { id: "sv-1", motivo: "é" }],
    nodi: [{ chiave_bozza: "k-a", caselle: [{ gruppo: "g", rif: "r1", allegato_id: "x" }, { gruppo: "g", rif: "r2", allegato_id: "x" }] }, { chiave_bozza: "k-b" }],
    archi: [{ padre: "k-a", figlio: "k-c" }, { padre: "k-b", figlio: "k-c" }] } };
  const f = (x) => P.canonico(P.formaCanonica(x, ""));
  const diverso = JSON.parse(JSON.stringify(b));
  diverso.bozza.decisioni[0].identita.codice = "é";
  out.push({ nome: "forma canonica di R5 (EM §3.10)", ok: f(a) === f(b) && f(a) !== f(diverso), avuto: [f(a), f(b), f(diverso)] });
  return out;
}"""


@prova("33", "casi condivisi: vassoio «Da ricollocare» (T-16, lato pagina) e azioni del vocabolario 2")
def b33(b):
    with open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "distinta_unica_casi.json"), encoding="utf-8") as f:
        casi = json.load(f)
    p2 = b.page.context.new_page()
    try:
        p2.goto(b.url + "/login")
        esiti = p2.evaluate(JS_CASI, casi)
        verifica(p2.locator("#du").count() == 0, "la scheda dei casi ha il guscio della pagina")
    finally:
        p2.close()
    falliti = [(e["nome"], e["avuto"]) for e in esiti if not e["ok"]]
    verifica(len(esiti) == len(casi["vassoio"]) + len(casi["applica"]) + 1 and not falliti, "casi falliti: %s" % falliti)


@prova("35", "§4.3 righe 1 e 2: un pezzo aggiunto con un file preassociato; «Non crearlo» lo manda nel vassoio; «Lascia senza associazione»")
def b35(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-7")
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-aggiungi-sotto")
    p.select_option("#f-tipo", "sciolto")
    p.fill("#f-desc", "Rondella aggiunta a mano")
    p.fill("#f-qta", "4")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="nuovo"]')).to_have_count(1, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    nodo = az[0]["dopo"]
    verifica([a["azione"] for a in az] == ["nodo_aggiunto", "arco_aggiunto"] and nodo["op"] == "crea" and nodo["origine"] == "utente"
             and nodo["identita"]["modo"] == "manuale" and "codice" not in nodo["identita"] and nodo["identita"]["emittente_presunto"] is True
             and az[1]["dopo"]["padre"] == "k-7" and az[1]["dopo"]["qta"] == 4, "«Aggiungi»: %s" % az)
    k = nodo["chiave_bozza"]
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # riga 1: il pannello del pezzo nuovo offre i file da smistare; senza codice la preassociazione è «in attesa del codice»
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", k)
    b.linguetta("documenti")
    opz = p.locator("#du-pannello-file option").all_inner_texts()
    verifica("NUOVO_DISEGNO.pdf" in opz, "file da smistare del pezzo nuovo: %s" % opz)
    pid = b.proposta_di("NUOVO_DISEGNO.pdf")
    i = b.segna()
    p.select_option("#du-pannello-file", label="NUOVO_DISEGNO.pdf")
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-doc-nodo")).to_contain_text("◈ preassociato", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"proposta_id": pid, "chiave_bozza": k})], "preassociazione: %s" % az)
    expect(p.locator("#du-doc-nodo")).to_contain_text("in_attesa_codice", timeout=T_LUNGO)
    # riga 2: «Non crearlo» toglie il nodo e manda il file nel vassoio (file_svincolato nodo_tolto), nello stesso salvataggio
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-togli")
    expect(b.finestra()).to_contain_text("1 file preassociati vanno in «Da ricollocare»")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="nuovo"]')).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_tolto", "file_svincolato", "nodo_tolto"] and az[1]["dopo"] == {"proposta_id": pid, "motivo": "nodo_tolto"},
             "«Non crearlo»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # il vassoio nella linguetta «Da ricollocare» della Sorgente (MAT §3.3) e nell'Elenco
    b.passo("sorgente")
    expect(p.locator("#du-cartella-ricollocare")).to_have_text("Da ricollocare (1)")
    p.click("#du-cartella-ricollocare")
    expect(p.locator("#du-file")).to_contain_text("NUOVO_DISEGNO.pdf")
    p.click("#du-cartella-attivi")
    b.passo("revisione")
    b.vista("elenco")
    vass = p.locator("#du-vassoio")
    expect(vass).to_contain_text("NUOVO_DISEGNO.pdf")
    expect(vass).to_contain_text("nodo_tolto")
    i = b.segna()
    p.click("#du-lascia-" + sid(pid))
    expect(vass).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_lasciato", {"proposta_id": pid})], "«Lascia senza associazione»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


@prova("36", "§4.3 righe 3 e 4: «Escludi» un registrato (scelta per le AR, documenti confermati, file nel vassoio con nodo_escluso, svincoli con da_decisione) e «Ripristina» dopo una ricarica")
def b36(b):
    p = b.page
    pid = b.proposta_di("NUOVO_DISEGNO.pdf")
    b.vista("org")
    b.scheda("k-40")
    b.linguetta("documenti")
    i = b.segna()
    p.select_option("#du-pannello-file", label="NUOVO_DISEGNO.pdf")
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-doc-nodo")).to_contain_text("◈ preassociato", timeout=T_LUNGO)
    b.ultime_azioni(i)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.linguetta("dettaglio")
    p.click("#du-escludi")
    ok = p.locator("#du-finestra #f-ok")
    expect(p.locator("#f-escludi-ar")).to_contain_text("ACM-1400_A.pdf")
    verifica("ACM-1400_ASSIEME.pdf" not in p.locator("#f-escludi-ar").inner_text(), "il documento confermato fra le associazioni da scegliere")
    expect(p.locator("#f-escludi-confermati")).to_contain_text("ACM-1400_ASSIEME.pdf")
    expect(p.locator("#f-escludi-pb")).to_contain_text("1 file preassociati vanno in «Da ricollocare»")
    verifica(p.locator("#du-finestra input[name=f-ar]:checked").count() == 0, "una scelta per le associazioni registrate è preselezionata")
    p.fill("#f-motivo", "il supporto arriva già montato")
    expect(ok).to_be_disabled()
    verifica("scegli come gestire le associazioni registrate" in (ok.get_attribute("title") or ""), "«Escludi» spento senza il motivo della scelta")
    p.check("#f-ar-mantieni")
    expect(ok).to_be_enabled()
    p.check("#f-ar-scollega")
    i = b.segna()
    b.ok_finestra()
    expect(p.locator("#nc-k-40")).to_have_attribute("data-du-stato", "escluso", timeout=T_LUNGO)
    az = b.ultime_azioni(i)

    def forma(a):
        d = a.get("dopo") or {}
        return (a["azione"], d.get("motivo") if a["azione"] == "file_svincolato" else d.get("op") or d.get("tipo"))
    # PE-22: le preassociazioni di un registrato escluso vanno nel vassoio con nodo_escluso (non nodo_tolto)
    verifica([forma(a) for a in az] == [("file_svincolato", "nodo_escluso"), ("nodo_modificato", "esclude"), ("decisione_aggiunta", "escludi"), ("decisione_aggiunta", "scollega_documento")],
             "«Escludi» con «Scollega alla conferma»: %s" % [forma(a) for a in az])
    es, sv = az[2]["dopo"], az[3]["dopo"]
    # PE-27: lo svincolo cita l'esclusione in da_decisione, al posto della convenzione .svN; il motivo è quello
    # dell'esclusione
    verifica(sv["proposta_id"] == b.pr["ACM-1400_A.pdf"] and sv.get("da_decisione") == es["id"] and ".sv" not in sv["id"] and sv["componente_id"] == es["componente_id"]
             and sv.get("motivo") == es["motivo"], "svincolo dell'esclusione: %s" % sv)
    # la chiave_bozza degli eventi di escludi e scollega_documento, tipi senza il campo: null, come la deduce il server
    # (EM §9.2, tabella delle forme; RPC PD-08)
    verifica(az[2]["chiave_bozza"] is None and az[3]["chiave_bozza"] is None and az[0]["chiave_bozza"] == "k-40" and az[1]["chiave_bozza"] == "k-40",
             "chiave_bozza degli eventi dell'esclusione: %s" % [(a["azione"], a["chiave_bozza"]) for a in az])
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    expect(p.locator("#el-k-40")).to_contain_text("⊘ da scollegare")
    expect(p.locator("#el-k-40")).to_contain_text("✓ confermato")
    expect(p.locator("#du-vassoio")).to_contain_text("NUOVO_DISEGNO.pdf")
    expect(p.locator("#du-vassoio [data-du-motivo=nodo_escluso]")).to_contain_text("pezzo escluso (nodo_escluso)")
    # un pezzo escluso non riceve preassociazioni (P-15): nel vassoio «Preassocia al nodo scelto» è spento con il motivo
    p.click("#el-k-40 .du-el-nodo")
    bt = p.locator("#du-ricolloca-" + sid(pid))
    expect(bt).to_be_disabled()
    verifica("escluso" in (bt.get_attribute("title") or ""), "«Preassocia al nodo scelto» su un pezzo escluso, senza il motivo: %r" % bt.get_attribute("title"))
    # riga 4, dopo una ricarica (la pila e il giornale riletti da R1): «↺ Ripristina» toglie l'esclusione e lo svincolo
    # che la cita, e il file torna preassociato con un evento esplicito
    p.reload()
    b.pronta()
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-40")
    b.linguetta("documenti")
    expect(p.locator("#du-doc-preassocia")).to_contain_text("Il pezzo è escluso")
    verifica(p.locator("#du-pannello-file").count() == 0, "il pannello di un pezzo escluso offre ancora «Preassocia»")
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-ripristina")
    expect(p.locator("#nc-k-40")).to_have_attribute("data-du-stato", "registrato", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["nodo_modificato", "decisione_tolta", "decisione_tolta", "file_preassociato"] and az[0]["dopo"] == {"op": "riusa"}
             and az[2]["prima"]["id"] == sv["id"] and az[3]["dopo"] == {"proposta_id": pid, "chiave_bozza": "k-40"}, "«Ripristina»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    expect(p.locator("#du-vassoio")).to_have_count(0)
    expect(p.locator("#el-k-40")).to_contain_text("● associato")
    expect(p.locator("#el-k-40")).to_contain_text("◈ preassociato")


@prova("37", "D1 nel wizard: «Lascia assieme», «Diventa assieme»; «Cambia tipo» (§4.3 riga 8); Sposta, Collega, Scollega dai dialoghi")
def b37(b):
    p = b.page
    b.vista("org")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    bl = p.locator('#du-wz-blocchi li[data-codice="D1_ULTIMO_FIGLIO"]')
    expect(bl).to_contain_text("ACM-1400", timeout=T_LUNGO)
    expect(p.locator("#du-wz-d1-particolare-k-40")).to_be_visible()
    i = b.segna()
    p.click("#du-wz-d1-resta-k-40")
    expect(bl).to_have_count(0, timeout=T_LUNGO)
    d = b.ultime_azioni(i)[0]["dopo"]
    verifica(d["tipo"] == "d1_resta_sottoassieme" and d["chiave_bozza"] == "k-40", "decisione D1: %s" % d)
    p.click("#du-wz-chiudi")
    # riga 8: un particolare che diventa commerciale, con l'avvertenza sul 2D
    b.scheda("k-41")
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-cambia-tipo")
    p.select_option("#f-tipo", "commerciale")
    expect(b.finestra()).to_contain_text("il suo 2D non è più richiesto, ma solo dopo la conferma")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("nodo_modificato", {"tipo": "commerciale"})], "«Cambia tipo»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # D1_PARTICOLARE_CON_FIGLI: un assieme diventato particolare che ha figli; «Diventa assieme»
    b.scheda("k-7")
    p.click("#du-cambia-tipo")
    p.select_option("#f-tipo", "sciolto")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.apri_wizard()
    p.click("#du-wz-fase2")
    bl2 = p.locator('#du-wz-blocchi li[data-codice="D1_PARTICOLARE_CON_FIGLI"]')
    expect(bl2).to_contain_text("ACM-1100", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-wz-d1-assieme-k-7")
    expect(bl2).to_have_count(0, timeout=T_LUNGO)
    d = b.ultime_azioni(i)[0]["dopo"]
    verifica(d["tipo"] == "d1_a_sottoassieme" and d["chiave_bozza"] == "k-7", "decisione D1: %s" % d)
    p.click("#du-wz-chiudi")
    # BZ-07, BZ-08, BZ-09 dai dialoghi del pannello
    b.scheda("k-9")
    i = b.segna()
    p.click("#du-sposta")
    p.select_option("#f-a", "k-40")
    p.fill("#f-qta", "2")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "arco_spostato" and az[0]["prima"]["padre"] == "k-7" and az[0]["dopo"] == {"padre": "k-40", "figlio": "k-9", "qta": 2, "origine": "utente"},
             "«Sposta sotto…»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.scheda("k-11")
    i = b.segna()
    p.click("#du-collega")
    p.select_option("#f-a", "k-40")
    p.fill("#f-qta", "1")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("arco_aggiunto", {"padre": "k-40", "figlio": "k-11", "qta": 1, "origine": "utente"})], "«Collega anche sotto…»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.scheda("k-11")
    i = b.segna()
    p.click("#du-scollega-padre")
    p.select_option("#f-da", "k-40")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_tolto"] and az[0]["prima"]["padre"] == "k-40", "«Scollega da un padre…»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


@prova("38", "PN-08: «Conferma l'identità proposta» rifiuta il P/N equivalente K4; «Adotta come codice» del codice corretto a mano")
def b38(b):
    p = b.page
    b.vista("org")
    b.scheda("k-41")
    b.linguetta("dettaglio")
    rif = p.locator('#du-sez-riferimenti li[data-du-riferimento="equivalente_k4"]')
    expect(rif).to_contain_text("400777")
    # «Adotta come codice…» del P/N K4: la lettura del codice della parte nel PDF della prova non è nel registro del banco,
    # quindi il gesto è spento con il motivo (e la pagina non va in errore cercandola)
    ad = rif.locator("button", has_text="Adotta come codice")
    expect(ad).to_be_disabled()
    verifica("PDF della prova" in (ad.get_attribute("title") or ""), "«Adotta come codice» del P/N K4 senza il motivo: %r" % ad.get_attribute("title"))
    bt = p.locator("#du-conferma-identita")
    expect(bt).to_be_enabled()
    i = b.segna()
    bt.click()
    expect(b.finestra()).to_contain_text("400777")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "il P/N del fornitore è un riferimento, non il codice del pezzo")
    b.ok_finestra()
    expect(p.locator("#nc-k-41")).to_have_attribute("data-du-stato", "decisa in bozza", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"]
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and dec["tipo"] == "identita" and dec["modo"] == "alternativa"
             and dec["chiave_lettura"].endswith("/l:#41") and dec["lettura"]["letterale"] == "ACM-1410" and dec["identita"]["codice"] == "ACM-1410"
             and dec["motivo"] and len(dec["importazione"]) == 64 and az[1]["dopo"]["identita"]["decisione"] == dec["id"]
             and az[0]["chiave_bozza"] == "k-41", "rifiuto del P/N K4: %s" % az)
    # la copia compatta della lettura come la rilegge il registro (R-7 della persistenza): i soli campi della copia, i
    # vuoti tolti anche dentro qualificatori e revisione (qui qualificatori {affissi: []} e revisione null: assenti)
    cl = dec["lettura"]
    verifica(set(cl) <= {"emittente", "hash_grammatica", "namespace", "letterale", "base", "qualificatori", "revisione"} and "qualificatori" not in cl and "revisione" not in cl
             and cl["base"] == {"originale": "ACM-1410", "normalizzata": "ACM-1410"} and cl["emittente"].startswith("cliente:") and cl["namespace"] == "acme"
             and len(cl["hash_grammatica"]) == 64, "copia compatta della lettura: %s" % cl)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    expect(p.locator("#du-conferma-identita")).to_be_disabled()
    verifica("già decisa" in (p.locator("#du-conferma-identita").get_attribute("title") or ""), "«Conferma l'identità proposta» spento senza il motivo")
    b.scheda("k-11")
    b.linguetta("dettaglio")
    li = p.locator("#du-sez-riferimenti li", has_text="codice corretto a mano")
    expect(li).to_contain_text("ACM-1125")
    i = b.segna()
    li.locator("button", has_text="Adotta come codice").click()
    p.fill("#f-motivo", "il codice corretto a mano dall'operatore è quello giusto")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"]
    verifica(dec["tipo"] == "identita" and dec["modo"] == "alternativa" and dec["riferimento"] == "manuale_legacy" and dec["identita"]["codice"] == "ACM-1125"
             and dec["identita"].get("rev") == "A", "«Adotta come codice» del codice manuale: %s" % dec)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


@prova("39", "«Includi di nuovo» (NI-07); una voce con un componente registrato no (PE-10); proposte «non richieste» di un nodo non incluso (§4.3 riga 5)")
def b39(b):
    p = b.page
    b.vista("org")
    noninc = p.locator('#du-vp .du-nc[data-du-stato="non incluso (bozza)"]')
    b.scheda("k-33")
    b.linguetta("dettaglio")
    p.click("#du-non-includere-nodo")
    p.fill("#f-motivo", "cordone di saldatura")
    b.ok_finestra()
    expect(noninc).to_have_count(1, timeout=T_LUNGO)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.click("#du-zoom-adatta")
    noninc.first.click()
    expect(p.locator("#du-includi")).to_be_enabled()
    i = b.segna()
    p.click("#du-includi")
    expect(noninc).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_tolta", "nodo_aggiunto", "arco_aggiunto"] and az[1]["dopo"]["chiave_bozza"] == "k-33"
             and az[1]["dopo"]["caselle"][0]["rif"].endswith(":#33") and az[2]["dopo"]["padre"] == "k-7" and az[2]["dopo"]["origine"] == "step", "«Includi di nuovo»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # una voce che adesso ha un componente registrato (decisione_esistente, risposta di R1 cambiata qui): «Includi di
    # nuovo» spento con il motivo, il browser non inventa l'identità registrata (PE-10)
    b.scheda("k-32")
    b.linguetta("dettaglio")
    p.click("#du-non-includere-nodo")
    p.fill("#f-motivo", "cordone di saldatura")
    b.ok_finestra()
    expect(noninc).to_have_count(1, timeout=T_LUNGO)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    letture_r1 = re.compile(r"/motore-a/bozza\?")

    def con_decisione(route):
        if route.request.method == "GET" and "importazione=1" in route.request.url:
            resp = route.fetch()
            j = resp.json()
            for v in j["importazione"]["contenuto"]["proposta"]["nodi"]:
                if v["origine"]["rif"].endswith(":#32"):
                    v["decisione_esistente"] = {"componente_id": "00000000-0000-4000-8000-000000009999", "codice": "ACM-1199", "rev": "A"}
            route.fulfill(response=resp, json=j)
        else:
            route.continue_()
    p.route(letture_r1, con_decisione)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        p.click("#du-zoom-adatta")
        noninc.first.click()
        expect(p.locator("#du-includi")).to_be_disabled()
        verifica("componente registrato" in (p.locator("#du-includi").get_attribute("title") or ""), "«Includi di nuovo» spento senza il motivo (PE-10)")
    finally:
        p.unroute(letture_r1, con_decisione)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    # riga 5: la proposta d'associazione di un nodo non incluso è «non richiesta: nodo non incluso (bozza)»
    b.scheda("k-7")
    b.linguetta("dettaglio")
    p.click("#du-non-includere-nodo")
    p.fill("#f-motivo", "prova della proposta non richiesta")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    riga = p.locator("#du-elenco [role=row][data-k^='ni:']", has_text="ACM-1100")
    expect(riga).to_contain_text("◇ non richiesta: nodo non incluso (bozza)")
    expect(riga).to_contain_text("ACM-1100_A.pdf")
    b.passo("sorgente")
    expect(p.locator("#du-file-" + sid(b.al["ACM-1100_A.pdf"]))).to_contain_text("non richiesta, nodo non incluso (bozza)")
    b.passo("revisione")
    b.vista("org")
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    expect(p.locator("#nc-k-7")).to_be_visible()


@prova("40", "letture e forme di R2 che non riescono (MAT §5.4): R1 e S1 in 5xx, annullamento_non_ammesso, i 422 di cambia_sorgente, la rete caduta, R3 409")
def b40(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    # R1 in 5xx: «Lettura non riuscita», nessun gesto di creazione (non è «nessuna bozza»)
    letture_r1 = re.compile(r"/motore-a/bozza\?")
    g = rispondi_json(500, {"esito": "da_verificare", "testo": "errore interno"}, metodo="GET", volte=100)
    b.passo("revisione")
    p.route(letture_r1, g)
    try:
        p.click("#du-aggiorna")
        expect(p.locator("#du-bozza-non-letta")).to_be_visible(timeout=T_LUNGO)
        verifica(p.locator("#du-da-registrato").count() == 0 and p.locator("#du-da-conferma").count() == 0 and p.locator("#du-vp").count() == 0,
                 "con R1 in 5xx la Revisione offre gesti di creazione o la tela")
        expect(p.locator("#du-passo-revisione")).to_contain_text("lettura non riuscita")
        expect(b.avviso()).to_contain_text("Lettura non riuscita")
    finally:
        p.unroute(letture_r1, g)
    p.click("#du-aggiorna")
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)
    # S1 in 5xx: l'elenco dei file non si mostra come attuale
    lettura_s1 = re.compile(r"/sorgente(\?|$)")
    g = rispondi_json(500, {"esito": "da_verificare", "testo": "errore interno"}, metodo="GET", volte=100)
    b.passo("sorgente")
    p.route(lettura_s1, g)
    try:
        p.click("#du-aggiorna")
        expect(p.locator("#du-file-non-letti")).to_be_visible(timeout=T_LUNGO)
        verifica(p.locator("#du-file li.du-fr").count() == 0, "con S1 in 5xx la pagina mostra l'elenco vecchio dei file")
        expect(p.locator("#du-step-non-letti")).to_be_visible()
    finally:
        p.unroute(lettura_s1, g)
    p.click("#du-aggiorna")
    expect(p.locator("#du-file li.du-fr").first).to_be_visible(timeout=T_LUNGO)
    # 422 annullamento_non_ammesso: il testo di MAT §5.4 (BZ-18); le modifiche restano da parte
    b.passo("revisione")
    b.vista("org")
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "CORPO_NON_VALIDO", "motivo": "annullamento_non_ammesso", "campo": "azioni", "testo": "annullamento oltre la versione del server"})
    p.route("**/motore-a/bozza", g)
    try:
        b.scheda("k-41")
        b.linguetta("dettaglio")
        p.click("#du-quantita")
        p.fill("#f-qta", "6")
        b.ok_finestra()
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "rifiutata", timeout=T_LUNGO)
        expect(p.locator("#du-conflitto-testo")).to_contain_text("Questo passo è prima di un aggiornamento fatto dal server")
    finally:
        p.unroute("**/motore-a/bozza", g)
    p.click("#du-rileggi-bozza")
    expect(p.locator("#du-da-parte")).to_be_visible(timeout=T_LUNGO)
    p.click("#du-scarta-da-parte")
    b.ok_finestra()
    expect(p.locator("#du-da-parte")).to_have_count(0)
    # i 422 di cambia_sorgente (risposte finte): gruppo_su_altra_occorrenza apre S6 sull'allegato della busta,
    # stessa_sorgente dice di usare «Aggiorna alla proposta di oggi», MOTIVO_OBBLIGATORIO lascia il dialogo aperto
    b.passo("sorgente")
    p.click("#du-guarda-" + sid(a1))
    expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    casi = [
        ({"esito": "rifiutato", "codice": "CORPO_NON_VALIDO", "motivo": "gruppo_su_altra_occorrenza", "allegato_id": a1, "testo": "gruppo su un'altra occorrenza"},
         "Questo gruppo sta su un'altra copia dello stesso file", True),
        ({"esito": "rifiutato", "codice": "CORPO_NON_VALIDO", "motivo": "stessa_sorgente", "testo": "stessa sorgente"}, "È la struttura della bozza", True),
        ({"esito": "rifiutato", "codice": "MOTIVO_OBBLIGATORIO", "campo": "cambia_sorgente.motivo", "testo": "Il cambio della fonte vuole il motivo."}, "MOTIVO_OBBLIGATORIO", False),
    ]
    for corpo, testo, chiude in casi:
        g = rispondi_json(422, corpo)
        p.route("**/motore-a/bozza", g)
        try:
            p.click("#du-cambia-sorgente")
            expect(p.locator("#f-effetto")).to_be_visible(timeout=T_LUNGO)
            p.fill("#f-motivo", "la BOM segue lo STEP del cliente")
            i = b.segna()
            b.ok_finestra()
            if chiude:
                expect(b.avviso()).to_contain_text(testo, timeout=T_LUNGO)
                expect(b.finestra()).to_be_hidden()
            else:
                expect(p.locator("#du-finestra-errore")).to_contain_text(testo, timeout=T_LUNGO)
                expect(b.finestra()).to_be_visible()
                b.chiudi_finestra()
            if corpo.get("motivo") == "gruppo_su_altra_occorrenza":
                b.attendi(lambda: [r for r in b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$") if a1 in r["path"]], "S6 non aperto sull'allegato della busta")
        finally:
            p.unroute("**/motore-a/bozza", g)
        expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    # la rete caduta su cambia_sorgente: «Esito da verificare», e il gesto ripetuto rimanda lo stesso corpo con la stessa
    # chiave_salvataggio (EM §9.2: se il primo era arrivato, «riproposto»)
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/motore-a/bozza", g)
    try:
        p.click("#du-cambia-sorgente")
        expect(p.locator("#f-effetto")).to_be_visible(timeout=T_LUNGO)
        p.fill("#f-motivo", "la BOM segue lo STEP del cliente")
        b.ok_finestra()
        expect(p.locator("#du-finestra-errore")).to_contain_text("Esito da verificare: ripeti lo stesso gesto", timeout=T_LUNGO)
        expect(b.finestra()).to_be_visible()
    finally:
        p.unroute("**/motore-a/bozza", g)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Fonte della bozza cambiata.", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(catturate) == 1 and len(c) == 1 and c[0]["chiave_salvataggio"] == catturate[0]["chiave_salvataggio"] and c[0] == catturate[0],
             "il gesto ripetuto dopo la rete caduta non rimanda lo stesso corpo con la stessa chiave")
    # R3 con 409 BOZZA_VERSIONE (bozza_chiusa, il reinvio dopo una risposta persa): la pagina rilegge R1 (MAT §5.4)
    b.passo("revisione")
    abbandona = re.compile(r"/abbandona$")
    g = rispondi_json(409, {"esito": "conflitto", "codice": "BOZZA_VERSIONE", "motivo": "bozza_chiusa", "testo": "La bozza è già chiusa."})
    p.route(abbandona, g)
    try:
        i = b.segna()
        p.click("#du-abbandona")
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("La bozza risulta già chiusa: la rileggo.", timeout=T_LUNGO)
        b.attendi(lambda: b.richieste_da(i, "GET", r"/motore-a/bozza$"), "R1 non riletto dopo il 409 di R3")
    finally:
        p.unroute(abbandona, g)
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)


@prova("41", "Sorgente: i 409 e i 422 dei gesti dei file (MAT §5.4), «Riattiva» di un archivio con le voci (FL-10), la coda degli atti (MAT §1.3)")
def b41(b):
    p = b.page
    zip2 = b.al["pacchetto2.zip"]
    v3, v4 = b.al["voce_3.pdf"], b.al["voce_4.pdf"]
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    # S3 409 ANTEPRIMA_SUPERATA (risposta finta): il dialogo si riapre con il dettaglio riletto, poi il gesto passa
    g = rispondi_json(409, {"esito": "conflitto", "codice": "ANTEPRIMA_SUPERATA", "anteprima": "voci_archivio", "testo": "Il perimetro delle voci è cambiato."})
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        i = b.segna()
        p.click("#du-altri-" + sid(zip2))
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("rivedi il dialogo aggiornato", timeout=T_LUNGO)
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS")
        b.attendi(lambda: len([r for r in b.richieste_da(i, "GET", r"/sorgente$") if "file=" in r["query"]]) >= 2, "il dettaglio di S1 non è riletto dopo il 409")
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1 and s3[0]["n_voci"] == 2 and len(s3[0]["impronta_voci"]) == 64, "S3 dell'archivio: %s" % s3)
    # S4 di un archivio: il dialogo dice le voci che tornano attive, S4 porta n_voci e impronta_voci del dettaglio (FL-10);
    # un 409 ANTEPRIMA_SUPERATA riapre il dialogo
    p.click("#du-cartella-altri")
    g = rispondi_json(409, {"esito": "conflitto", "codice": "ANTEPRIMA_SUPERATA", "anteprima": "voci_archivio", "testo": "Il perimetro delle voci è cambiato."})
    p.route("**/sorgente/file/*/riattiva", g)
    try:
        p.click("#du-riattiva-" + sid(zip2))
        expect(p.locator("#f-voci-riattiva")).to_contain_text("voce_3.pdf", timeout=T_LUNGO)
        expect(p.locator("#f-voci-riattiva")).to_contain_text("voce_4.pdf")
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("rivedi il dialogo aggiornato", timeout=T_LUNGO)
        expect(p.locator("#f-voci-riattiva")).to_be_visible()
    finally:
        p.unroute("**/sorgente/file/*/riattiva", g)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File riattivato", timeout=T_LUNGO)
    s4 = b.corpi(i, "POST", r"/riattiva$")
    verifica(len(s4) == 1 and s4[0]["n_voci"] == 2 and len(s4[0]["impronta_voci"]) == 64, "S4 dell'archivio senza il perimetro delle voci: %s" % s4)
    verifica(b.stato()["file"]["voce_3.pdf"]["stato"] == "attivo", "le voci dell'archivio non sono tornate attive")
    p.click("#du-cartella-attivi")
    # S3 409 FILE_VERSIONE (risposta finta): il dialogo si chiude e S1 si rilegge, con lo stato di prima e di adesso
    g = rispondi_json(409, {"esito": "conflitto", "codice": "FILE_VERSIONE", "stato": "altri_file", "versione": 9, "testo": "Lo stato del file è cambiato."})
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        p.click("#du-altri-" + sid(v4))
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
        i = b.segna()
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("Lo stato del file è cambiato (era attivo, ora altri_file): rileggo la Sorgente.", timeout=T_LUNGO)
        expect(b.finestra()).to_be_hidden()
        b.attendi(lambda: [r for r in b.richieste_da(i, "GET", r"/sorgente$") if "file=" not in r["query"]], "S1 non riletto dopo FILE_VERSIONE")
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    # S3 409 COPIA_IN_CORSO (risposta finta): esito certo; la striscia dice la copia in corso e «Riprova» usa la stessa chiave
    catturate = []
    g = rispondi_json(409, {"esito": "conflitto", "codice": "COPIA_IN_CORSO", "motivo": "copia_viva", "testo": "Una copia è già in corso."}, cattura=catturate)
    p.route("**/sorgente/file/*/altri-file", g)
    striscia = p.locator("#du-sospeso-" + sid("file:" + v4))
    try:
        p.click("#du-altri-" + sid(v4))
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("Una copia di conservazione è già in corso per questo file", timeout=T_LUNGO)
        expect(striscia).to_contain_text("una copia di conservazione è già in corso", timeout=T_LUNGO)
        verifica("non è ancora registrato" not in striscia.inner_text(), "la striscia di COPIA_IN_CORSO dice un esito incerto")
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    i = b.segna()
    p.click("#du-riprova-" + sid("file:" + v4))
    expect(b.avviso()).to_contain_text("File messo in Altri file", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1 and s3[0]["chiave_idempotenza"] == catturate[0]["chiave_idempotenza"], "«Riprova» dopo COPIA_IN_CORSO non riusa la chiave")
    expect(striscia).to_have_count(0)
    # S2 422 DIPENDENZE_ATTIVE e RISOLUZIONE_RICHIESTA (risposte finte): l'elenco voce per voce, il dialogo resta aperto
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "DIPENDENZE_ATTIVE", "testo": "Il file ha dipendenze attive.", "dipendenze": [
        {"allegato_id": v3, "codice": "D-6", "esito": "blocca", "testo": "è preassociato: togli prima la preassociazione e salva"},
        {"allegato_id": zip2, "codice": "D-6", "esito": "blocca", "testo": "è la fonte della bozza: cambia prima lo STEP"}]})
    p.route("**/sorgente/file/*/scarta", g)
    try:
        p.click("#du-scarta-" + sid(v3))
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        p.fill("#f-motivo", "doppione")
        b.ok_finestra()
        voci = p.locator("#f-dipendenze-attive li")
        expect(voci).to_have_count(2, timeout=T_LUNGO)
        expect(voci.nth(1)).to_contain_text("pacchetto2.zip")
        expect(b.finestra()).to_be_visible()
    finally:
        p.unroute("**/sorgente/file/*/scarta", g)
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "RISOLUZIONE_RICHIESTA", "testo": "Manca una risoluzione.", "occorrenze": []})
    p.route("**/sorgente/file/*/scarta", g)
    try:
        b.ok_finestra()
        expect(p.locator("#du-finestra-errore")).to_contain_text("Manca una dichiarazione richiesta dalle dipendenze", timeout=T_LUNGO)
        expect(b.finestra()).to_be_visible()
    finally:
        p.unroute("**/sorgente/file/*/scarta", g)
    b.chiudi_finestra()
    # S2 con la risposta persa e R6 «registrato_senza_esito» (risposta finta): l'evento c'è, il gesto è fatto
    gp = perdi_la_risposta()
    g6 = rispondi_json(200, {"stato": "registrato_senza_esito"}, metodo="GET", volte=5)
    p.route("**/sorgente/file/*/scarta", gp)
    p.route("**/motore-a/intento/*", g6)
    try:
        p.click("#du-scarta-" + sid(v3))
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        p.fill("#f-motivo", "doppione")
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("(l'esito di allora non è più conservato)", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/scarta", gp)
        p.unroute("**/motore-a/intento/*", g6)
    verifica(b.stato()["file"]["voce_3.pdf"]["stato"] == "scartato", "lo scarto non è nel banco")
    verifica(not [x for x in b.sospesi() if x[1]["risorsa"] == "file:" + v3], "la chiave resta in sospeso dopo «registrato_senza_esito»")
    # S7 409 ANTEPRIMA_SUPERATA (proposta) e 422 GIA_DOCUMENTO (risposte finte)
    a10 = b.al["ACM-1000_rev2.stp"]
    p.click("#du-guarda-" + sid(a10))
    expect(p.locator("#du-associa-step")).to_be_enabled(timeout=T_LUNGO)
    g = rispondi_json(409, {"esito": "conflitto", "codice": "ANTEPRIMA_SUPERATA", "anteprima": "proposta", "testo": "La proposta è cambiata."})
    p.route("**/sorgente/step/*/associa", g)
    try:
        p.click("#du-associa-step")
        expect(b.finestra()).to_contain_text("Associa lo STEP a ACM-1000")
        i = b.segna()
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("La proposta dello STEP è cambiata", timeout=T_LUNGO)
        expect(b.finestra()).to_be_hidden()
        b.attendi(lambda: b.richieste_da(i, "GET", r"/motore-a/dati$") and [r for r in b.richieste_da(i, "GET", r"/sorgente$")]
                  and b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$"), "dopo il 409 di S7 mancano le riletture di modello, S1 e S6")
    finally:
        p.unroute("**/sorgente/step/*/associa", g)
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "GIA_DOCUMENTO", "testo": "Già documento."})
    p.route("**/sorgente/step/*/associa", g)
    try:
        p.click("#du-associa-step")
        b.ok_finestra()
        expect(p.locator("#du-finestra-errore")).to_contain_text("È già un documento confermato: per cambiarlo serve una revisione o una sostituzione (non in V1).", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/step/*/associa", g)
    b.chiudi_finestra()
    # E1′: 422 NESSUN_FILE_CARICATO con i motivi file per file; 413 (risposta finta) «troppo grande»
    esiti = p.locator("#du-carica-esiti")
    p.set_input_files("#du-carica-file", files=[{"name": "solo.exe", "mimeType": "application/octet-stream", "buffer": b"MZ sintetico"}])
    p.click("#du-carica-invia")
    expect(esiti.locator('li[data-esito="rifiutato"]')).to_contain_text("tipo di file non ammesso", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("Nessun file è stato caricato")
    g = rispondi_json(413, {"esito": "rifiutato", "codice": "TROPPO_GRANDE", "testo": "troppo grande"})
    p.route("**/thread/*/carica", g)
    try:
        p.set_input_files("#du-carica-file", files=[{"name": "GRANDE.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% sintetico\n"}])
        p.click("#du-carica-invia")
        expect(b.avviso()).to_contain_text("Troppo grande: dividi in più invii.", timeout=T_LUNGO)
    finally:
        p.unroute("**/thread/*/carica", g)
    # gli atti in coda (MAT §1.3): «Rifai l'analisi» chiesto mentre «Prepara i file» è in volo parte dopo, non si perde
    b.ritardo("POST /thread/{id}/fascicolo/prepara", 2500)
    i = b.segna()
    p.click("#du-menu-rfq summary")
    p.click("#du-prepara")
    p.click("#du-menu-rfq summary")
    p.click("#du-rianalizza")
    expect(b.avviso()).to_contain_text("in coda dopo l'atto in corso")
    expect(b.avviso()).to_contain_text("Rianalisi accodata", timeout=T_LUNGO)
    pr = b.richieste_da(i, "POST", r"/fascicolo/prepara$")
    ri = b.richieste_da(i, "POST", r"/fascicolo/rianalizza$")
    verifica(len(pr) == 1 and len(ri) == 1 and b.richieste.index(pr[0]) < b.richieste.index(ri[0]), "gli atti in coda: %s" % [(r["metodo"], r["path"]) for r in b.richieste_da(i, "POST")])


@prova("42", "rebase con la base superata (BZ-23); BOM congelata (TS-11): fascia, «Chiedi una revisione autorizzata…» spento, modifiche spente")
def b42(b):
    p = b.page
    b.interruttore("base-superata", True)
    b.passo("revisione")
    b.vista("org")
    p.click("#du-aggiorna")
    expect(p.locator("#du-base-superata")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica("la base è superata" in (p.locator("#du-cta-bom").get_attribute("title") or ""), "il CTA con la base superata senza il motivo")
    i = b.segna()
    p.click("#du-rebase")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "la conferma di un collega ha cambiato la base")
    b.ok_finestra()
    expect(p.locator("#du-base-superata")).to_have_count(0, timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["rebase"]["motivo"] and "base" in c["rebase"] and [a["azione"] for a in c["azioni"]] == ["rebase"], "il salvataggio del rebase: %s" % c)
    b.interruttore("congelata", True)
    p.click("#du-aggiorna")
    fascia = p.locator("#du-congelata")
    expect(fascia).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-chiedi-revisione")).to_be_disabled()
    expect(fascia).to_contain_text("Revisione e congelamento non sono in questa pagina")
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    verifica("congelata" in (p.locator("#du-nuovo-ass").get_attribute("title") or ""), "gesto spento senza il motivo della BOM congelata")
    expect(p.locator("#du-abbandona")).to_be_disabled()
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    expect(p.locator("#du-stato-conferma")).to_contain_text("BOM congelata")
    b.interruttore("congelata", False)
    p.click("#du-aggiorna")
    expect(fascia).to_have_count(0, timeout=T_LUNGO)
    expect(p.locator("#du-nuovo-ass")).to_be_enabled()


@prova("43", "pannello: linguette, documenti del nodo e azioni non in V1 (PN-02, §6.1); «da ricodificare»; NI-06; linguette dei file; una candidata incerta (SG-02)")
def b43(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-40")
    b.linguetta("documenti")
    doc = p.locator("#du-doc-nodo")
    expect(doc).to_contain_text("● associato")
    expect(doc).to_contain_text("ACM-1400_A.pdf")
    expect(doc).to_contain_text("✓ confermato")
    nv = p.locator("#du-doc-non-v1")
    for testo in ("Conferma il documento", "Conferma 2D", "Correggi il codice di un file", "Deroga", "Revoca la deroga", "Sostituisci il principale con questo"):
        expect(nv.locator("button", has_text=testo).first).to_be_disabled()
    for motivo in ("I documenti si confermano solo con «Rivedi e conferma fascicolo» (V3)", "R10 (V4)", "Le deroghe non sono in questa pagina", "La sostituzione dei documenti non è in V1"):
        expect(nv).to_contain_text(motivo)
    expect(p.locator("#du-completezza")).to_contain_text("2D previsti all'importazione")
    # il visore dalla linguetta dei documenti (PN-04); un contenuto che non c'è (410, risposta finta) lo dice
    a11 = b.al["ACM-1400_A.pdf"]
    p.click("#du-doc-apri-" + sid(a11))
    expect(p.locator("#du-visore-tela canvas")).to_be_visible(timeout=T_LUNGO)
    b.chiudi_finestra()
    g = rispondi_json(410, {"esito": "rifiutato", "codice": "BYTE_NON_DISPONIBILI", "testo": "I byte non ci sono."}, metodo="GET", volte=5)
    p.route("**/sorgente/file/*/contenuto*", g)
    try:
        p.click("#du-doc-apri-" + sid(a11))
        expect(p.locator("#du-visore-errore")).to_contain_text("Contenuto non disponibile: il file è scartato o i suoi byte non ci sono più", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/contenuto*", g)
    b.chiudi_finestra()
    # «da ricodificare»: una ricodifica del registrato fa seguire il codice nuovo ai suoi file (MAT §4.3 riga 7)
    b.linguetta("dettaglio")
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1401")
    p.fill("#f-motivo", "codice nuovo del supporto")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    expect(p.locator("#el-k-40")).to_contain_text("da ricodificare")
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    expect(p.locator("#du-file-" + sid(b.al["ACM-1400_A.pdf"]))).to_contain_text("da ricodificare")
    b.passo("revisione")
    b.vista("org")
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # NI-06: la radice e un nodo con un componente hanno la casella spenta, con il motivo
    expect(p.locator("#sel-k-root")).to_be_disabled()
    verifica("è il prodotto" in (p.locator("#sel-k-root").get_attribute("title") or ""), "la casella della radice senza il motivo")
    expect(p.locator("#sel-k-40")).to_be_disabled()
    verifica("ha un componente" in (p.locator("#sel-k-40").get_attribute("title") or ""), "la casella di un registrato senza il motivo")
    # le linguette dei file della Sorgente (MAT §3.3), con data, autore e motivo in Altri file (FL-08)
    b.passo("sorgente")
    for k in ("attivi", "associati", "senza", "ricollocare", "altri", "scartati"):
        expect(p.locator("#du-cartella-" + k)).to_be_visible()
    p.click("#du-cartella-associati")
    expect(p.locator("#du-file")).to_contain_text("ACM-1400_A.pdf")
    p.click("#du-cartella-senza")
    verifica("ACM-1400_A.pdf" not in p.locator("#du-file .du-fl").inner_text(), "un file associato fra quelli senza associazione")
    p.click("#du-cartella-altri")
    expect(p.locator("#du-file-" + sid(b.al["voce_4.pdf"]))).to_contain_text("In Altri file da AD")
    p.click("#du-cartella-attivi")
    # «Gestisci nella Sorgente» dall'Elenco (MAT §3.6)
    b.passo("revisione")
    b.vista("elenco")
    p.click("#du-gestisci-sorgente")
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    b.vista("org")
    # una sola candidata incerta: niente in vista all'apertura, ma la si sceglie e la si usa (SG-02)
    a1 = b.al["ACM-1000_assieme.stp"]
    b.interruttore("s6-incerta", True)
    try:
        p.click("#du-aggiorna")
        expect(p.locator("#du-step-" + sid(a1))).to_contain_text("incerta", timeout=T_LUNGO)
        p.click("#du-guarda-" + sid(a1))
        expect(p.locator("#du-nessuna-in-vista")).to_be_visible(timeout=T_LUNGO)
        scelte = p.locator("#du-strutture button")
        expect(scelte).to_have_count(1)
        expect(scelte).to_have_attribute("aria-pressed", "false")
        verifica(p.locator("#du-cambia-sorgente").count() == 0 and p.locator("#du-riallinea-da-step").count() == 0 and p.locator("#du-anteprima .du-vp").count() == 0,
                 "una candidata incerta in vista all'apertura")
        scelte.click()
        expect(p.locator("#du-anteprima .du-vp")).to_be_visible(timeout=T_LUNGO)
        expect(p.locator("#du-riallinea-da-step, #du-cambia-sorgente").first).to_be_visible()
    finally:
        b.interruttore("s6-incerta", False)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)


@prova("44", "401 durante un salvataggio: si resta sulla pagina, «Riprova ora» dopo il login con la stessa chiave_salvataggio (MAT §5.4)")
def b44(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    catturate = []
    g = rispondi_json(401, {"esito": "rifiutato", "testo": "login"}, cattura=catturate, intestazioni={"HX-Redirect": "/login"})
    b.consenti_beforeunload = True
    p.route("**/motore-a/bozza", g)
    s = p.locator("#du-salvataggio")
    try:
        b.scheda("k-41")
        b.linguetta("dettaglio")
        p.click("#du-quantita")
        p.fill("#f-qta", "8")
        b.ok_finestra()
        expect(s).to_have_attribute("data-du-salvataggio", "non_arrivato", timeout=T_LUNGO)
        expect(p.locator("#du-salvataggio-testo")).to_contain_text("Sessione scaduta")
        expect(p.locator("#du-riprova-salvataggio")).to_be_visible()
    finally:
        p.unroute("**/motore-a/bozza", g)
        b.consenti_beforeunload = False
    verifica(b.beforeunload >= 1, "la pagina non ha chiesto conferma prima di uscire con un salvataggio in sospeso")
    i = b.segna()
    p.click("#du-riprova-salvataggio")
    expect(s).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(catturate) == 1 and len(c) == 1 and c[0] == catturate[0], "«Riprova ora» dopo il login non rimanda lo stesso corpo con la stessa chiave_salvataggio")


@prova("45", "«Verifica ora» di un gesto dei file in sospeso rilegge S1 e il modello, non butta le modifiche della bozza in volo (MAT §5.5 n. 7)")
def b45(b):
    p = b.page
    nd = b.allegato_di("NUOVO_DISEGNO.pdf")
    risorsa = "file:" + nd
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    gp = perdi_la_risposta()
    g6 = interrompi("GET")
    p.route("**/sorgente/file/*/altri-file", gp)
    p.route("**/motore-a/intento/*", g6)
    try:
        p.click("#du-altri-" + sid(nd))
        expect(b.finestra()).to_contain_text("Non è la cartella ALTRO del NAS", timeout=T_LUNGO)
        b.ok_finestra()
        expect(b.avviso()).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/altri-file", gp)
    # la pagina ricaricata con R6 che non risponde: il tentativo resta, e nessuna verifica automatica è in giro
    p.reload()
    b.pronta()
    striscia = p.locator("#du-sospeso-" + sid(risorsa))
    expect(striscia).to_contain_text("non ha un esito certo", timeout=T_LUNGO)
    # una modifica della bozza con il salvataggio lento: R2 è in volo mentre si verifica il gesto del file
    b.passo("revisione")
    b.vista("org")
    n_r2 = b.stato()["conti"]["R2"]
    b.ritardo("POST /thread/{id}/motore-a/bozza", 3000)
    b.scheda("k-41")
    b.linguetta("dettaglio")
    p.click("#du-quantita")
    p.fill("#f-qta", "7")
    b.ok_finestra()
    pillola = p.locator("#oq-k-40_k-41")
    expect(pillola).to_contain_text("×7")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "in_corso", timeout=T_LUNGO)
    p.unroute("**/motore-a/intento/*", g6)
    p.click("#du-verifica-" + sid(risorsa))
    expect(b.avviso()).to_contain_text("Verificato", timeout=T_LUNGO)
    expect(pillola).to_contain_text("×7")
    expect(striscia).to_have_count(0)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    expect(pillola).to_contain_text("×7")
    verifica(b.stato()["conti"]["R2"] == n_r2 + 1, "la modifica della bozza non è stata salvata una volta")
    verifica(b.stato()["file"]["NUOVO_DISEGNO.pdf"]["stato"] == "altri_file", "il gesto sul file non è nel banco")


@prova("46", "con una conferma in sospeso «Aggiorna alla proposta di oggi…», «Abbandona la bozza…» e i gesti sono spenti con il motivo (MAT §5.2)")
def b46(b):
    p = b.page
    b.proposta_cambia()
    b.passo("revisione")
    b.vista("org")
    p.click("#du-aggiorna")
    expect(p.locator("#du-riallinea")).to_be_enabled(timeout=T_LUNGO)
    # un tentativo di R5 in sospeso nella memoria della scheda, come lo trova la pagina dopo una ricarica
    chiave = "cockpit.du.sospeso.%s.%s.bozza:%s" % (b.dati["utente"], b.thread, b.p1)
    p.evaluate("([k, v]) => sessionStorage.setItem(k, JSON.stringify(v))", [chiave, {"risorsa": "bozza:" + b.p1, "nome": "confermaBom", "chiave": "00000000-0000-4000-8000-0000000e0f01",
                                                                                     "impronta": "x", "verificato": True, "quando": "2026-10-18T08:00:00Z", "etichetta": "Conferma BOM"}])
    p.click("#du-aggiorna")
    expect(b.striscia_sospesa()).to_be_visible(timeout=T_LUNGO)
    for sel in ("#du-riallinea", "#du-abbandona", "#du-nuovo-ass"):
        expect(p.locator(sel)).to_be_disabled()
        verifica("in sospeso" in (p.locator(sel).get_attribute("title") or ""), "%s spento senza il motivo della conferma in sospeso" % sel)
    b.striscia_sospesa().locator("button", has_text="Abbandona il tentativo").click()
    b.ok_finestra()
    expect(p.locator("#du-riallinea")).to_be_enabled(timeout=T_LUNGO)
    expect(p.locator("#du-abbandona")).to_be_enabled()
    verifica(not b.sospesi(), "il tentativo abbandonato resta nella memoria della scheda")


@prova("47", "«Converti…» (BZ-22), mono_pezzo nel wizard (BZ-14), R5 da verificare con il wizard chiuso; WZ-01 con la partenza «registrato» (PE-23)")
def b47(b):
    p = b.page
    b.passo("revisione")
    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    # una bozza del formato di prima: la striscia, «Converti…» con il motivo (converti_da), e l'abbandono ammesso
    b.bozza_formato2()
    p.click("#du-aggiorna")
    expect(p.locator("#du-formato-vecchio")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-abbandona-f2")).to_be_enabled()
    i = b.segna()
    p.click("#du-converti")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "la bozza va lavorata nel formato nuovo")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza convertita.", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["bozza_id"] is None and c["versione_attesa"] == 0 and c["converti_da"]["versione_attesa"] == 3 and c["converti_da"]["motivo"]
             and "snapshot" not in c and "azioni" not in c, "corpo della conversione: %s" % c)
    b.salvata(1)
    # mono_pezzo: il prodotto senza figli (avviso), il motivo obbligatorio, una decisione salvata e il riepilogo riletto
    b.vista("org")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-avvisi")).to_contain_text("PRODOTTO_SENZA_FIGLI", timeout=T_LUNGO)
    p.locator("#du-wz-corpo button", has_text="Dichiara mono_pezzo").click()
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.fill("#f-motivo", "è un pezzo unico")
    i = b.segna()
    b.ok_finestra()
    d = b.ultime_azioni(i)[0]["dopo"]
    verifica(d["tipo"] == "mono_pezzo" and d["motivo"], "decisione mono_pezzo: %s" % d)
    expect(p.locator("#du-wz-avvisi")).not_to_contain_text("PRODOTTO_SENZA_FIGLI", timeout=T_LUNGO)
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-stato-conferma")).to_contain_text("mono_pezzo proposto in bozza")
    # R5 «da verificare» e il wizard chiuso: la verifica con R6 va avanti da sola (MAT §5.5 n. 2)
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    b.modo("R5", "500_prima")
    p.click("#du-wz-conferma")
    expect(b.esito_wizard()).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()
    expect(b.avviso()).to_contain_text("non è ancora registrata", timeout=T_LUNGO)
    expect(b.striscia_sospesa()).to_contain_text("il server dice che non è ancora registrato", timeout=T_LUNGO)
    b.striscia_sospesa().locator("button", has_text="Abbandona il tentativo").click()
    b.ok_finestra()
    expect(b.striscia_sospesa()).to_have_count(0)
    # WZ-01 con la partenza «registrato» (decisioni dell'ondata A, §5 n. 1 e PE-23): il prodotto ha conferme formato 3,
    # quindi «Parti dalla BOM registrata» non si offre (spento, con il motivo scritto accanto); prima la pagina la offriva
    # e il CTA restava acceso, e una conferma n+1 uguale alla n passava
    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    reg = p.locator("#du-da-registrato")
    expect(reg).to_be_disabled()
    verifica("conferma del formato 3" in (reg.get_attribute("title") or ""), "«Parti dalla BOM registrata» spento senza il motivo: %r" % reg.get_attribute("title"))
    expect(p.locator("#du-nessuna-bozza")).to_contain_text("il prodotto ha già una conferma del formato 3")
    expect(p.locator("#du-da-conferma")).to_be_enabled()
    # una bozza «registrato» aperta altrove sullo stesso prodotto (la pagina nuova non la offre, ma una bozza così può
    # esserci): la base è l'ultima conferma valida formato 3 e non ci sono gesti, quindi CTA e «Conferma BOM» spenti
    n_r5 = b.stato()["conti"].get("R5", 0)
    b.sonda_post("bozza-registrato")
    p.click("#du-aggiorna")
    b.salvata(1)
    cta = p.locator("#du-cta-bom")
    expect(cta).to_be_disabled()
    verifica("nessuna modifica: la v3 è già confermata" in (cta.get_attribute("title") or ""), "CTA della bozza «registrato» senza modifiche: %r" % cta.get_attribute("title"))
    verifica(b.stato()["conti"].get("R5", 0) == n_r5, "una conferma registrata dalla bozza «registrato» senza modifiche")


def tab_prodotto(b, prodotto):
    return b.page.locator('.du-ptab[data-prodotto="%s"]' % prodotto)


def conflitto_e_rileggi(b, descrizione):
    """Un gesto sul nodo del prodotto (la descrizione) con il salvataggio in conflitto (409), poi «Rileggi la bozza
    salvata»: la modifica va da parte."""
    p = b.page
    b.modo("R2", "409")
    b.scheda("k-root")
    b.linguetta("dettaglio")
    p.click("#du-correggi")
    p.fill("#f-desc", descrizione)
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "conflitto", timeout=T_LUNGO)
    p.click("#du-rileggi-bozza")
    expect(p.locator("#du-da-parte")).to_be_visible(timeout=T_LUNGO)


@prova("48", "modifiche tenute da parte: il prodotto non cambia senza una scelta, «Riapplica» su un'altra bozza chiede conferma, l'uscita chiede conferma (§5 n. 2)")
def b48(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    n_r2 = b.stato()["conti"]["R2"]
    conflitto_e_rileggi(b, "Confezionatrice MX-1, rivista")
    dp = p.locator("#du-da-parte")
    expect(dp).to_contain_text("di ACM-1000 tenute da parte")
    # il cambio di prodotto: la scelta si fa prima (decisioni dell'ondata A, §5 n. 2); «Resta» lascia tutto com'era
    tab_prodotto(b, b.p2).click()
    expect(b.finestra()).to_contain_text("Prima di passare a ACM-2000", timeout=T_LUNGO)
    expect(p.locator("#f-da-parte-cambio")).to_contain_text("corretto")
    b.finestra().locator("button", has_text="Resta su ACM-1000").click()
    expect(b.finestra()).to_be_hidden()
    expect(tab_prodotto(b, b.p1)).to_have_attribute("aria-current", "true")
    expect(dp).to_be_visible()
    expect(p.locator("#du-riapplica")).to_be_enabled()
    verifica(b.stato()["conti"]["R2"] == n_r2, "il conflitto o la scelta del prodotto hanno scritto la bozza")
    # uscire dalla pagina con modifiche tenute da parte chiede conferma (beforeunload): si resta, e le modifiche ci sono
    n_bu = b.beforeunload
    b.consenti_beforeunload = True
    try:
        p.evaluate("() => { location.href = location.href; }")
        b.attendi(lambda: b.beforeunload > n_bu, "uscendo con le modifiche tenute da parte la pagina non chiede conferma")
    finally:
        b.consenti_beforeunload = False
    expect(dp).to_be_visible()
    # «Scartale e passa a ACM-2000»: le modifiche si perdono, il prodotto cambia, e niente arriva alla bozza del secondo
    tab_prodotto(b, b.p2).click()
    expect(b.finestra()).to_contain_text("Prima di passare a ACM-2000", timeout=T_LUNGO)
    i = b.segna()
    b.ok_finestra()
    expect(tab_prodotto(b, b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    expect(dp).to_have_count(0)
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "le modifiche di ACM-1000 sono andate a un salvataggio dopo il cambio di prodotto")
    tab_prodotto(b, b.p1).click()
    expect(tab_prodotto(b, b.p1)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.passo("revisione")
    b.vista("org")
    # la bozza chiusa e riaperta altrove mentre le modifiche sono da parte: «Riapplica» chiede una conferma esplicita,
    # con l'elenco, e solo dopo le manda alla bozza di adesso
    conflitto_e_rileggi(b, "Confezionatrice MX-1, seconda prova")
    vecchia = b.stato()["bozze"][b.p1]["bozza_id"]
    b.sonda_post("sostituisci-bozza")
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    nuova = b.stato()["bozze"][b.p1]["bozza_id"]
    verifica(nuova != vecchia, "la sonda non ha sostituito la bozza")
    expect(dp).to_be_visible()
    i = b.segna()
    p.click("#du-riapplica")
    expect(b.finestra()).to_contain_text("Riapplicare a un'altra bozza?", timeout=T_LUNGO)
    expect(p.locator("#f-da-parte-altra")).to_contain_text("corretto")
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "«Riapplica» su un'altra bozza è partito senza la conferma")
    b.ok_finestra()
    b.salvata(2)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(c) == 1 and c[0]["bozza_id"] == nuova and c[0]["versione_attesa"] == 1 and [a["azione"] for a in c[0]["azioni"]] == ["nodo_modificato"]
             and c[0]["azioni"][0]["dopo"].get("descrizione") == "Confezionatrice MX-1, seconda prova", "la riapplicazione sulla bozza di adesso: %s" % c)
    expect(dp).to_have_count(0)


@prova("52", "PE-23: base formato 2 (la prima conferma formato 3 è ammessa), conferma dalla fase 2 senza rilettura e senza errori, «registrato» non offerto, uguale_alla_base di R4, NESSUNA_MODIFICA di R5")
def b52(b):
    p = b.page
    # il secondo prodotto ha una sola conferma, formato 2 (della pagina vecchia)
    b.sonda_post("conferma-formato2")
    tab_prodotto(b, b.p2).click()
    expect(tab_prodotto(b, b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-da-registrato")).to_be_enabled()
    expect(p.locator("#du-da-conferma")).to_be_enabled()
    # «Modifica la BOM confermata» su una base formato 2: senza modifiche il CTA è acceso, la prima conferma formato 3
    # aggiunge identità e caselle
    p.click("#du-da-conferma")
    b.salvata(1)
    b.vista("org")
    expect(p.locator("#du-cta-bom")).to_be_enabled()
    # la conferma dalla fase 2 con l'avviso PRODOTTO_SENZA_FIGLI e le letture di dopo che non rispondono: nessun errore
    # JavaScript (la fase 2 leggeva lo snapshot della bozza, che dopo la conferma non c'è più), «la pagina non si è
    # riletta» nel wizard, e sotto il wizard niente bozza
    errori = []
    ascolta = lambda e: errori.append(str(e))  # noqa: E731
    p.on("pageerror", ascolta)
    try:
        b.apri_wizard()
        p.click("#du-wz-fase2")
        expect(p.locator("#du-wz-avvisi")).to_contain_text("PRODOTTO_SENZA_FIGLI", timeout=T_LUNGO)
        expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
        letture = re.compile(r"/motore-a/(dati|bozza|conferme)(\?|$)")
        g = rispondi_json(500, {"esito": "errore", "testo": "errore interno"}, metodo="GET", volte=100)
        p.route(letture, g)
        try:
            p.click("#du-wz-conferma")
            e = b.esito_wizard()
            expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
            expect(e).to_contain_text("Conferma n. 2 di ACM-2000")
            expect(p.locator("#du-wz-non-riletta")).to_contain_text("La conferma n. 2 è registrata; la pagina non si è riletta", timeout=T_LUNGO)
        finally:
            p.unroute(letture, g)
        p.click("#du-wz-rileggi")
        expect(p.locator("#du-wz-non-riletta")).to_have_count(0, timeout=T_LUNGO)
        expect(p.locator("#du-wizard-titolo")).to_contain_text("ACM-2000")
        verifica(not errori, "errori JavaScript dopo la conferma dalla fase 2: %s" % errori)
        p.click("#du-wz-chiudi")
        expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    finally:
        p.remove_listener("pageerror", ascolta)
    # adesso il secondo prodotto ha una conferma formato 3: «Parti dalla BOM registrata» non si offre, e la bozza dalla
    # conferma senza modifiche ha il CTA spento
    expect(p.locator("#du-da-registrato")).to_be_disabled()
    p.click("#du-da-conferma")
    b.salvata(1)
    cta = p.locator("#du-cta-bom")
    expect(cta).to_be_disabled()
    verifica("nessuna modifica: la v2 è già confermata" in (cta.get_attribute("title") or ""), "CTA sulla conferma formato 3 senza modifiche: %r" % cta.get_attribute("title"))
    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    tab_prodotto(b, b.p1).click()
    expect(tab_prodotto(b, b.p1)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    # uguale_alla_base di R4 (il segno del server dal passo 6; nel banco con l'interruttore): R4 decide anche quando il
    # giornale non basta, cioè dopo un annullamento netto
    b.interruttore("uguale-alla-base", True)
    try:
        b.passo("revisione")
        b.vista("org")
        p.click("#du-abbandona")
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
        p.click("#du-da-conferma")
        b.salvata(1)
        expect(cta).to_be_disabled()
        b.scheda("k-40")
        b.linguetta("dettaglio")
        p.click("#du-quantita")
        p.fill("#f-qta", "3")
        b.ok_finestra()
        b.salvata(2)
        # R4 in sottofondo (i conti della barra) dice uguale_alla_base falso: il CTA è acceso
        expect(p.locator("#du-conti")).to_contain_text("blocchi", timeout=T_LUNGO)
        expect(cta).to_be_enabled()
        p.click("#du-annulla")
        b.salvata(3)
        # l'annullamento netto: il giornale ha gli eventi, ma R4 dice che la bozza è uguale alla base
        expect(cta).to_be_disabled(timeout=T_LUNGO)
        expect(p.locator("#du-conti")).to_contain_text("blocchi", timeout=T_LUNGO)
        verifica("nessuna modifica: la v3 è già confermata" in (cta.get_attribute("title") or ""), "CTA dopo l'annullamento netto: %r" % cta.get_attribute("title"))
        # R5 risponde 422 NESSUNA_MODIFICA (risposta finta: con una modifica il banco non lo direbbe): l'esito lo dice,
        # niente resta in sospeso, «Conferma BOM» si spegne
        p.click("#du-ripeti")
        b.salvata(4)
        b.attendi(lambda: cta.is_enabled(), "dopo «Ripeti» il CTA non si accende")
        b.apri_wizard()
        p.click("#du-wz-fase2")
        ok = p.locator("#du-wz-conferma")
        expect(ok).to_be_enabled(timeout=T_LUNGO)
        g = rispondi_json(422, {"esito": "rifiutato", "codice": "NESSUNA_MODIFICA", "testo": "La bozza è uguale alla conferma di base."})
        p.route("**/motore-a/bom/conferma", g)
        try:
            ok.click()
            e = b.esito_wizard()
            expect(e).to_have_attribute("data-codice", "NESSUNA_MODIFICA", timeout=T_LUNGO)
            expect(e).to_contain_text("La bozza è uguale alla conferma n. 3: non c'è niente da confermare")
            expect(e).to_contain_text("niente è registrato")
            expect(ok).to_be_disabled()
            verifica("nessuna modifica rispetto alla conferma di base" in (ok.get_attribute("title") or ""), "«Conferma BOM» dopo NESSUNA_MODIFICA: %r" % ok.get_attribute("title"))
            verifica(not b.sospesi(), "NESSUNA_MODIFICA ha lasciato la chiave in sospeso")
        finally:
            p.unroute("**/motore-a/bom/conferma", g)
        p.click("#du-wz-chiudi")
    finally:
        b.interruttore("uguale-alla-base", False)


@prova("54", "P-6: il nodo del prodotto con il segnale LS-02 non si «Non include»: casella spenta, «Seleziona tutti i segnalati» lo salta, nessun «Non includere» nel pannello")
def b54(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    b.interruttore("radice-segnalata", True)
    try:
        # una bozza nuova dalla proposta dello STEP, con il segnale anche sulla voce della radice
        b.passo("sorgente")
        a1 = b.al["ACM-1000_assieme.stp"]
        p.click("#du-guarda-" + sid(a1))
        expect(p.locator("#du-usa-bom")).to_be_enabled(timeout=T_LUNGO)
        p.click("#du-usa-bom")
        b.ok_finestra()
        expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
        b.salvata(1)
        b.vista("org")
        expect(p.locator("#nc-k-root")).to_contain_text("possibile elemento CAD")
        verifica(p.locator("#sel-k-root").is_disabled(), "la scheda del prodotto ha la casella della selezione accesa")
        p.click("#du-segnalati-apri")
        riga = p.locator('#du-segnalati .du-segnalato[data-k="k-root"]')
        expect(riga).to_contain_text("non selezionabile: è il prodotto")
        expect(riga.locator("input")).to_be_disabled()
        p.click("#du-seleziona-segnalati")
        expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
        verifica(p.locator("#nc-k-root.du-nc--selezionato").count() == 0, "«Seleziona tutti i segnalati» ha preso il nodo del prodotto")
        p.click("#du-non-includere")
        verifica(p.locator('#f-elenco input[value="k-root"]').count() == 0, "il dialogo «Non includere» elenca il nodo del prodotto")
        b.chiudi_finestra()
        p.click("#du-svuota-selezione")
        b.scheda("k-root")
        b.linguetta("dettaglio")
        verifica(p.locator("#du-non-includere-nodo").count() == 0, "il pannello del prodotto offre «Non includere nella BOM…»")
        p.click("#du-segnalati-apri")
    finally:
        b.interruttore("radice-segnalata", False)


C40 = "00000000-0000-4000-8000-000000004000"


@prova("55", "P-7 e PE-09: «Cambia tipo» di un componente registrato legge l'effetto con la variante di R1 e scrive cambia_tipo{componente_id, tipo_nuovo, firma_effetto}")
def b55(b):
    p = b.page
    b.vista("org")
    b.scheda("k-40")
    b.linguetta("dettaglio")
    expect(p.locator("#du-cambia-tipo")).to_be_enabled()
    i = b.segna()
    p.click("#du-cambia-tipo")
    ok = p.locator("#du-finestra #f-ok")
    expect(ok).to_be_disabled()
    # un commerciale non si fa: lo dice l'effetto del server (spento), «Cambia» resta spento con quel motivo
    p.select_option("#f-tipo", "commerciale")
    expect(p.locator("#f-effetto-righe")).to_contain_text("Il cambio non si fa", timeout=T_LUNGO)
    expect(ok).to_be_disabled()
    verifica("E37" in (ok.get_attribute("title") or ""), "«Cambia» spento senza il motivo dell'effetto: %r" % ok.get_attribute("title"))
    # un particolare: l'effetto con l'avviso, poi la decisione con la firma dell'effetto
    p.select_option("#f-tipo", "sciolto")
    expect(p.locator("#f-effetto-righe")).to_contain_text("diventa un particolare", timeout=T_LUNGO)
    expect(ok).to_be_enabled()
    r1v = [r["query"] for r in b.richieste_da(i, "GET", r"/motore-a/bozza$")]
    verifica(any("effetto_cambio_tipo=" + C40 in q and "tipo_nuovo=sciolto" in q and "prodotto=" + b.p1 in q for q in r1v), "la variante di R1 non è stata letta: %s" % r1v)
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"] if az else {}
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and dec.get("tipo") == "cambia_tipo" and dec.get("tipo_nuovo") == "sciolto"
             and dec.get("componente_id") == C40 and re.fullmatch(r"[0-9a-f]{24,64}", dec.get("firma_effetto", "")) is not None
             and set(dec) == {"id", "tipo", "componente_id", "tipo_nuovo", "firma_effetto"} and az[1]["dopo"] == {"tipo": "sciolto"}, "«Cambia tipo» di un registrato: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # il cambio già deciso si toglie dal dialogo: la decisione esce e il pezzo torna al tipo di prima, dal giornale
    i = b.segna()
    p.click("#du-cambia-tipo")
    expect(p.locator("#f-togli-cambio")).to_contain_text("torna Assieme")
    p.click("#f-togli-tipo")
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_tolta", "nodo_modificato"] and az[0]["prima"]["id"] == dec["id"] and az[1]["dopo"] == {"tipo": "sottoassieme"},
             "«Togli il cambio di tipo»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # una lettura della bozza che non dà l'effetto (una R1 che la variante non la conosce ancora: risposta finta): il
    # dialogo lo dice e il cambio non parte
    def senza_effetto(route):
        if route.request.method == "GET" and "effetto_cambio_tipo=" in route.request.url:
            route.fulfill(status=200, headers={"Content-Type": "application/json"}, body=json.dumps({"stato": "nessuna_bozza", "versione_corrente": {"numero": 3}}))
        else:
            route.continue_()
    p.route(re.compile(r"/motore-a/bozza\?"), senza_effetto)
    try:
        p.click("#du-cambia-tipo")
        p.select_option("#f-tipo", "commerciale")
        expect(p.locator("#f-effetto-errore")).to_contain_text("non si legge", timeout=T_LUNGO)
        expect(ok).to_be_disabled()
        verifica("non si legge" in (ok.get_attribute("title") or ""), "«Cambia» spento senza il motivo della lettura: %r" % ok.get_attribute("title"))
        b.chiudi_finestra()
    finally:
        p.unroute(re.compile(r"/motore-a/bozza\?"), senza_effetto)


@prova("56", "P-15 e P-18 nei dialoghi: codice, rev e descrizione, il codice che non si svuota, la rev della working che non si tocca; «Adotta come codice» con la proiezione e la rev proposta (PE-08)")
def b56(b):
    p = b.page
    b.vista("org")
    ok = p.locator("#du-finestra #f-ok")
    # «+ Aggiungi un pezzo sotto…»: codice con uno spazio, oltre i 40 byte, rev oltre i 10 byte
    b.scheda("k-7")
    b.linguetta("dettaglio")
    p.click("#du-aggiungi-sotto")
    p.fill("#f-qta", "1")
    # i limiti sono in byte: maxlength dei campi conta i caratteri, e 21 «È» (42 byte) o 6 «È» (12 byte) ci stanno
    for codice, rev, parola in (("ACM 9000", "", "spazi"), ("È" * 21, "", "40 byte"), ("ACM-9000", "È" * 6, "10 byte")):
        p.fill("#f-codice", codice)
        p.fill("#f-rev", rev)
        expect(ok).to_be_disabled()
        verifica(parola in (ok.get_attribute("title") or ""), "«Aggiungi» con %r/%r spento senza il motivo: %r" % (codice, rev, ok.get_attribute("title")))
    p.fill("#f-rev", "b")
    expect(ok).to_be_enabled()
    b.chiudi_finestra()
    # «Correggi» di un pezzo importato: il codice non si svuota
    b.scheda("k-11")
    p.click("#du-correggi")
    p.fill("#f-codice", "")
    p.fill("#f-motivo", "prova del codice vuoto")
    expect(ok).to_be_disabled()
    verifica("codice è obbligatorio" in (ok.get_attribute("title") or ""), "«Correggi» con il codice vuoto: %r" % ok.get_attribute("title"))
    b.chiudi_finestra()
    # P-18: il componente registrato con la rev minuscola nella working (la risposta di R1 cambiata qui, dichiarata):
    # correggere la sola descrizione non tocca la rev né l'op; una modifica del componente con quella rev è spenta
    def rev_minuscola(route):
        if route.request.method == "GET" and "eventi=1" in route.request.url:
            resp = route.fetch()
            j = resp.json()
            for n in j.get("snapshot", {}).get("nodi", []):
                if n.get("chiave_bozza") == "k-40":
                    n["identita"]["rev"] = "a"
            route.fulfill(response=resp, json=j)
        else:
            route.continue_()
    letture_r1 = re.compile(r"/motore-a/bozza\?")
    p.route(letture_r1, rev_minuscola)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.scheda("k-40")
        b.linguetta("dettaglio")
        expect(p.locator("#nc-k-40")).to_contain_text("rev a")
        i = b.segna()
        p.click("#du-correggi")
        p.fill("#f-desc", "Supporto saldato, descrizione rivista")
        b.ok_finestra()
        az = b.ultime_azioni(i)
        verifica(len(az) == 1 and az[0]["azione"] == "nodo_modificato" and az[0]["dopo"] == {"descrizione": "Supporto saldato, descrizione rivista"}, "correzione della sola descrizione: %s" % az)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        p.click("#du-correggi")
        p.fill("#f-codice", "ACM-1401")
        p.fill("#f-motivo", "codice nuovo del supporto")
        expect(ok).to_be_disabled()
        verifica("minuscole" in (ok.get_attribute("title") or ""), "ricodifica con la rev minuscola: %r" % ok.get_attribute("title"))
        p.fill("#f-rev", "b")
        expect(ok).to_be_enabled()
        b.chiudi_finestra()
    finally:
        p.unroute(letture_r1, rev_minuscola)
    # PE-08: «Adotta come codice» della lettura con ruolo «riferimento»: la proiezione e la rev proposta del registro, la
    # copia compatta della lettura
    b.scheda("k-9")
    b.linguetta("dettaglio")
    li = p.locator('#du-sez-riferimenti li[data-du-riferimento="lettura_ruolo"]')
    bt = li.locator("button", has_text="Adotta come codice")
    expect(bt).to_be_enabled()
    i = b.segna()
    bt.click()
    expect(b.finestra()).to_contain_text("400123 rev B")
    p.fill("#f-motivo", "il codice del DXF del fornitore")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"]
    verifica(dec["tipo"] == "identita" and dec["modo"] == "alternativa" and dec["chiave_lettura"].endswith("/l:dxf")
             and dec["identita"] == {"emittente": "partner:beta", "namespace": "beta", "codice": "400123", "rev": "B"}
             and "qualificatori" not in dec["lettura"] and "revisione" not in dec["lettura"] and az[1]["dopo"]["identita"].get("rev") == "B", "«Adotta come codice» con PE-08: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


@prova("50", "E1′ lento che avanza: oltre i 120 s delle chiamate JSON, l'avviso dice l'avanzamento, il file entra (tempo d'inattività, §5 n. 3)")
def b50(b):
    p = b.page
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    cdp = p.context.new_cdp_session(p)
    cdp.send("Network.enable")
    # circa 64 KiB/s in salita: 8,2 MiB vogliono circa 131 s, più del tempo massimo delle chiamate JSON (120 s)
    cdp.send("Network.emulateNetworkConditions", {"offline": False, "latency": 0, "downloadThroughput": -1, "uploadThroughput": 65536})
    dati = b"%PDF-1.4\n% sintetico, caricamento lento\n" + b"0" * (8 * 1024 * 1024 + 200 * 1024)
    av = b.avviso()
    visti = []
    durata = 0.0
    try:
        p.set_input_files("#du-carica-file", files=[{"name": "DISEGNO_LENTO.pdf", "mimeType": "application/pdf", "buffer": dati}])
        t0 = time.time()
        p.click("#du-carica-invia")
        while time.time() - t0 < 320:
            p.wait_for_timeout(1000)
            pc = av.get_attribute("data-du-caricamento")
            if pc is not None:
                visti.append(int(pc))
            elif "Caricamento" not in (av.inner_text() or ""):
                break
        durata = time.time() - t0
    finally:
        cdp.send("Network.emulateNetworkConditions", {"offline": False, "latency": 0, "downloadThroughput": -1, "uploadThroughput": -1})
        cdp.detach()
    avanzamenti = sorted(set(x for x in visti if 0 < x < 100))
    verifica(durata > 121, "il caricamento lento è durato %.0f s: la prova non supera il tempo massimo delle chiamate JSON" % durata)
    verifica(len(avanzamenti) >= 5, "l'avviso non ha detto l'avanzamento: %s" % avanzamenti)
    expect(av).to_contain_text("1 file caricati", timeout=T_LUNGO)
    expect(p.locator("#du-carica-esiti li[data-esito=caricato]")).to_contain_text("DISEGNO_LENTO.pdf")
    verifica("DISEGNO_LENTO.pdf" in b.stato()["file"], "il file del caricamento lento non è nel banco")


@prova("51", "E1′ fermo: nessun byte inviato né ricevuto per il tempo d'inattività (60 s), il caricamento si ferma e lo dice (§5 n. 3)")
def b51(b):
    p = b.page
    b.passo("sorgente")
    p.click("#du-cartella-attivi")
    # il banco aspetta 75 s prima di leggere la richiesta, poi la lascia cadere: dopo l'invio non si muove niente
    b.ritardo("POST /thread/{id}/carica", 75000)
    b.modo("E1", "appeso")
    p.set_input_files("#du-carica-file", files=[{"name": "DISEGNO_FERMO.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% fermo\n"}])
    t0 = time.time()
    p.click("#du-carica-invia")
    av = b.avviso()
    expect(av).to_contain_text("Il caricamento si è fermato", timeout=100000)
    durata = time.time() - t0
    verifica(55 <= durata <= 90, "il caricamento fermo si è fermato dopo %.0f s, atteso circa il tempo d'inattività (60 s)" % durata)
    expect(av).to_contain_text("nessun byte inviato né ricevuto per 60 s")
    expect(av).to_contain_text("rimanda gli stessi file")
    verifica("DISEGNO_FERMO.pdf" not in b.stato()["file"], "il caricamento fermo ha registrato il file")
    expect(p.locator("#du-carica-invia")).to_be_enabled()


GUSCIO = [("GET", r"/thread/{T}/distinta-unica"), ("GET", r"/static/[A-Za-z0-9_.\-/]+"), ("GET", r"/stato/worker"), ("GET", r"/favicon\.ico")]
DEDICATE = [
    ("GET", r"/thread/{T}/motore-a/dati"), ("GET", r"/thread/{T}/motore-a/anteprima/step-strutturale"),
    ("GET", r"/thread/{T}/motore-a/bozza"), ("POST", r"/thread/{T}/motore-a/bozza"), ("POST", r"/thread/{T}/motore-a/bozza/{U}/abbandona"),
    ("POST", r"/thread/{T}/motore-a/bom/riepilogo"), ("POST", r"/thread/{T}/motore-a/bom/conferma"), ("GET", r"/thread/{T}/motore-a/intento/{U}"),
    ("GET", r"/thread/{T}/motore-a/conferme"), ("POST", r"/thread/{T}/motore-a/riallinea"), ("POST", r"/thread/{T}/carica"),
    ("GET", r"/thread/{T}/sorgente"), ("POST", r"/thread/{T}/sorgente/file/{U}/scarta"), ("POST", r"/thread/{T}/sorgente/file/{U}/altri-file"),
    ("POST", r"/thread/{T}/sorgente/file/{U}/riattiva"), ("GET", r"/thread/{T}/sorgente/file/{U}/contenuto"),
    ("GET", r"/thread/{T}/sorgente/step/{U}/anteprima"), ("POST", r"/thread/{T}/sorgente/step/{U}/associa"),
]
ATTI = [
    ("POST", r"/thread/{T}/fascicolo/componente/{U}/step-strutturale"), ("POST", r"/thread/{T}/fascicolo/componente/{U}/step-strutturale/revoca"),
    ("POST", r"/thread/{T}/fascicolo/conferma"), ("POST", r"/thread/{T}/fascicolo/prepara"), ("POST", r"/thread/{T}/fascicolo/rianalizza"),
    ("POST", r"/thread/{T}/riprova-copie"),
]


@prova("34", "richieste della pagina: guscio, statici, rotte del ponte, sei atti; nessun legacy")
def b34(b):
    def compila(elenco):
        return [(m, re.compile("^" + r.replace("{T}", re.escape(b.thread)).replace("{U}", UUID) + "$")) for m, r in elenco]
    guscio, dedicate, atti = compila(GUSCIO), compila(DEDICATE), compila(ATTI)
    # S5 è un documento: lo leggono il visore pdf.js e il browser per indirizzo, non il ponte (niente JSON)
    contenuto = re.compile("^/thread/" + re.escape(b.thread) + "/sorgente/file/" + UUID + "/contenuto$")

    def tra(r, elenco):
        return any(r["metodo"] == m and rx.match(r["path"]) for m, rx in elenco)
    estranee, senza_json, senza_hx = [], [], []
    for r in b.richieste:
        if tra(r, guscio):
            continue
        if r["metodo"] == "GET" and contenuto.match(r["path"]):
            continue
        if tra(r, dedicate):
            if r["headers"].get("accept") != "application/json" or "hx-request" in r["headers"]:
                senza_json.append((r["metodo"], r["path"], r["headers"].get("accept")))
            continue
        if tra(r, atti):
            if r["headers"].get("hx-request") != "true" or not r["headers"].get("hx-current-url", "").endswith("/thread/%s/motore-a" % b.thread):
                senza_hx.append((r["metodo"], r["path"]))
            continue
        estranee.append((r["metodo"], r["path"]))
    verifica(not estranee, "richieste fuori dal ponte: %s" % sorted(set(estranee)))
    verifica(not senza_json, "rotte dedicate senza Accept JSON o con HX-Request: %s" % senza_json)
    verifica(not senza_hx, "atti senza le intestazioni della pagina di motore-a: %s" % senza_hx)
    verifica(not b.stato()["legacy"], "il banco ha visto rotte legacy: %s" % b.stato()["legacy"])
    usate = {r["metodo"] + " " + r["path"] for r in b.richieste}
    verifica(len(usate) > 20, "troppo poche rotte nella sessione: %d" % len(usate))


@prova("49", "all'uscita («esci» del layout) le chiavi cockpit.du. della memoria della scheda si tolgono, le altre restano (R-3)")
def b49(b):
    p = b.page
    # un tentativo in sospeso della Distinta unica (come lo scrive il ponte) e una chiave che non è della pagina
    chiave = "cockpit.du.sospeso.%s.%s.bozza:%s" % (b.dati["utente"], b.thread, b.p1)
    p.evaluate("([k, v]) => { sessionStorage.setItem(k, JSON.stringify(v)); sessionStorage.setItem('prova.altra.chiave', '1'); }",
               [chiave, {"risorsa": "bozza:" + b.p1, "nome": "confermaBom", "chiave": "00000000-0000-4000-8000-0000000e0f02", "impronta": "x",
                         "verificato": True, "quando": "2026-10-18T08:00:00Z", "etichetta": "Conferma BOM"}])
    verifica(any(k.startswith("cockpit.du.") for k in p.evaluate("() => Object.keys(sessionStorage)")), "la chiave di prova non è nella memoria della scheda")
    with p.expect_navigation():
        p.click("#esci button")
    b.attendi(lambda: urllib.parse.urlsplit(p.url).path == "/login", "l'uscita non porta al login: %s" % p.url)
    chiavi = p.evaluate("() => Object.keys(sessionStorage)")
    verifica(not [k for k in chiavi if k.startswith("cockpit.du.")], "dopo l'uscita restano chiavi della Distinta unica: %s" % chiavi)
    verifica("prova.altra.chiave" in chiavi, "l'uscita ha tolto anche una chiave che non è della pagina: %s" % chiavi)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--prove", default="")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    scelte = {x.strip().zfill(2) for x in a.prove.split(",") if x.strip()}
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
        b = Banco(page, a)

        def su_dialogo(d):
            # «vuoi davvero uscire?» con un salvataggio in sospeso: atteso solo dove la prova lo dice (DU-B44), e si resta
            if d.type == "beforeunload" and b.consenti_beforeunload:
                b.beforeunload += 1
                d.dismiss()
                return
            errori.append("dialogo del browser inatteso (%s): %s" % (d.type, d.message))
            d.dismiss()
        page.on("dialog", su_dialogo)
        if scelte and "01" not in scelte:
            b.apri()
        for numero, nome, fn in PROVE:
            if scelte and numero not in scelte:
                continue
            fatte += 1
            inizio = time.time()
            try:
                fn(b)
                print("  DU-B%s  %-92s ESEGUITA PASSATA  (%.1fs)" % (numero, nome, time.time() - inizio))
            except Exception as e:
                falliti.append(numero)
                print("  DU-B%s  %-92s FALLITA  %s" % (numero, nome, str(e).splitlines()[0] if str(e) else repr(e)))
                testo = str(e)
                if "\n" in testo:
                    print("         " + "\n         ".join(testo.splitlines()[1:12]))
                b.foto("errore_B%s.png" % numero)
                # una prova fallita non trascina le altre: si chiudono la finestra e il wizard rimasti aperti
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
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
