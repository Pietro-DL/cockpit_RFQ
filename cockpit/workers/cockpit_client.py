"""Modulo comune dei worker Cockpit (piano di correzione, voce 0.4 — W7).

Qui vivono le parti che worker_outlook.py e worker_analisi.py avevano copiato una per uno:
client HTTP, lettura della configurazione, log rotante, thread di battito (heartbeat) con flag di
arresto. Nessuna dipendenza esterna: solo la libreria standard, così il modulo si prova senza Outlook.

Il thread di battito serve alla fase 1 (voce 1.5): il lavoro vero può bloccarsi dentro una chiamata
COM che non ritorna, quindi il battito non può stare sullo stesso thread. Quando il server risponde
409 (lease perso, oppure durata massima superata) il thread alza il flag di arresto: chi esegue il
lavoro lo controlla ai punti di ripresa e smette senza riportare nulla.
"""
from __future__ import annotations

import http.client
import json
import logging
import logging.handlers
import os
import socket
import sys
import threading
import tomllib
import urllib.error
import urllib.request

log = logging.getLogger("cockpit")

# Errori di rete che significano «server giù o riavviato»: si aspetta e si riprova, il worker non muore.
# ConnectionResetError/ConnectionRefusedError sono OSError NON incapsulati da urllib quando arrivano
# da getresponse().
ERRORI_RETE = (
    urllib.error.URLError,
    TimeoutError,
    socket.timeout,
    OSError,
    http.client.RemoteDisconnected,
    http.client.HTTPException,
)


class ErroreHTTP(RuntimeError):
    """Risposta HTTP di errore dal server. `stato` permette di distinguere i casi che contano.

    409 = questo tentativo non è più valido (lease perso o durata massima superata): chi lavora deve
    fermarsi e NON deve riportare il risultato, perché il job appartiene ormai a un altro tentativo.
    4xx = richiesta non accettabile: ritentarla identica non serve.
    5xx = problema momentaneo del server: si ritenta.
    """

    def __init__(self, metodo: str, percorso: str, stato: int, corpo: str):
        super().__init__(f"{metodo} {percorso} → {stato}: {corpo[:500]}")
        self.metodo = metodo
        self.percorso = percorso
        self.stato = stato
        self.corpo = corpo

    @property
    def tentativo_non_valido(self) -> bool:
        return self.stato == 409

    @property
    def ritentabile(self) -> bool:
        return self.stato >= 500


class ArrestoRichiesto(Exception):
    """Il tentativo corrente non è più valido: interrompere il lavoro senza riportare il risultato."""


class Cockpit:
    """Client dell'API worker di cockpit.exe (urllib: nessuna dipendenza)."""

    def __init__(self, url: str, token: str, worker_id: str = ""):
        self.url = url.rstrip("/")
        self.token = token
        self.worker_id = worker_id

    # ------------------------------------------------------------ trasporto

    def chiama(self, metodo: str, percorso: str, corpo: dict | None = None, timeout: int = 60):
        dati = json.dumps(corpo).encode("utf-8") if corpo is not None else None
        req = urllib.request.Request(
            self.url + percorso,
            data=dati,
            method=metodo,
            headers={"Content-Type": "application/json", "X-Cockpit-Token": self.token},
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                if r.status == 204:
                    return None
                return json.loads(r.read().decode("utf-8") or "null")
        except urllib.error.HTTPError as e:
            testo = e.read().decode("utf-8", "ignore")
            raise ErroreHTTP(metodo, percorso, e.code, testo) from None

    # ------------------------------------------------------------ coda

    def claim(self, worker: str, worker_id: str, attesa_s: int = 20) -> dict | None:
        return self.chiama(
            "POST",
            "/api/v1/jobs/claim",
            {"worker": worker, "worker_id": worker_id, "attesa_s": attesa_s},
            timeout=attesa_s + 15,
        )

    def heartbeat(self, job_id: int, worker_id: str) -> None:
        self.chiama("POST", f"/api/v1/jobs/{job_id}/heartbeat", {"worker_id": worker_id})

    def risultato(self, job_id: int, corpo: dict) -> None:
        self.chiama("POST", f"/api/v1/jobs/{job_id}/result", corpo)

    def ingest(self, richiesta: dict, timeout: int = 300) -> dict:
        return self.chiama("POST", "/api/v1/ingest/messaggi", richiesta, timeout=timeout)


class Battito:
    """Thread che manda il heartbeat mentre il lavoro gira su un altro thread.

    Uso:

        with Battito(api, job_id, worker_id, ogni_s=30) as b:
            fai_il_lavoro(controlla=b.controlla)     # b.controlla() alza ArrestoRichiesto dopo un 409
        if b.arresto.is_set():
            return                                   # niente result: il tentativo non è più nostro

    Il thread non tocca mai COM: chiama solo HTTP. Se il server risponde 409 alza `arresto`; se la
    rete è giù riprova, perché un buco di rete non significa aver perso il lease.
    """

    def __init__(self, api: Cockpit, job_id: int, worker_id: str, ogni_s: float = 30.0):
        self.api = api
        self.job_id = job_id
        self.worker_id = worker_id
        self.ogni_s = ogni_s
        self.arresto = threading.Event()
        self.motivo = ""
        self._ferma = threading.Event()
        self._thread: threading.Thread | None = None

    def __enter__(self) -> "Battito":
        self._thread = threading.Thread(target=self._loop, name=f"battito-{self.job_id}", daemon=True)
        self._thread.start()
        return self

    def __exit__(self, *_exc) -> None:
        self.chiudi()

    def chiudi(self, attesa_s: float = 5.0) -> None:
        self._ferma.set()
        if self._thread is not None:
            self._thread.join(timeout=attesa_s)
            self._thread = None

    def controlla(self) -> None:
        """Da chiamare ai punti di ripresa del lavoro: alza ArrestoRichiesto se il lease è perso."""
        if self.arresto.is_set():
            raise ArrestoRichiesto(self.motivo or "tentativo non più valido")

    def _loop(self) -> None:
        while not self._ferma.wait(self.ogni_s):
            try:
                self.api.heartbeat(self.job_id, self.worker_id)
            except ErroreHTTP as e:
                if e.tentativo_non_valido:
                    self.motivo = e.corpo[:200] or "409 dal server"
                    log.warning("job %d: lease perso (%s): chiedo l'arresto del lavoro", self.job_id, self.motivo)
                    self.arresto.set()
                    return
                log.warning("job %d: heartbeat rifiutato (%s)", self.job_id, e)
            except ERRORI_RETE as e:
                # rete giù: non è la perdita del lease, si riprova al battito successivo
                log.debug("job %d: heartbeat non recapitato (%s)", self.job_id, e)


# ---------------------------------------------------------------- configurazione e log


def carica_config(percorso: str, predefiniti: dict | None = None) -> dict:
    """worker.toml + variabili d'ambiente (che vincono sul file). Esce se manca il token."""
    cfg = {
        "server_url": "http://127.0.0.1:8080",
        "token": "",
        "staging": os.path.join("..", "_staging"),
        "consenti_invio": False,
    }
    if predefiniti:
        cfg.update(predefiniti)
    if os.path.isfile(percorso):
        with open(percorso, "rb") as f:
            cfg.update(tomllib.load(f))
    for chiave, env in (
        ("server_url", "COCKPIT_URL"),
        ("token", "COCKPIT_TOKEN"),
        ("staging", "COCKPIT_STAGING"),
        ("worker_id", "COCKPIT_WORKER_ID"),
    ):
        if os.environ.get(env):
            cfg[chiave] = os.environ[env]
    if not cfg["token"]:
        sys.exit(f"token mancante: {percorso} [token] o variabile COCKPIT_TOKEN")
    return cfg


def nome_worker(tipo: str, cfg: dict | None = None) -> str:
    """Nome stabile del worker: 'outlook@PC-FRANCESCO'.

    Stabile perché è la chiave di `worker_credenziale` (fase 2): non contiene il PID, che cambiava a
    ogni riavvio e rendeva inutile la tabella delle presenze.
    """
    if cfg and cfg.get("worker_id"):
        return str(cfg["worker_id"])
    return f"{tipo}@{socket.gethostname().upper()}"


def configura_log(debug: bool, cfg: dict, nome: str) -> None:
    """Console + file rotante in <staging>/log/<nome>.log (5 × 5 MB): il log sopravvive al terminale."""
    fmt = logging.Formatter("%(asctime)s %(levelname)-7s %(name)s: %(message)s")
    radice = logging.getLogger()
    radice.setLevel(logging.DEBUG if debug else logging.INFO)
    for h in list(radice.handlers):  # idempotente: riconfigurare non raddoppia le righe
        radice.removeHandler(h)
    console = logging.StreamHandler()
    console.setFormatter(fmt)
    radice.addHandler(console)
    try:
        cartella = os.path.join(os.path.abspath(cfg["staging"]), "log")
        os.makedirs(cartella, exist_ok=True)
        fh = logging.handlers.RotatingFileHandler(
            os.path.join(cartella, nome + ".log"), maxBytes=5 << 20, backupCount=5, encoding="utf-8"
        )
        fh.setFormatter(fmt)
        radice.addHandler(fh)
    except OSError as e:
        radice.warning("log su file non disponibile: %s", e)
