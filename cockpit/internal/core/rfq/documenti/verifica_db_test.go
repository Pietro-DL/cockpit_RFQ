//go:build integrazione

// L4 — la verifica di chi sta per servire i byte (blocco 8, B8.1), contro un PostgreSQL vero e file
// veri su disco.
//
// La prima prova qui sotto non prova il codice nuovo: FISSA IL COMPORTAMENTO DI QUELLO VECCHIO.
// `AllineaDocumento` su un conflitto rifiuta e non lascia niente scritto, e siccome l'anteprima
// arriva su quel documento senza che nessuno stia guardando quel documento, delegargli la verifica
// vorrebbe dire scoprire un conflitto e perderlo. Se un giorno «Allinea» imparera' ad aprire
// l'anomalia da solo, questa prova fallira': allora va riscritta insieme alla decisione, non
// cancellata per farla tornare verde.
package documenti

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

const contenutoAltrui = "questo e' il disegno di qualcun altro, della stessa RFQ o di un'altra"

// bancoConflitto prepara un documento «scritto» il cui file sul NAS NON e' quello del fascicolo, e
// restituisce la radice, il documento e il percorso assoluto del file.
func bancoConflitto(t *testing.T, ctx context.Context, p *pgxpool.Pool, q *db.Queries, suffisso, contenuto string) (string, uuid.UUID, string) {
	t.Helper()
	doc, _ := bancoIntegrita(t, ctx, p, suffisso)
	if _, err := p.Exec(ctx, `UPDATE documento SET stato_nas='scritto', scritto_il=now() WHERE documento_id=$1`, doc); err != nil {
		t.Fatal(err)
	}
	radice := t.TempDir()
	dst := destinazione(t, ctx, q, radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(contenuto), 0o644); err != nil {
		t.Fatal(err)
	}
	return radice, doc, dst
}

func anomalie(t *testing.T, ctx context.Context, p *pgxpool.Pool, doc uuid.UUID) int {
	t.Helper()
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*)::int FROM nas_anomalia WHERE documento_id=$1`, doc).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Il comportamento di oggi di «Allinea», guardato e fissato: su un conflitto non lascia traccia.
func TestAllineaDocumentoOggiNonApreNessunaAnomalia(t *testing.T) {
	p, q, ctx := preparaDB(t)
	radice, doc, _ := bancoConflitto(t, ctx, p, q, "V1", contenutoAltrui)

	err := AllineaDocumento(ctx, q, &nas.Scrittore{Radice: radice}, doc)
	if err == nil {
		t.Fatal("un file diverso e' stato dichiarato «scritto»")
	}
	if !strings.Contains(err.Error(), "conflitto") {
		t.Errorf("l'errore non dice che si tratta di un conflitto: %v", err)
	}
	if n := anomalie(t, ctx, p, doc); n != 0 {
		t.Fatalf("«Allinea» ha aperto %d anomalie: il comportamento e' cambiato, e l'anteprima che si "+
			"appoggiava a questa prova va rivista insieme alla decisione", n)
	}
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.VerificatoIl != nil {
		t.Error("«Allinea» ha segnato il documento come verificato dopo aver trovato un conflitto")
	}
}

// La verifica dell'anteprima, sullo stesso file, lascia la scoperta scritta: e' l'unica differenza
// che conta fra le due, perche' qui nessuno sta guardando quel documento in particolare.
func TestLaVerificaDellAnteprimaApreLAnomaliaSuUnConflitto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	_, doc, dst := bancoConflitto(t, ctx, p, q, "V2", contenutoAltrui)
	shaAltrui, _, err := nas.Sha256File(dst)
	if err != nil {
		t.Fatal(err)
	}
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = VerificaFileAperto(ctx, q, d, `ACME\WIP\prova\disegno.pdf`, f)
	if !errors.Is(err, ErrNonCorrisponde) {
		t.Fatalf("errore = %v, atteso ErrNonCorrisponde", err)
	}
	a, err := q.GetAnomaliaNas(ctx, doc)
	if err != nil {
		t.Fatalf("nessuna anomalia aperta dopo aver trovato un file diverso: la scoperta e' andata persa (%v)", err)
	}
	if a.Problema != db.ProblemaNasConflitto {
		t.Errorf("problema = %q, atteso conflitto", a.Problema)
	}
	if a.ShaTrovato.String != shaAltrui {
		t.Errorf("sha_trovato = %q, atteso quello del file che c'e' davvero (%q)", a.ShaTrovato.String, shaAltrui)
	}
	if a.ShaAtteso != d.Sha256 {
		t.Errorf("sha_atteso = %q, atteso quello del documento (%q)", a.ShaAtteso, d.Sha256)
	}
	if a.Percorso != `ACME\WIP\prova\disegno.pdf` {
		t.Errorf("l'anomalia non dice dove si e' guardato: %q", a.Percorso)
	}
	// Il documento resta com'era: la verifica non ripara e non degrada niente.
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Errorf("stato_nas = %q, atteso scritto (invariato)", s)
	}
	// `verificato_il` NON si tocca: un documento guardato e trovato sbagliato non deve finire in
	// fondo alla coda del ricognitore, che ordina proprio per quel campo. La segnalazione e' il
	// racconto dell'esame; questo campo e' «l'ultima volta che l'abbiamo guardato ed era a posto».
	if dopo, err := q.GetDocumento(ctx, doc); err != nil {
		t.Fatal(err)
	} else if dopo.VerificatoIl != nil {
		t.Error("il documento risulta verificato, e la verifica aveva trovato un altro file")
	}
	// Il lettore e' riavvolto: chi verifica e' lo stesso che serve, e deve poter partire da zero.
	if pos, err := f.Seek(0, io.SeekCurrent); err != nil || pos != 0 {
		t.Errorf("il file non e' riavvolto dopo la verifica (posizione %d, err %v)", pos, err)
	}
}

// Quando il file e' quello giusto la verifica chiude la segnalazione: non resta un allarme spento da
// spegnere a mano.
func TestLaVerificaChiudeLaSegnalazioneQuandoIlFileEQuelloGiusto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	_, doc, dst := bancoConflitto(t, ctx, p, q, "V3", contenutoProva)
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := Segnala(ctx, q, d, `ACME\prova.pdf`, db.ProblemaNasMancante, "", "segnalato prima"); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := VerificaFileAperto(ctx, q, d, `ACME\prova.pdf`, f); err != nil {
		t.Fatalf("il file e' byte per byte quello del documento: %v", err)
	}
	if _, err := q.GetAnomaliaNas(ctx, doc); err == nil {
		t.Error("la segnalazione e' rimasta aperta su un file che corrisponde")
	}
	dopo, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if dopo.VerificatoIl == nil {
		t.Error("il documento e' stato riletto per intero e non risulta verificato: il ricognitore lo rileggera' come se nessuno l'avesse guardato")
	}
}

// lettoreRotto legge qualche byte e poi si interrompe: e' la condivisione di rete che cade a meta'
// file, che capita e non e' un conflitto.
type lettoreRotto struct {
	letti int
	fino  int
}

func (l *lettoreRotto) Read(p []byte) (int, error) {
	if l.letti >= l.fino {
		return 0, errors.New("la connessione con il NAS e' caduta")
	}
	n := len(p)
	if n > l.fino-l.letti {
		n = l.fino - l.letti
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	l.letti += n
	return n, nil
}

func (l *lettoreRotto) Seek(int64, int) (int64, error) { l.letti = 0; return 0, nil }

// Una lettura interrotta non e' un conflitto: dire «conflitto» qui vorrebbe dire accusare qualcuno di
// aver sostituito un file sulla base di un cavo di rete.
func TestUnaLetturaInterrottaNonEUnConflitto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	_, doc, _ := bancoConflitto(t, ctx, p, q, "V4", contenutoProva)
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}

	err = VerificaFileAperto(ctx, q, d, `ACME\prova.pdf`, &lettoreRotto{fino: 7})
	if err == nil {
		t.Fatal("una lettura interrotta e' passata per una verifica riuscita")
	}
	if errors.Is(err, ErrNonCorrisponde) {
		t.Errorf("una lettura interrotta e' stata chiamata conflitto: %v", err)
	}
	a, err := q.GetAnomaliaNas(ctx, doc)
	if err != nil {
		t.Fatalf("nessuna anomalia aperta dopo una lettura interrotta: %v", err)
	}
	if a.Problema != db.ProblemaNasIlleggibile {
		t.Errorf("problema = %q, atteso illeggibile", a.Problema)
	}
	if a.ShaTrovato.Valid {
		t.Errorf("l'anomalia dichiara un hash trovato (%q) che nessuno ha potuto calcolare", a.ShaTrovato.String)
	}
}
