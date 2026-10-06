// L'avvio, un passo per funzione: il log, il database, i semi, le capacita'.
//
// Sono i passi che vengono PRIMA che esista qualunque servizio, e ognuno e' una cosa che puo'
// andare storta da sola: un log che non si apre, un database che non risponde, un ruolo scritto
// male in cockpit.toml, una capacita' che non si allinea. Tenerli separati serve a leggere il
// fallimento senza rileggere tutto l'avvio.

package runtime

import (
	"context"
	"errors"
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
	return pool, ultimaApplicata(applicate), nil
}

// ultimaApplicata: la versione massima di schema_versione. E' un involucro di migrazioni.UltimaApplicata (A1c,
// P-02): il controllo dello schema e' uno solo, per l'apertura in lettura e per il comando U5.
func ultimaApplicata(applicate map[int]bool) int {
	return migrazioni.UltimaApplicata(applicate)
}

// ApriDatabaseInLettura apre il pool per i comandi che leggono soltanto (Opzioni.SoloLettura): niente
// migrazioni, niente semi, niente allineamento della coda, e ogni transazione del pool in sola lettura
// (default_transaction_read_only), cosi' un comando che dice di leggere non scrive nemmeno per sbaglio.
//
// Lo schema deve essere quello del binario. Con uno schema piu' vecchio i conti si farebbero su tabelle
// che il binario non si aspetta, e migrarlo da qui vorrebbe dire cambiare il database con un comando che
// si lancia per guardare, magari senza un backup: ci si ferma e si dice che cosa fare.
//
// E' migrazioni.ApriInLettura sul DSN della configurazione (A1c, P-02: l'apertura in sola lettura sta in
// platform/migrazioni, dove la usano anche il banco del motore A e gli aiuti delle prove), con l'errore di
// schema tradotto nel testo di sempre (spiegaSchema).
func ApriDatabaseInLettura(ctx context.Context, cfg *config.Config, fsys fs.FS) (*pgxpool.Pool, error) {
	pool, err := migrazioni.ApriInLettura(ctx, cfg.DB.DSN, fsys)
	if err != nil {
		return nil, spiegaSchema(err, "legge soltanto e non migra")
	}
	return pool, nil
}

// ApriDatabaseSenzaMigrare apre il pool per un comando che scrive ma non deve cambiare lo schema: il comando
// U5 in applicazione (-riapri-agganci, Smistamento F7, P38). E' ApriDatabaseInLettura senza la sola lettura:
// niente migrazioni, niente semi, niente coda, e con uno schema diverso da quello del binario ci si ferma.
// Migrare da qui vorrebbe dire cambiare il database prima del backup che il comando chiede.
//
// Resta in app/runtime (A1c, P-02): apre un pool scrivibile, che al banco e alle prove non serve. Il controllo
// dello schema e' quello comune di platform/migrazioni (UltimaApplicata, SchemaDiverso).
func ApriDatabaseSenzaMigrare(ctx context.Context, cfg *config.Config, fsys fs.FS) (*pgxpool.Pool, error) {
	return apriSenzaMigrare(ctx, cfg, fsys)
}

// apriSenzaMigrare apre il pool scrivibile del comando U5 e controlla lo schema come migrazioni.ApriInLettura.
// Il ramo della sola lettura, che c'era qui, ora e' di ApriInLettura.
func apriSenzaMigrare(ctx context.Context, cfg *config.Config, fsys fs.FS) (*pgxpool.Pool, error) {
	migs, err := migrazioni.Elenca(fsys)
	if err != nil {
		return nil, err
	}
	delBinario := migs[len(migs)-1].Versione
	pc, err := pgxpool.ParseConfig(cfg.DB.DSN)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	applicate, err := migrazioni.Applicate(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if v := migrazioni.UltimaApplicata(applicate); v != delBinario {
		pool.Close()
		return nil, spiegaSchema(&migrazioni.SchemaDiverso{DelDatabase: v, DelBinario: delBinario}, "non migra")
	}
	return pool, nil
}

// spiegaSchema traduce migrazioni.SchemaDiverso nel testo del Cockpit: «backup, poi cockpit.exe -migra» oppure
// «serve il cockpit.exe aggiornato» (schemaDelBinario). cosa dice che cosa fa il comando («legge soltanto e non
// migra», «non migra»). Gli altri errori passano com'erano. Per il banco il rimedio e' un altro, e lo scrive il
// banco.
func spiegaSchema(err error, cosa string) error {
	var sd *migrazioni.SchemaDiverso
	if errors.As(err, &sd) {
		return schemaDelBinario(sd.DelDatabase, sd.DelBinario, cosa)
	}
	return err
}

// stessoSchema e' il rifiuto dei comandi in sola lettura quando lo schema del database non e' quello del
// binario.
func stessoSchema(delDatabase, delBinario int) error {
	return schemaDelBinario(delDatabase, delBinario, "legge soltanto e non migra")
}

// schemaDelBinario e' il rifiuto di un comando che non migra (cosa dice che cosa fa: «legge soltanto e non
// migra», «non migra») quando lo schema del database non e' quello del binario.
func schemaDelBinario(delDatabase, delBinario int, cosa string) error {
	switch {
	case delDatabase < delBinario:
		return fmt.Errorf("il database e' alla versione %d dello schema e questo cockpit.exe alla %d: questo comando %s. "+
			"Fare un backup del database, poi `cockpit.exe -migra`, poi rilanciare il comando", delDatabase, delBinario, cosa)
	case delDatabase > delBinario:
		return fmt.Errorf("il database e' alla versione %d dello schema, ma questo cockpit.exe conosce solo fino alla %d: "+
			"serve il cockpit.exe aggiornato (il database non e' stato toccato)", delDatabase, delBinario)
	}
	return nil
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
