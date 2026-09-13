import os
from pathlib import Path
from worker_analisi import analizza_file, sembra_codice, separa_codice_rev


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
    doc_path = Path(r"C:\promatec\docs\SO 5467.pdf")
    if doc_path.exists():
        res = analizza_file(str(doc_path), "SO 5467.pdf")
        assert res["tipo_proposto"] == "offerta_promatec"
        assert res["codice"] == ""
        assert res["rev"] == ""
        assert res["confidenza"] == 95
        assert res["fonte"] == "cartiglio"
        assert res["dettagli"].get("commerciale") is True


def test_analisi_cad_2d():
    doc_path = Path(r"C:\promatec\docs\6674611A_4.pdf")
    if doc_path.exists():
        res = analizza_file(str(doc_path), "6674611A_4.pdf")
        assert res["tipo_proposto"] == "disegno_2d"
        assert res["codice"] == "6674611A"
        assert res["rev"] == "4"
        assert res["confidenza"] == 95
        assert res["fonte"] == "cartiglio"
        assert res["dettagli"].get("cartiglio") is True
