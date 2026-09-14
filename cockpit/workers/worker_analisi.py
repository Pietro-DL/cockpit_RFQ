"""worker-analisi: client HTTP di cockpit.exe che esegue i job di tipo 'analizza_allegato'.

Distingue i disegni CAD dalle offerte commerciali analizzando il contenuto testuale (PyMuPDF)
e la struttura dei file (STEP), senza mai basarsi su nomi di clienti per evitare falsi positivi.

    python worker_analisi.py [--config worker.toml] [--una-volta]
"""
from __future__ import annotations

import argparse
import logging
import os
import re
import time
from pathlib import Path
from uuid import UUID

import pymupdf

from cockpit_client import ERRORI_RETE, Cockpit, ErroreHTTP, carica_config, configura_log, nome_worker
from contratti import Job, PayloadAnalizzaAllegato, RisultatoAnalisi, RisultatoRichiesta

log = logging.getLogger("worker-analisi")

# Parole chiave cartiglio tecnico CAD 2D (da specifiche utente)
TERMINI_CARTIGLIO_CAD = [
    "ZONA ESENTE DA SALDATURA",
    "TOLLERANZE GENERALI",
    "PESO KG",
    "SCALA",
]

# Parole chiave offerte commerciali (da specifiche utente).
# I termini "forti" bastano da soli; "PAGAMENTO"/"INCOTERMS" compaiono anche in contratti e convenzioni
# (es. un PDF di tirocinio) e contano solo se ne ricorrono almeno due.
TERMINI_OFFERTE_FORTI = [
    "SPETT. LE OFFERTA",
    "SPETT.LE OFFERTA",
    "CONDIZIONI GENERALI DI VENDITA",
]
TERMINI_OFFERTE_DEBOLI = [
    "PAGAMENTO",
    "INCOTERMS",
    "OFFERTA N",
    "VALIDITA' OFFERTA",
    "VALIDITÀ OFFERTA",
    "RESA",
]
TERMINI_OFFERTE_COMMERCIALI = TERMINI_OFFERTE_FORTI + TERMINI_OFFERTE_DEBOLI

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


def analizza_pdf(percorso: str, nome_file: str) -> dict:
    """Estrae testo con PyMuPDF e applica le regole commerciali e cartiglio tecnico."""
    testo_completo = ""
    try:
        with pymupdf.open(percorso) as doc:
            for i, pag in enumerate(doc):
                testo_completo += pag.get_text() + "\n"
                if i >= 10:  # non serve scorrere oltre 10 pagine
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

    # Normalizza spaziature e newline per ricerca esatta case-insensitive
    testo_norm = re.sub(r"\s+", " ", testo_completo).upper()
    nome_upper = nome_file.upper().strip()
    nome_senza_ext = Path(nome_file).stem

    # 1. Regola Offerte Commerciali:
    # Se il PDF contiene "Spett. le Offerta", "CONDIZIONI GENERALI DI VENDITA", "Pagamento" o "Incoterms",
    # oppure il nome file inizia per "SO "
    trovati_comm = [t for t in TERMINI_OFFERTE_COMMERCIALI if t in testo_norm]
    forti = [t for t in trovati_comm if t in TERMINI_OFFERTE_FORTI]
    e_offerta = nome_upper.startswith("SO ") or forti or len(trovati_comm) >= 2
    if e_offerta:
        log.info("Riconosciuta offerta commerciale in %s (termini: %s)", nome_file, trovati_comm)
        return {
            "tipo_proposto": "offerta_promatec",
            "codice": "",
            "rev": "",
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"commerciale": True, "termini_trovati": trovati_comm},
        }

    # 2. Regola CAD 2D:
    # Se il PDF contiene "ZONA ESENTE DA SALDATURA", "TOLLERANZE GENERALI", "PESO kG" o "SCALA"
    trovati_cad = [t for t in TERMINI_CARTIGLIO_CAD if t in testo_norm]
    codice, rev = separa_codice_rev(nome_senza_ext)

    if trovati_cad:
        log.info("Riconosciuto cartiglio CAD 2D in %s: codice=%s rev=%s (termini: %s)", nome_file, codice, rev, trovati_cad)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 95,
            "fonte": "cartiglio",
            "dettagli": {"cartiglio": True, "termini_trovati": trovati_cad},
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
        self.api = Cockpit(cfg["server_url"], cfg["token"])
        self.worker_id = nome_worker("analisi", cfg)

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s collegato a %s", self.worker_id, self.api.url)
        attesa = 5
        while True:
            try:
                r = self.api.claim("analisi", self.worker_id)
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
            ris = RisultatoRichiesta(esito="errore", errore=f"file mancante in staging: usa Riscarica ({e})", definitivo=True)
        except Exception as e:
            log.exception("job %d errore: %s", job.job_id, e)
            ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])

        try:
            self.api.risultato(job.job_id, ris.model_dump(mode="json"))
        except Exception as e:
            log.error("impossibile riportare risultato job %d: %s", job.job_id, e)
        log.info("job %d completato in %.2fs -> %s", job.job_id, time.time() - t0, ris.esito)


def main():
    p = argparse.ArgumentParser(description="Worker Analisi per Cockpit RFQ")
    p.add_argument("--config", default=os.path.join(os.path.dirname(__file__), "worker.toml"), help="percorso file di configurazione")
    p.add_argument("--una-volta", action="store_true", help="esegui un solo job ed esci")
    p.add_argument("--debug", action="store_true")
    args = p.parse_args()

    cfg = carica_config(args.config)
    configura_log(args.debug, cfg, "worker_analisi")
    w = WorkerAnalisi(cfg)
    w.esegui_per_sempre(una_volta=args.una_volta)


if __name__ == "__main__":
    main()
