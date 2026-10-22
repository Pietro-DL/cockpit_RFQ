# -*- coding: utf-8 -*-
"""La prova d1 del PM23 §12.2 (TestV1DistintaUnicaPercorsoPersistito): la Distinta unica contro cockpit.exe VERO.

Lo lancia distinta_unica_persistito_browser_test.go (tag «integrazione browser»), una fase per volta, ogni volta con un
browser nuovo e un accesso nuovo: il server è un processo, che la prova ferma con Kill e riavvia sullo stesso database
fra una fase e l'altra (F6, F9). Niente risposte finte: ogni esito viene dal server; le sole interferenze sono quelle del
§12.2, provocate qui (la prima risposta di R5 persa in F7). Lo script fallisce a ogni pageerror, console.error o risposta
≥ 400 non attesa (come motore_a.py), e a ogni richiesta verso le rotte che la pagina nuova non deve chiamare (§12.2,
«Controlli trasversali»).

Le fasi (l'elenco e le parti NON ESEGUITE stanno nel commento del test e nel resoconto della corsia):
  F1   apertura senza scritture: la pagina, la Sorgente e la Revisione, «Guarda» di due STEP (S6), «Apri» di un PDF (S5),
       la cartella del prodotto (S9); nessuna richiesta che non sia una GET
  F2   «+ Aggiungi file»: lo zip e due file, «in analisi» poi «letto»
  F3   «Conferma questo STEP come sorgente di 9120001…» (S8): la bozza v1 costruita dal server, nello stesso tab
  F4   le modifiche della bozza; F5 i file (Altri file, Scarta, i rifiuti, Riattiva)
  F6   dopo il riavvio vero: la pagina riaperta ridisegna la stessa bozza (l'impronta della pagina uguale a quella di prima)
  F6b  «Usa nuovo STEP per 9120001…» con lo STEP rev 2 prima di R5: la bozza sostituita (G39-03, giro G39-ter), con la
       presa d'atto; F6c il secondo prodotto: S8, «Riparti dalla proposta», «Abbandona»
  F7   il wizard e «Conferma BOM» con la prima risposta persa, R6; F8 R5 ripetuta (riproposto, poi 409)
  F9   dopo il secondo riavvio vero: la Revisione e lo storico; F10 il giro di sole letture
  F12  due sessioni con il wizard aperto: N1, N4, poi N2 o N5 (il worker lo avvia lo script)

Una riga «DATI {json}» passa al test quello che la pagina ha visto (l'impronta della pagina, il corpo di R5).
I dati sono inventati (ACME, 912xxxx).
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
# l'avviso dello scarto: il testo è del server (la rimozione dallo staging, la copia in ALTRO)
SCARTATO = re.compile(r"File scartato|Scarto registrato")
verifica = du.verifica
sid = du.sid

# §12.2, controlli trasversali: le rotte che la pagina nuova non chiama mai (D23:284)
VIETATE = [r"/distinta/albero(/|$)", r"/fascicolo/nodo/", r"/fascicolo/componente/[^/]+/(sposta|collega|scollega|figli|padre|struttura)",
           r"/fascicolo/rimozione/", r"/proposta/[^/]+/scarta$", r"/fascicolo/assegna$"]


class BancoD1(dus.BancoVero):
    """Gli aiuti della pagina di distinta_unica.py e distinta_unica_server.py, con i dati dal test."""

    def __init__(self, page, a):
        super().__init__(page, a)
        d = json.loads(a.dati)
        self.p2 = d.get("prodotto2", "")
        self.d = d
        self.a = a
        self.attese = []

    def vietate(self):
        return [(r["metodo"], r["path"]) for r in self.richieste if any(re.search(v, r["path"]) for v in VIETATE)]

    def impronta_pagina(self):
        """Quello che la pagina mostra della bozza del prodotto 1 e dei file: la barra del salvataggio, i nodi
        dell'organigramma (chiave, stato, testo), i file della cartella «Tutti i file» con il loro stato."""
        p = self.page
        self.passo("revisione")
        expect(p.locator("#du-corpo")).not_to_have_attribute("aria-busy", "true", timeout=T_LUNGO)
        out = {"salvataggio": p.locator("#du-salvataggio").get_attribute("data-du-salvataggio"),
               "versione": re.sub(r"\s+", " ", p.locator("#du-salvataggio").inner_text()).strip()}
        # senza bozza la Revisione mostra la BOM confermata (#du-working, messaggio 47 [U]): i nodi qui sono quelli della bozza
        if p.locator("#du-nessuna-bozza, #du-working").count() == 0:
            self.vista("org")
            expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)
            out["nodi"] = p.locator("#du-vp .du-nc[data-k]").evaluate_all(
                "(ls) => ls.map(l => [l.getAttribute('data-k'), l.getAttribute('data-du-stato') || '', l.innerText.replace(/\\s+/g, ' ').trim()]).sort()")
        self.passo("sorgente")
        self.cartella("tutti")
        out["file"] = p.locator("#du-sg-elenco li[data-allegato]").evaluate_all(
            "(ls) => ls.map(l => [l.getAttribute('data-allegato'), l.getAttribute('data-stato') || '', l.innerText.replace(/\\s+/g, ' ').trim()]).sort()")
        return out


def dati(**kw):
    print("DATI " + json.dumps(kw, ensure_ascii=False, sort_keys=True))


# ---------------------------------------------------------------- le fasi

def fase_f1(b):
    """Apertura senza scritture (TS-01…TS-05, SG-01, SG-02, FL-03; U2: S9 senza scritture)."""
    p = b.page
    expect(p.locator("#du-passo-sorgente")).to_have_attribute("aria-current", "step")
    expect(p.locator("#du-passo-revisione")).to_contain_text("nessuna bozza")
    # la Revisione senza bozza
    b.passo("revisione")
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    # «Guarda» di due STEP (S6): l'anteprima, nessuna scrittura
    for nome in ("9120001.stp", "9120001_r2.stp"):
        i = b.segna()
        b.guarda(b.al[nome])
        s6 = b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$")
        verifica(s6, "«Guarda» di %s senza S6" % nome)
        print("      S6 di %s: %s" % (nome, p.locator("#du-anteprima").inner_text().replace("\n", " · ")[:200]))
        p.click("#du-anteprima-chiudi")
    # «Apri» di un PDF (S5): il visore della pagina
    pdf = b.al["9120020A_1.pdf"]
    b.file(pdf)
    i = b.segna()
    p.click("#du-apri-" + sid(pdf))
    expect(p.locator("#du-visore-tela canvas")).to_be_visible(timeout=T_LUNGO)
    verifica(b.richieste_da(i, "GET", r"/contenuto$"), "il visore non ha letto S5")
    b.chiudi_finestra()
    # la cartella del prodotto: lo smistamento di S9
    i = b.segna()
    b.cartella("p:" + b.p1)
    p.wait_for_timeout(500)
    print("      cartella del prodotto: %s" % p.locator("#du-sg-elenco").inner_text().replace("\n", " · ")[:300])
    non_get = [(r["metodo"], r["path"]) for r in b.richieste if r["metodo"] != "GET"]
    verifica(not non_get, "richieste non GET nella fase di sola apertura: %s" % non_get)


def fase_f2(b):
    """«+ Aggiungi file»: lo zip con due voci e due file sciolti (FL-01, FL-02). La risposta JSON file per file; le righe
    «in analisi», poi «letto» (l'estrazione la fa il server, l'analisi il worker vero); nessun file su un componente."""
    p = b.page
    nomi = ["PACCHETTO_9120001.zip", "9120001A_SVILUPPO.dxf", "COLLAUDO_PROVA.pdf"]
    b.cartella("tutti")
    b.apri_carica()
    p.select_option("#du-carica-origine", "portale")
    p.set_input_files("#du-carica-file", files=[b.d["caricare"][n] for n in nomi])
    i = b.segna()
    with p.expect_response(lambda r: r.request.method == "POST" and r.url.split("?")[0].endswith("/carica")) as ri:
        p.click("#du-carica-invia")
    r = ri.value
    corpo = r.json()
    print("      E1′: %d %s" % (r.status, json.dumps({k: corpo[k] for k in corpo if k != "file"}, ensure_ascii=False)))
    for f in corpo.get("file", []):
        print("        - %s" % json.dumps(f, ensure_ascii=False)[:300])
    verifica(r.status == 200 and corpo.get("esito") == "caricati", "E1′: %d %s" % (r.status, corpo))
    esiti = {f.get("nome_file"): f.get("esito") for f in corpo.get("file", [])}
    verifica(all(esiti.get(n) == "caricato" for n in nomi), "E1′ file per file: %s" % esiti)
    e1 = b.richieste_da(i, "POST", r"/carica$")
    verifica(len(e1) == 1 and e1[0]["headers"].get("accept") == "application/json", "E1′: %s" % [x["headers"] for x in e1])
    expect(p.locator('#du-carica-esiti li[data-esito="caricato"]')).to_have_count(3, timeout=T_LUNGO)
    # le righe: «in analisi», poi «letto» (le voci dello zip arrivano con l'estrazione del server)
    visti = set()
    fine = time.time() + 90
    attesi = ["9120001A_SVILUPPO.dxf", "COLLAUDO_PROVA.pdf", "9120030A_1.pdf", "9120040A_1.pdf"]
    giri = 0
    while time.time() < fine:
        testi = {}
        for n in attesi:
            riga = p.locator("#du-sg-elenco li.du-sg-riga", has_text=n)
            testi[n] = riga.first.inner_text() if riga.count() else ""
            if "in analisi" in testi[n]:
                visti.add(n)
        if all("letto" in testi[n] for n in attesi):
            break
        p.wait_for_timeout(1000)
        giri += 1
        # la pagina rilegge S1 da sé quando cambia la firma; un «Aggiorna» ogni tanto (una lettura) per non dipenderne
        if giri % 8 == 0:
            p.click("#du-aggiorna")
            expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
            b.cartella("tutti")
    for n in attesi:
        riga = p.locator("#du-sg-elenco li.du-sg-riga", has_text=n).first
        expect(riga).to_contain_text("letto", timeout=T_LUNGO)
    print("      visti «in analisi» prima di «letto»: %s" % sorted(visti))
    verifica(visti, "nessuna riga è passata da «in analisi»")
    verifica(not b.richieste_da(i, "POST", r"/motore-a/bozza$"), "il caricamento ha scritto nella bozza")
    dati(caricamento=corpo, in_analisi=sorted(visti))


def nodo(b, testo):
    """La chiave della scheda dell'organigramma che comincia con il testo dato (un codice o «(senza codice) — Assieme»).
    Non legge l'innerText della scheda, che dal messaggio 46 comincia con il tipo e lo stato: compone codice, descrizione
    («—» se manca) e tipo dai campi della scheda (.du-nc-c, .du-nc-desc, .du-tipo)."""
    ks = b.page.locator("#du-vp .du-nc[data-k]").evaluate_all(
        "(ls, t) => ls.filter(l => { const q = (s) => { const e = l.querySelector(s); return e ? e.innerText.replace(/\\s+/g, ' ').trim() : ''; };"
        " return [q('.du-nc-c') || q('.du-nc-cod'), q('.du-nc-desc') || '—', q('.du-tipo')].join(' ').startsWith(t); })"
        ".map(l => l.getAttribute('data-k'))", testo)
    verifica(len(ks) == 1, "la scheda di %s: %s" % (testo, ks))
    return ks[0]


def salvata(b):
    expect(b.page.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)


def fase_f4(b):
    """Le modifiche della bozza (BZ, NI, EL): «Non includere» dal cassetto dei segnalati con un segnalato tolto dalla
    selezione, «Annulla» e «Ripeti» dopo la rilettura della pagina, «Correggi» del nodo senza codice, quantità, tipo,
    sposta e collega dai dialoghi, un pezzo aggiunto con un file preassociato e poi «Non crearlo», il vassoio e «Lascia
    senza associazione», la preassociazione in conflitto, «Accetta le proposte (N)»."""
    p = b.page
    b.passo("revisione")
    b.vista("org")
    # il cassetto «Nodi segnalati»: i tre segnalati (LS-02), WELDED SUPPORT no
    p.click("#du-segnalati-apri")
    expect(p.locator("#du-segnalati")).to_contain_text("Nodi segnalati (3)", timeout=T_LUNGO)
    testo = p.locator("#du-segnalati").inner_text()
    verifica("WELDED SUPPORT" not in testo, "WELDED SUPPORT fra i segnalati")
    seg = p.locator("#du-segnalati .du-segnalato[data-k]").evaluate_all("(ls) => ls.map(l => l.getAttribute('data-k'))")
    verifica(len(seg) == 3, "segnalati: %s" % seg)
    p.click("#du-seleziona-segnalati")
    expect(p.locator("#du-selezione")).to_contain_text("3 nodi selezionati")
    tenuto = seg[2]
    p.locator("#sel-" + sid(tenuto)).click()
    expect(p.locator("#du-selezione")).to_contain_text("2 nodi selezionati")
    i = b.segna()
    p.click("#du-non-includere")
    expect(p.locator("#f-elenco input:checked")).to_have_count(2)
    # messaggio 46 [U]: nessun motivo da scrivere o da scegliere
    verifica(p.locator("#du-finestra #f-motivo, #du-finestra #f-motivo-codice").count() == 0, "«Non includere» chiede ancora il motivo")
    b.ok_finestra()
    noninc = p.locator('#du-vp .du-nc[data-du-stato="non incluso (bozza)"]')
    expect(noninc).to_have_count(2, timeout=T_LUNGO)
    salvata(b)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    dec = [a["dopo"] for a in az if a["azione"] == "decisione_aggiunta"]
    ids = {(a.get("dopo") or {}).get("azione_id") for a in az}
    # la decisione porta la traccia del gesto e motivo_codice dal segnale visto (elemento_cad per il possibile elemento CAD) [T]
    verifica(len(dec) == 2 and all(d["tipo"] == "non_includere" and d["motivo"].startswith("gesto ") and "da «Non includere nella BOM»" in d["motivo"]
                                   and d["motivo_codice"] == ("elemento_cad" if (d.get("segnale_visto") or {}).get("codice") == "ancoraggio.possibile_elemento_cad" else "altro")
                                   for d in dec) and len(ids) == 1 and list(ids)[0],
             "«Non includere»: %s" % [(a["azione"], (a.get("dopo") or {}).get("azione_id")) for a in az])
    # «Annulla» e «Ripeti» del gesto dopo la rilettura della pagina (la pila dalla bozza salvata)
    p.reload()
    b.pronta()
    b.passo("revisione")
    b.vista("org")
    expect(noninc).to_have_count(2, timeout=T_LUNGO)
    i = b.segna()
    expect(p.locator("#du-annulla")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-annulla")
    expect(noninc).to_have_count(0, timeout=T_LUNGO)
    salvata(b)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica(az and all(a["azione"] == "annulla" for a in az), "«Annulla» dopo la rilettura: %s" % [a["azione"] for a in az])
    i = b.segna()
    p.click("#du-ripeti")
    expect(noninc).to_have_count(2, timeout=T_LUNGO)
    salvata(b)
    az = b.corpi(i, "POST", r"/motore-a/bozza$")[0]["azioni"]
    verifica(az and all(a["azione"] == "rifai" for a in az), "«Ripeti»: %s" % [a["azione"] for a in az])
    # «Correggi» del nodo non foglia senza codice (WELDED SUPPORT), senza motivo (messaggio 46 [U])
    k_ws = nodo(b, "(senza codice) — Assieme")
    b.scheda(k_ws)
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-correggi")
    p.fill("#f-codice", "9120050")
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "«Correggi» chiede ancora il motivo")
    expect(p.locator("#du-finestra #f-ok")).to_be_enabled(timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#nc-" + sid(k_ws))).to_contain_text("9120050", timeout=T_LUNGO)
    salvata(b)
    az = b.ultime_azioni(i)
    verifica(any(a["azione"] == "decisione_aggiunta" and a["dopo"]["tipo"] == "identita" for a in az), "«Correggi»: %s" % [a["azione"] for a in az])
    # la quantità di 9120040A sotto la radice
    k40 = nodo(b, "9120040A")
    b.scheda(k40)
    i = b.segna()
    p.click("#du-quantita")
    p.fill("#f-qta", "3")
    b.ok_finestra()
    salvata(b)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["qta_modificata"] and az[0]["dopo"]["qta"] == 3, "quantità: %s" % az)
    # «Cambia tipo»: 9120060A da particolare a commerciale
    k60 = nodo(b, "9120060A")
    b.scheda(k60)
    i = b.segna()
    p.click("#du-cambia-tipo")
    p.select_option("#f-tipo", "commerciale")
    b.ok_finestra()
    salvata(b)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("nodo_modificato", {"tipo": "commerciale"})], "«Cambia tipo»: %s" % az)
    # «Sposta sotto…» (dal dialogo) di 9120030A sotto la radice, con la quantità scritta; «Collega anche sotto…» 9120020A
    k_root = nodo(b, "9120001")
    k30, k20 = nodo(b, "9120030A"), nodo(b, "9120020A")
    b.scheda(k30)
    i = b.segna()
    p.click("#du-sposta")
    p.select_option("#f-a", k_root)
    p.fill("#f-qta", "2")
    b.ok_finestra()
    salvata(b)
    az = b.ultime_azioni(i)
    verifica(len(az) == 1 and az[0]["azione"] == "arco_spostato" and az[0]["dopo"]["padre"] == k_root and az[0]["dopo"]["qta"] == 2, "«Sposta sotto…»: %s" % az)
    b.scheda(k20)
    i = b.segna()
    p.click("#du-collega")
    p.select_option("#f-a", k_root)
    p.fill("#f-qta", "1")
    b.ok_finestra()
    salvata(b)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_aggiunto"] and az[0]["dopo"]["padre"] == k_root, "«Collega anche sotto…»: %s" % az)
    # un pezzo senza codice aggiunto sotto l'assieme, con NOTA_TECNICA.pdf preassociato dal pannello; «Non crearlo» lo
    # toglie e il file va nel vassoio; «Lascia senza associazione»
    nota = b.al["NOTA_TECNICA.pdf"]
    k10 = nodo(b, "9120010A")
    b.scheda(k10)
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-aggiungi-sotto")
    p.select_option("#f-tipo", "sciolto")
    p.fill("#f-desc", "Rondella aggiunta a mano")
    p.fill("#f-qta", "4")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="nuovo"]')).to_have_count(1, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["nodo_aggiunto", "arco_aggiunto"], "«Aggiungi»: %s" % [a["azione"] for a in az])
    k_nuovo = az[0]["dopo"]["chiave_bozza"]
    salvata(b)
    expect(p.locator("#du-pannello")).to_have_attribute("data-k", k_nuovo)
    b.linguetta("documenti")
    i = b.segna()
    p.select_option("#du-pannello-file", label="NOTA_TECNICA.pdf")
    p.click("#du-pannello-preassocia")
    expect(p.locator("#du-doc-nodo")).to_contain_text("◈ preassociato", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_preassociato", {"allegato_id": nota, "chiave_bozza": k_nuovo})], "preassociazione: %s" % az)
    salvata(b)
    b.linguetta("dettaglio")
    i = b.segna()
    p.click("#du-togli")
    expect(b.finestra()).to_contain_text("1 file preassociati vanno in «Da ricollocare»")
    b.ok_finestra()
    expect(p.locator('#du-vp .du-nc[data-du-stato="nuovo"]')).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["arco_tolto", "file_svincolato", "nodo_tolto"], "«Non crearlo»: %s" % [a["azione"] for a in az])
    salvata(b)
    b.vista("elenco")
    vass = p.locator("#du-vassoio")
    expect(vass).to_contain_text("NOTA_TECNICA.pdf", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-lascia-" + sid(nota))
    expect(vass).to_have_count(0, timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([(a["azione"], a["dopo"]) for a in az] == [("file_lasciato", {"allegato_id": nota})], "«Lascia senza associazione»: %s" % az)
    salvata(b)
    # «Accetta le proposte (N)» (EL-03, PE-21): le proposte del motore dopo l'apertura, se ce ne sono
    bt = p.locator("#du-accetta-valide")
    print("      %s · %s" % (bt.inner_text().strip(), bt.get_attribute("title") or ""))
    accettate = "NON ESEGUITA"
    if bt.is_enabled():
        i = b.segna()
        bt.click()
        expect(b.finestra()).to_contain_text("Accetta le proposte")
        p.locator("#f-proposte input").first.check()
        b.ok_finestra()
        salvata(b)
        az = b.ultime_azioni(i)
        verifica(az and all(a["azione"] == "file_preassociato" for a in az), "«Accetta le proposte»: %s" % az)
        accettate = "ESEGUITA"
    # la preassociazione in conflitto: 9120099A_1.pdf (codice che lo STEP non ha) su 9120030A, dalla Sorgente
    altro = b.al["9120099A_1.pdf"]
    b.azione("associa", altro)
    expect(b.finestra()).to_contain_text("Associa «9120099A_1.pdf» a un nodo", timeout=T_LUNGO)
    b.scegli_nodo("9120030A")
    i = b.segna()
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_preassociato"] and az[0]["dopo"]["allegato_id"] == altro, "preassociazione in conflitto: %s" % az)
    salvata(b)
    dati(segnalati=seg, tenuto=tenuto, accetta_le_proposte=accettate, chiavi={"radice": k_root, "ws": k_ws, "k20": k20, "k30": k30, "k40": k40, "k60": k60})
    dati(pagina=b.impronta_pagina())
    return "«Accetta le proposte (N)» %s" % accettate


def testo_di(loc):
    return loc.inner_text().replace("\n", " · ")[:500] if loc.count() else ""


def fase_f5(b):
    """I file (FL-04…FL-10; U2 §3): «Metti in Altri file» il DXF (la copia nella cartella ALTRO della RFQ sul NAS);
    «Scarta» il collaudo del cliente (unica copia: «Ho capito»); lo scarto del PDF preassociato e Altri file dello zip con
    una voce preassociata rifiutati con le dipendenze; una voce dello zip scartata dopo «Togli»; poi «Riattiva» il DXF."""
    p = b.page
    a = b.al
    ok = p.locator("#du-finestra #f-ok")
    err = p.locator("#du-finestra-errore")
    b.passo("sorgente")
    # Altri file: il DXF
    dxf = a["9120001A_SVILUPPO.dxf"]
    i = b.segna()
    b.azione("altri", dxf)
    expect(b.finestra()).to_contain_text("cartella ALTRO della RFQ sul NAS", timeout=T_LUNGO)
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File messo in Altri file", timeout=T_LUNGO)
    print("      Altri file: %s" % testo_di(b.avviso()))
    expect(p.locator("#du-sg-c-altri .du-sg-conto")).to_have_text("1", timeout=T_LUNGO)
    b.cartella("altri")
    riga = p.locator("#du-file-" + sid(dxf))
    expect(riga).to_have_attribute("data-stato", "altri_file", timeout=T_LUNGO)
    expect(riga).to_contain_text("Copia nella cartella ALTRO della RFQ verificata", timeout=T_LUNGO)
    s3 = b.corpi(i, "POST", r"/altri-file$")
    verifica(len(s3) == 1, "S3: %d invii" % len(s3))
    # Scarta: il collaudo del cliente, l'unica copia (nessuna mail la contiene)
    col = a["COLLAUDO_PROVA.pdf"]
    b.cartella("tutti")
    i = b.segna()
    b.azione("scarta", col)
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    print("      dialogo di Scarta del collaudo: %s" % testo_di(b.finestra()))
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "«Scarta» chiede ancora il motivo")
    unica = p.locator("#f-unica").count() > 0
    if unica:
        expect(ok).to_be_disabled()
        p.locator("#f-unica").check()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text(SCARTATO, timeout=T_LUNGO)
    print("      Scarta: %s" % testo_di(b.avviso()))
    s2 = b.corpi(i, "POST", r"/scarta$")
    verifica(len(s2) == 1 and "motivo" not in s2[0], "S2: %s" % s2)
    b.cartella("scartati")
    riga = p.locator("#du-file-" + sid(col))
    expect(riga).to_have_attribute("data-stato", "scartato", timeout=T_LUNGO)
    expect(riga).to_contain_text("Scartato da AD")
    # messaggio 46 [U]: la riga dice chi e quando, non la traccia del gesto che il server scrive come motivo
    verifica("gesto di" not in riga.inner_text(), "la riga dello scartato mostra la traccia: %s" % testo_di(riga))
    print("      riga dello scartato: %s" % testo_di(riga))
    # lo scarto del PDF preassociato (la PB del motore): le dipendenze nel dialogo, il server rifiuta se si insiste
    pdf20 = a["9120020A_1.pdf"]
    b.cartella("tutti")
    b.azione("scarta", pdf20)
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    dip = p.locator("#f-dipendenze li[data-codice]").evaluate_all("(ls) => ls.map(l => l.getAttribute('data-codice') + ': ' + l.innerText)")
    print("      dipendenze dello scarto di 9120020A_1.pdf: %s · conferma %s (%s)" % (dip, "accesa" if ok.is_enabled() else "spenta", ok.get_attribute("title") or ""))
    rifiuto_pdf = "dialogo"
    if ok.is_enabled():
        b.attese.append(("POST", r"/sorgente/file/%s/scarta$" % pdf20, 422))
        ok.click()
        expect(err).to_be_visible(timeout=T_LUNGO)
        print("      422 dello scarto: %s" % testo_di(err))
        rifiuto_pdf = "422"
    else:
        verifica(dip, "lo scarto del file preassociato è spento senza una dipendenza")
    b.chiudi_finestra()
    # Altri file dello zip che ha una voce preassociata
    zipa = a["PACCHETTO_9120001.zip"]
    b.azione("altri", zipa)
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    print("      dialogo di Altri file dello zip: %s · conferma %s (%s)" % (testo_di(b.finestra()), "accesa" if ok.is_enabled() else "spenta", ok.get_attribute("title") or ""))
    rifiuto_zip = "dialogo"
    if ok.is_enabled():
        b.attese.append(("POST", r"/sorgente/file/%s/altri-file$" % zipa, 422))
        ok.click()
        expect(err).to_be_visible(timeout=T_LUNGO)
        print("      422 di Altri file dello zip: %s" % testo_di(err))
        rifiuto_zip = "422"
    b.chiudi_finestra()
    # una voce dello zip: prima «Togli» la sua preassociazione (R2), poi «Scarta»
    v40 = a["9120040A_1.pdf"]
    b.cartella("tutti")
    i = b.segna()
    p.locator("#du-file-" + sid(v40)).get_by_role("button", name="Togli").click()
    az = b.ultime_azioni(i)
    verifica([x["azione"] for x in az] == ["file_svincolato"], "«Togli» della voce: %s" % az)
    salvata(b)
    i = b.segna()
    b.azione("scarta", v40)
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    if p.locator("#f-unica").count():
        p.locator("#f-unica").check()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text(SCARTATO, timeout=T_LUNGO)
    verifica(len(b.corpi(i, "POST", r"/scarta$")) == 1, "lo scarto della voce")
    # Riattiva il DXF
    i = b.segna()
    b.azione("riattiva", dxf, "altri")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("File riattivato", timeout=T_LUNGO)
    print("      Riattiva: %s" % testo_di(b.avviso()))
    expect(p.locator("#du-sg-c-altri .du-sg-conto")).to_have_text("0", timeout=T_LUNGO)
    b.cartella("tutti")
    riga = p.locator("#du-file-" + sid(dxf))
    expect(riga).to_have_attribute("data-stato", "attivo", timeout=T_LUNGO)
    print("      riga del DXF riattivato: %s" % testo_di(riga))
    b.file(dxf)
    print("      dettaglio del DXF riattivato: %s" % testo_di(p.locator("#du-sg-dettaglio")))
    dati(unica_copia=unica, rifiuto_pdf=rifiuto_pdf, rifiuto_zip=rifiuto_zip, pagina=b.impronta_pagina())


def fase_f3(b):
    """S8 sullo STEP del prodotto senza bozza: la bozza nasce dal server, nello stesso tab (U2 §2.2; dir. 39)."""
    p = b.page
    step = b.al["9120001.stp"]
    b.guarda(step)
    if p.locator("#du-strutture button").count():
        print("      candidate dello STEP: %s" % p.locator("#du-strutture button").all_inner_texts())
        p.locator("#du-strutture button").first.click()
    expect(p.locator("#du-conferma-sorgente")).to_contain_text("Conferma questo STEP come sorgente di 9120001", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    print("      dialogo di S8: %s" % b.finestra().inner_text().replace("\n", " · ")[:400])
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "la conferma della sorgente chiede ancora il motivo")
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_be_visible(timeout=T_LUNGO)
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    print("      esito di S8: %s" % esito.inner_text().replace("\n", " ")[:300])
    c = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c) == 1 and "motivo" not in c[0], "S8 inviata %d volte, o con il motivo: %s" % (len(c), c))
    print("      corpo di S8: %s" % {k: c[0][k] for k in c[0] if k not in ("chiave_idempotenza", "sha256")})
    b.passo("revisione")
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)
    b.salvata(1)
    # messaggio 46 (D05, rilievo della revisione ALBERO): sul server vero almeno una scheda porta la miniatura del suo 2D,
    # disegnata da pdf.js con i byte di S5
    fine = time.time() + 60
    while time.time() < fine and not p.locator("#du-vp .du-nc-mini img").evaluate_all("(ls) => ls.some(i => i.complete && i.naturalWidth > 0)"):
        p.wait_for_timeout(500)
    mini = p.locator("#du-vp .du-nc-mini").evaluate_all("(ls) => ls.map(l => [l.closest('.du-nc').getAttribute('data-k'), l.dataset.duStato || '', l.textContent.trim()])")
    print("      miniature dopo S8: %s" % mini)
    verifica(any(m[1] == "pronta" for m in mini), "nessuna miniatura disegnata dopo S8: %s" % mini)
    dati(pagina=b.impronta_pagina())


def fase_f6(b):
    """Dopo il riavvio vero: la pagina riaperta ridisegna la stessa bozza; «Altri file» si apre, gli scartati hanno la
    traccia (sola lettura)."""
    p = b.page
    dati(pagina=b.impronta_pagina())
    b.passo("sorgente")
    b.cartella("altri")
    expect(p.locator("#du-sg-elenco")).to_be_visible(timeout=T_LUNGO)
    b.cartella("scartati")
    for nome in ("COLLAUDO_PROVA.pdf", "9120040A_1.pdf"):
        riga = p.locator("#du-file-" + sid(b.al[nome]))
        expect(riga).to_have_attribute("data-stato", "scartato", timeout=T_LUNGO)
        expect(riga).to_contain_text("Scartato da AD")
        print("      dopo il riavvio, %s: %s" % (nome, testo_di(riga)[:200]))
    non_get = [(r["metodo"], r["path"]) for r in b.richieste if r["metodo"] != "GET"]
    verifica(not non_get, "richieste non GET nella riapertura dopo il riavvio: %s" % non_get)


def fase_f6b(b):
    """«Usa nuovo STEP per 9120001…» (S8 con il secondo STEP) prima di R5, sulla bozza nata dalla sorgente S8 di F3 e
    modificata in F4: G39-03 [U] e messaggio 43 caso 1 [U] (giro G39-ter): S8 sostituirebbe la bozza (annullata con la sua
    storia, sorgente revocata, bozza nuova dalla proposta) invece di cambiarne la fonte. Il dialogo legge S6 ?bozza=, dice
    la sostituzione al posto degli elenchi del cambio e, con le modifiche di F4, vuole la presa d'atto prima di «Usa questo
    STEP», poi S8 la scrive: la risposta lo dice, e la Revisione mostra la bozza nuova (versione 1) dalla proposta dello
    STEP rev 2 (le modifiche di F4 restano nella storia della bozza abbandonata)."""
    p = b.page
    r2 = b.al["9120001_r2.stp"]
    b.guarda(r2)
    if p.locator("#du-strutture button").count():
        print("      candidate dello STEP rev 2: %s" % p.locator("#du-strutture button").all_inner_texts())
        p.locator("#du-strutture button").first.click()
    gesto = p.locator("#du-cambia-sorgente, #du-conferma-sorgente").first
    expect(gesto).to_be_enabled(timeout=T_LUNGO)
    print("      gesto: %s" % gesto.inner_text())
    verifica("Usa nuovo STEP per 9120001" in gesto.inner_text(), "il gesto del cambio della sorgente: %s" % gesto.inner_text())
    i = b.segna()
    gesto.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    s6 = [r for r in b.richieste_da(i, "GET", r"/sorgente/step/[^/]+/anteprima$") if "bozza=" in r["query"]]
    verifica(s6, "il dialogo del cambio non ha letto l'effetto con S6 ?bozza=")
    fin = b.finestra()
    sost = fin.locator("#f-sostituzione-bozza")
    expect(sost).to_be_visible(timeout=T_LUNGO)
    expect(sost).to_contain_text("La bozza aperta viene sostituita")
    print("      sostituzione detta dal dialogo (S6 ?bozza=): %s" % testo_di(sost))
    verifica(fin.locator("#f-effetto").count() == 0, "il dialogo mostra ancora gli elenchi del cambio della fonte")
    verifica(fin.locator("#f-correzione-riga").count() == 0, "il dialogo offre la correzione della sorgente prima di R5")
    verifica(fin.locator("#f-motivo").count() == 0, "«Usa nuovo STEP» chiede ancora il motivo")
    ok = fin.locator("#f-ok")
    expect(ok).to_be_disabled(timeout=T_LUNGO)
    verifica("presa d'atto della bozza sostituita" in (ok.get_attribute("title") or ""), "«Usa questo STEP» senza la presa d'atto: %s" % ok.get_attribute("title"))
    detto = testo_di(sost)
    fin.locator("#f-presa-sostituzione").check()
    expect(ok).to_be_enabled(timeout=T_LUNGO)
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_be_visible(timeout=T_LUNGO)
    print("      esito di S8: %s · %s" % (esito.get_attribute("data-esito"), esito.inner_text().replace("\n", " ")[:400]))
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("sostituita", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c) == 1 and c[0].get("bozza") and "motivo" not in c[0] and (c[0].get("sostituisci") or {}).get("conferma") is True,
             "corpo di S8 della sostituzione: %s" % c)
    b.passo("revisione")
    b.salvata(1)
    dati(corpo_s8={k: c[0][k] for k in c[0] if k not in ("sha256",)}, sostituzione=detto, avviso=testo_di(b.avviso()))


def fase_f6c(b):
    """Il secondo prodotto finito (9130001): S8 sul suo STEP, poi «Riparti dalla proposta…» (nessuna PB passa alla bozza
    nuova, le AR invariate) e «Abbandona la bozza…» (le PB spariscono, i file tornano proposti)."""
    p = b.page
    s2 = b.al["9130001.stp"]
    b.guarda(s2, b.p2)
    if p.locator("#du-strutture button").count():
        p.locator("#du-strutture button").first.click()
    expect(p.locator("#du-conferma-sorgente")).to_contain_text("Conferma questo STEP come sorgente di 9130001", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "la conferma della sorgente del secondo prodotto chiede ancora il motivo")
    b.ok_finestra()
    expect(p.locator("#du-sg-esito-s8")).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    verifica(len(b.corpi(i, "POST", r"/conferma-sorgente$")) == 1, "S8 del secondo prodotto")
    b.passo("revisione")
    expect(p.locator("#du-vp")).to_be_visible(timeout=T_LUNGO)
    b.salvata(1)
    # «Riparti dalla proposta…»: la forma crea con «abbandona», senza motivo (messaggio 46 [U])
    i = b.segna()
    expect(p.locator("#du-riparti")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-riparti")
    expect(b.finestra()).to_contain_text("Le preassociazioni non passano alla bozza nuova", timeout=T_LUNGO)
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "«Riparti dalla proposta» chiede ancora il motivo")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza ripartita dalla proposta dello STEP", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")
    verifica(len(c) == 1 and c[0]["crea"]["partenza"] == "step" and c[0]["crea"]["abbandona"].get("bozza_id") and "motivo" not in c[0]["crea"]["abbandona"],
             "corpo di «Riparti»: %s" % c)
    b.salvata(1)
    # «Abbandona la bozza…»
    i = b.segna()
    p.click("#du-abbandona")
    expect(b.finestra()).to_contain_text("Le preassociazioni spariscono con lei", timeout=T_LUNGO)
    b.ok_finestra()
    expect(p.locator("#du-nessuna-bozza")).to_be_visible(timeout=T_LUNGO)
    verifica(len(b.richieste_da(i, "POST", r"/abbandona$")) == 1, "«Abbandona la bozza» non è arrivato al server")
    # i file del secondo prodotto tornano proposti (attendono la sua sorgente o sono proposti nella sua cartella)
    b.passo("sorgente")
    b.cartella("p:" + b.p2)
    print("      cartella di 9130001 dopo l'abbandono: %s" % testo_di(p.locator("#du-sg-elenco"))[:300])


def fase_f7(b):
    """Il riepilogo e la conferma (WZ-01…WZ-10, NI-09, NI-10): il wizard (R4 con gli avvisi del motore), i blocchi del
    server letti e le preassociazioni in conflitto risolte con il rimedio del wizard, «Conferma BOM» con la prima risposta
    di R5 persa (la richiesta arriva al server, la risposta no): «da verificare», R6, «Confermata»."""
    p = b.page
    b.passo("revisione")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(500)
    blocchi = b.blocchi_wizard()
    print("      blocchi di R4 (%d):" % len(blocchi))
    for x in blocchi:
        print("        - " + x.split(" Rimedi:")[0][:220])
    risolti = risolvi_conflitti(b)
    cta = p.locator("#du-wz-conferma")
    if cta.is_disabled():
        print("      perché spenta: %s" % cta.get_attribute("title"))
        for x in b.blocchi_wizard():
            print("        - " + x.split(" Rimedi:")[0][:220])
    expect(cta).to_be_enabled(timeout=T_LUNGO)
    return conferma_con_risposta_persa(b, blocchi, risolti)


def risolvi_conflitti(b):
    """Le preassociazioni in conflitto del riepilogo (R4), tolte con il rimedio del wizard «Togli la preassociazione»
    (un salvataggio R2, poi il riepilogo riletto); torna gli allegati tolti."""
    p = b.page
    conflitti = p.locator("#du-wz-documenti li[data-du-pb][data-esito=in_conflitto]")
    visti = conflitti.evaluate_all("(ls) => ls.map(l => l.getAttribute('data-du-pb') + ' ' + l.getAttribute('data-motivo') + ': ' + l.innerText.replace(/\\s+/g, ' ').slice(0, 200))")
    print("      preassociazioni in conflitto (%d): %s" % (len(visti), visti))
    risolti = []
    for _ in range(len(visti)):
        li = conflitti.first
        aid = li.get_attribute("data-du-pb")
        i = b.segna()
        li.locator("button[id^='du-wz-rimedio-togli-']").click()
        az = b.ultime_azioni(i)
        verifica(az and az[0]["azione"] == "file_svincolato", "il rimedio «Togli la preassociazione»: %s" % az)
        b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$"), "il riepilogo non si è riletto dopo il rimedio")
        expect(p.locator("#du-wz-documenti li[data-du-pb='%s'][data-esito=in_conflitto]" % aid)).to_have_count(0, timeout=T_LUNGO)
        risolti.append(aid)
    p.wait_for_timeout(500)
    return risolti


def conferma_con_risposta_persa(b, blocchi, risolti):
    """«Conferma BOM» con la prima risposta di R5 persa (la richiesta arriva al server, la risposta no): «da verificare»,
    R6 con la stessa chiave, «Confermata»."""
    p = b.page
    cta = p.locator("#du-wz-conferma")
    nonincl = testo_di(p.locator("#du-wz-noninclusi"))
    # messaggio 46 [U]: il wizard non mostra il motivo dei non inclusi (né la colonna né la traccia del gesto), ma chi ha deciso
    intest = [t.strip().lower() for t in p.locator("#du-wz-noninclusi thead th").all_inner_texts()]
    righe_ni = p.locator("#du-wz-noninclusi tbody tr[data-du-noninc]").count()
    verifica("motivo" not in intest and "gesto di" not in nonincl and (righe_ni == 0 or "deciso " in nonincl), "i non inclusi del wizard: %s · %s" % (intest, nonincl))
    g = du.perdi_la_risposta()
    p.route("**/motore-a/bom/conferma", g)
    try:
        i = b.segna()
        cta.click()
        e = b.esito_wizard()
        expect(e).to_have_attribute("data-esito", "da_verificare", timeout=T_LUNGO)
        print("      dopo la risposta persa: %s" % testo_di(e)[:300])
        expect(e).to_have_attribute("data-esito", "ok", timeout=60000)
        print("      dopo R6: %s" % testo_di(e)[:400])
    finally:
        p.unroute("**/motore-a/bom/conferma", g)
    r5 = b.corpi(i, "POST", r"/motore-a/bom/conferma$")
    verifica(len(r5) == 1, "la pagina ha mandato R5 %d volte" % len(r5))
    k = r5[0]["bozza"]["chiave_idempotenza"]
    r6 = b.richieste_da(i, "GET", r"/motore-a/intento/")
    verifica(r6 and all(r["path"].endswith("/" + k) for r in r6), "R6 senza la stessa chiave: %s" % [r["path"] for r in r6])
    verifica(not b.sospesi(), "la chiave resta in sospeso dopo «registrato»")
    p.click("#du-wz-chiudi")
    dati(r5=r5[0], chiave=k, blocchi=blocchi, conflitti_risolti=risolti, non_inclusi=nonincl)


def fase_f8(b):
    """L'idempotenza di R5 (MAT §5.2): lo stesso corpo con la stessa chiave → 200 riproposto, niente scritto; un corpo
    diverso con la stessa chiave → 409 CHIAVE_RIUSATA. Le richieste le manda lo script dalla pagina (stessa origine)."""
    p = b.page
    corpo = b.d["r5"]
    url = "/thread/%s/motore-a/bom/conferma" % b.thread
    manda = "async ([u, c]) => { const r = await fetch(u, {method: 'POST', headers: {'Content-Type': 'application/json', 'Accept': 'application/json'}, body: JSON.stringify(c)}); return [r.status, await r.text()]; }"
    st, testo = p.evaluate(manda, [url, corpo])
    print("      R5 con la stessa chiave e lo stesso corpo: %d %s" % (st, testo[:300]))
    j = json.loads(testo)
    verifica(st == 200 and j.get("riproposto") is True, "R5 ripetuta: %d %s" % (st, testo[:300]))
    diverso = json.loads(json.dumps(corpo))
    diverso["firma_anteprima"] = "0" * 64
    b.attese.append(("POST", r"/motore-a/bom/conferma$", 409))
    st, testo = p.evaluate(manda, [url, diverso])
    print("      R5 con la stessa chiave e un corpo diverso: %d %s" % (st, testo[:300]))
    verifica(st == 409 and json.loads(testo).get("codice") == "CHIAVE_RIUSATA", "R5 con un corpo diverso: %d %s" % (st, testo[:300]))
    dati(riproposto=j)


def fase_f9(b):
    """Dopo il secondo riavvio vero: la Revisione e lo storico (R7): «v1 confermata il … da …» e la verifica corrente
    «in prima istanza», due segni distinti; i nodi non inclusi della conferma."""
    p = b.page
    b.passo("revisione")
    sc = p.locator("#du-stato-conferma")
    expect(sc).to_contain_text("v1 confermata", timeout=T_LUNGO)
    print("      segni della conferma: %s" % testo_di(sc))
    # la verifica corrente la calcola il server a ogni lettura (/dati conferma_corrente.verifica_adesso): la pagina dice
    # lo stato e i motivi del server, distinti dalla conferma; qui si registra che cosa dice, senza giudicarlo
    va = p.locator("#du-verifica-adesso")
    expect(va).to_contain_text("Verificata in prima istanza", timeout=T_LUNGO)
    stato_va = va.get_attribute("data-du-verifica")
    motivi_va = va.locator("[title]").first.get_attribute("title") if va.locator("[title]").count() else ""
    print("      verifica corrente: %s · stato %s · motivi del server: %s" % (testo_di(va), stato_va, motivi_va))
    verifica("arriva in V2" not in testo_di(sc), "la Revisione dice ancora «Verificata: non ancora calcolata (arriva in V2)»")
    verifica("confermata" in testo_di(sc) and testo_di(va) and testo_di(va) in testo_di(sc), "i segni della conferma e della verifica non sono due segni distinti nella stessa riga")
    non_get = [(r["metodo"], r["path"]) for r in b.richieste if r["metodo"] != "GET"]
    verifica(not non_get, "richieste non GET nella riapertura dopo il secondo riavvio: %s" % non_get)
    cc = p.evaluate("async (u) => { const r = await fetch(u, {headers: {'Accept': 'application/json'}}); const j = await r.json(); return j.conferma_corrente || null; }",
                    "/thread/%s/motore-a/dati?prodotto=%s" % (b.thread, b.p1))
    print("      /dati conferma_corrente: %s" % json.dumps(cc, ensure_ascii=False)[:900])
    dati(segni=testo_di(sc), verifica=testo_di(va), stato_verifica=stato_va, motivi_verifica=motivi_va, conferma_corrente=cc)


def fase_f10(b):
    """Il giro finale di sole letture (MAT §1.5): la pagina in ogni passo e vista, la Sorgente con le sue cartelle, «Apri»
    di un PDF (S5), «Guarda» di uno STEP (S6), R1 anche ?confronto=1, R6 e R7, il wizard (R4). Il DB lo controlla il
    test: identico, nessun lavoro accodato."""
    p = b.page
    b.passo("sorgente")
    for k in ("dc", "step", "p:" + b.p1, "p:" + b.p2, "altri", "scartati", "tutti"):
        b.cartella(k)
    pdf = b.al["9120020A_1.pdf"]
    b.file(pdf)
    p.click("#du-apri-" + sid(pdf))
    expect(p.locator("#du-visore-tela canvas")).to_be_visible(timeout=T_LUNGO)
    b.chiudi_finestra()
    # «Guarda» di uno STEP della cartella (dopo la conferma, la candidatura che la pagina offre)
    b.cartella("step")
    gu = p.locator("#du-sg-elenco button[id^='du-guarda-']")
    print("      «Guarda» nella cartella degli STEP: %s" % gu.all_inner_texts())
    verifica(gu.count() > 0, "nessun «Guarda l'anteprima» nella cartella degli STEP")
    gu.first.click()
    expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
    p.click("#du-anteprima-chiudi")
    b.passo("revisione")
    for v in ("org", "elenco"):
        b.vista(v)
    letture = "async (us) => { const out = []; for (const u of us) { const r = await fetch(u, {headers: {'Accept': 'application/json'}}); out.push([u, r.status]); } return out; }"
    th = "/thread/%s" % b.thread
    esiti = p.evaluate(letture, [th + "/motore-a/bozza?prodotto=%s&confronto=1" % b.p1, th + "/motore-a/conferme?prodotto=%s" % b.p1,
                                 th + "/motore-a/intento/%s" % b.d["chiave"], th + "/motore-a/dati?prodotto=%s" % b.p1, th + "/sorgente"])
    print("      letture dirette: %s" % esiti)
    verifica(all(s == 200 for _, s in esiti), "letture non riuscite: %s" % esiti)
    # il wizard: R4 è una POST di sola lettura
    if p.locator("#du-cta-bom").count() == 0:
        r4 = "NON ESEGUITA (nessuna bozza aperta dopo la conferma: R4 vuole una bozza, e aprirla sarebbe una scrittura; il wizard con R4 si apre in F7 e in F12, fra gesti che scrivono)"
    elif p.locator("#du-cta-bom").is_enabled():
        b.apri_wizard()
        p.click("#du-wz-fase2")
        expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)
        p.click("#du-wz-chiudi")
        r4 = "eseguita"
    else:
        r4 = "NON ESEGUITA (" + (p.locator("#du-cta-bom").get_attribute("title") or "CTA spento") + ")"
    print("      R4 dal wizard: %s" % r4)
    scritture = [(r["metodo"], r["path"]) for r in b.richieste if r["metodo"] != "GET" and not r["path"].endswith("/motore-a/bom/riepilogo")]
    verifica(not scritture, "richieste di scrittura nel giro di sole letture: %s" % scritture)
    dati(r4=r4)


def fase_f12(b):
    """La notifica con il wizard aperto (U2 §6.3.6; CN-06, CN-08): due sessioni, A (l'admin) con il wizard della
    revisione aperto, B (un secondo amministratore: la pagina è dell'admin) in un altro browser. B carica un documento: N1 su A, con la presa d'atto, mai
    spuntata; B salva la bozza: N4 su A, «Conferma BOM» spento fino alla rilettura (il riepilogo superato non si
    conferma); poi il worker vero (lo avvia lo script, il test l'ha fermato) analizza il documento: N2 o N5 su A."""
    import subprocess
    p = b.page
    # A: la revisione dalla BOM confermata, una modifica (la quantità di un pezzo), il wizard. Senza bozza la Revisione
    # mostra la BOM confermata in sola lettura (messaggio 47 [U])
    b.passo("revisione")
    expect(p.locator("#du-working")).to_be_visible(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-da-conferma")
    b.salvata(1)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["crea"]["partenza"] == "conferma", "la revisione non parte dalla conferma: %s" % c["crea"])
    b.vista("org")
    k20 = nodo(b, "9120020A")
    b.scheda(k20)
    p.click("#du-quantita")
    p.fill("#f-qta", "5")
    b.ok_finestra()
    b.salvata(2)
    expect(p.locator("#du-cta-bom")).to_be_enabled(timeout=T_LUNGO)
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)
    p.wait_for_timeout(500)
    for x in b.blocchi_wizard():
        print("      blocco di R4 della revisione: " + x.split(" Rimedi:")[0][:220])
    risolvi_conflitti(b)
    cta = p.locator("#du-wz-conferma")
    acceso = cta.is_enabled()
    print("      «Conferma BOM» prima delle notifiche: %s" % ("acceso" if acceso else "spento (%s)" % cta.get_attribute("title")))
    i0 = b.segna()
    # B: un altro browser, un secondo amministratore
    ctx2 = p.context.browser.new_context(viewport={"width": 1440, "height": 900}, locale="it-IT")
    p2 = ctx2.new_page()
    p2.set_default_timeout(T_LUNGO)
    errori2 = []
    p2.on("pageerror", lambda e: errori2.append("pageerror: " + str(e)))
    p2.on("console", lambda m: errori2.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
    p2.goto(b.a.url + "/login")
    p2.fill("input[name=sigla]", "AB")
    p2.fill("input[name=password]", "prova-ab")
    p2.click("button[type=submit]")
    p2.wait_for_url("**/inbox*", timeout=15000)
    b2 = BancoD1(p2, b.a)
    b2.apri()
    nuovo = "9120010A_1.pdf"
    b2.apri_carica()
    p2.select_option("#du-carica-origine", "portale")
    p2.set_input_files("#du-carica-file", files=[b.d["caricare"][nuovo]])
    with p2.expect_response(lambda r: r.request.method == "POST" and r.url.split("?")[0].endswith("/carica")) as ri:
        p2.click("#du-carica-invia")
    verifica(ri.value.status == 200, "il caricamento di B: %d" % ri.value.status)
    # N1 su A: il documento in analisi (il worker è fermo), la presa d'atto mai spuntata, «Conferma BOM» spento senza
    n1 = p.locator("#du-wz-n1")
    u_ing = "/thread/%s/motore-a/bozza?prodotto=%s&ingressi=1" % (b.thread, b.p1)
    for giro in range(6):
        p.wait_for_timeout(5000)
        ing = [x for x in b.richieste_da(i0, "GET", r"/motore-a/bozza$") if "ingressi=1" in x["query"]]
        an = p.evaluate("async (u) => { const r = await fetch(u, {headers: {'Accept': 'application/json'}}); const j = await r.json(); return JSON.stringify(j.parti && j.parti.analisi); }", u_ing)
        print("      A, %2d s dopo il caricamento di B: letture degli ingressi della pagina %d; N1 %d; parte analisi adesso %s" % ((giro + 1) * 5, len(ing) - giro, n1.count(), an[:300]))
        if n1.count():
            break
    expect(n1).to_contain_text("È arrivato un documento nuovo (" + nuovo + "): è in analisi e il riepilogo non lo considera ancora.", timeout=40000)
    print("      N1 su A: %s" % testo_di(n1))
    expect(p.locator("#du-wz-in-analisi")).to_contain_text(nuovo, timeout=T_LUNGO)
    verifica(not p.locator("#du-wz-presa-analisi").is_checked(), "la presa d'atto dei documenti in analisi è già spuntata")
    expect(cta).to_be_disabled(timeout=T_LUNGO)
    print("      «Conferma BOM» con il documento in analisi: spento (%s)" % cta.get_attribute("title"))
    ing = [x for x in b.richieste_da(i0, "GET", r"/motore-a/bozza$") if "ingressi=1" in x["query"]]
    verifica(ing, "nessuna lettura di R1 ?ingressi=1 con il wizard aperto")
    # N4 su A: B salva la bozza (la quantità di un altro pezzo)
    b2.passo("revisione")
    b2.vista("org")
    k30 = nodo(b2, "9120030A")
    b2.scheda(k30)
    p2.click("#du-quantita")
    p2.fill("#f-qta", "3")
    b2.ok_finestra()
    expect(p2.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    n4 = p.locator("#du-wz-n4")
    expect(n4).to_contain_text("è stata salvata da", timeout=40000)
    print("      N4 su A: %s" % testo_di(n4))
    # chi ha salvato: un nome o «un altro utente», mai l'identificativo (difetto trovato da questa prova il 20/10: la
    # pagina mostrava l'utente_id di R1 ?ingressi=1)
    verifica(not re.search(r"[0-9a-f]{8}-[0-9a-f]{4}-", n4.inner_text()), "N4 mostra un identificativo invece del nome: %s" % n4.inner_text())
    expect(cta).to_be_disabled()
    print("      «Conferma BOM» con il riepilogo superato: spento (%s)" % cta.get_attribute("title"))
    p.click("#du-wz-n4-rileggi")
    expect(n4).to_have_count(0, timeout=T_LUNGO)
    # il worker vero analizza il documento: N2 (la proposta cambia) o N5 (non cambia), mai tutte e due
    w = b.d["worker"]
    env = dict(os.environ)
    env.update(w["env"])
    proc = subprocess.Popen([sys.executable] + w["arg"], cwd=w["dir"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    quale = ""
    try:
        n2, n5 = p.locator("#du-wz-n2"), p.locator("#du-wz-n5")
        fine = time.time() + 90
        while time.time() < fine and not (n2.count() or n5.count()):
            p.wait_for_timeout(500)
        verifica(n2.count() or n5.count(), "dopo l'analisi del documento né N2 né N5 su A")
        quale = "N2" if n2.count() else "N5"
        print("      %s su A: %s" % (quale, testo_di(n2 if n2.count() else n5)))
        verifica(not (n2.count() and n5.count()), "N2 e N5 insieme")
        if n2.count():
            verifica("BOM confermata" not in n2.inner_text() and "BOM registrata" not in n2.inner_text(), "N2 parla della BOM registrata")
            expect(cta).to_be_disabled()
            p.click("#du-wz-n2-apri")
            expect(n2).to_have_count(0, timeout=T_LUNGO)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=15)
        except Exception:
            proc.kill()
    b.foto("F12_wizard_notifiche.png")
    p.click("#du-wz-chiudi")
    if errori2:
        raise du.Rotto("errori nella pagina di B: %s" % errori2)
    ctx2.close()
    dati(dopo_analisi=quale, conferma_accesa_prima=acceso)


FASI = {"F1": ("apertura senza scritture (S6 ×2, S5, S9)", fase_f1),
        "F2": ("«+ Aggiungi file»: zip e due file, in analisi poi letto", fase_f2),
        "F3": ("S8: la bozza formato 3 nasce dal server", fase_f3),
        "F4": ("le modifiche della bozza", fase_f4),
        "F5": ("i file: Altri file, Scarta, i rifiuti, Riattiva", fase_f5),
        "F6": ("dopo il riavvio vero: la stessa bozza nella pagina", fase_f6),
        "F6b": ("«Usa nuovo STEP…» prima di R5: la bozza sostituita, con la presa d'atto", fase_f6b),
        "F6c": ("il secondo prodotto: S8, «Riparti dalla proposta», «Abbandona»", fase_f6c),
        "F7": ("riepilogo e «Conferma BOM» con la prima risposta persa, R6", fase_f7),
        "F8": ("R5 di nuovo: riproposto, poi 409 CHIAVE_RIUSATA", fase_f8),
        "F9": ("dopo il secondo riavvio: la Revisione e lo storico", fase_f9),
        "F10": ("il giro di sole letture", fase_f10),
        "F12": ("due sessioni: N1, N4, N2 o N5 con il wizard aperto", fase_f12)}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="")
    ap.add_argument("--sigla", default="AD")
    ap.add_argument("--password", default="prova-ad")
    ap.add_argument("--dati", required=True)
    ap.add_argument("--fase", required=True, choices=sorted(FASI))
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    expect.set_options(timeout=du.T_BREVE)
    nome, fn = FASI[a.fase]
    esito = 0
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(viewport={"width": 1440, "height": 900}, locale="it-IT")
        page = ctx.new_page()
        page.set_default_timeout(T_LUNGO)
        errori = []
        attese = []  # le risposte ≥ 400 che la fase si aspetta: (metodo, regex del percorso, stato)
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        risposte_ko = []
        page.on("response", lambda r: risposte_ko.append(r) if r.status >= 400 and not r.url.endswith("/favicon.ico") else None)
        page.on("dialog", lambda d: (errori.append("dialogo del browser inatteso (%s): %s" % (d.type, d.message)), d.dismiss()))
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=15000)
        b = BancoD1(page, a)
        b.attese = attese
        b.apri()
        inizio = time.time()
        try:
            nota = fn(b)
            print("  %-4s %-64s ESEGUITA PASSATA  (%.1fs)%s" % (a.fase, nome, time.time() - inizio, " — " + nota if nota else ""))
        except Exception as e:
            esito = 1
            print("  %-4s %-64s FALLITA  %s" % (a.fase, nome, e))
            b.foto("errore_d1_%s.png" % a.fase)
        for r in risposte_ko:
            u = r.url.split("?")[0]
            if any(r.request.method == m and re.search(rx, u) and r.status == st for m, rx, st in attese):
                continue
            try:
                corpo = r.text()[:600]
            except Exception:
                corpo = ""
            errori.append("risposta %d: %s %s %s" % (r.status, r.request.method, r.url, corpo))
        v = b.vietate()
        if v:
            errori.append("richieste verso le rotte che la pagina nuova non chiama: %s" % v)
        if errori:
            print("  errori nella pagina: %s" % errori)
            esito = 1
        browser.close()
    return esito


if __name__ == "__main__":
    sys.exit(main())
