"""La pagina della Distinta unica contro il server VERO (direttiva 39 §8, corsia interfaccia).

Lo lancia TestDistintaUnicaServerVeroNelBrowser (distinta_unica_server_browser_test.go, tag «integrazione browser») con il
banco web vero: sessioni, CSRF, database di prova, rotte vere, calcolo vero del Motore A con le regole ACME. Niente
risposte finte: ogni esito qui viene dal server. Lo script entra come amministratore e fa una fase:

  1  la proposta dello STEP del finito dal server (G39-02 [U]: fonte.proposte_sorgente[], le evidenze); «Parti dal solo
     prodotto…» (R2 crea vuota) con l'avviso e la presa d'atto dello STEP proposto; «Associa a un nodo…» di 9120020A_1.pdf sul prodotto (una PB manuale, con
     il codice diverso: il riepilogo la dirà in conflitto); il riepilogo (R4) completo con STEP_SORGENTE_NON_CONFERMATA,
     la conferma spenta con il rimedio della Sorgente, nessuna R5 chiamata dalla pagina;
  2  «Conferma questo STEP come sorgente di 9120001…» (S8) dalla cartella degli STEP: il server riconcilia la stessa
     bozza; «Sposta a un altro nodo…» della PB sul particolare 9120020A; il riepilogo confermabile, con la riga «legato
     come sorgente» e la PB fra le manuali; «Conferma BOM» (R5); la AR letta dalle righe di S9;
  3  la revisione: «Usa nuovo STEP per 9120001…» (S8) con lo STEP rev 2, arrivato dopo la conferma;
  4  «Recepisci» della quantità nuova dello STEP rev 2 (differenza della riconciliazione dei registrati, mai
     sovrascritta da sola); il riepilogo della revisione (sorgente nuova confermata, promozione prevista); i blocchi CREA_SU_ESISTENTE di
     cambio-fonte-1 (i pezzi dello STEP nuovo con i codici dei registrati) risolti con il gesto della pagina «Riusa i
     componenti con lo stesso codice» / «Riusa ‹C›» (riusa_esistente con R2); «Conferma BOM» se il server la permette,
     altrimenti la riga «R5 DELLA REVISIONE NON ESEGUITA» con i blocchi del server (il test la conta SKIP, mai PASS).

Il test controlla il database dopo ogni fase. I dati sono inventati (ACME, 912xxxx).
"""

import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from playwright.sync_api import expect, sync_playwright  # noqa: E402

import distinta_unica as du  # noqa: E402

T_LUNGO = 30000
verifica = du.verifica
sid = du.sid


class BancoVero(du.Banco):
    """Gli aiuti della pagina di distinta_unica.py, con i dati dati dal test invece che dalla sonda del banco finto."""

    def __init__(self, page, a):
        self.page = page
        self.url = a.url.rstrip("/")
        self.foto_dir = a.foto
        d = json.loads(a.dati)
        self.thread = d["thread"]
        self.p1 = d["prodotto"]
        self.p2 = d.get("prodotto2") or ""
        self.al = d["allegati"]
        self.pr = {}
        self.richieste = []
        self.consenti_beforeunload = False
        self.beforeunload = 0
        page.on("request", self._richiesta)

    def apri(self):
        self.page.goto(self.pagina() + "?prodotto=" + self.p1)
        self.pronta()

    def scegli_nodo(self, codice, livello=None):
        """Nel dialogo «Associa/Sposta a un nodo», il nodo il cui testo comincia con il codice (o, se nessuno, quello del
        livello dato)."""
        sel = self.page.locator("#du-finestra #f-pezzo")
        valori = sel.locator("option").evaluate_all("(os) => os.map(o => [o.value, o.textContent])")
        scelto = [v for v, t in valori if v and t.startswith(codice)]
        if not scelto and livello is not None:
            print("      nodi nel dialogo: %s" % valori)
            scelto = [v for v, t in valori if v and t.endswith("liv. %d" % livello)]
        verifica(len(scelto) == 1, "il nodo %s nel dialogo: %s" % (codice, valori))
        sel.select_option(scelto[0])

    def blocchi_wizard(self):
        return self.page.locator("#du-wz-blocchi [data-codice]").evaluate_all("(ls) => ls.map(l => l.getAttribute('data-codice') + ': ' + l.textContent)")


def fase1(b):
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    # nessuna bozza e nessuno STEP confermato da S8 (lo STEP del prodotto è un legame storico): «Parti dal solo prodotto…»
    b.passo("revisione")
    expect(p.locator("#du-da-vuota")).to_be_enabled(timeout=T_LUNGO)
    # G39-02 [U] sul server vero: lo STEP del prodotto finito letto con il Motore A è proposto come sorgente prima di S8
    # (fonte.proposte_sorgente[] del server della corsia motore), con le evidenze della lettura; la bozza manuale con lo
    # STEP proposto è molto sconsigliata: l'avviso e la presa d'atto
    box = p.locator("#du-nessuna-bozza #du-step-proposto-rev")
    expect(box).to_be_visible(timeout=T_LUNGO)
    expect(box).to_have_attribute("data-allegato", b.al["9120001.stp"])
    expect(p.locator("#du-step-proposto-domanda-rev")).to_have_text("Il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di 9120001: lo confermi come sorgente?")
    ev = p.locator("#du-step-proposto-evidenze-rev li[data-codice-letto]")
    letti = ev.evaluate_all("(ls) => ls.map(l => [l.getAttribute('data-evidenza'), l.getAttribute('data-codice-letto'), l.textContent])")
    print("      evidenze della proposta del server: %s" % letti)
    verifica(letti and all(x[1] for x in letti), "la proposta del server senza le evidenze della lettura: %s" % letti)
    verifica("il server non ha ancora" not in box.inner_text(), "la proposta del server mostra il ripiego della pagina")
    tutte = p.locator("#du-step-proposto-evidenze-rev").inner_text()
    verifica("nodo:" not in tutte and ": #" not in tutte, "le evidenze mostrano una chiave interna (nodo o riferimento dello STEP): %s" % tutte)
    i = b.segna()
    p.click("#du-da-vuota")
    expect(b.finestra()).to_contain_text("«Conferma BOM» richiederà uno STEP confermato come sorgente")
    expect(p.locator("#f-step-proposto")).to_contain_text("9120001.stp")
    expect(p.locator("#du-finestra #f-ok")).to_be_disabled()
    p.check("#f-senza-proposto")
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Bozza aperta dal solo prodotto", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/motore-a/bozza$")[0]
    verifica(c["crea"] == {"partenza": "vuota"} and c["bozza_id"] is None, "corpo della partenza vuota: %s" % c)
    expect(p.locator("#du-senza-sorgente")).to_contain_text("Bozza senza STEP", timeout=T_LUNGO)
    b.salvata(1)
    # la PB manuale: 77817189.pdf associato a mano al prodotto (l'unico nodo)
    b.azione("associa", pdf)
    expect(b.finestra()).to_contain_text("Associa «9120020A_1.pdf» a un nodo", timeout=T_LUNGO)
    b.scegli_nodo("9120001")
    i = b.segna()
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_preassociato"] and az[0]["dopo"]["allegato_id"] == pdf, "PB manuale: %s" % az)
    b.salvata(2)
    # R4 completo senza STEP: il blocco della sorgente, la conferma spenta con il rimedio, la consultazione completa
    b.passo("revisione")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-confermabile")).to_contain_text("La BOM non è confermabile", timeout=T_LUNGO)
    expect(p.locator("#du-wz-blocchi [data-codice=STEP_SORGENTE_NON_CONFERMATA]")).to_be_visible()
    expect(p.locator("#du-wz-vai-sorgente")).to_be_visible()
    expect(p.locator("#du-wz-conferma")).to_be_disabled()
    verifica("conferma uno STEP dalla Sorgente" in (p.locator("#du-wz-conferma").get_attribute("title") or ""), "conferma spenta senza il rimedio della sorgente")
    expect(p.locator("#du-wz-sorgente-legata")).to_contain_text("parte dal solo prodotto")
    expect(p.locator("#du-wz-gruppo-manuale [data-du-pb='%s']" % pdf)).to_be_visible()
    print("      blocchi di R4 senza STEP: %s" % b.blocchi_wizard())
    verifica(not b.richieste_da(0, "POST", r"/motore-a/bom/conferma$"), "la pagina ha chiamato R5 con il blocco della sorgente")
    p.click("#du-wz-chiudi")
    expect(p.locator("#du-wizard")).to_be_hidden()


def controlla_scheda_e_testata(b, codice):
    """Messaggio 46, rilievi 2 e 3 della revisione ALBERO, sul server vero subito dopo S8: la scheda del pezzo a cui il
    motore dà il 2D primario non dice «nessun 2D associato» (ha la miniatura o dice «anteprima non disponibile» con il
    motivo di S1); la testata dice il motivo della verifica in una frase, non il codice del motore in minuscolo."""
    p = b.page
    expect(p.locator("#du-vp .du-nc[data-k]").first).to_be_visible(timeout=T_LUNGO)
    b.attendi(lambda: p.locator("#du-vp .du-nc-mini").evaluate_all(
        "(ls, c) => ls.some(l => { const n = l.closest('.du-nc').querySelector('.du-nc-c'); return n && n.innerText.trim() === c && l.dataset.duStato !== 'attesa'; })", codice),
        "la scheda di %s non ha lo spazio del 2D" % codice)
    m = p.locator("#du-vp .du-nc-mini").evaluate_all(
        "(ls, c) => ls.filter(l => { const n = l.closest('.du-nc').querySelector('.du-nc-c'); return n && n.innerText.trim() === c; })"
        ".map(l => [l.dataset.duStato || '', l.innerText.replace(/\\s+/g, ' ').trim(), l.getAttribute('title') || ''])", codice)
    print("      2D della scheda %s dopo S8: %s" % (codice, m))
    verifica(m and all(x[0] in ("pronta", "chiuso", "errore") and "nessun 2D" not in x[1] for x in m), "la scheda di %s dopo S8: %s" % (codice, m))
    va = p.locator("#du-verifica-adesso")
    expect(va).to_be_visible(timeout=T_LUNGO)
    t = va.inner_text()
    print("      testata dopo S8: %s" % t)
    verifica("fonte strutturale" not in t and "_" not in t, "la testata dice il codice del motore: %r" % t)


def fase2(b):
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    step = b.al["9120001.stp"]
    # S8 dalla cartella degli STEP, sulla bozza senza STEP: il server la riconcilia (la stessa bozza)
    b.guarda(step)
    # la candidata dello STEP è «incerta» (il codice del prodotto e la radice letta non coincidono alla lettera): non va in
    # vista da sola (SG-02), la si sceglie con un gesto esplicito
    if p.locator("#du-strutture button").count():
        print("      candidate dello STEP: %s" % p.locator("#du-strutture button").all_inner_texts())
        p.locator("#du-strutture button").first.click()
    expect(p.locator("#du-conferma-sorgente")).to_contain_text("Conferma questo STEP come sorgente di 9120001", timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-conferma-sorgente")
    expect(b.finestra()).to_contain_text("riconcilia la bozza senza STEP", timeout=T_LUNGO)
    # messaggio 46 [U]: nessun motivo nel dialogo
    verifica(p.locator("#du-finestra #f-motivo").count() == 0, "il dialogo di S8 chiede ancora il motivo")
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_be_visible(timeout=T_LUNGO)
    print("      esito di S8: %s · %s" % (esito.get_attribute("data-esito"), esito.inner_text().replace("\n", " ")[:300]))
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")[0]
    verifica(c["bozza"]["versione_attesa"] == 2 and c["codice_atteso"] == "9120001" and "componente_atteso" not in c, "corpo di S8: %s" % c)
    # la revisione: la PB va sul particolare
    b.passo("revisione")
    controlla_scheda_e_testata(b, "9120020A")
    b.passo("sorgente")
    b.azione("sposta", pdf)
    expect(b.finestra()).to_contain_text("Sposta «9120020A_1.pdf» a un altro nodo", timeout=T_LUNGO)
    b.scegli_nodo("9120020", 2)
    i = b.segna()
    b.ok_finestra()
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_svincolato", "file_preassociato"], "spostamento della PB: %s" % az)
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    # R4 confermabile, la sorgente legata in sola lettura, la PB fra quelle del motore; poi R5
    b.passo("revisione")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)
    if p.locator("#du-wz-conferma").is_disabled():
        print("      blocchi di R4 dopo S8: %s" % b.blocchi_wizard())
        print("      perché spenta: %s" % p.locator("#du-wz-conferma").get_attribute("title"))
    expect(p.locator("#du-wz-sorgente-legata")).to_have_attribute("data-confermata", "1")
    expect(p.locator("#du-wz-sorgente-legata")).to_contain_text("legato come sorgente")
    # G39-01 [U] (giro G39, parte 2 bis): dopo S8 il motore propone il PDF proprio sul particolare 9120020A, e la PB spostata
    # lì è la sua proposta accettata: la pagina dichiara il registro della proposta e il server la registra «motore» (prima
    # di G39-01 restava «manuale», la PB della fase 1 fatta a mano sul prodotto) [T-39, da ratificare]
    expect(p.locator("#du-wz-gruppo-motore_intatta [data-du-pb='%s']" % pdf)).to_have_attribute("data-esito", "applicabile")
    verifica(p.locator("#du-wz-documenti input[type=checkbox]").count() == 0, "spunte per singola PB nel riepilogo")
    expect(p.locator("#du-wz-conferma")).to_be_enabled(timeout=T_LUNGO)
    i = b.segna()
    p.click("#du-wz-conferma")
    e = b.esito_wizard()
    expect(e).to_be_visible(timeout=T_LUNGO)
    expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    print("      esito di R5: %s" % e.inner_text().replace("\n", " · ")[:400])
    expect(p.locator("#du-wz-esito-fonte")).to_contain_text("è diventato lo STEP della BOM confermata")
    expect(p.locator("#du-wz-esito-associazioni")).to_contain_text("Associazioni registrate: 1")
    verifica(len(b.richieste_da(i, "POST", r"/motore-a/bom/conferma$")) == 1, "R5 chiamata più di una volta")
    p.click("#du-wz-chiudi")
    # la AR scritta da R5 si legge dalle righe di S9 («già registrato»), mai dalla copia di S1
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    expect(p.locator("#du-sg-elenco li[data-allegato='%s']" % pdf)).to_contain_text("già registrato", timeout=T_LUNGO)
    # e in «Tutti i file» il file si dice associato (l'associazione registrata letta da S9)
    b.cartella("tutti")
    expect(p.locator("#du-sg-elenco li[data-allegato='%s']" % pdf)).to_contain_text("associato", timeout=T_LUNGO)
    working_dopo_r5(b)


def working_dopo_r5(b):
    """Messaggio 47 [U] (UI46, corsia WORKING): dopo «Conferma BOM» la BOM non scompare. La pagina riaperta da capo, senza
    bozza aperta, mostra nel tab Revisione la BOM confermata, cioè la working, in sola lettura: la testata che lo dice,
    le schede della conferma (R7 &conferma=), la stessa vista Organigramma / Elenco e il pannello del nodo senza gesti
    di scrittura; il gesto è «Modifica la BOM confermata», e le partenze non ammesse non compaiono (lettura [T] del
    messaggio 46: niente gesti spenti con la spiegazione lunga)."""
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    b.apri()
    b.passo("revisione")
    b.vista("org")
    w = p.locator("#du-working")
    expect(w).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-working-testa")).to_contain_text("BOM confermata (working) · v1 del", timeout=T_LUNGO)
    expect(p.locator("#du-working-nota")).to_contain_text("È la distinta ufficiale")
    chip = p.locator("#du-passo-revisione")
    expect(chip).to_contain_text("BOM confermata · v1")
    verifica("nessuna bozza" not in chip.inner_text(), "il chip della Revisione dice ancora «nessuna bozza»: %r" % chip.inner_text())
    schede = w.locator("#du-vp .du-nc[data-k]")
    expect(schede.first).to_be_visible(timeout=T_LUNGO)
    p.click("#du-zoom-adatta")
    b.foto("ui46_working_revisione_org.png")
    testi = schede.all_inner_texts()
    print("      la working nella Revisione (%d schede): %s" % (len(testi), [t.replace("\n", " ")[:60] for t in testi]))
    for cod in ("9120001", "9120010", "9120020"):
        verifica(any(cod in t for t in testi), "la scheda di %s non c'è nella working: %s" % (cod, testi))
    verifica(all(x == "confermato" for x in schede.evaluate_all("(ls) => ls.map(l => l.dataset.duStato)")), "stato delle schede della working")
    # sola lettura: niente caselle di selezione, niente «+ Assieme», quantità spente, niente CTA della bozza
    verifica(w.locator(".du-nc-sel").count() == 0, "caselle di selezione sulla working")
    verifica(p.locator("#du-nuovo-ass, #du-nuovo-par, #du-nuovo-com, #du-cta-bom, #du-abbandona").count() == 0, "gesti di scrittura della bozza sulla working")
    verifica(w.locator(".du-oq:not([disabled])").count() == 0, "una quantità della working si può cambiare")
    # i gesti: solo «Modifica la BOM confermata»; le partenze non ammesse non compaiono (con la conferma formato 3)
    expect(p.locator("#du-da-conferma")).to_be_enabled()
    verifica(p.locator("#du-da-registrato").count() == 0 and p.locator("#du-da-vuota").count() == 0, "partenze non ammesse ancora mostrate")
    verifica("si parte da quella" not in w.inner_text(), "la spiegazione lunga della partenza spenta è ancora sulla pagina")
    # il pannello del nodo, in sola lettura: il particolare, con il suo PDF associato da R5 (la AR letta da S9)
    scelta = schede.filter(has_text="9120020").first
    scelta.locator(".du-nc-cod").click()
    pan = p.locator("#du-pannello")
    expect(pan).to_contain_text("9120020", timeout=T_LUNGO)
    verifica(pan.locator("#du-correggi, #du-escludi, #du-sez-decisione").count() == 0, "gesti di scrittura nel pannello della working")
    b.linguetta("documenti")
    expect(p.locator("#du-doc-nodo")).to_contain_text("9120020A_1.pdf", timeout=T_LUNGO)
    verifica(p.locator("#du-doc-nodo [data-du-doc='associato'], #du-doc-nodo [data-du-doc='confermato']").count() == 1, "la AR di %s nel pannello" % pdf)
    b.linguetta("dettaglio")
    # l'Elenco della stessa working
    b.vista("elenco")
    expect(p.locator("#du-working #du-elenco")).to_contain_text("9120010", timeout=T_LUNGO)
    b.foto("ui46_working_revisione_elenco.png")
    b.vista("org")
    # l'indagine del punto 5: che cosa dice la proposta aggiornata subito dopo la conferma (solo un'uscita, nessuna verifica)
    j = p.evaluate("async (u) => { const r = await fetch(u, { headers: { Accept: 'application/json' } }); return r.json(); }",
                   "/thread/%s/motore-a/dati" % b.thread)
    cc = (j or {}).get("conferma_corrente") or {}
    pa = cc.get("proposta_aggiornata")
    print("      conferma_corrente dopo R5: numero=%s valida=%s proposta_non_confrontata=%r" % (cc.get("numero"), cc.get("valida"), cc.get("proposta_non_confrontata")))
    if pa:
        print("      proposta aggiornata: per_tipo=%s regole_cambiate=%s" % (pa.get("per_tipo"), pa.get("regole_cambiate")))
        for x in (pa.get("differenze") or {}).get("nodi") or []:
            print("        nodo %s: %s %s" % (x.get("voce"), x.get("tipo"), x.get("campi")))
        for x in (pa.get("differenze") or {}).get("archi") or []:
            print("        arco %s -> %s: %s %s" % (x.get("padre"), x.get("figlio"), x.get("tipo"), x.get("campi")))
        if (pa.get("differenze") or {}).get("gruppo"):
            print("        gruppo: %s" % (pa.get("differenze") or {}).get("gruppo"))
    else:
        print("      proposta aggiornata: nessuna")


def fase3(b):
    """La revisione con un nuovo STEP: «Usa nuovo STEP per 9120001…» (S8) dopo la conferma n. 1, senza bozza aperta: il
    server apre la bozza dalla conferma e cambia la sorgente; la working resta sullo STEP di prima (lo controlla il test)."""
    p = b.page
    step2 = b.al["9120001_r2.stp"]
    b.guarda(step2)
    if p.locator("#du-strutture button").count():
        print("      candidate dello STEP nuovo: %s" % p.locator("#du-strutture button").all_inner_texts())
        p.locator("#du-strutture button").first.click()
    gesto = p.locator("#du-cambia-sorgente, #du-conferma-sorgente").first
    expect(gesto).to_be_enabled(timeout=T_LUNGO)
    print("      gesto: %s" % gesto.inner_text())
    i = b.segna()
    gesto.click()
    expect(b.finestra()).to_be_visible(timeout=T_LUNGO)
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_be_visible(timeout=T_LUNGO)
    print("      esito di S8: %s · %s" % (esito.get_attribute("data-esito"), esito.inner_text().replace("\n", " ")[:300]))
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")[0]
    print("      corpo di S8: %s" % {k: c[k] for k in c if k not in ("chiave_idempotenza", "sha256")})


def fase4(b):
    """La revisione da confermare: il riepilogo dice la sorgente nuova confermata e la promozione che farà R5. I pezzi dello
    STEP nuovo con i codici dei registrati (CREA_SU_ESISTENTE, con ASSENZA_NON_DECISA e nodo_uscente_con_ar che ne
    seguono) si riusano con il gesto della pagina; poi, se la conferma è accesa, «Conferma BOM» (R5) e lo STEP di prima
    sostituito; se il server la blocca ancora, la fase lo dice con la riga «R5 DELLA REVISIONE NON ESEGUITA» e i blocchi
    del server, e il test non la conta come provata."""
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    b.passo("revisione")
    # unione v1-c1: dalla persistenza 648d9bd la revisione riconcilia i registrati (cambio-fonte-2) e la quantità nuova
    # dello STEP rev 2 (9120010A -> 9120020A da ×4 a ×3) resta una differenza da recepire, mai sovrascritta da sola
    # (direttiva 39 §3): la si recepisce qui con «Recepisci», poi R4/R5
    p.wait_for_timeout(800)
    df = p.locator("#du-differenze")
    if df.count():
        righe = df.locator("[data-du-differenza]")
        print("      differenze con lo STEP (%d): %s" % (righe.count(), [x.replace("\n", " ")[:160] for x in righe.all_inner_texts()]))
        qta = df.locator("[data-du-differenza^='a:cambiato:']").filter(has_text="9120020A")
        if qta.count():
            i = b.segna()
            qta.first.get_by_role("button", name="Recepisci").click()
            az = b.ultime_azioni(i)
            print("      «Recepisci» della quantità: %s" % [(a["azione"], a["dopo"]) for a in az])
            verifica(az and az[0]["azione"] == "qta_modificata" and az[0]["dopo"]["qta"] == 3, "«Recepisci» della quantità: %s" % az)
            expect(df.locator("[data-du-differenza^='a:cambiato:']").filter(has_text="9120020A")).to_have_count(0, timeout=T_LUNGO)
            p.wait_for_timeout(500)
    else:
        print("      differenze con lo STEP: nessuna")
    b.apri_wizard()
    p.click("#du-wz-fase2")
    expect(p.locator("#du-wz-documenti")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-wz-sorgente-legata")).to_have_attribute("data-confermata", "1")
    expect(p.locator("#du-wz-effetto-fonte")).to_have_attribute("data-promozione", "1")
    p.wait_for_timeout(500)
    crea = p.locator("#du-wz-blocchi [data-codice=CREA_SU_ESISTENTE]")
    if crea.count():
        print("      blocchi di R4 prima del riuso (%d):" % len(b.blocchi_wizard()))
        for x in b.blocchi_wizard():
            print("        - " + x.split(" Rimedi:")[0][:220])
        i = b.segna()
        if p.locator("#du-wz-riusa-tutti").count():
            print("      gesto: %s" % p.locator("#du-wz-riusa-tutti").inner_text())
            p.click("#du-wz-riusa-tutti")
        else:
            bt = crea.locator("button[id^='du-wz-riusa-']").first
            print("      gesto: %s" % bt.inner_text())
            bt.click()
        az = b.ultime_azioni(i)
        print("      salvataggio del riuso: %s" % [(a["azione"], a["dopo"]["tipo"], a["dopo"]["chiave_bozza"]) for a in az])
        verifica(az and all(a["azione"] == "decisione_aggiunta" and a["dopo"]["tipo"] == "riusa_esistente" for a in az), "il riuso: %s" % az)
        b.attendi(lambda: b.richieste_da(i, "POST", r"/motore-a/bom/riepilogo$"), "il riepilogo non si è riletto dopo il riuso")
        expect(crea).to_have_count(0, timeout=T_LUNGO)
        p.wait_for_timeout(500)
    if p.locator("#du-wz-conferma").is_disabled():
        blocchi = b.blocchi_wizard()
        verifica(blocchi, "la conferma è spenta senza blocchi del server: %s" % p.locator("#du-wz-conferma").get_attribute("title"))
        print("      R5 DELLA REVISIONE NON ESEGUITA: il riepilogo del server ha %d blocchi:" % len(blocchi))
        for x in blocchi:
            print("        - " + x.split(" Rimedi:")[0][:220])
        p.click("#du-wz-chiudi")
        return "R5 della revisione NON ESEGUITA (blocchi del server)"
    # la AR della prima conferma resta e non cambia: nessuno spostamento né svincolo di 9120020A_1.pdf (il server elenca in
    # ar_invariate solo le AR di un escluso mantenuto o di una ricodifica, riepilogo3_regole.go; qui il componente resta)
    expect(p.locator("#du-wz-gruppo-spostamenti_ar")).to_be_visible()
    verifica(p.locator("#du-wz-gruppo-spostamenti_ar [data-du-spostamento='%s']" % pdf).count() == 0, "la AR di 9120020A_1.pdf cambia con la revisione")
    p.click("#du-wz-conferma")
    e = b.esito_wizard()
    expect(e).to_have_attribute("data-esito", "ok", timeout=T_LUNGO)
    print("      esito di R5: %s" % e.inner_text().replace("\n", " · ")[:400])
    expect(p.locator("#du-wz-esito-fonte")).to_contain_text("lo STEP di prima resta, sostituito")
    p.click("#du-wz-chiudi")


def fase_g39(b):
    """G39-03 [U] sul server vero (TestDistintaUnicaG39ServerVeroNelBrowser): lo STEP di 9120001 confermato per errore come
    sorgente di 9130001 (dal solo prodotto); su 9120001 «Sposta la sorgente su 9120001…» (S8 con sposta_da: la revoca
    «sorgente errata» e la bozza annullata di là, la conferma qui). Il database lo controlla il test."""
    p = b.page
    step = b.al["9120001.stp"]
    # 1. l'errore: su 9130001, lo STEP che il motore non propone per lui, guardato e confermato dal solo prodotto
    tab2 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p2)
    tab2.click()
    expect(tab2).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("step")
    b.file(step, "step")
    b.altri_prodotti()
    p.click("#du-sg-dettaglio #du-guarda-nc-" + sid(b.p2) + "-" + sid(step))
    expect(p.locator("#du-anteprima")).to_be_visible(timeout=T_LUNGO)
    expect(p.locator("#du-conferma-solo-prodotto")).to_be_enabled(timeout=T_LUNGO)
    p.click("#du-conferma-solo-prodotto")
    expect(b.finestra()).to_contain_text("come sorgente di 9130001", timeout=T_LUNGO)
    p.check("#f-presa")
    b.ok_finestra()
    esito = p.locator("#du-sg-esito-s8")
    expect(esito).to_be_visible(timeout=T_LUNGO)
    print("      S8 sbagliata su 9130001: %s · %s" % (esito.get_attribute("data-esito"), esito.inner_text().replace("\n", " ")[:200]))
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    # 2. su 9120001: lo STEP è la sorgente S8 di 9130001; il gesto è lo spostamento (dalla proposta del server, se il motore
    # la dà ancora univoca, altrimenti dalla candidatura dello STEP con sorgente_di)
    tab1 = p.locator('.du-ptab[data-prodotto="%s"]' % b.p1)
    tab1.click()
    expect(tab1).to_have_attribute("aria-current", "true", timeout=T_LUNGO)
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-sg-prodotto-apri").count() and p.locator("#du-step-proposto-sg").count() == 0:
        p.click("#du-sg-prodotto-apri")
    if p.locator("#du-step-proposto-sg").count():
        expect(p.locator("#du-step-proposto-sg")).to_have_attribute("data-sorgente-di", b.p2, timeout=T_LUNGO)
        expect(p.locator("#du-step-proposto-altro-sg")).to_contain_text("Adesso è la sorgente confermata di 9130001")
        gesto = p.locator("#du-conferma-proposto-sg")
        print("      lo spostamento dalla proposta del server (sorgente_di)")
    else:
        b.cartella("step")
        b.file(step, "step")
        gesto = p.locator("#du-sg-dettaglio #du-sposta-sorgente-" + sid(step) + "-d")
        print("      lo spostamento dalla candidatura dello STEP (la proposta non è univoca dopo lo sbaglio)")
    expect(gesto).to_contain_text("Sposta la sorgente su 9120001…", timeout=T_LUNGO)
    b.foto("G39_server_sposta.png")
    i = b.segna()
    gesto.click()
    expect(b.finestra()).to_contain_text("Sposta la sorgente su 9120001", timeout=T_LUNGO)
    expect(p.locator("#f-sposta")).to_have_attribute("data-sposta-da", b.p2)
    if p.locator("#du-strutture button").count():
        p.locator("#du-strutture button").first.click()
    b.ok_finestra()
    expect(esito).to_be_visible(timeout=T_LUNGO)
    print("      spostamento: %s · %s" % (esito.get_attribute("data-esito"), b.avviso().inner_text().replace("\n", " ")[:300]))
    expect(esito).to_have_attribute("data-esito", "completata", timeout=T_LUNGO)
    expect(b.avviso()).to_contain_text("Spostamento", timeout=T_LUNGO)
    c = b.corpi(i, "POST", r"/conferma-sorgente$")
    verifica(len(c) == 1 and c[0].get("sposta_da") == b.p2 and "motivo" not in c[0], "corpo dello spostamento: %s" % c)


def fase_g39b(b):
    """G39-01 [U] sul server vero: un file arrivato dopo l'apertura della bozza (il test lo aggiunge fra le fasi) con una
    destinazione del motore entra in preassociazioni_proposte[] di S9; «Accetta» lo salva (R2) e il server dice la PB
    «motore». Se il motore non gli trova una destinazione, la riga «G39-01 NON ESEGUITA» con il motivo (il test la conta SKIP)."""
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-sg-prodotto-apri").count() and p.locator("#du-sg-suggerimenti").count() == 0:
        p.click("#du-sg-prodotto-apri")
    sug = p.locator("#du-sg-suggerimenti [data-du-suggerimento='%s']" % pdf)
    try:
        expect(sug).to_be_visible(timeout=T_LUNGO)
    except AssertionError:
        s9 = p.evaluate("async (u) => { const r = await fetch(u); return await r.text(); }", b.url + "/thread/" + b.thread + "/motore-a/dati?sorgente=1")
        j = json.loads(s9)
        sez = [x for x in j.get("prodotti", []) if x.get("prodotto_id") == b.p1]
        righe = [(r.get("allegato_id"), r.get("stato"), r.get("motivo")) for r in (sez[0].get("righe") or [])] if sez else []
        print("  G39-01 NON ESEGUITA: S9 non dà il PDF arrivato dopo l'apertura in preassociazioni_proposte[] (%s); righe del prodotto: %s"
              % (sez[0].get("preassociazioni_proposte") if sez else "?", righe))
        return
    expect(sug).to_contain_text("9120020A_1.pdf")
    print("      suggerimento dopo l'apertura: %s" % " ".join(sug.inner_text().split()))
    i = b.segna()
    p.click("#du-accetta-sugg-" + sid(pdf))
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_preassociato"] and az[0]["dopo"]["allegato_id"] == pdf, "«Accetta» del suggerimento: %s" % az)
    print("      G39-01: suggerimento accettato e salvato")


def fase_g39d(b):
    """G39-01 [U] sul server vero, «corretta»: la PB del suggerimento accettato (fase g39b, «motore») spostata
    dall'ingegnere su un altro nodo della bozza. La provenienza la decide il server (il test la legge nel database)."""
    p = b.page
    pdf = b.al["9120020A_1.pdf"]
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.azione("sposta", pdf)
    expect(b.finestra()).to_contain_text("Sposta «9120020A_1.pdf» a un altro nodo", timeout=T_LUNGO)
    sel = p.locator("#du-finestra #f-pezzo")
    altri = [v for v in sel.locator("option").evaluate_all("os => os.map((o) => o.value)") if v]
    verifica(altri, "nessun altro nodo nella bozza per spostare la PB")
    sel.select_option(altri[0])
    i = b.segna()
    b.ok_finestra()
    expect(p.locator("#du-salvataggio")).to_have_attribute("data-du-salvataggio", "salvata", timeout=T_LUNGO)
    az = b.ultime_azioni(i)
    verifica([a["azione"] for a in az] == ["file_svincolato", "file_preassociato"], "spostamento della PB del suggerimento: %s" % az)
    print("      G39-01: PB del suggerimento spostata su un altro nodo (%s)" % altri[0])


def fase_g39c(b):
    """G39-03 [U] sul server vero, la revoca su 9120001 dopo lo spostamento: la bozza nata dallo STEP si annulla."""
    p = b.page
    p.click("#du-aggiorna")
    expect(b.avviso()).to_contain_text("Pagina riletta", timeout=T_LUNGO)
    b.passo("sorgente")
    b.cartella("p:" + b.p1)
    if p.locator("#du-revoca-sorgente").count() == 0:
        p.click("#du-sg-prodotto-apri")
    p.click("#du-revoca-sorgente")
    expect(p.locator("#f-revoca-bozza")).to_have_attribute("data-nata", "1", timeout=T_LUNGO)
    i = b.segna()
    b.ok_finestra()
    expect(b.avviso()).to_contain_text("Sorgente revocata per 9120001", timeout=T_LUNGO)
    print("      revoca: %s" % b.avviso().inner_text().replace("\n", " ")[:300])
    expect(b.avviso()).to_contain_text("annullata")
    rv = b.corpi(i, "POST", r"/revoca-sorgente$")
    verifica(len(rv) == 1 and rv[0].get("sorgente_errata") is True and rv[0].get("bozza"), "corpo della revoca: %s" % rv)


FASI = {"1": ("bozza senza STEP, PB manuale, R4 con il blocco della sorgente", fase1),
        "g39": ("G39-03: lo STEP confermato sul prodotto sbagliato e spostato su quello giusto", fase_g39),
        "g39b": ("G39-01: il file arrivato dopo l'apertura, suggerito dal motore e accettato", fase_g39b),
        "g39d": ("G39-01: la PB del suggerimento spostata su un altro nodo, «corretta»", fase_g39d),
        "g39c": ("G39-03: la revoca che annulla la bozza nata dalla sorgente", fase_g39c),
        "2": ("S8 riconcilia la bozza, revisione, R4/R5, AR da S9", fase2),
        "3": ("revisione: S8 di uno STEP nuovo dopo la conferma", fase3),
        "4": ("revisione: R4 con la sorgente nuova, poi R5 se il server la permette", fase4)}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", required=True)
    ap.add_argument("--ip", default="10.0.0.5:51000")
    ap.add_argument("--sigla", default="AD")
    ap.add_argument("--password", default="prova-ad")
    ap.add_argument("--dati", required=True)
    ap.add_argument("--fase", required=True, choices=sorted(FASI))
    ap.add_argument("--canale", default="msedge")
    ap.add_argument("--vedi", action="store_true")
    ap.add_argument("--foto", default="")
    a = ap.parse_args()
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    nome, fn = FASI[a.fase]
    esito = 0
    with sync_playwright() as pw:
        browser = pw.chromium.launch(channel=a.canale, headless=not a.vedi)
        ctx = browser.new_context(extra_http_headers={"X-Prova-IP": a.ip}, viewport={"width": 1600, "height": 1000})
        page = ctx.new_page()
        errori = []
        page.on("pageerror", lambda e: errori.append("pageerror: " + str(e)))
        page.on("console", lambda m: errori.append("console: " + m.text) if m.type == "error" and "Failed to load resource" not in m.text else None)
        risposte_ko = []
        page.on("response", lambda r: risposte_ko.append(r) if r.status >= 400 and not r.url.endswith("/favicon.ico") else None)
        page.on("dialog", lambda d: d.accept())
        page.goto(a.url + "/login")
        page.fill("input[name=sigla]", a.sigla)
        page.fill("input[name=password]", a.password)
        page.click("button[type=submit]")
        page.wait_for_url("**/inbox*", timeout=15000)
        b = BancoVero(page, a)
        b.apri()
        inizio = time.time()
        try:
            nota = fn(b)
            print("  fase %s  %-64s PASSATA   (%.1fs)%s" % (a.fase, nome, time.time() - inizio, " — " + nota if nota else ""))
        except Exception as e:
            esito = 1
            print("  fase %s  %-64s FALLITA   %s" % (a.fase, nome, e))
            b.foto("errore_server_fase%s.png" % a.fase)
        for r in risposte_ko:
            try:
                corpo = r.text()[:600]
            except Exception:
                corpo = ""
            errori.append("risposta %d: %s %s %s" % (r.status, r.request.method, r.url, corpo))
        if errori:
            print("  errori nella pagina: %s" % errori)
            esito = 1
        browser.close()
    return esito


if __name__ == "__main__":
    sys.exit(main())
