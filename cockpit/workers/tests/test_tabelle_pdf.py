"""GP-3, sotto-blocco 1 (direttiva 21 §5.1-5.2): il ruolo del PDF (distinta o disegno con evidenza tabellare) e le
TABELLE del testo nativo, con la completezza della lettura.

Le fixture sono COSTRUITE qui, con codici ACME e testi inventati: la forma imita quella che ha portato al blocco
(una distinta di un ERP in inglese e in tedesco, un modulo di richiesta con le righe su piu' linee e un paragrafo
dopo, un disegno d'assieme con l'elenco degli elementi), mai i file veri.

Le regole che questi test difendono:
  - una distinta in inglese con QUANTITY e una in tedesco con la dieresi (STÜCKLISTE) sono distinte;
  - un disegno d'assieme con la tabella Item/Quantity resta un DISEGNO, con l'evidenza tabellare in piu': mai degradato
    a sola distinta, anche quando la scala e' scritta in inglese;
  - le righe di una tabella si estraggono da qualunque PDF con un'intestazione riconoscibile, NON dal nome del file;
  - il worker estrae le righe, non dice che cosa siano (nessun «target», nessun codice);
  - la completezza dice quando la lettura non copre il documento («verifica incompleta», non «codice assente»);
  - le colonne personali non portano il valore.
"""
from __future__ import annotations

import pymupdf

import worker_analisi
from colonne import piega, ruolo_colonna
from contratti import TestoPDF as ModelloTestoPDF  # un nome che comincia per «Test» pytest lo raccoglierebbe
from tabelle_testo import trova_tabelle
from worker_analisi import ConfigOCR, Parola, analizza_file, termini_distinta

SPENTO = ConfigOCR(modo="spento")


def pdf(tmp_path, nome: str, pagine, larghezza=842, altezza=595) -> str:
    """Un PDF con le scritte date: `pagine` e' una lista di liste di (x, y, testo, corpo)."""
    doc = pymupdf.open()
    for scritte in pagine:
        pag = doc.new_page(width=larghezza, height=altezza)
        for x, y, testo, corpo in scritte:
            pag.insert_text((x, y), testo, fontsize=corpo)
    p = tmp_path / nome
    doc.save(str(p))
    doc.close()
    return str(p)


def distinta_inglese():
    s = [(37, 40, "bill of material", 10), (48, 70, "material:", 8), (98, 70, "ACME9000", 8),
         (28, 160, "level", 8), (62, 160, "item", 8), (97, 160, "component number", 8), (188, 160, "description", 8),
         (423, 160, "documentnumber", 8), (760, 160, "quantity", 8), (794, 160, "un", 8)]
    for k in range(4):
        y = 176 + 13.3 * k
        s += [(28, y, ".1", 8), (62, y, f"00{k + 1}0", 8), (97, y, f"ACME100{k + 1}", 8), (188, y, f"bracket part {k + 1}", 8),
              (423, y, f"ACME20{k + 1}0", 8), (787, y, "1", 8), (794, y, "PC", 8)]
    # il piede della pagina, staccato: non e' una riga della tabella
    s += [(214, 260, "Acme Industries Example Street 1 Sample Town Telephone 000 000", 6)]
    return s


def distinta_tedesca():
    s = [(37, 40, "Materialstückliste", 10), (48, 70, "Material:", 8), (98, 70, "ACME9000", 8),
         (28, 160, "Stufe", 8), (62, 160, "Position", 8), (97, 160, "Komponentennummer", 8), (188, 160, "Bezeichnung", 8),
         (766, 160, "Menge", 8), (794, 160, "ME", 8)]
    for k in range(4):
        y = 176 + 13.3 * k
        s += [(28, y, ".1", 8), (62, y, f"00{k + 1}0", 8), (97, y, f"ACME100{k + 1}", 8), (188, y, f"Blech {k + 1}", 8),
              (787, y, "1", 8), (794, y, "ST", 8)]
    return s


def tabella_di(t: dict, i: int = 0) -> dict:
    return t["tabelle"][i]


def valori(riga: dict) -> dict[str, str]:
    return {c["intestazione"]: c["valore"] for c in riga["celle"]}


# ---------------------------------------------------------------- il ruolo del documento

def test_distinta_inglese_con_quantity(tmp_path):
    """Con «bill of material» e «quantity» (prima era un termine solo) e le righe della tabella, e' una distinta."""
    p = pdf(tmp_path, "BOM_ACME9000_E.pdf", [distinta_inglese()])
    r = analizza_file(p, "BOM_ACME9000_E.pdf", SPENTO)
    assert r["tipo_proposto"] == "distinta_cliente", r["dettagli"].get("evidenza_tabellare")
    ev = r["dettagli"]["evidenza_tabellare"]
    assert {"BILL OF MATERIAL", "QUANTITY"} <= set(ev["termini"]) and ev["righe_tabella"] == 4


def test_distinta_tedesca_con_la_dieresi(tmp_path):
    """«Materialstückliste» con la dieresi, «Position» e «Menge»: i termini si confrontano sul testo piegato."""
    p = pdf(tmp_path, "BOM_ACME9000_D.pdf", [distinta_tedesca()])
    r = analizza_file(p, "BOM_ACME9000_D.pdf", SPENTO)
    assert r["tipo_proposto"] == "distinta_cliente", r["dettagli"]
    assert {"STUCKLISTE", "MENGE", "POSITION"} <= set(r["dettagli"]["evidenza_tabellare"]["termini"])
    t = r["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    ruoli = [c["ruolo"] for c in tabella_di(t)["colonne"]]
    assert ruoli == ["livello", "posizione", "codice", "descrizione", "quantita", "unita"], ruoli


def test_termini_piegati_e_parole_intere():
    assert set(termini_distinta(piega("Materialstückliste Position Menge"))) == {"STUCKLISTE", "POSITION", "MENGE"}
    assert "QUANTITA" in termini_distinta(piega("QUANTITÀ"))
    # POSITION e MENGE solo come parole intere: COMPOSITION e MENGENLEHRE non contano
    assert termini_distinta(piega("COMPOSITION MENGENLEHRE")) == []


def disegno_assieme(scala: str, extra=()):
    """Un disegno d'assieme: l'elenco degli elementi (Item/Quantity) sopra il cartiglio, con l'intestazione in basso e
    le righe sopra, e il cartiglio con la scala e il numero del disegno."""
    s = [(500, 380, "PARTS LIST", 9),
         (500, 450, "ITEM", 8), (540, 450, "PART NUMBER", 8), (640, 450, "DESCRIPTION", 8), (760, 450, "QUANTITY", 8)]
    for k in range(3):
        y = 437 - 12 * k
        s += [(505, y, str(k + 1), 8), (540, y, f"ACME500{k + 1}", 8), (640, y, f"PLATE {k + 1}", 8), (770, y, "2", 8)]
    s += [(500, 500, "DRAWING NO.", 8), (580, 500, "ACME5000", 8), (680, 500, scala, 8), (720, 500, "1:5", 8)]
    s += list(extra)
    return s


def test_disegno_d_assieme_con_tabella_resta_un_disegno(tmp_path):
    """Scala in inglese («SCALE», non fra i termini del cartiglio) e numero del disegno: con la tabella Item/Quantity e
    «PARTS LIST» il file resta un disegno con evidenza tabellare, non una distinta. Le righe stanno SOPRA
    l'intestazione."""
    p = pdf(tmp_path, "ACME5000.pdf", [disegno_assieme("SCALE")])
    r = analizza_file(p, "ACME5000.pdf", SPENTO)
    assert r["tipo_proposto"] == "disegno_2d", r["dettagli"]
    assert r["dettagli"]["evidenza_tabellare"]["righe_tabella"] == 3
    assert "SCALE" in r["dettagli"]["segni_disegno"]
    tab = tabella_di(r["dettagli"]["testo_pdf"])
    assert tab["verso"] == "sopra"
    assert [valori(x)["PART NUMBER"] for x in tab["righe"]] == ["ACME5001", "ACME5002", "ACME5003"]
    assert [valori(x)["QUANTITY"] for x in tab["righe"]] == ["2", "2", "2"]


def test_disegno_con_scala_italiana_porta_anche_l_evidenza(tmp_path):
    """Con «SCALA» il disegno lo dicevano gia' i termini del cartiglio (regola 2): adesso porta anche l'evidenza."""
    p = pdf(tmp_path, "ACME5000.pdf", [disegno_assieme("SCALA")])
    r = analizza_file(p, "ACME5000.pdf", SPENTO)
    assert r["tipo_proposto"] == "disegno_2d" and r["confidenza"] == 95
    assert r["dettagli"]["evidenza_tabellare"]["righe_tabella"] == 3


def test_senza_segni_di_disegno_la_stessa_tabella_e_una_distinta(tmp_path):
    """Controprova: senza scala e senza numero del disegno, la stessa tabella con PARTS LIST e' una distinta."""
    s = [x for x in disegno_assieme("SCALE") if x[2] not in ("DRAWING NO.", "ACME5000", "SCALE", "1:5")]
    p = pdf(tmp_path, "elenco.pdf", [s])
    assert analizza_file(p, "elenco.pdf", SPENTO)["tipo_proposto"] == "distinta_cliente"


# ---------------------------------------------------------------- le righe di un modulo di richiesta

def modulo_richiesta():
    """Un modulo di richiesta: intestazione con la linea sotto, righe con la descrizione su due linee e il lotto
    sotto, e un paragrafo dopo la tabella alla stessa distanza delle righe."""
    s = [(298, 100, "RFQ", 10), (298, 140, "9000000001", 9),
         (48, 477, "Item", 9), (91, 477, "Material", 9), (147, 477, "Name", 9), (318, 477, "Inquiry", 9),
         (403, 477, "Quot.deadline", 9), (506, 477, "Delivery date", 9),
         (48, 482, "_" * 84, 9)]
    y = 512
    for k, codice in enumerate(("ACME7001", "ACME7002")):
        s += [(48, y, f"00{k + 1}0", 9), (91, y, codice, 9), (147, y, "support frame re.", 9), (338, y, "200", 9),
              (362, y, "PC", 9)]
        s += [(147, y + 12, "weld.", 9)]
        s += [(91, y + 24, "LOT 40", 9), (133, y + 24, "pcs", 9)]
        y += 48
    s += [(48, y, "Simultaneously we kindly ask you for your assistance! Further development of raw", 9)]
    return s


def test_righe_di_un_modulo_qualunque_nome(tmp_path):
    """Le righe si leggono dall'intestazione, con qualunque nome del file: «richiesta.pdf» e «Anfrage_1.pdf» danno le
    stesse righe. Le linee di continuazione sono righe a se', segnate; il paragrafo dopo non e' una riga. Il tipo non
    cambia (il worker estrae, non decide che sono prodotti richiesti)."""
    uscite = []
    for nome in ("richiesta.pdf", "Anfrage_1.pdf"):
        p = pdf(tmp_path, nome, [modulo_richiesta()], larghezza=595, altezza=842)
        r = analizza_file(p, nome, SPENTO)
        assert r["tipo_proposto"] == "da_determinare"
        t = r["dettagli"]["testo_pdf"]
        ModelloTestoPDF.model_validate(t)
        uscite.append(t["tabelle"])
    assert uscite[0] == uscite[1]
    tab = uscite[0][0]
    assert [c["ruolo"] for c in tab["colonne"]][:3] == ["posizione", "materiale", "descrizione"]
    voci = [x for x in tab["righe"] if not x["continuazione"]]
    assert [valori(x)["Material"] for x in voci] == ["ACME7001", "ACME7002"]
    assert valori(voci[0])["Name"] == "support frame re." and valori(voci[0])["Inquiry"] == "200 PC"
    cont = [x for x in tab["righe"] if x["continuazione"]]
    assert len(cont) == 4 and {"Name": "weld."} in [valori(x) for x in cont]
    assert all("Simultaneously" not in c["valore"] for x in tab["righe"] for c in x["celle"])
    # nessun codice o «target» nel fatto: solo valori grezzi con le loro colonne
    assert not any(k in tab for k in ("codici", "target", "prodotti"))
    assert all(len(c["riquadro"]) == 4 for x in tab["righe"] for c in x["celle"])


def test_completezza_con_pagine_oltre_il_tetto_e_senza_testo(tmp_path):
    """Una richiesta con piu' pagine del tetto, o con una pagina senza testo nativo, e' letta in parte: la completezza
    lo dice (stato «parziale» con il motivo), e il server non puo' dire «codice assente»."""
    p = pdf(tmp_path, "lunga.pdf", [modulo_richiesta()] + [[(50, 50, f"pagina {i}", 9)] for i in range(31)],
            larghezza=595, altezza=842)
    c = analizza_file(p, "lunga.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]
    assert c["stato"] == "parziale" and c["pagine"] == 32 and c["pagine_lette"] == worker_analisi.TABELLE_PAGINE_MAX
    assert "32" in c["motivo"]
    p = pdf(tmp_path, "con_vuota.pdf", [modulo_richiesta(), []], larghezza=595, altezza=842)
    c = analizza_file(p, "con_vuota.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]
    assert c["stato"] == "parziale" and c["pagine_senza_testo"] == 1
    p = pdf(tmp_path, "intera.pdf", [modulo_richiesta()], larghezza=595, altezza=842)
    assert analizza_file(p, "intera.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]["stato"] == "completa"
    p = pdf(tmp_path, "muta.pdf", [[]])
    assert analizza_file(p, "muta.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]["stato"] == "non_leggibile"


def test_una_tabella_che_arriva_in_fondo_alla_pagina_rende_parziale(tmp_path):
    """Le righe arrivano fino all'ultima riga della pagina 1 e c'e' una pagina 2 senza intestazione: le righe di la'
    non si leggono, e la completezza lo dice (mai «completa» con righe forse mancanti)."""
    s = [(48, 100, "Item", 9), (91, 100, "Material", 9), (318, 100, "Qty", 9)]
    for k in range(5):
        s += [(48, 130 + 14 * k, f"00{k + 1}0", 9), (91, 130 + 14 * k, f"ACME800{k}", 9), (330, 130 + 14 * k, "4", 9)]
    seguito = [(48, 60, "0060", 9), (91, 60, "ACME8005", 9), (330, 60, "4", 9)]
    p = pdf(tmp_path, "lunga.pdf", [s, seguito], larghezza=595, altezza=842)
    c = analizza_file(p, "lunga.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]
    assert c["stato"] == "parziale" and "in fondo alla pagina 1" in c["motivo"], c
    p = pdf(tmp_path, "una.pdf", [s], larghezza=595, altezza=842)
    assert analizza_file(p, "una.pdf", SPENTO)["dettagli"]["testo_pdf"]["completezza"]["stato"] == "completa"


def test_le_colonne_personali_non_portano_il_valore(tmp_path):
    s = [(40, 100, "POS.", 8), (80, 100, "CODICE", 8), (200, 100, "Q.TA", 8), (260, 100, "DISEGNATO", 8),
         (40, 114, "1", 8), (80, 114, "ACME1234", 8), (205, 114, "3", 8), (260, 114, "Mario Rossi", 8)]
    p = pdf(tmp_path, "tab.pdf", [s])
    tab = analizza_file(p, "tab.pdf", SPENTO)["dettagli"]["testo_pdf"]["tabelle"][0]
    col = {c["intestazione"]: c for c in tab["colonne"]}
    assert col["DISEGNATO"]["omessa"] is True
    cella = [c for c in tab["righe"][0]["celle"] if c["intestazione"] == "DISEGNATO"][0]
    assert cella["valore"] == "" and cella["omessa"] is True
    assert "Rossi" not in str(tab)


def test_intestazione_non_e_un_campo_del_cartiglio():
    """Una riga di cartiglio con etichette e valori («CODICE 7120010 DESCRIZIONE STAFFA») non e' un'intestazione: le
    celle di un'intestazione non hanno cifre."""
    parole = [Parola(10, 10, 40, 20, "CODICE"), Parola(45, 10, 90, 20, "7120010"),
              Parola(120, 10, 180, 20, "DESCRIZIONE"), Parola(185, 10, 220, 20, "STAFFA")]
    assert trova_tabelle(parole, 1) == ([], False)


def test_due_intestazioni_attaccate_si_separano():
    """«quantity un»: due parole vicine con un ruolo ciascuno sono due colonne."""
    parole = [Parola(10, 10, 30, 20, "item"), Parola(60, 10, 120, 20, "description"), Parola(200, 10, 231, 20, "quantity"),
              Parola(234, 10, 244, 20, "un"), Parola(12, 25, 30, 35, "0010"), Parola(60, 25, 100, 35, "plate"),
              Parola(227, 25, 231, 35, "1"), Parola(234, 25, 245, 35, "PC")]
    tab, _ = trova_tabelle(parole, 1)
    assert [c["intestazione"] for c in tab[0]["colonne"]] == ["item", "description", "quantity", "un"]
    assert valori(tab[0]["righe"][0]) == {"item": "0010", "description": "plate", "quantity": "1", "un": "PC"}


def test_ruoli_delle_colonne():
    assert ruolo_colonna("component number") == "codice"
    assert ruolo_colonna("SUPPLIER PN") == "codice"
    assert ruolo_colonna("basic material") == "materiale"
    assert ruolo_colonna("Menge") == "quantita"
    assert ruolo_colonna("Q.tà") == "quantita"
    assert ruolo_colonna("MOD. N°") == ""        # la sigla breve conta solo da sola
    assert ruolo_colonna("N°") == "posizione"
    assert ruolo_colonna("Quot.deadline") == ""


def test_tetto_delle_righe(tmp_path):
    parole = [Parola(10, 10, 30, 20, "POS."), Parola(60, 10, 100, 20, "Q.TY")]
    for k in range(10):
        y = 25 + 12 * k
        parole += [Parola(10, y, 20, y + 10, str(k + 1)), Parola(60, y, 70, y + 10, "1")]
    tab, troncato = trova_tabelle(parole, 1, {"righe_max": 4})
    assert troncato and len(tab[0]["righe"]) == 4
