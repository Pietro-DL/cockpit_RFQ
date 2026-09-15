"""worker-outlook: client HTTP di cockpit.exe che esegue i job di tipo 'outlook' via COM.

    python worker_outlook.py [--config worker.toml] [--una-volta] [--cartelle] [--caselle]

Loop: POST /api/v1/jobs/claim (long-poll) → esegue → POST /api/v1/jobs/{id}/result.
Un solo thread: le chiamate COM sono serializzate per costruzione. Se Outlook non risponde il job
fallisce (ritentato dal server con backoff) e il worker resta vivo. Se il server è giù o si riavvia,
il worker aspetta e riprova: non termina mai da solo.

Dalla voce 2.6 il worker, prima di chiedere lavoro, chiede al server QUALI caselle deve servire
(GET /api/v1/worker/caselle) e le risolve nello store del proprio profilo Outlook: il claim dichiara
le caselle risolte e il server gli assegna solo job di quelle. Uno store del profilo che il server
non ha censito viene ignorato. I payload dei job portano casella_id + entry_id + Message-ID, mai uno
StoreID: lo store lo mette il worker, dal proprio profilo.
"""
from __future__ import annotations

import argparse
import logging
import os
import socket
import time
from datetime import datetime, timedelta, timezone

import pywintypes

from cockpit_client import (ERRORI_RETE, ArrestoRichiesto, Battito, Cockpit, ErroreHTTP, carica_config,
                            configura_log, leggi_marcatore_arresto, nome_worker)
from contratti import (CartellaEsito, CursoreLotto, IngestRichiesta, Job, PayloadApriElemento, PayloadCreaBozza,
                       PayloadSegnaLetto, PayloadSpostaCartella, PayloadStageAllegato, PayloadSyncOutlook,
                       RisultatoBozza, RisultatoElemento, RisultatoRichiesta, RisultatoStage, RisultatoSync)
from outlook_com import ErroreDefinitivo, Outlook

log = logging.getLogger("worker")


class ErroreStoreLocale(Exception):
    """La casella del job non è risolta nel profilo Outlook di questo PC: il job non è eseguibile
    QUI, ma può esserlo altrove (o qui, dopo aver sistemato il profilo). Non definitivo."""


class Worker:
    def __init__(self, cfg: dict):
        self.cfg = cfg
        self.api = Cockpit(cfg["server_url"], cfg["token"])
        self.worker_id = nome_worker("outlook", cfg)
        self.staging = os.path.abspath(cfg["staging"])
        os.makedirs(self.staging, exist_ok=True)
        self.outlook: Outlook | None = None
        self.battito: Battito | None = None
        # quanto si concede al lavoro per fermarsi da solo dopo un 409, prima dell'uscita forzata (C16)
        self.arresto_forzato_s = float(cfg.get("arresto_forzato_s", 15))
        self.postazione = str(cfg.get("postazione") or socket.gethostname()).upper()
        # voce 2.6: casella_id → store_id nel PROFILO DI QUESTO PC, risolto da risolvi_caselle
        self.store: dict[str, str] = {}
        self.caselle: list[dict] = []            # ciò che il server chiede di servire
        self.outlook_ok = True
        self._risolte_il = 0.0
        self.risolvi_ogni_s = float(cfg.get("risolvi_caselle_ogni_s", 300))
        self.marcatore_arresto = os.path.join(self.staging, "ultimo_arresto.txt")
        self.ultimo_arresto = leggi_marcatore_arresto(self.marcatore_arresto)

    def controlla(self) -> None:
        """Punto di ripresa: se il battito ha perso il lease, il lavoro si ferma qui.

        Va chiamata dove il lavoro NON è dentro una chiamata COM, cioè dove fermarsi è possibile e non
        lascia niente a metà. Dove invece COM non ritorna, a fermare il processo è l'uscita forzata del
        battito dopo 15 secondi (C16): sono i due mezzi dello stesso meccanismo, non alternative.
        """
        if self.battito is not None:
            self.battito.controlla()

    def ol(self) -> Outlook:
        if self.outlook is None:
            self.outlook = Outlook(consenti_invio=bool(self.cfg.get("consenti_invio", False)))
        return self.outlook

    # ------------------------------------------------------------ caselle → store locale (voce 2.6)

    def risolvi_caselle(self, forza: bool = False) -> None:
        """Chiede al server le caselle da servire e le risolve nel profilo Outlook di questo PC.

        Si ripete ogni `risolvi_ogni_s` (una cassetta aggiunta al profilo viene vista senza
        riavviare) e a ogni errore di store. Con zero caselle da servire Outlook NON viene aperto:
        non c'è niente da risolvere e niente che il worker possa eseguire.
        """
        if not forza and self.store and time.time() - self._risolte_il < self.risolvi_ogni_s:
            return
        self.caselle = self.api.caselle_worker(self.worker_id)
        self._risolte_il = time.time()
        if not self.caselle:
            log.warning("il server non mi assegna nessuna casella: controllare [[worker]].caselle per %s", self.worker_id)
            self.store = {}
            return
        try:
            trovate, mancanti = self.ol().risolvi_caselle(self.caselle)
            self.outlook_ok = True
        except pywintypes.com_error as e:
            log.error("Outlook non risponde, nessuna casella risolta: %s", e)
            self.outlook = None
            self.outlook_ok = False
            self.store = {}
            return
        self.store = trovate
        if mancanti:
            log.warning("caselle censite non trovate in questo profilo: %s (il server non mi assegnerà i loro job)", ", ".join(mancanti))

    def store_di(self, casella_id) -> str:
        """Lo store locale della casella di un job. Se non è risolto, riprova una volta: il profilo
        può essere cambiato. Se ancora manca, il job non è eseguibile QUI — e non doveva arrivare,
        perché il claim dichiara solo le caselle risolte."""
        cid = str(casella_id or "")
        if cid in self.store:
            return self.store[cid]
        self.risolvi_caselle(forza=True)
        if cid in self.store:
            return self.store[cid]
        raise ErroreStoreLocale(f"la casella {cid[:8]} non è risolta nel profilo Outlook di {self.postazione}")

    def dichiarazione(self) -> dict:
        """Che cosa il claim dichiara di questo worker (voce 2.2): il server interseca con la
        credenziale e non si fida di ciò che c'è qui."""
        d = {
            "postazione": self.postazione,
            "outlook_ok": self.outlook_ok,
            "caselle_aperte": [{"casella_id": cid, "store_id": sid} for cid, sid in sorted(self.store.items())],
        }
        if self.ultimo_arresto:
            d["ultimo_arresto"] = self.ultimo_arresto
        return d

    # ------------------------------------------------------------ loop

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s su %s → %s", self.worker_id, self.postazione, self.api.url)
        attesa = 5
        while True:
            try:
                self.risolvi_caselle()
                r = self.api.claim("outlook", self.worker_id, extra=self.dichiarazione())
                self.ultimo_arresto = ""            # riportato una volta: il server lo conserva
                job = Job.model_validate(r) if r else None
                attesa = 5
            except ErroreHTTP as e:
                if e.stato == 403:
                    # credenziale assente o postazione sbagliata: non si risolve aspettando, ma
                    # nemmeno uscendo — chi corregge cockpit.toml deve trovare il worker vivo
                    log.error("il server rifiuta questo worker: %s", e.corpo[:300])
                    if una_volta:
                        return
                    time.sleep(30)
                    continue
                log.warning("server non raggiungibile (%s): riprovo fra %d s", e, attesa)
                time.sleep(attesa)
                attesa = min(attesa * 2, 30)
                continue
            except ERRORI_RETE as e:
                log.warning("server non raggiungibile (%s): riprovo fra %d s", e, attesa)
                time.sleep(attesa)
                attesa = min(attesa * 2, 30)
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
        # Il battito sta su un thread suo per tutta la durata del job, non solo durante il sync: una
        # chiamata COM lunga (salva_allegato su un allegato da 200 MB, crea_bozza con Outlook occupato)
        # farebbe scadere il lease esattamente come una scansione lunga. Se il server risponde 409, il
        # thread alza il flag e concede 15 secondi al lavoro per fermarsi da solo; scaduti quelli, il
        # processo esce con codice 3 e l'attività pianificata lo riavvia (C16).
        with Battito(self.api, job.job_id, self.worker_id, job.lease_token,
                     ogni_s=max(5.0, job.lease_s / 4), arresto_forzato_s=self.arresto_forzato_s) as b:
            b.marcatore_arresto = self.marcatore_arresto
            self.battito = b
            b.segna_fase(job.tipo)
            try:
                dati = self.dispatch(job)
                ris = RisultatoRichiesta(esito="ok", dati=dati)
            except ArrestoRichiesto as e:
                # Il tentativo non è più nostro: il job è già tornato in coda lato server e qualcun altro
                # lo sta rifacendo. Riportare qualcosa adesso significherebbe scrivere sopra al suo
                # lavoro, quindi non si riporta niente: si passa al job successivo.
                log.warning("job %d interrotto: %s", job.job_id, e)
                self.battito = None
                return
            except ErroreDefinitivo as e:
                log.error("job %d errore definitivo: %s", job.job_id, e)
                ris = RisultatoRichiesta(esito="errore", errore=str(e), definitivo=True)
            except ErroreStoreLocale as e:
                # non definitivo: un altro worker che serve quella casella (o questo, dopo che il
                # profilo è stato sistemato) può farcela. Il claim non dovrebbe assegnarlo più qui.
                log.error("job %d: %s", job.job_id, e)
                ris = RisultatoRichiesta(esito="errore", errore=str(e))
            except pywintypes.com_error as e:
                log.exception("job %d errore COM", job.job_id)
                self.outlook = None  # riconnette al prossimo job (Outlook chiuso/riavviato)
                ris = RisultatoRichiesta(esito="errore", errore=f"COM: {e}")
            except Exception as e:  # noqa: BLE001 - il worker non deve mai morire per un job
                log.exception("job %d errore", job.job_id)
                ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])
            perso = b.arresto.is_set()
        self.battito = None
        if perso:
            log.warning("job %d: lease perso durante il lavoro, risultato non riportato", job.job_id)
            return
        try:
            self.api.risultato(job.job_id, ris.model_dump(mode="json"), self.worker_id, job.lease_token)
        except ErroreHTTP as e:
            if e.tentativo_non_valido:
                # il lease era già perso: il job è tornato in coda ed è stato ripreso da un altro
                # tentativo. Riportare il risultato adesso sarebbe scriverlo sopra al lavoro altrui.
                log.warning("job %d: risultato scartato dal server (409): il tentativo non era più valido", job.job_id)
            else:
                log.error("impossibile riportare il risultato del job %d: %s", job.job_id, e)
        except Exception as e:  # noqa: BLE001 - il lease scade e il server lo rimette in coda
            log.error("impossibile riportare il risultato del job %d: %s", job.job_id, e)
        log.info("job %d %s → %s in %.1fs", job.job_id, job.tipo, ris.esito, time.time() - t0)

    def dispatch(self, job: Job) -> dict:
        p = job.payload
        # Il payload porta casella_id, mai uno store_id (M12): lo store è di questo profilo e lo
        # mette il worker. Un job senza casella non è eseguibile: non si «prova» sullo store
        # predefinito, che potrebbe essere la casella sbagliata.
        match job.tipo:
            case "sync_outlook":
                return self.sync(job, PayloadSyncOutlook.model_validate(p))
            case "stage_allegato":
                return self.stage(job, PayloadStageAllegato.model_validate(p))
            case "crea_bozza_outlook":
                b = PayloadCreaBozza.model_validate(p)
                store = self.store_di(b.casella_id) if b.tipo != "nuovo" or b.casella_id else ""
                entry_id, inviata = self.ol().crea_bozza(b, store)
                return RisultatoBozza(entry_id=entry_id, inviata=inviata).model_dump(mode="json")
            case "apri_elemento_outlook":
                a = PayloadApriElemento.model_validate(p)
                return RisultatoElemento(**self.ol().apri(a.entry_id, self.store_di(a.casella_id), a.message_id)).model_dump(mode="json")
            case "sposta_in_cartella":
                s = PayloadSpostaCartella.model_validate(p)
                return RisultatoElemento(**self.ol().sposta(s.entry_id, self.store_di(s.casella_id), s.cartella, s.message_id)).model_dump(mode="json")
            case "segna_letto":
                l = PayloadSegnaLetto.model_validate(p)
                return RisultatoElemento(**self.ol().segna_letto(l.entry_id, self.store_di(l.casella_id), l.letto, l.message_id)).model_dump(mode="json")
        raise ErroreDefinitivo(f"tipo job sconosciuto per il worker outlook: {job.tipo}")

    # ------------------------------------------------------------ download di un allegato (voce 2.3)

    def stage(self, job: Job, s: PayloadStageAllegato) -> dict:
        """Salva l'allegato da Outlook in una cartella temporanea locale e lo CARICA al server.

        Fino alla fase 1 il file veniva scritto direttamente nello staging del server e il result ne
        dichiarava il percorso: funzionava solo con worker e server sullo stesso PC. Ora il file
        viaggia con PUT, legato al tentativo (job_id + lease_token), e il server lo promuove ad
        allegato solo con il result valido dello stesso tentativo: un tentativo scaduto a metà upload
        non consegna niente. Il file locale si cancella in ogni caso: qui è solo di passaggio.
        """
        locale = os.path.join(self.staging, "tmp", str(job.job_id))
        store = self.store_di(s.casella_id)
        if self.battito is not None:
            self.battito.segna_fase(f"salva allegato {s.indice} di {s.message_id or s.entry_id[:16]}")
        dest, sha, n, dove = self.ol().salva_allegato(s.entry_id, store, s.indice, s.nome_file, locale, s.message_id)
        try:
            self.controlla()                       # punto di ripresa: fuori da COM, prima del trasferimento
            if self.battito is not None:
                self.battito.segna_fase(f"upload allegato {s.allegato_id} ({n} byte)")
            self._carica(job, str(s.allegato_id), dest, n)
        finally:
            try:
                os.remove(dest)
                os.rmdir(locale)
            except OSError:
                pass
        return RisultatoStage(allegato_id=s.allegato_id, sha256=sha, bytes=n, **dove).model_dump(mode="json")

    def _carica(self, job: Job, allegato_id: str, percorso: str, n: int) -> None:
        try:
            self.api.carica_file(allegato_id, job.job_id, job.lease_token, self.worker_id, percorso)
        except ErroreHTTP as e:
            if e.tentativo_non_valido:
                raise ArrestoRichiesto(f"upload rifiutato: {e.corpo[:200]}") from e
            if e.stato == 413:
                # il limite è del server (max_upload_mb) e ricaricare non cambia la dimensione del file
                raise ErroreDefinitivo(f"allegato di {n} byte oltre il limite di upload del server: {e.corpo[:300]}") from e
            if not e.ritentabile:
                raise ErroreDefinitivo(f"upload non accettato dal server: {e}") from e
            raise                                   # 5xx: il job fallisce senza «definitivo» e viene ritentato
        log.info("allegato %s caricato al server (%d byte)", allegato_id, n)

    # ------------------------------------------------------------ sync

    def sync(self, job: Job, p: PayloadSyncOutlook) -> dict:
        # La casella del job decide QUALE store leggere (voce 2.6): «Posta in arrivo» di Commerciale
        # non è la Posta in arrivo del profilo. Senza casella il sync non sa che cosa leggere.
        if p.casella_id is None:
            raise ErroreDefinitivo("sync senza casella_id: non so quale store leggere")
        store = self.store_di(p.casella_id)
        esiti = []
        for c in p.cartelle:
            dal = (c.ultimo_received - timedelta(seconds=p.sovrapposizione_s)) if c.ultimo_received else p.dal
            if dal.tzinfo is None:
                dal = dal.replace(tzinfo=timezone.utc)
            al = p.al
            if al is not None and al.tzinfo is None:
                al = al.replace(tzinfo=timezone.utc)
            if self.battito is not None:
                self.battito.segna_fase(f"sync {c.cartella}")
            esito = CartellaEsito(cartella=c.cartella, ultimo_received=c.ultimo_received)
            lotto: list = []
            try:
                self.controlla()
                for m in self.ol().leggi(c.cartella, dal, al=al, store_id=store):
                    # punto di ripresa: fra un elemento e l'altro il lavoro è fuori da COM, quindi qui
                    # un arresto chiesto dal battito si può rispettare senza lasciare niente a metà
                    self.controlla()
                    lotto.append(m)
                    # W2 CHIUSO (voce 2.1). Il cursore avanza su ricevuto_il, che è il ReceivedTime in
                    # questa casella: lo stesso valore su cui filtra la scansione qui sopra. Prima
                    # avanzava su data_evento, che per la Posta inviata è SentOn — un'altra grandezza —
                    # e una mail scritta lunedì e inviata giovedì poteva spingere il cursore oltre
                    # elementi non ancora letti, che nessuno avrebbe più riletto.
                    quando = m.ricevuto_il or m.data_evento
                    if al is None and quando and (esito.ultimo_received is None or quando > esito.ultimo_received):
                        esito.ultimo_received = quando
                    if len(lotto) >= p.lotto:
                        esito.n_messaggi += self._invia(job, p, c.cartella, lotto, esito.ultimo_received)
                        lotto = []
                if lotto:
                    esito.n_messaggi += self._invia(job, p, c.cartella, lotto, esito.ultimo_received)
            except ErroreDefinitivo as e:
                esito.errore = str(e)
                log.error("cartella %s: %s", c.cartella, e)
            log.info("sync %s: finestra [%s, %s] → %d messaggi, cursore %s", c.cartella, dal.isoformat(), al.isoformat() if al else "now", esito.n_messaggi, esito.ultimo_received)
            esiti.append(esito)
        return RisultatoSync(cartelle=esiti).model_dump(mode="json")

    def _invia(self, job: Job, p: PayloadSyncOutlook, cartella: str, lotto: list, fin_qui) -> int:
        """Manda un lotto e fa avanzare il cursore INSIEME a esso.

        Il cursore viaggia nella stessa richiesta degli elementi perché il server lo scrive nella
        stessa transazione: se il lotto non entra, il cursore non si muove e il lotto si ripete
        identico. Il contrario — cursore avanzato e lotto perso — vorrebbe dire messaggi mai
        acquisiti che nessuno andrà più a cercare.
        """
        richiesta = IngestRichiesta(
            messaggi=lotto,
            casella_id=p.casella_id,
            job_id=job.job_id,
            lease_token=job.lease_token,
            worker_id=self.worker_id,
            cursore=CursoreLotto(cartella=cartella, ultimo_received=fin_qui) if fin_qui else None,
        )
        try:
            r = self.api.ingest(richiesta.model_dump(mode="json"))
        except ErroreHTTP as e:
            if e.tentativo_non_valido:
                raise ArrestoRichiesto(f"ingest rifiutato: {e.corpo[:200]}") from e
            raise
        falliti = r.get("falliti", 0)
        if falliti:
            log.warning("ingest: %d inseriti, %d aggiornati, %d SCARTATI (vedi /admin/scarti)",
                        r.get("inseriti", 0), r.get("aggiornati", 0), falliti)
        else:
            log.info("ingest: %d inseriti, %d aggiornati", r.get("inseriti", 0), r.get("aggiornati", 0))
        return len(lotto)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--config", default=os.path.join(os.path.dirname(__file__), "worker.toml"))
    ap.add_argument("--una-volta", action="store_true", help="esegue al più un job ed esce")
    ap.add_argument("--cartelle", action="store_true", help="stampa l'albero delle cartelle Outlook ed esce")
    ap.add_argument("--caselle", action="store_true",
                    help="M1: chiede al server le caselle da servire, le risolve nel profilo Outlook e stampa "
                         "l'esito senza prendere nessun job")
    ap.add_argument("--debug", action="store_true")
    a = ap.parse_args()
    cfg = carica_config(a.config, {"consenti_invio": False})
    configura_log(a.debug, cfg, "worker_outlook")
    w = Worker(cfg)
    if a.cartelle:
        print("\n".join(w.ol().elenca_cartelle()))
        return
    if a.caselle:
        w.risolvi_caselle(forza=True)
        for c in w.caselle:
            cid = str(c["casella_id"])
            print(f"{c.get('nome') or c['indirizzo']:<20} {c['indirizzo']:<45} "
                  f"{'store ' + w.store[cid][:24] + '…' if cid in w.store else 'NON TROVATA nel profilo'}")
        print(f"dichiarazione al claim: {w.dichiarazione()}")
        return
    w.esegui_per_sempre(una_volta=a.una_volta)


if __name__ == "__main__":
    main()
