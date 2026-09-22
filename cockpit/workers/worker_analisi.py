"""worker-analisi: client HTTP di cockpit.exe che esegue i job di tipo 'analizza_allegato'.

Distingue i disegni CAD dalle offerte commerciali analizzando il contenuto testuale (PyMuPDF)
e la struttura dei file (STEP), senza mai basarsi su nomi di clienti per evitare falsi positivi.

    python worker_analisi.py [--config worker.toml] [--una-volta]
"""
from __future__ import annotations

import argparse
import gc
import logging
import os
import re
import shutil
import socket
import time
from pathlib import Path
from uuid import UUID

import pymupdf

from cockpit_client import (ERRORI_RETE, ArrestoRichiesto, Battito, Cockpit, ContenutoIncompleto, ErroreHTTP,
                            ImprontaSbagliata, cadenza_battito, carica_config, configura_log, diagnosi,
                            leggi_marcatore_arresto, nome_worker, riporta_risultato)
from contratti import Job, PayloadAnalizzaAllegato, RisultatoAnalisi, RisultatoRichiesta
from step_struttura import leggi_struttura

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

# Capitolato / specifica tecnica: parla di requisiti e di come si collauda, non di un pezzo solo.
# Ne servono almeno due, perche' «SPECIFICA TECNICA» da sola compare anche nel cartiglio di un disegno.
TERMINI_CAPITOLATO = [
    "CAPITOLATO",
    "SPECIFICA TECNICA",
    "REQUISITI GENERALI",
    "PRESCRIZIONI",
    "NORME DI RIFERIMENTO",
    "PIANO DI CONTROLLO",
    "CONDIZIONI DI FORNITURA",
    "TECHNICAL SPECIFICATION",
    "GENERAL REQUIREMENTS",
    "QUALITY REQUIREMENTS",
]

# Distinta del cliente: l'intestazione di una tabella di codici con quantita'.
TERMINI_DISTINTA = [
    "DISTINTA BASE",
    "DISTINTA MATERIALI",
    "BILL OF MATERIAL",
    "PARTS LIST",
    "ELENCO PARTI",
    "STUCKLISTE",
    "POS.",
    "Q.TA",
    "QUANTITA",
]

# Una riga di distinta: qualcosa che sembra un codice, e poi un numero che sembra una quantita'.
RE_RIGA_DISTINTA = re.compile(r"\b[A-Z0-9][A-Z0-9._\-/]{4,}\b[^\n]{0,40}?\b\d{1,4}(?:[.,]\d{1,3})?\b")

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
    """Che cosa c'e' dentro questo PDF.

    Checkpoint 3R §5. Un PDF non e' un disegno: e' un contenitore. In una richiesta d'offerta vera
    contiene, con frequenze paragonabili, il disegno quotato, il capitolato, la distinta del cliente,
    l'offerta di un fornitore, la conferma d'ordine o la firma di qualcuno. Fino a qui il tipo lo
    decideva l'estensione, cioe' lo decideva prima di aprire il file; da qui lo decide il CONTENUTO, e
    quando il contenuto non basta la risposta e' `da_determinare` — che e' una risposta, non un errore.
    """
    testo_completo = ""
    try:
        # Il PDF si apre DA MEMORIA, non per percorso (7C.1, P0): su Windows PyMuPDF tiene aperto un
        # file che non riesce ad aprire, e il temporaneo del worker non si cancella piu' («utilizzato
        # da un altro processo»). Letto in memoria, il file e' chiuso prima che PyMuPDF lo veda.
        with open(percorso, "rb") as f:
            dati = f.read()
        with pymupdf.open(stream=dati, filetype="pdf") as doc:
            for i, pag in enumerate(doc):
                testo_completo += pag.get_text() + "\n"
                if i >= 10:  # non serve scorrere oltre 10 pagine
                    break
    except Exception as e:
        # Un PDF che non si apre non e' un disegno con confidenza 40: e' un PDF che non si apre.
        log.warning("impossibile leggere PDF %s con pymupdf: %s", percorso, e)
        return {
            "tipo_proposto": "da_determinare",
            "codice": "",
            "rev": "",
            "confidenza": 20,
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

    # 3. Capitolato / specifica tecnica: parla di requisiti, non di un pezzo.
    trovati_cap = [t for t in TERMINI_CAPITOLATO if t in testo_norm]
    if len(trovati_cap) >= 2:
        log.info("Riconosciuto capitolato in %s (termini: %s)", nome_file, trovati_cap)
        return {
            "tipo_proposto": "capitolato",
            "codice": "",
            "rev": "",
            "confidenza": 80,
            "fonte": "cartiglio",
            "dettagli": {"capitolato": True, "termini_trovati": trovati_cap},
        }

    # 4. Distinta del cliente: un elenco di codici con quantita'.
    trovati_dist = [t for t in TERMINI_DISTINTA if t in testo_norm]
    if len(trovati_dist) >= 2 and conta_righe_codice(testo_completo) >= 5:
        log.info("Riconosciuta distinta cliente in %s (termini: %s)", nome_file, trovati_dist)
        return {
            "tipo_proposto": "distinta_cliente",
            "codice": "",
            "rev": "",
            "confidenza": 75,
            "fonte": "cartiglio",
            "dettagli": {"distinta": True, "termini_trovati": trovati_dist},
        }

    # 5. Nessun contenuto riconosciuto. Il codice nel nome NON basta a dire che e' un disegno: e'
    #    esattamente cosi' che «6674611A.pdf» diventava un CAD al 70% mentre era l'offerta di un
    #    fornitore per quel pezzo. Il codice si conserva, il tipo resta da determinare.
    return {
        "tipo_proposto": "da_determinare",
        "codice": codice,
        "rev": rev,
        "confidenza": 40 if codice else 30,
        "fonte": "nome_file" if codice else "estensione",
        "dettagli": {"codice_riconosciuto": codice, "testo_letto": len(testo_norm)},
    }


def conta_righe_codice(testo: str) -> int:
    """Quante righe sembrano una voce di distinta: un codice e una quantita'."""
    n = 0
    for riga in testo.splitlines():
        if RE_RIGA_DISTINTA.search(riga):
            n += 1
    return n


def analizza_step(percorso: str, nome_file: str) -> dict:
    """Che cosa c'e' dentro questo file STEP.

    Due letture, e vanno tenute distinte (addendum B8, A1.2).

    La prima e' quella di sempre: il primo `PRODUCT` dei primi 100 KB, passato da `sembra_codice`, che
    diventa `codice`/`rev` del risultato. E' un'IPOTESI dal nome, buona per `documento_proposta`, e
    resta com'e' per non cambiare sotto i piedi a chi la legge gia'.

    La seconda e' la STRUTTURA: nodi, relazioni e quantita', con gli attributi grezzi delle entita' e
    nessuna interpretazione. Nei nodi non c'e' nessun codice, e non e' una dimenticanza: che cosa sia
    un codice lo dicono le regole del CLIENTE della richiesta, e il worker non sa nemmeno di che
    cliente si tratti. La struttura non fa mai fallire il job: se il file non si legge resta l'avviso,
    e il tipo lo dice comunque l'estensione.
    """
    nome_senza_ext = Path(nome_file).stem
    codice, rev = separa_codice_rev(nome_senza_ext)
    esito = {
        "tipo_proposto": "cad_3d",
        "codice": codice,
        "rev": rev,
        "confidenza": 80 if codice else 60,
        "fonte": "nome_file" if codice else "estensione",
        "dettagli": {"struttura": leggi_struttura(percorso)},
    }
    try:
        with open(percorso, "r", encoding="utf-8", errors="ignore") as f:
            # Leggi primi 100KB per trovare PRODUCT
            contenuto = f.read(100 * 1024)
        m = RE_STEP_PRODUCT.search(contenuto)
        if m:
            codice_step = m.group(1).strip()
            esito["dettagli"]["product_step"] = codice_step
            if sembra_codice(codice_step):
                c_step, r_step = separa_codice_rev(codice_step)
                esito.update({
                    "codice": c_step or codice_step,
                    "rev": r_step or rev,
                    "confidenza": 95,
                    "fonte": "step",
                })
    except Exception as e:
        log.warning("lettura STEP fallita per %s: %s", percorso, e)
    return esito


def rimuovi_con_pazienza(percorso: str, tentativi: int = 10, attesa_s: float = 0.2) -> bool:
    """Cancella un file temporaneo su Windows, dove un handle ancora aperto fa fallire la rimozione.

    E' successo alla prima prova del download (7C.1, P0): PyMuPDF che NON riesce ad aprire un file
    lo tiene comunque aperto finche' l'oggetto non viene raccolto, e `os.remove` risponde
    «Il file e' utilizzato da un altro processo». Il file restava nella tmp del worker fino al
    riavvio. Qui si forza la raccolta e si riprova per qualche decimo di secondo; se non basta, lo
    si dice nel log e ci pensa svuota_tmp all'avvio successivo.
    """
    for i in range(tentativi):
        try:
            if os.path.exists(percorso):
                os.remove(percorso)
            return True
        except OSError as e:
            if i == tentativi - 1:
                log.warning("file temporaneo non rimosso: %s (%s)", percorso, e)
                return False
            gc.collect()
            time.sleep(attesa_s)
    return False


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
    if ext == "dwg":
        # Un DWG e' un file CAD: l'estensione lo dice davvero, non lo suppone.
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "disegno_2d",
            "codice": codice,
            "rev": rev,
            "confidenza": 80 if codice else 60,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {},
        }
    if ext in ("tif", "tiff"):
        # Un'immagine scansionata puo' essere un disegno, ma anche un capitolato fotocopiato o un
        # documento di trasporto. Senza OCR non lo sappiamo, e non lo diciamo (checkpoint 3R §5).
        codice, rev = separa_codice_rev(Path(nome_file).stem)
        return {
            "tipo_proposto": "da_determinare",
            "codice": codice,
            "rev": rev,
            "confidenza": 40 if codice else 30,
            "fonte": "nome_file" if codice else "estensione",
            "dettagli": {"nota": "immagine non letta: serve OCR"},
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
        self.api = Cockpit(cfg["server_url"], cfg["token"], impronta=cfg.get("impronta", ""))
        self.worker_id = nome_worker("analisi", cfg)
        # Cartella DEL WORKER (7C.1, P0): qui scende il contenuto da analizzare, il tempo di leggerlo.
        # Non e' lo staging del server, che puo' stare su un altro PC.
        self.staging = os.path.abspath(cfg.get("staging") or ".")
        self.svuota_tmp()
        self.battito: Battito | None = None
        # quanto si concede al lavoro per fermarsi da solo dopo un 409, prima dell'uscita forzata (C16)
        self.arresto_forzato_s = float(cfg.get("arresto_forzato_s", 15))
        self.marcatore_arresto = os.path.join(self.staging, "ultimo_arresto.txt")
        self.ultimo_arresto = leggi_marcatore_arresto(self.marcatore_arresto)

    def svuota_tmp(self) -> int:
        """`tmp\\` e' di passaggio: il contenuto scaricato dal server sta li' il tempo dell'analisi e si
        cancella subito dopo. Cio' che resta e' di un processo morto a meta', e nessuno lo riprendera':
        il job e' tornato in coda e il tentativo dopo lo riscarica. Si svuota all'avvio."""
        tmp = os.path.join(self.staging, "tmp")
        if not os.path.isdir(tmp):
            return 0
        n = 0
        for nome in os.listdir(tmp):
            p = os.path.join(tmp, nome)
            try:
                if os.path.isdir(p):
                    shutil.rmtree(p)
                else:
                    os.remove(p)
                n += 1
            except OSError as e:
                log.warning("tmp non svuotata: %s (%s)", p, e)
        if n:
            log.info("cartella tmp svuotata all'avvio: %d voci di tentativi precedenti", n)
        return n

    def controlla(self) -> None:
        """Punto di ripresa: se il battito ha perso il lease, il lavoro si ferma qui.

        Sta fra il download e l'analisi, che e' il confine fra le due cose lunghe di questo worker: piu'
        in la' la lettura del file non e' interrompibile, e a fermare il processo e' l'uscita forzata
        del battito (C16)."""
        if self.battito is not None:
            self.battito.controlla()

    def segna(self, fase: str) -> None:
        if self.battito is not None:
            self.battito.segna_fase(fase)

    def esegui_per_sempre(self, una_volta: bool = False) -> None:
        log.info("worker %s collegato a %s", self.worker_id, self.api.url)
        attesa = 5
        while True:
            try:
                extra = {"postazione": socket.gethostname().upper()}
                if self.ultimo_arresto:
                    extra["ultimo_arresto"] = self.ultimo_arresto
                r = self.api.claim("analisi", self.worker_id, extra=extra)
                self.ultimo_arresto = ""            # riportato una volta: il server lo conserva
                job = Job.model_validate(r) if r else None
                attesa = 5
            except (ImprontaSbagliata, ErroreHTTP, *ERRORI_RETE) as e:
                # Stessa regola del worker Outlook: il log deve dire se il server ha risposto (e che
                # cosa) o se non si è fatto trovare. Sono due indagini diverse.
                d = diagnosi(e)
                (log.error if d.grave else log.warning)(
                    "%s: riprovo fra %d s", d.testo, d.pausa_s or attesa)
                if una_volta and d.grave:
                    return
                time.sleep(d.pausa_s or attesa)
                attesa = 5 if d.pausa_s else min(attesa * 2, 30)
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
        # Il battito sta su un thread suo per tutta la durata del job, come nel worker Outlook. Il
        # lease di un'analisi e' di 120 secondi (coda.go) e un PDF da qualche centinaio di pagine, o un
        # download da un server lento, ci arrivano senza fatica: senza battito il server riprende il
        # job a meta' lavoro e poi RIFIUTA il result con 409 — analisi fatta, tempo buttato, proposta
        # invariata. Se il 409 arriva lo stesso, il thread alza il flag: il lavoro si ferma al primo
        # punto di ripresa (controlla) e, se e' dentro una lettura che non ritorna, dopo
        # arresto_forzato_s il processo esce con codice 3 e l'attivita' pianificata lo riavvia (C16).
        with Battito(self.api, job.job_id, self.worker_id, job.lease_token,
                     ogni_s=cadenza_battito(job.lease_s), arresto_forzato_s=self.arresto_forzato_s) as b:
            b.marcatore_arresto = self.marcatore_arresto
            self.battito = b
            b.segna_fase(job.tipo)
            try:
                if job.tipo != "analizza_allegato":
                    raise ValueError(f"tipo job imprevisto per worker analisi: {job.tipo}")
                p = PayloadAnalizzaAllegato.model_validate(job.payload)
                ris = self.analizza(job, p)
            except ArrestoRichiesto as e:
                # il tentativo non e' piu' nostro: niente result, il job e' gia' di un altro tentativo
                log.warning("job %d interrotto: %s", job.job_id, e)
                self.battito = None
                return
            except Exception as e:
                log.exception("job %d errore: %s", job.job_id, e)
                ris = RisultatoRichiesta(esito="errore", errore=f"{type(e).__name__}: {e}"[:2000])
            perso = b.arresto.is_set()
        self.battito = None
        if perso:
            # il lease e' di un altro tentativo: riportare adesso sarebbe scrivere sopra al suo lavoro
            log.warning("job %d: lease perso durante l'analisi, risultato non riportato", job.job_id)
            return

        riporta_risultato(self.api, job.job_id, ris.model_dump(mode="json"), self.worker_id, job.lease_token, log)
        log.info("job %d completato in %.2fs -> %s", job.job_id, time.time() - t0, ris.esito)

    def analizza(self, job: Job, p: PayloadAnalizzaAllegato) -> RisultatoRichiesta:
        """Scarica il contenuto dal server, lo analizza, lo cancella (7C.1, P0).

        Il payload NON dice dove sta il file: dice quale allegato e' e che sha256 deve avere. I byte
        si prendono con GET /api/v1/allegati/{id}/contenuto dentro questo tentativo, si scrivono in
        `tmp\\<job>\\` e si confrontano con lo sha256 del payload — un file arrivato a meta' e' un
        file diverso, e si ripete. Un `path_staging` in un job vecchio ancora in coda si ignora:
        un percorso sul disco di un altro PC non dice niente a questo.
        """
        locale = os.path.join(self.staging, "tmp", str(job.job_id))
        ext = Path(p.nome_file).suffix.lower()
        dest = os.path.join(locale, "contenuto" + (ext if re.fullmatch(r"\.[a-z0-9]{1,10}", ext) else ""))
        try:
            try:
                self.segna(f"scarico {p.nome_file} ({p.bytes} byte)")
                n, sha = self.api.scarica_contenuto(str(p.allegato_id), job.job_id, job.lease_token,
                                                    self.worker_id, dest)
            except ErroreHTTP as e:
                if e.tentativo_non_valido:
                    raise ArrestoRichiesto(f"download rifiutato: {e.corpo[:200]}") from e
                if e.stato in (404, 410, 422):
                    # il contenuto non c'e' piu' nella cache del server, o il job non e' l'analisi di
                    # questo allegato: ritentare darebbe lo stesso esito
                    log.error("job %d contenuto non disponibile: %s", job.job_id, e)
                    return RisultatoRichiesta(esito="errore", definitivo=True,
                                              errore=f"contenuto non disponibile sul server: usa Riscarica ({e.corpo[:300]})")
                raise                                   # 5xx: il job fallisce senza «definitivo» e viene ritentato
            atteso = (p.sha256 or "").lower()
            if atteso and sha != atteso:
                raise ContenutoIncompleto(f"sha256 del contenuto scaricato {sha[:12]}… diverso da quello atteso {atteso[:12]}…")
            if p.bytes and n != p.bytes:
                raise ContenutoIncompleto(f"scaricati {n} byte, attesi {p.bytes}")
            self.controlla()
            self.segna(f"analisi di {p.nome_file}")
            esito = analizza_file(dest, p.nome_file)
        finally:
            rimuovi_con_pazienza(dest)
            try:
                if os.path.isdir(locale):
                    os.rmdir(locale)
            except OSError as e:
                log.warning("cartella temporanea non rimossa: %s (%s)", locale, e)
        ris_analisi = RisultatoAnalisi(
            allegato_id=p.allegato_id,
            tipo_proposto=esito["tipo_proposto"],
            codice=esito.get("codice", ""),
            rev=esito.get("rev", ""),
            confidenza=esito.get("confidenza", 50),
            fonte=esito.get("fonte", "cartiglio"),
            dettagli=esito.get("dettagli", {}),
            versione_analizzatore=p.versione_analizzatore,
            hash_configurazione=p.hash_configurazione,
        )
        return RisultatoRichiesta(esito="ok", dati=ris_analisi.model_dump(mode="json"))


def main():
    p = argparse.ArgumentParser(description="Worker Analisi per Cockpit RFQ")
    p.add_argument("--config", default=os.path.join(os.path.dirname(__file__), "worker.toml"), help="percorso file di configurazione")
    p.add_argument("--una-volta", action="store_true", help="esegui un solo job ed esci")
    p.add_argument("--debug", action="store_true")
    args = p.parse_args()

    cfg = carica_config(args.config, sezione="analisi")
    configura_log(args.debug, cfg, "worker_analisi")
    w = WorkerAnalisi(cfg)
    w.esegui_per_sempre(una_volta=args.una_volta)


if __name__ == "__main__":
    main()
