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
    assert sembra_codice("6674611A") is True
    assert sembra_codice("6674611A_4") is True
    assert sembra_codice("SO 5467") is False
    assert sembra_codice("SCREENSHOT_123") is False
    assert sembra_codice("2026-09-08") is False


def test_separa_codice_rev():
    assert separa_codice_rev("6674611A_4") == ("6674611A", "4")
    assert separa_codice_rev("6674611A-REV2") == ("6674611A", "2")
    assert separa_codice_rev("6674611A") == ("6674611A", "")


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
    doc_path = corpus("6674611A_4.pdf")
    res = analizza_file(str(doc_path), "6674611A_4.pdf")
    assert res["tipo_proposto"] == "disegno_2d"
    assert res["codice"] == "6674611A"
    assert res["rev"] == "4"
    assert res["confidenza"] == 95
    assert res["fonte"] == "cartiglio"
    assert res["dettagli"].get("cartiglio") is True
