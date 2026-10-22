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
  02 Sorgente (mockup v5, direzione A, ondata C): cartelle con i conti, la pagina che non scorre, «Da controllare» senza
     bozza, «Guarda l'anteprima per ‹P›» (S6), organigramma della proposta, pannello del nodo, zoom e pan senza richieste
  03 S8 «Conferma questo STEP come sorgente di ‹P›»: 409 al passo 0, «parziale» ritentabile ripreso con la stessa chiave,
     i tre passi, la cartella del prodotto; poi «Autorizza» (atto della lista chiusa, presa d'atto mai spuntata da sola)
  04 dopo S8: la versione 1 costruita dal server, «Vai alla Revisione →», nessun R4 all'apertura della bozza
  05 Elenco: «Accetta la proposta» (preassociazione) = R2 con vocabolario 2 e chiave_salvataggio; R4 in sottofondo; i
     gesti di V3 spenti con il motivo
  06 selezione multipla: «Seleziona tutti i segnalati», Ctrl+clic, «Non includere nella BOM...»: un salvataggio, un
     azione_id, le decisioni in fondo con segnale_visto
  07 Annulla e Ripeti: annulla{n} e rifai{n} al server, con prima = {azione_id} per gli eventi del gesto cumulativo
     (EM par. 7.1, PE-05) e null per gli altri; mai oltre la versione costruita dal server
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
  19 «Cambia la sorgente di ‹P› con questo STEP…» (S8 con la bozza): prima la revoca dello STEP autorizzato, l'effetto
     da S6, il motivo, sostituisci
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
  29 la cartella del prodotto con lo smistamento di S9: «Salva nella bozza», «Annulla la mossa», «Togli dalla bozza»;
     «Associa a un nodo...» con la riga fantasma e «Ultima mossa»; «Scarica» (FL-03)
  30 l'Elenco: filtri e ricerca (EL-01), «Accetta le proposte (N)» senza preselezione (EL-03, PE-21); «Confronto con la
     proposta» sulle schede (BZ-25)
  31 il pannello del nodo: la sezione «Documenti» (EL-08) e «⤴ Ricolloca i figli...» in un salvataggio (BZ-11)
  32 «Riepilogo RFQ» (TS-08), «STEP secondario disponibile» (SG-12) e «nessuna candidata» (SG-13)
  33 i casi di contratto condivisi (e2e/distinta_unica_casi.json): il vassoio «Da ricollocare» (PM23 T-16, la meta'
     della pagina) e le azioni del vocabolario 2 applicate allo snapshot, con le funzioni esportate dalla pagina,
     caricata in una scheda senza il guscio (l'avvio della pagina non parte senza #du); i casi hanno le forme complete
     della tabella di EM par. 9.2 (RIC-I1, RIC-I2 della corsia contratto)

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
     RISOLUZIONE_RICHIESTA, registrato_senza_esito, niente S7, NESSUN_FILE_CARICATO, 413),
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
  53 (secondo passaggio) WZ-01 con lo storico (R7) in 5xx: la bozza dalla conferma e la bozza «registrato» senza gesti
     hanno il CTA spento con «↻ Aggiorna», e niente dice «nessuna conferma»; R4 su una bozza «registrato» dopo un
     annullamento netto (RPC PD-01, punto 3)
  54 P-6: il nodo del prodotto segnalato non si «Non include» (casella spenta, «Seleziona tutti i segnalati» lo salta)
  55 P-7 e PE-09: «Cambia tipo» di un registrato con la variante di R1 e cambia_tipo{componente_id, tipo_nuovo, firma};
     un effetto con altri nomi non si legge e «Cambia» resta spento (secondo passaggio, PE-B05)
  56 P-15 e P-18 nei dialoghi (codice, rev e descrizione; il codice che non si svuota; la rev della working che non si
     tocca; la rev minuscola del codice manuale che «Correggi» non porta in una correzione; «Fissa l'emittente»);
     «Adotta come codice» con la proiezione e la rev proposta (PE-08)
  50 E1' lento che avanza: oltre i 120 s delle chiamate JSON, con l'avanzamento, il file entra (§5 n. 3)
  51 E1' fermo: nessun byte per il tempo d'inattività (120 s), il caricamento si ferma e lo dice (§5 n. 3); la prova
     aspetta che il banco abbia finito la richiesta lasciata appesa
  34 le richieste della pagina: solo guscio, statici, rotte del ponte e sei atti; nessun gesto legacy
  57 (secondo passaggio, dopo la 34) l'uscita annullata: con le modifiche tenute da parte «esci» chiede conferma, si
     resta, e le chiavi cockpit.du. restano; la pagina d'accesso aperta dopo non le toglie (R-3, MAT §5.5 n. 7)
  dall'ondata C (U2): 58 le notifiche con il wizard aperto (N1 con la presa d'atto, N2, N4, N5); 59 conferma_categoria
     dall'editor del nodo e dal wizard, «Conferma il tipo proposto», «Non è minuteria…»; 60 (dopo la 14) R15 «Rifiuta la
     proposta aggiornata…» senza bozza
  dalla direttiva 35 (C-2): 61 (dopo la 36) la preassociazione per allegato_id: un file senza proposta di documento si
     preassocia (Prova 2), niente STEP né archivi fra i file da smistare, «Sposta a un altro nodo…» con svincolo e
     preassociazione
  dal rilievo del revisore del 20/10: 67 (dopo la 66) CREA_SU_ESISTENTE con «Riusa ‹C›» e «Riusa i componenti con lo stesso
     codice (n)» (riusa_esistente con R2), «Togli il riuso» nel pannello
  dal messaggio 48 [U] (UI46, corsia wizard): W1 (solo se scelta, su un banco suo) il wizard scorso a metà non torna in
     cima con la rilettura in sottofondo, le preassociazioni senza la chiave del nodo, i testi senza apostrofi per accenti
  dal controllo di stabilità del 21/10 (UI46 [U]): X1-X9 (solo se scelte, su un banco loro) S1 riletto ogni 5 s senza
     ridisegnare, la ricerca che non perde tasti, l'elenco della Sorgente e il pannello del nodo che non tornano in cima
     dopo un gesto, il fuoco che torna sulla scheda senza far saltare la pagina, la barra della selezione e la striscia
     dell'esito che non spostano la tela, il fuoco dopo «Guarda l'anteprima» e dopo un dialogo con la pagina ridisegnata
  49 (per ultima: esce dalla sessione) all'uscita («esci» del layout) le chiavi cockpit.du. della memoria della scheda
     si tolgono sulla pagina d'accesso, quelle dell'editor subito, le altre restano (richiesta R-3 della corsia)

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
# la Revisione senza bozza: «nessuna bozza» o, con una conferma, la BOM confermata in sola lettura (UI46, messaggio 47 [U])
SENZA_BOZZA = "#du-nessuna-bozza, #du-working"
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

    def sorgente_s8(self, prodotto):
        """Dati di prova (sonda sorgente-s8): la bozza aperta del prodotto riceve una sorgente confermata da S8 e valida,
        così il gate della sorgente, acceso come nel server, la lascia confermare; poi la pagina si rilegge."""
        r = self.page.request.post(self.url + "/__banco/sorgente-s8", data=json.dumps({"prodotto": prodotto}), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda sorgente-s8 ha risposto %d" % r.status)
        self.page.click("#du-aggiorna")
        expect(self.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)

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
        return {k: v for k, v in c.items() if k in ("R2", "R3", "R4", "R5", "R11", "E1", "S2", "S3", "S4", "atti") and v}

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

    def cartella(self, k):
        """Una cartella del tab Sorgente (mockup v5, direzione A): l'albero a sinistra."""
        v = self.page.locator("#du-sg-c-" + sid(k))
        v.click()
        expect(v).to_have_attribute("aria-selected", "true", timeout=T_BREVE)

    def file(self, allegato, cartella="tutti"):
        """Il dettaglio di un file: la cartella, poi la riga (il nome), poi il dettaglio a destra."""
        if self.page.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
            self.passo("sorgente")
        self.cartella(cartella)
        self.page.locator("#du-file-%s .du-fr-nome, #du-step-%s .du-fr-nome" % (sid(allegato), sid(allegato))).first.click()
        expect(self.page.locator("#du-sg-file")).to_have_attribute("data-allegato", allegato, timeout=T_BREVE)

    def azione(self, nome, allegato, cartella="tutti"):
        """Un'azione del dettaglio di un file (la matrice del mockup v5): scegli il file, poi il pulsante «du-‹azione›-‹file›»."""
        self.file(allegato, cartella)
        self.page.click("#du-sg-dettaglio #du-" + nome + "-" + sid(allegato))

    def apri_carica(self):
        """«+ Aggiungi file» nella barra dello spazio di lavoro della Sorgente (mockup v5): il modulo di E1′."""
        if self.page.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
            self.passo("sorgente")
        if self.page.locator("#du-carica-file").count() == 0:
            self.page.click("#du-sg-aggiungi")
        expect(self.page.locator("#du-carica-file")).to_be_attached(timeout=T_BREVE)

    def guarda(self, allegato, prodotto=None):
        """«Guarda l'anteprima per ‹P›» dalla cartella «STEP della RFQ» (il gesto in riga della candidatura)."""
        if self.page.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
            self.passo("sorgente")
        self.cartella("step")
        suf = sid(allegato) + ("" if not prodotto or prodotto == self.p1 else "-" + sid(prodotto))
        bt = self.page.locator("#du-guarda-" + suf).first
        if bt.inner_text().startswith("Chiudi l'anteprima"):
            # l'anteprima di questo STEP è già aperta (nel riquadro stretto): il gesto la chiude, il secondo la riapre allargata
            bt.click()
            expect(self.page.locator("#du-guarda-" + suf).first).to_contain_text("Guarda l'anteprima", timeout=T_BREVE)
        self.page.locator("#du-guarda-" + suf).first.click()
        expect(self.page.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)

    def altri_prodotti(self):
        """Apre «Guardalo per un altro prodotto (n)» nel dettaglio di uno STEP (ripiegato dal messaggio 46, D27)."""
        d = self.page.locator("#du-sg-dettaglio #du-sg-altri-prodotti")
        expect(d).to_be_attached(timeout=T_LUNGO)
        if d.get_attribute("open") is None:
            d.locator("summary").click()

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


@prova("02", "Sorgente (mockup v5): cartelle con i conti, «Guarda l'anteprima per ‹P›» (S6), proposta, pannello del nodo, zoom e pan")
def b02(b):
    p = b.page
    # D-V5-01 (base [T] A′): finché nessun prodotto ha la bozza la Sorgente si apre su «STEP della RFQ»
    expect(p.locator("#du-sg-c-step")).to_have_attribute("aria-selected", "true")
    expect(p.locator("#du-sg-c-tutti .du-sg-conto")).to_have_text("15")
    expect(p.locator("#du-sg-c-altri .du-sg-conto")).to_have_text("0")
    expect(p.locator("#du-sg-c-scartati .du-sg-conto")).to_have_text("0")
    expect(p.locator("#du-sg-c-step")).to_contain_text("sorgenti: 0 di 2")
    expect(p.locator("#du-sg-c-p_" + sid(b.p1))).to_contain_text("○ da confermare · 2 STEP")
    # lo spazio di lavoro sta nel tab: la pagina non scorre (R1 del mockup v5, [U] M32)
    alt = p.evaluate("() => [document.documentElement.scrollHeight, window.innerHeight]")
    verifica(alt[0] <= alt[1] + 2, "la pagina scorre nella Sorgente: %s" % alt)
    a1 = b.al["ACM-1000_assieme.stp"]
    # SG-01: per ogni STEP le radici candidate, la compatibilità, lo stato della struttura (LS-01) e i nodi segnalati
    # messaggio 46 (D47): nella riga la candidata in breve, la diagnostica del motore nel titolo
    c1 = p.locator("#du-step-" + sid(a1) + " [data-du-candidata='%s']" % b.p1)
    expect(c1).to_contain_text("candidata per ACM-1000 · compatibile")
    verifica("struttura completa" in (c1.get_attribute("title") or "") and "segnalati: 3" in (c1.get_attribute("title") or ""), "la diagnostica della candidata non è nel titolo")
    expect(p.locator("#du-step-" + sid(b.al["ACM-1000_rev2.stp"]) + " [data-du-radice='ACM-1000-R2']")).to_contain_text("candidata per ACM-1000")
    # «Da controllare» senza bozza: la fascia «Prima conferma una sorgente» (D-V5-01 A′)
    b.cartella("dc")
    expect(p.locator("#du-sg-prima-sorgente")).to_contain_text("Prima conferma una sorgente")
    b.cartella("dc-senza")
    b.file(b.al["SCHEMA_GENERALE.pdf"], "dc-senza")
    expect(p.locator("#du-sg-motivo")).to_contain_text("nessun nodo candidato")
    expect(p.locator("#du-sg-nondisp")).to_contain_text("nessun prodotto ha ancora la bozza: conferma prima una sorgente")
    b.cartella("dc-att:" + b.p1)
    expect(p.locator("#du-sg-elenco")).to_contain_text("ACM-1100_A.pdf")
    expect(p.locator("#du-sg-elenco")).to_contain_text("attende la sorgente di ACM-1000")
    b.guarda(a1)
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
    expect(p.locator("#du-sez-proposta")).to_contain_text("fornitore beta")
    expect(p.locator("#du-sez-riferimenti")).to_contain_text("documentale")
    expect(p.locator("#du-sez-riferimenti")).to_contain_text("documentato da")
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


@prova("62", "direttiva 39: «Parti dal solo prodotto» senza STEP, pezzi e PB a mano, R4 con STEP_SORGENTE_NON_CONFERMATA e la conferma spenta; S8 riconcilia la stessa bozza (un abbinamento certo, uno da decidere, nessun doppione), «Abbina a…»; le differenze con lo STEP (quantità discordante, legami, tipo) con «Recepisci» e «Mantieni»; «Revoca come sorgente…»; lo STEP in analisi")
def b62(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    a10 = b.al["ACM-1000_rev2.stp"]
    # 1. nessuna bozza e nessuno STEP confermato: «Parti dal solo prodotto…» (R2 crea vuota, direttiva 39 §1)
    b.passo("revisione")
    expect(p.locator("#du-da-vuota")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-da-vuota")
    expect(b.finestra()).to_contain_text("«Conferma BOM» richiederà uno STEP confermato come sorgente")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza aperta dal solo prodotto", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["crea"] == {"partenza": "vuota"} and c["bozza_id"] is None and c["versione_attesa"] == 0 and c["vocabolario"] == 2, "corpo della partenza vuota: %s" % c)
    verifica(not b.richieste_da(i, "POST", r"/conferma-sorgente$"), "la partenza vuota ha chiamato S8")
    expect(p.locator("#du-senza-sorgente")).to_contain_text("Bozza senza STEP", timeout=T_LUNGO)
    b.salvata(1)
    # 2. due pezzi messi a mano sotto il prodotto (uno con codice e rev dello STEP che arriverà, uno con un'altra rev) e una
    # PB manuale: la bozza senza STEP si salva e si versiona (direttiva 39 §1)
    b.vista("org")
    chiavi = []
    for codice, rev, desc in (("ACM-1100", "A", "Telaio messo a mano"), ("ACM-1120", "B", "Traversa messa a mano")):
        b.scheda("k-root")
        b.linguetta("dettaglio")
        p.click("#du-aggiungi-sotto")
        p.select_option("#f-tipo", "sciolto")
        p.fill("#f-desc", desc)
        p.fill("#f-codice", codice)
        p.fill("#f-rev", rev)
        p.fill("#f-qta", "2")
        b.ok_finestra()
        expect(p.locator("#du-vp .du-nc", has_text=codice)).to_have_count(1, timeout=T_LUNGO)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        chiavi.append(p.locator("#du-pannello").get_attribute("data-k"))
    b.salvata(3)
    k_telaio, k_traversa = chiavi
    verifica(k_traversa == p.locator("#du-pannello").get_attribute("data-k"), "il pannello non è sulla traversa appena aggiunta")
    # prima dello STEP nessuna differenza da mostrare (non c'è un cambio della fonte)
    expect(p.locator("#du-differenze")).to_have_count(0)
    b.linguetta("documenti")
    i = b.segna()
    p.select_option("#du-pannello-file", label="SCHEMA_GENERALE.pdf")
    p.click("#du-pannello-preassocia")
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": b.al["SCHEMA_GENERALE.pdf"], "chiave_bozza": k_traversa})], "PB manuale: %s" % az)
    b.salvata(4)
    b.linguetta("dettaglio")
    # 3. R4 completo senza STEP, con il blocco della sorgente; la conferma spenta con il rimedio nella Sorgente (il gate è
    # acceso nel banco come nel server, dal blocco R4/R5 della persistenza)
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_contain_text("La BOM non è confermabile", timeout=T_LUNGO)
    expect(p.locator("#du-wz-blocchi [data-codice=STEP_SORGENTE_NON_CONFERMATA]")).to_contain_text("non ha uno STEP confermato come sorgente")
    expect(p.locator("#du-wz-vai-sorgente")).to_be_visible()
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    verifica("conferma uno STEP dalla Sorgente" in (p.locator("#du-wz-conferma").get_attribute("title") or ""), "conferma spenta senza il rimedio della sorgente")
    # la consultazione resta completa: la sorgente (nessuno STEP), i documenti per gruppo con la PB manuale
    expect(p.locator("#du-wz-sorgente-legata")).to_contain_text("parte dal solo prodotto")
    expect(p.locator("#du-wz-gruppo-manuale")).to_contain_text("SCHEMA_GENERALE.pdf")
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bom/conferma$"), "R5 chiamata con il blocco della sorgente")
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()
    # 4. senza S8 il motore non propone righe per il prodotto (D-AR11 B): i file attendono la sorgente
    b.passo("sorgente")
    expect(p.locator("#du-sg-c-p_" + sid(b.p1))).to_contain_text("bozza v4 senza sorgente confermata", timeout=T_LUNGO)
    expect(p.locator("#du-sg-c-dc-att_" + sid(b.p1))).to_be_visible()
    # 5. arriva lo STEP: S8 sulla bozza aperta riconcilia la STESSA bozza (dichiarata nel corpo), con il motivo
    b.guarda(a1)
    expect(p.locator("#du-conferma-sorgente")).to_have_text("Conferma questo STEP come sorgente di ACM-1000…", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_contain_text("riconcilia la bozza senza STEP", timeout=T_LUNGO)
    expect(p.locator("#f-effetto")).to_contain_text("nodi che entrano")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")[0]
    verifica(c["bozza"]["versione_attesa"] == 4 and c["codice_atteso"] == "ACM-1000" and c.get("sha256") and c["gruppo"].startswith("cand:") and "partenza" not in c
             and "componente_atteso" not in c and "motivo" not in c, "corpo di S8 sulla bozza senza STEP: %s" % c)
    s6 = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6 and "bozza=" in s6[-1]["query"], "l'effetto non è stato letto con S6 ?bozza=: %s" % [r["query"] for r in s6])
    expect(b.avviso()).to_contain_text("Abbinamenti da decidere nella Revisione: 1")
    st = b.stato()
    verifica(st["fonte"]["autorizzata"] is False, "S8 ha toccato la working (autorizzata): %s" % st["fonte"])
    verifica(st["bozze"][b.p1]["versione"] == 5, "la riconciliazione non è la versione 5 della stessa bozza: %s" % st["bozze"][b.p1])
    verifica(not b.richieste_da(i, "POST", r"/step-strutturale"), "S8 ha chiamato l'autorizzazione")
    # 6. la vista della riconciliazione: nessun doppione, l'abbinamento incerto fuori da «da rivedere», «Abbina a…»
    b.passo("revisione")
    ab = p.locator("#du-abbinamenti")
    expect(ab).to_contain_text("Abbinamenti da decidere (1)", timeout=T_LUNGO)
    expect(ab.locator("[data-du-abbinamento='%s']" % k_traversa)).to_have_attribute("data-motivo", "incompatibile")
    expect(p.locator("#du-vp .du-nc", has_text="ACM-1100")).to_have_count(1)
    expect(p.locator("#du-vp .du-nc", has_text="ACM-1120")).to_have_count(1)
    i = b.segna()
    ab.get_by_role("button", name=re.compile(r"^Abbina a ACM-1120")).click()
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "nodo_modificato" and az[0]["chiave_bozza"] == k_traversa and az[0]["prima"]["caselle"] is None
             and az[0]["dopo"]["caselle"][0]["rif"].endswith(":#11") and az[0]["dopo"]["identita"]["codice"] == "ACM-1120" and az[0]["dopo"]["identita"]["modo"] == "proposta",
             "«Abbina a…»: %s" % az)
    expect(ab).to_have_count(0, timeout=T_LUNGO)
    b.salvata(6)
    # 6-bis. le differenze con lo STEP (direttiva 39 §3 [U + T-39]: quantità, padre/figlio e tipi si mostrano; «Recepisci» è
    # un gesto normale, «Mantieni» non scrive): il telaio messo a mano ×2 sotto il prodotto, lo STEP dice ×1; la traversa
    # abbinata sta sotto il prodotto nella bozza e sotto il telaio nello STEP (×4); il telaio è un particolare, lo STEP un
    # assieme. Il server non le dà ancora nell'evento: la pagina le ricava dal registro della bozza (data-origine)
    df = p.locator("#du-differenze")
    expect(df).to_be_visible(timeout=T_LUNGO)
    qta = df.locator("[data-du-differenza='a:cambiato:k-root>%s']" % k_telaio)
    expect(qta).to_contain_text("bozza ×2, STEP ×1")
    expect(qta).to_have_attribute("data-origine", "registro")
    tolto = df.locator("[data-du-differenza='a:tolto:k-root>%s']" % k_traversa)
    expect(tolto).to_contain_text("legame della bozza che lo STEP non ha")
    aggiunto = df.locator("[data-du-differenza='a:aggiunto:%s>%s']" % (k_telaio, k_traversa))
    expect(aggiunto).to_contain_text("STEP ×4")
    # il legame sotto il telaio non si recepisce finché il telaio è un particolare (E37): prima il tipo
    expect(aggiunto.get_by_role("button", name="Recepisci")).to_be_disabled()
    verifica("non ha figli" in (aggiunto.get_by_role("button", name="Recepisci").get_attribute("title") or ""), "«Recepisci» spento senza il motivo E37")
    expect(df.locator("[data-du-differenza='n:tipo:%s']" % k_telaio)).to_contain_text("bozza Particolare, STEP Assieme")
    verifica(df.get_attribute("data-n") == "4", "differenze dopo la riconciliazione: %s" % df.get_attribute("data-n"))
    # «Recepisci» della quantità: qta_modificata da ×2 a ×1, un salvataggio; la riga sparisce (il legame è toccato)
    i = b.segna()
    qta.get_by_role("button", name="Recepisci").click()
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "qta_modificata" and az[0]["chiave_bozza"] == k_telaio and az[0]["prima"] == {"padre": "k-root", "figlio": k_telaio, "qta": 2}
             and az[0]["dopo"] == {"padre": "k-root", "figlio": k_telaio, "qta": 1}, "«Recepisci» della quantità: %s" % az)
    b.salvata(7)
    expect(qta).to_have_count(0, timeout=T_LUNGO)
    # «Mantieni» del legame che lo STEP non ha: nessuna scrittura, la bozza resta com'era
    i = b.segna()
    tolto.get_by_role("button", name="Mantieni").click()
    expect(tolto).to_have_count(0, timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("Mantenuto")
    p.wait_for_timeout(1800)
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "«Mantieni» ha salvato la bozza")
    verifica(b.stato()["bozze"][b.p1]["versione"] == 7, "«Mantieni» ha cambiato la bozza")
    verifica(any(a["padre"] == "k-root" and a["figlio"] == k_traversa and a["qta"] == 2 for a in b.stato()["bozze"][b.p1]["snapshot"]["archi"]),
             "il legame mantenuto non è più nella bozza")
    b.foto("B62_differenze.png")
    # 7. «Revoca come sorgente…» per uno STEP scelto per errore: il motivo, la rotta dedicata, la working non cambia
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-revoca-sorgente").count() == 0:
        p.click("#du-sg-prodotto-apri")
    # G39-03 [U]: la revoca porta la causa «sorgente errata» e la bozza che la pagina mostra; la bozza è nata da questo STEP
    # e la BOM non è confermata con lui: il server la annulla (la storia resta), la working non cambia
    p.click("#du-revoca-sorgente")
    expect(b.finestra()).to_contain_text("Serve a correggere uno STEP scelto per errore")
    expect(b.finestra()).to_contain_text("«sorgente errata»")
    expect(p.locator("#f-revoca-bozza")).to_have_attribute("data-nata", "1")
    expect(p.locator("#f-revoca-bozza")).to_contain_text("la revoca annulla la bozza")
    verifica("non cambia la BOM confermata, la bozza" not in b.finestra().inner_text(), "il dialogo della revoca dice ancora che la bozza non cambia")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Sorgente revocata per ACM-1000", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("bozza nata da questo STEP è annullata")
    rv = b.corpi(i, "POST", r"/sorgente/step/[^/]+/revoca-sorgente$")
    verifica(len(rv) == 1 and rv[0]["prodotto_id"] == b.p1 and "motivo" not in rv[0] and re.fullmatch(UUID, rv[0]["chiave_idempotenza"])
             and rv[0].get("sorgente_errata") is True and rv[0].get("bozza") == {"bozza_id": b.stato()["bozze"][b.p1]["bozza_id"], "versione_attesa": 7}
             and ("/step/%s/" % a1) in b.richieste_da(i, "POST", r"/revoca-sorgente$")[0]["path"], "corpo della revoca: %s" % rv)
    verifica(not b.richieste_da(i, "POST", r"/step-strutturale"), "la revoca è passata dall'atto legacy")
    expect(p.locator("#du-passo-sorgente")).to_contain_text("sorgente revocata", timeout=T_LUNGO)
    st = b.stato()
    verifica(st["fonte"]["autorizzata"] is False and st["bozze"][b.p1]["stato"] == "abbandonata" and st["bozze"][b.p1]["versione"] == 7,
             "la revoca ha cambiato la working o non ha annullato la bozza nata dallo STEP: %s" % st["bozze"][b.p1]["stato"])
    # 8. lo STEP in analisi: l'anteprima dice lo stato (mai una prova geometrica) e la partenza dal solo prodotto, che senza
    # bozza (annullata dalla revoca) non è spenta per «già uno STEP»
    b.page.request.post(b.url + "/__banco/analisi-step", data=json.dumps({"nome": "ACM-1000_rev2.stp", "stato": "in_analisi"}), headers={"Content-Type": "application/json"})
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.cartella("dc-analisi")
        b.file(a10, "dc-analisi")
        p.click("#du-sg-dettaglio #du-guarda-" + sid(b.p1) + "-" + sid(a10))
        expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
        expect(p.locator("#du-struttura-step")).to_have_attribute("data-struttura", "non_determinabile")
        expect(p.locator("#du-struttura-step")).to_contain_text("analisi in corso")
        expect(p.locator("#du-conferma-solo-prodotto")).to_be_visible()
        verifica("già uno STEP" not in (p.locator("#du-conferma-solo-prodotto").get_attribute("title") or ""), "partenza dal solo prodotto spenta per una bozza che non c'è più")
        p.click("#du-anteprima-chiudi")
    finally:
        b.page.request.post(b.url + "/__banco/analisi-step", data=json.dumps({"nome": "ACM-1000_rev2.stp", "stato": "letto"}), headers={"Content-Type": "application/json"})
    # 9. lo stato delle prove che seguono: la bozza abbandonata (dalla revoca, G39-03), la sorgente revocata
    b.passo("revisione")
    expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-da-vuota")).to_be_enabled()
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.guarda(a1)


@prova("68", "G39-02 [U]: lo STEP proposto come sorgente del finito (S9 e /dati fonte.proposte_sorgente[], la forma del server): la domanda «Il motore ha trovato…» con le evidenze e la conferma (S8) nella cartella del prodotto, nel dettaglio dello STEP e nella Revisione; la bozza manuale con lo STEP proposto molto sconsigliata (l'avviso, «Conferma prima lo STEP proposto…», la presa d'atto); lo STEP proposto mai fra i file da preassociare")
def b68(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    pdf = b.al["ACM-1100_A.pdf"]
    if p.locator("#du-anteprima-chiudi").count():
        p.click("#du-anteprima-chiudi")
    # la proposta come prima di ogni S8: la sequenza arriva qui con la sorgente di ACM-1000 revocata (DU-B62), e dopo il
    # messaggio 46 [U] per uno STEP revocato la pagina mostra lo stato della revoca, non la domanda «lo confermi?» (DU-B72)
    r = p.request.post(b.url + "/__banco/sorgente-s8-da-parte", data=json.dumps({"prodotto": b.p1}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda sorgente-s8-da-parte ha risposto %d" % r.status)
    b.interruttore("step-proposto", True)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        # (a) la cartella del prodotto: la domanda alla lettera, le evidenze della lettura del Motore A, la conferma (S8)
        b.passo("sorgente")
        b.cartella("p:" + b.p1)
        if p.locator("#du-step-proposto-sg").count() == 0:
            p.click("#du-sg-prodotto-apri")
        box = p.locator("#du-step-proposto-sg")
        expect(box).to_be_visible(timeout=T_LUNGO)
        expect(box).to_have_attribute("data-allegato", a1)
        expect(box).to_contain_text("Il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di ACM-1000: lo confermi come sorgente?")
        expect(box).to_contain_text("ACM-1000_assieme.stp")
        ev = p.locator("#du-step-proposto-evidenze-sg")
        expect(ev.locator("[data-evidenza=codice_prodotto]")).to_contain_text("codice del prodotto nella RFQ: ACM-1000")
        # le evidenze nella forma del server (evidenze[] con il selettore): il nome del file e la radice dello STEP con il
        # marcatore Z (PM-01: il codice letto con il marcatore contro il codice del prodotto)
        expect(ev.locator("[data-evidenza=nome_file]")).to_contain_text("codice letto dal nome del file: ACM-1000")
        expect(ev.locator("[data-evidenza=nome_file]")).to_contain_text("base: uguale")
        expect(ev.locator("[data-evidenza='step.radice_id']")).to_contain_text("ACM-1000Z")
        expect(ev.locator("[data-evidenza='step.radice_id']")).to_contain_text("marcatore Z")
        expect(ev.locator("[data-evidenza='step.radice_id']")).to_have_attribute("data-marcatore", "Z")
        verifica("il server non ha ancora" not in box.inner_text(), "la proposta mostra ancora il ripiego della pagina")
        expect(ev.locator("[data-evidenza=struttura_candidata]")).to_contain_text("una struttura candidata del prodotto ha la radice di questo STEP")
        verifica("nodo:" not in ev.inner_text(), "le evidenze mostrano la chiave interna del nodo: %s" % ev.inner_text())
        expect(p.locator("#du-conferma-proposto-sg")).to_contain_text("come sorgente di ACM-1000")
        expect(p.locator("#du-conferma-proposto-sg")).to_be_enabled()
        b.foto("B68_step_proposto.png")
        # il dettaglio dello STEP: la stessa proposta prima delle candidature
        b.file(a1, "step")
        expect(p.locator("#du-sg-dettaglio #du-step-proposto-d")).to_contain_text("molto probabilmente lo STEP strutturale di ACM-1000")
        # la Revisione senza bozza: la proposta accanto alle partenze
        b.passo("revisione")
        expect(p.locator("#du-nessuna-bozza #du-step-proposto-rev")).to_be_visible(timeout=T_LUNGO)
        # (b) «Parti dal solo prodotto…» con lo STEP proposto: l'avviso, la conferma spenta finché non si sceglie di partire
        # senza; «Conferma prima lo STEP proposto…» porta al dialogo di S8 della proposta
        i = b.segna()
        p.click("#du-da-vuota")
        expect(p.locator("#f-step-proposto")).to_contain_text("C'è uno STEP proposto come sorgente: è molto sconsigliato partire senza.")
        expect(p.locator("#f-step-proposto")).to_contain_text("ACM-1000_assieme.stp")
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        verifica("partire senza lo STEP proposto" in (p.locator("#du-finestra #f-ok").get_attribute("title") or ""), "la conferma della bozza manuale è spenta senza il motivo")
        p.click("#f-conferma-proposto")
        expect(b.finestra()).to_contain_text("Conferma ACM-1000_assieme.stp come sorgente di ACM-1000", timeout=T_LUNGO)
        p.locator("#du-finestra").get_by_role("button", name="Annulla").click()
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        verifica(not b.richieste_da(i, "POST"), "«Conferma prima lo STEP proposto…» poi «Annulla» ha scritto: %s" % [r["path"] for r in b.richieste_da(i, "POST")])
        # la bozza manuale, con la presa d'atto
        b.passo("revisione")
        p.click("#du-da-vuota")
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        p.check("#f-senza-proposto")
        i = b.segna()
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("Bozza aperta dal solo prodotto", timeout=T_LUNGO)
        c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
        verifica(c["crea"] == {"partenza": "vuota"}, "corpo della bozza manuale: %s" % c)
        # la bozza senza STEP: la proposta resta in vista nella Revisione
        expect(p.locator("#du-senza-sorgente")).to_be_visible(timeout=T_LUNGO)
        expect(p.locator("#du-step-proposto-rev")).to_contain_text("lo confermi come sorgente?")
        # (c) lo STEP proposto non è fra i file da preassociare a un nodo (un PDF sì)
        b.vista("org")
        b.scheda("k-root")
        b.linguetta("documenti")
        opzioni = p.locator("#du-pannello-file option").evaluate_all("(os) => os.map(o => o.value)")
        verifica(a1 not in opzioni, "lo STEP proposto è fra i file da preassociare: %s" % opzioni)
        verifica(pdf in opzioni, "i file da preassociare non hanno il PDF: %s" % opzioni)
        b.linguetta("dettaglio")
        # la conferma della proposta da qui riconcilia la bozza senza STEP (il dialogo di S8), poi «Annulla»
        p.click("#du-conferma-proposto-rev")
        expect(b.finestra()).to_contain_text("riconcilia la bozza senza STEP", timeout=T_LUNGO)
        p.locator("#du-finestra").get_by_role("button", name="Annulla").click()
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        # si torna allo stato delle prove che seguono: la bozza abbandonata, l'interruttore spento
        p.click("#du-abbandona")
        b.ok_finestra()
        expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    finally:
        b.interruttore("step-proposto", False)
        p.request.post(b.url + "/__banco/sorgente-s8-da-parte", data=json.dumps({"prodotto": b.p1, "rimetti": True}), headers={"Content-Type": "application/json"})
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    expect(p.locator("#du-step-proposto-rev")).to_have_count(0)
    b.passo("sorgente")
    b.guarda(a1)


# DU-B69 e DU-B70 girano solo se scelte (--prove 69, --prove 70), ciascuna su un banco suo: TestDistintaUnicaG39NelBrowser
# (lo spostamento cambia le sorgenti dei due prodotti, che le prove della sequenza lunga non si aspettano) e
# TestDistintaUnicaG39ProvenienzaNelBrowser (parte da un banco senza bozze); DU-BW1 solo con --prove W1, su un banco suo:
# TestDistintaUnicaWizardScorrimentoNelBrowser (UI46, messaggio 48); DU-BX1…DU-BX9 solo con --prove X1,…,X9, su un banco
# loro: TestDistintaUnicaStabilitaNelBrowser (UI46, controllo di stabilità del 21/10); DU-BX10…DU-BX13 (secondo giro del
# controllo) e DU-BX14 (seconda rimisura: la barra in fondo durante il salvataggio) con loro
SOLO_SE_SCELTE = {"69", "70", "71", "72", "73", "74", "75", "W1", "X1", "X2", "X3", "X4", "X5", "X6", "X7", "X8", "X9", "X10", "X11", "X12", "X13", "X14"}


@prova("69", "G39-03 [U]: lo STEP della BOM confermato per errore come sorgente di ACM-2000: S8 su ACM-1000 rifiutato (sorgente_spostabile) con il rimedio «Sposta la sorgente su ACM-1000…»; con la BOM di ACM-2000 confermata rifiuto bom_confermata con «Apri la revisione di ACM-2000»; la proposta con sorgente_di e lo spostamento (sposta_da, la revoca «sorgente errata» e la bozza annullata di là); «Usa nuovo STEP…» con la correzione della sorgente dichiarata e il segno nel riepilogo; la revoca che annulla la bozza nata dallo STEP")
def b69(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    a10 = b.al["ACM-1000_rev2.stp"]
    b.interruttore("step-proposto", True)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-step-proposto-sg").count() == 0:
        p.click("#du-sg-prodotto-apri")
    expect(p.locator("#du-step-proposto-sg")).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("#du-step-proposto-sg").get_attribute("data-sorgente-di") is None, "sorgente_di prima dello sbaglio")
    # 1. l'errore, fuori dalla pagina (la vista resta quella di prima): lo STEP della BOM confermato come sorgente di
    # ACM-2000, con la sua bozza nata da lì
    r = p.request.post(b.url + "/__banco/sorgente-s8", data=json.dumps({"prodotto": b.p2, "allegato": a1, "bozza": True}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda sorgente-s8 ha risposto %d" % r.status)
    bozza_p2 = b.stato()["bozze"][b.p2]["bozza_id"]
    # 2. S8 su ACM-1000 dalla proposta (vista vecchia): 422 sorgente_spostabile, niente scritto, il rimedio nel dialogo
    i = b.segna()
    p.click("#du-conferma-proposto-sg")
    expect(b.finestra()).to_contain_text("Conferma ACM-1000_assieme.stp come sorgente di ACM-1000", timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#du-finestra-errore")).to_contain_text("è la sorgente confermata di ACM-2000", timeout=T_LUNGO)
    expect(p.locator("#du-finestra-errore")).to_contain_text("sorgente_spostabile")
    verifica(b.stato()["s8"][b.p2]["valida"] is True and b.stato()["bozze"][b.p2]["stato"] == "aperta", "il rifiuto ha scritto")
    c0 = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c0) == 1 and "sposta_da" not in c0[0], "S8 senza il gesto ha mandato sposta_da: %s" % c0)
    # 3. il rimedio apre il dialogo dello spostamento: la spiegazione, nessun motivo (messaggio 46 [U])
    p.click("#f-rimedio-sposta")
    expect(b.finestra()).to_contain_text("Sposta la sorgente su ACM-1000", timeout=T_LUNGO)
    expect(p.locator("#f-sposta")).to_have_attribute("data-sposta-da", b.p2)
    expect(p.locator("#f-sposta")).to_contain_text("revoca la sorgente di ACM-2000 come «sorgente errata»")
    expect(p.locator("#f-sposta")).to_contain_text("annulla la bozza di ACM-2000")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    # 4. ACM-2000 ha già confermato la BOM con quello STEP: 422 bom_confermata, niente spostamento, il rimedio della revisione
    b.interruttore("bom-confermata-altro", True)
    try:
        i = b.segna()
        b.ok_finestra()
        expect(p.locator("#du-finestra-errore")).to_contain_text("è nella BOM confermata di ACM-2000", timeout=T_LUNGO)
        expect(p.locator("#f-rimedio-revisione")).to_contain_text("Apri la revisione di ACM-2000")
        c1 = b.corpi(i, "POST", r"/conferma-sorgente$")
        verifica(len(c1) == 1 and c1[0].get("sposta_da") == b.p2 and "motivo" not in c1[0], "corpo dello spostamento: %s" % c1)
        verifica(b.stato()["s8"][b.p2]["valida"] is True and b.stato()["bozze"][b.p2]["stato"] == "aperta", "bom_confermata ha scritto")
        p.click("#f-rimedio-revisione")
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        tab2 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)
        expect(tab2).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
        expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
    finally:
        b.interruttore("bom-confermata-altro", False)
    # 5. di nuovo su ACM-1000, riletta: la proposta dice di chi è adesso lo STEP e il suo gesto è lo spostamento
    tab1 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)
    tab1.click()
    expect(tab1).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-step-proposto-sg").count() == 0:
        p.click("#du-sg-prodotto-apri")
    box = p.locator("#du-step-proposto-sg")
    expect(box).to_have_attribute("data-sorgente-di", b.p2, timeout=T_LUNGO)
    expect(p.locator("#du-step-proposto-altro-sg")).to_contain_text("Adesso è la sorgente confermata di ACM-2000")
    expect(p.locator("#du-conferma-proposto-sg")).to_contain_text("Sposta la sorgente su ACM-1000…")
    b.foto("B69_sposta_sorgente.png")
    i = b.segna()
    p.click("#du-conferma-proposto-sg")
    expect(b.finestra()).to_contain_text("Sposta la sorgente su ACM-1000", timeout=T_LUNGO)
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Spostamento:", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("bozza di ACM-2000 annullata")
    c2 = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c2) == 1 and c2[0].get("sposta_da") == b.p2 and "motivo" not in c2[0] and "correzione_sorgente" not in c2[0],
             "corpo dello spostamento: %s" % c2)
    st = b.stato()
    verifica(st["s8"][b.p2]["valida"] is False and st["s8"][b.p2].get("causa") == "sorgente_errata" and st["bozze"][b.p2]["stato"] == "abbandonata"
             and st["bozze"][b.p2]["bozza_id"] == bozza_p2 and st["s8"][b.p1]["valida"] is True and st["s8"][b.p1]["allegato_id"] == a1,
             "lo spostamento non ha fatto la revoca, l'annullamento o la conferma: %s / %s" % (st["s8"], {k: v["stato"] for k, v in st["bozze"].items()}))
    expect(p.locator("#du-step-proposto-sg")).to_have_count(0)
    # 6. «Usa nuovo STEP per ACM-1000…» con la correzione della sorgente dichiarata (la spunta, mai spuntata): il segno nel
    # riepilogo (R4 effetto_sulla_fonte.correzione_sorgente)
    b.guarda(a10)
    expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-cambia-sorgente")
    expect(b.finestra()).to_contain_text("Usa nuovo STEP per ACM-1000", timeout=T_LUNGO)
    expect(p.locator("#f-correzione")).not_to_be_checked()
    expect(p.locator("#f-correzione-riga")).to_contain_text("una revisione interna, non una revisione del cliente")
    p.check("#f-correzione")
    i = b.segna()
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c3 = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c3) == 1 and c3[0].get("correzione_sorgente") is True and c3[0].get("sostituisci") == {"conferma": True}, "corpo della correzione: %s" % c3)
    b.passo("revisione")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-correzione-sorgente")).to_contain_text("revisione interna", timeout=T_LUNGO)
    expect(p.locator("#du-wz-correzione-sorgente")).to_contain_text("Non è una revisione del cliente")
    p.click("#du-wz-chiudi")
    # 7. la revoca della sorgente nuova: la bozza nata da lei si annulla (prima di R5), con la causa e la bozza nel corpo
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-revoca-sorgente").count() == 0:
        p.click("#du-sg-prodotto-apri")
    p.click("#du-revoca-sorgente")
    expect(p.locator("#f-revoca-bozza")).to_have_attribute("data-nata", "1", timeout=T_LUNGO)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("bozza nata da questo STEP è annullata", timeout=T_LUNGO)
    rv = b.corpi(i, "POST", r"/revoca-sorgente$")
    verifica(len(rv) == 1 and rv[0].get("sorgente_errata") is True and rv[0].get("bozza", {}).get("bozza_id") == b.stato()["bozze"][b.p1]["bozza_id"], "corpo della revoca: %s" % rv)
    verifica(b.stato()["bozze"][b.p1]["stato"] == "abbandonata", "la revoca non ha annullato la bozza nata dalla sorgente")
    b.interruttore("step-proposto", False)


@prova("70", "G39-01 e G39-02, parte 2: l'avviso della bozza manuale con gli STEP candidati di R1 (?suggerimenti_motore=1, anche non univoci); il 422 proposta_non_corrispondente rilegge i suggerimenti; «Accetta la proposta» dell'Elenco salvata «dal motore»; un file proposto dal motore associato a un altro nodo diventa «corretta»; i gruppi del riepilogo")
def b70(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    a2 = b.al["ACM-1100_A.pdf"]
    a3 = b.al["ACM-1120_B.pdf"]
    # 1. nessuna proposta univoca, ma R1 con ?suggerimenti_motore=1 dà lo STEP candidato sorgente: la bozza manuale resta
    # «molto sconsigliata» (G39-02 [U]); l'avviso arriva dopo la lettura, con la sua presa d'atto
    b.interruttore("step-candidati", True)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.passo("revisione")
        verifica(p.locator("#du-step-proposto-rev").count() == 0, "con il solo candidato la pagina mostra una proposta univoca")
        # R1 lento (rilievo minore della revisione indipendente 3 della corsia): finché la lettura dei candidati è in corso
        # la bozza manuale non parte; oltre il tempo massimo (12 s) parte con la nota, e l'avviso entra se R1 risponde dopo
        b.ritardo("GET /thread/{id}/motore-a/bozza", 2500)
        p.click("#du-da-vuota")
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "in_lettura", timeout=T_BREVE)
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "candidati", timeout=T_LUNGO)
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        p.locator("#du-finestra").get_by_role("button", name="Annulla").click()
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        b.ritardo("GET /thread/{id}/motore-a/bozza", 16000)
        p.click("#du-da-vuota")
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "in_lettura", timeout=T_BREVE)
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "scaduto", timeout=T_LUNGO)
        expect(p.locator("#f-step-candidati")).to_contain_text("non ha potuto controllare in tempo")
        expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "candidati", timeout=T_LUNGO)
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        p.locator("#du-finestra").get_by_role("button", name="Annulla").click()
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        i = b.segna()
        p.click("#du-da-vuota")
        expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "candidati", timeout=T_LUNGO)
        av = p.locator("#f-step-proposto")
        expect(av).to_contain_text("che il motore riconosce come sorgente: è molto sconsigliato partire senza")
        expect(av).to_contain_text("ACM-1000_assieme.stp")
        expect(av).to_have_attribute("data-candidati", a1)
        expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
        r1 = b.richieste_da(i, "GET", r"/motore-a/bozza$")
        verifica(any("suggerimenti_motore=1" in r["query"] for r in r1), "l'avviso non ha letto R1 con ?suggerimenti_motore=1: %s" % [r["query"] for r in r1])
        p.check("#f-senza-proposto")
        expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
        b.foto("B70_step_candidati.png")
        p.click("#f-vai-sorgente")
        expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
        expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step", timeout=T_BREVE)
        verifica(not b.richieste_da(i, "POST"), "l'avviso ha scritto: %s" % [r["path"] for r in b.richieste_da(i, "POST")])
    finally:
        b.interruttore("step-candidati", False)
    # senza candidati: nessun avviso e nessuna presa d'atto
    b.passo("revisione")
    p.click("#du-da-vuota")
    expect(p.locator("#f-step-candidati")).to_have_attribute("data-stato", "nessuno", timeout=T_LUNGO)
    expect(p.locator("#f-step-proposto")).to_have_count(0)
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    p.locator("#du-finestra").get_by_role("button", name="Annulla").click()
    expect(b.finestra()).to_be_hidden(timeout=T_BREVE)
    # 2. la bozza dallo STEP confermato (S8): il registro propone ACM-1100_A.pdf per ACM-1100 (k-7) e ACM-1120_B.pdf per
    # ACM-1120 (k-11), ciascuno con una sola destinazione
    b.guarda(a1)
    expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-conferma-sorgente")
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    imp = b.stato()["bozze"][b.p1]["snapshot"]["sorgente"]["importazione"]
    verifica(re.fullmatch(r"[0-9a-f]{64}", imp or ""), "la bozza dallo STEP senza registro: %r" % imp)
    # 3. il calcolo cambiato fra la lettura e il salvataggio (G39-01): 422 proposta_non_corrispondente con due modifiche
    # che dichiarano un registro, «Accetta la proposta» di ACM-1100_A.pdf e ACM-1120_B.pdf, proposto per ACM-1120 (k-11),
    # associato dall'ingegnere alla Piastrina (k-12). «Rileggi la bozza salvata» rilegge anche i suggerimenti e le tiene
    # da parte tutte e due, senza i registri (rilievo importante della revisione indipendente 3 della corsia: prima
    # sparivano in silenzio). «Riapplica» ricalcola i registri con i suggerimenti riletti: il server registra «motore» la
    # proposta accettata e «corretta» quella spostata su un altro nodo
    b.passo("revisione")
    b.vista("elenco")
    b.modo("r2-superato", "422")
    b.ritardo("POST /thread/{id}/motore-a/bozza", 4000)
    i = b.segna()
    p.click("#du-accetta-" + sid(a2))
    b.azione("sposta", a3)
    expect(p.locator("#f-provenienza-motore")).to_contain_text("su un altro nodo «corretta dall'ingegnere»", timeout=T_LUNGO)
    p.select_option("#f-pezzo", "k-12")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "rifiutata", timeout=T_LUNGO)
    expect(p.locator("#du-conflitto-testo")).to_contain_text("non vale più")
    cc = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(cc) == 1, "dopo il rifiuto la pagina ha salvato ancora: %d corpi" % len(cc))
    pb = [d for d in cc[0]["snapshot"]["documenti_preassociati"] if d["allegato_id"] == a2]
    verifica(pb == [{"allegato_id": a2, "chiave_bozza": "k-7", "importazione": imp}], "la PB accettata non dichiara il registro della proposta: %s" % pb)
    verifica(all(set(a["dopo"]) == {"allegato_id", "chiave_bozza"} for a in cc[0]["azioni"] if a["azione"] == "file_preassociato"), "le azioni portano il registro: %s" % cc[0]["azioni"])
    i = b.segna()
    p.click("#du-rileggi-bozza")
    b.attendi(lambda: any("suggerimenti_motore=1" in r["query"] for r in b.richieste_da(i, "GET", r"/motore-a/bozza$")),
              "dopo proposta_non_corrispondente R1 non è riletto con ?suggerimenti_motore=1")
    b.passo("revisione")
    expect(p.locator("#du-da-parte")).to_have_count(1, timeout=T_LUNGO)
    verifica(not b.stato()["bozze"][b.p1]["snapshot"].get("documenti_preassociati"), "il rifiuto ha scritto la PB")
    i = b.segna()
    p.click("#du-riapplica")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.attendi(lambda: {d["allegato_id"] for d in b.stato()["bozze"][b.p1]["snapshot"].get("documenti_preassociati", [])} >= {a2, a3}, "le modifiche riapplicate non sono salvate")
    expect(p.locator("#du-da-parte")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[-1]
    dich = {d["allegato_id"]: d for d in c["snapshot"]["documenti_preassociati"]}
    verifica(dich[a2] == {"allegato_id": a2, "chiave_bozza": "k-7", "importazione": imp} and dich[a3] == {"allegato_id": a3, "chiave_bozza": "k-12", "importazione": imp},
             "riapplicate, le PB non dichiarano il registro riletto (o scrivono la provenienza): %s" % dich)
    pb = {d["allegato_id"]: d for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"]}
    verifica(pb[a2].get("provenienza") == "motore" and pb[a2].get("importazione") == imp and pb[a2]["chiave_bozza"] == "k-7", "«Accetta la proposta» riapplicata non è «motore»: %s" % pb[a2])
    verifica(pb[a3]["chiave_bozza"] == "k-12" and pb[a3].get("provenienza") == "corretta" and pb[a3].get("importazione") == imp, "la proposta spostata su un altro nodo non è «corretta»: %s" % pb[a3])
    # 6. la pagina mostra la provenienza che il server restituisce: i gruppi del riepilogo (R4)
    b.passo("revisione")
    b.vista("org")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-gruppo-motore_intatta [data-du-pb='%s']" % a2)).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-wz-gruppo-corretta [data-du-pb='%s']" % a3)).to_be_visible()
    b.foto("B70_provenienze.png")
    p.click("#du-wz-chiudi")
    # 7. una PB legacy (la conversione di una pre-assegnazione delle pagine di prima, scritta dal server: sonda) spostata
    # dall'ingegnere sul nodo che il motore propone resta «manuale»: la pagina non dichiara il registro (regola [T-39]
    # «legacy → manuale» di provenienza3.go, rilievo minore della revisione indipendente 3 della corsia)
    r = p.request.post(b.url + "/__banco/provenienza", data=json.dumps({"allegato_id": a3, "provenienza": "legacy"}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda della provenienza: %d" % r.status)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.azione("sposta", a3)
    expect(p.locator("#f-provenienza-legacy")).to_contain_text("il server la registra «manuale»", timeout=T_LUNGO)
    expect(p.locator("#f-provenienza-motore")).to_have_count(0)
    p.select_option("#f-pezzo", "k-11")
    i = b.segna()
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.attendi(lambda: any(d["allegato_id"] == a3 and d["chiave_bozza"] == "k-11" for d in b.stato()["bozze"][b.p1]["snapshot"].get("documenti_preassociati", [])), "la PB legacy spostata non è salvata")
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[-1]
    dich = [d for d in c["snapshot"]["documenti_preassociati"] if d["allegato_id"] == a3]
    verifica(dich == [{"allegato_id": a3, "chiave_bozza": "k-11"}], "la PB legacy spostata dichiara il registro del motore: %s" % dich)
    pb = [d for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"] if d["allegato_id"] == a3]
    verifica(pb[0].get("provenienza") == "manuale" and not pb[0].get("importazione"), "la PB legacy spostata non è «manuale»: %s" % pb)


def testi_grezzi_wizard(p, chiavi):
    """UI46 [U] (messaggio 48): i testi del wizard che chi legge non deve vedere: le chiavi interne dei nodi, gli UUID,
    null, undefined, NaN, [object Object] e gli apostrofi al posto degli accenti («finche'», «e'», «gia'»)."""
    t = p.locator("#du-wizard").inner_text()
    out = []
    for k in sorted(set(chiavi)):
        if re.search(r"(?<![\w-])" + re.escape(k) + r"(?![\w-])", t):
            out.append("chiave " + k)
    for nome, pat in (("UUID", UUID), ("null", r"\bnull\b"), ("undefined", r"\bundefined\b"), ("NaN", r"\bNaN\b"), ("[object Object]", r"\[object Object\]"),
                      ("apostrofo per accento", r"[A-Za-z][aeiouAEIOU]'(?=[\s.,;:)»]|$)"), ("e' per è", r"(?<![A-Za-z'])[eE]'(?=\s)")):
        m = re.search(pat, t)
        if m:
            out.append("%s: «…%s…»" % (nome, t[max(0, m.start() - 40):m.end() + 20].replace("\n", " ")))
    return out


@prova("W1", "UI46 [U] (messaggio 48): il wizard aperto e scorso a metà non torna in cima né si ridisegna con la rilettura in sottofondo (R1 ?ingressi=1); una notifica nuova e la presa d'atto tengono scorrimento e fuoco; le preassociazioni mostrano codice e descrizione, mai la chiave del nodo, e i testi non hanno apostrofi al posto degli accenti")
def bW1(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    a2 = b.al["ACM-1100_A.pdf"]
    a3 = b.al["ACM-1120_B.pdf"]
    # la bozza dallo STEP confermato (S8) e due preassociazioni accettate dalla proposta (Elenco), come DU-B70
    b.guarda(a1)
    expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-conferma-sorgente")
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    b.passo("revisione")
    b.vista("elenco")
    p.click("#du-accetta-" + sid(a2))
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.click("#du-accetta-" + sid(a3))
    b.attendi(lambda: {d["allegato_id"] for d in b.stato()["bozze"][b.p1]["snapshot"].get("documenti_preassociati", [])} >= {a2, a3}, "le preassociazioni accettate non sono salvate")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("org")
    chiavi = [n["chiave_bozza"] for n in b.stato()["bozze"][b.p1]["snapshot"]["nodi"]]
    p.set_viewport_size({"width": 1024, "height": 640})
    try:
        b.apri_wizard()
        grezzi = testi_grezzi_wizard(p, chiavi)
        verifica(not grezzi, "fase 1 del wizard con testi grezzi: %s" % grezzi)
        p.click("#du-wz-fase2")
        riga = p.locator("#du-wz-documenti [data-du-pb='%s']" % a2)
        expect(riga).to_be_visible(timeout=T_LUNGO)
        # la riga della preassociazione: il codice del pezzo (ACM-1100) e la sua descrizione, non «(k-7)»
        verifica("ACM-1100" in riga.inner_text(), "la preassociazione non nomina il codice del pezzo: %r" % riga.inner_text())
        grezzi = testi_grezzi_wizard(p, chiavi)
        b.foto("BW1_wizard_fase2.png")
        riga.scroll_into_view_if_needed()
        b.foto("BW1_preassociazioni.png")
        verifica(not grezzi, "fase 2 del wizard con testi grezzi: %s" % grezzi)
        # lo scorrimento a metà del corpo del wizard; un segno sull'elemento dice se viene ridisegnato
        corpo = p.locator("#du-wz-corpo")
        alto = corpo.evaluate("c => { c.__segnoUI46 = 1; c.scrollTop = Math.round((c.scrollHeight - c.clientHeight) / 2); return c.scrollTop; }")
        verifica(alto > 50, "il corpo del wizard non scorre a 1024×640 (scrollTop %s): la prova non prova niente" % alto)
        # la rilettura in sottofondo con il wizard aperto (R1 ?ingressi=1, ogni 10 s) non cambia niente: il wizard resta com'è
        i = b.segna()
        b.attendi(lambda: [x for x in b.richieste_da(i, "GET", r"/motore-a/bozza$") if "ingressi=1" in x["query"]], "nessuna rilettura R1 ?ingressi=1 con il wizard aperto", ms=25000)
        p.wait_for_timeout(800)
        st = corpo.evaluate("c => ({ top: c.scrollTop, segno: c.__segnoUI46 === 1 })")
        b.foto("BW1_dopo_rilettura.png")
        verifica(abs(st["top"] - alto) <= 2, "la rilettura in sottofondo ha spostato lo scorrimento del wizard: da %s a %s" % (alto, st["top"]))
        verifica(st["segno"], "la rilettura in sottofondo ha ridisegnato il wizard senza che niente cambiasse")
        # un documento nuovo in analisi (N1): il wizard si ridisegna con la notifica, lo scorrimento non torna in cima
        r = p.request.post(b.url + "/__banco/documento-nuovo", data=json.dumps({"nome": "ACM-NOTA_W1.pdf"}), headers={"Content-Type": "application/json"})
        verifica(r.status == 200, "la sonda non ha aggiunto il documento: %d" % r.status)
        expect(p.locator("#du-wz-n1")).to_contain_text("ACM-NOTA_W1.pdf", timeout=25000)
        top = p.locator("#du-wz-corpo").evaluate("c => c.scrollTop")
        verifica(top >= alto - 2, "con la notifica N1 il wizard è tornato su: scrollTop %s, prima %s" % (top, alto))
        # la presa d'atto spuntata da tastiera: il wizard si ridisegna, scorrimento e fuoco restano
        p.focus("#du-wz-presa-analisi")
        alto = p.locator("#du-wz-corpo").evaluate("c => { c.scrollTop = Math.round((c.scrollHeight - c.clientHeight) / 2); return c.scrollTop; }")
        p.keyboard.press("Space")
        expect(p.locator("#du-wz-presa-analisi")).to_be_checked()
        st = p.evaluate("() => ({ top: document.querySelector('#du-wz-corpo').scrollTop, fuoco: document.activeElement && document.activeElement.id })")
        verifica(abs(st["top"] - alto) <= 2, "la presa d'atto ha spostato lo scorrimento del wizard: da %s a %s" % (alto, st["top"]))
        verifica(st["fuoco"] == "du-wz-presa-analisi", "la presa d'atto ha perso il fuoco: %r" % st["fuoco"])
        # cambiare fase riporta in cima (è un'altra pagina del wizard)
        p.click("#du-wz-fase1")
        verifica(p.locator("#du-wz-corpo").evaluate("c => c.scrollTop") == 0, "la fase 1 non parte dall'inizio")
        p.request.post(b.url + "/__banco/analisi-conclusa", data=json.dumps({"nome": "ACM-NOTA_W1.pdf", "cambia": False}), headers={"Content-Type": "application/json"})
        p.click("#du-wz-chiudi")
        expect(p.locator("#du-wizard")).to_be_hidden()
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


# ---------------------------------------------------------------- UI46, controllo di stabilità del 21/10 (X1-X6)
# «Controlla che l'UI non abbia problemi come "rileggeva i dati ogni 5-10 secondi" o problemi che quando si clicca un box
# o bottone la UI fa dei refresh che spostano la pagina» [U]. Una prova per ogni difetto trovato dai controllori
# sull'anteprima 2cedb10 (S1, S2, S3, R2 e i minori): ognuna misura scorrimento, fuoco e ridisegni prima e dopo il gesto,
# con clic veri del mouse sulle coordinate (senza lo scorrimento automatico di Playwright, che sporcherebbe la misura).
# Girano solo se scelte (--prove X1,...), in quest'ordine, su un banco loro: TestDistintaUnicaStabilitaNelBrowser.

SONDA_X = r"""() => {
  const c = document.querySelector('#du-corpo');
  if (!c) return false;
  if (window.__x) window.__x.oss.disconnect();
  const x = window.__x = { ridisegni: 0, oss: null };
  x.oss = new MutationObserver((ms) => { for (const m of ms) if (m.type === 'childList' && (m.addedNodes.length || m.removedNodes.length)) x.ridisegni++; });
  x.oss.observe(c, { childList: true });
  return true;
}"""


def x_sonda(p):
    verifica(p.evaluate(SONDA_X), "la sonda dei ridisegni non trova #du-corpo")


def x_ridisegni(p):
    return p.evaluate("() => window.__x ? window.__x.ridisegni : -1")


def x_top(p, sel):
    return p.evaluate("(s) => { const e = document.querySelector(s); return e ? Math.round(e.scrollTop) : null; }", sel)


def x_meta(p, sel, minimo=40):
    """Porta a metà lo scorrimento del riquadro sel; la prova non prova niente se il riquadro non scorre."""
    r = p.evaluate("(s) => { const e = document.querySelector(s); if (!e) return null; const max = e.scrollHeight - e.clientHeight;"
                   " e.scrollTop = Math.round(max / 2); return { top: Math.round(e.scrollTop), max }; }", sel)
    verifica(r is not None, "%s non c'è" % sel)
    verifica(r["max"] >= minimo, "%s non scorre abbastanza (max %s): la prova non prova niente" % (sel, r["max"]))
    p.wait_for_timeout(150)
    return r["top"]


def x_fuoco(p):
    return p.evaluate("() => { const a = document.activeElement; return a && a !== document.body ? (a.id || a.tagName) : 'BODY'; }")


def x_y(p, sel):
    return p.evaluate("(s) => { const e = document.querySelector(s); return e ? Math.round(e.getBoundingClientRect().top) : null; }", sel)


def x_ancora(p, sel, filtro="[id]"):
    """La prima riga o sezione con un id che comincia dentro il riquadro sel (sotto la testata appiccicata dell'elenco):
    il punto che chi legge sta guardando."""
    return p.evaluate("""([s, f]) => { const e = document.querySelector(s); if (!e) return null; const b = e.getBoundingClientRect();
      const t = e.querySelector('.du-sg-elenco-t'); const su = b.top + (t ? t.getBoundingClientRect().height : 0);
      for (const x of e.querySelectorAll(f)) { if (x.closest('.du-sg-elenco-t')) continue; const r = x.getBoundingClientRect();
        if (r.height && r.top >= su && r.bottom <= b.bottom) return x.id; } return null; }""", [sel, filtro])


def x_y_id(p, ident):
    return p.evaluate("(i) => { const e = document.getElementById(i); return e ? Math.round(e.getBoundingClientRect().top) : null; }", ident)


def x_fermo(p, sel, fai, cosa, filtro="[id]"):
    """Il gesto fai non sposta quello che si vede nel riquadro sel (scorso a metà prima): l'ancora resta alla stessa
    altezza sullo schermo e il riquadro non torna in cima."""
    alto = x_meta(p, sel)
    anc = x_ancora(p, sel, filtro)
    verifica(anc, "nessuna ancora con un id in vista in %s" % sel)
    y0 = x_y_id(p, anc)
    fai()
    p.wait_for_timeout(400)
    y1 = x_y_id(p, anc)
    top = x_top(p, sel)
    verifica(y1 is not None and abs(y1 - y0) <= 2 and (top or 0) > 0,
             "%s ha spostato %s: %s da y %s a %s, scrollTop da %s a %s" % (cosa, sel, anc, y0, y1, alto, top))
    return anc


def x_clic(p, sel, **kw):
    """Clic vero del mouse al centro dell'elemento, che deve essere già nel riquadro."""
    bb = p.locator(sel).first.bounding_box()
    vh = p.viewport_size["height"]
    verifica(bb is not None and bb["y"] >= 0 and bb["y"] + bb["height"] <= vh, "%s non è nel riquadro (%s)" % (sel, bb))
    p.mouse.click(bb["x"] + bb["width"] / 2, bb["y"] + bb["height"] / 2, **kw)


def x_riga_visibile(p, escludi=()):
    """Una riga dell'elenco della Sorgente tutta visibile nel riquadro dell'elenco, sotto la testata appiccicata."""
    return p.evaluate("""(escludi) => {
      const box = document.querySelector('#du-sg-elenco'); if (!box) return null;
      const b = box.getBoundingClientRect(); const t = box.querySelector('.du-sg-elenco-t');
      const su = b.top + (t ? t.getBoundingClientRect().height : 0) + 4;
      const righe = [...box.querySelectorAll('[role=row][id^=du-file-]')].filter((r) => !escludi.includes(r.id));
      const vis = righe.filter((r) => { const q = r.getBoundingClientRect(); return q.top >= su && q.bottom <= b.bottom - 4; });
      return vis.length ? vis[Math.floor(vis.length / 2)].id : null; }""", list(escludi))


def x_in_analisi(b, nome):
    """Un PDF nuovo in analisi (sonda documento-nuovo) e la pagina riletta: S1 si rilegge ogni 5 s finché resta in analisi."""
    p = b.page
    r = p.request.post(b.url + "/__banco/documento-nuovo", data=json.dumps({"nome": nome}), headers={"Content-Type": "application/json"})
    verifica(r.status == 200, "la sonda non ha aggiunto il documento: %d" % r.status)
    b.apri()
    if p.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
        b.passo("sorgente")
    i = b.segna()
    b.attendi(lambda: [x for x in b.richieste_da(i, "GET", r"/sorgente$") if "scartati" in x["query"]], "S1 non si rilegge con un file in analisi", ms=15000)


def x_conclusa(b, nome):
    b.page.request.post(b.url + "/__banco/analisi-conclusa", data=json.dumps({"nome": nome, "cambia": False}), headers={"Content-Type": "application/json"})


def x_bozza(b):
    """La Revisione con la bozza dallo STEP confermato (S8), come DU-BW1; la crea solo se non c'è."""
    p = b.page
    if not b.stato()["bozze"].get(b.p1):
        b.guarda(b.al["ACM-1000_assieme.stp"])
        expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
        p.click("#du-conferma-sorgente")
        b.ok_finestra()
        expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    if p.locator("#du-passo-revisione[aria-current=step]").count() == 0:
        b.passo("revisione")
    b.vista("org")
    expect(p.locator("#nc-k-12")).to_be_visible(timeout=T_LUNGO)


@prova("X1", "UI46 [U] (stabilità, S1): con un file in analisi S1 si rilegge ogni 5 s per 60 s senza ridisegnare la Sorgente: l'elenco scorso a metà resta dov'è e il fuoco resta sulla riga")
def bX1(b):
    p = b.page
    nome = "ACM-NOTA_X1.pdf"
    p.set_viewport_size({"width": 1280, "height": 600})
    try:
        x_in_analisi(b, nome)
        b.cartella("tutti")
        p.wait_for_timeout(300)
        alto = x_meta(p, "#du-sg-elenco")
        riga = x_riga_visibile(p)
        verifica(riga, "nessuna riga visibile nell'elenco scorso")
        p.evaluate("(id) => document.getElementById(id).focus({ preventScroll: true })", riga)
        alto = x_top(p, "#du-sg-elenco")
        x_sonda(p)
        i = b.segna()
        p.wait_for_timeout(60000)
        letture = [x for x in b.richieste_da(i, "GET", r"/sorgente$") if "scartati" in x["query"]]
        n = x_ridisegni(p)
        top = x_top(p, "#du-sg-elenco")
        fuoco = x_fuoco(p)
        b.foto("BX1_dopo_60s.png")
        verifica(len(letture) >= 10, "in 60 s S1 si è riletto %d volte: il ciclo di 5 s non gira, la prova non prova niente" % len(letture))
        verifica(n == 0, "in 60 s con %d riletture di S1 senza cambiamenti la Sorgente si è ridisegnata %d volte" % (len(letture), n))
        verifica(abs(top - alto) <= 2, "l'elenco scorso a metà si è spostato da solo: scrollTop da %s a %s" % (alto, top))
        verifica(fuoco == riga, "il fuoco non è più sulla riga %s: %s" % (riga, fuoco))
        # la fine dell'analisi cambia la firma di S1: un ridisegno (voluto), l'elenco resta nella stessa cartella e dov'era
        riga_nuova = p.locator("#du-sg-elenco [role=row][data-allegato]", has_text=nome)
        verifica(riga_nuova.count() == 1 and "in analisi" in riga_nuova.inner_text(), "la riga di %s non dice «in analisi»: %r" % (nome, riga_nuova.inner_text() if riga_nuova.count() else None))
        x_conclusa(b, nome)
        b.attendi(lambda: x_ridisegni(p) > 0, "la fine dell'analisi non ha ridisegnato la Sorgente", ms=15000)
        p.wait_for_timeout(1200)
        top = x_top(p, "#du-sg-elenco")
        verifica(abs(top - alto) <= 2, "alla fine dell'analisi l'elenco è tornato su: scrollTop da %s a %s" % (alto, top))
        # e quello che si vede è aggiornato: la riga non è più in analisi e «Dal sistema» lo dice
        b.attendi(lambda: "in analisi" not in riga_nuova.inner_text(), "alla fine dell'analisi la riga di %s dice ancora «in analisi»" % nome, ms=8000)
        sis = p.locator("#du-sg-sistema").inner_text()
        verifica(nome in sis and "letto dal Motore A" in sis, "«Dal sistema» non dice che %s è stato letto: %r" % (nome, sis))
    finally:
        x_conclusa(b, nome)
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X2", "UI46 [U] (stabilità, S1): con un file in analisi la ricerca della Sorgente scritta con la tastiera non perde tasti né il fuoco quando S1 si rilegge fra un tasto e l'altro")
def bX2(b):
    p = b.page
    nome = "ACM-NOTA_X2.pdf"
    p.set_viewport_size({"width": 1280, "height": 600})
    try:
        x_in_analisi(b, nome)
        testo = "ACM-1100"
        c = p.locator("#du-sg-cerca")
        c.click()
        c.fill("")
        x_sonda(p)
        i = b.segna()
        # i primi tasti, ciascuno seguito da una rilettura di S1 (il ciclo di 5 s) prima del tasto dopo; poi gli altri,
        # uno ogni 1,2 s come chi scrive piano
        for n, t in enumerate(testo):
            p.keyboard.type(t)
            if n < 3:
                p.wait_for_timeout(400)
                k = len([x for x in b.richieste_da(i, "GET", r"/sorgente$") if "scartati" in x["query"]])
                b.attendi(lambda: len([x for x in b.richieste_da(i, "GET", r"/sorgente$") if "scartati" in x["query"]]) > k, "S1 non si rilegge durante la digitazione", ms=12000)
                p.wait_for_timeout(500)
            else:
                p.wait_for_timeout(1200)
        letture = [x for x in b.richieste_da(i, "GET", r"/sorgente$") if "scartati" in x["query"]]
        valore = p.locator("#du-sg-cerca").input_value()
        fuoco = x_fuoco(p)
        b.foto("BX2_ricerca.png")
        verifica(valore == testo, "la ricerca ha perso dei tasti: «%s» invece di «%s» (%d riletture di S1, %d ridisegni)" % (valore, testo, len(letture), x_ridisegni(p)))
        verifica(fuoco == "du-sg-cerca", "la ricerca ha perso il fuoco: %s" % fuoco)
        p.keyboard.press("Escape")
    finally:
        x_conclusa(b, nome)
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X3", "UI46 [U] (stabilità, S3): nella Sorgente scegliere un file, spuntarlo, «Togli le spunte» e «+ Aggiungi file» nella stessa cartella non spostano l'elenco; la riga scelta resta in vista e il fuoco resta; un'altra cartella riparte dall'inizio")
def bX3(b):
    p = b.page
    el = "#du-sg-elenco"
    righe = "[role=row][id^=du-file-]"
    p.set_viewport_size({"width": 1280, "height": 600})
    try:
        b.apri()
        if p.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
            b.passo("sorgente")
        b.cartella("tutti")
        p.wait_for_timeout(300)
        # scegliere un file a metà dell'elenco: la riga scelta resta dov'è, in vista, con il fuoco
        stato = {}

        def scegli():
            stato["riga"] = x_riga_visibile(p)
            x_clic(p, "#%s .du-fr-nome" % stato["riga"])
            expect(p.locator("#du-sg-file")).to_have_attribute("data-allegato", p.locator("#" + stato["riga"]).get_attribute("data-allegato"), timeout=T_BREVE)
        x_fermo(p, el, scegli, "scegliere un file", righe)
        riga = stato["riga"]
        vis = p.evaluate("(id) => { const r = document.getElementById(id).getBoundingClientRect(); const b = document.querySelector('#du-sg-elenco').getBoundingClientRect(); return r.top >= b.top && r.bottom <= b.bottom; }", riga)
        b.foto("BX3_scelta.png")
        verifica(vis, "la riga scelta %s è fuori vista" % riga)
        verifica(x_fuoco(p) == riga, "dopo la scelta il fuoco non è sulla riga: %s" % x_fuoco(p))

        # spuntare un'altra riga (compare la barra delle azioni di gruppo)
        def spunta():
            stato["altra"] = x_riga_visibile(p, escludi=(riga,))
            x_clic(p, "#%s .du-sg-spunta" % stato["altra"])
            expect(p.locator("#%s .du-sg-spunta" % stato["altra"])).to_be_checked()
        x_fermo(p, el, spunta, "la spunta", righe)
        altra = stato["altra"]
        b.foto("BX3_spunta.png")
        verifica(x_fuoco(p) == altra + "-spunta", "dopo la spunta il fuoco non è sulla casella: %s" % x_fuoco(p))

        # una seconda spunta: il dettaglio mostra la selezione con le sue azioni
        def spunta2():
            stato["terza"] = x_riga_visibile(p, escludi=(riga, altra))
            x_clic(p, "#%s .du-sg-spunta" % stato["terza"])
            expect(p.locator("#%s .du-sg-spunta" % stato["terza"])).to_be_checked()
        x_fermo(p, el, spunta2, "la seconda spunta", righe)

        # «Togli le spunte» (dal dettaglio, dove la selezione è in vista)
        def togli():
            x_clic(p, "#du-sg-togli-spunte-d")
            expect(p.locator("#%s .du-sg-spunta" % altra)).not_to_be_checked()
        x_fermo(p, el, togli, "«Togli le spunte»", righe)

        # «+ Aggiungi file» aperto e chiuso
        def aggiungi():
            x_clic(p, "#du-sg-aggiungi")
            expect(p.locator("#du-carica-file")).to_be_attached(timeout=T_BREVE)
            x_clic(p, "#du-sg-aggiungi")
            expect(p.locator("#du-carica-file")).to_have_count(0)
        x_fermo(p, el, aggiungi, "«+ Aggiungi file»", righe)
        verifica(x_fuoco(p) == "du-sg-aggiungi", "dopo «+ Aggiungi file» il fuoco non è sul pulsante: %s" % x_fuoco(p))
        # un'altra cartella riparte dall'inizio (gesto voluto)
        b.cartella("step")
        b.cartella("tutti")
        verifica(x_top(p, el) == 0, "tornando alla cartella l'elenco non riparte dall'inizio")
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X4", "UI46 [U] (stabilità, S2): il pannello del nodo scorso a metà resta dov'è con la stessa scheda cliccata, la casella di selezione, «Confronto», l'OK di «Quantità…» e la chiusura del wizard; un altro nodo riparte dall'inizio")
def bX4(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 640})
    pn = ".du-zona--nodo"
    try:
        x_bozza(b)
        b.scheda("k-12")
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-12", timeout=T_BREVE)
        # la tela in vista nella finestra (il pannello resta appiccicato accanto)
        p.evaluate("() => { const v = document.getElementById('du-vp'); window.scrollTo(0, Math.max(0, v.getBoundingClientRect().top + scrollY - 10)); }")
        p.wait_for_timeout(200)
        x_fermo(p, pn, lambda: x_clic(p, "#nc-k-12 .du-nc-mini"), "la stessa scheda cliccata di nuovo")
        x_fermo(p, pn, lambda: x_clic(p, "#sel-k-31"), "la casella di selezione (spunta)")
        x_fermo(p, pn, lambda: x_clic(p, "#sel-k-31"), "la casella di selezione (toglie)")
        x_fermo(p, pn, lambda: x_clic(p, "#du-confronto"), "«Confronto» acceso")
        x_fermo(p, pn, lambda: x_clic(p, "#du-confronto"), "«Confronto» spento")
        # un altro nodo riparte dall'inizio del pannello (gesto voluto)
        b.scheda("k-41")
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-41", timeout=T_BREVE)
        verifica(x_top(p, pn) == 0, "un altro nodo non riparte dall'inizio del pannello")
        # l'OK di «Quantità…» su k-41: il salvataggio R2, poi R4 in sottofondo
        p.click("#du-quantita")
        expect(b.finestra()).to_be_visible()
        p.fill("#f-qta", "5")

        def ok_quantita():
            i = b.segna()
            b.ok_finestra()
            b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "l'OK di Quantità non ha salvato")
            expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
            p.wait_for_timeout(2500)
        x_fermo(p, pn, ok_quantita, "l'OK di Quantità")
        b.foto("BX4_dopo_quantita.png")
        # e quello che si vede è aggiornato: ×5 sul legame nella tela e nel pannello
        pillole = p.locator("#du-vp .du-oq").evaluate_all("xs => xs.map(x => x.getAttribute('aria-label') || '')")
        verifica(any("×5" in x for x in pillole), "dopo l'OK di Quantità nessun legame dice ×5: %s" % pillole)
        verifica("×5" in p.locator("#du-pannello").inner_text(), "dopo l'OK di Quantità il pannello di k-41 non dice ×5")

        # il wizard aperto e chiuso con il pannello scorso: il pannello resta, il fuoco torna al pulsante che lo apre
        def wizard():
            b.apri_wizard()
            p.click("#du-wz-chiudi")
            expect(p.locator("#du-wizard")).to_be_hidden()
        x_fermo(p, pn, wizard, "la chiusura del wizard")
        verifica(x_fuoco(p) == "du-cta-bom", "chiuso il wizard il fuoco non torna a «Rivedi e conferma BOM»: %s" % x_fuoco(p))
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


def x_trascina(p, da, su):
    s = p.locator("#nc-" + da).bounding_box()
    d = p.locator("#nc-" + su).bounding_box()
    p.mouse.move(s["x"] + 40, s["y"] + 10)
    p.mouse.down()
    p.mouse.move(s["x"] + 60, s["y"] + 30, steps=4)
    p.mouse.move(d["x"] + 40, d["y"] + 30, steps=10)
    p.mouse.up()


@prova("X5", "UI46 [U] (stabilità, R2): chiuso il dialogo aperto trascinando una scheda, la pagina non salta verso un pulsante lontano e il fuoco torna sulla scheda; lo stesso per il dialogo aperto dal menu del nodo")
def bX5(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 480})
    try:
        x_bozza(b)
        b.scheda("k-12")
        p.wait_for_timeout(300)
        # il fuoco su un pulsante in alto, poi la pagina scorsa fino alla tela
        p.evaluate("() => { window.scrollTo(0, 0); document.getElementById('du-aggiorna').focus(); }")
        p.evaluate("() => { const v = document.getElementById('du-vp'); window.scrollTo(0, Math.max(0, v.getBoundingClientRect().top + scrollY - 20)); }")
        p.wait_for_timeout(200)
        y0 = p.evaluate("() => Math.round(scrollY)")
        verifica(y0 > 100, "la pagina non scorre abbastanza a 1280×480 (scrollY %s): la prova non prova niente" % y0)
        aperto = False
        for dest in ("k-40", "k-11", "k-root"):
            if p.locator("#nc-" + dest).count() == 0:
                continue
            x_trascina(p, "k-12", dest)
            p.wait_for_timeout(500)
            if b.finestra().is_visible():
                aperto = True
                break
        verifica(aperto, "il trascinamento di k-12 non ha aperto il dialogo «Sposta…»")
        y1 = p.evaluate("() => Math.round(scrollY)")
        p.keyboard.press("Escape")
        expect(b.finestra()).to_be_hidden()
        p.wait_for_timeout(200)
        y2 = p.evaluate("() => Math.round(scrollY)")
        fuoco = x_fuoco(p)
        b.foto("BX5_dopo_esc.png")
        verifica(abs(y2 - y1) <= 2, "chiuso il dialogo del trascinamento la pagina è saltata: scrollY da %s a %s" % (y1, y2))
        verifica(fuoco == "oc-k-12", "chiuso il dialogo del trascinamento il fuoco non torna sulla scheda: %s" % fuoco)
        # il dialogo dal menu del nodo
        bb = p.locator("#nc-k-41").bounding_box()
        p.mouse.click(bb["x"] + 30, bb["y"] + 30, button="right")
        expect(p.locator("#du-menu-nodo")).to_be_visible()
        voce = p.locator("#du-menu-nodo button:not([disabled])", has_text="Quantit")
        verifica(voce.count() > 0, "il menu del nodo di k-41 non ha «Quantità…»")
        y1 = p.evaluate("() => Math.round(scrollY)")
        voce.first.click()
        expect(b.finestra()).to_be_visible()
        p.keyboard.press("Escape")
        expect(b.finestra()).to_be_hidden()
        p.wait_for_timeout(200)
        y2 = p.evaluate("() => Math.round(scrollY)")
        fuoco = x_fuoco(p)
        verifica(abs(y2 - y1) <= 2, "chiuso il dialogo del menu del nodo la pagina è saltata: scrollY da %s a %s" % (y1, y2))
        verifica(fuoco == "oc-k-41", "chiuso il dialogo del menu del nodo il fuoco non torna sulla scheda: %s" % fuoco)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X6", "UI46 [U] (stabilità, minore): la barra «N nodi selezionati» che compare alla prima spunta e sparisce con «Svuota» non sposta la tela")
def bX6(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 820})
    try:
        b.apri()
        x_bozza(b)
        p.click("#du-zoom-adatta")
        p.evaluate("window.scrollTo(0, 0)")
        p.wait_for_timeout(300)
        verifica(p.locator("#du-selezione").count() == 0, "c'è già una selezione: la prova non prova niente")
        y0 = x_y(p, "#du-vp")
        x_clic(p, "#sel-k-31")
        expect(p.locator("#du-selezione")).to_contain_text("1 nodi selezionati")
        y1 = x_y(p, "#du-vp")
        b.foto("BX6_selezione.png")
        verifica(abs(y1 - y0) <= 2, "la barra «N nodi selezionati» ha spostato la tela: y da %s a %s" % (y0, y1))
        x_clic(p, "#du-svuota-selezione")
        expect(p.locator("#du-selezione")).to_have_count(0)
        y2 = x_y(p, "#du-vp")
        verifica(abs(y2 - y0) <= 2, "«Svuota» ha spostato la tela: y da %s a %s" % (y0, y2))
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X7", "UI46 [U] (stabilità, minore): la striscia dell'esito di un trascinamento rifiutato, con la pagina in cima, non spinge giù la tela")
def bX7(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 820})
    try:
        b.apri()
        x_bozza(b)
        p.click("#du-zoom-adatta")
        p.evaluate("window.scrollTo(0, 0)")
        p.wait_for_timeout(300)
        verifica(not b.avviso().is_visible(), "la striscia dell'esito è già visibile: la prova non prova niente")
        y0 = x_y(p, "#du-vp")
        x_trascina(p, "k-12", "k-41")
        expect(b.avviso()).to_be_visible()
        expect(b.avviso()).to_contain_text("Lo spostamento non si fa")
        y1 = x_y(p, "#du-vp")
        b.foto("BX7_esito.png")
        verifica(abs(y1 - y0) <= 2, "la striscia dell'esito ha spinto giù la tela: y da %s a %s" % (y0, y1))
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X8", "UI46 [U] (stabilità, minore): dopo «Guarda l'anteprima» dalla riga dello STEP il fuoco va sull'anteprima, non sul body")
def bX8(b):
    p = b.page
    b.apri()
    b.guarda(b.al["ACM-1000_assieme.stp"])
    p.wait_for_timeout(300)
    fuoco = x_fuoco(p)
    b.foto("BX8_anteprima.png")
    verifica(fuoco != "BODY", "dopo «Guarda l'anteprima» il fuoco è sul body")
    verifica(p.evaluate("() => document.getElementById('du-anteprima').contains(document.activeElement) || document.activeElement.id === 'du-sg-riduci'"),
             "dopo «Guarda l'anteprima» il fuoco non è sull'anteprima: %s" % fuoco)


@prova("X9", "UI46 [U] (stabilità, R2): un dialogo della Sorgente chiuso con Esc dopo che la fine di un'analisi ha ridisegnato la pagina sotto riporta il fuoco al pulsante che l'ha aperto (ritrovato per id), senza far scorrere niente")
def bX9(b):
    p = b.page
    nome = "ACM-NOTA_X9.pdf"
    a = b.al["collaudo_interno.pdf"]
    try:
        x_in_analisi(b, nome)
        b.azione("scarta", a)
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        x_sonda(p)
        # la fine dell'analisi cambia la firma di S1: la Sorgente si ridisegna sotto il dialogo aperto
        x_conclusa(b, nome)
        b.attendi(lambda: x_ridisegni(p) > 0, "la fine dell'analisi non ha ridisegnato la Sorgente", ms=15000)
        p.wait_for_timeout(900)
        expect(b.finestra()).to_be_visible()
        y0 = p.evaluate("() => Math.round(scrollY)")
        p.keyboard.press("Escape")
        expect(b.finestra()).to_be_hidden()
        fuoco = x_fuoco(p)
        verifica(fuoco == "du-scarta-" + sid(a), "chiuso il dialogo il fuoco non torna a «Scarta…»: %s" % fuoco)
        verifica(abs(p.evaluate("() => Math.round(scrollY)") - y0) <= 2, "chiuso il dialogo la pagina è scorsa")
    finally:
        x_conclusa(b, nome)


def x_idc(p):
    """L'id. cliente nel pannello del nodo (sezione «Decisione dell'ingegnere»), o None se non c'è."""
    return p.evaluate("() => { const e = document.querySelector('#du-pannello #du-sez-decisione [data-du-idc]'); return e ? e.dataset.duIdc : null; }")


def x_codice(p, k):
    """Il codice sulla scheda del pezzo (la prima riga del pulsante #oc-‹k›)."""
    return (p.locator("#oc-" + sid(k)).inner_text().strip().splitlines() or [""])[0].strip()


@prova("X10", "UI46 [U] (stabilità, secondo giro, bloccante): dopo «Correggi…» e dopo «Annulla» l'id. cliente del pannello del nodo segue il codice della scheda, senza un altro gesto; la stessa scheda cliccata non lascia un id. cliente vecchio")
def bX10(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 720})
    try:
        x_bozza(b)
        b.scheda("k-41")
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-41", timeout=T_BREVE)
        # l'id. cliente lo dà R4, che parte dopo il primo salvataggio: all'apertura il pannello può non averlo
        cod0 = x_codice(p, "k-41")
        verifica(cod0 and cod0 != "ACM-1888", "codice della scheda k-41 inatteso: %r" % cod0)
        p.click("#du-correggi")
        expect(b.finestra()).to_be_visible()
        p.fill("#f-codice", "ACM-1888")
        b.ok_finestra()
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        b.attendi(lambda: x_codice(p, "k-41").startswith("ACM-1888"), "dopo «Correggi…» la scheda non dice ACM-1888")
        b.attendi(lambda: "ACM-1888" in (x_idc(p) or ""), "dopo «Correggi…» il pannello resta sull'id. cliente di prima: %r" % x_idc(p), ms=8000)
        b.foto("BX10_dopo_correggi.png")
        p.click("#du-annulla")
        b.attendi(lambda: x_codice(p, "k-41") == cod0, "dopo «Annulla» la scheda non torna a %s" % cod0)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        b.attendi(lambda: cod0 in (x_idc(p) or "") and "ACM-1888" not in (x_idc(p) or ""), "dopo «Annulla» il pannello resta su un id. cliente vecchio: %r (scheda %s)" % (x_idc(p), cod0), ms=8000)
        # la stessa scheda cliccata adesso: l'id. cliente resta quello giusto
        x_clic(p, "#nc-k-41 .du-nc-mini")
        p.wait_for_timeout(500)
        verifica(cod0 in (x_idc(p) or ""), "la stessa scheda cliccata mostra un id. cliente vecchio: %r" % x_idc(p))
        b.foto("BX10_dopo_annulla.png")
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X11", "UI46 [U] (stabilità, secondo giro): con il fuoco su un gesto del pannello di un nodo, il clic su un'altra scheda non lascia il fuoco sul gesto con lo stesso id del nodo nuovo (un Invio non apre il suo dialogo): va sulla scheda")
def bX11(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 720})
    try:
        x_bozza(b)
        for gesto in ("du-quantita", "du-correggi"):
            b.scheda("k-41")
            expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-41", timeout=T_BREVE)
            # il gesto aperto e chiuso con Esc: il fuoco torna sul suo pulsante nel pannello di k-41
            p.locator("#" + gesto).focus()
            p.keyboard.press("Enter")
            expect(b.finestra()).to_be_visible()
            p.keyboard.press("Escape")
            expect(b.finestra()).to_be_hidden()
            verifica(x_fuoco(p) == gesto, "chiuso il dialogo il fuoco non torna su %s: %s" % (gesto, x_fuoco(p)))
            x_clic(p, "#nc-k-12 .du-nc-mini")
            expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-12", timeout=T_BREVE)
            p.wait_for_timeout(300)
            fuoco = x_fuoco(p)
            p.keyboard.press("Enter")
            p.wait_for_timeout(400)
            aperto = b.finestra().is_visible()
            titolo = p.locator("#du-finestra-titolo").inner_text() if aperto else ""
            if aperto:
                p.keyboard.press("Escape")
                expect(b.finestra()).to_be_hidden()
            verifica(not aperto, "dopo il clic sulla scheda k-12 (fuoco su %s) un Invio ha aperto «%s»" % (fuoco, titolo))
            verifica(fuoco == "oc-k-12", "dopo il clic sulla scheda k-12 il fuoco è su %s (attesa la scheda oc-k-12)" % fuoco)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X12", "UI46 [U] (stabilità, secondo giro, [U] M32): con il posto della striscia dell'esito riservato, la pagina della Sorgente non scorre fra 620 e 660 px di altezza (come prima della correzione)")
def bX12(b):
    p = b.page
    try:
        b.apri()
        if p.locator("#du-passo-sorgente[aria-current=step]").count() == 0:
            b.passo("sorgente")
        b.cartella("tutti")
        fuori = []
        for w in (1366, 1920):
            for hh in (620, 640, 660):
                p.set_viewport_size({"width": w, "height": hh})
                p.wait_for_timeout(350)
                ecc = p.evaluate("() => document.documentElement.scrollHeight - innerHeight")
                if ecc > 1:
                    fuori.append((w, hh, ecc, p.evaluate("""() => { const a = document.getElementById('du-avviso'), s = document.getElementById('du-sg');
                      return 'avviso ' + (a.hidden ? 'vuoto ' : 'pieno ') + Math.round(a.getBoundingClientRect().height) + ', spazio ' + s.style.height + ' da y ' + Math.round(s.getBoundingClientRect().top + scrollY); }""")))
        b.foto("BX12_sorgente.png")
        verifica(not fuori, "la pagina della Sorgente scorre (larghezza, altezza, eccesso in px): %s" % fuori)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("X13", "UI46 [U] (stabilità, secondo giro, tastiera): il fuoco non finisce sul body dopo la spunta nel cassetto «Nodi segnalati», «Annulla» nella barra, l'Invio su una linguetta del pannello del nodo, «Togli le spunte» della Sorgente e un ridisegno con il fuoco su un <summary>")
def bX13(b):
    p = b.page
    nome = "ACM-NOTA_X13.pdf"
    p.set_viewport_size({"width": 1280, "height": 720})
    try:
        x_bozza(b)
        # il cassetto «Nodi segnalati»: la casella spuntata tiene il fuoco
        if not p.locator("#du-seleziona-segnalati").is_visible():
            p.click("#du-segnalati-apri")
        cb = p.locator("#du-segnalati input[type=checkbox]:not([disabled])")
        verifica(cb.count() > 0, "nessuna casella nel cassetto «Nodi segnalati»: la prova non prova niente")
        cb.first.click()
        expect(p.locator("#du-selezione")).to_contain_text("1 nodi selezionati")
        p.wait_for_timeout(200)
        verifica(p.evaluate("() => { const a = document.activeElement; return !!a && a.type === 'checkbox' && !!a.closest('#du-segnalati'); }"),
                 "dopo la spunta nel cassetto il fuoco è su %s" % x_fuoco(p))
        p.click("#du-svuota-selezione")
        expect(p.locator("#du-selezione")).to_have_count(0)
        # «Annulla» nella barra (con due gesti in pila resta acceso): il fuoco resta sul pulsante
        b.scheda("k-41")
        for q in ("6", "7"):
            p.click("#du-quantita")
            expect(b.finestra()).to_be_visible()
            p.fill("#f-qta", q)
            b.ok_finestra()
            expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        p.click("#du-annulla")
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        p.wait_for_timeout(2500)
        verifica(x_fuoco(p) == "du-annulla", "dopo «Annulla» nella barra il fuoco è su %s" % x_fuoco(p))
        # le linguette del pannello del nodo («Dettaglio», «Documenti»): con il fuoco da tastiera sulla linguetta, l'Invio
        # cambia la vista e il fuoco resta sulla linguetta (seconda rimisura)
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-41", timeout=T_BREVE)
        for k in ("documenti", "dettaglio"):
            p.locator("#du-ling-" + k).evaluate("e => e.focus({ preventScroll: true })")
            verifica(x_fuoco(p) == "du-ling-" + k, "la linguetta du-ling-%s non prende il fuoco: %s" % (k, x_fuoco(p)))
            p.keyboard.press("Enter")
            expect(p.locator("#du-ling-" + k)).to_have_attribute("aria-selected", "true", timeout=T_BREVE)
            p.wait_for_timeout(400)
            verifica(x_fuoco(p) == "du-ling-" + k, "dopo l'Invio sulla linguetta du-ling-%s il fuoco è su %s" % (k, x_fuoco(p)))
        # «Togli le spunte» della Sorgente: il pulsante sparisce, il fuoco va sulla riga attiva
        b.passo("sorgente")
        b.cartella("tutti")
        r1 = x_riga_visibile(p)
        x_clic(p, "#%s .du-fr-nome" % r1)
        r2 = x_riga_visibile(p, escludi=(r1,))
        x_clic(p, "#%s .du-sg-spunta" % r2)
        r3 = x_riga_visibile(p, escludi=(r1, r2))
        x_clic(p, "#%s .du-sg-spunta" % r3)
        expect(p.locator("#du-sg-togli-spunte-d")).to_be_visible()
        p.click("#du-sg-togli-spunte-d")
        expect(p.locator("#du-sg-elenco .du-sg-spunta:checked")).to_have_count(0)
        verifica(x_fuoco(p) == r1, "dopo «Togli le spunte» il fuoco è su %s (attesa la riga attiva %s)" % (x_fuoco(p), r1))
        # un <summary> di un <details> con un id (nel dettaglio di un file), con il fuoco, quando la fine di un'analisi
        # ridisegna la Sorgente
        x_in_analisi(b, nome)
        b.cartella("tutti")
        trova = """() => { for (const d of document.querySelectorAll('#du-corpo details[id]')) { const s = d.querySelector(':scope > summary');
            if (s && !s.id && s.getClientRects().length) { s.focus({ preventScroll: true }); return d.id; } } return null; }"""
        som = None
        for rid in p.locator("#du-sg-elenco [role=row][id^=du-file-]").evaluate_all("rs => rs.map(r => r.id)")[:12]:
            p.locator("#%s .du-fr-nome" % rid).click()
            p.wait_for_timeout(150)
            som = p.evaluate(trova)
            if som:
                break
        verifica(som, "nessun <summary> senza id in un <details> con un id nel dettaglio dei file: la prova non prova niente")
        x_sonda(p)
        x_conclusa(b, nome)
        b.attendi(lambda: x_ridisegni(p) > 0, "la fine dell'analisi non ha ridisegnato la Sorgente", ms=15000)
        p.wait_for_timeout(600)
        dove = p.evaluate("() => { const a = document.activeElement; return a && a.tagName === 'SUMMARY' && a.parentElement ? a.parentElement.id : (a && a !== document.body ? a.id || a.tagName : 'BODY'); }")
        verifica(dove == som, "dopo il ridisegno il fuoco non è sul <summary> di #%s: %s" % (som, dove))
    finally:
        x_conclusa(b, nome)
        p.set_viewport_size({"width": 1440, "height": 900})


# la sonda di DU-BX14: ogni ~50 ms (e a ogni cambio della barra o del corpo, e al frame dopo) lo scorrimento della pagina,
# il riquadro della tela #du-vp e l'altezza della barra in fondo; conta le risposte di R4 (/bom/riepilogo) arrivate
SONDA_X14 = r"""() => {
  if (window.__x14) { clearInterval(window.__x14.id); window.__x14.oss.disconnect(); }
  const x = window.__x14 = { c: [], r4: 0, id: 0, oss: null };
  const misura = (cosa) => { const v = document.getElementById('du-vp'), s = document.getElementById('du-salva');
    const b = v ? v.getBoundingClientRect() : null;
    x.c.push([cosa, Math.round(performance.now()), scrollY, b ? b.top : null, b ? b.left : null, s ? s.getBoundingClientRect().height : null]); };
  x.id = setInterval(() => misura('t'), 50);
  x.oss = new MutationObserver(() => { misura('m'); requestAnimationFrame(() => misura('f')); });
  for (const sel of ['#du-barra', '#du-corpo']) { const e = document.querySelector(sel); if (e) x.oss.observe(e, { childList: true, subtree: true }); }
  if (!window.__x14f) {
    window.__x14f = 1;
    const f0 = window.fetch;
    window.fetch = function (u) {
      const s = typeof u === 'string' ? u : (u && u.url) || '';
      const r = f0.apply(this, arguments);
      if (/\/bom\/riepilogo/.test(s)) r.then(() => setTimeout(() => { if (window.__x14) window.__x14.r4++; }, 0), () => {});
      return r;
    };
  }
  misura('0');
  return true;
}"""


def x14_r4(p):
    return p.evaluate("() => window.__x14 ? window.__x14.r4 : -1")


def x14_dopo_r4(b, n0, cosa):
    """Il salvataggio finito e R4 arrivato (una risposta in più di n0), con i conti nella barra; poi ancora 700 ms, per il
    ridisegno del pannello che R4 può chiedere."""
    p = b.page
    b.attendi(lambda: x14_r4(p) > n0 and p.locator("#du-salvataggio").get_attribute("data-du-salvataggio") == "salvata"
              and "blocchi" in (p.locator("#du-conti").inner_text() or ""),
              "dopo %s R4 non è arrivato (risposte %s, barra %r)" % (cosa, x14_r4(p), p.locator("#du-salva").inner_text()))
    p.wait_for_timeout(700)


def x14_in_fondo(p):
    """La pagina scorsa fino in fondo, senza lo scorrimento automatico di Playwright, e la sonda accesa da lì."""
    p.evaluate("() => window.scrollTo(0, document.documentElement.scrollHeight)")
    p.wait_for_timeout(400)
    y0, maxy = p.evaluate("() => [scrollY, document.documentElement.scrollHeight - innerHeight]")
    verifica(maxy > 40 and abs(y0 - maxy) <= 1, "la pagina non scorre fino in fondo (scrollY %s, massimo %s): la prova non prova niente" % (y0, maxy))
    p.evaluate(SONDA_X14)


def x14_ferma(b, cosa, foto, minimo):
    """Nessun campione della sonda con lo scorrimento della pagina o la tela spostati di più di 1 px dal primo."""
    p = b.page
    c = p.evaluate("() => { clearInterval(window.__x14.id); window.__x14.oss.disconnect(); return window.__x14.c; }")
    verifica(len([x for x in c if x[0] == "t"]) >= minimo, "la sonda ha preso troppo pochi campioni: %d" % len(c))
    rif = c[0]
    fuori = [x for x in c if abs(x[2] - rif[2]) > 1 or (x[3] is not None and rif[3] is not None and abs(x[3] - rif[3]) > 1)]
    alte = sorted({round(x[5]) for x in c if x[5] is not None})
    print("         DU-BX14, %s: %d campioni, altezze della barra viste %s" % (cosa, len(c), alte))
    b.foto(foto)
    verifica(not fuori, "con la pagina scorsa in fondo, %s l'hanno spostata durante il salvataggio (cosa, ms, scrollY, #du-vp top e "
             "left, altezza della barra; riferimento %s, altezze della barra viste %s): %s" % (cosa, rif, alte, fuori[:8]))


@prova("X14", "UI46 [U] (stabilità, seconda rimisura): con la pagina della Revisione scorsa fino in fondo a 1280×720, «Correggi…» e poi «Annulla» dalla barra non spostano la pagina né la tela di più di 1 px in nessun momento del salvataggio, fino all'arrivo di R4 (la barra in fondo non si accorcia con «conti in calcolo…»); lo stesso per «Ripeti» dopo aver ridimensionato la finestra a 1320 px")
def bX14(b):
    p = b.page
    p.set_viewport_size({"width": 1280, "height": 720})
    try:
        x_bozza(b)
        b.scheda("k-41")
        expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-41", timeout=T_BREVE)
        p.evaluate(SONDA_X14)
        # prima di misurare, la barra con i conti di R4 (alta come quando li ha): un gesto salvato, se non ci sono ancora
        if "blocchi" not in (p.locator("#du-conti").inner_text() or ""):
            n = x14_r4(p)
            p.click("#du-quantita")
            expect(b.finestra()).to_be_visible()
            p.fill("#f-qta", "5")
            b.ok_finestra()
            x14_dopo_r4(b, n, "il primo gesto")
        cod0 = x_codice(p, "k-41")
        verifica(cod0 and not cod0.startswith("ACM-1444"), "codice della scheda k-41 inatteso: %r" % cod0)
        x14_in_fondo(p)
        n = x14_r4(p)
        p.locator("#du-correggi").evaluate("e => e.click()")
        expect(b.finestra()).to_be_visible()
        p.fill("#f-codice", "ACM-1444")
        b.ok_finestra()
        x14_dopo_r4(b, n, "«Correggi…»")
        verifica(x_codice(p, "k-41").startswith("ACM-1444"), "dopo «Correggi…» la scheda non dice ACM-1444: %r" % x_codice(p, "k-41"))
        n = x14_r4(p)
        x_clic(p, "#du-annulla")
        x14_dopo_r4(b, n, "«Annulla»")
        verifica(x_codice(p, "k-41") == cod0, "dopo «Annulla» la scheda non torna a %s: %r" % (cod0, x_codice(p, "k-41")))
        x14_ferma(b, "«Correggi…» e «Annulla»", "BX14_in_fondo.png", 40)
        # la finestra ridimensionata, senza un render della barra: il primo «Ripeti» dopo non la accorcia lo stesso
        p.set_viewport_size({"width": 1320, "height": 720})
        p.wait_for_timeout(600)
        x14_in_fondo(p)
        n = x14_r4(p)
        x_clic(p, "#du-ripeti")
        x14_dopo_r4(b, n, "«Ripeti» dopo il ridimensionamento")
        verifica(x_codice(p, "k-41").startswith("ACM-1444"), "dopo «Ripeti» la scheda non dice ACM-1444: %r" % x_codice(p, "k-41"))
        x14_ferma(b, "«Ripeti» a 1320 px, dopo il ridimensionamento", "BX14_ridimensionata.png", 20)
    finally:
        p.set_viewport_size({"width": 1440, "height": 900})


@prova("03","S8 «Conferma questo STEP come sorgente di ‹P›»: 409 al passo 0, «parziale» ritentabile con la stessa chiave, i tre passi; niente «Autorizza» (direttiva 39)")
def b03(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    s8_prima = b.stato()["conti"].get("S8", 0)
    # la proposta cambia dopo l'anteprima: 409 ANTEPRIMA_SUPERATA al passo 0, niente scritto, S6 riletto
    b.proposta_cambia()
    i = b.segna()
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_contain_text("Conferma ACM-1000_assieme.stp come sorgente di ACM-1000")
    expect(b.finestra()).to_contain_text("Non è la conferma della BOM né del fascicolo")
    expect(b.finestra()).to_contain_text("cambia solo con «Conferma BOM»")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("La proposta è cambiata mentre guardavi", timeout=T_LUNGO)
    expect(b.avviso()).to_have_attribute("data-esito", "conflitto")
    expect(b.finestra()).to_be_hidden()
    b.attendi(lambda: b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$"), "S6 non riletto dopo ANTEPRIMA_SUPERATA")
    s8 = b.corpi(i, "POST", r"/sorgente/step/[^/]+/conferma-sorgente$")
    verifica(len(s8) == 1, "S8: %d richieste" % len(s8))
    c0 = s8[0]
    verifica(re.fullmatch(UUID, c0.get("chiave_idempotenza", "")) and c0.get("prodotto_id") == b.p1 and c0.get("codice_atteso") == "ACM-1000" and c0.get("gruppo", "").startswith("cand:")
             and c0.get("impronta_contenuto") and "componente_atteso" not in c0, "corpo di S8: %s" % c0)
    verifica(not [k for k in c0 if re.search("percorso|path|staging|autorizz", k, re.I)], "S8 manda un campo percorso o di autorizzazione: %s" % c0)
    verifica(b.stato()["conti"].get("S8", 0) == s8_prima, "S8 ha scritto dopo il 409 del passo 0")
    # il motore occupato al passo 3: «parziale» ritentabile, la striscia con «Riprova» che rimanda la stessa chiave
    b.modo("S8", "occupato")
    expect(p.locator("#du-conferma-sorgente")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_have_attribute("data-esito", "parziale", timeout=T_LUNGO)
    expect(esito).to_contain_text("Non riuscito: bozza")
    # collaudo UI46: l'associazione che non serve non è «riuscita»
    expect(esito).to_contain_text("Riuscito: conferma della sorgente")
    verifica("Riuscito: associazione" not in esito.inner_text(), "l'esito di S8 dice riuscita l'associazione che non serve: %r" % esito.inner_text())
    expect(p.locator("#du-sg-c-p_" + sid(b.p1))).to_have_attribute("aria-selected", "true")
    riprova = p.locator("#du-riprova-sorgente_" + sid(b.p1))
    expect(riprova).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-sospeso-sorgente_" + sid(b.p1))).to_contain_text("la bozza non è ancora nata")
    chiave1 = b.corpi(i, "POST", r"/conferma-sorgente$")[0]["chiave_idempotenza"]
    verifica(chiave1 != c0["chiave_idempotenza"], "dopo un 409 S8 riusa la chiave")
    i = b.segna()
    riprova.click()
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    # messaggio 46 (D23): il riquadro dice i passi, il testo intero dell'esito sta in «Ultima mossa» (una volta sola)
    expect(esito).to_contain_text("✓ Sorgente confermata.")
    expect(p.locator("#du-sg-mossa")).to_contain_text("Sorgente confermata: la bozza di ACM-1000 è pronta da rivedere. La BOM non è confermata.")
    rip = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(rip) == 1 and rip[0]["chiave_idempotenza"] == chiave1, "«Riprova» non rimanda la stessa chiave: %s" % rip)
    p.locator("#du-sg-esito-s8 summary").click()
    expect(esito.locator("[data-passo=associazione]")).to_contain_text("non_serve")
    expect(esito.locator("[data-passo=bozza]")).to_contain_text("fatto")
    expect(p.locator("#du-sospeso-sorgente_" + sid(b.p1))).to_have_count(0)
    # la pagina resta nella Sorgente, sulla cartella del prodotto (T-15), con lo smistamento letto da S9
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    expect(p.locator("#du-passo-sorgente")).to_contain_text("sorgente confermata")
    expect(p.locator("#du-sg-c-p_" + sid(b.p1))).to_contain_text("✓ sorgente · bozza v1 · ◇2 ◈0 ●2")
    expect(p.locator("#du-sg-mossa")).to_contain_text("Sorgente confermata")
    st = b.stato()
    verifica(st["conti"].get("S8", 0) == s8_prima + 2 and st["fonte"]["confermata"] is True and st["fonte"]["autorizzata"] is False, "S8 nel banco: %s" % st["conti"])
    verifica(b.richieste_da(i, "GET", r"/motore-a/dati$") and any("sorgente=1" in r["query"] for r in b.richieste_da(i, "GET", r"/motore-a/dati$")), "S9 non riletto dopo S8")
    # direttiva 39 §2 (D-TAB-02 B rivista [T-39]): niente «Autorizza» né «Revoca l'autorizzazione» nella pagina nuova; la
    # working cambia solo con la conferma della BOM; c'è «Revoca come sorgente…» per uno STEP scelto per errore
    if p.locator("#du-revoca-sorgente").count() == 0:
        p.click("#du-sg-prodotto-apri")
    expect(p.locator("#du-revoca-sorgente")).to_be_visible()
    verifica(p.locator("#du-autorizza, #du-revoca").count() == 0, "«Autorizza» o «Revoca l'autorizzazione» ancora nella pagina")
    expect(p.locator("#du-sg-fonte")).to_contain_text("diventa lo STEP della BOM con «Conferma BOM»")
    expect(p.locator("#du-sg-c-p_" + sid(b.p1))).to_contain_text("✓ sorgente")
    verifica(not b.richieste_da(0, "POST", r"/step-strutturale"), "la pagina ha chiamato l'autorizzazione legacy")
    verifica(not b.richieste_da(0, "POST", r"/fascicolo/conferma$"), "«Conferma la fonte» legacy chiamato: la fa S8")
    # i conti del banco dopo S8: la DU-B04 guarda che l'apertura della bozza non scriva (la DU-B62 prima ha scritto la sua)
    b.conti_s8 = dict(b.stato()["conti"])


@prova("04", "dopo S8: la bozza v1 costruita dal server; «Vai alla Revisione →»; nessun R4 all'apertura della bozza")
def b04(b):
    p = b.page
    # un R4 in sottofondo delle prove di prima (la DU-B62 salva la bozza poco prima) può arrivare in ritardo: si aspetta che
    # finisca e si contano gli R4 da qui
    p.wait_for_timeout(2500)
    r4_prima = b.stato()["conti"].get("R4", 0)
    expect(p.locator("#du-vai-revisione")).to_be_enabled()
    p.click("#du-vai-revisione")
    expect(p.locator("#du-passo-revisione")).to_have_attribute("aria-current", "step", timeout=T_LUNGO)
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(10)
    base = getattr(b, "conti_s8", {})
    verifica(b.stato()["conti"].get("R2", 0) == base.get("R2", 0), "la pagina ha scritto con R2: la bozza la crea S8")
    verifica(b.stato()["conti"].get("R4", 0) == r4_prima, "R4 all'apertura della bozza")
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
    # collaudo UI46, I7 (6c8d9fb): il motivo non dice più la versione del progetto («V3»), ma dove si fa il gesto
    verifica("Rivedi e conferma fascicolo" in (riga.locator("button", has_text="Conferma il documento").get_attribute("title") or ""), "«Conferma il documento» senza il motivo")
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica("Organigramma" in (p.locator("#du-cta-bom").get_attribute("title") or ""), "il CTA nell'Elenco senza il motivo")
    expect(p.locator("#du-cta-fascicolo")).to_be_disabled()
    i = b.segna()
    p.click("#du-accetta-" + sid(b.al["ACM-1100_A.pdf"]))
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "modifiche")
    expect(p.locator("#du-salvataggio")).to_contain_text("1 modifiche non salvate")
    b.salvata(2)
    r2 = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(r2) == 1, "R2: %d richieste (il debounce le raccoglie)" % len(r2))
    c = r2[0]
    verifica(c["vocabolario"] == 2 and c["versione_attesa"] == 1 and re.fullmatch(UUID, c["chiave_salvataggio"]), "testa di R2: %s" % {k: c[k] for k in c if k not in ("snapshot", "azioni")})
    verifica([a["azione"] for a in c["azioni"]] == ["file_preassociato"], "azioni: %s" % c["azioni"])
    # G39-01 [U]: la proposta del registro accettata porta nello snapshot il registro della bozza (le azioni no); la
    # provenienza «motore» la decide il server
    reg = b.stato()["bozze"][b.p1]["snapshot"]["sorgente"].get("importazione")
    verifica(reg and c["snapshot"]["documenti_preassociati"] == [{"allegato_id": b.al["ACM-1100_A.pdf"], "chiave_bozza": "k-7", "importazione": reg}], "snapshot: %s" % c["snapshot"]["documenti_preassociati"])
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
    expect(p.locator('#du-segnalati .du-segnalato[data-k="k-33"]')).to_contain_text("il grafo del file non è completo: che il nodo non abbia figli potrebbe non essere un'informazione (grafo_non_completo)")
    verifica("contro-indizi" not in p.locator('#du-segnalati .du-segnalato[data-k="k-31"]').inner_text(), "contro-indizi su un segnale che non ne ha")
    p.click("#du-seleziona-segnalati")
    expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
    b.scheda("k-12", modifiers=["Control"])
    expect(p.locator("#du-selezione")).to_contain_text("4 nodi selezionati")
    expect(p.locator("#nc-k-12")).to_have_class(re.compile(r"du-nc--selezionato"))
    p.locator("#sel-k-12").click()
    expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
    # una sola scrittura dopo S8 (la preassociazione della DU-B05); la DU-B62 prima ha scritto la sua bozza
    verifica(b.scritture().get("R2", 0) - getattr(b, "conti_s8", {}).get("R2", 0) == 1, "la selezione ha scritto qualcosa: %s" % b.scritture())
    i = b.segna()
    p.click("#du-non-includere")
    expect(p.locator("#f-elenco input:checked")).to_have_count(3)
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
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
        # messaggio 46 [U]: l'utente non sceglie né scrive il motivo; la pagina scrive motivo_codice dal segnale visto
        # (il possibile elemento CAD: «elemento_cad») e la traccia del gesto [T]
        verifica(d["tipo"] == "non_includere" and d["motivo_codice"] == "elemento_cad" and d["motivo"].startswith("gesto ") and "da «Non includere nella BOM»: «" in d["motivo"]
                 and d["casella"]["allegato_id"] == a1
                 and d["casella"]["gruppo"].startswith("cand:") and d["casella"]["rif"].startswith("nodo:"), "decisione: %s" % d)
        verifica(d["segnale_visto"] == {"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"},
                 "segnale_visto: %s" % d.get("segnale_visto"))
    b.foto("B06_revisione_organigramma.png")


@prova("07", "Annulla e Ripeti: annulla{n} e rifai{n}, prima = {azione_id} per il gesto cumulativo (PE-05); mai oltre la versione del server")
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
    # la ripetizione del gesto cumulativo porta lo stesso azione_id nel «prima», come l'annullamento (EM §7.1; PE-05)
    verifica(all(a["prima"] == prime[0] for a in az), "«prima» della ripetizione: %s" % [a["prima"] for a in az])
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
    i = b.segna()
    p.click("#du-ripeti")
    b.salvata(8)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["prima"], a["dopo"]) for a in az] == [("rifai", None, {"n": 3})], "rifai della preassociazione: %s" % az)
    i = b.segna()
    p.click("#du-ripeti")
    b.salvata(9)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([a["azione"] for a in az] == ["rifai"] * 9 and [a["dopo"]["n"] for a in az] == list(range(4, 13)) and all(a["prima"] == prime[0] for a in az),
             "rifai del gesto cumulativo dopo un altro passo: %s" % az)
    expect(noninc).to_have_count(3)
    expect(p.locator("#du-ripeti")).to_be_disabled()


@prova("08", "pannello del nodo e «Correggi» su un pezzo importato: decisione motivata")
def b08(b):
    p = b.page
    b.scheda("k-12")
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", "k-12")
    expect(p.locator("#du-sez-proposta")).to_contain_text("nessuna lettura")
    expect(p.locator("#du-sez-decisione")).to_contain_text("da decidere")
    i = b.segna()
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1130")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
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
    verifica(dec["tipo"] == "identita" and dec["modo"] == "correzione" and dec["identita"]["codice"] == "ACM-1130" and "motivo" not in dec, "decisione: %s" % dec)
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


@prova("12", "wizard: R4 con gli avvisi del motore (firmati), fasi, non inclusi con gli effetti; avvisi non calcolati → conferma spenta, «Riprova»")
def b12(b):
    p = b.page
    i = b.segna()
    b.apri_wizard()
    r4 = b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$")
    verifica(r4 and "avvisi_motore=1" in r4[-1]["query"], "R4 del wizard senza gli avvisi del motore: %s" % [r["query"] for r in r4])
    p.click("#du-wz-g-noninc")
    expect(p.locator("#du-wz-noninclusi tbody tr")).to_have_count(3)
    expect(p.locator("#du-wz-noninclusi")).to_contain_text("Saldatura")
    expect(p.locator("#du-wz-noninclusi")).to_contain_text("1 riga dello STEP si chiude alla conferma")
    expect(p.locator("#du-wz-noninclusi")).to_contain_text("nessuna cascata sui figli")
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_contain_text("La BOM è confermabile.")
    # collaudo UI46, I4 (giro 2, C2-I1): gli avvisi si dicono con il testo del server, il codice resta in data-codice
    expect(p.locator("#du-wz-avvisi [data-codice=VERIFICA_DOPO_COMMIT]")).to_be_visible()
    expect(p.locator("#du-wz-avvisi")).not_to_contain_text("VERIFICA_DOPO_COMMIT")
    expect(p.locator("#du-wz-motore")).to_contain_text("calcolati con il riepilogo e firmati")
    expect(p.locator("#du-wz-conflitti-motore")).to_have_attribute("data-n", "0")
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
    verifica("avvisi del motore non sono calcolati" in (p.locator("#du-wz-conferma").get_attribute("title") or ""), "conferma spenta senza il motivo")
    # il blocco «R4 completo» della persistenza: il riepilogo senza il motore non si conferma, niente presa d'atto
    verifica(p.locator("#du-wz-presa").count() == 0, "la presa d'atto degli avvisi del motore è ancora nel wizard")
    expect(p.locator("#du-wz-confermabile")).to_contain_text("finché gli avvisi del motore non sono calcolati")
    p.click("#du-wz-riprova-motore")
    expect(p.locator("#du-wz-motore")).to_contain_text("calcolati con il riepilogo e firmati", timeout=T_LUNGO)
    expect(p.locator("#du-wz-conferma")).to_be_enabled()


@prova("63", "R4/R5 formato 3 (direttiva 39 §4, §5, §8): D-ASS-01 su «Correggi codice» (annulla, mantieni, scollega); i gruppi D-PB-04 con la legacy da verificare e il suo rimedio; gia_associata_altrove con «Scollega da ‹C› e sostituisci» e lo spostamento di AR; condivisione[]; la riga «legato come sorgente»; R5 cumulativa: 503 con «Riprova» e la stessa chiave, R5_NON_APPLICABILE, il gate ricontrollato da R5, ingressi_in_movimento")
def b63(b):
    p = b.page
    if p.locator("#du-wizard").is_visible():
        p.click("#du-wz-chiudi")
    a2, a11 = b.al["ACM-1100_A.pdf"], b.al["ACM-1400_A.pdf"]
    st = b.stato()
    nodi = {n["chiave_bozza"] for n in st["bozze"][b.p1]["snapshot"]["nodi"]}
    verifica("k-7" in nodi and "k-41" in nodi, "la bozza non ha k-7 e k-41: %s" % sorted(nodi))
    n_conf = len(st["conferme"].get(b.p1, []))

    def provenienza(aid, prov, chiave=None, imp=None):
        corpo = {"allegato_id": aid, "provenienza": prov}
        if chiave:
            corpo["chiave_bozza"] = chiave
        if imp:
            corpo["importazione"] = imp
        r = p.request.post(b.url + "/__banco/provenienza", data=json.dumps(corpo), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda della provenienza: %d" % r.status)

    def rileggi():
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)

    def wizard_fase2():
        if p.locator("#du-wizard").is_visible():
            p.click("#du-wz-chiudi")
        b.apri_wizard()
        p.click("#du-wz-fase2")
        expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)

    # le PB scritte dal server (sonda): la PB iniziale del motore di ACM-1100_A.pdf su k-7 (D-PB-01 A, con il registro) e
    # la conversione della pre-assegnazione P1 di ACM-1400_A.pdf, registrato su ACM-1400, sul pezzo k-41 (D-AR14 A)
    provenienza(a2, "motore", "k-7", "registro")
    provenienza(a11, "legacy", "k-41")
    rileggi()
    b.passo("revisione")
    b.vista("org")
    # 1. D-ASS-01 [U]: la rev nuova di k-7 non corrisponde più al file collegato: «mantieni / scollega / annulla»
    def correggi_rev(rev):
        b.scheda("k-7")
        b.linguetta("dettaglio")
        p.click("#du-correggi")
        p.fill("#f-rev", rev)
        b.ok_finestra()
        expect(b.finestra()).to_contain_text("I file collegati non corrispondono più", timeout=T_LUNGO)
        expect(p.locator("#f-discordanti [data-du-discordante='%s']" % a2)).to_have_attribute("data-tipo", "pb")
    i = b.segna()
    correggi_rev("B")
    b.finestra().get_by_role("button", name="Annulla").click()
    expect(b.finestra()).to_be_hidden()
    p.wait_for_timeout(1800)
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "«Annulla» di D-ASS-01 ha salvato")
    i = b.segna()
    correggi_rev("B")
    p.click("#f-mantieni-discordanti")
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and az[1]["dopo"]["identita"]["rev"] == "B", "«Mantieni i collegamenti»: %s" % az)
    verifica(any(d["allegato_id"] == a2 and d["chiave_bozza"] == "k-7" for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"]), "«Mantieni» ha tolto la PB")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    i = b.segna()
    correggi_rev("B")
    p.click("#f-scollega-discordanti")
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato", "file_svincolato"] and az[2]["prima"]["allegato_id"] == a2, "«Scollega i file»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.attendi(lambda: any(d["allegato_id"] == a2 for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"]), "dopo «Annulla» la PB non è tornata")
    # 2. il riepilogo: la sorgente legata in sola lettura, i gruppi, la legacy da verificare con il suo rimedio, nessuna spunta
    wizard_fase2()
    expect(p.locator("#du-wz-sorgente-legata")).to_have_attribute("data-confermata", "1")
    expect(p.locator("#du-wz-sorgente-legata")).to_contain_text("legato come sorgente")
    expect(p.locator("#du-wz-gruppo-motore_intatta [data-du-pb='%s']" % a2)).to_be_visible()
    leg = p.locator("#du-wz-gruppo-legacy_da_verificare [data-du-pb='%s']" % a11)
    expect(leg).to_have_attribute("data-motivo", "legacy_da_verificare")
    expect(leg).to_contain_text("in conflitto")
    expect(p.locator("#du-wz-rimedio-togli-" + sid(a11))).to_be_visible()
    # R39-15: «Riconosci…» = un salvataggio con file_riconosciuto, lo snapshot invariato; la PB resta legacy e diventa
    # verificata (gruppo «Legacy riconosciute»); resta in conflitto solo perché il file è registrato su ACM-1400
    i = b.segna()
    p.click("#du-wz-rimedio-riconosci-" + sid(a11))
    expect(b.finestra()).to_contain_text("Riconosci la preassociazione legacy", timeout=T_LUNGO)
    expect(b.finestra()).to_contain_text("non si annulla")
    b.ok_finestra()
    b.attendi(lambda: b.corpi(i, "POST", r"/motore-a/bozza$"), "«Riconosci…» non ha salvato")
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    az = c["azioni"]
    verifica(len(az) == 1 and az[0]["azione"] == "file_riconosciuto" and az[0]["prima"] == {"allegato_id": a11, "chiave_bozza": "k-41"}
             and az[0]["dopo"] == {"allegato_id": a11, "chiave_bozza": "k-41"}, "«Riconosci…»: %s" % az)
    verifica(not any(d.get("verificata") for d in c["snapshot"]["documenti_preassociati"]), "la pagina ha dichiarato la verifica: %s" % c["snapshot"]["documenti_preassociati"])
    ver = p.locator("#du-wz-gruppo-legacy_verificate [data-du-pb='%s']" % a11)
    expect(ver).to_have_attribute("data-motivo", "gia_associata_altrove", timeout=T_LUNGO)
    expect(p.locator("#du-wz-gruppo-legacy_da_verificare [data-du-pb='%s']" % a11)).to_have_count(0)
    pb = [d for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"] if d["allegato_id"] == a11]
    verifica(pb and pb[0].get("provenienza") == "legacy" and pb[0].get("verificata") is True, "la PB riconosciuta nel banco: %s" % pb)
    expect(p.locator("#du-wz-cumulativa")).to_contain_text("non ci sono spunte per singolo file")
    verifica(p.locator("#du-wz-documenti input[type=checkbox]").count() == 0, "spunte per singola PB nel riepilogo")
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    # 3. la PB rifatta a mano (manuale, come la decide il server): il file è registrato su ACM-1400, nello stesso prodotto
    provenienza(a11, "manuale")
    p.click("#du-wz-chiudi")
    rileggi()
    wizard_fase2()
    man = p.locator("#du-wz-gruppo-manuale [data-du-pb='%s']" % a11)
    expect(man).to_have_attribute("data-motivo", "gia_associata_altrove")
    expect(man).to_contain_text("nello stesso prodotto")
    i = b.segna()
    p.click("#du-wz-rimedio-sostituisci-" + sid(a11))
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "decisione_aggiunta" and az[0]["dopo"]["tipo"] == "scollega_documento" and az[0]["dopo"]["allegato_id"] == a11
             and az[0]["dopo"]["componente_id"] == C40, "«Scollega da ACM-1400 e sostituisci»: %s" % az)
    sp = p.locator("#du-wz-gruppo-spostamenti_ar [data-du-spostamento='%s']" % a11)
    expect(sp).to_have_attribute("data-tipo", "sostituzione", timeout=T_LUNGO)
    expect(sp).to_contain_text("da ACM-1400")
    expect(man).to_have_attribute("data-esito", "sostituzione")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    # 4. condivisione[]: un altro prodotto toccato, con la conseguenza attribuita a chi conferma (D-TAB-03 A)
    b.interruttore("condivisione", True)
    try:
        wizard_fase2()
        expect(p.locator("#du-wz-condivisione")).to_contain_text("ACM-2000")
        expect(p.locator("#du-wz-condivisione")).to_contain_text("attribuite a chi la conferma")
    finally:
        b.interruttore("condivisione", False)
    wizard_fase2()
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    # 5. R5 cumulativa: 503 non_registrato con «Riprova» e la stessa chiave; poi R5_NON_APPLICABILE, niente scritto
    e = b.esito_wizard()
    b.modo("R5", "503")
    i = b.segna()
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-esito", "occupato", timeout=T_LUNGO)
    expect(e).to_contain_text("Riprova fra poco con la stessa chiave")
    sosp = b.sospesi()
    verifica(len(sosp) == 1 and sosp[0][1].get("certo") is True and sosp[0][1].get("codice") == "CALCOLO_OCCUPATO", "503 di R5: memoria della scheda %s" % sosp)
    b.modo("R5", "422_non_applicabile")
    p.click("#du-wz-riprova-occupato")
    expect(e).to_have_attribute("data-codice", "BLOCCHI", timeout=T_LUNGO)
    expect(p.locator("#du-wz-esito-blocchi [data-codice=R5_NON_APPLICABILE]")).to_contain_text("una parte che la conferma non sa ancora applicare")
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 2 and r5[0]["bozza"]["chiave_idempotenza"] == r5[1]["bozza"]["chiave_idempotenza"] and r5[0] == r5[1], "«Riprova» dopo il 503 non riusa la chiave e il corpo")
    verifica(not b.sospesi(), "BLOCCHI ha lasciato la chiave in sospeso")
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    p.click("#du-wz-rileggi-blocchi")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    # 6. il gate ricontrollato da R5 sotto il lucchetto (una revoca della sorgente dopo R4): risposta finta del server
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "BLOCCHI", "testo": "La BOM non si conferma: 1 blocchi.",
                            "blocchi": [{"codice": "STEP_SORGENTE_NON_CONFERMATA", "motivo": "revocata", "testo": "La sorgente della bozza e' stata revocata: conferma di nuovo uno STEP come sorgente dalla Sorgente.",
                                         "rimedi": ["«Conferma questo STEP come sorgente» del prodotto (S8)"]}]})
    p.route("**/motore-a/bom/conferma", g)
    try:
        p.click("#du-wz-conferma")
        expect(e).to_have_attribute("data-codice", "BLOCCHI", timeout=T_LUNGO)
        expect(p.locator("#du-wz-esito-blocchi [data-codice=STEP_SORGENTE_NON_CONFERMATA]")).to_have_attribute("data-motivo", "revocata")
        expect(p.locator("#du-wz-esito-vai-sorgente")).to_be_visible()
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    p.click("#du-wz-rileggi-blocchi")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    # 7. 409 ANTEPRIMA_SUPERATA ingressi_in_movimento: niente registrato, la bozza intatta
    b.modo("R5", "409_movimento")
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-codice", "ANTEPRIMA_SUPERATA", timeout=T_LUNGO)
    expect(e).to_contain_text("i dati della proposta stavano cambiando")
    st = b.stato()
    verifica(len(st["conferme"].get(b.p1, [])) == n_conf and st["conti"].get("R5", 0) == 0 and not b.sospesi(), "un rifiuto di R5 ha registrato una conferma")
    # 8. si torna allo stato delle prove che seguono: lo svincolo annullato, la PB di ACM-1400_A.pdf tolta (il rimedio)
    p.click("#du-wz-chiudi")
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    wizard_fase2()
    i = b.segna()
    p.click("#du-wz-rimedio-togli-" + sid(a11))
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_svincolato"], "il rimedio «Togli la preassociazione»: %s" % az)
    expect(p.locator("#du-wz-gruppo-manuale [data-du-pb='%s']" % a11)).to_have_count(0, timeout=T_LUNGO)
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    verifica(not any(d["allegato_id"] == a11 for d in b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"]), "la PB di ACM-1400_A.pdf è rimasta")


@prova("66", "R4 e R5 completi (persistenza): avviso del registro con il valore, blocco con il motivo (RICODIFICA_FILE_NAS documento_confermato), avvisi e conflitti del motore firmati, esclusioni legacy ed ereditata; R5 504 CALCOLO_SCADUTO e 500 ERRORE_CALCOLO non registrati con «Riprova» e la stessa chiave")
def b66(b):
    p = b.page

    def wizard_fase2():
        if p.locator("#du-wizard").is_visible():
            p.click("#du-wz-chiudi")
        b.apri_wizard()
        p.click("#du-wz-fase2")
        expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)

    # 1. le forme del blocco «R4 completo» e «R5 completo» (interruttore del banco r4-completo)
    b.interruttore("r4-completo", True)
    try:
        wizard_fase2()
        bl = p.locator("#du-wz-blocchi [data-codice=RICODIFICA_FILE_NAS]")
        expect(bl).to_have_attribute("data-motivo", "documento_confermato", timeout=T_LUNGO)
        expect(bl).to_contain_text("correggi il codice dalla scheda del componente")
        expect(p.locator("#du-wz-avvisi [data-codice=EQUIVALENTE_NON_ADOTTATO] [data-du-valore='K4-0099']")).to_be_visible()
        am = p.locator("#du-wz-avvisi-motore [data-codice=PROPOSTA_CAMBIATA]")
        expect(am).to_have_attribute("data-motivo", "contenuto")
        expect(am).to_contain_text("nodi diversi 2, legami diversi 1")
        expect(p.locator("#du-wz-motore")).to_contain_text("calcolati con il riepilogo e firmati")
        expect(p.locator("#du-wz-conflitti-motore")).to_have_attribute("data-n", "1")
        expect(p.locator("#du-wz-conflitti-motore")).to_contain_text("nomenclatura")
        expect(p.locator("#du-wz-altre-esclusioni tr[data-origine=legacy]")).to_contain_text("scartata nelle pagine di prima")
        expect(p.locator("#du-wz-altre-esclusioni tr[data-origine=ereditata]")).to_contain_text("solo la traccia")
        expect(p.locator("#du-wz-conferma")).to_be_disabled()
        b.foto("B66_r4_completo.png")
    finally:
        b.interruttore("r4-completo", False)
    wizard_fase2()
    expect(p.locator("#du-wz-blocchi [data-codice=RICODIFICA_FILE_NAS]")).to_have_count(0)
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    # 2. R5 con il motore nella connessione S: 504 CALCOLO_SCADUTO e 500 ERRORE_CALCOLO non registrati, il tentativo resta
    # «certo» con la stessa chiave; «Riprova» rimanda lo stesso corpo; il 500 ERRORE_CALCOLO non è il controllo finale
    e = b.esito_wizard()
    n_conf = len(b.stato()["conferme"].get(b.p1, []))
    i = b.segna()
    b.modo("R5", "504")
    p.click("#du-wz-conferma")
    expect(e).to_have_attribute("data-esito", "occupato", timeout=T_LUNGO)
    expect(e).to_contain_text("non è finito in tempo")
    sosp = b.sospesi()
    verifica(len(sosp) == 1 and sosp[0][1].get("certo") is True and sosp[0][1].get("codice") == "CALCOLO_SCADUTO", "504 di R5: memoria della scheda %s" % sosp)
    b.modo("R5", "500_calcolo")
    p.click("#du-wz-riprova-occupato")
    expect(e).to_have_attribute("data-codice", "ERRORE_CALCOLO", timeout=T_LUNGO)
    expect(e).to_have_attribute("data-esito", "occupato")
    expect(e).to_contain_text("Il calcolo del motore non è riuscito")
    expect(p.locator("#du-wz-rileggi-calcolo")).to_be_visible()
    sosp = b.sospesi()
    verifica(len(sosp) == 1 and sosp[0][1].get("certo") is True and sosp[0][1].get("codice") == "ERRORE_CALCOLO", "500 ERRORE_CALCOLO di R5: memoria della scheda %s" % sosp)
    # si chiude il tentativo come DU-B63: un 422 BLOCCHI (esito certo) lo toglie dalla memoria della scheda
    b.modo("R5", "422")
    p.click("#du-wz-riprova-occupato")
    expect(e).to_have_attribute("data-codice", "BLOCCHI", timeout=T_LUNGO)
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 3 and r5[0] == r5[1] == r5[2], "«Riprova» dopo il 504 e il 500 non riusa la chiave e il corpo: %d invii" % len(r5))
    verifica(not b.sospesi(), "BLOCCHI ha lasciato la chiave in sospeso")
    st = b.stato()
    verifica(len(st["conferme"].get(b.p1, [])) == n_conf and st["conti"].get("R5", 0) == 0, "un rifiuto di R5 ha registrato una conferma")
    p.click("#du-wz-rileggi-blocchi")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)


@prova("67", "CREA_SU_ESISTENTE (rilievo del 20/10): «Riusa ‹C›» nel riepilogo scrive riusa_esistente con R2 e toglie il blocco; «Riusa i componenti con lo stesso codice (n)» in un salvataggio; «Togli il riuso» nel pannello")
def b67(b):
    p = b.page
    if p.locator("#du-wizard").is_visible():
        p.click("#du-wz-chiudi")
    b.passo("revisione")
    b.vista("org")

    def aggiungi(desc):
        b.scheda("k-root")
        b.linguetta("dettaglio")
        p.click("#du-aggiungi-sotto")
        p.select_option("#f-tipo", "sciolto")
        p.fill("#f-desc", desc)
        p.fill("#f-codice", "ACM-1400")
        p.fill("#f-rev", "A")
        p.fill("#f-qta", "1")
        b.ok_finestra()
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        k = p.locator("#du-pannello").get_attribute("data-k")
        verifica(k and k.startswith("k-u"), "il pezzo nuovo non è il nodo scelto: %s" % k)
        # un pezzo scritto a mano non ha una voce con decisione_esistente: nel pannello nessun «Riusa…» (il rimedio sta nel
        # riepilogo, dove il server dice il componente)
        verifica(p.locator("#du-riusa-esistente, #du-riuso-nodo").count() == 0, "riuso nel pannello di un pezzo senza voce")
        return k

    def fase2():
        if p.locator("#du-wizard").is_visible():
            p.click("#du-wz-chiudi")
        b.apri_wizard()
        p.click("#du-wz-fase2")
        expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)

    # 1. un pezzo nuovo con il codice del registrato ACM-1400: CREA_SU_ESISTENTE con «Riusa ACM-1400» (uno solo: niente
    # gesto in blocco), la conferma spenta
    k1 = aggiungi("Supporto rifatto a mano")
    fase2()
    bl = p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")
    expect(bl).to_have_count(1, timeout=T_LUNGO)
    riusa = p.locator("#du-wz-riusa-" + sid(k1))
    expect(riusa).to_have_text("Riusa ACM-1400")
    expect(riusa).to_be_enabled()
    expect(p.locator("#du-wz-riusa-tutti")).to_have_count(0)
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    i = b.segna()
    riusa.click()
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "decisione_aggiunta" and az[0]["chiave_bozza"] == k1 and az[0]["dopo"]["tipo"] == "riusa_esistente"
             and az[0]["dopo"]["chiave_bozza"] == k1 and az[0]["dopo"]["componente_id"] == C40 and set(az[0]["dopo"]) == {"id", "tipo", "chiave_bozza", "componente_id"},
             "«Riusa ACM-1400»: %s" % az)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$"), "il riepilogo non si è riletto dopo il riuso")
    expect(p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")).to_have_count(0, timeout=T_LUNGO)
    # il riepilogo dice il nodo nel componente registrato (componente_id dell'identità, come componenteDi del server)
    p.click("#du-wz-fase1")
    expect(p.locator("#du-wz-identita tr[data-k='%s']" % k1)).to_be_visible(timeout=T_LUNGO)
    p.click("#du-wz-chiudi")
    # 2. il pannello del nodo dice il riuso e «Togli il riuso» lo toglie (un salvataggio): il blocco torna
    b.scheda(k1)
    b.linguetta("dettaglio")
    expect(p.locator("#du-riuso-nodo")).to_have_attribute("data-componente", C40, timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-togli-riuso")
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "decisione_tolta" and az[0]["prima"]["tipo"] == "riusa_esistente" and az[0]["dopo"] is None, "«Togli il riuso»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # 3. due pezzi nuovi con lo stesso codice: «Riusa i componenti con lo stesso codice (2)», un salvataggio con le due
    # decisioni; poi «Annulla» le toglie insieme
    k2 = aggiungi("Supporto doppio")
    fase2()
    expect(p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")).to_have_count(2, timeout=T_LUNGO)
    tutti = p.locator("#du-wz-riusa-tutti")
    expect(tutti).to_have_text("Riusa i componenti con lo stesso codice (2)")
    i = b.segna()
    tutti.click()
    az = b.ultime_azioni(i)
    verifica(sorted((a["azione"], a["dopo"]["tipo"], a["dopo"]["chiave_bozza"], a["dopo"]["componente_id"]) for a in az) ==
             sorted([("decisione_aggiunta", "riusa_esistente", k, C40) for k in (k1, k2)]), "«Riusa i componenti con lo stesso codice»: %s" % az)
    expect(p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")).to_have_count(0, timeout=T_LUNGO)
    b.foto("B67_riuso.png")
    p.click("#du-wz-chiudi")
    # 4. i due pezzi si tolgono («Non crearlo»: le decisioni del nodo escono con lui), e la bozza torna confermabile
    for k in (k2, k1):
        b.scheda(k)
        b.linguetta("dettaglio")
        i = b.segna()
        p.click("#du-togli")
        b.ok_finestra()
        az = b.ultime_azioni(i)
        verifica([a["azione"] for a in az] == ["arco_tolto", "decisione_tolta", "nodo_tolto"] and az[1]["prima"]["tipo"] == "riusa_esistente",
                 "«Non crearlo» di un pezzo riusato: %s" % [a["azione"] for a in az])
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    fase2()
    expect(p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")).to_have_count(0)
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)


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
    # dopo un 422 BLOCCHI di R5 la conferma resta spenta finché il riepilogo non si rilegge
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    p.click("#du-wz-rileggi-blocchi")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
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
    # i segni di U2 §5.5 (WZ-09): la conferma registrata e la verifica corrente di /dati, «in prima istanza», ricalcolata dal
    # server (qui lo STEP della fonte è diventato lo STEP della working con questa conferma, R5: sì), mai «non ancora
    # calcolata (arriva in V2)»
    expect(p.locator("#du-wz-segni")).to_contain_text("v1 confermata")
    expect(p.locator("#du-verifica-adesso-wz")).to_contain_text("Verificata in prima istanza, adesso: sì · poggia sulla conferma n. 1")
    verifica("arriva in V2" not in e.inner_text(), "il wizard dice ancora «Verificata: non ancora calcolata (arriva in V2)»")
    # i nodi non inclusi della conferma, con il nome (MAT NI-10)
    expect(p.locator("#du-wz-noninclusi-conferma")).to_contain_text("Nodi non inclusi nella conferma n. 1")
    expect(p.locator("#du-wz-noninclusi-conferma")).to_contain_text("Saldatura")
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 2 and r5[0]["bozza"]["chiave_idempotenza"] == k == r5[1]["bozza"]["chiave_idempotenza"], "«Riprova» non riusa la chiave")
    verifica(r5[0] == r5[1], "«Riprova» non manda lo stesso corpo")
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo la conferma")
    verifica(len(b.stato()["conferme"][b.p1]) == 1, "conferme registrate: %s" % b.stato()["conferme"])


@prova("60", "R15 «Rifiuta la proposta aggiornata…» senza bozza (U2 §5.7): un nome diverso da «Scarta» e da «Abbandona», il motivo, ANTEPRIMA_SUPERATA, il segno del rifiuto")
def b60(b):
    p = b.page
    if p.locator("#du-wizard").is_visible():
        p.click("#du-wz-chiudi")
    expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    b.proposta_cambia()
    p.click("#du-aggiorna")
    seg = p.locator("#du-proposta-aggiornata")
    expect(seg).to_have_attribute("data-du-proposta", "aperta", timeout=T_LUNGO)
    expect(seg).to_contain_text("Proposta aggiornata: 1 differenze da valutare")
    expect(p.locator("#du-pa-rifiuta")).to_have_text("Rifiuta la proposta aggiornata…")
    p.click("#du-pa-rifiuta")
    expect(b.finestra()).to_contain_text("Rifiuta la proposta aggiornata di ACM-1000")
    expect(b.finestra()).to_contain_text("non cambia la verifica corrente")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    # la proposta cambia ancora mentre il dialogo è aperto: 409 ANTEPRIMA_SUPERATA, niente scritto, il segno riletto
    b.proposta_cambia()
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("ANTEPRIMA_SUPERATA", timeout=T_LUNGO)
    verifica(b.stato()["conti"].get("R15", 0) == 0, "R15 ha scritto con l'impronta vecchia")
    r15 = b.corpi(i, "POST", r"/proposta-aggiornata/rifiuta$")
    verifica(len(r15) == 1 and r15[0]["prodotto_id"] == b.p1 and r15[0]["conferma_id"] and r15[0]["gruppo"].startswith("cand:") and len(r15[0]["impronta_contenuto"]) == 64
             and "motivo" not in r15[0] and re.fullmatch(UUID, r15[0]["chiave_idempotenza"]), "corpo di R15: %s" % r15)
    expect(seg).to_have_attribute("data-du-proposta", "aperta", timeout=T_LUNGO)
    p.click("#du-pa-rifiuta")
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Proposta aggiornata rifiutata: la BOM confermata resta com'è.", timeout=T_LUNGO)
    expect(seg).to_have_attribute("data-du-proposta", "rifiutata", timeout=T_LUNGO)
    expect(seg).to_contain_text("rifiutata da AD")
    r15b = b.corpi(i, "POST", r"/proposta-aggiornata/rifiuta$")
    verifica(len(r15b) == 1 and r15b[0]["chiave_idempotenza"] != r15[0]["chiave_idempotenza"], "il rifiuto dopo il 409 riusa la chiave: %s" % r15b)
    verifica(b.stato()["conti"].get("R15", 0) == 1, "R15 non registrato nel banco")
    verifica(not b.sospesi(), "R15 ha lasciato una chiave in sospeso")


@prova("15", "WZ-01: senza modifiche sulla BOM confermata il CTA è spento; R5 con la risposta persa: R6 «in corso» poi «registrato»")
def b15(b):
    p = b.page
    if p.locator("#du-wizard").is_visible():
        p.click("#du-wz-chiudi")
    expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    p.click("#du-da-conferma")
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc[data-k]")).to_have_count(2)
    # «v1 confermata il … da …» e la verifica corrente restano due segni distinti (U2 §5.5): la verifica «in prima istanza»
    # la calcola il server a ogni lettura di /dati (conferma_corrente.verifica_adesso), mai la pagina
    expect(p.locator("#du-stato-conferma")).to_contain_text("v1 confermata")
    expect(p.locator("#du-stato-conferma")).to_contain_text(" da AD")
    expect(p.locator("#du-verifica-adesso")).to_contain_text("Verificata in prima istanza, adesso: sì · poggia sulla conferma n. 1")
    expect(p.locator("#du-verifica-adesso")).to_have_attribute("data-du-verifica", "si")
    # WZ-01: nessuna modifica su una versione confermata, il CTA è spento con il motivo (una conferma n+1 uguale alla n no)
    expect(p.locator("#du-cta-bom")).to_be_disabled()
    verifica("nessuna modifica: la v1 è già confermata" in (p.locator("#du-cta-bom").get_attribute("title") or ""), "il CTA senza modifiche non è spento con il motivo")
    # la AR che R5 ha registrato con la conferma n. 1 (ACM-1100_A.pdf sul componente di ACM-1100) si legge dalle righe di S9
    # (gia_registrato), mai dalla copia di S1 (DIR35-DP [U]); il documento confermato e la AR di ACM-1400 restano
    a2 = b.al["ACM-1100_A.pdf"]
    verifica(b.stato()["file"]["ACM-1100_A.pdf"]["stato"] == "attivo", "ACM-1100_A.pdf non è attivo")
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    expect(p.locator("#du-sg-elenco li[data-allegato='%s']" % a2)).to_contain_text("già registrato", timeout=T_LUNGO)
    # si torna ai dati delle prove che seguono (scritte con ACM-1100_A.pdf proposto dal motore e non registrato): la sonda
    # toglie la AR, la pagina rilegge
    r = p.request.post(b.url + "/__banco/ar", data=json.dumps({"nome": "ACM-1100_A.pdf", "componente_id": ""}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda della AR: %d" % r.status)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-40")
    b.linguetta("documenti")
    expect(p.locator("#du-doc-nodo [data-du-doc=associato]")).to_contain_text("ACM-1400_A.pdf")
    expect(p.locator("#du-doc-nodo [data-du-doc=confermato]")).to_contain_text("ACM-1400_ASSIEME.pdf")
    b.linguetta("dettaglio")
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
    verifica(dec["tipo"] == "scollega_documento" and dec["allegato_id"] == b.al["ACM-1400_A.pdf"] and re.fullmatch(UUID, dec["componente_id"]), "svincolo: %s" % dec)
    # PN-08: «Fissa l'emittente…» sul prodotto registrato presunto (decisione di-…): un'altra decisione, dopo la prima
    b.vista("org")
    b.scheda("k-root")
    i = b.segna()
    p.click("#du-fissa-emittente")
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
    # R5 ha applicato lo svincolo: la AR di ACM-1400_A.pdf è chiusa (direttiva 39 §7, AR dal servizio unico)
    verifica(b.stato()["file"]["ACM-1400_A.pdf"]["componente"] == "", "la conferma non ha chiuso la AR scollegata")
    # si torna allo stato delle prove che seguono (dati di prova): la AR rimessa dalla sonda (la pagina la rilegge con la
    # prossima lettura di S9)
    r = p.request.post(b.url + "/__banco/ar", data=json.dumps({"nome": "ACM-1400_A.pdf", "componente_id": C40}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda della AR: %d" % r.status)


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
    expect(p.locator(SENZA_BOZZA)).to_be_visible()
    r3 = b.richieste_da(i, "POST", r"/motore-a/bozza/[^/]+/abbandona$")
    verifica(len(r3) == 1 and json.loads(r3[0]["corpo"]) == {"versione_attesa": 2}, "R3: %s" % [r["corpo"] for r in r3])


def working_altrove(route):
    """/dati con fonte.autorizzata falsa: la working su un altro STEP (risposta finta dichiarata; il banco non ha il caso).
    Dopo 911d90e (collaudo UI46, B3) con la BOM confermata su uno STEP e nessuna bozza la Sorgente non offre più
    «Riprendi: crea la bozza» ma «Vai alla Revisione»: le prove che vogliono una bozza importata dallo STEP dopo una
    conferma (DU-B18, DU-B54) la chiedono con la working altrove."""
    r = route.fetch()
    try:
        dati = r.json()
    except Exception:
        route.fulfill(response=r)
        return

    def giro(x):
        if isinstance(x, dict):
            fo = x.get("fonte")
            if isinstance(fo, dict) and "autorizzata" in fo:
                fo["autorizzata"] = False
            for v in x.values():
                giro(v)
        elif isinstance(x, list):
            for v in x:
                giro(v)
    giro(dati)
    route.fulfill(response=r, body=json.dumps(dati))


@prova("18", "proposta cambiata: striscia, «Vedi le differenze», «Aggiorna alla proposta di oggi…»")
def b18(b):
    p = b.page
    a1 = b.al["ACM-1000_assieme.stp"]
    b.guarda(a1)
    # collaudo UI46, B3 (911d90e): con la BOM confermata su questo STEP e nessuna bozza non c'è niente da riprendere; il
    # gesto porta alla Revisione, dove le modifiche partono dalla BOM confermata
    expect(p.locator("#du-conferma-sorgente")).to_have_text("Vai alla Revisione di ACM-1000 →", timeout=T_LUNGO)
    # il resto della prova (la proposta cambiata dopo l'importazione) vuole una bozza importata dallo STEP: con la working
    # su un altro STEP (working_altrove) il gesto torna «Riprendi: crea la bozza», come prima dell'unione
    p.route("**/motore-a/dati*", working_altrove)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.guarda(a1)
        expect(p.locator("#du-conferma-sorgente")).to_have_text("Riprendi: crea la bozza di ACM-1000", timeout=T_LUNGO)
        p.click("#du-conferma-sorgente")
        b.ok_finestra()
        expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    finally:
        p.unroute("**/motore-a/dati*", working_altrove)
    b.passo("revisione")
    b.salvata(1)
    verifica(p.locator("#du-proposta-cambiata").count() == 0, "striscia della proposta cambiata su una bozza appena importata")
    b.proposta_cambia()
    p.click("#du-aggiorna")
    expect(p.locator("#du-proposta-cambiata")).to_contain_text("La proposta del Motore A è cambiata dopo l'importazione.", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-differenze")
    # collaudo UI46, B2: la differenza con il codice del pezzo e a parole, mai la chiave della bozza né i nomi dei campi
    # (il pezzo si nomina con il codice che ha nella bozza: dopo le prove di prima, nel banco è ACM-1120 o ACM-1121)
    expect(p.locator("#f-differenze")).to_contain_text(re.compile(r"ACM-112[01]: cambia [(]codice letto, codice[)]"), timeout=T_LUNGO)
    verifica("k-11" not in p.locator("#f-differenze").inner_text() and "codice_proposto" not in p.locator("#f-differenze").inner_text(), "le differenze mostrano la chiave o i campi del registro")
    verifica([r for r in b.richieste_da(i, "GET", r"/motore-a/bozza$") if "confronto=1" in r["query"]], "«Vedi le differenze» senza R1 ?confronto=1")
    b.chiudi_finestra()
    p.click("#du-riallinea")
    expect(b.finestra()).to_contain_text("Aggiorna la bozza alla proposta di oggi", timeout=T_LUNGO)
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza aggiornata alla proposta di oggi.", timeout=T_LUNGO)
    expect(p.locator("#du-ultima-forma")).to_contain_text("identità aggiornate: 1")
    expect(p.locator("#du-proposta-cambiata")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica("riallinea_proposta" in c and "snapshot" not in c and "azioni" not in c and "motivo" not in c["riallinea_proposta"] and len(c["riallinea_proposta"]["impronta_contenuto"]) == 64,
             "corpo di riallinea_proposta: %s" % c)
    b.salvata(2)


@prova("19", "«Usa nuovo STEP per ‹P›…» (S8 con la bozza, direttiva 39): nessuna revoca prima, l'effetto da S6, il motivo, sostituisci come presa d'atto; la working resta sullo STEP di prima")
def b19(b):
    p = b.page
    a10 = b.al["ACM-1000_rev2.stp"]
    i = b.segna()
    b.guarda(a10)
    s6 = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6 and "bozza=" in s6[-1]["query"], "S6 senza la bozza: %s" % [r["query"] for r in s6])
    expect(p.locator("#du-anteprima-codici")).to_contain_text("diverso dal codice del prodotto")
    # direttiva 39 §2: lo STEP della working (promosso dalle conferme della BOM) non si revoca prima del cambio: la working
    # resta su di lui fino alla prossima «Conferma BOM»; «Usa nuovo STEP» è subito disponibile
    verifica(b.stato()["fonte"]["autorizzata"] is True, "la working non ha lo STEP di prima: %s" % b.stato()["fonte"])
    expect(p.locator("#du-cambia-sorgente")).to_have_text("Usa nuovo STEP per ACM-1000…")
    expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-cambia-sorgente")
    eff = p.locator("#f-effetto")
    expect(eff).to_contain_text("nodi che restano: 1", timeout=T_LUNGO)
    expect(eff).to_contain_text("nodi che escono: 1")
    expect(eff).to_contain_text("nodi che entrano: 3")
    # integrazione UI46 (revisione della corsia SORGENTE, rilievo 1): i pezzi per nome, mai la chiave della bozza («k-…»)
    expect(eff).to_contain_text("ACM-1000")
    verifica(not re.search(r"\bk-(root|\d+)\b", eff.inner_text()), "l'effetto sulla bozza mostra le chiavi della bozza: %r" % eff.inner_text())
    expect(p.locator("#f-sostituisci")).to_contain_text("ACM-1000_assieme.stp")
    # l'effetto è quello di S6 riletto adesso con la bozza (MAT §1.5, SG-04)
    s6b = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6b and "bozza=" in s6b[-1]["query"], "«Usa nuovo STEP…» non rilegge S6 con la bozza prima del dialogo: %s" % [r["query"] for r in s6b])
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    i = b.segna()
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")[0]
    verifica("motivo" not in c and c["bozza"]["versione_attesa"] == 2 and c["sostituisci"] == {"conferma": True} and "/step/%s/" % a10 in b.richieste_da(i, "POST", r"/conferma-sorgente$")[0]["path"],
             "S8 del cambio: %s" % c)
    verifica(b.stato()["conti"].get("R2", 0) == 0 or not b.corpi(i, "POST", r"/motore-a/bozza$"), "il cambio della sorgente passa da R2: lo fa S8")
    verifica(not b.richieste_da(i, "POST", r"/step-strutturale"), "il cambio della sorgente ha chiamato l'autorizzazione o la revoca legacy")
    # la working resta sullo STEP di prima fino alla conferma della BOM (direttiva 39 §2, revisione)
    verifica(b.stato()["fonte"]["autorizzata"] is True, "S8 ha cambiato la working: %s" % b.stato()["fonte"])
    b.guarda(a10)
    expect(p.locator("#du-riallinea-da-step")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-riallinea-da-step")).to_have_text("Aggiorna proposta…")


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
    # la radice dello STEP rev D non ha il codice del prodotto: la pagina lo dice, senza chiedere un motivo (messaggio 46 [U])
    expect(p.locator("#f-radice-diversa")).to_contain_text("ACM-1000-R2")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza ripartita dalla proposta dello STEP", timeout=T_LUNGO)
    b.salvata(1)
    expect(p.locator("#du-vp .du-nc", has_text="ACM-1500")).to_have_count(0)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["bozza_id"] is None and c["versione_attesa"] == 0 and c["crea"]["partenza"] == "step" and c["crea"]["sorgente"]["allegato_id"] == a10
             and c["crea"]["abbandona"]["versione_attesa"] == 4 and "motivo" not in c["crea"]["abbandona"] and len(c["crea"]["impronta_contenuto"]) == 64
             and "motivo" not in c["crea"], "corpo della ripartenza: %s" % c)
    verifica(b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$"), "«Riparti» senza la proposta di oggi della sorgente (S6)")
    verifica(b.stato()["conti"].get("ripartenze") == 1, "il banco non ha visto la ripartenza")


@prova("21", "file: Scarta (D-6, D-7, D-14, archivio), Altri file, Riattiva, 503 NON_CONFIGURATO")
def b21(b):
    p = b.page
    a = b.al
    ok = p.locator("#du-finestra #f-ok")
    b.passo("sorgente")
    b.cartella("tutti")
    # lo STEP sorgente della bozza: «Scarta» e «Metti in Altri file» spenti con la ragione nel dettaglio (matrice v5, «STEP
    # sorgente»), fra le «Non disponibili»
    b.file(a["ACM-1000_rev2.stp"])
    nd = p.locator("#du-sg-nondisp")
    # messaggio 46 (D18, D29): il perché in parole, senza le versioni del progetto
    expect(nd.locator('[data-azione="scarta"]')).to_contain_text("è la sorgente di ACM-1000: prima revocala")
    expect(nd.locator('[data-azione="altri"]')).to_contain_text("è la sorgente di ACM-1000")
    verifica(p.locator("#du-sg-dettaglio #du-scarta-" + sid(a["ACM-1000_rev2.stp"])).count() == 0, "«Scarta» offerto sullo STEP sorgente")
    # direttiva 39 §2, §5: lo STEP superato come sorgente da «Usa nuovo STEP» resta un documento confermato (S8 non
    # sostituisce): non si scarta da qui
    b.file(a["ACM-1000_assieme.stp"])
    expect(nd.locator('[data-azione="scarta"]')).to_contain_text("è un documento confermato")
    # D-7: il file associato a un componente (AR) vuole «Scollega e scarta»
    b.azione("scarta", a["ACM-1400_A.pdf"])
    expect(p.locator('#f-dipendenze li[data-codice="D-7"]')).to_be_visible(timeout=T_LUNGO)
    expect(ok).to_be_disabled()
    verifica("Scollega e scarta" in (ok.get_attribute("title") or ""), "Scarta spento senza il motivo D-7")
    b.chiudi_finestra()
    # D-14: l'unica copia
    i = b.segna()
    b.azione("scarta", a["collaudo_interno.pdf"])
    expect(p.locator("#f-unica")).to_be_visible(timeout=T_LUNGO)
    expect(b.finestra()).to_contain_text("La mail originale, i documenti confermati e la storia non si toccano. Se il file è in Altri file, anche la sua copia nella cartella ALTRO della RFQ sul NAS viene eliminata. Non si annulla: per riaverlo dovrai ricaricarlo.")
    expect(ok).to_be_disabled()
    p.locator("#f-unica").check()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Scarto registrato; rimozione dallo staging ancora da completare.", timeout=T_LUNGO)
    s2 = b.corpi(i, "POST", r"/scarta$")
    verifica(len(s2) == 1 and s2[0].get("conferma_unica_copia") is True and "motivo" not in s2[0] and re.fullmatch(UUID, s2[0]["chiave_idempotenza"])
             and len(s2[0]["impronta_dipendenze"]) == 64 and s2[0]["versione_attesa"] == 0, "corpo di S2: %s" % s2)
    expect(p.locator("#du-sg-c-scartati .du-sg-conto")).to_have_text("1", timeout=T_LUNGO)
    b.cartella("scartati")
    riga = p.locator("#du-file-" + sid(a["collaudo_interno.pdf"]))
    expect(riga).to_have_attribute("data-stato", "scartato")
    expect(riga).to_contain_text("Scartato da AD")
    expect(riga).to_contain_text("File scartato ed eliminato dallo staging.", timeout=25000)
    # la riga dice lo stato di adesso (S1), non la frase della risposta di S2
    expect(riga).not_to_contain_text("ancora da completare")
    b.foto("B20_file_scartati.png")
    # lo ZIP con le voci attive: niente cascata, il server rifiuta (VOCI_ATTIVE) e il dialogo dice quali voci
    b.cartella("tutti")
    i = b.segna()
    b.azione("scarta", a["pacchetto.zip"])
    expect(p.locator("#f-scarta-zip")).to_contain_text("si scarta solo il contenitore", timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#du-finestra-errore")).to_contain_text("Lo ZIP ha 2 file estratti ancora attivi (voce_1.pdf, voce_2.pdf)", timeout=T_LUNGO)
    b.chiudi_finestra()
    verifica(b.stato()["file"]["voce_1.pdf"]["stato"] == "attivo" and b.stato()["file"]["pacchetto.zip"]["stato"] == "attivo", "VOCI_ATTIVE ha scritto")
    # le due voci in gruppo (D-V5-02, base [T] A): un dialogo, un motivo, l'esito file per file; poi lo ZIP da solo
    b.cartella("tutti")
    for nome in ("voce_1.pdf", "voce_2.pdf"):
        p.locator("#du-file-" + sid(a[nome]) + " .du-sg-spunta").check()
    expect(p.locator("#du-sg-gruppo")).to_contain_text("2 selezionati")
    i = b.segna()
    p.click("#du-sg-elenco #du-g-scarta")
    expect(p.locator("#f-gruppo li")).to_have_count(2, timeout=T_LUNGO)
    # messaggio 46 [U]: nessun motivo, «Scarta (2)» è acceso subito
    expect(ok).to_be_enabled()
    b.ok_finestra()
    expect(p.locator("#du-sg-mossa")).to_contain_text("2 file → Scartati", timeout=T_LUNGO)
    c2 = b.corpi(i, "POST", r"/scarta$")
    verifica(len(c2) == 2 and len({c["chiave_idempotenza"] for c in c2}) == 2 and all("motivo" not in c for c in c2), "S2 di gruppo: %s" % c2)
    b.azione("scarta", a["pacchetto.zip"])
    b.ok_finestra()
    expect(p.locator("#du-sg-c-scartati .du-sg-conto")).to_have_text("4", timeout=T_LUNGO)
    expect(p.locator("#du-sg-mossa")).to_contain_text("pacchetto.zip → Scartati")
    # Altri file e Riattiva
    b.azione("altri", a["PIASTRA_400123.dxf"])
    expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file: la copia è nella cartella ALTRO della RFQ.", timeout=T_LUNGO)
    expect(p.locator("#du-sg-c-altri .du-sg-conto")).to_have_text("1", timeout=T_LUNGO)
    b.cartella("altri")
    riga = p.locator("#du-file-" + sid(a["PIASTRA_400123.dxf"]))
    expect(riga).to_have_attribute("data-stato", "altri_file")
    expect(riga).to_contain_text("Copia nella cartella ALTRO della RFQ verificata")
    b.azione("riattiva", a["PIASTRA_400123.dxf"])
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File riattivato", timeout=T_LUNGO)
    expect(p.locator("#du-sg-c-altri .du-sg-conto")).to_have_text("0", timeout=T_LUNGO)
    # 503 NON_CONFIGURATO: il dialogo resta aperto, niente cambia
    b.cartella("tutti")
    b.modo("S3", "503")
    b.azione("altri", a["SCHEMA_GENERALE.pdf"])
    b.ok_finestra()
    expect(p.locator("#du-finestra-errore")).to_contain_text("La scrittura sul NAS è spenta su questo server: la cartella ALTRO sta sul NAS. Niente è cambiato. (NON_CONFIGURATO", timeout=T_LUNGO)
    expect(b.finestra()).to_be_visible()
    b.chiudi_finestra()
    verifica(b.stato()["file"]["SCHEMA_GENERALE.pdf"]["stato"] == "attivo", "il 503 ha cambiato il file")
    verifica(not b.sospesi(), "un 503 ha lasciato la chiave in sospeso")


@prova("65", "D-ASS-03 nel formato 3 (C-6b): «Scollega e metti in Altri file» come gesto composto, prima lo svincolo con R2 poi S3 con «scollega»; lo stato parziale se il secondo effetto non riesce; con lo svincolo già salvato D-6 chiede solo «Scollega»")
def b65(b):
    p = b.page
    a11 = b.al["ACM-1400_A.pdf"]
    ok = p.locator("#du-finestra #f-ok")
    st = b.stato()
    verifica(any(n.get("componente_id") == C40 for n in st["bozze"][b.p1]["snapshot"]["nodi"]), "la bozza aperta non ha ACM-1400")
    b.passo("sorgente")
    b.cartella("tutti")
    # 1. D-7 «blocca» (AR senza svincolo nella RFQ formato 3): il gesto composto; il secondo effetto rifiutato (risposta finta)
    b.azione("altri", a11)
    expect(p.locator('#f-dipendenze li[data-codice="D-7"]')).to_have_attribute("data-esito", "blocca", timeout=T_LUNGO)
    expect(p.locator("#f-scollega-composto")).to_contain_text("prima lo svincolo si salva nella bozza di ACM-1000")
    expect(ok).to_be_disabled()
    p.locator("#f-scollega").check()
    catturate = []
    g = rispondi_json(422, {"esito": "rifiutato", "codice": "CORPO_NON_VALIDO", "testo": "rifiuto di prova"}, cattura=catturate)
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        i = b.segna()
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("è salvato nella bozza; il gesto sul file non è riuscito", timeout=T_LUNGO)
        az = b.ultime_azioni(i)
        verifica(len(az) == 1 and az[0]["dopo"]["tipo"] == "scollega_documento" and az[0]["dopo"]["allegato_id"] == a11 and az[0]["dopo"]["componente_id"] == C40, "il primo effetto: %s" % az)
        b.attendi(lambda: len(catturate) == 1, "S3 non è partito dopo lo svincolo")
        verifica(catturate[0].get("scollega") is True and len(catturate[0]["impronta_dipendenze"]) == 64, "il secondo effetto: %s" % catturate[0])
        r2 = [r for r in b.richieste_da(i) if r["metodo"] == "POST" and r["path"].endswith("/motore-a/bozza")]
        s3 = [r for r in b.richieste_da(i) if r["metodo"] == "POST" and r["path"].endswith("/altri-file")]
        verifica(r2 and s3 and b.richieste.index(r2[0]) < b.richieste.index(s3[0]), "gli effetti non sono in ordine (prima R2, poi S3)")
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    b.chiudi_finestra()
    verifica(b.stato()["file"]["ACM-1400_A.pdf"]["stato"] == "attivo", "il rifiuto di S3 ha cambiato il file")
    # 2. con lo svincolo già nella bozza D-6 chiede «Scollega»: il gesto va avanti senza un altro R2
    b.azione("altri", a11)
    expect(p.locator('#f-dipendenze li[data-codice="D-6"]')).to_have_attribute("data-esito", "risoluzione", timeout=T_LUNGO)
    verifica(p.locator("#f-scollega-composto").count() == 0, "gesto composto con lo svincolo già salvato")
    p.locator("#f-scollega").check()
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file", timeout=T_LUNGO)
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "un altro svincolo salvato")
    verifica(b.corpi(i, "POST", r"/altri-file$")[0].get("scollega") is True, "S3 senza «scollega»")
    verifica(b.stato()["file"]["ACM-1400_A.pdf"]["stato"] == "altri_file", "il file non è in Altri file")
    # 3. si torna allo stato delle prove che seguono: il file riattivato, la AR rimessa (sonda), lo svincolo annullato
    b.azione("riattiva", a11, "altri")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File riattivato", timeout=T_LUNGO)
    r = p.request.post(b.url + "/__banco/ar", data=json.dumps({"nome": "ACM-1400_A.pdf", "componente_id": C40}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda della AR: %d" % r.status)
    b.passo("revisione")
    b.vista("org")
    p.click("#du-annulla")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.attendi(lambda: not any(d.get("tipo") == "scollega_documento" and d.get("allegato_id") == a11 for d in b.stato()["bozze"][b.p1]["snapshot"]["decisioni"]), "lo svincolo non è stato annullato")
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)


@prova("22", "S2 con la risposta persa: «Esito da verificare», R6 con la stessa chiave, «registrato»")
def b22(b):
    p = b.page
    a4 = b.al["SCHEMA_GENERALE.pdf"]
    g = perdi_la_risposta()
    p.route("**/sorgente/file/*/scarta", g)
    try:
        b.azione("scarta", a4)
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
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
    expect(p.locator("#du-sg-c-scartati .du-sg-conto")).to_have_text("5", timeout=T_LUNGO)


@prova("23", "S3 mai arrivata: R6 «non registrato», ricarica, «Riprova» dalla striscia con la stessa chiave")
def b23(b):
    p = b.page
    a3 = b.al["ACM-1120_B.pdf"]
    risorsa = "file:" + a3
    b.cartella("tutti")
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        b.azione("altri", a3)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
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
    expect(b.avviso()).to_contain_text("File messo in Altri file: la copia è nella cartella ALTRO della RFQ.", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1 and s3[0] == catturate[0], "«Riprova» non rimanda lo stesso corpo con la stessa chiave: %s" % s3)
    expect(striscia).to_have_count(0)
    verifica(not [x for x in b.sospesi() if x[1]["risorsa"] == risorsa], "la chiave resta in sospeso dopo «Riprova»")
    verifica(b.stato()["file"]["ACM-1120_B.pdf"]["stato"] == "altri_file", "il file non è in Altri file nel banco")


@prova("24", "«+ Aggiungi file» (E1′): parziale per file, in analisi poi letto, «già presente»")
def b24(b):
    p = b.page
    b.cartella("tutti")
    i = b.segna()
    b.apri_carica()
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
    riga = p.locator("#du-sg-elenco li.du-sg-riga", has_text="NUOVO_DISEGNO.pdf")
    expect(riga).to_contain_text("in analisi", timeout=T_LUNGO)
    expect(riga).to_contain_text("interna")
    expect(riga).to_contain_text("letto", timeout=25000)
    b.apri_carica()
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
    # direttiva 39 §2: nessun atto dell'autorizzazione nella pagina nuova (la revoca della sorgente sta nella DU-B62)
    verifica(not b.richieste_da(0, "POST", r"/step-strutturale"), "la pagina ha chiamato un atto dell'autorizzazione")
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
    b.file(dxf)
    href = p.locator("#du-apri-" + sid(dxf)).get_attribute("href")
    verifica(href == "/thread/%s/sorgente/file/%s/contenuto?modo=anteprima" % (b.thread, dxf), "«Apri»: %s" % href)
    resp = p.request.get(b.url + href)
    verifica(resp.status == 200 and "attachment" in resp.headers.get("content-disposition", "") and resp.headers.get("x-content-type-options") == "nosniff",
             "S5: %d %s" % (resp.status, resp.headers))
    verifica(not b.richieste_da(i, "GET", r"/contenuto$"), "la pagina ha chiesto S5 con una fetch")
    # «Apri» di un PDF: il visore pdf.js della pagina, che legge l'indirizzo di S5 (modo=anteprima), una pagina alla volta
    a2 = b.al["ACM-1100_A.pdf"]
    b.file(a2)
    i = b.segna()
    p.click("#du-apri-" + sid(a2))
    expect(p.locator("#du-visore-tela canvas")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-visore-pagina")).to_have_text("pagina 1 di 1")
    expect(p.locator("#du-visore-dopo")).to_be_disabled()
    s5 = b.richieste_da(i, "GET", r"/contenuto$")
    verifica(s5 and all("modo=anteprima" in r["query"] for r in s5) and all(r["headers"].get("accept") != "application/json" for r in s5),
             "il visore non legge S5 per indirizzo: %s" % [(r["path"], r["query"]) for r in s5])
    b.chiudi_finestra()


@prova("26", "secondo prodotto: indirizzo; senza candidate la sua cartella lo dice; lo STEP non candidato si guarda e si conferma con S8 (niente S7)")
def b26(b):
    p = b.page
    a10 = b.al["ACM-1000_rev2.stp"]
    tab2 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)
    tab2.click()
    expect(tab2).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.attendi(lambda: b.indirizzo().get("prodotto") == b.p2, "l'indirizzo non porta il secondo prodotto")
    b.passo("sorgente")
    b.cartella("p:" + b.p2)
    expect(p.locator("#du-sg-c-p_" + sid(b.p2))).to_contain_text("○ da confermare · 0 STEP")
    if p.locator("#du-nessuna-candidata").count() == 0:
        p.click("#du-sg-prodotto-apri")
    expect(p.locator("#du-nessuna-candidata")).to_contain_text("Il Motore A non propone nessuno STEP per ACM-2000")
    # lo STEP che il motore non propone per ACM-2000: «Guarda l'anteprima per ACM-2000», niente S7 («Associa lo STEP…», che il
    # server non registra), nessuna anteprima inventata
    b.file(a10, "step")
    expect(p.locator("#du-sg-candidature")).to_contain_text("Guarda l'anteprima per ACM-2000")
    verifica("Associa lo STEP" not in p.locator("#du-sg-candidature").inner_text(), "la candidatura offre ancora S7 per ACM-2000")
    verifica(p.locator("#du-guarda-" + sid(a10) + "-" + sid(b.p2)).count() == 0, "una candidatura inventata per ACM-2000")
    # direttiva 39 §3: lo STEP senza struttura per ACM-2000 si guarda lo stesso (S6 dice «nessuna struttura importabile») e
    # si può confermare come sorgente partendo dal solo prodotto, con la presa d'atto; qui solo guardato
    i = b.segna()
    b.altri_prodotti()
    p.click("#du-sg-dettaglio #du-guarda-nc-" + sid(b.p2) + "-" + sid(a10))
    expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-struttura-step")).to_have_attribute("data-struttura", "nessuna_struttura")
    expect(p.locator("#du-struttura-step")).to_contain_text("nessuna struttura importabile")
    expect(p.locator("#du-conferma-solo-prodotto")).to_be_enabled()
    verifica(p.locator("#du-conferma-sorgente, #du-cambia-sorgente").count() == 0, "S8 dallo STEP con una struttura inventata per ACM-2000")
    s6 = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
    verifica(s6 and ("prodotto=" + b.p2) in s6[-1]["query"] and "gruppo=" not in s6[-1]["query"], "S6 per ACM-2000: %s" % [r["query"] for r in s6])
    p.click("#du-anteprima-chiudi")
    verifica(not b.richieste_da(i, "POST", r"/conferma-sorgente$"), "guardare l'anteprima ha confermato la sorgente")
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


@prova("29", "Sorgente (mockup v5): lo smistamento di S9 nella cartella del prodotto; «Salva nella bozza», «Annulla la mossa», «Togli dalla bozza»; «Associa a un nodo…» con la riga fantasma; «Scarica»")
def b29(b):
    p = b.page
    a2 = b.al["ACM-1100_A.pdf"]
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    riga = p.locator("#du-file-" + sid(a2))
    expect(riga).to_contain_text("◇ proposto", timeout=T_LUNGO)
    expect(riga).to_contain_text("ACM-1100")
    b.file(a2, "p:" + b.p1)
    sc = p.locator("#du-scarica-" + sid(a2))
    verifica(sc.get_attribute("href") == "/thread/%s/sorgente/file/%s/contenuto?modo=scarica" % (b.thread, a2) and sc.get_attribute("download") == "ACM-1100_A.pdf",
             "«Scarica»: %s %s" % (sc.get_attribute("href"), sc.get_attribute("download")))
    i = b.segna()
    p.click("#du-sg-dettaglio #du-salva-" + sid(a2))
    expect(riga).to_contain_text("◈ salvato nella bozza", timeout=T_LUNGO)
    expect(p.locator("#du-sg-mossa")).to_contain_text("ACM-1100_A.pdf → salvato nella bozza di ACM-1000")
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Salva nella bozza»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": b.al["ACM-1100_A.pdf"], "chiave_bozza": "k-7"})], "«Salva nella bozza»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # «Annulla la mossa»: l'inverso esatto, una R2 nuova
    i = b.segna()
    p.click("#du-sg-mossa-annulla")
    expect(riga).to_contain_text("◇ proposto", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Annulla la mossa»")
    verifica([a["azione"] for a in b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]] == ["annulla"], "«Annulla la mossa» non manda annulla{n}")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # «Togli dalla bozza»
    p.click("#du-sg-dettaglio #du-salva-" + sid(a2))
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-sg-dettaglio #du-togli-" + sid(a2))
    expect(riga).to_contain_text("◇", timeout=T_LUNGO)
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Togli dalla bozza»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([a["azione"] for a in az] == ["file_svincolato"] and az[0]["dopo"]["motivo"] == "utente", "«Togli dalla bozza»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # «Associa a un nodo…» di un file senza proposta: il dialogo della v4.2 (prodotto preselezionato, nodo da scegliere)
    nd = b.allegato_di("NUOVO_DISEGNO.pdf")
    b.file(nd, "dc-senza")
    i = b.segna()
    p.click("#du-sg-dettaglio #du-associa-" + sid(nd))
    expect(p.locator("#f-senza-codice")).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("#f-prodotto").input_value() == b.p1 and p.locator("#f-pezzo").input_value() == "", "«Associa a un nodo…»: preselezioni sbagliate")
    verifica(p.locator('#f-prodotto option[value="%s"]' % b.p2).get_attribute("disabled") is not None, "ACM-2000 senza bozza offerto nel dialogo")
    ok = p.locator("#du-finestra #f-ok")
    expect(ok).to_be_disabled()
    verifica("scegli il prodotto e il nodo" in (ok.get_attribute("title") or ""), "«Associa» spento senza la ragione")
    p.select_option("#f-pezzo", "k-12")
    b.ok_finestra()
    expect(p.locator("#du-sg-fantasma")).to_contain_text("NUOVO_DISEGNO.pdf → ACM-1000", timeout=T_LUNGO)
    expect(p.locator("#du-sg-mossa")).to_contain_text("NUOVO_DISEGNO.pdf → ACM-1000")
    b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bozza$"), "nessun salvataggio dopo «Associa a un nodo…»")
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica([(a["azione"], a["dopo"]["chiave_bozza"]) for a in az] == [("file_preassociato", "k-12")], "«Associa a un nodo…»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    p.click("#du-sg-mossa-annulla")
    expect(p.locator("#du-salvataggio")).to_contain_text("Bozza salvata", timeout=T_LUNGO)
    b.attendi(lambda: b.stato()["bozze"][b.p1]["snapshot"].get("documenti_preassociati", []) == [], "l'annullamento non ha tolto la preassociazione")


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
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": b.al["PIASTRA_400123.dxf"], "chiave_bozza": "k-41"})], "preassociazione dal pannello: %s" % az)
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
    b.cartella("p:" + b.p1)
    if p.locator("#du-sg-fonte").count() == 0:
        p.click("#du-sg-prodotto-apri")
    expect(p.locator("#du-step-secondario")).to_contain_text("STEP secondario disponibile")
    expect(p.locator("#du-step-secondario")).to_contain_text("ACM-1000_rev2.stp")
    expect(p.locator("#du-step-secondario")).to_contain_text("non si accettano ancora")
    verifica(p.locator("#du-nessuna-candidata").count() == 0, "«nessuna candidata» per un prodotto con candidate")
    b.cartella("p:" + b.p2)
    if p.locator("#du-nessuna-candidata").count() == 0:
        p.click("#du-sg-prodotto-apri")
    expect(p.locator("#du-nessuna-candidata")).to_contain_text("ACM-2000")
    expect(p.locator("#du-nessuna-candidata")).to_contain_text("+ Aggiungi file")
    verifica(p.locator("#du-step-secondario").count() == 0, "STEP secondario per un prodotto senza")
    b.cartella("p:" + b.p1)
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
    aid = b.allegato_di("NUOVO_DISEGNO.pdf")
    i = b.segna()
    p.select_option("#du-pannello-file", label="NUOVO_DISEGNO.pdf")
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-doc-nodo")).to_contain_text("◈ preassociato", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": aid, "chiave_bozza": k})], "preassociazione: %s" % az)
    expect(p.locator("#du-doc-nodo")).to_contain_text("in_attesa_codice", timeout=T_LUNGO)
    # riga 2: «Non crearlo» toglie il nodo e manda il file nel vassoio (file_svincolato nodo_tolto), nello stesso salvataggio
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-togli")
    expect(b.finestra()).to_contain_text("1 file preassociati vanno in «Da ricollocare»")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="nuovo"]')).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_tolto", "file_svincolato", "nodo_tolto"] and az[1]["dopo"] == {"allegato_id": aid, "motivo": "nodo_tolto"},
             "«Non crearlo»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # il vassoio nella linguetta «Da ricollocare» della Sorgente (MAT §3.3) e nell'Elenco
    b.passo("sorgente")
    expect(p.locator("#du-sg-c-dc-ric_" + sid(b.p1) + " .du-sg-conto")).to_have_text("1", timeout=T_LUNGO)
    b.cartella("dc-ric:" + b.p1)
    expect(p.locator("#du-sg-elenco")).to_contain_text("NUOVO_DISEGNO.pdf")
    b.cartella("tutti")
    b.passo("revisione")
    b.vista("elenco")
    vass = p.locator("#du-vassoio")
    expect(vass).to_contain_text("NUOVO_DISEGNO.pdf")
    expect(vass).to_contain_text("nodo_tolto")
    i = b.segna()
    p.click("#du-lascia-" + sid(aid))
    expect(vass).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_lasciato", {"allegato_id": aid})], "«Lascia senza associazione»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


@prova("36", "§4.3 righe 3 e 4: «Escludi» un registrato (scelta per le AR, documenti confermati, file nel vassoio con nodo_escluso, svincoli con da_decisione) e «Ripristina» dopo una ricarica")
def b36(b):
    p = b.page
    aid = b.allegato_di("NUOVO_DISEGNO.pdf")
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
    # PE-27: lo svincolo cita l'esclusione in da_decisione, al posto della convenzione .svN; nessuno dei due ha un motivo
    # (messaggio 46 [U])
    verifica(sv["allegato_id"] == b.al["ACM-1400_A.pdf"] and sv.get("da_decisione") == es["id"] and ".sv" not in sv["id"] and sv["componente_id"] == es["componente_id"]
             and "motivo" not in sv and "motivo" not in es, "svincolo dell'esclusione: %s" % sv)
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
    bt = p.locator("#du-ricolloca-" + sid(aid))
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
             and az[2]["prima"]["id"] == sv["id"] and az[3]["dopo"] == {"allegato_id": aid, "chiave_bozza": "k-40"}, "«Ripristina»: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    expect(p.locator("#du-vassoio")).to_have_count(0)
    expect(p.locator("#el-k-40")).to_contain_text("● associato")
    expect(p.locator("#el-k-40")).to_contain_text("◈ preassociato")


@prova("61", "direttiva 35, C-2: la preassociazione per allegato_id; un file senza proposta di documento si preassocia (Prova 2), niente STEP né archivi fra i file da smistare; «Sposta a un altro nodo…» manda svincolo e preassociazione")
def b61(b):
    p = b.page

    def pannello():
        b.passo("revisione")
        b.vista("org")
        b.scheda("k-41")
        b.linguetta("documenti")
        expect(p.locator("#du-sez-documenti")).to_be_visible()
        return [x for x in p.locator("#du-pannello-file option").all_inner_texts() if not x.startswith("—")]

    opz = pannello()
    verifica(opz and not [x for x in opz if re.search(r"\.(stp|step|zip|7z|rar)$", x, re.I)], "file da smistare nel pannello (né STEP né archivi): %s" % opz)
    nome = opz[0]
    aid = b.allegato_di(nome)
    # Prova 2: lo stesso file senza proposta di documento (S1 con proposta_id e proposta null) resta fra i file da smistare
    r = p.request.post(b.url + "/__banco/interruttore", data=json.dumps({"nome": "senza-proposta", "attivo": True, "file": aid}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda non ha preso l'interruttore senza-proposta: %d" % r.status)
    p.reload()
    b.pronta()
    opz = pannello()
    verifica(nome in opz, "il file senza proposta non è fra i file da smistare: %s" % opz)
    i = b.segna()
    p.select_option("#du-pannello-file", label=nome)
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-doc-nodo")).to_contain_text(nome, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": aid, "chiave_bozza": "k-41"})], "preassociazione senza proposta: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    pb = b.stato()["bozze"][b.p1]["snapshot"]["documenti_preassociati"]
    # la PB salvata porta anche la provenienza, che decide il server (direttiva 39 §4: una PB nuova della pagina è manuale)
    verifica({"allegato_id": aid, "chiave_bozza": "k-41", "provenienza": "manuale"} in pb and not [x for x in pb if "proposta_id" in x], "PB salvate dal banco: %s" % pb)
    # «Sposta a un altro nodo…» dalla Sorgente: il server non sostituisce una PB, quindi svincolo e preassociazione
    i = b.segna()
    b.azione("sposta", aid)
    p.select_option("#f-pezzo", "k-7")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_svincolato", "file_preassociato"] and az[0]["prima"] == {"allegato_id": aid, "chiave_bozza": "k-41"}
             and az[0]["dopo"] == {"allegato_id": aid, "motivo": "utente"} and az[1]["dopo"] == {"allegato_id": aid, "chiave_bozza": "k-7"}, "«Sposta»: %s" % az)
    # lo stato di prima: «Togli dalla bozza», poi il file torna con la sua proposta
    i = b.segna()
    b.azione("togli", aid)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["prima"], a["dopo"]) for a in az] == [("file_svincolato", {"allegato_id": aid, "chiave_bozza": "k-7"}, {"allegato_id": aid, "motivo": "utente"})], "«Togli dalla bozza»: %s" % az)
    r = p.request.post(b.url + "/__banco/interruttore", data=json.dumps({"nome": "senza-proposta", "attivo": False, "file": aid}), headers={"Content-Type": "application/json"})
    verifica(r.status == 204, "la sonda non ha tolto l'interruttore senza-proposta: %d" % r.status)
    p.reload()
    b.pronta()
    # dove la DU-B36 lascia la pagina, per la prova che segue: la Revisione, l'Elenco
    b.passo("revisione")
    b.vista("elenco")


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
    expect(b.finestra()).to_contain_text("Anche il commerciale chiede il 2D: non lo chiede solo la minuteria confermata da una persona")
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
    # «Adotta come codice…» del P/N K4: la lettura del codice della parte nel PDF della prova sta in file[].letture[] del
    # registro con la sua proiezione (U2-P18; DEC-B PD-07 (a)), quindi il gesto è acceso (DU-B38 rovesciata, ondata C)
    ad = rif.locator("button", has_text="Adotta come codice")
    expect(ad).to_be_enabled()
    bt = p.locator("#du-conferma-identita")
    expect(bt).to_be_enabled()
    i = b.segna()
    bt.click()
    expect(b.finestra()).to_contain_text("400777")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    b.ok_finestra()
    expect(p.locator("#nc-k-41")).to_have_attribute("data-du-stato", "decisa in bozza", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"]
    verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and dec["tipo"] == "identita" and dec["modo"] == "alternativa"
             and dec["chiave_lettura"].endswith("/l:#41") and dec["lettura"]["letterale"] == "ACM-1410" and dec["identita"]["codice"] == "ACM-1410"
             and "motivo" not in dec and len(dec["importazione"]) == 64 and az[1]["dopo"]["identita"]["decisione"] == dec["id"]
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
    # adesso il P/N K4: «Adotta come codice…» cita la lettura del PDF della prova (<allegato>/<lettura_codice>), con la sua
    # proiezione e l'emittente del fornitore; la decisione di prima dello stesso nodo si toglie nello stesso gesto (PE-B16)
    i = b.segna()
    rif.locator("button", has_text="Adotta come codice").click()
    expect(b.finestra()).to_contain_text("400777")
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = az[1]["dopo"] if len(az) > 1 else {}
    verifica([a["azione"] for a in az] == ["decisione_tolta", "decisione_aggiunta", "nodo_modificato"] and dec.get("modo") == "alternativa"
             and dec.get("chiave_lettura") == b.al["ACM-1100_A.pdf"] + "/l:pdf:pn" and dec["identita"] == {"emittente": "partner:beta", "namespace": "beta", "codice": "400777"}
             and dec["lettura"]["letterale"] == "400777", "«Adotta come codice» del P/N K4: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.scheda("k-11")
    b.linguetta("dettaglio")
    li = p.locator("#du-sez-riferimenti li", has_text="codice corretto a mano")
    expect(li).to_contain_text("ACM-1125")
    i = b.segna()
    li.locator("button", has_text="Adotta come codice").click()
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
        # con l'identità registrata del componente data da /dati (decisione_esistente.identita_registrata dei nodi del
        # gruppo: EM §9.9, PE-10; B-3) il gesto si accende e il nodo torna «registrato», con quell'identità byte per byte
        # (MAT NI-07, BZ-15: «la corsia interfaccia accende il gesto all'ondata C»)
        letture_dati = re.compile(r"/motore-a/dati(\?|$)")

        def con_identita(route):
            if route.request.method == "GET" and "sorgente=1" not in route.request.url:
                resp = route.fetch()
                j = resp.json()
                for g in j.get("gruppi") or []:
                    for n in g.get("nodi") or []:
                        if str(n.get("rif", "")).endswith(":#32"):
                            n["decisione_esistente"] = {"componente_id": "00000000-0000-4000-8000-000000009999", "identita_registrata": {
                                "emittente": "cliente:" + b.dati["cliente"], "emittente_presunto": True, "codice": "ACM-1199", "rev": "A", "modo": "registrato"}}
                route.fulfill(response=resp, json=j)
            else:
                route.continue_()
        p.route(letture_dati, con_identita)
        try:
            p.click("#du-aggiorna")
            expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
            p.click("#du-zoom-adatta")
            noninc.first.click()
            expect(p.locator("#du-includi")).to_be_enabled()
            i = b.segna()
            p.click("#du-includi")
            az = b.ultime_azioni(i)
            nodo = az[1]["dopo"] if len(az) > 1 else {}
            verifica([a["azione"] for a in az][:2] == ["decisione_tolta", "nodo_aggiunto"] and nodo.get("op") == "riusa" and nodo.get("componente_id") == "00000000-0000-4000-8000-000000009999"
                     and nodo["identita"] == {"emittente": "cliente:" + b.dati["cliente"], "emittente_presunto": True, "codice": "ACM-1199", "rev": "A", "modo": "registrato"},
                     "«Includi di nuovo» di una voce registrata: %s" % az)
            expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
            # si torna com'era: «Annulla» (il componente 9999 non è del banco)
            p.click("#du-annulla")
            expect(noninc).to_have_count(1, timeout=T_LUNGO)
            expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        finally:
            p.unroute(letture_dati, con_identita)
    finally:
        p.unroute(letture_r1, con_decisione)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    # riga 5: la proposta d'associazione di un nodo non incluso è «non richiesta: nodo non incluso (bozza)»
    b.scheda("k-7")
    b.linguetta("dettaglio")
    p.click("#du-non-includere-nodo")
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    riga = p.locator("#du-elenco [role=row][data-k^='ni:']", has_text="ACM-1100")
    expect(riga).to_contain_text("◇ non richiesta: nodo non incluso (bozza)")
    expect(riga).to_contain_text("ACM-1100_A.pdf")
    b.passo("sorgente")
    b.cartella("tutti")
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
        verifica(p.locator("#du-sg-elenco li.du-sg-riga").count() == 0, "con S1 in 5xx la pagina mostra l'elenco vecchio dei file")
        expect(p.locator("#du-step-non-letti")).to_be_visible()
    finally:
        p.unroute(lettura_s1, g)
    p.click("#du-aggiorna")
    expect(p.locator("#du-sg-elenco li.du-sg-riga").first).to_be_visible(timeout=T_LUNGO)
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
    # il cambio della sorgente è S8 (U2 §2.2): i rifiuti del passo 0 (risposte finte) lasciano il dialogo aperto con il
    # motivo (STEP_SORGENTE_DI_ALTRO_PRODOTTO, MOTIVO_OBBLIGATORIO); 409 BOZZA_VERSIONE lo chiude e rilegge R1
    b.guarda(a1)
    expect(p.locator("#du-cambia-sorgente")).to_be_enabled(timeout=T_LUNGO)
    s8_prima = b.stato()["conti"].get("S8", 0)
    casi = [
        ({"esito": "rifiutato", "codice": "STEP_SORGENTE_DI_ALTRO_PRODOTTO", "testo": "Questo STEP è già la sorgente di ACM-2000"}, 422, "uno STEP è la sorgente di un solo prodotto", False),
        # un server di prima del messaggio 46 che chiede ancora il motivo: la pagina lo dice, niente scritto
        ({"esito": "rifiutato", "codice": "MOTIVO_OBBLIGATORIO", "campo": "motivo", "testo": "serve il motivo"}, 422, "Il server chiede ancora un motivo scritto", False),
        ({"esito": "conflitto", "codice": "BOZZA_VERSIONE", "motivo": "altra_bozza_aperta", "testo": "la bozza è cambiata"}, 409, "La bozza è cambiata altrove", True),
    ]
    for corpo, stato, testo, chiude in casi:
        g = rispondi_json(stato, corpo)
        p.route("**/conferma-sorgente", g)
        try:
            if p.locator("#du-cambia-sorgente").count() == 0:
                b.guarda(a1)
            p.click("#du-cambia-sorgente")
            expect(p.locator("#f-effetto")).to_be_visible(timeout=T_LUNGO)
            i = b.segna()
            b.ok_finestra()
            if chiude:
                expect(b.avviso()).to_contain_text(testo, timeout=T_LUNGO)
                expect(b.finestra()).to_be_hidden()
            else:
                expect(p.locator("#du-finestra-errore")).to_contain_text(testo, timeout=T_LUNGO)
                expect(b.finestra()).to_be_visible()
                b.chiudi_finestra()
        finally:
            p.unroute("**/conferma-sorgente", g)
    verifica(b.stato()["conti"].get("S8", 0) == s8_prima, "i rifiuti finti hanno scritto: S8 = %s" % b.stato()["conti"].get("S8"))
    # la rete caduta su S8: «Esito da verificare», R6 con la stessa chiave dice «non registrato», «Riprova la conferma della
    # sorgente» dalla striscia rimanda lo stesso corpo con la stessa chiave; la procedura riprende e finisce
    catturate = []
    g = blocca_la_richiesta(catturate)
    p.route("**/conferma-sorgente", g)
    try:
        if p.locator("#du-cambia-sorgente").count() == 0:
            b.guarda(a1)
        p.click("#du-cambia-sorgente")
        expect(p.locator("#f-effetto")).to_be_visible(timeout=T_LUNGO)
        b.ok_finestra()
        riprova = p.locator("#du-riprova-sorgente_" + sid(b.p1))
        expect(riprova).to_be_visible(timeout=T_LUNGO)
    finally:
        p.unroute("**/conferma-sorgente", g)
    verifica(len(catturate) == 1, "richieste S8 bloccate: %d" % len(catturate))
    i = b.segna()
    riprova.click()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c) == 1 and c[0] == catturate[0], "«Riprova» non rimanda lo stesso corpo con la stessa chiave: %s / %s" % (c, catturate))
    expect(p.locator("#du-sospeso-sorgente_" + sid(b.p1))).to_have_count(0)
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
    b.cartella("tutti")
    # S3 409 ANTEPRIMA_SUPERATA (risposta finta): il dialogo si riapre con il dettaglio riletto, poi il gesto passa
    g = rispondi_json(409, {"esito": "conflitto", "codice": "ANTEPRIMA_SUPERATA", "anteprima": "voci_archivio", "testo": "Il perimetro delle voci è cambiato."})
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        i = b.segna()
        # il gesto esteso, con il perimetro delle voci (U2 §3.10, [U] Q21): «Metti in Altri file anche i contenuti (N)…»
        b.azione("altri-voci", zip2)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
        expect(p.locator("#f-voci-altri")).to_contain_text("voce_3.pdf")
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("rivedi il dialogo aggiornato", timeout=T_LUNGO)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS")
        b.attendi(lambda: len([r for r in b.richieste_da(i, "GET", r"/sorgente$") if "file=" in r["query"]]) >= 2, "il dettaglio di S1 non è riletto dopo il 409")
    finally:
        p.unroute("**/sorgente/file/*/altri-file", g)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1 and s3[0]["perimetro"] == "contenitore_e_voci" and s3[0]["n_voci"] == 2 and len(s3[0]["impronta_voci"]) == 64, "S3 dell'archivio: %s" % s3)
    # S4 di un archivio: il dialogo dice le voci che tornano attive, S4 porta n_voci e impronta_voci del dettaglio (FL-10);
    # un 409 ANTEPRIMA_SUPERATA riapre il dialogo
    b.cartella("altri")
    g = rispondi_json(409, {"esito": "conflitto", "codice": "ANTEPRIMA_SUPERATA", "anteprima": "voci_archivio", "testo": "Il perimetro delle voci è cambiato."})
    p.route("**/sorgente/file/*/riattiva", g)
    try:
        b.azione("riattiva-voci", zip2)
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
    verifica(len(s4) == 1 and s4[0]["perimetro"] == "contenitore_e_voci" and s4[0]["n_voci"] == 2 and len(s4[0]["impronta_voci"]) == 64, "S4 dell'archivio senza il perimetro delle voci: %s" % s4)
    verifica(b.stato()["file"]["voce_3.pdf"]["stato"] == "attivo", "le voci dell'archivio non sono tornate attive")
    b.cartella("tutti")
    # S3 409 FILE_VERSIONE (risposta finta): il dialogo si chiude e S1 si rilegge, con lo stato di prima e di adesso
    g = rispondi_json(409, {"esito": "conflitto", "codice": "FILE_VERSIONE", "stato": "altri_file", "versione": 9, "testo": "Lo stato del file è cambiato."})
    p.route("**/sorgente/file/*/altri-file", g)
    try:
        b.azione("altri", v4)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
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
        b.azione("altri", v4)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
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
        b.azione("scarta", v3)
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
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
        b.azione("scarta", v3)
        expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
        b.ok_finestra()
        expect(b.avviso()).to_contain_text("(l'esito di allora non è più conservato)", timeout=T_LUNGO)
    finally:
        p.unroute("**/sorgente/file/*/scarta", gp)
        p.unroute("**/motore-a/intento/*", g6)
    verifica(b.stato()["file"]["voce_3.pdf"]["stato"] == "scartato", "lo scarto non è nel banco")
    verifica(not [x for x in b.sospesi() if x[1]["risorsa"] == "file:" + v3], "la chiave resta in sospeso dopo «registrato_senza_esito»")
    # S7 non c'è (il server non registra …/associa: rilievo del 20/10, [T-39]): lo STEP non candidato per ACM-2000 ha solo
    # l'anteprima per quel prodotto, poi S8; nessun gesto «Associa lo STEP…» e nessuna richiesta a …/associa
    a10 = b.al["ACM-1000_rev2.stp"]
    b.file(a10, "step")
    b.altri_prodotti()
    expect(p.locator("#du-sg-dettaglio #du-guarda-nc-" + sid(b.p2) + "-" + sid(a10))).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("[id^='du-associa-step']").count() == 0, "la pagina offre ancora S7 («Associa lo STEP…»)")
    verifica("Associa lo STEP" not in p.locator("#du-sg-dettaglio").inner_text(), "il dettaglio dello STEP nomina ancora S7")
    verifica(not [r for r in b.richieste if re.search(r"/associa$", r["path"])], "la pagina ha chiamato …/associa")
    tab1 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)
    tab1.click()
    expect(tab1).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    # E1′: 422 NESSUN_FILE_CARICATO con i motivi file per file; 413 (risposta finta) «troppo grande»
    esiti = p.locator("#du-carica-esiti")
    b.apri_carica()
    p.set_input_files("#du-carica-file", files=[{"name": "solo.exe", "mimeType": "application/octet-stream", "buffer": b"MZ sintetico"}])
    p.click("#du-carica-invia")
    expect(esiti.locator('li[data-esito="rifiutato"]')).to_contain_text("tipo di file non ammesso", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("Nessun file è stato caricato")
    g = rispondi_json(413, {"esito": "rifiutato", "codice": "TROPPO_GRANDE", "testo": "troppo grande"})
    p.route("**/thread/*/carica", g)
    try:
        b.apri_carica()
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
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    b.ok_finestra()
    expect(p.locator("#du-base-superata")).to_have_count(0, timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica("motivo" not in c["rebase"] and "base" in c["rebase"] and [a["azione"] for a in c["azioni"]] == ["rebase"], "il salvataggio del rebase: %s" % c)
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
    # collaudo UI46, I7: i motivi senza le versioni del progetto
    for motivo in ("I documenti si confermano con «Rivedi e conferma fascicolo», non da questa pagina", "La correzione di un file non è in questa pagina", "Le deroghe non sono in questa pagina",
                   "La sostituzione dei documenti non è in questa pagina"):
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
        # il motivo lo dice il codice della risposta di S5 (X-Motivo o il codice della busta: EM §10.9; DEC-B PF-25), letto
        # dal ponte con una richiesta di un byte dopo che il visore non ha aperto il file
        expect(p.locator("#du-visore-errore")).to_contain_text("Contenuto non disponibile: i byte del file non ci sono più", timeout=T_LUNGO)
        expect(p.locator("#du-visore-errore")).to_have_attribute("data-du-motivo", "BYTE_NON_DISPONIBILI")
    finally:
        p.unroute("**/sorgente/file/*/contenuto*", g)
    b.chiudi_finestra()
    # «da ricodificare»: una ricodifica del registrato fa seguire il codice nuovo ai suoi file (MAT §4.3 riga 7)
    b.linguetta("dettaglio")
    p.click("#du-correggi")
    p.fill("#f-codice", "ACM-1401")
    b.ok_finestra()
    # D-ASS-01 [U]: il file registrato non corrisponde più al codice nuovo; «Mantieni i collegamenti»: segue il codice nuovo
    expect(p.locator("#f-discordanti [data-du-discordante='%s']" % b.al["ACM-1400_A.pdf"])).to_have_attribute("data-tipo", "ar", timeout=T_LUNGO)
    expect(p.locator("#f-discordanza")).to_contain_text("seguono il codice nuovo alla conferma")
    p.click("#f-mantieni-discordanti")
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    b.vista("elenco")
    expect(p.locator("#el-k-40")).to_contain_text("da ricodificare")
    b.passo("sorgente")
    b.cartella("tutti")
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
    for k in ("dc", "dc-senza", "dc-analisi", "step", "prodotti", "altri", "scartati", "tutti"):
        expect(p.locator("#du-sg-c-" + sid(k))).to_be_visible()
    b.cartella("p:" + b.p1)
    expect(p.locator("#du-sg-elenco")).to_contain_text("ACM-1400_A.pdf")
    expect(p.locator("#du-file-" + sid(b.al["ACM-1400_A.pdf"]))).to_contain_text("● già registrato")
    b.cartella("dc-senza")
    verifica("ACM-1400_A.pdf" not in p.locator("#du-sg-elenco").inner_text(), "un file associato fra quelli senza proposta")
    b.cartella("altri")
    expect(p.locator("#du-file-" + sid(b.al["voce_4.pdf"]))).to_contain_text("In Altri file da AD")
    b.cartella("tutti")
    # «Gestisci nella Sorgente» dall'Elenco (MAT §3.6)
    b.passo("revisione")
    b.vista("elenco")
    p.click("#du-gestisci-sorgente")
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    b.vista("org")
    # una sola candidata incerta: messaggio 46 [U] (supera SG-02 per l'anteprima): «Guarda l'anteprima» la mette in vista
    # subito, con la struttura e il gesto della conferma; la conferma resta il gesto separato
    a1 = b.al["ACM-1000_assieme.stp"]
    b.interruttore("s6-incerta", True)
    try:
        p.click("#du-aggiorna")
        b.cartella("step")
        # la riga dice «incerta» solo se lo STEP non ha lo stato di una sorgente (messaggio 46: uno stato per prodotto)
        riga_a1 = p.locator("#du-step-" + sid(a1))
        expect(riga_a1).to_be_visible(timeout=T_LUNGO)
        b.attendi(lambda: "incerta" in riga_a1.inner_text() or "sorgente" in riga_a1.inner_text(), "la riga dello STEP non dice né la candidata incerta né lo stato della sorgente")
        b.guarda(a1)
        expect(p.locator("#du-anteprima .du-vp")).to_be_visible(timeout=T_LUNGO)
        verifica(p.locator("#du-nessuna-in-vista").count() == 0, "una candidata incerta sola non è in vista all'apertura")
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
    b.cartella("tutti")
    gp = perdi_la_risposta()
    g6 = interrompi("GET")
    p.route("**/sorgente/file/*/altri-file", gp)
    p.route("**/motore-a/intento/*", g6)
    try:
        b.azione("altri", nd)
        expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
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
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza convertita.", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["bozza_id"] is None and c["versione_attesa"] == 0 and c["converti_da"]["versione_attesa"] == 3 and "motivo" not in c["converti_da"]
             and "snapshot" not in c and "azioni" not in c, "corpo della conversione: %s" % c)
    b.salvata(1)
    # mono_pezzo: il prodotto senza figli (avviso), senza motivo (messaggio 46 [U]), una decisione salvata e il riepilogo riletto
    b.vista("org")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-avvisi [data-codice=PRODOTTO_SENZA_FIGLI]")).to_be_visible(timeout=T_LUNGO)
    p.locator("#du-wz-corpo button", has_text="Dichiara mono_pezzo").click()
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    i = b.segna()
    b.ok_finestra()
    d = b.ultime_azioni(i)[0]["dopo"]
    verifica(d["tipo"] == "mono_pezzo" and "motivo" not in d, "decisione mono_pezzo: %s" % d)
    expect(p.locator("#du-wz-avvisi [data-codice=PRODOTTO_SENZA_FIGLI]")).to_have_count(0, timeout=T_LUNGO)
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-stato-conferma")).to_contain_text("mono_pezzo proposto in bozza")
    # la bozza convertita non ha uno STEP sorgente: il gate della direttiva 39 spegne la conferma, con il rimedio
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-conferma")).to_be_disabled(timeout=T_LUNGO)
    verifica("conferma uno STEP dalla Sorgente" in (p.locator("#du-wz-conferma").get_attribute("title") or ""), "la bozza convertita senza sorgente: la conferma non dice il gate")
    p.click("#du-wz-chiudi")
    # R5 «da verificare» e il wizard chiuso: la verifica con R6 va avanti da sola (MAT §5.5 n. 2). Il gate della sorgente
    # resta acceso, come nel server: la bozza riceve una sorgente S8 valida dai dati di prova (sonda sorgente-s8)
    b.sorgente_s8(b.p1)
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
    # UI46 (messaggi 46-47): senza bozza la Revisione mostra la BOM confermata (#du-working) e le partenze non ammesse non
    # compaiono più, nemmeno spente con la spiegazione lunga [T]
    expect(p.locator("#du-working")).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("#du-da-registrato").count() == 0, "«Parti dalla BOM registrata» offerta con una conferma formato 3")
    verifica("conferma del formato 3" not in p.locator("#du-working").inner_text(), "la spiegazione lunga della partenza spenta è ancora sulla pagina")
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
    # il secondo prodotto ha una sola conferma, formato 2 (della pagina vecchia). Nel banco non ha candidate: il gate della
    # sorgente resta acceso, come nel server, e le bozze ricevono una sorgente S8 valida dai dati di prova (sonda
    # sorgente-s8); il gate stesso lo provano DU-B62, DU-B63 e la prova sul server vero
    b52_corpo(b)


def b52_corpo(b):
    p = b.page
    b.sonda_post("conferma-formato2")
    tab_prodotto(b, b.p2).click()
    expect(tab_prodotto(b, b.p2)).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.passo("revisione")
    expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-da-registrato")).to_be_enabled()
    expect(p.locator("#du-da-conferma")).to_be_enabled()
    # «Modifica la BOM confermata» su una base formato 2: senza modifiche il CTA è acceso, la prima conferma formato 3
    # aggiunge identità e caselle
    p.click("#du-da-conferma")
    b.salvata(1)
    b.sorgente_s8(b.p2)
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
        expect(p.locator("#du-wz-avvisi [data-codice=PRODOTTO_SENZA_FIGLI]")).to_be_visible(timeout=T_LUNGO)
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
        expect(p.locator(SENZA_BOZZA)).to_be_visible(timeout=T_LUNGO)
    finally:
        p.remove_listener("pageerror", ascolta)
    # adesso il secondo prodotto ha una conferma formato 3: «Parti dalla BOM registrata» non si offre, e la bozza dalla
    # conferma senza modifiche ha il CTA spento
    expect(p.locator("#du-da-registrato")).to_have_count(0)
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
        b.sorgente_s8(b.p1)
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
        # il banco torna com'è per difetto: uguale_alla_base acceso, come il server dal blocco R4 della persistenza
        b.interruttore("uguale-alla-base", True)


@prova("53", "WZ-01 con lo storico (R7) in 5xx: la bozza dalla conferma e quella «registrato» senza gesti hanno il CTA spento con «↻ Aggiorna», niente dice «nessuna conferma»; R4 su una bozza «registrato» dopo un annullamento netto (RPC PD-01, punto 3)")
def b53(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    cta = p.locator("#du-cta-bom")
    n_r5 = b.stato()["conti"].get("R5", 0)
    r7 = re.compile(r"/motore-a/conferme(\?|$)")

    def cta_spento(motivo, cosa):
        expect(cta).to_be_disabled(timeout=T_LUNGO)
        verifica(motivo in (cta.get_attribute("title") or ""), "%s: CTA %r, atteso %r" % (cosa, cta.get_attribute("title"), motivo))

    def con_r7_in_errore(cosa):
        # lo storico in 5xx (risposta finta, dichiarata): la pagina non sa se la base è l'ultima conferma valida formato 3,
        # e nel dubbio il CTA resta spento come le due partenze (PE-B03). Prima la regola del giornale usciva senza motivo:
        # CTA e «Conferma BOM» accesi, e una conferma n+1 uguale alla n passava (rilievo 1 del secondo passaggio)
        g = rispondi_json(500, {"esito": "errore", "testo": "errore interno"}, metodo="GET", volte=100)
        p.route(r7, g)
        try:
            p.click("#du-aggiorna")
            expect(b.avviso()).to_contain_text("Lettura non riuscita (conferme)", timeout=T_LUNGO)
            b.salvata(1)
            cta_spento("lo storico delle conferme non si legge", cosa)
            expect(p.locator("#du-conferme-non-lette")).to_be_visible()
            verifica("nessuna conferma" not in p.locator("#du-stato-conferma").inner_text(), "%s: con R7 in errore la pagina dice «nessuna conferma»" % cosa)
        finally:
            p.unroute(r7, g)
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        cta_spento("nessuna modifica: la v3 è già confermata", cosa)

    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    p.click("#du-da-conferma")
    b.salvata(1)
    cta_spento("nessuna modifica: la v3 è già confermata", "la bozza dalla conferma con R7 letto")
    con_r7_in_errore("la bozza dalla conferma senza gesti")
    # una bozza «registrato» aperta altrove (sonda), senza gesti, sulla stessa base: lo stesso
    p.click("#du-abbandona")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza abbandonata.", timeout=T_LUNGO)
    b.sonda_post("bozza-registrato")
    con_r7_in_errore("la bozza «registrato» senza gesti")
    verifica(b.stato()["conti"].get("R5", 0) == n_r5, "una conferma registrata con lo storico non letto")
    # R4 sulla bozza «registrato» (il segno del server dal passo 6; nel banco con l'interruttore): «uguale» è lo snapshot
    # che la partenza della bozza costruirebbe dalla stessa base (RPC PD-01, punto 3, raccomandata a). Dopo un gesto R4
    # dice falso e il CTA si accende; dopo il suo annullamento R4 dice vero, e il CTA si rispegne
    b.interruttore("uguale-alla-base", True)
    try:
        b.scheda("k-root")
        b.linguetta("dettaglio")
        p.click("#du-correggi")
        p.fill("#f-desc", "Confezionatrice MX-1, prova di R4")
        b.ok_finestra()
        b.salvata(2)
        expect(p.locator("#du-conti")).to_contain_text("blocchi", timeout=T_LUNGO)
        expect(cta).to_be_enabled(timeout=T_LUNGO)
        p.click("#du-annulla")
        b.salvata(3)
        cta_spento("nessuna modifica: la v3 è già confermata", "la bozza «registrato» dopo l'annullamento netto")
    finally:
        # il banco torna com'è per difetto: uguale_alla_base acceso, come il server dal blocco R4 della persistenza
        b.interruttore("uguale-alla-base", True)


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
        a1 = b.al["ACM-1000_assieme.stp"]
        # la BOM è confermata su a1: la bozza importata dallo STEP con la working altrove (vedi working_altrove)
        p.route("**/motore-a/dati*", working_altrove)
        try:
            p.click("#du-aggiorna")
            expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
            b.guarda(a1)
            expect(p.locator("#du-conferma-sorgente")).to_have_text("Riprendi: crea la bozza di ACM-1000", timeout=T_LUNGO)
            p.click("#du-conferma-sorgente")
            b.ok_finestra()
            expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
        finally:
            p.unroute("**/motore-a/dati*", working_altrove)
        b.passo("revisione")
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
    # un effetto con altri nomi (i campi della struct Go EffettoTipo senza tag json: la risposta del banco riscritta qui,
    # dichiarata; RPC PD-05 aperto): il ponte non lo legge (PONTE_RISPOSTA), il dialogo lo dice e «Cambia» resta spento,
    # anche per il cambio che il server dice spento. Prima la pagina mostrava «nessun altro effetto» e accendeva «Cambia»
    # (rilievo 5 del secondo passaggio)
    nomi_go = {"spento": "Spento", "sospende": "Sospende", "con_lei": "ConLei", "rimozioni": "Rimozioni", "resta_sospesa": "RestaSospesa",
               "autorizzabile": "Autorizzabile", "step_strutturale": "StepStrutturale", "svuota_step": "SvuotaStep", "avvisi": "Avvisi"}

    def effetto_go(route):
        if route.request.method == "GET" and "effetto_cambio_tipo=" in route.request.url:
            resp = route.fetch()
            j = resp.json()
            j["effetto"] = {nomi_go.get(k, k): v for k, v in (j.get("effetto") or {}).items()}
            route.fulfill(response=resp, json=j)
        else:
            route.continue_()
    varianti = re.compile(r"/motore-a/bozza\?")
    p.route(varianti, effetto_go)
    try:
        n_r2 = b.stato()["conti"]["R2"]
        p.click("#du-cambia-tipo")
        for tipo in ("commerciale", "sciolto"):
            p.select_option("#f-tipo", tipo)
            expect(p.locator("#f-effetto-errore")).to_contain_text("non si legge", timeout=T_LUNGO)
            expect(p.locator("#f-effetto-righe")).to_have_count(0)
            expect(ok).to_be_disabled()
            verifica("non si legge" in (ok.get_attribute("title") or ""), "«Cambia» verso %s con l'effetto dai nomi della struct Go: %r" % (tipo, ok.get_attribute("title")))
        b.chiudi_finestra()
        verifica(b.stato()["conti"]["R2"] == n_r2, "un cambio di tipo con l'effetto illeggibile ha scritto la bozza")
    finally:
        p.unroute(varianti, effetto_go)


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
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = az[0]["dopo"]
    verifica(dec["tipo"] == "identita" and dec["modo"] == "alternativa" and dec["chiave_lettura"].endswith("/l:dxf")
             and dec["identita"] == {"emittente": "partner:beta", "namespace": "beta", "codice": "400123", "rev": "B"}
             and "qualificatori" not in dec["lettura"] and "revisione" not in dec["lettura"] and az[1]["dopo"]["identita"].get("rev") == "B", "«Adotta come codice» con PE-08: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # P-18 su un pezzo importato (rilievo 4 del secondo passaggio): il codice corretto a mano nella pagina vecchia ha la
    # rev minuscola (la risposta di R1 cambiata qui, dichiarata). «Adotta come codice» la copia byte per byte (ammesso:
    # manuale_legacy); «Correggi» con un codice nuovo la porterebbe in una decisione di correzione, dove il server non
    # ammette minuscole: «Salva nella bozza» resta spento con il motivo, e con la rev in maiuscolo si salva. La correzione
    # toglie nello stesso gesto la decisione identita di prima del nodo (PE-B16): il banco, come il server, rifiuta una
    # decisione identita che il suo nodo non cita
    def manuale_minuscolo(route):
        if route.request.method == "GET" and "importazione=1" in route.request.url:
            resp = route.fetch()
            j = resp.json()
            for n in (((j.get("importazione") or {}).get("contenuto") or {}).get("proposta") or {}).get("nodi", []):
                if isinstance(n.get("codice_manuale_legacy"), dict):
                    n["codice_manuale_legacy"]["rev"] = "a"
            route.fulfill(response=resp, json=j)
        else:
            route.continue_()
    p.route(letture_r1, manuale_minuscolo)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.scheda("k-11")
        b.linguetta("dettaglio")
        li = p.locator("#du-sez-riferimenti li", has_text="codice corretto a mano")
        expect(li).to_contain_text("ACM-1125")
        i = b.segna()
        li.locator("button", has_text="Adotta come codice").click()
        expect(b.finestra()).to_contain_text("ACM-1125 rev a")
        b.ok_finestra()
        az = b.ultime_azioni(i)
        adottata = az[0]["dopo"]
        verifica([a["azione"] for a in az] == ["decisione_aggiunta", "nodo_modificato"] and adottata.get("riferimento") == "manuale_legacy"
                 and adottata["identita"].get("rev") == "a" and az[1]["dopo"]["identita"].get("rev") == "a", "«Adotta come codice» del codice manuale con la rev minuscola: %s" % az)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        n_r2 = b.stato()["conti"]["R2"]
        p.click("#du-correggi")
        p.fill("#f-codice", "ACM-1126")
        expect(ok).to_be_disabled()
        verifica("minuscole" in (ok.get_attribute("title") or ""), "«Correggi» con un codice nuovo e la rev minuscola del codice manuale: %r" % ok.get_attribute("title"))
        verifica(b.stato()["conti"]["R2"] == n_r2, "«Correggi» con la rev minuscola ha scritto la bozza")
        p.fill("#f-rev", "A")
        expect(ok).to_be_enabled()
        i = b.segna()
        b.ok_finestra()
        az = b.ultime_azioni(i)
        verifica([a["azione"] for a in az] == ["decisione_tolta", "decisione_aggiunta", "nodo_modificato"] and az[0]["prima"]["id"] == adottata["id"]
                 and az[1]["dopo"]["modo"] == "correzione" and az[1]["dopo"]["identita"] == {"emittente": adottata["identita"]["emittente"], "codice": "ACM-1126", "rev": "A"}
                 and az[2]["dopo"]["identita"]["decisione"] == az[1]["dopo"]["id"], "«Correggi» dopo «Adotta come codice»: %s" % az)
        expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
        decise = [d for d in b.stato()["bozze"][b.p1]["snapshot"]["decisioni"] if d.get("tipo") == "identita" and d.get("chiave_bozza") == "k-11"]
        verifica(len(decise) == 1 and decise[0]["id"] == az[1]["dopo"]["id"], "decisioni identita di k-11 nella bozza salvata: %s" % decise)
    finally:
        p.unroute(letture_r1, manuale_minuscolo)
    # «Fissa l'emittente…» di un registrato (P-18): la rev minuscola della working passa byte per byte finché il nodo
    # resta riusa; con un nodo «modifica» (qui formato_2, la risposta di R1 cambiata, dichiarata) la decisione di
    # correzione la farebbe nuova, e «Fissa» resta spento con il motivo
    b.scheda("k-40")
    b.linguetta("dettaglio")
    expect(p.locator("#nc-k-40")).to_contain_text("rev a")
    p.click("#du-fissa-emittente")
    expect(ok).to_be_enabled()
    b.chiudi_finestra()

    def modifica_formato2(route):
        if route.request.method == "GET" and "eventi=1" in route.request.url:
            resp = route.fetch()
            j = resp.json()
            for n in j.get("snapshot", {}).get("nodi", []):
                if n.get("chiave_bozza") == "k-40":
                    n["op"] = "modifica"
                    n["identita"]["modo"] = "formato_2"
                    n["identita"]["rev"] = "a"
            route.fulfill(response=resp, json=j)
        else:
            route.continue_()
    p.route(letture_r1, modifica_formato2)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.scheda("k-40")
        b.linguetta("dettaglio")
        p.click("#du-fissa-emittente")
        expect(ok).to_be_disabled()
        verifica("minuscole" in (ok.get_attribute("title") or ""), "«Fissa l'emittente» di un nodo modifica con la rev minuscola: %r" % ok.get_attribute("title"))
        b.chiudi_finestra()
    finally:
        p.unroute(letture_r1, modifica_formato2)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)


@prova("58", "RV3-25 con il wizard aperto (U2 §6.3; CN-06, CN-08): N1 documento in analisi con la presa d'atto, N2 proposta cambiata e il riepilogo nuovo, N4 bozza salvata altrove, N5 analisi senza cambi")
def b58(b):
    p = b.page
    b.interruttore("firma-con-proposta", True)
    try:
        b.passo("revisione")
        expect(p.locator("#du-cta-bom")).to_be_enabled(timeout=T_LUNGO)
        b.apri_wizard()
        cta = p.locator("#du-wz-conferma")
        i0 = b.segna()
        # N1: un documento nuovo entra in analisi (un altro utente lo carica): la notifica e la presa d'atto, mai spuntata
        r = p.request.post(b.url + "/__banco/documento-nuovo", data=json.dumps({"nome": "ALLEGATO_TECNICO.pdf"}), headers={"Content-Type": "application/json"})
        verifica(r.status == 200, "la sonda non ha aggiunto il documento: %d" % r.status)
        n1 = p.locator("#du-wz-n1")
        expect(n1).to_contain_text("È arrivato un documento nuovo (ALLEGATO_TECNICO.pdf): è in analisi e il riepilogo non lo considera ancora.", timeout=25000)
        expect(p.locator("#du-wz-in-analisi")).to_contain_text("Confermo senza i documenti ancora in analisi: ALLEGATO_TECNICO.pdf")
        verifica(not p.locator("#du-wz-presa-analisi").is_checked(), "la presa d'atto è già spuntata")
        if cta.is_enabled():
            raise Rotto("«Conferma BOM» acceso senza la presa d'atto dei documenti in analisi")
        verifica("presa d'atto" in (cta.get_attribute("title") or ""), "«Conferma BOM» spento senza la ragione: %s" % cta.get_attribute("title"))
        ing = [x for x in b.richieste_da(i0, "GET", r"/motore-a/bozza$") if "ingressi=1" in x["query"]]
        verifica(ing, "nessuna lettura di R1 ?ingressi=1 con il wizard aperto")
        # N2: l'analisi finisce e cambia la proposta: un R4 nuovo, firma diversa, «Conferma BOM» spento fino al riepilogo nuovo
        r = p.request.post(b.url + "/__banco/analisi-conclusa", data=json.dumps({"nome": "ALLEGATO_TECNICO.pdf", "cambia": True}), headers={"Content-Type": "application/json"})
        verifica(r.status == 204, "la sonda non ha concluso l'analisi: %d" % r.status)
        n2 = p.locator("#du-wz-n2")
        expect(n2).to_contain_text("Attenzione: il riepilogo non è più aggiornato. Un nuovo documento ha modificato la proposta del motore.", timeout=50000)
        verifica("BOM confermata" not in n2.inner_text() and "BOM registrata" not in n2.inner_text(), "N2 parla della BOM registrata")
        expect(cta).to_be_disabled()
        verifica("riepilogo nuovo" in (cta.get_attribute("title") or ""), "«Conferma BOM» spento senza la ragione di N2")
        p.click("#du-wz-n2-apri")
        expect(n2).to_have_count(0)
        expect(p.locator("#du-wz-in-analisi")).to_have_count(0)
        # N4: un collega salva la bozza
        b.sonda_post("bozza-altrove")
        n4 = p.locator("#du-wz-n4")
        # aggiornata_da è un utente_id (come nel server): la pagina non mostra mai l'identificativo
        expect(n4).to_contain_text("è stata salvata da un altro utente", timeout=25000)
        verifica(not re.search(r"[0-9a-f]{8}-[0-9a-f]{4}-", n4.inner_text()), "N4 mostra un identificativo: %s" % n4.inner_text())
        expect(n4).to_contain_text("Rileggi la bozza.")
        expect(cta).to_be_disabled()
        p.click("#du-wz-n4-rileggi")
        expect(n4).to_have_count(0, timeout=T_LUNGO)
        expect(p.locator("#du-wz-identita")).to_be_visible(timeout=T_LUNGO)
        # N5: un'analisi che non cambia la proposta: una riga quieta, nessun conflitto
        r = p.request.post(b.url + "/__banco/documento-nuovo", data=json.dumps({"nome": "NOTA_TECNICA.pdf"}), headers={"Content-Type": "application/json"})
        expect(p.locator("#du-wz-n1")).to_contain_text("NOTA_TECNICA.pdf", timeout=25000)
        p.request.post(b.url + "/__banco/analisi-conclusa", data=json.dumps({"nome": "NOTA_TECNICA.pdf", "cambia": False}), headers={"Content-Type": "application/json"})
        expect(p.locator("#du-wz-n5")).to_contain_text("Il documento NOTA_TECNICA.pdf è stato analizzato: la proposta non cambia.", timeout=60000)
        verifica(p.locator("#du-wz-n2").count() == 0, "N2 per un'analisi che non cambia la proposta")
        b.foto("B58_wizard_notifiche.png")
        p.click("#du-wz-chiudi")
        expect(p.locator("#du-wizard")).to_be_hidden()
    finally:
        b.interruttore("firma-con-proposta", False)


@prova("59", "RV3-34 (U2 §6.4; CAT-01, CAT-02): «Conferma la categoria…» dall'editor del nodo, «Conferma il tipo proposto», il gruppo «Categorie da confermare» nel wizard, «Non è minuteria…»")
def b59(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    b.scheda("k-12")
    sez = p.locator("#du-sez-categoria")
    expect(sez).to_contain_text("minuteria proposta, normativa 2.1051", timeout=T_LUNGO)
    expect(sez).to_contain_text("da sola non esenta dal 2D")
    i = b.segna()
    p.click("#du-conferma-categoria")
    expect(b.finestra()).to_contain_text("Solo la minuteria confermata da una persona non chiede il 2D")
    verifica(p.locator("#f-origine").input_value() == "classe_grammatica", "origine della categoria: %s" % p.locator("#f-origine").input_value())
    b.ok_finestra()
    az = b.ultime_azioni(i)
    dec = [a for a in az if a["azione"] == "decisione_aggiunta"]
    verifica(len(dec) == 1 and dec[0]["chiave_bozza"] == "k-12", "«Conferma la categoria…»: %s" % az)
    d = dec[0]["dopo"]
    verifica(d["tipo"] == "conferma_categoria" and d["soggetto"] == {"chiave_bozza": "k-12"} and d["categoria"] == "minuteria" and d["esito"] == "conferma"
             and d["origine_categoria"] == "classe_grammatica" and d["ingresso"] == "editor" and d["classe_proposta"]["valore"] == "2.1051" and len(d.get("importazione", "")) == 64,
             "la decisione conferma_categoria: %s" % d)
    verifica(set(d) <= {"id", "tipo", "soggetto", "categoria", "esito", "origine_categoria", "classe_proposta", "importazione", "ingresso", "motivo", "da_conferma"}, "campi fuori dalla forma: %s" % sorted(d))
    expect(p.locator("#du-cat-effetto")).to_contain_text("in bozza: il 2D non sarà richiesto dopo la conferma", timeout=T_LUNGO)
    b.salvata(b.stato()["bozze"][b.p1]["versione"])
    # «Conferma il tipo proposto»: un nodo_modificato con tipo_confermato, nessun effetto sul 2D
    i = b.segna()
    p.click("#du-conferma-tipo")
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["chiave_bozza"], a["dopo"].get("tipo_confermato")) for a in az] == [("nodo_modificato", "k-12", True)], "«Conferma il tipo proposto»: %s" % az)
    expect(p.locator("#du-sez-categoria")).to_contain_text("tipo confermato", timeout=T_LUNGO)
    b.salvata(b.stato()["bozze"][b.p1]["versione"])
    # il wizard: la decisione dell'editor c'è già, senza una seconda accettazione; le altre proposte non sono spuntate
    b.apri_wizard()
    p.click("#du-wz-g-categorie")
    tab = p.locator("#du-wz-categorie")
    expect(tab.locator('tr[data-k="k-12"]')).to_contain_text("minuteria confermata", timeout=T_LUNGO)
    expect(tab.locator('tr[data-k="k-12"]')).to_contain_text("dall'editor")
    verifica(tab.locator('tr[data-k="k-12"] input').count() == 0, "la decisione dell'editor chiede una seconda accettazione")
    if tab.locator('tr[data-k="k-9"]').count():
        verifica(not tab.locator('tr[data-k="k-9"] input').is_checked(), "una proposta di categoria già spuntata")
        expect(p.locator("#du-wz-cat-conferma")).to_be_disabled()
        p.click("#du-wz-cat-tutte")
        i = b.segna()
        p.click("#du-wz-cat-conferma")
        expect(tab.locator('tr[data-k="k-9"]')).to_contain_text("commerciale confermata", timeout=T_LUNGO)
        corpi = b.corpi(i, "POST", r"/motore-a/bozza$")
        dw = [a["dopo"] for c in corpi for a in c["azioni"] if a["azione"] == "decisione_aggiunta"]
        verifica(len(dw) == 1 and dw[0]["ingresso"] == "wizard" and dw[0]["categoria"] == "commerciale" and dw[0]["soggetto"] == {"chiave_bozza": "k-9"}, "«Conferma le categorie (N)»: %s" % dw)
        expect(tab.locator('tr[data-k="k-9"]')).to_contain_text("2D richiesto")
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()
    # «Non è minuteria…»: senza motivo (messaggio 46 [U]), la decisione di prima si sostituisce nello stesso gesto
    b.scheda("k-12")
    p.click("#du-non-minuteria")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled()
    i = b.segna()
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["decisione_tolta", "decisione_aggiunta"] and az[1]["dopo"]["esito"] == "rifiuto" and "motivo" not in az[1]["dopo"], "«Non è minuteria…»: %s" % az)
    expect(p.locator("#du-sez-categoria")).to_contain_text("non è minuteria", timeout=T_LUNGO)
    b.salvata(b.stato()["bozze"][b.p1]["versione"])


@prova("50","E1′ lento che avanza: oltre i 120 s delle chiamate JSON, l'avviso dice l'avanzamento, il file entra (tempo d'inattività, §5 n. 3)")
def b50(b):
    p = b.page
    b.passo("sorgente")
    b.cartella("tutti")
    cdp = p.context.new_cdp_session(p)
    cdp.send("Network.enable")
    # circa 64 KiB/s in salita: 8,2 MiB vogliono circa 131 s, più del tempo massimo delle chiamate JSON (120 s)
    cdp.send("Network.emulateNetworkConditions", {"offline": False, "latency": 0, "downloadThroughput": -1, "uploadThroughput": 65536})
    dati = b"%PDF-1.4\n% sintetico, caricamento lento\n" + b"0" * (8 * 1024 * 1024 + 200 * 1024)
    av = b.avviso()
    visti = []
    durata = 0.0
    try:
        b.apri_carica()
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


@prova("51", "E1′ fermo: nessun byte inviato né ricevuto per il tempo d'inattività (120 s), il caricamento si ferma e lo dice (§5 n. 3); il banco finisce la richiesta prima della prova seguente")
def b51(b):
    p = b.page
    b.passo("sorgente")
    b.cartella("tutti")
    # il banco aspetta 130 s prima di leggere la richiesta, poi la lascia cadere: dopo l'invio non si muove niente. Il
    # tempo d'inattività è di 120 s, come il tempo massimo che sostituisce (secondo passaggio: un server che risponde fra
    # 60 e 120 s dopo il corpo deve arrivare in fondo come prima); i 10 s in più tengono la risposta del banco dopo
    b.ritardo("POST /thread/{id}/carica", 130000)
    b.modo("E1", "appeso")
    b.apri_carica()
    p.set_input_files("#du-carica-file", files=[{"name": "DISEGNO_FERMO.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% fermo\n"}])
    t0 = time.time()
    p.click("#du-carica-invia")
    av = b.avviso()
    expect(av).to_contain_text("Il caricamento si è fermato", timeout=150000)
    durata = time.time() - t0
    verifica(115 <= durata <= 129, "il caricamento fermo si è fermato dopo %.0f s, atteso circa il tempo d'inattività (120 s)" % durata)
    expect(av).to_contain_text("nessun byte inviato né ricevuto per 120 s")
    expect(av).to_contain_text("rimanda gli stessi file")
    expect(p.locator("#du-carica-invia")).to_be_enabled()
    # il banco finisce la richiesta lasciata appesa (consuma il modo «appeso») prima della prova seguente: una E1 di dopo
    # non deve prendere il 503 destinato a questa, e il server di prova non resta occupato alla chiusura
    b.attendi(lambda: "E1" not in b.stato().get("modi", {}), "il banco non ha finito la richiesta del caricamento fermo", ms=30000)
    verifica("DISEGNO_FERMO.pdf" not in b.stato()["file"], "il caricamento fermo ha registrato il file")


GUSCIO = [("GET", r"/thread/{T}/distinta-unica"), ("GET", r"/static/[A-Za-z0-9_.\-/]+"), ("GET", r"/stato/worker"), ("GET", r"/favicon\.ico")]
DEDICATE = [
    ("GET", r"/thread/{T}/motore-a/dati"),
    ("GET", r"/thread/{T}/motore-a/bozza"), ("POST", r"/thread/{T}/motore-a/bozza"), ("POST", r"/thread/{T}/motore-a/bozza/{U}/abbandona"),
    ("POST", r"/thread/{T}/motore-a/bom/riepilogo"), ("POST", r"/thread/{T}/motore-a/bom/conferma"), ("GET", r"/thread/{T}/motore-a/intento/{U}"),
    ("GET", r"/thread/{T}/motore-a/conferme"), ("POST", r"/thread/{T}/motore-a/riallinea"), ("POST", r"/thread/{T}/carica"),
    ("GET", r"/thread/{T}/sorgente"), ("POST", r"/thread/{T}/sorgente/file/{U}/scarta"), ("POST", r"/thread/{T}/sorgente/file/{U}/altri-file"),
    ("POST", r"/thread/{T}/sorgente/file/{U}/riattiva"), ("GET", r"/thread/{T}/sorgente/file/{U}/contenuto"),
    ("GET", r"/thread/{T}/sorgente/step/{U}/anteprima"),
    # dall'ondata C (U2): S8 e R15 (S9 e R1 ?ingressi=1 sono varianti di lettura di /dati e di R1)
    ("POST", r"/thread/{T}/sorgente/step/{U}/conferma-sorgente"), ("POST", r"/thread/{T}/motore-a/proposta-aggiornata/rifiuta"),
    # direttiva 39 §2: la revoca della sorgente della bozza
    ("POST", r"/thread/{T}/sorgente/step/{U}/revoca-sorgente"),
]
# direttiva 39 §2 (D-TAB-02 B rivista [T-39]): «Autorizza» e «Revoca l'autorizzazione» non sono più atti della pagina
ATTI = [
    ("POST", r"/thread/{T}/fascicolo/prepara"), ("POST", r"/thread/{T}/fascicolo/rianalizza"),
    ("POST", r"/thread/{T}/riprova-copie"),
]


@prova("34", "richieste della pagina: guscio, statici, rotte del ponte, tre atti (niente «Conferma la fonte», niente «Autorizza»: direttiva 39); nessun legacy")
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


TENTATIVO_DI_PROVA = {"nome": "confermaBom", "chiave": "00000000-0000-4000-8000-0000000e0f02", "impronta": "x", "verificato": True,
                      "quando": "2026-10-18T08:00:00Z", "etichetta": "Conferma BOM"}


def chiavi_du(p):
    return sorted(k for k in p.evaluate("() => Object.keys(sessionStorage)") if k.startswith("cockpit.du."))


@prova("57", "l'uscita annullata (R-3, MAT §5.5 n. 7): con le modifiche tenute da parte «esci» chiede conferma, si resta, le chiavi cockpit.du. restano; la pagina d'accesso aperta dopo non le toglie")
def b57(b):
    p = b.page
    b.passo("revisione")
    b.vista("org")
    conflitto_e_rileggi(b, "Confezionatrice MX-1, prova dell'uscita")
    # un tentativo in sospeso della Distinta unica, come lo scrive il ponte (la guardia PONTE_TENTATIVO_IN_SOSPESO)
    chiave = "cockpit.du.sospeso.%s.%s.bozza:%s" % (b.dati["utente"], b.thread, b.p1)
    p.evaluate("([k, v]) => sessionStorage.setItem(k, JSON.stringify(v))", [chiave, dict(TENTATIVO_DI_PROVA, risorsa="bozza:" + b.p1)])
    prima = chiavi_du(p)
    verifica(chiave in prima, "la chiave di prova non è nella memoria della scheda: %s" % prima)
    pagina = p.url
    n_bu = b.beforeunload
    b.consenti_beforeunload = True
    try:
        # «esci», poi «resta» nel dialogo del browser: prima del secondo passaggio le chiavi si toglievano già al clic, e la
        # pagina restava senza i suoi tentativi in sospeso
        p.click("#esci button", no_wait_after=True)
        b.attendi(lambda: b.beforeunload > n_bu, "con le modifiche tenute da parte «esci» non chiede conferma")
        p.wait_for_timeout(500)
    finally:
        b.consenti_beforeunload = False
    verifica(p.url == pagina, "l'uscita annullata ha lasciato la pagina: %s" % p.url)
    expect(p.locator("#du-da-parte")).to_be_visible()
    verifica(chiavi_du(p) == prima, "dopo l'uscita annullata le chiavi cockpit.du. sono cambiate: %s, prima %s" % (chiavi_du(p), prima))
    # si resta e si lavora: il primo gesto sulla pagina toglie la marca dell'uscita, e la pagina d'accesso aperta dopo
    # nella stessa scheda (per esempio dopo una sessione scaduta) non toglie le chiavi
    verifica(p.evaluate("() => sessionStorage.getItem('cockpit.uscita')") is not None, "il clic su «esci» non ha lasciato la marca dell'uscita")
    p.keyboard.press("Shift")
    verifica(p.evaluate("() => sessionStorage.getItem('cockpit.uscita')") is None, "dopo un gesto sulla pagina la marca dell'uscita resta")
    p.click("#du-scarta-da-parte")
    b.ok_finestra()
    expect(p.locator("#du-da-parte")).to_have_count(0)
    p.goto(b.url + "/login")
    expect(p.locator("form[action='/login']")).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("#esci").count() == 0, "la pagina d'accesso del banco ha il form «esci»")
    verifica(chiavi_du(p) == prima, "la pagina d'accesso aperta dopo l'uscita annullata ha tolto le chiavi: %s" % chiavi_du(p))
    # pulizia: la chiave di prova si toglie, e si torna alla pagina per la prova seguente
    p.evaluate("([k]) => sessionStorage.removeItem(k)", [chiave])
    b.apri()


@prova("49", "all'uscita («esci» del layout) le chiavi cockpit.du. si tolgono sulla pagina d'accesso, quelle dell'editor subito, le altre restano (R-3)")
def b49(b):
    p = b.page
    # un tentativo in sospeso della Distinta unica (come lo scrive il ponte), una bozza dell'editor e una chiave che non è
    # della pagina
    chiave = "cockpit.du.sospeso.%s.%s.bozza:%s" % (b.dati["utente"], b.thread, b.p1)
    p.evaluate("([k, v]) => { sessionStorage.setItem(k, JSON.stringify(v)); sessionStorage.setItem('cockpit.editor.prova', '{}'); sessionStorage.setItem('prova.altra.chiave', '1'); }",
               [chiave, dict(TENTATIVO_DI_PROVA, risorsa="bozza:" + b.p1)])
    verifica(chiavi_du(p), "la chiave di prova non è nella memoria della scheda")
    with p.expect_navigation():
        p.click("#esci button")
    b.attendi(lambda: urllib.parse.urlsplit(p.url).path == "/login", "l'uscita non porta al login: %s" % p.url)
    # le chiavi si tolgono sulla pagina d'accesso (lo script del layout, senza il form «esci»), a uscita avvenuta
    expect(p.locator("form[action='/login']")).to_be_visible(timeout=T_LUNGO)
    b.attendi(lambda: not chiavi_du(p), "dopo l'uscita restano chiavi della Distinta unica: %s" % chiavi_du(p))
    chiavi = p.evaluate("() => Object.keys(sessionStorage)")
    verifica("prova.altra.chiave" in chiavi, "l'uscita ha tolto anche una chiave che non è della pagina: %s" % chiavi)
    verifica(not [k for k in chiavi if k.startswith("cockpit.editor.")], "dopo l'uscita restano bozze dell'editor: %s" % chiavi)
    verifica("cockpit.uscita" not in chiavi, "dopo l'uscita resta la marca: %s" % chiavi)


# ---- messaggio 46 dell'utente (21/10), corsia SORGENTE: le prove dei difetti visti a schermo (l'elenco D01…D47 dei
# fotogrammi del video dell'utente), ognuna su un banco suo (distinta_unica_ui46_browser_test.go). Ogni controllo porta il
# codice del difetto e non ferma la prova: alla fine la prova fallisce con l'elenco dei difetti ancora visibili, così su
# 342010f si vede che fallisce ogni controllo, uno per uno. Le foto UI46_B7n_*.png si fanno PRIMA dei controlli: su
# 342010f mostrano il difetto, dopo la correzione lo stato giusto.

class Difetti:
    def __init__(self, b):
        self.b = b
        self.ko = []
        self.visti = []

    def controlla(self, codice, fn, messaggio=""):
        """fn: un expect (che solleva) o una funzione che torna False se il difetto si vede."""
        self.visti.append(codice)
        try:
            if fn() is False:
                raise Rotto(messaggio or "falso")
        except Exception as e:
            riga = (messaggio or (str(e).splitlines()[0] if str(e) else repr(e))).replace("\n", " / ")
            self.ko.append((codice, riga[:200]))

    def fine(self):
        if self.ko:
            codici = sorted({c for c, _ in self.ko})
            raise Rotto("difetti ancora visibili: %s (%d di %d controlli)\n%s" % (", ".join(codici), len(self.ko), len(self.visti), "\n".join("%s (%s)" % x for x in self.ko)))


def ui46_conferma_nel_dialogo(b):
    """Il dialogo di S8 aperto: il motivo, se il dialogo lo chiede ancora (la corsia MOTIVO lo toglie), poi la conferma."""
    p = b.page
    expect(b.finestra()).to_contain_text("come sorgente di ACM-1000", timeout=T_LUNGO)
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova")
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)


def ui46_s8_dalla_proposta(b, a1):
    b.file(a1, "step")
    b.page.click("#du-sg-dettaglio #du-conferma-proposto-d")
    ui46_conferma_nel_dialogo(b)


def ui46_partenza(b, incerta=False):
    p = b.page
    b.interruttore("step-proposto", True)
    if incerta:
        b.interruttore("s6-incerta", True)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("step")


@prova("71", "messaggio 46 [U] punto 3: l'anteprima dello STEP con la candidata incerta in vista da sola (D08, D31, D32, D33) e la striscia che non resta dopo la chiusura (D24); il dialogo di conferma con la struttura e i codici, senza identificativi interni (D09, D17), la × accanto al titolo (D34), l'attesa nel dialogo (D26); l'esito una volta sola (D23)")
def b71(b):
    p = b.page
    d = Difetti(b)
    a1 = b.al["ACM-1000_assieme.stp"]
    ui46_partenza(b, incerta=True)
    b.file(a1, "step")
    p.click("#du-sg-dettaglio #du-guarda-proposto-d")
    expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(700)
    b.foto("UI46_B71_anteprima.png")
    ant = p.locator("#du-anteprima")
    # D08: la struttura in vista senza un altro clic; D32: il gesto della conferma dall'anteprima
    d.controlla("D08", lambda: expect(p.locator("#du-anteprima .du-vp")).to_be_visible(), "l'anteprima della candidata incerta non mostra la struttura")
    d.controlla("D08", lambda: p.locator("#du-nessuna-in-vista").count() == 0, "l'anteprima dice «Nessuna struttura in vista»")
    d.controlla("D32", lambda: expect(p.locator("#du-anteprima #du-conferma-sorgente")).to_be_visible(), "dall'anteprima non si conferma")
    # D31: il codice letto dalla radice si dice, mai «non determinabile» con la struttura in vista
    d.controlla("D31", lambda: "non determinabile" not in ant.inner_text(), "l'anteprima dice «Radice letta: non determinabile»")
    # D33: «Riduci» senza il carattere che si vede come una parentesi
    d.controlla("D33", lambda: "⟩" not in p.locator("#du-sg-riduci").inner_text(), "il pulsante è «%s»" % p.locator("#du-sg-riduci").inner_text())
    # D24: chiusa l'anteprima, la striscia «Proposta dello STEP in vista» non resta
    p.click("#du-anteprima-chiudi")
    p.wait_for_timeout(300)
    d.controlla("D24", lambda: not b.avviso().is_visible() or "in vista" not in b.avviso().inner_text(), "dopo la chiusura la striscia dice «%s»" % b.avviso().inner_text())
    # D09, D17, D34: il dialogo dice che cosa si conferma
    b.ritardo("POST /thread/{id}/sorgente/step/{a}/conferma-sorgente", 1500)
    b.file(a1, "step")
    p.click("#du-sg-dettaglio #du-conferma-proposto-d")
    fin = b.finestra()
    expect(fin).to_contain_text("Conferma ACM-1000_assieme.stp come sorgente di ACM-1000", timeout=T_LUNGO)
    p.wait_for_timeout(400)
    b.foto("UI46_B71_dialogo.png")
    d.controlla("D17", lambda: "cand:" not in fin.inner_text() and "nodo:" not in fin.inner_text(), "il dialogo mostra l'identificativo interno della struttura")
    d.controlla("D09", lambda: expect(p.locator("#f-struttura")).to_have_attribute("data-nodi", "10"), "il dialogo non mostra la struttura che si conferma")
    d.controlla("D09", lambda: expect(p.locator("#f-codici")).to_contain_text("codice letto dalla radice dello STEP"), "il dialogo non dice il codice letto e quello del prodotto")

    def x_accanto():
        t = p.locator("#du-finestra-titolo").bounding_box()
        x = p.locator("#du-finestra .du-fin-t > button").bounding_box()
        return x["y"] < t["y"] + t["height"] and x["x"] >= t["x"] + t["width"] - 2
    # D34: con un titolo che va a capo (lo schermo stretto: nel video era il nome lungo del file) la × resta in alto a destra
    p.set_viewport_size({"width": 420, "height": 900})
    p.wait_for_timeout(300)
    b.foto("UI46_B71_dialogo_stretto.png")
    d.controlla("D34", x_accanto, "la × del dialogo non sta accanto al titolo")
    p.set_viewport_size({"width": 1440, "height": 900})
    p.wait_for_timeout(300)
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova")
    b.ok_finestra()
    # D26: l'attesa si vede nel dialogo
    d.controlla("D26", lambda: expect(p.locator("#f-in-corso")).to_be_visible(timeout=1200), "durante la conferma il dialogo non dice che è in corso")
    b.foto("UI46_B71_attesa.png")
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    p.wait_for_timeout(300)
    b.foto("UI46_B71_esito.png")
    # D23: l'esito non si ripete tre volte (striscia, riquadro, ultima mossa)
    d.controlla("D23", lambda: not b.avviso().is_visible(), "la striscia ripete l'esito: %s" % b.avviso().inner_text())
    d.controlla("D23", lambda: "pronta da rivedere" not in esito.inner_text(), "il riquadro ripete l'esito: %s" % esito.inner_text())
    expect(p.locator("#du-sg-mossa")).to_contain_text("Sorgente confermata")
    d.fine()


@prova("72", "messaggio 46 [U] punto 4: uno stato solo per lo STEP sorgente dopo la conferma e dopo la revoca, mai «confermato» con «da confermare» o «proposto» (D10, D11, D27, D28, D47), il riquadro dello stato al posto della domanda, la revoca senza gergo (D29, D46)")
def b72(b):
    p = b.page
    d = Difetti(b)
    a1 = b.al["ACM-1000_assieme.stp"]
    ui46_partenza(b)
    # D47: prima della conferma, la riga dice la candidata in breve (la diagnostica del motore sta nel titolo)
    d.controlla("D47", lambda: "segnalati" not in p.locator("#du-step-" + sid(a1)).inner_text(), "la riga della candidata ha la diagnostica del motore")
    # D28: prima della conferma, nel dettaglio dello STEP la conferma e l'anteprima per ACM-1000 una volta sola (la proposta)
    b.file(a1, "step")
    det0 = p.locator("#du-sg-dettaglio")
    d.controlla("D28", lambda: det0.get_by_role("button", name="Conferma questo STEP come sorgente di ACM-1000").count() <= 1 and det0.get_by_role("button", name="Guarda l'anteprima per ACM-1000").count() <= 1,
                "la conferma o l'anteprima per ACM-1000 ripetute nel dettaglio")
    ui46_s8_dalla_proposta(b, a1)
    b.file(a1, "step")
    riga = p.locator("#du-step-" + sid(a1))
    det = p.locator("#du-sg-dettaglio")
    p.wait_for_timeout(300)
    b.foto("UI46_B72_confermata.png")
    d.controlla("D11", lambda: expect(riga).to_contain_text("sorgente di ACM-1000"), "la riga della sorgente non dice «sorgente di ACM-1000»")
    d.controlla("D11", lambda: "✓ confermato" not in riga.inner_text() and "candidata" not in riga.inner_text(), "la riga della sorgente dice più stati: %s" % riga.inner_text())
    d.controlla("D27", lambda: "Il Motore A non propone" not in det.inner_text(), "accanto allo STEP sorgente «Il Motore A non propone questo STEP per nessun prodotto»")
    d.controlla("D11", lambda: "lo confermi come sorgente" not in det.inner_text() and "✓ confermato" not in det.inner_text(), "il dettaglio della sorgente dice più stati")
    d.controlla("D11", lambda: expect(det.locator("#du-stato-sorgente-" + sid(b.p1))).to_have_attribute("data-stato", "confermata"), "il dettaglio non ha il riquadro dello stato")
    d.controlla("D28", lambda: det.get_by_role("button", name="Guarda l'anteprima per ACM-1000").count() <= 1, "«Guarda l'anteprima» ripetuto nel dettaglio")
    d.controlla("D27", lambda: det.get_by_role("button", name="Guarda l'anteprima per ACM-2000").count() == 0 or not det.get_by_role("button", name="Guarda l'anteprima per ACM-2000").first.is_visible(),
                "l'anteprima per ogni altro prodotto è in vista nel dettaglio")
    d.controlla("D29", lambda: "working" not in det.inner_text() and "(S8)" not in det.inner_text(), "il dettaglio ha il gergo («working», «(S8)»)")
    # la revoca
    p.click("#du-sg-dettaglio #du-revoca-sorgente-" + sid(a1))
    expect(b.finestra()).to_contain_text("Revoca", timeout=T_LUNGO)
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Sorgente revocata per ACM-1000", timeout=T_LUNGO)
    av = b.avviso().inner_text()
    d.controlla("D46", lambda: "(S8)" not in av and "working" not in av, "la striscia della revoca ha il gergo: %s" % av)
    b.file(a1, "step")
    p.wait_for_timeout(300)
    b.foto("UI46_B72_revocata.png")
    d.controlla("D10", lambda: expect(riga).to_contain_text("sorgente revocata per ACM-1000"), "la riga non dice la revoca")
    d.controlla("D10", lambda: "✓ confermato" not in riga.inner_text(), "la riga dopo la revoca dice «✓ confermato»: %s" % riga.inner_text())
    d.controlla("D10", lambda: "lo confermi come sorgente" not in det.inner_text() and "✓ confermato" not in det.inner_text(), "il dettaglio dopo la revoca dice confermato e da confermare")
    d.controlla("D10", lambda: expect(det.locator("#du-stato-sorgente-" + sid(b.p1) + " #du-riconferma-" + sid(b.p1))).to_contain_text("Conferma di nuovo come sorgente di ACM-1000"),
                "il dettaglio dopo la revoca non offre «Conferma di nuovo come sorgente»")
    # la cartella del prodotto: niente «Sorgente confermata» di prima accanto alla revoca, il riquadro dello stato
    b.cartella("p:" + b.p1)
    if p.locator("#du-stato-sorgente-sg").count() == 0 and p.locator("#du-step-proposto-sg").count() == 0 and p.locator("#du-sg-prodotto-apri").count():
        p.click("#du-sg-prodotto-apri")
    p.wait_for_timeout(300)
    b.foto("UI46_B72_prodotto.png")
    d.controlla("D10", lambda: p.locator("#du-sg-esito-s8").count() == 0, "la cartella del prodotto mostra ancora l'esito «Sorgente confermata»")
    d.controlla("D10", lambda: p.locator("#du-step-proposto-sg").count() == 0, "la cartella del prodotto chiede ancora «lo confermi?» dopo la revoca")
    d.fine()


@prova("73", "messaggio 46: la barra dei gesti sulle selezioni senza il muro di «non vale per …» (D18), con la conferma come gesto principale dello STEP candidato (D19), le spunte tolte dopo la conferma (D20)")
def b73(b):
    p = b.page
    d = Difetti(b)
    a1 = b.al["ACM-1000_assieme.stp"]
    ui46_partenza(b)
    p.check("#du-step-" + sid(a1) + " .du-sg-spunta")
    barra = p.locator("#du-sg-elenco #du-sg-gruppo")
    expect(barra).to_be_visible(timeout=T_BREVE)
    b.foto("UI46_B73_candidata.png")
    d.controlla("D18", lambda: "non vale per" not in barra.inner_text() and "non ammesso in questo stato" not in barra.inner_text(), "la barra elenca i gesti che non valgono")
    d.controlla("D18", lambda: barra.locator("button[disabled]").count() == 0, "la barra ha pulsanti spenti")
    d.controlla("D19", lambda: expect(barra.locator("#du-g-principale")).to_contain_text("Conferma questo STEP come sorgente di ACM-1000"), "la conferma non è nella barra")
    if barra.locator("#du-g-principale").count():
        barra.locator("#du-g-principale").click()
    else:
        b.file(a1, "step")
        p.click("#du-sg-dettaglio #du-conferma-proposto-d")
    ui46_conferma_nel_dialogo(b)
    b.cartella("step")
    d.controlla("D20", lambda: p.locator("#du-sg-elenco .du-sg-spunta:checked").count() == 0, "le spunte restano dopo la conferma")
    # la sorgente spuntata: i gesti che non valgono stanno ripiegati, con il perché in breve
    if p.locator("#du-step-" + sid(a1) + " .du-sg-spunta:checked").count() == 0:
        p.check("#du-step-" + sid(a1) + " .du-sg-spunta")
    expect(barra).to_be_visible(timeout=T_BREVE)
    b.foto("UI46_B73_sorgente.png")
    d.controlla("D18", lambda: "non vale per" not in barra.inner_text() and "(non in V1)" not in barra.inner_text(), "la barra della sorgente elenca i gesti che non valgono")
    d.controlla("D18", lambda: expect(barra.locator("#du-sg-nonvale")).to_contain_text("è la sorgente di ACM-1000"), "il perché dei gesti che non valgono non si dice in breve")
    d.fine()


@prova("74", "messaggio 46: il prodotto scelto dalle linguette porta con sé la cartella, la striscia e l'ultima mossa parlano del loro prodotto (D21, D22); l'identificativo del cliente come testo (D12); la bozza da rivedere senza il colore d'allarme (D35)")
def b74(b):
    p = b.page
    d = Difetti(b)
    a1 = b.al["ACM-1000_assieme.stp"]
    ui46_partenza(b)
    ui46_s8_dalla_proposta(b, a1)
    el = p.locator("#du-sg-elenco")
    expect(el).to_contain_text("ACM-1100_A.pdf", timeout=T_LUNGO)
    b.foto("UI46_B74_prodotto1.png")
    d.controlla("D12", lambda: "[object Object]" not in el.inner_text(), "le righe dicono «id. cliente [object Object]»")
    d.controlla("D12", lambda: expect(el).to_contain_text("id. cliente ACM-1100/A"), "le righe non dicono l'identificativo del cliente")
    cls = p.locator("#du-passo-revisione .du-st").first.get_attribute("class") or ""
    d.controlla("D35", lambda: "du-st--bozza" not in cls and "du-st--ko" not in cls, "la bozza da rivedere ha il colore d'allarme: %s" % cls)
    # una striscia che parla di ACM-1000: l'anteprima del suo STEP (l'esito riuscito di S8 non ha più la striscia, D23)
    b.guarda(a1)
    expect(b.avviso()).to_contain_text("Proposta dello STEP in vista", timeout=T_LUNGO)
    b.cartella("p:" + b.p1)
    tab2 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)
    tab2.click()
    expect(tab2).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    p.wait_for_timeout(500)
    b.foto("UI46_B74_prodotto2.png")
    d.controlla("D21", lambda: expect(p.locator("#du-sg-titolo")).to_contain_text("ACM-2000"), "dopo la linguetta di ACM-2000 l'elenco resta su ACM-1000")
    d.controlla("D21", lambda: expect(p.locator("#du-sg-c-p_" + sid(b.p2))).to_have_attribute("aria-selected", "true"), "la cartella di ACM-2000 non è scelta")
    d.controlla("D22", lambda: not b.avviso().is_visible(), "la striscia parla ancora di ACM-1000: %s" % b.avviso().inner_text())
    d.controlla("D22", lambda: expect(p.locator("#du-sg-mossa")).to_contain_text("Ultima mossa · ACM-1000"), "l'ultima mossa non dice che è di ACM-1000")
    d.fine()


@prova("75", "messaggio 46, revisione della corsia SORGENTE: niente «null» nell'intestazione di un prodotto senza sorgente (R1); niente versioni del progetto nel dettaglio di uno STEP candidato (R2); la sorgente superata senza «adesso è un altro STEP» (R5); una pillola sola per lo stato della sorgente nella riga e il separatore in «Destinazioni» (R6)")
def b75(b):
    p = b.page
    d = Difetti(b)
    a1 = b.al["ACM-1000_assieme.stp"]
    a2 = b.al["ACM-1100_A.pdf"]
    a10 = b.al["ACM-1000_rev2.stp"]

    def grezzo(testo):
        return [l for l in testo.splitlines() if l.strip() in ("null", "undefined")] + (["[object Object]"] if "[object Object]" in testo else [])

    def intestazione(k):
        b.cartella(k)
        if p.locator("#du-sg-prodotto .du-sg-prodotto-c").count() == 0 and p.locator("#du-sg-prodotto-apri").count():
            p.click("#du-sg-prodotto-apri")
        p.wait_for_timeout(300)
        return p.locator("#du-sg-elenco")

    ui46_partenza(b)
    # R2: il «Rifiuta la candidata…» spento nel dettaglio di uno STEP candidato (non proposto) non dice la versione in cui
    # arriva, né «rifiuto motivato» (il motivo dell'utente si toglie da ogni widget, messaggio 46 [U] punto 1)
    b.file(a10, "step")
    det = p.locator("#du-sg-dettaglio")
    expect(det).to_contain_text("Rifiuta la candidata", timeout=T_LUNGO)
    p.wait_for_timeout(300)
    b.foto("UI46_B75_candidata.png")

    def versioni():
        return [l for l in det.inner_text().splitlines() if re.search(r"\bV[1-9]\b", l)]
    d.controlla("R2", lambda: not versioni(), "il dettaglio dello STEP candidato dice una versione del progetto: %s" % versioni())
    d.controlla("R2", lambda: "rifiuto motivato" not in det.inner_text(), "il dettaglio dello STEP candidato dice «rifiuto motivato»")
    # R1: l'intestazione del secondo prodotto, che non ha sorgente né bozza
    el = intestazione("p:" + b.p2)
    b.foto("UI46_B75_prodotto2.png")
    d.controlla("R1", lambda: not grezzo(el.inner_text()), "testo grezzo nell'intestazione di ACM-2000: %s" % grezzo(el.inner_text()))
    # R6: dopo la conferma, nella riga dello STEP sorgente lo stato una volta sola (la cartella di ACM-1000 riporta il
    # prodotto scelto, D21)
    b.cartella("p:" + b.p1)
    ui46_s8_dalla_proposta(b, a1)
    intestazione("p:" + b.p1)
    riga = p.locator('#du-sg-elenco li[data-allegato="%s"]' % a1).first
    expect(riga).to_be_visible(timeout=T_LUNGO)
    b.foto("UI46_B75_riga_sorgente.png")

    def stati_sorgente():
        return [t for t in riga.locator(".du-st").all_inner_texts() if "sorgente" in t]
    d.controlla("R6", lambda: len(stati_sorgente()) == 1, "la riga dello STEP sorgente dice lo stato %d volte: %s" % (len(stati_sorgente()), stati_sorgente()))
    # R6: «Destinazioni» con « · » fra la revisione e l'identificativo del cliente
    b.file(a2)
    dest = p.locator("#du-sg-destinazioni")
    expect(dest).to_contain_text("id. cliente", timeout=T_LUNGO)
    d.controlla("R6", lambda: re.search(r"(?<!· )id\. cliente", dest.inner_text()) is None, "in «Destinazioni» manca il separatore: %s" % dest.inner_text())
    # R1: l'intestazione del primo prodotto dopo la revoca (nessuna sorgente valida, la bozza annullata)
    b.file(a1, "step")
    p.click("#du-sg-dettaglio #du-revoca-sorgente-" + sid(a1))
    expect(b.finestra()).to_contain_text("Revoca", timeout=T_LUNGO)
    if p.locator("#du-finestra #f-motivo").count():
        p.fill("#du-finestra #f-motivo", "prova")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Sorgente revocata per ACM-1000", timeout=T_LUNGO)
    el = intestazione("p:" + b.p1)
    b.foto("UI46_B75_prodotto1_revocato.png")
    d.controlla("R1", lambda: not grezzo(el.inner_text()), "testo grezzo nell'intestazione di ACM-1000 dopo la revoca: %s" % grezzo(el.inner_text()))
    # R5: il server dice «superata» anche per il documento sostituito; la conferma di S8 di ACM-1000 vista superata
    # (la lettura di /dati ritoccata qui: il banco non ha il caso)

    def superata(route):
        r = route.fetch()
        try:
            dati = r.json()
        except Exception:
            route.fulfill(response=r)
            return

        def giro(x):
            if isinstance(x, dict):
                s8 = x.get("sorgente_s8")
                if isinstance(s8, dict) and str(s8.get("allegato_id", "")).lower() == a1.lower():
                    s8["valida"] = False
                    s8["motivo"] = "superata"
                for v in x.values():
                    giro(v)
            elif isinstance(x, list):
                for v in x:
                    giro(v)
        giro(dati)
        route.fulfill(response=r, body=json.dumps(dati))
    p.route("**/motore-a/dati*", superata)
    try:
        p.click("#du-aggiorna")
        expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
        b.file(a1, "step")
        st = p.locator("#du-sg-dettaglio #du-stato-sorgente-" + sid(b.p1))
        d.controlla("R5", lambda: expect(st).to_have_attribute("data-stato", "sostituita", timeout=T_LUNGO), "il dettaglio non ha il riquadro della sorgente superata")
        b.foto("UI46_B75_superata.png")
        d.controlla("R5", lambda: "un'altra conferma" not in p.locator("#du-passo-sorgente").inner_text(), "la testata dice «%s»" % p.locator("#du-passo-sorgente").inner_text())
        d.controlla("R5", lambda: "adesso è un altro STEP" not in p.locator("#du-sg-dettaglio").inner_text(), "la sorgente superata dice «%s»" % p.locator("#du-sg-dettaglio").inner_text().replace("\n", " "))
    finally:
        p.unroute("**/motore-a/dati*", superata)
    d.fine()


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
            if not scelte and numero in SOLO_SE_SCELTE:
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
