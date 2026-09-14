"""L2 — il ciclo dei due worker contro il server finto (accettazione della voce 0.4: `--una-volta`).

Nessuna chiamata COM e nessun PDF: si prova che il ciclo prende un job, riporta un risultato ed esce
quando la coda è vuota. Il comportamento di Outlook è materia di L5, quello dell'analisi di
test_worker_analisi.py.
"""
from __future__ import annotations

import pytest

from server_finto import ServerFinto

worker_analisi = pytest.importorskip("worker_analisi")
worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")


def test_outlook_una_volta_esce_con_la_coda_vuota(tmp_path):
    with ServerFinto() as s:
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)          # 204 → esce subito, senza aprire Outlook
        assert len(s.claim_fatti) == 1
        assert s.claim_fatti[0]["worker"] == "outlook"
        assert s.claim_fatti[0]["worker_id"].startswith("outlook@")
        assert w.outlook is None, "il worker ha aperto Outlook pur non avendo job"


def test_analisi_una_volta_esce_con_la_coda_vuota(tmp_path):
    with ServerFinto() as s:
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert len(s.claim_fatti) == 1 and s.claim_fatti[0]["worker"] == "analisi"


def test_un_job_che_fallisce_viene_riportato_non_taciuto(tmp_path):
    """Un errore del worker deve arrivare al server: è la differenza fra un job fallito e un job appeso."""
    with ServerFinto() as s:
        s.metti_job({"job_id": 11, "tipo": "tipo_inesistente", "payload": {}, "tentativi": 1, "lease_s": 120})
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert 11 in s.risultati, "il worker non ha riportato nulla"
        assert s.risultati[11]["esito"] == "errore"
        assert "tipo_inesistente" in s.risultati[11]["errore"]


def test_file_mancante_in_staging_e_definitivo(tmp_path):
    """Ritentare cinque volte un file che non c'è è tempo perso: l'errore è definitivo con rimedio."""
    with ServerFinto() as s:
        s.metti_job({
            "job_id": 12, "tipo": "analizza_allegato", "tentativi": 1, "lease_s": 120,
            "payload": {
                "allegato_id": "00000000-0000-0000-0000-000000000001",
                "messaggio_id": "00000000-0000-0000-0000-000000000002",
                "sha256": "0" * 64,
                "path_staging": str(tmp_path / "non-esiste.pdf"),
                "nome_file": "non-esiste.pdf",
            },
        })
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert s.risultati[12]["esito"] == "errore"
        assert s.risultati[12]["definitivo"] is True
        assert "Riscarica" in s.risultati[12]["errore"]


def test_server_giu_al_claim_non_uccide_il_worker(tmp_path, monkeypatch):
    """Se cockpit.exe si riavvia il worker aspetta e riprova: non termina e non perde il ciclo."""
    with ServerFinto() as s:
        url_morto = s.url
    attese: list[float] = []
    monkeypatch.setattr(worker_analisi.time, "sleep", lambda n: attese.append(n))
    w = worker_analisi.WorkerAnalisi({"server_url": url_morto, "token": "x", "staging": str(tmp_path)})

    tentativi = {"n": 0}
    claim_vero = w.api.claim

    def claim_finito(*a, **k):
        tentativi["n"] += 1
        if tentativi["n"] > 3:
            raise KeyboardInterrupt  # ci basta sapere che ha ritentato, poi usciamo dal ciclo
        return claim_vero(*a, **k)

    w.api.claim = claim_finito
    with pytest.raises(KeyboardInterrupt):
        w.esegui_per_sempre(una_volta=True)
    assert tentativi["n"] == 4, "il worker ha smesso di riprovare"
    assert attese == [5, 10, 20], f"attesa esponenziale attesa [5,10,20], ottenuta {attese}"
