"""L2 — modulo comune dei worker (voce 0.4) contro il server finto: niente Outlook, niente DB."""
from __future__ import annotations

import logging
import os
import tempfile
import time

import pytest

from cockpit_client import (ArrestoRichiesto, Battito, Cockpit, ErroreHTTP, carica_config, configura_log,
                            nome_worker)
from server_finto import ServerFinto


def test_claim_restituisce_il_job_e_poi_niente():
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token)
        s.metti_job({"job_id": 7, "tipo": "sync_outlook", "payload": {}, "tentativi": 1, "lease_s": 120})
        assert api.claim("outlook", "outlook@PC", attesa_s=1)["job_id"] == 7
        assert api.claim("outlook", "outlook@PC", attesa_s=1) is None  # 204 → None, non un errore
        assert [c["worker"] for c in s.claim_fatti] == ["outlook", "outlook"]


def test_token_sbagliato_da_401_non_un_errore_di_rete():
    with ServerFinto() as s:
        api = Cockpit(s.url, "token-sbagliato")
        with pytest.raises(ErroreHTTP) as e:
            api.claim("outlook", "outlook@PC", attesa_s=1)
        assert e.value.stato == 401
        assert not e.value.ritentabile        # ritentare con lo stesso token non serve a nulla
        assert not e.value.tentativo_non_valido
        assert s.non_autorizzati == 1


def test_409_e_riconoscibile_e_5xx_e_ritentabile():
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token)
        s.stato_heartbeat = 409
        with pytest.raises(ErroreHTTP) as e:
            api.heartbeat(1, "outlook@PC", "t-1")
        assert e.value.tentativo_non_valido and not e.value.ritentabile

        s.stato_ingest = 503
        with pytest.raises(ErroreHTTP) as e:
            api.ingest({"messaggi": []})
        assert e.value.ritentabile and not e.value.tentativo_non_valido


def test_il_battito_manda_heartbeat_mentre_il_lavoro_gira():
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token)
        with Battito(api, 42, "outlook@PC", "tok-42", ogni_s=0.05) as b:
            time.sleep(0.35)
            b.controlla()                      # lease valido: non deve alzare nulla
        assert len(s.battiti) >= 3, f"attesi almeno 3 battiti, ricevuti {len(s.battiti)}"
        assert set(s.battiti) == {42}
        assert not b.arresto.is_set()
        # senza il token il server non potrebbe distinguere questo tentativo da uno scaduto
        assert {c.get("lease_token") for c in s.battiti_corpo} == {"tok-42"}


def test_il_risultato_porta_sempre_il_tentativo():
    """Il result senza tentativo sarebbe indistinguibile da quello di un tentativo già scaduto."""
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token, worker_id="outlook@PC")
        api.risultato(9, {"esito": "ok", "dati": {}}, "outlook@PC", "tok-9")
        assert s.risultati[9]["worker_id"] == "outlook@PC"
        assert s.risultati[9]["lease_token"] == "tok-9"


def test_il_risultato_rifiutato_con_409_e_riconoscibile():
    """Il worker deve poter distinguere «non ti ascolto più» da «riprova»."""
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token)
        s.stato_result = 409
        with pytest.raises(ErroreHTTP) as e:
            api.risultato(10, {"esito": "ok"}, "outlook@PC", "tok-vecchio")
        assert e.value.tentativo_non_valido and not e.value.ritentabile


def test_il_battito_chiede_arresto_dopo_un_409():
    """Q5/C16 (parte L2): il 409 non può essere ignorato, e chi lavora lo scopre al punto di ripresa."""
    with ServerFinto() as s:
        api = Cockpit(s.url, s.token)
        s.stato_heartbeat = 409
        with Battito(api, 43, "outlook@PC", "tok-43", ogni_s=0.05) as b:
            scadenza = time.time() + 3
            while not b.arresto.is_set() and time.time() < scadenza:
                time.sleep(0.02)
            assert b.arresto.is_set(), "il battito non ha alzato il flag di arresto dopo il 409"
            with pytest.raises(ArrestoRichiesto):
                b.controlla()
        assert len(s.battiti) == 1, "dopo un 409 il battito deve smettere, non insistere"


def test_il_battito_sopravvive_alla_rete_giu():
    """Un buco di rete non è la perdita del lease: il worker non deve buttare via il lavoro fatto."""
    with ServerFinto() as s:
        url_morto = s.url
    api = Cockpit(url_morto, "token-di-prova")   # server chiuso: connessione rifiutata
    with Battito(api, 44, "outlook@PC", "tok-44", ogni_s=0.05) as b:
        time.sleep(0.3)
        assert not b.arresto.is_set()
        b.controlla()


def test_carica_config_file_e_ambiente(tmp_path, monkeypatch):
    f = tmp_path / "worker.toml"
    f.write_text('server_url = "http://server-dal-file:8080"\ntoken = "dal-file"\nstaging = "."\n', encoding="utf-8")
    cfg = carica_config(str(f))
    assert cfg["server_url"] == "http://server-dal-file:8080" and cfg["token"] == "dal-file"

    monkeypatch.setenv("COCKPIT_URL", "http://da-ambiente:9090")
    monkeypatch.setenv("COCKPIT_TOKEN", "da-ambiente")
    cfg = carica_config(str(f))
    assert cfg["server_url"] == "http://da-ambiente:9090" and cfg["token"] == "da-ambiente"


def test_carica_config_senza_token_esce(tmp_path, monkeypatch):
    monkeypatch.delenv("COCKPIT_TOKEN", raising=False)
    with pytest.raises(SystemExit):
        carica_config(str(tmp_path / "assente.toml"))


def test_nome_worker_stabile_fra_riavvii():
    """Il nome è la chiave di worker_credenziale (fase 2): non può contenere il PID."""
    a, b = nome_worker("outlook"), nome_worker("outlook")
    assert a == b and a.startswith("outlook@") and "#" not in a
    assert nome_worker("analisi", {"worker_id": "analisi@PC-PROVA"}) == "analisi@PC-PROVA"


def test_configura_log_non_raddoppia_le_righe():
    with tempfile.TemporaryDirectory() as d:
        cfg = {"staging": d}
        configura_log(False, cfg, "prova")
        configura_log(False, cfg, "prova")
        logging.getLogger("prova").info("una riga sola")
        logging.shutdown()
        with open(os.path.join(d, "log", "prova.log"), encoding="utf-8") as f:
            righe = [r for r in f if "una riga sola" in r]
        assert len(righe) == 1, f"la riga è stata scritta {len(righe)} volte"
