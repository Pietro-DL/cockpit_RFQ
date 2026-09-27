//go:build integrazione

// L4 — Smistamento F2, giro di correzione: un codice della richiesta rimasto senza componente mentre la BOM
// e' congelata entra come prodotto aprendo la revisione (D26). Dopo F1 (la GET non crea) e F2 (il pannello
// non ha piu' «+ Prodotto») era l'unica strada che mancava: AssicuraProdottiDellaRichiesta con la BOM
// congelata esce con Bloccata e non crea niente, e nessun gesto successivo la richiamava.

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Il caso: la BOM e' congelata (V1) e la RFQ ha un codice della richiesta confermato senza componente. La
// pagina lo mostra con la strada vera; aprire le pagine e prepararne i file non lo crea (F1: nessuna
// decisione); aprire la revisione si', come prodotto finito, con chi aveva confermato il codice, una volta.
//
// Smistamento F3 (rilievo della verifica di F2, R1, E11): solo il codice confermato mentre la BOM era
// congelata. Un prodotto della richiesta tolto dalla BOM prima del congelamento (il suo codice resta fra
// quelli della richiesta) non rinasce aprendo la revisione: quella e' una decisione sulla versione, non su
// quel codice. E la riga di quel codice non promette la revisione: dice che il prodotto non rinasce da solo
// (riscritta nella frase: prima fissava per il codice fermato «il prodotto nasce con la creazione della RFQ
// dal triage, o aprendo una revisione della BOM congelata», che valeva anche per quello tolto).
func TestUnCodiceDellaRichiestaEntraComeProdottoAprendoLaRevisione(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("PRV87")
	w := operatore(b)

	// un prodotto della richiesta che l'operatore toglie prima del congelamento: non aveva storia
	s.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, '77790001', 'manuale', 100, $2)`,
		s.thread, s.utente)
	var tolto uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '77790001', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.utente).Scan(&tolto); err != nil {
		t.Fatal(err)
	}
	if a := s.gesto(w, s.comp(tolto, "rimuovi"), nil); !strings.Contains(a, "77790001 tolto") {
		t.Fatalf("rimuovi: %q", a)
	}

	s.gesto(w, s.comp(s.assieme, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"lo facciamo noi"}})
	s.gesto(w, s.comp(s.particolare, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"a commessa"}})
	s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}})
	s.gesto(w, s.comp(s.prodotto, "deroga-struttura"), url.Values{"motivo": {"non ancora analizzato, va bene"}})
	if a := s.gesto(w, s.base()+"/congela", url.Values{"motivo": {"prima baseline"}}); !strings.HasPrefix(a, "BOM congelata: V1 (preventivo).") {
		t.Fatalf("congela: %q", a)
	}
	// il codice della richiesta senza componente: confermato da una persona, visto anche nel corpo della mail
	s.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, '77790000', 'manuale', 100, $2)`,
		s.thread, s.utente)
	s.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, origine, punteggio, evidenza) VALUES ($1, '77790000', 'prodotto', 'generico', 30, 'corpo'),
		($1, '77790001', 'prodotto', 'generico', 30, 'corpo')`, s.msg)
	quanti := func() int {
		return s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND upper(codice) = '77790000'`, s.thread)
	}

	// la pagina dice la strada vera, e ne' leggerla ne' preparare i file crea il prodotto
	_, html := w.fai(http.MethodGet, "/thread/"+s.thread.String(), nil, false)
	riga := rigaDelCodice(t, html, "77790000")
	for _, atteso := range []string{"codice della richiesta senza componente", "confermato dopo il congelamento della V1", fraseRichiestaConRevisione} {
		if !strings.Contains(riga, atteso) {
			t.Errorf("la riga di 77790000 non dice %q:\n%s", atteso, riga)
		}
	}
	tolto77 := rigaDelCodice(t, html, "77790001")
	if !strings.Contains(tolto77, fraseRichiestaSenzaRevisione) || strings.Contains(tolto77, fraseRichiestaConRevisione) {
		t.Errorf("la riga di 77790001 (tolto prima del congelamento) promette la revisione:\n%s", tolto77)
	}
	if strings.Contains(riga, "preparazione dei file") {
		t.Errorf("la riga promette ancora la preparazione dei file:\n%s", riga)
	}
	w.fai(http.MethodGet, s.base(), nil, false)
	w.daFascicolo(http.MethodPost, s.base()+"/prepara", url.Values{"auto": {"1"}}, s.thread, "")
	w.daFascicolo(http.MethodPost, s.base()+"/prepara", url.Values{"da": {"fascicolo"}}, s.thread, "")
	if n := quanti(); n != 0 {
		t.Fatalf("con la BOM congelata, pagine e preparazione hanno creato %d componenti 77790000", n)
	}

	// la revisione: il prodotto entra, e il messaggio lo dice
	s.fase("ACCETTATA")
	if a := s.gesto(w, s.base()+"/revisione/apri", url.Values{"contesto": {"tecnica"}, "motivo": {"il cliente aggiunge un prodotto"}}); a !=
		"Revisione V2 aperta (tecnica): la fase resta ACCETTATA. La BOM working si modifica di nuovo. 77790000 nella BOM come prodotto finito." {
		t.Fatalf("apri: %q", a)
	}
	if got := s.valore(`SELECT tipo::text || ':' || origine::text || ':' || (confermato_da = $2)::text || ':' || (archiviato_il IS NULL)::text
		FROM componente WHERE thread_id = $1 AND codice = '77790000'`, s.thread, s.utente); got != "finito:manuale:true:true" {
		t.Errorf("il prodotto nato con la revisione = %s, atteso finito:manuale:true:true", got)
	}
	if n := quanti(); n != 1 {
		t.Errorf("%d componenti 77790000, atteso 1", n)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '77790001'`, s.thread); n != 0 {
		t.Errorf("il prodotto tolto prima del congelamento e' rinato aprendo la revisione: %d", n)
	}
	// gli altri codici della richiesta, se ce ne sono, non si toccano: nessun altro componente nuovo
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, s.thread); n != 4 {
		t.Errorf("%d componenti, attesi 4 (i tre della scena e 77790000)", n)
	}
}
