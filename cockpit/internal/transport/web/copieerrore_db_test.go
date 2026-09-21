//go:build integrazione

// L4 — blocco 4C: anche una copia FALLITA si può rimettere in coda.
//
// «Riprova copie» guardava solo i documenti `in_coda`. Nella prova reale sul NAS di test i due
// documenti fermi sono stati rimessi in coda correttamente, la copia è partita e si è fermata perché
// il contenuto non era più nello staging: i documenti sono passati a `errore`, e da lì il pulsante
// non li riprendeva più. Il primo clic li toglieva dall'unico stato da cui potevano ripartire.
//
// In attesa e in errore sono due modi diversi di dire la stessa cosa a chi guarda — quel disegno non
// è nel fascicolo — e il modo di rimediare è lo stesso.
package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
)

func (b *bancoWeb) inErrore(doc uuid.UUID, motivo string) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE documento SET stato_nas = 'errore', errore_nas = $2 WHERE documento_id = $1`,
		doc, motivo); err != nil {
		b.t.Fatal(err)
	}
}

// Un documento la cui copia è fallita si rimette in coda come uno che non ci ha mai provato.
func TestRiprovaCopieRiprendeAncheQuelleFallite(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, docs := b.rfqConDocumentoInCoda(2)
	b.inErrore(docs[0], "il contenuto di \"disegno.pdf\" non e' piu' nello staging del server")

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")

	// la pagina li conta tutti e due: per chi guarda sono la stessa notizia
	_, pagina := w.fai(http.MethodGet, "/thread/"+thread.String(), nil, true)
	if !strings.Contains(pagina, "riprova-copie") {
		t.Fatal("con una copia fallita la pagina non offre più «Riprova copie»")
	}
	if !strings.Contains(pagina, "2 documenti") {
		t.Errorf("il conto non comprende il documento in errore:\n%s", estrai(pagina, "riprova-copie"))
	}
	// e il motivo del fallimento deve essere leggibile, non solo lo stato
	if !strings.Contains(pagina, "staging") {
		t.Errorf("il motivo dell'errore non compare in pagina:\n%s", estrai(pagina, "errore"))
	}

	_, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if !strings.Contains(html, "2 copie rimesse in coda") {
		t.Errorf("la copia fallita non è stata rimessa in coda:\n%s", estrai(html, "avviso"))
	}
	if n := len(b.copieInCoda()); n != 2 {
		t.Errorf("copie accodate: %d, attese 2", n)
	}
}

// Un documento già sul NAS resta fuori: «riprovare» non vuol dire «riscrivere tutto».
func TestRiprovaCopieNonToccaQuelliGiaScritti(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	thread, docs := b.rfqConDocumentoInCoda(2)
	if _, err := b.pool.Exec(b.ctx, `UPDATE documento SET stato_nas = 'scritto', scritto_il = now() WHERE documento_id = $1`, docs[0]); err != nil {
		t.Fatal(err)
	}
	b.inErrore(docs[1], "NAS irraggiungibile")

	w := b.browser("10.0.0.1")
	w.login("FP", "prova-fp")
	_, html := w.fai(http.MethodPost, "/thread/"+thread.String()+"/riprova-copie", nil, true)
	if !strings.Contains(html, "1 copia rimessa in coda") {
		t.Errorf("atteso un solo documento rimesso in coda:\n%s", estrai(html, "avviso"))
	}
	if n := len(b.copieInCoda()); n != 1 {
		t.Errorf("copie accodate: %d, attesa 1 (quella già scritta non si riscrive)", n)
	}
	// blocco 5A: i documenti già sul NAS vanno CONTATI, non ignorati in silenzio. «Nessuna copia
	// nuova» può voler dire «è tutto a posto» oppure «non ho guardato niente», e sono due notizie
	// molto diverse per chi sta aspettando quel disegno.
	if !strings.Contains(leggibile(html), "1 gia' sul NAS") {
		t.Errorf("l'avviso non dice quanti documenti erano già sul NAS:\n%s", estrai(html, "avviso"))
	}
}
