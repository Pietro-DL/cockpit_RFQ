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
import logging
import os
import re
import time
from datetime import datetime, timedelta, timezone
from typing import Iterator

import pythoncom
import pywintypes
import win32com.client

from contratti import AllegatoIn, Destinatario, MessaggioIn, PayloadCreaBozza

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


def _scorri(items):
    """Gli elementi di una collezione COM, uno alla volta. GetFirst/GetNext e non `for x in items`:
    l'iteratore di pywin32 su una collezione Items grande è più lento e non rispetta sempre Sort."""
    it = items.GetFirst()
    while it is not None:
        yield it
        it = items.GetNext()


def _entry_id(elementi) -> set:
    """L'insieme degli EntryID di una sequenza di elementi, per il confronto del self-test."""
    out = set()
    for it in elementi:
        try:
            out.add(it.EntryID)
        except pywintypes.com_error:
            continue
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
    def __init__(self, consenti_invio: bool = False, usa_restrict: bool = True, autoprova_giorni: int = 7):
        pythoncom.CoInitialize()
        self.app = win32com.client.Dispatch("Outlook.Application")
        self.ns = self.app.GetNamespace("MAPI")
        self.consenti_invio = consenti_invio
        # voce 2.9: `usa_restrict = false` in worker.toml spegne il filtro e lascia solo la scansione
        # lineare. Non è un'opzione di comodo, è la via d'uscita se un giorno un profilo si comporta
        # in modo che il self-test non prevede: meglio lento che incompleto.
        self.usa_restrict = usa_restrict
        self.autoprova_giorni = autoprova_giorni
        self.restrict_ok: dict[tuple, bool] = {}
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

    def leggi(self, nome_cartella: str, dal: datetime, al: datetime | None = None, store_id: str = "") -> Iterator[MessaggioIn]:
        """Elementi MailItem della cartella con ReceivedTime >= dal (e <= al se specificato), in
        ordine cronologico CRESCENTE, uno alla volta.

        Due modi di trovarli, e il primo è quello buono (voce 2.9):

          Restrict   un filtro DASL in UTC: Outlook usa il proprio indice e restituisce solo la
                     finestra. Costa quanto i messaggi della finestra, non quanti ne ha la cartella.
                     Su Commerciale, con 20 000 elementi, è la differenza fra un secondo e due
                     minuti — cioè fra un sync al minuto e un sync che non sta dietro a niente;
          lineare    la scansione all'indietro dal più recente, che si ferma al primo più vecchio di
                     `dal`. È il ripiego di quando il self-test dice che `Restrict` non è affidabile
                     su questa cartella, e resta la definizione di «giusto» con cui il self-test
                     confronta l'altro.

        L'ordine crescente non è un dettaglio estetico: il cursore avanza mentre i lotti partono, e
        con un ordine qualunque un lotto potrebbe portare un cursore più avanti di messaggi non
        ancora spediti. Se il job muore lì, quei messaggi restano indietro al cursore e nessuno li
        rilegge più. Perciò, quando non si riesce a ordinare in modo crescente, si usa la lineare —
        che l'ordine ce l'ha per costruzione.

        `store_id` è lo store della casella del job (voce 2.6): la cartella è la SUA.
        """
        cart = self.cartella(nome_cartella, store_id)
        e_inviata = False
        try:
            inviata = self._store(store_id).GetDefaultFolder(5) if store_id else self.ns.GetDefaultFolder(5)
            e_inviata = cart.DefaultItemType == 0 and cart.EntryID == inviata.EntryID
        except (pywintypes.com_error, AttributeError):
            pass
        store_id = cart.StoreID
        saltati = 0
        for item in self._elementi(cart, dal, al):
            try:
                if item.Class != OL_MAIL:
                    saltati += 1
                    continue
                yield self._converti(item, store_id, cart.Name, e_inviata)
            except pywintypes.com_error as e:
                log.warning("elemento saltato in %s: %s", nome_cartella, e)
                saltati += 1
        if saltati:
            log.info("%s: %d elementi non-mail saltati", nome_cartella, saltati)

    # ------------------------------------------------------------ i due modi di trovare la finestra

    def _elementi(self, cart, dal: datetime, al: datetime | None):
        """Gli elementi della finestra in ordine crescente, con Restrict se si può fidare."""
        if self.usa_restrict and self._restrict_affidabile(cart, dal, al):
            ristretti = self._ristretti(cart, dal, al)
            if ristretti is not None:
                return ristretti
        return self._lineari(cart, dal, al)

    def _ristretti(self, cart, dal: datetime, al: datetime | None):
        """Items.Restrict + ordinamento crescente. None se una delle due cose non riesce: chi chiama
        passa alla lineare invece di leggere in un ordine qualunque."""
        try:
            items = cart.Items.Restrict(filtro_finestra(dal, al))
            items.Sort("[ReceivedTime]", False)
        except pywintypes.com_error as e:
            log.warning("Restrict non disponibile su %s (%s): scansione lineare", cart.Name, e)
            return None
        return _scorri(items)

    def _lineari(self, cart, dal: datetime, al: datetime | None):
        """La scansione all'indietro: dal più recente fino al primo più vecchio di `dal`.

        Raccoglie e inverte, quindi tiene in memoria la finestra: è il motivo per cui non è il modo
        buono su una cartella grande, oltre al tempo. Resta però quello di cui ci si fida.
        """
        items = cart.Items
        items.Sort("[ReceivedTime]", True)
        raccolti = []
        item = items.GetFirst()
        while item is not None:
            try:
                rt = _utc(item.ReceivedTime)
                if al is not None and rt > al:
                    item = items.GetNext()
                    continue
                if rt < dal:
                    break
                raccolti.append(item)
            except pywintypes.com_error as e:
                log.warning("elemento saltato in %s: %s", cart.Name, e)
            item = items.GetNext()
        raccolti.reverse()
        return iter(raccolti)

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
        processo, o lo dimostra adesso. Il risultato si ricorda per cartella, non per sync."""
        try:
            chiave = (cart.StoreID, cart.EntryID)
        except pywintypes.com_error:
            return False
        if chiave in self.restrict_ok:
            return self.restrict_ok[chiave]
        esito = self.autoprova_restrict(cart, dal, al)
        if esito is not None:
            self.restrict_ok[chiave] = esito
            return esito
        return False        # prova non concludente: per questo giro si va di lineare, si riproverà

    def autoprova_restrict(self, cart, dal: datetime, al: datetime | None) -> bool | None:
        """C3: confronta gli INSIEMI di EntryID dei due modi sulla stessa finestra.

        Non confronta i conteggi, che coinciderebbero anche scambiando un messaggio con un altro, e
        non confronta il primo e l'ultimo: confronta chi c'è. È l'unico controllo che, se passa,
        dice davvero «il filtro non sta perdendo niente».

        La prova si fa sulla CODA della finestra (gli ultimi `autoprova_giorni` giorni): la scansione
        lineare su trent'anni di archivio costerebbe esattamente ciò che la voce 2.9 vuole evitare, e
        quello che c'è da dimostrare — che il fuso e il formato della data siano quelli giusti — si
        dimostra su un campione come su tutto.

        Restituisce None se la finestra di prova è vuota da entrambe le parti: due insiemi vuoti sono
        uguali, ma non hanno dimostrato niente, e ricordarsi un «passata» ottenuto così sarebbe
        peggio che non provare.
        """
        fine = al or datetime.now(timezone.utc)
        inizio = dal
        if self.autoprova_giorni > 0:
            inizio = max(dal, fine - timedelta(days=self.autoprova_giorni))
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
        except pywintypes.com_error:
            corpo = ""
        try:
            html = it.HTMLBody or ""
        except pywintypes.com_error:
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
            categorie=categorie, allegati=self._allegati(it, html),
        )

    def _smtp_mittente(self, it) -> str:
        try:
            if it.SenderEmailType == "SMTP" and it.SenderEmailAddress:
                return it.SenderEmailAddress.lower()
        except pywintypes.com_error:
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
        except pywintypes.com_error:
            pass
        return (it.SenderEmailAddress or "").lower()

    def _destinatari(self, it) -> list[Destinatario]:
        out = []
        tipi = {OL_TO: "a", OL_CC: "cc", OL_BCC: "ccn"}
        try:
            for r in it.Recipients:
                smtp = _prop(r, PR_SMTP_ADDRESS)
                if not (isinstance(smtp, str) and "@" in smtp):
                    smtp = r.Address if "@" in (r.Address or "") else ""
                out.append(Destinatario(nome=r.Name or "", indirizzo=(smtp or "").lower(), tipo=tipi.get(r.Type, "a")))
        except pywintypes.com_error:
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
        `store_id` è lo store locale della casella dell'originale (voce 2.6)."""
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
