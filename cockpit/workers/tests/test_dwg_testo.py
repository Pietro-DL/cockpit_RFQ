"""GP-3, sotto-blocco 4 (direttiva 21 §5.1): il testo dei DWG R11/R12 (AC1009).

I DWG sono COSTRUITI qui byte per byte, con lo stesso schema che il lettore verifica sui file veri (intestazione,
tabella dei blocchi, entita' con l'intestazione comune e i campi propri, CRC finale): codici ACME e testi inventati.

Le regole difese: le stesse unita' Testo del DXF (stessa regola del cartiglio, stesse tabelle, stessi INSERT annidati);
ogni campo ha la sorgente nel file (offset dell'entita'); due numeri impilati sotto la stessa etichetta si riportano
tutti e due; le altre versioni DWG sono «non_supportato» con la versione, mai un successo vuoto; un'entita' che non
torna con la sua lunghezza non si indovina (non_decodificate, parziale); i nomi di persona non escono.
"""
from __future__ import annotations

import json
import math
import struct

from contratti import TestoDWG as ModelloTestoDWG  # un nome che comincia per «Test» pytest lo raccoglierebbe
from dwg_testo import VERSIONE_TESTO_DWG, leggi_testo_dwg, leggi_testo_dwg_da_byte
from worker_analisi import analizza_file


def tv(s: str) -> bytes:
    b = s.encode("cp1252")
    return struct.pack("<H", len(b)) + b


def entita(tipo: int, corpo: bytes, opts: int = 0, colore: int | None = 1, ltype: int | None = None,
           elev: float | None = None, handle: bytes | None = b"\x01", extra: int | None = None,
           cancellata: bool = False, flag_extra: int = 0, eed: bytes = b"") -> bytes:
    """Un'entita' R12: tipo, flag, lunghezza, layer, opzioni, i campi facoltativi dell'intestazione, il corpo, il CRC."""
    flag = flag_extra
    testa = b""
    if extra is not None:
        flag |= 0x40
        testa += bytes([extra])
    if colore is not None:
        flag |= 0x01
        testa += bytes([colore])
    if ltype is not None:
        flag |= 0x02
        testa += struct.pack("<H", ltype)
    if elev is not None:
        flag |= 0x04
        testa += struct.pack("<d", elev)
    if handle is not None:
        flag |= 0x20
        testa += bytes([len(handle)]) + handle
    testa += eed
    if extra is not None and extra & 0x04:
        testa += struct.pack("<H", 1)
    corpo_tot = struct.pack("<HH", 1, opts) + testa + corpo + b"\x00\x00"
    size = 4 + len(corpo_tot)
    return bytes([tipo | (0x80 if cancellata else 0), flag]) + struct.pack("<H", size) + corpo_tot


def text(x: float, y: float, h: float, s: str, rot: float | None = None, **kw) -> bytes:
    opts = 0x08 | (0x01 if rot is not None else 0)
    corpo = struct.pack("<ddd", x, y, h) + tv(s)
    if rot is not None:
        corpo += struct.pack("<d", math.radians(rot))
    corpo += b"\x01"  # stile
    return entita(7, corpo, opts, **kw)


def attrib(x: float, y: float, h: float, valore: str, tag: str) -> bytes:
    return entita(16, struct.pack("<ddd", x, y, h) + tv(valore) + tv(tag) + b"\x00", 0)


def insert(indice: int, x: float, y: float, sx: float = 1.0, sy: float = 1.0, rot: float = 0.0, **kw) -> bytes:
    corpo = struct.pack("<Hdd", indice, x, y)
    opts = 0
    for bit, v in ((1, sx), (2, sy), (4, math.radians(rot))):
        if v != (1.0 if bit < 4 else 0.0):
            opts |= bit
            corpo += struct.pack("<d", v)
    return entita(14, corpo, opts, **kw)


def blocco(base=(0.0, 0.0)) -> bytes:
    return entita(12, struct.pack("<dd", *base), 0, colore=None, handle=b"\x02\xf3", extra=4)


def fine_blocco() -> bytes:
    return entita(13, b"", 0, colore=None, handle=b"\x02\xf4", extra=4)


def dwg(entita_: list[bytes], blocchi: list[tuple[str, list[bytes]]] = (), versione: bytes = b"AC1009") -> bytes:
    """Un DWG R12: intestazione (0x60 byte), ENTITIES, BLOCKS, tabella dei blocchi (voci da 45 byte)."""
    ent = b"".join(entita_)
    ent_s = 0x60
    blk_s = ent_s + len(ent)
    corpo_blk, offset = b"", []
    for _, contenuto in blocchi:
        offset.append(len(corpo_blk))
        corpo_blk += blocco() + b"".join(contenuto) + fine_blocco()
    tab = blk_s + len(corpo_blk)
    voci = b""
    for (nome, _), off in zip(blocchi, offset):
        voci += b"\x00" + nome.encode().ljust(32, b"\x00") + b"\x00\x00" + struct.pack("<I", off | 0x40000000) + b"\x00" * 6
    testa = bytearray(0x60)
    testa[0:6] = versione
    struct.pack_into("<IIIIII", testa, 0x14, ent_s, blk_s, blk_s, len(corpo_blk) | 0x40000000, 0, 0x80000000)
    struct.pack_into("<HHHI", testa, 0x2c, 45, len(blocchi), 0, tab)
    return bytes(testa) + ent + corpo_blk + voci


def cartiglio_acme() -> list[bytes]:
    """Un cartiglio: «DWG NO.» con due numeri impilati (una tavola comune), «Descrizione», «Materiale» con il
    segnaposto «.» del modello, e una cella con il nome di una persona."""
    return [text(300, 100, 5.6, "DWG NO."), text(300.2, 84, 12, "ACME1001"), text(300.2, 66, 12, "ACME2001"),
            text(100, 100, 5.6, "Descrizione"), text(100.5, 84, 12, "STAFFA ACME"),
            text(200, 100, 5.6, "Materiale"), text(200.5, 84, 12, "."),
            text(400, 100, 5.6, "Disegnato"), text(400.5, 84, 12, "Mario Rossi")]


def campi(f: dict) -> list[tuple[str, str]]:
    return sorted((c["etichetta"], c["valore"]) for c in f["cartiglio"])


def test_cartiglio_con_sorgente_e_numeri_impilati():
    dati = dwg(cartiglio_acme())
    f = leggi_testo_dwg_da_byte(dati)
    ModelloTestoDWG.model_validate(f)
    assert f["versione"] == VERSIONE_TESTO_DWG == 1 and f["esito"] == "letto" and f["versione_dwg"] == "AC1009"
    assert f["codifica"] == "cp1252" and not f["parziale"] and f["non_decodificate"] == 0
    assert campi(f) == [("numero_disegno", "ACME1001"), ("numero_disegno", "ACME2001"), ("titolo", "STAFFA ACME")]
    for c in f["cartiglio"]:
        s = c["sorgente"]
        assert s["sezione"] == "ENTITIES" and s["tipo_entita"] == "TEXT" and s["offset_entita"] >= 0x60
        assert dati[s["offset_entita"]] == 7 and dati[s["offset_etichetta"]] == 7   # l'offset punta a un TEXT
        assert len(c["riquadro"]) == 4
    testo = json.dumps(f)
    assert "Mario" not in testo and "Disegnato" not in testo        # solo i campi riconosciuti
    assert "." not in [c["valore"] for c in f["cartiglio"]]          # il segnaposto non e' un valore


def test_intestazioni_d_entita_con_tutti_i_campi_facoltativi():
    """Colore, tipo di linea, elevazione, handle lungo, extra con lo spazio: i campi si leggono comunque."""
    ent = [text(300, 100, 5.6, "DWG NO.", colore=3, ltype=7, elev=1.5, handle=b"\x01\x20\xc2", extra=4),
           text(300.2, 84, 12, "ACME1002", rot=0.0, colore=None, handle=None)]
    f = leggi_testo_dwg_da_byte(dwg(ent))
    assert campi(f) == [("numero_disegno", "ACME1002")] and f["non_decodificate"] == 0


def test_insert_annidati_ruotati_nel_sistema_del_blocco():
    """Il cartiglio in un blocco INTERNO, inserito in ESTERNO, inserito ruotato di 90° (radianti nel file): la regola
    vale nel sistema del blocco, come per il DXF."""
    interno = [text(0, 10, 5.6, "DWG NO."), text(0.2, -6, 12, "ACME1003")]
    esterno = [insert(0, 50, 0)]
    f = leggi_testo_dwg_da_byte(dwg([insert(1, 1000, 500, rot=90.0)], [("INTERNO", interno), ("ESTERNO", esterno)]))
    assert campi(f) == [("numero_disegno", "ACME1003")] and f["annidati_seguiti"] == 1
    c = f["cartiglio"][0]
    assert c["sorgente"]["sezione"] == "BLOCKS" and c["sorgente"]["blocco"] == "INTERNO"


def test_attrib_e_insert_con_dati_estesi():
    """Un ATTRIB si legge come un testo; un INSERT con dati estesi si legge dalla fine (i suoi campi hanno una
    lunghezza nota); un TEXT con dati estesi non si indovina: non_decodificate, parziale."""
    eed = b"\x00\x04ACME\x02\x01"
    ent = [attrib(300, 100, 5.6, "DWG NO.", "ETICHETTA"), attrib(300.2, 84, 12, "ACME1004", "NUMERO"),
           insert(0, 0, 0, extra=2, eed=eed)]
    f = leggi_testo_dwg_da_byte(dwg(ent, [("VUOTO", [])]))
    assert campi(f) == [("numero_disegno", "ACME1004")]
    assert f["cartiglio"][0]["sorgente"]["tipo_entita"] == "ATTRIB"
    assert f["con_dati_estesi"] == 1 and f["non_decodificate"] == 0
    f = leggi_testo_dwg_da_byte(dwg([text(0, 0, 5, "ACME", extra=2, eed=eed)]))
    assert f["non_decodificate"] == 1 and f["parziale"] is True and "dati estesi" in f["motivo"]


def test_entita_cancellate_flag_sconosciuti_e_lunghezze_che_non_tornano():
    ent = [text(0, 0, 5, "ACME CANCELLATO", cancellata=True), text(0, 20, 5, "ACME", flag_extra=0x10)] + cartiglio_acme()
    f = leggi_testo_dwg_da_byte(dwg(ent))
    assert f["cancellate_ignorate"] == 1 and f["non_decodificate"] == 1 and f["parziale"] is True
    assert ("numero_disegno", "ACME1001") in campi(f)
    # una lunghezza dichiarata piu' corta dei campi: l'entita' non torna e non si legge
    rotta = bytearray(text(0, 0, 5, "ACME"))
    struct.pack_into("<H", rotta, 2, len(rotta) - 3)
    f = leggi_testo_dwg_da_byte(dwg([bytes(rotta[:-3])] + cartiglio_acme()))
    assert f["non_decodificate"] >= 1 and f["parziale"] is True
    # una lunghezza oltre la sezione: il resto della sezione non si legge, e si dice
    oltre = bytearray(text(0, 0, 5, "ACME"))
    struct.pack_into("<H", oltre, 2, 60000)
    f = leggi_testo_dwg_da_byte(dwg(cartiglio_acme() + [bytes(oltre)]))
    assert f["parziale"] is True and "entita' non valida" in f["motivo"] and ("titolo", "STAFFA ACME") in campi(f)


def test_elenco_particolari_con_le_righe_sopra_l_intestazione():
    ent = [text(500, 200, 8, "POS."), text(540, 200, 8, "CODICE"), text(640, 200, 8, "DENOMINAZIONE"),
           text(800, 200, 8, "Q.TA")]
    for k in range(3):
        y = 215 + 15 * k
        ent += [text(505, y, 8, str(k + 1)), text(540, y, 8, f"ACME50{k}"), text(640, y, 8, f"PIASTRA {k}"),
                text(805, y, 8, "2")]
    f = leggi_testo_dwg_da_byte(dwg(ent + cartiglio_acme()))
    (tab,) = f["tabelle"]
    assert tab["verso"] == "sopra" and [c["ruolo"] for c in tab["colonne"]] == ["posizione", "codice", "descrizione", "quantita"]
    valori = [{c["intestazione"]: c["valore"] for c in r["celle"]} for r in tab["righe"]]
    assert [v["CODICE"] for v in valori] == ["ACME500", "ACME501", "ACME502"]
    cella = tab["righe"][0]["celle"][1]
    assert len(cella["sorgente"]) == 1 and cella["sorgente"][0] >= 0x60
    assert tab["righe"][0]["riquadro"][1] < tab["righe"][1]["riquadro"][1]       # y verso l'alto nel disegno
    ModelloTestoDWG.model_validate(f)


def test_versioni_non_supportate_e_file_che_non_sono_dwg(tmp_path):
    for firma, nome in ((b"AC1015", "2000"), (b"AC1018", "2004"), (b"AC1032", "2018")):
        f = leggi_testo_dwg_da_byte(firma + b"\x00" * 200)
        assert f["esito"] == "non_supportato" and f["versione_dwg"] == firma.decode() and nome in f["motivo"]
        assert f["cartiglio"] == [] and f["tabelle"] == []
    assert leggi_testo_dwg_da_byte(b"non sono un disegno")["esito"] == "non_letto"
    assert leggi_testo_dwg_da_byte(b"AC1009" + b"\x00" * 10)["esito"] == "non_letto"
    assert leggi_testo_dwg(str(tmp_path / "manca.dwg"))["esito"] == "non_letto"
    vuoto = leggi_testo_dwg_da_byte(dwg([]))
    assert vuoto["esito"] == "non_letto" and vuoto["versione_dwg"] == "AC1009"


def test_analisi_di_un_dwg(tmp_path):
    p = tmp_path / "ACME1001.dwg"
    p.write_bytes(dwg(cartiglio_acme()))
    r = analizza_file(str(p), "ACME1001.dwg")
    assert r["tipo_proposto"] == "disegno_2d" and r["codice"] == "ACME1001"
    t = r["dettagli"]["testo_dwg"]
    ModelloTestoDWG.model_validate(t)
    assert t["esito"] == "letto" and len(t["cartiglio"]) == 3
    p2 = tmp_path / "nuovo.dwg"
    p2.write_bytes(b"AC1027" + b"\x00" * 100)
    assert analizza_file(str(p2), "nuovo.dwg")["dettagli"]["testo_dwg"]["esito"] == "non_supportato"


def test_limiti():
    f = leggi_testo_dwg_da_byte(dwg(cartiglio_acme()), {"testi_max": 2})
    assert f["troncato"] and f["parziale"] and f["testi"] == 2 and f["limiti"]["testi_max"] == 2
    f = leggi_testo_dwg_da_byte(dwg(cartiglio_acme()), {"entita_max": 3})
    assert f["parziale"] and "entita'" in f["motivo"]
