"""Adattatore Outlook classico via COM (pywin32).

Tutto ciò che tocca Outlook sta qui: lettura cartelle con tutti i metadati MAPI, salvataggio allegati
in staging, creazione bozze/risposte/inoltri, apertura elementi, spostamento, letto/non letto.
Non decide nulla e non scrive sul NAS. Va eseguito in un thread solo (COM STA): Outlook non tollera
accessi concorrenti, quindi il worker serializza tutte le chiamate.

Proprietà MAPI verificate sul profilo Exchange (13/09/2026): Message-ID 0x1035001F, SMTP mittente
0x5D01001F/0x5D02001F, SMTP destinatari 0x39FE001F, header 0x007D001F, Content-ID 0x3712001F,
hidden 0x7FFE000B. StoreID ~600 caratteri, EntryID 140.
"""
from __future__ import annotations

import email.parser
import hashlib
import json
import logging
import os
import re
import socket
import time
from datetime import datetime, timedelta, timezone
from typing import Iterator

import pythoncom
import pywintypes
import win32com.client

from contratti import AllegatoIn, Destinatario, ElementoSaltato, MessaggioIn, PayloadCreaBozza

log = logging.getLogger("outlook")

P = "http://schemas.microsoft.com/mapi/proptag/"
PR_INTERNET_MESSAGE_ID = P + "0x1035001F"
PR_TRANSPORT_HEADERS = P + "0x007D001F"
PR_SENDER_SMTP = P + "0x5D01001F"
PR_SENT_REPR_SMTP = P + "0x5D02001F"
PR_SMTP_ADDRESS = P + "0x39FE001F"
PR_ATTACH_CONTENT_ID = P + "0x3712001F"
PR_ATTACHMENT_HIDDEN = P + "0x7FFE000B"
PR_ATTACH_MIME_TAG = P + "0x370E001F"

OL_MAIL = 43
OL_FOLDER = {"inbox": 6, "posta in arrivo": 6, "sent items": 5, "posta inviata": 5, "drafts": 16, "bozze": 16,
             "deleted items": 3, "posta eliminata": 3, "junk": 23, "posta indesiderata": 23}
OL_INBOX = 6
PR_SMTP_ADDRESS_STORE = P + "0x39FE001F"   # sullo store: SMTP del proprietario, quando Outlook lo espone
OL_ATT_BYVALUE, OL_ATT_BYREF, OL_ATT_EMBEDDED, OL_ATT_OLE = 1, 4, 5, 6
OL_TO, OL_CC, OL_BCC = 1, 2, 3
MAPI_E_NOT_FOUND = -2147221233  # 0x8004010F

_NOME_VIETATI = re.compile(r'[<>:"/\\|?*\x00-\x1f]')


class ErroreDefinitivo(Exception):
    """Errore che non ha senso ritentare (elemento eliminato, allegato non salvabile)."""


# Il tipo che pywin32 restituisce per ogni data che arriva da COM (`pywintypes.datetime`). Se una
# versione di pywin32 non lo esportasse, la tupla vuota rende ogni `isinstance` falso: si torna al
# comportamento di prima invece di fermare il worker.
_TIPO_ORA_COM = getattr(pywintypes, "TimeType", ())


def _utc(d, fuso_locale=None) -> datetime:
    """Una data letta da Outlook → datetime UTC **vero**.

    Difetto del 16/09/2026, e vale la pena scriverlo per esteso perché non si vede guardando il
    codice. `ReceivedTime` e `SentOn` dell'Object Model sono VT_DATE: un numero SENZA fuso che porta
    l'ora **locale** del PC. pywin32 lo converte in un `pywintypes.datetime` e gli attacca `tzinfo`
    **UTC**, perché è la convenzione della sua conversione, non perché quel valore sia in UTC. Ne
    esce un oggetto che dichiara UTC e porta i numeri dell'ora locale, e `timestamp()` lo prende in
    parola: le 10:52 di Roma diventano le 10:52 UTC, cioè le 12:52 di Roma. Due ore nel futuro.

    Non dà errore da nessuna parte. Si è visto nei cursori di sync — `ultimo_received` avanti di due
    ore rispetto all'orologio del server, quindi una finestra che comincia nel futuro e un sync che
    non legge più niente finché le due ore non sono passate — e in ogni `ricevuto_il` scritto in
    database (log del 16/09, 08:52).

    La correzione è prendere i CAMPI e leggerli nel fuso del PC, che è ciò che sono. Per i valori che
    non vengono da COM (i test, un `datetime` costruito a mano) resta la conversione di prima: lì il
    fuso dichiarato è quello vero e `timestamp()` è affidabile.

    `fuso_locale` serve ai test per non dipendere dal fuso della macchina che li esegue; in
    produzione è sempre None, cioè «il fuso di questo PC, con l'ora legale del giorno di quella
    data».
    """
    if d is None:
        return datetime.now(timezone.utc)
    if isinstance(d, _TIPO_ORA_COM):
        locale = datetime(d.year, d.month, d.day, d.hour, d.minute, d.second)
        if fuso_locale is not None:
            locale = locale.replace(tzinfo=fuso_locale)
        else:
            locale = locale.astimezone()   # naive → fuso di questo PC, ora legale compresa
        return locale.astimezone(timezone.utc)
    return datetime.fromtimestamp(d.timestamp(), tz=timezone.utc).replace(microsecond=0)


def _prop(o, tag, default=""):
    try:
        v = o.PropertyAccessor.GetProperty(tag)
        return v if v is not None else default
    except pywintypes.com_error:
        return default
    except Exception:  # noqa: BLE001 - qualunque anomalia COM su una proprietà non deve fermare il sync
        return default


OL_TEXT = 1  # OlUserPropertyType.olText


def marcatori_di(it) -> dict[str, str]:
    """Le UserProperties `Cockpit*` dell'elemento (blocco 7B). Le ha scritte il Cockpit creando la
    bozza; se la mail e' partita, sono ancora li'. Qualunque anomalia COM = nessun marcatore: un
    marcatore non letto costa una conferma a mano, un sync fermato costa la posta."""
    out: dict[str, str] = {}
    try:
        props = it.UserProperties
        for i in range(1, int(props.Count) + 1):
            p = props.Item(i)
            nome = str(p.Name or "")
            if nome.startswith("Cockpit"):
                v = p.Value
                if v is not None and str(v) != "":
                    out[nome] = str(v)
    except Exception:  # noqa: BLE001 - nessun marcatore vale piu' di un sync fermato
        return {}
    return out


def scrivi_marcatori(item, marcatori: dict[str, str]) -> None:
    """Scrive (o aggiorna) le UserProperties `Cockpit*` su una bozza. Un marcatore che non si riesce
    a scrivere non ferma la bozza: la mail parte lo stesso e si lega a mano."""
    for nome, valore in marcatori.items():
        try:
            props = item.UserProperties
            p = props.Find(nome)
            if p is None:
                p = props.Add(nome, OL_TEXT, False)
            p.Value = valore
        except Exception as e:  # noqa: BLE001
            log.warning("marcatore %s non scritto sulla bozza: %s", nome, e)


# ---------------------------------------------------------------- il confine COM (3R, blocco 1)
#
# Regola unica: prima di leggere una proprieta di MailItem si guarda `Class`, e OGNI proprieta COM
# si legge protetta. Non e prudenza generica, e il difetto visto sulla Posta inviata vera:
#
#     AttributeError: GetNext.ReceivedTime
#
# Con il late binding di pywin32 una proprieta che l oggetto NON HA non alza `com_error`: alza
# `AttributeError`, e il nome nel messaggio e quello del METODO che ha prodotto l oggetto
# (`GetNext`), non quello dell elemento - per questo la riga di errore non dice nemmeno quale
# elemento fosse. Un solo elemento non-mail in una cartella (un rapporto di consegna, un
# appuntamento, un elemento di Sync Issues) bastava a far cadere la scansione lineare, quindi il
# self-test di Restrict che la usa, quindi il sync di TUTTA la cartella.
#
# Da qui in avanti: un elemento illeggibile si salta e si conta; una cartella non muore per colpa
# di un suo elemento.
ERRORI_ELEMENTO = (pywintypes.com_error, AttributeError)


class LetturaIncompleta(Exception):
    """L enumerazione della cartella si e rotta a meta.

    Non e un elemento saltato: e un pezzo di finestra che nessuno ha guardato. La scansione lineare
    va dal piu recente al piu vecchio, quindi cio che si e raccolto e la parte NUOVA: consegnarla
    farebbe avanzare il cursore oltre messaggi mai visti, e nessuno andrebbe piu a cercarli. La
    cartella fallisce e si rilegge tutta al giro dopo, che costa una rilettura e non una perdita.
    """


def classe_di(it) -> int | None:
    """`OlObjectClass` dell elemento (43 = MailItem), None se non si riesce a leggerla.

    None NON vuol dire "non e una mail": vuol dire "non lo so", e chi chiama deve provare a
    trattarlo come tale invece di scartarlo per un dubbio.
    """
    try:
        return int(it.Class)
    except ERRORI_ELEMENTO:
        return None
    except (TypeError, ValueError):
        return None


def entryid_di(it) -> str:
    """L EntryID, "" se non si riesce a leggerlo. Senza, l elemento non e nemmeno rileggibile: il
    server rifiuta uno scarto senza entry_id, quindi qui resta solo la riga di log."""
    try:
        return str(it.EntryID or "")
    except ERRORI_ELEMENTO:
        return ""


def oggetto_di(it) -> str:
    try:
        return str(it.Subject or "")
    except ERRORI_ELEMENTO:
        return ""


def ricevuta_di(it, fuso_locale=None) -> datetime | None:
    """`ReceivedTime` in UTC vero; None se l elemento non ce l ha o non la dà.

    E la proprieta del difetto: un elemento non-mail non la espone e il suo accesso alza
    `AttributeError`. None e una risposta, non un errore.
    """
    try:
        v = it.ReceivedTime
    except ERRORI_ELEMENTO:
        return None
    try:
        return _utc(v, fuso_locale)
    except (TypeError, ValueError, OSError, OverflowError):
        return None


class Saltati:
    """Gli elementi visti e non consegnati (3R, blocco 1.C).

    Due numeri diversi, e apposta: `elementi` sono quelli che il server può RILEGGERE, perché hanno
    un EntryID e diventano scarti di lettura; `totale` sono tutti quelli visti, compresi quelli che
    non hanno detto nemmeno chi fossero. Riportare solo i primi direbbe che ne sono stati saltati
    meno di quanti ne sono stati saltati davvero, ed è il genere di conteggio che fa cercare nel
    posto sbagliato.
    """

    def __init__(self) -> None:
        self.elementi: list = []
        self.totale: int = 0

    def aggiungi(self, elemento=None) -> None:
        self.totale += 1
        if elemento is not None:
            self.elementi.append(elemento)

    def svuota(self) -> list:
        """Gli elementi rileggibili raccolti finora, e la lista riparte: chi li prende li sta
        consegnando al server, e consegnarli due volte li scriverebbe due volte."""
        fuori, self.elementi = self.elementi, []
        return fuori

    def __len__(self) -> int:
        return self.totale


def nome_di(cart, default: str = "?") -> str:
    try:
        return str(cart.Name or default)
    except ERRORI_ELEMENTO:
        return default


def _sicuro(nome: str, max_len: int = 120) -> str:
    nome = _NOME_VIETATI.sub("_", (nome or "").strip()) or "allegato"
    return nome[:max_len]


# ---------------------------------------------------------------- finestra temporale (voce 2.9, N40)

# La proprietà DASL della data di ricezione. Con `@SQL=` e QUESTA proprietà il confronto lo fa
# Outlook in UTC e il formato della data è fisso: 'AAAA-MM-GG HH:MM'. È l'unico modo di filtrare per
# data che non dipenda dal locale del PC — con la sintassi Jet (`[ReceivedTime] >= '15/09/2026'`) la
# stessa riga significa 15 settembre su un PC italiano e nulla su uno americano, e un filtro che non
# corrisponde a niente non dà errore: dà zero messaggi.
URN_RICEVUTA = "urn:schemas:httpmail:datereceived"


def _utc_dasl(d: datetime) -> str:
    """Un istante nel formato che Outlook si aspetta dentro un filtro DASL, in UTC."""
    if d.tzinfo is None:
        d = d.replace(tzinfo=timezone.utc)
    return d.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M")


def filtro_finestra(dal: datetime, al: datetime | None = None) -> str:
    """Il filtro `Restrict` della finestra [dal, al], in UTC (voce 2.9).

    I due estremi vengono ALLARGATI al minuto: `dal` si arrotonda in giù, `al` in su. Il formato DASL
    ha la risoluzione del minuto, e fra le due direzioni dell'errore ce n'è una sola che si può
    correggere dopo: leggere un messaggio in più costa una deduplica per Message-ID, che il server fa
    comunque; leggerne uno in meno significa non averlo mai visto, e nessuno andrà a cercarlo.
    """
    parti = ['"%s" >= \'%s\'' % (URN_RICEVUTA, _utc_dasl(dal.replace(second=0, microsecond=0)))]
    if al is not None:
        su = al.replace(second=0, microsecond=0) + timedelta(minutes=1)
        parti.append('"%s" <= \'%s\'' % (URN_RICEVUTA, _utc_dasl(su)))
    return "@SQL=" + " AND ".join(parti)


def confronta_insiemi(con_restrict: set, lineare: set) -> tuple[bool, list, list]:
    """Il self-test della voce 2.9 (C3): i due insiemi di EntryID devono coincidere.

    Le due differenze non pesano uguale, e il confronto le tiene separate apposta:

      mancanti  elementi che la scansione lineare vede e `Restrict` no. È il caso grave: sono
                messaggi che non entrerebbero mai nel Cockpit, in silenzio. Basta uno perché
                `Restrict` vada spento per quella cartella;
      in_più    elementi che `Restrict` restituisce e la lineare no. Succede al bordo della finestra,
                perché il filtro è arrotondato al minuto: il server li deduplica per Message-ID e non
                fanno danno. Si segnalano, non si contano come fallimento.
    """
    mancanti = sorted(lineare - con_restrict)
    in_piu = sorted(con_restrict - lineare)
    return (not mancanti), mancanti, in_piu


class MemoriaRestrict:
    """L'esito del self-test di `Restrict`, che sopravvive al riavvio del worker (3R, blocco 1.D).

    Perché esista: la prova costa una scansione lineare della finestra di prova, cioè esattamente
    ciò che `Restrict` serve a evitare. Tenerla solo in memoria significa rifarla su ogni cartella
    a ogni riavvio — e il worker si riavvia spesso: attività pianificata, uscita forzata del battito
    (C16), aggiornamento del pacchetto. L'esito però non dipende dal processo: dipende dall'indice
    di Outlook di QUEL profilo su QUEL PC, e fra un riavvio e l'altro non cambia.

    La chiave è `postazione + casella + cartella`, perché è di quello che l'esito parla: lo stesso
    indice può essere sano su un PC e rotto su un altro. Store e cartella si accorciano in uno
    sha256 — uno StoreID è lungo 600 caratteri — e accanto resta il nome leggibile, perché un file
    di stato che non si riesce a leggere non aiuta chi deve capire.

    Un esito NEGATIVO si ricorda come uno positivo: dice «su questa cartella si va di scansione
    lineare», ed è altrettanto costoso da riscoprire. Un esito non concludente (finestra di prova
    vuota) non si scrive: non ha dimostrato niente, e ricordarlo sarebbe peggio che non provare.

    Il file è di comodo, non un dato: se manca, è illeggibile o ha un'altra forma, si riparte dalla
    prova. Non deve mai fermare un sync.
    """

    VERSIONE = 1

    def __init__(self, percorso: str, valide_ore: float = 168.0):
        self.percorso = percorso
        self.valide_ore = float(valide_ore)
        self._dati: dict = {}
        self._caricato = False

    @staticmethod
    def chiave(postazione: str, store_id: str, cartella_id: str) -> str:
        grezza = "|".join([(postazione or "").upper(), store_id or "", cartella_id or ""])
        return hashlib.sha256(grezza.encode("utf-8", "ignore")).hexdigest()[:32]

    def _carica(self) -> None:
        if self._caricato:
            return
        self._caricato = True
        try:
            with open(self.percorso, encoding="utf-8") as f:
                dati = json.load(f)
            if isinstance(dati, dict) and dati.get("versione") == self.VERSIONE:
                voci = dati.get("cartelle")
                self._dati = voci if isinstance(voci, dict) else {}
        except (OSError, ValueError, TypeError):
            self._dati = {}

    def leggi(self, postazione: str, store_id: str, cartella_id: str) -> bool | None:
        """L'esito ricordato, o None se non c'è, se è scaduto o se il file non si legge."""
        self._carica()
        voce = self._dati.get(self.chiave(postazione, store_id, cartella_id))
        if not isinstance(voce, dict) or not isinstance(voce.get("esito"), bool):
            return None
        try:
            quando = datetime.fromisoformat(str(voce.get("quando")))
        except (TypeError, ValueError):
            return None
        if quando.tzinfo is None:
            quando = quando.replace(tzinfo=timezone.utc)
        if self.valide_ore > 0 and datetime.now(timezone.utc) - quando > timedelta(hours=self.valide_ore):
            return None
        return voce["esito"]

    def scrivi(self, postazione: str, store_id: str, cartella_id: str, esito: bool, nome: str = "") -> None:
        self._carica()
        self._dati[self.chiave(postazione, store_id, cartella_id)] = {
            "esito": bool(esito),
            "quando": datetime.now(timezone.utc).isoformat(timespec="seconds"),
            "postazione": (postazione or "").upper(),
            "cartella": nome,
        }
        try:
            os.makedirs(os.path.dirname(os.path.abspath(self.percorso)), exist_ok=True)
            tmp = self.percorso + ".tmp"
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump({"versione": self.VERSIONE, "cartelle": self._dati}, f, ensure_ascii=False, indent=1)
            os.replace(tmp, self.percorso)
        except OSError as e:
            log.warning("esito del self-test Restrict non memorizzato in %s: %s", self.percorso, e)


def _scorri(items, dove: str = ""):
    """Gli elementi di una collezione COM, uno alla volta. GetFirst/GetNext e non `for x in items`:
    l'iteratore di pywin32 su una collezione Items grande e' piu' lento e non rispetta sempre Sort.

    L'avanzamento stesso e' protetto: `GetNext` su una collezione che Outlook sta ricostruendo puo'
    alzare `com_error`. Che cosa fare dopo dipende dall'ORDINE, e con il blocco 3 del 3R l'ordine e'
    cambiato in tutti e due i modi di leggere:

      prima, crescente    cio' che si era letto era il pezzo VECCHIO della finestra. Fermarsi li' era
                          sicuro: il cursore avanzava solo su cio' che era stato consegnato, e il
                          resto — la parte nuova — veniva riletto al giro dopo. Quindi si registrava
                          l'errore e ci si chiudeva in silenzio;
      adesso, decrescente cio' che si e' letto e' il pezzo NUOVO. Cio' che resta fuori e' il pezzo
                          VECCHIO, e sopra ci passa un limite superiore di finestra che, se qualcuno
                          dichiarasse conclusa quella finestra, coprirebbe anche la parte mai
                          guardata: messaggi che nessuno andrebbe piu' a cercare.

    Percio' adesso l'interruzione alza sempre. Non e' una precauzione in piu': e' l'unico modo in cui
    chi ha chiesto la lettura puo' sapere di non aver visto tutto. Una finestra interrotta si
    rilegge; una finestra interrotta e creduta completa e' posta persa.
    """
    try:
        it = items.GetFirst()
    except ERRORI_ELEMENTO as e:
        raise LetturaIncompleta("%s: la collezione non si apre (%s)" % (dove or "cartella", e)) from e
    while it is not None:
        yield it
        try:
            it = items.GetNext()
        except ERRORI_ELEMENTO as e:
            raise LetturaIncompleta("%s: enumerazione interrotta (%s)" % (dove or "cartella", e)) from e


def _entry_id(elementi) -> set:
    """L'insieme degli EntryID di una sequenza di elementi, per il confronto del self-test."""
    out = set()
    for it in elementi:
        eid = entryid_di(it)
        if eid:
            out.add(eid)
    return out


def sha256_file(path: str) -> tuple[str, int]:
    h = hashlib.sha256()
    n = 0
    with open(path, "rb") as f:
        for blocco in iter(lambda: f.read(1 << 20), b""):
            h.update(blocco)
            n += len(blocco)
    return h.hexdigest(), n


class Outlook:
    def __init__(self, consenti_invio: bool = False, usa_restrict: bool = True, autoprova_ore: float = 24.0,
                 memoria: "MemoriaRestrict | None" = None, postazione: str = ""):
        pythoncom.CoInitialize()
        self.app = win32com.client.Dispatch("Outlook.Application")
        self.ns = self.app.GetNamespace("MAPI")
        self.consenti_invio = consenti_invio
        # voce 2.9: `usa_restrict = false` in worker.toml spegne il filtro e lascia solo la scansione
        # lineare. Non è un'opzione di comodo, è la via d'uscita se un giorno un profilo si comporta
        # in modo che il self-test non prevede: meglio lento che incompleto.
        self.usa_restrict = usa_restrict
        # La prova ORDINARIA guarda le ultime 24 ore (3R 1.D): è un campione, e ciò che deve
        # dimostrare — che il fuso e il formato della data del filtro siano quelli giusti — si
        # dimostra su un campione come su tutto. La prova larga resta a `--restrict GIORNI`, che
        # è una richiesta esplicita di chi la sta facendo e sa quanto costa.
        self.autoprova_ore = float(autoprova_ore)
        self.restrict_ok: dict[tuple, bool] = {}
        # dove l'esito della prova sopravvive al riavvio; None = solo in memoria, come prima
        self.memoria = memoria
        self.postazione = (postazione or socket.gethostname()).upper()
        self.indirizzi_propri = set()
        try:
            for i in range(1, self.ns.Accounts.Count + 1):
                a = self.ns.Accounts.Item(i)
                if a.SmtpAddress:
                    self.indirizzi_propri.add(a.SmtpAddress.lower())
        except pywintypes.com_error:
            pass
        log.info("Outlook %s, profilo %s, account %s", self.app.Version, self.ns.CurrentProfileName, sorted(self.indirizzi_propri))

    # ------------------------------------------------------------ caselle → store locale (voce 2.6, N44)

    def risolvi_caselle(self, caselle: list[dict]) -> tuple[dict[str, str], list[str]]:
        """Per ogni casella censita che il server chiede di servire, trova lo store del PROFILO
        LOCALE che la contiene. Restituisce {casella_id: store_id} e l'elenco delle non trovate.

        Si parte dall'elenco del server, MAI dal profilo: uno store che c'è nel profilo e non è in
        elenco (la casella di un collega, un archivio) non viene aperto, letto né censito (M1).

        D5 è aperta — Commerciale può essere una cassetta condivisa con delega oppure un account
        con credenziali proprie — quindi non si assume nessuna delle due forme: si prova, in
        ordine, ciò che funziona per entrambe.
          1. un account del profilo con quello SMTP → il suo DeliveryStore (account dedicato);
          2. uno store del profilo il cui proprietario/nome corrisponde all'indirizzo (cassetta
             aggiunta o automappata: compare in ns.Stores);
          3. CreateRecipient(indirizzo) + GetSharedDefaultFolder(Inbox).Store (delega Exchange).
        Il log dice quale strada ha funzionato: è ciò che M1 chiede di leggere.
        """
        trovate: dict[str, str] = {}
        mancanti: list[str] = []
        for c in caselle:
            cid, ind = str(c.get("casella_id", "")), str(c.get("indirizzo", "")).lower()
            if not cid or not ind:
                continue
            store_id, via = self._store_di(ind)
            if store_id:
                trovate[cid] = store_id
                log.info("servo: %s → store %.24s… (%s)", c.get("nome") or ind, store_id, via)
            else:
                mancanti.append(c.get("nome") or ind)
                log.warning("casella %s (%s) non trovata nel profilo Outlook di questo PC: non la servo", c.get("nome") or ind, ind)
        return trovate, mancanti

    def _store_di(self, indirizzo: str) -> tuple[str, str]:
        # 1. account dedicato
        try:
            for i in range(1, self.ns.Accounts.Count + 1):
                a = self.ns.Accounts.Item(i)
                if (a.SmtpAddress or "").lower() == indirizzo:
                    st = a.DeliveryStore
                    if st is not None and st.StoreID:
                        return st.StoreID, "account del profilo"
        except pywintypes.com_error:
            pass
        # 2. store presente nel profilo (cassetta aggiunta o automappata)
        try:
            for i in range(1, self.ns.Stores.Count + 1):
                st = self.ns.Stores.Item(i)
                nome = (st.DisplayName or "").lower()
                smtp = _prop(st, PR_SMTP_ADDRESS_STORE)
                if nome == indirizzo or (isinstance(smtp, str) and smtp.lower() == indirizzo):
                    return st.StoreID, "store del profilo"
        except pywintypes.com_error:
            pass
        # 3. cassetta condivisa con delega
        try:
            r = self.ns.CreateRecipient(indirizzo)
            if r.Resolve():
                cart = self.ns.GetSharedDefaultFolder(r, OL_INBOX)
                st = cart.Store
                if st is not None and st.StoreID:
                    return st.StoreID, "cassetta condivisa (delega)"
        except pywintypes.com_error:
            pass
        return "", ""

    def _store(self, store_id: str):
        """Lo Store con quello StoreID nel profilo corrente; None se non c'è (StoreID di un altro PC)."""
        if not store_id:
            return None
        try:
            for i in range(1, self.ns.Stores.Count + 1):
                st = self.ns.Stores.Item(i)
                if st.StoreID == store_id:
                    return st
        except pywintypes.com_error:
            pass
        return None

    # ------------------------------------------------------------ cartelle

    def cartella(self, nome: str, store_id: str = ""):
        """'Inbox' / 'Sent Items' / 'Bozze' oppure un percorso 'Store\\Cartella\\Sotto'.

        Con `store_id` la cartella è cercata DENTRO quello store (la casella risolta per il job):
        'Posta in arrivo' di Commerciale non è la Posta in arrivo del profilo. Senza, vale il
        comportamento di prima: lo store predefinito.
        """
        chiave = nome.strip().lower()
        st = self._store(store_id)
        if store_id and st is None:
            raise ErroreDefinitivo(f"store {store_id[:24]}… non presente nel profilo Outlook di questo PC")
        if chiave in OL_FOLDER:
            if st is not None:
                try:
                    return st.GetDefaultFolder(OL_FOLDER[chiave])
                except pywintypes.com_error as e:
                    raise ErroreDefinitivo(f"cartella predefinita {nome!r} non disponibile nello store {st.DisplayName!r}: {e}") from e
            return self.ns.GetDefaultFolder(OL_FOLDER[chiave])
        parti = [p for p in re.split(r"[\\/]", nome) if p]
        radice = None
        if st is not None:
            radice = st.GetRootFolder()
        else:
            for i in range(1, self.ns.Folders.Count + 1):
                s = self.ns.Folders.Item(i)
                if s.Name.lower() == parti[0].lower():
                    radice, parti = s, parti[1:]
                    break
        if radice is None:
            radice = self.ns.GetDefaultFolder(6).Parent
        f = radice
        for p in parti:
            trovata = None
            for j in range(1, f.Folders.Count + 1):
                s = f.Folders.Item(j)
                if s.Name.lower() == p.lower():
                    trovata = s
                    break
            if trovata is None:
                raise ErroreDefinitivo(f"cartella non trovata: {nome} (manca '{p}')")
            f = trovata
        return f

    def elenca_cartelle(self) -> list[str]:
        out = []

        def visita(f, prefisso):
            for i in range(1, f.Folders.Count + 1):
                s = f.Folders.Item(i)
                out.append(f"{prefisso}{s.Name} ({s.Items.Count})")
                visita(s, prefisso + s.Name + "\\")

        for i in range(1, self.ns.Folders.Count + 1):
            st = self.ns.Folders.Item(i)
            out.append(st.Name)
            visita(st, st.Name + "\\")
        return out

    # ------------------------------------------------------------ lettura

    def leggi(self, nome_cartella: str, dal: datetime, al: datetime | None = None, store_id: str = "",
              saltati: "Saltati | None" = None) -> Iterator[MessaggioIn]:
        """Elementi MailItem della finestra [dal, al], dal piu’ RECENTE al piu’ vecchio, uno alla volta.

        Due modi di trovarli, e il primo è quello buono (voce 2.9):

          Restrict   un filtro DASL in UTC: Outlook usa il proprio indice e restituisce solo la
                     finestra. Costa quanto i messaggi della finestra, non quanti ne ha la cartella.
                     Su Commerciale, con 20 000 elementi, è la differenza fra un secondo e due
                     minuti — cioè fra un sync al minuto e un sync che non sta dietro a niente;
          lineare    la scansione all'indietro dal più recente, che si ferma al primo più vecchio di
                     `dal`. È il ripiego di quando il self-test dice che `Restrict` non è affidabile
                     su questa cartella, e resta la definizione di «giusto» con cui il self-test
                     confronta l'altro.

        L'ordine non è un dettaglio estetico, e dal blocco 3 del 3R è il contrario di prima.

        Prima si leggeva in ordine CRESCENTE perché il cursore avanzava mentre i lotti partivano:
        in quel modo un'interruzione lasciava indietro solo la parte nuova della finestra, e quella
        si rileggeva al giro dopo. Il prezzo era che l'operatore vedeva comparire per prima la posta
        più vecchia — su una notte intera, la mail delle 17:05 di ieri prima di quella delle 09:00.

        Adesso si legge dal più recente, che è l'ordine in cui la posta serve, e la sicurezza non
        viene più dall'ordine: viene dal fatto che NESSUNA frontiera si muove finché la finestra non
        è stata percorsa tutta. I lotti partono comunque man mano — il lavoro fatto non si butta —
        ma coprono, non promettono. Per questo un'enumerazione interrotta qui alza sempre
        `LetturaIncompleta`: è il solo modo che ha chi chiama di non dichiarare completa una finestra
        di cui ha visto solo la cima.

        Quando non si riesce a ordinare in modo decrescente la cartella fallisce: senza ordine non
        c'è né un inizio né una fine sicuri.

        `store_id` è lo store della casella del job (voce 2.6): la cartella è la SUA.

        `saltati` è un `Saltati` dove finiscono gli elementi visti e non consegnati: chi chiama lo
        passa se ha modo di registrarli (il worker li manda al server come scarti di lettura, e da
        lì si rileggono uno per uno). Un elemento saltato NON ferma la cartella (3R, blocco 1).
        """
        cart = self.cartella(nome_cartella, store_id)
        e_inviata = False
        try:
            inviata = self._store(store_id).GetDefaultFolder(5) if store_id else self.ns.GetDefaultFolder(5)
            e_inviata = cart.DefaultItemType == 0 and cart.EntryID == inviata.EntryID
        except ERRORI_ELEMENTO:
            pass
        store_id = cart.StoreID
        nome = nome_di(cart, nome_cartella)
        n_saltati = 0
        for item in self._elementi(cart, dal, al, saltati):
            # PRIMA la classe, POI le proprietà di MailItem: è la regola del confine COM, e l'ordine
            # è tutta la correzione. `Class` non letta (None) non è «non è una mail»: si prova lo
            # stesso a convertirlo, e se non si può diventa uno scarto invece di un dubbio.
            classe = classe_di(item)
            if classe is not None and classe != OL_MAIL:
                n_saltati += self._registra_saltato(saltati, cart, item, "elemento non-mail", classe, rileggibile=False)
                continue
            try:
                yield self._converti(item, store_id, nome, e_inviata)
            except ERRORI_ELEMENTO as e:
                n_saltati += self._registra_saltato(saltati, cart, item, "conversione non riuscita: %s" % e, classe)
        if n_saltati:
            log.info("%s: %d elementi saltati (non-mail o illeggibili)", nome_cartella, n_saltati)

    def _registra_saltato(self, saltati: "Saltati | None", cart, it, motivo: str, classe: int | None = None,
                          rileggibile: bool = True) -> int:
        """Un elemento che non entra: si conta sempre, si scrive sempre nel log, e diventa uno scarto
        di lettura solo se rileggerlo può servire a qualcosa.

        `rileggibile` separa due cose che prima si somigliavano, e la differenza l'ha mostrata il
        profilo vero: nella Posta in arrivo di questa azienda ci sono rapporti di consegna
        (`Class=46`), inviti (53) e appuntamenti (26) salvati fra la posta. Un elemento NON-MAIL non
        entrerà mai, per quante volte lo si rilegga: metterlo in `ingest_scarto` riempirebbe
        /admin/scarti di righe che nessuno potrà mai chiudere, e le righe che nessuno può chiudere
        sono il modo più rapido per far smettere di guardare quella pagina. Un elemento ILLEGGIBILE
        — `com_error` su una proprietà, conversione fallita — è invece una mail che OGGI non si è
        riusciti a leggere: quello va in scarto, perché una rilettura mirata può farcela.

        Lo skip non è mai silenzioso: la riga porta cartella, Class ed EntryID, cioè le tre cose con
        cui si va a cercare l'elemento dentro Outlook. Senza EntryID resta solo la riga (il server
        rifiuta uno scarto che non saprebbe rileggere), ma il conteggio c'è lo stesso.
        """
        eid = entryid_di(it)
        nome = nome_di(cart)
        errore = "lettura: %s (Class=%s)" % (motivo, classe if classe is not None else "?")
        log.warning("%s: elemento saltato - %s, EntryID=%s", nome, errore, (eid[:24] + "...") if eid else "non leggibile")
        if saltati is not None:
            scarto = None
            if rileggibile and eid:
                scarto = ElementoSaltato(entry_id=eid, cartella=nome[:200], oggetto=oggetto_di(it)[:500],
                                         ricevuto_il=ricevuta_di(it), errore=errore[:2000])
            saltati.aggiungi(scarto)
        return 1

    # ------------------------------------------------------------ i due modi di trovare la finestra

    def _elementi(self, cart, dal: datetime, al: datetime | None, saltati: "Saltati | None" = None):
        """Gli elementi della finestra in ordine crescente, con Restrict se si può fidare."""
        if self.usa_restrict and self._restrict_affidabile(cart, dal, al):
            ristretti = self._ristretti(cart, dal, al)
            if ristretti is not None:
                return ristretti
        return self._lineari(cart, dal, al, saltati)

    def _ristretti(self, cart, dal: datetime, al: datetime | None):
        """Items.Restrict + ordinamento DECRESCENTE. None se una delle due cose non riesce: chi
        chiama passa alla lineare invece di leggere in un ordine qualunque."""
        try:
            items = cart.Items.Restrict(filtro_finestra(dal, al))
            items.Sort("[ReceivedTime]", True)
        except ERRORI_ELEMENTO as e:
            log.warning("Restrict non disponibile su %s (%s): scansione lineare", nome_di(cart), e)
            return None
        return _scorri(items, dove=nome_di(cart))

    def _lineari(self, cart, dal: datetime, al: datetime | None, saltati: "Saltati | None" = None):
        """La scansione all'indietro: dal più recente fino al primo più vecchio di `dal`.

        Resta il modo lento — deve passare su tutta la cartella per trovare il bordo della finestra
        — ma è quello di cui ci si fida, e dal blocco 3 non tiene più niente in memoria: prima
        raccoglieva tutta la finestra in una lista per poterla invertire e restituirla in ordine
        crescente, adesso l'ordine chiesto è quello in cui la collezione già si trova.

        Qui viveva il difetto del 3R: `item.ReceivedTime` veniva letto PRIMA di qualunque controllo
        sulla classe dell'elemento, e protetto dal solo `com_error`. Un elemento non-mail alza
        `AttributeError`, che usciva da questa funzione, attraversava `leggi`, faceva fallire il
        self-test di Restrict che chiama questa stessa scansione, e portava con sé il sync di tutta
        la cartella — per un elemento che non sarebbe mai entrato comunque.
        """
        items = cart.Items
        nome = nome_di(cart)
        try:
            items.Sort("[ReceivedTime]", True)
        except ERRORI_ELEMENTO as e:
            # senza ordinamento decrescente la scansione non ha né inizio né fine sicuri
            raise LetturaIncompleta("%s: ordinamento non riuscito (%s)" % (nome, e)) from e
        for item in _scorri(items, dove=nome):
            classe = classe_di(item)
            if classe is not None and classe != OL_MAIL:
                self._registra_saltato(saltati, cart, item, "elemento non-mail", classe, rileggibile=False)
                continue
            rt = ricevuta_di(item)
            if rt is None:
                # l'elemento c'è ma non dice quando è arrivato: non si può collocare nella finestra
                # e non si può nemmeno usare per decidere dove fermarsi. Si salta, non si ferma.
                self._registra_saltato(saltati, cart, item, "ReceivedTime non leggibile", classe)
                continue
            if al is not None and rt > al:
                continue
            if rt < dal:
                return
            yield item

    def misura_finestra(self, cart, dal: datetime, al: datetime | None = None) -> dict:
        """Esegue i DUE modi sulla stessa finestra e restituisce insiemi e tempi.

        È il corpo di C2 e C3 insieme: gli insiemi rispondono a «trova le stesse cose», i tempi a
        «quanto costa». Non converte niente e non tocca gli elementi: legge solo gli EntryID, così la
        misura è del filtro e non della lettura dei corpi.
        """
        t0 = time.monotonic()
        ristretti = self._ristretti(cart, dal, al)
        con = _entry_id(ristretti) if ristretti is not None else set()
        t1 = time.monotonic()
        lineare = _entry_id(self._lineari(cart, dal, al))
        t2 = time.monotonic()
        return {"disponibile": ristretti is not None, "restrict": con, "lineare": lineare,
                "t_restrict": t1 - t0, "t_lineare": t2 - t1, "filtro": filtro_finestra(dal, al)}

    # ------------------------------------------------------------ self-test per insieme (C3)

    def _restrict_affidabile(self, cart, dal: datetime, al: datetime | None) -> bool:
        """Vero se su QUESTA cartella `Restrict` può essere usato: o l'ha già dimostrato in questo
        processo, o lo ha dimostrato in uno precedente (3R 1.D), o lo dimostra adesso.

        Tre memorie in fila, dalla più economica: il dizionario di questo processo, il file di stato
        della postazione, la prova vera. La prova costa una scansione lineare, cioè esattamente ciò
        che `Restrict` serve a evitare: rifarla a ogni riavvio del worker su ogni cartella era un
        costo pagato per sapere una cosa che non era cambiata.
        """
        try:
            chiave = (cart.StoreID, cart.EntryID)
        except ERRORI_ELEMENTO:
            return False
        if chiave in self.restrict_ok:
            return self.restrict_ok[chiave]
        if self.memoria is not None:
            ricordato = self.memoria.leggi(self.postazione, chiave[0], chiave[1])
            if ricordato is not None:
                self.restrict_ok[chiave] = ricordato
                log.info("self-test Restrict su %s: esito già noto (%s), non lo rifaccio",
                         nome_di(cart), "attendibile" if ricordato else "NON attendibile")
                return ricordato
        esito = self.autoprova_restrict(cart, dal, al)
        if esito is not None:
            self.restrict_ok[chiave] = esito
            if self.memoria is not None:
                self.memoria.scrivi(self.postazione, chiave[0], chiave[1], esito, nome=nome_di(cart))
            return esito
        return False        # prova non concludente: per questo giro si va di lineare, si riproverà

    def autoprova_restrict(self, cart, dal: datetime, al: datetime | None) -> bool | None:
        """C3: confronta gli INSIEMI di EntryID dei due modi sulla stessa finestra.

        Non confronta i conteggi, che coinciderebbero anche scambiando un messaggio con un altro, e
        non confronta il primo e l'ultimo: confronta chi c'è. È l'unico controllo che, se passa,
        dice davvero «il filtro non sta perdendo niente».

        La prova si fa sulla CODA della finestra (le ultime `autoprova_ore` ore, 24 di regola): la
        scansione lineare su trent'anni di archivio costerebbe esattamente ciò che la voce 2.9 vuole
        evitare, e quello che c'è da dimostrare — che il fuso e il formato della data siano quelli
        giusti — si dimostra su un campione come su tutto.

        Restituisce None se la finestra di prova è vuota da entrambe le parti: due insiemi vuoti sono
        uguali, ma non hanno dimostrato niente, e ricordarsi un «passata» ottenuto così sarebbe
        peggio che non provare.
        """
        fine = al or datetime.now(timezone.utc)
        inizio = dal
        if self.autoprova_ore > 0:
            inizio = max(dal, fine - timedelta(hours=self.autoprova_ore))
        m = self.misura_finestra(cart, inizio, al)
        if not m["disponibile"]:
            return False
        con, lineare = m["restrict"], m["lineare"]
        if not con and not lineare:
            log.info("self-test Restrict su %s: finestra di prova vuota, non concludente", cart.Name)
            return None
        ok, mancanti, in_piu = confronta_insiemi(con, lineare)
        if not ok:
            log.error("self-test Restrict FALLITO su %s: %d elementi che la scansione trova e il filtro no. "
                      "Uso la scansione lineare su questa cartella (più lenta, ma non perde niente). "
                      "Finestra di prova [%s, %s], primi mancanti: %s",
                      cart.Name, len(mancanti), inizio.isoformat(), fine.isoformat(), mancanti[:3])
            return False
        if in_piu:
            log.info("self-test Restrict su %s: %d elementi in più ai bordi della finestra (arrotondamento al minuto): innocui",
                     cart.Name, len(in_piu))
        log.info("self-test Restrict su %s: insiemi uguali su %d elementi, filtro attendibile", cart.Name, len(lineare))
        return True

    def _converti(self, it, store_id: str, nome_cartella: str, e_inviata: bool) -> MessaggioIn:
        message_id = (_prop(it, PR_INTERNET_MESSAGE_ID) or "").strip()
        mittente = self._smtp_mittente(it)
        if not message_id:
            base = f"{it.Subject}|{mittente}|{_utc(it.ReceivedTime).isoformat()}"
            message_id = "noid:" + hashlib.sha1(base.encode("utf-8", "ignore")).hexdigest()
        in_reply_to, riferimenti = "", []
        hdr = _prop(it, PR_TRANSPORT_HEADERS)
        if isinstance(hdr, str) and hdr:
            try:
                h = email.parser.HeaderParser().parsestr(hdr)
                in_reply_to = " ".join((h.get("In-Reply-To") or "").split())
                riferimenti = (h.get("References") or "").split()
            except Exception:  # noqa: BLE001
                pass
        # E solo un'ipotesi del worker, ed e fragile: su una casella condivisa "la Posta inviata" e
        # "i miei indirizzi" dipendono da come e configurato QUESTO profilo. Dalla voce 2.1 decide il
        # server, confrontando il mittente con le caselle censite; qui resta perche serve a scegliere
        # data_evento e perche un worker deve poter parlare anche con un server piu vecchio.
        direzione = "uscita" if (e_inviata or mittente in self.indirizzi_propri) else "entrata"
        # data_evento e QUANDO E SUCCESSO (SentOn per la posta inviata); ricevuto_il e QUANDO E
        # ARRIVATO IN QUESTA CASELLA, ed e il valore su cui il server fa avanzare il cursore, perche
        # e lo stesso su cui filtra la scansione qui sopra (W2).
        data = it.SentOn if direzione == "uscita" else it.ReceivedTime
        try:
            corpo = it.Body or ""
        except ERRORI_ELEMENTO:
            corpo = ""
        try:
            html = it.HTMLBody or ""
        except ERRORI_ELEMENTO:
            html = ""
        categorie = [c.strip() for c in (it.Categories or "").split(",") if c.strip()]
        return MessaggioIn(
            message_id=message_id, entry_id=it.EntryID, store_id=store_id,
            conversation_id=it.ConversationID or "", conversation_index=it.ConversationIndex or "",
            in_reply_to=in_reply_to, riferimenti=riferimenti, cartella=nome_cartella, direzione=direzione,
            data_evento=_utc(data), ricevuto_il=_utc(it.ReceivedTime),
            mittente_nome=it.SenderName or "", mittente_indirizzo=mittente,
            destinatari=self._destinatari(it), oggetto=it.Subject or "", corpo_testo=corpo, corpo_html=html,
            importanza=int(it.Importance), non_letto=bool(it.UnRead), flag_stato=int(it.FlagStatus or 0),
            categorie=categorie, allegati=self._allegati(it, html), marcatori=marcatori_di(it),
        )

    def _smtp_mittente(self, it) -> str:
        try:
            if it.SenderEmailType == "SMTP" and it.SenderEmailAddress:
                return it.SenderEmailAddress.lower()
        except ERRORI_ELEMENTO:
            pass
        for tag in (PR_SENDER_SMTP, PR_SENT_REPR_SMTP):
            v = _prop(it, tag)
            if isinstance(v, str) and "@" in v:
                return v.lower()
        try:
            s = it.Sender
            if s is not None:
                eu = s.GetExchangeUser()
                if eu is not None and eu.PrimarySmtpAddress:
                    return eu.PrimarySmtpAddress.lower()
        except ERRORI_ELEMENTO:
            pass
        try:
            return (it.SenderEmailAddress or "").lower()
        except ERRORI_ELEMENTO:
            return ""

    def _destinatari(self, it) -> list[Destinatario]:
        out = []
        tipi = {OL_TO: "a", OL_CC: "cc", OL_BCC: "ccn"}
        try:
            for r in it.Recipients:
                smtp = _prop(r, PR_SMTP_ADDRESS)
                if not (isinstance(smtp, str) and "@" in smtp):
                    smtp = r.Address if "@" in (r.Address or "") else ""
                out.append(Destinatario(nome=r.Name or "", indirizzo=(smtp or "").lower(), tipo=tipi.get(r.Type, "a")))
        except ERRORI_ELEMENTO:
            pass
        return out

    def _allegati(self, it, html: str) -> list[AllegatoIn]:
        out = []
        try:
            n = it.Attachments.Count
        except pywintypes.com_error:
            return out
        for i in range(1, n + 1):
            a = it.Attachments.Item(i)
            try:
                nome = a.FileName or a.DisplayName or f"allegato_{i}"
            except pywintypes.com_error:
                nome = f"allegato_{i}"
            tipo = int(a.Type)
            cid = _prop(a, PR_ATTACH_CONTENT_ID) or ""
            hidden = bool(_prop(a, PR_ATTACHMENT_HIDDEN, False))
            if tipo == OL_ATT_EMBEDDED:
                natura = "elemento_outlook"
                if not nome.lower().endswith(".msg"):
                    nome += ".msg"
            elif tipo == OL_ATT_BYREF:
                natura = "collegamento"
            elif hidden or (cid and f"cid:{cid}" in html):
                natura = "inline"
            else:
                natura = "file"
            ext = os.path.splitext(nome)[1].lstrip(".").lower()
            try:
                size = int(a.Size)
            except pywintypes.com_error:
                size = 0
            out.append(AllegatoIn(indice=i, nome_file=nome, estensione=ext, content_type=str(_prop(a, PR_ATTACH_MIME_TAG) or ""),
                                  natura=natura, bytes=size, content_id=str(cid)))
        return out

    # ------------------------------------------------------------ allegati → staging

    def salva_allegato(self, entry_id: str, store_id: str, indice: int, nome_file: str, cartella_staging: str,
                       message_id: str = "") -> tuple[str, str, int, dict]:
        it = self._item(entry_id, store_id, message_id)
        if indice < 1 or indice > it.Attachments.Count:
            raise ErroreDefinitivo(f"allegato {indice} non presente (l'elemento ne ha {it.Attachments.Count})")
        a = it.Attachments.Item(indice)
        os.makedirs(cartella_staging, exist_ok=True)
        dest = os.path.join(cartella_staging, f"{indice:02d}_{_sicuro(nome_file)}")
        if int(a.Type) == OL_ATT_EMBEDDED and not dest.lower().endswith(".msg"):
            dest += ".msg"
        try:
            a.SaveAsFile(dest)
        except pywintypes.com_error as e:
            raise ErroreDefinitivo(f"SaveAsFile fallito: {e}") from e
        sha, n = sha256_file(dest)
        return dest, sha, n, self.dove(it)

    # ------------------------------------------------------------ comandi

    def _item(self, entry_id: str, store_id: str, message_id: str = ""):
        """Ritrova l'elemento: prima per EntryID (veloce), poi per Message-ID in tutte le cartelle DI
        QUELLO STORE (l'EntryID cambia quando l'elemento viene spostato dopo l'ultimo sync).

        `store_id` è lo store della casella risolto in QUESTO profilo (voce 2.6): non arriva dal
        server, che non conosce gli store di questo PC."""
        try:
            return self.ns.GetItemFromID(entry_id, store_id)
        except pywintypes.com_error as e:
            if not (e.hresult == MAPI_E_NOT_FOUND or (e.excepinfo and e.excepinfo[5] == MAPI_E_NOT_FOUND)):
                raise
        if message_id:
            it = self._cerca_per_message_id(message_id, store_id)
            if it is not None:
                log.info("elemento ritrovato per Message-ID in %s", it.Parent.Name)
                return it
        raise ErroreDefinitivo("elemento non trovato in nessuna cartella della casella (eliminato?)")

    def _cerca_per_message_id(self, message_id: str, store_id: str = ""):
        """DASL Find sulla proprietà PR_INTERNET_MESSAGE_ID, cartella per cartella (locale-indipendente),
        dentro lo store indicato."""
        filtro = "@SQL=\"%s\" = '%s'" % (PR_INTERNET_MESSAGE_ID, message_id.replace("'", "''"))
        for cartella in self._tutte_le_cartelle(store_id):
            try:
                if cartella.DefaultItemType != 0:
                    continue
                it = cartella.Items.Find(filtro)
                if it is not None:
                    return it
            except pywintypes.com_error:
                continue
        return None

    def _tutte_le_cartelle(self, store_id: str = ""):
        """Cartelle di posta di UNO store (quello della casella; senza, il predefinito), in profondità,
        Inbox e Posta inviata per prime. Non si esce mai dallo store: cercare un Message-ID negli
        altri store del profilo troverebbe la copia di un'altra casella, e l'EntryID riportato al
        server finirebbe sulla presenza sbagliata."""
        st = self._store(store_id)
        if store_id and st is None:
            return
        radice = st.GetRootFolder() if st is not None else self.ns.GetDefaultFolder(6).Parent
        prime = []
        for idx in (6, 5, 3, 16):
            try:
                prime.append(st.GetDefaultFolder(idx) if st is not None else self.ns.GetDefaultFolder(idx))
            except pywintypes.com_error:
                pass
        visti = set()
        pila = prime + [radice]
        while pila:
            c = pila.pop(0)
            try:
                eid = c.EntryID
            except pywintypes.com_error:
                continue
            if eid in visti:
                continue
            visti.add(eid)
            yield c
            try:
                pila.extend(list(c.Folders))
            except pywintypes.com_error:
                pass

    @staticmethod
    def dove(it) -> dict:
        """EntryID e cartella effettivi dell'elemento: il server riallinea la presenza. Nessuno
        store_id: al server non direbbe niente (è di questo profilo)."""
        try:
            return {"entry_id": it.EntryID, "cartella": it.Parent.Name}
        except pywintypes.com_error:
            return {}

    def apri(self, entry_id: str, store_id: str, message_id: str = "") -> dict:
        it = self._item(entry_id, store_id, message_id)
        it.Display(False)
        return self.dove(it)

    def segna_letto(self, entry_id: str, store_id: str, letto: bool, message_id: str = "") -> dict:
        it = self._item(entry_id, store_id, message_id)
        it.UnRead = not letto
        it.Save()
        return self.dove(it)

    def sposta(self, entry_id: str, store_id: str, cartella: str, message_id: str = "") -> dict:
        it = self._item(entry_id, store_id, message_id)
        nuovo = it.Move(self.cartella(cartella, store_id))
        return self.dove(nuovo)

    def crea_bozza(self, p: PayloadCreaBozza, store_id: str = "") -> tuple[str, bool]:
        """Prepara la mail e la lascia come bozza aperta in Outlook. Send() solo con consenti_invio.
        `store_id` è lo store locale della casella dell'originale (voce 2.6).

        Sulla bozza si scrivono i MARCATORI (blocco 7B): `CockpitBozza` = bozza_id, sempre, piu' quelli
        del payload (`CockpitRichiestaFornitore` = richiesta_id). Restano attaccati alla mail quando
        parte, e il sync della Posta inviata li rilegge: e' cosi' che il server lega la mail inviata a
        cio' che l'ha generata, senza indovinare dall'oggetto."""
        if p.tipo == "nuovo":
            item = self.app.CreateItem(0)
        else:
            orig = self._item(p.entry_id, store_id, p.message_id)
            if p.tipo in ("risposta", "sollecito"):
                item = orig.Reply()
            elif p.tipo == "rispondi_tutti":
                item = orig.ReplyAll()
            else:
                item = orig.Forward()
        tipi = {"a": OL_TO, "cc": OL_CC, "ccn": OL_BCC}
        for d in p.destinatari:
            if d.indirizzo:
                r = item.Recipients.Add(d.indirizzo)
                r.Type = tipi.get(d.tipo, OL_TO)
        if p.destinatari:
            item.Recipients.ResolveAll()
        if p.oggetto:
            item.Subject = p.oggetto
        if p.mostra:
            item.Display(False)  # prima di toccare il corpo: così Outlook inserisce la firma
        if p.corpo_html:
            item.HTMLBody = p.corpo_html + (item.HTMLBody or "")
        elif p.corpo_testo:
            item.Body = p.corpo_testo + "\n\n" + (item.Body or "")
        for percorso in p.allegati:
            if os.path.isfile(percorso):
                item.Attachments.Add(percorso)
            else:
                log.warning("allegato bozza non trovato: %s", percorso)
        marcatori = {"CockpitBozza": str(p.bozza_id)}
        marcatori.update({k: v for k, v in (p.marcatori or {}).items() if k.startswith("Cockpit") and v})
        scrivi_marcatori(item, marcatori)
        item.Save()
        entry_id = item.EntryID
        inviata = False
        if p.invia:
            if not self.consenti_invio:
                log.warning("invio richiesto ma consenti_invio=false: resta bozza")
            else:
                item.Send()
                inviata = True
        return entry_id, inviata
