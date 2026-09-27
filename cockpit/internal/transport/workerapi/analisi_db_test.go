//go:build integrazione

// L4 — A15, seconda metà: i fatti di UNA analisi si distribuiscono a tutte le proposte aperte dello
// stesso file, e non toccano quelle già decise.
//
// La prima metà (un solo job accodato) è in platform/coda. Qui si prova cosa succede quando quel job
// finisce: i fatti sono del file, le proposte sono delle RFQ. Confonderli significa, a seconda del
// verso, o rianalizzare lo stesso disegno per ogni RFQ, o scrivere in tutte le RFQ la lettura fatta
// per una sola.
package workerapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

const shaA15 = "aa11bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff00112233"

type copiaFile struct {
	allegato uuid.UUID
	proposta uuid.UUID
}

// Riscritta per lo Smistamento (F4, P15, E03): prima tutte le copie si chiamavano «disegno.pdf» e ricevevano
// tipo, codice e confidenza dell'esito del worker (disegno_2d, 1234567A, 92). Adesso le copie hanno nomi
// diversi: ricevono il tipo (e' del contenuto, dai termini del testo) e rileggono il LORO nome, e la confidenza
// e' lo score della lettura del codice (il nome, 45), non quello del worker. La proposta decisa resta com'era.
func TestIFattiDiUnAnalisiArrivanoATutteLeProposteAperte(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 1, Parametri: map[string]any{"termini": []any{"scala"}}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}

	crea := func(n int, nome, direzione string, statoProposta string) copiaFile {
		var convID, msgID uuid.UUID
		var c copiaFile
		suffisso := string(rune('a' + n))
		if err := pool.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
			VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-W-"+suffisso).Scan(&convID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
			VALUES ('outlook', $1, $2, $3, now(), 'RFQ A15') RETURNING messaggio_id`,
			"<w-"+suffisso+"@acme.example>", convID, direzione).Scan(&msgID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura,
			origine, bytes, sha256, path_staging, ricevuto_il)
			VALUES ($1, 1, $4, 'pdf', 'file', 'outlook', 1000, $2, $3, now()) RETURNING allegato_id`,
			msgID, shaA15, `C:\staging\a15\disegno.pdf`, nome).Scan(&c.allegato); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte, stato)
			VALUES ($1, 'altro', 20, 'estensione', $2) RETURNING proposta_id`, c.allegato, statoProposta).Scan(&c.proposta); err != nil {
			t.Fatal(err)
		}
		return c
	}

	// due proposte ancora aperte, una già decisa da un operatore
	prima := crea(0, "1234567A_4.pdf", "entrata", "aperta")
	seconda := crea(1, "1234567B.pdf", "entrata", "aperta")
	decisa := crea(2, "disegno.pdf", "entrata", "confermata")

	// il job è quello partito per la prima copia; le altre due non ne hanno uno (A15, prima metà)
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{
		AllegatoID: prima.allegato, Bytes: 1000, Sha256: shaA15,
		NomeFile: "1234567A_4.pdf", VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}

	dati, _ := json.Marshal(worker.RisultatoAnalisi{
		AllegatoID: prima.allegato, TipoProposto: "disegno_2d", Codice: "1234567A", Rev: "4",
		Confidenza: 92, Fonte: "cartiglio", Dettagli: json.RawMessage(`{"termini":["scala","materiale"]}`),
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.applicaRisultato(ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("applica risultato: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// i FATTI: una riga sola, per (contenuto, versione, configurazione)
	if n := testutil.Conta(t, pool, "analisi_fatti"); n != 1 {
		t.Errorf("righe in analisi_fatti = %d, attesa 1", n)
	}
	fatti, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{
		Sha256: shaA15, VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	if err != nil {
		t.Fatalf("i fatti non sono stati conservati sotto la chiave chiesta: %v", err)
	}
	if len(fatti.Fatti) == 0 {
		t.Error("fatti vuoti")
	}

	// le PROPOSTE aperte: tutte aggiornate, ciascuna la sua, con il codice del SUO nome
	for nome, c := range map[string]struct {
		copiaFile
		codice, rev string
	}{"prima": {prima, "1234567A", "4"}, "seconda": {seconda, "1234567B", ""}} {
		p, err := q.GetProposta(ctx, c.proposta)
		if err != nil {
			t.Fatal(err)
		}
		if p.TipoProposto != db.TipoDocumentoDisegno2d || p.Codice.String != c.codice || p.Rev.String != c.rev || p.Confidenza != 45 ||
			p.Fonte != db.FontePropostaNomeFile {
			t.Errorf("proposta %s non aggiornata dai fatti: tipo=%s codice=%q rev=%q conf=%d fonte=%s",
				nome, p.TipoProposto, p.Codice.String, p.Rev.String, p.Confidenza, p.Fonte)
		}
		if v, ok := classificazione.LeggiValutazione(p.Dettagli); !ok || v.Da != classificazione.DaAnalisi || v.Tipo.Regola != "pdf_termini_cartiglio" {
			t.Errorf("proposta %s senza la valutazione dell'analisi: %s", nome, p.Dettagli)
		}
	}

	// la proposta già decisa non si tocca: una decisione dell'operatore non viene riscritta da un worker
	p, err := q.GetProposta(ctx, decisa.proposta)
	if err != nil {
		t.Fatal(err)
	}
	if p.TipoProposto != db.TipoDocumentoAltro || p.Confidenza != 20 {
		t.Errorf("la proposta già decisa è stata riscritta: tipo=%s conf=%d", p.TipoProposto, p.Confidenza)
	}
}

// Il risultato che dichiara una combinazione diversa da quella chiesta non è applicabile: archivierebbe
// i fatti sotto una chiave che non li descrive, e da lì verrebbero riusati per file che non c'entrano.
func TestRisultatoConConfigurazioneDiversaNonSiApplica(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	an := coda.Analizzatore{Versione: 1, Parametri: map[string]any{"termini": []any{"scala"}}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}

	var convID, msgID, allegatoID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook','CONV-X', now()) RETURNING conversazione_id`).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		VALUES ('outlook','<x@acme.example>',$1,'entrata', now()) RETURNING messaggio_id`, convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, ricevuto_il)
		VALUES ($1, 1, 'disegno.pdf','pdf','file','outlook',1000,$2,'C:\staging\x.pdf', now()) RETURNING allegato_id`,
		msgID, shaA15).Scan(&allegatoID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{
		AllegatoID: allegatoID, Sha256: shaA15, NomeFile: "disegno.pdf",
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato','analisi',$1,120,600,'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := db.New(pool).GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}

	casi := []struct {
		nome string
		ris  worker.RisultatoAnalisi
	}{
		{"versione diversa", worker.RisultatoAnalisi{AllegatoID: allegatoID, TipoProposto: "disegno_2d", Confidenza: 90,
			Fonte: "cartiglio", VersioneAnalizzatore: 2, HashConfigurazione: an.Hash()}},
		{"configurazione diversa", worker.RisultatoAnalisi{AllegatoID: allegatoID, TipoProposto: "disegno_2d", Confidenza: 90,
			Fonte: "cartiglio", VersioneAnalizzatore: 1, HashConfigurazione: "0000000000000000000000000000000000000000000000000000000000000000"}},
		{"tipo fuori enum", worker.RisultatoAnalisi{AllegatoID: allegatoID, TipoProposto: "boh", Confidenza: 90,
			Fonte: "cartiglio", VersioneAnalizzatore: 1, HashConfigurazione: an.Hash()}},
		{"fonte fuori enum", worker.RisultatoAnalisi{AllegatoID: allegatoID, TipoProposto: "disegno_2d", Confidenza: 90,
			Fonte: "telepatia", VersioneAnalizzatore: 1, HashConfigurazione: an.Hash()}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			dati, _ := json.Marshal(c.ris)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			err = s.applicaRisultato(ctx, db.New(tx), &j, dati, nil)
			if err == nil {
				t.Fatalf("il risultato è stato applicato: doveva essere rifiutato (422, job fallito definitivo)")
			}
			if n := testutil.Conta(t, pool, "analisi_fatti"); n != 0 {
				t.Errorf("un risultato rifiutato ha scritto %d righe in analisi_fatti", n)
			}
		})
	}
}
