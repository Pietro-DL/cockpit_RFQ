"""worker-analisi: client HTTP di cockpit.exe che esegue i job di tipo 'analizza_allegato'.

Distingue i disegni CAD dalle offerte commerciali analizzando il contenuto testuale (PyMuPDF)
e la struttura dei file (STEP), senza mai basarsi su nomi di clienti per evitare falsi positivi.

    python worker_analisi.py [--config worker.toml] [--una-volta]
"""
from __future__ import annotations

import argparse
import json
import logging
import os
import re
import socket
import sys
import time
import tomllib
import urllib.error
import urllib.request
from pathlib import Path
from uuid import UUID

import pymupdf

from contratti import Job, PayloadAnalizzaAllegato, RisultatoAnalisi, RisultatoRichiesta

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(name)s: %(message)s")
log = logging.getLogger("worker-analisi")

# Parole chiave cartiglio tecnico (CAD 2D/3D)
TERMINI_CARTIGLIO = [
    "TOLLERANZE",
    "TOLLERANZA",
    "MATERIALE",
    "SCALA",
    "PESO",
    "TRATTAMENTO",
    "ZONA ESENTE DA SALDATURA",
    "DISEGNO",
    "DRAWING",
    "TITLE BLOCK",
    "PROPRIETA' RISERVATA",
    "CONFIDENTIAL PROPERTY",
]

# Parole chiave offerte commerciali
TERMINI_COMMERCIALI = [
    "SPETT. LE OFFERTA",
    "SPETT.LE OFFERTA",
    "CONDIZIONI GENERALI DI VENDITA",
    "PAGAMENTO",
    "INCOTERMS",
    "CONDIZIONI GENERALI",
    "PREZZO UNITARIO",
    "PRZ. UNIT.",
]

STOP_CODICI = {
    "SCREENSHOT", "WHATSAPP", "IMAGE", "PHOTO", "IMG", "DOCUMENT", "OFFERTA",
    "DISEGNO", "ALLEGATO", "REV", "STEP", "STP", "PDF", "DXF", "DWG", "ZIP",
}

RE_REV = re.compile(r"^(.+?)[_\-](?:REV|R)?([0-9]{1,2}|[A-Z])$", re.IGNORECASE)
RE_STEP_PRODUCT = re.compile(r"PRODUCT\s*\(\s*'([^']+)'", re.IGNORECASE)


def sembra_codice(s: str) -> bool:
    """Verifica se una stringa rispetta i pattern codice prodotto (almeno 5 char, almeno 3 cifre)."""
    s = s.strip().upper()
    if len(s) < 5 or len(s) > 40:
        return False
    if s in STOP_CODICI or any(s.startswith(p) for p in ("SCREENSHOT", "IMG", "WHATSAPP", "SCAN", "PHOTO", "SO ", "SO-", "SO_")):
        return False
    if " " in s:
        return False
    cifre = sum(1 for c in s if c.isdigit())
    lettere = sum(1 for c in s if c.isalpha())
    if cifre < 3:
        return False
    # Evita date tipo 2026-09-08
    if lettere == 0 and (s.count("-") >= 2 or s.count("/") >= 2 or s.count(".") >= 2):
        return False
    return True


def separa_codice_rev(nome_base: str) -> tuple[str, str]:
    """Separa codice e revisione dal nome file senza estensione."""
    m = RE_REV.match(nome_base)
    if m:
        base, rev = m.group(1), m.group(2)
        if sembra_codice(base):
            return base, rev
    if sembra_codice(nome_base):
        return nome_base, ""
    return "", ""


class CockpitClient:
    """Client minimale HTTP per cockpit.exe."""

    def __init__(self, url: str, token: str):
        self.url = url.rstrip("/")
        self.token = token

    def _chiama(self, metodo: str, percorso: str, corpo: dict | None = None, timeout: int = 60):
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
            raise RuntimeError(f"{metodo} {percorso} -> {e.code}: {testo[:500]}") from None

    def claim(self, worker_id: str, attesa_s: int = 20) -> Job | None:
        r = self._chiama(
            "POST",
            "/api/v1/jobs/claim",
            {"worker": "analisi", "worker_id": worker_id, "attesa_s": attesa_s},
            timeout=attesa_s + 15,
        )
        return Job.model_validate(r) if r else None

    def heartbeat(self, job_id: int, worker_id: str) -> None:
        self._chiama("POST", f"/api/v1/jobs/{job_id}/heartbeat", {"worker_id": worker_id})

    def risultato(self, job_id: int, r: RisultatoRichiesta) -> None:
        self._chiama("POST", f"/api/v1/jobs/{job_id}/result", r.model_dump(mode="json"))


def analizza_pdf(percorso: str, nome_file: str) -> dict:
    """Estrae testo con PyMuPDF e applica le regole commerciali e cartiglio tecnico."""
    testo_completo = ""
    try:
        with pymupdf.open(percorso) as doc:
            for i, pag in enumerate(doc):
                testo_completo += pag.get_text() + "\n"
                if i >= 10:  # non serve scorrere documenti enormi oltre 10 pagine
                    break
    except Exception as e:
        log.warning("impossibile leggere PDF %s con pymupdf: %s", percorso, e)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": "",
            "rev": "",
            "confidenza": 40,
            "fonte": "estensione",
            "dettagli": {"errore_pdf": str(e)},
        }

    testo_upper = testo_completo.upper()
    nome_upper = nome_file.upper().strip()
    nome_senza_ext = Path(nome_file).stem

    # 1. Regola Offerte Commerciali
    # Se il nome file inizia per "SO " o il testo contiene pattern commerciali
    trovati_comm = [t for t in TERMINI_COMMERCIALI if t in testo_upper]
    if nome_upper.startswith("SO ") or len(trovati_comm) >= 2 or "SPETT. LE OFFERTA" in testo_upper or "SPETT.LE OFFERTA" in testo_upper or "CONDIZIONI GENERALI DI VENDITA" in testo_upper:
        log.info("Riconosciuta offerta commerciale in %s (termini: %s)", nome_file, trovati_comm)
        return {
            "tipo_proposto": "offerta_promatec",
            "codice": "",
            "rev": "",
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"commerciale": True, "termini_trovati": trovati_comm},
        }

    # 2. Regola CAD 2D/3D (Cartiglio tecnico)
    # Campi tipici cartiglio + rispetto pattern codice nel nome file
    trovati_cart = [t for t in TERMINI_CARTIGLIO if t in testo_upper]
    codice, rev = separa_codice_rev(nome_senza_ext)

    if trovati_cart and codice:
        log.info("Riconosciuto cartiglio CAD in %s: codice=%s rev=%s (termini: %s)", nome_file, codice, rev, trovati_cart)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"cartiglio": True, "termini_trovati": trovati_cart},
        }

    if trovati_cart:
        log.info("Trovati termini cartiglio ma nome file non e un codice valido (%s)", nome_file)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": "",
            "rev": "",
            "confidenza": 75,
            "fonte": "cartiglio",
            "dettagli": {"cartiglio": True, "termini_trovati": trovati_cart},
        }

    if codice:
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 70,
            "fonte": "nome_file",
            "dettagli": {"codice_riconosciuto": codice},
        }

    return {
        "tipo_proposto": "altro",
        "codice": "",
        "rev": "",
        "confidenza": 30,
        "fonte": "estensione",
        "dettagli": {},
    }


def analizza_step(percorso: str, nome_file: str) -> dict:
    """Estrae definizioni PRODUCT da file STEP ISO 10303-21."""
    nome_senza_ext = Path(nome_file).stem
    codice, rev = separa_codice_rev(nome_senza_ext)
    try:
        with open(percorso, "r", encoding="utf-8", errors="ignore") as f:
            # Leggi primi 100KB per trovare PRODUCT
            contenuto = f.read(100 * 1024)
        m = RE_STEP_PRODUCT.search(contenuto)
        if m:
            codice_step = m.group(1).strip()
            if sembra_codice(codice_step):
                c_step, r_step = separa_codice_rev(codice_step)
                return {
                    "tipo_proposto": "cad_3d",
                    "codice": c_step or codice_step,
                    "rev": r_step or rev,
                    "confidenza": 95,
                    "fonte": "step",
                    "dettagli": {"product_step": codice_step},
                }
    except Exception as e:
        log.warning("lettura STEP fallita per %s: %s", percorso, e)

    return {
        "tipo_proposto": "cad_3d",
        "codice": codice,
        "rev": rev,
        "confidenza": 80 if codice else 60,
        "fonte": "nome_file" if codice else "estensione",
        "dettagli": {},
    }


def analizza_file(path_staging: str, nome_file: str) -> dict:
    """Smista l'analisi in base all'estensione del file in staging."""
    ext = Path(nome_file).suffix.lower().lstrip(".")
    if not os.path.isfile(path_staging):
        raise FileNotFoundError(f"file non trovato in staging: {path_staging}")

    if ext == "pdf":
        return analizza_pdf(path_staging, nome_file)
    if ext in ("stp", "step"):
        return analizza_step(path_staging, nome_file)
    if ext == "dxf":
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "sviluppo_dxf",
            "codice": codice,
            "rev": rev,
            "confidenza": 85 if codice else 70,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {},
        }
    if ext in ("dwg", "tif", "tiff"):
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 80 if codice else 60,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {},
        }
    if ext in ("xls", "xlsx", "csv"):
        return {
            "tipo_proposto": "commerciale",
            "codice": "",
            "rev": "",
            "confidenza": 60,
            "fonte": "estensione",
            "dettagli": {},
        }

    codice, rev = separa_codice_rev(Path(nome_file).stem)
    return {
        "tipo_proposto": "altro",
        "codice": codice,
        "rev": rev,
        "confidenza": 30,
        "fonte": "estensione",
        "dettagli": {},
    }


class WorkerAnalisi:
    def __init__(self, cfg: dict):
        self.cfg = cfg
        self.api = CockpitClient(cfg["server_url"], cfg["token"])
        self.worker_id = cfg.get("worker_id") or f"analisi@{socket.gethostname()}#{os.getpid()}"

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s collegato a %s", self.worker_id, self.api.url)
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
            if job.tipo != "analizza_allegato":
                raise ValueError(f"tipo job imprevisto per worker analisi: {job.tipo}")

            p = PayloadAnalizzaAllegato.model_validate(job.payload)
            esito = analizza_file(p.path_staging, p.nome_file)
            ris_analisi = RisultatoAnalisi(
                allegato_id=p.allegato_id,
                tipo_proposto=esito["tipo_proposto"],
                codice=esito.get("codice", ""),
                rev=esito.get("rev", ""),
                confidenza=esito.get("confidenza", 50),
                fonte=esito.get("fonte", "cartiglio"),
                dettagli=esito.get("dettagli", {}),
            )
            ris = RisultatoRichiesta(esito="ok", dati=ris_analisi.model_dump(mode="json"))
        except FileNotFoundError as e:
            log.error("job %d file non trovato: %s", job.job_id, e)
            ris = RisultatoRichiesta(esito="errore", errore=str(e), definitivo=True)
        except Exception as e:
            log.exception("job %d errore: %s", job.job_id, e)
            ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])

        try:
            self.api.risultato(job.job_id, ris)
        except Exception as e:
            log.error("impossibile riportare risultato job %d: %s", job.job_id, e)
        log.info("job %d completato in %.2fs -> %s", job.job_id, time.time() - t0, ris.esito)


def carica_config(percorso: str) -> dict:
    if os.path.exists(percorso):
        with open(percorso, "rb") as f:
            return tomllib.load(f)
    return {
        "server_url": "http://127.0.0.1:8080",
        "token": "dev-token-cambiami-in-produzione",
    }


def main():
    p = argparse.ArgumentParser(description="Worker Analisi per Cockpit RFQ")
    p.add_argument("--config", default="worker.toml", help="percorso file di configurazione")
    p.add_argument("--una-volta", action="store_true", help="esegui un solo job ed esci")
    args = p.parse_args()

    cfg = carica_config(args.config)
    w = WorkerAnalisi(cfg)
    w.esegui_per_sempre(una_volta=args.una_volta)


if __name__ == "__main__":
    main()
