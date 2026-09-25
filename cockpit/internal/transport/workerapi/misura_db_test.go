//go:build integrazione

// L4 — 7C.1, P0: nessun valore derivato da un nome di file o da un testo esterno rompe il result
// dopo che il file e' stato caricato.
//
// Al banco a due macchine del 20/09/2026 uno stage_allegato ha caricato 16.658 byte, il server li
// ha verificati e promossi, e poi il result e' stato rifiutato con 422: «value too long for type
// character varying(60)». La colonna era documento_proposta.codice; il valore era l'intero nome
// del file, maiuscolo, perche' CodiceRev restituiva il nome intero quando non riconosceva un
// suffisso di revisione e PropostaDaNome lo scriveva come codice se dentro c'era un token qualsiasi
// che sembrava un codice.
package workerapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// nomeLungo e' un nome di file di oltre cento caratteri con dentro dei codici: prima diventava un
// codice di proposta da 100+ caratteri.
const nomeLungo = "Offerta 12345678 per fornitura staffe zincate 1234567A rev finale allegato tecnico completo e definitivo del cliente.pdf"

func TestUnNomeFileLungoNonRompeIlResultDiStage(t *testing.T) {
	b := preparaBanco(t, 0)
	if len(nomeLungo) <= 60 {
		t.Fatalf("il nome di prova deve superare i 60 caratteri della colonna: %d", len(nomeLungo))
	}
	a, _ := b.messaggioConAllegato(nomeLungo, "pdf")
	contenuto, sha := contenutoCasuale(16_658, 7)
	// l'allegato e' gia' in staging, come lo lascia il result dopo Promuovi: qui si prova dopoStaging
	pathDefinitivo, err := staging.PercorsoContenuto(b.cartella, sha, a.NomeFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET sha256 = $2, bytes = $3, path_staging = $4 WHERE allegato_id = $1`,
		a.AllegatoID, sha, len(contenuto), pathDefinitivo); err != nil {
		t.Fatal(err)
	}

	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := b.s.dopoStaging(b.ctx, db.New(tx), worker.RisultatoStage{AllegatoID: a.AllegatoID, Sha256: sha, Bytes: int64(len(contenuto))}); err != nil {
		t.Fatalf("il result di stage di un file dal nome lungo non e' applicabile: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}

	p := propostaDi(t, b.pool, a.AllegatoID)
	if p.Codice.Valid {
		t.Errorf("il nome intero e' finito nella colonna codice: %q", p.Codice.String)
	}
	var dett map[string]any
	if err := json.Unmarshal(p.Dettagli, &dett); err != nil {
		t.Fatal(err)
	}
	codici, _ := dett["codici_nel_nome"].([]any)
	if len(codici) != 2 || codici[0] != "12345678" || codici[1] != "1234567A" {
		t.Errorf("i codici trovati nel nome non sono nei dettagli: %v", dett)
	}
	// e l'analisi parte lo stesso: il nome non c'entra con il contenuto
	if n := testutil.Conta(t, b.pool, "job"); n < 2 {
		t.Errorf("job in coda = %d: manca l'analisi del contenuto", n)
	}
}

// Il codice e la revisione del RISULTATO del worker analisi sono testo esterno: fuori misura non
// entrano nella colonna, restano grezzi nei dettagli, e il result si applica.
func TestUnCodiceFuoriMisuraDelWorkerNonRompeIlResult(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	an := coda.Analizzatore{Versione: 1}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: an}

	var convID, msgID, allID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', 'CONV-MISURA', now()) RETURNING conversazione_id`).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto)
		VALUES ('outlook', '<misura@acme.example>', $1, 'entrata', now(), 'RFQ misura') RETURNING messaggio_id`, convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, ricevuto_il)
		VALUES ($1, 1, 'disegno.pdf', 'pdf', 'file', 'outlook', 1000, $2, 'C:\staging\misura\disegno.pdf', now()) RETURNING allegato_id`,
		msgID, shaA15).Scan(&allID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte, stato)
		VALUES ($1, 'da_determinare', 40, 'estensione', 'aperta')`, allID); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: allID, Sha256: shaA15, Bytes: 1000, NomeFile: "disegno.pdf",
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash()})
	var jobID int64
	if err := pool.QueryRow(ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	codiceLungo := "1234567A" + strings.Repeat("X", classificazione.MaxCodice)
	dati, _ := json.Marshal(worker.RisultatoAnalisi{
		AllegatoID: allID, TipoProposto: "disegno_2d", Codice: codiceLungo, Rev: "REVISIONE_LUNGA_02",
		Confidenza: 90, Fonte: "cartiglio", Dettagli: json.RawMessage(`{"cartiglio":true}`),
		VersioneAnalizzatore: 1, HashConfigurazione: an.Hash(),
	})
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := s.applicaRisultato(ctx, db.New(tx), &j, dati, nil); err != nil {
		t.Fatalf("il result con un codice fuori misura non e' applicabile: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	p := propostaDi(t, pool, allID)
	if p.TipoProposto != db.TipoDocumentoDisegno2d || p.Confidenza != 90 {
		t.Errorf("il resto del risultato non e' stato applicato: %s %d", p.TipoProposto, p.Confidenza)
	}
	if p.Codice.Valid || p.Rev.Valid {
		t.Errorf("valori fuori misura scritti nelle colonne: codice=%q rev=%q", p.Codice.String, p.Rev.String)
	}
	var dett map[string]any
	_ = json.Unmarshal(p.Dettagli, &dett)
	if dett["codice_scartato"] != codiceLungo || dett["rev_scartata"] != "REVISIONE_LUNGA_02" || dett["cartiglio"] != true {
		t.Errorf("i valori grezzi non sono nei dettagli, o i dettagli del worker sono andati persi: %v", dett)
	}
}

type propostaLetta struct {
	TipoProposto db.TipoDocumento
	Codice       pgtype.Text
	Rev          pgtype.Text
	Confidenza   int16
	Dettagli     []byte
}

// propostaDi legge la proposta di un allegato cosi' com'e' in tabella.
func propostaDi(t *testing.T, pool *pgxpool.Pool, allegatoID uuid.UUID) propostaLetta {
	t.Helper()
	var p propostaLetta
	if err := pool.QueryRow(context.Background(), `SELECT tipo_proposto, codice, rev, confidenza, dettagli
		FROM documento_proposta WHERE allegato_id = $1`, allegatoID).Scan(&p.TipoProposto, &p.Codice, &p.Rev, &p.Confidenza, &p.Dettagli); err != nil {
		t.Fatalf("proposta dell'allegato %s: %v", allegatoID, err)
	}
	return p
}
