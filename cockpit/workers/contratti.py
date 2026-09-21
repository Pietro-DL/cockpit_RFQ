"""Contratti JSON fra worker Python e cockpit.exe (fonte unica lato Python).

Speculare a internal/platform/contratti/api/tipi.go. `python genera_contratti.py` esporta contracts/*.schema.json,
che i test di entrambe le parti validano: un cambio di contratto rompe i test, mai la produzione.
"""
from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

from protocollo import ATTESA_CLAIM_S


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
    # Accettato ma IGNORATO dal server (voce 2.6): lo StoreID è del profilo Outlook di questa
    # postazione, non della copia, e il server lo apprende dal claim (casella_store).
    store_id: str = ""
    conversation_id: str = ""
    conversation_index: str = ""
    in_reply_to: str = ""
    riferimenti: list[str] = Field(default_factory=list)
    cartella: str
    direzione: Literal["entrata", "uscita"]
    data_evento: datetime
    # ReceivedTime in QUESTA casella, anche per la posta inviata. Chiude W2: il cursore avanzava su
    # data_evento (SentOn per la Posta inviata) mentre il filtro della scansione usa ReceivedTime, e
    # una mail scritta lunedi e inviata giovedi poteva spingere il cursore oltre elementi non letti.
    ricevuto_il: datetime | None = None
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
    # Le UserProperties `Cockpit*` dell'elemento (blocco 7B): CockpitBozza = bozza_id,
    # CockpitRichiestaFornitore = richiesta_id. Le ha scritte il Cockpit creando la bozza; il sync
    # della Posta inviata le rilegge e il server lega la mail a cio' che l'ha generata.
    marcatori: dict[str, str] = Field(default_factory=dict)


class CursoreLotto(Base):
    """Fin dove arriva questo lotto. Il server lo scrive nella stessa transazione degli elementi:
    o avanzano insieme o non avanza niente."""

    cartella: str
    ultimo_received: datetime


class ElementoSaltato(Base):
    """Elemento visto ma non convertito (com_error sul Body, elemento non-mail, ...). Non si butta:
    finisce in scarto con origine 'lettura' e si rilegge con un job dedicato, perché il payload
    completo qui non c'è."""

    entry_id: str
    cartella: str = ""
    message_id: str = ""
    ricevuto_il: datetime | None = None
    oggetto: str = ""
    errore: str


class IngestRichiesta(Base):
    messaggi: list[MessaggioIn]
    # Il lotto appartiene a una casella sola: casella_id sta qui, non nei singoli elementi.
    casella_id: UUID | None = None
    # Il tentativo che sta consegnando il lotto: senza, il server non potrebbe distinguere un worker
    # vivo da uno scaduto che sta ancora scrivendo.
    job_id: int = 0
    lease_token: str = ""
    worker_id: str = ""
    cursore: CursoreLotto | None = None
    saltati: list[ElementoSaltato] = Field(default_factory=list)


class EsitoMessaggio(Base):
    message_id: str
    messaggio_id: UUID | None = None
    inserito: bool = False
    thread_id: UUID | None = None
    aggancio: str = "nessuno"
    allegati_da_stage: int = 0
    errore: str = ""      # valorizzato = elemento scartato, non acquisito


class IngestRisposta(Base):
    inseriti: int
    aggiornati: int
    falliti: int = 0
    esiti: list[EsitoMessaggio]
    # quanto il SERVER ha impiegato a scrivere il lotto (7C.1): la differenza con il tempo della
    # chiamata HTTPS misurata dal worker e' la rete
    durata_ms: int = 0


# ---------------------------------------------------------------- coda job

class CasellaAperta(Base):
    """Come il worker vede una casella censita nel PROPRIO profilo Outlook (voce 2.6): lo store_id
    ha senso solo su questa postazione; il server lo registra in casella_store e non lo mette mai
    in un payload."""
    casella_id: UUID
    store_id: str


class CasellaServita(Base):
    """Una casella che il server chiede al worker di risolvere nel proprio profilo (risposta di
    GET /api/v1/worker/caselle). Il worker tocca solo queste: uno store del profilo che non è qui
    viene ignorato, non censito d'ufficio (M1)."""
    casella_id: UUID
    indirizzo: str
    nome: str = ""
    condivisa: bool = False


class ClaimRichiesta(Base):
    worker: Literal["outlook", "analisi"]
    worker_id: str
    attesa_s: int = ATTESA_CLAIM_S
    # Nome host da cui il worker gira: il server lo CONFRONTA con la postazione della credenziale
    # (un worker.toml copiato su un altro PC viene rifiutato), ma il routing usa la credenziale.
    postazione: str = ""
    outlook_ok: bool = True
    # Le caselle censite risolte nel profilo locale: il server interseca con l'autorizzazione (Q18)
    # e assegna solo job di queste (M12).
    caselle_aperte: list[CasellaAperta] = Field(default_factory=list)
    # Motivo dell'ultima uscita forzata (C16), letto dal marcatore al riavvio.
    ultimo_arresto: str = ""


class Job(Base):
    job_id: int
    tipo: str
    payload: dict
    tentativi: int = 1
    lease_s: int = 120
    # Identifica QUESTO tentativo: va rimandato in heartbeat, result e ingest. È l'unica cosa che
    # distingue il tentativo in corso da uno scaduto che sta ancora lavorando.
    lease_token: str = ""
    durata_max_s: int = 1800
    casella_id: UUID | None = None
    # Job interattivo destinato a QUESTA postazione (voce 2.2): il claim lo ha già filtrato.
    postazione_id: UUID | None = None


class HeartbeatRichiesta(Base):
    worker_id: str
    lease_token: str = ""


class RisultatoRichiesta(Base):
    esito: Literal["ok", "errore"]
    dati: dict | None = None
    errore: str = ""
    definitivo: bool = False
    worker_id: str = ""
    lease_token: str = ""


# ---------------------------------------------------------------- payload e risultati per tipo di job

# I tre modi di un sync_outlook (blocco 3 del 3R). Non sono tre meccanismi: sono tre modi di decidere
# la PRIMA finestra. Da li' in poi tutti e tre leggono un intervallo chiuso [dal, al] fissato
# all'accodamento, dal piu' recente al piu' vecchio, e il server sposta la frontiera solo quando il
# worker dichiara di aver percorso la finestra per intero.
MODO_AGGIORNAMENTO = "aggiornamento"
MODO_BOOTSTRAP = "bootstrap"
MODO_STORICO = "storico"


class CartellaCursore(Base):
    """Una cartella da leggere con la SUA finestra, decisa dal server all'accodamento.

    Il worker non calcola piu' nessun limite: due cartelle della stessa casella possono essere a
    punti diversi (una sincronizzata da mesi, una aggiunta stamattina), e il limite di ciascuna
    dipende dalla sua copertura, non da quella della vicina.
    """

    cartella: str
    dal: datetime | None = None          # limite inferiore di QUESTA cartella, sovrapposizione gia' sottratta
    al: datetime | None = None           # limite superiore di QUESTA cartella
    bootstrap: bool = False              # non aveva ancora una copertura: `dal` e' la finestra iniziale
    ultimo_received: datetime | None = None   # diagnostica: la mail piu' recente che il server ha
    coperto_fino_a: datetime | None = None    # diagnostica: fin dove si era gia' guardato


class PayloadSyncOutlook(Base):
    casella_id: UUID | None = None
    # vuoto = payload accodato prima del blocco 3 e rimasto in coda: si legge con modo_effettivo(),
    # dove l'unico segnale era il limite superiore. Il server ha la stessa scaletta (`modoDi`), e
    # devono restare d'accordo: se una parte credesse storico cio' che l'altra crede aggiornamento,
    # si muoverebbe la frontiera sbagliata.
    modo: str = ""
    cartelle: list[CartellaCursore]
    dal: datetime                        # inviluppo: il piu' vecchio dei `cartelle[].dal`
    al: datetime | None = None           # inviluppo: fissato all'accodamento
    sovrapposizione_s: int = 600
    lotto: int = 50


    def modo_effettivo(self) -> str:
        if self.modo:
            return self.modo
        return MODO_STORICO if self.al is not None else MODO_AGGIORNAMENTO


class CartellaEsito(Base):
    cartella: str
    ultimo_received: datetime | None = None
    n_messaggi: int = 0
    # elementi visti e non consegnati (non-mail, illeggibili): quelli con un EntryID viaggiano in
    # `saltati` dell'ingest e diventano scarti di lettura, questo e il conto di TUTTI.
    saltati: int = 0
    errore: str = ""
    # completa: la finestra [dal, al] e' stata percorsa per INTERO. E' l'unica cosa che fa avanzare
    # una frontiera, ed e' una dichiarazione positiva: un'enumerazione COM che si interrompe puo'
    # finire senza eccezioni e con `errore` vuoto, e in lettura dal piu' recente al piu' vecchio cio'
    # che resta fuori e' la parte VECCHIA della finestra. Assente = False = si rilegge.
    completa: bool = False


class RisultatoSync(Base):
    cartelle: list[CartellaEsito]
    # Dove il sync ha passato il suo tempo, in secondi (7C.1): com, serializzazione, https, server.
    tempi: dict[str, float] = Field(default_factory=dict)


class PayloadRileggiElemento(Base):
    """Rilettura mirata di un solo elemento, dopo che il worker non era riuscito a convertirlo."""

    casella_id: UUID
    entry_id: str
    cartella: str = ""
    message_id: str = ""


class RiferimentoElemento(Base):
    """Riferimento stabile a un elemento Outlook: IDENTITÀ LOGICHE, mai uno StoreID (voce 2.6, M12).

    casella_id dice in quale casella cercare: il worker la traduce nello store del PROPRIO profilo.
    Se l'EntryID è stantio (elemento spostato) il worker lo ricerca per message_id DENTRO quello
    store; messaggio_id e casella_id servono al server per riallineare la PRESENZA giusta: dalla
    0004 lo stesso messaggio ha un EntryID diverso in ogni casella."""
    messaggio_id: UUID | None = None
    casella_id: UUID | None = None
    message_id: str = ""


class RisultatoElemento(Base):
    """Dove l'elemento è stato trovato davvero. Nessuno store_id: al server non direbbe niente."""
    entry_id: str = ""
    cartella: str = ""


class PayloadStageAllegato(RiferimentoElemento):
    allegato_id: UUID
    entry_id: str
    indice: int
    nome_file: str
    cartella: str


class RisultatoStage(RisultatoElemento):
    """Chiude un download. Il file è già stato caricato con PUT /api/v1/allegati/{id}/file dallo
    stesso tentativo (voce 2.3): il server lo promuove a definitivo solo se questo result è valido e
    lo sha256 coincide. Nessun path_staging: un percorso sul disco del worker non dice niente al
    server, che può stare su un altro PC."""

    allegato_id: UUID
    sha256: str
    bytes: int


class PayloadCreaBozza(RiferimentoElemento):
    bozza_id: UUID
    tipo: Literal["risposta", "rispondi_tutti", "inoltro", "nuovo", "sollecito"]
    entry_id: str = ""
    destinatari: list[Destinatario] = Field(default_factory=list)
    oggetto: str = ""
    corpo_html: str = ""
    corpo_testo: str = ""
    allegati: list[str] = Field(default_factory=list)
    mostra: bool = True
    invia: bool = False
    # UserProperties `Cockpit*` da scrivere sulla bozza, oltre a CockpitBozza (blocco 7B)
    marcatori: dict[str, str] = Field(default_factory=dict)


class RisultatoBozza(Base):
    entry_id: str
    inviata: bool = False


class PayloadApriElemento(RiferimentoElemento):
    entry_id: str


class PayloadSpostaCartella(RiferimentoElemento):
    entry_id: str
    cartella: str


class RisultatoSposta(Base):
    entry_id: str


class PayloadSegnaLetto(RiferimentoElemento):
    entry_id: str
    letto: bool = True


class PayloadAnalizzaAllegato(Base):
    """Che cosa analizzare, non dove sta (7C.1, P0).

    Fino al banco a due macchine del 20/09/2026 c'era `path_staging`: il percorso del file sul
    disco del SERVER. Il worker sull'altro PC lo cercava sul proprio e falliva. Ora il worker si
    prende i byte con GET /api/v1/allegati/{id}/contenuto dentro il proprio tentativo, li verifica
    con `sha256` e li cancella dopo l'analisi. Un `path_staging` in un job vecchio ancora in coda
    viene ignorato, non letto."""
    allegato_id: UUID
    sha256: str
    bytes: int = 0
    nome_file: str
    thread_id: UUID | None = None
    messaggio_id: UUID
    # Con che cosa il server chiede di analizzare (voce 1.12). Il worker li rimanda indietro tali e
    # quali: sono la chiave sotto cui i fatti vengono conservati e ritrovati.
    versione_analizzatore: int = 0
    hash_configurazione: str = ""
    parametri: dict = Field(default_factory=dict)


class RisultatoAnalisi(Base):
    allegato_id: UUID
    tipo_proposto: str
    codice: str = ""
    rev: str = ""
    confidenza: int = 50
    fonte: str = "cartiglio"
    dettagli: dict = Field(default_factory=dict)
    # Eco del payload: il server rifiuta un risultato che dichiara una combinazione diversa da quella
    # richiesta, perché archivierebbe i fatti sotto una chiave che non li descrive.
    versione_analizzatore: int = 0
    hash_configurazione: str = ""


CONTRATTI = {
    "messaggio_in": MessaggioIn,
    "ingest_richiesta": IngestRichiesta,
    "ingest_risposta": IngestRisposta,
    "claim_richiesta": ClaimRichiesta,
    "casella_servita": CasellaServita,
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
    "payload_rileggi_elemento": PayloadRileggiElemento,
    "heartbeat_richiesta": HeartbeatRichiesta,
    "payload_analizza_allegato": PayloadAnalizzaAllegato,
    "risultato_analisi": RisultatoAnalisi,
}
