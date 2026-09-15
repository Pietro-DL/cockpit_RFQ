"""L2 — C16: il recupero di un worker bloccato dentro una chiamata COM.

Il problema che questi test difendono. Una chiamata COM non è interrompibile dall'esterno: se Outlook
si ferma su una finestra modale o su un elemento corrotto, il thread che l'ha chiamata non torna, e
nessun flag può farlo tornare. Intanto il lease scade, il server rimette il job in coda e un altro
tentativo lo esegue: il primo processo diventa un fantasma che tiene occupato Outlook e non prende più
lavoro. L'unica uscita è terminare il processo e lasciarlo riavviare dall'attività pianificata.

Sono due meccanismi che lavorano insieme, e i test li separano apposta:
  - il lavoro che PUÒ fermarsi lo fa da solo, ai punti di ripresa (controlla());
  - il lavoro che NON può fermarsi viene interrotto terminando il processo (uscita forzata).
"""
import threading
import time

import pytest

from cockpit_client import ArrestoRichiesto, Battito, ErroreHTTP


class ApiFinta:
    """Cockpit finto: il heartbeat risponde 409 dopo `ok_per` chiamate."""

    def __init__(self, ok_per: int = 0):
        self.ok_per = ok_per
        self.chiamate = 0

    def heartbeat(self, job_id, worker_id, lease_token):
        self.chiamate += 1
        if self.chiamate > self.ok_per:
            raise ErroreHTTP("POST", f"/api/v1/jobs/{job_id}/heartbeat", 409,
                             '{"errore":"tentativo non più valido"}')


def test_il_lavoro_che_puo_fermarsi_si_ferma_da_solo():
    """Caso normale: il 409 arriva, il lavoro è fra due elementi e smette. Nessuna uscita forzata."""
    uscite = []
    api = ApiFinta(ok_per=0)
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05, arresto_forzato_s=5,
                uscita=uscite.append)
    with b:
        # il lavoro controlla a ogni giro, come fa il sync fra un elemento e l'altro
        with pytest.raises(ArrestoRichiesto):
            for _ in range(200):
                b.controlla()
                time.sleep(0.01)
    assert b.arresto.is_set(), "il battito non ha chiesto l'arresto dopo il 409"
    assert uscite == [], f"il processo è stato terminato benché il lavoro si fosse fermato: {uscite}"
    assert not b.uscita_forzata


def test_il_lavoro_bloccato_in_com_fa_terminare_il_processo():
    """Caso C16: il 409 arriva, ma il lavoro è dentro COM e non controlla nulla.

    Dopo `arresto_forzato_s` il battito termina il processo. Nel test `uscita` è sostituita, altrimenti
    os._exit ucciderebbe pytest: ciò che si verifica è che venga chiamata, con quale codice, e solo
    dopo aver atteso.
    """
    uscite = []
    api = ApiFinta(ok_per=0)
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05, arresto_forzato_s=0.5,
                uscita=uscite.append)
    b.segna_fase("stage_allegato")
    inizio = time.monotonic()
    with b:
        # il «lavoro» è dentro una chiamata COM che non ritorna: non chiama mai controlla()
        time.sleep(1.5)
    durata = time.monotonic() - inizio

    assert uscite == [3], f"uscita forzata attesa con codice 3, ottenuto {uscite}"
    assert b.uscita_forzata
    assert durata >= 0.5, "il processo è stato terminato senza concedere il tempo di fermarsi da solo"
    assert b.fase == "stage_allegato", "la fase non è stata registrata: il log non direbbe dove era bloccato"


def test_un_buco_di_rete_non_e_la_perdita_del_lease():
    """Il server irraggiungibile non significa lease perso: il worker non deve arrestarsi né uscire."""
    class ApiGiu:
        def __init__(self):
            self.chiamate = 0

        def heartbeat(self, job_id, worker_id, lease_token):
            self.chiamate += 1
            raise TimeoutError("server irraggiungibile")

    uscite = []
    api = ApiGiu()
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05, arresto_forzato_s=0.3, uscita=uscite.append)
    with b:
        time.sleep(0.5)
    assert api.chiamate >= 3, "il battito ha smesso di provare"
    assert not b.arresto.is_set(), "un errore di rete è stato scambiato per una perdita del lease"
    assert uscite == [], "il processo è stato terminato per un buco di rete"


def test_il_battito_non_esce_se_il_lavoro_finisce_durante_lattesa():
    """Corsa fra l'uscita forzata e la fine del lavoro: se il lavoro finisce entro il tempo, si resta."""
    uscite = []
    api = ApiFinta(ok_per=0)
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05, arresto_forzato_s=2.0, uscita=uscite.append)
    with b:
        # il lavoro non controlla il flag, ma termina prima che scada l'attesa
        time.sleep(0.4)
    time.sleep(0.3)  # tempo perché un'eventuale uscita sbagliata si manifesti
    assert uscite == [], f"il processo è stato terminato benché il lavoro fosse finito in tempo: {uscite}"


def test_il_battito_tiene_vivo_il_lease_finche_il_server_risponde_ok():
    """Controprova: finché il server risponde, il battito continua e non chiede niente a nessuno."""
    uscite = []
    api = ApiFinta(ok_per=1000)
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05, arresto_forzato_s=0.3, uscita=uscite.append)
    with b:
        for _ in range(30):
            b.controlla()  # non deve mai alzare
            time.sleep(0.01)
    assert api.chiamate >= 3, "il battito non ha battuto"
    assert not b.arresto.is_set()
    assert uscite == []


def test_chiudi_ferma_il_thread():
    """Nessun thread di battito deve sopravvivere alla fine del job."""
    api = ApiFinta(ok_per=1000)
    b = Battito(api, 7, "outlook@PC-A", "tok", ogni_s=0.05)
    vivi_prima = threading.active_count()
    with b:
        time.sleep(0.15)
    assert threading.active_count() <= vivi_prima, "il thread del battito è rimasto vivo dopo il job"
