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
  - v2: gli INSERT dentro un blocco si seguono ricorsivamente, con la trasformazione affine composta di ogni livello
    (base del blocco, scala, rotazione, punto d'inserimento), fino a profondita_max livelli (8), con la guardia sui
    cicli (un blocco gia' sul cammino non si rientra) e i tetti dei testi e degli INSERT visitati. Quello che non si
    segue si conta (`annidati`, `oltre_profondita`, `cicli`) e il fatto e' `parziale`, con i motivi;
  - la rotazione propria dei testi non si considera (i cartigli sono orizzontali). Un INSERT ruotato si segue (v2): i
    suoi testi stanno nel gruppo della loro rotazione composta, e la regola del cartiglio lavora nel sistema di quel
    gruppo (le coordinate del disegno ruotate indietro), cosi' «sotto» resta «sotto» nel blocco. Un blocco
    specchiato (scala negativa) non si usa per il cartiglio: i suoi testi si contano in `specchiati`;
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

# v2 (GP-3, direttiva 21 §5.1): gli INSERT annidati si seguono ricorsivamente, con la trasformazione composta, la
# profondita' massima, la guardia sui cicli e i budget; gli INSERT ruotati si seguono (la regola del cartiglio vale nel
# sistema del blocco); il fatto dice i conti e `parziale`.
VERSIONE_TESTO_DXF = 2

# Fin dove si seguono gli INSERT annidati: il caso che ha portato alla v2 ne aveva 4.
PROFONDITA_MAX = 8

LIMITI_DEFAULT = {
    "byte_max": 64 * 1024 * 1024,  # byte del file letti al massimo
    "testi_max": 5000,             # testi risolti (dopo gli INSERT) al massimo
    "testo_max": 512,              # caratteri conservati di un testo
    "campi_max": 24,               # campi del cartiglio nel fatto
    "profondita_max": PROFONDITA_MAX,  # livelli di INSERT seguiti
    "inserti_max": 20000,          # INSERT visitati in tutto (un blocco ripetuto dentro un blocco ripetuto...)
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
    """Un'entita' letta: il tipo, i gruppi che servono (come stringhe, con i codici DXF) e il testo. `origine` e' da dove
    viene nel file, per chi lo sa dire (il lettore DWG: offset, sezione, blocco); None per il DXF."""
    __slots__ = ("tipo", "g", "testo", "origine")

    def __init__(self, tipo: str):
        self.tipo = tipo
        self.g: dict[str, str] = {}
        self.testo: list[str] = []  # i gruppi 3 e poi 1, nell'ordine del file
        self.origine: dict | None = None

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
                blocco = {"base": (_num(cur.g, "10"), _num(cur.g, "20")), "elementi": [], "annidati": 0}
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
                    # v2: anche gli INSERT di un blocco si tengono (si seguono, risolvi_testi), nell'ordine del file
                    if cur.tipo == "INSERT":
                        blocco["annidati"] += 1
                    if nei_blocchi < massimo:
                        blocco["elementi"].append(cur)
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
    """Un testo risolto: l'inizio stimato (x0), il centro verticale (cy), l'altezza, la larghezza stimata, il testo
    pulito e lo spazio.

    v2 (INSERT annidati): x0 e cy sono nel sistema del suo GRUPPO, cioe' le coordinate del disegno ruotate indietro
    della rotazione composta degli INSERT che lo portano (`rot`, in gradi): la regola del cartiglio («il valore sta
    sotto l'etichetta») vale cosi' nel sistema locale del blocco anche quando il blocco e' inserito ruotato. `spazio`
    e' il gruppo: (0 modello / 1 carta, rotazione in gradi interi, specchiato). Con rotazione 0 le coordinate sono
    quelle del disegno, come nella v1. `origine` e' da dove viene il testo nel file (DWG), None altrimenti."""
    __slots__ = ("x0", "cy", "h", "w", "testo", "spazio", "rot", "origine")

    def __init__(self, x0: float, cy: float, h: float, w: float, testo: str, spazio, rot: float = 0.0,
                 origine: dict | None = None):
        self.x0, self.cy, self.h, self.w, self.testo, self.spazio = x0, cy, h, w, testo, spazio
        self.rot, self.origine = rot, origine

    def riquadro(self) -> list[float]:
        """Il riquadro stimato [x0, y0, x1, y1] nelle coordinate del DISEGNO (y verso l'alto): con rotazione 0 e' quello
        della v1; ruotato, e' il rettangolo che contiene il testo riportato nel disegno."""
        x0, y0, x1, y1 = self.x0, self.cy - self.h / 2, self.x0 + self.w, self.cy + self.h / 2
        if not self.rot:
            return [round(x0, 2), round(y0, 2), round(x1, 2), round(y1, 2)]
        co, si = math.cos(math.radians(self.rot)), math.sin(math.radians(self.rot))
        punti = [(x * co - y * si, x * si + y * co) for x, y in ((x0, y0), (x1, y0), (x0, y1), (x1, y1))]
        return [round(min(p[0] for p in punti), 2), round(min(p[1] for p in punti), 2),
                round(max(p[0] for p in punti), 2), round(max(p[1] for p in punti), 2)]


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


IDENTITA = (1.0, 0.0, 0.0, 1.0, 0.0, 0.0)  # (a, b, c, d, tx, ty): X = a*x + b*y + tx, Y = c*x + d*y + ty


def trasformazione_insert(ins: _Entita, base: tuple[float, float]) -> tuple:
    """La trasformazione affine di un INSERT, da un punto del blocco a un punto del sistema che lo contiene: meno la
    base del blocco, la scala (41, 42), la rotazione (50, in gradi) e il punto d'inserimento (10, 20)."""
    g = ins.g
    sx, sy = _num(g, "41", 1.0) or 1.0, _num(g, "42", 1.0) or 1.0
    rot = math.radians(_num(g, "50"))
    co, si = math.cos(rot), math.sin(rot)
    px, py = _num(g, "10"), _num(g, "20")
    bx, by = base
    a, b, c, d = sx * co, -sy * si, sx * si, sy * co
    return (a, b, c, d, px - (a * bx + b * by), py - (c * bx + d * by))


def componi(m: tuple, t: tuple) -> tuple:
    """m dopo t: prima la trasformazione dell'INSERT interno (t), poi quella di chi lo contiene (m)."""
    ma, mb, mc, md, mx, my = m
    ta, tb, tc, td, tx, ty = t
    return (ma * ta + mb * tc, ma * tb + mb * td, mc * ta + md * tc, mc * tb + md * td,
            ma * tx + mb * ty + mx, mc * tx + md * ty + my)


def ruotato(ins: _Entita) -> bool:
    """L'INSERT e' ruotato: la rotazione (50) e' lontana da un giro intero piu' di ROTAZIONE_TOLLERATA gradi."""
    r = _num(ins.g, "50") % 360.0
    return min(r, 360.0 - r) > ROTAZIONE_TOLLERATA


class Risolti:
    """Che cosa ha dato la risoluzione dei testi: i testi e i conti degli INSERT (v2)."""
    __slots__ = ("testi", "annidati", "annidati_seguiti", "oltre_profondita", "cicli", "specchiati", "fermo",
                 "motivi")

    def __init__(self):
        self.testi: list[Testo] = []
        self.annidati = 0           # INSERT dentro un blocco NON seguiti (profondita', ciclo, tetto)
        self.annidati_seguiti = 0   # INSERT dentro un blocco seguiti
        self.oltre_profondita = 0   # INSERT non seguiti perche' oltre profondita_max
        self.cicli = 0              # INSERT non seguiti perche' il blocco e' gia' sul cammino
        self.specchiati = 0         # testi in un sistema specchiato: la regola del cartiglio non li usa
        self.fermo = False          # tetto dei testi o degli INSERT raggiunto
        self.motivi: list[str] = []


def risolvi_testi(entita: list, blocchi: dict, lim: dict) -> Risolti:
    """I testi del disegno, nell'ordine del file: quelli delle ENTITIES e quelli dei blocchi richiamati con un INSERT,
    seguiti RICORSIVAMENTE (v2) con la trasformazione affine composta di ogni livello (punto d'inserimento, scala,
    rotazione), fino a profondita_max livelli, con la guardia sui cicli (un blocco gia' sul cammino non si rientra) e i
    budget dei testi e degli INSERT visitati. Un INSERT ruotato si segue: i suoi testi vanno nel gruppo della loro
    rotazione composta (Testo). MINSERT: la prima istanza."""
    r = Risolti()
    tmax, cmax = int(lim["testi_max"]), int(lim["testo_max"])
    pmax = int(lim.get("profondita_max", PROFONDITA_MAX))
    imax = int(lim.get("inserti_max", FATTORE_LETTI * tmax))
    visitati = [0]

    def aggiungi(e: _Entita, m: tuple, spazio: int) -> bool:
        if len(r.testi) >= tmax:
            r.fermo = True
            return False
        testo = e.scritto()[:cmax]
        if not testo:
            return True
        x0, cy, h, w = _ancora(e, testo)
        a, b, c, d, tx, ty = m
        X, Y = a * x0 + b * cy + tx, c * x0 + d * cy + ty
        rot = math.degrees(math.atan2(c, a))
        gradi = int(round(rot)) % 360
        specchiato = a * d - b * c < 0
        if gradi:
            co, si = math.cos(math.radians(gradi)), math.sin(math.radians(gradi))
            X, Y = X * co + Y * si, -X * si + Y * co
        r.testi.append(Testo(X, Y, h * math.hypot(b, d), w * math.hypot(a, c), testo, (spazio, gradi, specchiato),
                             float(gradi), e.origine))
        if specchiato:
            r.specchiati += 1
        return True

    def visita(elementi: list, m: tuple, spazio_padre: int | None, profondita: int, cammino: frozenset) -> bool:
        for e in elementi:
            spazio = _intero(e.g, "67") if spazio_padre is None else spazio_padre
            if e.tipo in _TESTI:
                if not aggiungi(e, m, spazio):
                    return False
                continue
            if e.tipo != "INSERT":
                continue
            nome = e.g.get("2", "")
            b = blocchi.get(nome)
            if b is None:
                continue
            if nome in cammino:
                r.cicli += 1
                r.annidati += profondita > 0
                continue
            if profondita >= pmax:
                r.oltre_profondita += 1
                r.annidati += 1
                continue
            if visitati[0] >= imax:
                r.fermo = True
                r.annidati += profondita > 0
                return False
            visitati[0] += 1
            if profondita > 0:
                r.annidati_seguiti += 1
            if not visita(b["elementi"], componi(m, trasformazione_insert(e, b["base"])), spazio, profondita + 1,
                          cammino | {nome}):
                return False
        return True

    visita(entita, IDENTITA, None, 0, frozenset())
    if r.oltre_profondita:
        r.motivi.append(f"{r.oltre_profondita} INSERT oltre {pmax} livelli non seguiti")
    if r.cicli:
        r.motivi.append(f"{r.cicli} INSERT che rientrano in un blocco gia' sul cammino (ciclo) non seguiti")
    if r.specchiati:
        r.motivi.append(f"{r.specchiati} testi in blocchi specchiati: la regola del cartiglio non li usa")
    return r


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


def etichetta_di(testo: str, voci: dict[str, set[str]] | None = None) -> str | None:
    """La voce di cui il testo intero e' l'etichetta, o None. `voci` e' il vocabolario (voce -> scritte senza spazi in
    maiuscolo): quello del DXF se manca (il lettore DWG ne passa uno suo)."""
    pezzi = _norm(testo)
    if not pezzi or len(pezzi) > 2:
        return None
    for campo, scritte in (voci or _VOCI).items():
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


def campi_del_cartiglio(testi: list[Testo], campi_max: int, voci: dict[str, set[str]] | None = None,
                        origini: bool = False) -> tuple[list[dict], bool]:
    """I campi riconosciuti, come i CampoCartiglio del testo dei PDF: etichetta (la voce), letta (com'e' scritta),
    valore (il testo grezzo sotto), riquadro del valore [x0, y0, x1, y1] in unita' del disegno con y verso
    l'alto (stimato). In ordine di posizione: dall'alto in basso, da sinistra a destra, gruppo per gruppo.

    v2: la regola vale dentro ogni gruppo (stesso spazio, stessa rotazione composta), nel sistema del gruppo; i testi
    dei gruppi specchiati non si usano. `voci` e' il vocabolario delle etichette (quello del DXF se manca); con
    `origini` ogni campo porta anche `_origine_valore` e `_origine_etichetta` (Testo.origine), per il lettore DWG."""
    usabili = [t for t in testi if not (isinstance(t.spazio, tuple) and t.spazio[2])]
    etichette = {i: c for i, t in enumerate(usabili) if (c := etichetta_di(t.testo, voci)) is not None}
    scelti: list[tuple[Testo, str, Testo]] = []
    for i, campo in etichette.items():
        et = usabili[i]
        if intestazione_tabella(riga_vista(et, usabili)):
            continue  # il nome di una colonna dell'elenco particolari, non un'etichetta del cartiglio
        if campo == "revisione":
            # la revisione della riga del numero del disegno: l'etichetta riconosciuta piu' vicina a sinistra
            riga = [(usabili[j].x0, c) for j, c in etichette.items()
                    if c != "revisione" and usabili[j].spazio == et.spazio and usabili[j].x0 < et.x0
                    and abs(usabili[j].cy - et.cy) <= et.h / 2]
            if not riga or max(riga)[1] != "numero_disegno":
                continue
        v = _valore_sotto(i, usabili, etichette)
        if v is not None:
            scelti.append((et, campo, v))
    scelti.sort(key=lambda x: (x[0].spazio, -x[0].cy, x[0].x0, x[1]))
    out: list[dict] = []
    visti = set()
    for et, campo, v in scelti:
        riq = v.riquadro()
        chiave = (campo, v.testo, tuple(riq))
        if chiave in visti:
            continue
        visti.add(chiave)
        if len(out) >= campi_max:
            return out, True
        campo_d = {"etichetta": campo, "letta": et.testo, "valore": v.testo[:VALORE_MAX], "pagina": 1,
                   "zona": "pagina", "fonte": "nativo", "riquadro": riq, "confidenza": None}
        if origini:
            campo_d["_origine_valore"], campo_d["_origine_etichetta"] = v.origine, et.origine
        out.append(campo_d)
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
        "annidati_seguiti": 0,
        "oltre_profondita": 0,
        "cicli": 0,
        "specchiati": 0,
        "troncato": False,
        "parziale": False,
        "cartiglio": [],
        "limiti": {k: int(lim[k]) for k in ("byte_max", "testi_max", "testo_max", "campi_max", "profondita_max",
                                            "inserti_max")},
    }
    f.update(altro)
    return f


def _limiti(limiti: dict | None) -> dict:
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    lim.setdefault("profondita_max", PROFONDITA_MAX)
    lim.setdefault("inserti_max", FATTORE_LETTI * int(lim["testi_max"]))
    return lim


def leggi_testo_dxf_da_byte(dati: bytes, limiti: dict | None = None, troncato: bool = False) -> dict:
    """Il fatto `testo_dxf` di un contenuto DXF. Non alza mai: un DXF binario, un file che non e' un DXF ASCII o
    un errore di lettura sono un ESITO («non_letto», con il motivo), e il tipo lo dice comunque l'estensione.

    v2: `parziale` e' vero quando qualcosa non e' stato letto (un tetto: `troncato`; INSERT oltre profondita_max o
    in un ciclo; testi in blocchi specchiati), con i motivi in `motivo`. Un cartiglio vuoto di una lettura parziale non
    e' un DXF «senza cartiglio»."""
    lim = _limiti(limiti)
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
        r = risolvi_testi(letti.entita, letti.blocchi, lim)
        campi, pieno = campi_del_cartiglio(r.testi, int(lim["campi_max"]))
    except Exception as e:  # noqa: BLE001 — una lettura fallita e' un esito, mai un job fallito
        log.warning("lettura DXF fallita: %s", e)
        return _fatto(lim, DXF_NON_LETTO, f"lettura fallita: {type(e).__name__}: {e}"[:300])
    tronc = troncato or letti.pieno or r.fermo or pieno
    motivi = list(r.motivi)
    if tronc:
        motivi.insert(0, "fermato su un tetto (byte, testi, INSERT o campi)")
    return _fatto(lim, DXF_LETTO, "; ".join(motivi)[:300], versione_dxf=versione, codifica=codifica,
                  testi=len(r.testi), annidati=r.annidati, annidati_seguiti=r.annidati_seguiti,
                  oltre_profondita=r.oltre_profondita, cicli=r.cicli, specchiati=r.specchiati, troncato=tronc,
                  parziale=bool(tronc or r.oltre_profondita or r.cicli or r.specchiati), cartiglio=campi)


def leggi_testo_dxf(percorso: str, limiti: dict | None = None) -> dict:
    """Il fatto `testo_dxf` del file: al piu' byte_max byte (oltre, `troncato`). Non alza mai."""
    lim = _limiti(limiti)
    try:
        with open(percorso, "rb") as f:
            dati = f.read(int(lim["byte_max"]) + 1)
    except OSError as e:
        return _fatto(lim, DXF_NON_LETTO, f"file non letto: {e}"[:300])
    return leggi_testo_dxf_da_byte(dati, lim)
