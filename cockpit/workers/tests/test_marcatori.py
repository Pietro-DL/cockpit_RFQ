"""L2 — blocco 7B: i marcatori `Cockpit*` viaggiano sulla mail.

La bozza creata dal Cockpit porta `CockpitBozza` (sempre) e i marcatori del payload
(`CockpitRichiestaFornitore`); quando la mail parte, le UserProperties restano attaccate e il sync
della Posta inviata le rilegge. È così che il server lega la mail inviata alla richiesta che l'ha
generata senza indovinare dall'oggetto (IB2).

Che cosa provano: la lettura di un elemento con e senza marcatori, l'elemento che alza sulle
UserProperties (nessun marcatore, nessun errore: un sync fermato costa più di una conferma a mano),
la scrittura sulla bozza «nuovo», e che solo i nomi `Cockpit*` passano.

Che cosa NON provano: che Outlook conservi le UserProperties dopo Send(). Quello è L5 (IB2 sul banco).
"""
from __future__ import annotations

from datetime import datetime, timezone
from uuid import UUID

import pytest

outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

from contratti import PayloadCreaBozza  # noqa: E402
from finti_outlook import MailFinta, com_error  # noqa: E402

BASE = datetime(2026, 9, 18, 8, 0, tzinfo=timezone.utc)


class _Prop:
    def __init__(self, nome, valore=None):
        self.Name, self.Value = nome, valore


class _UserProperties:
    """Le UserProperties di Outlook: 1-based, Find per nome, Add(nome, tipo, campi_cartella)."""

    def __init__(self, props=()):
        self._p = list(props)

    @property
    def Count(self):
        return len(self._p)

    def Item(self, i):
        return self._p[i - 1]

    def Find(self, nome):
        for p in self._p:
            if p.Name == nome:
                return p
        return None

    def Add(self, nome, tipo, campi_cartella=False):
        p = _Prop(nome)
        self._p.append(p)
        return p


class _UserPropertiesRotte:
    @property
    def Count(self):
        raise com_error("UserProperties non disponibili")


def test_i_marcatori_cockpit_si_leggono_e_gli_altri_no():
    it = MailFinta("E001", BASE)
    it.UserProperties = _UserProperties([_Prop("CockpitBozza", "11111111-1111-1111-1111-111111111111"),
                                         _Prop("CockpitRichiestaFornitore", "22222222-2222-2222-2222-222222222222"),
                                         _Prop("Categoria interna", "x"), _Prop("CockpitVuoto", "")])
    assert outlook_com.marcatori_di(it) == {
        "CockpitBozza": "11111111-1111-1111-1111-111111111111",
        "CockpitRichiestaFornitore": "22222222-2222-2222-2222-222222222222",
    }


def test_senza_userproperties_o_con_com_rotto_nessun_marcatore_e_nessun_errore():
    assert outlook_com.marcatori_di(MailFinta("E002", BASE)) == {}
    it = MailFinta("E003", BASE)
    it.UserProperties = _UserPropertiesRotte()
    assert outlook_com.marcatori_di(it) == {}


def test_la_conversione_porta_i_marcatori_nel_messaggio_in():
    o = object.__new__(outlook_com.Outlook)
    o.indirizzi_propri = {"commerciale@azienda.example"}
    it = MailFinta("E004", BASE, mittente="commerciale@azienda.example")
    it.UserProperties = _UserProperties([_Prop("CockpitRichiestaFornitore", "22222222-2222-2222-2222-222222222222")])
    m = o._converti(it, "STORE", "Posta inviata", True)
    assert m.direzione == "uscita"
    assert m.marcatori == {"CockpitRichiestaFornitore": "22222222-2222-2222-2222-222222222222"}
    assert "marcatori" in m.model_dump(mode="json")


class _BozzaFinta:
    def __init__(self):
        self.Recipients = _Recipients()
        self.Subject = ""
        self.Body = ""
        self.HTMLBody = ""
        self.Attachments = _Attachments()
        self.UserProperties = _UserProperties()
        self.EntryID = "BOZZA-1"
        self.salvata = False

    def Display(self, modale):
        pass

    def Save(self):
        self.salvata = True

    def Send(self):
        raise AssertionError("Send() non deve essere chiamato")


class _Recipients(list):
    def Add(self, indirizzo):
        r = _Prop(indirizzo)
        r.Type = 1
        self.append(r)
        return r

    def ResolveAll(self):
        return True


class _Attachments:
    def Add(self, percorso):
        pass


def test_la_bozza_nuova_porta_cockpitbozza_e_i_marcatori_del_payload_solo_se_cockpit():
    o = object.__new__(outlook_com.Outlook)
    o.consenti_invio = False
    bozza = _BozzaFinta()

    class _App:
        def CreateItem(self, tipo):
            return bozza

    o.app = _App()
    p = PayloadCreaBozza(bozza_id=UUID("11111111-1111-1111-1111-111111111111"), tipo="nuovo",
                         destinatari=[{"nome": "Ordini", "indirizzo": "ordini@fornitore.example", "tipo": "a"}],
                         oggetto="RFQ ACME Rossi 1234567A", corpo_testo="Buongiorno,",
                         marcatori={"CockpitRichiestaFornitore": "22222222-2222-2222-2222-222222222222",
                                    "NonCockpit": "no", "CockpitVuoto": ""})
    entry_id, inviata = o.crea_bozza(p)
    assert entry_id == "BOZZA-1" and inviata is False and bozza.salvata
    scritti = {q.Name: q.Value for q in bozza.UserProperties._p}
    assert scritti == {"CockpitBozza": "11111111-1111-1111-1111-111111111111",
                       "CockpitRichiestaFornitore": "22222222-2222-2222-2222-222222222222"}
    assert bozza.Subject == "RFQ ACME Rossi 1234567A"
    assert [r.Name for r in bozza.Recipients] == ["ordini@fornitore.example"]


def test_un_marcatore_che_non_si_scrive_non_ferma_la_bozza():
    item = _BozzaFinta()
    item.UserProperties = _UserPropertiesRotte()
    outlook_com.scrivi_marcatori(item, {"CockpitBozza": "x"})  # non alza
