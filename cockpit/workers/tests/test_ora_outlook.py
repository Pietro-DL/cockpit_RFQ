"""L1/L2 — l'ora che Outlook consegna non è l'ora che dichiara (correzione del 16/09/2026).

Il difetto. `ReceivedTime` e `SentOn` dell'Object Model sono VT_DATE: un numero senza fuso che porta
l'ora **locale** del PC. pywin32 lo converte in un `pywintypes.datetime` e gli attacca `tzinfo` UTC,
perché è la convenzione della sua conversione — non perché quel valore sia in UTC. Ne esce un oggetto
che dichiara UTC e porta i numeri di Roma, e chi lo prende in parola sposta tutto avanti di due ore.

Come si è visto. Log del 16/09/2026, 08:52: i cursori di sync (`ultimo_received`) erano **due ore nel
futuro** rispetto all'orologio del server, e ogni `ricevuto_il` in database con loro. Un cursore nel
futuro non dà errore: apre una finestra che comincia fra due ore, quindi il sync successivo non legge
più niente finché quelle due ore non sono passate. Silenzioso, e verde da tutte le parti.

Qui il caso si riproduce con un `pywintypes.datetime` VERO — non un oggetto che gli somiglia —
costruito come lo costruisce pywin32: i campi dell'ora locale, il `tzinfo` a UTC. È l'unico modo di
provare la correzione senza Outlook: il valore che arriva è esattamente quello, byte per byte.

NON PROVANO che Outlook consegni davvero VT_DATE in ora locale su quel profilo: questo si vede solo
sul profilo vero, ed è la riga del registro reale (l'ora letta in Outlook accanto a quella in
database). Provano che, ricevuto un valore fatto così, il worker ne ricava l'istante giusto.
"""
from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

pywintypes = pytest.importorskip("pywintypes", reason="serve pywin32 (solo su Windows)")
outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

_utc = outlook_com._utc

ROMA_LEGALE = timezone(timedelta(hours=2))   # da fine marzo a fine ottobre
ROMA_SOLARE = timezone(timedelta(hours=1))   # il resto dell'anno


def ora_di_outlook(anno, mese, giorno, ore, minuti, secondi=0):
    """Una data come la consegna pywin32: i numeri dell'ora locale, l'etichetta UTC.

    `pywintypes.TimeType` è la classe vera (`pywintypes.datetime`), quella che il worker riceve da
    COM. Costruirla a mano è ciò che rende questo test capace di bocciare la conversione sbagliata:
    un `datetime` normale con `tzinfo=utc` sarebbe un istante UTC legittimo, e non avrebbe niente da
    correggere.
    """
    return pywintypes.TimeType(anno, mese, giorno, ore, minuti, secondi, tzinfo=timezone.utc)


# ---------------------------------------------------------------- la conversione

def test_una_data_di_outlook_e_ora_locale_travestita_da_utc():
    # Outlook mostra «16/09/2026 10:52» nella colonna Ricevuto, su un PC a Roma.
    ricevuto = ora_di_outlook(2026, 9, 16, 10, 52)
    assert _utc(ricevuto, ROMA_LEGALE) == datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)
    # e soprattutto NON le 10:52 UTC, che sono le 12:52 di Roma: il difetto del 16/09.
    assert _utc(ricevuto, ROMA_LEGALE) != datetime(2026, 9, 16, 10, 52, tzinfo=timezone.utc)


def test_il_cursore_non_finisce_piu_nel_futuro():
    """Il sintomo del log, riprodotto: `ultimo_received` avanti di due ore sull'orologio del server."""
    adesso = datetime.now()
    ricevuto = ora_di_outlook(adesso.year, adesso.month, adesso.day, adesso.hour, adesso.minute)
    convertito = _utc(ricevuto)          # senza `fuso_locale`: quello di questo PC, come in produzione
    scarto = convertito - datetime.now(timezone.utc)
    assert scarto < timedelta(minutes=2), (
        "un messaggio appena ricevuto risulta nel futuro di %s: è il cursore che si blocca" % scarto)


def test_l_ora_legale_la_decide_la_data_non_l_oggi():
    """Gennaio è +01:00 e luglio +02:00: l'offset è quello del giorno di QUELLA data."""
    assert _utc(ora_di_outlook(2026, 1, 15, 9, 30), ROMA_SOLARE) == datetime(2026, 1, 15, 8, 30, tzinfo=timezone.utc)
    assert _utc(ora_di_outlook(2026, 7, 15, 9, 30), ROMA_LEGALE) == datetime(2026, 7, 15, 7, 30, tzinfo=timezone.utc)
    # senza fuso dichiarato decide il PC, e il conto deve tornare comunque: riletta in locale, la
    # data convertita mostra l'orologio che si vede in Outlook.
    for mese in (1, 7):
        d = ora_di_outlook(2026, mese, 15, 9, 30)
        assert _utc(d).astimezone().strftime("%Y-%m-%d %H:%M") == "2026-%02d-15 09:30" % mese


def test_un_datetime_con_un_fuso_vero_non_viene_spostato():
    """Ciò che non viene da COM dichiara il fuso giusto: si converte com'è, senza reinterpretarlo."""
    vero = datetime(2026, 9, 16, 10, 52, tzinfo=ROMA_LEGALE)
    assert _utc(vero) == datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)
    gia_utc = datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)
    assert _utc(gia_utc) == gia_utc


# ---------------------------------------------------------------- L2: il messaggio che parte per il server

class AllegatiFinti:
    Count = 0


class ElementoFinto:
    """Un MailItem ridotto a ciò che `_converti` legge davvero."""

    Class = 43
    EntryID = "ENTRY-1"
    Subject = "Richiesta di offerta"
    ConversationID = "CONV-1"
    ConversationIndex = "IDX"
    Body = "testo"
    HTMLBody = "<p>testo</p>"
    Categories = ""
    SenderName = "Buyer"
    SenderEmailType = "SMTP"
    SenderEmailAddress = "buyer@cliente.example"
    Importance = 1
    UnRead = True
    FlagStatus = 0
    Recipients: list = []
    Attachments = AllegatiFinti()

    def __init__(self, ricevuto, inviato=None):
        self.ReceivedTime = ricevuto
        self.SentOn = inviato if inviato is not None else ricevuto


def converti(item, e_inviata=False, fuso=ROMA_LEGALE, monkeypatch=None):
    """`_converti` con il PC fermo su un fuso noto, e senza costruire un `Outlook` vero (che aprirebbe COM)."""
    ol = outlook_com.Outlook.__new__(outlook_com.Outlook)
    ol.indirizzi_propri = {"noi@azienda.example"}
    vera = outlook_com._utc          # va presa PRIMA, o la lambda richiamerebbe se stessa
    monkeypatch.setattr(outlook_com, "_utc", lambda d: vera(d, fuso))
    return ol._converti(item, "STORE-1", "Posta in arrivo", e_inviata)


def test_il_messaggio_porta_l_ora_giusta_al_server(monkeypatch):
    m = converti(ElementoFinto(ora_di_outlook(2026, 9, 16, 10, 52)), monkeypatch=monkeypatch)
    # `ricevuto_il` è il valore su cui avanza il cursore (W2): è quello che andava corretto.
    assert m.ricevuto_il == datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)
    assert m.data_evento == datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)


def test_anche_la_posta_inviata_usa_l_ora_giusta(monkeypatch):
    """In uscita `data_evento` è SentOn, e arriva dallo stesso VT_DATE: va corretto anche lì."""
    it = ElementoFinto(ora_di_outlook(2026, 9, 16, 10, 52), inviato=ora_di_outlook(2026, 9, 16, 10, 40))
    m = converti(it, e_inviata=True, monkeypatch=monkeypatch)
    assert m.direzione == "uscita"
    assert m.data_evento == datetime(2026, 9, 16, 8, 40, tzinfo=timezone.utc)
    assert m.ricevuto_il == datetime(2026, 9, 16, 8, 52, tzinfo=timezone.utc)
