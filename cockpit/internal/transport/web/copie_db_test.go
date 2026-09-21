//go:build integrazione

// L4 — blocco 4: un documento «in_coda» non resta bloccato per sempre.
//
// È il punto 6 del checkpoint: il sistema deve poter riconciliare uno stato incompleto, e un
// documento fermo non può restarlo senza un'azione VISIBILE. Con le capacità separate quello stato
// diventa la norma, non l'eccezione: si conferma una proposta mentre `nas_scrittura` è spenta — la
// decisione dell'operatore è registrata, la scrittura aspetta — e quando la capacità si accende i
// job che avevano aspettato vengono ANNULLATI, non eseguiti. Senza un pulsante quel documento non
// arriverebbe mai sul NAS: bisognerebbe rifare la conferma, cioè prendere due volte la stessa
// decisione.
//
// Il disegno della shadow prometteva «riprova copie» fin dalla voce 9.5. Il pulsante non c'era.
package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// ImpostaCapacitaProva fissa le capacita' del processo per la durata del test e le rimette com'erano
// (tutte accese: e' lo stato di default, vedi jobs/capacita.go).
func ImpostaCapacitaProva(t *testing.T, c coda.Capacita) {
	t.Helper()
	coda.ImpostaCapacita(c)
	t.Cleanup(func() { coda.ImpostaCapacita(coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}) })
}

// rfqConDocumentoInCoda costruisce una RFQ con un documento confermato e non ancora scritto sul NAS.
func (b *bancoWeb) rfqConDocumentoInCoda(quanti int) (uuid.UUID, []uuid.UUID) {
	b.t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook', now(), 'RFQ copie', 'ACME\WIP\2026 09 17 prova', 1) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		b.t.Fatal(err)
	}
	var utente uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&utente); err != nil {
		b.t.Fatal(err)
	}
	var docs []uuid.UUID
	for i := 0; i < quanti; i++ {
		var d uuid.UUID
		sha := strings.Repeat(string(rune('a'+i)), 64)
		if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes,
			path_relativo, stato_nas, confermato_da) VALUES ($1,'disegno_2d',$2,'pdf',$3,1000,$4,'in_coda',$5) RETURNING documento_id`,
			thread, "disegno.pdf", sha, `2D\disegno.pdf`, utente).Scan(&d); err != nil {
			b.t.Fatal(err)
		}
		docs = append(docs, d)
	}
	return thread, docs
}

func (b *bancoWeb) copieInCoda() []db.Job {
	b.t.Helper()
	return b.jobDiTipo(db.TipoJobCopiaNas)
}

// Con la capacità accesa, «Riprova copie» rimette in coda ciò che era rimasto indietro — e non lo
// rimette due volte.
func TestRiprovaCopieRimetteInCodaIDocumentiFermi(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, docs := b.rfqConDocumentoInCoda(2)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")

	// il pulsante deve esserci, e dire quanti sono: un'azione che non si vede non è un'azione
	_, pagina := w.fai(http.MethodGet, "/thread/"+thread.String(), nil, true)
	if !strings.Contains(pagina, "riprova-copie") {
		t.Fatalf("la pagina della RFQ non offre nessun modo di rimettere in coda i documenti fermi:\n%s", estrai(pagina, "Documenti sul NAS"))
	}

	resp, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if resp.StatusCode != 200 {
		t.Fatalf("riprova copie: HTTP %d", resp.StatusCode)
	}
	if !strings.Contains(html, "2 copie rimesse in coda") {
		t.Errorf("l'avviso non dice che cosa è successo:\n%s", estrai(html, "avviso"))
	}
	job := b.copieInCoda()
	if len(job) != len(docs) {
		t.Fatalf("copie accodate: %d, attese %d", len(job), len(docs))
	}

	// un secondo clic non ne accoda altre: la chiave di idempotenza è per documento
	_, html = w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if n := len(b.copieInCoda()); n != len(docs) {
		t.Errorf("un secondo clic ha accodato altre copie: %d in tutto", n)
	}
	if !strings.Contains(html, "gia") && !strings.Contains(html, "già") {
		t.Errorf("il secondo clic non dice che erano già in coda:\n%s", estrai(html, "avviso"))
	}
}

// Con la capacità ancora spenta non si finge niente: l'avviso dice quale riga del file cambiare, e in
// coda non entra nessuna copia.
func TestRiprovaCopieConLaCapacitaSpentaLoDice(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{})
	thread, _ := b.rfqConDocumentoInCoda(1)

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	_, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)

	if n := len(b.copieInCoda()); n != 0 {
		t.Errorf("con nas_scrittura spenta sono entrate %d copie in coda", n)
	}
	if !strings.Contains(html, coda.CapNasScrittura) {
		t.Errorf("l'avviso non nomina la capacità spenta:\n%s", estrai(html, "avviso"))
	}
}

// Una RFQ che ha già tutto sul NAS non mostra il pulsante, e se lo si chiama lo dice.
func TestSenzaDocumentiFermiNonCEeNienteDaRimettereInCoda(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, docs := b.rfqConDocumentoInCoda(1)
	if _, err := b.pool.Exec(b.ctx, `UPDATE documento SET stato_nas = 'scritto', scritto_il = now() WHERE documento_id = $1`, docs[0]); err != nil {
		t.Fatal(err)
	}

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	_, pagina := w.fai(http.MethodGet, "/thread/"+thread.String(), nil, true)
	if strings.Contains(pagina, "riprova-copie") {
		t.Error("la pagina offre «Riprova copie» senza nessun documento fermo")
	}
	_, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if n := len(b.copieInCoda()); n != 0 {
		t.Errorf("accodate %d copie per documenti già scritti", n)
	}
	if !strings.Contains(html, "Nessun documento in attesa") {
		t.Errorf("l'avviso non dice che non c'era niente da fare:\n%s", estrai(html, "avviso"))
	}
}
