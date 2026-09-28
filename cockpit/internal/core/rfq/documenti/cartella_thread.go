// La cartella di una RFQ sul NAS: quali sottocartelle nascono subito e quali alla prima copia.
//
// Fino a B6b stava dentro lo switch dell'esecutore, che e' il posto dove si decide CHI esegue, non
// COSA. Quali sottocartelle esistono da subito e' una convenzione del fascicolo.

package documenti

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// CreaCartellaThread crea sul NAS la cartella di una RFQ con le sottocartelle della convenzione.
func CreaCartellaThread(ctx context.Context, q *db.Queries, scrittore *nas.Scrittore, threadID uuid.UUID) (any, error) {
	t, err := q.GetThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if !t.CartellaRelativa.Valid {
		return nil, fmt.Errorf("thread %s senza cartella_relativa", threadID)
	}
	layout, err := q.ListCartellaDocumento(ctx)
	if err != nil {
		return nil, err
	}
	// solo le sottocartelle della convenzione (ELENCO DISEGNI, OFFERTE FORNITORI); le altre nascono alla prima copia
	var sotto []string
	for _, l := range layout {
		if l.CreaSempre {
			sotto = append(sotto, l.Sottocartella)
		}
	}
	base, err := scrittore.CreaCartella(t.CartellaRelativa.String, sotto)
	if err != nil {
		return nil, err
	}
	if !scrittore.DryRun {
		if err := q.SetCartellaCreata(ctx, threadID); err != nil {
			return nil, err
		}
	}
	return map[string]any{"cartella": base, "dry_run": scrittore.DryRun}, nil
}
