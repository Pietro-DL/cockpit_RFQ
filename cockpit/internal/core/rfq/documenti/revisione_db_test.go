//go:build integrazione

// L4 — le correzioni della revisione del 25/09 sul fascicolo sul NAS: una copia vecchia non abbassa un
// documento gia' scritto, una copia che finisce durante la passata del ricognitore non apre una falsa
// anomalia, la ripresa di un contenuto senza job non si ferma, e un file caricato a mano non manda a
// cercare «Riscarica» in Outlook.

package documenti

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

// contenutoDelDocumento e' il file nella cache che la copia del documento usa.
func contenutoDelDocumento(t *testing.T, ctx context.Context, p *pgxpool.Pool, sha string) (uuid.UUID, string) {
	t.Helper()
	var id uuid.UUID
	var percorso string
	if err := p.QueryRow(ctx, `SELECT allegato_id, path_staging FROM allegato WHERE sha256 = $1`, sha).Scan(&id, &percorso); err != nil {
		t.Fatal(err)
	}
	return id, percorso
}

func contaJobStage(t *testing.T, ctx context.Context, p *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM job WHERE chiave_idempotenza LIKE 'stage:%' OR chiave_idempotenza LIKE 'estrai:%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Una copia_nas esaurita e riaccodata all'avvio, per un documento gia' scritto il cui contenuto il custode
// ha tolto dalla cache: prima partiva la ripresa, falliva, e SetDocumentoErrore metteva in errore un
// documento che sul NAS era a posto. Adesso il file e' li' con l'hash giusto, e la copia finisce subito.
func TestUnaCopiaVecchiaNonMetteInErroreUnDocumentoGiaScritto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, tutto)
	doc, sha := bancoIntegrita(t, ctx, p, "V")
	scr := &nas.Scrittore{Radice: t.TempDir()}
	if _, err := CopiaSulNas(ctx, q, scr, nil, doc); err != nil {
		t.Fatalf("la prima copia: %v", err)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Fatalf("dopo la prima copia: %s", s)
	}
	_, percorso := contenutoDelDocumento(t, ctx, p, sha)
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}

	res, err := CopiaSulNas(ctx, q, scr, &db.Job{CreatoIl: time.Now()}, doc)
	if err != nil {
		t.Fatalf("la copia di un documento gia' scritto, con il file sul NAS, e' fallita: %v", err)
	}
	if m, _ := res.(map[string]any); m["gia_scritto"] != true {
		t.Errorf("l'esito non dice che il documento era gia' scritto: %v", res)
	}
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.StatoNas != db.StatoNasScritto || d.ErroreNas.Valid {
		t.Errorf("il documento e' diventato %s (%q)", d.StatoNas, d.ErroreNas.String)
	}
	if n := contaJobStage(t, ctx, p); n != 0 {
		t.Errorf("per un documento gia' sul NAS e' partita la ripresa del contenuto (%d job)", n)
	}
}

// E se il file sul NAS manca davvero e il contenuto non c'e' piu', la copia fallisce — ma il documento resta
// «scritto»: SetDocumentoErrore non lo abbassa. Che il file manchi lo dice il ricognitore, con la sua
// anomalia.
func TestSetDocumentoErroreNonAbbassaUnoScritto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, tutto)
	doc, sha := bancoIntegrita(t, ctx, p, "W")
	if _, err := p.Exec(ctx, `UPDATE documento SET stato_nas = 'scritto', scritto_il = now() WHERE documento_id = $1`, doc); err != nil {
		t.Fatal(err)
	}
	_, percorso := contenutoDelDocumento(t, ctx, p, sha)
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := CopiaSulNas(ctx, q, &nas.Scrittore{Radice: t.TempDir()}, nil, doc); !errors.Is(err, ErrContenutoMancante) {
		t.Fatalf("senza il file sul NAS e senza il contenuto la copia deve fallire: %v", err)
	}
	if err := q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{DocumentoID: doc, ErroreNas: ptesto("prova")}); err != nil {
		t.Fatal(err)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Errorf("un documento scritto e' diventato %s", s)
	}
	// su un documento ancora da copiare l'errore invece si scrive, come prima
	altro, _ := bancoIntegrita(t, ctx, p, "X")
	if err := q.SetDocumentoErrore(ctx, db.SetDocumentoErroreParams{DocumentoID: altro, ErroreNas: ptesto("prova")}); err != nil {
		t.Fatal(err)
	}
	if s := statoNas(t, ctx, q, altro); s != db.StatoNasErrore {
		t.Errorf("un documento in coda con un errore di copia: %s, atteso errore", s)
	}
}

// Il ricognitore legge le righe all'inizio della passata e i file dopo: una copia che finisce in mezzo
// lasciava la riga vecchia («in coda») accanto al file giusto, e la passata apriva «gia' presente» su un
// documento appena scritto, fermando gate e anteprima fino alla passata dopo.
func TestUnaCopiaCheFinisceDuranteLaPassataNonApreUnaFalsaAnomalia(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, _ := bancoIntegrita(t, ctx, p, "R")
	radice := t.TempDir()
	dst := destinazione(t, ctx, q, radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(contenutoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: radice}, Log: testutil.LogSilenzioso()}
	r.mentreGuarda = func(d db.Documento) {
		// la copia finisce adesso: fra la lettura della riga e la scrittura dell'esito
		if _, err := q.SetDocumentoScritto(ctx, db.SetDocumentoScrittoParams{DocumentoID: d.DocumentoID, PathRelativo: d.PathRelativo}); err != nil {
			t.Error(err)
		}
	}
	e, err := r.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if e.Aperte != 0 {
		t.Errorf("la passata ha aperto %d anomalie (%v) su un documento appena scritto", e.Aperte, e.PerProblema)
	}
	if _, err := q.GetAnomaliaNas(ctx, doc); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("c'e' un'anomalia aperta: %v", err)
	}
}

// Una ripresa chiamata fuori dalla coda (j nil) con un download gia' fallito in passato: prima era un
// panic su j.CreatoIl.
func TestLaRipresaSenzaJobNonSiFerma(t *testing.T) {
	p, q, ctx := preparaDB(t)
	_, sha := bancoIntegrita(t, ctx, p, "N")
	allegato, _ := contenutoDelDocumento(t, ctx, p, sha)
	if _, err := p.Exec(ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato, errore, chiuso_il)
		VALUES ('stage_allegato','outlook','{}',$1,'fallito','elemento non trovato in Outlook', now())`, "stage:"+allegato.String()); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetAllegato(ctx, allegato)
	if err != nil {
		t.Fatal(err)
	}
	originale := errors.New("contenuto sparito")
	err = RiprendiContenuto(ctx, q, nil, a, originale)
	if !errors.Is(err, originale) || !strings.Contains(err.Error(), "nessuna casella attiva") {
		t.Errorf("la ripresa senza job: %v", err)
	}
}

// Un file caricato a mano nel Fascicolo non e' in Outlook: se il suo contenuto sparisce dalla cache il
// messaggio dice di ricaricarlo dal Fascicolo, non di premere «Riscarica», e non accoda un download che
// non puo' riuscire.
func TestUnFileCaricatoAManoSiRicaricaDalFascicolo(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, tutto)
	doc, sha := bancoIntegrita(t, ctx, p, "M")
	_, percorso := contenutoDelDocumento(t, ctx, p, sha)
	if _, err := p.Exec(ctx, `UPDATE allegato SET origine = 'manuale' WHERE sha256 = $1`, sha); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}
	_, err := CopiaSulNas(ctx, q, &nas.Scrittore{Radice: t.TempDir()}, &db.Job{CreatoIl: time.Now()}, doc)
	if !errors.Is(err, ErrContenutoMancante) {
		t.Fatalf("errore: %v, atteso contenuto mancante", err)
	}
	if !strings.Contains(err.Error(), "Fascicolo") || strings.Contains(err.Error(), "Riscarica") {
		t.Errorf("il messaggio per un file caricato a mano: %v", err)
	}
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.ErroreNas.String, "si ricarica il file dal Fascicolo") {
		t.Errorf("il fascicolo non dice di ricaricare il file: %q", d.ErroreNas.String)
	}
	if n := contaJobStage(t, ctx, p); n != 0 {
		t.Errorf("per un file caricato a mano e' stato accodato un download da Outlook (%d job)", n)
	}
}
