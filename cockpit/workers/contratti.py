"""Contratti JSON fra worker Python e cockpit.exe (fonte unica lato Python).

Speculare a internal/api/tipi.go. `python genera_contratti.py` esporta contracts/*.schema.json,
che i test di entrambe le parti validano: un cambio di contratto rompe i test, mai la produzione.
"""
from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class Base(BaseModel):
    model_config = ConfigDict(populate_by_name=True, extra="ignore")


# ---------------------------------------------------------------- ingest (FATTO)

class Destinatario(Base):
    nome: str = ""
    indirizzo: str = ""
    tipo: Literal["a", "cc", "ccn"] = "a"


class AllegatoIn(Base):
    indice: int
    nome_file: str
    estensione: str = ""
    content_type: str = ""
    natura: Literal["file", "inline", "elemento_outlook", "collegamento"] = "file"
    bytes: int = 0
    content_id: str = ""


class MessaggioIn(Base):
    """Un elemento Outlook letto via COM. Identità = message_id (Internet Message-ID), non entry_id."""
    message_id: str
    parent_message_id: str = ""
    entry_id: str
    store_id: str
    conversation_id: str = ""
    conversation_index: str = ""
    in_reply_to: str = ""
    riferimenti: list[str] = Field(default_factory=list)
    cartella: str
    direzione: Literal["entrata", "uscita"]
    data_evento: datetime
    mittente_nome: str = ""
    mittente_indirizzo: str = ""
    destinatari: list[Destinatario] = Field(default_factory=list)
    oggetto: str = ""
    corpo_testo: str = ""
    corpo_html: str = ""
    importanza: int = 1
    non_letto: bool = False
    flag_stato: int = 0
    categorie: list[str] = Field(default_factory=list)
    allegati: list[AllegatoIn] = Field(default_factory=list)


class IngestRichiesta(Base):
    messaggi: list[MessaggioIn]


class EsitoMessaggio(Base):
    message_id: str
    messaggio_id: UUID
    inserito: bool
    thread_id: UUID | None = None
    aggancio: str = "nessuno"
    allegati_da_stage: int = 0


class IngestRisposta(Base):
    inseriti: int
    aggiornati: int
    esiti: list[EsitoMessaggio]


# ---------------------------------------------------------------- coda job

class ClaimRichiesta(Base):
    worker: Literal["outlook", "analisi"]
    worker_id: str
    attesa_s: int = 20


class Job(Base):
    job_id: int
    tipo: str
    payload: dict
    tentativi: int = 1
    lease_s: int = 120


class HeartbeatRichiesta(Base):
    worker_id: str


class RisultatoRichiesta(Base):
    esito: Literal["ok", "errore"]
    dati: dict | None = None
    errore: str = ""
    definitivo: bool = False


# ---------------------------------------------------------------- payload e risultati per tipo di job

class CartellaCursore(Base):
    cartella: str
    ultimo_received: datetime | None = None


class PayloadSyncOutlook(Base):
    cartelle: list[CartellaCursore]
    dal: datetime
    al: datetime | None = None
    sovrapposizione_s: int = 600
    lotto: int = 50


class CartellaEsito(Base):
    cartella: str
    ultimo_received: datetime | None = None
    n_messaggi: int = 0
    errore: str = ""


class RisultatoSync(Base):
    cartelle: list[CartellaEsito]


class RiferimentoElemento(Base):
    """Riferimento stabile a un elemento Outlook: se l'EntryID è stantio (elemento spostato) il worker
    lo ricerca per message_id; messaggio_id serve al server per riallineare messaggio_outlook."""
    messaggio_id: UUID | None = None
    message_id: str = ""


class RisultatoElemento(Base):
    """Dove l'elemento è stato trovato davvero."""
    entry_id: str = ""
    store_id: str = ""
    cartella: str = ""


class PayloadStageAllegato(RiferimentoElemento):
    allegato_id: UUID
    entry_id: str
    store_id: str
    indice: int
    nome_file: str
    cartella: str


class RisultatoStage(RisultatoElemento):
    allegato_id: UUID
    path_staging: str
    sha256: str
    bytes: int


class PayloadCreaBozza(RiferimentoElemento):
    bozza_id: UUID
    tipo: Literal["risposta", "rispondi_tutti", "inoltro", "nuovo", "sollecito"]
    entry_id: str = ""
    store_id: str = ""
    destinatari: list[Destinatario] = Field(default_factory=list)
    oggetto: str = ""
    corpo_html: str = ""
    corpo_testo: str = ""
    allegati: list[str] = Field(default_factory=list)
    mostra: bool = True
    invia: bool = False


class RisultatoBozza(Base):
    entry_id: str
    inviata: bool = False


class PayloadApriElemento(RiferimentoElemento):
    entry_id: str
    store_id: str


class PayloadSpostaCartella(RiferimentoElemento):
    entry_id: str
    store_id: str
    cartella: str


class RisultatoSposta(Base):
    entry_id: str
    store_id: str = ""


class PayloadSegnaLetto(RiferimentoElemento):
    entry_id: str
    store_id: str
    letto: bool = True


class PayloadAnalizzaAllegato(Base):
    allegato_id: UUID
    path_staging: str
    sha256: str
    nome_file: str
    thread_id: UUID | None = None
    messaggio_id: UUID


class RisultatoAnalisi(Base):
    allegato_id: UUID
    tipo_proposto: str
    codice: str = ""
    rev: str = ""
    confidenza: int = 50
    fonte: str = "cartiglio"
    dettagli: dict = Field(default_factory=dict)


CONTRATTI = {
    "messaggio_in": MessaggioIn,
    "ingest_richiesta": IngestRichiesta,
    "ingest_risposta": IngestRisposta,
    "claim_richiesta": ClaimRichiesta,
    "job": Job,
    "risultato_richiesta": RisultatoRichiesta,
    "payload_sync_outlook": PayloadSyncOutlook,
    "risultato_sync": RisultatoSync,
    "payload_stage_allegato": PayloadStageAllegato,
    "risultato_stage": RisultatoStage,
    "risultato_elemento": RisultatoElemento,
    "payload_crea_bozza": PayloadCreaBozza,
    "risultato_bozza": RisultatoBozza,
    "payload_apri_elemento": PayloadApriElemento,
    "payload_sposta_cartella": PayloadSpostaCartella,
    "risultato_sposta": RisultatoSposta,
    "payload_segna_letto": PayloadSegnaLetto,
    "payload_analizza_allegato": PayloadAnalizzaAllegato,
    "risultato_analisi": RisultatoAnalisi,
}
