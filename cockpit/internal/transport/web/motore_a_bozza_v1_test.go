package web

// L1 — V1, ondata B, corsia persistenza: le rotte della bozza della Distinta unica (motore_a_bozza_v1.go: R1, R2, R3, R6,
// R7, R11), con httptest e senza database:
//   - flag spento: ogni metodo su ogni indirizzo risponde come un indirizzo che non esiste, prima della sessione (Pool nil);
//   - flag acceso: senza sessione il rimando al login con no-store; gli altri metodi 405 con Allow e no-store; chi non e'
//     admin il 403 di nega; l'admin arriva al gestore (i rifiuti che non leggono il database: 400, 422, 413);
//   - le rotte si montano accanto a quelle di Registra senza conflitti di pattern (anche dopo che registraMotoreA le monta);
//   - la traduzione degli errori: ogni codice dell'elenco unico (CodiciServerBozzaV1) con il suo stato, l'esito
//     «conflitto» per i 409, il 500 generico senza dettaglio (T-15: nessun percorso nella risposta);
//   - T-05: l'elenco unico dei codici del server contro i cataloghi del ponte (CONFLITTI per i 409, RIFIUTI per i 422);
//   - T-12 (meta' L1, TestV1NessunaBozzaFormato2Nuova): R2 salva solo con SalvaFormato3, nessun codice di produzione chiama
//     il salvataggio formato 2; T-12b (meta' L1): nessuna strada del Go scrive una conferma formato 2.
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
	"path/filepath"
	"regexp"
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
// richiesta prima di toccare il database (Pool nil): un id che non e' un UUID (400), un corpo che non si legge (422).
func TestBozzaV1SoloLAdmin(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	id := mwUUID(1).String()
	casi := []struct {
		nome, metodo, percorso, corpo string
		valori                        map[string]string
		h                             http.HandlerFunc
		stato                         int
	}{
		{"R1 senza prodotto", http.MethodGet, "/thread/" + id + "/motore-a/bozza", "", map[string]string{"id": id}, s.bozzaV1Leggi, http.StatusUnprocessableEntity},
		{"R1 registro non valido", http.MethodGet, "/thread/" + id + "/motore-a/bozza?prodotto=" + id + "&registro=xyz", "", map[string]string{"id": id}, s.bozzaV1Leggi, http.StatusUnprocessableEntity},
		{"R1 effetto senza componente", http.MethodGet, "/thread/" + id + "/motore-a/bozza?prodotto=" + id + "&tipo_nuovo=sciolto", "", map[string]string{"id": id}, s.bozzaV1Leggi, http.StatusUnprocessableEntity},
		{"R1 effetto con un tipo sconosciuto", http.MethodGet, "/thread/" + id + "/motore-a/bozza?prodotto=" + id + "&effetto_cambio_tipo=" + id + "&tipo_nuovo=pezzo", "", map[string]string{"id": id}, s.bozzaV1Leggi, http.StatusUnprocessableEntity},
		{"R2 corpo vuoto", http.MethodPost, "/thread/" + id + "/motore-a/bozza", "{}", map[string]string{"id": id}, s.bozzaV1Salva, http.StatusUnprocessableEntity},
		{"R2 thread non valido", http.MethodPost, "/thread/xyz/motore-a/bozza", "{}", map[string]string{"id": "xyz"}, s.bozzaV1Salva, http.StatusBadRequest},
		{"R3 bozza non valida", http.MethodPost, "/thread/" + id + "/motore-a/bozza/xyz/abbandona", "{}", map[string]string{"id": id, "bozza": "xyz"}, s.bozzaV1Abbandona, http.StatusBadRequest},
		{"R3 corpo senza versione", http.MethodPost, "/thread/" + id + "/motore-a/bozza/" + id + "/abbandona", "{}", map[string]string{"id": id, "bozza": id}, s.bozzaV1Abbandona, http.StatusUnprocessableEntity},
		{"R6 chiave non valida", http.MethodGet, "/thread/" + id + "/motore-a/intento/xyz", "", map[string]string{"id": id, "chiave": "xyz"}, s.bozzaV1Intento, http.StatusBadRequest},
		{"R7 conferma non valida", http.MethodGet, "/thread/" + id + "/motore-a/conferme?prodotto=" + id + "&conferma=xyz", "", map[string]string{"id": id}, s.bozzaV1Storico, http.StatusUnprocessableEntity},
		{"R11 con un corpo", http.MethodPost, "/thread/" + id + "/motore-a/riallinea", `{"tutto":true}`, map[string]string{"id": id}, s.bozzaV1Riallinea, http.StatusUnprocessableEntity},
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

// Il corpo oltre 2 MiB + 4 KiB: 413 TROPPO_GRANDE, prima di leggerlo.
func TestBozzaV1CorpoTroppoGrande(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	req := httptest.NewRequest(http.MethodPost, "/thread/"+mwUUID(1).String()+"/motore-a/bozza", bytes.NewReader(bytes.Repeat([]byte(" "), maxCorpoBozzaV1+1)))
	req.SetPathValue("id", mwUUID(1).String())
	req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Ruolo: db.RuoloUtenteAdmin}))
	rec, panico := mwServi(http.HandlerFunc(s.bozzaV1Salva), req)
	if panico != nil || rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), codiceTroppoGrandeV1) {
		t.Fatalf("%v %d %s", panico, rec.Code, rec.Body)
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
	funzione := map[string]string{"R1": "leggiBozza", "R2": "salvaBozza", "R3": "abbandonaBozza", "R6": "esitoIntento", "R7": "storicoConferme"}
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
// SalvaFormato3; nessun codice di produzione (fuori dalle prove) chiama il salvataggio formato 2 (distintabozza.Salva) ne'
// scrive una testata di bozza con il formato 2. La meta' L4 manda un corpo formato 2 a R2 e conta le righe.
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
	radice := bzRadiceModulo(t)
	for _, file := range bzFileGoDiProduzione(t, radice) {
		pf, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(pf, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok && x.Name == "distintabozza" && sel.Sel.Name == "Salva" {
						t.Errorf("%s: chiama distintabozza.Salva (il salvataggio formato 2)", file)
					}
				}
			}
			return true
		})
	}
}

// T-12b, meta' L1: nessuna strada del Go scrive una conferma formato 2. Le query che scrivono conferma_strutturale sono
// una sola (InsertConfermaStrutturale, con il formato come parametro), e nessun costruttore dei suoi parametri nel codice
// di produzione mette il formato 2 (un letterale 2 o una costante con «2» nel nome); nessun SQL scritto a mano nel Go
// scrive la testata della conferma.
func TestV1T12bNessunaConfermaFormato2(t *testing.T) {
	radice := bzRadiceModulo(t)
	query, err := filepath.Glob(filepath.Join(radice, "internal", "platform", "db", "queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	reInsert := regexp.MustCompile(`(?is)INSERT\s+INTO\s+conferma_strutturale\s*\(([^)]*)\)\s*VALUES\s*\(`)
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
	for _, file := range bzFileGoDiProduzione(t, radice) {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(file, ".sql.go") {
			continue
		}
		if reInsert.Match(b) {
			t.Errorf("%s: SQL scritto a mano che scrive conferma_strutturale", file)
		}
		pf, err := parser.ParseFile(token.NewFileSet(), file, b, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(pf, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := cl.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "InsertConfermaStrutturaleParams" {
				return true
			}
			for _, el := range cl.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Formato" {
					switch v := kv.Value.(type) {
					case *ast.BasicLit:
						if v.Value == "2" {
							t.Errorf("%s: InsertConfermaStrutturaleParams con Formato 2", file)
						}
					case *ast.Ident:
						if strings.Contains(v.Name, "2") {
							t.Errorf("%s: InsertConfermaStrutturaleParams con Formato %s", file, v.Name)
						}
					case *ast.SelectorExpr:
						if strings.Contains(v.Sel.Name, "2") {
							t.Errorf("%s: InsertConfermaStrutturaleParams con Formato %s", file, v.Sel.Name)
						}
					}
				}
			}
			return true
		})
	}
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
