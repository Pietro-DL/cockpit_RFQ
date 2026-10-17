"""GP-3, sotto-blocco 6 (direttiva 21 §5.1): OCR con la confidenza parola per parola (Tesseract TSV) e i caratteri
ambigui segnalati, mai corretti.

Tutto con motori e processi FINTI: su questa postazione Tesseract non ha la lingua italiana, e una lettura vera con
l'OCR e' NON ESEGUITA (lo dice il rapporto delle prove). Le regole difese: la confidenza arriva per parola; il testo
dell'OCR resta quello letto (nessuna sostituzione B/8, 0/O), con le posizioni ambigue elencate; un Tesseract senza
le lingue chieste non e' un motore (non_disponibile con il motivo); un errore del motore non fa fallire l'analisi.
"""
from __future__ import annotations

import types

import pymupdf

from contratti import TestoPDF as ModelloTestoPDF
from worker_analisi import (ConfigOCR, MotoreTesseractTSV, Parola, ambigui, analizza_file, trova_tesseract)

TSV = ("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"
       "1\t1\t0\t0\t0\t0\t0\t0\t1000\t500\t-1\t\n"
       "5\t1\t1\t1\t1\t1\t100\t200\t150\t40\t91.5\tDWG\n"
       "5\t1\t1\t1\t1\t2\t270\t200\t100\t40\t88\tNO.\n"
       "5\t1\t1\t1\t1\t3\t400\t200\t250\t40\t57.25\t1650B0\n"
       "5\t1\t1\t1\t1\t4\t700\t200\t50\t40\t-1\t\n")


def esito(stdout: bytes = b"", stderr: bytes = b"", codice: int = 0):
    return types.SimpleNamespace(returncode=codice, stdout=stdout, stderr=stderr)


def test_le_parole_del_tsv_con_la_confidenza():
    clip = pymupdf.Rect(400, 300, 800, 500)
    parole = MotoreTesseractTSV.parole_da_tsv(TSV, clip, 300)
    assert [p.testo for p in parole] == ["DWG", "NO.", "1650B0"]
    assert [p.confidenza for p in parole] == [91.5, 88.0, 57.25]
    p = parole[0]
    # pixel -> punti: 72 / 300 = 0,24, piu' l'origine della zona
    assert (round(p.x0, 2), round(p.y0, 2), round(p.x1, 2), round(p.y1, 2)) == (424.0, 348.0, 460.0, 357.6)
    assert MotoreTesseractTSV.parole_da_tsv("niente intestazione\n", clip, 300) == []
    assert MotoreTesseractTSV.parole_da_tsv("", clip, 300) == []


def test_il_motore_tsv_passa_l_immagine_e_legge_lo_stdout():
    doc = pymupdf.open()
    pag = doc.new_page(width=200, height=100)
    chiamate = []

    def esegui(argomenti, ingresso, tempo):
        chiamate.append((argomenti, ingresso[:8], tempo))
        return esito(TSV.encode())
    m = MotoreTesseractTSV("tesseract.exe", "ita+eng", 300, esegui=esegui)
    parole = m.leggi(pag, pymupdf.Rect(0, 0, 200, 100))
    assert [p.testo for p in parole] == ["DWG", "NO.", "1650B0"]
    argomenti, png, _ = chiamate[0]
    assert argomenti[:6] == ["tesseract.exe", "stdin", "stdout", "-l", "ita+eng", "--psm"] and argomenti[-1] == "tsv"
    assert png.startswith(b"\x89PNG")
    rotto = MotoreTesseractTSV("tesseract.exe", "ita+eng", 300, esegui=lambda *a: esito(stderr=b"errore", codice=1))
    try:
        rotto.leggi(pag, pymupdf.Rect(0, 0, 200, 100))
        assert False, "un'uscita diversa da 0 e' un errore del motore"
    except RuntimeError as e:
        assert "uscito con 1" in str(e)


def test_tesseract_senza_le_lingue_non_e_un_motore(tmp_path):
    exe = tmp_path / "tesseract.exe"
    exe.write_bytes(b"")
    solo_eng = lambda *a: esito(b"List of available languages (2):\neng\nosd\n")
    assert trova_tesseract(str(exe), "ita+eng", solo_eng) == ("", "Tesseract senza le lingue ita")
    entrambe = lambda *a: esito(b"List of available languages (3):\neng\nita\nosd\n")
    assert trova_tesseract(str(exe), "ita+eng", entrambe) == (str(exe), "")
    cfg = ConfigOCR(tesseract=str(exe), esegui=entrambe)
    motore, motivo = cfg.motore_pronto()
    assert isinstance(motore, MotoreTesseractTSV) and motivo == ""


def test_ambigui_segnalati_mai_corretti():
    assert ambigui("1650B0") == [{"posizione": 0, "letto": "1", "alternative": ["I", "l"]},
                                 {"posizione": 2, "letto": "5", "alternative": ["S"]},
                                 {"posizione": 3, "letto": "0", "alternative": ["O", "o"]},
                                 {"posizione": 4, "letto": "B", "alternative": ["8"]},
                                 {"posizione": 5, "letto": "0", "alternative": ["O", "o"]}]
    assert ambigui("ACME") == []


class MotoreFinto:
    """Un motore OCR finto che legge un cartiglio in basso a destra con la confidenza per parola."""
    nome = "finto (tsv)"

    def leggi(self, pag, clip):
        x, y = clip.x0 + 0.6 * clip.width, clip.y0 + 0.7 * clip.height
        return [Parola(x, y, x + 30, y + 9, "DWG", 1, 1, 1, 90.0), Parola(x + 33, y, x + 50, y + 9, "NO.", 1, 1, 2, 88.0),
                Parola(x + 60, y, x + 110, y + 9, "1650B0", 1, 1, 3, 57.0)]


def test_l_ocr_porta_parole_e_ambigui_nel_fatto(tmp_path):
    p = tmp_path / "vettoriale.pdf"
    doc = pymupdf.open()
    pag = doc.new_page(width=842, height=595)
    pag.draw_rect(pymupdf.Rect(500, 400, 800, 560))   # solo linee, nessun testo
    doc.save(str(p))
    doc.close()
    r = analizza_file(str(p), "vettoriale.pdf", ConfigOCR(motore=MotoreFinto()))
    t = r["dettagli"]["testo_pdf"]
    ModelloTestoPDF.model_validate(t)
    assert t["estraibile"] is False and t["ocr"]["stato"] == "eseguito"
    ocr = [f for f in t["frammenti"] if f["fonte"] == "ocr"]
    assert ocr and all(f["parole"] for f in ocr)
    conf = {w["testo"]: w["confidenza"] for f in ocr for w in f["parole"]}
    assert conf["1650B0"] == 57.0 and conf["DWG"] == 90.0
    assert any(a["letto"] == "B" and a["alternative"] == ["8"] for f in ocr for a in f["ambigui"])
    campo = [c for c in t["cartiglio"] if c["fonte"] == "ocr" and c["etichetta"] == "numero_disegno"]
    assert campo and campo[0]["valore"] == "1650B0"                  # il valore letto, non corretto
    assert {a["posizione"] for a in campo[0]["ambigui"]} == {0, 2, 3, 4, 5}
    # i frammenti nativi non portano parole OCR ne' ambigui
    assert all(f["parole"] == [] and f["ambigui"] == [] for f in t["frammenti"] if f["fonte"] == "nativo")


def test_tesseract_parte_senza_finestra(monkeypatch):
    # Il worker gira con pythonw: senza CREATE_NO_WINDOW ogni tesseract.exe apre una console che lampeggia.
    import os
    import subprocess

    import worker_analisi
    visti = {}

    def finto_run(argomenti, **kw):
        visti.update(kw)
        return esito(b"ok")

    monkeypatch.setattr(subprocess, "run", finto_run)
    worker_analisi._esegui_processo(["tesseract", "--list-langs"], None, 5.0)
    if os.name == "nt":
        assert visti["creationflags"] & subprocess.CREATE_NO_WINDOW
    else:
        assert "creationflags" not in visti
    assert visti["capture_output"] is True
