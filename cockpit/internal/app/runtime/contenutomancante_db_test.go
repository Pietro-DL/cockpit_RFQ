//go:build integrazione

// L4 — blocco 4C: un contenuto sparito dallo staging non è un vicolo cieco.
//
// È emerso dalla prova reale sul NAS di test. Due documenti confermati il 17/09 mentre
// `nas_scrittura` era spenta: «Riprova copie» li ha rimessi in coda correttamente, la copia è
// partita, e si è fermata così:
//
//	open C:\...\_staging\_contenuti\73\739f....pdf: Impossibile trovare il percorso specificato.
//
// Il contenuto non c'era più. Il documento è finito in `errore` con quella frase dentro, e da lì non
// si muoveva più: la frase dice dove il file non c'era e non dice a nessuno che cosa fare, e
// «Riprova copie» guardava solo i documenti `in_coda`, quindi quello in errore non lo riprendeva
// nemmeno. Due vicoli ciechi in fila, dopo quello che il pulsante era nato per togliere.
//
// `path_staging` dice dove il contenuto è stato messo, non che ci sia ancora: fra la conferma e la
// copia possono passare giorni, e in mezzo ci sono la pulizia dello staging, un disco rifatto, una
// cartella svuotata. L'originale però è ancora in Outlook, e si riprende con «Riscarica».
package runtime

import (
	"context"
	"errors"
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
)

// documentoConContenuto prepara una RFQ con un allegato in staging e il documento confermato che lo
// nomina. Restituisce il documento e il percorso del file sul disco.
func documentoConContenuto(t *testing.T, ctx context.Context, p *pgxpool.Pool, sha string) (uuid.UUID, string) {
	t.Helper()
	staging := t.TempDir()
	percorso := filepath.Join(staging, sha+".pdf")
	if err := os.WriteFile(percorso, []byte("contenuto di prova"), 0o644); err != nil {
		t.Fatal(err)
	}

	var cliente, thread, conversazione, messaggio, allegato, documento, utente uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale)
		VALUES ('ACME','Acme S.p.A.') RETURNING cliente_id`).Scan(&cliente); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ contenuti', 'ACME\WIP\2026 09 17 prova', 1) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook',$1,now()) RETURNING conversazione_id`, "CONV-"+sha[:8]).Scan(&conversazione); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook',$1,$2,'entrata',now(),'prova',$3) RETURNING messaggio_id`,
		"MSG-"+sha[:8], conversazione, thread).Scan(&messaggio); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'disegno.pdf','pdf','file',18,$2,$3,'analizzato',now()) RETURNING allegato_id`,
		messaggio, sha, percorso).Scan(&allegato); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo)
		VALUES ($1,'Prova','tecnico','operatore') RETURNING utente_id`, "U"+sha[:3]).Scan(&utente); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1,'disegno_2d','disegno.pdf','pdf',$2,18,'2D\disegno.pdf','in_coda',$3) RETURNING documento_id`,
		thread, sha, utente).Scan(&documento); err != nil {
		t.Fatal(err)
	}
	return documento, percorso
}

// Finché il file c'è, si copia: nessun cambiamento per il caso normale.
func TestIlContenutoCEeSiCopia(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, percorso := documentoConContenuto(t, ctx, p, strings.Repeat("a", 64))

	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	src, err := documenti.SorgenteStaging(ctx, q, d)
	if err != nil {
		t.Fatalf("il contenuto c'è e non viene trovato: %v", err)
	}
	if src != percorso {
		t.Errorf("sorgente = %q, atteso %q", src, percorso)
	}
}

// Se il file non c'è più, l'errore lo DICE e dice anche come si riprende. Non è un guasto del NAS e
// non si risolve riprovando: il file da copiare non esiste.
func TestUnContenutoSparitoLoDiceEDiceCosaFare(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, percorso := documentoConContenuto(t, ctx, p, strings.Repeat("b", 64))
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}

	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = documenti.SorgenteStaging(ctx, q, d)
	if err == nil {
		t.Fatal("il contenuto non c'è e la copia parte lo stesso: si fermerà con un errore del filesystem")
	}
	if !errors.Is(err, documenti.ErrContenutoMancante) {
		t.Errorf("l'errore non è riconoscibile come «contenuto mancante»: %v", err)
	}
	// le tre cose che deve dire: quale file, che non è più in staging, e da dove si riprende
	for _, parola := range []string{"disegno.pdf", "staging", "Riscarica"} {
		if !strings.Contains(err.Error(), parola) {
			t.Errorf("l'errore non nomina %q: %v", parola, err)
		}
	}
}

// Una cartella con il nome del file non è il file: non si copia una directory.
func TestUnaCartellaAlPostoDelFileNonEeIlContenuto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, percorso := documentoConContenuto(t, ctx, p, strings.Repeat("c", 64))
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(percorso, 0o755); err != nil {
		t.Fatal(err)
	}

	d, _ := q.GetDocumento(ctx, doc)
	if _, err := documenti.SorgenteStaging(ctx, q, d); !errors.Is(err, documenti.ErrContenutoMancante) {
		t.Errorf("una cartella è stata presa per il contenuto: %v", err)
	}
}

// Un documento che non ha proprio nessun allegato con quel hash è un caso diverso, e resta diverso:
// lì non c'è niente da riscaricare, c'è una riga che non torna.
func TestSenzaNessunAllegatoLErroreNonParlaDiRiscarica(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, percorso := documentoConContenuto(t, ctx, p, strings.Repeat("d", 64))
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE allegato SET sha256 = $1 WHERE sha256 = $2`,
		strings.Repeat("e", 64), strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}

	d, _ := q.GetDocumento(ctx, doc)
	_, err := documenti.SorgenteStaging(ctx, q, d)
	if err == nil {
		t.Fatal("un documento senza allegati corrispondenti non ha dato errore")
	}
	if errors.Is(err, documenti.ErrContenutoMancante) {
		t.Errorf("si consiglia «Riscarica» su un allegato che non esiste: %v", err)
	}
}

// Il motivo deve finire NEL FASCICOLO, non solo nel log del server.
//
// Nella prova reale la frase giusta — quale contenuto manca e che si riprende con «Riscarica» — e'
// finita nel log, mentre nella riga del documento restava l'errore del filesystem di ore prima. Chi
// aspetta quel disegno guarda la RFQ, non il log: delle due versioni, l'unica visibile era quella che
// non diceva niente.
func TestIlMotivoFinisceSulDocumentoNonSoloNelLog(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, percorso := documentoConContenuto(t, ctx, p, strings.Repeat("f", 64))
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}

	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	j := db.Job{Tipo: db.TipoJobCopiaNas, Payload: []byte(`{"documento_id":"` + doc.String() + `"}`)}
	if _, err := e.esegui(ctx, q, &j, coda.Tentativo{}); err == nil {
		t.Fatal("la copia di un contenuto sparito e' andata a buon fine")
	}

	var motivo, stato string
	if err := p.QueryRow(ctx, `SELECT coalesce(errore_nas,''), stato_nas::text FROM documento WHERE documento_id = $1`,
		doc).Scan(&motivo, &stato); err != nil {
		t.Fatal(err)
	}
	if motivo == "" {
		t.Fatal("il documento non porta nessun motivo: chi guarda la RFQ vede «errore» e basta")
	}
	if !strings.Contains(motivo, "Riscarica") {
		t.Errorf("il motivo sul documento non dice come si riprende: %q", motivo)
	}
	if stato != "errore" {
		t.Errorf("stato_nas = %q, atteso errore", stato)
	}
}
