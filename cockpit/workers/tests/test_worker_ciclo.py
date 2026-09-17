"""L2 — il ciclo dei due worker contro il server finto (accettazione della voce 0.4: `--una-volta`).

Nessuna chiamata COM e nessun PDF: si prova che il ciclo prende un job, riporta un risultato ed esce
quando la coda è vuota. Il comportamento di Outlook è materia di L5, quello dell'analisi di
test_worker_analisi.py.
"""
from __future__ import annotations

import hashlib
import os
from datetime import datetime, timezone

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

# ---------------------------------------------------------------- sync: tentativo e cursore


CASELLA_FRANCESCO = "11111111-1111-1111-1111-111111111111"
CASELLA_COMMERCIALE = "33333333-3333-3333-3333-333333333333"
CASELLA_LUIGI = "44444444-4444-4444-4444-444444444444"

# Il profilo Outlook finto di questa postazione: tre store, come sul PC di prova reale (Francesco,
# Commerciale e un terzo — Filippo — che il Cockpit NON censisce e deve ignorare, M1).
PROFILO_FINTO = {
    "francesco@azienda.example": "STORE-FRANCESCO-LOCALE",
    "commerciale@azienda.example": "STORE-COMMERCIALE-LOCALE",
    "filippo@azienda.example": "STORE-FILIPPO-NON-CENSITO",
}

CASELLE_SERVITE = [
    {"casella_id": CASELLA_FRANCESCO, "indirizzo": "francesco@azienda.example", "nome": "Francesco", "condivisa": False},
    {"casella_id": CASELLA_COMMERCIALE, "indirizzo": "commerciale@azienda.example", "nome": "Commerciale", "condivisa": True},
]


class ProfiloFinto:
    """La parte dell'adattatore COM che risolve le caselle nel profilo locale (voce 2.6), finta:
    risponde solo per gli indirizzi che il server chiede, come farebbe Outlook con CreateRecipient."""

    def __init__(self, profilo: dict | None = None):
        self.profilo = dict(PROFILO_FINTO if profilo is None else profilo)
        self.chieste: list[str] = []      # indirizzi che il worker ha chiesto di risolvere

    def risolvi_caselle(self, caselle):
        trovate, mancanti = {}, []
        for c in caselle:
            self.chieste.append(c["indirizzo"])
            if c["indirizzo"] in self.profilo:
                trovate[str(c["casella_id"])] = self.profilo[c["indirizzo"]]
            else:
                mancanti.append(c.get("nome") or c["indirizzo"])
        return trovate, mancanti


class OutlookFinto(ProfiloFinto):
    """Sostituisce l'adattatore COM: restituisce messaggi già pronti, senza Outlook."""

    def __init__(self, messaggi, profilo: dict | None = None):
        super().__init__(profilo)
        self.messaggi = messaggi
        self.letture: list[tuple[str, str]] = []   # (cartella, store_id) di ogni leggi()
        self.finestre: dict[str, tuple] = {}       # cartella -> (dal, al): quale finestra è stata chiesta

    def leggi(self, cartella, dal, al=None, store_id="", saltati=None):
        self.letture.append((cartella, store_id))
        self.finestre[cartella] = (dal, al)
        self.saltati_visti = saltati
        for m in self.messaggi:
            yield m


def _messaggio(n: int):
    from contratti import MessaggioIn
    return MessaggioIn(
        message_id=f"<m{n}@prova>", entry_id=f"E{n}", store_id="S", cartella="Inbox",
        direzione="entrata", data_evento=datetime(2026, 9, 1, 10, n, tzinfo=timezone.utc),
        oggetto=f"prova {n}",
    )


def _job_sync(lotto: int = 2) -> dict:
    return {
        "job_id": 20, "tipo": "sync_outlook", "tentativi": 1, "lease_s": 120,
        "lease_token": "tok-20", "durata_max_s": 1800,
        "payload": {
            "casella_id": "11111111-1111-1111-1111-111111111111",
            "cartelle": [{"cartella": "Inbox", "ultimo_received": None}],
            "dal": "2026-09-01T00:00:00Z", "sovrapposizione_s": 600, "lotto": lotto,
        },
    }


def test_il_lotto_porta_il_tentativo_e_il_cursore(tmp_path, monkeypatch):
    """P5: senza tentativo nel lotto il server non può distinguere un worker vivo da uno scaduto.

    E il cursore viaggia con il lotto, non dopo: il server li scrive nella stessa transazione.
    """
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync(lotto=2))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([_messaggio(1), _messaggio(2), _messaggio(3)])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        # voce 2.6: la cartella letta è quella dello STORE della casella del job, non del profilo
        assert finto.letture == [("Inbox", "STORE-FRANCESCO-LOCALE")], finto.letture

        assert len(s.lotti) == 2, f"attesi 2 lotti da 2+1, ricevuti {len(s.lotti)}"
        primo = s.lotti[0]
        assert primo["job_id"] == 20
        assert primo["lease_token"] == "tok-20"
        assert primo["worker_id"].startswith("outlook@")
        assert primo["casella_id"] == "11111111-1111-1111-1111-111111111111"
        assert primo["cursore"]["cartella"] == "Inbox"
        # il cursore del primo lotto è l'ultimo messaggio DI QUEL LOTTO, non dell'intera scansione
        assert primo["cursore"]["ultimo_received"].startswith("2026-09-01T10:02")
        assert s.lotti[1]["cursore"]["ultimo_received"].startswith("2026-09-01T10:03")
        assert s.risultati[20]["esito"] == "ok"


def test_un_409_sull_ingest_ferma_la_scansione(tmp_path, monkeypatch):
    """Se il tentativo non vale più, continuare a mandare lotti significa scrivere sopra al lavoro
    del tentativo che è subentrato: il worker si ferma al primo rifiuto e NON riporta niente.

    L'asserzione sul risultato è cambiata il 15/09 insieme a C16, e vale la pena dire perché: prima il
    worker, dopo il 409, mandava comunque un `/result` con esito "errore". È esattamente ciò che Q19
    vieta — un tentativo scaduto che fa fallire il job di un altro — e il server lo respinge con un
    altro 409. Non riportare niente non è un'omissione: è la conseguenza del fatto che il job, lato
    server, appartiene già a qualcun altro.
    """
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.stato_ingest = 409
        s.metti_job(_job_sync(lotto=2))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        monkeypatch.setattr(w, "ol", lambda: OutlookFinto([_messaggio(i) for i in range(1, 7)]))
        w.esegui_per_sempre(una_volta=True)

        assert len(s.lotti) == 1, f"dopo un 409 il worker ha continuato a mandare ({len(s.lotti)} lotti)"
        assert 20 not in s.risultati, f"il worker ha riportato un risultato di un tentativo non più valido: {s.risultati.get(20)}"


# ---------------------------------------------------------------- la finestra: cursore o ripiego

def _job_sync_due_cartelle(cursore_inbox, dal, al=None) -> dict:
    j = _job_sync(lotto=50)
    j["payload"]["cartelle"] = [
        {"cartella": "Inbox", "ultimo_received": cursore_inbox},
        {"cartella": "Sent Items", "ultimo_received": None},
    ]
    j["payload"]["dal"] = dal
    if al is not None:
        j["payload"]["al"] = al
    return j


def test_il_cursore_di_una_cartella_vince_sul_limite_del_payload(tmp_path, monkeypatch):
    """SI3 dal lato del worker: è QUI che la precedenza si applica davvero.

    Il server manda, per ogni cartella, il suo cursore (o niente) e un limite inferiore di ripiego —
    la finestra iniziale di `giorni_sync_iniziale`. Una cartella che ha un cursore riparte da lì,
    meno la sovrapposizione; una che non ce l'ha usa il ripiego. Se il worker prendesse `dal` per
    tutte, ogni sync rileggerebbe la finestra iniziale da capo: nessun errore, nessun buco, solo il
    worker occupato per niente a ogni giro — e la deduplica per Message-ID a nascondere il sintomo.
    """
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(cursore_inbox="2026-09-15T08:00:00Z", dal="2026-09-09T00:00:00Z"))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

    assert set(finto.finestre) == {"Inbox", "Sent Items"}, finto.finestre
    # il cursore, meno i 600 s di sovrapposizione dichiarati nel payload
    assert finto.finestre["Inbox"][0] == datetime(2026, 9, 15, 7, 50, tzinfo=timezone.utc), finto.finestre["Inbox"]
    # nessun cursore: il ripiego del payload, cioè la finestra iniziale calcolata dal server
    assert finto.finestre["Sent Items"][0] == datetime(2026, 9, 9, 0, 0, tzinfo=timezone.utc), finto.finestre["Sent Items"]
    # il sync ordinario non ha limite superiore: legge fino a adesso
    assert [al for _dal, al in finto.finestre.values()] == [None, None]


def test_un_sync_storico_non_sposta_il_cursore_in_avanti(tmp_path, monkeypatch):
    """«Carica precedenti» legge una finestra CHIUSA nel passato: farle muovere il cursore
    significherebbe dichiarare letto fino a due giorni fa tutto ciò che sta in mezzo, e la posta
    arrivata nel frattempo non la rileggerebbe più nessuno."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(cursore_inbox=None, dal="2026-09-12T00:00:00Z",
                                           al="2026-09-14T00:00:00Z"))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([_messaggio(1), _messaggio(2)])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        assert finto.finestre["Inbox"] == (datetime(2026, 9, 12, tzinfo=timezone.utc),
                                           datetime(2026, 9, 14, tzinfo=timezone.utc))
        assert s.risultati[20]["esito"] == "ok"
        for c in s.risultati[20]["dati"]["cartelle"]:
            assert c["ultimo_received"] is None, f"{c['cartella']}: lo storico ha mosso il cursore a {c['ultimo_received']}"
        for lotto in s.lotti:
            assert lotto.get("cursore") is None, f"lo storico ha mandato un cursore: {lotto['cursore']}"


# ---------------------------------------------------------------- download di un allegato: upload al server (voce 2.3)


class OutlookFintoAllegato(ProfiloFinto):
    """Sostituisce l'adattatore COM per il solo salva_allegato: scrive un file nella cartella chiesta."""

    def __init__(self, contenuto: bytes):
        super().__init__()
        self.contenuto = contenuto
        self.salvati: list[str] = []
        self.store_usati: list[str] = []

    def salva_allegato(self, entry_id, store_id, indice, nome_file, cartella_staging, message_id=""):
        self.store_usati.append(store_id)
        os.makedirs(cartella_staging, exist_ok=True)
        dest = os.path.join(cartella_staging, f"{indice:02d}_{nome_file}")
        with open(dest, "wb") as f:
            f.write(self.contenuto)
        self.salvati.append(dest)
        return dest, hashlib.sha256(self.contenuto).hexdigest(), len(self.contenuto), {"entry_id": entry_id, "cartella": "Posta in arrivo"}


def _job_stage() -> dict:
    return {
        "job_id": 30, "tipo": "stage_allegato", "tentativi": 1, "lease_s": 120,
        "lease_token": "tok-30", "durata_max_s": 600,
        "casella_id": CASELLA_COMMERCIALE,
        "payload": {
            "allegato_id": "22222222-2222-2222-2222-222222222222", "entry_id": "E1",
            "indice": 1, "nome_file": "disegno.pdf", "cartella": "abc123abc123", "message_id": "<m1@prova>",
            "casella_id": CASELLA_COMMERCIALE,
        },
    }


def test_lo_stage_carica_il_file_al_server_prima_del_result(tmp_path, monkeypatch):
    """M7, lato worker: il file va al server con PUT, legato al tentativo, PRIMA del result; il result
    porta sha256 e byte e nessun percorso locale; il file temporaneo non resta sul disco del worker."""
    contenuto = b"%PDF-1.4 " + bytes(range(256)) * 40
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_stage())
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFintoAllegato(contenuto)
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        # M12, lato worker: il payload non porta store_id; lo store lo mette il worker dal SUO profilo
        assert finto.store_usati == ["STORE-COMMERCIALE-LOCALE"], finto.store_usati

        assert s.eventi == ["upload", "result"], f"ordine delle chiamate: {s.eventi}"
        assert len(s.caricamenti) == 1
        c = s.caricamenti[0]
        assert c["allegato_id"] == "22222222-2222-2222-2222-222222222222"
        assert c["query"]["job_id"] == "30" and c["query"]["lease_token"] == "tok-30"
        assert c["query"]["worker_id"].startswith("outlook@")
        assert c["bytes"] == len(contenuto) and c["sha256"] == hashlib.sha256(contenuto).hexdigest()

        r = s.risultati[30]
        assert r["esito"] == "ok"
        assert r["dati"]["sha256"] == c["sha256"] and r["dati"]["bytes"] == len(contenuto)
        assert "path_staging" not in r["dati"], "il result dichiara ancora un percorso locale"
        assert not os.path.exists(finto.salvati[0]), "il file temporaneo del worker non è stato rimosso"


def test_un_413_sull_upload_e_un_errore_definitivo(tmp_path, monkeypatch):
    """M8, lato worker: il limite è del server e ricaricare non rimpicciolisce il file."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.stato_upload = 413
        s.metti_job(_job_stage())
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFintoAllegato(b"x" * 1000)
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        r = s.risultati[30]
        assert r["esito"] == "errore" and r["definitivo"] is True
        assert "limite" in r["errore"]
        assert not os.path.exists(finto.salvati[0])


def test_un_409_sull_upload_non_riporta_niente(tmp_path, monkeypatch):
    """M13, lato worker: se l'upload è rifiutato perché il tentativo non vale più, il job appartiene
    a un altro tentativo e questo worker non deve riportare nulla."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.stato_upload = 409
        s.metti_job(_job_stage())
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFintoAllegato(b"x" * 1000)
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        assert 30 not in s.risultati, f"riportato un risultato con il tentativo non più valido: {s.risultati.get(30)}"
        assert s.eventi == ["upload"]
        assert not os.path.exists(finto.salvati[0])


# ---------------------------------------------------------------- caselle → store locale (voce 2.6, M1 e M12 lato worker)


def test_il_worker_risolve_solo_le_caselle_censite_e_le_dichiara_al_claim(tmp_path, monkeypatch):
    """M1 (parte L2): il worker parte dall'elenco del SERVER, non dal profilo. Il profilo finto ha tre
    store — come il PC di prova reale, dove il terzo è Filippo — ma il server ne censisce due: il
    terzo non viene chiesto, risolto né dichiarato. Il claim porta le due caselle con lo StoreID
    LOCALE, la postazione e outlook_ok."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)         # nessun job: un claim solo, poi esce

        assert s.richieste_caselle == [w.worker_id]
        assert sorted(finto.chieste) == ["commerciale@azienda.example", "francesco@azienda.example"]
        assert "filippo@azienda.example" not in finto.chieste, "lo store non censito è stato toccato"

        c = s.claim_fatti[0]
        assert c["postazione"] == w.postazione and c["outlook_ok"] is True
        aperte = {a["casella_id"]: a["store_id"] for a in c["caselle_aperte"]}
        assert aperte == {CASELLA_FRANCESCO: "STORE-FRANCESCO-LOCALE", CASELLA_COMMERCIALE: "STORE-COMMERCIALE-LOCALE"}
        assert "STORE-FILIPPO-NON-CENSITO" not in aperte.values()


def test_una_casella_censita_ma_assente_dal_profilo_non_viene_dichiarata(tmp_path, monkeypatch):
    """Il server censisce Luigi, ma questo profilo non ce l'ha: il worker non la dichiara (il server
    non gli assegnerà i suoi job, M12) e non inventa uno store."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE + [
            {"casella_id": CASELLA_LUIGI, "indirizzo": "luigi@azienda.example", "nome": "Luigi", "condivisa": False}]
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        monkeypatch.setattr(w, "ol", lambda: OutlookFinto([]))
        w.esegui_per_sempre(una_volta=True)

        aperte = {a["casella_id"] for a in s.claim_fatti[0]["caselle_aperte"]}
        assert aperte == {CASELLA_FRANCESCO, CASELLA_COMMERCIALE}
        assert CASELLA_LUIGI not in aperte


def test_senza_caselle_da_servire_il_worker_non_apre_outlook(tmp_path):
    """Con zero caselle assegnate non c'è niente da risolvere: Outlook non si apre e il claim dichiara
    caselle_aperte vuoto. È anche il motivo per cui il primo test di questo file resta valido."""
    with ServerFinto() as s:
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert w.outlook is None
        assert s.claim_fatti[0]["caselle_aperte"] == []


def test_un_job_di_una_casella_non_risolta_fallisce_senza_definitivo(tmp_path, monkeypatch):
    """Se — nonostante il claim — arriva un job di una casella che questo profilo non ha, il worker
    non «prova» sullo store predefinito: riporta un errore NON definitivo, così un altro worker che
    la serve può farcela."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        job = _job_sync(lotto=2)
        job["casella_id"] = job["payload"]["casella_id"] = CASELLA_LUIGI
        s.metti_job(job)
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([_messaggio(1)])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

        assert finto.letture == [], "ha letto una cartella pur non avendo lo store della casella"
        assert s.lotti == []
        r = s.risultati[20]
        assert r["esito"] == "errore" and r["definitivo"] is False
        assert "non è risolta nel profilo" in r["errore"]


def test_il_worker_rifiutato_con_403_non_muore_e_lo_dice(tmp_path, monkeypatch, caplog):
    """Un worker non censito (o su un'altra postazione) riceve 403 al claim: resta vivo, lo scrive
    nel log e riprova più tardi, perché chi corregge cockpit.toml deve trovarlo acceso."""
    import logging
    with ServerFinto() as s:
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        from cockpit_client import ErroreHTTP

        def claim_403(*a, **k):
            raise ErroreHTTP("POST", "/api/v1/jobs/claim", 403, '{"errore":"worker \"outlook@X\" non censito"}')

        monkeypatch.setattr(w.api, "claim", claim_403)
        with caplog.at_level(logging.ERROR):
            w.esegui_per_sempre(una_volta=True)
        assert any("rifiuta questo worker" in r.message for r in caplog.records)
