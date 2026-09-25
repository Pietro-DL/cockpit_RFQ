//go:build integrazione

// L4 — le correzioni della revisione del 25/09 sui gesti del Fascicolo: un prodotto archiviato non lascia
// rimozioni aperte, le note lunghe si tagliano, lo STEP strutturale non si sceglie per un archiviato, gli
// scarti bloccano la RFQ come gli altri gesti, una deroga strutturale si revoca solo dalla sua RFQ, un
// arco messo a mano non si propone subito da togliere, D16 non riscrive una decisione dell'operatore.

package fascicolo_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// stepApplicato: la working P1 → B, C, D con lo STEP strutturale di P1 letto per intero, che propone di
// togliere P1 → D (nello STEP D sta sotto C) e di aggiungere F.
func (b *banco) stepApplicato() (map[string]uuid.UUID, uuid.UUID, db.Allegato) {
	b.t.Helper()
	c := b.workingPBCD()
	doc, a := b.stepDelProdotto(c["P1"], codiceDi("P1"), stepPBCD())
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, c["P1"], doc)
	if es := b.applica(a, stepPBCD().json()); !es.Rimozioni.Calcolate {
		b.t.Fatalf("lo STEP strutturale non e' letto per intero: %+v", es)
	}
	if got := b.rimozioni(); got != "P1>D:aperta" {
		b.t.Fatalf("rimozioni di partenza = %q, attesa P1>D:aperta", got)
	}
	return c, doc, a
}

// Un prodotto archiviato con lo STEP strutturale chiudeva niente: le sue rimozioni restavano aperte per
// sempre (il ricalcolo guarda solo i prodotti attivi) e il gate le contava fra le proposte da decidere.
func TestArchiviareUnProdottoChiudeLeRimozioniDelSuoStep(t *testing.T) {
	b := nuovoBanco(t)
	c, _, _ := b.stepApplicato()
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ArchiviaComponente(b.ctx, q, b.thread, c["P1"], b.utente, "il cliente non lo chiede piu'")
	})
	ok(t, err)
	if got := b.rimozioni(); got != "P1>D:scartata" {
		t.Errorf("rimozioni dopo l'archiviazione = %q, attesa P1>D:scartata", got)
	}
	if nota := uno[string](b, `SELECT coalesce(nota, '') FROM rimozione_proposta WHERE thread_id = $1`, b.thread); nota != "prodotto archiviato" {
		t.Errorf("nota della rimozione chiusa: %q", nota)
	}
	g, err := db.New(b.p).GateCongelamento(b.ctx, b.thread)
	ok(t, err)
	if g.NProposteRimozione != 0 {
		t.Errorf("il gate conta ancora %d rimozioni aperte del prodotto archiviato", g.NProposteRimozione)
	}
}

// Una nota costruita con il nome di un file («superata da <nome>», fino a 300 caratteri) finiva in una
// colonna da 200 e la sostituzione falliva con un errore grezzo del database.
func TestUnaNotaConUnNomeLungoNonFermaLaSostituzione(t *testing.T) {
	b := nuovoBanco(t)
	p1 := b.componente("P1", db.TipoComponenteFinito)
	f1 := b.componente("F1", db.TipoComponenteSciolto)
	b.arco(p1, f1, 1)
	s1 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	s2 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	b.scegliStep(p1, s1)
	b.esegui(`INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 1)`,
		b.thread, s1, p1, f1)
	lungo := strings.Repeat("è", 280) + ".stp"
	b.esegui(`UPDATE documento SET nome_file = $2 WHERE documento_id = $1`, s2, lungo)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Sostituisci(b.ctx, q, b.thread, s1, s2, false)
	})
	ok(t, err)
	nota := uno[string](b, `SELECT nota FROM rimozione_proposta WHERE thread_id = $1`, b.thread)
	if utf8.RuneCountInString(nota) != 200 || !strings.HasPrefix(nota, "superata da èèè") {
		t.Errorf("nota di %d caratteri: %q", utf8.RuneCountInString(nota), nota)
	}
}

// Lo STEP strutturale di un archiviato non si sceglie: le rimozioni che il nuovo riferimento aprirebbe
// non le ricalcolerebbe nessuno. Lo stesso per la sostituzione che sposta il riferimento; la sostituzione
// semplice resta possibile.
func TestLoStepStrutturaleNonSiScegliePerUnArchiviato(t *testing.T) {
	b := nuovoBanco(t)
	p1 := b.componente("P1", db.TipoComponenteFinito)
	s1 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	s2 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	b.scegliStep(p1, s1)
	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ArchiviaComponente(b.ctx, q, b.thread, p1, b.utente, "prova")
	})
	ok(t, err)

	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, p1, s2)
	})
	deveRifiutare(t, err, "P1 è archiviato")
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Sostituisci(b.ctx, q, b.thread, s1, s2, true)
	})
	deveRifiutare(t, err, "P1 è archiviato")
	if got := uno[string](b, `SELECT coalesce(step_strutturale_id::text, '') || ' ' || (SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL)
		FROM componente WHERE componente_id = $1`, p1); got != s1.String()+" 0" {
		t.Errorf("il rifiuto non ha annullato tutto: %s", got)
	}
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Sostituisci(b.ctx, q, b.thread, s1, s2, false)
	})
	ok(t, err)
}

// Gli scarti e il codice di un nodo bloccano la RFQ come gli altri gesti (decisioni.go lo promette): con la
// RFQ bloccata da un'altra transazione aspettano, e da soli funzionano come prima.
func TestGliScartiEIlCodiceBloccanoLaRfq(t *testing.T) {
	b := nuovoBanco(t)
	c, doc, a := b.stepApplicato()
	k := func(p, f string) fascicolo.ChiaveRelazione {
		return fascicolo.ChiaveRelazione{Allegato: a.AllegatoID, Padre: p, Figlio: f}
	}
	gesti := []struct {
		nome, esito string
		f           func(q *db.Queries) (string, error)
	}{
		{"CodiceDelNodo", "ha il codice", func(q *db.Queries) (string, error) {
			return fascicolo.CodiceDelNodo(b.ctx, q, b.thread, b.proposta("#5"), codiceDi("X1"), "")
		}},
		{"ScartaRelazione", "Relazione scartata", func(q *db.Queries) (string, error) {
			return fascicolo.ScartaRelazione(b.ctx, q, b.thread, k("#3", "#4"), b.utente)
		}},
		{"ScartaNodo", "scartato", func(q *db.Queries) (string, error) {
			return fascicolo.ScartaNodo(b.ctx, q, b.thread, b.proposta("#5"), b.utente)
		}},
		{"ScartaRimozione", "l'arco resta", func(q *db.Queries) (string, error) {
			return fascicolo.ScartaRimozione(b.ctx, q, b.thread, fascicolo.ChiaveRimozione{Step: doc, Padre: c["P1"], Figlio: c["D"]}, b.utente)
		}},
	}
	for _, g := range gesti {
		// la RFQ bloccata da un'altra transazione: il gesto aspetta il lucchetto, e qui lo si vede
		// scadere invece di passare
		tieni, err := b.p.Begin(b.ctx)
		ok(t, err)
		if _, err := db.New(tieni).BloccaThread(b.ctx, b.thread); err != nil {
			t.Fatal(err)
		}
		tx, err := b.p.Begin(b.ctx)
		ok(t, err)
		if _, err := tx.Exec(b.ctx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
			t.Fatal(err)
		}
		_, err = g.f(db.New(tx))
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != "55P03" {
			t.Errorf("%s con la RFQ bloccata da un altro: %v, atteso che aspetti il lucchetto (55P03)", g.nome, err)
		}
		_ = tx.Rollback(b.ctx)
		_ = tieni.Rollback(b.ctx)

		msg, err := b.gesto(g.f)
		if err != nil || !strings.Contains(msg, g.esito) {
			t.Errorf("%s: %q, %v; atteso che contenga %q", g.nome, msg, err, g.esito)
		}
	}
}

// Una deroga strutturale si revoca solo dalla sua RFQ: l'id arriva da un indirizzo, e una POST sotto la RFQ
// B non deve poter cancellare la deroga della RFQ A.
func TestUnaDerogaStrutturaleSiRevocaSoloDallaSuaRfq(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	b.analisi(r.step, struttura3Troncata)
	var deroga db.DerogaStruttura
	ok(t, b.tx(func(q *db.Queries) (err error) {
		deroga, err = fascicolo.ConcediDerogaStruttura(b.ctx, q, b.thread, r.p1, b.utente, "la distinta la controlliamo a mano")
		return err
	}))
	altra := uno[uuid.UUID](b, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa)
		SELECT cliente_id, 'outlook', now(), 'PROVAA4\WIP\altra' FROM thread_offerta WHERE thread_id = $1 RETURNING thread_id`, b.thread)

	err := b.tx(func(q *db.Queries) error {
		return fascicolo.RevocaDerogaStruttura(b.ctx, q, altra, deroga.DerogaStrutturaID)
	})
	deveRifiutare(t, err, "non trovata in questa RFQ")
	if n := uno[int64](b, `SELECT count(*) FROM deroga_struttura WHERE deroga_struttura_id = $1`, deroga.DerogaStrutturaID); n != 1 {
		t.Fatal("la deroga della RFQ A e' stata cancellata da un gesto sulla RFQ B")
	}
	ok(t, b.tx(func(q *db.Queries) error {
		return fascicolo.RevocaDerogaStruttura(b.ctx, q, b.thread, deroga.DerogaStrutturaID)
	}))
	if n := uno[int64](b, `SELECT count(*) FROM deroga_struttura`); n != 0 {
		t.Errorf("dalla sua RFQ la deroga non si revoca: %d righe", n)
	}
}

// Un arco messo a mano sotto un prodotto con lo STEP strutturale letto per intero: lo STEP non ce l'ha, e
// il ricalcolo lo proponeva subito da togliere. Si tiene, come fa l'editor della struttura; le rimozioni
// che c'erano gia' su altri archi restano aperte.
func TestUnArcoMessoAManoNonSiProponeDaTogliere(t *testing.T) {
	b := nuovoBanco(t)
	c, _, _ := b.stepApplicato()
	x := b.comp("X1", db.TipoComponenteSciolto)
	y := b.comp("Y1", db.TipoComponenteSciolto)

	_, err := b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Collega(b.ctx, q, b.thread, c["P1"], x, b.utente, 2)
	})
	ok(t, err)
	_, err = b.gesto(func(q *db.Queries) (string, error) {
		return fascicolo.Sposta(b.ctx, q, b.thread, y, uuid.NullUUID{}, uuid.NullUUID{UUID: c["B"], Valid: true}, b.utente, 1)
	})
	ok(t, err)
	if got := b.rimozioni(); got != "P1>D:aperta P1>X1:scartata B>Y1:scartata" {
		t.Errorf("rimozioni = %q: gli archi appena messi a mano sono proposti da togliere", got)
	}
	if got := uno[string](b, `SELECT string_agg(coalesce(nota, '') || ' ' || (deciso_da IS NOT NULL), '|' ORDER BY nota)
		FROM rimozione_proposta WHERE thread_id = $1 AND stato = 'scartata'`, b.thread); got != "tenuto nella struttura confermata true|tenuto nella struttura confermata true" {
		t.Errorf("le rimozioni chiuse non dicono chi e perche': %s", got)
	}
	g, err := db.New(b.p).GateCongelamento(b.ctx, b.thread)
	ok(t, err)
	if g.NProposteRimozione != 1 {
		t.Errorf("rimozioni aperte per il gate: %d, attesa 1 (P1 → D)", g.NProposteRimozione)
	}
}

// D16 non riscrive la proposta di un documento il cui codice l'ha scritto l'operatore, anche se la legge
// prima che l'operatore la scriva: il muro sta nella query.
func TestLaRadiceDiFamigliaNonRiscriveUnaDecisioneDellOperatore(t *testing.T) {
	b := nuovoBanco(t)
	msg := b.mail()
	a := b.fileConProposta(msg, "assieme.stp", "cad_3d", "OP-001", "", "operatore", "aperta", "")
	n, err := db.New(b.p).PropostaDocumentoDaRadice(b.ctx, db.PropostaDocumentoDaRadiceParams{
		AllegatoID: a, Codice: pgtype.Text{String: "77722757", Valid: true}, Confidenza: 80, Dettagli: []byte(`{"famiglia": "disegni 777"}`)})
	ok(t, err)
	if n != 0 {
		t.Errorf("la radice di famiglia ha riscritto %d proposte dell'operatore", n)
	}
	if got := uno[string](b, `SELECT codice || ' ' || fonte FROM documento_proposta WHERE allegato_id = $1`, a); got != "OP-001 operatore" {
		t.Errorf("proposta dopo D16: %s", got)
	}
	// una proposta letta da una regola invece si corregge, come prima
	a2 := b.fileConProposta(msg, "assieme2.stp", "cad_3d", "", "", "nome_file", "aperta", "")
	n, err = db.New(b.p).PropostaDocumentoDaRadice(b.ctx, db.PropostaDocumentoDaRadiceParams{
		AllegatoID: a2, Codice: pgtype.Text{String: "77722757", Valid: true}, Confidenza: 80, Dettagli: []byte(`{}`)})
	ok(t, err)
	if n != 1 {
		t.Errorf("una proposta non dell'operatore non si e' corretta (%d righe)", n)
	}
}
