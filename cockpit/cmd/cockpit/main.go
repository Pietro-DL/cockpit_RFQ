// cockpit.exe: API JSON per i worker, HTML per il browser, coda job, unico scrittore del NAS.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/fondazioni"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/logfile"
	"promatec/cockpit/internal/migrazioni"
	"promatec/cockpit/internal/nas"
	"promatec/cockpit/internal/rete"
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
	// Il log va sullo stdout E su un file ([server].log_file, di default <staging>\log\cockpit.log).
	// Solo sullo stdout durava quanto la finestra che aveva avviato il server: chiusa quella, di ciò
	// che il server aveva risposto ai worker non restava niente da leggere.
	var dove io.Writer = os.Stdout
	var avvisoLog string
	percorsoLog := cfg.PercorsoLog()
	if percorsoLog != "" {
		f, err := logfile.Apri(percorsoLog, logfile.MaxByteDefault, logfile.CopieDefault)
		if err != nil {
			// un log che non si apre non è un motivo per non partire: si dice e si va avanti
			avvisoLog = fmt.Sprintf("log su file non disponibile (%s): %v", percorsoLog, err)
			percorsoLog = ""
		} else {
			defer f.Close()
			dove = io.MultiWriter(os.Stdout, f)
		}
	}
	log := slog.New(slog.NewTextHandler(dove, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)
	if avvisoLog != "" {
		log.Warn(avvisoLog)
	}

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
	semi, err := fondazioni.Semina(ctx, q, cfg, log)
	if err != nil {
		return err
	}
	// Un worker che non può collegarsi è un avviso d'avvio, non una scoperta del primo 401: il seed ha
	// già scritto riga per riga il perché, qui resta l'indirizzo a cui si sistema. Non è un motivo per
	// non partire — è dalla pagina Postazioni che si genera il pacchetto, e per aprirla il server deve
	// essere acceso.
	if len(semi.CredenzialiDaGenerare) > 0 {
		log.Warn("credenziali da rigenerare: questi worker riceveranno 401 finché non si scarica il pacchetto della loro postazione",
			"worker", strings.Join(semi.CredenzialiDaGenerare, ", "), "dove", "/admin/postazioni")
	}
	// Finché lo schema è alla 0003 il cursore di sincronizzazione è per sola cartella: più di una
	// casella attiva farebbe perdere messaggi in silenzio. Meglio non partire (vedi il commento sulla
	// funzione: il perché è tutto lì).
	applicate, err := migrazioni.Applicate(ctx, pool)
	if err != nil {
		return err
	}
	versione := 0
	for v := range applicate {
		if v > versione {
			versione = v
		}
	}
	if err := fondazioni.UnaSolaCasellaAttiva(ctx, q, versione); err != nil {
		return fmt.Errorf("configurazione delle caselle: %w", err)
	}
	// Modalità (§2.7, voce 9.5). Si fissa PRIMA che scheduler ed esecutore partano: il primo claim
	// arriva pochi millisecondi dopo, e un claim fatto mentre la modalità è ancora quella di default
	// eseguirebbe proprio i job che la shadow deve fermare.
	modalita := jobs.Modalita(cfg.Server.Modalita)
	jobs.ImpostaModalita(modalita)
	if err := jobs.AllineaCoda(ctx, q, modalita, log); err != nil {
		return err
	}
	if modalita == jobs.ModalitaShadow {
		log.Warn("MODALITÀ SHADOW: sola lettura verso il mondo", "bloccati", jobs.TipiBloccatiOra(),
			"nas_dry_run", cfg.NAS.DryRun, "nota", "«Apri in Outlook» resta l'unica azione consentita; si cambia con [server].modalita")
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
	staging, _ := filepath.Abs(cfg.NAS.Staging)
	// Le stesse opzioni per lo scheduler e per «Aggiorna ora»: due sync della stessa casella con
	// cartelle diverse farebbero avanzare il cursore su una finestra che l'altro non ha letto.
	opzioniSync := jobs.SyncOpzioni{Cartelle: cfg.Outlook.Cartelle, Dal: dal, Lotto: cfg.Outlook.Lotto}
	intervalloSync := time.Duration(cfg.Outlook.IntervalloSyncS) * time.Second
	if intervalloSync <= 0 {
		log.Warn("sincronizzazione automatica disattivata: nessun sync viene accodato finché intervallo_sync_s resta 0")
	}
	(&jobs.Scheduler{
		Q: q, Log: log, Cartelle: opzioniSync.Cartelle,
		IntervalloSync: intervalloSync,
		Dal:            opzioniSync.Dal, Lotto: opzioniSync.Lotto,
		RetentionGiorni: cfg.Retention.GiorniJob,
		Staging:         staging,
	}).Avvia(ctx)
	(&jobs.EsecutoreServer{Pool: pool, NAS: scrittore, Log: log}).Avvia(ctx)

	// ---------------------------------------------------------------- rete (voce 2.4)
	//
	// Il materiale TLS si prepara PRIMA di registrare le rotte, perché l'impronta del certificato
	// finisce in due posti che servono a farlo usare: il log dell'avvio e la pagina Postazioni, da cui
	// si scarica il worker.toml che la contiene già. Un'impronta che va copiata a mano da un file di
	// certificato è un'impronta che nessuno verifica.
	var materiale *rete.Materiale
	if cfg.ETLS() {
		nomi := cfg.Server.TLSNomi
		if len(nomi) == 0 {
			nomi = rete.NomiPredefiniti(cfg.Server.Indirizzo)
		}
		materiale, err = rete.Prepara(cfg.Server.TLSCert, cfg.Server.TLSKey, nomi)
		if err != nil {
			return err
		}
		if materiale.Generato {
			log.Warn("certificato TLS generato ora (autofirmato)", "cert", cfg.Server.TLSCert, "chiave", cfg.Server.TLSKey,
				"nomi", materiale.Nomi, "scade", materiale.Scadenza.Format("02/01/2006"),
				"nota", "va copiato nei worker.toml come impronta = ...")
		}
		log.Info("TLS attivo", "impronta", materiale.Impronta, "scade", materiale.Scadenza.Format("02/01/2006"))
	} else if !config.SuLoopback(cfg.Server.Indirizzo) {
		// Ci si arriva solo con consenti_lan_in_chiaro: la configurazione lo rifiuta da sola. Va
		// ricordato a ogni avvio, perché una scelta presa una volta diventa lo stato normale.
		log.Warn("IN CHIARO SULLA LAN: posta, token dei worker e cookie di sessione viaggiano leggibili",
			"indirizzo", cfg.Server.Indirizzo, "nota", "consenti_lan_in_chiaro = true; togliere la riga e indicare tls_cert/tls_key")
	}
	if cfg.Server.TokenWorker != "" {
		log.Warn("[server].token_worker è ancora nel file e non autentica più niente (voce 2.4): togliere la riga",
			"nota", "ogni worker usa il token della sua [[worker]], e il server lo riconosce per sha256")
	}

	templ, _ := fs.Sub(risorse.FS, "web/templates")
	static, _ := fs.Sub(risorse.FS, "web/static")
	servizioIngest := &ingest.Servizio{Pool: pool, Log: log}
	ws := &web.Server{Pool: pool, Log: log, NAS: scrittore, Ingest: servizioIngest, Templ: templ, Static: static,
		IntervalloSync: intervalloSync, Sync: opzioniSync, Modalita: cfg.Server.Modalita,
		TLS: materiale, Indirizzo: cfg.Server.Indirizzo, Workers: risorse.FS}
	if err := ws.Init(); err != nil {
		return err
	}
	wa := &workerapi.Server{
		Pool: pool, Log: log, Ingest: servizioIngest,
		Staging: staging, CasellaDefault: cfg.Outlook.CasellaDefault,
		Analizzatore: jobs.Analizzatore{Versione: cfg.Analisi.Versione, Parametri: cfg.Analisi.Parametri},
		MaxUpload:    int64(cfg.Server.MaxUploadMB) << 20,
	}

	mux := http.NewServeMux()
	ws.Registra(mux)
	wa.Registra(mux)

	// CSRF (voce 2.5): il perché sta su web.ProtezioneCSRF, che è la stessa funzione che i test
	// montano — una protezione che in produzione e nei test passa da due strade diverse è una
	// protezione che una delle due strade prima o poi perde.
	srv := &http.Server{Addr: cfg.Server.Indirizzo, Handler: web.ProtezioneCSRF(logga(log, mux)), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("cockpit in ascolto", "indirizzo", cfg.Schema()+"://"+cfg.Server.Indirizzo, "staging", staging,
		"max_upload_mb", cfg.Server.MaxUploadMB, "log", percorsoLog, "livello", lvl)
	if materiale != nil {
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{materiale.Certificato}, MinVersion: tls.VersionTLS12}
		// I due percorsi sono vuoti di proposito: il certificato è già in TLSConfig, e rileggerlo dal
		// disco qui vorrebbe dire poter servire qualcosa di diverso da ciò di cui abbiamo scritto
		// l'impronta nel log.
		err = srv.ListenAndServeTLS("", "")
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
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
