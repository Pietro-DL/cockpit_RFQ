package web

// L1 — V1, corsia file: il registro delle rotte della Sorgente (PM23 PP-10, ROT-10 per le rotte di questa corsia;
// EM §10.9). Le rotte S1-S5 con le loro righe gemelle e il ramo JSON di «carica» (E1′) stanno in questa tabella, con la
// catena flag → no-store → admin; una rotta nuova della Sorgente, o una catena diversa, fa fallire la prova. Il
// registro di tutte le rotte del server (T-06) e' della corsia interfaccia.

import (
	"os"
	"regexp"
	"strings"
	"testing"
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
		"catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }",
		"gemella := func(allow string) http.HandlerFunc { return s.conMotoreA(conNoStore(metodoNonAmmessoSorgente(allow))) }",
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
