package web

// L1 — Fascicolo v3: i file statici. Il viewer della vista Documenti e' un modulo (fascicolo.mjs) che carica
// pdf.js (un altro modulo, un lavoratore, dei .wasm, le mappe dei caratteri): se il server li manda con il tipo
// sbagliato il browser non li esegue e la vista resta vuota, e il tipo non lo deve decidere il registro di
// Windows. I file con l'impronta nell'indirizzo, e la cartella di pdf.js con la sua versione, il browser li
// tiene; gli altri no. La copia di pdf.js e' quella dichiarata in VERSIONE.txt, byte per byte.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

func TestIFileStaticiHannoIlTipoGiustoELaCacheGiusta(t *testing.T) {
	s := serverTest(t)
	h := http.StripPrefix("/static/", s.statici())
	chiedi := func(percorso string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, percorso, nil))
		return rec
	}
	casi := []struct {
		percorso, tipo string
		immutabile     bool
	}{
		{"/static/fascicolo.mjs", "text/javascript; charset=utf-8", false},
		{s.statico("fascicolo.mjs"), "text/javascript; charset=utf-8", true},
		{"/static/style.css", "text/css; charset=utf-8", false},
		{"/static/pdfjs-6.3.289/pdf.min.mjs", "text/javascript; charset=utf-8", true},
		{"/static/pdfjs-6.3.289/pdf.worker.min.mjs", "text/javascript; charset=utf-8", true},
		{"/static/pdfjs-6.3.289/wasm/openjpeg.wasm", "application/wasm", true},
		{"/static/pdfjs-6.3.289/wasm/jbig2.wasm", "application/wasm", true},
		{"/static/pdfjs-6.3.289/cmaps/78-H.bcmap", "application/octet-stream", true},
	}
	for _, c := range casi {
		rec := chiedi(c.percorso)
		if rec.Code != 200 {
			t.Errorf("%s: %d", c.percorso, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.tipo {
			t.Errorf("%s: Content-Type %q, atteso %q", c.percorso, got, c.tipo)
		}
		immutabile := strings.Contains(rec.Header().Get("Cache-Control"), "immutable")
		if immutabile != c.immutabile {
			t.Errorf("%s: immutabile %v, atteso %v (%q)", c.percorso, immutabile, c.immutabile, rec.Header().Get("Cache-Control"))
		}
	}
	// un font standard di pdf.js, qualunque sia
	fonts, _ := fs.ReadDir(risorse.FS, "web/static/pdfjs-6.3.289/standard_fonts")
	for _, f := range fonts {
		nome := f.Name()
		atteso := map[string]string{".pfb": "application/octet-stream", ".ttf": "font/ttf"}[path.Ext(nome)]
		if atteso == "" {
			continue
		}
		if got := chiedi("/static/pdfjs-6.3.289/standard_fonts/" + nome).Header().Get("Content-Type"); got != atteso {
			t.Errorf("%s: %q", nome, got)
		}
	}

	// l'impronta e' quella del contenuto
	b, err := fs.ReadFile(risorse.FS, "web/static/fascicolo.mjs")
	if err != nil {
		t.Fatal(err)
	}
	h6 := sha256.Sum256(b)
	if got, atteso := s.statico("fascicolo.mjs"), "/static/fascicolo.mjs?v="+hex.EncodeToString(h6[:6]); got != atteso {
		t.Errorf("statico: %q, atteso %q", got, atteso)
	}
	if got := s.statico("non-c-e.js"); got != "/static/non-c-e.js" {
		t.Errorf("un file che non c'e': %q", got)
	}
	// fuori dalla cartella statica non si esce
	for _, p := range []string{"/static/../templates/fascicolo.html", "/static/..%2ftemplates%2ffascicolo.html", "/static/pdfjs-6.3.289/../../templates/layout.html"} {
		if rec := chiedi(p); rec.Code == 200 && strings.Contains(rec.Body.String(), "{{define") {
			t.Errorf("%s: esce dalla cartella statica", p)
		}
	}
}

// La copia di pdf.js e' quella del pacchetto npm dichiarato: ogni file elencato in VERSIONE.txt c'e' con la sua
// impronta, e non c'e' niente che l'elenco non dica (una mappa .map, un file cambiato a mano).
func TestLaCopiaDiPdfjsEQuellaDichiarata(t *testing.T) {
	const dir = "web/static/pdfjs-6.3.289"
	elenco, err := fs.ReadFile(risorse.FS, dir+"/VERSIONE.txt")
	if err != nil {
		t.Fatal(err)
	}
	dichiarati := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(elenco)))
	for sc.Scan() {
		riga := strings.TrimSpace(sc.Text())
		sha, nome, ok := strings.Cut(riga, " *./")
		if !ok || len(sha) != 64 {
			continue
		}
		dichiarati[nome] = sha
	}
	if len(dichiarati) < 50 {
		t.Fatalf("VERSIONE.txt elenca %d file", len(dichiarati))
	}
	for _, nome := range []string{"pdf.min.mjs", "pdf.worker.min.mjs", "LICENSE", "wasm/openjpeg.wasm"} {
		if _, ok := dichiarati[nome]; !ok {
			t.Errorf("%s non e' nell'elenco", nome)
		}
	}
	trovati := 0
	err = fs.WalkDir(risorse.FS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		nome := strings.TrimPrefix(p, dir+"/")
		if nome == "VERSIONE.txt" {
			return nil
		}
		trovati++
		atteso, ok := dichiarati[nome]
		if !ok {
			t.Errorf("%s non e' nell'elenco di VERSIONE.txt", nome)
			return nil
		}
		b, err := fs.ReadFile(risorse.FS, p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != atteso {
			t.Errorf("%s: il contenuto non e' quello dichiarato", nome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if trovati != len(dichiarati) {
		t.Errorf("file nella cartella %d, nell'elenco %d", trovati, len(dichiarati))
	}
}
