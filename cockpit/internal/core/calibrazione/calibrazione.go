// Package calibrazione misura quante volte il primo proposto era quello giusto (Smistamento M3, A5.14.6,
// A5.16.6). Legge soltanto: le fotografie delle decisioni che il server scrive al momento del gesto, e i
// dati di prima con le query «retro». È il materiale con cui uno «score» diventerà, un giorno, una
// frequenza vera; fino ad allora nessuno score si mostra come percentuale (U7).
//
// Nessuna dipendenza dal web: le query stanno in `platform/db/queries/calibrazione.sql`, il calcolo qui, e
// il comando `cockpit -calibrazione` (app/runtime) apre il database in sola lettura, chiama Misura e
// stampa con Scrivi.
package calibrazione

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// CampioneMinimo è il numero di decisioni sotto il quale una cella non dice una precisione: con tre
// decisioni «0,667» è un caso, non una misura (A5.14.6). La cella mostra i conteggi e scrive «campione
// insufficiente».
const CampioneMinimo = 20

// FraseCampioneInsufficiente è quello che una cella sotto il campione scrive al posto della precisione.
const FraseCampioneInsufficiente = "campione insufficiente"

// Rapporto è tutto quello che il comando stampa.
type Rapporto struct {
	// Host e Database sono quelli che il comando legge davvero, senza password: la prima riga, perché un
	// numero letto sul database sbagliato è peggio di nessun numero.
	Host, Database string
	// SolaLettura è `transaction_read_only` letto dentro la transazione delle misure.
	SolaLettura bool
	Dal         *time.Time
	Posta       Posta
	// La parte dei FILE (A5.14.6: `dettagli.decisione.calibrazione` e la «retro» sui documenti) la
	// aggiunge la linea dei file (F11): un campo qui accanto a Posta, la sua misura in Misura e la sua
	// sezione in Scrivi, ai tre punti segnati «parte dei file».
}

// Misura legge tutto in una transazione READ ONLY: il comando legge, e non deve poter scrivere nemmeno per
// sbaglio (il pool del comando è già in sola lettura, ApriDatabaseInLettura; qui la transazione lo dice
// da sé, e il rapporto riporta che cosa il database ha risposto).
func Misura(ctx context.Context, pool *pgxpool.Pool, dal *time.Time) (Rapporto, error) {
	cc := pool.Config().ConnConfig
	r := Rapporto{Host: fmt.Sprintf("%s:%d", cc.Host, cc.Port), Database: cc.Database, Dal: dal}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return r, fmt.Errorf("calibrazione: %w", err)
	}
	defer tx.Rollback(ctx)
	var ro string
	if err := tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&ro); err != nil {
		return r, fmt.Errorf("calibrazione: %w", err)
	}
	r.SolaLettura = ro == "on"
	q := db.New(tx)
	if r.Posta, err = MisuraPosta(ctx, q, dal); err != nil {
		return r, err
	}
	// parte dei file: qui la sua misura, nella stessa transazione
	return r, nil
}

// Precisione è giuste/n. Sufficiente dice se il campione basta a dirla (CampioneMinimo); il valore c'è
// comunque, per chi vuole guardare i conteggi.
func Precisione(giuste, n int) (p float64, sufficiente bool) {
	if n == 0 {
		return 0, false
	}
	return float64(giuste) / float64(n), n >= CampioneMinimo
}

// CellaPrecisione è la precisione come la stampa il comando: tre decimali con la virgola, oppure
// «campione insufficiente».
func CellaPrecisione(giuste, n int) string {
	p, ok := Precisione(giuste, n)
	if !ok {
		return FraseCampioneInsufficiente
	}
	return decimale(p)
}

// CellaFrequenza è una media su n decisioni che non è una precisione (l'MRR): con il campione, tre
// decimali; sotto, SenzaCampione. Anche l'MRR è una frequenza, e su tre casi non dice niente (A5.14.6).
func CellaFrequenza(x float64, n int) string {
	if n < CampioneMinimo {
		return SenzaCampione
	}
	return decimale(x)
}

// SenzaCampione è quello che una media sotto il campione scrive al posto del numero: più corto della
// frase delle precisioni, che la stessa riga dice già.
const SenzaCampione = "–"

func decimale(x float64) string {
	return strings.Replace(fmt.Sprintf("%.3f", x), ".", ",", 1)
}

// Scrivi stampa il rapporto in testo allineato. La prima riga dice che cosa si legge.
func (r Rapporto) Scrivi(w io.Writer) error {
	ro := "no"
	if r.SolaLettura {
		ro = "sì"
	}
	dal := "dall'inizio"
	if r.Dal != nil {
		dal = "dal " + r.Dal.Format("2006-01-02")
	}
	fmt.Fprintf(w, "calibrazione: database %s su %s\n", r.Database, r.Host)
	fmt.Fprintf(w, "transazione in sola lettura: %s · decisioni %s\n", ro, dal)
	fmt.Fprintf(w, "sotto %d decisioni una precisione scrive «%s» e l'MRR «%s»: sono conteggi, non ancora frequenze\n",
		CampioneMinimo, FraseCampioneInsufficiente, SenzaCampione)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	r.Posta.scrivi(tw)
	// parte dei file: qui la sua sezione
	return tw.Flush()
}
