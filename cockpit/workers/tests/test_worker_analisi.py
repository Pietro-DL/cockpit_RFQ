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
