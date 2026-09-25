//go:build integrazione

// L4 — A4 (B8.A4a) nella conferma, nell'assegnazione e nell'anteprima: il nome sul NAS con la
// revisione e il nome originale in nome_file; il file identico a una revisione gia' sostituita; lo
// STEP di una baseline che non cambia componente; la BOM congelata detta con le parole giuste invece
// che con l'errore del trigger; il documento sostituito che resta apribile.
//
// Nomi delle prove di A4.12 dell'addendum: 13, 51, 68, 69, 70 (la parte della conferma e
// dell'assegnazione).

package web

import (
	"bytes"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

func confermaProposta(w *browser, p uuid.UUID, form url.Values) string {
	_, html := w.fai(http.MethodPost, "/proposta/"+p.String()+"/conferma", form, true)
	return leggibile(html)
}

func (r *rfqFascicolo) esegui(sql string, arg ...any) {
	r.b.t.Helper()
	if _, err := r.b.pool.Exec(r.b.ctx, sql, arg...); err != nil {
		r.b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (r *rfqFascicolo) conta(sql string, arg ...any) int {
	r.b.t.Helper()
	var n int
	if err := r.b.pool.QueryRow(r.b.ctx, sql, arg...).Scan(&n); err != nil {
		r.b.t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

// congelaV1 congela a mano la prima versione della RFQ (il congelamento vero ha le sue prove in
// core/rfq/fascicolo): la fase FATTIBILITA e la V1 in bozza; chiudiV1 la congela.
func (r *rfqFascicolo) congelaV1() uuid.UUID {
	r.b.t.Helper()
	var fase, v1 uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO fase_log (thread_id, nome_fase, inizio, fine, esito)
		VALUES ($1, 'FATTIBILITA', now() - interval '1 hour', now() - interval '30 minutes', 'OK') RETURNING fase_log_id`, r.thread).Scan(&fase); err != nil {
		r.b.t.Fatal(err)
	}
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prima baseline', $3) RETURNING bom_versione_id`, r.thread, fase, r.utente).Scan(&v1); err != nil {
		r.b.t.Fatal(err)
	}
	return v1
}

// chiudiV1 congela la V1; con bozza = true apre anche la V2, e la working torna libera.
func (r *rfqFascicolo) chiudiV1(v1 uuid.UUID, bozza bool) {
	r.b.t.Helper()
	r.esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, r.utente)
	if bozza {
		var fase uuid.UUID
		if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'SCHEDA_COSTO', now()) RETURNING fase_log_id`, r.thread).Scan(&fase); err != nil {
			r.b.t.Fatal(err)
		}
		r.esegui(`INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
			versione_precedente_id, numero_precedente, stato_precedente) VALUES ($1, 2, 'preventivo', 'SCHEDA_COSTO', $2, 'revisione', $3, $4, 1, 'congelata')`,
			r.thread, fase, r.utente, v1)
	}
}

// Prova 51: il nome sul NAS e' quello del pezzo, il nome originale resta in nome_file.
func TestIlNomeOriginaleRestaInNomeFile(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "NOME51")
	p := r.proposta("Disegno 77720517 rev B (cliente).pdf", "77720517", uuid.Nil)
	avviso := confermaProposta(operatore(b), p, url.Values{"codice": {"77720517"}, "rev": {"B"}})
	if !strings.Contains(avviso, `Confermato: ELENCO DISEGNI\77720517\77720517_REV_B.pdf`) {
		t.Fatalf("avviso: %s", estrai(avviso, "avviso"))
	}
	var nome, percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT nome_file, path_relativo FROM documento WHERE thread_id = $1`, r.thread).Scan(&nome, &percorso); err != nil {
		t.Fatal(err)
	}
	if nome != "Disegno 77720517 rev B (cliente).pdf" || percorso != `ELENCO DISEGNI\77720517\77720517_REV_B.pdf` {
		t.Errorf("nome_file %q, percorso %q", nome, percorso)
	}
}

// allegatoCon e' un allegato del messaggio della RFQ con uno SHA dato, e la sua proposta aperta: lo
// stesso file di un documento gia' confermato, arrivato di nuovo.
func (r *rfqFascicolo) allegatoCon(sha, nome string) uuid.UUID {
	r.b.t.Helper()
	r.n++
	percorso := filepath.Join(r.staging, "copia_"+nome)
	if err := os.WriteFile(percorso, []byte("stesso contenuto"), 0o644); err != nil {
		r.b.t.Fatal(err)
	}
	var a, p uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 16, $4, $5, 'analizzato', now()) RETURNING allegato_id`, r.msg, 100+r.n, nome, sha, percorso).Scan(&a); err != nil {
		r.b.t.Fatal(err)
	}
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', 'P1', 60, 'estensione') RETURNING proposta_id`, a, r.thread).Scan(&p); err != nil {
		r.b.t.Fatal(err)
	}
	return p
}

// Prova 68: un file identico a una revisione gia' sostituita si rifiuta con il suo messaggio, senza
// scrivere niente; due conferme concorrenti dello stesso file danno il messaggio della sessione
// concorrente; uno SHA di un documento corrente fa come prima.
func TestUnFileUgualeAUnaRevisioneSostituitaSiRifiuta(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "SHA68")
	comp := r.componente("P1")
	a := r.documento("a.pdf", "P1", comp, db.StatoNasScritto)
	bb := r.documento("b.pdf", "P1", comp, db.StatoNasScritto)
	r.esegui(`UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, a, bb)
	shaDi := func(doc uuid.UUID) string {
		d, err := b.q.GetDocumento(b.ctx, doc)
		if err != nil {
			t.Fatal(err)
		}
		return d.Sha256
	}
	w := operatore(b)
	fermo := func(p uuid.UUID, frase string) {
		t.Helper()
		documenti, provenienze := r.conta(`SELECT count(*) FROM documento`), r.conta(`SELECT count(*) FROM documento_provenienza`)
		avviso := confermaProposta(w, p, url.Values{})
		if !strings.Contains(avviso, frase) {
			t.Fatalf("atteso «%s»:\n%s", frase, estrai(avviso, "avviso"))
		}
		if r.conta(`SELECT count(*) FROM documento`) != documenti || r.conta(`SELECT count(*) FROM documento_provenienza`) != provenienze {
			t.Error("il rifiuto ha scritto qualcosa")
		}
		if got := b.fotoProposta(p); !strings.HasSuffix(got, "stato=aperta") {
			t.Errorf("la proposta non e' rimasta aperta: %s", got)
		}
	}
	fermo(r.allegatoCon(shaDi(a), "a di nuovo.pdf"), "il file è identico a a.pdf, che b.pdf ha sostituito: per tornare a a.pdf si annulla quella sostituzione")

	c := r.documento("c.pdf", "P1", comp, db.StatoNasScritto)
	r.esegui(`UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, bb, c)
	fermo(r.allegatoCon(shaDi(a), "a ancora.pdf"), "il file è identico a a.pdf, sostituito da b.pdf e poi da c.pdf: tornare a una revisione più vecchia della precedente non è ancora gestito")

	// lo SHA del documento corrente: come prima, una provenienza in piu'
	if avviso := confermaProposta(w, r.allegatoCon(shaDi(c), "c di nuovo.pdf"), url.Values{}); !strings.Contains(avviso, "File già presente nel fascicolo") {
		t.Errorf("SHA del documento corrente:\n%s", estrai(avviso, "avviso"))
	}

	t.Run("due conferme dello stesso file insieme", func(t *testing.T) {
		sha := strings.Repeat("7", 64)
		p := r.allegatoCon(sha, "nuovo.pdf")
		// la prima sessione ha appena inserito il documento e non ha ancora fatto COMMIT
		tx, err := b.pool.Begin(b.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(b.ctx)
		if _, err := tx.Exec(b.ctx, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, 'disegno_2d', 'P1', 'nuovo.pdf', 'pdf', $2, 'ELENCO DISEGNI\P1\altra sessione.pdf', $3)`, r.thread, sha, r.utente); err != nil {
			t.Fatal(err)
		}
		risposta := make(chan string, 1)
		go func() { risposta <- confermaProposta(w, p, url.Values{}) }()
		limite := time.Now().Add(10 * time.Second)
		for r.conta(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`) == 0 {
			if time.Now().After(limite) {
				t.Fatal("la seconda conferma non si e' messa in fila")
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err := tx.Commit(b.ctx); err != nil {
			t.Fatal(err)
		}
		if avviso := <-risposta; !strings.Contains(avviso, "lo stesso file è stato confermato in questo momento da un'altra sessione: ricarica la pagina") {
			t.Errorf("seconda conferma:\n%s", estrai(avviso, "avviso"))
		}
	})
}

// Prova 69: uno STEP registrato in una baseline non cambia piu' componente, e il rifiuto nomina la
// baseline invece di mostrare l'errore della FK.
func TestUnoStepDiUnaBaselineNonCambiaComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "STEP69")
	var p1, step uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'P1', 'finito', $2) RETURNING componente_id`,
		r.thread, r.utente).Scan(&p1); err != nil {
		t.Fatal(err)
	}
	q1 := r.componente("Q1")
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'P1', 'assieme.stp', 'stp', repeat('5', 64), 'ELENCO DISEGNI\P1\P1_REV_ND.stp', $3) RETURNING documento_id`,
		r.thread, p1, r.utente).Scan(&step); err != nil {
		t.Fatal(err)
	}
	v1 := r.congelaV1()
	r.esegui(`INSERT INTO bom_versione_componente (bom_versione_id, thread_id, componente_id, codice, tipo, qta, step_strutturale_id, step_sha256)
		VALUES ($1, $2, $3, 'P1', 'finito', 1, $4, repeat('5', 64))`, v1, r.thread, p1, step)
	r.chiudiV1(v1, true) // la V2 e' aperta: la working e' libera, e a fermare e' la baseline
	prima := b.foto(step)
	avviso := r.assegna(operatore(b), url.Values{"componente": {q1.String()}, "documento": {step.String()}, "correggi_codice": {"1"}})
	if !strings.Contains(avviso, "assieme.stp è lo STEP strutturale registrato nella baseline V1: non cambia più componente") {
		t.Fatalf("avviso: %q", avviso)
	}
	if b.foto(step) != prima {
		t.Errorf("il documento e' cambiato: %s, era %s", b.foto(step), prima)
	}
	// sganciarlo e' la stessa cosa
	if avviso := r.assegna(operatore(b), url.Values{"componente": {""}, "documento": {step.String()}}); !strings.Contains(avviso, "baseline V1") {
		t.Errorf("sganciare: %q", avviso)
	}
}

// Prova 70, la parte della conferma e dell'assegnazione: con la BOM congelata rispondono con il
// messaggio, non con l'errore del trigger; un file senza componente si conferma come sempre.
func TestLaBomCongelataSiDiceConLeParoleGiuste(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "D26")
	comp := r.componente("P1")
	doc := r.documento("gia.pdf", "P1", uuid.Nil, db.StatoNasInCoda)
	p := r.proposta("nuovo.pdf", "P1", uuid.Nil)
	r.chiudiV1(r.congelaV1(), false)
	w := operatore(b)

	avviso := confermaProposta(w, p, url.Values{"componente_id": {comp.String()}})
	if !strings.Contains(avviso, "la BOM è congelata nella V1: il file entra senza componente, e lo si assegna aprendo una revisione") {
		t.Fatalf("conferma con il componente:\n%s", estrai(avviso, "avviso"))
	}
	if strings.Contains(avviso, "BOM01") || strings.Contains(avviso, "SQLSTATE") {
		t.Errorf("l'operatore vede l'errore del database:\n%s", estrai(avviso, "avviso"))
	}
	if r.conta(`SELECT count(*) FROM documento WHERE thread_id = $1`, r.thread) != 1 || !strings.HasSuffix(b.fotoProposta(p), "stato=aperta") {
		t.Fatal("il rifiuto ha scritto qualcosa")
	}
	if a := r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {doc.String()}}); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("assegnazione: %q", a)
	}
	if a := r.correggiCodice(w, comp, "P2"); !strings.Contains(a, "la BOM è congelata nella V1: il codice si corregge aprendo una revisione") {
		t.Errorf("correzione del codice: %q", a)
	}
	// senza componente la conferma va come sempre
	if avviso := confermaProposta(w, p, url.Values{}); !strings.Contains(avviso, "Senza componente") {
		t.Errorf("conferma senza componente:\n%s", estrai(avviso, "avviso"))
	}
}

// Prova 13: un documento sostituito resta apribile, dal suo percorso.
func TestUnDocumentoSostituitoRestaApribile(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(32*1024), false, true)
	var comp, nuovo uuid.UUID
	var utente uuid.UUID
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&utente); err != nil {
		t.Fatal(err)
	}
	if err := s.b.pool.QueryRow(s.b.ctx, `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ($1, 'D1', $2) RETURNING componente_id`,
		s.thread, utente).Scan(&comp); err != nil {
		t.Fatal(err)
	}
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE documento SET componente_id = $2 WHERE documento_id = $1`, s.doc, comp); err != nil {
		t.Fatal(err)
	}
	if err := s.b.pool.QueryRow(s.b.ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'disegno_2d', 'D1', 'nuovo.pdf', 'pdf', repeat('9', 64), 'ELENCO DISEGNI\D1\D1_REV_B.pdf', $3) RETURNING documento_id`,
		s.thread, comp, utente).Scan(&nuovo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, s.doc, nuovo); err != nil {
		t.Fatal(err)
	}
	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 || !bytes.Equal(corpo, s.pdf) {
		t.Fatalf("il documento sostituito non si apre: stato %d, %d byte", resp.StatusCode, len(corpo))
	}
	d, err := s.b.q.GetDocumento(s.b.ctx, s.doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.PathRelativo != pathDocProva || !d.SostituitoDa.Valid {
		t.Errorf("percorso %q, sostituito %v", d.PathRelativo, d.SostituitoDa.Valid)
	}
}
