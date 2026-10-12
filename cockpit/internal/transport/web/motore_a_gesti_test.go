package web

// L1 — giro 5, A1d-Q3: il ramo Motore A di threadFrammento (motore_a.go: dalMotoreA, esitoMotoreA, rispondiMotoreA;
// thread.go: le righe del ramo) e la caratterizzazione di avvisoNegativo sui gesti del ponte (mappa dei gesti A1d-P3,
// §3; prove CQ-01). Derivate in parte dalle prove della PR #18 di Stefano Valo (67dcc02: motore_a_test.go,
// TestMotoreALaRispostaDelGesto, TestMotoreADallaMotoreA, TestThreadFrammentoRispondeAllaMotoreA), senza l'elenco
// rifiutiMotoreA (Q-A1d-09 = A) e quindi senza TestMotoreAIRifiutiCon200SonoNo né la guardia
// TestMotoreALeFrasiDeiGestiSonoClassificate: al loro posto la caratterizzazione qui sotto.
//
// LA CARATTERIZZAZIONE FISSA IL COMPORTAMENTO ATTUALE, NON QUELLO CORRETTO. Ogni riga dice la lettura di oggi
// (colonna «Attuale» del §3) e, a parte, quella che il contratto vorrebbe («Corretta»): dove sono diverse la riga è un
// disallineamento registrato (TEC-02, TEC-04, TEC-05, TEC-06, TEC-07), non un difetto da correggere in P3. Chi cambia
// avvisoNegativo o una frase cambia la prova di proposito, con una decisione. La guardia dei testi
// (TestCaratterizzazioneAvvisoNegativoITestiCiSonoAncora) legge i sorgenti del package e di core/rfq/fascicolo: se una
// frase caratterizzata sparisce o cambia, la prova cade e la riga va rivista.
//
// I clienti di questi test sono inventati: codici 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// TestMotoreADalMotoreA: solo l'indirizzo esatto della pagina, come lo manda il ponte (P3-GO-01 della revisione di P3):
// percorso esatto, stesso host se l'indirizzo è assoluto, id canonico (maiuscole ammesse).
func TestMotoreADalMotoreA(t *testing.T) {
	id := uuid.MustParse("00000000-0000-4000-8000-0000000000ab")
	req := func(v string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/thread/"+id.String()+"/riprova-copie", nil)
		r.Host = "cockpit.acme.example"
		if v != "" {
			r.Header.Set("HX-Current-URL", v)
		}
		return r
	}
	for _, v := range []string{
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a",
		"https://cockpit.acme.example/thread/" + id.String() + "/motore-a",
		"https://COCKPIT.acme.example/thread/" + id.String() + "/motore-a",
		"http://cockpit.acme.example/thread/" + strings.ToUpper(id.String()) + "/motore-a",
		"/thread/" + id.String() + "/motore-a",
	} {
		if !dalMotoreA(req(v), id) {
			t.Errorf("%q: atteso vero", v)
		}
	}
	senzaTrattini := strings.ReplaceAll(id.String(), "-", "")
	for _, v := range []string{
		"",
		"%zz",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a/",                           // la / finale
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a?vista=documenti&prodotto=x", // la query
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a?",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a#documenti", // il frammento
		"https://altro.acme.example/thread/" + id.String() + "/motore-a",            // un altro host
		"//altro.acme.example/thread/" + id.String() + "/motore-a",                  // senza schema, altro host
		"ftp://cockpit.acme.example/thread/" + id.String() + "/motore-a",            // un altro schema
		"http://utente@cockpit.acme.example/thread/" + id.String() + "/motore-a",    // con le credenziali
		"http:/thread/" + id.String() + "/motore-a",                                 // schema senza host
		"http://cockpit.acme.example/thread/{" + id.String() + "}/motore-a",         // graffe
		"http://cockpit.acme.example/thread/urn:uuid:" + id.String() + "/motore-a",  // urn
		"http://cockpit.acme.example/thread/" + senzaTrattini + "/motore-a",         // 32 cifre
		"http://cockpit.acme.example/thread/" + id.String() + "%2Fmotore-a",         // / codificata
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a/dati",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-abc",
		"http://cockpit.acme.example/thread/" + id.String() + "/distinta",
		"http://cockpit.acme.example/thread/" + id.String() + "/fascicolo",
		"http://cockpit.acme.example/thread/" + id.String(),
		"http://cockpit.acme.example/thread/00000000-0000-4000-8000-000000000002/motore-a", // un'altra RFQ
		"http://cockpit.acme.example/x/thread/" + id.String() + "/motore-a",
		"http://cockpit.acme.example/thread/7120010/motore-a",
		"http://cockpit.acme.example/thread/" + id.String() + "//motore-a",
	} {
		if dalMotoreA(req(v), id) {
			t.Errorf("%q: atteso falso", v)
		}
	}
}

// TestMotoreALaRispostaDelGesto: la scrittura dell'esito (scriviEsitoMotoreA). Le intestazioni HX-* messe prima da un
// gestore legacy (l'HX-Trigger di pannelloConAvviso su /proposta/{u}/scarta) si tolgono (P3-GO-02, Q3-05); la catena
// vera la prova la L4 (gestoPonte, P3-L4-24).
func TestMotoreALaRispostaDelGesto(t *testing.T) {
	for _, c := range []struct{ avviso, corpo string }{
		{"Proposta scartata.", `<div data-ma-esito="ok">Proposta scartata.</div>`},
		{"", `<div data-ma-esito="ok"></div>`}, // il ponte scrive il suo «Fatto»
		{"Niente è cambiato: la proposta non è più aperta.", `<div data-ma-esito="no">Niente è cambiato: la proposta non è più aperta.</div>`},
		// senza rifiutiMotoreA (Q-A1d-09 = A): «Nessun file scelto» resta «ok», come in avvisoNegativo (TEC-04)
		{"Nessun file scelto: niente da assegnare.", `<div data-ma-esito="ok">Nessun file scelto: niente da assegnare.</div>`},
		{`Nota su 7120110.pdf: <b>quota</b> & "foro" l'angolo`,
			`<div data-ma-esito="ok">Nota su 7120110.pdf: &lt;b&gt;quota&lt;/b&gt; &amp; &#34;foro&#34; l&#39;angolo</div>`},
		{`<div data-ma-esito="ok">finto</div>`, `<div data-ma-esito="ok">&lt;div data-ma-esito=&#34;ok&#34;&gt;finto&lt;/div&gt;</div>`},
		{`Niente è cambiato: <script>alert(1)</script>`, `<div data-ma-esito="no">Niente è cambiato: &lt;script&gt;alert(1)&lt;/script&gt;</div>`},
	} {
		rec := httptest.NewRecorder()
		rec.Header().Set("HX-Trigger", "inbox-aggiorna") // come pannelloConAvviso prima di threadFrammento
		rec.Header().Set("HX-Redirect", "/altrove")
		scriviEsitoMotoreA(rec, c.avviso)
		if rec.Code != http.StatusOK {
			t.Errorf("%q: stato %d, atteso 200", c.avviso, rec.Code)
		}
		if g := rec.Header().Get("Content-Type"); g != "text/html; charset=utf-8" {
			t.Errorf("%q: Content-Type %q", c.avviso, g)
		}
		if g := rec.Header().Get("Cache-Control"); g != "no-store" {
			t.Errorf("%q: Cache-Control %q", c.avviso, g)
		}
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "hx-") {
				t.Errorf("%q: intestazione %s nella risposta", c.avviso, k)
			}
		}
		corpo := rec.Body.String()
		if corpo != c.corpo {
			t.Errorf("%q: corpo\n%s\natteso\n%s", c.avviso, corpo, c.corpo)
		}
		// un solo elemento: l'apertura e la chiusura del div sono gli unici «<» del corpo
		if strings.Count(corpo, "<") != 2 || !strings.HasPrefix(corpo, `<div data-ma-esito="`) || !strings.HasSuffix(corpo, "</div>") {
			t.Errorf("%q: il corpo non è un solo <div data-ma-esito>: %s", c.avviso, corpo)
		}
	}
}

// TestMotoreAEsitoEAvvisoNegativo: l'esito del ramo è avvisoNegativo così com'è, frase per frase (Q-A1d-09 = A): «no»
// esattamente quando avvisoNegativo dice vero, su tutte le frasi della caratterizzazione e su qualche variante.
func TestMotoreAEsitoEAvvisoNegativo(t *testing.T) {
	frasi := []string{"", "Niente è cambiato", "niente è cambiato: minuscolo", "  Niente è cambiato: con spazi davanti",
		"Fatto, nessun cambiamento", "Nessun cambiamento: maiuscolo", "Conferma non riuscita: x", "Conferma riuscita"}
	for _, r := range caratterizzazioneAvvisoNegativo() {
		frasi = append(frasi, r.frase)
	}
	for _, f := range frasi {
		atteso := "ok"
		if avvisoNegativo(f) {
			atteso = "no"
		}
		if g := esitoMotoreA(f); g != atteso {
			t.Errorf("%q: esito %q, avvisoNegativo dice %q", f, g, atteso)
		}
	}
}

// TestThreadFrammentoRispondeAllaMotoreA: a flag spento, da un'altra pagina, da un'altra RFQ o con un indirizzo non
// esatto il ramo non scatta e la risposta non è il div. Il ramo, quando scatta, legge l'esistenza della RFQ (GetThread,
// P3-GO-03): qui il Pool è nil e ogni lettura va in panic, quindi la risposta del ramo con il database (il div, il 404
// per una RFQ che non c'è, le regole in errore di TEC-01) la provano le L4 (gestoPonte in ogni TestPonteL4*, che
// accendono il flag con le regole in errore; TestPonteL4RFQInesistenteECaratterizzazioniB7). Qui: i casi negativi, e
// che a flag acceso dalla pagina esatta il ramo scatti prima della vecchia interfaccia (la risposta non porta niente
// del thread_corpo e le intestazioni HX-* messe prima sono già tolte quando la lettura cade).
func TestThreadFrammentoRispondeAllaMotoreA(t *testing.T) {
	id := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	pagina := "http://cockpit.acme.example/thread/" + id.String() + "/motore-a"
	chiama := func(s *Server, corrente, avviso string) (corpo string, h http.Header, caduta bool) {
		req := httptest.NewRequest(http.MethodPost, "/thread/"+id.String()+"/fascicolo/nodo/00000000-0000-4000-8000-000000000030/scarta", nil)
		req.Host = "cockpit.acme.example"
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", corrente)
		rec := httptest.NewRecorder()
		rec.Header().Set("HX-Trigger", "inbox-aggiorna")
		defer func() {
			caduta = recover() != nil // senza database ogni lettura cade
			corpo, h = rec.Body.String(), rec.Header()
		}()
		s.threadFrammento(rec, req, id, avviso)
		return rec.Body.String(), rec.Header(), false
	}
	acceso := &Server{MotoreA: motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta}}
	if g, h, caduta := chiama(acceso, pagina, "7121007 scartato."); !caduta || g != "" || h.Get("HX-Trigger") != "" {
		t.Errorf("acceso, dalla pagina: il ramo doveva togliere HX-Trigger e leggere l'esistenza della RFQ: caduta %v, corpo %q, HX-Trigger %q",
			caduta, g, h.Get("HX-Trigger"))
	}
	for nome, c := range map[string]struct {
		s        *Server
		corrente string
	}{
		"spento":          {&Server{}, pagina},
		"spento, regole":  {&Server{MotoreA: motorea_web.Regole{Accesa: false, Stato: motorea_web.StatoSpenta}}, pagina},
		"altra pagina":    {acceso, "http://cockpit.acme.example/thread/" + id.String() + "/distinta"},
		"il Fascicolo":    {acceso, "http://cockpit.acme.example/thread/" + id.String() + "/fascicolo"},
		"altra RFQ":       {acceso, "http://cockpit.acme.example/thread/00000000-0000-4000-8000-000000000002/motore-a"},
		"le dati JSON":    {acceso, "http://cockpit.acme.example/thread/" + id.String() + "/motore-a/dati"},
		"senza la pagina": {acceso, ""},
		"con la query":    {acceso, pagina + "?vista=documenti"},
		"barra finale":    {acceso, pagina + "/"},
		"altro host":      {acceso, "http://altro.acme.example/thread/" + id.String() + "/motore-a"},
	} {
		// il ramo toglie l'HX-Trigger messo prima; il percorso legacy lo lascia
		if g, h, _ := chiama(c.s, c.corrente, "7121007 scartato."); strings.Contains(g, "data-ma-esito") || h.Get("HX-Trigger") != "inbox-aggiorna" {
			t.Errorf("%s: il ramo motore-a è scattato: %q, HX-Trigger %q", nome, g, h.Get("HX-Trigger"))
		}
	}
	// il ramo è il primo di threadFrammento, prima della Distinta e del Fascicolo
	src, err := os.ReadFile("thread.go")
	if err != nil {
		t.Fatal(err)
	}
	_, corpo, ok := strings.Cut(strings.ReplaceAll(string(src), "\r\n", "\n"), "func (s *Server) threadFrammento(")
	if !ok {
		t.Fatal("manca threadFrammento")
	}
	m, d := strings.Index(corpo, "if s.MotoreA.Accesa && dalMotoreA(r, id) {"), strings.Index(corpo, "dallaDistinta(r.Header, id)")
	if m < 0 || d < 0 || m > d {
		t.Error("il ramo motore-a non è il primo di threadFrammento")
	}
}

// ---------------------------------------------------------------- la caratterizzazione (mappa §3, CQ-01)

// rigaCaratterizzazione: una frase di un gesto del ponte com'è oggi.
//   - n: la riga del §3 della mappa; gesto: il gesto o i gesti;
//   - frase: la frase (costruita con le stesse funzioni del codice dove si può);
//   - attuale: la lettura di avvisoNegativo oggi («ok» / «no»), quella che la prova FISSA;
//   - corretta: la lettura che il contratto vorrebbe («ok» / «no» / «errore»), solo documentata;
//   - categoria: la categoria di Q-A1d-09;
//   - fonte, frammento: il file (relativo a questo package) e un pezzo letterale del testo, per la guardia dei testi.
type rigaCaratterizzazione struct {
	n                 int
	gesto, frase      string
	attuale, corretta string
	categoria         string
	fonte, frammento  string
}

const (
	catRifiuto      = "rifiuto letto «no»"
	catRifiutoOk    = "rifiuto con 200 letto «ok»"
	catErrore       = "errore tecnico letto come rifiuto"
	catErroreOk     = "errore tecnico dentro un «ok»"
	catParziale     = "successo parziale letto «ok»"
	catIdempotente  = "idempotente"
	catSuccesso     = "successo"
	catTestoNeg     = "successo con testo negativo letto «no»"
	catIdempotenteN = "idempotente letto «no»"
)

const dirFascicolo = "../../core/rfq/fascicolo/"

func caratterizzazioneAvvisoNegativo() []rigaCaratterizzazione {
	deadlock := "Niente è cambiato: " + spiegaErrore(&pgconn.PgError{Code: "40P01"})
	congelata := "Niente è cambiato: " + spiegaErrore(&pgconn.PgError{Code: "BOM01"})
	return []rigaCaratterizzazione{
		{1, "tutti i gesti con s.gesto", "Niente è cambiato: 7121007: la proposta è già decisa", "no", "no", catRifiuto + " (+ rismistamento dopo il rifiuto apparente)",
			"proposte.go", `msg := "Niente è cambiato: " + spiegaErrore(err)`},
		{2, "tutti i gesti con s.gesto", deadlock, "no", "errore", catErrore + " (40P01)",
			"fascicolo.go", "un'altra operazione sulla stessa RFQ era in corso nello stesso momento: riprova"},
		{2, "tutti i gesti con s.gesto", congelata, "no", "no", catRifiuto + " (BOM congelata, dal trigger)",
			"fascicolo.go", "la BOM è congelata: si modifica solo aprendo una revisione"},
		{2, "tutti i gesti con s.gesto (Commit fallito)", "Niente è cambiato: " + spiegaErrore(errors.New("conn closed")), "no", "errore", catErrore + " (Commit fallito: «niente è cambiato» può essere falso)",
			"proposte.go", `msg := "Niente è cambiato: " + spiegaErrore(err)`},
		{3, "conferma documento (con HX: 422/500 calcolati ignorati)", "Niente è cambiato: " + spiegaErrore(errors.New("x")), "no", "errore", catErrore,
			"fascicolo_conferma.go", `msg = "Niente è cambiato: " + spiegaErrore(err)`},
		{4, "codice del componente", "Correzione non riuscita, niente è cambiato: x", "no", "errore", catErrore,
			"fascicolo.go", `"Correzione non riuscita, niente è cambiato: "`},
		{5, "assegna", "Assegnazione non riuscita, nessun file cambiato: x", "no", "errore", catErrore,
			"fascicolo.go", `"Assegnazione non riuscita, nessun file cambiato: "`},
		{6, "assegna senza file", "Nessun file scelto: niente da assegnare.", "ok", "no", catRifiutoOk,
			"fascicolo.go", `"Nessun file scelto: niente da assegnare."`},
		{7, "riprova copie, capacità spenta", esitoCopie{Spenta: "nas_scrittura"}.Frase(), "ok", "no", catRifiutoOk,
			"thread.go", "Copie NON rimesse in coda: la capacita' [sicurezza].%s e' spenta su questo server."},
		{8, "riprova copie, niente accodato per errori", esitoCopie{Errori: 1, Motivo: "la coda non risponde"}.Frase(), "ok", "errore", catErroreOk,
			"thread.go", `return "Nessuna copia nuova" + dettaglio + "."`},
		{9, "riprova copie, parziale", esitoCopie{Accodati: 2, Errori: 1, Motivo: "x"}.Frase(), "ok", "ok (con avvertenza)", catParziale,
			"thread.go", `"%d non accodat%s: %s"`},
		{10, "riprova copie, niente da fare", esitoCopie{}.Frase(), "ok", "ok", catIdempotente,
			"thread.go", "Nessun documento in attesa: questa RFQ non ha documenti confermati."},
		{10, "riprova copie, già in attesa", esitoCopie{GiaInCoda: 2}.Frase(), "ok", "ok", catIdempotente,
			"thread.go", `"%d gia' in attesa"`},
		{10, "riprova copie, già sul NAS", esitoCopie{NonApplicabili: 3}.Frase(), "ok", "ok", catIdempotente,
			"thread.go", "Nessun documento in attesa: sono tutti gia' sul NAS (%d)."},
		{11, "aggiorna smistamento, RFQ chiusa", "Le proposte di una RFQ " + fascicolo.EsclusaChiusa + " non si aggiornano.", "ok", "no", catRifiutoOk,
			"smistamento.go", `avviso = "Le proposte di una RFQ " + es.Saltata + " non si aggiornano."`},
		{11, "aggiorna smistamento, RFQ unita", "Le proposte di una RFQ " + fascicolo.EsclusaUnita + " non si aggiornano.", "ok", "no", catRifiutoOk,
			"smistamento.go", `avviso = "Le proposte di una RFQ " + es.Saltata + " non si aggiornano."`},
		{12, "aggiorna smistamento", "Le proposte erano già aggiornate (3 file).", "ok", "ok", catIdempotente,
			"smistamento.go", `"Le proposte erano già aggiornate (%s)."`},
		{13, "aggiorna smistamento, errore", "Niente è cambiato: " + spiegaErrore(errors.New("x")), "no", "errore", catErrore,
			"smistamento.go", `avviso = "Niente è cambiato: " + spiegaErrore(err)`},
		{14, "prepara", "Niente da preparare: nessun file da scaricare, estrarre o analizzare.", "ok", "ok", catIdempotente,
			"fascicolo_preparazione.go", `"Niente da preparare: nessun file da scaricare, estrarre o analizzare."`},
		{15, "prepara, parziale", frasePreparazione(fascicolo.Preparazione{Download: 1, Rimandati: 2, Saltati: []string{"7120001.pdf: link scaduto"}},
			fascicolo.Rianalisi{SenzaStaging: 1}), "ok", "ok (con avvertenza)", catParziale,
			"fascicolo_preparazione.go", `frase += " Non scaricati da soli: "`},
		{16, "prepara, errore (con HX)", "Niente è cambiato: " + spiegaErrore(errors.New("x")), "no", "errore", catErrore + " (+ rismistamento dei file segnati prima del rollback [T])",
			"fascicolo_preparazione.go", `avviso = "Niente è cambiato: " + spiegaErrore(err)`},
		{16, "prepara, errore (senza HX)", "Preparazione dei file non riuscita. Niente è cambiato: x", "no", "errore", catErrore,
			"fascicolo_preparazione.go", `"Preparazione dei file non riuscita. "`},
		{17, "rianalizza senza analizzatore", "Niente è cambiato: " + spiegaErrore(rifiuto("l'analisi non è configurata ([analisi].versione)")), "no", "no", catRifiuto + " (+ rismistamento dopo il rifiuto apparente)",
			"proposte.go", `rifiuto("l'analisi non è configurata ([analisi].versione)")`},
		{18, "rianalizza", "Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente.", "ok", "ok", catIdempotente,
			"proposte.go", `"Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente."`},
		{19, "codice del componente uguale", "Nessun cambiamento: il codice è già 7120010.", "ok", "ok", catIdempotente + " («Nessun» maiuscolo: né prefisso né Contains)",
			"fascicolo.go", `"Nessun cambiamento: il codice è già "`},
		{20, "rev e descrizione uguali", "Nessun cambiamento: 7120010 è già così.", "ok", "ok", catIdempotente,
			dirFascicolo + "modifiche.go", `"Nessun cambiamento: " + c.Codice + " è già così."`},
		{21, "collega, arco già uguale", "7120010 è già sotto 7120100 ×2.", "ok", "ok", catIdempotente,
			dirFascicolo + "modifiche.go", `"%s è già sotto %s ×%d."`},
		{22, "accetta nodo, codice ritrovato", "7120010 è già nella BOM: la proposta lo ritrova.", "ok", "ok (ma il tipo scelto è ignorato: TEC-12)", catSuccesso + " (duplicato)",
			dirFascicolo + "decisioni.go", `"%s è già nella BOM: la proposta lo ritrova."`},
		{23, "riapri nodo già riaperto", "7121007 riaperto: torna fra le proposte.", "ok", "ok", catIdempotente,
			dirFascicolo + "albero_conferma.go", `frase := nome + " riaperto: torna fra le proposte"`},
		{24, "autorizza STEP già autorizzato", "7120010.stp è già lo STEP autorizzato di 7120010.", "ok", "ok", catIdempotente + " (rilegge comunque i file)",
			dirFascicolo + "autorizzazione.go", `"%s è già lo STEP autorizzato di %s"`},
		{25, "autorizza STEP, autorizzazione che non vale", "7120010.stp è lo STEP autorizzato di 7120010: 2 figli diretti nell'autorità, 1 nodo resta guida. L'autorizzazione non vale: la radice non è nel file.",
			"ok", "ok (con avvertenza)", catParziale, dirFascicolo + "autorizzazione.go", `"l'autorizzazione non vale: "+y.Problema`},
		{25, "autorizza STEP, rimozioni non calcolate", "7120010.stp è lo STEP autorizzato di 7120010: 2 figli diretti nell'autorità, 1 nodo resta guida. Rimozioni non calcolate: l'autorizzazione non vale: x.",
			"ok", "ok (con avvertenza)", catParziale, dirFascicolo + "autorizzazione.go", `"rimozioni non calcolate: "+es.Sospese`},
		{26, "accetta rimozione, arco già assente", "7120010 → 7121003: l'arco non c'era già più nella BOM working.", "ok", "ok", catIdempotente + " (decisione registrata)",
			dirFascicolo + "decisioni.go", `": l'arco non c'era già più nella BOM working."`},
		{27, "conferma documento, copia in attesa", "Fascicolo confermato: 1 documento (copie sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta).", "ok", "ok", catSuccesso,
			"fascicolo_conferma.go", `"copie sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta"`},
		{27, "conferma documento, file già presente", "Fascicolo confermato: 1 file già nel fascicolo: registrata la provenienza.", "ok", "ok", catIdempotente,
			"fascicolo_conferma.go", `"file già nel fascicolo: registrata la provenienza"`},
		{28, "scarta file già deciso", "La proposta era già stata decisa: nessun cambiamento.", "no", "no (scelta legacy, bug 7 del 29/09)", catIdempotenteN + " (incoerente con 19-21; + rismistamento)",
			"allegati.go", `"La proposta era già stata decisa: nessun cambiamento."`},
		{29, "deroga con un motivo libero", "7120010: disegno_2d derogato («nessun cambiamento sul disegno»).", "no", "ok", catTestoNeg + " (la pagina non si rilegge dopo la scrittura)",
			dirFascicolo + "modifiche.go", `"%s: %s derogato («%s»)."`},
		{30, "scarta nodo con un nome grezzo libero", "staffa nessun cambiamento 2 scartato.", "no", "ok", catTestoNeg + " (teorico)",
			dirFascicolo + "decisioni.go", `nomeNodo(p)+" scartato."`},
	}
}

// TestCaratterizzazioneAvvisoNegativoSuiGestiDelPonte (CQ-01): la lettura ATTUALE di ogni frase del §3, righe 1-30. Le
// righe 31-34 non passano da avvisoNegativo: 31 (400/404/500 testuali) e 33 (Conferma l'albero, dallo stato HTTP) le
// legge il ponte (motore_a_ponte_test.go e le L4), 32 (thread_corpo a flag spento o senza il ramo) è
// TestThreadFrammentoRispondeAllaMotoreA e P3-L4-00, 34 (riprova su una RFQ inesistente) non è più la riga 10: dopo la
// revisione di P3 il ramo risponde 404 come il legacy (P3-GO-03, TestPonteL4RFQInesistenteECaratterizzazioniB7).
func TestCaratterizzazioneAvvisoNegativoSuiGestiDelPonte(t *testing.T) {
	righe := caratterizzazioneAvvisoNegativo()
	viste := map[int]bool{}
	disallineate := 0
	for _, r := range righe {
		viste[r.n] = true
		if g := esitoMotoreA(r.frase); g != r.attuale {
			t.Errorf("riga %d (%s), %q: letta %q, la caratterizzazione dice %q (%s)", r.n, r.gesto, r.frase, g, r.attuale, r.categoria)
		}
		if !strings.HasPrefix(r.corretta, r.attuale) {
			disallineate++
		}
	}
	for n := 1; n <= 30; n++ {
		if !viste[n] {
			t.Errorf("la riga %d del §3 non è caratterizzata", n)
		}
	}
	// le righe in cui la lettura di oggi non è quella del contratto: i disallineamenti registrati (TEC-02, -04, -06)
	if disallineate != 15 {
		t.Errorf("righe disallineate: %d, la caratterizzazione ne registra 15 (se il numero cambia, aggiornare la mappa §3)", disallineate)
	}
}

// TestCaratterizzazioneAvvisoNegativoITestiCiSonoAncora: la guardia dei testi. Ogni frase caratterizzata ha il suo
// pezzo letterale nel sorgente da cui viene (questo package o core/rfq/fascicolo): se il testo cambia, la riga va
// rivista insieme alla lettura.
func TestCaratterizzazioneAvvisoNegativoITestiCiSonoAncora(t *testing.T) {
	letti := map[string]string{}
	for _, r := range caratterizzazioneAvvisoNegativo() {
		src, ok := letti[r.fonte]
		if !ok {
			b, err := os.ReadFile(filepath.FromSlash(r.fonte))
			if err != nil {
				t.Errorf("riga %d: %v", r.n, err)
				continue
			}
			src = strings.ReplaceAll(string(b), "\r\n", "\n")
			letti[r.fonte] = src
		}
		if !strings.Contains(src, r.frammento) {
			t.Errorf("riga %d (%s): in %s non c'è più %q", r.n, r.gesto, r.fonte, r.frammento)
		}
	}
	// e avvisoNegativo è ancora quello caratterizzato: sei prefissi e «nessun cambiamento» minuscolo
	b, err := os.ReadFile("distinta.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, v := range []string{`"Niente è cambiato", "Assegnazione non riuscita", "Conferma non riuscita", "Correzione non riuscita", "Preparazione dei file non riuscita",`,
		`"La proposta era già stata decisa"}`, `return strings.Contains(a, "nessun cambiamento")`} {
		if !strings.Contains(src, v) {
			t.Errorf("avvisoNegativo non è più quello caratterizzato: manca %q", v)
		}
	}
}
