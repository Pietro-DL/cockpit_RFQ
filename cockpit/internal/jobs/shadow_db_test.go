//go:build integrazione

// L4 — modalità shadow contro PostgreSQL vero (voce 9.5, §2.7, D16). Test SH1, SH2, SH3.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/jobs/
//
// La shadow serve a far girare il Cockpit sulla posta vera senza toccarla. Vale se e solo se regge
// nei due punti insieme: quando un job si accoda e quando un job si prende. Il primo da solo è una
// porta chiusa con la finestra aperta — la coda sopravvive al cambio di modalità, e un `copia_nas`
// della settimana scorsa scriverebbe sul NAS vero appena un worker lo prende.
package jobs

import (
	"errors"
	"strings"
	"testing"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

// shadow mette il processo in shadow per la durata del test e lo rimette com'era.
func shadow(t *testing.T) {
	t.Helper()
	ImpostaModalita(ModalitaShadow)
	t.Cleanup(func() { ImpostaModalita(ModalitaProduzione) })
}

func produzione(t *testing.T) {
	t.Helper()
	ImpostaModalita(ModalitaProduzione)
	t.Cleanup(func() { ImpostaModalita(ModalitaProduzione) })
}

// SH1 — in shadow i job che toccano il mondo non entrano in coda, gli altri sì.
func TestSH1InShadowSoloIJobCheToccanoIlMondoNonSiAccodano(t *testing.T) {
	p, q, ctx := preparaDB(t)
	shadow(t)

	for _, tipo := range []db.TipoJob{
		db.TipoJobCreaBozzaOutlook, db.TipoJobSegnaLetto, db.TipoJobSpostaInCartella,
		db.TipoJobCopiaNas, db.TipoJobCreaCartellaThread,
	} {
		j, err := AccodaCon(ctx, q, tipo, map[string]any{}, "shadow:"+string(tipo), 5, Opzioni{})
		if !errors.Is(err, ErrShadow) {
			t.Errorf("%s: accodato in shadow (err=%v, job=%v)", tipo, err, j)
		}
		if j != nil {
			t.Errorf("%s: il job è stato creato lo stesso", tipo)
		}
	}
	// ciò che LEGGE deve continuare a funzionare, altrimenti la shadow non serve a niente:
	// in shadow si vuole vedere il sistema lavorare, non fermarlo
	for _, tipo := range []db.TipoJob{
		db.TipoJobSyncOutlook, db.TipoJobStageAllegato, db.TipoJobAnalizzaAllegato,
		db.TipoJobApriElementoOutlook, db.TipoJobRileggiElemento,
	} {
		if _, err := AccodaCon(ctx, q, tipo, map[string]any{}, "letti:"+string(tipo), 5, Opzioni{}); err != nil {
			t.Errorf("%s: la shadow ha bloccato un job di sola lettura: %v", tipo, err)
		}
	}
	if n := testutil.Conta(t, p, "job"); n != 5 {
		t.Errorf("in coda %d job, attesi i 5 di sola lettura", n)
	}
}

// SH3 (a) — «Apri in Outlook» è l'unica azione Outlook consentita in shadow, e si esegue davvero.
func TestSH3ApriInOutlookSiEseguePureInShadow(t *testing.T) {
	_, q, ctx := preparaDB(t)
	shadow(t)
	if _, err := AccodaCon(ctx, q, db.TipoJobApriElementoOutlook, map[string]any{}, "apri", 1, Opzioni{}); err != nil {
		t.Fatalf("apri_elemento_outlook rifiutato in shadow: %v", err)
	}
	j := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if j == nil || j.Tipo != db.TipoJobApriElementoOutlook {
		t.Fatalf("il worker non ha potuto prendere l'apertura: %v", j)
	}
}

// SH2 — i job già in coda non si eseguono in shadow, e al ritorno in produzione vengono annullati:
// nulla di ciò che ha aspettato si mette in moto da solo.
func TestSH2IJobInCodaAspettanoInShadowEPoiVengonoAnnullati(t *testing.T) {
	_, q, ctx := preparaDB(t)
	produzione(t)

	// in PRODUZIONE: tre copie NAS e una bozza entrano regolarmente in coda
	var prima []int64
	for _, c := range []string{"a", "b", "c"} {
		prima = append(prima, accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:"+c, Opzioni{}).JobID)
	}
	prima = append(prima, accoda(t, ctx, q, db.TipoJobCreaBozzaOutlook, "bozza:x", Opzioni{}).JobID)
	// e un job di sola lettura, che deve sopravvivere a tutto quanto
	letto := accoda(t, ctx, q, db.TipoJobAnalizzaAllegato, "analisi:x", Opzioni{}).JobID

	// passaggio a SHADOW
	ImpostaModalita(ModalitaShadow)
	if err := AllineaCoda(ctx, q, ModalitaShadow, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	for _, id := range prima {
		j := statoDi(t, ctx, q, id)
		if j.Stato != db.StatoJobPronto {
			t.Errorf("job %d: stato %s, atteso pronto (in shadow restano in coda, visibili)", id, j.Stato)
		}
		if !j.Errore.Valid || !strings.Contains(j.Errore.String, "in attesa di produzione") {
			t.Errorf("job %d: in admin non si legge perché è fermo (errore=%q)", id, j.Errore.String)
		}
	}
	// NESSUNO dei quattro viene assegnato, né al server né al worker Outlook
	for giro := 0; giro < 3; giro++ {
		if j := claim(t, ctx, q, db.WorkerTipoServer, "server"); j != nil {
			t.Fatalf("in shadow il server ha preso il job %d (%s): avrebbe scritto sul NAS", j.JobID, j.Tipo)
		}
		if j := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC"); j != nil {
			t.Fatalf("in shadow il worker ha preso il job %d (%s)", j.JobID, j.Tipo)
		}
	}
	// il job di sola lettura invece si prende: la shadow non è un blocco generale
	if j := claim(t, ctx, q, db.WorkerTipoAnalisi, "analisi@PC"); j == nil || j.JobID != letto {
		t.Fatalf("in shadow il job di analisi non è stato assegnato: %v", j)
	}

	// una copia NAS accodata DOPO il ritorno in produzione non c'entra niente con la shadow
	ImpostaModalita(ModalitaProduzione)
	nuova := accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:dopo", Opzioni{}).JobID
	if err := AllineaCoda(ctx, q, ModalitaProduzione, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	for _, id := range prima {
		j := statoDi(t, ctx, q, id)
		if j.Stato != db.StatoJobAnnullato {
			t.Errorf("job %d: stato %s, atteso annullato al ritorno in produzione", id, j.Stato)
		}
		if !strings.Contains(j.Errore.String, "shadow") {
			t.Errorf("job %d: il motivo dell'annullamento non nomina la shadow (%q)", id, j.Errore.String)
		}
		if j.Tentativi != 0 {
			t.Errorf("job %d: eseguito %d volte, doveva non partire mai", id, j.Tentativi)
		}
	}
	if j := statoDi(t, ctx, q, nuova); j.Stato != db.StatoJobPronto {
		t.Errorf("la copia accodata dopo il ritorno in produzione è stata annullata (%s): il marcatore non distingue", j.Stato)
	}
	// e adesso il server la prende
	if j := claim(t, ctx, q, db.WorkerTipoServer, "server"); j == nil || j.JobID != nuova {
		t.Fatalf("in produzione il server non ha preso la copia nuova: %v", j)
	}
}

// Un avvio in produzione senza nessuna shadow alle spalle non deve annullare niente: sarebbe il modo
// più rapido di buttare via le copie NAS in coda a ogni riavvio.
func TestAllineaCodaInProduzioneNonAnnullaCiòCheNonHaVistoLaShadow(t *testing.T) {
	_, q, ctx := preparaDB(t)
	produzione(t)
	id := accoda(t, ctx, q, db.TipoJobCopiaNas, "nas:solo", Opzioni{}).JobID
	if err := AllineaCoda(ctx, q, ModalitaProduzione, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if j := statoDi(t, ctx, q, id); j.Stato != db.StatoJobPronto {
		t.Fatalf("job annullato senza motivo: stato %s, errore %q", j.Stato, j.Errore.String)
	}
}
