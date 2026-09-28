import json
import os
from pathlib import Path

import pytest

from worker_analisi import analizza_file, sembra_codice, separa_codice_rev

# I due test sui PDF veri hanno bisogno del corpus riservato, che non sta nel repository.
# Se manca vengono SALTATI, non passati in silenzio: un test che non verifica nulla ma si dichiara
# verde è peggio di un test assente, perché toglie il segnale senza togliere la fiducia.
# La fase 5 del piano (voce 5.5) li sostituirà con PDF generati, che il repository può contenere.
CORPUS = Path(os.environ.get("COCKPIT_CORPUS", r"C:\promatec\docs"))


def corpus(nome: str) -> Path:
    p = CORPUS / nome
    if not p.exists():
        pytest.skip(f"corpus assente: {p} (impostare COCKPIT_CORPUS)")
    return p


def test_sembra_codice():
    assert sembra_codice("1234567A") is True
    assert sembra_codice("1234567A_4") is True
    assert sembra_codice("SO 5467") is False
    assert sembra_codice("SCREENSHOT_123") is False
    assert sembra_codice("2026-09-08") is False


def test_separa_codice_rev():
    assert separa_codice_rev("1234567A_4") == ("1234567A", "4")
    assert separa_codice_rev("1234567A-REV2") == ("1234567A", "2")
    assert separa_codice_rev("1234567A") == ("1234567A", "")


def test_analisi_offerta_promatec():
    doc_path = corpus("SO 5467.pdf")
    res = analizza_file(str(doc_path), "SO 5467.pdf")
    assert res["tipo_proposto"] == "offerta_promatec"
    assert res["codice"] == ""
    assert res["rev"] == ""
    assert res["confidenza"] == 95
    assert res["fonte"] == "cartiglio"
    assert res["dettagli"].get("commerciale") is True


def test_analisi_cad_2d():
    doc_path = corpus("1234567A_4.pdf")
    res = analizza_file(str(doc_path), "1234567A_4.pdf")
    assert res["tipo_proposto"] == "disegno_2d"
    assert res["codice"] == "1234567A"
    assert res["rev"] == "4"
    assert res["confidenza"] == 95
    assert res["fonte"] == "cartiglio"
    assert res["dettagli"].get("cartiglio") is True


# ---------------------------------------------------------------- checkpoint 3R §5: PDF non e' disegno
#
# Questi PDF vengono COSTRUITI qui, non presi dal corpus riservato: e' la voce 5.5 del piano
# («test veri e contratti a due lati») applicata in anticipo, e serve perche' la regola da provare —
# «il tipo lo dice il contenuto, non l'estensione» — si prova solo variando il contenuto.

import pymupdf


def scrivi_pdf(tmp_path, nome: str, testo: str) -> str:
    doc = pymupdf.open()
    pagina = doc.new_page()
    y = 72
    for riga in testo.splitlines():
        pagina.insert_text((50, y), riga, fontsize=10)
        y += 14
    percorso = tmp_path / nome
    doc.save(str(percorso))
    doc.close()
    return str(percorso)


def test_pdf_generico_non_e_cad(tmp_path):
    """Un PDF qualunque non e' un disegno, nemmeno se si chiama come un codice.

    E' il difetto di §5: `pdf => disegno_2d` per estensione, e +20 di confidenza se il nome
    assomigliava a un codice. «1234567A.pdf» diventava un CAD al 70% mentre poteva benissimo essere
    l'offerta di un fornitore PER quel pezzo, o la conferma d'ordine.
    """
    p = scrivi_pdf(tmp_path, "1234567A.pdf", "Buongiorno,\nin allegato il documento richiesto.\nCordiali saluti")
    res = analizza_file(p, "1234567A.pdf")
    assert res["tipo_proposto"] == "da_determinare", res
    assert res["codice"] == "1234567A"  # il codice si conserva: e' un indizio utile
    assert res["confidenza"] <= 50


def test_pdf_con_cartiglio_e_disegno(tmp_path):
    """Con i termini del cartiglio, invece, il tipo si sa: e lo si sa perche' il file e' stato letto."""
    p = scrivi_pdf(tmp_path, "1234567A_4.pdf",
                   "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nPESO KG 1,340\nZONA ESENTE DA SALDATURA")
    res = analizza_file(p, "1234567A_4.pdf")
    assert res["tipo_proposto"] == "disegno_2d", res
    assert res["codice"] == "1234567A" and res["rev"] == "4"
    assert res["confidenza"] == 95
    assert res["dettagli"].get("cartiglio") is True


def test_pdf_capitolato(tmp_path):
    p = scrivi_pdf(tmp_path, "allegato_tecnico.pdf",
                   "CAPITOLATO DI FORNITURA\nREQUISITI GENERALI\nNORME DI RIFERIMENTO UNI EN ISO 9001\nPIANO DI CONTROLLO")
    res = analizza_file(p, "allegato_tecnico.pdf")
    assert res["tipo_proposto"] == "capitolato", res
    assert res["dettagli"].get("capitolato") is True


def test_pdf_distinta(tmp_path):
    righe = ["DISTINTA BASE", "POS.  CODICE        DESCRIZIONE        Q.TA"]
    for i in range(1, 8):
        righe.append(f"{i}  123456{i}A  Particolare {i}  {i * 2}")
    p = scrivi_pdf(tmp_path, "distinta.pdf", "\n".join(righe))
    res = analizza_file(p, "distinta.pdf")
    assert res["tipo_proposto"] == "distinta_cliente", res


def test_pdf_offerta_resta_offerta(tmp_path):
    p = scrivi_pdf(tmp_path, "offerta_fornitore.pdf",
                   "CONDIZIONI GENERALI DI VENDITA\nPAGAMENTO 60 GG\nINCOTERMS EXW")
    res = analizza_file(p, "offerta_fornitore.pdf")
    assert res["tipo_proposto"] == "offerta_promatec", res


def test_pdf_illeggibile_non_e_un_disegno(tmp_path):
    """Un PDF che non si apre non e' «un disegno con confidenza 40»: e' un PDF che non si apre."""
    percorso = tmp_path / "rotto.pdf"
    percorso.write_bytes(b"%PDF-1.4 questo non e' un PDF valido")
    res = analizza_file(str(percorso), "rotto.pdf")
    assert res["tipo_proposto"] == "da_determinare", res
    assert "errore_pdf" in res["dettagli"]


def test_immagine_scansionata_resta_da_determinare(tmp_path):
    """Un TIF puo' essere un disegno scansionato o un documento di trasporto: senza OCR non si sa."""
    percorso = tmp_path / "1234567A.tif"
    percorso.write_bytes(b"II*\x00")  # intestazione TIFF: basta, il file non viene letto
    res = analizza_file(str(percorso), "1234567A.tif")
    assert res["tipo_proposto"] == "da_determinare", res
    assert res["codice"] == "1234567A"


# ---------------------------------------------------------------- STEP (B8.4)
#
# `analizza_file` su uno STEP fa due letture che vanno tenute distinte: il tipo e il codice come
# sempre — ipotesi dal nome o dal primo PRODUCT — e la STRUTTURA, che e' un fatto del contenuto e non
# porta nessuna classificazione. Le prove sul parser stanno in test_step_struttura.py; qui si prova
# che l'analisi le metta tutte e due nel risultato senza confonderle.

from tests.test_step_struttura import scrivi_step


def test_uno_step_porta_la_struttura_nei_dettagli(tmp_path):
    percorso, rif = scrivi_step(tmp_path, "77722757.step", [
        ("A", "77722757", "77722757", "PRODOTTO", "B"),
        ("B", "77720517", "77720517", "PIASTRA", "1"),
    ], [("A", "B")])
    res = analizza_file(percorso, "77722757.step")
    assert res["tipo_proposto"] == "cad_3d"
    struttura = res["dettagli"]["struttura"]
    assert struttura["versione"] == 3
    assert struttura["scarti"] == {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
                                   "occorrenze_su_se_stesse": 0, "testi_troncati": 0}
    assert [n["chiave"] for n in struttura["nodi"]] == [rif["A"], rif["B"]]
    assert struttura["relazioni"][0]["qta"] == 1
    # il codice del risultato resta l'ipotesi di sempre, e non viene dalla struttura
    assert res["codice"] == "77722757" and res["fonte"] == "step"
    assert res["dettagli"]["product_step"] == "77722757"


def test_uno_step_con_un_nome_che_non_e_un_codice(tmp_path):
    """Senza codice il documento resta senza codice, ma la struttura c'e' lo stesso: e' il caso in
    cui l'ingegnere lo digita guardando i nomi dei nodi."""
    percorso, _ = scrivi_step(tmp_path, "Assem2.step", [("A", "Assem2", "Assem2", "", "")])
    res = analizza_file(percorso, "Assem2.step")
    assert res["codice"] == "" and res["fonte"] == "estensione"
    assert [n["nome_grezzo"] for n in res["dettagli"]["struttura"]["nodi"]] == ["Assem2"]


def test_uno_step_illeggibile_non_fa_fallire_l_analisi(tmp_path):
    percorso = tmp_path / "rotto.step"
    percorso.write_bytes(b"non sono uno step\x00\x01\x02")
    res = analizza_file(str(percorso), "rotto.step")
    assert res["tipo_proposto"] == "cad_3d"
    assert res["dettagli"]["struttura"]["avvisi"] == ["non e' un file STEP Part 21"]
    assert res["dettagli"]["struttura"]["nodi"] == []


def test_la_struttura_non_arriva_dai_pdf(tmp_path):
    """Solo gli STEP hanno una struttura: un PDF non deve portare un campo vuoto che sembra un grafo."""
    p = scrivi_pdf(tmp_path, "capitolato.pdf", "CONDIZIONI GENERALI\nQUALITA'")
    res = analizza_file(p, "capitolato.pdf")
    assert "struttura" not in res["dettagli"]


# ---------------------------------------------------------------- testo dei PDF, lettura strutturata (Smistamento F9)
#
# Dall'analizzatore 4 il worker riporta i FATTI del testo di un PDF in `dettagli.testo_pdf` (addendum A5.13.8,
# decisioni del 27/09 «ter»): frammenti con pagina e riquadro, i campi del probabile cartiglio come
# etichetta -> valore, i metadati senza l'autore, l'esito dell'OCR selettivo e i limiti. Nessun codice: i codici
# li cerca il server con le famiglie del cliente di ciascuna RFQ. I PDF si costruiscono qui, con pymupdf; l'OCR
# si prova con un motore FINTO, perche' Tesseract sul banco non c'e' (e dove c'e' non deve cambiare le prove).

import worker_analisi
# importato con un altro nome: un nome che comincia per «Test» pytest proverebbe a raccoglierlo come prova
from contratti import TestoPDF as ModelloTestoPDF
from worker_analisi import (OCR_PAGINE_MAX, OCR_SOGLIA_CARTIGLIO, OCR_SOGLIA_PAGINA, TESTO_PDF_CAMPI_MAX,
                            TESTO_PDF_CARATTERI_MAX, TESTO_PDF_FRAMMENTI_MAX, TESTO_PDF_FRAMMENTO_MAX,
                            TESTO_PDF_PAGINE_MAX, ConfigOCR, Parola, ocr_da_config)

VIETATI_NEL_TESTO = {"codice", "codici", "rev", "tipo", "famiglia"}


@pytest.fixture(autouse=True)
def ocr_come_sul_banco(monkeypatch):
    """Tesseract come sul banco: non c'e'. Una postazione che l'avesse non deve cambiare l'esito delle prove, e
    la configurazione predefinita riparte da zero (ricorda il motore che ha cercato)."""
    def assente(tessdata=None):
        if tessdata:
            return tessdata
        raise RuntimeError("No tessdata specified and Tesseract is not installed")
    monkeypatch.setattr(pymupdf, "get_tessdata", assente)
    monkeypatch.setattr(worker_analisi, "OCR_PREDEFINITO", ConfigOCR())


class MotoreFinto:
    """Un motore OCR finto: registra dove gli si chiede di leggere e risponde con le parole date (sulla pagina
    vista), oppure solleva. `passo` fa avanzare l'orologio finto di tanti secondi a ogni lettura."""
    nome = "finto"

    def __init__(self, risposte=None, errore: Exception | None = None, orologio=None, passo: float = 0.0):
        self.risposte = risposte or {}
        self.errore, self.orologio, self.passo = errore, orologio, passo
        self.chiamate: list[tuple[int, tuple]] = []

    def leggi(self, pag, clip):
        self.chiamate.append((pag.number + 1, tuple(round(v, 1) for v in clip)))
        if self.orologio is not None:
            self.orologio.t += self.passo
        if self.errore is not None:
            raise self.errore
        return self.risposte.get(pag.number + 1, [])


class OrologioFinto:
    def __init__(self):
        self.t = 0.0

    def __call__(self):
        return self.t


_righe_ocr = iter(range(1, 10_000))


def parole_ocr(x: float, y: float, testo: str, confidenza: float = 71.0) -> list[Parola]:
    """Le parole di una riga letta dall'OCR a partire da (x, y) sulla pagina vista, una dopo l'altra: ogni riga
    e' un blocco suo, come le da' Tesseract per righe lontane."""
    blocco, out = next(_righe_ocr), []
    for n, w in enumerate(testo.split()):
        out.append(Parola(x, y, x + 6 * len(w), y + 10, w, blocco, 0, n, confidenza))
        x += 6 * len(w) + 4
    return out


def pdf_con_cartiglio(tmp_path, nome: str, rotazione: int = 0) -> str:
    """Un disegno finto su un A4 orizzontale: una nota in alto a sinistra e il cartiglio in basso a destra, con
    le etichette e i valori in oggetti di testo separati (come li scrive un CAD). Con `rotazione` la pagina e'
    salvata in verticale con /Rotate, e il testo e' scritto dove si VEDE."""
    doc = pymupdf.open()
    if rotazione in (90, 270):
        pagina = doc.new_page(width=595, height=842)
    else:
        pagina = doc.new_page(width=842, height=595)
    pagina.set_rotation(rotazione)

    def scrivi(x, y, testo):
        # (x, y) sulla pagina vista: si riporta sulla pagina del file, e il testo si ruota con lei
        pagina.insert_text(pymupdf.Point(x, y) * pagina.derotation_matrix, testo, fontsize=9, rotate=rotazione)

    scrivi(40, 60, "NOTA 7120011 VEDERE CAPITOLATO")
    scrivi(40, 80, "vedere le note generali")
    scrivi(560, 500, "TOLLERANZE GENERALI ISO 2768-mK")
    scrivi(560, 520, "SCALA 1:2")
    scrivi(680, 520, "REV.")
    scrivi(710, 520, "B")
    scrivi(560, 540, "DISEGNO N.")
    scrivi(620, 540, "7120010")
    scrivi(560, 560, "TITOLO")
    scrivi(560, 572, "STAFFA DI SUPPORTO")
    scrivi(700, 560, "ACME")
    percorso = tmp_path / nome
    doc.save(str(percorso))
    doc.close()
    return str(percorso)


def pdf_vettoriale(pagina) -> None:
    """Una pagina di sole curve: il riquadro, la linea del cartiglio, un cerchio. Nessun testo."""
    pagina.draw_rect(pymupdf.Rect(20, 20, pagina.rect.width - 20, pagina.rect.height - 20), color=(0, 0, 0))
    pagina.draw_line(pymupdf.Point(pagina.rect.width * 0.6, pagina.rect.height * 0.8),
                     pymupdf.Point(pagina.rect.width - 20, pagina.rect.height * 0.8), color=(0, 0, 0))
    pagina.draw_circle(pymupdf.Point(300, 300), 80, color=(0, 0, 0))


def frammenti(t: dict, zona: str | None = None, fonte: str | None = None, pagina: int | None = None) -> list[dict]:
    return [f for f in t["frammenti"] if (zona is None or f["zona"] == zona) and (fonte is None or f["fonte"] == fonte)
            and (pagina is None or f["pagina"] == pagina)]


def unisci_testo(fr: list[dict]) -> str:
    return "\n".join(f["testo"] for f in fr)


def campo(t: dict, etichetta: str, fonte: str = "nativo") -> dict | None:
    return next((c for c in t["cartiglio"] if c["etichetta"] == etichetta and c["fonte"] == fonte), None)


def dentro(riquadro: list[float], zona: tuple[float, float, float, float]) -> bool:
    return (riquadro[0] >= zona[0] - 0.5 and riquadro[1] >= zona[1] - 0.5 and riquadro[2] <= zona[2] + 0.5
            and riquadro[3] <= zona[3] + 0.5)


def test_pdf_porta_il_testo_per_frammenti(tmp_path):
    """Prova 176. Il cartiglio sta nei frammenti della zona in basso a destra, con il riquadro dentro la zona; la
    nota in alto a sinistra sta in un frammento della zona `pagina`, e una riga senza cifre del resto della
    pagina non viaggia (non serve a interpretare il file). Il fatto e' valido per il contratto, non porta codici
    ne' il testo intero, e l'OCR non serviva. Il codice in testa resta quello del nome, e `fonti` lo dice."""
    p = pdf_con_cartiglio(tmp_path, "7120010_1.pdf")
    res = analizza_file(p, "7120010_1.pdf")
    assert res["tipo_proposto"] == "disegno_2d", res
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)  # la forma e' quella del contratto
    assert t["versione"] == 1 and t["estraibile"] is True
    assert t["pagine"] == 1 and t["pagine_lette"] == 1 and t["troncato"] is False
    assert t["formato_pagina1"] == [842.0, 595.0]
    bd = frammenti(t, "basso_destra")
    assert bd and all(f["pagina"] == 1 and f["fonte"] == "nativo" and f["confidenza"] is None for f in bd)
    assert all(dentro(f["riquadro"], (421, 357, 842, 595)) for f in bd), bd
    testo_bd = unisci_testo(bd)
    for atteso in ("DISEGNO N. 7120010", "SCALA 1:2", "ACME", "TOLLERANZE GENERALI", "STAFFA DI SUPPORTO"):
        assert atteso in testo_bd, (atteso, testo_bd)
    assert "NOTA" not in testo_bd and "7120011" not in testo_bd
    resto = frammenti(t, "pagina")
    assert [f["testo"] for f in resto] == ["NOTA 7120011 VEDERE CAPITOLATO"]
    assert resto[0]["riquadro"][0] == 40.0 and resto[0]["riquadro"][3] < 100
    assert "note generali" not in str(t)  # una riga senza cifre fuori dal cartiglio non e' un frammento utile
    assert t["caratteri"] > sum(len(f["testo"]) for f in t["frammenti"])
    assert t["ocr"] == {"stato": "non_necessario", "motivo": "", "motore": "", "tentativi": []}
    assert t["limiti"] == {"pagine_max": TESTO_PDF_PAGINE_MAX, "frammenti_max": TESTO_PDF_FRAMMENTI_MAX,
                           "frammento_max": TESTO_PDF_FRAMMENTO_MAX, "caratteri_max": TESTO_PDF_CARATTERI_MAX,
                           "campi_max": TESTO_PDF_CAMPI_MAX, "ocr_soglia_pagina": OCR_SOGLIA_PAGINA,
                           "ocr_soglia_cartiglio": OCR_SOGLIA_CARTIGLIO, "ocr_pagine_max": OCR_PAGINE_MAX,
                           "ocr_tempo_max_s": 60, "ocr_dpi": 300}
    # nessun codice nel fatto: ne' fra le chiavi del testo ne' fra quelle dei frammenti e dei campi
    assert not VIETATI_NEL_TESTO & set(t)
    assert all(set(f) == {"pagina", "zona", "fonte", "testo", "riquadro", "confidenza"} for f in t["frammenti"])
    assert not VIETATI_NEL_TESTO & {k for c in t["cartiglio"] for k in c}
    # in testa, il nome del file (compatibilita'), e detto
    assert res["codice"] == "7120010" and res["rev"] == "1"
    assert res["dettagli"]["fonti"] == {"tipo": "termini_pdf", "codice": "nome_file", "rev": "nome_file"}


def test_il_cartiglio_come_etichetta_e_valore(tmp_path):
    """Prova 176, i campi del cartiglio. Le etichette tipiche con il valore accanto (a destra, anche in un altro
    oggetto di testo) o nella riga sotto, con il riquadro del valore e la zona dell'etichetta. Il valore e'
    GREZZO: «7120010_1» resta «7120010_1», nessuna regola di codice (quelle sono del cliente, al server). Una
    etichetta fuori dalla zona in basso a destra si riporta lo stesso, con la sua zona."""
    doc = pymupdf.open()
    pag = doc.new_page(width=842, height=595)
    pag.insert_text((40, 120), "MATERIALE: S235JR", fontsize=9)            # fuori dalla zona del cartiglio
    pag.insert_text((500, 470), "DRAWING NO.", fontsize=9)
    pag.insert_text((580, 470), "7120010_1", fontsize=9)
    pag.insert_text((500, 500), "REVISIONE", fontsize=9)
    pag.insert_text((500, 512), "C", fontsize=9)                          # il valore nella riga sotto
    pag.insert_text((600, 500), "SCALA", fontsize=9)
    pag.insert_text((640, 500), "1:5", fontsize=9)
    pag.insert_text((500, 540), "DENOMINAZIONE", fontsize=9)
    pag.insert_text((500, 552), "STAFFA ACME DX", fontsize=9)
    pag.insert_text((700, 540), "CODICE:", fontsize=9)
    pag.insert_text((745, 540), "7120012", fontsize=9)
    percorso = tmp_path / "cartiglio.pdf"
    doc.save(str(percorso))
    doc.close()
    t = analizza_file(str(percorso), "tavola.pdf")["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    valori = {(c["etichetta"], c["valore"], c["zona"]) for c in t["cartiglio"]}
    assert valori == {("numero_disegno", "7120010_1", "basso_destra"), ("revisione", "C", "basso_destra"),
                      ("scala", "1:5", "basso_destra"), ("titolo", "STAFFA ACME DX", "basso_destra"),
                      ("codice", "7120012", "basso_destra"), ("materiale", "S235JR", "pagina")}, t["cartiglio"]
    dis = campo(t, "numero_disegno")
    assert dis["letta"] == "DRAWING NO." and dis["pagina"] == 1 and dis["fonte"] == "nativo"
    assert dis["riquadro"][0] == 580.0 and dentro(dis["riquadro"], (578, 455, 640, 475)), dis
    rev = campo(t, "revisione")
    assert rev["riquadro"][1] > 500, rev  # il valore e' nella riga sotto l'etichetta
    assert campo(t, "codice")["letta"] == "CODICE:"


def test_pdf_ruotato_il_cartiglio_resta_in_basso_a_destra(tmp_path):
    """Prova 176, pagina ruotata. Un disegno orizzontale salvato in verticale con /Rotate: PyMuPDF da' le
    coordinate delle parole sulla pagina NON ruotata, e senza normalizzarle il cartiglio finirebbe fuori
    dalla zona (e la nota in alto a sinistra, con una rotazione, dentro). Riquadri e campi sono sulla pagina
    vista."""
    for rotazione in (90, 180, 270):
        p = pdf_con_cartiglio(tmp_path, f"ruotato_{rotazione}.pdf", rotazione)
        t = analizza_file(p, f"ruotato_{rotazione}.pdf")["dettagli"]["testo_pdf"]
        bd = frammenti(t, "basso_destra")
        assert "DISEGNO N. 7120010" in unisci_testo(bd) and "ACME" in unisci_testo(bd), (rotazione, t["frammenti"])
        assert "NOTA" not in unisci_testo(bd) and "7120011" not in unisci_testo(bd), rotazione
        assert all(dentro(f["riquadro"], (421, 357, 842, 595)) for f in bd), (rotazione, bd)
        assert t["formato_pagina1"] == [842.0, 595.0], rotazione
        dis = campo(t, "numero_disegno")
        assert dis and dis["valore"] == "7120010" and dis["zona"] == "basso_destra", (rotazione, t["cartiglio"])


def test_pdf_solo_vettoriale_dice_non_estraibile(tmp_path):
    """Prova 177. Un disegno fatto solo di curve (un CAD esportato «come geometria», o una scansione) non ha
    testo: `estraibile` e' falso e non ci sono frammenti. E' la risposta, non un errore: il job riesce, il tipo
    resta da determinare e il fatto c'e' (il server dira' «il PDF non ha testo: serve l'OCR»). L'OCR si
    tenta solo li', sulla pagina intera; qui il motore non c'e', e il fatto lo dice."""
    doc = pymupdf.open()
    pdf_vettoriale(doc.new_page(width=842, height=595))
    doc.set_metadata({"title": "7120010"})
    percorso = tmp_path / "7120010.pdf"
    doc.save(str(percorso))
    doc.close()
    res = analizza_file(str(percorso), "7120010.pdf")
    assert res["tipo_proposto"] == "da_determinare", res
    assert "errore_pdf" not in res["dettagli"]
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["estraibile"] is False
    assert t["frammenti"] == [] and t["cartiglio"] == [] and t["caratteri"] == 0 and t["troncato"] is False
    assert t["pagine"] == 1 and t["pagine_lette"] == 1
    assert t["ocr"]["stato"] == "non_disponibile"
    assert [(x["pagina"], x["zona"], x["esito"]) for x in t["ocr"]["tentativi"]] == [(1, "pagina", "saltato")]
    # i metadati ci sono anche senza testo: sono l'unica cosa che il file dice di se'
    assert t["metadati"]["titolo"] == "7120010"
    assert res["codice"] == "7120010" and res["fonte"] == "nome_file"
    assert res["dettagli"]["fonti"] == {"codice": "nome_file"}

    # con un motore, l'OCR legge proprio quella pagina: i suoi frammenti sono `ocr`, `estraibile` resta falso
    motore = MotoreFinto({1: parole_ocr(600, 520, "DISEGNO N. 7120010")})
    t = analizza_file(str(percorso), "7120010.pdf", ConfigOCR(motore=motore))["dettagli"]["testo_pdf"]
    assert motore.chiamate == [(1, (0.0, 0.0, 842.0, 595.0))]
    assert t["estraibile"] is False and t["caratteri"] == 0
    assert t["ocr"]["stato"] == "eseguito" and t["ocr"]["motore"] == "finto"
    assert [(f["zona"], f["fonte"], f["testo"], f["confidenza"]) for f in t["frammenti"]] == \
        [("basso_destra", "ocr", "DISEGNO N. 7120010", 71.0)]
    assert campo(t, "numero_disegno", "ocr")["valore"] == "7120010"


def test_ocr_assente_lo_dice_senza_errore(tmp_path):
    """OCR assente (Tesseract non installato: pymupdf.get_tessdata solleva). Il PDF ha testo ma la zona in basso
    a destra no, quindi l'OCR della zona servirebbe: il fatto dice `non_disponibile` con il motivo, e il resto
    dell'analisi e' quello del testo nativo, senza errore."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    res = analizza_file(p, "7120010.pdf")
    assert res["tipo_proposto"] == "disegno_2d", res
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["estraibile"] is True and frammenti(t, "pagina")
    assert t["ocr"]["stato"] == "non_disponibile"
    assert "Tesseract" in t["ocr"]["motivo"] and "RuntimeError" in t["ocr"]["motivo"]
    assert [(x["pagina"], x["zona"], x["esito"]) for x in t["ocr"]["tentativi"]] == [(1, "basso_destra", "saltato")]
    assert not frammenti(t, fonte="ocr")


def test_ocr_che_solleva_non_ferma_l_analisi(tmp_path):
    """Un motore OCR che solleva un'eccezione (un Tesseract rotto, una lingua mancante): l'analisi finisce lo
    stesso, con il tipo e i frammenti del testo nativo, e il fatto dice `fallito` con il motivo."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    motore = MotoreFinto(errore=RuntimeError("Tesseract: lingua ita mancante"))
    res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
    assert res["tipo_proposto"] == "disegno_2d", res
    assert res["codice"] == "7120010" and "errore_pdf" not in res["dettagli"]
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert len(motore.chiamate) == 1
    assert t["ocr"]["stato"] == "fallito"
    assert t["ocr"]["motivo"] == "RuntimeError: Tesseract: lingua ita mancante"
    assert t["ocr"]["tentativi"][0]["esito"] == "fallito"
    assert "NOTA 7120011" in unisci_testo(frammenti(t, "pagina", "nativo"))

    # nemmeno un errore fuori dal motore (qui: il piano) ferma l'analisi
    def rotto(_):
        raise ValueError("piano rotto")
    orig = worker_analisi.piano_ocr
    worker_analisi.piano_ocr = rotto
    try:
        res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
    finally:
        worker_analisi.piano_ocr = orig
    assert res["tipo_proposto"] == "disegno_2d"
    assert res["dettagli"]["testo_pdf"]["ocr"]["stato"] == "fallito"
    assert "piano rotto" in res["dettagli"]["testo_pdf"]["ocr"]["motivo"]


def test_ocr_con_risultato_storto_non_ferma_l_analisi(tmp_path):
    """Correzione F9 (decisione dell'utente: «fallimento/assenza OCR non deve fallire l'intera analisi del
    PDF»). Un motore che RISPONDE ma con parole storte (coordinate None, la confidenza scritta a parole) non deve
    far fallire l'analisi: prima le parole arrivavano ai frammenti fuori dalla guardia, e il PDF tornava
    `da_determinare` con `errore_pdf` e senza `testo_pdf` (per il server: «il PDF non si apre»). Ora l'analisi
    finisce con il testo nativo, e il fatto dice `fallito` con il motivo."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    storte = [Parola(None, None, None, None, "7120010", 1, 0, 0, "alta"),
              Parola(400.0, 700.0, "x", 710.0, "7120012", 1, 0, 1, None)]
    motore = MotoreFinto({1: storte})
    res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
    assert res["tipo_proposto"] == "disegno_2d", res
    assert "errore_pdf" not in res["dettagli"]
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert len(motore.chiamate) == 1
    assert t["ocr"]["stato"] == "fallito"
    assert "risultato del motore non valido" in t["ocr"]["motivo"] and "2 parole" in t["ocr"]["motivo"]
    assert [x["esito"] for x in t["ocr"]["tentativi"]] == ["fallito"]
    assert not frammenti(t, fonte="ocr") and not [c for c in t["cartiglio"] if c["fonte"] == "ocr"]
    assert "NOTA 7120011" in unisci_testo(frammenti(t, "pagina", "nativo"))

    # parole in parte buone: le storte si scartano (e si dice quante), una confidenza che non e' un numero
    # diventa None, e quello che resta e' un'evidenza `ocr` come le altre
    misto = MotoreFinto({1: [Parola(400.0, 700.0, 460.0, 710.0, "7120010", 1, 0, 0, "alta"),
                             Parola(None, 700.0, 500.0, 710.0, "7120012", 1, 0, 1, 80.0),
                             Parola(470.0, 700.0, 490.0, 710.0, 42, 1, 0, 2, 80.0)]})
    t = analizza_file(p, "7120010.pdf", ConfigOCR(motore=misto))["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["ocr"]["stato"] == "eseguito"
    assert [(x["esito"], x["motivo"]) for x in t["ocr"]["tentativi"]] == [
        ("letto", "2 parole del motore scartate: non valide")]
    assert [(f["testo"], f["confidenza"]) for f in frammenti(t, "basso_destra", "ocr")] == [("7120010", None)]
    assert "7120012" not in str(t)


def test_ocr_con_un_motore_senza_nome_non_ferma_l_analisi(tmp_path):
    """Correzione F9, giro 2. Il nome del motore arriva nel fatto: un motore con `nome = None` (o un oggetto che
    non e' una stringa) faceva fallire la validazione di OCRPDF fuori da ogni guardia, e l'analisi intera finiva
    in `errore_pdf` senza `testo_pdf` (per il server: «il PDF non si apre»). Ora il nome si rende una stringa,
    e l'esito si valida dentro la guardia."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    for nome, atteso in ((None, "MotoreFinto"), (42, "42"), ("x" * 500, "x" * 100)):
        motore = MotoreFinto({1: parole_ocr(400, 700, "DISEGNO N. 7120010")})
        motore.nome = nome
        res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
        assert res["tipo_proposto"] == "disegno_2d" and "errore_pdf" not in res["dettagli"], (nome, res)
        t = res["dettagli"]["testo_pdf"]
        ModelloTestoPDF.model_validate(t)
        assert t["ocr"]["stato"] == "eseguito" and t["ocr"]["motore"] == atteso, (nome, t["ocr"])
        assert "NOTA 7120011" in unisci_testo(frammenti(t, "pagina", "nativo"))

    # un nome la cui conversione in stringa solleva: il nome della classe
    class NomeRotto:
        def __str__(self):
            raise RuntimeError("nome rotto")
    motore = MotoreFinto({1: parole_ocr(400, 700, "DISEGNO N. 7120010")})
    motore.nome = NomeRotto()
    t = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))["dettagli"]["testo_pdf"]
    assert t["ocr"]["stato"] == "eseguito" and t["ocr"]["motore"] == "MotoreFinto"


def test_un_esito_dell_ocr_che_non_si_valida_non_ferma_l_analisi(tmp_path, monkeypatch):
    """Correzione F9, giro 2. Se l'esito dell'OCR stesso non passa il contratto (qui: un nome che non e' una
    stringa, arrivato saltando nome_motore), la guardia lo sostituisce con `fallito` e il motivo, senza i
    frammenti OCR: l'analisi finisce con il testo nativo."""
    monkeypatch.setattr(worker_analisi, "nome_motore", lambda motore: None)
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    motore = MotoreFinto({1: parole_ocr(400, 700, "DISEGNO N. 7120010")})
    res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
    assert res["tipo_proposto"] == "disegno_2d" and "errore_pdf" not in res["dettagli"], res
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["ocr"]["stato"] == "fallito" and "non utilizzabile" in t["ocr"]["motivo"] and t["ocr"]["motore"] == ""
    assert not frammenti(t, fonte="ocr")
    assert "NOTA 7120011" in unisci_testo(frammenti(t, "pagina", "nativo"))


def test_ocr_che_rompe_i_frammenti_non_ferma_l_analisi(tmp_path, monkeypatch):
    """Correzione F9. Anche quello che viene DOPO il motore (i frammenti e i campi ricavati dalle parole
    dell'OCR, fino ai modelli del contratto) sta sotto la guardia: se si rompe, i frammenti OCR si buttano, il
    passaggio `letto` diventa `fallito` con il motivo e i frammenti nativi viaggiano com'erano."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "TOLLERANZE GENERALI ISO 2768-mK\nSCALA 1:2\nNOTA 7120011")
    originale = worker_analisi.componi_frammenti

    def rotta(parole, pagina, fonte, zona_di):
        if fonte == "ocr":
            raise TypeError("frammento rotto")
        return originale(parole, pagina, fonte, zona_di)
    monkeypatch.setattr(worker_analisi, "componi_frammenti", rotta)
    motore = MotoreFinto({1: parole_ocr(400, 700, "DISEGNO N. 7120010")})
    res = analizza_file(p, "7120010.pdf", ConfigOCR(motore=motore))
    assert res["tipo_proposto"] == "disegno_2d" and "errore_pdf" not in res["dettagli"]
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["ocr"]["stato"] == "fallito" and "frammento rotto" in t["ocr"]["motivo"]
    assert [x["esito"] for x in t["ocr"]["tentativi"]] == ["fallito"]
    assert not frammenti(t, fonte="ocr") and not [c for c in t["cartiglio"] if c["fonte"] == "ocr"]
    assert "NOTA 7120011" in unisci_testo(frammenti(t, "pagina", "nativo"))


def test_l_ocr_e_selettivo(tmp_path):
    """La logica dell'OCR selettivo, con un motore finto: le soglie e le zone.

    Pagina 1: testo, ma la zona in basso a destra vuota (< OCR_SOGLIA_CARTIGLIO) -> OCR della SOLA zona.
    Pagina 2: testo -> niente OCR. Pagine 3 e 5: solo curve; pagina 4: una parola (< OCR_SOGLIA_PAGINA) ->
    OCR della pagina intera, ma al piu' OCR_PAGINE_MAX pagine: la 5 si salta e lo si dice. I frammenti dell'OCR
    sono `fonte: ocr` con la confidenza del motore, le parole dell'OCR nella zona del cartiglio fanno i campi
    `ocr`, e i termini letti dall'OCR non cambiano il tipo (si cercano nel testo nativo)."""
    doc = pymupdf.open()
    p1 = doc.new_page(width=842, height=595)
    p1.insert_text((40, 60), "NOTA 7120011 VEDERE CAPITOLATO", fontsize=9)
    p1.insert_text((40, 80), "Buongiorno, in allegato il disegno 4500012345", fontsize=9)
    p2 = doc.new_page(width=842, height=595)
    p2.insert_text((40, 60), "pagina due con il suo testo nativo 7120012", fontsize=9)
    pdf_vettoriale(doc.new_page(width=842, height=595))
    doc.new_page(width=842, height=595).insert_text((40, 60), "FOGLIO", fontsize=9)
    pdf_vettoriale(doc.new_page(width=842, height=595))
    percorso = tmp_path / "misto.pdf"
    doc.save(str(percorso))
    doc.close()

    motore = MotoreFinto({
        1: parole_ocr(560, 520, "SCALA 1:2") + parole_ocr(560, 540, "DISEGNO N. 7120010") + parole_ocr(40, 300, "FUORI 7120099"),
        3: parole_ocr(100, 100, "PARTICOLARE 7120012", 55.0),
        4: parole_ocr(100, 100, "---"),
    })
    res = analizza_file(str(percorso), "misto.pdf", ConfigOCR(motore=motore))
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert motore.chiamate == [(1, (421.0, 357.0, 842.0, 595.0)), (3, (0.0, 0.0, 842.0, 595.0)),
                               (4, (0.0, 0.0, 842.0, 595.0))]
    assert [(x["pagina"], x["zona"], x["esito"]) for x in t["ocr"]["tentativi"]] == [
        (1, "basso_destra", "letto"), (3, "pagina", "letto"), (4, "pagina", "vuoto"), (5, "pagina", "saltato")]
    assert t["ocr"]["stato"] == "eseguito"
    assert t["estraibile"] is True and t["pagine_lette"] == 5

    bd = frammenti(t, "basso_destra")
    assert bd and all(f["fonte"] == "ocr" and f["confidenza"] == 71.0 for f in bd)
    assert "DISEGNO N. 7120010" in unisci_testo(bd)
    assert "7120099" not in str(t)  # l'OCR di una zona legge solo la zona
    assert [(f["pagina"], f["testo"], f["confidenza"]) for f in frammenti(t, "pagina", "ocr")] == \
        [(3, "PARTICOLARE 7120012", 55.0)]
    assert frammenti(t, "pagina", "nativo", 1) and frammenti(t, "pagina", "nativo", 2)
    assert campo(t, "numero_disegno", "ocr")["valore"] == "7120010"
    assert campo(t, "numero_disegno") is None  # nessun campo nativo: il cartiglio l'ha letto solo l'OCR
    # «SCALA» c'e' solo nell'OCR: non fa un disegno
    assert res["tipo_proposto"] == "da_determinare", res


def test_ocr_vuoto_scaduto_e_spento(tmp_path):
    """Un OCR che non legge niente di leggibile e' `illeggibile`; il tempo e' un budget per file (un passaggio
    partito non si interrompe, quelli dopo si saltano come `scaduto`); con `ocr = "spento"` il motore non si
    chiama nemmeno. In tutti i casi l'analisi finisce."""
    doc = pymupdf.open()
    pdf_vettoriale(doc.new_page(width=842, height=595))
    pdf_vettoriale(doc.new_page(width=842, height=595))
    percorso = tmp_path / "curve.pdf"
    doc.save(str(percorso))
    doc.close()

    vuoto = MotoreFinto({1: parole_ocr(10, 10, "~ ."), 2: []})
    t = analizza_file(str(percorso), "curve.pdf", ConfigOCR(motore=vuoto))["dettagli"]["testo_pdf"]
    assert t["ocr"]["stato"] == "illeggibile" and [x["esito"] for x in t["ocr"]["tentativi"]] == ["vuoto", "vuoto"]
    assert t["frammenti"] == []

    orologio = OrologioFinto()
    lento = MotoreFinto({1: parole_ocr(10, 10, "7120010")}, orologio=orologio, passo=61)
    t = analizza_file(str(percorso), "curve.pdf", ConfigOCR(motore=lento, orologio=orologio))["dettagli"]["testo_pdf"]
    assert len(lento.chiamate) == 1
    assert [(x["pagina"], x["esito"]) for x in t["ocr"]["tentativi"]] == [(1, "letto"), (2, "scaduto")]
    assert t["ocr"]["tentativi"][0]["secondi"] == 61
    assert t["ocr"]["stato"] == "eseguito" and "tempo dell'OCR finito" in t["ocr"]["motivo"]

    orologio = OrologioFinto()
    tutto_lento = MotoreFinto({}, orologio=orologio, passo=61)
    t = analizza_file(str(percorso), "curve.pdf", ConfigOCR(motore=tutto_lento, orologio=orologio))["dettagli"]["testo_pdf"]
    assert t["ocr"]["stato"] == "scaduto"

    spento = MotoreFinto({1: parole_ocr(10, 10, "7120010")})
    res = analizza_file(str(percorso), "curve.pdf", ConfigOCR(modo="spento", motore=spento))
    t = res["dettagli"]["testo_pdf"]
    assert spento.chiamate == [] and t["ocr"]["stato"] == "spento"
    assert [x["esito"] for x in t["ocr"]["tentativi"]] == ["saltato", "saltato"]
    assert res["tipo_proposto"] == "da_determinare"


def test_la_configurazione_dell_ocr(caplog):
    """`[analisi] ocr` di worker.toml: «automatico» se manca, «spento» a richiesta, un valore sconosciuto vale il
    default e lo si dice nel log; lingua, dpi e tessdata passano al motore."""
    assert ocr_da_config({}).modo == "automatico"
    c = ocr_da_config({"ocr": "Spento", "ocr_lingua": "ita", "ocr_dpi": 200, "ocr_tessdata": r"C:\tessdata"})
    assert (c.modo, c.lingua, c.dpi, c.tessdata) == ("spento", "ita", 200, r"C:\tessdata")
    with caplog.at_level("WARNING"):
        assert ocr_da_config({"ocr": "sempre"}).modo == "automatico"
    assert "sempre" in caplog.text
    # la cartella tessdata dichiarata si passa a pymupdf: il motore c'e' (qui, per finta: la prova non lo usa)
    motore, motivo = ConfigOCR(tessdata=r"C:\tessdata").motore_pronto()
    assert motore is not None and motivo == "" and motore.tessdata == r"C:\tessdata"
    motore, motivo = ConfigOCR().motore_pronto()
    assert motore is None and "Tesseract" in motivo


def test_pdf_illeggibile_non_porta_un_testo(tmp_path):
    """Prova 177, il rovescio. Un PDF che non si apre non e' un PDF senza testo: e' un PDF che non si e'
    letto. Niente `testo_pdf`, e resta `errore_pdf`."""
    percorso = tmp_path / "rotto.pdf"
    percorso.write_bytes(b"%PDF-1.4 questo non e' un PDF valido")
    res = analizza_file(str(percorso), "rotto.pdf")
    assert "errore_pdf" in res["dettagli"]
    assert "testo_pdf" not in res["dettagli"]


def test_pdf_metadati_senza_autore(tmp_path):
    """Prova 178. Titolo, soggetto, parole chiave, creatore e produttore viaggiano; l'autore no: e' il nome di
    una persona, non dice che cosa sia il file e i fatti restano archiviati per anni."""
    p = scrivi_pdf(tmp_path, "7120010.pdf", "SCALA 1:1")
    doc = pymupdf.open(p)
    doc.set_metadata({"title": "7120010", "subject": "Staffa ACME", "keywords": "staffa; ACME",
                      "author": "Fornitore Esempio", "creator": "CAD di prova", "producer": "PDF di prova"})
    doc.save(str(tmp_path / "con_metadati.pdf"))
    doc.close()
    res = analizza_file(str(tmp_path / "con_metadati.pdf"), "7120010.pdf")
    m = res["dettagli"]["testo_pdf"]["metadati"]
    assert m == {"titolo": "7120010", "soggetto": "Staffa ACME", "parole_chiave": "staffa; ACME",
                 "creatore": "CAD di prova", "produttore": "PDF di prova"}
    assert "Fornitore Esempio" not in str(res["dettagli"]["testo_pdf"])
    assert "author" not in m and "autore" not in m


def test_il_testo_si_tronca_al_limite(tmp_path):
    """Prova 179. Un documento lungo: viaggiano solo frammenti, ciascuno al piu' TESTO_PDF_FRAMMENTO_MAX, tutti
    insieme al piu' TESTO_PDF_CARATTERI_MAX e TESTO_PDF_FRAMMENTI_MAX, dalle prime TESTO_PDF_PAGINE_MAX pagine;
    `troncato` lo dice, i limiti viaggiano nel fatto, e il testo intero NON c'e' (il fatto resta piccolo). Il
    job non fallisce e il tipo si legge lo stesso."""
    doc = pymupdf.open()
    riga = "CAPITOLATO REQUISITI GENERALI 7120001 " + "X" * 40
    for _ in range(TESTO_PDF_PAGINE_MAX + 2):
        pagina = doc.new_page(width=595, height=842)
        for i in range(110):
            pagina.insert_text((20, 20 + i * 7.3), riga, fontsize=6)
    percorso = tmp_path / "lungo.pdf"
    doc.save(str(percorso))
    doc.close()
    res = analizza_file(str(percorso), "lungo.pdf")
    assert res["tipo_proposto"] == "capitolato", res
    t = res["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["troncato"] is True
    assert t["pagine"] == TESTO_PDF_PAGINE_MAX + 2 and t["pagine_lette"] == TESTO_PDF_PAGINE_MAX
    assert t["caratteri"] > 3 * TESTO_PDF_CARATTERI_MAX  # letti: molti di piu' di quanti ne viaggiano
    assert all(len(f["testo"]) <= TESTO_PDF_FRAMMENTO_MAX for f in t["frammenti"])
    assert any(len(f["testo"]) == TESTO_PDF_FRAMMENTO_MAX for f in t["frammenti"])
    assert len(t["frammenti"]) <= TESTO_PDF_FRAMMENTI_MAX
    assert sum(len(f["testo"]) for f in t["frammenti"]) <= TESTO_PDF_CARATTERI_MAX
    assert all(f["riquadro"] and f["pagina"] >= 1 for f in t["frammenti"])
    assert len(json.dumps(t)) < 2 * TESTO_PDF_CARATTERI_MAX
    assert t["limiti"]["caratteri_max"] == TESTO_PDF_CARATTERI_MAX and t["limiti"]["frammento_max"] == TESTO_PDF_FRAMMENTO_MAX

    # il limite sul NUMERO dei frammenti, da solo: tanti blocchi corti con una cifra, lontani fra loro (ognuno
    # e' un blocco suo), molti piu' di TESTO_PDF_FRAMMENTI_MAX ma pochi caratteri in tutto. Viaggiano
    # esattamente TESTO_PDF_FRAMMENTI_MAX frammenti, nessuno vuoto, e `troncato` lo dice.
    doc = pymupdf.open()
    for _ in range(TESTO_PDF_PAGINE_MAX):
        pagina = doc.new_page(width=595, height=842)
        for i in range(30):
            pagina.insert_text((40 + (i % 2) * 300, 30 + i * 26), f"N {i:02d} 7120001", fontsize=8)
    percorso = tmp_path / "tanti.pdf"
    doc.save(str(percorso))
    doc.close()
    t = analizza_file(str(percorso), "tanti.pdf")["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert TESTO_PDF_PAGINE_MAX * 30 > TESTO_PDF_FRAMMENTI_MAX
    assert TESTO_PDF_PAGINE_MAX * 30 * len("N 00 7120001") < TESTO_PDF_CARATTERI_MAX  # non e' il limite dei caratteri
    assert len(t["frammenti"]) == TESTO_PDF_FRAMMENTI_MAX
    assert t["troncato"] is True
    assert all(f["testo"].strip() for f in t["frammenti"])


def test_il_testo_non_dipende_dal_nome_del_file(tmp_path):
    """Prova 180. Il testo e' del CONTENUTO: lo stesso file sotto due nomi da' lo stesso `testo_pdf`, e i
    fatti si conservano per contenuto (sha256), non per nome. Il nome cambia solo il codice in testa, che e'
    detto `nome_file` (A5.13.8, P15)."""
    p = pdf_con_cartiglio(tmp_path, "originale.pdf")
    a = analizza_file(p, "7120010_1.pdf")
    b = analizza_file(p, "7120012.pdf")
    assert a["dettagli"]["testo_pdf"] == b["dettagli"]["testo_pdf"]
    assert "7120012" not in str(b["dettagli"]["testo_pdf"])
    assert (a["codice"], b["codice"]) == ("7120010", "7120012")
    assert a["dettagli"]["fonti"]["codice"] == b["dettagli"]["fonti"]["codice"] == "nome_file"


@pytest.mark.parametrize("nome,testo", [
    ("offerta.pdf", "CONDIZIONI GENERALI DI VENDITA\nPAGAMENTO 60 GG"),
    ("SO 7120001.pdf", "Buongiorno"),
    ("7120010_1.pdf", "SCALA 1:2"),
    ("specifica.pdf", "CAPITOLATO\nREQUISITI GENERALI"),
    ("7120011.pdf", "nessun termine"),
])
def test_la_fonte_di_un_pdf_e_sempre_esplicita(tmp_path, nome, testo):
    """A5.13.8: ogni ramo di analizza_pdf scrive `fonte` e `fonti`; il default «cartiglio» di
    RisultatoAnalisi non si usa piu' (il worker legge esito["fonte"])."""
    res = analizza_file(scrivi_pdf(tmp_path, nome, testo), nome)
    assert res["fonte"] in {"cartiglio", "nome_file", "estensione"}
    assert "fonti" in res["dettagli"] and "testo_pdf" in res["dettagli"]
    if res["codice"]:
        assert res["dettagli"]["fonti"]["codice"] == "nome_file"
    if nome.startswith("SO "):
        assert res["dettagli"]["fonti"] == {"tipo": "nome_file"}
