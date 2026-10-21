package web

// L1 — V1, corsia file: il registro delle rotte della Sorgente (PM23 PP-10, ROT-10 per le rotte di questa corsia;
// EM §10.9). Le rotte S1-S5 con le loro righe gemelle e il ramo JSON di «carica» (E1′) stanno in questa tabella, con la
// catena flag → no-store → admin; una rotta nuova della Sorgente, o una catena diversa, fa fallire la prova. Il
// registro di tutte le rotte del server (T-06) e' della corsia interfaccia.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// rotteSorgente: le rotte della corsia file, come le registra registraSorgente (metodo vuoto = la riga gemella).
var rotteSorgente = []struct{ metodo, percorso, gestore string }{
	{"GET", "/thread/{id}/sorgente", "catena(s.sorgenteModello)"},
	{"", "/thread/{id}/sorgente", `gemella("GET, HEAD")`},
	{"POST", "/thread/{id}/sorgente/file/{allegato}/scarta", "catena(s.dopoIlGesto(s.sorgenteScarta))"},
	{"", "/thread/{id}/sorgente/file/{allegato}/scarta", `gemella("POST")`},
	{"POST", "/thread/{id}/sorgente/file/{allegato}/altri-file", "catena(s.dopoIlGesto(s.sorgenteAltriFile))"},
	{"", "/thread/{id}/sorgente/file/{allegato}/altri-file", `gemella("POST")`},
	{"POST", "/thread/{id}/sorgente/file/{allegato}/riattiva", "catena(s.dopoIlGesto(s.sorgenteRiattiva))"},
	{"", "/thread/{id}/sorgente/file/{allegato}/riattiva", `gemella("POST")`},
	{"POST", "/thread/{id}/sorgente/file/{allegato}/riprova-rimozione", "catena(s.sorgenteRiprovaRimozione)"},
	{"", "/thread/{id}/sorgente/file/{allegato}/riprova-rimozione", `gemella("POST")`},
	{"GET", "/thread/{id}/sorgente/file/{allegato}/contenuto", "catena(s.sorgenteContenuto)"},
	{"", "/thread/{id}/sorgente/file/{allegato}/contenuto", `gemella("GET, HEAD")`},
}

func sorgenteDi(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func TestPP10RegistroDelleRotteDellaSorgente(t *testing.T) {
	src := sorgenteDi(t, "sorgente.go")
	reRotta := regexp.MustCompile(`mux\.HandleFunc\("((?:GET|POST|HEAD|PUT|DELETE|PATCH) )?(/thread/\{id\}/sorgente[^"]*)", ([^\n]+)\)\n`)
	registrate := map[string]string{}
	for _, m := range reRotta.FindAllStringSubmatch(src, -1) {
		registrate[strings.TrimSpace(m[1])+" "+m[2]] = m[3]
	}
	if len(registrate) != len(rotteSorgente) {
		t.Errorf("sorgente.go registra %d rotte della Sorgente, la tabella ne ha %d: %v", len(registrate), len(rotteSorgente), registrate)
	}
	for _, r := range rotteSorgente {
		chiave := r.metodo + " " + r.percorso
		if got, ok := registrate[chiave]; !ok || got != r.gestore {
			t.Errorf("%s: registrata con %q, attesa %q", chiave, got, r.gestore)
		}
	}
	for _, catena := range []string{
		"catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conXMotivo(conNoStore(s.soloAdmin(h)))) }",
		"gemella := func(allow string) http.HandlerFunc {\n\t\treturn s.conMotoreA(conXMotivo(conNoStore(metodoNonAmmessoSorgente(allow))))\n\t}",
	} {
		if !strings.Contains(src, catena) {
			t.Errorf("la catena delle rotte della Sorgente e' cambiata: manca %q", catena)
		}
	}
	// E1′: la rotta di «carica» sceglie il ramo; il ramo JSON ha la catena della Distinta nuova
	if !strings.Contains(sorgenteDi(t, "fascicolo_rotte.go"), `mux.HandleFunc("POST /thread/{id}/carica", s.caricaRFQ())`) {
		t.Error("POST /thread/{id}/carica non passa da caricaRFQ")
	}
	if !strings.Contains(sorgenteDi(t, "sorgente_carica.go"), "jsonRamo := s.conMotoreA(conNoStore(s.soloAdmin(s.dopoICaricamenti(s.caricaJSON))))") {
		t.Error("il ramo JSON di «carica» non ha la catena flag → no-store → admin")
	}
}

// TestConXMotivo (PE-29; rilievo della revisione): una risposta d'errore senza X-Motivo ne riceve uno per lo stato; una
// che lo ha gia' lo tiene; una riuscita non lo porta.
func TestConXMotivo(t *testing.T) {
	casi := []struct {
		nome   string
		h      http.HandlerFunc
		stato  int
		motivo string
	}{
		{"405 di http.Error", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusMethodNotAllowed)
		}, http.StatusMethodNotAllowed, "METODO_NON_AMMESSO"},
		{"403 di una pagina", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<p>no</p>"))
		}, http.StatusForbidden, "NON_AUTORIZZATO"},
		{"416 di ServeContent", func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("abc"))
		}, http.StatusRequestedRangeNotSatisfiable, "RANGE_NON_VALIDO"},
		{"410 con il suo codice", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Motivo", "FILE_SCARTATO")
			w.WriteHeader(http.StatusGone)
		}, http.StatusGone, "FILE_SCARTATO"},
		{"200", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }, http.StatusOK, ""},
	}
	for _, c := range casi {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("Range", "bytes=10-20")
		w := httptest.NewRecorder()
		conXMotivo(c.h)(w, r)
		if w.Code != c.stato || w.Header().Get("X-Motivo") != c.motivo {
			t.Errorf("%s: %d X-Motivo %q, attesi %d %q", c.nome, w.Code, w.Header().Get("X-Motivo"), c.stato, c.motivo)
		}
	}
}

// TestNomiDeiFileDiE1 (rilievo della revisione): i nomi che allegato.nome_file non accetta si riconoscono prima dello
// staging; il nome detto nella risposta e' testo valido e corto.
func TestNomiDeiFileDiE1(t *testing.T) {
	for nome, rifiutato := range map[string]bool{
		"ACME-1.pdf":                          false,
		"disegno finale (rev B).pdf":          false,
		strings.Repeat("è", 300):              false,
		strings.Repeat("A", 297) + ".pdf":     true,
		"ACME\x01.pdf":                        true,
		"ACME\x00.pdf":                        true,
		string([]byte{0x41, 0xff, 0x2e, 'p'}): true,
	} {
		if got := nomeNonCaricabile(nome) != ""; got != rifiutato {
			t.Errorf("nomeNonCaricabile(%q) rifiuta = %v, atteso %v", nome, got, rifiutato)
		}
	}
	d := nomeDetto(strings.Repeat("A", 200) + "\x01" + string([]byte{0xff}))
	if !utf8.ValidString(d) || strings.ContainsRune(d, '\x01') || utf8.RuneCountInString(d) > 121 {
		t.Errorf("nomeDetto: %q", d)
	}
}
