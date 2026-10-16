"""Lettura dei FOGLI DI CALCOLO: celle, tipi, testo originale, stile che porta uno stato (GP-3, direttiva 21 §5.1).

Che cosa fa e che cosa NON fa. Come step_struttura per lo STEP, il modulo dice che cosa c'e' scritto nel file: per ogni
cella non vuota il foglio, l'indirizzo («E9»), il tipo del valore memorizzato, il testo ESATTO come sta nel file (gli
zeri iniziali di una stringa, i due spazi di «1  117667Z», il numero come e' scritto, non riformattato), la formula
separata dal valore memorizzato, il formato del numero, lo stile che puo' portare uno stato (il colore del carattere,
il barrato, il grassetto, il riempimento, l'apice iniziale) e il collegamento ridotto a nome ed estensione. NON dice
che cosa significhino: «rosso = obsoleto» o «barrato = sostituito» lo dice una regola del cliente sul server, mai il
worker. Le intestazioni candidate (una riga di nomi di colonna con un ruolo tipico, colonne.py) sono un aiuto di
lettura, non un'interpretazione.

Sicurezza, scritta qui perche' il file e' di terzi:
  - nessuna macro si esegue e nessun collegamento si segue: le macro si segnalano (`macro_presenti`), i collegamenti
    esterni si contano (`collegamenti_esterni`), i collegamenti delle celle perdono cartelle e host (solo il nome
    del file e l'estensione: puntano spesso a cartelle personali);
  - XML con DTD o entita' (<!DOCTYPE, <!ENTITY) non si legge: e' la via delle «bombe» XML;
  - ogni parte compressa si legge solo sotto un tetto di byte e di rapporto di compressione (le «bombe» zip);
  - tetti su fogli, celle, caratteri per cella, celle unite: oltre, `esito: parziale` e `troncato`.

Formati: XLSX/XLSM (zip + XML), SpreadsheetML 2003 (XML, i «.xls» esportati da molti gestionali), CSV. Il BIFF di
Excel 97-2003 (OLE2) e' `non_supportato` con il motivo: leggerlo chiede una libreria che il worker non ha. Un file
che non e' nessuno di questi e' `non_letto`. Mai un successo vuoto.

La versione della cartella e' lo sha256 dei byte: due versioni dello stesso file sono due fatti, e ogni cella resta
con la sua versione.
"""
from __future__ import annotations

import csv
import hashlib
import io
import logging
import posixpath
import re
import zipfile
from urllib.parse import unquote, urlsplit
from xml.etree import ElementTree as ET

from colonne import CODICE, POSIZIONE, QUANTITA, ruolo_colonna

log = logging.getLogger("worker-analisi")

VERSIONE_FOGLIO_DATI = 1

LIMITI_DEFAULT = {
    "byte_max": 64 * 1024 * 1024,   # byte del file
    "parte_max": 64 * 1024 * 1024,  # byte decompressi di una parte XML (o del CSV)
    "rapporto_max": 200,            # rapporto decompresso/compresso di una parte
    "fogli_max": 50,                # fogli letti
    "celle_max": 10000,             # celle nel fatto, in tutti i fogli
    "testo_max": 1000,              # caratteri conservati di una cella o di una formula
    "unite_max": 500,               # celle unite per foglio
}
RIGHE_INTESTAZIONE = 30             # le righe di ogni foglio in cui si cercano le intestazioni candidate
INTESTAZIONI_PER_FOGLIO = 5

LETTO, PARZIALE, NON_SUPPORTATO, NON_LETTO = "letto", "parziale", "non_supportato", "non_letto"

FIRMA_OLE2 = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
NS_SS = "urn:schemas-microsoft-com:office:spreadsheet"
NS_X = "urn:schemas-microsoft-com:office:excel"
NS_MAIN = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
NS_REL = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
NS_PKG = "http://schemas.openxmlformats.org/package/2006/relationships"

# I formati numerici predefiniti di Excel piu' comuni (gli altri restano «id:N»).
FORMATI_PREDEFINITI = {0: "General", 1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00", 9: "0%", 10: "0.00%", 11: "0.00E+00",
                       12: "# ?/?", 13: "# ??/??", 14: "mm-dd-yy", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy",
                       18: "h:mm AM/PM", 19: "h:mm:ss AM/PM", 20: "h:mm", 21: "h:mm:ss", 22: "m/d/yy h:mm", 49: "@"}
# La tavolozza dei colori indicizzati (i primi 16 di quella predefinita): ARGB.
TAVOLOZZA = {0: "FF000000", 1: "FFFFFFFF", 2: "FFFF0000", 3: "FF00FF00", 4: "FF0000FF", 5: "FFFFFF00", 6: "FFFF00FF",
             7: "FF00FFFF", 8: "FF000000", 9: "FFFFFFFF", 10: "FFFF0000", 11: "FF00FF00", 12: "FF0000FF",
             13: "FFFFFF00", 14: "FFFF00FF", 15: "FF00FFFF", 16: "FF800000", 17: "FF008000", 18: "FF000080",
             19: "FF808000", 20: "FF800080", 21: "FF008080", 22: "FFC0C0C0", 23: "FF808080"}

_RE_RIF = re.compile(r"^([A-Z]+)(\d+)$")


class _Fermo(Exception):
    """Un tetto raggiunto: la lettura si ferma, quello che c'e' vale."""


def indirizzo(r: int, c: int) -> str:
    """(riga, colonna) da 1 -> «E9»."""
    s = ""
    while c > 0:
        c, k = divmod(c - 1, 26)
        s = chr(65 + k) + s
    return f"{s}{r}"


def rc(rif: str) -> tuple[int, int]:
    """«E9» -> (9, 5)."""
    m = _RE_RIF.match(rif.upper().replace("$", ""))
    if not m:
        raise ValueError(f"riferimento di cella non valido: {rif!r}")
    c = 0
    for ch in m.group(1):
        c = c * 26 + ord(ch) - 64
    return int(m.group(2)), c


def collegamento(destinazione: str, interno: bool = False) -> dict:
    """Un collegamento ridotto: il tipo, il nome del file e l'estensione. Mai cartelle, host o indirizzi di posta."""
    if interno:
        return {"tipo": "interno", "nome_base": "", "estensione": ""}
    d = destinazione.strip()
    basso = d.lower()
    if basso.startswith("mailto:"):
        return {"tipo": "posta", "nome_base": "", "estensione": ""}
    if re.match(r"^[a-z][a-z0-9+.-]*://", basso) and not basso.startswith("file:"):
        percorso = unquote(urlsplit(d).path)
        tipo = "url"
    elif basso.startswith("file:") or re.match(r"^[a-z]:[\\/]", basso) or d.startswith("\\\\") or d.startswith("/"):
        percorso = unquote(d[5:] if basso.startswith("file:") else d)
        tipo = "assoluto"
    elif d.startswith("#"):
        return {"tipo": "interno", "nome_base": "", "estensione": ""}
    else:
        percorso = unquote(d)
        tipo = "relativo"
    nome = re.split(r"[\\/]", percorso.rstrip("\\/"))[-1] if percorso else ""
    est = nome.rsplit(".", 1)[1].lower() if "." in nome else ""
    return {"tipo": tipo, "nome_base": nome[:200], "estensione": est[:20]}


class _Raccolta:
    """Le celle e i fogli letti, con i tetti."""

    def __init__(self, lim: dict):
        self.lim = lim
        self.fogli: list[dict] = []
        self.celle: list[dict] = []
        self.stili: list[dict] = []          # gli stili distinti; una cella porta l'indice
        self._indice_stile: dict[tuple, int] = {}
        self.troncato = False
        self.motivi: list[str] = []

    def stile(self, s: dict | None) -> int | None:
        if s is None:
            return None
        k = tuple(sorted(s.items()))
        if k not in self._indice_stile:
            self._indice_stile[k] = len(self.stili)
            self.stili.append(dict(s))
        return self._indice_stile[k]

    def tronca(self, motivo: str) -> None:
        self.troncato = True
        if motivo not in self.motivi:
            self.motivi.append(motivo)

    def testo(self, s: str) -> str:
        if len(s) > self.lim["testo_max"]:
            self.tronca(f"celle oltre {self.lim['testo_max']} caratteri")
            return s[:self.lim["testo_max"]]
        return s

    def cella(self, foglio: int, r: int, c: int, tipo: str, grezzo: str, formula: str = "", da_formula: bool = False,
              stile: dict | None = None, coll: dict | None = None) -> None:
        if len(self.celle) >= self.lim["celle_max"]:
            self.tronca(f"oltre {self.lim['celle_max']} celle")
            raise _Fermo()
        self.celle.append({"foglio": foglio, "ind": indirizzo(r, c), "r": r, "c": c, "tipo": tipo,
                           "grezzo": self.testo(grezzo), "formula": self.testo(formula), "da_formula": da_formula,
                           "stile": self.stile(stile), "collegamento": coll})


def _senza_dtd(dati: bytes) -> None:
    testa = dati[:4096].upper()
    if b"<!DOCTYPE" in testa or b"<!ENTITY" in dati.upper():
        raise ValueError("XML con DTD o entita': non si legge (protezione dalle bombe XML)")


def _ln(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


# ---------------------------------------------------------------- XLSX

def _parte(z: zipfile.ZipFile, nome: str, lim: dict) -> bytes | None:
    try:
        info = z.getinfo(nome)
    except KeyError:
        return None
    if info.file_size > lim["parte_max"]:
        raise ValueError(f"parte {nome} di {info.file_size} byte: oltre il tetto di {lim['parte_max']}")
    if info.compress_size and info.file_size / max(info.compress_size, 1) > lim["rapporto_max"]:
        raise ValueError(f"parte {nome} compressa oltre il rapporto {lim['rapporto_max']}: protezione dalle bombe zip")
    dati = z.read(nome)
    if len(dati) > lim["parte_max"]:
        raise ValueError(f"parte {nome} oltre il tetto di byte")
    return dati


def _xml(dati: bytes):
    _senza_dtd(dati)
    return ET.fromstring(dati)


def _relazioni(z, nome, lim) -> dict[str, tuple[str, str]]:
    dati = _parte(z, nome, lim)
    if dati is None:
        return {}
    out = {}
    for rel in _xml(dati):
        out[rel.get("Id", "")] = (rel.get("Target", ""), rel.get("TargetMode", ""))
    return out


def _colore(el) -> str:
    if el is None:
        return ""
    if el.get("rgb"):
        return el.get("rgb").upper()
    if el.get("indexed") is not None:
        try:
            i = int(el.get("indexed"))
        except ValueError:
            return ""
        return TAVOLOZZA.get(i, f"indexed:{i}")
    if el.get("theme") is not None:
        tinta = el.get("tint")
        return f"theme:{el.get('theme')}" + (f":{tinta}" if tinta else "")
    return ""


def _stili_xlsx(z, lim) -> list[dict | None]:
    """Lo stile di ogni xf di cellXfs (None se non porta niente di notevole rispetto al predefinito)."""
    dati = _parte(z, "xl/styles.xml", lim)
    if dati is None:
        return []
    root = _xml(dati)
    ns = {"m": NS_MAIN}
    formati = dict(FORMATI_PREDEFINITI)
    for nf in root.findall("m:numFmts/m:numFmt", ns):
        try:
            formati[int(nf.get("numFmtId"))] = nf.get("formatCode", "")
        except (TypeError, ValueError):
            pass
    fonts = []
    for f in root.findall("m:fonts/m:font", ns):
        def ha(tag):
            e = f.find(f"m:{tag}", ns)
            return e is not None and e.get("val", "1") not in ("0", "false", "none")
        fonts.append({"colore_font": _colore(f.find("m:color", ns)), "grassetto": ha("b"), "corsivo": ha("i"),
                      "barrato": ha("strike"), "sottolineato": ha("u")})
    fills = []
    for fl in root.findall("m:fills/m:fill", ns):
        pf = fl.find("m:patternFill", ns)
        if pf is not None and pf.get("patternType") not in (None, "none", "gray125"):
            fills.append(_colore(pf.find("m:fgColor", ns)) or pf.get("patternType", ""))
        else:
            fills.append("")
    base = fonts[0] if fonts else {"colore_font": "", "grassetto": False, "corsivo": False, "barrato": False,
                                   "sottolineato": False}
    out: list[dict | None] = []
    for xf in root.findall("m:cellXfs/m:xf", ns):
        def intero(a):
            try:
                return int(xf.get(a, "0"))
            except ValueError:
                return 0
        font = fonts[intero("fontId")] if intero("fontId") < len(fonts) else base
        riemp = fills[intero("fillId")] if intero("fillId") < len(fills) else ""
        nf = intero("numFmtId")
        apice = xf.get("quotePrefix") in ("1", "true")
        stile = {**font, "riempimento": riemp, "apice": apice, "formato_numero": formati.get(nf, f"id:{nf}") if nf else ""}
        notevole = font != base or riemp or apice or nf
        out.append(stile if notevole else None)
    return out


def _condivise(z, lim, racc: _Raccolta) -> list[str]:
    dati = _parte(z, "xl/sharedStrings.xml", lim)
    if dati is None:
        return []
    out = []
    m = f"{{{NS_MAIN}}}"
    for si in _xml(dati).findall(f"{m}si"):
        # il testo di <t> e delle <t> dei pezzi <r>, concatenato com'e', spazi compresi; la pronuncia <rPh> no
        pezzi = []
        for figlio in si:
            if figlio.tag == f"{m}t":
                pezzi.append(figlio.text or "")
            elif figlio.tag == f"{m}r":
                pezzi += [t.text or "" for t in figlio.findall(f"{m}t")]
        out.append("".join(pezzi))
    return out


def _leggi_xlsx(dati: bytes, lim: dict, racc: _Raccolta) -> dict:
    extra = {"macro_presenti": False, "collegamenti_esterni": 0, "formato": "xlsx"}
    with zipfile.ZipFile(io.BytesIO(dati)) as z:
        nomi = set(z.namelist())
        if "xl/workbook.xml" not in nomi:
            raise ValueError("zip senza xl/workbook.xml: non e' una cartella XLSX")
        extra["macro_presenti"] = any(n.lower().endswith("vbaproject.bin") for n in nomi)
        if extra["macro_presenti"]:
            extra["formato"] = "xlsm"
        extra["collegamenti_esterni"] = sum(1 for n in nomi if n.startswith("xl/externalLinks/") and n.endswith(".xml"))
        rel_wb = _relazioni(z, "xl/_rels/workbook.xml.rels", lim)
        stili = _stili_xlsx(z, lim)
        condivise = _condivise(z, lim, racc)
        wb = _xml(_parte(z, "xl/workbook.xml", lim))
        fogli = wb.findall(f"{{{NS_MAIN}}}sheets/{{{NS_MAIN}}}sheet")
        for i, sh in enumerate(fogli):
            if i >= lim["fogli_max"]:
                racc.tronca(f"oltre {lim['fogli_max']} fogli")
                break
            stato = {"hidden": "nascosto", "veryHidden": "molto_nascosto"}.get(sh.get("state", ""), "visibile")
            info = {"indice": i, "nome": sh.get("name", "")[:100], "stato": stato, "dimensione": "", "celle": 0,
                    "unite": [], "troncato": False}
            racc.fogli.append(info)
            destinazione = rel_wb.get(sh.get(f"{{{NS_REL}}}id", ""), ("", ""))[0]
            if not destinazione:
                info["troncato"] = True
                racc.tronca(f"foglio «{info['nome']}» senza parte")
                continue
            parte = posixpath.normpath(posixpath.join("xl", destinazione)) if not destinazione.startswith("/") \
                else destinazione.lstrip("/")
            rel_foglio = _relazioni(z, posixpath.join(posixpath.dirname(parte), "_rels",
                                                      posixpath.basename(parte) + ".rels"), lim)
            xml = _parte(z, parte, lim)
            if xml is None:
                info["troncato"] = True
                racc.tronca(f"foglio «{info['nome']}» mancante")
                continue
            _leggi_foglio_xlsx(xml, i, info, stili, condivise, rel_foglio, lim, racc)
    return extra


def _leggi_foglio_xlsx(xml: bytes, i: int, info: dict, stili, condivise, rel, lim, racc: _Raccolta) -> None:
    root = _xml(xml)
    m = f"{{{NS_MAIN}}}"
    dim = root.find(f"{m}dimension")
    if dim is not None:
        info["dimensione"] = dim.get("ref", "")[:30]
    for mc in root.findall(f"{m}mergeCells/{m}mergeCell"):
        if len(info["unite"]) >= lim["unite_max"]:
            info["troncato"] = True
            racc.tronca(f"oltre {lim['unite_max']} celle unite in un foglio")
            break
        info["unite"].append(mc.get("ref", "")[:30])
    collegamenti: dict[tuple[int, int], dict] = {}
    for h in root.findall(f"{m}hyperlinks/{m}hyperlink"):
        ref = h.get("ref", "")
        rid = h.get(f"{{{NS_REL}}}id")
        if rid and rid in rel:
            cl = collegamento(rel[rid][0])
        else:
            cl = collegamento("", interno=True)
        try:
            a, _, b = ref.partition(":")
            r0, c0 = rc(a)
            r1, c1 = rc(b) if b else (r0, c0)
        except ValueError:
            continue
        for r in range(r0, min(r1, r0 + 1000) + 1):
            for c in range(c0, min(c1, c0 + 100) + 1):
                collegamenti[(r, c)] = cl
    n = 0
    for c_el in root.iter(f"{m}c"):
        ref = c_el.get("r")
        if not ref:
            continue
        try:
            r, c = rc(ref)
        except ValueError:
            continue
        t = c_el.get("t", "n")
        f_el = c_el.find(f"{m}f")
        v_el = c_el.find(f"{m}v")
        formula = (f_el.text or "") if f_el is not None else ""
        da_formula = f_el is not None
        if t == "inlineStr":
            is_el = c_el.find(f"{m}is")
            grezzo = "".join(x.text or "" for x in is_el.iter(f"{m}t")) if is_el is not None else ""
            tipo = "s"
        else:
            grezzo = (v_el.text or "") if v_el is not None else ""
            if t == "s":
                try:
                    grezzo, tipo = condivise[int(grezzo)], "s"
                except (ValueError, IndexError):
                    racc.tronca("riferimenti a stringhe condivise che non esistono")
                    tipo = "e"
            else:
                tipo = {"str": "s", "b": "b", "e": "e", "d": "d"}.get(t, "n")
        if grezzo == "" and not da_formula:
            continue
        try:
            s = int(c_el.get("s", "0"))
        except ValueError:
            s = 0
        stile = stili[s] if s < len(stili) else None
        racc.cella(i, r, c, tipo, grezzo, formula, da_formula, stile, collegamenti.get((r, c)))
        n += 1
    info["celle"] = n


# ---------------------------------------------------------------- SpreadsheetML 2003

def _leggi_spreadsheetml(dati: bytes, lim: dict, racc: _Raccolta) -> dict:
    root = _xml(dati)
    ss = f"{{{NS_SS}}}"
    if _ln(root.tag) != "Workbook":
        raise ValueError("XML senza Workbook: non e' SpreadsheetML")
    stili: dict[str, dict | None] = {}
    for st in root.findall(f"{ss}Styles/{ss}Style"):
        font = st.find(f"{ss}Font")
        interno = st.find(f"{ss}Interior")
        formato = st.find(f"{ss}NumberFormat")

        def a(el, nome):
            return el.get(f"{ss}{nome}", "") if el is not None else ""
        colore = a(font, "Color").lstrip("#").upper()
        stile = {"colore_font": ("FF" + colore) if len(colore) == 6 else colore,
                 "grassetto": a(font, "Bold") == "1", "corsivo": a(font, "Italic") == "1",
                 "barrato": a(font, "StrikeThrough") == "1", "sottolineato": a(font, "Underline") not in ("", "None"),
                 "riempimento": (("FF" + a(interno, "Color").lstrip("#").upper()) if a(interno, "Color") else ""),
                 "apice": False, "formato_numero": a(formato, "Format")}
        notevole = any(v for k, v in stile.items() if k != "colore_font") or stile["colore_font"] not in ("", "FF000000")
        stili[st.get(f"{ss}ID", "")] = stile if notevole else None
    for i, ws in enumerate(root.findall(f"{ss}Worksheet")):
        if i >= lim["fogli_max"]:
            racc.tronca(f"oltre {lim['fogli_max']} fogli")
            break
        nascosto = ws.find(f"{{{NS_X}}}WorksheetOptions/{{{NS_X}}}Visible")
        stato = {"SheetHidden": "nascosto", "SheetVeryHidden": "molto_nascosto"}.get(
            (nascosto.text or "") if nascosto is not None else "", "visibile")
        info = {"indice": i, "nome": ws.get(f"{ss}Name", "")[:100], "stato": stato, "dimensione": "", "celle": 0,
                "unite": [], "troncato": False}
        racc.fogli.append(info)
        tabella = ws.find(f"{ss}Table")
        if tabella is None:
            continue
        r = 0
        n = 0
        rmax = cmax = 0
        for riga in tabella.findall(f"{ss}Row"):
            r = int(riga.get(f"{ss}Index", r + 1))
            c = 0
            for cel in riga.findall(f"{ss}Cell"):
                c = int(cel.get(f"{ss}Index", c + 1))
                across, down = int(cel.get(f"{ss}MergeAcross", "0") or 0), int(cel.get(f"{ss}MergeDown", "0") or 0)
                if (across or down):
                    if len(info["unite"]) < lim["unite_max"]:
                        info["unite"].append(f"{indirizzo(r, c)}:{indirizzo(r + down, c + across)}")
                    else:
                        info["troncato"] = True
                        racc.tronca(f"oltre {lim['unite_max']} celle unite in un foglio")
                d = cel.find(f"{ss}Data")
                formula = cel.get(f"{ss}Formula", "")
                if d is None:
                    d = next((x for x in cel if _ln(x.tag) == "Data"), None)
                grezzo = "".join(d.itertext()) if d is not None else ""
                tipo = {"String": "s", "Number": "n", "DateTime": "d", "Boolean": "b", "Error": "e"}.get(
                    d.get(f"{ss}Type", "String") if d is not None else "String", "s")
                if grezzo or formula:
                    href = cel.get(f"{ss}HRef", "")
                    racc.cella(i, r, c, tipo, grezzo, formula, bool(formula), stili.get(cel.get(f"{ss}StyleID", "")),
                               collegamento(href) if href else None)
                    n += 1
                    rmax, cmax = max(rmax, r), max(cmax, c)
                c += across
        info["celle"] = n
        info["dimensione"] = f"A1:{indirizzo(rmax, cmax)}" if n else ""
    return {"macro_presenti": False, "collegamenti_esterni": 0, "formato": "xls_xml"}


# ---------------------------------------------------------------- CSV

def _leggi_csv(dati: bytes, lim: dict, racc: _Raccolta) -> dict:
    if len(dati) > lim["parte_max"]:
        dati = dati[:lim["parte_max"]]
        racc.tronca(f"CSV oltre {lim['parte_max']} byte")
    try:
        testo = dati.decode("utf-8-sig")
        codifica = "utf-8"
    except UnicodeDecodeError:
        testo = dati.decode("cp1252", errors="replace")
        codifica = "cp1252"
    try:
        dialetto = csv.Sniffer().sniff(testo[:8192], delimiters=";,\t|")
        sep = dialetto.delimiter
    except csv.Error:
        sep = ";"
    info = {"indice": 0, "nome": "", "stato": "visibile", "dimensione": "", "celle": 0, "unite": [], "troncato": False}
    racc.fogli.append(info)
    n = rmax = cmax = 0
    for r, riga in enumerate(csv.reader(io.StringIO(testo, newline=""), delimiter=sep), start=1):
        for c, v in enumerate(riga, start=1):
            if v != "":
                racc.cella(0, r, c, "s", v)
                n += 1
                rmax, cmax = max(rmax, r), max(cmax, c)
    info["celle"] = n
    info["dimensione"] = f"A1:{indirizzo(rmax, cmax)}" if n else ""
    return {"macro_presenti": False, "collegamenti_esterni": 0, "formato": "csv", "codifica": codifica}


# ---------------------------------------------------------------- insieme

def intestazioni_candidate(celle: list[dict]) -> list[dict]:
    """Le righe (fra le prime RIGHE_INTESTAZIONE di ogni foglio) con almeno due celle di testo con un ruolo tipico di
    colonna e nessun numero: «Level | SUPPLIER PN | Description | NOTE». Un aiuto di lettura, non un'interpretazione."""
    per_riga: dict[tuple[int, int], list[dict]] = {}
    for c in celle:
        if c["r"] <= RIGHE_INTESTAZIONE:
            per_riga.setdefault((c["foglio"], c["r"]), []).append(c)
    out, per_foglio = [], {}
    for (f, r), cs in sorted(per_riga.items()):
        if any(c["tipo"] != "s" for c in cs):
            continue
        colonne = [{"c": c["c"], "ind": c["ind"], "testo": c["grezzo"][:200], "ruolo": ruolo_colonna(c["grezzo"])}
                   for c in sorted(cs, key=lambda x: x["c"])]
        if len({x["ruolo"] for x in colonne if x["ruolo"]}) < 2:
            continue
        if per_foglio.get(f, 0) >= INTESTAZIONI_PER_FOGLIO:
            continue
        per_foglio[f] = per_foglio.get(f, 0) + 1
        out.append({"foglio": f, "riga": r, "colonne": colonne})
    return out


def ha_intestazione_di_distinta(candidate: list[dict]) -> bool:
    """Una intestazione candidata con la quantita' e la posizione o il codice: una distinta."""
    for h in candidate:
        ruoli = {c["ruolo"] for c in h["colonne"]}
        if QUANTITA in ruoli and (POSIZIONE in ruoli or CODICE in ruoli):
            return True
    return False


def _fatto(lim: dict, esito: str, motivo: str = "", **altro) -> dict:
    f = {"versione": VERSIONE_FOGLIO_DATI, "esito": esito, "motivo": motivo[:300], "formato": "sconosciuto",
         "sha256": "", "codifica": "", "fogli": [], "celle": [], "stili": [], "intestazioni_candidate": [],
         "macro_presenti": False,
         "collegamenti_esterni": 0, "troncato": False,
         "limiti": {k: int(lim[k]) for k in ("byte_max", "parte_max", "rapporto_max", "fogli_max", "celle_max",
                                             "testo_max", "unite_max")}}
    f.update(altro)
    return f


def formato_di(dati: bytes, nome_file: str = "") -> str:
    """Il formato dai byte (non dall'estensione): xlsx (zip), xls_biff (OLE2), xls_xml (XML SpreadsheetML), html, csv
    (testo, solo con l'estensione .csv), sconosciuto."""
    if dati.startswith(b"PK\x03\x04"):
        return "xlsx"
    if dati.startswith(FIRMA_OLE2):
        return "xls_biff"
    testa = dati[:4096].lstrip(b"\xef\xbb\xbf \t\r\n").lower()
    if testa.startswith(b"<?xml") or testa.startswith(b"<workbook"):
        return "xls_xml" if b"urn:schemas-microsoft-com:office:spreadsheet" in dati[:65536].lower() else "xml"
    if testa.startswith(b"<html") or testa.startswith(b"<!doctype html") or b"<table" in testa:
        return "html"
    if nome_file.lower().endswith(".csv"):
        return "csv"
    return "sconosciuto"


def leggi_foglio_da_byte(dati: bytes, nome_file: str = "", limiti: dict | None = None) -> dict:
    """Il fatto `foglio_dati` di un contenuto. Non alza mai: un formato non supportato o un file illeggibile sono un
    ESITO con il motivo, mai un job fallito e mai un successo vuoto."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    sha = hashlib.sha256(dati).hexdigest()
    formato = formato_di(dati, nome_file)
    if len(dati) > lim["byte_max"]:
        return _fatto(lim, NON_LETTO, f"file di {len(dati)} byte: oltre il tetto di {lim['byte_max']}", formato=formato,
                      sha256=sha)
    if formato == "xls_biff":
        motivo = ("Excel 97-2003 binario (BIFF/OLE2): non supportato, il worker non ha una libreria per leggerlo"
                  if not nome_file.lower().endswith((".xlsx", ".xlsm")) else
                  "contenitore OLE2 con estensione xlsx: cartella cifrata con password, non supportata")
        return _fatto(lim, NON_SUPPORTATO, motivo, formato=formato, sha256=sha)
    if formato in ("html", "xml", "sconosciuto"):
        return _fatto(lim, NON_SUPPORTATO if formato != "sconosciuto" else NON_LETTO,
                      {"html": "pagina HTML con estensione da foglio di calcolo: non supportata",
                       "xml": "XML che non e' SpreadsheetML: non supportato",
                       "sconosciuto": "non e' XLSX, SpreadsheetML, BIFF ne' CSV"}[formato],
                      formato=formato, sha256=sha)
    racc = _Raccolta(lim)
    try:
        if formato == "xlsx":
            extra = _leggi_xlsx(dati, lim, racc)
        elif formato == "xls_xml":
            extra = _leggi_spreadsheetml(dati, lim, racc)
        else:
            extra = _leggi_csv(dati, lim, racc)
    except _Fermo:
        extra = {"formato": formato}
    except (zipfile.BadZipFile, ET.ParseError, ValueError, KeyError, OSError, RuntimeError, EOFError) as e:
        log.warning("foglio non letto: %s", e)
        if racc.celle:
            racc.tronca(f"lettura interrotta: {type(e).__name__}: {e}")
            extra = {"formato": formato}
        else:
            return _fatto(lim, NON_LETTO, f"lettura fallita: {type(e).__name__}: {e}", formato=formato, sha256=sha)
    candidate = intestazioni_candidate(racc.celle)
    return _fatto(lim, PARZIALE if racc.troncato else LETTO, "; ".join(racc.motivi), formato=extra.get("formato", formato),
                  sha256=sha, codifica=extra.get("codifica", "utf-8" if formato != "csv" else ""),
                  fogli=racc.fogli, celle=racc.celle, stili=racc.stili, intestazioni_candidate=candidate,
                  macro_presenti=bool(extra.get("macro_presenti")), collegamenti_esterni=int(extra.get("collegamenti_esterni", 0)),
                  troncato=racc.troncato)


def leggi_foglio(percorso: str, nome_file: str = "", limiti: dict | None = None) -> dict:
    """Il fatto `foglio_dati` del file (al piu' byte_max + 1 byte letti). Non alza mai."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    try:
        with open(percorso, "rb") as f:
            dati = f.read(int(lim["byte_max"]) + 1)
    except OSError as e:
        return _fatto(lim, NON_LETTO, f"file non letto: {e}")
    return leggi_foglio_da_byte(dati, nome_file or percorso, lim)
