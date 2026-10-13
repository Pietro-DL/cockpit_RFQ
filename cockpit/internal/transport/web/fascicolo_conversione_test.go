package web

// L1 — A1d P4, D-K6-01 = A [U] con la precisazione della direttiva 12 §6: la conferma contestuale del cambio di tipo
// nel Fascicolo legacy (fascicolo_conversione.go). Senza database: il form che rimanda il gesto, la lettura del campo
// `assieme`, la domanda disegnata al posto dell'avviso, la scelta della pagina che la puo' mostrare. Il percorso con
// il database e il server vero (la domanda non scrive, la conferma e' atomica) e' nella L4
// TestScenarioGConfermaContestualeDalFascicolo (fascicolo_conversione_db_test.go). Codici della famiglia ACME 712.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func richiestaForm(t *testing.T, form url.Values) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/thread/x/fascicolo/relazione/accetta", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return r
}

// Il form della domanda rimanda lo stesso gesto: la rotta costruita dal server, i campi del gesto com'erano, gli id
// gia' confermati e quello nuovo in `assieme`. corpo_chiave no: lo rimanda hx-include, come per ogni gesto.
func TestDomandaConversioneRimandaLoStessoGesto(t *testing.T) {
	thread := uuid.MustParse("a1000000-0000-4000-8000-0000000000f0")
	b := uuid.MustParse("a1000000-0000-4000-8000-000000000012")
	gia := uuid.MustParse("a1000000-0000-4000-8000-000000000013")
	rc := fascicolo.RichiestaConversione{Componente: b, Codice: "7120012", Figlio: "7120021"}
	r := richiestaForm(t, url.Values{"allegato": {"a1000000-0000-4000-8000-0000000000aa"}, "padre": {" #1 "}, "figlio": {"#2"},
		"corpo_chiave": {"vuoto"}, campoAssieme: {gia.String()}})
	c := domandaConversione(r, "/relazione/accetta", "allegato", "padre", "figlio")(thread, rc)
	if c == nil {
		t.Fatal("attesa la domanda")
	}
	if c.Azione != "/thread/"+thread.String()+"/fascicolo/relazione/accetta" || c.Codice != "7120012" || c.Componente != b.String() {
		t.Errorf("rotta, codice, componente: %+v", c)
	}
	if c.Domanda != rc.Domanda() {
		t.Errorf("domanda: %q", c.Domanda)
	}
	attesi := []campoNascosto{{"allegato", "a1000000-0000-4000-8000-0000000000aa"}, {"padre", "#1"}, {"figlio", "#2"},
		{campoAssieme, gia.String()}, {campoAssieme, b.String()}}
	if !reflect.DeepEqual(c.Campi, attesi) {
		t.Errorf("campi:\n  %+v\natteso:\n  %+v", c.Campi, attesi)
	}

	// niente domanda: componente gia' confermato (il core lo avrebbe convertito), rotta non costruita, id non valido
	r = richiestaForm(t, url.Values{"padre": {"#1"}, campoAssieme: {b.String()}})
	if c := domandaConversione(r, "/relazione/accetta", "padre")(thread, rc); c != nil {
		t.Errorf("componente gia' confermato: nessuna seconda domanda, %+v", c)
	}
	r = richiestaForm(t, url.Values{"chiave": {"#2"}})
	if c := domandaConversione(r, "", "chiave")(thread, rc); c != nil {
		t.Errorf("senza rotta nessuna domanda: %+v", c)
	}
	r = richiestaForm(t, url.Values{campoAssieme: {"non-un-id"}})
	if c := domandaConversione(r, "/relazione/accetta")(thread, rc); c != nil {
		t.Errorf("con un id non valido nessuna domanda: %+v", c)
	}
}

// Il campo `assieme`, ripetuto, diventa DecisioneTipo; vuoto vale nessuna dichiarazione; un id non valido e' un
// rifiuto (la pagina torna con «Niente è cambiato»), non un 400.
func TestDecisioneTipoDalForm(t *testing.T) {
	b, c := uuid.New(), uuid.New()
	d, err := decisioneTipoDal(richiestaForm(t, url.Values{campoAssieme: {b.String(), " ", c.String()}}))
	if err != nil || !reflect.DeepEqual(d.Assiemi, []uuid.UUID{b, c}) {
		t.Errorf("due id: %+v %v", d, err)
	}
	if d, err := decisioneTipoDal(richiestaForm(t, url.Values{})); err != nil || len(d.Assiemi) != 0 {
		t.Errorf("senza campo: %+v %v", d, err)
	}
	_, err = decisioneTipoDal(richiestaForm(t, url.Values{campoAssieme: {"7120012"}}))
	var r rifiuto
	if !errors.As(err, &r) || string(r) != "componente da cambiare in assieme non valido" {
		t.Errorf("id non valido: %v", err)
	}
}

// La domanda al posto dell'avviso: il testo con il componente e il cambiamento, il form che rimanda il gesto con i
// campi nascosti, «Sì» e «No». Non e' un avviso di esito (role="status"): chi legge l'esito non la prende per fatta.
func TestLaDomandaDelCambioDiTipoAlPostoDellAvviso(t *testing.T) {
	tid := uuid.MustParse("a1000000-0000-4000-8000-0000000000f0")
	b := uuid.MustParse("a1000000-0000-4000-8000-000000000012")
	d := &fascicoloDati{T: db.ThreadOfferta{ThreadID: tid}, Base: "/thread/" + tid.String() + "/fascicolo"}
	rc := fascicolo.RichiestaConversione{Componente: b, Codice: "7120012", Figlio: "7120021"}
	r := richiestaForm(t, url.Values{"allegato": {"a1000000-0000-4000-8000-0000000000aa"}, "padre": {"#1"}, "figlio": {"#2"}})
	d.ConfermaTipo = domandaConversione(r, "/relazione/accetta", "allegato", "padre", "figlio")(tid, rc)
	html := rendiParte(t, "fasc_avviso", d)
	haTesto(t, "la domanda", html,
		`class="avviso-f no conferma-tipo" role="alertdialog"`,
		`data-componente="`+b.String()+`"`,
		"Il componente 7120012 è attualmente un particolare. Aggiungendo 7120021 come figlio, 7120012 diventerà un sottoassieme. Vuoi confermare questa modifica?",
		"Finché non confermi, niente è cambiato.",
		`<form class="in-linea gesto" hx-post="/thread/`+tid.String()+`/fascicolo/relazione/accetta">`,
		`<input type="hidden" name="allegato" value="a1000000-0000-4000-8000-0000000000aa">`,
		`<input type="hidden" name="padre" value="#1">`,
		`<input type="hidden" name="figlio" value="#2">`,
		`<input type="hidden" name="assieme" value="`+b.String()+`">`,
		`<button type="submit" class="btn small primary">Sì, 7120012 diventa un sottoassieme</button>`,
		`<button type="button" class="btn small" onclick="this.closest('.conferma-tipo').remove()">No, lascia com'è</button>`)
	if strings.Contains(html, `role="status"`) {
		t.Errorf("la domanda non e' un esito:\n%s", html)
	}
	if strings.Count(html, "hx-post=") != 1 {
		t.Errorf("un solo gesto, quello confermato: «No» non manda niente:\n%s", html)
	}

	// senza domanda l'avviso e' quello di sempre
	d.ConfermaTipo, d.Avviso = nil, "Niente è cambiato: 7120012 → 7120021: 7120012 è un particolare"
	html = rendiParte(t, "fasc_avviso", d)
	if strings.Contains(html, "conferma-tipo") || !strings.Contains(html, `<div class="avviso-f no" role="status">Niente è cambiato:`) {
		t.Errorf("l'avviso di un rifiuto:\n%s", html)
	}
}

// Il form della domanda eredita da #fascicolo bersaglio, sostituzione e hx-include, come i pulsanti da cui il gesto
// parte: l'avviso sta dentro #fascicolo. Se la pagina cambiasse questa forma, «Sì» manderebbe la risposta altrove.
func TestLaDomandaStaDentroIlFascicolo(t *testing.T) {
	src, err := os.ReadFile("../../../web/templates/fascicolo.html")
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	i := strings.Index(s, `<div id="fascicolo" class="fasc`)
	j := strings.Index(s, `{{template "fasc_corpo" .}}`)
	k := strings.Index(s, `<div id="fasc-avviso" class="fasc-avviso" aria-live="polite">{{template "fasc_avviso" .}}</div>`)
	if i < 0 || j < i || k < 0 {
		t.Fatalf("la forma della pagina e' cambiata (#fascicolo %d, fasc_corpo %d, #fasc-avviso %d)", i, j, k)
	}
	testa := s[i : strings.Index(s[i:], ">")+i]
	for _, a := range []string{`hx-target="#fasc-avviso"`, `hx-swap="innerHTML"`, `hx-include="#corpo-chiave"`} {
		if !strings.Contains(testa, a) {
			t.Errorf("#fascicolo non ha piu' %s: %s", a, testa)
		}
	}
	if !strings.Contains(s[strings.Index(s, `{{define "fasc_corpo"}}`):], `<div id="fasc-avviso"`) {
		t.Error("#fasc-avviso non e' piu' in fasc_corpo, dentro #fascicolo")
	}
}

// La domanda si mostra solo nella schermata del Fascicolo di questa RFQ: da un'altra pagina, o dalla Distinta, resta
// il rifiuto (che nomina la scheda). Non dalla vista Documenti, la predefinita, dove fascicolo.mjs svuota l'avviso al
// primo clic e il «Sì» non partirebbe (revisione D-E del 13/10, rilievo 1): li' resta il rifiuto, con la vista Albero.
func TestLaDomandaSoloDalFascicolo(t *testing.T) {
	s := &Server{}
	tid := uuid.MustParse("a1000000-0000-4000-8000-0000000000f0")
	for _, c := range []struct {
		corrente string
		atteso   bool
	}{
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo?vista=albero", true},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo?vista=bom", true},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo", false},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo/?cassetto=verifica", false},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo?vista=sconosciuta&cassetto=verifica", false},
		{"https://cockpit.example/thread/" + tid.String() + "/distinta", false},
		{"https://cockpit.example/thread/" + tid.String(), false},
		{"https://cockpit.example/thread/" + uuid.NewString() + "/fascicolo", false},
		{"", false},
	} {
		r := httptest.NewRequest(http.MethodPost, "/thread/"+tid.String()+"/fascicolo/relazione/accetta", nil)
		if c.corrente != "" {
			r.Header.Set("HX-Current-URL", c.corrente)
		}
		if _, ok := s.soloDalFascicolo(r, tid); ok != c.atteso {
			t.Errorf("da %q: %v, atteso %v", c.corrente, ok, c.atteso)
		}
	}
}

// Dalla vista Documenti il rifiuto della conversione nomina anche la vista Albero; da un'altra vista o da un'altra
// pagina no, e nemmeno per un errore che non e' una richiesta di conversione (rilievo 1 della revisione D-E).
func TestDallaVistaDocumentiIlRifiutoNominaLaVistaAlbero(t *testing.T) {
	s := &Server{}
	tid := uuid.MustParse("a1000000-0000-4000-8000-0000000000f0")
	rc := fascicolo.RichiestaConversione{Componente: uuid.MustParse("a1000000-0000-4000-8000-000000000012"), Codice: "7120012", Figlio: "7120021"}
	base := "Niente è cambiato: 7120012 → 7120021: 7120012 è un particolare, e il tipo non cambia da solo: " +
		"se contiene dei pezzi, cambialo prima in assieme dalla sua scheda"
	for _, c := range []struct {
		corrente    string
		conversione bool
		atteso      string
	}{
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo", true, base + stradaDallaVistaAlbero},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo?cassetto=verifica", true, base + stradaDallaVistaAlbero},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo?vista=albero", true, base},
		{"https://cockpit.example/thread/" + tid.String(), true, base},
		{"https://cockpit.example/thread/" + tid.String() + "/fascicolo", false, base},
	} {
		r := httptest.NewRequest(http.MethodPost, "/thread/"+tid.String()+"/fascicolo/relazione/accetta", nil)
		r.Header.Set("HX-Current-URL", c.corrente)
		if got := "Niente è cambiato: " + spiegaErrore(rc) + s.stradaInPiuDellaConversione(r, tid, c.conversione); got != c.atteso {
			t.Errorf("da %q (conversione %v):\n %q\natteso\n %q", c.corrente, c.conversione, got, c.atteso)
		}
	}
	if !strings.Contains(stradaDallaVistaAlbero, "vista Albero") || !strings.Contains(stradaDallaVistaAlbero, "confermare il cambio") {
		t.Errorf("la strada della vista Albero: %q", stradaDallaVistaAlbero)
	}
}
