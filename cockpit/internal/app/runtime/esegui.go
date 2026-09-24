// L'avvio del processo: la sequenza, un passo per riga.
//
// main.go e' la riga di comando — i flag, la compilazione di Opzioni, il codice di uscita — e niente
// altro: l'ordine in cui il server nasce e' una cosa del programma, non del suo involucro, e da qui
// si puo' leggere e provare senza passare da un eseguibile.
//
// I passi stanno in avvio.go (log, database, semi, capacita'), comandi.go (i lavori della riga di
// comando), servizi.go (i pezzi di lungo periodo) e ascolto.go (TLS, rotte, listener).

package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
)

// Opzioni sono i lavori amministrativi che si chiedono all'eseguibile dalla riga di comando. Ognuno
// fa il suo, poi esce: nessuno di questi mette il server in ascolto, e nessuno parte da solo
// all'avvio normale. Seminare un'anagrafica è una decisione, non un effetto collaterale (7B.5).
type Opzioni struct {
	SoloMigrazioni   bool
	SemeAnagrafica   string
	SemeFornitori    string
	ApplicaFornitori bool
	ContaAnagrafiche bool
}

// Esegui e' l'avvio: l'ordine in cui il server nasce, e niente altro.
//
// Si legge dall'alto in basso e ogni riga dice un passo. Chi vuole sapere COME si apre il log o COME
// si preparano le fondazioni apre `avvio.go`; chi vuole sapere IN CHE ORDINE succedono le cose, e che
// cosa ferma l'avvio, legge qui.
//
// I quattro lavori della riga di comando escono prima dell'ascolto: fanno il loro e tornano, e il
// server non si mette in ascolto. Sono in `comandi.go`.
//
// `rete` e' la rete dichiarata sulla riga di comando (gli avviatori di scripts/avvio-rete); vuota =
// quella del file.
func Esegui(cfgPath string, rete config.Rete, o Opzioni) error {
	cfg, err := config.CaricaConRete(cfgPath, rete)
	if err != nil {
		return err
	}
	lvl := slog.LevelInfo
	if cfg.Server.LogLivello == "debug" {
		lvl = slog.LevelDebug
	}
	dove, percorsoLog, chiudiLog, avvisoLog := ApriLog(cfg)
	defer chiudiLog()
	log := slog.New(slog.NewTextHandler(dove, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)
	if avvisoLog != "" {
		log.Warn(avvisoLog)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	pool, versione, err := ApriDatabase(ctx, cfg, risorse.FS, log)
	if err != nil {
		return err
	}
	defer pool.Close()
	q := db.New(pool)
	if err := Semina(ctx, q, cfg, versione, log); err != nil {
		return err
	}
	capacita, err := ImpostaCapacita(ctx, q, cfg, log)
	if err != nil {
		return err
	}
	// Blocco 7A (CP8): i messaggi entrati prima della 0014 non hanno una controparte. Si risolvono
	// una volta, qui, con il conteggio per tipo prima/dopo nel log; le proposte non si toccano.
	if n, err := ingest.RicalcolaControparti(ctx, pool, log); err != nil {
		return fmt.Errorf("ricalcolo delle controparti: %w", err)
	} else if n > 0 {
		log.Info("controparti risolte al primo avvio dopo la 0014", "messaggi", n)
	}
	if o.SemeAnagrafica != "" {
		return SeminaAnagrafica(ctx, pool, log, o.SemeAnagrafica)
	}
	if o.SemeFornitori != "" {
		return SemeFornitori(ctx, pool, log, o.SemeFornitori, o.ApplicaFornitori)
	}
	if o.ContaAnagrafiche {
		return ContaAnagrafiche(ctx, pool)
	}
	if o.SoloMigrazioni {
		log.Info("migrazioni e seed completati (-migra): esco senza mettermi in ascolto")
		return nil
	}
	servizi, err := CostruisciServizi(ctx, cfg, pool, q, log, capacita)
	if err != nil {
		return err
	}
	materiale, err := PreparaTLS(cfg, log)
	if err != nil {
		return err
	}
	return Ascolta(ctx, cfg, servizi, materiale, log, percorsoLog, lvl)
}
