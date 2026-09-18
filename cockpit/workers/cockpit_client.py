"""Modulo comune dei worker Cockpit (piano di correzione, voce 0.4 — W7).

Qui vivono le parti che worker_outlook.py e worker_analisi.py avevano copiato una per uno:
client HTTP, lettura della configurazione, log rotante, thread di battito (heartbeat) con flag di
arresto. Nessuna dipendenza esterna: solo la libreria standard (e protocollo.py, che e' altrettanto
nuda), così il modulo si prova senza Outlook.

Il thread di battito serve alla fase 1 (voce 1.5): il lavoro vero può bloccarsi dentro una chiamata
COM che non ritorna, quindi il battito non può stare sullo stesso thread. Quando il server risponde
409 (lease perso, oppure durata massima superata) il thread alza il flag di arresto: chi esegue il
lavoro lo controlla ai punti di ripresa e smette senza riportare nulla.
"""
from __future__ import annotations

import hashlib
import hmac
import http.client
import json
import logging
import logging.handlers
import os
import socket
import ssl
import sys
import threading
import tomllib
import urllib.error
import urllib.parse
import urllib.request
from typing import NamedTuple

from protocollo import ATTESA_CLAIM_S, BATTITO_MAX_S

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


class ImprontaSbagliata(RuntimeError):
    """Il certificato del server non è quello dichiarato in worker.toml (voce 2.4).

    NON è un errore di rete e non si ritenta: o il certificato del server è stato rifatto — e allora
    va scaricato un pacchetto nuovo dalla pagina Postazioni — oppure dall'altra parte c'è qualcun
    altro. Nei due casi la cosa giusta è la stessa: fermarsi e dirlo.
    """


def normalizza_impronta(s: str) -> str:
    """Un'impronta si copia in tanti modi: con i due punti, in maiuscolo, con gli spazi."""
    return "".join(c for c in (s or "").lower() if c in "0123456789abcdef")


class Diagnosi(NamedTuple):
    """Che cosa è andato storto e che cosa si fa, in una riga sola di log.

    Prima il loop dei due worker scriveva «server non raggiungibile» per QUALUNQUE errore, compresi i
    401 e i 403 — cioè proprio i casi in cui il server risponde benissimo e sta dicendo qualcosa di
    preciso. Chi leggeva il log andava a cercare la rete, il firewall o il servizio spento, mentre il
    problema era un token: è successo davvero, sul banco di prova, con le credenziali individuali
    appena introdotte. Un messaggio sbagliato non è un dettaglio estetico: manda a cercare altrove.

    categoria: rete | credenziale | autorizzazione | tentativo | server | richiesta | certificato
    grave:     True  → aspettare non lo risolve: lo risolve una persona (log.error)
    pausa_s:   0     → lasciare al chiamante il suo backoff; >0 → aspettare questo, senza raddoppiare
    """

    categoria: str
    grave: bool
    testo: str
    pausa_s: float


def diagnosi(e: BaseException) -> Diagnosi:
    """Classifica un errore del ciclo di lavoro di un worker.

    Sta qui e non nei due worker perché la distinzione è la stessa per tutti e due, e perché è
    provabile senza server: le sei righe che seguono sono i sei modi in cui una richiesta può non
    riuscire, e ognuna dice che cosa fare.
    """
    if isinstance(e, ImprontaSbagliata):
        return Diagnosi("certificato", True, f"IMPRONTA DEL CERTIFICATO SBAGLIATA: {e}", 60)
    if isinstance(e, ErroreHTTP):
        corpo = (e.corpo or "").strip()[:300]
        if e.stato == 401:
            return Diagnosi("credenziale", True,
                            f"credenziale RIFIUTATA dal server (401): {corpo}. Il server risponde, ma non "
                            "riconosce il token di questo worker: scaricare il pacchetto della postazione "
                            "dalla pagina Postazioni del Cockpit e sostituire worker.toml", 30)
        if e.stato == 403:
            return Diagnosi("autorizzazione", True,
                            f"il server rifiuta questo worker (403): {corpo}. La credenziale è valida ma non "
                            "autorizza ciò che sta chiedendo: controllare [[worker]] in cockpit.toml "
                            "(nome, postazione, caselle)", 30)
        if e.stato == 409:
            return Diagnosi("tentativo", False,
                            f"questo tentativo non è più valido (409): {corpo}. Il job è già tornato in coda "
                            "lato server: non c'è niente da riportare", 0)
        if e.stato >= 500:
            return Diagnosi("server", False, f"errore del server ({e.stato}): {corpo}", 0)
        return Diagnosi("richiesta", True,
                        f"richiesta rifiutata ({e.stato}): {corpo}. Ripeterla identica non cambia l'esito: "
                        "è un difetto da correggere, non un'attesa", 30)
    return Diagnosi("rete", False, f"server non raggiungibile ({e})", 0)


class _ConnessioneFissata(http.client.HTTPSConnection):
    """Una connessione TLS che accetta UN certificato solo: quello con l'impronta attesa."""

    impronta_attesa = ""

    def connect(self):
        super().connect()
        der = self.sock.getpeercert(binary_form=True)
        vera = hashlib.sha256(der or b"").hexdigest()
        if not hmac.compare_digest(vera, self.impronta_attesa):
            self.close()
            raise ImprontaSbagliata(
                "il server ha presentato un certificato diverso da quello atteso.\n"
                f"  atteso : {self.impronta_attesa}\n"
                f"  ricevuto: {vera}\n"
                "Se il certificato del server è stato rifatto, scaricare il pacchetto nuovo dalla pagina "
                "Postazioni. Non togliere `impronta` da worker.toml per far ripartire il worker: "
                "senza impronta non si sa più con chi si sta parlando."
            )


class _HandlerImpronta(urllib.request.HTTPSHandler):
    """Aggancia il controllo dell'impronta a urllib.

    La verifica della catena è spenta di proposito: il certificato del server è autofirmato, non c'è
    nessuna CA da consultare, e il controllo che conta lo fa l'impronta — che è più stretto, non più
    largo. Verificare la catena e NON l'impronta accetterebbe qualunque certificato firmato da
    chiunque il PC si fidi; qui ne passa uno solo.
    """

    def __init__(self, impronta: str):
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        super().__init__(context=ctx)
        self._impronta = impronta

    def https_open(self, req):
        def costruisci(host, **kw):
            c = _ConnessioneFissata(host, context=self._context, **kw)
            c.impronta_attesa = self._impronta
            return c
        return self.do_open(costruisci, req)


class Cockpit:
    """Client dell'API worker di cockpit.exe (urllib: nessuna dipendenza).

    `token` è il segreto INDIVIDUALE di questo worker (voce 2.4): il server lo cerca per sha256 in
    `worker_credenziale` e da lì sa chi sta chiamando. Un token condiviso da due worker non identifica
    nessuno e riceve 401.

    `impronta` è lo sha256 del certificato del server, quando il collegamento è https. Senza, verso un
    https si usa la verifica normale della catena — che su un certificato autofirmato non passa, ed è
    giusto così: il modo di collegarsi al Cockpit è avere la sua impronta.
    """

    def __init__(self, url: str, token: str, worker_id: str = "", impronta: str = ""):
        self.url = url.rstrip("/")
        self.token = token
        self.worker_id = worker_id
        self.impronta = normalizza_impronta(impronta)
        if self.impronta and len(self.impronta) != 64:
            raise ValueError(f"impronta non valida ({len(self.impronta)} caratteri esadecimali, attesi 64)")
        if self.impronta and not self.url.lower().startswith("https://"):
            raise ValueError("worker.toml dichiara un'impronta ma server_url non è https: "
                             "l'impronta non verrebbe verificata da nessuno")
        self._apri = urllib.request.build_opener(_HandlerImpronta(self.impronta)).open if self.impronta \
            else urllib.request.urlopen

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
            with self._apri(req, timeout=timeout) as r:
                if r.status == 204:
                    return None
                return json.loads(r.read().decode("utf-8") or "null")
        except urllib.error.HTTPError as e:
            testo = e.read().decode("utf-8", "ignore")
            raise ErroreHTTP(metodo, percorso, e.code, testo) from None

    # ------------------------------------------------------------ coda

    def claim(self, worker: str, worker_id: str, attesa_s: int = ATTESA_CLAIM_S, extra: dict | None = None) -> dict | None:
        """POST /api/v1/jobs/claim. `extra` porta ciò che la voce 2.2 aggiunge alla richiesta:
        postazione, outlook_ok, caselle_aperte, ultimo_arresto. Il server interseca le caselle
        con la credenziale del worker e assegna solo job che questo worker può eseguire."""
        corpo = {"worker": worker, "worker_id": worker_id, "attesa_s": attesa_s}
        if extra:
            corpo.update(extra)
        return self.chiama("POST", "/api/v1/jobs/claim", corpo, timeout=attesa_s + 15)

    def caselle_worker(self, worker_id: str) -> list[dict]:
        """GET /api/v1/worker/caselle: le caselle censite che QUESTO worker deve risolvere nel
        proprio profilo Outlook (voce 2.6). È l'elenco da cui il worker parte: uno store del profilo
        che non è qui non viene aperto né censito."""
        r = self.chiama("GET", "/api/v1/worker/caselle?" + urllib.parse.urlencode({"worker_id": worker_id}))
        return list(r or [])

    def heartbeat(self, job_id: int, worker_id: str, lease_token: str) -> None:
        self.chiama(
            "POST",
            f"/api/v1/jobs/{job_id}/heartbeat",
            {"worker_id": worker_id, "lease_token": lease_token},
        )

    def risultato(self, job_id: int, corpo: dict, worker_id: str = "", lease_token: str = "") -> None:
        """POST /api/v1/jobs/{id}/result: l'esito del lavoro, firmato dal tentativo che l'ha fatto.

        worker_id e lease_token dicono al server QUALE tentativo sta riportando il risultato: senza,
        il risultato di un tentativo scaduto si applicherebbe al tentativo subentrato. Il server li
        pretende (400 senza worker_id, 400 senza un lease_token che sia un UUID).

        ASSEGNAZIONE, non setdefault. `corpo` arriva da RisultatoRichiesta.model_dump(), e il modello
        dichiara `worker_id: str = ""` e `lease_token: str = ""`: le chiavi CI SONO GIÀ, vuote.
        setdefault le vedeva presenti e non le toccava, il server riceveva worker_id="" e rispondeva
        400; il job restava in_corso, il lease scadeva, lo scheduler lo rimetteva pronto e il worker
        lo rifaceva da capo, all'infinito. Chi passa qui il valore vero è il chiamante, che conosce
        il tentativo: è lui a vincere, sempre.
        """
        corpo = dict(corpo)
        corpo["worker_id"] = worker_id or corpo.get("worker_id") or self.worker_id
        corpo["lease_token"] = lease_token or corpo.get("lease_token") or ""
        self.chiama("POST", f"/api/v1/jobs/{job_id}/result", corpo)

    def ingest(self, richiesta: dict, timeout: int = 300) -> dict:
        return self.chiama("POST", "/api/v1/ingest/messaggi", richiesta, timeout=timeout)

    # ------------------------------------------------------------ upload (voce 2.3)

    def carica_file(self, allegato_id: str, job_id: int, lease_token: str, worker_id: str, percorso: str,
                    timeout: int = 900) -> None:
        """PUT /api/v1/allegati/{id}/file: il file scaricato da Outlook va al server, legato al tentativo.

        Il server lo tiene come .parte.<lease_token> e lo promuove ad allegato solo con il result
        valido dello stesso tentativo. Quindi: 409 = il tentativo non vale più, fermarsi e non riportare
        niente; 413 = oltre max_upload_mb, errore definitivo (ricaricare non lo rimpicciolisce);
        5xx = trasferimento interrotto, si ripete tale e quale.

        Il corpo è il file letto a blocchi: non si carica tutto in memoria, e Content-Length è
        dichiarato così il server può rifiutare un file troppo grande prima di riceverlo.
        """
        n = os.path.getsize(percorso)
        percorso_api = f"/api/v1/allegati/{allegato_id}/file?" + urllib.parse.urlencode(
            {"job_id": str(job_id), "lease_token": lease_token, "worker_id": worker_id or self.worker_id})
        with open(percorso, "rb") as f:
            req = urllib.request.Request(
                self.url + percorso_api, data=f, method="PUT",
                headers={"Content-Type": "application/octet-stream", "Content-Length": str(n),
                         "X-Cockpit-Token": self.token},
            )
            try:
                with self._apri(req, timeout=timeout):
                    return None
            except urllib.error.HTTPError as e:
                testo = e.read().decode("utf-8", "ignore")
                raise ErroreHTTP("PUT", percorso_api, e.code, testo) from None


def cadenza_battito(lease_s: float) -> float:
    """Ogni quanto battere durante un job: abbastanza spesso da rinnovare il lease PRIMA che scada, e
    mai piu' lento di BATTITO_MAX_S, che e' quanto il server aspetta prima di dare il worker per
    spento. I due vincoli sono diversi e vanno rispettati tutti e due: il primo protegge il lavoro,
    il secondo protegge cio' che l'operatore vede."""
    return min(max(5.0, lease_s / 4), BATTITO_MAX_S)


class Battito:
    """Thread che manda il heartbeat mentre il lavoro gira su un altro thread.

    Uso:

        with Battito(api, job_id, worker_id, job.lease_token, ogni_s=cadenza_battito(job.lease_s)) as b:
            fai_il_lavoro(controlla=b.controlla)     # b.controlla() alza ArrestoRichiesto dopo un 409
        if b.arresto.is_set():
            return                                   # niente result: il tentativo non è più nostro

    Il thread non tocca mai COM: chiama solo HTTP. Se il server risponde 409 alza `arresto`; se la
    rete è giù riprova, perché un buco di rete non significa aver perso il lease.

    USCITA FORZATA (C16). Alzare il flag non basta, ed è il punto delicato: una chiamata COM non è
    interrompibile: se Outlook è fermo dentro una finestra modale, il thread di lavoro non arriverà mai
    al prossimo punto di ripresa, e il flag resterebbe alzato per sempre. Nel frattempo il server ha
    già rimesso il job in coda e un altro tentativo lo sta rifacendo: questo processo, che nessuno
    aspetta più, continuerebbe a tenere aperto Outlook e a non prendere altri job.
    Quindi dopo `arresto_forzato_s` secondi di attesa il processo termina con codice 3, dopo aver
    scritto nel log quale job e quale fase lo tenevano bloccato. Il rilancio è dell'attività
    pianificata (fase 9.1), che ha il riavvio automatico proprio per questo.
    """

    def __init__(self, api: Cockpit, job_id: int, worker_id: str, lease_token: str = "",
                 ogni_s: float = BATTITO_MAX_S, arresto_forzato_s: float = 15.0, uscita=None):
        self.api = api
        self.job_id = job_id
        self.worker_id = worker_id
        self.lease_token = lease_token
        # Il tetto non e' negoziabile, e per questo sta qui e non nei chiamanti (blocco 2 del 3R):
        # dentro un job il worker non entra piu' in claim, quindi il battito e' l'unica cosa che lo
        # tiene riconoscibile dal server. Un battito piu' lento di BATTITO_MAX_S lo fa sparire dalla
        # testata mentre lavora — che era esattamente il difetto: `lease_s / 4` su un sync con lease
        # da dieci minuti voleva dire un battito ogni 150 secondi, e una soglia di 40. Piu' fitto va
        # sempre bene: costa una riga di UPDATE, e i test lo usano a 0,05 s.
        self.ogni_s = min(ogni_s, BATTITO_MAX_S)
        self.arresto_forzato_s = arresto_forzato_s
        # os._exit e non sys.exit: sys.exit alza un'eccezione nel thread del battito, dove non serve a
        # niente, e comunque l'interprete aspetterebbe il thread bloccato in COM. Qui si deve uscire.
        self.uscita = uscita or (lambda codice: os._exit(codice))
        # Dove lasciare il motivo dell'uscita forzata: al riavvio il worker lo legge e lo riporta
        # nel primo claim (worker_presenza.ultimo_arresto), poi lo cancella. None = non scrivere.
        self.marcatore_arresto: str | None = None
        self.arresto = threading.Event()
        self.motivo = ""
        self.fase = ""          # dove si trovava il lavoro: finisce nel log dell'uscita forzata
        self.uscita_forzata = False
        self._lavoro_finito = threading.Event()
        self._ferma = threading.Event()
        self._thread: threading.Thread | None = None

    def __enter__(self) -> "Battito":
        self._thread = threading.Thread(target=self._loop, name=f"battito-{self.job_id}", daemon=True)
        self._thread.start()
        return self

    def __exit__(self, *_exc) -> None:
        self.chiudi()

    def chiudi(self, attesa_s: float = 5.0) -> None:
        # prima di tutto: il lavoro è finito, quindi il conto alla rovescia dell'uscita forzata si ferma
        self._lavoro_finito.set()
        self._ferma.set()
        if self._thread is not None:
            self._thread.join(timeout=attesa_s)
            self._thread = None

    def segna_fase(self, fase: str) -> None:
        """Dichiara che cosa sta facendo il lavoro. Serve solo al log dell'uscita forzata, ma è la
        differenza fra «il worker si è riavviato» e «il worker si è riavviato mentre leggeva Inbox»."""
        self.fase = fase

    def controlla(self) -> None:
        """Da chiamare ai punti di ripresa del lavoro: alza ArrestoRichiesto se il lease è perso."""
        if self.arresto.is_set():
            raise ArrestoRichiesto(self.motivo or "tentativo non più valido")

    def _loop(self) -> None:
        while not self._ferma.wait(self.ogni_s):
            try:
                self.api.heartbeat(self.job_id, self.worker_id, self.lease_token)
            except ErroreHTTP as e:
                if e.tentativo_non_valido:
                    self.motivo = e.corpo[:200] or "409 dal server"
                    log.warning("job %d: lease perso (%s): chiedo l'arresto del lavoro", self.job_id, self.motivo)
                    self.arresto.set()
                    self._attendi_o_esci()
                    return
                log.warning("job %d: heartbeat rifiutato (%s)", self.job_id, e)
            except ERRORI_RETE as e:
                # rete giù: non è la perdita del lease, si riprova al battito successivo
                log.debug("job %d: heartbeat non recapitato (%s)", self.job_id, e)

    def _attendi_o_esci(self) -> None:
        """Concede al lavoro il tempo di fermarsi da solo; scaduto quello, termina il processo (C16)."""
        if self._lavoro_finito.wait(self.arresto_forzato_s):
            return                      # il lavoro ha visto il flag e si è fermato: tutto regolare
        motivo = (f"job {self.job_id}: il lavoro non si è fermato entro {self.arresto_forzato_s:.0f} s dalla "
                  f"perdita del lease (fase: {self.fase or 'sconosciuta'}; {self.motivo or '409'})")
        log.error(
            "%s. Il job è già tornato in coda lato server: esco con codice 3 e lascio riavviare l'attività pianificata.",
            motivo)
        scrivi_marcatore_arresto(self.marcatore_arresto, motivo)
        self.uscita_forzata = True
        self.uscita(3)


def scrivi_marcatore_arresto(percorso: str | None, motivo: str) -> None:
    """Lascia sul disco il motivo dell'uscita forzata (C16): il processo sta per terminare e non può
    dirlo al server; lo dirà il worker al riavvio, nel primo claim."""
    if not percorso:
        return
    try:
        with open(percorso, "w", encoding="utf-8") as f:
            f.write(motivo)
    except OSError:
        pass


def leggi_marcatore_arresto(percorso: str | None) -> str:
    """Il motivo lasciato dall'ultima uscita forzata, se c'è; il file viene rimosso."""
    if not percorso or not os.path.isfile(percorso):
        return ""
    try:
        with open(percorso, encoding="utf-8") as f:
            motivo = f.read().strip()
        os.remove(percorso)
        return motivo[:500]
    except OSError:
        return ""


# ---------------------------------------------------------------- configurazione e log


def carica_config(percorso: str, predefiniti: dict | None = None, sezione: str = "") -> dict:
    """worker.toml + variabili d'ambiente (che vincono sul file). Esce se manca il token.

    `sezione` è il tipo di worker ("outlook", "analisi"). Dalla voce 2.4 ogni worker ha un token
    SUO, e sullo stesso PC ne girano due: un file con un token solo non basta più. Le chiavi comuni
    (server_url, impronta, staging) restano in cima e le due tabelle `[outlook]` e `[analisi]`
    portano `token` e `worker_id` di ciascuno, sovrascrivendo ciò che c'è sopra.

    Un worker.toml vecchio, con il solo `token` in cima, continua a essere letto: è il server che
    dirà — con parole precise — che quel token è di due worker e non identifica nessuno.
    """
    cfg = {
        "server_url": "http://127.0.0.1:8080",
        "token": "",
        "impronta": "",
        "staging": os.path.join("..", "_staging"),
        "consenti_invio": False,
    }
    if predefiniti:
        cfg.update(predefiniti)
    if os.path.isfile(percorso):
        with open(percorso, "rb") as f:
            letto = tomllib.load(f)
        proprie = letto.pop(sezione, None) if sezione else None
        # Le altre sezioni di worker non riguardano questo worker: se finissero in cfg, `token`
        # dipenderebbe dall'ordine delle tabelle nel file.
        for t in ("outlook", "analisi"):
            letto.pop(t, None)
        cfg.update(letto)
        if isinstance(proprie, dict):
            cfg.update(proprie)
    for chiave, env in (
        ("server_url", "COCKPIT_URL"),
        ("token", "COCKPIT_TOKEN"),
        ("impronta", "COCKPIT_IMPRONTA"),
        ("staging", "COCKPIT_STAGING"),
        ("worker_id", "COCKPIT_WORKER_ID"),
    ):
        if os.environ.get(env):
            cfg[chiave] = os.environ[env]
    if not cfg["token"]:
        dove = f"[{sezione}].token" if sezione else "token"
        sys.exit(f"token mancante: {percorso} {dove} o variabile COCKPIT_TOKEN. "
                 "Il pacchetto con i token di questo PC si scarica dalla pagina Postazioni del Cockpit.")
    controlla_staging(cfg["staging"], percorso)
    return cfg


# I nomi che il SERVER si riserva dentro il proprio staging: `_contenuti` e' il magazzino indirizzato
# per contenuto (un file = uno sha256), `_parti` sono i trasferimenti in corso.
CARTELLE_DEL_SERVER = ("_contenuti", "_parti")


def controlla_staging(staging: str, percorso: str) -> None:
    """Il `staging` del worker e' una cartella SUA: file temporanei, log, restrict.json.

    Non e' lo staging del server, e soprattutto non e' una delle cartelle che il server si riserva
    dentro il proprio. Puntarlo li' dentro non da' nessun errore — il worker lavora, il server lavora
    — ma i due finiscono per scrivere nello stesso posto: i log e i file di appoggio del worker si
    mescolano ai contenuti, e tutto cio' che pulisce la cartella di un worker cancella i contenuti a
    cui i documenti confermati rimandano. Un fascicolo che aspetta la copia si trova il file sparito.

    E' successo su questo banco, e non l'ha segnalato nessuno: per questo e' un rifiuto all'avvio e
    non un avviso nel log.
    """
    parti = [p.lower() for p in os.path.normpath(os.path.abspath(staging)).split(os.sep)]
    for riservata in CARTELLE_DEL_SERVER:
        if riservata in parti:
            sys.exit(
                f"staging non valido: {staging}\n"
                f"  contiene «{riservata}», che e' una cartella del SERVER (il magazzino dei contenuti).\n"
                f"  Il worker deve avere una cartella sua, fuori dallo staging del server: e' li' che\n"
                f"  tiene file temporanei, log e restrict.json, e tutto cio' che ci mette lo cancella.\n"
                f"  Correggere `staging` in {percorso}.")


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
