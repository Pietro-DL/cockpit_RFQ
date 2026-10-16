"""GP-3, sotto-blocco 2 (direttiva 21 §5.1): il fatto `foglio_dati` dei fogli di calcolo, con la sola libreria standard.

Le cartelle sono COSTRUITE qui, parte per parte (XML scritto a mano dentro uno zip, SpreadsheetML, CSV), con codici
ACME inventati: niente openpyxl (il worker non l'ha fra le dipendenze) e niente file veri.

Le regole difese: il testo resta esatto (zeri iniziali, due spazi); formula e valore memorizzato restano distinti; lo
stile che porta uno stato (rosso, barrato, apice) si conserva senza interpretarlo; i collegamenti perdono cartelle e
host; le macro e i collegamenti esterni si segnalano e non si toccano; il BIFF e' «non supportato», mai un successo
vuoto; XML con DTD e parti compresse oltre misura non si leggono; due versioni dello stesso foglio sono due fatti.
"""
from __future__ import annotations

import io
import zipfile

from contratti import FoglioDati as ModelloFoglio
from foglio_dati import (FIRMA_OLE2, ha_intestazione_di_distinta, indirizzo, leggi_foglio, leggi_foglio_da_byte, rc)
from worker_analisi import analizza_file

M = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
P = "http://schemas.openxmlformats.org/package/2006/relationships"


def xlsx(celle_xml: str, condivise: list[str], extra: dict[str, bytes] | None = None, collegamenti: str = "",
         rel_foglio: str = "", stato: str = "", unite: str = "") -> bytes:
    """Una cartella XLSX minima: un foglio, le stringhe condivise e gli stili (0 predefinito, 1 rosso con l'apice, 2
    barrato, 3 formato 00000)."""
    sst = "".join(f'<si><t xml:space="preserve">{s}</t></si>' for s in condivise)
    stili = (f'<styleSheet xmlns="{M}"><numFmts count="1"><numFmt numFmtId="164" formatCode="00000"/></numFmts>'
             '<fonts count="3"><font><color theme="1"/></font><font><color rgb="FFFF0000"/></font><font><strike/></font></fonts>'
             '<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>'
             '<cellXfs count="4"><xf fontId="0" fillId="0" numFmtId="0"/><xf fontId="1" fillId="0" numFmtId="0" quotePrefix="1"/>'
             '<xf fontId="2" fillId="0" numFmtId="0"/><xf fontId="0" fillId="0" numFmtId="164"/></cellXfs></styleSheet>')
    foglio = (f'<worksheet xmlns="{M}" xmlns:r="{R}"><dimension ref="A1:H4"/><sheetData>{celle_xml}</sheetData>'
              f'{unite}{collegamenti}</worksheet>')
    parti = {
        "[Content_Types].xml": b"<Types/>",
        "xl/workbook.xml": (f'<workbook xmlns="{M}" xmlns:r="{R}"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"'
                            f'{stato}/></sheets></workbook>').encode(),
        "xl/_rels/workbook.xml.rels": (f'<Relationships xmlns="{P}"><Relationship Id="rId1" Type="x" '
                                       'Target="worksheets/sheet1.xml"/></Relationships>').encode(),
        "xl/worksheets/sheet1.xml": foglio.encode(),
        "xl/sharedStrings.xml": f'<sst xmlns="{M}">{sst}</sst>'.encode(),
        "xl/styles.xml": stili.encode(),
    }
    if rel_foglio:
        parti["xl/worksheets/_rels/sheet1.xml.rels"] = f'<Relationships xmlns="{P}">{rel_foglio}</Relationships>'.encode()
    parti.update(extra or {})
    b = io.BytesIO()
    with zipfile.ZipFile(b, "w", zipfile.ZIP_DEFLATED) as z:
        for n, d in parti.items():
            z.writestr(n, d)
    return b.getvalue()


CONDIVISE = ["Level", "SUPPLIER PN", "CUSTOMER PN", "Description", "Drawing PDF", "File STEP",
             "1  ACME117Z", "ACME.0741GT", "ACME_117.pdf", "ACME_117.step", "00123", "vecchio"]


def cartella_acme() -> bytes:
    celle = ('<row r="2"><c r="C2" t="s"><v>0</v></c><c r="D2" t="s"><v>1</v></c><c r="E2" t="s"><v>2</v></c>'
             '<c r="F2" t="s"><v>3</v></c><c r="G2" t="s"><v>4</v></c><c r="H2" t="s"><v>5</v></c></row>'
             '<row r="3"><c r="C3"><v>1</v></c><c r="D3" t="s"><v>6</v></c><c r="E3" t="s" s="1"><v>7</v></c>'
             '<c r="F3" t="s" s="2"><v>11</v></c><c r="G3" t="s"><v>8</v></c><c r="H3" t="s"><v>9</v></c></row>'
             '<row r="4"><c r="C4" t="s"><v>10</v></c><c r="D4" s="3"><v>123</v></c><c r="E4"><f>SUM(C3:C3)</f><v>1</v></c>'
             '<c r="F4" t="inlineStr"><is><t>riga  libera</t></is></c><c r="G4" t="b"><v>1</v></c><c r="H4"/></row>')
    coll = ('<hyperlinks><hyperlink ref="G3" r:id="rId7"/><hyperlink ref="H3" r:id="rId8"/>'
            '<hyperlink ref="C4" location="Sheet1!A1"/></hyperlinks>')
    rel = ('<Relationship Id="rId7" Type="h" Target="../../Cartella%20Personale/ACME_117.pdf" TargetMode="External"/>'
           '<Relationship Id="rId8" Type="h" Target="https://server.example/condivisi/ACME_117.step?x=1" TargetMode="External"/>')
    return xlsx(celle, CONDIVISE, collegamenti=coll, rel_foglio=rel, unite='<mergeCells><mergeCell ref="C1:H1"/></mergeCells>')


def per_indirizzo(f: dict) -> dict[str, dict]:
    return {c["ind"]: c for c in f["celle"]}


def test_xlsx_testo_esatto_tipi_formule_stili_collegamenti():
    f = leggi_foglio_da_byte(cartella_acme(), "ACME.xlsx")
    ModelloFoglio.model_validate(f)
    assert f["esito"] == "letto" and f["formato"] == "xlsx" and not f["troncato"], f["motivo"]
    c = per_indirizzo(f)
    assert c["D3"]["grezzo"] == "1  ACME117Z" and c["D3"]["tipo"] == "s"          # i due spazi restano
    assert c["C4"]["grezzo"] == "00123" and c["C4"]["tipo"] == "s"                # gli zeri di una stringa restano
    assert c["D4"]["grezzo"] == "123" and c["D4"]["tipo"] == "n"                  # il numero com'e' scritto ...
    assert f["stili"][c["D4"]["stile"]]["formato_numero"] == "00000"              # ... e il suo formato a parte
    assert c["C3"]["grezzo"] == "1" and c["C3"]["tipo"] == "n" and c["C3"]["stile"] is None
    assert c["E4"]["da_formula"] and c["E4"]["formula"] == "SUM(C3:C3)" and c["E4"]["grezzo"] == "1"
    assert c["F4"]["grezzo"] == "riga  libera" and c["G4"]["tipo"] == "b"
    assert "H4" not in c                                                          # le celle vuote no
    rosso = f["stili"][c["E3"]["stile"]]
    assert rosso["colore_font"] == "FFFF0000" and rosso["apice"] is True          # lo stato portato dallo stile
    assert f["stili"][c["F3"]["stile"]]["barrato"] is True
    assert c["G3"]["collegamento"] == {"tipo": "relativo", "nome_base": "ACME_117.pdf", "estensione": "pdf"}
    assert c["H3"]["collegamento"] == {"tipo": "url", "nome_base": "ACME_117.step", "estensione": "step"}
    assert c["C4"]["collegamento"]["tipo"] == "interno"
    assert "server.example" not in str(f) and "Personale" not in str(f)          # cartelle e host non viaggiano
    assert f["fogli"][0]["unite"] == ["C1:H1"] and f["fogli"][0]["dimensione"] == "A1:H4"
    # niente interpretazione: nessuna chiave di stato o di codice nelle celle
    assert not any(k in c["E3"] for k in ("obsoleto", "codice", "stato"))


def test_intestazione_candidata_e_distinta():
    f = leggi_foglio_da_byte(cartella_acme(), "ACME.xlsx")
    (h,) = f["intestazioni_candidate"]
    assert h["riga"] == 2 and [x["ruolo"] for x in h["colonne"]][:4] == ["livello", "codice", "codice", "descrizione"]
    assert not ha_intestazione_di_distinta(f["intestazioni_candidate"])   # niente quantita': corrispondenza, non distinta


def test_due_versioni_due_fatti():
    a = cartella_acme()
    b = xlsx('<row r="1"><c r="A1" t="s"><v>0</v></c></row>', ["ACME"])
    fa, fb = leggi_foglio_da_byte(a, "x.xlsx"), leggi_foglio_da_byte(b, "x.xlsx")
    assert fa["sha256"] != fb["sha256"] and len(fa["sha256"]) == 64
    assert leggi_foglio_da_byte(a, "copia.xlsx")["sha256"] == fa["sha256"]


def test_macro_e_collegamenti_esterni_si_segnalano():
    dati = xlsx('<row r="1"><c r="A1" t="s"><v>0</v></c></row>', ["ACME"],
                extra={"xl/vbaProject.bin": b"\x00" * 64, "xl/externalLinks/externalLink1.xml": b"<externalLink/>"})
    f = leggi_foglio_da_byte(dati, "macro.xlsm")
    assert f["esito"] == "letto" and f["formato"] == "xlsm" and f["macro_presenti"] is True
    assert f["collegamenti_esterni"] == 1


def test_foglio_nascosto():
    f = leggi_foglio_da_byte(xlsx('<row r="1"><c r="A1" t="s"><v>0</v></c></row>', ["ACME"], stato=' state="hidden"'), "h.xlsx")
    assert f["fogli"][0]["stato"] == "nascosto"


def test_xml_con_dtd_non_si_legge():
    bomba = ('<!DOCTYPE x [<!ENTITY a "aaaa"><!ENTITY b "&a;&a;&a;">]>'
             f'<worksheet xmlns="{M}"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>&b;</t></is></c></row>'
             '</sheetData></worksheet>').encode()
    f = leggi_foglio_da_byte(xlsx("", [], extra={"xl/worksheets/sheet1.xml": bomba}), "dtd.xlsx")
    assert f["esito"] == "non_letto" and "DTD" in f["motivo"], f


def test_parte_compressa_oltre_misura_non_si_legge():
    f = leggi_foglio_da_byte(xlsx("", [], extra={"xl/sharedStrings.xml": b" " * 3_000_000}), "zip.xlsx")
    assert f["esito"] == "non_letto" and "rapporto" in f["motivo"], f


def test_tetto_delle_celle_da_parziale():
    f = leggi_foglio_da_byte(cartella_acme(), "x.xlsx", {"celle_max": 5})
    assert f["esito"] == "parziale" and f["troncato"] and len(f["celle"]) == 5 and "celle" in f["motivo"]


def spreadsheetml() -> bytes:
    return ("""<?xml version="1.0"?>
<Workbook xmlns="urn:schemas-microsoft-com:office:spreadsheet" xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet"
 xmlns:x="urn:schemas-microsoft-com:office:excel">
 <Styles><Style ss:ID="Default"><Font ss:Color="#000000"/></Style>
  <Style ss:ID="s1"><Font ss:Color="#FF0000" ss:StrikeThrough="1"/></Style></Styles>
 <Worksheet ss:Name="BOM">
  <Table>
   <Row><Cell ss:MergeAcross="2"><Data ss:Type="String">Stückliste ACME</Data></Cell></Row>
   <Row ss:Index="3"><Cell><Data ss:Type="String">Position</Data></Cell><Cell><Data ss:Type="String">Komponentennummer</Data></Cell>
    <Cell ss:Index="4"><Data ss:Type="String">Menge</Data></Cell></Row>
   <Row><Cell><Data ss:Type="String">0010</Data></Cell><Cell ss:StyleID="s1"><Data ss:Type="String">ACME 4711</Data></Cell>
    <Cell ss:Index="4" ss:Formula="=1+1"><Data ss:Type="Number">2</Data></Cell>
    <Cell ss:HRef="file:///C:/Utenti/qualcuno/ACME_4711.pdf"><Data ss:Type="String">disegno</Data></Cell></Row>
  </Table>
 </Worksheet>
</Workbook>""").encode("utf-8")


def test_spreadsheetml(tmp_path):
    f = leggi_foglio_da_byte(spreadsheetml(), "BOM_ACME.xls")
    ModelloFoglio.model_validate(f)
    assert f["esito"] == "letto" and f["formato"] == "xls_xml"
    c = per_indirizzo(f)
    assert c["A1"]["grezzo"] == "Stückliste ACME" and f["fogli"][0]["unite"] == ["A1:C1"]
    assert c["A4"]["grezzo"] == "0010" and c["B4"]["grezzo"] == "ACME 4711"
    st = f["stili"][c["B4"]["stile"]]
    assert st["colore_font"] == "FFFF0000" and st["barrato"] is True
    assert c["D4"]["tipo"] == "n" and c["D4"]["grezzo"] == "2" and c["D4"]["formula"] == "=1+1" and c["D4"]["da_formula"]
    assert c["E4"]["collegamento"] == {"tipo": "assoluto", "nome_base": "ACME_4711.pdf", "estensione": "pdf"}
    assert ha_intestazione_di_distinta(f["intestazioni_candidate"])
    p = tmp_path / "BOM_ACME.xls"
    p.write_bytes(spreadsheetml())
    r = analizza_file(str(p), "BOM_ACME.xls")
    assert r["tipo_proposto"] == "distinta_cliente" and r["dettagli"]["foglio_dati"]["esito"] == "letto"


def test_biff_e_non_supportato_mai_un_successo_vuoto(tmp_path):
    p = tmp_path / "vecchio.xls"
    p.write_bytes(FIRMA_OLE2 + b"\x00" * 512)
    r = analizza_file(str(p), "vecchio.xls")
    f = r["dettagli"]["foglio_dati"]
    assert r["tipo_proposto"] == "commerciale"
    assert f["esito"] == "non_supportato" and f["formato"] == "xls_biff" and "BIFF" in f["motivo"] and f["celle"] == []
    assert leggi_foglio_da_byte(FIRMA_OLE2 + b"\x00" * 64, "cifrato.xlsx")["motivo"].startswith("contenitore OLE2")


def test_formati_che_non_sono_fogli():
    assert leggi_foglio_da_byte(b"<html><table><tr><td>1</td></tr></table></html>", "x.xls")["esito"] == "non_supportato"
    f = leggi_foglio_da_byte(b"\x00\x01\x02 non un foglio", "x.xlsx")
    assert f["esito"] == "non_letto" and f["motivo"]
    f = leggi_foglio_da_byte(b"PK\x03\x04 rotto", "x.xlsx")
    assert f["esito"] == "non_letto"


def test_csv(tmp_path):
    p = tmp_path / "elenco.csv"
    p.write_bytes("POS;CODICE;Q.TA\n1;00123;2\n2;ACME  77;1\n".encode("utf-8"))
    f = leggi_foglio(str(p), "elenco.csv")
    ModelloFoglio.model_validate(f)
    c = per_indirizzo(f)
    assert f["formato"] == "csv" and c["B2"]["grezzo"] == "00123" and c["B3"]["grezzo"] == "ACME  77"
    assert ha_intestazione_di_distinta(f["intestazioni_candidate"])


def test_indirizzi():
    assert indirizzo(9, 5) == "E9" and indirizzo(1, 27) == "AA1" and rc("AA1") == (1, 27) and rc("$E$9") == (9, 5)
