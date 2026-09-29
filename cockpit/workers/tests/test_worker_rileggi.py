"""L2 — Smistamento 4.13: il job `rileggi_elemento`, che il server accodava e il worker non sapeva fare.

«Riprova» su uno scarto di LETTURA (un elemento che il sync ha visto e non è riuscito a convertire)
accoda `rileggi_elemento` (ingest/replay.go: «lo rilegge da quella casella e lo rimanda in un lotto da
uno»). Il worker non aveva il ramo: il job finiva in «tipo job sconosciuto per il worker outlook»,
definitivo, e lo scarto non si chiudeva mai. Qui si prova il ramo:

  * l'elemento si ritrova per EntryID, e se è stato spostato per Message-ID dentro lo store della
    casella; si converte come lo converte il sync;
  * arriva al server in un lotto da uno, senza cursore e senza elementi saltati: una rilettura non dice
    niente della finestra da cui l'elemento viene, e non la deve muovere (la stessa regola il server la
    applica da sé: ingest, L4);
  * «non trovato» è un errore DEFINITIVO, e così un elemento che non è una mail; una casella che questo
    profilo non ha e una proprietà che COM oggi non dà no: un altro tentativo può farcela.

Il server è quello finto e Outlook è una cartella finta: si prova che cosa il worker cerca e che cosa
MANDA. Mittenti, oggetti e identificativi sono inventati.
"""
from __future__ import annotations

import re
from datetime import datetime, timezone

import pytest

from server_finto import ServerFinto

worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")
outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

import pywintypes                                                        # noqa: E402

from finti_outlook import (PR_INTERNET_MESSAGE_ID, ElementoNonMail, ElementoSordo, MailFinta,  # noqa: E402
                           com_error)

CASELLA = "11111111-1111-1111-1111-111111111111"
CASELLA_ALTRA = "22222222-2222-2222-2222-222222222222"
STORE = "STORE-COMMERCIALE-LOCALE"
CASELLE_SERVITE = [
    {"casella_id": CASELLA, "indirizzo": "commerciale@azienda.example", "nome": "Commerciale", "condivisa": True},
]
QUANDO = datetime(2026, 9, 15, 10, 0, tzinfo=timezone.utc)


class _ItemsConFind:
    """La collezione Items ridotta a `Find`, con il filtro DASL sul Message-ID che scrive
    `_cerca_per_message_id`: si legge il valore dal filtro e si confronta con la proprietà MAPI."""

    def __init__(self, elementi):
        self._elementi = elementi

    def Find(self, filtro):  # noqa: N802 (nome di Outlook)
        m = re.search(r"= '(.*)'$", filtro)
        assert m and PR_INTERNET_MESSAGE_ID in filtro, filtro
        cercato = m.group(1).replace("''", "'")
        for e in self._elementi:
            if e.PropertyAccessor.GetProperty(PR_INTERNET_MESSAGE_ID) == cercato:
                return e
        return None


class CartellaDiPosta:
    """Una cartella di posta dello store della casella: nome, store, e gli elementi che contiene."""

    def __init__(self, nome: str, elementi=(), entry_id: str = ""):
        self.Name = nome
        self.StoreID = STORE
        self.EntryID = entry_id or ("cartella-" + nome)
        self.DefaultItemType = 0
        self._elementi = list(elementi)
        for e in self._elementi:
            e.Parent = self

    @property
    def Items(self):  # noqa: N802
        return _ItemsConFind(self._elementi)


class NamespaceFinto:
    """GetItemFromID come in MAPI: l'elemento se l'EntryID è ancora quello, altrimenti MAPI_E_NOT_FOUND.
    `sordo` = un altro errore COM (lo store non risponde), che non è «non trovato»."""

    def __init__(self, per_entry_id: dict, sordo: bool = False):
        self.per_entry_id = per_entry_id
        self.sordo = sordo
        self.chiesti: list[tuple[str, str]] = []

    def GetItemFromID(self, entry_id, store_id=""):  # noqa: N802
        self.chiesti.append((entry_id, store_id))
        if self.sordo:
            raise com_error("lo store non risponde")
        if entry_id in self.per_entry_id:
            return self.per_entry_id[entry_id]
        raise pywintypes.com_error(outlook_com.MAPI_E_NOT_FOUND, "non trovato", None, None)


def _outlook(ns, cartelle):
    """L'adattatore senza COM, con il namespace e le cartelle dello store finti."""
    o = object.__new__(outlook_com.Outlook)
    o.usa_restrict = False
    o.autoprova_ore = 0
    o.restrict_ok = {}
    o.memoria = None
    o.postazione = "PC-PROVA"
    o.indirizzi_propri = set()
    o.ns = ns
    o._store = lambda store_id: None                # la Posta inviata non si risolve: il codice lo prevede
    o._tutte_le_cartelle = lambda store_id="": list(cartelle)
    # il profilo di questo PC ha la sola casella Commerciale
    o.risolvi_caselle = lambda caselle: ({CASELLA: STORE}, [])
    return o


def _mail(entry_id: str, message_id: str, oggetto: str = "RFQ 7120001") -> MailFinta:
    return MailFinta(entry_id, QUANDO, oggetto=oggetto, mittente="buyer@acme.example", message_id=message_id)


def _job(job_id: int = 40, entry_id: str = "E-VECCHIO", message_id: str = "<rfq-7120001@acme.example>",
         casella: str = CASELLA) -> dict:
    return {
        "job_id": job_id, "tipo": "rileggi_elemento", "tentativi": 1, "lease_s": 120,
        "lease_token": f"tok-{job_id}", "durata_max_s": 600, "casella_id": casella,
        "payload": {"casella_id": casella, "entry_id": entry_id, "cartella": "Posta in arrivo",
                    "message_id": message_id},
    }


def _esegui(s, tmp_path, monkeypatch, ol, job):
    s.caselle_worker = CASELLE_SERVITE
    s.metti_job(job)
    w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
    monkeypatch.setattr(w, "ol", lambda: ol)
    w.esegui_per_sempre(una_volta=True)
    return w


# ---------------------------------------------------------------- Outlook.rileggi

def test_si_rilegge_per_entry_id():
    mail = _mail("E-VECCHIO", "<rfq-7120001@acme.example>")
    arrivo = CartellaDiPosta("Posta in arrivo", [mail])
    ns = NamespaceFinto({"E-VECCHIO": mail})
    m = _outlook(ns, [arrivo]).rileggi("E-VECCHIO", STORE, "<rfq-7120001@acme.example>")
    assert ns.chiesti == [("E-VECCHIO", STORE)], "l'elemento si cerca nello store della casella"
    assert m.entry_id == "E-VECCHIO" and m.message_id == "<rfq-7120001@acme.example>"
    assert m.cartella == "Posta in arrivo" and m.store_id == STORE
    assert m.oggetto == "RFQ 7120001" and m.mittente_indirizzo == "buyer@acme.example"


def test_spostato_si_ritrova_per_message_id():
    """L'EntryID cambia quando l'elemento viene spostato dopo il sync: si ritrova per Message-ID, e il
    lotto porta l'EntryID e la cartella NUOVI (sono quelli con cui il server riallinea la presenza)."""
    spostata = _mail("E-NUOVO", "<rfq-7120001@acme.example>")
    archivio = CartellaDiPosta("Archivio ACME", [spostata])
    altra = CartellaDiPosta("Posta in arrivo", [_mail("E-ALTRO", "<altra@acme.example>")])
    m = _outlook(NamespaceFinto({}), [altra, archivio]).rileggi("E-VECCHIO", STORE, "<rfq-7120001@acme.example>")
    assert m.entry_id == "E-NUOVO"
    assert m.cartella == "Archivio ACME"
    assert m.message_id == "<rfq-7120001@acme.example>"


def test_non_trovato_e_un_errore_definitivo():
    arrivo = CartellaDiPosta("Posta in arrivo", [_mail("E-ALTRO", "<altra@acme.example>")])
    o = _outlook(NamespaceFinto({}), [arrivo])
    with pytest.raises(outlook_com.ErroreDefinitivo, match="non trovato"):
        o.rileggi("E-VECCHIO", STORE, "<rfq-7120001@acme.example>")
    # senza Message-ID non c'è una seconda strada: definitivo lo stesso
    with pytest.raises(outlook_com.ErroreDefinitivo, match="non trovato"):
        o.rileggi("E-VECCHIO", STORE, "")


def test_un_elemento_che_non_e_una_mail_e_definitivo():
    """Un rapporto di consegna non entrerà mai, per quante volte lo si rilegga."""
    rapporto = ElementoNonMail("E-RAPPORTO")
    o = _outlook(NamespaceFinto({"E-RAPPORTO": rapporto}), [])
    with pytest.raises(outlook_com.ErroreDefinitivo, match="Class=46"):
        o.rileggi("E-RAPPORTO", STORE, "")


def test_uno_store_che_non_risponde_non_e_non_trovato():
    """Un errore COM diverso da MAPI_E_NOT_FOUND non è «non trovato»: si ritenta."""
    o = _outlook(NamespaceFinto({}, sordo=True), [])
    with pytest.raises(pywintypes.com_error):
        o.rileggi("E-VECCHIO", STORE, "<rfq-7120001@acme.example>")


# ---------------------------------------------------------------- il job, dal claim al result

def test_il_job_consegna_un_lotto_da_uno_senza_cursore(tmp_path, monkeypatch):
    spostata = _mail("E-NUOVO", "<rfq-7120001@acme.example>")
    ol = _outlook(NamespaceFinto({}), [CartellaDiPosta("Archivio ACME", [spostata])])
    with ServerFinto() as s:
        _esegui(s, tmp_path, monkeypatch, ol, _job())

        assert len(s.lotti) == 1, s.lotti
        lotto = s.lotti[0]
        assert [m["message_id"] for m in lotto["messaggi"]] == ["<rfq-7120001@acme.example>"]
        assert lotto["messaggi"][0]["entry_id"] == "E-NUOVO"
        assert lotto["casella_id"] == CASELLA
        assert lotto["job_id"] == 40 and lotto["lease_token"] == "tok-40"
        assert lotto.get("cursore") is None, "una rilettura non muove la finestra: niente cursore"
        assert (lotto.get("saltati") or []) == []
        r = s.risultati[40]
        assert r["esito"] == "ok", r
        assert r["dati"] == {"entry_id": "E-NUOVO", "cartella": "Archivio ACME"}


def test_il_job_di_un_elemento_sparito_e_definitivo_e_non_manda_niente(tmp_path, monkeypatch):
    ol = _outlook(NamespaceFinto({}), [CartellaDiPosta("Posta in arrivo")])
    with ServerFinto() as s:
        _esegui(s, tmp_path, monkeypatch, ol, _job(job_id=41))

        assert s.lotti == []
        r = s.risultati[41]
        assert r["esito"] == "errore" and r["definitivo"] is True, r
        assert "non trovato" in r["errore"]
        assert "sconosciuto" not in r["errore"], "il worker non conosce ancora il tipo di job"


def test_una_proprieta_illeggibile_oggi_si_ritenta(tmp_path, monkeypatch):
    """È proprio il caso dello scarto di lettura: se COM oggi non dà la proprietà, il job fallisce
    SENZA «definitivo» e il server lo ritenta."""
    sordo = ElementoSordo("E-VECCHIO", QUANDO)
    CartellaDiPosta("Posta in arrivo", [sordo])
    ol = _outlook(NamespaceFinto({"E-VECCHIO": sordo}), [])
    with ServerFinto() as s:
        _esegui(s, tmp_path, monkeypatch, ol, _job(job_id=42))

        assert s.lotti == []
        r = s.risultati[42]
        assert r["esito"] == "errore" and not r.get("definitivo"), r


def test_una_casella_che_questo_profilo_non_ha_non_e_definitivo(tmp_path, monkeypatch):
    ns = NamespaceFinto({})
    ol = _outlook(ns, [])
    with ServerFinto() as s:
        _esegui(s, tmp_path, monkeypatch, ol, _job(job_id=43, casella=CASELLA_ALTRA))

        assert ns.chiesti == [], "ha cercato l'elemento in uno store che non è della casella"
        assert s.lotti == []
        r = s.risultati[43]
        assert r["esito"] == "errore" and r["definitivo"] is False, r
