//go:build integrazione

// L4 — le capacità di scrittura contro PostgreSQL vero (blocco 4 del checkpoint 3R; SH1, SH2, SH3).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/jobs/
//
// Il blocco vale se e solo se regge nei due punti INSIEME: quando un job si accoda e quando un job si
// prende. Il primo da solo è una porta chiusa con la finestra aperta — la coda sopravvive al cambio
// di configurazione, e un `copia_nas` della settimana scorsa scriverebbe sul NAS appena un worker lo
// prende.
//
// La novità del blocco 4 è che le capacità sono TRE e si spengono una per volta. La prova che conta
// è quella con `nas_scrittura` accesa e le due di Outlook spente: è la configurazione con cui si
// prova la copia sul NAS di prova, e finché esisteva un interruttore solo non si poteva nemmeno
// esprimere.
package jobs

import (
	"errors"
	"strings"
	"testing"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

// tutto sono le capacità accese: è lo stato di default del processo (vedi capacita.go).
var tutto = Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}

// conCapacita fissa le capacità per la durata del test e le rimette com'erano.
func conCapacita(t *testing.T, c Capacita) {
	t.Helper()
	ImpostaCapacita(c)
	t.Cleanup(func() { ImpostaCapacita(tutto) })
}

func nessunaCapacita(t *testing.T) { t.Helper(); conCapacita(t, Capacita{}) }
func tutteLeCapacita(t *testing.T) { t.Helper(); conCapacita(t, tutto) }

// letture sono i tipi di job che non modificano niente fuori dal Cockpit: non si spengono mai.
var letture = []db.TipoJob{
	db.TipoJobSyncOutlook, db.TipoJobStageAllegato, db.TipoJobAnalizzaAllegato,
	db.TipoJobApriElementoOutlook, db.TipoJobRileggiElemento, db.TipoJobEstraiArchivio,
}

// scritture sono i tipi che una capacità governa, con la capacità che serve a ciascuno.
var scritture = map[db.TipoJob]string{
	db.TipoJobSegnaLetto:         CapOutlookScrittura,
	db.TipoJobSpostaInCartella:   CapOutlookScrittura,
	db.TipoJobCreaBozzaOutlook:   CapBozze,
	db.TipoJobCopiaNas:           CapNasScrittura,
	db.TipoJobCreaCartellaThread: CapNasScrittura,
}

// SH1 — con tutte le capacità spente i job che toccano il mondo non entrano in coda, gli altri sì.
func TestSH1ConTutteLeCapacitaSpenteSoloLeLettureSiAccodano(t *testing.T) {
	p, q, ctx := preparaDB(t)
	nessunaCapacita(t)

	for tipo := range scritture {
		j, err := AccodaCon(ctx, q, tipo, map[string]any{}, "spento:"+string(tipo), 5, Opzioni{})
		if !errors.Is(err, ErrCapacitaSpenta) {
			t.Errorf("%s: accodato con le capacità spente (err=%v, job=%v)", tipo, err, j)
		}
		if j != nil {
			t.Errorf("%s: il job è stato creato lo stesso", tipo)
		}
	}
	// ciò che LEGGE deve continuare a funzionare, altrimenti spegnere le capacità non serve a niente:
	// si vuole vedere il sistema lavorare, non fermarlo
	for _, tipo := range letture {
		if _, err := AccodaCon(ctx, q, tipo, map[string]any{}, "letti:"+string(tipo), 5, Opzioni{}); err != nil {
			t.Errorf("%s: una capacità spenta ha bloccato un job di sola lettura: %v", tipo, err)
		}
	}
	if n := testutil.Conta(t, p, "job"); n != len(letture) {
		t.Errorf("in coda %d job, attesi i %d di sola lettura", n, len(letture))
	}
}

// SH3 (a) — «Apri in Outlook» resta consentito: apre una finestra e non modifica niente.
func TestSH3ApriInOutlookSiEseguePureSenzaCapacita(t *testing.T) {
	_, q, ctx := preparaDB(t)
	nessunaCapacita(t)
	if _, err := AccodaCon(ctx, q, db.TipoJobApriElementoOutlook, map[string]any{}, "apri", 1, Opzioni{}); err != nil {
		t.Fatalf("apri_elemento_outlook rifiutato: %v", err)
	}
	j := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if j == nil || j.Tipo != db.TipoJobApriElementoOutlook {
		t.Fatalf("il worker non ha potuto prendere l'apertura: %v", j)
	}
}

// LA PROVA DEL BLOCCO 4: il NAS di prova acceso, Outlook intatto.
//
// È la configurazione con cui si prova la copia reale: `nas_scrittura = true`, `outlook_scrittura` e
// `bozze` spente. Nessuna mail deve poter diventare letta, spostarsi o generare una bozza mentre un
// file viene scritto nel fascicolo. Con un interruttore solo questa frase non si poteva nemmeno
// scrivere in un file di configurazione.
func TestIlNasSiAccendeSenzaAccendereOutlook(t *testing.T) {
	_, q, ctx := preparaDB(t)
	conCapacita(t, Capacita{NasScrittura: true})

	for _, tipo := range []db.TipoJob{db.TipoJobCopiaNas, db.TipoJobCreaCartellaThread} {
		j, err := AccodaCon(ctx, q, tipo, map[string]any{}, "nas:"+string(tipo), 5, Opzioni{})
		if err != nil || j == nil {
			t.Errorf("%s: rifiutato con nas_scrittura accesa (err=%v)", tipo, err)
		}
	}
	for _, tipo := range []db.TipoJob{db.TipoJobSegnaLetto, db.TipoJobSpostaInCartella, db.TipoJobCreaBozzaOutlook} {
		j, err := AccodaCon(ctx, q, tipo, map[string]any{}, "out:"+string(tipo), 5, Opzioni{})
		if !errors.Is(err, ErrCapacitaSpenta) || j != nil {
			t.Errorf("%s: accodato mentre si accendeva SOLO il NAS (err=%v, job=%v)", tipo, err, j)
		}
	}
	// e il claim è d'accordo con l'accodamento: i due punti non possono dire cose diverse
	visti := map[db.TipoJob]bool{}
	for giro := 0; giro < 3; giro++ {
		if j := claim(t, ctx, q, db.WorkerTipoServer, "server"); j != nil {
			visti[j.Tipo] = true
		}
	}
	if !visti[db.TipoJobCopiaNas] || !visti[db.TipoJobCreaCartellaThread] {
		t.Errorf("il server non ha preso le scritture sul NAS che erano consentite: %v", visti)
	}
	if j := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC"); j != nil {
		t.Errorf("il worker Outlook ha preso il job %d (%s) con outlook_scrittura spenta", j.JobID, j.Tipo)
	}
}

// Le bozze sono una capacità A PARTE: aprire una finestra di composizione sul PC di qualcuno non è la
// stessa cosa di segnare letta una mail, e il mandato le tiene separate.
func TestLeBozzeSonoUnaCapacitaAParte(t *testing.T) {
	_, q, ctx := preparaDB(t)

	conCapacita(t, Capacita{OutlookScrittura: true})
	if _, err := AccodaCon(ctx, q, db.TipoJobSegnaLetto, map[string]any{}, "letto:1", 5, Opzioni{}); err != nil {
		t.Errorf("segna_letto rifiutato con outlook_scrittura accesa: %v", err)
	}
	_, err := AccodaCon(ctx, q, db.TipoJobCreaBozzaOutlook, map[string]any{}, "bozza:1", 5, Opzioni{})
	if !errors.Is(err, ErrCapacitaSpenta) {
		t.Errorf("la bozza è passata con la sola outlook_scrittura: %v", err)
	}

	ImpostaCapacita(Capacita{Bozze: true})
	if _, err := AccodaCon(ctx, q, db.TipoJobCreaBozzaOutlook, map[string]any{}, "bozza:2", 5, Opzioni{}); err != nil {
		t.Errorf("bozza rifiutata con bozze accesa: %v", err)
	}
	_, err = AccodaCon(ctx, q, db.TipoJobSegnaLetto, map[string]any{}, "letto:2", 5, Opzioni{})
	if !errors.Is(err, ErrCapacitaSpenta) {
		t.Errorf("segna_letto è passato con la sola capacità bozze: %v", err)
	}
}

// L'errore dice QUALE capacità manca. Chi legge deve sapere quale riga del file cambiare: «il server
// è in shadow» era vero e non aiutava, perché spegneva tre cose insieme.
func TestLErroreNominaLaCapacitaCheManca(t *testing.T) {
	_, q, ctx := preparaDB(t)
	nessunaCapacita(t)
	for tipo, atteso := range scritture {
		_, err := AccodaCon(ctx, q, tipo, map[string]any{}, "motivo:"+string(tipo), 5, Opzioni{})
		if got := CapacitaMancante(err); got != atteso {
			t.Errorf("%s: capacità mancante = %q, attesa %q", tipo, got, atteso)
		}
		if !strings.Contains(err.Error(), atteso) || !strings.Contains(err.Error(), "sicurezza") {
			t.Errorf("%s: l'errore non dice dove si accende: %v", tipo, err)
		}
	}
}

// SH2 — i job già in coda non si eseguono, e quando la loro capacità si accende vengono ANNULLATI:
// nulla di ciò che ha aspettato si mette in moto da solo.
func TestSH2IJobInCodaAspettanoEPoiVengonoAnnullati(t *testing.T) {
	_, q, ctx := preparaDB(t)
	tutteLeCapacita(t)

	// con tutto acceso: tre copie NAS e una bozza entrano regolarmente in coda
	var prima []int64
	for _, c := range []string{"a", "b", "c"} {
		prima = append(prima, accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:"+c, Opzioni{}).JobID)
	}
	bozza := accoda(t, ctx, q, db.TipoJobCreaBozzaOutlook, "bozza:x", Opzioni{}).JobID
	// e un job di sola lettura, che deve sopravvivere a tutto quanto
	letto := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "analisi:x", Opzioni{}).JobID

	// si spegne tutto
	ImpostaCapacita(Capacita{})
	if err := AllineaCoda(ctx, q, Capacita{}, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	for _, id := range append(append([]int64{}, prima...), bozza) {
		j := statoDi(t, ctx, q, id)
		if j.Stato != db.StatoJobPronto {
			t.Errorf("job %d: stato %s, atteso pronto (restano in coda, visibili)", id, j.Stato)
		}
		if !j.Errore.Valid || !strings.Contains(j.Errore.String, "in attesa di produzione") {
			t.Errorf("job %d: in admin non si legge perché è fermo (errore=%q)", id, j.Errore.String)
		}
	}
	// NESSUNO viene assegnato, né al server né al worker Outlook
	for giro := 0; giro < 3; giro++ {
		if j := claim(t, ctx, q, db.WorkerTipoServer, "server"); j != nil {
			t.Fatalf("il server ha preso il job %d (%s): avrebbe scritto sul NAS", j.JobID, j.Tipo)
		}
		if j := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC"); j != nil {
			t.Fatalf("il worker ha preso il job %d (%s)", j.JobID, j.Tipo)
		}
	}
	// il job di sola lettura invece si prende: non è un blocco generale
	if j := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC"); j == nil || j.JobID != letto {
		t.Fatalf("il job di analisi non è stato assegnato: %v", j)
	}

	// SI ACCENDE SOLO IL NAS. Le copie che avevano aspettato vengono annullate; la bozza, la cui
	// capacità è ancora spenta, resta dov'è ad aspettare.
	solaScritturaNas := Capacita{NasScrittura: true}
	ImpostaCapacita(solaScritturaNas)
	nuova := accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:dopo", Opzioni{}).JobID
	if err := AllineaCoda(ctx, q, solaScritturaNas, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	for _, id := range prima {
		j := statoDi(t, ctx, q, id)
		if j.Stato != db.StatoJobAnnullato {
			t.Errorf("job %d: stato %s, atteso annullato quando la sua capacità si è accesa", id, j.Stato)
		}
		if j.Tentativi != 0 {
			t.Errorf("job %d: eseguito %d volte, doveva non partire mai", id, j.Tentativi)
		}
	}
	if j := statoDi(t, ctx, q, bozza); j.Stato != db.StatoJobPronto {
		t.Errorf("la bozza è stata annullata (%s) mentre la sua capacità è ancora spenta: aspetta, non si butta", j.Stato)
	}
	if j := statoDi(t, ctx, q, nuova); j.Stato != db.StatoJobPronto {
		t.Errorf("la copia accodata dopo l'accensione è stata annullata (%s): il marcatore non distingue", j.Stato)
	}
	// e adesso il server la prende
	if j := claim(t, ctx, q, db.WorkerTipoServer, "server"); j == nil || j.JobID != nuova {
		t.Fatalf("il server non ha preso la copia nuova: %v", j)
	}
}

// Un avvio con le capacità già accese e nessun job marcato non deve annullare niente: sarebbe il modo
// più rapido di buttare via le copie NAS in coda a ogni riavvio.
func TestAllineaCodaNonAnnullaCiòCheNonHaMaiAspettato(t *testing.T) {
	_, q, ctx := preparaDB(t)
	tutteLeCapacita(t)
	id := accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:solo", Opzioni{}).JobID
	if err := AllineaCoda(ctx, q, tutto, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if j := statoDi(t, ctx, q, id); j.Stato != db.StatoJobPronto {
		t.Fatalf("job annullato senza motivo: stato %s, errore %q", j.Stato, j.Errore.String)
	}
}
