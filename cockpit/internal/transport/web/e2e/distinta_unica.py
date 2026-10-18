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
  09 R2 409 BOZZA_VERSIONE: «La bozza e' cambiata altrove», gesti spenti con il motivo, «Rileggi la bozza salvata»
  10 R2 422: «Salvataggio rifiutato» con il motivo, «Rileggi la bozza salvata»
  11 R2 con la risposta persa: «riprovo», reinvio con la stessa chiave_salvataggio, «riproposto», nessun doppione
  12 wizard «Rivedi e conferma BOM»: R4 con gli avvisi del motore, fasi, non inclusi con gli effetti; avvisi del motore
     non calcolati = «Riprova» e presa d'atto
  13 R5 rifiutata: 409 BASE_SUPERATA, 422 BLOCCHI, 500 CONTROLLO_FINALE (nessun «Riprova»); chiave nuova, niente in sospeso
  14 R5 «esito da verificare» (500): R6 automatico, «non ancora registrata», «Riprova la conferma» con la stessa chiave
  15 R5 con la risposta persa: R6 «in corso» poi «registrato», confermata una volta, nessun reinvio
  16 R5 mai arrivata e pagina ricaricata: la chiave resta nella memoria della scheda, R6 all'apertura, «Riprova la
     conferma...» dalla striscia manda la stessa chiave
  17 abbandono esplicito del tentativo in sospeso, poi «Abbandona la bozza...» (R3)
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
     R11 «Riallinea i documenti» con F8 in errore; «Apri» e' un link a S5, mai una fetch
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

I clienti, i codici e i nomi dei file sono inventati (ACME, BETA, ACM-xxxx).
  34 le richieste della pagina: solo guscio, statici, rotte del ponte e sei atti; nessun gesto legacy
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
    expect(p.locator("#du-cartella-attivi")).to_have_text("File attivi (10)")
    expect(p.locator("#du-cartella-altri")).to_have_text("Altri file (0)")
    expect(p.locator("#du-cartella-scartati")).to_have_text("Scartati (0)")
    a1 = b.al["ACM-1000_assieme.stp"]
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


@prova("09", "R2 409 BOZZA_VERSIONE: conflitto, gesti spenti con il motivo, «Rileggi la bozza salvata»")
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
    expect(p.locator("#du-nuovo-ass")).to_be_disabled()
    verifica("cambiata altrove" in (p.locator("#du-nuovo-ass").get_attribute("title") or ""), "gesto spento senza il motivo")
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica(b.stato()["conti"]["R2"] == n_r2, "un 409 ha scritto la bozza")
    p.click("#du-rileggi-bozza")
    b.salvata(10)
    expect(p.locator("#oq-k-40_k-41")).to_contain_text("×2")
    expect(p.locator("#du-nuovo-ass")).to_be_enabled()


@prova("10", "R2 422: «Salvataggio rifiutato» con il motivo, «Rileggi la bozza salvata»")
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
    b.salvata(10)


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
        p.fill("#f-qta", "3")
        b.ok_finestra()
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "non_arrivato", timeout=T_LUNGO)
        expect(p.locator("#du-salvataggio")).to_contain_text("riprovo")
        b.salvata(11)
        expect(p.locator("#du-salvataggio")).to_contain_text("(riproposto)")
        corpi = b.corpi(i, "POST", r"/motore-a/bozza$")
        verifica(len(corpi) == 2 and corpi[0] == corpi[1], "il reinvio non è lo stesso corpo con la stessa chiave: %d invii" % len(corpi))
        verifica(b.stato()["conti"]["R2"] == n_r2 + 1, "la bozza è stata scritta due volte")
        expect(p.locator("#oq-k-40_k-41")).to_contain_text("×3")
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


@prova("13", "R5 rifiutata: 409 BASE_SUPERATA, 422 BLOCCHI, 500 CONTROLLO_FINALE; niente in sospeso")
def b13(b):
    p = b.page
    chiavi0 = len(b.stato()["chiavi_r5"])
    e = b.esito_wizard()
    b.modo("R5", "409_base")
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-codice", "BASE_SUPERATA", timeout=T_LUNGO)
    expect(e).to_contain_text("fai il rebase")
    verifica(not b.sospesi(), "BASE_SUPERATA ha lasciato la chiave in sospeso")
    b.modo("R5", "422")
    expect(p.locator("#du-wz-conferma")).to_be_enabled()
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-codice", "BLOCCHI", timeout=T_LUNGO)
    expect(e).to_contain_text("D1_ULTIMO_FIGLIO")
    expect(e).to_contain_text("Niente è registrato, la bozza resta")
    verifica(not b.sospesi(), "BLOCCHI ha lasciato la chiave in sospeso")
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
    p.click("#du-wz-riprova")
    expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    expect(e).to_contain_text("Conferma n. 1 di ACM-1000")
    expect(e).to_contain_text("Verificata: non ancora calcolata (arriva in V2)")
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 2 and r5[0]["bozza"]["chiave_idempotenza"] == k == r5[1]["bozza"]["chiave_idempotenza"], "«Riprova» non riusa la chiave")
    verifica(r5[0] == r5[1], "«Riprova» non manda lo stesso corpo")
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo la conferma")
    verifica(len(b.stato()["conferme"][b.p1]) == 1, "conferme registrate: %s" % b.stato()["conferme"])


@prova("15", "R5 con la risposta persa: R6 «in corso» poi «registrato», nessun reinvio")
def b15(b):
    p = b.page
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    p.click("#du-da-conferma")
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(1)
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


@prova("16", "R5 mai arrivata, pagina ricaricata: la chiave resta, R6 all'apertura, «Riprova la conferma…»")
def b16(b):
    p = b.page
    p.click("#du-wz-chiudi")
    p.click("#du-da-conferma")
    b.salvata(1)
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
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo la conferma")
    expect(striscia).to_have_count(0)


@prova("17", "abbandono esplicito del tentativo in sospeso, poi «Abbandona la bozza…» (R3)")
def b17(b):
    p = b.page
    p.click("#du-wz-chiudi")
    p.click("#du-da-conferma")
    b.salvata(1)
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
    verifica(len(r3) == 1 and json.loads(r3[0]["corpo"]) == {"versione_attesa": 1}, "R3: %s" % [r["corpo"] for r in r3])


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
    p.click("#du-cambia-sorgente")
    eff = p.locator("#f-effetto")
    expect(eff).to_contain_text("nodi che restano: 1")
    expect(eff).to_contain_text("nodi che escono: 1")
    expect(eff).to_contain_text("nodi che entrano: 3")
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
    expect(p.locator("#du-finestra-errore")).to_contain_text("non è configurata su questo server: niente è cambiato. (NON_CONFIGURATO)", timeout=T_LUNGO)
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


@prova("25", "atti del menu e «Revoca»; R11 con F8 in errore; «Apri» è un link a S5")
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
    a2 = b.al["ACM-1100_A.pdf"]
    href = p.locator("#du-apri-" + sid(a2)).get_attribute("href")
    verifica(href == "/thread/%s/sorgente/file/%s/contenuto?modo=anteprima" % (b.thread, a2), "«Apri»: %s" % href)
    resp = p.request.get(b.url + href)
    verifica(resp.status == 200 and "attachment" in resp.headers.get("content-disposition", "") and resp.headers.get("x-content-type-options") == "nosniff",
             "S5: %d %s" % (resp.status, resp.headers))
    verifica(not b.richieste_da(i, "GET", r"/contenuto$"), "la pagina ha chiesto S5 con una fetch")


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


@prova("30", "Elenco: filtri e ricerca (EL-01), «Accetta le proposte valide» (EL-03), «Confronto con la proposta» (BZ-25)")
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
    p.click("#du-accetta-valide")
    elenco = p.locator("#f-proposte input")
    verifica(elenco.count() >= 1 and p.locator("#f-proposte input:checked").count() == 0, "«Accetta le proposte valide»: %d proposte, preselezioni presenti" % elenco.count())
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
    # la sezione «Documenti» del pannello: un file da smistare preassociato a questo pezzo
    b.scheda("k-41")
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
    verifica(len(esiti) == len(casi["vassoio"]) + len(casi["applica"]) and not falliti, "casi falliti: %s" % falliti)


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

    def tra(r, elenco):
        return any(r["metodo"] == m and rx.match(r["path"]) for m, rx in elenco)
    estranee, senza_json, senza_hx = [], [], []
    for r in b.richieste:
        if tra(r, guscio):
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
        page.on("dialog", lambda d: (errori.append("dialogo del browser inatteso: " + d.message), d.dismiss()))
        b = Banco(page, a)
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
        if errori:
            print("  errori nella pagina: %s" % errori)
            falliti.append("javascript")
        browser.close()
    print("\n%d prove nel browser: %d passate, %d fallite" % (fatte, fatte - len([f for f in falliti if f != "javascript"]), len(falliti)))
    return 1 if falliti else 0


if __name__ == "__main__":
    sys.exit(main())
