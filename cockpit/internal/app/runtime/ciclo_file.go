package runtime

// ciclo_file.go: il servizio del ciclo dei file della Sorgente (V1, EM §10; corsia file) e il custode che riprende le
// rimozioni e i rilasci rimasti a meta'.
//
// Il deposito locale degli «Altri file» delle prove dell'ondata B ([conservazione], la cartella con il segnaposto
// preparata all'avvio) non c'e' piu' (U2 §3.13; [U] Q06 e C-02 B): gli Altri file stanno nella cartella ALTRO della
// RFQ sul NAS, scritta solo da storage/nas. L'avvio non crea cartelle ne' segnaposto per loro, e una sezione
// [conservazione] rimasta nel file ferma gia' il caricamento della configurazione (platform/config).
//
// Il posto delle copie e' nas.Conservatore sulla radice del NAS della configurazione ([nas].radice, la stessa dei
// documenti), con DryRun quando la capacita' nas_scrittura e' spenta all'avvio e con la capacita' riletta a ogni
// gesto (U2 §3.5): con la scrittura spenta «Metti in Altri file» risponde 503 NON_CONFIGURATO / nas_scrittura e non
// scrive niente, e una copia scartata resta da togliere finche' qualcuno non usa «Riprova la rimozione dalla cartella
// ALTRO». Le protezioni della configurazione restano quelle dei documenti (shadow, radici di produzione,
// consenti_nas_produzione). Senza [nas].radice nessun posto: 503 NON_CONFIGURATO / conservazione.

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/storage/nas"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// preparaCicloFile costruisce il servizio del ciclo dei file, con il posto delle copie degli Altri file (la cartella
// ALTRO della RFQ sotto [nas].radice). L'avvio non scrive niente sul disco.
func preparaCicloFile(_ context.Context, cfg *config.Config, pool *pgxpool.Pool, radiceStaging string, capacita coda.Capacita,
	log *slog.Logger) (*filerfq.Servizio, error) {
	svc := &filerfq.Servizio{Pool: pool, Staging: radiceStaging, NASRadice: cfg.NAS.Radice, Log: log}
	if strings.TrimSpace(cfg.NAS.Radice) == "" {
		log.Info("Altri file: [nas].radice non c'e': «Metti in Altri file» risponde 503 NON_CONFIGURATO")
		return svc, nil
	}
	svc.Conservatore = conservatoreNAS(cfg.NAS.Radice, capacita)
	if ok, motivo := svc.Conservatore.Disponibile(); !ok {
		log.Info("Altri file: la scrittura sul NAS e' spenta: «Metti in Altri file» risponde 503 NON_CONFIGURATO", "motivo", motivo)
	}
	return svc, nil
}

// conservatoreNAS: lo scrittore delle copie degli Altri file sulla radice del NAS (U2 §3.13: l'unica implementazione di
// produzione), con il DryRun dell'avvio e la capacita' nas_scrittura riletta a ogni gesto (U2 §3.5).
func conservatoreNAS(radice string, capacita coda.Capacita) *nas.Conservatore {
	return &nas.Conservatore{Radice: radice, DryRun: !capacita.NasScrittura,
		Scrittura: func() bool { return coda.CapacitaAttuali().NasScrittura }}
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
