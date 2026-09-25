//go:build integrazione

// L4 — blocco 5A: la conferma e «Riprova copie» accodano la STESSA copia, non due.
//
// I punti che mettono in coda una `copia_nas` sono diventati tre — la conferma di una proposta,
// «Riprova copie» sulla RFQ, la riconciliazione dell'integrita' — e l'unica cosa che impedisce di
// copiare due volte lo stesso file e' che tutti e tre usino la stessa chiave di idempotenza.
//
// Se una delle tre la componesse in modo anche solo leggermente diverso, non si romperebbe niente:
// le due copie partirebbero tutte e due, tutte e due riuscirebbero (la destinazione e' la stessa e la
// seconda troverebbe il file gia' li' con l'hash giusto), e il difetto resterebbe invisibile finche'
// qualcuno non guarda la coda. E' per questo che la chiave e' una funzione sola, e per questo la
// prova attraversa DUE strade diverse fino allo stesso documento.
package web

import (
	htmlesc "html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// leggibile e' l'HTML come lo legge una persona: il template scrive «gia&#39;», non «gia'», e un test
// che cerca la frase cosi' come e' scritta nel codice non la troverebbe mai.
func leggibile(html string) string { return htmlesc.UnescapeString(html) }

// propostaDaConfermare prepara una RFQ con un allegato vero in staging e la proposta aperta che lo
// riguarda. Restituisce thread, messaggio e proposta.
func (b *bancoWeb) propostaDaConfermare(chiave string) (uuid.UUID, uuid.UUID, uuid.UUID) {
	b.t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook', now(), 'RFQ chiave', 'ACME\WIP\2026 09 17 chiave', 1) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		b.t.Fatal(err)
	}
	msg := b.messaggioIn(chiave)
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, msg, thread); err != nil {
		b.t.Fatal(err)
	}
	percorso := filepath.Join(b.t.TempDir(), "1234567A_4.pdf")
	if err := os.WriteFile(percorso, []byte("contenuto di prova"), 0o644); err != nil {
		b.t.Fatal(err)
	}
	var allegato, proposta uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'1234567A_4.pdf','pdf','file','outlook',18,repeat('a',64),$2,'analizzato',now())
		RETURNING allegato_id`, msg, percorso).Scan(&allegato); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte)
		VALUES ($1,$2,'disegno_2d','1234567A','4',90,'nome_file') RETURNING proposta_id`, allegato, thread).Scan(&proposta); err != nil {
		b.t.Fatal(err)
	}
	return thread, msg, proposta
}

func TestConfermaERiprovaCopieNonAccodanoDueVolteLoStessoDocumento(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, _, proposta := b.propostaDaConfermare("K1")

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")

	// prima strada: la conferma della proposta accoda la copia
	_, html := w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", url.Values{}, true)
	if !strings.Contains(html, "copia sul NAS in coda") {
		t.Fatalf("la conferma non ha accodato nessuna copia:\n%s", estrai(html, "avviso"))
	}
	if n := len(b.copieInCoda()); n != 1 {
		t.Fatalf("copie in coda dopo la conferma: %d, attesa 1", n)
	}

	// seconda strada: «Riprova copie» sulla stessa RFQ non ne accoda una seconda
	_, html = w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if n := len(b.copieInCoda()); n != 1 {
		t.Errorf("copie in coda dopo «Riprova copie»: %d, attesa 1. Le due strade non usano la stessa chiave: "+
			"lo stesso file verrebbe copiato due volte e nessuno se ne accorgerebbe", n)
	}
	if !strings.Contains(leggibile(html), "1 gia' in attesa") {
		t.Errorf("l'avviso non riconosce la copia gia' accodata dalla conferma:\n%s", estrai(html, "avviso"))
	}
	if n := len(b.jobDiTipo(db.TipoJobCopiaNas)); n != 1 {
		t.Errorf("job copia_nas totali: %d", n)
	}
}
