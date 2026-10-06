// lettura.go: l'apertura del database in sola lettura e il controllo dello schema che le serve (piano A, A1c,
// 6.4.3 con la base del 3.3.6; P-02). Il file serve ad APRIRE, non a migrare: non chiama mai Applica e non
// aggiunge migrazioni (in A1 l'ultima resta la 0021: G10). Le migrazioni incorporate servono solo a sapere
// qual è l'ultima versione del binario.
//
// Stanno qui, e non in app/runtime, perché le usano anche il banco del motore A e gli aiuti delle prove
// (platform/testutil), che così non dipendono da tutto il processo del server. In app/runtime restano gli
// involucri ApriDatabaseInLettura, ultimaApplicata e DestinazioneDelDSN, con i testi di sempre, e
// ApriDatabaseSenzaMigrare con la sua apertura del pool scrivibile (il comando U5): per il controllo dello
// schema usa UltimaApplicata e SchemaDiverso. Gli import nuovi sono della sola libreria standard (net, strconv):
// nessun pacchetto del progetto (G9).

package migrazioni

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApriInLettura apre un pool che non migra e non scrive: default_transaction_read_only=on nei parametri di
// connessione, così ogni transazione del pool è in sola lettura anche se chi la apre dimentica di dirlo. Lo
// schema del DB deve essere quello dell'ultima migrazione di fsys; altrimenti il pool si chiude e torna
// *SchemaDiverso, con un testo neutro: il rimedio lo dice chi chiama (per cockpit backup e -migra, per il
// banco una copia migrata dal proprietario).
//
// Gli errori di apertura sono quelli di prima (avvio.go, «db: …»): pgx toglie la password dal DSN che cita.
func ApriInLettura(ctx context.Context, dsn string, fsys fs.FS) (*pgxpool.Pool, error) {
	delBinario, err := ultimaDelBinario(fsys)
	if err != nil {
		return nil, err
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if pc.ConnConfig.RuntimeParams == nil {
		pc.ConnConfig.RuntimeParams = map[string]string{}
	}
	pc.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	applicate, err := Applicate(ctx, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if v := UltimaApplicata(applicate); v != delBinario {
		pool.Close()
		return nil, &SchemaDiverso{DelDatabase: v, DelBinario: delBinario}
	}
	return pool, nil
}

// ultimaDelBinario: la versione dell'ultima migrazione incorporata, cioè lo schema che il binario conosce.
func ultimaDelBinario(fsys fs.FS) (int, error) {
	migs, err := Elenca(fsys)
	if err != nil {
		return 0, err
	}
	return migs[len(migs)-1].Versione, nil
}

// UltimaApplicata: il massimo delle versioni registrate in schema_versione; 0 = nessuna. È il controllo dello
// schema comune all'apertura in lettura e al comando U5, che non migrano.
func UltimaApplicata(applicate map[int]bool) int {
	versione := 0
	for v := range applicate {
		if v > versione {
			versione = v
		}
	}
	return versione
}

// SchemaDiverso: il database e il binario non sono alla stessa versione dello schema. Il testo non dice che
// cosa fare, perché il rimedio dipende da chi apre: cockpit lo traduce in «backup, poi -migra» o «serve il
// cockpit.exe aggiornato» (app/runtime), il banco in «una copia migrata dal proprietario».
type SchemaDiverso struct{ DelDatabase, DelBinario int }

func (e *SchemaDiverso) Error() string {
	return fmt.Sprintf("schema diverso: il database è alla versione %d, il binario alla %d; il database non è stato toccato",
		e.DelDatabase, e.DelBinario)
}

// Destinazione dice dove porta un DSN come lo leggerà pgx, senza la password: «host:porta/nome come utente», e
// il nome del database. Il nome è quello risolto (un DSN chiave=valore, un `?dbname=` che vince sul percorso, un
// DSN senza database), non il testo del DSN: è lo stesso criterio di testutil.DatabaseDiTest. Un DSN che non si
// legge dà un errore che non lo ripete: il testo può contenere la password.
func Destinazione(dsn string) (testo, nome string, err error) {
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "", "", errors.New("migrazioni: il DSN non si legge (non si ripete: può contenere la password)")
	}
	cc := pc.ConnConfig
	return fmt.Sprintf("%s/%s come %s", net.JoinHostPort(cc.Host, strconv.Itoa(int(cc.Port))), cc.Database, cc.User), cc.Database, nil
}

// Collegamento: che cosa può fare il collegamento, letto dal catalogo da ControllaSolaLettura.
type Collegamento struct {
	Utente, Database string
	// SolaLettura: SHOW default_transaction_read_only è «on».
	SolaLettura bool
	// Superutente: il ruolo ha almeno uno fra rolsuper, rolcreatedb, rolcreaterole, rolbypassrls.
	Superutente bool
	// Scrive: il ruolo ha un privilegio di scrittura su una relazione di public, o CREATE sullo schema.
	Scrive bool
	// Ruoli: i ruoli di cui l'utente è membro (pg_auth_members), in ordine.
	Ruoli []string
	// EscluseLeggibili: le tabelle escluse che il ruolo riesce a leggere, in ordine.
	EscluseLeggibili []string
}

// Le domande di ControllaSolaLettura: solo letture del catalogo, una riga ciascuna. Le relazioni guardate sono
// quelle su cui un privilegio di tabella vuol dire scrivere: tabelle, tabelle partizionate, viste, viste
// materializzate e tabelle esterne di public.
const (
	sqlDefaultSolaLettura = `SHOW default_transaction_read_only`
	sqlUtenti             = `SELECT current_user::text, session_user::text, current_database()::text`
	sqlAttributiRuolo     = `SELECT r.rolsuper, r.rolcreatedb, r.rolcreaterole, r.rolbypassrls FROM pg_roles r WHERE r.rolname = current_user`
	sqlAppartenenze       = `SELECT coalesce(array_agg(g.rolname::text ORDER BY g.rolname::text), '{}')
  FROM pg_auth_members m JOIN pg_roles g ON g.oid = m.roleid JOIN pg_roles u ON u.oid = m.member
 WHERE u.rolname = current_user`
	sqlRelazioniScrivibili = `SELECT coalesce(array_agg(c.relname::text ORDER BY c.relname::text), '{}')
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
   AND has_table_privilege(c.oid, 'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')`
	sqlCreateSulloSchema = `SELECT has_schema_privilege('public', 'CREATE')`
	sqlEscluseLeggibili  = `SELECT coalesce(array_agg(e ORDER BY e), '{}')
  FROM unnest($1::text[]) AS e
 WHERE to_regclass(quote_ident('public') || '.' || quote_ident(e)) IS NOT NULL
   AND has_table_privilege(to_regclass(quote_ident('public') || '.' || quote_ident(e)), 'SELECT')`
)

// ControllaSolaLettura legge dal catalogo i permessi del collegamento e si ferma al primo controllo che non
// passa, con un errore che lo nomina. Fa solo letture. I controlli, in quest'ordine (6.4.3):
//  1. SHOW default_transaction_read_only = on;
//  2. current_user e session_user uguali (nessun SET ROLE);
//  3. rolsuper, rolcreatedb, rolcreaterole, rolbypassrls falsi;
//  4. nessuna appartenenza in pg_auth_members (esclude anche pg_read_all_data);
//  5. nessun privilegio di scrittura (INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER) su una relazione
//     di public, e niente CREATE sullo schema public;
//  6. nessuna delle tabelle escluse leggibile (SELECT).
//
// Il Collegamento torna sempre con ciò che si è letto fino al controllo fallito. Serve al banco (-dsn) e a
// testutil.PoolDump, prima di qualunque lettura di dominio.
func ControllaSolaLettura(ctx context.Context, c lettore, escluse []string) (Collegamento, error) {
	var col Collegamento
	var ro string
	if err := c.QueryRow(ctx, sqlDefaultSolaLettura).Scan(&ro); err != nil {
		return col, fmt.Errorf("sola lettura: default_transaction_read_only: %w", err)
	}
	col.SolaLettura = ro == "on"
	if !col.SolaLettura {
		return col, fmt.Errorf("sola lettura: default_transaction_read_only è %q, serve «on»", ro)
	}

	var sessione string
	if err := c.QueryRow(ctx, sqlUtenti).Scan(&col.Utente, &sessione, &col.Database); err != nil {
		return col, fmt.Errorf("sola lettura: utente: %w", err)
	}
	if col.Utente != sessione {
		return col, fmt.Errorf("sola lettura: current_user %q diverso da session_user %q (SET ROLE)", col.Utente, sessione)
	}

	var super, creaDB, creaRuoli, bypass bool
	if err := c.QueryRow(ctx, sqlAttributiRuolo).Scan(&super, &creaDB, &creaRuoli, &bypass); err != nil {
		return col, fmt.Errorf("sola lettura: attributi del ruolo %q: %w", col.Utente, err)
	}
	var attributi []string
	for _, a := range []struct {
		nome string
		ha   bool
	}{{"rolsuper", super}, {"rolcreatedb", creaDB}, {"rolcreaterole", creaRuoli}, {"rolbypassrls", bypass}} {
		if a.ha {
			attributi = append(attributi, a.nome)
		}
	}
	col.Superutente = len(attributi) > 0
	if col.Superutente {
		return col, fmt.Errorf("sola lettura: il ruolo %q ha %s", col.Utente, strings.Join(attributi, ", "))
	}

	if err := c.QueryRow(ctx, sqlAppartenenze).Scan(&col.Ruoli); err != nil {
		return col, fmt.Errorf("sola lettura: appartenenze (pg_auth_members): %w", err)
	}
	if len(col.Ruoli) > 0 {
		return col, fmt.Errorf("sola lettura: il ruolo %q è membro di %s (pg_auth_members)", col.Utente, strings.Join(col.Ruoli, ", "))
	}

	var scrivibili []string
	if err := c.QueryRow(ctx, sqlRelazioniScrivibili).Scan(&scrivibili); err != nil {
		return col, fmt.Errorf("sola lettura: privilegi di scrittura: %w", err)
	}
	if len(scrivibili) > 0 {
		col.Scrive = true
		return col, fmt.Errorf("sola lettura: il ruolo %q ha privilegi di scrittura su %d relazioni di public (per esempio %s)",
			col.Utente, len(scrivibili), scrivibili[0])
	}
	var crea bool
	if err := c.QueryRow(ctx, sqlCreateSulloSchema).Scan(&crea); err != nil {
		return col, fmt.Errorf("sola lettura: CREATE sullo schema public: %w", err)
	}
	if crea {
		col.Scrive = true
		return col, fmt.Errorf("sola lettura: il ruolo %q ha CREATE sullo schema public", col.Utente)
	}

	if escluse == nil {
		escluse = []string{}
	}
	if err := c.QueryRow(ctx, sqlEscluseLeggibili, escluse).Scan(&col.EscluseLeggibili); err != nil {
		return col, fmt.Errorf("sola lettura: tabelle escluse: %w", err)
	}
	if len(col.EscluseLeggibili) > 0 {
		return col, fmt.Errorf("sola lettura: il ruolo %q legge le tabelle escluse %s", col.Utente, strings.Join(col.EscluseLeggibili, ", "))
	}
	return col, nil
}
