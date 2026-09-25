# -*- coding: utf-8 -*-
"""Lo ZIP della richiesta di prova (B8.7b): costruito qui, non preso dal corpus riservato.

    python zip_richiesta.py <cartella>      # scrive <cartella>/RFQ ACME 52922757.zip e stampa il percorso

Lo usa `zip_browser_test.go`. Dentro:
  52922757.stp              assieme AP214: 52922757 → 52920517 ×2 → 53011111 ×2, 52922757 → 53017189 ×4,
                            52920517 → 53017189 ×1 (condiviso)
  52922757.pdf              disegno con cartiglio (SCALA, TOLLERANZE GENERALI)
  52920517.pdf              disegno con cartiglio
  53017189 foglio 2.pdf     disegno con cartiglio, ma il nome non e' un codice: il codice lo dice una persona
  Capitolato fornitura.pdf  capitolato (tre termini)

I PDF li scrive PyMuPDF, la stessa libreria con cui il worker di analisi li legge.
"""
from __future__ import annotations

import os
import sys
import zipfile

import pymupdf

STEP = """ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('52922757.stp','2026-09-24T00:00:00',(''),(''),'','','');
FILE_SCHEMA(('AUTOMOTIVE_DESIGN {{ 1 0 10303 214 3 1 1 }}'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('prova');
{prodotti}{occorrenze}ENDSEC;
END-ISO-10303-21;
"""

PRODOTTI = [
    ("A", "52922757", "SUPPORTO COFANO"),
    ("B", "52920517", "PIASTRA"),
    ("C", "53011111", "VITE SPECIALE"),
    ("D", "53017189", "RINFORZO"),
]
# un'occorrenza per riga: le quantita' sono il numero delle righe uguali
OCCORRENZE = [("A", "B"), ("A", "B"), ("B", "C"), ("B", "C"), ("A", "D"), ("A", "D"), ("A", "D"), ("A", "D"), ("B", "D")]


def step() -> str:
    prodotti, definizione = [], {}
    for i, (etichetta, codice, descrizione) in enumerate(PRODOTTI):
        base = 100 + 10 * i
        definizione[etichetta] = f"#{base + 2}"
        prodotti.append(f"#{base}=PRODUCT('{codice}','{codice}','{descrizione}',(#1));\n")
        prodotti.append(f"#{base + 1}=PRODUCT_DEFINITION_FORMATION('','',#{base});\n")
        prodotti.append(f"#{base + 2}=PRODUCT_DEFINITION('design','',#{base + 1},#1);\n")
    occ = []
    for j, (padre, figlio) in enumerate(OCCORRENZE):
        occ.append(f"#{500 + j}=NEXT_ASSEMBLY_USAGE_OCCURRENCE('{j + 1}','pos','',{definizione[padre]},{definizione[figlio]},$);\n")
    return STEP.format(prodotti="".join(prodotti), occorrenze="".join(occ))


def pdf(righe: list[str]) -> bytes:
    doc = pymupdf.open()
    pag = doc.new_page()
    y = 72
    for r in righe:
        pag.insert_text((72, y), r, fontsize=11)
        y += 18
    dati = doc.tobytes()
    doc.close()
    return dati


def cartiglio(codice: str, titolo: str) -> bytes:
    return pdf([f"DISEGNO {codice}", titolo, "SCALA 1:2", "TOLLERANZE GENERALI ISO 2768-mK", "MATERIALE S235JR"])


def scrivi(cartella: str) -> str:
    os.makedirs(cartella, exist_ok=True)
    percorso = os.path.join(cartella, "RFQ ACME 52922757.zip")
    with zipfile.ZipFile(percorso, "w", compression=zipfile.ZIP_DEFLATED) as z:
        z.writestr("52922757.stp", step().encode("latin-1"))
        z.writestr("52922757.pdf", cartiglio("52922757", "SUPPORTO COFANO"))
        z.writestr("52920517.pdf", cartiglio("52920517", "PIASTRA"))
        z.writestr("53017189 foglio 2.pdf", cartiglio("53017189", "RINFORZO - FOGLIO 2"))
        z.writestr("Capitolato fornitura.pdf", pdf(["CAPITOLATO DI FORNITURA", "REQUISITI GENERALI", "NORME DI RIFERIMENTO"]))
    return percorso


if __name__ == "__main__":
    print(scrivi(sys.argv[1] if len(sys.argv) > 1 else "."))
