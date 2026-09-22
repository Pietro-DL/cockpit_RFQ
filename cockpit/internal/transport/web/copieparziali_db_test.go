//go:build integrazione

// L4 — blocco 5A: un documento che non si puo' accodare non ferma gli altri.
//
// E' il primo principio del checkpoint applicato qui: un singolo elemento anomalo non deve poter
// fermare il lavoro di tutti gli altri. Prima, se l'accodamento di UN documento falliva, l'intera
// azione usciva con un errore 500: le copie degli altri documenti della stessa RFQ — che non avevano
// niente che non andasse — non partivano, la pagina non cambiava, e all'operatore restava una
// schermata identica a prima senza nessuna spiegazione.
//
// L'errore qui e' vero e viene dal database: un trigger rifiuta l'inserimento del job di UN
// documento. Non e' un finto errore iniettato nel codice — e' il database che dice di no, che e' la
// forma che questo guasto ha davvero.
package web

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
)

// quotaSQL e' un letterale di testo per PostgreSQL. Serve perche' il corpo di una funzione plpgsql
// non ha parametri $1: la chiave va scritta dentro. Il valore e' un uuid generato dal test.
func quotaSQL(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// rifiutaLAccodamentoDi fa dire di no al database quando si accoda la copia di QUEL documento.
func (b *bancoWeb) rifiutaLAccodamentoDi(doc uuid.UUID) {
	b.t.Helper()
	corpo := fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION prova_rifiuta_job() RETURNS trigger LANGUAGE plpgsql AS $F$
		BEGIN
			IF NEW.chiave_idempotenza = %s THEN
				RAISE EXCEPTION 'prova: questo job non si accoda';
			END IF;
			RETURN NEW;
		END $F$`, quotaSQL(coda.ChiaveCopia(doc)))
	if _, err := b.pool.Exec(b.ctx, corpo); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `CREATE TRIGGER prova_rifiuta_job BEFORE INSERT ON job
		FOR EACH ROW EXECUTE FUNCTION prova_rifiuta_job()`); err != nil {
		b.t.Fatal(err)
	}
	b.t.Cleanup(func() {
		_, _ = b.pool.Exec(b.ctx, `DROP TRIGGER IF EXISTS prova_rifiuta_job ON job`)
	})
}

func TestUnDocumentoCheNonSiAccodaNonFermaGliAltri(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, docs := b.rfqConDocumentoInCoda(3)
	b.rifiutaLAccodamentoDi(docs[1])

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	resp, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)

	if resp.StatusCode != 200 {
		t.Fatalf("un documento storto ha fatto uscire l'intera azione: HTTP %d", resp.StatusCode)
	}
	if n := len(b.copieInCoda()); n != 2 {
		t.Errorf("copie accodate: %d, attese 2 (il terzo documento non deve fermare gli altri due)", n)
	}
	if !strings.Contains(html, "2 copie rimesse in coda") {
		t.Errorf("l'avviso non dice quante copie sono partite:\n%s", estrai(html, "avviso"))
	}
	if !strings.Contains(html, "1 non accodato") {
		t.Errorf("l'avviso tace sul documento che non si e' potuto accodare:\n%s", estrai(html, "avviso"))
	}
	if !strings.Contains(html, "questo job non si accoda") {
		t.Errorf("il motivo del rifiuto non arriva in pagina: va cercato nel log del server\n%s", estrai(html, "avviso"))
	}
}
