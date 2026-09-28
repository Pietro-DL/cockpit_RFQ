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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

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
	// Calibrazione stampa le misure dello Smistamento (M3, A5.14.6): quante volte il primo proposto era
	// quello giusto. CalibrazioneDal limita alle decisioni da quel giorno; nil = tutte.
	Calibrazione    bool
	CalibrazioneDal *time.Time
	// RiapriAgganci e' il comando U5 dello Smistamento (F7, A5.15): riapre, nelle RFQ in corso, gli agganci
	// automatici fatti prima dello Smistamento per sola uguaglianza di codice. Senza DatabaseRiapertura e'
	// l'anteprima (-anteprima-riapri-agganci: sola lettura); con, e' l'applicazione (-riapri-agganci <nome>),
	// e DatabaseRiapertura e' il nome del database che chi lancia vuole scrivere: deve essere quello del file
	// e quello a cui ci si collega (P38). RfqRiapertura limita il comando a una RFQ (nil = tutte);
	// UscitaRiapertura e' il rapporto JSON (vuoto = nella cartella del log).
	RiapriAgganci      bool
	DatabaseRiapertura string
	RfqRiapertura      *uuid.UUID
	UscitaRiapertura   string
}

// PreparaRiapertura legge le opzioni del comando U5 dalla riga di comando: anteprima e' -anteprima-riapri-agganci,
// applica dice che -riapri-agganci c'era (con il nome del database in database), rfq e uscita sono -rfq e
// -uscita. Le due forme insieme sono un errore (prima si guarda, poi si scrive), come per i fornitori;
// -riapri-agganci vuole il nome del database; -rfq e -uscita da soli non vogliono dire niente.
func (o *Opzioni) PreparaRiapertura(anteprima, applica bool, database, rfq, uscita string) error {
	database, rfq = strings.TrimSpace(database), strings.TrimSpace(rfq)
	switch {
	case anteprima && applica:
		return errors.New("-anteprima-riapri-agganci e -riapri-agganci insieme non hanno senso: prima si guarda, poi si scrive")
	case applica && database == "":
		return errors.New("-riapri-agganci vuole il nome del database da scrivere (quello del DSN del -config): -riapri-agganci <nome>")
	case !anteprima && !applica:
		if rfq != "" || strings.TrimSpace(uscita) != "" {
			return errors.New("-rfq e -uscita valgono solo con -anteprima-riapri-agganci o -riapri-agganci")
		}
		return nil
	}
	o.RiapriAgganci, o.DatabaseRiapertura, o.UscitaRiapertura = true, database, strings.TrimSpace(uscita)
	if rfq != "" {
		id, err := uuid.Parse(rfq)
		if err != nil {
			return fmt.Errorf("-rfq %q: serve l'identificativo della RFQ (un uuid)", rfq)
		}
		o.RfqRiapertura = &id
	}
	return nil
}

// SoloLettura dice se il lavoro chiesto legge soltanto: -conta-anagrafiche, -anteprima-fornitori e
// -calibrazione. Questi non migrano, non seminano e non toccano la coda (ApriDatabaseInLettura): un
// «conta» lanciato con un binario nuovo su un database vecchio applicava le migrazioni, cioe' faceva
// proprio la cosa che si fa solo dopo un backup. Legge soltanto anche -anteprima-riapri-agganci (il comando
// U5 senza il nome del database), che pero' ha la sua strada in Esegui: RiapriAgganci.
func (o Opzioni) SoloLettura() bool {
	return o.ContaAnagrafiche || o.Calibrazione || (o.SemeFornitori != "" && !o.ApplicaFornitori) ||
		(o.RiapriAgganci && o.DatabaseRiapertura == "")
}

// Esegui e' l'avvio: l'ordine in cui il server nasce, e niente altro.
//
// Si legge dall'alto in basso e ogni riga dice un passo. Chi vuole sapere COME si apre il log o COME
// si preparano le fondazioni apre `avvio.go`; chi vuole sapere IN CHE ORDINE succedono le cose, e che
// cosa ferma l'avvio, legge qui.
//
// I lavori della riga di comando escono prima dell'ascolto: fanno il loro e tornano, e il server non
// si mette in ascolto. Sono in `comandi.go`. Quelli che leggono soltanto (Opzioni.SoloLettura) escono
// prima ancora di migrare.
//
// Un errore dopo l'apertura del log si scrive anche nel log, non solo sullo stderr che main stampa: un
// servizio o un'attivita' pianificata una finestra non ce l'hanno, e il perche' di un avvio fallito
// restava solo nella memoria di chi l'aveva visto.
//
// `rete` e' la rete dichiarata sulla riga di comando (gli avviatori di scripts/avvio-rete); vuota =
// quella del file.
func Esegui(cfgPath string, rete config.Rete, o Opzioni) (err error) {
	cfg, err := config.CaricaConRete(cfgPath, rete)
	if err != nil {
		return err
	}
	if o.RiapriAgganci {
		// Il comando U5 (Smistamento F7, P38): la prima riga stampata dice su quale database si lavora, prima
		// di aprire il log o il database. Non semina, non migra, non tocca la coda, e non si mette in ascolto.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return RiapriAgganci(ctx, cfg, cfgPath, os.Stdout, o)
	}
	lvl, lvlRiconosciuto := LivelloLog(cfg.Server.LogLivello)
	dove, percorsoLog, chiudiLog, avvisoLog := ApriLog(cfg)
	defer chiudiLog()
	log := slog.New(slog.NewTextHandler(dove, &slog.HandlerOptions{Level: lvl}))
	slog.SetDefault(log)
	// dopo chiudiLog nell'elenco dei defer, quindi prima nell'esecuzione: la riga arriva nel file
	defer func() {
		if err != nil {
			log.Error("cockpit si ferma", "err", err)
		}
	}()
	if avvisoLog != "" {
		log.Warn(avvisoLog)
	}
	if !lvlRiconosciuto {
		log.Warn("[server].log_livello non riconosciuto: si usa info", "valore", cfg.Server.LogLivello, "ammessi", "debug | info | warn | error")
	}
	for _, a := range cfg.Avvisi {
		log.Warn("cockpit.toml", "avviso", a)
	}

	// SIGTERM e' come Ctrl+C: e' quello che manda chi ferma un servizio, e senza il server moriva senza
	// chiudere le connessioni.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if o.SoloLettura() {
		return EseguiInLettura(ctx, cfg, log, o)
	}
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

// EseguiInLettura fa i lavori che leggono soltanto, su un database aperto senza migrarlo
// (ApriDatabaseInLettura): -conta-anagrafiche, -calibrazione e -anteprima-fornitori.
func EseguiInLettura(ctx context.Context, cfg *config.Config, log *slog.Logger, o Opzioni) error {
	pool, err := ApriDatabaseInLettura(ctx, cfg, risorse.FS)
	if err != nil {
		return err
	}
	defer pool.Close()
	if o.ContaAnagrafiche {
		return ContaAnagrafiche(ctx, pool)
	}
	if o.Calibrazione {
		return Calibrazione(ctx, pool, os.Stdout, o.CalibrazioneDal)
	}
	return SemeFornitori(ctx, pool, log, o.SemeFornitori, false)
}

// LivelloLog legge [server].log_livello: debug, info, warn (o warning), error. Prima tutto quello che non
// era «debug» valeva info, e un «warn» scritto nel file non abbassava niente. Una parola che non si
// riconosce vale info, e ok = false lo fa dire nel log: non e' un motivo per non partire.
func LivelloLog(s string) (livello slog.Level, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, true
	case "", "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}
