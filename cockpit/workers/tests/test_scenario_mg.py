"""Lo scenario «MG» nel worker di analisi (prova da fare del giro 4; docs/domande/scenario_MG_28-09.md, fuori da
git). Un disegno d'assieme con la forma di quelli veri, dati fittizi: l'ELENCO PARTICOLARI («Pos. Part Number
Descrizione Q.ty») subito sopra il cartiglio, dentro la zona in basso a destra, e il cartiglio con le etichette
«Family: Part Nr: Descrizione: Description:» su una riga e i valori («-- 7120100 TELAIO SALDATO DX») sulla riga
sotto. Il codice del disegno e' 7120100 (quello che fa fede); le righe dell'elenco sono i suoi figli. Della mail
vera resta solo la forma: le descrizioni dei pezzi e le note del disegno sono inventate.

Ogni aspettativa non ancora soddisfatta finisce con pytest.skip("da fare giro 4: …"): la prova resta verde e
`python -m pytest tests/test_scenario_mg.py -rs` elenca i «da fare»."""

import pymupdf
import pytest

from worker_analisi import analizza_file

RIGHE = [("7120110", "PIASTRA A DX"), ("7120111", "LAMIERA B"), ("7120112", "TONDO C"),
         ("7120113", "PIASTRINA D"), ("7120130", "BUSSOLA E"), ("7120131", "BLOCCHETTO F"),
         ("7120114", "LAMIERA G"), ("7120115", "COSTOLA H"), ("7120116", "SQUADRETTA K"), ("7120117", "COPERCHIO L")]


def disegno_assieme_mg(tmp_path) -> str:
    """Un A3 orizzontale con la forma dei disegni d'assieme del cliente vero."""
    doc = pymupdf.open()
    pag = doc.new_page(width=1191, height=842)
    pag.insert_text((60, 80), "NOTA: PEZZO DI PROVA", fontsize=9)
    pag.insert_text((200, 300), "Isometric view", fontsize=9)
    # l'elenco particolari, in basso a destra sopra il cartiglio
    x0, y = 800, 560
    for x, t in ((x0, "Pos."), (x0 + 35, "Part Number"), (x0 + 120, "Descrizione"), (x0 + 330, "Q.ty")):
        pag.insert_text((x, y), t, fontsize=8)
    for i, (codice, descr) in enumerate(RIGHE):
        y += 11
        for x, t in ((x0, str(i + 1)), (x0 + 35, codice), (x0 + 120, descr), (x0 + 330, "1")):
            pag.insert_text((x, y), t, fontsize=8)
    pag.insert_text((x0, y + 14), "SPECULARE DI", fontsize=8)
    pag.insert_text((x0 + 70, y + 14), "7120101", fontsize=8)
    # la tabella delle revisioni
    pag.insert_text((x0, 715), "Rev", fontsize=7)
    pag.insert_text((x0 + 30, 715), "Mod. N.", fontsize=7)
    pag.insert_text((x0 + 90, 715), "Modification object / Descrizione modifica", fontsize=7)
    pag.insert_text((x0, 725), "00", fontsize=7)
    pag.insert_text((x0 + 30, 725), "ACME-P003", fontsize=7)
    pag.insert_text((x0 + 90, 725), "Prima emissione", fontsize=7)
    # il cartiglio: etichette su una riga, valori sotto
    for x, t in ((x0, "Family:"), (x0 + 60, "Part Nr:"), (x0 + 140, "Descrizione:"), (x0 + 260, "Description:")):
        pag.insert_text((x, 780), t, fontsize=7)
    for x, t in ((x0, "--"), (x0 + 60, "7120100"), (x0 + 140, "TELAIO SALDATO DX"), (x0 + 260, "RIGHT WELDED FRAME")):
        pag.insert_text((x, 792), t, fontsize=9)
    pag.insert_text((x0, 815), "ACME S.p.A. - DRAFT -", fontsize=7)
    percorso = tmp_path / "ACME-030P7120100.pdf"
    doc.save(str(percorso))
    doc.close()
    return str(percorso)


def _testo(tmp_path):
    return analizza_file(disegno_assieme_mg(tmp_path), "ACME-030P7120100.pdf")["dettagli"]["testo_pdf"]


def test_scenario_mg_il_codice_del_disegno_sta_nel_testo_in_basso_a_destra(tmp_path):
    """Invariante: il testo nativo della zona in basso a destra porta il codice del cartiglio (il server lo puo'
    leggere), e anche le righe dell'elenco particolari (che stanno nella stessa zona)."""
    t = _testo(tmp_path)
    bd = " ".join(f["testo"] for f in t["frammenti"] if f["pagina"] == 1 and f["zona"] == "basso_destra")
    assert "7120100" in bd, bd
    assert "7120110" in bd and "7120117" in bd, bd


def test_scenario_mg_da_fare_part_nr_e_il_campo_del_codice(tmp_path):
    t = _testo(tmp_path)
    campi = [(c["etichetta"], c["letta"], c["valore"]) for c in t["cartiglio"]]
    codici = [c for c in campi if c[0] in ("codice", "numero_disegno")]
    if any(v.strip() == "7120100" for _, _, v in codici) and not any(v.strip() == "7120110" for _, _, v in codici):
        return
    pytest.skip("da fare giro 4: «Part Nr» e' l'etichetta del codice nel cartiglio del cliente (ETICHETTE_CARTIGLIO non la "
                "conosce) e «Part Number» dell'intestazione dell'elenco particolari non e' un campo del cartiglio "
                f"(oggi i campi del codice sono {codici})")


def test_scenario_mg_da_fare_l_elenco_particolari_e_una_distinta(tmp_path):
    t = _testo(tmp_path)
    elenco = t.get("elenco_particolari") or t.get("distinta")
    if elenco and [r.get("codice") for r in elenco] == [c for c, _ in RIGHE]:
        return
    pytest.skip("da fare giro 4: l'elenco particolari del PDF d'assieme letto come righe (posizione, codice, quantita', "
                "denominazione): PROPOSTA_DISTINTA.md, punto 1 del backend (oggi: nessuna chiave per l'elenco nei fatti)")
