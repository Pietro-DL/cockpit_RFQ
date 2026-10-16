"""Lettura del TESTO di un DWG AutoCAD R11/R12 (AC1009): testi, INSERT e blocchi, cartiglio e righe d'elenco (GP-3).

Che cosa fa e che cosa NON fa. Come dxf_testo per il DXF, il modulo dice che cosa c'e' scritto nel disegno: legge le
entita' TEXT, ATTRIB, ATTDEF e INSERT delle sezioni ENTITIES, BLOCKS ed «extra», ne fa le stesse entita' che produce
il lettore DXF (_Entita con i gruppi DXF: 1, 10, 20, 40, 50, 72, 73, 11, 21; 2, 41, 42 per gli INSERT) e da li' usa le
STESSE regole: risolvi_testi (INSERT annidati con le trasformazioni composte, profondita', cicli), campi_del_cartiglio
(la regola geometrica, con un vocabolario delle etichette che conosce anche le scritte dei cartigli italiani e
inglesi: «DWG NO.», «DESCRIZIONE», «MATERIALE», «SCALA»...) e le tabelle (tabelle_testo, come per i PDF). Non decide
che cosa sia un codice: i valori sono testo grezzo. Il fatto porta SOLO i campi riconosciuti e le righe delle tabelle
riconosciute, mai gli altri testi del disegno (ci sono nomi di persona), e le colonne personali restano vuote.

Il formato R12, come lo legge questo modulo (verificato sui DWG reali disponibili: ogni TEXT e quasi ogni INSERT
torna con la lunghezza dichiarata):
  - intestazione: la firma «AC1009» a 0; a 0x14 inizio e fine delle ENTITIES, inizio e dimensione dei BLOCKS (con un
    flag nei 2 bit alti), inizio e dimensione dell'«extra»; a 0x2c la tabella dei blocchi (dimensione della voce,
    numero, flag, indirizzo): ogni voce ha il flag, il nome (32 byte) e, a +35, l'offset del suo BLOCK nella sezione;
  - entita': tipo (RC, bit 0x80 = cancellata), flag (RC), lunghezza (RS, compreso il CRC finale di 2 byte), layer
    (RS), opzioni (RS); poi, secondo i flag: 0x40 un byte «extra» (subito), 0x01 il colore (RC), 0x02 il tipo di linea
    (RS), 0x04 l'elevazione (RD), 0x08 lo spessore (RD), 0x20 l'handle (RC lunghezza + byte); secondo l'extra: 0x04 un
    RS. Poi i campi propri: TEXT = punto (2RD), altezza (RD), testo (RS lunghezza + byte), e secondo le opzioni la
    rotazione in radianti, il fattore di larghezza, l'obliquita', lo stile, la generazione, l'allineamento orizzontale,
    il secondo punto, l'estrusione, l'allineamento verticale; INSERT = indice del blocco (RS), punto, e secondo le
    opzioni le scale, la rotazione in radianti, la scala z, righe e colonne (MINSERT: si legge la prima istanza);
  - ogni entita' letta si controlla: i campi devono finire esattamente prima del CRC. Una che non torna non si
    indovina: si conta in `non_decodificate` e il fatto e' `parziale`.
Un'entita' con dati estesi (extra 0x02) ha un formato che qui non si legge: per un INSERT, i cui campi hanno una
lunghezza nota, si leggono dalla fine; per un testo no (si conta). ATTRIB e ATTDEF non c'erano nei DWG reali visti:
la loro lettura segue lo stesso schema, provata solo su file costruiti.

Gli altri formati DWG (R13 e successivi, AC1012…AC1032) sono `non_supportato`, con la versione letta: mai un successo
vuoto. Un file che non comincia con una firma DWG e' `non_letto`.

Codifica: cp1252 (il codepage dell'intestazione R12 non si legge: lo dice `codifica`).
"""
from __future__ import annotations

import logging
import math
import struct

from dxf_testo import PROFONDITA_MAX, Testo, _Entita, campi_del_cartiglio, risolvi_testi
from tabelle_testo import LIMITI_TABELLE, trova_tabelle

log = logging.getLogger("worker-analisi")

VERSIONE_TESTO_DWG = 1

LIMITI_DEFAULT = {
    "byte_max": 64 * 1024 * 1024,   # byte del file letti al massimo
    "entita_max": 500_000,          # entita' scorse al massimo, in tutte le sezioni
    "testi_max": 5000,              # testi risolti (dopo gli INSERT) al massimo
    "testo_max": 512,               # caratteri conservati di un testo
    "campi_max": 24,                # campi del cartiglio
    "profondita_max": PROFONDITA_MAX,
    "inserti_max": 20000,
    "righe_max": LIMITI_TABELLE["righe_max"],
}

LETTO, NON_LETTO, NON_SUPPORTATO = "letto", "non_letto", "non_supportato"
CODIFICA = "cp1252"

VERSIONI = {"AC1009": "R11/R12", "AC1006": "R10", "AC1004": "R9", "AC1003": "2.6", "AC1002": "2.5",
            "AC1012": "R13", "AC1014": "R14", "AC1015": "2000", "AC1018": "2004", "AC1021": "2007",
            "AC1024": "2010", "AC1027": "2013", "AC1032": "2018"}

T_TEXT, T_BLOCK, T_ENDBLK, T_INSERT, T_ATTDEF, T_ATTRIB = 7, 12, 13, 14, 15, 16
NOMI = {T_TEXT: "TEXT", T_INSERT: "INSERT", T_ATTDEF: "ATTDEF", T_ATTRIB: "ATTRIB", T_BLOCK: "BLOCK"}

# Il vocabolario delle etichette del cartiglio per il DWG: quello del DXF (dxf_testo.ETICHETTE_DXF) piu' le scritte
# intere dei cartigli italiani e inglesi (come worker_analisi.ETICHETTE_CARTIGLIO, senza spazi, in maiuscolo).
ETICHETTE_DWG = (
    ("numero_disegno", ("DOC.NO.", "DOC.NO", "DOK.NR.", "DOK.NR", "DWGNO.", "DWGNO", "DWG.NO.", "DRAWINGNO.",
                        "DRAWINGNO", "DRAWINGNUMBER", "N.DISEGNO", "NUMERODISEGNO", "DISEGNON.", "DIS.N.", "N°DISEGNO")),
    ("codice", ("PARTNO.", "PARTNO", "PARTNUMBER", "MAT.NR.", "MAT.NR", "MATNR.", "MATNR", "CODICE", "COD.", "CODE",
                "ARTICOLO", "PARTN°", "PART.N°")),
    ("riferimento_3d", ("REF.3DDOC.", "REF.3DDOC", "REF.3DDOK.", "REF.3DDOK")),
    ("revisione", ("REV.", "REV", "REVISIONE", "REVISION", "EDIZIONE")),
    ("titolo", ("DESCRIZIONE", "DENOMINAZIONE", "DESCRIPTION", "TITOLO", "TITLE")),
    ("materiale", ("MATERIALE", "MATERIAL")),
    ("scala", ("SCALA", "SCALE")),
)
VOCI_DWG = {campo: set(scritte) for campo, scritte in ETICHETTE_DWG}


class _Lettore:
    """Le entita' di un DWG R12 letto da memoria, con i conti."""

    def __init__(self, dati: bytes, lim: dict):
        self.b = dati
        self.lim = lim
        self.scorse = 0
        self.cancellate = 0
        self.non_decodificate = 0
        self.con_eed = 0
        self.motivi: list[str] = []
        self.fermo = False
        self.nomi_blocchi: list[str] = []
        self.offset_blocchi: dict[int, str] = {}

    # ---- primitive con il controllo dei confini
    def rs(self, p: int) -> int:
        return struct.unpack_from("<H", self.b, p)[0]

    def rl(self, p: int) -> int:
        return struct.unpack_from("<I", self.b, p)[0]

    def rd(self, p: int) -> float:
        v = struct.unpack_from("<d", self.b, p)[0]
        if not math.isfinite(v):
            raise ValueError("numero non finito")
        return v

    def tv(self, p: int, fine: int) -> tuple[str, int]:
        n = self.rs(p)
        if p + 2 + n > fine:
            raise ValueError("testo oltre l'entita'")
        return self.b[p + 2:p + 2 + n].split(b"\x00")[0].decode(CODIFICA, errors="replace"), p + 2 + n

    def motivo(self, m: str) -> None:
        if m not in self.motivi:
            self.motivi.append(m)

    # ---- intestazione e tabella dei blocchi
    def tabella_blocchi(self) -> None:
        dim, n, _flag, ind = struct.unpack_from("<HHHI", self.b, 0x2c)
        if dim < 39 or ind + dim * n > len(self.b):
            if n:
                self.motivo("tabella dei blocchi fuori dal file")
            return
        for i in range(n):
            q = ind + i * dim
            nome = self.b[q + 1:q + 33].split(b"\x00")[0].decode(CODIFICA, errors="replace")
            self.nomi_blocchi.append(nome)
            self.offset_blocchi[self.rl(q + 35) & 0x3FFFFFFF] = nome

    # ---- un'entita'
    def testa(self, q: int) -> tuple[int, int, int, int, int, int]:
        """(tipo, flag, lunghezza, opzioni, extra, inizio dei campi propri)."""
        t, f, size = self.b[q], self.b[q + 1], self.rs(q + 2)
        p = q + 6
        opts = self.rs(p)
        p += 2
        extra = 0
        if f & 0x40:
            extra = self.b[p]
            p += 1
        if f & 0x01:
            p += 1
        if f & 0x02:
            p += 2
        if f & 0x04:
            p += 8
        if f & 0x08:
            p += 8
        if f & 0x20:
            p += 1 + self.b[p]
        if extra & 0x04:
            p += 2
        return t, f, size, opts, extra, p

    def campi_testo(self, tipo: int, p: int, fine: int, opts: int) -> tuple[dict, int]:
        """I campi di TEXT, ATTRIB, ATTDEF: (gruppi DXF, fine dei campi)."""
        g: dict[str, str] = {}
        x, y, h = self.rd(p), self.rd(p + 8), self.rd(p + 16)
        p += 24
        testo, p = self.tv(p, fine)
        if tipo == T_ATTRIB:
            _tag, p = self.tv(p, fine)
            p += 1                                  # flag dell'attributo
        elif tipo == T_ATTDEF:
            _prompt, p = self.tv(p, fine)
            _tag, p = self.tv(p, fine)
            p += 1
        g.update({"1": testo, "10": repr(x), "20": repr(y), "40": repr(h)})
        if opts & 1:
            g["50"] = repr(math.degrees(self.rd(p)))
            p += 8
        if opts & 2:
            p += 8
        if opts & 4:
            p += 8
        if opts & 8:
            p += 1
        if opts & 16:
            p += 1
        if opts & 32:
            g["72"] = str(self.b[p])
            p += 1
        if opts & 64:
            g["11"], g["21"] = repr(self.rd(p)), repr(self.rd(p + 8))
            p += 16
        if opts & 128:
            p += 24
        if opts & 256:
            g["74" if tipo == T_ATTRIB else "73"] = str(self.b[p])
            p += 1
        return g, p

    def campi_insert(self, p: int, opts: int) -> tuple[int, dict, int]:
        """(indice del blocco, gruppi DXF, fine dei campi) di un INSERT."""
        i = self.rs(p)
        g = {"10": repr(self.rd(p + 2)), "20": repr(self.rd(p + 10))}
        p += 18
        if opts & 1:
            g["41"] = repr(self.rd(p))
            p += 8
        if opts & 2:
            g["42"] = repr(self.rd(p))
            p += 8
        if opts & 4:
            g["50"] = repr(math.degrees(self.rd(p)))
            p += 8
        for bit, n in ((8, 8), (16, 2), (32, 2), (64, 8), (128, 8), (256, 24)):
            if opts & bit:
                p += n
        return i, g, p

    @staticmethod
    def lunghezza_insert(opts: int) -> int:
        n = 18
        for bit, k in ((1, 8), (2, 8), (4, 8), (8, 8), (16, 2), (32, 2), (64, 8), (128, 8), (256, 24)):
            if opts & bit:
                n += k
        return n

    def sezione(self, s: int, e: int, nome: str, dest: list, blocchi: dict | None) -> None:
        """Le entita' della sezione [s, e): i testi e gli INSERT in `dest` (ENTITIES, extra) o nei blocchi (BLOCKS)."""
        if not (0 < s <= e <= len(self.b)):
            if e > s:
                self.motivo(f"sezione {nome} fuori dal file")
            return
        q = s
        blocco: dict | None = None
        nome_blocco = ""
        while q + 4 <= e:
            if self.scorse >= self.lim["entita_max"]:
                self.fermo = True
                self.motivo(f"oltre {self.lim['entita_max']} entita'")
                return
            t, f, size = self.b[q], self.b[q + 1], self.rs(q + 2)
            if size < 6 or q + size > e:
                self.motivo(f"{nome}: entita' non valida a 0x{q:x}: il resto della sezione non si legge")
                self.fermo = True
                return
            self.scorse += 1
            tipo = t & 0x7F
            if t & 0x80:
                self.cancellate += 1
                q += size
                continue
            if f & 0x10 or f & 0x80:
                if tipo in NOMI:
                    self.non_decodificate += 1
                    self.motivo(f"flag d'entita' sconosciuti (0x{f:02x})")
                q += size
                continue
            fine = q + size - 2
            try:
                if tipo == T_BLOCK and blocchi is not None:
                    _, _, _, opts, extra, p = self.testa(q)
                    base = (self.rd(p), self.rd(p + 8))
                    nome_blocco = self.offset_blocchi.get(q - s, "")
                    if not nome_blocco and opts & 4:
                        nome_blocco, _ = self.tv(p + 16, fine)
                    blocco = {"base": base, "elementi": [], "annidati": 0}
                    if nome_blocco and nome_blocco not in blocchi:
                        blocchi[nome_blocco] = blocco
                    else:
                        blocco = None               # senza nome o ripetuto: non serve
                elif tipo == T_ENDBLK:
                    blocco, nome_blocco = None, ""
                elif tipo in (T_TEXT, T_ATTRIB, T_ATTDEF, T_INSERT):
                    e_ = self.entita(q, tipo, fine, nome, nome_blocco)
                    if e_ is not None:
                        if blocchi is None:
                            dest.append(e_)
                        elif blocco is not None:
                            if tipo == T_INSERT:
                                blocco["annidati"] += 1
                            blocco["elementi"].append(e_)
            except (struct.error, ValueError, IndexError):
                if tipo in NOMI:
                    self.non_decodificate += 1
            q += size

    def entita(self, q: int, tipo: int, fine: int, sezione: str, blocco: str) -> _Entita | None:
        _, _, _, opts, extra, p = self.testa(q)
        origine = {"offset_entita": q, "sezione": sezione, "blocco": blocco, "tipo_entita": NOMI[tipo]}
        if extra & 0x02:
            self.con_eed += 1
            if tipo != T_INSERT:
                self.non_decodificate += 1
                self.motivo("testi con dati estesi: non letti")
                return None
            p = fine - self.lunghezza_insert(opts)   # i campi dell'INSERT hanno una lunghezza nota: dalla fine
        if tipo == T_INSERT:
            i, g, fp = self.campi_insert(p, opts)
            if fp != fine or i >= len(self.nomi_blocchi):
                self.non_decodificate += 1
                return None
            g["2"] = self.nomi_blocchi[i]
            e = _Entita("INSERT")
        else:
            g, fp = self.campi_testo(tipo, p, fine, opts)
            if fp != fine:
                self.non_decodificate += 1
                return None
            if tipo == T_ATTDEF:
                return None                         # la definizione di un attributo e' un modello, non un testo
            e = _Entita("ATTRIB" if tipo == T_ATTRIB else "TEXT")
        e.g = g
        e.origine = origine
        return e


def _fatto(lim: dict, esito: str, motivo: str = "", **altro) -> dict:
    f = {"versione": VERSIONE_TESTO_DWG, "esito": esito, "motivo": motivo[:300], "versione_dwg": "", "codifica": "",
         "entita": 0, "testi": 0, "cancellate_ignorate": 0, "non_decodificate": 0, "con_dati_estesi": 0,
         "annidati": 0, "annidati_seguiti": 0, "oltre_profondita": 0, "cicli": 0, "specchiati": 0,
         "troncato": False, "parziale": False, "cartiglio": [], "tabelle": [],
         "limiti": {k: int(lim[k]) for k in LIMITI_DEFAULT}}
    f.update(altro)
    return f


def _cartiglio(campi: list[dict]) -> list[dict]:
    """I campi con la sorgente nel file (offset dell'entita' del valore e di quella dell'etichetta). Un valore senza
    lettere ne' cifre («.», il segnaposto delle celle vuote di un modello di cartiglio) non e' un valore: si toglie."""
    out = []
    for c in campi:
        ov, oe = c.pop("_origine_valore", None) or {}, c.pop("_origine_etichetta", None) or {}
        if not any(ch.isalnum() for ch in c["valore"]):
            continue
        c["sorgente"] = {"offset_entita": int(ov.get("offset_entita", -1)), "sezione": ov.get("sezione", ""),
                         "blocco": ov.get("blocco", ""), "tipo_entita": ov.get("tipo_entita", ""),
                         "offset_etichetta": int(oe.get("offset_entita", -1))}
        out.append(c)
    return out


class _Parola:
    """Un testo del disegno come «parola» per le tabelle (tabelle_testo): y verso il basso, con l'offset d'origine."""
    __slots__ = ("x0", "y0", "x1", "y1", "testo", "sorgente")

    def __init__(self, t: Testo):
        self.x0, self.x1 = t.x0, t.x0 + max(t.w, 0.1)
        self.y0, self.y1 = -(t.cy + t.h / 2), -(t.cy - t.h / 2)
        self.testo = t.testo
        self.sorgente = [t.origine["offset_entita"]] if t.origine else []


def _riquadro_disegno(r: list[float]) -> list[float]:
    """Un riquadro delle tabelle (y verso il basso) riportato nel disegno (y verso l'alto)."""
    return [r[0], round(-r[3], 2), r[2], round(-r[1], 2)] if len(r) == 4 else r


def _tabelle(testi: list[Testo], lim: dict) -> tuple[list[dict], bool]:
    """Le tabelle (elenco particolari) dei testi diritti, gruppo per gruppo (spazio), con i riquadri nel disegno."""
    gruppi: dict = {}
    visti = set()
    for t in testi:
        if isinstance(t.spazio, tuple) and (t.spazio[1] or t.spazio[2]):
            continue
        # lo stesso testo nello stesso posto (un blocco inserito piu' volte nello stesso punto) conta una volta
        k = (t.testo, round(t.x0, 1), round(t.cy, 1), round(t.h, 2), t.spazio)
        if k in visti:
            continue
        visti.add(k)
        gruppi.setdefault(t.spazio, []).append(_Parola(t))
    tabelle, troncato = [], False
    for _, parole in sorted(gruppi.items(), key=lambda x: str(x[0])):
        trovate, fermo = trova_tabelle(parole, 1, {"righe_max": lim["righe_max"]},
                                       gia=sum(len(x["righe"]) for x in tabelle))
        troncato = troncato or fermo
        for tab in trovate:
            tab.pop("_fino_in_fondo", None)   # un disegno ha una pagina sola
            tab["riquadro_intestazione"] = _riquadro_disegno(tab["riquadro_intestazione"])
            for c in tab["colonne"]:
                c["riquadro"] = _riquadro_disegno(c["riquadro"])
            for r in tab["righe"]:
                r["riquadro"] = _riquadro_disegno(r["riquadro"])
                for c in r["celle"]:
                    c["riquadro"] = _riquadro_disegno(c["riquadro"])
            # «sopra» e «sotto» con la y verso il basso: nel disegno restano gli stessi (le righe sopra l'intestazione)
            tabelle.append(tab)
    for k, t in enumerate(tabelle):
        t["indice"] = k + 1
    return tabelle, troncato


def leggi_testo_dwg_da_byte(dati: bytes, limiti: dict | None = None) -> dict:
    """Il fatto `testo_dwg` di un contenuto DWG. Non alza mai: un formato diverso da R12 e' `non_supportato` con la
    versione, un file che non e' un DWG o una lettura fallita sono `non_letto` con il motivo."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    troncato = False
    if len(dati) > lim["byte_max"]:
        dati, troncato = dati[:int(lim["byte_max"])], True
    firma = dati[:6].decode("ascii", errors="replace")
    if not firma.startswith("AC") or not firma[2:].isdigit():
        return _fatto(lim, NON_LETTO, "non comincia con una firma DWG (AC10xx)")
    if firma != "AC1009":
        nome = VERSIONI.get(firma, "sconosciuta")
        return _fatto(lim, NON_SUPPORTATO, f"DWG {nome} ({firma}): si leggono solo i DWG R11/R12 (AC1009)",
                      versione_dwg=firma)
    if len(dati) < 0x60:
        return _fatto(lim, NON_LETTO, "intestazione R12 incompleta", versione_dwg=firma)
    try:
        lt = _Lettore(dati, lim)
        ent_s, ent_e, blk_s, blk_dim, ext_s, ext_dim = struct.unpack_from("<IIIIII", dati, 0x14)
        lt.tabella_blocchi()
        blocchi: dict[str, dict] = {}
        entita: list[_Entita] = []
        lt.sezione(blk_s, blk_s + (blk_dim & 0x3FFFFFFF), "BLOCKS", [], blocchi)
        lt.sezione(ent_s, ent_e, "ENTITIES", entita, None)
        if ext_s and (ext_dim & 0x3FFFFFFF):
            lt.sezione(ext_s, ext_s + (ext_dim & 0x3FFFFFFF), "EXTRA", entita, None)
        lim_r = {"testi_max": lim["testi_max"], "testo_max": lim["testo_max"], "profondita_max": lim["profondita_max"],
                 "inserti_max": lim["inserti_max"]}
        r = risolvi_testi(entita, blocchi, lim_r)
        campi, pieno = campi_del_cartiglio(r.testi, int(lim["campi_max"]), VOCI_DWG, origini=True,
                                           impilati=("numero_disegno", "codice"))
        tabelle, tab_troncate = _tabelle(r.testi, lim)
    except Exception as e:  # noqa: BLE001 — una lettura fallita e' un esito, mai un job fallito
        log.warning("lettura DWG fallita: %s", e)
        return _fatto(lim, NON_LETTO, f"lettura fallita: {type(e).__name__}: {e}", versione_dwg=firma)
    if lt.scorse == 0:
        return _fatto(lim, NON_LETTO, "nessuna entita' leggibile: " + "; ".join(lt.motivi), versione_dwg=firma)
    tronc = troncato or lt.fermo or r.fermo or pieno or tab_troncate
    motivi = list(lt.motivi) + list(r.motivi)
    if lt.non_decodificate:
        motivi.insert(0, f"{lt.non_decodificate} entita' di testo o INSERT non decodificate")
    if troncato or r.fermo or pieno or tab_troncate:
        motivi.insert(0, "fermato su un tetto (byte, testi, INSERT, campi o righe)")
    return _fatto(lim, LETTO, "; ".join(motivi), versione_dwg=firma, codifica=CODIFICA, entita=lt.scorse,
                  testi=len(r.testi), cancellate_ignorate=lt.cancellate, non_decodificate=lt.non_decodificate,
                  con_dati_estesi=lt.con_eed, annidati=r.annidati, annidati_seguiti=r.annidati_seguiti,
                  oltre_profondita=r.oltre_profondita, cicli=r.cicli, specchiati=r.specchiati, troncato=tronc,
                  parziale=bool(tronc or lt.non_decodificate or r.oltre_profondita or r.cicli or r.specchiati),
                  cartiglio=_cartiglio(campi), tabelle=tabelle)


def leggi_testo_dwg(percorso: str, limiti: dict | None = None) -> dict:
    """Il fatto `testo_dwg` del file (al piu' byte_max + 1 byte letti). Non alza mai."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    try:
        with open(percorso, "rb") as f:
            dati = f.read(int(lim["byte_max"]) + 1)
    except OSError as e:
        return _fatto(lim, NON_LETTO, f"file non letto: {e}")
    return leggi_testo_dwg_da_byte(dati, lim)

