package web

// L1 — giro 5, A1d: la Distinta nuova del motore A, motore_a.go (BE-07, BE-12, BE-17; contratto A1d §2, §3.1; v1.1
// §4.10; piano A1d backend §5.1, §5.2, §6, §7.2; P-07 b, P-12; T-A1d-02, T-A1d-07, T-A1d-11, T-A1d-12, T-A1d-13,
// T-A1d-15, T-A1d-17, T-A1d-18, T-A1d-36, T-A1d-39; RAC-05, RAC-14, RAC-21, RAC-25).
//
// La risposta dei gesti: rispondiMotoreA scrive un solo <div data-ma-esito="ok|no"> con il testo passato dall'escape;
// esitoMotoreA dice «no» per le frasi di avvisoNegativo e per le 9 dell'elenco chiuso, «ok» per le idempotenti;
// dallaMotoreA riconosce la pagina dal percorso di HX-Current-URL, con l'UUID confrontato come UUID; threadFrammento ha
// il ramo solo a flag acceso. La guardia statica legge i sorgenti del pacchetto: ogni funzione che arriva a
// threadFrammento è classificata (rotta del ponte, rotta fuori dal ponte, inoltro), e ogni frase con l'iniziale
// maiuscola che vi compare ha la sua classe; una frase nuova senza classe fa cadere la prova (T-A1d-39). Le risposte del
// kit del ponte (testdata/motore_a/kit/prove/risposte_acme.json) sono quelle che il server scrive, byte per byte: il
// kit jsdom (ponte.cjs) prova il ponte sulle stesse.
//
// La porta e i dati: a flag spento 404 a tutti prima della sessione; acceso, la sessione e poi il ruolo admin; le rotte
// montate come conMotoreA(soloAdmin(…)); il calcolo con il limite e il recover; la versione dell'esito; la pagina
// composta sulla scena ACME dell'indice (scenaIndiceACME, motore_a_indice_test.go); il JSON di /dati; HEAD senza
// calcolo; la fase e l'esistenza della RFQ in una transazione READ ONLY con un iniziatore finto; nessuna scrittura e
// nessuna tabella dell'agente nei sorgenti motore_a*.go.
//
// Sta a L1: nessun database, nessun server (httptest); il Pool è nil e una lettura del database va in panic, che le
// prove recuperano e dicono. Le prove con le sessioni e le RFQ vere (403 per ruolo, 404 di una RFQ che non c'è, le GET
// che non scrivono) sono A1d-L4-01…03 in motore_a_db_test.go (A1d-Q3). I clienti di questi test sono inventati: codici
// 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn, acme.example.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// maRifiuti9: le 9 frasi dell'elenco chiuso (T-A1d-11), come le scrive il codice, una per voce di rifiutiMotoreA e
// nello stesso ordine (piano A1d backend §6.1-§6.3).
var maRifiuti9 = []string{
	esitoCopie{Spenta: "nas_scrittura"}.Frase(),                   // thread.go, riprova-copie
	esitoCopie{Errori: 1, Motivo: "la coda non risponde"}.Frase(), // thread.go, riprova-copie: niente accodato
	"Le proposte di una RFQ " + fascicolo.EsclusaChiusa + " non si aggiornano.",
	"Nessun file scelto: niente da assegnare.",
	"Prima crea la RFQ o aggancia il messaggio a una RFQ esistente: i file scaricati vanno nella sua cartella.",
	"Scegli un fornitore.",
	"Fornitore non trovato.",
	"Richiesta non creata: la lavorazione non esiste",
	"Richiesta non trovata in questa RFQ.",
}

func TestMotoreALaRispostaDelGesto(t *testing.T) {
	s := &Server{}
	casi := []struct{ avviso, corpo string }{
		{"Proposta scartata.", `<div data-ma-esito="ok">Proposta scartata.</div>`},
		{"", `<div data-ma-esito="ok"></div>`}, // il ponte scrive il testo di P-09
		{"Niente è cambiato: la proposta non è più aperta.", `<div data-ma-esito="no">Niente è cambiato: la proposta non è più aperta.</div>`},
		{"Nessun file scelto: niente da assegnare.", `<div data-ma-esito="no">Nessun file scelto: niente da assegnare.</div>`},
		{`Nota su 7120110.pdf: <b>quota</b> & "foro" l'angolo`,
			`<div data-ma-esito="ok">Nota su 7120110.pdf: &lt;b&gt;quota&lt;/b&gt; &amp; &#34;foro&#34; l&#39;angolo</div>`},
		{`<div data-ma-esito="ok">finto</div>`, `<div data-ma-esito="ok">&lt;div data-ma-esito=&#34;ok&#34;&gt;finto&lt;/div&gt;</div>`},
	}
	for _, c := range casi {
		rec := httptest.NewRecorder()
		s.rispondiMotoreA(rec, c.avviso)
		if rec.Code != http.StatusOK {
			t.Errorf("%q: stato %d, atteso 200", c.avviso, rec.Code)
		}
		if g := rec.Header().Get("Content-Type"); g != "text/html; charset=utf-8" {
			t.Errorf("%q: Content-Type %q", c.avviso, g)
		}
		if g := rec.Header().Get("Cache-Control"); g != "no-store" {
			t.Errorf("%q: Cache-Control %q", c.avviso, g)
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

func TestMotoreAIRifiutiCon200SonoNo(t *testing.T) {
	if len(maRifiuti9) != 9 || len(rifiutiMotoreA) != 9 {
		t.Fatalf("l'elenco chiuso ha %d voci e le frasi della prova %d: devono essere 9 (T-A1d-11)", len(rifiutiMotoreA), len(maRifiuti9))
	}
	// ogni voce dell'elenco chiuso ha la sua frase, e avvisoNegativo da solo la legge «ok»: la voce serve
	for i, f := range maRifiuti9 {
		r := rifiutiMotoreA[i]
		if !strings.HasPrefix(f, r.prefisso) || !strings.Contains(f, r.contiene) {
			t.Errorf("la frase %d %q non è quella della voce %q/%q", i, f, r.prefisso, r.contiene)
		}
		if avvisoNegativo(f) {
			t.Errorf("%q: avvisoNegativo la legge già «no», la voce %d dell'elenco chiuso non servirebbe", f, i)
		}
		if g := esitoMotoreA(f); g != "no" {
			t.Errorf("rifiuto dell'elenco chiuso %q: esito %q, atteso «no»", f, g)
		}
	}
	no := []string{
		"Le proposte di una RFQ " + fascicolo.EsclusaUnita + " non si aggiornano.",
		esitoCopie{Errori: 2, Motivo: "x"}.Frase(),
		esitoCopie{GiaInCoda: 1, Errori: 1, Motivo: "x", NonApplicabili: 1}.Frase(),
		"Prima crea o aggancia la RFQ.",
		// le frasi di avvisoNegativo (distinta.go)
		"Niente è cambiato: la proposta non è più aperta.",
		"Assegnazione non riuscita, nessun file cambiato: x",
		"Conferma non riuscita: x",
		"Correzione non riuscita, niente è cambiato: x",
		"Preparazione dei file non riuscita. x",
		"La proposta era già stata decisa: nessun cambiamento.",
	}
	for _, f := range no {
		if g := esitoMotoreA(f); g != "no" {
			t.Errorf("rifiuto %q: esito %q, atteso «no»", f, g)
		}
	}
	// le idempotenti e i successi restano «ok»: il gesto è andato e lo stato è quello chiesto
	ok := []string{
		"",
		"Proposta scartata.",
		"Nessun cambiamento: il codice è già 7120010.",
		"Nessun cambiamento: 7120010 è già così.",
		"7120010 è già sotto 7120100 ×2.",
		"Niente da preparare: nessun file da scaricare, estrarre o analizzare.",
		"Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente.",
		"Le proposte erano già aggiornate (3 file).",
		"Proposte aggiornate: 2 destinazioni riscritte su 3 file.",
		esitoCopie{}.Frase(),
		esitoCopie{GiaInCoda: 2}.Frase(),
		esitoCopie{NonApplicabili: 3}.Frase(),
		esitoCopie{Accodati: 1, Errori: 1, Motivo: "x"}.Frase(),
		esitoCopie{Accodati: 2}.Frase(),
		"Fascicolo confermato: 3 documenti.",
		"Richiesta a ACME Forniture creata.",
	}
	for _, f := range ok {
		if g := esitoMotoreA(f); g != "ok" {
			t.Errorf("%q: esito %q, atteso «ok»", f, g)
		}
	}
}

func TestMotoreADallaMotoreA(t *testing.T) {
	id := uuid.MustParse("00000000-0000-4000-8000-0000000000ab")
	h := func(v string) http.Header {
		x := http.Header{}
		if v != "" {
			x.Set("HX-Current-URL", v)
		}
		return x
	}
	vero := []string{
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a/",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a?vista=documenti&prodotto=x",
		"http://cockpit.acme.example/thread/" + strings.ToUpper(id.String()) + "/motore-a",
		"/thread/" + id.String() + "/motore-a",
	}
	for _, v := range vero {
		if !dallaMotoreA(h(v), id) {
			t.Errorf("%q: atteso vero", v)
		}
	}
	falso := []string{
		"",
		"%zz",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-a/dati",
		"http://cockpit.acme.example/thread/" + id.String() + "/motore-abc",
		"http://cockpit.acme.example/thread/" + id.String() + "/distinta",
		"http://cockpit.acme.example/thread/" + id.String() + "/fascicolo",
		"http://cockpit.acme.example/thread/" + id.String(),
		"http://cockpit.acme.example/thread/00000000-0000-4000-8000-000000000002/motore-a",
		"http://cockpit.acme.example/x/thread/" + id.String() + "/motore-a",
		"http://cockpit.acme.example/thread/7120010/motore-a",
	}
	for _, v := range falso {
		if dallaMotoreA(h(v), id) {
			t.Errorf("%q: atteso falso", v)
		}
	}
}

// TestMotoreALeRisposteDelKitSonoQuelleDelServer: i corpi con «avviso» di risposte_acme.json (il kit del ponte,
// QFE-04) sono, byte per byte e con le intestazioni, quelli di rispondiMotoreA; le 9 frasi «rifiuti» del kit sono
// quelle dell'elenco chiuso, una per voce (RAC-14, RAC-25).
func TestMotoreALeRisposteDelKitSonoQuelleDelServer(t *testing.T) {
	b, err := os.ReadFile("testdata/motore_a/kit/prove/risposte_acme.json")
	if err != nil {
		t.Fatal(err)
	}
	type voce struct {
		Stato        int               `json:"stato"`
		Intestazioni map[string]string `json:"intestazioni"`
		Avviso       *string           `json:"avviso"`
		Esito        string            `json:"esito"`
		Corpo        string            `json:"corpo"`
	}
	var kit struct {
		Pagina  string          `json:"pagina"`
		Thread  string          `json:"thread"`
		Gesti   map[string]voce `json:"gesti"`
		Rifiuti []voce          `json:"rifiuti"`
	}
	if err := json.Unmarshal(b, &kit); err != nil {
		t.Fatal(err)
	}
	if !dallaMotoreA(http.Header{"Hx-Current-Url": {kit.Pagina}}, uuid.MustParse(kit.Thread)) {
		t.Errorf("la pagina del kit %q non è la pagina motore-a della RFQ %s", kit.Pagina, kit.Thread)
	}
	s := &Server{}
	prova := func(nome string, v voce) {
		if v.Avviso == nil {
			return
		}
		rec := httptest.NewRecorder()
		s.rispondiMotoreA(rec, *v.Avviso)
		if rec.Code != v.Stato || rec.Body.String() != v.Corpo {
			t.Errorf("%s: il server scrive %d %s, il kit %d %s", nome, rec.Code, rec.Body.String(), v.Stato, v.Corpo)
		}
		if g := esitoMotoreA(*v.Avviso); g != v.Esito {
			t.Errorf("%s: esito %q, il kit dice %q", nome, g, v.Esito)
		}
		if len(v.Intestazioni) != 2 {
			t.Errorf("%s: il kit ha %d intestazioni, il server ne scrive 2", nome, len(v.Intestazioni))
		}
		for k, x := range v.Intestazioni {
			if g := rec.Header().Get(k); g != x {
				t.Errorf("%s: intestazione %s %q, il kit %q", nome, k, g, x)
			}
		}
	}
	n := 0
	for nome, v := range kit.Gesti {
		if v.Avviso != nil {
			n++
		}
		prova("gesti."+nome, v)
	}
	if n < 5 {
		t.Errorf("il kit ha %d risposte del ramo motore-a, almeno 5 (ok, ok vuoto, escape, idempotente, no)", n)
	}
	if len(kit.Rifiuti) != 9 {
		t.Fatalf("il kit ha %d rifiuti, l'elenco chiuso 9", len(kit.Rifiuti))
	}
	for i, v := range kit.Rifiuti {
		prova(fmt.Sprintf("rifiuti[%d]", i), v)
		r := rifiutiMotoreA[i]
		if v.Avviso == nil || v.Esito != "no" || !strings.HasPrefix(*v.Avviso, r.prefisso) || !strings.Contains(*v.Avviso, r.contiene) {
			t.Errorf("rifiuti[%d] del kit non è la voce %q/%q dell'elenco chiuso", i, r.prefisso, r.contiene)
		}
	}
}

// ---------------------------------------------------------------- la guardia statica delle frasi (T-A1d-39)

// maInoltri: le funzioni che arrivano a threadFrammento senza essere il gestore di una rotta, con la parte che
// servono: «inoltro» passa la frase di chi la chiama (le frasi che scrive da sé sono dei gesti che la usano); «fuori»
// serve solo una rotta fuori dal ponte.
var maInoltri = map[string]struct{ parte, nota string }{
	"allegati.go:pannelloConAvviso":          {"inoltro", "ritorna_thread: il pannello del messaggio, o la pagina della RFQ"},
	"proposte.go:gesto":                      {"inoltro", "i gesti con una transazione e la frase del dominio"},
	"proposte.go:gestoPoi":                   {"inoltro", "idem, con il passo dopo il commit"},
	"fascicolo_tipo.go:gestoConEsito":        {"inoltro", "Cambia tipo: con htmx è gesto, senza il 422 del Fascicolo"},
	"fascicolo_conferma.go:rispostaConferma": {"inoltro", "Conferma Fascicolo: con htmx e fuori dal Fascicolo, threadFrammento"},
	"triage.go:avvisoRFQEsistente":           {"fuori", "Nuova RFQ quando il messaggio era già agganciato (/messaggio/{id}/rfq)"},
}

// maFrasiAiutanti: le funzioni che compongono le frasi dei gesti senza chiamare threadFrammento (piano A1d backend
// §7.2), con la parte a cui servono.
var maFrasiAiutanti = map[string]string{
	"thread.go:Frase": "ponte", // esitoCopie, riprova-copie
	"fascicolo_preparazione.go:frasePreparazione": "ponte", // prepara
	"fascicolo.go:assegnaAlComponente":            "ponte", // assegna
	"fascicolo.go:correggiCodiceComponente":       "ponte", // componente/{u}/codice
	"fascicolo_conferma.go:applicaPiano":          "ponte", // fascicolo/conferma
	"proposte.go:fraseRianalisi":                  "ponte", // rianalizza
	"allegati.go:frase":                           "fuori", // contiDownload: scarica, riscarica
}

// Le classi delle frasi:
//
//	no      rifiuto con 200: esitoMotoreA dice «no» (avvisoNegativo o l'elenco chiuso)
//	ok      riuscito o idempotente: esitoMotoreA dice «ok»
//	fuori   rifiuto di una rotta fuori dal ponte, che esitoMotoreA legge «ok»: la pagina motore-a non la chiama mai
//	        (nessun gesto la manda con HX-Current-URL della pagina); vedi DUBBI del pezzo «ponte»
//	dipende la classe la decide il seguito (esempio «no», esempioOk «ok»)
//	pezzo   il seguito di una frase che comincia altrove (dopo un «+» o un %s): non è l'inizio di un avviso
//	altro   non è un avviso: un'intestazione, un titolo, una query
type maClasse struct {
	classe    string
	esempio   string // la frase intera, quando il letterale da solo non basta a dirne la classe
	esempioOk string // solo per «dipende»: una frase intera che resta «ok»
}

// maFrasi: ogni letterale con l'iniziale maiuscola delle funzioni che arrivano a threadFrammento e degli aiutanti,
// tolti gli argomenti di http.Error, s.Log.*, fmt.Errorf, errors.New e rifiuto(…) (piano A1d backend §6, §7.2).
var maFrasi = map[string]maClasse{
	// --- rotte del ponte (e i loro inoltri e aiutanti)
	"Niente è cambiato: ":                                                                {"no", "", ""}, // gestoPoi, prepara, confermaFascicolo, smistamento, …
	"Assegnazione non riuscita, nessun file cambiato: ":                                  {"no", "", ""}, // assegna
	"Nessun file scelto: niente da assegnare.":                                           {"no", "", ""}, // assegna, elenco chiuso
	"Correzione non riuscita, niente è cambiato: ":                                       {"no", "", ""}, // componente/{u}/codice
	"Nessun cambiamento: il codice è già ":                                               {"ok", "", ""}, // idempotente
	"Codice corretto: %s → %s (%d document%s, %d propost%s).":                            {"ok", "", ""},
	"Fascicolo confermato: ":                                                             {"ok", "", ""}, // applicaPiano
	"La proposta era già stata decisa: nessun cambiamento.":                              {"no", "", ""}, // /proposta/{u}/scarta
	"Proposta scartata.":                                                                 {"ok", "", ""},
	"Copie NON rimesse in coda: la capacita' [sicurezza].%s e' spenta su questo server.": {"no", "", ""}, // elenco chiuso
	"Nessuna copia nuova": {"dipende",
		esitoCopie{Errori: 1, Motivo: "x"}.Frase(), // niente accodato, per errori: elenco chiuso
		esitoCopie{GiaInCoda: 2}.Frase()},          // tutte già in attesa: idempotente
	"Nessun documento in attesa: questa RFQ non ha documenti confermati.":                       {"ok", "", ""},
	"Nessun documento in attesa: sono tutti gia' sul NAS (%d).":                                 {"ok", "", ""},
	"Preparazione dei file non riuscita. ":                                                      {"no", "", ""},
	"Preparazione dei file: ":                                                                   {"ok", "", ""},
	"Niente da preparare: nessun file da scaricare, estrarre o analizzare.":                     {"ok", "", ""}, // idempotente
	"Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente.": {"ok", "", ""},
	"Le proposte di una RFQ ":                                                                   {"no", "Le proposte di una RFQ " + fascicolo.EsclusaChiusa + " non si aggiornano.", ""}, // elenco chiuso
	"Le proposte erano già aggiornate (%s).":                                                    {"ok", "", ""},                                                                          // idempotente
	"Proposte aggiornate: %s su %s.":                                                            {"ok", "", ""},
	// i pezzi di fraseRianalisi e frasePreparazione: dopo il numero di conta(…), mai all'inizio
	"STEP riletto": {"pezzo", "", ""}, "STEP riletti": {"pezzo", "", ""}, "STEP rimandato": {"pezzo", "", ""},
	"STEP rimandati": {"pezzo", "", ""}, "STEP non più in staging": {"pezzo", "", ""}, "PDF accodato": {"pezzo", "", ""},
	"PDF accodati": {"pezzo", "", ""}, "PDF già in coda": {"pezzo", "", ""}, "PDF rimandato": {"pezzo", "", ""},
	"PDF rimandati": {"pezzo", "", ""}, "PDF non più in staging": {"pezzo", "", ""},
	// non sono avvisi
	"Content-Type": {"altro", "", ""}, "HX-Request": {"altro", "", ""}, "HX-Push-Url": {"altro", "", ""},
	"HX-Trigger":        {"altro", "", ""},
	"Fascicolo":         {"altro", "", ""}, // il titolo della pagina del Fascicolo
	"File già presente": {"altro", "", ""}, // applicaPiano: un confronto con strings.HasPrefix su un messaggio del dominio
	"IN ATTESA":         {"altro", "", ""}, // applicaPiano: idem, strings.Contains
	"SET CONSTRAINTS fk_documento_componente, fk_proposta_componente DEFERRED": {"altro", "", ""},

	// --- rotte fuori dal ponte (piano A1d backend §6.2): la pagina motore-a non le chiama
	"Prima crea la RFQ o aggancia il messaggio a una RFQ esistente: i file scaricati vanno nella sua cartella.": {"no", "", ""}, // elenco chiuso
	"Prima crea o aggancia la RFQ.":        {"no", "", ""}, // elenco chiuso
	"Nessun allegato da scaricare.":        {"ok", "", ""}, // idempotente
	"Conferma non riuscita: ":              {"no", "", ""},
	"Scegli un fornitore.":                 {"no", "", ""}, // elenco chiuso
	"Fornitore non trovato.":               {"no", "", ""}, // elenco chiuso
	"Richiesta non creata: ":               {"no", "", ""}, // elenco chiuso
	"Richiesta non trovata in questa RFQ.": {"no", "", ""}, // elenco chiuso
	"Richiesta non trovata.":               {"no", "", ""}, // elenco chiuso (lo stesso prefisso)
	"Richiesta a %s creata.":               {"ok", "", ""},
	"Richiesta annullata. La mail, se è partita, resta nel suo quadrante.":                                        {"ok", "", ""},
	"Registrata come richiesta a %s per la RFQ %s. La risposta del fornitore verrà proposta su questa richiesta.": {"ok", "", ""},
	"Agganciata come risposta di %s alla richiesta per la RFQ %s.":                                                {"ok", "", ""},
	"Agganciato alla RFQ %s.%s%s":                                             {"ok", "", ""},
	"RFQ creata: %s. %s%s%s":                                                  {"ok", "", ""},
	"Messaggio ignorato (lo ritrovi nel filtro «ignorati»).":                  {"ok", "", ""},
	"Candidati ricalcolati con le regole di adesso.":                          {"ok", "", ""},
	"Analisi richiesta. La proposta comparirà qui sotto quando sarà pronta: ": {"ok", "", ""},
	"Nota aggiunta su %s, pagina %d (%s).":                                    {"ok", "", ""},
	"Nota cambiata.":                                                          {"ok", "", ""},
	"Nota tolta.":                                                             {"ok", "", ""},
	"Deroga strutturale revocata.":                                            {"ok", "", ""},
	"Revisione V":                                                             {"ok", "", ""},
	"BOM congelata: V":                                                        {"ok", "", ""},
	// rifiuti fuori dal ponte e fuori dall'elenco chiuso (DUBBI del pezzo «ponte»: §6.2 del piano non li elenca)
	"Analisi non accodata: ":                                {"fuori", "", ""},
	"L'analisi semantica non è attiva per questa casella. ": {"fuori", "", ""},
	"Atto non previsto: ":                                   {"fuori", "", ""},
	"Scegli una richiesta.":                                 {"fuori", "", ""},
	"Servono la RFQ e il fornitore.":                        {"fuori", "", ""},
	"RFQ non trovata.":                                      {"fuori", "", ""},
	"Scegli una RFQ dall'elenco.":                           {"fuori", "", ""},
	"Il messaggio è già stato deciso: i suoi candidati non si ricalcolano.":                                              {"fuori", "", ""},
	"Nel frattempo il messaggio è stato agganciato a %s: non ne è stata creata una seconda. Ricarica per vedere la RFQ.": {"fuori", "", ""},
	// pezzi e non avvisi
	"Si accende in [agente] del file di configurazione, per casella, e serve la chiave nella variabile d'ambiente indicata.": {"pezzo", "", ""},
	"Diventa un documento con «Conferma», dove si sceglie se si aggiunge o che cosa sostituisce (con il motivo).":            {"pezzo", "", ""},
	"Sul NAS della RFQ va solo con la conferma.": {"pezzo", "", ""},
	"Cartella in creazione.":                     {"pezzo", "", ""}, // dentro «RFQ creata: …»
	"Cartella sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta.": {"pezzo", "", ""}, // idem
	"NAS: ": {"altro", "", ""}, // il percorso interno di un caricamento
	"SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR KEY SHARE": {"altro", "", ""},
}

// maVerbi: i verbi di fmt nell'esempio di un formato.
var maVerbi = strings.NewReplacer("%s", "7120010", "%d", "2", "%v", "x", "%q", `"x"`, "%%", "%")

func TestMotoreALeFrasiDeiGestiSonoClassificate(t *testing.T) {
	fset := token.NewFileSet()
	voci, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	funzioni := map[string]*ast.FuncDecl{} // "file.go:nome"
	gestori := map[string]string{}         // nome del metodo → il pattern della sua rotta
	reRotta := regexp.MustCompile(`^(GET|POST) (/\S+)$`)
	for _, v := range voci {
		n := v.Name()
		if v.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			funzioni[n+":"+fd.Name.Name] = fd
			// le registrazioni: mux.HandleFunc("POST /…", …s.gestore…)
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				c, ok := x.(*ast.CallExpr)
				if !ok || len(c.Args) < 2 {
					return true
				}
				sel, ok := c.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
					return true
				}
				lit, ok := c.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				p, _ := strconv.Unquote(lit.Value)
				if !reRotta.MatchString(p) {
					return true
				}
				// il gestore è il metodo più interno: s.autenticato(s.dopoIlGesto(s.gestore))
				ultimo := ""
				ast.Inspect(c.Args[1], func(y ast.Node) bool {
					if s, ok := y.(*ast.SelectorExpr); ok {
						if id, ok := s.X.(*ast.Ident); ok && id.Name == "s" {
							ultimo = s.Sel.Name
						}
					}
					return true
				})
				if ultimo != "" {
					gestori[ultimo] = p
				}
				return true
			})
		}
	}
	if len(gestori) < 50 {
		t.Fatalf("lette %d rotte: la lettura delle registrazioni non funziona", len(gestori))
	}

	// le rotte del ponte, con i segnaposto di ServeMux al posto di {id} e {u}
	rotte := ponteElenco(t, "ROTTE")
	delPonte := map[string]bool{}
	for _, r := range rotte {
		delPonte["POST "+maNormalizzaRotta(r)] = true
	}

	// chi arriva a threadFrammento: chi lo chiama, chi chiama uno di quelli, e così via (dentro il pacchetto)
	arriva := map[string]bool{"threadFrammento": true}
	chiamanti := map[string]bool{}
	for cambia := true; cambia; {
		cambia = false
		for k, fd := range funzioni {
			if chiamanti[k] || fd.Name.Name == "threadFrammento" {
				continue
			}
			ast.Inspect(fd.Body, func(x ast.Node) bool {
				c, ok := x.(*ast.CallExpr)
				if !ok {
					return true
				}
				nome := ""
				switch f := c.Fun.(type) {
				case *ast.SelectorExpr:
					nome = f.Sel.Name
				case *ast.Ident:
					nome = f.Name
				}
				if arriva[nome] && nome != fd.Name.Name && !chiamanti[k] {
					chiamanti[k] = true
					arriva[fd.Name.Name] = true
					cambia = true
				}
				return true
			})
		}
	}

	// ogni chiamante è il gestore di una rotta (del ponte o fuori) o un inoltro dichiarato
	parte := map[string]string{} // "file.go:nome" → ponte | fuori | inoltro
	for k := range chiamanti {
		nome := k[strings.Index(k, ":")+1:]
		switch p, gestore := gestori[nome]; {
		case maInoltri[k].parte != "":
			parte[k] = maInoltri[k].parte
		case gestore && delPonte[maNormalizzaPattern(p)]:
			parte[k] = "ponte"
		case gestore:
			parte[k] = "fuori"
		default:
			t.Errorf("%s arriva a threadFrammento e non è né il gestore di una rotta né un inoltro dichiarato: classificalo (maInoltri)", k)
		}
	}
	for k := range maInoltri {
		if !chiamanti[k] {
			t.Errorf("l'inoltro %s non arriva più a threadFrammento: toglilo da maInoltri", k)
		}
	}
	// ogni rotta del ponte che risponde con un avviso arriva a threadFrammento (tranne la conferma dell'albero, JSON)
	for _, r := range rotte {
		pat := "POST " + maNormalizzaRotta(r)
		trovato := false
		for k, p := range parte {
			if p == "ponte" && maNormalizzaPattern(gestori[k[strings.Index(k, ":")+1:]]) == pat {
				trovato = true
			}
		}
		if !trovato {
			t.Errorf("la rotta del ponte %s non ha un gestore che arriva a threadFrammento", r)
		}
	}
	for k, p := range maFrasiAiutanti {
		if funzioni[k] == nil {
			t.Errorf("l'aiutante %s non c'è più: toglilo da maFrasiAiutanti", k)
		}
		parte[k] = p
	}

	// le frasi
	visti := map[string][]string{}
	for k := range parte {
		for _, f := range maLetterali(funzioni[k]) {
			visti[f] = append(visti[f], k)
		}
	}
	var mancanti []string
	for f, dove := range visti {
		c, ok := maFrasi[f]
		if !ok {
			sort.Strings(dove)
			mancanti = append(mancanti, fmt.Sprintf("\t%q: {%q, \"\"}, // %s", f, "?", strings.Join(dove, ", ")))
			continue
		}
		for _, k := range dove {
			if c.classe == "fuori" && parte[k] != "fuori" {
				t.Errorf("%q è in %s (%s): la classe «fuori» vale solo per le rotte fuori dal ponte", f, k, parte[k])
			}
		}
		esempio := c.esempio
		if esempio == "" {
			esempio = maVerbi.Replace(f)
		}
		switch c.classe {
		case "no", "ok":
			if g := esitoMotoreA(esempio); g != c.classe {
				t.Errorf("%q (esempio %q): esitoMotoreA dice %q, la tabella %q", f, esempio, g, c.classe)
			}
		case "dipende":
			if c.esempio == "" || c.esempioOk == "" || esitoMotoreA(c.esempio) != "no" || esitoMotoreA(c.esempioOk) != "ok" {
				t.Errorf("%q: «dipende» vuole un esempio «no» e uno «ok» (%q, %q)", f, c.esempio, c.esempioOk)
			}
		case "fuori", "pezzo", "altro":
		default:
			t.Errorf("%q: classe sconosciuta %q", f, c.classe)
		}
	}
	sort.Strings(mancanti)
	if len(mancanti) > 0 {
		t.Errorf("%d frasi con 200 senza classe (T-A1d-39): un rifiuto nuovo con 200 deve dire se è «no» (maFrasi)\n%s",
			len(mancanti), strings.Join(mancanti, "\n"))
	}
	for f := range maFrasi {
		if visti[f] == nil {
			t.Errorf("%q non compare più nei gesti: toglila da maFrasi", f)
		}
	}
}

// maLetterali: i letterali stringa con l'iniziale maiuscola del corpo di fd, tolti quelli dentro http.Error, s.Log.*,
// fmt.Errorf, errors.New e rifiuto(…).
func maLetterali(fd *ast.FuncDecl) []string {
	var out []string
	var visita func(n ast.Node) bool
	visita = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if maEscluso(x.Fun) {
				return false
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				s, err := strconv.Unquote(x.Value)
				r, _ := utf8.DecodeRuneInString(s)
				if err == nil && unicode.IsUpper(r) {
					out = append(out, s)
				}
			}
		}
		return true
	}
	ast.Inspect(fd.Body, visita)
	return out
}

func maEscluso(f ast.Expr) bool {
	switch x := f.(type) {
	case *ast.Ident:
		return x.Name == "rifiuto"
	case *ast.SelectorExpr:
		if p, ok := x.X.(*ast.Ident); ok {
			switch p.Name + "." + x.Sel.Name {
			case "http.Error", "fmt.Errorf", "errors.New":
				return true
			}
		}
		if l, ok := x.X.(*ast.SelectorExpr); ok && l.Sel.Name == "Log" {
			return true
		}
	}
	return false
}

// maNormalizzaRotta: una rotta del ponte con i segnaposto {} di ServeMux.
func maNormalizzaRotta(r string) string {
	return strings.NewReplacer("{id}", "{}", "{u}", "{}").Replace(r)
}

var maReSegnaposto = regexp.MustCompile(`\{[^}]*\}`)

// maNormalizzaPattern: un pattern di ServeMux con i segnaposto senza nome.
func maNormalizzaPattern(p string) string {
	return maReSegnaposto.ReplaceAllString(p, "{}")
}

// TestThreadFrammentoRispondeAllaMotoreA: a flag acceso e con HX-Current-URL della pagina motore-a della stessa RFQ,
// threadFrammento risponde con il solo <div data-ma-esito>, prima di ogni lettura (Pool nil: una lettura andrebbe in
// panic); a flag spento, da un'altra pagina o da un'altra RFQ il ramo non scatta e la risposta non è il div (T-A1d-07).
func TestThreadFrammentoRispondeAllaMotoreA(t *testing.T) {
	id := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	pagina := "http://cockpit.acme.example/thread/" + id.String() + "/motore-a?vista=documenti"
	chiama := func(s *Server, corrente, avviso string) (corpo string) {
		req := httptest.NewRequest("POST", "/proposta/00000000-0000-4000-8000-000000000030/scarta", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Current-URL", corrente)
		rec := httptest.NewRecorder()
		defer func() {
			_ = recover() // fuori dal ramo threadFrammento legge il database, che qui non c'è
			corpo = rec.Body.String()
		}()
		s.threadFrammento(rec, req, id, avviso)
		return rec.Body.String()
	}
	acceso := &Server{MotoreA: AnteprimaMotoreA{Accesa: true}}
	if g := chiama(acceso, pagina, "Proposta scartata."); g != `<div data-ma-esito="ok">Proposta scartata.</div>` {
		t.Errorf("acceso, dalla pagina: %q", g)
	}
	if g := chiama(acceso, pagina, "Nessun file scelto: niente da assegnare."); g != `<div data-ma-esito="no">Nessun file scelto: niente da assegnare.</div>` {
		t.Errorf("acceso, un rifiuto dell'elenco chiuso: %q", g)
	}
	for nome, c := range map[string]struct {
		s        *Server
		corrente string
	}{
		"spento":          {&Server{}, pagina},
		"altra pagina":    {acceso, "http://cockpit.acme.example/thread/" + id.String() + "/distinta"},
		"altra RFQ":       {acceso, "http://cockpit.acme.example/thread/00000000-0000-4000-8000-000000000002/motore-a"},
		"senza la pagina": {acceso, ""},
	} {
		if g := chiama(c.s, c.corrente, "Proposta scartata."); strings.Contains(g, "data-ma-esito") {
			t.Errorf("%s: il ramo motore-a è scattato: %q", nome, g)
		}
	}
}

// ---------------------------------------------------------------- la porta e le rotte (§5.1.3)

// maServerMotoreA: il server delle prove della porta, con i template (serverTest), il log scartato e il flag dato. Il
// Pool è nil: una lettura del database (la sessione, la fase) va in panic, e maServi lo dice.
func maServerMotoreA(t *testing.T, a AnteprimaMotoreA) *Server {
	t.Helper()
	s := serverTest(t)
	s.Log = slog.New(slog.DiscardHandler)
	s.MotoreA = a
	return s
}

// maServi: una richiesta servita da h, con il panic di una lettura del database recuperato e restituito.
func maServi(h http.Handler, req *http.Request) (rec *httptest.ResponseRecorder, panico any) {
	rec = httptest.NewRecorder()
	func() {
		defer func() { panico = recover() }()
		h.ServeHTTP(rec, req)
	}()
	return rec, panico
}

// maContaCalcoli: un Calcola che conta le chiamate e non calcola niente (le prove della porta: nessun calcolo).
func maContaCalcoli(n *int) func(context.Context, uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
	return func(context.Context, uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
		*n++
		return fotorfq.Fotografia{}, valutazione.Esito{}, confronto.Esito{}, errors.New("la prova non calcola")
	}
}

// maRotteMotoreA: le due GET di A1d-P2 sotto /thread/{id} (la pagina arriva in A1d-P3).
var maRotteMotoreA = []string{"/motore-a/dati", "/motore-a/anteprima/step-strutturale?componente=" + maUUID(10).String()}

// A flag spento ogni rotta di motore-a risponde 404 a tutti, anonimi compresi, PRIMA della sessione (R36): con il cookie
// di una sessione e Pool nil, una porta che leggesse la sessione andrebbe in panic. Con e senza HX-Request, GET e HEAD,
// anche con un id che non è un UUID; nessun calcolo, nessun rimando al login (M-A1d-03). Anche con gli altri metodi
// (POST, PUT, DELETE, PATCH, OPTIONS): 404 e nessun Allow, mai il 405 del mux, che direbbe che la rotta c'è (contratto
// §2.1 «404 a tutti»; S-01 della revisione di A1d-P2).
func TestMotoreASpentoRisponde404PrimaDellaSessione(t *testing.T) {
	calcoli := 0
	s := maServerMotoreA(t, AnteprimaMotoreA{Calcola: maContaCalcoli(&calcoli)}) // Accesa falso: il valore zero
	mux := http.NewServeMux()
	s.Registra(mux)
	for _, thread := range []string{maUUID(1).String(), "xyz"} {
		for _, rotta := range maRotteMotoreA {
			for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
				http.MethodPatch, http.MethodOptions} {
				for _, cookie := range []bool{false, true} {
					for _, hx := range []bool{false, true} {
						nome := fmt.Sprintf("%s %s%s cookie=%v htmx=%v", metodo, thread, rotta, cookie, hx)
						req := httptest.NewRequest(metodo, "/thread/"+thread+rotta, nil)
						if cookie {
							req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
						}
						if hx {
							req.Header.Set("HX-Request", "true")
						}
						rec, panico := maServi(mux, req)
						if panico != nil {
							t.Errorf("%s: la porta ha toccato la sessione (Pool nil): %v", nome, panico)
							continue
						}
						if rec.Code != http.StatusNotFound {
							t.Errorf("%s: stato %d, atteso 404", nome, rec.Code)
						}
						if a := rec.Header().Get("Allow"); a != "" {
							t.Errorf("%s: Allow %q: la rotta si distingue da una che non c'è", nome, a)
						}
						if rec.Header().Get("HX-Redirect") != "" || rec.Header().Get("Location") != "" {
							t.Errorf("%s: rimanda al login (%q, %q): la rotta si distingue da una che non c'è", nome,
								rec.Header().Get("HX-Redirect"), rec.Header().Get("Location"))
						}
					}
				}
			}
		}
	}
	if calcoli != 0 {
		t.Errorf("%d calcoli a flag spento", calcoli)
	}
}

// A flag acceso la porta passa la mano a soloAdmin: senza sessione si va al login (302, o 401 con HX-Redirect per
// htmx), mai 404 e mai un calcolo; il cookie di una sessione si legge solo qui (non provato: legge il DB, A1d-L4-02).
func TestMotoreAAccesoChiedeLaSessione(t *testing.T) {
	calcoli := 0
	s := maServerMotoreA(t, AnteprimaMotoreA{Accesa: true, Calcola: maContaCalcoli(&calcoli)})
	mux := http.NewServeMux()
	s.Registra(mux)
	for _, rotta := range maRotteMotoreA {
		for _, hx := range []bool{false, true} {
			req := httptest.NewRequest(http.MethodGet, "/thread/"+maUUID(1).String()+rotta, nil)
			if hx {
				req.Header.Set("HX-Request", "true")
			}
			rec, panico := maServi(mux, req)
			if panico != nil {
				t.Errorf("%s htmx=%v: panic senza cookie: %v", rotta, hx, panico)
				continue
			}
			switch {
			case hx && (rec.Code != http.StatusUnauthorized || rec.Header().Get("HX-Redirect") != "/login"):
				t.Errorf("%s con htmx: %d %q, attesi 401 e HX-Redirect /login", rotta, rec.Code, rec.Header().Get("HX-Redirect"))
			case !hx && (rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login"):
				t.Errorf("%s: %d %q, attesi 302 verso /login", rotta, rec.Code, rec.Header().Get("Location"))
			}
		}
	}
	if calcoli != 0 {
		t.Errorf("%d calcoli senza sessione", calcoli)
	}
}

// A flag acceso gli altri metodi hanno il 405 che il mux dava da sé, con Allow: GET, HEAD, senza leggere la sessione
// (Pool nil: nessun panic), senza calcolo e senza rimando al login (S-01 della revisione di A1d-P2).
func TestMotoreAAccesoGliAltriMetodiSono405(t *testing.T) {
	calcoli := 0
	s := maServerMotoreA(t, AnteprimaMotoreA{Accesa: true, Calcola: maContaCalcoli(&calcoli)})
	mux := http.NewServeMux()
	s.Registra(mux)
	for _, rotta := range maRotteMotoreA {
		for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			req := httptest.NewRequest(metodo, "/thread/"+maUUID(1).String()+rotta, nil)
			req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
			rec, panico := maServi(mux, req)
			if panico != nil {
				t.Errorf("%s %s: ha letto la sessione (Pool nil): %v", metodo, rotta, panico)
				continue
			}
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" || rec.Header().Get("Location") != "" {
				t.Errorf("%s %s: %d, Allow %q, Location %q; attesi 405 e «GET, HEAD»", metodo, rotta, rec.Code,
					rec.Header().Get("Allow"), rec.Header().Get("Location"))
			}
		}
	}
	if calcoli != 0 {
		t.Errorf("%d calcoli con un metodo non ammesso", calcoli)
	}
}

// Il ruolo: dietro la porta accesa passa solo l'admin, gli altri ruoli hanno il 403 di nega (con htmx il frammento
// «vietato», che non legge il DB) e nessun calcolo; l'admin arriva al gestore (HEAD di /dati: 200 senza calcolo;
// l'anteprima con un componente non valido: 200 JSON senza DB). La catena è quella di registraMotoreA meno
// autenticato, che legge la sessione dal DB (soloAdmin = autenticato + soloRuolo admin, ruoli.go): che le rotte siano
// montate proprio con conMotoreA(soloAdmin(…)) lo controlla TestMotoreALeRotteSiMontano; la catena intera con le
// sessioni vere è A1d-L4-01/-02 (NON ESEGUIBILE QUI).
func TestMotoreASoloLAdmin(t *testing.T) {
	calcoli := 0
	s := maServerMotoreA(t, AnteprimaMotoreA{Accesa: true, Calcola: maContaCalcoli(&calcoli)})
	id := maUUID(1).String()
	for _, c := range []struct {
		nome, metodo, percorso string
		h                      http.HandlerFunc
		tipo                   string
	}{
		{"dati", http.MethodHead, "/thread/" + id + "/motore-a/dati", s.motoreADati, "application/json; charset=utf-8"},
		{"anteprima", http.MethodGet, "/thread/" + id + "/motore-a/anteprima/step-strutturale?componente=xyz", s.motoreAAnteprimaStep,
			"application/json; charset=utf-8"},
	} {
		porta := s.conMotoreA(s.soloRuolo(db.RuoloUtenteAdmin, c.h))
		for _, ruolo := range db.AllRuoloUtenteValues() {
			req := httptest.NewRequest(c.metodo, c.percorso, nil)
			req.SetPathValue("id", id)
			req.Header.Set("HX-Request", "true")
			req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: maUUID(9), Sigla: "acme", Ruolo: ruolo}))
			rec, panico := maServi(porta, req)
			if panico != nil {
				t.Errorf("%s, %s: panic (il gestore ha letto il DB?): %v", c.nome, ruolo, panico)
				continue
			}
			if ruolo == db.RuoloUtenteAdmin {
				if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != c.tipo {
					t.Errorf("%s, admin: %d %q, attesi 200 e %q", c.nome, rec.Code, rec.Header().Get("Content-Type"), c.tipo)
				}
				continue
			}
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Header().Get("Content-Type"), "json") {
				t.Errorf("%s, %s: %d %q, atteso il 403 HTML di nega", c.nome, ruolo, rec.Code, rec.Header().Get("Content-Type"))
			}
		}
	}
	if calcoli != 0 {
		t.Errorf("%d calcoli: HEAD e i 403 non calcolano", calcoli)
	}
}

// Registra intero non va in panic (nessun pattern in conflitto) e le due GET di P2 arrivano ai loro pattern, anche con
// HEAD; gli altri metodi arrivano alla riga gemella senza metodo (S-01). Nel sorgente ogni GET di registraMotoreA è
// montata come conMotoreA(soloAdmin(gestore)): prima il flag, poi la sessione e il ruolo (piano §5.1.3, M-A1d-03); ogni
// riga gemella come conMotoreA(metodoNonAmmessoMotoreA), una per ogni GET e con lo stesso indirizzo.
func TestMotoreALeRotteSiMontano(t *testing.T) {
	s := maServerMotoreA(t, AnteprimaMotoreA{})
	mux := http.NewServeMux()
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Fatalf("Registra va in panic (un pattern in conflitto?): %v", v)
			}
		}()
		s.Registra(mux)
	}()
	id := maUUID(1).String()
	for _, c := range []struct{ percorso, pattern string }{
		{"/thread/" + id + "/motore-a/dati", "GET /thread/{id}/motore-a/dati"},
		{"/thread/" + id + "/motore-a/anteprima/step-strutturale?componente=" + maUUID(10).String(), "GET /thread/{id}/motore-a/anteprima/step-strutturale"},
	} {
		for _, metodo := range []string{http.MethodGet, http.MethodHead} {
			if _, pat := mux.Handler(httptest.NewRequest(metodo, c.percorso, nil)); pat != c.pattern {
				t.Errorf("%s %s: pattern %q, atteso %q", metodo, c.percorso, pat, c.pattern)
			}
		}
		gemello := strings.TrimPrefix(c.pattern, "GET ")
		for _, metodo := range []string{http.MethodPost, http.MethodDelete} {
			if _, pat := mux.Handler(httptest.NewRequest(metodo, c.percorso, nil)); pat != gemello {
				t.Errorf("%s %s: pattern %q, atteso %q", metodo, c.percorso, pat, gemello)
			}
		}
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "motore_a.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	reGestore := regexp.MustCompile(`^s\.conMotoreA\(s\.soloAdmin\(s\.[A-Za-z]+\)\)$`)
	montate := 0
	var get, gemelle []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "registraMotoreA" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := c.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "HandleFunc" || sel.Sel.Name == "Handle") {
				montate++
				if len(c.Args) != 2 {
					t.Errorf("registraMotoreA: %s con %d argomenti", types.ExprString(c), len(c.Args))
					return true
				}
				pattern, _ := strconv.Unquote(types.ExprString(c.Args[0]))
				g := types.ExprString(c.Args[1])
				switch {
				case strings.HasPrefix(pattern, "GET "):
					get = append(get, strings.TrimPrefix(pattern, "GET "))
					if !reGestore.MatchString(g) {
						t.Errorf("registraMotoreA: %s montata con %s, attesa s.conMotoreA(s.soloAdmin(s.<gestore>))", pattern, g)
					}
				case strings.HasPrefix(pattern, "/"):
					gemelle = append(gemelle, pattern)
					if g != "s.conMotoreA(metodoNonAmmessoMotoreA)" {
						t.Errorf("registraMotoreA: la riga gemella %s montata con %s, attesa s.conMotoreA(metodoNonAmmessoMotoreA)", pattern, g)
					}
				default:
					t.Errorf("registraMotoreA: %s non è né una GET né una riga gemella senza metodo", pattern)
				}
			}
			return true
		})
	}
	if montate != 4 || len(get) != 2 {
		t.Errorf("registraMotoreA monta %d schemi (%d GET), attesi 4 in A1d-P2: dati e anteprima dello STEP, con le loro "+
			"righe gemelle", montate, len(get))
	}
	sort.Strings(get)
	sort.Strings(gemelle)
	if !reflect.DeepEqual(get, gemelle) {
		t.Errorf("GET %v e righe gemelle %v: ogni GET ha la sua riga gemella con lo stesso indirizzo (S-01)", get, gemelle)
	}
}

// ---------------------------------------------------------------- il calcolo (§5.1.5)

// calcolaConLimite con un Calcola finto (T-A1d-15): l'errore torna com'è; un panic diventa errPanicMotoreA e il processo
// resta vivo (M-A1d-10); un calcolo che non guarda il contesto si ferma al limite con errTempoMotoreA, entro la guardia di
// 2 s (M-A1d-11); un calcolo che risponde con l'errore del contesto scaduto è anche lui tempo scaduto; senza limite
// (Limite 0) il calcolo finisce; la richiesta chiusa da chi l'ha fatta non è tempo scaduto; il calcolo riuscito dà i tre
// valori, con il limite nel contesto.
func TestMotoreACalcoloConLimite(t *testing.T) {
	s := &Server{Log: slog.New(slog.DiscardHandler)}
	id := maUUID(1)
	f0, e0, c0 := scenaIndiceACME()
	errACME := errors.New("calcolo ACME rotto")
	libera := make(chan struct{})
	defer close(libera)
	// guardia: calcolaConLimite deve tornare entro 2 s, anche quando il calcolo non torna mai
	guardia := func(t *testing.T, ctx context.Context, a AnteprimaMotoreA) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
		t.Helper()
		type uscita struct {
			f   fotorfq.Fotografia
			e   valutazione.Esito
			c   confronto.Esito
			err error
		}
		ris := make(chan uscita, 1)
		go func() {
			f, e, c, err := s.calcolaConLimite(ctx, a, id)
			ris <- uscita{f, e, c, err}
		}()
		select {
		case u := <-ris:
			return u.f, u.e, u.c, u.err
		case <-time.After(2 * time.Second):
			t.Fatal("calcolaConLimite non è tornato entro 2 s: il limite non vale")
		}
		return fotorfq.Fotografia{}, valutazione.Esito{}, confronto.Esito{}, nil
	}
	calcolo := func(fn func(ctx context.Context) error) func(context.Context, uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
		return func(ctx context.Context, th uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
			if th != id {
				return fotorfq.Fotografia{}, valutazione.Esito{}, confronto.Esito{}, fmt.Errorf("thread %s, atteso %s", th, id)
			}
			if err := fn(ctx); err != nil {
				return fotorfq.Fotografia{}, valutazione.Esito{}, confronto.Esito{}, err
			}
			return f0, e0, c0, nil
		}
	}

	t.Run("riuscito", func(t *testing.T) {
		var scadenza time.Time
		a := AnteprimaMotoreA{Limite: 20 * time.Second, Calcola: calcolo(func(ctx context.Context) error {
			scadenza, _ = ctx.Deadline()
			return nil
		})}
		f, e, c, err := guardia(t, context.Background(), a)
		if err != nil || !f.PresaIl.Equal(f0.PresaIl) || e.VersioneValutazione != e0.VersioneValutazione || len(c.File) != len(c0.File) {
			t.Errorf("riuscito: err %v, presa %v, versione %q, %d file", err, f.PresaIl, e.VersioneValutazione, len(c.File))
		}
		if scadenza.IsZero() || time.Until(scadenza) > 20*time.Second {
			t.Errorf("il calcolo non ha il limite nel contesto (scadenza %v)", scadenza)
		}
	})
	t.Run("errore", func(t *testing.T) {
		_, _, _, err := guardia(t, context.Background(), AnteprimaMotoreA{Limite: 20 * time.Second, Calcola: calcolo(func(context.Context) error { return errACME })})
		if !errors.Is(err, errACME) || errors.Is(err, errTempoMotoreA) || errors.Is(err, errPanicMotoreA) {
			t.Errorf("errore: %v, atteso %v", err, errACME)
		}
	})
	t.Run("panic", func(t *testing.T) {
		_, _, _, err := guardia(t, context.Background(), AnteprimaMotoreA{Limite: 20 * time.Second,
			Calcola: calcolo(func(context.Context) error { panic("indice ACME fuori dall'elenco") })})
		if !errors.Is(err, errPanicMotoreA) || !strings.Contains(err.Error(), "indice ACME fuori dall'elenco") {
			t.Errorf("panic: %v, atteso errPanicMotoreA con il valore", err)
		}
	})
	t.Run("tempo, un calcolo che non guarda il contesto", func(t *testing.T) {
		inizio := time.Now()
		_, _, _, err := guardia(t, context.Background(), AnteprimaMotoreA{Limite: 20 * time.Millisecond,
			Calcola: calcolo(func(context.Context) error { <-libera; return nil })})
		if !errors.Is(err, errTempoMotoreA) || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("tempo: %v, atteso errTempoMotoreA", err)
		}
		if d := time.Since(inizio); d < 20*time.Millisecond {
			t.Errorf("tornato dopo %v, prima del limite", d)
		}
	})
	t.Run("tempo, un calcolo che risponde con il contesto scaduto", func(t *testing.T) {
		for i := 0; i < 20; i++ { // la select ha due casi pronti: il risultato non deve dipendere da quale prende
			_, _, _, err := guardia(t, context.Background(), AnteprimaMotoreA{Limite: time.Millisecond,
				Calcola: calcolo(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })})
			if !errors.Is(err, errTempoMotoreA) {
				t.Fatalf("giro %d: %v, atteso errTempoMotoreA", i, err)
			}
		}
	})
	t.Run("tempo, il contesto è già scaduto", func(t *testing.T) {
		// il risultato e la fine del contesto sono pronti insieme: la select ne prende uno a caso, l'esito no
		scaduto, annulla := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer annulla()
		for i := 0; i < 50; i++ {
			_, _, _, err := guardia(t, scaduto, AnteprimaMotoreA{Limite: 20 * time.Second,
				Calcola: calcolo(func(ctx context.Context) error { return ctx.Err() })})
			if !errors.Is(err, errTempoMotoreA) {
				t.Fatalf("giro %d: %v, atteso errTempoMotoreA", i, err)
			}
		}
	})
	t.Run("senza limite", func(t *testing.T) {
		_, _, _, err := guardia(t, context.Background(), AnteprimaMotoreA{Calcola: calcolo(func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); ok {
				return errors.New("un limite senza Limite")
			}
			time.Sleep(30 * time.Millisecond)
			return nil
		})})
		if err != nil {
			t.Errorf("senza limite: %v", err)
		}
	})
	t.Run("richiesta annullata", func(t *testing.T) {
		ctx, annulla := context.WithCancel(context.Background())
		go func() { time.Sleep(10 * time.Millisecond); annulla() }()
		_, _, _, err := guardia(t, ctx, AnteprimaMotoreA{Limite: 20 * time.Second, Calcola: calcolo(func(context.Context) error { <-libera; return nil })})
		if err == nil || errors.Is(err, errTempoMotoreA) || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "richiesta annullata") {
			t.Errorf("annullata: %v", err)
		}
	})
}

// ---------------------------------------------------------------- la pagina (§5.1.4)

// La pagina legge solo l'esito «valutazione-3» (X-07, T-A1d-17): la costante coincide con quella di valutazione, e un
// esito di un'altra versione dà Errore con le due versioni, senza Esito, Indice, Confronto né testa (M-A1d-12).
func TestMotoreALaVersioneDellEsito(t *testing.T) {
	if valutazione.VersioneValutazione != versioneEsitoMotoreA {
		t.Errorf("valutazione.VersioneValutazione = %q, la pagina legge %q: si rilegge la pagina contro la versione nuova",
			valutazione.VersioneValutazione, versioneEsitoMotoreA)
	}
	f, e, c := scenaIndiceACME()
	e.VersioneValutazione = "valutazione-1"
	p := &paginaMotoreA{ThreadID: maUUID(1), Vista: vistaDistinta}
	componiPaginaMotoreA(p, f, e, c, maP1)
	if !strings.Contains(p.Errore, "«valutazione-1»") || !strings.Contains(p.Errore, "«"+versioneEsitoMotoreA+"»") {
		t.Errorf("Errore %q: atteso con le due versioni", p.Errore)
	}
	if p.Esito != nil || p.Indice != nil || p.Confronto != nil || p.Prodotto != "" || !p.Calcolato.IsZero() || p.Testa.Cliente != "" {
		t.Errorf("una pagina a metà: %+v", p)
	}
}

// componiPaginaMotoreA sulla scena ACME (T-A1d-18): la testa dalla fotografia (cliente, oggetto, riferimento, stato) con
// la fase del gestore; l'esito del thread, il confronto, l'indice; il prodotto chiesto, il primo se sconosciuto, "" senza
// prodotti; Calcolato = PresaIl. Un esito o una fotografia senza il thread chiesto danno l'errore generico. vistaMotoreA.
func TestMotoreAComponiLaPagina(t *testing.T) {
	fase := &faseRegistrata{Nome: string(db.FaseFATTIBILITA), Etichetta: "Fattibilità ACME di prova", Semaforo: "verde"}
	nuova := func() *paginaMotoreA {
		return &paginaMotoreA{ThreadID: maUUID(1), Vista: vistaDocumenti, Testa: testaMotoreA{Fase: fase}, Scrive: true}
	}
	f, e, c := scenaIndiceACME()
	p := nuova()
	componiPaginaMotoreA(p, f, e, c, maP2)
	atteso := testaMotoreA{Cliente: "ACME S.p.A.", Oggetto: "RFQ 7120001", RiferimentoCliente: "ACME-030P7120001",
		StatoThread: string(db.StatoThreadAPERTA), Fase: fase}
	if p.Errore != "" || !reflect.DeepEqual(p.Testa, atteso) {
		t.Errorf("testa %+v (errore %q), attesa %+v", p.Testa, p.Errore, atteso)
	}
	if p.Esito == nil || p.Esito.ThreadID != maUUID(1) || p.Confronto == nil || len(p.Confronto.File) != 2 || p.Indice == nil {
		t.Fatalf("esito %v, confronto %v, indice %v", p.Esito != nil, p.Confronto, p.Indice != nil)
	}
	if p.Indice.Prodotto(maP1) == nil || p.Indice.File(maUUID(34)) == nil {
		t.Error("l'indice non è quello dell'esito e della fotografia della pagina")
	}
	if p.Prodotto != maP2 || !p.Calcolato.Equal(f.PresaIl) || p.Vista != vistaDocumenti || !p.Scrive {
		t.Errorf("prodotto %q, calcolato %v, vista %q, scrive %v", p.Prodotto, p.Calcolato, p.Vista, p.Scrive)
	}
	for chiesto, scelto := range map[string]string{"": maP1, "componente:boh": maP1, maP1: maP1, maP2: maP2} {
		p := nuova()
		componiPaginaMotoreA(p, f, e, c, chiesto)
		if p.Prodotto != scelto {
			t.Errorf("prodotto chiesto %q: %q, atteso %q", chiesto, p.Prodotto, scelto)
		}
	}
	senza := e
	senza.Thread = []valutazione.EsitoThread{e.Thread[0]}
	senza.Thread[0].ProdottiValutati = nil
	p = nuova()
	componiPaginaMotoreA(p, f, senza, c, maP1)
	if p.Prodotto != "" || p.Errore != "" {
		t.Errorf("senza prodotti: prodotto %q, errore %q", p.Prodotto, p.Errore)
	}
	// oggetto e riferimento nil = ""
	f2, e2, c2 := scenaIndiceACME()
	f2.Thread[0].Oggetto, f2.Thread[0].Riferimento = nil, nil
	p = nuova()
	componiPaginaMotoreA(p, f2, e2, c2, "")
	if p.Testa.Oggetto != "" || p.Testa.RiferimentoCliente != "" || p.Testa.Cliente != "ACME S.p.A." {
		t.Errorf("testa senza oggetto e riferimento: %+v", p.Testa)
	}
	// il thread chiesto manca dall'esito, o dalla fotografia: l'errore generico, niente a metà
	for nome, x := range map[string]func() (fotorfq.Fotografia, valutazione.Esito){
		"esito": func() (fotorfq.Fotografia, valutazione.Esito) {
			f, e, _ := scenaIndiceACME()
			e.Thread = nil
			return f, e
		},
		"fotografia": func() (fotorfq.Fotografia, valutazione.Esito) {
			f, e, _ := scenaIndiceACME()
			f.Thread = nil
			return f, e
		},
	} {
		f, e := x()
		p := nuova()
		componiPaginaMotoreA(p, f, e, c, maP1)
		if p.Errore != erroreCalcoloMotoreA || p.Esito != nil || p.Indice != nil || p.Confronto != nil || p.Testa.Cliente != "" {
			t.Errorf("%s senza il thread: errore %q, esito %v, indice %v", nome, p.Errore, p.Esito != nil, p.Indice != nil)
		}
	}
	for chiesta, vista := range map[string]string{"": vistaDistinta, "boh": vistaDistinta, "DOCUMENTI": vistaDistinta,
		vistaDistinta: vistaDistinta, vistaDocumenti: vistaDocumenti, vistaRFQ: vistaRFQ} {
		if g := vistaMotoreA(chiesta); g != vista {
			t.Errorf("vistaMotoreA(%q) = %q, atteso %q", chiesta, g, vista)
		}
	}
}

// maChiaviJSON: le chiavi di un oggetto JSON, in ordine.
func maChiaviJSON(t *testing.T, b []byte) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Il JSON di /dati (contratto §2.4; T-A1d-18): application/json, no-store; le chiavi di primo livello, della testa e della
// fase sono esattamente quelle del contratto, senza scrive né indice; con Errore (RAC-05) esito e confronto sono null.
func TestMotoreADatiJSON(t *testing.T) {
	f, e, c := scenaIndiceACME()
	fase := &faseRegistrata{Nome: string(db.FaseFATTIBILITA), Etichetta: "Fattibilità ACME di prova", Inizio: f.PresaIl,
		GgInFase: 2, Semaforo: "verde"}
	p := &paginaMotoreA{ThreadID: maUUID(1), Vista: vistaDistinta, Testa: testaMotoreA{Fase: fase}, Scrive: true}
	componiPaginaMotoreA(p, f, e, c, "")
	errata := &paginaMotoreA{ThreadID: maUUID(1), Vista: vistaRFQ, Testa: testaMotoreA{Fase: fase}, Errore: erroreTempoMotoreA}
	for nome, p := range map[string]*paginaMotoreA{"calcolata": p, "con errore": errata} {
		rec := httptest.NewRecorder()
		scriviDatiMotoreA(rec, p)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" ||
			rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: %d %q %q nosniff %q", nome, rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"),
				rec.Header().Get("X-Content-Type-Options"))
		}
		corpo := rec.Body.Bytes()
		if g := strings.Join(maChiaviJSON(t, corpo), ","); g != "calcolato,confronto,errore,esito,prodotto,testa,thread_id,vista" {
			t.Errorf("%s: chiavi %s", nome, g)
		}
		var d struct {
			ThreadID  string          `json:"thread_id"`
			Testa     json.RawMessage `json:"testa"`
			Esito     json.RawMessage `json:"esito"`
			Confronto json.RawMessage `json:"confronto"`
			Errore    string          `json:"errore"`
		}
		if err := json.Unmarshal(corpo, &d); err != nil {
			t.Fatal(err)
		}
		if g := strings.Join(maChiaviJSON(t, d.Testa), ","); g != "cliente,fase,oggetto,riferimento_cliente,stato_thread" {
			t.Errorf("%s: chiavi della testa %s", nome, g)
		}
		var testa struct {
			Fase json.RawMessage `json:"fase"`
		}
		_ = json.Unmarshal(d.Testa, &testa)
		if g := strings.Join(maChiaviJSON(t, testa.Fase), ","); g != "etichetta,gg_in_fase,inizio,nome,semaforo,terminale" {
			t.Errorf("%s: chiavi della fase %s", nome, g)
		}
		if d.ThreadID != maUUID(1).String() || d.Errore != p.Errore {
			t.Errorf("%s: thread %q, errore %q", nome, d.ThreadID, d.Errore)
		}
		nulli := string(d.Esito) == "null" && string(d.Confronto) == "null"
		if nulli != (p.Errore != "") {
			t.Errorf("%s: esito %.40s, confronto %.40s", nome, d.Esito, d.Confronto)
		}
	}
}

// HEAD di /dati (T-A1d-13): 200, le intestazioni del JSON e no-store, nessun corpo, nessun calcolo e nessuna lettura
// del DB (Pool nil); un id che non è un UUID è 400, anche lui no-store.
func TestMotoreAHeadSenzaCalcolo(t *testing.T) {
	calcoli := 0
	s := &Server{Log: slog.New(slog.DiscardHandler), MotoreA: AnteprimaMotoreA{Accesa: true, Limite: time.Second, Calcola: maContaCalcoli(&calcoli)}}
	admin := &db.Utente{UtenteID: maUUID(9), Sigla: "acme", Ruolo: db.RuoloUtenteAdmin}
	for _, c := range []struct {
		metodo, id string
		stato      int
		tipo       string
	}{
		{http.MethodHead, maUUID(1).String(), http.StatusOK, "application/json; charset=utf-8"},
		{http.MethodHead, "xyz", http.StatusBadRequest, "text/plain; charset=utf-8"},
		{http.MethodGet, "xyz", http.StatusBadRequest, "text/plain; charset=utf-8"},
	} {
		req := httptest.NewRequest(c.metodo, "/thread/"+c.id+"/motore-a/dati", nil)
		req.SetPathValue("id", c.id)
		req = req.WithContext(context.WithValue(req.Context(), ctxUtente, admin))
		rec, panico := maServi(http.HandlerFunc(s.motoreADati), req)
		if panico != nil {
			t.Errorf("%s %s: il gestore ha letto il database: %v", c.metodo, c.id, panico)
			continue
		}
		if rec.Code != c.stato || rec.Header().Get("Content-Type") != c.tipo || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s: %d %q %q", c.metodo, c.id, rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Cache-Control"))
		}
		if c.stato == http.StatusOK && rec.Body.Len() != 0 {
			t.Errorf("HEAD con un corpo: %q", rec.Body.String())
		}
		if g := rec.Header().Get("X-Content-Type-Options"); g != "nosniff" {
			t.Errorf("%s %s: X-Content-Type-Options %q, atteso nosniff (I-02)", c.metodo, c.id, g)
		}
	}
	if calcoli != 0 {
		t.Errorf("%d calcoli per HEAD", calcoli)
	}
}

// I rami di paginaMotoreADa, dal gestore /dati, senza database (R-1, R-02, R-03, R-04 della revisione di A1d-P2; P-12,
// RAC-05, T-A1d-15): la lettura della fase (leggiFase) e il calcolo (Calcola) sono finti. La RFQ che non c'è è 404 «RFQ
// non trovata», senza calcolo; un errore della lettura è il testo generico; le regole non caricate danno a.Errore, o
// ErroreRegoleMotoreA, senza calcolo; il tempo scaduto dà il testo del tempo (una riga WARN); un errore o un panic del
// calcolo il testo generico (ERROR; per il panic con la pila, R-03); un panic fuori dal calcolo (qui nella lettura della
// fase) il testo generico, non una connessione chiusa (R-02); una richiesta chiusa da chi l'aveva fatta va nel log come
// INFO, mai come ERROR (R-04); il calcolo riuscito dà l'esito con la fase letta. Nessun testo interno arriva al JSON.
func TestMotoreAIRamiDellaPagina(t *testing.T) {
	admin := &db.Utente{UtenteID: maUUID(9), Sigla: "acme", Ruolo: db.RuoloUtenteAdmin}
	fase := &faseRegistrata{Nome: string(db.FaseFATTIBILITA), Etichetta: "Fattibilità ACME di prova", Semaforo: "verde"}
	regole := &motorea.InsiemeRegole{} // non nil: «regole caricate»; il calcolo finto non le legge
	errACME := errors.New("lettura ACME rotta")
	libera := make(chan struct{})
	defer close(libera)
	leggi := func(f *faseRegistrata, err error) func(context.Context, uuid.UUID) (*faseRegistrata, error) {
		return func(context.Context, uuid.UUID) (*faseRegistrata, error) { return f, err }
	}
	var calcoli atomic.Int32 // il calcolo gira nella goroutine di calcolaConLimite
	calcola := func(fn func() error) func(context.Context, uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
		return func(context.Context, uuid.UUID) (fotorfq.Fotografia, valutazione.Esito, confronto.Esito, error) {
			calcoli.Add(1)
			if err := fn(); err != nil {
				return fotorfq.Fotografia{}, valutazione.Esito{}, confronto.Esito{}, err
			}
			f, e, c := scenaIndiceACME()
			return f, e, c, nil
		}
	}
	riuscito := calcola(func() error { return nil })
	for _, c := range []struct {
		nome      string
		a         AnteprimaMotoreA
		annullata bool
		stato     int
		errore    string // il campo errore del JSON
		esito     bool   // esito e confronto non nulli
		fase      bool   // la fase letta nella testa
		calcoli   int32  // -1 = non si conta (la richiesta chiusa: la select può tornare prima che il calcolo parta)
		log       string // un pezzo che il log deve avere
		mai       string // un pezzo che il log non deve avere
	}{
		{nome: "la RFQ non c'è", a: AnteprimaMotoreA{Regole: regole, leggiFase: leggi(nil, errRFQAssente), Calcola: riuscito},
			stato: http.StatusNotFound, mai: "level="},
		{nome: "la lettura non riesce", a: AnteprimaMotoreA{Regole: regole, leggiFase: leggi(nil, fmt.Errorf("motore A: la RFQ non si legge: %w", errACME)), Calcola: riuscito},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, log: "level=ERROR"},
		{nome: "le regole non caricate, con il loro testo", a: AnteprimaMotoreA{Errore: "Regole ACME non caricate.", leggiFase: leggi(fase, nil), Calcola: riuscito},
			stato: http.StatusOK, errore: "Regole ACME non caricate.", fase: true, mai: "level="},
		{nome: "le regole non caricate, senza testo", a: AnteprimaMotoreA{leggiFase: leggi(nil, nil), Calcola: riuscito},
			stato: http.StatusOK, errore: ErroreRegoleMotoreA, mai: "level="},
		{nome: "il tempo scaduto", a: AnteprimaMotoreA{Regole: regole, Limite: 20 * time.Millisecond, leggiFase: leggi(fase, nil),
			Calcola: calcola(func() error { <-libera; return nil })},
			stato: http.StatusOK, errore: erroreTempoMotoreA, fase: true, calcoli: 1, log: "level=WARN", mai: "level=ERROR"},
		{nome: "il calcolo non riesce", a: AnteprimaMotoreA{Regole: regole, Limite: 20 * time.Second, leggiFase: leggi(fase, nil),
			Calcola: calcola(func() error { return errACME })},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, fase: true, calcoli: 1, log: "level=ERROR"},
		{nome: "il calcolo va in panic", a: AnteprimaMotoreA{Regole: regole, Limite: 20 * time.Second, leggiFase: leggi(fase, nil),
			Calcola: calcola(func() error { panic("indice ACME fuori dall'elenco") })},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, fase: true, calcoli: 1, log: "goroutine "},
		{nome: "la lettura della fase va in panic", a: AnteprimaMotoreA{Regole: regole, Calcola: riuscito,
			leggiFase: func(context.Context, uuid.UUID) (*faseRegistrata, error) { panic("pool ACME assente") }},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, log: "goroutine "},
		{nome: "la lettura chiusa da chi l'aveva fatta", annullata: true, a: AnteprimaMotoreA{Regole: regole, Calcola: riuscito,
			leggiFase: leggi(nil, fmt.Errorf("motore A: la RFQ non si legge: %w", context.Canceled))},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, log: "level=INFO", mai: "level=ERROR"},
		{nome: "riuscito", a: AnteprimaMotoreA{Regole: regole, Limite: 20 * time.Second, leggiFase: leggi(fase, nil), Calcola: riuscito},
			stato: http.StatusOK, esito: true, fase: true, calcoli: 1, mai: "level="},
		// l'ultima: il suo calcolo può partire dopo la fine della sottoprova
		{nome: "la richiesta chiusa da chi l'aveva fatta", annullata: true, a: AnteprimaMotoreA{Regole: regole, Limite: 20 * time.Second,
			leggiFase: leggi(fase, nil), Calcola: calcola(func() error { <-libera; return nil })},
			stato: http.StatusOK, errore: erroreCalcoloMotoreA, fase: true, calcoli: -1, log: "level=INFO", mai: "level=ERROR"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			var registro strings.Builder
			c.a.Accesa = true
			s := &Server{Log: slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelDebug})), MotoreA: c.a}
			calcoli.Store(0)
			ctx := context.WithValue(context.Background(), ctxUtente, admin)
			if c.annullata {
				var annulla context.CancelFunc
				ctx, annulla = context.WithCancel(ctx)
				annulla()
			}
			req := httptest.NewRequest(http.MethodGet, "/thread/"+maUUID(1).String()+"/motore-a/dati", nil).WithContext(ctx)
			req.SetPathValue("id", maUUID(1).String())
			rec, panico := maServi(http.HandlerFunc(s.motoreADati), req)
			if panico != nil {
				t.Fatalf("panic fuori dal gestore: %v", panico)
			}
			if rec.Code != c.stato || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("stato %d, Cache-Control %q; attesi %d e no-store", rec.Code, rec.Header().Get("Cache-Control"), c.stato)
			}
			if c.stato == http.StatusNotFound {
				if g := strings.TrimSpace(rec.Body.String()); g != "RFQ non trovata" {
					t.Errorf("404 con %q, atteso «RFQ non trovata»", g)
				}
			} else {
				var d struct {
					Errore    string          `json:"errore"`
					Esito     json.RawMessage `json:"esito"`
					Confronto json.RawMessage `json:"confronto"`
					Testa     struct {
						Fase *struct {
							Nome string `json:"nome"`
						} `json:"fase"`
					} `json:"testa"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
					t.Fatalf("JSON: %v: %s", err, rec.Body.String())
				}
				if d.Errore != c.errore {
					t.Errorf("errore %q, atteso %q", d.Errore, c.errore)
				}
				if pieni := string(d.Esito) != "null" && string(d.Confronto) != "null"; pieni != c.esito {
					t.Errorf("esito %.40s, confronto %.40s (attesi pieni: %v)", d.Esito, d.Confronto, c.esito)
				}
				if (d.Testa.Fase != nil && d.Testa.Fase.Nome == fase.Nome) != c.fase {
					t.Errorf("fase %+v (attesa: %v)", d.Testa.Fase, c.fase)
				}
				for _, interno := range []string{"ACME rotta", "fuori dall'elenco", "pool ACME", "goroutine", "panic"} {
					if strings.Contains(rec.Body.String(), interno) {
						t.Errorf("il JSON ha un testo interno (%q): %s", interno, rec.Body.String())
					}
				}
			}
			if g := calcoli.Load(); c.calcoli >= 0 && g != c.calcoli {
				t.Errorf("%d calcoli, attesi %d", g, c.calcoli)
			}
			if c.log != "" && !strings.Contains(registro.String(), c.log) {
				t.Errorf("il log non ha %q:\n%s", c.log, registro.String())
			}
			if c.mai != "" && strings.Contains(registro.String(), c.mai) {
				t.Errorf("il log ha %q:\n%s", c.mai, registro.String())
			}
		})
	}
}

// ---------------------------------------------------------------- la fase e la RFQ, in sola lettura (§5.1.6)

// maFaseTx: una transazione finta per faseMotoreADa. Ogni query risponde per nome (la riga «-- name: X» di sqlc) con le
// sue righe (una riga nil = una riga che non riempie niente) o con il suo errore; senza righe pgx.ErrNoRows. Exec,
// Commit e gli altri metodi contano o vanno in panic: la fase non li usa.
type maFaseTx struct {
	pgx.Tx
	risposte map[string]maFaseRisposta
	sql      []string
	exec     int
	rollback int
	commit   int
}

type maFaseRisposta struct {
	righe [][]any
	err   error
}

func maNomeQuery(sql string) string {
	n, _, _ := strings.Cut(strings.TrimPrefix(sql, "-- name: "), " ")
	return n
}

func (x *maFaseTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	x.sql = append(x.sql, sql)
	r := x.risposte[maNomeQuery(sql)]
	switch {
	case r.err != nil:
		return maFaseRiga{err: r.err}
	case len(r.righe) == 0:
		return maFaseRiga{err: pgx.ErrNoRows}
	}
	return maFaseRiga{valori: r.righe[0]}
}

func (x *maFaseTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	x.sql = append(x.sql, sql)
	r := x.risposte[maNomeQuery(sql)]
	if r.err != nil {
		return nil, r.err
	}
	return &maFaseRighe{righe: r.righe, i: -1}, nil
}

func (x *maFaseTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	x.sql = append(x.sql, sql)
	x.exec++
	return pgconn.CommandTag{}, errors.New("la fase non scrive")
}

func (x *maFaseTx) Rollback(context.Context) error { x.rollback++; return nil }

func (x *maFaseTx) Commit(context.Context) error { x.commit++; return nil }

// maFaseRiga: la riga di una QueryRow finta.
type maFaseRiga struct {
	valori []any
	err    error
}

func (r maFaseRiga) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return maAssegna(dest, r.valori)
}

// maFaseRighe: le righe di una Query finta (Next, Scan, Err, Close: quelli che sqlc usa).
type maFaseRighe struct {
	pgx.Rows
	righe [][]any
	i     int
}

func (r *maFaseRighe) Next() bool             { r.i++; return r.i < len(r.righe) }
func (r *maFaseRighe) Scan(dest ...any) error { return maAssegna(dest, r.righe[r.i]) }
func (r *maFaseRighe) Err() error             { return nil }
func (r *maFaseRighe) Close()                 {}

// maAssegna copia i valori nelle destinazioni di Scan (stesso numero, stessi tipi); valori nil = niente da copiare.
func maAssegna(dest []any, valori []any) error {
	if valori == nil {
		return nil
	}
	if len(dest) != len(valori) {
		return fmt.Errorf("%d colonne, %d valori", len(dest), len(valori))
	}
	for i := range dest {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(valori[i]))
	}
	return nil
}

// maFaseIniziatore: l'iniziatore finto. Registra le opzioni chieste e dà la transazione finta, o il suo errore.
type maFaseIniziatore struct {
	opzioni []pgx.TxOptions
	tx      *maFaseTx
	errore  error
}

func (i *maFaseIniziatore) BeginTx(_ context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	i.opzioni = append(i.opzioni, o)
	if i.errore != nil {
		return nil, i.errore
	}
	return i.tx, nil
}

// La fase e l'esistenza della RFQ (T-A1d-02, T-A1d-12; P-12), senza database: una sola transazione, READ ONLY e
// nient'altro (piano §5.1.6), chiusa sempre con il rollback e mai con il commit; solo SELECT, nessun Exec. La RFQ che non
// c'è (GetThread senza righe) è errRFQAssente, cioè il 404 del gestore, senza leggere la fase; nessuna fase aperta = nil;
// l'etichetta viene dal catalogo (qui un testo inventato, mai una descrizione del catalogo vero: RAC-26), "" se manca;
// un errore del database torna avvolto e non è errRFQAssente; una transazione che non si apre non legge niente. Che
// s.Pool apra davvero una transazione in sola lettura e che la GET non scriva lo provano A1d-L4-01…03 sul DB.
func TestMotoreALaFaseInSolaLettura(t *testing.T) {
	ro := pgx.TxOptions{AccessMode: pgx.ReadOnly}
	thread := maUUID(1)
	inizio := time.Date(2026, 9, 28, 8, 0, 0, 0, time.UTC)
	rigaFase := []any{thread, maUUID(70), db.FaseFATTIBILITA, inizio, uuid.NullUUID{}, int32(3), pgtype.Int4{Int32: 5, Valid: true}, false, int32(3), "giallo"}
	catalogo := [][]any{
		{db.FaseRICEVUTA, int32(1), "Ricevuta ACME di prova", "ufficio ACME", pgtype.Int4{}, false},
		{db.FaseFATTIBILITA, int32(3), "Fattibilità ACME di prova", "ufficio ACME", pgtype.Int4{Int32: 5, Valid: true}, false},
	}
	errRete := errors.New("connessione ACME persa")
	errPool := errors.New("pool ACME chiuso")
	laRFQ := maFaseRisposta{righe: [][]any{nil}}
	for _, c := range []struct {
		nome     string
		risposte map[string]maFaseRisposta
		apertura error
		fase     *faseRegistrata
		err      error // errors.Is
		letture  int
		rollback int
	}{
		{nome: "la RFQ non c'è", risposte: map[string]maFaseRisposta{}, err: errRFQAssente, letture: 1, rollback: 1},
		{nome: "nessuna fase aperta", risposte: map[string]maFaseRisposta{"GetThread": laRFQ}, letture: 2, rollback: 1},
		{nome: "la fase con il catalogo", risposte: map[string]maFaseRisposta{"GetThread": laRFQ, "GetFaseCorrente": {righe: [][]any{rigaFase}},
			"ListFaseCatalogo": {righe: catalogo}}, letture: 3, rollback: 1,
			fase: &faseRegistrata{Nome: "FATTIBILITA", Etichetta: "Fattibilità ACME di prova", Inizio: inizio, GgInFase: 3, Semaforo: "giallo"}},
		{nome: "la fase senza voce nel catalogo", risposte: map[string]maFaseRisposta{"GetThread": laRFQ, "GetFaseCorrente": {righe: [][]any{rigaFase}},
			"ListFaseCatalogo": {righe: catalogo[:1]}}, letture: 3, rollback: 1,
			fase: &faseRegistrata{Nome: "FATTIBILITA", Inizio: inizio, GgInFase: 3, Semaforo: "giallo"}},
		{nome: "la RFQ non si legge", risposte: map[string]maFaseRisposta{"GetThread": {err: errRete}}, err: errRete, letture: 1, rollback: 1},
		{nome: "la fase non si legge", risposte: map[string]maFaseRisposta{"GetThread": laRFQ, "GetFaseCorrente": {err: errRete}}, err: errRete,
			letture: 2, rollback: 1},
		{nome: "la transazione non si apre", apertura: errPool, err: errPool},
	} {
		t.Run(c.nome, func(t *testing.T) {
			tx := &maFaseTx{risposte: c.risposte}
			ini := &maFaseIniziatore{tx: tx, errore: c.apertura}
			fase, err := faseMotoreADa(context.Background(), ini, thread)
			if !reflect.DeepEqual(fase, c.fase) {
				t.Errorf("fase %+v, attesa %+v", fase, c.fase)
			}
			if (c.err == nil) != (err == nil) || (c.err != nil && !errors.Is(err, c.err)) {
				t.Errorf("errore %v, atteso %v", err, c.err)
			}
			if c.err != errRFQAssente && errors.Is(err, errRFQAssente) {
				t.Errorf("errore %v: un errore del database non è la RFQ che non c'è", err)
			}
			if len(ini.opzioni) != 1 || ini.opzioni[0] != ro {
				t.Errorf("opzioni %+v, attese una volta %+v", ini.opzioni, ro)
			}
			if len(tx.sql) != c.letture || tx.rollback != c.rollback || tx.commit != 0 || tx.exec != 0 {
				t.Errorf("%d letture, %d rollback, %d commit, %d exec; attesi %d, %d, 0, 0", len(tx.sql), tx.rollback, tx.commit, tx.exec, c.letture, c.rollback)
			}
			for _, q := range tx.sql {
				m := strings.ToUpper(q)
				if !strings.Contains(m, "SELECT") || strings.Contains(m, "INSERT") || strings.Contains(m, "UPDATE") ||
					strings.Contains(m, "DELETE") || strings.Contains(m, "FOR SHARE") {
					t.Errorf("non è una lettura semplice: %s", q)
				}
			}
		})
	}
}

// ---------------------------------------------------------------- le guardie sul sorgente

// maFileMotoreA: i file Go del pacchetto con il prefisso motore_a (con le prove o senza), letti dal disco.
func maFileMotoreA(t *testing.T, prove bool) map[string]*ast.File {
	t.Helper()
	voci, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, v := range voci {
		n := v.Name()
		if v.IsDir() || !strings.HasPrefix(n, "motore_a") || !strings.HasSuffix(n, ".go") || (!prove && strings.HasSuffix(n, "_test.go")) {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = f
	}
	if len(out) < 5 {
		t.Fatalf("letti %d file motore_a*.go: la lettura non funziona", len(out))
	}
	return out
}

// Una GET di motore-a non scrive (contratto §2.1; piano §5.1.9): nei file di prodotto motore_a*.go nessun Commit, Exec,
// Begin, BeginFunc, CopyFrom, SendBatch; ogni BeginTx ha AccessMode: pgx.ReadOnly; le query chiamate su db.New(…) sono
// solo GetThread, GetFaseCorrente e ListFaseCatalogo; nessuna chiamata a threadFrammento, gesto o gestoPoi (i gesti sono
// del ponte). Le letture del caricatore e di EffettoAutorizzazione hanno le loro prove (caricatore, A1d-L4-03, -05).
func TestMotoreANessunaScritturaNelCodice(t *testing.T) {
	vietate := map[string]bool{"Commit": true, "Exec": true, "Begin": true, "BeginFunc": true, "CopyFrom": true, "SendBatch": true,
		"threadFrammento": true, "gesto": true, "gestoPoi": true}
	ammesse := map[string]bool{"GetThread": true, "GetFaseCorrente": true, "ListFaseCatalogo": true}
	dbNew := func(e ast.Expr) bool {
		c, ok := e.(*ast.CallExpr)
		return ok && types.ExprString(c.Fun) == "db.New"
	}
	transazioni, letture := 0, 0
	for nome, f := range maFileMotoreA(t, false) {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			// le variabili di questa funzione che valgono db.New(…)
			query := map[string]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if a, ok := n.(*ast.AssignStmt); ok && len(a.Lhs) == len(a.Rhs) {
					for i, r := range a.Rhs {
						if id, ok := a.Lhs[i].(*ast.Ident); ok && dbNew(r) {
							query[id.Name] = true
						}
					}
				}
				return true
			})
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				c, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := c.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if vietate[sel.Sel.Name] {
					t.Errorf("%s, %s: %s", nome, fd.Name.Name, types.ExprString(c.Fun))
				}
				if sel.Sel.Name == "BeginTx" {
					transazioni++
					sola := false
					if len(c.Args) == 2 {
						ast.Inspect(c.Args[1], func(m ast.Node) bool {
							if kv, ok := m.(*ast.KeyValueExpr); ok && types.ExprString(kv.Key) == "AccessMode" && types.ExprString(kv.Value) == "pgx.ReadOnly" {
								sola = true
							}
							return true
						})
					}
					if !sola {
						t.Errorf("%s, %s: una transazione senza AccessMode: pgx.ReadOnly", nome, fd.Name.Name)
					}
				}
				id, ok := sel.X.(*ast.Ident)
				if (ok && query[id.Name]) || dbNew(sel.X) {
					letture++
					if !ammesse[sel.Sel.Name] {
						t.Errorf("%s, %s: %s non è fra le letture della pagina", nome, fd.Name.Name, types.ExprString(c.Fun))
					}
				}
				return true
			})
		}
	}
	if transazioni != 2 || letture != 3 {
		t.Errorf("%d BeginTx e %d letture su db.New nei file motore_a*.go, attese 2 (la fase, l'anteprima dello STEP) e 3", transazioni, letture)
	}
}

// MOTORE-SENZA-LLM (piano §5.1.9; RAC-21): in nessun file motore_a*.go del pacchetto (prove comprese) e nel ponte
// (da risorse.FS) c'è il nome della tabella dell'agente, maiuscole indifferenti; i file di prodotto non importano
// internal/ai né internal/app (transport non importa app, internal/README.md).
func TestMotoreANessunaTabellaDellAgente(t *testing.T) {
	vietata := "analisi" + "_messaggio"
	for nome := range maFileMotoreA(t, true) {
		b, err := os.ReadFile(nome)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), vietata) {
			t.Errorf("%s nomina la tabella dell'agente", nome)
		}
	}
	ponte, err := fs.ReadFile(risorse.FS, "web/static/motore_a_gesti.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(ponte)), vietata) {
		t.Error("il ponte nomina la tabella dell'agente")
	}
	for nome, f := range maFileMotoreA(t, false) {
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, "promatec/cockpit/internal/ai/") || strings.HasPrefix(p, "promatec/cockpit/internal/app/") {
				t.Errorf("%s importa %s", nome, p)
			}
		}
	}
}
