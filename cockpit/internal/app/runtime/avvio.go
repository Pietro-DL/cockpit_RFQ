// L'avvio, un passo per funzione: il log, il database, i semi, le capacita'.
//
// Sono i passi che vengono PRIMA che esista qualunque servizio, e ognuno e' una cosa che puo'
// andare storta da sola: un log che non si apre, un database che non risponde, un ruolo scritto
// male in cockpit.toml, una capacita' che non si allinea. Tenerli separati serve a leggere il
// fallimento senza rileggere tutto l'avvio.

package runtime

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/logfile"
	"promatec/cockpit/internal/platform/migrazioni"
)

// ApriLog apre il log del server: sullo stdout sempre, e anche su file se [server].log_file (o il suo
// default sotto lo staging) dice dove. Solo sullo stdout durava quanto la finestra che aveva avviato il
// server: chiusa quella, di cio' che il server aveva risposto ai worker non restava niente da leggere.
//
// `chiudi` va messa in defer dal chiamante. `percorso` e' quello davvero in uso — vuoto se il file non
// si e' aperto — perche' finisce nella riga «cockpit in ascolto»: dire dove si scrive un log che non
// esiste manderebbe a cercare un file che non c'e'.
func ApriLog(cfg *config.Config) (scrivi io.Writer, percorso string, chiudi func(), avviso string) {
	scrivi, chiudi = os.Stdout, func() {}
	percorso = cfg.PercorsoLog()
	if percorso != "" {
		f, err := logfile.Apri(percorso, logfile.MaxByteDefault, logfile.CopieDefault)
		if err != nil {
			// un log che non si apre non è un motivo per non partire: si dice e si va avanti
			avviso = fmt.Sprintf("log su file non disponibile (%s): %v", percorso, err)
			percorso = ""
		} else {
			chiudi = func() { _ = f.Close() }
			scrivi = io.MultiWriter(os.Stdout, f)
		}
	}
	return scrivi, percorso, chiudi, avviso
}

// ApriDatabase apre il pool, applica le migrazioni e dice a che versione dello schema siamo.
//
// La versione serve subito dopo, a `Semina`: e' quella che decide se la regola «una sola casella
// attiva» vale ancora. Chiudere il pool e' del chiamante, che sa fino a quando serve.
func ApriDatabase(ctx context.Context, cfg *config.Config, fsys fs.FS, log *slog.Logger) (*pgxpool.Pool, int, error) {
	pool, err := pgxpool.New(ctx, cfg.DB.DSN)
	if err != nil {
		return nil, 0, fmt.Errorf("db: %w", err)
	}
	if _, err := migrazioni.Applica(ctx, pool, fsys, log); err != nil {
		pool.Close()
		return nil, 0, err
	}
	applicate, err := migrazioni.Applicate(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, 0, err
	}
	versione := 0
	for v := range applicate {
		if v > versione {
			versione = v
		}
	}
	return pool, versione, nil
}

// Semina porta in database quello che cockpit.toml dichiara. L'ordine non e' libero: prima gli
// utenti, poi caselle, postazioni e worker, che agli utenti si riferiscono per sigla.
func Semina(ctx context.Context, q *db.Queries, cfg *config.Config, versione int, log *slog.Logger) error {
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
	// L'analizzatore corrente (addendum A4.5): la versione e l'hash della configurazione che questo
	// server manda al worker. v_step_prodotto la legge per sapere quale analisi di uno STEP è quella
	// corrente, e quindi se una deroga strutturale vale ancora. Si scrive a ogni avvio: cambiare
	// [analisi] cambia la chiave, e analisi e deroghe della chiave vecchia smettono di contare.
	if versione >= 20 {
		an := coda.Analizzatore{Versione: cfg.Analisi.Versione, Parametri: cfg.Analisi.Parametri}
		if err := q.ImpostaAnalizzatoreCorrente(ctx, db.ImpostaAnalizzatoreCorrenteParams{
			VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: an.Hash()}); err != nil {
			return fmt.Errorf("analizzatore corrente: %w", err)
		}
		log.Info("analizzatore corrente", "versione", an.Versione, "configurazione", an.Hash()[:12])
	}
	// Finché lo schema è alla 0003 il cursore di sincronizzazione è per sola cartella: più di una
	// casella attiva farebbe perdere messaggi in silenzio. Meglio non partire (vedi il commento sulla
	// funzione: il perché è tutto lì).
	if err := fondazioni.UnaSolaCasellaAttiva(ctx, q, versione); err != nil {
		return fmt.Errorf("configurazione delle caselle: %w", err)
	}
	return nil
}

// CAPACITÀ DI SCRITTURA (blocco 4). Si fissano PRIMA che scheduler ed esecutore partano: il primo
// claim arriva pochi millisecondi dopo, e un claim fatto mentre le capacità sono ancora quelle di
// default eseguirebbe proprio i job che la configurazione vuole fermi.
func ImpostaCapacita(ctx context.Context, q *db.Queries, cfg *config.Config, log *slog.Logger) (coda.Capacita, error) {
	sic := cfg.Capacita()
	capacita := coda.Capacita{OutlookScrittura: sic.OutlookScrittura, Bozze: sic.Bozze, NasScrittura: sic.NasScrittura}
	coda.ImpostaCapacita(capacita)
	if err := coda.AllineaCoda(ctx, q, capacita, log); err != nil {
		return capacita, err
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
	return capacita, nil
}
