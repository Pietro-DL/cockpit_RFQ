"""Server finto dell'API worker: regge i test L2 dei worker senza cockpit.exe né PostgreSQL.

Non è un mock delle regole del server — quelle si provano in L4 contro PostgreSQL vero. Qui serve
solo a far girare il ciclo del worker e a osservare che cosa manda: quali job prende, quanti battiti
invia, che cosa riporta, come si comporta quando una risposta è 409, 503 o non arriva affatto.

    with ServerFinto() as s:
        s.metti_job({"job_id": 1, "tipo": "sync_outlook", "payload": {}, "tentativi": 1, "lease_s": 120})
        ...
        assert s.risultati[1]["esito"] == "ok"
"""
from __future__ import annotations

import hashlib
import json
import threading
from urllib.parse import parse_qs, urlsplit
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class ServerFinto:
    def __init__(self, token: str = "token-di-prova"):
        self.token = token
        self.lock = threading.Lock()
        self.job: list[dict] = []              # coda dei job da consegnare al claim
        self.claim_fatti: list[dict] = []      # corpi delle richieste di claim ricevute
        self.battiti: list[int] = []           # job_id di ogni heartbeat ricevuto
        self.battiti_corpo: list[dict] = []    # corpo di ogni heartbeat: serve a vedere il lease_token
        self.risultati: dict[int, dict] = {}   # job_id → corpo del result
        self.lotti: list[dict] = []            # corpi di ogni POST /ingest/messaggi
        self.stato_heartbeat = 204             # forzabile a 409 per simulare il lease perso
        self.stato_ingest = 200                # forzabile a 503 per simulare il server occupato
        self.stato_result = 204
        self.stato_upload = 204                # forzabile a 409 / 413 per provare l'upload (voce 2.3)
        self.caricamenti: list[dict] = []      # ogni PUT /allegati/{id}/file: id, query, bytes, sha256
        self.eventi: list[str] = []            # ordine delle chiamate che contano: "upload", "result"
        self.risposta_ingest: dict | None = None
        # GET /api/v1/worker/caselle (voce 2.6): le caselle che il server chiede al worker di
        # risolvere. Vuoto = il worker non ha niente da risolvere e non apre Outlook.
        self.caselle_worker: list[dict] = []
        self.richieste_caselle: list[str] = []      # worker_id di ogni GET /worker/caselle
        self.non_autorizzati = 0
        self._srv: ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None

    # ------------------------------------------------------------ ciclo di vita

    def __enter__(self) -> "ServerFinto":
        padrone = self

        class Gestore(BaseHTTPRequestHandler):
            def log_message(self, *_a):  # niente rumore sullo stderr dei test
                pass

            def _corpo(self) -> dict:
                n = int(self.headers.get("Content-Length") or 0)
                return json.loads(self.rfile.read(n) or b"{}") if n else {}

            def _rispondi(self, stato: int, corpo=None):
                dati = json.dumps(corpo).encode() if corpo is not None else b""
                self.send_response(stato)
                if dati:
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(dati)))
                self.end_headers()
                if dati:
                    self.wfile.write(dati)

            def do_POST(self):  # noqa: N802 (nome imposto da BaseHTTPRequestHandler)
                if self.headers.get("X-Cockpit-Token") != padrone.token:
                    with padrone.lock:
                        padrone.non_autorizzati += 1
                    self._rispondi(401, {"errore": "token worker non valido"})
                    return
                corpo = self._corpo()
                percorso = self.path
                if percorso == "/api/v1/jobs/claim":
                    with padrone.lock:
                        padrone.claim_fatti.append(corpo)
                        job = padrone.job.pop(0) if padrone.job else None
                    self._rispondi(200, job) if job else self._rispondi(204)
                elif percorso.endswith("/heartbeat"):
                    with padrone.lock:
                        padrone.battiti.append(int(percorso.split("/")[-2]))
                        padrone.battiti_corpo.append(corpo)
                        stato = padrone.stato_heartbeat
                    if stato == 204:
                        self._rispondi(204)
                    else:
                        self._rispondi(stato, {"errore": "job non in corso per questo worker (lease perso?)"})
                elif percorso.endswith("/result"):
                    with padrone.lock:
                        padrone.risultati[int(percorso.split("/")[-2])] = corpo
                        padrone.eventi.append("result")
                        stato = padrone.stato_result
                    self._rispondi(stato) if stato == 204 else self._rispondi(stato, {"errore": "result rifiutato"})
                elif percorso == "/api/v1/ingest/messaggi":
                    with padrone.lock:
                        padrone.lotti.append(corpo)
                        stato = padrone.stato_ingest
                        risposta = padrone.risposta_ingest
                    if stato != 200:
                        self._rispondi(stato, {"errore": "ingest non disponibile"})
                    else:
                        self._rispondi(200, risposta or {"inseriti": len(corpo.get("messaggi", [])), "aggiornati": 0, "falliti": 0, "esiti": []})
                else:
                    self._rispondi(404, {"errore": "rotta sconosciuta: " + percorso})

            def do_GET(self):  # noqa: N802
                if self.headers.get("X-Cockpit-Token") != padrone.token:
                    with padrone.lock:
                        padrone.non_autorizzati += 1
                    self._rispondi(401, {"errore": "token worker non valido"})
                    return
                parti = urlsplit(self.path)
                if parti.path == "/api/v1/worker/caselle":
                    with padrone.lock:
                        padrone.richieste_caselle.append(parse_qs(parti.query).get("worker_id", [""])[0])
                        caselle = list(padrone.caselle_worker)
                    self._rispondi(200, caselle)
                else:
                    self._rispondi(404, {"errore": "rotta sconosciuta: " + self.path})

            def do_PUT(self):  # noqa: N802
                """PUT /api/v1/allegati/{id}/file?job_id=&lease_token=&worker_id= (voce 2.3): registra
                che cosa il worker ha caricato e con quale tentativo. Il corpo si legge tutto, come
                fa il server vero, e se ne calcola lo sha256 per confrontarlo con il result."""
                if self.headers.get("X-Cockpit-Token") != padrone.token:
                    with padrone.lock:
                        padrone.non_autorizzati += 1
                    self._rispondi(401, {"errore": "token worker non valido"})
                    return
                parti = urlsplit(self.path)
                pezzi = parti.path.split("/")
                if not (parti.path.startswith("/api/v1/allegati/") and parti.path.endswith("/file")):
                    self._rispondi(404, {"errore": "rotta sconosciuta: " + self.path})
                    return
                n = int(self.headers.get("Content-Length") or 0)
                h = hashlib.sha256()
                letti = 0
                while letti < n:
                    blocco = self.rfile.read(min(1 << 16, n - letti))
                    if not blocco:
                        break
                    h.update(blocco)
                    letti += len(blocco)
                with padrone.lock:
                    padrone.caricamenti.append({
                        "allegato_id": pezzi[4],
                        "query": {k: v[0] for k, v in parse_qs(parti.query).items()},
                        "bytes": letti, "sha256": h.hexdigest(),
                    })
                    padrone.eventi.append("upload")
                    stato = padrone.stato_upload
                if stato == 204:
                    self._rispondi(204)
                elif stato == 413:
                    self._rispondi(413, {"errore": "file oltre il limite di upload di 64 MB (max_upload_mb)"})
                elif stato == 409:
                    self._rispondi(409, {"errore": "tentativo non più valido (lease perso o job ripreso da un altro tentativo)"})
                else:
                    self._rispondi(stato, {"errore": "upload rifiutato"})

        self._srv = ThreadingHTTPServer(("127.0.0.1", 0), Gestore)
        self._thread = threading.Thread(target=self._srv.serve_forever, daemon=True)
        self._thread.start()
        return self

    def __exit__(self, *_exc) -> None:
        if self._srv is not None:
            self._srv.shutdown()
            self._srv.server_close()
        if self._thread is not None:
            self._thread.join(timeout=5)

    # ------------------------------------------------------------ comodità

    @property
    def url(self) -> str:
        assert self._srv is not None, "il server finto non è avviato"
        return f"http://127.0.0.1:{self._srv.server_address[1]}"

    def metti_job(self, job: dict) -> None:
        with self.lock:
            self.job.append(job)

    def config(self, **extra) -> dict:
        cfg = {"server_url": self.url, "token": self.token, "staging": ".", "consenti_invio": False}
        cfg.update(extra)
        return cfg
