package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// Copia è UNA presenza di un messaggio: la casella e l'EntryID dell'elemento in quella casella. È
// ciò che un job porta per ritrovare l'elemento — insieme al Message-ID — e niente di più: lo store
// lo risolve il worker nel proprio profilo (voce 2.6).
type Copia struct {
	CasellaID   uuid.UUID
	CasellaNome string
	EntryID     string
	NonLetto    bool
	// WorkerNome è il worker della postazione che serve questa copia (solo per CopiaPerPostazione).
	WorkerNome string
}

// ErrNessunaPostazione: la sessione non è associata a nessuna postazione, quindi non c'è un PC su
// cui far succedere l'azione. Non si sceglie «una postazione qualsiasi»: una finestra aperta sul PC
// sbagliato non è un'azione riuscita.
var ErrNessunaPostazione = errors.New("nessuna postazione associata alla sessione")

// ErrNessunWorkerIdoneo: la postazione del richiedente non ha un worker Outlook autorizzato su nessuna
// casella in cui il messaggio è presente (M4, M10). Il job non si crea e il motivo lo dice Motivo.
type ErrNessunWorkerIdoneo struct {
	Postazione string
	Servite    []string // caselle che i worker della postazione servono
	Presenze   []string // caselle in cui il messaggio si trova
}

func (e *ErrNessunWorkerIdoneo) Error() string { return e.Motivo() }

// Motivo è la frase per l'operatore: dice che cosa manca, non solo che l'azione non parte.
func (e *ErrNessunWorkerIdoneo) Motivo() string {
	if len(e.Servite) == 0 {
		return fmt.Sprintf("nessun worker Outlook è configurato sulla postazione %s: nessuna azione su Outlook può partire da qui", e.Postazione)
	}
	return fmt.Sprintf("il worker Outlook di %s serve %s, ma questo messaggio è presente solo in %s: l'azione non parte e non viene dirottata su un altro PC",
		e.Postazione, strings.Join(e.Servite, ", "), strings.Join(e.Presenze, ", "))
}

// CopiaPerPostazione decide SU QUALE COPIA agisce un job interattivo chiesto da `richiedente` sulla
// postazione `postazione` (voci 2.2 e 2.7, M3): la copia in una casella attiva che un worker Outlook
// di quella postazione è autorizzato a servire. Preferisce la casella personale del richiedente, poi
// la copia più vecchia. Senza postazione → ErrNessunaPostazione; senza copia servibile →
// *ErrNessunWorkerIdoneo con il motivo. Mai un ripiego su un'altra postazione.
func CopiaPerPostazione(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, postazione uuid.NullUUID, richiedente uuid.UUID) (Copia, error) {
	if !postazione.Valid {
		return Copia{}, ErrNessunaPostazione
	}
	r, err := q.CopiaPerPostazione(ctx, db.CopiaPerPostazioneParams{
		PostazioneID: postazione, MessaggioID: messaggioID, Richiedente: uuid.NullUUID{UUID: richiedente, Valid: true},
	})
	if err == nil {
		return Copia{CasellaID: r.CasellaID, CasellaNome: r.CasellaNome, EntryID: r.EntryID, NonLetto: r.NonLetto, WorkerNome: r.WorkerNome}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Copia{}, err
	}
	// il motivo: quali caselle serve la postazione, in quali sta il messaggio
	e := &ErrNessunWorkerIdoneo{Postazione: postazione.UUID.String()[:8]}
	if p, err := q.GetPostazione(ctx, postazione.UUID); err == nil {
		e.Postazione = p.NomeHost
	}
	if servite, err := q.CaselleServiteDaPostazione(ctx, postazione); err == nil {
		for _, c := range servite {
			e.Servite = append(e.Servite, c.Nome)
		}
	}
	if presenze, err := q.ListPresenze(ctx, messaggioID); err == nil {
		for _, p := range presenze {
			e.Presenze = append(e.Presenze, p.CasellaNome)
		}
	}
	if len(e.Presenze) == 0 {
		return Copia{}, errors.New("il messaggio non è presente in nessuna casella attiva")
	}
	return Copia{}, e
}

// CopiaPerDownload sceglie da quale copia scaricare un allegato. Un download non apre finestre e non
// ha bisogno del PC del richiedente: lo esegue qualunque worker autorizzato sulla casella. Se però la
// postazione della sessione serve una delle copie, si preferisce quella: il file arriva dal worker
// che sta accanto a chi lo ha chiesto, e il caso a due PC (M11) fa esattamente ciò che ci si aspetta.
// Altrimenti la copia di riferimento (la più vecchia, ripetibile).
func CopiaPerDownload(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, postazione uuid.NullUUID, richiedente uuid.UUID) (Copia, error) {
	if postazione.Valid {
		c, err := CopiaPerPostazione(ctx, q, messaggioID, postazione, richiedente)
		if err == nil {
			return c, nil
		}
		var nessuno *ErrNessunWorkerIdoneo
		if !errors.As(err, &nessuno) {
			return Copia{}, err
		}
	}
	pr, err := q.PresenzaDaAprire(ctx, messaggioID)
	if err != nil {
		return Copia{}, err
	}
	return Copia{CasellaID: pr.CasellaID, CasellaNome: pr.CasellaNome, EntryID: pr.EntryID, NonLetto: pr.NonLetto}, nil
}

// ScadenzaInterattiva: oltre quanto un job interattivo non serve più (piano §2.3): «Apri» 10 minuti,
// una bozza 60. Un job che nessuno ha eseguito entro quel tempo viene annullato dallo scheduler
// invece di aprire una finestra un'ora dopo su un PC dove l'operatore non c'è più.
func ScadenzaInterattiva(t db.TipoJob) time.Duration {
	if t == db.TipoJobCreaBozzaOutlook {
		return 60 * time.Minute
	}
	return 10 * time.Minute
}

// OpzioniInterattive costruisce i vincoli di un job interattivo: la casella della copia, la
// postazione del richiedente, chi lo ha chiesto, la scadenza. Tutti e quattro insieme: un job
// interattivo senza postazione sarebbe proprio il «prima copia disponibile» che il piano vieta.
func OpzioniInterattive(tipo db.TipoJob, c Copia, postazione uuid.NullUUID, richiedente uuid.UUID) Opzioni {
	scade := time.Now().Add(ScadenzaInterattiva(tipo))
	return Opzioni{
		Casella:     uuid.NullUUID{UUID: c.CasellaID, Valid: true},
		Postazione:  postazione,
		RichiestoDa: uuid.NullUUID{UUID: richiedente, Valid: true},
		ScadeIl:     &scade,
	}
}
