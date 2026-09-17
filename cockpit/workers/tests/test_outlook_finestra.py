"""L1/L2 — voce 2.9: la finestra temporale con `Restrict` in UTC e il self-test per insieme.

Che cosa questi test provano e che cosa no.

PROVANO che il filtro viene scritto in UTC e nel formato DASL (C2 in miniatura), che il confronto
per insieme distingue un messaggio perso da uno letto due volte (C3), e — la parte che conta — che
un `Restrict` che perde qualcosa viene SMASCHERATO dal self-test e messo da parte, con la scansione
lineare che continua a consegnare tutti i messaggi della finestra.

NON PROVANO che Outlook si comporti come la cartella finta di questo file. Quello è C2/C3 sul
profilo vero (L5), e si esegue con `python worker_outlook.py --restrict`. Qui Outlook non c'è: c'è
una cartella che INTERPRETA il filtro come lo interpreterebbe Outlook, cioè in UTC. È proprio questo
a rendere il test capace di bocciare l'errore che conta: se il codice scrivesse l'ora locale nel
filtro, la cartella finta restituirebbe l'insieme sbagliato e il confronto se ne accorgerebbe.

La cartella finta sta in `finti_outlook.py`, perché la usano anche i test del confine COM.
"""
from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

from finti_outlook import CartellaFinta, ElementoFinto   # noqa: E402  (dopo l'importorskip: serve pywin32)

filtro_finestra = outlook_com.filtro_finestra
confronta_insiemi = outlook_com.confronta_insiemi

ROMA = timezone(timedelta(hours=2))          # ora legale italiana: +02:00


def _outlook(usa_restrict=True, autoprova_ore=0, memoria=None, postazione="BANCO"):
    """Un adattatore senza COM: qui interessano solo le funzioni della finestra."""
    o = object.__new__(outlook_com.Outlook)
    o.usa_restrict = usa_restrict
    o.autoprova_ore = autoprova_ore
    o.restrict_ok = {}
    o.memoria = memoria
    o.postazione = postazione
    return o


def _posta(n: int, base: datetime) -> list:
    return [ElementoFinto(f"E{i:03d}", base + timedelta(hours=i)) for i in range(n)]


# ---------------------------------------------------------------- il filtro (C2 in miniatura)

def test_il_filtro_e_dasl_e_in_utc():
    dal = datetime(2026, 9, 15, 12, 0, 30, tzinfo=ROMA)      # = 10:00:30 UTC
    f = filtro_finestra(dal)
    assert f.startswith("@SQL=")
    assert outlook_com.URN_RICEVUTA in f
    assert "'2026-09-15 10:00'" in f, f
    assert "12:00" not in f, "l'ora locale è finita nel filtro: su un PC con un altro fuso leggerebbe la finestra sbagliata"


def test_il_filtro_allarga_al_minuto_e_non_stringe():
    """L'arrotondamento deve andare sempre verso il «leggo qualcosa in più», mai verso il «ne perdo uno»."""
    dal = datetime(2026, 9, 15, 10, 0, 45, tzinfo=timezone.utc)
    al = datetime(2026, 9, 15, 11, 30, 15, tzinfo=timezone.utc)
    f = filtro_finestra(dal, al)
    assert "'2026-09-15 10:00'" in f, "il limite inferiore va arrotondato in giù"
    assert "'2026-09-15 11:31'" in f, "il limite superiore va arrotondato in su"


def test_una_data_senza_fuso_vale_come_utc():
    f = filtro_finestra(datetime(2026, 9, 15, 10, 0))
    assert "'2026-09-15 10:00'" in f


def test_senza_limite_superiore_il_filtro_ha_una_sola_condizione():
    f = filtro_finestra(datetime(2026, 9, 15, 10, 0, tzinfo=timezone.utc))
    assert "<=" not in f and ">=" in f


# ---------------------------------------------------------------- il confronto per insieme (C3)

def test_il_confronto_distingue_chi_manca_da_chi_e_di_troppo():
    ok, mancanti, in_piu = confronta_insiemi({"a", "b"}, {"a", "b"})
    assert ok and not mancanti and not in_piu

    # un elemento che il filtro non vede: è il caso grave, e deve bocciare
    ok, mancanti, in_piu = confronta_insiemi({"a"}, {"a", "b"})
    assert not ok and mancanti == ["b"]

    # un elemento in più ai bordi: si segnala, non boccia (il server deduplica per Message-ID)
    ok, mancanti, in_piu = confronta_insiemi({"a", "b", "c"}, {"a", "b"})
    assert ok and in_piu == ["c"]


# ---------------------------------------------------------------- self-test e ripiego

def test_un_restrict_onesto_passa_la_prova_e_viene_usato():
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    cart = CartellaFinta(_posta(30, base))
    o = _outlook()
    dal = base - timedelta(hours=1)

    assert o.autoprova_restrict(cart, dal, None) is True
    assert o._restrict_affidabile(cart, dal, None) is True
    assert o.restrict_ok == {("store-finto", "cartella-finta"): True}

    letti = [e.EntryID for e in o._elementi(cart, dal, None)]
    assert len(letti) == 30


def test_un_restrict_che_perde_un_messaggio_viene_smascherato_e_non_si_usa():
    """Il caso per cui il self-test esiste: il filtro mente, e nessuno se ne accorgerebbe mai.

    Un messaggio che `Restrict` non restituisce non entra nel Cockpit e non lascia nessuna traccia:
    non c'è un errore, non c'è un conteggio sbagliato, c'è una mail che non esiste. Dopo la prova il
    filtro resta spento su quella cartella e la scansione lineare li consegna tutti e trenta.
    """
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    cart = CartellaFinta(_posta(30, base), perde={"E007"})
    o = _outlook()
    dal = base - timedelta(hours=1)

    assert o.autoprova_restrict(cart, dal, None) is False
    assert o._restrict_affidabile(cart, dal, None) is False

    letti = [e.EntryID for e in o._elementi(cart, dal, None)]
    assert "E007" in letti, "il messaggio che il filtro perdeva non è stato letto nemmeno dal ripiego"
    assert len(letti) == 30


def test_una_finestra_vuota_non_conta_come_prova_superata():
    """Due insiemi vuoti sono uguali e non dimostrano niente: ricordarsi un «passata» così sarebbe peggio."""
    cart = CartellaFinta([])
    o = _outlook()
    dal = datetime(2026, 9, 1, tzinfo=timezone.utc)
    assert o.autoprova_restrict(cart, dal, None) is None
    assert o._restrict_affidabile(cart, dal, None) is False
    assert o.restrict_ok == {}, "un esito non concludente non va memorizzato: si riproverà"


def test_gli_elementi_arrivano_dal_piu_recente_con_tutti_e_due_i_modi():
    """Blocco 3: si legge dal piu’ RECENTE al piu’ vecchio, ed e’ l'inverso di prima.

    Prima l'ordine era crescente perche’ il cursore avanzava insieme ai lotti: in quel modo
    un'interruzione lasciava fuori solo la parte nuova della finestra. Adesso la sicurezza non viene
    piu’ dall'ordine — nessuna frontiera si muove prima che la finestra sia conclusa — e l'ordine puo’
    essere quello in cui la posta serve: chi apre l'Inbox dopo una notte vuole vedere le 09:00, non
    le 17:05 di ieri.

    I DUE modi devono darlo uguale. Se il ripiego lineare tornasse crescente, una cartella su cui
    Restrict non e’ attendibile mostrerebbe la posta al contrario senza che nessuno se ne accorga.
    """
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    dal = base - timedelta(hours=1)
    for perde in (None, {"E003"}):                     # con Restrict e con il ripiego
        cart = CartellaFinta(_posta(10, base), perde=perde)
        o = _outlook()
        tempi = [e.utc for e in o._elementi(cart, dal, None)]
        assert tempi == sorted(tempi, reverse=True), f"ordine non decrescente (perde={perde})"


def test_la_finestra_con_limite_superiore_esclude_il_futuro():
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    cart = CartellaFinta(_posta(10, base))
    o = _outlook()
    al = base + timedelta(hours=4, minutes=30)
    letti = [e.EntryID for e in o._elementi(cart, base - timedelta(hours=1), al)]
    assert letti == ["E004", "E003", "E002", "E001", "E000"], letti


def test_usa_restrict_false_lascia_solo_la_scansione_lineare():
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    cart = CartellaFinta(_posta(5, base))
    o = _outlook(usa_restrict=False)
    letti = [e.EntryID for e in o._elementi(cart, base - timedelta(hours=1), None)]
    assert len(letti) == 5
    assert o.restrict_ok == {}, "con usa_restrict=false non si deve nemmeno provare il filtro"


def test_la_misura_riporta_i_due_tempi_e_il_filtro_usato():
    base = datetime(2026, 9, 1, 8, 0, tzinfo=timezone.utc)
    cart = CartellaFinta(_posta(12, base))
    m = _outlook().misura_finestra(cart, base - timedelta(hours=1))
    assert m["disponibile"] and len(m["restrict"]) == 12 and len(m["lineare"]) == 12
    assert m["t_restrict"] >= 0 and m["t_lineare"] >= 0
    assert m["filtro"].startswith("@SQL=")


def test_la_prova_si_fa_sulla_coda_della_finestra_non_su_tutto_l_archivio():
    """`autoprova_ore` limita la prova alla coda della finestra: la scansione lineare su un archivio
    di anni costerebbe esattamente ciò che la voce 2.9 vuole evitare."""
    ora = datetime.now(timezone.utc)
    vecchi = [ElementoFinto(f"V{i}", ora - timedelta(days=300 + i)) for i in range(5)]
    nuovi = [ElementoFinto(f"N{i}", ora - timedelta(hours=i + 1)) for i in range(3)]
    cart = CartellaFinta(vecchi + nuovi)
    o = _outlook(autoprova_ore=7 * 24)
    assert o.autoprova_restrict(cart, ora - timedelta(days=400), None) is True
    # la prova ha guardato solo i tre recenti, ma la lettura vera copre tutto l'archivio
    letti = [e.EntryID for e in o._elementi(cart, ora - timedelta(days=400), None)]
    assert len(letti) == 8
