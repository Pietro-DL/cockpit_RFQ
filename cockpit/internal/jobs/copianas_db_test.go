//go:build integrazione

// L4 — blocco 5B: i due casi della riconciliazione che passano dall'ESECUTORE.
//
//	documento in_coda → Riprova → scritto
//	hash diverso → conflitto, e il file NON viene toccato
//
// Gli altri tre casi del checkpoint — file gia' corretto, file assente ma DB «scritto», NAS
// irraggiungibile — provano il RICOGNITORE e stanno con lui, in `core/rfq/documenti`. Questi due
// stanno qui perche' fanno eseguire davvero il job di copia, e l'esecutore e' qui.
//
// bancoIntegrita, destinazione e statoNas sono ripetuti di la': impalcatura di prova, non codice.
package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

const contenutoProva = "questo e' il disegno vero"

// bancoIntegrita prepara una RFQ con un documento confermato il cui contenuto e' in staging, con
// l'hash VERO del contenuto: la copia sul NAS verifica l'hash dopo aver copiato, e un hash finto
// farebbe fallire la copia per il motivo sbagliato.
func bancoIntegrita(t *testing.T, ctx context.Context, p *pgxpool.Pool, suffisso string) (uuid.UUID, string) {
	t.Helper()
	staging := t.TempDir()
	percorso := filepath.Join(staging, "disegno"+suffisso+".pdf")
	if err := os.WriteFile(percorso, []byte(contenutoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := nas.Sha256File(percorso)
	if err != nil {
		t.Fatal(err)
	}

	var cliente, thread, conv, msg, allegato, utente, documento uuid.UUID
	deve := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	deve(p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1,$2) RETURNING cliente_id`,
		"ACME"+suffisso, "Acme "+suffisso).Scan(&cliente))
	deve(p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ integrita',$2,1) RETURNING thread_id`,
		cliente, `ACME`+suffisso+`\WIP\2026 09 17 prova`).Scan(&thread))
	deve(p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook',$1,now()) RETURNING conversazione_id`, "CONV-"+suffisso).Scan(&conv))
	deve(p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook',$1,$2,'entrata',now(),'prova',$3) RETURNING messaggio_id`,
		"MSG-"+suffisso, conv, thread).Scan(&msg))
	deve(p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'disegno.pdf','pdf','file',$2,$3,$4,'analizzato',now()) RETURNING allegato_id`,
		msg, len(contenutoProva), sha, percorso).Scan(&allegato))
	deve(p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1,'Prova','tecnico','operatore') RETURNING utente_id`,
		"I"+suffisso).Scan(&utente))
	deve(p.QueryRow(ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da, confermato_il)
		VALUES ($1,'disegno_2d','disegno.pdf','pdf',$2,$3,$4,'in_coda',$5, now() - interval '3 days') RETURNING documento_id`,
		thread, sha, len(contenutoProva), `ELENCO DISEGNI\disegno.pdf`, utente).Scan(&documento))
	return documento, sha
}

// destinazione e' il percorso assoluto del file sul NAS di questo documento.
func destinazione(t *testing.T, ctx context.Context, q *db.Queries, radice string, doc uuid.UUID) string {
	t.Helper()
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	th, err := q.GetThread(ctx, d.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(radice, filepath.FromSlash(strings.ReplaceAll(th.CartellaRelativa.String, `\`, "/")),
		filepath.FromSlash(strings.ReplaceAll(d.PathRelativo, `\`, "/")))
}

func statoNas(t *testing.T, ctx context.Context, q *db.Queries, doc uuid.UUID) db.StatoNas {
	t.Helper()
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	return d.StatoNas
}

// eseguiLaCopia prende dalla coda il job di copia e lo esegue: e' la stessa strada del server vero.
func eseguiLaCopia(t *testing.T, ctx context.Context, q *db.Queries, e *EsecutoreServer) error {
	t.Helper()
	j, err := coda.Claim(ctx, q, db.WorkerTipoServer, "prova", coda.Destinazione{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if j == nil {
		t.Fatal("nessun job da eseguire: la copia non e' stata accodata")
	}
	if j.Tipo != db.TipoJobCopiaNas {
		t.Fatalf("job di tipo %s, atteso copia_nas", j.Tipo)
	}
	_, err = e.esegui(ctx, q, j, coda.Tentativo{JobID: j.JobID, LeaseToken: j.LeaseToken.UUID, WorkerID: "prova"})
	return err
}

// 1. Il caso normale del checkpoint: documento in_coda → si riaccoda → il file e' sul NAS.
func TestInCodaRiaccodatoDiventaScritto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	doc, sha := bancoIntegrita(t, ctx, p, "A")
	radice := t.TempDir()

	if _, err := coda.AccodaCopia(ctx, q, doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: radice}}
	if err := eseguiLaCopia(t, ctx, q, e); err != nil {
		t.Fatalf("la copia non e' riuscita: %v", err)
	}

	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Errorf("stato_nas = %q, atteso scritto", s)
	}
	dst := destinazione(t, ctx, q, radice, doc)
	trovato, _, err := nas.Sha256File(dst)
	if err != nil {
		t.Fatalf("il file non e' sul NAS: %v", err)
	}
	if trovato != sha {
		t.Errorf("sul NAS c'e' un file con un altro hash: %s invece di %s", trovato, sha)
	}
}

// 3. Hash diverso: e' un conflitto, e il file NON viene toccato. Ne' dal ricognitore, ne' da una
// copia riaccodata sopra.
func TestHashDiversoEeUnConflittoEIlFileNonSiTocca(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	doc, _ := bancoIntegrita(t, ctx, p, "C")
	radice := t.TempDir()
	dst := destinazione(t, ctx, q, radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("la revisione che ha messo li' qualcun altro"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &documenti.Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: radice}, Log: testutil.LogSilenzioso()}

	if _, err := r.Giro(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetAnomaliaNas(ctx, doc)
	if err != nil {
		t.Fatalf("un file diverso da quello del documento non viene segnalato: %v", err)
	}
	if a.Problema != db.ProblemaNasConflitto {
		t.Fatalf("problema = %q, atteso conflitto", a.Problema)
	}
	if !a.ShaTrovato.Valid || a.ShaTrovato.String == a.ShaAtteso {
		t.Errorf("l'anomalia non registra l'hash trovato: il conflitto non sarebbe verificabile da nessuno")
	}

	// riaccodare la copia e' permesso, ma la copia NON deve sovrascrivere
	if _, err := coda.AccodaCopia(ctx, q, doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: radice}}
	if err := eseguiLaCopia(t, ctx, q, e); err == nil {
		t.Error("la copia sopra un file diverso e' andata a buon fine")
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != "la revisione che ha messo li' qualcun altro" {
		t.Fatalf("il file di qualcun altro e' stato sovrascritto: %q (%v)", string(b), err)
	}
}
