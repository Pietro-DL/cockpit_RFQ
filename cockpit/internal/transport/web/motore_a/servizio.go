package motore_a

// servizio.go: il servizio della Distinta nuova del motore A (giro 5, A1d-P2; direttiva A1d dell'11/10, §3; piano
// P2-P3 dell'11/10). Derivato dalla PR #18 di Stefano Valo (c543e89: AnteprimaMotoreA, calcolaConLimite, calcolaMotoreA,
// faseMotoreADa), ristrutturato: il tipo sta qui e non in transport/web, gli errori sono esportati e distinti, gli agganci
// delle prove non sono più campi esportati del tipo di produzione (F-01, F-14 della revisione della PR).
//
// Il servizio non guarda né il flag né il ruolo: la porta (404 a flag spento, prima della sessione), soloAdmin, il
// no-store e i codici HTTP stanno nell'adattatore transport/web/motore_a.go. Qui si legge, si calcola e si compone,
// sempre in sola lettura: nessuna scrittura, nessun job, nessuna cache, nessuno stato del dominio fra le richieste
// (direttiva §3.1, §3.5). Lo stato è Regole, preparato una volta all'avvio da app/runtime e mai cambiato, più il
// semaforo dei calcoli (MassimoCalcoli), che è un limite di risorse e non tiene dati.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// VersioneAttesa: la versione dell'esito che questo package sa leggere (direttiva A1d §0: «obbligatoriamente
// valutazione-3», avanzata a valutazione-4 nella notte del 14/10, a valutazione-5 il 14/10 con la tabella delle classi del
// cliente: l'origine del tipo proposto_dalla_grammatica e NodoBOM.Classe, rilette qui). È un letterale, non valutazione.VersioneValutazione: confrontare la costante con sé stessa sarebbe
// sempre vero. Se il core cambia versione, Calcola risponde ErrVersione finché i modelli, l'indice e le etichette non
// sono riletti contro la versione nuova; una prova del QA confronta questo letterale con la costante del core.
const VersioneAttesa = "valutazione-5"

// Gli errori del calcolo, distinti (direttiva §3.1: «distinguere errori di configurazione, RFQ inesistente, timeout ed
// errori di valutazione»). Si riconoscono con errors.Is; il testo che li accompagna è tecnico e va solo nel log: al
// client l'adattatore manda un testo generico.
var (
	// ErrRegole: le regole del motore A non sono pronte (Regole.Pronta() falso). Si dice PRIMA di calcolare: con le
	// regole nil valutazione.Calcola non sbaglia, ma dà ogni thread senza_grammatica_a.
	ErrRegole = errors.New("motore A: regole non pronte")
	// ErrRFQInesistente: il thread chiesto non esiste (GetThread nella stessa transazione della fotografia).
	ErrRFQInesistente = errors.New("motore A: la RFQ non esiste")
	// ErrTempo: il calcolo non è finito entro Regole.Limite (limiti.anteprima.tempo_massimo_ms dell'indice).
	ErrTempo = errors.New("motore A: tempo scaduto")
	// ErrPanic: un panic nella lettura o nel calcolo, recuperato; la pila è nel testo dell'errore.
	ErrPanic = errors.New("motore A: panic nel calcolo")
	// ErrVersione: l'esito ha una versione diversa da VersioneAttesa.
	ErrVersione = errors.New("motore A: versione dell'esito diversa da quella che la pagina legge")
	// ErrValutazione: valutazione.Calcola ha dato un errore (di contratto della fotografia), o l'esito non ha il thread.
	ErrValutazione = errors.New("motore A: valutazione non riuscita")
	// ErrLettura: il database non ha risposto (la transazione non si apre, una lettura non riesce, il caricatore
	// rifiuta). Non è una RFQ inesistente.
	ErrLettura = errors.New("motore A: lettura non riuscita")
	// ErrOccupato: ci sono già MassimoCalcoli calcoli in corso (anche orfani, finiti oltre il limite ma non ancora
	// conclusi): il calcolo non parte e non legge niente (R-01 della revisione di A1d-P2). L'adattatore risponde 503.
	ErrOccupato = errors.New("motore A: troppi calcoli in corso")
)

// MassimoCalcoli: quanti calcoli della pagina girano insieme nel processo, al più [T] (R-01 della revisione di A1d-P2).
// valutazione.Calcola non guarda il contesto (A1c congelato): un calcolo oltre il limite risponde ErrTempo ma continua in
// memoria finché non finisce. Senza un tetto, ricariche ripetute di una RFQ pesante accumulerebbero calcoli pieni in
// parallelo e rallenterebbero tutto il Cockpit. Il posto si prende prima di leggere e si lascia quando la goroutine del
// calcolo finisce davvero, anche dopo il limite; oltre il tetto la risposta è subito ErrOccupato. Due: le rotte sono
// solo dell'amministratore, e due calcoli insieme lasciano libera la CPU per il resto del Cockpit. Non è una cache né uno
// stato del dominio: è un limite di risorse, uguale per ogni richiesta.
const MassimoCalcoli = 2

// postiCalcolo: il semaforo di processo dei calcoli (MassimoCalcoli posti).
var postiCalcolo = make(chan struct{}, MassimoCalcoli)

// LimiteDiRipiego: il limite di tempo delle letture quando le regole non ne danno uno (Regole.Limite <= 0) [T] (R-05,
// HTTP-03 della revisione di A1d-P2). Capita a flag acceso con l'indice assente o rifiutato: app/runtime esce prima di
// leggere i limiti, e l'anteprima dello STEP e gli HEAD, che non chiedono regole pronte, girerebbero senza scadenza
// (una lettura ferma dietro un lucchetto terrebbe impegnati il gestore e la connessione). Trenta secondi: sotto il tetto
// di 60 s che grammatica.Limiti.Valida dà al limite dell'indice.
const LimiteDiRipiego = 30 * time.Second

// Iniziatore: chi apre la transazione in sola lettura. Ha la forma di caricatore.Iniziatore: lo soddisfa
// *pgxpool.Pool, il Pool del server web.
type Iniziatore interface {
	BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error)
}

// Servizio: le letture della Distinta nuova. È un valore immutabile, che l'adattatore costruisce per ogni richiesta dai
// campi del Server (Regole = Server.MotoreA, Pool = Server.Pool, Log = Server.Log): nessun puntatore condiviso che una
// prova possa cambiare a server acceso (F-14).
//
// Pool nil = nessun database: ogni lettura dà ErrLettura. Le prove del package sostituiscono la lettura e la
// valutazione con i due campi non esportati, che fuori da qui non si vedono né si impostano.
type Servizio struct {
	Regole Regole
	Pool   Iniziatore
	Log    *slog.Logger

	// leggi: nil = leggiSnapshot (il database). Solo le prove del package.
	leggi func(ctx context.Context, thread uuid.UUID) (lettura, error)
	// valuta: nil = valutazione.Calcola. Solo le prove del package.
	valuta func(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error)
	// componi: nil = ComponiPagina. Solo le prove del package (un panic nella composizione).
	componi func(r *Risultato, rp RichiestaPagina) *Pagina
	// posti: nil = postiCalcolo, il semaforo del processo. Solo le prove del package, che ne danno uno loro per non
	// dipendere dai calcoli orfani delle altre prove.
	posti chan struct{}
}

// log: il log del servizio, o uno che scarta se manca.
func (s Servizio) log() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Log
}

// semaforo: il semaforo dei calcoli del servizio.
func (s Servizio) semaforo() chan struct{} {
	if s.posti != nil {
		return s.posti
	}
	return postiCalcolo
}

// limite: il limite delle regole, o LimiteDiRipiego se le regole non ne danno uno (indice assente o rifiutato, o una
// prova).
func (s Servizio) limite() time.Duration {
	if s.Regole.Limite > 0 {
		return s.Regole.Limite
	}
	return LimiteDiRipiego
}

// conLimite: il contesto con il limite del servizio (Servizio.limite: mai senza scadenza).
func (s Servizio) conLimite(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, s.limite())
}

// tempoScaduto: ErrTempo con il limite e l'errore del contesto, per il log; il dettaglio della lettura interrotta resta
// nel testo (dettaglio può essere nil).
func (s Servizio) tempoScaduto(ctx context.Context, dettaglio error) error {
	if dettaglio == nil {
		return fmt.Errorf("%w (limite %s): %w", ErrTempo, s.limite(), ctx.Err())
	}
	return fmt.Errorf("%w (limite %s): %w; %v", ErrTempo, s.limite(), ctx.Err(), dettaglio)
}

// scaduto: il contesto limitato è arrivato alla scadenza (non: la richiesta chiusa da chi l'ha fatta).
func scaduto(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}
