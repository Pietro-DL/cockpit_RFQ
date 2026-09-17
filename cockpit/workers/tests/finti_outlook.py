"""La cartella Outlook finta con cui si provano il filtro della finestra e il confine COM.

Non è un mock che risponde quello che gli si dice: INTERPRETA il filtro DASL come lo interpreterebbe
Outlook, cioè in UTC, e ordina davvero. È questo a renderla capace di bocciare l'errore che conta —
se il codice scrivesse l'ora locale dentro il filtro, la cartella finta restituirebbe l'insieme
sbagliato e il confronto se ne accorgerebbe.

Gli elementi anomali non sono casi di scuola: sono i tre modi in cui un elemento vero rifiuta di
farsi leggere, e ognuno si presenta in modo diverso.

  ElementoNonMail     ha `Class`, ma non ha `ReceivedTime`. Con il late binding di pywin32 leggere
                      una proprietà che l'oggetto non ha alza `AttributeError`, e il nome nel
                      messaggio è quello del METODO che ha prodotto l'oggetto: «GetNext.ReceivedTime»
                      è la riga esatta che ha fermato il sync della Posta inviata vera;
  ElementoSordo       ha le proprietà, ma COM alza `com_error` quando le si chiede (elemento
                      eliminato mentre lo si legge, store che non risponde);
  ElementoSenzaClasse è una mail normale che però non dice che cosa è. Non va scartata per il
                      dubbio: `Class` non letta vuol dire «non lo so», non «non è una mail».
"""
from __future__ import annotations

import re
from datetime import datetime, timezone

import pywintypes

OL_MAIL = 43
OL_REPORT = 46          # rapporto di consegna/mancata consegna: il non-mail più comune in Posta inviata


def com_error(messaggio: str = "elemento non disponibile"):
    """Un `pywintypes.com_error` come quello che alza COM, costruito a mano."""
    return pywintypes.com_error(-2147352567, "Eccezione.", (0, None, messaggio, None, 0, -2147024894), None)


def quando(filtro: str) -> tuple[datetime | None, datetime | None]:
    """Legge la finestra dal filtro DASL INTERPRETANDOLA IN UTC, come fa Outlook."""
    ge = re.search(r">=\s*'([^']+)'", filtro)
    le = re.search(r"<=\s*'([^']+)'", filtro)

    def leggi(m):
        if m is None:
            return None
        return datetime.strptime(m.group(1), "%Y-%m-%d %H:%M").replace(tzinfo=timezone.utc)

    return leggi(ge), leggi(le)


class ElementoFinto:
    def __init__(self, entry_id: str, ricevuto: datetime, classe: int = OL_MAIL):
        self.EntryID = entry_id
        self.ReceivedTime = ricevuto
        self.Class = classe

    @property
    def utc(self) -> datetime:
        return self.ReceivedTime.astimezone(timezone.utc)


class ElementoNonMail:
    """Un elemento che non è una mail: `Class` si legge, tutto il resto non esiste.

    `__getattr__` imita il late binding di pywin32: qualunque proprietà non prevista alza
    `AttributeError` con il nome del metodo che ha prodotto l'oggetto davanti. Fra le proprietà che
    non esistono c'è `ReceivedTime`, ed è tutto il difetto.
    """

    def __init__(self, entry_id: str, classe: int = OL_REPORT, ricevuto: datetime | None = None):
        self.EntryID = entry_id
        self.Class = classe
        self._utc = ricevuto          # serve solo all'ordinamento della collezione finta

    def __getattr__(self, nome):
        raise AttributeError("GetNext.%s" % nome)

    @property
    def utc(self) -> datetime:
        return self._utc if self._utc is not None else datetime(1970, 1, 1, tzinfo=timezone.utc)


class ElementoSordo:
    """Ha le proprietà, ma COM non le dà: è l'elemento eliminato mentre lo si stava leggendo."""

    def __init__(self, entry_id: str, ricevuto: datetime, classe: int = OL_MAIL):
        self.EntryID = entry_id
        self.Class = classe
        self._utc = ricevuto

    @property
    def ReceivedTime(self):
        raise com_error("ReceivedTime non disponibile")

    @property
    def utc(self) -> datetime:
        return self._utc


# Il tag MAPI del Message-ID, ripetuto qui per non importare outlook_com dentro i finti: questo
# modulo deve poter essere letto anche da chi sta guardando solo i test.
PR_INTERNET_MESSAGE_ID = "http://schemas.microsoft.com/mapi/proptag/0x1035001F"


class _Conteggio:
    def __init__(self, n: int = 0):
        self.Count = n


class _Proprieta:
    """Il PropertyAccessor di Outlook: conosce i tag che gli sono stati dati, sugli altri alza."""

    def __init__(self, valori: dict):
        self._valori = valori

    def GetProperty(self, tag):
        if tag in self._valori:
            return self._valori[tag]
        raise com_error("proprieta %s non presente" % tag)


class MailFinta(ElementoFinto):
    """Una mail completa quanto basta perche `_converti` ne faccia un MessaggioIn.

    Serve dove il test deve arrivare fino in fondo alla lettura, non solo alla scansione: una
    ElementoFinto senza Subject diventerebbe uno scarto di conversione, e il test proverebbe il
    contrario di quello che vuole provare.
    """

    def __init__(self, entry_id: str, ricevuto: datetime, oggetto: str = "prova",
                 mittente: str = "cliente@esempio.it", message_id: str = ""):
        super().__init__(entry_id, ricevuto)
        self.Subject = oggetto
        self.SenderName = "Cliente"
        self.SenderEmailType = "SMTP"
        self.SenderEmailAddress = mittente
        self.Sender = None
        self.ConversationID = "conv-" + entry_id
        self.ConversationIndex = ""
        self.SentOn = ricevuto
        self.Body = "corpo del messaggio"
        self.HTMLBody = ""
        self.Categories = ""
        self.Importance = 1
        self.UnRead = True
        self.FlagStatus = 0
        self.Recipients = []
        self.Attachments = _Conteggio(0)
        self.PropertyAccessor = _Proprieta({PR_INTERNET_MESSAGE_ID: message_id or ("<%s@finto>" % entry_id)})


class ElementoSenzaClasse(ElementoFinto):
    """Una mail vera che non dice che cosa è: `Class` alza, `ReceivedTime` risponde."""

    def __init__(self, entry_id: str, ricevuto: datetime):
        super().__init__(entry_id, ricevuto)
        del self.Class

    def __getattr__(self, nome):
        if nome == "Class":
            raise AttributeError("GetNext.Class")
        raise AttributeError(nome)


class ItemsFinti:
    """La collezione Items di Outlook, ridotta a ciò che il codice usa: Sort, Restrict, GetFirst/GetNext.

    `rompe_dopo` interrompe l'enumerazione a metà come fa una collezione che Outlook sta
    ricostruendo: GetNext alza invece di restituire l'elemento successivo.
    """

    def __init__(self, elementi, perde: set | None = None, rompe_dopo: int | None = None):
        self._elenco = list(elementi)
        self._perde = perde or set()
        self._rompe_dopo = rompe_dopo
        self._i = 0
        self.ordinata = None
        self.filtro = None

    def Sort(self, campo, decrescente):
        assert campo == "[ReceivedTime]", campo
        self._elenco.sort(key=lambda e: e.utc, reverse=bool(decrescente))
        self.ordinata = "decrescente" if decrescente else "crescente"

    def Restrict(self, filtro):
        dal, al = quando(filtro)
        dentro = [e for e in self._elenco
                  if (dal is None or e.utc >= dal) and (al is None or e.utc <= al)
                  and e.EntryID not in self._perde]
        fuori = ItemsFinti(dentro)
        fuori.filtro = filtro
        return fuori

    def GetFirst(self):
        self._i = 0
        return self.GetNext()

    def GetNext(self):
        if self._rompe_dopo is not None and self._i >= self._rompe_dopo:
            raise com_error("la collezione non è più valida")
        if self._i >= len(self._elenco):
            return None
        self._i += 1
        return self._elenco[self._i - 1]


class CartellaFinta:
    def __init__(self, elementi, perde: set | None = None, nome="Posta in arrivo", rompe_dopo: int | None = None,
                 store_id="store-finto", entry_id="cartella-finta"):
        self._elementi = list(elementi)
        self._perde = perde or set()
        self._rompe_dopo = rompe_dopo
        self.Name = nome
        self.StoreID = store_id
        self.EntryID = entry_id
        self.ultimo_filtro = None
        # quante volte qualcuno ha chiesto la collezione: e la misura del COSTO, cioe cio che il
        # self-test memorizzato deve far scendere
        self.aperture = 0

    @property
    def Items(self):
        # una collezione NUOVA a ogni accesso, come fa Outlook: altrimenti il Sort di una lettura
        # resterebbe addosso alla successiva e il test proverebbe qualcosa che non succede
        self.aperture += 1
        i = ItemsFinti(self._elementi, self._perde, self._rompe_dopo)
        self._ultima = i
        return i
