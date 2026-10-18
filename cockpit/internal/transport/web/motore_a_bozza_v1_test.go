package web

// L1 — V1, ondata B, corsia persistenza: le rotte della bozza della Distinta unica (motore_a_bozza_v1.go: R1, R2, R3, R6,
// R7, R11), con httptest e senza database:
//   - flag spento: ogni metodo su ogni indirizzo risponde come un indirizzo che non esiste, prima della sessione (Pool nil);
//   - flag acceso: senza sessione il rimando al login con no-store; gli altri metodi 405 con Allow e no-store; chi non e'
//     admin il 403 di nega; l'admin arriva al gestore (il 400 di un id del percorso, che viene prima di ogni lettura del
//     database: l'esistenza della RFQ, il 413 e i 422 della forma li provano le L4, EM §9 «prima l'esistenza»);
//   - le rotte si montano accanto a quelle di Registra senza conflitti di pattern (anche dopo che registraMotoreA le monta);
//   - la traduzione degli errori: ogni codice dell'elenco unico (CodiciServerBozzaV1) con il suo stato, l'esito
//     «conflitto» per i 409, il 500 generico senza dettaglio (T-15: nessun percorso nella risposta);
//   - T-05: l'elenco unico dei codici del server contro i cataloghi del ponte (CONFLITTI per i 409, RIFIUTI per i 422);
//   - T-12 (meta' L1, TestV1NessunaBozzaFormato2Nuova): R2 salva solo con SalvaFormato3, nessun codice di produzione chiama
//     il salvataggio formato 2 (anche dentro distintabozza, senza il nome del pacchetto) ne' scrive una testata di bozza con
//     un formato che non vale 3; T-12b (meta' L1): nessuna strada del Go scrive una conferma con un formato che non vale 3
//     (il valore delle costanti e delle conversioni int16(...) si calcola, non si indovina dal nome). Tutte e due provano il
//     mutante del revisore come caso negativo.
// Dati inventati: UUID 00000000-0000-4000-8000-0000000000nn (mwUUID).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// bzRotta: un indirizzo delle rotte della bozza, con i metodi ammessi (l'Allow della riga gemella).
type bzRotta struct {
	nome, percorso, pattern, ammessi string
}

func bzRotte() []bzRotta {
	id := mwUUID(1).String()
	return []bzRotta{
		{"R1", "/thread/" + id + "/motore-a/bozza?prodotto=" + mwUUID(10).String(), "GET /thread/{id}/motore-a/bozza", "GET, HEAD, POST"},
		{"R2", "/thread/" + id + "/motore-a/bozza", "POST /thread/{id}/motore-a/bozza", "GET, HEAD, POST"},
		{"R3", "/thread/" + id + "/motore-a/bozza/" + mwUUID(20).String() + "/abbandona", "POST /thread/{id}/motore-a/bozza/{bozza}/abbandona", "POST"},
		{"R6", "/thread/" + id + "/motore-a/intento/" + mwUUID(30).String(), "GET /thread/{id}/motore-a/intento/{chiave}", "GET, HEAD"},
		{"R7", "/thread/" + id + "/motore-a/conferme?prodotto=" + mwUUID(10).String(), "GET /thread/{id}/motore-a/conferme", "GET, HEAD"},
		{"R11", "/thread/" + id + "/motore-a/riallinea", "POST /thread/{id}/motore-a/riallinea", "POST"},
	}
}

func (r bzRotta) metodo() string {
	return strings.SplitN(r.pattern, " ", 2)[0]
}

// bzMux: il mux con Registra e le rotte della bozza (montate qui finche' registraMotoreA non le monta: richiesta nel
// resoconto della corsia).
func bzMux(t *testing.T, s *Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Fatalf("Registra o registraBozzaMotoreA vanno in panic (un pattern in conflitto?): %v", v)
			}
		}()
		s.Registra(mux)
		if _, pat := mux.Handler(httptest.NewRequest(http.MethodPost, bzRotte()[1].percorso, nil)); pat != bzRotte()[1].pattern {
			s.registraBozzaMotoreA(mux)
		}
	}()
	return mux
}

func TestBozzaV1SpentoComeUnIndirizzoCheNonCe(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Stato: motorea_web.StatoSpenta})
	mux := bzMux(t, s)
	for _, r := range bzRotte() {
		for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete} {
			for _, cookie := range []bool{false, true} {
				req := httptest.NewRequest(metodo, r.percorso, strings.NewReader("{}"))
				if cookie {
					req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
				}
				rec, panico := mwServi(mux, req)
				if panico != nil {
					t.Errorf("%s %s cookie=%v: la porta ha toccato la sessione (Pool nil): %v", metodo, r.nome, cookie, panico)
					continue
				}
				rif, _ := mwServi(mux, httptest.NewRequest(metodo, "/v1-indirizzo-che-non-esiste", nil))
				if rec.Code != http.StatusNotFound || rec.Body.String() != rif.Body.String() || mwIntestazioni(rec.Header()) != mwIntestazioni(rif.Header()) {
					t.Errorf("%s %s cookie=%v: %d %q %s, atteso il 404 di un indirizzo che non esiste", metodo, r.nome, cookie, rec.Code, rec.Body, mwIntestazioni(rec.Header()))
				}
			}
		}
	}
}

func TestBozzaV1AccesoSenzaSessioneEAltriMetodi(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	mux := bzMux(t, s)
	for _, r := range bzRotte() {
		req := httptest.NewRequest(r.metodo(), r.percorso, strings.NewReader("{}"))
		rec, panico := mwServi(mux, req)
		if panico != nil {
			t.Errorf("%s senza cookie: panic: %v", r.nome, panico)
			continue
		}
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s senza sessione: %d %q %q, attesi 302 verso /login con no-store", r.nome, rec.Code, rec.Header().Get("Location"), rec.Header().Get("Cache-Control"))
		}
		for _, metodo := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
			req := httptest.NewRequest(metodo, r.percorso, nil)
			req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
			rec, panico := mwServi(mux, req)
			if panico != nil {
				t.Errorf("%s %s: ha letto la sessione: %v", metodo, r.nome, panico)
				continue
			}
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != r.ammessi || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s %s: %d, Allow %q, Cache-Control %q (attesi 405, %q, no-store)", metodo, r.nome, rec.Code,
					rec.Header().Get("Allow"), rec.Header().Get("Cache-Control"), r.ammessi)
			}
		}
	}
}

// Il ruolo: la catena meno autenticato (che legge la sessione dal database). Per l'admin il gestore parte e rifiuta la
// richiesta prima di toccare il database (Pool nil): un id del percorso che non e' un UUID (400), che viene prima
// dell'esistenza della RFQ (EM §9: prima l'esistenza, poi la forma; i 422 della forma e il 404 sono nelle L4).
func TestBozzaV1SoloLAdmin(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	id := mwUUID(1).String()
	casi := []struct {
		nome, metodo, percorso, corpo string
		valori                        map[string]string
		h                             http.HandlerFunc
		stato                         int
	}{
		{"R1 thread non valido", http.MethodGet, "/thread/xyz/motore-a/bozza?prodotto=" + id, "", map[string]string{"id": "xyz"}, s.bozzaV1Leggi, http.StatusBadRequest},
		{"R2 thread non valido", http.MethodPost, "/thread/xyz/motore-a/bozza", "{}", map[string]string{"id": "xyz"}, s.bozzaV1Salva, http.StatusBadRequest},
		{"R3 bozza non valida", http.MethodPost, "/thread/" + id + "/motore-a/bozza/xyz/abbandona", "{}", map[string]string{"id": id, "bozza": "xyz"}, s.bozzaV1Abbandona, http.StatusBadRequest},
		{"R6 chiave non valida", http.MethodGet, "/thread/" + id + "/motore-a/intento/xyz", "", map[string]string{"id": id, "chiave": "xyz"}, s.bozzaV1Intento, http.StatusBadRequest},
		{"R7 thread non valido", http.MethodGet, "/thread/xyz/motore-a/conferme?prodotto=" + id, "", map[string]string{"id": "xyz"}, s.bozzaV1Storico, http.StatusBadRequest},
		{"R11 thread non valido", http.MethodPost, "/thread/xyz/motore-a/riallinea", `{}`, map[string]string{"id": "xyz"}, s.bozzaV1Riallinea, http.StatusBadRequest},
	}
	for _, c := range casi {
		porta := s.conMotoreA(conNoStore(s.soloRuolo(db.RuoloUtenteAdmin, c.h)))
		for _, ruolo := range db.AllRuoloUtenteValues() {
			req := httptest.NewRequest(c.metodo, c.percorso, strings.NewReader(c.corpo))
			for k, v := range c.valori {
				req.SetPathValue(k, v)
			}
			req.Header.Set("HX-Request", "true")
			req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "acme", Ruolo: ruolo}))
			rec, panico := mwServi(porta, req)
			if panico != nil {
				t.Errorf("%s, %s: panic (il gestore ha letto il database?): %v", c.nome, ruolo, panico)
				continue
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s, %s: Cache-Control %q", c.nome, ruolo, rec.Header().Get("Cache-Control"))
			}
			if ruolo != db.RuoloUtenteAdmin {
				if rec.Code != http.StatusForbidden || strings.Contains(rec.Header().Get("Content-Type"), "json") {
					t.Errorf("%s, %s: %d %q, atteso il 403 HTML di nega", c.nome, ruolo, rec.Code, rec.Header().Get("Content-Type"))
				}
				continue
			}
			var corpo rifiutoBozzaV1
			if rec.Code != c.stato || json.Unmarshal(rec.Body.Bytes(), &corpo) != nil || corpo.Esito != "rifiutato" || corpo.Codice == "" ||
				rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("%s, admin: %d %s, atteso %d con il rifiuto JSON", c.nome, rec.Code, rec.Body, c.stato)
			}
		}
	}
}

// Il corpo oltre 2 MiB + 4 KiB: 413 TROPPO_GRANDE (leggiCorpoV1, senza database; nelle rotte viene dopo l'esistenza
// della RFQ, che le L4 provano).
func TestBozzaV1CorpoTroppoGrande(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	req := httptest.NewRequest(http.MethodPost, "/thread/"+mwUUID(1).String()+"/motore-a/bozza", bytes.NewReader(bytes.Repeat([]byte(" "), maxCorpoBozzaV1+1)))
	rec := httptest.NewRecorder()
	if _, ok := s.leggiCorpoV1(rec, req); ok || rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), codiceTroppoGrandeV1) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

func TestBozzaV1LeRotteSiMontano(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{})
	mux := bzMux(t, s)
	for _, r := range bzRotte() {
		metodi := []string{r.metodo()}
		if r.metodo() == http.MethodGet {
			metodi = append(metodi, http.MethodHead)
		}
		for _, m := range metodi {
			if _, pat := mux.Handler(httptest.NewRequest(m, r.percorso, nil)); pat != r.pattern {
				t.Errorf("%s %s: pattern %q, atteso %q", m, r.nome, pat, r.pattern)
			}
		}
		gemello := strings.SplitN(r.pattern, " ", 2)[1]
		if _, pat := mux.Handler(httptest.NewRequest(http.MethodDelete, r.percorso, nil)); pat != gemello {
			t.Errorf("DELETE %s: pattern %q, atteso %q", r.nome, pat, gemello)
		}
	}
	// nel sorgente: ogni rotta e' catena(s.gestore) o gemella("…"), e la catena e' quella di motore_a.go
	src, err := os.ReadFile("motore_a_bozza_v1.go")
	if err != nil {
		t.Fatal(err)
	}
	testo := strings.ReplaceAll(string(src), "\r\n", "\n")
	if !strings.Contains(testo, "catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }") ||
		!strings.Contains(testo, "return s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1(ammessi)))") {
		t.Error("la catena delle rotte della bozza non e' conMotoreA → conNoStore → soloAdmin (o la gemella senza sessione)")
	}
	re := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)", (catena\(s\.bozzaV1[A-Za-z]+\)|gemella\("[A-Z, ]+"\))\)`)
	if n := len(re.FindAllString(testo, -1)); n != 11 || strings.Count(testo, "mux.HandleFunc(") != 11 {
		t.Errorf("registraBozzaMotoreA: %d righe nella forma attesa su %d (attese 11: sei rotte e cinque gemelle, R1 e R2 sullo stesso indirizzo)", n, strings.Count(testo, "mux.HandleFunc("))
	}
}

// La traduzione degli errori (EM §9): ogni codice dell'elenco unico con il suo stato, «conflitto» per i 409, i campi del
// conflitto; il 500 e' generico (T-15: niente dettaglio, niente percorsi).
func TestBozzaV1RispostaErrore(t *testing.T) {
	for _, c := range CodiciServerBozzaV1 {
		stato, corpo := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: c.Codice, Campo: "x", Motivo: "m", Messaggio: "testo"})
		atteso := "rifiutato"
		if c.Stato == http.StatusConflict {
			atteso = "conflitto"
		}
		if stato != c.Stato || corpo.Esito != atteso || corpo.Codice != c.Codice || corpo.Campo != "x" || corpo.Motivo != "m" || corpo.Testo != "testo" {
			t.Errorf("%s %s: %d %+v", c.Rotta, c.Codice, stato, corpo)
		}
		if statoDelCodiceV1[c.Codice] != c.Stato {
			t.Errorf("%s: statoDelCodiceV1 %d, elenco %d", c.Codice, statoDelCodiceV1[c.Codice], c.Stato)
		}
	}
	if stato, _ := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: distintabozza.CodiceRFQInesistente}); stato != http.StatusNotFound {
		t.Errorf("RFQ_INESISTENTE: %d", stato)
	}
	stato, corpo := rispostaErroreBozzaV1(fascicolo.Rifiuto("il componente non e' della RFQ"))
	if stato != http.StatusUnprocessableEntity || corpo.Codice != distintabozza.CodiceFuoriRFQ {
		t.Errorf("fascicolo.Rifiuto: %d %+v", stato, corpo)
	}
	segreto := fmt.Errorf("lettura di C:\\dati\\cockpit\\x.stp: %w", errors.New(`\\server\condivisa`))
	stato, corpo = rispostaErroreBozzaV1(segreto)
	b, _ := json.Marshal(corpo)
	if stato != http.StatusInternalServerError || corpo.Esito != "errore" || bzHaPercorsi(string(b)) {
		t.Errorf("500: %d %s", stato, b)
	}
	// un codice che le rotte della bozza non conoscono e' un 500, non un 4xx inventato
	if stato, _ := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: "NUOVO_CODICE"}); stato != http.StatusInternalServerError {
		t.Errorf("codice sconosciuto: %d", stato)
	}
	// il conflitto di versione porta i campi della bozza
	c := &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Conflitto: &distintabozza.Conflitto{BozzaID: mwUUID(20), Stato: "aperta", Versione: 3,
		AggiornataDa: mwUUID(9)}}
	stato, corpo = rispostaErroreBozzaV1(c)
	if stato != http.StatusConflict || corpo.BozzaID == nil || *corpo.BozzaID != mwUUID(20) || corpo.Versione == nil || *corpo.Versione != 3 || corpo.StatoBozza != "aperta" {
		t.Errorf("conflitto: %d %+v", stato, corpo)
	}
}

// Il 500 di R11 (F8 non riuscito) sta nella busta unica degli errori {esito, codice, testo, …} (EM §9, REV RS-08), con
// f8_stato e aggiornati in piu'; il 200 ha solo f8_stato e aggiornati.
func TestBozzaV1R11NellaBustaUnica(t *testing.T) {
	b, err := json.Marshal(erroreR11(2))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["esito"] != "errore" || m["codice"] != codiceErroreV1 || m["testo"] == "" || m["f8_stato"] != "errore" || m["aggiornati"] != float64(2) || bzHaPercorsi(string(b)) {
		t.Errorf("500 di R11: %s", b)
	}
	ok, _ := json.Marshal(rispostaR11{F8Stato: "eseguito", Aggiornati: 1})
	if string(ok) != `{"f8_stato":"eseguito","aggiornati":1}` {
		t.Errorf("200 di R11: %s", ok)
	}
}

// bzHaPercorsi: T-15, una lettera di unita' (C:\) o una barra rovescia in una risposta JSON.
var reBzPercorso = regexp.MustCompile(`[A-Za-z]:\\\\|\\\\`)

func bzHaPercorsi(s string) bool { return reBzPercorso.MatchString(s) }

// T-05: l'elenco unico dei codici delle rotte della bozza contro i cataloghi del ponte, rotta per rotta: un 409 sta in
// CONFLITTI della funzione del ponte, un 422 in RIFIUTI; ogni codice di statoDelCodiceV1 (tranne il 404) sta nell'elenco.
func TestBozzaV1T05CodiciControIlPonte(t *testing.T) {
	b, err := fs.ReadFile(risorse.FS, "web/static/distinta_unica_ponte.mjs")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.ReplaceAll(string(b), "\r\n", "\n")
	i, j := strings.Index(src, "// CATALOGHI:inizio"), strings.Index(src, "// CATALOGHI:fine")
	if i < 0 || j < i {
		t.Fatal("il ponte non ha il blocco CATALOGHI")
	}
	blocco := strings.ReplaceAll(src[i:j], "\n  ", " ")
	elenco := func(s string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`"([A-Z_]+)"`).FindAllStringSubmatch(s, -1) {
			out = append(out, m[1])
		}
		return out
	}
	costanti := map[string][]string{}
	for _, m := range regexp.MustCompile(`(?m)^const (RIFIUTI_R2|RIFIUTI_FILE) = (\[[^\]]*\]);`).FindAllStringSubmatch(blocco, -1) {
		costanti[m[1]] = elenco(m[2])
	}
	catalogo := func(nome string) map[string][]string {
		k := strings.Index(blocco, "const "+nome+" = {")
		if k < 0 {
			t.Fatalf("manca %s", nome)
		}
		fine := strings.Index(blocco[k:], "};")
		out := map[string][]string{}
		for _, m := range regexp.MustCompile(`(\w+): (\[[^\]]*\]|[A-Z0-9_]+(?:\.concat\(\[[^\]]*\]\))?)`).FindAllStringSubmatch(blocco[k:k+fine], -1) {
			v := m[2]
			switch {
			case strings.HasPrefix(v, "["):
				out[m[1]] = elenco(v)
			case strings.Contains(v, ".concat("):
				out[m[1]] = append(append([]string{}, costanti[v[:strings.Index(v, ".")]]...), elenco(v)...)
			default:
				out[m[1]] = costanti[v]
			}
		}
		return out
	}
	conflitti, rifiuti := catalogo("CONFLITTI"), catalogo("RIFIUTI")
	funzione := map[string]string{"R1": "leggiBozza", "R2": "salvaBozza", "R3": "abbandonaBozza", "R6": "esitoIntento", "R7": "storicoConferme",
		"R11": "riallineaDocumenti"}
	contiene := func(xx []string, x string) bool {
		for _, y := range xx {
			if y == x {
				return true
			}
		}
		return false
	}
	nell := map[string]bool{}
	for _, c := range CodiciServerBozzaV1 {
		nell[c.Codice] = true
		f, ok := funzione[c.Rotta]
		if !ok {
			t.Errorf("%s: rotta senza funzione del ponte", c.Rotta)
			continue
		}
		if _, ha := rifiuti[f]; !ha && c.Rotta == "R11" {
			// il ponte non ha ancora il catalogo dei rifiuti di riallineaDocumenti: richiesta RQ-3 alla corsia interfaccia;
			// appena c'e', la prova lo confronta come gli altri
			t.Logf("%s %s: il ponte non ha ancora RIFIUTI.%s (RQ-3)", c.Rotta, c.Codice, f)
			continue
		}
		switch c.Stato {
		case http.StatusConflict:
			if !contiene(conflitti[f], c.Codice) {
				t.Errorf("%s %s (409): non e' fra i CONFLITTI di %s %v", c.Rotta, c.Codice, f, conflitti[f])
			}
		case http.StatusUnprocessableEntity:
			if !contiene(rifiuti[f], c.Codice) {
				t.Errorf("%s %s (422): non e' fra i RIFIUTI di %s %v", c.Rotta, c.Codice, f, rifiuti[f])
			}
		default:
			t.Errorf("%s %s: stato %d fuori dai cataloghi", c.Rotta, c.Codice, c.Stato)
		}
	}
	for codice, stato := range statoDelCodiceV1 {
		if stato != http.StatusNotFound && !nell[codice] {
			t.Errorf("%s: in statoDelCodiceV1 ma non nell'elenco unico", codice)
		}
	}
}

// T-12, meta' L1 (TestV1NessunaBozzaFormato2Nuova): il gestore di R2 legge il corpo con LeggiRichiestaR2 e salva solo con
// SalvaFormato3; nessun codice di produzione (fuori dalle prove) chiama il salvataggio formato 2 (distintabozza.Salva, e
// Salva senza il nome del pacchetto dentro distintabozza) ne' costruisce i parametri di una testata di bozza
// (InsertDistintaBozzaParams, InsertDistintaBozzaConIDParams) con un Formato che non vale 3, tranne il Salva del formato 2
// stesso, che nessuno chiama. Il mutante del revisore (una chiamata a Salva dentro il pacchetto, una testata con
// FormatoSnapshot) e' il caso negativo. La meta' L4 manda un corpo formato 2 a R2 e conta le righe.
func TestV1NessunaBozzaFormato2Nuova(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "motore_a_bozza_v1.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	chiamate := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "bozzaV1Salva" {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok {
						chiamate[x.Name+"."+sel.Sel.Name] = true
					}
				}
			}
			return true
		})
	}
	if !chiamate["distintabozza.LeggiRichiestaR2"] || !chiamate["distintabozza.SalvaFormato3"] || chiamate["distintabozza.Salva"] {
		t.Errorf("bozzaV1Salva chiama %v: attesi LeggiRichiestaR2 e SalvaFormato3, mai Salva", chiamate)
	}
	m := bzLeggiModulo(t)
	if v := bzViolazioniT12(m, m.file); len(v) > 0 {
		t.Errorf("T-12: %s", strings.Join(v, "\n"))
	}
	// il caso negativo: il mutante del revisore, aggiunto ai file del pacchetto distintabozza
	if v := bzViolazioniT12(m, []bzFileGo{bzMutante(m)}); len(v) != 2 {
		t.Errorf("T-12 sul mutante: %d violazioni, attese 2 (Salva senza il pacchetto, la testata con FormatoSnapshot):\n%s", len(v), strings.Join(v, "\n"))
	}
	if v := bzViolazioniT12(m, []bzFileGo{bzBuono(m)}); len(v) != 0 {
		t.Errorf("T-12 sul caso buono: %s", strings.Join(v, "\n"))
	}
}

// reInsertConferma: una scrittura di conferma_strutturale scritta in SQL.
var reInsertConferma = regexp.MustCompile(`(?is)INSERT\s+INTO\s+conferma_strutturale\s*\(([^)]*)\)\s*VALUES\s*\(`)

// T-12b, meta' L1: nessuna strada del Go scrive una conferma con un formato che non vale 3. Le query che scrivono
// conferma_strutturale sono una sola (InsertConfermaStrutturale, con il formato come parametro); ogni chiamata del codice di
// produzione le passa i parametri in un letterale (cosi' il Formato si verifica), e il Formato del letterale e' una
// costante che vale 3: il valore si calcola dalle costanti del modulo e dalle conversioni (int16(FormatoSnapshot) vale 2),
// non si indovina dal nome; un Formato che non e' costante non passa. Nessun SQL scritto a mano nel Go scrive la testata
// della conferma. Il mutante del revisore e' il caso negativo.
func TestV1T12bNessunaConfermaFormato2(t *testing.T) {
	radice := bzRadiceModulo(t)
	query, err := filepath.Glob(filepath.Join(radice, "internal", "platform", "db", "queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reInsert := reInsertConferma
	scrittori := 0
	for _, q := range query {
		b, err := os.ReadFile(q)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reInsert.FindAllStringSubmatchIndex(string(b), -1) {
			scrittori++
			resto := string(b)[m[1]:]
			if k := strings.Index(resto, ";"); k > 0 {
				resto = resto[:k]
			}
			if !strings.Contains(resto, "sqlc.arg(formato)") {
				t.Errorf("%s: una scrittura di conferma_strutturale senza il formato come parametro", filepath.Base(q))
			}
		}
	}
	if scrittori != 1 {
		t.Errorf("%d query scrivono conferma_strutturale, attesa una (InsertConfermaStrutturale)", scrittori)
	}
	m := bzLeggiModulo(t)
	if v := bzViolazioniT12b(m, m.file); len(v) > 0 {
		t.Errorf("T-12b: %s", strings.Join(v, "\n"))
	}
	// il caso negativo: il mutante del revisore (Formato: FormatoSnapshot e int16(FormatoSnapshot)), piu' un Formato non
	// costante e una chiamata con i parametri in una variabile
	if v := bzViolazioniT12b(m, []bzFileGo{bzMutante(m)}); len(v) != 4 {
		t.Errorf("T-12b sul mutante: %d violazioni, attese 4:\n%s", len(v), strings.Join(v, "\n"))
	}
	if v := bzViolazioniT12b(m, []bzFileGo{bzBuono(m)}); len(v) != 0 {
		t.Errorf("T-12b sul caso buono: %s", strings.Join(v, "\n"))
	}
}

// bzFileGo: un file sorgente di produzione, con il percorso d'importazione del suo pacchetto.
type bzFileGo struct {
	percorso, pacchetto string
	src                 []byte
}

// bzModulo: il nome del modulo, i suoi file di produzione e le costanti intere di primo livello, per pacchetto.
type bzModulo struct {
	nome     string
	file     []bzFileGo
	costanti map[string]map[string]int64
}

func bzLeggiModulo(t *testing.T) bzModulo {
	t.Helper()
	radice := bzRadiceModulo(t)
	gm, err := os.ReadFile(filepath.Join(radice, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := bzModulo{}
	for _, r := range strings.Split(strings.ReplaceAll(string(gm), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(r, "module ") {
			m.nome = strings.TrimSpace(strings.TrimPrefix(r, "module "))
		}
	}
	if m.nome == "" {
		t.Fatal("go.mod senza module")
	}
	for _, p := range bzFileGoDiProduzione(t, radice) {
		rel, err := filepath.Rel(radice, filepath.Dir(p))
		if err != nil {
			t.Fatal(err)
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		m.file = append(m.file, bzFileGo{percorso: p, pacchetto: m.nome + "/" + filepath.ToSlash(rel), src: src})
	}
	m.costanti = bzCostanti(t, m.file)
	if v := m.costanti[m.nome+"/internal/core/rfq/distintabozza"]; v["FormatoSnapshot3"] != 3 || v["FormatoSnapshot"] != 2 {
		t.Fatalf("le costanti dei formati non si leggono: %v", v)
	}
	return m
}

// bzImportazioni: nome locale → percorso d'importazione dei pacchetti importati da un file.
func bzImportazioni(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, is := range f.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err != nil {
			continue
		}
		nome := path.Base(p)
		if is.Name != nil {
			nome = is.Name.Name
		}
		out[nome] = p
	}
	return out
}

type bzCostante struct {
	pacchetto string
	imp       map[string]string
	e         ast.Expr
}

// bzCostanti: le costanti di primo livello con un valore intero che si calcola (un letterale, un'altra costante, una
// conversione), per pacchetto.
func bzCostanti(t *testing.T, ff []bzFileGo) map[string]map[string]int64 {
	t.Helper()
	da := map[string]map[string]bzCostante{}
	for _, f := range ff {
		pf, err := parser.ParseFile(token.NewFileSet(), f.percorso, f.src, 0)
		if err != nil {
			t.Fatal(err)
		}
		imp := bzImportazioni(pf)
		for _, d := range pf.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs, ok := sp.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, n := range vs.Names {
					if i < len(vs.Values) {
						if da[f.pacchetto] == nil {
							da[f.pacchetto] = map[string]bzCostante{}
						}
						da[f.pacchetto][n.Name] = bzCostante{pacchetto: f.pacchetto, imp: imp, e: vs.Values[i]}
					}
				}
			}
		}
	}
	out := map[string]map[string]int64{}
	for giro := 0; giro < 8; giro++ {
		for p, mm := range da {
			for n, c := range mm {
				if v, ok := bzValuta(c.e, c.pacchetto, c.imp, out); ok {
					if out[p] == nil {
						out[p] = map[string]int64{}
					}
					out[p][n] = v
					delete(mm, n)
				}
			}
		}
	}
	return out
}

var bzTipiInteri = map[string]bool{"int": true, "int8": true, "int16": true, "int32": true, "int64": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true}

// bzValuta: il valore intero di un'espressione costante (letterale, costante del pacchetto o di un pacchetto importato,
// conversione a un tipo intero, parentesi); ok falso se non e' costante.
func bzValuta(e ast.Expr, pacchetto string, imp map[string]string, cost map[string]map[string]int64) (int64, bool) {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.INT {
			v, err := strconv.ParseInt(x.Value, 0, 64)
			return v, err == nil
		}
	case *ast.ParenExpr:
		return bzValuta(x.X, pacchetto, imp, cost)
	case *ast.Ident:
		v, ok := cost[pacchetto][x.Name]
		return v, ok
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			if p, ok := imp[id.Name]; ok {
				v, ok := cost[p][x.Sel.Name]
				return v, ok
			}
		}
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok && len(x.Args) == 1 && bzTipiInteri[id.Name] {
			return bzValuta(x.Args[0], pacchetto, imp, cost)
		}
	}
	return 0, false
}

// bzNomeTipo: il nome del tipo di un letterale composto (db.X o X).
func bzNomeTipo(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.Ident:
		return x.Name
	}
	return ""
}

// bzFormato3: "" se il letterale ha il campo Formato con una costante che vale 3; altrimenti il difetto.
func bzFormato3(cl *ast.CompositeLit, pacchetto string, imp map[string]string, cost map[string]map[string]int64) string {
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Formato" {
			v, ok := bzValuta(kv.Value, pacchetto, imp, cost)
			switch {
			case !ok:
				return "Formato non costante: non si verifica"
			case v != distintabozza.FormatoSnapshot3:
				return fmt.Sprintf("Formato %d", v)
			}
			return ""
		}
	}
	return "senza Formato"
}

// bzViolazioniT12: le chiamate al salvataggio formato 2 e le testate di bozza con un formato che non vale 3 (commento di
// TestV1NessunaBozzaFormato2Nuova).
func bzViolazioniT12(m bzModulo, ff []bzFileGo) []string {
	bozza := m.nome + "/internal/core/rfq/distintabozza"
	var out []string
	for _, f := range ff {
		pf, err := parser.ParseFile(token.NewFileSet(), f.percorso, f.src, 0)
		if err != nil {
			out = append(out, f.percorso+": "+err.Error())
			continue
		}
		imp := bzImportazioni(pf)
		for _, d := range pf.Decls {
			fd, ok := d.(*ast.FuncDecl)
			dentroSalva := ok && fd.Recv == nil && fd.Name.Name == "Salva" && f.pacchetto == bozza
			ast.Inspect(d, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					switch fn := x.Fun.(type) {
					case *ast.SelectorExpr:
						if id, ok := fn.X.(*ast.Ident); ok && imp[id.Name] == bozza && fn.Sel.Name == "Salva" {
							out = append(out, f.percorso+": chiama distintabozza.Salva (il salvataggio formato 2)")
						}
					case *ast.Ident:
						if f.pacchetto == bozza && fn.Name == "Salva" && !dentroSalva {
							out = append(out, f.percorso+": chiama Salva (il salvataggio formato 2) dentro distintabozza")
						}
					}
				case *ast.CompositeLit:
					if tipo := bzNomeTipo(x.Type); (tipo == "InsertDistintaBozzaParams" || tipo == "InsertDistintaBozzaConIDParams") && !dentroSalva {
						if msg := bzFormato3(x, f.pacchetto, imp, m.costanti); msg != "" {
							out = append(out, fmt.Sprintf("%s: %s con %s", f.percorso, tipo, msg))
						}
					}
				}
				return true
			})
		}
	}
	return out
}

// bzViolazioniT12b: le scritture di una conferma che T-12b non accetta (commento di TestV1T12bNessunaConfermaFormato2).
func bzViolazioniT12b(m bzModulo, ff []bzFileGo) []string {
	var out []string
	for _, f := range ff {
		if strings.HasSuffix(f.percorso, ".sql.go") {
			continue
		}
		if reInsertConferma.Match(f.src) {
			out = append(out, f.percorso+": SQL scritto a mano che scrive conferma_strutturale")
		}
		pf, err := parser.ParseFile(token.NewFileSet(), f.percorso, f.src, 0)
		if err != nil {
			out = append(out, f.percorso+": "+err.Error())
			continue
		}
		imp := bzImportazioni(pf)
		ast.Inspect(pf, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if bzNomeTipo(x.Type) == "InsertConfermaStrutturaleParams" {
					if msg := bzFormato3(x, f.pacchetto, imp, m.costanti); msg != "" {
						out = append(out, f.percorso+": InsertConfermaStrutturaleParams con "+msg)
					}
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "InsertConfermaStrutturale" {
					cl, ok := (*ast.CompositeLit)(nil), false
					if len(x.Args) == 2 {
						cl, ok = x.Args[1].(*ast.CompositeLit)
					}
					if !ok || bzNomeTipo(cl.Type) != "InsertConfermaStrutturaleParams" {
						out = append(out, f.percorso+": InsertConfermaStrutturale con i parametri fuori da un letterale (il Formato non si verifica)")
					}
				}
			}
			return true
		})
	}
	return out
}

// bzMutante: il mutante del revisore (rev-pers-confini, zz_mutante.go), nel pacchetto distintabozza: una conferma con la
// costante del formato 2 e con la sua conversione, una con un Formato non costante, una chiamata con i parametri in una
// variabile, una chiamata a Salva senza il nome del pacchetto, una testata di bozza con il formato 2.
func bzMutante(m bzModulo) bzFileGo {
	return bzFileGo{percorso: "distintabozza/zz_mutante.go", pacchetto: m.nome + "/internal/core/rfq/distintabozza", src: []byte(`package distintabozza

import (
	"context"

	"promatec/cockpit/internal/platform/db"
)

func mutanteFormato2(ctx context.Context, q *db.Queries, pool Avviatore, s Salvataggio, f int16) {
	_, _ = q.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{Formato: FormatoSnapshot})
	_, _ = q.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{Formato: int16(FormatoSnapshot)})
	p := db.InsertConfermaStrutturaleParams{Formato: f}
	_, _ = q.InsertConfermaStrutturale(ctx, p)
	_, _ = Salva(ctx, pool, s)
	_, _ = q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{Formato: FormatoSnapshot})
}
`)}
}

// bzBuono: le stesse scritture con il formato 3 (la costante e la sua conversione): nessuna violazione.
func bzBuono(m bzModulo) bzFileGo {
	return bzFileGo{percorso: "distintabozza/zz_buono.go", pacchetto: m.nome + "/internal/core/rfq/distintabozza", src: []byte(`package distintabozza

import (
	"context"

	"promatec/cockpit/internal/platform/db"
)

func buonoFormato3(ctx context.Context, q *db.Queries) {
	_, _ = q.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{Formato: FormatoSnapshot3})
	_, _ = q.InsertConfermaStrutturale(ctx, db.InsertConfermaStrutturaleParams{Formato: int16(FormatoSnapshot3)})
	_, _ = q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{Formato: FormatoSnapshot3})
}
`)}
}

// bzRadiceModulo: la cartella di go.mod (le prove girano nella cartella del pacchetto).
func bzRadiceModulo(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		su := filepath.Dir(d)
		if su == d {
			t.Fatal("go.mod non trovato")
		}
		d = su
	}
}

// bzFileGoDiProduzione: i .go del modulo fuori dalle prove (internal/ e cmd/).
func bzFileGoDiProduzione(t *testing.T, radice string) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"internal", "cmd"} {
		base := filepath.Join(radice, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) < 50 {
		t.Fatalf("solo %d file di produzione: la ricerca non guarda dove deve", len(out))
	}
	return out
}
