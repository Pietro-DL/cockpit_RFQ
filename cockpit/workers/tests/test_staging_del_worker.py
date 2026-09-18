# -*- coding: utf-8 -*-
"""Blocco 4C: il worker non lavora dentro il magazzino dei contenuti del server.

Sul banco reale `worker.toml` aveva:

    staging = 'C:\\...\\_staging\\_contenuti'

cioè la cartella in cui il server tiene i contenuti indirizzati per sha256. Nessun errore da nessuna
parte: il worker ci scriveva `restrict.json`, `log\\` e `tmp\\`, il server ci teneva i contenuti, e i
due lavoravano sopra gli stessi file. Il giorno in cui la prova sul NAS è arrivata alla copia, i
contenuti che due documenti confermati aspettavano non c'erano più.

Adesso è un rifiuto all'avvio e non un avviso: un avviso in mezzo al log d'avvio di un worker non lo
legge nessuno, e il danno si vede settimane dopo, quando il file serve.
"""
import os

import pytest

from cockpit_client import carica_config


def scrivi(tmp_path, staging):
    f = tmp_path / "worker.toml"
    # stringa TOML LETTERALE (apici singoli): in un percorso di Windows le barre non si raddoppiano,
    # e con %r di Python ci finirebbero — il file letto direbbe un percorso che non esiste.
    f.write_text('token = "t"\nstaging = ' + "'" + str(staging) + "'\n", encoding="utf-8")
    return str(f)


@pytest.mark.parametrize("dentro", ["_contenuti", "_parti"])
def test_lo_staging_dentro_le_cartelle_del_server_e_rifiutato(tmp_path, dentro):
    percorso = scrivi(tmp_path, tmp_path / "_staging" / dentro)
    with pytest.raises(SystemExit) as e:
        carica_config(percorso)
    messaggio = str(e.value)
    assert dentro in messaggio, "il rifiuto non dice quale cartella ha riconosciuto"
    assert "worker.toml" in messaggio, "il rifiuto non dice quale file correggere"


def test_anche_una_sottocartella_e_rifiutata(tmp_path):
    """`_contenuti\\ab` è una cartella di contenuti come le altre."""
    percorso = scrivi(tmp_path, tmp_path / "_staging" / "_contenuti" / "ab")
    with pytest.raises(SystemExit):
        carica_config(percorso)


def test_una_cartella_del_worker_va_bene(tmp_path):
    suo = tmp_path / "_staging_worker"
    cfg = carica_config(scrivi(tmp_path, suo))
    assert cfg["staging"] == str(suo)


def test_il_nome_non_basta_se_non_e_un_pezzo_di_percorso(tmp_path):
    """Una cartella che si chiama «contenuti_vecchi» non è `_contenuti`: il controllo guarda i
    segmenti del percorso, non una sottostringa. Un controllo troppo largo si disattiva da solo."""
    suo = tmp_path / "contenuti_vecchi"
    cfg = carica_config(scrivi(tmp_path, suo))
    assert cfg["staging"] == str(suo)


def test_vale_anche_per_la_variabile_di_ambiente(tmp_path, monkeypatch):
    """COCKPIT_STAGING vince sul file: il controllo deve venire dopo, non prima."""
    monkeypatch.setenv("COCKPIT_STAGING", str(tmp_path / "_staging" / "_contenuti"))
    with pytest.raises(SystemExit):
        carica_config(scrivi(tmp_path, tmp_path / "va_bene"))
    monkeypatch.delenv("COCKPIT_STAGING")


def test_il_percorso_relativo_viene_risolto(tmp_path, monkeypatch):
    """`..\\_staging\\_contenuti` è lo stesso posto scritto in un altro modo."""
    dentro = tmp_path / "_staging" / "_contenuti"
    os.makedirs(dentro, exist_ok=True)
    monkeypatch.chdir(tmp_path / "_staging")
    with pytest.raises(SystemExit):
        carica_config(scrivi(tmp_path, os.path.join(".", "_contenuti")))



def test_la_cartella_tmp_si_svuota_all_avvio_e_il_resto_no(tmp_path):
    """Pre-7: `tmp\\` e' di passaggio, e cio' che ci resta e' di un tentativo morto in mezzo. Si svuota
    all'avvio. `restrict.json` e `log\\` sono stato e log del worker: non si toccano."""
    worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")
    from server_finto import ServerFinto

    resto = tmp_path / "tmp" / "123"
    resto.mkdir(parents=True)
    (resto / "disegno.pdf").write_bytes(b"meta' di un allegato")
    (tmp_path / "tmp" / "sciolto.bin").write_bytes(b"x")
    (tmp_path / "restrict.json").write_text("{}", encoding="utf-8")
    (tmp_path / "log").mkdir()
    (tmp_path / "log" / "worker.log").write_text("riga", encoding="utf-8")

    with ServerFinto() as s:
        worker_outlook.Worker(s.config(staging=str(tmp_path)))

    assert not resto.exists(), "la cartella di un tentativo morto e' ancora li'"
    assert not (tmp_path / "tmp" / "sciolto.bin").exists()
    assert (tmp_path / "restrict.json").exists(), "lo stato del self-test e' stato cancellato"
    assert (tmp_path / "log" / "worker.log").exists(), "il log e' stato cancellato"


def test_senza_tmp_l_avvio_non_si_lamenta(tmp_path):
    worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")
    from server_finto import ServerFinto

    with ServerFinto() as s:
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
    assert w.svuota_tmp() == 0
