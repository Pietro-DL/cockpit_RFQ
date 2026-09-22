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
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/ai/agente"
	"promatec/cockpit/internal/app/runtime"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/registro/anagrafica"
	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/logfile"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/rete"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/transport/web"
	"promatec/cockpit/internal/transport/workerapi"
)

// opzioni sono i lavori amministrativi che si chiedono all'eseguibile dalla riga di comando. Ognuno
// fa il suo, poi esce: nessuno di questi mette il server in ascolto, e nessuno parte da solo
// all'avvio normale. Seminare un'anagrafica è una decisione, non un effetto collaterale (7B.5).
type opzioni struct {
	soloMigrazioni   bool
	semeAnagrafica   string
	semeFornitori    string
	applicaFornitori bool
	contaAnagrafiche bool
}

func main() {
	cfgPath := flag.String("config", "cockpit.toml", "percorso di cockpit.toml")
	var o opzioni
	flag.BoolVar(&o.soloMigrazioni, "migra", false, "applica le migrazioni e il seed, poi esce (nessun ascolto HTTP)")
	flag.StringVar(&o.semeAnagrafica, "semina-anagrafica", "", "file JSON di clienti, domini e buyer da seminare; poi esce")
	anteprimaFornitori := flag.String("anteprima-fornitori", "", "file JSON del seme fornitori: dice che cosa scriverebbe e NON scrive; poi esce")
	importaFornitori := flag.String("importa-fornitori", "", "file JSON del seme fornitori: lo applica davvero; poi esce")
	flag.BoolVar(&o.contaAnagrafiche, "conta-anagrafiche", false, "stampa quante righe ci sono in anagrafica (clienti, buyer, fornitori...); poi esce")
	flag.Parse()
	o.semeFornitori, o.applicaFornitori = *anteprimaFornitori, false
	if *importaFornitori != "" {
		if o.semeFornitori != "" {
			fmt.Fprintln(os.Stderr, "errore: -anteprima-fornitori e -importa-fornitori insieme non hanno senso: prima si guarda, poi si scrive")
			os.Exit(1)
		}
		o.semeFornitori, o.applicaFornitori = *importaFornitori, true
	}
	if err := run(*cfgPath, o); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, o opzioni) error {
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
	if err := fondazioni.SeedUtenti(ctx, q, utenti, log); err != nil {
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
	// CAPACITÀ DI SCRITTURA (blocco 4). Si fissano PRIMA che scheduler ed esecutore partano: il primo
	// claim arriva pochi millisecondi dopo, e un claim fatto mentre le capacità sono ancora quelle di
	// default eseguirebbe proprio i job che la configurazione vuole fermi.
	sic := cfg.Capacita()
	capacita := coda.Capacita{OutlookScrittura: sic.OutlookScrittura, Bozze: sic.Bozze, NasScrittura: sic.NasScrittura}
	coda.ImpostaCapacita(capacita)
	if err := coda.AllineaCoda(ctx, q, capacita, log); err != nil {
		return err
	}
	// Una riga per capacità, con scritto ATTIVA o SPENTA. Un elenco solo non basta: chi legge il log
	// per capire perché una copia non parte cerca il nome di quella capacità, non un riassunto.
	for _, nome := range coda.TutteLeCapacita {
		stato := "SPENTA"
		if capacita.Ha(nome) {
			stato = "ATTIVA"
		}
		log.Warn("capacità di scrittura", "capacita", nome, "stato", stato)
	}
	if capacita.TuttoSpento() {
		log.Warn("questo server NON modifica niente fuori da se'", "modalita", cfg.Server.Modalita,
			"bloccati", coda.TipiBloccatiOra(),
			"nota", "sincronizzazione, download in staging, analisi e «Apri in Outlook» restano consentiti")
	}
	for _, a := range sic.Avvisi {
		log.Warn("sicurezza", "avviso", a)
	}
	// Blocco 7A (CP8): i messaggi entrati prima della 0014 non hanno una controparte. Si risolvono
	// una volta, qui, con il conteggio per tipo prima/dopo nel log; le proposte non si toccano.
	if n, err := ingest.RicalcolaControparti(ctx, pool, log); err != nil {
		return fmt.Errorf("ricalcolo delle controparti: %w", err)
	} else if n > 0 {
		log.Info("controparti risolte al primo avvio dopo la 0014", "messaggi", n)
	}
	// Il seme dell'anagrafica (blocco 3). Si legge e si CONVALIDA prima di scrivere: se una sola
	// regola di un solo cliente ha un esempio che non corrisponde alla propria regex, non parte
	// niente. Un cliente gia' presente viene saltato per intero — il file e' una fotografia di un
	// foglio, il database e' dove qualcuno ha gia' corretto a mano cio' che il foglio sbagliava.
	if o.semeAnagrafica != "" {
		seme, err := anagrafica.LeggiFile(o.semeAnagrafica)
		if err != nil {
			return err
		}
		esito, err := anagrafica.Semina(ctx, pool, seme)
		if err != nil {
			return err
		}
		log.Info("anagrafica seminata", "creati", len(esito.ClientiCreati), "gia_presenti", len(esito.ClientiPresenti),
			"domini", esito.DominiAggiunti, "buyer", esito.BuyerCreati)
		for _, c := range esito.ClientiCreati {
			log.Info("cliente creato", "cliente", c)
		}
		for _, c := range esito.ClientiPresenti {
			log.Info("cliente gia' presente: lasciato com'era", "cliente", c)
		}
		for _, a := range esito.Avvisi {
			log.Warn("seme anagrafica", "avviso", a)
		}
		if err := ricalcola(ctx, pool, log, esito.IndirizziScritti, esito.DominiScritti); err != nil {
			return err
		}
		return nil
	}

	// Il seme dei fornitori (7A.4) dalla riga di comando: LO STESSO motore della schermata
	// Admin > Anagrafica > Fornitori > Importa, cioe' `fornitori.Leggi`, `Calcola`, `Applica`. Non
	// c'e' un secondo lettore del file e non c'e' un secondo importatore: PowerShell orchestra, Go
	// convalida e scrive. Senza `-importa-fornitori` non viene scritta nemmeno una riga.
	if o.semeFornitori != "" {
		seme, err := fornitori.LeggiFile(o.semeFornitori)
		if err != nil {
			return err
		}
		var ant fornitori.Anteprima
		if o.applicaFornitori {
			ant, err = fornitori.Applica(ctx, pool, seme)
		} else {
			ant, err = fornitori.Calcola(ctx, db.New(pool), seme)
		}
		if err != nil {
			return err
		}
		verbo := "da creare"
		if o.applicaFornitori {
			verbo = "creati"
		}
		log.Info("seme fornitori: "+verbo, "fornitori", len(ant.FornitoriDaCreare), "gia_presenti", len(ant.FornitoriPresenti),
			"righe", len(ant.DaAggiungere), "gia_in_database", len(ant.Presenti), "non_risolti", len(ant.NonRisolti))
		for _, f := range ant.FornitoriDaCreare {
			log.Info("fornitore "+verbo, "fornitore", f)
		}
		for _, r := range ant.NonRisolti {
			log.Warn("seme fornitori: NON risolto, non viene scritto", "riga", r.String())
		}
		for _, r := range ant.Avvisi {
			log.Warn("seme fornitori", "avviso", r.String())
		}
		if !o.applicaFornitori {
			fmt.Println("anteprima: non e' stato scritto niente. Per applicare: -importa-fornitori " + o.semeFornitori)
			return nil
		}
		if err := ricalcola(ctx, pool, log, ant.IndirizziScritti, ant.DominiScritti); err != nil {
			return err
		}
		return nil
	}

	if o.contaAnagrafiche {
		c, err := db.New(pool).ContaAnagrafiche(ctx)
		if err != nil {
			return err
		}
		// Una riga per voce, `chiave=valore`: lo script di bootstrap le mostra e basta, senza dover
		// sapere com'e' fatto lo schema.
		for _, v := range []struct {
			nome string
			n    int32
		}{
			{"clienti", c.Clienti}, {"domini_cliente", c.DominiCliente}, {"buyer", c.Buyer},
			{"fornitori", c.Fornitori}, {"domini_fornitore", c.DominiFornitore},
			{"contatti_fornitore", c.ContattiFornitore}, {"lavorazioni_fornitore", c.LavorazioniFornitore},
			{"qualifiche", c.Qualifiche},
		} {
			fmt.Printf("%s=%d\n", v.nome, v.n)
		}
		return nil
	}

	if o.soloMigrazioni {
		log.Info("migrazioni e seed completati (-migra): esco senza mettermi in ascolto")
		return nil
	}

	// DryRun non viene piu' dal file: e' l'altra faccia di [sicurezza].nas_scrittura. Due voci per la
	// stessa decisione sono due voci che prima o poi si contraddicono, e la piu' silenziosa vince.
	scrittore := &nas.Scrittore{Radice: cfg.NAS.Radice, DryRun: !capacita.NasScrittura}
	if scrittore.Raggiungibile() {
		log.Info("NAS raggiungibile", "radice", cfg.NAS.Radice, "scrittura", capacita.NasScrittura)
	} else {
		log.Warn("NAS non raggiungibile: le copie resteranno in coda", "radice", cfg.NAS.Radice)
	}

	// `dal` è un override e basta: senza, il limite inferiore lo decide chi accoda il sync (il
	// cursore della cartella, altrimenti la finestra iniziale) e lo decide AL MOMENTO. Calcolarlo qui
	// una volta sola voleva dire che un server acceso da un mese apriva ancora la finestra di un mese
	// fa. La data è già stata validata da config.Carica: qui non può più fallire.
	var dal time.Time
	if cfg.Outlook.Dal != "" {
		dal, _ = config.DataDal(cfg.Outlook.Dal)
		log.Warn("finestra iniziale forzata da [outlook].dal: vale solo per le cartelle senza cursore", "dal", cfg.Outlook.Dal)
	}
	radiceStaging, _ := filepath.Abs(cfg.NAS.Staging)
	// Le stesse opzioni per lo scheduler e per «Aggiorna ora»: due sync della stessa casella con
	// cartelle diverse farebbero avanzare il cursore su una finestra che l'altro non ha letto.
	opzioniSync := coda.SyncOpzioni{Cartelle: cfg.Outlook.Cartelle, Dal: dal,
		GiorniIniziali: cfg.Outlook.GiorniSyncIniziale, Lotto: cfg.Outlook.Lotto}
	intervalloSync := time.Duration(cfg.Outlook.IntervalloSyncS) * time.Second
	// Assente = acceso. Il sync di apertura non è il sync periodico: `intervallo_sync_s = 0` dice che
	// il server non accoda niente DA SOLO, non che debba ignorare chi sta aprendo l'Inbox adesso.
	syncApertura := cfg.Outlook.SyncAperturaInbox == nil || *cfg.Outlook.SyncAperturaInbox
	if intervalloSync <= 0 {
		log.Warn("sincronizzazione periodica disattivata: nessun sync viene accodato a tempo finché intervallo_sync_s resta 0",
			"sync_apertura_inbox", syncApertura)
	}
	(&coda.Scheduler{
		Q: q, Log: log, Cartelle: opzioniSync.Cartelle,
		IntervalloSync: intervalloSync,
		Dal:            opzioniSync.Dal, GiorniIniziali: opzioniSync.GiorniIniziali, Lotto: opzioniSync.Lotto,
		RetentionGiorni: cfg.Retention.GiorniJob,
		Staging:         radiceStaging,
	}).Avvia(ctx)
	// La cache dei contenuti (Pre-7, D31): `_contenuti` resta dopo la copia sul NAS, e si svuota per
	// eta' o per capienza, mai sotto a chi la sta usando.
	if cfg.Retention.GiorniStaging > 0 && cfg.Retention.CacheGiorni == nil {
		log.Warn("[retention].giorni_staging e' deprecata: vale come cache_gg", "giorni", cfg.Retention.GiorniStaging,
			"nota", "scrivere cache_gg = "+strconv.Itoa(cfg.Retention.GiorniStaging)+" e togliere la riga")
	}
	(&staging.Cache{Pool: pool, Staging: radiceStaging, Log: log, Retention: cfg.RetentionCache(), MaxByte: cfg.CacheMaxByte()}).Avvia(ctx)
	// L'analisi semantica (checkpoint 3R §9). Spenta se non la si accende in [agente], e comunque
	// spenta se manca la chiave: `DaAmbiente` restituisce nil, e un servizio senza modello non chiama
	// nessuno. Il testo delle mail dei clienti esce verso un servizio esterno, e quella e' una cosa
	// che si mette per iscritto prima.
	servizioAgente := &agente.Servizio{
		Attivo:  cfg.Agente.Attivo,
		Modello: agente.DaAmbiente(cfg.Agente.URL, cfg.Agente.Modello, cfg.Agente.ChiaveEnv),
		Caselle: map[string]bool{},
	}
	for _, c := range cfg.Agente.Caselle {
		servizioAgente.Caselle[strings.ToLower(strings.TrimSpace(c))] = true
	}
	if cfg.Agente.Attivo && servizioAgente.Modello == nil {
		log.Warn("analisi semantica richiesta ma spenta: manca la chiave", "variabile", cfg.Agente.ChiaveEnv)
	}
	if servizioAgente.Attivo && servizioAgente.Modello != nil {
		log.Info("analisi semantica attiva", "modello", servizioAgente.Modello.Nome(), "caselle", len(servizioAgente.Caselle))
	}
	esecutore := &runtime.EsecutoreServer{Pool: pool, NAS: scrittore, Log: log, Agente: servizioAgente}
	ricognitore := &documenti.Ricognitore{Pool: pool, NAS: scrittore, Log: log, Ogni: cfg.IntervalloIntegrita()}

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
		// L'host di url_pubblico entra nei nomi del certificato (7C.1, P1): e' quello che il browser
		// digita, e un certificato che non lo nomina da' un avviso in piu' da ignorare ogni volta.
		if h := cfg.HostPubblico(); h != "" {
			nomi = append(nomi, h)
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
		if h := cfg.HostPubblico(); h != "" && !materiale.Copre(h) {
			log.Warn("il certificato non nomina l'host di [server].url_pubblico: il browser dira' che il nome non corrisponde",
				"host", h, "nomi_del_certificato", materiale.Nomi,
				"rimedio", "aggiungerlo a [server].tls_nomi e cancellare cert/key per rigenerarli (poi rigenerare i pacchetti: cambia l'impronta)")
		}
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
	// D30: lo staging automatico si accende dal file di configurazione ([staging] automatico = true) e
	// non dal binario. Acceso, gli allegati di un mittente riconosciuto scendono da soli e l'analisi
	// puo' dire che cosa sono; spento, si comporta come prima del checkpoint 3R.
	servizioIngest := &ingest.Servizio{Pool: pool, Log: log,
		StagingAutomatico: cfg.Staging.Automatico,
		StagingBootstrap:  cfg.Staging.Bootstrap,
		StagingMaxByte:    int64(cfg.Staging.MaxMB) * 1024 * 1024}
	ws := &web.Server{Pool: pool, Log: log, NAS: scrittore, Ingest: servizioIngest, Templ: templ, Static: static,
		IntervalloSync: intervalloSync, Sync: opzioniSync, SyncAperturaInbox: syncApertura, Modalita: cfg.Server.Modalita,
		TLS: materiale, Indirizzo: cfg.Server.Indirizzo, URLPubblico: cfg.Server.URLPubblico, Workers: risorse.FS, Agente: servizioAgente,
		Ricognitore: ricognitore}
	if err := ws.Init(); err != nil {
		return err
	}
	if cfg.Server.URLPubblico == "" {
		log.Info("[server].url_pubblico non dichiarato: i pacchetti delle postazioni useranno l'indirizzo derivato dal bind",
			"url", ws.URLServer(), "nota", "sulla LAN conviene dichiarare un IPv4 stabile o un FQDN che si risolve da ogni PC")
	} else {
		log.Info("indirizzo pubblico del server", "url", ws.URLServer())
	}
	wa := &workerapi.Server{
		Pool: pool, Log: log, Ingest: servizioIngest,
		Staging: radiceStaging, CasellaDefault: cfg.Outlook.CasellaDefault,
		Analizzatore: coda.Analizzatore{Versione: cfg.Analisi.Versione, Parametri: cfg.Analisi.Parametri},
		MaxUpload:    int64(cfg.Server.MaxUploadMB) << 20,
	}
	// L'esecutore interno parte QUI e non prima, perche' gli serve chi sa scompattare un archivio, e
	// quel qualcuno e' la stessa parte che riceve i risultati dei worker: dal blocco 4A l'estrazione
	// di uno zip e' un job, non un pezzo della richiesta HTTP con cui il download viene consegnato.
	esecutore.Archivi = wa
	esecutore.Avvia(ctx)
	// Il ricognitore dell'integrita' (blocco 5B). Parte SEMPRE, anche con la scrittura spenta: legge
	// il NAS e basta, e un server che non scrive puo' benissimo accorgersi che un file dichiarato nel
	// fascicolo non c'e' piu'. Si spegne solo con [nas].intervallo_integrita_s = 0.
	ricognitore.Avvia(ctx)

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
	log.Info("cockpit in ascolto", "indirizzo", cfg.Schema()+"://"+cfg.Server.Indirizzo, "staging", radiceStaging,
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

// ricalcola riguarda i messaggi gia' arrivati dopo un seed: quelli che parlano con i domini e gli
// indirizzi appena scritti, e che nessuno ha ancora deciso (7B.5).
//
// Senza questo passo, seminare l'anagrafica su un database che ha gia' dentro la posta non sposta
// di un messaggio il quadrante Da validare: la controparte e' un FATTO scritto sul messaggio
// all'ingest (0014), non una domanda che l'Inbox rifa' a ogni lettura. Riavviare il server non
// basterebbe: il ricalcolo dell'avvio guarda solo i messaggi che una controparte non ce l'hanno.
func ricalcola(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, indirizzi, domini []string) error {
	if len(indirizzi) == 0 && len(domini) == 0 {
		log.Info("niente da riguardare: il seme non ha scritto nessun dominio e nessun indirizzo")
		return nil
	}
	esito, err := (&ingest.Servizio{Pool: pool, Log: log}).RitriageMolti(ctx, indirizzi, domini)
	if err != nil {
		return fmt.Errorf("ricalcolo dei messaggi gia' arrivati: %w", err)
	}
	log.Info("messaggi gia' arrivati riguardati", "esito", esito.String())
	for _, d := range esito.Dettagli {
		log.Info("ricalcolo", "cambiato", d)
	}
	return nil
}
