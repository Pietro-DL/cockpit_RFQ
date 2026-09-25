"""L2 — il ciclo dei due worker contro il server finto (accettazione della voce 0.4: `--una-volta`).

Nessuna chiamata COM e nessun PDF: si prova che il ciclo prende un job, riporta un risultato ed esce
quando la coda è vuota. Il comportamento di Outlook è materia di L5, quello dell'analisi di
test_worker_analisi.py.
"""
from __future__ import annotations

import hashlib
import os
import time
from datetime import datetime, timezone

import pytest

from server_finto import ServerFinto

worker_analisi = pytest.importorskip("worker_analisi")
worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")
LetturaIncompleta = pytest.importorskip("outlook_com").LetturaIncompleta


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


ALLEGATO_ANALISI = "00000000-0000-0000-0000-000000000001"


def _job_analisi(job_id: int, contenuto: bytes | None, nome_file: str = "documento.pdf", **extra) -> dict:
    payload = {
        "allegato_id": ALLEGATO_ANALISI,
        "messaggio_id": "00000000-0000-0000-0000-000000000002",
        "sha256": hashlib.sha256(contenuto).hexdigest() if contenuto is not None else "0" * 64,
        "bytes": len(contenuto) if contenuto is not None else 0,
        "nome_file": nome_file,
    }
    payload.update(extra)
    return {"job_id": job_id, "tipo": "analizza_allegato", "tentativi": 1, "lease_s": 120,
            "lease_token": f"tok-{job_id}", "payload": payload}


def test_contenuto_sparito_dal_server_e_definitivo(tmp_path):
    """Ritentare cinque volte un contenuto che il server non ha più è tempo perso: l'errore è
    definitivo con rimedio (Riscarica). Il server risponde 410 al GET del contenuto."""
    with ServerFinto() as s:
        s.metti_job(_job_analisi(12, None, "non-esiste.pdf"))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert s.risultati[12]["esito"] == "errore"
        assert s.risultati[12]["definitivo"] is True
        assert "Riscarica" in s.risultati[12]["errore"]


def test_il_worker_analisi_prende_i_byte_dal_server_non_da_un_percorso(tmp_path):
    """7C.1, P0: il contratto vecchio portava `path_staging`, il percorso sul disco del SERVER, e il
    worker sull'altro PC lo cercava sul proprio. Qui il payload porta ancora un path_staging — che
    punta a un file che NON esiste su questo PC — e il worker deve ignorarlo: scarica i byte con GET
    /allegati/{id}/contenuto dentro il proprio tentativo, li verifica con lo sha256, analizza e
    non lascia niente nella propria cartella."""
    contenuto = b"0\nSECTION\n2\nENTITIES\n" + bytes(range(256)) * 20
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto
        s.metti_job(_job_analisi(13, contenuto, "1234567A_4.dxf",
                                 path_staging=str(tmp_path / "server-di-un-altro-pc" / "1234567A_4.dxf")))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)

        assert s.eventi == ["contenuto", "result"], f"ordine delle chiamate: {s.eventi}"
        d = s.scaricamenti[0]
        assert d["allegato_id"] == ALLEGATO_ANALISI
        assert d["query"]["job_id"] == "13" and d["query"]["lease_token"] == "tok-13"
        assert d["query"]["worker_id"].startswith("analisi@")
        r = s.risultati[13]
        assert r["esito"] == "ok", r
        assert r["dati"]["allegato_id"] == ALLEGATO_ANALISI
        assert r["dati"]["codice"] == "1234567A" and r["dati"]["rev"] == "4"
        assert not os.path.exists(tmp_path / "tmp" / "13"), "la cartella temporanea del job non è stata rimossa"


def test_un_pdf_illeggibile_non_lascia_file_temporanei(tmp_path):
    """Su Windows PyMuPDF tiene aperto un file che non riesce ad aprire: alla prima prova del
    download il temporaneo restava nella tmp del worker («utilizzato da un altro processo»)."""
    contenuto = b"%PDF-1.4 " + bytes(range(256)) * 20      # comincia da PDF, non lo è
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto
        s.metti_job(_job_analisi(16, contenuto, "illeggibile.pdf"))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        r = s.risultati[16]
        assert r["esito"] == "ok" and r["dati"]["tipo_proposto"] == "da_determinare", r
        assert "errore_pdf" in r["dati"]["dettagli"]
        assert not os.path.exists(tmp_path / "tmp" / "16"), os.listdir(tmp_path / "tmp" / "16")


def test_un_contenuto_diverso_dallo_sha256_atteso_non_si_analizza(tmp_path):
    """Un file arrivato a metà, o un altro file, è un errore NON definitivo: si riscarica."""
    contenuto = b"%PDF-1.4 " + bytes(range(256)) * 20
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto + b"byte in piu'"
        s.metti_job(_job_analisi(14, contenuto))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        r = s.risultati[14]
        assert r["esito"] == "errore" and not r.get("definitivo"), r
        assert "sha256" in r["errore"] or "byte" in r["errore"]
        assert not os.path.exists(tmp_path / "tmp" / "14")


def test_un_409_sul_download_non_riporta_niente(tmp_path):
    """Il tentativo non vale più: il job è di un altro tentativo, questo worker tace."""
    contenuto = b"x" * 1000
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto
        s.metti_job(_job_analisi(15, contenuto))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        # il server consegna il job con un token, poi «cambia idea»: al download il tentativo è un altro
        w.api.claim = (lambda vero: (lambda *a, **k: _cambia_token(s, vero(*a, **k))))(w.api.claim)
        w.esegui_per_sempre(una_volta=True)
        assert 15 not in s.risultati, f"riportato un risultato con il tentativo non più valido: {s.risultati.get(15)}"
        assert s.eventi == []


def _cambia_token(s: ServerFinto, job: dict | None) -> dict | None:
    if job:
        with s.lock:
            s.tentativi[job["job_id"]]["lease_token"] = "un-altro-tentativo"
    return job


def _analisi_lenta(monkeypatch, durata_s: float = 0.25) -> None:
    """Un'analisi che dura piu' di qualche battito: e' la condizione in cui il difetto si vedeva."""
    vera = worker_analisi.analizza_file
    monkeypatch.setattr(worker_analisi, "cadenza_battito", lambda _lease_s: 0.05)
    monkeypatch.setattr(worker_analisi, "analizza_file",
                        lambda percorso, nome_file: (time.sleep(durata_s), vera(percorso, nome_file))[1])


def test_il_worker_analisi_batte_durante_l_analisi(tmp_path, monkeypatch):
    """B8.0: il worker analisi non aveva il battito, e il worker Outlook si'.

    Con il lease a 120 s bastava un PDF grosso (o un download lento) perche' il server desse il
    tentativo per perso, rimettesse il job in coda e poi RIFIUTASSE il result con 409: l'analisi era
    stata fatta davvero, e la proposta restava quella dal nome. Qui l'analisi dura piu' di qualche
    battito e il server deve vedere arrivare gli heartbeat di QUESTO tentativo."""
    contenuto = b"%PDF-1.4 " + bytes(range(256)) * 20
    _analisi_lenta(monkeypatch)
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto
        s.metti_job(_job_analisi(17, contenuto, "lento.pdf"))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert len(s.battiti) >= 2, f"attesi almeno 2 battiti durante l'analisi, ricevuti {s.battiti}"
        assert set(s.battiti) == {17}
        # senza il token il server non potrebbe distinguere questo tentativo da uno scaduto
        assert {c.get("lease_token") for c in s.battiti_corpo} == {"tok-17"}
        assert s.risultati[17]["esito"] == "ok", s.risultati[17]


def test_un_409_al_battito_ferma_l_analisi_senza_riportare_niente(tmp_path, monkeypatch):
    """Il rovescio: se il lease e' davvero perso, il job e' gia' di un altro tentativo e questo tace.

    Riportare adesso vorrebbe dire scrivere sopra al lavoro di quell'altro."""
    contenuto = b"%PDF-1.4 " + bytes(range(256)) * 20
    _analisi_lenta(monkeypatch)
    with ServerFinto() as s:
        s.contenuti[ALLEGATO_ANALISI] = contenuto
        s.stato_heartbeat = 409
        s.metti_job(_job_analisi(18, contenuto, "perso.pdf"))
        w = worker_analisi.WorkerAnalisi(s.config(staging=str(tmp_path)))
        w.esegui_per_sempre(una_volta=True)
        assert 18 not in s.risultati, f"riportato con il tentativo perso: {s.risultati.get(18)}"
        assert w.battito is None, "il battito e' rimasto appeso al worker dopo il job"


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


def test_il_sync_riporta_dove_ha_passato_il_tempo(tmp_path, monkeypatch):
    """7C.1, P1: il result del sync porta i tempi (COM, serializzazione, HTTPS, server), cosi' un
    bootstrap lento si spiega con dei numeri e non con «sara' la rete»."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.risposta_ingest = {"inseriti": 2, "aggiornati": 0, "falliti": 0, "esiti": [], "durata_ms": 250}
        s.metti_job(_job_sync(lotto=2))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([_messaggio(1), _messaggio(2), _messaggio(3)])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)
        r = s.risultati[20]
        assert r["esito"] == "ok", r
        tempi = r["dati"]["tempi"]
        assert set(tempi) == {"com", "serializzazione", "https", "server"}, tempi
        assert all(v >= 0 for v in tempi.values())
        assert tempi["https"] > 0, "due lotti sono passati per HTTPS e il tempo e' zero"
        # il server ha dichiarato 250 ms per ciascuno dei due lotti
        assert abs(tempi["server"] - 0.5) < 1e-6, tempi


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

def _job_sync_due_cartelle(dal_inbox, dal, al=None, modo=None, lotto=50) -> dict:
    """Un sync su due cartelle. Dal blocco 3 la finestra la decide il SERVER, una per cartella: qui
    si scrive quello che il server avrebbe messo nel payload."""
    j = _job_sync(lotto=lotto)
    j["payload"]["cartelle"] = [
        {"cartella": "Inbox", "dal": dal_inbox or dal, "al": al},
        {"cartella": "Sent Items", "dal": dal, "al": al},
    ]
    j["payload"]["dal"] = dal
    if al is not None:
        j["payload"]["al"] = al
    if modo is not None:
        j["payload"]["modo"] = modo
    return j


def test_il_worker_legge_la_finestra_della_cartella_non_quella_del_payload(tmp_path, monkeypatch):
    """Blocco 3: la finestra la decide il server, una per cartella, e il worker la esegue.

    Prima era il worker a comporla — cursore meno sovrapposizione, oppure il ripiego del payload — e
    la stessa aritmetica viveva in due posti: quella del worker e quella che il server usava per
    scrivere il ripiego. Adesso `cartelle[].dal` e `cartelle[].al` SONO la finestra, e l'inviluppo
    `payload.dal` serve all'operatore e ai job accodati prima di questo blocco.

    Due cartelle della stessa casella possono essere a punti diversi — una sincronizzata da mesi, una
    aggiunta stamattina — e se il worker applicasse a tutte il limite del payload, la prima
    rileggerebbe ogni volta la finestra iniziale da capo: nessun errore, nessun buco, solo il worker
    occupato per niente a ogni giro, con la deduplica per Message-ID a nascondere il sintomo.
    """
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox="2026-09-15T07:50:00Z", dal="2026-09-09T00:00:00Z",
                                           al="2026-09-16T09:00:00Z"))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

    assert set(finto.finestre) == {"Inbox", "Sent Items"}, finto.finestre
    atteso = datetime(2026, 9, 16, 9, 0, tzinfo=timezone.utc)
    # la copertura della cartella, sovrapposizione già sottratta dal server
    assert finto.finestre["Inbox"] == (datetime(2026, 9, 15, 7, 50, tzinfo=timezone.utc), atteso), finto.finestre["Inbox"]
    # nessuna copertura: la finestra iniziale, sempre calcolata dal server
    assert finto.finestre["Sent Items"] == (datetime(2026, 9, 9, 0, 0, tzinfo=timezone.utc), atteso), finto.finestre["Sent Items"]


def test_il_limite_superiore_e_fissato_e_non_diventa_adesso(tmp_path, monkeypatch):
    """Una finestra il cui estremo superiore è «adesso» cambia mentre il job gira, e allora non c'è
    nessun istante di cui si possa dire «scandito fino a qui». Dal blocco 3 `al` c'è sempre, anche
    nell'aggiornamento ordinario, ed è quello che il server ha fissato all'accodamento."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox=None, dal="2026-09-09T00:00:00Z",
                                           al="2026-09-16T09:00:00Z", modo="aggiornamento"))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        finto = OutlookFinto([])
        monkeypatch.setattr(w, "ol", lambda: finto)
        w.esegui_per_sempre(una_volta=True)

    for cartella, finestra in finto.finestre.items():
        assert finestra[1] == datetime(2026, 9, 16, 9, 0, tzinfo=timezone.utc), (cartella, finestra)


def test_una_finestra_percorsa_per_intero_viene_dichiarata_completa(tmp_path, monkeypatch):
    """`completa` è l'unica cosa che fa muovere una frontiera, e va detta esplicitamente: il server
    non la deduce piu’ dal fatto che il job sia finito senza errori."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox=None, dal="2026-09-09T00:00:00Z", al="2026-09-16T09:00:00Z"))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        monkeypatch.setattr(w, "ol", lambda: OutlookFinto([_messaggio(1)]))
        w.esegui_per_sempre(una_volta=True)

        cartelle = s.risultati[20]["dati"]["cartelle"]
    assert [c["completa"] for c in cartelle] == [True, True], cartelle


def test_una_finestra_interrotta_a_meta_non_viene_dichiarata_completa(tmp_path, monkeypatch):
    """Il caso che il revisore ha chiesto di aggiungere al blocco 3, dal lato del worker.

    L'enumerazione si rompe a meta’ con un limite superiore finito. Il job HTTP finisce senza panic e
    consegna anche dei messaggi — quelli che aveva gia’ letto — ma la cartella NON può risultare
    completa: in ordine decrescente quello che è rimasto fuori è il pezzo VECCHIO della finestra, e
    un `al` scritto sopra quel pezzo lo renderebbe invisibile per sempre.
    """
    class OutlookRotto(OutlookFinto):
        """Consegna un lotto intero, poi la collezione si rompe. Con `lotto=1` il primo messaggio e’
        gia’ partito quando arriva l'eccezione: e’ il caso che distingue «la finestra non e’
        conclusa» da «non e’ stato fatto niente»."""

        def leggi(self, cartella, dal, al=None, store_id="", saltati=None):
            self.finestre[cartella] = (dal, al)
            yield _messaggio(1)
            raise LetturaIncompleta(cartella + ": enumerazione interrotta (com_error simulato)")

    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox=None, dal="2026-09-09T00:00:00Z",
                                           al="2026-09-16T09:00:00Z", lotto=1))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        monkeypatch.setattr(w, "ol", lambda: OutlookRotto([]))
        w.esegui_per_sempre(una_volta=True)

        assert s.risultati[20]["esito"] == "ok", "il job non fallisce: le altre cartelle vanno lette lo stesso"
        cartelle = s.risultati[20]["dati"]["cartelle"]
    for c in cartelle:
        assert c["completa"] is False, "finestra interrotta dichiarata completa: " + repr(c)
        assert "LetturaIncompleta" in c["errore"], c["errore"]
    assert all(c["n_messaggi"] == 1 for c in cartelle), (
        "il lotto gia’ consegnato prima dell'interruzione non si butta: rileggere non e’ un danno, "
        "ma buttare via lavoro fatto per poi rifarlo identico sì")


def test_un_sync_storico_non_sposta_il_cursore_in_avanti(tmp_path, monkeypatch):
    """«Carica precedenti» legge una finestra CHIUSA nel passato: farle muovere il cursore
    significherebbe dichiarare letto fino a due giorni fa tutto ciò che sta in mezzo, e la posta
    arrivata nel frattempo non la rileggerebbe più nessuno."""
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox=None, dal="2026-09-12T00:00:00Z",
                                           al="2026-09-14T00:00:00Z", modo="storico"))
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


def test_il_worker_consegna_nell_ordine_in_cui_legge_il_piu_recente_per_primo(tmp_path, monkeypatch):
    """D del blocco 3: la posta piu' recente arriva per PRIMA, ed e' questo che l'operatore vede.

    L'ordine lo stabilisce l'enumerazione (test_outlook_finestra.py, con Restrict e con il ripiego);
    qui si tiene fermo che il worker non lo rimescoli fra la lettura e la consegna. Con un lotto per
    volta, il primo POST /ingest deve portare il messaggio piu' nuovo della finestra: se il worker
    accumulasse e invertisse, la prima cosa che compare nell'Inbox dopo una notte sarebbe la mail
    delle 17:05 di ieri invece di quella delle 09:00.
    """
    recenti = [_messaggio(9), _messaggio(5), _messaggio(1)]      # 10:09, 10:05, 10:01
    with ServerFinto() as s:
        s.caselle_worker = CASELLE_SERVITE
        s.metti_job(_job_sync_due_cartelle(dal_inbox=None, dal="2026-09-01T00:00:00Z",
                                           al="2026-09-02T00:00:00Z", lotto=1))
        w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
        monkeypatch.setattr(w, "ol", lambda: OutlookFinto(recenti))
        w.esegui_per_sempre(una_volta=True)

        consegnati = [m["message_id"] for lotto in s.lotti for m in lotto["messaggi"]]
    # due cartelle, tre messaggi ciascuna: conta l'ordine dentro la prima
    assert consegnati[:3] == ["<m9@prova>", "<m5@prova>", "<m1@prova>"], consegnati
