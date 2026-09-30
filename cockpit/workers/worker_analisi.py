"""worker-analisi: client HTTP di cockpit.exe che esegue i job di tipo 'analizza_allegato'.

Distingue i disegni CAD dalle offerte commerciali analizzando il contenuto testuale (PyMuPDF)
e la struttura dei file (STEP), senza mai basarsi su nomi di clienti per evitare falsi positivi.

    python worker_analisi.py [--config worker.toml] [--una-volta]
"""
from __future__ import annotations

import argparse
import gc
import logging
import math
import os
import re
import shutil
import socket
import time
from pathlib import Path
from typing import NamedTuple
from uuid import UUID

import pymupdf

from cockpit_client import (ERRORI_RETE, ArrestoRichiesto, Battito, Cockpit, ContenutoIncompleto, ErroreHTTP,
                            ImprontaSbagliata, cadenza_battito, carica_config, configura_log, diagnosi,
                            leggi_marcatore_arresto, nome_worker, riporta_risultato)
from contratti import (OCRPDF, CampoCartiglio, FrammentoPDF, Job, LimitiTestoPDF, MetadatiPDF,
                       PayloadAnalizzaAllegato, RisultatoAnalisi, RisultatoRichiesta, TestoPDF)
from step_struttura import leggi_struttura

log = logging.getLogger("worker-analisi")

# Parole chiave cartiglio tecnico CAD 2D (da specifiche utente)
TERMINI_CARTIGLIO_CAD = [
    "ZONA ESENTE DA SALDATURA",
    "TOLLERANZE GENERALI",
    "PESO KG",
    "SCALA",
]

# Parole chiave offerte commerciali (da specifiche utente).
# I termini "forti" bastano da soli; "PAGAMENTO"/"INCOTERMS" compaiono anche in contratti e convenzioni
# (es. un PDF di tirocinio) e contano solo se ne ricorrono almeno due.
TERMINI_OFFERTE_FORTI = [
    "SPETT. LE OFFERTA",
    "SPETT.LE OFFERTA",
    "CONDIZIONI GENERALI DI VENDITA",
]
TERMINI_OFFERTE_DEBOLI = [
    "PAGAMENTO",
    "INCOTERMS",
    "OFFERTA N",
    "VALIDITA' OFFERTA",
    "VALIDITÀ OFFERTA",
    "RESA",
]
TERMINI_OFFERTE_COMMERCIALI = TERMINI_OFFERTE_FORTI + TERMINI_OFFERTE_DEBOLI

# Capitolato / specifica tecnica: parla di requisiti e di come si collauda, non di un pezzo solo.
# Ne servono almeno due, perche' «SPECIFICA TECNICA» da sola compare anche nel cartiglio di un disegno.
TERMINI_CAPITOLATO = [
    "CAPITOLATO",
    "SPECIFICA TECNICA",
    "REQUISITI GENERALI",
    "PRESCRIZIONI",
    "NORME DI RIFERIMENTO",
    "PIANO DI CONTROLLO",
    "CONDIZIONI DI FORNITURA",
    "TECHNICAL SPECIFICATION",
    "GENERAL REQUIREMENTS",
    "QUALITY REQUIREMENTS",
]

# Distinta del cliente: l'intestazione di una tabella di codici con quantita'.
TERMINI_DISTINTA = [
    "DISTINTA BASE",
    "DISTINTA MATERIALI",
    "BILL OF MATERIAL",
    "PARTS LIST",
    "ELENCO PARTI",
    "STUCKLISTE",
    "POS.",
    "Q.TA",
    "QUANTITA",
]

# Una riga di distinta: qualcosa che sembra un codice, e poi un numero che sembra una quantita'.
RE_RIGA_DISTINTA = re.compile(r"\b[A-Z0-9][A-Z0-9._\-/]{4,}\b[^\n]{0,40}?\b\d{1,4}(?:[.,]\d{1,3})?\b")

STOP_CODICI = {
    "SCREENSHOT", "WHATSAPP", "IMAGE", "PHOTO", "IMG", "DOCUMENT", "OFFERTA",
    "DISEGNO", "ALLEGATO", "REV", "STEP", "STP", "PDF", "DXF", "DWG", "ZIP",
}

RE_REV = re.compile(r"^(.+?)[_\-](?:REV|R)?([0-9]{1,2}|[A-Z])$", re.IGNORECASE)
RE_STEP_PRODUCT = re.compile(r"PRODUCT\s*\(\s*'([^']+)'", re.IGNORECASE)


def sembra_codice(s: str) -> bool:
    """Verifica se una stringa rispetta i pattern codice prodotto (almeno 5 char, almeno 3 cifre)."""
    s = s.strip().upper()
    if len(s) < 5 or len(s) > 40:
        return False
    if s in STOP_CODICI or any(s.startswith(p) for p in ("SCREENSHOT", "IMG", "WHATSAPP", "SCAN", "PHOTO", "SO ", "SO-", "SO_")):
        return False
    if " " in s:
        return False
    cifre = sum(1 for c in s if c.isdigit())
    lettere = sum(1 for c in s if c.isalpha())
    if cifre < 3:
        return False
    # Evita date tipo 2026-09-08
    if lettere == 0 and (s.count("-") >= 2 or s.count("/") >= 2 or s.count(".") >= 2):
        return False
    return True


def separa_codice_rev(nome_base: str) -> tuple[str, str]:
    """Separa codice e revisione dal nome file senza estensione."""
    m = RE_REV.match(nome_base)
    if m:
        base, rev = m.group(1), m.group(2)
        if sembra_codice(base):
            return base, rev
    if sembra_codice(nome_base):
        return nome_base, ""
    return "", ""


# ---------------------------------------------------------------- il testo dei PDF, lettura strutturata (Smistamento F9)
#
# Fino all'analizzatore 3 il testo di un PDF restava qui: al server tornava solo la sua lunghezza
# (`testo_letto`), e il codice veniva sempre dal nome del file, anche quando la fonte diceva «cartiglio».
# Dalla 4 il worker riporta i FATTI del testo in `dettagli.testo_pdf` (addendum A5.13.8, decisioni del 27/09
# «ter»): frammenti con pagina e riquadro, i campi del probabile cartiglio come etichetta -> valore, i
# metadati, che cosa ha fatto l'OCR e i limiti. Il testo integrale di un documento grande NON viaggia: solo i
# frammenti utili, dentro limiti fissi scritti nel fatto. I codici non li cerca il worker: li cerca il server
# con le famiglie del cliente di ciascuna RFQ, perche' il worker non sa per quale RFQ analizza e lo stesso
# contenuto puo' stare in RFQ di clienti diversi (A5.13.7). Per la stessa ragione i campi del cartiglio sono
# testo grezzo: «DISEGNO N.» -> «7120010», non «il codice e' 7120010».
#
# Le coordinate sono quelle della pagina COME SI VEDE (rotazione applicata), in punti, con l'origine in alto a
# sinistra. `find_tables` non si usa: sui disegni vettoriali (migliaia di segmenti) costa piu' di tutta la
# lettura, e i campi del cartiglio si ritrovano gia' con le parole e le loro coordinate.
#
# VERSIONE_TESTO_PDF e' la sottoversione della FORMA del testo, non la versione dell'analizzatore (quella la decide
# il server, e il worker la ripete). La 2 (giro 4, fase 4.6) legge il cartiglio meglio della 1: l'etichetta con la
# gemella inglese dopo la barra si legge intera («DENOMINAZIONE / NAME»: il valore non e' piu' «/ NAME»), «PART N°»
# e «PART NR» sono etichette del codice, «NAME» e' il titolo solo come gemella, c'e' il campo del particolare simile
# (e il segnaposto del modello non da' niente), l'intestazione di una tabella («Pos. Part Number Q.ty», «N. CODICE
# DESCRIZIONE QTA») non e' un campo. Con la
# sottoversione nuova il server riconosce i PDF letti prima, e «Rianalizza» li rilegge. La fase 4.12 (l'elenco
# particolari) usera' la stessa: un solo pacchetto della postazione, una sola rianalisi.
VERSIONE_TESTO_PDF = 2
TESTO_PDF_PAGINE_MAX = 11        # le pagine lette: le stesse che si scorrevano gia' per i termini
TESTO_PDF_FRAMMENTI_MAX = 200    # frammenti nel fatto
TESTO_PDF_FRAMMENTO_MAX = 512    # caratteri di un frammento
TESTO_PDF_CARATTERI_MAX = 16384  # caratteri di tutti i frammenti insieme
TESTO_PDF_CAMPI_MAX = 24         # campi del cartiglio
TESTO_PDF_VALORE_MAX = 120       # caratteri del valore di un campo

# La zona in basso a destra della pagina 1, dove un cartiglio sta quasi sempre: frazioni della larghezza e
# dell'altezza della pagina vista. E' un fatto geometrico: che li' ci sia «probabilmente il cartiglio» lo dice
# il server, non il worker.
BASSO_DESTRA_X = 0.5
BASSO_DESTRA_Y = 0.6
ZONA_BASSO_DESTRA = "basso_destra"
ZONA_PAGINA = "pagina"

# L'OCR SELETTIVO (decisioni del 27/09 «ter», Domanda 4 = B): solo dove il testo nativo manca o non basta, e
# solo li'.
#   - Una pagina letta con meno di OCR_SOGLIA_PAGINA caratteri di testo nativo e' «senza testo» (curve, una
#     scansione) e si legge intera con l'OCR: al piu' OCR_PAGINE_MAX pagine, le prime.
#   - La pagina 1 con il suo testo, ma con meno di OCR_SOGLIA_CARTIGLIO caratteri nativi nella zona in basso a
#     destra, fa leggere con l'OCR SOLO quella zona (un cartiglio incollato come immagine).
#   - Altrimenti niente OCR.
# L'OCR e' un'evidenza, non un automatismo: i suoi frammenti hanno `fonte: ocr` e non cambiano ne' il tipo (i
# termini si cercano nel solo testo nativo) ne' `estraibile`. E non fa MAI fallire l'analisi: un motore assente,
# un'eccezione, il tempo finito o un risultato illeggibile si scrivono in `ocr` (stato e motivo) e l'analisi
# prosegue con il testo nativo. Il tempo e' un budget per file: un passaggio gia' partito non si interrompe
# (Tesseract non lo consente), quelli dopo si saltano come `scaduto`.
OCR_SOGLIA_PAGINA = 20
OCR_SOGLIA_CARTIGLIO = 8
OCR_PAGINE_MAX = 2
OCR_TEMPO_MAX_S = 60
OCR_DPI = 300
OCR_LINGUA = "ita+eng"

RE_SPAZI = re.compile(r"[ \t ]+")
RE_CIFRA = re.compile(r"\d")


def normalizza_testo(testo: str) -> str:
    """Il testo del file con meno aria: spazi ripetuti in uno, righe vuote tolte. Le parole restano quelle."""
    righe = (RE_SPAZI.sub(" ", r).strip() for r in testo.splitlines())
    return "\n".join(r for r in righe if r)


class Parola(NamedTuple):
    """Una parola sulla pagina VISTA: il riquadro, il testo, blocco, riga e posizione nella riga come li da'
    PyMuPDF (servono a ricomporre i frammenti), e la confidenza 0-100 del motore OCR quando la da' (None per il
    testo nativo)."""
    x0: float
    y0: float
    x1: float
    y1: float
    testo: str
    blocco: int = 0
    riga: int = 0
    n: int = 0
    confidenza: float | None = None


def parole_native(pag) -> list[Parola]:
    """Le parole del testo nativo della pagina, portate sulla pagina vista.

    PyMuPDF da' le coordinate delle parole sulla pagina NON ruotata, mentre `pag.rect` e' la pagina come si
    vede: un disegno orizzontale salvato in verticale con /Rotate 90 ha il cartiglio in basso a destra sullo
    schermo e altrove nelle coordinate del file. Ogni parola si porta sulla pagina vista con `rotation_matrix`
    prima di confrontarla con larghezza e altezza."""
    ruota = pag.rotation_matrix
    out = []
    for x0, y0, x1, y1, parola, blocco, riga, n in pag.get_text("words"):
        r = (pymupdf.Rect(x0, y0, x1, y1) * ruota).normalize()
        out.append(Parola(r.x0, r.y0, r.x1, r.y1, parola, blocco, riga, n))
    return out


def zona_basso_destra(vista) -> "pymupdf.Rect":
    return pymupdf.Rect(vista.x0 + BASSO_DESTRA_X * vista.width, vista.y0 + BASSO_DESTRA_Y * vista.height,
                        vista.x1, vista.y1)


def in_zona(p: Parola, zona) -> bool:
    return p.x0 >= zona.x0 and p.y0 >= zona.y0


def riquadro(parole: list[Parola]) -> list[float]:
    return [round(min(p.x0 for p in parole), 1), round(min(p.y0 for p in parole), 1),
            round(max(p.x1 for p in parole), 1), round(max(p.y1 for p in parole), 1)]


def confidenza_media(parole: list[Parola]) -> float | None:
    c = [p.confidenza for p in parole]
    if not c or any(x is None for x in c):
        return None
    return round(sum(c) / len(c), 1)


def componi_frammenti(parole: list[Parola], pagina: int, fonte: str, zona_di) -> list[dict]:
    """I frammenti di una pagina: un frammento per blocco del file (o dell'OCR) e per zona, con le righe del
    blocco nell'ordine del file e il riquadro delle sue parole. Un blocco a cavallo del bordo della zona in
    basso a destra diventa due frammenti, cosi' nessuna parola del cartiglio finisce fuori dalla zona."""
    gruppi: dict[tuple[str, int], list[Parola]] = {}
    for p in parole:
        gruppi.setdefault((zona_di(p), p.blocco), []).append(p)
    out = []
    for (zona, _), ps in gruppi.items():  # l'ordine di inserimento e' l'ordine del file
        righe: dict[int, list[Parola]] = {}
        for p in ps:
            righe.setdefault(p.riga, []).append(p)
        testo = normalizza_testo("\n".join(" ".join(q.testo for q in sorted(r, key=lambda q: q.n))
                                           for _, r in sorted(righe.items())))
        if testo:
            out.append({"pagina": pagina, "zona": zona, "fonte": fonte, "testo": testo,
                        "riquadro": riquadro(ps), "confidenza": confidenza_media(ps)})
    return out


# ---- i campi del cartiglio
#
# Le etichette tipiche di un cartiglio, in italiano e in inglese: per ciascuna voce le scritte che la dicono.
# Si confrontano parola per parola, in maiuscolo e senza i due punti finali; a parita' di inizio vince la piu'
# lunga («DISEGNO N.» prima di «DISEGNO»). Il valore e' il testo che segue l'etichetta sulla stessa riga vista,
# fino alla prossima etichetta o a un salto largo (un'altra cella); se sulla riga non c'e' niente, e' la riga
# subito sotto, nella colonna dell'etichetta. Testo grezzo: nessuna regola di codice.
#
# Sottoversione 2 del testo (giro 4, fase 4.6):
#   - un'etichetta seguita dalla barra e dalla sua gemella inglese della stessa voce («DENOMINAZIONE / NAME»,
#     «PARTICOLARE SIMILE / SIMILAR PART», anche attaccate: «DENOMINAZIONE/NAME») e' UNA etichetta: prima la
#     gemella che non si conosceva finiva nel valore («/ NAME»), e quella che si conosceva faceva un campo suo;
#   - «PART N°», «PART NR» e le loro varianti sono etichette del codice (i cartigli di certi clienti non ne hanno
#     altre); «NAME» e' il titolo, ma solo come gemella (SOLO_GEMELLE);
#   - il particolare simile («PARTICOLARE SIMILE», «PART. SIMILE», «SIMILAR PART») e' un campo suo, con il valore
#     grezzo: che cosa sia lo dice il server (una nota, «simile a X»). Il segnaposto del modello nel campo vuoto
#     («Inserire codice particolare simile») non e' un valore, e le sue parole non sono etichette (segnaposto_a);
#   - l'intestazione di una tabella non e' una riga di etichette (intestazione_tabella).
ETICHETTE_CARTIGLIO = (
    ("numero_disegno", ("NUMERO DISEGNO", "N. DISEGNO", "N.DISEGNO", "N° DISEGNO", "NR. DISEGNO", "DISEGNO N.",
                        "DISEGNO N°", "DISEGNO NR.", "DIS. N.", "DRAWING NUMBER", "DRAWING NO.", "DRAWING NO",
                        "DRAWING N.", "DWG. NO.", "DWG NO.", "DWG NO")),
    ("codice", ("PART NUMBER", "PART NO.", "PART NO", "PART. NO.", "PART. NO", "PART.NO.", "PART N°", "PART. N°",
                "PART.N°", "PART N.", "PART. N.", "PART NR", "PART NR.", "PART. NR", "PART. NR.", "PART.NR", "PART.NR.",
                "P/N", "CODICE", "COD.", "CODE", "ARTICOLO")),
    ("revisione", ("REVISIONE", "REVISION", "REV.", "REV", "EDIZIONE")),
    ("titolo", ("DENOMINAZIONE", "DESCRIZIONE", "TITOLO", "DESCRIPTION", "TITLE")),
    ("scala", ("SCALA", "SCALE")),
    ("materiale", ("MATERIALE", "MATERIAL")),
    ("particolare_simile", ("PARTICOLARE SIMILE", "PART. SIMILE", "PART SIMILE", "PART.SIMILE", "SIMILAR PART")),
)
# Le scritte che sono un'etichetta soltanto come gemelle, dopo la barra, di un'etichetta della stessa voce (fase
# 4.6r): «NAME» e' il titolo in «DENOMINAZIONE / NAME», ma da sola, accanto a «DRAWN», «CHECKED», «DATE», e' il nome
# di chi ha disegnato o controllato il disegno, e faceva un titolo con il nome di una persona.
SOLO_GEMELLE = (
    ("titolo", ("NAME",)),
)
# i segni fra un'etichetta e il suo valore; la barra dalla sottoversione 2, perche' una barra da sola non e' un valore
# («CODICE / DESCRIPTION»: la gemella di un'altra voce non si unisce, e il codice non vale «/»)
_SEPARATORI_VALORE = {":", "-", "=", "#", "–", "/"}


def _norm_etichetta(parola: str) -> str:
    """Una parola (o un pezzo di parola) come si confronta con le etichette: in maiuscolo, senza i due punti
    finali, con il segno del numero in una forma sola («Nº», «N˚» -> «N°»)."""
    return parola.upper().rstrip(":").replace("º", "°").replace("˚", "°")


def _pezzi(testo: str) -> list[str]:
    """Una parola divisa alla barra, con la barra come pezzo a se' («DENOMINAZIONE/NAME» -> DENOMINAZIONE, /, NAME;
    «P/N» -> P, /, N), normalizzata. Le etichette si confrontano pezzo per pezzo: cosi' la gemella attaccata alla
    barra e quella staccata sono la stessa cosa."""
    out: list[str] = []
    for k, p in enumerate(testo.split("/")):
        if k:
            out.append("/")
        p = _norm_etichetta(p)
        if p:
            out.append(p)
    return out


def _in_pezzi(voci) -> list[tuple[tuple[str, ...], str]]:
    """Le etichette in pezzi, le piu' lunghe prima."""
    return sorted(((tuple(x for w in e.split() for x in _pezzi(w)), campo) for campo, scritte in voci for e in scritte),
                  key=lambda x: -len(x[0]))


# le etichette in pezzi; dopo la barra valgono anche le gemelle sole
_ETICHETTE = _in_pezzi(ETICHETTE_CARTIGLIO)
_GEMELLE = _in_pezzi(ETICHETTE_CARTIGLIO + SOLO_GEMELLE)


# quante parole si guardano da un inizio per trovarci un'etichetta con le sue gemelle: le etichette piu' lunghe ne
# hanno due, con le gemelle cinque o sei; cosi' una riga lunga (una nota) non costa il quadrato delle sue parole
_PAROLE_ETICHETTA_MAX = 12


def _flusso(parole: list[Parola], i: int) -> list[tuple[str, int, bool]]:
    """I pezzi delle parole da parole[i] in poi (al piu' _PAROLE_ETICHETTA_MAX), ciascuno con l'indice della sua
    parola e se chiude la parola."""
    out = []
    for w in range(i, min(len(parole), i + _PAROLE_ETICHETTA_MAX)):
        pz = _pezzi(parole[w].testo)
        for k, p in enumerate(pz):
            out.append((p, w, k == len(pz) - 1))
    return out


def _combacia(flusso: list[tuple[str, int, bool]], da: int, campo: str | None = None) -> tuple[str, int] | None:
    """(voce, quanti pezzi) dell'etichetta piu' lunga che comincia al pezzo `da` del flusso; con `campo`, la gemella
    di quella voce (anche fra SOLO_GEMELLE). L'etichetta finisce alla fine di una parola o prima di una barra:
    «CODICE» non e' l'inizio di «CODICEX»."""
    for toks, c in (_ETICHETTE if campo is None else _GEMELLE):
        n = len(toks)
        if campo is not None and c != campo:
            continue
        if da + n > len(flusso) or any(flusso[da + k][0] != toks[k] for k in range(n)):
            continue
        if flusso[da + n - 1][2] or (da + n < len(flusso) and flusso[da + n][0] == "/"):
            return c, n
    return None


def etichetta_a(parole: list[Parola], i: int) -> tuple[str, int] | None:
    """(voce, quante parole) se da parole[i] comincia un'etichetta del cartiglio. Una gemella della stessa voce dopo
    la barra fa parte dell'etichetta («DENOMINAZIONE / NAME», «DENOMINAZIONE/NAME»: una etichetta, il valore dopo);
    un'etichetta attaccata alla barra con dopo una parola che non e' un'etichetta si prende con tutta la parola
    («DENOMINAZIONE/BEZEICHNUNG»)."""
    flusso = _flusso(parole, i)
    trovata = _combacia(flusso, 0)
    if trovata is None:
        return None
    campo, k = trovata
    while k < len(flusso) and flusso[k][0] == "/":
        j = k
        while j < len(flusso) and flusso[j][0] == "/":
            j += 1
        gemella = _combacia(flusso, j, campo) if j < len(flusso) else None
        if gemella is None:
            break
        k = j + gemella[1]
    # l'etichetta che finisce dentro una parola, prima di una barra senza gemella («DENOMINAZIONE/BEZEICHNUNG»):
    # la parola intera e' l'etichetta
    while not flusso[k - 1][2]:
        k += 1
    return campo, flusso[k - 1][1] - i + 1


# Il segnaposto del modello nel campo del particolare simile, quando il campo e' vuoto: «Inserire codice particolare
# simile» (anche «Inserire il codice del part. simile», «Insert the code of the similar part»). Non e' un valore, e
# le parole che contiene non sono etichette: prese per tali, «codice» faceva un campo del codice con la parola dopo,
# e nel cartiglio a tabella dei disegni veri (la cella del segnaposto, accanto quella di «PART. N°») la parola dopo
# «particolare simile» e' il codice del file stesso. Fra «Inserire»/«Insert» e l'etichetta ci sono solo le parole
# della frase del modello (le stesse della regola del server, classificazione.paroleSegnaposto), almeno una.
_SEGNAPOSTO_INIZIO = {"INSERIRE", "INSERT"}
_PAROLE_SEGNAPOSTO = {"QUI", "HERE", "IL", "LO", "LA", "THE", "DI", "DEL", "DELLO", "DELLA", "OF", "CODICE", "COD",
                      "COD.", "CODE", "NUMERO", "NUMBER", "NUM", "NUM.", "NR", "NR.", "NO", "NO.", "N", "N.", "N°"}


def segnaposto_a(parole: list[Parola], i: int) -> int:
    """Quante parole fa il segnaposto del particolare simile che comincia in parole[i]; 0 se li' non comincia."""
    if _norm_etichetta(parole[i].testo) not in _SEGNAPOSTO_INIZIO:
        return 0
    j = i + 1
    while j < len(parole) and j - i <= 8 and _norm_etichetta(parole[j].testo) in _PAROLE_SEGNAPOSTO:
        j += 1
    if j == i + 1 or j >= len(parole):
        return 0
    e = etichetta_a(parole, j)
    if e is None or e[0] != "particolare_simile":
        return 0
    return j + e[1] - i


# L'intestazione di una tabella dentro la pagina (l'elenco particolari di un disegno d'assieme, sopra il cartiglio
# e spesso dentro la zona in basso a destra): una riga vista con il nome della colonna della posizione e quello
# della quantita' («Pos. Part Number Descrizione Q.ty», «INDICE/INDEX CODICE/CODE Q.tà/Q.ty DENOMINAZIONE/
# DESCRIPTION», «Item Q.ty Denominazione-Name N. dis-P/N»). Le sue parole sono nomi di colonne, non etichette di
# campi: prima «Part Number» era il campo del codice, e il suo valore «nella riga sotto» era la prima riga
# dell'elenco, cioe' un figlio (scenario del 28/09). Le righe dell'elenco non si leggono qui (fase 4.12).
_COLONNA_POSIZIONE = {"POS", "POS.", "POSIZIONE", "POSITION", "ITEM", "INDICE", "INDEX"}
_COLONNA_QUANTITA = {"Q.TY", "QTY", "QTY.", "Q.TA", "Q.TÀ", "Q.TA'", "QTA", "QTÀ", "QUANTITA", "QUANTITÀ",
                     "QUANTITA'", "QUANTITY"}
# I nomi brevi della colonna della posizione (fase 4.6r: «N. CODICE DESCRIZIONE QTA», «RIF. PART NUMBER DESCRIPTION
# QTY»; «ITEM NO.» ha gia' «ITEM»). Contano solo come parole a se', fuori da un'etichetta del cartiglio: in «N.
# DISEGNO», «NR. DISEGNO» e «PART N.» sono pezzi dell'etichetta, e una riga del cartiglio con la quantita' accanto
# non diventa un'intestazione (perderebbe il suo campo).
_COLONNA_POSIZIONE_BREVE = {"N.", "N°", "NR", "NR.", "RIF", "RIF."}


def _parole_fuori_etichette(riga: list[Parola]) -> list[int]:
    """Gli indici delle parole della riga che non fanno parte di un'etichetta del cartiglio."""
    fuori: list[int] = []
    i = 0
    while i < len(riga):
        e = etichetta_a(riga, i)
        if e:
            i += e[1]
            continue
        fuori.append(i)
        i += 1
    return fuori


def intestazione_tabella(riga: list[Parola]) -> bool:
    """La riga vista e' l'intestazione di una tabella: ha la colonna della posizione e quella della quantita'. Un nome
    breve della posizione («N.», «RIF.») conta solo fuori dalle etichette del cartiglio."""
    pezzi = {p for w in riga for p in _pezzi(w.testo)}
    if not pezzi & _COLONNA_QUANTITA:
        return False
    if pezzi & _COLONNA_POSIZIONE:
        return True
    return any(_norm_etichetta(riga[i].testo) in _COLONNA_POSIZIONE_BREVE for i in _parole_fuori_etichette(riga))


def righe_viste(parole: list[Parola]) -> list[list[Parola]]:
    """Le parole raggruppate per riga VISTA (lo stesso centro verticale, a meno di mezza altezza), ognuna da
    sinistra a destra. Etichetta e valore di un cartiglio stanno spesso in due oggetti di testo diversi, che
    per PyMuPDF sono blocchi diversi: la riga vista li rimette insieme."""
    righe: list[list[Parola]] = []
    for p in sorted(parole, key=lambda q: ((q.y0 + q.y1) / 2, q.x0)):
        c = (p.y0 + p.y1) / 2
        if righe:
            ultima = righe[-1]
            cu = sum((q.y0 + q.y1) / 2 for q in ultima) / len(ultima)
            h = max(q.y1 - q.y0 for q in ultima)
            if abs(c - cu) <= h / 2:
                ultima.append(p)
                continue
        righe.append([p])
    return [sorted(r, key=lambda q: q.x0) for r in righe]


def _valore_da(parole: list[Parola], da: int, altezza: float, dopo_x1: float | None = None) -> list[Parola]:
    """Le parole di una riga da `da` in poi che fanno il valore: fino alla prossima etichetta o a un salto
    piu' largo di quattro altezze (un'altra cella del cartiglio), contato anche dalla fine dell'etichetta
    (`dopo_x1`) quando il valore e' sulla sua stessa riga. I separatori in testa («:», «-», «/») si tolgono. Il
    segnaposto del particolare simile e' una cella intera: se il valore comincia con lui e' lui, e basta; dopo
    altre parole chiude il valore."""
    out: list[Parola] = []
    ultimo = dopo_x1
    for i in range(da, len(parole)):
        p = parole[i]
        s = segnaposto_a(parole, i)
        if s:
            if not out and (ultimo is None or p.x0 - ultimo <= 4 * altezza):
                out = parole[i:i + s]
            break
        if etichetta_a(parole, i):
            break
        if ultimo is not None and p.x0 - ultimo > 4 * altezza:
            break
        ultimo = p.x1
        if out or p.testo.strip() not in _SEPARATORI_VALORE:
            out.append(p)
    return out


def campi_cartiglio(parole: list[Parola], zona, fonte: str) -> list[dict]:
    """I campi etichetta -> valore della pagina 1, con il riquadro del valore e la zona dell'etichetta. Le righe
    che sono l'intestazione di una tabella non hanno etichette; il segnaposto del particolare simile non ha
    etichette e non e' il valore del campo (un campo senza valore non si riporta)."""
    righe = righe_viste(parole)
    out: list[dict] = []
    for r_i, riga in enumerate(righe):
        if intestazione_tabella(riga):
            continue
        i = 0
        while i < len(riga):
            s = segnaposto_a(riga, i)
            if s:
                i += s
                continue
            trovata = etichetta_a(riga, i)
            if not trovata:
                i += 1
                continue
            campo, n = trovata
            et = riga[i:i + n]
            h = max(p.y1 - p.y0 for p in et)
            valore = _valore_da(riga, i + n, h, et[-1].x1)
            if not valore:
                # la riga subito sotto, nella colonna dell'etichetta; l'intestazione di una tabella sotto dice che
                # li' comincia la tabella, non il valore
                x0, x1, y1 = et[0].x0 - 2 * h, et[-1].x1 + 2 * h, max(p.y1 for p in et)
                for sotto in righe[r_i + 1:]:
                    dy = min(p.y0 for p in sotto) - y1
                    if dy > 2.5 * h or intestazione_tabella(sotto):
                        break
                    if dy < -0.3 * h:
                        continue
                    inizio = next((k for k, p in enumerate(sotto) if p.x1 >= x0 and p.x0 <= x1), None)
                    if inizio is not None:
                        valore = _valore_da(sotto, inizio, h)
                        break
            if valore and segnaposto_a(valore, 0):
                valore = []  # il segnaposto del modello: il campo e' vuoto
            testo = " ".join(p.testo for p in valore).strip()[:TESTO_PDF_VALORE_MAX]
            if testo:
                out.append({"etichetta": campo, "letta": " ".join(p.testo for p in et), "valore": testo,
                            "pagina": 1, "zona": ZONA_BASSO_DESTRA if in_zona(et[0], zona) else ZONA_PAGINA,
                            "fonte": fonte, "riquadro": riquadro(valore), "confidenza": confidenza_media(valore)})
            i += n
    return out


# ---- il motore OCR

class MotoreTesseract:
    """L'OCR di PyMuPDF (Tesseract dentro MuPDF), su una zona della pagina vista: l'immagine della sola zona,
    alla risoluzione data, e le parole lette riportate sulla pagina. PyMuPDF non da' la confidenza delle
    parole: resta None."""
    nome = "tesseract (pymupdf)"

    def __init__(self, lingua: str, dpi: int, tessdata: str):
        self.lingua, self.dpi, self.tessdata = lingua, dpi, tessdata

    def leggi(self, pag, clip) -> list[Parola]:
        # `clip` e' sulla pagina vista, come l'immagine che ne esce (get_pixmap applica la rotazione)
        pix = pag.get_pixmap(dpi=self.dpi, clip=clip)
        dati = pix.pdfocr_tobytes(language=self.lingua, tessdata=self.tessdata)
        with pymupdf.open("pdf", dati) as d:
            p = d[0]
            sx, sy = clip.width / p.rect.width, clip.height / p.rect.height
            return [Parola(clip.x0 + x0 * sx, clip.y0 + y0 * sy, clip.x0 + x1 * sx, clip.y0 + y1 * sy, w, b, r, n)
                    for x0, y0, x1, y1, w, b, r, n in p.get_text("words")]


class ConfigOCR:
    """Come si fa l'OCR su questa postazione: `[analisi] ocr` di worker.toml («automatico», il default: se il
    testo nativo non basta e Tesseract c'e'; «spento»: mai), la lingua, la risoluzione, la cartella tessdata.

    Il motore si cerca la prima volta che serve (pymupdf.get_tessdata lancia un processo) e l'esito si
    ricorda. `motore` e `orologio` si possono passare per le prove (un motore finto, un tempo finto)."""

    def __init__(self, modo: str = "automatico", lingua: str = OCR_LINGUA, dpi: int = OCR_DPI, tessdata: str = "",
                 tempo_max_s: float = OCR_TEMPO_MAX_S, motore=None, orologio=time.monotonic):
        self.modo, self.lingua, self.dpi, self.tessdata = modo, lingua, dpi, tessdata
        self.tempo_max_s, self.motore, self.orologio = tempo_max_s, motore, orologio
        self._pronto: tuple[object | None, str] | None = None

    def motore_pronto(self) -> tuple[object | None, str]:
        """(motore, "") o (None, perche' non c'e')."""
        if self.motore is not None:
            return self.motore, ""
        if self._pronto is None:
            try:
                td = pymupdf.get_tessdata(self.tessdata or None)
                self._pronto = (MotoreTesseract(self.lingua, self.dpi, td), "")
            except Exception as e:  # noqa: BLE001 — qualunque motivo, il motore non c'e'
                self._pronto = (None, f"Tesseract non disponibile su questa postazione ({type(e).__name__}: {e})"[:300])
        return self._pronto


def ocr_da_config(cfg: dict) -> ConfigOCR:
    """La configurazione dell'OCR dal worker.toml: `ocr` («automatico» | «spento»), `ocr_lingua`, `ocr_dpi`,
    `ocr_tessdata`. Un valore sconosciuto di `ocr` vale il default, e si dice nel log."""
    modo = str(cfg.get("ocr", "automatico")).strip().lower()
    if modo not in ("automatico", "spento"):
        log.warning("ocr = %r non e' un valore previsto (automatico, spento): vale «automatico»", cfg.get("ocr"))
        modo = "automatico"
    return ConfigOCR(modo=modo, lingua=str(cfg.get("ocr_lingua") or OCR_LINGUA),
                     dpi=int(cfg.get("ocr_dpi") or OCR_DPI), tessdata=str(cfg.get("ocr_tessdata") or ""))


# La configurazione dell'OCR di questo processo: WorkerAnalisi la sostituisce con quella del suo worker.toml.
OCR_PREDEFINITO = ConfigOCR()


def piano_ocr(pagine: list[tuple]) -> tuple[list[tuple[int, str, object]], list[int]]:
    """Dove serve l'OCR (le soglie in testa): le pagine senza testo nativo, intere, e la sola zona in basso a
    destra della pagina 1 quando la pagina ha testo ma la zona no. `pagine` sono (vista, parole, caratteri) delle
    pagine lette. Restituisce il piano (indice, zona, riquadro) e le pagine senza testo oltre OCR_PAGINE_MAX."""
    piano, oltre = [], []
    for i, (vista, parole, caratteri) in enumerate(pagine):
        if caratteri < OCR_SOGLIA_PAGINA:
            if sum(1 for _, z, _ in piano if z == ZONA_PAGINA) < OCR_PAGINE_MAX:
                piano.append((i, ZONA_PAGINA, vista))
            else:
                oltre.append(i)
        elif i == 0:
            zona = zona_basso_destra(vista)
            if len(normalizza_testo(" ".join(p.testo for p in parole if in_zona(p, zona)))) < OCR_SOGLIA_CARTIGLIO:
                piano.append((0, ZONA_BASSO_DESTRA, zona))
    return piano, oltre


def parola_ocr(p) -> Parola | None:
    """Una parola del motore OCR resa sicura, o None se non si puo' usare. Il motore e' codice di terzi (o
    finto): le coordinate devono essere numeri finiti, il testo una stringa non vuota; una confidenza che non e'
    un numero finito diventa None (la parola resta, senza confidenza). Blocco, riga e posizione mancanti o
    storti valgono 0. Cosi' quello che viene dopo (frammenti, campi, il fatto) lavora solo su parole ben fatte e
    un risultato storto del motore non fa fallire l'analisi."""
    try:
        x0, y0, x1, y1 = (float(v) for v in (p.x0, p.y0, p.x1, p.y1))
        testo = p.testo
    except (AttributeError, TypeError, ValueError):
        return None
    if not all(math.isfinite(v) for v in (x0, y0, x1, y1)) or not isinstance(testo, str) or not testo.strip():
        return None

    def intero(nome: str) -> int:
        try:
            return int(getattr(p, nome, 0) or 0)
        except (TypeError, ValueError):
            return 0

    try:
        c = getattr(p, "confidenza", None)
        confidenza = float(c) if c is not None else None
    except (TypeError, ValueError):
        confidenza = None
    if confidenza is not None and not math.isfinite(confidenza):
        confidenza = None
    return Parola(min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1), testo, intero("blocco"), intero("riga"),
                  intero("n"), confidenza)


def esegui_ocr(doc, piano: list[tuple[int, str, object]], oltre: list[int], cfg: ConfigOCR) -> tuple[dict, list]:
    """Esegue il piano dell'OCR e dice com'e' andata (il fatto `ocr`) con le parole lette per passaggio. Non
    solleva MAI: ogni passaggio che non va si scrive, e l'analisi prosegue con il testo nativo."""
    def tentativo(i, zona, esito, motivo="", caratteri=0, secondi=0.0):
        return {"pagina": i + 1, "zona": zona, "esito": esito, "motivo": motivo, "caratteri": caratteri,
                "secondi": round(secondi, 2)}

    fuori = [tentativo(i, ZONA_PAGINA, "saltato", f"oltre le {OCR_PAGINE_MAX} pagine lette con l'OCR") for i in oltre]
    if not piano:
        return {"stato": "non_necessario", "motivo": "", "motore": "", "tentativi": []}, []
    if cfg.modo == "spento":
        motivo = "OCR spento nella configurazione del worker ([analisi] ocr)"
        return {"stato": "spento", "motivo": motivo, "motore": "",
                "tentativi": [tentativo(i, z, "saltato", motivo) for i, z, _ in piano] + fuori}, []
    motore, motivo = cfg.motore_pronto()
    if motore is None:
        return {"stato": "non_disponibile", "motivo": motivo, "motore": "",
                "tentativi": [tentativo(i, z, "saltato", motivo) for i, z, _ in piano] + fuori}, []
    tentativi, letture = [], []
    inizio = cfg.orologio()
    for i, zona, clip in piano:
        t0 = cfg.orologio()
        if t0 - inizio >= cfg.tempo_max_s:
            tentativi.append(tentativo(i, zona, "scaduto", f"tempo dell'OCR finito ({cfg.tempo_max_s:g} s per file)"))
            continue
        try:
            # le parole del motore si rendono sicure QUI, dentro la guardia: un risultato storto (coordinate
            # mancanti, una confidenza scritta a parole) non deve arrivare ai frammenti e ai campi
            grezze = list(motore.leggi(doc[i], clip) or [])
            parole = [q for q in map(parola_ocr, grezze) if q is not None]
        except Exception as e:  # noqa: BLE001 — l'OCR e' un'evidenza: il suo errore non e' l'errore dell'analisi
            log.warning("OCR fallito su pagina %d (%s): %s", i + 1, zona, e)
            tentativi.append(tentativo(i, zona, "fallito", f"{type(e).__name__}: {e}"[:300], secondi=cfg.orologio() - t0))
            continue
        scartate = len(grezze) - len(parole)
        if zona == ZONA_BASSO_DESTRA:
            parole = [p for p in parole if in_zona(p, clip)]
        secondi = cfg.orologio() - t0
        if scartate and not parole:
            # il motore ha risposto, ma niente di quello che ha dato e' una parola usabile: e' un motore che non va
            tentativi.append(tentativo(i, zona, "fallito", f"risultato del motore non valido: {scartate} parole "
                                       "scartate (coordinate o testo mancanti)", secondi=secondi))
            continue
        if not any(c.isalnum() for p in parole for c in p.testo):
            tentativi.append(tentativo(i, zona, "vuoto", "l'OCR non ha letto niente di leggibile", secondi=secondi))
            continue
        tentativi.append(tentativo(i, zona, "letto", f"{scartate} parole del motore scartate: non valide" if scartate else "",
                                   caratteri=sum(len(p.testo) for p in parole), secondi=secondi))
        letture.append((i, zona, parole))
    esiti = {t["esito"] for t in tentativi}
    stato = ("eseguito" if "letto" in esiti else "scaduto" if "scaduto" in esiti
             else "fallito" if "fallito" in esiti else "illeggibile")
    motivo = next((t["motivo"] for t in tentativi if t["esito"] != "letto" and t["motivo"]), "")
    return {"stato": stato, "motivo": motivo, "motore": nome_motore(motore), "tentativi": tentativi + fuori}, letture


def nome_motore(motore) -> str:
    """Il nome del motore per il fatto, sempre una stringa corta. Il nome lo da' il motore, e un motore che
    dichiara `nome = None` (o un oggetto qualunque) non deve far fallire la validazione del fatto: prima lo
    faceva, e l'analisi intera finiva in `errore_pdf` (correzione F9, giro 2)."""
    try:
        return str(getattr(motore, "nome", "") or type(motore).__name__)[:100]
    except Exception:  # noqa: BLE001 — un __str__ che solleva: si usa il nome della classe
        return type(motore).__name__[:100]


def leggi_testo_pdf(doc, ocr: ConfigOCR | None = None) -> tuple[str, dict]:
    """Legge le prime pagine del PDF una volta sola, per due cose: il testo in cui cercare i termini (come
    prima: il testo nativo delle pagine una dopo l'altra) e il fatto `testo_pdf`.

    Il fatto: i frammenti della zona in basso a destra della pagina 1 (tutti) e quelli del resto delle pagine
    che hanno almeno una cifra (un codice ne ha: il resto non serve a interpretare il file e non viaggia),
    prima i nativi e poi quelli dell'OCR, fino ai limiti (`troncato`); i campi del cartiglio della pagina 1; i
    metadati senza l'autore; l'esito dell'OCR selettivo (piano_ocr, esegui_ocr); i limiti e le soglie in vigore.
    `estraibile = false` quando nelle pagine lette non c'e' testo nativo: e' la risposta, non un errore."""
    ocr = ocr or OCR_PREDEFINITO
    testo_completo = ""
    pagine: list[tuple] = []  # (vista, parole, caratteri nativi) di ogni pagina letta
    for i, pag in enumerate(doc):
        if i >= TESTO_PDF_PAGINE_MAX:  # non serve scorrere oltre
            break
        grezzo = pag.get_text()
        testo_completo += grezzo + "\n"
        pagine.append((pag.rect, parole_native(pag), len(normalizza_testo(grezzo))))

    zona1 = zona_basso_destra(pagine[0][0]) if pagine else None

    def zona_di(i: int):
        return lambda p: ZONA_BASSO_DESTRA if i == 0 and in_zona(p, zona1) else ZONA_PAGINA

    nativi = [f for i, (_, parole, _) in enumerate(pagine) for f in componi_frammenti(parole, i + 1, "nativo", zona_di(i))]
    campi = campi_cartiglio(pagine[0][1], zona1, "nativo") if pagine else []

    try:
        piano, oltre = piano_ocr(pagine)
        esito_ocr, letture = esegui_ocr(doc, piano, oltre, ocr)
    except Exception as e:  # noqa: BLE001 — nemmeno un errore del piano fa fallire l'analisi
        log.warning("OCR non eseguito: %s", e)
        esito_ocr, letture = {"stato": "fallito", "motivo": f"{type(e).__name__}: {e}"[:300], "motore": "",
                              "tentativi": []}, []
    # Anche quello che si ricava dalle parole dell'OCR sta sotto una guardia, fino ai modelli del contratto:
    # se qualcosa non va, i frammenti e i campi dell'OCR si buttano, il fatto dice `fallito` con il motivo e il
    # testo nativo viaggia com'e'.
    da_ocr, campi_ocr = [], []
    try:
        for i, _, parole in letture:
            da_ocr += componi_frammenti(parole, i + 1, "ocr", zona_di(i))
            if i == 0:
                campi_ocr += campi_cartiglio(parole, zona1, "ocr")
        for f in da_ocr:
            FrammentoPDF(**f)
        for c in campi_ocr:
            CampoCartiglio(**c)
        OCRPDF(**esito_ocr)  # anche l'esito, che porta cose dette dal motore (il nome)
    except Exception as e:  # noqa: BLE001 — l'OCR e' un'evidenza: il suo errore non e' l'errore dell'analisi
        log.warning("OCR scartato: %s", e)
        motivo = f"risultato dell'OCR non utilizzabile ({type(e).__name__}: {e})"[:300]
        esito_ocr = {**esito_ocr, "stato": "fallito", "motivo": motivo,
                     "tentativi": [{**t, "esito": "fallito", "motivo": motivo} if t["esito"] == "letto" else t
                                   for t in esito_ocr.get("tentativi", [])]}
        da_ocr, campi_ocr = [], []
        try:
            OCRPDF(**esito_ocr)
        except Exception:  # noqa: BLE001 — l'esito stesso non si valida: resta solo quello che si sa di sicuro
            esito_ocr = {"stato": "fallito", "motivo": motivo, "motore": "", "tentativi": []}
    campi += campi_ocr

    def utili(fr: list[dict], zona: str) -> list[dict]:
        return [f for f in fr if f["zona"] == zona and (zona == ZONA_BASSO_DESTRA or RE_CIFRA.search(f["testo"]))]

    candidati = (utili(nativi, ZONA_BASSO_DESTRA) + utili(da_ocr, ZONA_BASSO_DESTRA)
                 + utili(nativi, ZONA_PAGINA) + utili(da_ocr, ZONA_PAGINA))
    frammenti, resto, troncato = [], TESTO_PDF_CARATTERI_MAX, False
    for f in candidati:
        if len(frammenti) >= TESTO_PDF_FRAMMENTI_MAX or resto <= 0:
            troncato = True
            break
        tetto = min(TESTO_PDF_FRAMMENTO_MAX, resto)
        if len(f["testo"]) > tetto:
            f["testo"], troncato = f["testo"][:tetto], True
        frammenti.append(f)
        resto -= len(f["testo"])
    visti, unici = set(), []
    for c in campi:
        k = (c["etichetta"], c["valore"].upper(), c["fonte"])
        if k not in visti:
            visti.add(k)
            unici.append(c)
    if len(unici) > TESTO_PDF_CAMPI_MAX:
        unici, troncato = unici[:TESTO_PDF_CAMPI_MAX], True

    md = doc.metadata or {}

    def meta(chiave: str) -> str:
        return (md.get(chiave) or "").strip()[:TESTO_PDF_FRAMMENTO_MAX]

    caratteri = sum(c for _, _, c in pagine)
    fatto = TestoPDF(
        versione=VERSIONE_TESTO_PDF,
        estraibile=caratteri > 0,
        pagine=doc.page_count,
        pagine_lette=len(pagine),
        caratteri=caratteri,
        troncato=troncato,
        formato_pagina1=[round(pagine[0][0].width, 1), round(pagine[0][0].height, 1)] if pagine else [],
        frammenti=[FrammentoPDF(**f) for f in frammenti],
        cartiglio=[CampoCartiglio(**c) for c in unici],
        metadati=MetadatiPDF(titolo=meta("title"), soggetto=meta("subject"), parole_chiave=meta("keywords"),
                             creatore=meta("creator"), produttore=meta("producer")),
        ocr=OCRPDF(**esito_ocr),
        limiti=LimitiTestoPDF(pagine_max=TESTO_PDF_PAGINE_MAX, frammenti_max=TESTO_PDF_FRAMMENTI_MAX,
                              frammento_max=TESTO_PDF_FRAMMENTO_MAX, caratteri_max=TESTO_PDF_CARATTERI_MAX,
                              campi_max=TESTO_PDF_CAMPI_MAX, ocr_soglia_pagina=OCR_SOGLIA_PAGINA,
                              ocr_soglia_cartiglio=OCR_SOGLIA_CARTIGLIO, ocr_pagine_max=OCR_PAGINE_MAX,
                              ocr_tempo_max_s=int(ocr.tempo_max_s), ocr_dpi=ocr.dpi),
    )
    return testo_completo, fatto.model_dump(mode="json")


def fonti_pdf(tipo: str, codice: str, rev: str) -> dict:
    """Da dove viene ciascuna lettura in testa al risultato di un PDF (A5.13.8): il tipo dai termini del
    testo nativo (o dal nome «SO …»), codice e rev SEMPRE dal nome del file. `codice`/`rev`/`fonte` in testa
    restano quelli di prima per compatibilita'; il server di oggi non li usa per le colonne (P15, P16), e un
    codice scritto dentro il file lo trova lui nel `testo_pdf`."""
    fonti = {}
    if tipo:
        fonti["tipo"] = tipo
    if codice:
        fonti["codice"] = "nome_file"
    if rev:
        fonti["rev"] = "nome_file"
    return fonti


def analizza_pdf(percorso: str, nome_file: str, ocr: ConfigOCR | None = None) -> dict:
    """Che cosa c'e' dentro questo PDF.

    Checkpoint 3R §5. Un PDF non e' un disegno: e' un contenitore. In una richiesta d'offerta vera
    contiene, con frequenze paragonabili, il disegno quotato, il capitolato, la distinta del cliente,
    l'offerta di un fornitore, la conferma d'ordine o la firma di qualcuno. Fino a qui il tipo lo
    decideva l'estensione, cioe' lo decideva prima di aprire il file; da qui lo decide il CONTENUTO, e
    quando il contenuto non basta la risposta e' `da_determinare` — che e' una risposta, non un errore.

    Smistamento F9 (analizzatore 4): ogni ramo di un PDF che si apre porta anche `dettagli.testo_pdf`, la
    lettura strutturata del testo con l'OCR selettivo (leggi_testo_pdf; `ocr` e' la configurazione
    dell'OCR, OCR_PREDEFINITO se manca), e `dettagli.fonti`, che dice da dove viene ciascuna lettura in
    testa. Il tipo si legge dai termini del solo testo NATIVO: l'OCR e' un'evidenza per il server, non
    decide il tipo qui.
    """
    try:
        # Il PDF si apre DA MEMORIA, non per percorso (7C.1, P0): su Windows PyMuPDF tiene aperto un
        # file che non riesce ad aprire, e il temporaneo del worker non si cancella piu' («utilizzato
        # da un altro processo»). Letto in memoria, il file e' chiuso prima che PyMuPDF lo veda.
        with open(percorso, "rb") as f:
            dati = f.read()
        with pymupdf.open(stream=dati, filetype="pdf") as doc:
            testo_completo, testo_pdf = leggi_testo_pdf(doc, ocr)
    except Exception as e:
        # Un PDF che non si apre non e' un disegno con confidenza 40: e' un PDF che non si apre. Niente
        # `testo_pdf`: non e' un PDF senza testo, e' un PDF che non si e' letto (lo dice `errore_pdf`).
        log.warning("impossibile leggere PDF %s con pymupdf: %s", percorso, e)
        return {
            "tipo_proposto": "da_determinare",
            "codice": "",
            "rev": "",
            "confidenza": 20,
            "fonte": "estensione",
            "dettagli": {"errore_pdf": str(e)},
        }

    # Normalizza spaziature e newline per ricerca esatta case-insensitive
    testo_norm = re.sub(r"\s+", " ", testo_completo).upper()
    nome_upper = nome_file.upper().strip()
    nome_senza_ext = Path(nome_file).stem

    # 1. Regola Offerte Commerciali:
    # Se il PDF contiene "Spett. le Offerta", "CONDIZIONI GENERALI DI VENDITA", "Pagamento" o "Incoterms",
    # oppure il nome file inizia per "SO "
    trovati_comm = [t for t in TERMINI_OFFERTE_COMMERCIALI if t in testo_norm]
    forti = [t for t in trovati_comm if t in TERMINI_OFFERTE_FORTI]
    dai_termini = bool(forti) or len(trovati_comm) >= 2
    e_offerta = nome_upper.startswith("SO ") or dai_termini
    if e_offerta:
        log.info("Riconosciuta offerta commerciale in %s (termini: %s)", nome_file, trovati_comm)
        return {
            "tipo_proposto": "offerta_promatec",
            "codice": "",
            "rev": "",
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"commerciale": True, "termini_trovati": trovati_comm, "testo_pdf": testo_pdf,
                         "fonti": fonti_pdf("termini_pdf" if dai_termini else "nome_file", "", "")},
        }

    # 2. Regola CAD 2D:
    # Se il PDF contiene "ZONA ESENTE DA SALDATURA", "TOLLERANZE GENERALI", "PESO kG" o "SCALA"
    trovati_cad = [t for t in TERMINI_CARTIGLIO_CAD if t in testo_norm]
    codice, rev = separa_codice_rev(nome_senza_ext)

    if trovati_cad:
        # «cartiglio» e' la fonte del TIPO (i termini del cartiglio nel testo); codice e rev sono il nome del
        # file, e `fonti` lo dice (P16)
        log.info("Riconosciuto cartiglio CAD 2D in %s: codice=%s rev=%s (termini: %s)", nome_file, codice, rev, trovati_cad)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"cartiglio": True, "termini_trovati": trovati_cad, "testo_pdf": testo_pdf,
                         "fonti": fonti_pdf("termini_pdf", codice, rev)},
        }

    # 3. Capitolato / specifica tecnica: parla di requisiti, non di un pezzo.
    trovati_cap = [t for t in TERMINI_CAPITOLATO if t in testo_norm]
    if len(trovati_cap) >= 2:
        log.info("Riconosciuto capitolato in %s (termini: %s)", nome_file, trovati_cap)
        return {
            "tipo_proposto": "capitolato",
            "codice": "",
            "rev": "",
            "confidenza": 80,
            "fonte": "cartiglio",
            "dettagli": {"capitolato": True, "termini_trovati": trovati_cap, "testo_pdf": testo_pdf,
                         "fonti": fonti_pdf("termini_pdf", "", "")},
        }

    # 4. Distinta del cliente: un elenco di codici con quantita'.
    trovati_dist = [t for t in TERMINI_DISTINTA if t in testo_norm]
    if len(trovati_dist) >= 2 and conta_righe_codice(testo_completo) >= 5:
        log.info("Riconosciuta distinta cliente in %s (termini: %s)", nome_file, trovati_dist)
        return {
            "tipo_proposto": "distinta_cliente",
            "codice": "",
            "rev": "",
            "confidenza": 75,
            "fonte": "cartiglio",
            "dettagli": {"distinta": True, "termini_trovati": trovati_dist, "testo_pdf": testo_pdf,
                         "fonti": fonti_pdf("termini_pdf", "", "")},
        }

    # 5. Nessun contenuto riconosciuto. Il codice nel nome NON basta a dire che e' un disegno: e'
    #    esattamente cosi' che «1234567A.pdf» diventava un CAD al 70% mentre era l'offerta di un
    #    fornitore per quel pezzo. Il codice si conserva, il tipo resta da determinare.
    return {
        "tipo_proposto": "da_determinare",
        "codice": codice,
        "rev": rev,
        "confidenza": 40 if codice else 30,
        "fonte": "nome_file" if codice else "estensione",
        "dettagli": {"codice_riconosciuto": codice, "testo_letto": len(testo_norm), "testo_pdf": testo_pdf,
                     "fonti": fonti_pdf("", codice, rev)},
    }


def conta_righe_codice(testo: str) -> int:
    """Quante righe sembrano una voce di distinta: un codice e una quantita'."""
    n = 0
    for riga in testo.splitlines():
        if RE_RIGA_DISTINTA.search(riga):
            n += 1
    return n


def analizza_step(percorso: str, nome_file: str) -> dict:
    """Che cosa c'e' dentro questo file STEP.

    Due letture, e vanno tenute distinte (addendum B8, A1.2).

    La prima e' quella di sempre: il primo `PRODUCT` dei primi 100 KB, passato da `sembra_codice`, che
    diventa `codice`/`rev` del risultato. E' un'IPOTESI dal nome, buona per `documento_proposta`, e
    resta com'e' per non cambiare sotto i piedi a chi la legge gia'.

    La seconda e' la STRUTTURA: nodi, relazioni e quantita', con gli attributi grezzi delle entita' e
    nessuna interpretazione. Nei nodi non c'e' nessun codice, e non e' una dimenticanza: che cosa sia
    un codice lo dicono le regole del CLIENTE della richiesta, e il worker non sa nemmeno di che
    cliente si tratti. La struttura non fa mai fallire il job: se il file non si legge resta l'avviso,
    e il tipo lo dice comunque l'estensione.
    """
    nome_senza_ext = Path(nome_file).stem
    codice, rev = separa_codice_rev(nome_senza_ext)
    esito = {
        "tipo_proposto": "cad_3d",
        "codice": codice,
        "rev": rev,
        "confidenza": 80 if codice else 60,
        "fonte": "nome_file" if codice else "estensione",
        "dettagli": {"struttura": leggi_struttura(percorso)},
    }
    try:
        with open(percorso, "r", encoding="utf-8", errors="ignore") as f:
            # Leggi primi 100KB per trovare PRODUCT
            contenuto = f.read(100 * 1024)
        m = RE_STEP_PRODUCT.search(contenuto)
        if m:
            codice_step = m.group(1).strip()
            esito["dettagli"]["product_step"] = codice_step
            if sembra_codice(codice_step):
                c_step, r_step = separa_codice_rev(codice_step)
                esito.update({
                    "codice": c_step or codice_step,
                    "rev": r_step or rev,
                    "confidenza": 95,
                    "fonte": "step",
                })
    except Exception as e:
        log.warning("lettura STEP fallita per %s: %s", percorso, e)
    return esito


def rimuovi_con_pazienza(percorso: str, tentativi: int = 10, attesa_s: float = 0.2) -> bool:
    """Cancella un file temporaneo su Windows, dove un handle ancora aperto fa fallire la rimozione.

    E' successo alla prima prova del download (7C.1, P0): PyMuPDF che NON riesce ad aprire un file
    lo tiene comunque aperto finche' l'oggetto non viene raccolto, e `os.remove` risponde
    «Il file e' utilizzato da un altro processo». Il file restava nella tmp del worker fino al
    riavvio. Qui si forza la raccolta e si riprova per qualche decimo di secondo; se non basta, lo
    si dice nel log e ci pensa svuota_tmp all'avvio successivo.
    """
    for i in range(tentativi):
        try:
            if os.path.exists(percorso):
                os.remove(percorso)
            return True
        except OSError as e:
            if i == tentativi - 1:
                log.warning("file temporaneo non rimosso: %s (%s)", percorso, e)
                return False
            gc.collect()
            time.sleep(attesa_s)
    return False


def analizza_file(path_staging: str, nome_file: str, ocr: ConfigOCR | None = None) -> dict:
    """Smista l'analisi in base all'estensione del file in staging. `ocr` vale solo per i PDF."""
    ext = Path(nome_file).suffix.lower().lstrip(".")
    if not os.path.isfile(path_staging):
        raise FileNotFoundError(f"file non trovato in staging: {path_staging}")

    if ext == "pdf":
        return analizza_pdf(path_staging, nome_file, ocr)
    if ext in ("stp", "step"):
        return analizza_step(path_staging, nome_file)
    if ext == "dxf":
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "sviluppo_dxf",
            "codice": codice,
            "rev": rev,
            "confidenza": 85 if codice else 70,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {},
        }
    if ext == "dwg":
        # Un DWG e' un file CAD: l'estensione lo dice davvero, non lo suppone.
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 80 if codice else 60,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {},
        }
    if ext in ("tif", "tiff"):
        # Un'immagine scansionata puo' essere un disegno, ma anche un capitolato fotocopiato o un
        # documento di trasporto. Senza OCR non lo sappiamo, e non lo diciamo (checkpoint 3R §5).
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "da_determinare",
            "codice": codice,
            "rev": rev,
            "confidenza": 40 if codice else 30,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {"nota": "immagine non letta: serve OCR"},
        }
    if ext in ("xls", "xlsx", "csv"):
        return {
            "tipo_proposto": "commerciale",
            "codice": "",
            "rev": "",
            "confidenza": 60,
            "fonte": "estensione",
            "dettagli": {},
        }

    codice, rev = separa_codice_rev(Path(nome_file).stem)
    return {
        "tipo_proposto": "altro",
        "codice": codice,
        "rev": rev,
        "confidenza": 30,
        "fonte": "estensione",
        "dettagli": {},
    }


class WorkerAnalisi:
    def __init__(self, cfg: dict):
        self.cfg = cfg
        self.api = Cockpit(cfg["server_url"], cfg["token"], impronta=cfg.get("impronta", ""))
        self.worker_id = nome_worker("analisi", cfg)
        # Cartella DEL WORKER (7C.1, P0): qui scende il contenuto da analizzare, il tempo di leggerlo.
        # Non e' lo staging del server, che puo' stare su un altro PC.
        self.staging = os.path.abspath(cfg.get("staging") or ".")
        self.svuota_tmp()
        self.battito: Battito | None = None
        # quanto si concede al lavoro per fermarsi da solo dopo un 409, prima dell'uscita forzata (C16)
        self.arresto_forzato_s = float(cfg.get("arresto_forzato_s", 15))
        self.marcatore_arresto = os.path.join(self.staging, "ultimo_arresto.txt")
        self.ultimo_arresto = leggi_marcatore_arresto(self.marcatore_arresto)
        # l'OCR selettivo dei PDF (F9): «automatico» se c'e' Tesseract su questo PC, «spento» a richiesta
        self.ocr = ocr_da_config(cfg)

    def svuota_tmp(self) -> int:
        """`tmp\\` e' di passaggio: il contenuto scaricato dal server sta li' il tempo dell'analisi e si
        cancella subito dopo. Cio' che resta e' di un processo morto a meta', e nessuno lo riprendera':
        il job e' tornato in coda e il tentativo dopo lo riscarica. Si svuota all'avvio."""
        tmp = os.path.join(self.staging, "tmp")
        if not os.path.isdir(tmp):
            return 0
        n = 0
        for nome in os.listdir(tmp):
            p = os.path.join(tmp, nome)
            try:
                if os.path.isdir(p):
                    shutil.rmtree(p)
                else:
                    os.remove(p)
                n += 1
            except OSError as e:
                log.warning("tmp non svuotata: %s (%s)", p, e)
        if n:
            log.info("cartella tmp svuotata all'avvio: %d voci di tentativi precedenti", n)
        return n

    def controlla(self) -> None:
        """Punto di ripresa: se il battito ha perso il lease, il lavoro si ferma qui.

        Sta fra il download e l'analisi, che e' il confine fra le due cose lunghe di questo worker: piu'
        in la' la lettura del file non e' interrompibile, e a fermare il processo e' l'uscita forzata
        del battito (C16)."""
        if self.battito is not None:
            self.battito.controlla()

    def segna(self, fase: str) -> None:
        if self.battito is not None:
            self.battito.segna_fase(fase)

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s collegato a %s", self.worker_id, self.api.url)
        attesa = 5
        while True:
            try:
                extra = {"postazione": socket.gethostname().upper()}
                if self.ultimo_arresto:
                    extra["ultimo_arresto"] = self.ultimo_arresto
                r = self.api.claim("analisi", self.worker_id, extra=extra)
                self.ultimo_arresto = ""            # riportato una volta: il server lo conserva
                job = Job.model_validate(r) if r else None
                attesa = 5
            except (ImprontaSbagliata, ErroreHTTP, *ERRORI_RETE) as e:
                # Stessa regola del worker Outlook: il log deve dire se il server ha risposto (e che
                # cosa) o se non si è fatto trovare. Sono due indagini diverse.
                d = diagnosi(e)
                (log.error if d.grave else log.warning)(
                    "%s: riprovo fra %d s", d.testo, d.pausa_s or attesa)
                if una_volta and d.grave:
                    return
                time.sleep(d.pausa_s or attesa)
                attesa = 5 if d.pausa_s else min(attesa * 2, 30)
                continue

            if job is None:
                if una_volta:
                    return
                continue

            self.esegui(job)
            if una_volta:
                return

    def esegui(self, job: Job) -> None:
        t0 = time.time()
        log.info("job %d %s (tentativo %d)", job.job_id, job.tipo, job.tentativi)
        # Il battito sta su un thread suo per tutta la durata del job, come nel worker Outlook. Il
        # lease di un'analisi e' di 120 secondi (coda.go) e un PDF da qualche centinaio di pagine, o un
        # download da un server lento, ci arrivano senza fatica: senza battito il server riprende il
        # job a meta' lavoro e poi RIFIUTA il result con 409 — analisi fatta, tempo buttato, proposta
        # invariata. Se il 409 arriva lo stesso, il thread alza il flag: il lavoro si ferma al primo
        # punto di ripresa (controlla) e, se e' dentro una lettura che non ritorna, dopo
        # arresto_forzato_s il processo esce con codice 3 e l'attivita' pianificata lo riavvia (C16).
        with Battito(self.api, job.job_id, self.worker_id, job.lease_token,
                     ogni_s=cadenza_battito(job.lease_s), arresto_forzato_s=self.arresto_forzato_s) as b:
            b.marcatore_arresto = self.marcatore_arresto
            self.battito = b
            b.segna_fase(job.tipo)
            try:
                if job.tipo != "analizza_allegato":
                    raise ValueError(f"tipo job imprevisto per worker analisi: {job.tipo}")
                p = PayloadAnalizzaAllegato.model_validate(job.payload)
                ris = self.analizza(job, p)
            except ArrestoRichiesto as e:
                # il tentativo non e' piu' nostro: niente result, il job e' gia' di un altro tentativo
                log.warning("job %d interrotto: %s", job.job_id, e)
                self.battito = None
                return
            except Exception as e:
                log.exception("job %d errore: %s", job.job_id, e)
                ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])
            perso = b.arresto.is_set()
        self.battito = None
        if perso:
            # il lease e' di un altro tentativo: riportare adesso sarebbe scrivere sopra al suo lavoro
            log.warning("job %d: lease perso durante l'analisi, risultato non riportato", job.job_id)
            return

        riporta_risultato(self.api, job.job_id, ris.model_dump(mode="json"), self.worker_id, job.lease_token, log)
        log.info("job %d completato in %.2fs -> %s", job.job_id, time.time() - t0, ris.esito)

    def analizza(self, job: Job, p: PayloadAnalizzaAllegato) -> RisultatoRichiesta:
        """Scarica il contenuto dal server, lo analizza, lo cancella (7C.1, P0).

        Il payload NON dice dove sta il file: dice quale allegato e' e che sha256 deve avere. I byte
        si prendono con GET /api/v1/allegati/{id}/contenuto dentro questo tentativo, si scrivono in
        `tmp\\<job>\\` e si confrontano con lo sha256 del payload — un file arrivato a meta' e' un
        file diverso, e si ripete. Un `path_staging` in un job vecchio ancora in coda si ignora:
        un percorso sul disco di un altro PC non dice niente a questo.
        """
        locale = os.path.join(self.staging, "tmp", str(job.job_id))
        ext = Path(p.nome_file).suffix.lower()
        dest = os.path.join(locale, "contenuto" + (ext if re.fullmatch(r"\.[a-z0-9]{1,10}", ext) else ""))
        try:
            try:
                self.segna(f"scarico {p.nome_file} ({p.bytes} byte)")
                n, sha = self.api.scarica_contenuto(str(p.allegato_id), job.job_id, job.lease_token,
                                                    self.worker_id, dest)
            except ErroreHTTP as e:
                if e.tentativo_non_valido:
                    raise ArrestoRichiesto(f"download rifiutato: {e.corpo[:200]}") from e
                if e.stato in (404, 410, 422):
                    # il contenuto non c'e' piu' nella cache del server, o il job non e' l'analisi di
                    # questo allegato: ritentare darebbe lo stesso esito
                    log.error("job %d contenuto non disponibile: %s", job.job_id, e)
                    return RisultatoRichiesta(esito="errore", definitivo=True,
                                              errore=f"contenuto non disponibile sul server: usa Riscarica ({e.corpo[:300]})")
                raise                                   # 5xx: il job fallisce senza «definitivo» e viene ritentato
            atteso = (p.sha256 or "").lower()
            if atteso and sha != atteso:
                raise ContenutoIncompleto(f"sha256 del contenuto scaricato {sha[:12]}… diverso da quello atteso {atteso[:12]}…")
            if p.bytes and n != p.bytes:
                raise ContenutoIncompleto(f"scaricati {n} byte, attesi {p.bytes}")
            self.controlla()
            self.segna(f"analisi di {p.nome_file}")
            esito = analizza_file(dest, p.nome_file, self.ocr)
        finally:
            rimuovi_con_pazienza(dest)
            try:
                if os.path.isdir(locale):
                    os.rmdir(locale)
            except OSError as e:
                log.warning("cartella temporanea non rimossa: %s (%s)", locale, e)
        ris_analisi = RisultatoAnalisi(
            allegato_id=p.allegato_id,
            tipo_proposto=esito["tipo_proposto"],
            codice=esito.get("codice", ""),
            rev=esito.get("rev", ""),
            confidenza=esito.get("confidenza", 50),
            fonte=esito["fonte"],  # ogni ramo la scrive: il default «cartiglio» non si usa piu' (A5.13.8)
            dettagli=esito.get("dettagli", {}),
            versione_analizzatore=p.versione_analizzatore,
            hash_configurazione=p.hash_configurazione,
        )
        return RisultatoRichiesta(esito="ok", dati=ris_analisi.model_dump(mode="json"))


def main():
    p = argparse.ArgumentParser(description="Worker Analisi per Cockpit RFQ")
    p.add_argument("--config", default=os.path.join(os.path.dirname(__file__), "worker.toml"), help="percorso file di configurazione")
    p.add_argument("--una-volta", action="store_true", help="esegui un solo job ed esci")
    p.add_argument("--debug", action="store_true")
    args = p.parse_args()

    cfg = carica_config(args.config, sezione="analisi")
    configura_log(args.debug, cfg, "worker_analisi")
    w = WorkerAnalisi(cfg)
    w.esegui_per_sempre(una_volta=args.una_volta)


if __name__ == "__main__":
    main()
