package motore_a

// snapshot.go: la fotografia unica della Distinta nuova (giro 5, A1d-P2; direttiva A1d §3.1, §3.4; bozza delle risposte
// §14 P2/Q2: «lo stesso snapshot REPEATABLE READ READ ONLY per fotografia, esistenza della RFQ, fase e dichiarazioni
// STEP»). Nuovo rispetto alla PR #18 (c543e89), che leggeva la fase e l'esistenza della RFQ in una transazione READ
// ONLY sua, senza REPEATABLE READ, prima di quella del caricatore (F-04 della revisione).
//
// Una sola transazione esterna REPEATABLE READ READ ONLY, aperta qui, sempre chiusa con ROLLBACK e mai confermata.
// Dentro, nell'ordine e sulla stessa connessione:
//  1. GetThread: la RFQ esiste (pgx.ErrNoRows = ErrRFQInesistente, senza leggere il testo dell'errore del caricatore);
//  2. la fase registrata: GetFaseCorrente (la vista v_thread_fase) e ListFaseCatalogo (l'etichetta);
//  3. caricatore.Carica con un Iniziatore che, invece di aprire una transazione nuova, apre un SAVEPOINT della
//     transazione esterna (pgx Tx.Begin). I due SHOW di controllo del caricatore (apriInSolaLettura) leggono lo stato
//     della transazione esterna, quindi «on» e «repeatable read»; il ROLLBACK del caricatore diventa ROLLBACK TO
//     SAVEPOINT e lascia aperta la transazione esterna, con la sua istantanea. Il caricatore non cambia (A1c congelato):
//     cambia solo chi gli dà la transazione [T];
//  4. le dichiarazioni dello STEP (fascicolo.LeggiDichiarazioni: ListDichiarazioniRfq + ValutaDichiarazioni), che il
//     caricatore non legge: la validità delle autorizzazioni la dice il Fascicolo, qui non si ricostruisce.
//
// In REPEATABLE READ l'istantanea nasce alla prima lettura (GetThread) e vale per tutte le altre, compresa la
// fotografia: esistenza, fase, fotografia e dichiarazioni sono dello stesso istante. SELECT now() del caricatore dà
// l'inizio della transazione esterna.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// opzioniSnapshot: le opzioni della transazione esterna, le stesse che il caricatore chiede (caricatore.go,
// apriInSolaLettura).
var opzioniSnapshot = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// lettura: ciò che lo snapshot legge per un thread.
type lettura struct {
	Foto          fotorfq.Fotografia
	Fase          *Fase
	Dichiarazioni fascicolo.Dichiarazioni
}

// tempoRollback: quanto può durare il ROLLBACK dello snapshot, staccato dal contesto della richiesta [T].
const tempoRollback = 2 * time.Second

// nelloSnapshot apre la transazione esterna REPEATABLE READ READ ONLY, chiama f e la chiude con ROLLBACK, sempre: in sola
// lettura non c'è niente da confermare. Pool nil = ErrLettura.
//
// Il ROLLBACK usa un contesto staccato da quello della richiesta (context.WithoutCancel), con un suo limite breve
// (R-04 della revisione di A1d-P2): se il limite scade o il client chiude la richiesta fra una lettura e il ROLLBACK,
// un ROLLBACK con il contesto già chiuso fallirebbe e pgx distruggerebbe la connessione invece di restituirla al pool.
// Se la lettura era stata interrotta a metà, pgx ha già chiuso la connessione comunque.
func nelloSnapshot(ctx context.Context, p Iniziatore, f func(tx pgx.Tx) error) error {
	if p == nil {
		return fmt.Errorf("%w: nessun database", ErrLettura)
	}
	tx, err := p.BeginTx(ctx, opzioniSnapshot)
	if err != nil {
		return fmt.Errorf("%w: la transazione in sola lettura non si apre: %w", ErrLettura, err)
	}
	defer func() {
		rc, fine := context.WithTimeout(context.WithoutCancel(ctx), tempoRollback)
		defer fine()
		_ = tx.Rollback(rc)
	}()
	return f(tx)
}

// savepoint: l'Iniziatore che il caricatore riceve dentro lo snapshot. BeginTx non apre una transazione: apre un
// savepoint della transazione esterna (pgx Tx.Begin), su cui il caricatore fa i suoi SHOW, le sue letture e il suo
// ROLLBACK (che diventa ROLLBACK TO SAVEPOINT). Accetta solo le opzioni dello snapshot: un chiamante che chiedesse
// altro (un'altra modalità, un altro isolamento) riceverebbe un errore e non una transazione diversa da quella chiesta.
type savepoint struct{ tx pgx.Tx }

func (s savepoint) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	if o.IsoLevel != opzioniSnapshot.IsoLevel || o.AccessMode != opzioniSnapshot.AccessMode {
		return nil, fmt.Errorf("motore A: il savepoint dello snapshot vale solo per REPEATABLE READ READ ONLY (chiesto %q %q)",
			o.IsoLevel, o.AccessMode)
	}
	return s.tx.Begin(ctx)
}

// esisteIn: GetThread sulla transazione; pgx.ErrNoRows = ErrRFQInesistente.
func esisteIn(ctx context.Context, q *db.Queries, thread uuid.UUID) error {
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRFQInesistente
		}
		return fmt.Errorf("%w: la RFQ non si legge: %w", ErrLettura, err)
	}
	return nil
}

// leggiSnapshot: la lettura di un thread, tutta nello stesso snapshot (i quattro passi del commento di testa).
func (s Servizio) leggiSnapshot(ctx context.Context, thread uuid.UUID) (lettura, error) {
	var l lettura
	err := nelloSnapshot(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := esisteIn(ctx, q, thread); err != nil {
			return err
		}
		fase, err := leggiFase(ctx, q, thread)
		if err != nil {
			return err
		}
		// Richiesta del solo thread, senza Messaggi né Clienti (direttiva §3.1: «la fotografia del solo thread»): la
		// fotografia non ha messaggi fuori RFQ e ha il solo cliente del thread (vedi calcolo.go per l'impronta).
		f, err := caricatore.Carica(ctx, savepoint{tx}, caricatore.Richiesta{Thread: []uuid.UUID{thread}})
		if err != nil {
			return fmt.Errorf("%w: la fotografia non si legge: %w", ErrLettura, err)
		}
		d, err := fascicolo.LeggiDichiarazioni(ctx, q, thread)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrLettura, err)
		}
		l = lettura{Foto: f, Fase: fase, Dichiarazioni: d}
		return nil
	})
	return l, err
}

// leggiFase: la fase registrata dalla vista v_thread_fase (GetFaseCorrente) con l'etichetta del catalogo
// (ListFaseCatalogo); nessuna fase aperta = nil. Le due query sono SELECT già generate (platform/db).
func leggiFase(ctx context.Context, q *db.Queries, thread uuid.UUID) (*Fase, error) {
	v, err := q.GetFaseCorrente(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: la fase non si legge: %w", ErrLettura, err)
	}
	cat, err := q.ListFaseCatalogo(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: il catalogo delle fasi non si legge: %w", ErrLettura, err)
	}
	descrizione := ""
	for _, c := range cat {
		if c.NomeFase == v.NomeFase {
			descrizione = c.Descrizione
			break
		}
	}
	return &Fase{Nome: string(v.NomeFase), Etichetta: descrizione, Inizio: v.Inizio, GgInFase: v.GgInFase,
		Semaforo: v.Semaforo, Terminale: v.Terminale}, nil
}

// Esiste: il solo controllo d'esistenza della RFQ (GetThread) in una transazione REPEATABLE READ READ ONLY, con il
// limite del servizio (quello delle regole, o LimiteDiRipiego); è ciò che fa HEAD, senza calcolo. nil = esiste;
// ErrRFQInesistente; ErrTempo se la lettura non è finita entro il limite (HTTP-01 della revisione di A1d-P2: 504, non
// 500); ErrLettura altrimenti.
func (s Servizio) Esiste(ctx context.Context, thread uuid.UUID) error {
	ctx, cancel := s.conLimite(ctx)
	defer cancel()
	err := nelloSnapshot(ctx, s.Pool, func(tx pgx.Tx) error { return esisteIn(ctx, db.New(tx), thread) })
	if err != nil && !errors.Is(err, ErrRFQInesistente) && scaduto(ctx) {
		return s.tempoScaduto(ctx, err)
	}
	return err
}
