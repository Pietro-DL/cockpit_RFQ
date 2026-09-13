"""worker-outlook: client HTTP di cockpit.exe che esegue i job di tipo 'outlook' via COM.

    python worker_outlook.py [--config worker.toml] [--una-volta] [--cartelle]

Loop: POST /api/v1/jobs/claim (long-poll) → esegue → POST /api/v1/jobs/{id}/result.
Un solo thread: le chiamate COM sono serializzate per costruzione. Se Outlook non risponde il job
fallisce (ritentato dal server con backoff) e il worker resta vivo.
"""
from __future__ import annotations

import argparse
import json
import logging
import os
import socket
import sys
import time
import tomllib
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone

import pywintypes

from contratti import (CartellaEsito, IngestRichiesta, Job, PayloadApriElemento, PayloadCreaBozza,
                       PayloadSegnaLetto, PayloadSpostaCartella, PayloadStageAllegato, PayloadSyncOutlook,
                       RisultatoBozza, RisultatoRichiesta, RisultatoSposta, RisultatoStage, RisultatoSync)
from outlook_com import ErroreDefinitivo, Outlook

log = logging.getLogger("worker")


class Cockpit:
    """Client minimale dell'API worker (urllib: nessuna dipendenza)."""

    def __init__(self, url: str, token: str):
        self.url = url.rstrip("/")
        self.token = token

    def _chiama(self, metodo: str, percorso: str, corpo: dict | None = None, timeout: int = 60):
        dati = json.dumps(corpo).encode("utf-8") if corpo is not None else None
        req = urllib.request.Request(self.url + percorso, data=dati, method=metodo,
                                     headers={"Content-Type": "application/json", "X-Cockpit-Token": self.token})
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                if r.status == 204:
                    return None
                return json.loads(r.read().decode("utf-8") or "null")
        except urllib.error.HTTPError as e:
            testo = e.read().decode("utf-8", "ignore")
            raise RuntimeError(f"{metodo} {percorso} → {e.code}: {testo[:500]}") from None

    def claim(self, worker_id: str, attesa_s: int = 20) -> Job | None:
        r = self._chiama("POST", "/api/v1/jobs/claim", {"worker": "outlook", "worker_id": worker_id, "attesa_s": attesa_s}, timeout=attesa_s + 15)
        return Job.model_validate(r) if r else None

    def heartbeat(self, job_id: int, worker_id: str) -> None:
        self._chiama("POST", f"/api/v1/jobs/{job_id}/heartbeat", {"worker_id": worker_id})

    def risultato(self, job_id: int, r: RisultatoRichiesta) -> None:
        self._chiama("POST", f"/api/v1/jobs/{job_id}/result", r.model_dump(mode="json"))

    def ingest(self, richiesta: IngestRichiesta) -> dict:
        return self._chiama("POST", "/api/v1/ingest/messaggi", richiesta.model_dump(mode="json"), timeout=300)


class Worker:
    def __init__(self, cfg: dict):
        self.cfg = cfg
        self.api = Cockpit(cfg["server_url"], cfg["token"])
        self.worker_id = cfg.get("worker_id") or f"outlook@{socket.gethostname()}#{os.getpid()}"
        self.staging = os.path.abspath(cfg["staging"])
        os.makedirs(self.staging, exist_ok=True)
        self.outlook: Outlook | None = None

    def ol(self) -> Outlook:
        if self.outlook is None:
            self.outlook = Outlook(consenti_invio=bool(self.cfg.get("consenti_invio", False)))
        return self.outlook

    # ------------------------------------------------------------ loop

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s → %s", self.worker_id, self.api.url)
        while True:
            try:
                job = self.api.claim(self.worker_id)
            except (RuntimeError, urllib.error.URLError, TimeoutError, socket.timeout) as e:
                log.warning("server non raggiungibile: %s", e)
                time.sleep(5)
                continue
            if job is None:
                if una_volta:
                    return
                continue
            self.esegui(job)
            if una_volta:
                return

    def esegui(self, job: Job) -> None:
        t0 = time.time()
        log.info("job %d %s (tentativo %d)", job.job_id, job.tipo, job.tentativi)
        try:
            dati = self.dispatch(job)
            ris = RisultatoRichiesta(esito="ok", dati=dati)
        except ErroreDefinitivo as e:
            log.error("job %d errore definitivo: %s", job.job_id, e)
            ris = RisultatoRichiesta(esito="errore", errore=str(e), definitivo=True)
        except pywintypes.com_error as e:
            log.exception("job %d errore COM", job.job_id)
            self.outlook = None  # riconnette al prossimo job (Outlook chiuso/riavviato)
            ris = RisultatoRichiesta(esito="errore", errore=f"COM: {e}")
        except Exception as e:  # noqa: BLE001 - il worker non deve mai morire per un job
            log.exception("job %d errore", job.job_id)
            ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])
        try:
            self.api.risultato(job.job_id, ris)
        except Exception as e:  # noqa: BLE001
            log.error("impossibile riportare il risultato del job %d: %s", job.job_id, e)
        log.info("job %d %s → %s in %.1fs", job.job_id, job.tipo, ris.esito, time.time() - t0)

    def dispatch(self, job: Job) -> dict:
        p = job.payload
        match job.tipo:
            case "sync_outlook":
                return self.sync(job.job_id, PayloadSyncOutlook.model_validate(p))
            case "stage_allegato":
                s = PayloadStageAllegato.model_validate(p)
                dest, sha, n = self.ol().salva_allegato(s.entry_id, s.store_id, s.indice, s.nome_file, os.path.join(self.staging, s.cartella))
                return RisultatoStage(allegato_id=s.allegato_id, path_staging=dest, sha256=sha, bytes=n).model_dump(mode="json")
            case "crea_bozza_outlook":
                b = PayloadCreaBozza.model_validate(p)
                entry_id, inviata = self.ol().crea_bozza(b)
                return RisultatoBozza(entry_id=entry_id, inviata=inviata).model_dump(mode="json")
            case "apri_elemento_outlook":
                a = PayloadApriElemento.model_validate(p)
                self.ol().apri(a.entry_id, a.store_id)
                return {}
            case "sposta_in_cartella":
                s = PayloadSpostaCartella.model_validate(p)
                return RisultatoSposta(entry_id=self.ol().sposta(s.entry_id, s.store_id, s.cartella)).model_dump(mode="json")
            case "segna_letto":
                l = PayloadSegnaLetto.model_validate(p)
                self.ol().segna_letto(l.entry_id, l.store_id, l.letto)
                return {}
        raise ErroreDefinitivo(f"tipo job sconosciuto per il worker outlook: {job.tipo}")

    # ------------------------------------------------------------ sync

    def sync(self, job_id: int, p: PayloadSyncOutlook) -> dict:
        esiti = []
        ultimo_hb = time.time()
        for c in p.cartelle:
            dal = (c.ultimo_received - timedelta(seconds=p.sovrapposizione_s)) if c.ultimo_received else p.dal
            if dal.tzinfo is None:
                dal = dal.replace(tzinfo=timezone.utc)
            al = p.al
            if al is not None and al.tzinfo is None:
                al = al.replace(tzinfo=timezone.utc)
            esito = CartellaEsito(cartella=c.cartella, ultimo_received=c.ultimo_received)
            lotto: list = []
            try:
                for m in self.ol().leggi(c.cartella, dal, al=al):
                    lotto.append(m)
                    if al is None and m.data_evento and (esito.ultimo_received is None or m.data_evento > esito.ultimo_received):
                        esito.ultimo_received = m.data_evento
                    if len(lotto) >= p.lotto:
                        esito.n_messaggi += self._invia(lotto)
                        lotto = []
                    if time.time() - ultimo_hb > 60:
                        self.api.heartbeat(job_id, self.worker_id)
                        ultimo_hb = time.time()
                if lotto:
                    esito.n_messaggi += self._invia(lotto)
            except ErroreDefinitivo as e:
                esito.errore = str(e)
                log.error("cartella %s: %s", c.cartella, e)
            log.info("sync %s: finestra [%s, %s] → %d messaggi, cursore %s", c.cartella, dal.isoformat(), al.isoformat() if al else "now", esito.n_messaggi, esito.ultimo_received)
            esiti.append(esito)
        return RisultatoSync(cartelle=esiti).model_dump(mode="json")

    def _invia(self, lotto: list) -> int:
        r = self.api.ingest(IngestRichiesta(messaggi=lotto))
        log.info("ingest: %d inseriti, %d aggiornati", r.get("inseriti", 0), r.get("aggiornati", 0))
        return len(lotto)


def carica_config(percorso: str) -> dict:
    cfg = {"server_url": "http://127.0.0.1:8080", "token": "", "staging": "..\\_staging", "consenti_invio": False}
    if os.path.isfile(percorso):
        with open(percorso, "rb") as f:
            cfg.update(tomllib.load(f))
    for k, env in (("server_url", "COCKPIT_URL"), ("token", "COCKPIT_TOKEN"), ("staging", "COCKPIT_STAGING")):
        if os.environ.get(env):
            cfg[k] = os.environ[env]
    if not cfg["token"]:
        sys.exit("token mancante: worker.toml [token] o variabile COCKPIT_TOKEN")
    return cfg


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--config", default=os.path.join(os.path.dirname(__file__), "worker.toml"))
    ap.add_argument("--una-volta", action="store_true", help="esegue al più un job ed esce")
    ap.add_argument("--cartelle", action="store_true", help="stampa l'albero delle cartelle Outlook ed esce")
    ap.add_argument("--debug", action="store_true")
    a = ap.parse_args()
    logging.basicConfig(level=logging.DEBUG if a.debug else logging.INFO, format="%(asctime)s %(levelname)-7s %(name)s: %(message)s")
    cfg = carica_config(a.config)
    w = Worker(cfg)
    if a.cartelle:
        print("\n".join(w.ol().elenca_cartelle()))
        return
    w.esegui_per_sempre(una_volta=a.una_volta)


if __name__ == "__main__":
    main()
