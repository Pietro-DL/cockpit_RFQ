// cockpit.exe: API JSON per i worker, HTML per il browser, coda job, unico scrittore del NAS.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/fondazioni"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/migrazioni"
	"promatec/cockpit/internal/nas"
	"promatec/cockpit/internal/web"
	"promatec/cockpit/internal/workerapi"
)

func main() {
	cfgPath := flag.String("config", "cockpit.toml", "percorso di cockpit.toml")
	soloMigrazioni := flag.Bool("migra", false, "applica le migrazioni e il seed, poi esce (nessun ascolto HTTP)")
	flag.Parse()
	if err := run(*cfgPath, *soloMigrazioni); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, soloMigrazioni bool) error {
	cfg, err := config.Carica(cfgPath)
	if err != nil {
		return err
	}
	lvl := slog.LevelInfo
	if cfg.Server.LogLivello == "debug" {
		lvl = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DB.DSN)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	if _, err := migrazioni.Applica(ctx, pool, risorse.FS, log); err != nil {
		return err
	}
	q := db.New(pool)
	utenti := make([]struct{ Sigla, Nome, Ufficio, Ruolo, Password string }, 0, len(cfg.Utenti))
	for _, u := range cfg.Utenti {
		utenti = append(utenti, struct{ Sigla, Nome, Ufficio, Ruolo, Password string }{u.Sigla, u.Nome, u.Ufficio, u.Ruolo, u.Password})
	}
	if err := web.SeedUtenti(ctx, q, utenti); err != nil {
		return err
	}
	if _, err := fondazioni.Semina(ctx, q, cfg, log); err != nil {
		return err
	}
	if soloMigrazioni {
		log.Info("migrazioni e seed completati (-migra): esco senza mettermi in ascolto")
		return nil
	}

	scrittore := &nas.Scrittore{Radice: cfg.NAS.Radice, DryRun: cfg.NAS.DryRun}
	if scrittore.Raggiungibile() {
		log.Info("NAS raggiungibile", "radice", cfg.NAS.Radice, "dry_run", cfg.NAS.DryRun)
	} else {
		log.Warn("NAS non raggiungibile: le copie resteranno in coda", "radice", cfg.NAS.Radice)
	}

	dal := time.Now().AddDate(0, -1, 0)
	if cfg.Outlook.Dal != "" {
		if d, err := time.ParseInLocation("2006-01-02", cfg.Outlook.Dal, time.Local); err == nil {
			dal = d
		}
	}
	(&jobs.Scheduler{
		Q: q, Log: log, Cartelle: cfg.Outlook.Cartelle,
		IntervalloSync: time.Duration(cfg.Outlook.IntervalloSyncS) * time.Second,
		Dal:            dal, Lotto: cfg.Outlook.Lotto,
		CasellaDefault: cfg.Outlook.CasellaDefault, RetentionGiorni: cfg.Retention.GiorniJob,
	}).Avvia(ctx)
	(&jobs.EsecutoreServer{Pool: pool, NAS: scrittore, Log: log}).Avvia(ctx)

	templ, _ := fs.Sub(risorse.FS, "web/templates")
	static, _ := fs.Sub(risorse.FS, "web/static")
	servizioIngest := &ingest.Servizio{Pool: pool, Log: log}
	ws := &web.Server{Pool: pool, Log: log, NAS: scrittore, Ingest: servizioIngest, Templ: templ, Static: static}
	if err := ws.Init(); err != nil {
		return err
	}
	staging, _ := filepath.Abs(cfg.NAS.Staging)
	wa := &workerapi.Server{
		Pool: pool, Log: log, Token: cfg.Server.TokenWorker, Ingest: servizioIngest,
		Staging: staging, CasellaDefault: cfg.Outlook.CasellaDefault,
	}

	mux := http.NewServeMux()
	ws.Registra(mux)
	wa.Registra(mux)

	srv := &http.Server{Addr: cfg.Server.Indirizzo, Handler: logga(log, mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("cockpit in ascolto", "indirizzo", "http://"+cfg.Server.Indirizzo, "staging", staging)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func logga(log *slog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		h.ServeHTTP(w, r)
		if r.URL.Path != "/api/v1/jobs/claim" && r.URL.Path != "/healthz" {
			log.Debug("http", "m", r.Method, "p", r.URL.Path, "ms", time.Since(t).Milliseconds())
		}
	})
}
