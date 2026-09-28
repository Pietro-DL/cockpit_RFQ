"""Server finto dell'API worker: regge i test L2 dei worker senza cockpit.exe né PostgreSQL.

Non è un mock delle regole del server — quelle si provano in L4 contro PostgreSQL vero. Qui serve
solo a far girare il ciclo del worker e a osservare che cosa manda: quali job prende, quanti battiti
invia, che cosa riporta, come si comporta quando una risposta è 409, 503 o non arriva affatto.

UNA COSA PERÒ LA VERIFICA, ED È OBBLIGATORIA: l'identità del tentativo. Il 15/09/2026 il worker vero
ha girato a vuoto per un pomeriggio perché mandava `worker_id: ""` a ogni result e il server vero
rispondeva 400; qui passava, perché questo server accettava qualunque cosa. Un server finto che
accetta ciò che quello vero rifiuta non è un banco di prova: è un test che passa per costruzione.
Quindi claim, heartbeat, result, ingest e upload pretendono worker_id e lease_token (400 senza) e
rispondono 409 a chi presenta un tentativo diverso da quello consegnato dal claim, come fa il
predicato SQL del server vero. Ciò che resta finto è il resto: la coda, il database, le regole.

    with ServerFinto() as s:
        s.metti_job({"job_id": 1, "tipo": "sync_outlook", "payload": {}, "tentativi": 1, "lease_s": 120})
        ...
        assert s.risultati[1]["esito"] == "ok"
"""
from __future__ import annotations

import hashlib
import json
import threading
import uuid
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
        # GET /allegati/{id}/contenuto (7C.1, P0): i byte che il server «ha in staging», per allegato_id
        self.contenuti: dict[str, bytes] = {}
        self.scaricamenti: list[dict] = []     # ogni GET del contenuto: id e query (tentativo)
        self.stato_contenuto = 200             # forzabile a 503 per provare la ripetizione
        self.eventi: list[str] = []            # ordine delle chiamate che contano: "contenuto", "upload", "result"
        self.risposta_ingest: dict | None = None
        # GET /api/v1/worker/caselle (voce 2.6): le caselle che il server chiede al worker di
        # risolvere. Vuoto = il worker non ha niente da risolvere e non apre Outlook.
        self.caselle_worker: list[dict] = []
        self.richieste_caselle: list[str] = []      # worker_id di ogni GET /worker/caselle
        self.non_autorizzati = 0
        # job_id -> il tentativo consegnato dal claim: chi si presenta con un altro riceve 409
        self.tentativi: dict[int, dict] = {}
        self.respinte: list[dict] = []         # rotta, stato e motivo di ogni richiesta rifiutata
        self._srv: ThreadingHTTPServer | None = None
        self._thread: threading.Thread | None = None

    # ------------------------------------------------------------ verifica del tentativo

    def verifica_tentativo(self, rotta: str, job_id, worker_id, lease_token) -> tuple[int, str]:
        """(0, "") se il tentativo è accettabile, altrimenti (stato, motivo) come il server vero.

        Il server vero fa due cose distinte: `tentativo()` rifiuta con 400 una richiesta che non dice
        CHI è e con quale lease sta scrivendo; il predicato SQL rifiuta con 409 chi lo dice ma non è
        più il tentativo in corso. Qui si riproducono entrambe, perché il worker le tratta in due modi
        opposti: 400 è un difetto del client da correggere, 409 è «fermati e non riportare niente».
        Il formato UUID del token non si verifica: lo genera PostgreSQL e il worker lo riporta e
        basta, quindi è materia di L4.
        """
        stato, motivo = 0, ""
        if not str(worker_id or "").strip():
            stato, motivo = 400, "worker_id mancante"
        elif not str(lease_token or "").strip():
            stato, motivo = 400, "lease_token mancante o non valido"
        else:
            atteso = self.tentativi.get(job_id)
            if atteso and (atteso["lease_token"] != lease_token or atteso["worker_id"] != worker_id):
                stato = 409
                motivo = "tentativo non più valido (lease perso o job ripreso da un altro tentativo)"
        if stato:
            with self.lock:
                self.respinte.append({"rotta": rotta, "job_id": job_id, "stato": stato, "errore": motivo})
        return stato, motivo

    # ------------------------------------------------------------ ciclo di vita

    def __enter__(self) -> "ServerFinto":
        padrone = self

        class Gestore(BaseHTTPRequestHandler):
            def log_message(self, *_a):  # niente rumore sullo stderr dei test
                pass

            def _corpo(self) -> dict:
                n = int(self.headers.get("Content-Length") or 0)
                return json.loads(self.rfile.read(n) or b"{}") if n else {}

            def _scarta_corpo(self) -> None:
                """Legge e butta via il corpo della richiesta prima di rifiutarla.

                Un server che risponde senza aver letto ciò che gli è stato mandato chiude la
                connessione lasciando dati non consumati, e lo stack TCP manda un RST: su Windows il
                client che sta ancora leggendo la risposta riceve «connessione interrotta» invece del
                401. Il risultato è un test che fallisce una volta ogni tre per un motivo che non ha
                niente a che vedere con quello che prova. Il server vero legge sempre la richiesta;
                questo deve fare lo stesso.
                """
                n = int(self.headers.get("Content-Length") or 0)
                while n > 0:
                    letto = self.rfile.read(min(n, 1 << 16))
                    if not letto:
                        return
                    n -= len(letto)

            def _non_autorizzato(self) -> None:
                self._scarta_corpo()
                with padrone.lock:
                    padrone.non_autorizzati += 1
                self._rispondi(401, {"errore": "token worker non valido"})

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
                    self._non_autorizzato()
                    return
                corpo = self._corpo()
                percorso = self.path
                if percorso == "/api/v1/jobs/claim":
                    if not str(corpo.get("worker") or "").strip() or not str(corpo.get("worker_id") or "").strip():
                        self._rispondi(400, {"errore": "worker_id mancante"})
                        return
                    with padrone.lock:
                        padrone.claim_fatti.append(corpo)
                        job = padrone.job.pop(0) if padrone.job else None
                        if job:
                            # da qui in poi solo questo tentativo può scrivere su questo job
                            padrone.tentativi[job["job_id"]] = {
                                "worker_id": corpo["worker_id"], "lease_token": job.get("lease_token", "")}
                    self._rispondi(200, job) if job else self._rispondi(204)
                elif percorso.endswith("/heartbeat"):
                    job_id = int(percorso.split("/")[-2])
                    stato, motivo = padrone.verifica_tentativo(
                        "heartbeat", job_id, corpo.get("worker_id"), corpo.get("lease_token"))
                    if stato:
                        self._rispondi(stato, {"errore": motivo})
                        return
                    with padrone.lock:
                        padrone.battiti.append(job_id)
                        padrone.battiti_corpo.append(corpo)
                        stato = padrone.stato_heartbeat
                    if stato == 204:
                        self._rispondi(204)
                    else:
                        self._rispondi(stato, {"errore": "job non in corso per questo worker (lease perso?)"})
                elif percorso.endswith("/result"):
                    job_id = int(percorso.split("/")[-2])
                    stato, motivo = padrone.verifica_tentativo(
                        "result", job_id, corpo.get("worker_id"), corpo.get("lease_token"))
                    if stato:
                        # un risultato non attribuibile non si applica: il job resta come sta
                        self._rispondi(stato, {"errore": motivo})
                        return
                    with padrone.lock:
                        padrone.risultati[job_id] = corpo
                        padrone.eventi.append("result")
                        stato = padrone.stato_result
                    self._rispondi(stato) if stato == 204 else self._rispondi(stato, {"errore": "result rifiutato"})
                elif percorso == "/api/v1/ingest/messaggi":
                    stato, motivo = padrone.verifica_tentativo(
                        "ingest", corpo.get("job_id"), corpo.get("worker_id"), corpo.get("lease_token"))
                    if stato:
                        self._rispondi(stato, {"errore": "ingest senza tentativo valido: " + motivo})
                        return
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
                    self._non_autorizzato()
                    return
                parti = urlsplit(self.path)
                if parti.path == "/api/v1/worker/caselle":
                    with padrone.lock:
                        padrone.richieste_caselle.append(parse_qs(parti.query).get("worker_id", [""])[0])
                        caselle = list(padrone.caselle_worker)
                    self._rispondi(200, caselle)
                elif parti.path.startswith("/api/v1/allegati/") and parti.path.endswith("/contenuto"):
                    # GET /api/v1/allegati/{id}/contenuto (7C.1, P0): i byte di un allegato, solo al
                    # tentativo che deve analizzarlo. Il contenuto lo mette il test in `contenuti`.
                    query = {k: v[0] for k, v in parse_qs(parti.query).items()}
                    allegato_id = parti.path.split("/")[4]
                    try:
                        job_id = int(query.get("job_id", ""))
                    except ValueError:
                        self._rispondi(400, {"errore": "job_id mancante o non valido"})
                        return
                    stato, motivo = padrone.verifica_tentativo(
                        "contenuto", job_id, query.get("worker_id"), query.get("lease_token"))
                    if stato:
                        self._rispondi(stato, {"errore": motivo})
                        return
                    with padrone.lock:
                        padrone.scaricamenti.append({"allegato_id": allegato_id, "query": query})
                        padrone.eventi.append("contenuto")
                        stato = padrone.stato_contenuto
                        dati = padrone.contenuti.get(allegato_id)
                    if stato != 200:
                        self._rispondi(stato, {"errore": "contenuto non disponibile"})
                    elif dati is None:
                        self._rispondi(410, {"errore": "contenuto non presente in staging: usa Riscarica"})
                    else:
                        self.send_response(200)
                        self.send_header("Content-Type", "application/octet-stream")
                        self.send_header("Content-Length", str(len(dati)))
                        self.send_header("X-Cockpit-Sha256", hashlib.sha256(dati).hexdigest())
                        self.end_headers()
                        self.wfile.write(dati)
                else:
                    self._rispondi(404, {"errore": "rotta sconosciuta: " + self.path})

            def do_PUT(self):  # noqa: N802
                """PUT /api/v1/allegati/{id}/file?job_id=&lease_token=&worker_id= (voce 2.3): registra
                che cosa il worker ha caricato e con quale tentativo. Il corpo si legge tutto, come
                fa il server vero, e se ne calcola lo sha256 per confrontarlo con il result."""
                if self.headers.get("X-Cockpit-Token") != padrone.token:
                    self._non_autorizzato()
                    return
                parti = urlsplit(self.path)
                pezzi = parti.path.split("/")
                if not (parti.path.startswith("/api/v1/allegati/") and parti.path.endswith("/file")):
                    self._rispondi(404, {"errore": "rotta sconosciuta: " + self.path})
                    return
                query = {k: v[0] for k, v in parse_qs(parti.query).items()}
                try:
                    job_id = int(query.get("job_id", ""))
                except ValueError:
                    padrone.respinte.append({"rotta": "upload", "job_id": None, "stato": 400,
                                             "errore": "job_id mancante o non valido"})
                    self._rispondi(400, {"errore": "job_id mancante o non valido"})
                    return
                stato, motivo = padrone.verifica_tentativo(
                    "upload", job_id, query.get("worker_id"), query.get("lease_token"))
                if stato:
                    self._rispondi(stato, {"errore": motivo})
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
                        "allegato_id": pezzi[4], "query": query,
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
        """Accoda un job da consegnare al prossimo claim.

        Il lease_token non è facoltativo: il server vero lo genera con gen_random_uuid() a ogni
        claim ed è la firma con cui il worker dovrà riportare il risultato. Un job finto senza token
        farebbe passare un worker che non lo riporta, che è il difetto corretto il 15/09/2026.
        """
        job = dict(job)
        job.setdefault("lease_token", str(uuid.uuid4()))
        with self.lock:
            self.job.append(job)

    def config(self, **extra) -> dict:
        cfg = {"server_url": self.url, "token": self.token, "staging": ".", "consenti_invio": False}
        cfg.update(extra)
        return cfg
