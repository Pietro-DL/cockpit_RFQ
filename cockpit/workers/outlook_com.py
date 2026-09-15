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
from datetime import datetime, timezone
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
OL_ATT_BYVALUE, OL_ATT_BYREF, OL_ATT_EMBEDDED, OL_ATT_OLE = 1, 4, 5, 6
OL_TO, OL_CC, OL_BCC = 1, 2, 3
MAPI_E_NOT_FOUND = -2147221233  # 0x8004010F

_NOME_VIETATI = re.compile(r'[<>:"/\\|?*\x00-\x1f]')


class ErroreDefinitivo(Exception):
    """Errore che non ha senso ritentare (elemento eliminato, allegato non salvabile)."""


def _utc(d) -> datetime:
    """pywintypes.datetime → datetime UTC aware (il tzinfo COM è inaffidabile, timestamp() no)."""
    if d is None:
        return datetime.now(timezone.utc)
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


def sha256_file(path: str) -> tuple[str, int]:
    h = hashlib.sha256()
    n = 0
    with open(path, "rb") as f:
        for blocco in iter(lambda: f.read(1 << 20), b""):
            h.update(blocco)
            n += len(blocco)
    return h.hexdigest(), n


class Outlook:
    def __init__(self, consenti_invio: bool = False):
        pythoncom.CoInitialize()
        self.app = win32com.client.Dispatch("Outlook.Application")
        self.ns = self.app.GetNamespace("MAPI")
        self.consenti_invio = consenti_invio
        self.indirizzi_propri = set()
        try:
            for i in range(1, self.ns.Accounts.Count + 1):
                a = self.ns.Accounts.Item(i)
                if a.SmtpAddress:
                    self.indirizzi_propri.add(a.SmtpAddress.lower())
        except pywintypes.com_error:
            pass
        log.info("Outlook %s, profilo %s, account %s", self.app.Version, self.ns.CurrentProfileName, sorted(self.indirizzi_propri))

    # ------------------------------------------------------------ cartelle

    def cartella(self, nome: str):
        """'Inbox' / 'Sent Items' / 'Bozze' oppure un percorso 'Store\\Cartella\\Sotto'."""
        chiave = nome.strip().lower()
        if chiave in OL_FOLDER:
            return self.ns.GetDefaultFolder(OL_FOLDER[chiave])
        parti = [p for p in re.split(r"[\\/]", nome) if p]
        radice = None
        for i in range(1, self.ns.Folders.Count + 1):
            st = self.ns.Folders.Item(i)
            if st.Name.lower() == parti[0].lower():
                radice, parti = st, parti[1:]
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

    def leggi(self, nome_cartella: str, dal: datetime, al: datetime | None = None) -> Iterator[MessaggioIn]:
        """Elementi MailItem della cartella con ReceivedTime >= dal (e <= al se specificato), in ordine cronologico.

        Scorre gli elementi ordinati per data decrescente e si ferma al primo più vecchio di `dal`:
        costa O(nuovi), è indipendente dal locale (niente Restrict con date formattate) e la
        deduplica per Message-ID lato server rende innocua la sovrapposizione.
        """
        cart = self.cartella(nome_cartella)
        e_inviata = cart.DefaultItemType == 0 and cart.EntryID == self.ns.GetDefaultFolder(5).EntryID
        items = cart.Items
        items.Sort("[ReceivedTime]", True)
        store_id = cart.StoreID
        raccolti: list[MessaggioIn] = []
        saltati = 0
        item = items.GetFirst()
        while item is not None:
            try:
                if item.Class != OL_MAIL:
                    saltati += 1
                    item = items.GetNext()
                    continue
                rt = _utc(item.ReceivedTime)
                if al is not None and rt > al:
                    item = items.GetNext()
                    continue
                if rt < dal:
                    break
                raccolti.append(self._converti(item, store_id, cart.Name, e_inviata))
            except pywintypes.com_error as e:
                log.warning("elemento saltato in %s: %s", nome_cartella, e)
                saltati += 1
            item = items.GetNext()
        if saltati:
            log.info("%s: %d elementi non-mail saltati", nome_cartella, saltati)
        raccolti.reverse()
        yield from raccolti

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
        """Ritrova l'elemento: prima per EntryID (veloce), poi per Message-ID in tutte le cartelle dello store
        (l'EntryID cambia quando l'elemento viene spostato dopo l'ultimo sync)."""
        try:
            return self.ns.GetItemFromID(entry_id, store_id)
        except pywintypes.com_error as e:
            if not (e.hresult == MAPI_E_NOT_FOUND or (e.excepinfo and e.excepinfo[5] == MAPI_E_NOT_FOUND)):
                raise
        if message_id:
            it = self._cerca_per_message_id(message_id)
            if it is not None:
                log.info("elemento ritrovato per Message-ID in %s", it.Parent.Name)
                return it
        raise ErroreDefinitivo("elemento non trovato in nessuna cartella (eliminato?)")

    def _cerca_per_message_id(self, message_id: str):
        """DASL Find sulla proprietà PR_INTERNET_MESSAGE_ID, cartella per cartella (locale-indipendente)."""
        filtro = "@SQL=\"%s\" = '%s'" % (PR_INTERNET_MESSAGE_ID, message_id.replace("'", "''"))
        for cartella in self._tutte_le_cartelle():
            try:
                if cartella.DefaultItemType != 0:
                    continue
                it = cartella.Items.Find(filtro)
                if it is not None:
                    return it
            except pywintypes.com_error:
                continue
        return None

    def _tutte_le_cartelle(self):
        """Cartelle di posta dello store predefinito, in profondità (Inbox e Posta inviata per prime)."""
        radice = self.ns.GetDefaultFolder(6).Parent
        prime = []
        for idx in (6, 5, 3, 16):
            try:
                prime.append(self.ns.GetDefaultFolder(idx))
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
        """EntryID/StoreID/cartella effettivi dell'elemento: il server riallinea messaggio_outlook."""
        try:
            return {"entry_id": it.EntryID, "store_id": it.Parent.StoreID, "cartella": it.Parent.Name}
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
        nuovo = it.Move(self.cartella(cartella))
        return self.dove(nuovo)

    def crea_bozza(self, p: PayloadCreaBozza) -> tuple[str, bool]:
        """Prepara la mail e la lascia come bozza aperta in Outlook. Send() solo con consenti_invio."""
        if p.tipo == "nuovo":
            item = self.app.CreateItem(0)
        else:
            orig = self._item(p.entry_id, p.store_id, p.message_id)
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
