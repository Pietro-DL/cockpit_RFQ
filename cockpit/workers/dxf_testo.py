"""Lettura del TESTO di un DXF ASCII: i testi del disegno e i campi del cartiglio che si riconoscono (K4).

Che cosa fa e che cosa NON fa. Come step_struttura per lo STEP, questo modulo dice che cosa c'e' scritto nel
file: i testi (TEXT, MTEXT, ATTRIB) delle ENTITIES e dei BLOCKS richiamati dalle ENTITIES con un INSERT, portati
nelle coordinate del disegno, e da quelli i campi di un cartiglio con le etichette che conosce. Non decide se un
valore e' un codice: il valore resta testo GREZZO, e che cosa sia lo dicono le regole del cliente della RFQ, sul
server (A5.13.7). Il fatto porta SOLO i campi riconosciuti, mai gli altri testi del disegno: nel cartiglio ci
sono i nomi delle persone («approved by», «drawn»), e i fatti restano archiviati per anni (come MetadatiPDF senza
l'autore).

Il cartiglio di un DXF non e' una riga di parole come in un PDF: etichetta e valore sono spesso due entita'
separate, la piccola etichetta in alto nella cella e il valore sotto, piu' grande. La regola e' geometrica:

  - l'etichetta si riconosce sul testo INTERO della entita' (senza badare a maiuscole e spazi), con la gemella
    dopo la barra della stessa voce: «part no./Mat. Nr.» e' il codice, «doc. no./Dok. Nr.» il numero del
    disegno, «ref.3D doc./Ref.3D Dok.» il riferimento al documento 3D, «rev./Rev.» la revisione;
  - il valore e' il testo non-etichetta piu' vicino SOTTO l'etichetta, con il centro fra 0 (escluso) e 5 altezze
    dell'etichetta piu' in basso e l'inizio fra 1 altezza a sinistra e 2 a destra dell'inizio dell'etichetta.
    Centro e inizio di un testo si stimano dall'attacco (MTEXT 71, TEXT 72/73): un valore centrato nella cella
    («00» sotto «rev./Rev.») ha il punto d'inserimento a meta' della sua larghezza, che si stima dal numero dei
    caratteri;
  - la ricerca del valore non passa nella cella sotto: un testo della finestra alto come l'etichetta (entro un
    quarto della sua altezza) e' l'etichetta della cella dopo, e ferma la ricerca. Se il testo piu' vicino e' li'
    o dopo, la cella del valore e' vuota e il campo non si riporta (sotto ci sono anche i nomi delle persone);
  - la revisione e' quella della riga del numero del disegno: «rev./Rev.» conta solo se l'etichetta riconosciuta
    piu' vicina alla sua sinistra, sulla stessa riga, e' «doc. no.». Nei cartigli che hanno anche la revisione
    del documento 3D («rev./Rev.» accanto a «ref.3D doc.») quella non si riporta: non e' la revisione di questo
    disegno;
  - l'intestazione di una tabella non ha etichette, come nel testo dei PDF (worker_analisi.intestazione_tabella):
    se la riga vista di un'etichetta (i testi con il centro entro mezza altezza, nello stesso spazio) ha anche la
    colonna della posizione e quella della quantita', e' l'elenco particolari di un assieme («POS. PART NO. QTY»),
    e «PART NO.» li' e' il nome di una colonna, non il codice del cartiglio.

Nessuna dipendenza nuova: un DXF ASCII e' una sequenza di coppie (codice di gruppo, valore), una per due righe.
Limiti e tagli, dichiarati nel fatto:
  - un DXF BINARIO non si legge: e' un esito («non_letto», con il motivo), non un errore del job;
  - si legge al piu' byte_max byte del file; oltre, la lettura e' troncata;
  - si risolvono al piu' testi_max testi e si conservano testo_max caratteri per testo; gia' durante la lettura
    si tengono al piu' FATTORE_LETTI * testi_max testi e INSERT delle ENTITIES e altrettanti testi e blocchi dei
    BLOCKS (la memoria non cresce con il file): oltre, la lettura e' troncata;
  - un INSERT dentro un blocco (il secondo livello di annidamento) non si segue: si conta in `annidati`;
  - la rotazione propria dei testi non si considera (i cartigli sono orizzontali). Un INSERT ruotato (non di un
    giro intero) non si segue: con il blocco girato «sotto» non e' piu' sotto, e la regola prenderebbe un altro
    testo per il valore. I suoi testi non sono letti, e il fatto lo dice nel `motivo` (con l'esito «letto»);
    la scala e la base dell'INSERT si applicano alle posizioni;
  - un DXF ASCII senza testi e senza blocchi e' letto (con il cartiglio vuoto) se si e' vista almeno una SECTION
    o la coppia di fine (0, EOF); senza nessuna delle due il file non sembra un DXF ASCII.

Codifica: UTF-8 dal formato AC1021 (AutoCAD 2007) in poi; prima, la code page di $DWGCODEPAGE (ANSI_1252 ->
cp1252, la piu' comune), e cp1252 quando manca o non si conosce.
"""
from __future__ import annotations

import io
import logging
import math
import re

log = logging.getLogger("worker-analisi")

VERSIONE_TESTO_DXF = 1

LIMITI_DEFAULT = {
    "byte_max": 64 * 1024 * 1024,  # byte del file letti al massimo
    "testi_max": 5000,             # testi risolti (dopo gli INSERT) al massimo
    "testo_max": 512,              # caratteri conservati di un testo
    "campi_max": 24,               # campi del cartiglio nel fatto
}
# Quanti elementi si tengono durante la lettura, in multipli di testi_max: testi e INSERT delle ENTITIES da una
# parte, testi e blocchi dei BLOCKS dall'altra (le quote di un disegno sono blocchi con un testo ciascuno).
FATTORE_LETTI = 4
# Un INSERT con la rotazione entro questi gradi da un giro intero non e' ruotato.
ROTAZIONE_TOLLERATA = 0.5
VALORE_MAX = 120  # caratteri del valore di un campo, come TESTO_PDF_VALORE_MAX

DXF_LETTO = "letto"
DXF_NON_LETTO = "non_letto"

FIRMA_BINARIA = b"AutoCAD Binary DXF"

# La larghezza media di un carattere, in altezze del testo: serve solo a stimare dove comincia un testo centrato
# o allineato a destra. Un font tecnico sta fra 0,5 e 0,7.
LARGHEZZA_CARATTERE = 0.6

# Le etichette del cartiglio, per voce: le scritte senza spazi, in maiuscolo. Un testo e' l'etichetta di una voce
# quando ogni suo pezzo fra le barre e' una scritta di quella voce («PART NO./MAT. NR.» -> PARTNO., MAT.NR.).
ETICHETTE_DXF = (
    ("codice", ("PARTNO.", "PARTNO", "MAT.NR.", "MAT.NR", "MATNR.", "MATNR")),
    ("numero_disegno", ("DOC.NO.", "DOC.NO", "DOK.NR.", "DOK.NR")),
    ("riferimento_3d", ("REF.3DDOC.", "REF.3DDOC", "REF.3DDOK.", "REF.3DDOK")),
    ("revisione", ("REV.", "REV")),
)

# Le entita' che si leggono. Per ognuna i gruppi che servono; 1 e 3 (il testo) si tengono tutti, in ordine.
_GRUPPI = {
    "TEXT": {"1", "10", "20", "11", "21", "40", "72", "73", "67"},
    "ATTRIB": {"1", "10", "20", "11", "21", "40", "72", "74", "67"},
    "MTEXT": {"1", "3", "10", "20", "40", "71", "67"},
    "INSERT": {"2", "10", "20", "41", "42", "50", "67"},
    "BLOCK": {"2", "10", "20"},
}
_TESTI = ("TEXT", "ATTRIB", "MTEXT")

_RE_SPAZI = re.compile(r"\s+")
_RE_UNICODE = re.compile(r"\\U\+([0-9A-Fa-f]{4})")
_RE_PERCENTO = re.compile(r"%%(\d{3}|.)", re.S)
_PERCENTO = {"d": "°", "p": "±", "c": "Ø", "%": "%"}


def pulisci_mtext(s: str) -> str:
    """Il testo di un MTEXT senza i codici di formato: \\P, \\N e \\~ diventano spazi; \\f..; \\H..; \\W..; \\C..;
    \\Q..; \\T..; \\A..; \\p..; e simili si tolgono con il loro argomento; \\L \\O \\K (sottolineato, sopralineato,
    barrato) e le graffe si tolgono; \\S..; (la frazione) resta come «a/b»; \\U+XXXX diventa il suo carattere; \\\\,
    \\{ e \\} restano come carattere."""
    out: list[str] = []
    i, n = 0, len(s)
    while i < n:
        c = s[i]
        if c == "\\" and i + 1 < n:
            k = s[i + 1]
            if k in "PNX~":
                out.append(" ")
                i += 2
            elif k in "\\{}":
                out.append(k)
                i += 2
            elif k in "LlOoKk":
                i += 2
            elif k == "U" and _RE_UNICODE.match(s, i):
                out.append(chr(int(s[i + 3:i + 7], 16)))
                i += 7
            elif k == "S":
                j = s.find(";", i + 2)
                corpo = s[i + 2:j] if j >= 0 else s[i + 2:]
                out.append(corpo.replace("^", "/").replace("#", "/"))
                i = j + 1 if j >= 0 else n
            elif k in "fFhHwWcCqQtTaApl":
                j = s.find(";", i + 2)
                i = j + 1 if j >= 0 else n
            else:
                out.append(k)
                i += 2
        elif c in "{}":
            i += 1
        else:
            out.append(c)
            i += 1
    return _RE_SPAZI.sub(" ", "".join(out)).strip()


def pulisci_text(s: str) -> str:
    """Il testo di un TEXT o di un ATTRIB senza i codici di controllo: %%d -> °, %%p -> ±, %%c -> Ø, %%nnn -> il
    carattere nnn, %%u/%%o/%%k (le linee) si tolgono; \\U+XXXX diventa il suo carattere."""
    def sostituisci(m: re.Match) -> str:
        x = m.group(1)
        if len(x) == 3:
            return chr(int(x))
        return _PERCENTO.get(x.lower(), "")
    s = _RE_PERCENTO.sub(sostituisci, s)
    s = _RE_UNICODE.sub(lambda m: chr(int(m.group(1), 16)), s)
    return _RE_SPAZI.sub(" ", s).strip()


def _num(g: dict, codice: str, predefinito: float = 0.0) -> float:
    try:
        v = float(g.get(codice, predefinito))
    except (TypeError, ValueError):
        return predefinito
    return v if math.isfinite(v) else predefinito


def _intero(g: dict, codice: str) -> int:
    try:
        return int(float(g.get(codice, 0)))
    except (TypeError, ValueError):
        return 0


def codifica_del_dxf(testa: bytes) -> tuple[str, str]:
    """(versione del formato, codifica) dall'intestazione: $ACADVER e $DWGCODEPAGE stanno nei primi KB."""
    righe = testa.decode("latin-1").splitlines()
    var = {}
    for i in range(len(righe) - 3):
        if righe[i].strip() == "9" and righe[i + 1].strip() in ("$ACADVER", "$DWGCODEPAGE"):
            var[righe[i + 1].strip()] = righe[i + 3].strip()
    versione = var.get("$ACADVER", "")
    if versione >= "AC1021":
        return versione, "utf-8"
    pagina = var.get("$DWGCODEPAGE", "").upper()
    if pagina.startswith("ANSI_"):
        nome = "cp" + pagina[len("ANSI_"):]
        try:
            "".encode(nome)
            return versione, nome
        except LookupError:
            pass
    return versione, "cp1252"


def _coppie(righe):
    """Le coppie (codice di gruppo, valore) del file; il valore senza il fine riga, con gli spazi che ha."""
    while True:
        codice = next(righe, None)
        valore = next(righe, None)
        if codice is None or valore is None:
            return
        yield codice.strip(), valore.rstrip("\r\n")


class _Entita:
    __slots__ = ("tipo", "g", "testo")

    def __init__(self, tipo: str):
        self.tipo = tipo
        self.g: dict[str, str] = {}
        self.testo: list[str] = []  # i gruppi 3 e poi 1, nell'ordine del file

    def aggiungi(self, codice: str, valore: str) -> None:
        if codice not in _GRUPPI[self.tipo]:
            return
        if codice in ("1", "3"):
            if codice == "3":
                self.testo.append(valore)
            else:
                self.g.setdefault("1", valore)
            return
        self.g.setdefault(codice, valore.strip())

    def scritto(self) -> str:
        if self.tipo == "MTEXT":
            return pulisci_mtext("".join(self.testo) + self.g.get("1", ""))
        return pulisci_text(self.g.get("1", ""))


class _Letti:
    """Che cosa ha dato la lettura delle coppie: le ENTITIES (testi e INSERT nell'ordine del file), i BLOCKS (nome ->
    base, testi, quanti INSERT), se si e' vista una SECTION o la coppia di fine (`dxf`), e se ci si e' fermati al
    tetto degli elementi tenuti (`pieno`)."""
    __slots__ = ("entita", "blocchi", "dxf", "pieno")

    def __init__(self):
        self.entita: list[_Entita] = []
        self.blocchi: dict[str, dict] = {}
        self.dxf = False
        self.pieno = False


def _leggi_entita(righe, massimo: int) -> _Letti:
    """Le ENTITIES e i BLOCKS del file, tenendo al piu' `massimo` elementi per parte (testi e INSERT delle ENTITIES;
    testi e blocchi dei BLOCKS): oltre si continua a leggere le coppie, senza tenere niente, e `pieno` e' vero."""
    letti = _Letti()
    entita, blocchi = letti.entita, letti.blocchi
    nei_blocchi = 0  # i testi e i blocchi tenuti dei BLOCKS
    sezione = ""
    attesa_sezione = False
    blocco: dict | None = None
    cur: _Entita | None = None
    for codice, valore in _coppie(righe):
        if codice == "0":
            if cur is not None and cur.tipo == "BLOCK":
                blocco = {"base": (_num(cur.g, "10"), _num(cur.g, "20")), "testi": [], "annidati": 0}
                nome = cur.g.get("2", "")
                if nome in blocchi:
                    blocco = None  # un nome ripetuto: vale il primo blocco, i testi di questo non servono
                elif nei_blocchi < massimo:
                    blocchi[nome] = blocco
                    nei_blocchi += 1
                else:
                    letti.pieno, blocco = True, None
            elif cur is not None:
                if sezione == "ENTITIES":
                    if len(entita) < massimo:
                        entita.append(cur)
                    else:
                        letti.pieno = True
                elif sezione == "BLOCKS" and blocco is not None:
                    if cur.tipo == "INSERT":
                        blocco["annidati"] += 1
                    elif nei_blocchi < massimo:
                        blocco["testi"].append(cur)
                        nei_blocchi += 1
                    else:
                        letti.pieno = True
            cur = None
            tipo = valore.strip()
            if tipo == "SECTION":
                attesa_sezione = True
                letti.dxf = True
            elif tipo == "ENDSEC":
                sezione = ""
            elif tipo == "ENDBLK":
                blocco = None
            elif tipo == "EOF":
                letti.dxf = True
                break
            elif tipo == "BLOCK" and sezione == "BLOCKS":
                cur = _Entita("BLOCK")
            elif tipo in _GRUPPI and tipo != "BLOCK" and sezione in ("ENTITIES", "BLOCKS"):
                cur = _Entita(tipo)
            continue
        if attesa_sezione:
            if codice == "2":
                sezione = valore.strip()
            attesa_sezione = False
            continue
        if cur is not None:
            cur.aggiungi(codice, valore)
    return letti


class Testo:
    """Un testo risolto nelle coordinate del disegno: l'inizio stimato (x0), il centro verticale (cy), l'altezza,
    la larghezza stimata, il testo pulito e lo spazio (0 modello, 1 carta)."""
    __slots__ = ("x0", "cy", "h", "w", "testo", "spazio")

    def __init__(self, x0: float, cy: float, h: float, w: float, testo: str, spazio: int):
        self.x0, self.cy, self.h, self.w, self.testo, self.spazio = x0, cy, h, w, testo, spazio


def _ancora(e: _Entita, testo: str) -> tuple[float, float, float, float]:
    """(x0, cy, h, w) nelle coordinate dell'entita': l'inizio e il centro verticale stimati dall'attacco."""
    g = e.g
    h = abs(_num(g, "40"))
    w = len(testo) * LARGHEZZA_CARATTERE * h
    if e.tipo == "MTEXT":
        x, y = _num(g, "10"), _num(g, "20")
        a = _intero(g, "71") or 1
        colonna, riga = (a - 1) % 3, min((a - 1) // 3, 2)
        x0 = x - (w / 2 if colonna == 1 else w if colonna == 2 else 0.0)
        cy = y - h / 2 if riga == 0 else y if riga == 1 else y + h / 2
        return x0, cy, h, w
    orizz = _intero(g, "72")
    vert = _intero(g, "74" if e.tipo == "ATTRIB" else "73")
    if orizz in (0, 3, 5) and vert == 0:
        # sulla linea di base, da sinistra (o «allineato»/«adatta», che vanno dal primo punto al secondo)
        return _num(g, "10"), _num(g, "20") + h / 2, h, w
    x, y = _num(g, "11", _num(g, "10")), _num(g, "21", _num(g, "20"))
    x0 = x - (w / 2 if orizz in (1, 4) else w if orizz == 2 else 0.0)
    if orizz == 4 or vert == 2:
        cy = y
    elif vert == 3:
        cy = y - h / 2
    else:
        cy = y + h / 2
    return x0, cy, h, w


def _trasformazione(ins: _Entita, base: tuple[float, float]):
    """La funzione che porta un punto del blocco nel disegno: meno la base del blocco, la scala (41, 42), la
    rotazione (50, in gradi) e il punto d'inserimento (10, 20). Con la scala verticale delle altezze."""
    g = ins.g
    sx, sy = _num(g, "41", 1.0) or 1.0, _num(g, "42", 1.0) or 1.0
    rot = math.radians(_num(g, "50"))
    co, si = math.cos(rot), math.sin(rot)
    px, py = _num(g, "10"), _num(g, "20")
    bx, by = base

    def porta(x: float, y: float) -> tuple[float, float]:
        lx, ly = (x - bx) * sx, (y - by) * sy
        return px + lx * co - ly * si, py + lx * si + ly * co

    return porta, abs(sx), abs(sy)


def ruotato(ins: _Entita) -> bool:
    """L'INSERT e' ruotato: la rotazione (50) e' lontana da un giro intero piu' di ROTAZIONE_TOLLERATA gradi."""
    r = _num(ins.g, "50") % 360.0
    return min(r, 360.0 - r) > ROTAZIONE_TOLLERATA


def risolvi_testi(entita: list, blocchi: dict, lim: dict) -> tuple[list[Testo], int, int, bool]:
    """I testi del disegno, nell'ordine del file: quelli delle ENTITIES e quelli dei blocchi richiamati con un
    INSERT (un livello) non ruotato. Restituisce anche gli INSERT annidati non seguiti, gli INSERT ruotati non
    seguiti (di un blocco con dei testi) e se ci si e' fermati a testi_max."""
    out: list[Testo] = []
    annidati = 0
    ruotati = 0
    tmax, cmax = int(lim["testi_max"]), int(lim["testo_max"])

    def aggiungi(e: _Entita, porta=None, scx: float = 1.0, scy: float = 1.0, spazio: int = 0) -> bool:
        if len(out) >= tmax:
            return False
        testo = e.scritto()[:cmax]
        if not testo:
            return True
        x0, cy, h, w = _ancora(e, testo)
        if porta is not None:
            x0, cy = porta(x0, cy)
            h, w = h * scy, w * scx
        out.append(Testo(x0, cy, h, w, testo, spazio))
        return True

    for e in entita:
        spazio = _intero(e.g, "67")
        if e.tipo in _TESTI:
            if not aggiungi(e, spazio=spazio):
                return out, annidati, ruotati, True
            continue
        b = blocchi.get(e.g.get("2", ""))
        if b is None:
            continue
        if ruotato(e):
            if b["testi"]:
                ruotati += 1
            continue
        annidati += b["annidati"]
        porta, scx, scy = _trasformazione(e, b["base"])
        for t in b["testi"]:
            if not aggiungi(t, porta, scx, scy, spazio):
                return out, annidati, ruotati, True
    return out, annidati, ruotati, False


def _norm(testo: str) -> list[str]:
    """I pezzi di un testo fra le barre, in maiuscolo e senza spazi ne' due punti finali."""
    return [p for p in (x.rstrip(":") for x in _RE_SPAZI.sub("", testo.upper()).split("/")) if p]


_VOCI = {campo: set(scritte) for campo, scritte in ETICHETTE_DXF}

# L'intestazione di una tabella, come nel testo dei PDF (worker_analisi: _COLONNA_POSIZIONE, _COLONNA_QUANTITA,
# _COLONNA_POSIZIONE_BREVE, intestazione_tabella). Gli insiemi sono gli stessi, copiati perche' worker_analisi importa
# questo modulo; un test controlla che non si separino.
_COLONNA_POSIZIONE = {"POS", "POS.", "POSIZIONE", "POSITION", "ITEM", "INDICE", "INDEX"}
_COLONNA_QUANTITA = {"Q.TY", "QTY", "QTY.", "Q.TA", "Q.TÀ", "Q.TA'", "QTA", "QTÀ", "QUANTITA", "QUANTITÀ",
                     "QUANTITA'", "QUANTITY"}
_COLONNA_POSIZIONE_BREVE = {"N.", "N°", "NR", "NR.", "RIF", "RIF."}


def etichetta_di(testo: str) -> str | None:
    """La voce di cui il testo intero e' l'etichetta, o None."""
    pezzi = _norm(testo)
    if not pezzi or len(pezzi) > 2:
        return None
    for campo, scritte in _VOCI.items():
        if all(p in scritte for p in pezzi):
            return campo
    return None


def _pezzi_colonna(testo: str) -> set[str]:
    """Le parole di un testo divise anche alla barra, in maiuscolo, senza i due punti finali, con il segno del numero
    in una forma sola: come worker_analisi._pezzi sulle parole di una riga del PDF."""
    out = set()
    for parola in testo.split():
        for p in parola.split("/"):
            p = p.upper().rstrip(":").replace("º", "°").replace("˚", "°")
            if p:
                out.add(p)
    return out


def riga_vista(et: Testo, testi: list[Testo]) -> list[Testo]:
    """I testi sulla riga vista di `et`: lo stesso spazio e il centro verticale entro mezza altezza (la maggiore
    delle due), come worker_analisi.righe_viste."""
    return [t for t in testi if t.spazio == et.spazio and abs(t.cy - et.cy) <= max(t.h, et.h) / 2]


def intestazione_tabella(riga: list[Testo]) -> bool:
    """La riga vista e' l'intestazione di una tabella: ha la colonna della posizione e quella della quantita'. Un nome
    breve della posizione («N.», «RIF.») conta solo come testo a se', che non e' un'etichetta del cartiglio: in
    «doc. no./Dok. Nr.» «NR.» e' un pezzo dell'etichetta."""
    pezzi: set[str] = set()
    for t in riga:
        pezzi |= _pezzi_colonna(t.testo)
    if not pezzi & _COLONNA_QUANTITA:
        return False
    if pezzi & _COLONNA_POSIZIONE:
        return True
    for t in riga:
        p = _pezzi_colonna(t.testo)
        if p and p <= _COLONNA_POSIZIONE_BREVE and etichetta_di(t.testo) is None:
            return True
    return False


def _valore_sotto(i: int, testi: list[Testo], etichette: dict[int, str]) -> Testo | None:
    """Il testo non-etichetta piu' vicino sotto l'etichetta testi[i], dentro la finestra della regola (docstring del
    modulo); a parita', il primo nel file. Senza passare nella cella sotto: un testo della finestra alto come
    l'etichetta (entro un quarto della sua altezza, riconosciuto o no) e' l'etichetta della cella dopo; se il testo
    scelto e' a quella distanza o oltre, anche perche' e' lui stesso alto cosi', il valore manca (None)."""
    et = testi[i]
    h = et.h
    if h <= 0:
        return None
    migliore, chiave, dy_migliore = None, None, 0.0
    fermo = math.inf  # la distanza dell'etichetta della cella sotto
    for j, t in enumerate(testi):
        if j == i or t.spazio != et.spazio:
            continue
        dy, dx = et.cy - t.cy, t.x0 - et.x0
        if not (0 < dy <= 5 * h and -h <= dx <= 2 * h):
            continue
        if abs(t.h - h) <= 0.25 * h:
            fermo = min(fermo, dy)
        if j in etichette:
            continue
        k = (dy + abs(dx), j)
        if chiave is None or k < chiave:
            migliore, chiave, dy_migliore = t, k, dy
    if migliore is None or dy_migliore >= fermo:
        return None
    return migliore


def campi_del_cartiglio(testi: list[Testo], campi_max: int) -> tuple[list[dict], bool]:
    """I campi riconosciuti, come i CampoCartiglio del testo dei PDF: etichetta (la voce), letta (com'e' scritta),
    valore (il testo grezzo sotto), riquadro del valore [x0, y0, x1, y1] in unita' del disegno con y verso
    l'alto (stimato). In ordine di posizione: dall'alto in basso, da sinistra a destra."""
    etichette = {i: c for i, t in enumerate(testi) if (c := etichetta_di(t.testo)) is not None}
    scelti: list[tuple[Testo, str, Testo]] = []
    for i, campo in etichette.items():
        et = testi[i]
        if intestazione_tabella(riga_vista(et, testi)):
            continue  # il nome di una colonna dell'elenco particolari, non un'etichetta del cartiglio
        if campo == "revisione":
            # la revisione della riga del numero del disegno: l'etichetta riconosciuta piu' vicina a sinistra
            riga = [(testi[j].x0, c) for j, c in etichette.items()
                    if c != "revisione" and testi[j].spazio == et.spazio and testi[j].x0 < et.x0
                    and abs(testi[j].cy - et.cy) <= et.h / 2]
            if not riga or max(riga)[1] != "numero_disegno":
                continue
        v = _valore_sotto(i, testi, etichette)
        if v is not None:
            scelti.append((et, campo, v))
    scelti.sort(key=lambda x: (x[0].spazio, -x[0].cy, x[0].x0, x[1]))
    out: list[dict] = []
    visti = set()
    for et, campo, v in scelti:
        riq = [round(v.x0, 2), round(v.cy - v.h / 2, 2), round(v.x0 + v.w, 2), round(v.cy + v.h / 2, 2)]
        chiave = (campo, v.testo, tuple(riq))
        if chiave in visti:
            continue
        visti.add(chiave)
        if len(out) >= campi_max:
            return out, True
        out.append({"etichetta": campo, "letta": et.testo, "valore": v.testo[:VALORE_MAX], "pagina": 1,
                    "zona": "pagina", "fonte": "nativo", "riquadro": riq, "confidenza": None})
    return out, False


def _fatto(lim: dict, esito: str, motivo: str = "", **altro) -> dict:
    f = {
        "versione": VERSIONE_TESTO_DXF,
        "esito": esito,
        "motivo": motivo,
        "versione_dxf": "",
        "codifica": "",
        "testi": 0,
        "annidati": 0,
        "troncato": False,
        "cartiglio": [],
        "limiti": {k: int(lim[k]) for k in ("byte_max", "testi_max", "testo_max", "campi_max")},
    }
    f.update(altro)
    return f


def leggi_testo_dxf_da_byte(dati: bytes, limiti: dict | None = None, troncato: bool = False) -> dict:
    """Il fatto `testo_dxf` di un contenuto DXF. Non alza mai: un DXF binario, un file che non e' un DXF ASCII o
    un errore di lettura sono un ESITO («non_letto», con il motivo), e il tipo lo dice comunque l'estensione."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    if dati.startswith(FIRMA_BINARIA):
        return _fatto(lim, DXF_NON_LETTO, "DXF binario: si leggono solo i DXF ASCII")
    if len(dati) > lim["byte_max"]:
        dati, troncato = dati[:int(lim["byte_max"])], True
    try:
        versione, codifica = codifica_del_dxf(dati[:65536])
        righe = io.TextIOWrapper(io.BytesIO(dati), encoding=codifica, errors="replace", newline=None)
        letti = _leggi_entita(iter(righe), FATTORE_LETTI * int(lim["testi_max"]))
        if not letti.entita and not letti.blocchi and not letti.dxf:
            return _fatto(lim, DXF_NON_LETTO, "nessuna entita' e nessuna SECTION: non sembra un DXF ASCII",
                          versione_dxf=versione, codifica=codifica, troncato=troncato)
        testi, annidati, ruotati, fermo = risolvi_testi(letti.entita, letti.blocchi, lim)
        campi, pieno = campi_del_cartiglio(testi, int(lim["campi_max"]))
    except Exception as e:  # noqa: BLE001 — una lettura fallita e' un esito, mai un job fallito
        log.warning("lettura DXF fallita: %s", e)
        return _fatto(lim, DXF_NON_LETTO, f"lettura fallita: {type(e).__name__}: {e}"[:300])
    motivo = ""
    if ruotati:
        motivo = (f"{ruotati} INSERT ruotati non seguiti: i testi dei loro blocchi non sono letti (la regola del "
                  "cartiglio vale per i blocchi diritti)")
    return _fatto(lim, DXF_LETTO, motivo, versione_dxf=versione, codifica=codifica, testi=len(testi),
                  annidati=annidati, troncato=troncato or letti.pieno or fermo or pieno, cartiglio=campi)


def leggi_testo_dxf(percorso: str, limiti: dict | None = None) -> dict:
    """Il fatto `testo_dxf` del file: al piu' byte_max byte (oltre, `troncato`). Non alza mai."""
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    try:
        with open(percorso, "rb") as f:
            dati = f.read(int(lim["byte_max"]) + 1)
    except OSError as e:
        return _fatto(lim, DXF_NON_LETTO, f"file non letto: {e}"[:300])
    return leggi_testo_dxf_da_byte(dati, lim)
