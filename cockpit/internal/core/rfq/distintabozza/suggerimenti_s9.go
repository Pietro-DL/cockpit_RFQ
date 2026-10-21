package distintabozza

// I suggerimenti dopo l'apertura della bozza aperta di un prodotto per S9 (direttiva 39 §4, D-PB01 A, D-PB02 A, D-PB03 A
// [U]; EM I.8). Aggancio della corsia motore in un file proprio [T-39, corsia motore, rilievo della revisione]: S9 e R1
// hanno una sola sorgente e una sola forma di preassociazioni_proposte[], quelle di R1 (suggerimentiDopoApertura: le
// proposte univoche del registro della versione, con la regola delle PB iniziali ClassificaProposte sui dati salvati di
// adesso, con chiave_bozza e importazione sempre). Cosi' un suggerimento di S9 accettato porta il registro che lo propone
// e R2 lo riconosce «motore» (provenienzeDelServer), e il criterio di «univoca» e' lo stesso alla nascita e dopo.
//
// Solo SELECT, sulla transazione di chi chiama (la stessa fotografia REPEATABLE READ READ ONLY del caricatore del motore).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// SuggerimentiDellaBozzaAperta da' i suggerimenti dopo l'apertura della bozza aperta formato 3 del prodotto, gli stessi di
// R1 (LeggiR1, PreassociazioniProposte): vuoti senza bozza aperta, per una bozza formato 2, per una versione senza
// registro (vuota, manuale, solo_prodotto, da-conferma-1) e senza la sorgente confermata da S8 (D-AR11 B [U]). Uno
// snapshot o un registro che non si rileggono sono ErrSnapshotIncoerente, come in R1.
func SuggerimentiDellaBozzaAperta(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID) ([]PreassociazioneProposta3, error) {
	out := []PreassociazioneProposta3{}
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return nil, err
	}
	letto, err := RileggiVersione(int(b.Formato), v.Snapshot, v.Impronta)
	if err != nil {
		return nil, err
	}
	if letto.Formato3 == nil {
		return out, nil
	}
	s := *letto.Formato3
	if s.Sorgente == nil || s.Sorgente.Importazione == "" {
		return out, nil
	}
	rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: thread, ProdottoID: prodotto,
		Impronte: []string{s.Sorgente.Importazione}})
	if err != nil {
		return nil, err
	}
	for _, r := range rr {
		if strings.TrimSpace(r.Impronta) == s.Sorgente.Importazione {
			return suggerimentiDopoApertura(ctx, q, thread, prodotto, s, rigaRegistro(r), b.BozzaID, int(b.Versione))
		}
	}
	return nil, fmt.Errorf("%w: la bozza %s cita il registro %s che non c'e'", ErrSnapshotIncoerente, b.BozzaID, s.Sorgente.Importazione)
}
