// Package filerfq e' il servizio del ciclo dei file della Sorgente (V1, EM §10; PM23 §14 passo 5): lo stato di
// un'occorrenza (RFQ, allegato) — attivo, in Altri file, scartato —, i suoi eventi, la rimozione dallo staging in due
// tempi, la copia di conservazione degli «Altri file» (dietro l'interfaccia Conservatore), il ritorno ai file attivi, le dipendenze da
// controllare prima dei gesti, il custode che riprende quello che un gesto non ha finito, e la registrazione dei file
// caricati (E1′ e il ramo HTML di «carica»).
//
// Nome del pacchetto [T] della corsia file: l'EM nomina «il servizio del ciclo dei file» senza un pacchetto (MAT §3:
// «il nome del pacchetto lo sceglie chi lo scrive»); filerfq come le tabelle file_rfq*.
//
// Regole che valgono in tutto il pacchetto:
//   - lo stato dell'occorrenza lo scrive solo questo pacchetto (prova statica CO-12);
//   - la RFQ dell'occorrenza e' sempre messaggio.thread_id, letta insieme all'allegato;
//   - ordine dei lucchetti: la RFQ (BloccaThread), poi le righe (proposte, allegati, rimozioni, copie), per ultimo il
//     contenuto (BloccaContenuto), con piu' sha in ordine crescente (EM §10.6);
//   - nessun percorso esce verso il browser: chi chiama riceve stati e identificativi (EM §10.9, SIC-15);
//   - i byte si leggono e si scrivono solo dentro lo staging, con il contenimento controllato, e attraverso il
//     Conservatore per le copie degli Altri file (l'unico scrittore del NAS e' storage/nas).
package filerfq

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// Gli stati dell'occorrenza nella RFQ (file_rfq.stato). Nessuna riga vale attivo.
const (
	StatoAttivo    = "attivo"
	StatoAltriFile = "altri_file"
	StatoScartato  = "scartato"
)

// I gesti con chiave della Sorgente (esito_intento.gesto, EM §8.4).
const (
	GestoScarta    = "scarta_file"
	GestoAltriFile = "altri_file"
	GestoRiattiva  = "riattiva_file"
)

// Le origini degli eventi (file_rfq_evento.origine).
const (
	OriginePonte  = "ponte"
	OrigineLegacy = "legacy"
	OrigineServer = "server"
)

// Pipeline e' la strada d'ingresso di un file appena arrivato nello staging (workerapi.Server.DopoCaricamento): la
// «Riattiva» ci rimanda l'occorrenza come appena arrivata, senza un allegato nuovo (EM §10.6).
type Pipeline interface {
	DopoCaricamento(ctx context.Context, q *db.Queries, allegato uuid.UUID) error
}

// Contesto e' un contesto di lavorazione partner dichiarato per una RFQ, ridotto a quello che serve alla dipendenza
// D-13: l'id e il perimetro dei file. Il servizio non legge i file dei contesti: glieli passa chi li ha (il motore).
type Contesto struct {
	ID        string
	Perimetro []OccorrenzaContesto
}

// OccorrenzaContesto e' un'occorrenza del perimetro di un contesto: coincide con un file se coincide l'allegato,
// oppure lo sha256 E il nome ricevuto (e il contenitore, se scritto), mai per sola impronta (GP1:60).
type OccorrenzaContesto struct {
	AllegatoID  string
	Sha256      string
	Nome        string
	Contenitore string
}

// Servizio e' il ciclo dei file. I campi si impostano una volta all'avvio (app/runtime) o nelle prove.
type Servizio struct {
	Pool *pgxpool.Pool
	// Staging: la radice dello staging ([nas].staging, assoluta). Vuota = niente si legge ne' si cancella dallo
	// staging (il tempo 2 resta in attesa).
	Staging string
	// Conservatore: il posto delle copie degli «Altri file» (conservatore.go); nil = nessun posto su questo server (S3
	// risponde 503 NON_CONFIGURATO, i rilasci aspettano).
	Conservatore Conservatore
	// NASRadice: la radice del NAS, solo in lettura (una sorgente dei byte per la copia di conservazione e per S5).
	NASRadice string
	// Pipeline: la strada d'ingresso, per la «Riattiva». Nil = la «Riattiva» non rimette il file in analisi (solo nelle
	// prove senza worker).
	Pipeline Pipeline
	// Contesti: i contesti partner della RFQ, per D-13. Nil = nessun contesto.
	Contesti func(thread uuid.UUID) []Contesto
	Log      *slog.Logger
	// Adesso: l'orologio. Nil = time.Now (le prove lo spostano avanti).
	Adesso func() time.Time
	// TempoTentativo: il tempo massimo del tentativo del tempo 2 e del rilascio subito dopo il COMMIT del gesto (EM
	// §10.4: «proposta: 2 secondi»). Zero = 2 secondi.
	TempoTentativo time.Duration
	// EtaMinima: un file dello staging piu' giovane di cosi' non si cancella (la stessa della cache). Zero = 10 minuti.
	EtaMinima time.Duration
	// Agganci: solo per le prove di concorrenza (PM23 SC-16, CO-06, AF-11, CO-15). Nil in produzione.
	Agganci *Agganci
}

// Agganci sono i punti in cui una prova puo' fermare un protocollo per far succedere qualcos'altro in mezzo. Ogni
// funzione nil non fa niente.
type Agganci struct {
	// PrimaDiCancellare: tempo 2, fra il passo 9 e il passo 10 (dentro la transazione, con BloccaContenuto preso).
	PrimaDiCancellare func(rimozioneID int64)
	// DopoLaCancellazione: tempo 2, fra os.Remove e il COMMIT (PM23 SC-06: un COMMIT che fallisce dopo la cancellazione).
	DopoLaCancellazione func(rimozioneID int64)
	// DopoLaCopia: «Metti in Altri file», fra la copia verificata e la fase C.
	DopoLaCopia func(allegato uuid.UUID)
	// PrimaDiCopiare: «Metti in Altri file», prima della copia di ogni riga del gesto (fuori transazione): una prova vi
	// chiude la riga con uno «Scarta» per il 409 tentativo_chiuso nella copia.
	PrimaDiCopiare func(allegato uuid.UUID)
	// PrimaDellaFaseC: «Riattiva», fra il controllo dello staging (passo 2) e la fase C.
	PrimaDellaFaseC func(allegato uuid.UUID)
	// PrimaDellaRimozioneNAS: il rilascio di una copia degli Altri file, fra il passo 1 (la riga in_rimozione, COMMIT) e
	// la rimozione dal NAS (U2 §3.7): una prova vi controlla che nessuna transazione tenga la riga.
	PrimaDellaRimozioneNAS func(conservazioneID int64)
	// SaltaTentativi: dopo il COMMIT di un gesto non si fa il tentativo del tempo 2 ne' quello del rilascio: li fa il
	// custode. Le prove lo chiedono per guardare gli stati intermedi.
	SaltaTentativi bool
}

func (s *Servizio) adesso() time.Time {
	if s.Adesso != nil {
		return s.Adesso()
	}
	return time.Now()
}

func (s *Servizio) tempoTentativo() time.Duration {
	if s.TempoTentativo > 0 {
		return s.TempoTentativo
	}
	return 2 * time.Second
}

func (s *Servizio) etaMinima() time.Duration {
	if s.EtaMinima > 0 {
		return s.EtaMinima
	}
	return 10 * time.Minute
}

func (s *Servizio) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// ConservazioneConfigurata: c'e' un posto per le copie degli «Altri file».
func (s *Servizio) ConservazioneConfigurata() bool { return s.Conservatore != nil }

// Attivo e' la gemella Go del predicato SQL file_rfq_attivo (EM §10.7): un'occorrenza e' attiva senza riga di stato o
// con lo stato attivo. Uno scarto col vecchio «Scarta il file» vale attivo per il predicato (le viste escludono gia' le
// proposte scartate); il ciclo dei file lo tratta come scartato (Scartato).
func Attivo(stato string) bool { return stato == "" || stato == StatoAttivo }

// attesaDopo: le attese crescenti dei ritentativi del tempo 2 e del rilascio (EM §10.4: 1, 5, 15, 60 minuti, poi ogni
// 6 ore), dopo il tentativo n (contato da 1).
func attesaDopo(tentativo int32) time.Duration {
	switch {
	case tentativo <= 1:
		return time.Minute
	case tentativo == 2:
		return 5 * time.Minute
	case tentativo == 3:
		return 15 * time.Minute
	case tentativo == 4:
		return time.Hour
	}
	return 6 * time.Hour
}
