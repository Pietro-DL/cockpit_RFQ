package web

// motore_a_bozza_v1_motore.go: il calcolo del Motore A per R4 (?avvisi_motore=1) e per la connessione S di R5 (direttiva 39,
// blocco «R4 completo» della corsia persistenza; EM §8.2, §9.4, §9.5 con U2 §6.3.5). Come le altre rotte (/dati, R15, le
// forme di R2): Servizio.Calcola in sola lettura, con il semaforo dei calcoli (ErrOccupato senza un posto) e il limite del
// servizio. Il core (distintabozza, riepilogo3_motore.go) riceve il prodotto valutato, i gruppi e l'importazione di un gruppo
// e compone gli avvisi e i conflitti.
//
// R4 calcola il motore nella sua fotografia (il pool), prima della transazione di sola lettura del riepilogo; un errore del
// motore non e' un errore di R4 (200 con avvisi_motore_calcolati falso). R5 lo calcola dentro la connessione S: l'Iniziatore
// del servizio apre un SAVEPOINT della transazione S (REPEATABLE READ READ ONLY), cosi' il motore legge la stessa istantanea
// del resto del riepilogo ricalcolato. Se il calcolo scade, la goroutine del servizio puo' tenere ancora la connessione di S:
// R5 risponde subito 504 e chiude S con ROLLBACK (pgx distrugge la connessione se il ROLLBACK non riesce).

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// ingressoMotoreV1: il calcolo del motore nella forma del core per il prodotto (motorea_web.Risultato.IngressoMotoreV1,
// la stessa che S9 usa per i suggerimenti di adesso: giro G39, G39-01): il prodotto valutato (nil se il motore non lo
// valuta), i suoi gruppi, le occorrenze non attive, l'importazione di un gruppo, i gruppi degli altri prodotti finiti e gli
// STEP candidati sorgente dei finiti.
func ingressoMotoreV1(ris *motorea_web.Risultato, prodotto uuid.UUID) *distintabozza.IngressoMotoreR4 {
	return ris.IngressoMotoreV1(prodotto)
}

// erroreMotoreV1: un errore del servizio del motore negli errori che il core riconosce.
func erroreMotoreV1(err error) error {
	switch {
	case errors.Is(err, motorea_web.ErrOccupato):
		return fmt.Errorf("%w: %w", distintabozza.ErrMotoreOccupato, err)
	case errors.Is(err, motorea_web.ErrRegole):
		return fmt.Errorf("%w: %w", distintabozza.ErrMotoreNonPronto, err)
	case errors.Is(err, motorea_web.ErrTempo) || errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", distintabozza.ErrMotoreScaduto, err)
	}
	return err
}

// motoreR4: il calcolo del motore per R4 con ?avvisi_motore=1, nella sua fotografia; l'errore resta nell'esito.
func (s *Server) motoreR4(ctx context.Context, thread, prodotto uuid.UUID) *distintabozza.EsitoMotoreR4 {
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		if s.Log != nil && !errors.Is(err, motorea_web.ErrOccupato) && !errors.Is(err, motorea_web.ErrRegole) {
			s.Log.Warn("motore A, R4: avvisi del motore non calcolati", "thread", thread, "err", err)
		}
		return &distintabozza.EsitoMotoreR4{Err: erroreMotoreV1(err)}
	}
	return &distintabozza.EsitoMotoreR4{Ingresso: ingressoMotoreV1(ris, prodotto)}
}

// calcoloAdessoV1 (G39-01, G39-02 [U]; distintabozza/adesso3.go): il calcolo del motore di adesso per la bozza aperta del
// prodotto (bozza: quella del corpo di R2, uuid.Nil = quella aperta del prodotto, per R1), nella sua fotografia e con il
// semaforo dei calcoli, prima di ogni transazione: il registro del gruppo corrente della sorgente della bozza (nessuno senza
// una sorgente STEP o se il motore non propone piu' quel gruppo) e gli STEP candidati sorgente dei finiti.
func (s *Server) calcoloAdessoV1(ctx context.Context, thread, prodotto, bozza uuid.UUID) (*distintabozza.CalcoloAdesso, error) {
	var src *propostabom.SorgenteBozza
	err := s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		id := bozza
		if id == uuid.Nil {
			b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			id = b.BozzaID
		}
		x, ok, err := distintabozza.SorgenteDellaBozza(ctx, q, thread, prodotto, id)
		if ok {
			src = &x
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		return nil, err
	}
	return distintabozza.CalcoloAdessoDa(ingressoMotoreV1(ris, prodotto), src)
}

// stepCandidatiV1 (G39-02 [U]): i soli STEP candidati sorgente dei finiti del calcolo del motore di adesso, nella sua
// fotografia e con il semaforo dei calcoli, prima di ogni transazione; senza l'importazione di un gruppo (serve a R2 per
// una PB nuova o spostata su uno STEP, e alla nascita da-conferma-1, che non importano). L'errore del servizio torna a
// chi chiama: ErrRegole (regole non configurate) = il ripiego [T-39]; occupato, scaduto o in errore = 503/504/500.
func (s *Server) stepCandidatiV1(ctx context.Context, thread uuid.UUID) (*distintabozza.StepCandidati, error) {
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		return nil, err
	}
	return distintabozza.StepCandidatiDalMotore(ris.EsitoThread, ris.Thread), nil
}

// codiceErroreMotoreV1: il codice dell'esito del motore che non ha calcolato (R1 ?suggerimenti_motore=1, come gli avvisi
// del motore di R4): occupato o regole non pronte CALCOLO_OCCUPATO, scaduto CALCOLO_SCADUTO, altrimenti ERRORE_CALCOLO.
func codiceErroreMotoreV1(err error) string {
	e := erroreMotoreV1(err)
	switch {
	case errors.Is(e, distintabozza.ErrMotoreOccupato), errors.Is(e, distintabozza.ErrMotoreNonPronto):
		return distintabozza.MotoreErroreOccupato
	case errors.Is(e, distintabozza.ErrMotoreScaduto):
		return distintabozza.MotoreErroreScaduto
	}
	return distintabozza.CodiceErroreCalcolo
}

// motoreR5: il calcolo del motore dentro la connessione S di R5 (distintabozza.CalcoloMotoreR5).
func (s *Server) motoreR5(ctx context.Context, tx pgx.Tx, thread, prodotto uuid.UUID) (*distintabozza.IngressoMotoreR4, error) {
	svc := s.servizioMotoreA()
	svc.Pool = iniziatoreInS{tx: tx}
	ris, err := svc.Calcola(ctx, thread)
	if err != nil {
		return nil, erroreMotoreV1(err)
	}
	return ingressoMotoreV1(ris, prodotto), nil
}

// iniziatoreInS: l'Iniziatore del servizio del motore dentro la connessione S: un SAVEPOINT della transazione S, solo in
// sola lettura (la transazione S e' gia' REPEATABLE READ READ ONLY; chi chiede altro riceve un errore).
type iniziatoreInS struct{ tx pgx.Tx }

func (i iniziatoreInS) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	if o.AccessMode != pgx.ReadOnly {
		return nil, fmt.Errorf("motore A in R5: la connessione S e' solo in lettura (chiesto %q)", o.AccessMode)
	}
	return i.tx.Begin(ctx)
}
