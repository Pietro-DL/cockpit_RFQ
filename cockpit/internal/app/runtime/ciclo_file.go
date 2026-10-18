package runtime

// ciclo_file.go: il servizio del ciclo dei file della Sorgente (V1, EM §10; corsia file, ondata B): il deposito degli
// «Altri file» ([conservazione]), il servizio che usano le rotte della Sorgente e il custode che riprende le rimozioni e
// i rilasci rimasti a meta'.

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// preparaCicloFile apre il deposito degli «Altri file» e costruisce il servizio. Un deposito che non torna con le copie
// registrate (segnaposto assente o di un'altra radice, EM §10.5) ferma l'avvio: le copie registrate non sarebbero in
// quella cartella. Senza [conservazione] il servizio c'e' lo stesso, senza deposito: «Metti in Altri file» risponde 503.
//
// Con il flag [motore_a] spento il deposito non si prepara, anche con la radice scritta: le rotte della Sorgente non
// esistono, e l'avvio non crea cartelle ne' segnaposto sul disco del server e non si ferma per una radice sbagliata
// (EM §10.5: «nessuna scrittura senza il suo OK»; rilievo della revisione). Il custode e la cache vedono un deposito non
// configurato: i rilasci aspettano, e la cache tiene lo staging dei file in Altri file (il lato che non cancella).
func preparaCicloFile(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, radiceStaging string,
	log *slog.Logger) (*filerfq.Servizio, error) {
	radice := cfg.Conservazione.Radice
	if !cfg.MotoreA.Anteprima && strings.TrimSpace(radice) != "" {
		log.Info("deposito degli Altri file non preparato: [motore_a] e' spento, la Sorgente non c'e'", "voce", "[conservazione].radice")
		radice = ""
	}
	dep, err := filerfq.PreparaDeposito(ctx, db.New(pool), radice)
	if err != nil {
		return nil, fmt.Errorf("[conservazione]: %w", err)
	}
	if dep != nil {
		log.Info("deposito degli Altri file pronto", "radice", dep.Radice, "deposito", dep.ID)
	} else if cfg.MotoreA.Anteprima {
		log.Info("deposito degli Altri file non configurato: «Metti in Altri file» risponde 503 NON_CONFIGURATO", "voce", "[conservazione].radice")
	}
	return &filerfq.Servizio{Pool: pool, Staging: radiceStaging, Deposito: dep, NASRadice: cfg.NAS.Radice, Log: log}, nil
}

// contestiPerIlCicloFile: i contesti partner dichiarati per una RFQ, ridotti all'id e al perimetro dei file, per la
// dipendenza D-13 (EM §10.3, §10.7: il ciclo dei file non modifica i contesti, la Sorgente mostra chi cita un file).
func contestiPerIlCicloFile(r motorea_web.Regole) func(uuid.UUID) []filerfq.Contesto {
	if !r.Pronta() || r.Insieme == nil || len(r.Insieme.Contesti) == 0 {
		return nil
	}
	tutti := r.Insieme.Contesti
	return func(thread uuid.UUID) []filerfq.Contesto {
		var out []filerfq.Contesto
		for _, c := range tutti {
			if !strings.EqualFold(c.RFQ, thread.String()) {
				continue
			}
			x := filerfq.Contesto{ID: c.ID}
			for _, p := range c.Perimetro {
				x.Perimetro = append(x.Perimetro, filerfq.OccorrenzaContesto{AllegatoID: p.AllegatoID, Sha256: p.Sha256,
					Nome: p.Nome, Contenitore: p.Contenitore})
			}
			out = append(out, x)
		}
		return out
	}
}
