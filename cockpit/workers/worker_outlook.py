"""worker-outlook: client HTTP di cockpit.exe che esegue i job di tipo 'outlook' via COM.

    python worker_outlook.py [--config worker.toml] [--una-volta] [--cartelle]

Loop: POST /api/v1/jobs/claim (long-poll) → esegue → POST /api/v1/jobs/{id}/result.
Un solo thread: le chiamate COM sono serializzate per costruzione. Se Outlook non risponde il job
fallisce (ritentato dal server con backoff) e il worker resta vivo. Se il server è giù o si riavvia,
il worker aspetta e riprova: non termina mai da solo.
"""
from __future__ import annotations

import argparse
import logging
import os
import time
from datetime import datetime, timedelta, timezone

import pywintypes

from cockpit_client import (ERRORI_RETE, ArrestoRichiesto, Battito, Cockpit, ErroreHTTP, carica_config,
                            configura_log, nome_worker)
from contratti import (CartellaEsito, CursoreLotto, IngestRichiesta, Job, PayloadApriElemento, PayloadCreaBozza,
                       PayloadSegnaLetto, PayloadSpostaCartella, PayloadStageAllegato, PayloadSyncOutlook,
                       RisultatoBozza, RisultatoElemento, RisultatoRichiesta, RisultatoStage, RisultatoSync)
from outlook_com import ErroreDefinitivo, Outlook

log = logging.getLogger("worker")


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

    # ------------------------------------------------------------ loop

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s → %s", self.worker_id, self.api.url)
        attesa = 5
        while True:
            try:
                r = self.api.claim("outlook", self.worker_id)
                job = Job.model_validate(r) if r else None
                attesa = 5
            except (*ERRORI_RETE, ErroreHTTP) as e:
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
        match job.tipo:
            case "sync_outlook":
                return self.sync(job, PayloadSyncOutlook.model_validate(p))
            case "stage_allegato":
                s = PayloadStageAllegato.model_validate(p)
                dest, sha, n, dove = self.ol().salva_allegato(s.entry_id, s.store_id, s.indice, s.nome_file,
                                                              os.path.join(self.staging, s.cartella), s.message_id)
                return RisultatoStage(allegato_id=s.allegato_id, path_staging=dest, sha256=sha, bytes=n, **dove).model_dump(mode="json")
            case "crea_bozza_outlook":
                b = PayloadCreaBozza.model_validate(p)
                entry_id, inviata = self.ol().crea_bozza(b)
                return RisultatoBozza(entry_id=entry_id, inviata=inviata).model_dump(mode="json")
            case "apri_elemento_outlook":
                a = PayloadApriElemento.model_validate(p)
                return RisultatoElemento(**self.ol().apri(a.entry_id, a.store_id, a.message_id)).model_dump(mode="json")
            case "sposta_in_cartella":
                s = PayloadSpostaCartella.model_validate(p)
                return RisultatoElemento(**self.ol().sposta(s.entry_id, s.store_id, s.cartella, s.message_id)).model_dump(mode="json")
            case "segna_letto":
                l = PayloadSegnaLetto.model_validate(p)
                return RisultatoElemento(**self.ol().segna_letto(l.entry_id, l.store_id, l.letto, l.message_id)).model_dump(mode="json")
        raise ErroreDefinitivo(f"tipo job sconosciuto per il worker outlook: {job.tipo}")

    # ------------------------------------------------------------ sync

    def sync(self, job: Job, p: PayloadSyncOutlook) -> dict:
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
                for m in self.ol().leggi(c.cartella, dal, al=al):
                    # punto di ripresa: fra un elemento e l'altro il lavoro è fuori da COM, quindi qui
                    # un arresto chiesto dal battito si può rispettare senza lasciare niente a metà
                    self.controlla()
                    lotto.append(m)
                    # W2, ANCORA APERTO (avvertenza della revisione del 15/09). Il cursore avanza su
                    # data_evento, che per la Posta inviata è SentOn, mentre il filtro della scansione
                    # usa ReceivedTime. Per la Posta in arrivo i due coincidono e non si vede niente;
                    # per la Posta inviata no, e un messaggio inviato molto dopo essere stato scritto
                    # può spingere il cursore oltre elementi non ancora letti. Si chiude in fase 2, con
                    # MessaggioIn.ricevuto_il e messaggio_casella.ricevuto_il (voce 2.1): finché il
                    # contratto non porta ricevuto_il, qui non c’è il dato giusto da usare.
                    if al is None and m.data_evento and (esito.ultimo_received is None or m.data_evento > esito.ultimo_received):
                        esito.ultimo_received = m.data_evento
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
    ap.add_argument("--debug", action="store_true")
    a = ap.parse_args()
    cfg = carica_config(a.config, {"consenti_invio": False})
    configura_log(a.debug, cfg, "worker_outlook")
    w = Worker(cfg)
    if a.cartelle:
        print("\n".join(w.ol().elenca_cartelle()))
        return
    w.esegui_per_sempre(una_volta=a.una_volta)


if __name__ == "__main__":
    main()
