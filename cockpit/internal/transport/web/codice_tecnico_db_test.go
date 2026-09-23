//go:build integrazione

// L4 — B8.2, addendum A2.2: un file tecnico (CAD 3D, disegno 2D, sviluppo DXF) entra nel fascicolo
// solo con il codice del pezzo, e un documento agganciato porta il codice del suo componente lettera
// per lettera.
//
// Dalla 0018 il database lo impone da solo (ck_documento_tecnico_ha_codice e la FK composita
// fk_documento_componente). Queste prove guardano l'altra meta': che la conferma lo dica PRIMA,
// con parole che l'operatore capisce, e lasci la proposta aperta invece di finire su un vincolo.
package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// propostaTecnica prepara una RFQ con un allegato in staging e una proposta aperta del tipo e del
// codice dati (codice "" = proposta senza codice). Restituisce thread e proposta.
func (b *bancoWeb) propostaTecnica(chiave, tipo, codice string) (uuid.UUID, uuid.UUID) {
	b.t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook', now(), 'RFQ codice tecnico', 'ACME\WIP\2026 09 23 codice tecnico', 1) RETURNING thread_id`, cliente).Scan(&thread); err != nil {
		b.t.Fatal(err)
	}
	msg := b.messaggioIn(chiave)
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, msg, thread); err != nil {
		b.t.Fatal(err)
	}
	percorso := filepath.Join(b.t.TempDir(), "assieme.stp")
	if err := os.WriteFile(percorso, []byte("ISO-10303-21;"), 0o644); err != nil {
		b.t.Fatal(err)
	}
	var allegato, proposta uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine,
		bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'assieme.stp','stp','file','outlook',13,repeat('b',64),$2,'analizzato',now())
		RETURNING allegato_id`, msg, percorso).Scan(&allegato); err != nil {
		b.t.Fatal(err)
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1,$2,$3,NULLIF($4,''),60,'estensione') RETURNING proposta_id`, allegato, thread, tipo, codice).Scan(&proposta); err != nil {
		b.t.Fatal(err)
	}
	return thread, proposta
}

func (b *bancoWeb) contaNelThread(tabella string, thread uuid.UUID) int {
	b.t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM `+tabella+` WHERE thread_id = $1`, thread).Scan(&n); err != nil {
		b.t.Fatal(err)
	}
	return n
}

// Il CAD senza codice: la conferma lo rifiuta con un messaggio che dice che cosa manca, la proposta
// resta aperta e nel fascicolo non entra niente. Due strade: la proposta senza codice confermata
// cosi' com'e', e il form con il campo codice vuoto.
func TestUnFileTecnicoSenzaCodiceNonEntraNelFascicolo(t *testing.T) {
	casi := []struct {
		nome string
		form url.Values
	}{
		{"proposta senza codice", url.Values{}},
		{"codice cancellato nel form", url.Values{"codice": {"   "}, "tipo": {"cad_3d"}}},
	}
	for i, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			b := preparaBancoWeb(t)
			thread, proposta := b.propostaTecnica("TEC"+string(rune('1'+i)), "cad_3d", "")
			w := b.browser("10.0.0.1")
			w.login("FP", "prova-fp")

			_, html := w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", c.form, true)
			if !strings.Contains(leggibile(html), "entra nel fascicolo solo con il codice del pezzo") {
				t.Fatalf("la conferma non dice che manca il codice:\n%s", estrai(html, "avviso"))
			}
			if strings.Contains(html, "ck_documento_tecnico_ha_codice") || strings.Contains(html, "SQLSTATE") {
				t.Errorf("l'operatore vede l'errore del database invece della spiegazione:\n%s", estrai(html, "avviso"))
			}
			var stato string
			if err := b.pool.QueryRow(b.ctx, `SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, proposta).Scan(&stato); err != nil {
				t.Fatal(err)
			}
			if stato != "aperta" {
				t.Errorf("la proposta e' %s: doveva restare aperta", stato)
			}
			if n := b.contaNelThread("documento", thread); n != 0 {
				t.Errorf("%d documenti nel fascicolo: un CAD senza codice non doveva entrare", n)
			}
			if n := b.contaNelThread("componente", thread); n != 0 {
				t.Errorf("%d componenti creati da una conferma rifiutata", n)
			}
		})
	}
}

// Un documento non tecnico senza codice (qui un capitolato) si conferma come prima: la regola vale
// per i file che descrivono un pezzo, non per tutto il fascicolo.
func TestUnCapitolatoSenzaCodiceSiConfermaComePrima(t *testing.T) {
	b := preparaBancoWeb(t)
	thread, proposta := b.propostaTecnica("CAP1", "capitolato", "")
	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")

	_, html := w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", url.Values{}, true)
	if !strings.Contains(html, "Confermato: CAPITOLATI") {
		t.Fatalf("il capitolato senza codice non si conferma piu':\n%s", estrai(html, "avviso"))
	}
	if n := b.contaNelThread("documento", thread); n != 1 {
		t.Errorf("documenti nel fascicolo: %d, atteso 1", n)
	}
}

// Il componente esiste gia' con il codice scritto in minuscolo; la proposta lo scrive in maiuscolo.
// La conferma aggancia il documento a QUEL componente (niente secondo componente) e gli copia il
// codice cosi' com'e' scritto sul componente: la FK composita non accetterebbe una differenza di
// maiuscole, e senza questa copia la conferma fallirebbe.
func TestIlDocumentoAgganciatoPrendeIlCodiceDelComponente(t *testing.T) {
	b := preparaBancoWeb(t)
	thread, proposta := b.propostaTecnica("AGG1", "cad_3d", "AB12")
	var comp uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da)
		VALUES ($1, 'ab12', 'sciolto', (SELECT utente_id FROM utente WHERE sigla = 'FP')) RETURNING componente_id`, thread).Scan(&comp); err != nil {
		t.Fatal(err)
	}
	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")

	_, html := w.fai(http.MethodPost, "/proposta/"+proposta.String()+"/conferma", url.Values{}, true)
	if !strings.Contains(html, "Confermato:") {
		t.Fatalf("la conferma non e' riuscita:\n%s", estrai(html, "avviso"))
	}
	var agganciato uuid.UUID
	var codice string
	if err := b.pool.QueryRow(b.ctx, `SELECT componente_id, codice FROM documento WHERE thread_id = $1`, thread).Scan(&agganciato, &codice); err != nil {
		t.Fatal(err)
	}
	if agganciato != comp {
		t.Errorf("documento agganciato a %s, atteso il componente esistente %s", agganciato, comp)
	}
	if codice != "ab12" {
		t.Errorf("codice del documento %q: doveva essere quello del componente, %q", codice, "ab12")
	}
	if n := b.contaNelThread("componente", thread); n != 1 {
		t.Errorf("componenti nel thread: %d, atteso 1 (la conferma non ne crea un secondo con le maiuscole)", n)
	}
}
