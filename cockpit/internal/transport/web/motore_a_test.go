package web

// L1 — giro 5, A1d-Q2: l'adattatore della Distinta nuova (motore_a.go), con httptest e senza database. Derivate dalle
// prove della PR #18 di Stefano Valo (67dcc02: motore_a_test.go, motore_a_anteprima_test.go), rifatte sull'adattatore di
// A1d-P2:
//   - flag spento: ogni metodo su ogni indirizzo delle due rotte risponde come un indirizzo che non esiste (stesso
//     codice, stesse intestazioni, stesso corpo), PRIMA della sessione (Pool nil: una lettura andrebbe in panic), con e
//     senza cookie, con e senza htmx (R36; Q-A1d-12 = A);
//   - flag acceso: senza sessione 302 verso /login, o 401 con HX-Redirect per htmx, sempre con no-store; gli altri
//     metodi 405 con Allow e no-store; chi non è admin il 403 di nega con no-store, l'admin arriva al gestore;
//   - registraMotoreA monta proprio conMotoreA(conNoStore(soloAdmin(gestore))) e le righe gemelle, e solo le due rotte
//     JSON: la pagina /thread/{id}/motore-a non c'è (P4), la funzione etichetta non è fra quelle dei template;
//   - i codici: 400, 404 (RFQ inesistente, anche nell'anteprima), 503 (regole non pronte, senza toccare il database, con
//     il testo delle regole del servizio: S-05; troppi calcoli in corso: R-01), 504 (tempo scaduto, anche nell'anteprima
//     e negli HEAD: HTTP-01), 500 (testo generico, senza il dettaglio), sempre JSON con no-store e nosniff, anche
//     quando un valore non si codifica (HTTP-04: l'errore nel log);
//   - HEAD: le regole e il solo GetThread, nessun calcolo, con il codice della GET (400 per un componente non valido
//     dell'anteprima: HTTP-02);
//   - senza sessione e senza htmx la risposta è il 302 HTML verso il login, non JSON (HTTP-05: il client lo dice nel
//     README);
//   - l'anteprima: i rifiuti della richiesta sono 200 con «errore» e non leggono il database; la deriva delle frasi
//     contro il modulo legacy fasc_autorizzazione reso dal suo template.
// La catena intera con le sessioni vere, la sola lettura provata sul database e le GET senza scritture sono le L4
// (motore_a_db_test.go, nessunaget_db_test.go).
//
// I clienti di questi test sono inventati: codici 712xxxx, UUID 00000000-0000-4000-8000-0000000000nn.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	htmlesc "html"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// mwUUID: l'UUID ACME «00000000-0000-4000-8000-0000000000nn».
func mwUUID(nn int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", nn))
}

// mwRotte: le due GET di A1d-P2 sotto /thread/{id}.
var mwRotte = []string{"/motore-a/dati", "/motore-a/anteprima/step-strutturale?componente=" + mwUUID(10).String()}

// mwServer: il server delle prove della porta, con i template, il log scartato e le regole date. Pool nil.
func mwServer(t *testing.T, r motorea_web.Regole) *Server {
	t.Helper()
	s := serverTest(t)
	s.Log = slog.New(slog.DiscardHandler)
	s.MotoreA = r
	return s
}

// mwServi: una richiesta servita da h, con il panic di una lettura del database recuperato e restituito.
func mwServi(h http.Handler, req *http.Request) (rec *httptest.ResponseRecorder, panico any) {
	rec = httptest.NewRecorder()
	func() {
		defer func() { panico = recover() }()
		h.ServeHTTP(rec, req)
	}()
	return rec, panico
}

// mwPronte: regole pronte (un insieme vuoto ma non nil), con il limite dato.
func mwPronte(limite time.Duration) motorea_web.Regole {
	return motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta, Insieme: &motorea.InsiemeRegole{}, Limite: limite}
}

// A flag spento ogni rotta di motore-a risponde come un indirizzo che non esiste: lo stesso codice, le stesse intestazioni
// e lo stesso corpo di un percorso che nessun pattern riconosce, con ogni metodo, con e senza cookie e htmx, anche con un
// id che non è un UUID. Nessuna intestazione in più (niente no-store, niente Allow, niente rimando al login) e nessuna
// lettura della sessione (Pool nil).
func TestMotoreASpentoComeUnIndirizzoCheNonCe(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Stato: motorea_web.StatoSpenta})
	mux := http.NewServeMux()
	s.Registra(mux)
	for _, thread := range []string{mwUUID(1).String(), "xyz"} {
		for _, rotta := range mwRotte {
			for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
				http.MethodPatch, http.MethodOptions} {
				for _, cookie := range []bool{false, true} {
					for _, hx := range []bool{false, true} {
						nome := fmt.Sprintf("%s %s%s cookie=%v htmx=%v", metodo, thread, rotta, cookie, hx)
						richiesta := func(percorso string) *http.Request {
							req := httptest.NewRequest(metodo, percorso, nil)
							if cookie {
								req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
							}
							if hx {
								req.Header.Set("HX-Request", "true")
							}
							return req
						}
						rec, panico := mwServi(mux, richiesta("/thread/"+thread+rotta))
						if panico != nil {
							t.Errorf("%s: la porta ha toccato la sessione (Pool nil): %v", nome, panico)
							continue
						}
						rif, _ := mwServi(mux, richiesta("/a1d-indirizzo-che-non-esiste"+strings.ReplaceAll(rotta, "/", "-")))
						if rec.Code != http.StatusNotFound || rif.Code != http.StatusNotFound {
							t.Errorf("%s: stato %d (riferimento %d), atteso 404", nome, rec.Code, rif.Code)
						}
						if a, b := mwIntestazioni(rec.Header()), mwIntestazioni(rif.Header()); a != b {
							t.Errorf("%s: intestazioni %s, quelle di un indirizzo che non esiste %s", nome, a, b)
						}
						if rec.Body.String() != rif.Body.String() {
							t.Errorf("%s: corpo %q, quello di un indirizzo che non esiste %q", nome, rec.Body, rif.Body)
						}
					}
				}
			}
		}
	}
}

// mwIntestazioni: le intestazioni in una riga, in ordine.
func mwIntestazioni(h http.Header) string {
	var out []string
	for k, v := range h {
		out = append(out, k+"="+strings.Join(v, ","))
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// A flag acceso, senza sessione: 302 verso /login, o 401 con HX-Redirect per htmx (aLogin), con no-store e senza
// leggere il database; mai 404. Gli altri metodi: 405 con Allow e no-store, senza sessione.
func TestMotoreAAccesoSenzaSessioneEAltriMetodi(t *testing.T) {
	s := mwServer(t, mwPronte(time.Second))
	mux := http.NewServeMux()
	s.Registra(mux)
	for _, rotta := range mwRotte {
		for _, hx := range []bool{false, true} {
			req := httptest.NewRequest(http.MethodGet, "/thread/"+mwUUID(1).String()+rotta, nil)
			if hx {
				req.Header.Set("HX-Request", "true")
			}
			rec, panico := mwServi(mux, req)
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
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("%s htmx=%v: Cache-Control %q, atteso no-store", rotta, hx, cc)
			}
			// HTTP-05: la risposta di aLogin non è JSON; il client che usa fetch lo riconosce dal Content-Type o da
			// res.redirected (README del modulo), oppure manda HX-Request e riceve 401 con HX-Redirect
			if strings.Contains(rec.Header().Get("Content-Type"), "json") {
				t.Errorf("%s htmx=%v: Content-Type %q: il rimando al login non è JSON", rotta, hx, rec.Header().Get("Content-Type"))
			}
		}
		for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			req := httptest.NewRequest(metodo, "/thread/"+mwUUID(1).String()+rotta, nil)
			req.AddCookie(&http.Cookie{Name: cookieSessione, Value: "sessione-acme-di-prova"})
			rec, panico := mwServi(mux, req)
			if panico != nil {
				t.Errorf("%s %s: ha letto la sessione: %v", metodo, rotta, panico)
				continue
			}
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s %s: %d, Allow %q, Cache-Control %q", metodo, rotta, rec.Code, rec.Header().Get("Allow"), rec.Header().Get("Cache-Control"))
			}
		}
	}
}

// Il ruolo, dietro la porta accesa: la catena di registraMotoreA meno autenticato (che legge la sessione dal DB):
// conMotoreA(conNoStore(soloRuolo(admin, gestore))). Chi non è admin riceve il 403 HTML di nega, con no-store, e il
// gestore non parte; l'admin arriva al gestore (qui: regole non pronte, 503 JSON). Che le rotte siano montate proprio
// così lo controlla TestMotoreALeRotteSiMontano; la catena con le sessioni vere è nelle L4.
func TestMotoreASoloLAdmin(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	id := mwUUID(1).String()
	for _, c := range []struct {
		nome, percorso string
		h              http.HandlerFunc
	}{
		{"dati", "/thread/" + id + "/motore-a/dati", s.motoreADati},
		{"anteprima", "/thread/" + id + "/motore-a/anteprima/step-strutturale?componente=xyz", s.motoreAAnteprimaStep},
	} {
		porta := s.conMotoreA(conNoStore(s.soloRuolo(db.RuoloUtenteAdmin, c.h)))
		for _, ruolo := range db.AllRuoloUtenteValues() {
			req := httptest.NewRequest(http.MethodGet, c.percorso, nil)
			req.SetPathValue("id", id)
			req.Header.Set("HX-Request", "true")
			req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "acme", Ruolo: ruolo}))
			rec, panico := mwServi(porta, req)
			if panico != nil {
				t.Errorf("%s, %s: panic (il gestore ha letto il DB?): %v", c.nome, ruolo, panico)
				continue
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("%s, %s: Cache-Control %q", c.nome, ruolo, rec.Header().Get("Cache-Control"))
			}
			if ruolo == db.RuoloUtenteAdmin {
				if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") || rec.Code == http.StatusForbidden {
					t.Errorf("%s, admin: %d %q, atteso il JSON del gestore", c.nome, rec.Code, rec.Header().Get("Content-Type"))
				}
				continue
			}
			if rec.Code != http.StatusForbidden || strings.Contains(rec.Header().Get("Content-Type"), "json") {
				t.Errorf("%s, %s: %d %q, atteso il 403 HTML di nega", c.nome, ruolo, rec.Code, rec.Header().Get("Content-Type"))
			}
		}
	}
}

// Registra intero non va in panic e le due GET arrivano ai loro pattern, anche con HEAD; gli altri metodi alla riga
// gemella. Nel sorgente ogni GET di registraMotoreA è montata come conMotoreA(conNoStore(soloAdmin(gestore))) e ogni
// riga gemella come conMotoreA(conNoStore(metodoNonAmmessoMotoreA)): quattro schemi, due GET. La pagina
// /thread/{id}/motore-a non è montata in A1d-P2, e la funzione etichetta non è fra quelle dei template (arrivano con P4).
func TestMotoreALeRotteSiMontano(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{})
	mux := http.NewServeMux()
	func() {
		defer func() {
			if v := recover(); v != nil {
				t.Fatalf("Registra va in panic (un pattern in conflitto?): %v", v)
			}
		}()
		s.Registra(mux)
	}()
	id := mwUUID(1).String()
	for _, c := range []struct{ percorso, pattern string }{
		{"/thread/" + id + "/motore-a/dati", "GET /thread/{id}/motore-a/dati"},
		{"/thread/" + id + "/motore-a/anteprima/step-strutturale?componente=" + mwUUID(10).String(), "GET /thread/{id}/motore-a/anteprima/step-strutturale"},
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
	if _, pat := mux.Handler(httptest.NewRequest(http.MethodGet, "/thread/"+id+"/motore-a", nil)); strings.Contains(pat, "motore-a") {
		t.Errorf("la pagina /thread/{id}/motore-a è montata (%q): arriva con A1d-P4", pat)
	}
	if _, ok := funzioni["etichetta"]; ok {
		t.Error("la funzione etichetta è fra quelle dei template: arriva con A1d-P4")
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "motore_a.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	reGestore := regexp.MustCompile(`^s\.conMotoreA\(conNoStore\(s\.soloAdmin\(s\.[A-Za-z]+\)\)\)$`)
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
				pattern, _ := strconv.Unquote(types.ExprString(c.Args[0]))
				g := types.ExprString(c.Args[1])
				switch {
				case strings.HasPrefix(pattern, "GET "):
					get = append(get, strings.TrimPrefix(pattern, "GET "))
					if !reGestore.MatchString(g) {
						t.Errorf("registraMotoreA: %s montata con %s", pattern, g)
					}
				case strings.HasPrefix(pattern, "/"):
					gemelle = append(gemelle, pattern)
					if g != "s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA))" {
						t.Errorf("registraMotoreA: la riga gemella %s montata con %s", pattern, g)
					}
				default:
					t.Errorf("registraMotoreA: %s non è né una GET né una riga gemella", pattern)
				}
			}
			return true
		})
	}
	sort.Strings(get)
	sort.Strings(gemelle)
	if montate != 4 || !slices.Equal(get, gemelle) ||
		!slices.Equal(get, []string{"/thread/{id}/motore-a/anteprima/step-strutturale", "/thread/{id}/motore-a/dati"}) {
		t.Errorf("registraMotoreA monta %d schemi: GET %v, gemelle %v; attese le due rotte JSON con le loro gemelle", montate, get, gemelle)
	}
}

// ---------------------------------------------------------------- un database finto, per i codici

// mwTx: una transazione finta. Ogni lettura registra il testo; GetThread risponde con getThread; le altre letture con
// altro. Begin (il savepoint del caricatore) non serve ai casi di queste prove.
type mwTx struct {
	pgx.Tx
	getThread, altro error
	sql              []string
	rollback, commit int
}

func (x *mwTx) risposta(sql string) error {
	x.sql = append(x.sql, sql)
	if strings.Contains(sql, "name: GetThread ") {
		return x.getThread
	}
	return x.altro
}

func (x *mwTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return mwRiga{x.risposta(sql)}
}

func (x *mwTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	return nil, x.risposta(sql)
}

func (x *mwTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, x.risposta(sql)
}

func (x *mwTx) Rollback(context.Context) error { x.rollback++; return nil }
func (x *mwTx) Commit(context.Context) error   { x.commit++; return nil }

type mwRiga struct{ err error }

func (r mwRiga) Scan(...any) error { return r.err }

// mwIniziatore: l'iniziatore finto. Con blocca, BeginTx aspetta la fine del contesto (il tempo scaduto). aperture è
// atomico: con blocca BeginTx gira nella goroutine del calcolo, che la prova legge dopo il tempo scaduto senza
// sincronizzarsi (Q2-06 a della revisione di A1d-P2: una corsa dati che -race segnalerebbe).
type mwIniziatore struct {
	tx       *mwTx
	errore   error
	blocca   bool
	aperture atomic.Int32
}

func (i *mwIniziatore) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	i.aperture.Add(1)
	if i.blocca {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if i.errore != nil {
		return nil, i.errore
	}
	return i.tx, nil
}

// mwChiama: un gestore con il servizio dato, l'utente admin nel contesto e {id} nel percorso.
func mwChiama(t *testing.T, s *Server, gestore func(http.ResponseWriter, *http.Request, motorea_web.Servizio), svc motorea_web.Servizio,
	metodo, id, resto string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(metodo, "/thread/"+id+resto, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "AD", Ruolo: db.RuoloUtenteAdmin}))
	rec, panico := mwServi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gestore(w, r, svc) }), req)
	if panico != nil {
		t.Fatalf("%s %s: panic: %v", metodo, resto, panico)
	}
	return rec
}

// mwErrore: il JSON {"errore": …} di una risposta, controllando Content-Type, no-store e nosniff.
func mwErrore(t *testing.T, nome string, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("%s: intestazioni %s", nome, mwIntestazioni(rec.Header()))
	}
	var e struct {
		Errore string `json:"errore"`
	}
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		t.Errorf("%s: il corpo non è {\"errore\"}: %v: %s", nome, err, rec.Body)
	}
	return e.Errore
}

// I codici di /dati e HEAD, con il servizio passato al gestore (un iniziatore finto al posto del pool).
func TestMotoreADatiICodici(t *testing.T) {
	var registro bytes.Buffer
	s := mwServer(t, motorea_web.Regole{})
	s.Log = slog.New(slog.NewTextHandler(&registro, nil))
	id := mwUUID(1).String()
	segreto := errors.New("dettaglio tecnico ACME-SEGRETO")
	for _, c := range []struct {
		nome     string
		metodo   string
		regole   motorea_web.Regole
		ini      *mwIniziatore
		id       string
		stato    int
		testo    string
		aperture int32
		letture  []string
	}{
		{nome: "id non valido", metodo: http.MethodGet, regole: mwPronte(time.Second), ini: &mwIniziatore{tx: &mwTx{}}, id: "xyz",
			stato: http.StatusBadRequest, testo: "id non valido"},
		{nome: "regole non pronte", metodo: http.MethodGet, regole: motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoNessunaGrammatica},
			ini: &mwIniziatore{tx: &mwTx{}}, id: id, stato: http.StatusServiceUnavailable,
			testo: motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoNessunaGrammatica}.Errore()},
		{nome: "regole non pronte, HEAD", metodo: http.MethodHead, regole: motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente},
			ini: &mwIniziatore{tx: &mwTx{}}, id: id, stato: http.StatusServiceUnavailable},
		{nome: "RFQ inesistente", metodo: http.MethodGet, regole: mwPronte(time.Second), ini: &mwIniziatore{tx: &mwTx{getThread: pgx.ErrNoRows}},
			id: id, stato: http.StatusNotFound, testo: testoMotoreARFQ, aperture: 1, letture: []string{"GetThread"}},
		{nome: "RFQ inesistente, HEAD", metodo: http.MethodHead, regole: mwPronte(time.Second), ini: &mwIniziatore{tx: &mwTx{getThread: pgx.ErrNoRows}},
			id: id, stato: http.StatusNotFound, aperture: 1, letture: []string{"GetThread"}},
		{nome: "la RFQ c'è, HEAD: nessun calcolo", metodo: http.MethodHead, regole: mwPronte(time.Second), ini: &mwIniziatore{tx: &mwTx{}},
			id: id, stato: http.StatusOK, aperture: 1, letture: []string{"GetThread"}},
		{nome: "il database", metodo: http.MethodGet, regole: mwPronte(time.Second), ini: &mwIniziatore{errore: segreto},
			id: id, stato: http.StatusInternalServerError, testo: testoMotoreAErrore, aperture: 1},
		{nome: "il tempo", metodo: http.MethodGet, regole: mwPronte(20 * time.Millisecond), ini: &mwIniziatore{blocca: true},
			id: id, stato: http.StatusGatewayTimeout, testo: testoMotoreATempo, aperture: 1},
		// HTTP-01: anche HEAD al limite è 504, non 500
		{nome: "il tempo, HEAD", metodo: http.MethodHead, regole: mwPronte(20 * time.Millisecond), ini: &mwIniziatore{blocca: true},
			id: id, stato: http.StatusGatewayTimeout, aperture: 1},
		{nome: "id non valido, HEAD", metodo: http.MethodHead, regole: mwPronte(time.Second), ini: &mwIniziatore{tx: &mwTx{}}, id: "xyz",
			stato: http.StatusBadRequest},
	} {
		t.Run(c.nome, func(t *testing.T) {
			s.MotoreA = c.regole
			svc := motorea_web.Servizio{Regole: c.regole, Pool: c.ini, Log: s.Log}
			rec := mwChiama(t, s, s.datiMotoreA, svc, c.metodo, c.id, "/motore-a/dati")
			if rec.Code != c.stato {
				t.Fatalf("stato %d, atteso %d: %s", rec.Code, c.stato, rec.Body)
			}
			if c.metodo == http.MethodHead {
				if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
					t.Errorf("HEAD: Content-Type %q", rec.Header().Get("Content-Type"))
				}
			} else if got := mwErrore(t, c.nome, rec); got != c.testo {
				t.Errorf("errore %q, atteso %q", got, c.testo)
			}
			if strings.Contains(rec.Body.String(), "ACME-SEGRETO") {
				t.Error("il dettaglio tecnico è arrivato al client")
			}
			if n := c.ini.aperture.Load(); n != c.aperture {
				t.Errorf("%d transazioni aperte, attese %d", n, c.aperture)
			}
			if c.ini.tx != nil {
				if len(c.ini.tx.sql) != len(c.letture) || (len(c.letture) == 1 && !strings.Contains(c.ini.tx.sql[0], "GetThread")) {
					t.Errorf("letture %q, attese %q", c.ini.tx.sql, c.letture)
				}
				if c.ini.tx.commit != 0 {
					t.Error("un commit")
				}
			}
		})
	}
	if !strings.Contains(registro.String(), "ACME-SEGRETO") {
		t.Error("il dettaglio dell'errore del database non è nel log")
	}
}

// L'anteprima: id, form e componente non validi sono 400; i rifiuti di richiestaAutorizzazione (il codice del Fascicolo,
// riusato) sono 200 con «errore» e non aprono transazioni; una RFQ inesistente è 404; un errore del database 500 con il
// testo dell'anteprima; HEAD solo GetThread.
func TestMotoreAAnteprimaICodici(t *testing.T) {
	s := mwServer(t, mwPronte(time.Second))
	id := mwUUID(1).String()
	cid := "componente=" + mwUUID(10).String()
	segreto := errors.New("dettaglio tecnico ACME-SEGRETO")
	for _, c := range []struct {
		nome, metodo, id, query string
		ini                     *mwIniziatore
		stato                   int
		errore                  string
		aperture                int32
	}{
		{"id", http.MethodGet, "boh", cid, &mwIniziatore{tx: &mwTx{}}, 400, "id non valido", 0},
		{"form", http.MethodGet, id, "componente=%zz", &mwIniziatore{tx: &mwTx{}}, 400, "richiesta non valida", 0},
		{"componente", http.MethodGet, id, "componente=xyz", &mwIniziatore{tx: &mwTx{}}, 400, "componente non valido", 0},
		{"senza componente", http.MethodGet, id, "", &mwIniziatore{tx: &mwTx{}}, 400, "componente non valido", 0},
		{"documento", http.MethodGet, id, cid + "&documento=xyz", &mwIniziatore{tx: &mwTx{}}, 200, "documento non valido", 0},
		{"nodo senza chiave", http.MethodGet, id, cid + "&nodo=" + mwUUID(31).String() + ":", &mwIniziatore{tx: &mwTx{}}, 200, "nodo non valido", 0},
		{"file e nodo", http.MethodGet, id, cid + "&documento=" + mwUUID(41).String() + "&nodo=" + mwUUID(31).String() + ":%231",
			&mwIniziatore{tx: &mwTx{}}, 200, "si autorizza un file o si delega un nodo, non tutti e due", 0},
		{"RFQ inesistente", http.MethodGet, id, cid + "&documento=" + mwUUID(41).String(), &mwIniziatore{tx: &mwTx{getThread: pgx.ErrNoRows}},
			404, testoMotoreARFQ, 1},
		{"il componente non è della RFQ", http.MethodGet, id, cid + "&documento=" + mwUUID(41).String(), &mwIniziatore{tx: &mwTx{altro: pgx.ErrNoRows}},
			200, "il componente non è di questa RFQ", 1},
		{"il database", http.MethodGet, id, cid, &mwIniziatore{errore: segreto}, 500, testoMotoreAAnteprim, 1},
		{"HEAD", http.MethodHead, id, cid, &mwIniziatore{tx: &mwTx{}}, 200, "", 1},
		{"HEAD, RFQ inesistente", http.MethodHead, id, cid, &mwIniziatore{tx: &mwTx{getThread: pgx.ErrNoRows}}, 404, "", 1},
		// HTTP-02: HEAD risponde 400 dove la GET risponde 400, senza leggere
		{"HEAD, componente non valido", http.MethodHead, id, "componente=xyz", &mwIniziatore{tx: &mwTx{}}, 400, "", 0},
		{"HEAD, senza componente", http.MethodHead, id, "", &mwIniziatore{tx: &mwTx{}}, 400, "", 0},
		{"HEAD, query illeggibile", http.MethodHead, id, "componente=%zz", &mwIniziatore{tx: &mwTx{}}, 400, "", 0},
		{"HEAD, id non valido", http.MethodHead, "boh", cid, &mwIniziatore{tx: &mwTx{}}, 400, "", 0},
	} {
		t.Run(c.nome, func(t *testing.T) {
			svc := motorea_web.Servizio{Regole: s.MotoreA, Pool: c.ini, Log: s.Log}
			rec := mwChiama(t, s, s.anteprimaStepMotoreA, svc, c.metodo, c.id, "/motore-a/anteprima/step-strutturale?"+c.query)
			if rec.Code != c.stato {
				t.Fatalf("stato %d, atteso %d: %s", rec.Code, c.stato, rec.Body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
				rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
				t.Errorf("intestazioni %s", mwIntestazioni(rec.Header()))
			}
			if n := c.ini.aperture.Load(); n != c.aperture {
				t.Errorf("%d transazioni aperte, attese %d", n, c.aperture)
			}
			if c.metodo == http.MethodHead {
				if c.ini.tx != nil && len(c.ini.tx.sql) != int(c.aperture) {
					t.Errorf("HEAD: letture %q, atteso solo GetThread se la richiesta è valida", c.ini.tx.sql)
				}
				return
			}
			var out map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			var errore string
			_ = json.Unmarshal(out["errore"], &errore)
			if errore != c.errore || strings.Contains(rec.Body.String(), "ACME-SEGRETO") {
				t.Errorf("errore %q, atteso %q (corpo %s)", errore, c.errore, rec.Body)
			}
			if c.stato == 200 {
				// un rifiuto ha la forma dell'anteprima, con il motore e gli elenchi vuoti
				if string(out["motore"]) != `"`+motorea_web.MotoreAnteprima+`"` || string(out["frasi"]) != "[]" || string(out["figli"]) != "[]" {
					t.Errorf("il rifiuto non ha la forma dell'anteprima: %s", rec.Body)
				}
			}
		})
	}
}

// L'anteprima al limite (HTTP-01): 504 con il testo dell'anteprima, sia GET sia HEAD, e un log Warn (non Error).
func TestMotoreAAnteprimaIlTempo(t *testing.T) {
	var registro bytes.Buffer
	s := mwServer(t, mwPronte(20*time.Millisecond))
	s.Log = slog.New(slog.NewTextHandler(&registro, nil))
	cid := "componente=" + mwUUID(10).String()
	for _, metodo := range []string{http.MethodGet, http.MethodHead} {
		ini := &mwIniziatore{blocca: true}
		svc := motorea_web.Servizio{Regole: s.MotoreA, Pool: ini, Log: s.Log}
		rec := mwChiama(t, s, s.anteprimaStepMotoreA, svc, metodo, mwUUID(1).String(), "/motore-a/anteprima/step-strutturale?"+cid+"&documento="+mwUUID(41).String())
		if rec.Code != http.StatusGatewayTimeout {
			t.Fatalf("%s: stato %d, atteso 504: %s", metodo, rec.Code, rec.Body)
		}
		if metodo == http.MethodGet {
			if got := mwErrore(t, "anteprima al limite", rec); got != testoMotoreATempoAnteprim {
				t.Errorf("errore %q, atteso %q", got, testoMotoreATempoAnteprim)
			}
		}
	}
	if !strings.Contains(registro.String(), "level=WARN") || strings.Contains(registro.String(), "level=ERROR") {
		t.Errorf("il tempo scaduto dell'anteprima va nel log come avviso, non come errore:\n%s", registro.String())
	}
}

// Le regole del 503 sono quelle del servizio della richiesta, non quelle del Server (S-05); troppi calcoli in corso è
// 503 con il suo testo (R-01).
func TestMotoreAIl503(t *testing.T) {
	s := mwServer(t, motorea_web.Regole{})
	assente := motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente}
	for _, c := range []struct {
		nome  string
		err   error
		testo string
	}{
		{"regole non pronte", motorea_web.ErrRegole, assente.Errore()},
		{"troppi calcoli", fmt.Errorf("%w (al più 2 insieme)", motorea_web.ErrOccupato), testoMotoreAOccupato},
	} {
		req := httptest.NewRequest(http.MethodGet, "/thread/"+mwUUID(1).String()+"/motore-a/dati", nil)
		rec := httptest.NewRecorder()
		s.erroreMotoreA(rec, req, mwUUID(1), motorea_web.Servizio{Regole: assente}, c.err)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: stato %d", c.nome, rec.Code)
		}
		if got := mwErrore(t, c.nome, rec); got != c.testo || got == "" {
			t.Errorf("%s: errore %q, atteso %q", c.nome, got, c.testo)
		}
	}
}

// scriviJSONMotoreA: il buffer, poi le intestazioni e lo stato; un valore che non si codifica è 500, mai un JSON a metà:
// il corpo è il JSON d'errore generico, con no-store e nosniff, e l'errore va nel log con il percorso (HTTP-04).
func TestMotoreAIlJSONInUnBuffer(t *testing.T) {
	var registro bytes.Buffer
	s := mwServer(t, motorea_web.Regole{})
	s.Log = slog.New(slog.NewTextHandler(&registro, nil))
	req := httptest.NewRequest(http.MethodGet, "/thread/"+mwUUID(1).String()+"/motore-a/dati", nil)
	rec := httptest.NewRecorder()
	s.scriviJSONMotoreA(rec, req, http.StatusOK, map[string]any{"a": 1})
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		strings.TrimSpace(rec.Body.String()) != `{"a":1}` {
		t.Errorf("%d %s %q", rec.Code, mwIntestazioni(rec.Header()), rec.Body)
	}
	rec = httptest.NewRecorder()
	s.scriviJSONMotoreA(rec, req, http.StatusOK, map[string]any{"a": func() {}})
	if rec.Code != 500 {
		t.Fatalf("un valore che non si codifica: %d %q", rec.Code, rec.Body)
	}
	if got := mwErrore(t, "un valore che non si codifica", rec); got != testoMotoreAErrore {
		t.Errorf("un valore che non si codifica: errore %q, atteso %q", got, testoMotoreAErrore)
	}
	if !strings.Contains(registro.String(), "level=ERROR") || !strings.Contains(registro.String(), "/motore-a/dati") {
		t.Errorf("l'errore di codifica non è nel log con il percorso:\n%s", registro.String())
	}
	// il corpo di ripiego è il testo generico, scritto a mano: deve restare uguale alla costante
	var e struct{ Errore string }
	if err := json.Unmarshal([]byte(corpoMotoreANonCodificato), &e); err != nil || e.Errore != testoMotoreAErrore {
		t.Errorf("corpoMotoreANonCodificato %q non è {\"errore\": testoMotoreAErrore}: %v", corpoMotoreANonCodificato, err)
	}
}

// ---------------------------------------------------------------- la deriva dell'anteprima

// mwEffetti: gli effetti ACME della deriva, uno per forma del riquadro (gli stessi delle prove del sottopackage).
func mwEffetti() map[string]fascicolo.Effetto {
	prodotto := db.Componente{ComponenteID: mwUUID(10), ThreadID: mwUUID(1), Codice: "7120100", Tipo: db.TipoComponenteFinito}
	sotto := db.Componente{ComponenteID: mwUUID(11), ThreadID: mwUUID(1), Codice: "7120110", Tipo: db.TipoComponenteSottoassieme}
	doc := db.Documento{DocumentoID: mwUUID(41), ThreadID: mwUUID(1), NomeFile: "7120100_C.stp"}
	const firma = "a1d0a1d0a1d0a1d0a1d0a1d0"
	radice := fascicolo.NodoEffetto{Chiave: "#1", Nome: "7120100", Codice: "7120100", Figli: 3, Stato: db.StatoPropostaAperta}
	figli := []fascicolo.NodoEffetto{
		{Chiave: "#2", Nome: "7120110", Codice: "7120110", Qta: 2, Figli: 1, Deciso: "7120110", Stato: db.StatoPropostaConfermata},
		{Chiave: "#3", Nome: "7129020", Codice: "7129020", Qta: 8, Stato: db.StatoPropostaAperta},
		{Chiave: "#4", Nome: "ACME-CONTENITORE", Figli: 2, Stato: db.StatoPropostaAperta},
	}
	nodo6 := fascicolo.NodoEffetto{Chiave: "#6", Nome: "7120160", Codice: "7120160", Qta: 1, Figli: 2, Stato: db.StatoPropostaAperta}
	nodo7 := fascicolo.NodoEffetto{Chiave: "#7", Nome: "7120170", Codice: "7120170", Qta: 1, Stato: db.StatoPropostaAperta}
	delegaDecisa := fascicolo.DelegaPossibile{Nodo: figli[0], ComponenteID: mwUUID(11), Codice: "7120110",
		Revoca: []string{"7120110_A.stp (STEP autorizzato di 7120110)"}}
	delegaSpenta := fascicolo.DelegaPossibile{Nodo: nodo6, Spenta: "il nodo non è ancora un componente: si delega quando una persona lo decide"}
	delegato := figli[0]
	return map[string]fascicolo.Effetto{
		"spento": {Componente: prodotto, Spento: "7120100 è archiviato: si ripristina prima di autorizzare un suo STEP", Firma: firma},
		"radici": {Componente: prodotto, Documento: doc, Allegato: mwUUID(31), NomeFile: "7120100_C.stp",
			Radici: []fascicolo.NodoEffetto{radice, {Chiave: "#5", Nome: "7120105", Codice: "7120105", Figli: 1, Stato: db.StatoPropostaAperta}},
			Firma:  firma},
		"sorgente": {Componente: prodotto, Documento: doc, Allegato: mwUUID(31), NomeFile: "7120100_C.stp",
			Radici: []fascicolo.NodoEffetto{radice}, Sorgente: &radice,
			PresaDAtto:     "il file chiama la radice «PRODOTTO ACME»: per questa RFQ è 7120100",
			Raggruppamenti: []fascicolo.Raggruppamento{{Nodo: figli[2], Figli: []fascicolo.NodoEffetto{nodo6, nodo7}}},
			Figli:          figli, Deleghe: []fascicolo.DelegaPossibile{delegaDecisa, delegaSpenta},
			Guida: 3, GuidaSotto: []string{"sotto 7120110: 1"},
			Rimozioni: []string{"7120100 → 7120200 ×1: il file non ha il legame"},
			Revoche:   []string{"7120100_B.stp (STEP autorizzato di 7120100)"},
			Avvisi:    []string{"è un prodotto finito: il file diventa anche il suo STEP strutturale"},
			Firma:     firma},
		"gia_con_scelte": {Componente: prodotto, Documento: doc, Allegato: mwUUID(31), NomeFile: "7120100_C.stp",
			Gia:    "7120100_C.stp è già lo STEP autorizzato di 7120100",
			Radici: []fascicolo.NodoEffetto{radice}, Sorgente: &radice, Figli: figli[:1],
			Deleghe: []fascicolo.DelegaPossibile{delegaDecisa}, Guida: 1,
			RimozioniSospese: "si calcolano quando una persona ha deciso tutti i figli diretti", Firma: firma},
		"delega": {Componente: sotto, Documento: doc, Titolare: "7120100", Allegato: mwUUID(31),
			NomeFile: "7120100_C.stp", Delega: true, Sorgente: &delegato,
			Figli: []fascicolo.NodoEffetto{{Chiave: "#8", Nome: "7120111", Codice: "7120111", Qta: 4, Stato: db.StatoPropostaAperta}},
			Guida: 5, Firma: firma},
	}
}

// La deriva (T-A1d-28): lo stesso Effetto reso dal modulo legacy come lo rende anteprimaAutorizzazione (con Scrive vero).
// I paragrafi del modulo sono, nello stesso ordine, le frasi del JSON (più il motivo del riquadro spento); le radici da
// indicare sono le voci dell'elenco; raggruppamenti, deleghe, presa d'atto sono etichette del modulo; il motivo di una
// delega spenta e le sue revoche sono note del modulo; il bottone c'è quando il JSON dice inviabile, con lo stesso testo.
// Se il template legacy cambia una riga, la prova cade.
func TestMotoreAAnteprimaLeFrasiSonoQuelleDelModuloLegacy(t *testing.T) {
	s := serverTest(t)
	tag := regexp.MustCompile(`<[^>]*>`)
	testo := func(h string) string {
		return strings.Join(strings.Fields(htmlesc.UnescapeString(tag.ReplaceAllString(h, " "))), " ")
	}
	elementi := func(h, nome string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`(?s)<`+nome+`\b[^>]*>(.*?)</`+nome+`>`).FindAllStringSubmatch(h, -1) {
			out = append(out, testo(m[1]))
		}
		return out
	}
	thread := mwUUID(1)
	for caso, e := range mwEffetti() {
		t.Run(caso, func(t *testing.T) {
			v := autorizzazioneVista{Base: "/thread/" + thread.String() + "/fascicolo", Cid: e.Componente.ComponenteID, E: e, Scrive: true}
			var buf bytes.Buffer
			if err := s.pagine["fascicolo.html"].ExecuteTemplate(&buf, "fasc_autorizzazione", vista{Dati: v, Frammento: true}); err != nil {
				t.Fatal(err)
			}
			h := buf.String()
			out := motorea_web.AnteprimaDa(e, true)
			paragrafi := slices.Clone(out.Frasi)
			if out.Spento != "" {
				paragrafi = append(paragrafi, out.Spento)
			}
			if got := elementi(h, "p"); !slices.Equal(got, paragrafi) {
				t.Errorf("paragrafi del modulo legacy:\n %q\nfrasi del JSON:\n %q", got, paragrafi)
			}
			if e.Sorgente == nil {
				var et []string
				for _, x := range out.Sorgenti {
					et = append(et, x.Etichetta)
				}
				if got := elementi(h, "li"); !slices.Equal(got, et) {
					t.Errorf("radici del modulo legacy %q, sorgenti del JSON %q", got, et)
				}
			}
			etichette := elementi(h, "label")
			note := elementi(h, "small")
			for _, x := range out.Raggruppamenti {
				if !slices.Contains(etichette, x.Etichetta) {
					t.Errorf("raggruppamento %q non è un'etichetta del modulo legacy %q", x.Etichetta, etichette)
				}
			}
			for _, d := range out.Deleghe {
				if !slices.Contains(etichette, d.Etichetta) {
					t.Errorf("delega %q non è un'etichetta del modulo legacy %q", d.Etichetta, etichette)
				}
				if d.Spenta != "" && !slices.Contains(note, d.Spenta) {
					t.Errorf("delega spenta %q non è una nota del modulo legacy %q", d.Spenta, note)
				}
				for _, x := range d.Revoca {
					if r := "Un componente, un'autorizzazione: " + x + " si revoca."; !slices.Contains(note, r) {
						t.Errorf("revoca della delega %q non è una nota del modulo legacy %q", r, note)
					}
				}
			}
			if out.PresaDAtto != "" && !slices.Contains(etichette, "Presa d'atto: "+out.PresaDAtto) {
				t.Errorf("presa d'atto %q non è un'etichetta del modulo legacy %q", out.PresaDAtto, etichette)
			}
			bottoni := elementi(h, "button")
			if out.Inviabile != (len(bottoni) == 1) || (out.Inviabile && bottoni[0] != out.Bottone) {
				t.Errorf("bottoni del modulo legacy %q; inviabile %v, bottone %q", bottoni, out.Inviabile, out.Bottone)
			}
		})
	}
}

// Il sorgente dell'adattatore non scrive e non nomina la tabella dell'agente: nessuna query di scrittura, nessun Exec,
// nessun Commit, nessuna coda. (Le letture stanno nel sottopackage; la stessa guardia, sul DB vero, è nelle L4.)
func TestMotoreAAdattatoreSenzaScritture(t *testing.T) {
	b, err := readFileMW("motore_a.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, vietato := range []string{"Exec(", "Commit(", "Insert", "Update", "Delete", "Accoda", "coda.", "analisi" + "_messaggio", "Begin("} {
		if strings.Contains(b, vietato) {
			t.Errorf("motore_a.go contiene %q", vietato)
		}
	}
}

func readFileMW(nome string) (string, error) {
	f, err := http.Dir(".").Open(nome)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	return string(b), err
}
