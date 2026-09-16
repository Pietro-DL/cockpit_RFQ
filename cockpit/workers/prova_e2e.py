"""Banco di prova end-to-end: il worker VERO contro il server VERO, senza Outlook.

    python prova_e2e.py            # un solo job, poi esce (come `worker_outlook.py --una-volta`)

Non è un test: è l'unica riga di codice che il test L4 `internal/workerapi/e2e_worker_db_test.go`
aggiunge al worker per poterlo far girare su un PC senza Outlook. Sostituisce SOLO l'adattatore COM
— la classe `Outlook` di worker_outlook — e poi chiama `worker_outlook.main()`: configurazione,
risoluzione delle caselle, claim, battito, upload e result restano quelli veri, byte per byte.

Perché esiste. Il 15/09/2026 il worker vero ha girato a vuoto un pomeriggio: mandava
`worker_id: ""` in ogni result, il server rispondeva 400 e nessun job chiudeva. Quattro livelli di
prove erano verdi, perché il server finto dell'L2 accettava qualunque corpo e il confronto dei
contratti (L3) verifica che gli schemi coincidano, non che il client li compili. Mancava una prova
in cui il client vero parla con il server vero: è questa.

Configurazione, tutta da variabili d'ambiente (le legge `carica_config`):

    COCKPIT_URL, COCKPIT_TOKEN, COCKPIT_WORKER_ID, COCKPIT_STAGING

più quelle di questo banco:

    COCKPIT_E2E_LAVORO_S     secondi di «lavoro» simulato dentro ogni chiamata COM (default 0).
                             Serve a far scattare almeno un battito mentre il job è in corso.
    COCKPIT_E2E_BLOCCHI      dimensione del finto allegato, in blocchi da 256 byte (default 8).
    COCKPIT_E2E_NON_RISOLTE  indirizzi, separati da virgola, che il «profilo» NON contiene: servono
                             a provare una casella censita ma assente da questo PC.
"""
from __future__ import annotations

import hashlib
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import worker_outlook  # noqa: E402  (l'import deve seguire il sys.path)

LAVORO_S = float(os.environ.get("COCKPIT_E2E_LAVORO_S", "0"))
BLOCCHI = int(os.environ.get("COCKPIT_E2E_BLOCCHI", "8"))
NON_RISOLTE = {x.strip().lower() for x in os.environ.get("COCKPIT_E2E_NON_RISOLTE", "").split(",") if x.strip()}

# Contenuto del finto allegato: deterministico, così il test Go sa che sha256 aspettarsi senza
# doverselo far dire dal worker.
CONTENUTO = bytes(range(256)) * BLOCCHI


class OutlookFintoE2E:
    """L'adattatore COM, meno COM. Nessun Outlook, nessuna posta, nessun profilo.

    Ogni metodo attende `COCKPIT_E2E_LAVORO_S` secondi prima di rispondere: è lì che il worker vero
    passa il suo tempo, ed è mentre lo passa che il battito deve rinnovare il lease.
    """

    def __init__(self, consenti_invio: bool = False, **opzioni):
        self.consenti_invio = consenti_invio
        # Le opzioni della finestra temporale (voce 2.9: usa_restrict, autoprova_giorni) arrivano
        # perché il worker vero le passa sempre. Qui non servono — non c'è nessuna cartella da
        # filtrare — ma vanno ACCETTATE: questo test esiste per provare il worker vero, e il worker
        # vero costruisce l'adattatore con i parametri che ha. Un `**opzioni` che le ignora è la
        # differenza fra un doppione che segue il codice e uno che va aggiornato a mano ogni volta.
        self.opzioni = opzioni

    @staticmethod
    def _lavora() -> None:
        if LAVORO_S > 0:
            time.sleep(LAVORO_S)

    # ---------------------------------------------------------- voce 2.6: caselle → store locale

    def risolvi_caselle(self, caselle):
        trovate, mancanti = {}, []
        for c in caselle:
            if c["indirizzo"].lower() in NON_RISOLTE:
                mancanti.append(c.get("nome") or c["indirizzo"])
            else:
                trovate[str(c["casella_id"])] = "STORE-LOCALE-" + c["indirizzo"]
        return trovate, mancanti

    # ---------------------------------------------------------- job

    def salva_allegato(self, entry_id, store_id, indice, nome_file, cartella_staging, message_id=""):
        self._lavora()
        os.makedirs(cartella_staging, exist_ok=True)
        dest = os.path.join(cartella_staging, f"{indice:02d}_{nome_file}")
        with open(dest, "wb") as f:
            f.write(CONTENUTO)
        return (dest, hashlib.sha256(CONTENUTO).hexdigest(), len(CONTENUTO),
                {"entry_id": entry_id, "cartella": "Posta in arrivo"})

    def apri(self, entry_id, store_id, message_id=""):
        self._lavora()
        return {"entry_id": entry_id, "cartella": "Posta in arrivo"}

    def segna_letto(self, entry_id, store_id, letto, message_id=""):
        self._lavora()
        return {"entry_id": entry_id, "cartella": "Posta in arrivo"}

    def sposta(self, entry_id, store_id, cartella, message_id=""):
        self._lavora()
        return {"entry_id": entry_id, "cartella": cartella}

    def crea_bozza(self, p, store_id=""):
        self._lavora()
        return "ENTRY-BOZZA-E2E", False

    def leggi(self, cartella, dal, al=None, store_id=""):
        """Nessun messaggio: la lettura della posta è materia di L5, non di questo banco."""
        self._lavora()
        return iter(())


def main() -> None:
    worker_outlook.Outlook = OutlookFintoE2E
    # Un percorso che non esiste: la configurazione deve venire tutta dall'ambiente, mai dal
    # worker.toml vero di questo PC, che punta al server e alle caselle reali.
    config = os.environ.get("COCKPIT_CONFIG") or os.path.join(
        os.environ.get("COCKPIT_STAGING", "."), "nessun-worker.toml")
    sys.argv = [sys.argv[0], "--una-volta", "--config", config, "--debug"]
    worker_outlook.main()


if __name__ == "__main__":
    main()
