//go:build integrazione

// L4 — V1 (emendamento di V1 §9.9; piano della 0023 §14 passo 3, corsia motore; A-27 e A-28 la parte L4): le letture
// nuove del modello di /motore-a/dati sul database di prova (COCKPIT_TEST_DSN), nello stesso snapshot della fotografia,
// sulla scena ACME di servizio_db_test.go (scenaMA):
//   - senza righe di V1: nessuna occorrenza non attiva, F8 mai previsto, nessuna bozza; i gruppi del prodotto con
//     l'impronta_contenuto di un'importazione fatta adesso (64 esadecimali) e i nodi, la fonte confermata e autorizzata
//     (lo STEP autorizzato della scena);
//   - con il PDF scartato (le righe del ciclo dei file della 0023: stato, evento, rimozione) e una riga di F8 in errore:
//     il PDF fra le occorrenze non attive, f8_stato «errore» nel JSON;
//   - il calcolo non cambia il database.
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4MotoreALeLettureDiV1(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	prima := testutil.FotoDelDatabase(t, p)
	ris, err := svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if len(ris.V1.NonAttivi) != 0 || ris.V1.F8 != "" || len(ris.V1.Bozze) != 0 {
		t.Errorf("le letture di V1 senza righe: %+v", ris.V1)
	}
	pg, err := svc.Pagina(ctx, RichiestaPagina{Thread: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	var bom *propostabom.GruppoDelModello
	for i := range pg.Gruppi {
		if pg.Gruppi[i].Chiave == propostabom.GruppoBOM {
			bom = &pg.Gruppi[i]
		}
	}
	if bom == nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(bom.ImprontaContenuto) || len(bom.Nodi) != 2 || bom.Importazione != nil ||
		bom.RadiceLetterale != "9120100A" || bom.NomeFile != "9120100A.stp" {
		t.Fatalf("il gruppo bom della scena: %+v", bom)
	}
	if pg.Fonte == nil || !pg.Fonte.Confermata || !pg.Fonte.Autorizzata || pg.Fonte.DocumentoID == nil || *pg.Fonte.DocumentoID != s.docStep {
		t.Errorf("la fonte del prodotto: %+v", pg.Fonte)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}

	// il PDF scartato e F8 in errore
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []struct {
		sql string
		arg []any
	}{
		{`INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
			estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
		  SELECT m.thread_id, a.allegato_id, 'scartato', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
			a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
		    FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{s.aPdf, s.utente}},
		{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, origine, autore_id)
		  VALUES ($1, $2, 'scarta', 'attivo', 'scartato', 'non serve alla RFQ', '{}', '{}', 'ponte', $3)`, []any{s.thread, s.aPdf, s.utente}},
		{`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, sha256)
		  SELECT e.thread_id, e.allegato_id, e.evento_id, a.sha256 FROM file_rfq_evento e JOIN allegato a ON a.allegato_id = e.allegato_id
		   WHERE e.allegato_id = $1 AND e.azione = 'scarta'`, []any{s.aPdf}},
		{`INSERT INTO f8_stato (thread_id, chiave_intento, stato, errore) VALUES ($1, gen_random_uuid(), 'errore', 'prova L4 di V1')`, []any{s.thread}},
	} {
		if _, err := tx.Exec(ctx, q.sql, q.arg...); err != nil {
			t.Fatalf("le righe di V1: %v\n%s", err, q.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT delle righe di V1: %v", err)
	}
	prima = testutil.FotoDelDatabase(t, p)
	ris, err = svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if ris.V1.NonAttivi[s.aPdf] != "scartato" || len(ris.V1.NonAttivi) != 1 || ris.V1.F8 != F8Errore {
		t.Errorf("le letture di V1 con il PDF scartato e F8 in errore: %+v", ris.V1)
	}
	pg, err = svc.Pagina(ctx, RichiestaPagina{Thread: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(pg)
	if err != nil {
		t.Fatal(err)
	}
	var j struct {
		F8Stato string            `json:"f8_stato"`
		Gruppi  []json.RawMessage `json:"gruppi"`
	}
	if err := json.Unmarshal(b, &j); err != nil || j.F8Stato != F8Errore || len(j.Gruppi) == 0 {
		t.Errorf("il JSON di /dati: f8_stato %q, gruppi %d (%v)", j.F8Stato, len(j.Gruppi), err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}
}
