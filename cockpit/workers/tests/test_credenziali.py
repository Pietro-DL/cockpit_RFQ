"""L1/L2 — voce 2.4 lato worker: un token per worker, e il certificato del server si riconosce.

Sullo stesso PC girano due worker — Outlook e analisi — e da questo blocco ognuno ha un token suo:
il server lo cerca per sha256 e da lì sa CHI sta chiamando, non soltanto che qualcuno conosce il
segreto. Un file di configurazione per PC con un token solo non basta più, e da qui nascono le due
tabelle `[outlook]` e `[analisi]`.

L'impronta del certificato vive nello stesso file per lo stesso motivo: sono le due metà della stessa
domanda — «con chi sto parlando» e «chi sono io». La verifica vera dell'impronta, contro un server
TLS acceso, è il test W11 in `internal/transport/workerapi` (fa girare questo client contro un certificato
generato al momento): qui si provano le regole che valgono prima di aprire la connessione.
"""
from __future__ import annotations

import pytest

from cockpit_client import (Cockpit, ErroreHTTP, ImprontaSbagliata, carica_config, diagnosi,
                            normalizza_impronta)

IMPRONTA = "ab" * 32


def scrivi(tmp_path, testo):
    f = tmp_path / "worker.toml"
    f.write_text(testo, encoding="utf-8")
    return str(f)


def pulisci(monkeypatch):
    for v in ("COCKPIT_URL", "COCKPIT_TOKEN", "COCKPIT_IMPRONTA", "COCKPIT_STAGING", "COCKPIT_WORKER_ID"):
        monkeypatch.delenv(v, raising=False)


def test_ogni_worker_legge_il_proprio_token(tmp_path, monkeypatch):
    pulisci(monkeypatch)
    f = scrivi(tmp_path, f'''
server_url = "https://cockpit.azienda:8443"
impronta = "{IMPRONTA}"
staging = "."

[outlook]
worker_id = "outlook@PC-A"
token = "segreto-di-outlook"

[analisi]
worker_id = "analisi@PC-A"
token = "segreto-di-analisi"
''')
    o = carica_config(f, sezione="outlook")
    a = carica_config(f, sezione="analisi")
    assert o["token"] == "segreto-di-outlook" and o["worker_id"] == "outlook@PC-A"
    assert a["token"] == "segreto-di-analisi" and a["worker_id"] == "analisi@PC-A"
    # le chiavi comuni restano comuni: si scrivono una volta sola
    assert o["server_url"] == a["server_url"] == "https://cockpit.azienda:8443"
    assert o["impronta"] == a["impronta"] == IMPRONTA


def test_il_token_dell_altro_worker_non_arriva_qui(tmp_path, monkeypatch):
    """Se le sezioni finissero tutte nella configurazione, `token` dipenderebbe dall'ordine nel file."""
    pulisci(monkeypatch)
    f = scrivi(tmp_path, '''
server_url = "http://127.0.0.1:8080"
staging = "."

[analisi]
token = "segreto-di-analisi"

[outlook]
token = "segreto-di-outlook"
''')
    cfg = carica_config(f, sezione="outlook")
    assert cfg["token"] == "segreto-di-outlook"
    assert "analisi" not in cfg and "outlook" not in cfg


def test_un_worker_toml_vecchio_si_legge_ancora(tmp_path, monkeypatch):
    """Un file con il solo token in cima continua a essere letto.

    Non è indulgenza: è il server a dire che quel token è di due worker e non identifica nessuno, con
    parole che spiegano come si sistema. Un worker che si rifiuta di partire direbbe molto meno.
    """
    pulisci(monkeypatch)
    f = scrivi(tmp_path, 'server_url = "http://127.0.0.1:8080"\ntoken = "quello-di-prima"\nstaging = "."\n')
    assert carica_config(f, sezione="outlook")["token"] == "quello-di-prima"


def test_l_ambiente_vince_sul_file(tmp_path, monkeypatch):
    pulisci(monkeypatch)
    f = scrivi(tmp_path, 'token = "dal-file"\nstaging = "."\n\n[outlook]\ntoken = "dalla-sezione"\n')
    monkeypatch.setenv("COCKPIT_TOKEN", "dall-ambiente")
    assert carica_config(f, sezione="outlook")["token"] == "dall-ambiente"


def test_senza_token_il_worker_dice_dove_prenderlo(tmp_path, monkeypatch):
    pulisci(monkeypatch)
    f = scrivi(tmp_path, 'server_url = "http://127.0.0.1:8080"\nstaging = "."\n')
    with pytest.raises(SystemExit) as e:
        carica_config(f, sezione="outlook")
    assert "[outlook].token" in str(e.value) and "Postazioni" in str(e.value)


# ---------------------------------------------------------------- l'impronta

def test_un_impronta_si_copia_come_capita():
    """Con i due punti, in maiuscolo, con gli spazi: è sempre la stessa impronta."""
    a = normalizza_impronta("AB:CD:ef 01")
    assert a == "abcdef01"
    assert normalizza_impronta("") == "" and normalizza_impronta(None) == ""


def test_un_impronta_lunga_male_non_passa():
    # Mezza impronta incollata male non deve diventare «nessuna impronta»: sarebbe il modo silenzioso
    # di spegnere il controllo.
    with pytest.raises(ValueError):
        Cockpit("https://cockpit:8443", "t", impronta="ab" * 10)


def test_un_impronta_senza_https_non_ha_senso():
    with pytest.raises(ValueError):
        Cockpit("http://cockpit:8080", "t", impronta=IMPRONTA)


def test_senza_impronta_il_client_resta_quello_di_prima():
    api = Cockpit("http://127.0.0.1:8080", "t")
    assert api.impronta == ""


# ---------------------------------------------------------------- il log dice che cosa è successo

# Sul banco di prova, con le credenziali individuali appena introdotte, il worker scriveva
# «server non raggiungibile (POST /api/v1/jobs/claim → 401 …)» e continuava a riprovare. Il server
# rispondeva benissimo: stava dicendo che quel token era di due worker. Un messaggio che nomina la
# rete manda a cercare la rete, e quella diagnosi è costata tempo. Da qui questi test.

def diagnosi_di(stato, corpo="{}"):
    return diagnosi(ErroreHTTP("POST", "/api/v1/jobs/claim", stato, corpo))


def test_un_401_non_e_un_server_irraggiungibile():
    d = diagnosi_di(401, '{"errore":"questo token è di piu\' worker (a, b)"}')
    assert d.categoria == "credenziale"
    assert d.grave, "un token rifiutato non si risolve aspettando: va scritto come errore"
    assert "non raggiungibile" not in d.testo
    assert "401" in d.testo and "Postazioni" in d.testo
    # il corpo del server arriva nel log: è lì che c'è scritto CHI sono i due worker
    assert "di piu' worker" in d.testo
    assert d.pausa_s == 30, "un errore che non si risolve aspettando non deve raddoppiare l'attesa"


def test_un_403_parla_di_configurazione_non_di_rete():
    d = diagnosi_di(403, '{"errore":"worker non censito su questa postazione"}')
    assert d.categoria == "autorizzazione" and d.grave
    assert "non raggiungibile" not in d.testo and "cockpit.toml" in d.testo


def test_un_409_e_il_lease_perso_e_non_e_grave():
    d = diagnosi_di(409, '{"errore":"tentativo non valido"}')
    assert d.categoria == "tentativo"
    assert not d.grave, "perdere il lease è normale: il job torna in coda da solo"
    assert d.pausa_s == 0, "il 409 non impone un'attesa: il claim successivo è legittimo"


def test_un_5xx_e_del_server_e_si_ritenta():
    d = diagnosi_di(503, "manutenzione")
    assert d.categoria == "server" and not d.grave and d.pausa_s == 0


def test_un_4xx_qualunque_non_si_risolve_ripetendolo():
    d = diagnosi_di(400, '{"errore":"worker_id mancante"}')
    assert d.categoria == "richiesta" and d.grave


def test_un_errore_di_rete_resta_un_errore_di_rete():
    d = diagnosi(ConnectionRefusedError("[Errno 10061] rifiutata"))
    assert d.categoria == "rete" and not d.grave
    assert "non raggiungibile" in d.testo


def test_un_certificato_diverso_non_e_un_errore_di_rete():
    d = diagnosi(ImprontaSbagliata("il server ha presentato un certificato diverso"))
    assert d.categoria == "certificato" and d.grave
    assert "non raggiungibile" not in d.testo


def test_il_ciclo_del_worker_registra_il_401_come_errore(tmp_path, monkeypatch, caplog):
    """L2: non la funzione, il ciclo vero — è lì che la riga sbagliata veniva scritta."""
    import logging

    import worker_analisi

    attese: list[float] = []
    monkeypatch.setattr(worker_analisi.time, "sleep", lambda n: attese.append(n))
    w = worker_analisi.WorkerAnalisi({"server_url": "http://127.0.0.1:1", "token": "x", "staging": str(tmp_path)})

    giri = {"n": 0}

    def claim_401(*a, **k):
        giri["n"] += 1
        if giri["n"] > 2:
            raise KeyboardInterrupt
        raise ErroreHTTP("POST", "/api/v1/jobs/claim", 401, '{"errore":"credenziale non riconosciuta"}')

    w.api.claim = claim_401
    with caplog.at_level(logging.DEBUG):
        with pytest.raises(KeyboardInterrupt):
            w.esegui_per_sempre()
    righe = [r for r in caplog.records if r.levelno >= logging.WARNING]
    assert righe, "il 401 non è finito nel log"
    assert all(r.levelno == logging.ERROR for r in righe), "un 401 registrato come semplice avviso"
    testo = "\n".join(r.getMessage() for r in righe)
    assert "non raggiungibile" not in testo
    assert "401" in testo and "Postazioni" in testo
    assert attese == [30, 30], f"attesa fissa attesa [30,30], ottenuta {attese}"
