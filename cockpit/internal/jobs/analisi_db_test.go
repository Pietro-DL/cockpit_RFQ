//go:build integrazione

// L4 — A15: un solo job di analisi per contenuto, versione e configurazione.
//
// Lo stesso disegno arriva in tre richieste di due clienti diversi. Con la chiave per allegato
// partivano tre job che aprivano lo stesso file e producevano gli stessi fatti; il lavoro era triplo e
// il risultato identico. La parte che resta legittimamente tripla è la PROPOSTA — dipende dalla RFQ e
// dal cliente — e infatti la distribuzione dei fatti alle tre proposte è provata in internal/workerapi.
// Qui si prova la metà che riguarda la coda: quanti job partono, e quando ne riparte uno.
package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

const shaProva = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// allegatoConHash crea messaggio e allegato con lo sha256 indicato, in un messaggio proprio: è la
// situazione vera, cioè lo stesso file in messaggi diversi, non lo stesso allegato riletto.
func allegatoConHash(t *testing.T, ctx context.Context, p *pgxpool.Pool, n int, sha string) db.Allegato {
	t.Helper()
	var convID, msgID, allID uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-A15-"+string(rune('a'+n))).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ A15') RETURNING messaggio_id`,
		"<a15-"+string(rune('a'+n))+"@acme.example>", convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, ricevuto_il)
		VALUES ($1, 1, 'disegno.pdf', 'pdf', 'file', 'outlook', 1000, $2, $3, now()) RETURNING allegato_id`,
		msgID, sha, `C:\staging\a15\disegno.pdf`).Scan(&allID); err != nil {
		t.Fatal(err)
	}
	a, err := db.New(p).GetAllegato(ctx, allID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func contaJobAnalisi(t *testing.T, p *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(),
		`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato' AND stato IN ('pronto','in_corso')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUnSoloJobDiAnalisiPerContenutoVersioneEConfigurazione(t *testing.T) {
	p, q, ctx := preparaDB(t)
	an := Analizzatore{Versione: 1, Parametri: map[string]any{"termini_cad": []any{"scala", "materiale"}}}

	// tre allegati, tre messaggi, lo stesso contenuto: tutti scaricati prima che l'analisi finisca
	var allegati []db.Allegato
	for i := 0; i < 3; i++ {
		allegati = append(allegati, allegatoConHash(t, ctx, p, i, shaProva))
	}
	accodati := 0
	for _, a := range allegati {
		j, err := AccodaAnalisi(ctx, q, a, uuid.NullUUID{}, an)
		if err != nil {
			t.Fatal(err)
		}
		if j != nil {
			accodati++
		}
	}
	if accodati != 1 || contaJobAnalisi(t, p) != 1 {
		t.Fatalf("job accodati = %d (in coda %d), atteso 1: lo stesso file non va analizzato tre volte",
			accodati, contaJobAnalisi(t, p))
	}

	// la chiave dice a colpo d'occhio con che cosa è stata chiesta l'analisi
	var chiave string
	if err := p.QueryRow(ctx, `SELECT chiave_idempotenza FROM job WHERE tipo = 'analizza_allegato'`).Scan(&chiave); err != nil {
		t.Fatal(err)
	}
	if atteso := "analizza:" + shaProva + ":1:" + an.Hash(); chiave != atteso {
		t.Errorf("chiave = %q\natteso   %q", chiave, atteso)
	}

	// finita l'analisi, i fatti restano per quella terna: non si riaccoda niente
	dett := []byte(`{"termini":["scala"]}`)
	if _, err := q.UpsertAnalisiFatti(ctx, db.UpsertAnalisiFattiParams{
		Sha256: shaProva, VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(), Fatti: dett,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'analizza_allegato'`); err != nil {
		t.Fatal(err)
	}
	for _, a := range allegati {
		j, err := AccodaAnalisi(ctx, q, a, uuid.NullUUID{}, an)
		if err != nil {
			t.Fatal(err)
		}
		if j != nil {
			t.Errorf("rianalisi accodata per un file i cui fatti sono già calcolati (job %d)", j.JobID)
		}
	}

	// cambia il dizionario dei termini: configurazione diversa, fatti diversi, un job nuovo
	an2 := Analizzatore{Versione: 1, Parametri: map[string]any{"termini_cad": []any{"scala", "materiale", "tolleranza"}}}
	if an2.Hash() == an.Hash() {
		t.Fatal("due configurazioni diverse hanno lo stesso hash")
	}
	j, err := AccodaAnalisi(ctx, q, allegati[0], uuid.NullUUID{}, an2)
	if err != nil {
		t.Fatal(err)
	}
	if j == nil {
		t.Fatal("con una configurazione nuova l'analisi non è ripartita: i fatti vecchi non valgono più")
	}
	// e resta uno solo anche per la configurazione nuova
	for _, a := range allegati[1:] {
		if altro, err := AccodaAnalisi(ctx, q, a, uuid.NullUUID{}, an2); err != nil {
			t.Fatal(err)
		} else if altro != nil {
			t.Errorf("secondo job con la stessa configurazione nuova: %d", altro.JobID)
		}
	}

	// anche una versione nuova dell'analizzatore fa ripartire l'analisi
	an3 := Analizzatore{Versione: 2, Parametri: an.Parametri}
	if j3, err := AccodaAnalisi(ctx, q, allegati[0], uuid.NullUUID{}, an3); err != nil {
		t.Fatal(err)
	} else if j3 == nil {
		t.Error("con una versione nuova dell'analizzatore l'analisi non è ripartita")
	}
}

// L'hash della configurazione non deve dipendere dall'ordine in cui i parametri sono scritti: se
// dipendesse, riordinare due righe del file di configurazione farebbe rianalizzare l'intero archivio.
func TestHashDellaConfigurazioneStabile(t *testing.T) {
	a := Analizzatore{Versione: 1, Parametri: map[string]any{"b": 2, "a": 1, "c": []any{"x", "y"}}}
	b := Analizzatore{Versione: 1, Parametri: map[string]any{"c": []any{"x", "y"}, "a": 1, "b": 2}}
	if a.Hash() != b.Hash() {
		t.Errorf("stessi parametri in ordine diverso danno hash diversi:\n%s\n%s", a.Hash(), b.Hash())
	}
	// ma l'ordine DENTRO una lista conta: è un dizionario di termini, non un insieme
	c := Analizzatore{Versione: 1, Parametri: map[string]any{"a": 1, "b": 2, "c": []any{"y", "x"}}}
	if a.Hash() == c.Hash() {
		t.Error("due liste di termini in ordine diverso danno lo stesso hash")
	}
	if len(a.Hash()) != 64 {
		t.Errorf("l'hash non è uno sha256 esadecimale: %q", a.Hash())
	}
}

// un allegato senza hash (file non ancora scaricato) non deve far saltare la deduplica né bloccarsi
func TestAnalisiSenzaHashNonSiDeduplica(t *testing.T) {
	p, q, ctx := preparaDB(t)
	an := Analizzatore{Versione: 1}
	a := allegatoConHash(t, ctx, p, 0, shaProva)
	a.Sha256 = pgtype.Text{}
	if _, err := AccodaAnalisi(ctx, q, a, uuid.NullUUID{}, an); err != nil {
		t.Fatalf("allegato senza hash: %v", err)
	}
	if n := contaJobAnalisi(t, p); n != 1 {
		t.Errorf("job in coda = %d, atteso 1", n)
	}
}
