package web

// L1 — il form di triage e il pannello, senza database: che cosa nasce spuntato fra i codici trovati,
// come si legge la controparte «altro», e la ricerca di «Aggancia a…» che prende alla lettera quello che
// l'operatore scrive.

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// Un codice di famiglia trovato solo nella storia citata si vede, con la sua evidenza, ma non nasce
// spuntato: e' la regola di classificazione.Estrazione.Proponibili. Era gia' stato deciso in un altro
// messaggio, e spuntarlo da solo a ogni risposta trasforma un «ricevuto, grazie» in una richiesta. Il
// codice di famiglia del messaggio nasce spuntato come prima; il generico no, come prima.
func TestUnCodiceDellaStoriaCitataSiVedeMaNonNasceSpuntato(t *testing.T) {
	s := serverTest(t)
	_, td, _ := datiSintetici()
	td.Proponibili = []db.CandidatoCodice{
		{MessaggioID: td.M.MessaggioID, Codice: "PZ-001", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia,
			Famiglia: "PZ", Punteggio: 80, Evidenza: "oggetto"},
		{MessaggioID: td.M.MessaggioID, Codice: "PZ-002", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia,
			Famiglia: "PZ", Punteggio: 80, Evidenza: classificazione.DoveStoria},
		{MessaggioID: td.M.MessaggioID, Codice: "20260925", Ruolo: db.RuoloCodiceNonClassificato, Origine: db.OrigineCodiceGenerico,
			Punteggio: 30, Evidenza: "corpo"},
	}
	for _, c := range []struct {
		codice   string
		spuntato bool
	}{{"PZ-001", true}, {"PZ-002", false}, {"20260925", false}} {
		for _, x := range td.Proponibili {
			if x.Codice == c.codice && td.Spuntato(x) != c.spuntato {
				t.Errorf("Spuntato(%s) = %v, atteso %v", c.codice, !c.spuntato, c.spuntato)
			}
		}
	}

	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "triage_form", vista{Dati: td, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	casella := func(codice string) string {
		m := regexp.MustCompile(`<input type="checkbox" name="codice" value="` + regexp.QuoteMeta(codice) + `"[^>]*>`).FindString(html)
		if m == "" {
			t.Fatalf("il codice %s non e' fra le caselle del form", codice)
		}
		return m
	}
	if !strings.Contains(casella("PZ-001"), "checked") {
		t.Error("il codice di famiglia del messaggio non nasce spuntato")
	}
	if strings.Contains(casella("PZ-002"), "checked") {
		t.Error("il codice di famiglia della storia citata nasce spuntato")
	}
	if strings.Contains(casella("20260925"), "checked") {
		t.Error("il numero generico nasce spuntato")
	}
	if !strings.Contains(html, classificazione.DoveStoria) {
		t.Error("il form non dice che il codice viene dalla storia citata")
	}
}

// La testata del pannello dice «altro · <etichetta>» per un soggetto censito come altro, come la riga
// della lista. Prima diceva «mittente non censito», cioe' il contrario di quello che l'Inbox mostrava.
func TestIlPannelloDiceAltroComeLaLista(t *testing.T) {
	s := serverTest(t)
	md, _, _ := datiSintetici()
	md.Riga.ControparteTipo, md.Riga.Controparte = "altro", txtT("Corriere Esempio")
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "chip_controparte", md.Riga); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "altro · Corriere Esempio") || !strings.Contains(html, `class="chip altro"`) {
		t.Errorf("chip della controparte «altro»: %s", html)
	}
	if strings.Contains(html, "non censito") {
		t.Errorf("un soggetto censito come altro risulta «non censito»: %s", html)
	}
}

// «Aggancia a…» mette il testo fra due '%': % _ e \ scritti dall'operatore si prendono alla lettera,
// come nella barra della pagina Richieste. «7120_400» cerca quello, non «7120 400» o «7120-400».
func TestLaRicercaDiAgganciaPrendeIlTestoAllaLettera(t *testing.T) {
	for dentro, atteso := range map[string]string{
		"7120_400": `7120\_400`,
		"50%":      `50\%`,
		`A\B`:      `A\\B`,
		"PZ-001":   "PZ-001",
		"":         "",
	} {
		if got := testoLetterale(dentro); got != atteso {
			t.Errorf("testoLetterale(%q) = %q, atteso %q", dentro, got, atteso)
		}
	}
	// e il modello della pagina Richieste e' lo stesso testo, fra i due '%'
	if m := modelloRicerca("7120_400"); !m.Valid || m.String != `%7120\_400%` {
		t.Errorf("modelloRicerca: %+v", m)
	}
}
