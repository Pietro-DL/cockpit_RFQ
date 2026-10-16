"""I nomi delle colonne di una tabella e il loro RUOLO tipico (GP-3, direttiva 21 §5.1).

Un modulo di vocabolario, senza regole di cliente: dice che «QUANTITY», «Menge», «Q.TÀ» sono di solito la colonna
della quantita', che «Bezeichnung» e «Name» sono di solito la descrizione, e cosi' via. Il ruolo e' un AIUTO di
lettura per chi viene dopo, non un'interpretazione: l'intestazione resta scritta com'e' nel fatto, e che cosa sia
davvero una colonna («Material» e' il codice nella richiesta di un ERP e il materiale grezzo in una distinta) lo dice
il server, con il profilo del cliente. Per questo un ruolo ambiguo non si sceglie: «MATERIAL» e' `materiale` e basta.

Lo usano le tabelle dei PDF (tabelle_testo), i fogli (foglio_dati) e le righe d'elenco dei DWG (dwg_testo).

Le colonne «personali» (chi ha disegnato, controllato, approvato, la firma, la data della firma) non portano il
valore nel fatto: nei cartigli e nelle tabelle delle revisioni ci sono nomi di persona, e i fatti restano archiviati
per anni (come MetadatiPDF senza l'autore).
"""
from __future__ import annotations

import re
import unicodedata

# I ruoli possibili. "" = nessun ruolo riconosciuto (l'intestazione resta, il ruolo no).
POSIZIONE = "posizione"
LIVELLO = "livello"
CODICE = "codice"
DESCRIZIONE = "descrizione"
QUANTITA = "quantita"
UNITA = "unita"
MATERIALE = "materiale"
REVISIONE = "revisione"
DOCUMENTO = "documento"
NOTA = "nota"
RUOLI = (POSIZIONE, LIVELLO, CODICE, DESCRIZIONE, QUANTITA, UNITA, MATERIALE, REVISIONE, DOCUMENTO, NOTA)

# Frasi intere (dopo piega()): valgono prima delle parole sciolte.
_FRASI = {
    "PART NUMBER": CODICE, "PART NO.": CODICE, "PART NO": CODICE, "PART N.": CODICE, "PART N°": CODICE,
    "PART NR.": CODICE, "PART NR": CODICE, "P/N": CODICE, "COMPONENT NUMBER": CODICE, "ARTICLE NUMBER": CODICE,
    "CODICE ARTICOLO": CODICE, "N. CODICE": CODICE, "MATERIAL NUMBER": CODICE,
    "ITEM NO.": POSIZIONE, "ITEM NO": POSIZIONE, "ITEM N.": POSIZIONE, "POS. NO.": POSIZIONE, "LFD. NR.": POSIZIONE,
    "N. POS.": POSIZIONE, "N° POS.": POSIZIONE,
    "BASIC MATERIAL": MATERIALE, "RAW MATERIAL": MATERIALE,
    "DOCUMENT NUMBER": DOCUMENTO, "DRAWING NUMBER": DOCUMENTO, "DRAWING NO.": DOCUMENTO, "DWG NO.": DOCUMENTO,
    "DWG NO": DOCUMENTO, "N. DISEGNO": DOCUMENTO, "NUMERO DISEGNO": DOCUMENTO,
    "UNIT OF MEASURE": UNITA, "U.M.": UNITA, "U. M.": UNITA,
    "N. PEZZI": QUANTITA, "NR. PEZZI": QUANTITA,
}

# Parole sciolte (dopo piega(), senza i due punti finali). Una parola con il punto finale vale anche senza.
_PAROLE = {
    POSIZIONE: {"POS", "POSIZIONE", "POSITION", "ITEM", "INDICE", "INDEX", "RIF", "N", "N°", "NR", "NO", "POSITIONSNUMMER"},
    LIVELLO: {"LEVEL", "LIVELLO", "STUFE", "LVL"},
    CODICE: {"CODICE", "COD", "CODE", "ARTICOLO", "ARTICLE", "PN", "KOMPONENTENNUMMER", "SACHNUMMER", "TEILENUMMER",
             "MATERIALNUMMER", "ARTIKELNUMMER", "IDENTNUMMER", "IDENT-NR", "MATNR", "COMPONENT"},
    DESCRIZIONE: {"DESCRIPTION", "DESCRIZIONE", "DENOMINAZIONE", "BEZEICHNUNG", "BENENNUNG", "NAME", "KURZTEXT",
                  "DESIGNATION", "DESIGNAZIONE", "TITLE", "TITOLO"},
    QUANTITA: {"QUANTITY", "QUANTITA", "QUANTITA'", "QTY", "Q.TY", "Q.TA", "Q.TA'", "QTA", "MENGE", "ANZAHL", "PZ",
               "PEZZI", "STK", "STUCK", "QUANT", "Q"},
    UNITA: {"UNIT", "UNITA", "UN", "UM", "UOM", "ME", "EINHEIT", "MENGENEINHEIT"},
    MATERIALE: {"MATERIAL", "MATERIALE", "WERKSTOFF", "WERKSTOFFE"},
    REVISIONE: {"REV", "REVISIONE", "REVISION", "VERSION", "VERSIONE", "VS", "AUSGABE", "ESPONENTE"},
    DOCUMENTO: {"DOCUMENTNUMBER", "DOKUMENTENNUMMER", "DOCUMENT", "DOKUMENT", "DISEGNO", "DRAWING", "DWG", "FILE",
                "ZEICHNUNG", "ZEICHNUNGSNUMMER"},
    NOTA: {"NOTE", "NOTES", "NOTA", "BEMERKUNG", "ANMERKUNG", "REMARK", "REMARKS", "OSSERVAZIONI"},
}
_RUOLO_DI_PAROLA = {p: r for r, ps in _PAROLE.items() for p in ps}

# Le colonne personali: il loro valore non viaggia (docstring del modulo).
_PERSONALI = {"DRAWN", "DISEGNATO", "DIS", "CHECKED", "CONTROLLATO", "CONTR", "APPROVED", "APPROVATO", "APPR", "GEZ",
              "GEZEICHNET", "GEPR", "GEPRUFT", "FREIGABE", "FREIGEGEBEN", "FIRMA", "SIGNATURE", "SIGN", "VISTO",
              "AUTORE", "AUTHOR", "BEARBEITER", "ERSTELLT", "ERSTELLER", "NOME", "COGNOME", "VERIFICATO", "REDATTO",
              "DESIGNER", "ENGINEER", "PROGETTISTA"}

_RE_SPAZI = re.compile(r"\s+")


def piega(testo: str) -> str:
    """Il testo per i confronti: in maiuscolo, senza segni diacritici (STÜCKLISTE -> STUCKLISTE, QUANTITÀ ->
    QUANTITA), ß -> SS, gli spazi ripetuti in uno. Solo per confrontare: nel fatto resta il testo originale."""
    s = unicodedata.normalize("NFKD", testo.replace("ß", "ss").replace("ẞ", "SS"))
    s = "".join(c for c in s if not unicodedata.combining(c))
    return _RE_SPAZI.sub(" ", s.upper()).strip()


def _parole(testo: str) -> list[str]:
    out = []
    for w in piega(testo).replace("/", " / ").split():
        w = w.rstrip(":")
        if w and w != "/":
            out.append(w)
    return out


def _ruolo_parola(w: str) -> str:
    r = _RUOLO_DI_PAROLA.get(w)
    if r is None and w.endswith("."):
        r = _RUOLO_DI_PAROLA.get(w.rstrip("."))
    if r is None and w.endswith("'"):
        r = _RUOLO_DI_PAROLA.get(w.rstrip("'"))
    return r or ""


# Le sigle brevi della posizione valgono solo da sole: in «MOD. N°» (il numero della modifica, in una tabella delle
# revisioni) o in «N. DISEGNO» il «N°» non e' la colonna della posizione.
_SOLO_DA_SOLE = {"N", "N°", "NR", "NO", "RIF", "Q"}


def ruolo_colonna(testo: str) -> str:
    """Il ruolo tipico di un'intestazione di colonna, o "" se non si riconosce. Prima la frase intera, poi la
    prima parola con un ruolo (da sinistra): «BASIC MATERIAL» -> materiale, «SUPPLIER PN» -> codice. Le sigle brevi
    («N°», «NR», «RIF») contano solo come intestazione intera."""
    f = piega(testo).rstrip(":").strip()
    if f in _FRASI:
        return _FRASI[f]
    ws = _parole(testo)
    for w in ws:
        if len(ws) > 1 and w.rstrip(".") in _SOLO_DA_SOLE:
            continue
        r = _ruolo_parola(w)
        if r:
            return r
    return ""


def ruolo_di_parola(testo: str) -> str:
    """Il ruolo di un testo che e' UNA parola sola (per separare due intestazioni attaccate: «quantity un»)."""
    ws = _parole(testo)
    if len(ws) != 1:
        return ""
    f = piega(testo).rstrip(":")
    return _FRASI.get(f) or _ruolo_parola(ws[0])


def colonna_personale(testo: str) -> bool:
    """L'intestazione e' quella di una colonna con nomi di persona (disegnato, controllato, firma...)."""
    return any(w.rstrip(".") in _PERSONALI for w in _parole(testo))


def ha_cifre(testo: str) -> bool:
    return any(c.isdigit() for c in testo)
